package world

import (
	"context"
	"fmt"
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

// checkModifySpeedBounds mirrors modify_commandscript::CheckModifySpeed
// (cs_modify.cpp:374) minus the target/security legs, which each arm handles
// itself: the value must parse and sit inside [minimumBound, maximumBound].
func checkModifySpeedBounds(arg string, minimumBound, maximumBound float64) (float32, bool) {
	speed, err := strconv.ParseFloat(arg, 32)
	if err != nil || speed > maximumBound || speed < minimumBound {
		return 0, false
	}
	return float32(speed), true
}

// handleModifyScale mirrors HandleModifyScaleCommand (cs_modify.cpp:474):
// the selected unit (online player only in the Go tree) or the handler's own
// player gets the object scale.
func (s *session) handleModifyScale(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyScale) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify scale <val>")
		return
	}
	scale, ok := checkModifySpeedBounds(args[0], 0.1, 10.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	// LANG_YOU_CHANGE_SIZE 147 / LANG_YOURS_SIZE_CHANGED 148.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's size to %f.", target.player.Name, scale),
		fmt.Sprintf("%s changed your size to %f.", s.player.Name, scale))
	target.scale = scale
	target.sendPlayerUpdate()
}

// handleModifySpell mirrors HandleModifySpellCommand (cs_modify.cpp:295):
// SMSG_SET_FLAT_SPELL_MODIFIER (0x266) with uint8 flat id, uint8 op,
// uint16 value, uint16 mark (default 65535) goes to the selected player or
// the handler's own player.
func (s *session) handleModifySpell(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifySpell) {
		return
	}
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .modify spell <spellflatid> <op> <val> [mark]")
		return
	}
	flatID, err1 := strconv.ParseUint(args[0], 10, 8)
	op, err2 := strconv.ParseUint(args[1], 10, 8)
	val, err3 := strconv.ParseUint(args[2], 10, 16)
	mark := uint64(65535)
	if len(args) > 3 {
		mark, err3 = strconv.ParseUint(args[3], 10, 16)
	}
	if err1 != nil || err2 != nil || err3 != nil {
		s.sendSysMessage("Syntax: .modify spell <spellflatid> <op> <val> [mark]")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	// LANG_YOU_CHANGE_SPELLFLATID 131 / LANG_YOURS_SPELLFLATID_CHANGED 132.
	s.notifyModify(target,
		fmt.Sprintf("You set spell flat id %d to %d (mark %d) for %s.", flatID, val, mark, target.player.Name),
		fmt.Sprintf("%s set your spell flat id %d to %d (mark %d).", s.player.Name, flatID, val, mark))
	packet := protocol.NewBuffer(6)
	packet.WriteU8(uint8(flatID))
	packet.WriteU8(uint8(op))
	packet.WriteU16(uint16(val))
	packet.WriteU16(uint16(mark))
	_ = target.write(uint16(protocol.OpcodeSMSG_SET_FLAT_SPELL_MODIFIER), packet.Bytes(), true)
}

// handleModifyStandState mirrors HandleModifyStandStateCommand
// (cs_modify.cpp:857): UNIT_NPC_EMOTESTATE on the handler's own player.
func (s *session) handleModifyStandState(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyStandState) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify standstate <animid>")
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	animID := uint32(cAtoi(args[0]))
	s.player.EmoteState = animID
	s.sendPlayerUpdate()
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
func (s *session) handleModifyMount(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyMount) {
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .modify mount <id> <speed>")
		return
	}
	mount := uint32(cAtoi(args[0]))
	if !s.mountDisplayValid(mount) {
		s.sendSysMessage("There is no such mount.") // LANG_NO_MOUNT 149
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	speed, ok := checkModifySpeedBounds(args[1], 0.1, 50.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return
	}
	if target.isInFlight() {
		s.sendSysMessage(miscCharInFlight) // LANG_CHAR_IN_FLIGHT 21
		return
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
}

// extendModifyDispatcher adds the chunk-2a arms to the modify root. It is
// called from handleCmdModify's dispatch switch.
func (s *session) handleModifyChunk2a(ctx context.Context, sub string, rest []string) bool {
	switch {
	case strings.HasPrefix("scale", sub):
		s.handleModifyScale(ctx, rest)
	case strings.HasPrefix("spell", sub):
		s.handleModifySpell(ctx, rest)
	case strings.HasPrefix("standstate", sub):
		s.handleModifyStandState(ctx, rest)
	case strings.HasPrefix("mount", sub):
		s.handleModifyMount(ctx, rest)
	default:
		if s.handleModifyChunk2b(ctx, sub, rest) {
			return true
		}
		return false
	}
	return true
}
