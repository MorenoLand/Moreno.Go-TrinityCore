package world

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
// commands and the Lua Start/StopGameEvent globals flip the cached
// override, and the ApplyNewEvent/UnApplyEvent legs run on the tick
// transition in applyGameEventTransition (announce + worldstate write;
// the spawn legs are query-time: the event clause in the creature and
// gameobject sweeps, the model/equip overrides, and the event vendor
// rows). The first tick after startup fires start for every
// already-active event, matching StartSystem's initial Update
// (GameEventMgr.cpp:1075-1081); map-update fires only for maps carrying
// at least one session, never for player-less maps.

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
// before firing so a re-entrant tick cannot re-fire the same edge. The
// world-event condition machine runs first (its transitions invalidate
// the cache, so the diff below sees the new states in the same tick).
func (s *Server) updateGameEventHooks(ctx context.Context) {
	if s == nil || s.stopEvent.Load() {
		return
	}
	s.updateWorldGameEvents(ctx)
	current := s.cachedActiveGameEvents(ctx)
	next := make(map[int64]struct{}, len(current))
	for id := range current {
		next[id] = struct{}{}
	}
	s.gameEventHookMu.Lock()
	starts, stops := gameEventSetDiff(s.gameEventActive, next)
	s.gameEventActive = next
	s.gameEventHookMu.Unlock()
	for _, id := range starts {
		s.applyGameEventTransition(ctx, id, true)
	}
	for _, id := range stops {
		s.applyGameEventTransition(ctx, id, false)
	}
	s.fireGameEventHookTransitions(ctx, starts, stops)
}

// langEventMessage is LANG_EVENTMESSAGE (Language.h:36), the world-text
// id ApplyNewEvent announces with.
const langEventMessage = 4

// applyGameEventTransition runs the ApplyNewEvent/UnApplyEvent legs that
// live outside the query-time spawn application, in C++ order (apply
// before the Eluna hook fire): on start, the worldstate write for
// NORMAL/INTERNAL events (StartEvent, GameEventMgr.cpp:134-152 — the
// world-event branch writes nothing) and the announce
// (ApplyNewEvent, GameEventMgr.cpp:1182-1188); on stop, the worldstate
// cleanup (StopEvent, GameEventMgr.cpp:185-197, unconditional). It runs
// on the tick diff, so command- and Lua-driven transitions land within
// one 100ms tick, like the 34/35 hooks. DELTAS: the condition machine
// (GAMEEVENT_WORLD_CONDITIONS/NEXTPHASE, CheckOneGameEventConditions,
// SaveWorldEventStateToDB) is ported in game_event_conditions.go, so
// world-event rows reach this path through the schedule; the holiday
// worldstate leg (UpdateWorldStates, GameEventMgr.cpp:1595-1614) needs
// the DBC BattlemasterList HolidayWorldState, and RunSmartAIScripts
// (SMART_EVENT_GAME_EVENT_START/END) needs a SmartAI engine — neither
// exists in Go. Remaining deltas: the per-player condition worldstates
// (SendWorldStateUpdate, GameEventMgr.cpp:1710-1720) have no Go model —
// the server-wide worldstate write above is the only condition progress
// signal — and the StopEvent world branch (condition reset +
// game_event_save delete, GameEventMgr.cpp:185-232) is not ported, so a
// stopped world event keeps its saved CONDITIONS progress until a GM
// restarts it.
func (s *Server) applyGameEventTransition(ctx context.Context, id int64, start bool) {
	if s == nil {
		return
	}
	if !start {
		s.setWorldState(ctx, uint32(id), 0)
		return
	}
	data := s.loadGameEventDataMap(ctx)
	ev, ok := data[id]
	if !ok {
		return
	}
	if ev.WorldEvent == gameEventStateNormal || ev.WorldEvent == gameEventStateInternal {
		s.setWorldState(ctx, uint32(id), uint64(time.Now().Unix()))
	}
	if ev.Announce == 1 || (ev.Announce == 2 && s.Config.EventAnnounce) {
		s.broadcastGameEventAnnounce(ctx, ev.Description)
	}
}

// broadcastGameEventAnnounce sends the ApplyNewEvent world text
// (SendWorldText(LANG_EVENTMESSAGE, description)): the LANG 4 format
// string comes from trinity_string at runtime (the terrain.go
// LANG_NOT_EQUIPPED_ITEM precedent), formatted with the event
// description; a missing row or a format without a verb skips the
// broadcast rather than sending a broken line.
func (s *Server) broadcastGameEventAnnounce(ctx context.Context, description string) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var format string
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT content_default FROM trinity_string WHERE entry = ?", langEventMessage).Scan(&format); err != nil || format == "" {
		return
	}
	if !strings.Contains(format, "%s") {
		return
	}
	s.broadcastMessageChatAll(fmt.Sprintf(format, description))
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
