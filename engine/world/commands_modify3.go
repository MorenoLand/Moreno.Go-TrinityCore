package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// modify command port, chunk 2b: the modify arms gender (native) plus the
// documented-blocked arms bit, faction, phase, the speed sub-table and
// talentpoints (cs_modify.cpp). Chunk 2c (commands_modify4.go) covers the
// final arm, reputation, which closes cs_modify.cpp (23/23 arms).
//
//   - gender is native: playerState.Gender drives both the update-field
//     builder (unitFieldBytes0) and the race display-id lookup, which is the
//     Go equivalent of the SetGender/SetNativeGender/InitDisplayIds triple.
//   - bit has no Go bridge: update fields are built from playerState and
//     there is no generic SetFlag/RemoveFlag unit-value store.
//   - faction is wired on the selected creature (motion Faction/UnitFlags/
//     NPCFlags/DynamicFlags slots + values-update broadcast); the no-arg
//     form reports the current values.
//   - phase is wired on the selected unit: creatures take the motion
//     PhaseMask write, players take the session PhaseMaskOverride slot
//     (currentPlayerPhaseMask precedence), falling back to the invoker.
//   - the speed sub-table needs a per-type speed-rate store; Go movement
//     speeds are aura-derived (movementSpeeds).
//   - talentpoints sets the session free-talent-points override
//     (handleModifyTalentPoints); the pet arm stays out (no pet
//     points store; selections resolve to player sessions only).

// handleModifyGender mirrors HandleModifyGenderCommand (cs_modify.cpp:899):
// "male"/"female" (prefix-matched with the case-sensitive C++ strncmp) on
// the selected player or the handler's own player. The sObjectMgr->GetPlayerInfo gate is
// moot: race/class pairs are validated at character creation in the Go tree.
func (s *session) handleModifyGender(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyGender) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify gender <male|female>")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return
	}
	genderStr := strings.Join(args, " ") // C++ matches the whole tail string, case-sensitive strncmp
	var gender uint8
	switch {
	case strings.HasPrefix("male", genderStr):
		if target.player.Gender == 0 {
			return // already male: C++ returns true silently
		}
		gender = 0
	case strings.HasPrefix("female", genderStr):
		if target.player.Gender == 1 {
			return // already female: C++ returns true silently
		}
		gender = 1
	default:
		s.sendSysMessage("You must specify male or female.") // LANG_MUST_MALE_OR_FEMALE 1119
		return
	}
	target.player.Gender = gender
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET gender = ? WHERE guid = ?", gender, target.playerGUIDOf(target))
	}
	target.sendPlayerUpdate()
	genderFull := "male"
	if gender == 1 {
		genderFull = "female"
	}
	// LANG_YOU_CHANGE_GENDER 1120 / LANG_YOUR_GENDER_CHANGED 1121.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's gender to %s.", target.player.Name, genderFull),
		fmt.Sprintf("%s changed your gender to %s.", s.player.Name, genderFull))
}

// handleModifyBit is documented-blocked (cs_modify.cpp:614): toggling an
// arbitrary unit-field flag bit needs a generic Unit value store with
// SetFlag/RemoveFlag; the Go tree builds update fields from playerState and
// has no such bridge.
func (s *session) handleModifyBit(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandModifyBit) {
		return
	}
	s.sendSysMessage("modify bit is not supported: generic unit-field flag toggles have no Go bridge.")
}

// handleModifyFaction mirrors HandleModifyFactionCommand (cs_modify.cpp:209):
// the selected creature only (getSelectedCreature; players are never a
// target here). With no argument it reports the current faction/flags/
// npcflags/dyflags; otherwise it writes factionid plus the optional flag,
// npcflag and dyflag values, each defaulting to the current value, after
// the FactionTemplate.dbc lookup (LANG_WRONG_FACTION on a miss). The
// writes ride the motion slots plus a creature values-update broadcast,
// the SetFaction bridge pattern.
func (s *session) handleModifyFaction(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyFaction) {
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	srv := s.server
	srv.motionMu.Lock()
	faction, flags, npcflags, dyflags := motion.Faction, motion.UnitFlags, motion.NPCFlags, motion.DynamicFlags
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	srv.motionMu.Unlock()
	low := uint32(guid & 0x00FFFFFF)
	if len(args) == 0 {
		s.sendSysMessage(fmt.Sprintf("Current faction for creature %d is %d, flags %d, npcflags %d, dyflags %d.", low, faction, flags, npcflags, dyflags))
		return
	}
	if strings.HasPrefix(args[0], "|") {
		// C++ extractKeyFromLink (Chat.cpp:362): a |Hfaction:key| link
		// yields the key and tokenizing continues after the link tail;
		// a wrong link type yields null, i.e. the no-arg report. The
		// synthetic first arg keeps its link tail: cAtoi stops at the
		// first non-digit, so it reads exactly the key.
		joined := strings.Join(args, " ")
		fi := strings.Index(joined, "Hfaction:")
		if fi < 0 {
			s.sendSysMessage(fmt.Sprintf("Current faction for creature %d is %d, flags %d, npcflags %d, dyflags %d.", low, faction, flags, npcflags, dyflags))
			return
		}
		rest := joined[fi+len("Hfaction:"):]
		tail := ""
		if hi := strings.Index(rest, "|h|r"); hi >= 0 {
			tail = rest[hi+len("|h|r"):]
		}
		args = append([]string{rest}, strings.Fields(tail)...)
	}
	faction = uint32(cAtoi(args[0]))
	if len(args) > 1 {
		flags = uint32(cAtoi(args[1]))
	}
	if len(args) > 2 {
		npcflags = uint32(cAtoi(args[2]))
	}
	if len(args) > 3 {
		dyflags = uint32(cAtoi(args[3]))
	}
	if srv == nil || srv.Data == nil {
		s.sendSysMessage("Faction data is unavailable.")
		return
	}
	if _, found := s.factionTemplateEntry(faction); !found {
		s.sendSysMessage(fmt.Sprintf("Wrong faction %d.", faction))
		return
	}
	srv.motionMu.Lock()
	motion.Faction, motion.UnitFlags, motion.NPCFlags, motion.DynamicFlags = faction, flags, npcflags, dyflags
	srv.motionMu.Unlock()
	srv.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldFaction: faction, unitFieldFlags: flags, unitFieldNPCFlags: npcflags, unitFieldDynamicFlags: dyflags})
	s.sendSysMessage(fmt.Sprintf("You changed faction for creature %d to %d, flags %d, npcflags %d, dyflags %d.", low, faction, flags, npcflags, dyflags))
}

// handleModifyPhase mirrors HandleModifyPhaseCommand (cs_modify.cpp:836):
// the selected unit, falling back to the invoker when nothing resolves
// (getSelectedUnit nil arm). A selected creature takes the motion
// PhaseMask write (the Lua SetPhaseMask twin, no visibility fan-out);
// a selected player takes the session PhaseMaskOverride slot, which
// currentPlayerPhaseMask reads before the GM/aura legs, matching C++'s
// stored-mask semantics (Player::SetGameMaster itself writes
// PHASEMASK_ANYWHERE into that slot). The player-target security guard
// mirrors the other modify arms; creature targets carry none in C++.
func (s *session) handleModifyPhase(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyPhase) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify phase <phasemask>")
		return
	}
	mask := uint32(cAtoi(args[0]))
	if motion := s.castCommandSelectedCreature(); motion != nil {
		srv := s.server
		srv.motionMu.Lock()
		motion.PhaseMask = mask
		srv.motionMu.Unlock()
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	target.player.PhaseMaskOverride = mask
	target.player.PhaseMaskOverrideSet = true
}

// handleModifySpeed implements the "speed" sub-table (cs_modify.cpp:47-56):
// all/backwalk/fly/walk/swim plus the "" entry (bare ".modify speed <val>"
// shares the all-speeds handler HandleModifyASpeedCommand and the root speed
// permission, 561). Rates go through the Unit::SetSpeedRate bridge
// (setGMSpeedRate): the SMSG_FORCE_*_SPEED_CHANGE to self and
// MSG_MOVE_SET_*_SPEED to nearby are real, and the ACK bookkeeping covers
// all nine move types.
func (s *session) handleModifySpeed(ctx context.Context, args []string) {
	const syntax = "Syntax: .modify speed [all|backwalk|fly|walk|swim] <val>"
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandModifySpeed) {
			return
		}
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// Bare ".modify speed <val>" hits the "" table entry, which shares the
	// all-speeds handler and the root speed permission (561).
	if _, err := strconv.ParseFloat(sub, 32); err == nil {
		s.modifySpeedAll(ctx, permissionCommandModifySpeed, args)
		return
	}
	switch {
	case strings.HasPrefix("all", sub):
		s.modifySpeedAll(ctx, permissionCommandModifySpeedAll, rest)
	case strings.HasPrefix("backwalk", sub):
		s.modifySpeedType(ctx, permissionCommandModifySpeedBackwalk, rest, "back", []int{moveTypeRunBack}, true)
	case strings.HasPrefix("fly", sub):
		s.modifySpeedType(ctx, permissionCommandModifySpeedFly, rest, "fly", []int{moveTypeFlight}, false)
	case strings.HasPrefix("walk", sub):
		s.modifySpeedType(ctx, permissionCommandModifySpeedWalk, rest, "", []int{moveTypeRun}, true)
	case strings.HasPrefix("swim", sub):
		s.modifySpeedType(ctx, permissionCommandModifySpeedSwim, rest, "swim", []int{moveTypeSwim}, true)
	default:
		s.sendSysMessage(syntax)
	}
}

// modifySpeedAll mirrors HandleModifyASpeedCommand (cs_modify.cpp:417):
// the WALK, RUN, SWIM and FLIGHT rates are all set to the same value.
func (s *session) modifySpeedAll(ctx context.Context, perm uint32, args []string) {
	if s.miscDeny(ctx, perm) {
		return
	}
	speed, target, ok := s.checkModifySpeedTarget(ctx, args, true)
	if !ok {
		return
	}
	// LANG_YOU_CHANGE_ASPEED 137 / LANG_YOURS_ASPEED_CHANGED 138.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's all speeds to %.2f.", target.player.Name, speed),
		fmt.Sprintf("%s changed your all speeds to %.2f.", s.player.Name, speed))
	for _, mtype := range []int{moveTypeWalk, moveTypeRun, moveTypeSwim, moveTypeFlight} {
		target.setGMSpeedRate(mtype, speed)
	}
}

// modifySpeedType mirrors the single-type arms (cs_modify.cpp:434-492):
// walk -> MOVE_RUN, backwalk -> MOVE_RUN_BACK, swim -> MOVE_SWIM,
// fly -> MOVE_FLIGHT. Only fly skips the in-flight check.
func (s *session) modifySpeedType(ctx context.Context, perm uint32, args []string, kind string, mtypes []int, checkInFlight bool) {
	if s.miscDeny(ctx, perm) {
		return
	}
	speed, target, ok := s.checkModifySpeedTarget(ctx, args, checkInFlight)
	if !ok {
		return
	}
	label := "speed"
	if kind != "" {
		label = kind + " speed"
	}
	s.notifyModify(target,
		fmt.Sprintf("You change %s's %s to %.2f.", target.player.Name, label, speed),
		fmt.Sprintf("%s changed your %s to %.2f.", s.player.Name, label, speed))
	for _, mtype := range mtypes {
		target.setGMSpeedRate(mtype, speed)
	}
}

// checkModifySpeedTarget mirrors modify_commandscript::CheckModifySpeed
// (cs_modify.cpp:379): the value must parse inside [0.1, 50.0]
// (LANG_BAD_VALUE 115), the target is the selected player or self with the
// lower-security guard, and a target in flight is rejected unless the arm
// skips the flight check (LANG_CHAR_IN_FLIGHT 21).
func (s *session) checkModifySpeedTarget(ctx context.Context, args []string, checkInFlight bool) (float32, *session, bool) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify speed [all|backwalk|fly|walk|swim] <val>")
		return 0, nil, false
	}
	speed, ok := checkModifySpeedBounds(args[0], 0.1, 50.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return 0, nil, false
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return 0, nil, false
	}
	if checkInFlight && target.isInFlight() {
		s.sendSysMessage(miscCharInFlight) // LANG_CHAR_IN_FLIGHT 21
		return 0, nil, false
	}
	return speed, target, true
}

// handleModifyTalentPoints mirrors HandleModifyTalentPointsCommand
// (cs_modify.cpp:342): the player arm sets stored free talent points
// and resends the talents frame. Go stores the value as the session
// free-talent-points override; the pet arm stays out (Pet::
// SetFreeTalentPoints writes the pet byte field, and Go selections
// resolve to player sessions only).
func (s *session) handleModifyTalentPoints(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandModifyTalentPoints) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .modify talentpoints <value>")
		return
	}
	tp := cAtoi(args[0]) // C++ atoi: "abc" is 0 points, not a syntax error
	if tp < 0 {
		s.sendSysMessage("Syntax: .modify talentpoints <value>")
		return
	}
	if s.castCommandSelectedCreature() != nil {
		// C++ getSelectedUnit arm: a non-player, non-pet unit gets
		// LANG_NO_CHAR_SELECTED instead of the invoker's points (the
		// pet leg itself stays out: no pet points store).
		s.sendSysMessage("No character selected.")
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	target.player.FreeTalentPointsOverride = uint32(tp)
	target.player.FreeTalentPointsOverrideSet = true
	_ = target.sendTalentsInfo(false)
}

// handleModifyChunk2b extends the modify dispatcher with the chunk-2b arms.
// The chunk-2c arm (reputation) chains off the default case.
func (s *session) handleModifyChunk2b(ctx context.Context, sub string, rest []string) bool {
	switch {
	case strings.HasPrefix("gender", sub):
		s.handleModifyGender(ctx, rest)
	case strings.HasPrefix("bit", sub):
		s.handleModifyBit(ctx)
	case strings.HasPrefix("faction", sub):
		s.handleModifyFaction(ctx, rest)
	case strings.HasPrefix("phase", sub):
		s.handleModifyPhase(ctx, rest)
	case strings.HasPrefix("speed", sub):
		s.handleModifySpeed(ctx, rest)
	case strings.HasPrefix("talentpoints", sub):
		s.handleModifyTalentPoints(ctx, rest)
	default:
		return s.handleModifyChunk2c(ctx, sub, rest)
	}
	return true
}
