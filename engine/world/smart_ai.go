// SmartAI engine — first slice (item-6 census Axis1#2).
//
// C++: SmartScript / SmartAI (src/server/game/AI/SmartScripts/SmartScript.cpp,
// SmartAI.cpp, SmartScriptMgr.h). Go had no SmartAI engine and no smart_scripts
// loader (the WORLD_SEL_SMART_SCRIPTS statement existed unused, the .reload
// arm was a stub), so every SmartAI creature's on-aggro script was silently
// dead. This slice loads the creature-source rows (source_type 0), keeps
// per-creature runtime state on the motion, and dispatches the direct events
// (AGGRO, DEATH, AI_INIT, JUST_CREATED, TEXT_OVER, EVENT_PHASE_CHANGE) plus
// the timer events (UPDATE, UPDATE_IC, UPDATE_OOC, HEALTH_PCT,
// TARGET_HEALTH_PCT, MANA_PCT, TARGET_MANA_PCT, RANGE) from the creature
// tick. Action coverage is the slice-1 set: TALK, SET_FACTION, PLAY_EMOTE,
// CAST, AUTO_ATTACK, SET_EVENT_PHASE. Everything else (SUMMON_CREATURE,
// timed-action lists, GO/instance/area-trigger sources, .reload refresh,
// the charmed gate, RESET wiring, condition checks) is a documented residual
// in smartProcessAction / the call sites below, not a silent gap.
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
	smartEventTargetHealthPc uint32 = 12
	smartEventTargetManaPct  uint32 = 18
	smartEventAIInit         uint32 = 37
	smartEventTextOver       uint32 = 52
	smartEventUpdate         uint32 = 60
	smartEventLink           uint32 = 61
	smartEventJustCreated    uint32 = 63
	smartEventPhaseChange    uint32 = 66
)

// SMART_ACTION ids (SmartScriptMgr.h:457-600), slice-1 set.
const (
	smartActionTalk          uint32 = 1
	smartActionSetFaction    uint32 = 2
	smartActionPlayEmote     uint32 = 5
	smartActionCast          uint32 = 11
	smartActionAutoAttack    uint32 = 20
	smartActionSetEventPhase uint32 = 22
)

// SMART_TARGET ids (SmartScriptMgr.h:1221-1228).
const (
	smartTargetSelf          uint32 = 1
	smartTargetVictim        uint32 = 2
	smartTargetActionInvoker uint32 = 7
)

// SMART_EVENT_FLAG bits (SmartScriptMgr.h:1522-1531).
const (
	smartEventFlagNotRepeatable uint32 = 0x001
	smartEventFlagWhileCharmed  uint32 = 0x200
)

// SMARTCAST flags (SmartScriptMgr.h:1539-1540).
const (
	smartCastInterruptPrevious uint32 = 0x01
	smartCastTriggered         uint32 = 0x02
)

// SMART_EVENT_PHASE bits (SmartScriptMgr.h:61-78): phase 1 = bit 1, ...,
// phase 12 = bit 2048, all = 4095. SetPhase stores the phase NUMBER verbatim
// (SmartScript.cpp:4010-4014) and IsInPhase (:4020-4025) tests
// (1 << (phase-1)) & mask.
const smartEventPhaseAll uint32 = 4095

const smartMaxNestedEvents = 10

// smartScriptRow is one smart_scripts row (SmartScriptHolder: SmartScriptMgr.h).
type smartScriptRow struct {
	entryOrGUID    uint32
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
// event list, current phase number (default 1), last invoker, and the
// TALK duration timer that fires SMART_EVENT_TEXT_OVER.
type smartScriptState struct {
	events         []*smartScriptEvent
	phase          uint32
	nested         int
	lastInvoker    uint64
	defaultFaction uint32
	textTimer      uint32
	useTextTimer   bool
	lastTextID     uint32
	talkerEntry    uint32
	lastTick       time.Time
	firedInit      bool
}

// loadSmartScripts loads the creature-source smart_scripts rows into the
// server store (SmartScriptMgr::LoadSmartScripts). The repo placeholder
// world.db carries zero rows, so this is a no-op there; a populated world DB
// fills the map. GO/instance/area-trigger sources are out of slice 1.
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
	byEntry := make(map[uint32][]*smartScriptRow)
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
}

func (s *Server) smartRowsFor(entry uint32) []*smartScriptRow {
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
	rows := s.smartRowsFor(motion.Entry)
	if len(rows) == 0 {
		return nil
	}
	st := &smartScriptState{phase: 1, defaultFaction: motion.Faction}
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
// Conditions (sConditionMgr) have no Go analog and are skipped.
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
	for _, ev := range st.events {
		if ev.row.eventType == smartEventLink {
			continue
		}
		if ev.row.eventType == eventType {
			s.smartProcessEvent(ctx, motion, st, ev, invokerGUID, var0, var1)
		}
	}
	st.nested--
}

// smartProcessEvent is SmartScript::ProcessEvent (:2882-...): the active,
// phase-mask and NOT_REPEATABLE gates, then the per-type dispatch — direct
// events run their action, timed events run ProcessTimedAction.
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
	// Residual: the WHILE_CHARMED gate (SmartScript.cpp:2890) needs the
	// charmed leg; charmed SmartAI creatures are rare and the gate is
	// skipped rather than guessed.
	switch ev.row.eventType {
	case smartEventLink:
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
// re-arm on the repeat window, then run the action.
func (s *Server) smartProcessTimedAction(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, min, max uint32, invokerGUID uint64, var0, var1 uint32) {
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

// smartProcessAction is SmartScript::ProcessAction (:255-2379), slice-1
// action set. Chance: a 0 chance always fires (SmartScript.cpp:256-260);
// runOnce marks the event for the NOT_REPEATABLE gate.
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
		s.smartActionTalk(ctx, motion, st, ev)
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
	case smartActionPlayEmote:
		packet := protocol.NewBuffer(12)
		packet.WriteU32(ev.row.actionParam1)
		packet.WriteU64(motion.GUID)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
	case smartActionCast:
		targetGUID := uint64(0)
		switch ev.row.targetType {
		case smartTargetSelf:
			targetGUID = motion.GUID
		case smartTargetVictim:
			targetGUID = motion.TargetGUID
		case smartTargetActionInvoker:
			targetGUID = invokerGUID
			if targetGUID == 0 {
				targetGUID = st.lastInvoker
			}
		}
		if targetGUID == 0 {
			return
		}
		if ev.row.actionParam3&smartCastTriggered != 0 {
			s.castCreatureSpellTriggered(ctx, motion, ev.row.actionParam1, targetGUID)
		} else {
			s.castCreatureSpell(ctx, motion, ev.row.actionParam1, targetGUID)
		}
	case smartActionAutoAttack:
		motion.NoMeleeAttack = ev.row.actionParam1 == 0
	case smartActionSetEventPhase:
		oldPhase := st.phase
		st.phase = ev.row.actionParam1
		if oldPhase != st.phase {
			s.smartProcessEvents(ctx, motion, smartEventPhaseChange, invokerGUID, 0, 0)
		}
	default:
		// Residual: SUMMON_CREATURE, SOUND, MORPH, TELEPORT, and the
		// remaining ~130 actions have no slice-1 twin; the event still
		// consumed its chance roll and link above.
	}
	if ev.row.link != 0 && ev.row.link != ev.row.id {
		for _, linked := range st.events {
			if linked.row.id == ev.row.link && linked.row.eventType == smartEventLink {
				s.smartProcessEvent(ctx, motion, st, linked, invokerGUID, var0, var1)
				break
			}
		}
	}
}

// smartActionTalk is SMART_ACTION_TALK (SmartScript.cpp:260-294): the
// creature_text group plays via broadcastCreatureTalk; the duration arms
// SMART_EVENT_TEXT_OVER from the tick. useTalkTarget whispers the invoker.
func (s *Server) smartActionTalk(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent) {
	whisperTarget := uint64(0)
	if ev.row.actionParam3 != 0 {
		whisperTarget = st.lastInvoker
	}
	name := ""
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(name, '') FROM creature_template WHERE entry = ?`, motion.Entry).Scan(&name)
	}
	st.talkerEntry = motion.Entry
	st.lastTextID = ev.row.actionParam1
	st.textTimer = ev.row.actionParam2
	st.useTextTimer = true
	s.broadcastCreatureTalk(ctx, motion.Map, motion.InstanceID, motion.GUID, motion.Entry, name, uint8(ev.row.actionParam1), whisperTarget)
}
