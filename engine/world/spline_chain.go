package world

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// Spline chains (SplineChainMovementGenerator.cpp / ScriptSystem.cpp
// LoadScriptSplineChains): DB-driven multi-spline paths keyed by
// (creature entry, chainId), loaded from world
// script_spline_chain_meta (entry, chainId, splineId,
// expectedDuration, msUntilNext, velocity) and
// script_spline_chain_waypoints (entry, chainId, splineId, wpId, x,
// y, z). A chain is a list of links; each link is one spline through
// its waypoints at the link velocity (0 = the creature's walk/run
// speed for the walk flag). The generator sends the next link
// msUntilNext after the previous link's launch, scaled by
// actualDuration/expectedDuration when the spline ran long or short
// (SendSplineFor), and on the final link's landing fires
// MovementInform(SPLINE_CHAIN_MOTION_TYPE, pointId). A missing table
// or empty table loads zero chains, the C++ "tables are empty" path —
// the repo's world.db placeholder carries no such tables, so the
// bridge is live but idle until the deployment DB ships the rows.
// Go walks a link's points as sequential single-point splines (the
// engine has no multi-point spline primitive), so a link's next point
// still waits for the previous leg to land; the next LINK cuts the
// running spline short once its msUntilNext wait expires
// (SplineChainMovementGenerator::Update sends mid-spline), launching
// from the interpolated position.

// splineChainPoint is one waypoint of a spline-chain link.
type splineChainPoint struct {
	X, Y, Z float32
}

// splineChainLink is one SplineChainLink (SplineChain.h): a single
// spline through Points, launched at Velocity, with the wait until
// the next link (MsUntilNext, TimeToNext) and the duration the DB
// row expects the spline to take (ExpectedDurationMs).
type splineChainLink struct {
	ExpectedDurationMs uint32
	MsUntilNext        uint32
	Velocity           float32
	Points             []splineChainPoint
}

type splineChainKey struct {
	Entry   uint32
	ChainID uint16
}

// splineChainRun is the live SplineChainMovementGenerator state on a
// motion: the chain being walked, the next link/point to send, the
// MovementInform id reported at the final landing, and the wait
// until the next link may be sent (measured from the current link's
// launch, the C++ _msToNext arm). EnteredCycle is the MoveSpline
// enter-cycle arm (MoveSpline.cpp:290-330): after the first full
// revolution the spline's first point (the owner's pre-cycle
// position) is removed and the velocity re-derived so the next
// revolution keeps the first cycle's duration; set once, on the
// first cyclic wrap, in landSplineChainLink.
type splineChainRun struct {
	Links          []splineChainLink
	LinkIndex      int // next link to send
	PointIndex     int // next point within Links[LinkIndex] to send
	PointID        uint32
	Walk           bool
	Flying         bool // MoveCirclePath twin: the flying branch sets SetFly (MotionMaster.cpp:851-855); passed through to the wire by sendSplineChainLink
	AnimTier       int  // MoveCirclePath twin: the flying branch sets SetAnimation(Hover); animTierNone otherwise
	Cyclic         bool // MoveCirclePath twin: the run loops to point 0 at the final landing with no wait and no inform, until superseded
	EnteredCycle   bool
	LinkLaunchedAt time.Time
	LinkActualMs   uint32 // summed leg durations of the current link
	WaitUntil      time.Time
	// WaitArmed says WaitUntil belongs to the next link and has not
	// been consumed yet: advanceSplineChain must only fire on an
	// armed wait. A fresh run's WaitUntil is zero-valued (already
	// expired), so without the arm the mid-flight advance would
	// re-send the in-flight link every tick and it could never land.
	WaitArmed bool
}

// loadSplineChains is the SystemMgr::LoadScriptSplineChains twin
// (ScriptSystem.cpp:88-158): meta rows build the links in splineId
// order (an orphaned splineId is skipped, the C++ warn-and-continue),
// waypoint rows append points in wpId order (orphaned waypoints
// skipped). A missing table or query error loads zero chains.
func loadSplineChains(ctx context.Context, db *sql.DB) map[splineChainKey][]splineChainLink {
	chains := map[splineChainKey][]splineChainLink{}
	if db == nil {
		return chains
	}
	metaRows, err := db.QueryContext(ctx, "SELECT entry, chainId, splineId, expectedDuration, msUntilNext, velocity FROM script_spline_chain_meta ORDER BY entry asc, chainId asc, splineId asc")
	if err != nil {
		return chains
	}
	defer metaRows.Close()
	for metaRows.Next() {
		var entry uint32
		var chainID uint16
		var splineID uint8
		var link splineChainLink
		if err := metaRows.Scan(&entry, &chainID, &splineID, &link.ExpectedDurationMs, &link.MsUntilNext, &link.Velocity); err != nil {
			continue
		}
		key := splineChainKey{Entry: entry, ChainID: chainID}
		chain := chains[key]
		if splineID != uint8(len(chain)) {
			continue
		}
		chains[key] = append(chain, link)
	}
	_ = metaRows.Close()
	wpRows, err := db.QueryContext(ctx, "SELECT entry, chainId, splineId, wpId, x, y, z FROM script_spline_chain_waypoints ORDER BY entry asc, chainId asc, splineId asc, wpId asc")
	if err != nil {
		return chains
	}
	defer wpRows.Close()
	for wpRows.Next() {
		var entry uint32
		var chainID uint16
		var splineID, wpID uint8
		var pt splineChainPoint
		if err := wpRows.Scan(&entry, &chainID, &splineID, &wpID, &pt.X, &pt.Y, &pt.Z); err != nil {
			continue
		}
		key := splineChainKey{Entry: entry, ChainID: chainID}
		chain, ok := chains[key]
		if !ok || int(splineID) >= len(chain) {
			continue
		}
		link := &chain[splineID]
		if wpID != uint8(len(link.Points)) {
			continue
		}
		link.Points = append(link.Points, pt)
	}
	return chains
}

// splineChainsFor is the lazy SystemMgr::GetSplineChain store: the
// first MoveAlongSplineChain call loads both tables once (a missing
// table loads zero chains, the C++ empty-tables path) and every later
// call reads the cache.
func (s *Server) splineChainsFor(ctx context.Context) map[splineChainKey][]splineChainLink {
	if s == nil {
		return nil
	}
	s.splineChainsOnce.Do(func() {
		var db *sql.DB
		if s.WorldStore != nil {
			db = s.WorldStore.DB
		}
		s.splineChains = loadSplineChains(ctx, db)
	})
	return s.splineChains
}

// moveAlongSplineChain is MotionMaster::MoveAlongSplineChain(pointId,
// dbChainId, walk) (MotionMaster.cpp:172-194): the chain is looked
// up by the caller's entry (or entryOverride, the
// MoveInfiniteOnSpawn twin — C++ looks the infinite chains up by
// NPC_INFINITE_ADVERSARY whatever spawned them); a missing chain
// logs nothing and returns false, the C++ error-and-return. A live
// chain starts a splineChainRun and sends its first link; the
// landing gate advances it link by link and fires
// MovementInform(SPLINE_CHAIN_MOTION_TYPE, pointId) at the end.
func (s *Server) moveAlongSplineChain(ctx context.Context, motion *creatureMotion, pointID uint32, chainID uint16, walk bool, entryOverride uint32) bool {
	if s == nil || motion == nil {
		return false
	}
	entry := motion.Entry
	if entryOverride != 0 {
		entry = entryOverride
	}
	chain, ok := s.splineChainsFor(ctx)[splineChainKey{Entry: entry, ChainID: chainID}]
	if !ok || len(chain) == 0 {
		return false
	}
	s.motionMu.Lock()
	motion.SplineChain = &splineChainRun{Links: chain, PointID: pointID, Walk: walk, AnimTier: animTierNone}
	s.motionMu.Unlock()
	return s.sendSplineChainLink(ctx, motion, time.Now())
}

// splineChainWaitMs scales a link's msUntilNext by the ratio of the
// actually-launched duration to the DB expected duration (the C++
// actual-duration comparison in the census note): the wait runs from
// the link's launch, so it is armed once the link's last point is
// sent and its actual duration is known.
func splineChainWaitMs(link splineChainLink, actualMs uint32) uint32 {
	waitMs := link.MsUntilNext
	if waitMs < 1 {
		waitMs = 1
	}
	if link.ExpectedDurationMs > 0 && actualMs > 0 && actualMs != link.ExpectedDurationMs {
		waitMs = waitMs * actualMs / link.ExpectedDurationMs
		if waitMs < 1 {
			waitMs = 1
		}
	}
	return waitMs
}

// sendSplineChainLink sends the current link of a run (its next
// point) as one atomic script spline at the link velocity
// (SplineChainMovementGenerator::SendPathSpline: velocity > 0 wins,
// else the creature walk/run speed). The leg is stamped
// SPLINE_CHAIN so the landing gate routes it back here instead of
// firing a point inform.
func (s *Server) sendSplineChainLink(ctx context.Context, motion *creatureMotion, now time.Time) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	run := motion.SplineChain
	if run == nil || run.LinkIndex >= len(run.Links) {
		s.motionMu.Unlock()
		return false
	}
	link := run.Links[run.LinkIndex]
	if run.PointIndex >= len(link.Points) {
		s.motionMu.Unlock()
		return false
	}
	pt := link.Points[run.PointIndex]
	if run.PointIndex == 0 {
		run.LinkLaunchedAt = now
		run.LinkActualMs = 0
	}
	speed := link.Velocity
	if speed <= 0 {
		speed = creatureSplineVelocityExplicit(motion, run.Walk)
	}
	// The wait scaling below compares against the C++ actual duration,
	// which Launch computes from the capped velocity (capSplineVelocity),
	// so the accumulation must use the capped speed too. The distance
	// runs from the interpolated spline position
	// (MoveSplineInit::Launch, MoveSplineInit.cpp:60-68): a link
	// launched over a live spline starts where the creature is, not
	// where the running leg will land.
	speed = capSplineVelocity(speed, motion.RunSpeed)
	fromX, fromY, fromZ := creatureSplinePosition(motion, now)
	dist := distance3D(fromX, fromY, fromZ, pt.X, pt.Y, pt.Z)
	run.LinkActualMs += splineDurationMs(float64(dist), speed)
	// The wait for the next link runs from this link's launch
	// (SplineChainMovementGenerator::_msToNext), so it is armed
	// once the link's last point is sent and its actual duration
	// is known; until then advanceSplineChain must not re-send
	// the in-flight link. The arm is re-asserted at landing with
	// the identical value (LinkActualMs is final by then).
	run.WaitArmed = false
	if run.PointIndex == len(link.Points)-1 && run.LinkIndex < len(run.Links)-1 {
		run.WaitUntil = run.LinkLaunchedAt.Add(time.Duration(splineChainWaitMs(link, run.LinkActualMs)) * time.Millisecond)
		run.WaitArmed = true
	}
	pointID, walk := run.PointID, run.Walk
	flying, animTier := run.Flying, run.AnimTier
	s.motionMu.Unlock()
	return s.launchCreatureScriptSplineSpeed(motion, pt.X, pt.Y, pt.Z, walk, movementGeneratorSplineChain, pointID, true, speed, flying, animTier)
}

// splineChainClosedLength is the closed-loop length of a chain link's
// points: consecutive segments plus the closing segment from the last
// point back to the first, the length a cyclic spline traverses per
// revolution.
func splineChainClosedLength(points []splineChainPoint) float64 {
	if len(points) < 2 {
		return 0
	}
	total := float64(0)
	for i := 1; i < len(points); i++ {
		total += distance3D(points[i-1].X, points[i-1].Y, points[i-1].Z, points[i].X, points[i].Y, points[i].Z)
	}
	total += distance3D(points[len(points)-1].X, points[len(points)-1].Y, points[len(points)-1].Z, points[0].X, points[0].Y, points[0].Z)
	return total
}

// moveCirclePath is MotionMaster::MoveCirclePath
// (MotionMaster.cpp:827-865): a GenericMovementGenerator
// (EFFECT_MOTION_TYPE 16) whose spline is the owner's current
// position followed by stepCount points around the circle
// (center + radius at angle, step 2*PI/stepCount, clockwise
// negated), SetCyclic so it loops until superseded. Go walks it
// as a single-link cyclic spline-chain run: the chain walker
// sends one point at a time and wraps to point 0 at the end with
// no wait and no inform, matching the C++ cyclic spline. The
// flying branch takes the passed z; the ground branch takes
// GetMapHeight(x, y, z) + GetHoverOffset() per point — the hover
// offset has no Go creature model and reads 0. After the first
// revolution the owner's starting point is removed and the
// velocity re-derived to preserve the first cycle's duration
// (MoveSpline.cpp:290-330, the enter-cycle arm in
// landSplineChainLink). All in-tree callers fly (quel_delar sword,
// malygos phase 3, yogg-saron). Speed is the Launch-selected
// velocity (creatureSplineVelocity): the flying branch's SetFly
// only sets the spline flying flag (MoveSplineInit.h:146) — the
// velocity still comes from SelectSpeedType, which for creatures
// never takes the FLYING leg (player-only flag), so the circle
// runs at MOVE_RUN, not the flight speed. A non-chain launch
// supersedes the run (the LAND
// leg's MoveLand); StopMoving alone does not clear a chain run
// (the standing MoveClear collapse delta), so callers halt a
// circle with a fresh launch. stepCount 0 or radius <= 0 fails
// validation with no spline, like the sub-0.001 speed leg.
func (s *Server) moveCirclePath(ctx context.Context, motion *creatureMotion, centerX, centerY, centerZ, radius float32, clockwise bool, stepCount uint8) bool {
	if s == nil || motion == nil || stepCount == 0 || radius <= 0 {
		return false
	}
	// Snapshot the launch state under the lock; the ground heights
	// below read .map files through Server.mapHeight and must not
	// run under motionMu. The install re-checks the snapshot so a
	// motion that moved or was relaunched mid-computation is not
	// clobbered with a stale circle.
	s.motionMu.Lock()
	ox, oy, oz := motion.X, motion.Y, motion.Z
	mapID := motion.Map
	prevChain := motion.SplineChain
	// MotionMaster.cpp:851-856: the IsFlying branch sets SetFly +
	// SetCyclic + SetAnimation(Hover); the ground branch walks
	// (SetWalk(true)) with no flags. IsFlying is Unit.h:1644
	// (MOVEMENTFLAG_FLYING | MOVEMENTFLAG_DISABLE_GRAVITY), whose
	// Go twins are motion.Flying (the per-tick UpdateMovementFlags
	// fly leg) and motion.GravityDisabled (the DisableGravity leg)
	// — motion.CanFly is the template capability, not the flag, so
	// a grounded CanFly creature still takes the ground branch,
	// the way C++ clears both flags out of air. The ground branch
	// walks: speed is the walk speed and the run carries Walk so
	// the chain link's mode packet walks too.
	flying := motion.Flying || motion.GravityDisabled
	walk := !flying || motion.Walking
	speed := creatureSplineVelocity(motion, walk)
	// MoveSplineInit::Launch (MoveSplineInit.cpp:123) caps the velocity at
	// 50 for a flying-flag spline — min(velocity, catmullrom||flying ? 50 :
	// max(28, run*4)) — and MoveCirclePath sets that flag only on the
	// flying branch (MotionMaster.cpp:851-855, SetFly). The non-flying
	// leg caps at max(28, run*4), which the walk speed never exceeds, so
	// only a flying circle can bind: a script-set MOVE_RUN rate can push
	// a flying circle's run speed past 50, where C++ clamps the spline
	// and Go previously did not.
	if flying && speed > 50 {
		speed = 50
	}
	s.motionMu.Unlock()
	angle := math.Atan2(float64(oy-centerY), float64(ox-centerX))
	step := 2 * math.Pi / float64(stepCount)
	if clockwise {
		step = -step
	}
	// C++ pushes the owner's current position first — it is removed
	// after the first cycle by the enter-cycle arm, so the run's
	// point 0 is the pre-cycle position, not a circle point.
	points := make([]splineChainPoint, 0, int(stepCount)+1)
	points = append(points, splineChainPoint{X: ox, Y: oy, Z: oz})
	for i := uint8(0); i < stepCount; i++ {
		px := centerX + radius*float32(math.Cos(angle))
		py := centerY + radius*float32(math.Sin(angle))
		pz := centerZ
		if !flying {
			if ground, ok := s.mapHeight(mapID, px, py, centerZ, true, terrainDefaultHeightSearch); ok {
				pz = ground
			}
		}
		points = append(points, splineChainPoint{X: px, Y: py, Z: pz})
		angle += step
	}
	animTier := animTierNone
	if flying {
		animTier = animTierHover
	}
	s.motionMu.Lock()
	if motion.X != ox || motion.Y != oy || motion.Z != oz || motion.SplineChain != prevChain {
		s.motionMu.Unlock()
		return false
	}
	motion.SplineChain = &splineChainRun{
		Links:    []splineChainLink{{Velocity: speed, Points: points}},
		Cyclic:   true,
		Walk:     walk,
		Flying:   flying,
		AnimTier: animTier,
	}
	s.motionMu.Unlock()
	return s.sendSplineChainLink(ctx, motion, time.Now())
}

// landSplineChainLink is the SplineChainMovementGenerator::Update /
// Finalize twin for one landed leg: the next point of the link goes
// out immediately; at the link's end the next link is armed after
// the scaled msUntilNext wait (SendSplineFor: wait =
// msUntilNext * actualDuration / expectedDuration, floored at 1ms,
// measured from the link's launch); at the final link's end the run
// clears and MovementInform(SPLINE_CHAIN_MOTION_TYPE, pointId)
// fires. A landing off the stamped destination means the chain was
// superseded mid-flight — the run clears silently, the C++
// Finalize(active, movementInform=false) arm.
func (s *Server) landSplineChainLink(ctx context.Context, motion *creatureMotion, now time.Time) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	run := motion.SplineChain
	if run == nil {
		s.motionMu.Unlock()
		return
	}
	if motion.X != motion.ScriptMoveDestX || motion.Y != motion.ScriptMoveDestY || motion.Z != motion.ScriptMoveDestZ {
		motion.SplineChain = nil
		motion.ScriptMoveType = 0
		motion.ScriptMovePointID = 0
		motion.ScriptMoveDestX = 0
		motion.ScriptMoveDestY = 0
		motion.ScriptMoveDestZ = 0
		s.motionMu.Unlock()
		return
	}
	run.PointIndex++
	link := run.Links[run.LinkIndex]
	lastLink := run.LinkIndex >= len(run.Links)-1
	if run.PointIndex < len(link.Points) {
		s.motionMu.Unlock()
		s.sendSplineChainLink(ctx, motion, now)
		return
	}
	if lastLink {
		if run.Cyclic {
			// MoveCirclePath cyclic twin (MotionMaster.cpp:827-865):
			// the path is one spline with SetCyclic, so the run
			// never finalizes — the next revolution starts at once
			// with no wait and no MovementInform, until a fresh
			// launch supersedes it (the LAND leg's MoveLand).
			// MoveSpline::_updateState enter-cycle arm
			// (MoveSpline.cpp:290-330): after the first full cycle
			// the spline's first point — the owner's pre-cycle
			// position — is removed, and the velocity is
			// re-derived so the new cycle keeps the first cycle's
			// duration (new length / old duration, i.e. the old
			// velocity scaled by new/old loop length). Later
			// revolutions keep that velocity: the point is already
			// gone, so there is nothing left to remove.
			if !run.EnteredCycle {
				run.EnteredCycle = true
				link := &run.Links[run.LinkIndex]
				if oldLen := splineChainClosedLength(link.Points); len(link.Points) > 1 && oldLen > 0 {
					link.Points = link.Points[1:]
					if newLen := splineChainClosedLength(link.Points); newLen > 0 {
						link.Velocity *= float32(newLen / oldLen)
					}
				}
			}
			run.LinkIndex = 0
			run.PointIndex = 0
			s.motionMu.Unlock()
			s.sendSplineChainLink(ctx, motion, now)
			return
		}
		pointID := run.PointID
		motion.SplineChain = nil
		motion.ScriptMoveType = 0
		motion.ScriptMovePointID = 0
		motion.ScriptMoveDestX = 0
		motion.ScriptMoveDestY = 0
		motion.ScriptMoveDestZ = 0
		s.motionMu.Unlock()
		s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnMovementInform, uint32(movementGeneratorSplineChain), pointID)
		return
	}
	run.WaitUntil = run.LinkLaunchedAt.Add(time.Duration(splineChainWaitMs(link, run.LinkActualMs)) * time.Millisecond)
	run.WaitArmed = true
	run.LinkIndex++
	run.PointIndex = 0
	motion.ScriptMoveType = 0
	motion.ScriptMovePointID = 0
	motion.ScriptMoveDestX = 0
	motion.ScriptMoveDestY = 0
	motion.ScriptMoveDestZ = 0
	motion.WaitUntil = run.WaitUntil
	s.motionMu.Unlock()
}

// advanceSplineChain sends the next link of a waiting run once its
// msUntilNext wait has expired (the Update arm: _msToNext <= diff).
// SplineChainMovementGenerator::Update (SplineChainMovementGenerator.cpp:
// 149-171) sends the next link even mid-spline — only the final link
// waits for the running spline to finalize — so the run advances
// whether or not the creature is still moving; the launch restarts
// from the interpolated position (creatureSplinePosition).
func (s *Server) advanceSplineChain(ctx context.Context, motion *creatureMotion, now time.Time) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	run := motion.SplineChain
	if run == nil || !run.WaitArmed || now.Before(run.WaitUntil) {
		s.motionMu.Unlock()
		return
	}
	s.motionMu.Unlock()
	s.sendSplineChainLink(ctx, motion, now)
}
