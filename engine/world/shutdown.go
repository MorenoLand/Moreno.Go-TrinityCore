package world

import (
	"context"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// Shutdown-timer port: World::ShutdownServ / ShutdownCancel / ShutdownMsg
// (World.cpp:2983-3072), the World::Update expiry leg (World.cpp:2983-3000),
// and cs_server.cpp ShutdownServer arg parsing (cs_server.cpp:457-521).
// World.h:54-65: SHUTDOWN_MASK_RESTART = 1, SHUTDOWN_MASK_IDLE = 2,
// SHUTDOWN_MASK_FORCE = 4, SHUTDOWN_EXIT_CODE = 0, RESTART_EXIT_CODE = 2.
// World.cpp:1027: GM.ForceShutdownThreshold defaults to 30 seconds.
// DELTAS: Go Stop() cannot restart the process, so a restart timer ends
// in Stop() like a shutdown (the exit code is recorded, not honored —
// a process supervisor must relaunch); the C++ IsOnlyUser address check
// has no Go twin (sessions carry no remote address), so the force
// threshold override applies whenever any other session is connected;
// queued sessions do not hold an idle timer (only live sessions count).

const (
	shutdownMaskRestart = 1
	shutdownMaskIdle    = 2
	shutdownMaskForce   = 4

	shutdownExitCode = 0
	restartExitCode  = 2

	forceShutdownThresholdSecs = 30
)

// serverShutdown mirrors World's m_ShutdownTimer / m_ShutdownMask /
// m_ExitCode triple, with a wall-clock deadline in place of the
// per-second decrementing counter (the Go world tick runs every 100ms).
type serverShutdown struct {
	active                 bool
	mask                   uint32
	exitCode               uint8
	reason                 string
	deadline               time.Time
	lastBroadcastRemaining int64
}

// broadcastShutdownMessage mirrors World::SendServerMessage: every
// connected session sees the countdown / cancel line.
func (s *Server) broadcastShutdownMessage(msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess != nil && sess.conn != nil {
			sess.sendSysMessage(msg)
		}
	}
}

// formatShutdownDuration mirrors secsToTimeString (Numeric): h:mm:ss
// above an hour, m:ss below it.
func formatShutdownDuration(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	if secs >= 86400 {
		return fmt.Sprintf("%d:%02d:%02d:%02d", secs/86400, (secs%86400)/3600, (secs%3600)/60, secs%60)
	}
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// shutdownCountdownMessage builds the SERVER_MSG_SHUTDOWN_TIME /
// SERVER_MSG_RESTART_TIME line for the remaining seconds.
func shutdownCountdownMessage(mask uint32, secs int64, reason string) string {
	verb := "shutdown"
	if mask&shutdownMaskRestart != 0 {
		verb = "restart"
	}
	msg := fmt.Sprintf("Server %s in %s.", verb, formatShutdownDuration(secs))
	if reason != "" {
		msg += " - " + reason
	}
	return msg
}

// shouldBroadcastShutdown mirrors World::ShutdownMsg's cadence gate
// (World.cpp:3035-3039): every 15s under 5 minutes, every minute under
// 15, every 5 minutes under 30, hourly under 12 hours, every 12 hours
// above that. Idle timers never broadcast (ShutdownMsg early return).
func shouldBroadcastShutdown(mask uint32, secs int64) bool {
	if mask&shutdownMaskIdle != 0 {
		return false
	}
	switch {
	case secs < 5*60:
		return secs%15 == 0
	case secs < 15*60:
		return secs%60 == 0
	case secs < 30*60:
		return secs%300 == 0
	case secs < 12*3600:
		return secs%3600 == 0
	default:
		// C++ broadcasts only when > 12h (World.cpp:3039), so exactly
		// 12h stays silent; Go's old %43200 branch rang at 43200.
		return secs > 12*3600 && secs%43200 == 0
	}
}

// shutdownRemainingSecs reports the whole seconds left, rounding up so
// a timer started with delay N still reads N on the first tick.
func shutdownRemainingSecs(deadline, now time.Time) int64 {
	secs := int64((deadline.Sub(now) + time.Second - 1) / time.Second)
	if secs < 0 {
		secs = 0
	}
	return secs
}

// initiateShutdown mirrors World::ShutdownServ: overwrite any running
// timer, warn once (unless delay 0, which C++ arms silently at 1 second,
// or the idle mask, which never broadcasts), then fire
// WORLD_EVENT_ON_SHUTDOWN_INIT (11) with (event, code, mask) per
// ServerHooks.cpp:282-288. Ignored once Stop() has run (IsStopped).
func (s *Server) initiateShutdown(ctx context.Context, delaySecs int, mask uint32, exitCode uint8, reason string) {
	if s == nil || s.stopEvent.Load() {
		return
	}
	now := time.Now()
	s.shutdownMu.Lock()
	if delaySecs <= 0 {
		s.shutdown = serverShutdown{active: true, mask: mask, exitCode: exitCode, reason: reason, deadline: now.Add(time.Second), lastBroadcastRemaining: -1}
	} else {
		s.shutdown = serverShutdown{active: true, mask: mask, exitCode: exitCode, reason: reason, deadline: now.Add(time.Duration(delaySecs) * time.Second), lastBroadcastRemaining: int64(delaySecs)}
	}
	s.shutdownMu.Unlock()
	if delaySecs > 0 && mask&shutdownMaskIdle == 0 {
		s.broadcastShutdownMessage(shutdownCountdownMessage(mask, int64(delaySecs), reason))
	}
	if s.Features != nil && s.Features.Scripts != nil {
		_, _ = s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventShutdownInit, uint32(exitCode), mask)
	}
}

// cancelShutdown mirrors World::ShutdownCancel: ok reports whether a
// timer actually existed (C++ returns 0 and fires nothing when none did),
// remaining is the seconds it had left, and when a timer existed it
// broadcasts the cancel line and fires WORLD_EVENT_ON_SHUTDOWN_CANCEL
// (12) with (event) per ServerHooks.cpp:290-295.
func (s *Server) cancelShutdown(ctx context.Context) (remaining uint32, wasRestart bool, ok bool) {
	if s == nil || s.stopEvent.Load() {
		return 0, false, false
	}
	now := time.Now()
	s.shutdownMu.Lock()
	if !s.shutdown.active {
		s.shutdownMu.Unlock()
		return 0, false, false
	}
	remaining = uint32(shutdownRemainingSecs(s.shutdown.deadline, now))
	wasRestart = s.shutdown.mask&shutdownMaskRestart != 0
	s.shutdown = serverShutdown{lastBroadcastRemaining: -1}
	s.shutdownMu.Unlock()
	verb := "shutdown"
	if wasRestart {
		verb = "restart"
	}
	s.broadcastShutdownMessage(fmt.Sprintf("Server %s cancelled.", verb))
	if s.Features != nil && s.Features.Scripts != nil {
		_, _ = s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventShutdownCancel)
	}
	return remaining, wasRestart, true
}

// loginShutdownNotice mirrors the login-time ShutdownMsg(true, player)
// (CharacterHandler.cpp:989-990): a player logging in while a shutdown
// timer runs is shown the countdown immediately. The reason is not
// repeated here — ShutdownMsg's default reason is empty (World.cpp:3027),
// it only rides the initiate broadcast — and idle timers never show
// (ShutdownMsg early return, World.cpp:3029-3031).
func (s *Server) loginShutdownNotice(sess *session) {
	if s == nil || sess == nil {
		return
	}
	s.shutdownMu.Lock()
	active, mask, deadline := s.shutdown.active, s.shutdown.mask, s.shutdown.deadline
	s.shutdownMu.Unlock()
	if !active || mask&shutdownMaskIdle != 0 {
		return
	}
	sess.sendSysMessage(shutdownCountdownMessage(mask, shutdownRemainingSecs(deadline, time.Now()), ""))
}

// updateShutdownTimer mirrors the World::Update shutdown leg, driven by
// the 100ms world tick instead of the 1s C++ update: on expiry an idle
// timer with live sessions re-arms at 1 second, anything else stops the
// server (m_stopEvent -> Stop(), which fires WORLD_EVENT_ON_SHUTDOWN
// (15)); otherwise a cadence-hit countdown line goes out once per
// remaining-second value.
func (s *Server) updateShutdownTimer(ctx context.Context, now time.Time) {
	if s == nil || s.stopEvent.Load() {
		return
	}
	s.shutdownMu.Lock()
	if !s.shutdown.active {
		s.shutdownMu.Unlock()
		return
	}
	remaining := shutdownRemainingSecs(s.shutdown.deadline, now)
	if remaining <= 0 {
		idle := s.shutdown.mask&shutdownMaskIdle != 0
		s.shutdownMu.Unlock()
		if idle {
			s.sessionsMu.RLock()
			live := len(s.sessions)
			s.sessionsMu.RUnlock()
			if live > 0 {
				s.shutdownMu.Lock()
				if s.shutdown.active {
					s.shutdown.deadline = now.Add(time.Second)
					s.shutdown.lastBroadcastRemaining = -1
				}
				s.shutdownMu.Unlock()
				return
			}
		}
		s.shutdownMu.Lock()
		s.shutdown = serverShutdown{lastBroadcastRemaining: -1}
		s.shutdownMu.Unlock()
		s.Stop()
		return
	}
	if remaining != s.shutdown.lastBroadcastRemaining && shouldBroadcastShutdown(s.shutdown.mask, remaining) {
		mask := s.shutdown.mask
		s.shutdown.lastBroadcastRemaining = remaining
		s.shutdownMu.Unlock()
		// The reason rides only the initiate broadcast (ShutdownServ,
		// World.cpp:3020): the World::Update leg calls ShutdownMsg()
		// with the default empty reason (World.cpp:2998-3000).
		s.broadcastShutdownMessage(shutdownCountdownMessage(mask, remaining, ""))
		return
	}
	s.shutdownMu.Unlock()
}

// parseShutdownDelay mirrors ShutdownServer's delay parse
// (cs_server.cpp:469-488): all digits -> atoi with the C++ zero guard
// (a "0" result from any other token is invalid, negatives invalid),
// otherwise TimeStringToSecs (Util.cpp:240: d/h/m/s suffixes, a bad
// suffix or a zero total is invalid; a trailing digit run with no suffix
// is dropped, exactly like C++).
func parseShutdownDelay(token string) (int, bool) {
	if token == "" {
		return 0, false
	}
	if isNumericString(token) {
		delay := cAtoi(token)
		// C++ zero guard: a 0 result from anything but exactly "0"
		// ("00", "000") is a syntax error, not a 0s delay.
		if delay < 0 || (delay == 0 && token != "0") {
			return 0, false
		}
		return delay, true
	}
	var secs, buffer uint32
	for i := 0; i < len(token); i++ {
		c := token[i]
		if c >= '0' && c <= '9' {
			buffer = buffer*10 + uint32(c-'0')
			continue
		}
		var mult uint32
		switch c {
		case 'd':
			mult = 86400
		case 'h':
			mult = 3600
		case 'm':
			mult = 60
		case 's':
			mult = 1
		default:
			return 0, false
		}
		secs += buffer * mult
		buffer = 0
	}
	if secs == 0 {
		return 0, false
	}
	return int(secs), true
}

// parseShutdownExitCode mirrors ParseExitCode (cs_server.cpp:440-455):
// atoi with the zero guard, range 0-125.
func parseShutdownExitCode(token string) (uint8, bool) {
	if !isNumericString(token) {
		return 0, false
	}
	code := cAtoi(token)
	// Same C++ zero guard as the delay: "00" is invalid, "0" is not.
	if code < 0 || code > 125 || (code == 0 && token != "0") {
		return 0, false
	}
	return uint8(code), true
}

// parseShutdownArgs mirrors the ShutdownServer token walk
// (cs_server.cpp:490-509): the first numeric token after the delay is
// the exit code (later numerics overwrite it), the first non-numeric
// token starts the reason, which runs to the end of the line.
func parseShutdownArgs(tokens []string, defaultExitCode uint8) (exitCode uint8, reason string, ok bool) {
	exitCode = defaultExitCode
	for i, tok := range tokens {
		if isNumericString(tok) {
			code, valid := parseShutdownExitCode(tok)
			if !valid {
				return 0, "", false
			}
			exitCode = code
			continue
		}
		reason = tok
		for _, rest := range tokens[i+1:] {
			reason += " " + rest
		}
		break
	}
	return exitCode, reason, true
}
