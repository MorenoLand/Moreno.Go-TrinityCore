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
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

// groupMemberFlags mirrors GroupMemberFlags (Group.h:70-74).
const (
	groupMemberFlagAssistant  uint8 = 0x01 // MEMBER_FLAG_ASSISTANT
	groupMemberFlagMainTank   uint8 = 0x02 // MEMBER_FLAG_MAINTANK
	groupMemberFlagMainAssist uint8 = 0x04 // MEMBER_FLAG_MAINASSIST
)

// groupMatchName mirrors the ChatCommandNode prefix match
// (ChatCommand.cpp:261-288): case-insensitive, an exact name wins,
// otherwise the token must uniquely prefix one candidate; ambiguous
// tokens return the candidate list (LANG_SUBCMD_AMBIGUOUS = 7), no
// match returns ok=false. Visibility filtering (invisible arms excluded
// from the candidates) is not modeled: permission is checked on the leaf
// after the match, per the tree's deny convention.
func groupMatchName(token string, names []string) (string, []string, bool) {
	cands := make([]groupSubcommand, 0, len(names))
	for _, n := range names {
		cands = append(cands, groupSubcommand{name: n})
	}
	return groupMatchNameVisible(token, cands, nil)
}

// groupSubcommand pairs a prefix-matchable subcommand name with the RBAC
// permissions that make it visible (ChatCommandNode::IsVisible,
// ChatCommand.h:200: invoker-visible, or any visible subcommand). A
// container entry carries its own permission plus every descendant leaf
// permission, so "set" stays visible when any of its arms is; a leaf
// carries just its own. An entry with no permissions counts as visible,
// like a perm-0 tree node (commandNodeVisible).
type groupSubcommand struct {
	name  string
	perms []uint32
}

// groupMatchNameVisible mirrors groupMatchName over the invoker-visible
// candidates only (FilteredCommandListIterator, ChatCommand.cpp:207-237):
// a prefix that matches several names but leaves one visible resolves to
// it, and names the invoker cannot see never surface as ambiguous
// candidates. A nil visible predicate matches everything. The framework
// resolve() already filters the first subcommand token this way; this
// covers the paths that reach the handlers raw — a token with no visible
// match, and the nested group-set / rbac-account levels, which resolve()
// never descends into.
func groupMatchNameVisible(token string, cands []groupSubcommand, visible func(perm uint32) bool) (string, []string, bool) {
	token = strings.ToLower(token)
	if token == "" {
		return "", nil, false
	}
	var matches []string
	for _, c := range cands {
		if visible != nil {
			seen := len(c.perms) == 0
			for _, perm := range c.perms {
				if visible(perm) {
					seen = true
					break
				}
			}
			if !seen {
				continue
			}
		}
		if c.name == token {
			return c.name, nil, true
		}
		if strings.HasPrefix(c.name, token) {
			matches = append(matches, c.name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil, true
	}
	return "", matches, false
}

// groupAmbiguousMessage mirrors the LANG_SUBCMD_AMBIGUOUS branch
// (ChatCommand.cpp:270-285): the ambiguous token plus each candidate.
func (s *session) groupAmbiguousMessage(token string, candidates []string) {
	s.sendSysMessage(fmt.Sprintf("Ambiguous subcommand '%s', matches: %s.", token, strings.Join(candidates, ", "))) // LANG_SUBCMD_AMBIGUOUS (7)
}

// handleCmdGroup dispatches the group sub-commands, mirroring
// group_commandscript::GetCommands (cs_group.cpp:35-68).
func (s *session) handleCmdGroup(ctx context.Context, args []string) bool {
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
	// (ChatCommand.cpp:261-288 + FilteredCommandListIterator :207-237):
	// ".group lea" reaches leader, ".group s" is ambiguous (set/summon)
	// only when both are visible, and resolves uniquely to "set" for a GM
	// without GROUP_SUMMON. The "set" entry carries its own permission
	// plus every set-arm permission (IsVisible ORs the container perm
	// with its visible subcommands, ChatCommand.h:200).
	visible := func(perm uint32) bool { return !deny(perm) }
	sub, cands, ok := groupMatchNameVisible(args[0], []groupSubcommand{
		{"set", []uint32{permissionCommandGroupSet, permissionCommandGroupLeader, permissionCommandGroupAssistant, permissionCommandGroupMainTank, permissionCommandGroupMainAssist}},
		{"leader", []uint32{permissionCommandGroupLeader}},
		{"disband", []uint32{permissionCommandGroupDisband}},
		{"remove", []uint32{permissionCommandGroupRemove}},
		{"join", []uint32{permissionCommandGroupJoin}},
		{"list", []uint32{permissionCommandGroupList}},
		{"summon", []uint32{permissionCommandGroupSummon}},
	}, visible)
	if !ok {
		if len(cands) > 1 {
			s.groupAmbiguousMessage(args[0], cands)
			return true
		}
		return false
	}
	switch sub {
	case "set":
		// The C++ groupSetCommandTable is a second nesting level; the
		// Trinity parser prefix-matches at every level, mirrored here.
		// A bare ".group set leader" (no name) is valid in C++: the
		// strtok yields a null nameStr and GetPlayerGroupAndGUIDByName
		// (Chat.cpp:806) falls back to the selected / own player.
		if len(args) < 2 {
			return false
		}
		// ".group set main" is ambiguous (maintank/mainassist), not
		// maintank: the old HasPrefix-first-match silently picked one.
		// The nested level is matched here, not by the framework
		// resolve(), so it filters by visibility the same way.
		setSub, setCands, setOK := groupMatchNameVisible(args[1], []groupSubcommand{
			{"leader", []uint32{permissionCommandGroupLeader}},
			{"assistant", []uint32{permissionCommandGroupAssistant}},
			{"maintank", []uint32{permissionCommandGroupMainTank}},
			{"mainassist", []uint32{permissionCommandGroupMainAssist}},
		}, visible)
		if !setOK {
			if len(setCands) > 1 {
				s.groupAmbiguousMessage(args[1], setCands)
				return true
			}
			return false
		}
		switch setSub {
		case "leader":
			if deny(permissionCommandGroupLeader) {
				return false
			}
			return s.handleGroupLeaderCommand(args[2:])
		case "assistant":
			if deny(permissionCommandGroupAssistant) {
				return false
			}
			return s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagAssistant, "Assistant")
		case "maintank":
			if deny(permissionCommandGroupMainTank) {
				return false
			}
			return s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagMainTank, "Main Tank")
		case "mainassist":
			if deny(permissionCommandGroupMainAssist) {
				return false
			}
			return s.handleGroupFlagCommand(ctx, args[2:], groupMemberFlagMainAssist, "Main Assist")
		}
	case "leader":
		if deny(permissionCommandGroupLeader) {
			return false
		}
		return s.handleGroupLeaderCommand(args[1:])
	case "disband":
		if deny(permissionCommandGroupDisband) {
			return false
		}
		return s.handleGroupDisbandCommand(ctx, args[1:])
	case "remove":
		if deny(permissionCommandGroupRemove) {
			return false
		}
		return s.handleGroupRemoveCommand(ctx, args[1:])
	case "join":
		if deny(permissionCommandGroupJoin) {
			return false
		}
		return s.handleGroupJoinCommand(ctx, args[1:])
	case "list":
		if deny(permissionCommandGroupList) {
			return false
		}
		return s.handleGroupListCommand(ctx, args[1:])
	case "summon":
		if deny(permissionCommandGroupSummon) {
			return false
		}
		return s.handleGroupSummonCommand(ctx, args[1:])
	}
	return true
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
func (s *session) handleGroupLeaderCommand(args []string) bool {
	t, _, ok := s.resolveGroupPlayer(context.Background(), args, false)
	if !ok {
		return true
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return true
	}
	if g.LeaderGUID != t.guid {
		s.server.setGroupLeader(g, t.guid)
	}
	return true
}

// handleGroupFlagCommand mirrors GroupFlagCommand (cs_group.cpp:187): the
// assistant / main-tank / main-assist raid flags toggle for the named player.
func (s *session) handleGroupFlagCommand(ctx context.Context, args []string, flag uint8, what string) bool {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return true
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return true
	}
	if !g.IsRaid {
		s.sendSysMessage(fmt.Sprintf("%s is not in a raid group.", t.name)) // LANG_GROUP_NOT_IN_RAID_GROUP (1185)
		return true
	}
	if flag == groupMemberFlagAssistant && g.LeaderGUID == t.guid {
		s.sendSysMessage("The leader cannot be an assistant.") // LANG_LEADER_CANNOT_BE_ASSISTANT (1187)
		return true
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
		return true
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
	return true
}

// handleGroupDisbandCommand mirrors HandleGroupDisbandCommand
// (cs_group.cpp:246): the named player's whole group is disbanded.
func (s *session) handleGroupDisbandCommand(ctx context.Context, args []string) bool {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return true
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return true
	}
	s.server.disbandGroup(ctx, g, false)
	return true
}

// handleGroupRemoveCommand mirrors HandleGroupRemoveCommand
// (cs_group.cpp:267): the named player is removed from their group.
func (s *session) handleGroupRemoveCommand(ctx context.Context, args []string) bool {
	t, _, ok := s.resolveGroupPlayer(ctx, args, false)
	if !ok {
		return true
	}
	g := s.server.groupOfGUID(t.guid)
	if g == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", t.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return true
	}
	s.server.removeGroupMemberByGUID(ctx, g, t.guid, groupRemoveMethodDefault)
	return true
}

// handleGroupJoinCommand mirrors HandleGroupJoinCommand (cs_group.cpp:288):
// ".group join <playerInGroup> <player>" pulls the second player into the
// first player's group.
func (s *session) handleGroupJoinCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	source, _, ok := s.resolveGroupPlayer(ctx, args[:1], true)
	if !ok {
		return true
	}
	// resolveGroupPlayer always falls back to the selection/self player,
	// exactly like GetPlayerGroupAndGUIDByName (Chat.cpp:806-844), so the
	// online pointer is never nil here; the cached offline guid is unused
	// by the C++ join handler, which adds the fallback player.
	groupSource := s.server.groupOfGUID(source.guid)
	if groupSource == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in a group.", source.name)) // LANG_GROUP_NOT_IN_GROUP (1147)
		return true
	}
	targetArgs := args[1:2]
	if len(args) < 2 { // C++ second strtok yields NULL -> selected/self fallback
		targetArgs = nil
	}
	target, _, ok := s.resolveGroupPlayer(ctx, targetArgs, true)
	if !ok {
		return true
	}
	// C++ (cs_group.cpp:313): any existing group membership — the target's
	// own group OR the source group — rejects with ALREADY_IN_GROUP, not
	// just membership in the source group.
	if s.server.groupOfGUID(target.guid) != nil {
		s.sendSysMessage(fmt.Sprintf("%s is already in a group.", target.name)) // LANG_GROUP_ALREADY_IN_GROUP (1145)
		return true
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
		return true
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
			return true
		}
	}
	s.server.groupsMu.Lock()
	groupSource.Members = append(groupSource.Members, groupMember{GUID: target.guid, Name: target.name, SubGroup: subGroup})
	s.server.groupsMu.Unlock()
	target.online.groupID = groupSource.ID
	// Group::_addMember DB persist (Group.cpp:465): non-BG groups only.
	// A BG raid or LFG group (DBID 0, no groups row) previously wrote
	// a guid-0 orphan row.
	if groupSource.DBID != 0 && !groupSource.isBGGroup() && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP_MEMBER"), groupSource.DBID, target.guid, 0, subGroup, 0)
	}
	s.server.broadcastGroupList(groupSource)                                         // Group::BroadcastGroupUpdate
	s.sendSysMessage(fmt.Sprintf("%s joined %s's group.", target.name, source.name)) // LANG_GROUP_PLAYER_JOINED (1146)
	return true
}

// groupAtoul mirrors atoul (cs_group.cpp:340): the leading decimal digit
// run, wrapping mod 2^32 like the uint32 cast; no leading digit yields 0
// and the name path.
func groupAtoul(s string) uint32 {
	var v uint32
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		v = v*10 + uint32(s[i]-'0')
	}
	return v
}

// handleGroupListCommand mirrors HandleGroupListCommand (cs_group.cpp:335):
// the target may be a bare guid, a name, or (by selection) the GM's target.
func (s *session) handleGroupListCommand(ctx context.Context, args []string) bool {
	var (
		nameTarget string
		guidTarget uint64
		playerSess *session
	)
	if len(args) > 0 && args[0] != "" {
		// atoul takes the leading digits, so "123abc" is guid 123 where
		// the old ParseUint fell through to the name path.
		if guid := groupAtoul(args[0]); guid != 0 {
			// Bare guid form.
			guidTarget = uint64(guid)
			playerSess = s.server.findSessionByGUID(uint64(guid))
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
				// Offline named player: extractPlayerTarget still
				// resolves the guid from the character cache
				// (Chat.cpp:657-710); the group is then found from
				// the DB below.
				if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
					var dbGUID uint64
					if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&dbGUID); err == nil && dbGUID != 0 {
						guidTarget = dbGUID
						nameTarget = name
					}
				}
				if guidTarget == 0 {
					s.sendSysMessage("Player not found.")
					return false
				}
			}
		} else if s.selection != 0 && s.server != nil {
			playerSess = s.server.playerSessionForGUID(s.selection)
		}
		if guidTarget == 0 {
			if playerSess == nil {
				playerSess = s
			}
			if playerSess.player == nil {
				s.sendSysMessage("Player not found.")
				return false
			}
			nameTarget = playerSess.player.Name
			guidTarget = playerSess.playerGUID
		}
	}
	if nameTarget == "" {
		s.sendSysMessage("Player not found.")
		return false
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
		return false
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
			// Zone name via the member's area row's parent (cs_group.cpp:418-426):
			// AreaTable is bridged (Store.Area), so the old "" placeholder goes.
			// A missing area or parent leaves it empty, as in C++ (where a
			// missing lookup keeps the previous member's value; Go resets per
			// member instead of carrying that stale value across rows).
			zoneName = ""
			if s.server != nil && s.server.Data != nil {
				_, areaID := s.server.zoneAndAreaID(p.player.Map, p.player.X, p.player.Y, p.player.Z, 0)
				if areaID != 0 {
					if area, found, err := s.server.Data.Area(areaID); err == nil && found && area.ParentAreaID != 0 {
						if zone, found, err := s.server.Data.Area(area.ParentAreaID); err == nil && found {
							zoneName = zone.Name
						}
					}
				}
			}
			if p.player.ExtraFlags&playerExtraGMOn == 0 {
				phase = p.currentPlayerPhaseMask()
			} else {
				phase = ^uint32(0) // C++ assigns -1 to the uint32 phase
			}
		}
		s.sendSysMessage(fmt.Sprintf("%s: %s, Zone: %s, Phase: %d, Guid: %d, Flags: %s, Roles: %s",
			slot.Name, onlineState, zoneName, phase, slot.GUID, flags, lfgRolesString(slot.Roles))) // LANG_GROUP_PLAYER_NAME_GUID (1150)
	}
	return true
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

// lfgRolesString mirrors lfg::GetRolesString (LFG.cpp:41-73): Tank/Healer/
// Damage/Leader in that order, and LANG_LFG_ROLE_NONE ("None") when no
// role bits are set. The English role words are inlined; the C++ resolves
// them through trinity_string. (Second-pass correction: the tick-28 port
// dropped the Leader leg and the None fallback by reading the masked
// LFGQueue.cpp call sites instead of the function itself; both the
// PrintPlayerInfo line and the cs_group.cpp:446 member line call the
// function unmasked.)
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
func (s *session) handleGroupSummonCommand(ctx context.Context, args []string) bool {
	// extractPlayerTarget: name, selection, then self.
	var target *session
	if len(args) > 0 && args[0] != "" {
		target = s.sessionForPlayerName(normalizePlayerName(args[0]))
		if target == nil {
			s.sendSysMessage("Player not found.")
			return true
		}
	} else if s.selection != 0 && s.server != nil {
		target = s.server.playerSessionForGUID(s.selection)
	}
	if target == nil {
		target = s
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: target, guid: target.playerGUID, name: target.player.Name}) {
		s.sendSysMessage("Your security level is too low.") // LANG_YOURS_SECURITY_IS_LOW (Chat.cpp:96-99)
		return true
	}
	group := s.server.groupOfGUID(target.playerGUID)
	if group == nil {
		// C++ prints LANG_NOT_IN_GROUP and returns false with no
		// SetSentErrorMessage (cs_group.cpp:86-87): silent false.
		return false
	}
	srv := s.server
	gmMapID := s.player.Map
	gmInstance := s.player.InstanceID
	toInstance := instanceableMapID(srv, gmMapID)
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
			s.sendSysMessage("Your security level is too low.") // LANG_YOURS_SECURITY_IS_LOW (Chat.cpp:96-99)
			continue
		}
		if player.isBeingTeleported() {
			s.sendSysMessage(fmt.Sprintf(miscIsTeleported, miscPlayerLink(player.player.Name))) // LANG_IS_TELEPORTED (102)
			continue
		}
		if toInstance {
			playerMapID := player.player.Map
			instanceable := false
			if entry, found, err := srv.Data.Map(playerMapID); err == nil && found {
				instanceable = entry.Instanceable()
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
		// Stop flight if needed, else snapshot the recall position
		// (cs_group.cpp:150, same non-flight rule as goDoTeleport).
		if player.isInFlight() {
			player.finishTaxiFlight()
		} else {
			player.saveRecallPosition()
		}
		// WorldObject::GetClosePoint (Object.cpp:3307): the GM's reach plus
		// the member's reach along the GM's orientation, at the GM's Z.
		// C++ group summon sets no phase mask, unlike .summon.
		cx, cy := miscClosePoint(s.player.X, s.player.Y, s.player.Orientation, miscPlayerCombatReach(s.player), miscPlayerCombatReach(player.player), 0)
		player.teleportTo(gmMapID, cx, cy, s.player.Z, player.player.Orientation)
	}
	return true
}
