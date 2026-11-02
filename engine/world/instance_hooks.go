package world

import (
	"context"
	"strconv"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaInstanceData builds the instance_data argument for the Eluna
// INSTANCE_EVENT_* hooks. C++ pushes the ElunaInstanceAI's Lua data table
// (LuaEngine/LuaEngine.cpp PushInstanceData), the script's persistent
// per-instance storage. Go keeps no per-instance Lua data store — there is
// no ElunaInstanceAI shim and instance save data is never loaded — so the
// bridge passes an empty table. Documented delta; the map and player/go
// arguments are the live objects.
func luaInstanceData() *scripting.Object {
	return &scripting.Object{Type: "InstanceData", Fields: map[string]any{}}
}

// mapLive builds the live MapLive behind Lua Map objects (MapMethods.h):
// the DBC-backed snapshot from the wotlk store, the per-instance spawn
// mode from the instance table (the creature IsHeroic read, same row),
// the map-instance player roster (the luaPlayers pattern: worldReady
// sessions enumerated under sessionsMu, player objects built under it),
// and the zone weather writer. The world side supplies all four so the
// scripting package keeps its data-source decoupling (GuildLive/GroupLive
// pattern). A nil server yields a snapshot-only live object.
func (s *Server) mapLive(mapID, instanceID uint32) scripting.MapLive {
	live := scripting.MapLive{Load: func() (scripting.MapSnapshot, bool) {
		if s == nil || s.Data == nil {
			return scripting.MapSnapshot{}, false
		}
		entry, found, err := s.Data.Map(mapID)
		if err != nil || !found {
			return scripting.MapSnapshot{}, false
		}
		return scripting.MapSnapshot{
			Name:           entry.MapName,
			IsDungeon:      entry.IsDungeon(),
			IsRaid:         entry.IsRaid(),
			IsArena:        entry.IsBattleArena(),
			IsBattleground: entry.IsBattleground(),
		}, true
	}}
	if s == nil {
		return live
	}
	live.Difficulty = func(ctx context.Context) (int64, bool) {
		if instanceID == 0 || s.CharactersStore == nil || s.CharactersStore.DB == nil {
			return 0, false
		}
		var difficulty int64
		if err := s.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT difficulty FROM instance WHERE id = ?", instanceID).Scan(&difficulty); err != nil {
			return 0, false
		}
		return difficulty, true
	}
	live.Players = func() []scripting.MapPlayerLive {
		s.sessionsMu.RLock()
		defer s.sessionsMu.RUnlock()
		var out []scripting.MapPlayerLive
		for sess := range s.sessions {
			if sess == nil || !sess.worldReady.Load() || sess.player == nil {
				continue
			}
			if sess.player.Map != mapID || sess.player.InstanceID != instanceID {
				continue
			}
			player := sess.luaPlayer()
			if player == nil {
				continue
			}
			out = append(out, scripting.MapPlayerLive{
				Player: player,
				IsGM:   sess.player.ExtraFlags&playerExtraGMOn != 0,
				Team:   teamForRace(sess.player.Race),
			})
		}
		return out
	}
	live.SetWeather = func(ctx context.Context, zone, wtype uint32, grade float32) {
		s.setZoneWeather(ctx, zone, weatherType(wtype), grade)
	}
	live.WorldObject = func(ctx context.Context, guid uint64) *scripting.Object {
		return s.mapWorldObject(ctx, mapID, instanceID, guid)
	}
	// AreaID is Map::GetAreaId (Map.cpp:2595) via the same
	// zoneAndAreaID ladder the creature/GO GetAreaId legs use:
	// WMO area, else grid area, else the map entry's AreaTableID.
	live.AreaID = func(x, y, z float32) uint32 {
		_, areaID := s.zoneAndAreaID(mapID, x, y, z, 0)
		return areaID
	}
	return live
}

// mapWorldObject is the MapMethods.h GetWorldObject dispatch
// (ObjectGuid.h HighGuid values): player 0x0000 via the map-scoped
// session roster (C++ eObjectAccessor::GetPlayer(map, guid), not the
// global lookup), unit 0xF130 and vehicle 0xF150 via the live creature
// motion (C++ map->GetCreature; Go creature objects are motion-backed,
// so unspawned DB rows read nil like session luaCreature), pet 0xF140
// via the same motion map (pet motions live there), gameobject 0xF110
// via the map-scoped GO resolver below, corpse 0xF101 via the owner's
// session corpse row on this map instance (bones excluded, matching
// the loadCorpseObject model; offline owners read nil — Go has no
// session-free corpse object). Transport 0xF120 and MO-transport
// 0x1FC0 read nil: Go mints no 0xF120 GUIDs and transport spawns
// carry only wire GUIDs with no scripting object surface. Dynamic
// object 0xF100 reads nil: dynamic spells have no scripting object.
// Any other high part reads nil, the C++ default arm.
func (s *Server) mapWorldObject(ctx context.Context, mapID, instanceID uint32, guid uint64) *scripting.Object {
	if s == nil || guid == 0 {
		return nil
	}
	switch uint16(guid >> 48) {
	case 0x0000:
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil &&
			sess.player.Map == mapID && sess.player.InstanceID == instanceID {
			return sess.luaPlayer()
		}
		return nil
	case 0xF130, 0xF150, 0xF140:
		if motion := s.findCreatureMotion(mapID, instanceID, guid); motion != nil {
			return s.luaMotionCreature(motion)
		}
		return nil
	case 0xF110:
		return s.mapLuaGameObject(ctx, mapID, instanceID, guid)
	case 0xF101:
		ownerGUID := guid & 0xFFFFFFFFFFFF
		sess := s.findSessionByGUID(ownerGUID)
		if sess == nil {
			return nil
		}
		corpse, ok := sess.loadCorpseObject(ctx)
		if !ok || corpse.MapID != mapID || corpse.InstanceID != instanceID {
			return nil
		}
		return sess.luaCorpseObject(corpse)
	default:
		return nil
	}
}

// mapLuaGameObject is the map-scoped twin of serverLuaGameObject:
// dynamic/instance spawns on this map instance resolve from the object
// registry, static template GOs from the gameobject row; a spawn on
// another map, or one hidden in this instance, reads nil.
func (s *Server) mapLuaGameObject(ctx context.Context, mapID, instanceID uint32, guid uint64) *scripting.Object {
	if s == nil || uint16(guid>>48) != 0xF110 {
		return nil
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var state luaGameObjectState
	var ok bool
	s.objectsMu.RLock()
	if dyn, found := s.dynamicGameObjects[guid]; found && dyn != nil &&
		dyn.Map == mapID && dyn.InstanceID == instanceID {
		state = luaGameObjectState{GUID: dyn.GUID, LowGUID: dyn.LowGUID, Entry: dyn.Entry, DisplayID: dyn.DisplayID, Map: dyn.Map, InstanceID: dyn.InstanceID, X: dyn.X, Y: dyn.Y, Z: dyn.Z, Orientation: dyn.Orientation, GoState: uint32(dyn.State), LootState: 1, Type: uint32(dyn.Type), OwnerGUID: dyn.OwnerGUID, PhaseMask: dyn.PhaseMask}
		ok = true
	}
	if !ok {
		if states, found := s.instanceGameObjects[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; found {
			if dyn := states[guid]; dyn != nil {
				state = luaGameObjectState{GUID: dyn.GUID, LowGUID: dyn.LowGUID, Entry: dyn.Entry, DisplayID: dyn.DisplayID, Map: dyn.Map, InstanceID: dyn.InstanceID, X: dyn.X, Y: dyn.Y, Z: dyn.Z, Orientation: dyn.Orientation, GoState: uint32(dyn.State), LootState: 1, Type: uint32(dyn.Type), OwnerGUID: dyn.OwnerGUID, PhaseMask: dyn.PhaseMask}
				ok = true
			}
		}
	}
	s.objectsMu.RUnlock()
	if !ok {
		var loaded bool
		state, loaded = s.loadLuaGameObject(ctx, low, entry)
		if !loaded || state.Map != mapID {
			return nil
		}
		state.InstanceID = instanceID
		ok = true
	}
	if s.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) {
		return nil
	}
	return s.luaGameObjectObject(state)
}

// instanceMapObject builds the map argument for the Eluna
// INSTANCE_EVENT_* hooks, the Go model of Eluna::InstanceHooks'
// Push(instance): the Map object carrying the map and instance IDs plus
// the live Map surface (scripting.NewMapObject).
func instanceMapObject(srv *Server, mapID, instanceID uint32) *scripting.Object {
	if srv == nil {
		return scripting.NewMapObject(mapID, instanceID, scripting.MapLive{})
	}
	return scripting.NewMapObject(mapID, instanceID, srv.mapLive(mapID, instanceID))
}

// isInstanceScriptMap reports whether the map can carry Eluna instance
// hooks. C++ installs the ElunaInstanceAI shim only on instanceable maps
// (Eluna::GetInstanceData, consulted from InstanceMap::SetInstanceData), so
// the hooks never fire for continents or battleground maps.
func (s *Server) isInstanceScriptMap(mapID uint32) bool {
	if s == nil || s.Data == nil {
		return false
	}
	entry, found, err := s.Data.Map(mapID)
	return err == nil && found && entry.IsDungeon()
}

// fireInstancePlayerEnter dispatches Eluna INSTANCE_EVENT_ON_PLAYER_ENTER
// (4), fired from InstanceMap::AddPlayerToMap (Maps/Map.cpp:3979) once the
// player is on the instance map: (event, instance_data, map, player).
// Returns mirror the C++ call, which discards them (CallAllFunctions).
func (s *session) fireInstancePlayerEnter(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	if s.player.InstanceID == 0 || !s.server.isInstanceScriptMap(s.player.Map) {
		return
	}
	if _, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, s.player.Map, s.player.InstanceID, scripting.InstanceEventOnPlayerEnter, luaInstanceData(), instanceMapObject(s.server, s.player.Map, s.player.InstanceID), s.luaPlayer()); err != nil {
		s.debug("lua instance player-enter event failed", "map", s.player.Map, "instance", s.player.InstanceID, "error", err)
	}
}

// triggerInstanceGameObjectCreate dispatches Eluna
// INSTANCE_EVENT_ON_GAMEOBJECT_CREATE (6), fired from
// GameObject::AddToWorld (GameObject.cpp:213) via the instance's zone
// script: (event, instance_data, map, go). Only dynamic/instance spawns
// have a live server-side object — static template GOs are per-client
// update packets, so their AddToWorld has no Go counterpart (the same
// documented delta as the GAMEOBJECT_EVENT_ON_ADD_TO_WORLD bridge).
// Returns mirror the C++ call, which discards them (CallAllFunctions).
func (s *Server) triggerInstanceGameObjectCreate(dyn *dynamicGameObjectState) {
	if s == nil || dyn == nil || dyn.InstanceID == 0 || !s.isInstanceScriptMap(dyn.Map) || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	goObj := s.serverLuaGameObject(context.Background(), dyn.GUID)
	if goObj == nil {
		return
	}
	if _, err := s.Features.Scripts.TriggerInstanceEvent(context.Background(), dyn.Map, dyn.InstanceID, scripting.InstanceEventOnGameObjectCreate, luaInstanceData(), instanceMapObject(s, dyn.Map, dyn.InstanceID), goObj); err != nil {
		s.debug("lua instance gameobject-create event failed", "map", dyn.Map, "instance", dyn.InstanceID, "error", err)
	}
}

// triggerInstanceCreatureCreate dispatches Eluna INSTANCE_EVENT_ON_CREATURE_CREATE
// (5), fired from Creature::AddToWorld (Creature.cpp:301) via the instance's zone
// script: (event, instance_data, map, creature). Only dynamically-materialized
// motions pass through the Go motion funnel (motionFor) — static template spawns
// are per-client update packets with no server-side AddToWorld, the same
// documented delta as the INSTANCE_EVENT_ON_GAMEOBJECT_CREATE bridge.
// Direct-constructed motions (temp summons, boss minions, totems, pets) bypass
// the funnel and don't fire; documented delta. Returns mirror the C++ call,
// which discards them (CallAllFunctions).
func (s *Server) triggerInstanceCreatureCreate(motion *creatureMotion) {
	if s == nil || motion == nil || motion.InstanceID == 0 || !s.isInstanceScriptMap(motion.Map) || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	if _, err := s.Features.Scripts.TriggerInstanceEvent(context.Background(), motion.Map, motion.InstanceID, scripting.InstanceEventOnCreatureCreate, luaInstanceData(), instanceMapObject(s, motion.Map, motion.InstanceID), creature); err != nil {
		s.debug("lua instance creature-create event failed", "map", motion.Map, "instance", motion.InstanceID, "error", err)
	}
}

// instanceEncounterHooked reports whether any Eluna
// INSTANCE_EVENT_ON_CHECK_ENCOUNTER_IN_PROGRESS (7) handler returns true,
// mirroring ElunaInstanceAI::IsEncounterInProgress
// (LuaEngine/ElunaInstanceAI.h), which returns CallAllFunctionsBool(...):
// true when any handler returns boolean true, false by default
// (LuaEngine/HookHelpers.h). C++ consults it in InstanceMap::CanEnter
// (Maps/Map.cpp:3852) to reject entry while an encounter is in progress.
// Go ORs it with the native instanceEncounters tracking — the C++ shim
// replaces the native check outright, but Go's native boss-pull tracking
// is a separately bridged system, so the hook augments rather than
// replaces it. Documented delta.
func (s *session) instanceEncounterHooked(ctx context.Context, mapID, instanceID uint32) bool {
	if s == nil || s.server == nil || instanceID == 0 || !s.server.isInstanceScriptMap(mapID) || s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	values, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, mapID, instanceID, scripting.InstanceEventOnCheckEncounterProgress, luaInstanceData(), instanceMapObject(s.server, mapID, instanceID))
	if err != nil {
		s.debug("lua instance encounter check failed", "map", mapID, "instance", instanceID, "error", err)
	}
	for _, value := range values {
		if inProgress, ok := value.(bool); ok && inProgress {
			return true
		}
	}
	return false
}

// updateInstanceScriptTicks dispatches Eluna INSTANCE_EVENT_ON_UPDATE (3) per
// instance map carrying registered handlers, fired from
// ElunaInstanceAI::Update (LuaEngine/ElunaInstanceAI.h:118-130) on every
// Map::Update: (event, instance_data, map, diff). C++ installs the
// ElunaInstanceAI shim (and thus runs the update hook) only on maps that have
// bindings (Eluna::GetInstanceData, LuaEngine.cpp:1238-1255), so Go pre-checks
// Runtime.HasHook on both the map and instance binding keys — the Go model of
// START_HOOK's HasBindingsFor gate. Fires only for instance maps with players;
// a player-less Go instance has no materialized map update at all. diff is the
// world-tick delta in ms (C++ uint32 diff). Returns mirror the C++ call, which
// discards them (CallAllFunctions).
func (s *Server) updateInstanceScriptTicks(ctx context.Context, diff time.Duration) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	scripts := s.Features.Scripts
	type instanceKey struct {
		mapID      uint32
		instanceID uint32
	}
	seen := map[instanceKey]struct{}{}
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess == nil || sess.player == nil || sess.player.InstanceID == 0 {
			continue
		}
		seen[instanceKey{sess.player.Map, sess.player.InstanceID}] = struct{}{}
	}
	s.sessionsMu.RUnlock()
	for k := range seen {
		if !s.isInstanceScriptMap(k.mapID) {
			continue
		}
		mapKind := "map:" + strconv.FormatUint(uint64(k.mapID), 10)
		instKind := "instance:" + strconv.FormatUint(uint64(k.instanceID), 10)
		if !scripts.HasHook(mapKind, scripting.InstanceEventOnUpdate) && !scripts.HasHook(instKind, scripting.InstanceEventOnUpdate) {
			continue
		}
		if _, err := scripts.TriggerInstanceEvent(ctx, k.mapID, k.instanceID, scripting.InstanceEventOnUpdate, luaInstanceData(), instanceMapObject(s, k.mapID, k.instanceID), uint32(diff.Milliseconds())); err != nil {
			s.debug("lua instance update event failed", "map", k.mapID, "instance", k.instanceID, "error", err)
		}
	}
}
