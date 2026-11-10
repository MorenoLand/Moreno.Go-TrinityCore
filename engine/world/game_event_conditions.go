package world

import (
	"context"
	"sync"
	"time"
)

// World-event condition machine: GameEventMgr::CheckOneGameEventConditions,
// HandleQuestComplete and SaveWorldEventStateToDB
// (GameEventMgr.cpp:1617-1708) plus the world-event legs of
// GameEventMgr::Update (GameEventMgr.cpp:1087-1160) and the CheckOneGameEvent
// state arms (GameEventMgr.cpp:44-80). The `game_event.world_event` column
// is the initial GameEventState (GameEventMgr.h:35-40, loaded raw at
// LoadFromDB :264); `game_event_save` in the characters DB overrides the
// state and nextstart for non-NORMAL/INTERNAL rows at load (:300-344).

const (
	gameEventStateWorldInactive   = 1
	gameEventStateWorldConditions = 2
	gameEventStateWorldNextPhase  = 3
	gameEventStateWorldFinished   = 4
)

// gameEventCondition is one `game_event_condition` row with its saved
// progress merged in (LoadFromDB :669-740): req_num is the target,
// done the saved progress (0 when no `game_event_condition_save` row),
// and the world-state fields drive SendWorldStateUpdate.
type gameEventCondition struct {
	EventID        int64
	ConditionID    int64
	ReqNum         float64
	Done           float64
	MaxWorldState  int64
	DoneWorldState int64
}

// gameEventQuestCondition is one `game_event_quest_condition` row
// (LoadFromDB :630-666): completing quest maps to (event, condition, num).
type gameEventQuestCondition struct {
	QuestID     int64
	EventID     int64
	ConditionID int64
	Num         float64
}

// gameEventSave is one `game_event_save` row (characters DB): the saved
// state and nextstart for a world event.
type gameEventSave struct {
	State     int64
	NextStart int64
}

// loadGameEventConditions loads the `game_event_condition` rows with the
// saved `done` values from `game_event_condition_save` merged in. A save
// row for an unloaded condition is skipped, like C++ LoadFromDB (:734).
// A missing table or query error degrades to no conditions, matching C++
// behavior with empty tables.
func (s *Server) loadGameEventConditions(ctx context.Context) map[int64]map[int64]*gameEventCondition {
	conds := make(map[int64]map[int64]*gameEventCondition)
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return conds
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT eventEntry, condition_id, req_num, max_world_state_field, done_world_state_field FROM game_event_condition")
	if err != nil {
		return conds
	}
	defer rows.Close()
	for rows.Next() {
		var c gameEventCondition
		if err := rows.Scan(&c.EventID, &c.ConditionID, &c.ReqNum, &c.MaxWorldState, &c.DoneWorldState); err != nil {
			continue
		}
		if conds[c.EventID] == nil {
			conds[c.EventID] = make(map[int64]*gameEventCondition)
		}
		cc := c
		conds[c.EventID][c.ConditionID] = &cc
	}
	if err := rows.Err(); err != nil {
		return conds
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return conds
	}
	saved, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT eventEntry, condition_id, done FROM game_event_condition_save")
	if err != nil {
		return conds
	}
	defer saved.Close()
	for saved.Next() {
		var eventID, conditionID int64
		var done float64
		if err := saved.Scan(&eventID, &conditionID, &done); err != nil {
			continue
		}
		if byEvent, ok := conds[eventID]; ok {
			if c, ok := byEvent[conditionID]; ok {
				c.Done = done
			}
		}
	}
	return conds
}

// loadGameEventQuestConditions loads the `game_event_quest_condition` rows
// keyed by quest id; a later row for the same quest overwrites, matching
// the std::map assignment in LoadFromDB.
func (s *Server) loadGameEventQuestConditions(ctx context.Context) map[int64]gameEventQuestCondition {
	byQuest := make(map[int64]gameEventQuestCondition)
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return byQuest
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT quest, eventEntry, condition_id, num FROM game_event_quest_condition")
	if err != nil {
		return byQuest
	}
	defer rows.Close()
	for rows.Next() {
		var qc gameEventQuestCondition
		if err := rows.Scan(&qc.QuestID, &qc.EventID, &qc.ConditionID, &qc.Num); err != nil {
			continue
		}
		byQuest[qc.QuestID] = qc
	}
	return byQuest
}

// loadGameEventSaves loads the `game_event_save` rows (characters DB),
// keyed by event id.
func (s *Server) loadGameEventSaves(ctx context.Context) map[int64]gameEventSave {
	saves := make(map[int64]gameEventSave)
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return saves
	}
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT eventEntry, state, next_start FROM game_event_save")
	if err != nil {
		return saves
	}
	defer rows.Close()
	for rows.Next() {
		var eventID int64
		var save gameEventSave
		if err := rows.Scan(&eventID, &save.State, &save.NextStart); err != nil {
			continue
		}
		saves[eventID] = save
	}
	return saves
}

// loadGameEventPrerequisites loads the `game_event_prerequisite` rows
// (LoadFromDB :350-397), keyed by event id.
func (s *Server) loadGameEventPrerequisites(ctx context.Context) map[int64][]int64 {
	prereqs := make(map[int64][]int64)
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return prereqs
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT eventEntry, prerequisite_event FROM game_event_prerequisite")
	if err != nil {
		return prereqs
	}
	defer rows.Close()
	for rows.Next() {
		var eventID, prereq int64
		if err := rows.Scan(&eventID, &prereq); err != nil {
			continue
		}
		prereqs[eventID] = append(prereqs[eventID], prereq)
	}
	return prereqs
}

// checkOneGameEventConditions mirrors GameEventMgr::CheckOneGameEventConditions
// (GameEventMgr.cpp:1672-1687): every condition must have done >= reqNum.
// When all are met the event moves to WORLD_NEXTPHASE and, when nextstart
// is unset, it becomes now + length minutes (length * 60, seconds). It
// returns the met flag and the nextstart the caller stores.
func checkOneGameEventConditions(conds map[int64]*gameEventCondition, length, nextstart, now int64) (bool, int64) {
	for _, c := range conds {
		if c.Done < c.ReqNum {
			return false, nextstart
		}
	}
	if nextstart == 0 {
		nextstart = now + length*60
	}
	return true, nextstart
}

// saveWorldEventStateToDB mirrors GameEventMgr::SaveWorldEventStateToDB
// (GameEventMgr.cpp:1689-1708): delete + insert into `game_event_save`
// (event, state, nextstart or 0 when unset).
func (s *Server) saveWorldEventStateToDB(ctx context.Context, eventID, state, nextstart int64) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	if _, err := db.ExecContext(ctx, "DELETE FROM game_event_save WHERE eventEntry = ?", eventID); err != nil {
		return
	}
	var next uint64
	if nextstart != 0 {
		next = uint64(nextstart)
	}
	_, _ = db.ExecContext(ctx, "INSERT INTO game_event_save (eventEntry, state, next_start) VALUES (?, ?, ?)", eventID, uint8(state), next)
}

// saveGameEventConditionProgress persists one condition's done count to
// `game_event_condition_save` (CHAR_DEL/INS_GAME_EVENT_CONDITION_SAVE,
// GameEventMgr.cpp:1642-1655): delete + insert of (event, condition, done).
func (s *Server) saveGameEventConditionProgress(ctx context.Context, eventID, conditionID int64, done float64) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	if _, err := db.ExecContext(ctx, "DELETE FROM game_event_condition_save WHERE eventEntry = ? AND condition_id = ?", eventID, conditionID); err != nil {
		return
	}
	_, _ = db.ExecContext(ctx, "INSERT INTO game_event_condition_save (eventEntry, condition_id, done) VALUES (?, ?, ?)", eventID, conditionID, done)
}

// worldGameEventState resolves an event's live state: the `game_event_save`
// row wins for world events, else the `game_event.world_event` column.
func worldGameEventState(worldEvent, entry int64, saves map[int64]gameEventSave) (state, nextstart int64) {
	state, nextstart = worldEvent, 0
	if save, ok := saves[entry]; ok && worldEvent != gameEventStateNormal && worldEvent != gameEventStateInternal {
		state, nextstart = save.State, save.NextStart
	}
	return state, nextstart
}

// checkOneWorldGameEvent mirrors the world-event arms of
// GameEventMgr::CheckOneGameEvent (GameEventMgr.cpp:44-80): CONDITIONS and
// NEXTPHASE are always active, FINISHED and INTERNAL never are, and an
// INACTIVE event is active only when every prerequisite sits in NEXTPHASE
// or FINISHED with nextstart reached — and only when it has at least one
// prerequisite (a prerequisite-less inactive event starts by GM command
// only).
func checkOneWorldGameEvent(state, nextstart int64, prereqs []int64, states map[int64]int64, nextstarts map[int64]int64, now int64) bool {
	switch state {
	case gameEventStateWorldConditions, gameEventStateWorldNextPhase:
		return true
	case gameEventStateWorldFinished, gameEventStateInternal:
		return false
	case gameEventStateWorldInactive:
		if len(prereqs) == 0 {
			return false
		}
		for _, prereq := range prereqs {
			ps, ok := states[prereq]
			if !ok || (ps != gameEventStateWorldNextPhase && ps != gameEventStateWorldFinished) {
				return false
			}
			if nextstarts[prereq] > now {
				return false
			}
		}
		return true
	}
	return false
}

var (
	worldGameEventMu      sync.Mutex
	worldGameEventLastRun time.Time
	worldGameEventForced  bool
)

// forceWorldGameEventUpdate mirrors sWorld->ForceGameEventUpdate: the next
// world-event tick runs immediately instead of waiting out the minute.
func (s *Server) forceWorldGameEventUpdate() {
	worldGameEventMu.Lock()
	worldGameEventForced = true
	worldGameEventMu.Unlock()
}

// updateWorldGameEvents runs the world-event legs of GameEventMgr::Update
// (GameEventMgr.cpp:1087-1160) on the world tick, at most once a minute
// (the Go cadence for the event-cache refresh) or immediately after a
// forced update. For each world event: a NEXTPHASE event whose nextstart
// has passed finishes (state FINISHED, nextstart 0, saved, queued for
// deactivation through the cached-set diff); a CONDITIONS event whose
// conditions are all met advances to NEXTPHASE and saves; an INACTIVE
// event whose prerequisites are met enters CONDITIONS (the StartEvent
// world branch, GameEventMgr.cpp:155-177: state flip, condition check,
// save) and, when its conditions are already met, forces another update
// pass. The cached active set is invalidated afterwards so the tick diff
// picks the transitions up.
func (s *Server) updateWorldGameEvents(ctx context.Context) {
	if s == nil {
		return
	}
	worldGameEventMu.Lock()
	forced := worldGameEventForced
	if !forced && time.Since(worldGameEventLastRun) < time.Minute {
		worldGameEventMu.Unlock()
		return
	}
	worldGameEventForced = false
	worldGameEventLastRun = time.Now()
	worldGameEventMu.Unlock()

	data := s.loadGameEventDataMap(ctx)
	if len(data) == 0 {
		return
	}
	saves := s.loadGameEventSaves(ctx)
	conds := s.loadGameEventConditions(ctx)
	prereqs := s.loadGameEventPrerequisites(ctx)

	states := make(map[int64]int64, len(data))
	nextstarts := make(map[int64]int64, len(data))
	for id, ev := range data {
		state, nextstart := worldGameEventState(ev.WorldEvent, ev.Entry, saves)
		states[id] = state
		nextstarts[id] = nextstart
	}
	now := time.Now().Unix()
	changed := false
	for id, ev := range data {
		if ev.WorldEvent == gameEventStateNormal || ev.WorldEvent == gameEventStateInternal {
			continue
		}
		state := states[id]
		nextstart := nextstarts[id]
		switch state {
		case gameEventStateWorldNextPhase:
			if nextstart <= now {
				states[id] = gameEventStateWorldFinished
				nextstarts[id] = 0
				s.saveWorldEventStateToDB(ctx, id, gameEventStateWorldFinished, 0)
				changed = true
			}
		case gameEventStateWorldConditions:
			if met, next := checkOneGameEventConditions(conds[id], ev.Length, nextstart, now); met {
				states[id] = gameEventStateWorldNextPhase
				nextstarts[id] = next
				s.saveWorldEventStateToDB(ctx, id, gameEventStateWorldNextPhase, next)
				changed = true
			}
		case gameEventStateWorldInactive:
			if checkOneWorldGameEvent(state, nextstart, prereqs[id], states, nextstarts, now) {
				states[id] = gameEventStateWorldConditions
				s.saveWorldEventStateToDB(ctx, id, gameEventStateWorldConditions, nextstart)
				changed = true
				if met, next := checkOneGameEventConditions(conds[id], ev.Length, nextstart, now); met {
					states[id] = gameEventStateWorldNextPhase
					nextstarts[id] = next
					s.saveWorldEventStateToDB(ctx, id, gameEventStateWorldNextPhase, next)
					s.forceWorldGameEventUpdate()
				}
			}
		}
	}
	if changed {
		eventCache.mu.Lock()
		eventCache.refreshed = time.Time{}
		eventCache.mu.Unlock()
	}
}

// handleGameEventQuestComplete mirrors GameEventMgr::HandleQuestComplete
// (GameEventMgr.cpp:1617-1670), called from Player::SendQuestReward
// (Player.cpp:17016): a registered quest bumps its event condition's done
// count (capped at reqNum), persists it, and when all conditions are met
// advances the event to NEXTPHASE, saves the state, and forces an event
// update. The guards are verbatim: the quest must map to an event, the
// event must be active in WORLD_CONDITIONS state, the condition must be
// registered, and done must still be below reqNum.
func (s *Server) handleGameEventQuestComplete(ctx context.Context, questID uint32) {
	if s == nil {
		return
	}
	byQuest := s.loadGameEventQuestConditions(ctx)
	qc, ok := byQuest[int64(questID)]
	if !ok {
		return
	}
	data := s.loadGameEventDataMap(ctx)
	ev, ok := data[qc.EventID]
	if !ok {
		return
	}
	saves := s.loadGameEventSaves(ctx)
	state, nextstart := worldGameEventState(ev.WorldEvent, ev.Entry, saves)
	if state != gameEventStateWorldConditions {
		return
	}
	conds := s.loadGameEventConditions(ctx)
	byEvent, ok := conds[qc.EventID]
	if !ok {
		return
	}
	c, ok := byEvent[qc.ConditionID]
	if !ok {
		return
	}
	if c.Done >= c.ReqNum {
		return
	}
	c.Done += qc.Num
	if c.Done > c.ReqNum {
		c.Done = c.ReqNum
	}
	s.saveGameEventConditionProgress(ctx, qc.EventID, qc.ConditionID, c.Done)
	if met, next := checkOneGameEventConditions(byEvent, ev.Length, nextstart, time.Now().Unix()); met {
		s.saveWorldEventStateToDB(ctx, qc.EventID, gameEventStateWorldNextPhase, next)
		s.forceWorldGameEventUpdate()
	}
}
