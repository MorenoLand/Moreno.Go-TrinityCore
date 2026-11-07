package world

import (
	"context"
	"strconv"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// instanceDataObject is the instance_data argument for the Eluna
// INSTANCE_EVENT_* hooks and for Map:GetInstanceData. C++ pushes the
// ElunaInstanceAI's Lua data table (LuaEngine/LuaEngine.cpp
// PushInstanceData), one persistent table per instance; the Go table
// itself lives in the scripting runtime's registry (see
// scripting.instanceDataStoreKey), so the same script writes survive
// across hooks. This cache additionally keeps Lua object identity
// stable (C++ pushes the same table, so data == data holds) and lets
// Map:GetInstanceData return the hook argument's object. A nil server
// yields a keyed handle that still reads the registry table. The table
// persists to instance.data through Map:SaveInstanceData
// (scripting.MarshalInstanceData) and loads back on the first entry of
// a server run (fireInstanceLoad, INSTANCE_EVENT_ON_LOAD).
func (s *Server) instanceDataObject(mapID, instanceID uint32) *scripting.Object {
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	if s == nil {
		return &scripting.Object{Type: "InstanceData", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID}}
	}
	s.instanceDataMu.Lock()
	defer s.instanceDataMu.Unlock()
	if obj, ok := s.instanceDataObjects[key]; ok {
		return obj
	}
	if s.instanceDataObjects == nil {
		s.instanceDataObjects = make(map[instanceAdmissionKey]*scripting.Object)
	}
	obj := &scripting.Object{Type: "InstanceData", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID}}
	s.instanceDataObjects[key] = obj
	return obj
}

// instanceDataForMap is the MapLive.InstanceData gate behind
// Map:GetInstanceData (MapMethods.h): C++ returns the ElunaInstanceAI
// table only when the map carries the shim, which Eluna installs only
// for maps with INSTANCE_EVENT bindings (LuaEngine.cpp:1238-1255), and
// nil for non-instance maps (ToInstanceMap fails) and for C++-scripted
// instances. Go mirrors both gates: instanceID 0 reads nil, and a map
// with no map-level or instance-level binding for any instance event
// (1-7) reads nil.
func (s *Server) instanceDataForMap(mapID, instanceID uint32) *scripting.Object {
	if s == nil || instanceID == 0 || s.Features == nil || s.Features.Scripts == nil {
		return nil
	}
	scripts := s.Features.Scripts
	mapKind := "map:" + strconv.FormatUint(uint64(mapID), 10)
	instKind := "instance:" + strconv.FormatUint(uint64(instanceID), 10)
	for event := scripting.InstanceEventOnInitialize; event <= scripting.InstanceEventOnCheckEncounterProgress; event++ {
		if scripts.HasHook(mapKind, event) || scripts.HasHook(instKind, event) {
			return s.instanceDataObject(mapID, instanceID)
		}
	}
	return nil
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
	live.InstanceData = func() *scripting.Object {
		return s.instanceDataForMap(mapID, instanceID)
	}
	live.SaveInstanceData = func(ctx context.Context) error {
		return s.saveInstanceData(ctx, mapID, instanceID)
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
	live.SetZoneWeatherOverride = func(ctx context.Context, zone, state uint32, grade float32) {
		s.setZoneWeatherOverride(zone, state, grade)
	}
	live.SetZoneLightOverride = func(ctx context.Context, zone, area, override, transition uint32) {
		s.setZoneLightOverride(zone, area, override, transition)
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
	// Height is Map::GetHeight for the Eluna call shape (MapMethods.h
	// x, y, phasemask = 1; z = MAX_HEIGHT): the static-vmap and
	// dynamic-floor rays from 100000 down 50 hit nothing, so the
	// Eluna shape is exactly the .map grid height
	// (Server.mapGridHeight); a missing or unreadable tile reads
	// invalid, and the scripting method turns that into nil.
	live.Height = func(x, y float32) (float32, bool) {
		return s.mapGridHeight(mapID, x, y)
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

// saveInstanceData is InstanceScript::SaveToDB (Instances/
// InstanceScript.cpp:60-71) for the Lua data table half: marshal the
// registry table (Go JSON+base64 format, scripting.MarshalInstanceData)
// plus the live boss states (WriteSaveDataBossStates writes the raw
// states; normalization happens on load, never here) and write it via
// CHAR_UPD_INSTANCE_DATA, preserving the completedEncounters mask the
// encounter-credit path owns. A missing instance row is a silent no-op
// (C++'s UPDATE affects zero rows); non-instance maps never reach it
// (no shim exists outside dungeons and raids, matching Eluna's
// instance-only install).
func (s *Server) saveInstanceData(ctx context.Context, mapID, instanceID uint32) error {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || instanceID == 0 || !s.isInstanceScriptMap(mapID) {
		return nil
	}
	encoded, err := s.Features.Scripts.MarshalInstanceData(mapID, instanceID, s.instanceBossStatesSnapshot(mapID, instanceID))
	if err != nil {
		return err
	}
	var mask int64
	if err := s.CharactersStore.DB.QueryRowContext(ctx, "SELECT completedEncounters FROM instance WHERE id = ?", instanceID).Scan(&mask); err != nil {
		return nil
	}
	_, err = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_INSTANCE_DATA", uint32(mask), encoded, instanceID)
	return err
}

// fireInstanceLoad dispatches Eluna INSTANCE_EVENT_ON_LOAD (2), fired
// from InstanceMap::LoadInstanceData (Maps/Map.cpp:4040-4065) when the
// instance map loads: (event, instance_data, map). C++ loads the saved
// data blob first (ElunaInstanceAI::Load base64-decodes + mar_decodes
// it into a fresh table, or creates an empty table when data is empty)
// and fires OnLoad on every load, empty or not; the boss states ride
// in the blob's bossStates section and are normalized + applied before
// the event (ReadSaveDataBossStates). Go has no InstanceMap
// object lifecycle, so the load runs once per (map, instance) per
// server run, at the first player entry after the instance row is
// known to exist (persist/ensure run before both entry sites), and
// fires before ON_PLAYER_ENTER (4). A blob the Go format cannot
// decode (including any C++-produced mar_encode blob) logs and loads
// an empty table, mirroring C++'s decode-failure arm; the once-gate
// is already set, so a later entry does not retry it.
// Returns mirror the C++ call, which discards them (CallAllFunctions).
func (s *session) fireInstanceLoad(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	if s.player.InstanceID == 0 || !s.server.isInstanceScriptMap(s.player.Map) {
		return
	}
	s.server.instanceAdmissionMu.Lock()
	if s.server.instanceLoaded == nil {
		s.server.instanceLoaded = make(map[instanceAdmissionKey]struct{})
	}
	key := instanceAdmissionKey{MapID: s.player.Map, InstanceID: s.player.InstanceID}
	if _, loaded := s.server.instanceLoaded[key]; loaded {
		s.server.instanceAdmissionMu.Unlock()
		return
	}
	s.server.instanceLoaded[key] = struct{}{}
	s.server.instanceAdmissionMu.Unlock()
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var data string
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT data FROM instance WHERE id = ?", s.player.InstanceID).Scan(&data); err == nil && data != "" {
			if bossStates, err := s.server.Features.Scripts.UnmarshalInstanceData(s.player.Map, s.player.InstanceID, data); err != nil {
				s.debug("lua instance data load failed", "map", s.player.Map, "instance", s.player.InstanceID, "error", err)
			} else {
				// Boss states load before ON_LOAD fires, with the C++
				// normalization (IN_PROGRESS/FAIL/SPECIAL to NOT_STARTED,
				// TO_BE_DECIDED skipped) and one spawn-group pass —
				// InstanceScript::ReadSaveDataBossStates, called from
				// InstanceScript::Load ahead of ElunaInstanceAI::Load.
				s.server.loadInstanceBossStates(ctx, s.player.Map, s.player.InstanceID, bossStates)
			}
		}
	}
	if _, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, s.player.Map, s.player.InstanceID, scripting.InstanceEventOnLoad, s.server.instanceDataObject(s.player.Map, s.player.InstanceID), instanceMapObject(s.server, s.player.Map, s.player.InstanceID)); err != nil {
		s.debug("lua instance load event failed", "map", s.player.Map, "instance", s.player.InstanceID, "error", err)
	}
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
	if _, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, s.player.Map, s.player.InstanceID, scripting.InstanceEventOnPlayerEnter, s.server.instanceDataObject(s.player.Map, s.player.InstanceID), instanceMapObject(s.server, s.player.Map, s.player.InstanceID), s.luaPlayer()); err != nil {
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
	if _, err := s.Features.Scripts.TriggerInstanceEvent(context.Background(), dyn.Map, dyn.InstanceID, scripting.InstanceEventOnGameObjectCreate, s.instanceDataObject(dyn.Map, dyn.InstanceID), instanceMapObject(s, dyn.Map, dyn.InstanceID), goObj); err != nil {
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
	if _, err := s.Features.Scripts.TriggerInstanceEvent(context.Background(), motion.Map, motion.InstanceID, scripting.InstanceEventOnCreatureCreate, s.instanceDataObject(motion.Map, motion.InstanceID), instanceMapObject(s, motion.Map, motion.InstanceID), creature); err != nil {
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
	values, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, mapID, instanceID, scripting.InstanceEventOnCheckEncounterProgress, s.server.instanceDataObject(mapID, instanceID), instanceMapObject(s.server, mapID, instanceID))
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
		if _, err := scripts.TriggerInstanceEvent(ctx, k.mapID, k.instanceID, scripting.InstanceEventOnUpdate, s.instanceDataObject(k.mapID, k.instanceID), instanceMapObject(s, k.mapID, k.instanceID), uint32(diff.Milliseconds())); err != nil {
			s.debug("lua instance update event failed", "map", k.mapID, "instance", k.instanceID, "error", err)
		}
	}
}
