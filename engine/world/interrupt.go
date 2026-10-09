package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellEffectInterruptCast      = 68   // SPELL_EFFECT_INTERRUPT_CAST (SharedDefines.h:879)
	spellInterruptFlagInterrupt   = 0x01 // SPELL_INTERRUPT_FLAG_INTERRUPT (SharedDefines.h:1120)
	channelInterruptFlagInterrupt = 0x08 // CHANNEL_INTERRUPT_FLAG_INTERRUPT (SharedDefines.h:1126)
	spellPreventionTypeSilence    = 1    // SPELL_PREVENTION_TYPE_SILENCE (SharedDefines.h:1134)
	spellPreventionTypePacify     = 2    // SPELL_PREVENTION_TYPE_PACIFY (SharedDefines.h:1135)
	spellFailedInterrupted        = 40   // SPELL_FAILED_INTERRUPTED (SharedDefines.h:1023)
	spellFailedMoving             = 51   // SPELL_FAILED_MOVING (SharedDefines.h:1033)
	spellFailedSpellUnavailable   = 107  // SPELL_FAILED_SPELL_UNAVAILABLE (SharedDefines.h:1089)
)

// isInterruptSpell identifies primary interrupt spells that abort active casts and lock spell schools.
func isInterruptSpell(spellID uint32) bool {
	switch spellID {
	case 1766, // Kick (Rogue)
		2139,  // Counterspell (Mage)
		6552,  // Pummel (Warrior)
		72,    // Shield Bash (Warrior)
		19244, // Spell Lock (Warlock Felhunter)
		47528, // Mind Freeze (Death Knight)
		57994: // Wind Shear (Shaman)
		return true
	default:
		return false
	}
}

// getInterruptDuration resolves the lockout duration in milliseconds for an interrupt spell.
func (s *session) getInterruptDuration(spell wotlk.Spell) uint32 {
	if s.server != nil && s.server.Data != nil && spell.DurationIndex > 0 {
		lvl := uint32(80)
		if s.player != nil && s.player.Level > 0 {
			lvl = uint32(s.player.Level)
		}
		if dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, lvl); err == nil && ok && dur > 0 {
			return uint32(dur)
		}
	}
	switch spell.ID {
	case 2139: // Counterspell (8s)
		return 8000
	case 72: // Shield Bash (6s)
		return 6000
	case 1766: // Kick (5s)
		return 5000
	case 6552, 47528: // Pummel, Mind Freeze (4s)
		return 4000
	case 19244: // Spell Lock (3s)
		return 3000
	case 57994: // Wind Shear (2s)
		return 2000
	default:
		return 4000
	}
}

// handleEffectInterruptCast processes SPELL_EFFECT_INTERRUPT_CAST (68) and interrupts active target casts/channels.
// Mirrors TrinityCore Spell::EffectInterruptCast (SpellEffects.cpp:3508-3551), with the school lockout
// landing through SpellHistory::LockSpellSchool (SpellHistory.cpp:517-570).
func (s *session) handleEffectInterruptCast(ctx context.Context, targetGUID uint64, interruptSpell wotlk.Spell, eff wotlk.SpellEffect) {
	if targetGUID == 0 || s.server == nil {
		return
	}

	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		return
	}
	// Spell::EffectInterruptCast (SpellEffects.cpp:3511-3512): dead targets
	// are not interrupted.
	if targetSess.player.Health == 0 {
		return
	}

	// Spell::EffectInterruptCast loops CURRENT_FIRST_NON_MELEE_SPELL up to
	// CURRENT_AUTOREPEAT_SPELL (SpellEffects.cpp:3518-3520): the generic cast
	// and the channeled spell are evaluated independently, so one interrupt
	// can stop both, each locking its own school below. (The
	// SPELL_STATE_PREPARING + GetCastTime() > 0 leg at 3527-3529 is
	// structural: Go's activeCast is created at cast start for castTime > 0
	// — spells.go:1899 — so preparing casts are already covered by
	// activeCast != nil, and instant casts have no preparing state in C++
	// either.)
	type interruptedCast struct {
		schoolMask uint32
		spellID    uint32
	}
	var interrupted []interruptedCast

	// 1. Check target's active cast (CURRENT_GENERIC_SPELL).
	targetSess.castMu.Lock()
	if targetSess.activeCast != nil {
		curCastID := targetSess.activeCast.CastID
		curSpellID := targetSess.activeCast.SpellID
		canInterrupt := true
		schoolMask := uint32(1)

		if targetSess.activeCast.InterruptFlg != 0 && (targetSess.activeCast.InterruptFlg&spellInterruptFlagInterrupt == 0) {
			canInterrupt = false
		}
		if s.server != nil && s.server.Data != nil {
			if curSpellInfo, found, _ := s.server.Data.Spell(curSpellID); found {
				if curSpellInfo.InterruptFlags != 0 && (curSpellInfo.InterruptFlags&spellInterruptFlagInterrupt == 0) {
					canInterrupt = false
				}
				// SpellEffects.cpp:3530-3532: the interrupted spell must be
				// preventable by silence; without the DBC row the gate
				// cannot be evaluated and the cast stays interruptible.
				if curSpellInfo.PreventionType != spellPreventionTypeSilence {
					canInterrupt = false
				}
				if curSpellInfo.SchoolMask != 0 {
					schoolMask = curSpellInfo.SchoolMask
				}
			}
		}

		if canInterrupt {
			if targetSess.activeCast.Timer != nil {
				targetSess.activeCast.Timer.Stop()
			}
			targetSess.activeCast.Cancelled = true
			targetSess.activeCast = nil
			interrupted = append(interrupted, interruptedCast{schoolMask: schoolMask, spellID: curSpellID})
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(curCastID, curSpellID, spellFailedInterrupted), true)
			targetSess.sendInterrupted(curCastID, curSpellID, 0)
		}
	}
	targetSess.castMu.Unlock()

	// 2. Check target's active channel (CURRENT_CHANNELED_SPELL), independent
	// of the generic arm above.
	interruptedChannel := false
	targetSess.castMu.Lock()
	if targetSess.activeChannel != nil {
		curCastID := targetSess.activeChannel.CastID
		curSpellID := targetSess.activeChannel.SpellID
		canInterrupt := true
		schoolMask := uint32(1)

		if s.server.Data != nil {
			if curSpellInfo, found, _ := s.server.Data.Spell(curSpellID); found {
				if curSpellInfo.ChannelInterrupt != 0 && (curSpellInfo.ChannelInterrupt&channelInterruptFlagInterrupt == 0) {
					canInterrupt = false
				}
				// Same PreventionType gate as the generic arm above
				// (SpellEffects.cpp:3530-3532).
				if curSpellInfo.PreventionType != spellPreventionTypeSilence {
					canInterrupt = false
				}
				if curSpellInfo.SchoolMask != 0 {
					schoolMask = curSpellInfo.SchoolMask
				}
			}
		}

		if canInterrupt {
			channel := targetSess.activeChannel
			targetSess.activeChannel = nil
			if channel.Timer != nil {
				channel.Timer.Stop()
			}
			if channel.TickTimer != nil {
				channel.TickTimer.Stop()
			}
			channel.Stopped = true
			interrupted = append(interrupted, interruptedCast{schoolMask: schoolMask, spellID: curSpellID})
			interruptedChannel = true
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(curCastID, curSpellID, spellFailedInterrupted), true)
			targetSess.sendInterrupted(curCastID, curSpellID, 0)
		}
	}
	targetSess.castMu.Unlock()
	if interruptedChannel {
		targetSess.sendChannelUpdate(0)
	}

	// 3. Log and lock the school of each interrupted spell
	// (SpellEffects.cpp:3536-3537, 3547-3549). Not bridged: the lockout
	// duration's victim-side mechanic/dispel duration mods
	// (unitTarget->ModSpellDuration, Object.cpp:2354-2400) — Go has no
	// victim-side duration-mod application model (applySpellMod is the
	// caster-side Player::ApplySpellMod, the wrong model here); and the
	// ProcSkillsAndAuras interrupt procs (SpellEffects.cpp:3540-3545) — Go
	// has no unit-aura proc trigger model (procs.go is item/enchant only;
	// procHitInterrupt is defined but unwired).
	if len(interrupted) > 0 {
		durationMs := s.getInterruptDuration(interruptSpell)
		for _, in := range interrupted {
			s.sendInterruptCastLog(interruptSpell.ID, targetGUID, in.spellID)
			if in.schoolMask != 0 && durationMs > 0 {
				targetSess.prohibitSpellSchool(ctx, in.schoolMask, durationMs)
			}
		}
	}
}

// sendInterruptCastLog mirrors Spell::ExecuteLogEffectInterruptCast
// (Spell.cpp:4573-4578) as flushed by SendLogExecute (Spell.cpp:4523-4555):
// SMSG_SPELLLOGEXECUTE carrying the effect id, the InitEffectExecuteData
// target counter, the interrupted unit, and the interrupted spell id.
func (s *session) sendInterruptCastLog(interruptSpellID uint32, victimGUID uint64, interruptedSpellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	log := protocol.NewBuffer(32)
	log.WritePackedGUID(s.playerGUID)
	log.WriteU32(interruptSpellID)
	log.WriteU32(1)
	log.WriteU32(spellEffectInterruptCast)
	log.WriteU32(1)
	log.WritePackedGUID(victimGUID)
	log.WriteU32(interruptedSpellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), s)
	}
}

// prohibitSpellSchool applies a school lockout cooldown to all silence-preventable spells of the specified school.
// Mirrors TrinityCore SpellHistory::LockSpellSchool (SpellHistory.cpp:517-570).
func (s *session) prohibitSpellSchool(ctx context.Context, schoolMask uint32, durationMs uint32) {
	if s.player == nil || durationMs == 0 || schoolMask == 0 {
		return
	}

	cooldowns := make(map[uint32]uint32)
	nowUnix := time.Now().Unix()
	cooldownEnd := nowUnix + int64((durationMs+999)/1000)

	// Track school lockout end timestamp on session
	s.castMu.Lock()
	if s.schoolLockouts == nil {
		s.schoolLockouts = make(map[uint32]int64)
	}
	s.schoolLockouts[schoolMask] = cooldownEnd
	s.castMu.Unlock()

	if s.server != nil && s.server.Data != nil {
		for _, ls := range s.player.Spells {
			if !ls.Active {
				continue
			}
			spellInfo, found, err := s.server.Data.Spell(ls.ID)
			if err != nil || !found {
				continue
			}
			// SpellHistory::LockSpellSchool (SpellHistory.cpp:555-563):
			// cooldown-on-event spells are skipped, PreventionType must be
			// silence exactly, and only spells sharing the interrupted
			// school whose remaining cooldown is shorter than the lockout
			// are extended.
			if s.server.spellIsCooldownStartedOnEvent(spellInfo) {
				continue
			}
			if (spellInfo.SchoolMask&schoolMask != 0) && spellInfo.PreventionType == spellPreventionTypeSilence {
				foundCD := false
				for i := range s.player.Cooldowns {
					if s.player.Cooldowns[i].Spell == ls.ID {
						if s.player.Cooldowns[i].End < cooldownEnd {
							s.player.Cooldowns[i].End = cooldownEnd
							cooldowns[ls.ID] = durationMs
						}
						foundCD = true
						break
					}
				}
				if !foundCD {
					cooldowns[ls.ID] = durationMs
					s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: ls.ID, End: cooldownEnd})
				}
			}
		}
	}

	if len(cooldowns) > 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldownMulti(s.playerGUID, cooldowns), true)
	}
}

// isSchoolLocked returns true if the spell's school is currently locked out by an interrupt.
// Mirrors TrinityCore Player::IsSchoolLocked.
func (s *session) isSchoolLocked(spell wotlk.Spell) bool {
	if spell.SchoolMask == 0 {
		return false
	}
	if spell.PreventionType == spellPreventionTypePacify {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if len(s.schoolLockouts) == 0 {
		return false
	}
	nowUnix := time.Now().Unix()
	for mask, lockoutEnd := range s.schoolLockouts {
		if lockoutEnd > nowUnix && (mask&spell.SchoolMask != 0) {
			return true
		}
	}
	return false
}

// buildSpellCooldownMulti packs multiple spell cooldowns into a single SMSG_SPELL_COOLDOWN packet.
// Mirrors TrinityCore Unit::BuildCooldownPacket (Unit.cpp:1310-1325).
func buildSpellCooldownMulti(playerGUID uint64, cooldowns map[uint32]uint32) []byte {
	buf := protocol.NewBuffer(8 + 1 + len(cooldowns)*(4+4))
	buf.WriteU64(playerGUID)
	buf.WriteU8(0) // flags = 0
	for spellID, durationMs := range cooldowns {
		buf.WriteU32(spellID)
		buf.WriteU32(durationMs)
	}
	return buf.Bytes()
}
