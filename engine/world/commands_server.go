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
//     it); the security-level names and negative values set
//     allowedSecurityLevel (admission enforces it, server.go:10283);
//     "reset" (any case-sensitive prefix == C++ strncmp order) restores
//     Config.PlayerLimit and reloads the level from the realmlist row.
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
// for one node: args are "<delay> [#exit_code] [reason]". The bool
// reports C++ handler success: every parse failure (empty args, a
// remainder over 255 chars, an unparseable delay or exit code) returns
// false silently == C++, so dispatchCommand shows the node's help
// listing (ChatCommand.cpp:296-309); the force-threshold leg prints its
// message and returns true. The force-threshold override
// (World.cpp:1027, cs_server.cpp:511-516) raises a sub-30s delay to 30s
// while other players are connected, unless the force mask is set.
func (s *session) serverShutdownTimed(ctx context.Context, args []string, mask uint32, defaultExitCode uint8) bool {
	// C++ rejects a remainder longer than 255 chars (cs_server.cpp:460);
	// the tokenized args re-join approximates that remainder here.
	if len(strings.Join(args, " ")) > 255 {
		return false
	}
	if len(args) == 0 {
		return false
	}
	delay, ok := parseShutdownDelay(args[0])
	if !ok {
		return false
	}
	exitCode, reason, ok := parseShutdownArgs(args[1:], defaultExitCode)
	if !ok {
		return false
	}
	if delay < forceShutdownThresholdSecs && mask&shutdownMaskForce == 0 && !s.serverIsOnlyUser() {
		delay = forceShutdownThresholdSecs
		s.sendSysMessage(fmt.Sprintf("Shutdown delayed to %d seconds.", delay))
	}
	s.server.initiateShutdown(ctx, delay, mask, exitCode, reason)
	return true
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
func (s *session) serverShutdownCancel(ctx context.Context, perm uint32) bool {
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	remaining, wasRestart, ok := s.server.cancelShutdown(ctx)
	if !ok {
		// C++ ShutdownCancel returns 0 with no message when no timer runs.
		return true
	}
	verb := "shutdown"
	if wasRestart {
		verb = "restart"
	}
	s.sendSysMessage(fmt.Sprintf("Server %s cancelled with %d seconds remaining.", verb, remaining))
	return true
}

// handleCmdServer dispatches the "server" root (cs_server.cpp:99-110).
// The bool reports C++ handler success: RBAC denial returns false
// silently so dispatchCommand shows the node's help listing
// (ChatCommand.cpp:296-309); paths that print a message return true.
// The ShutdownServer parse failures return false silently == C++
// (cs_server.cpp:457-521: empty args, over-255 remainder, unparseable
// delay or exit code); SetClosed's bad value prints LANG_USE_BOL and
// returns true (C++ sends it with SetSentErrorMessage, :409-425).
func (s *session) handleCmdServer(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Bare ".server": the root perm 718 is dead in C++ (deprecated
		// 6-arg nullptr+subtable overload), so this is ungated.
		s.sendSysMessage("Syntax: .server corpses|debug|exit|idlerestart|idleshutdown|info|motd|plimit|restart|shutdown|set")
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("corpses", sub):
		return s.handleServerCorpses(ctx)
	case strings.HasPrefix("debug", sub):
		return s.handleServerDebug(ctx)
	case strings.HasPrefix("exit", sub):
		return s.handleServerExit(ctx)
	case strings.HasPrefix("idlerestart", sub):
		return s.handleServerIdleRestart(ctx, rest)
	case strings.HasPrefix("idleshutdown", sub):
		return s.handleServerIdleShutdown(ctx, rest)
	case strings.HasPrefix("info", sub):
		return s.handleServerInfo(ctx)
	case strings.HasPrefix("motd", sub):
		return s.handleServerMotd(ctx)
	case strings.HasPrefix("plimit", sub):
		return s.handleServerPLimit(ctx, rest)
	case strings.HasPrefix("restart", sub):
		return s.handleServerRestart(ctx, rest)
	case strings.HasPrefix("shutdown", sub):
		return s.handleServerShutdown(ctx, rest)
	case strings.HasPrefix("set", sub):
		return s.handleServerSet(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .server corpses|debug|exit|idlerestart|idleshutdown|info|motd|plimit|restart|shutdown|set")
		return true
	}
}

// handleServerCorpses mirrors HandleServerCorpsesCommand (cs_server.cpp:121):
// force the expired-corpse sweep now (World::RemoveOldCorpses) and report
// the count. The automatic sweep also runs from the world tick every 20
// minutes (WUPDATE_CORPSES, World.cpp:2106).
func (s *session) handleServerCorpses(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerCorpses) {
		return false
	}
	if s.server == nil {
		return true
	}
	s.server.lastCorpseExpiry = time.Now()
	// C++ returns true silently; the sweep itself is the effect.
	s.server.expireOldCorpses(ctx)
	return true
}

// handleServerDebug mirrors HandleServerDebugCommand (cs_server.cpp:126):
// partial — Go runtime/build info, world DB version, listen port.
func (s *session) handleServerDebug(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerDebug) {
		return false
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
	return true
}

// handleServerExit mirrors HandleServerExitCommand (cs_server.cpp:383):
// native via Server.Stop().
func (s *session) handleServerExit(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerExit) {
		return false
	}
	s.sendSysMessage("Exiting.") // LANG_COMMAND_EXIT 1000
	s.server.Stop()
	return true
}

// handleServerIdleRestart mirrors HandleServerIdleRestartCommand
// (cs_server.cpp:373): cancel arm + the "" arm with
// SHUTDOWN_MASK_RESTART | SHUTDOWN_MASK_IDLE.
func (s *session) handleServerIdleRestart(ctx context.Context, args []string) bool {
	// Sub-table names prefix-match like every other level (ChatCommand).
	if len(args) > 0 && strings.HasPrefix("cancel", strings.ToLower(args[0])) {
		return s.serverShutdownCancel(ctx, permissionCommandServerIdlerestartCancel)
	}
	if !s.commandAllowed(ctx, permissionCommandServerIdlerestart) {
		return false
	}
	return s.serverShutdownTimed(ctx, args, shutdownMaskRestart|shutdownMaskIdle, restartExitCode)
}

// handleServerIdleShutdown mirrors HandleServerIdleShutDownCommand
// (cs_server.cpp:368): cancel arm + the "" arm with SHUTDOWN_MASK_IDLE.
func (s *session) handleServerIdleShutdown(ctx context.Context, args []string) bool {
	// Sub-table names prefix-match like every other level (ChatCommand).
	if len(args) > 0 && strings.HasPrefix("cancel", strings.ToLower(args[0])) {
		return s.serverShutdownCancel(ctx, permissionCommandServerIdleshutdownCancel)
	}
	if !s.commandAllowed(ctx, permissionCommandServerIdleshutdown) {
		return false
	}
	return s.serverShutdownTimed(ctx, args, shutdownMaskIdle, shutdownExitCode)
}

// handleServerRestart mirrors HandleServerRestartCommand (cs_server.cpp:358)
// plus the force node (cs_server.cpp:379): cancel / force / "" arms.
func (s *session) handleServerRestart(ctx context.Context, args []string) bool {
	if len(args) > 0 {
		// Sub-table names prefix-match: "c"/"forc" route like full words.
		switch {
		case strings.HasPrefix("cancel", strings.ToLower(args[0])):
			return s.serverShutdownCancel(ctx, permissionCommandServerRestartCancel)
		case strings.HasPrefix("force", strings.ToLower(args[0])):
			if !s.commandAllowed(ctx, permissionCommandServerRestartForce) {
				return false
			}
			return s.serverShutdownTimed(ctx, args[1:], shutdownMaskForce|shutdownMaskRestart, restartExitCode)
		}
	}
	if !s.commandAllowed(ctx, permissionCommandServerRestart) {
		return false
	}
	return s.serverShutdownTimed(ctx, args, shutdownMaskRestart, restartExitCode)
}

// handleServerShutdown mirrors HandleServerShutDownCommand (cs_server.cpp:353)
// plus the force node (cs_server.cpp:374): cancel / force / "" arms.
func (s *session) handleServerShutdown(ctx context.Context, args []string) bool {
	if len(args) > 0 {
		// Sub-table names prefix-match: "c"/"forc" route like full words.
		switch {
		case strings.HasPrefix("cancel", strings.ToLower(args[0])):
			return s.serverShutdownCancel(ctx, permissionCommandServerShutdownCancel)
		case strings.HasPrefix("force", strings.ToLower(args[0])):
			if !s.commandAllowed(ctx, permissionCommandServerShutdownForce) {
				return false
			}
			return s.serverShutdownTimed(ctx, args[1:], shutdownMaskForce, shutdownExitCode)
		}
	}
	if !s.commandAllowed(ctx, permissionCommandServerShutdown) {
		return false
	}
	return s.serverShutdownTimed(ctx, args, 0, shutdownExitCode)
}

// formatSecsFullText mirrors secsToTimeString FullText (Util.cpp:97):
// "1 Day 2 Hours 3 Minutes 4 Seconds", singulars for 1 (hours: <= 1),
// zero seconds only when the whole value is zero.
func formatSecsFullText(total int64) string {
	if total < 0 {
		total = 0
	}
	days, hours, minutes, secs := total/86400, total%86400/3600, total%3600/60, total%60
	var b strings.Builder
	if days > 0 {
		if days == 1 {
			b.WriteString("1 Day ")
		} else {
			fmt.Fprintf(&b, "%d Days ", days)
		}
	}
	if hours > 0 {
		if hours == 1 {
			b.WriteString("1 Hour ")
		} else {
			fmt.Fprintf(&b, "%d Hours ", hours)
		}
	}
	if minutes > 0 {
		if minutes == 1 {
			b.WriteString("1 Minute ")
		} else {
			fmt.Fprintf(&b, "%d Minutes ", minutes)
		}
	}
	if secs > 0 || (days == 0 && hours == 0 && minutes == 0) {
		// C++ uses the singular for 0 too (secs <= 1, Util.cpp:180-184:
		// "0 Second."), not just for 1.
		if secs <= 1 {
			fmt.Fprintf(&b, "%d Second", secs)
		} else {
			fmt.Fprintf(&b, "%d Seconds", secs)
		}
	}
	return strings.TrimSpace(b.String())
}

// handleServerInfo mirrors HandleServerInfoCommand (cs_server.cpp:240).
func (s *session) handleServerInfo(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerInfo) {
		return false
	}
	s.server.sessionsMu.RLock()
	online := len(s.server.sessions)
	s.server.sessionsMu.RUnlock()
	uptime := formatSecsFullText(int64(time.Since(applicationStartTime) / time.Second))
	s.sendSysMessage("Moreno.Go-TrinityCore (WotLK 3.3.5a 12340)")
	s.sendSysMessage(fmt.Sprintf("Connected players: %d.", online)) // LANG_CONNECTED_PLAYERS 60
	s.sendSysMessage(fmt.Sprintf("Server uptime: %s.", uptime))     // LANG_UPTIME 13
	// C++ prints the shutdown timeleft while a timer runs (cs_server.cpp:273).
	if s.server != nil {
		s.server.shutdownMu.Lock()
		active, deadline := s.server.shutdown.active, s.server.shutdown.deadline
		s.server.shutdownMu.Unlock()
		if active {
			s.sendSysMessage(fmt.Sprintf("Server shutdown in %s.", formatSecsFullText(shutdownRemainingSecs(deadline, time.Now()))))
		}
	}
	return true
}

// handleServerMotd mirrors HandleServerMotdCommand (cs_server.cpp:279).
func (s *session) handleServerMotd(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerMotd) {
		return false
	}
	// C++ displays FormattedMotd: the stored text split on '@' into lines.
	motd := strings.Join(strings.Split(s.server.Config.Motd, "@"), "\n")
	s.sendSysMessage(fmt.Sprintf("MOTD: %s", motd)) // LANG_MOTD_CURRENT 56
	return true
}

// handleServerPLimit mirrors HandleServerPLimitCommand (cs_server.cpp:285):
// the amount limit is native (admission honors it); the security-level
// names and negative values set allowedSecurityLevel (admission enforces
// it, server.go:10283); "reset" restores Config.PlayerLimit and reloads
// the level from the realmlist row (LoadDBAllowedSecurityLevel).
func (s *session) handleServerPLimit(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandServerPlimit) {
		return false
	}
	if len(args) > 0 {
		arg := args[0]
		// C++ checks the names with strncmp(paramStr, name, strlen(paramStr))
		// — a case-sensitive prefix match — before falling through to
		// atoi (cs_server.cpp:295-315). Mirror that order here.
		switch {
		case isStrncmpPrefix(arg, "player"):
			s.server.allowedSecurityLevel = 0
		case isStrncmpPrefix(arg, "moderator"):
			s.server.allowedSecurityLevel = 1
		case isStrncmpPrefix(arg, "gamemaster"):
			s.server.allowedSecurityLevel = 2
		case isStrncmpPrefix(arg, "administrator"):
			s.server.allowedSecurityLevel = 3
		case isStrncmpPrefix(arg, "reset"):
			s.server.playerLimit = s.server.Config.PlayerLimit
			_ = s.server.loadPlayerSecurityLimit(ctx)
		default:
			v := cAtoi(arg)
			if v < 0 {
				// C++ SetPlayerSecurityLimit(AccountTypes(-value)).
				lvl := -v
				if lvl > 255 {
					lvl = 255
				}
				s.server.allowedSecurityLevel = uint8(lvl)
			} else {
				s.server.playerLimit = uint32(v)
			}
		}
	}
	s.sendSysMessage(fmt.Sprintf("Player limits: amount %d, min. security level %s.", s.server.playerLimit, serverSecurityLevelName(s.server.allowedSecurityLevel)))
	return true
}

// serverSecurityLevelName mirrors the plimit status switch
// (cs_server.cpp:323-342): 0-3 named, anything else "<unknown>".
func serverSecurityLevelName(level uint8) string {
	switch level {
	case 0:
		return "Player"
	case 1:
		return "Moderator"
	case 2:
		return "Gamemaster"
	case 3:
		return "Administrator"
	default:
		return "<unknown>"
	}
}

// isStrncmpPrefix reports whether arg would satisfy
// strncmp(arg, name, strlen(arg)) == 0: arg is a non-empty case-sensitive
// prefix of name.
func isStrncmpPrefix(arg, name string) bool {
	return arg != "" && strings.HasPrefix(name, arg)
}

// handleServerSet dispatches the "server set" sub-table
// (cs_server.cpp:92-94): loglevel, motd, closed.
func (s *session) handleServerSet(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Bare ".server set": the "set" node perm 729 is dead in C++ (the
		// deprecated 6-arg nullptr+subtable overload drops it and there is
		// no "" child), so this is ungated.
		s.sendSysMessage("Syntax: .server set loglevel|motd|closed")
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("loglevel", sub):
		return s.handleServerSetLogLevel(ctx)
	case strings.HasPrefix("motd", sub):
		return s.handleServerSetMotd(ctx, rest)
	case strings.HasPrefix("closed", sub):
		return s.handleServerSetClosed(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .server set loglevel|motd|closed")
		return true
	}
}

// handleServerSetLogLevel mirrors HandleServerSetLogLevelCommand
// (cs_server.cpp:431): documented-blocked — the slog logger's level is fixed
// at handler creation.
func (s *session) handleServerSetLogLevel(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandServerSetLoglevel) {
		return false
	}
	s.sendSysMessage("Changing the log level at runtime is not supported by this server.")
	return true
}

// handleServerSetMotd mirrors HandleServerSetMotdCommand (cs_server.cpp:393).
func (s *session) handleServerSetMotd(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandServerSetMotd) {
		return false
	}
	motd := strings.Join(args, " ")
	s.server.Config.Motd = motd
	s.sendSysMessage(fmt.Sprintf("New MOTD: %s", motd)) // LANG_MOTD_NEW 1101
	return true
}

// handleServerSetClosed mirrors HandleServerSetClosedCommand
// (cs_server.cpp:409).
func (s *session) handleServerSetClosed(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandServerSetClosed) {
		return false
	}
	// C++ compares the whole remainder with strncmp(args, "on", 3) /
	// strncmp(args, "off", 4): only exactly "on"/"off" match, so extra
	// tokens ("on extra") reject with LANG_USE_BOL, like empty input.
	// The bad-value leg prints the message (SetSentErrorMessage in C++)
	// and returns true, so no help listing follows.
	if len(args) != 1 {
		s.sendSysMessage("Use on/off.") // LANG_USE_BOL 259
		return true
	}
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
	return true
}
