// Port of the TrinityCore guild_commandscript (cs_guild.cpp) into the Go
// command dispatcher.
//
// Sole-source audit: the whole cs_guild.cpp is a single column-0 `class`
// def (`guild_commandscript`); the name hits only cs_guild.cpp and
// cs_script_loader.cpp (decl 36 / call 81), so this unit is the EIGHTEENTH
// Commands group in loader call order (group(80) -> guild(81) -> honor(82)).
// Registration: AddSC_guild_commandscript() = `new guild_commandscript();`
//
// 7 arms: guild create (402), delete (403), invite (404), uninvite (405),
// rank (406), rename (407), info (794). Trinity's ChatCommandNode only
// permission-checks the invoker (leaf) node, so each arm gates its own RBAC
// permission exactly like the C++.
//
// LANG texts are inlined from TDB enUS recall (the tree carries no
// trinity_string seed); the LANG id is cited on each message.
//
// Documented fidelity gaps (not stubs): sObjectMgr->IsReservedName /
// ObjectMgr::IsValidCharterName have no Go bridge — create/rename enforce
// the empty and >24-char rejects only; guild ids come from
// MAX(guildid)+1 instead of GuildMgr's in-memory NextGuildId; the
// sScriptMgr OnGuild* hooks have no Go script bridge (consistent with the
// rest of the tree).
package world

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleCmdGuild dispatches the guild sub-commands, mirroring
// guild_commandscript::GetCommands (cs_guild.cpp:46-69).
func (s *session) handleCmdGuild(ctx context.Context, args []string) {
	const syntax = "Syntax: .guild create [<player>] \"<name>\" | .guild delete \"<name>\" | .guild invite [<player>] \"<name>\" | .guild uninvite <player> | .guild rank [<player>] <rank> | .guild rename \"<old>\" \"<new>\" | .guild info [id|\"<name>\"]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	deny := func(perm uint32) bool {
		if !s.commandAllowed(ctx, perm) {
			s.sendNotification("You do not have permission to use that command.")
			return true
		}
		return false
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "create":
		if deny(permissionCommandGuildCreate) {
			return
		}
		s.handleGuildCreateCommand(ctx, args[1:])
	case "delete":
		if deny(permissionCommandGuildDelete) {
			return
		}
		s.handleGuildDeleteCommand(ctx, args[1:])
	case "invite":
		if deny(permissionCommandGuildInvite) {
			return
		}
		s.handleGuildInviteCommand(ctx, args[1:])
	case "uninvite":
		if deny(permissionCommandGuildUninvite) {
			return
		}
		s.handleGuildUninviteCommand(ctx, args[1:])
	case "rank":
		if deny(permissionCommandGuildRank) {
			return
		}
		s.handleGuildRankCommand(ctx, args[1:])
	case "rename":
		if deny(permissionCommandGuildRename) {
			return
		}
		s.handleGuildRenameCommand(ctx, args[1:])
	case "info":
		if deny(permissionCommandGuildInfo) {
			return
		}
		s.handleGuildInfoCommand(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
	}
}

// guildCharactersDB mirrors the arenaCharactersDB accessor for the guild
// GM arms, which all read/write the characters database.
func (s *session) guildCharactersDB() *sql.DB {
	if s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	return s.server.CharactersStore.DB
}

// guildIDByName mirrors sGuildMgr->GetGuildByName.
func guildIDByName(ctx context.Context, cdb *sql.DB, name string) uint32 {
	var id uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE name = ? LIMIT 1", name).Scan(&id); err != nil {
		return 0
	}
	return id
}

// guildLowestRankID mirrors Guild::_GetLowestRankId (highest rid), the rank
// newly invited members receive.
func guildLowestRankID(ctx context.Context, cdb *sql.DB, guildID uint32) uint8 {
	var maxRank int64 = 4
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(rid), 4) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRank)
	if maxRank > 255 {
		maxRank = 255
	}
	return uint8(maxRank)
}

// broadcastGuildEventTo delivers a guild event packet to every online guild
// member, the Go shape of Guild::_BroadcastEvent (Guild.cpp:2430).
func (s *session) broadcastGuildEventTo(guildID uint32, payload []byte) {
	if s.server == nil {
		return
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), payload, true)
	}
}

// guildAddMember mirrors Guild::AddMember (Guild.cpp:2195) for the GM
// invite/create arms: the target must be guildless, petitions are cleaned,
// the member row is inserted at the given rank, an online target is put in
// the guild and sent its login info, the JOIN event is logged and GE_JOINED
// broadcast to every online member.
func (s *session) guildAddMember(ctx context.Context, cdb *sql.DB, guildID uint32, guid uint64, rankID uint8, name string) bool {
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil && ts.player.GuildID != 0 {
		return false
	}
	var existing uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", guid).Scan(&existing); err == nil {
		return false
	}
	// Player::RemovePetitionsAndSigns(guid, GUILD_CHARTER_TYPE): prevent
	// corrupt data from a pending charter (Guild.cpp:2209). Best-effort.
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE playerguid = ?", guid)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition WHERE owerguid = ?", guid)
	if _, err := cdb.ExecContext(ctx, "INSERT INTO guild_member (guildid, guid, `rank`, pnote, offnote) VALUES (?, ?, ?, '', '')", guildID, guid, rankID); err != nil {
		return false
	}
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		ts.player.GuildID = guildID
		ts.player.GuildRank = rankID
		ts.sendPlayerUpdate()
		ts.sendGuildLoginInfo(ctx) // Guild::AddMember -> SendLoginInfo (Guild.cpp:2230)
	}
	s.logGuildEvent(ctx, guildID, guildEventLogJoinGuild, guid, 0, 0)
	s.broadcastGuildEventTo(guildID, guildEventPayload(guildEventJoined, guid, name))
	return true
}

// guildDeleteMember mirrors Guild::DeleteMember(trans, guid, false, true,
// true) (Guild.cpp:2277) for the GM uninvite arm. A removed leader promotes
// the lowest-rank remaining member (rank 0), broadcasting GE_LEADER_CHANGED
// then GE_LEFT while the old leader is still a member; with no successor the
// guild disbands. Non-leader removal is silent, exactly as in C++ —
// DeleteMember itself logs and broadcasts nothing.
func (s *session) guildDeleteMember(ctx context.Context, cdb *sql.DB, guildID uint32, guid uint64) {
	var leaderGUID uint64
	_ = cdb.QueryRowContext(ctx, "SELECT leaderguid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&leaderGUID)
	var oldName string
	_ = cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", guid).Scan(&oldName)
	if leaderGUID == guid && guid != 0 {
		var newGUID, newRank int64
		var newName string
		err := cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank, c.name FROM guild_member gm
			JOIN characters c ON c.guid = gm.guid
			WHERE gm.guildid = ? AND gm.guid != ? ORDER BY gm.rank ASC, gm.guid ASC LIMIT 1`, guildID, guid).Scan(&newGUID, &newRank, &newName)
		if err != nil || newGUID == 0 {
			s.disbandGuildByID(ctx, guildID) // Guild.cpp:2296-2300
			return
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE guild SET leaderguid = ? WHERE guildid = ?", newGUID, guildID)
		_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET `rank` = 0 WHERE guid = ? AND guildid = ?", newGUID, guildID)
		if ns := s.server.findSessionByGUID(uint64(newGUID)); ns != nil && ns.player != nil && ns.player.GuildID == guildID {
			ns.player.GuildRank = 0
			ns.sendPlayerUpdate()
		}
		s.broadcastGuildEventTo(guildID, guildEventPayload(guildEventLeaderChanged, 0, oldName, newName))
		s.broadcastGuildEventTo(guildID, guildEventPayload(guildEventLeft, guid, oldName))
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ? AND guildid = ?", guid, guildID)
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil && ts.player.GuildID == guildID {
		ts.player.GuildID = 0
		ts.player.GuildRank = 0
		ts.sendPlayerUpdate()
	}
}

// disbandGuildByID mirrors Guild::Disband (Guild.cpp:1141) for GM use:
// GE_DISBANDED reaches every online member before the rows are deleted, then
// every online member's session guild state is cleared. Shared with the
// CMSG_GUILD_DISBAND handler in guild.go.
func (s *session) disbandGuildByID(ctx context.Context, guildID uint32) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	s.broadcastGuildEventTo(guildID, guildEventPayload(guildEventDisbanded, 0))
	execGuildDisband(ctx, cdb, int64(guildID))
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
			continue
		}
		target.player.GuildID = 0
		target.player.GuildRank = 0
		target.sendPlayerUpdate()
	}
}

// guildSelectionOrSelf mirrors the extractPlayerTarget null-args branch
// (Chat.cpp:657): the selected player, else the handler's own player.
func (s *session) guildSelectionOrSelf() *session {
	if s.selection != 0 && s.server != nil {
		if sel := s.server.playerSessionForGUID(s.selection); sel != nil {
			return sel
		}
	}
	return s
}

// handleGuildCreateCommand mirrors HandleGuildCreateCommand
// (cs_guild.cpp:76-137): `guild create [<player>] "<name>"`.
func (s *session) handleGuildCreateCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	toks := splitQuotedArgs(args)
	if len(toks) == 0 {
		s.sendSysMessage("Syntax: .guild create [<player>] \"<name>\"")
		return
	}
	// *args != '"' -> the first token names the leader; otherwise the
	// target is the selection/self and the first token is the guild name
	// (cs_guild.cpp:87-92).
	targetSess := s.guildSelectionOrSelf()
	rest := toks
	if !strings.HasPrefix(toks[0], "\"") {
		name := normalizePlayerName(unquoteCommandToken(toks[0]))
		targetSess = s.server.findSessionByName(name)
		if targetSess == nil || targetSess.player == nil {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return
		}
		rest = toks[1:]
	}
	if len(rest) == 0 {
		s.sendSysMessage("Syntax: .guild create [<player>] \"<name>\"")
		return
	}
	guildName := unquoteCommandToken(rest[0])
	targetGUID := targetSess.playerGUID

	if targetSess.player.GuildID != 0 {
		s.sendSysMessage("Player is in guild already.") // LANG_PLAYER_IN_GUILD 500
		return
	}
	if guildIDByName(ctx, cdb, guildName) != 0 {
		s.sendSysMessage("There is already another guild with the same name.") // LANG_GUILD_RENAME_ALREADY_EXISTS 96
		return
	}
	// IsReservedName / IsValidCharterName have no Go bridge (documented
	// gap); the empty and >24-char rejects are enforced (Guild.cpp:1224).
	if guildName == "" || len(guildName) > 24 {
		s.sendSysMessage("Incorrect value") // LANG_BAD_VALUE 115
		return
	}

	// Guild::Create (Guild.cpp:1086): fresh id (in-memory NextGuildId has no
	// Go bridge; MAX+1 is the documented stand-in), leader = target,
	// motd "No message set.", emblem fields zeroed, then the five default
	// ranks (Guild.cpp:2428) and the leader at GR_GUILDMASTER.
	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var maxID uint32
	_ = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(guildid), 0) FROM guild").Scan(&maxID)
	guildID := maxID + 1
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO guild (guildid, name, leaderguid, info, motd, createdate, EmblemStyle, EmblemColor, BorderStyle, BorderColor, BackgroundColor, BankMoney)
		VALUES (?, ?, ?, '', 'No message set.', ?, 0, 0, 0, 0, 0, 0)`, guildID, guildName, targetGUID, now); err != nil {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return
	}
	defaultRanks := []struct {
		name   string
		rights uint32
	}{
		{"Guild Master", guildRightAll},                                            // LANG_GUILD_MASTER 811
		{"Officer", guildRightAll},                                                 // LANG_GUILD_OFFICER 812
		{"Veteran", guildRightGChatListen | guildRightGChatSpeak},                 // LANG_GUILD_VETERAN 813
		{"Member", guildRightGChatListen | guildRightGChatSpeak},                   // LANG_GUILD_MEMBER 814
		{"Initiate", guildRightGChatListen | guildRightGChatSpeak},                 // LANG_GUILD_INITIATE 815
	}
	for rid, r := range defaultRanks {
		if _, err := tx.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, ?, ?, ?, 0)", guildID, rid, r.name, r.rights); err != nil {
			s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return
	}
	committed = true
	// Guild::Create -> AddMember(trans, m_leaderGuid, GR_GUILDMASTER)
	// (Guild.cpp:1131).
	if !s.guildAddMember(ctx, cdb, guildID, targetGUID, 0, targetSess.player.Name) {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return
	}
}

// handleGuildDeleteCommand mirrors HandleGuildDeleteCommand
// (cs_guild.cpp:139-156): `guild delete "<name>"`.
func (s *session) handleGuildDeleteCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	toks := splitQuotedArgs(args)
	if len(toks) == 0 {
		s.sendSysMessage("Syntax: .guild delete \"<name>\"")
		return
	}
	guildID := guildIDByName(ctx, cdb, unquoteCommandToken(toks[0]))
	if guildID == 0 {
		return // silent, like the C++ (cs_guild.cpp:151)
	}
	s.disbandGuildByID(ctx, guildID)
}

// handleGuildInviteCommand mirrors HandleGuildInviteCommand
// (cs_guild.cpp:158-182): `guild invite [<player>] "<name>"`.
func (s *session) handleGuildInviteCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	toks := splitQuotedArgs(args)
	if len(toks) == 0 {
		s.sendSysMessage("Syntax: .guild invite [<player>] \"<name>\"")
		return
	}
	var targetGUID uint64
	var targetName string
	rest := toks
	if !strings.HasPrefix(toks[0], "\"") {
		name := normalizePlayerName(unquoteCommandToken(toks[0]))
		if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
			targetGUID, targetName = ts.playerGUID, ts.player.Name
		} else {
			// extractPlayerTarget with a guid out-param falls back to the
			// character cache (Chat.cpp:676).
			_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&targetGUID)
			targetName = name
		}
		if targetGUID == 0 {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return
		}
		rest = toks[1:]
	} else {
		targetSess := s.guildSelectionOrSelf()
		if targetSess.player == nil {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return
		}
		targetGUID, targetName = targetSess.playerGUID, targetSess.player.Name
	}
	if len(rest) == 0 {
		s.sendSysMessage("Syntax: .guild invite [<player>] \"<name>\"")
		return
	}
	guildID := guildIDByName(ctx, cdb, unquoteCommandToken(rest[0]))
	if guildID == 0 {
		return // silent, like the C++ (cs_guild.cpp:176)
	}
	// The C++ notes "player's guild membership checked in AddMember before
	// add" (cs_guild.cpp:178).
	s.guildAddMember(ctx, cdb, guildID, targetGUID, guildLowestRankID(ctx, cdb, guildID), targetName)
}

// handleGuildUninviteCommand mirrors HandleGuildUninviteCommand
// (cs_guild.cpp:184-202): `guild uninvite <player>`.
func (s *session) handleGuildUninviteCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	// extractPlayerTarget(args, &target, &targetGuid): name, selection, then
	// self (Chat.cpp:657).
	target := s.guildSelectionOrSelf()
	var guid uint64
	if len(args) > 0 {
		name := normalizePlayerName(unquoteCommandToken(splitQuotedArgs(args)[0]))
		if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
			target, guid = ts, ts.playerGUID
		} else {
			_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid)
			target = nil
			if guid == 0 {
				s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
				return
			}
		}
	} else if target.player != nil {
		guid = target.playerGUID
	}
	var guildID uint32
	if target != nil && target.player != nil {
		guildID = target.player.GuildID
	} else {
		_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", guid).Scan(&guildID)
	}
	if guildID == 0 {
		return // silent, like the C++ (cs_guild.cpp:193)
	}
	var exists uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&exists); err != nil {
		return
	}
	// DeleteMember(trans, targetGuid, false, true, true) (cs_guild.cpp:200).
	s.guildDeleteMember(ctx, cdb, guildID, guid)
}

// handleGuildRankCommand mirrors HandleGuildRankCommand (cs_guild.cpp:204-221):
// `guild rank [<player>] <rank>`.
func (s *session) handleGuildRankCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	toks := splitQuotedArgs(args)
	if len(toks) == 0 {
		s.sendSysMessage("Syntax: .guild rank [<player>] <rank>")
		return
	}
	var guid uint64
	var rankTok string
	if len(toks) == 1 {
		// Optional<PlayerIdentifier> absent -> FromTargetOrSelf
		// (cs_guild.cpp:207). A non-numeric single token is a player name
		// with the rank missing; the C++ parser rejects it with syntax.
		if _, err := strconv.ParseUint(toks[0], 10, 8); err != nil {
			s.sendSysMessage("Syntax: .guild rank [<player>] <rank>")
			return
		}
		if target := s.guildSelectionOrSelf(); target.player != nil {
			guid = target.playerGUID
		}
		rankTok = toks[0]
	} else {
		name := normalizePlayerName(unquoteCommandToken(toks[0]))
		if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
			guid = ts.playerGUID
		} else {
			_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid)
		}
		if guid == 0 {
			return // unknown name: identifier has no guid, guild lookup misses
		}
		rankTok = toks[1]
	}
	newRank64, err := strconv.ParseUint(rankTok, 10, 8)
	if err != nil {
		return
	}
	newRank := uint8(newRank64)
	var guildID uint32
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		guildID = ts.player.GuildID
	} else {
		_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", guid).Scan(&guildID)
	}
	if guildID == 0 {
		return
	}
	var exists uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&exists); err != nil {
		return
	}
	// Guild::ChangeMemberRank (Guild.cpp:2336): rank must be an existing
	// rank id and the target must be a member.
	if newRank > guildLowestRankID(ctx, cdb, guildID) {
		return
	}
	var member uint64
	if err := cdb.QueryRowContext(ctx, "SELECT guid FROM guild_member WHERE guildid = ? AND guid = ? LIMIT 1", guildID, guid).Scan(&member); err != nil {
		return
	}
	// Member::ChangeRank (Guild.cpp:577): DB row plus the online player's
	// rank field.
	if _, err := cdb.ExecContext(ctx, "UPDATE guild_member SET `rank` = ? WHERE guid = ? AND guildid = ?", newRank, guid, guildID); err != nil {
		return
	}
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil && ts.player.GuildID == guildID {
		ts.player.GuildRank = newRank
		ts.sendPlayerUpdate()
	}
}

// handleGuildRenameCommand mirrors HandleGuildRenameCommand
// (cs_guild.cpp:223-265): `guild rename "<old>" "<new>"`.
func (s *session) handleGuildRenameCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	toks := splitQuotedArgs(args)
	if len(toks) < 1 {
		s.sendSysMessage("Incorrect value") // LANG_BAD_VALUE 115
		return
	}
	if len(toks) < 2 {
		s.sendSysMessage("Insert guild name.") // LANG_INSERT_GUILD_NAME 498
		return
	}
	oldName := unquoteCommandToken(toks[0])
	newName := unquoteCommandToken(toks[1])
	guildID := guildIDByName(ctx, cdb, oldName)
	if guildID == 0 {
		s.sendSysMessage(fmt.Sprintf("The command couldn't find '%s'", oldName)) // LANG_COMMAND_COULDNOTFIND 434
		return
	}
	if guildIDByName(ctx, cdb, newName) != 0 {
		s.sendSysMessage(fmt.Sprintf("There is already another guild with the same name.")) // LANG_GUILD_RENAME_ALREADY_EXISTS 96
		return
	}
	// Guild::SetName (Guild.cpp:1221): same name, empty, >24 chars,
	// reserved or invalid charter name fail. The reserved/charter checks
	// have no Go bridge (documented gap).
	if newName == "" || len(newName) > 24 {
		s.sendSysMessage("Incorrect value") // LANG_BAD_VALUE 115
		return
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE guild SET name = ? WHERE guildid = ?", newName, guildID); err != nil {
		s.sendSysMessage("Incorrect value") // LANG_BAD_VALUE 115
		return
	}
	s.sendSysMessage(fmt.Sprintf("'%s' Guild name changed to '%s'", oldName, newName)) // LANG_GUILD_RENAME_DONE 97
}

// handleGuildInfoCommand mirrors HandleGuildInfoCommand (cs_guild.cpp:267-309):
// `guild info [id|"<name>"]`, defaulting to the selection's/self's guild.
func (s *session) handleGuildInfoCommand(ctx context.Context, args []string) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	var guildID uint32
	toks := splitQuotedArgs(args)
	if len(toks) > 0 {
		tok := unquoteCommandToken(toks[0])
		if isAllDigits(tok) {
			if id, err := strconv.ParseUint(tok, 10, 32); err == nil {
				guildID = uint32(id)
			}
		} else {
			guildID = guildIDByName(ctx, cdb, tok)
		}
	} else if target := s.guildSelectionOrSelf(); target.player != nil {
		guildID = target.player.GuildID
	}
	if guildID == 0 {
		return
	}
	var name, motd, info string
	var leaderGUID uint64
	var createdDate int64
	var bankMoney uint64
	err := cdb.QueryRowContext(ctx, "SELECT name, leaderguid, motd, info, createdate, BankMoney FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name, &leaderGUID, &motd, &info, &createdDate, &bankMoney)
	if err != nil {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Guild %s Id %d", name, guildID)) // LANG_GUILD_INFO_NAME 1177
	var leaderName string
	if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", leaderGUID).Scan(&leaderName); err == nil {
		s.sendSysMessage(fmt.Sprintf("Guild Master %s Guid %d", leaderName, uint32(leaderGUID))) // LANG_GUILD_INFO_GUILD_MASTER 1178
	}
	created := time.Unix(createdDate, 0).Local().Format("2006-01-02 15:04:05")
	s.sendSysMessage(fmt.Sprintf("Creation Date %s", created)) // LANG_GUILD_INFO_CREATION_DATE 1179
	var memberCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_member WHERE guildid = ?", guildID).Scan(&memberCount)
	s.sendSysMessage(fmt.Sprintf("Members %d", memberCount))       // LANG_GUILD_INFO_MEMBER_COUNT 1180
	s.sendSysMessage(fmt.Sprintf("Bank Gold %d", bankMoney/100/100)) // LANG_GUILD_INFO_BANK_GOLD 1181
	s.sendSysMessage(fmt.Sprintf("MOTD %s", motd))                  // LANG_GUILD_INFO_MOTD 1182
	s.sendSysMessage(fmt.Sprintf("Extra Info %s", info))            // LANG_GUILD_INFO_EXTRA_INFO 1183
}

// isAllDigits mirrors the isNumeric(args) gate in HandleGuildInfoCommand
// (cs_guild.cpp:275).
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
