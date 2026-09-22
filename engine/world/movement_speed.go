package world

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellAuraIncreaseSpeed            uint32 = 31
	spellAuraIncreaseMountedSpeed     uint32 = 32
	spellAuraDecreaseSpeed            uint32 = 33
	spellAuraIncreaseSwimSpeed        uint32 = 58
	spellAuraMountedSpeedAlways       uint32 = 130
	spellAuraMountedSpeedNotStack     uint32 = 172
	spellAuraUseNormalMovementSpeed   uint32 = 191
	spellAuraIncreaseVehicleFlight    uint32 = 206
	spellAuraMountedFlightSpeed       uint32 = 207
	spellAuraIncreaseFlightSpeed      uint32 = 208
	spellAuraMountedFlightSpeedAlways uint32 = 209
	spellAuraVehicleSpeedAlways       uint32 = 210
	spellAuraFlightSpeedNotStack      uint32 = 211
	spellAuraMinimumSpeed             uint32 = 305
)

func (s *session) maxPositiveAuraModifier(auraType uint32) int32 {
	maxValue := int32(0)
	for _, amount := range s.auraTypeModifiers(auraType) {
		if amount > maxValue {
			maxValue = amount
		}
	}
	return maxValue
}

func (s *session) maxNegativeAuraModifier(auraType uint32) int32 {
	minValue := int32(0)
	for _, amount := range s.auraTypeModifiers(auraType) {
		if amount < minValue {
			minValue = amount
		}
	}
	return minValue
}

func (s *session) totalAuraModifier(auraType uint32) int32 {
	var total int32
	for _, amount := range s.auraTypeModifiers(auraType) {
		total += amount
	}
	return total
}

func (s *session) auraTypeModifiers(auraType uint32) []int32 {
	if s == nil {
		return nil
	}
	result := make([]int32, 0)
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		matched := false
		usedStoredAmount := false
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != auraType || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					amount := aura.Amounts[index]
					if amount == 0 && aura.AuraType == auraType && !usedStoredAmount {
						amount = int32(aura.Amount)
						usedStoredAmount = true
					}
					if amount == 0 {
						amount = effect.BasePoints + 1
					}
					result = append(result, amount)
					matched = true
				}
			}
		}
		if !matched && aura.AuraType == auraType {
			amount := int32(aura.Amount)
			if amount == 0 {
				for _, storedAmount := range aura.Amounts {
					if storedAmount != 0 {
						amount = storedAmount
						break
					}
				}
			}
			result = append(result, amount)
		}
	}
	return result
}

func (s *session) auraTypeModifiersByMiscMask(auraType, miscMask uint32) []int32 {
	if s == nil || miscMask == 0 {
		return nil
	}
	result := make([]int32, 0)
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		matched := false
		usedStoredAmount := false
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != auraType || aura.EffectMask&(1<<uint(index)) == 0 || uint32(effect.MiscValue)&miscMask == 0 {
						continue
					}
					amount := aura.Amounts[index]
					if amount == 0 && aura.AuraType == auraType && uint32(aura.MiscValue)&miscMask != 0 && !usedStoredAmount {
						amount = int32(aura.Amount)
						usedStoredAmount = true
					}
					if amount == 0 {
						amount = effect.BasePoints + 1
					}
					result = append(result, amount)
					matched = true
				}
			}
		}
		if !matched && aura.AuraType == auraType && uint32(aura.MiscValue)&miscMask != 0 {
			amount := int32(aura.Amount)
			if amount == 0 {
				for _, storedAmount := range aura.Amounts {
					if storedAmount != 0 {
						amount = storedAmount
						break
					}
				}
			}
			result = append(result, amount)
		}
	}
	return result
}

func (s *session) auraMultiplier(auraType uint32) float32 {
	multiplier := float32(1)
	for _, amount := range s.auraTypeModifiers(auraType) {
		multiplier *= 1 + float32(amount)/100
	}
	return multiplier
}

func (s *session) mountedRunSpeed() float32 {
	main := s.maxPositiveAuraModifier(spellAuraIncreaseSpeed)
	stack := s.auraMultiplier(129)
	nonStack := float32(1) + float32(s.maxPositiveAuraModifier(171))/100
	if s.hasAuraType(spellAuraMounted) || s.player.MountDisplayID != 0 {
		main = s.maxPositiveAuraModifier(spellAuraIncreaseMountedSpeed)
		stack = s.auraMultiplier(spellAuraMountedSpeedAlways)
		nonStack = float32(1) + float32(s.maxPositiveAuraModifier(spellAuraMountedSpeedNotStack))/100
	}
	multiplier := float32(math.Max(float64(stack), float64(nonStack)))
	if main != 0 {
		multiplier *= 1 + float32(main)/100
	}
	return s.adjustMovementSpeed(7 * multiplier)
}

func (s *session) mountedFlightSpeed() float32 {
	main := s.totalAuraModifier(spellAuraIncreaseFlightSpeed) + s.totalAuraModifier(spellAuraIncreaseVehicleFlight)
	stack, nonStack := float32(1), float32(1)
	if s.hasAuraType(spellAuraMounted) || s.player != nil && s.player.MountDisplayID != 0 {
		main = s.maxPositiveAuraModifier(spellAuraMountedFlightSpeed)
		stack = s.auraMultiplier(spellAuraMountedFlightSpeedAlways)
	}
	nonStack += float32(s.maxPositiveAuraModifier(spellAuraFlightSpeedNotStack)) / 100
	multiplier := float32(math.Max(float64(stack), float64(nonStack)))
	if main != 0 {
		multiplier *= 1 + float32(main)/100
	}
	return s.adjustMovementSpeed(7 * multiplier)
}

func (s *session) adjustMovementSpeed(speed float32) float32 {
	if normalization := float32(s.maxPositiveAuraModifier(spellAuraUseNormalMovementSpeed)); normalization > 0 && speed > normalization {
		speed = normalization
	}
	return ResolveMovementSpeed(speed, s.maxNegativeAuraModifier(spellAuraDecreaseSpeed), s.maxPositiveAuraModifier(spellAuraMinimumSpeed))
}

func ResolveMovementSpeed(speed float32, slowPercent, minimumPercent int32) float32 {
	speed *= 1 + float32(slowPercent)/100
	if minimum := float32(minimumPercent) / 100; speed < minimum {
		return minimum
	}
	return speed
}

func (s *session) movementSpeeds() [9]float32 {
	const walk, runBack, swim, swimBack, flightBack = float32(2.5), float32(4.5), float32(4.722222), float32(2.5), float32(4.5)
	swimSpeed := swim * (1 + float32(s.maxPositiveAuraModifier(spellAuraIncreaseSwimSpeed))/100)
	return [9]float32{walk, s.mountedRunSpeed(), s.adjustMovementSpeed(runBack), s.adjustMovementSpeed(swimSpeed), s.adjustMovementSpeed(swimBack), s.mountedFlightSpeed(), s.adjustMovementSpeed(flightBack), 3.141594, 3.14}
}

func (s *session) castSpeedMultiplier() float32 {
	amounts := make([]int32, 0)
	for _, auraType := range []uint32{spellAuraCastingSpeedNotStack, spellAuraHasteSpells} {
		amounts = append(amounts, s.auraTypeModifiers(auraType)...)
	}
	return ResolveCastSpeedMultiplier(amounts)
}

func (s *session) attackPowerAuraMultiplier() float32 {
	return ResolveAuraPercentMultiplier(s.auraTypeModifiers(spellAuraAttackPowerPercent))
}

func (s *session) rangedAttackPowerAuraMultiplier() float32 {
	return ResolveAuraPercentMultiplier(s.auraTypeModifiers(spellAuraRangedAttackPowerPercent))
}

func (s *session) damageDoneAuraMultiplier(school uint8) float32 {
	if school >= 7 {
		return 1
	}
	return ResolveAuraPercentMultiplier(s.auraTypeModifiersByMiscMask(spellAuraDamagePercentDone, 1<<school))
}

func ResolveCastSpeedMultiplier(amounts []int32) float32 {
	multiplier := float32(1)
	for _, amount := range amounts {
		if amount > 0 {
			multiplier /= 1 + float32(amount)/100
		} else if amount < 0 {
			multiplier *= 1 - float32(amount)/100
		}
	}
	return multiplier
}

func ResolveAuraPercentMultiplier(amounts []int32) float32 {
	multiplier := float32(1)
	for _, amount := range amounts {
		multiplier *= 1 + float32(amount)/100
	}
	return multiplier
}

func movementSpeedAura(auraType uint32) bool {
	switch auraType {
	case 31, 32, 33, 58, 78, 129, 130, 171, 172, 191, 201, 206, 207, 208, 209, 210, 211, 305:
		return true
	default:
		return false
	}
}

func AffectsMovementSpeedAura(auraType uint32) bool {
	return movementSpeedAura(auraType)
}

func (s *session) sendRuntimeMovementSpeed(opcode protocol.Opcode, nearbyOpcode protocol.Opcode, speed float32, run bool) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	self := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 9)
	self.WritePackedGUID(s.playerGUID)
	self.WriteU32(0)
	if run {
		self.WriteU8(1)
	}
	self.WriteF32(speed)
	if err := s.write(uint16(opcode), self.Bytes(), true); err != nil {
		return
	}
	info := s.movementInfoForCreate(*s.player)
	info.Time = uint32(time.Now().UnixMilli())
	nearby := protocol.NewBuffer(64)
	writeMovementInfo(nearby, info)
	nearby.WriteF32(speed)
	s.server.broadcastToNearby(uint16(nearbyOpcode), nearby.Bytes(), s)
}

func (s *session) sendRuntimeFlightState() {
	if s == nil || s.player == nil {
		return
	}
	canFly := s.hasAuraType(201) || s.hasAuraType(207)
	packet := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 4)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(0)
	if canFly {
		_ = s.write(uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY), packet.Bytes(), true)
		s.broadcastLoginMovementState(protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY, 0x01000000)
	} else {
		_ = s.write(uint16(protocol.OpcodeSMSG_MOVE_UNSET_CAN_FLY), packet.Bytes(), true)
		s.broadcastLoginMovementState(protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY, 0)
	}
}

func (s *session) sendRuntimeMovementUpdates(auraType uint32) {
	if s == nil || s.player == nil || !movementSpeedAura(auraType) {
		return
	}
	if auraType == 201 || auraType == 207 {
		s.sendRuntimeFlightState()
	}
	s.sendRuntimeMovementSpeed(protocol.OpcodeSMSG_FORCE_RUN_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_RUN_SPEED, s.mountedRunSpeed(), true)
	if s.hasAuraType(201) || s.hasAuraType(207) || s.hasAuraType(spellAuraIncreaseFlightSpeed) {
		s.sendRuntimeMovementSpeed(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED, s.mountedFlightSpeed(), false)
	}
}
