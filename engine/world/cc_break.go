package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// SpellAuraInterruptFlags bitmask values from TrinityCore SharedDefines.h:925-955
const (
	auraInterruptFlagHitBySpell            uint32 = 0x00000001 // removed by any damage (direct or periodic)
	auraInterruptFlagTakeDamage            uint32 = 0x00000002 // removed by damage taken
	auraInterruptFlagCast                  uint32 = 0x00000004 // removed by casting
	auraInterruptFlagMove                  uint32 = 0x00000008 // removed by moving
	auraInterruptFlagTurning               uint32 = 0x00000010 // removed by turning
	auraInterruptFlagJump                  uint32 = 0x00000020 // removed by jumping
	auraInterruptFlagNotMounted            uint32 = 0x00000040 // removed by dismounting
	auraInterruptFlagNotSeated             uint32 = 0x00040000 // removed by standing up (AURA_INTERRUPT_FLAG_NOT_SEATED, SpellDefines.h:65)
	auraInterruptFlagTalk                  uint32 = 0x00000400 // removed by talking to an npc (AURA_INTERRUPT_FLAG_TALK, SpellDefines.h:57)
	auraInterruptFlagChangeMap             uint32 = 0x00080000 // removed by changing map (AURA_INTERRUPT_FLAG_CHANGE_MAP, SpellDefines.h:66)
	auraInterruptFlagEnterPvPCombat        uint32 = 0x00800000 // removed by entering pvp combat (AURA_INTERRUPT_FLAG_ENTER_PVP_COMBAT, SpellDefines.h:70)
	auraInterruptFlagDirectDamage          uint32 = 0x01000000 // removed only by direct damage (AURA_INTERRUPT_FLAG_DIRECT_DAMAGE, SpellDefines.h:71)
	auraInterruptFlagLanding               uint32 = 0x02000000 // removed on landing
	auraInterruptFlagSpellAttack           uint32 = 0x00002000 // AURA_INTERRUPT_FLAG_SPELL_ATTACK (SpellDefines.h:60)
	auraInterruptFlagMeleeAttack           uint32 = 0x00001000 // AURA_INTERRUPT_FLAG_MELEE_ATTACK (SpellDefines.h:59): removed by attacking
	auraInterruptFlagImmuneOrLostSelection uint32 = 0x00100000 // AURA_INTERRUPT_FLAG_IMMUNE_OR_LOST_SELECTION (SpellDefines.h:67)
	auraInterruptFlagMount                 uint32 = 0x00020000 // removed by mounting (AURA_INTERRUPT_FLAG_MOUNT, SpellDefines.h:64)
	auraInterruptFlagLeaveCombat           uint32 = 0x80000000 // removed by leaving combat (AURA_INTERRUPT_FLAG_LEAVE_COMBAT, SpellDefines.h:73)
)

// Spell attribute from TrinityCore src/server/shared/SharedDefines.h:574.
const (
	spellAttr4DamageDoesntBreakAuras uint32 = 0x00004000 // SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS: the spell's damage does not strip TAKE_DAMAGE-interrupt auras
)

// tickDamageBreaksAuras mirrors the Unit::DealDamage gate (Unit.cpp:743):
// a damaging spell skips the TAKE_DAMAGE aura strip only when the spell
// carries SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS. Spells with no DBC row
// take the strip — the no-data regime carries no attribute data, so the
// gate only ever suppresses the strip once Spell.dbc is loaded.
func tickDamageBreaksAuras(srv *Server, spellID uint32) bool {
	if srv == nil || srv.Data == nil {
		return true
	}
	if sp, found, err := srv.Data.Spell(spellID); err == nil && found {
		return sp.AttributesEx4&spellAttr4DamageDoesntBreakAuras == 0
	}
	return true
}

// isExceptedSpellID mirrors the `except` parameter of
// Unit::RemoveAurasWithInterruptFlags: the aura of the spell that caused the
// removal is kept.
func isExceptedSpellID(spellID uint32, except []uint32) bool {
	for _, id := range except {
		if id != 0 && id == spellID {
			return true
		}
	}
	return false
}

// getSpellAuraInterruptFlags returns the aura interrupt bitmask from Spell.dbc with
// hardcoded fallbacks for primary crowd-control and utility spells if DBC is unavailable.
func getSpellAuraInterruptFlags(spellID uint32, dbcFlags uint32) uint32 {
	if dbcFlags != 0 {
		return dbcFlags
	}
	switch spellID {
	case 1776: // Gouge (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 6770, 2070, 11297: // Sap (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 2094: // Blind (Rogue)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 118, 12824, 12825, 12826, 28271, 28272, 61305, 61721, 61780: // Polymorph (Mage)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 1499, 14308, 14309, 3355: // Freezing Trap (Hunter)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 20066: // Repentance (Paladin)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 2637, 18657, 18658: // Hibernate (Druid)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 9484, 9485, 10955: // Shackle Undead (Priest)
		return auraInterruptFlagTakeDamage | auraInterruptFlagDirectDamage
	case 1784: // Stealth (Rogue)
		return auraInterruptFlagTakeDamage
	case 5215: // Prowl (Druid)
		return auraInterruptFlagTakeDamage
	case 58984: // Shadowmeld (Night Elf)
		return auraInterruptFlagTakeDamage | auraInterruptFlagMove
	case 430, 431, 432, 1133, 1135, 1137: // Food / Drink
		return auraInterruptFlagMove | auraInterruptFlagTakeDamage | auraInterruptFlagCast
	default:
		return 0
	}
}

// isBreakableCCAuraType mirrors the AuraEffect::HandleProc switch
// (SpellAuraEffects.cpp:1017-1024): the CC aura types whose own amount
// drains on damage procs via HandleBreakableCCAuraProc.
func isBreakableCCAuraType(auraType uint32) bool {
	switch auraType {
	case 5, 7, 12, 26, 56: // MOD_CONFUSE, MOD_FEAR, MOD_STUN, MOD_ROOT, TRANSFORM
		return true
	}
	return false
}

// drainBreakableCCAura mirrors AuraEffect::HandleBreakableCCAuraProc
// (SpellAuraEffects.cpp:5644-5653): per damage hit, a breakable-CC aura's
// own effect amount drops by the damage dealt; at zero or below the aura
// breaks. drained=false with remove=false means the spell carries no
// breakable-CC effect or lacks the taken proc flag — C++ runs no drain
// there (the aura breaks only via interrupt flags, handled by the caller).
// Multi-effect CC auras drain each CC effect and break when the first
// drained effect hits zero (single-effect auras: exact).
func drainBreakableCCAura(aura *activeAura, spell wotlk.Spell, wantProcFlag uint32, dmg uint32) (drained, remove bool) {
	if aura == nil || spell.ProcTypeMask&wantProcFlag == 0 {
		return false, false
	}
	first := -1
	for idx := range spell.Effects {
		eff := &spell.Effects[idx]
		if eff.Effect == 0 || !isBreakableCCAuraType(eff.Aura) || idx >= len(aura.Amounts) {
			continue
		}
		if first < 0 {
			first = idx
		}
		if left := aura.Amounts[idx] - int32(dmg); left <= 0 {
			return false, true
		} else {
			aura.Amounts[idx] = left
		}
	}
	if first < 0 {
		return false, false
	}
	// Keep the primary amount in sync with the first drained effect.
	aura.Amount = uint32(aura.Amounts[first])
	return true, false
}

// procDamageAuras removes CC and break-on-damage auras (e.g. Gouge, Sap, Blind, Polymorph, Stealth)
// when the player takes damage, and drains breakable-CC aura amounts per
// AuraEffect::HandleBreakableCCAuraProc.
// Mirrors RemoveAurasWithInterruptFlags (Unit.cpp:744/897) plus the proc drain.
func (s *session) procDamageAuras(isDirectDamage bool, damage ...uint32) {
	flags := auraInterruptFlagTakeDamage | auraInterruptFlagHitBySpell
	if isDirectDamage {
		flags |= auraInterruptFlagDirectDamage
	}
	s.removeAurasWithInterruptFlags(flags)

	dmg := uint32(0)
	if len(damage) > 0 {
		dmg = damage[0]
	}
	if dmg == 0 {
		return
	}

	// The taken proc flag kind follows the damage kind (C++ proc-flag
	// matching: direct damage -> PROC_FLAG_TAKEN_DAMAGE, otherwise
	// PROC_FLAG_TAKEN_PERIODIC). C++ has no max-HP threshold model for
	// CC breaks — the 15%/10% accumulation this replaces was a Go
	// invention (the cited Unit::ProcDamageAndSpellFor does not exist in
	// this TrinityCore fork).
	wantProcFlag := procFlagTakenPeriodic
	if isDirectDamage {
		wantProcFlag = procFlagTakenDamage
	}
	s.drainBreakableCCAurasOnTakenDamage(wantProcFlag, dmg)
}

// procSpellDirectDamageAuras mirrors the two Unit::DealDamage strip arms for
// spell-caused direct damage (Unit.cpp:743-748 + 897-898): the TAKE_DAMAGE
// strip — skipped when the damaging spell carries
// SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS — and the DIRECT_DAMAGE strip, both
// excepting the damaging spell's own aura; then the breakable-CC drain with
// the taken-damage proc flag. HITBYSPELL rides along with no exception:
// Spell::PreprocessSpellHit (Spell.cpp:2743-2744) strips it at spell-hit
// time on the player-cast path, and the pet/creature-spell funnels never
// model that hit arm, so the damage-time strip is their only coverage.
func (s *session) procSpellDirectDamageAuras(spellID uint32, damage ...uint32) {
	if tickDamageBreaksAuras(s.server, spellID) {
		s.removeAurasWithInterruptFlags(auraInterruptFlagTakeDamage, spellID)
	}
	s.removeAurasWithInterruptFlags(auraInterruptFlagDirectDamage, spellID)
	s.removeAurasWithInterruptFlags(auraInterruptFlagHitBySpell)

	dmg := uint32(0)
	if len(damage) > 0 {
		dmg = damage[0]
	}
	if dmg == 0 {
		return
	}
	s.drainBreakableCCAurasOnTakenDamage(procFlagTakenDamage, dmg)
}

// procCreatureDirectDamageAuras mirrors the Unit::DealDamage strip arms for
// direct damage on a creature victim (Unit.cpp:743-748 + 897-898): the
// TAKE_DAMAGE strip (attr4-gated, excepting the damaging spell) and the
// DIRECT_DAMAGE strip (excepting the damaging spell), then the breakable-CC
// drain with the taken-damage proc flag. spellID 0 is the melee arm
// (spellProto null in C++): both strips run with no exception. The
// HITBYSPELL strip rides along for spell hits only:
// Spell::PreprocessSpellHit (Spell.cpp:2743-2744) strips it at hit time on
// every non-miss spell hit — melee never passes through that arm — and the
// pet funnel never models the hit arm, so this is its only coverage
// (ungated, no exception, like C++).
func (s *Server) procCreatureDirectDamageAuras(key creatureAuraKey, spellID uint32, damage uint32, maxHealth uint32) {
	if s != nil {
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
		s.motionMu.Unlock()
		if m != nil {
			if tickDamageBreaksAuras(s, spellID) {
				s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagTakeDamage, spellID)
			}
			s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagDirectDamage, spellID)
			if spellID != 0 {
				s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagHitBySpell)
			}
		}
	}
	s.procCreatureDamageAuras(key, true, damage, maxHealth)
}

// stripRecursiveTakeDamageAuras mirrors the first Unit::DealDamage strip arm
// (Unit.cpp:738-750) for recursive direct-damage strikes that never pass
// through a spell hit (damage shields, split-damage redirection): the
// TAKE_DAMAGE strip — skipped when the striking spell carries
// SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS, excepting the striking spell's own
// aura. No HITBYSPELL strip: C++ never strips it on this path (no spell hit
// happens; Spell.cpp:2747 fires only in DoSpellHitOnUnit).
func (s *session) stripRecursiveTakeDamageAuras(spellID uint32) {
	if tickDamageBreaksAuras(s.server, spellID) {
		s.removeAurasWithInterruptFlags(auraInterruptFlagTakeDamage, spellID)
	}
}

// stripRecursiveDirectDamageAuras mirrors the second Unit::DealDamage strip
// arm (Unit.cpp:897-898) plus the taken proc drain for recursive
// direct-damage strikes: the DIRECT_DAMAGE strip excepting the striking
// spell's own aura, then the breakable-CC drain with the taken-damage proc
// flag (AuraEffect::HandleProc, SpellAuraEffects.cpp:1017-1025).
func (s *session) stripRecursiveDirectDamageAuras(spellID uint32, damage uint32) {
	s.removeAurasWithInterruptFlags(auraInterruptFlagDirectDamage, spellID)
	if damage > 0 {
		s.drainBreakableCCAurasOnTakenDamage(procFlagTakenDamage, damage)
	}
}

// stripCreatureRecursiveTakeDamageAuras is the creature-victim twin of
// stripRecursiveTakeDamageAuras (Unit.cpp:738-750).
func (s *Server) stripCreatureRecursiveTakeDamageAuras(m *creatureMotion, spellID uint32) {
	if s == nil || m == nil {
		return
	}
	if tickDamageBreaksAuras(s, spellID) {
		s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagTakeDamage, spellID)
	}
}

// stripCreatureRecursiveDirectDamageAuras is the creature-victim twin of
// stripRecursiveDirectDamageAuras (Unit.cpp:897-898 + the taken proc drain).
func (s *Server) stripCreatureRecursiveDirectDamageAuras(key creatureAuraKey, spellID uint32, damage uint32, maxHealth uint32) {
	if s != nil {
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
		s.motionMu.Unlock()
		if m != nil {
			s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagDirectDamage, spellID)
		}
	}
	s.procCreatureDamageAuras(key, true, damage, maxHealth)
}

// drainBreakableCCAurasOnTakenDamage runs the breakable-CC amount drain on
// the session's active auras per AuraEffect::HandleBreakableCCAuraProc
// (SpellAuraEffects.cpp:5644-5653): per damage hit, a breakable-CC aura's
// own effect amount drops by the damage dealt, and the aura breaks at zero.
func (s *session) drainBreakableCCAurasOnTakenDamage(wantProcFlag uint32, dmg uint32) {
	var toRemove []uint32
	s.castMu.Lock()
	if s.activeAuras != nil && s.server != nil && s.server.Data != nil {
		for spellID, aura := range s.activeAuras {
			if aura == nil || aura.Stopped {
				continue
			}
			sp, found, err := s.server.Data.Spell(spellID)
			if err != nil || !found {
				continue
			}
			if _, remove := drainBreakableCCAura(aura, sp, wantProcFlag, dmg); remove {
				toRemove = append(toRemove, spellID)
			}
		}
	}
	s.castMu.Unlock()

	for _, spellID := range toRemove {
		s.removeAura(spellID)
	}
}

// procPeriodicTickDamageAuras mirrors the Unit::DealDamage victim arm
// (Unit.cpp:738-750) for a periodic (DoT) tick: the strip covers
// AURA_INTERRUPT_FLAG_TAKE_DAMAGE only — HITBYSPELL was already stripped at
// spell-hit time (Spell.cpp:2743-2744) — excludes the ticking spell's own
// aura, and is skipped entirely when the tick spell carries
// SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS (Unit.cpp:743). The strip runs even
// on zero-damage ticks (it precedes the !damage early-return in C++); the
// breakable-CC drain follows with the taken-periodic proc flag.
func (s *session) procPeriodicTickDamageAuras(spellID uint32, damage uint32) {
	if tickDamageBreaksAuras(s.server, spellID) {
		s.removeAurasWithInterruptFlags(auraInterruptFlagTakeDamage, spellID)
	}
	if damage == 0 {
		return
	}
	s.drainBreakableCCAurasOnTakenDamage(procFlagTakenPeriodic, damage)
}

// procCreatureDamageAuras drains breakable-CC aura amounts on creatures per
// AuraEffect::HandleBreakableCCAuraProc (SpellAuraEffects.cpp:5644-5653),
// mirroring the player path above.
func (s *Server) procCreatureDamageAuras(key creatureAuraKey, isDirectDamage bool, damage uint32, maxHealth uint32) {
	if s == nil || key.GUID == 0 || damage == 0 {
		return
	}

	wantProcFlag := procFlagTakenPeriodic
	if isDirectDamage {
		wantProcFlag = procFlagTakenDamage
	}

	var toRemove []uint32
	s.auraMu.Lock()
	if s.activeCreatureAuras != nil && s.Data != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok && auras != nil {
			for spellID, aura := range auras {
				if aura == nil || aura.Stopped {
					continue
				}
				sp, found, err := s.Data.Spell(spellID)
				if err != nil || !found {
					continue
				}
				if _, remove := drainBreakableCCAura(aura, sp, wantProcFlag, damage); remove {
					toRemove = append(toRemove, spellID)
				}
			}
		}
	}
	s.auraMu.Unlock()

	for _, spellID := range toRemove {
		s.removeCreatureAura(key, spellID)
	}
}

// procCastAuras removes auras broken by casting an active spell (e.g. Food, Drink, Stealth).
func (s *session) procCastAuras() {
	s.removeAurasWithInterruptFlags(auraInterruptFlagCast)
}

// removeCreatureAurasWithInterruptFlags mirrors Unit::RemoveAurasWithInterruptFlags
// for creature motions: drops the motion's activeCreatureAuras whose spell
// interrupt flags intersect the mask. exceptSpellID mirrors the C++ `except`
// parameter — the aura of the spell that caused the removal is kept.
// Serves the Spell::prepare stealth-break
// arm (Spell.cpp:3180-3191, AURA_INTERRUPT_FLAG_CAST and
// AURA_INTERRUPT_FLAG_SPELL_ATTACK) on the creature and pet cast paths.
// Collects under auraMu, then removes outside it — removeCreatureAura locks
// auraMu itself.
func (s *Server) removeCreatureAurasWithInterruptFlags(m *creatureMotion, flags uint32, exceptSpellID ...uint32) {
	if s == nil || m == nil || flags == 0 {
		return
	}
	key := creatureAuraKeyForMotion(m)
	var toRemove []uint32
	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			for spellID, aura := range auras {
				if aura == nil {
					continue
				}
				if isExceptedSpellID(spellID, exceptSpellID) {
					continue
				}
				if getSpellAuraInterruptFlags(spellID, aura.AuraInterruptFlags)&flags != 0 {
					toRemove = append(toRemove, spellID)
				}
			}
		}
	}
	s.auraMu.Unlock()
	for _, spellID := range toRemove {
		s.removeCreatureAura(key, spellID)
	}
}
