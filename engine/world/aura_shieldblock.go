// Player::GetShieldBlockValue (Player.cpp:5426-5430) folds gear/enchant
// block value (state.Block, fed by HandleBaseModFlatValue(SHIELD_BLOCK_VALUE)
// for ITEM_MOD_BLOCK_VALUE, Player.cpp:7682-7683, proto->Block :7731, and
// enchantments :14217-14218) together with the 158 flat leg
// (AuraEffect::HandleShieldBlockValue, SpellAuraEffects.cpp:4212: plain
// HandleBaseModFlatValue add, player-only) and the strength term, then
// applies the 150 pct leg (AuraEffect::HandleShieldBlockValuePercent,
// SpellAuraEffects.cpp:4224: multiplicative ApplyBaseModPctValue on the
// base-pct mod, whose remove leg re-sets GetTotalAuraMultiplier over the
// remaining auras — identical to a live product fold). The live-aura fold
// needs no apply/remove recalc hook, like the haste and damage folds.
// Documented delta: creature victims keep the Creature::GetShieldBlockValue
// (Creature.cpp:2715) level/2 stand-in at the block-amount site.
package world

func (s *session) getShieldBlockValue() uint32 {
	flat := float64(0)
	str := float64(0)
	if s != nil && s.player != nil {
		flat = float64(s.player.Block)
		if len(s.player.Stats) > 0 {
			str = float64(s.player.Stats[0])
		}
	}
	for _, amount := range s.auraTypeModifiersFiltered(spellAuraModShieldBlockvalue, func(int32) bool { return true }) {
		flat += float64(amount)
	}
	pctMult := 1.0
	for _, amount := range s.auraTypeModifiersFiltered(spellAuraModShieldBlockvaluePct, func(int32) bool { return true }) {
		pctMult *= 1.0 + float64(amount)/100.0
	}
	if value := (flat + str*0.5 - 10) * pctMult; value > 0 {
		return uint32(value)
	}
	return 0
}
