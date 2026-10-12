package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// auraModOffhandDamagePct is SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (122) — the
// off-hand-only percent leg of Unit::UpdateDamagePctDoneMods.
const auraModOffhandDamagePct uint32 = 122 // SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (Unit.cpp:9412-9413)

// attackFitsAuraRequirement mirrors Player::CheckAttackFitToAuraRequirement
// (Player.cpp:7945-7954): the aura's spell applies to the given attack type
// when it carries no equipped-item requirement (EquippedItemClass == -1), or
// the useable weapon for that attack type fits the spell's class/subclass
// requirement. The weapon lookup is Player::GetWeaponForAttack(useable=true)
// (Player.cpp:10083-10109) via equippedWeaponInstance: the equipped weapon of
// ITEM_CLASS_WEAPON in the attack type's slot (mainhand/offhand/ranged),
// unbroken; in feral form (cat/bear/dire bear) it returns null, so
// item-requiring spells never fit. The fit test is
// Item::IsFitToSpellRequirements' non-enchant arm (Item.cpp:804-819): class
// equality plus the subclass mask (0 = any subclass). Aura spells are never
// enchant spells, so the vellum and inventory-type arms are vacuous.
func (s *session) attackFitsAuraRequirement(ctx context.Context, spell wotlk.Spell, attackType protocol.WeaponAttackType) bool {
	if spell.EquippedItemClass < 0 {
		return true
	}
	if s == nil || s.server == nil {
		return false
	}
	var form uint8
	if s.player != nil {
		form = s.player.ShapeshiftForm
	}
	if form == 1 || form == 5 || form == 8 { // FORM_CAT, FORM_BEAR, FORM_DIREBEAR
		return false
	}
	var slot uint8
	switch attackType {
	case protocol.OffAttack:
		slot = equipSlotOffhand
	case protocol.RangedAttack:
		slot = equipSlotRanged
	default:
		slot = equipSlotMainhand
	}
	_, entry := s.equippedWeaponInstance(ctx, slot)
	if entry == 0 {
		return false
	}
	info, ok := s.server.getItemStoreTemplateInfo(ctx, entry)
	return ok && int32(info.Class) == spell.EquippedItemClass &&
		(spell.EquippedItemSubClass == 0 || spell.EquippedItemSubClass&(1<<info.SubClass) != 0)
}

// damageDoneAuraAmounts iterates the live effects of the given aura type on
// the player, yielding each effect's amount after the
// CheckAttackFitToAuraRequirement weapon-fit gate for the attack type. Misc
// mask 0 disables the misc filter: the 122 off-hand leg carries no misc gate
// in C++ (Unit.cpp:9412-9413). Amount resolution mirrors
// auraTypeModifiersByMiscMask (stored amount, then BasePoints); auras whose
// spell data is missing are skipped since the gate needs the spell's
// equipped-item requirement (the crit-fold gate helper does the same).
func (s *session) damageDoneAuraAmounts(ctx context.Context, auraType uint32, miscMask uint32, attackType protocol.WeaponAttackType) []int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	var result []int32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if !s.attackFitsAuraRequirement(ctx, spell, attackType) {
			continue
		}
		usedStoredAmount := false
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraType {
				continue
			}
			if miscMask != 0 && uint32(effect.MiscValue)&miscMask == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == auraType && (miscMask == 0 || uint32(aura.MiscValue)&miscMask != 0) && !usedStoredAmount {
				amount = int32(aura.Amount)
				usedStoredAmount = true
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			result = append(result, amount)
		}
	}
	return result
}

// meleeDamageDoneFlat mirrors the TOTAL_VALUE arm of Unit::UpdateDamageDoneMods
// (Unit.cpp:9334-9360): per attack type, the flat sum of
// SPELL_AURA_MOD_DAMAGE_DONE (13) effects whose misc value carries the NORMAL
// school bit (SPELL_SCHOOL_MASK_NORMAL) and which pass the
// CheckAttackFitToAuraRequirement weapon-fit gate (Player.cpp:7945-7954).
func (s *session) meleeDamageDoneFlat(ctx context.Context, attackType protocol.WeaponAttackType) int32 {
	var total int32
	for _, amt := range s.damageDoneAuraAmounts(ctx, spellAuraModDamageDone, spellSchoolMaskNormal, attackType) {
		total += amt
	}
	return total
}

// meleeDamageDonePctMod mirrors the TOTAL_PCT arm of
// Unit::UpdateDamagePctDoneMods (Unit.cpp:9380-9416): per attack type, the
// product of (1 + amount/100) over SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79)
// effects with the NORMAL school bit passing the weapon-fit gate, times the
// SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (122) product for the off hand (122
// carries no misc gate in C++). The C++ 0.5 off-hand factor is not part of
// this fold: Go keeps it as the hardcoded halving in executeMeleeSwing,
// matching Unit::AttackerStateUpdate's off-hand penalty (Unit.cpp:353).
// Same-spell group stacking is a plain product, matching the spell-side 79
// fold in spellDamagePctDone; the mutual-dormancy quirk of
// GetTotalAuraMultiplier stays unmodeled.
func (s *session) meleeDamageDonePctMod(ctx context.Context, attackType protocol.WeaponAttackType) float64 {
	mod := 1.0
	for _, amt := range s.damageDoneAuraAmounts(ctx, auraModDamagePercentDone, spellSchoolMaskNormal, attackType) {
		mod *= 1 + float64(amt)/100
	}
	if attackType == protocol.OffAttack {
		for _, amt := range s.damageDoneAuraAmounts(ctx, auraModOffhandDamagePct, 0, attackType) {
			mod *= 1 + float64(amt)/100
		}
	}
	return mod
}

// meleeDamageDoneVersusMod mirrors the school-independent versus legs of
// Unit::MeleeDamageBonusDone (Unit.cpp:8061-8068), which C++ runs on every
// melee swing (CalculateMeleeDamage, Unit.cpp:1236): the
// MOD_DAMAGE_DONE_VERSUS (168) multiplier over the caster's auras whose
// misc creature-type mask intersects the victim's, and the
// MOD_DAMAGE_DONE_VERSUS_AURASTATE (303) multiplier over the caster's auras
// whose misc aura state is active on the victim. Unlike the NORMAL-school
// arm of MeleeDamageBonusDone (baked into the weapon-damage base in C++ and
// into meleeDamageDonePctMod in Go), these legs are NOT gated by
// SPELL_ATTR6_LIMIT_PCT_DAMAGE_MODS, so they ride on top of the existing
// fold. Creature attackers keep the gap: no versus aura infra.
func (s *session) meleeDamageDoneVersusMod(ctx context.Context, targetGUID uint64) float64 {
	mod := 1.0
	// MOD_DAMAGE_DONE_VERSUS (Unit.cpp:8061): GetTotalAuraMultiplierByMiscMask
	// over the victim's creature-type mask.
	if typMask, ok := s.targetCreatureTypeMask(ctx, targetGUID); ok && typMask != 0 {
		for _, amt := range s.auraTypeModifiersByMiscMask(auraModDamageDoneVersus, typMask) {
			mod *= 1 + float64(amt)/100
		}
	}
	// MOD_DAMAGE_DONE_VERSUS_AURASTATE (Unit.cpp:8064-8068): the predicate is
	// victim->HasAuraState(misc) — the same read as the spell-side 303 leg in
	// spellDamagePctDone.
	for _, ae := range s.auraTypeMiscAmounts(auraModDamageDoneVersusAurastate) {
		if s.targetHasAuraState(ctx, targetGUID, uint32(ae.misc), wotlk.Spell{}) {
			mod *= 1 + float64(ae.amount)/100
		}
	}
	return mod
}
