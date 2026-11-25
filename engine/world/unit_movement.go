package world

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Shared Unit:: movement-flag and facing helpers (Unit.cpp:13313-13806).
// The Lua bridges used to inline these bodies once per surface (handler,
// session-creature, SmartAI); the helpers below are the single Unit-level
// twin each bridge now calls, with the C++ early-out and flag transitions.

// creatureSetWalk is Unit::SetWalk (Unit.cpp:13700-13710) as overridden by
// Creature::SetWalk (Creature.cpp:3012-3021): no-op false when unchanged,
// else flip MOVEMENTFLAG_WALKING and broadcast
// SMSG_SPLINE_MOVE_SET_WALK_MODE / SET_RUN_MODE (packed GUID only).
// Returns whether the flag changed.
func (s *Server) creatureSetWalk(motion *creatureMotion, enable bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	if motion.Walking == enable {
		s.motionMu.Unlock()
		return false
	}
	motion.Walking = enable
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
	if enable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	return true
}

// creatureSetSwim is Unit::SetSwim (Unit.cpp:13728-13738): no-op false when
// unchanged, else flip MOVEMENTFLAG_SWIMMING only. C++ itself sends no
// packet for the flip; the Go twin broadcasts
// SMSG_SPLINE_MOVE_START_SWIM / STOP_SWIM (packed GUID only, via
// broadcastCreatureSplineMove) so clients observe the change, the same
// broadcast-leg approach as SetHover/SetCanFly.
func (s *Server) creatureSetSwim(motion *creatureMotion, enable bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	if motion.Swimming == enable {
		s.motionMu.Unlock()
		return false
	}
	motion.Swimming = enable
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_STOP_SWIM
	if enable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_START_SWIM
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	return true
}

// creatureSetCanFly is Unit::SetCanFly (Unit.cpp:13740-13753) as overridden
// by Creature::SetCanFly (Creature.cpp:3060-3071): no-op false when
// unchanged; on enable add MOVEMENTFLAG_CAN_FLY and clear
// MOVEMENTFLAG_FALLING; on disable remove CAN_FLY. The
// MOVEMENTFLAG_MASK_MOVING_FLY disable-clear (Unit.cpp:13751) has no Go
// model — the only fly state is the Flying bool — so the disable leg
// clears that and nothing else. Broadcasts
// SMSG_SPLINE_MOVE_SET_FLYING / UNSET_FLYING (packed GUID only); no
// animation-tier fold (Creature.cpp:3060-3071 carries none).
func (s *Server) creatureSetCanFly(motion *creatureMotion, enable bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	if motion.Flying == enable {
		s.motionMu.Unlock()
		return false
	}
	motion.Flying = enable
	if enable {
		// Unit::SetCanFly(true) clears FALLING (Unit.cpp:13748).
		motion.Falling = false
	}
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_FLYING
	if enable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_FLYING
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	return true
}

// creatureSetDisableGravity is Unit::SetDisableGravity
// (Unit.cpp:13712-13726) as overridden by Creature::SetDisableGravity
// (Creature.cpp:3023-3047): no-op false when unchanged; on enable add
// MOVEMENTFLAG_DISABLE_GRAVITY and clear MOVEMENTFLAG_FALLING; on disable
// remove DISABLE_GRAVITY only. The animation-tier fold
// (Creature.cpp:3030-3037) rides along via creatureFoldAnimationTier, then
// SMSG_SPLINE_MOVE_GRAVITY_DISABLE / ENABLE (packed GUID only,
// Creature.cpp:3040-3045). C++ skips the packet when the move spline is
// uninitialized; the Go movement model has no splines, so the broadcast
// always goes. The packetOnly / updateAnimationTier parameters are ignored
// at Unit level, as in C++.
func (s *Server) creatureSetDisableGravity(motion *creatureMotion, disable bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	if motion.GravityDisabled == disable {
		s.motionMu.Unlock()
		return false
	}
	motion.GravityDisabled = disable
	if disable {
		// Unit::SetDisableGravity(true) clears FALLING (Unit.cpp:13720).
		motion.Falling = false
	}
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	// Creature::SetDisableGravity folds the animation tier before the
	// packet (Creature.cpp:3030-3037, then :3040-3045).
	s.creatureFoldAnimationTier(motion)
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_ENABLE
	if disable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_DISABLE
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	return true
}

// creatureSetFeatherFall is Unit::SetFeatherFall (Unit.cpp:13767-13777) as
// overridden by Creature::SetFeatherFall (Creature.cpp:3099-3110): no-op
// false when unchanged, else flip MOVEMENTFLAG_FALLING_SLOW and broadcast
// SMSG_SPLINE_MOVE_FEATHER_FALL / SMSG_SPLINE_MOVE_NORMAL_FALL (packed GUID only).
// C++ skips the packet when the move spline is uninitialized; the Go
// movement model has no splines, so the broadcast always goes, matching
// the SetSwim broadcast-leg precedent.
func (s *Server) creatureSetFeatherFall(motion *creatureMotion, enable bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	if motion.FeatherFalling == enable {
		s.motionMu.Unlock()
		return false
	}
	motion.FeatherFalling = enable
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_NORMAL_FALL
	if enable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_FEATHER_FALL
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	return true
}

// creatureUpdateOrientation is Unit::UpdateOrientation
// (Unit.cpp:13434-13440): the server-side orientation write only
// (SetOrientation, normalized to [0, 2pi)), then the vehicle-passenger
// relocate when this unit is a vehicle base. No client broadcast.
func (s *Server) creatureUpdateOrientation(motion *creatureMotion, ori float32) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.Orientation = normalizeOrientation(ori)
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	x, y, z, o := motion.X, motion.Y, motion.Z, motion.Orientation
	s.motionMu.Unlock()
	// Unit::UpdateOrientation relocates passengers when the unit is a
	// vehicle (Unit.cpp:13438-13439); relocatePassengers no-ops without
	// a kit, which is the IsVehicle() false leg.
	s.relocatePassengers(mapID, instanceID, guid, x, y, z, o)
}

// creatureUpdateHeight is Unit::UpdateHeight (Unit.cpp:13442-13447): the
// server-side height write only (Relocate with X/Y unchanged), then the
// vehicle-passenger relocate when this unit is a vehicle base. No client
// broadcast.
func (s *Server) creatureUpdateHeight(motion *creatureMotion, newZ float32) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.Z = newZ
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	x, y, z, o := motion.X, motion.Y, motion.Z, motion.Orientation
	s.motionMu.Unlock()
	// Unit::UpdateHeight relocates passengers when the unit is a vehicle
	// (Unit.cpp:13445-13446); relocatePassengers no-ops without a kit.
	s.relocatePassengers(mapID, instanceID, guid, x, y, z, o)
}

// creatureSetInFront is Unit::SetInFront (Unit.cpp:13663-13667): face the
// target's absolute angle (atan2, normalized to [0, 2pi)) with a raw
// orientation-slot write — no packet, no spline, no movement check. The
// UNIT_STATE_CANNOT_TURN gate has no Go model (no unit-state bits are
// stored), so it is not gated here; the inline legs note the same gap.
func (s *Server) creatureSetInFront(motion *creatureMotion, tx, ty float32) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	angle := float32(math.Atan2(float64(ty-motion.Y), float64(tx-motion.X)))
	motion.Orientation = normalizeOrientation(angle)
	s.motionMu.Unlock()
}

// creatureSetFacingTo is Unit::SetFacingTo (Unit.cpp:13669-13684): the
// in-place turn to an orientation — the logical slot write plus the
// in-place broadcastMonsterMove turn packet (start == dest, 1000ms, the
// Final_Angle arm). The C++ IsStopped()/movespline-Finalized() gate has no
// spline model in Go; the Moving/MoveEnds proxy (the swing-tick gate,
// creaturemotion.go:9814) stands in for it, bypassed when force is true.
// Returns whether the turn ran.
func (s *Server) creatureSetFacingTo(motion *creatureMotion, ori float32, force bool) bool {
	if s == nil || motion == nil {
		return false
	}
	ori = normalizeOrientation(ori)
	s.motionMu.Lock()
	if !force && (motion.Moving || time.Now().Before(motion.MoveEnds)) {
		s.motionMu.Unlock()
		return false
	}
	motion.Orientation = ori
	guid, mapID, instID := motion.GUID, motion.Map, motion.InstanceID
	phase := motion.PhaseMask
	transportGUID := motion.TransportGUID
	x, y, z := motion.X, motion.Y, motion.Z
	s.motionMu.Unlock()
	s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, ori, true, phase, transportGUID)
	return true
}

// creatureSetFacingToObject is Unit::SetFacingToObject
// (Unit.cpp:13685-13699): the in-place turn to the absolute angle of a
// target point, via creatureSetFacingTo. The transport leg differs from
// SetFacingTo in C++ (no DisableTransportPathTransformations call); Go has
// no such transform, and both overloads ride the same transport-aware
// monster-move packet, so the distinction is unmodeled.
func (s *Server) creatureSetFacingToObject(motion *creatureMotion, tx, ty float32, force bool) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	mx, my := motion.X, motion.Y
	s.motionMu.Unlock()
	angle := float32(math.Atan2(float64(ty-my), float64(tx-mx)))
	return s.creatureSetFacingTo(motion, angle, force)
}

// unitCanSwimMotion is Unit::CanSwim (Unit.cpp:13318-13330) for a creature
// motion: CANNOT_SWIM denies, PLAYER_CONTROLLED allows, the raw
// UNIT_FIELD_FLAGS_2 0x1000000 bit denies, a pet in combat allows, else the
// RENAME|SWIMMING flags decide. IsPet() reads OwnerGUID != 0 (the
// creatureCanSwimMotion precedent). This is the Unit-level flag chain, not
// Creature::CanSwim (Creature.h:105, template-based) — the two answer
// different questions and must not be conflated.
func unitCanSwimMotion(motion *creatureMotion) bool {
	if motion == nil {
		return false
	}
	if motion.UnitFlags&unitFlagCannotSwim != 0 {
		return false
	}
	if motion.UnitFlags&unitFlagPlayerControlled != 0 {
		return true
	}
	if motion.UnitFlags2&unitFlag2SwimDeny != 0 {
		return false
	}
	if motion.OwnerGUID != 0 && motion.UnitFlags&unitFlagPetInCombat != 0 {
		return true
	}
	return motion.UnitFlags&(unitFlagRename|unitFlagSwimming) != 0
}

// playerCanSwim is Unit::CanSwim (Unit.cpp:13318-13330) for a player: the
// PLAYER_CONTROLLED arm always allows, so only the preceding CANNOT_SWIM
// check can deny (the FLAGS_2 leg is unreachable past the player arm).
// The unit flags are the live UNIT_FIELD_FLAGS words on the player state.
func playerCanSwim(unitFlags uint32) bool {
	return unitFlags&unitFlagCannotSwim == 0
}

// creatureSendClearTarget is Unit::SendClearTarget (Unit.cpp:13574-13579)
// for a creature motion: SMSG_BREAK_TARGET with the unit's own packed GUID,
// broadcast to the visibility set. It rides sendPassengerClearTarget's
// fan-out (100.0 range + phase filter), the general SendMessageToSet twin.
func (s *Server) creatureSendClearTarget(motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.motionMu.Unlock()
	s.sendPassengerClearTarget(mapID, instanceID, guid, x, y, z, phase)
}
