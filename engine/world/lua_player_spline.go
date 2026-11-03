package world

import (
	"math"
	"time"
)

// launchPlayerScriptSpline is the player twin of launchCreatureScriptSpline:
// the atomic-spline launch for script Move* legs. The server snaps to the
// destination at launch and broadcasts SMSG_MONSTER_MOVE through the shared
// instance fan-out (which reaches the mover's own session as well as nearby
// observers, the C++ MoveSpline launch's audience), so clients interpolate
// while the server already holds the destination. Dead players never launch
// (C++ generators never run on a corpse) and a sub-0.001 speed fails
// validation with no spline and no packet (MoveSpline.cpp:210), like the
// creature leg. The genPath / pathfinding arm of MovePoint has no bridge —
// Go has no PathGenerator, so every script spline is straight-line. The
// stamp is time-gated, not cleared, by the halt legs: C++ Unit::StopMoving
// stops the spline but leaves the generator armed, and the creature
// stopCreatureMotion twin likewise never clears ScriptMoveType, so
// GetMovementType reads the stamp only while it is in flight. A player on
// a transport still gets the plain (non-transport) packet form: the
// transport spline arm is unmodeled, matching the taxi launch.
func (s *session) launchPlayerScriptSpline(destX, destY, destZ float32, walk bool, moveType uint32) bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	if s.player.Health == 0 {
		return false
	}
	mt := moveTypeRun
	if walk {
		mt = moveTypeWalk
	}
	speed := s.movementSpeeds()[mt]
	if speed < 0.001 {
		return false
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, destX, destY, destZ)
	duration := splineDurationMs(dist, speed)
	mapID, instanceID, guid := s.player.Map, s.player.InstanceID, s.playerGUID
	phase := s.currentPlayerPhaseMask()
	fromX, fromY, fromZ := s.player.X, s.player.Y, s.player.Z
	s.player.X, s.player.Y, s.player.Z = destX, destY, destZ
	s.playerScriptMoveType = moveType
	s.playerScriptMoveEnds = time.Now().Add(time.Duration(duration) * time.Millisecond)
	s.server.broadcastMonsterMoveInInstance(mapID, instanceID, guid, fromX, fromY, fromZ, destX, destY, destZ, duration, walk, 0, false, phase, 0)
	return true
}

// launchPlayerJumpSpline is the player twin of launchCreatureJumpSpline:
// MotionMaster::MoveJump (MotionMaster.cpp:785-824) via a parabolic spline.
// The Eluna 4th arg drives horizontal speed and the 5th drives the vertical
// profile (the C++ param order, not the Eluna doc names — see the creature
// leg). speedXY below 0.01 launches nothing (MotionMaster.cpp:790). The
// jump stamps EFFECT, like the creature leg; its id is accepted but
// unrecorded — the arrival arm is MovementInform EFFECT, event 28, which Go
// never registered.
func (s *session) launchPlayerJumpSpline(destX, destY, destZ, speedXY, speedZ float32) bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	if s.player.Health == 0 || speedXY < 0.01 {
		return false
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, destX, destY, destZ)
	duration := splineDurationMs(dist, speedXY)
	_, verticalAccel := jumpParabolicParams(speedZ, duration)
	mapID, instanceID, guid := s.player.Map, s.player.InstanceID, s.playerGUID
	phase := s.currentPlayerPhaseMask()
	fromX, fromY, fromZ := s.player.X, s.player.Y, s.player.Z
	s.player.X, s.player.Y, s.player.Z = destX, destY, destZ
	s.playerScriptMoveType = movementGeneratorEffect
	s.playerScriptMoveEnds = time.Now().Add(time.Duration(duration) * time.Millisecond)
	s.server.broadcastMonsterMoveJumpInInstance(mapID, instanceID, guid, fromX, fromY, fromZ, destX, destY, destZ, duration, verticalAccel, phase, 0)
	return true
}

// playerScriptMoveTypeNow reports the in-flight script spline's generator
// type, or false once it has landed. Taxi is not a script spline: callers
// check the flight leg separately.
func (s *session) playerScriptMoveTypeNow() (uint32, bool) {
	if s == nil || s.playerScriptMoveType == 0 || !time.Now().Before(s.playerScriptMoveEnds) {
		return 0, false
	}
	return s.playerScriptMoveType, true
}

// playerFleeDestination mirrors startCreatureFlee's 24yd straight-line
// flight away from the enemy (the C++ FleeingMovementGenerator repaths
// continuously; the single spline is the same documented delta as the
// creature leg). A coincident enemy flees due east.
func playerFleeDestination(fromX, fromY, fromZ, enemyX, enemyY float32) (float32, float32, float32) {
	dx := float64(fromX - enemyX)
	dy := float64(fromY - enemyY)
	dist := math.Hypot(dx, dy)
	if dist < 0.2 {
		dx, dy, dist = 1, 0, 1
	}
	const fleeDist = 24.0
	return fromX + float32(dx/dist*fleeDist), fromY + float32(dy/dist*fleeDist), fromZ
}
