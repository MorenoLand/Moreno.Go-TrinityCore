package world

// auraModOffhandDamagePct is SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (122) — the
// off-hand-only percent leg of Unit::UpdateDamagePctDoneMods.
const auraModOffhandDamagePct uint32 = 122 // SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (Unit.cpp:9412-9413)

// meleeDamageDoneFlat mirrors the TOTAL_VALUE arm of Unit::UpdateDamageDoneMods
// (Unit.cpp:9334-9360): the flat sum of SPELL_AURA_MOD_DAMAGE_DONE (13) effects
// whose misc value carries the NORMAL school bit (SPELL_SCHOOL_MASK_NORMAL).
// C++ computes the same sum for each attack type — only the
// CheckAttackFitToAuraRequirement weapon-fit gate varies — so one fold serves
// main hand, off hand and ranged. The gate itself has no Go model (same
// standing delta as the aura-52 weapon-crit fold): amounts sum unconditionally.
func (s *session) meleeDamageDoneFlat() int32 {
	if s == nil || s.player == nil {
		return 0
	}
	var total int32
	for _, amt := range s.auraTypeModifiersByMiscMask(spellAuraModDamageDone, 1) {
		total += amt
	}
	return total
}

// meleeDamageDonePctMod mirrors the TOTAL_PCT arm of
// Unit::UpdateDamagePctDoneMods (Unit.cpp:9380-9416): the product of
// (1 + amount/100) over SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79) effects with
// the NORMAL school bit, times the SPELL_AURA_MOD_OFFHAND_DAMAGE_PCT (122)
// product for the off hand. The C++ 0.5 off-hand factor is not part of this
// fold: Go keeps it as the hardcoded halving in executeMeleeSwing, matching
// Unit::AttackerStateUpdate's off-hand penalty (Unit.cpp:353). Same-spell
// group stacking is a plain product here, matching the spell-side 79 fold in
// spellDamagePctDone; the mutual-dormancy quirk of GetTotalAuraMultiplier
// stays unmodeled.
func (s *session) meleeDamageDonePctMod(offhand bool) float64 {
	if s == nil || s.player == nil {
		return 1.0
	}
	mod := 1.0
	for _, amt := range s.auraTypeModifiersByMiscMask(auraModDamagePercentDone, 1) {
		mod *= 1 + float64(amt)/100
	}
	if offhand {
		for _, amt := range s.auraTypeModifiers(auraModOffhandDamagePct) {
			mod *= 1 + float64(amt)/100
		}
	}
	return mod
}
