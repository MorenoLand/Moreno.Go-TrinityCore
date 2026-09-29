package world

import (
	"math"
	"time"
)

// Combat rating indices for melee/ranged matching TrinityCore SharedDefines.h:1760-1790
const (
	CombatRatingDefenseSkill     uint8 = 1  // CR_DEFENSE_SKILL
	CombatRatingDodge            uint8 = 2  // CR_DODGE
	CombatRatingParry            uint8 = 3  // CR_PARRY
	CombatRatingBlock            uint8 = 4  // CR_BLOCK
	CombatRatingHitMelee         uint8 = 5  // CR_HIT_MELEE
	CombatRatingHitRanged        uint8 = 6  // CR_HIT_RANGED
	CombatRatingCritMelee        uint8 = 8  // CR_CRIT_MELEE
	CombatRatingCritRanged       uint8 = 9  // CR_CRIT_RANGED
	CombatRatingHasteMelee       uint8 = 17 // CR_HASTE_MELEE
	CombatRatingHasteRanged      uint8 = 18 // CR_HASTE_RANGED
	CombatRatingExpertise        uint8 = 23 // CR_EXPERTISE
	CombatRatingArmorPenetration uint8 = 24 // CR_ARMOR_PENETRATION
)

// getMeleeHitPct returns the bonus melee hit percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HIT_MELEE) (Player.cpp:8800).
func (s *session) getMeleeHitPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHitMelee])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% hit (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getRangedHitPct returns the bonus ranged hit percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HIT_RANGED) (Player.cpp:8800).
func (s *session) getRangedHitPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHitRanged])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% hit (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getMeleeCritFromAgility returns the melee crit percentage contributed by Agility.
// Mirrors TrinityCore Player::GetMeleeCritFromAgility (Player.cpp:5432):
// crit = critBase + GetStat(STAT_AGILITY)*critRatio, *100, using the
// gtChanceToMeleeCritBase (index = class-1) and gtChanceToMeleeCrit
// (index = (class-1)*100 + level-1, level clamped to 100) tables.
func (s *session) getMeleeCritFromAgility() float64 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	classID := uint32(s.player.Class)
	level := uint32(s.player.Level)
	if level > 100 {
		level = 100
	}
	base, found, err := s.server.Data.GtChanceToMeleeCritBase(classID)
	if err != nil || !found {
		return 0
	}
	ratio, found, err := s.server.Data.GtChanceToMeleeCrit(classID, level)
	if err != nil || !found {
		return 0
	}
	agi := float64(s.player.Stats[1]) // STAT_AGILITY = 1
	return (float64(base) + agi*float64(ratio)) * 100.0
}

// dodgeBaseByClass and critToDodgeByClass are the per-class tables from
// Player::GetDodgeFromAgility (Player.cpp:5451-5482). The 12th slot is the
// unused class index (C++ MAX_CLASSES=12, SharedDefines.h:143).
var dodgeBaseByClass = [...]float64{
	0.036640, 0.034943, -0.040873, 0.020957, 0.034178, 0.036640,
	0.021080, 0.036587, 0.024211, 0, 0.056097, 0,
}
var critToDodgeByClass = [...]float64{
	0.85 / 1.15, 1 / 1.15, 1.11 / 1.15, 2 / 1.15, 1 / 1.15, 0.85 / 1.15,
	1.60 / 1.15, 1 / 1.15, 0.97 / 1.15, 0, 2 / 1.15, 0,
}

// getDodgeFromAgility returns the diminishing (gear agility) and
// non-diminishing (base) dodge percentage contributions from Agility.
// Mirrors TrinityCore Player::GetDodgeFromAgility (Player.cpp:5449):
// dodgeRatio = GtChanceToMeleeCrit at (class-1)*100 + level-1 with level
// clamped to GT_MAX_LEVEL; base_agility = GetCreateStat(STAT_AGILITY) *
// GetPctModifierValue(UNIT_MOD_STAT_START + STAT_AGILITY, BASE_PCT).
// Go stores the create stats in BaseStats (same player_levelstats source as
// C++ GetCreateStat); Go has no aura pct-modifier group
// (C++ m_auraPctModifiersGroup, Unit.cpp:9290), and the C++ BASE_PCT
// multiplier initializes to 1.0, changing only under specific stat auras
// (SpellAuraEffects.cpp:3436), so base_agility = BaseStats[1].
func (s *session) getDodgeFromAgility(state *playerState) (diminishing, nondiminishing float64) {
	if s == nil || s.server == nil || s.server.Data == nil || state == nil {
		return 0, 0
	}
	classID := uint32(state.Class)
	level := uint32(state.Level)
	if level > 100 {
		level = 100
	}
	if classID == 0 || classID > 12 {
		return 0, 0
	}
	ratio, found, err := s.server.Data.GtChanceToMeleeCrit(classID, level)
	if err != nil || !found {
		return 0, 0
	}
	baseAgility := float64(state.BaseStats[1])
	bonusAgility := float64(state.Stats[1]) - baseAgility
	idx := classID - 1
	diminishing = 100.0 * bonusAgility * float64(ratio) * critToDodgeByClass[idx]
	nondiminishing = 100.0 * (dodgeBaseByClass[idx] + baseAgility*float64(ratio)*critToDodgeByClass[idx])
	return diminishing, nondiminishing
}

// getSpellCritFromIntellect returns the spell crit percentage contributed by Intellect.
// Mirrors TrinityCore Player::GetSpellCritFromIntellect (Player.cpp:5502):
// crit = critBase + GetStat(STAT_INTELLECT)*critRatio, *100, using the
// gtChanceToSpellCritBase (index = class-1) and gtChanceToSpellCrit
// (index = (class-1)*100 + level-1, level clamped to 100) tables; missing
// entries return 0 like the C++ nullptr check.
func (s *session) getSpellCritFromIntellect(state *playerState) float64 {
	if s == nil || s.server == nil || s.server.Data == nil || state == nil {
		return 0
	}
	classID := uint32(state.Class)
	level := uint32(state.Level)
	if level > 100 {
		level = 100
	}
	base, found, err := s.server.Data.GtChanceToSpellCritBase(classID)
	if err != nil || !found {
		return 0
	}
	ratio, found, err := s.server.Data.GtChanceToSpellCrit(classID, level)
	if err != nil || !found {
		return 0
	}
	intellect := float64(state.Stats[3]) // STAT_INTELLECT = 3
	return (float64(base) + intellect*float64(ratio)) * 100.0
}

// getMeleeCritPct returns the bonus melee crit percentage from Agility and gear rating.
// Mirrors TrinityCore Player::GetMeleeCritFromAgility and Player::GetRatingBonusValue(CR_CRIT_MELEE).
func (s *session) getMeleeCritPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}

	// 1. Agility contribution
	critPct := s.getMeleeCritFromAgility()

	// 2. Melee Crit Rating (CR_CRIT_MELEE = 8): 45.905987 rating per 1.0% crit at level 80
	rating := float64(s.player.CombatRatings[CombatRatingCritMelee])
	if rating > 0 {
		ratingPerPct := 45.905987 * (lvl / 80.0)
		if ratingPerPct < 5.0 {
			ratingPerPct = 5.0
		}
		critPct += rating / ratingPerPct
	}
	return critPct
}

// getRangedCritPct returns the bonus ranged crit percentage from Agility and gear rating.
// Mirrors TrinityCore Player::GetRangedCritFromAgility and Player::GetRatingBonusValue(CR_CRIT_RANGED).
func (s *session) getRangedCritPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}

	// 1. Agility contribution for ranged
	critPct := 0.0
	agi := float64(s.player.Stats[1])
	if agi > 0 {
		agiPerPct := 83.333333 * (lvl / 80.0)
		critPct += agi / agiPerPct
	}

	// 2. Ranged Crit Rating (CR_CRIT_RANGED = 9): 45.905987 rating per 1.0% crit at level 80
	rating := float64(s.player.CombatRatings[CombatRatingCritRanged])
	if rating > 0 {
		ratingPerPct := 45.905987 * (lvl / 80.0)
		if ratingPerPct < 5.0 {
			ratingPerPct = 5.0
		}
		critPct += rating / ratingPerPct
	}
	return critPct
}

// getMeleeHastePct returns the melee haste percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HASTE_MELEE) (Player.cpp:8800).
func (s *session) getMeleeHastePct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHasteMelee])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% haste (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getRangedHastePct returns the ranged haste percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HASTE_RANGED) (Player.cpp:8800).
func (s *session) getRangedHastePct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHasteRanged])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% haste (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getHastedMeleeSpeed modifies base attack speed by melee haste.
func (s *session) getHastedMeleeSpeed(baseSpeed time.Duration) time.Duration {
	hastePct := s.getMeleeHastePct()
	if hastePct <= 0 {
		return baseSpeed
	}
	hasted := float64(baseSpeed) / (1.0 + hastePct/100.0)
	if hasted < float64(200*time.Millisecond) {
		hasted = float64(200 * time.Millisecond)
	}
	return time.Duration(math.Round(hasted))
}

// getHastedRangedSpeed modifies base ranged attack speed by ranged haste.
func (s *session) getHastedRangedSpeed(baseSpeed time.Duration) time.Duration {
	hastePct := s.getRangedHastePct()
	if hastePct <= 0 {
		return baseSpeed
	}
	hasted := float64(baseSpeed) / (1.0 + hastePct/100.0)
	if hasted < float64(200*time.Millisecond) {
		hasted = float64(200 * time.Millisecond)
	}
	return time.Duration(math.Round(hasted))
}

// getExpertise returns the total expertise skill points from gear rating.
// Mirrors TrinityCore Player::GetExpertise (Player.cpp:8850).
func (s *session) getExpertise() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingExpertise])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 8.197496 rating = 1.0 expertise skill point (from gtCombatRatings.dbc)
	ratingPerPoint := 8.197496 * (lvl / 80.0)
	if ratingPerPoint < 1.0 {
		ratingPerPoint = 1.0
	}
	return rating / ratingPerPoint
}

// getExpertiseDodgeParryReductionPct returns the percentage reduction to defender dodge and parry chances.
// Each 1.0 point of expertise skill reduces target dodge and parry chance by 0.25% (25 basis points).
// Mirrors TrinityCore Player::GetExpertiseDodgeOrParryReduction (Player.cpp:8860).
func (s *session) getExpertiseDodgeParryReductionPct() float64 {
	return s.getExpertise() * 0.25
}

// getArmorPenPct returns the armor penetration percentage (0.0 to 100.0%) from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_ARMOR_PENETRATION) (Player.cpp:8800).
func (s *session) getArmorPenPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingArmorPenetration])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 15.3953 rating = 1.0% ArP (from gtCombatRatings.dbc in 3.3.5)
	ratingPerPct := 15.3953 * (lvl / 80.0)
	if ratingPerPct < 2.0 {
		ratingPerPct = 2.0
	}
	arpPct := rating / ratingPerPct
	if arpPct > 100.0 {
		arpPct = 100.0
	}
	return arpPct
}
