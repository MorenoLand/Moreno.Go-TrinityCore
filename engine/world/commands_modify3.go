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
//   - faction needs a selected creature; command selections resolve only to
//     online player sessions.
//   - phase needs a stored phase-mask model; the Go phase mask is
//     aura-derived (currentPlayerPhaseMask).
//   - the speed sub-table needs a per-type speed-rate store; Go movement
//     speeds are aura-derived (movementSpeeds).
//   - talentpoints needs a stored free-talent-points model; Go derives free
//     talent points from level minus spent (freeTalentPoints).

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
	genderStr := args[0] // case-sensitive, like the C++ strncmp
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

// handleModifyFaction is documented-blocked (cs_modify.cpp:209): the arm
// operates on the selected creature; command selections resolve only to
// online player sessions, so no creature target can be named.
func (s *session) handleModifyFaction(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandModifyFaction) {
		return
	}
	s.sendSysMessage("modify faction is not supported: selected-unit creature targets have no Go bridge.")
}

// handleModifyPhase is documented-blocked (cs_modify.cpp:836): the arm sets
// a temporary phase mask on the unit, but the Go phase mask is aura-derived
// (currentPlayerPhaseMask) with no stored phase-mask model.
func (s *session) handleModifyPhase(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandModifyPhase) {
		return
	}
	s.sendSysMessage("modify phase is not supported: stored phase masks have no Go bridge.")
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

// handleModifyTalentPoints is documented-blocked (cs_modify.cpp:342): the
// arm sets stored free talent points, but the Go tree derives free talent
// points from level minus spent (freeTalentPoints) with no stored model —
// for players and for pets.
func (s *session) handleModifyTalentPoints(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandModifyTalentPoints) {
		return
	}
	s.sendSysMessage("modify talentpoints is not supported: stored free talent points have no Go bridge.")
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
		s.handleModifyFaction(ctx)
	case strings.HasPrefix("phase", sub):
		s.handleModifyPhase(ctx)
	case strings.HasPrefix("speed", sub):
		s.handleModifySpeed(ctx, rest)
	case strings.HasPrefix("talentpoints", sub):
		s.handleModifyTalentPoints(ctx)
	default:
		return s.handleModifyChunk2c(ctx, sub, rest)
	}
	return true
}
