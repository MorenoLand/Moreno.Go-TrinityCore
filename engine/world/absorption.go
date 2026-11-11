package world

import (
	"math"
	"math/rand"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Absorption aura types from TrinityCore SpellAuraDefines.h
const (
	SpellAuraSchoolAbsorb uint32 = 69 // SPELL_AURA_SCHOOL_ABSORB (Power Word: Shield, Ice Barrier, Sacred Shield)
	SpellAuraManaShield   uint32 = 97 // SPELL_AURA_MANA_SHIELD (Mana Shield, SpellAuraDefines.h:177)
)

// NOTE: there is no SPELL_AURA_MAGIC_ABSORB in this C++ tree — 256 is
// SPELL_AURA_NO_REAGENT_USE (SpellAuraDefines.h:336), and Anti-Magic Shell
// (48707) is a SPELL_AURA_SCHOOL_ABSORB (69) aura with a script absorb
// handler (spell_dk.cpp). A former 256 absorb leg is therefore removed,
// not kept as an Anti-Magic Shell model.

// absorbAuraPriority returns the priority order for damage absorption shields.
// Within SPELL_AURA_SCHOOL_ABSORB (69) it mirrors TrinityCore's
// AbsorbAuraOrderPred (SpellAuraEffects.h:316-367):
//  1. Wards — mage/warlock family, category 56
//  2. Sacred Shield (58597)
//  3. Fel Blossom (28527)
//  4. Divine Aegis (47753)
//  5. Ice Barrier (category 471)
//  6. Sacrifice — warlock family, icon 693
//  7. other school-specific absorbs
//
// The 97 (Mana Shield) slot keeps the Go tree's existing combined-list
// position — C++ consumes 69 and 97 in separate CalcAbsorbResist loops
// (Unit.cpp:1864 vs 1914), so the merged pass is already a Go-side
// approximation; generic 69s stay ahead of mana shield, matching C++.
func (s *Server) absorbAuraPriority(aura *activeAura) int {
	if aura == nil {
		return 99
	}
	if aura.AuraType == SpellAuraSchoolAbsorb {
		var spell wotlk.Spell
		known := false
		if s != nil && s.Data != nil {
			if sp, found, err := s.Data.Spell(aura.SpellID); err == nil && found {
				spell, known = sp, true
			}
		}
		if known {
			isWard := (spell.SpellFamilyName == spellFamilyMage || spell.SpellFamilyName == spellFamilyWarlock) && spell.Category == 56
			if isWard {
				return 1
			}
			switch aura.SpellID {
			case 58597: // Sacred Shield
				return 2
			case 28527: // Fel Blossom
				return 3
			case 47753: // Divine Aegis
				return 4
			}
			if spell.Category == 471 { // Ice Barrier
				return 5
			}
			if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellIconID == 693 { // Sacrifice
				return 6
			}
		}
		if aura.SchoolMask != 0 && aura.SchoolMask != 127 {
			return 7
		}
		return 9
	}
	switch aura.AuraType {
	case SpellAuraManaShield:
		return 10
	default:
		return 99
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

// creatureIgnoreTargetResistArmorForSpell mirrors the creature-attacker arms
// of Unit::CalcArmorReducedDamage (Unit.cpp:1632-1650) on the direct-spell
// path (non-null spellInfo): the attacker's SPELL_AURA_MOD_TARGET_RESISTANCE
// (123) NORMAL-misc flat sum lands on the victim's armor first, then each
// SPELL_AURA_MOD_ABILITY_IGNORE_TARGET_RESIST (270) NORMAL-misc effect whose
// granting spell affects the cast spell, then each
// SPELL_AURA_MOD_IGNORE_TARGET_RESIST (269) NORMAL-misc effect (no affect
// gate), shrinks it by floor(armor*(100-amount)/100) per effect. The
// SPELLMOD_IGNORE_ARMOR leg is player-only (a creature has no spell mod
// owner), and the rating/280 armor-penetration leg is TYPEID_PLAYER-gated in
// C++ — a creature caster gets neither. An unknown spell skips the 270
// affect gate (the 194-side fallback convention).
func (s *Server) creatureIgnoreTargetResistArmorForSpell(key creatureAuraKey, armor float64, spell wotlk.Spell, spellKnown bool) float64 {
	if armor <= 0 {
		return armor
	}
	var flat int32
	for _, amount := range s.creatureAuraTypeAmountsByMiscMask(key, spellAuraModTargetResistance, spellSchoolMaskNormal) {
		flat += amount
	}
	armor += float64(flat)
	if spellKnown && s != nil && s.Data != nil {
		s.auraMu.Lock()
		for _, aura := range s.activeCreatureAuras[key] {
			if aura == nil || aura.Stopped {
				continue
			}
			auraSpell, found, err := s.Data.Spell(aura.SpellID)
			if err != nil || !found {
				continue
			}
			for index, effect := range auraSpell.Effects {
				if effect.Aura != spellAuraModAbilityIgnoreTargetResist || aura.EffectMask&(1<<uint(index)) == 0 {
					continue
				}
				if uint32(effect.MiscValue)&spellSchoolMaskNormal == 0 {
					continue
				}
				if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
					continue
				}
				amount := int32(0)
				if index < len(aura.Amounts) {
					amount = aura.Amounts[index]
				}
				if amount == 0 {
					amount = int32(aura.Amount)
				}
				if amount == 0 {
					amount = effect.BasePoints
				}
				armor = math.Floor(armor * (100 - float64(amount)) / 100)
			}
		}
		s.auraMu.Unlock()
	}
	for _, amount := range s.creatureAuraTypeAmountsByMiscMask(key, spellAuraModIgnoreTargetResist, spellSchoolMaskNormal) {
		armor = math.Floor(armor * (100 - float64(amount)) / 100)
	}
	if armor < 0 {
		armor = 0
	}
	return armor
}

// creatureIgnoreResistPctForSpell mirrors the attacker's ignore-resist legs of
// Unit::CalcSpellResistedDamage (Unit.cpp:1749-1760) for creature casters: the
// SPELL_AURA_MOD_ABILITY_IGNORE_TARGET_RESIST (270) amounts whose misc mask
// overlaps the spell's school mask and whose aura spell affects the cast spell,
// plus the SPELL_AURA_MOD_IGNORE_TARGET_RESIST (269) misc-masked sum, capped at
// 100 only — negative amounts boost the resisted portion, like C++ and the
// player-side casterIgnoreResistPct. An unknown spell skips the 270 leg.
func (s *Server) creatureIgnoreResistPctForSpell(key creatureAuraKey, spell wotlk.Spell, spellKnown bool, schoolMask uint32) int32 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	var pct int32
	for _, amount := range s.creatureAuraTypeAmountsByMiscMask(key, spellAuraModIgnoreTargetResist, schoolMask) {
		pct += amount
	}
	if spellKnown && s.Data != nil {
		s.auraMu.Lock()
		for _, aura := range s.activeCreatureAuras[key] {
			if aura == nil || aura.Stopped {
				continue
			}
			auraSpell, found, err := s.Data.Spell(aura.SpellID)
			if err != nil || !found {
				continue
			}
			for index, effect := range auraSpell.Effects {
				if effect.Aura != spellAuraModAbilityIgnoreTargetResist || aura.EffectMask&(1<<uint(index)) == 0 {
					continue
				}
				if uint32(effect.MiscValue)&schoolMask == 0 {
					continue
				}
				if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
					continue
				}
				amount := int32(0)
				if index < len(aura.Amounts) {
					amount = aura.Amounts[index]
				}
				if amount == 0 {
					amount = int32(aura.Amount)
				}
				if amount == 0 {
					amount = effect.BasePoints
				}
				pct += amount
			}
		}
		s.auraMu.Unlock()
	}
	if pct > 100 {
		pct = 100
	}
	return pct
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

// scriptAbsorbPct returns the script absorb percent for a
// SPELL_AURA_SCHOOL_ABSORB aura whose C++ script overrides the per-hit
// absorb with CalculatePct(damage, pct): 50462 Anti-Magic Shell (raid)
// and 50461 Anti-Magic Zone (spell_dk.cpp:253-301, 366-434), where pct is
// the aura spell's own effect-0 CalcValue at the caster's level,
// 49145 Spell Deflection (spell_dk.cpp:2338-2340), where the same
// effect-0 CalcValue is the absorb pct gated on a parry roll (the roll
// itself lives in scriptAbsorbHit, not here), and 52284 Will of the
// Necropolis (spell_dk.cpp:2696-2701), where the same effect-0 CalcValue
// is the absorb pct gated on the target's health vs the matching-rank
// talent spell's min-HP pct (the gate itself lives in scriptAbsorbHit,
// not here). A zero or unreadable pct falls through to the generic pool leg.
func (s *Server) scriptAbsorbPct(spellID uint32, casterLevel uint8) (uint32, bool) {
	if s == nil || s.Data == nil {
		return 0, false
	}
	switch spellID {
	case 50462, 50461, 49145, 52284:
	default:
		return 0, false
	}
	spell, found, err := s.Data.Spell(spellID)
	if err != nil || !found || len(spell.Effects) == 0 {
		return 0, false
	}
	pct := spell.Effects[0].CalcValueForLevel(spell, uint32(casterLevel))
	if pct <= 0 {
		return 0, false
	}
	return uint32(pct), true
}

// scriptAbsorbHit applies a script-override school-absorb hit: the
// per-hit absorb is CalculatePct(remainingDamage, pct), not the pool.
// 50462's pool is unlimited (-1 in C++), so the amount is never
// consumed; 50461's pool is finite (talent+AP CalculateAmount, see
// auraEffectParams), so the amount is consumed and the depleted flag
// reports exhaustion for the caller's removal pass. 49145 Spell
// Deflection (spell_dk.cpp:2345-2358) adds a parry roll on top of the
// pct: the victim's parry chance (PLAYER_PARRY_PERCENTAGE for a player;
// pass a negative parryChancePct when the victim has no parry model,
// which reports the hit handled with zero absorbed — the C++ creature
// arm needs GetCreatureParryChance plus the attacker's expertise
// reduction, neither of which the absorb call sites carry). A failed
// roll absorbs nothing and leaves the pool untouched; the pool itself
// is unlimited (-1 in C++), like 50462, so it is never consumed.
// 52284 Will of the Necropolis (spell_dk.cpp:2711-2726) absorbs
// pct(remainingDamage) only when the victim's health after this damage
// drops below the min-HP pct in effect 0 of the matching-rank talent
// spell (49189, same rank as the aura); the pool is unlimited (-1 in
// C++), so it is never consumed, and a gate that does not trip absorbs
// nothing with the pool untouched. A missing rank row, talent spell, or
// victim max health falls through to the generic pool leg (today's
// behaviour) instead of inventing a gate. victimHealth/victimMaxHealth
// are the victim's current and max health; pass zeros when the victim's
// health is not resolvable.
// Every absorb call site is a direct-damage path (melee / spell hit),
// so the C++ SPELL_DIRECT_DAMAGE gate holds by construction.
func (s *Server) scriptAbsorbHit(aura *activeAura, remainingDamage uint32, parryChancePct float64, victimHealth, victimMaxHealth uint32) (uint32, bool, bool) {
	if aura == nil || remainingDamage == 0 {
		return 0, false, false
	}
	pct, ok := s.scriptAbsorbPct(aura.SpellID, aura.CasterLevel)
	if !ok {
		return 0, false, false
	}
	if aura.SpellID == 49145 {
		if parryChancePct <= 0 || rand.Float64()*100 >= parryChancePct {
			return 0, false, true
		}
		absorbThis := uint32(uint64(remainingDamage) * uint64(pct) / 100)
		if absorbThis > remainingDamage {
			absorbThis = remainingDamage
		}
		return absorbThis, false, true
	}
	if aura.SpellID == 52284 {
		rank := s.spellRank(aura.SpellID)
		talentID := s.spellWithRank(49189, rank)
		if rank == 0 || talentID == 0 || victimMaxHealth == 0 {
			return 0, false, false
		}
		talent, found, err := s.Data.Spell(talentID)
		if err != nil || !found || len(talent.Effects) == 0 {
			return 0, false, false
		}
		minHpPct := talent.Effects[0].CalcValueForLevel(talent, uint32(aura.CasterLevel))
		if minHpPct <= 0 {
			return 0, false, false
		}
		minHp := uint64(victimMaxHealth) * uint64(minHpPct) / 100
		remainingHp := int64(victimHealth) - int64(remainingDamage)
		if remainingHp >= int64(minHp) {
			return 0, false, true
		}
		absorbThis := uint32(uint64(remainingDamage) * uint64(pct) / 100)
		if absorbThis > remainingDamage {
			absorbThis = remainingDamage
		}
		return absorbThis, false, true
	}
	absorbThis := uint32(uint64(remainingDamage) * uint64(pct) / 100)
	if absorbThis > remainingDamage {
		absorbThis = remainingDamage
	}
	if aura.SpellID == 50461 {
		if absorbThis > aura.Amount {
			absorbThis = aura.Amount
		}
		aura.Amount -= absorbThis
		return absorbThis, aura.Amount == 0, true
	}
	return absorbThis, false, true
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

	// The mana-shield leg resolves each shield caster's spell-mod owner
	// through findCreatureMotion (motionMu) for pet casters, so those
	// resolutions run before castMu is taken: a motionMu holder taking
	// this session's castMu (pet spawn, pet spell damage, formation
	// broadcast) would otherwise order against this castMu->motionMu
	// leg. The absorb loop below only consults the snapshot.
	var shieldList []*activeAura
	var modOwnerGUIDs []uint64
	var mapID, instanceID uint32
	s.castMu.Lock()
	if s.player != nil {
		mapID, instanceID = s.player.Map, s.player.InstanceID
	}
	if s.activeAuras != nil {
		for _, aura := range s.activeAuras {
			if aura != nil && !aura.Stopped && aura.Amount > 0 {
				shieldList = append(shieldList, aura)
				if aura.AuraType == SpellAuraManaShield {
					modOwnerGUIDs = append(modOwnerGUIDs, aura.CasterGUID)
				}
			}
		}
	}
	s.castMu.Unlock()
	modOwners := make(map[uint64]*session, len(modOwnerGUIDs))
	for _, guid := range modOwnerGUIDs {
		if _, ok := modOwners[guid]; !ok {
			modOwners[guid] = s.server.spellModOwnerSession(mapID, instanceID, guid)
		}
	}

	s.castMu.Lock()
	if len(shieldList) > 0 {
		sort.SliceStable(shieldList, func(i, j int) bool {
			pI := s.server.absorbAuraPriority(shieldList[i])
			pJ := s.server.absorbAuraPriority(shieldList[j])
			if pI != pJ {
				return pI < pJ
			}
			return shieldList[i].SpellID < shieldList[j].SpellID
		})

		for _, aura := range shieldList {
			if remainingDamage == 0 {
				break
			}
			if aura == nil || aura.Stopped || aura.Amount == 0 {
				continue
			}

			// 1. School Absorb (SPELL_AURA_SCHOOL_ABSORB = 69)
			if aura.AuraType == SpellAuraSchoolAbsorb {
				// C++ gates on the absorb effect's MiscValue, not the aura
				// spell's school (Unit.cpp:1881): Power Word: Shield and
				// Ice Barrier carry MiscValue 127 (all schools) on a holy /
				// frost spell, so gating on SchoolMask left them absorbing
				// only their own spell school. A zero MiscValue absorbs
				// nothing, like C++ — there is no all-schools fallback.
				if aura.MiscValue&int32(schoolMask) != 0 {
					if absorbThis, depleted, ok := s.server.scriptAbsorbHit(aura, remainingDamage, float64(s.player.ParryPercentage), s.player.Health, s.player.MaxHealth); ok {
						remainingDamage -= absorbThis
						absorbed += absorbThis
						if depleted {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
						continue
					}
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

			// 2. Mana Shield (SPELL_AURA_MANA_SHIELD = 97)
			// Each point absorbed drains the spell's value multiplier mana
			// (Unit.cpp:1945-1948: int32(manaReduction * CalcValueMultiplier);
			// Mana Shield's DBC value is 2.0, talent-folded via
			// SPELLMOD_VALUE_MULTIPLIER). A zero multiplier leaves the mana
			// cost unchanged, mirroring the C++ if (float manaMultiplier).
			if aura.AuraType == SpellAuraManaShield && s.player.Powers[0] > 0 {
				// The mana-shield loop gates on the effect MiscValue like
				// the 69 loop (Unit.cpp:1922); the old code had no school
				// gate at all.
				if aura.MiscValue&int32(schoolMask) == 0 {
					continue
				}
				mult := s.calcValueMultiplierForAura(aura.SpellID, SpellAuraManaShield,
					modOwners[aura.CasterGUID])
				if mult == 0 {
					mult = 1
				}
				// Unit.cpp:1938-1953, exactly: the candidate is the
				// shield amount clamped to the running damage, the mana
				// cost truncates (int32 cast, not Ceil), and a mana
				// shortfall scales the absorb by the paid fraction,
				// truncating again. The old Ceil cost under-absorbed
				// whenever mana covered the truncated cost but not the
				// ceiling (e.g. 3 damage at 1.5x with 4 mana: C++ absorbs
				// 3, the old code 2).
				absorbCandidate := aura.Amount
				if absorbCandidate > remainingDamage {
					absorbCandidate = remainingDamage
				}
				manaReduction := int32(float64(absorbCandidate) * float64(mult))
				if manaReduction < 0 {
					manaReduction = 0
				}
				manaTaken := s.player.Powers[0]
				if manaTaken > uint32(manaReduction) {
					manaTaken = uint32(manaReduction)
				}
				absorbPossible := absorbCandidate
				if manaReduction > 0 && manaTaken < uint32(manaReduction) {
					absorbPossible = uint32(float64(absorbCandidate) * (float64(manaTaken) / float64(manaReduction)))
				}
				if absorbPossible > 0 {
					s.player.Powers[0] -= manaTaken
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

	// Remove exhausted shields outside the lock; a shield that hits zero
	// absorbed amount is destroyed by an enemy spell, not a default remove
	// (Unit.cpp:1895, 1965).
	for _, id := range exhaustedSpells {
		s.removeAuraWithMode(id, auraRemoveByEnemySpell)
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

	// The mana-shield leg drains the creature's mana pool (Powers[0], filled
	// from the template mana row at spawn); resolved ahead of the aura lock
	// so the leg below never takes motionMu under auraMu.
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)

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
				pI := s.absorbAuraPriority(shieldList[i])
				pJ := s.absorbAuraPriority(shieldList[j])
				if pI != pJ {
					return pI < pJ
				}
				return shieldList[i].SpellID < shieldList[j].SpellID
			})

			for _, aura := range shieldList {
				if remainingDamage == 0 {
					break
				}
				if aura.AuraType == SpellAuraSchoolAbsorb {
					// C++ gates on the absorb effect's MiscValue
					// (Unit.cpp:1881), not the aura spell's school — see
					// the player leg above. 256 is not an absorb type.
					if aura.MiscValue&int32(schoolMask) != 0 {
						var creatureHealth, creatureMaxHealth uint32
						if motion != nil {
							creatureHealth, creatureMaxHealth = motion.Health, motion.MaxHealth
						}
						if absorbThis, depleted, ok := s.scriptAbsorbHit(aura, remainingDamage, -1, creatureHealth, creatureMaxHealth); ok {
							remainingDamage -= absorbThis
							absorbed += absorbThis
							if depleted {
								exhaustedSpells = append(exhaustedSpells, aura.SpellID)
							}
							continue
						}
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
				// 3. Mana Shield (SPELL_AURA_MANA_SHIELD = 97): the mana-cost
				// absorb loop of Unit::CalcAbsorbResist (Unit.cpp:1914-1965)
				// runs after the school-absorb loop (the sort above already
				// orders 97 last). Each point absorbed drains 1.5 mana (the
				// DBC ValueMultiplier — CalcValueMultiplier with no
				// spellmods, the same hardcode as the player leg); when mana
				// runs out the absorb scales by the fraction actually paid
				// (Unit.cpp:1949), and the school-mask gate (Unit.cpp:1922)
				// applies like the 69 arm's.
				if aura.AuraType == SpellAuraManaShield && motion != nil && motion.Powers[0] > 0 {
					// Unit.cpp:1922 gates on the effect MiscValue, not the
					// spell school, and Unit.cpp:1945 truncates the mana
					// cost (int32 cast), so the Ceil cost below is replaced
					// with the truncated cost like the player leg.
					if aura.MiscValue&int32(schoolMask) == 0 {
						continue
					}
					absorbPossible := aura.Amount
					if absorbPossible > remainingDamage {
						absorbPossible = remainingDamage
					}
					neededMana := uint32(int32(float64(absorbPossible) * 1.5))
					manaTaken := motion.Powers[0]
					if manaTaken > neededMana {
						manaTaken = neededMana
					}
					motion.Powers[0] -= manaTaken
					if neededMana > 0 && manaTaken < neededMana {
						absorbPossible = uint32(float64(absorbPossible) * (float64(manaTaken) / float64(neededMana)))
					}
					if absorbPossible > 0 {
						aura.Amount -= absorbPossible
						remainingDamage -= absorbPossible
						absorbed += absorbPossible
						if aura.Amount == 0 {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
					}
				}
			}
		}
	}
	s.auraMu.Unlock()

	// Unit.cpp:1965: an exhausted shield is removed with
	// AURA_REMOVE_BY_ENEMY_SPELL, not the default mode.
	for _, id := range exhaustedSpells {
		s.removeCreatureAuraWithMode(key, id, auraRemoveByEnemySpell)
	}

	return absorbed, remainingDamage
}
