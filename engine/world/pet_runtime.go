package world

import "time"

const (
	petFocusRegenInterval = 4 * time.Second
	petHappinessInterval  = 7500 * time.Millisecond
)

type PetRuntimeState struct {
	PetType         uint8
	PowerType       uint32
	UnitFlags2      uint32
	Powers          [7]uint32
	MaxPowers       [7]uint32
	FocusRegenTimer time.Duration
	HappinessTimer  time.Duration
	InCombat        bool
}

func AdvancePetRuntime(state PetRuntimeState, diff time.Duration, focusRate float64) (PetRuntimeState, map[int]uint32) {
	if diff <= 0 {
		return state, nil
	}
	var fields map[int]uint32
	if state.FocusRegenTimer > 0 {
		if state.FocusRegenTimer > diff {
			state.FocusRegenTimer -= diff
		} else if state.PowerType == 2 {
			if state.UnitFlags2&unitFlag2RegeneratePower != 0 && state.Powers[2] < state.MaxPowers[2] {
				gain := int32(24 * focusRate)
				value := int64(state.Powers[2]) + int64(gain)
				if value < 0 {
					value = 0
				} else if value > int64(state.MaxPowers[2]) {
					value = int64(state.MaxPowers[2])
				}
				if uint32(value) != state.Powers[2] {
					state.Powers[2] = uint32(value)
					fields = map[int]uint32{unitFieldPower1 + 2: state.Powers[2]}
				}
			}
			state.FocusRegenTimer += petFocusRegenInterval - diff
			if state.FocusRegenTimer == 0 {
				state.FocusRegenTimer = time.Millisecond
			}
			if state.FocusRegenTimer < 0 || state.FocusRegenTimer > petFocusRegenInterval {
				state.FocusRegenTimer = petFocusRegenInterval
			}
		} else {
			state.FocusRegenTimer = 0
		}
	}
	if state.PetType == 1 {
		if state.HappinessTimer <= diff {
			loss := uint32(670)
			if state.InCombat {
				loss = uint32(float32(loss) * 1.5)
			}
			value := uint32(0)
			if state.Powers[4] > loss {
				value = state.Powers[4] - loss
			}
			if value != state.Powers[4] {
				state.Powers[4] = value
				if fields == nil {
					fields = make(map[int]uint32, 2)
				}
				fields[unitFieldPower1+4] = value
			}
			state.HappinessTimer = petHappinessInterval
		} else {
			state.HappinessTimer -= diff
		}
	}
	return state, fields
}

func (s *Server) updatePetRuntime(diff time.Duration) {
	if s == nil || diff <= 0 {
		return
	}
	type petUpdate struct {
		mapID  uint32
		guid   uint64
		fields map[int]uint32
	}
	updates := make([]petUpdate, 0)
	seen := make(map[*creatureMotion]struct{})
	s.motionMu.Lock()
	for _, motion := range s.creatureMotion {
		if motion == nil || motion.PetID == 0 || motion.OwnerGUID == 0 || motion.Health == 0 {
			continue
		}
		if _, ok := seen[motion]; ok {
			continue
		}
		seen[motion] = struct{}{}
		state := PetRuntimeState{PetType: motion.PetType, PowerType: motion.PowerType, UnitFlags2: motion.UnitFlags2, Powers: motion.Powers, MaxPowers: motion.MaxPowers, FocusRegenTimer: motion.FocusRegenTimer, HappinessTimer: motion.HappinessTimer, InCombat: motion.InCombat || motion.UnitFlags&unitFlagInCombat != 0}
		state, fields := AdvancePetRuntime(state, diff, s.Config.FocusRate)
		motion.Powers = state.Powers
		motion.FocusRegenTimer = state.FocusRegenTimer
		motion.HappinessTimer = state.HappinessTimer
		motion.Happiness = state.Powers[4]
		if len(fields) != 0 {
			updates = append(updates, petUpdate{mapID: motion.Map, guid: motion.GUID, fields: fields})
		}
	}
	s.motionMu.Unlock()
	for _, update := range updates {
		s.broadcastCreatureValuesUpdate(update.mapID, update.guid, update.fields)
	}
}
