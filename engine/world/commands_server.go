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
// THIRTY-FIFTH of 39 Commands groups (cs_script_loader.cpp decl 54 / call
// 99; call order send(98) -> server(99)). Trinity checks permission only on
// the invoker leaf node (ChatCommand.cpp:487), so each arm gates exactly its
// own C++ permission (RBAC.h:586-604, 706-707, 738; the root permission 718
// covers the bare ".server"). This replaces the earlier partial
// handleCmdServer in commands.go (info/motd/restart/shutdown, ungated).
//
// Native arms:
//   - `info`: online/max player counts, uptime from applicationStartTime.
//   - `motd` / `set motd`: read/write Config.Motd.
//   - `set closed on|off`: the server closed atomic.Bool (admission already
//     honors it).
//   - `plimit [n]`: the playerLimit amount gate (admission already honors
//     it); the security-level names (player/moderator/gamemaster/
//     administrator/reset/negative) have no Go bridge — admission checks a
//     head count, not an account security level.
//   - `exit`: Server.Stop().
//   - `corpses`: force the expired-corpse sweep (World::RemoveOldCorpses,
//     cs_server.cpp:120-123 — the command forces the WUPDATE_CORPSES timer;
//     Go runs the sweep synchronously and reports the count).
//   - `debug`: partial — Go runtime/build info, world DB version, listen
//     port; the C++ SSL/Boost/MySQL/CMake/VMAP/MMAP/DBC-locale legs have no
//     Go equivalent.
//
// Documented-blocked arms (no bridge, honest message instead of a stub):
//   - `shutdown` / `restart` / `idleshutdown` / `idlerestart` (and their
//     cancel/force arms): no shutdown-timer machinery exists in the Go
//     runtime (no delayed-shutdown broadcast, no cancel).
//   - `set loglevel`: the slog logger's level is fixed at handler creation.
//
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// serverShutdownBlockedCtx answers the timed shutdown/restart family.
func (s *session) serverShutdownBlockedCtx(ctx context.Context, perm uint32, what string) {
	if s.miscDeny(ctx, perm) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Timed server %s is not supported by this server (no shutdown-timer machinery).", what))
}

// handleCmdServer dispatches the "server" root (cs_server.cpp:99-110).
func (s *session) handleCmdServer(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandServer) {
			return
		}
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
		v := cAtoi(args[0])
		if v >= 0 {
			s.server.playerLimit = uint32(v)
		} else {
			s.sendSysMessage("Security-level player limits are not supported by this server (head-count limit only).")
		}
	}
	s.sendSysMessage(fmt.Sprintf("Player limits: amount %d.", s.server.playerLimit))
}

// handleServerSet dispatches the "server set" sub-table
// (cs_server.cpp:92-94): loglevel, motd, closed.
func (s *session) handleServerSet(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandServerSet) {
			return
		}
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
	switch strings.ToLower(args[0]) {
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
