package world

import (
	"math"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Absorption aura types from TrinityCore SharedDefines.h:600-750
const (
	SpellAuraSchoolAbsorb uint32 = 69  // SPELL_AURA_SCHOOL_ABSORB (Power Word: Shield, Ice Barrier, Sacred Shield)
	SpellAuraManaShield   uint32 = 72  // SPELL_AURA_MANA_SHIELD (Mana Shield)
	SpellAuraMagicAbsorb  uint32 = 256 // SPELL_AURA_MAGIC_ABSORB (Anti-Magic Shell)
)

// getAbsorptionPriority returns the priority order for damage absorption shields.
// Mirrors TrinityCore Unit::CalcAbsorbResist (Unit.cpp:2000-2080):
// 1. Specific school shields (Fire Ward, Frost Ward, Shadow Ward)
// 2. Anti-Magic Shell / Magic Absorb (Aura 256)
// 3. Generic school shields (Power Word: Shield, Ice Barrier, Sacred Shield)
// 4. Mana Shield (Aura 72) - absorbs last to protect player mana
func getAbsorptionPriority(aura *activeAura) int {
	if aura == nil {
		return 99
	}
	switch aura.AuraType {
	case SpellAuraSchoolAbsorb:
		if aura.SchoolMask != 0 && aura.SchoolMask != 127 {
			return 1
		}
		return 3
	case SpellAuraMagicAbsorb:
		return 2
	case SpellAuraManaShield:
		return 4
	default:
		return 5
	}
}

// absorbIgnorePct mirrors the Unit::CalcAbsorbResist absorb-ignore arm
// (Unit.cpp:1839-1857): the attacker's max positive
// SPELL_AURA_MOD_TARGET_ABSORB_SCHOOL (194) amount whose MiscValue overlaps
// the school mask, clamped to 0..100. That percent of the post-resist damage
// bypasses the victim's absorb shields. Melee-only entry: the sibling 245
// (MOD_TARGET_ABILITY_ABSORB_SCHOOL) predicate requires a spell
// (AuraEffect::IsAffectedOnSpell is false for nil, SpellAuraEffects.cpp:848),
// so 245 never applies to melee.
func (s *session) absorbIgnorePct(schoolMask uint32) float64 {
	if s == nil {
		return 0
	}
	pct := s.maxPositiveAuraModifierByMiscMask(spellAuraModTargetAbsorbSchool, int32(schoolMask))
	if pct > 100 {
		pct = 100
	}
	return float64(pct)
}

// absorbIgnorePctForSpell is the spell-damage entry of the same CalcAbsorbResist
// arm: max(194 pct, the attacker's max positive
// SPELL_AURA_MOD_TARGET_ABILITY_ABSORB_SCHOOL (245) amount whose MiscValue
// overlaps the school mask and whose aura spell affects the damaging spell,
// Unit.cpp:1840-1848), clamped to 0..100.
func (s *session) absorbIgnorePctForSpell(schoolMask uint32, spellID uint32) float64 {
	if s == nil {
		return 0
	}
	pct := s.maxPositiveAuraModifierByMiscMask(spellAuraModTargetAbsorbSchool, int32(schoolMask))
	if spellID != 0 && s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			if ability := s.maxPositiveAuraModifierByAffectMask(spellAuraModTargetAbilityAbsorbSchool, spell, schoolMask); ability > pct {
				pct = ability
			}
		}
	}
	if pct > 100 {
		pct = 100
	}
	return float64(pct)
}

// creatureAbsorbIgnorePct is the creature-attacker analog of absorbIgnorePct,
// scanning activeCreatureAuras for SPELL_AURA_MOD_TARGET_ABSORB_SCHOOL (194)
// amounts whose effect MiscValue overlaps the school mask.
func (s *Server) creatureAbsorbIgnorePct(key creatureAuraKey, schoolMask uint32) float64 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	maxValue := int32(0)
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		matched := false
		if s.Data != nil {
			if spell, found, err := s.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != spellAuraModTargetAbsorbSchool || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if effect.MiscValue&int32(schoolMask) == 0 {
						continue
					}
					if amount := aura.Amounts[index]; amount > maxValue {
						maxValue = amount
					}
					matched = true
				}
			}
		}
		if !matched && aura.AuraType == spellAuraModTargetAbsorbSchool && aura.MiscValue&int32(schoolMask) != 0 {
			if amount := int32(aura.Amount); amount > maxValue {
				maxValue = amount
			}
		}
	}
	s.auraMu.Unlock()
	if maxValue > 100 {
		maxValue = 100
	}
	return float64(maxValue)
}

// creatureMaxPositiveAuraModifierByAffectMask is the creature-attacker analog
// of maxPositiveAuraModifierByAffectMask: the largest positive
// SPELL_AURA_MOD_TARGET_ABILITY_ABSORB_SCHOOL (245) amount over
// activeCreatureAuras whose effect MiscValue overlaps the school mask and
// whose aura spell affects the damaging spell (Unit.cpp:1840-1848).
func (s *Server) creatureMaxPositiveAuraModifierByAffectMask(key creatureAuraKey, spell wotlk.Spell, schoolMask uint32) int32 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	maxValue := int32(0)
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if s.Data != nil {
			if auraSpell, found, err := s.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range auraSpell.Effects {
					if effect.Aura != spellAuraModTargetAbilityAbsorbSchool || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if effect.MiscValue&int32(schoolMask) == 0 {
						continue
					}
					if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
						continue
					}
					amount := aura.Amounts[index]
					if amount == 0 {
						amount = int32(aura.Amount)
					}
					if amount > maxValue {
						maxValue = amount
					}
				}
				continue
			}
		}
		if aura.AuraType == spellAuraModTargetAbilityAbsorbSchool && aura.MiscValue&int32(schoolMask) != 0 {
			if amount := int32(aura.Amount); amount > maxValue {
				maxValue = amount
			}
		}
	}
	s.auraMu.Unlock()
	return maxValue
}

// creatureAbsorbIgnorePctForSpell is the creature-attacker spell-damage entry
// of the CalcAbsorbResist arm: max(194 pct, 245 pct affecting the spell).
func (s *Server) creatureAbsorbIgnorePctForSpell(key creatureAuraKey, schoolMask uint32, spellID uint32) float64 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	pct := int32(s.creatureAbsorbIgnorePct(key, schoolMask))
	if spellID != 0 && s.Data != nil {
		if spell, found, err := s.Data.Spell(spellID); err == nil && found {
			if ability := s.creatureMaxPositiveAuraModifierByAffectMask(key, spell, schoolMask); ability > pct {
				pct = ability
			}
		}
	}
	if pct > 100 {
		pct = 100
	}
	return float64(pct)
}

// absorbIgnoreBypass returns the portion of damage that runs past the
// victim's absorb shields under the attacker's absorb-ignore pct (the
// CalculatePct arm of Unit::CalcAbsorbResist, Unit.cpp:1853-1855).
func absorbIgnoreBypass(damage uint32, ignorePct float64) uint32 {
	if damage == 0 || ignorePct <= 0 {
		return 0
	}
	return uint32(float64(damage) * ignorePct / 100)
}

// tickCasterAbsorbIgnorePct resolves the Unit::CalcAbsorbResist 194 arm for a
// periodic tick: the caster session's auras when the caster is a known player,
// else the creature caster's auras via its creature aura key. Melee-only
// entry: the 245 predicate needs the tick's spell, so ticks use
// tickCasterAbsorbIgnorePctForSpell.
func (s *Server) tickCasterAbsorbIgnorePct(casterSess *session, casterGUID uint64, mapID, instanceID uint32, schoolMask uint32) float64 {
	if casterSess != nil {
		return casterSess.absorbIgnorePct(schoolMask)
	}
	if s == nil || casterGUID == 0 {
		return 0
	}
	return s.creatureAbsorbIgnorePct(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: casterGUID}, schoolMask)
}

// tickCasterAbsorbIgnorePctForSpell is the periodic-tick entry of the
// CalcAbsorbResist arm: max(194 pct, 245 pct affecting the tick's spell,
// Unit.cpp:1840-1848). C++ carries the aura's SpellInfo on the tick's
// DamageInfo, so spellID is the periodic aura's spell.
func (s *Server) tickCasterAbsorbIgnorePctForSpell(casterSess *session, casterGUID uint64, mapID, instanceID uint32, schoolMask uint32, spellID uint32) float64 {
	if casterSess != nil {
		return casterSess.absorbIgnorePctForSpell(schoolMask, spellID)
	}
	if s == nil || casterGUID == 0 {
		return 0
	}
	return s.creatureAbsorbIgnorePctForSpell(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: casterGUID}, schoolMask, spellID)
}

// applyAbsorptionShields applies active absorption shields (Power Word: Shield, Ice Barrier, etc.)
// to mitigate incoming damage of the given school mask according to TrinityCore priority order.
// Returns absorbed damage and remaining unabsorbed damage.
// Reference: TrinityCore Unit::CalcAbsorbResist (Unit.cpp:2000-2080).
func (s *session) applyAbsorptionShields(damage uint32, schoolMask uint8) (absorbed uint32, remainingDamage uint32) {
	if s == nil || s.player == nil || damage == 0 {
		return 0, damage
	}

	remainingDamage = damage
	var exhaustedSpells []uint32

	s.castMu.Lock()
	if s.activeAuras != nil {
		var shieldList []*activeAura
		for _, aura := range s.activeAuras {
			if aura != nil && !aura.Stopped && aura.Amount > 0 {
				shieldList = append(shieldList, aura)
			}
		}
		sort.SliceStable(shieldList, func(i, j int) bool {
			pI := getAbsorptionPriority(shieldList[i])
			pJ := getAbsorptionPriority(shieldList[j])
			if pI != pJ {
				return pI < pJ
			}
			return shieldList[i].SpellID < shieldList[j].SpellID
		})

		for _, aura := range shieldList {
			if remainingDamage == 0 {
				break
			}

			// 1. School Absorb (SPELL_AURA_SCHOOL_ABSORB = 69)
			if aura.AuraType == SpellAuraSchoolAbsorb {
				// SchoolMask == 0 absorbs all schools, otherwise bitmask match
				if aura.SchoolMask == 0 || (aura.SchoolMask&uint32(schoolMask)) != 0 {
					absorbThis := aura.Amount
					if absorbThis > remainingDamage {
						absorbThis = remainingDamage
					}
					aura.Amount -= absorbThis
					remainingDamage -= absorbThis
					absorbed += absorbThis
					if aura.Amount == 0 {
						exhaustedSpells = append(exhaustedSpells, aura.SpellID)
					}
				}
				continue
			}

			// 2. Magic Absorb (SPELL_AURA_MAGIC_ABSORB = 256)
			// Absorbs non-physical magical damage (Anti-Magic Shell)
			if aura.AuraType == SpellAuraMagicAbsorb && schoolMask&1 == 0 {
				if aura.SchoolMask == 0 || (aura.SchoolMask&uint32(schoolMask)) != 0 {
					absorbThis := aura.Amount
					if absorbThis > remainingDamage {
						absorbThis = remainingDamage
					}
					aura.Amount -= absorbThis
					remainingDamage -= absorbThis
					absorbed += absorbThis
					if aura.Amount == 0 {
						exhaustedSpells = append(exhaustedSpells, aura.SpellID)
					}
				}
				continue
			}

			// 3. Mana Shield (SPELL_AURA_MANA_SHIELD = 72)
			// In WotLK, Mana Shield absorbs all damage and drains 1.5 mana per point absorbed
			if aura.AuraType == SpellAuraManaShield && s.player.Powers[0] > 0 {
				currMana := s.player.Powers[0]
				neededMana := uint32(math.Ceil(float64(remainingDamage) * 1.5))
				absorbPossible := remainingDamage
				if currMana < neededMana {
					absorbPossible = uint32(float64(currMana) / 1.5)
				}
				if absorbPossible > aura.Amount {
					absorbPossible = aura.Amount
				}
				if absorbPossible > 0 {
					manaDrain := uint32(math.Ceil(float64(absorbPossible) * 1.5))
					if manaDrain > s.player.Powers[0] {
						s.player.Powers[0] = 0
					} else {
						s.player.Powers[0] -= manaDrain
					}
					aura.Amount -= absorbPossible
					remainingDamage -= absorbPossible
					absorbed += absorbPossible
					if aura.Amount == 0 {
						exhaustedSpells = append(exhaustedSpells, aura.SpellID)
					}
				}
				continue
			}
		}
	}
	s.castMu.Unlock()

	// Remove exhausted shields outside the lock
	for _, id := range exhaustedSpells {
		s.removeAura(id)
	}

	return absorbed, remainingDamage
}

// applyCreatureAbsorptionShields applies active absorption shields on a creature.
func (s *Server) applyCreatureAbsorptionShields(key creatureAuraKey, damage uint32, schoolMask uint8) (absorbed uint32, remainingDamage uint32) {
	if s == nil || key.GUID == 0 || damage == 0 {
		return 0, damage
	}

	remainingDamage = damage
	var exhaustedSpells []uint32

	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok && auras != nil {
			var shieldList []*activeAura
			for _, aura := range auras {
				if aura != nil && !aura.Stopped && aura.Amount > 0 {
					shieldList = append(shieldList, aura)
				}
			}
			sort.SliceStable(shieldList, func(i, j int) bool {
				pI := getAbsorptionPriority(shieldList[i])
				pJ := getAbsorptionPriority(shieldList[j])
				if pI != pJ {
					return pI < pJ
				}
				return shieldList[i].SpellID < shieldList[j].SpellID
			})

			for _, aura := range shieldList {
				if remainingDamage == 0 {
					break
				}
				if aura.AuraType == SpellAuraSchoolAbsorb || (aura.AuraType == SpellAuraMagicAbsorb && schoolMask&1 == 0) {
					if aura.SchoolMask == 0 || (aura.SchoolMask&uint32(schoolMask)) != 0 {
						absorbThis := aura.Amount
						if absorbThis > remainingDamage {
							absorbThis = remainingDamage
						}
						aura.Amount -= absorbThis
						remainingDamage -= absorbThis
						absorbed += absorbThis
						if aura.Amount == 0 {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
					}
				}
			}
		}
	}
	s.auraMu.Unlock()

	for _, id := range exhaustedSpells {
		s.removeCreatureAura(key, id)
	}

	return absorbed, remainingDamage
}
