package world

import "math"

// octRegenHPPerSpiritFormula mirrors the formula of Player::OCTRegenHPPerSpirit
// (Player.cpp:5562-5574): baseSpirit = min(spirit, 50), moreSpirit = spirit -
// baseSpirit, regen = baseSpirit*baseRatio + moreSpirit*moreRatio. The DBC
// lookups ((class-1)*GT_MAX_LEVEL + level-1 of GtOCTRegenHP and
// GtRegenHPPerSpt) are done by the Server method below.
func octRegenHPPerSpiritFormula(spirit float64, baseRatio, moreRatio float32) float64 {
	baseSpirit := spirit
	if baseSpirit > 50 {
		baseSpirit = 50
	}
	moreSpirit := spirit - baseSpirit
	return baseSpirit*float64(baseRatio) + moreSpirit*float64(moreRatio)
}

// octRegenHPPerSpirit mirrors Player::OCTRegenHPPerSpirit (Player.cpp:5549-5574).
// Missing DBC entries (or the store) yield 0.0, as in C++.
func (s *Server) octRegenHPPerSpirit(classID, level uint32, spirit float64) float64 {
	if s == nil || s.Data == nil {
		return 0
	}
	baseRatio, okBase, _ := s.Data.GtOCTRegenHP(classID, level)
	moreRatio, okMore, _ := s.Data.GtRegenHPPerSpt(classID, level)
	if !okBase || !okMore {
		return 0
	}
	return octRegenHPPerSpiritFormula(spirit, baseRatio, moreRatio)
}

// octRegenMPPerSpt mirrors Player::OCTRegenMPPerSpirit (Player.cpp:5576-5594):
// regen = spirit * GtRegenMPPerSpt[(class-1)*GT_MAX_LEVEL + level-1]. The
// GtOCTRegenMP line is commented out in C++, so it stays unmodeled here too.
// Missing entries (or the store) yield 0.0, as in C++.
func (s *Server) octRegenMPPerSpt(classID, level uint32, spirit float64) float64 {
	if s == nil || s.Data == nil {
		return 0
	}
	moreRatio, ok, _ := s.Data.GtRegenMPPerSpt(classID, level)
	if !ok {
		return 0
	}
	return spirit * float64(moreRatio)
}

// octRegenLowLevelMultiplier mirrors the level<15 multiplier applied to the
// regen rates in Player::RegenerateHealth and Player::Regenerate(POWER_MANA)
// (Player.cpp:2239-2241, 2135-2136): rate * (2.066 - level*0.066), with the
// default rate of 1.0 since Go has no RATE_HEALTH/RATE_POWER_MANA config.
func octRegenLowLevelMultiplier(level uint8) float64 {
	if level < 15 {
		return 2.066 - float64(level)*0.066
	}
	return 1
}

// manaRegenTickGain mirrors the spirit term of the 2s POWER_MANA regen tick
// (Player.cpp:2130-2144: addValue = UNIT_FIELD_POWER_REGEN_FLAT_MODIFIER *
// ManaIncreaseRate * 0.001 * m_regenTimer): with m_regenTimer = 2000ms the
// factor is 2.0, and the flat modifier's spirit term is sqrt(Intellect) *
// OCTRegenMPPerSpirit() (StatSystem.cpp:916). The mp5 aura/base terms are
// unmodeled bigger-unit slices.
func (s *Server) manaRegenTickGain(classID uint32, level uint8, intellect, spirit uint32) uint32 {
	return uint32(math.Sqrt(float64(intellect)) * s.octRegenMPPerSpt(classID, uint32(level), float64(spirit)) * octRegenLowLevelMultiplier(level) * 2.0)
}
