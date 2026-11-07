package world

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// modify command port, chunk 2a: the modify arms scale, spell, standstate
// and mount (cs_modify.cpp). Chunk 1 (commands_modify.go) covered morph,
// demorph and the hp/mana/energy/rage/runicpower/money/honor/arenapoints/xp/
// drunk arms. Chunk 2b will cover bit, faction, gender, phase, reputation,
// the speed sub-table and talentpoints.
//
// mount is partial: the CreatureDisplayInfo.dbc gate and MountDisplayID are
// native, and the SetSpeedRate(MOVE_RUN/MOVE_FLIGHT) legs ride the
// setGMSpeedRate bridge (movement_speed.go); the Go mount itself stays
// display-only (no mount aura), so a later aura-driven speed recompute
// clears the override exactly like C++ UpdateSpeed overwriting m_speed_rate.

// modifyAtof mirrors C atof as used by CheckModifySpeed (cs_modify.cpp:379):
// the longest leading numeric prefix parses and trailing junk is ignored,
// so "2.5x" is 2.5; with no numeric prefix the value is 0.0, which then
// fails the bounds check exactly like the C++ (pure garbage is BAD_VALUE
// in both).
func modifyAtof(arg string) float64 {
	i := 0
	for i < len(arg) && (arg[i] == ' ' || arg[i] == '\t' || arg[i] == '\n' || arg[i] == '\v' || arg[i] == '\f' || arg[i] == '\r') {
		i++
	}
	j := i
	if j < len(arg) && (arg[j] == '+' || arg[j] == '-') {
		j++
	}
	// C strtod also parses hexadecimal floats ("0x10" is 16, "0x10.8" is
	// 16.5, optional p-exponent); the decimal scan below would stop at the
	// 'x' and read 0, so handle the hex form first.
	if j+1 < len(arg) && arg[j] == '0' && (arg[j+1] == 'x' || arg[j+1] == 'X') {
		k := j + 2
		intVal := 0.0
		intDigits := 0
		for k < len(arg) && isHexDigit(arg[k]) {
			intVal = intVal*16 + float64(hexDigitValue(arg[k]))
			k++
			intDigits++
		}
		fracVal := 0.0
		fracDigits := 0
		if k < len(arg) && arg[k] == '.' {
			k++
			div := 16.0
			for k < len(arg) && isHexDigit(arg[k]) {
				fracVal += float64(hexDigitValue(arg[k])) / div
				div *= 16
				k++
				fracDigits++
			}
		}
		if intDigits == 0 && fracDigits == 0 {
			// C strtod("0x") consumes just the leading "0".
			return 0
		}
		v := intVal + fracVal
		if k < len(arg) && (arg[k] == 'p' || arg[k] == 'P') {
			k++
			negExp := false
			if k < len(arg) && (arg[k] == '+' || arg[k] == '-') {
				negExp = arg[k] == '-'
				k++
			}
			exp := 0
			expDigits := 0
			for k < len(arg) && arg[k] >= '0' && arg[k] <= '9' {
				exp = exp*10 + int(arg[k]-'0')
				k++
				expDigits++
			}
			if expDigits > 0 {
				if negExp {
					exp = -exp
				}
				v = math.Ldexp(v, exp)
			}
		}
		if j > i && arg[i] == '-' {
			return -v
		}
		return v
	}
	digits := 0
	for j < len(arg) && arg[j] >= '0' && arg[j] <= '9' {
		j++
		digits++
	}
	if j < len(arg) && arg[j] == '.' {
		j++
		for j < len(arg) && arg[j] >= '0' && arg[j] <= '9' {
			j++
			digits++
		}
	}
	if digits == 0 {
		return 0
	}
	if j < len(arg) && (arg[j] == 'e' || arg[j] == 'E') {
		k := j + 1
		if k < len(arg) && (arg[k] == '+' || arg[k] == '-') {
			k++
		}
		expDigits := 0
		for k < len(arg) && arg[k] >= '0' && arg[k] <= '9' {
			k++
			expDigits++
		}
		if expDigits > 0 {
			j = k
		}
	}
	v, err := strconv.ParseFloat(arg[i:j], 64)
	if err != nil {
		return 0
	}
	return v
}

// isHexDigit / hexDigitValue are the hex-float legs of modifyAtof.
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexDigitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// checkModifySpeedBounds mirrors modify_commandscript::CheckModifySpeed
// (cs_modify.cpp:374) minus the target/security legs, which each arm handles
// itself: the atof value, cast to float like the C++, must sit inside
// [minimumBound, maximumBound].
func checkModifySpeedBounds(arg string, minimumBound, maximumBound float64) (float32, bool) {
	speed := float32(modifyAtof(arg))
	if float64(speed) > maximumBound || float64(speed) < minimumBound {
		return 0, false
	}
	return speed, true
}

// handleModifyScale mirrors HandleModifyScaleCommand (cs_modify.cpp:474):
// the selected unit (online player only in the Go tree) or the handler's own
// player gets the object scale.
func (s *session) handleModifyScale(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyScale) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args
		// (cs_modify.cpp:381-382 via CheckModifySpeed).
		return false
	}
	scale, ok := checkModifySpeedBounds(args[0], 0.1, 10.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	if motion := s.castCommandSelectedCreature(); motion != nil {
		// C++ scale targets the selected unit (cs_modify.cpp:490); a
		// creature takes the SetObjectScale twin (Lua SetScale): the
		// script override while known, broadcast with the aura-adjusted
		// effective scale. No message: NotifyModification only reports
		// for player targets (cs_modify.cpp:91-96).
		if s.server != nil {
			s.server.motionMu.Lock()
			motion.ScriptScale = scale
			motion.ScriptScaleKnown = true
			mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
			key := creatureAuraKeyForMotion(motion)
			s.server.motionMu.Unlock()
			effective := float64(scale) + float64(s.server.creatureAuraScaleSum(key))/100
			if effective < 0.01 {
				effective = 0.01
			}
			s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{objectFieldScale: math.Float32bits(float32(effective))})
		}
		return true
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	// LANG_YOU_CHANGE_SIZE 147 / LANG_YOURS_SIZE_CHANGED 148.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's size to %f.", target.player.Name, scale),
		fmt.Sprintf("%s changed your size to %f.", s.player.Name, scale))
	target.scale = scale
	target.sendPlayerUpdate()
	return true
}

// handleModifySpell mirrors HandleModifySpellCommand (cs_modify.cpp:295):
// SMSG_SET_FLAT_SPELL_MODIFIER (0x266) with uint8 flat id, uint8 op,
// uint16 value, uint16 mark (default 65535) goes to the selected player or
// the handler's own player.
func (s *session) handleModifySpell(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifySpell) {
		return false
	}
	if len(args) < 3 {
		// C++ returns false with no message on a missing spellflatid/op/
		// val token (cs_modify.cpp:282-292: null strtok results).
		return false
	}
	// C++ uses atoi with uint8/uint8/uint16/uint16 truncation (mark
	// defaults to 65535), so 300 becomes 44 and "abc" becomes 0 instead
	// of a syntax error (cs_modify.cpp:295-310).
	flatID := uint8(cAtoi(args[0]))
	op := uint8(cAtoi(args[1]))
	val := uint16(cAtoi(args[2]))
	mark := uint16(65535)
	if len(args) > 3 {
		mark = uint16(cAtoi(args[3]))
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	// LANG_YOU_CHANGE_SPELLFLATID 131 / LANG_YOURS_SPELLFLATID_CHANGED 132.
	s.notifyModify(target,
		fmt.Sprintf("You set spell flat id %d to %d (mark %d) for %s.", flatID, val, mark, target.player.Name),
		fmt.Sprintf("%s set your spell flat id %d to %d (mark %d).", s.player.Name, flatID, val, mark))
	packet := protocol.NewBuffer(6)
	packet.WriteU8(flatID)
	packet.WriteU8(op)
	packet.WriteU16(val)
	packet.WriteU16(mark)
	_ = target.write(uint16(protocol.OpcodeSMSG_SET_FLAT_SPELL_MODIFIER), packet.Bytes(), true)
	return true
}

// handleModifyStandState mirrors HandleModifyStandStateCommand
// (cs_modify.cpp:857): UNIT_NPC_EMOTESTATE on the handler's own player.
func (s *session) handleModifyStandState(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyStandState) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args (cs_modify.cpp:856-857).
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	animID := uint32(cAtoi(args[0]))
	s.player.EmoteState = animID
	s.sendPlayerUpdate()
	return true
}

// mountDisplayValid mirrors the sCreatureDisplayInfoStore.LookupEntry gate in
// HandleModifyMountCommand (cs_modify.cpp:492) via the DBC store.
func (s *session) mountDisplayValid(mount uint32) bool {
	if s.server == nil || s.server.Data == nil {
		return false
	}
	file, err := s.server.Data.File("CreatureDisplayInfo")
	if err != nil {
		return false
	}
	_, found := file.Find(mount)
	return found
}

// handleModifyMount mirrors HandleModifyMountCommand (cs_modify.cpp:486):
// the selected player or the handler's own player is mounted on the display
// id, and the RUN/FLIGHT rates take the given speed via the SetSpeedRate
// bridge (Unit.cpp:8825).
func (s *session) handleModifyMount(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyMount) {
		return false
	}
	if len(args) < 2 {
		// C++ returns false with no message on a missing mount/speed
		// token (cs_modify.cpp:510-513: null strtok results).
		return false
	}
	mount := uint32(cAtoi(args[0]))
	if !s.mountDisplayValid(mount) {
		s.sendSysMessage("There is no such mount.") // LANG_NO_MOUNT 149
		return true
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	speed, ok := checkModifySpeedBounds(args[1], 0.1, 50.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	if target.isInFlight() {
		s.sendSysMessage(miscCharInFlight) // LANG_CHAR_IN_FLIGHT 21
		return true
	}
	// LANG_YOU_GIVE_MOUNT 150 / LANG_MOUNT_GIVED 151.
	s.notifyModify(target,
		fmt.Sprintf("You give %s a mount.", target.player.Name),
		fmt.Sprintf("%s gives you a mount.", s.player.Name))
	target.player.MountDisplayID = mount
	target.sendPlayerUpdate()
	target.sendPlayerCollisionHeight()
	// C++: Mount(mount) then SetSpeedRate(MOVE_RUN, speed) and
	// SetSpeedRate(MOVE_FLIGHT, speed) (cs_modify.cpp:541-542). The Go mount
	// is display-only (no mount aura), so the rates land directly.
	target.setGMSpeedRate(moveTypeRun, speed)
	target.setGMSpeedRate(moveTypeFlight, speed)
	return true
}

// extendModifyDispatcher adds the chunk-2a arms to the modify root. It is
// called from handleCmdModify's dispatch switch. It returns (handled, ok):
// handled is false when no arm matched; ok is the matched arm's bool.
func (s *session) handleModifyChunk2a(ctx context.Context, sub string, rest []string) (bool, bool) {
	switch {
	case strings.HasPrefix("scale", sub):
		return true, s.handleModifyScale(ctx, rest)
	case strings.HasPrefix("spell", sub):
		return true, s.handleModifySpell(ctx, rest)
	case strings.HasPrefix("standstate", sub):
		return true, s.handleModifyStandState(ctx, rest)
	case strings.HasPrefix("mount", sub):
		return true, s.handleModifyMount(ctx, rest)
	default:
		return s.handleModifyChunk2b(ctx, sub, rest)
	}
}
