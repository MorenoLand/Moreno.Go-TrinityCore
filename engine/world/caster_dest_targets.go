package world

import (
	"math"
	"math/rand/v2"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	implicitTargetDestCaster           uint32 = 18 // TARGET_DEST_CASTER (SharedDefines.h:1463)
	implicitTargetDestCasterSummon     uint32 = 32 // TARGET_DEST_CASTER_SUMMON (SharedDefines.h:1477)
	implicitTargetDestCasterFrontRight uint32 = 41 // TARGET_DEST_CASTER_FRONT_RIGHT (SharedDefines.h:1486)
	implicitTargetDestCasterBackRight  uint32 = 42 // TARGET_DEST_CASTER_BACK_RIGHT
	implicitTargetDestCasterBackLeft   uint32 = 43 // TARGET_DEST_CASTER_BACK_LEFT
	implicitTargetDestCasterFrontLeft  uint32 = 44 // TARGET_DEST_CASTER_FRONT_LEFT
	implicitTargetDestCasterFront      uint32 = 47 // TARGET_DEST_CASTER_FRONT (SharedDefines.h:1492)
	implicitTargetDestCasterBack       uint32 = 48 // TARGET_DEST_CASTER_BACK
	implicitTargetDestCasterRight      uint32 = 49 // TARGET_DEST_CASTER_RIGHT
	implicitTargetDestCasterLeft       uint32 = 50 // TARGET_DEST_CASTER_LEFT
	implicitTargetDestCasterFrontLeap  uint32 = 55 // TARGET_DEST_CASTER_FRONT_LEAP (SharedDefines.h:1500)
	implicitTargetDestCasterRandom     uint32 = 72 // TARGET_DEST_CASTER_RANDOM (SharedDefines.h:1517)
	implicitTargetDestCasterRadius     uint32 = 73 // TARGET_DEST_CASTER_RADIUS

	defaultTotemDistance = 3.0 // DefaultTotemDistance (Spell.cpp:1404)
	petFollowDistanceDBC = 1.0 // PET_FOLLOW_DIST (PetDefines.h:84)
)

func isCasterDestTarget(target uint32) bool {
	switch target {
	case implicitTargetDestCaster, implicitTargetDestCasterSummon,
		implicitTargetDestCasterFrontRight, implicitTargetDestCasterBackRight,
		implicitTargetDestCasterBackLeft, implicitTargetDestCasterFrontLeft,
		implicitTargetDestCasterFront, implicitTargetDestCasterBack,
		implicitTargetDestCasterRight, implicitTargetDestCasterLeft,
		implicitTargetDestCasterFrontLeap, implicitTargetDestCasterRandom,
		implicitTargetDestCasterRadius:
		return true
	}
	return false
}

// casterDestDirectionAngle mirrors SpellImplicitTargetInfo::CalcDirectionAngle (SpellInfo.cpp:102-124).
func casterDestDirectionAngle(target uint32) float32 {
	switch target {
	case implicitTargetDestCasterFront:
		return 0
	case implicitTargetDestCasterBack:
		return float32(math.Pi)
	case implicitTargetDestCasterRight:
		return float32(-math.Pi / 2)
	case implicitTargetDestCasterLeft:
		return float32(math.Pi / 2)
	case implicitTargetDestCasterFrontRight:
		return float32(-math.Pi / 4)
	case implicitTargetDestCasterBackRight:
		return float32(-3 * math.Pi / 4)
	case implicitTargetDestCasterBackLeft:
		return float32(3 * math.Pi / 4)
	case implicitTargetDestCasterSummon, implicitTargetDestCasterFrontLeft:
		return float32(math.Pi / 4)
	default:
		return 0
	}
}

// resolveCasterDestPosition mirrors the default branch of
// Spell::SelectImplicitCasterDestTargets (Spell.cpp:1393-1424): the spell
// destination is the caster's position moved by dist along the direction
// angle (angle is relative, added to orientation like
// WorldObject::MovePosition, Object.cpp:3340). Go has no collision or map
// height queries, so MovePositionToFirstCollision degrades to a plain
// offset and z is kept.
func resolveCasterDestPosition(store *wotlk.Store, target uint32, radiusIndex uint32, level uint32, x, y, z, orientation, combatReach float32) (float32, float32, float32) {
	if target == implicitTargetDestCaster {
		return x, y, z
	}
	radius := float32(0)
	if store != nil {
		if value, ok, err := store.SpellRadius(radiusIndex, level); err == nil && ok {
			radius = value
		}
	}
	angle := casterDestDirectionAngle(target)
	dist := radius
	switch target {
	case implicitTargetDestCasterFrontLeap:
		angle = 0
	case implicitTargetDestCasterSummon:
		dist = petFollowDistanceDBC
	case implicitTargetDestCasterRandom:
		if dist > combatReach {
			dist = combatReach + (dist-combatReach)*rand.Float32()
		}
		angle = rand.Float32() * 2 * float32(math.Pi)
	case implicitTargetDestCasterRadius:
		angle = rand.Float32() * 2 * float32(math.Pi)
	case implicitTargetDestCasterFrontRight, implicitTargetDestCasterBackRight,
		implicitTargetDestCasterBackLeft, implicitTargetDestCasterFrontLeft:
		if radiusIndex == 0 {
			dist = defaultTotemDistance
		}
	}
	if target != implicitTargetDestCasterFrontLeap && dist < combatReach {
		dist = combatReach
	}
	angle += orientation
	return x + dist*float32(math.Cos(float64(angle))), y + dist*float32(math.Sin(float64(angle))), z
}

// totemSpellDestTarget finds the caster-dest implicit target driving a
// totem summon's destination: the summon effect's target first, like
// Spell::SelectEffectImplicitTargets resolves per effect (Spell.cpp:867).
func totemSpellDestTarget(spell wotlk.Spell) (uint32, uint32, bool) {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if isCasterDestTarget(targetType) && eff.Effect == 87 {
				return targetType, eff.RadiusIndex, true
			}
		}
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if isCasterDestTarget(targetType) {
				return targetType, eff.RadiusIndex, true
			}
		}
	}
	return 0, 0, false
}
