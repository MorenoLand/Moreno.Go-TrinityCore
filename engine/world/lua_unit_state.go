package world

import "time"

// The Eluna trio HasUnitState / AddUnitState / ClearUnitState
// (UnitMethods.h:562-684) is a direct read / OR / AND-NOT over
// Unit::m_state, so HasUnitState reports true when ANY requested bit is
// present (Unit.h:831: (m_state & f) != 0), and Add/Clear are pure bit
// ops with no SetControlled side effects. Go has no m_state mask, so
// the trio is split: a live leg read from the engine model below, plus
// a script mask (motion.ScriptUnitState, session.scriptUnitState)
// holding bits AddUnitState wrote for states with no live model.
// HasUnitState ORs both halves, so a script-written bit reads back true
// exactly like C++; the live half keeps engine-driven states honest (a
// real cast reads CASTING without any script write, a root aura reads
// ROOT). ClearUnitState clears the script half and, for ROOT on
// creatures, the ScriptRooted gate slot (the C++ single-bit
// convergence: SetRooted and AddUnitState(ROOT) write the same bit).
// It cannot clear a live-held leg — a real cast, an aura, an engaged
// melee swing, a running spline — which parallels the C++ SetControlled
// clear guards (Unit.cpp:11502-11540, clear returns early while the
// aura holds the state); scripts needing a real clear use
// InterruptNonMeleeSpells, aura removal, or AttackStop instead. Player
// AddUnitState(ROOT) is mask-only on purpose: the player rooted slot
// drives SMSG_FORCE_MOVE packets C++ AddUnitState never sends; SetRooted
// is the behavior arm. The _MOVE legs read the live generator legs
// only, never the mask: a script-written CONFUSED bit without a moving
// spline is not a confused movement generator.
//
// UNIT_STATE_* bit values (Unit.h:209-241, Trinity). Ten bits already
// live in conditions.go (Died, MeleeAttacking, Charmed, Stunned, Fleeing,
// InFlight, Root, Confused, Casting, Move) for the CONDITION_UNIT_STATE
// arm; the rest are defined here so the trio covers the full enum.
const (
	unitStateRoaming             = 0x00000010
	unitStateChase               = 0x00000020
	unitStateFocusing            = 0x00000040
	unitStateFollow              = 0x00000200
	unitStateDistracted          = 0x00001000
	unitStateIsolated            = 0x00002000
	unitStateAttackPlayer        = 0x00004000
	unitStatePossessed           = 0x00010000
	unitStateCharging            = 0x00020000
	unitStateJumping             = 0x00040000
	unitStateFollowFormation     = 0x00080000
	unitStateRotating            = 0x00200000
	unitStateEvade               = 0x00400000
	unitStateRoamingMove         = 0x00800000
	unitStateConfusedMove        = 0x01000000
	unitStateFleeingMove         = 0x02000000
	unitStateChaseMove           = 0x04000000
	unitStateFollowMove          = 0x08000000
	unitStateIgnorePathfinding   = 0x10000000
	unitStateFollowFormationMove = 0x20000000
)

// unitStateFacts is the live-model half of the m_state mask, gathered
// by each surface under its own locks. Unlisted states have no live
// leg and read from the script mask only:
//   - DIED (fake-death aura): no feign-death model on either surface.
//   - ROAMING / FOLLOW / FOLLOW_MOVE / ROAMING_MOVE: no live generator
//     tracking; motion.MoveType is the spawn template (1 random,
//     2 waypoint), not the running generator, and ChaseTX/ChaseTY are
//     a last-position record that is never cleared, so CHASE and
//     CHASE_MOVE are mask-only too (a chase that ended would read
//     stuck-true from the record).
//   - FOCUSING / ISOLATED / CHARGING / JUMPING / ROTATING /
//     IGNORE_PATHFINDING: no model on either surface.
//   - ATTACK_PLAYER (contested PvP, Player.cpp:20727 SetContestedPvP):
//     no contested-PvP model; mapping it to "victim is a player" would
//     misreport ordinary PvE aggro, which never sets it in C++.
//   - IN_FLIGHT on creatures: player-only flight mode; the creature
//     Flying flag is a movement flag, not a taxi.
//   - Player MOVE / EVADE / DISTRACTED / CHASE / FOLLOW_FORMATION and
//     their _MOVE legs: player movement is client-driven, with no
//     server generator state to read.
type unitStateFacts struct {
	Casting         bool
	Rooted          bool
	Stunned         bool
	Confused        bool
	Fleeing         bool
	MeleeAttacking  bool
	Charmed         bool
	Possessed       bool
	Distracted      bool
	Moving          bool
	Evading         bool
	InFlight        bool
	FollowFormation bool
}

// unitStateLiveMask folds live facts into m_state bits. The _MOVE legs
// pair the live state with the running spline, never the script mask.
func unitStateLiveMask(f unitStateFacts) uint32 {
	var mask uint32
	if f.Casting {
		mask |= unitStateCasting
	}
	if f.Rooted {
		mask |= unitStateRoot
	}
	if f.Stunned {
		mask |= unitStateStunned
	}
	if f.Confused {
		mask |= unitStateConfused
	}
	if f.Fleeing {
		mask |= unitStateFleeing
	}
	if f.MeleeAttacking {
		mask |= unitStateMeleeAttacking
	}
	if f.Charmed {
		mask |= unitStateCharmed
	}
	if f.Possessed {
		mask |= unitStatePossessed
	}
	if f.Distracted {
		mask |= unitStateDistracted
	}
	if f.Moving {
		mask |= unitStateMove
	}
	if f.Evading {
		mask |= unitStateEvade
	}
	if f.InFlight {
		mask |= unitStateInFlight
	}
	if f.FollowFormation {
		mask |= unitStateFollowFormation
	}
	if f.Fleeing && f.Moving {
		mask |= unitStateFleeingMove
	}
	if f.Confused && f.Moving {
		mask |= unitStateConfusedMove
	}
	if f.FollowFormation && f.Moving {
		mask |= unitStateFollowFormationMove
	}
	return mask
}

// creatureUnitStateHas is the creature half of HasUnitState. Aura reads
// lock auraMu internally and release before motionMu is taken, so no
// lock order is nested. The live legs: CASTING from the CastingUntil
// window the combat tick gates on (the pending-inline window stamps
// CastingUntil at schedule, so no second read is needed); ROOT from
// ScriptRooted or a MOD_ROOT aura; STUNNED / CONFUSED / FLEEING from
// their MOD_* auras (the victimIsControlled 5/7/12 set) plus the
// FleeingUntil / Fleeing legs for the timed and untimed fear
// generators and the Confused latch for the script confused
// generator; MELEE_ATTACKING from the engaged combat pair
// (Unit::Attack, Unit.cpp:5691-5723, sets it on melee engage); CHARMED
// from MOD_CHARM / MOD_AOE_CHARM; POSSESSED from MOD_POSSESS;
// DISTRACTED from the EffectDistract hold; MOVE / EVADE from the
// spline and evade flags; FOLLOW_FORMATION from the formation pull.
func (s *Server) creatureUnitStateHas(m *creatureMotion, requested uint32) bool {
	if s == nil || m == nil {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	stunned := s.creatureHasAuraType(key, spellAuraModStun)
	confused := s.creatureHasAuraType(key, spellAuraModConfuse)
	feared := s.creatureHasAuraType(key, spellAuraModFear)
	rootedAura := s.creatureHasAuraType(key, spellAuraModRoot)
	charmed := s.creatureHasAuraType(key, spellAuraCharm) || s.creatureHasAuraType(key, spellAuraAoeCharm)
	possessed := s.creatureHasAuraType(key, spellAuraModPossess)
	now := time.Now()
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	facts := unitStateFacts{
		Casting:         now.Before(m.CastingUntil),
		Rooted:          m.ScriptRooted || rootedAura,
		Stunned:         stunned,
		Confused:        confused || m.Confused,
		Fleeing:         feared || now.Before(m.FleeingUntil) || m.Fleeing,
		MeleeAttacking:  m.InCombat && m.TargetGUID != 0,
		Charmed:         charmed,
		Possessed:       possessed,
		Distracted:      now.Before(m.DistractedUntil),
		Moving:          m.Moving,
		Evading:         m.Evading,
		FollowFormation: m.FormationActive,
	}
	return (unitStateLiveMask(facts)|m.ScriptUnitState)&requested != 0
}

// creatureUnitStateAdd is the creature half of AddUnitState: a pure
// mask OR, plus the ROOT gate slot (see the file header: the C++
// single-bit convergence for SetRooted / AddUnitState(ROOT)).
func (s *Server) creatureUnitStateAdd(m *creatureMotion, add uint32) {
	if s == nil || m == nil {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	m.ScriptUnitState |= add
	if add&unitStateRoot != 0 {
		m.ScriptRooted = true
	}
}

// creatureUnitStateClear is the creature half of ClearUnitState: a pure
// mask AND-NOT, plus the ROOT gate slot clear.
func (s *Server) creatureUnitStateClear(m *creatureMotion, clear uint32) {
	if s == nil || m == nil {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	m.ScriptUnitState &^= clear
	if clear&unitStateRoot != 0 {
		m.ScriptRooted = false
	}
}

// playerUnitStateLive is the player half of the live leg: CASTING from
// s.activeCast (the IsCasting twin); ROOT from the session rooted slot
// or a MOD_ROOT aura; STUNNED / CONFUSED / FLEEING / CHARMED /
// POSSESSED from their MOD_* auras; MELEE_ATTACKING from the session
// attack target; IN_FLIGHT from the live TaxiPath flag (the in-tree
// flight gate combat.go uses). The session has no spline, formation,
// evade, or distract state, so those legs stay mask-only.
func (s *session) playerUnitStateLive() uint32 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	casting := s.activeCast != nil
	s.castMu.Unlock()
	facts := unitStateFacts{
		Casting:        casting,
		Rooted:         s.rooted || s.hasAuraType(spellAuraModRoot),
		Stunned:        s.hasAuraType(spellAuraModStun),
		Confused:       s.hasAuraType(spellAuraModConfuse),
		Fleeing:        s.hasAuraType(spellAuraModFear),
		MeleeAttacking: s.attackTarget != 0,
		Charmed:        s.hasAuraType(spellAuraCharm) || s.hasAuraType(spellAuraAoeCharm),
		Possessed:      s.hasAuraType(spellAuraModPossess),
		InFlight:       s.player != nil && s.player.TaxiPath != "",
	}
	return unitStateLiveMask(facts)
}

// playerUnitStateHas / playerUnitStateAdd / playerUnitStateClear are the
// player halves of the trio; Add/Clear touch only the session mask
// (see the file header for why player ROOT writes no live slot).
func (s *session) playerUnitStateHas(requested uint32) bool {
	if s == nil {
		return false
	}
	return (s.playerUnitStateLive()|s.scriptUnitState)&requested != 0
}

func (s *session) playerUnitStateAdd(add uint32) {
	if s == nil {
		return
	}
	s.scriptUnitState |= add
}

func (s *session) playerUnitStateClear(clear uint32) {
	if s == nil {
		return
	}
	s.scriptUnitState &^= clear
}
