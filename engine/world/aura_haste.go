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
