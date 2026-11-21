package world

import (
	"context"
	"time"
)

// areaBoundary mirrors AreaBoundary (Maps/AreaBoundary.h): a shape test
// over a creature position. IsWithinBoundary applies the inversion on top
// of the shape, exactly like AreaBoundary::IsWithinBoundary.
type areaBoundary interface {
	within(x, y, z float32) bool
}

// circleBoundary mirrors CircleBoundary (AreaBoundary.cpp:37-47): 2D
// distance from the center against the squared radius.
type circleBoundary struct {
	centerX, centerY float32
	radius           float64
	inverted         bool
}

func (b circleBoundary) within(x, y, z float32) bool {
	offX := float64(b.centerX - x)
	offY := float64(b.centerY - y)
	inside := offX*offX+offY*offY <= b.radius*b.radius
	return inside != b.inverted
}

// rectangleBoundary mirrors RectangleBoundary (AreaBoundary.cpp:23-33):
// southX/northX bound X, eastY/westY bound Y, inclusive on both ends.
type rectangleBoundary struct {
	minX, maxX, minY, maxY float32
	inverted               bool
}

func (b rectangleBoundary) within(x, y, z float32) bool {
	inside := !(x < b.minX || x > b.maxX || y < b.minY || y > b.maxY)
	return inside != b.inverted
}

// zRangeBoundary mirrors ZRangeBoundary (AreaBoundary.cpp:92-98):
// inclusive Z band.
type zRangeBoundary struct {
	minZ, maxZ float32
	inverted   bool
}

func (b zRangeBoundary) within(x, y, z float32) bool {
	inside := b.minZ <= z && z <= b.maxZ
	return inside != b.inverted
}

// unionBoundary mirrors BoundaryUnionBoundary (AreaBoundary.cpp:101-116):
// inside when either half contains the point.
type unionBoundary struct {
	first, second areaBoundary
	inverted      bool
}

func (b unionBoundary) within(x, y, z float32) bool {
	inside := (b.first != nil && b.first.within(x, y, z)) || (b.second != nil && b.second.within(x, y, z))
	return inside != b.inverted
}

// ellipseBoundary mirrors EllipseBoundary (AreaBoundary.cpp:57-66):
// (offX^2)*(rY^2/rX^2) + offY^2 <= rY^2, the C++ form verbatim.
type ellipseBoundary struct {
	centerX, centerY float32
	radiusX, radiusY float64
	scaleXSq         float64
	inverted         bool
}

func newEllipseBoundary(centerX, centerY float32, radiusX, radiusY float64) ellipseBoundary {
	return ellipseBoundary{centerX: centerX, centerY: centerY, radiusX: radiusX, radiusY: radiusY, scaleXSq: radiusY * radiusY / (radiusX * radiusX)}
}

func (b ellipseBoundary) within(x, y, z float32) bool {
	offX := float64(b.centerX - x)
	offY := float64(b.centerY - y)
	inside := (offX*offX)*b.scaleXSq+(offY*offY) <= b.radiusY*b.radiusY
	return inside != b.inverted
}

// parallelogramBoundary mirrors ParallelogramBoundary
// (AreaBoundary.cpp:92-113): corners A, B, D with C = D + (B - A); the
// point is inside when all four edge half-plane signs agree.
type parallelogramBoundary struct {
	aX, aY, bX, bY, cX, cY, dX, dY float32
	abx, aby, dax, day             float64
	inverted                       bool
}

func newParallelogramBoundary(aX, aY, bX, bY, dX, dY float32) parallelogramBoundary {
	cX := dX + (bX - aX)
	cY := dY + (bY - aY)
	return parallelogramBoundary{
		aX: aX, aY: aY, bX: bX, bY: bY, cX: cX, cY: cY, dX: dX, dY: dY,
		abx: float64(bX - aX), aby: float64(bY - aY),
		dax: float64(aX - dX), day: float64(aY - dY),
	}
}

func (b parallelogramBoundary) within(x, y, z float32) bool {
	sign1 := (float64(x)-float64(b.bX))*b.aby-(float64(y)-float64(b.bY))*b.abx < 0
	sign2 := (float64(x)-float64(b.aX))*b.day-(float64(y)-float64(b.aY))*b.dax < 0
	sign3 := (float64(y)-float64(b.dY))*b.abx-(float64(x)-float64(b.dX))*b.aby < 0
	sign4 := (float64(y)-float64(b.cY))*b.dax-(float64(x)-float64(b.cX))*b.day < 0
	inside := sign1 == sign2 && sign2 == sign3 && sign3 == sign4
	return inside != b.inverted
}

type bossBoundaryKey struct {
	mapID uint32
	entry uint32
}

// instanceBossBoundaries mirrors the per-instance LoadBossBoundaries data
// (InstanceScript::GetBossBoundary, InstanceScript.h:239): the boss room
// polygons, keyed by map + boss entry. Only rows verified against the C++
// instance script are listed; other instances' rows stay unported, so
// their bosses carry no boundary until added here.
var instanceBossBoundaries = map[bossBoundaryKey][]areaBoundary{
	// Magtheridon (map 544, entry 17257): instance_magtheridons_lair.cpp:26-29,
	// BossBoundaryData circle (-18.70, 2.24) r52.30 for DATA_MAGTHERIDON.
	{mapID: 544, entry: 17257}: {circleBoundary{centerX: -18.70, centerY: 2.24, radius: 52.30}},
	// Sunwell Plateau (map 580): instance_sunwell_plateau.cpp:64-67,
	// DATA_KALECGOS union of circle (1704.9, 928.4) r34.0 + rect.
	{mapID: 580, entry: 24850}: {unionBoundary{first: circleBoundary{centerX: 1704.9, centerY: 928.4, radius: 34.0}, second: rectangleBoundary{minX: 1689.2, maxX: 1713.3, minY: 762.2, maxY: 1074.8}}},
	// Onyxia's Lair (map 249): instance_onyxias_lair.cpp:33-36, DATA_ONYXIA.
	{mapID: 249, entry: 10184}: {circleBoundary{centerX: -34.3697, centerY: -212.3296, radius: 100.0}},
	// Ahn'kahet (map 619): instance_ahnkahet.cpp:50-53, DATA_JEDOGA_SHADOWSEEKER.
	{mapID: 619, entry: 29310}: {newParallelogramBoundary(460.365, -661.997, 364.958, -790.211, 347.436, -657.978)},
	// Azjol-Nerub (map 601): instance_azjol_nerub.cpp:52-57.
	{mapID: 601, entry: 28684}: {rectangleBoundary{minX: 400.0, maxX: 580.0, minY: 623.5, maxY: 810.0}},
	{mapID: 601, entry: 28921}: {zRangeBoundary{minZ: 666.0, maxZ: 776.0}},
	{mapID: 601, entry: 29120}: {circleBoundary{centerX: 550.6178, centerY: 253.5917, radius: 26.0}},
	// Obsidian Sanctum (map 615): instance_obsidian_sanctum.cpp:28-31, DATA_SARTHARION.
	{mapID: 615, entry: 28860}: {rectangleBoundary{minX: 3218.86, maxX: 3275.69, minY: 484.68, maxY: 572.4}},
	// Ruby Sanctum (map 669): instance_ruby_sanctum.cpp:30-34.
	{mapID: 669, entry: 39746}: {newEllipseBoundary(3013.409, 529.492, 45.0, 100.0)},
	{mapID: 669, entry: 39863}: {circleBoundary{centerX: 3156.037, centerY: 533.2656, radius: 48.5}},
	// Trial of the Crusader (map 649): instance_trial_of_the_crusader.cpp:32-39.
	// DATA_NORTHREND_BEASTS / JARAXXUS / FACTION_CRUSADERS / TWIN_VALKIRIES
	// share the arena circle; the champions controller row is NOT seeded
	// (its BossAI bossId is unverified — the champions' own AI carries it).
	{mapID: 649, entry: 34796}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 35144}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34799}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34797}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34780}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34497}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34496}: {circleBoundary{centerX: 563.26, centerY: 139.6, radius: 75.0}},
	{mapID: 649, entry: 34564}: {newEllipseBoundary(746.0, 135.0, 100.0, 75.0)},
	// Forge of Souls (map 632): instance_forge_of_souls.cpp:26-30.
	{mapID: 632, entry: 36497}: {circleBoundary{centerX: 5297.3, centerY: 2506.45, radius: 100.96}},
	{mapID: 632, entry: 36502}: {newParallelogramBoundary(5663.56, 2570.53, 5724.39, 2520.45, 5570.36, 2461.42)},
	// Icecrown Citadel (map 631): instance_icecrown_citadel.cpp:60-76.
	// LoadBossBoundaries pushes every row (InstanceScript.cpp:136-141), so
	// duplicate DATA ids accumulate and IsInBounds ANDs them all.
	{mapID: 631, entry: 36612}: {circleBoundary{centerX: -428.0, centerY: 2211.0, radius: 95.0}, rectangleBoundary{minX: -430.0, maxX: -330.0, minY: 2110.0, maxY: 2310.0}},
	{mapID: 631, entry: 36855}: {rectangleBoundary{minX: -670.0, maxX: -520.0, minY: 2145.0, maxY: 2280.0}},
	{mapID: 631, entry: 37813}: {rectangleBoundary{minX: -565.0, maxX: -465.0, minY: 2160.0, maxY: 2260.0}},
	{mapID: 631, entry: 36627}: {rectangleBoundary{minX: 4385.0, maxX: 4505.0, minY: 3082.0, maxY: 3195.0}},
	{mapID: 631, entry: 36626}: {rectangleBoundary{minX: 4205.0, maxX: 4325.0, minY: 3082.0, maxY: 3195.0}},
	{mapID: 631, entry: 36678}: {newParallelogramBoundary(4356.0, 3290.0, 4435.0, 3194.0, 4280.0, 3194.0), rectangleBoundary{minX: 4280.0, maxX: 4435.0, minY: 3150.0, maxY: 4360.0}},
	{mapID: 631, entry: 37970}: {newEllipseBoundary(4660.95, 2769.194, 85.0, 60.0)},
	{mapID: 631, entry: 37972}: {newEllipseBoundary(4660.95, 2769.194, 85.0, 60.0)},
	{mapID: 631, entry: 37973}: {newEllipseBoundary(4660.95, 2769.194, 85.0, 60.0)},
	{mapID: 631, entry: 37955}: {circleBoundary{centerX: 4595.93, centerY: 2769.365, radius: 64.0}, zRangeBoundary{minZ: 391.78, maxZ: 473.43}},
	{mapID: 631, entry: 37126}: {rectangleBoundary{minX: 4291.0, maxX: 4423.0, minY: 2438.0, maxY: 2653.0}},
	{mapID: 631, entry: 36789}: {rectangleBoundary{minX: 4112.5, maxX: 4293.5, minY: 2385.0, maxY: 2585.0}},
	{mapID: 631, entry: 36853}: {newEllipseBoundary(4408.6, 2484.0, 100.0, 75.0)},
	// Naxxramas (map 533): instance_naxxramas.cpp:27-58. Circle rows built
	// from a point on the circle use the exact 2D distance as radius
	// (CircleBoundary(center, pointOnCircle), AreaBoundary.cpp:46-48).
	{mapID: 533, entry: 15956}: {circleBoundary{centerX: 3273.376709, centerY: -3475.876709, radius: 77.70851439386894}},
	{mapID: 533, entry: 15953}: {rectangleBoundary{minX: 3315.0, maxX: 3402.0, minY: -3727.0, maxY: -3590.0}, circleBoundary{centerX: 3372.68, centerY: -3648.2, radius: 79.72042398281651}},
	{mapID: 533, entry: 15952}: {circleBoundary{centerX: 3502.2587, centerY: -3892.1697, radius: 98.32843349682697}},
	{mapID: 533, entry: 15954}: {rectangleBoundary{minX: 2618.0, maxX: 2754.0, minY: -3557.43, maxY: -3450.0}},
	{mapID: 533, entry: 15936}: {circleBoundary{centerX: 2772.57, centerY: -3685.28, radius: 56.0}},
	{mapID: 533, entry: 16011}: {circleBoundary{centerX: 2909.0, centerY: -3997.41, radius: 57.0}},
	{mapID: 533, entry: 16061}: {zRangeBoundary{minZ: 260.0, maxZ: 287.0}},
	{mapID: 533, entry: 16060}: {rectangleBoundary{minX: 2627.0, maxX: 2764.0, minY: -3440.0, maxY: -3275.0}},
	{mapID: 533, entry: 16064}: {newParallelogramBoundary(2646.0, -2959.0, 2529.0, -3075.0, 2506.0, -2854.0)},
	{mapID: 533, entry: 16065}: {newParallelogramBoundary(2646.0, -2959.0, 2529.0, -3075.0, 2506.0, -2854.0)},
	{mapID: 533, entry: 16063}: {newParallelogramBoundary(2646.0, -2959.0, 2529.0, -3075.0, 2506.0, -2854.0)},
	{mapID: 533, entry: 30549}: {newParallelogramBoundary(2646.0, -2959.0, 2529.0, -3075.0, 2506.0, -2854.0)},
	{mapID: 533, entry: 16028}: {circleBoundary{centerX: 3204.0, centerY: -3241.4, radius: 240.0}, circleBoundary{centerX: 3130.8576, centerY: -3210.36, radius: 46.46699746874108, inverted: true}},
	{mapID: 533, entry: 15931}: {circleBoundary{centerX: 3204.0, centerY: -3241.4, radius: 240.0}, rectangleBoundary{minX: 3295.0, maxX: 3340.0, minY: -3254.2, maxY: -3230.18, inverted: true}},
	{mapID: 533, entry: 15932}: {circleBoundary{centerX: 3293.0, centerY: -3142.0, radius: 80.0}, newParallelogramBoundary(3401.0, -3149.0, 3261.0, -3028.0, 3320.0, -3267.0), zRangeBoundary{minZ: 285.0, maxZ: 310.0}},
	{mapID: 533, entry: 15928}: {newParallelogramBoundary(3478.3, -3070.0, 3370.0, -2961.5, 3580.0, -2961.5)},
	{mapID: 533, entry: 15989}: {circleBoundary{centerX: 3517.627, centerY: -5255.5, radius: 110.0}},
	{mapID: 533, entry: 15990}: {circleBoundary{centerX: 3716.0, centerY: -5107.0, radius: 85.0}},
	// Eye of Eternity (map 616): instance_eye_of_eternity.cpp:28-31, DATA_MALYGOS_EVENT.
	{mapID: 616, entry: 28859}: {circleBoundary{centerX: 754.362, centerY: 1301.609985, radius: 280.0}},
	// Utgarde Pinnacle (map 575): instance_utgarde_pinnacle.cpp:24-27, DATA_KING_YMIRON.
	{mapID: 575, entry: 26861}: {rectangleBoundary{minX: 340.0, maxX: 443.0, minY: -412.0, maxY: -275.0}},
	// Black Temple (map 564): instance_black_temple.cpp:44-57.
	{mapID: 564, entry: 22887}: {rectangleBoundary{minX: 394.0, maxX: 479.4, minY: 707.8, maxY: 859.1}},
	{mapID: 564, entry: 22898}: {rectangleBoundary{minX: 556.1, maxX: 850.2, minY: 542.0, maxY: 1001.0}},
	{mapID: 564, entry: 22841}: {rectangleBoundary{minX: 406.8, maxX: 564.0, minY: 327.9, maxY: 473.5}},
	{mapID: 564, entry: 22871}: {rectangleBoundary{minX: 512.5, maxX: 613.3, minY: 373.2, maxY: 432.0}, zRangeBoundary{minZ: 179.5, maxZ: 223.6}},
	{mapID: 564, entry: 22948}: {rectangleBoundary{minX: 720.5, maxX: 864.5, minY: 159.3, maxY: 316.0}},
	{mapID: 564, entry: 22856}: {rectangleBoundary{minX: 435.9, maxX: 660.3, minY: 21.2, maxY: 229.6}, zRangeBoundary{minZ: 81.8, maxZ: 148.0}},
	{mapID: 564, entry: 22947}: {rectangleBoundary{minX: 903.4, maxX: 982.1, minY: 92.4, maxY: 313.2}},
	{mapID: 564, entry: 22949}: {newEllipseBoundary(696.6, 305.0, 70.0, 85.0)},
	{mapID: 564, entry: 22950}: {newEllipseBoundary(696.6, 305.0, 70.0, 85.0)},
	{mapID: 564, entry: 22951}: {newEllipseBoundary(696.6, 305.0, 70.0, 85.0)},
	{mapID: 564, entry: 22952}: {newEllipseBoundary(696.6, 305.0, 70.0, 85.0)},
	{mapID: 564, entry: 22917}: {newEllipseBoundary(694.8, 309.0, 80.0, 95.0)},
	// Ulduar (map 603): instance_ulduar.cpp:33-48. Thorim sets its
	// boundary dynamically, so no static row exists for it.
	{mapID: 603, entry: 33113}: {rectangleBoundary{minX: 148.0, maxX: 401.3, minY: -155.0, maxY: 90.0}},
	{mapID: 603, entry: 33118}: {rectangleBoundary{minX: 495.0, maxX: 680.0, minY: 90.0, maxY: 400.0}},
	{mapID: 603, entry: 33186}: {rectangleBoundary{minX: 370.0, maxX: 810.0, minY: -542.0, maxY: -55.0}},
	{mapID: 603, entry: 33293}: {rectangleBoundary{minX: 755.0, maxX: 940.0, minY: -125.0, maxY: 95.0}},
	{mapID: 603, entry: 32867}: {circleBoundary{centerX: 1587.2, centerY: 121.0, radius: 90.0}},
	{mapID: 603, entry: 32857}: {circleBoundary{centerX: 1587.2, centerY: 121.0, radius: 90.0}},
	{mapID: 603, entry: 32927}: {circleBoundary{centerX: 1587.2, centerY: 121.0, radius: 90.0}},
	{mapID: 603, entry: 32871}: {circleBoundary{centerX: 1632.668, centerY: -307.7656, radius: 45.0}, zRangeBoundary{minZ: 410.0, maxZ: 470.0}},
	{mapID: 603, entry: 32845}: {newEllipseBoundary(2001.5, -240.0, 50.0, 75.0)},
	{mapID: 603, entry: 32906}: {rectangleBoundary{minX: 2094.6, maxX: 2520.0, minY: -250.0, maxY: 200.0}},
	{mapID: 603, entry: 33350}: {circleBoundary{centerX: 2744.0, centerY: 2569.0, radius: 70.0}},
	{mapID: 603, entry: 33271}: {rectangleBoundary{minX: 1740.0, maxX: 1930.0, minY: 31.0, maxY: 228.0}},
	{mapID: 603, entry: 33288}: {circleBoundary{centerX: 1980.42, centerY: -27.68, radius: 105.0}},
}

// boundaryCheckInterval is Creature::m_boundaryCheckTime's reload
// (Creature.cpp:830-838): CheckInRoom runs every 2.5s while engaged.
const boundaryCheckInterval = 2500 * time.Millisecond

// applyBossBoundary mirrors the BossAI ctor SetBoundary arm
// (ScriptedCreature.cpp:486-488: SetBoundary(instance->GetBossBoundary(bossId))):
// on first engage, a boss with a registered instance boundary takes it and
// arms the 2.5s check. A creature with no registered row keeps none, so the
// tick below stays inert — the C++ empty-boundary case (IsInBounds over an
// empty CreatureBoundary returns true, CreatureAI.cpp:411-417).
func applyBossBoundary(motion *creatureMotion) {
	if motion == nil || len(motion.Boundary) > 0 {
		return
	}
	if boundary, ok := instanceBossBoundaries[bossBoundaryKey{mapID: motion.Map, entry: motion.Entry}]; ok {
		motion.Boundary = boundary
		motion.BoundaryCheckTime = boundaryCheckInterval
	}
}

// creatureWithinBoundary mirrors CreatureAI::IsInBounds
// (CreatureAI.cpp:411-417): every area of the boundary must contain the
// creature; an empty boundary contains everything.
func creatureWithinBoundary(motion *creatureMotion) bool {
	if motion == nil {
		return true
	}
	for _, area := range motion.Boundary {
		if area == nil || !area.within(motion.X, motion.Y, motion.Z) {
			return false
		}
	}
	return true
}

// stepCreatureBoundaryCheck mirrors the Creature::Update boundary arm
// (Creature.cpp:830-838): every 2.5s while engaged, AI()->CheckInRoom()
// (CreatureAI.cpp:419-427) evades out-of-room. True only when the evade
// actually took — a Lua-vetoed evade leaves combat running, so the tick
// continues.
func (s *Server) stepCreatureBoundaryCheck(ctx context.Context, motion *creatureMotion, now time.Time) bool {
	if motion == nil || len(motion.Boundary) == 0 {
		return false
	}
	motion.BoundaryCheckTime -= 100 * time.Millisecond
	if motion.BoundaryCheckTime > 0 {
		return false
	}
	motion.BoundaryCheckTime = boundaryCheckInterval
	if creatureWithinBoundary(motion) {
		return false
	}
	s.triggerCreatureEvade(ctx, motion, now)
	return motion.Evading
}

// pointWithinBoundary mirrors CreatureAI::IsInBounds
// (CreatureAI.cpp:411-417) for an arbitrary position against the boss's
// boundary: every area must contain the point; an empty boundary contains
// everything (the C++ no-boundary case, IsInBoundary returning true).
func pointWithinBoundary(motion *creatureMotion, x, y, z float32) bool {
	if motion == nil {
		return true
	}
	for _, area := range motion.Boundary {
		if area == nil || !area.within(x, y, z) {
			return false
		}
	}
	return true
}

// teleportCheaters mirrors BossAI::TeleportCheaters
// (ScriptedCreature.cpp:546-557): every player-controlled unit in the
// boss's PvE combat refs standing outside the boss boundary is
// NearTeleportTo'd to the boss position (orientation 0). No boundary
// means every target is in-bounds (CreatureAI::IsInBoundary returns
// true), so the sweep is a no-op. The refs are the threat entries
// (ThreatManager::GetPvECombatRefs twin); a victim is player-controlled
// when it is a player session or a creature whose owner/charmer resolves
// to a player session (Unit::IsControlledByPlayer, Unit.h:1168).
// In-flight players skip, like the engine's own spell-teleport arm
// (spells.go) — a taxi rider cannot be a combat ref in practice.
func (s *Server) teleportCheaters(motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	applyBossBoundary(motion)
	if motion.ThreatMgr == nil || len(motion.Boundary) == 0 {
		s.motionMu.Unlock()
		return
	}
	mapID, instanceID := motion.Map, motion.InstanceID
	bossX, bossY, bossZ := motion.X, motion.Y, motion.Z
	boundary := append([]areaBoundary(nil), motion.Boundary...)
	threatMgr := motion.ThreatMgr
	s.motionMu.Unlock()
	inBounds := func(x, y, z float32) bool {
		for _, area := range boundary {
			if area == nil || !area.within(x, y, z) {
				return false
			}
		}
		return true
	}
	for _, victimGUID := range threatMgr.EntryGUIDs() {
		if victimGUID == 0 || victimGUID == motion.GUID {
			continue
		}
		if sess := s.findSessionByGUID(victimGUID); sess != nil && sess.player != nil {
			if sess.inFlight || inBounds(sess.player.X, sess.player.Y, sess.player.Z) {
				continue
			}
			sess.nearTeleportMove(bossX, bossY, bossZ, 0, true)
			continue
		}
		victim := s.findCreatureMotion(mapID, instanceID, victimGUID)
		if victim == nil {
			continue
		}
		controlled := false
		if victim.OwnerGUID != 0 && s.findSessionByGUID(victim.OwnerGUID) != nil {
			controlled = true
		} else if victim.Charmed && victim.CharmerGUID != 0 && s.findSessionByGUID(victim.CharmerGUID) != nil {
			controlled = true
		}
		if !controlled {
			continue
		}
		s.motionMu.Lock()
		motions := s.motionMapLocked(mapID, instanceID)
		m, ok := motions[victimGUID]
		if !ok || m == nil || inBounds(m.X, m.Y, m.Z) {
			s.motionMu.Unlock()
			continue
		}
		m.X, m.Y, m.Z, m.Orientation = bossX, bossY, bossZ, 0
		m.Moving = false
		phase := m.PhaseMask
		s.motionMu.Unlock()
		_, nearby := buildTeleportMovementPackets(victimGUID, movementInfo{Time: s.gameTimeMilliseconds(), X: bossX, Y: bossY, Z: bossZ, Orientation: 0})
		s.broadcastCreatureTeleportMovement(mapID, instanceID, victimGUID, bossX, bossY, bossZ, nearby, phase)
		s.syncCreatureVehiclePassengers(m)
	}
}
