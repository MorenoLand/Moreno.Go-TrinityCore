package world

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// server command port: server_commandscript (cs_server.cpp), the "server"
// root with 11 arms (corpses, debug, exit, idlerestart, idleshutdown, info,
// motd, plimit, restart, shutdown, set[loglevel|motd|closed]).
// THIRTY-SIXTH of 40 Commands groups (cs_script_loader.cpp decl 54 / call
// 99; call order send(98) -> server(99)). Trinity checks permission only on
// the invoker leaf node (ChatCommand.cpp:487), so each arm gates exactly its
// own C++ permission (RBAC.h:586-604, 706-707, 738). The root permission 718
// is DEAD in C++ (the deprecated 6-arg nullptr+subtable overload,
// ChatCommand.h:260-263, drops RBACPermissions), so bare ".server" prints
// the syntax line ungated (same pattern as the reset/reload/rbac/pet/quest/
// modify/mmap ports); the same holds for the "set" node (729) on bare
// ".server set". The idlerestart/idleshutdown/restart/shutdown node perms
// (721/723/727/734) are dead the same way, but the "" child entries merge
// their invokers into those nodes (ChatCommand.cpp LoadCommandsIntoMap),
// so the same values gate the bare ".server restart <args>" paths live.
// This replaces the earlier partial handleCmdServer in commands.go
// (info/motd/restart/shutdown, ungated).
//
// Native arms:
//   - `info`: online player count, uptime from applicationStartTime (the
//     max-count, session-stats, update-diff and shutdown-timeleft legs have
//     no Go bridge).
//   - `motd` / `set motd`: read/write Config.Motd.
//   - `set closed on|off`: the server closed atomic.Bool (admission already
//     honors it); the value token is compared case-sensitively == C++
//     strncmp (ChatCommand-level name matching stays case-insensitive).
//   - `plimit [n]`: the amount limit is native (admission already honors
//     it); "reset" (any case-sensitive prefix == C++ strncmp order)
//     restores Config.PlayerLimit; the security-level names and negative
//     values have no Go bridge — admission checks a head count, not an
//     account security level — and leave the amount limit untouched == C++.
//   - `exit`: Server.Stop().
//   - `corpses`: force the expired-corpse sweep (World::RemoveOldCorpses,
//     cs_server.cpp:120-123 — the command forces the WUPDATE_CORPSES timer;
//     Go runs the sweep synchronously and reports the count, an extra line
//     over C++'s silent true).
//   - `debug`: partial — Go runtime/build info, world DB version, listen
//     port; the C++ SSL/Boost/MySQL/CMake/VMAP/MMAP/DBC-locale legs have no
//     Go equivalent.
//
// Documented-blocked arms (no bridge, honest message instead of a stub):
//   - `shutdown` / `restart` / `idleshutdown` / `idlerestart` (and their
//     cancel/force arms): no shutdown-timer machinery exists in the Go
//     runtime (no delayed-shutdown broadcast, no cancel). The force arms
//     gate only their own leaf perm (839/840) == Trinity's leaf-only check.
//   - `set loglevel`: the slog logger's level is fixed at handler creation.
//
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// serverShutdownBlockedMsg is the honest stand-in for the timed
// shutdown/restart family: no shutdown-timer machinery exists in Go.
func serverShutdownBlockedMsg(s *session, what string) {
	s.sendSysMessage(fmt.Sprintf("Timed server %s is not supported by this server (no shutdown-timer machinery).", what))
}

// serverShutdownBlockedCtx answers the timed shutdown/restart family,
// gating the ""-arm leaf permission (== the invoker perm Trinity checks).
func (s *session) serverShutdownBlockedCtx(ctx context.Context, perm uint32, what string) {
	if s.miscDeny(ctx, perm) {
		return
	}
	serverShutdownBlockedMsg(s, what)
}

// handleCmdServer dispatches the "server" root (cs_server.cpp:99-110).
func (s *session) handleCmdServer(ctx context.Context, args []string) {
	if len(args) == 0 {
		// Bare ".server": the root perm 718 is dead in C++ (deprecated
		// 6-arg nullptr+subtable overload), so this is ungated.
		s.sendSysMessage("Syntax: .server corpses|debug|exit|idlerestart|idleshutdown|info|motd|plimit|restart|shutdown|set")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("corpses", sub):
		s.handleServerCorpses(ctx)
	case strings.HasPrefix("debug", sub):
		s.handleServerDebug(ctx)
	case strings.HasPrefix("exit", sub):
		s.handleServerExit(ctx)
	case strings.HasPrefix("idlerestart", sub):
		s.handleServerIdleRestart(ctx, rest)
	case strings.HasPrefix("idleshutdown", sub):
		s.handleServerIdleShutdown(ctx, rest)
	case strings.HasPrefix("info", sub):
		s.handleServerInfo(ctx)
	case strings.HasPrefix("motd", sub):
		s.handleServerMotd(ctx)
	case strings.HasPrefix("plimit", sub):
		s.handleServerPLimit(ctx, rest)
	case strings.HasPrefix("restart", sub):
		s.handleServerRestart(ctx, rest)
	case strings.HasPrefix("shutdown", sub):
		s.handleServerShutdown(ctx, rest)
	case strings.HasPrefix("set", sub):
		s.handleServerSet(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .server corpses|debug|exit|idlerestart|idleshutdown|info|motd|plimit|restart|shutdown|set")
	}
}

// handleServerCorpses mirrors HandleServerCorpsesCommand (cs_server.cpp:121):
// force the expired-corpse sweep now (World::RemoveOldCorpses) and report
// the count. The automatic sweep also runs from the world tick every 20
// minutes (WUPDATE_CORPSES, World.cpp:2106).
func (s *session) handleServerCorpses(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerCorpses) {
		return
	}
	if s.server == nil {
		return
	}
	s.server.lastCorpseExpiry = time.Now()
	expired := s.server.expireOldCorpses(ctx)
	s.sendSysMessage(fmt.Sprintf("Expired %d old corpse(s).", expired))
}

// handleServerDebug mirrors HandleServerDebugCommand (cs_server.cpp:126):
// partial — Go runtime/build info, world DB version, listen port.
func (s *session) handleServerDebug(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerDebug) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("%s", "Moreno.Go-TrinityCore (WotLK 3.3.5a 12340)"))
	s.sendSysMessage(fmt.Sprintf("Using Go version: %s", runtime.Version()))
	var dbVersion string
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT version FROM version LIMIT 1").Scan(&dbVersion)
		if dbVersion == "" {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE name = 'version'").Scan(&dbVersion)
		}
	}
	if dbVersion != "" {
		s.sendSysMessage(fmt.Sprintf("Using World DB: %s", dbVersion))
	}
	s.sendSysMessage(fmt.Sprintf("Worldserver listening connections on port %d", s.server.Config.WorldServerPort))
}

// handleServerExit mirrors HandleServerExitCommand (cs_server.cpp:383):
// native via Server.Stop().
func (s *session) handleServerExit(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerExit) {
		return
	}
	s.sendSysMessage("Exiting.") // LANG_COMMAND_EXIT 1000
	s.server.Stop()
}

// handleServerIdleRestart mirrors HandleServerIdleRestartCommand
// (cs_server.cpp:373): documented-blocked.
func (s *session) handleServerIdleRestart(ctx context.Context, args []string) {
	if len(args) > 0 && strings.ToLower(args[0]) == "cancel" {
		if s.miscDeny(ctx, permissionCommandServerIdlerestartCancel) {
			return
		}
		s.sendSysMessage("There is no shutdown timer to cancel.")
		return
	}
	s.serverShutdownBlockedCtx(ctx, permissionCommandServerIdlerestart, "idle restart")
}

// handleServerIdleShutdown mirrors HandleServerIdleShutDownCommand
// (cs_server.cpp:368): documented-blocked.
func (s *session) handleServerIdleShutdown(ctx context.Context, args []string) {
	if len(args) > 0 && strings.ToLower(args[0]) == "cancel" {
		if s.miscDeny(ctx, permissionCommandServerIdleshutdownCancel) {
			return
		}
		s.sendSysMessage("There is no shutdown timer to cancel.")
		return
	}
	s.serverShutdownBlockedCtx(ctx, permissionCommandServerIdleshutdown, "idle shutdown")
}

// handleServerRestart mirrors HandleServerRestartCommand (cs_server.cpp:358):
// documented-blocked (the force arm shares the fate: no timer to force).
func (s *session) handleServerRestart(ctx context.Context, args []string) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "cancel":
			if s.miscDeny(ctx, permissionCommandServerRestartCancel) {
				return
			}
			s.sendSysMessage("There is no shutdown timer to cancel.")
			return
		case "force":
			if s.miscDeny(ctx, permissionCommandServerRestartForce) {
				return
			}
			// Leaf-only permission check (ChatCommand.cpp:487): the force
			// arm's own perm is the only gate; the blocked message itself
			// is ungated.
			serverShutdownBlockedMsg(s, "restart")
			return
		}
	}
	s.serverShutdownBlockedCtx(ctx, permissionCommandServerRestart, "restart")
}

// handleServerShutdown mirrors HandleServerShutDownCommand (cs_server.cpp:353):
// documented-blocked (the force arm shares the fate: no timer to force).
func (s *session) handleServerShutdown(ctx context.Context, args []string) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "cancel":
			if s.miscDeny(ctx, permissionCommandServerShutdownCancel) {
				return
			}
			s.sendSysMessage("There is no shutdown timer to cancel.")
			return
		case "force":
			if s.miscDeny(ctx, permissionCommandServerShutdownForce) {
				return
			}
			// Leaf-only permission check (ChatCommand.cpp:487): the force
			// arm's own perm is the only gate; the blocked message itself
			// is ungated.
			serverShutdownBlockedMsg(s, "shutdown")
			return
		}
	}
	s.serverShutdownBlockedCtx(ctx, permissionCommandServerShutdown, "shutdown")
}

// handleServerInfo mirrors HandleServerInfoCommand (cs_server.cpp:240).
func (s *session) handleServerInfo(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerInfo) {
		return
	}
	s.server.sessionsMu.RLock()
	online := len(s.server.sessions)
	s.server.sessionsMu.RUnlock()
	uptime := time.Since(applicationStartTime).Truncate(time.Second).String()
	s.sendSysMessage("Moreno.Go-TrinityCore (WotLK 3.3.5a 12340)")
	s.sendSysMessage(fmt.Sprintf("Connected players: %d.", online)) // LANG_CONNECTED_PLAYERS 60
	s.sendSysMessage(fmt.Sprintf("Server uptime: %s.", uptime))     // LANG_UPTIME 13
}

// handleServerMotd mirrors HandleServerMotdCommand (cs_server.cpp:279).
func (s *session) handleServerMotd(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerMotd) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("MOTD: %s", s.server.Config.Motd)) // LANG_MOTD_CURRENT 56
}

// handleServerPLimit mirrors HandleServerPLimitCommand (cs_server.cpp:285):
// the amount limit is native; the security-level names have no Go bridge
// (admission checks a head count, not an account security level).
func (s *session) handleServerPLimit(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandServerPlimit) {
		return
	}
	if len(args) > 0 {
		arg := args[0]
		// C++ checks the names with strncmp(paramStr, name, strlen(paramStr))
		// — a case-sensitive prefix match — before falling through to
		// atoi (cs_server.cpp:295-315). Mirror that order here.
		switch {
		case isStrncmpPrefix(arg, "player"), isStrncmpPrefix(arg, "moderator"),
			isStrncmpPrefix(arg, "gamemaster"), isStrncmpPrefix(arg, "administrator"):
			// Security-level limits have no Go bridge (admission checks a
			// head count, not an account security level); the amount limit
			// is left untouched == C++.
			s.sendSysMessage("Security-level player limits are not supported by this server (head-count limit only).")
		case isStrncmpPrefix(arg, "reset"):
			s.server.playerLimit = s.server.Config.PlayerLimit
		default:
			v := cAtoi(arg)
			if v >= 0 {
				s.server.playerLimit = uint32(v)
			} else {
				s.sendSysMessage("Security-level player limits are not supported by this server (head-count limit only).")
			}
		}
	}
	s.sendSysMessage(fmt.Sprintf("Player limits: amount %d.", s.server.playerLimit))
}

// isStrncmpPrefix reports whether arg would satisfy
// strncmp(arg, name, strlen(arg)) == 0: arg is a non-empty case-sensitive
// prefix of name.
func isStrncmpPrefix(arg, name string) bool {
	return arg != "" && strings.HasPrefix(name, arg)
}

// handleServerSet dispatches the "server set" sub-table
// (cs_server.cpp:92-94): loglevel, motd, closed.
func (s *session) handleServerSet(ctx context.Context, args []string) {
	if len(args) == 0 {
		// Bare ".server set": the "set" node perm 729 is dead in C++ (the
		// deprecated 6-arg nullptr+subtable overload drops it and there is
		// no "" child), so this is ungated.
		s.sendSysMessage("Syntax: .server set loglevel|motd|closed")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("loglevel", sub):
		s.handleServerSetLogLevel(ctx)
	case strings.HasPrefix("motd", sub):
		s.handleServerSetMotd(ctx, rest)
	case strings.HasPrefix("closed", sub):
		s.handleServerSetClosed(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .server set loglevel|motd|closed")
	}
}

// handleServerSetLogLevel mirrors HandleServerSetLogLevelCommand
// (cs_server.cpp:431): documented-blocked — the slog logger's level is fixed
// at handler creation.
func (s *session) handleServerSetLogLevel(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandServerSetLoglevel) {
		return
	}
	s.sendSysMessage("Changing the log level at runtime is not supported by this server.")
}

// handleServerSetMotd mirrors HandleServerSetMotdCommand (cs_server.cpp:393).
func (s *session) handleServerSetMotd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandServerSetMotd) {
		return
	}
	motd := strings.Join(args, " ")
	s.server.Config.Motd = motd
	s.sendSysMessage(fmt.Sprintf("New MOTD: %s", motd)) // LANG_MOTD_NEW 1101
}

// handleServerSetClosed mirrors HandleServerSetClosedCommand
// (cs_server.cpp:409).
func (s *session) handleServerSetClosed(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandServerSetClosed) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Use on/off.") // LANG_USE_BOL 259
		return
	}
	// C++ compares with strncmp(args, "on", 3) / strncmp(args, "off", 4):
	// case-sensitive, and the length bound means only the exact tokens
	// "on"/"off" match (a third/fourth char already differs from '\0').
	switch args[0] {
	case "on":
		s.server.closed.Store(true)
		s.sendSysMessage("World closed.") // LANG_WORLD_CLOSED 7523
	case "off":
		s.server.closed.Store(false)
		s.sendSysMessage("World opened.") // LANG_WORLD_OPENED 7524
	default:
		s.sendSysMessage("Use on/off.") // LANG_USE_BOL 259
	}
}
