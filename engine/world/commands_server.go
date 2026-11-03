package world

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
//   - `shutdown` / `restart` / `idleshutdown` / `idlerestart` (and their
//     cancel/force arms): the shutdown timer (shutdown.go, mirroring
//     World::ShutdownServ): delay in seconds or a d/h/m/s time string,
//     optional exit code (0-125) and reason, the 30s force-threshold
//     override while other players are connected, countdown broadcasts
//     on the ShutdownMsg cadence, idle timers held while sessions are
//     live, expiry stops the server (a restart timer ends in Stop() too
//     — Go cannot relaunch itself, see shutdown.go). The force arms
//     gate only their own leaf perm (839/840) == Trinity's leaf-only check.
//
// Documented-blocked arms (no bridge, honest message instead of a stub):
//   - `set loglevel`: the slog logger's level is fixed at handler creation.
//
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// serverShutdownTimed mirrors ShutdownServer (cs_server.cpp:457-521)
// for one node: args are "<delay> [#exit_code] [reason]". Empty or
// unparseable args print the syntax line (C++ returns false, which
// ChatHandler answers with syntax). The force-threshold override
// (World.cpp:1027, cs_server.cpp:511-516) raises a sub-30s delay to 30s
// while other players are connected, unless the force mask is set.
func (s *session) serverShutdownTimed(ctx context.Context, args []string, mask uint32, defaultExitCode uint8, what string) {
	if len(args) == 0 {
		s.sendSysMessage(fmt.Sprintf("Syntax: .server %s <delay> [#exit_code] [reason]", what))
		return
	}
	delay, ok := parseShutdownDelay(args[0])
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Syntax: .server %s <delay> [#exit_code] [reason]", what))
		return
	}
	exitCode, reason, ok := parseShutdownArgs(args[1:], defaultExitCode)
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Syntax: .server %s <delay> [#exit_code] [reason]", what))
		return
	}
	if delay < forceShutdownThresholdSecs && mask&shutdownMaskForce == 0 && !s.serverIsOnlyUser() {
		delay = forceShutdownThresholdSecs
		s.sendSysMessage(fmt.Sprintf("Shutdown delayed to %d seconds.", delay))
	}
	s.server.initiateShutdown(ctx, delay, mask, exitCode, reason)
}

// serverIsOnlyUser mirrors IsOnlyUser (cs_server.cpp:344-355) as far as
// Go can: C++ compares remote addresses, which Go sessions do not carry,
// so any other connected session counts as another user.
func (s *session) serverIsOnlyUser() bool {
	if s == nil || s.server == nil {
		return true
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	return len(s.server.sessions) <= 1
}

// serverShutdownCancel mirrors HandleServerShutDownCancelCommand
// (cs_server.cpp:341-347): the shared cancel arm for all four nodes.
func (s *session) serverShutdownCancel(ctx context.Context, perm uint32) {
	if s.miscDeny(ctx, perm) {
		return
	}
	remaining, wasRestart, ok := s.server.cancelShutdown(ctx)
	if !ok {
		s.sendSysMessage("There is no shutdown timer to cancel.")
		return
	}
	verb := "shutdown"
	if wasRestart {
		verb = "restart"
	}
	s.sendSysMessage(fmt.Sprintf("Server %s cancelled with %d seconds remaining.", verb, remaining))
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
// (cs_server.cpp:373): cancel arm + the "" arm with
// SHUTDOWN_MASK_RESTART | SHUTDOWN_MASK_IDLE.
func (s *session) handleServerIdleRestart(ctx context.Context, args []string) {
	if len(args) > 0 && strings.ToLower(args[0]) == "cancel" {
		s.serverShutdownCancel(ctx, permissionCommandServerIdlerestartCancel)
		return
	}
	if s.miscDeny(ctx, permissionCommandServerIdlerestart) {
		return
	}
	s.serverShutdownTimed(ctx, args, shutdownMaskRestart|shutdownMaskIdle, restartExitCode, "idlerestart")
}

// handleServerIdleShutdown mirrors HandleServerIdleShutDownCommand
// (cs_server.cpp:368): cancel arm + the "" arm with SHUTDOWN_MASK_IDLE.
func (s *session) handleServerIdleShutdown(ctx context.Context, args []string) {
	if len(args) > 0 && strings.ToLower(args[0]) == "cancel" {
		s.serverShutdownCancel(ctx, permissionCommandServerIdleshutdownCancel)
		return
	}
	if s.miscDeny(ctx, permissionCommandServerIdleshutdown) {
		return
	}
	s.serverShutdownTimed(ctx, args, shutdownMaskIdle, shutdownExitCode, "idleshutdown")
}

// handleServerRestart mirrors HandleServerRestartCommand (cs_server.cpp:358)
// plus the force node (cs_server.cpp:379): cancel / force / "" arms.
func (s *session) handleServerRestart(ctx context.Context, args []string) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "cancel":
			s.serverShutdownCancel(ctx, permissionCommandServerRestartCancel)
			return
		case "force":
			if s.miscDeny(ctx, permissionCommandServerRestartForce) {
				return
			}
			s.serverShutdownTimed(ctx, args[1:], shutdownMaskForce|shutdownMaskRestart, restartExitCode, "restart force")
			return
		}
	}
	if s.miscDeny(ctx, permissionCommandServerRestart) {
		return
	}
	s.serverShutdownTimed(ctx, args, shutdownMaskRestart, restartExitCode, "restart")
}

// handleServerShutdown mirrors HandleServerShutDownCommand (cs_server.cpp:353)
// plus the force node (cs_server.cpp:374): cancel / force / "" arms.
func (s *session) handleServerShutdown(ctx context.Context, args []string) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "cancel":
			s.serverShutdownCancel(ctx, permissionCommandServerShutdownCancel)
			return
		case "force":
			if s.miscDeny(ctx, permissionCommandServerShutdownForce) {
				return
			}
			s.serverShutdownTimed(ctx, args[1:], shutdownMaskForce, shutdownExitCode, "shutdown force")
			return
		}
	}
	if s.miscDeny(ctx, permissionCommandServerShutdown) {
		return
	}
	s.serverShutdownTimed(ctx, args, 0, shutdownExitCode, "shutdown")
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
		// World::SetClosed inverts the value for scripters
		// (World.cpp:208): closed on pushes open=false.
		if s.server.Features != nil && s.server.Features.Scripts != nil {
			_, _ = s.server.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventOpenStateChange, false)
		}
	case "off":
		s.server.closed.Store(false)
		s.sendSysMessage("World opened.") // LANG_WORLD_OPENED 7524
		if s.server.Features != nil && s.server.Features.Scripts != nil {
			_, _ = s.server.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventOpenStateChange, true)
		}
	default:
		s.sendSysMessage("Use on/off.") // LANG_USE_BOL 259
	}
}
