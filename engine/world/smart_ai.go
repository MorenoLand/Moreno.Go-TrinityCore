// SmartAI engine — item-6 census Axis1#2 (slice 1: engine + hooks + TALK /
// SET_FACTION / PLAY_EMOTE / CAST / AUTO_ATTACK / SET_EVENT_PHASE;
// slice 2: the remaining AGGRO-used actions and targets; slice 3:
// timed-action lists (CALL_TIMED_ACTIONLIST / RANDOM / RANGE + the
// list engine below), the phase actions (INC/RANDOM/RANDOM_RANGE),
// ATTACK_START, and the small state actions (SET_REACT_STATE,
// ALLOW_COMBAT_MOVEMENT, REMOVE_NPC_FLAG, SET_ROOT, SET_VISIBILITY,
// SET_ACTIVE, INTERRUPT_SPELL) — of which the phase actions,
// ATTACK_START, SET_REACT_STATE, the NPC-flag family,
// ALLOW_COMBAT_MOVEMENT, SET_ROOT, SET_VISIBILITY and INTERRUPT_SPELL
// are ported; SET_ACTIVE stays residual.
//
// C++: SmartScript / SmartAI (src/server/game/AI/SmartScripts/SmartScript.cpp,
// SmartAI.cpp, SmartScriptMgr.h). Go had no SmartAI engine and no smart_scripts
// loader (the WORLD_SEL_SMART_SCRIPTS statement existed unused, the .reload
// arm was a probe-only stub), so every SmartAI creature's on-aggro script was
// silently dead. This file loads the creature-source rows (source_type 0), keeps
// per-creature runtime state on the motion, and dispatches the direct events
// (AGGRO, DEATH, DATA_SET, AI_INIT, JUST_CREATED, TEXT_OVER,
// EVENT_PHASE_CHANGE) plus the timer events (UPDATE, UPDATE_IC, UPDATE_OOC,
// HEALTH_PCT, TARGET_HEALTH_PCT, MANA_PCT, TARGET_MANA_PCT, RANGE) from the
// creature tick. Action coverage is the AGGRO-used set (TDB 26091: 1,634
// source_type-0 AGGRO rows over 1,426 entries): TALK, CAST, SET_FACTION,
// PLAY_EMOTE, AUTO_ATTACK, SET_EVENT_PHASE, SET_INST_DATA (type 1),
// CALL_FOR_HELP, SET_DATA, SOUND, STORE_TARGET_LIST, REMOVEAURASFROMSPELL,
// REMOVE_UNIT_FIELD_BYTES_1 (byte 0 only), SUMMON_CREATURE,
// CALL_TIMED_ACTIONLIST + CALL_RANDOM_TIMED_ACTIONLIST +
// CALL_RANDOM_RANGE_TIMED_ACTIONLIST (source_type-9 timed-action lists,
// installed per target creature, stepped from smartUpdateTimers), and
// SET_REACT_STATE + the SET/ADD/REMOVE_NPC_FLAG family, INC_EVENT_PHASE +
// RANDOM_PHASE + RANDOM_PHASE_RANGE + ATTACK_START, ALLOW_COMBAT_MOVEMENT +
// SET_ROOT + SET_VISIBILITY + INTERRUPT_SPELL + per-target PLAY_EMOTE.
// RESET wiring
// landed with smartReset / smartJustReachedHome (evade-follow legs, home
// arrival, spawn): the EVADE event fires from triggerCreatureEvade, RESPAWN +
// OnReset from fireCreatureSpawned. `.reload smart_scripts` re-runs the
// loader (reloadSmartScripts). Everything else (GO/instance/area-trigger
// sources) is a documented residual in
// smartProcessAction / the call sites below, not a silent gap.
//
// Locking: every entry point assumes motionMu is NOT held (the Nov-17
// motionMu self-deadlock). The three hook sites run after their local unlock:
// triggerCreatureAggro tail, applyCreatureDeathState post-unlock,
// stepCreatureMotion (motionMu is not held on that path, :7026).

package world

import (
	"context"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// SMART_EVENT ids (SmartScriptMgr.h:97-183).
const (
	smartEventUpdateIC            uint32 = 0
	smartEventUpdateOOC           uint32 = 1
	smartEventHealthPct           uint32 = 2
	smartEventManaPct             uint32 = 3
	smartEventAggro               uint32 = 4
	smartEventKill                uint32 = 5
	smartEventDeath               uint32 = 6
	smartEventEvade               uint32 = 7
	smartEventRange               uint32 = 9
	smartEventRespawn             uint32 = 11
	smartEventReachedHome         uint32 = 21
	smartEventReset               uint32 = 25
	smartEventDataSet             uint32 = 38 // SMART_EVENT_DATA_SET; 26 is SMART_EVENT_IC_LOS, not DATA_SET
	smartEventTargetHealthPc      uint32 = 12
	smartEventTargetManaPct       uint32 = 18
	smartEventAIInit              uint32 = 37
	smartEventTextOver            uint32 = 52
	smartEventTimedEventTriggered uint32 = 59 // SMART_EVENT_TIMED_EVENT_TRIGGERED (SmartScriptMgr.h:158), event_param1 = timed-event id
	smartEventUpdate              uint32 = 60
	smartEventLink                uint32 = 61
	smartEventJustCreated         uint32 = 63
	smartEventPhaseChange         uint32 = 66
	smartEventPhase12             uint32 = 12 // SMART_EVENT_PHASE_12 (SmartScriptMgr.h:55), the IncPhase clamp (SmartScript.cpp:3996-4000)
)

// SMART_ACTION ids (SmartScriptMgr.h:457-600), ported set. The
// remaining AGGRO-row action (146) hits the default arm: it has no enum
// value in this C++ revision (SMART_ACTION_END = 140), so C++ log-and-skips
// it too (SmartScript.cpp:2370-2372). The timed-event store (67/73/74/125
// + mStoredEvents/mRemIDs) is ported below.
// Full action census (TDB 26091, all 140 enum ids x Go consts x rows)
// lives in hidden_files/smartai_action_census.py; 6 FAIL_QUEST (89
// rows), 7 OFFER_QUEST (1 row, directAdd) and 9 ACTIVATE_GOBJECT (60
// live rows) are ported below with the GO/player/owner target arms
// (13/14/15/17/18/20/23). 15 CALL_AREAEXPLOREDOREVENTHAPPENS (187
// live rows), 26 CALL_GROUPEVENTHAPPENS (15) and 27 COMBAT_STOP (13)
// are ported below with the THREAT_LIST (24) and LOOT_RECIPIENTS (27)
// target arms. The next unported block starts at 29 FOLLOW (114
// live rows).
const (
	smartActionTalk                           uint32 = 1
	smartActionSetFaction                     uint32 = 2
	smartActionMorphToEntryOrModel            uint32 = 3
	smartActionSound                          uint32 = 4
	smartActionPlayEmote                      uint32 = 5
	smartActionFailQuest                      uint32 = 6
	smartActionOfferQuest                     uint32 = 7
	smartActionActivateGObject                uint32 = 9
	smartActionSetReactState                  uint32 = 8
	smartActionRandomEmote                    uint32 = 10
	smartActionCast                           uint32 = 11
	smartActionSummonCreature                 uint32 = 12
	smartActionThreatSinglePct                uint32 = 13
	smartActionThreatAllPct                   uint32 = 14
	smartActionCallAreaExploredOrEventHappens uint32 = 15
	smartActionSetEmoteState                  uint32 = 17
	smartActionAutoAttack                     uint32 = 20
	smartActionAllowCombatMovement            uint32 = 21
	smartActionSetEventPhase                  uint32 = 22
	smartActionIncEventPhase                  uint32 = 23
	smartActionEvade                          uint32 = 24
	smartActionFleeForAssist                  uint32 = 25
	smartActionCallGroupEventHappens          uint32 = 26
	smartActionCombatStop                     uint32 = 27
	smartActionRemoveAurasFromSpell           uint32 = 28
	smartActionRandomPhase                    uint32 = 30
	smartActionRandomPhaseRange               uint32 = 31
	smartActionSetInstData                    uint32 = 34
	smartActionUpdateTemplate                 uint32 = 36
	smartActionCallForHelp                    uint32 = 39
	smartActionForceDespawn                   uint32 = 41
	smartActionSetInvincibilityHPLevel        uint32 = 42
	smartActionMountToEntryOrModel            uint32 = 43
	smartActionSetData                        uint32 = 45
	smartActionSetVisibility                  uint32 = 47
	smartActionSetActive                      uint32 = 48
	smartActionAttackStart                    uint32 = 49
	smartActionStoreTargetList                uint32 = 64
	smartActionCreateTimedEvent               uint32 = 67
	smartActionTriggerTimedEvent              uint32 = 73
	smartActionRemoveTimedEvent               uint32 = 74
	smartActionCallTimedActionList            uint32 = 80
	smartActionSetNPCFlag                     uint32 = 81
	smartActionAddNPCFlag                     uint32 = 82
	smartActionRemoveNPCFlag                  uint32 = 83
	smartActionCallRandomTimedActionList      uint32 = 87
	smartActionCallRandomRangeTimedActionList uint32 = 88
	smartActionRemoveUnitFieldBytes1          uint32 = 91
	smartActionInterruptSpell                 uint32 = 92
	smartActionSetRoot                        uint32 = 103
	smartActionTriggerRandomTimedEvent        uint32 = 125
	smartActionRespawnBySpawnID               uint32 = 133
)

// SMART_TARGET ids (SmartScriptMgr.h:1221-1253), AGGRO-used set. Target 8
// (POSITION) resolves to no unit — GetTargets returns nothing for it
// (SmartScript.cpp:2864-2867) — and the SUMMON_CREATURE action reads the
// row's absolute target_x/y/z/o itself for that leg; target 13
// (GAMEOBJECT_RANGE) and 21 (CLOSEST_PLAYER) have zero AGGRO rows in
// TDB 26091.
const (
	smartTargetNone              uint32 = 0
	smartTargetSelf              uint32 = 1
	smartTargetVictim            uint32 = 2
	smartTargetActionInvoker     uint32 = 7
	smartTargetPosition          uint32 = 8
	smartTargetCreatureRange     uint32 = 9
	smartTargetCreatureGUID      uint32 = 10
	smartTargetCreatureDist      uint32 = 11
	smartTargetStored            uint32 = 12
	smartTargetGameObjectRange   uint32 = 13
	smartTargetGameObjectGUID    uint32 = 14
	smartTargetGameObjectDist    uint32 = 15
	smartTargetInvokerParty      uint32 = 16
	smartTargetPlayerRange       uint32 = 17
	smartTargetPlayerDist        uint32 = 18
	smartTargetClosestCreature   uint32 = 19
	smartTargetClosestGameObject uint32 = 20
	smartTargetOwnerOrSummoner   uint32 = 23
	smartTargetThreatList        uint32 = 24
	smartTargetLootRecipients    uint32 = 27
)

// SMART_EVENT_FLAG bits (SmartScriptMgr.h:1522-1531).
const (
	smartEventFlagNotRepeatable uint32 = 0x001
	smartEventFlagDontReset     uint32 = 0x100
	smartEventFlagDifficulty0   uint32 = 0x002
	smartEventFlagDifficulty1   uint32 = 0x004
	smartEventFlagDifficulty2   uint32 = 0x008
	smartEventFlagDifficulty3   uint32 = 0x010
	smartEventFlagWhileCharmed  uint32 = 0x200
)

const smartEventFlagDifficultyAll uint32 = smartEventFlagDifficulty0 | smartEventFlagDifficulty1 | smartEventFlagDifficulty2 | smartEventFlagDifficulty3

// SMARTCAST flags (SmartScriptMgr.h:1539-1549). Flag 0x04 (FORCE_CAST)
// is commented out in this C++ revision and has no leg.
const (
	smartCastInterruptPrevious uint32 = 0x01
	smartCastTriggered         uint32 = 0x02
	smartCastAuraNotPresent    uint32 = 0x20
	smartCastCombatMove        uint32 = 0x40
)

// SMART_EVENT_PHASE bits (SmartScriptMgr.h:61-78): phase 1 = bit 1, ...,
// phase 12 = bit 2048, all = 4095. SetPhase stores the phase NUMBER verbatim
// (SmartScript.cpp:4010-4014) and IsInPhase (:4020-4025) tests
// (1 << (phase-1)) & mask.
const smartEventPhaseAll uint32 = 4095

const smartMaxNestedEvents = 10

// smartScriptRow is one smart_scripts row (SmartScriptHolder: SmartScriptMgr.h).
// entryOrGUID is int32, not uint32: the DB column is a signed int and
// negative values are spawn-GUID-specific rows (SmartScriptMgr convention),
// which a uint32 scan would drop as out-of-range.
type smartScriptRow struct {
	entryOrGUID    int32
	sourceType     uint32
	id             uint32
	link           uint32
	eventType      uint32
	eventPhaseMask uint32
	eventChance    uint32
	eventFlags     uint32
	eventParam1    uint32
	eventParam2    uint32
	eventParam3    uint32
	eventParam4    uint32
	eventParam5    uint32
	actionType     uint32
	actionParam1   uint32
	actionParam2   uint32
	actionParam3   uint32
	actionParam4   uint32
	actionParam5   uint32
	actionParam6   uint32
	targetType     uint32
	targetParam1   uint32
	targetParam2   uint32
	targetParam3   uint32
	targetParam4   uint32
	targetX        float32
	targetY        float32
	targetZ        float32
	targetO        float32
}

// smartScriptEvent is a row plus its runtime state (SmartScriptHolder:
// timer, active, runOnce; SmartScript.cpp InitTimer/RecalcTimer).
// enableTimed is the timed-action-list chaining flag (SmartScriptMgr.h:1573):
// only the enabled list event ticks, and firing it enables the next one.
// timedEventID is the C++ event_id of a stored timed event (SmartScript.cpp:
// CREATE_TIMED_EVENT stores the synthetic holder with event_id = the timed
// event's id; RemoveStoredEvent matches on it); 0 on ordinary rows.
type smartScriptEvent struct {
	row          *smartScriptRow
	timer        uint32
	active       bool
	runOnce      bool
	enableTimed  bool
	timedEventID uint32
}

// smartScriptState is the per-creature SmartScript (SmartScript.h): the
// event list, current phase number (default 0 — SmartScript.cpp:51, the
// ctor sets mEventPhase = 0, so a nonzero phase mask matches nothing until
// a SET_EVENT_PHASE arm runs), last invoker, the TALK duration timer that
// fires SMART_EVENT_TEXT_OVER, and the stored-target lists
// (SmartScript::_storedTargets, SmartScript.cpp:108-121). timedList is the
// installed SMART_SCRIPT_TYPE_TIMED_ACTIONLIST (SmartScript.h:109): its
// events run one at a time, chained by enableTimed, with timedListInvoker
// as their invoker; processingTimedList is the isProcessingTimedActionList
// guard (SmartScript.cpp:3952-3958) that refuses a list overwrite from
// inside a list event's own action.
type smartScriptState struct {
	events               []*smartScriptEvent
	phase                uint32
	nested               int
	linkDepth            int
	lastInvoker          uint64
	defaultFaction       uint32
	textTimer            uint32
	useTextTimer         bool
	lastTextID           uint32
	talkerEntry          uint32
	lastTick             time.Time
	firedInit            bool
	storedTargets        map[uint32][]uint64
	timedList            []*smartScriptEvent
	timedListInvoker     uint64
	processingTimedList  bool
	invincibilityHPLevel uint32
	storedEvents         []*smartScriptEvent // mStoredEvents (SmartScript.h:122): CREATE_TIMED_EVENT holders, ticked in smartUpdateTimers
	remTimedEventIDs     []uint32            // mRemIDs (SmartScript.h:123): stored-event ids queued for removal, drained after the timed-list leg
}

// loadSmartScripts loads the creature-source smart_scripts rows into the
// server store (SmartScriptMgr::LoadSmartScripts), plus the source_type-9
// timed-action-list rows into their own map (SmartAIMgr::GetScript,
// SmartScriptMgr.cpp:380-391: lists are keyed by entryOrGuid alone).
// The repo placeholder world.db carries zero rows, so this is a no-op
// there; a populated world DB fills the maps. GO/instance/area-trigger
// sources are out of scope. The
// conditions table is loaded alongside as a skip-set: TDB 26091 carries
// 1,205 SourceType-22 (smart event) condition rows, 69 of them on AGGRO
// events (18 distinct events), and Go has no conditions engine, so an event
// with any condition row never fires rather than firing unconditionally —
// the conservative twin of IsObjectMeetingSmartEventConditions
// (SmartScript.cpp:240, ProcessEventsFor). A missing conditions table
// leaves the set empty, which is exact: no table means no conditions.
func (s *Server) loadSmartScripts(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.smartScriptsMu.Lock()
	if s.smartScriptsLoaded {
		s.smartScriptsMu.Unlock()
		return
	}
	s.smartScriptsLoaded = true
	s.smartScriptsMu.Unlock()
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT entryorguid, source_type, id, link, event_type, event_phase_mask, event_chance, event_flags, event_param1, event_param2, event_param3, event_param4, event_param5, action_type, action_param1, action_param2, action_param3, action_param4, action_param5, action_param6, target_type, target_param1, target_param2, target_param3, target_param4, target_x, target_y, target_z, target_o FROM smart_scripts ORDER BY entryorguid, source_type, id, link`)
	if err != nil {
		return
	}
	defer rows.Close()
	byEntry := make(map[int32][]*smartScriptRow)
	byList := make(map[int32][]*smartScriptRow)
	for rows.Next() {
		var r smartScriptRow
		var tx, ty, tz, to float64
		if err := rows.Scan(&r.entryOrGUID, &r.sourceType, &r.id, &r.link, &r.eventType, &r.eventPhaseMask, &r.eventChance, &r.eventFlags, &r.eventParam1, &r.eventParam2, &r.eventParam3, &r.eventParam4, &r.eventParam5, &r.actionType, &r.actionParam1, &r.actionParam2, &r.actionParam3, &r.actionParam4, &r.actionParam5, &r.actionParam6, &r.targetType, &r.targetParam1, &r.targetParam2, &r.targetParam3, &r.targetParam4, &tx, &ty, &tz, &to); err != nil {
			continue
		}
		if r.sourceType != 0 && r.sourceType != 9 {
			continue
		}
		r.targetX, r.targetY, r.targetZ, r.targetO = float32(tx), float32(ty), float32(tz), float32(to)
		row := r
		if row.sourceType == 9 {
			byList[row.entryOrGUID] = append(byList[row.entryOrGUID], &row)
			continue
		}
		byEntry[row.entryOrGUID] = append(byEntry[row.entryOrGUID], &row)
	}
	s.smartScriptsMu.Lock()
	s.smartScripts = byEntry
	s.smartTimedActionLists = byList
	s.smartScriptsMu.Unlock()
	s.loadSmartConditions(ctx)
}

// loadSmartConditions fills the smart-event condition skip-set (see
// loadSmartScripts). A query error (missing table) leaves it empty.
func (s *Server) loadSmartConditions(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT SourceEntry, SourceId FROM conditions WHERE SourceType = 22`)
	if err != nil {
		return
	}
	defer rows.Close()
	conds := make(map[smartConditionKey]struct{})
	for rows.Next() {
		var entry int32
		var id uint32
		if err := rows.Scan(&entry, &id); err != nil {
			continue
		}
		conds[smartConditionKey{entry: entry, id: id}] = struct{}{}
	}
	s.smartScriptsMu.Lock()
	s.smartConditions = conds
	s.smartScriptsMu.Unlock()
}

// reloadSmartScripts is the `.reload smart_scripts` twin of C++
// HandleReloadSmartScripts (cs_reload.cpp:1156-1163), which re-runs
// SmartAIMgr::LoadSmartAIFromDB: the load-once guard drops so the
// loader re-reads the table (and the conditions skip-set with it).
// Attached motions keep their loaded events, matching C++ — live
// SmartScripts hold their own mEvents copies (SmartScript.h:107), so
// only creatures spawned or attached after the reload see new rows.
// A failed reload keeps the previous rows (the loader returns early
// on query error); C++ clears first and ends up empty instead.
func (s *Server) reloadSmartScripts(ctx context.Context) {
	if s == nil {
		return
	}
	s.smartScriptsMu.Lock()
	s.smartScriptsLoaded = false
	s.smartScriptsMu.Unlock()
	s.loadSmartScripts(ctx)
}

// reloadSmartConditions re-reads the smart-event condition skip-set:
// the `.reload conditions` leg for SmartAI. C++ reloads the whole
// conditions store (sConditionMgr->LoadConditions) and SmartScript
// checks it live per event; Go snapshots the SourceType-22 rows, so
// the snapshot refreshes here.
func (s *Server) reloadSmartConditions(ctx context.Context) {
	s.loadSmartConditions(ctx)
}

// smartConditionKey is one conditions row for a smart event: SourceType 22,
// SourceEntry = entryOrGuid, SourceId = event id (ConditionMgr.h).
type smartConditionKey struct {
	entry int32
	id    uint32
}

func (s *Server) smartHasConditions(entry int32, id uint32) bool {
	if s == nil {
		return false
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	_, ok := s.smartConditions[smartConditionKey{entry: entry, id: id}]
	return ok
}

func (s *Server) smartRowsFor(entry int32) []*smartScriptRow {
	if s == nil {
		return nil
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	return s.smartScripts[entry]
}

// smartTimedActionListRows is SmartAIMgr::GetScript (SmartScriptMgr.cpp:
// 380-391) for SMART_SCRIPT_TYPE_TIMED_ACTIONLIST: the list rows by id,
// or nil when the list does not exist (C++ logs at debug and returns an
// empty list; the install then clears and stops).
func (s *Server) smartTimedActionListRows(listID int32) []*smartScriptRow {
	if s == nil {
		return nil
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	return s.smartTimedActionLists[listID]
}

// smartStateFor returns the creature's SmartScript state, attaching it on
// first use when the creature is SmartAI and has script rows (SmartAI ctor
// + SmartScript::OnInitialize). Init events fire here, in C++ order
// (AI_INIT, then JUST_CREATED), because attach runs on the unlocked tick
// path; motionForLocked cannot fire them (it runs under motionMu).
func (s *Server) smartStateFor(ctx context.Context, motion *creatureMotion) *smartScriptState {
	if s == nil || motion == nil || motion.AIName != "SmartAI" {
		return nil
	}
	if motion.smart != nil {
		return motion.smart
	}
	rows := s.smartRowsFor(int32(motion.Entry))
	// Spawn-GUID-specific rows (SmartScriptMgr convention: a negative
	// entryorguid is -spawnGUID, TDB 26091: 59 AGGRO rows over 56 spawns)
	// ride after the entry rows, like C++'s GetScript merge.
	if guidRows := s.smartRowsFor(-int32(uint32(motion.GUID & 0x00FFFFFF))); len(guidRows) > 0 {
		rows = append(append([]*smartScriptRow{}, rows...), guidRows...)
	}
	if len(rows) == 0 {
		return nil
	}
	st := &smartScriptState{phase: 0, defaultFaction: motion.Faction}
	for _, row := range rows {
		ev := &smartScriptEvent{row: row}
		s.smartInitTimer(ev)
		st.events = append(st.events, ev)
	}
	motion.smart = st
	st.firedInit = true
	s.smartProcessEvents(ctx, motion, smartEventAIInit, 0, 0, 0)
	s.smartProcessEvents(ctx, motion, smartEventJustCreated, 0, 0, 0)
	return st
}

// smartInitTimer is SmartScript::InitTimer (:3472-3486): timed-update events
// start on their initial min/max roll, everything else starts active.
func (s *Server) smartInitTimer(ev *smartScriptEvent) {
	switch ev.row.eventType {
	case smartEventUpdate, smartEventUpdateIC, smartEventUpdateOOC:
		s.smartRecalcTimer(ev, ev.row.eventParam1, ev.row.eventParam2)
	default:
		ev.active = true
	}
}

// smartRecalcTimer is SmartScript::RecalcTimer (:3491-3496): timer =
// urand(min, max); active = (timer == 0).
func (s *Server) smartRecalcTimer(ev *smartScriptEvent, min, max uint32) {
	if max < min {
		max = min
	}
	if max == min {
		ev.timer = min
	} else {
		ev.timer = min + uint32(rand.Int63n(int64(max-min+1)))
	}
	ev.active = ev.timer == 0
}

// smartReset is SmartScript::OnReset (SmartScript.cpp:170-189): every
// event without the DONT_RESET flag re-inits its timer and clears
// runOnce, then the SMART_EVENT_RESET events fire, then the last
// invoker clears. The priority reset has no Go twin (no per-event
// priority field); ResetBaseObject is a no-op (targets resolve from
// the motion on every call, there is no cached base object). Call
// sites are the C++ OnReset callers in SmartAI.cpp: EnterEvadeMode
// (:453, only when the EVADE state cleared — the owner-follow legs),
// JustAppeared (:539, after the RESPAWN event) and JustReachedHome
// (:544, before the REACHED_HOME event).
func (s *Server) smartReset(ctx context.Context, motion *creatureMotion) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	for _, ev := range st.events {
		if ev.row.eventFlags&smartEventFlagDontReset == 0 {
			s.smartInitTimer(ev)
			ev.runOnce = false
		}
	}
	st.lastInvoker = 0
	s.smartProcessEvents(ctx, motion, smartEventReset, 0, 0, 0)
}

// smartJustReachedHome is SmartAI::JustReachedHome (SmartAI.cpp:542-545):
// OnReset, then the SMART_EVENT_REACHED_HOME events fire.
func (s *Server) smartJustReachedHome(ctx context.Context, motion *creatureMotion) {
	s.smartReset(ctx, motion)
	s.smartProcessEvents(ctx, motion, smartEventReachedHome, 0, 0, 0)
}

func (st *smartScriptState) inPhase(mask uint32) bool {
	if mask == 0 {
		return true
	}
	if st.phase == 0 {
		return false
	}
	return (uint32(1)<<(st.phase-1))&mask != 0
}

// smartProcessEvents is SmartScript::ProcessEventsFor (:221-250): every
// non-LINK event of the given type runs, nested calls capped at 10.
// Conditioned events never fire from here (the skip-set in loadSmartScripts);
// C++ checks conditions at this same point (:240), not inside ProcessEvent,
// so LINK-chained events keep firing condition-free below.
func (s *Server) smartProcessEvents(ctx context.Context, motion *creatureMotion, eventType uint32, invokerGUID uint64, var0, var1 uint32) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	st.nested++
	if st.nested > smartMaxNestedEvents {
		st.nested--
		return
	}
	st.linkDepth = 0
	for _, ev := range st.events {
		if ev.row.eventType == smartEventLink {
			continue
		}
		if ev.row.eventType == eventType {
			if s.smartHasConditions(ev.row.entryOrGUID, ev.row.id) {
				continue
			}
			s.smartProcessEvent(ctx, motion, st, ev, invokerGUID, var0, var1)
		}
	}
	st.nested--
}

// smartSpawnMode is the C++ GetSpawnMode value FillScript gates difficulty
// flags on (SmartScript.cpp:3797-3808): 0 normal, 1 heroic (5-man), 0-3
// 10N/25N/10H/25H (raid). Go has no per-instance spawn mode, so the
// engaging player's difficulty stands in (playerSpawnMode, commands_wp.go:
// raid maps read RaidDifficulty, dungeon maps DungeonDifficulty, world
// maps 0) — identical in practice for dungeon/raid engages; a
// creature-invoked dispatch reads 0.
func (s *Server) smartSpawnMode(invokerGUID uint64) uint32 {
	if sess := s.findSessionByGUID(invokerGUID); sess != nil {
		return sess.playerSpawnMode()
	}
	return 0
}

// smartProcessEvent is SmartScript::ProcessEvent (:2882-...): the active,
// phase-mask, NOT_REPEATABLE, WHILE_CHARMED and difficulty gates, then the
// per-type dispatch — direct events run their action, timed events run
// ProcessTimedAction.
func (s *Server) smartProcessEvent(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64, var0, var1 uint32) {
	if !ev.active && ev.row.eventType != smartEventLink {
		return
	}
	if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
		return
	}
	if ev.row.eventFlags&smartEventFlagNotRepeatable != 0 && ev.runOnce {
		return
	}
	// WHILE_CHARMED gate (SmartScript.cpp:2890): the event fires while
	// charmed only when the flag is set. Go tracks charm as CharmerGUID
	// (the CallForHelp gate reads the same field).
	if ev.row.eventFlags&smartEventFlagWhileCharmed == 0 && motion.CharmerGUID != 0 {
		return
	}
	// Difficulty gate (SmartScript::FillScript, SmartScript.cpp:3797-3808):
	// a flagged event loads only on a dungeon map whose spawn mode
	// matches (1 << (spawnMode+1)) & flags; on a world map it never loads.
	// TDB 26091: 182 AGGRO rows carry difficulty flags.
	if ev.row.eventFlags&smartEventFlagDifficultyAll != 0 {
		if !dungeonMapID(s, motion.Map) && !raidMapID(s, motion.Map) {
			return
		}
		if (uint32(1)<<(s.smartSpawnMode(invokerGUID)+1))&ev.row.eventFlags == 0 {
			return
		}
	}
	switch ev.row.eventType {
	case smartEventLink:
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventDataSet:
		// SMART_EVENT_DATA_SET (SmartScript.cpp:3286-3292): the event
		// fires only for its (field, value) pair, then re-arms its
		// cooldown (RecalcTimer); the tick re-activates it on expiry.
		if ev.row.eventParam1 != var0 || ev.row.eventParam2 != var1 {
			return
		}
		s.smartRecalcTimer(ev, ev.row.eventParam3, ev.row.eventParam4)
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventUpdate, smartEventUpdateOOC, smartEventUpdateIC:
		s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, invokerGUID, var0, var1)
	case smartEventHealthPct, smartEventTargetHealthPc:
		if motion == nil || !motion.InCombat || motion.MaxHealth == 0 {
			return
		}
		health := motion.Health
		victimGUID := motion.TargetGUID
		if ev.row.eventType == smartEventTargetHealthPc {
			victim := s.findCreatureMotion(motion.Map, motion.InstanceID, victimGUID)
			if victim == nil {
				if ps := s.findSessionByGUID(victimGUID); ps != nil && ps.player != nil {
					health = ps.player.Health
					if ps.player.MaxHealth == 0 || uint32(100*uint64(health)/uint64(ps.player.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(ps.player.MaxHealth)) < ev.row.eventParam1 {
						return
					}
					s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, victimGUID, var0, var1)
					return
				}
				return
			}
			if victim.MaxHealth == 0 {
				return
			}
			health = victim.Health
			if uint32(100*uint64(health)/uint64(victim.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(victim.MaxHealth)) < ev.row.eventParam1 {
				return
			}
			s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, victimGUID, var0, var1)
			return
		}
		if uint32(100*uint64(health)/uint64(motion.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(motion.MaxHealth)) < ev.row.eventParam1 {
			return
		}
		s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, invokerGUID, var0, var1)
	case smartEventManaPct, smartEventTargetManaPct:
		// Mana pct needs the live power value, which the motion record does
		// not carry for creatures (powers live in the aura pipeline);
		// residual — the timer still re-arms below via the default arm.
		s.smartRecalcTimer(ev, ev.row.eventParam3, ev.row.eventParam4)
		return
	case smartEventRange:
		if motion == nil || !motion.InCombat || motion.TargetGUID == 0 {
			return
		}
		s.smartRecalcTimer(ev, 500, 500)
		return
	case smartEventTimedEventTriggered:
		// SMART_EVENT_TIMED_EVENT_TRIGGERED (SmartScript.cpp:3303-3307):
		// the event fires only for its timed-event id, then runs its
		// action with no timer re-arm (a stored event's own repeat
		// window re-arms the stored holder, not this row).
		if ev.row.eventParam1 != var0 {
			return
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventAggro, smartEventKill, smartEventDeath, smartEventEvade,
		smartEventRespawn, smartEventReachedHome, smartEventReset,
		smartEventAIInit, smartEventJustCreated,
		smartEventTextOver, smartEventPhaseChange:
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	default:
		// Timed/conditioned types with no Go condition evaluation
		// (SPELLHIT, DAMAGED, FRIENDLY_*, HAS_AURA, ...) never fire from
		// the direct path; their timer arm re-arms in smartUpdateTimers.
		return
	}
}

// smartProcessTimedAction is SmartScript::ProcessTimedAction (:2385-2395):
// the condition check runs first (a failing event re-arms on a short
// window instead of firing); on pass, re-arm on the repeat window, then
// run the action.
func (s *Server) smartProcessTimedAction(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, min, max uint32, invokerGUID uint64, var0, var1 uint32) {
	if s.smartHasConditions(ev.row.entryOrGUID, ev.row.id) {
		if min > 5000 {
			min = 5000
		}
		s.smartRecalcTimer(ev, min, min)
		return
	}
	s.smartRecalcTimer(ev, min, max)
	s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
}

// smartUpdateTimers is SmartScript::OnUpdate (:3676-...) + UpdateTimer
// (:3499-3597) for the creature slice: dead/evading creatures run no
// timers, UPDATE_IC needs combat, UPDATE_OOC needs out-of-combat, and only
// the timed event types fire from here.
func (s *Server) smartUpdateTimers(ctx context.Context, motion *creatureMotion, now time.Time) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	if motion.Evading {
		// SmartScript::OnUpdate's evade branch (SmartScript.cpp:3685-3696):
		// a finished list (no enabled event left) clears so a later
		// CALL_TIMED_ACTIONLIST can install; a running list survives the
		// evade and resumes on return.
		if len(st.timedList) > 0 {
			running := false
			for _, ev := range st.timedList {
				if ev.enableTimed {
					running = true
					break
				}
			}
			if !running {
				st.timedList = nil
			}
		}
		return
	}
	diff := uint32(0)
	if !st.lastTick.IsZero() {
		d := now.Sub(st.lastTick)
		if d > 0 {
			diff = uint32(d.Milliseconds())
			if diff > 60000 {
				diff = 60000
			}
		}
	}
	st.lastTick = now
	if st.useTextTimer {
		if st.textTimer < diff {
			textID, entry := st.lastTextID, st.talkerEntry
			st.lastTextID, st.talkerEntry, st.textTimer, st.useTextTimer = 0, 0, 0, false
			s.smartProcessEvents(ctx, motion, smartEventTextOver, 0, textID, entry)
		} else {
			st.textTimer -= diff
		}
	}
	for _, ev := range st.events {
		if ev.row.eventType == smartEventLink {
			continue
		}
		if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
			continue
		}
		if ev.row.eventType == smartEventUpdateIC && !motion.InCombat {
			continue
		}
		if ev.row.eventType == smartEventUpdateOOC && motion.InCombat {
			continue
		}
		if ev.timer < diff {
			ev.active = true
			switch ev.row.eventType {
			case smartEventUpdate, smartEventUpdateOOC, smartEventUpdateIC,
				smartEventHealthPct, smartEventTargetHealthPc, smartEventManaPct,
				smartEventTargetManaPct, smartEventRange:
				s.smartProcessEvent(ctx, motion, st, ev, 0, 0, 0)
			}
		} else {
			ev.timer -= diff
		}
	}
	s.smartUpdateStoredEvents(ctx, motion, st, diff)
	s.smartUpdateTimedActionList(ctx, motion, st, diff)
	// mRemIDs drain (SmartScript.cpp:3741-3746): after the stored-event
	// and timed-list legs, each queued id drops its stored event (first
	// match only, RemoveStoredEvent :3615-3623), then the queue clears.
	if len(st.remTimedEventIDs) > 0 {
		for _, id := range st.remTimedEventIDs {
			s.smartRemoveStoredEvent(st, id)
		}
		st.remTimedEventIDs = nil
	}
}

// smartCreateTimedEvent is the SMART_ACTION_CREATE_TIMED_EVENT arm
// (SmartScript.cpp:1586-1613): the synthetic holder is an UPDATE event
// with the creator's chance (0 defaults to 100), the initial window in
// event_param1/2 and the repeat window in param3/4, NOT_REPEATABLE when
// neither repeat bound is set, the creator's target, and the
// TRIGGER_TIMED_EVENT action for the id; InitTimer rolls the initial
// window and the holder joins mStoredEvents. The row id is the timed
// event's id (C++ ev.event_id), so the ProcessTimedAction conditions
// check and RemoveStoredEvent both key on it, exactly as in C++.
func (s *Server) smartCreateTimedEvent(st *smartScriptState, ev *smartScriptEvent) {
	if st == nil || ev == nil || ev.row == nil {
		return
	}
	chance := ev.row.actionParam6
	if chance == 0 {
		chance = 100
	}
	flags := uint32(0)
	if ev.row.actionParam4 == 0 && ev.row.actionParam5 == 0 {
		flags = smartEventFlagNotRepeatable
	}
	row := &smartScriptRow{
		// entryOrGUID 0: C++ builds the holder from a default
		// SmartScriptHolder (SmartScriptMgr.h:1551, entryOrGuid(0)),
		// so the holder's condition check keys on (0, id) and never
		// inherits the creator row's conditions.
		entryOrGUID:  0,
		sourceType:   ev.row.sourceType,
		id:           ev.row.actionParam1,
		eventType:    smartEventUpdate,
		eventChance:  chance,
		eventFlags:   flags,
		eventParam1:  ev.row.actionParam2,
		eventParam2:  ev.row.actionParam3,
		eventParam3:  ev.row.actionParam4,
		eventParam4:  ev.row.actionParam5,
		actionType:   smartActionTriggerTimedEvent,
		actionParam1: ev.row.actionParam1,
		targetType:   ev.row.targetType,
		targetParam1: ev.row.targetParam1,
		targetParam2: ev.row.targetParam2,
		targetParam3: ev.row.targetParam3,
		targetParam4: ev.row.targetParam4,
		targetX:      ev.row.targetX,
		targetY:      ev.row.targetY,
		targetZ:      ev.row.targetZ,
		targetO:      ev.row.targetO,
	}
	sev := &smartScriptEvent{row: row, timedEventID: ev.row.actionParam1}
	s.smartInitTimer(sev)
	st.storedEvents = append(st.storedEvents, sev)
}

// smartRemoveStoredEvent is SmartScript::RemoveStoredEvent
// (SmartScript.cpp:3615-3623): erase the first stored event whose
// event_id matches, then stop.
func (s *Server) smartRemoveStoredEvent(st *smartScriptState, id uint32) {
	if st == nil {
		return
	}
	for i, sev := range st.storedEvents {
		if sev.timedEventID == id {
			st.storedEvents = append(st.storedEvents[:i], st.storedEvents[i+1:]...)
			return
		}
	}
}

// smartUpdateStoredEvents is the OnUpdate stored-events leg
// (SmartScript.cpp:3710-3717): each stored holder ticks like a main-loop
// UPDATE event — phase gate, timer expiry sets active and runs
// ProcessEvent (whose UPDATE arm re-arms on the repeat window via
// ProcessTimedAction, then fires the TRIGGER_TIMED_EVENT action).
// Stored events never tick while evading: the caller returns before this
// point on the evade branch, matching OnUpdate's early return.
func (s *Server) smartUpdateStoredEvents(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if st == nil || len(st.storedEvents) == 0 {
		return
	}
	for _, sev := range st.storedEvents {
		if sev.row.eventPhaseMask != 0 && !st.inPhase(sev.row.eventPhaseMask) {
			continue
		}
		if sev.timer < diff {
			sev.active = true
			s.smartProcessEvent(ctx, motion, st, sev, 0, 0, 0)
		} else {
			sev.timer -= diff
		}
	}
}

// smartUpdateTimedActionList is SmartScript::OnUpdate's timed-action-list
// leg (SmartScript.cpp:3713-3739) plus the UpdateTimer timed branch
// (:3541-3570): only enabled list events tick; the IC/OOC gates hold the
// timer while the combat state mismatches (the same gate as the main
// loop, mirroring UpdateTimer's early return); on expiry the event fires
// with the list invoker, then disables itself and enables the next
// higher-id event — the advance is unconditional, matching C++ (a gated
// action still moves the list on). When no event is enabled the list is
// finished and clears. The UpdateTimer CAST-delay and root arms have no
// Go twin in the main loop either and stay out here too.
func (s *Server) smartUpdateTimedActionList(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if len(st.timedList) == 0 {
		return
	}
	st.processingTimedList = true
	needCleanup := true
	for _, ev := range st.timedList {
		if !ev.enableTimed {
			continue
		}
		needCleanup = false
		// UpdateTimer's phase gate (:3502-3503) sits ahead of the timer:
		// a phase-mismatched list event holds its timer AND its place
		// in the list — unlike the gates inside ProcessEvent (runOnce,
		// WHILE_CHARMED, difficulty), which still advance the list.
		if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
			continue
		}
		if ev.row.eventType == smartEventUpdateIC && !motion.InCombat {
			continue
		}
		if ev.row.eventType == smartEventUpdateOOC && motion.InCombat {
			continue
		}
		if ev.timer < diff {
			ev.active = true
			s.smartProcessEvent(ctx, motion, st, ev, st.timedListInvoker, 0, 0)
			ev.enableTimed = false
			for _, next := range st.timedList {
				if next.row.id > ev.row.id {
					next.enableTimed = true
					break
				}
			}
		} else {
			ev.timer -= diff
		}
	}
	st.processingTimedList = false
	if needCleanup {
		st.timedList = nil
	}
}

// smartStateForTimedList attaches the SmartScript state for a timed-list
// install on a creature with no creature-source rows: C++ installs the
// list on any SmartAI creature (SetTimedActionList needs only IsSmart),
// so a row-less SmartAI still runs its list. No init events fire —
// there are no rows to fire them from.
func (s *Server) smartStateForTimedList(ctx context.Context, motion *creatureMotion) *smartScriptState {
	if st := s.smartStateFor(ctx, motion); st != nil {
		return st
	}
	if s == nil || motion == nil || motion.AIName != "SmartAI" {
		return nil
	}
	st := &smartScriptState{phase: 0, defaultFaction: motion.Faction}
	motion.smart = st
	return st
}

// smartSetTimedActionList is SmartScript::SetTimedActionList
// (SmartScript.cpp:3950-3977): a running list blocks a new install
// unless allowOverride is set, and a list started from inside a list
// event's own action is refused (the isProcessingTimedActionList guard
// — overwriting mid-iteration would invalidate the holder the caller
// is running from). The install clears first, so an unknown id empties
// the running list and stops. Each row is copied before its event type
// is forced — the stored rows are shared — and the timer type picks the
// forced type: 0 OOC, 1 IC, anything else UPDATE. Only the first event
// is enabled; InitTimer rolls its initial window (event_param1/2), the
// repeat window (param3/4) re-arms on fire.
func (s *Server) smartSetTimedActionList(ctx context.Context, motion *creatureMotion, listID, timerType uint32, allowOverride bool, invokerGUID uint64) {
	st := s.smartStateForTimedList(ctx, motion)
	if st == nil {
		return
	}
	if st.processingTimedList {
		return
	}
	if !allowOverride && len(st.timedList) > 0 {
		return
	}
	rows := s.smartTimedActionListRows(int32(listID))
	st.timedList = nil
	if len(rows) == 0 {
		return
	}
	st.timedListInvoker = invokerGUID
	forced := smartEventUpdate
	switch timerType {
	case 0:
		forced = smartEventUpdateOOC
	case 1:
		forced = smartEventUpdateIC
	}
	for i, row := range rows {
		rc := *row
		rc.eventType = forced
		ev := &smartScriptEvent{row: &rc, enableTimed: i == 0}
		s.smartInitTimer(ev)
		st.timedList = append(st.timedList, ev)
	}
}

// smartProcessAction is SmartScript::ProcessAction (:255-2379), AGGRO-used
// action set. Chance: a 0 chance always fires (SmartScript.cpp:256-260);
// runOnce marks the event for the NOT_REPEATABLE gate. The link step
// (:2375-2381) re-enters ProcessEvent for the linked event; C++ caps only
// ProcessEventsFor nesting (MAX_NESTED_EVENTS), not link chains, so a
// link cycle would recurse forever there — Go caps the chain at the same
// 10 as a hardening deviation (no such cycle exists in TDB 26091).
func (s *Server) smartProcessAction(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64, var0, var1 uint32) {
	if ev.row.eventType != smartEventLink && ev.row.eventChance < 100 && ev.row.eventChance != 0 {
		if uint32(rand.Intn(100)) >= ev.row.eventChance {
			return
		}
	}
	ev.runOnce = true
	if invokerGUID != 0 {
		st.lastInvoker = invokerGUID
	}
	switch ev.row.actionType {
	case smartActionTalk:
		s.smartActionTalk(ctx, motion, st, ev, invokerGUID)
	case smartActionSetFaction:
		faction := ev.row.actionParam1
		if faction == 0 {
			faction = st.defaultFaction
		}
		s.motionMu.Lock()
		motion.Faction = faction
		motion.FactionKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldFaction: faction})
	case smartActionSound:
		s.smartActionSound(ctx, motion, st, ev, invokerGUID)
	case smartActionPlayEmote:
		// SMART_ACTION_PLAY_EMOTE (SmartScript.cpp:325-337): the emote
		// plays on every target unit, not just the owner — the self-only
		// broadcast was the slice-1 partial (TDB 26091: the 1 live row
		// targets SELF, so the old form was faithful for it). Per-target
		// coords feed the visible set, falling back to the owner's when
		// the target has no live motion or session.
		for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
			mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
			if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid); tm != nil {
				s.motionMu.Lock()
				mapID, instanceID, x, y, z, phase = tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask
				s.motionMu.Unlock()
			} else if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
				mapID, instanceID, x, y, z = sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z
				phase = sess.currentPlayerPhaseMask()
			}
			packet := protocol.NewBuffer(12)
			packet.WriteU32(ev.row.actionParam1)
			packet.WriteU64(tgt.guid)
			s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), mapID, instanceID, x, y, z, phase)
		}
	case smartActionRandomEmote:
		s.smartActionRandomEmote(ctx, motion, st, ev, invokerGUID)
	case smartActionFailQuest:
		s.smartActionFailQuest(ctx, motion, st, ev, invokerGUID)
	case smartActionOfferQuest:
		s.smartActionOfferQuest(ctx, motion, st, ev, invokerGUID)
	case smartActionCallAreaExploredOrEventHappens:
		s.smartActionCallAreaExploredOrEventHappens(ctx, motion, st, ev, invokerGUID)
	case smartActionCallGroupEventHappens:
		s.smartActionCallGroupEventHappens(ctx, motion, st, ev, invokerGUID)
	case smartActionCombatStop:
		s.smartActionCombatStop(ctx, motion, st, ev, invokerGUID)
	case smartActionActivateGObject:
		s.smartActionActivateGObject(ctx, motion, st, ev, invokerGUID)
	case smartActionSetReactState:
		s.smartActionSetReactState(ctx, motion, st, ev, invokerGUID)
	case smartActionSetEmoteState:
		s.smartActionSetEmoteState(ctx, motion, st, ev, invokerGUID)
	case smartActionSetVisibility:
		s.smartActionSetVisibility(ctx, motion, st, ev, invokerGUID)
	case smartActionInterruptSpell:
		s.smartActionInterruptSpell(ctx, motion, st, ev, invokerGUID)
	case smartActionSetRoot:
		s.smartActionSetRoot(ctx, motion, st, ev, invokerGUID)
	case smartActionCast:
		s.smartActionCast(ctx, motion, st, ev, invokerGUID)
	case smartActionSummonCreature:
		s.smartActionSummonCreature(ctx, motion, st, ev, invokerGUID)
	case smartActionThreatSinglePct:
		s.smartActionThreatSinglePct(ctx, motion, st, ev, invokerGUID)
	case smartActionThreatAllPct:
		s.smartActionThreatAllPct(ctx, motion, st, ev, invokerGUID)
	case smartActionAutoAttack:
		motion.NoMeleeAttack = ev.row.actionParam1 == 0
	case smartActionAllowCombatMovement:
		// SMART_ACTION_ALLOW_COMBAT_MOVEMENT (SmartScript.cpp:788-798):
		// me-only (not targets), gated on IsSmart — the dispatch only
		// runs for smart-script creatures. SmartAI::SetCombatMove
		// (SmartAI.cpp:787-815) flips _canCombatMove; the Go twin is
		// the inverted NoCombatMovement flag, honored by the combat-tick
		// chase gate. The engaged legs ride the next tick: on=true
		// re-chases via that gate (which carries the passive arm), and
		// on=false halts the active chase now (the Remove(chase)
		// twin) instead of letting the spline run to MoveEnds.
		move := ev.row.actionParam1 != 0
		s.motionMu.Lock()
		motion.NoCombatMovement = !move
		if !move && motion.InCombat {
			motion.Moving = false
		}
		s.motionMu.Unlock()
	case smartActionSetEventPhase:
		s.smartSetEventPhase(ctx, motion, st, ev.row.actionParam1, invokerGUID)
	case smartActionIncEventPhase:
		// SMART_ACTION_INC_EVENT_PHASE (SmartScript.cpp:809-819): IncPhase
		// then DecPhase, each through SetPhase, so a row with only one
		// nonzero leg still passes through the other as a no-op. IncPhase
		// clamps at PHASE_12 (SmartScript.cpp:3996-4000); DecPhase clamps
		// at 0 (SmartScript.cpp:4002-4008); each leg fires the
		// phase-change event only if it moved the phase.
		phase := st.phase
		if ev.row.actionParam1 > smartEventPhase12-phase {
			phase = smartEventPhase12
		} else {
			phase += ev.row.actionParam1
		}
		s.smartSetEventPhase(ctx, motion, st, phase, invokerGUID)
		if ev.row.actionParam2 >= st.phase {
			phase = 0
		} else {
			phase = st.phase - ev.row.actionParam2
		}
		s.smartSetEventPhase(ctx, motion, st, phase, invokerGUID)
	case smartActionFleeForAssist:
		// SMART_ACTION_FLEE_FOR_ASSIST (SmartScript.cpp:837-851):
		// DoFleeToGetAssistance on me. The withEmote leg (param1, 683
		// of 698 AGGRO rows) sends BROADCAST_TEXT_FLEE_FOR_ASSIST via
		// CreatureTextMgr — Go has no BroadcastText twin, so the emote
		// stays out, the CALL_FOR_HELP precedent.
		s.creatureFleeToGetAssistance(ctx, motion)
	case smartActionEvade:
		s.smartActionEvade(ctx, motion, st, ev, invokerGUID)
	case smartActionRandomPhase:
		// SMART_ACTION_RANDOM_PHASE (SmartScript.cpp:939-953): pick one of
		// the nonzero phases (params 1-6). All-zero rows are rejected at
		// load (SmartScriptMgr.cpp:1255-1267), so the empty pick is a
		// guard only — C++ would select from an empty vector there.
		phases := make([]uint32, 0, 6)
		for _, p := range [6]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
			if p != 0 {
				phases = append(phases, p)
			}
		}
		if len(phases) > 0 {
			s.smartSetEventPhase(ctx, motion, st, phases[rand.Intn(len(phases))], invokerGUID)
		}
	case smartActionRandomPhaseRange:
		// SMART_ACTION_RANDOM_PHASE_RANGE (SmartScript.cpp:954-965): urand
		// of phaseMin..phaseMax (params 1-2, inclusive). C++ urand ASSERTs
		// max >= min (Random.cpp:42); the clamp keeps an unvalidated row
		// defined instead of panicking rand.Intn.
		phaseMin, phaseMax := ev.row.actionParam1, ev.row.actionParam2
		if phaseMax < phaseMin {
			phaseMax = phaseMin
		}
		s.smartSetEventPhase(ctx, motion, st, phaseMin+uint32(rand.Intn(int(phaseMax-phaseMin+1))), invokerGUID)
	case smartActionAttackStart:
		// SMART_ACTION_ATTACK_START (SmartScript.cpp:1238-1249): engage a
		// random target. The engage runs the AttackStart funnel
		// (triggerCreatureAggro): its Evading skip is C++ Unit::Attack's
		// IsInEvadeMode gate (Unit.cpp: "creatures cannot attack while
		// evading"), not a deviation, and the passive skip does not fire
		// for fromAttack calls — C++ SmartAI::AttackStart has no
		// react-state gate on this path (SmartAI.cpp:593-608).
		targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
		if len(targets) > 0 {
			s.triggerCreatureAggro(ctx, motion.GUID, targets[rand.Intn(len(targets))].guid, true)
		}
	case smartActionRemoveAurasFromSpell:
		s.smartActionRemoveAuras(ctx, motion, st, ev, invokerGUID)
	case smartActionSetInstData:
		s.smartActionSetInstData(ctx, motion, ev)
	case smartActionCallForHelp:
		s.callCreatureForHelp(ctx, motion, float32(ev.row.actionParam1))
		// withEmote (param2, 2 AGGRO rows): C++ sends the
		// BROADCAST_TEXT_CALL_FOR_HELP monster emote
		// (SmartScript.cpp:1080-1091); Go has no BroadcastText twin,
		// so the emote leg stays out — the engage itself ports.
	case smartActionSetData:
		s.smartActionSetData(ctx, motion, st, ev, invokerGUID)
	case smartActionUpdateTemplate:
		s.smartActionUpdateTemplate(ctx, motion, st, ev, invokerGUID)
	case smartActionForceDespawn:
		s.smartActionForceDespawn(ctx, motion, st, ev, invokerGUID)
	case smartActionSetInvincibilityHPLevel:
		s.smartActionSetInvincibilityHPLevel(ctx, motion, st, ev, invokerGUID)
	case smartActionMountToEntryOrModel:
		s.smartActionMountToEntryOrModel(ctx, motion, st, ev, invokerGUID)
	case smartActionMorphToEntryOrModel:
		s.smartActionMorphToEntryOrModel(ctx, motion, st, ev, invokerGUID)
	case smartActionRespawnBySpawnID:
		s.smartActionRespawnBySpawnID(ctx, motion, ev)
	case smartActionStoreTargetList:
		targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
		guids := make([]uint64, 0, len(targets))
		for _, tgt := range targets {
			guids = append(guids, tgt.guid)
		}
		if st.storedTargets == nil {
			st.storedTargets = make(map[uint32][]uint64)
		}
		st.storedTargets[ev.row.actionParam1] = guids
	case smartActionCreateTimedEvent:
		// SMART_ACTION_CREATE_TIMED_EVENT (SmartScript.cpp:1586-1613):
		// builds a synthetic UPDATE holder — chance (0 defaults to
		// 100), initial window (min/max), repeat window, NOT_REPEATABLE
		// when no repeat window — whose action is TRIGGER_TIMED_EVENT
		// for the id, then stores it in mStoredEvents. The target
		// rides along (ev.target = e.target), so the trigger resolves
		// the creator's targets. InitTimer rolls the initial window.
		s.smartCreateTimedEvent(st, ev)
	case smartActionTriggerTimedEvent:
		// SMART_ACTION_TRIGGER_TIMED_EVENT (SmartScript.cpp:1615-1620):
		// fires every TIMED_EVENT_TRIGGERED row for the id (invoker
		// nil in C++), then queues the id for removal when the firing
		// event is NOT_REPEATABLE — for a stored event's own fire
		// that is the stored holder's flag, which is what C++ reads.
		s.smartProcessEvents(ctx, motion, smartEventTimedEventTriggered, 0, ev.row.actionParam1, 0)
		if ev.row.eventFlags&smartEventFlagNotRepeatable != 0 {
			st.remTimedEventIDs = append(st.remTimedEventIDs, ev.row.actionParam1)
		}
	case smartActionRemoveTimedEvent:
		// SMART_ACTION_REMOVE_TIMED_EVENT (SmartScript.cpp:1622-1623):
		// queues the id for removal; the drain runs after the
		// timed-list leg, matching OnUpdate's order.
		st.remTimedEventIDs = append(st.remTimedEventIDs, ev.row.actionParam1)
	case smartActionTriggerRandomTimedEvent:
		// SMART_ACTION_TRIGGER_RANDOM_TIMED_EVENT (SmartScript.cpp:
		// 2295-2298): a random id in [minId, maxId] fires its
		// TIMED_EVENT_TRIGGERED rows; the stored holder keeps ticking
		// on its own repeat window — no removal queued.
		minID, maxID := ev.row.actionParam1, ev.row.actionParam2
		if maxID < minID {
			maxID = minID
		}
		id := minID
		if maxID > minID {
			id = minID + uint32(rand.Int63n(int64(maxID-minID+1)))
		}
		s.smartProcessEvents(ctx, motion, smartEventTimedEventTriggered, 0, id, 0)
	case smartActionCallTimedActionList, smartActionCallRandomTimedActionList, smartActionCallRandomRangeTimedActionList:
		s.smartActionCallTimedActionList(ctx, motion, st, ev, invokerGUID)
	case smartActionSetNPCFlag, smartActionAddNPCFlag, smartActionRemoveNPCFlag:
		s.smartActionNPCFlag(ctx, motion, st, ev, invokerGUID)
	case smartActionRemoveUnitFieldBytes1:
		s.smartActionRemoveUnitFieldBytes1(ctx, motion, st, ev, invokerGUID)
	default:
		// Residual: the remaining AGGRO-row action (146) has no Go
		// twin. 146 has no enum value in this C++ revision
		// (SMART_ACTION_END = 140, SmartScriptMgr.h:600), so its 70
		// TDB rows hit C++'s log-and-skip default exactly as they hit
		// this one (SmartScript.cpp:2370-2372). The event still
		// consumed its chance roll and link below.
	}
	if ev.row.link != 0 && ev.row.link != ev.row.id {
		if st.linkDepth >= smartMaxNestedEvents {
			return
		}
		st.linkDepth++
		for _, linked := range st.events {
			if linked.row.id == ev.row.link && linked.row.eventType == smartEventLink {
				s.smartProcessEvent(ctx, motion, st, linked, invokerGUID, var0, var1)
				break
			}
		}
		st.linkDepth--
	}
}

// smartSetEventPhase is SmartScript::SetPhase (SmartScript.cpp:4010-4018):
// store the phase number and fire SMART_EVENT_EVENT_PHASE_CHANGE only when
// it moved.
func (s *Server) smartSetEventPhase(ctx context.Context, motion *creatureMotion, st *smartScriptState, phase uint32, invokerGUID uint64) {
	oldPhase := st.phase
	st.phase = phase
	if oldPhase != st.phase {
		s.smartProcessEvents(ctx, motion, smartEventPhaseChange, invokerGUID, 0, 0)
	}
}

// smartActionCast is SMART_ACTION_CAST (SmartScript.cpp:543-599). Param
// mapping (SmartScriptMgr.h cast struct): param1 spell, param2 cast flags,
// param3 trigger flags, param4 targets limit. TRIGGERED (0x02) casts ride
// castCreatureSpellTriggered; a nonzero triggerFlags value collapses to
// that same full-mask trigger — Go has no per-flag triggered variant.
// AURA_NOT_PRESENT (0x20, 1 AGGRO row) skips targets that already carry
// the aura. INTERRUPT_PREVIOUS (0x01, 0 AGGRO rows) interrupts first.
// COMBAT_MOVE (0x40, 4 rows) stays out: C++ keys it on the cast result
// (SetCombatMove(spellCastFailed, true)) and the Go cast helpers return
// no result to key it on. The all-failed retry (RaisePriority) stays out
// for the same reason. The stale FORCE_CAST flag (0x04, 1 row) is a
// no-op in C++ too (commented out of SmartCastFlags).
func (s *Server) smartActionCast(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	if limit := ev.row.actionParam4; limit > 0 && uint32(len(targets)) > limit {
		rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
		targets = targets[:limit]
	}
	castFlags := ev.row.actionParam2
	if castFlags&smartCastInterruptPrevious != 0 {
		for _, arm := range s.interruptCreatureNonMeleeCasts(motion) {
			s.sendCreatureInterruptPackets(motion, arm)
		}
	}
	triggered := castFlags&smartCastTriggered != 0
	for _, tgt := range targets {
		if castFlags&smartCastAuraNotPresent != 0 && s.creatureSpellTargetHasAura(motion, tgt.guid, ev.row.actionParam1) {
			continue
		}
		if triggered {
			s.castCreatureSpellTriggered(ctx, motion, ev.row.actionParam1, tgt.guid)
		} else {
			s.castCreatureSpell(ctx, motion, ev.row.actionParam1, tgt.guid)
		}
	}
}

// smartActionSummonCreature is SMART_ACTION_SUMMON_CREATURE
// (SmartScript.cpp:1251-1277). Param mapping (SmartScriptMgr.h:471:
// CreatureID, summonType, duration in ms, attackInvoker): param1
// entry, param2 C++ TempSummonType (1-8), param3 duration ms, param4
// attackInvoker. The type translates down by one into the bridge's
// 0-based scheme (the SummonCreatureGroup precedent,
// lua_creature_events.go:4841); rows outside 1..8 are skipped, the
// C++ `> TEMPSUMMON_MANUAL_DESPAWN` skip twin — C++ itself would
// summon then instantly UnSummon on the Update default arm
// (TemporarySummon.cpp:168-170), so the skip loses at most one tick
// of a creature no faithful row produces. For each target the summon
// lands at the target position plus the row's target_x/y/z/o offset;
// a POSITION target resolves to no unit (GetTargets returns nothing,
// SmartScript.cpp:2864-2867), so that leg summons once at the row's
// absolute target position. attackInvoker runs the EngageWithTarget
// funnel (triggerCreatureAggro) — the C++ AttackStart twin — on the
// target unit, or on the invoker (falling back to the last invoker,
// the smartTargets convention) for the POSITION leg; the funnel's
// Evading skip is C++ Unit::Attack's IsInEvadeMode gate and the passive
// skip does not fire for fromAttack calls, so neither leg deviates on
// this path (see the smartActionAttackStart comment).
func (s *Server) smartActionSummonCreature(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	summonType := ev.row.actionParam2
	if summonType < 1 || summonType > 8 {
		return
	}
	origin := s.summonOriginFromMotion(motion)
	attackInvoker := ev.row.actionParam4 != 0
	summonAt := func(x, y, z, o float32, attackGUID uint64) {
		summoned := s.summonTempCreature(ctx, origin, ev.row.actionParam1, x, y, z, o, uint8(summonType-1), ev.row.actionParam3)
		if summoned == nil || !attackInvoker || attackGUID == 0 {
			return
		}
		s.triggerCreatureAggro(ctx, summoned.GUID, attackGUID, true)
	}
	if ev.row.targetType == smartTargetPosition {
		invoker := invokerGUID
		if invoker == 0 {
			invoker = st.lastInvoker
		}
		summonAt(ev.row.targetX, ev.row.targetY, ev.row.targetZ, ev.row.targetO, invoker)
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		x, y, z, o, ok := s.smartTargetPosition(motion, tgt)
		if !ok {
			continue
		}
		summonAt(x+ev.row.targetX, y+ev.row.targetY, z+ev.row.targetZ, o+ev.row.targetO, tgt.guid)
	}
}

// smartTargetPosition resolves one SmartAI target to its world position:
// a player reads the session state, a creature its motion.
func (s *Server) smartTargetPosition(motion *creatureMotion, tgt smartTarget) (x, y, z, o float32, ok bool) {
	if tgt.isPlayer {
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || sess.player == nil {
			return 0, 0, 0, 0, false
		}
		return sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, true
	}
	tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
	if tm == nil {
		return 0, 0, 0, 0, false
	}
	return tm.X, tm.Y, tm.Z, tm.Orientation, true
}

// smartActionTalk is SMART_ACTION_TALK (SmartScript.cpp:260-294): the
// talker is the first non-pet creature target (or self when the target is
// a player, when useTalkTarget is set, or when the target type is NONE
// with no targets); the talk target is that creature/player, falling back
// to the last invoker. The talk target rides broadcastCreatureTalk's
// whisper slot, which only whispers for whisper-type rows — for SAY/YELL
// rows it is ignored, matching C++ SendChat. The duration arms
// SMART_EVENT_TEXT_OVER from the tick (a 0 duration fires on the next
// tick, like C++'s mTextTimer = 0 arm).
func (s *Server) smartActionTalk(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	talker := (*creatureMotion)(nil)
	talkTargetGUID := uint64(0)
	if ev.row.targetType == smartTargetNone {
		talker = motion
	}
	for _, tgt := range targets {
		if tgt.isPlayer {
			talker = motion
			talkTargetGUID = tgt.guid
			break
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || motionIsPet(tm) {
			continue
		}
		if ev.row.actionParam3 != 0 {
			talker = motion
			talkTargetGUID = tgt.guid
		} else {
			talker = tm
		}
		break
	}
	if talker == nil {
		return
	}
	if talkTargetGUID == 0 {
		talkTargetGUID = st.lastInvoker
	}
	st.talkerEntry = talker.Entry
	st.lastTextID = ev.row.actionParam1
	st.textTimer = ev.row.actionParam2
	st.useTextTimer = true
	s.broadcastCreatureTalk(ctx, talker.Map, talker.InstanceID, talker.GUID, talker.Entry, talker.Name, uint8(ev.row.actionParam1), talkTargetGUID)
}

// smartActionSound is SMART_ACTION_SOUND (SmartScript.cpp:346-361):
// param1 sound, param2 onlySelf, param3 distance (1 = distance sound).
// onlySelf sends to the target player alone; otherwise the sound goes to
// the visibility set around the target (Unit::PlayDirectSound). The
// distance variant has no Go packet twin — it rides the same send, so
// its 2 AGGRO rows play at full volume instead of distance-attenuated.
func (s *Server) smartActionSound(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	onlySelf := ev.row.actionParam2 != 0
	for _, tgt := range targets {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(ev.row.actionParam1)
		payload := packet.Bytes()
		if tgt.isPlayer {
			sess := s.findSessionByGUID(tgt.guid)
			if sess == nil || sess.player == nil {
				continue
			}
			if onlySelf {
				_ = sess.write(uint16(protocol.OpcodeSMSG_PLAY_SOUND), payload, true)
				continue
			}
			s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_PLAY_SOUND), payload, sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z, motion.PhaseMask)
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_PLAY_SOUND), payload, tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask)
	}
}

// smartActionRemoveAuras is SMART_ACTION_REMOVEAURASFROMSPELL
// (SmartScript.cpp:882-911): param1 spell (0 removes all auras), param2
// charges (0 removes the aura outright), param3 onlyOwnedAuras. TDB
// 26091: all 38 AGGRO rows carry charges 0 and onlyOwnedAuras 0, so the
// plain remove legs are the live ones; the charge leg rides
// ModCharges(-charges, AURA_REMOVE_BY_EXPIRE) via modCreatureAuraStacks
// (creature) / repeated stack pops (player), and the spell-0 leg sweeps
// every live aura id. An onlyOwnedAuras row would need a caster-filtered
// removal twin Go does not have — it falls back to unfiltered, documented.
func (s *Server) smartActionRemoveAuras(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	spellID := ev.row.actionParam1
	charges := ev.row.actionParam2
	for _, tgt := range targets {
		if tgt.isPlayer {
			sess := s.findSessionByGUID(tgt.guid)
			if sess == nil {
				continue
			}
			if spellID == 0 {
				for {
					id := sess.smartFirstActiveAuraSpellID()
					if id == 0 {
						break
					}
					for !sess.removePlayerAuraFromStack(id) {
					}
				}
				continue
			}
			if charges > 0 {
				for i := uint32(0); i < charges; i++ {
					if sess.removePlayerAuraFromStack(spellID) {
						break
					}
				}
				continue
			}
			for !sess.removePlayerAuraFromStack(spellID) {
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		key := creatureAuraKey{Map: tm.Map, InstanceID: tm.InstanceID, GUID: tm.GUID}
		if spellID == 0 {
			s.auraMu.Lock()
			ids := make(map[uint32]struct{})
			for id := range s.activeCreatureAuras[key] {
				ids[id] = struct{}{}
			}
			for id := range s.creatureAuras[key] {
				ids[id] = struct{}{}
			}
			s.auraMu.Unlock()
			for id := range ids {
				s.removeCreatureAura(key, id)
			}
			continue
		}
		if charges > 0 {
			s.modCreatureAuraStacks(key, spellID, -int32(charges), auraRemoveByExpire, 0)
			continue
		}
		s.removeCreatureAura(key, spellID)
	}
}

// smartActionSetInstData is SMART_ACTION_SET_INST_DATA
// (SmartScript.cpp:998-1036): param1 field, param2 data, param3 type
// (0 = SetData, 1 = SetBossState). All 9 AGGRO rows are type 1 with data
// IN_PROGRESS (1), riding setInstanceBossState over the live instance
// boss-state store. Type 0 stays out: Go has no instance SetData store —
// instanceDataObject.SetData only fires a script event and stores
// nothing (instance_hooks.go:39-47).
func (s *Server) smartActionSetInstData(ctx context.Context, motion *creatureMotion, ev *smartScriptEvent) {
	if ev.row.actionParam3 != 1 {
		return
	}
	if !dungeonMapID(s, motion.Map) && !raidMapID(s, motion.Map) {
		return
	}
	s.setInstanceBossState(ctx, motion.Map, motion.InstanceID, ev.row.actionParam1, uint8(ev.row.actionParam2), 0, false)
}

// smartActionSetData is SMART_ACTION_SET_DATA (SmartScript.cpp:1173-1195):
// SmartAI targets fire their own SMART_EVENT_DATA_SET (SmartAI::SetData,
// SmartAI.cpp:718-721); any other creature target latches the pair into
// its ScriptData store (the map Lua creature SetData/GetData reads) —
// a Go-side latch with no C++ twin, since base CreatureAI::SetData is a
// no-op and only SmartAI overrides it. TDB 26091: 19 AGGRO rows, and none
// of their (field, value) pairs matches a DATA_SET event in the table, so
// the live effect is the latch.
func (s *Server) smartActionSetData(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	for _, tgt := range targets {
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if tm.AIName == "SmartAI" {
			s.smartProcessEvents(ctx, tm, smartEventDataSet, invokerGUID, ev.row.actionParam1, ev.row.actionParam2)
			continue
		}
		s.motionMu.Lock()
		if tm.ScriptData == nil {
			tm.ScriptData = make(map[uint32]uint32)
		}
		tm.ScriptData[ev.row.actionParam1] = ev.row.actionParam2
		s.motionMu.Unlock()
	}
}

// smartActionUpdateTemplate is SMART_ACTION_UPDATE_TEMPLATE
// (SmartScript.cpp:1055-1061): Creature::UpdateEntry(entry, nullptr,
// updateLevel != 0) on every creature target — the seatCreatureEntry
// twin. TDB 26091: 76 rows, all updateLevel=0 (5357->14604,
// 5736->5742/5739/5741/5743, 8519-8521->13279), so the level roll
// stays out and the creature keeps its level; the template's model,
// faction, flags and stats still re-seat.
func (s *Server) smartActionUpdateTemplate(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.seatCreatureEntry(ctx, tm, ev.row.actionParam1, ev.row.actionParam2 != 0)
	}
}

// smartActionMorphToEntryOrModel is SMART_ACTION_MORPH_TO_ENTRY_OR_MODEL
// (SmartScript.cpp:383-417): Creature::SetDisplayId on every creature
// target — the entry leg takes the template's ChooseDisplayId
// (ObjectMgr.cpp:1581-1593, the smartChooseDisplayID twin); a zero
// entry with a nonzero model sets the model id directly; both zero
// demorphs (DeMorph restores the native display). A missing template
// skips the target outright. A zero ChooseDisplayId (template with no
// valid model) is skipped like the Mount twin's guard — no TDB 26091
// row hits it (all 34 morph entries carry a model). TDB 26091: 140
// rows (37 entry, 99 model, 4 demorph).
func (s *Server) smartActionMorphToEntryOrModel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if ev.row.actionParam1 != 0 {
			displayID, ok := s.smartChooseDisplayID(ctx, ev.row.actionParam1)
			if !ok || displayID == 0 {
				continue
			}
			s.setMotionDisplayID(ctx, tm, displayID)
		} else if ev.row.actionParam2 != 0 {
			s.setMotionDisplayID(ctx, tm, ev.row.actionParam2)
		} else {
			s.motionMu.Lock()
			native := tm.NativeDisplayID
			s.motionMu.Unlock()
			if native != 0 {
				s.setMotionDisplayID(ctx, tm, native)
			}
		}
	}
}

// smartActionRandomEmote is SMART_ACTION_RANDOM_EMOTE
// (SmartScript.cpp:475-491): one random emote per target unit, drawn
// uniformly from the nonzero params 1-6 — the PLAY_EMOTE broadcast
// (SMSG_EMOTE to the per-target visible set) with a per-target pick.
// All-zero rows would select from an empty vector in C++; TDB 26091
// has none (min 1 nonzero of 121 rows), so the empty guard is
// defensive only.
func (s *Server) smartActionRandomEmote(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	emotes := make([]uint32, 0, 6)
	for _, e := range [6]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
		if e != 0 {
			emotes = append(emotes, e)
		}
	}
	if len(emotes) == 0 {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
		if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid); tm != nil {
			s.motionMu.Lock()
			mapID, instanceID, x, y, z, phase = tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask
			s.motionMu.Unlock()
		} else if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
			mapID, instanceID, x, y, z = sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z
			phase = sess.currentPlayerPhaseMask()
		}
		packet := protocol.NewBuffer(12)
		packet.WriteU32(emotes[rand.Intn(len(emotes))])
		packet.WriteU64(tgt.guid)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), mapID, instanceID, x, y, z, phase)
	}
}

// smartThreatPct is the THREAT_PCT value (SmartScript.cpp:493-519):
// threatINC (param1) minus threatDEC (param2), floored at -100 —
// the ModifyThreatByPercent percent, applied as a 0.01*(100+pct)
// scale (ThreatManager.h:143; the Lua ModifyThreatByPercent bridge).
func smartThreatPct(inc, dec uint32) int32 {
	pct := int32(inc) - int32(dec)
	if pct < -100 {
		pct = -100
	}
	return pct
}

// smartActionFailQuest is SMART_ACTION_FAIL_QUEST
// (SmartScript.cpp:420-431): every player target fails the quest
// (param1) via the Player::FailQuest twin (session.failQuest):
// INCOMPLETE-only (or COMPLETE timed with COMPLETED_AT_START), FAILED
// status, log-slot timer 1 + QUEST_STATE_FAIL,
// SMSG_QUESTGIVER_QUEST_FAILED, quest-item destruction.
func (s *Server) smartActionFailQuest(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.failQuest(ctx, questID)
		}
	}
}

// smartActionOfferQuest is SMART_ACTION_OFFER_QUEST
// (SmartScript.cpp:433-463): every player target is offered the quest
// (param1). With directAdd (param2) the quest is added straight via
// AddQuestAndCheckCompletion (Player.cpp:15103: AddQuest, then
// CompleteQuest when the objectives are already done — questGiver nil,
// so no OnQuestAccept leg runs); without it the giver shows the
// quest-details window when the player can take the quest
// (CanTakeQuest, msg=true — the Go accept-flow twin is canAddQuest).
// The template gate is the GetQuestTemplate null check. The one live
// row (quest 5209) uses directAdd, so the menu leg is unexercised.
func (s *Server) smartActionOfferQuest(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID, directAdd := ev.row.actionParam1, ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || !sess.questTemplateGate(ctx, questID) {
			continue
		}
		if directAdd == 0 {
			if !sess.canAddQuest(ctx, questID, true) {
				continue
			}
			data, err := sess.loadQuestDetailData(ctx, questID)
			if err != nil {
				continue
			}
			sess.sendQuestGiverQuestDetails(ctx, motion.GUID, data)
			continue
		}
		// AddQuestAndCheckCompletion's completion check already rides
		// inside addQuestToPlayer's tail (Player.cpp:15103 via
		// AddQuest:15284/15290 + the immediate-complete arm), so no
		// second canCompleteQuest/completeQuest call here — it would
		// send a duplicate quest update and fire the status-change
		// hook twice.
		sess.addQuestToPlayer(ctx, questID, 0)
	}
}

// smartVehicleKitForTarget resolves the vehicle kit on a target unit,
// the luaVehicleRef.resolve twin (lua_vehicle.go): the same-map key
// first, then the GUID scan for a base whose kit is keyed on a stale
// map/instance (a player base that far-teleported).
func (s *Server) smartVehicleKitForTarget(motion *creatureMotion, guid uint64) *VehicleKit {
	if motion == nil || guid == 0 {
		return nil
	}
	if kit := s.getVehicleKit(motion.Map, motion.InstanceID, guid); kit != nil {
		return kit
	}
	for _, loc := range s.findVehicleKits(guid) {
		return loc.kit
	}
	return nil
}

// smartActionCallAreaExploredOrEventHappens is
// SMART_ACTION_CALL_AREAEXPLOREDOREVENTHAPPENS (SmartScript.cpp:522-542):
// every player target takes the AreaExploredOrEventHappens credit
// (param1) via the session twin (quests.go); a unit target that IS a
// vehicle credits each seated player passenger instead (GetVehicleKit
// + seat scan, :527-531) — both arms fire for a player vehicle base.
// Gameobject targets take neither arm (C++'s IsUnit gate). TDB 26091:
// 187 live rows.
func (s *Server) smartActionCallAreaExploredOrEventHappens(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			if kit := s.smartVehicleKitForTarget(motion, tgt.guid); kit != nil {
				for _, passengerGUID := range kit.seatedPassengerGUIDs() {
					if sess := s.findSessionByGUID(passengerGUID); sess != nil {
						sess.areaExploredOrEventHappens(ctx, questID)
					}
				}
			}
		}
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.areaExploredOrEventHappens(ctx, questID)
		}
	}
}

// smartInvokerPlayerSession is
// WorldObject::GetCharmerOrOwnerPlayerOrPlayerItself (Object.cpp:2226-2233)
// for a SmartScript invoker: the invoker itself when it is a player
// session, else the charmer-or-owner player of an invoker creature (one
// level — GetCharmerOrOwnerGUID, Unit.h:1170), else nil.
func (s *Server) smartInvokerPlayerSession(motion *creatureMotion, invokerGUID uint64) *session {
	if sess := s.findSessionByGUID(invokerGUID); sess != nil {
		return sess
	}
	if motion == nil {
		return nil
	}
	tm := s.findCreatureMotion(motion.Map, motion.InstanceID, invokerGUID)
	if tm == nil {
		return nil
	}
	if ownerGUID := creatureCharmerOrOwnerGUID(tm); ownerGUID != 0 {
		return s.findSessionByGUID(ownerGUID)
	}
	return nil
}

// smartActionCallGroupEventHappens is SMART_ACTION_CALL_GROUPEVENTHAPPENS
// (SmartScript.cpp:852-872): the invoker's controlling player takes the
// group credit (param1) via the session groupEventHappens twin
// (quests.go) — GetCharmerOrOwnerPlayerOrPlayerItself, so a pet/charm
// invoker credits its owner — anchored on the base object (the motion)
// for the group reward distance test; a vehicle-riding invoker credits
// each seated passenger the same way (:864-867). The C++ `unit` is the
// invoker; a missing invoker no-ops (:854-855). TDB 26091: 15 rows.
func (s *Server) smartActionCallGroupEventHappens(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	invoker := invokerGUID
	if invoker == 0 {
		invoker = st.lastInvoker
	}
	if invoker == 0 {
		return
	}
	questID := ev.row.actionParam1
	base := s.luaMotionCreature(motion)
	credit := func(sess *session) {
		if sess != nil {
			sess.groupEventHappens(ctx, questID, base)
		}
	}
	credit(s.smartInvokerPlayerSession(motion, invoker))
	if kit := s.smartVehicleKitForTarget(motion, invoker); kit != nil {
		for _, passengerGUID := range kit.seatedPassengerGUIDs() {
			credit(s.findSessionByGUID(passengerGUID))
		}
	}
}

// smartRemoveAllAttackers is the Unit::RemoveAllAttackers half of
// CombatStop (Unit.cpp:5867-5876): every current attacker AttackStops —
// creatures whose target is me take the Lua AttackStop twin (visual
// target clear, stop packet, assistance latch reset, threat untouched),
// players attacking me clear their attack target and take the
// SMSG_ATTACK_STOP packet. Attackers keep their combat state: C++
// AttackStop ends no combat, only the m_attacking link.
func (s *Server) smartRemoveAllAttackers(ctx context.Context, motion *creatureMotion) {
	s.motionMu.Lock()
	var attackers []*creatureMotion
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || m.GUID == motion.GUID || m.TargetGUID != motion.GUID {
			continue
		}
		attackers = append(attackers, m)
	}
	s.motionMu.Unlock()
	for _, m := range attackers {
		s.motionMu.Lock()
		victim := m.TargetGUID
		if victim != motion.GUID {
			s.motionMu.Unlock()
			continue
		}
		m.TargetGUID = 0
		m.AlreadyCallAssistance = false
		mapID, instanceID, x, y, z, phase := m.Map, m.InstanceID, m.X, m.Y, m.Z, m.PhaseMask
		guid := m.GUID
		s.motionMu.Unlock()
		s.clearCreatureVisualTarget(m)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(guid, victim, false), mapID, instanceID, x, y, z, phase)
	}
	s.sessionsMu.RLock()
	var sessions []*session
	for sess := range s.sessions {
		if sess != nil && sess.attackTarget == motion.GUID {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess.attackTarget != motion.GUID {
			continue
		}
		sess.attackTarget = 0
		_ = sess.sendAttackStop(motion.GUID, false)
	}
}

// smartActionCombatStop is SMART_ACTION_COMBAT_STOP
// (SmartScript.cpp:873-881): me->CombatStop(true) (Unit.cpp:5809-5826) —
// interrupt the non-melee cast, AttackStop (visual target clear, stop
// packet, assistance latch reset), RemoveAllAttackers, then end PvE
// combat (InCombat=false; the mutualPvP=false leg — EndAllPvECombat /
// SuppressPvPCombat, no ClearInCombat). Threat is untouched: C++ keeps
// the threat list and re-engagement rides the normal aggro paths.
// TDB 26091: 13 rows, all SELF-targeted.
func (s *Server) smartActionCombatStop(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	interrupted := s.interruptCreatureNonMeleeCasts(motion)
	if len(interrupted) == 0 {
		motion.CastingUntil = time.Time{}
		s.cancelPendingCreatureCast(motion)
	}
	for _, arm := range interrupted {
		s.sendCreatureInterruptPackets(motion, arm)
	}
	s.motionMu.Lock()
	victim := motion.TargetGUID
	motion.InCombat = false
	if victim != 0 {
		motion.TargetGUID = 0
		motion.AlreadyCallAssistance = false
	}
	mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.motionMu.Unlock()
	if victim != 0 {
		s.clearCreatureVisualTarget(motion)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(motion.GUID, victim, false), mapID, instanceID, x, y, z, phase)
	}
	s.smartRemoveAllAttackers(ctx, motion)
}

// smartActionActivateGObject is SMART_ACTION_ACTIVATE_GOBJECT
// (SmartScript.cpp:693-706): every gameobject target is re-armed to
// GO_READY (loot state 1) and used via the UseDoorOrButton twin
// (delay 0 = template auto-close, alternative false). Non-GO targets
// (e.g. SELF on a creature row) are skipped, like C++'s IsGameObject
// check.
func (s *Server) smartActionActivateGObject(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		s.setGameObjectLootState(motion.Map, motion.InstanceID, tgt.guid, 1)
		s.useDoorOrButton(motion.Map, motion.InstanceID, tgt.guid, false, 0)
		// GameObject::UseDoorOrButton (GameObject.cpp:1443) leaves the
		// GO GO_ACTIVATED (loot state 2); the Go twin tracks state via
		// the GO_STATE/flag broadcast only, so the loot-state tail is
		// written here to match the C++ end state.
		s.setGameObjectLootState(motion.Map, motion.InstanceID, tgt.guid, 2)
	}
}

// smartActionThreatSinglePct is SMART_ACTION_THREAT_SINGLE_PCT
// (SmartScript.cpp:506-519): ModifyThreatByPercent on every target
// unit — a victim with no threat entry no-ops (ScaleThreat's unknown
// victim leg), and pct 0 scales by 1.0, the C++ no-op. TDB 26091:
// 11 rows, all SELF-targeted via VICTIM/INVOKER.
func (s *Server) smartActionThreatSinglePct(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if motion.ThreatMgr == nil {
		return
	}
	pct := smartThreatPct(ev.row.actionParam1, ev.row.actionParam2)
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		motion.ThreatMgr.ScaleThreat(tgt.guid, float32(0.01*float64(100+pct)))
	}
}

// smartActionThreatAllPct is SMART_ACTION_THREAT_ALL_PCT
// (SmartScript.cpp:493-504): ModifyThreatByPercent on every entry in
// the owner's threat list (the EntryGUIDs snapshot — C++ iterates the
// modifiable list). TDB 26091: 4 rows.
func (s *Server) smartActionThreatAllPct(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if motion.ThreatMgr == nil {
		return
	}
	pct := smartThreatPct(ev.row.actionParam1, ev.row.actionParam2)
	for _, guid := range motion.ThreatMgr.EntryGUIDs() {
		motion.ThreatMgr.ScaleThreat(guid, float32(0.01*float64(100+pct)))
	}
}

// smartActionSetEmoteState is SMART_ACTION_SET_EMOTE_STATE
// (SmartScript.cpp:721-730): SetUInt32Value(UNIT_NPC_EMOTESTATE) on
// every target unit — the motion EmoteState slot plus the values
// broadcast, the .npc playemote arm's path. Player targets stay out:
// Go has no player-side emote-state twin on this path and no TDB
// 26091 row targets one (target types 1/0/19/10/11 only). TDB 26091:
// 646 rows, 321 of them clearing to 0.
func (s *Server) smartActionSetEmoteState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.EmoteState = ev.row.actionParam1
		tm.EmoteStateKnown = true
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCEmoteState: ev.row.actionParam1})
	}
}

// smartActionEvade is SMART_ACTION_EVADE (SmartScript.cpp:820-835):
// when param1 (toRespawnPosition) is 0 the home position resets to
// the respawn position first, then EnterEvadeMode runs — the
// triggerCreatureEvade twin, which already fires SMART_EVENT_EVADE
// after _EnterEvadeMode (SmartAI.cpp:429) and takes the owner-follow
// leg. TDB 26091: 121 rows, param1 0 on all of them, so the reset
// always runs. The _evadeDisabled early arm (SmartAI.cpp:411-415,
// action 117, unported) stays out: evade is never disabled in Go.
// Summons/pets/totems carry no Spawn record, so the reset is a
// no-op for them and their (undrifted) home stands.
func (s *Server) smartActionEvade(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam1 == 0 {
		s.motionMu.Lock()
		if motion.SpawnKnown {
			motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO = motion.SpawnX, motion.SpawnY, motion.SpawnZ, motion.SpawnO
		}
		s.motionMu.Unlock()
	}
	s.triggerCreatureEvade(ctx, motion, time.Now())
}

// smartActionForceDespawn is SMART_ACTION_FORCE_DESPAWN
// (SmartScript.cpp:1104-1131): Creature::DespawnOrUnsummon(delay,
// forceRespawnTimer) on every target — the despawnCreatureMotion twin.
// C++ clamps a <=0 delay to 1ms so at least a world update tick runs
// before the despawn, keeping linked actions alive, so the despawn
// always rides a timer (ForcedDespawnDelayEvent, Creature.cpp:2219-2225;
// ForcedUnsummonDelayEvent, TemporarySummon.cpp:259-267). A nonzero
// forceRespawnTimer overrides the respawn time at expiry (the
// SaveRespawnTime leg, Creature.cpp:2258-2261). GO targets stay out:
// smartTargets yields no gameobjects.
func (s *Server) smartActionForceDespawn(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	delay := ev.row.actionParam1
	if delay == 0 {
		delay = 1
	}
	timer := ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, tgt.guid
		time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
			s.despawnCreatureMotion(context.Background(), mapID, instanceID, guid)
			if timer > 0 {
				s.overrideCreatureRespawnTime(context.Background(), uint32(guid&0x00FFFFFF), mapID, instanceID, time.Now().Add(time.Duration(timer)*time.Second))
			}
		})
	}
}

// smartActionSetInvincibilityHPLevel is SMART_ACTION_SET_INVINCIBILITY_HP_LEVEL
// (SmartScript.cpp:1155-1171): SetInvincibilityHpLevel on the target
// creature's SmartAI — the CAST_AI(SmartAI) gate is the AIName check
// plus a state attach, and a nonzero percent wins over the flat minHP
// (CountPctFromMaxHealth over the target's max health). The clamp
// itself lives in fireDealDamageAIHooks (SmartAI::DamageTaken,
// SmartAI.cpp:626-636). TDB 26091: 69 rows (minHP=1 x47, pct rows x14:
// pct=1 x11, 19/49/50 x1 each; 8 rows carry 0/0, which sets level 0
// and leaves the clamp off, matching C++'s falsy-level guard).
func (s *Server) smartActionSetInvincibilityHPLevel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		tst := s.smartStateFor(ctx, tm)
		if tst == nil {
			continue
		}
		level := ev.row.actionParam1
		if ev.row.actionParam2 != 0 {
			s.motionMu.Lock()
			maxHealth := tm.MaxHealth
			s.motionMu.Unlock()
			level = maxHealth * ev.row.actionParam2 / 100
		}
		tst.invincibilityHPLevel = level
	}
}

// smartActionMountToEntryOrModel is SMART_ACTION_MOUNT_TO_ENTRY_OR_MODEL
// (SmartScript.cpp:1133-1153): Unit::Mount on every target unit — the
// Lua Mount/Dismount twin (lua_creature_events.go:7183-7221) on the
// motion's Mount display + UNIT_FLAG_MOUNT. A nonzero creature entry
// mounts the template's ChooseDisplayId (ObjectMgr.cpp:1581-1593);
// a zero entry with a nonzero model mounts the model id directly;
// both zero dismounts. A missing template skips the target outright.
// Player targets stay out: Go has no player-side Mount twin for this
// path. TDB 26091: 42 rows (creature-entry mounts x3, model mounts
// x14, dismounts x25 — e.g. 19592/19594->19596, 24238->22657;
// 18294->14375, 22448->20964; 23030, 23580, 30703, 30704).
func (s *Server) smartActionMountToEntryOrModel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if ev.row.actionParam1 == 0 && ev.row.actionParam2 == 0 {
			s.motionMu.Lock()
			if tm.UnitFlags&0x08000000 == 0 { // not IsMounted
				s.motionMu.Unlock()
				continue
			}
			tm.Mount = 0
			tm.UnitFlags &^= 0x08000000
			tm.UnitFlagsKnown = true
			tm.MountKnown = true
			s.motionMu.Unlock()
			continue
		}
		displayID := ev.row.actionParam2
		if ev.row.actionParam1 > 0 {
			var ok bool
			displayID, ok = s.smartChooseDisplayID(ctx, ev.row.actionParam1)
			if !ok {
				continue
			}
		}
		s.motionMu.Lock()
		if displayID != 0 {
			tm.Mount = displayID
		}
		tm.UnitFlags |= 0x08000000 // UNIT_FLAG_MOUNT
		tm.UnitFlagsKnown = true
		tm.MountKnown = true
		s.motionMu.Unlock()
	}
}

// smartChooseDisplayID is ObjectMgr::ChooseDisplayId (ObjectMgr.cpp:1581-1593)
// for a creature_template entry: a trigger template
// (CREATURE_FLAG_EXTRA_TRIGGER, 0x80) takes its first is_trigger model,
// any other template takes a uniform random valid model
// (CreatureTemplate::GetRandomValidModelId, Creature.cpp:112-125;
// GetFirstInvisibleModel, Creature.cpp:134-145). False when the template
// is missing — C++ skips the mount then — and a 0 display with true means
// the template has no valid model, which still mounts the flag
// (Unit::Mount(0), Unit.cpp:8310-8315).
func (s *Server) smartChooseDisplayID(ctx context.Context, entry uint32) (uint32, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0, false
	}
	var model1, model2, model3, model4, flagsExtra int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(modelid1, 0), COALESCE(modelid2, 0), COALESCE(modelid3, 0), COALESCE(modelid4, 0), COALESCE(flags_extra, 0) FROM creature_template WHERE entry = ?", entry).Scan(&model1, &model2, &model3, &model4, &flagsExtra); err != nil {
		return 0, false
	}
	models := []int64{model1, model2, model3, model4}
	if flagsExtra&0x80 != 0 {
		for _, m := range models {
			if m == 0 {
				continue
			}
			var trigger int64
			if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(is_trigger, 0) FROM creature_model_info WHERE DisplayID = ?", m).Scan(&trigger); err == nil && trigger != 0 {
				return uint32(m), true
			}
		}
		return 0, true
	}
	var valid []uint32
	for _, m := range models {
		if m > 0 {
			valid = append(valid, uint32(m))
		}
	}
	if len(valid) == 0 {
		return 0, true
	}
	return valid[rand.Intn(len(valid))], true
}

// smartActionRespawnBySpawnID is SMART_ACTION_RESPAWN_BY_SPAWNID
// (SmartScript.cpp:2313-2326): Map::Respawn(spawnType, spawnId) —
// GetRespawnInfo, then respawnTime = now. The Go twin queues a due-now
// creature respawn record for the spawn id, which the world tick's
// processCreatureRespawns revives; a live motion for the spawn is a
// no-op, matching C++ (no RespawnInfo exists for a live spawn). The
// gameobject type stays out: Go has no GO-respawn model. TDB 26091:
// 17 rows (15 creature-type, e.g. 18471->48191, 24023->74518/74519/
// 74520; 2 gameobject-type, which stay out).
func (s *Server) smartActionRespawnBySpawnID(ctx context.Context, motion *creatureMotion, ev *smartScriptEvent) {
	if ev.row.actionParam1 != 0 {
		return
	}
	spawnID := ev.row.actionParam2
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var entry, mapID int64
	var x, y, z float64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map, position_x, position_y, position_z FROM creature WHERE guid = ?", spawnID).Scan(&entry, &mapID, &x, &y, &z); err != nil {
		return
	}
	s.motionMu.Lock()
	live := s.findCreatureMotionLocked(uint32(mapID), 0, creatureWorldGUID(spawnID, uint32(entry)))
	alive := live != nil && live.Health > 0
	s.motionMu.Unlock()
	if alive {
		return
	}
	stats := s.loadCreatureStats(ctx, uint32(entry))
	now := time.Now()
	s.motionMu.Lock()
	if s.creatureRespawns == nil {
		s.creatureRespawns = make(map[uint32]creatureRespawn)
	}
	s.creatureRespawns[spawnID] = creatureRespawn{GUID: spawnID, At: now, Health: stats.MaxHealth, Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), CorpseRemoved: true}
	s.motionMu.Unlock()
	s.saveCreatureRespawnRow(ctx, spawnID, now, uint32(mapID), 0)
}

// smartActionRemoveUnitFieldBytes1 is SMART_ACTION_REMOVE_UNIT_FIELD_BYTES_1
// (SmartScript.cpp:1851-1856): RemoveByteFlag(UNIT_FIELD_BYTES_1, type,
// byte1) — param1 is the flag byte, param2 the byte offset. Byte 0 is the
// stand-state byte, which Go models as motion.StandState (all 12 AGGRO
// rows are offset 0, e.g. clearing SIT to stand). Bytes 1-3 (animation
// tier, shapeshift, ...) have no Go model and stay out.
func (s *Server) smartActionRemoveUnitFieldBytes1(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam2 != 0 {
		return
	}
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	for _, tgt := range targets {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.StandState &^= uint8(ev.row.actionParam1)
		tm.StandStateKnown = true
		s.motionMu.Unlock()
	}
}

// smartActionSetReactState is SMART_ACTION_SET_REACT_STATE
// (SmartScript.cpp:464-472): every creature target takes the param1
// react state (Creature.h:117: a plain setter, no broadcast). The
// ReactStateKnown stamp keeps the sweep from re-affirming the template
// value (creaturemotion.go:5945) — the same latch the Lua SetReactState
// bridge stamps. The Go react values match C++'s ReactStates order
// (UnitDefines.h:340-342: passive 0, defensive 1, aggressive 2).
func (s *Server) smartActionSetReactState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ReactState = uint8(ev.row.actionParam1)
		tm.ReactStateKnown = true
		s.motionMu.Unlock()
	}
}

// smartActionNPCFlag is SMART_ACTION_SET_NPC_FLAG / ADD_NPC_FLAG /
// REMOVE_NPC_FLAG (SmartScript.cpp:1700-1720): SetUInt32Value /
// SetFlag / RemoveFlag on UNIT_NPC_FLAGS (field 82) for creature
// targets — whole-value / OR / AND-NOT. The motion value plus the
// NPCFlagsKnown latch (the Lua SetNPCFlags bridge's twin) and the
// field-82 values broadcast carry it to clients.
func (s *Server) smartActionNPCFlag(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		switch ev.row.actionType {
		case smartActionSetNPCFlag:
			tm.NPCFlags = ev.row.actionParam1
		case smartActionAddNPCFlag:
			tm.NPCFlags |= ev.row.actionParam1
		default:
			tm.NPCFlags &^= ev.row.actionParam1
		}
		tm.NPCFlagsKnown = true
		flags := tm.NPCFlags
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCFlags: flags})
	}
}

// broadcastCreatureDestroyToPlayers pushes SMSG_DESTROY_OBJECT for a
// creature to every non-GM-seer session in its map/instance — the hide
// half of the Lua SetVisible twin (lua_creature_events.go:7031). GM
// seers (gmVisibilityDetect != 0) keep seeing the unit, matching C++
// server-side visibility SERVERSIDE_VISIBILITY_GM.
func (s *Server) broadcastCreatureDestroyToPlayers(mapID, instanceID uint32, guid uint64) {
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(guid)
	destroyBuf.WriteU8(0)
	payload := destroyBuf.Bytes()
	opcode := uint16(protocol.OpcodeSMSG_DESTROY_OBJECT)
	s.sessionsMu.RLock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil ||
			target.player.Map != mapID || target.player.InstanceID != instanceID ||
			target.gmVisibilityDetect != 0 {
			continue
		}
		_ = target.write(opcode, payload, true)
	}
	s.sessionsMu.RUnlock()
}

// smartActionSetVisibility is SMART_ACTION_SET_VISIBILITY
// (SmartScript.cpp:1225-1231): Unit::SetVisible on every target unit
// (Unit.cpp:8656-8663) — server-side visibility GM/PLAYER plus an
// object-visibility update. The creature leg is the Lua SetVisible twin
// (lua_creature_events.go:7031): ScriptHidden set under motionMu, and on
// hide the destroy object is pushed to the non-GM-seer sessions in the
// instance; on show the flag clears and the periodic nearby-creature
// rebuild re-creates the unit. Player targets stay out: Go has no
// player-side server-visibility twin (TDB 26091: the 1 live row
// targets SELF).
func (s *Server) smartActionSetVisibility(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	visible := ev.row.actionParam1 != 0
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ScriptHidden = !visible
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		if !visible {
			s.broadcastCreatureDestroyToPlayers(mapID, instanceID, guid)
		}
	}
}

// smartActionInterruptSpell is SMART_ACTION_INTERRUPT_SPELL
// (SmartScript.cpp:1858-1867): InterruptNonMeleeSpells(withDelayed,
// spell_id, withInstant) on every target unit. The creature leg runs the
// spell-filtered twin (interruptCreatureNonMeleeCastsForSpell, param2
// as the id gate — TDB 26091: the 1 live row names 44473) and sends the
// cancel packets per interrupted arm, the INTERRUPT_PREVIOUS precedent
// in smartActionCast. withDelayed/withInstant are vacuous in Go's
// synchronous cast model (see the twin's comment). Player targets stay
// out: Go has no plain player-side InterruptNonMeleeSpells twin.
func (s *Server) smartActionInterruptSpell(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		for _, arm := range s.interruptCreatureNonMeleeCastsForSpell(tm, ev.row.actionParam2) {
			s.sendCreatureInterruptPackets(tm, arm)
		}
	}
}

// smartActionSetRoot is SMART_ACTION_SET_ROOT (SmartScript.cpp:2007-2014):
// SetControlled(root, UNIT_STATE_ROOT) on creature targets only — C++
// skips non-creatures itself (IsCreature). ROOT is not in
// UNIT_STATE_CONTROLLED (Unit.h:251), so apply runs no CastStop leg —
// the plain ScriptRooted set is the exact twin (Lua SetRooted,
// lua_creature_events.go:5637); the combat-tick chase gate honors it.
func (s *Server) smartActionSetRoot(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	root := ev.row.actionParam1 != 0
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ScriptRooted = root
		s.motionMu.Unlock()
	}
}

// smartActionCallTimedActionList is SMART_ACTION_CALL_TIMED_ACTIONLIST
// (SmartScript.cpp:1677-1700) plus the RANDOM (87, :1763-1792) and
// RANDOM_RANGE (88, :1794-1814) twins: each target creature that IsSmart
// installs the list with the calling script's last invoker. TARGET_NONE
// installs nothing (C++ logs the bad row and breaks; Go resolves no
// targets for it). The random twins share the calling row's param2/3 as
// timerType/allowOverride through the C++ action union overlap
// (SmartScriptMgr.h:977-983: randTimedActionList.actionLists[1..2] are
// timedActionList.timerType/allowOverride), so 87 reads its ids from
// params 1-6 and 88 its range from params 1-2 at the same positions.
func (s *Server) smartActionCallTimedActionList(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.targetType == smartTargetNone {
		return
	}
	listID := ev.row.actionParam1
	switch ev.row.actionType {
	case smartActionCallRandomTimedActionList:
		var ids []uint32
		for _, p := range []uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
			if p != 0 {
				ids = append(ids, p)
			}
		}
		if len(ids) == 0 {
			return
		}
		listID = ids[rand.Intn(len(ids))]
	case smartActionCallRandomRangeTimedActionList:
		min, max := ev.row.actionParam1, ev.row.actionParam2
		if max < min {
			max = min
		}
		if max == min {
			listID = min
		} else {
			listID = min + uint32(rand.Int63n(int64(max-min+1)))
		}
	}
	invoker := invokerGUID
	if invoker == 0 {
		invoker = st.lastInvoker
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		s.smartSetTimedActionList(ctx, tm, listID, ev.row.actionParam2, ev.row.actionParam3 != 0, invoker)
	}
}

// smartTarget is one resolved SmartAI target: the GUID plus whether it is
// a player (creature otherwise — GOs never reach the AGGRO-used actions).
type smartTarget struct {
	guid         uint64
	isPlayer     bool
	isGameObject bool
}

// smartTargets is SmartScript::GetTargets (SmartScript.cpp:2510-...): the
// target list for the row's target type, resolved against the motion, the
// invoker (falling back to the last invoker, like C++'s scriptTrigger),
// and the stored-target lists. Range scans read the instance motion map
// under motionMu — the same-instance approximation of C++'s grid search
// (C++ searches the creature's grid cell; Go has no grid, so the whole
// instance is the search space). Distances are 3D, matching C++'s
// IsInRange / GetWorldObjectsInDist.
func (s *Server) smartTargets(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) []smartTarget {
	if motion == nil || ev == nil {
		return nil
	}
	trigger := invokerGUID
	if trigger == 0 {
		trigger = st.lastInvoker
	}
	switch ev.row.targetType {
	case smartTargetNone:
		return nil
	case smartTargetSelf:
		return []smartTarget{{guid: motion.GUID}}
	case smartTargetVictim:
		if motion.TargetGUID == 0 {
			return nil
		}
		return []smartTarget{{guid: motion.TargetGUID, isPlayer: s.smartIsPlayerGUID(motion.TargetGUID)}}
	case smartTargetActionInvoker:
		if trigger == 0 {
			return nil
		}
		return []smartTarget{{guid: trigger, isPlayer: s.smartIsPlayerGUID(trigger)}}
	case smartTargetPosition:
		return nil
	case smartTargetCreatureRange:
		return s.smartCreatureRangeTargets(motion, ev.row.targetParam1, float32(ev.row.targetParam2), float32(ev.row.targetParam3), ev.row.targetParam4)
	case smartTargetCreatureGUID:
		return s.smartCreatureGUIDTarget(motion, ev.row.targetParam1, ev.row.targetParam2)
	case smartTargetCreatureDist:
		return s.smartCreatureRangeTargets(motion, ev.row.targetParam1, 0, float32(ev.row.targetParam2), ev.row.targetParam3)
	case smartTargetStored:
		stored := st.storedTargets[ev.row.targetParam1]
		out := make([]smartTarget, 0, len(stored))
		for _, guid := range stored {
			out = append(out, smartTarget{guid: guid, isPlayer: s.smartIsPlayerGUID(guid)})
		}
		return out
	case smartTargetInvokerParty:
		return s.smartInvokerPartyTargets(motion, trigger)
	case smartTargetGameObjectRange:
		return s.smartGameObjectRangeTargets(ctx, motion, ev.row.targetParam1, float32(ev.row.targetParam2), float32(ev.row.targetParam3), ev.row.targetParam4)
	case smartTargetGameObjectGUID:
		return s.smartGameObjectGUIDTarget(ctx, motion, ev.row.targetParam1, ev.row.targetParam2)
	case smartTargetGameObjectDist:
		return s.smartGameObjectRangeTargets(ctx, motion, ev.row.targetParam1, 0, float32(ev.row.targetParam2), ev.row.targetParam3)
	case smartTargetPlayerRange:
		return s.smartPlayerRangeTargets(motion, float32(ev.row.targetParam1), float32(ev.row.targetParam2))
	case smartTargetPlayerDist:
		return s.smartPlayerRangeTargets(motion, 0, float32(ev.row.targetParam1))
	case smartTargetClosestCreature:
		return s.smartClosestCreatureTarget(motion, ev.row.targetParam1, ev.row.targetParam2, ev.row.targetParam3)
	case smartTargetClosestGameObject:
		return s.smartClosestGameObjectTarget(ctx, motion, ev.row.targetParam1, float32(ev.row.targetParam2))
	case smartTargetOwnerOrSummoner:
		return s.smartOwnerOrSummonerTarget(motion, ev.row.targetParam1)
	case smartTargetThreatList:
		return s.smartThreatListTargets(motion, float32(ev.row.targetParam1))
	case smartTargetLootRecipients:
		return s.smartLootRecipientTargets(motion)
	default:
		return nil
	}
}

func (s *Server) smartIsPlayerGUID(guid uint64) bool {
	if guid == 0 {
		return false
	}
	return s.findSessionByGUID(guid) != nil
}

// smartCreatureRangeTargets is the CREATURE_RANGE (SmartScript.cpp:2584)
// / CREATURE_DISTANCE (:2605) scan: creatures of the entry (0 = any) in
// [minDist, maxDist] of the motion, excluding self, capped to a random
// maxSize when set (Trinity::Containers::RandomResize).
func (s *Server) smartCreatureRangeTargets(motion *creatureMotion, entry uint32, minDist, maxDist float32, maxSize uint32) []smartTarget {
	s.motionMu.Lock()
	var found []*creatureMotion
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || m.GUID == motion.GUID {
			continue
		}
		// No alive check: the C++ arms test entry and range only, so a
		// corpse in range is a valid target (the dead filter lives in
		// the CLOSEST_CREATURE arm's dead flag, not here).
		if entry != 0 && m.Entry != entry {
			continue
		}
		d := distance3D(motion.X, motion.Y, motion.Z, m.X, m.Y, m.Z)
		if d < float64(minDist) || d > float64(maxDist) {
			continue
		}
		found = append(found, m)
	}
	s.motionMu.Unlock()
	if maxSize > 0 && uint32(len(found)) > maxSize {
		rand.Shuffle(len(found), func(i, j int) { found[i], found[j] = found[j], found[i] })
		found = found[:maxSize]
	}
	out := make([]smartTarget, 0, len(found))
	for _, m := range found {
		out = append(out, smartTarget{guid: m.GUID})
	}
	return out
}

// smartCreatureGUIDTarget is SMART_TARGET_CREATURE_GUID
// (SmartScript.cpp:2623): the creature with spawn GUID dbGuid (entry 0 =
// any) near the trigger/base object. C++ searches the grid around the
// trigger; Go scans the instance (same documented approximation as the
// range scans) and takes the first match.
func (s *Server) smartCreatureGUIDTarget(motion *creatureMotion, dbGuid, entry uint32) []smartTarget {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil {
			continue
		}
		// No alive check here either: FindCreatureNear tests guid and
		// entry only.
		if uint32(m.GUID&0x00FFFFFF) != dbGuid {
			continue
		}
		if entry != 0 && m.Entry != entry {
			continue
		}
		return []smartTarget{{guid: m.GUID}}
	}
	return nil
}

// smartInvokerPartyTargets is SMART_TARGET_INVOKER_PARTY
// (SmartScript.cpp:2557-2582): the invoker player's group members on the
// same map (same instance in Go), or the invoker alone with no group.
func (s *Server) smartInvokerPartyTargets(motion *creatureMotion, trigger uint64) []smartTarget {
	invokerSess := s.findSessionByGUID(trigger)
	if invokerSess == nil || invokerSess.player == nil {
		if trigger == 0 {
			return nil
		}
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	if invokerSess.groupID == 0 {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	grp := s.findGroupByID(invokerSess.groupID)
	if grp == nil {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	var out []smartTarget
	for _, member := range grp.Members {
		sess := s.findSessionByGUID(member.GUID)
		if sess == nil || sess.player == nil {
			continue
		}
		if sess.player.Map != invokerSess.player.Map || sess.player.InstanceID != invokerSess.player.InstanceID {
			continue
		}
		out = append(out, smartTarget{guid: member.GUID, isPlayer: true})
	}
	if len(out) == 0 {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	return out
}

// smartClosestCreatureTarget is SMART_TARGET_CLOSEST_CREATURE
// (SmartScript.cpp:2684): the nearest creature of the entry (0 = any)
// within dist (0 = 100), alive unless the dead flag is set. C++'s
// FindNearestCreature arm carries no self-exclusion; Go excludes self,
// matching the CREATURE_RANGE/DISTANCE arms and the sane reading — a
// creature's "closest creature X" target is never itself.
func (s *Server) smartClosestCreatureTarget(motion *creatureMotion, entry, dist, dead uint32) []smartTarget {
	maxDist := float64(dist)
	if maxDist == 0 {
		maxDist = 100
	}
	s.motionMu.Lock()
	var best *creatureMotion
	bestDist := maxDist
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || m.GUID == motion.GUID {
			continue
		}
		if entry != 0 && m.Entry != entry {
			continue
		}
		if dead == 0 && m.Health == 0 {
			continue
		}
		d := distance3D(motion.X, motion.Y, motion.Z, m.X, m.Y, m.Z)
		if d <= bestDist {
			bestDist, best = d, m
		}
	}
	s.motionMu.Unlock()
	if best == nil {
		return nil
	}
	return []smartTarget{{guid: best.GUID}}
}

// smartGOCandidate is one gameobject found by the GO target scans, with
// its 3D distance from the motion kept so the range/closest arms can
// filter without re-scanning.
type smartGOCandidate struct {
	guid uint64
	dist float64
}

// smartGameObjectNear is the shared scan behind the GAMEOBJECT target
// arms: static spawns from the gameobject table plus runtime GOs from
// the dynamic/instance stores, deduplicated by GUID (a materialized
// static GO exists in both), within maxDist of the motion. Hidden GOs
// (despawned) are filtered after the snapshot, like
// nearestGameObjectByEntry — isGameObjectHiddenInInstance takes
// objectsMu.
func (s *Server) smartGameObjectNear(ctx context.Context, motion *creatureMotion, entry uint32, maxDist float32) []smartGOCandidate {
	seen := map[uint64]bool{}
	var found []smartGOCandidate
	add := func(guid uint64, x, y, z float32) {
		if seen[guid] {
			return
		}
		d := distance3D(motion.X, motion.Y, motion.Z, x, y, z)
		if d > float64(maxDist) {
			return
		}
		seen[guid] = true
		found = append(found, smartGOCandidate{guid: guid, dist: d})
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT guid, id, position_x, position_y, position_z FROM gameobject WHERE map = ? AND (? = 0 OR id = ?) AND position_x BETWEEN ? AND ? AND position_y BETWEEN ? AND ?", motion.Map, entry, entry, float64(motion.X-maxDist), float64(motion.X+maxDist), float64(motion.Y-maxDist), float64(motion.Y+maxDist))
		if err == nil {
			for rows.Next() {
				var lowGUID, id uint32
				var x, y, z float32
				if err := rows.Scan(&lowGUID, &id, &x, &y, &z); err != nil {
					continue
				}
				add(gameObjectGUID(lowGUID, id), x, y, z)
			}
			rows.Close()
		}
	}
	s.objectsMu.RLock()
	var dyn []*dynamicGameObjectState
	for _, d := range s.dynamicGameObjects {
		dyn = append(dyn, d)
	}
	for _, d := range s.instanceGameObjects[instanceAdmissionKey{MapID: motion.Map, InstanceID: motion.InstanceID}] {
		dyn = append(dyn, d)
	}
	s.objectsMu.RUnlock()
	for _, d := range dyn {
		if d == nil || d.Map != motion.Map || d.InstanceID != motion.InstanceID {
			continue
		}
		if entry != 0 && d.Entry != entry {
			continue
		}
		add(d.GUID, d.X, d.Y, d.Z)
	}
	out := found[:0]
	for _, c := range found {
		if s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, c.guid) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// smartGameObjectRangeTargets is SMART_TARGET_GAMEOBJECT_RANGE
// (SmartScript.cpp:2676-2699) / GAMEOBJECT_DISTANCE (:2655-2674): GOs of
// the entry (0 = any) in [minDist, maxDist] of the motion, capped to a
// random maxSize when set (Trinity::Containers::RandomResize). C++
// skips the base GO when the script runs on a GO; the Go engine only
// runs creature scripts, so there is no self GO to skip.
func (s *Server) smartGameObjectRangeTargets(ctx context.Context, motion *creatureMotion, entry uint32, minDist, maxDist float32, maxSize uint32) []smartTarget {
	near := s.smartGameObjectNear(ctx, motion, entry, maxDist)
	out := make([]smartTarget, 0, len(near))
	for _, c := range near {
		if c.dist < float64(minDist) {
			continue
		}
		out = append(out, smartTarget{guid: c.guid, isGameObject: true})
	}
	if maxSize > 0 && uint32(len(out)) > maxSize {
		rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		out = out[:maxSize]
	}
	return out
}

// smartGameObjectGUIDTarget is SMART_TARGET_GAMEOBJECT_GUID
// (SmartScript.cpp:2710-2722): the GO with spawn GUID dbGuid (entry 0 =
// any) on the creature's map. C++ looks the spawn id up in the map's
// store with no distance check (FindGameObjectNear,
// SmartScript.cpp:147-154); the runtime stores are checked the same way
// for summoned GOs.
func (s *Server) smartGameObjectGUIDTarget(ctx context.Context, motion *creatureMotion, dbGuid, entry uint32) []smartTarget {
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		var id uint32
		if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM gameobject WHERE guid = ? AND map = ?", dbGuid, motion.Map).Scan(&id); err == nil {
			if entry == 0 || id == entry {
				guid := gameObjectGUID(dbGuid, id)
				if !s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, guid) {
					return []smartTarget{{guid: guid, isGameObject: true}}
				}
			}
			return nil
		}
	}
	s.objectsMu.RLock()
	var match *dynamicGameObjectState
	for _, d := range s.dynamicGameObjects {
		if d != nil && d.LowGUID == dbGuid && d.Map == motion.Map && d.InstanceID == motion.InstanceID {
			match = d
			break
		}
	}
	if match == nil {
		for _, d := range s.instanceGameObjects[instanceAdmissionKey{MapID: motion.Map, InstanceID: motion.InstanceID}] {
			if d != nil && d.LowGUID == dbGuid {
				match = d
				break
			}
		}
	}
	s.objectsMu.RUnlock()
	if match != nil && (entry == 0 || match.Entry == entry) {
		if !s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, match.GUID) {
			return []smartTarget{{guid: match.GUID, isGameObject: true}}
		}
	}
	return nil
}

// smartClosestGameObjectTarget is SMART_TARGET_CLOSEST_GAMEOBJECT
// (SmartScript.cpp:2761-2767): the nearest GO of the entry (0 = any)
// within dist of the motion (0 defaults to 100, like
// FindNearestGameObject).
func (s *Server) smartClosestGameObjectTarget(ctx context.Context, motion *creatureMotion, entry uint32, dist float32) []smartTarget {
	maxDist := dist
	if maxDist == 0 {
		maxDist = 100
	}
	near := s.smartGameObjectNear(ctx, motion, entry, maxDist)
	if len(near) == 0 {
		return nil
	}
	best := near[0]
	for _, c := range near[1:] {
		if c.dist < best.dist {
			best = c
		}
	}
	return []smartTarget{{guid: best.guid, isGameObject: true}}
}

// smartPlayerRangeTargets is SMART_TARGET_PLAYER_RANGE
// (SmartScript.cpp:2723-2733) / PLAYER_DISTANCE (:2734-2742): players
// in [minDist, maxDist] of the motion on the same map and instance.
// C++ filters no dead players here, so Go doesn't either.
func (s *Server) smartPlayerRangeTargets(motion *creatureMotion, minDist, maxDist float32) []smartTarget {
	s.sessionsMu.RLock()
	var found []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
			continue
		}
		found = append(found, sess)
	}
	s.sessionsMu.RUnlock()
	var out []smartTarget
	for _, sess := range found {
		d := distance3D(motion.X, motion.Y, motion.Z, sess.player.X, sess.player.Y, sess.player.Z)
		if d < float64(minDist) || d > float64(maxDist) {
			continue
		}
		out = append(out, smartTarget{guid: sess.player.GUID, isPlayer: true})
	}
	return out
}

// smartOwnerOrSummonerTarget is SMART_TARGET_OWNER_OR_SUMMONER
// (SmartScript.cpp:2774-2806): the motion's charmer, else owner, else
// creator/summoner (GetCharmerOrOwnerGUID, then the TempSummon summoner
// leg, then GetCreatorGUID — the summon leg is folded into
// CreatedByGUID, seeded by summonTempCreature). With useCharmerOrOwner
// the owner's charmer/owner is returned instead. A player owner with no
// charmer resolves to nothing, like C++'s GetUnit on an empty GUID.
func (s *Server) smartOwnerOrSummonerTarget(motion *creatureMotion, useCharmerOrOwner uint32) []smartTarget {
	guid := motion.CharmerGUID
	if guid == 0 {
		guid = motion.OwnerGUID
	}
	if guid == 0 {
		guid = motion.CreatedByGUID
	}
	if guid == 0 {
		return nil
	}
	if useCharmerOrOwner != 0 {
		if om := s.findCreatureMotion(motion.Map, motion.InstanceID, guid); om != nil {
			chained := om.CharmerGUID
			if chained == 0 {
				chained = om.OwnerGUID
			}
			if chained == 0 {
				chained = om.CreatedByGUID
			}
			if chained == 0 {
				return nil
			}
			guid = chained
		} else {
			return nil
		}
	}
	if s.findSessionByGUID(guid) != nil {
		return []smartTarget{{guid: guid, isPlayer: true}}
	}
	if s.findCreatureMotion(motion.Map, motion.InstanceID, guid) != nil {
		return []smartTarget{{guid: guid}}
	}
	return nil
}

// smartThreatListTargets is SMART_TARGET_THREAT_LIST
// (SmartScript.cpp:2808-2815): every unit on the owner's threat list
// within maxDist (target_param1 = hostilRandom.maxDist, Mgr.h:1274-1279;
// 0 = no limit). CanHaveThreatList's Go twin is a live threat manager;
// distances are plain 3D, like the other smart target range arms.
// Unresolvable victims (no live motion or session to measure from)
// are skipped.
func (s *Server) smartThreatListTargets(motion *creatureMotion, maxDist float32) []smartTarget {
	if motion == nil || motion.ThreatMgr == nil {
		return nil
	}
	var out []smartTarget
	for _, guid := range motion.ThreatMgr.EntryGUIDs() {
		if maxDist > 0 {
			var x, y, z float32
			if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, guid); tm != nil {
				x, y, z = tm.X, tm.Y, tm.Z
			} else if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
				x, y, z = sess.player.X, sess.player.Y, sess.player.Z
			} else {
				continue
			}
			if distance3D(motion.X, motion.Y, motion.Z, x, y, z) > float64(maxDist) {
				continue
			}
		}
		out = append(out, smartTarget{guid: guid, isPlayer: s.smartIsPlayerGUID(guid)})
	}
	return out
}

// smartLootRecipientTargets is SMART_TARGET_LOOT_RECIPIENTS
// (SmartScript.cpp:2830-2845): the tap group's members on the same map
// and instance when the creature was tapped by a group
// (GetLootRecipientGroup, Creature.cpp:1321-1326 — the m_lootRecipientGroup
// twin is motion.TapGroupID), else the single tap player
// (GetLootRecipient). The single-recipient leg carries no IsInMap gate
// in C++; the tap player is returned whenever the session resolves.
func (s *Server) smartLootRecipientTargets(motion *creatureMotion) []smartTarget {
	if motion == nil {
		return nil
	}
	s.motionMu.Lock()
	tapPlayer, tapGroup := motion.TapPlayerGUID, motion.TapGroupID
	mapID, instanceID := motion.Map, motion.InstanceID
	s.motionMu.Unlock()
	if tapGroup != 0 {
		if grp := s.findGroupByID(tapGroup); grp != nil {
			var out []smartTarget
			for _, member := range grp.Members {
				sess := s.findSessionByGUID(member.GUID)
				if sess == nil || sess.player == nil {
					continue
				}
				if sess.player.Map != mapID || sess.player.InstanceID != instanceID {
					continue
				}
				out = append(out, smartTarget{guid: member.GUID, isPlayer: true})
			}
			return out
		}
	}
	if tapPlayer != 0 {
		if sess := s.findSessionByGUID(tapPlayer); sess != nil {
			return []smartTarget{{guid: tapPlayer, isPlayer: true}}
		}
	}
	return nil
}

// smartFirstActiveAuraSpellID returns one live player aura id, or 0 — the
// peek the REMOVEAURASFROMSPELL spell-0 sweep needs to drain every aura
// without snapshotting the session's aura map.
func (s *session) smartFirstActiveAuraSpellID() uint32 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	for id := range s.activeAuras {
		return id
	}
	return 0
}
