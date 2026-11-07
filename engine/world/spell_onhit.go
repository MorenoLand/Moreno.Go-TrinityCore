package world

import (
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellScriptOnHitDamage is the pre-damage SpellScript OnHit dispatch:
// C++ runs the OnHit handlers in Spell::PreprocessTarget, before the
// effect loop applies the hit damage (Spell.cpp:2350), so a handler's
// SetHitDamage rewrites the DBC base damage while the taken/crit/
// resist/absorb legs below still run on the rewritten value. Go has no
// SpellScript registry, so the dispatch is a per-spell table consulted
// by the creature-caster damage arms (creatureEffectSpellDamage and
// creatureEffectSpellDamageCreature) right after damage := baseDamage,
// the position of C++'s SetHitDamage. The player-caster leg is open:
// 50811/33671 are cast by the stoned unit itself, so a stoned player
// routes through the session cast pipeline, which has no dispatch yet.
func (s *Server) spellScriptOnHitDamage(spell wotlk.Spell, casterLevel uint32, casterX, casterY, victimX, victimY float32, victimHealth, victimMaxHealth, baseDamage uint32) (uint32, bool) {
	var radius float32
	radiusOK := false
	switch spell.ID {
	case 50811, 33671:
		// spell_krystallus_shatter_effect / spell_gruul_shatter_effect
		// (boss_krystallus.cpp:184-195, boss_gruul.cpp:311-323): the
		// falloff needs EFFECT_0 CalcRadius(caster); C++ returns early
		// on a zero radius, so a failed DBC lookup leaves the damage.
		if s != nil && s.Data != nil {
			if value, ok, err := s.Data.SpellRadius(spell.Effects[0].RadiusIndex, casterLevel); err == nil && ok {
				radius, radiusOK = value, true
			}
		}
	}
	return onHitDamageRewrite(spell.ID, radius, radiusOK, distance2D(casterX, casterY, victimX, victimY), victimHealth, victimMaxHealth, baseDamage)
}

// onHitDamageRewrite is the pure per-spell rewrite table behind
// spellScriptOnHitDamage: (rewritten damage, true) when the spell has
// an OnHit SetHitDamage arm, (baseDamage, false) otherwise. Distances
// are the C++ GetDistance2d values; the float products truncate toward
// zero on the int32 conversion, matching SetHitDamage(int32(...)).
func onHitDamageRewrite(spellID uint32, radius float32, radiusOK bool, distance float64, victimHealth, victimMaxHealth, baseDamage uint32) (uint32, bool) {
	switch spellID {
	case 48292:
		// spell_dark_slash (boss_ymiron.cpp:315-334): the OnHit handler
		// sets the hit damage to ceil(victim health / 2) before the
		// effect loop; health is pre-damage here, matching
		// GetHitUnit()->GetHealth() at OnHit time.
		return (victimHealth + 1) / 2, true
	case 33666, 38795:
		// spell_murmur_sonic_boom_effect (boss_murmur.cpp:192-211):
		// damage = 80% of the target's max health
		// (CountPctFromMaxHealth(80) = float(maxHealth)*80/100,
		// Util.h:66-69). The class header binds both the normal (33666)
		// and heroic (38795) effect ids.
		return uint32(float32(victimMaxHealth) * 80.0 / 100.0), true
	case 52961:
		// spell_loken_pulsing_shockwave (boss_loken.cpp:222-230):
		// OnEffectHitTarget EFFECT_0 SCHOOL_DAMAGE scales the hit
		// damage by the caster-to-target 2D distance past 1 yard.
		if distance > 1.0 {
			return clampOnHitDamage(float64(baseDamage) * distance), true
		}
		return baseDamage, false
	case 50811, 33671:
		// spell_krystallus_shatter_effect (boss_krystallus.cpp:184-195)
		// and spell_gruul_shatter_effect (boss_gruul.cpp:311-323):
		// damage falls off linearly from the EFFECT_0 radius past 1
		// yard. The factor clamps at zero: hit targets are selected
		// inside the radius, so C++ never produces a negative factor
		// in practice, and Go damage is unsigned.
		if !radiusOK || radius <= 0 {
			return baseDamage, false
		}
		if distance > 1.0 {
			factor := (float64(radius) - distance) / float64(radius)
			if factor < 0 {
				factor = 0
			}
			return clampOnHitDamage(float64(baseDamage) * factor), true
		}
		return baseDamage, false
	}
	return baseDamage, false
}

// clampOnHitDamage bounds a rewritten float damage to the int32 range
// before the unsigned conversion: C++ stores the hit damage as int32,
// and the distance/falloff products stay far below the bound in
// practice, so this only guards the conversion, never live values.
func clampOnHitDamage(value float64) uint32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < 0 {
		return 0
	}
	return uint32(int32(value))
}
