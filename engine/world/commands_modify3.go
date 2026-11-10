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
//   - bit is wired on the selected unit via the indexed-value backends
//     (lua_values.go): a field the backend reads AND writes round-trips
//     with a change-only broadcast; valid-range but unbacked fields (and
//     bits a partial-byte slot silently drops) report honestly.
//   - faction is wired on the selected creature (motion Faction/UnitFlags/
//     NPCFlags/DynamicFlags slots + values-update broadcast); the no-arg
//     form reports the current values.
//   - phase is wired on the selected unit: creatures take the motion
//     PhaseMask write, players take the session PhaseMaskOverride slot
//     (currentPlayerPhaseMask precedence), falling back to the invoker.
//   - the speed sub-table needs a per-type speed-rate store; Go movement
//     speeds are aura-derived (movementSpeeds).
//   - talentpoints sets the session free-talent-points override
//     (handleModifyTalentPoints); the pet arm sets the live motion's
//     override on a permanent pet of a player owner and resends the
//     owner's pet talents frame.

// handleModifyGender mirrors HandleModifyGenderCommand (cs_modify.cpp:899):
// "male"/"female" (prefix-matched with the case-sensitive C++ strncmp) on
// the selected player or the handler's own player. The sObjectMgr->GetPlayerInfo gate is
// moot: race/class pairs are validated at character creation in the Go tree.
func (s *session) handleModifyGender(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyGender) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args (cs_modify.cpp:889-890).
		return false
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	genderStr := strings.Join(args, " ") // C++ matches the whole tail string, case-sensitive strncmp
	var gender uint8
	switch {
	case strings.HasPrefix("male", genderStr):
		if target.player.Gender == 0 {
			return true // already male: C++ returns true silently
		}
		gender = 0
	case strings.HasPrefix("female", genderStr):
		if target.player.Gender == 1 {
			return true // already female: C++ returns true silently
		}
		gender = 1
	default:
		s.sendSysMessage("You must specify male or female.") // LANG_MUST_MALE_OR_FEMALE 1119
		return true
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
	return true
}

// handleModifyBit mirrors HandleModifyBitCommand (cs_modify.cpp:616): the
// selected unit (getSelectedUnit: selected creature, else selected player,
// else self), security gate only for player targets, field = atoi, bit =
// atoi, field in [OBJECT_END, GetValuesCount) (creature UNIT_END 148,
// player PLAYER_END 1326, UpdateFields.h:30,175,392), bit in [1, 32], and
// the bit toggles via HasFlag with LANG_SET_BIT / LANG_REMOVE_BIT in
// (bit, field) order. The toggle rides the indexed-value backends
// (lua_values.go): a field the backend reads AND writes round-trips, and
// the backend's change-only broadcast carries it; a valid-range but
// unbacked field (e.g. player bytes0/faction/attack-time, creature
// offhand/ranged attack time) — or a bit the backend silently drops
// (partial-byte slots: creature bytes0/bytes2, player bytes1/bytes2) —
// reports honestly instead of claiming the toggle. The two LANG strings
// are trinity_string 161/162 ("You remove/set bit %u in field %u."),
// unseeded in-tree.
func (s *session) handleModifyBit(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyBit) {
		return false
	}
	if len(args) < 2 {
		// C++ returns false with no message on a missing field/bit token
		// (cs_modify.cpp:638-644).
		return false
	}
	if motion := s.castCommandSelectedCreature(); motion != nil {
		return s.modifyBitUnit(ctx, motionIndexedValueBackend(s.server, motion), args, 148)
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	return s.modifyBitUnit(ctx, playerIndexedValueBackend(target, target.player), args, 1326)
}

// modifyBitUnit is the shared toggle leg of handleModifyBit: range checks
// against valuesCount, then a read-modify-write through the backend. A
// write the backend cannot store (ok false, or stored == old so the bit
// did not stick) is an honest not-supported, never a false SET/REMOVE.
// Every leg prints a message, so it always returns true.
func (s *session) modifyBitUnit(ctx context.Context, backend indexedValueBackend, args []string, valuesCount int) bool {
	_ = ctx
	field := cAtoi(args[0])
	bit := cAtoi(args[1])
	if field < 6 || field >= valuesCount || bit < 1 || bit > 32 {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	old, ok := backend.read(field)
	if !ok {
		s.sendSysMessage("modify bit is not supported for this field: no Go value store backs it.")
		return true
	}
	mask := uint32(1) << uint(bit-1)
	fields, changed, ok := backend.write(field, func(v uint32) uint32 { return v ^ mask })
	if !ok || !changed {
		s.sendSysMessage("modify bit is not supported for this field: no Go value store backs it.")
		return true
	}
	backend.broadcast(fields)
	if old&mask != 0 {
		s.sendSysMessage(fmt.Sprintf("You remove bit %d in field %d.", bit, field))
		return true
	}
	s.sendSysMessage(fmt.Sprintf("You set bit %d in field %d.", bit, field))
	return true
}

// handleModifyFaction mirrors HandleModifyFactionCommand (cs_modify.cpp:209):
// the selected creature only (getSelectedCreature; players are never a
// target here). With no argument it reports the current faction/flags/
// npcflags/dyflags; otherwise it writes factionid plus the optional flag,
// npcflag and dyflag values, each defaulting to the current value, after
// the FactionTemplate.dbc lookup (LANG_WRONG_FACTION on a miss). The
// writes ride the motion slots plus a creature values-update broadcast,
// the SetFaction bridge pattern.
func (s *session) handleModifyFaction(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyFaction) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args
		// (cs_modify.cpp:212-213), before the creature check — so the
		// bare form never reports; the report leg is the wrong-link-type
		// branch below (cs_modify.cpp:225-232).
		return false
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return true
	}
	srv := s.server
	srv.motionMu.Lock()
	faction, flags, npcflags, dyflags := motion.Faction, motion.UnitFlags, motion.NPCFlags, motion.DynamicFlags
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	srv.motionMu.Unlock()
	low := uint32(guid & 0x00FFFFFF)
	if strings.HasPrefix(args[0], "|") {
		// C++ extractKeyFromLink (Chat.cpp:362): a |Hfaction:key| link
		// yields the key and tokenizing continues after the link tail;
		// a wrong link type yields null, i.e. the current-values report. The
		// synthetic first arg keeps its link tail: cAtoi stops at the
		// first non-digit, so it reads exactly the key.
		joined := strings.Join(args, " ")
		fi := strings.Index(joined, "Hfaction:")
		if fi < 0 {
			s.sendSysMessage(fmt.Sprintf("Current faction for creature %d is %d, flags %d, npcflags %d, dyflags %d.", low, faction, flags, npcflags, dyflags))
			return true
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
		return true
	}
	if _, found := s.factionTemplateEntry(faction); !found {
		s.sendSysMessage(fmt.Sprintf("Wrong faction %d.", faction))
		return true
	}
	srv.motionMu.Lock()
	motion.Faction, motion.UnitFlags, motion.NPCFlags, motion.DynamicFlags = faction, flags, npcflags, dyflags
	srv.motionMu.Unlock()
	srv.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldFaction: faction, unitFieldFlags: flags, unitFieldNPCFlags: npcflags, unitFieldDynamicFlags: dyflags})
	s.sendSysMessage(fmt.Sprintf("You changed faction for creature %d to %d, flags %d, npcflags %d, dyflags %d.", low, faction, flags, npcflags, dyflags))
	return true
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
func (s *session) handleModifyPhase(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyPhase) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args (cs_modify.cpp:835-836).
		return false
	}
	mask := uint32(cAtoi(args[0]))
	if motion := s.castCommandSelectedCreature(); motion != nil {
		srv := s.server
		srv.motionMu.Lock()
		motion.PhaseMask = mask
		srv.motionMu.Unlock()
		return true
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	target.player.PhaseMaskOverride = mask
	target.player.PhaseMaskOverrideSet = true
	return true
}

// handleModifySpeed implements the "speed" sub-table (cs_modify.cpp:47-56):
// all/backwalk/fly/walk/swim plus the "" entry (bare ".modify speed <val>"
// shares the all-speeds handler HandleModifyASpeedCommand and the root speed
// permission, 561). Rates go through the Unit::SetSpeedRate bridge
// (setGMSpeedRate): the SMSG_FORCE_*_SPEED_CHANGE to self and
// MSG_MOVE_SET_*_SPEED to nearby are real, and the ACK bookkeeping covers
// all nine move types.
func (s *session) handleModifySpeed(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .modify speed [all|backwalk|fly|walk|swim] <val>"
	if len(args) == 0 {
		// Bare ".modify speed": the speed node is a sub-table container
		// like the modify root, so its help listing prints (no arm runs).
		if !s.commandAllowed(ctx, permissionCommandModifySpeed) {
			return false
		}
		s.sendSysMessage(syntax)
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// Bare ".modify speed <val>" hits the "" table entry, which shares the
	// all-speeds handler and the root speed permission (561).
	if _, err := strconv.ParseFloat(sub, 32); err == nil {
		return s.modifySpeedAll(ctx, permissionCommandModifySpeed, args)
	}
	switch {
	case strings.HasPrefix("all", sub):
		return s.modifySpeedAll(ctx, permissionCommandModifySpeedAll, rest)
	case strings.HasPrefix("backwalk", sub):
		return s.modifySpeedType(ctx, permissionCommandModifySpeedBackwalk, rest, "back", []int{moveTypeRunBack}, true)
	case strings.HasPrefix("fly", sub):
		return s.modifySpeedType(ctx, permissionCommandModifySpeedFly, rest, "fly", []int{moveTypeFlight}, false)
	case strings.HasPrefix("walk", sub):
		return s.modifySpeedType(ctx, permissionCommandModifySpeedWalk, rest, "", []int{moveTypeRun}, true)
	case strings.HasPrefix("swim", sub):
		return s.modifySpeedType(ctx, permissionCommandModifySpeedSwim, rest, "swim", []int{moveTypeSwim}, true)
	default:
		// Unmatched token: C++ TryExecuteCommand breaks out of the
		// subcommand loop and invokes the last-matched node — the speed
		// node itself, whose "" entry is the all-speeds handler
		// (cs_modify.cpp:55, permission 561) — with the full unmatched
		// tail, so "2.5x" / "0x10" / "foo" atof through CheckModifySpeed
		// (2.5 / 16 / BAD_VALUE) instead of printing syntax.
		return s.modifySpeedAll(ctx, permissionCommandModifySpeed, args)
	}
}

// modifySpeedAll mirrors HandleModifyASpeedCommand (cs_modify.cpp:417):
// the WALK, RUN, SWIM and FLIGHT rates are all set to the same value.
func (s *session) modifySpeedAll(ctx context.Context, perm uint32, args []string) bool {
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	speed, target, ok, silent := s.checkModifySpeedTarget(ctx, args, true)
	if !ok {
		if silent {
			return false
		}
		return true
	}
	// LANG_YOU_CHANGE_ASPEED 137 / LANG_YOURS_ASPEED_CHANGED 138.
	s.notifyModify(target,
		fmt.Sprintf("You change %s's all speeds to %.2f.", target.player.Name, speed),
		fmt.Sprintf("%s changed your all speeds to %.2f.", s.player.Name, speed))
	for _, mtype := range []int{moveTypeWalk, moveTypeRun, moveTypeSwim, moveTypeFlight} {
		target.setGMSpeedRate(mtype, speed)
	}
	return true
}

// modifySpeedType mirrors the single-type arms (cs_modify.cpp:434-492):
// walk -> MOVE_RUN, backwalk -> MOVE_RUN_BACK, swim -> MOVE_SWIM,
// fly -> MOVE_FLIGHT. Only fly skips the in-flight check.
func (s *session) modifySpeedType(ctx context.Context, perm uint32, args []string, kind string, mtypes []int, checkInFlight bool) bool {
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	speed, target, ok, silent := s.checkModifySpeedTarget(ctx, args, checkInFlight)
	if !ok {
		if silent {
			return false
		}
		return true
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
	return true
}

// checkModifySpeedTarget mirrors modify_commandscript::CheckModifySpeed
// (cs_modify.cpp:379): the value must parse inside [0.1, 50.0]
// (LANG_BAD_VALUE 115), the target is the selected player or self with the
// lower-security guard, and a target in flight is rejected unless the arm
// skips the flight check (LANG_CHAR_IN_FLIGHT 21). The fourth result is
// true only for the silent legs (empty args, cs_modify.cpp:381-382, and
// the lower-security guard, cs_modify.cpp:403-404): callers return false
// on those and true on the messaged legs.
func (s *session) checkModifySpeedTarget(ctx context.Context, args []string, checkInFlight bool) (float32, *session, bool, bool) {
	if len(args) == 0 {
		return 0, nil, false, true
	}
	speed, ok := checkModifySpeedBounds(args[0], 0.1, 50.0)
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return 0, nil, false, false
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return 0, nil, false, false
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return 0, nil, false, true
	}
	if checkInFlight && target.isInFlight() {
		s.sendSysMessage(miscCharInFlight) // LANG_CHAR_IN_FLIGHT 21
		return 0, nil, false, false
	}
	return speed, target, true, false
}

// handleModifyTalentPoints mirrors HandleModifyTalentPointsCommand
// (cs_modify.cpp:334-372): the player arm sets stored free talent points
// and resends the talents frame; the pet arm sets the pet's
// UNIT_FIELD_BYTES_1 pet-talents byte (Pet::SetFreeTalentPoints,
// Pet.h:138) on a permanent pet of a player owner and resends the owner's
// pet talents frame. Go stores the pet value as the live motion's
// free-talent-points override, read back by petFreeTalentPoints and the
// owner's pet frame.
func (s *session) handleModifyTalentPoints(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandModifyTalentPoints) {
		return false
	}
	if len(args) == 0 {
		// C++ returns false with no message on empty args (cs_modify.cpp:336-337).
		return false
	}
	tp := cAtoi(args[0]) // C++ atoi: "abc" is 0 points, not a syntax error
	if tp < 0 {
		// C++ returns false with no message on a negative value (cs_modify.cpp:340-341).
		return false
	}
	if motion := s.castCommandSelectedCreature(); motion != nil {
		owner, silent := s.modifyTalentPointsPetOwner(ctx, motion)
		if silent {
			return false
		}
		if owner == nil {
			// C++ getSelectedUnit arm: a non-player, non-pet unit gets
			// LANG_NO_CHAR_SELECTED instead of the invoker's points.
			s.sendSysMessage("No character selected.")
			return true
		}
		s.server.motionMu.Lock()
		motion.PetFreeTalentPointsOverride = uint32(tp)
		motion.PetFreeTalentPointsOverrideSet = true
		s.server.motionMu.Unlock()
		_ = owner.sendTalentsInfo(true)
		return true
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil {
		return true
	}
	if s.modifyTargetLowerSecurity(ctx, target) {
		return false
	}
	// cs_modify.cpp:356: target->ToPlayer()->SetFreeTalentPoints(tp) —
	// the override write + event-16 fire live in the shared
	// setFreeTalentPointsOverride (skills.go); the hook fires for the
	// target player, matching the C++ call on the target.
	target.setFreeTalentPointsOverride(ctx, uint32(tp))
	_ = target.sendTalentsInfo(false)
	return true
}

// modifyTalentPointsPetOwner mirrors the pet leg of
// HandleModifyTalentPointsCommand (cs_modify.cpp:356-366): the selected
// unit must be a pet (Unit::IsPet) with a player owner, and a permanent
// pet for that owner (Pet::IsPermanentPetFor, Pet.cpp:1832-1847), with the
// lower-security guard on the owner. It returns the owner's session, or
// nil when the leg does not apply (the caller prints
// LANG_NO_CHAR_SELECTED); silent is true when the security guard failed
// and the caller must stay silent.
func (s *session) modifyTalentPointsPetOwner(ctx context.Context, motion *creatureMotion) (owner *session, silent bool) {
	if motion == nil || !motionIsPet(motion) || motion.OwnerGUID == 0 || s.server == nil {
		return nil, false
	}
	owner = s.server.findSessionByGUID(motion.OwnerGUID)
	if owner == nil || owner.player == nil {
		return nil, false
	}
	if !s.modifyPetIsPermanentForOwner(ctx, motion, owner) {
		return nil, false
	}
	if s.modifyTargetLowerSecurity(ctx, owner) {
		return nil, true
	}
	return owner, false
}

// modifyPetIsPermanentForOwner mirrors Pet::IsPermanentPetFor
// (Pet.cpp:1832-1847): HUNTER_PET is always permanent; SUMMON_PET is
// permanent for a warlock's demon or a death knight's undead pet. The
// creature type comes from creature_template — pet motions carry no
// CreatureType — matching the sendPetSpells precedent (pets.go:1842).
func (s *session) modifyPetIsPermanentForOwner(ctx context.Context, motion *creatureMotion, owner *session) bool {
	if motion == nil || owner == nil || owner.player == nil {
		return false
	}
	switch motion.PetType {
	case petTypeHunter:
		return true
	case petTypeSummon:
		if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return false
		}
		var creatureType int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", motion.Entry).Scan(&creatureType); err != nil {
			return false
		}
		return owner.player.Class == 9 && uint32(creatureType) == creatureTypeDemon ||
			owner.player.Class == 6 && uint32(creatureType) == creatureTypeUndead
	default:
		return false
	}
}

// handleModifyChunk2b extends the modify dispatcher with the chunk-2b arms.
// The chunk-2c arm (reputation) chains off the default case. It returns
// (handled, ok): handled is false when no arm matched; ok is the matched
// arm's bool.
func (s *session) handleModifyChunk2b(ctx context.Context, sub string, rest []string) (bool, bool) {
	switch {
	case strings.HasPrefix("gender", sub):
		return true, s.handleModifyGender(ctx, rest)
	case strings.HasPrefix("bit", sub):
		return true, s.handleModifyBit(ctx, rest)
	case strings.HasPrefix("faction", sub):
		return true, s.handleModifyFaction(ctx, rest)
	case strings.HasPrefix("phase", sub):
		return true, s.handleModifyPhase(ctx, rest)
	case strings.HasPrefix("speed", sub):
		return true, s.handleModifySpeed(ctx, rest)
	case strings.HasPrefix("talentpoints", sub):
		return true, s.handleModifyTalentPoints(ctx, rest)
	default:
		return s.handleModifyChunk2c(ctx, sub, rest)
	}
}
