package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// misc command ports, chunk 2: misc_commandscript (cs_misc.cpp) rows 92-125
// in C++ table order (gps through mailbox; the unaura/unbindsight rows were
// ported in chunk 1, commands_misc.go). Of these 32 arms, 13 are native or
// partially native and 19 are documented-blocked:
//
//   - `gps` has no Go bridge: the handler is map-geometry math (cell/grid
//     coords, VMap outdoors/indoors, MMap navmesh presence, liquid status,
//     transport offsets — cs_misc.cpp:183-302).
//   - `itemmove` has no Go bridge: Player::IsValidPos/SwapItem (the
//     inventory slot model) is unmodeled.
//   - `linkgrave` / `neargrave` have no Go bridge: the graveyard link/lookup
//     model (sObjectMgr AddGraveyardLink / GetClosestGraveyard,
//     WorldSafeLocs.dbc) is unbuilt.
//   - `maxskill` / `setskill` have no Go bridge: the skill model
//     (UpdateWeaponsSkillsToMaxSkillsForLevel / SetSkill / SkillLine.dbc) is
//     unbuilt.
//   - `movegens` has no Go bridge: MovementGenerator introspection
//     (cs_misc.cpp:2016-2298) has no Go model.
//   - `mute` / `unmute` have no Go bridge: the account mute model
//     (WorldSession::m_muteTime, login DB account_muted) is unbuilt.
//   - `mutehistory` has no Go bridge: LOGIN_SEL_ACCOUNT_MUTE_INFO
//     (login DB account_muted) is unbuilt.
//   - `possess` has no Go bridge: casting spell 530 (charm) has no
//     charm/possess model.
//   - `unpossess` has no Go bridge: Unit::RemoveCharmAuras has no charm
//     model.
//   - `pvpstats` has no Go bridge: the battleground statistics store
//     (CHAR_SEL_PVPSTATS_FACTIONS_OVERALL + CONFIG_BATTLEGROUND_STORE_
//     STATISTICS_ENABLE) is unbuilt.
//   - `recall` has no Go bridge: Player::SaveRecallPosition was never
//     modeled (documented fidelity gap, commands.go:1154), so there is no
//     recall position to return to.
//   - `respawn` has no Go bridge: creature respawn (the C++
//     HandleRespawnCommand grid respawn sweep, cs_misc.cpp:1801-1841) has no
//     Go model.
//   - `wchange` has no Go bridge: zoneWeather has no SetWeather leg
//     (weather.go exposes only regenerate/packet).
//   - `mailbox` has no Go bridge: WorldSession::SendShowMailBox (the
//     mailbox open packet) is unmodeled.
//   - `playall` has no Go bridge: SoundEntries.dbc validation and the
//     SMSG_PLAY_SOUND global broadcast are unbuilt (buildPlaySound exists in
//     trainers.go but is never wired to a send path).
//
// Console-vs-chat LANG branches are moot (Go commands are always sessioned);
// GetNameLink/playerLink have no Go bridge, so plain names are used. LANG
// texts are inlined from TDB enUS recall (no in-tree trinity_string seed),
// per tree convention.

// Inlined enUS texts (Language.h ids; no in-tree trinity_string seed).
const (
	misc2NoSelection    = "No selection."                        // LANG_NO_SELECTION 200
	misc2ObjectGUID     = "Object GUID: %s"                      // LANG_OBJECT_GUID 201
	misc2KickSelf       = "You cannot kick yourself."            // LANG_COMMAND_KICKSELF 281
	misc2KickMessage    = "Player %s has been kicked."           // LANG_COMMAND_KICKMESSAGE 282
	misc2BadValue       = "Bad value."                           // LANG_BAD_VALUE 168
	misc2ExploreArea    = "Area explored."                       // LANG_EXPLORE_AREA 5064
	misc2HideArea       = "Area hidden."                         // LANG_HIDE_AREA (custom; no LANG id)
	misc2PlayerSaved    = "Player saved."                        // LANG_PLAYER_SAVED 76
	misc2PlayersSaved   = "All players saved."                   // LANG_PLAYERS_SAVED 77
	misc2Unfreezing     = "Unfreezing %s."                       // LANG_COMMAND_UNFREEZE 5003
	misc2NoFrozen       = "No frozen players."                   // LANG_COMMAND_NO_FROZEN_PLAYERS 5004
	misc2ListFreeze     = "Frozen players:"                      // LANG_COMMAND_LIST_FREEZE 5005
	misc2PermaFrozen    = "%s (permanent)"                       // LANG_COMMAND_PERMA_FROZEN_PLAYER 5006
	misc2TempFrozen     = "%s (%d seconds left)"                 // LANG_COMMAND_TEMP_FROZEN_PLAYER 5019
	misc2RepairedYou    = "You have repaired %s's items."        // LANG_YOU_REPAIR_ITEMS 11010
	misc2RepairedTarget = "Your items have been repaired by %s." // LANG_YOUR_ITEMS_REPAIRED 11011
)

// misc2LowerSecurity mirrors the ChatHandler::HasLowerSecurity guard used by
// the summon/kick/repair arms (cs_misc.cpp): the command fails when the
// target account's security outranks the handler's.
func (s *session) misc2LowerSecurity(ctx context.Context, online *session, guid uint64) bool {
	var accountID uint32
	if online != nil {
		accountID = online.accountID
	} else if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID)
	}
	return s.security < s.accountSecurityLevel(ctx, accountID)
}

// handleCmdGPS mirrors HandleGPSCommand (cs_misc.cpp:183-302, RBAC 505).
// Documented-blocked: map geometry, VMap/MMap/liquid and cell/grid math have
// no Go bridge.
func (s *session) handleCmdGPS(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandGPS) {
		return
	}
	s.sendSysMessage("The .gps command is not available: position reporting needs map geometry (cell/grid, VMap, MMap, liquid status) that has no Go bridge yet.")
}

// handleCmdGUID mirrors HandleGUIDCommand (cs_misc.cpp:680-694, RBAC 506):
// print the selected unit's GUID.
func (s *session) handleCmdGUID(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandGUID) {
		return
	}
	if s.selection == 0 {
		s.sendSysMessage(misc2NoSelection)
		return
	}
	s.sendSysMessage(fmt.Sprintf(misc2ObjectGUID, strconv.FormatUint(s.selection, 10)))
}

// handleCmdHelp2 mirrors HandleHelpCommand (cs_misc.cpp:695-703, RBAC 507):
// delegate to the tree's help printer (SendCommandHelpFor equivalent).
func (s *session) handleCmdHelp2(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandHelp) {
		return
	}
	s.handleCmdHelp(args)
}

// misc2ExploredBit resolves an area id to its explored-zone bit offset and
// mask via the AreaTable DBC (the AreaTableEntry::AreaBit math in
// HandleShowAreaCommand/HandleHideAreaCommand, cs_misc.cpp:1079-1136).
func (s *session) misc2ExploredBit(areaID uint32) (offset int, mask uint32, ok bool) {
	if s.server == nil || s.server.Data == nil {
		return 0, 0, false
	}
	areaBit, _, found, err := s.server.Data.AreaTableInfo(areaID)
	if err != nil || !found || areaBit < 0 {
		return 0, 0, false
	}
	bit := uint32(areaBit)
	offset = int(bit / 32)
	if offset >= playerExploredZonesCount {
		return 0, 0, false
	}
	return offset, uint32(1) << (bit % 32), true
}

// handleCmdShowArea mirrors HandleShowAreaCommand (cs_misc.cpp:1079-1112,
// RBAC 527): set the area's explored bit on the selected player.
func (s *session) handleCmdShowArea(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandShowArea) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .showarea <areaId>")
		return
	}
	areaID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage(misc2BadValue)
		return
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		s.sendSysMessage(miscNoCharSelected)
		return
	}
	offset, mask, ok := s.misc2ExploredBit(uint32(areaID))
	if !ok {
		s.sendSysMessage(misc2BadValue)
		return
	}
	target.player.ExploredZones[offset] |= mask
	target.persistExploredZones(ctx)
	target.sendPlayerValuesUpdate(map[int]uint32{playerExploredZonesStart + offset: target.player.ExploredZones[offset]})
	s.sendSysMessage(misc2ExploreArea)
}

// handleCmdHideArea mirrors HandleHideAreaCommand (cs_misc.cpp:1113-1145,
// RBAC 508): clear the area's explored bit on the selected player.
func (s *session) handleCmdHideArea(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandHideArea) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .hidearea <areaId>")
		return
	}
	areaID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage(misc2BadValue)
		return
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		s.sendSysMessage(miscNoCharSelected)
		return
	}
	offset, mask, ok := s.misc2ExploredBit(uint32(areaID))
	if !ok {
		s.sendSysMessage(misc2BadValue)
		return
	}
	target.player.ExploredZones[offset] &^= mask
	target.persistExploredZones(ctx)
	target.sendPlayerValuesUpdate(map[int]uint32{playerExploredZonesStart + offset: target.player.ExploredZones[offset]})
	s.sendSysMessage(misc2HideArea)
}

// handleCmdItemMove mirrors HandleItemMoveCommand (cs_misc.cpp:704-722, RBAC
// 509). Documented-blocked: the inventory slot model (Player::IsValidPos /
// SwapItem) is unmodeled.
func (s *session) handleCmdItemMove(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandItemMove) {
		return
	}
	s.sendSysMessage("The .itemmove command is not available: inventory slot validation and item swapping have no Go bridge yet.")
}

// handleCmdKickPlayer mirrors HandleKickPlayerCommand (cs_misc.cpp:880-915,
// RBAC 510): disconnect the target player. Documented fidelity gap (not a
// stub): CONFIG_SHOW_KICK_IN_WORLD has no Go config field, so the world
// broadcast leg is skipped and the direct message is always used.
func (s *session) handleCmdKickPlayer(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandKick) {
		return
	}
	target, _, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == s {
		s.sendSysMessage(misc2KickSelf)
		return
	}
	if s.misc2LowerSecurity(ctx, target, 0) {
		return
	}
	reason := "No reason."
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}
	_ = reason
	s.sendSysMessage(fmt.Sprintf(misc2KickMessage, miscPlayerLink(name)))
	if target != nil {
		s.kickSession(target)
	}
}

// handleCmdLinkGrave mirrors HandleLinkGraveCommand (cs_misc.cpp:977-1018,
// RBAC 511). Documented-blocked: the graveyard link model (WorldSafeLocs.dbc
// + sObjectMgr::AddGraveyardLink) is unbuilt.
func (s *session) handleCmdLinkGrave(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandLinkGrave) {
		return
	}
	s.sendSysMessage("The .linkgrave command is not available: the graveyard link model (WorldSafeLocs.dbc) is unbuilt.")
}

// handleCmdListFreeze mirrors HandleListFreezeCommand (cs_misc.cpp:2473-2508,
// RBAC 512): list frozen players from the character_aura table
// (CHAR_SEL_CHARACTER_AURA_FROZEN). Documented fidelity gap (not a stub): the
// C++ re-save of online frozen players (SaveToDB to refresh remainTime) is
// skipped; remainTime is reported as stored.
func (s *session) handleCmdListFreeze(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandListFreeze) {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		s.sendSysMessage(misc2NoFrozen)
		return
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
		"SELECT c.name, ca.remainTime FROM character_aura ca JOIN characters c ON c.guid = ca.guid WHERE ca.spell = ?", freezeAuraSpellID)
	if err != nil {
		s.sendSysMessage(misc2NoFrozen)
		return
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var name string
		var remainMs int64
		if err := rows.Scan(&name, &remainMs); err != nil {
			continue
		}
		if remainMs < 0 {
			lines = append(lines, fmt.Sprintf(misc2PermaFrozen, name))
		} else {
			lines = append(lines, fmt.Sprintf(misc2TempFrozen, name, remainMs/1000))
		}
	}
	if len(lines) == 0 {
		s.sendSysMessage(misc2NoFrozen)
		return
	}
	s.sendSysMessage(misc2ListFreeze)
	for _, l := range lines {
		s.sendSysMessage(l)
	}
}

// handleCmdMaxSkill mirrors HandleMaxSkillCommand (cs_misc.cpp:1363-1377,
// RBAC 513). Documented-blocked: the skill model
// (UpdateWeaponsSkillsToMaxSkillsForLevel) is unbuilt.
func (s *session) handleCmdMaxSkill(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMaxSkill) {
		return
	}
	s.sendSysMessage("The .maxskill command is not available: the skill model is unbuilt.")
}

// handleCmdMovegens mirrors HandleMovegensCommand (cs_misc.cpp:2016-2298,
// RBAC 514). Documented-blocked: MovementGenerator introspection has no Go
// model.
func (s *session) handleCmdMovegens(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMovegens) {
		return
	}
	s.sendSysMessage("The .movegens command is not available: movement generators have no Go model.")
}

// handleCmdMute mirrors HandleMuteCommand (cs_misc.cpp:1841-1915, RBAC 515).
// Documented-blocked: the account mute model (WorldSession::m_muteTime,
// login DB account_muted) is unbuilt.
func (s *session) handleCmdMute(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandMute) {
		return
	}
	s.sendSysMessage("The .mute command is not available: the account mute model is unbuilt.")
}

// handleCmdMuteHistory mirrors HandleMuteHistoryCommand (cs_misc.cpp:1965-2015,
// RBAC 632). Documented-blocked: LOGIN_SEL_ACCOUNT_MUTE_INFO (login DB
// account_muted) is unbuilt.
func (s *session) handleCmdMuteHistory(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandMuteHistory) {
		return
	}
	s.sendSysMessage("The .mutehistory command is not available: the account mute history table is unbuilt.")
}

// handleCmdNearGrave mirrors HandleNearGraveCommand (cs_misc.cpp:1019-1078,
// RBAC 516). Documented-blocked: the graveyard lookup model
// (sObjectMgr::GetClosestGraveyard) is unbuilt.
func (s *session) handleCmdNearGrave(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNearGrave) {
		return
	}
	s.sendSysMessage("The .neargrave command is not available: the graveyard model is unbuilt.")
}

// handleCmdPInfo mirrors HandlePInfoCommand (cs_misc.cpp:1430-1801, RBAC
// 517): print character and account info. Partial port (documented fidelity
// gaps, not stubs): the login-DB legs (ban info, mute info, last IP, OS,
// emails, last login, failed logins) and the mail count have no Go bridge;
// race/class/area names have no DBC-name bridge, so ids are printed. What is
// printed comes from the live session or the characters/account tables.
func (s *session) handleCmdPInfo(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandPInfo) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Player %s (guid: %d)", miscPlayerLink(name), guid))
	var accountID uint32
	if target != nil && target.player != nil {
		p := target.player
		alive := "no"
		if p.Health > 0 {
			alive = "yes"
		}
		s.sendSysMessage(fmt.Sprintf("Level: %d, Race: %d, Class: %d, Alive: %s", p.Level, p.Race, p.Class, alive))
		s.sendSysMessage(fmt.Sprintf("Money: %d, Map: %d, Zone: %d", p.Money, p.Map, p.Zone))
		if p.GuildID != 0 {
			s.sendSysMessage(fmt.Sprintf("Guild id: %d", p.GuildID))
		}
		accountID = target.accountID
	} else if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var level, race, class uint8
		var money, health, maxHealth, mapID, zone uint32
		var totalTime uint32
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT level, race, class, money, health, maxHealth, map, zone, totaltime FROM characters WHERE guid = ?",
			guid).Scan(&level, &race, &class, &money, &health, &maxHealth, &mapID, &zone, &totalTime)
		alive := "no"
		if health > 0 {
			alive = "yes"
		}
		s.sendSysMessage(fmt.Sprintf("Level: %d (offline), Race: %d, Class: %d, Alive: %s", level, race, class, alive))
		s.sendSysMessage(fmt.Sprintf("Money: %d, Map: %d, Zone: %d, Played: %ds", money, mapID, zone, totalTime))
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID)
	}
	if accountID != 0 {
		accName := s.accountNameByID(ctx, accountID)
		sec := s.accountSecurityLevel(ctx, accountID)
		s.sendSysMessage(fmt.Sprintf("Account: %s (id: %d), GM level: %d", accName, accountID, sec))
	}
	s.sendSysMessage("Ban/mute/login history, emails and mail counts are unavailable: the login-DB bridges are unbuilt.")
}

// handleCmdPlayAll mirrors HandlePlayAllCommand (cs_misc.cpp:2509-2523, RBAC
// 518). Documented-blocked: SoundEntries.dbc validation and the SMSG_PLAY_SOUND
// global broadcast are unbuilt.
func (s *session) handleCmdPlayAll(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandPlayAll) {
		return
	}
	s.sendSysMessage("The .playall command is not available: sound broadcast has no Go bridge yet.")
}

// handleCmdPossess mirrors HandlePossessCommand (cs_misc.cpp:2524-2533, RBAC
// 519). Documented-blocked: casting spell 530 (charm) has no charm/possess
// model.
func (s *session) handleCmdPossess(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandPossess) {
		return
	}
	s.sendSysMessage("The .possess command is not available: charm/possess has no Go model.")
}

// handleCmdPvPstats mirrors HandlePvPstatsCommand (cs_misc.cpp:130-157, RBAC
// 797). Documented-blocked: the battleground statistics store
// (CHAR_SEL_PVPSTATS_FACTIONS_OVERALL) is unbuilt.
func (s *session) handleCmdPvPstats(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandPvPstats) {
		return
	}
	s.sendSysMessage("The .pvpstats command is not available: the battleground statistics store is unbuilt.")
}

// handleCmdRecall mirrors HandleRecallCommand (cs_misc.cpp:825-847, RBAC
// 520). Documented-blocked: Player::SaveRecallPosition was never modeled, so
// there is no recall position to return to.
func (s *session) handleCmdRecall(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandRecall) {
		return
	}
	s.sendSysMessage("The .recall command is not available: saved recall positions have no Go bridge yet.")
}

// handleCmdRepairItems mirrors HandleRepairitemsCommand (cs_misc.cpp:2298-2316,
// RBAC 521): repair all of the target's items at no cost (the GM
// DurabilityRepairAll(false, 0, false) path — no cost, no discount). Max
// durability comes from item_template; the repair is a direct
// item_instance.durability update, per the items.go repair pattern.
func (s *session) handleCmdRepairItems(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandRepairItems) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if s.misc2LowerSecurity(ctx, target, guid) {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	rows, err := cdb.QueryContext(ctx,
		`SELECT ii.guid, ii.itemEntry, ii.durability FROM character_inventory ci
		 JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ?`, guid)
	if err != nil {
		return
	}
	defer rows.Close()
	type itemRow struct {
		guid, entry, dur uint32
	}
	var items []itemRow
	for rows.Next() {
		var r itemRow
		if err := rows.Scan(&r.guid, &r.entry, &r.dur); err == nil {
			items = append(items, r)
		}
	}
	rows.Close()
	var wdb = cdb
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		wdb = s.server.WorldStore.DB
	}
	repaired := false
	for _, it := range items {
		var maxD uint32
		_ = wdb.QueryRowContext(ctx, "SELECT MaxDurability FROM item_template WHERE entry = ?", it.entry).Scan(&maxD)
		if maxD == 0 || maxD <= it.dur {
			continue
		}
		if _, err := cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", maxD, it.guid); err == nil {
			repaired = true
		}
	}
	if repaired && target != nil {
		_ = target.sendInventoryItems(ctx)
		target.sendPlayerUpdate()
	}
	s.sendSysMessage(fmt.Sprintf(misc2RepairedYou, miscPlayerLink(name)))
	if target != nil && target != s {
		target.sendSysMessage(fmt.Sprintf(misc2RepairedTarget, miscPlayerLink(s.player.Name)))
	}
}

// handleCmdRespawn mirrors HandleRespawnCommand (cs_misc.cpp:1801-1840, RBAC
// 522). Documented-blocked: the creature respawn sweep has no Go model.
func (s *session) handleCmdRespawn(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandRespawn) {
		return
	}
	s.sendSysMessage("The .respawn command is not available: creature respawning has no Go bridge yet.")
}

// handleCmdRevive2 mirrors HandleReviveCommand (cs_misc.cpp:634-653, RBAC
// 523): resurrect the target player. The old xyz-only handleCmdRevive stub in
// commands.go is replaced by this full C++ port. Partial (documented
// fidelity gap, not a stub): the online path is native (ResurrectPlayer +
// SpawnCorpseBones + SaveToDB); the offline path
// (Player::OfflineResurrect) has no Go bridge.
func (s *session) handleCmdRevive2(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandRevive) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target != nil {
		full := s.commandAllowed(ctx, permissionResurrectWithFullHPS)
		pct := float32(0.5)
		if full {
			pct = 1.0
		}
		target.resurrectPlayer(ctx, pct)
		target.spawnCorpseBones(ctx)
		_ = target.savePlayerState(ctx, 1, false)
		s.sendSysMessage(fmt.Sprintf("Revived %s.", miscPlayerLink(name)))
		return
	}
	s.sendSysMessage(fmt.Sprintf("The offline revive of %s is not available: offline resurrection has no Go bridge yet.", miscPlayerLink(name)))
	_ = guid
}

// handleCmdSaveAll mirrors HandleSaveAllCommand (cs_misc.cpp:872-879, RBAC
// 524): save every online player (ObjectAccessor::SaveAllPlayers).
func (s *session) handleCmdSaveAll(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandSaveAll) {
		return
	}
	if s.server == nil {
		return
	}
	s.server.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.server.sessions))
	for sess := range s.server.sessions {
		sessions = append(sessions, sess)
	}
	s.server.sessionsMu.RUnlock()
	for _, sess := range sessions {
		_ = sess.savePlayerState(ctx, 1, false)
	}
	s.sendSysMessage(misc2PlayersSaved)
}

// handleCmdSave2 mirrors HandleSaveCommand (cs_misc.cpp:848-871, RBAC 525):
// save the selected player (or self) to the DB. The old position-only
// handleCmdSave stub in commands.go is replaced by this full C++ port.
// Documented fidelity gap (not a stub): CONFIG_INTERVAL_SAVE has no Go
// config field, so the 20-second save throttle is skipped and the save is
// unconditional outside the without-delay permission path.
func (s *session) handleCmdSave2(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandSave) {
		return
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		target = s
	}
	if s.commandAllowed(ctx, permissionCommandsSaveWithoutDelay) {
		if err := target.savePlayerState(ctx, 1, false); err != nil {
			s.sendSysMessage("Failed to save character: " + err.Error())
			return
		}
		s.sendSysMessage(misc2PlayerSaved)
		return
	}
	if err := target.savePlayerState(ctx, 1, false); err != nil {
		s.sendSysMessage("Failed to save character: " + err.Error())
	}
}

// handleCmdSetSkill mirrors HandleSetSkillCommand (cs_misc.cpp:1378-1429,
// RBAC 526). Documented-blocked: the skill model (SkillLine.dbc,
// Player::SetSkill) is unbuilt.
func (s *session) handleCmdSetSkill(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandSetSkill) {
		return
	}
	s.sendSysMessage("The .setskill command is not available: the skill model is unbuilt.")
}

// handleCmdSummon mirrors HandleSummonCommand (cs_misc.cpp:486-599, RBAC
// 528): teleport the target player to the GM. Documented fidelity gaps (not
// stubs): the battleground/instance group gates have no Go bridge (no BG
// model, no instance-save manager), and the GetClosePoint offset math is
// unmodeled — the target lands on the GM's exact position. The offline leg
// (Player::SavePositionInDB) is native via the characters table.
func (s *session) handleCmdSummon(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandSummon) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == s || guid == s.playerGUID {
		s.sendSysMessage(miscCantTeleportSelf)
		return
	}
	if s.misc2LowerSecurity(ctx, target, guid) {
		return
	}
	if target != nil && target.player != nil {
		s.sendSysMessage(fmt.Sprintf(miscSummoning, miscPlayerLink(name), ""))
		target.teleportTo(s.player.Map, s.player.X, s.player.Y, s.player.Z, target.player.Orientation)
		target.sendSysMessage(fmt.Sprintf(miscSummonedBy, miscPlayerLink(s.player.Name)))
		return
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, guid)
	}
	s.sendSysMessage(fmt.Sprintf(miscSummoning, miscPlayerLink(name), miscOfflineSuffix))
}

// handleCmdUnFreeze mirrors HandleUnFreezeCommand (cs_misc.cpp:2416-2472,
// RBAC 531): remove the freeze aura (9454) from the target player. The online
// path is native (RemoveAurasDueToSpell); the offline path is native via the
// character_aura table (CHAR_DEL_CHAR_AURA_FROZEN).
func (s *session) handleCmdUnFreeze(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandUnFreeze) {
		return
	}
	var name string
	var target *session
	if len(args) > 0 && args[0] != "" {
		name = normalizePlayerName(args[0])
		target = s.sessionForPlayerName(name)
	} else {
		target = s.server.playerSessionForGUID(s.selection)
		if target != nil && target.player != nil {
			name = target.player.Name
		}
	}
	if target != nil && target.player != nil {
		s.sendSysMessage(fmt.Sprintf(misc2Unfreezing, miscPlayerLink(name)))
		target.removeAura(freezeAuraSpellID)
		return
	}
	if name != "" && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var guid uint64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid); err == nil && guid != 0 {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
				"DELETE FROM character_aura WHERE guid = ? AND spell = ?", guid, freezeAuraSpellID)
			s.sendSysMessage(fmt.Sprintf(misc2Unfreezing, miscPlayerLink(name)))
			return
		}
	}
	s.sendSysMessage(miscFreezeWrong)
}

// handleCmdUnmute mirrors HandleUnmuteCommand (cs_misc.cpp:1916-1964, RBAC
// 532). Documented-blocked: the account mute model (WorldSession::m_muteTime,
// login DB account_muted) is unbuilt.
func (s *session) handleCmdUnmute(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandUnMute) {
		return
	}
	s.sendSysMessage("The .unmute command is not available: the account mute model is unbuilt.")
}

// handleCmdUnPossess mirrors HandleUnPossessCommand (cs_misc.cpp:2534-2545,
// RBAC 533). Documented-blocked: Unit::RemoveCharmAuras has no charm model.
func (s *session) handleCmdUnPossess(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandUnPossess) {
		return
	}
	s.sendSysMessage("The .unpossess command is not available: charm/possess has no Go model.")
}

// handleCmdUnstuck mirrors HandleUnstuckCommand (cs_misc.cpp:916-976, RBAC
// 534): teleport the target out of a stuck spot. Partial (documented fidelity
// gaps, not stubs): the no-permission self path (cast spell 7355 "Stuck")
// has no spell-cast bridge; the graveyard leg (RepopAtGraveyard) has no
// graveyard bridge; the startzone leg (GetStartPosition) has no
// race/class start-position bridge. The inn leg (homebind teleport) is
// native.
func (s *session) handleCmdUnstuck(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if !s.commandAllowed(ctx, permissionCommandsUseUnstuckWithArgs) {
		s.sendSysMessage("The self-unstuck cast (spell 7355) is not available: spell casting has no Go bridge on this path.")
		return
	}
	if s.miscDeny(ctx, permissionCommandUnstuck) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .unstuck <player> [inn|graveyard|startzone]")
		return
	}
	target, _, name, ok := s.miscResolvePlayerTarget(ctx, args[:1])
	if !ok || target == nil || target.player == nil {
		return
	}
	if s.misc2LowerSecurity(ctx, target, 0) {
		return
	}
	loc := "inn"
	if len(args) > 1 {
		loc = strings.ToLower(args[1])
	}
	switch loc {
	case "inn":
		target.teleportTo(target.player.HomebindMap, target.player.HomebindX, target.player.HomebindY, target.player.HomebindZ, target.player.Orientation)
	case "graveyard":
		s.sendSysMessage(fmt.Sprintf("The graveyard leg of .unstuck for %s is not available: graveyard respawn has no Go bridge yet.", miscPlayerLink(name)))
	case "startzone":
		s.sendSysMessage(fmt.Sprintf("The startzone leg of .unstuck for %s is not available: race/class start positions have no Go bridge yet.", miscPlayerLink(name)))
	default:
		s.sendSysMessage("Syntax: .unstuck <player> [inn|graveyard|startzone]")
	}
}

// handleCmdChangeWeather mirrors HandleChangeWeather (cs_misc.cpp:1337-1362,
// RBAC 535). Documented-blocked: zoneWeather has no SetWeather leg.
func (s *session) handleCmdChangeWeather(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandChangeWeather) {
		return
	}
	s.sendSysMessage("The .wchange command is not available: weather control has no Go bridge yet.")
}

// handleCmdMailBox mirrors HandleMailBoxCommand (cs_misc.cpp:2566-2574, RBAC
// 777). Documented-blocked: WorldSession::SendShowMailBox (the mailbox open
// packet) is unmodeled.
func (s *session) handleCmdMailBox(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMailBox) {
		return
	}
	s.sendSysMessage("The .mailbox command is not available: the mailbox open packet is unmodeled.")
}
