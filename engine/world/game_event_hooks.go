package world

import (
	"context"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// Server game-event and map-update hooks: GameEventMgr::Update
// (GameEventMgr.cpp:1087-1160) re-evaluates every event on its timer and
// fires OnGameEventStart for newly active events before OnGameEventStop
// for newly inactive ones, and Map::Update (Map.cpp:907) fires
// OnMapUpdate per map per update. Go has no GameEventMgr update loop
// and no loaded-map list, so both legs ride the 100ms world tick:
// the game-event leg diffs the cached active set (conditions.go,
// refreshed at most once a minute — the Go cadence for NextCheck's
// timer), and the map leg enumerates session-occupied maps.
// DELTAS: only GAMEEVENT_NORMAL rows ever enter the cached set, so
// only schedule-driven normal transitions fire — the .event start/stop
// commands flip only the cached override (no ApplyNewEvent spawn
// application), so command-driven transitions fire the 34/35 hooks but
// change no spawns; the first tick after startup fires
// start for every already-active event, matching StartSystem's initial
// Update (GameEventMgr.cpp:1075-1081); map-update fires only for maps
// carrying at least one session, never for player-less maps.

// gameEventSetDiff splits a cached-set transition into ascending starts
// then ascending stops, the GameEventMgr::Update queue order.
func gameEventSetDiff(last, current map[int64]struct{}) (starts, stops []int64) {
	for id := range current {
		if _, ok := last[id]; !ok {
			starts = append(starts, id)
		}
	}
	for id := range last {
		if _, ok := current[id]; !ok {
			stops = append(stops, id)
		}
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	sort.Slice(stops, func(i, j int) bool { return stops[i] < stops[j] })
	return starts, stops
}

// fireGameEventHookTransitions fires GAME_EVENT_START (34) for each id
// in starts, then GAME_EVENT_STOP (35) for each id in stops, both with
// (event, gameeventid) per ServerHooks.cpp:90-101.
func (s *Server) fireGameEventHookTransitions(ctx context.Context, starts, stops []int64) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	for _, id := range starts {
		_, _ = s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventGameEventStart, uint32(id))
	}
	for _, id := range stops {
		_, _ = s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventGameEventStop, uint32(id))
	}
}

// updateGameEventHooks diffs the cached active game-event set against
// the last fired set and fires the transitions. The last set is stored
// before firing so a re-entrant tick cannot re-fire the same edge.
func (s *Server) updateGameEventHooks(ctx context.Context) {
	if s == nil || s.stopEvent.Load() {
		return
	}
	current := s.cachedActiveGameEvents(ctx)
	next := make(map[int64]struct{}, len(current))
	for id := range current {
		next[id] = struct{}{}
	}
	s.gameEventHookMu.Lock()
	starts, stops := gameEventSetDiff(s.gameEventActive, next)
	s.gameEventActive = next
	s.gameEventHookMu.Unlock()
	s.fireGameEventHookTransitions(ctx, starts, stops)
}

// updateMapServerEvents fires MAP_EVENT_ON_UPDATE (23) with
// (event, map, diff) per ServerHooks.cpp:354-360 for each distinct
// session-occupied map, in (mapID, instanceID) order. The HasHook gate
// mirrors updateInstanceScriptTicks' START_HOOK gate: with no server
// event-23 binding registered the scan costs nothing.
func (s *Server) updateMapServerEvents(ctx context.Context, diff time.Duration) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	if !s.Features.Scripts.HasHook("server", scripting.ServerEventMapUpdate) {
		return
	}
	type mapKey struct {
		mapID      uint32
		instanceID uint32
	}
	seen := map[mapKey]struct{}{}
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess == nil || sess.player == nil {
			continue
		}
		seen[mapKey{sess.player.Map, sess.player.InstanceID}] = struct{}{}
	}
	s.sessionsMu.RUnlock()
	keys := make([]mapKey, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].mapID != keys[j].mapID {
			return keys[i].mapID < keys[j].mapID
		}
		return keys[i].instanceID < keys[j].instanceID
	})
	for _, k := range keys {
		mapObject := instanceMapObject(s, k.mapID, k.instanceID)
		if _, err := s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventMapUpdate, mapObject, uint32(diff.Milliseconds())); err != nil {
			s.debug("lua map update event failed", "map", k.mapID, "instance", k.instanceID, "error", err)
		}
	}
}
