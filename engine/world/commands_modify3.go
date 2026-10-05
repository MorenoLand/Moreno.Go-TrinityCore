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

// handleModifySpeed dispatches the "speed" sub-table (cs_modify.cpp:47-56):
// all/backwalk/fly/walk/swim plus the "" entry (bare ".modify speed <val>"
// behaves like "all"). All six are documented-blocked: per-type speed rates
// have no Go store (movementSpeeds is aura-derived), so neither the rate nor
// the SMSG_FORCE_*_SPEED_CHANGE broadcast can be modeled faithfully.
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
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeed, "all")
		return
	}
	switch {
	case strings.HasPrefix("all", sub):
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeedAll, "all")
	case strings.HasPrefix("backwalk", sub):
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeedBackwalk, "backwalk")
	case strings.HasPrefix("fly", sub):
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeedFly, "fly")
	case strings.HasPrefix("walk", sub):
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeedWalk, "walk")
	case strings.HasPrefix("swim", sub):
		s.modifySpeedBlocked(ctx, permissionCommandModifySpeedSwim, "swim")
	default:
		_ = rest
		s.sendSysMessage(syntax)
	}
}

func (s *session) modifySpeedBlocked(ctx context.Context, perm uint32, what string) {
	if s.miscDeny(ctx, perm) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("modify speed %s is not supported: per-type speed rates have no Go bridge.", what))
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
