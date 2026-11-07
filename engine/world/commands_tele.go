package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// tele command port: tele_commandscript (cs_tele.cpp), the "tele" root with
// 5 arms (add, del, name, group, ""). THIRTY-SEVENTH of 40 Commands groups
// (cs_script_loader.cpp decl 55 / call 100; call order server(99) ->
// tele(100)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:605-609); the root permission 737 covers the bare ".tele <name>".
//
// All five arms are native on the world `game_tele` table (the Go equivalent
// of ObjectMgr::_gameTeleStore, which C++ loads from the same table):
//   - `add <name>` inserts the handler's current position after an exact
//     case-insensitive name check (ObjectMgr::GetGameTeleExactName).
//   - `del <name|id|link>` deletes the row.
//   - `name [player] <tele|$home>` teleports a player: online targets ride
//     teleportTo (an active flight is finished first); offline targets get
//     their saved characters position rewritten (Player::SavePositionInDB);
//     `$home` teleports to the target's homebind (character_homebind for
//     offline targets).
//   - `group <tele>` teleports every online member of the selected player's
//     group, gated on the battleground/arena map check.
//   - `.tele <name>` teleports the handler, gated on combat (requires the
//     name permission 740 while in combat) and the battleground/arena map
//     check (allowed when already on that map or in GM mode).
//
// Tele location resolution mirrors ArgInfo<GameTele const*>::TryConsume
// (ChatCommandArgs.cpp:51): a |Htele:id| link resolves by id, otherwise the
// token matches a name case-insensitively exact first, then the first
// case-insensitive substring match (ObjectMgr::GetGameTele,
// ObjectMgr.cpp:9020).
//
// Documented fidelity gaps (not stubs): a game_tele literally named
// "$home" wins over the homebind leg in C++ (the Variant tries GameTele
// first, ChatCommandArgs.h:261-285); Go checks $home first, so that
// deliberately-created row is unreachable by name.
// A name miss in
// teleResolveTele reports the LANG_COMMAND_TELE_NOTFOUND (164) stand-in —
// C++ reports LANG_CMDPARSER_GAME_TELE_NO_EXIST (1512) whose enUS text is
// not in the tree, and the handler's own null checks are dead arms (the
// non-Optional GameTele parse fails before the handler runs).
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).
//
// R13: the root and all 5 arms return bool — bare/unknown subcommands
// return false (the framework shows the help listing), denial is a silent
// false (the C++ framework denies before invoking the handler), framework
// parse failures (empty/missing/trailing args) are silent false, message
// legs return true (SetSentErrorMessage in C++), and the lower-security
// legs message + true (HasLowerSecurity prints, Chat.cpp:96-99).

// teleLocation is one game_tele row.
type teleLocation struct {
	id    uint32
	name  string
	mapID uint32
	x     float32
	y     float32
	z     float32
	o     float32
}

func scanTeleRow(row *sql.Row) (teleLocation, error) {
	var t teleLocation
	err := row.Scan(&t.id, &t.name, &t.mapID, &t.x, &t.y, &t.z, &t.o)
	return t, err
}

// teleResolveTele mirrors the GameTele command-arg parse: |Htele:id| link
// resolves by id (LANG_CMDPARSER_GAME_TELE_ID_NO_EXIST on miss); otherwise
// the name matches case-insensitively exact first, then substring
// (ObjectMgr::GetGameTele). A miss reports LANG_COMMAND_TELE_NOTFOUND.
func (s *session) teleResolveTele(ctx context.Context, token string) (teleLocation, bool) {
	var t teleLocation
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return t, false
	}
	db := s.server.WorldStore.DB
	if id := miscExtractLinkID(token, "Htele"); id != 0 {
		t, err := scanTeleRow(db.QueryRowContext(ctx,
			"SELECT id, name, map, position_x, position_y, position_z, orientation FROM game_tele WHERE id = ? LIMIT 1", id))
		if errors.Is(err, sql.ErrNoRows) {
			s.sendSysMessage(fmt.Sprintf("Tele location with id %d doesn't exist.", id)) // LANG_CMDPARSER_GAME_TELE_ID_NO_EXIST
			return t, false
		}
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
			return t, false
		}
		return t, true
	}
	t, err := scanTeleRow(db.QueryRowContext(ctx,
		"SELECT id, name, map, position_x, position_y, position_z, orientation FROM game_tele WHERE LOWER(name) = LOWER(?) LIMIT 1", token))
	if err == nil {
		return t, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
		return t, false
	}
	// C++ iterates the id-ordered store with a literal substring find:
	// LIKE wildcards in the token must not act as wildcards, and the
	// first match must be the lowest-id row, so escape and ORDER BY id.
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(token))
	t, err = scanTeleRow(db.QueryRowContext(ctx,
		"SELECT id, name, map, position_x, position_y, position_z, orientation FROM game_tele WHERE LOWER(name) LIKE ? ESCAPE '!' ORDER BY id ASC LIMIT 1",
		"%"+escaped+"%"))
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage("Teleport location not found.") // LANG_COMMAND_TELE_NOTFOUND (164)
		return t, false
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
		return t, false
	}
	return t, true
}

// handleCmdTele dispatches the ".tele" arms (cs_tele.cpp:46-54) with Trinity
// per-level prefix matching; each arm gates its own leaf permission.
// Args arrive as the dispatcher's whitespace split (executeCommand), which
// is the C++ tokenize rule (ChatCommandHelpers.h:55-72: split on space, no
// quote handling) — quotes stay literal characters in the token, so no
// quote-aware re-splitting happens here: no arm takes a QuotedString, and
// every handler arg (std::string, GameTele, PlayerIdentifier, $home)
// consumes exactly one token, with trailing text failing the parse
// ("the entire string must be consumed", ChatCommand.h:107-109).
func (s *session) handleCmdTele(ctx context.Context, args []string) bool {
	toks := args
	if len(toks) == 0 {
		return false
	}
	sub := strings.ToLower(toks[0])
	rest := toks[1:]
	switch {
	case strings.HasPrefix("add", sub):
		if !s.commandAllowed(ctx, permissionCommandTeleAdd) {
			return false
		}
		return s.handleTeleAddCommand(ctx, rest)
	case strings.HasPrefix("del", sub):
		if !s.commandAllowed(ctx, permissionCommandTeleDel) {
			return false
		}
		return s.handleTeleDelCommand(ctx, rest)
	case strings.HasPrefix("name", sub):
		if !s.commandAllowed(ctx, permissionCommandTeleName) {
			return false
		}
		return s.handleTeleNameCommand(ctx, rest)
	case strings.HasPrefix("group", sub):
		if !s.commandAllowed(ctx, permissionCommandTeleGroup) {
			return false
		}
		return s.handleTeleGroupCommand(ctx, rest)
	default:
		if !s.commandAllowed(ctx, permissionCommandTele) {
			return false
		}
		return s.handleTeleBareCommand(ctx, toks)
	}
}

// handleTeleAddCommand mirrors HandleTeleAddCommand (cs_tele.cpp:68-99,
// RBAC 738): in-game only, saves the handler's current position to
// game_tele (ObjectMgr::AddGameTele).
func (s *session) handleTeleAddCommand(ctx context.Context, args []string) bool {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if len(args) == 0 || len(args) > 1 {
		return false
	}
	// std::string consumes one token (ChatCommandArgs.h:93-102); trailing
	// text fails the parse, it is never joined into the name.
	name := args[0]
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	var existing string
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT name FROM game_tele WHERE LOWER(name) = LOWER(?) LIMIT 1", name).Scan(&existing)
	if err == nil {
		s.sendSysMessage("The location already exists.") // LANG_COMMAND_TP_ALREADYEXIST (462)
		return true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
		return true
	}
	p := s.player
	_, err = s.server.WorldStore.DB.ExecContext(ctx,
		"INSERT INTO game_tele (name, map, position_x, position_y, position_z, orientation) VALUES (?, ?, ?, ?, ?, ?)",
		name, p.Map, p.X, p.Y, p.Z, p.Orientation)
	if err != nil {
		s.sendSysMessage("Error adding location.") // LANG_COMMAND_TP_ADDEDERR (464)
		return true
	}
	s.sendSysMessage("Location added.") // LANG_COMMAND_TP_ADDED (463)
	return true
}

// handleTeleDelCommand mirrors HandleTeleDelCommand (cs_tele.cpp:101-114,
// RBAC 739, console allowed): deletes the resolved game_tele row.
func (s *session) handleTeleDelCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// GameTele consumes one token (ChatCommandArgs.cpp:51); trailing text
	// fails the parse, it is never joined into the lookup. The typed
	// parse error wins over trailing text (ChatCommand.h:63-73), so the
	// resolve runs before the length check.
	tele, ok := s.teleResolveTele(ctx, args[0])
	if !ok {
		return true
	}
	if len(args) > 1 {
		return false
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if _, err := s.server.WorldStore.DB.ExecContext(ctx, "DELETE FROM game_tele WHERE id = ?", tele.id); err != nil {
		s.sendSysMessage(fmt.Sprintf("Teleport delete error: %v", err))
		return true
	}
	s.sendSysMessage("Location deleted.") // LANG_COMMAND_TP_DELETED (465)
	return true
}

// handleTeleNameCommand mirrors HandleTeleNameCommand (cs_tele.cpp:117-196,
// RBAC 740, console allowed): teleports [player] (default: target-or-self)
// to <tele> or $home. Online targets ride teleportTo; offline targets get
// their saved characters position rewritten.
func (s *session) handleTeleNameCommand(ctx context.Context, args []string) bool {
	if len(args) > 2 {
		// A parse error on an earlier arg wins over trailing text
		// (ChatCommand.h:77-103): resolve the player and the destination
		// first; either failure already messages, return true. If both
		// resolve, the omit-player path still errors on token 1 as
		// `where`, so the arm messages and returns true. Caveat: when
		// token 1 is both a valid player name and a valid tele name C++
		// is silent-false; this messages instead (one-token corner).
		if _, err := strconv.ParseUint(args[0], 0, 32); err == nil {
			if _, _, _, ok := s.telePlayerByGUID(ctx, args[0]); !ok {
				s.sendSysMessage(miscPlayerNotFound)
				return true
			}
		} else if _, _, _, ok := s.miscResolvePlayerTarget(ctx, args[:1]); !ok {
			return true
		}
		if !strings.EqualFold(args[1], "$home") {
			if _, ok := s.teleResolveTele(ctx, args[1]); !ok {
				return true
			}
		}
		s.sendSysMessage("Syntax: .tele name [player] <location|$home>")
		return true
	}
	if len(args) == 0 {
		return false
	}
	var target *session
	var guid uint64
	var name string
	if len(args) == 2 {
		var ok bool
		if _, err := strconv.ParseUint(args[0], 0, 32); err == nil {
			// Bare numeric token: the ObjectGuid::LowType alternative of
			// PlayerIdentifier::TryConsume (ChatCommandTags.cpp:90-103),
			// base-0 like StringTo<uint32>(token, 0).
			target, guid, name, ok = s.telePlayerByGUID(ctx, args[0])
			if !ok {
				s.sendSysMessage(miscPlayerNotFound)
				return true
			}
		} else {
			target, guid, name, ok = s.miscResolvePlayerTarget(ctx, args[:1])
			if !ok {
				return true
			}
		}
	} else {
		target = s.miscSelectedPlayerOrSelf()
		if target == nil || target.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		guid, name = target.playerGUID, target.player.Name
	}
	dest := args[len(args)-1]
	// EXACT_SEQUENCE("$home") matches case-insensitively
	// (ChatCommandTags.h:102-104: StringEqualI).
	if strings.EqualFold(dest, "$home") {
		s.teleToHomebind(ctx, target, guid)
		return true
	}
	tele, ok := s.teleResolveTele(ctx, dest)
	if !ok {
		return true
	}
	if target != nil && target.player != nil {
		if s.characterTargetLowerSecurity(ctx, characterTarget{online: target, guid: guid, name: name}) {
			// HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW
			// (Chat.cpp:96-99).
			s.sendSysMessage("Your security level is too low.")
			return true
		}
		if target.isBeingTeleported() {
			s.sendSysMessage(fmt.Sprintf(miscIsTeleported, miscPlayerLink(name))) // LANG_IS_TELEPORTED (102)
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Teleporting %s to %s.", miscPlayerLink(name), tele.name)) // LANG_TELEPORTING_TO (110)
		// needReportToTarget (Chat.cpp:754): target is told only if the
		// invoker is visible globally FOR the target, i.e. the target is
		// the viewer and the invoker the seen object
		// (isVisibleGloballyFor(viewer, target), social.go:118); the
		// console always reports (CliHandler::needReportToTarget,
		// Chat.cpp:801-804, name "Console" per CliHandler::GetNameLink).
		if target != s && (s.player == nil || isVisibleGloballyFor(target, s)) {
			invokerName := "Console"
			if s.player != nil {
				invokerName = s.player.Name
			}
			target.sendSysMessage(fmt.Sprintf("%s teleported you.", miscPlayerLink(invokerName))) // LANG_TELEPORTED_TO_BY (111)
		}
		if target.isInFlight() {
			target.finishTaxiFlight()
		} else {
			target.saveRecallPosition() // C++ SaveRecallPosition: save only in non-flight case
		}
		target.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{guid: guid, name: name}) {
		// HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW
		// (Chat.cpp:96-99).
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Teleporting %s%s to %s.", miscPlayerLink(name), " (offline)", tele.name)) // LANG_TELEPORTING_TO (110) + LANG_OFFLINE (37)
	// Player::SavePositionInDB (Player.cpp:20545) writes map + position +
	// orientation + zone; the zone is sMapMgr::GetZoneId(PHASEMASK_NORMAL,
	// tele) (Map::GetZoneId, Map.cpp:2631), bridged by
	// Server.zoneAndAreaID (terrain.go:109).
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		zoneID, _ := s.server.zoneAndAreaID(tele.mapID, tele.x, tele.y, tele.z, 0)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE characters SET map = ?, zone = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			tele.mapID, zoneID, tele.x, tele.y, tele.z, tele.o, guid)
	}
	return true
}

// telePlayerByGUID resolves a bare numeric PlayerIdentifier token as a
// player GUID low (ChatCommandTags.cpp:95-102): online first, then the
// character row (the sCharacterCache leg).
func (s *session) telePlayerByGUID(ctx context.Context, token string) (online *session, guid uint64, name string, ok bool) {
	low, err := strconv.ParseUint(token, 0, 32)
	if err != nil {
		return nil, 0, "", false
	}
	if s.server != nil {
		if sess := s.server.playerSessionForGUID(low); sess != nil && sess.player != nil {
			return sess, sess.playerGUID, sess.player.Name, true
		}
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var n string
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT name FROM characters WHERE guid = ? LIMIT 1", low).Scan(&n); err == nil && n != "" {
			return nil, low, n, true
		}
	}
	return nil, 0, "", false
}

// teleToHomebind mirrors the $home leg of HandleTeleNameCommand: online
// targets teleport to their stored homebind; offline targets get their
// saved position rewritten from character_homebind.
func (s *session) teleToHomebind(ctx context.Context, target *session, guid uint64) {
	if target != nil && target.player != nil {
		p := target.player
		target.teleportTo(p.HomebindMap, p.HomebindX, p.HomebindY, p.HomebindZ, p.Orientation)
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	var mapID uint32
	var zoneID uint16
	var x, y, z float64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
		"SELECT mapId, zoneId, posX, posY, posZ FROM character_homebind WHERE guid = ? LIMIT 1", guid).Scan(&mapID, &zoneID, &x, &y, &z); err != nil {
		// C++ with no homebind row writes nothing and returns true:
		// a missing row is a silent success, not an error.
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		s.sendSysMessage(fmt.Sprintf("Homebind lookup error: %v", err))
		return
	}
	// SavePositionInDB writes the homebind row's zoneId too (cs_tele.cpp:135-137).
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
		"UPDATE characters SET map = ?, zone = ?, position_x = ?, position_y = ?, position_z = ?, orientation = 0 WHERE guid = ?",
		mapID, zoneID, x, y, z, guid)
}

// teleIsBattlegroundOrArena mirrors MapEntry::IsBattlegroundOrArena
// (DBCStructure.h): InstanceType 3 (battleground) or 4 (arena).
func teleIsBattlegroundOrArena(entry wotlk.MapEntry) bool {
	return entry.InstanceType == 3 || entry.InstanceType == 4
}

// teleGroupTarget mirrors ChatHandler::getSelectedPlayer (Chat.cpp:300) for
// the group arm: no selection resolves to the invoker, but an unresolvable
// selection (targeting a creature, an offline GUID) reports
// LANG_NO_CHAR_SELECTED (116) instead of falling back to self — the same
// convention as the quest port's questTargetPlayer.
func (s *session) teleGroupTarget() *session {
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts
		}
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED (116)
		return nil
	}
	if s.player == nil {
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED (116)
		return nil
	}
	return s
}

// handleTeleGroupCommand mirrors HandleTeleGroupCommand (cs_tele.cpp:199-262,
// RBAC 741): teleports every online member of the selected player's group.
func (s *session) handleTeleGroupCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// GameTele consumes one token (ChatCommandArgs.cpp:51); trailing text
	// fails the parse, it is never joined into the lookup. The typed
	// parse error wins over trailing text (ChatCommand.h:63-73), so the
	// resolve runs before the length check.
	tele, ok := s.teleResolveTele(ctx, args[0])
	if !ok {
		return true
	}
	if len(args) > 1 {
		return false
	}
	selected := s.teleGroupTarget()
	if selected == nil || selected.player == nil {
		return true // teleGroupTarget already reported LANG_NO_CHAR_SELECTED
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: selected, guid: selected.playerGUID, name: selected.player.Name}) {
		// HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW
		// (Chat.cpp:96-99).
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if entry, found, err := s.server.Data.Map(tele.mapID); err != nil || !found || teleIsBattlegroundOrArena(entry) {
		s.sendSysMessage("You cannot teleport to a battleground or arena map.") // LANG_CANNOT_TELE_TO_BG (733)
		return true
	}
	group := s.server.groupOfGUID(selected.playerGUID)
	if group == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in your group.", selected.player.Name)) // LANG_NOT_IN_GROUP (117)
		return true
	}
	srv := s.server
	srv.groupsMu.RLock()
	members := make([]uint64, len(group.Members))
	for i, m := range group.Members {
		members[i] = m.GUID
	}
	srv.groupsMu.RUnlock()
	for _, guid := range members {
		player := srv.findSessionByGUID(guid)
		if player == nil || player.player == nil {
			continue // C++ skips members with no session
		}
		if s.characterTargetLowerSecurity(ctx, characterTarget{online: player, guid: player.playerGUID, name: player.player.Name}) {
			// HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW
			// (Chat.cpp:96-99); the C++ aborts the whole command.
			s.sendSysMessage("Your security level is too low.")
			return true
		}
		if player.isBeingTeleported() {
			s.sendSysMessage(fmt.Sprintf(miscIsTeleported, miscPlayerLink(player.player.Name))) // LANG_IS_TELEPORTED (102)
			continue
		}
		s.sendSysMessage(fmt.Sprintf("Teleporting %s to %s.", miscPlayerLink(player.player.Name), tele.name)) // LANG_TELEPORTING_TO (110)
		// The C++ reports with the selected player's link (cs_tele.cpp:248);
		// needReportToTarget (Chat.cpp:754) gates on the invoker being
		// visible globally FOR the member, i.e. the member is the viewer
		// (social.go:118); the console always reports (Chat.cpp:801-804).
		if player != s && (s.player == nil || isVisibleGloballyFor(player, s)) {
			player.sendSysMessage(fmt.Sprintf("%s teleported you.", miscPlayerLink(selected.player.Name))) // LANG_TELEPORTED_TO_BY (111)
		}
		if player.isInFlight() {
			player.finishTaxiFlight()
		} else {
			player.saveRecallPosition() // C++ SaveRecallPosition: save only in non-flight case
		}
		player.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
	}
	return true
}

// handleTeleBareCommand mirrors HandleTeleCommand (cs_tele.cpp:264-296,
// RBAC 737): teleports the handler to the named location, gated on combat
// (in combat requires the name permission 740) and on battleground/arena
// maps (allowed when already on that map or in GM mode).
func (s *session) handleTeleBareCommand(ctx context.Context, args []string) bool {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if len(args) == 0 {
		return false
	}
	// GameTele consumes one token (ChatCommandArgs.cpp:51); trailing text
	// fails the parse, it is never joined into the lookup. The typed
	// parse error wins over trailing text (ChatCommand.h:63-73), so the
	// resolve runs before the length check.
	tele, ok := s.teleResolveTele(ctx, args[0])
	if !ok {
		return true
	}
	if len(args) > 1 {
		return false
	}
	if s.isInCombat() && !s.commandAllowed(ctx, permissionCommandTeleName) {
		s.sendSysMessage("You are in combat!") // LANG_YOU_IN_COMBAT (23)
		return true
	}
	isGM := s.player.ExtraFlags&playerExtraGMOn != 0
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if entry, found, err := s.server.Data.Map(tele.mapID); err != nil || !found ||
		(teleIsBattlegroundOrArena(entry) && (s.player.Map != tele.mapID || !isGM)) {
		s.sendSysMessage("You cannot teleport to a battleground or arena map.") // LANG_CANNOT_TELE_TO_BG (733)
		return true
	}
	if s.isInFlight() {
		s.finishTaxiFlight()
	} else {
		s.saveRecallPosition() // C++ SaveRecallPosition: save only in non-flight case
	}
	s.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
	return true
}
