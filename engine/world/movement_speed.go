package world

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellAuraIncreaseSpeed            uint32 = 31
	spellAuraIncreaseMountedSpeed     uint32 = 32
	spellAuraMountedSpeedAlways       uint32 = 130
	spellAuraMountedSpeedNotStack     uint32 = 172
	spellAuraIncreaseFlightSpeed      uint32 = 208
	spellAuraMountedFlightSpeedAlways uint32 = 209
	spellAuraFlightSpeedNotStack      uint32 = 211
)

func (s *session) maxPositiveAuraModifier(auraType uint32) int32 {
	maxValue := int32(0)
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == auraType && int32(aura.Amount) > maxValue {
			maxValue = int32(aura.Amount)
		}
	}
	return maxValue
}

func (s *session) auraMultiplier(auraType uint32) float32 {
	multiplier := float32(1)
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == auraType && aura.Amount > 0 {
			multiplier *= 1 + float32(aura.Amount)/100
		}
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
	if main > 0 {
		multiplier *= 1 + float32(main)/100
	}
	return 7 * multiplier
}

func (s *session) mountedFlightSpeed() float32 {
	main := s.maxPositiveAuraModifier(207)
	stack := s.auraMultiplier(spellAuraMountedFlightSpeedAlways)
	nonStack := float32(1) + float32(s.maxPositiveAuraModifier(spellAuraFlightSpeedNotStack))/100
	if main == 0 && s.mounts != nil {
		if preferred := s.mounts.PreferredFlightSpeed(true); preferred > 100 {
			main = int32(preferred - 100)
		}
	}
	multiplier := float32(math.Max(float64(stack), float64(nonStack)))
	if main > 0 {
		multiplier *= 1 + float32(main)/100
	}
	return 7 * multiplier
}

func movementSpeedAura(auraType uint32) bool {
	switch auraType {
	case 31, 32, 78, 129, 130, 171, 172, 201, 207, 208, 209, 211:
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
	nearby := protocol.NewBuffer(96)
	nearby.WritePackedGUID(s.playerGUID)
	nearby.WriteU32(0)
	nearby.WriteU16(0)
	nearby.WriteU32(uint32(time.Now().UnixMilli()))
	nearby.WriteF32(s.player.X)
	nearby.WriteF32(s.player.Y)
	nearby.WriteF32(s.player.Z)
	nearby.WriteF32(s.player.Orientation)
	if s.player.TransportGUID != 0 {
		nearby.WritePackedGUID(s.player.TransportGUID)
		nearby.WriteF32(s.player.TransportX)
		nearby.WriteF32(s.player.TransportY)
		nearby.WriteF32(s.player.TransportZ)
		nearby.WriteF32(s.player.TransportO)
		nearby.WriteU32(0)
		nearby.WriteI8(s.player.TransportSeat)
	}
	nearby.WriteU32(0)
	for _, base := range []float32{2.5, 7, 4.5, 4.722222, 2.5, 7, 4.5, 3.141594, 3.14} {
		nearby.WriteF32(base)
	}
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
