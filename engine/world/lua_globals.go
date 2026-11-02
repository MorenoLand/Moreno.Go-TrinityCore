package world

import (
	"context"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaGameEventIDs is the GameEventProvider behind the Eluna
// GetActiveGameEvents / IsGameEventActive globals: the cached active event
// set (conditions.go, refreshed at most once a minute), ascending like the
// C++ GameEventMgr::ActiveEvents std::set iteration order.
func (s *Server) luaGameEventIDs() []uint32 {
	if s == nil {
		return nil
	}
	active := s.cachedActiveGameEvents(context.Background())
	ids := make([]uint32, 0, len(active))
	for id := range active {
		ids = append(ids, uint32(id))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// luaMapByID is the MapProvider behind the Eluna GetMapById global: the
// Map object for a known map row, nil when the row is missing (the C++
// MapMgr::FindMap nullptr arm) or without a data store. The instance id is
// carried on the object; Go has no per-instance map registry to check it
// against, so an unknown instance still yields the map object.
func (s *Server) luaMapByID(mapID, instanceID uint32) *scripting.Object {
	if s == nil || s.Data == nil {
		return nil
	}
	if _, found, err := s.Data.Map(mapID); err != nil || !found {
		return nil
	}
	return scripting.NewMapObject(mapID, instanceID, s.mapLive(mapID, instanceID))
}

// luaWorldMessage is the WorldMessageProvider behind the Eluna
// SendWorldMessage global: World::SendServerMessage(SERVER_MSG_STRING,
// message) (World.cpp:3074), bridged by broadcastServerMessageAll with the
// raw message, not the .announce LANG_SYSTEMMESSAGE wrap.
func (s *Server) luaWorldMessage(message string) {
	if s == nil {
		return
	}
	s.broadcastServerMessageAll(serverMessageString, message)
}

// luaAreaName is the AreaNameProvider behind the Eluna GetAreaName
// global: the AreaTable.dbc name for the id, unknown ids read not-found
// (the C++ sAreaTableStore null arm).
func (s *Server) luaAreaName(id uint32) (string, bool) {
	if s == nil || s.Data == nil {
		return "", false
	}
	area, found, err := s.Data.Area(id)
	if err != nil || !found {
		return "", false
	}
	return area.Name, true
}
