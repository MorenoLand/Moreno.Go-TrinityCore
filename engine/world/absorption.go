package world

import (
	"math"
	"sort"
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
