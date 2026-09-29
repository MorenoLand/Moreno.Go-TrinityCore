package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	spellAuraSchoolImmunity      uint32 = 2  // SPELL_AURA_SCHOOL_IMMUNITY (SpellAuraDefines.h:32)
	spellAuraDamageImmunity      uint32 = 4  // SPELL_AURA_DAMAGE_IMMUNITY (SpellAuraDefines.h:34)
	spellAuraReflectSpells       uint32 = 63 // SPELL_AURA_REFLECT_SPELLS (SpellAuraDefines.h:93)
	spellAuraReflectSpellsSchool uint32 = 64 // SPELL_AURA_REFLECT_SPELLS_SCHOOL (SpellAuraDefines.h:94)
)

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

	// 3. Aura effect check: SPELL_AURA_DAMAGE_IMMUNITY (4) and SPELL_AURA_SCHOOL_IMMUNITY (2)
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
