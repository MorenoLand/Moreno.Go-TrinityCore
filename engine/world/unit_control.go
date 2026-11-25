package world

import (
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// UNIT_STATE_CONTROLLED (Unit.h:251) is CONFUSED | STUNNED | FLEEING —
// ROOT is not in it, so SetControlled apply CastStops for stun, confuse
// and fear, but not for root.
const unitStateControlled = unitStateStunned | unitStateConfused | unitStateFleeing

// castStopCreature is the CastStop leg Unit::SetControlled (Unit.cpp:11492)
// and Unit::SetStunned (Unit.cpp:11616) share: interrupt the non-melee
// casts, and when no arm reports, clear the plain CastingUntil gate and
// the pending cast (the interrupt helper only clears state for arms it
// reports). Takes no lock across the interrupt itself.
func (s *Server) castStopCreature(m *creatureMotion) {
	if s == nil || m == nil {
		return
	}
	interrupted := s.interruptCreatureNonMeleeCasts(m)
	s.motionMu.Lock()
	if len(interrupted) == 0 {
		m.CastingUntil = time.Time{}
		s.cancelPendingCreatureCast(m)
	}
	s.motionMu.Unlock()
	for _, arm := range interrupted {
		s.sendCreatureInterruptPackets(m, arm)
	}
}

// sendCreatureMeleeAttackStop is the SendMeleeAttackStop leg of
// Unit::SetControlled (Unit.cpp:11528, :11545) for CONFUSED and FLEEING
// apply: SMSG_ATTACK_STOP to the visible set, without dropping the
// victim — C++ only clears the MELEE_ATTACKING bit, the victim stays
// for the clear-side restore. Sent even with no victim:
// SendMeleeAttackStop defaults its victim to null (Unit.h:827).
func (s *Server) sendCreatureMeleeAttackStop(m *creatureMotion) {
	if s == nil || m == nil {
		return
	}
	s.motionMu.Lock()
	victim := m.TargetGUID
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	x, y, z, phase := m.X, m.Y, m.Z, m.PhaseMask
	s.motionMu.Unlock()
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(guid, victim, false), mapID, instanceID, x, y, z, phase)
}

// setControlledCreature mirrors Unit::SetControlled (Unit.cpp:11484-11564)
// for creatures. Apply early-returns on an already-set state, CastStops
// for CONTROLLED states, adds the state bit, then runs the per-state leg
// with its stun gates (ROOT/CONFUSED skip while stunned, FLEEING skips
// while stunned or confused); CONFUSED/FLEEING also stop the melee swing.
// Remove early-returns while the matching MOD_* aura holds the state
// (ROOT additionally while seated on a vehicle or movement-template
// rooted), clears the bit, runs the per-state clear leg, then re-applies
// any state an aura still holds (ApplyControlStatesIfNeeded).
func (s *Server) setControlledCreature(m *creatureMotion, apply bool, state uint32) {
	if s == nil || m == nil {
		return
	}
	if apply {
		if s.creatureUnitStateHas(m, state) {
			return
		}
		if state&unitStateControlled != 0 {
			s.castStopCreature(m)
		}
		s.creatureUnitStateAdd(m, state)
		switch state {
		case unitStateStunned:
			s.setStunnedCreature(m, true)
		case unitStateRoot:
			if !s.creatureUnitStateHas(m, unitStateStunned) {
				s.setRootedCreature(m, true)
			}
		case unitStateConfused:
			if !s.creatureUnitStateHas(m, unitStateStunned) {
				s.sendCreatureMeleeAttackStop(m)
				s.setCreatureConfused(m, true)
			}
		case unitStateFleeing:
			if !s.creatureUnitStateHas(m, unitStateStunned|unitStateConfused) {
				s.sendCreatureMeleeAttackStop(m)
				s.setCreatureFeared(m, true)
			}
		}
		return
	}
	key := creatureAuraKeyForMotion(m)
	switch state {
	case unitStateStunned:
		if s.creatureHasAuraType(key, spellAuraModStun) {
			return
		}
		s.creatureUnitStateClear(m, state)
		s.setStunnedCreature(m, false)
	case unitStateRoot:
		s.motionMu.Lock()
		vehicleGUID, moveRooted := m.VehicleGUID, m.MoveRooted
		s.motionMu.Unlock()
		if s.creatureHasAuraType(key, spellAuraModRoot) || vehicleGUID != 0 || moveRooted {
			return
		}
		s.creatureUnitStateClear(m, state)
		s.setRootedCreature(m, false)
	case unitStateConfused:
		if s.creatureHasAuraType(key, spellAuraModConfuse) {
			return
		}
		s.creatureUnitStateClear(m, state)
		s.setCreatureConfused(m, false)
	case unitStateFleeing:
		if s.creatureHasAuraType(key, spellAuraModFear) {
			return
		}
		s.creatureUnitStateClear(m, state)
		s.setCreatureFeared(m, false)
	default:
		return
	}
	s.applyControlStatesIfNeededCreature(m)
}

// applyControlStatesIfNeededCreature mirrors
// Unit::ApplyControlStatesIfNeeded (Unit.cpp:11566-11580): a unit state
// may already be cleared while its aura still holds it, so each state
// is re-applied when the state bit or the matching aura type is present.
func (s *Server) applyControlStatesIfNeededCreature(m *creatureMotion) {
	if s == nil || m == nil {
		return
	}
	key := creatureAuraKeyForMotion(m)
	if s.creatureUnitStateHas(m, unitStateStunned) || s.creatureHasAuraType(key, spellAuraModStun) {
		s.setStunnedCreature(m, true)
	}
	if s.creatureUnitStateHas(m, unitStateRoot) || s.creatureHasAuraType(key, spellAuraModRoot) {
		s.setRootedCreature(m, true)
	}
	if s.creatureUnitStateHas(m, unitStateConfused) || s.creatureHasAuraType(key, spellAuraModConfuse) {
		s.setCreatureConfused(m, true)
	}
	if s.creatureUnitStateHas(m, unitStateFleeing) || s.creatureHasAuraType(key, spellAuraModFear) {
		s.setCreatureFeared(m, true)
	}
}

// stopCreatureSpline is the StopMoving leg of Unit::SetStunned /
// Unit::SetRooted (Unit.cpp:11600, :11665): halt the in-flight spline
// at the current position (the SMSG_MONSTER_MOVE stop broadcast).
func (s *Server) stopCreatureSpline(m *creatureMotion) {
	if s == nil || m == nil {
		return
	}
	s.motionMu.Lock()
	if !m.Moving {
		s.motionMu.Unlock()
		return
	}
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	x, y, z, phase, transportGUID := m.X, m.Y, m.Z, m.PhaseMask, m.TransportGUID
	m.Moving = false
	m.MoveEnds = time.Time{}
	s.motionMu.Unlock()
	s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z, phase, transportGUID)
}

// setStunnedCreature is the Unit::SetStunned body (Unit.cpp:11582-11647)
// for creatures: the state slot itself is owned by setControlledCreature
// (creatureUnitStateAdd/Clear), this is the flag / target / movement /
// packet leg. Apply clears the visual target, sets UNIT_FLAG_STUNNED,
// stops the spline and sends SMSG_SPLINE_MOVE_ROOT, then CastStops.
// Remove restores the victim target while alive, keeps the flag when
// the charmer/owner player is mounted (a disabled pet interface), and
// sends SMSG_SPLINE_MOVE_UNROOT only while not rooted.
func (s *Server) setStunnedCreature(m *creatureMotion, apply bool) {
	if s == nil || m == nil {
		return
	}
	if apply {
		s.motionMu.Lock()
		if m.RootTimes > 0 {
			m.RootTimes++
		}
		m.UnitFlags |= unitFlagStunned
		m.UnitFlagsKnown = true
		mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
		x, y, z, phase := m.X, m.Y, m.Z, m.PhaseMask
		flags := m.UnitFlags
		s.motionMu.Unlock()
		s.clearCreatureVisualTarget(m)
		s.stopCreatureSpline(m)
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldFlags: flags})
		s.sendCreatureSplineMoveRoot(mapID, instanceID, guid, x, y, z, phase)
		s.castStopCreature(m)
		return
	}
	s.motionMu.Lock()
	alive := m.Health > 0
	victim := uint64(0)
	if alive {
		if m.ThreatMgr != nil {
			victim = m.ThreatMgr.GetCurrentVictim()
		}
		if victim == 0 {
			victim = m.TargetGUID
		}
	}
	ownerGUID := creatureCharmerOrOwnerGUID(m)
	s.motionMu.Unlock()
	// Don't remove UNIT_FLAG_STUNNED for a pet when its owner is mounted
	// (disabled pet interface, Unit.cpp:11624-11626). C++ IsMounted is
	// the UNIT_FLAG_MOUNT flag read (Unit.h:932), not the mounted aura.
	mounted := false
	if ownerGUID != 0 {
		if owner := s.findSessionByGUID(ownerGUID); owner != nil && owner.player != nil &&
			owner.player.UnitFlags&unitFlagMount != 0 {
			mounted = true
		}
	}
	s.motionMu.Lock()
	if !mounted {
		m.UnitFlags &^= unitFlagStunned
		m.UnitFlagsKnown = true
	}
	flags := m.UnitFlags
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	x, y, z, phase := m.X, m.Y, m.Z, m.PhaseMask
	s.motionMu.Unlock()
	if victim != 0 {
		s.motionMu.Lock()
		m.ScriptTargetGUID = victim
		m.ScriptTargetKnown = true
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldTarget: uint32(victim), unitFieldTarget + 1: uint32(victim >> 32)})
	}
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
		map[int]uint32{unitFieldFlags: flags})
	if !s.creatureUnitStateHas(m, unitStateRoot) {
		s.sendCreatureSplineMoveUnroot(mapID, instanceID, guid, x, y, z, phase)
	}
}

// setRootedCreature is the Unit::SetRooted body (Unit.cpp:11649-11699)
// for creatures: the state slot itself is owned by setControlledCreature,
// this is the movement / packet leg. Apply stops the spline and sends
// SMSG_SPLINE_MOVE_ROOT. Remove sends SMSG_SPLINE_MOVE_UNROOT only while
// not stunned.
func (s *Server) setRootedCreature(m *creatureMotion, apply bool) {
	if s == nil || m == nil {
		return
	}
	s.motionMu.Lock()
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	x, y, z, phase := m.X, m.Y, m.Z, m.PhaseMask
	if apply {
		if m.RootTimes > 0 {
			m.RootTimes++
		}
	}
	s.motionMu.Unlock()
	if apply {
		s.stopCreatureSpline(m)
		s.sendCreatureSplineMoveRoot(mapID, instanceID, guid, x, y, z, phase)
		return
	}
	if !s.creatureUnitStateHas(m, unitStateStunned) {
		s.sendCreatureSplineMoveUnroot(mapID, instanceID, guid, x, y, z, phase)
	}
}

// sendForcedRootMovement sends SMSG_FORCE_MOVE_ROOT / UNROOT with the
// m_rootTimes counter (Unit.cpp:11600-11614, :11638-11647): on apply the
// counter increments only while already nonzero, on unroot it always
// increments first, so the first root after connect sends counter 0.
func (s *session) sendForcedRootMovement(opcode uint16, isUnroot bool) {
	if s == nil || s.player == nil {
		return
	}
	if isUnroot || s.rootTimes > 0 {
		s.rootTimes++
	}
	packet := protocol.NewBuffer(12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(s.rootTimes)
	_ = s.write(opcode, packet.Bytes(), true)
}

// setStunnedPlayer is the Unit::SetStunned body (Unit.cpp:11582-11647)
// for players, shared by the SetControlled dispatch and the stun-aura
// apply/remove legs. Apply clears the selection, sets UNIT_FLAG_STUNNED
// and the rooted slot, stands the player up, CastStops, and sends
// SMSG_FORCE_MOVE_ROOT with the counted counter. Remove restores the
// victim selection while alive, clears the flag, and sends
// SMSG_FORCE_MOVE_UNROOT (counter always incremented) only while no
// root aura, script root, or script-mask ROOT bit (Lua AddUnitState)
// still holds the player — the rooted slot itself is not consulted,
// it is the stun's own root (C++ SetStunned gates on HasUnitState(ROOT),
// Unit.cpp:11629).
func (s *session) setStunnedPlayer(apply bool) {
	if s == nil || s.player == nil {
		return
	}
	if apply {
		s.player.Selection = 0
		s.player.UnitFlags |= unitFlagStunned
		s.rooted = true
		s.player.StandState = 0 // UNIT_STAND_STATE_STAND
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
		s.cancelAutoRepeatOnInterrupt()
		s.sendForcedRootMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), false)
		s.sendPlayerUpdate()
		return
	}
	if s.player.Health > 0 && s.attackTarget != 0 {
		s.player.Selection = s.attackTarget
	}
	s.player.UnitFlags &^= unitFlagStunned
	if !s.hasAuraType(spellAuraModRoot) && !s.rootedByScript && s.scriptUnitState&unitStateRoot == 0 {
		s.rooted = false
		s.sendForcedRootMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT), true)
	}
	s.sendPlayerUpdate()
}

// setRootedPlayer is the Unit::SetRooted body (Unit.cpp:11649-11699) for
// players, shared by the SetControlled dispatch and the root-aura
// apply/remove legs. The rootedByScript marker is owned by
// session.setRooted (the Lua/vehicle/sit entry), not here, so the aura
// path never claims a script root. Apply sets the rooted slot and sends
// SMSG_FORCE_MOVE_ROOT with the counted counter. Remove sends
// SMSG_FORCE_MOVE_UNROOT (counter always incremented) only while not
// stunned; while stunned the root is held and the slot stays set until
// the stun clears.
func (s *session) setRootedPlayer(apply bool) {
	if s == nil || s.player == nil {
		return
	}
	if apply {
		s.rooted = true
		s.sendForcedRootMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), false)
		s.sendPlayerUpdate()
		return
	}
	// While stunned the root is held and the slot stays set until the
	// stun clears; while a script root is held the unroot belongs to
	// the script's own clear, not this one.
	if s.playerUnitStateHas(unitStateStunned) || s.rootedByScript {
		return
	}
	s.rooted = false
	s.sendForcedRootMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT), true)
	s.sendPlayerUpdate()
}

// setPlayerControlLeg is the shared player leg of Unit::SetConfused
// (Unit.cpp:11734) and Unit::SetFeared (Unit.cpp:11700): both clear the
// target on apply and, for a player, block/allow control to the real
// player via SetClientControl (Player.cpp:24256, SMSG_CLIENT_CONTROL_UPDATE)
// — the m_playerMovingMe arm, which runs whether or not the mover is
// alive. On clear the victim target is restored only while alive, per
// the C++ IsAlive gate. The confused/flee movement itself is NOT wired:
// players have no server spline model (movement.go: the SPLINE_ENABLED
// strip), so no wander/flee spline can be driven; only the control and
// target legs are faithful here.
func (s *session) setPlayerControlLeg(apply bool) {
	if s == nil || s.player == nil {
		return
	}
	if apply {
		s.player.Selection = 0
		s.sendClientControl(s.playerGUID, false)
		return
	}
	s.sendClientControl(s.playerGUID, true)
	if s.player.Health > 0 && s.attackTarget != 0 {
		s.player.Selection = s.attackTarget
	}
}

// setControlledPlayer mirrors Unit::SetControlled (Unit.cpp:11484-11564)
// for players: the creature twin's dispatch with the player legs
// (SetStunned/SetRooted bodies above, the control/target leg for
// CONFUSED/FLEEING). The vehicle arm of the ROOT remove guard is the
// player VehicleGUID; the movement-template arm has no player twin.
func (s *session) setControlledPlayer(apply bool, state uint32) {
	if s == nil || s.player == nil {
		return
	}
	if apply {
		if s.playerUnitStateHas(state) {
			return
		}
		if state&unitStateControlled != 0 {
			s.interruptCurrentCast()
			s.interruptCurrentChannel()
			s.cancelAutoRepeatOnInterrupt()
		}
		s.playerUnitStateAdd(state)
		switch state {
		case unitStateStunned:
			s.setStunnedPlayer(true)
		case unitStateRoot:
			if !s.playerUnitStateHas(unitStateStunned) {
				s.setRootedPlayer(true)
			}
		case unitStateConfused:
			if !s.playerUnitStateHas(unitStateStunned) {
				_ = s.sendAttackStop(s.attackTarget, false)
				s.setPlayerControlLeg(true)
			}
		case unitStateFleeing:
			if !s.playerUnitStateHas(unitStateStunned | unitStateConfused) {
				_ = s.sendAttackStop(s.attackTarget, false)
				s.setPlayerControlLeg(true)
			}
		}
		return
	}
	switch state {
	case unitStateStunned:
		if s.hasAuraType(spellAuraModStun) {
			return
		}
		s.playerUnitStateClear(state)
		s.setStunnedPlayer(false)
	case unitStateRoot:
		if s.hasAuraType(spellAuraModRoot) || s.player.VehicleGUID != 0 {
			return
		}
		s.playerUnitStateClear(state)
		s.rootedByScript = false
		s.setRootedPlayer(false)
	case unitStateConfused:
		if s.hasAuraType(spellAuraModConfuse) {
			return
		}
		s.playerUnitStateClear(state)
		s.setPlayerControlLeg(false)
	case unitStateFleeing:
		if s.hasAuraType(spellAuraModFear) {
			return
		}
		s.playerUnitStateClear(state)
		s.setPlayerControlLeg(false)
	default:
		return
	}
	s.applyControlStatesIfNeededPlayer()
}

// applyControlStatesIfNeededPlayer mirrors
// Unit::ApplyControlStatesIfNeeded (Unit.cpp:11566-11580) for players.
func (s *session) applyControlStatesIfNeededPlayer() {
	if s == nil || s.player == nil {
		return
	}
	if s.playerUnitStateHas(unitStateStunned) || s.hasAuraType(spellAuraModStun) {
		s.setStunnedPlayer(true)
	}
	if s.playerUnitStateHas(unitStateRoot) || s.hasAuraType(spellAuraModRoot) {
		s.setRootedPlayer(true)
	}
	if s.playerUnitStateHas(unitStateConfused) || s.hasAuraType(spellAuraModConfuse) {
		s.setPlayerControlLeg(true)
	}
	if s.playerUnitStateHas(unitStateFleeing) || s.hasAuraType(spellAuraModFear) {
		s.setPlayerControlLeg(true)
	}
}
