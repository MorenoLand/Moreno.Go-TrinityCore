// reset command port: reset_commandscript (cs_reset.cpp), the "reset" root
// with 7 arms (achievements, honor, level, spells, stats, talents, all).
// THIRTY-THIRD of 39 Commands groups (cs_script_loader.cpp decl 52 / call 97;
// call order reload(96) -> reset(97)). Trinity checks permission only on the
// invoker leaf node (ChatCommand.cpp:487), so each arm gates exactly its own
// C++ permission (RBAC.h:578-585, 8 constants in permissions.go); the root
// permission 710 covers the bare `.reset`.
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
//     the C++ quirk of setting only the pet-talent flag. The hunter-pet
//     selection branch and Pet::resetTalentsForAllPetsOf have no bridge: pet
//     talents are unmodeled (the learn port documents the same gap), and Go
//     selection tracks players only.
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
//     "spells" and "talents" match, like the C++ == comparison.
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
func (s *session) handleCmdReset(ctx context.Context, args []string) {
	const syntax = "Syntax: .reset achievements|honor|level|spells|stats|talents [$player] | .reset all spells|talents"
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandReset) {
			return
		}
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("achievements", sub):
		if s.miscDeny(ctx, permissionCommandResetAchievements) {
			return
		}
		s.handleResetAchievements(ctx, args[1:])
	case strings.HasPrefix("honor", sub):
		if s.miscDeny(ctx, permissionCommandResetHonor) {
			return
		}
		s.handleResetHonor(ctx, args[1:])
	case strings.HasPrefix("level", sub):
		if s.miscDeny(ctx, permissionCommandResetLevel) {
			return
		}
		s.handleResetLevel(ctx, args[1:])
	case strings.HasPrefix("spells", sub):
		if s.miscDeny(ctx, permissionCommandResetSpells) {
			return
		}
		s.handleResetSpells(ctx, args[1:])
	case strings.HasPrefix("stats", sub):
		if s.miscDeny(ctx, permissionCommandResetStats) {
			return
		}
		s.handleResetStats(ctx, args[1:])
	case strings.HasPrefix("talents", sub):
		if s.miscDeny(ctx, permissionCommandResetTalents) {
			return
		}
		s.handleResetTalents(ctx, args[1:])
	case strings.HasPrefix("all", sub):
		if s.miscDeny(ctx, permissionCommandResetAll) {
			return
		}
		s.handleResetAll(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
	}
}

// handleResetAchievements mirrors HandleResetAchievementsCommand
// (cs_reset.cpp:66-79): online targets get their achievement state wiped and
// the client refreshed; offline targets get the DB rows deleted.
func (s *session) handleResetAchievements(ctx context.Context, args []string) {
	target, guid, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	chars := s.server.CharactersStore
	if chars == nil || chars.DB == nil {
		return
	}
	_, _ = chars.DB.ExecContext(ctx, "DELETE FROM character_achievement WHERE guid = ?", guid)
	_, _ = chars.DB.ExecContext(ctx, "DELETE FROM character_achievement_progress WHERE guid = ?", guid)
	if target != nil {
		target.earnedAchievements = make(map[uint32]uint32)
		target.criteriaProgress = make(map[uint32]*criteriaProgressState)
		target.sendAllAchievementData()
	}
}

// handleResetHonor mirrors HandleResetHonorCommand (cs_reset.cpp:81-95):
// online targets only.
func (s *session) handleResetHonor(ctx context.Context, args []string) {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == nil || target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	t := target
	t.player.TotalHonorPoints = 0     // Player::SetHonorPoints(0)
	t.player.TodayKills = 0           // PLAYER_FIELD_KILLS
	t.player.TotalKills = 0           // PLAYER_FIELD_LIFETIME_HONORABLE_KILLS
	t.player.TodayHonorPoints = 0     // PLAYER_FIELD_TODAY_CONTRIBUTION
	t.player.YesterdayHonorPoints = 0 // PLAYER_FIELD_YESTERDAY_CONTRIBUTION
	t.persistHonorFields(ctx)
	t.sendPlayerUpdate()
	// Player::UpdateAchievementCriteria(ACHIEVEMENT_CRITERIA_TYPE_EARN_HONORABLE_KILL).
	t.updateAchievementCriteria(criteriaTypeEarnHonorableKill, 0, 1)
}

// handleResetLevel mirrors HandleResetLevelCommand (cs_reset.cpp:129-163):
// online targets only. The HandleResetStatsOrLevelHelper unit-field resets
// and the Init*ForLevel / pet-sync / script-hook steps have no Go bridge
// (see file header); the native core resets level and XP to the configured
// start level.
func (s *session) handleResetLevel(ctx context.Context, args []string) {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == nil || target.player == nil {
		s.sendSysMessage("Player not found.")
		return
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
	t.sendPlayerUpdate()
}

// handleResetSpells mirrors HandleResetSpellsCommand (cs_reset.cpp:165-192).
func (s *session) handleResetSpells(ctx context.Context, args []string) {
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target != nil && target.player != nil {
		// Player::ResetSpells(), run immediately via the at-login reset
		// (identical DELETE + SMSG_REMOVED_SPELL sequence).
		_ = target.resetSpellsAtLogin(ctx)
		target.sendSysMessage("Your spells have been reset.") // LANG_RESET_SPELLS 215
		if s.player == nil || s.playerGUID != target.playerGUID {
			s.sendSysMessage(miscPlayerLink(name) + "'s spells have been reset.") // LANG_RESET_SPELLS_ONLINE 211
		}
		return
	}
	chars := s.server.CharactersStore
	if chars == nil {
		return
	}
	_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(atLoginResetSpells), guid)
	s.sendSysMessage("Spells of " + miscPlayerLink(name) + " will be reset on next login.") // LANG_RESET_SPELLS_OFFLINE 212
}

// handleResetStats mirrors HandleResetStatsCommand (cs_reset.cpp:194-210):
// online targets only. The helper and Init*ForLevel recompute steps have no
// Go bridge (see file header); the native core is a client refresh.
func (s *session) handleResetStats(ctx context.Context, args []string) {
	target, _, _, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == nil || target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	target.sendPlayerUpdate()
}

// handleResetTalents mirrors HandleResetTalentsCommand (cs_reset.cpp:212-270).
// The hunter-pet selection branch has no bridge (Go selection tracks players
// only, and pet talents are unmodeled); it reports LANG_NO_CHAR_SELECTED
// like the C++ fallthrough.
func (s *session) handleResetTalents(ctx context.Context, args []string) {
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target != nil && target.player != nil {
		// Player::ResetTalents(true) + SendTalentsInfoData(false); the pet
		// talent reset (Pet::resetTalentsForAllPetsOf) has no bridge.
		if target.resetTalents(ctx, true) {
			_ = target.sendTalentsInfo(false)
		}
		target.sendSysMessage("Your talents have been reset.") // LANG_RESET_TALENTS 216
		if s.player == nil || s.playerGUID != target.playerGUID {
			s.sendSysMessage(miscPlayerLink(name) + "'s talents have been reset.") // LANG_RESET_TALENTS_ONLINE 213
		}
		return
	}
	if guid != 0 {
		chars := s.server.CharactersStore
		if chars == nil {
			return
		}
		// C++ sets AT_LOGIN_NONE | AT_LOGIN_RESET_PET_TALENTS.
		_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(atLoginResetPetTalents), guid)
		s.sendSysMessage("Talents of " + miscPlayerLink(name) + " will be reset on next login.") // LANG_RESET_TALENTS_OFFLINE 214
		return
	}
	s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
}

// handleResetAll mirrors HandleResetAllCommand (cs_reset.cpp:272-312): only
// the exact case names "spells" and "talents" match.
func (s *session) handleResetAll(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .reset all spells|talents")
		return
	}
	var atLogin uint64
	var worldText string
	switch args[0] {
	case "spells":
		atLogin = atLoginResetSpells
		worldText = "Reset all spells." // LANG_RESETALL_SPELLS 218
	case "talents":
		atLogin = atLoginResetTalents | atLoginResetPetTalents
		worldText = "Reset all talents." // LANG_RESETALL_TALENTS 219
	default:
		s.sendSysMessage("Unknown case " + args[0] + ". Use one of: spells, talents.") // LANG_RESETALL_UNKNOWN_CASE 217
		return
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
}
