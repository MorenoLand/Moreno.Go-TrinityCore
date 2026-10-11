package world

// Aura-driven combat-rating arms: AuraEffect::HandleModRating
// (SpellAuraEffects.cpp:4030 — aura 189) and the SPELL_AURA_MOD_RATING_FROM_STAT
// leg of Player::UpdateRating (Player.cpp:5631-5636 — aura 220), plus the
// SPELL_AURA_MOD_RESISTANCE_OF_STAT_PERCENT leg of Player::UpdateArmor
// (StatSystem.cpp:270-274 — aura 182).
//
// C++ keeps the flat gear rating in Player::m_baseRatingValue; ApplyRatingMod
// (Player.cpp:5596-5626) adds/subtracts the 189 amount per rating bit and then
// calls UpdateRating, which adds the 220 stat-pct bonus
// (CalculatePct(GetStat(Stats(miscValueB)), amount)) per rating bit), clamps at
// 0, and broadcasts PLAYER_FIELD_COMBAT_RATING_1+cr. UpdateRating then recascades
// into UpdateDodgePercentage / UpdateParryPercentage / UpdateBlockPercentage /
// UpdateMeleeHitChance / UpdateCritPercentage / ApplyAttackTimePercentMod /
// ApplyCastTimePercentMod / UpdateExpertise / UpdateArmorPenetration — Go
// reads the ratings live at every consumer (melee_stats.go, spell_stats.go,
// player_state.go broadcasts), so materializing the folds into
// state.CombatRatings on each calculatePlayerStats rebuild reproduces the whole
// cascade: gear base (m_baseRatingValue) + 189 flat + 220 stat-pct, clamped at 0,
// exactly what C++ broadcasts. The haste legs of ApplyRatingMod
// (attack/cast-time pct mods) ride the same broadcast value — Go's haste
// consumers read the rating directly, so no extra leg is needed. All three
// handlers are player-only in C++ (189/220 return early for non-players; the
// 182 loop lives in Player::UpdateArmor), and Go has no creature rating/stat
// model, so the folds stay on the player path.

// auraRatingFlatFold sums live SPELL_AURA_MOD_RATING (189) amounts per rating
// index: GetMiscValue() & (1 << rating) selects the rating (HandleModRating,
// SpellAuraEffects.cpp:4037-4039). Amounts sum signed; UpdateRating clamps the
// total at 0 (Player.cpp:5638-5639), applied in applyAuraRatingFolds.
func (s *session) auraRatingFlatFold() (flat [25]int32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return flat
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
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
			if effect.Aura != spellAuraModRating {
				continue
			}
			misc := effect.MiscValue
			for rating := 0; rating < len(flat); rating++ {
				if misc&(1<<uint(rating)) != 0 {
					flat[rating] += aura.Amounts[index]
				}
			}
		}
	}
	return flat
}

// auraRatingFromStatFold mirrors the MOD_RATING_FROM_STAT leg of
// Player::UpdateRating (Player.cpp:5631-5636): each live 220 effect adds
// CalculatePct(GetStat(Stats(miscValueB)), amount) to every rating whose
// MiscValue bit is set. GetStat is the fully derived stat — base*basePct +
// items + aura flat, times total pct — which is what state.Stats holds after
// the 29/80/137 fold in calculatePlayerStats; the fold runs after that point.
func (s *session) auraRatingFromStatFold(stats [5]uint32) (bonus [25]int32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return bonus
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
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
			if effect.Aura != spellAuraModRatingFromStat {
				continue
			}
			statIdx := effect.MiscValueB
			if statIdx < 0 || int(statIdx) >= len(stats) {
				continue
			}
			// CalculatePct (Util.h): stat * amount / 100, truncating.
			add := int32(int64(stats[statIdx]) * int64(aura.Amounts[index]) / 100)
			misc := effect.MiscValue
			for rating := 0; rating < len(bonus); rating++ {
				if misc&(1<<uint(rating)) != 0 {
					bonus[rating] += add
				}
			}
		}
	}
	return bonus
}

// applyAuraRatingFolds materializes the 189/220 folds into state.CombatRatings.
// Call after the gear ratings and the stat fold are final in
// calculatePlayerStats: effective = gear + 189 flat + 220 stat-pct, clamped at
// 0 like Player::UpdateRating (Player.cpp:5638-5639). Recalc runs on every
// aura apply/merge/remove, so the broadcast (PLAYER_FIELD_COMBAT_RATING_1+i)
// and every consumer (dodge/parry/block/hit/crit/haste/expertise/armorpen)
// always see the C++ value.
func (s *session) applyAuraRatingFolds(state *playerState) {
	if s == nil || state == nil {
		return
	}
	flat := s.auraRatingFlatFold()
	fromStat := s.auraRatingFromStatFold(state.Stats)
	for r := range state.CombatRatings {
		v := int64(state.CombatRatings[r]) + int64(flat[r]) + int64(fromStat[r])
		if v < 0 {
			v = 0
		}
		state.CombatRatings[r] = uint32(v)
	}
}

// auraArmorOfStatPercentFold mirrors the dynamic flat leg of Player::UpdateArmor
// (StatSystem.cpp:270-274): each live SPELL_AURA_MOD_RESISTANCE_OF_STAT_PERCENT
// (182) effect with MiscValue & SPELL_SCHOOL_MASK_NORMAL adds
// CalculatePct(GetStat(Stats(miscValueB)), amount) to armor, before the
// TOTAL_PCT multiplier. Non-NORMAL misc auras log an error in C++ and apply
// nothing (the handler at SpellAuraEffects.cpp:3584-3590 returns early), so
// they are skipped here too. GetStat is the fully derived stat, same as the
// 220 fold.
func (s *session) auraArmorOfStatPercentFold(stats [5]uint32) int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total int32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
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
			if effect.Aura != spellAuraModResistanceOfStatPercent {
				continue
			}
			if effect.MiscValue&int32(spellSchoolMaskNormal) == 0 {
				continue
			}
			statIdx := effect.MiscValueB
			if statIdx < 0 || int(statIdx) >= len(stats) {
				continue
			}
			total += int32(int64(stats[statIdx]) * int64(aura.Amounts[index]) / 100)
		}
	}
	return total
}
