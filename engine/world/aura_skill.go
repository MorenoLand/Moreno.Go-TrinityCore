package world

// This file bridges AuraEffect::HandleAuraModSkill
// (SpellAuraEffects.cpp:2559-2574): SPELL_AURA_MOD_SKILL (30) adds a
// temporary skill bonus and SPELL_AURA_MOD_SKILL_TALENT (98) a permanent
// one via Player::ModifySkillBonus (Player.cpp:6024-6041) — a no-op when
// the player hasn't learned the skill. The bonus folds into
// playerSkillTotalValue (Player::GetSkillValue, Player.cpp:6240-6255) and
// the PLAYER_SKILL_INFO bonus word, packed MAKE_SKILL_BONUS(t, p) style:
// temp bonus low, permanent high.
//
// Documented delta: the SKILL_DEFENSE (95) arm's UpdateDefenseBonusesMod
// leg (SpellAuraEffects.cpp:2572-2573) has no Go defense-bonus model, so
// defense-skill auras move the bonus but trigger no stat rebuild.
func (s *session) auraTouchesSkillBonus(aura *activeAura) bool {
	return aura != nil && (s.activeAuraHasEffect(aura, spellAuraModSkill) || s.activeAuraHasEffect(aura, spellAuraModSkillTalent))
}

func (s *session) playerHasSkill(skillID uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	for _, sk := range s.player.Skills {
		if uint32(sk.Skill) == skillID {
			return true
		}
	}
	return false
}

// applySkillBonusArms walks the aura record's masked effects; sign is +1
// on apply and -1 on remove, matching ModifySkillBonus's
// apply ? points : -points (SpellAuraEffects.cpp:2570).
func (s *session) applySkillBonusArms(aura *activeAura, sign int32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || aura == nil {
		return
	}
	sp, found, err := s.server.Data.Spell(aura.SpellID)
	if err != nil || !found {
		return
	}
	for index, effect := range sp.Effects {
		if aura.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		var talent bool
		switch effect.Aura {
		case spellAuraModSkill:
		case spellAuraModSkillTalent:
			talent = true
		default:
			continue
		}
		skillID := uint32(effect.MiscValue)
		if !s.playerHasSkill(skillID) {
			continue
		}
		var amount int32
		if index < len(aura.Amounts) {
			amount = aura.Amounts[index]
		}
		if talent {
			if s.skillTalentBonus == nil {
				s.skillTalentBonus = make(map[uint32]int32)
			}
			s.skillTalentBonus[skillID] += sign * amount
		} else {
			if s.skillTempBonus == nil {
				s.skillTempBonus = make(map[uint32]int32)
			}
			s.skillTempBonus[skillID] += sign * amount
		}
	}
}

// skillAuraBonus returns the accumulated temporary (30) and talent (98)
// bonuses for a skill line, or zero when the session is nil.
func (s *session) skillAuraBonus(skillID uint32) (temp, talent int32) {
	if s == nil {
		return 0, 0
	}
	return s.skillTempBonus[skillID], s.skillTalentBonus[skillID]
}

// skillBonusFieldValue packs the PLAYER_SKILL_INFO bonus word as C++
// MAKE_SKILL_BONUS(temp, perm) (Player.cpp:126): temp bonus in the low 16
// bits, permanent bonus in the high 16.
func skillBonusFieldValue(perm uint16, temp, talent int32) uint32 {
	return uint32(uint16(int32(perm)+talent))<<16 | uint32(uint16(temp))
}
