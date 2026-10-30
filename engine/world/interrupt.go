package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellEffectInterruptCast      = 68   // SPELL_EFFECT_INTERRUPT_CAST (SharedDefines.h:879)
	spellInterruptFlagInterrupt   = 0x08 // SPELL_INTERRUPT_FLAG_INTERRUPT (SpellDefines.h:33)
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
	// SpellInfo::GetDuration (SpellInfo.cpp:3077-3082): the raw Duration column,
	// absolute value, no per-level scaling — the level-scaled SpellDuration
	// accessor is the aura-duration path, not this one.
	if s.server != nil && s.server.Data != nil && spell.DurationIndex > 0 {
		if dur, ok, err := s.server.Data.SpellDurationBase(spell.DurationIndex); err == nil && ok && dur > 0 {
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
		castID     uint8
		spellID    uint32
		schoolMask uint32
		channeled  bool
		channel    *activeChannelState
	}
	var interrupted []interruptedCast

	// 1. Check target's active cast (CURRENT_GENERIC_SPELL).
	targetSess.castMu.Lock()
	if targetSess.activeCast != nil {
		curCastID := targetSess.activeCast.CastID
		curSpellID := targetSess.activeCast.SpellID
		canInterrupt := true
		// Unit::InterruptSpell (Unit.cpp:3145-3146): the IsInterruptable
		// gate (m_executedCurrently, Spell.h:548) rides every interrupt —
		// a cast already executing its effects cannot be interrupted, so a
		// Kick landing mid-effect-execution must not nil the fired cast.
		if targetSess.activeCast.Executing {
			canInterrupt = false
		}
		schoolMask := uint32(1)

		// SpellEffects.cpp:3533-3535: the unconditional InterruptFlags bit
		// test — a zero InterruptFlags (or one lacking the INTERRUPT bit)
		// is not interruptible, no != 0 guard.
		if targetSess.activeCast.InterruptFlg&spellInterruptFlagInterrupt == 0 {
			canInterrupt = false
		}
		if s.server != nil && s.server.Data != nil {
			if curSpellInfo, found, _ := s.server.Data.Spell(curSpellID); found {
				if curSpellInfo.InterruptFlags&spellInterruptFlagInterrupt == 0 {
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
			// The packets ride the deferred per-spell loop below, in
			// SpellEffects.cpp:3536-3548 order (lockout -> log -> the
			// cancel's own packets).
			interrupted = append(interrupted, interruptedCast{castID: curCastID, spellID: curSpellID, schoolMask: schoolMask})
		}
	}
	targetSess.castMu.Unlock()

	// 2. Check target's active channel (CURRENT_CHANNELED_SPELL), independent
	// of the generic arm above.
	targetSess.castMu.Lock()
	if targetSess.activeChannel != nil {
		curCastID := targetSess.activeChannel.CastID
		curSpellID := targetSess.activeChannel.SpellID
		canInterrupt := true
		// Unit::InterruptSpell (Unit.cpp:3145-3146): the IsInterruptable
		// gate (m_executedCurrently, Spell.h:548) — a channel already
		// executing its tick effects cannot be interrupted.
		if targetSess.activeChannel.Executing {
			canInterrupt = false
		}
		schoolMask := uint32(1)

		if s.server.Data != nil {
			if curSpellInfo, found, _ := s.server.Data.Spell(curSpellID); found {
				// SpellEffects.cpp:3535: the ChannelInterruptFlags bit test
				// is likewise unconditional.
				if curSpellInfo.ChannelInterrupt&channelInterruptFlagInterrupt == 0 {
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
			// Packets ride the deferred per-spell loop below (lockout ->
			// log -> cancel packets), like the generic arm above.
			interrupted = append(interrupted, interruptedCast{castID: curCastID, spellID: curSpellID, schoolMask: schoolMask, channeled: true, channel: channel})
		}
	}
	targetSess.castMu.Unlock()

	// 3. Per interrupted spell, in SpellEffects.cpp:3536-3548 order:
	// SpellHistory::LockSpellSchool (the cooldown packet), the interrupt
	// log, then Unit::InterruptSpell -> Spell::cancel's own packets.
	// cancel() (Spell.cpp:3210-3238): the PREPARING arm refunds the
	// interrupted spell's global cooldown (CancelGlobalCooldown) before
	// SendInterrupted(0); the CASTING (channeled) arm starts with
	// SendChannelUpdate(0) and refunds nothing; both arms then send
	// SendInterrupted(0) before SendCastResult(SPELL_FAILED_INTERRUPTED).
	if len(interrupted) > 0 {
		durationMs := s.getInterruptDuration(interruptSpell)
		for _, in := range interrupted {
			if in.schoolMask != 0 && durationMs > 0 {
				targetSess.prohibitSpellSchool(ctx, in.schoolMask, durationMs)
			}
			s.sendInterruptCastLog(interruptSpell.ID, targetGUID, in.spellID)
			if in.channeled {
				// Spell::cancel CASTING arm + tail (Spell.cpp:3227-3238,
				// 3248-3253): the same cleanup composition as
				// interruptCurrentChannel — the channeled spell's owned
				// auras come off the caster and target, its summoned game
				// objects and dynamic objects despawn — before
				// SendChannelUpdate(0), SendInterrupted(0), and
				// SendCastResult(INTERRUPTED).
				targetSess.removeChannelGameObjects(in.spellID)
				if targetSess.server != nil {
					targetSess.server.despawnCasterDynObjects(targetSess.playerGUID, in.spellID)
				}
				if in.channel != nil {
					targetSess.expireChannelAuras(in.channel)
				}
				targetSess.sendChannelUpdate(0)
			} else {
				targetSess.cancelGlobalCooldown(in.spellID)
			}
			targetSess.sendInterrupted(in.castID, in.spellID, 0)
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(in.castID, in.spellID, spellFailedInterrupted), true)
			// Spell::cancel tail (Spell.cpp:3248-3250): the RemoveDynObject
			// arm has no channeled gate — cancelling a recast mid-cast
			// also removes the previous cast's dynamic object of the same
			// spell (interruptCurrentCast carries the same leg).
			if !in.channeled && targetSess.server != nil {
				targetSess.server.despawnCasterDynObjects(targetSess.playerGUID, in.spellID)
			}
		}
	}
	// Not bridged: the lockout duration's victim-side mechanic/dispel
	// duration mods (unitTarget->ModSpellDuration, Object.cpp:2354-2400) —
	// Go has no victim-side duration-mod application model (applySpellMod
	// is the caster-side Player::ApplySpellMod, the wrong model here); and
	// the ProcSkillsAndAuras interrupt procs (SpellEffects.cpp:3540-3545) —
	// Go has no unit-aura proc trigger model (procs.go is item/enchant
	// only; procHitInterrupt is defined but unwired).
}

// interruptCreatureSpellGate evaluates EffectInterruptCast's victim-spell
// gates (SpellEffects.cpp:3530-3535): the victim spell must be preventable
// by silence, and the INTERRUPT bit must be set — InterruptFlags 0x08
// for the generic arm, ChannelInterruptFlags 0x08 for the channeled arm.
// A missing DBC row leaves the gates unevaluable; like the session arm
// above, the cast then stays interruptible.
func (s *Server) interruptCreatureSpellGate(spellID uint32, channeled bool) bool {
	if s == nil || s.Data == nil {
		return true
	}
	sp, found, err := s.Data.Spell(spellID)
	if err != nil || !found {
		return true
	}
	if sp.PreventionType != spellPreventionTypeSilence {
		return false
	}
	if channeled {
		return sp.ChannelInterrupt&channelInterruptFlagInterrupt != 0
	}
	return sp.InterruptFlags&spellInterruptFlagInterrupt != 0
}

// interruptedCreatureCast is one Spell::cancel victim arm: the generic
// (PREPARING deferral window) or channeled (CASTING) spell an interrupt
// stopped, so callers can send the log first and the cancel packets
// after, in SpellEffects.cpp:3536-3548 order.
type interruptedCreatureCast struct {
	spellID   uint32
	channeled bool
}

// interruptCreatureCast mirrors Spell::EffectInterruptCast
// (SpellEffects.cpp:3511-3551) for a creature victim, returning the
// interrupted arms in C++ loop order (generic, then channeled) so
// callers can log each and then send its Spell::cancel packets. The
// generic arm covers the deferral window: a twin cast-time cast
// (PendingCastSpellID) or a pending inline cast (PendingInlineSpellID)
// — C++ SPELL_STATE_PREPARING with cast time > 0. The channeled arm
// covers a live channel stamp (CURRENT_CHANNELED_SPELL,
// SPELL_STATE_CASTING). Dead victims skip (SpellEffects.cpp:3519-3520).
// Takes no locks itself; callers follow the ATTR7 lookup-then-operate
// pattern. Deltas, matching the standing creature model: no SpellHistory
// school lockout (no creature lockout registry), no AI
// OnSpellCastFinished hook (Unit.cpp:3171-3172), and no IsInterruptable
// executing gate — Go creature casts execute synchronously, so no
// executing window exists to protect. A CastingUntil window with no
// pending spell ID — the triggered-cast stamp quirk, whose spell cannot
// be gate-checked — is left alone, as are infinite-duration channels,
// which stamp no channel state at all.
func (s *Server) interruptCreatureCast(m *creatureMotion) []interruptedCreatureCast {
	if s == nil || m == nil || m.Health == 0 {
		return nil
	}
	var interrupted []interruptedCreatureCast
	var genericSpellID uint32
	if m.PendingInlineSpellID != 0 {
		genericSpellID = m.PendingInlineSpellID
	} else if m.PendingCastSpellID != 0 {
		genericSpellID = m.PendingCastSpellID
	}
	if genericSpellID != 0 && s.interruptCreatureSpellGate(genericSpellID, false) {
		s.cancelPendingCreatureCast(m)
		if !s.creatureChannelLive(m) {
			m.CastingUntil = time.Time{}
		}
		interrupted = append(interrupted, interruptedCreatureCast{spellID: genericSpellID})
	}
	if s.creatureChannelLive(m) && s.interruptCreatureSpellGate(m.ChannelSpellID, true) {
		channelSpellID := m.ChannelSpellID
		m.CastingUntil = time.Time{}
		m.ChannelSpellID = 0
		m.ChannelTargetGUID = 0
		m.ChannelHasDest = false
		m.ChannelDestX, m.ChannelDestY, m.ChannelDestZ = 0, 0, 0
		m.ChannelUntil = time.Time{}
		s.cancelPendingCreatureCast(m)
		interrupted = append(interrupted, interruptedCreatureCast{spellID: channelSpellID, channeled: true})
	}
	return interrupted
}

// interruptCreatureTargetCast applies EffectInterruptCast to a creature
// victim of a session-cast interrupt spell, logging each interrupted
// spell through the caster session like the player-victim arm.
func (s *session) interruptCreatureTargetCast(targetGUID uint64, interruptSpell wotlk.Spell) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	vm := s.findCreatureMotion(targetGUID)
	if vm == nil {
		return
	}
	for _, interrupted := range s.server.interruptCreatureCast(vm) {
		s.sendInterruptCastLog(interruptSpell.ID, targetGUID, interrupted.spellID)
		s.server.sendCreatureInterruptPackets(vm, interrupted)
	}
}

// sendCreatureInterruptPackets mirrors the packet tail of Spell::cancel
// (Spell.cpp:3210-3238) for a creature victim, sent by callers after the
// interrupt log in SpellEffects.cpp:3536-3548 order. PREPARING sends
// SendInterrupted(0) only; CASTING sends SendChannelUpdate(0) first,
// then SendInterrupted(0). SendInterrupted has no caster-type gate
// (Spell.cpp:4624-4639: SMSG_SPELL_FAILURE + SMSG_SPELL_FAILED_OTHER to
// the caster's set), so creature victims get both, with cast count 0
// per the server-side GO convention. SendCastResult sends nothing here:
// the member arm returns early for non-player casters
// (Spell.cpp:4160-4166), so no SMSG_CAST_FAILED rides a creature
// interrupt. Delivery is the visible set around the victim, the
// SendMessageToSet shape; a creature victim has no session of its own.
func (s *Server) sendCreatureInterruptPackets(m *creatureMotion, interrupted interruptedCreatureCast) {
	if s == nil || m == nil || interrupted.spellID == 0 {
		return
	}
	if interrupted.channeled {
		channelPkt := protocol.NewBuffer(12)
		channelPkt.WritePackedGUID(m.GUID)
		channelPkt.WriteU32(0)
		s.broadcastToVisibleSet(uint16(protocol.OpcodeMSG_CHANNEL_UPDATE), channelPkt.Bytes(), m.Map, m.InstanceID, m.X, m.Y, m.Z, nil)
	}
	failure := protocol.BuildSpellFailure(m.GUID, 0, interrupted.spellID, 0)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELL_FAILURE), failure, m.Map, m.InstanceID, m.X, m.Y, m.Z, nil)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELL_FAILED_OTHER), failure, m.Map, m.InstanceID, m.X, m.Y, m.Z, nil)
}

// buildInterruptCastLog packs one SMSG_SPELLLOGEXECUTE interrupt row:
// the caster, the interrupt spell, the effect id, the interrupted unit
// and the interrupted spell id.
func buildInterruptCastLog(casterGUID uint64, interruptSpellID uint32, victimGUID uint64, interruptedSpellID uint32) []byte {
	log := protocol.NewBuffer(32)
	log.WritePackedGUID(casterGUID)
	log.WriteU32(interruptSpellID)
	log.WriteU32(1)
	log.WriteU32(spellEffectInterruptCast)
	log.WriteU32(1)
	log.WritePackedGUID(victimGUID)
	log.WriteU32(interruptedSpellID)
	return log.Bytes()
}

// sendInterruptCastLog mirrors Spell::ExecuteLogEffectInterruptCast
// (Spell.cpp:4573-4578) as flushed by SendLogExecute (Spell.cpp:4523-4555):
// SMSG_SPELLLOGEXECUTE carrying the effect id, the InitEffectExecuteData
// target counter, the interrupted unit, and the interrupted spell id.
func (s *session) sendInterruptCastLog(interruptSpellID uint32, victimGUID uint64, interruptedSpellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	payload := buildInterruptCastLog(s.playerGUID, interruptSpellID, victimGUID, interruptedSpellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), payload, true)
	if s.server != nil {
		s.server.broadcastToVisibleSetFromSession(s, uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), payload)
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

	// SpellHistory::LockSpellSchool (SpellHistory.cpp:520-525) writes one
	// millisecond-exact end per set school bit, not one entry keyed by the
	// whole mask: a narrower re-lock narrows only its own bits, and the
	// per-spell cooldown entries above stay second-granular (the
	// player.Cooldowns model) while the lockout map itself is exact.
	lockoutEndMs := time.Now().UnixMilli() + int64(durationMs)
	s.castMu.Lock()
	if s.schoolLockouts == nil {
		s.schoolLockouts = make(map[uint32]int64)
	}
	for bit := uint32(0); bit < 7; bit++ {
		if schoolMask&(1<<bit) != 0 {
			s.schoolLockouts[1<<bit] = lockoutEndMs
		}
	}
	s.castMu.Unlock()

	if s.server != nil && s.server.Data != nil {
		for _, ls := range s.player.Spells {
			spellInfo, found, err := s.server.Data.Spell(ls.ID)
			if err != nil || !found {
				continue
			}
			// SpellHistory::LockSpellSchool (SpellHistory.cpp:517-570): the
			// known-spell sweep covers every non-removed spell — C++ checks
			// only state != PLAYERSPELL_REMOVED, not the active flag —
			// cooldown-on-event spells are skipped, PreventionType must be
			// silence exactly, and only spells sharing the interrupted
			// school whose remaining cooldown is shorter than the lockout
			// are extended.
			if s.server.spellIsCooldownStartedOnEvent(spellInfo) {
				continue
			}
			if (spellInfo.SchoolMask&schoolMask != 0) && spellInfo.PreventionType == spellPreventionTypeSilence {
				// SpellHistory::GetRemainingCooldown
				// (SpellHistory.cpp:494-515): an existing entry decides
				// alone (expired reads 0, never falling through to the
				// category arm); the category arm runs only with no
				// entry. A remaining cooldown at or past the lockout
				// means C++ skips the spell entirely — no entry touch, no
				// packet row.
				remainingMs := int64(0)
				entryIdx := -1
				for i := range s.player.Cooldowns {
					if s.player.Cooldowns[i].Spell == ls.ID {
						entryIdx = i
						if s.player.Cooldowns[i].End > nowUnix {
							remainingMs = (s.player.Cooldowns[i].End - nowUnix) * 1000
						}
						break
					}
				}
				if entryIdx < 0 && spellInfo.Category != 0 {
					// GetRemainingCooldown (SpellHistory.cpp:503-508)
					// reads a single _categoryCooldowns pointer — the
					// entry last AddCooldown'd for the category — not
					// the maximum over same-category entries, so the
					// last matching entry decides, expired or not.
					var lastCatEnd int64
					catFound := false
					for i := range s.player.Cooldowns {
						cd := &s.player.Cooldowns[i]
						if cd.Category == spellInfo.Category {
							lastCatEnd = cd.CategoryEnd
							catFound = true
						}
					}
					if catFound && lastCatEnd > nowUnix {
						remainingMs = (lastCatEnd - nowUnix) * 1000
					}
				}
				if remainingMs >= int64(durationMs) {
					continue
				}
				// AddCooldown(spellId, 0, lockoutEnd, 0, now) overwrites
				// the whole entry: a stale category/item/onHold tail
				// rides no longer.
				if entryIdx >= 0 {
					s.player.Cooldowns[entryIdx] = spellCooldown{Spell: ls.ID, End: cooldownEnd}
				} else {
					s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: ls.ID, End: cooldownEnd})
				}
				cooldowns[ls.ID] = durationMs
			}
		}
	}

	if len(cooldowns) > 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldownMulti(s.playerGUID, cooldowns), true)
	}
}

// isSchoolLocked returns true if the spell's school is currently locked out by an interrupt.
// SpellHistory::IsReady (SpellHistory.cpp:190-199) consults the school lockout only for
// SPELL_PREVENTION_TYPE_SILENCE spells — pacify and no-prevention spells cast through it.
func (s *session) isSchoolLocked(spell wotlk.Spell) bool {
	if spell.SchoolMask == 0 {
		return false
	}
	if spell.PreventionType != spellPreventionTypeSilence {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if len(s.schoolLockouts) == 0 {
		return false
	}
	nowMs := time.Now().UnixMilli()
	for mask, lockoutEnd := range s.schoolLockouts {
		if lockoutEnd > nowMs && (mask&spell.SchoolMask != 0) {
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
