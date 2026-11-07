// reset command port: reset_commandscript (cs_reset.cpp), the "reset" root
// with 7 arms (achievements, honor, level, spells, stats, talents, all).
// THIRTY-FOURTH of 40 Commands groups (cs_script_loader.cpp decl 52 /
// call 97; call order reload(96) -> reset(97)). Trinity checks permission
// Trinity checks permission only on the invoker leaf node (ChatCommand.cpp:487), so each arm gates
// exactly its own C++ permission (RBAC.h:579-585, 7 constants in
// permissions.go); the root's RBAC_PERM_COMMAND_RESET 710 is dead in C++
// (deprecated 6-arg nullptr+subtable overload drops it, same as the
// reload/rbac/pet/quest/modify/mmap roots), so bare ".reset" returns false
// (the framework shows the help listing) and permissionCommandReset stays
// in permissions.go as documentation.
//
// R13: the root and all 7 arms return bool — bare/unknown subcommands return
// false, denial is a silent false, ambiguous subcommands list both and
// return true, and every message leg returns true. extractPlayerTarget
// failures already sent "Player not found." by miscResolvePlayerTarget, so
// those arms return true; the talents no-target leg prints
// LANG_NO_CHAR_SELECTED and returns true.
//
// Player targeting mirrors ChatHandler::extractPlayerTarget via
// miscResolvePlayerTarget (name -> selection -> self, offline guids resolved
// through the characters table). cs_reset.cpp never calls HasLowerSecurity,
// so neither does this port.
//
// Documented fidelity gaps (not stubs):
//   - HandleResetStatsOrLevelHelper (cs_reset.cpp:97-127): the ChrClasses
//     DisplayPower power-type reset, shapeshift-form reset, SetFactionForRace,
//     InitDisplayIds, and the watched-faction/unit-flag field resets have no
//     Go bridge (no ChrClasses DBC accessor and no unit-field model in the
//     world package). The class lookup itself is moot in Go: players carry
//     their power type from creation.
//   - reset level (cs_reset.cpp:129-163): InitRunes, InitStatsForLevel,
//     InitTaxiNodesForLevel, InitGlyphsForLevel, InitTalentForLevel have no Go
//     bridge (the character-level arm documents the same gap), pet
//     SynchronizeLevelWithOwner has no Go pet-level model, and the
//     OnPlayerLevelChanged script hook has no script bridge. Native: level
//     and XP are reset to the configured start level (worldserver.conf
//     StartPlayerLevel / StartDeathKnightPlayerLevel, DK default 55 like
//     character creation) and the client is refreshed.
//   - reset stats (cs_reset.cpp:194-210): same helper gaps plus the
//     Init*ForLevel stat-recompute calls; the Go engine holds derived stats
//     on the live state, so the native core is a client refresh via
//     sendPlayerUpdate, exactly as much as the engine can recompute.
//   - reset spells (cs_reset.cpp:165-192): online targets run the existing
//     at-login spell reset immediately (resetSpellsAtLogin: DELETE
//     character_spell + SMSG_REMOVED_SPELL per spell, the Go equivalent of
//     Player::ResetSpells); offline targets get CHAR_UPD_ADD_AT_LOGIN_FLAG
//     with AT_LOGIN_RESET_SPELLS, which the existing at-login processor
//     honors.
//   - reset talents (cs_reset.cpp:212-270): online targets run the existing
//     resetTalents(ctx, free=true) + SendTalentsInfoData(false); offline
//     targets get at_login 0x010 (AT_LOGIN_RESET_PET_TALENTS), faithful to
//     the C++ quirk of setting only the pet-talent flag. The online pet leg
//     is ported via resetActivePetTalents: Pet::resetTalentsForAllPetsOf
//     (Pet.cpp:1702) clears the pending pet-talent at-login flag, resets
//     the online pet's talents, and the handler sends the pet frame — but
//     only when the pet survived ResetTalents, which dismisses it on the
//     reset path (Player.cpp:4019), so GetPet() is null there and the leg
//     is a no-op; on the no-talents path the pet stays and its talents
//     reset. The stabled/offline DELETE leg is dead in C++ (inverted
//     `if (!petIds.empty()) return;`, Pet.cpp:1724-1725) and is not ported.
//     The hunter-pet selection branch (cs_reset.cpp:224-241) is dead in
//     C++ too: extract with empty args returns the invoker for any in-game
//     session (Chat.cpp:344-356) and console has no selection, so extract
//     never fails with a pet selected; only its LANG_NO_CHAR_SELECTED
//     fallthrough message is ported.
//   - reset achievements (cs_reset.cpp:66-79): native — DELETE FROM
//     character_achievement and character_achievement_progress (the Go
//     equivalent of AchievementMgr::ResetAchievements / DeleteFromDB for
//     both online and offline targets) plus clearing the session's in-memory
//     earned/criteria maps and resending all-achievement-data for online
//     targets.
//   - reset honor (cs_reset.cpp:81-95): native — zeroes the five C++ fields
//     (SetHonorPoints + PLAYER_FIELD_KILLS + PLAYER_FIELD_LIFETIME_HONORABLE_KILLS
//   - PLAYER_FIELD_TODAY/YESTERDAY_CONTRIBUTION, mapped to TotalHonorPoints,
//     TodayKills, TotalKills, TodayHonorPoints, YesterdayHonorPoints;
//     YesterdayKills is untouched, like C++), persists, refreshes the client,
//     and pokes the EarnHonorableKill criterion exactly like C++.
//   - reset all (cs_reset.cpp:272-312): native — ORs the at-login flag onto
//     every characters row (CHAR_UPD_ALL_AT_LOGIN_FLAGS) and every online
//     session, then broadcasts the C++ world text. Only the exact case names
//     "spells" and "talents" match, like the C++ == comparison. With no case
//     arg C++ returns false with no message, so the Go arm returns false
//     too (the framework shows the help listing).
//
// LANG texts are inlined from TDB enUS (no in-tree trinity_string seed), per
// tree convention; the LANG id is cited on each message.
package world

import (
	"context"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

// handleCmdReset dispatches the "reset" arms (cs_reset.cpp:51-62).
func (s *session) handleCmdReset(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Bare ".reset": the root's RBAC_PERM_COMMAND_RESET is dead in C++
		// (deprecated 6-arg nullptr+subtable overload drops it) and the
		// root has no handler, so the framework shows the help listing.
		return false
	}
	// The Trinity parser prefix-matches (ChatCommand.cpp:262-288): an
	// exact token wins, one prefix match executes, and several print the
	// ambiguous list instead of executing the first match (previously
	// "a" silently ran achievements and "s" silently ran spells, where
	// C++ lists achievements+all and spells+stats). Order is the C++
	// table order (cs_reset.cpp:51-57).
	type resetArm struct {
		name string
		perm uint32
		run  func(context.Context, []string) bool
	}
	arms := []resetArm{
		{"achievements", permissionCommandResetAchievements, s.handleResetAchievements},
		{"honor", permissionCommandResetHonor, s.handleResetHonor},
		{"level", permissionCommandResetLevel, s.handleResetLevel},
		{"spells", permissionCommandResetSpells, s.handleResetSpells},
		{"stats", permissionCommandResetStats, s.handleResetStats},
		{"talents", permissionCommandResetTalents, s.handleResetTalents},
		{"all", permissionCommandResetAll, s.handleResetAll},
	}
	// Prefix-match over the invoker-visible arms only
	// (FilteredCommandListIterator, ChatCommand.cpp:207-237): a prefix
	// that matches several names but leaves one visible resolves to it,
	// and names the invoker cannot see never surface as ambiguous.
	visible := func(perm uint32) bool { return s.commandAllowed(ctx, perm) }
	subs := make([]groupSubcommand, len(arms))
	for i, arm := range arms {
		subs[i] = groupSubcommand{arm.name, []uint32{arm.perm}}
	}
	sub, cands, ok := groupMatchNameVisible(args[0], subs, visible)
	if !ok {
		if len(cands) > 1 {
			s.groupAmbiguousMessage(args[0], cands)
			return true
		}
		return false
	}
	for _, arm := range arms {
		if arm.name == sub {
			if !s.commandAllowed(ctx, arm.perm) {
				return false
			}
			return arm.run(ctx, args[1:])
		}
	}
	return false
}

// handleResetAchievements mirrors HandleResetAchievementsCommand
// (cs_reset.cpp:66-79): online targets get their achievement state wiped and
// the client refreshed; offline targets get the DB rows deleted.
func (s *session) handleResetAchievements(ctx context.Context, args []string) bool {
	target, guid, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// extractPlayerTarget already sent LANG_PLAYER_NOT_FOUND.
		return true
	}
	chars := s.server.CharactersStore
	if chars == nil || chars.DB == nil {
		return false
	}
	_, _ = chars.DB.ExecContext(ctx, "DELETE FROM character_achievement WHERE guid = ?", guid)
	_, _ = chars.DB.ExecContext(ctx, "DELETE FROM character_achievement_progress WHERE guid = ?", guid)
	if target != nil {
		target.resetAchievements(ctx)
	}
	return true
}

// handleResetHonor mirrors HandleResetHonorCommand (cs_reset.cpp:81-95):
// online targets only.
func (s *session) handleResetHonor(ctx context.Context, args []string) bool {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// extractPlayerTarget already sent LANG_PLAYER_NOT_FOUND.
		return true
	}
	if target == nil || target.player == nil {
		// The 2-arg C++ extract allows online targets only: an offline
		// named player prints LANG_PLAYER_NOT_FOUND and returns false
		// (Chat.cpp:657-710), i.e. message + true here.
		s.sendSysMessage("Player not found.")
		return true
	}
	t := target
	t.player.TotalHonorPoints = 0     // Player::SetHonorPoints(0)
	t.player.TodayKills = 0           // PLAYER_FIELD_KILLS
	t.player.TotalKills = 0           // PLAYER_FIELD_LIFETIME_HONORABLE_KILLS
	t.player.TodayHonorPoints = 0     // PLAYER_FIELD_TODAY_CONTRIBUTION
	t.player.YesterdayHonorPoints = 0 // PLAYER_FIELD_YESTERDAY_CONTRIBUTION
	t.persistHonorFields(ctx)
	t.sendPlayerUpdate()
	// Player::UpdateAchievementCriteria(ACHIEVEMENT_CRITERIA_TYPE_EARN_HONORABLE_KILL)
	// ignores its misc values: the C++ switch SETS progress to the player's
	// current lifetime kills (AchievementMgr.cpp:1039), which this arm just
	// zeroed — so this is a set-to-zero, not an increment.
	t.setAchievementCriteria(criteriaTypeEarnHonorableKill, 0, t.player.TotalKills)
	return true
}

// handleResetLevel mirrors HandleResetLevelCommand (cs_reset.cpp:129-163):
// online targets only. The HandleResetStatsOrLevelHelper unit-field resets
// and the Init*ForLevel / pet-sync / script-hook steps have no Go bridge
// (see file header); the native core resets level and XP to the configured
// start level.
func (s *session) handleResetLevel(ctx context.Context, args []string) bool {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// extractPlayerTarget already sent LANG_PLAYER_NOT_FOUND.
		return true
	}
	if target == nil || target.player == nil {
		// Online-only arm (2-arg extract): offline named player prints
		// LANG_PLAYER_NOT_FOUND (Chat.cpp:657-710), message + true.
		s.sendSysMessage("Player not found.")
		return true
	}
	t := target
	startLevel := uint8(1)
	if s.server.Config.StartPlayerLevel > 0 {
		startLevel = uint8(s.server.Config.StartPlayerLevel)
	}
	if t.player.Class == 6 { // Death Knight, like character creation
		if s.server.Config.StartDeathKnightPlayerLevel > 0 {
			startLevel = uint8(s.server.Config.StartDeathKnightPlayerLevel)
		} else {
			startLevel = 55
		}
	}
	t.player.Level = startLevel // Player::SetLevel(startLevel)
	t.player.XP = 0             // Player::SetXP(0)
	// AutoBalance_PlayerScript::OnLevelChanged (AutoBalance.cpp).
	t.autoBalanceOnLevelChanged()
	t.sendPlayerUpdate()
	return true
}

// handleResetSpells mirrors HandleResetSpellsCommand (cs_reset.cpp:165-192).
func (s *session) handleResetSpells(ctx context.Context, args []string) bool {
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// extractPlayerTarget already sent LANG_PLAYER_NOT_FOUND.
		return true
	}
	if target != nil && target.player != nil {
		// Player::ResetSpells(), run immediately via the at-login reset
		// (identical DELETE + SMSG_REMOVED_SPELL sequence).
		_ = target.resetSpellsAtLogin(ctx)
		target.sendSysMessage("Your spells have been reset.") // LANG_RESET_SPELLS 215
		if s.player == nil || s.playerGUID != target.playerGUID {
			s.sendSysMessage(miscPlayerLink(name) + "'s spells have been reset.") // LANG_RESET_SPELLS_ONLINE 211
		}
		return true
	}
	chars := s.server.CharactersStore
	if chars == nil {
		return false
	}
	_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(atLoginResetSpells), guid)
	s.sendSysMessage("Spells of " + miscPlayerLink(name) + " will be reset on next login.") // LANG_RESET_SPELLS_OFFLINE 212
	return true
}

// handleResetStats mirrors HandleResetStatsCommand (cs_reset.cpp:194-210):
// online targets only. The helper and Init*ForLevel recompute steps have no
// Go bridge (see file header); the native core is a client refresh.
func (s *session) handleResetStats(ctx context.Context, args []string) bool {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// extractPlayerTarget already sent LANG_PLAYER_NOT_FOUND.
		return true
	}
	if target == nil || target.player == nil {
		// Online-only arm (2-arg extract): offline named player prints
		// LANG_PLAYER_NOT_FOUND (Chat.cpp:657-710), message + true.
		s.sendSysMessage("Player not found.")
		return true
	}
	target.sendPlayerUpdate()
	return true
}

// handleResetTalents mirrors HandleResetTalentsCommand (cs_reset.cpp:212-270).
func (s *session) handleResetTalents(ctx context.Context, args []string) bool {
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		// C++ sends LANG_NO_CHAR_SELECTED after extractPlayerTarget
		// already sent LANG_PLAYER_NOT_FOUND (cs_reset.cpp:238-240);
		// the hunter-pet selection branch above it is dead (see file
		// header), so only the fallthrough message is ported. C++ returns
		// false with SetSentErrorMessage: message + true here.
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return true
	}
	if target != nil && target.player != nil {
		// Player::ResetTalents(true) + SendTalentsInfoData(false) go out
		// unconditionally — cs_reset.cpp ignores the return.
		target.resetTalents(ctx, true)
		_ = target.sendTalentsInfo(false)
		target.sendSysMessage("Your talents have been reset.") // LANG_RESET_TALENTS 216
		if s.player == nil || s.playerGUID != target.playerGUID {
			s.sendSysMessage(miscPlayerLink(name) + "'s talents have been reset.") // LANG_RESET_TALENTS_ONLINE 213
		}
		// Pet::resetTalentsForAllPetsOf (Pet.cpp:1702-1706) clears the
		// pending AT_LOGIN_RESET_PET_TALENTS flag before anything else.
		if target.player.AtLogin&uint32(atLoginResetPetTalents) != 0 {
			target.player.AtLogin &^= uint32(atLoginResetPetTalents)
			if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
				_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~16 WHERE guid = ?", target.playerGUID)
			}
		}
		// The online-pet leg runs only when the pet survived
		// ResetTalents: the reset path dismisses it (Player.cpp:4019),
		// so GetPet() is null there; on the no-talents path the pet
		// stays. resetActivePetTalents sends the pet frame on success;
		// C++ sends SendTalentsInfoData(true) whenever a pet exists
		// (cs_reset.cpp:254-255), so send it when the reset no-ops.
		if target.player.PetGUID != 0 {
			if !target.resetActivePetTalents(ctx) {
				_ = target.sendTalentsInfo(true)
			}
		}
		return true
	}
	if guid != 0 {
		chars := s.server.CharactersStore
		if chars == nil {
			return false
		}
		// C++ sets AT_LOGIN_NONE | AT_LOGIN_RESET_PET_TALENTS.
		_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(atLoginResetPetTalents), guid)
		s.sendSysMessage("Talents of " + miscPlayerLink(name) + " will be reset on next login.") // LANG_RESET_TALENTS_OFFLINE 214
		return true
	}
	s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
	return true
}

// handleResetAll mirrors HandleResetAllCommand (cs_reset.cpp:272-312): only
// the exact case names "spells" and "talents" match.
func (s *session) handleResetAll(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// C++ returns false with no message (cs_reset.cpp:274-275).
		return false
	}
	// C++ compares the whole tail string (caseName == "spells"), so a
	// multi-token tail like "spells foo" is unknown, not a spells reset
	// (previously the first token alone decided).
	var atLogin uint64
	var worldText string
	if len(args) == 1 {
		switch args[0] {
		case "spells":
			atLogin = atLoginResetSpells
			worldText = "Reset all spells." // LANG_RESETALL_SPELLS 218
		case "talents":
			atLogin = atLoginResetTalents | atLoginResetPetTalents
			worldText = "Reset all talents." // LANG_RESETALL_TALENTS 219
		}
	}
	if worldText == "" {
		// C++ prints LANG_RESETALL_UNKNOWN_CASE + SetSentErrorMessage and
		// returns false: message + true here.
		s.sendSysMessage("Unknown case " + strings.Join(args, " ") + ". Use one of: spells, talents.") // LANG_RESETALL_UNKNOWN_CASE 217
		return true
	}
	// CHAR_UPD_ALL_AT_LOGIN_FLAGS: UPDATE characters SET at_login = at_login | ?.
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login | ?", atLogin)
	}
	s.server.sessionsMu.RLock()
	for sess := range s.server.sessions {
		if sess != nil && sess.player != nil {
			sess.player.AtLogin |= uint32(atLogin)
		}
	}
	s.server.sessionsMu.RUnlock()
	s.server.broadcastMessageChatAll(worldText) // sWorld->SendWorldText
	if s.player == nil {
		s.sendSysMessage(worldText)
	}
	return true
}
