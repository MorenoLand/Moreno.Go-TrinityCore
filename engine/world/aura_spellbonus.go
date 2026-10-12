package world

// spellDamageOfStatFold mirrors the "damage bonus from stats" leg of
// Unit::SpellBaseDamageBonusDone (Unit.cpp:7149-7157): each live
// SPELL_AURA_MOD_SPELL_DAMAGE_OF_STAT_PERCENT (174) effect whose MiscValue
// school mask touches the spell's school adds
// CalculatePct(GetStat(Stats(miscValueB)), amount). The whole leg is
// player-only in C++ (GetTypeId() == TYPEID_PLAYER gate), and so is this.
func (s *session) spellDamageOfStatFold(schoolMask uint32) int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || schoolMask == 0 {
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
			if effect.Aura != spellAuraModSpellDamageOfStatPercent {
				continue
			}
			if uint32(effect.MiscValue)&schoolMask == 0 {
				continue
			}
			statIdx := effect.MiscValueB
			if statIdx < 0 || int(statIdx) >= len(s.player.Stats) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == spellAuraModSpellDamageOfStatPercent &&
				uint32(aura.MiscValue)&schoolMask != 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			// CalculatePct (Util.h): stat * amount / 100, truncating.
			total += int32(int64(s.player.Stats[statIdx]) * int64(amount) / 100)
		}
	}
	return total
}

// spellDamageOfAttackPowerFold mirrors the "and attack power" leg of
// Unit::SpellBaseDamageBonusDone (Unit.cpp:7161): CalculatePct of the caster's
// melee attack power by the summed school-gated
// SPELL_AURA_MOD_SPELL_DAMAGE_OF_ATTACK_POWER (237) aura amounts. Player-only
// like the whole C++ block.
func (s *session) spellDamageOfAttackPowerFold(schoolMask uint32) int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || schoolMask == 0 {
		return 0
	}
	var amountSum int32
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
			if effect.Aura != spellAuraModSpellDamageOfAttackPower {
				continue
			}
			if uint32(effect.MiscValue)&schoolMask == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == spellAuraModSpellDamageOfAttackPower &&
				uint32(aura.MiscValue)&schoolMask != 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			// C++ sums the amounts first (GetTotalAuraModifierByMiscMask) and
			// takes one CalculatePct against total attack power.
			amountSum += amount
		}
	}
	if amountSum == 0 {
		return 0
	}
	return int32(int64(s.totalAttackPowerValue(false)) * int64(amountSum) / 100)
}

// spellHealingOfStatFold mirrors the "healing bonus from stats" leg of
// Unit::SpellBaseHealingBonusDone (Unit.cpp:7783-7789): each live
// SPELL_AURA_MOD_SPELL_HEALING_OF_STAT_PERCENT (175) effect adds
// CalculatePct(GetStat(stat), amount) with NO school gate — and unlike the
// damage twin (174), the stat comes from MiscValue (the spell effect's own
// MiscValue, Unit.cpp:7786), not MiscValueB. Player-only like the C++ block.
func (s *session) spellHealingOfStatFold() int32 {
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
			if effect.Aura != spellAuraModSpellHealingOfStatPercent {
				continue
			}
			statIdx := effect.MiscValue
			if statIdx < 0 || int(statIdx) >= len(s.player.Stats) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == spellAuraModSpellHealingOfStatPercent {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			total += int32(int64(s.player.Stats[statIdx]) * int64(amount) / 100)
		}
	}
	return total
}

// spellHealingOfAttackPowerFold mirrors the "and attack power" leg of
// Unit::SpellBaseHealingBonusDone (Unit.cpp:7792-7795): each live
// SPELL_AURA_MOD_SPELL_HEALING_OF_ATTACK_POWER (238) effect whose MiscValue
// school mask touches the spell's school adds
// CalculatePct(GetTotalAttackPowerValue(BASE_ATTACK), amount). Player-only
// like the C++ block.
func (s *session) spellHealingOfAttackPowerFold(schoolMask uint32) int32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || schoolMask == 0 {
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
			if effect.Aura != spellAuraModSpellHealingOfAttackPower {
				continue
			}
			if uint32(effect.MiscValue)&schoolMask == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == spellAuraModSpellHealingOfAttackPower &&
				uint32(aura.MiscValue)&schoolMask != 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			total += int32(int64(s.totalAttackPowerValue(false)) * int64(amount) / 100)
		}
	}
	return total
}
