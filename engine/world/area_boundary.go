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
