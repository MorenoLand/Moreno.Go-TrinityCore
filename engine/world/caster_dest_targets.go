package world

import (
	"context"
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
	// SpellEffectInfo::HasRadius (SpellInfo.cpp:546-549) is RadiusEntry !=
	// nullptr — a nonzero index whose DBC row is missing counts as no
	// radius (Store.SpellRadius reports ok for index 0, so the index must
	// be checked as well).
	hasRadius := false
	if store != nil {
		if value, ok, err := store.SpellRadius(radiusIndex, level); err == nil && ok {
			radius = value
			hasRadius = radiusIndex != 0
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
		// Spell.cpp:1403-1405: the totem diagonals fall back to
		// DefaultTotemDistance when the effect has no radius entry.
		if !hasRadius {
			dist = defaultTotemDistance
		}
	}
	// Spell.cpp:1419-1420: the combat-reach floor applies to every target
	// in the default branch, including FRONT_LEAP.
	if dist < combatReach {
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

// spellTargetPositionDest ports the TARGET_DEST_DB (17) arm of
// Spell::SelectImplicitCasterDestTargets (Spell.cpp:1342-1357): the
// destination comes from `spell_target_position` for (spellID, effIndex).
// Spells with a TELEPORT_UNITS (5) or BIND (11) effect take the row's map
// and coordinates; other spells only when the row sits on the caster's
// map. A zero row orientation falls back to the caster's orientation,
// mirroring Spell::EffectTeleportUnits (SpellEffects.cpp:1219-1221) where
// the unit target is the caster. No row returns false — C++ falls back to
// the object target (the caster), a self-teleport no-op.
func (s *session) spellTargetPositionDest(ctx context.Context, spell wotlk.Spell, spellID uint32, effIndex uint32) (x, y, z, orientation float32, mapID uint32, ok bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, 0, 0, 0, 0, false
	}
	var rowMap int64
	var rx, ry, rz, rOri float64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT MapID, PositionX, PositionY, PositionZ, Orientation FROM spell_target_position WHERE ID = ? AND EffectIndex = ?", spellID, effIndex).Scan(&rowMap, &rx, &ry, &rz, &rOri); err != nil {
		return 0, 0, 0, 0, 0, false
	}
	if !spellHasEffect(spell, 5) && !spellHasEffect(spell, spellEffectBind) && uint32(rowMap) != s.player.Map {
		return 0, 0, 0, 0, 0, false
	}
	ori := float32(rOri)
	if ori == 0 {
		ori = s.player.Orientation
	}
	return float32(rx), float32(ry), float32(rz), ori, uint32(rowMap), true
}
