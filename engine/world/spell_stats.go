package world

import (
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Combat rating indices for spells matching TrinityCore SharedDefines.h:1760-1790
const (
	CombatRatingHitSpell   uint8 = 7  // CR_HIT_SPELL
	CombatRatingCritSpell  uint8 = 10 // CR_CRIT_SPELL
	CombatRatingHasteSpell uint8 = 19 // CR_HASTE_SPELL
)

// getSpellHastePct returns the spell haste percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HASTE_SPELL) (Player.cpp:8800).
func (s *session) getSpellHastePct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHasteSpell])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% spell haste (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getSpellHitPct returns the bonus spell hit percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HIT_SPELL) (Player.cpp:8800).
func (s *session) getSpellHitPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHitSpell])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 26.231995 rating = 1.0% spell hit (from gtCombatRatings.dbc)
	ratingPerPct := 26.231995 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// calculateSpellHitChance calculates the chance [0.0, 1.0] for a spell to hit the target,
// based on caster level, target level, whether target is a player or creature, and caster's Spell Hit Rating.
// Formula mirrors TrinityCore Unit::MagicSpellHitResult (Unit.cpp:2470-2520).
func (s *session) calculateSpellHitChance(targetLevel uint8, isTargetPlayer bool) float64 {
	if s == nil || s.player == nil {
		return 1.0
	}
	casterLevel := s.player.Level
	if casterLevel == 0 {
		casterLevel = 80
	}
	if targetLevel == 0 {
		targetLevel = casterLevel
	}

	levelDiff := int(targetLevel) - int(casterLevel)
	hitChance := 100.0

	if isTargetPlayer {
		if levelDiff <= 0 {
			hitChance = 96.0 // 4% base miss in PvP
		} else if levelDiff == 1 {
			hitChance = 95.0 // 5% miss
		} else if levelDiff == 2 {
			hitChance = 94.0 // 6% miss
		} else {
			hitChance = 94.0 - float64(levelDiff-2)*7.0
		}
	} else {
		// PvE against creatures
		if levelDiff <= 0 {
			hitChance = 96.0 // 4% base miss
		} else if levelDiff == 1 {
			hitChance = 95.0 // 5% miss
		} else if levelDiff == 2 {
			hitChance = 94.0 // 6% miss
		} else {
			// Level 83 boss against Level 80 player: 83% hit chance (17% miss)
			hitChance = 83.0 - float64(levelDiff-3)*11.0
		}
	}

	// Add spell hit percentage from gear rating (CombatRatingHitSpell = 7)
	hitChance += s.getSpellHitPct()

	if hitChance > 100.0 {
		hitChance = 100.0
	}
	if hitChance < 1.0 {
		hitChance = 1.0
	}

	return hitChance / 100.0
}

// rollSpellHit rolls whether a spell hits or misses the target.
func (s *session) rollSpellHit(targetLevel uint8, isTargetPlayer bool) bool {
	chance := s.calculateSpellHitChance(targetLevel, isTargetPlayer)
	return rand.Float64() < chance
}

// calculateSpellCastTime mirrors TrinityCore SpellInfo::CalcCastTime
// (SpellInfo.cpp:3091) plus WorldObject::ModSpellCastTime (Object.cpp:2448):
// the SpellCastTimes.dbc base is multiplied by the cast-speed modifier
// (UNIT_MOD_CAST_SPEED = 1/(1+hastePct/100), fed by CR_HASTE_SPELL rating
// via Player::ApplyCastTimePercentMod — Player.cpp:5621), then non-auto-repeat
// ranged spells with ATTR0_REQ_AMMO gain +500ms (SpellInfo.cpp:3102).
// Haste skips ATTR0_ABILITY / ATTR0_TRADESPELL / ATTR3_NO_DONE_BONUS spells
// and, for players, spells with no spell family name (Object.cpp:2462-2468).
// Noted gaps (not stubs): SPELLMOD_CASTING_TIME has no spellmod infra in Go;
// CanInstantCast (SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK amount >= 1000,
// SpellAuraEffects.cpp:3904) has no cast-speed aura infra; the ranged-attack-speed
// branch (m_modAttackSpeedPct[RANGED_ATTACK], Object.cpp:2470) has no Go
// ranged-haste infra, so that branch is a no-op here.
func (s *session) calculateSpellCastTime(spell wotlk.Spell) uint32 {
	baseCastTime := int32(0)
	if s.server != nil && s.server.Data != nil && spell.CastingTimeIndex > 0 {
		if value, ok, err := s.server.Data.SpellCastTime(spell.CastingTimeIndex); err == nil && ok && value > 0 {
			baseCastTime = int32(value)
		}
	}
	if baseCastTime == 0 {
		return 0
	}
	castTime := baseCastTime
	reqAmmo := spell.Attributes&spellAttr0ReqAmmo != 0 && spell.AttributesEx1&spellAttr2AutorepeatFlag == 0
	switch {
	case s.player != nil && spell.Attributes&(spellAttr0Ability|spellAttr0Tradespell|spellAttr3NoDoneBonus) == 0 && spell.SpellFamilyName != 0:
		if hastePct := s.getSpellHastePct(); hastePct > 0 {
			castTime = int32(float64(castTime) / (1.0 + hastePct/100.0))
		}
	case reqAmmo:
		// Ranged-attack-speed branch (Object.cpp:2470) — no Go ranged-haste infra, no-op.
	case len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3881 && s.hasAura(67556):
		castTime = 500 // cooking with Chef Hat (Object.cpp:2471)
	}
	if reqAmmo {
		castTime += 500
	}
	if castTime < 0 {
		return 0
	}
	return uint32(castTime)
}

// calculateSpellCritChance resolves the probability [0.0, 1.0] of a spell critical strike.
// Mirrors TrinityCore Player::UpdateSpellCritChance (StatSystem.cpp:819):
// school 0 (SPELL_SCHOOL_NORMAL) is zeroed; other schools sum the Gt-table
// Intellect contribution (Player::GetSpellCritFromIntellect, Player.cpp:5502),
// SPELL_AURA_MOD_SPELL_CRIT_CHANCE (57) and SPELL_AURA_MOD_CRIT_PCT (290),
// the school-masked SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL (71) term, and the
// CR_CRIT_SPELL rating bonus.
func (s *session) calculateSpellCritChance(targetGUID uint64, schoolMask uint8) float64 {
	if s == nil || s.player == nil {
		return 0.05
	}

	// Physical schools never crit via spell crit.
	if schoolMask == 1 {
		return 0
	}

	// 1. Intellect contribution from the Gt tables (includes the class base
	// crit from gtChanceToSpellCritBase.dbc): crit = critBase +
	// GetStat(STAT_INTELLECT)*critRatio, *100.
	critPct := s.getSpellCritFromIntellect(s.player)

	// 2. Flat aura bonuses.
	critPct += float64(s.playerAuraModifier(spellAuraModSpellCritChance))
	critPct += float64(s.playerAuraModifier(spellAuraModCritPct))

	// 3. School-specific aura bonus (Unit.cpp:4937: effect MiscValue & school mask).
	critPct += float64(s.playerAuraModifierByMiscMask(spellAuraModSpellCritChanceSchool, int32(schoolMask)))

	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}

	// 4. Spell Crit Rating (CR_CRIT_SPELL = 10): 45.905987 rating per 1.0% crit at level 80
	rating := float64(s.player.CombatRatings[CombatRatingCritSpell])
	if rating > 0 {
		ratingPerPct := 45.905987 * (lvl / 80.0)
		if ratingPerPct < 5.0 {
			ratingPerPct = 5.0
		}
		critPct += rating / ratingPerPct
	}

	// 5. Defender resilience reduction (in PvP)
	if targetGUID != 0 && targetGUID != s.playerGUID && s.server != nil {
		if vicSess := s.server.findSessionByGUID(targetGUID); vicSess != nil {
			critBP := int32(math.Round(critPct * 100))
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenSpell, &critBP)
			critPct = float64(critBP) / 100.0
		}
	}

	if critPct < 0 {
		critPct = 0
	}
	if critPct > 100 {
		critPct = 100
	}

	return critPct / 100.0
}

// playerAuraModifierByMiscMask sums the active amounts of auras of the given
// type whose MiscValue overlaps miscMask.
// Mirrors TrinityCore Unit::GetTotalAuraModifierByMiscMask (Unit.cpp:4937):
// (effect MiscValue & miscMask) != 0.
func (s *session) playerAuraModifierByMiscMask(auraType uint32, miscMask int32) float32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total float32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraType {
				continue
			}
			if effect.MiscValue&miscMask == 0 {
				continue
			}
			total += float32(aura.Amounts[index])
		}
	}
	return total
}

// rollSpellCrit rolls whether the spell achieves a critical strike.
func (s *session) rollSpellCrit(targetGUID uint64, schoolMask uint8) bool {
	chance := s.calculateSpellCritChance(targetGUID, schoolMask)
	return rand.Float64() < chance
}

// getSpellCritMultiplier calculates the critical strike damage/healing multiplier for a spell,
// factoring in the base 150% multiplier, talents/auras modifying critical bonus (AuraType 182),
// and metagem modifiers (+3% crit damage).
// Mirrors TrinityCore Unit::SpellCriticalDamageBonus (Unit.cpp:1650-1700).
func (s *session) getSpellCritMultiplier(spell wotlk.Spell) float64 {
	baseBonusPct := 50.0 // 1.5x base multiplier (1.0 + 50/100)
	extraBonusPct := 0.0
	metaBonusPct := 0.0

	if s == nil || s.player == nil {
		return 1.5
	}

	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped {
			continue
		}
		// SPELL_AURA_MOD_CRIT_DAMAGE_BONUS = 182
		if aura.AuraType == 182 {
			if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
				extraBonusPct += float64(aura.Amount)
			}
		}
		// Chaotic Skyflare Diamond / Relentless Earthsiege Diamond (3% increased critical damage)
		if aura.SpellID == 26297 || aura.SpellID == 44795 || aura.SpellID == 55341 || aura.SpellID == 28557 {
			metaBonusPct += 3.0
		}
	}
	s.castMu.Unlock()

	// Check learned talent spells:
	// Ruin (Warlock 17959): 100% extra bonus for Destruction (Fire 4 / Shadow 32) spells
	// Elemental Fury (Shaman 16089): 100% extra bonus for Fire 4 / Nature 8 / Frost 16 spells
	// Ice Shards (Mage 15058): up to 100% extra bonus for Frost 16 spells
	// Spell Power (Mage 35581): 25%/50% extra bonus
	if s.hasActiveSpell(17959) && (spell.SchoolMask&4 != 0 || spell.SchoolMask&32 != 0) { // Ruin
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(16089) && (spell.SchoolMask&4 != 0 || spell.SchoolMask&8 != 0 || spell.SchoolMask&16 != 0) { // Elemental Fury
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(15058) && (spell.SchoolMask&16 != 0) { // Ice Shards Rank 3
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(35581) { // Spell Power Rank 2
		extraBonusPct += 25.0
	}

	multiplier := 1.0 + (baseBonusPct+extraBonusPct)/100.0
	if metaBonusPct > 0 {
		multiplier *= (1.0 + metaBonusPct/100.0)
	}
	return multiplier
}
