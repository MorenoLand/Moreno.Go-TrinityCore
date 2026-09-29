package world

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetDestTargetEnemy  uint32 = 53 // TARGET_DEST_TARGET_ENEMY (SharedDefines.h:1489)
	implicitTargetDestTargetAny    uint32 = 63 // TARGET_DEST_TARGET_ANY (SharedDefines.h:1499)
	implicitTargetDestTargetFront  uint32 = 64 // TARGET_DEST_TARGET_FRONT (SharedDefines.h:1500)
	implicitTargetDestTargetBack   uint32 = 65
	implicitTargetDestTargetRight  uint32 = 66
	implicitTargetDestTargetLeft   uint32 = 67
	implicitTargetDestTargetFRight uint32 = 68
	implicitTargetDestTargetBRight uint32 = 69
	implicitTargetDestTargetBLeft  uint32 = 70
	implicitTargetDestTargetFLeft  uint32 = 71
	implicitTargetDestTargetRandom uint32 = 74 // TARGET_DEST_TARGET_RANDOM (SharedDefines.h:1510)
	implicitTargetDestTargetRadius uint32 = 75 // TARGET_DEST_TARGET_RADIUS (SharedDefines.h:1511)

	implicitTargetDestDynobjEnemy uint32 = 28 // TARGET_DEST_DYNOBJ_ENEMY (SharedDefines.h:1464)
	implicitTargetDestDynobjAlly  uint32 = 29 // TARGET_DEST_DYNOBJ_ALLY (SharedDefines.h:1465)
	implicitTargetDestDestFront   uint32 = 78 // TARGET_DEST_DEST_FRONT (SharedDefines.h:1514)
	implicitTargetDestDestBack    uint32 = 79
	implicitTargetDestDestRight   uint32 = 80
	implicitTargetDestDestLeft    uint32 = 81
	implicitTargetDestDestFRight  uint32 = 82
	implicitTargetDestDestBRight  uint32 = 83
	implicitTargetDestDestBLeft   uint32 = 84
	implicitTargetDestDestFLeft   uint32 = 85
	implicitTargetDestDestRandom  uint32 = 86 // TARGET_DEST_DEST_RANDOM (SharedDefines.h:1520)
	implicitTargetDestDest        uint32 = 87 // TARGET_DEST_DEST (SharedDefines.h:1521)
	implicitTargetDestDynobjNone  uint32 = 88 // TARGET_DEST_DYNOBJ_NONE (SharedDefines.h:1522)
	implicitTargetDestDestRadius  uint32 = 91 // TARGET_DEST_DEST_RADIUS (SharedDefines.h:1524)
)

// isTargetDestTarget reports TARGET_REFERENCE_TYPE_TARGET dest values:
// SelectImplicitTargetDestTargets (Spell.cpp:1433).
func isTargetDestTarget(target uint32) bool {
	switch target {
	case implicitTargetDestTargetEnemy, implicitTargetDestTargetAny,
		implicitTargetDestTargetFront, implicitTargetDestTargetBack,
		implicitTargetDestTargetRight, implicitTargetDestTargetLeft,
		implicitTargetDestTargetFRight, implicitTargetDestTargetBRight,
		implicitTargetDestTargetBLeft, implicitTargetDestTargetFLeft,
		implicitTargetDestTargetRandom, implicitTargetDestTargetRadius:
		return true
	}
	return false
}

// isDestDestTarget reports TARGET_REFERENCE_TYPE_DEST dest values:
// SelectImplicitDestDestTargets (Spell.cpp:1464).
func isDestDestTarget(target uint32) bool {
	switch target {
	case implicitTargetDestDynobjEnemy, implicitTargetDestDynobjAlly,
		implicitTargetDestDestFront, implicitTargetDestDestBack,
		implicitTargetDestDestRight, implicitTargetDestDestLeft,
		implicitTargetDestDestFRight, implicitTargetDestDestBRight,
		implicitTargetDestDestBLeft, implicitTargetDestDestFLeft,
		implicitTargetDestDestRandom, implicitTargetDestDest,
		implicitTargetDestDynobjNone, implicitTargetDestDestRadius:
		return true
	}
	return false
}

// destDirectionAngle mirrors SpellImplicitTargetInfo::CalcDirectionAngle
// (SpellInfo.cpp:102-124) for the target-dest and dest-dest families. The
// directional values share the same static DirectionType rows as the
// caster-dest family, so they delegate to casterDestDirectionAngle; the
// random/radius values carry TARGET_DIR_RANDOM.
func destDirectionAngle(target uint32) float32 {
	switch target {
	case implicitTargetDestTargetFront, implicitTargetDestDestFront:
		return casterDestDirectionAngle(implicitTargetDestCasterFront)
	case implicitTargetDestTargetBack, implicitTargetDestDestBack:
		return casterDestDirectionAngle(implicitTargetDestCasterBack)
	case implicitTargetDestTargetRight, implicitTargetDestDestRight:
		return casterDestDirectionAngle(implicitTargetDestCasterRight)
	case implicitTargetDestTargetLeft, implicitTargetDestDestLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterLeft)
	case implicitTargetDestTargetFRight, implicitTargetDestDestFRight:
		return casterDestDirectionAngle(implicitTargetDestCasterFrontRight)
	case implicitTargetDestTargetBRight, implicitTargetDestDestBRight:
		return casterDestDirectionAngle(implicitTargetDestCasterBackRight)
	case implicitTargetDestTargetBLeft, implicitTargetDestDestBLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterBackLeft)
	case implicitTargetDestTargetFLeft, implicitTargetDestDestFLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterFrontLeft)
	case implicitTargetDestTargetRandom, implicitTargetDestTargetRadius,
		implicitTargetDestDestRandom, implicitTargetDestDestRadius:
		return rand.Float32() * 2 * float32(math.Pi)
	default:
		return 0
	}
}

// resolveTargetDestPosition mirrors Spell::SelectImplicitTargetDestTargets
// (Spell.cpp:1433-1462): 53/63 keep the target position; otherwise the
// target position moved by CalcRadius(nullptr) along CalcDirectionAngle (74
// scales the distance by rand_norm, 75 uses a random angle). There is no
// combat-reach floor on this path, and the angle is relative to the target's
// orientation (target->MovePositionToFirstCollision). Go has no collision or
// map height queries, so the move degrades to a plain offset and z is kept.
func resolveTargetDestPosition(store *wotlk.Store, target uint32, radiusIndex uint32, tx, ty, tz, tOrientation float32) (float32, float32, float32) {
	if target == implicitTargetDestTargetEnemy || target == implicitTargetDestTargetAny {
		return tx, ty, tz
	}
	radius := float32(0)
	if store != nil {
		// CalcRadius(nullptr): base radius only, no per-level or max terms (SpellInfo.cpp:551-574).
		if value, ok, err := store.SpellRadius(radiusIndex, 0); err == nil && ok {
			radius = value
		}
	}
	dist := radius
	if target == implicitTargetDestTargetRandom {
		dist *= rand.Float32()
	}
	angle := destDirectionAngle(target) + tOrientation
	return tx + dist*float32(math.Cos(float64(angle))), ty + dist*float32(math.Sin(float64(angle))), tz
}

// resolveDestDestPosition mirrors Spell::SelectImplicitDestDestTargets
// (Spell.cpp:1464-1499): 28/29/87/88 leave the destination unchanged (the
// C++ early return); otherwise the current destination moved by
// CalcRadius(m_caster) along CalcDirectionAngle (86 scales the distance by
// rand_norm, 91 uses a random angle). The angle is relative to the caster's
// orientation (m_caster->MovePositionToFirstCollision), the start position
// is the current destination, and there is no combat-reach floor. Collision
// degrades to a plain offset, z is kept. changed reports whether the
// destination was modified.
func resolveDestDestPosition(store *wotlk.Store, target uint32, radiusIndex uint32, level uint32, dx, dy, dz, cOrientation float32) (float32, float32, float32, bool) {
	switch target {
	case implicitTargetDestDynobjEnemy, implicitTargetDestDynobjAlly,
		implicitTargetDestDest, implicitTargetDestDynobjNone:
		return dx, dy, dz, false
	}
	radius := float32(0)
	if store != nil {
		if value, ok, err := store.SpellRadius(radiusIndex, level); err == nil && ok {
			radius = value
		}
	}
	dist := radius
	if target == implicitTargetDestDestRandom {
		dist *= rand.Float32()
	}
	angle := destDirectionAngle(target) + cOrientation
	return dx + dist*float32(math.Cos(float64(angle))), dy + dist*float32(math.Sin(float64(angle))), dz, true
}

// spellHasDestFamilyTarget reports whether any active effect carries a
// target-dest or dest-dest implicit target.
func spellHasDestFamilyTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isTargetDestTarget(eff.ImplicitTargetA) || isTargetDestTarget(eff.ImplicitTargetB) ||
			isDestDestTarget(eff.ImplicitTargetA) || isDestDestTarget(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// resolveImplicitSpellDestination mirrors the destination half of
// Spell::SelectSpellTargets (Spell.cpp:758-794) for the target-dest and
// dest-dest families, the 89 traj destination, the 76/106 channel
// destinations, and TARGET_DEST_NEARBY_ENTRY (46, the DEST half of
// Spell::SelectImplicitNearbyTargets, Spell.cpp:1036): it starts from the
// client-supplied destination (falling back to the caster position, like
// CheckDst in Spell.cpp:6523) and applies the per-effect resolutions in
// effect order — target-dest replaces the destination with the
// unit-target-derived position (Spell.cpp:1433), dest-dest offsets the
// current destination (Spell.cpp:1464), 46 sets it to the nearest
// entry-matched object's position. The result is written back into target
// with the dest-location flag so the area-selection and persistent-area read
// sites consume it through their existing client-dest branches. It runs once
// at cast time; the dynamic-aura tick path reuses those read sites with an
// already resolved destination and must not re-apply the offsets. ok is
// false when a 46 effect finds no object, which fails the cast with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
func (s *session) resolveImplicitSpellDestination(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) (protocol.SpellTargetData, bool) {
	if s == nil || s.player == nil || s.server == nil || (!spellHasDestFamilyTarget(spell) && !spellHasTrajTarget(spell) && !spellHasChannelDestTarget(spell) && !spellHasDestNearbyEntryTarget(spell)) {
		return target, true
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	if target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	}
	var unitTarget combatTarget
	hasUnitTarget := false
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		if resolved, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			unitTarget, hasUnitTarget = resolved, true
		}
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch {
			case isTargetDestTarget(targetType):
				// C++ asserts a non-null object target here; without a
				// resolvable unit target Go keeps the current destination.
				if !hasUnitTarget {
					continue
				}
				x, y, z = resolveTargetDestPosition(s.server.Data, targetType, eff.RadiusIndex, unitTarget.X, unitTarget.Y, unitTarget.Z, unitTarget.Orientation)
			case isDestDestTarget(targetType):
				nx, ny, nz, changed := resolveDestDestPosition(s.server.Data, targetType, eff.RadiusIndex, uint32(s.player.Level), x, y, z, s.player.Orientation)
				if changed {
					x, y, z = nx, ny, nz
				}
			case targetType == implicitTargetDestTraj:
				// Spell::SelectImplicitTrajTargets (Spell.cpp:1626):
				// shorten the destination to the first missile-body
				// collision along the ballistic trajectory. CheckDst is
				// the caster-position fallback already applied above.
				if nx, ny, nz, changed := s.resolveTrajDestination(ctx, spell, eff, target, x, y, z); changed {
					x, y, z = nx, ny, nz
				}
			case targetType == implicitTargetDestChannelTarget || targetType == implicitTargetDestChannelCaster:
				// Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032):
				// resolve the destination from the live channel. The
				// client-cast path breaks any prior channel at cast start
				// (mirroring C++ SetCurrentCastSpell in Spell::prepare, which
				// runs before SelectSpellTargets in _cast), so the C++ null
				// gate means nothing resolves here in practice.
				if nx, ny, nz, ok := s.channelDestForSpell(ctx, spell); ok {
					x, y, z = nx, ny, nz
				}
			case targetType == implicitTargetDestNearbyEntry:
				// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
				// TARGET_DEST_NEARBY_ENTRY (46) — the destination becomes the
				// nearest entry-matched object's position; no match fails the
				// cast (Spell.cpp:1111), and without conditions a
				// RequiresSpellFocus spell uses the focus object's position.
				if nx, ny, nz, ok := s.spellEntryNearbyDestPosition(ctx, spell, spellID); ok {
					x, y, z = nx, ny, nz
				} else {
					return target, false
				}
			}
		}
	}
	target.Flags |= protocol.SpellTargetFlagDestLocation
	target.Destination = protocol.SpellTargetLocation{X: x, Y: y, Z: z}
	return target, true
}
