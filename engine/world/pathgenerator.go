// PathGenerator port: PathGenerator (PathGenerator.h, PathGenerator.cpp)
// — the navmesh path builder Movement generators and the .mmap path /
// testarea arms run over. CalculatePath + every private leg is ported
// name-by-name against the C++ text; detour calls ride the ported query
// (tools/mmaps-generator/detour). Coordinate order: pathVec is WoW
// (x, y, z); detour Vec3 is (y, z, x), converted at every query boundary
// exactly as C++ builds startPoint/endPoint (PathGenerator.cpp:179-180).
//
// Source model: C++ takes a WorldObject const*; Go has no WorldObject /
// Unit / Creature type (creatureMotion + playerState are the models), so
// the port reads through the pathGeneratorSource interface. Adapters
// snapshot the live state at construction: C++ re-reads the position in
// CalculatePath, but Go creature fields are only consistent under
// Server.motionMu (sync.Mutex, not re-entrant), so a snapshot taken under
// the caller's lock is the faithful unit — command arms construct then
// calculate immediately, and the testarea sweep snapshots under its lock.
// A movement generator that reuses a PathGenerator across ticks must
// reconstruct it per tick.
//
// Ported exactly:
//   - 9-value PathType (PathGenerator.h:40-52), MAX_PATH_LENGTH /
//     MAX_POINT_PATH_LENGTH 74, SMOOTH_PATH_STEP_SIZE 4.0, SMOOTH_PATH_SLOP
//     0.3 (PathGenerator.h:30-37); NavTerrainFlag values from
//     MapDefines.h:63-73 (GROUND 1, GROUND_STEEP 2, WATER 4, MAGMA_SLIME 8).
//   - CalculatePath incl. the no-navmesh / no-tile shortcut leg
//     (PathGenerator.cpp:84-106), BuildPolyPath incl. the hole, far-from-
//     poly, same-poly, subpath, suffix and raycast legs (:176-478),
//     BuildPointPath incl. the 1-poly/1-point append, SHORT and force-
//     destination legs (:480-580), BuildShortcut, NormalizePath,
//     FixupCorridor, GetSteerTarget, FindSmoothPath, ShortenPathUntilDist,
//     GetPathPolyByPosition / GetPolyByLocation (3.0/50.0 extents),
//     CreateFilter / UpdateFilter, GetNavTerrain, HaveTile,
//     AddFarFromPolyFlags, IsInvalidDestinationZ.
//   - UpdateAllowedPositionZ (Object.cpp:1458-1511) behind NormalizePath,
//     via Server.mapHeight / mapWaterOrGroundLevel (Map.cpp:2463-2487);
//     GetHitSpherePointFor (Object.cpp:1243-1250) behind
//     ShortenPathUntilDist; IsPathfindingEnabled (DisableMgr.cpp:418-422)
//     behind the constructor, via the cached server-level disables check
//     (DISABLE_TYPE_MMAP rows disable on mere presence, DisableMgr.cpp:
//     397-399).
//   - Unit::CanSwim (Unit.cpp:13318-13326): players are always swimmers
//     (UNIT_FLAG_PLAYER_CONTROLLED), creatures follow the movement
//     template (creatureCanSwimMotion).
//   - UNIT_STATE_IGNORE_PATHFINDING shortcut arm (PathGenerator.cpp:
//     102-104): the snapshot carries the live mask bit (creature
//     ScriptUnitState / session mask), so a flagged source takes the
//     BuildShortcut leg.
//   - Unit::GetHoverOffset (Unit.h:1648) in UpdateAllowedPositionZ
//     (Object.cpp:1458-1511): creatures read Hovering + HoverHeight,
//     players the hover aura + 1.0 field; the offset lifts the
//     ground/max clamps.
//
// Flagged gaps, NOT fabricated:
//   - Unit::IsFalling in the far-from-poly leg (:283-284): the creature
//     adapter reads creatureIsFalling (Unit::IsFalling twin: the FALLING
//     latch OR the MoveFall spline marker), so the falling shortcut fires
//     for falling creatures as well as falling players. The player adapter
//     reads the session falling flags, which are the live model.
//   - IsInEvadeMode: the Lua surface reads motion.Evading
//     (lua_creature_events.go:1628); the creature adapter snapshots that
//     field. There is no persistent bit beyond it.
//   - GetCollisionHeight: no per-creature model-data lookup exists in
//     Go, so the creature adapter uses terrainDefaultCollisionHeight
//     (2.03128, DEFAULT_COLLISION_HEIGHT, Object.h:69) — the C++ default
//     when no model data resolves. The player adapter resolves the race
//     display lookup with the same fallback.
//   - Transport sources: UpdateAllowedPositionZ's transport early-out
//     (Object.cpp:1460-1467) has no Go source model; path sources are
//     creatures and players, never transports.
//   - LoS in ShortenPathUntilDist rides Server.hasLineOfSight, which is
//     the ModelIgnoreFlags::Nothing twin (hasLineOfSightFlags with
//     ignoreM2=false, terrain.go:1128-1130) — the flag args are bridged;
//     what stays VMAP-only is the gameobject-collision leg, which Go has
//     no model for.
//   - C++ logs (TC_LOG_DEBUG/ERROR) are dropped; error legs keep their
//     exact control flow and return values.

package world

import (
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/detour"
)

// pathType mirrors PathType (PathGenerator.h:40-52).
type pathType uint32

const (
	pathfindBlank            pathType = 0x00
	pathfindNormal           pathType = 0x01
	pathfindShortcut         pathType = 0x02
	pathfindIncomplete       pathType = 0x04
	pathfindNopath           pathType = 0x08
	pathfindNotUsingPath     pathType = 0x10
	pathfindShort            pathType = 0x20
	pathfindFarFromPolyStart pathType = 0x40
	pathfindFarFromPolyEnd   pathType = 0x80
	pathfindFarFromPoly      pathType = pathfindFarFromPolyStart | pathfindFarFromPolyEnd
)

// navTerrainFlag mirrors NavTerrainFlag (MapDefines.h:63-73).
type navTerrainFlag uint16

const (
	navTerrainEmpty       navTerrainFlag = 0x00
	navTerrainGround      navTerrainFlag = 1
	navTerrainGroundSteep navTerrainFlag = 2
	navTerrainWater       navTerrainFlag = 4
	navTerrainMagmaSlime  navTerrainFlag = 8
)

const (
	maxPathLength      = 74
	maxPointPathLength = 74
	smoothPathStepSize = float32(4.0)
	smoothPathSlop     = float32(0.3)
	maxSteerPoints     = 3
	maxVisitPoly       = 16
	invalidPolyRef     = detour.PolyRef(0)
	pathRaycastNoHit   = float32(math.MaxFloat32)
)

// pathVec is a WoW-space (x, y, z) point: G3D::Vector3 in the C++.
type pathVec struct {
	X, Y, Z float32
}

func (v pathVec) sub(o pathVec) pathVec { return pathVec{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v pathVec) add(o pathVec) pathVec { return pathVec{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v pathVec) scale(s float32) pathVec {
	return pathVec{v.X * s, v.Y * s, v.Z * s}
}
func (v pathVec) lengthSqr() float32 { return v.X*v.X + v.Y*v.Y + v.Z*v.Z }
func (v pathVec) length() float32    { return float32(math.Sqrt(float64(v.lengthSqr()))) }

// direction mirrors G3D::Vector3::direction with a zero guard: C++
// divides by the length unguarded, which is NaN for a zero segment;
// the guard keeps a degenerate segment from poisoning the path.
func (v pathVec) direction() pathVec {
	if l := v.length(); l > 0 {
		return v.scale(1.0 / l)
	}
	return pathVec{}
}

// wowToDetour / detourToWow convert between WoW (x, y, z) and detour
// (y, z, x) order (PathGenerator.cpp:179-180).
func wowToDetour(p pathVec) detour.Vec3 { return detour.Vec3{p.Y, p.Z, p.X} }
func detourToWow(v detour.Vec3) pathVec { return pathVec{v[2], v[0], v[1]} }

func detourDistSqr(a, b detour.Vec3) float32 {
	dx, dy, dz := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return dx*dx + dy*dy + dz*dz
}

func detourDist(a, b detour.Vec3) float32 {
	return float32(math.Sqrt(float64(detourDistSqr(a, b))))
}

// pathGeneratorSource is the minimal WorldObject surface PathGenerator
// reads (PathGenerator.h: owner + the ToUnit/ToCreature queries in
// PathGenerator.cpp). All sources are units; isCreature selects the
// CreateFilter branch (TYPEID_UNIT vs the player else-branch).
type pathGeneratorSource interface {
	pathPosition() (x, y, z float32)
	pathMapID() uint32
	pathInstanceID() uint32
	pathPhaseMask() uint32
	pathIsCreature() bool
	pathCanWalk() bool
	pathCanSwim() bool
	pathCanFly() bool
	pathIsInWater() bool
	pathIsUnderWater() bool
	pathIsInCombat() bool
	pathIsInEvadeMode() bool
	pathIsFalling() bool
	pathCollisionHeight() float32
	pathCombatReach() float32
	pathIgnorePathfinding() bool
	pathHoverOffset() float32
}

// pathSourceSnapshot is the point-in-time source state. Adapters fill it
// from the live structs; PathGenerator only ever reads the snapshot.
type pathSourceSnapshot struct {
	x, y, z                  float32
	mapID, instanceID        uint32
	phaseMask                uint32
	creature                 bool
	walk, swim, fly          bool
	inWater, underWater      bool
	inCombat, evade, falling bool
	ignorePathfinding        bool
	hoverOffset              float32
	collisionHeight          float32
	combatReach              float32
}

func (s *pathSourceSnapshot) pathPosition() (float32, float32, float32) {
	return s.x, s.y, s.z
}
func (s *pathSourceSnapshot) pathMapID() uint32       { return s.mapID }
func (s *pathSourceSnapshot) pathInstanceID() uint32  { return s.instanceID }
func (s *pathSourceSnapshot) pathPhaseMask() uint32   { return s.phaseMask }
func (s *pathSourceSnapshot) pathIsCreature() bool    { return s.creature }
func (s *pathSourceSnapshot) pathCanWalk() bool       { return s.walk }
func (s *pathSourceSnapshot) pathCanSwim() bool       { return s.swim }
func (s *pathSourceSnapshot) pathCanFly() bool        { return s.fly }
func (s *pathSourceSnapshot) pathIsInWater() bool     { return s.inWater }
func (s *pathSourceSnapshot) pathIsUnderWater() bool  { return s.underWater }
func (s *pathSourceSnapshot) pathIsInCombat() bool    { return s.inCombat }
func (s *pathSourceSnapshot) pathIsInEvadeMode() bool { return s.evade }
func (s *pathSourceSnapshot) pathIsFalling() bool     { return s.falling }
func (s *pathSourceSnapshot) pathCollisionHeight() float32 {
	return s.collisionHeight
}
func (s *pathSourceSnapshot) pathCombatReach() float32 { return s.combatReach }
func (s *pathSourceSnapshot) pathIgnorePathfinding() bool {
	return s.ignorePathfinding
}
func (s *pathSourceSnapshot) pathHoverOffset() float32 { return s.hoverOffset }

// newCreaturePathSource snapshots a creatureMotion as a point read — the
// way the cast arms read a selected motion (castCommandSelectedCreature);
// callers already holding Server.motionMu get a consistent snapshot.
// Collision height has no per-creature Go model: terrainDefaultCollisionHeight
// is the C++ default (see the file header). Falling reads creatureIsFalling
// (Unit::IsFalling twin: the FALLING latch OR the MoveFall spline marker),
// so the far-from-poly falling shortcut fires for falling creatures.
func newCreaturePathSource(srv *Server, m *creatureMotion) pathGeneratorSource {
	if m == nil {
		return nil
	}
	phase := m.PhaseMask
	if phase == 0 {
		phase = 1 // PHASEMASK_NORMAL, as for unset motions
	}
	snap := &pathSourceSnapshot{
		x: m.X, y: m.Y, z: m.Z,
		mapID: m.Map, instanceID: m.InstanceID, phaseMask: phase,
		creature:        true,
		walk:            creatureCanWalkMotion(m),
		swim:            creatureCanSwimMotion(m),
		fly:             m.CanFly || m.Flying,
		inCombat:        m.InCombat,
		evade:           m.Evading,
		falling:         creatureIsFalling(m),
		collisionHeight: terrainDefaultCollisionHeight,
		combatReach:     m.combatReachOrDefault(),
	}
	// UNIT_STATE_IGNORE_PATHFINDING (PathGenerator.cpp:102-104) reads the
	// live mask: creatureUnitStateAdd writes ScriptUnitState
	// (lua_unit_state.go:196), and transport passengers carry the bit
	// (transports.go:1331). The direct field read matches the adapter's
	// other snapshot reads — the locking reader would deadlock under a
	// caller-held motionMu.
	if m.ScriptUnitState&unitStateIgnorePathfinding != 0 {
		snap.ignorePathfinding = true
	}
	// Unit::GetHoverOffset (Unit.h:1648): UNIT_FIELD_HOVERHEIGHT while
	// the HOVER movement flag is set, 0 otherwise. motion.Hovering is
	// the live flag, motion.HoverHeight the template field value.
	if m.Hovering {
		snap.hoverOffset = m.HoverHeight
	}
	if srv != nil {
		snap.inWater = srv.mapIsInWater(m.Map, m.X, m.Y, m.Z)
		snap.underWater = srv.mapIsUnderWater(m.Map, m.X, m.Y, m.Z)
	}
	return snap
}

// newPlayerPathSource snapshots a player session: players always walk
// and swim (Unit::CanSwim, Unit.cpp:13318-13326, PLAYER_CONTROLLED leg),
// fly only while the session reports flight, and fall per the session
// falling flags (lua_player.go:949). Collision height resolves the race
// display lookup (sendPlayerCollisionHeight) with the default fallback.
func newPlayerPathSource(sess *session) pathGeneratorSource {
	if sess == nil || sess.player == nil {
		return nil
	}
	p := sess.player
	snap := &pathSourceSnapshot{
		x: p.X, y: p.Y, z: p.Z,
		mapID: p.Map, instanceID: p.InstanceID,
		phaseMask:       sess.currentPlayerPhaseMask(),
		creature:        false,
		walk:            true,
		swim:            true,
		fly:             sess.inFlight,
		inCombat:        sess.isInCombat(),
		falling:         sess.isFalling || sess.isFallingFar,
		collisionHeight: terrainDefaultCollisionHeight,
		combatReach:     miscPlayerCombatReach(p),
	}
	// UNIT_STATE_IGNORE_PATHFINDING rides the session mask
	// (playerUnitStateHas, lua_unit_state.go:259).
	if sess.playerUnitStateHas(unitStateIgnorePathfinding) {
		snap.ignorePathfinding = true
	}
	// Unit::GetHoverOffset (Unit.h:1648): the field is written as 1.0
	// (player_state.go:4001) while the HOVER flag is set, 0 otherwise;
	// the flag rides the hover aura (characters.go:1758).
	if sess.hasAuraType(spellAuraHover) {
		snap.hoverOffset = 1.0
	}
	if sess.server != nil {
		snap.inWater = sess.server.mapIsInWater(p.Map, p.X, p.Y, p.Z)
		snap.underWater = sess.server.mapIsUnderWater(p.Map, p.X, p.Y, p.Z)
		if height, found, err := playerCollisionHeightFor(sess); err == nil && found {
			snap.collisionHeight = height
		}
	}
	return snap
}

// playerCollisionHeightFor is the sendPlayerCollisionHeight lookup
// (player_state.go:5043-5061) as a value: Unit::GetCollisionHeight
// (Unit.cpp:14122-14146) with the scale-modulated height.
func playerCollisionHeightFor(s *session) (float32, bool, error) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, false, nil
	}
	nativeDisplayID := uint32(0)
	if race, found, err := s.server.Data.Race(uint32(s.player.Race)); err == nil && found {
		nativeDisplayID = race.MaleDisplayID
		if s.player.Gender != 0 {
			nativeDisplayID = race.FemaleDisplayID
		}
	}
	return s.server.Data.CollisionHeight(s.player.MountDisplayID, nativeDisplayID, s.player.MountDisplayID != 0 || s.hasAuraType(spellAuraMounted), s.auraObjectScale())
}

// PathGenerator mirrors PathGenerator (PathGenerator.h:55-145).
type PathGenerator struct {
	srv          *Server
	source       pathGeneratorSource
	navMesh      *detour.NavMesh
	navMeshQuery *detour.NavMeshQuery
	filter       *detour.QueryFilter

	pathPolyRefs [maxPathLength]detour.PolyRef
	polyLength   uint32
	pathPoints   []pathVec
	pathType     pathType

	useStraightPath  bool
	forceDestination bool
	pointPathLimit   uint32
	useRaycast       bool

	startPosition     pathVec
	endPosition       pathVec
	actualEndPosition pathVec
}

// NewPathGenerator mirrors PathGenerator::PathGenerator
// (PathGenerator.cpp:35-53): the mesh/query resolve through Server.mmaps
// only when pathfinding is enabled for the map (DisableMgr::IsPathfindingEnabled,
// DisableMgr.cpp:418-422); a nil mesh/query is the C++-exact no-navmesh
// state and CalculatePath then takes the shortcut leg.
func NewPathGenerator(srv *Server, source pathGeneratorSource) *PathGenerator {
	g := &PathGenerator{
		srv:            srv,
		source:         source,
		pathType:       pathfindBlank,
		pointPathLimit: maxPointPathLength,
		filter:         detour.NewQueryFilter(),
	}
	if srv != nil && source != nil && srv.mmapPathfindingEnabled(source.pathMapID()) {
		mgr := srv.mmapManager()
		g.navMesh = mgr.GetNavMesh(source.pathMapID())
		g.navMeshQuery = mgr.GetNavMeshQuery(source.pathMapID(), source.pathInstanceID())
	}
	g.createFilter()
	return g
}

// SetUseStraightPath mirrors PathGenerator::SetUseStraightPath.
func (g *PathGenerator) SetUseStraightPath(useStraightPath bool) {
	g.useStraightPath = useStraightPath
}

// SetPathLengthLimit mirrors PathGenerator::SetPathLengthLimit:
// min(distance/SMOOTH_PATH_STEP_SIZE, MAX_POINT_PATH_LENGTH).
func (g *PathGenerator) SetPathLengthLimit(distance float32) {
	limit := uint32(distance / smoothPathStepSize)
	if limit > maxPointPathLength {
		limit = maxPointPathLength
	}
	g.pointPathLimit = limit
}

// SetUseRaycast mirrors PathGenerator::SetUseRaycast.
func (g *PathGenerator) SetUseRaycast(useRaycast bool) { g.useRaycast = useRaycast }

// StartPosition mirrors PathGenerator::GetStartPosition.
func (g *PathGenerator) StartPosition() pathVec { return g.startPosition }

// EndPosition mirrors PathGenerator::GetEndPosition.
func (g *PathGenerator) EndPosition() pathVec { return g.endPosition }

// ActualEndPosition mirrors PathGenerator::GetActualEndPosition.
func (g *PathGenerator) ActualEndPosition() pathVec { return g.actualEndPosition }

// PathPoints mirrors PathGenerator::GetPath.
func (g *PathGenerator) PathPoints() []pathVec { return g.pathPoints }

// PathType mirrors PathGenerator::GetPathType.
func (g *PathGenerator) PathType() pathType { return g.pathType }

// CalculatePath mirrors PathGenerator::CalculatePath
// (PathGenerator.cpp:62-106), including the UNIT_STATE_IGNORE_PATHFINDING
// shortcut arm (:102-104) via the snapshot mask.
func (g *PathGenerator) CalculatePath(destX, destY, destZ float32, forceDest bool) bool {
	// The shared per-(map, instance) query keeps its search state on
	// the query object, so concurrent queries corrupt each other;
	// navMeshQueryMu (mmap.go) serializes every CalculatePath, and the
	// .mmap loc arm takes the same lock for its direct FindNearestPoly.
	navMeshQueryMu.Lock()
	defer navMeshQueryMu.Unlock()
	x, y, z := g.source.pathPosition()
	if !validMovementPosition(destX, destY, destZ, 0) || !validMovementPosition(x, y, z, 0) {
		return false
	}

	dest := pathVec{destX, destY, destZ}
	g.endPosition = dest
	g.actualEndPosition = dest

	start := pathVec{x, y, z}
	g.startPosition = start

	g.forceDestination = forceDest

	// make sure navMesh works - we can run on map w/o mmap
	// (PathGenerator.cpp:84-90): the UNIT_STATE_IGNORE_PATHFINDING arm
	// (:102-104) rides the snapshot mask (see the adapters).
	if g.navMesh == nil || g.navMeshQuery == nil || g.source.pathIgnorePathfinding() || !g.haveTile(start) || !g.haveTile(dest) {
		g.buildShortcut()
		g.pathType = pathfindNormal | pathfindNotUsingPath
		return true
	}

	g.updateFilter()

	g.buildPolyPath(start, dest)
	return true
}

// IsInvalidDestinationZ mirrors PathGenerator::IsInvalidDestinationZ
// (PathGenerator.cpp:1001-1004).
func (g *PathGenerator) IsInvalidDestinationZ(targetZ float32) bool {
	return (targetZ - g.actualEndPosition.Z) > 5.0
}

// ShortenPathUntilDist mirrors PathGenerator::ShortenPathUntilDist
// (PathGenerator.cpp:936-999).
func (g *PathGenerator) ShortenPathUntilDist(target pathVec, dist float32) {
	if g.pathType == pathfindBlank || len(g.pathPoints) < 2 {
		return
	}

	distSq := dist * dist

	// the first point of the path must be outside the specified range
	if g.pathPoints[0].sub(target).lengthSqr() < distSq {
		return
	}

	// check if we even need to do anything
	if g.pathPoints[len(g.pathPoints)-1].sub(target).lengthSqr() >= distSq {
		return
	}

	i := len(g.pathPoints) - 1
	collisionHeight := g.source.pathCollisionHeight()
	for {
		// we know that pathPoints[i] is too close already (from the
		// previous iteration)
		if g.pathPoints[i-1].sub(target).lengthSqr() >= distSq {
			break // bingo!
		}

		// check if the shortened path is still in LoS with the target
		hit := g.hitSpherePointFor(pathVec{g.pathPoints[i-1].X, g.pathPoints[i-1].Y, g.pathPoints[i-1].Z + collisionHeight})
		if g.srv != nil && !g.srv.hasLineOfSight(g.source.pathMapID(), hit.X, hit.Y, hit.Z, g.pathPoints[i-1].X, g.pathPoints[i-1].Y, g.pathPoints[i-1].Z+collisionHeight) {
			// whenver we find a point that is not in LoS anymore,
			// simply use last valid path
			g.pathPoints = g.pathPoints[:i+1]
			return
		}

		i--
		if i == 0 {
			// no point found that fulfills the condition
			g.pathPoints[0] = g.pathPoints[1]
			g.pathPoints = g.pathPoints[:2]
			return
		}
	}

	// ok, _pathPoints[i] is too close, _pathPoints[i-1] is not, so our
	// target point is somewhere between the two...
	g.pathPoints[i] = g.pathPoints[i].add(g.pathPoints[i-1].sub(g.pathPoints[i]).direction().scale(dist - g.pathPoints[i].sub(target).length()))
	g.pathPoints = g.pathPoints[:i+1]
}

// hitSpherePointFor mirrors WorldObject::GetHitSpherePointFor
// (Object.cpp:1243-1250).
func (g *PathGenerator) hitSpherePointFor(dest pathVec) pathVec {
	sx, sy, sz := g.source.pathPosition()
	vThis := pathVec{sx, sy, sz + g.source.pathCollisionHeight()}
	delta := dest.sub(vThis)
	dir := delta.direction()
	reach := g.source.pathCombatReach()
	d := dest.sub(pathVec{sx, sy, sz}).length()
	t := d
	if reach < t {
		t = reach
	}
	return vThis.add(dir.scale(t))
}

// normalizePath mirrors PathGenerator::NormalizePath
// (PathGenerator.cpp:582-586).
func (g *PathGenerator) normalizePath() {
	for i := range g.pathPoints {
		g.updateAllowedPositionZ(g.pathPoints[i].X, g.pathPoints[i].Y, &g.pathPoints[i].Z)
	}
}

// updateAllowedPositionZ mirrors WorldObject::UpdateAllowedPositionZ
// (Object.cpp:1458-1511) for a unit source: flyers clamp up to ground,
// swimmers clamp into [ground, water-or-ground], walkers clamp into
// [ground, ground]; hovering units cannot go below their hover height
// (GetHoverOffset lifts both clamps). A nil server leaves Z untouched.
// The transport early-out (:1460-1467) has no Go source model.
func (g *PathGenerator) updateAllowedPositionZ(x, y float32, z *float32) {
	if g.srv == nil {
		return
	}
	mapID := g.source.pathMapID()
	// Unit::GetHoverOffset (Unit.h:1648): hovering units cannot go below
	// their hover height — the offset lifts the ground/max clamps below
	// (Object.cpp:1458-1511).
	hoverOffset := g.source.pathHoverOffset()
	if g.source.pathCanFly() {
		ground, ok := g.srv.mapHeight(mapID, x, y, *z, true, terrainDefaultHeightSearch)
		if !ok {
			ground = terrainInvalidHeight
		} else {
			ground += hoverOffset
		}
		if *z < ground {
			*z = ground
		}
		return
	}
	var groundZ, maxZ float32
	if g.source.pathCanSwim() {
		maxZ, groundZ = g.srv.mapWaterOrGroundLevel(mapID, x, y, *z, g.source.pathCollisionHeight())
	} else {
		groundZ, ok := g.srv.mapHeight(mapID, x, y, *z, true, terrainDefaultHeightSearch)
		if !ok {
			groundZ = terrainInvalidHeight
		}
		maxZ = groundZ
	}
	if maxZ > terrainInvalidHeight {
		maxZ += hoverOffset
		groundZ += hoverOffset
		if *z > maxZ {
			*z = maxZ
		} else if *z < groundZ {
			*z = groundZ
		}
	}
}

// mapWaterOrGroundLevel mirrors Map::GetWaterOrGroundLevel
// (Map.cpp:2463-2487): the water level when the point is above water,
// the ground when there is no water, the liquid level otherwise;
// terrainInvalidHeight when no height resolves.
func (s *Server) mapWaterOrGroundLevel(mapID uint32, x, y, z, collisionHeight float32) (float32, float32) {
	if s == nil {
		return terrainInvalidHeight, terrainInvalidHeight
	}
	ground, ok := s.mapHeight(mapID, x, y, z+collisionHeight, true, terrainDefaultHeightSearch)
	if !ok {
		return terrainInvalidHeight, terrainInvalidHeight
	}
	status, data := s.mapLiquidStatus(mapID, x, y, ground, liquidTypeAll, collisionHeight)
	switch status {
	case liquidStatusAboveWater:
		if data.level > ground {
			return data.level, ground
		}
		return ground, ground
	case liquidStatusNoWater:
		return ground, ground
	default:
		return data.level, ground
	}
}

// buildShortcut mirrors PathGenerator::BuildShortcut
// (PathGenerator.cpp:588-600).
func (g *PathGenerator) buildShortcut() {
	g.clear()

	g.pathPoints = []pathVec{g.startPosition, g.actualEndPosition}

	g.normalizePath()

	g.pathType = pathfindShortcut
}

// clear mirrors PathGenerator::Clear (PathGenerator.h:120-124).
func (g *PathGenerator) clear() {
	g.polyLength = 0
	g.pathPoints = nil
}

func (g *PathGenerator) inRange(p1, p2 pathVec, r, h float32) bool {
	d := p1.sub(p2)
	return (d.X*d.X+d.Y*d.Y) < r*r && float32(math.Abs(float64(d.Z))) < h
}

func (g *PathGenerator) dist3DSqr(p1, p2 pathVec) float32 {
	return p1.sub(p2).lengthSqr()
}

// inRangeYZX mirrors PathGenerator::InRangeYZX (PathGenerator.cpp:918-924).
func (g *PathGenerator) inRangeYZX(v1, v2 detour.Vec3, r, h float32) bool {
	dx := v2[0] - v1[0]
	dy := v2[1] - v1[1] // elevation
	dz := v2[2] - v1[2]
	return (dx*dx+dz*dz) < r*r && float32(math.Abs(float64(dy))) < h
}

// getPathPolyByPosition mirrors PathGenerator::GetPathPolyByPosition
// (PathGenerator.cpp:109-139): the nearest poly in polyPath by 3D
// squared distance, INVALID_POLYREF unless within 3.0 (squared-distance
// 3.0, as in C++). distance is set even on the INVALID return.
func (g *PathGenerator) getPathPolyByPosition(polyPath []detour.PolyRef, point detour.Vec3, distance *float32) detour.PolyRef {
	if len(polyPath) == 0 {
		return invalidPolyRef
	}

	nearestPoly := invalidPolyRef
	minDist := float32(math.MaxFloat32)

	for _, ref := range polyPath {
		closestPoint, _, st := g.navMeshQuery.ClosestPointOnPoly(ref, point)
		if detour.StatusFailed(st) {
			continue
		}

		d := detourDistSqr(point, closestPoint)
		if d < minDist {
			minDist = d
			nearestPoly = ref
		}

		if minDist < 1.0 { // shortcut out - close enough for us
			break
		}
	}

	if distance != nil {
		*distance = float32(math.Sqrt(float64(minDist)))
	}

	if minDist < 3.0 {
		return nearestPoly
	}
	return invalidPolyRef
}

// getPolyByLocation mirrors PathGenerator::GetPolyByLocation
// (PathGenerator.cpp:141-174).
func (g *PathGenerator) getPolyByLocation(point detour.Vec3, distance *float32) detour.PolyRef {
	polyRef := g.getPathPolyByPosition(g.pathPolyRefs[:g.polyLength], point, distance)
	if polyRef != invalidPolyRef {
		return polyRef
	}

	// first try with low search box
	extents := detour.Vec3{3.0, 5.0, 3.0}
	if ref, closestPoint, st := g.navMeshQuery.FindNearestPoly(point, extents, g.filter); detour.StatusSucceed(st) && ref != invalidPolyRef {
		*distance = detourDist(closestPoint, point)
		return ref
	}

	// try with bigger search box
	extents[1] = 50.0
	if ref, closestPoint, st := g.navMeshQuery.FindNearestPoly(point, extents, g.filter); detour.StatusSucceed(st) && ref != invalidPolyRef {
		*distance = detourDist(closestPoint, point)
		return ref
	}

	*distance = float32(math.MaxFloat32)
	return invalidPolyRef
}

// haveTile mirrors PathGenerator::HaveTile (PathGenerator.cpp:724-739).
func (g *PathGenerator) haveTile(p pathVec) bool {
	point := wowToDetour(p)
	tx, ty := g.navMesh.CalcTileLoc(&point)
	if tx < 0 || ty < 0 {
		return false
	}
	return g.navMesh.GetTileAt(tx, ty, 0) != nil
}

// addFarFromPolyFlags mirrors PathGenerator::AddFarFromPolyFlags
// (PathGenerator.cpp:1006-1012).
func (g *PathGenerator) addFarFromPolyFlags(startFarFromPoly, endFarFromPoly bool) {
	if startFarFromPoly {
		g.pathType |= pathfindFarFromPolyStart
	}
	if endFarFromPoly {
		g.pathType |= pathfindFarFromPolyEnd
	}
}

// buildPolyPath mirrors PathGenerator::BuildPolyPath
// (PathGenerator.cpp:176-478).
func (g *PathGenerator) buildPolyPath(startPos, endPos pathVec) {
	var distToStartPoly, distToEndPoly float32
	startPoint := wowToDetour(startPos)
	endPoint := wowToDetour(endPos)

	startPoly := g.getPolyByLocation(startPoint, &distToStartPoly)
	endPoly := g.getPolyByLocation(endPoint, &distToEndPoly)

	g.pathType = pathfindNormal

	// we have a hole in our mesh
	if startPoly == invalidPolyRef || endPoly == invalidPolyRef {
		g.buildShortcut()
		path := g.source.pathIsCreature() && g.source.pathCanFly()

		waterPath := g.source.pathIsCreature() && g.source.pathCanSwim()
		if waterPath {
			// Check both start and end points, if they're both in
			// water, then we can *safely* let the creature move
			for _, p := range g.pathPoints {
				status, _ := g.srv.mapLiquidStatus(g.source.pathMapID(), p.X, p.Y, p.Z, liquidTypeAll, g.source.pathCollisionHeight())
				// One of the points is not in the water, cancel movement.
				if status == liquidStatusNoWater {
					waterPath = false
					break
				}
			}
		}

		if path || waterPath {
			g.pathType = pathfindNormal | pathfindNotUsingPath
			return
		}

		// raycast doesn't need endPoly to be valid
		if !g.useRaycast {
			g.pathType = pathfindNopath
			return
		}
	}

	// we may need a better number here
	startFarFromPoly := distToStartPoly > 7.0
	endFarFromPoly := distToEndPoly > 7.0
	if startFarFromPoly || endFarFromPoly {
		var p pathVec
		if distToStartPoly > 7.0 {
			p = startPos
		} else {
			p = endPos
		}
		buildShortcut := false

		if g.srv.mapIsUnderWater(g.source.pathMapID(), p.X, p.Y, p.Z) {
			if g.source.pathCanSwim() {
				buildShortcut = true
			}
		} else {
			if g.source.pathCanFly() {
				buildShortcut = true
				// Allow to build a shortcut if the unit is falling and
				// it's trying to move downwards towards a target (i.e.
				// charging). Both adapters report Unit::IsFalling: the
				// creature adapter reads the FALLING latch OR the MoveFall
				// spline marker, the player adapter the session flags.
			} else if g.source.pathIsFalling() && endPos.Z < startPos.Z {
				buildShortcut = true
			}
		}

		if buildShortcut {
			g.buildShortcut()
			g.pathType = pathfindNormal | pathfindNotUsingPath

			g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)

			return
		}

		closestPoint, _, st := g.navMeshQuery.ClosestPointOnPoly(endPoly, endPoint)
		// we may want to use closestPointOnPolyBoundary instead
		if detour.StatusSucceed(st) {
			endPoint = closestPoint
			g.actualEndPosition = detourToWow(endPoint)
		}

		g.pathType = pathfindIncomplete

		g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)
	}

	// start and end are on same polygon
	if startPoly == endPoly && !g.useRaycast {
		g.pathPolyRefs[0] = startPoly
		g.polyLength = 1

		if startFarFromPoly || endFarFromPoly {
			g.pathType = pathfindIncomplete

			g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)
		} else {
			g.pathType = pathfindNormal
		}

		g.buildPointPath(startPoint, endPoint)
		return
	}

	// look for startPoly/endPoly in current path
	startPolyFound := false
	endPolyFound := false
	pathStartIndex := 0
	pathEndIndex := 0

	if g.polyLength > 0 {
		for ; pathStartIndex < int(g.polyLength); pathStartIndex++ {
			// here to catch few bugs
			if g.pathPolyRefs[pathStartIndex] == invalidPolyRef {
				break
			}

			if g.pathPolyRefs[pathStartIndex] == startPoly {
				startPolyFound = true
				break
			}
		}

		for pathEndIndex = int(g.polyLength) - 1; pathEndIndex > pathStartIndex; pathEndIndex-- {
			if g.pathPolyRefs[pathEndIndex] == endPoly {
				endPolyFound = true
				break
			}
		}
	}

	if startPolyFound && endPolyFound {
		// we moved along the path and the target did not move out of
		// our old poly-path: just "cut" it out
		g.polyLength = uint32(pathEndIndex - pathStartIndex + 1)
		copy(g.pathPolyRefs[:g.polyLength], g.pathPolyRefs[pathStartIndex:pathStartIndex+int(g.polyLength)])
	} else if startPolyFound && !endPolyFound {
		// we are moving on the old path but target moved out: keep a
		// ~80% prefix and generate the suffix
		g.polyLength -= uint32(pathStartIndex)

		// take ~80% of the original length
		prefixPolyLength := uint32(float32(g.polyLength)*0.8 + 0.5)
		copy(g.pathPolyRefs[:prefixPolyLength], g.pathPolyRefs[pathStartIndex:pathStartIndex+int(prefixPolyLength)])

		suffixStartPoly := g.pathPolyRefs[prefixPolyLength-1]

		// we need any point on our suffix start poly to generate
		// poly-path, so we need last poly in prefix data
		suffixEndPoint, _, st := g.navMeshQuery.ClosestPointOnPoly(suffixStartPoly, endPoint)
		if detour.StatusFailed(st) {
			// we can hit offmesh connection as last poly -
			// closestPointOnPoly() don't like that; try to recover
			// by using prev polyref
			prefixPolyLength--
			if prefixPolyLength == 0 {
				// degenerate: no prefix poly left to recover
				// from (C++ indexes _pathPolyRefs[-1] here).
				g.buildShortcut()
				g.pathType = pathfindNopath
				return
			}
			suffixStartPoly = g.pathPolyRefs[prefixPolyLength-1]
			suffixEndPoint, _, st = g.navMeshQuery.ClosestPointOnPoly(suffixStartPoly, endPoint)
			if detour.StatusFailed(st) {
				// suffixStartPoly is still invalid, error state
				g.buildShortcut()
				g.pathType = pathfindNopath
				return
			}
		}

		// generate suffix
		var suffixPolyLength uint32

		var dtResult detour.Status
		if g.useRaycast {
			g.buildShortcut()
			g.pathType = pathfindNopath
			return
		}
		suffix, st := g.navMeshQuery.FindPath(
			suffixStartPoly,                     // start polygon
			endPoly,                             // end polygon
			suffixEndPoint,                      // start position
			endPoint,                            // end position
			g.filter,                            // polygon search filter
			maxPathLength-int(prefixPolyLength)) // max number of polygons in output path
		dtResult = st
		suffixPolyLength = uint32(len(suffix))
		copy(g.pathPolyRefs[prefixPolyLength-1:], suffix)

		if suffixPolyLength == 0 || detour.StatusFailed(dtResult) {
			// this is probably an error state, but we'll leave it
			// and hopefully recover on the next Update
		}

		// new path = prefix + suffix - overlap
		g.polyLength = prefixPolyLength + suffixPolyLength - 1
		if g.polyLength == 0 {
			// degenerate: the suffix failed and the prefix was a
			// single poly (C++ indexes _pathPolyRefs[-1] here).
			g.buildShortcut()
			g.pathType = pathfindNopath
			return
		}
	} else {
		// either we have no path at all -> first run, or something
		// went really wrong: just generate new path
		g.clear()

		if g.useRaycast {
			hit, hitNormal, path, dtResult := g.navMeshQuery.RaycastSimple(
				startPoly,
				startPoint,
				endPoint,
				g.filter,
				maxPathLength)
			_ = hitNormal
			copy(g.pathPolyRefs[:], path)
			g.polyLength = uint32(len(path))

			if g.polyLength == 0 || detour.StatusFailed(dtResult) {
				g.buildShortcut()
				g.pathType = pathfindNopath
				g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)
				return
			}

			// raycast() sets hit to FLT_MAX if there is a ray
			// between start and end
			if hit != pathRaycastNoHit {
				// Walk back a bit from the hit point to make sure
				// it's in the mesh (sometimes the point is actually
				// outside of the polygons due to float precision
				// issues)
				hit *= 0.99
				hitPos := detour.Vec3{
					startPoint[0] + (endPoint[0]-startPoint[0])*hit,
					startPoint[1] + (endPoint[1]-startPoint[1])*hit,
					startPoint[2] + (endPoint[2]-startPoint[2])*hit,
				}

				// if it fails again, clamp to poly boundary
				if h, st := g.navMeshQuery.GetPolyHeight(g.pathPolyRefs[g.polyLength-1], hitPos); detour.StatusFailed(st) {
					hitPos, _ = g.navMeshQuery.ClosestPointOnPolyBoundary(g.pathPolyRefs[g.polyLength-1], hitPos)
				} else {
					hitPos[1] = h
				}

				g.pathPoints = []pathVec{g.startPosition, detourToWow(hitPos)}

				g.normalizePath()
				g.pathType = pathfindIncomplete
				g.addFarFromPolyFlags(startFarFromPoly, false)
				return
			}

			// clamp to poly boundary if we fail to get the height
			if h, st := g.navMeshQuery.GetPolyHeight(g.pathPolyRefs[g.polyLength-1], endPoint); detour.StatusFailed(st) {
				endPoint, _ = g.navMeshQuery.ClosestPointOnPolyBoundary(g.pathPolyRefs[g.polyLength-1], endPoint)
			} else {
				endPoint[1] = h
			}

			g.pathPoints = []pathVec{g.startPosition, detourToWow(endPoint)}

			g.normalizePath()
			if startFarFromPoly || endFarFromPoly {
				g.pathType = pathfindIncomplete

				g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)
			} else {
				g.pathType = pathfindNormal
			}
			return
		}

		path, dtResult := g.navMeshQuery.FindPath(
			startPoly,     // start polygon
			endPoly,       // end polygon
			startPoint,    // start position
			endPoint,      // end position
			g.filter,      // polygon search filter
			maxPathLength) // max number of polygons in output path
		copy(g.pathPolyRefs[:], path)
		g.polyLength = uint32(len(path))

		if g.polyLength == 0 || detour.StatusFailed(dtResult) {
			// only happens if we passed bad data to findPath(), or
			// navmesh is messed up
			g.buildShortcut()
			g.pathType = pathfindNopath
			return
		}
	}

	// by now we know what type of path we can get
	if g.pathPolyRefs[g.polyLength-1] == endPoly && (g.pathType&pathfindIncomplete) == 0 {
		g.pathType = pathfindNormal
	} else {
		g.pathType = pathfindIncomplete
	}

	g.addFarFromPolyFlags(startFarFromPoly, endFarFromPoly)

	// generate the point-path out of our up-to-date poly-path
	g.buildPointPath(startPoint, endPoint)
}

// buildPointPath mirrors PathGenerator::BuildPointPath
// (PathGenerator.cpp:480-580).
func (g *PathGenerator) buildPointPath(startPoint, endPoint detour.Vec3) {
	var pointCount int
	var dtResult detour.Status
	var points []detour.Vec3
	if g.useRaycast {
		// _straightLine uses raycast and it currently doesn't support
		// building a point path, only a 2-point path with start and
		// hitpoint/end is returned
		g.buildShortcut()
		g.pathType = pathfindNopath
		return
	} else if g.useStraightPath {
		points, _, _, dtResult = g.navMeshQuery.FindStraightPath(
			startPoint,                    // start position
			endPoint,                      // end position
			g.pathPolyRefs[:g.polyLength], // current path
			int(g.pointPathLimit),         // maximum number of points/polygons to use
			0)
		pointCount = len(points)
	} else {
		points, dtResult = g.findSmoothPath(
			startPoint,                    // start position
			endPoint,                      // end position
			g.pathPolyRefs[:g.polyLength], // current path
			int(g.pointPathLimit))         // maximum number of points
		pointCount = len(points)
	}

	// Special case with start and end positions very close to each other
	if g.polyLength == 1 && pointCount == 1 {
		// First point is start position, append end position
		points = append(points, endPoint)
		pointCount++
	} else if pointCount < 2 || detour.StatusFailed(dtResult) {
		// only happens if pass bad data to findStraightPath or navmesh
		// is broken; single point paths can be generated here
		g.buildShortcut()
		g.pathType |= pathfindNopath
		return
	} else if pointCount >= int(g.pointPathLimit) {
		g.buildShortcut()
		g.pathType |= pathfindShort
		return
	}

	g.pathPoints = make([]pathVec, pointCount)
	for i := 0; i < pointCount; i++ {
		g.pathPoints[i] = detourToWow(points[i])
	}

	g.normalizePath()

	// first point is always our current location - we need the next one
	g.actualEndPosition = g.pathPoints[pointCount-1]

	// force the given destination, if needed
	if g.forceDestination &&
		((g.pathType&pathfindNormal) == 0 || !g.inRange(g.endPosition, g.actualEndPosition, 1.0, 1.0)) {
		// we may want to keep partial subpath
		if g.dist3DSqr(g.actualEndPosition, g.endPosition) < 0.3*g.dist3DSqr(g.startPosition, g.endPosition) {
			g.actualEndPosition = g.endPosition
			g.pathPoints[len(g.pathPoints)-1] = g.endPosition
		} else {
			g.actualEndPosition = g.endPosition
			g.buildShortcut()
		}

		g.pathType = pathfindNormal | pathfindNotUsingPath
	}
}

// createFilter mirrors PathGenerator::CreateFilter
// (PathGenerator.cpp:602-628).
func (g *PathGenerator) createFilter() {
	var includeFlags uint16
	var excludeFlags uint16

	if g.source.pathIsCreature() {
		if g.source.pathCanWalk() {
			includeFlags |= uint16(navTerrainGround) // walk
		}

		// creatures don't take environmental damage
		if g.source.pathCanSwim() {
			includeFlags |= uint16(navTerrainWater | navTerrainMagmaSlime) // swim
		}
	} else { // assume Player
		// perfect support not possible, just stay 'safe'
		includeFlags |= uint16(navTerrainGround | navTerrainWater | navTerrainMagmaSlime)
	}

	g.filter.SetIncludeFlags(includeFlags)
	g.filter.SetExcludeFlags(excludeFlags)

	g.updateFilter()
}

// updateFilter mirrors PathGenerator::UpdateFilter
// (PathGenerator.cpp:630-649).
func (g *PathGenerator) updateFilter() {
	// allow creatures to cheat and use different movement types if they
	// are moved forcefully into terrain they can't normally move in
	if g.source.pathIsInWater() || g.source.pathIsUnderWater() {
		x, y, z := g.source.pathPosition()
		includedFlags := g.filter.GetIncludeFlags()
		includedFlags |= uint16(g.getNavTerrain(x, y, z))
		g.filter.SetIncludeFlags(includedFlags)
	}

	if g.source.pathIsCreature() {
		if g.source.pathIsInCombat() || g.source.pathIsInEvadeMode() {
			g.filter.SetIncludeFlags(g.filter.GetIncludeFlags() | uint16(navTerrainGroundSteep))
		}
	}
}

// getNavTerrain mirrors PathGenerator::GetNavTerrain
// (PathGenerator.cpp:651-670).
func (g *PathGenerator) getNavTerrain(x, y, z float32) navTerrainFlag {
	if g.srv == nil {
		return navTerrainGround
	}
	status, data := g.srv.mapLiquidStatus(g.source.pathMapID(), x, y, z, liquidTypeAll, g.source.pathCollisionHeight())
	if status == liquidStatusNoWater {
		return navTerrainGround
	}

	switch data.typeFlags {
	case liquidTypeWater, liquidTypeOcean:
		return navTerrainWater
	case liquidTypeMagma, liquidTypeSlime:
		return navTerrainMagmaSlime
	default:
		return navTerrainGround
	}
}

// fixupCorridor mirrors PathGenerator::FixupCorridor
// (PathGenerator.cpp:741-779).
func (g *PathGenerator) fixupCorridor(path []detour.PolyRef, npath, maxPath int, visited []detour.PolyRef) int {
	nvisited := len(visited)
	furthestPath := -1
	furthestVisited := -1

	// Find furthest common polygon.
	for i := npath - 1; i >= 0; i-- {
		found := false
		for j := nvisited - 1; j >= 0; j-- {
			if path[i] == visited[j] {
				furthestPath = i
				furthestVisited = j
				found = true
			}
		}
		if found {
			break
		}
	}

	// If no intersection found just return current path.
	if furthestPath == -1 || furthestVisited == -1 {
		return npath
	}

	// Concatenate paths.

	// Adjust beginning of the buffer to include the visited.
	req := nvisited - furthestVisited
	orig := furthestPath + 1
	if orig > npath {
		orig = npath
	}
	size := 0
	if npath > orig {
		size = npath - orig
	}
	if req+size > maxPath {
		size = maxPath - req
	}

	if size > 0 {
		copy(path[req:req+size], path[orig:orig+size])
	}

	// Store visited
	for i := 0; i < req; i++ {
		path[i] = visited[(nvisited-1)-i]
	}

	return req + size
}

// getSteerTarget mirrors PathGenerator::GetSteerTarget
// (PathGenerator.cpp:781-815).
func (g *PathGenerator) getSteerTarget(startPos, endPos detour.Vec3, minTargetDist float32, path []detour.PolyRef) (steerPos detour.Vec3, steerPosFlag uint8, steerPosRef detour.PolyRef, ok bool) {
	// Find steer target.
	steerPath, steerPathFlags, steerPathPolys, dtResult := g.navMeshQuery.FindStraightPath(startPos, endPos, path, maxSteerPoints, 0)
	nsteerPath := len(steerPath)
	if nsteerPath == 0 || detour.StatusFailed(dtResult) {
		return steerPos, 0, invalidPolyRef, false
	}

	// Find vertex far enough to steer to.
	ns := 0
	for ns < nsteerPath {
		// Stop at Off-Mesh link or when point is further than slop away.
		if (steerPathFlags[ns]&detour.DT_STRAIGHTPATH_OFFMESH_CONNECTION) != 0 ||
			!g.inRangeYZX(steerPath[ns], startPos, minTargetDist, 1000.0) {
			break
		}
		ns++
	}
	// Failed to find good point to steer to.
	if ns >= nsteerPath {
		return steerPos, 0, invalidPolyRef, false
	}

	steerPos = steerPath[ns]
	steerPos[1] = startPos[1] // keep Z value
	steerPosFlag = steerPathFlags[ns]
	steerPosRef = steerPathPolys[ns]

	return steerPos, steerPosFlag, steerPosRef, true
}

// findSmoothPath mirrors PathGenerator::FindSmoothPath
// (PathGenerator.cpp:817-933). Returns the smooth path points and the
// dtStatus: DT_SUCCESS unless the point count reached
// MAX_POINT_PATH_LENGTH.
func (g *PathGenerator) findSmoothPath(startPos, endPos detour.Vec3, polyPath []detour.PolyRef, maxSmoothPathSize int) ([]detour.Vec3, detour.Status) {
	var smoothPath []detour.Vec3
	polyPathSize := len(polyPath)

	polys := make([]detour.PolyRef, maxPathLength)
	copy(polys, polyPath)
	npolys := polyPathSize

	var iterPos, targetPos detour.Vec3

	if polyPathSize > 1 {
		// Pick the closest points on poly border
		pos, st := g.navMeshQuery.ClosestPointOnPolyBoundary(polys[0], startPos)
		if detour.StatusFailed(st) {
			return smoothPath, detour.DT_FAILURE
		}
		iterPos = pos

		pos, st = g.navMeshQuery.ClosestPointOnPolyBoundary(polys[npolys-1], endPos)
		if detour.StatusFailed(st) {
			return smoothPath, detour.DT_FAILURE
		}
		targetPos = pos
	} else {
		// Case where the path is on the same poly
		iterPos = startPos
		targetPos = endPos
	}

	smoothPath = append(smoothPath, iterPos)

	// Move towards target a small advancement at a time until target
	// reached or when ran out of memory to store the path.
	for npolys > 0 && len(smoothPath) < maxSmoothPathSize {
		// Find location to steer towards.
		steerPos, steerPosFlag, steerPosRef, ok := g.getSteerTarget(iterPos, targetPos, smoothPathSlop, polys[:npolys])
		if !ok {
			break
		}

		endOfPath := (steerPosFlag & detour.DT_STRAIGHTPATH_END) != 0
		offMeshConnection := (steerPosFlag & detour.DT_STRAIGHTPATH_OFFMESH_CONNECTION) != 0

		// Find movement delta.
		delta := detour.Vec3{steerPos[0] - iterPos[0], steerPos[1] - iterPos[1], steerPos[2] - iterPos[2]}
		length := float32(math.Sqrt(float64(delta[0]*delta[0] + delta[1]*delta[1] + delta[2]*delta[2])))
		// If the steer target is end of path or off-mesh link, do not
		// move past the location.
		var step float32
		if (endOfPath || offMeshConnection) && length < smoothPathStepSize {
			step = 1.0
		} else {
			step = smoothPathStepSize / length
		}

		moveTgt := detour.Vec3{iterPos[0] + delta[0]*step, iterPos[1] + delta[1]*step, iterPos[2] + delta[2]*step}

		// Move
		result, visited, st := g.navMeshQuery.MoveAlongSurface(polys[0], iterPos, moveTgt, g.filter, maxVisitPoly)
		if detour.StatusFailed(st) {
			return smoothPath, detour.DT_FAILURE
		}
		npolys = g.fixupCorridor(polys, npolys, maxPathLength, visited)

		if h, st := g.navMeshQuery.GetPolyHeight(polys[0], result); detour.StatusFailed(st) {
			// Cannot find height at position; keep the
			// MoveAlongSurface height, as in C++ (log only).
		} else {
			result[1] = h
		}
		result[1] += 0.5
		iterPos = result

		// Handle end of path and off-mesh links when close enough.
		if endOfPath && g.inRangeYZX(iterPos, steerPos, smoothPathSlop, 1.0) {
			// Reached end of path.
			iterPos = targetPos
			if len(smoothPath) < maxSmoothPathSize {
				smoothPath = append(smoothPath, iterPos)
			}
			break
		} else if offMeshConnection && g.inRangeYZX(iterPos, steerPos, smoothPathSlop, 1.0) {
			// Advance the path up to and over the off-mesh connection.
			prevRef := invalidPolyRef
			polyRef := polys[0]
			npos := 0
			for npos < npolys && polyRef != steerPosRef {
				prevRef = polyRef
				polyRef = polys[npos]
				npos++
			}

			copy(polys, polys[npos:npolys])
			npolys -= npos

			// Handle the connection.
			if npolys == 0 {
				// degenerate: the connection was the last poly,
				// so polys[0] below would read past the path
				// (C++ reads stale array memory here).
				return smoothPath, detour.DT_FAILURE
			}
			connectionStartPos, connectionEndPos, st := g.navMesh.GetOffMeshConnectionPolyEndPoints(prevRef, polyRef)
			if detour.StatusSucceed(st) {
				if len(smoothPath) < maxSmoothPathSize {
					smoothPath = append(smoothPath, connectionStartPos)
				}
				// Move position at the other side of the off-mesh link.
				iterPos = connectionEndPos
				if h, st := g.navMeshQuery.GetPolyHeight(polys[0], iterPos); detour.StatusFailed(st) {
					return smoothPath, detour.DT_FAILURE
				} else {
					iterPos[1] = h
				}
				iterPos[1] += 0.5
			}
		}

		// Store results.
		if len(smoothPath) < maxSmoothPathSize {
			smoothPath = append(smoothPath, iterPos)
		}
	}

	// this is most likely a loop
	if len(smoothPath) < maxPointPathLength {
		return smoothPath, detour.DT_SUCCESS
	}
	return smoothPath, detour.DT_FAILURE
}
