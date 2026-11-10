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
// rest of the tree). Unterminated quotes are accepted like the C++
// strtok-based extractQuotedArg: the arms re-split with
// splitQuotedArgsLenient, so `"My Guild` yields the name `My Guild`.
package world

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleCmdGuild dispatches the guild sub-commands, mirroring
// guild_commandscript::GetCommands (cs_guild.cpp:46-69).
func (s *session) handleCmdGuild(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	deny := func(perm uint32) bool {
		return !s.commandAllowed(ctx, perm)
	}
	// Subcommand names prefix-match over the invoker-visible arms only
	// (ChatCommand.cpp:261-288 + FilteredCommandListIterator :207-237),
	// like group: ".guild c" reaches create, ".guild i" is ambiguous
	// (invite/info) only when both are visible.
	visible := func(perm uint32) bool { return !deny(perm) }
	sub, cands, ok := groupMatchNameVisible(args[0], []groupSubcommand{
		{"create", []uint32{permissionCommandGuildCreate}},
		{"delete", []uint32{permissionCommandGuildDelete}},
		{"invite", []uint32{permissionCommandGuildInvite}},
		{"uninvite", []uint32{permissionCommandGuildUninvite}},
		{"rank", []uint32{permissionCommandGuildRank}},
		{"rename", []uint32{permissionCommandGuildRename}},
		{"info", []uint32{permissionCommandGuildInfo}},
	}, visible)
	if !ok {
		if len(cands) > 1 {
			s.groupAmbiguousMessage(args[0], cands)
			return true
		}
		return false
	}
	switch sub {
	case "create":
		if deny(permissionCommandGuildCreate) {
			return false
		}
		return s.handleGuildCreateCommand(ctx, args[1:])
	case "delete":
		if deny(permissionCommandGuildDelete) {
			return false
		}
		return s.handleGuildDeleteCommand(ctx, args[1:])
	case "invite":
		if deny(permissionCommandGuildInvite) {
			return false
		}
		return s.handleGuildInviteCommand(ctx, args[1:])
	case "uninvite":
		if deny(permissionCommandGuildUninvite) {
			return false
		}
		return s.handleGuildUninviteCommand(ctx, args[1:])
	case "rank":
		if deny(permissionCommandGuildRank) {
			return false
		}
		return s.handleGuildRankCommand(ctx, args[1:])
	case "rename":
		if deny(permissionCommandGuildRename) {
			return false
		}
		return s.handleGuildRenameCommand(ctx, args[1:])
	case "info":
		if deny(permissionCommandGuildInfo) {
			return false
		}
		return s.handleGuildInfoCommand(ctx, args[1:])
	}
	return true
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
// GuildMgr::GetGuildByName (GuildMgr.cpp:65-72) compares with StringEqualI
// (case-insensitive); the query is case-insensitive too.
func guildIDByName(ctx context.Context, cdb *sql.DB, name string) uint32 {
	var id uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE UPPER(name) = UPPER(?) LIMIT 1", name).Scan(&id); err != nil {
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
// C++ Guild::BroadcastPacket (Guild.cpp:2155) reaches every connected roster
// member via FindConnectedPlayer, which carries no IsInWorld gate, so no
// worldReady gate here either.
func (s *session) broadcastGuildEventTo(guildID uint32, payload []byte) {
	if s.server == nil {
		return
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if target.player == nil || target.player.GuildID != guildID {
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
	removeGuildCharterPetitions(ctx, cdb, guid)
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

// guildDeleteMember mirrors Guild::DeleteMember(trans, guid, isDisbanding,
// false, true) (Guild.cpp:2277) for the GM uninvite arm and the Eluna
// DeleteMember path. A removed leader promotes the lowest-rank remaining
// member (rank 0), broadcasting GE_LEADER_CHANGED then GE_LEFT while the old
// leader is still a member; with no successor the guild disbands. The
// leader-replacement arm is skipped when isDisbanding is set (the C++
// !isDisbanding gate). Non-leader removal is silent, exactly as in C++ —
// DeleteMember itself logs and broadcasts nothing.
func (s *session) guildDeleteMember(ctx context.Context, cdb *sql.DB, guildID uint32, guid uint64, isDisbanding bool) {
	var leaderGUID uint64
	_ = cdb.QueryRowContext(ctx, "SELECT leaderguid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&leaderGUID)
	var oldName string
	_ = cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", guid).Scan(&oldName)
	if leaderGUID == guid && guid != 0 && !isDisbanding {
		var newGUID, newRank int64
		var newName string
		err := cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank, c.name FROM guild_member gm
			JOIN characters c ON c.guid = gm.guid
			WHERE gm.guildid = ? AND gm.guid != ? ORDER BY gm.rank ASC, gm.guid ASC LIMIT 1`, guildID, guid).Scan(&newGUID, &newRank, &newName)
		if err != nil || newGUID == 0 {
			// Guild::DeleteMember (Guild.cpp:2296-2300): no successor ->
			// Disband(), which fires OnGuildDisband + GE_DISBANDED + the
			// per-member OnGuildRemoveMember(true) sweep; the leader gets
			// no separate OnGuildRemoveMember(false) (Disband returns
			// before that line).
			s.disbandGuildWithLuaEvents(ctx, guildID)
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
	// Guild::DeleteMember (Guild.cpp:2313): the script hook fires before
	// the member is erased from the guild and the database. The leader
	// disband path above returns before this line, so a disbanded leader
	// gets only the disband sweep's OnGuildRemoveMember(true).
	var playerArg *scripting.Object
	if ts := s.server.findSessionByGUID(guid); ts != nil {
		playerArg = ts.luaPlayer()
	}
	s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, s.luaGuildObject(ctx, guildID), playerArg, isDisbanding)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ? AND guildid = ?", guid, guildID)
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil && ts.player.GuildID == guildID {
		ts.player.GuildID = 0
		ts.player.GuildRank = 0
		ts.sendPlayerUpdate()
	}
}

// disbandGuildRows deletes the guild rows and clears every online member's
// session guild state. Split from disbandGuildByID so the Lua disband path
// can broadcast GE_DISBANDED before the per-member sweep (Guild.cpp:1144).
func (s *session) disbandGuildRows(ctx context.Context, guildID uint32) {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return
	}
	execGuildDisband(ctx, cdb, int64(guildID))
	// Snapshot the members under RLock first: sendPlayerUpdate fans out
	// through broadcastPlayerValuesUpdateFromSession, which takes
	// sessionsMu.RLock, so updating under this lock nests the read lock.
	s.server.sessionsMu.RLock()
	var members []*session
	for target := range s.server.sessions {
		if target.player == nil || target.player.GuildID != guildID {
			continue
		}
		members = append(members, target)
	}
	s.server.sessionsMu.RUnlock()
	for _, target := range members {
		if target.player == nil {
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

// guildQuotedArg mirrors ChatHandler::extractQuotedArg (Chat.cpp:714-744)
// for the guild-name argument: the name must be a double-quoted string —
// an unquoted token makes extractQuotedArg return null, so the handler
// fails. Single quotes do not count: the C++ quoter only recognizes '"',
// and "" yields an empty name, like the C++ "" special case.
// extractQuotedArg is strtok-based: it stops at the closing quote and
// ignores trailing junk (`"Name"x` yields `Name`), and an unterminated
// quote runs to end-of-string (D34).
func guildQuotedArg(tok string) (string, bool) {
	if len(tok) == 0 || tok[0] != '"' {
		return "", false
	}
	rest := tok[1:]
	if i := strings.IndexByte(rest, '"'); i >= 0 {
		return rest[:i], true
	}
	return rest, true
}

// handleGuildCreateCommand mirrors HandleGuildCreateCommand
// (cs_guild.cpp:73-127): `guild create [<player>] "<name>"`.
func (s *session) handleGuildCreateCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	toks := splitQuotedArgsLenient(args)
	if len(toks) == 0 {
		return false
	}
	// *args != '"' -> the first token names the leader; otherwise the
	// target is the selection/self and the first token is the guild name
	// (cs_guild.cpp:87-92).
	targetSess := s.guildSelectionOrSelf()
	rest := toks
	if !strings.HasPrefix(toks[0], "\"") {
		name := normalizePlayerName(extractPlayerNameFromLink(unquoteCommandToken(toks[0])))
		targetSess = s.server.findSessionByName(name)
		if targetSess == nil || targetSess.player == nil {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return true
		}
		rest = toks[1:]
	}
	if len(rest) == 0 {
		return false
	}
	// extractQuotedArg (Chat.cpp:714) requires the guild name to be
	// double-quoted; an unquoted name fails the handler outright.
	guildName, ok := guildQuotedArg(rest[0])
	if !ok {
		return false
	}
	targetGUID := targetSess.playerGUID

	if targetSess.player.GuildID != 0 {
		s.sendSysMessage("Player is in guild already.") // LANG_PLAYER_IN_GUILD 500
		return true
	}
	if guildIDByName(ctx, cdb, guildName) != 0 {
		s.sendSysMessage("There is already another guild with the same name.") // LANG_GUILD_RENAME_ALREADY_EXISTS 96
		return true
	}
	// IsReservedName / IsValidCharterName have no Go bridge (documented
	// gap); the empty and >24-char rejects are enforced (Guild.cpp:1224).
	if guildName == "" || len(guildName) > 24 {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}

	// Guild::Create (Guild.cpp:1086): fresh id (in-memory NextGuildId has no
	// Go bridge; MAX+1 is the documented stand-in), leader = target,
	// motd "No message set.", emblem fields zeroed, then the five default
	// ranks (Guild.cpp:2428) and the leader at GR_GUILDMASTER.
	now := time.Now().Unix()
	var guildID uint32
	if err := s.server.CharactersStore.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		var maxID uint32
		_ = tx.QueryRowContext(txCtx, "SELECT COALESCE(MAX(guildid), 0) FROM guild").Scan(&maxID)
		guildID = maxID + 1
		if _, err := tx.ExecContext(txCtx, `INSERT INTO guild (guildid, name, leaderguid, info, motd, createdate, EmblemStyle, EmblemColor, BorderStyle, BorderColor, BackgroundColor, BankMoney)
		VALUES (?, ?, ?, '', 'No message set.', ?, 0, 0, 0, 0, 0, 0)`, guildID, guildName, targetGUID, now); err != nil {
			return err
		}
		defaultRanks := []struct {
			name   string
			rights uint32
		}{
			{"Guild Master", guildRightAll},                            // LANG_GUILD_MASTER 811
			{"Officer", guildRightAll},                                 // LANG_GUILD_OFFICER 812
			{"Veteran", guildRightGChatListen | guildRightGChatSpeak},  // LANG_GUILD_VETERAN 813
			{"Member", guildRightGChatListen | guildRightGChatSpeak},   // LANG_GUILD_MEMBER 814
			{"Initiate", guildRightGChatListen | guildRightGChatSpeak}, // LANG_GUILD_INITIATE 815
		}
		for rid, r := range defaultRanks {
			if _, err := tx.ExecContext(txCtx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, ?, ?, ?, 0)", guildID, rid, r.name, r.rights); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return true
	}
	// Guild::Create -> AddMember(trans, m_leaderGuid, GR_GUILDMASTER)
	// (Guild.cpp:1131).
	if !s.guildAddMember(ctx, cdb, guildID, targetGUID, 0, targetSess.player.Name) {
		s.sendSysMessage("Guild is not created.") // LANG_GUILD_NOT_CREATED 501
		return true
	}
	return true
}

// handleGuildDeleteCommand mirrors HandleGuildDeleteCommand
// (cs_guild.cpp:128-148): `guild delete "<name>"`.
func (s *session) handleGuildDeleteCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	toks := splitQuotedArgsLenient(args)
	if len(toks) == 0 {
		return false
	}
	// extractQuotedArg (Chat.cpp:714) requires a double-quoted name.
	guildName, ok := guildQuotedArg(toks[0])
	if !ok {
		return false
	}
	guildID := guildIDByName(ctx, cdb, guildName)
	if guildID == 0 {
		// C++ returns false with no message; the framework shows the
		// command help (ChatCommand.cpp:302-309).
		return false
	}
	// cs_guild.cpp:143 calls Guild::Disband(), which fires the script hooks.
	s.disbandGuildWithLuaEvents(ctx, guildID)
	return true
}

// handleGuildInviteCommand mirrors HandleGuildInviteCommand
// (cs_guild.cpp:149-176): `guild invite [<player>] "<name>"`.
func (s *session) handleGuildInviteCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	toks := splitQuotedArgsLenient(args)
	if len(toks) == 0 {
		return false
	}
	var targetGUID uint64
	var targetName string
	rest := toks
	if !strings.HasPrefix(toks[0], "\"") {
		name := normalizePlayerName(extractPlayerNameFromLink(unquoteCommandToken(toks[0])))
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
			return true
		}
		rest = toks[1:]
	} else {
		targetSess := s.guildSelectionOrSelf()
		if targetSess.player == nil {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return true
		}
		targetGUID, targetName = targetSess.playerGUID, targetSess.player.Name
	}
	if len(rest) == 0 {
		return false
	}
	// extractQuotedArg (Chat.cpp:714) requires a double-quoted guild name.
	guildName, ok := guildQuotedArg(rest[0])
	if !ok {
		return false
	}
	guildID := guildIDByName(ctx, cdb, guildName)
	if guildID == 0 {
		// C++ returns false with no message; the framework shows the
		// command help (ChatCommand.cpp:302-309).
		return false
	}
	// The C++ notes "player's guild membership checked in AddMember before
	// add" (cs_guild.cpp:178); a failed AddMember returns false from the
	// handler, which the framework shows as help.
	if !s.guildAddMember(ctx, cdb, guildID, targetGUID, guildLowestRankID(ctx, cdb, guildID), targetName) {
		return false
	}
	return true
}

// handleGuildUninviteCommand mirrors HandleGuildUninviteCommand
// (cs_guild.cpp:177-196): `guild uninvite <player>`.
func (s *session) handleGuildUninviteCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	// extractPlayerTarget(args, &target, &targetGuid): name, selection, then
	// self (Chat.cpp:657).
	target := s.guildSelectionOrSelf()
	var guid uint64
	toks := splitQuotedArgsLenient(args)
	if len(toks) > 0 {
		name := normalizePlayerName(extractPlayerNameFromLink(unquoteCommandToken(toks[0])))
		if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
			target, guid = ts, ts.playerGUID
		} else {
			_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid)
			target = nil
			if guid == 0 {
				s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
				return true
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
		// C++ returns false with no message; the framework shows the
		// command help (ChatCommand.cpp:302-309).
		return false
	}
	var exists uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&exists); err != nil {
		return false
	}
	// DeleteMember(trans, targetGuid, false, true, true) (cs_guild.cpp:200).
	s.guildDeleteMember(ctx, cdb, guildID, guid, false)
	return true
}

// handleGuildRankCommand mirrors HandleGuildRankCommand (cs_guild.cpp:197-214):
// `guild rank [<player>] <rank>`.
func (s *session) handleGuildRankCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	const syntax = "Syntax: .guild rank [<player>] <rank>"
	toks := splitQuotedArgsLenient(args)
	// The C++ handler is framework-parsed (Optional<PlayerIdentifier>,
	// uint8), so the whole string must be consumed (ChatCommand.h:112):
	// trailing tokens fail the parse.
	if len(toks) == 0 {
		return false
	}
	if len(toks) > 2 {
		// Trailing tokens: the Optional backtrack re-parses the player
		// token as the uint8 and errors (ChatCommand.h:77-98), so the
		// parser message survives -> message + true.
		s.sendSysMessage(syntax)
		return true
	}
	var guid uint64
	var newRank uint8
	if len(toks) == 1 {
		// Optional<PlayerIdentifier> absent -> FromTargetOrSelf
		// (cs_guild.cpp:201-202). The C++ Optional backtracks
		// (ChatCommand.h:77-98), so even a numeric single token is the
		// rank here, never a player GUID. A non-numeric single token is
		// a player name with the rank missing; the parser rejects it.
		// The rank is StringTo<uint8> base 0 (StringConvert.h:43-69):
		// 0x/0b accepted, never octal, must fit in 8 bits.
		rank, ok := parseCommandUint8(toks[0])
		if !ok {
			s.sendSysMessage(syntax)
			return true
		}
		newRank = rank
		if target := s.guildSelectionOrSelf(); target.player != nil {
			guid = target.playerGUID
		}
	} else {
		// PlayerIdentifier::TryConsume (ChatCommandTags.cpp:88-121): a
		// |Hplayer link, a numeric low GUID, or a plain name.
		if strings.HasPrefix(toks[0], "|") {
			name := normalizePlayerName(extractPlayerNameFromLink(toks[0]))
			if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
				guid = ts.playerGUID
			} else {
				_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid)
			}
			if guid == 0 {
				s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
				return true
			}
		} else if low, ok := parseCommandUint32(toks[0]); ok {
			if ts := s.server.findSessionByGUID(uint64(low)); ts != nil && ts.player != nil {
				guid = ts.playerGUID
			} else {
				var dbName string
				if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", low).Scan(&dbName); err != nil || dbName == "" {
					s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
					return true
				}
				guid = uint64(low)
			}
		} else {
			name := normalizePlayerName(extractPlayerNameFromLink(unquoteCommandToken(toks[0])))
			if ts := s.server.findSessionByName(name); ts != nil && ts.player != nil {
				guid = ts.playerGUID
			} else {
				_ = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid)
			}
			if guid == 0 {
				s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
				return true
			}
		}
		rank, ok := parseCommandUint8(toks[1])
		if !ok {
			s.sendSysMessage(syntax)
			return true
		}
		newRank = rank
	}
	var guildID uint32
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		guildID = ts.player.GuildID
	} else {
		_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", guid).Scan(&guildID)
	}
	if guildID == 0 {
		// C++ returns false with no message; the framework shows the
		// command help (ChatCommand.cpp:302-309).
		return false
	}
	var exists uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&exists); err != nil {
		return false
	}
	// Guild::ChangeMemberRank (Guild.cpp:2336): rank must be an existing
	// rank id and the target must be a member.
	if newRank > guildLowestRankID(ctx, cdb, guildID) {
		return false
	}
	var member uint64
	if err := cdb.QueryRowContext(ctx, "SELECT guid FROM guild_member WHERE guildid = ? AND guid = ? LIMIT 1", guildID, guid).Scan(&member); err != nil {
		return false
	}
	// Member::ChangeRank (Guild.cpp:577): DB row plus the online player's
	// rank field.
	if _, err := cdb.ExecContext(ctx, "UPDATE guild_member SET `rank` = ? WHERE guid = ? AND guildid = ?", newRank, guid, guildID); err != nil {
		return true
	}
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil && ts.player.GuildID == guildID {
		ts.player.GuildRank = newRank
		ts.sendPlayerUpdate()
	}
	return true
}

// handleGuildRenameCommand mirrors HandleGuildRenameCommand
// (cs_guild.cpp:215-263): `guild rename "<old>" "<new>"`.
func (s *session) handleGuildRenameCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	toks := splitQuotedArgsLenient(args)
	if len(toks) < 1 {
		// No args at all: the C++ handler returns false with no message
		// and the framework shows the command help.
		return false
	}
	// Both names come from extractQuotedArg (Chat.cpp:714): an unquoted
	// old name fails with LANG_BAD_VALUE, an unquoted/missing new name
	// with LANG_INSERT_GUILD_NAME.
	oldName, ok := guildQuotedArg(toks[0])
	if !ok {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	if len(toks) < 2 {
		s.sendSysMessage("Insert guild name.") // LANG_INSERT_GUILD_NAME 498
		return true
	}
	newName, ok := guildQuotedArg(toks[1])
	if !ok {
		s.sendSysMessage("Insert guild name.") // LANG_INSERT_GUILD_NAME 498
		return true
	}
	guildID := guildIDByName(ctx, cdb, oldName)
	if guildID == 0 {
		s.sendSysMessage(fmt.Sprintf("The command couldn't find '%s'", oldName)) // LANG_COMMAND_COULDNOTFIND 434
		return true
	}
	if guildIDByName(ctx, cdb, newName) != 0 {
		s.sendSysMessage(fmt.Sprintf("There is already another guild with the same name.")) // LANG_GUILD_RENAME_ALREADY_EXISTS 96
		return true
	}
	// Guild::SetName (Guild.cpp:1221): same name, empty, >24 chars,
	// reserved or invalid charter name fail. The reserved/charter checks
	// have no Go bridge (documented gap).
	if newName == "" || len(newName) > 24 {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE guild SET name = ? WHERE guildid = ?", newName, guildID); err != nil {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE 115
		return true
	}
	s.sendSysMessage(fmt.Sprintf("'%s' Guild name changed to '%s'", oldName, newName)) // LANG_GUILD_RENAME_DONE 97
	return true
}

// handleGuildInfoCommand mirrors HandleGuildInfoCommand (cs_guild.cpp:264-306):
// `guild info [id|"<name>"]`, defaulting to the selection's/self's guild.
func (s *session) handleGuildInfoCommand(ctx context.Context, args []string) bool {
	cdb := s.guildCharactersDB()
	if cdb == nil {
		return true
	}
	var guildID uint32
	// The C++ arm reads the whole remainder verbatim as the name
	// (cs_guild.cpp:273-284): quotes are NOT stripped, so a quoted name
	// looks up the literal quoted string and misses.
	if len(args) > 0 {
		name := strings.Join(args, " ")
		if isAllDigits(name) {
			if id, err := strconv.ParseUint(name, 10, 32); err == nil {
				guildID = uint32(id)
			}
		} else {
			guildID = guildIDByName(ctx, cdb, name)
		}
	} else if target := s.guildSelectionOrSelf(); target.player != nil {
		guildID = target.player.GuildID
	}
	if guildID == 0 {
		// C++ returns false with no message; the framework shows the
		// command help (ChatCommand.cpp:302-309).
		return false
	}
	var name, motd, info string
	var leaderGUID uint64
	var createdDate int64
	var bankMoney uint64
	err := cdb.QueryRowContext(ctx, "SELECT name, leaderguid, motd, info, createdate, BankMoney FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name, &leaderGUID, &motd, &info, &createdDate, &bankMoney)
	if err != nil {
		// C++ GetGuildById misses -> return false -> framework help.
		return false
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
	return true
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
