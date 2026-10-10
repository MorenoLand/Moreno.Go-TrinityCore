package world

import (
	"context"
	"math"
	"time"
)

// This file bridges the creature formation system: FormationMgr /
// CreatureGroup (src/server/game/Entities/Creature/CreatureGroups.cpp) and
// FormationMovementGenerator
// (src/server/game/Movement/MovementGenerators/FormationMovementGenerator.cpp).
// creature_formations rows previously had a write path only (the
// WORLD_INS_CREATURE_FORMATION statement); nothing ever read them.

const flagIdleInFormation = 0x200 // GroupAIFlags::FLAG_IDLE_IN_FORMATION (CreatureGroups.h): member follows the leader when pathing idly

type formationRow struct {
	leaderGUID uint32
	dist       float32
	angle      float32
	groupAI    uint32
	point1     uint32
	point2     uint32
}

// ensureFormationsLoaded mirrors FormationMgr::LoadCreatureFormations
// (CreatureGroups.cpp:101-135): SELECT leaderGUID, memberGUID, dist, angle,
// groupAI, point_1, point_2 FROM creature_formations ORDER BY leaderGUID.
// Runs once; a missing table or missing point_1/point_2 columns falls back
// to the 5-column shape rather than failing the motion tick.
func (s *Server) ensureFormationsLoaded(ctx context.Context) {
	s.formationMu.RLock()
	if s.formationLoaded {
		s.formationMu.RUnlock()
		return
	}
	s.formationMu.RUnlock()

	s.formationMu.Lock()
	defer s.formationMu.Unlock()
	if s.formationLoaded {
		return
	}
	s.formationLoaded = true
	s.formationRows = make(map[uint32]formationRow)
	s.formationLeaders = make(map[uint32][]uint32)
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT leaderGUID, memberGUID, dist, angle, groupAI, point_1, point_2 FROM creature_formations ORDER BY leaderGUID")
	wide := true
	if err != nil {
		wide = false
		rows, err = s.WorldStore.DB.QueryContext(ctx, "SELECT leaderGUID, memberGUID, dist, angle, groupAI FROM creature_formations ORDER BY leaderGUID")
		if err != nil {
			return
		}
	}
	defer rows.Close()
	for rows.Next() {
		var leaderGUID, memberGUID, groupAI, point1, point2 uint64
		var dist, angle float64
		var scanErr error
		if wide {
			scanErr = rows.Scan(&leaderGUID, &memberGUID, &dist, &angle, &groupAI, &point1, &point2)
		} else {
			scanErr = rows.Scan(&leaderGUID, &memberGUID, &dist, &angle, &groupAI)
		}
		if scanErr != nil {
			continue
		}
		low := uint32(memberGUID)
		s.formationRows[low] = formationRow{leaderGUID: uint32(leaderGUID), dist: float32(dist), angle: float32(angle), groupAI: uint32(groupAI), point1: uint32(point1), point2: uint32(point2)}
		s.formationLeaders[uint32(leaderGUID)] = append(s.formationLeaders[uint32(leaderGUID)], low)
	}
}

func (s *Server) formationRowFor(memberLow uint32) (formationRow, bool) {
	s.formationMu.RLock()
	defer s.formationMu.RUnlock()
	row, ok := s.formationRows[memberLow]
	return row, ok
}

func (s *Server) addFormationRow(memberLow uint32, row formationRow) {
	s.formationMu.Lock()
	defer s.formationMu.Unlock()
	if s.formationRows == nil {
		s.formationRows = make(map[uint32]formationRow)
		s.formationLeaders = make(map[uint32][]uint32)
	}
	s.formationRows[memberLow] = row
	s.formationLeaders[row.leaderGUID] = append(s.formationLeaders[row.leaderGUID], memberLow)
}

// formationLeaderStartedMoving mirrors CreatureGroup::LeaderStartedMoving
// (CreatureGroups.cpp:280-295), reached via Creature::SignalFormationMovement
// (Creature.cpp:362-369) from the point/wander/waypoint generators. After a
// leader launches an idle-path leg, each live member row with
// FLAG_IDLE_IN_FORMATION whose creature is alive and not engaged joins
// formation movement (MoveFormation). The slot angle is inverted
// (FollowAngle + pi) per the C++ comment; CanLeaderStartMoving (the leader
// waiting for members) has no Go model.
func (s *Server) formationLeaderStartedMoving(ctx context.Context, motion *creatureMotion, now time.Time) {
	s.ensureFormationsLoaded(ctx)
	s.formationMu.RLock()
	memberLows := append([]uint32(nil), s.formationLeaders[uint32(motion.GUID&0x00FFFFFF)]...)
	rows := make(map[uint32]formationRow, len(memberLows))
	for _, low := range memberLows {
		if row, ok := s.formationRows[low]; ok {
			rows[low] = row
		}
	}
	s.formationMu.RUnlock()
	if len(memberLows) == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motions := s.motionMapLocked(motion.Map, motion.InstanceID)
	for _, low := range memberLows {
		row, ok := rows[low]
		if !ok || row.groupAI&flagIdleInFormation == 0 {
			continue
		}
		var member *creatureMotion
		for guid, m := range motions {
			if uint32(guid&0x00FFFFFF) == low && m != motion {
				member = m
				break
			}
		}
		if member == nil || member.Health == 0 || member.InCombat || member.FormationActive {
			continue
		}
		member.FormationActive = true
		member.FormationLeadGUID = motion.GUID
		member.FormationDist = row.dist
		member.FormationAngle = row.angle + float32(math.Pi)
		member.FormationPoint1 = row.point1
		member.FormationPoint2 = row.point2
		member.FormationLeaderMoveEnds = motion.MoveEnds
		member.FormationLeaderX, member.FormationLeaderY = motion.X, motion.Y
		member.FormationNextCheck = now.Add(1200 * time.Millisecond)
		s.launchFormationLeg(member, motion, now)
	}
}

// stepFormationMember runs one FormationMovementGenerator::DoUpdate tick for
// a member pulled into formation. It returns true when the member's own
// wander/waypoint legs stay suppressed for this tick. The C++ generator
// replaces the member's movement generator outright, so while active the
// member never runs its own legs; combat still takes precedence (the C++
// chase generator is pushed over the formation generator, and
// LeaderStartedMoving skips engaged members).
func (s *Server) stepFormationMember(motion *creatureMotion, now time.Time) bool {
	leader := s.findCreatureMotion(motion.Map, motion.InstanceID, motion.FormationLeadGUID)
	if leader == nil || leader.Health == 0 {
		// C++ finalizes the generator when the leader is gone; Go falls back
		// to the member's own legs (no idle-hold model).
		motion.FormationActive = false
		return false
	}
	leaderMoving := leader.Moving && now.Before(leader.MoveEnds)
	// DoUpdate: the leader's spline finalized on the predicted spline — stop
	// the member and align facing with the leader (MovementInform has no Go
	// consumer; the Lua OnReachWP event is waypoint-generator specific).
	if !leaderMoving && motion.FormationPredicted && !motion.FormationLeaderMoveEnds.IsZero() && leader.MoveEnds == motion.FormationLeaderMoveEnds {
		motion.Moving = false
		motion.Orientation = leader.Orientation
		motion.FormationPredicted = false
		return true
	}
	// DoUpdate: the leader launched a new spline — relaunch the member. The
	// waypoint flip (2pi - angle when the leader's current waypoint is one of
	// the member's LeaderWaypointIDs) rides here.
	if leaderMoving && leader.MoveEnds != motion.FormationLeaderMoveEnds {
		if motion.FormationPoint1 != 0 && leader.MoveType == 2 && len(leader.Points) > 0 {
			arrived := (leader.NextIdx - 1 + len(leader.Points)) % len(leader.Points)
			if uint32(arrived) == motion.FormationPoint1 || uint32(arrived) == motion.FormationPoint2 {
				motion.FormationAngle = float32(2*math.Pi) - motion.FormationAngle
			}
		}
		s.launchFormationLeg(motion, leader, now)
		return true
	}
	// DoUpdate: FORMATION_MOVEMENT_INTERVAL (1200ms) re-check — relaunch when
	// the leader's position changed since the last check.
	if now.Before(motion.FormationNextCheck) {
		return true
	}
	motion.FormationNextCheck = now.Add(1200 * time.Millisecond)
	if leader.X != motion.FormationLeaderX || leader.Y != motion.FormationLeaderY {
		s.launchFormationLeg(motion, leader, now)
	}
	return true
}

// launchFormationLeg mirrors FormationMovementGenerator::LaunchMovement
// (FormationMovementGenerator.cpp:140-183). The member's destination is the
// formation slot: the leader's position offset by (dist, angle+heading),
// where heading is the leader's travel heading while it moves
// (DoUpdate's relativeAngle = leader.GetRelativeAngle(spline destination))
// and 0 when it is stationary. C++ predicts the leader's position 1.65s
// ahead (the sniffed 1650ms spline duration); Go's atomic splines already
// jump the leader to its leg destination at launch, so the server-side
// leader position IS the predicted position and no extra prediction is
// applied. Catchup: member velocity = leader velocity *
// min(slotDist/(leaderVel*1.65), 1.5). The member faces the leader's
// orientation at launch; C++ sets it at spline end (documented timing
// delta — Go has no mid-leg update hook). Collision legs
// (MovePositionToFirstCollision) have no Go model.
func (s *Server) launchFormationLeg(motion, leader *creatureMotion, now time.Time) {
	leaderMoving := leader.Moving && now.Before(leader.MoveEnds)
	var frame float32
	vel := leader.Speed
	if vel <= 0 {
		vel = creatureBaseWalkSpeed
	}
	if leaderMoving {
		frame = leader.MoveHeading
		if leader.MoveVelocity > 0 {
			vel = leader.MoveVelocity
		}
	}
	ang := float64(motion.FormationAngle + frame)
	destX := leader.X + motion.FormationDist*float32(math.Cos(ang))
	destY := leader.Y + motion.FormationDist*float32(math.Sin(ang))
	slotDist := math.Hypot(float64(destX-motion.X), float64(destY-motion.Y))
	if slotDist < 0.5 {
		motion.FormationLeaderX, motion.FormationLeaderY = leader.X, leader.Y
		motion.FormationLeaderMoveEnds = leader.MoveEnds
		motion.FormationPredicted = leaderMoving
		return
	}
	velMod := slotDist / float64(vel*1.65)
	if velMod > 1.5 {
		velMod = 1.5
	}
	memberVel := float32(float64(vel) * velMod)
	if memberVel <= 0 {
		memberVel = vel
	}
	duration := splineDurationMs(slotDist, memberVel)
	heading := float32(math.Atan2(float64(destY-motion.Y), float64(destX-motion.X)))
	s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, leader.Z, duration, false, leader.Orientation, true)
	motion.X, motion.Y = destX, destY
	motion.Z = leader.Z
	motion.Moving = true
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.WaitUntil = motion.MoveEnds
	motion.MoveHeading = heading
	motion.MoveVelocity = memberVel
	motion.FormationPredicted = leaderMoving
	motion.FormationLeaderMoveEnds = leader.MoveEnds
	motion.FormationLeaderX, motion.FormationLeaderY = leader.X, leader.Y
	motion.FormationNextCheck = now.Add(1200 * time.Millisecond)
}
