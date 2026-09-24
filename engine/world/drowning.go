package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	mirrorTimerFatigue uint32 = 0
	mirrorTimerBreath  uint32 = 1
	mirrorTimerFire    uint32 = 2

	maxFatigueTimerMs int32 = 60000  // 1 minute in 3.3.5 (Player.cpp:819)
	maxBreathTimerMs  int32 = 180000 // 3 minutes in 3.3.5 (Player.cpp:826)
	maxFireTimerMs    int32 = 1000   // 1 second in 3.3.5 (Player.cpp:834)
)

// buildStartMirrorTimer builds SMSG_START_MIRROR_TIMER (0x1D9).
// Mirrors TrinityCore WorldPackets::Misc::StartMirrorTimer::Write.
func buildStartMirrorTimer(timerType uint32, value, maxValue uint32, scale int32, paused bool, spellID uint32) []byte {
	buf := protocol.NewBuffer(21)
	buf.WriteU32(timerType)
	buf.WriteU32(value)
	buf.WriteU32(maxValue)
	buf.WriteI32(scale)
	if paused {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(spellID)
	return buf.Bytes()
}

// buildPauseMirrorTimer builds SMSG_PAUSE_MIRROR_TIMER (0x1DA).
func buildPauseMirrorTimer(timerType uint32, paused bool) []byte {
	buf := protocol.NewBuffer(5)
	buf.WriteU32(timerType)
	if paused {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	return buf.Bytes()
}

// buildStopMirrorTimer builds SMSG_STOP_MIRROR_TIMER (0x1DB).
func buildStopMirrorTimer(timerType uint32) []byte {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(timerType)
	return buf.Bytes()
}

func (s *session) sendMirrorTimer(timerType uint32, value, maxValue uint32, scale int32) {
	pkt := buildStartMirrorTimer(timerType, value, maxValue, scale, false, 0)
	_ = s.write(uint16(protocol.OpcodeSMSG_START_MIRROR_TIMER), pkt, true)
}

func (s *session) stopMirrorTimer(timerType uint32) {
	pkt := buildStopMirrorTimer(timerType)
	_ = s.write(uint16(protocol.OpcodeSMSG_STOP_MIRROR_TIMER), pkt, true)
}

// handleEnterSwimming initializes breath countdown if water breathing is not active.
func (s *session) handleEnterSwimming() {
	if s.player == nil || s.player.Health == 0 {
		return
	}
	// Water breathing check (SPELL_AURA_WATER_BREATHING = 82, SPELL_AURA_MOD_WATER_BREATHING = 155)
	if s.hasAuraType(82) || s.hasAuraType(155) {
		s.breathTimer = -1
		return
	}
	if (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0 {
		s.breathTimer = -1
		return
	}
	if s.breathTimer <= 0 {
		s.breathTimer = maxBreathTimerMs
	}
	s.lastBreathTick = time.Now()
	s.sendMirrorTimer(mirrorTimerBreath, uint32(s.breathTimer), uint32(maxBreathTimerMs), -1)
}

// handleExitSwimming switches breath mirror timer into regen mode (+10 scale) until full.
func (s *session) handleExitSwimming() {
	if s.breathTimer != -1 && s.breathTimer < maxBreathTimerMs {
		s.lastBreathTick = time.Now()
		s.sendMirrorTimer(mirrorTimerBreath, uint32(s.breathTimer), uint32(maxBreathTimerMs), 10)
	} else if s.breathTimer >= maxBreathTimerMs {
		s.breathTimer = -1
		s.stopMirrorTimer(mirrorTimerBreath)
	}
}

// handleDrowningTick updates breath countdown or deals drowning environmental damage.
// Mirrors TrinityCore Player::HandleDrowning (Player.cpp:860-900).
func (s *session) handleDrowningTick(ctx context.Context, now time.Time) {
	if s.player == nil || s.player.Health == 0 {
		if s.breathTimer != -1 {
			s.breathTimer = -1
			s.stopMirrorTimer(mirrorTimerBreath)
		}
		return
	}

	if s.isSwimming {
		// Water breathing check
		if s.hasAuraType(82) || s.hasAuraType(155) || (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0 {
			if s.breathTimer != -1 {
				s.breathTimer = -1
				s.stopMirrorTimer(mirrorTimerBreath)
			}
			return
		}

		if s.breathTimer == -1 {
			s.breathTimer = maxBreathTimerMs
			s.lastBreathTick = now
			s.sendMirrorTimer(mirrorTimerBreath, uint32(s.breathTimer), uint32(maxBreathTimerMs), -1)
			return
		}

		if s.lastBreathTick.IsZero() {
			s.lastBreathTick = now
			return
		}

		diffMs := int32(now.Sub(s.lastBreathTick).Milliseconds())
		if diffMs <= 0 {
			return
		}
		s.lastBreathTick = now
		s.breathTimer -= diffMs

		if s.breathTimer <= 0 {
			// Drowning tick: deal damage every second (Player.cpp:880-884)
			s.breathTimer = 0
			damage := s.player.MaxHealth / 5
			if damage == 0 {
				damage = 1
			}
			s.environmentalDamage(ctx, damageDrowning, damage)
		}
	} else if s.breathTimer != -1 {
		// Regenerate breath at 10x speed when surfaced (Player.cpp:894)
		if s.lastBreathTick.IsZero() {
			s.lastBreathTick = now
			return
		}
		diffMs := int32(now.Sub(s.lastBreathTick).Milliseconds())
		if diffMs <= 0 {
			return
		}
		s.lastBreathTick = now
		s.breathTimer += 10 * diffMs
		if s.breathTimer >= maxBreathTimerMs {
			s.breathTimer = -1
			s.stopMirrorTimer(mirrorTimerBreath)
		}
	}
}

// handleEnterDarkWater initializes fatigue countdown if fatigue is not disabled.
func (s *session) handleEnterDarkWater() {
	if s.player == nil {
		return
	}
	// Denveous's Marker AX3: GM / security check (Player.cpp:817)
	if (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0 {
		s.fatigueTimer = -1
		return
	}
	// In flight (Player.cpp:24539)
	if s.isInFlight() {
		s.fatigueTimer = -1
		return
	}
	// Dead and not a ghost
	isGhost := s.player.PlayerFlags&playerFlagGhost != 0
	if s.player.Health == 0 && !isGhost {
		s.fatigueTimer = -1
		return
	}
	if s.fatigueTimer <= 0 {
		s.fatigueTimer = maxFatigueTimerMs
	}
	s.lastFatigueTick = time.Now()
	s.sendMirrorTimer(mirrorTimerFatigue, uint32(s.fatigueTimer), uint32(maxFatigueTimerMs), -1)
}

// handleExitDarkWater switches fatigue mirror timer into regen mode (+10 scale) until full.
func (s *session) handleExitDarkWater() {
	if s.fatigueTimer != -1 && s.fatigueTimer < maxFatigueTimerMs {
		s.lastFatigueTick = time.Now()
		s.sendMirrorTimer(mirrorTimerFatigue, uint32(s.fatigueTimer), uint32(maxFatigueTimerMs), 10)
	} else if s.fatigueTimer >= maxFatigueTimerMs {
		s.fatigueTimer = -1
		s.stopMirrorTimer(mirrorTimerFatigue)
	}
}

// setInDarkWater sets whether the session is currently in dark water (fatigue zone).
func (s *session) setInDarkWater(darkWater bool) {
	if s.inDarkWater == darkWater {
		return
	}
	s.inDarkWater = darkWater
	if darkWater {
		s.handleEnterDarkWater()
	} else {
		s.handleExitDarkWater()
	}
}

// handleFatigueTick updates fatigue countdown or deals exhaustion environmental damage / teleports ghost.
// Mirrors TrinityCore Player::HandleDrowning (Player.cpp:901-937).
func (s *session) handleFatigueTick(ctx context.Context, now time.Time) {
	if s.player == nil {
		if s.fatigueTimer != -1 {
			s.fatigueTimer = -1
			s.stopMirrorTimer(mirrorTimerFatigue)
		}
		return
	}

	isGhost := s.player.PlayerFlags&playerFlagGhost != 0
	if s.player.Health == 0 && !isGhost {
		if s.fatigueTimer != -1 {
			s.fatigueTimer = -1
			s.stopMirrorTimer(mirrorTimerFatigue)
		}
		return
	}

	if s.inDarkWater {
		// Fatigue immunity / bypass check (GM or flight)
		if (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0 || s.isInFlight() {
			if s.fatigueTimer != -1 {
				s.fatigueTimer = -1
				s.stopMirrorTimer(mirrorTimerFatigue)
			}
			return
		}

		if s.fatigueTimer == -1 {
			s.fatigueTimer = maxFatigueTimerMs
			s.lastFatigueTick = now
			s.sendMirrorTimer(mirrorTimerFatigue, uint32(s.fatigueTimer), uint32(maxFatigueTimerMs), -1)
			return
		}

		if s.lastFatigueTick.IsZero() {
			s.lastFatigueTick = now
			return
		}

		diffMs := int32(now.Sub(s.lastFatigueTick).Milliseconds())
		if diffMs <= 0 {
			return
		}
		s.lastFatigueTick = now
		s.fatigueTimer -= diffMs

		if s.fatigueTimer <= 0 {
			// Fatigue timer depleted (Player.cpp:914-924):
			// Reset timer to 1 second so damage ticks every 1000ms
			s.fatigueTimer += 1000
			if s.fatigueTimer < 0 {
				s.fatigueTimer = 0
			}
			if s.player.Health > 0 && !isGhost {
				damage := s.player.MaxHealth / 5
				if damage == 0 {
					damage = 1
				}
				s.environmentalDamage(ctx, damageExhausted, damage)
			} else if isGhost {
				s.repopAtGraveyard(ctx)
			}
		}
	} else if s.fatigueTimer != -1 {
		// Regenerate fatigue at 10x speed when out of dark water (Player.cpp:932)
		if s.lastFatigueTick.IsZero() {
			s.lastFatigueTick = now
			return
		}
		diffMs := int32(now.Sub(s.lastFatigueTick).Milliseconds())
		if diffMs <= 0 {
			return
		}
		s.lastFatigueTick = now
		s.fatigueTimer += 10 * diffMs
		if s.fatigueTimer >= maxFatigueTimerMs || (s.player.Health == 0 && !isGhost) {
			s.fatigueTimer = -1
			s.stopMirrorTimer(mirrorTimerFatigue)
		}
	}
}

// stopMirrorTimers stops all active mirror timers.
// Mirrors Player::StopMirrorTimers (Player.cpp:848-853).
func (s *session) stopMirrorTimers() {
	if s.breathTimer != -1 {
		s.breathTimer = -1
		s.stopMirrorTimer(mirrorTimerBreath)
	}
	if s.fatigueTimer != -1 {
		s.fatigueTimer = -1
		s.stopMirrorTimer(mirrorTimerFatigue)
	}
}

// isFatigueActive returns true if the player is in dark water or currently regenerating fatigue.
// Mirrors Player::IsMirrorTimerActive(FATIGUE_TIMER).
func (s *session) isFatigueActive() bool {
	if s == nil {
		return false
	}
	return s.inDarkWater || (s.fatigueTimer > 0 && s.fatigueTimer < maxFatigueTimerMs)
}

// isBreathActive returns true if the player is underwater or currently regenerating breath.
// Mirrors Player::IsMirrorTimerActive(BREATH_TIMER).
func (s *session) isBreathActive() bool {
	if s == nil {
		return false
	}
	return s.isSwimming || (s.breathTimer > 0 && s.breathTimer < maxBreathTimerMs)
}

// updatePlayerUnderwater iterates over active sessions and processes drowning/breath and fatigue ticks.
func (s *Server) updatePlayerUnderwater(ctx context.Context, now time.Time) {
	s.sessionsMu.RLock()
	var sessions []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil &&
			(sess.isSwimming || (sess.breathTimer != -1 && sess.breathTimer < maxBreathTimerMs) ||
				sess.inDarkWater || (sess.fatigueTimer != -1 && sess.fatigueTimer < maxFatigueTimerMs)) {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()

	for _, sess := range sessions {
		sess.handleDrowningTick(ctx, now)
		sess.handleFatigueTick(ctx, now)
	}
}
