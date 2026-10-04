package world

// SpellAuraInterruptFlags bitmask values from TrinityCore SharedDefines.h:925-955
const (
	auraInterruptFlagHitBySpell     uint32 = 0x00000001 // removed by any damage (direct or periodic)
	auraInterruptFlagTakeDamage     uint32 = 0x00000002 // removed by damage taken
	auraInterruptFlagCast           uint32 = 0x00000004 // removed by casting
	auraInterruptFlagMove           uint32 = 0x00000008 // removed by moving
	auraInterruptFlagTurning        uint32 = 0x00000010 // removed by turning
	auraInterruptFlagJump           uint32 = 0x00000020 // removed by jumping
	auraInterruptFlagNotMounted     uint32 = 0x00000040 // removed by dismounting
	auraInterruptFlagNotSeated      uint32 = 0x00040000 // removed by standing up (AURA_INTERRUPT_FLAG_NOT_SEATED, SpellDefines.h:65)
	auraInterruptFlagChangeMap      uint32 = 0x00000100 // removed by changing map
	auraInterruptFlagEnterCombat    uint32 = 0x00000400 // removed on entering combat
	auraInterruptFlagEnterPvPCombat uint32 = 0x00800000 // removed by entering pvp combat (AURA_INTERRUPT_FLAG_ENTER_PVP_COMBAT, SpellDefines.h:70)
	auraInterruptFlagDirectDamage   uint32 = 0x00001000 // removed only by direct damage
	auraInterruptFlagLanding        uint32 = 0x02000000 // removed on landing
	auraInterruptFlagSpellAttack    uint32 = 0x00002000 // AURA_INTERRUPT_FLAG_SPELL_ATTACK (SpellDefines.h:60)
	auraInterruptFlagMount          uint32 = 0x00020000 // removed by mounting (AURA_INTERRUPT_FLAG_MOUNT, SpellDefines.h:64)
)

// getSpellAuraInterruptFlags returns the aura interrupt bitmask from Spell.dbc with
// hardcoded fallbacks for primary crowd-control and utility spells if DBC is unavailable.
func getSpellAuraInterruptFlags(spellID uint32, dbcFlags uint32) uint32 {
	if dbcFlags != 0 {
		return dbcFlags
	}
	switch spellID {
	case 1776: // Gouge (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 6770, 2070, 11297: // Sap (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 2094: // Blind (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 118, 12824, 12825, 12826, 28271, 28272, 61305, 61721, 61780: // Polymorph (Mage)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 1499, 14308, 14309, 3355: // Freezing Trap (Hunter)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 20066: // Repentance (Paladin)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 2637, 18657, 18658: // Hibernate (Druid)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 9484, 9485, 10955: // Shackle Undead (Priest)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 1784: // Stealth (Rogue)
		return auraInterruptFlagTakeDamage
	case 5215: // Prowl (Druid)
		return auraInterruptFlagTakeDamage
	case 58984: // Shadowmeld (Night Elf)
		return auraInterruptFlagTakeDamage | auraInterruptFlagMove
	case 430, 431, 432, 1133, 1135, 1137: // Food / Drink
		return auraInterruptFlagMove | auraInterruptFlagTakeDamage | auraInterruptFlagCast
	default:
		return 0
	}
}

// procDamageAuras removes CC and break-on-damage auras (e.g. Gouge, Sap, Blind, Polymorph, Stealth)
// when the player takes damage. Also tracks damage threshold breaks for Root and Fear mechanics.
// Mirrors TrinityCore Unit::ProcDamageAndSpellFor and RemoveAurasWithInterruptFlags.
func (s *session) procDamageAuras(isDirectDamage bool, damage ...uint32) {
	flags := auraInterruptFlagTakeDamage | auraInterruptFlagHitBySpell
	if isDirectDamage {
		flags |= auraInterruptFlagDirectDamage
	}
	s.removeAurasWithInterruptFlags(flags)

	dmg := uint32(0)
	if len(damage) > 0 {
		dmg = damage[0]
	}
	if dmg == 0 {
		return
	}

	maxHP := uint32(1000)
	if s.player != nil && s.player.MaxHealth > 0 {
		maxHP = s.player.MaxHealth
	}

	// Root threshold: ~15% of max HP (TrinityCore Unit::ProcDamageAndSpellFor)
	rootThreshold := uint32(float64(maxHP) * 0.15)
	if rootThreshold < 100 {
		rootThreshold = 100
	}

	// Fear threshold: ~10% of max HP
	fearThreshold := uint32(float64(maxHP) * 0.10)
	if fearThreshold < 100 {
		fearThreshold = 100
	}

	var toRemove []uint32
	s.castMu.Lock()
	if s.activeAuras != nil {
		for spellID, aura := range s.activeAuras {
			if aura == nil || aura.Stopped {
				continue
			}

			// Root (SPELL_AURA_MOD_ROOT = 26 or MechanicRoot = 7)
			if aura.AuraType == 26 || aura.Mechanic == 7 {
				aura.DamageTaken += dmg
				if aura.DamageTaken >= rootThreshold {
					toRemove = append(toRemove, spellID)
				} else if isDirectDamage && float64(dmg)/float64(rootThreshold) >= 1.0 {
					toRemove = append(toRemove, spellID)
				}
				continue
			}

			// Fear (SPELL_AURA_MOD_FEAR = 7 or MechanicFear = 5)
			if aura.AuraType == 7 || aura.Mechanic == 5 {
				aura.DamageTaken += dmg
				if aura.DamageTaken >= fearThreshold {
					toRemove = append(toRemove, spellID)
				} else if isDirectDamage && float64(dmg)/float64(fearThreshold) >= 1.0 {
					toRemove = append(toRemove, spellID)
				}
				continue
			}
		}
	}
	s.castMu.Unlock()

	for _, spellID := range toRemove {
		s.removeAura(spellID)
	}
}

// procCreatureDamageAuras breaks root and fear auras on creatures after exceeding damage thresholds.
func (s *Server) procCreatureDamageAuras(key creatureAuraKey, isDirectDamage bool, damage uint32, maxHealth uint32) {
	if s == nil || key.GUID == 0 || damage == 0 {
		return
	}

	maxHP := maxHealth
	if maxHP == 0 {
		maxHP = 1000
	}
	rootThreshold := uint32(float64(maxHP) * 0.15)
	if rootThreshold < 100 {
		rootThreshold = 100
	}
	fearThreshold := uint32(float64(maxHP) * 0.10)
	if fearThreshold < 100 {
		fearThreshold = 100
	}

	var toRemove []uint32
	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok && auras != nil {
			for spellID, aura := range auras {
				if aura == nil || aura.Stopped {
					continue
				}
				if aura.AuraType == 26 || aura.Mechanic == 7 {
					aura.DamageTaken += damage
					if aura.DamageTaken >= rootThreshold {
						toRemove = append(toRemove, spellID)
					}
				} else if aura.AuraType == 7 || aura.Mechanic == 5 {
					aura.DamageTaken += damage
					if aura.DamageTaken >= fearThreshold {
						toRemove = append(toRemove, spellID)
					}
				}
			}
		}
	}
	s.auraMu.Unlock()

	for _, spellID := range toRemove {
		s.removeCreatureAura(key, spellID)
	}
}

// procCastAuras removes auras broken by casting an active spell (e.g. Food, Drink, Stealth).
func (s *session) procCastAuras() {
	s.removeAurasWithInterruptFlags(auraInterruptFlagCast)
}
