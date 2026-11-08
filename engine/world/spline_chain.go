package world

import (
	"context"
	"database/sql"
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
// engine has no multi-point spline primitive), so a link's next leg
// only launches once the previous leg has landed; the msUntilNext
// wait still runs from the link's launch, so a slow link delays the
// next one past the wait instead of cutting the spline short.

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
// launch, the C++ _msToNext arm).
type splineChainRun struct {
	Links          []splineChainLink
	LinkIndex      int // next link to send
	PointIndex     int // next point within Links[LinkIndex] to send
	PointID        uint32
	Walk           bool
	LinkLaunchedAt time.Time
	LinkActualMs   uint32 // summed leg durations of the current link
	WaitUntil      time.Time
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
	motion.SplineChain = &splineChainRun{Links: chain, PointID: pointID, Walk: walk}
	s.motionMu.Unlock()
	return s.sendSplineChainLink(ctx, motion, time.Now())
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
		speed = creatureSplineVelocity(motion, run.Walk)
	}
	dist := distance3D(motion.X, motion.Y, motion.Z, pt.X, pt.Y, pt.Z)
	run.LinkActualMs += splineDurationMs(float64(dist), speed)
	pointID, walk := run.PointID, run.Walk
	s.motionMu.Unlock()
	return s.launchCreatureScriptSplineSpeed(motion, pt.X, pt.Y, pt.Z, walk, movementGeneratorSplineChain, pointID, true, speed)
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
	waitMs := link.MsUntilNext
	if waitMs < 1 {
		waitMs = 1
	}
	if link.ExpectedDurationMs > 0 && run.LinkActualMs > 0 && run.LinkActualMs != link.ExpectedDurationMs {
		waitMs = waitMs * run.LinkActualMs / link.ExpectedDurationMs
		if waitMs < 1 {
			waitMs = 1
		}
	}
	run.WaitUntil = run.LinkLaunchedAt.Add(time.Duration(waitMs) * time.Millisecond)
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
// It runs from the step tick only while the creature is not moving.
func (s *Server) advanceSplineChain(ctx context.Context, motion *creatureMotion, now time.Time) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	run := motion.SplineChain
	if run == nil || motion.Moving || now.Before(run.WaitUntil) {
		s.motionMu.Unlock()
		return
	}
	s.motionMu.Unlock()
	s.sendSplineChainLink(ctx, motion, now)
}
