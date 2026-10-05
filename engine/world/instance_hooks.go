package world

import (
	"context"

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

// instanceMapObject builds the map argument for the Eluna
// INSTANCE_EVENT_* hooks, the Go model of Eluna::InstanceHooks'
// Push(instance): the Map object carrying the map and instance IDs.
func instanceMapObject(mapID, instanceID uint32) *scripting.Object {
	return &scripting.Object{Type: "Map", Fields: map[string]any{"MapId": mapID, "InstanceId": instanceID, "InWorld": true}}
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
	if _, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, s.player.Map, s.player.InstanceID, scripting.InstanceEventOnPlayerEnter, luaInstanceData(), instanceMapObject(s.player.Map, s.player.InstanceID), s.luaPlayer()); err != nil {
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
	if _, err := s.Features.Scripts.TriggerInstanceEvent(context.Background(), dyn.Map, dyn.InstanceID, scripting.InstanceEventOnGameObjectCreate, luaInstanceData(), instanceMapObject(dyn.Map, dyn.InstanceID), goObj); err != nil {
		s.debug("lua instance gameobject-create event failed", "map", dyn.Map, "instance", dyn.InstanceID, "error", err)
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
	values, err := s.server.Features.Scripts.TriggerInstanceEvent(ctx, mapID, instanceID, scripting.InstanceEventOnCheckEncounterProgress, luaInstanceData(), instanceMapObject(mapID, instanceID))
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
