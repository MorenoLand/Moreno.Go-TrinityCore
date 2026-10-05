package world

import (
	"context"
	"fmt"
	"strconv"
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
func (s *session) handleCmdLFG(ctx context.Context, args []string) {
	const syntax = "Syntax: .lfg player|group|queue|clean|options"
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
	if s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		s.sendSysMessage("LFG is unavailable.")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("player", sub):
		if deny(permissionCommandLfgPlayer) {
			return
		}
		s.handleLfgPlayerInfoCommand(ctx, strings.Join(rest, " "))
	case strings.HasPrefix("group", sub):
		if deny(permissionCommandLfgGroup) {
			return
		}
		s.handleLfgGroupInfoCommand(ctx, strings.Join(rest, " "))
	case strings.HasPrefix("queue", sub):
		if deny(permissionCommandLfgQueue) {
			return
		}
		s.handleLfgQueueInfoCommand(strings.Join(rest, " "))
	case strings.HasPrefix("clean", sub):
		if deny(permissionCommandLfgClean) {
			return
		}
		s.handleLfgCleanCommand()
	case strings.HasPrefix("options", sub):
		if deny(permissionCommandLfgOptions) {
			return
		}
		s.handleLfgOptionsCommand(strings.Join(rest, " "))
	default:
		s.sendSysMessage(syntax)
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
// optional name, else selection, else self; an unresolvable target reports
// LANG_PLAYER_NOT_FOUND (499) per tree convention (the C++
// PlayerIdentifier::FromTargetOrSelf silently fails for offline players).
func (s *session) handleLfgPlayerInfoCommand(ctx context.Context, name string) {
	var target *session
	if name != "" {
		target = s.sessionForPlayerName(normalizePlayerName(name))
		if target == nil {
			s.sendSysMessage("Player not found.")
			return
		}
	} else {
		if ts := s.lookupSelectedPlayer(); ts != nil {
			target = ts
		} else {
			target = s
		}
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	s.lfgPlayerInfoLine(target)
}

// handleLfgGroupInfoCommand mirrors HandleLfgGroupInfoCommand (cs_lfg.cpp:83-117):
// LANG_LFG_GROUP_INFO (9981) from the group's own LFG state (the Go tree keeps
// IsLFG/LFGState/LFGDungeonID on groupState rather than in a separate LFG
// group store), then one PrintPlayerInfo line per online member, else
// "%s is offline.". No group -> LANG_LFG_NOT_IN_GROUP (9982). The group is
// found via the member's online session, else the CHAR_SEL_GROUP_MEMBER
// character-DB fallback used by the group port.
func (s *session) handleLfgGroupInfoCommand(ctx context.Context, name string) {
	var playerSess *session
	nameTarget := ""
	guidTarget := uint64(0)
	if name != "" {
		playerSess = s.sessionForPlayerName(normalizePlayerName(name))
		if playerSess == nil {
			s.sendSysMessage("Player not found.")
			return
		}
		nameTarget = playerSess.player.Name
		guidTarget = playerSess.playerGUID
	} else {
		playerSess = s.lookupSelectedPlayer()
		if playerSess == nil {
			playerSess = s
		}
		nameTarget = playerSess.player.Name
		guidTarget = playerSess.playerGUID
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
		s.sendSysMessage(fmt.Sprintf("%s is not in group", nameTarget)) // LANG_LFG_NOT_IN_GROUP (9982)
		return
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
}

// handleLfgQueueInfoCommand mirrors HandleLfgQueueInfoCommand (cs_lfg.cpp:140-145):
// DumpQueueInfo with detail when any text follows (C++ Tail `full`).
func (s *session) handleLfgQueueInfoCommand(full string) {
	s.sendSysMessage(s.server.Features.LFG.DumpQueueInfo(strings.TrimSpace(full) != ""))
}

// handleLfgCleanCommand mirrors HandleLfgCleanCommand (cs_lfg.cpp:147-152).
func (s *session) handleLfgCleanCommand() {
	s.sendSysMessage("LFG tables cleaned.") // LANG_LFG_CLEAN (9983)
	s.server.Features.LFG.Clean()
}

// handleLfgOptionsCommand mirrors HandleLfgOptionsCommand (cs_lfg.cpp:133-145):
// an argument sets the options mask (C++ Optional<uint32> consumed via
// StringTo<uint32> (ChatCommandArgs.h:62) — strict base-10 parse that fails
// the command on garbage, so a bad value prints the syntax line == the
// tree's LANG 1502 convention), then the current value is always reported:
// LANG_LFG_OPTIONS_CHANGED (9985) / LANG_LFG_OPTIONS (9984).
func (s *session) handleLfgOptionsCommand(arg string) {
	const syntax = "Syntax: .lfg options [options]"
	if trimmed := strings.TrimSpace(arg); trimmed != "" {
		options, err := strconv.ParseUint(trimmed, 10, 32)
		if err != nil {
			s.sendSysMessage(syntax)
			return
		}
		s.server.Features.LFG.SetOptions(uint32(options))
		s.sendSysMessage("LFG options changed.") // LANG_LFG_OPTIONS_CHANGED (9985)
	}
	s.sendSysMessage(fmt.Sprintf("LFG options: %d", s.server.Features.LFG.GetOptions())) // LANG_LFG_OPTIONS (9984)
}
