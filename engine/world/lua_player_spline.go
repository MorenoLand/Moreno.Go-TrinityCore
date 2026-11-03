package world

import (
	"math"
	"math/rand"
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

// clearPlayerScriptMoveSlots drops the MoveChase / MoveFollow /
// MoveConfused ACTIVE slots (MotionMaster::Clear, the MoveClear /
// MoveExpire twin, and the ACTIVE-replacement legs of MoveTo,
// MoveJump and MoveFleeing). It never touches the in-flight spline
// stamp: Clear removes generators, not the spline in motion, the
// creature clearScriptChaseFollow precedent.
func (s *session) clearPlayerScriptMoveSlots() {
	if s == nil {
		return
	}
	s.playerChaseTargetGUID = 0
	s.playerChaseDist = 0
	s.playerChaseAngle = 0
	s.playerFollowTargetGUID = 0
	s.playerFollowDist = 0
	s.playerFollowAngle = 0
	s.playerConfused = false
	s.playerConfusedWaitUntil = time.Time{}
}

// playerScriptTarget resolves a MoveChase / MoveFollow target GUID to
// a live unit position: another session first (findSessionByGUID),
// then a creature motion in the player's map/instance — the
// scriptFollowTarget resolution order, minus its sweep list. A
// self-target never resolves (MotionMaster::MoveFollow / MoveChase
// ignore target == owner), and an unresolvable target means the
// generator finalizes (the caller clears its slot).
func (s *session) playerScriptTarget(guid uint64) (x, y, z, orientation, reach float32, ok bool) {
	if s == nil || s.server == nil || s.player == nil || guid == 0 || guid == s.playerGUID {
		return 0, 0, 0, 0, 0, false
	}
	if sess := s.server.findSessionByGUID(guid); sess != nil && sess.player != nil {
		reach = sess.player.CombatReach
		if reach <= 0 {
			reach = 1.5
		}
		return sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, reach, true
	}
	if cm := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, guid); cm != nil {
		reach = cm.CombatReach
		if reach <= 0 {
			reach = 1.5
		}
		return cm.X, cm.Y, cm.Z, cm.Orientation, reach, true
	}
	return 0, 0, 0, 0, 0, false
}

// playerScriptMoveNotMove is the Chase / Confused generator pause:
// UNIT_STATE_NOT_MOVE (root | stun) or a movement-preventing cast
// (ChaseMovementGenerator.cpp:98-106,
// ConfusedMovementGenerator.cpp:DoUpdate). The slot stays armed
// while paused; the tick retries once the gate clears.
func (s *session) playerScriptMoveNotMove() bool {
	if s == nil {
		return true
	}
	if s.rooted || s.hasAuraType(spellAuraModStun) {
		return true
	}
	s.castMu.Lock()
	casting := s.activeCast != nil
	s.castMu.Unlock()
	return casting
}

// updatePlayerScriptMovement steps every armed player script-move
// slot from the 100ms world tick (runWorldTick) — the player
// movement tick the parked remainder was waiting on. Sessions
// leave s.sessions on disconnect, so no step can outlive the
// session the way a timer loop could.
func (s *Server) updatePlayerScriptMovement(now time.Time) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		sess.stepPlayerScriptMovement(now)
	}
}

// stepPlayerScriptMovement is one world-tick step for a single
// player. Death finalizes the generators (Chase / Confused Update
// return false on a dead owner); a taxi flight owns movement, so
// the slots wait it out instead of fighting it. Slots are
// mutually exclusive (arming one clears the others), so the
// chain order only mirrors the creature GetMovementType
// precedence (confused, chase, follow).
func (s *session) stepPlayerScriptMovement(now time.Time) {
	if s == nil || s.player == nil {
		return
	}
	if s.player.Health == 0 {
		s.clearPlayerScriptMoveSlots()
		return
	}
	if s.player.TaxiPath != "" {
		return
	}
	switch {
	case s.playerConfused:
		s.stepPlayerConfused(now)
	case s.playerChaseTargetGUID != 0:
		s.stepPlayerChase()
	case s.playerFollowTargetGUID != 0:
		s.stepPlayerFollow()
	}
}

// stepPlayerChase is ChaseMovementGenerator::Update for a script
// MoveChase slot (ChaseMovementGenerator.cpp:94-230), on the
// creature combat-pursuit leg's script-chase arm: the slot moves
// nothing while the target is not the victim (HasLostTarget —
// owner->GetVictim() != target, :29-32), so a chase armed outside
// combat stays paused but armed, and GetMovementType still
// reports CHASE. Inside the stop ring (dist + hitboxSum) the
// player stands; outside it the spline repaths to the near
// point on the target's absolute angle (orientation + relative
// angle), at run speed, straight-line (no PathGenerator).
func (s *session) stepPlayerChase() {
	tx, ty, tz, tOrient, tReach, ok := s.playerScriptTarget(s.playerChaseTargetGUID)
	if !ok {
		s.playerChaseTargetGUID = 0
		s.playerChaseDist = 0
		s.playerChaseAngle = 0
		return
	}
	if s.playerChaseTargetGUID != s.attackTarget {
		return
	}
	if s.playerScriptMoveNotMove() {
		return
	}
	ownerReach := s.player.CombatReach
	if ownerReach <= 0 {
		ownerReach = 1.5
	}
	shorten := s.playerChaseDist + ownerReach + tReach
	dist := float32(distance3D(s.player.X, s.player.Y, s.player.Z, tx, ty, tz))
	if dist <= shorten {
		return
	}
	absAngle := normalizeScriptAngle(tOrient + s.playerChaseAngle)
	destX := tx + shorten*float32(math.Cos(float64(absAngle)))
	destY := ty + shorten*float32(math.Sin(float64(absAngle)))
	s.launchPlayerScriptSpline(destX, destY, tz, false, movementGeneratorChase)
}

// stepPlayerFollow is FollowMovementGenerator::DoUpdate for a
// script MoveFollow slot (FollowMovementGenerator.cpp:47-177),
// on the creature stepScriptFollow leg: inside range + both
// reaches + FOLLOW_RANGE_TOLERANCE (1.0) the player stands;
// past it the spline repaths to the near point (range + both
// reaches along the tolerance-band angle pick) at run speed —
// walk follows the target's walk state in C++, but Go has no
// target-walk signal, the creature follow delta. The in-range
// MovementInform (FOLLOW_MOTION_TYPE) has no hook (event 28
// unregistered).
func (s *session) stepPlayerFollow() {
	tx, ty, tz, tOrient, tReach, ok := s.playerScriptTarget(s.playerFollowTargetGUID)
	if !ok {
		s.playerFollowTargetGUID = 0
		s.playerFollowDist = 0
		s.playerFollowAngle = 0
		return
	}
	if s.playerScriptMoveNotMove() {
		return
	}
	ownerReach := s.player.CombatReach
	if ownerReach <= 0 {
		ownerReach = 1.5
	}
	reachSum := ownerReach + tReach
	dist := float32(distance3D(s.player.X, s.player.Y, s.player.Z, tx, ty, tz))
	if dist <= reachSum+s.playerFollowDist+1.0 {
		return
	}
	curRel := normalizeScriptAngle(float32(math.Atan2(float64(s.player.Y-ty), float64(s.player.X-tx))) - tOrient)
	tAngle := scriptFollowAngle(curRel, s.playerFollowAngle)
	absAngle := normalizeScriptAngle(tOrient + tAngle)
	radius := reachSum + s.playerFollowDist
	destX := tx + radius*float32(math.Cos(float64(absAngle)))
	destY := ty + radius*float32(math.Sin(float64(absAngle)))
	s.launchPlayerScriptSpline(destX, destY, tz, false, movementGeneratorFollow)
}

// stepPlayerConfused is ConfusedMovementGenerator::DoUpdate for a
// script MoveConfused slot (ConfusedMovementGenerator.cpp): one
// hop per timer expiry — 4*frand-2 yards along a random angle,
// walk, then a 800-1500ms pause stamped on
// playerConfusedWaitUntil (the creature launchCreatureConfusedSpline
// arm). LOS and pathfinding re-checks have no bridge (no
// PathGenerator). A failed launch retries after 500ms instead
// of hot-looping the tick.
func (s *session) stepPlayerConfused(now time.Time) {
	if now.Before(s.playerConfusedWaitUntil) {
		return
	}
	if s.playerScriptMoveNotMove() {
		return
	}
	angle := rand.Float64() * 2 * math.Pi
	dist := 4*rand.Float64() - 2
	destX := s.player.X + float32(dist*math.Cos(angle))
	destY := s.player.Y + float32(dist*math.Sin(angle))
	if !s.launchPlayerScriptSpline(destX, destY, s.player.Z, true, movementGeneratorConfused) {
		s.playerConfusedWaitUntil = now.Add(500 * time.Millisecond)
		return
	}
	s.playerConfusedWaitUntil = s.playerScriptMoveEnds.Add(time.Duration(800+rand.Intn(701)) * time.Millisecond)
}

// playerScriptMovementType derives the armed-slot type for
// GetMovementType, in the creature creatureMovementType
// precedence (confused, chase, follow); 0 when no slot is armed.
func (s *session) playerScriptMovementType() uint32 {
	if s == nil {
		return 0
	}
	switch {
	case s.playerConfused:
		return movementGeneratorConfused
	case s.playerChaseTargetGUID != 0:
		return movementGeneratorChase
	case s.playerFollowTargetGUID != 0:
		return movementGeneratorFollow
	default:
		return 0
	}
}
