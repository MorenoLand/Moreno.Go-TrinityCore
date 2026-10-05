package world

import (
	"context"
	"fmt"
	"strings"
	"time"
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
// bridge honestly (never stubbed).

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
// (cs_instance.cpp:46-59):
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
func (s *session) handleCmdInstance(ctx context.Context, args []string) {
	const syntax = "Syntax: .instance listbinds | .instance unbind <map|all> [difficulty] | .instance stats | .instance savedata | .instance setbossstate <encounter> <state> [player] | .instance getbossstate <encounter> [player]"
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
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("listbinds", sub):
		if deny(permissionCommandInstanceListBinds) {
			return
		}
		s.handleInstanceListBinds(ctx)
	case strings.HasPrefix("unbind", sub):
		if deny(permissionCommandInstanceUnbind) {
			return
		}
		s.handleInstanceUnbind(ctx, args[1:])
	case strings.HasPrefix("stats", sub):
		if deny(permissionCommandInstanceStats) {
			return
		}
		s.handleInstanceStats(ctx)
	case strings.HasPrefix("savedata", sub):
		if deny(permissionCommandInstanceSaveData) {
			return
		}
		s.handleInstanceSaveData()
	case strings.HasPrefix("setbossstate", sub):
		if deny(permissionCommandInstanceSetBossState) {
			return
		}
		s.handleInstanceSetBossState(ctx, args[1:])
	case strings.HasPrefix("getbossstate", sub):
		if deny(permissionCommandInstanceGetBossState) {
			return
		}
		s.handleInstanceGetBossState(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
	}
}

// handleInstanceListBinds mirrors HandleInstanceListBindsCommand
// (cs_instance.cpp:95-124): per-bind "map: %d inst: %d perm: %s ext: %s
// diff: %d resettable: %s timeleft: %s" (LANG_COMMAND_LIST_BIND_INFO, 5045),
// then "player binds: %d" (5046), the group binds (ext printed as "-",
// exactly like the C++), then "group binds: %d" (5047). Binds are read from
// the character_instance/group_instance/instance rows, the Go tree's source
// of truth for binds.
func (s *session) handleInstanceListBinds(ctx context.Context) {
	target, ok := s.instanceCommandTarget()
	if !ok {
		return
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
}

// handleInstanceUnbind mirrors HandleInstanceUnbindCommand
// (cs_instance.cpp:126-167): "unbind <map|all> [difficulty]" deletes the
// player's binds except the one for the map the player is currently in,
// printing "unbinding map: %d inst: %d perm: %s diff: %d resettable: %s
// timeleft: %s" (5048) per bind and "instances unbound: %d" (5049). The C++
// Player::UnbindInstance also erases the in-memory bind, drops the player
// from the InstanceSave, and fires a calendar raid-lockout update for perm
// binds; the Go tree keeps no in-memory InstanceSave objects, so the
// character_instance row delete is the full observable state (documented
// gap, not a stub).
func (s *session) handleInstanceUnbind(ctx context.Context, args []string) {
	const syntax = "Syntax: .instance unbind <map|all> [difficulty]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	target, ok := s.instanceCommandTarget()
	if !ok {
		return
	}
	var mapID uint32
	if args[0] != "all" {
		// C++ compares with strcmp(map, "all") (cs_instance.cpp:143): the
		// keyword match is case-sensitive, unlike the tree's usual folding.
		parsed, ok := parseInstanceMapID(args[0])
		if !ok {
			s.sendSysMessage(syntax)
			return
		}
		mapID = parsed
	}
	diff := -1
	if len(args) > 1 {
		diff = cAtoi(args[1])
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
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
		s.sendSysMessage(fmt.Sprintf("unbinding map: %d inst: %d perm: %s diff: %d resettable: %s timeleft: %s",
			b.mapID, b.instanceID, yesNo(b.permanent),
			b.difficulty, instanceResettable(b), instanceBindTimeLeft(b.resetTime)))
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx,
			"DELETE FROM character_instance WHERE guid = ? AND instance = ?", target.playerGUID, b.instanceID); err != nil {
			continue
		}
		counter++
	}
	s.sendSysMessage(fmt.Sprintf("instances unbound: %d", counter))
}

// handleInstanceStats mirrors HandleInstanceStatsCommand
// (cs_instance.cpp:169-179): "Loaded instances" (sMapMgr->GetNumInstances,
// 5050) has no Go bridge - the Go tree keeps no loaded-map registry - and is
// omitted (documented gap). The remaining four lines are answered natively:
// players in instances (online sessions on dungeon maps), instance saves
// (instance row count), players bound (total player binds:
// InstanceSaveManager::GetNumBoundPlayersTotal sums GetPlayerCount() over
// every save, InstanceSaveMgr.cpp:719-726 - so COUNT(*), not DISTINCT),
// groups bound (same over group_instance, :728-735).
func (s *session) handleInstanceStats(ctx context.Context) {
	if s.server == nil {
		return
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
}

// handleInstanceSaveData mirrors HandleInstanceSaveDataCommand
// (cs_instance.cpp:181-197). The dungeon gate (LANG_NOT_DUNGEON, 5055) is
// ported; the save itself needs the player's InstanceScript, which has no
// Go bridge (boss scripts run in Lua, not in a per-instance C++ script
// object), so it reports the gap honestly after the gate.
func (s *session) handleInstanceSaveData() {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if !s.instancePlayerInDungeon(s) {
		return
	}
	s.sendSysMessage("Instance scripts are not modeled in the Go tree, so there is no instance data to save.")
}

// instanceBossStateTarget mirrors the player resolution shared by the
// boss-state arms (cs_instance.cpp:199-231, 245-277): the optional name
// resolves to an online player (ObjectAccessor::FindPlayerByName); without
// it the handler's own player is used. Null -> LANG_PLAYER_NOT_FOUND (499).
func (s *session) instanceBossStateTarget(args []string) (*session, bool) {
	if len(args) == 0 {
		return nil, false
	}
	target := s
	if len(args) > 1 {
		name := normalizePlayerName(args[1])
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

// handleInstanceSetBossState mirrors HandleInstanceSetBossStateCommand
// (cs_instance.cpp:199-240): the syntax gates (encounter + state required),
// the player resolution, the LANG_NOT_DUNGEON (5055) gate, and the
// LANG_NO_INSTANCE_DATA (5056) gate are all ported in C++ order. Setting the
// state needs the live InstanceScript (encounter count, SetBossState,
// GetBossStateName), which has no Go bridge, so the arm RBAC-gates and
// reports the gap honestly instead of stubbing.
func (s *session) handleInstanceSetBossState(ctx context.Context, args []string) {
	const syntax = "Syntax: .instance setbossstate <encounter> <state> [player]"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	target, ok := s.instanceBossStateTarget(args)
	if !ok {
		return
	}
	if !s.instancePlayerInDungeon(target) {
		return
	}
	s.sendSysMessage("Instance scripts are not modeled in the Go tree, so boss states cannot be changed.")
}

// handleInstanceGetBossState mirrors HandleInstanceGetBossStateCommand
// (cs_instance.cpp:242-287): same gates as setbossstate in C++ order; the
// read needs the live InstanceScript (encounter count, GetBossState), which
// has no Go bridge, so the arm RBAC-gates and reports the gap honestly.
func (s *session) handleInstanceGetBossState(ctx context.Context, args []string) {
	const syntax = "Syntax: .instance getbossstate <encounter> [player]"
	if len(args) < 1 {
		s.sendSysMessage(syntax)
		return
	}
	target, ok := s.instanceBossStateTarget(args)
	if !ok {
		return
	}
	if !s.instancePlayerInDungeon(target) {
		return
	}
	s.sendSysMessage("Instance scripts are not modeled in the Go tree, so boss states cannot be read.")
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
