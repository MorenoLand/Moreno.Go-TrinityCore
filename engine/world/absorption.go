package world

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"time"

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
// absorbHookPending is one deferred OnEffectAbsorb hook: the aura, the
// pool candidate and the remaining damage at its turn in the absorb
// order. The hook fires after the absorb lock is released (see
// fireAuraAbsorb), so the loop records the C++ hook inputs here and
// the post-lock pass applies the returned amount.
type absorbHookPending struct {
	aura   *activeAura
	pool   uint32
	damage uint32
}

// absorbHookPoolUnlimited reports whether the C++ script for the spell
// sets an unlimited (-1) absorb pool via DoEffectCalcAmount, so the
// hook's absorb is never consumed from the Go pool and the aura is
// never exhausted by it (Unit.cpp:1909-1914). Go stores the DBC base
// in Amount instead, so the set is per-spell knowledge, like the
// scriptAbsorbHit switch: moonkin form passive (69366) and primal
// tenacity (33851) both set -1, and cheat death (31228) sets -1 via
// DoEffectCalcAmount (spell_rogue.cpp:137-141). Any other hook
// spell's pool is finite and decrements like the generic leg.
func absorbHookPoolUnlimited(spellID uint32) bool {
	switch spellID {
	case 69366, 33851, 31228:
		return true
	}
	return false
}

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
// itself lives in scriptAbsorbHit, not here), 52284 Will of the
// Necropolis (spell_dk.cpp:2696-2701), where the same effect-0 CalcValue
// is the absorb pct gated on the target's health vs the matching-rank
// talent spell's min-HP pct (the gate itself lives in scriptAbsorbHit,
// not here), and 51474 Astral Shift (spell_shaman.cpp:205-231), where
// the same effect-0 CalcValue is the absorb pct gated on the
// stun/fear/silence flags (the gate itself lives in astralShiftAbsorbHit,
// not here — scriptAbsorbHit has no flag/mechanic inputs). A zero or
// unreadable pct falls through to the generic pool leg.
func (s *Server) scriptAbsorbPct(spellID uint32, casterLevel uint8) (uint32, bool) {
	if s == nil || s.Data == nil {
		return 0, false
	}
	switch spellID {
	case 50462, 50461, 49145, 52284, 51474:
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

// ardentDefenderAbsorbHit mirrors spell_pal_ardent_defender::Absorb
// (spell_paladin.cpp:183-252): the SCHOOL_ABSORB script leg for 31850.
// _absorbPct is the DBC effect-0 CalcValue, _healPct the effect-1
// CalcValue (Load, :166-167); the pool is unlimited (CalculateAmount
// sets -1, :177-181), so the amount is never consumed and the leg never
// reports depletion. A lethal hit with the 66235 heal off cooldown
// absorbs everything and casts the heal with BP0 =
// CountPctFromMaxHealth(lroundf(_healPct * defenseFactor)), defenseFactor
// = min(1, defenseSkill / (maxSkillForLevel + 140)); the heal then takes
// a 2-minute cooldown (AddCooldown, :235). Otherwise a hit landing below
// 35% max health absorbs CalculatePct(damageToReduce, _absorbPct), where
// damageToReduce is the full damage when already below 35%, else only
// the part below the line (:238-248). The remaining-damage argument is
// the C++ dmgInfo damage at this aura's turn in CalcAbsorbResist, and
// the if/else-if chain is preserved: a lethal hit with the heal on
// cooldown still takes the sub-35% partial absorb. C++ casts the heal
// non-triggered; Go rides the direct-cast path like every script cast,
// and the cast carries context.Background() because the absorb funnel
// has no request context. The npcbot branch is the same player math;
// Go has no npcbot model and Load() admits players only, so the
// creature absorb leg stays out. Returns (absorb, depleted, handled);
// handled is true whenever the DBC reads, so the generic pool leg never
// consumes this aura's amount — an unreadable DBC falls through to it,
// the scriptAbsorbHit convention.
func (s *session) ardentDefenderAbsorbHit(aura *activeAura, remainingDamage uint32) (uint32, bool, bool) {
	if s == nil || s.player == nil || aura == nil || aura.SpellID != 31850 || remainingDamage == 0 {
		return 0, false, false
	}
	if s.server == nil || s.server.Data == nil {
		return 0, false, false
	}
	spell, found, err := s.server.Data.Spell(31850)
	if err != nil || !found || len(spell.Effects) < 2 {
		return 0, false, false
	}
	absorbPct := spell.Effects[0].CalcValueForLevel(spell, uint32(aura.CasterLevel))
	healPct := spell.Effects[1].CalcValueForLevel(spell, uint32(aura.CasterLevel))
	if absorbPct <= 0 || healPct <= 0 {
		return 0, false, false
	}
	health := s.player.Health
	maxHealth := s.player.MaxHealth
	if maxHealth == 0 {
		return 0, false, false
	}
	damage := remainingDamage
	remainingHealth := int64(health) - int64(damage)
	allowedHealth := uint32(uint64(maxHealth) * 35 / 100)
	if remainingHealth <= 0 && s.playerCooldownRemainingMs(66235, 0) == 0 {
		defenseSkill := uint32(0)
		for _, skill := range s.player.Skills {
			if skill.Skill == 95 {
				defenseSkill = uint32(skill.Value)
				break
			}
		}
		reqDef := float64(uint32(s.player.Level)*5 + 140)
		defenseFactor := float64(defenseSkill) / reqDef
		if defenseFactor > 1 {
			defenseFactor = 1
		}
		healBP := uint32(uint64(maxHealth) * uint64(int64(math.Round(float64(healPct)*defenseFactor))) / 100)
		s.castSpellDirectWithBasePoint(context.Background(), 66235, s.playerGUID, healBP)
		s.replaceSpellCooldownEntry(spellCooldown{Spell: 66235, End: time.Now().Unix() + 120})
		return damage, false, true
	}
	if remainingHealth < int64(allowedHealth) {
		var damageToReduce uint32
		if health < allowedHealth {
			damageToReduce = damage
		} else {
			damageToReduce = uint32(int64(allowedHealth) - remainingHealth)
		}
		return uint32(uint64(damageToReduce) * uint64(absorbPct) / 100), false, true
	}
	return 0, false, true
}

// astralShiftAbsorbHit mirrors spell_sha_astral_shift::Absorb
// (spell_shaman.cpp:224-231): the SCHOOL_ABSORB script leg for 51474.
// The pool is unlimited (CalculateAmount sets -1), so the amount is
// never consumed; the per-hit absorb is CalculatePct(damage, absorbPct),
// the DBC effect-0 CalcValue read by scriptAbsorbPct. The gate is the
// C++ flag check: the target is fleeing or silenced, or stunned with a
// stun-mechanic aura (HasAuraWithMechanic, the auraSpellMechanicMask
// twin — the absorb path carries no ctx for targetHasAuraWithMechanic).
// A gate that does not trip absorbs nothing with the pool untouched —
// the (0, false, true) return, like the failed parry roll in
// scriptAbsorbHit — so the generic pool leg below must not run. Player
// path only: Astral Shift is a player talent, and the creature absorb
// loop has no per-spell gate site (ardent-defender precedent). DORMANT
// until Spell.dbc ships the 51474 record.
func (s *session) astralShiftAbsorbHit(aura *activeAura, remainingDamage uint32) (uint32, bool, bool) {
	if s == nil || s.player == nil || aura == nil || aura.SpellID != 51474 || remainingDamage == 0 {
		return 0, false, false
	}
	if s.player.UnitFlags&(unitFlagFleeing|unitFlagSilenced) == 0 {
		if s.player.UnitFlags&unitFlagStunned == 0 {
			return 0, false, true
		}
		hasStunMechanic := false
		for _, a := range s.activeAuras {
			if a != nil && !a.Stopped && s.auraSpellMechanicMask(a)&(uint32(1)<<mechanicStun) != 0 {
				hasStunMechanic = true
				break
			}
		}
		if !hasStunMechanic {
			return 0, false, true
		}
	}
	pct, ok := s.server.scriptAbsorbPct(aura.SpellID, aura.CasterLevel)
	if !ok {
		return 0, false, false
	}
	absorbThis := uint32(uint64(remainingDamage) * uint64(pct) / 100)
	if absorbThis > remainingDamage {
		absorbThis = remainingDamage
	}
	return absorbThis, false, true
}

// guardianSpiritAbsorbHit mirrors spell_pri_guardian_spirit::Absorb
// (spell_priest.cpp:560-573): the SCHOOL_ABSORB script leg for 47788.
// healPct is the EFFECT_1 CalcValue read at Load (no-level CalcValue,
// SpellInfo.cpp:402) — the DBC heal pct, not the applied amount. A
// non-lethal hit absorbs nothing and leaves the aura in place; a lethal
// hit absorbs the whole remaining damage, removes the aura (the depleted
// flag feeds the exhaustedSpells pass, which removes with
// AURA_REMOVE_BY_ENEMY_SPELL like C++), and the target casts Guardian
// Spirit Heal (48153) on itself with BP0 = healPct% of max health. The
// heal is NOT cast here: C++ removes the aura first (spell_priest.cpp
// :568, "we don't want 40% healing bonus"), and the Go direct-heal path
// folds SPELL_AURA_MOD_HEALING_RECEIVED while the aura still stands, so
// a self-cast Guardian Spirit would inflate its own heal. The leg
// returns the heal amount and the caller casts after the exhaustedSpells
// removal pass below. Returns (absorb, depleted, handled, heal). The
// pool is unlimited (CalculateAmount sets -1, see spells.go), so the leg
// never reports a partial consume and the generic pool leg must never run
// for this aura — handled is true whenever the DBC reads, the
// scriptAbsorbHit convention. The school gate reads the absorb effect's
// own MiscValue from the DBC (Unit.cpp:1881): the record's MiscValue
// belongs to EFFECT_0 (the +40% healing-received aura), not the absorb.
// The record's AuraType is EFFECT_0's too, so this leg is tried ahead of
// the 69 leg, not inside it. C++ casts the heal with TRIGGERED_FULL_MASK;
// Go rides the direct-cast path like every script cast, and the cast
// carries context.Background() because the absorb funnel has no request
// context (the ardentDefender precedent).
func (s *session) guardianSpiritAbsorbHit(aura *activeAura, remainingDamage uint32, schoolMask uint8) (uint32, bool, bool, uint32) {
	if s == nil || s.player == nil || aura == nil || aura.SpellID != 47788 || remainingDamage == 0 {
		return 0, false, false, 0
	}
	if s.server == nil || s.server.Data == nil {
		return 0, false, false, 0
	}
	spell, found, err := s.server.Data.Spell(47788)
	if err != nil || !found || len(spell.Effects) < 2 || spell.Effects[1].Aura != SpellAuraSchoolAbsorb {
		return 0, false, false, 0
	}
	if spell.Effects[1].MiscValue&int32(schoolMask) == 0 {
		return 0, false, false, 0
	}
	healPct := spell.Effects[1].CalcValue()
	if healPct <= 0 {
		return 0, false, false, 0
	}
	health := s.player.Health
	maxHealth := s.player.MaxHealth
	if maxHealth == 0 || health == 0 {
		return 0, false, true, 0
	}
	if remainingDamage < health {
		return 0, false, true, 0
	}
	healAmount := uint32(uint64(maxHealth) * uint64(healPct) / 100)
	return remainingDamage, true, true, healAmount
}

// guardianSpiritCreatureAbsorbHit is the creature-side twin of
// guardianSpiritAbsorbHit: same lethal-only absorb, same 48153 self-cast
// via fireScriptProcCast (which resolves creature casters), same
// AURA_REMOVE_BY_ENEMY_SPELL removal through the depleted flag. The heal
// is returned, not cast: fireScriptProcCast resolves the caster through
// findCreatureMotion (motionMu), which must never be taken under auraMu,
// and C++ removes the aura before casting. The caller casts after the
// exhaustedSpells removal pass, once auraMu is released. The heal cast
// carries context.Background() because the absorb funnel has no request
// context. Returns (absorb, depleted, handled, heal).
func (s *Server) guardianSpiritCreatureAbsorbHit(key creatureAuraKey, aura *activeAura, remainingDamage uint32, schoolMask uint8, creatureHealth, creatureMaxHealth uint32) (uint32, bool, bool, uint32) {
	if s == nil || key.GUID == 0 || aura == nil || aura.SpellID != 47788 || remainingDamage == 0 {
		return 0, false, false, 0
	}
	if s.Data == nil {
		return 0, false, false, 0
	}
	spell, found, err := s.Data.Spell(47788)
	if err != nil || !found || len(spell.Effects) < 2 || spell.Effects[1].Aura != SpellAuraSchoolAbsorb {
		return 0, false, false, 0
	}
	if spell.Effects[1].MiscValue&int32(schoolMask) == 0 {
		return 0, false, false, 0
	}
	healPct := spell.Effects[1].CalcValue()
	if healPct <= 0 {
		return 0, false, false, 0
	}
	if creatureMaxHealth == 0 || creatureHealth == 0 {
		return 0, false, true, 0
	}
	if remainingDamage < creatureHealth {
		return 0, false, true, 0
	}
	healAmount := uint32(uint64(creatureMaxHealth) * uint64(healPct) / 100)
	return remainingDamage, true, true, healAmount
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
	var guardianHeal uint32
	var absorbPending []absorbHookPending

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
			// A school-absorb aura with a registered OnEffectAbsorb
			// hook joins even with a zero pool: the C++ -1
			// (unlimited) pool reads 0 at the hook, and the handler
			// computes the absorb from the damage.
			if aura != nil && !aura.Stopped && (aura.Amount > 0 || (aura.AuraType == SpellAuraSchoolAbsorb && s.server.hasAuraAbsorbHook(aura.SpellID))) {
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
			if absorbThis, depleted, ok, heal := s.guardianSpiritAbsorbHit(aura, remainingDamage, schoolMask); ok {
				remainingDamage -= absorbThis
				absorbed += absorbThis
				if depleted {
					exhaustedSpells = append(exhaustedSpells, aura.SpellID)
					guardianHeal = heal
				}
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
					if absorbThis, depleted, ok := s.ardentDefenderAbsorbHit(aura, remainingDamage); ok {
						remainingDamage -= absorbThis
						absorbed += absorbThis
						if depleted {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
						continue
					}
					if absorbThis, depleted, ok := s.astralShiftAbsorbHit(aura, remainingDamage); ok {
						remainingDamage -= absorbThis
						absorbed += absorbThis
						if depleted {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
						continue
					}
					if absorbThis, depleted, ok := s.server.scriptAbsorbHit(aura, remainingDamage, float64(s.player.ParryPercentage), s.player.Health, s.player.MaxHealth); ok {
						remainingDamage -= absorbThis
						absorbed += absorbThis
						if depleted {
							exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						}
						continue
					}
					// OnEffectAbsorb (Unit.cpp:1897-1903): a
					// hook-registered aura defers to the post-lock
					// pass instead of applying the pool here — Lua
					// never runs under castMu (see the
					// AuraEventOnEffectAbsorb contract). The pool
					// arg is the live Amount; damage is the
					// remaining damage at this aura's turn.
					if s.server.hasAuraAbsorbHook(aura.SpellID) {
						absorbPending = append(absorbPending, absorbHookPending{aura: aura, pool: aura.Amount, damage: remainingDamage})
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

	// OnEffectAbsorb hook pass (Unit.cpp:1897-1912): the deferred Lua
	// fire for hook-registered school-absorb auras, after castMu is
	// released so a handler may read the aura object and cast without
	// deadlocking on the absorb lock. Each aura fires with the at-turn
	// damage recorded in the loop; the returned amount is clamped to
	// the running remaining damage and, for a finite pool, to the live
	// pool at apply time (a concurrent damage event may have consumed
	// part of it between the loop and this pass — C++ has no such
	// race, this clamp is the Go-only guard). A vetoed aura absorbs
	// nothing; an unlimited-pool aura never consumes or exhausts.
	for _, pend := range absorbPending {
		aura := pend.aura
		if aura == nil || aura.Stopped || remainingDamage == 0 {
			continue
		}
		auraObj := (&luaAuraRef{server: s.server, spellID: aura.SpellID, player: s, mapID: mapID, instanceID: instanceID}).luaAuraObject()
		amount, prevented := s.server.fireAuraAbsorb(context.Background(), aura.SpellID, auraObj, s.luaPlayer(), pend.damage, pend.pool)
		if prevented || amount == 0 {
			continue
		}
		if amount > remainingDamage {
			amount = remainingDamage
		}
		if absorbHookPoolUnlimited(aura.SpellID) {
			remainingDamage -= amount
			absorbed += amount
			continue
		}
		if amount > aura.Amount {
			amount = aura.Amount
		}
		if amount == 0 {
			continue
		}
		aura.Amount -= amount
		remainingDamage -= amount
		absorbed += amount
		if aura.Amount == 0 {
			exhaustedSpells = append(exhaustedSpells, aura.SpellID)
		}
	}

	// Remove exhausted shields outside the lock; a shield that hits zero
	// absorbed amount is destroyed by an enemy spell, not a default remove
	// (Unit.cpp:1895, 1965).
	for _, id := range exhaustedSpells {
		s.removeAuraWithMode(id, auraRemoveByEnemySpell)
	}

	// Guardian Spirit's heal fires here, after the aura is gone (the C++
	// Remove-then-cast order, spell_priest.cpp:568): the direct-heal path
	// folds SPELL_AURA_MOD_HEALING_RECEIVED while the aura stands, so
	// casting inside the loop would inflate a self-cast heal with the
	// aura's own +40%.
	if guardianHeal > 0 {
		s.castSpellDirectWithBasePoint(context.Background(), 48153, s.playerGUID, guardianHeal)
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
	var guardianHeal uint32
	var absorbPending []absorbHookPending

	// The mana-shield leg drains the creature's mana pool (Powers[0], filled
	// from the template mana row at spawn); resolved ahead of the aura lock
	// so the leg below never takes motionMu under auraMu.
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)

	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok && auras != nil {
			var shieldList []*activeAura
			for _, aura := range auras {
				// A school-absorb aura with a registered OnEffectAbsorb
				// hook joins even with a zero pool (player-leg twin).
				if aura != nil && !aura.Stopped && (aura.Amount > 0 || (aura.AuraType == SpellAuraSchoolAbsorb && s.hasAuraAbsorbHook(aura.SpellID))) {
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
				var creatureHealth, creatureMaxHealth uint32
				if motion != nil {
					creatureHealth, creatureMaxHealth = motion.Health, motion.MaxHealth
				}
				if absorbThis, depleted, ok, heal := s.guardianSpiritCreatureAbsorbHit(key, aura, remainingDamage, schoolMask, creatureHealth, creatureMaxHealth); ok {
					remainingDamage -= absorbThis
					absorbed += absorbThis
					if depleted {
						exhaustedSpells = append(exhaustedSpells, aura.SpellID)
						guardianHeal = heal
					}
					continue
				}
				if aura.AuraType == SpellAuraSchoolAbsorb {
					// C++ gates on the absorb effect's MiscValue
					// (Unit.cpp:1881), not the aura spell's school — see
					// the player leg above. 256 is not an absorb type.
					if aura.MiscValue&int32(schoolMask) != 0 {
						if absorbThis, depleted, ok := s.scriptAbsorbHit(aura, remainingDamage, -1, creatureHealth, creatureMaxHealth); ok {
							remainingDamage -= absorbThis
							absorbed += absorbThis
							if depleted {
								exhaustedSpells = append(exhaustedSpells, aura.SpellID)
							}
							continue
						}
						// OnEffectAbsorb (Unit.cpp:1897-1903): the
						// player-leg twin — defer to the post-lock
						// pass, Lua never runs under auraMu.
						if s.hasAuraAbsorbHook(aura.SpellID) {
							absorbPending = append(absorbPending, absorbHookPending{aura: aura, pool: aura.Amount, damage: remainingDamage})
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

	// OnEffectAbsorb hook pass (Unit.cpp:1897-1912): the player-leg
	// twin, after auraMu is released. The holder is the creature
	// motion object; a despawned creature (nil motion) cannot fire.
	for _, pend := range absorbPending {
		aura := pend.aura
		if aura == nil || aura.Stopped || remainingDamage == 0 {
			continue
		}
		target := s.luaMotionCreature(motion)
		if target == nil {
			continue
		}
		auraObj := (&luaAuraRef{server: s, spellID: aura.SpellID, creatureKey: key, mapID: key.Map, instanceID: key.InstanceID}).luaAuraObject()
		amount, prevented := s.fireAuraAbsorb(context.Background(), aura.SpellID, auraObj, target, pend.damage, pend.pool)
		if prevented || amount == 0 {
			continue
		}
		if amount > remainingDamage {
			amount = remainingDamage
		}
		if absorbHookPoolUnlimited(aura.SpellID) {
			remainingDamage -= amount
			absorbed += amount
			continue
		}
		if amount > aura.Amount {
			amount = aura.Amount
		}
		if amount == 0 {
			continue
		}
		aura.Amount -= amount
		remainingDamage -= amount
		absorbed += amount
		if aura.Amount == 0 {
			exhaustedSpells = append(exhaustedSpells, aura.SpellID)
		}
	}

	// Unit.cpp:1965: an exhausted shield is removed with
	// AURA_REMOVE_BY_ENEMY_SPELL, not the default mode.
	for _, id := range exhaustedSpells {
		s.removeCreatureAuraWithMode(key, id, auraRemoveByEnemySpell)
	}

	// Guardian Spirit's heal fires here, after the aura is gone and auraMu
	// is released (the C++ Remove-then-cast order, spell_priest.cpp:568):
	// fireScriptProcCast resolves the caster through findCreatureMotion
	// (motionMu), which must never be taken under auraMu.
	if guardianHeal > 0 {
		s.fireScriptProcCast(context.Background(), key.Map, key.InstanceID, scriptProcCast{spellID: 48153, casterGUID: key.GUID, targetGUID: key.GUID, bp0: guardianHeal, bp0Set: true})
	}

	return absorbed, remainingDamage
}
