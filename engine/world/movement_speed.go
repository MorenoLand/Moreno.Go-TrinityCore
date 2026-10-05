package world

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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

// Mounted run speed mirrors Unit::UpdateSpeed's MOVE_RUN mounted branch (Unit.cpp:8678-8694):
// max-positive 32 main mod, total-multiplier 130 stack, max-positive 172 non-stack, speed = max(stack, nonstack)
// with the main mod applied as AddPct — verified equal here. C++ gates on IsMounted() (Unit.h:932,
// UNIT_FLAG_MOUNT); this gate additionally treats MountDisplayID != 0 as mounted, so Go's display-only
// mounts (taxi legs, .modify mount) take the mounted formula where C++ would see the flag unset.
func (s *session) mountedRunSpeed() float32 {
	// A .modify speed override shadows the aura computation while active,
	// mirroring m_speed_rate (Unit::GetSpeed, Unit.cpp:8811).
	if ov := s.gmSpeedOverride(moveTypeRun); ov > 0 {
		return ov
	}
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

// Mounted flight speed mirrors Unit::UpdateSpeed's MOVE_FLIGHT mounted branch (Unit.cpp:8714-8722):
// max-positive 207 main, total-multiplier 209 stack, max-positive 211 non-stack — verified equal here,
// as is the unmounted total-of-208 + total-of-206 arm below it. No bridge: the vehicle-flight arms
// (TYPEID_UNIT controlled-by-player 206-with-vehicle + GetCharmer owner max, and the
// GetVehicleBase()->UpdateSpeed(MOVE_FLIGHT) propagation at the MOVE_FLIGHT tail) and the minion
// follow-speed leg (Unit.cpp:8774-8788, FOLLOW-motion pets borrowing the owner's rate near 10yd);
// the 191 normalization snare/daze immune-mask skip is vacuous for players.
func (s *session) mountedFlightSpeed() float32 {
	// A .modify speed override shadows the aura computation while active,
	// mirroring m_speed_rate (Unit::GetSpeed, Unit.cpp:8811).
	if ov := s.gmSpeedOverride(moveTypeFlight); ov > 0 {
		return ov
	}
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
	// UnitMoveType order (Unit.h:262): walk, run, runBack, swim, swimBack,
	// turnRate, flight, flightBack, pitchRate. (Previously flight/flightBack
	// sat at 5/6 with turnRate at 7 — misordered vs the client's parse.)
	speeds := [9]float32{
		walk,                              // MOVE_WALK
		s.mountedRunSpeed(),               // MOVE_RUN
		s.adjustMovementSpeed(runBack),    // MOVE_RUN_BACK
		s.adjustMovementSpeed(swimSpeed),  // MOVE_SWIM
		s.adjustMovementSpeed(swimBack),   // MOVE_SWIM_BACK
		3.141594,                          // MOVE_TURN_RATE
		s.mountedFlightSpeed(),            // MOVE_FLIGHT
		s.adjustMovementSpeed(flightBack), // MOVE_FLIGHT_BACK
		3.14,                              // MOVE_PITCH_RATE
	}
	// .modify speed overrides shadow the aura-derived values while active
	// (Unit::GetSpeed = m_speed_rate * base, Unit.cpp:8811).
	for mtype := 0; mtype < len(speeds); mtype++ {
		if ov := s.gmSpeedOverride(mtype); ov > 0 {
			speeds[mtype] = ov
		}
	}
	return speeds
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

func movementRunSpeedAura(auraType uint32) bool {
	switch auraType {
	case 31, 32, 33, 58, 78, 129, 130, 171, 172, 191, 305:
		return true
	default:
		return false
	}
}

func movementFlightSpeedAura(auraType uint32) bool {
	return auraType == spellAuraDecreaseSpeed || auraType >= spellAuraIncreaseVehicleFlight && auraType <= spellAuraFlightSpeedNotStack
}

func mountedFlightAuraEffects(spell wotlk.Spell) (uint8, int32, [3]int32, [3]int32, bool) {
	var mask uint8
	var mountMisc int32
	var amounts, baseAmounts [3]int32
	hasMount, hasFlightSpeed := false, false
	for index, effect := range spell.Effects {
		if effect.Effect == 0 || effect.Aura == 0 {
			continue
		}
		switch effect.Aura {
		case spellAuraMounted:
			hasMount, mountMisc = true, effect.MiscValue
			mask |= 1 << uint(index)
			amounts[index], baseAmounts[index] = effect.BasePoints+1, effect.BasePoints
		case spellAuraMountedFlightSpeed, spellAuraMountedFlightSpeedAlways, spellAuraFlightSpeedNotStack:
			hasFlightSpeed = true
			mask |= 1 << uint(index)
			amounts[index], baseAmounts[index] = effect.BasePoints+1, effect.BasePoints
		}
	}
	return mask, mountMisc, amounts, baseAmounts, hasMount && hasFlightSpeed
}

func (s *session) activeAuraHasEffect(aura *activeAura, auraType uint32) bool {
	if aura == nil || aura.Stopped {
		return false
	}
	if aura.AuraType == auraType {
		return true
	}
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	spell, found, err := s.server.Data.Spell(aura.SpellID)
	if err != nil || !found {
		return false
	}
	for index, effect := range spell.Effects {
		if aura.EffectMask&(1<<uint(index)) != 0 && effect.Aura == auraType {
			return true
		}
	}
	return false
}

func (s *session) hasActiveFlightCapability() bool {
	if s == nil {
		return false
	}
	for _, aura := range s.loadedAuras() {
		if s.activeAuraHasEffect(aura, 201) || s.activeAuraHasEffect(aura, spellAuraMountedFlightSpeed) {
			return true
		}
	}
	return false
}

func movementSpeedAura(auraType uint32) bool {
	return movementRunSpeedAura(auraType) || movementFlightSpeedAura(auraType) || auraType == 201
}

func AffectsMovementSpeedAura(auraType uint32) bool {
	return movementSpeedAura(auraType)
}

func AffectsRunSpeedAura(auraType uint32) bool { return movementRunSpeedAura(auraType) }

func AffectsFlightSpeedAura(auraType uint32) bool { return movementFlightSpeedAura(auraType) }

func ResolveMovementSpeedUpdatePlan(auraType uint32, hasFlightSpeedAura, hasFlightCapability bool) (runSpeed, flightSpeed, canFly bool) {
	if !movementSpeedAura(auraType) {
		return false, false, false
	}
	return movementRunSpeedAura(auraType), movementFlightSpeedAura(auraType) || auraType == spellAuraMounted && hasFlightSpeedAura, auraType == 201 || auraType == spellAuraMountedFlightSpeed || auraType == spellAuraMounted && hasFlightCapability
}

func (s *session) hasFlightSpeedAura() bool {
	auraTypes := []uint32{201, spellAuraIncreaseVehicleFlight, spellAuraMountedFlightSpeed, spellAuraIncreaseFlightSpeed, spellAuraMountedFlightSpeedAlways, spellAuraVehicleSpeedAlways, spellAuraFlightSpeedNotStack}
	for _, auraType := range auraTypes {
		if s.hasAuraType(auraType) {
			return true
		}
	}
	for _, aura := range s.loadedAuras() {
		for _, auraType := range auraTypes {
			if s.activeAuraHasEffect(aura, auraType) {
				return true
			}
		}
	}
	return false
}

// UnitMoveType indices (Unit.h:262), used for the forced-speed-change ACK
// bookkeeping that mirrors Player::m_forced_speed_changes (Player.h:2023).
const (
	moveTypeWalk = iota
	moveTypeRun
	moveTypeRunBack
	moveTypeSwim
	moveTypeSwimBack
	moveTypeTurnRate
	moveTypeFlight
	moveTypeFlightBack
	moveTypePitchRate
	moveTypeCount
)

// playerBaseMoveSpeed mirrors Unit::playerBaseMoveSpeed (Unit.cpp:101-112):
// the absolute base speed per UnitMoveType that m_speed_rate multiplies.
var playerBaseMoveSpeed = [9]float32{
	2.5,      // MOVE_WALK
	7.0,      // MOVE_RUN
	4.5,      // MOVE_RUN_BACK
	4.722222, // MOVE_SWIM
	2.5,      // MOVE_SWIM_BACK
	3.141594, // MOVE_TURN_RATE
	7.0,      // MOVE_FLIGHT
	4.5,      // MOVE_FLIGHT_BACK
	3.14,     // MOVE_PITCH_RATE
}

// speedForceOpcodes maps each UnitMoveType to its {SMSG_FORCE_*_SPEED_CHANGE,
// MSG_MOVE_SET_*_SPEED} pair (Unit::SetSpeedRate's moveTypeToOpcode table,
// Unit.cpp:8837-8847). Only the types the command and aura systems touch
// are populated; the rest have no Go sender.
var speedForceOpcodes = [9][2]protocol.Opcode{
	moveTypeWalk:       {protocol.OpcodeSMSG_FORCE_WALK_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_WALK_SPEED},
	moveTypeRun:        {protocol.OpcodeSMSG_FORCE_RUN_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_RUN_SPEED},
	moveTypeRunBack:    {protocol.OpcodeSMSG_FORCE_RUN_BACK_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_RUN_BACK_SPEED},
	moveTypeSwim:       {protocol.OpcodeSMSG_FORCE_SWIM_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_SWIM_SPEED},
	moveTypeFlight:     {protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED},
	moveTypeFlightBack: {protocol.OpcodeSMSG_FORCE_FLIGHT_BACK_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_FLIGHT_BACK_SPEED},
}

// setGMSpeedRate mirrors Unit::SetSpeedRate (Unit.cpp:8825-8880) for the
// .modify speed command: the no-op-on-unchanged gate, the m_speed_rate
// store (here gmSpeedRates), the forced-speed-change ACK registration, and
// the SMSG_FORCE_*_SPEED_CHANGE to self plus MSG_MOVE_SET_*_SPEED to
// nearby. PropagateSpeedChange is MotionMaster-only (no Go bridge) and the
// pet propagation needs no bridge (Go pets have no speed model).
func (s *session) setGMSpeedRate(mtype int, rate float32) {
	if s == nil || s.player == nil || mtype < 0 || mtype >= len(speedForceOpcodes) {
		return
	}
	if rate < 0 {
		rate = 0
	}
	// Update speed only on change (Unit.cpp:8827-8828).
	if s.gmSpeedRates[mtype] == rate {
		return
	}
	s.gmSpeedRates[mtype] = rate
	opcodes := speedForceOpcodes[mtype]
	if opcodes[0] == 0 {
		return
	}
	s.sendRuntimeMovementSpeed(opcodes[0], opcodes[1], rate*playerBaseMoveSpeed[mtype], mtype == moveTypeRun)
}

// gmSpeedOverride returns the absolute GM-forced speed for a move type, or
// 0 when no override is active (Unit::GetSpeed =
// m_speed_rate[mtype] * playerBaseMoveSpeed[mtype], Unit.cpp:8811-8814).
func (s *session) gmSpeedOverride(mtype int) float32 {
	if s == nil || mtype < 0 || mtype >= len(s.gmSpeedRates) {
		return 0
	}
	if rate := s.gmSpeedRates[mtype]; rate > 0 {
		return rate * playerBaseMoveSpeed[mtype]
	}
	return 0
}

// clearGMSpeedRate drops the GM override for a move type, mirroring C++
// UpdateSpeed overwriting m_speed_rate via SetSpeedRate at the end of every
// aura-driven recompute (Unit.cpp:8779).
func (s *session) clearGMSpeedRate(mtype int) {
	if s == nil || mtype < 0 || mtype >= len(s.gmSpeedRates) {
		return
	}
	s.gmSpeedRates[mtype] = 0
}

// forcedSpeedMoveType maps an SMSG_FORCE_*_SPEED_CHANGE opcode to its
// UnitMoveType index (Unit::SendSpeedChange, Unit.cpp:8835).
func forcedSpeedMoveType(opcode protocol.Opcode) (int, bool) {
	switch opcode {
	case protocol.OpcodeSMSG_FORCE_WALK_SPEED_CHANGE:
		return moveTypeWalk, true
	case protocol.OpcodeSMSG_FORCE_RUN_SPEED_CHANGE:
		return moveTypeRun, true
	case protocol.OpcodeSMSG_FORCE_RUN_BACK_SPEED_CHANGE:
		return moveTypeRunBack, true
	case protocol.OpcodeSMSG_FORCE_SWIM_SPEED_CHANGE:
		return moveTypeSwim, true
	case protocol.OpcodeSMSG_FORCE_SWIM_BACK_SPEED_CHANGE:
		return moveTypeSwimBack, true
	case protocol.OpcodeSMSG_FORCE_TURN_RATE_CHANGE:
		return moveTypeTurnRate, true
	case protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE:
		return moveTypeFlight, true
	case protocol.OpcodeSMSG_FORCE_FLIGHT_BACK_SPEED_CHANGE:
		return moveTypeFlightBack, true
	case protocol.OpcodeSMSG_FORCE_PITCH_RATE_CHANGE:
		return moveTypePitchRate, true
	}
	return 0, false
}

// forcedSpeedAckMoveType maps a CMSG_FORCE_*_SPEED_CHANGE_ACK opcode to its
// UnitMoveType index (WorldSession::HandleForceSpeedChangeAck, MovementHandler.cpp:477).
func forcedSpeedAckMoveType(opcode uint16) (int, bool) {
	switch protocol.Opcode(opcode) {
	case protocol.OpcodeCMSG_FORCE_WALK_SPEED_CHANGE_ACK:
		return moveTypeWalk, true
	case protocol.OpcodeCMSG_FORCE_RUN_SPEED_CHANGE_ACK:
		return moveTypeRun, true
	case protocol.OpcodeCMSG_FORCE_RUN_BACK_SPEED_CHANGE_ACK:
		return moveTypeRunBack, true
	case protocol.OpcodeCMSG_FORCE_SWIM_SPEED_CHANGE_ACK:
		return moveTypeSwim, true
	case protocol.OpcodeCMSG_FORCE_SWIM_BACK_SPEED_CHANGE_ACK:
		return moveTypeSwimBack, true
	case protocol.OpcodeCMSG_FORCE_TURN_RATE_CHANGE_ACK:
		return moveTypeTurnRate, true
	case protocol.OpcodeCMSG_FORCE_FLIGHT_SPEED_CHANGE_ACK:
		return moveTypeFlight, true
	case protocol.OpcodeCMSG_FORCE_FLIGHT_BACK_SPEED_CHANGE_ACK:
		return moveTypeFlightBack, true
	case protocol.OpcodeCMSG_FORCE_PITCH_RATE_CHANGE_ACK:
		return moveTypePitchRate, true
	}
	return 0, false
}

func (s *session) sendRuntimeMovementSpeed(opcode protocol.Opcode, nearbyOpcode protocol.Opcode, speed float32, run bool) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	// Unit::SendSpeedChange (Unit.cpp:8852) registers every real sent forced
	// speed packet so WorldSession::HandleForceSpeedChangeAck can skip all
	// but the last ACK per type.
	if mt, ok := forcedSpeedMoveType(opcode); ok {
		if s.forcedSpeedChanges[mt] < 255 {
			s.forcedSpeedChanges[mt]++
		}
		s.forcedSpeedExpected[mt] = speed
		s.forcedSpeedSent[mt] = true
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
	s.debug("movement speed change sent", "account", s.accountName, "guid", s.playerGUID, "opcode", opcodeName(uint32(opcode)), "speed", speed, "mounted", s.hasAuraType(spellAuraMounted), "mount_display", s.player.MountDisplayID, "flight_aura_206", s.auraTypeModifiers(spellAuraIncreaseVehicleFlight), "flight_aura_207", s.auraTypeModifiers(spellAuraMountedFlightSpeed), "flight_aura_208", s.auraTypeModifiers(spellAuraIncreaseFlightSpeed), "flight_aura_209", s.auraTypeModifiers(spellAuraMountedFlightSpeedAlways), "flight_aura_211", s.auraTypeModifiers(spellAuraFlightSpeedNotStack))
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
	canFly := s.hasActiveFlightCapability()
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
	if s == nil || s.player == nil {
		return
	}
	runSpeed, flightSpeed, canFly := ResolveMovementSpeedUpdatePlan(auraType, s.hasFlightSpeedAura(), s.hasActiveFlightCapability())
	if canFly {
		s.sendRuntimeFlightState()
	}
	if runSpeed {
		// C++ UpdateSpeed overwrites m_speed_rate via SetSpeedRate
		// (Unit.cpp:8779): a .modify speed override is transient and dies
		// on the next aura-driven recompute for its type.
		s.clearGMSpeedRate(moveTypeRun)
		s.sendRuntimeMovementSpeed(protocol.OpcodeSMSG_FORCE_RUN_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_RUN_SPEED, s.mountedRunSpeed(), true)
	}
	if flightSpeed {
		s.clearGMSpeedRate(moveTypeFlight)
		s.sendRuntimeMovementSpeed(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED, s.mountedFlightSpeed(), false)
	}
}
