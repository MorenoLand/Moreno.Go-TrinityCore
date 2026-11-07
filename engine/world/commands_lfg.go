package world

import (
	"context"
	"fmt"
	"strings"
)

// lfg command ports lfg_commandscript (cs_lfg.cpp), the TWENTY-THIRD Commands
// group in loader call order (AddSC_lookup_commandscript() is call 85, this is
// call 86). All 5 arms bottom out in the Go LFGManager (lfg.go): `player` and
// `group` report the queue entry's state/dungeons/roles/comment; `queue`
// dumps the queue; `clean` drops the queue; `options` reads/writes the
// options mask. Console-vs-chat LANG branches are moot (Go commands are
// always sessioned), and LANG texts are inlined from TDB enUS recall (no
// in-tree trinity_string seed), per tree convention.
//
// Fidelity notes (documented gaps, not stubs): the C++ groups queues per
// dungeon group and keeps a compatible-map used by the full queue dump; the
// Go manager holds one flat player queue, so the dump's group and compatible
// sections report 0. The C++ group info reads state/dungeon from the LFG
// manager's group data; the Go tree keeps the LFG state on the group struct
// itself (groupState.IsLFG/LFGState/LFGDungeonID), so the same fields are
// rendered from there.

// handleCmdLFG ports the lfg command table (cs_lfg.cpp:51-66): 5 arms,
// player and group are Console::No in the C++ (moot here), Trinity per-level
// prefix matching, each arm gated on its own RBAC permission
// (ChatCommand.cpp:487 checks the invoker/leaf node only).
func (s *session) handleCmdLFG(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The C++ framework resolves the leaf and its RBAC visibility before
	// the handler body runs (ChatCommand.cpp:207-237,481-488), so an
	// unknown subcommand or a denied leaf answers silent false even when
	// the Go-only gates below would fire first (R13 D2/D3).
	var perm uint32
	known := false
	for _, arm := range []struct {
		name string
		perm uint32
	}{
		{"player", permissionCommandLfgPlayer},
		{"group", permissionCommandLfgGroup},
		{"queue", permissionCommandLfgQueue},
		{"clean", permissionCommandLfgClean},
		{"options", permissionCommandLfgOptions},
	} {
		if strings.HasPrefix(arm.name, sub) {
			perm, known = arm.perm, true
			break
		}
	}
	if !known {
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		s.sendSysMessage("LFG is unavailable.")
		return true
	}
	switch {
	case strings.HasPrefix("player", sub):
		return s.handleLfgPlayerInfoCommand(ctx, rest)
	case strings.HasPrefix("group", sub):
		return s.handleLfgGroupInfoCommand(ctx, rest)
	case strings.HasPrefix("queue", sub):
		return s.handleLfgQueueInfoCommand(s.commandTailAfter(args, 1))
	case strings.HasPrefix("clean", sub):
		if len(rest) > 0 {
			return false
		}
		return s.handleLfgCleanCommand()
	case strings.HasPrefix("options", sub):
		return s.handleLfgOptionsCommand(s.commandTailAfter(args, 1))
	default:
		return false
	}
}

// lfgPlayerInfoLine mirrors PrintPlayerInfo (cs_lfg.cpp:34-46): LANG_LFG_PLAYER_INFO
// (9980) rendered from the player's queue entry; a player not in the queue
// reports state "None" with no dungeons/roles/comment, exactly like the C++
// empty LfgQueueData.
func (s *session) lfgPlayerInfoLine(target *session) {
	entry, _ := s.server.Features.LFG.Status(target.playerGUID)
	s.sendSysMessage(fmt.Sprintf("Player info: %s, State: %s, Selected dungeons: %d, Dungeons: %s, Roles: %s, Comment: %s",
		target.player.Name, lfgStateString(entry.State), len(entry.Dungeons),
		concatenateLFGDungeons(entry.Dungeons), lfgRolesString(entry.Roles), entry.Comment))
}

// handleLfgPlayerInfoCommand mirrors HandleLfgPlayerInfoCommand (cs_lfg.cpp:68-81):
// the target is the framework-parsed Optional<PlayerIdentifier> (name, |Hplayer
// link, or numeric low GUID, offline players resolved through the character
// DB via resolveCharacterTarget); with no argument it is the selected
// connected player, else self (PlayerIdentifier::FromTargetOrSelf,
// ChatCommandTags.h:189). A resolved-but-offline player makes the C++
// handler return false (GetConnectedPlayer() is null), so the Go handler
// returns false silently (R13); wholly unknown names keep the
// "Player not found." message via resolveCharacterTarget (MESSAGE + TRUE,
// per the tree convention).
func (s *session) handleLfgPlayerInfoCommand(ctx context.Context, args []string) bool {
	t, rest, ok := s.resolveCharacterTarget(ctx, args)
	if !ok {
		return true
	}
	if len(rest) > 0 {
		return false
	}
	if t.online == nil {
		return false
	}
	s.lfgPlayerInfoLine(t.online)
	return true
}

// handleLfgGroupInfoCommand mirrors HandleLfgGroupInfoCommand (cs_lfg.cpp:83-117):
// LANG_LFG_GROUP_INFO (9981) from the group's own LFG state (the Go tree keeps
// IsLFG/LFGState/LFGDungeonID on groupState rather than in a separate LFG
// group store), then one PrintPlayerInfo line per online member, else
// "%s is offline.". No group -> LANG_LFG_NOT_IN_GROUP (9982). The target is
// the framework-parsed Optional<PlayerIdentifier> via resolveCharacterTarget
// (name, |Hplayer link, or numeric low GUID); the group is found via the
// member's online session, else the CHAR_SEL_GROUP_MEMBER character-DB
// fallback used by the group port. C++ only takes the CHAR_SEL_GROUP_MEMBER
// branch when the named player is NOT connected (cs_lfg.cpp:96-104
// else-branch); a connected player with no in-memory group reports
// NOT_IN_GROUP, it never falls through to a possibly stale group_member row.
func (s *session) handleLfgGroupInfoCommand(ctx context.Context, args []string) bool {
	t, rest, ok := s.resolveCharacterTarget(ctx, args)
	if !ok {
		return true
	}
	if len(rest) > 0 {
		return false
	}

	var groupTarget *groupState
	if t.online != nil && t.online.groupID != 0 {
		groupTarget = s.server.findGroupByID(t.online.groupID)
	}
	if groupTarget == nil && t.online == nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		// CHAR_SEL_GROUP_MEMBER: find the group through the character DB.
		var dbid uint64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM group_member WHERE memberGuid = ? LIMIT 1", t.guid).Scan(&dbid); err == nil && dbid != 0 {
			groupTarget = s.server.findGroupByDBID(dbid)
		}
	}
	if groupTarget == nil {
		s.sendSysMessage(fmt.Sprintf("%s is not in group", t.name)) // LANG_LFG_NOT_IN_GROUP (9982)
		return true
	}

	s.server.groupsMu.RLock()
	isLFG := uint8(0)
	if groupTarget.IsLFG {
		isLFG = 1
	}
	state := groupTarget.LFGState
	dungeon := groupTarget.LFGDungeonID
	members := make([]groupMember, len(groupTarget.Members))
	copy(members, groupTarget.Members)
	s.server.groupsMu.RUnlock()

	s.sendSysMessage(fmt.Sprintf("Group info: Group is LFG: %d, State: %s, Dungeon: %d",
		isLFG, lfgStateString(state), dungeon)) // LANG_LFG_GROUP_INFO (9981)
	for _, slot := range members {
		if ms := s.server.findSessionByGUID(slot.GUID); ms != nil && ms.player != nil {
			s.lfgPlayerInfoLine(ms)
		} else {
			s.sendSysMessage(fmt.Sprintf("%s is offline.", slot.Name))
		}
	}
	return true
}

// handleLfgQueueInfoCommand mirrors HandleLfgQueueInfoCommand (cs_lfg.cpp:140-145):
// DumpQueueInfo with detail when any text follows (C++ Tail `full`).
func (s *session) handleLfgQueueInfoCommand(full string) bool {
	s.sendSysMessage(s.server.Features.LFG.DumpQueueInfo(strings.TrimSpace(full) != ""))
	return true
}

// handleLfgCleanCommand mirrors HandleLfgCleanCommand (cs_lfg.cpp:147-152).
func (s *session) handleLfgCleanCommand() bool {
	s.sendSysMessage("LFG tables cleaned.") // LANG_LFG_CLEAN (9983)
	s.server.Features.LFG.Clean()
	return true
}

// handleLfgOptionsCommand mirrors HandleLfgOptionsCommand (cs_lfg.cpp:133-145):
// an argument sets the options mask (C++ Optional<uint32> consumed via
// StringTo<uint32> base 0 (ChatCommandArgs.h:62, StringConvert.h) — 0x/0b
// prefixes accepted, trailing garbage fails the whole-string rule, so a
// bad value prints the syntax line, the tree's LANG 1502 convention), then
// the current value is always reported: LANG_LFG_OPTIONS_CHANGED (9985) /
// LANG_LFG_OPTIONS (9984).
func (s *session) handleLfgOptionsCommand(arg string) bool {
	const syntax = "Syntax: .lfg options [options]"
	if trimmed := strings.TrimSpace(arg); trimmed != "" {
		if strings.ContainsAny(trimmed, " \t") {
			return false
		}
		options, ok := parseCommandUint32(trimmed)
		if !ok {
			s.sendSysMessage(syntax)
			return true
		}
		s.server.Features.LFG.SetOptions(options)
		s.sendSysMessage("LFG options changed.") // LANG_LFG_OPTIONS_CHANGED (9985)
	}
	s.sendSysMessage(fmt.Sprintf("LFG options: %d", s.server.Features.LFG.GetOptions())) // LANG_LFG_OPTIONS (9984)
	return true
}
