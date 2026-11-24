package world

// getCreatePowerValue is Unit::GetCreatePowerValue (Unit.cpp:9679-9701):
// the create-time max for a power type. Only hunter pets have FOCUS
// (100) and HAPPINESS (1050000); MANA reads the base-mana value
// (UNIT_FIELD_BASE_MANA); RAGE/RUNIC_POWER 1000, ENERGY 100, RUNE/HEALTH 0.
func getCreatePowerValue(power uint32, hunterPet bool, baseMana uint32) uint32 {
	switch power {
	case 0: // POWER_MANA
		return baseMana
	case 1: // POWER_RAGE
		return 1000
	case 2: // POWER_FOCUS
		if hunterPet {
			return 100
		}
		return 0
	case 3: // POWER_ENERGY
		return 100
	case 4: // POWER_HAPPINESS
		if hunterPet {
			return 1050000
		}
		return 0
	case 6: // POWER_RUNIC_POWER
		return 1000
	default: // POWER_RUNE, POWER_HEALTH, anything else
		return 0
	}
}

// creatureSetHealthGuard is the death-state arm of Unit::SetHealth
// (Unit.cpp:9556-9560): a JUST_DIED/CORPSE creature's health is forced
// to 0 whatever value SetHealth is asked for; otherwise the value passes
// through (the max clamp is the caller's job).
func creatureSetHealthGuard(deathState uint8, val uint32) uint32 {
	if deathState == creatureDeathJustDied || deathState == creatureDeathCorpse {
		return 0
	}
	return val
}

// playerSetHealthGuard is the death-state arm of Unit::SetHealth
// (Unit.cpp:9556-9562) for players: a DEAD player (ghost) is forced to
// 1, a JUST_DIED/CORPSE player (health 0, no ghost flag) to 0.
func playerSetHealthGuard(health uint32, ghost bool, val uint32) uint32 {
	if health == 0 {
		if ghost {
			return 1
		}
		return 0
	}
	return val
}
