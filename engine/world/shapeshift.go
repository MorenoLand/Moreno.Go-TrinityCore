package world

import "github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"

func playerPowerType(state *playerState) uint8 {
	if state == nil {
		return 0
	}
	switch state.ShapeshiftForm {
	case 1, 7:
		return 3
	case 5, 8, 17, 18, 19:
		return 1
	default:
		return classPowerType(state.Class)
	}
}

func updatePlayerShapeshiftForm(state *playerState, data *wotlk.Store, form uint8, spellID uint32) uint32 {
	if state == nil {
		return 0
	}
	if state.ShapeshiftForm == 0 && form != 0 {
		state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime = state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime
	}
	if form == 0 {
		if state.ShapeshiftForm != 0 {
			state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime = state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime
			state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime = 0, 0, 0
		}
		state.ShapeshiftForm = 0
		return 0
	}
	state.ShapeshiftForm = form
	if data != nil {
		if shape, found, err := data.ShapeshiftForm(uint32(form)); err == nil && found && shape.CombatRoundTime != 0 {
			state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime = shape.CombatRoundTime, shape.CombatRoundTime, 2000
		}
	}
	return shapeshiftFormDisplayID(data, state, form, spellID)
}

func shapeshiftFormDisplayID(data *wotlk.Store, state *playerState, form uint8, spellID uint32) uint32 {
	if state == nil {
		return 0
	}
	switch spellID {
	case 7090:
		return 29414
	case 35200:
		return 4877
	}
	if form == 1 {
		if state.Race == 4 {
			switch state.HairColor {
			case 7, 8:
				return 29405
			case 3:
				return 29406
			case 0, 1, 2:
				return 29407
			case 4:
				return 29408
			default:
				return 892
			}
		}
		if state.Race == 6 {
			if state.Gender == 0 {
				switch state.Skin {
				case 12, 13, 14, 18:
					return 29409
				case 9, 10, 11:
					return 29410
				case 6, 7, 8:
					return 29411
				case 0, 1, 2, 3, 4, 5:
					return 29412
				default:
					return 8571
				}
			}
			switch state.Skin {
			case 10:
				return 29409
			case 6, 7:
				return 29410
			case 4, 5:
				return 29411
			case 0, 1, 2, 3:
				return 29412
			default:
				return 8571
			}
		}
		if isAllianceRace(state.Race) {
			return 892
		}
		return 8571
	}
	if form == 5 || form == 8 {
		if state.Race == 4 {
			switch state.HairColor {
			case 0, 1, 2:
				return 29413
			case 6:
				return 29414
			case 4:
				return 29416
			case 3:
				return 29417
			default:
				return 2281
			}
		}
		if state.Race == 6 {
			if state.Gender == 0 {
				switch state.Skin {
				case 0, 1, 2:
					return 29418
				case 3, 4, 5, 12, 13, 14:
					return 29419
				case 9, 10, 11, 15, 16, 17:
					return 29420
				case 18:
					return 29421
				default:
					return 2289
				}
			}
			switch state.Skin {
			case 0, 1:
				return 29418
			case 2, 3:
				return 29419
			case 6, 7, 8, 9:
				return 29420
			case 10:
				return 29421
			default:
				return 2289
			}
		}
		if isAllianceRace(state.Race) {
			return 2281
		}
		return 2289
	}
	if form == 29 {
		if isAllianceRace(state.Race) {
			return 20857
		}
		return 20872
	}
	if form == 27 {
		if isAllianceRace(state.Race) {
			return 21243
		}
		return 21244
	}
	if data == nil {
		return 0
	}
	shape, found, err := data.ShapeshiftForm(uint32(form))
	if err != nil || !found {
		return 0
	}
	if isAllianceRace(state.Race) || shape.CreatureDisplayIDs[1] == 0 {
		return shape.CreatureDisplayIDs[0]
	}
	return shape.CreatureDisplayIDs[1]
}
