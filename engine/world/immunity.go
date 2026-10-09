package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	spellAuraSchoolImmunity            uint32 = 39         // SPELL_AURA_SCHOOL_IMMUNITY (SpellAuraDefines.h:119) — was mislabeled as 2 (SPELL_AURA_MOD_POSSESS); corrected 2026-10-03 during the CheckCasterAuras audit
	spellAuraDamageImmunity            uint32 = 4          // SPELL_AURA_DAMAGE_IMMUNITY (SpellAuraDefines.h:34)
	spellAuraReflectSpells             uint32 = 63         // SPELL_AURA_REFLECT_SPELLS (SpellAuraDefines.h:93)
	spellAuraReflectSpellsSchool       uint32 = 64         // SPELL_AURA_REFLECT_SPELLS_SCHOOL (SpellAuraDefines.h:94)
	spellAuraModMaxAffectedTargets     uint32 = 277        // SPELL_AURA_MOD_MAX_AFFECTED_TARGETS (SpellAuraDefines.h:357)
	spellAuraModIgnoreShapeshift       uint32 = 275        // SPELL_AURA_MOD_IGNORE_SHAPESHIFT (SpellAuraDefines.h:355)
	spellAttr3IgnoreHitResult          uint32 = 0x00040000 // SPELL_ATTR3_IGNORE_HIT_RESULT (SharedDefines.h:541) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr1UnaffectedBySchoolImmune uint32 = 0x00010000 // SPELL_ATTR1_UNAFFECTED_BY_SCHOOL_IMMUNE (SharedDefines.h:465) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	mechanicBanish                     uint32 = 18         // MECHANIC_BANISH (SharedDefines.h:1375)
	mechanicInvulnerability            uint32 = 25         // MECHANIC_INVULNERABILITY (SharedDefines.h:1382)
	mechanicImmuneShield               uint32 = 29         // MECHANIC_IMMUNE_SHIELD (SharedDefines.h:1386)
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
// Mirrors TrinityCore Unit::IsImmunedToSpell (Unit.cpp:7852-7922), the
// whole-spell immunity gate C++ consults in Unit::AddAura (Unit.cpp:12282)
// before applying any aura of the spell. The pre-existing arms below
// (Cyclone/Banish, Divine Shield, Blessing of Protection, Anti-Magic Shell,
// Cloak of Shadows, school/damage aura scan) are the Go engine's live
// approximations of the C++ state/mechanic immunity model; the
// UNAFFECTED_BY_INVULNERABILITY arm keeps its C++ position ahead of all of
// them (Unit.cpp:7861-7862 — even Banish's mechanic list sits below it), and
// the structured arms (IMMUNITY_DISPEL / IMMUNITY_MECHANIC / the
// all-effects fold / the school fold) run after them in C++ order.
// IMMUNITY_ID stays a documented gap: its only player-relevant writer is the
// spell_linked_spell negative row at aura apply (SpellAuras.cpp:1337-1342),
// and Go has no ApplySpellImmune model (fireSpellLinkedTriggers bridges only
// the aura-removal arm).
func (s *session) isImmuneToSpell(spell wotlk.Spell, caster *session) bool {
	if s == nil || s.player == nil {
		return false
	}

	s.castMu.Lock()
	defer s.castMu.Unlock()

	// Unit.cpp:7861-7862 — invulnerability-piercing spells are never immune,
	// ahead of every state/mechanic list.
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}

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

	// Unit.cpp:7864-7868 — IMMUNITY_DISPEL: the list holds dispel types
	// written by SPELL_AURA_DISPEL_IMMUNITY (76) effects
	// (SpellInfo::_LoadImmunityInfo, SpellInfo.cpp:2786-2788 — misc is the
	// dispel type), derived here live from the active auras.
	if spell.DispelType != 0 && s.server != nil && s.server.Data != nil {
		dispelImmune := false
		for _, aura := range s.activeAuras {
			if aura == nil {
				continue
			}
			auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
			if err != nil || !found {
				continue
			}
			for _, eff := range auraSpell.Effects {
				if eff.Effect == spellEffectApplyAura && eff.Aura == spellAuraDispelImmunity && uint32(eff.MiscValue) == spell.DispelType {
					dispelImmune = true
					break
				}
			}
			if dispelImmune {
				break
			}
		}
		if dispelImmune {
			return true
		}
	}

	// Unit.cpp:7871-7876 — IMMUNITY_MECHANIC (spell-level mechanic): writers
	// are the SPELL_AURA_MECHANIC_IMMUNITY (77) effects (the
	// _LoadImmunityInfo carve-outs SpellInfo.cpp:2750-2780, mirrored by
	// spellAllowedMechanicMask) and SPELL_AURA_MECHANIC_IMMUNITY_MASK (147)
	// misc bits.
	if spell.Mechanic != 0 && s.server != nil && s.server.Data != nil {
		mechanicImmune := false
		for _, aura := range s.activeAuras {
			if aura == nil {
				continue
			}
			switch aura.AuraType {
			case spellAuraMechanicImmunity:
				auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
				if err != nil || !found {
					continue
				}
				if spellAllowedMechanicMask(auraSpell)&(1<<spell.Mechanic) != 0 {
					mechanicImmune = true
				}
			case spellAuraMechanicImmunityMask:
				if uint32(aura.MiscValue)&(1<<spell.Mechanic) != 0 {
					mechanicImmune = true
				}
			}
			if mechanicImmune {
				break
			}
		}
		if mechanicImmune {
			return true
		}
	}

	// Unit.cpp:7884-7898 — immune to the whole spell when immune to all of
	// its effects, via the per-effect model. C++ counts a zero-effect spell
	// as immune here, but IsImmunedToSpell only ever runs on aura spells in
	// C++ (Unit::AddAura); Go consults this gate at hit resolution for every
	// spell, so an effectless spell reports no immunity (the same gate the
	// IMMUNE2 strip uses at AddUnitTarget, Spell.cpp:2105).
	snap := s.immuneAuraSnapshotLocked()
	anyEffect := false
	immuneToAllEffects := true
	for i := range spell.Effects {
		if spell.Effects[i].Effect == 0 {
			continue
		}
		anyEffect = true
		if !immunedToSpellEffectEval(spell, i, s, caster, snap) {
			immuneToAllEffects = false
			break
		}
	}
	if anyEffect && immuneToAllEffects {
		return true
	}

	// Unit.cpp:7900-7922 — school fold over the IMMUNITY_SCHOOL list
	// (aura-39 writers, SpellInfo.cpp:2785): an entry counts only when its
	// mask overlaps the spell's, the positivity/friendly gates pass, and the
	// spell cannot pierce the immune aura.
	if schoolMask := spell.SchoolMask; schoolMask != 0 && s.server != nil && s.server.Data != nil {
		var schoolImmunityMask uint32
		for _, a := range snap {
			if a.auraType != spellAuraSchoolImmunity {
				continue
			}
			mask := uint32(a.misc)
			if mask&schoolMask == 0 {
				continue
			}
			immuneSpell, found, err := s.server.Data.Spell(a.spellID)
			if err != nil {
				continue
			}
			// (immuneSpellInfo && !IsPositive()) || !IsPositive() || !caster || !IsFriendlyTo(caster)
			if (found && !spellIsPositive(immuneSpell)) || !spellIsPositive(spell) || caster == nil || !s.isFriendlyToPlayer(caster) {
				// C++ CanPierceImmuneAura(nullptr) is false — an
				// unresolvable immune aura cannot be pierced.
				if !found || !canSpellPierceImmuneAura(spell, immuneSpell) {
					schoolImmunityMask |= mask
				}
			}
		}
		if schoolImmunityMask&schoolMask == schoolMask {
			return true
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

// immuneAura is one active aura flattened for the immunity model.
type immuneAura struct {
	auraType uint32
	misc     int32
	spellID  uint32
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
	if s == nil || s.player == nil {
		return false
	}
	s.castMu.Lock()
	snap := s.immuneAuraSnapshotLocked()
	s.castMu.Unlock()
	return immunedToSpellEffectEval(spell, effIndex, s, caster, snap)
}

// immuneAuraSnapshotLocked collects the immunity-relevant active auras.
// Caller holds castMu.
func (s *session) immuneAuraSnapshotLocked() []immuneAura {
	var auras []immuneAura
	for _, aura := range s.activeAuras {
		if aura == nil {
			continue
		}
		switch aura.AuraType {
		case spellAuraEffectImmunity, spellAuraMechanicImmunity,
			spellAuraMechanicImmunityMask, spellAuraStateImmunity,
			spellAuraModImmuneAuraApplySchool, spellAuraSchoolImmunity:
			auras = append(auras, immuneAura{aura.AuraType, aura.MiscValue, aura.SpellID})
		}
	}
	return auras
}

// immunedToSpellEffectEval is the lock-free evaluation half of
// isImmunedToSpellEffect: target is the immune target, caster the spell
// caster, auras a snapshot from immuneAuraSnapshotLocked.
func immunedToSpellEffectEval(spell wotlk.Spell, effIndex int, target, caster *session, auras []immuneAura) bool {
	if target == nil || target.player == nil {
		return false
	}
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
			if target.server == nil || target.server.Data == nil {
				continue
			}
			auraSpell, found, err := target.server.Data.Spell(a.spellID)
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
				if caster == nil || !target.isFriendlyToPlayer(caster) || !spell.IsPositiveEffect(effIndex) {
					return true
				}
			}
		}
	}
	return false
}

// canSpellPierceImmuneAura mirrors SpellInfo::CanPierceImmuneAura
// (SpellInfo.cpp:1336-1361): whether the incoming spell pierces the aura
// that grants the target its immunity. The DISPEL_AURAS_ON_IMMUNITY arm
// reuses spellCancelsAuraEffect per aura effect as the Go stand-in for
// CanSpellProvideImmunityAgainstAura (the C++ form also folds the school
// mask and the all-effects rule — a residual delta).
func canSpellPierceImmuneAura(spell, immuneSpell wotlk.Spell) bool {
	// Aura can't be pierced.
	if immuneSpell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	// These spells pierce all available spells (Resurrection Sickness for example).
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return true
	}
	// These spells (Cyclone for example) can pierce all...
	if spell.AttributesEx&spellAttr1UnaffectedBySchoolImmune != 0 ||
		spell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune != 0 {
		// ...but not these (Divine shield, Ice block, Cyclone and Banish for example).
		if immuneSpell.Mechanic != mechanicImmuneShield &&
			immuneSpell.Mechanic != mechanicInvulnerability &&
			immuneSpell.Mechanic != mechanicBanish {
			return true
		}
	}
	// Dispels other auras on immunity: the spell pierces when it would
	// cancel the immune aura.
	if spell.AttributesEx&spellAttr1DispelAurasOnImmunity != 0 {
		for i := range immuneSpell.Effects {
			if spellCancelsAuraEffect(spell, immuneSpell, i) {
				return true
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
