package world

// attackSpeedFactor composes the C++ m_modAttackSpeedPct product
// (Unit::ApplyAttackTimePercentMod, Unit.cpp:10847-10864): every aura arm
// multiplies the attack-time divisor by (1 + amount/100), positive amounts
// speeding the swing up and negative amounts slowing it down. The rating
// legs ride the same path (Player::ApplyRatingMod, Player.cpp:5607-5616),
// so rating and aura factors are uniformly multiplicative here.
func attackSpeedFactor(amounts []int32) float32 {
	f := float32(1)
	for _, a := range amounts {
		f *= 1 + float32(a)/100
	}
	return f
}

// attackSpeedAuraFactor folds the live amounts of one aura type into the
// m_modAttackSpeedPct-style product (see attackSpeedFactor above).
func (s *session) attackSpeedAuraFactor(auraType uint32) float32 {
	return attackSpeedFactor(s.auraTypeModifiers(auraType))
}

// meleeHaste138Factor mirrors AuraEffect::HandleModMeleeSpeedPct
// (SpellAuraEffects.cpp:3984-4004): SPELL_AURA_MOD_MELEE_HASTE (138)
// applies to base and offhand attacks under the exclusive-same-effect
// spell-group gate (GetHighestExclusiveSameEffectSpellGroupValue,
// Unit.cpp:13967), reused from the stat-aura winner-takes-all fold
// (statWinners in aura_stats.go). Ungrouped effects always apply.
func (s *session) meleeHaste138Factor() float32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 1
	}
	var cands []statCandidate
	auras := s.loadedAuras()
	for i, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraModMeleeHaste {
				continue
			}
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			cands = append(cands, statCandidate{
				order:       i,
				spellID:     aura.SpellID,
				effectIndex: index,
				amount:      aura.Amounts[index],
				firstRank:   s.server.spellFirstRank(aura.SpellID),
			})
		}
	}
	var amounts []int32
	for _, w := range statWinners(cands, s.server.petAuraStackGroups()) {
		amounts = append(amounts, w.amount)
	}
	return attackSpeedFactor(amounts)
}

// combatSlowLiveAmounts mirrors the exclusive gate of
// AuraEffect::HandleModCombatSpeedPct (SpellAuraEffects.cpp:3950-3971):
// SPELL_AURA_MELEE_SLOW (193) and SPELL_AURA_MOD_SPEED_SLOW_ALL (252) apply
// under GetHighestExclusiveSameEffectSpellGroupValue(this,
// SPELL_AURA_MELEE_SLOW) (Unit.cpp:13967) — the rival scan is type-193-only,
// so a 193 effect is gated by other 193 effects in a shared exclusive group
// while 252 effects never gate each other (they always stack). An effect is
// dormant when a rival's |amount| >= its own (ties lose, C++ :3957). The
// returned amounts feed the attack-time divisor (same m_modAttackSpeedPct
// accumulators as the 9/138/140/141/192 arms) and the cast-time multiplier.
func (s *session) combatSlowLiveAmounts() []int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	var cands []statCandidate
	auras := s.loadedAuras()
	for i, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraMeleeSlow && effect.Aura != spellAuraModSpeedSlowAll {
				continue
			}
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			cands = append(cands, statCandidate{
				order:       i,
				spellID:     aura.SpellID,
				effectIndex: index,
				amount:      aura.Amounts[index],
				firstRank:   s.server.spellFirstRank(aura.SpellID),
				auraType:    effect.Aura,
			})
		}
	}
	if len(cands) == 0 {
		return nil
	}
	cache := s.server.petAuraStackGroups()
	var live []int32
	for _, c := range cands {
		rivalMax := int32(0)
		for _, r := range cands {
			if r.auraType != spellAuraMeleeSlow {
				continue
			}
			if r.spellID == c.spellID && r.effectIndex == c.effectIndex && r.order == c.order {
				continue
			}
			if !sharesExclusiveSameEffectGroup(c.firstRank, r.firstRank, cache) {
				continue
			}
			if absAuraAmount(r.amount) > absAuraAmount(rivalMax) {
				rivalMax = r.amount
			}
		}
		if absAuraAmount(rivalMax) >= absAuraAmount(c.amount) {
			continue
		}
		live = append(live, c.amount)
	}
	return live
}

// combatSlowCastFactor mirrors the cast-time arm of
// AuraEffect::HandleModCombatSpeedPct (SpellAuraEffects.cpp:3962/3967) via
// Unit::ApplyCastTimePercentMod (Unit.cpp:10867): each live 193/252 effect
// multiplies UNIT_MOD_CAST_SPEED — positive amounts divide by (1+amount/100)
// (haste), negative amounts multiply by (1+|amount|/100) (slow) — composing
// multiplicatively with the rating-haste divisor in calculateSpellCastTime.
func (s *session) combatSlowCastFactor() float64 {
	factor := 1.0
	for _, a := range s.combatSlowLiveAmounts() {
		if a > 0 {
			factor /= 1.0 + float64(a)/100.0
		} else {
			factor *= 1.0 + float64(-a)/100.0
		}
	}
	return factor
}

// attackPowerOfStatFold mirrors the dynamic legs of
// Player::UpdateAttackPowerAndDamage (StatSystem.cpp:466-477):
// SPELL_AURA_MOD_ATTACK_POWER_OF_STAT_PERCENT (268) and
// SPELL_AURA_MOD_RANGED_ATTACK_POWER_OF_STAT_PERCENT (212) add
// CalculatePct(GetStat(misc), amount) to the flat attack-power mods
// (the same bucket as the 99/124 flat legs). GetStat is the fully
// derived stat — base*basePct + items + aura flat, times total pct —
// which is what s.player.Stats holds after the 29/80/137 fold in
// calculatePlayerStats. Negative/out-of-range misc values hit no stat
// in the DBC-driven handlers' data (C++ reads UNIT_FIELD_STAT0+stat
// unchecked); Go guards 0..4 like the other of-stat folds.
func (s *session) attackPowerOfStatFold(auraType uint32) int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total int32
	stats := s.player.Stats
	for _, ae := range s.auraTypeMiscAmounts(auraType) {
		if ae.misc < 0 || int(ae.misc) >= len(stats) {
			continue
		}
		total += int32(int64(stats[ae.misc]) * int64(ae.amount) / 100)
	}
	return total
}

// castingSpeedNotStackLiveAmounts mirrors the sub-1000 arm of
// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3888-3936):
// SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (65) and SPELL_AURA_HASTE_SPELLS
// (216) route through the same handler. Amounts >= 1000 take the
// instant-cast path (hasInstantCastAura) and never reach the percent arm.
// Each remaining effect is gated by
// GetHighestExclusiveSameEffectSpellGroupValue(this, GetAuraType())
// (Unit.cpp:13967): rivals are the same aura type only, and the effect goes
// dormant when a same-type rival in a shared exclusive group carries
// |amount| >= its own (ties lose, C++ :3931). The surviving amounts feed
// Unit::ApplyCastTimePercentMod (Unit.cpp:10867).
func (s *session) castingSpeedNotStackLiveAmounts() []int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	var cands []statCandidate
	auras := s.loadedAuras()
	for i, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraCastingSpeedNotStack && effect.Aura != spellAuraHasteSpells {
				continue
			}
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if aura.Amounts[index] >= 1000 {
				continue
			}
			cands = append(cands, statCandidate{
				order:       i,
				spellID:     aura.SpellID,
				effectIndex: index,
				amount:      aura.Amounts[index],
				firstRank:   s.server.spellFirstRank(aura.SpellID),
				auraType:    effect.Aura,
			})
		}
	}
	if len(cands) == 0 {
		return nil
	}
	cache := s.server.petAuraStackGroups()
	var live []int32
	for _, c := range cands {
		rivalMax := int32(0)
		for _, r := range cands {
			if r.auraType != c.auraType {
				continue
			}
			if r.spellID == c.spellID && r.effectIndex == c.effectIndex && r.order == c.order {
				continue
			}
			if !sharesExclusiveSameEffectGroup(c.firstRank, r.firstRank, cache) {
				continue
			}
			if absAuraAmount(r.amount) > absAuraAmount(rivalMax) {
				rivalMax = r.amount
			}
		}
		if absAuraAmount(rivalMax) >= absAuraAmount(c.amount) {
			continue
		}
		live = append(live, c.amount)
	}
	return live
}

// castingSpeedCastFactor mirrors the cast-time arm of
// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3933/3935) via
// Unit::ApplyCastTimePercentMod (Unit.cpp:10867): each live 65/216 effect
// multiplies UNIT_MOD_CAST_SPEED — positive amounts divide by (1+amount/100)
// (haste), negative amounts multiply by (1+|amount|/100) (slow) — composing
// multiplicatively with the combat-slow factor and the rating-haste divisor
// in calculateSpellCastTime.
func (s *session) castingSpeedCastFactor() float64 {
	factor := 1.0
	for _, a := range s.castingSpeedNotStackLiveAmounts() {
		if a > 0 {
			factor /= 1.0 + float64(a)/100.0
		} else {
			factor *= 1.0 + float64(-a)/100.0
		}
	}
	return factor
}
