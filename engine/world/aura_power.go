package world

// Aura-driven max-power arms: AuraEffect::HandleAuraModIncreaseEnergy
// (SpellAuraEffects.cpp:3686 — aura type 35, SPELL_AURA_MOD_INCREASE_ENERGY)
// and HandleAuraModIncreaseEnergyPercent (SpellAuraEffects.cpp:3698 — aura
// type 233, SPELL_AURA_MOD_INCREASE_ENERGY_PERCENT). C++ keeps these in
// Unit::m_auraModifiersGroup[UNIT_MOD_POWER_START + power] (Unit.cpp:9237):
// the flat TOTAL_VALUE adds (35) and TOTAL_PCT multiplies the total (233),
// the pct leg folding multiplicatively
// (AddPct: m *= 1+pct/100; the remove leg rebuilds the product from the
// remaining auras via GetTotalAuraMultiplierByMiscValue,
// SpellAuraEffects.cpp:3705). Go has no UNIT_MOD group, so the live max
// power per power type is recomputed from the active aura records on demand,
// mirroring the aura-health fold (aura_health.go). The misc value of both
// effects is the Powers type they target (SpellAuraEffects.cpp:3690/3702);
// Go's player power indices match the C++ Powers enum (0=mana, 1=rage,
// 2=focus, 3=energy, 4=happiness, 5=rune, 6=runic power).
//
// Regen aura arms: HandleModPowerRegen (SpellAuraEffects.cpp:3613 — aura
// type 85, SPELL_AURA_MOD_POWER_REGEN) and HandleModPowerRegenPCT
// (SpellAuraEffects.cpp:3631 — aura type 110,
// SPELL_AURA_MOD_POWER_REGEN_PERCENT) are folded at tick time in
// Player::Regenerate (Player.cpp:2114-2168) — the misc value is the Powers
// type, the flat leg adds amount * tickMs / 5000 and the pct leg multiplies
// the whole addvalue (GetTotalAuraMultiplierByMiscValue, Player.cpp:2163).
// The mana arms run through Player::UpdateManaRegen (StatSystem.cpp:912):
// the spirit-term regen is multiplied by the 110 misc-mana product, the mp5
// term is (85 misc-mana sum + m_baseManaRegen) / 5, HandleModManaRegen
// (SpellAuraEffects.cpp:3636 — aura type 219,
// SPELL_AURA_MOD_MANA_REGEN_FROM_STAT) adds stat[misc] * amount / 500, and
// HandleAuraModRegenInterrupt (SpellAuraEffects.cpp:3812 — aura type 134,
// SPELL_AURA_MOD_MANA_REGEN_INTERRUPT) feeds the interrupted-cast regen via
// UNIT_FIELD_POWER_REGEN_INTERRUPTED_FLAT_MODIFIER = mp5 + 134%-of-spirit,
// capped at 100% (StatSystem.cpp:933-935).

const (
	spellAuraModIncreaseEnergy        = 35  // SPELL_AURA_MOD_INCREASE_ENERGY (SpellAuraEffects.cpp:100)
	spellAuraModIncreaseEnergyPercent = 233 // SPELL_AURA_MOD_INCREASE_ENERGY_PERCENT (SpellAuraEffects.cpp:298)
	spellAuraModPowerRegenPct         = 110 // SPELL_AURA_MOD_POWER_REGEN_PERCENT (SpellAuraEffects.cpp:175)
	spellAuraModManaRegenInterrupt    = 134 // SPELL_AURA_MOD_MANA_REGEN_INTERRUPT (SpellAuraEffects.cpp:199)
	spellAuraModManaRegenFromStat     = 219 // SPELL_AURA_MOD_MANA_REGEN_FROM_STAT (SpellAuraEffects.cpp:284)
	// spellAuraModPowerRegen (85) already exists in spells.go.
)

// auraPowerMods folds the live max-power-modifier aura effects for one power
// index into the flat TOTAL_VALUE sum (35) and the TOTAL_PCT product (233).
// extra carries aura records that are not (or no longer) in the session's
// active map — the removed aura on the remove leg, whose C++ handler still
// sees the effect when it runs (Unit::_UnapplyAura erases before the
// handlers, Unit.cpp:3548).
func (s *session) auraPowerMods(powerIdx int, excludeSpellID uint32, extra ...*activeAura) (flat int32, totalMult float32) {
	totalMult = 1
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || powerIdx < 0 || powerIdx >= len(s.player.MaxPowers) {
		return 0, 1
	}
	auras := s.loadedAuras()
	auras = append(auras, extra...)
	for i, aura := range auras {
		if aura == nil || aura.SpellID == excludeSpellID {
			continue
		}
		if i < len(auras)-len(extra) && aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if int(effect.MiscValue) != powerIdx {
				continue
			}
			amount := aura.Amounts[index]
			switch effect.Aura {
			case spellAuraModIncreaseEnergy:
				flat += amount
			case spellAuraModIncreaseEnergyPercent:
				totalMult *= 1 + float32(amount)/100
			}
		}
	}
	return flat, totalMult
}

// effectiveMaxPower mirrors Unit::UpdateMaxPower for
// UNIT_MOD_POWER_START+power (Unit.cpp:9339): (base + totalValue) * totalPct,
// clamped at 0 like the uint32 field math.
func (s *session) effectiveMaxPower(powerIdx int) uint32 {
	if s == nil || s.player == nil || powerIdx < 0 || powerIdx >= len(s.player.MaxPowers) {
		return 0
	}
	flat, totalMult := s.auraPowerMods(powerIdx, 0)
	v := (float64(s.player.MaxPowers[powerIdx]) + float64(flat)) * float64(totalMult)
	if v < 0 {
		v = 0
	}
	return uint32(v)
}

// effectiveMaxPowerExcluding recomputes the max power for the power index as
// if the named spell's aura were absent — the pre-apply max for the 233
// power-shift arm.
func (s *session) effectiveMaxPowerExcluding(powerIdx int, spellID uint32) uint32 {
	if s == nil || s.player == nil || powerIdx < 0 || powerIdx >= len(s.player.MaxPowers) {
		return 0
	}
	flat, totalMult := s.auraPowerMods(powerIdx, spellID)
	v := (float64(s.player.MaxPowers[powerIdx]) + float64(flat)) * float64(totalMult)
	if v < 0 {
		v = 0
	}
	return uint32(v)
}

// auraTouchesPower reports whether any of the aura's effects is a max-power
// modifier (35/233), mirroring the two handlers' table rows
// (SpellAuraEffects.cpp:100/298).
func (s *session) auraTouchesPower(aura *activeAura) bool {
	for _, auraType := range []uint32{
		spellAuraModIncreaseEnergy,
		spellAuraModIncreaseEnergyPercent,
	} {
		if s.activeAuraHasEffect(aura, auraType) {
			return true
		}
	}
	return false
}

// clampPowerToMax mirrors the current-power half of Unit::ModifyPower
// (Unit.cpp): SetPower clamps the value into [0, max].
func clampPowerToMax(cur, max uint32) uint32 {
	if cur > max {
		return max
	}
	return cur
}

// applyPowerAuraArms runs the current-power half of the max-power-aura apply
// leg, called from the fresh-apply tail of applyAuraToTarget once the aura
// record is registered. The 35 flat arm has no current-power leg in C++
// (SpellAuraEffects.cpp:3686-3695 just moves the stat modifier); the 233 arm
// shifts current power by the max delta (change = newMax - oldMax,
// SpellAuraEffects.cpp:3718-3721).
func (s *session) applyPowerAuraArms(spellID, auraType uint32, misc int32, amount int32) {
	if s == nil || s.player == nil || auraType != spellAuraModIncreaseEnergyPercent {
		return
	}
	powerIdx := int(misc)
	if powerIdx < 0 || powerIdx >= len(s.player.Powers) {
		return
	}
	newMax := s.effectiveMaxPower(powerIdx)
	oldMax := s.effectiveMaxPowerExcluding(powerIdx, spellID)
	shift := int64(newMax) - int64(oldMax)
	cur := int64(s.player.Powers[powerIdx]) + shift
	if cur < 0 {
		cur = 0
	}
	s.player.Powers[powerIdx] = clampPowerToMax(uint32(cur), newMax)
}

// removePowerAuraArms runs the current-power half of the max-power-aura
// remove leg. The aura record is already erased (Unit::_UnapplyAura erases
// before the handlers, Unit.cpp:3548), so the recompute sees the remaining
// auras. The 35 arm has no current-power leg; the 233 arm shifts current
// power by the max delta against the pre-remove max, which still includes
// the removed aura (same extra-record trick as the health remove leg).
func (s *session) removePowerAuraArms(removed *activeAura) {
	if s == nil || s.player == nil || removed == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(removed.SpellID)
	if err != nil || !found {
		return
	}
	for index, effect := range spell.Effects {
		if index >= len(removed.Amounts) || removed.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		if effect.Aura != spellAuraModIncreaseEnergyPercent {
			continue
		}
		powerIdx := int(effect.MiscValue)
		if powerIdx < 0 || powerIdx >= len(s.player.Powers) {
			continue
		}
		newMax := s.effectiveMaxPower(powerIdx)
		flat, totalMult := s.auraPowerMods(powerIdx, 0, removed)
		base := float64(s.player.MaxPowers[powerIdx])
		v := (base + float64(flat)) * float64(totalMult)
		if v < 0 {
			v = 0
		}
		oldMax := uint32(v)
		shift := int64(newMax) - int64(oldMax)
		cur := int64(s.player.Powers[powerIdx]) + shift
		if cur < 0 {
			cur = 0
		}
		s.player.Powers[powerIdx] = clampPowerToMax(uint32(cur), newMax)
	}
}

// auraPowerRegenTerms folds the live 85/110 effects for one power index into
// the per-5s flat sum and the multiplier product that Player::Regenerate
// applies to non-mana powers (Player.cpp:2163-2168).
func (s *session) auraPowerRegenTerms(powerIdx int) (flat int32, mult float32) {
	mult = 1
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || powerIdx < 0 {
		return 0, 1
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if int(effect.MiscValue) != powerIdx {
				continue
			}
			switch effect.Aura {
			case spellAuraModPowerRegen:
				flat += aura.Amounts[index]
			case spellAuraModPowerRegenPct:
				mult *= 1 + float32(aura.Amounts[index])/100
			}
		}
	}
	return flat, mult
}

// auraManaRegenTerms folds the live mana-regen aura effects into the
// UpdateManaRegen terms (StatSystem.cpp:912-937): the 110 misc-mana
// multiplier on the spirit-based regen, the mp5-per-second term from 85
// misc-mana ((sum + m_baseManaRegen) / 5 — m_baseManaRegen rides the live
// ItemManaRegenBonus field, folded in by the caller at server.go), plus the
// 219 from-stat arms (stat[misc] * amount / 500, misc is a Stats index), and
// the 134 interrupt percentage (capped at 100) that feeds the under-5s-rule
// regen arm.
func (s *session) auraManaRegenTerms() (spiritMult float32, mp5PerSec float32, interruptPct int32) {
	spiritMult = 1
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 1, 0, 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			switch effect.Aura {
			case spellAuraModPowerRegenPct:
				if int(effect.MiscValue) == 0 { // POWER_MANA
					spiritMult *= 1 + float32(amount)/100
				}
			case spellAuraModPowerRegen:
				if int(effect.MiscValue) == 0 { // POWER_MANA
					mp5PerSec += float32(amount) / 5.0
				}
			case spellAuraModManaRegenFromStat:
				misc := int(effect.MiscValue)
				if misc >= 0 && misc < len(s.player.Stats) {
					mp5PerSec += float32(s.player.Stats[misc]) * float32(amount) / 500.0
				}
			case spellAuraModManaRegenInterrupt:
				interruptPct += amount
			}
		}
	}
	if interruptPct > 100 {
		interruptPct = 100
	}
	return spiritMult, mp5PerSec, interruptPct
}

// Creature targets are out of scope here: the C++ handlers are unit-generic
// (SpellAuraEffects.cpp:3613-3726) but Go tracks creature power deltas
// nowhere; bridging the creature leg needs the creature stat-mod model
// (standing gap, same queue as creature-caster EffectTransmitted). Rune
// regen (Player::UpdateRuneRegen, StatSystem.cpp:940) is bridged in
// runes.go: updatePlayerRuneRegenValues fills the PLAYER_RUNE_REGEN fields
// on every player update build and runeBaseCooldownMsForSlot folds the 110
// POWER_RUNE aura leg (Player::GetRuneBaseCooldown, Player.cpp:24840). Rage
// decay (Player::Regenerate POWER_RAGE, Player.cpp:2147-2155) has no Go
// rage model and stays unbridged.
