package world

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// This file ports AddSC_instance_commandscript (cs_instance.cpp, whole file
// is 1 column-0 `class` def: 1 x `: public CommandScript`
// "instance_commandscript"). Sole-source verified: the class is referenced
// only by cs_instance.cpp and cs_script_loader.cpp (decl 38 / call 83 -
// TWENTIETH of 39 groups; call order re-verified: honor(82) -> instance(83)
// -> learn(84)).
//
// The C++ arms bottom out in the in-memory InstanceSaveManager (player and
// group binds with perm/extendState/reset-time) and InstanceScript (boss
// encounter states). The Go tree keeps no InstanceSave objects and no
// InstanceScript bridge: binds live only in the character_instance /
// group_instance / instance DB rows (validateBoundInstances in
// player_state.go already treats the DB as the source of truth), and boss
// scripts run in Lua, not in a per-instance C++ script object. The arms that
// can be answered from the DB rows are ported natively; the arms that need
// the live save/script machinery are RBAC-gated and report the missing
// bridge honestly (never stubbed). The boss-state arms gate on the
// per-map instance-script census (instanceScriptEncounterCounts, D40/D41):
// presence is the GetInstanceScript() check and the count is the
// GetEncounterCount() check, so the gates are live even though the script
// objects themselves have no Go bridge.

// instanceExtendExpired/instanceExtendNormal/instanceExtendExtended mirror
// InstanceExtendState (Player.h:762-764); listbinds prints "expired" for
// EXPIRED, "yes" for EXTENDED, "no" otherwise (cs_instance.cpp:108).
const (
	instanceExtendExpired  = 0
	instanceExtendNormal   = 1
	instanceExtendExtended = 2
)

// instanceTimeString mirrors instance_commandscript::GetTimeString
// (cs_instance.cpp:92-102): "Xd Xh Xm", days/hours omitted when zero, minutes
// always printed. A reset time already past yields "0m" (the C++ uint64
// subtraction would wrap to a huge value; expired saves are deleted at
// server load, so the wrap is unreachable in practice).
func instanceTimeString(seconds uint64) string {
	const day, hour, minute = uint64(86400), uint64(3600), uint64(60)
	days := seconds / day
	hours := (seconds % day) / hour
	minutes := (seconds % hour) / minute
	var sb strings.Builder
	if days > 0 {
		fmt.Fprintf(&sb, "%dd ", days)
	}
	if hours > 0 {
		fmt.Fprintf(&sb, "%dh ", hours)
	}
	fmt.Fprintf(&sb, "%dm", minutes)
	return sb.String()
}

// instanceBindRow is one player or group instance bind read straight from
// the DB, the Go equivalent of one Player::BoundInstancesMap entry plus its
// InstanceSave (map, instance id, perm, extendState, difficulty, resetTime).
type instanceBindRow struct {
	mapID       uint32
	instanceID  uint32
	permanent   bool
	extendState uint8
	difficulty  uint8
	resetTime   int64
}

// instanceBindsForCharacter mirrors the listbinds/unbind bind enumeration
// (cs_instance.cpp:104-124, 141-161): the C++ loops difficulties 0..3 and
// walks the in-memory BoundInstancesMap (ordered by map id); the same rows
// are read from character_instance JOIN instance ordered identically.
func (s *session) instanceBindsForCharacter(ctx context.Context, charGUID uint64) []instanceBindRow {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
		`SELECT i.map, i.id, ci.permanent, ci.extendState, i.difficulty, i.resettime
		 FROM character_instance AS ci JOIN instance AS i ON i.id = ci.instance
		 WHERE ci.guid = ? ORDER BY i.difficulty, i.map, i.id`, charGUID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []instanceBindRow
	for rows.Next() {
		var r instanceBindRow
		var perm int64
		var ext, diff int64
		if rows.Scan(&r.mapID, &r.instanceID, &perm, &ext, &diff, &r.resetTime) != nil {
			continue
		}
		r.permanent = perm != 0
		r.extendState = uint8(ext)
		r.difficulty = uint8(diff)
		out = append(out, r)
	}
	return out
}

// instanceBindsForGroup mirrors the group half of listbinds
// (cs_instance.cpp:126-141): group binds come from group_instance JOIN
// instance (group_instance has no extendState column; the C++ prints "-" for
// the ext field, Group::LoadGroupFromDB loads permanent only).
func (s *session) instanceBindsForGroup(ctx context.Context, groupDBID uint32) []instanceBindRow {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
		`SELECT i.map, i.id, gi.permanent, i.difficulty, i.resettime
		 FROM group_instance AS gi JOIN instance AS i ON i.id = gi.instance
		 WHERE gi.guid = ? ORDER BY i.difficulty, i.map, i.id`, groupDBID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []instanceBindRow
	for rows.Next() {
		var r instanceBindRow
		var perm int64
		var diff int64
		if rows.Scan(&r.mapID, &r.instanceID, &perm, &diff, &r.resetTime) != nil {
			continue
		}
		r.permanent = perm != 0
		r.difficulty = uint8(diff)
		out = append(out, r)
	}
	return out
}

// instanceResettable mirrors InstanceSave::CanReset for a bind loaded from
// the DB: _LoadBoundInstances recreates the in-memory save with
// canReset = !perm (Player.cpp:19060); live-created saves pass true
// (Map.cpp:3893) but those are the same binds once reloaded.
func instanceResettable(r instanceBindRow) string {
	if r.permanent {
		return "no"
	}
	return "yes"
}

// sendSavedInstances mirrors Player::SendSavedInstances (Player.cpp:19248):
// SMSG_UPDATE_INSTANCE_OWNERSHIP carries u32(hasBeenSaved) — whether the
// player has any permanent bind — and then one SMSG_UPDATE_LAST_INSTANCE
// (u32 mapId) goes out per permanent bound instance, difficulty order then
// map order like the C++ BoundInstancesMap walk. C++ fires it after every
// far teleport's SMSG_NEW_WORLD (Player.cpp:1878).
func (s *session) sendSavedInstances(ctx context.Context) {
	ob := protocol.NewBuffer(4)
	var maps []uint32
	for _, b := range s.instanceBindsForCharacter(ctx, s.playerGUID) {
		if b.permanent {
			maps = append(maps, b.mapID)
		}
	}
	if len(maps) > 0 {
		ob.WriteU32(1)
	} else {
		ob.WriteU32(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_UPDATE_INSTANCE_OWNERSHIP), ob.Bytes(), true)
	for _, mapID := range maps {
		lb := protocol.NewBuffer(4)
		lb.WriteU32(mapID)
		_ = s.write(uint16(protocol.OpcodeSMSG_UPDATE_LAST_INSTANCE), lb.Bytes(), true)
	}
}

// yesNo renders a bool as the "yes"/"no" literals the C++ bind lines use.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// instanceBindTimeLeft mirrors GetTimeString(save->GetResetTime() -
// GameTime::GetGameTime()) (cs_instance.cpp:109), clamped at zero.
func instanceBindTimeLeft(resetTime int64) string {
	now := time.Now().Unix()
	if resetTime > now {
		return instanceTimeString(uint64(resetTime - now))
	}
	return instanceTimeString(0)
}

// instanceExtendLabel mirrors the extendState rendering in listbinds
// (cs_instance.cpp:110).
func instanceExtendLabel(extendState uint8) string {
	switch extendState {
	case instanceExtendExpired:
		return "expired"
	case instanceExtendExtended:
		return "yes"
	default:
		return "no"
	}
}

// instanceCommandTarget mirrors ChatHandler::getSelectedPlayer
// (cs_instance.cpp:97-99, 135-137): own player when nothing is targeted,
// otherwise the connected player matching the selection. When the selection
// is set but resolves to nothing, Chat::getSelectedPlayer (Chat.cpp:300)
// returns null and the C++ handlers silently fall back to the handler's own
// player (no LANG_PLAYER_NOT_FOUND arm) - so this does the same.
func (s *session) instanceCommandTarget() (*session, bool) {
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
		}
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return nil, false
	}
	return target, true
}

// instancePlayerInDungeon mirrors the ToInstanceMap gate shared by savedata
// and the boss-state arms (cs_instance.cpp:173, 214, 260): the player's map
// must be a dungeon map, else LANG_NOT_DUNGEON (5055).
func (s *session) instancePlayerInDungeon(target *session) bool {
	if s.server == nil || s.server.Data == nil {
		return false
	}
	entry, found, err := s.server.Data.Map(target.player.Map)
	if err != nil || !found || !entry.IsDungeon() {
		s.sendSysMessage("Map is not a dungeon.")
		return false
	}
	return true
}

// handleCmdInstance mirrors the instance command table
// (cs_instance.cpp:49-62):
//
//	instance listbinds                    -> HandleInstanceListBindsCommand   (RBAC_PERM_COMMAND_INSTANCE_LISTBINDS, 413)
//	instance unbind <map|all> [difficulty] -> HandleInstanceUnbindCommand     (RBAC_PERM_COMMAND_INSTANCE_UNBIND, 414)
//	instance stats                        -> HandleInstanceStatsCommand       (RBAC_PERM_COMMAND_INSTANCE_STATS, 415)
//	instance savedata                     -> HandleInstanceSaveDataCommand    (RBAC_PERM_COMMAND_INSTANCE_SAVEDATA, 416)
//	instance setbossstate <enc> <state> [player] -> HandleInstanceSetBossStateCommand (RBAC_PERM_COMMAND_INSTANCE_SET_BOSS_STATE, 795)
//	instance getbossstate <enc> [player]  -> HandleInstanceGetBossStateCommand (RBAC_PERM_COMMAND_INSTANCE_GET_BOSS_STATE, 796)
//
// Trinity checks permission only on the invoker (leaf) node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission.
func (s *session) handleCmdInstance(ctx context.Context, args []string) bool {
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
	// like group: ".instance s" is ambiguous (stats/savedata/setbossstate)
	// only when several are visible, not stats.
	visible := func(perm uint32) bool { return !deny(perm) }
	sub, cands, ok := groupMatchNameVisible(args[0], []groupSubcommand{
		{"listbinds", []uint32{permissionCommandInstanceListBinds}},
		{"unbind", []uint32{permissionCommandInstanceUnbind}},
		{"stats", []uint32{permissionCommandInstanceStats}},
		{"savedata", []uint32{permissionCommandInstanceSaveData}},
		{"setbossstate", []uint32{permissionCommandInstanceSetBossState}},
		{"getbossstate", []uint32{permissionCommandInstanceGetBossState}},
	}, visible)
	if !ok {
		if len(cands) > 1 {
			s.groupAmbiguousMessage(args[0], cands)
			return true
		}
		return false
	}
	switch sub {
	case "listbinds":
		if deny(permissionCommandInstanceListBinds) {
			return false
		}
		return s.handleInstanceListBinds(ctx)
	case "unbind":
		if deny(permissionCommandInstanceUnbind) {
			return false
		}
		return s.handleInstanceUnbind(ctx, args[1:])
	case "stats":
		if deny(permissionCommandInstanceStats) {
			return false
		}
		return s.handleInstanceStats(ctx)
	case "savedata":
		if deny(permissionCommandInstanceSaveData) {
			return false
		}
		return s.handleInstanceSaveData()
	case "setbossstate":
		if deny(permissionCommandInstanceSetBossState) {
			return false
		}
		return s.handleInstanceSetBossState(ctx, args[1:])
	case "getbossstate":
		if deny(permissionCommandInstanceGetBossState) {
			return false
		}
		return s.handleInstanceGetBossState(ctx, args[1:])
	}
	return true
}

// handleInstanceListBinds mirrors HandleInstanceListBindsCommand
// (cs_instance.cpp:80-119): per-bind "map: %d inst: %d perm: %s ext: %s
// diff: %d resettable: %s timeleft: %s" (LANG_COMMAND_LIST_BIND_INFO, 5045),
// then "player binds: %d" (5046), the group binds (ext printed as "-",
// exactly like the C++), then "group binds: %d" (5047). Binds are read from
// the character_instance/group_instance/instance rows, the Go tree's source
// of truth for binds.
func (s *session) handleInstanceListBinds(ctx context.Context) bool {
	target, ok := s.instanceCommandTarget()
	if !ok {
		return true
	}
	binds := s.instanceBindsForCharacter(ctx, target.playerGUID)
	counter := 0
	for _, b := range binds {
		s.sendSysMessage(fmt.Sprintf("map: %d inst: %d perm: %s ext: %s diff: %d resettable: %s timeleft: %s",
			b.mapID, b.instanceID, yesNo(b.permanent),
			instanceExtendLabel(b.extendState), b.difficulty, instanceResettable(b), instanceBindTimeLeft(b.resetTime)))
		counter++
	}
	s.sendSysMessage(fmt.Sprintf("player binds: %d", counter))

	counter = 0
	if target.groupID != 0 && s.server != nil {
		if g := s.server.getGroup(target.groupID); g != nil {
			for _, b := range s.instanceBindsForGroup(ctx, g.DBID) {
				s.sendSysMessage(fmt.Sprintf("map: %d inst: %d perm: %s ext: %s diff: %d resettable: %s timeleft: %s",
					b.mapID, b.instanceID, yesNo(b.permanent),
					"-", b.difficulty, instanceResettable(b), instanceBindTimeLeft(b.resetTime)))
				counter++
			}
		}
	}
	s.sendSysMessage(fmt.Sprintf("group binds: %d", counter))
	return true
}

// handleInstanceUnbind mirrors HandleInstanceUnbindCommand
// (cs_instance.cpp:120-165): "unbind <map|all> [difficulty]" deletes the
// player's binds except the one for the map the player is currently in,
// printing "unbinding map: %d inst: %d perm: %s diff: %d resettable: %s
// timeleft: %s" (5048) per bind and "instances unbound: %d" (5049). The C++
// Player::UnbindInstance also erases the in-memory bind and drops the
// player from the InstanceSave; the Go tree keeps no in-memory InstanceSave
// objects, so the character_instance row delete plus the calendar
// raid-lockout removal for perm binds is the full observable state.
func (s *session) handleInstanceUnbind(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	target, ok := s.instanceCommandTarget()
	if !ok {
		return true
	}
	var mapID uint32
	if args[0] != "all" {
		// C++ compares with strcmp(map, "all") (cs_instance.cpp:143): the
		// keyword match is case-sensitive, unlike the tree's usual folding.
		parsed, ok := parseInstanceMapID(args[0])
		if !ok {
			return false
		}
		mapID = parsed
	}
	// C++ stores the token in int8 (cs_instance.cpp:139-141): atoi's value
	// truncates, so "256" selects difficulty 0 and "300" selects 44 (no
	// match), while a full-int comparison would match nothing for both.
	diff := -1
	if len(args) > 1 {
		diff = int(int8(cAtoi(args[1])))
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	counter := 0
	for _, b := range s.instanceBindsForCharacter(ctx, target.playerGUID) {
		if b.mapID == target.player.Map {
			continue
		}
		if mapID != 0 && mapID != b.mapID {
			continue
		}
		if diff != -1 && diff != int(b.difficulty) {
			continue
		}
		// Delete first: the C++ erase cannot fail, so a failed row delete
		// must not print the "unbinding" line for a bind that stays bound.
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx,
			"DELETE FROM character_instance WHERE guid = ? AND instance = ?", target.playerGUID, b.instanceID); err != nil {
			continue
		}
		// Player::UnbindInstance (Player.cpp:19113-19114) sends the
		// calendar raid-lockout removal for permanent binds, to the
		// bound player's own session.
		if b.permanent {
			var remaining uint32
			if rt := b.resetTime - time.Now().Unix(); rt > 0 {
				remaining = uint32(rt)
			}
			target.sendCalendarRaidLockout(b.mapID, uint32(b.difficulty), remaining, uint64(b.instanceID), false)
		}
		s.sendSysMessage(fmt.Sprintf("unbinding map: %d inst: %d perm: %s diff: %d resettable: %s timeleft: %s",
			b.mapID, b.instanceID, yesNo(b.permanent),
			b.difficulty, instanceResettable(b), instanceBindTimeLeft(b.resetTime)))
		counter++
	}
	s.sendSysMessage(fmt.Sprintf("instances unbound: %d", counter))
	return true
}

// unbindInstanceBinds is the Player::UnbindInstance arm
// (Player.cpp:19099-19115) shared by the Eluna unbind methods: for each
// matching bind row it deletes the character_instance row and, for
// permanent binds, sends the calendar raid-lockout removal with
// seconds-remaining like the bind-creation arm (instance_binds.go).
// The Go tree keeps no in-memory InstanceSave objects, so the row
// delete plus the lockout packet is the full observable state (the
// same convention as the .instance unbind twin).
func (s *session) unbindInstanceBinds(ctx context.Context, match func(instanceBindRow) bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	for _, b := range s.instanceBindsForCharacter(ctx, s.playerGUID) {
		if !match(b) {
			continue
		}
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, b.instanceID); err != nil {
			continue
		}
		if b.permanent {
			var remaining uint32
			if rt := b.resetTime - time.Now().Unix(); rt > 0 {
				remaining = uint32(rt)
			}
			s.sendCalendarRaidLockout(b.mapID, uint32(b.difficulty), remaining, uint64(b.instanceID), false)
		}
	}
}

// handleInstanceStats mirrors HandleInstanceStatsCommand
// (cs_instance.cpp:166-176): "Loaded instances" (sMapMgr->GetNumInstances,
// 5050) has no Go bridge - the Go tree keeps no loaded-map registry - and is
// omitted (documented gap). The remaining four lines are answered natively:
// players in instances (online sessions on dungeon maps), instance saves
// (instance row count), players bound (total player binds:
// InstanceSaveManager::GetNumBoundPlayersTotal sums GetPlayerCount() over
// every save, InstanceSaveMgr.cpp:719-726 - so COUNT(*), not DISTINCT),
// groups bound (same over group_instance, :728-735).
func (s *session) handleInstanceStats(ctx context.Context) bool {
	if s.server == nil {
		return true
	}
	playersIn := 0
	if s.server.Data != nil {
		s.server.sessionsMu.RLock()
		for sess := range s.server.sessions {
			if sess == nil || !sess.playerLoaded || sess.player == nil {
				continue
			}
			if entry, found, err := s.server.Data.Map(sess.player.Map); err == nil && found && entry.IsDungeon() {
				playersIn++
			}
		}
		s.server.sessionsMu.RUnlock()
	}
	var saves, playersBound, groupsBound int64
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM instance").Scan(&saves)
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_instance").Scan(&playersBound)
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM group_instance").Scan(&groupsBound)
	}
	s.sendSysMessage(fmt.Sprintf("Players in instances: %d", playersIn))
	s.sendSysMessage(fmt.Sprintf("Instance saves: %d", saves))
	s.sendSysMessage(fmt.Sprintf("Players bound: %d", playersBound))
	s.sendSysMessage(fmt.Sprintf("Groups bound: %d", groupsBound))
	return true
}

// handleInstanceSaveData mirrors HandleInstanceSaveDataCommand
// (cs_instance.cpp:177-199). The dungeon gate (LANG_NOT_DUNGEON, 5055)
// and the LANG_NO_INSTANCE_DATA (5056) gate are ported in C++ order; the
// save itself needs the player's InstanceScript, which has no Go bridge
// (boss scripts run in Lua, not in a per-instance C++ script object), so
// it reports the gap honestly after the gates.
func (s *session) handleInstanceSaveData() bool {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if !s.instancePlayerInDungeon(s) {
		return true
	}
	if _, ok := instanceScriptEncounterCount(s.player.Map); !ok {
		s.sendSysMessage("No instance data.")
		return true
	}
	s.sendSysMessage("Instance scripts are not modeled in the Go tree, so there is no instance data to save.")
	return true
}

// instanceBossStateTarget mirrors the player resolution shared by the
// boss-state arms (cs_instance.cpp:199-231, 245-277): the optional name
// resolves to an online player (ObjectAccessor::FindPlayerByName); without
// it the handler's own player is used. Null -> LANG_PLAYER_NOT_FOUND (499).
// The name sits at a different index per arm (get: [enc, player], set:
// [enc, state, player]), so the caller passes its index; reading args[1]
// for set would resolve the state token as a player name.
func (s *session) instanceBossStateTarget(args []string, playerIdx int) (*session, bool) {
	if len(args) == 0 {
		return nil, false
	}
	target := s
	if len(args) > playerIdx {
		name := normalizePlayerName(args[playerIdx])
		ts := s.server.findSessionByName(name)
		if ts == nil || ts.player == nil {
			s.sendSysMessage("Player not found.")
			return nil, false
		}
		target = ts
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return nil, false
	}
	return target, true
}

// instanceScriptEncounterCounts is the D40/D41 per-map census: every map
// carrying a C++ InstanceMapScript (InstanceMapScript("name", mapId) in
// src/server/scripts/**/instance_*.cpp, 74 scripts, no duplicate map) and
// that script's encounter count (SetBossNumber, resolved through the
// file's/sibling header's EncounterCount/MAX_ENCOUNTER/NUM_ENCOUNTER;
// scripts with no SetBossNumber count 0, bosses.size()==0). Presence in
// this table is the GetInstanceScript() gate (D40); the count is the
// GetEncounterCount() gate (D41). Generated from the census JSON by
// d40_d41_instance_census.py; re-run that script to refresh.
var instanceScriptEncounterCounts = map[uint32]uint32{
	33:  0,  // shadowfang_keep
	34:  0,  // the_stockade
	36:  7,  // deadmines
	43:  0,  // wailing_caverns
	47:  0,  // razorfen_kraul
	48:  3,  // blackfathom_deeps
	70:  0,  // uldaman
	90:  5,  // gnomeregan
	109: 0,  // sunken_temple
	129: 5,  // razorfen_downs
	189: 9,  // scarlet_monastery
	209: 0,  // zulfarrak
	229: 23, // blackrock_spire
	230: 0,  // blackrock_depths
	249: 1,  // onyxias_lair
	269: 0,  // the_black_morass
	289: 8,  // scholomance
	309: 12, // zulgurub
	329: 0,  // stratholme
	349: 0,  // maraudon
	389: 0,  // ragefire_chasm
	409: 10, // molten_core
	429: 17, // dire_maul
	469: 8,  // blackwing_lair
	509: 6,  // ruins_of_ahnqiraj
	531: 9,  // temple_of_ahnqiraj
	532: 12, // karazhan
	533: 15, // naxxramas
	534: 0,  // hyjal
	540: 4,  // shattered_halls
	542: 3,  // blood_furnace
	543: 4,  // hellfire_ramparts
	544: 1,  // magtheridons_lair
	545: 3,  // steam_vault
	546: 0,  // the_underbog
	547: 0,  // the_slave_pens
	548: 0,  // serpent_shrine
	550: 4,  // the_eye
	552: 4,  // arcatraz
	553: 0,  // the_botanica
	554: 5,  // mechanar
	555: 4,  // shadow_labyrinth
	556: 3,  // sethekk_halls
	557: 4,  // mana_tombs
	558: 2,  // auchenai_crypts
	560: 0,  // old_hillsbrad
	564: 9,  // black_temple
	565: 2,  // gruuls_lair
	568: 6,  // zulaman
	574: 3,  // utgarde_keep
	575: 4,  // utgarde_pinnacle
	576: 5,  // nexus
	578: 4,  // oculus
	580: 6,  // sunwell_plateau
	585: 4,  // magisters_terrace
	595: 5,  // culling_of_stratholme
	599: 4,  // halls_of_stone
	600: 4,  // drak_tharon_keep
	601: 3,  // azjol_nerub
	602: 4,  // halls_of_lightning
	603: 17, // ulduar
	604: 5,  // gundrak
	608: 3,  // violet_hold
	615: 5,  // obsidian_sanctum
	616: 1,  // eye_of_eternity
	619: 5,  // ahnkahet
	624: 4,  // vault_of_archavon
	631: 13, // icecrown_citadel
	632: 2,  // forge_of_souls
	649: 6,  // trial_of_the_crusader
	650: 4,  // trial_of_the_champion
	658: 3,  // pit_of_saron
	668: 3,  // halls_of_reflection
	724: 4,  // ruby_sanctum
}

// instanceScriptEncounterCount mirrors InstanceMap::GetInstanceScript()
// presence plus InstanceScript::GetEncounterCount(): the second return is
// false when the map carries no instance script at all.
func instanceScriptEncounterCount(mapID uint32) (uint32, bool) {
	count, ok := instanceScriptEncounterCounts[mapID]
	return count, ok
}

// handleInstanceSetBossState mirrors HandleInstanceSetBossStateCommand
// (cs_instance.cpp:200-267): the syntax gates (encounter + state required),
// the player resolution, the LANG_NOT_DUNGEON (5055) gate, the
// LANG_NO_INSTANCE_DATA (5056) gate, and the combined state >
// TO_BE_DECIDED / encounterId > GetEncounterCount() LANG_BAD_VALUE gate
// are all ported in C++ order, then the live boss-state store takes the
// set and the LANG_COMMAND_INST_SET_BOSS_STATE line prints
// unconditionally, as in C++ (the SetBossState return is ignored there
// too). A negative state passes the gate like C++ (the check is `state >
// TO_BE_DECIDED` only, cs_instance.cpp:256): it wraps to the uint8 enum,
// and the name prints INVALID via encounterStateName, exactly as
// GetBossStateName does for the wrapped value. The encounter-count gate
// reads instanceScriptEncounterCounts (D41 census); an encounterId equal
// to the count passes the gate but is NOT stored — C++ SetBossState
// ignores id >= bosses.size() (InstanceScript.cpp) while the success
// message still prints. The message wording is a plain-English recall
// (LANG text is TDB).
func (s *session) handleInstanceSetBossState(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .instance setbossstate <encounter> <state> [player]"
	if len(args) == 0 {
		return false
	}
	if len(args) < 2 {
		// State token missing: LANG_CMD_SYNTAX + SetSentErrorMessage
		// (cs_instance.cpp:215-218) -> message + true.
		s.sendSysMessage(syntax)
		return true
	}
	target, ok := s.instanceBossStateTarget(args, 2)
	if !ok {
		return true
	}
	if !s.instancePlayerInDungeon(target) {
		return true
	}
	encounterCount, ok := instanceScriptEncounterCount(target.player.Map)
	if !ok {
		s.sendSysMessage("No instance data.")
		return true
	}
	encounterID := uint32(cAtoi(args[0]))
	state := cAtoi(args[1])
	// C++ rejects state > TO_BE_DECIDED or encounterId >
	// GetEncounterCount() in one check (cs_instance.cpp:256-260); a
	// negative state passes and wraps into the EncounterState enum, so
	// it is set (uint8(state)) and reported INVALID, not Bad value.
	if state > int(encounterStateToBeDecided) || encounterID > encounterCount {
		s.sendSysMessage("Bad value.")
		return true
	}
	// encounterId == count passes the gate but SetBossState ignores
	// id >= bosses.size(); only an in-range id reaches the store.
	if encounterID < encounterCount {
		s.server.setInstanceBossState(ctx, target.player.Map, target.player.InstanceID, encounterID, uint8(state), effectiveTeamFor(target.player), true)
	}
	s.sendSysMessage(fmt.Sprintf("Boss state for encounter %d set to %s (%d).", encounterID, encounterStateName(uint8(state)), state))
	return true
}

// handleInstanceGetBossState mirrors HandleInstanceGetBossStateCommand
// (cs_instance.cpp:268-319): same gates as setbossstate in C++ order, then
// the live store read and the LANG_COMMAND_INST_GET_BOSS_STATE line. An
// encounterId equal to the count passes the gate and reads TO_BE_DECIDED,
// matching C++ GetBossState for id >= bosses.size(). Wording is a recall.
func (s *session) handleInstanceGetBossState(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .instance getbossstate <encounter> [player]"
	if len(args) < 1 {
		return false
	}
	target, ok := s.instanceBossStateTarget(args, 1)
	if !ok {
		return true
	}
	if !s.instancePlayerInDungeon(target) {
		return true
	}
	encounterCount, ok := instanceScriptEncounterCount(target.player.Map)
	if !ok {
		s.sendSysMessage("No instance data.")
		return true
	}
	encounterID := uint32(cAtoi(args[0]))
	if encounterID > encounterCount {
		s.sendSysMessage("Bad value.")
		return true
	}
	state := s.server.instanceBossState(target.player.Map, target.player.InstanceID, encounterID)
	s.sendSysMessage(fmt.Sprintf("Boss state for encounter %d is %s (%d).", encounterID, encounterStateName(state), state))
	return true
}

// parseInstanceMapID parses the <map|all> token's numeric form, mirroring
// MapId = uint16(atoi(map)) plus the !MapId gate (cs_instance.cpp:145-150):
// cAtoi is the tree's atoi clone (stops at the first non-digit, 0 when there
// are none), and the uint16 cast keeps the C++ wrap for out-of-range values.
func parseInstanceMapID(tok string) (uint32, bool) {
	mapID := uint16(cAtoi(tok))
	if mapID == 0 {
		return 0, false
	}
	return uint32(mapID), true
}
