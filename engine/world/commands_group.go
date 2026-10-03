// Port of the TrinityCore group_commandscript (cs_group.cpp) into the Go
// command dispatcher.
//
// Sole-source audit: the whole 457-line cs_group.cpp is a single column-0
// `class` def (`group_commandscript`); the name hits only cs_group.cpp and
// cs_script_loader.cpp (decl 35 / call 80), so this unit is the SEVENTEENTH
// Commands group in loader call order (gobject(79) -> group(80) -> guild(81)).
// Registration: AddSC_group_commandscript() = `new group_commandscript();`
//
// 11 arms: group set {leader|assistant|maintank|mainassist} (861/473/862-864),
// group leader (473), disband (474), remove (475), join (476), list (477),
// summon (478). Trinity's ChatCommandNode only permission-checks the invoker
// (leaf) node, so each arm gates its own RBAC permission exactly like the C++.
//
// LANG texts are inlined from TDB enUS recall (the tree carries no
// trinity_string seed); the LANG id is cited on each message.
package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

// groupMemberFlags mirrors GroupMemberFlags (Group.h:70-74).
const (
	groupMemberFlagAssistant  uint8 = 0x01 // MEMBER_FLAG_ASSISTANT
	groupMemberFlagMainTank   uint8 = 0x02 // MEMBER_FLAG_MAINTANK
	groupMemberFlagMainAssist uint8 = 0x04 // MEMBER_FLAG_MAINASSIST
)

// handleCmdGroup dispatches the group sub-commands, mirroring
// group_commandscript::GetCommands (cs_group.cpp:35-68).
func (s *session) handleCmdGroup(ctx context.Context, args []string) {
	const syntax = "Syntax: .group set leader|assistant|maintank|mainassist <name> | .group leader <name> | .group disband [name] | .group remove <name> | .group join <playerInGroup> <player> | .group list [player|guid] | .group summon <name>"
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
	case "set":
		// The C++ groupSetCommandTable is a second nesting level; the
		// Trinity parser prefix-matches at every level, mirrored here.
		if len(args) < 3 {
			s.sendSysMessage(syntax)
			return
		}
		switch lvl := strings.ToLower(args[1]); {
		case strings.HasPrefix("leader", lvl):
			if deny(permissionCommandGroupLeader) {
				return
			}
			s.handleGroupLeaderCommand(args[2:])
		case strings.HasPrefix("assistant", lvl):
			if deny(permissionCommandGroupAssistant) {
				return
			}
			s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagAssistant, "Assistant")
		case strings.HasPrefix("maintank", lvl):
			if deny(permissionCommandGroupMainTank) {
				return
			}
			s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagMainTank, "Main Tank")
		case strings.HasPrefix("mainassist", lvl):
			if deny(permissionCommandGroupMainAssist) {
				return
			}
			s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagMainAssist, "Main Assist")
		default:
			s.sendSysMessage(syntax)
		}
	case "leader":
		if deny(permissionCommandGroupLeader) {
			return
		}
		s.handleGroupLeaderCommand(args[1:])
	case "disband":
		if deny(permissionCommandGroupDisband) {
			return
		}
		s.handleGroupDisbandCommand(ctx, args[1:])
	case "remove":
		if deny(permissionCommandGroupRemove) {
			return
		}
		s.handleGroupRemoveCommand(ctx, args[1:])
	case "join":
		if deny(permissionCommandGroupJoin) {
			return
		}
		s.handleGroupJoinCommand(ctx, args[1:])
	case "list":
		if deny(permissionCommandGroupList) {
			return
		}
		s.handleGroupListCommand(ctx, args[1:])
	case "summon":
		if deny(permissionCommandGroupSummon) {
			return
		}
		s.handleGroupSummonCommand(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
	}
}

// groupPlayerTarget mirrors the player/group/guid triple filled by
// ChatHandler::GetPlayerGroupAndGUIDByName (Chat.cpp:806).
type groupPlayerTarget struct {
	online *session // resolved connected session; nil when the name is offline
	name   string   // display name of the resolved player
	guid   uint64
}

// resolveGroupPlayer mirrors GetPlayerGroupAndGUIDByName with the offline
// flag: an optional name is normalized and looked up among connected players
// (offline=true additionally reads the guid from the characters table); when
// the name resolves to nothing online, the C++ falls back to the selected
// player, then the handler's own player.
func (s *session) resolveGroupPlayer(ctx context.Context, args []string, offline bool) (groupPlayerTarget, []string, bool) {
	var t groupPlayerTarget
	rest := args
	if len(args) > 0 && args[0] != "" {
		name := normalizePlayerName(args[0])
		if name == "" {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND
			return t, rest, false
		}
		rest = args[1:]
		t.online = s.sessionForPlayerName(name)
		if offline && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&t.guid)
		}
	}
	if t.online != nil {
		if t.online.player != nil {
			t.name = t.online.player.Name
		}
		t.guid = t.online.playerGUID
		return t, rest, true
	}
	// C++ fallback: selected player, then the handler's own player.
	target := s
	if s.selection != 0 && s.server != nil {
		if sel := s.server.playerSessionForGUID(s.selection); sel != nil {
			target = sel
		}
	}
	t.online = target
	if target.player != nil {
		t.name = target.player.Name
	}
	t.guid = target.playerGUID
	return t, rest, true
}

// groupOfGUID finds the in-memory group holding the member guid.
func (s *Server) groupOfGUID(guid uint64) *groupState {
	s.groupsMu.RLock()
	defer s.groupsMu.RUnlock()
	for _, g := range s.groups {
		for _, m := range g.Members {
			if m.GUID == guid {
				return g
			}
		}
	}
	return nil
}

// handleGroupLeaderCommand mirrors HandleGroupLeaderCommand
// (cs_group.cpp:161): the named player's group gets a new leader.
func (s *session) handleGroupLeaderCommand(args []string) {
	t, _, ok := s.resolveGroupPlayer(context.Background(), args, false)
	if !ok {
		return
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	if g.LeaderGUID != t.guid {
		s.server.setGroupLeader(g, t.guid)
	}
}

// handleGroupFlagCommand mirrors GroupFlagCommand (cs_group.cpp:187): the
// assistant / main-tank / main-assist raid flags toggle for the named player.
func (s *session) handleGroupFlagCommand(ctx context.Context, args []string, flag uint8, what string) {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	if !g.IsRaid {
		s.sendSysMessage(fmt.Sprintf("%s is not in a raid group.", t.name)) // LANG_GROUP_NOT_IN_RAID_GROUP (1185)
		return
	}
	if flag == groupMemberFlagAssistant && g.LeaderGUID == t.guid {
		s.sendSysMessage("The leader cannot be an assistant.") // LANG_LEADER_CANNOT_BE_ASSISTANT (1187)
		return
	}
	srv := s.server
	srv.groupsMu.Lock()
	idx := -1
	for i, m := range g.Members {
		if m.GUID == t.guid {
			idx = i
			break
		}
	}
	if idx < 0 {
		srv.groupsMu.Unlock()
		return
	}
	apply := g.Members[idx].Flags&flag == 0
	// RemoveUniqueGroupMemberFlag: main tank / main assist are unique.
	if apply && flag != groupMemberFlagAssistant {
		for i := range g.Members {
			g.Members[i].Flags &^= flag
		}
	}
	if apply {
		g.Members[idx].Flags |= flag
	} else {
		g.Members[idx].Flags &^= flag
	}
	srv.groupsMu.Unlock()
	// CHAR_UPD_GROUP_MEMBER_FLAG = Group::SetGroupMemberFlag DB persist.
	if srv.CharactersStore != nil && srv.CharactersStore.DB != nil {
		srv.groupsMu.RLock()
		flags := g.Members[idx].Flags
		srv.groupsMu.RUnlock()
		_, _ = srv.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_UPD_GROUP_MEMBER_FLAG"), flags, t.guid)
	}
	// Group::SendUpdate after a flag change.
	srv.broadcastGroupList(g)
	when := "now"
	if !apply {
		when = "no longer"
	}
	s.sendSysMessage(fmt.Sprintf("%s is %s an %s.", t.name, when, what)) // LANG_GROUP_ROLE_CHANGED (1186)
}

// handleGroupDisbandCommand mirrors HandleGroupDisbandCommand
// (cs_group.cpp:246): the named player's whole group is disbanded.
func (s *session) handleGroupDisbandCommand(ctx context.Context, args []string) {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	s.server.disbandGroup(ctx, g)
}

// handleGroupRemoveCommand mirrors HandleGroupRemoveCommand
// (cs_group.cpp:267): the named player is removed from their group.
func (s *session) handleGroupRemoveCommand(ctx context.Context, args []string) {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	s.server.removeGroupMemberByGUID(ctx, g, t.guid)
}

// handleGroupJoinCommand mirrors HandleGroupJoinCommand (cs_group.cpp:288):
// ".group join <playerInGroup> <player>" pulls the second player into the
// first player's group.
func (s *session) handleGroupJoinCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .group join <playerInGroup> <player>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	source, _, ok := s.resolveGroupPlayer(ctx, args[:1], true)
	if !ok {
		return
	}
	if source.online == nil {
		s.sendSysMessage("Player not found.") // C++ would dereference a null Player*; the name must be online
		return
	}
	groupSource := s.server.groupOfGUID(source.guid)
	if groupSource == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", source.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	target, _, ok := s.resolveGroupPlayer(ctx, args[1:2], true)
	if !ok {
		return
	}
	if target.online == nil {
		s.sendSysMessage("Player not found.") // C++ Group::AddMember needs a live player
		return
	}
	if groupTarget := s.server.groupOfGUID(target.guid); groupTarget != nil && (groupTarget == groupSource || target.guid == source.guid) {
		s.sendSysMessage(fmt.Sprintf("%s is already in a group.", target.name)) // LANG_GROUP_ALREADY_IN_GROUP (1145)
		return
	}
	cap := maxGroupSize
	if groupSource.IsRaid {
		cap = 40 // MAXRAIDSIZE (Group.h:43)
	}
	s.server.groupsMu.RLock()
	full := len(groupSource.Members) >= cap
	s.server.groupsMu.RUnlock()
	if full {
		s.sendSysMessage("This group is full.") // LANG_GROUP_FULL (1148)
		return
	}
	subGroup := uint8(0)
	if groupSource.IsRaid {
		// First non-full subgroup, mirroring Group::AddMember (Group.cpp:404).
		counts := [8]int{}
		s.server.groupsMu.RLock()
		for _, m := range groupSource.Members {
			if m.SubGroup < 8 {
				counts[m.SubGroup]++
			}
		}
		s.server.groupsMu.RUnlock()
		subGroup = 8
		for i := 0; i < 8; i++ {
			if counts[i] < maxGroupSize {
				subGroup = uint8(i)
				break
			}
		}
		if subGroup == 8 {
			s.sendSysMessage("This group is full.") // LANG_GROUP_FULL (1148)
			return
		}
	}
	s.server.groupsMu.Lock()
	groupSource.Members = append(groupSource.Members, groupMember{GUID: target.guid, Name: target.name, SubGroup: subGroup})
	s.server.groupsMu.Unlock()
	target.online.groupID = groupSource.ID
	// Group::_addMember DB persist (Group.cpp:465).
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP_MEMBER"), groupSource.DBID, target.guid, 0, subGroup, 0)
	}
	s.server.broadcastGroupList(groupSource) // Group::BroadcastGroupUpdate
	s.sendSysMessage(fmt.Sprintf("%s joined %s's group.", target.name, source.name)) // LANG_GROUP_PLAYER_JOINED (1146)
}

// handleGroupListCommand mirrors HandleGroupListCommand (cs_group.cpp:335):
// the target may be a bare guid, a name, or (by selection) the GM's target.
func (s *session) handleGroupListCommand(ctx context.Context, args []string) {
	var (
		nameTarget string
		guidTarget uint64
		playerSess *session
	)
	if len(args) > 0 && args[0] != "" {
		if guid, err := strconv.ParseUint(args[0], 10, 32); err == nil && guid != 0 {
			// Bare guid form.
			guidTarget = guid
			playerSess = s.server.findSessionByGUID(guid)
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid).Scan(&nameTarget)
			}
		}
	}
	if guidTarget == 0 {
		// extractPlayerTarget: name/link, selection, then self.
		if len(args) > 0 && args[0] != "" {
			name := normalizePlayerName(args[0])
			playerSess = s.sessionForPlayerName(name)
			if playerSess == nil {
				s.sendSysMessage("Player not found.")
				return
			}
		} else if s.selection != 0 && s.server != nil {
			playerSess = s.server.playerSessionForGUID(s.selection)
		}
		if playerSess == nil {
			playerSess = s
		}
		if playerSess.player == nil {
			s.sendSysMessage("Player not found.")
			return
		}
		nameTarget = playerSess.player.Name
		guidTarget = playerSess.playerGUID
	}
	if nameTarget == "" {
		s.sendSysMessage("Player not found.")
		return
	}
	var groupTarget *groupState
	if playerSess != nil && playerSess.groupID != 0 {
		groupTarget = s.server.findGroupByID(playerSess.groupID)
	}
	if groupTarget == nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		// CHAR_SEL_GROUP_MEMBER: find the group through the character DB.
		var dbid uint64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM group_member WHERE memberGuid = ? LIMIT 1", guidTarget).Scan(&dbid); err == nil && dbid != 0 {
			groupTarget = s.server.findGroupByDBID(dbid)
		}
	}
	if groupTarget == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", nameTarget)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return
	}
	s.server.groupsMu.RLock()
	members := make([]groupMember, len(groupTarget.Members))
	copy(members, groupTarget.Members)
	isRaid := groupTarget.IsRaid
	s.server.groupsMu.RUnlock()
	typ := "party"
	if isRaid {
		typ = "raid"
	}
	s.sendSysMessage(fmt.Sprintf("Group type: %s. Members count: %d.", typ, len(members))) // LANG_GROUP_TYPE (1149)
	for _, slot := range members {
		flags := groupFlagNames(slot.Flags)
		onlineState := "Offline"
		zoneName := "<ERROR>"
		var phase uint32
		if p := s.server.findSessionByGUID(slot.GUID); p != nil && p.player != nil {
			onlineState = "online"
			zoneName = "" // AreaTable zone names have no Go bridge
			if p.player.ExtraFlags&playerExtraGMOn == 0 {
				phase = p.currentPlayerPhaseMask()
			} else {
				phase = ^uint32(0) // C++ assigns -1 to the uint32 phase
			}
		}
		s.sendSysMessage(fmt.Sprintf("%s: %s, Zone: %s, Phase: %d, Guid: %d, Flags: %s, Roles: %s",
			slot.Name, onlineState, zoneName, phase, slot.GUID, flags, lfgRolesString(slot.Roles))) // LANG_GROUP_PLAYER_NAME_GUID (1150)
	}
}

// groupFlagNames mirrors the flag-label assembly in HandleGroupListCommand
// (cs_group.cpp:395-415): "Assistant", "MainTank", "MainAssist", "None".
func groupFlagNames(flags uint8) string {
	var parts []string
	if flags&groupMemberFlagAssistant != 0 {
		parts = append(parts, "Assistant")
	}
	if flags&groupMemberFlagMainTank != 0 {
		parts = append(parts, "MainTank")
	}
	if flags&groupMemberFlagMainAssist != 0 {
		parts = append(parts, "MainAssist")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
}

// lfgRolesString mirrors lfg::GetRolesString (LFG.cpp:41): Tank/Healer/
// Damage/Leader from the PLAYER_ROLE bits (LFG.h:39-44), "None" when empty.
// The English role words are inlined; the C++ resolves them through
// trinity_string.
func lfgRolesString(roles uint8) string {
	var parts []string
	if roles&0x02 != 0 { // PLAYER_ROLE_TANK
		parts = append(parts, "Tank")
	}
	if roles&0x04 != 0 { // PLAYER_ROLE_HEALER
		parts = append(parts, "Healer")
	}
	if roles&0x08 != 0 { // PLAYER_ROLE_DAMAGE
		parts = append(parts, "Damage")
	}
	if roles&0x01 != 0 { // PLAYER_ROLE_LEADER
		parts = append(parts, "Leader")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
}

// handleGroupSummonCommand mirrors HandleGroupSummonCommand (cs_group.cpp:70):
// every member of the target's group is summoned to the GM.
func (s *session) handleGroupSummonCommand(ctx context.Context, args []string) {
	// extractPlayerTarget: name, selection, then self.
	var target *session
	if len(args) > 0 && args[0] != "" {
		target = s.sessionForPlayerName(normalizePlayerName(args[0]))
		if target == nil {
			s.sendSysMessage("Player not found.")
			return
		}
	} else if s.selection != 0 && s.server != nil {
		target = s.server.playerSessionForGUID(s.selection)
	}
	if target == nil {
		target = s
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: target, guid: target.playerGUID, name: target.player.Name}) {
		return // C++ HasLowerSecurity: silent fail
	}
	group := s.server.groupOfGUID(target.playerGUID)
	if group == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in your group.", target.player.Name)) // LANG_NOT_IN_GROUP (117)
		return
	}
	srv := s.server
	gmMapID := s.player.Map
	gmInstance := s.player.InstanceID
	toInstance := s.isDungeonMap(gmMapID)
	onlyLocalSummon := false
	if toInstance {
		// The group leader must be on our instance of the map, else no far
		// summon (cs_group.cpp:93-102).
		leader := srv.findSessionByGUID(group.LeaderGUID)
		if leader == nil || leader.player == nil || leader.player.Map != gmMapID || leader.player.InstanceID != gmInstance {
			s.sendSysMessage("Only summoning players that are in your instance.") // LANG_PARTIAL_GROUP_SUMMON (187)
			onlyLocalSummon = true
		}
	}
	srv.groupsMu.RLock()
	members := make([]uint64, len(group.Members))
	for i, m := range group.Members {
		members[i] = m.GUID
	}
	srv.groupsMu.RUnlock()
	for _, guid := range members {
		player := srv.findSessionByGUID(guid)
		if player == nil || player == s || player.player == nil {
			continue
		}
		if s.characterTargetLowerSecurity(ctx, characterTarget{online: player, guid: player.playerGUID, name: player.player.Name}) {
			continue // C++ HasLowerSecurity: silent fail
		}
		// Player::IsBeingTeleported has no Go bridge; teleportTo is the
		// single re-entrant path (documented gap vs LANG_IS_TELEPORTED (102)).
		if toInstance {
			playerMapID := player.player.Map
			instanceable := false
			if entry, found, err := srv.Data.Map(playerMapID); err == nil && found {
				instanceable = entry.IsDungeon()
			}
			if (onlyLocalSummon || (instanceable && playerMapID == gmMapID)) &&
				(playerMapID != gmMapID || player.player.InstanceID != gmInstance) {
				// Cannot summon from instance to instance.
				s.sendSysMessage(fmt.Sprintf("%s cannot be summoned from an instance to an instance.", player.player.Name)) // LANG_CANNOT_SUMMON_INST_INST (107)
				continue
			}
		}
		s.sendSysMessage(fmt.Sprintf("Summoning %s%s.", player.player.Name, "")) // LANG_SUMMONING (108)
		// needReportToTarget = pl != chr && IsVisibleGloballyFor(chr); the
		// visibility check has no Go bridge, so every other member is told.
		if player != s {
			player.sendSysMessage(fmt.Sprintf("You have been summoned by %s.", s.player.Name)) // LANG_SUMMONED_BY (109)
		}
		// Stop flight if needed; SaveRecallPosition has no Go bridge
		// (same gap as goDoTeleport).
		if player.isInFlight() {
			player.finishTaxiFlight()
		}
		// GetClosePoint has no Go bridge: summon to the GM's position.
		player.teleportTo(gmMapID, s.player.X+1, s.player.Y, s.player.Z, player.player.Orientation)
	}
}
