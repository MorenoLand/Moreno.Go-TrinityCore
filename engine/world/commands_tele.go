package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
// Documented fidelity gaps (not stubs): Player::SaveRecallPosition has no Go
// bridge (same gap as goDoTeleport, cs_go.cpp:73); Player::IsBeingTeleported
// has no Go bridge, so teleportTo is the single re-entrant path (as in the
// group summon port); the zone column written by Player::SavePositionInDB
// is not rewritten (sMapMgr::GetZoneId has no Go bridge); a name miss in
// teleResolveTele reports the LANG_COMMAND_TELE_NOTFOUND (164) stand-in —
// C++ reports LANG_CMDPARSER_GAME_TELE_NO_EXIST (1512) whose enUS text is
// not in the tree, and the handler's own null checks are dead arms (the
// non-Optional GameTele parse fails before the handler runs).
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

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
	t, err = scanTeleRow(db.QueryRowContext(ctx,
		"SELECT id, name, map, position_x, position_y, position_z, orientation FROM game_tele WHERE LOWER(name) LIKE ? LIMIT 1",
		"%"+strings.ToLower(token)+"%"))
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
func (s *session) handleCmdTele(ctx context.Context, args []string) {
	const syntax = "Syntax: .tele add <name>|.tele del <name>|.tele name [player] <tele|$home>|.tele group <tele>|.tele <name>"
	toks := splitQuotedArgs(args)
	if len(toks) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(toks[0])
	rest := toks[1:]
	switch {
	case strings.HasPrefix("add", sub):
		if s.miscDeny(ctx, permissionCommandTeleAdd) {
			return
		}
		s.handleTeleAddCommand(ctx, rest)
	case strings.HasPrefix("del", sub):
		if s.miscDeny(ctx, permissionCommandTeleDel) {
			return
		}
		s.handleTeleDelCommand(ctx, rest)
	case strings.HasPrefix("name", sub):
		if s.miscDeny(ctx, permissionCommandTeleName) {
			return
		}
		s.handleTeleNameCommand(ctx, rest)
	case strings.HasPrefix("group", sub):
		if s.miscDeny(ctx, permissionCommandTeleGroup) {
			return
		}
		s.handleTeleGroupCommand(ctx, rest)
	default:
		if s.miscDeny(ctx, permissionCommandTele) {
			return
		}
		s.handleTeleBareCommand(ctx, toks)
	}
}

// handleTeleAddCommand mirrors HandleTeleAddCommand (cs_tele.cpp:68-99,
// RBAC 738): in-game only, saves the handler's current position to
// game_tele (ObjectMgr::AddGameTele).
func (s *session) handleTeleAddCommand(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .tele add <name>")
		return
	}
	name := unquoteCommandToken(strings.Join(args, " "))
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	var existing string
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT name FROM game_tele WHERE LOWER(name) = LOWER(?) LIMIT 1", name).Scan(&existing)
	if err == nil {
		s.sendSysMessage("The location already exists.") // LANG_COMMAND_TP_ALREADYEXIST (462)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
		return
	}
	p := s.player
	_, err = s.server.WorldStore.DB.ExecContext(ctx,
		"INSERT INTO game_tele (name, map, position_x, position_y, position_z, orientation) VALUES (?, ?, ?, ?, ?, ?)",
		name, p.Map, p.X, p.Y, p.Z, p.Orientation)
	if err != nil {
		s.sendSysMessage("Error adding location.") // LANG_COMMAND_TP_ADDEDERR (464)
		return
	}
	s.sendSysMessage("Location added.") // LANG_COMMAND_TP_ADDED (463)
}

// handleTeleDelCommand mirrors HandleTeleDelCommand (cs_tele.cpp:101-114,
// RBAC 739, console allowed): deletes the resolved game_tele row.
func (s *session) handleTeleDelCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .tele del <name>")
		return
	}
	tele, ok := s.teleResolveTele(ctx, unquoteCommandToken(strings.Join(args, " ")))
	if !ok {
		return
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if _, err := s.server.WorldStore.DB.ExecContext(ctx, "DELETE FROM game_tele WHERE id = ?", tele.id); err != nil {
		s.sendSysMessage(fmt.Sprintf("Teleport delete error: %v", err))
		return
	}
	s.sendSysMessage("Location deleted.") // LANG_COMMAND_TP_DELETED (465)
}

// handleTeleNameCommand mirrors HandleTeleNameCommand (cs_tele.cpp:117-196,
// RBAC 740, console allowed): teleports [player] (default: target-or-self)
// to <tele> or $home. Online targets ride teleportTo; offline targets get
// their saved characters position rewritten.
func (s *session) handleTeleNameCommand(ctx context.Context, args []string) {
	if len(args) == 0 || len(args) > 2 {
		s.sendSysMessage("Syntax: .tele name [player] <tele|$home>")
		return
	}
	var target *session
	var guid uint64
	var name string
	if len(args) == 2 {
		var ok bool
		target, guid, name, ok = s.miscResolvePlayerTarget(ctx, args[:1])
		if !ok {
			return
		}
	} else {
		target = s.miscSelectedPlayerOrSelf()
		if target == nil || target.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return
		}
		guid, name = target.playerGUID, target.player.Name
	}
	dest := unquoteCommandToken(args[len(args)-1])
	if dest == "$home" {
		s.teleToHomebind(ctx, target, guid)
		return
	}
	tele, ok := s.teleResolveTele(ctx, dest)
	if !ok {
		return
	}
	if target != nil && target.player != nil {
		if s.characterTargetLowerSecurity(ctx, characterTarget{online: target, guid: guid, name: name}) {
			return // C++ HasLowerSecurity: silent fail
		}
		s.sendSysMessage(fmt.Sprintf("Teleporting %s to %s.", miscPlayerLink(name), tele.name)) // LANG_TELEPORTING_TO (110)
		// needReportToTarget: the C++ visibility check has no Go bridge, so
		// every other player is told (as in the group summon port).
		if target != s {
			target.sendSysMessage(fmt.Sprintf("%s teleported you.", miscPlayerLink(s.player.Name))) // LANG_TELEPORTED_TO_BY (111)
		}
		if target.isInFlight() {
			target.finishTaxiFlight()
		}
		// SaveRecallPosition has no Go bridge (same gap as goDoTeleport).
		target.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{guid: guid, name: name}) {
		return // C++ HasLowerSecurity: silent fail
	}
	s.sendSysMessage(fmt.Sprintf("Teleporting %s%s to %s.", miscPlayerLink(name), " (offline)", tele.name)) // LANG_TELEPORTING_TO (110) + LANG_OFFLINE (37)
	// Player::SavePositionInDB: the zone column has no Go bridge
	// (sMapMgr::GetZoneId is unbuilt), so only map + position are rewritten.
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			tele.mapID, tele.x, tele.y, tele.z, tele.o, guid)
	}
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
	var x, y, z float64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
		"SELECT mapId, posX, posY, posZ FROM character_homebind WHERE guid = ? LIMIT 1", guid).Scan(&mapID, &x, &y, &z); err != nil {
		s.sendSysMessage(fmt.Sprintf("Homebind lookup error: %v", err))
		return
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
		"UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = 0 WHERE guid = ?",
		mapID, x, y, z, guid)
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
func (s *session) handleTeleGroupCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .tele group <tele>")
		return
	}
	tele, ok := s.teleResolveTele(ctx, unquoteCommandToken(strings.Join(args, " ")))
	if !ok {
		return
	}
	selected := s.teleGroupTarget()
	if selected == nil || selected.player == nil {
		return // teleGroupTarget already reported LANG_NO_CHAR_SELECTED
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: selected, guid: selected.playerGUID, name: selected.player.Name}) {
		return // C++ HasLowerSecurity: silent fail
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if entry, found, err := s.server.Data.Map(tele.mapID); err != nil || !found || teleIsBattlegroundOrArena(entry) {
		s.sendSysMessage("You cannot teleport to a battleground or arena map.") // LANG_CANNOT_TELE_TO_BG (733)
		return
	}
	group := s.server.groupOfGUID(selected.playerGUID)
	if group == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in your group.", selected.player.Name)) // LANG_NOT_IN_GROUP (117)
		return
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
			return // C++ HasLowerSecurity: aborts the whole command
		}
		s.sendSysMessage(fmt.Sprintf("Teleporting %s to %s.", miscPlayerLink(player.player.Name), tele.name)) // LANG_TELEPORTING_TO (110)
		// The C++ reports with the selected player's link (cs_tele.cpp:248);
		// needReportToTarget's visibility check has no Go bridge, so every
		// other member is told (as in the group summon port).
		if player != s {
			player.sendSysMessage(fmt.Sprintf("%s teleported you.", miscPlayerLink(selected.player.Name))) // LANG_TELEPORTED_TO_BY (111)
		}
		if player.isInFlight() {
			player.finishTaxiFlight()
		}
		// SaveRecallPosition and Player::IsBeingTeleported have no Go
		// bridge (same gaps as goDoTeleport and the group summon port).
		player.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
	}
}

// handleTeleBareCommand mirrors HandleTeleCommand (cs_tele.cpp:264-296,
// RBAC 737): teleports the handler to the named location, gated on combat
// (in combat requires the name permission 740) and on battleground/arena
// maps (allowed when already on that map or in GM mode).
func (s *session) handleTeleBareCommand(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	tele, ok := s.teleResolveTele(ctx, unquoteCommandToken(strings.Join(args, " ")))
	if !ok {
		return
	}
	if s.isInCombat() && !s.commandAllowed(ctx, permissionCommandTeleName) {
		s.sendSysMessage("You are in combat!") // LANG_YOU_IN_COMBAT (23)
		return
	}
	isGM := s.player.ExtraFlags&playerExtraGMOn != 0
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if entry, found, err := s.server.Data.Map(tele.mapID); err != nil || !found ||
		(teleIsBattlegroundOrArena(entry) && (s.player.Map != tele.mapID || !isGM)) {
		s.sendSysMessage("You cannot teleport to a battleground or arena map.") // LANG_CANNOT_TELE_TO_BG (733)
		return
	}
	if s.isInFlight() {
		s.finishTaxiFlight()
	}
	// SaveRecallPosition has no Go bridge (same gap as goDoTeleport).
	s.teleportTo(tele.mapID, tele.x, tele.y, tele.z, tele.o)
}
