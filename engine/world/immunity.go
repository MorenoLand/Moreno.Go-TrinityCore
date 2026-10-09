package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	spellAuraSchoolImmunity        uint32 = 39         // SPELL_AURA_SCHOOL_IMMUNITY (SpellAuraDefines.h:119) — was mislabeled as 2 (SPELL_AURA_MOD_POSSESS); corrected 2026-10-03 during the CheckCasterAuras audit
	spellAuraDamageImmunity        uint32 = 4          // SPELL_AURA_DAMAGE_IMMUNITY (SpellAuraDefines.h:34)
	spellAuraReflectSpells         uint32 = 63         // SPELL_AURA_REFLECT_SPELLS (SpellAuraDefines.h:93)
	spellAuraReflectSpellsSchool   uint32 = 64         // SPELL_AURA_REFLECT_SPELLS_SCHOOL (SpellAuraDefines.h:94)
	spellAuraModMaxAffectedTargets uint32 = 277        // SPELL_AURA_MOD_MAX_AFFECTED_TARGETS (SpellAuraDefines.h:357)
	spellAuraModIgnoreShapeshift   uint32 = 275        // SPELL_AURA_MOD_IGNORE_SHAPESHIFT (SpellAuraDefines.h:355)
	spellAttr3IgnoreHitResult      uint32 = 0x00040000 // SPELL_ATTR3_IGNORE_HIT_RESULT (SharedDefines.h:541) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
)

// isTotalImmune mirrors Player::isTotalImmune (Player.cpp:24785-24798): any
// SPELL_AURA_SCHOOL_IMMUNITY (39) aura counts — C++ ORs the aura effects'
// misc values into immuneMask and tests (immuneMask & SPELL_SCHOOL_MASK_ALL)
// as a boolean, so any covered school (not just full coverage) trips it.
func (s *session) isTotalImmune() bool {
	if s == nil || s.player == nil {
		return false
	}

	s.castMu.Lock()
	defer s.castMu.Unlock()

	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == spellAuraSchoolImmunity {
			return true
		}
	}
	return false
}

// isImmuneToDamage determines whether the player is immune to damage of the given schoolMask.
// Mirrors TrinityCore Unit::IsImmuneToDamage (Unit.cpp:8950-9050).
func (s *session) isImmuneToDamage(schoolMask uint32) bool {
	if s == nil || s.player == nil {
		return false
	}

	s.castMu.Lock()
	defer s.castMu.Unlock()

	// 1. Total damage immunities (all damage schools)
	// Divine Shield (642), Ice Block (45438), Cyclone (33786), Banish (710, 18647)
	totalImmunitySpells := []uint32{642, 45438, 33786, 710, 18647}
	for _, id := range totalImmunitySpells {
		if _, ok := s.auras[id]; ok {
			return true
		}
		if _, ok := s.activeAuras[id]; ok {
			return true
		}
	}

	// 2. Physical damage immunity
	// Blessing of Protection (1022, 5599, 10278) / Hand of Protection
	if schoolMask&1 != 0 {
		bopSpells := []uint32{1022, 5599, 10278}
		for _, id := range bopSpells {
			if _, ok := s.auras[id]; ok {
				return true
			}
			if _, ok := s.activeAuras[id]; ok {
				return true
			}
		}
	}

	// 3. Aura effect check: SPELL_AURA_DAMAGE_IMMUNITY (4) and SPELL_AURA_SCHOOL_IMMUNITY (39)
	for _, aura := range s.activeAuras {
		if aura == nil {
			continue
		}
		if aura.AuraType == spellAuraDamageImmunity {
			return true
		}
		if aura.AuraType == spellAuraSchoolImmunity {
			if aura.SchoolMask == 0 || (aura.SchoolMask&schoolMask != 0) {
				return true
			}
		}
	}

	return false
}

// isImmuneToSpell determines whether the player is immune to the effects of an incoming spell.
// Mirrors TrinityCore Unit::IsImmuneToSpell (Spell.cpp:6300-6450).
func (s *session) isImmuneToSpell(spell wotlk.Spell) bool {
	if s == nil || s.player == nil {
		return false
	}

	s.castMu.Lock()
	defer s.castMu.Unlock()

	harmful := isHarmfulSpell(spell)

	// Cyclone (33786) and Banish (710, 18647) make the unit immune to ALL spells (beneficial or harmful)
	// except Banish itself on a banished target
	if _, ok := s.auras[33786]; ok {
		return true
	}
	if _, ok := s.activeAuras[33786]; ok {
		return true
	}
	if (s.hasAuraInLock(710) || s.hasAuraInLock(18647)) && spell.ID != 710 && spell.ID != 18647 {
		return true
	}

	if !harmful {
		return false
	}

	// Harmful spell immunities:
	// Divine Shield (642) and Ice Block (45438) make the caster immune to all harmful spells
	if s.hasAuraInLock(642) || s.hasAuraInLock(45438) {
		return true
	}

	// Blessing of Protection (1022, 5599, 10278) makes target immune to physical spells
	if spell.SchoolMask == 0 || spell.SchoolMask&1 != 0 {
		if s.hasAuraInLock(1022) || s.hasAuraInLock(5599) || s.hasAuraInLock(10278) {
			return true
		}
	}

	// Anti-Magic Shell (48707) grants immunity to harmful magical debuffs
	if spell.SchoolMask&1 == 0 && s.hasAuraInLock(48707) {
		return true
	}

	// Cloak of Shadows (31224) grants immunity to harmful magical spells
	if spell.SchoolMask&1 == 0 && s.hasAuraInLock(31224) {
		return true
	}

	// Check SPELL_AURA_SCHOOL_IMMUNITY or SPELL_AURA_DAMAGE_IMMUNITY
	for _, aura := range s.activeAuras {
		if aura == nil {
			continue
		}
		if aura.AuraType == spellAuraDamageImmunity {
			return true
		}
		if aura.AuraType == spellAuraSchoolImmunity {
			if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
				return true
			}
		}
	}

	return false
}

// hasAuraInLock checks if the session has an aura while castMu is already held.
func (s *session) hasAuraInLock(spellID uint32) bool {
	if s.auras != nil {
		if _, ok := s.auras[spellID]; ok {
			return true
		}
	}
	if s.activeAuras != nil {
		if _, ok := s.activeAuras[spellID]; ok {
			return true
		}
	}
	return false
}

// spellCanBeReflected mirrors Spell::prepare's m_canReflect computation
// (Spell.cpp:622): only spells of the magic damage class (Spell.dbc field
// 213, DefenseType) that are not abilities, passives, invulnerability-piercing
// or flagged unreflectable can be reflected.
func spellCanBeReflected(spell wotlk.Spell) bool {
	if spell.DefenseType != spellDamageClassMagic {
		return false
	}
	if spell.Attributes&(spellAttr0Ability|spellAttr0UnaffectedByInvulnerability|spellAttributePassive) != 0 {
		return false
	}
	return spell.AttributesEx&spellAttr1CantBeReflected == 0
}

// checkSpellReflection checks if the incoming harmful spell is reflected by the target.
// If reflected, the reflection aura is consumed and returns true.
// Mirrors TrinityCore Unit::CheckSpellReflection (Unit.cpp:8230-8350).
func (s *session) checkSpellReflection(spell wotlk.Spell) bool {
	if s == nil || s.player == nil {
		return false
	}

	// Can only reflect harmful non-channeled spells
	if !isHarmfulSpell(spell) || isChanneledSpell(spell) {
		return false
	}

	if !spellCanBeReflected(spell) {
		return false
	}

	var reflectSpellID uint32

	s.castMu.Lock()
	// 1. Warrior Spell Reflection (23920)
	if _, ok := s.auras[23920]; ok {
		reflectSpellID = 23920
	} else if _, ok := s.activeAuras[23920]; ok {
		reflectSpellID = 23920
	}

	// 2. Check aura type SPELL_AURA_REFLECT_SPELLS (63) or SPELL_AURA_REFLECT_SPELLS_SCHOOL (64)
	if reflectSpellID == 0 {
		for _, aura := range s.activeAuras {
			if aura == nil {
				continue
			}
			if aura.AuraType == spellAuraReflectSpells {
				reflectSpellID = aura.SpellID
				break
			}
			if aura.AuraType == spellAuraReflectSpellsSchool {
				if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
					reflectSpellID = aura.SpellID
					break
				}
			}
		}
	}
	s.castMu.Unlock()

	if reflectSpellID != 0 {
		// Reflection consumes the buff
		s.removeAura(reflectSpellID)
		return true
	}

	return false
}

// isImmunedToSpellEffect mirrors Unit::IsImmunedToSpellEffect
// (Unit.cpp:7952-7994) with Player::IsImmunedToSpellEffect's pre-arms
// (Player.cpp:2015-2024): players are immune to the taunt aura and the
// ATTACK_ME effect. Go has no m_spellImmune container (ApplySpellImmune
// is unbridged), so the IMMUNITY_* lists are derived live from the
// target's active auras — the same data the C++ writers feed through
// SpellInfo::ApplyAllSpellImmunitiesTo (SpellInfo.cpp:2860-2920):
//   - IMMUNITY_EFFECT <- SPELL_AURA_EFFECT_IMMUNITY (37) misc (:2920)
//   - IMMUNITY_MECHANIC <- SPELL_AURA_MECHANIC_IMMUNITY (77) via
//     spellAllowedMechanicMask (the _LoadImmunityInfo carve-outs + misc
//     mapping, :2877) and SPELL_AURA_MECHANIC_IMMUNITY_MASK (147) bits
//   - IMMUNITY_STATE <- SPELL_AURA_STATE_IMMUNITY (38) misc (:2913)
//
// The SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (267) arm is a live
// aura-list check in C++ (no m_spellImmune entry; handler is
// HandleNoImmediateEffect, SpellAuraEffects.cpp:332), mirrored here.
// Creature-side writers (template mechanic/school masks, NO_TAUNT and
// IMMUNITY_KNOCKBACK flag-extra, the Totem/Creature override arms) and
// the IMMUNITY_DISPEL/IMMUNITY_SCHOOL/IMMUNITY_DAMAGE lists (never
// consulted by IsImmunedToSpellEffect) have no Go model and stay
// documented gaps.
func (s *session) isImmunedToSpellEffect(spell wotlk.Spell, effIndex int, caster *session) bool {
	if effIndex < 0 || effIndex >= len(spell.Effects) {
		return false
	}
	eff := spell.Effects[effIndex]
	if eff.Effect == 0 {
		return false
	}
	// Player::IsImmunedToSpellEffect pre-arms (Player.cpp:2018-2022):
	// players are immune to taunt — the aura and the spell effect.
	if eff.Aura == spellAuraModTaunt || eff.Effect == spellEffectAttackMe {
		return true
	}
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	if s == nil || s.player == nil {
		return false
	}
	type immuneAura struct {
		auraType uint32
		misc     int32
		spellID  uint32
	}
	var auras []immuneAura
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil {
			continue
		}
		switch aura.AuraType {
		case spellAuraEffectImmunity, spellAuraMechanicImmunity,
			spellAuraMechanicImmunityMask, spellAuraStateImmunity,
			spellAuraModImmuneAuraApplySchool:
			auras = append(auras, immuneAura{aura.AuraType, aura.MiscValue, aura.SpellID})
		}
	}
	s.castMu.Unlock()
	for _, a := range auras {
		switch a.auraType {
		case spellAuraEffectImmunity: // IMMUNITY_EFFECT (SpellInfo.cpp:2920)
			if a.misc == int32(eff.Effect) {
				return true
			}
		case spellAuraMechanicImmunity, spellAuraMechanicImmunityMask: // IMMUNITY_MECHANIC (SpellInfo.cpp:2877)
			if eff.Mechanic == 0 {
				continue
			}
			if a.auraType == spellAuraMechanicImmunityMask {
				if uint32(a.misc)&(1<<eff.Mechanic) != 0 {
					return true
				}
				continue
			}
			if s.server == nil || s.server.Data == nil {
				continue
			}
			auraSpell, found, err := s.server.Data.Spell(a.spellID)
			if err != nil || !found {
				continue
			}
			if spellAllowedMechanicMask(auraSpell)&(1<<eff.Mechanic) != 0 {
				return true
			}
		}
	}
	if spell.AttributesEx3&spellAttr3IgnoreHitResult == 0 && eff.Aura != 0 {
		for _, a := range auras {
			if a.auraType == spellAuraStateImmunity && a.misc == int32(eff.Aura) { // IMMUNITY_STATE (SpellInfo.cpp:2913)
				return true
			}
		}
		if spell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune == 0 {
			for _, a := range auras {
				if a.auraType != spellAuraModImmuneAuraApplySchool {
					continue
				}
				if uint32(a.misc)&spell.SchoolMask == 0 {
					continue
				}
				// (caster && !IsFriendlyTo(caster)) || !IsPositiveEffect(index) (Unit.cpp:7971)
				if caster == nil || !s.isFriendlyToPlayer(caster) || !spell.IsPositiveEffect(effIndex) {
					return true
				}
			}
		}
	}
	return false
}

// spellTargetFullyEffectImmune reduces the AddUnitTarget effect-immune
// strip (Spell.cpp:2108-2112) to its IMMUNE2 consequence: true when the
// spell has at least one effect and every non-zero effect is per-effect
// immune on the target. (A spell with zero effects never reaches the
// strip in C++ — AddUnitTarget returns at the "no effects left" gate,
// Spell.cpp:2105 — so it reports no IMMUNE2.)
func (s *session) spellTargetFullyEffectImmune(spell wotlk.Spell, targetSess *session) bool {
	anyEffect := false
	for i := range spell.Effects {
		if spell.Effects[i].Effect == 0 {
			continue
		}
		anyEffect = true
		if !targetSess.isImmunedToSpellEffect(spell, i, s) {
			return false
		}
	}
	return anyEffect
}
