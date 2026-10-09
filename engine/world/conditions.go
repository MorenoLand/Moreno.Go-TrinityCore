package world

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
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

// gameEventFull extends gameEvent with the description column so the event
// command arms can render the GameEventDataMap the way cs_event.cpp does
// (GameEventData.description plus the schedule columns). isValid mirrors
// GameEventData::isValid (GameEventMgr.h:77): length > 0 || state > GAMEEVENT_NORMAL.
type gameEventFull struct {
	gameEvent
	Description string
}

// isValid mirrors GameEventData::isValid.
func (e gameEventFull) isValid() bool {
	return e.Length > 0 || e.WorldEvent > gameEventStateNormal
}

// loadGameEventDataMap loads the full `game_event` rows (eventEntry to
// GameEventData) the way GameEventMgr::LoadFromDB does; the world_event column
// is the GameEventState stored in GameEventData.state.
func (s *Server) loadGameEventDataMap(ctx context.Context) map[int64]gameEventFull {
	data := make(map[int64]gameEventFull)
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return data
	}
	query := "SELECT eventEntry, COALESCE(UNIX_TIMESTAMP(start_time), 0), COALESCE(UNIX_TIMESTAMP(end_time), 0), occurence, length, holiday, world_event, description FROM game_event"
	if s.WorldStore.Backend == database.BackendSQLite {
		query = "SELECT eventEntry, CAST(strftime('%s', start_time) AS INTEGER), CAST(strftime('%s', end_time) AS INTEGER), occurence, length, holiday, world_event, description FROM game_event"
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, query)
	if err != nil {
		return data
	}
	defer rows.Close()
	for rows.Next() {
		var event gameEventFull
		if err := rows.Scan(&event.Entry, &event.Start, &event.End, &event.Occurrence, &event.Length, &event.Holiday, &event.WorldEvent, &event.Description); err != nil {
			continue
		}
		data[event.Entry] = event
	}
	return data
}

// conditionRow is one `conditions` row; rows sharing an ElseGroup are AND'ed
// while distinct ElseGroups OR together, exactly like ConditionMgr.
// ErrorType/ErrorTextId feed the spell-cast conditions failure mapping
// (Spell.cpp:5340-5348); loaders that don't need them leave them zero.
type conditionRow struct {
	ElseGroup       int64
	ConditionType   int64
	ConditionTarget int64
	Value1          int64
	Value2          int64
	Value3          int64
	Negative        bool
	ErrorType       int64
	ErrorTextId     int64
}

const conditionSourceGossipMenuOption = 15

const conditionSourceSpellCast = 17 // CONDITION_SOURCE_TYPE_SPELL (ConditionMgr.h:140)

const conditionSourceSpellImplicitTarget = 13 // CONDITION_SOURCE_TYPE_SPELL_IMPLICIT_TARGET (ConditionMgr.h:136)

const conditionSourceSpellClickEvent = 18 // CONDITION_SOURCE_TYPE_SPELL_CLICK_EVENT (ConditionMgr.h:141)

// Object type bits from the C++ TypeMask enum (ObjectGuid.h:48-56).
// isType (Object.h:91) tests (mask & m_objectType) != 0 with the mask
// truncated to uint16. m_objectType accumulates OBJECT (Object.cpp:71) |
// UNIT (Unit.cpp:311) plus PLAYER (Player.cpp:186), so a player tests as
// 0x0019 and a creature as 0x0009 (matching creatureTypeMask).
const (
	typeMaskUnit   uint16 = 0x0008
	typeMaskPlayer uint16 = 0x0010

	playerObjectTypeMask   uint16 = 0x0001 | typeMaskUnit | typeMaskPlayer
	creatureObjectTypeMask uint16 = 0x0001 | typeMaskUnit

	// MAX_PET_TYPE from the PetType enum (PetDefines.h:29-34): SUMMON_PET 0,
	// HUNTER_PET 1, MAX_PET_TYPE 4. CONDITION_PET_TYPE rows whose Value1 mask
	// reaches bit 4 are skipped at load (ConditionMgr.cpp:2355-2362).
	maxPetType uint8 = 4

	// UnitState bits from the UnitState enum (Unit.h:212-244). Only the
	// bits carried by UNIT_STATE_ALL_STATE_SUPPORTED pass the
	// CONDITION_UNIT_STATE IsValid arm (ConditionMgr.cpp:2297-2304); rows
	// with no supported bit are skipped at load, mirrored as fail-closed.
	unitStateDied              uint32 = 0x00000001
	unitStateMeleeAttacking    uint32 = 0x00000002
	unitStateCharmed           uint32 = 0x00000004
	unitStateStunned           uint32 = 0x00000008
	unitStateFleeing           uint32 = 0x00000080
	unitStateInFlight          uint32 = 0x00000100
	unitStateRoot              uint32 = 0x00000400
	unitStateConfused          uint32 = 0x00000800
	unitStateCasting           uint32 = 0x00008000
	unitStateMove              uint32 = 0x00100000
	unitStateAllStateSupported uint32 = 0x0037FFFF
)

// loadImplicitTargetConditions fetches the `conditions` rows attached to a
// spell's implicit targets (SourceEntry = spell id, SourceGroup = effect
// mask). TARGET_CHECK_ENTRY targets (7/8/38/40/46/60) resolve their entry
// filter from these rows; a missing table or query error degrades to no
// rows, matching C++ behavior with no ImplicitTargetConditions.
func (s *session) loadImplicitTargetConditions(ctx context.Context, spellID uint32, effectMask uint32) []conditionRow {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || effectMask == 0 {
		return nil
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = ? AND SourceEntry = ? AND (SourceGroup & ?) <> 0 ORDER BY ElseGroup", conditionSourceSpellImplicitTarget, spellID, effectMask)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]conditionRow, 0, 2)
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative); err != nil {
			return nil
		}
		result = append(result, row)
	}
	return result
}

// loadSpellCastConditions fetches the `conditions` rows attached to a
// spell's cast itself (SourceEntry = spell id; no SourceGroup filter —
// this is the NotGrouped variant, ConditionMgr.cpp:978-991). A missing
// table or query error degrades to no rows, matching C++ behavior with
// no conditions (IsObjectMeetingNotGroupedConditions returns true when
// no entry exists for the source type).
func (s *session) loadSpellCastConditions(ctx context.Context, spellID uint32) []conditionRow {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition, ErrorType, ErrorTextId FROM conditions WHERE SourceTypeOrReferenceId = ? AND SourceEntry = ? ORDER BY ElseGroup", conditionSourceSpellCast, spellID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]conditionRow, 0, 2)
	for rows.Next() {
		var row conditionRow
		if err := rows.Scan(&row.ElseGroup, &row.ConditionType, &row.ConditionTarget, &row.Value1, &row.Value2, &row.Value3, &row.Negative, &row.ErrorType, &row.ErrorTextId); err != nil {
			return nil
		}
		result = append(result, row)
	}
	return result
}

// checkSpellCastConditions mirrors the CheckCast conditions block
// (Spell.cpp:5337-5348): sConditionMgr->IsObjectMeetingNotGroupedConditions
// (CONDITION_SOURCE_TYPE_SPELL, spell id, ConditionSourceInfo(caster,
// object target)). Unlike evalConditionGroups this does NOT break out of
// a failing group: C++ IsObjectMeetToConditionList evaluates every row
// (ConditionMgr.cpp:883-926) and Condition::Meets overwrites
// mLastFailedCondition on each failure (ConditionMgr.cpp:593), so the
// last-failed condition is the last failing row in scan order. Condition
// target 0 is the caster (the session player); target 1 is the wire
// object target, passed as the creature context of evalCondition —
// player/gameobject targets have no Go resolution there, so target-1
// conditions against them degrade to unmet (documented delta). Reference
// rows (ConditionTypeOrReference < 0) have no Go model (no
// ConditionReferenceStore); they are skipped. Returns the
// SPELL_FAILED_* result and, for SPELL_FAILED_CUSTOM_ERROR, the
// ErrorTextId carried as the extended packet param.
func (s *session) checkSpellCastConditions(ctx context.Context, spellID uint32, target protocol.SpellTargetData) (uint8, uint32) {
	rows := s.loadSpellCastConditions(ctx, spellID)
	if len(rows) == 0 {
		return 0, 0
	}
	var creatureEntry uint32
	var creatureGUID uint64
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		creatureGUID = target.UnitGUID
		if s.server != nil {
			s.server.motionMu.Lock()
			motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.UnitGUID)
			s.server.motionMu.Unlock()
			if motion != nil {
				creatureEntry = motion.Entry
			}
		}
	}
	groupMet := make(map[int64]bool)
	var lastFailed *conditionRow
	for i := range rows {
		row := &rows[i]
		met, known := groupMet[row.ElseGroup]
		if known && !met {
			continue
		}
		if !known {
			groupMet[row.ElseGroup] = true
		}
		if row.ConditionType < 0 {
			continue
		}
		ok, err := s.evalCondition(ctx, *row, creatureEntry, creatureGUID)
		if err != nil {
			ok = false
		}
		if row.Negative {
			ok = !ok
		}
		if !ok {
			groupMet[row.ElseGroup] = false
			lastFailed = row
		}
	}
	for _, met := range groupMet {
		if met {
			return 0, 0
		}
	}
	if lastFailed != nil && lastFailed.ErrorType != 0 {
		if lastFailed.ErrorType == int64(spellFailedCustomError) {
			return spellFailedCustomError, uint32(lastFailed.ErrorTextId)
		}
		return uint8(lastFailed.ErrorType), 0
	}
	if lastFailed == nil || lastFailed.ConditionTarget == 0 {
		return spellFailedCasterAurastate, 0
	}
	return spellFailedBadTargets, 0
}

// loadGossipMenuConditions fetches conditions attached to a gossip menu title
// row (SourceType 14: SourceGroup = MenuID, SourceEntry = TextID; see
// ConditionMgr::addToGossipMenus, ConditionMgr.cpp:1368).
func (s *session) loadGossipMenuConditions(ctx context.Context, menuID, textID uint32) ([]conditionRow, error) {
	rows, err := s.server.WorldStore.DB.QueryContext(ctx,
		"SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = 14 AND SourceGroup = ? AND SourceEntry = ?",
		menuID, textID)
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

// evalConditionGroups evaluates the ElseGroup clause set; empty sets pass (no
// conditions attached).
func (s *session) evalConditionGroups(ctx context.Context, conditions []conditionRow, eval func(context.Context, conditionRow) (bool, error)) (bool, error) {
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
			ok, err := eval(ctx, row)
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

// meetSpellClickConditions mirrors ConditionMgr::IsObjectMeetingSpellClickConditions
// (ConditionMgr.cpp:1008): `conditions` rows for source type 18 keyed on the
// spell-click entry (SourceGroup) and spell id (SourceEntry) must evaluate
// with the player as condition target 0 and the clicked unit as target 1.
// A missing row set passes; a missing table degrades to pass, other query
// errors fail closed like the gossip/vendor condition arms.
func (s *session) meetSpellClickConditions(ctx context.Context, clickEntry, spellID uint32, creatureGUID uint64) (bool, error) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx,
		"SELECT ElseGroup, ConditionTypeOrReference, ConditionTarget, ConditionValue1, ConditionValue2, ConditionValue3, NegativeCondition FROM conditions WHERE SourceTypeOrReferenceId = ? AND SourceGroup = ? AND SourceEntry = ? ORDER BY ElseGroup",
		conditionSourceSpellClickEvent, clickEntry, spellID)
	if err != nil {
		if missingTable(err) {
			return true, nil
		}
		return false, err
	}
	defer rows.Close()
	var conditions []conditionRow
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
	return s.evalConditionGroups(ctx, conditions, func(ctx context.Context, row conditionRow) (bool, error) {
		return s.evalCondition(ctx, row, clickEntry, creatureGUID)
	})
}

// meetGossipMenuConditions mirrors the Conditions arm of Player::GetGossipTextId
// (Player.cpp:14722): a gossip_menu title row applies only when its attached
// conditions pass.
func (s *session) meetGossipMenuConditions(ctx context.Context, menuID, textID, creatureEntry uint32, creatureGUID uint64) (bool, error) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	conditions, err := s.loadGossipMenuConditions(ctx, menuID, textID)
	if err != nil {
		return false, err
	}
	return s.evalConditionGroups(ctx, conditions, func(ctx context.Context, row conditionRow) (bool, error) {
		return s.evalCondition(ctx, row, creatureEntry, creatureGUID)
	})
}

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
	ok, err := s.evalCondition(ctx, row, 0, 0)
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
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 42, 43, 44, 45, 46, 47, 48, 49, 50:
		return true
	default:
		return false
	}
}

// drunkenStateByValue ports Player::GetDrunkenstateByValue (Player.cpp:988-997)
// and the DrunkenState enum (Player.h:323-326): 0 sober, 1 tipsy, 2 drunk, 3 smashed.
func drunkenStateByValue(value uint16) uint32 {
	if value >= 90 {
		return 3
	}
	if value >= 50 {
		return 2
	}
	if value > 0 {
		return 1
	}
	return 0
}

// meetGossipOptionConditions evaluates the ElseGroup clause set; empty sets
// pass (no conditions attached).
func (s *session) meetGossipOptionConditions(ctx context.Context, menuID, optionID uint32, creatureEntry uint32, creatureGUID uint64) (bool, error) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true, nil
	}
	conditions, err := s.loadGossipOptionConditions(ctx, menuID, optionID)
	if err != nil {
		return false, err
	}
	return s.evalConditionGroups(ctx, conditions, func(ctx context.Context, row conditionRow) (bool, error) {
		return s.evalCondition(ctx, row, creatureEntry, creatureGUID)
	})
}

func (s *session) meetVendorItemConditions(ctx context.Context, creatureEntry, itemEntry uint32, vendorGUID uint64) (bool, error) {
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
			ok, err := s.evalCondition(ctx, row, creatureEntry, vendorGUID)
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
func (s *session) evalCondition(ctx context.Context, row conditionRow, creatureEntry uint32, creatureGUID uint64) (bool, error) {
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
	case 10: // CONDITION_DRUNKENSTATE
		if s.player == nil {
			return false, nil
		}
		return drunkenStateByValue(s.player.DrunkenState) >= uint32(row.Value1), nil
	case 11: // CONDITION_WORLD_STATE (ConditionMgr.cpp:456-460)
		return uint64(row.Value2) == s.server.getWorldState(uint32(row.Value1)), nil
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
	case 21: // CONDITION_UNIT_STATE (ConditionMgr.cpp:477-482:
		// condMeets = object->ToUnit() && unit->HasUnitState(ConditionValue1),
		// the UnitState enum at Unit.h:212-244. C++ IsValid skips the row at
		// load when no bit of Value1 is in UNIT_STATE_ALL_STATE_SUPPORTED
		// (ConditionMgr.cpp:2297-2304); mirror that as fail-closed. The
		// object resolves via conditionTargetUnit (the ToUnit arm);
		// an unresolvable target fails closed.
		if uint32(row.Value1)&unitStateAllStateSupported == 0 {
			return false, nil
		}
		unit, ok := s.conditionTargetUnit(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		var state uint32
		if unit.isPlayer {
			// UNIT_STATE_MELEE_ATTACKING is set when the unit attacks
			// (Unit.cpp:5693/5723); Go's live source is the session attack
			// target.
			if s.attackTarget != 0 {
				state |= unitStateMeleeAttacking
			}
			if s.hasAuraType(spellAuraCharm) { // same source as CONDITION_CHARMED (44)
				state |= unitStateCharmed
			}
			if s.hasAuraType(spellAuraModStun) { // SpellAuraEffects HandleAuraModStun arm
				state |= unitStateStunned
			}
			if s.hasAuraType(spellAuraModFear) { // SpellAuraEffects HandleAuraModFear arm
				state |= unitStateFleeing
			}
			if s.inFlight { // same source as CONDITION_TAXI (46)
				state |= unitStateInFlight
			}
			if s.rooted { // UNIT_STATE_ROOT
				state |= unitStateRoot
			}
			if s.hasAuraType(spellAuraModConfuse) { // SpellAuraEffects HandleAuraModConfuse arm
				state |= unitStateConfused
			}
			if s.activeCast != nil { // UNIT_STATE_CASTING
				state |= unitStateCasting
			}
			if s.isMoving { // UNIT_STATE_MOVE
				state |= unitStateMove
			}
			// Deliberately unset: DIED (no feign-death aura type in Go),
			// ATTACK_PLAYER (contested-PvP only, Player.cpp:20731-20744; no
			// Go contested-PvP model), DISTRACTED/ISOLATED/POSSESSED and the
			// movement-generator states (ROAMING/CHASE/FOCUSING/FOLLOW/
			// CHARGING/JUMPING/ROTATING/EVADE and the *_MOVE arms — no Go
			// movement-generator state model).
		} else {
			if s.server == nil || creatureGUID == 0 {
				return false, nil
			}
			s.server.motionMu.Lock()
			motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, creatureGUID)
			s.server.motionMu.Unlock()
			if motion == nil {
				return false, nil
			}
			if motion.TargetGUID != 0 {
				state |= unitStateMeleeAttacking
			}
			if motion.Charmed {
				state |= unitStateCharmed
			}
			// No Go aura model on creature motions and no contested-PvP
			// model; the remaining states stay unset (documented above).
		}
		return state&uint32(row.Value1) != 0, nil
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
	case 29: // CONDITION_NEAR_CREATURE (ConditionMgr.cpp:348-351:
		// condMeets = object->FindNearestCreature(ConditionValue1,
		// (float)ConditionValue2, bool(!ConditionValue3)) != nullptr. C++
		// IsValid rejects the row at load when the creature template is
		// missing (2095-2103); mirror that as fail-closed. The search
		// origin resolves via conditionTargetPos; an unavailable position
		// fails closed, like the C++ null arm.
		if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return false, nil
		}
		var templateExists int
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature_template WHERE entry = ?", uint32(row.Value1)).Scan(&templateExists); err != nil {
			return false, nil
		}
		ox, oy, oz, ok := s.conditionTargetPos(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		excludeGUID := s.playerGUID
		if row.ConditionTarget == 1 {
			excludeGUID = creatureGUID
		}
		return s.nearestCreatureEntryInRange(uint32(row.Value1), float32(row.Value2), ox, oy, oz, excludeGUID, row.Value3 == 0), nil
	case 30: // CONDITION_NEAR_GAMEOBJECT (ConditionMgr.cpp:353-356:
		// condMeets = object->FindNearestGameObject(ConditionValue1,
		// (float)ConditionValue2) != nullptr, spawnedOnly defaulting to
		// true). C++ IsValid rejects the row at load when the gameobject
		// template is missing (2104-2111); mirror that as fail-closed. The
		// search origin resolves via conditionTargetPos; an unavailable
		// position fails closed, like the C++ null arm.
		if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return false, nil
		}
		var goTemplateExists int
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM gameobject_template WHERE entry = ?", uint32(row.Value1)).Scan(&goTemplateExists); err != nil {
			return false, nil
		}
		ox, oy, oz, ok := s.conditionTargetPos(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		return s.nearestGameObjectEntryInRange(ctx, uint32(row.Value1), float32(row.Value2), ox, oy, oz), nil
	case 31: // CONDITION_OBJECT_ENTRY_GUID (ConditionMgr.cpp:358-380: type id
		// must match, then the entry matches unless Value2 is 0, then the
		// spawn id matches when Value3 is set)
		// C++ IsValid rejects the row at load for dangling template/data
		// references (2113-2160); mirror those as fail-closed. The condition
		// object is the ConditionTarget endpoint (ConditionMgr.cpp:130-138):
		// target 0 is the player (TYPEID_PLAYER = 4, ObjectGuid.h:38 — Player
		// never sets OBJECT_FIELD_ENTRY, so its entry reads 0), target 1 the
		// gossip/vendor creature (TYPEID_UNIT = 3) resolved through the
		// motion registry; anything else or an unresolvable target fails
		// closed, matching the C++ null-target arm. The creature's spawn id
		// is the low 24 bits of its motion key: the spawn DB guid for world
		// creatures (creatureWorldGUID, creatures.go:376), the pet number
		// for pets (makePetGUID, pets.go:108) — exactly C++ GetSpawnId()
		// (Creature.h:87; Pet.cpp sets m_spawnId = guidlow). Go serves no
		// gameobject condition object, so TYPEID_GAMEOBJECT (5) rows fail
		// the type check, C++-exact in these contexts (the gameobject-data
		// IsValid arm stays unmodeled for the same reason).
		if s == nil || s.player == nil {
			return false, nil
		}
		if uint32(row.Value1) == 3 { // TYPEID_UNIT
			if row.Value2 != 0 || row.Value3 != 0 {
				if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
					return false, nil
				}
				if row.Value2 != 0 {
					var templateExists int
					if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature_template WHERE entry = ?", uint32(row.Value2)).Scan(&templateExists); err != nil {
						return false, nil
					}
				}
				if row.Value3 != 0 {
					var dataEntry uint32
					if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", uint32(row.Value3)).Scan(&dataEntry); err != nil {
						return false, nil
					}
					if row.Value2 != 0 && dataEntry != uint32(row.Value2) {
						return false, nil
					}
				}
			}
		}
		var typeID, entry, spawnID uint32
		switch row.ConditionTarget {
		case 0:
			typeID = 4 // TYPEID_PLAYER (ObjectGuid.h:38)
		case 1:
			if s.server == nil || creatureGUID == 0 {
				return false, nil
			}
			s.server.motionMu.Lock()
			motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, creatureGUID)
			s.server.motionMu.Unlock()
			if motion == nil {
				return false, nil
			}
			typeID = 3 // TYPEID_UNIT (ObjectGuid.h:37)
			entry = motion.Entry
			spawnID = uint32(motion.GUID & 0xFFFFFF)
		default:
			return false, nil
		}
		if typeID != uint32(row.Value1) {
			return false, nil
		}
		meets := row.Value2 == 0 || entry == uint32(row.Value2)
		if row.Value3 != 0 && typeID == 3 { // TYPEID_UNIT; other type ids ignore Value3 (default arm)
			meets = meets && spawnID == uint32(row.Value3)
		}
		return meets, nil
	case 32: // CONDITION_TYPE_MASK (ConditionMgr.cpp:381-385: object->isType(ConditionValue1))
		// The condition object is the ConditionTarget entry of the target list
		// (Condition::Meets, ConditionMgr.cpp:130-138); a missing target fails
		// closed. Target 0 is the player in every context evalCondition serves
		// (quest conditions: Player.cpp:15906/16337; gossip: Player.cpp:14392
		// and 14722 pass the player first); target 1 is the gossip/vendor
		// creature when one is involved, absent otherwise.
		var objectType uint16
		switch row.ConditionTarget {
		case 0:
			objectType = playerObjectTypeMask
		case 1:
			if creatureEntry == 0 {
				return false, nil
			}
			objectType = creatureObjectTypeMask
		default:
			return false, nil
		}
		return uint16(row.Value1)&objectType != 0, nil
	case 33: // CONDITION_RELATION_TO (ConditionMgr.cpp:386-420)
		// C++ IsValid rejects the row at load when Value1 >= max targets,
		// Value1 == ConditionTarget, or Value2 >= RELATION_MAX (2185-2202);
		// mirror that as fail-closed. Go serves targets {0,1} like the
		// 32/35 units, and a missing target (or a non-unit one) leaves
		// condMeets false, like the C++ null/ToUnit arms.
		if row.Value1 < 0 || row.Value1 > 1 || row.Value1 == row.ConditionTarget ||
			row.Value2 < 0 || row.Value2 > 5 {
			return false, nil
		}
		u1, ok := s.conditionTargetUnit(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		u2, ok := s.conditionTargetUnit(row.Value1, creatureGUID)
		if !ok {
			return false, nil
		}
		return s.relationToMeets(ctx, u1, u2, uint32(row.Value2)), nil
	case 34: // CONDITION_REACTION_TO (ConditionMgr.cpp:421-430:
		// condMeets = ((1 << unit->GetReactionTo(toUnit)) & ConditionValue2)
		// != 0, unit the ConditionTarget endpoint and toUnit the Value1
		// endpoint. C++ IsValid rejects the row at load when Value1 >= max
		// targets, Value1 == ConditionTarget, or Value2 == 0 (2204-2222);
		// mirror that as fail-closed. A missing target or a non-unit
		// endpoint leaves condMeets false, like the C++ null/ToUnit arms.
		if row.Value1 < 0 || row.Value1 > 1 || row.Value1 == row.ConditionTarget || row.Value2 == 0 {
			return false, nil
		}
		u1, ok := s.conditionTargetUnit(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		u2, ok := s.conditionTargetUnit(row.Value1, creatureGUID)
		if !ok {
			return false, nil
		}
		return (uint64(1)<<s.reactionRankTo(u1, u2))&uint64(row.Value2) != 0, nil
	case 35: // CONDITION_DISTANCE_TO (ConditionMgr.cpp:432-437:
		// condMeets = CompareValues(ComparisionType(Value3),
		// object->GetDistance(toObject), float(Value2)), with toObject the
		// ConditionValue1 target). C++ IsValid rejects the row at load when
		// Value1 >= max targets, Value1 == ConditionTarget, or Value3 >=
		// COMP_TYPE_MAX (2223-2238); mirror that as fail-closed. A missing
		// target leaves condMeets false, like the C++ null arm.
		if row.Value1 < 0 || row.Value1 > 1 || row.Value1 == row.ConditionTarget ||
			row.Value3 < 0 || row.Value3 >= 6 {
			return false, nil
		}
		ox, oy, oz, ok := s.conditionTargetPos(row.ConditionTarget, creatureGUID)
		if !ok {
			return false, nil
		}
		tx, ty, tz, ok := s.conditionTargetPos(row.Value1, creatureGUID)
		if !ok {
			return false, nil
		}
		return compareValuesFloat(row.Value3, float32(distance3D(ox, oy, oz, tx, ty, tz)), float32(row.Value2)), nil
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
	case 39: // CONDITION_REALM_ACHIEVEMENT
		return s.realmAchievementCompleted(ctx, uint32(row.Value1)), nil
	case 40: // CONDITION_IN_WATER
		return s.isSwimming, nil
	case 42: // CONDITION_STAND_STATE
		if s.player == nil {
			return false, nil
		}
		return (row.Value1 == 0 && int64(s.player.StandState) == row.Value2) || (row.Value1 == 1 && row.Value2 == 0 && s.player.StandState == 0), nil
	case 43: // CONDITION_DAILY_QUEST_DONE
		return false, nil
	case 44: // CONDITION_CHARMED
		return s.hasAuraType(spellAuraCharm), nil
	case 45: // CONDITION_PET_TYPE (ConditionMgr.cpp:527-533: (1 << pet->getPetType()) & ConditionValue1)
		if row.Value1 >= 1<<maxPetType { // C++ skips such rows at load; fail closed
			return false, nil
		}
		unit, ok := s.conditionTargetUnit(row.ConditionTarget, creatureGUID)
		if !ok || !unit.isPlayer { // object->ToPlayer() null arm
			return false, nil
		}
		petType, ok := s.activePetType()
		if !ok { // player->GetPet() null arm
			return false, nil
		}
		return (int64(1)<<petType)&row.Value1 != 0, nil
	case 46: // CONDITION_TAXI
		return s.inFlight, nil
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
	case 48: // CONDITION_QUEST_OBJECTIVE_PROGRESS
		if s.player == nil {
			return false, nil
		}
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			entry := s.player.QuestLog[slot]
			if entry.QuestID != uint32(row.Value1) {
				continue
			}
			counter := row.Value2
			if counter < 0 || counter >= int64(len(entry.Counters)) {
				return false, nil
			}
			return entry.Counters[counter] == uint16(row.Value3), nil
		}
		return false, nil
	case 49: // CONDITION_DIFFICULTY_ID
		if s.player == nil {
			return false, nil
		}
		difficulty, _ := s.loginInstanceDifficulty(ctx, *s.player)
		return difficulty == uint32(row.Value1), nil
	case 50: // CONDITION_GAMEMASTER
		if s.player == nil {
			return false, nil
		}
		if row.Value1 == 1 {
			return s.security >= 1, nil
		}
		return s.player.ExtraFlags&playerExtraGMOn != 0, nil
	default:
		return false, nil
	}
}

// nearestCreatureEntryInRange ports WorldObject::FindNearestCreature
// (Object.cpp:2149-2156) with the
// NearestCreatureEntryWithLiveStateInObjectRangeCheck terms
// (GridNotifiers.h:1329-1353): same map and instance, matching entry, the
// alive arm against the Go death model (Health > 0 is C++ ALIVE; Health ==
// 0 while still registered is the lingering JUST_DIED/CORPSE corpse, the
// registry prune being C++ DEAD removal), the search origin excluded by
// GUID, and 3D distance within radius (C++ IsWithinDistInMap). Reports
// whether any creature qualifies.
func (s *session) nearestCreatureEntryInRange(entry uint32, radius float32, x, y, z float32, excludeGUID uint64, alive bool) bool {
	if s == nil || s.server == nil || s.player == nil {
		return false
	}
	srv := s.server
	srv.motionMu.Lock()
	defer srv.motionMu.Unlock()
	for _, motion := range srv.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if motion == nil || motion.Map != s.player.Map || motion.InstanceID != s.player.InstanceID {
			continue
		}
		if motion.Entry != entry || motion.GUID == excludeGUID {
			continue
		}
		if (motion.Health > 0) != alive {
			continue
		}
		if distance3D(x, y, z, motion.X, motion.Y, motion.Z) > float64(radius) {
			continue
		}
		return true
	}
	return false
}

// nearestGameObjectEntryInRange ports WorldObject::FindNearestGameObject
// (Object.cpp:2158-2166) with the NearestGameObjectEntryInObjectRangeCheck
// terms (GridNotifiers.h:736-760): same map and instance, matching entry,
// the spawned-only arm (spawnedOnly defaults to true at the condition call
// site), and 3D distance within radius (C++ IsWithinDistInMap). The checker's
// grid sweep is Go's runtime-plus-static container scan: registry entries
// are spawned by construction (despawned runtime gameobjects are deleted
// from the registry, hidden ones skipped by the scan), while static DB rows
// have no respawn-state model and are treated as in-grid, matching the
// spell-target scan's semantics. The checker's GUID self-exclusion is
// vacuous here — the search origin is always the player or a gossip/vendor
// creature while gameobject GUIDs carry the 0xF110 high part, so they can
// never collide. Phase masks have no Go model (same as condition 29).
// Reports whether any gameobject qualifies.
func (s *session) nearestGameObjectEntryInRange(ctx context.Context, entry uint32, radius float32, x, y, z float32) bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	found := false
	s.scanNearbyGameObjectsAt(ctx, s.player.Map, s.player.InstanceID, x, y, radius, func(g nearbyGameObject) {
		if found || g.entry != entry {
			return
		}
		if distance3D(x, y, z, g.x, g.y, g.z) > float64(radius) {
			return
		}
		found = true
	})
	return found
}

// conditionTargetPos resolves a ConditionTarget index to a world position.
// Target 0 is the player in every context evalCondition serves; target 1 is
// the gossip/vendor creature, resolved through the creature motion registry
// by instance GUID. Any other target — or one whose position is unavailable
// — fails closed, matching the C++ null-target arm of CONDITION_DISTANCE_TO
// (ConditionMgr.cpp:432-437: toObject null leaves condMeets false).
func (s *session) conditionTargetPos(target int64, creatureGUID uint64) (x, y, z float32, ok bool) {
	if s == nil || s.player == nil {
		return 0, 0, 0, false
	}
	switch target {
	case 0:
		return s.player.X, s.player.Y, s.player.Z, true
	case 1:
		if s.server == nil || creatureGUID == 0 {
			return 0, 0, 0, false
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, creatureGUID)
		s.server.motionMu.Unlock()
		if motion == nil {
			return 0, 0, 0, false
		}
		return motion.X, motion.Y, motion.Z, true
	default:
		return 0, 0, 0, false
	}
}

// conditionUnit is the Go rendering of a Unit endpoint of a condition
// target, for the arms that need ToUnit() on both object and toObject
// (CONDITION_RELATION_TO, ConditionMgr.cpp:386-420). Target 0 is the player;
// target 1 is the gossip/vendor creature, resolved through the creature
// motion registry by instance GUID like conditionTargetPos. Anything else —
// or an unresolvable target — fails closed, matching the C++ null-target arm.
type conditionUnit struct {
	isPlayer    bool
	guid        uint64
	ownerGUID   uint64 // UNIT_FIELD_SUMMONEDBY; players never carry one (Unit.h:1147)
	charmed     bool
	charmerGUID uint64
	faction     uint32 // faction template id; creatures only
	entry       uint32 // creature entry; 0 for the player
	vehicleBase uint64 // player.VehicleGUID; creatures have no Go vehicle model
}

func (s *session) conditionTargetUnit(target int64, creatureGUID uint64) (conditionUnit, bool) {
	if s == nil || s.player == nil {
		return conditionUnit{}, false
	}
	switch target {
	case 0:
		return conditionUnit{isPlayer: true, guid: s.playerGUID, vehicleBase: s.player.VehicleGUID}, true
	case 1:
		if s.server == nil || creatureGUID == 0 {
			return conditionUnit{}, false
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, creatureGUID)
		s.server.motionMu.Unlock()
		if motion == nil {
			return conditionUnit{}, false
		}
		return conditionUnit{
			guid:        creatureGUID,
			ownerGUID:   motion.OwnerGUID,
			charmed:     motion.Charmed,
			charmerGUID: motion.CharmerGUID,
			faction:     motion.Faction,
			entry:       motion.Entry,
		}, true
	default:
		return conditionUnit{}, false
	}
}

// activePetType resolves the player's current pet to its PetType (the
// character_pet.PetType value Go stores on the pet motion at register time),
// the Pet::getPetType() source for CONDITION_PET_TYPE
// (ConditionMgr.cpp:527-533). A dismissed pet (PetGUID 0) or one whose motion
// is gone fails closed, matching the C++ GetPet() null arm.
func (s *session) activePetType() (uint8, bool) {
	if s == nil || s.player == nil || s.server == nil || s.player.PetGUID == 0 {
		return 0, false
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	s.server.motionMu.Unlock()
	if motion == nil || motion.PetID == 0 {
		return 0, false
	}
	return motion.PetType, true
}

// charmerOrOwnerOrSelfGUID ports the identity resolution inside
// Unit::IsInPartyWith/IsInRaidWith via WorldObject::GetCharmerOrOwnerOrSelf
// (Object.cpp:2218-2225) and Unit::GetCharmerOrOwner (Unit.h:1175:
// IsCharmed() ? GetCharmer() : GetOwner(), else self). The C++ null-charmer
// fallback to self is not distinguished: a set charmer GUID resolves to the
// charmer, which is the live player session in every context evalCondition
// serves.
func charmerOrOwnerOrSelfGUID(u conditionUnit) uint64 {
	if u.charmed && u.charmerGUID != 0 {
		return u.charmerGUID
	}
	if !u.charmed && u.ownerGUID != 0 {
		return u.ownerGUID
	}
	return u.guid
}

// relationToMeets ports the CONDITION_RELATION_TO arms
// (ConditionMgr.cpp:386-420) over the two resolved unit endpoints.
func (s *session) relationToMeets(ctx context.Context, u1, u2 conditionUnit, relation uint32) bool {
	switch relation {
	case 0: // RELATION_SELF: unit == toUnit
		return u1.isPlayer == u2.isPlayer && u1.guid == u2.guid
	case 1, 2: // RELATION_IN_PARTY / RELATION_IN_RAID_OR_PARTY
		// (Unit.cpp:12126-12153, 12155-12182): charmer/owner-or-self identity
		// first — a player's pet is in party/raid with the player — then the
		// TREAT_AS_RAID_UNIT cross arm. The raw this == unit check is subsumed
		// by the identity arm. The player-player group arms
		// (IsInSameGroupWith/IsInSameRaidWith) and the same-faction creature
		// arm cannot fire here — Go's condition targets are always player (0)
		// versus creature (1) — and the npcbot arms have no Go model.
		if charmerOrOwnerOrSelfGUID(u1) == charmerOrOwnerOrSelfGUID(u2) {
			return true
		}
		var creature conditionUnit
		switch {
		case !u1.isPlayer:
			creature = u1
		case !u2.isPlayer:
			creature = u2
		default:
			return false
		}
		if s.server == nil || s.server.WorldStore == nil {
			return false
		}
		var typeFlags int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?", creature.entry).Scan(&typeFlags)
		return typeFlags&0x04000000 != 0 // CREATURE_TYPE_FLAG_TREAT_AS_RAID_UNIT (SharedDefines.h:2755)
	case 3: // RELATION_OWNED_BY: unit->GetOwnerGUID() == toUnit->GetGUID()
		return u1.ownerGUID != 0 && u1.ownerGUID == u2.guid
	case 4: // RELATION_PASSENGER_OF: unit->IsOnVehicle(toUnit)
		// (Unit.cpp:12074-12077: m_vehicle && m_vehicle == vehicle->GetVehicleKit();
		// the kit belongs to the vehicle base, which Go tracks as the
		// player's vehicle GUID)
		return u1.vehicleBase != 0 && u1.vehicleBase == u2.guid
	case 5: // RELATION_CREATED_BY: unit->GetCreatorGUID() == toUnit->GetGUID()
		// (Unit.h:1149: UNIT_FIELD_CREATEDBY; no Go model — players never
		// carry one and creature creators are untracked)
		return false
	default:
		return false
	}
}

// reactionRankTo ports WorldObject::GetReactionTo (Object.cpp:2708-2793)
// for the player/creature endpoint pairs Go condition contexts serve,
// returning the full ReputationRank (SharedDefines.h:209-220: HATED 0,
// HOSTILE 1, UNFRIENDLY 2, NEUTRAL 3, FRIENDLY 4, HONORED 5, REVERED 6,
// EXALTED 7). Self and charmer/owner-or-self identity are always friendly;
// the forced-reputation arm (SPELL_AURA_FORCE_REACTION) has no Go model;
// the duel/group/FFA_PVP player-controlled arms and the
// UNIT_FLAG2_IGNORE_REPUTATION check cannot fire here — endpoints are
// always player (0) versus creature (1), and Go models no creature FFA or
// ignore-reputation state.
func (s *session) reactionRankTo(u, toUnit conditionUnit) uint32 {
	if charmerOrOwnerOrSelfGUID(u) == charmerOrOwnerOrSelfGUID(toUnit) {
		return 4 // REP_FRIENDLY
	}
	var creature conditionUnit
	switch {
	case !u.isPlayer:
		creature = u
	case !toUnit.isPlayer:
		creature = toUnit
	default:
		return 4 // REP_FRIENDLY
	}
	player := playerPos{
		Race:        s.player.Race,
		Class:       s.player.Class,
		Reputations: playerReputationMap(s.player.Reputations),
		Sess:        s,
	}
	if s.server != nil {
		player.FactionTemplate = s.server.raceFaction(s.player.Race)
	}
	if u.isPlayer {
		return s.playerToCreatureRank(player, creature.faction)
	}
	return s.creatureToPlayerRank(creature.faction, player)
}

// playerToCreatureRank ports the player->creature arms of
// WorldObject::GetReactionTo (Object.cpp:2737-2790): the reputation branch
// is hostile only when at war (else friendly), contested guards are hostile
// to contested-PvP players, and non-reputation factions fall through to the
// faction-template terms.
func (s *session) playerToCreatureRank(player playerPos, creatureFaction uint32) uint32 {
	tpl, found := s.factionTemplateEntry(creatureFaction)
	if !found {
		return s.fallbackReactionRank(creatureFaction, player.Race)
	}
	if tpl.Flags&0x00001000 != 0 && player.Sess != nil && player.Sess.contestedPvPActive(time.Now()) { // FACTION_TEMPLATE_FLAG_CONTESTED_GUARD (DBCEnums.h:320)
		return 1 // REP_HOSTILE
	}
	if rep, ok, err := s.server.Data.Reputation(tpl.Faction, player.Race, player.Class); err == nil && ok && rep.ReputationList >= 0 {
		if saved, has := player.Reputations[tpl.Faction]; has && saved.Flags&factionFlagAtWar != 0 {
			return 1 // REP_HOSTILE
		}
		return 4 // REP_FRIENDLY
	}
	return s.factionTemplateCommonRank(tpl, player.FactionTemplate)
}

// creatureToPlayerRank ports the creature->player direction, which is the
// GetFactionReactionTo tail (Object.cpp:2795-2842): the CvP arm reads the
// player's actual standing rank and clamps it down to neutral at war, the
// contested-guard arm fires before the template terms, and the common
// faction check decides hostile/friendly/neutral.
func (s *session) creatureToPlayerRank(creatureFaction uint32, player playerPos) uint32 {
	tpl, found := s.factionTemplateEntry(creatureFaction)
	if !found {
		return 3 // REP_NEUTRAL — null faction template entry (Object.cpp:2797-2799)
	}
	if tpl.Flags&0x00001000 != 0 && player.Sess != nil && player.Sess.contestedPvPActive(time.Now()) { // FACTION_TEMPLATE_FLAG_CONTESTED_GUARD (DBCEnums.h:320)
		return 1 // REP_HOSTILE
	}
	if rep, ok, err := s.server.Data.Reputation(tpl.Faction, player.Race, player.Class); err == nil && ok && rep.ReputationList >= 0 {
		standing := int64(rep.BaseStanding)
		atWar := false
		if saved, has := player.Reputations[tpl.Faction]; has {
			standing = int64(totalReputationStanding(saved))
			atWar = saved.Flags&factionFlagAtWar != 0
		}
		rank := reputationRank(standing)
		if atWar && rank > 3 {
			rank = 3 // CvP clamp: std::min(REP_NEUTRAL, repRank) (Object.cpp:2821-2822)
		}
		return rank
	}
	return s.factionTemplateCommonRank(tpl, player.FactionTemplate)
}

// factionTemplateEntry looks up a FactionTemplate row, failing closed to the
// fallback path when the data store is unavailable.
func (s *session) factionTemplateEntry(faction uint32) (wotlk.FactionTemplate, bool) {
	if s.server == nil || s.server.Data == nil || faction == 0 {
		return wotlk.FactionTemplate{}, false
	}
	tpl, found, err := s.server.Data.FactionTemplate(faction)
	if err != nil || !found {
		return wotlk.FactionTemplate{}, false
	}
	return tpl, true
}

// factionTemplateCommonRank ports the common faction based check tail of
// WorldObject::GetFactionReactionTo (Object.cpp:2826-2842) using
// FactionTemplateEntry::IsHostileTo/IsFriendlyTo (DBCStructure.h:697-722).
func (s *session) factionTemplateCommonRank(creatureTpl wotlk.FactionTemplate, playerFaction uint32) uint32 {
	playerTpl, found := s.factionTemplateEntry(playerFaction)
	if !found {
		return s.fallbackReactionRank(creatureTpl.ID, 0)
	}
	if factionTemplateHostileTo(creatureTpl, playerTpl) {
		return 1 // REP_HOSTILE
	}
	if factionTemplateFriendlyTo(creatureTpl, playerTpl) || factionTemplateFriendlyTo(playerTpl, creatureTpl) {
		return 4 // REP_FRIENDLY
	}
	if creatureTpl.Flags&0x00002000 != 0 { // FACTION_TEMPLATE_FLAG_HOSTILE_BY_DEFAULT (DBCEnums.h:321)
		return 1 // REP_HOSTILE
	}
	return 3 // REP_NEUTRAL
}

// factionTemplateHostileTo ports FactionTemplateEntry::IsHostileTo
// (DBCStructure.h:710-722).
func factionTemplateHostileTo(a, b wotlk.FactionTemplate) bool {
	if b.Faction != 0 {
		for _, enemy := range a.Enemies {
			if enemy != 0 && enemy == b.Faction {
				return true
			}
		}
		for _, friend := range a.Friends {
			if friend != 0 && friend == b.Faction {
				return false
			}
		}
	}
	return a.EnemyGroup&b.FactionGroup != 0
}

// factionTemplateFriendlyTo ports FactionTemplateEntry::IsFriendlyTo
// (DBCStructure.h:697-709).
func factionTemplateFriendlyTo(a, b wotlk.FactionTemplate) bool {
	if b.Faction != 0 {
		for _, enemy := range a.Enemies {
			if enemy != 0 && enemy == b.Faction {
				return false
			}
		}
		for _, friend := range a.Friends {
			if friend != 0 && friend == b.Faction {
				return true
			}
		}
	}
	return a.FriendGroup&b.FactionGroup != 0 || a.FactionGroup&b.FriendGroup != 0
}

// fallbackReactionRank degrades the rank to the same fallback terms the
// hostile/friendly faction helpers use when DBC data is unavailable.
func (s *session) fallbackReactionRank(creatureFaction uint32, playerRace uint8) uint32 {
	if s.server != nil && s.server.isHostileFaction(creatureFaction, playerPos{Race: playerRace}) {
		return 1 // REP_HOSTILE
	}
	if isFriendlyFactionFallback(creatureFaction, playerRace) {
		return 4 // REP_FRIENDLY
	}
	return 3 // REP_NEUTRAL
}

// compareValuesFloat ports the float instantiation of TrinityCore
// CompareValues (Util.h:516), used by CONDITION_DISTANCE_TO against
// WorldObject::GetDistance and float(Value2).
func compareValuesFloat(compType int64, a, b float32) bool {
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
