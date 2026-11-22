// SmartAI engine — item-6 census Axis1#2 (slice 1: engine + hooks + TALK /
// SET_FACTION / PLAY_EMOTE / CAST / AUTO_ATTACK / SET_EVENT_PHASE;
// slice 2: the remaining AGGRO-used actions and targets).
//
// C++: SmartScript / SmartAI (src/server/game/AI/SmartScripts/SmartScript.cpp,
// SmartAI.cpp, SmartScriptMgr.h). Go had no SmartAI engine and no smart_scripts
// loader (the WORLD_SEL_SMART_SCRIPTS statement existed unused, the .reload
// arm was a stub), so every SmartAI creature's on-aggro script was silently
// dead. This file loads the creature-source rows (source_type 0), keeps
// per-creature runtime state on the motion, and dispatches the direct events
// (AGGRO, DEATH, DATA_SET, AI_INIT, JUST_CREATED, TEXT_OVER,
// EVENT_PHASE_CHANGE) plus the timer events (UPDATE, UPDATE_IC, UPDATE_OOC,
// HEALTH_PCT, TARGET_HEALTH_PCT, MANA_PCT, TARGET_MANA_PCT, RANGE) from the
// creature tick. Action coverage is the AGGRO-used set (TDB 26091: 1,634
// source_type-0 AGGRO rows over 1,426 entries): TALK, CAST, SET_FACTION,
// PLAY_EMOTE, AUTO_ATTACK, SET_EVENT_PHASE, SET_INST_DATA (type 1),
// CALL_FOR_HELP, SET_DATA, SOUND, STORE_TARGET_LIST, REMOVEAURASFROMSPELL,
// REMOVE_UNIT_FIELD_BYTES_1 (byte 0 only). Everything else (SUMMON_CREATURE,
// timed-action lists, GO/instance/area-trigger sources, .reload refresh,
// RESET wiring) is a documented residual in smartProcessAction / the call
// sites below, not a silent gap.
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
	smartEventUpdateIC       uint32 = 0
	smartEventUpdateOOC      uint32 = 1
	smartEventHealthPct      uint32 = 2
	smartEventManaPct        uint32 = 3
	smartEventAggro          uint32 = 4
	smartEventKill           uint32 = 5
	smartEventDeath          uint32 = 6
	smartEventEvade          uint32 = 7
	smartEventRange          uint32 = 9
	smartEventReset          uint32 = 25
	smartEventDataSet        uint32 = 38 // SMART_EVENT_DATA_SET; 26 is SMART_EVENT_IC_LOS, not DATA_SET
	smartEventTargetHealthPc uint32 = 12
	smartEventTargetManaPct  uint32 = 18
	smartEventAIInit         uint32 = 37
	smartEventTextOver       uint32 = 52
	smartEventUpdate         uint32 = 60
	smartEventLink           uint32 = 61
	smartEventJustCreated    uint32 = 63
	smartEventPhaseChange    uint32 = 66
)

// SMART_ACTION ids (SmartScriptMgr.h:457-600), AGGRO-used set. The
// remaining AGGRO-row actions (80, 31, 21, 83, 8, 88, 43, 42, 146, 103,
// 23, 49, 36, 41, 87, 25, 47, 5, 12, 74, 92, 133, 30 — 71 rows) hit the
// default arm, matching C++'s "Unhandled Action type" log-and-skip
// (SmartScript.cpp:2370-2372): no timed-actionlist, summon, react-state,
// or template machinery exists in Go to run them on.
const (
	smartActionTalk                  uint32 = 1
	smartActionSetFaction            uint32 = 2
	smartActionSound                 uint32 = 4
	smartActionPlayEmote             uint32 = 5
	smartActionCast                  uint32 = 11
	smartActionAutoAttack            uint32 = 20
	smartActionSetEventPhase         uint32 = 22
	smartActionRemoveAurasFromSpell  uint32 = 28
	smartActionSetInstData           uint32 = 34
	smartActionCallForHelp           uint32 = 39
	smartActionSetData               uint32 = 45
	smartActionStoreTargetList       uint32 = 64
	smartActionRemoveUnitFieldBytes1 uint32 = 91
)

// SMART_TARGET ids (SmartScriptMgr.h:1221-1253), AGGRO-used set. Target 8
// (POSITION) resolves to no unit — its only AGGRO row drives the unhandled
// SUMMON_CREATURE action, so it stays empty; target 13 (GAMEOBJECT_RANGE)
// and 21 (CLOSEST_PLAYER) have zero AGGRO rows in TDB 26091.
const (
	smartTargetNone            uint32 = 0
	smartTargetSelf            uint32 = 1
	smartTargetVictim          uint32 = 2
	smartTargetActionInvoker   uint32 = 7
	smartTargetPosition        uint32 = 8
	smartTargetCreatureRange   uint32 = 9
	smartTargetCreatureGUID    uint32 = 10
	smartTargetCreatureDist    uint32 = 11
	smartTargetStored          uint32 = 12
	smartTargetInvokerParty    uint32 = 16
	smartTargetClosestCreature uint32 = 19
)

// SMART_EVENT_FLAG bits (SmartScriptMgr.h:1522-1531).
const (
	smartEventFlagNotRepeatable uint32 = 0x001
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
type smartScriptEvent struct {
	row     *smartScriptRow
	timer   uint32
	active  bool
	runOnce bool
}

// smartScriptState is the per-creature SmartScript (SmartScript.h): the
// event list, current phase number (default 0 — SmartScript.cpp:51, the
// ctor sets mEventPhase = 0, so a nonzero phase mask matches nothing until
// a SET_EVENT_PHASE arm runs), last invoker, the TALK duration timer that
// fires SMART_EVENT_TEXT_OVER, and the stored-target lists
// (SmartScript::_storedTargets, SmartScript.cpp:108-121).
type smartScriptState struct {
	events         []*smartScriptEvent
	phase          uint32
	nested         int
	linkDepth      int
	lastInvoker    uint64
	defaultFaction uint32
	textTimer      uint32
	useTextTimer   bool
	lastTextID     uint32
	talkerEntry    uint32
	lastTick       time.Time
	firedInit      bool
	storedTargets  map[uint32][]uint64
}

// loadSmartScripts loads the creature-source smart_scripts rows into the
// server store (SmartScriptMgr::LoadSmartScripts). The repo placeholder
// world.db carries zero rows, so this is a no-op there; a populated world DB
// fills the map. GO/instance/area-trigger sources are out of scope. The
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
	for rows.Next() {
		var r smartScriptRow
		var tx, ty, tz, to float64
		if err := rows.Scan(&r.entryOrGUID, &r.sourceType, &r.id, &r.link, &r.eventType, &r.eventPhaseMask, &r.eventChance, &r.eventFlags, &r.eventParam1, &r.eventParam2, &r.eventParam3, &r.eventParam4, &r.eventParam5, &r.actionType, &r.actionParam1, &r.actionParam2, &r.actionParam3, &r.actionParam4, &r.actionParam5, &r.actionParam6, &r.targetType, &r.targetParam1, &r.targetParam2, &r.targetParam3, &r.targetParam4, &tx, &ty, &tz, &to); err != nil {
			continue
		}
		if r.sourceType != 0 {
			continue
		}
		r.targetX, r.targetY, r.targetZ, r.targetO = float32(tx), float32(ty), float32(tz), float32(to)
		row := r
		byEntry[row.entryOrGUID] = append(byEntry[row.entryOrGUID], &row)
	}
	s.smartScriptsMu.Lock()
	s.smartScripts = byEntry
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
	case smartEventAggro, smartEventKill, smartEventDeath, smartEventEvade,
		smartEventReset, smartEventAIInit, smartEventJustCreated,
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
		packet := protocol.NewBuffer(12)
		packet.WriteU32(ev.row.actionParam1)
		packet.WriteU64(motion.GUID)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
	case smartActionCast:
		s.smartActionCast(ctx, motion, st, ev, invokerGUID)
	case smartActionAutoAttack:
		motion.NoMeleeAttack = ev.row.actionParam1 == 0
	case smartActionSetEventPhase:
		oldPhase := st.phase
		st.phase = ev.row.actionParam1
		if oldPhase != st.phase {
			s.smartProcessEvents(ctx, motion, smartEventPhaseChange, invokerGUID, 0, 0)
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
	case smartActionStoreTargetList:
		targets := s.smartTargets(motion, st, ev, invokerGUID)
		guids := make([]uint64, 0, len(targets))
		for _, tgt := range targets {
			guids = append(guids, tgt.guid)
		}
		if st.storedTargets == nil {
			st.storedTargets = make(map[uint32][]uint64)
		}
		st.storedTargets[ev.row.actionParam1] = guids
	case smartActionRemoveUnitFieldBytes1:
		s.smartActionRemoveUnitFieldBytes1(motion, st, ev, invokerGUID)
	default:
		// Residual: the remaining AGGRO-row actions (80, 31, 21, 83, 8,
		// 88, 43, 42, 146, 103, 23, 49, 36, 41, 87, 25, 47, 5, 12, 74,
		// 92, 133, 30) have no Go twin — timed-action lists, summons,
		// react state, mounts, invincibility HP, visibility — and the
		// event still consumed its chance roll and link below, matching
		// C++'s log-and-skip default (SmartScript.cpp:2370-2372).
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
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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

// smartActionRemoveUnitFieldBytes1 is SMART_ACTION_REMOVE_UNIT_FIELD_BYTES_1
// (SmartScript.cpp:1851-1856): RemoveByteFlag(UNIT_FIELD_BYTES_1, type,
// byte1) — param1 is the flag byte, param2 the byte offset. Byte 0 is the
// stand-state byte, which Go models as motion.StandState (all 12 AGGRO
// rows are offset 0, e.g. clearing SIT to stand). Bytes 1-3 (animation
// tier, shapeshift, ...) have no Go model and stay out.
func (s *Server) smartActionRemoveUnitFieldBytes1(motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam2 != 0 {
		return
	}
	targets := s.smartTargets(motion, st, ev, invokerGUID)
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

// smartTarget is one resolved SmartAI target: the GUID plus whether it is
// a player (creature otherwise — GOs never reach the AGGRO-used actions).
type smartTarget struct {
	guid     uint64
	isPlayer bool
}

// smartTargets is SmartScript::GetTargets (SmartScript.cpp:2510-...): the
// target list for the row's target type, resolved against the motion, the
// invoker (falling back to the last invoker, like C++'s scriptTrigger),
// and the stored-target lists. Range scans read the instance motion map
// under motionMu — the same-instance approximation of C++'s grid search
// (C++ searches the creature's grid cell; Go has no grid, so the whole
// instance is the search space). Distances are 3D, matching C++'s
// IsInRange / GetWorldObjectsInDist.
func (s *Server) smartTargets(motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) []smartTarget {
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
	case smartTargetClosestCreature:
		return s.smartClosestCreatureTarget(motion, ev.row.targetParam1, ev.row.targetParam2, ev.row.targetParam3)
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
