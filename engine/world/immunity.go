package world

import (
	"context"
	"math/rand/v2"

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
	mechanicDisoriented                uint32 = 2          // MECHANIC_DISORIENTED (SharedDefines.h:1359)
	mechanicSilence                    uint32 = 9          // MECHANIC_SILENCE (SharedDefines.h:1366)
	mechanicStun                       uint32 = 12         // MECHANIC_STUN (SharedDefines.h:1369)
	mechanicFreeze                     uint32 = 13         // MECHANIC_FREEZE (SharedDefines.h:1370)
	mechanicKnockout                   uint32 = 14         // MECHANIC_KNOCKOUT (SharedDefines.h:1371)
	mechanicInterrupt                  uint32 = 26         // MECHANIC_INTERRUPT (SharedDefines.h:1383)
	spellAuraModDecreaseSpeed          uint32 = 33         // SPELL_AURA_MOD_DECREASE_SPEED (SpellAuraDefines.h:113)
	spellAuraModDisarm                 uint32 = 67         // SPELL_AURA_MOD_DISARM (SpellAuraDefines.h:147)
	// Creature-side immunity writers (Creature::LoadTemplateImmunities,
	// Creature.cpp:2279-2313; flags_extra arms, Creature.cpp:634-638 /
	// 1184-1187; Totem::IsImmunedToSpellEffect, Totem.cpp:183-204).
	creatureTypeMechanical uint32 = 9     // CREATURE_TYPE_MECHANICAL (UnitMethods.h:1115)
	sentryStoneclawSpellID uint32 = 55277 // SENTRY_STONECLAW_SPELLID (Totem.h:35)
	sentryBindSightSpellID uint32 = 6277  // SENTRY_BIND_SIGHT_SPELLID (Totem.h:36)
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
// Mirrors TrinityCore Unit::IsImmunedToDamage(SpellSchoolMask) (Unit.cpp:7800-7816):
// full-coverage OR over the live SPELL_AURA_SCHOOL_IMMUNITY (39) misc values
// (m_spellImmune[IMMUNITY_SCHOOL]), then the SPELL_AURA_DAMAGE_IMMUNITY (4)
// misc OR (m_spellImmune[IMMUNITY_DAMAGE]). The misc value — not the granting
// spell's school — is the immune mask (ApplyAllSpellImmunitiesTo,
// SpellInfo.cpp:2860-2910). The hardcoded spell-ID arms below predate the
// live-aura fold and stay as belt-and-braces for auras whose rows may not
// have been recorded; they agree with the fold for non-piercing spells.
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

	// 3. Live-aura folds, C++-exact: misc-based full coverage, no pierce
	// filter (the mask overload has no spell to pierce with).
	// Unit.cpp:7806-7813.
	if s.immuneSchoolMaskLocked(wotlk.Spell{}, schoolMask)&schoolMask == schoolMask && schoolMask != 0 {
		return true
	}
	if s.immuneDamageMaskLocked()&schoolMask == schoolMask && schoolMask != 0 {
		return true
	}

	return false
}

// immuneSchoolMaskLocked is the Unit.cpp:7831-7841 fold over live aura-39
// rows: each entry whose misc overlaps schoolMask contributes its misc,
// unless the incoming spell pierces that entry's granting aura
// (SpellInfo::CanPierceImmuneAura per entry). Caller holds castMu.
func (s *session) immuneSchoolMaskLocked(spell wotlk.Spell, schoolMask uint32) uint32 {
	var mask uint32
	for _, aura := range s.activeAuras {
		if aura == nil || aura.AuraType != spellAuraSchoolImmunity {
			continue
		}
		misc := uint32(aura.MiscValue)
		if misc&schoolMask == 0 {
			continue
		}
		if s.server != nil && s.server.Data != nil {
			if immuneSpell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found && canSpellPierceImmuneAura(spell, immuneSpell) {
				continue
			}
		}
		mask |= misc
	}
	return mask
}

// immuneDamageMaskLocked is the Unit.cpp:7844-7847 fold: the OR of the live
// aura-4 (SPELL_AURA_DAMAGE_IMMUNITY) misc school masks. Caller holds castMu.
func (s *session) immuneDamageMaskLocked() uint32 {
	var mask uint32
	for _, aura := range s.activeAuras {
		if aura == nil || aura.AuraType != spellAuraDamageImmunity {
			continue
		}
		mask |= uint32(aura.MiscValue)
	}
	return mask
}

// isImmuneToDamageSpell mirrors Unit::IsImmunedToDamage(SpellInfo const*)
// (Unit.cpp:7818-7860): the attribute gates run before any aura fold, and
// the school-immunity fold applies the per-entry CanPierceImmuneAura filter.
// A spell with zero school mask never immunizes via this path
// (Unit.cpp:7830).
func (s *session) isImmuneToDamageSpell(spell wotlk.Spell, schoolMask uint32) bool {
	if s == nil || s.player == nil || schoolMask == 0 {
		return false
	}
	// Unit.cpp:7824-7825 — e.g. 40175.
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 &&
		spell.AttributesEx3&spellAttr3IgnoreHitResult != 0 {
		return false
	}
	// Unit.cpp:7827-7828.
	if spell.AttributesEx&spellAttr1UnaffectedBySchoolImmune != 0 ||
		spell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune != 0 {
		return false
	}

	s.castMu.Lock()
	defer s.castMu.Unlock()

	// Hardcoded total/physical arms agree with the folds below for
	// non-piercing spells (Divine Shield etc. grant all-school misc rows);
	// the attribute gates above already handled the piercing case.
	totalImmunitySpells := []uint32{642, 45438, 33786, 710, 18647}
	for _, id := range totalImmunitySpells {
		if _, ok := s.auras[id]; ok {
			return true
		}
		if _, ok := s.activeAuras[id]; ok {
			return true
		}
	}
	if schoolMask&1 != 0 {
		for _, id := range []uint32{1022, 5599, 10278} {
			if _, ok := s.auras[id]; ok {
				return true
			}
			if _, ok := s.activeAuras[id]; ok {
				return true
			}
		}
	}

	// Unit.cpp:7831-7841 — full coverage required.
	if s.immuneSchoolMaskLocked(spell, schoolMask)&schoolMask == schoolMask {
		return true
	}
	// Unit.cpp:7844-7847.
	if s.immuneDamageMaskLocked()&schoolMask == schoolMask {
		return true
	}
	return false
}

// mechanicImmuneMaskGranted mirrors the per-effect MechanicImmuneMask rows
// (SpellInfo::_LoadImmunityInfo, SpellInfo.cpp:2592-2818) exactly as
// SpellInfo::ApplyAllSpellImmunitiesTo writes them into
// m_spellImmune[IMMUNITY_MECHANIC] (SpellInfo.cpp:2883-2891): the union of
// mechanicImmuneMaskForEffect over every effect of the spell. The
// SPELL_ATTR5_USABLE_WHILE_* bits are deliberately EXCLUDED — C++ ORs those
// into _allowedMechanicMask only (SpellInfo.cpp:2822-2856), never into the
// per-effect ImmunityInfo rows the live immunity evals consult. The live
// evals below must use this, not spellAllowedMechanicMask (whose ATTR5 bits
// would wrongly grant mechanic immunity — e.g. a USABLE_WHILE_STUNNED spell
// reporting stun immunity); spellAllowedMechanicMask stays the correct mask
// for the CheckCast mechanicCheck path (Spell.cpp:6309), the one C++
// consumer of GetAllowedMechanicMask.
func mechanicImmuneMaskGranted(spell wotlk.Spell) uint32 {
	var mask uint32
	for _, eff := range spell.Effects {
		mask |= mechanicImmuneMaskForEffect(spell, eff)
	}
	return mask
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
	// _LoadImmunityInfo carve-outs SpellInfo.cpp:2750-2780) and the
	// SPELL_AURA_MECHANIC_IMMUNITY_MASK (147) miscVal table
	// (SpellInfo.cpp:2594-2724, mechanicImmuneMask147) — the raw misc bits
	// are NOT a mechanic mask in C++, and the ATTR5 usable-while bits never
	// enter these rows (they feed _allowedMechanicMask only), so the eval
	// uses mechanicImmuneMaskGranted, not spellAllowedMechanicMask.
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
				if mechanicImmuneMaskGranted(auraSpell)&(1<<spell.Mechanic) != 0 {
					mechanicImmune = true
				}
			case spellAuraMechanicImmunityMask:
				auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
				if err != nil || !found {
					continue
				}
				if mechanicImmuneMask147(auraSpell)&(1<<spell.Mechanic) != 0 {
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

// spellReflectOffered mirrors the canReflect argument Spell::AddUnitTarget
// passes to WorldObject::SpellHitResult (Spell.cpp:2152):
// m_canReflect && !(IsPositive() && m_caster->IsFriendlyTo(target)).
// SpellInfo::IsPositive (SpellInfo.cpp:1205) is !SPELL_ATTR0_CU_NEGATIVE,
// i.e. none of the three load-computed CU_NEGATIVE_EFF bits is set. Without
// this gate a positive harmful spell (e.g. an AoE heal/damage hybrid) cast
// on a friendly target could be reflected back at the caster.
// checkSpellReflection keeps the C++ roll itself (Object.cpp:2641-2648).
func spellReflectOffered(spell wotlk.Spell, caster *session, targetGUID uint64, targetSess *session) bool {
	if !spellCanBeReflected(spell) {
		return false
	}
	if spell.AttributesCu&spellAttr0CuNegativeMask == 0 && caster.isFriendlyToTarget(targetGUID, targetSess) {
		return false
	}
	return true
}

// checkSpellReflection rolls the target's reflect chance for an incoming
// spell whose reflect was offered. The offer itself — m_canReflect
// (Spell.cpp:622) plus the !(IsPositive() && m_caster->IsFriendlyTo(target))
// carve-out (Spell.cpp:2152) — is decided by the caller via
// spellReflectOffered; this only performs the WorldObject::SpellHitResult
// roll (Object.cpp:2641-2648):
//
//	reflectchance = GetTotalAuraModifier(SPELL_AURA_REFLECT_SPELLS)
//	              + GetTotalAuraModifierByMiscMask(SPELL_AURA_REFLECT_SPELLS_SCHOOL, schoolMask)
//	if (reflectchance > 0 && roll_chance_i(reflectchance)) return SPELL_MISS_REFLECT
//
// The flattened activeAura entries carry the aura effect's own amount, so
// the sums match C++ per-effect GetAmount accumulation. On success the
// reflect aura is consumed (Go consumes at roll time; C++ spends the aura's
// charge through the proc system at arrival).
func (s *session) checkSpellReflection(spell wotlk.Spell) bool {
	if s == nil || s.player == nil {
		return false
	}

	// Pre-existing Go gate: channeled spells never reflect. C++ offers and
	// rolls reflect for channeled spells too (no exclusion at Spell.cpp:622
	// or in SpellHitResult) — documented deviation, kept deliberately.
	if isChanneledSpell(spell) {
		return false
	}

	if !spellCanBeReflected(spell) {
		return false
	}

	s.castMu.Lock()
	var chance int32
	var reflectSpellID uint32
	seen23920 := false
	for _, aura := range s.activeAuras {
		if aura == nil {
			continue
		}
		switch aura.AuraType {
		case spellAuraReflectSpells:
			chance += int32(aura.Amount)
			if reflectSpellID == 0 {
				reflectSpellID = aura.SpellID
			}
			if aura.SpellID == 23920 {
				seen23920 = true
			}
		case spellAuraReflectSpellsSchool:
			if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
				chance += int32(aura.Amount)
				if reflectSpellID == 0 {
					reflectSpellID = aura.SpellID
				}
			}
		}
	}
	s.castMu.Unlock()

	// Warrior Spell Reflection (23920) tracked in the presence set rather
	// than the per-effect map: its DBC reflect effect carries amount 100.
	if _, ok := s.auras[23920]; ok && !seen23920 {
		chance += 100
		if reflectSpellID == 0 {
			reflectSpellID = 23920
		}
	}

	// roll_chance_i(reflectchance): urand(0, 99) < chance.
	if chance <= 0 || rand.Float64()*100 >= float64(chance) {
		return false
	}

	if reflectSpellID != 0 {
		// Reflection consumes the buff
		s.removeAura(reflectSpellID)
	}
	return true
}

// creatureReflectOffered is the creature-target form of spellReflectOffered:
// the same Spell.cpp:622/2152 offer gate, with the positive-and-friendly
// carve-out resolved through the creature's faction
// (creatureFriendlyToCaster) since there is no target session.
func (s *session) creatureReflectOffered(spell wotlk.Spell, targetFaction uint32) bool {
	if !spellCanBeReflected(spell) {
		return false
	}
	if spell.AttributesCu&spellAttr0CuNegativeMask == 0 && s.creatureFriendlyToCaster(s, targetFaction) {
		return false
	}
	return true
}

// creatureCheckSpellReflection is the creature-target analog of
// checkSpellReflection: WorldObject::SpellHitResult's reflect arm
// (Object.cpp:2641-2648) runs on any Unit, so a creature victim carrying
// SPELL_AURA_REFLECT_SPELLS (63) / SPELL_AURA_REFLECT_SPELLS_SCHOOL (64)
// auras rolls the same summed chance. The activeCreatureAuras entries carry
// the aura effect's own amount, matching the player-side flattened model.
// On success the reflect aura is consumed. The player-side channeled
// exclusion applies here too (documented Go-wide deviation: C++ offers
// reflect for channeled spells as well). The 23920 presence-set fallback
// is player-session state and has no creature analog.
func (srv *Server) creatureCheckSpellReflection(key creatureAuraKey, spell wotlk.Spell) bool {
	if srv == nil || key.GUID == 0 {
		return false
	}
	if isChanneledSpell(spell) {
		return false
	}
	if !spellCanBeReflected(spell) {
		return false
	}
	srv.auraMu.Lock()
	var chance int32
	var reflectSpellID uint32
	for _, aura := range srv.activeCreatureAuras[key] {
		if aura == nil {
			continue
		}
		switch aura.AuraType {
		case spellAuraReflectSpells:
			chance += int32(aura.Amount)
			if reflectSpellID == 0 {
				reflectSpellID = aura.SpellID
			}
		case spellAuraReflectSpellsSchool:
			if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
				chance += int32(aura.Amount)
				if reflectSpellID == 0 {
					reflectSpellID = aura.SpellID
				}
			}
		}
	}
	srv.auraMu.Unlock()

	// roll_chance_i(reflectchance): urand(0, 99) < chance.
	if chance <= 0 || rand.Float64()*100 >= float64(chance) {
		return false
	}

	if reflectSpellID != 0 {
		srv.removeCreatureAura(key, reflectSpellID)
	}
	return true
}

// spellHasOnlyDamageEffects mirrors SpellInfo::HasOnlyDamageEffects
// (SpellInfo.cpp:906-928): every present effect is a damage effect
// (weapon/normalized/percent/school/environmental damage or health leech).
// WorldObject::SpellHitResult (Object.cpp:2624-2627) consults damage
// immunity at hit resolution only for such spells; for other spells the
// GO packet must show a hit and the immunity zeroes the damage instead
// (the isImmuneToDamageSpell arm at the damage path).
func spellHasOnlyDamageEffects(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		switch eff.Effect {
		case spellEffectWeaponDamage, spellEffectWeaponDamageNoschool,
			spellEffectNormalizedWeaponDmg, spellEffectWeaponPercentDamage,
			spellEffectSchoolDamage, spellEffectEnvironmentalDMG,
			spellEffectHealthLeech:
			continue
		default:
			return false
		}
	}
	return true
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
//     mechanicImmuneMaskGranted (the _LoadImmunityInfo carve-outs + misc
//     mapping, :2877 — the ATTR5 _allowedMechanicMask bits are excluded,
//     they never enter the per-effect rows) and
//     SPELL_AURA_MECHANIC_IMMUNITY_MASK (147) via the
//     miscVal table (mechanicImmuneMask147 / mechanicMask147Grants,
//     :2594-2742)
//   - IMMUNITY_STATE <- SPELL_AURA_STATE_IMMUNITY (38) misc (:2913) plus
//     the 147 AuraTypeImmune inserts (mechanicMask147Grants)
//
// The SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (267) arm is a live
// aura-list check in C++ (no m_spellImmune entry; handler is
// HandleNoImmediateEffect, SpellAuraEffects.cpp:332), mirrored here.
// The creature-target chain (creatureImmuneToSpellEffect) mirrors the same
// Unit arms from the Creature/Totem template writers, including the 267
// live-aura arm. IMMUNITY_ID (spell_linked_spell negative rows,
// ApplySpellImmune unbridged) and IMMUNITY_DISPEL (never consulted by
// IsImmunedToSpellEffect) have no writer model and stay documented gaps.
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
		case spellAuraMechanicImmunity: // IMMUNITY_MECHANIC (SpellInfo.cpp:2877) — the granting spell's per-effect rows only, no ATTR5 bits
			if eff.Mechanic == 0 {
				continue
			}
			if target.server == nil || target.server.Data == nil {
				continue
			}
			auraSpell, found, err := target.server.Data.Spell(a.spellID)
			if err != nil || !found {
				continue
			}
			if mechanicImmuneMaskGranted(auraSpell)&(1<<eff.Mechanic) != 0 {
				return true
			}
		case spellAuraMechanicImmunityMask: // IMMUNITY_MECHANIC/EFFECT via the 147 miscVal table (SpellInfo.cpp:2594-2724)
			if target.server == nil || target.server.Data == nil {
				continue
			}
			auraSpell, found, err := target.server.Data.Spell(a.spellID)
			if err != nil || !found {
				continue
			}
			if eff.Mechanic != 0 && mechanicImmuneMask147(auraSpell)&(1<<eff.Mechanic) != 0 {
				return true
			}
			if _, spellEffects := mechanicMask147SpellGrants(auraSpell); uint32InSlice(eff.Effect, spellEffects) {
				return true
			}
		}
	}
	if spell.AttributesEx3&spellAttr3IgnoreHitResult == 0 && eff.Aura != 0 {
		for _, a := range auras {
			if a.auraType == spellAuraStateImmunity && a.misc == int32(eff.Aura) { // IMMUNITY_STATE (SpellInfo.cpp:2913)
				return true
			}
			// 147 AuraTypeImmune inserts (SpellInfo.cpp:2594-2742) also land
			// in the IMMUNITY_STATE list via ApplyAllSpellImmunitiesTo.
			if a.auraType == spellAuraMechanicImmunityMask {
				if target.server == nil || target.server.Data == nil {
					continue
				}
				auraSpell, found, err := target.server.Data.Spell(a.spellID)
				if err != nil || !found {
					continue
				}
				if auraTypes, _ := mechanicMask147SpellGrants(auraSpell); uint32InSlice(eff.Aura, auraTypes) {
					return true
				}
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
				// (caster && !IsFriendlyTo(caster)) || !IsPositiveEffect(index) (Unit.cpp:7987)
				if (caster != nil && !target.isFriendlyToPlayer(caster)) || !spell.IsPositiveEffect(effIndex) {
					return true
				}
			}
		}
	}
	return false
}

// spellEffectImmunityInfo is one per-effect ImmunityInfo row
// (SpellInfo::_LoadImmunityInfo, SpellInfo.cpp:2560-2818): the immunities a
// single effect of a spell grants while applied as an aura.
type spellEffectImmunityInfo struct {
	schoolMask       uint32          // SPELL_AURA_SCHOOL_IMMUNITY (39) misc
	mechanicMask     uint32          // 77 carve-outs/misc + 147 table (mechanicImmuneMaskForEffect)
	dispelImmune     uint32          // SPELL_AURA_DISPEL_IMMUNITY (41) misc
	spellEffects     map[uint32]bool // SPELL_AURA_EFFECT_IMMUNITY (37) misc set + 147 inserts
	auraTypes        map[uint32]bool // SPELL_AURA_STATE_IMMUNITY (38) misc set + 147 inserts
	applyHarmfulMask uint32          // SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (267) misc
}

// immunityInfoForEffect builds one effect's ImmunityInfo row from the spell
// row (SpellInfo.cpp:2585-2818). Only the writers
// CanSpellProvideImmunityAgainstAura consults are included: the ATTR5
// _allowedMechanicMask bits and the damage-immunity mask are not part of
// ImmunityInfo in C++.
func immunityInfoForEffect(spell wotlk.Spell, effIndex int) spellEffectImmunityInfo {
	info := spellEffectImmunityInfo{
		spellEffects: map[uint32]bool{},
		auraTypes:    map[uint32]bool{},
	}
	if effIndex < 0 || effIndex >= len(spell.Effects) {
		return info
	}
	eff := spell.Effects[effIndex]
	info.mechanicMask = mechanicImmuneMaskForEffect(spell, eff)
	switch eff.Aura {
	case spellAuraSchoolImmunity:
		info.schoolMask |= uint32(eff.MiscValue)
	case spellAuraDispelImmunity:
		info.dispelImmune = uint32(eff.MiscValue)
	case spellAuraEffectImmunity:
		info.spellEffects[uint32(eff.MiscValue)] = true
	case spellAuraStateImmunity:
		info.auraTypes[uint32(eff.MiscValue)] = true
	case spellAuraModImmuneAuraApplySchool:
		info.applyHarmfulMask |= uint32(eff.MiscValue)
	case spellAuraMechanicImmunityMask:
		auraTypes, spellEffects := mechanicMask147Grants(spell.ID, eff)
		for _, a := range auraTypes {
			info.auraTypes[a] = true
		}
		for _, s := range spellEffects {
			info.spellEffects[s] = true
		}
	}
	return info
}

// canSpellProvideImmunityAgainstAura mirrors
// SpellInfo::CanSpellProvideImmunityAgainstAura (SpellInfo.cpp:2923-2998):
// whether the incoming spell grants any immunity that covers the aura spell
// already on the target.
func canSpellProvideImmunityAgainstAura(spell, auraSpell wotlk.Spell) bool {
	for i := range spell.Effects {
		info := immunityInfoForEffect(spell, i)
		if auraSpell.AttributesEx&spellAttr1UnaffectedBySchoolImmune == 0 &&
			auraSpell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune == 0 {
			if info.schoolMask != 0 && auraSpell.SchoolMask&info.schoolMask != 0 {
				return true
			}
		}
		if info.mechanicMask&(1<<auraSpell.Mechanic) != 0 {
			return true
		}
		if info.dispelImmune != 0 && auraSpell.DispelType == info.dispelImmune {
			return true
		}
		immuneToAllEffects := true
		for effIndex := range auraSpell.Effects {
			aeff := auraSpell.Effects[effIndex]
			if aeff.Effect == 0 {
				continue
			}
			if !info.spellEffects[aeff.Effect] {
				immuneToAllEffects = false
				break
			}
			if aeff.Mechanic != 0 && info.mechanicMask&(1<<aeff.Mechanic) == 0 {
				immuneToAllEffects = false
				break
			}
			if auraSpell.AttributesEx3&spellAttr3IgnoreHitResult == 0 && aeff.Aura != 0 {
				immune := info.auraTypes[aeff.Aura]
				if !immune && !auraSpell.IsPositiveEffect(effIndex) &&
					auraSpell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune == 0 {
					if info.applyHarmfulMask != 0 && auraSpell.SchoolMask&info.applyHarmfulMask != 0 {
						immune = true
					}
				}
				if !immune {
					immuneToAllEffects = false
					break
				}
			}
		}
		if immuneToAllEffects {
			return true
		}
	}
	return false
}

// canSpellPierceImmuneAura mirrors SpellInfo::CanPierceImmuneAura
// (SpellInfo.cpp:1336-1361): whether the incoming spell pierces the aura
// that grants the target its immunity. The DISPEL_AURAS_ON_IMMUNITY arm is
// the full CanSpellProvideImmunityAgainstAura form (SpellInfo.cpp:2923-2998).
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
	// Dispels other auras on immunity: the spell pierces when it provides
	// immunity against the aura spell.
	if spell.AttributesEx&spellAttr1DispelAurasOnImmunity != 0 &&
		canSpellProvideImmunityAgainstAura(spell, immuneSpell) {
		return true
	}
	return false
}

// spellCanDispelAura mirrors SpellInfo::CanDispelAura
// (SpellInfo.cpp:1363-1381): whether the applying spell may dispel the
// already-applied aura spell.
func spellCanDispelAura(spell, auraSpell wotlk.Spell) bool {
	// These auras (like Divine Shield) can't be dispelled.
	if auraSpell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	// These spells (like Mass Dispel) can dispel all auras.
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return true
	}
	// These auras (Cyclone for example) are not dispelable.
	if (auraSpell.AttributesEx&spellAttr1UnaffectedBySchoolImmune != 0 && auraSpell.Mechanic != 0) ||
		auraSpell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune != 0 {
		return false
	}
	return true
}

// dispelAurasOnImmunityApply mirrors the ATTR1_DISPEL_AURAS_ON_IMMUNITY
// removal arms of SpellInfo::ApplyAllSpellImmunitiesTo
// (SpellInfo.cpp:2860-2920): when the just-applied aura effect grants
// immunity and its spell carries the attribute, the target's existing auras
// are dispelled per the granted row — school mask (RemoveAppliedAuras
// predicate, SpellInfo.cpp:2866-2875), mechanic mask
// (Unit::RemoveAurasWithMechanic, Unit.cpp:4217-4232), dispel type
// (SpellInfo.cpp:2894-2903), and aura-type list (RemoveAurasByType,
// SpellInfo.cpp:2908-2916). C++ runs this per aura effect from the immunity
// aura handlers (SpellAuraEffects.cpp:3108-3221); the IMMUNITY_DAMAGE row
// has no dispel arm. The school arm skips the applying spell itself and the
// mechanic arm passes it as except (C++); the self-skip on the dispel arm is
// a harmless guard (C++ has none, but self-match needs pathological data).
func dispelAurasOnImmunityApply(target *session, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if target == nil || target.player == nil {
		return
	}
	if spell.AttributesEx&spellAttr1DispelAurasOnImmunity == 0 {
		return
	}
	if target.server == nil || target.server.Data == nil {
		return
	}
	var schoolMask uint32
	if eff.Aura == spellAuraSchoolImmunity {
		schoolMask = uint32(eff.MiscValue)
	}
	mechanicMask := mechanicImmuneMaskForEffect(spell, eff)
	var dispelImmune uint32
	if eff.Aura == spellAuraDispelImmunity {
		dispelImmune = uint32(eff.MiscValue)
	}
	var auraTypes []uint32
	switch eff.Aura {
	case spellAuraStateImmunity:
		auraTypes = append(auraTypes, uint32(eff.MiscValue))
	case spellAuraMechanicImmunityMask:
		types, _ := mechanicMask147Grants(spell.ID, eff)
		auraTypes = append(auraTypes, types...)
	}
	if schoolMask == 0 && mechanicMask == 0 && dispelImmune == 0 && len(auraTypes) == 0 {
		return
	}
	spellPositive := spellIsPositive(spell)
	for _, aura := range target.loadedAuras() {
		if aura == nil || aura.SpellID == spell.ID {
			continue
		}
		auraSpell, found, err := target.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		remove := false
		if schoolMask != 0 && auraSpell.SchoolMask&schoolMask != 0 &&
			spellCanDispelAura(spell, auraSpell) &&
			spellPositive != aura.Positive &&
			auraSpell.Attributes&spellAttributePassive == 0 {
			remove = true
		}
		if !remove && mechanicMask != 0 && spellMechanicMask(auraSpell)&mechanicMask != 0 {
			remove = true
		}
		if !remove && dispelImmune != 0 && auraSpell.DispelType == dispelImmune {
			remove = true
		}
		if remove {
			target.removeAura(aura.SpellID)
		}
	}
	for _, auraType := range auraTypes {
		target.removeAurasByType(auraType)
	}
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

// creatureImmuneToDamageSpell is the creature-target analog of
// isImmuneToDamageSpell: Unit::IsImmunedToDamage(SpellInfo const*)
// (Unit.cpp:7818-7860) applies to any Unit, but Go's player-only
// isImmuneToDamage skipped creature victims entirely, so a creature with
// live aura-39/aura-4 rows took full spell damage. Fold is the same
// misc-based full-coverage OR with the per-entry CanPierceImmuneAura filter
// and the attribute gates; the school/damage masks come from the target's
// live creature auras (m_spellImmune[IMMUNITY_SCHOOL/IMMUNITY_DAMAGE]).
func creatureImmuneToDamageSpell(srv *Server, key creatureAuraKey, spell wotlk.Spell, schoolMask uint32) bool {
	if srv == nil || schoolMask == 0 {
		return false
	}
	// Unit.cpp:7824-7828 attribute gates.
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 &&
		spell.AttributesEx3&spellAttr3IgnoreHitResult != 0 {
		return false
	}
	if spell.AttributesEx&spellAttr1UnaffectedBySchoolImmune != 0 ||
		spell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune != 0 {
		return false
	}
	srv.auraMu.Lock()
	defer srv.auraMu.Unlock()
	var schoolImmunityMask uint32
	for _, aura := range srv.activeCreatureAuras[key] {
		if aura == nil || aura.AuraType != spellAuraSchoolImmunity {
			continue
		}
		misc := uint32(aura.MiscValue)
		if misc&schoolMask == 0 {
			continue
		}
		if srv.Data != nil {
			if immuneSpell, found, err := srv.Data.Spell(aura.SpellID); err == nil && found && canSpellPierceImmuneAura(spell, immuneSpell) {
				continue
			}
		}
		schoolImmunityMask |= misc
	}
	if schoolImmunityMask&schoolMask == schoolMask {
		return true
	}
	var damageImmunityMask uint32
	for _, aura := range srv.activeCreatureAuras[key] {
		if aura == nil || aura.AuraType != spellAuraDamageImmunity {
			continue
		}
		damageImmunityMask |= uint32(aura.MiscValue)
	}
	return damageImmunityMask&schoolMask == schoolMask
}

// creatureImmuneAuraApplySchoolMask is the creature-target analog of the
// player-side 267 snapshot arm: the OR of the misc (school-mask) values of
// the target's live SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (267) auras
// (GetAuraEffectsByType, Unit.cpp:7984). Go stores one activeAura per
// spell id, so the scan reads the stored AuraType/MiscValue the same way
// immuneAuraSnapshotLocked does for players.
func (s *Server) creatureImmuneAuraApplySchoolMask(key creatureAuraKey) uint32 {
	if s == nil {
		return 0
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	var mask uint32
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.AuraType != spellAuraModImmuneAuraApplySchool {
			continue
		}
		mask |= uint32(aura.MiscValue)
	}
	return mask
}

// creatureImmuneToSpellEffect mirrors the creature-target per-effect
// immunity chain: Totem::IsImmunedToSpellEffect (Totem.cpp:183-204),
// Creature::IsImmunedToSpellEffect (Creature.cpp:2336-2341), then
// Unit::IsImmunedToSpellEffect (Unit.cpp:7952-7994) fed by the
// Creature::LoadTemplateImmunities (Creature.cpp:2279-2313) and
// flags_extra (Creature.cpp:634-638, 1184-1187) ApplySpellImmune writers.
// stats carries the template rows; isTotem marks a live player totem;
// the 267 arm scans the target's live creature auras via
// creatureImmuneAuraApplySchoolMask (GetAuraEffectsByType, Unit.cpp:7984),
// and friendly is the target's IsFriendlyTo(caster) verdict, precomputed
// by the caller.
func creatureImmuneToSpellEffect(srv *Server, key creatureAuraKey, spell wotlk.Spell, effIndex int, stats creatureStats, isTotem bool, caster *session, friendly bool) bool {
	if effIndex < 0 || effIndex >= len(spell.Effects) {
		return false
	}
	eff := spell.Effects[effIndex]
	if eff.Effect == 0 {
		return false
	}
	// Unit::IsImmunedToSpellEffect (Unit.cpp:7958-7959).
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	// Totem::IsImmunedToSpellEffect (Totem.cpp:183-194): immune to all
	// positive spells except DUMMY/SCRIPT_EFFECT effects, effects whose
	// first implicit target is the caster, TARGET_CHECK_ENTRY area effects
	// (Go models the entry-area target ids 7/8 via isEntryAreaTargetType),
	// and the stoneclaw-absorb / sentry-bind-sight spells.
	if isTotem {
		if eff.Effect != spellEffectDummy && eff.Effect != spellEffectScriptEffect &&
			spellIsPositive(spell) && eff.ImplicitTargetA != targetUnitCaster &&
			!isEntryAreaTargetType(eff.ImplicitTargetA) &&
			spell.ID != sentryStoneclawSpellID && spell.ID != sentryBindSightSpellID {
			return true
		}
		switch eff.Aura {
		case spellAuraPeriodicDamage, spellAuraPeriodicLeech, spellAuraModFear, spellAuraTransform:
			return true
		}
	}
	// Creature::IsImmunedToSpellEffect (Creature.cpp:2336-2341):
	// mechanical creatures are immune to SPELL_EFFECT_HEAL.
	if stats.CreatureType == creatureTypeMechanical && eff.Effect == spellEffectHeal {
		return true
	}
	// IMMUNITY_EFFECT (Unit.cpp:7962-7966): the NO_TAUNT flags_extra arm
	// (Creature.cpp:634-638) and the knockback-immunity arm
	// (Creature.cpp:1184-1187).
	if stats.FlagsExtra&creatureFlagExtraNoTaunt != 0 && eff.Effect == spellEffectAttackMe {
		return true
	}
	if stats.FlagsExtra&creatureFlagExtraImmunityKnockback != 0 &&
		(eff.Effect == spellEffectKnockBack || eff.Effect == spellEffectKnockBackDest) {
		return true
	}
	// IMMUNITY_MECHANIC (Unit.cpp:7968-7973): the template mask stores bit
	// (i-1) for mechanic i (Creature.cpp:2299-2301).
	if eff.Mechanic != 0 && stats.MechanicImmuneMask&(1<<(eff.Mechanic-1)) != 0 {
		return true
	}
	// IMMUNITY_STATE (Unit.cpp:7977-7989): the NO_TAUNT arm's MOD_TAUNT row.
	if spell.AttributesEx3&spellAttr3IgnoreHitResult == 0 && eff.Aura != 0 {
		if stats.FlagsExtra&creatureFlagExtraNoTaunt != 0 && eff.Aura == spellAuraModTaunt {
			return true
		}
		// SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (267) arm
		// (Unit.cpp:7981-7989): immune to the application of harmful
		// magical effects whose school overlaps a live 267 row's misc
		// mask — harmful means the caster exists and is not friendly to
		// the target, or the effect is not positive.
		if spell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune == 0 {
			if mask := srv.creatureImmuneAuraApplySchoolMask(key); mask&spell.SchoolMask != 0 &&
				((caster != nil && !friendly) || !spell.IsPositiveEffect(effIndex)) {
				return true
			}
		}
	}
	return false
}

// creatureImmuneToSpell mirrors Creature::IsImmunedToSpell
// (Creature.cpp:2315-2333): the all-effects per-effect fold, then the
// Unit::IsImmunedToSpell arms (Unit.cpp:7852-7922) reachable from the
// template writers — spell-level mechanic and the school fold. IMMUNITY_ID
// has no writer model (spell_linked_spell negative rows unbridged) and
// IMMUNITY_DISPEL has no template writer, so both arms are vacuous here.
func creatureImmuneToSpell(srv *Server, key creatureAuraKey, spell wotlk.Spell, stats creatureStats, isTotem bool, caster *session, targetFaction uint32, friendlyToCaster func(uint32) bool) bool {
	if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	friendly := caster != nil && friendlyToCaster != nil && friendlyToCaster(targetFaction)
	anyEffect := false
	immunedToAllEffects := true
	for i := range spell.Effects {
		if spell.Effects[i].Effect == 0 {
			continue
		}
		anyEffect = true
		if !creatureImmuneToSpellEffect(srv, key, spell, i, stats, isTotem, caster, friendly) {
			immunedToAllEffects = false
			break
		}
	}
	if anyEffect && immunedToAllEffects {
		return true
	}
	// Spell-level mechanic (Unit.cpp:7870-7876) vs the template
	// IMMUNITY_MECHANIC rows.
	if spell.Mechanic != 0 && stats.MechanicImmuneMask&(1<<(spell.Mechanic-1)) != 0 {
		return true
	}
	// School fold (Unit.cpp:7888-7919): each template IMMUNITY_SCHOOL row
	// is a single-school bit (Creature.cpp:2303-2312) with the placeholder
	// spell id, so immuneSpellInfo is nil — the row grants immunity unless
	// the spell is positive AND has a caster AND the target is friendly to
	// the caster, and the spell cannot pierce it (nil immuneSpellInfo
	// behaves like Go's zero-value immune spell in
	// canSpellPierceImmuneAura).
	if schoolMask := spell.SchoolMask; schoolMask != 0 {
		var schoolImmunityMask uint32
		for row := stats.SpellSchoolImmuneMask; row != 0; row &= row - 1 {
			bit := row & -row
			if bit&schoolMask == 0 {
				continue
			}
			if !spellIsPositive(spell) || caster == nil || !friendly {
				if !canSpellPierceImmuneAura(spell, wotlk.Spell{}) {
					schoolImmunityMask |= bit
				}
			}
		}
		if schoolImmunityMask&schoolMask == schoolMask {
			return true
		}
	}
	return false
}

// creatureTargetImmunityStats resolves the template rows for a creature
// hit target: entry via the live creature motion, masks via the cached
// per-entry loadCreatureStats. Reports false when the GUID is not a live
// creature (player targets and unresolvable GUIDs use the session-side
// immunity evals instead).
func (s *session) creatureTargetImmunityStats(ctx context.Context, targetGUID uint64) (creatureStats, bool) {
	if s == nil || s.server == nil || s.player == nil {
		return creatureStats{}, false
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	s.server.motionMu.Unlock()
	if motion == nil {
		return creatureStats{}, false
	}
	return s.server.loadCreatureStats(ctx, motion.Entry), true
}

// creatureFriendlyToCaster mirrors the IsFriendlyTo(caster) verdict from
// the target creature's side: the target faction is not attackable by the
// caster (the same verdict the school folds use).
func (s *session) creatureFriendlyToCaster(caster *session, targetFaction uint32) bool {
	if s == nil || s.server == nil || caster == nil || caster.player == nil {
		return false
	}
	pos := playerPos{Map: caster.player.Map, InstanceID: caster.player.InstanceID, X: caster.player.X, Y: caster.player.Y, Z: caster.player.Z, GUID: caster.playerGUID, Race: caster.player.Race, Class: caster.player.Class, Level: caster.player.Level, FactionTemplate: s.server.raceFaction(caster.player.Race), Reputations: playerReputationMap(caster.player.Reputations), Sess: caster}
	return !s.server.isAttackableFaction(targetFaction, pos)
}

// creatureTargetImmuneToSpell is the creature-target whole-spell gate
// (Creature::IsImmunedToSpell) at hit resolution; caster is the casting
// session for the school fold's friendliness gate.
func (s *session) creatureTargetImmuneToSpell(ctx context.Context, targetGUID uint64, spell wotlk.Spell, caster *session, targetFaction uint32) bool {
	stats, ok := s.creatureTargetImmunityStats(ctx, targetGUID)
	if !ok {
		return false
	}
	isTotem := s.server.isTotemGUID(targetGUID)
	friendlyToCaster := func(faction uint32) bool {
		return s.creatureFriendlyToCaster(caster, faction)
	}
	return creatureImmuneToSpell(s.server, creatureAuraKeyForPlayer(*s.player, targetGUID), spell, stats, isTotem, caster, targetFaction, friendlyToCaster)
}

// creatureTargetFullyEffectImmune is the creature-target IMMUNE2 gate:
// every non-zero effect per-effect immune (Spell.cpp:2108-2112, 4491-4492).
func (s *session) creatureTargetFullyEffectImmune(ctx context.Context, targetGUID uint64, spell wotlk.Spell, caster *session, targetFaction uint32) bool {
	stats, ok := s.creatureTargetImmunityStats(ctx, targetGUID)
	if !ok {
		return false
	}
	isTotem := s.server.isTotemGUID(targetGUID)
	friendly := s.creatureFriendlyToCaster(caster, targetFaction)
	key := creatureAuraKeyForPlayer(*s.player, targetGUID)
	anyEffect := false
	for i := range spell.Effects {
		if spell.Effects[i].Effect == 0 {
			continue
		}
		anyEffect = true
		if !creatureImmuneToSpellEffect(s.server, key, spell, i, stats, isTotem, caster, friendly) {
			return false
		}
	}
	return anyEffect
}
