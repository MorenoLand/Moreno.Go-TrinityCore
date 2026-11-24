package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// This file bridges the creature formation system: FormationMgr /
// CreatureGroup (src/server/game/Entities/Creature/CreatureGroups.cpp) and
// FormationMovementGenerator
// (src/server/game/Movement/MovementGenerators/FormationMovementGenerator.cpp).
// creature_formations rows previously had a write path only (the
// WORLD_INS_CREATURE_FORMATION statement); nothing ever read them.

const flagIdleInFormation = 0x200   // GroupAIFlags::FLAG_IDLE_IN_FORMATION (CreatureGroups.h): member follows the leader when pathing idly
const flagMembersAssistLeader = 0x1 // GroupAIFlags::FLAG_MEMBERS_ASSIST_LEADER (CreatureGroups.h): members engage when the leader engages
const flagLeaderAssistsMember = 0x2 // GroupAIFlags::FLAG_LEADER_ASSISTS_MEMBER (CreatureGroups.h): the leader engages when a member engages

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

// loadFormationRow restores one creature_formations row by member spawn id
// (FormationMgr::LoadCreatureFormations, CreatureGroups.cpp:101-135, single-row
// form): the DB row is the authority C++'s FormationMgr info map keeps for the
// process lifetime — RemoveMember never touches it — so a boarded-out member
// (strip 56) re-adds from the DB on respawn instead of staying formation-less
// for the rest of the process. Stored verbatim like ensureFormationsLoaded,
// with the same narrow-table fallback when point_1/point_2 are absent.
func (s *Server) loadFormationRow(ctx context.Context, memberLow uint32) (formationRow, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return formationRow{}, false
	}
	var leaderGUID, groupAI, point1, point2 uint64
	var dist, angle float64
	err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT leaderGUID, dist, angle, groupAI, point_1, point_2 FROM creature_formations WHERE memberGUID = ?", memberLow).Scan(&leaderGUID, &dist, &angle, &groupAI, &point1, &point2)
	if err != nil {
		err = s.WorldStore.DB.QueryRowContext(ctx, "SELECT leaderGUID, dist, angle, groupAI FROM creature_formations WHERE memberGUID = ?", memberLow).Scan(&leaderGUID, &dist, &angle, &groupAI)
		if err != nil {
			return formationRow{}, false
		}
	}
	return formationRow{leaderGUID: uint32(leaderGUID), dist: float32(dist), angle: float32(angle), groupAI: uint32(groupAI), point1: uint32(point1), point2: uint32(point2)}, true
}

// searchFormation mirrors Creature::SearchFormation (Creature.cpp:340-351),
// called from Creature::AddToWorld (:295): a creature with a DB spawn id
// re-joins its formation group on (re)spawn. The join is
// FormationMgr::AddCreatureToGroup (CreatureGroups.cpp:41-83): the row's
// leader group gains the member, or a new group forms. Temp summons never
// carry a spawn id (IsSummon gate, :341-342): the flag is stamped only after
// motionFor returns (lua_creature_events.go:179), so the dynamic-low floor
// is the operative guard here — summon lows start at 2000000
// (totems.go nextDynamicCreatureLowGUID), below which DB spawn ids live
// (creaturemotion.go:8772 states the same split). A roster listing already
// present (dynamic-spawn dedup, CreatureGroups.cpp:52-64) is not duplicated,
// and no per-motion state is set: the member only moves when the leader
// pulls it (LeaderStartedMoving).
func (s *Server) searchFormation(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || motion.IsTempSummon {
		return
	}
	memberLow := uint32(motion.GUID & 0x00FFFFFF)
	if memberLow == 0 || memberLow >= 2000000 {
		return
	}
	s.ensureFormationsLoaded(ctx)
	row, ok := s.formationRowFor(memberLow)
	if !ok {
		var loaded bool
		row, loaded = s.loadFormationRow(ctx, memberLow)
		if !loaded {
			return
		}
	}
	s.formationMu.Lock()
	defer s.formationMu.Unlock()
	if _, ok := s.formationRows[memberLow]; !ok {
		s.formationRows[memberLow] = row
	}
	members := s.formationLeaders[row.leaderGUID]
	for _, low := range members {
		if low == memberLow {
			return
		}
	}
	s.formationLeaders[row.leaderGUID] = append(members, memberLow)
}

// isLiveFormationLeader reports whether the spawn low still belongs to a
// formation as its leader: the info row exists, names this low as leader,
// and the live roster still lists it. C++'s m_formation pointer plus
// CreatureGroup::IsLeader (Creature.cpp:354-359, :361-369) in one test; a
// leader boarded out of its group (strip 56 removeFormationMember) fails the
// roster leg even though its cached info row survives (and U4 restores it on
// respawn, when membership legitimately returns).
func (s *Server) isLiveFormationLeader(leaderLow uint32) bool {
	row, ok := s.formationRowFor(leaderLow)
	if !ok || row.leaderGUID != leaderLow {
		return false
	}
	s.formationMu.RLock()
	defer s.formationMu.RUnlock()
	for _, low := range s.formationLeaders[leaderLow] {
		if low == leaderLow {
			return true
		}
	}
	return false
}

// removeFormationMember mirrors CreatureGroup::RemoveMember
// (CreatureGroups.cpp:217-223), reached from Unit::_EnterVehicle
// (Unit.cpp:13109-13115): a player boarding a creature-base vehicle
// that is part of a formation removes the base from its formation —
// the member leaves the group and its formation pointer clears, so
// it no longer assists the leader or follows it. The remaining
// members keep their rows: C++ nulls only the leader pointer when
// the removed member is the leader, and MemberEngagingTarget still
// runs over the members left in the group. Runtime-only, matching
// C++ (the DB row stays; a respawn re-adds through
// FormationMgr::AddCreatureToGroup).
func (s *Server) removeFormationMember(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.ensureFormationsLoaded(ctx)
	memberLow := uint32(motion.GUID & 0x00FFFFFF)
	s.formationMu.Lock()
	row, ok := s.formationRows[memberLow]
	if !ok {
		s.formationMu.Unlock()
		return
	}
	delete(s.formationRows, memberLow)
	members := s.formationLeaders[row.leaderGUID]
	for i, low := range members {
		if low == memberLow {
			members = append(members[:i], members[i+1:]...)
			break
		}
	}
	if len(members) == 0 {
		delete(s.formationLeaders, row.leaderGUID)
	} else {
		s.formationLeaders[row.leaderGUID] = members
	}
	s.formationMu.Unlock()
	// Creature::SetFormation(nullptr) twin: the live formation state
	// clears so the step tick stops pulling the base into formation
	// movement (stepFormationMember keys off FormationLeadGUID).
	s.motionMu.Lock()
	motion.FormationActive = false
	motion.FormationLeadGUID = 0
	s.motionMu.Unlock()
}

// memberEngagingTarget bridges CreatureGroup::MemberEngagingTarget
// (CreatureGroups.cpp:226-255), reached via Creature::AtEngage
// (Creature.cpp:3431-3453) — the Go call sites are the two fresh-engage
// paths (player-damage aggro, sight acquisition). When a formation member
// engages a target, live formation members assist per the row's GroupAI:
// FLAG_MEMBERS_ASSIST_LEADER (0x1) makes members engage when the leader
// engages; FLAG_LEADER_ASSISTS_MEMBER (0x2) makes the leader engage when a
// member engages (FLAG_MEMBERS_ASSIST_MEMBER, 0x3, is both). Each assist
// mirrors Unit::EngageWithTarget (Unit.cpp:8429-8438): a zero-threat seed
// plus combat state. Like C++, assisted engages never cascade —
// EngageWithTarget does not run AtEngage, so no _engaging recursion guard
// is needed. Members already fighting the same target are skipped (C++'s
// AddThreat is idempotent; Go's explicit attack-start broadcast is not).
// The charm re-engage path deliberately does not call this: its C++ owner
// is Unit::EngageWithTarget, which never runs AtEngage/MemberEngagingTarget.
func (s *Server) memberEngagingTarget(ctx context.Context, motion *creatureMotion, targetGUID uint64) {
	if s == nil || motion == nil || targetGUID == 0 {
		return
	}
	s.ensureFormationsLoaded(ctx)
	memberLow := uint32(motion.GUID & 0x00FFFFFF)
	row, ok := s.formationRowFor(memberLow)
	if !ok || row.groupAI == 0 {
		return
	}
	leaderLow := row.leaderGUID
	if memberLow == leaderLow {
		if row.groupAI&flagMembersAssistLeader == 0 {
			return
		}
	} else if row.groupAI&flagLeaderAssistsMember == 0 {
		return
	}
	// IsValidAttackTarget (Object.cpp:2930) from each member's perspective:
	// the target is always a player in Go and the engage sites already
	// validated it for the engager, so the live essence is that the target
	// player is still online and alive (no GM/UNATTACKABLE models in Go).
	targetSess := s.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil || targetSess.player.Health == 0 {
		return
	}
	s.formationMu.RLock()
	memberLows := append([]uint32(nil), s.formationLeaders[leaderLow]...)
	s.formationMu.RUnlock()
	var engaged []*creatureMotion
	s.motionMu.Lock()
	motions := s.motionMapLocked(motion.Map, motion.InstanceID)
	for _, low := range memberLows {
		var member *creatureMotion
		for guid, m := range motions {
			if uint32(guid&0x00FFFFFF) == low {
				member = m
				break
			}
		}
		if member == nil || member == motion || member.Health == 0 {
			continue
		}
		isLeader := low == leaderLow
		if !((!isLeader && row.groupAI&flagMembersAssistLeader != 0) || (isLeader && row.groupAI&flagLeaderAssistsMember != 0)) {
			continue
		}
		if member.InCombat && member.TargetGUID == targetGUID {
			continue
		}
		if member.ThreatMgr == nil {
			member.ThreatMgr = NewThreatManager(member)
		}
		wasInCombat := member.InCombat
		s.creatureAddThreat(member, targetGUID, 0.0, true, targetSess, nil)
		// Creature::AtEngage (Creature.cpp:3435-3436, 3446-3447) via
		// CreatureGroup::MemberEngagingTarget (CreatureGroups.cpp:226-255)
		// -> EngageWithTarget: each freshly engaged member dismounts and
		// re-homes; an already-engaged member never re-fires (CreatureAI
		// ::EngagementStart is _isEngaged-guarded), so the legs ride the
		// pre-add engagement state.
		if !wasInCombat {
			atEngageDismount(member)
			atEngageSetHomePosition(member)
		}
		member.TargetGUID = targetGUID
		member.InCombat = true
		member.Moving = false
		engaged = append(engaged, member)
	}
	s.motionMu.Unlock()
	for _, member := range engaged {
		// CreatureAI::UpdateVictim (CreatureAI.cpp:251-257): only a
		// non-passive member's AI calls AttackStart -> Unit::Attack, the
		// sole sender of SMSG_ATTACK_START (Unit.cpp:5694/5748) and the
		// AI_REACTION_HOSTILE reaction (Unit.cpp:5734-5737).
		// CreatureGroup::MemberEngagingTarget (CreatureGroups.cpp:226-255)
		// engages via Unit::EngageWithTarget (Unit.cpp:8429-8438), which
		// sends neither — a passive member engages silently (combat state +
		// threat, cleared by the step tick's passive arm) and must not
		// broadcast attack packets.
		if isCreaturePassive(member) {
			continue
		}
		s.broadcastAIReactionInInstance(motion.Map, motion.InstanceID, member.GUID, 2)
		startPkt := buildAttackStart(member.GUID, targetGUID)
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, motion.Map, motion.InstanceID, member.X, member.Y, member.Z, targetSess)
	}
}

// formationLeaderStartedMoving mirrors CreatureGroup::LeaderStartedMoving
// (CreatureGroups.cpp:280-295), reached via Creature::SignalFormationMovement
// (Creature.cpp:362-369) from the point/wander/waypoint generators. After a
// leader launches an idle-path leg, each live member row with
// FLAG_IDLE_IN_FORMATION whose creature is alive and not engaged joins
// formation movement (MoveFormation). The slot angle is inverted
// canFormationLeaderStartMoving mirrors CreatureGroup::CanLeaderStartMoving
// (CreatureGroups.cpp:298-308) via Creature::IsFormationLeaderMoveAllowed
// (Creature.cpp:372-378): a formation leader may start its waypoint leg only
// when no other alive group member is engaged (CreatureAI::IsEngaged —
// Go's InCombat) or returning home (Creature::IsReturningHome — the
// HOME_MOTION_TYPE leg; Go's Evading home-return move is the twin). The C++
// gate sits in WaypointMovementGenerator::DoUpdate
// (WaypointMovementGenerator.cpp:273), before the waypoint index advances,
// so a blocked leader retries the same node on the next tick.
func (s *Server) canFormationLeaderStartMoving(ctx context.Context, motion *creatureMotion) bool {
	s.ensureFormationsLoaded(ctx)
	leaderLow := uint32(motion.GUID & 0x00FFFFFF)
	// WaypointMovementGenerator::StartMove (WaypointMovementGenerator.cpp:273):
	// the move-allowed gate is consulted only for a formation leader
	// (owner->IsFormationLeader() && !owner->IsFormationLeaderMoveAllowed());
	// a creature with no formation — or a leader boarded out of its group
	// (strip 56 removeFormationMember clears m_formation) — moves freely.
	// The stale roster still lists the other members, so without this guard
	// a removed leader's waypoint leg stays blocked by ex-members.
	if !s.isLiveFormationLeader(leaderLow) {
		return true
	}
	s.formationMu.RLock()
	memberLows := append([]uint32(nil), s.formationLeaders[leaderLow]...)
	s.formationMu.RUnlock()
	if len(memberLows) == 0 {
		return true
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motions := s.motionMapLocked(motion.Map, motion.InstanceID)
	for _, memberLow := range memberLows {
		for guid, m := range motions {
			if m == motion || uint32(guid&0x00FFFFFF) != memberLow {
				continue
			}
			if m.Health > 0 && (m.InCombat || m.Evading) {
				return false
			}
			break
		}
	}
	return true
}

// formationLeaderStartedMoving mirrors CreatureGroup::LeaderStartedMoving
// (CreatureGroups.cpp:279-295): the leader pulls FLAG_IDLE_IN_FORMATION
// members into formation movement on its idle-path legs, with
// (FollowAngle + pi) per the C++ comment.
func (s *Server) formationLeaderStartedMoving(ctx context.Context, motion *creatureMotion, now time.Time) {
	s.ensureFormationsLoaded(ctx)
	// Creature::SignalFormationMovement (Creature.cpp:361-369): the pull
	// fires only when the mover still belongs to a formation AND is its
	// leader — the m_formation and IsLeader early returns. A leader
	// boarded out of its group (strip 56 removeFormationMember) pulls
	// nobody: the cached info row alone does not prove membership (C++'s
	// FormationMgr info map is never mutated by RemoveMember), so the
	// live roster must still list the leader low under its own leader id.
	leaderLow := uint32(motion.GUID & 0x00FFFFFF)
	if !s.isLiveFormationLeader(leaderLow) {
		return
	}
	s.formationMu.RLock()
	memberLows := append([]uint32(nil), s.formationLeaders[leaderLow]...)
	rows := make(map[uint32]formationRow, len(memberLows))
	for _, low := range memberLows {
		if r, ok := s.formationRows[low]; ok {
			rows[low] = r
		}
	}
	s.formationMu.RUnlock()
	if len(memberLows) == 0 {
		return
	}
	s.motionMu.Lock()
	motions := s.motionMapLocked(motion.Map, motion.InstanceID)
	var pendingMoves []*pendingFormationMove
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
		member.FormationLeaderZ, member.FormationLeaderO = motion.Z, motion.Orientation
		member.FormationNextCheck = now.Add(1200 * time.Millisecond)
		if pending := s.planFormationLeg(member, motion, now); pending != nil {
			pendingMoves = append(pendingMoves, pending)
		}
	}
	s.motionMu.Unlock()
	for _, pending := range pendingMoves {
		s.broadcastPendingFormationMove(pending)
	}
}

// stepFormationMember runs one FormationMovementGenerator::DoUpdate tick for
// a member pulled into formation. It returns true when the member's own
// wander/waypoint legs stay suppressed for this tick. The C++ generator
// replaces the member's movement generator outright, so while active the
// member never runs its own legs; combat still takes precedence (the C++
// chase generator is pushed over the formation generator, and
// LeaderStartedMoving skips engaged members).
func (s *Server) stepFormationMember(ctx context.Context, motion *creatureMotion, now time.Time) bool {
	leader := s.findCreatureMotion(motion.Map, motion.InstanceID, motion.FormationLeadGUID)
	if leader == nil || leader.Health == 0 {
		// C++ finalizes the generator when the leader is gone; Go falls back
		// to the member's own legs (no idle-hold model).
		motion.FormationActive = false
		return false
	}
	leaderMoving := leader.Moving && now.Before(leader.MoveEnds)
	// DoUpdate: the leader's spline finalized on the predicted spline — stop
	// the member and align facing with the leader (FormationMovementGenerator
	// .cpp:134: MovementInform(owner) on the member-spline-finalized arm —
	// AI()->MovementInform(FORMATION_MOTION_TYPE, 0), Eluna
	// CREATURE_EVENT_ON_MOVEMENT_INFORM (28)). The DoFinalize arm (:215)
	// is dead code in C++ too (DoInitialize never sets INFORM_ENABLED),
	// so only this Update arm informs. FormationPredicted clears here,
	// so the inform fires once per predicted move.
	if !leaderMoving && motion.FormationPredicted && !motion.FormationLeaderMoveEnds.IsZero() && leader.MoveEnds == motion.FormationLeaderMoveEnds {
		motion.Moving = false
		motion.Orientation = leader.Orientation
		motion.FormationPredicted = false
		s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnMovementInform, uint32(movementGeneratorFormation), 0)
		s.smartMovementInform(ctx, motion, movementGeneratorFormation, 0)
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
	// the leader's position changed since the last check. The comparison is
	// the full position (FormationMovementGenerator.cpp:122, Position::
	// operator==, Position.cpp:28-34), so a leader turning in place or
	// changing height relaunches members too — the slot destination is
	// oriented off the leader's facing.
	if now.Before(motion.FormationNextCheck) {
		return true
	}
	motion.FormationNextCheck = now.Add(1200 * time.Millisecond)
	if leader.X != motion.FormationLeaderX || leader.Y != motion.FormationLeaderY || leader.Z != motion.FormationLeaderZ || leader.Orientation != motion.FormationLeaderO {
		s.launchFormationLeg(motion, leader, now)
	}
	return true
}

// launchFormationLeg mirrors FormationMovementGenerator::LaunchMovement
// (FormationMovementGenerator.cpp:140-183). The member's destination is the
// formation slot: the leader's position offset by (dist, angle+heading),
// where heading is the leader's travel heading while it moves
// (DoUpdate's relativeAngle = leader.GetRelativeAngle(spline destination),
// which MovePositionToFirstCollision re-adds the orientation to) and the
// leader's orientation while it stands still (relativeAngle stays 0 and
// MovePositionToFirstCollision adds the orientation, Object.cpp:3386-3390).
// C++ predicts the leader's position 1.65s
// ahead (the sniffed 1650ms spline duration); Go's atomic splines already
// jump the leader to its leg destination at launch, so the server-side
// leader position IS the predicted position and no extra prediction is
// applied. Catchup: while the leader moves, member velocity = leader
// velocity * min(slotDist/(leaderVel*1.65), 1.5); a stationary leader's
// member walks at the leader's walk speed with no mod, and the Launch cap
// (MoveSplineInit.cpp:122) binds either way. The member faces the leader's
// orientation at launch; C++ sets it at spline end (documented timing
// delta — Go has no mid-leg update hook). Collision legs
// (MovePositionToFirstCollision) have no Go model.
// pendingFormationMove carries a formation leg's monster-move broadcast,
// deferred until the caller's motionMu span ends: the broadcast filters
// recipients through currentPlayerPhaseMask, which takes each recipient's
// castMu, so broadcasting inside formationLeaderStartedMoving's motionMu
// span nests motionMu->castMu.
type pendingFormationMove struct {
	mapID, instanceID                        uint32
	guid                                     uint64
	fromX, fromY, fromZ, destX, destY, destZ float32
	duration                                 uint32
	facing                                   float32
	phase                                    uint32
	transportGUID                            uint64
	// points carries a routed CalculatePath poly (3+ points) for the
	// multi-point broadcast; nil keeps the direct single segment.
	points []pathVec
}

func (s *Server) launchFormationLeg(motion, leader *creatureMotion, now time.Time) {
	if pending := s.planFormationLeg(motion, leader, now); pending != nil {
		s.broadcastPendingFormationMove(pending)
	}
}

// broadcastPendingFormationMove flushes a planned formation leg: a routed
// poly rides the multi-point writer, a direct leg the single-point writer
// (the member faces the leader's orientation for the whole leg, as the
// direct arm already did).
func (s *Server) broadcastPendingFormationMove(pending *pendingFormationMove) {
	if s == nil || pending == nil {
		return
	}
	if pending.points != nil {
		s.broadcastMonsterMoveMultiInInstance(pending.mapID, pending.instanceID, pending.guid, pending.fromX, pending.fromY, pending.fromZ, pending.points, pending.duration, false, pending.facing, true, pending.phase, pending.transportGUID, false, animTierNone)
		return
	}
	s.broadcastMonsterMoveInInstance(pending.mapID, pending.instanceID, pending.guid, pending.fromX, pending.fromY, pending.fromZ, pending.destX, pending.destY, pending.destZ, pending.duration, false, pending.facing, true, pending.phase, pending.transportGUID)
}

// planFormationLeg is launchFormationLeg's motion-state half: it computes
// the formation slot destination and records the leg on the member, but
// returns the broadcast for the caller to flush after its motionMu span
// instead of broadcasting inside it.
func (s *Server) planFormationLeg(motion, leader *creatureMotion, now time.Time) *pendingFormationMove {
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
	} else {
		// Stationary leader: relativeAngle stays 0 and
		// MovePositionToFirstCollision adds the leader's orientation
		// (Object.cpp:3386-3390), so the slot sits at angle+orientation.
		frame = leader.Orientation
	}
	ang := float64(motion.FormationAngle + frame)
	destX := leader.X + motion.FormationDist*float32(math.Cos(ang))
	destY := leader.Y + motion.FormationDist*float32(math.Sin(ang))
	// GetExactDist (FormationMovementGenerator.cpp:165) is 3D, like the
	// waypoint launcher's distance3D leg length (Spline.cpp:152-156).
	slotDist := distance3D(motion.X, motion.Y, motion.Z, destX, destY, leader.Z)
	if slotDist < 0.5 {
		motion.FormationLeaderX, motion.FormationLeaderY = leader.X, leader.Y
		motion.FormationLeaderZ, motion.FormationLeaderO = leader.Z, leader.Orientation
		motion.FormationLeaderMoveEnds = leader.MoveEnds
		motion.FormationPredicted = leaderMoving
		return nil
	}
	// The catchup mod lives inside LaunchMovement's moving branch only
	// (FormationMovementGenerator.cpp:149-166); a stationary leader's
	// velocity falls through to GetSpeed(MOVE_WALK) with no mod, so the
	// member walks at the leader's walk speed (vel above) instead of up
	// to 1.5x it.
	memberVel := vel
	if leaderMoving {
		velMod := slotDist / float64(vel*1.65)
		if velMod > 1.5 {
			velMod = 1.5
		}
		memberVel = float32(float64(vel) * velMod)
	}
	// MoveSplineInit::Launch caps every SetVelocity spline
	// (MoveSplineInit.cpp:122); the formation init is no exception.
	memberVel = capSplineVelocity(memberVel, motion.RunSpeed)
	if memberVel <= 0 {
		memberVel = vel
	}
	// FormationMovementGenerator::LaunchMovement
	// (FormationMovementGenerator.cpp:192-193): init.MoveTo runs
	// CalculatePath with the MoveTo default generatePath=true
	// (MoveSplineInit.cpp:245-256), falling back to the direct leg
	// below on failure or PATHFIND_NOPATH — MoveTo rejects only
	// NOPATH. No path-length limit. A routed poly (3+ points) rides
	// the multi-point broadcast; a 2-point shortcut is geometrically
	// the direct leg, so it keeps the pending single segment. The
	// path runs here, inside the plan, so the broadcast stays deferred:
	// planFormationLeg runs under motionMu in the
	// formationLeaderStartedMoving path, and CalculatePath takes no
	// motionMu (lock-free source snapshot).
	var polyPoints []pathVec
	path := NewPathGenerator(s, newCreaturePathSource(s, motion))
	if ok := path.CalculatePath(destX, destY, leader.Z, false); ok && path.PathType()&pathfindNopath == 0 {
		if points := path.PathPoints(); len(points) > 2 {
			polyPoints = points
		}
	}
	duration := splineDurationMs(slotDist, memberVel)
	if polyPoints != nil {
		travelDist := float32(0)
		for i := 1; i < len(polyPoints); i++ {
			travelDist += polyPoints[i].sub(polyPoints[i-1]).length()
		}
		duration = splineDurationMs(float64(travelDist), memberVel)
	}
	heading := float32(math.Atan2(float64(destY-motion.Y), float64(destX-motion.X)))
	pending := &pendingFormationMove{mapID: motion.Map, instanceID: motion.InstanceID, guid: motion.GUID, fromX: motion.X, fromY: motion.Y, fromZ: motion.Z, destX: destX, destY: destY, destZ: leader.Z, duration: duration, facing: leader.Orientation, phase: motion.PhaseMask, transportGUID: motion.TransportGUID, points: polyPoints}
	motion.X, motion.Y = destX, destY
	motion.Z = leader.Z
	// FormationMovementGenerator::DoUpdate (FormationMovementGenerator.cpp:91):
	// while the member spline is in flight the home position tracks the
	// member's current position, so an evade after formation movement
	// returns to the last formation position instead of spawn. The
	// atomic model snaps to the launch destination, so the destination
	// is the in-flight position — the waypoint twin of this arm
	// (creaturemotion.go:13998-14015) does the same, and HomeO carries
	// the member's CURRENT orientation (SetHomePosition(x,y,z,o)), not
	// the leader's (the member takes the leader's orientation at spline
	// end, per the documented delta in this function).
	motion.HomeX, motion.HomeY, motion.HomeZ = destX, destY, leader.Z
	motion.HomeO = motion.Orientation
	motion.Moving = true
	motion.ScriptMoveFromKnown = false
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.WaitUntil = motion.MoveEnds
	motion.MoveHeading = heading
	motion.MoveVelocity = memberVel
	motion.ScriptMoveExplicitSpeed = 0
	motion.FormationPredicted = leaderMoving
	motion.FormationLeaderMoveEnds = leader.MoveEnds
	motion.FormationLeaderX, motion.FormationLeaderY = leader.X, leader.Y
	motion.FormationLeaderZ, motion.FormationLeaderO = leader.Z, leader.Orientation
	motion.FormationNextCheck = now.Add(1200 * time.Millisecond)
	return pending
}
