package world

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

// gameEvent mirrors the world `game_event` scheduling columns used by
// TrinityCore GameEventMgr::CheckOneGameEvent.
type gameEvent struct {
	Entry      int64
	Start      int64
	End        int64
	Occurrence int64 // minutes
	Length     int64 // minutes
	Holiday    int64
	WorldEvent int64
}

const gameEventStateNormal = 0

// activeGameEvents computes the currently active event set the way
// GameEventMgr does for GAMEEVENT_NORMAL rows: the event is active when now
// lies inside [start, end) and inside the current occurrence window.
func (s *Server) activeGameEventSets(ctx context.Context) (map[int64]struct{}, map[int64]struct{}) {
	active := make(map[int64]struct{})
	holidays := make(map[int64]struct{})
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return active, holidays
	}
	now := time.Now().Unix()
	query := "SELECT eventEntry, COALESCE(UNIX_TIMESTAMP(start_time), 0), COALESCE(UNIX_TIMESTAMP(end_time), 0), occurence, length, holiday, world_event FROM game_event"
	if s.WorldStore.Backend == database.BackendSQLite {
		query = "SELECT eventEntry, CAST(strftime('%s', start_time) AS INTEGER), CAST(strftime('%s', end_time) AS INTEGER), occurence, length, holiday, world_event FROM game_event"
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, query)
	if err != nil {
		return active, holidays
	}
	defer rows.Close()
	for rows.Next() {
		var event gameEvent
		if err := rows.Scan(&event.Entry, &event.Start, &event.End, &event.Occurrence, &event.Length, &event.Holiday, &event.WorldEvent); err != nil {
			continue
		}
		if event.WorldEvent != gameEventStateNormal {
			// Conditions-driven world events stay inactive without their
			// state machinery, matching an idle GameEventMgr.
			continue
		}
		if event.Length <= 0 {
			continue
		}
		if event.Start >= now || now >= event.End {
			continue
		}
		elapsed := (now - event.Start) / 60
		if event.Occurrence > 0 && elapsed%(event.Occurrence) >= event.Length {
			continue
		}
		active[event.Entry] = struct{}{}
		if event.Holiday > 0 {
			holidays[event.Holiday] = struct{}{}
		}
	}
	return active, holidays
}

func (s *Server) activeGameEvents(ctx context.Context) map[int64]struct{} {
	active, _ := s.activeGameEventSets(ctx)
	return active
}

type gameEventCache struct {
	mu        sync.RWMutex
	events    map[int64]struct{}
	holidays  map[int64]struct{}
	refreshed time.Time
}

var eventCache gameEventCache

// cachedActiveGameEvents refreshes the active event set at most once a
// minute; TrinityCore re-evaluates on its GameEvent update interval.
func (s *Server) cachedActiveGameEvents(ctx context.Context) map[int64]struct{} {
	eventCache.mu.RLock()
	fresh := time.Since(eventCache.refreshed) < time.Minute
	events := eventCache.events
	eventCache.mu.RUnlock()
	if fresh {
		return events
	}
	active, holidays := s.activeGameEventSets(ctx)
	eventCache.mu.Lock()
	eventCache.events = active
	eventCache.holidays = holidays
	eventCache.refreshed = time.Now()
	eventCache.mu.Unlock()
	return active
}

func (s *Server) cachedActiveGameHolidays(ctx context.Context) map[int64]struct{} {
	eventCache.mu.RLock()
	fresh := time.Since(eventCache.refreshed) < time.Minute
	holidays := eventCache.holidays
	eventCache.mu.RUnlock()
	if fresh {
		return holidays
	}
	active, refreshedHolidays := s.activeGameEventSets(ctx)
	eventCache.mu.Lock()
	eventCache.events = active
	eventCache.holidays = refreshedHolidays
	eventCache.refreshed = time.Now()
	eventCache.mu.Unlock()
	return refreshedHolidays
}

// conditionRow is one `conditions` row; rows sharing an ElseGroup are AND'ed
// while distinct ElseGroups OR together, exactly like ConditionMgr.
type conditionRow struct {
	ElseGroup       int64
	ConditionType   int64
	ConditionTarget int64
	Value1          int64
	Value2          int64
	Value3          int64
	Negative        bool
}

const conditionSourceGossipMenuOption = 15

// loadGossipOptionConditions fetches conditions attached to a gossip menu
// option (SourceType 15: SourceGroup = MenuID, SourceEntry = OptionID).
func (s *session) loadGossipOptionConditions(ctx context.Context, menuID, optionID uint32) ([]conditionRow, error) {
	rows, err := s.server.WorldStore.DB.QueryContext(ctx,
		"SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId IN (14, 15) AND SourceGroup = ? AND SourceEntry = ?",
		menuID, optionID)
	if err != nil {
		if missingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	result := make([]conditionRow, 0, 4)
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *session) loadQuestConditions(ctx context.Context, questID uint32) ([]conditionRow, error) {
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = 19 AND SourceEntry = ? ORDER BY ElseGroup", questID)
	if err != nil {
		if missingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	result := make([]conditionRow, 0, 4)
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *session) meetQuestConditions(ctx context.Context, questID uint32) (bool, error) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	conditions, err := s.loadQuestConditions(ctx, questID)
	if err != nil {
		return false, err
	}
	if len(conditions) == 0 {
		return true, nil
	}
	groups := make(map[int64][]conditionRow)
	for _, row := range conditions {
		groups[row.ElseGroup] = append(groups[row.ElseGroup], row)
	}
	for _, group := range groups {
		met := true
		for _, row := range group {
			ok, err := s.evalQuestCondition(ctx, row)
			if err != nil {
				return false, err
			}
			if row.Negative {
				ok = !ok
			}
			if !ok {
				met = false
				break
			}
		}
		if met {
			return true, nil
		}
	}
	return false, nil
}

func (s *session) evalQuestCondition(ctx context.Context, row conditionRow) (bool, error) {
	ok, err := s.evalCondition(ctx, row, 0)
	if err != nil {
		return false, err
	}
	if !ok && !isImplementedConditionType(row.ConditionType) {
		return true, nil
	}
	return ok, nil
}

func isImplementedConditionType(condType int64) bool {
	switch condType {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 14, 15, 16, 17, 18, 19, 20, 22, 23, 24, 25, 26, 27, 28, 31, 36, 37, 38, 42, 43, 47, 50:
		return true
	default:
		return false
	}
}

// meetGossipOptionConditions evaluates the ElseGroup clause set; empty sets
// pass (no conditions attached).
func (s *session) meetGossipOptionConditions(ctx context.Context, menuID, optionID uint32, creatureEntry uint32) (bool, error) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	conditions, err := s.loadGossipOptionConditions(ctx, menuID, optionID)
	if err != nil {
		return false, err
	}
	if len(conditions) == 0 {
		return true, nil
	}
	groups := make(map[int64][]conditionRow)
	for _, row := range conditions {
		groups[row.ElseGroup] = append(groups[row.ElseGroup], row)
	}
	for _, group := range groups {
		met := true
		for _, row := range group {
			ok, err := s.evalCondition(ctx, row, creatureEntry)
			if err != nil {
				return false, err
			}
			if row.Negative {
				ok = !ok
			}
			if !ok {
				met = false
				break
			}
		}
		if met {
			return true, nil
		}
	}
	return false, nil
}

func (s *session) meetVendorItemConditions(ctx context.Context, creatureEntry, itemEntry uint32) (bool, error) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = 23 AND SourceGroup = ? AND SourceEntry = ? ORDER BY ElseGroup", creatureEntry, itemEntry)
	if err != nil {
		if missingTable(err) {
			return true, nil
		}
		return false, err
	}
	defer rows.Close()
	conditions := make([]conditionRow, 0, 4)
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative); err != nil {
			return false, err
		}
		conditions = append(conditions, row)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(conditions) == 0 {
		return true, nil
	}
	groups := make(map[int64][]conditionRow)
	for _, row := range conditions {
		groups[row.ElseGroup] = append(groups[row.ElseGroup], row)
	}
	for _, group := range groups {
		met := true
		for _, row := range group {
			ok, err := s.evalCondition(ctx, row, creatureEntry)
			if err != nil {
				return false, err
			}
			if row.Negative {
				ok = !ok
			}
			if !ok {
				met = false
				break
			}
		}
		if met {
			return true, nil
		}
	}
	return false, nil
}

// evalCondition mirrors Condition::Meets for the types the gossip option
// data actually uses; unimplemented types report not-met so hidden options
// stay hidden, matching conservative TC behavior.
func (s *session) evalCondition(ctx context.Context, row conditionRow, creatureEntry uint32) (bool, error) {
	cdb := func() *sql.DB {
		if s.server.CharactersStore == nil {
			return nil
		}
		return s.server.CharactersStore.DB
	}
	count := func(query string, args ...any) int64 {
		db := cdb()
		if db == nil {
			return 0
		}
		var n int64
		_ = db.QueryRowContext(ctx, query, args...).Scan(&n)
		return n
	}
	switch row.ConditionType {
	case 0: // CONDITION_NONE
		return true, nil
	case 1: // CONDITION_AURA
		if s.auras == nil {
			return false, nil
		}
		_, ok := s.auras[uint32(row.Value1)]
		return ok, nil
	case 2: // CONDITION_ITEM
		total := count(`SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci
			JOIN item_instance ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, row.Value1)
		return total >= row.Value2, nil
	case 3: // CONDITION_ITEM_EQUIPPED
		n := count(`SELECT COUNT(1) FROM character_inventory ci
			JOIN item_instance ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ii.itemEntry = ? AND ci.bag = 255 AND ci.slot < 19`, s.playerGUID, row.Value1)
		return n > 0, nil
	case 4: // CONDITION_ZONEID
		return s.player != nil && uint32(row.Value1) == s.player.Zone, nil
	case 5: // CONDITION_REPUTATION_RANK (rank mask, HATED=0 .. EXALTED=7)
		var standing int64
		db := cdb()
		if db == nil {
			return false, nil
		}
		_ = db.QueryRowContext(ctx, "SELECT standing FROM character_reputation WHERE guid = ? AND faction = ?", s.playerGUID, row.Value1).Scan(&standing)
		rank := reputationRank(standing)
		return uint32(row.Value2)&(1<<rank) != 0, nil
	case 6: // CONDITION_TEAM (469 Alliance, 67 Horde)
		if s.player == nil {
			return false, nil
		}
		alliance := s.playerAlliance()
		if row.Value1 == 469 {
			return alliance, nil
		} else if row.Value1 == 67 {
			return !alliance, nil
		}
		return false, nil
	case 7: // CONDITION_SKILL
		var value int64
		db := cdb()
		if db == nil {
			return false, nil
		}
		_ = db.QueryRowContext(ctx, "SELECT value FROM character_skills WHERE guid = ? AND skill = ?", s.playerGUID, row.Value1).Scan(&value)
		return value >= row.Value2, nil
	case 8: // CONDITION_QUESTREWARDED
		n := count("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, row.Value1)
		return n > 0, nil
	case 9: // CONDITION_QUESTTAKEN
		status, _ := s.characterQuestStatus(ctx, uint32(row.Value1))
		return status == questStatusIncomplete || status == questStatusComplete, nil
	case 12: // CONDITION_ACTIVE_EVENT
		active := s.server.cachedActiveGameEvents(ctx)
		_, ok := active[row.Value1]
		return ok, nil
	case 14: // CONDITION_QUEST_NONE
		status, _ := s.characterQuestStatus(ctx, uint32(row.Value1))
		rewarded := count("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, row.Value1)
		return status == 0 && rewarded == 0, nil
	case 15: // CONDITION_CLASS (bitmask: 1<<(class-1))
		if s.player == nil || s.player.Class == 0 {
			return false, nil
		}
		return (1<<(s.player.Class-1))&uint32(row.Value1) != 0, nil
	case 16: // CONDITION_RACE (bitmask: 1<<(race-1))
		if s.player == nil || s.player.Race == 0 {
			return false, nil
		}
		return (1<<(s.player.Race-1))&uint32(row.Value1) != 0, nil
	case 19: // CONDITION_SPAWNMASK
		return s.player != nil && uint32(row.Value1)&1 != 0, nil
	case 20: // CONDITION_GENDER
		return s.player != nil && uint32(row.Value1) == uint32(s.player.Gender), nil
	case 22: // CONDITION_MAPID
		return s.player != nil && uint32(row.Value1) == s.player.Map, nil
	case 23: // CONDITION_AREAID
		return s.player != nil && uint32(row.Value1) == s.player.Zone, nil
	case 24: // CONDITION_CREATURE_TYPE
		if s.server.WorldStore == nil || creatureEntry == 0 {
			return false, nil
		}
		var ctype int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", creatureEntry).Scan(&ctype)
		return ctype == row.Value1, nil
	case 25: // CONDITION_SPELL
		n := count("SELECT COUNT(1) FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, row.Value1)
		return n > 0, nil
	case 26: // CONDITION_PHASEMASK
		return s.player != nil && uint32(row.Value1)&1 != 0, nil
	case 27: // CONDITION_LEVEL
		if s.player == nil {
			return false, nil
		}
		return compareValues(int(row.Value2), int64(s.player.Level), row.Value1), nil
	case 28: // CONDITION_QUEST_COMPLETE (complete but not rewarded)
		status, _ := s.characterQuestStatus(ctx, uint32(row.Value1))
		rewarded := count("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, row.Value1)
		return status == questStatusComplete && rewarded == 0, nil
	case 31: // CONDITION_OBJECT_ENTRY_GUID (TypeID 3 unit, entry match)
		if row.Value1 != 3 {
			return false, nil
		}
		return row.Value2 == 0 || uint32(row.Value2) == creatureEntry, nil
	case 17: // CONDITION_ACHIEVEMENT
		return true, nil
	case 18: // CONDITION_TITLE
		return true, nil
	case 36: // CONDITION_ALIVE
		return s.player != nil && s.player.Health > 0, nil
	case 37: // CONDITION_HP_VAL
		if s.player == nil {
			return false, nil
		}
		return compareValues(int(row.Value2), int64(s.player.Health), row.Value1), nil
	case 38: // CONDITION_HP_PCT
		if s.player == nil || s.player.MaxHealth == 0 {
			return false, nil
		}
		return compareValues(int(row.Value2), int64(s.player.Health*100/s.player.MaxHealth), row.Value1), nil
	case 42: // CONDITION_STAND_STATE
		if s.player == nil {
			return false, nil
		}
		return (row.Value1 == 0 && int64(s.player.StandState) == row.Value2) || (row.Value1 == 1 && row.Value2 == 0 && s.player.StandState == 0), nil
	case 43: // CONDITION_DAILY_QUEST_DONE
		return false, nil
	case 47: // CONDITION_QUESTSTATE (1 none, 2 complete, 8 in progress, 32 failed, 64 rewarded)
		status, _ := s.characterQuestStatus(ctx, uint32(row.Value1))
		var bit uint32
		switch status {
		case questStatusComplete:
			bit = 2
		case questStatusIncomplete:
			bit = 8
		}
		if bit == 0 {
			bit = 1
		}
		rewarded := count("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, row.Value1)
		if rewarded > 0 {
			return uint32(row.Value2)&64 != 0, nil
		}
		return uint32(row.Value2)&bit != 0, nil
	case 50: // CONDITION_GAMEMASTER
		return true, nil
	default:
		return false, nil
	}
}

// compareValues ports TrinityCore CompareValues (COMP_TYPE_*).
func compareValues(compType int, a, b int64) bool {
	switch compType {
	case 0:
		return a == b
	case 1:
		return a > b
	case 2:
		return a < b
	case 3:
		return a >= b
	case 4:
		return a <= b
	default:
		return false
	}
}

// reputationRank converts a standing value to the ReputationRank bit index
// used by rank masks (HATED=0, HOSTILE=1, UNFRIENDLY=2, NEUTRAL=3,
// FRIENDLY=4, HONORED=5, REVERED=6, EXALTED=7).
func reputationRank(standing int64) uint32 {
	switch {
	case standing >= 42999:
		return 7
	case standing >= 21000:
		return 6
	case standing >= 9000:
		return 5
	case standing >= 3000:
		return 4
	case standing >= 0:
		return 3
	case standing >= -3000:
		return 2
	case standing >= -6000:
		return 1
	default:
		return 0
	}
}

// activeEventList returns the cached active event ids as a slice.
func (s *Server) activeEventList(ctx context.Context) []int64 {
	active := s.cachedActiveGameEvents(ctx)
	list := make([]int64, 0, len(active))
	for entry := range active {
		list = append(list, entry)
	}
	return list
}

// gameEventSpawnClause builds the SQL fragment and args filtering event
// spawns: positive entries spawn only while active, negative entries are
// removed while active (TC mGameEventCreatureGuids semantics).
func gameEventSpawnClause(column string, active []int64, args *[]any) string {
	if len(active) == 0 {
		return "(" + column + " IS NULL OR " + column + " = 0)"
	}
	positive := "(" + column + " IS NULL OR " + column + " = 0"
	placeholders := ""
	for _, entry := range active {
		if entry <= 0 {
			continue
		}
		if placeholders != "" {
			placeholders += ", "
		}
		placeholders += "?"
		*args = append(*args, entry)
	}
	if placeholders != "" {
		positive += " OR " + column + " IN (" + placeholders + ")"
	}
	positive += ")"
	negative := column + " IS NULL"
	negPlaceholders := ""
	for _, entry := range active {
		if entry >= 0 {
			continue
		}
		if negPlaceholders != "" {
			negPlaceholders += ", "
		}
		negPlaceholders += "?"
		*args = append(*args, -entry)
	}
	if negPlaceholders != "" {
		negative = "(" + column + " IS NULL OR " + column + " NOT IN (" + negPlaceholders + "))"
	}
	return positive + " AND " + negative
}

// gameEventNPCFlagJoin returns a COALESCE-able sum of event npc flags that
// apply to a spawn while their events run (game_event_npcflag).
func (s *Server) gameEventNPCFlagClause(ctx context.Context, args *[]any) (string, bool) {
	active := s.activeEventList(ctx)
	if len(active) == 0 {
		return "0", false
	}
	placeholders := ""
	for i, entry := range active {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		*args = append(*args, entry)
	}
	return "(SELECT COALESCE(SUM(nf.npcflag), 0) FROM game_event_npcflag nf WHERE nf.guid = c.guid AND nf.eventEntry IN (" + placeholders + "))", true
}
