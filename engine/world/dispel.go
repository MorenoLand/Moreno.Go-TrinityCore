package world

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	DispelNone         uint32 = 0
	DispelMagic        uint32 = 1
	DispelCurse        uint32 = 2
	DispelDisease      uint32 = 3
	DispelPoison       uint32 = 4
	DispelStealth      uint32 = 5
	DispelInvisibility uint32 = 6
	DispelAll          uint32 = 7
	DispelSpeNpcOnly   uint32 = 8
	DispelEnrage       uint32 = 9
	DispelZgTicket     uint32 = 10
	DispelOldUnused    uint32 = 11

	DispelAllMask uint32 = (1 << DispelMagic) | (1 << DispelCurse) | (1 << DispelDisease) | (1 << DispelPoison)

	spellFailedNothingToDispel uint8 = 86 // SPELL_FAILED_NOTHING_TO_DISPEL (SharedDefines.h:1068)
	spellFailedNothingToSteal  uint8 = 87 // SPELL_FAILED_NOTHING_TO_STEAL (SharedDefines.h:1069)

	spellAuraModDispelResist uint32 = 235   // SPELL_AURA_MOD_DISPEL_RESIST (SpellAuraDefines.h:315)
	spellUnholyBlight        uint32 = 50536 // DK Unholy Blight aura preventing disease dispel (Unit.cpp:4591)
)

// getDispelMask converts a DispelType to its bitmask.
// Mirrors TrinityCore SpellInfo::GetDispelMask (SpellInfo.cpp:1947-1954).
func getDispelMask(dispelType uint32) uint32 {
	if dispelType == DispelAll {
		return DispelAllMask
	}
	return 1 << dispelType
}

// buildSpellDispelLog builds SMSG_SPELLDISPELLOG (0x27B).
// Mirrors TrinityCore SpellEffects.cpp:2503-2516.
func buildSpellDispelLog(victimGUID, casterGUID uint64, dispelSpellID uint32, dispelledSpells []uint32) []byte {
	buf := protocol.NewBuffer(32 + len(dispelledSpells)*5)
	buf.WritePackedGUID(victimGUID)
	buf.WritePackedGUID(casterGUID)
	buf.WriteU32(dispelSpellID)
	buf.WriteU8(0) // not used
	buf.WriteU32(uint32(len(dispelledSpells)))
	for _, id := range dispelledSpells {
		buf.WriteU32(id)
		buf.WriteU8(0) // 0 = dispelled, !=0 cleansed
	}
	return buf.Bytes()
}

// buildDispelFailed builds SMSG_DISPEL_FAILED (0x262).
// Mirrors TrinityCore SpellEffects.cpp:2486-2493.
func buildDispelFailed(casterGUID, victimGUID uint64, dispelSpellID uint32, failedSpells []uint32) []byte {
	buf := protocol.NewBuffer(20 + len(failedSpells)*4)
	buf.WriteU64(casterGUID)
	buf.WriteU64(victimGUID)
	buf.WriteU32(dispelSpellID)
	for _, id := range failedSpells {
		buf.WriteU32(id)
	}
	return buf.Bytes()
}

// buildSpellstealLog builds SMSG_SPELLSTEALLOG (0x333).
// Mirrors TrinityCore SpellEffects.cpp:5238-5250.
func buildSpellstealLog(victimGUID, casterGUID uint64, stealSpellID uint32, stolenSpells []uint32) []byte {
	buf := protocol.NewBuffer(32 + len(stolenSpells)*5)
	buf.WritePackedGUID(victimGUID)
	buf.WritePackedGUID(casterGUID)
	buf.WriteU32(stealSpellID)
	buf.WriteU8(0) // not used
	buf.WriteU32(uint32(len(stolenSpells)))
	for _, id := range stolenSpells {
		buf.WriteU32(id)
		buf.WriteU8(0) // 0 = steals, !=0 transfers
	}
	return buf.Bytes()
}

type dispelCandidate struct {
	SpellID    uint32
	DispelType uint32
	Mechanic   uint32
	AuraType   uint32
	Slot       uint8
	Chance     int32
	Positive   bool
	DurationMs uint32
	PeriodMs   uint32
	Amount     uint32
	// CasterGUID is the aura caster's GUID (GetCaster() in the C++ backlash
	// scripts): the backlash is cast by the aura's caster, not the
	// dispeller.
	CasterGUID uint64
	// Amounts is the per-effect amount snapshot (activeAura.Amounts):
	// Amount is the last same-aura-type effect's roll, but the dispel
	// backlashes key on a specific effect index (EFFECT_0 for Unstable
	// Affliction, EFFECT_1 for Vampiric Touch).
	Amounts    [3]int32
	SchoolMask uint32
}

func (s *session) isFriendlyToPlayer(targetSess *session) bool {
	if targetSess == nil || s == targetSess {
		return true
	}
	if s.player == nil || targetSess.player == nil {
		return false
	}
	// Duel check: during active duel, opponents are hostile
	if s.player.DuelTeam != 0 && s.duelPartner == targetSess.playerGUID {
		return false
	}
	return playerTeam(s.player.Race) == playerTeam(targetSess.player.Race)
}

func (s *session) isFriendlyToTarget(targetGUID uint64, targetSess *session) bool {
	if targetGUID == s.playerGUID || targetGUID == 0 {
		return true
	}
	if targetSess != nil {
		return s.isFriendlyToPlayer(targetSess)
	}
	if s.player != nil && s.player.PetGUID != 0 && s.player.PetGUID == targetGUID {
		return true
	}
	return false
}

// dispelResistTotalLocked sums the target's SPELL_AURA_MOD_DISPEL_RESIST
// total — the applied-aura arm plus the passive-spellbook bridge — mirroring
// the second half of Aura::CalcDispelChance (SpellAuras.cpp:1228-1229).
// Assumes the target's castMu is held.
func (s *session) dispelResistTotalLocked() int32 {
	resistChance := int32(0)
	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == spellAuraModDispelResist {
			resistChance += int32(aura.Amount)
		}
	}
	if s.server != nil && s.server.Data != nil && s.player != nil {
		for _, pSpell := range s.player.Spells {
			if !pSpell.Active || pSpell.Disabled {
				continue
			}
			// The loop above already counted applied auras; C++
			// GetTotalAuraModifier (Unit.cpp:4831) counts each applied aura
			// exactly once, so a spell that is both learned and applied
			// (e.g. Pain Suppression self-cast) must not count again here.
			if aura, applied := s.activeAuras[pSpell.ID]; applied && aura != nil {
				continue
			}
			spell, found, err := s.server.Data.Spell(pSpell.ID)
			if err != nil || !found {
				continue
			}
			// C++ sums applied auras only: a learned-but-uncast non-passive
			// spell contributes nothing. Passive spells are auto-applied in
			// C++ (Player::AddSpell) but Go only records them in s.auras,
			// so the scan bridges exactly the passive gap.
			if spell.Attributes&spellAttributePassive == 0 {
				continue
			}
			for _, eff := range spell.Effects {
				if eff.Aura == spellAuraModDispelResist {
					resistChance += eff.BasePoints
				}
			}
		}
	}
	return resistChance
}

// auraDispelChance mirrors TrinityCore Aura::CalcDispelChance
// (SpellAuras.cpp:1218-1236) for one candidate aura: 100 minus the
// RoundToInterval-clamped resist total. The resist total is the aura
// caster's SPELLMOD_RESIST_DISPEL_CHANCE fold (SpellAuras.cpp:1221-1226)
// plus the target's SPELL_AURA_MOD_DISPEL_RESIST total on offensive dispels
// (SpellAuras.cpp:1228-1229). Call with no session lock held: the
// caster-mod fold takes the mod owner's castMu.
func (s *Server) auraDispelChance(mapID, instanceID uint32, aura *activeAura, resistTotal int32, offensive bool) int32 {
	resist := s.dispelResistSpellMod(mapID, instanceID, aura)
	if offensive {
		resist += resistTotal
	}
	if resist < 0 {
		resist = 0
	} else if resist > 100 {
		resist = 100
	}
	return 100 - resist
}

// spellModOwnerSession mirrors the caster half of
// WorldObject::GetSpellModOwner (Object.cpp:2246-2261): a player caster GUID
// resolves to its own session; a pet or totem creature caster resolves to its
// owner player's session; anything else (offline, unknown, plain creature)
// yields nil — the C++ null-modOwner path.
func (s *Server) spellModOwnerSession(mapID, instanceID uint32, casterGUID uint64) *session {
	if s == nil || casterGUID == 0 {
		return nil
	}
	if cs := s.findSessionByGUID(casterGUID); cs != nil && cs.player != nil {
		return cs
	}
	if m := s.findCreatureMotion(mapID, instanceID, casterGUID); m != nil && m.PetID != 0 && m.OwnerGUID != 0 {
		if os := s.findSessionByGUID(m.OwnerGUID); os != nil && os.player != nil {
			return os
		}
		return nil
	}
	if s.isTotemGUID(casterGUID) {
		s.totemMu.RLock()
		defer s.totemMu.RUnlock()
		for ownerGUID, slots := range s.activeTotems {
			for _, t := range slots {
				if t != nil && t.TotemGUID == casterGUID {
					if os := s.findSessionByGUID(ownerGUID); os != nil && os.player != nil {
						return os
					}
					return nil
				}
			}
		}
	}
	return nil
}

// dispelResistSpellMod mirrors the first arm of Aura::CalcDispelChance
// (SpellAuras.cpp:1221-1226): the aura caster's spell-mod owner folds
// SPELLMOD_RESIST_DISPEL_CHANCE mods affecting the dispelled spell,
// starting from a zero base. Caster resolution mirrors
// WorldObject::GetSpellModOwner (Object.cpp:2246-2261): a player caster is
// its own mod owner; a pet or totem creature caster resolves to its owner
// player; anything else contributes nothing.
func (s *Server) dispelResistSpellMod(mapID, instanceID uint32, aura *activeAura) int32 {
	if s == nil || s.Data == nil || aura == nil || aura.CasterGUID == 0 {
		return 0
	}
	spell, found, err := s.Data.Spell(aura.SpellID)
	if err != nil || !found {
		return 0
	}
	modOwner := s.spellModOwnerSession(mapID, instanceID, aura.CasterGUID)
	if modOwner == nil {
		return 0
	}
	return modOwner.applySpellMod(spell, spellModResistDispelChance, 0)
}

// getDispellableAuraListForPlayer returns the list of auras that can be dispelled from a player.
// Mirrors TrinityCore Unit::GetDispellableAuraList (Unit.cpp:4588-4628).
// stealMode switches to the steal-list semantics of both C++ steal paths
// (positivity-only gate, no isReflect/friendliness term); keepResisted skips
// the 100%-resist exclusion (the C++ CheckCast steal arm has no
// CalcDispelChance gate).
func (s *session) getDispellableAuraListForPlayer(targetSess *session, dispelMask uint32, isReflected, stealMode, keepResisted bool) []dispelCandidate {
	if targetSess == nil || targetSess.player == nil {
		return nil
	}

	// If target is affected by Unholy Blight (50536), cannot dispel diseases (Unit.cpp:4591).
	// The carve lives in Unit::GetDispellableAuraList, which neither steal arm calls
	// (Spell.cpp:5957-5984 pre-cast; SpellEffects.cpp:5160+ effect), so stealMode skips it.
	if !stealMode && (dispelMask&(1<<DispelDisease) != 0) && targetSess.hasAura(spellUnholyBlight) {
		dispelMask &^= (1 << DispelDisease)
	}

	isFriendly := s.isFriendlyToPlayer(targetSess)

	// The per-aura dispel chance (Aura::CalcDispelChance) folds the aura
	// caster's spell mods, which takes the caster session's lock — so the
	// chance is computed after releasing the target's lock (two passes).
	targetSess.castMu.Lock()
	offensive := !isFriendly
	resistTotal := int32(0)
	if offensive {
		resistTotal = targetSess.dispelResistTotalLocked()
	}
	var candidates []dispelCandidate
	var candAuras []*activeAura
	for spellID, aura := range targetSess.activeAuras {
		if aura == nil || aura.Stopped {
			continue
		}
		// Owned-only scan (Unit.cpp:4595, SpellEffects.cpp:5161): a
		// persistent-area aura applied by another caster's dynamic
		// object is visible on the target but never owned, so no
		// dispel or steal effect may list it. The steal pre-cast
		// (Spell.cpp:5964) scans GetVisibleAuras instead and keeps it;
		// keepResisted doubles as that pre-cast flag on the steal path.
		if aura.PersistentAreaAura && aura.CasterGUID != targetSess.playerGUID && (!stealMode || !keepResisted) {
			continue
		}
		var sp wotlk.Spell
		var found bool
		if s.server != nil && s.server.Data != nil {
			sp, found, _ = s.server.Data.Spell(spellID)
		}
		// Passive auras cannot be dispelled (Unit.cpp:4603)
		if found && sp.Attributes&spellAttributePassive != 0 {
			continue
		}

		// Auras whose dispel removes individual charges only appear in the
		// dispel list while they still have charges (Unit.cpp:4622-4625).
		if found && sp.AttributesEx7&spellAttr7DispelCharges != 0 && aura.RemainingCharges == 0 {
			continue
		}

		dispelType := aura.DispelType
		if dispelType == 0 && found {
			dispelType = sp.DispelType
		}
		if (getDispelMask(dispelType) & dispelMask) == 0 {
			continue
		}

		if stealMode {
			// Both C++ steal paths (CheckCast arm Spell.cpp:5957-5984 and
			// EffectStealBeneficialBuff SpellEffects.cpp:5169-5189) key only on
			// application positivity — the isReflect/friendliness term of
			// Unit.cpp:4608-4612 does not exist there.
			if !aura.Positive {
				continue
			}
		} else if isReflected != (isFriendly == aura.Positive) {
			// Friendly target: remove harmful debuffs (!aura.Positive)
			// Hostile target: remove beneficial buffs (aura.Positive)
			// Mirrors Unit.cpp:4608-4612; a reflected cast inverts the gate —
			// the dispeller strips one of its own beneficial buffs.
			continue
		}

		candidates = append(candidates, dispelCandidate{
			SpellID:    spellID,
			DispelType: dispelType,
			Mechanic:   aura.Mechanic,
			AuraType:   aura.AuraType,
			Slot:       aura.Slot,
			Positive:   aura.Positive,
			DurationMs: aura.DurationMs,
			PeriodMs:   aura.PeriodMs,
			Amount:     aura.Amount,
			CasterGUID: aura.CasterGUID,
			Amounts:    aura.Amounts,
			SchoolMask: aura.SchoolMask,
		})
		candAuras = append(candAuras, aura)
	}
	mapID, instanceID := targetSess.player.Map, targetSess.player.InstanceID
	targetSess.castMu.Unlock()

	// Aura::CalcDispelChance (SpellAuras.cpp:1218-1236) runs per aura.
	// 2.4.3 patch note arm (Unit.cpp:4617-4619): auras with 100% dispel
	// resistance (chance 0) never enter the dispel list — without this
	// they would always fail and land in SMSG_DISPEL_FAILED, which C++
	// never sends for them.
	kept := candidates[:0]
	for i := range candidates {
		chance := s.server.auraDispelChance(mapID, instanceID, candAuras[i], resistTotal, offensive)
		if chance <= 0 && !keepResisted {
			continue
		}
		candidates[i].Chance = chance
		kept = append(kept, candidates[i])
	}
	return kept
}

// getDispellableAuraListForCreature returns dispellable auras on a creature.
// stealMode/keepResisted carry the steal-list semantics described on
// getDispellableAuraListForPlayer.
func (s *session) getDispellableAuraListForCreature(creatureGUID uint64, dispelMask uint32, isReflected, stealMode, keepResisted bool) []dispelCandidate {
	if s.server == nil || s.player == nil {
		return nil
	}
	s.server.auraMu.Lock()

	auras := s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, creatureGUID)]
	if len(auras) == 0 {
		s.server.auraMu.Unlock()
		return nil
	}

	// Unit.cpp:4591-4592: Unholy Blight (50536) on the target kills the
	// disease bit before the scan. The carve is not applied in stealMode:
	// neither C++ steal arm calls Unit::GetDispellableAuraList.
	if !stealMode && dispelMask&(1<<DispelDisease) != 0 {
		if blight, ok := auras[spellUnholyBlight]; ok && blight != nil && !blight.Stopped {
			dispelMask &^= (1 << DispelDisease)
		}
	}

	isFriendly := false
	if s.player != nil && s.player.PetGUID != 0 && s.player.PetGUID == creatureGUID {
		isFriendly = true
	}

	// The per-aura dispel chance (Aura::CalcDispelChance) folds the aura
	// caster's spell mods, which takes the caster session's lock — so the
	// chance is computed after releasing auraMu (two passes).
	offensive := !isFriendly
	resistTotal := int32(0)
	if offensive {
		for _, ra := range auras {
			if ra != nil && !ra.Stopped && ra.AuraType == spellAuraModDispelResist {
				resistTotal += int32(ra.Amount)
			}
		}
	}
	var candidates []dispelCandidate
	var candAuras []*activeAura
	for spellID, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		// Owned-only scan, like the player leg above: another caster's
		// persistent-area aura applications are never dispelled/stolen.
		if aura.PersistentAreaAura && aura.CasterGUID != creatureGUID && (!stealMode || !keepResisted) {
			continue
		}
		var sp wotlk.Spell
		var found bool
		if s.server.Data != nil {
			sp, found, _ = s.server.Data.Spell(spellID)
		}
		if found && sp.Attributes&spellAttributePassive != 0 {
			continue
		}

		// Auras whose dispel removes individual charges only appear in the
		// dispel list while they still have charges (Unit.cpp:4622-4625).
		if found && sp.AttributesEx7&spellAttr7DispelCharges != 0 && aura.RemainingCharges == 0 {
			continue
		}

		dispelType := aura.DispelType
		if dispelType == 0 && found {
			dispelType = sp.DispelType
		}
		if (getDispelMask(dispelType) & dispelMask) == 0 {
			continue
		}

		if stealMode {
			// Steal paths (Spell.cpp:5957-5984, SpellEffects.cpp:5169-5189) key
			// only on application positivity — no isReflect/friendliness term.
			if !aura.Positive {
				continue
			}
		} else if isReflected != (isFriendly == aura.Positive) {
			// Unit.cpp:4608-4612, isReflect arm: a reflected cast strips the
			// caster's own beneficial buffs instead of cleansing debuffs.
			continue
		}

		candidates = append(candidates, dispelCandidate{
			SpellID:    spellID,
			DispelType: dispelType,
			Mechanic:   aura.Mechanic,
			AuraType:   aura.AuraType,
			Slot:       aura.Slot,
			Positive:   aura.Positive,
			DurationMs: aura.DurationMs,
			PeriodMs:   aura.PeriodMs,
			Amount:     aura.Amount,
			CasterGUID: aura.CasterGUID,
			Amounts:    aura.Amounts,
			SchoolMask: aura.SchoolMask,
		})
		candAuras = append(candAuras, aura)
	}
	mapID, instanceID := s.player.Map, s.player.InstanceID
	s.server.auraMu.Unlock()

	// Aura::CalcDispelChance (SpellAuras.cpp:1218-1236) runs per aura.
	// 2.4.3 patch note arm (Unit.cpp:4617-4619): auras at 100% resistance
	// never enter the list.
	kept := candidates[:0]
	for i := range candidates {
		chance := s.server.auraDispelChance(mapID, instanceID, candAuras[i], resistTotal, offensive)
		if chance <= 0 && !keepResisted {
			continue
		}
		candidates[i].Chance = chance
		kept = append(kept, candidates[i])
	}
	return kept
}

// creatureDispelPreCast mirrors the SPELL_EFFECT_DISPEL arm of
// Spell::CheckCast (Spell.cpp:5517-5562) for a creature caster: a spell whose
// only effects are non-area dispels, with a non-empty dispel mask, fizzles
// when its unit target carries no dispellable auras
// (SPELL_FAILED_NOTHING_TO_DISPEL). The !IsTriggered() gate (Spell.cpp:5540)
// runs for triggered casts too in Go's model only as a skip — triggered
// creature casts bypass this arm entirely. A missing or unresolvable wire
// unit target skips the arm (Spell.cpp:5541, GetUnitTarget null) and leaves
// the cast to the lost-explicit-target arm in _cast. Creature casts fizzle
// silently — there is no failure-packet path — so false means fizzle.
// Sanctuary (Spell.cpp:5545-5552): a creature caster is never in sanctuary —
// UNIT_BYTE2_FLAG_SANCTUARY is set only by Player::UpdateArea
// (Player.cpp:7179/7185), no creature setter exists — and neither are
// creature targets, so only a player target's pvpFlagSanctuary bit is read
// (inert: Go never populates it, like the player path). The duel carve-out
// needs a player caster (unitCaster->ToPlayer()), so it is vacuous here.
func (s *Server) creatureDispelPreCast(m *creatureMotion, spell wotlk.Spell, targetGUID uint64, triggered bool) bool {
	if m == nil || triggered || targetGUID == 0 {
		return true
	}
	var dispelMask uint32
	hasNonDispelEffect := false
	hasDispellableAura := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectDispel {
			// SpellEffectInfo::IsTargetingArea (SpellInfo.cpp:380-383):
			// TargetA.IsArea() || TargetB.IsArea().
			if spellImplicitTargetIsArea(eff.ImplicitTargetA) || spellImplicitTargetIsArea(eff.ImplicitTargetB) ||
				spell.AttributesEx&spellAttr1MeleeCombatStart != 0 {
				hasDispellableAura = true
				break
			}
			dispelMask |= getDispelMask(uint32(eff.MiscValue))
		} else if eff.Effect != 0 {
			hasNonDispelEffect = true
			break
		}
	}
	if hasNonDispelEffect || hasDispellableAura || dispelMask == 0 {
		return true
	}
	isFriendly := s.creatureFriendlyToTarget(m, targetGUID)
	if !isFriendly {
		if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
			if ts.player.PVPFlags&pvpFlagSanctuary != 0 {
				return false
			}
		}
	}
	// C++ CheckCast builds the list with the default isReflect=false
	// (Spell.cpp:5563) — reflection is rolled later at AddUnitTarget
	// (Spell.cpp:2152).
	return len(s.creatureCasterDispelCandidates(m, targetGUID, dispelMask, isFriendly, false)) != 0
}

// creatureCasterDispelCandidates mirrors Unit::GetDispellableAuraList
// (Unit.cpp:4588-4628) with a creature as the dispeller: it scans the
// target's auras — the player's activeAuras for a player target, the
// server's activeCreatureAuras for a creature target — skipping passives and
// applying the dispel mask, keeping only auras whose positivity opposes the
// caster-target friendliness (Unit.cpp:4608-4612). The Unholy Blight disease
// arm (Unit.cpp:4591-4592) applies to both target kinds. Dispel chance is
// Aura::CalcDispelChance (SpellAuras.cpp:1218-1236): offensive dispels
// subtract the target's SPELL_AURA_MOD_DISPEL_RESIST total, and auras at 100%
// resistance never enter the list (Unit.cpp:4614-4617). The aura-caster
// SPELLMOD_RESIST_DISPEL_CHANCE fold (SpellAuras.cpp:1224-1226) runs per
// aura via auraDispelChance, two passes so the caster-mod fold never nests
// session locks.
func (s *Server) creatureCasterDispelCandidates(m *creatureMotion, targetGUID uint64, dispelMask uint32, isFriendly bool, isReflected bool) []dispelCandidate {
	if s == nil || m == nil || targetGUID == 0 {
		return nil
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if (dispelMask&(1<<DispelDisease) != 0) && ts.hasAura(spellUnholyBlight) {
			dispelMask &^= (1 << DispelDisease)
		}
		// Two passes: the per-aura chance folds the aura caster's spell
		// mods (takes the caster session's lock), so it runs after the
		// target's lock is released.
		ts.castMu.Lock()
		offensive := !isFriendly
		resistTotal := int32(0)
		if offensive {
			resistTotal = ts.dispelResistTotalLocked()
		}
		type pendingAura struct {
			spellID uint32
			aura    *activeAura
		}
		var pending []pendingAura
		for spellID, aura := range ts.activeAuras {
			pending = append(pending, pendingAura{spellID, aura})
		}
		mapID, instanceID := ts.player.Map, ts.player.InstanceID
		ts.castMu.Unlock()
		var candidates []dispelCandidate
		for _, p := range pending {
			chance := s.auraDispelChance(mapID, instanceID, p.aura, resistTotal, offensive)
			if c := s.dispelCandidateFromAura(p.spellID, p.aura, dispelMask, isFriendly, chance, isReflected); c != nil {
				candidates = append(candidates, *c)
			}
		}
		return candidates
	}
	tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID)
	if tm == nil {
		return nil
	}
	key := creatureAuraKeyForMotion(tm)
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[key]
	if dispelMask&(1<<DispelDisease) != 0 {
		if blight, ok := auras[spellUnholyBlight]; ok && blight != nil && !blight.Stopped {
			dispelMask &^= (1 << DispelDisease)
		}
	}
	offensive := !isFriendly
	resistTotal := int32(0)
	if offensive {
		for _, ra := range auras {
			if ra != nil && !ra.Stopped && ra.AuraType == spellAuraModDispelResist {
				resistTotal += int32(ra.Amount)
			}
		}
	}
	type pendingAura struct {
		spellID uint32
		aura    *activeAura
	}
	var pending []pendingAura
	for spellID, aura := range auras {
		pending = append(pending, pendingAura{spellID, aura})
	}
	mapID, instanceID := m.Map, m.InstanceID
	s.auraMu.Unlock()
	var candidates []dispelCandidate
	for _, p := range pending {
		chance := s.auraDispelChance(mapID, instanceID, p.aura, resistTotal, offensive)
		if c := s.dispelCandidateFromAura(p.spellID, p.aura, dispelMask, isFriendly, chance, isReflected); c != nil {
			candidates = append(candidates, *c)
		}
	}
	return candidates
}

// dispelCandidateFromAura applies the per-aura terms of
// Unit::GetDispellableAuraList (Unit.cpp:4602-4625) shared by the player and
// creature scans: passive skip, dispel-mask match, the positivity-vs-
// friendliness skip (Unit.cpp:4608-4612, inverted for reflected casts —
// the dispeller strips one of its own beneficial buffs), the 100%-
// resistance exclusion (Unit.cpp:4614-4617), and the dispel-charges
// zero-charge skip (Unit.cpp:4619-4625). Returns nil when the aura cannot
// enter the list.
func (s *Server) dispelCandidateFromAura(spellID uint32, aura *activeAura, dispelMask uint32, isFriendly bool, chance int32, isReflected bool) *dispelCandidate {
	if aura == nil || aura.Stopped {
		return nil
	}
	var sp wotlk.Spell
	var found bool
	if s.Data != nil {
		sp, found, _ = s.Data.Spell(spellID)
	}
	if found && sp.Attributes&spellAttributePassive != 0 {
		return nil
	}
	if found && sp.AttributesEx7&spellAttr7DispelCharges != 0 && aura.RemainingCharges == 0 {
		return nil
	}
	dispelType := aura.DispelType
	if dispelType == 0 && found {
		dispelType = sp.DispelType
	}
	if (getDispelMask(dispelType) & dispelMask) == 0 {
		return nil
	}
	// Unit.cpp:4608-4612, isReflect arm: a reflected cast strips the
	// caster's own beneficial buffs instead of cleansing debuffs.
	if isReflected != (isFriendly == aura.Positive) {
		return nil
	}
	if chance <= 0 {
		return nil
	}
	return &dispelCandidate{
		SpellID:    spellID,
		DispelType: dispelType,
		Mechanic:   aura.Mechanic,
		AuraType:   aura.AuraType,
		Slot:       aura.Slot,
		Chance:     chance,
		Positive:   aura.Positive,
		DurationMs: aura.DurationMs,
		PeriodMs:   aura.PeriodMs,
		Amount:     aura.Amount,
		CasterGUID: aura.CasterGUID,
		Amounts:    aura.Amounts,
		SchoolMask: aura.SchoolMask,
	}
}

// steal candidate list from the dispel-mask candidate list: positive auras only,
// skipping auras whose spell carries SPELL_ATTR4_NOT_STEALABLE. Passive auras are
// already excluded by the dispel helpers (the spellAttributePassive gate).
// C++ builds both steal lists with no isReflect term (CheckCast arm
// Spell.cpp:5957-5984; EffectStealBeneficialBuff SpellEffects.cpp:5169-5189), so
// steal runs the helpers in stealMode (positivity-only gate) with isReflected
// false.
func (s *session) stealableAuraList(targetSess *session, targetGUID uint64, isTargetPlayer bool, dispelMask uint32, preCast bool) []dispelCandidate {
	var candidates []dispelCandidate
	if isTargetPlayer {
		// preCast skips the 100%-resist exclusion: the C++ CheckCast steal arm
		// (Spell.cpp:5957-5984) has no CalcDispelChance gate, while the effect
		// (SpellEffects.cpp:5181-5183) keeps it.
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask, false, true, preCast)
	} else {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask, false, true, preCast)
	}

	var stealable []dispelCandidate
	for _, c := range candidates {
		if !c.Positive {
			continue
		}
		if s.server != nil && s.server.Data != nil {
			if sp, found, _ := s.server.Data.Spell(c.SpellID); found && sp.AttributesEx4&spellAttr4NotStealable != 0 {
				continue
			}
		}
		stealable = append(stealable, c)
	}
	return stealable
}

// checkStealPreCast mirrors TrinityCore Spell::CheckCast (Spell.cpp:5957-5984) for
// SPELL_EFFECT_STEAL_BENEFICIAL_BUFF (126): no unit target or self target fails with
// SPELL_FAILED_BAD_TARGETS; a target carrying no stealable aura fails with
// SPELL_FAILED_NOTHING_TO_STEAL. The pre-cast keeps 100%-resist auras in the
// list — the C++ arm has no CalcDispelChance gate (the effect twin does).
func (s *session) checkStealPreCast(spell wotlk.Spell, targetGUID uint64) uint8 {
	for _, eff := range spell.Effects {
		if eff.Effect != 126 { // SPELL_EFFECT_STEAL_BENEFICIAL_BUFF
			continue
		}
		if targetGUID == 0 || targetGUID == s.playerGUID {
			return spellFailedBadTargets
		}
		dispelMask := getDispelMask(uint32(eff.MiscValue))

		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		isTargetPlayer := (targetSess != nil && targetSess.player != nil)
		if len(s.stealableAuraList(targetSess, targetGUID, isTargetPlayer, dispelMask, true)) == 0 {
			return spellFailedNothingToSteal
		}
	}
	return 0
}

// checkDispelPreCast mirrors TrinityCore Spell::CheckCast (Spell.cpp:5520-5565).
// Returns spellFailedNothingToDispel (86) if the spell only dispels and the
// target has nothing to dispel. The !IsTriggered() arm is structural:
// handleCastSpell serves client-initiated casts only; triggered casts go
// through castSpellDirect.
// spellImplicitTargetIsArea mirrors SpellImplicitTargetInfo::IsArea()
// (SpellInfo.cpp:71-75): the selection category is AREA or CONE. Exact
// target set from the _data table (SpellInfo.cpp:227-328) — the pure
// category test, with no radius-index proxy and no dest-target additions.
func spellImplicitTargetIsArea(target uint32) bool {
	switch target {
	case 7, 8, 15, 16, 20, 24, 30, 31, 33, 34, 37, 51, 52, 54, 56, 59, 60, 61, 93, 104, 108:
		return true
	}
	return false
}

func (s *session) checkDispelPreCast(spell wotlk.Spell, targetGUID uint64) uint8 {
	hasNonDispelEffect := false
	hasDispellableAura := false
	dispelMask := uint32(0)

	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectDispel {
			// SpellEffectInfo::IsTargetingArea (SpellInfo.cpp:380-383):
			// TargetA.IsArea() || TargetB.IsArea().
			if spellImplicitTargetIsArea(eff.ImplicitTargetA) || spellImplicitTargetIsArea(eff.ImplicitTargetB) ||
				spell.AttributesEx&spellAttr1MeleeCombatStart != 0 {
				hasDispellableAura = true
				break
			}
			dispelMask |= getDispelMask(uint32(eff.MiscValue))
		} else if eff.Effect != 0 {
			hasNonDispelEffect = true
			break
		}
	}

	if hasNonDispelEffect || hasDispellableAura || dispelMask == 0 {
		return 0
	}

	// Spell.cpp:5538: the arm keys on m_targets.GetUnitTarget(); a missing
	// wire unit target skips the whole arm — CheckCast runs before
	// SetTargetMap (Spell.cpp:3100), so there is no target-map fallback here.
	if targetGUID == 0 {
		return 0
	}

	// Target resolution
	var targetSess *session
	if targetGUID == s.playerGUID {
		targetSess = s
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	// Hostile targets in sanctuary fail outright, except when the target is
	// the caster's duel opponent (Spell.cpp:5539-5552). Player sanctuary is
	// the UNIT_BYTE2_FLAG_SANCTUARY bridge (pvpFlagSanctuary, also used by the
	// explicit-target PvP gates); creature targets carry no per-creature
	// bytes2 model in Go, so only the caster side applies for them.
	// Friendliness matches the dispel-list scan below: non-player targets
	// other than the own pet count as hostile, like
	// getDispellableAuraListForCreature does.
	if !s.isFriendlyToTarget(targetGUID, targetSess) {
		targetSanctuary := false
		if targetSess != nil && targetSess.player != nil {
			targetSanctuary = targetSess.player.PVPFlags&pvpFlagSanctuary != 0
		}
		if s.player.PVPFlags&pvpFlagSanctuary != 0 || targetSanctuary {
			if s.duelPartner == 0 || s.duelPartner != targetGUID {
				return spellFailedNothingToDispel
			}
		}
	}

	var candidates []dispelCandidate
	if targetSess != nil && targetSess.player != nil {
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask, false, false, false)
	} else if targetGUID != 0 {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask, false, false, false)
	}

	if len(candidates) == 0 {
		return spellFailedNothingToDispel
	}
	return 0
}

func isDevourMagicSpell(spellID uint32) bool {
	switch spellID {
	case 19505, 19731, 19734, 19736, 27276, 27277, 48011:
		return true
	default:
		return false
	}
}

// isUnstableAfflictionSpell returns true if the spell is Unstable Affliction (TrinityCore SpellMgr.cpp).
func isUnstableAfflictionSpell(spellID uint32) bool {
	switch spellID {
	case 30108, 30404, 30405, 47841, 47843:
		return true
	default:
		return false
	}
}

// isVampiricTouchSpell returns true if the spell is Vampiric Touch (TrinityCore SpellMgr.cpp).
func isVampiricTouchSpell(spellID uint32) bool {
	switch spellID {
	case 34914, 34916, 34917, 48159, 48160:
		return true
	default:
		return false
	}
}

// handleEffectDispel processes SPELL_EFFECT_DISPEL (38).
// Mirrors TrinityCore Spell::EffectDispel (SpellEffects.cpp:2429-2531).
// auraWireStackCount mirrors the stack-count wire field in
// AuraApplication::BuildUpdatePacket (SpellAuras.cpp:241-242): the field
// carries the spell's stack amount when the spell is stackable, else the
// charge count — stack has priority over charges (checked on retail with
// spell 50262). The full-list paths (auraUpdateRecords in player_auras.go,
// the pet load path in pets.go) and the normal re-cast refresh
// (spells.go) already apply this; the single-update dispel/steal paths
// below must too, or a partial charge change on a stackable charge aura
// shows the charge count where C++ keeps showing the stack amount.
func auraWireStackCount(aura *activeAura) uint8 {
	if aura == nil {
		return 1
	}
	if aura.StackAmount > 0 {
		if aura.StackCount == 0 {
			return 1
		}
		return aura.StackCount
	}
	return aura.RemainingCharges
}

// rescaleAuraAmountsByStack mirrors the Aura::SetStackAmount recalc
// (SpellAuras.cpp:1008) on the ModStackAmount stack-change paths: Go's aura
// amounts are linear in the stack count (a fresh apply carries one stack's
// amount; the 4c8b0e4 merge unit scales by the live count), so a surviving
// stack change rescales every effect amount by newCount/oldCount — the
// CalculateAmount x new-count term (SpellAuraEffects.cpp:537). The ratio is
// exact because the stored amounts are the base amount times oldCount.
// Charge changes never rescale (Aura::ModCharges has no recalc).
func rescaleAuraAmountsByStack(aura *activeAura, oldCount, newCount uint32) {
	if aura == nil || oldCount == 0 || newCount == 0 || oldCount == newCount {
		return
	}
	rescale := func(v int64) int64 { return v * int64(newCount) / int64(oldCount) }
	aura.Amount = uint32(rescale(int64(aura.Amount)))
	for index := range aura.Amounts {
		if aura.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		aura.Amounts[index] = int32(rescale(int64(aura.Amounts[index])))
	}
}

// dispelPlayerAuraCharge mirrors Unit::RemoveAurasDueToSpellByDispel
// (Unit.cpp:3938-3950): a successful dispel removes one charge
// (Aura::ModCharges, SpellAuras.cpp:964) from auras carrying
// SPELL_ATTR7_DISPEL_CHARGES, or one stack (Aura::ModStackAmount,
// SpellAuras.cpp:1030) otherwise; the aura survives until its last
// charge/stack is gone. Returns true when the aura was fully removed.
// Go has no AuraScript infra, so the OnDispel/AfterDispel hooks and the
// script-mutable DispelInfo charge count are vacuous (one per success).
func (ts *session) dispelPlayerAuraCharge(spellID uint32) bool {
	ts.castMu.Lock()
	aura := ts.activeAuras[spellID]
	dispelCharges := false
	if aura != nil && !aura.Stopped {
		if ts.server != nil && ts.server.Data != nil {
			if sp, found, _ := ts.server.Data.Spell(spellID); found {
				dispelCharges = sp.AttributesEx7&spellAttr7DispelCharges != 0
			}
		}
		if dispelCharges {
			// Aura::ModCharges (SpellAuras.cpp:966-985): charges decrement
			// only while the aura is using charges (m_procCharges != 0,
			// SpellAuras.cpp:441) — a zero-charge ATTR7 aura is untouched
			// by dispel/steal, with no client update. The aura is removed
			// only when the decrement reaches zero; a 2->1 decrement
			// survives with one charge.
			if aura.RemainingCharges > 0 {
				aura.RemainingCharges--
				advanceAuraDuration(aura, time.Now())
			} else {
				ts.castMu.Unlock()
				return false
			}
		} else if aura.StackCount > 1 {
			// Aura::ModStackAmount(-1): the aura loses one stack and
			// survives while stacks remain; no timer refresh on decrement.
			oldCount := uint32(aura.StackCount)
			aura.StackCount--
			rescaleAuraAmountsByStack(aura, oldCount, uint32(aura.StackCount))
			advanceAuraDuration(aura, time.Now())
		}
	}
	var slot uint8
	var positive bool
	var maxDuration, remaining uint32
	var count uint8
	removed := aura == nil
	if aura != nil {
		slot, positive = aura.Slot, aura.Positive
		maxDuration, remaining = aura.DurationMs, aura.RemainingMs
		if dispelCharges {
			removed = aura.RemainingCharges == 0
		} else {
			// A zero StackCount reads as a single stack (the
			// auraUpdateRecords wire default), so ModStackAmount(-1) on
			// it removes the aura.
			removed = aura.StackCount <= 1
		}
		if !removed {
			count = auraWireStackCount(aura)
		}
	}
	ts.castMu.Unlock()

	if aura == nil {
		return true
	}
	if removed {
		// Dispel full removal (Unit::RemoveAurasDueToSpellByDispel,
		// Unit.cpp:3936): AURA_REMOVE_BY_ENEMY_SPELL, not BY_EXPIRE.
		ts.dispelPlayerAura(spellID)
		return true
	}
	ts.sendAuraUpdateWithStack(slot, spellID, false, positive, maxDuration, remaining, count)
	return false
}

// dispelCreatureAuraCharge is the creature-aura counterpart of
// dispelPlayerAuraCharge: auras lose one charge (SPELL_ATTR7_DISPEL_CHARGES)
// or one stack (Aura::ModStackAmount, SpellAuras.cpp:1030) per successful
// dispel and survive until the last charge/stack is gone. Returns true when
// the aura was fully removed.
func (s *session) dispelCreatureAuraCharge(key creatureAuraKey, spellID uint32, slot uint8) bool {
	if s.server == nil {
		return true
	}
	s.server.auraMu.Lock()
	var aura *activeAura
	if s.server.activeCreatureAuras != nil {
		aura = s.server.activeCreatureAuras[key][spellID]
	}
	dispelCharges := false
	if aura != nil && !aura.Stopped {
		if s.server.Data != nil {
			if sp, found, _ := s.server.Data.Spell(spellID); found {
				dispelCharges = sp.AttributesEx7&spellAttr7DispelCharges != 0
			}
		}
		if dispelCharges {
			// Aura::ModCharges (SpellAuras.cpp:966-985): charges decrement
			// only while the aura is using charges (m_procCharges != 0,
			// SpellAuras.cpp:441) — a zero-charge ATTR7 aura is untouched
			// by dispel/steal, with no client update. The aura is removed
			// only when the decrement reaches zero; a 2->1 decrement
			// survives with one charge.
			if aura.RemainingCharges > 0 {
				aura.RemainingCharges--
			} else {
				s.server.auraMu.Unlock()
				return false
			}
		} else if aura.StackCount > 1 {
			oldCount := uint32(aura.StackCount)
			aura.StackCount--
			rescaleAuraAmountsByStack(aura, oldCount, uint32(aura.StackCount))
		}
	}
	var positive bool
	var count uint8
	removed := aura == nil
	if aura != nil {
		positive = aura.Positive
		if dispelCharges {
			removed = aura.RemainingCharges == 0
		} else {
			// A zero StackCount reads as a single stack (the
			// auraUpdateRecords wire default), so ModStackAmount(-1) on
			// it removes the aura.
			removed = aura.StackCount <= 1
		}
		if !removed {
			count = auraWireStackCount(aura)
		}
	}
	s.server.auraMu.Unlock()

	if aura == nil || removed {
		// Dispel full removal: AURA_REMOVE_BY_ENEMY_SPELL, not BY_EXPIRE.
		s.dispelCreatureAura(key, spellID, slot)
		return true
	}
	// AuraApplication::BuildUpdatePacket (SpellAuras.cpp:244) sends
	// aura->GetCasterGUID(), not the dispeller — the aura record's own
	// caster GUID rides the partial-charge/stack update.
	casterGUID := aura.CasterGUID
	if casterGUID == 0 {
		casterGUID = s.playerGUID
	}
	updatePkt := protocol.BuildAuraUpdateWithStack(key.GUID, casterGUID, slot, spellID, false, positive, 0, 0, 1, count)
	s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, nil)
	return false
}

func (s *session) handleEffectDispel(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, isReflected bool) {
	s.dispelEffectAsCaster(ctx, 0, targetGUID, spell, eff, nil, isReflected)
}

// executePetSpellDispel runs SPELL_EFFECT_DISPEL (38) for a pet caster with
// m_caster = the pet (SpellEffects.cpp:2429-2531): felhunter Devour Magic
// never reached handleEffectDispel because executePetSpellWithOptions had no
// effect-38 arm, so the dispel did nothing.
func (s *session) executePetSpellDispel(ctx context.Context, motion *creatureMotion, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, isReflected bool) {
	if motion == nil {
		return
	}
	s.dispelEffectAsCaster(ctx, motion.GUID, targetGUID, spell, eff, motion, isReflected)
}

// backlashTickAmount returns the C++ AuraEffect amount backing a dispel
// backlash: the EFFECT_0 tick for Unstable Affliction
// (spell_warlock.cpp:1547), the EFFECT_1 tick for Vampiric Touch
// (spell_priest.cpp:1424). The candidate's flat Amount is the last
// same-aura-type effect's roll, which coincides for UA but not for VT
// (EFFECT_0 there is the dummy), so the per-effect snapshot wins when
// nonzero and the flat amount is the no-data fallback.
func backlashTickAmount(cand dispelCandidate, effectIndex int) uint32 {
	if effectIndex >= 0 && effectIndex < len(cand.Amounts) && cand.Amounts[effectIndex] > 0 {
		return uint32(cand.Amounts[effectIndex])
	}
	return cand.Amount
}

// backlashCasterSession resolves the aura caster's session for a dispel
// backlash: C++ casts the backlash with the aura's caster (GetCaster()),
// not the dispeller (spell_warlock.cpp:1555,
// spell_priest.cpp:1431). Falls back to the dispeller's session when the
// aura caster is not an online player (offline, creature, unrecorded).
func (s *session) backlashCasterSession(casterGUID uint64) *session {
	if s != nil && s.server != nil && casterGUID != 0 {
		if cs := s.server.findSessionByGUID(casterGUID); cs != nil && cs.player != nil {
			return cs
		}
	}
	return s
}

// shadowAffinityDispelRestore mirrors the Shadow Affinity arm of
// Aura::HandleAuraSpecificMods (SpellAuras.cpp:1636-1646): on an enemy-spell
// removal (dispel) of Shadow Word: Pain or Vampiric Touch, the AURA caster's
// Shadow Affinity dummy aura (priest family, SpellIconID 178, effect index 1 —
// Unit::GetDummyAuraEffect's iconId/effIndex pair, Unit.h:1294) restores
// amount% of the caster's create mana via 64103. No-op when the removed spell
// isn't SW:P/VT or the aura caster has no live session (the C++
// `if (!caster) break` arm); creature casters stay a documented delta.
func (s *session) shadowAffinityDispelRestore(ctx context.Context, removedSpellID uint32, auraCasterGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	removed, found, err := s.server.Data.Spell(removedSpellID)
	if err != nil || !found || removed.SpellFamilyName != spellFamilyPriest ||
		(removed.SpellFamilyFlags[0]&0x00008000 == 0 && removed.SpellFamilyFlags[1]&0x00000400 == 0) {
		return
	}
	casterSess := s.server.findSessionByGUID(auraCasterGUID)
	if casterSess == nil || casterSess.player == nil {
		return
	}
	var amount int32
	for _, aura := range casterSess.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask&(1<<1) == 0 {
			continue
		}
		sp, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || sp.SpellFamilyName != spellFamilyPriest || sp.SpellIconID != 178 {
			continue
		}
		if len(sp.Effects) > 1 && sp.Effects[1].Aura == spellAuraDummy {
			amount = aura.Amounts[1]
			break
		}
	}
	if amount <= 0 {
		return
	}
	restore := int32(int64(amount) * int64(energizeCreateMana(casterSess)) / 100)
	if restore <= 0 {
		return
	}
	// C++ casts 64103 (SPELL_EFFECT_ENERGIZE, misc power mana) triggered on
	// the caster; Go applies the energize + the Unit::EnergizeBySpell
	// log/threat tail directly, like the other internal energize arms.
	spell64103, found, err := s.server.Data.Spell(64103)
	if err != nil || !found {
		spell64103 = wotlk.Spell{ID: 64103}
	}
	casterSess.applySpellEnergize(ctx, auraCasterGUID, 0, restore)
	casterSess.energizeBySpellTail(ctx, spell64103, 64103, auraCasterGUID, 0, restore)
}

// raptureDispelProc mirrors the Rapture arm of Aura::HandleAuraSpecificMods
// (SpellAuras.cpp:1648-1704): on an enemy-spell removal (dispel) of Power
// Word: Shield (priest family, SpellFamilyFlags[0] & 0x00000001), the AURA
// caster's Rapture talent aura (any rank of 47535, via the
// GetAuraOfRankedSpell chain walk, Unit.cpp:4570-4586) restores
// CalculatePct(caster max mana, multiplier) mana to the caster via 47755,
// where multiplier is the first effect's amount with the C++ rank
// adjustments (47535: -0.5, 47537: +0.5). The 12s player cooldown gate
// (HasCooldown + the 11s minimal-delay floor before the cooldown takes
// effect, so all bubbles broken by one damage source proc the return)
// rides the caster session's cooldown list; C++ AddCooldown sends no
// client packet (SpellHistory.cpp:395-407), so none is sent here. On a
// roll_chance_i of the second effect's amount, the aura target is
// energized by power type (mana: 63654 with CalculatePct(target max mana,
// 2); rage: 63653; energy: 63655; runic power: 63652), cast by the aura
// caster like the C++ triggered casts. No-op when the removed spell isn't
// PW:S, the aura caster has no live player session (the C++
// `if (!caster) break` arm), or the caster has no Rapture aura; creature
// casters are a documented delta (no session holds the talent aura or
// the cooldown).
func (s *session) raptureDispelProc(ctx context.Context, targetGUID uint64, removedSpellID uint32, auraCasterGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	removed, found, err := s.server.Data.Spell(removedSpellID)
	if err != nil || !found || removed.SpellFamilyName != spellFamilyPriest ||
		len(removed.SpellFamilyFlags) < 1 || removed.SpellFamilyFlags[0]&0x00000001 == 0 {
		return
	}
	casterSess := s.server.findSessionByGUID(auraCasterGUID)
	if casterSess == nil || casterSess.player == nil {
		return
	}
	var rapture *activeAura
	for rank := s.server.getFirstSpellInChain(47535); rank != 0; rank = s.server.getNextSpellInChain(rank) {
		for _, aura := range casterSess.loadedAuras() {
			if aura != nil && !aura.Stopped && aura.SpellID == rank {
				rapture = aura
				break
			}
		}
		if rapture != nil {
			break
		}
	}
	if rapture == nil {
		return
	}
	// Cooldown gate: the caster is a player here, so the C++ TYPEID_PLAYER
	// arm always applies.
	nowUnix := time.Now().Unix()
	remaining := int64(-1)
	for _, cd := range casterSess.player.Cooldowns {
		if cd.Spell == rapture.SpellID && cd.End > nowUnix {
			remaining = cd.End - nowUnix
			break
		}
	}
	if remaining >= 0 {
		if remaining <= 11 {
			return
		}
	} else {
		casterSess.replaceSpellCooldownEntry(spellCooldown{Spell: rapture.SpellID, End: nowUnix + 12})
	}
	// Effect on caster: 47755 with BP0 = CalculatePct(max mana, multiplier).
	multiplier := float64(rapture.Amounts[0])
	switch rapture.SpellID {
	case 47535:
		multiplier -= 0.5
	case 47537:
		multiplier += 0.5
	}
	if restore := int32(float64(casterSess.player.MaxPowers[0]) * multiplier / 100); restore > 0 {
		spell47755, found, err := s.server.Data.Spell(47755)
		if err != nil || !found {
			spell47755 = wotlk.Spell{ID: 47755}
		}
		casterSess.applySpellEnergize(ctx, auraCasterGUID, 0, restore)
		casterSess.energizeBySpellTail(ctx, spell47755, 47755, auraCasterGUID, 0, restore)
	}
	// Effect on aura target: roll_chance_i on the second effect's amount.
	if chance := rapture.Amounts[1]; chance <= 0 || rand.IntN(100) >= int(chance) {
		return
	}
	var targetPower uint8
	var targetMaxMana uint32
	targetResolved := false
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		targetPower = playerPowerType(ts.player)
		targetMaxMana = ts.player.MaxPowers[0]
		targetResolved = true
	} else if s.player != nil {
		if m := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, targetGUID); m != nil {
			targetPower = uint8(m.PowerType)
			targetMaxMana = m.MaxPowers[0]
			targetResolved = true
		}
	}
	if !targetResolved {
		return
	}
	var triggerID uint32
	var triggerPower int32
	switch targetPower {
	case 0: // POWER_MANA
		triggerID = 63654
		triggerPower = 0
	case powerRage:
		triggerID = 63653
		triggerPower = int32(powerRage)
	case powerEnergy:
		triggerID = 63655
		triggerPower = int32(powerEnergy)
	case powerRunicPower:
		triggerID = 63652
		triggerPower = int32(powerRunicPower)
	default:
		return
	}
	trigger, found, err := s.server.Data.Spell(triggerID)
	if err != nil || !found {
		return
	}
	amount := int32(0)
	if triggerID == 63654 {
		// C++ overrides BP0 with CalculatePct(target max mana, 2).
		amount = int32(int64(targetMaxMana) * 2 / 100)
	} else {
		for _, eff := range trigger.Effects {
			if eff.Effect == spellEffectEnergize {
				amount = eff.CalcValueForLevel(trigger, uint32(casterSess.player.Level))
				break
			}
		}
	}
	if amount <= 0 {
		return
	}
	casterSess.applySpellEnergize(ctx, targetGUID, triggerPower, amount)
	casterSess.energizeBySpellTail(ctx, trigger, triggerID, targetGUID, triggerPower, amount)
}

func (s *session) dispelEffectAsCaster(ctx context.Context, casterGUID uint64, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, petCaster *creatureMotion, isReflected bool) {
	if s.player == nil {
		return
	}
	if casterGUID == 0 {
		casterGUID = s.playerGUID
	}

	dispelType := uint32(eff.MiscValue)
	dispelMask := getDispelMask(dispelType)

	var targetSess *session
	if targetGUID == s.playerGUID || targetGUID == 0 {
		targetSess = s
		targetGUID = s.playerGUID
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	var candidates []dispelCandidate
	isTargetPlayer := (targetSess != nil && targetSess.player != nil)
	if isTargetPlayer {
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask, isReflected, false, false)
	} else {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask, isReflected, false, false)
	}

	if len(candidates) == 0 {
		return
	}

	casterLevel := uint32(s.player.Level)
	if petCaster != nil {
		casterLevel = uint32(petCaster.Level)
	}
	maxDispelled := int(eff.CalcValueForLevel(spell, casterLevel))
	// C++ EffectDispel (SpellEffects.cpp:2455-2461) uses the raw damage
	// member (Spell.cpp:5165, CalculateDamage with no clamp): a value <= 0
	// zero-iterates the roll loop and the empty successList arm returns
	// before any packet is built, so there is no forced-to-1.

	var successList []uint32
	var failList []uint32

	for count := 0; count < maxDispelled && len(candidates) > 0; count++ {
		idx := rand.IntN(len(candidates))
		cand := candidates[idx]

		roll := rand.IntN(100)
		if int32(roll) < cand.Chance {
			// Dispel success. Spell::EffectDispel (SpellEffects.cpp:2453-2471)
			// merges repeat successes on the same aura into one successList
			// entry (find_if on spell id + caster, charges accumulated), so
			// SMSG_SPELLDISPELLOG carries each distinct dispelled aura once.
			// Go holds one aura per spell ID per target, so the caster leg of
			// the key is vacuous and the spell-ID dedup is the faithful twin.
			// EffectStealBeneficialBuff has no such dedup
			// (SpellEffects.cpp:5211), so the steal path stays as-is.
			alreadyLogged := false
			for _, logged := range successList {
				if logged == cand.SpellID {
					alreadyLogged = true
					break
				}
			}
			if !alreadyLogged {
				successList = append(successList, cand.SpellID)
			}
			// Charge auras stay in the roll list until their last charge is
			// gone (Spell::EffectDispel DecrementCharge loop, SpellEffects.cpp:2458).
			fullyRemoved := true
			if isTargetPlayer {
				fullyRemoved = targetSess.dispelPlayerAuraCharge(cand.SpellID)
			} else {
				fullyRemoved = s.dispelCreatureAuraCharge(creatureAuraKeyForPlayer(*s.player, targetGUID), cand.SpellID, cand.Slot)
			}
			if fullyRemoved {
				candidates[idx] = candidates[len(candidates)-1]
				candidates = candidates[:len(candidates)-1]
			}

			// Dispel Backfire / Backlash Mechanics.
			// Unstable Affliction (spell_warlock.cpp:1531-1560): the AURA
			// caster casts 31117 on the dispeller — 9 * EFFECT_0 tick
			// amount — plus the 5-second silence. C++ folds the
			// dispeller's damage-taken bonuses into the tick before the
			// x9 (SpellDamageBonusTaken, :1551); Go's damage pipeline
			// applies taken bonuses after the multiply instead, a
			// second-order residual.
			if isUnstableAfflictionSpell(cand.SpellID) {
				backlashDamage := backlashTickAmount(cand, 0) * 9
				if backlashDamage == 0 {
					backlashDamage = 1800
				}
				dispellerGUID := casterGUID
				if petCaster != nil {
					dispellerGUID = petCaster.GUID
				}
				casterSess := s.backlashCasterSession(cand.CasterGUID)
				if petCaster != nil && casterSess == s {
					s.executePetSpellDamage(ctx, petCaster, dispellerGUID, 31117, backlashDamage, 0, true)
				} else {
					casterSess.executeSpellDamage(ctx, dispellerGUID, 31117, backlashDamage, 0)
				}

				silenceSpell := wotlk.Spell{
					ID:         31117,
					DispelType: DispelMagic,
					Mechanic:   9, // MECHANIC_SILENCE
				}
				silenceEff := wotlk.SpellEffect{
					Effect: 6,  // SPELL_EFFECT_APPLY_AURA
					Aura:   18, // SPELL_AURA_MOD_SILENCE
				}
				silenceCasterGUID := cand.CasterGUID
				if silenceCasterGUID == 0 {
					silenceCasterGUID = dispellerGUID
				}
				if petCaster != nil {
					s.applyPetAura(ctx, petCaster, silenceSpell, silenceEff, dispellerGUID, 5000, 0)
				} else {
					casterSess.applyAuraToTarget(ctx, dispellerGUID, silenceSpell, silenceEff, -1, 5000, 0, 0, 32, nil, false, silenceCasterGUID, false, 0, false)
				}
			}

			// Vampiric Touch (spell_priest.cpp:1416-1435): the AURA caster
			// casts 64085 on the AURA OWNER (the dispel target, via
			// GetUnitOwner), not the dispeller — 8 * EFFECT_1 tick
			// amount. The old code dealt 2x to the dispeller instead.
			if isVampiricTouchSpell(cand.SpellID) {
				backlashDamage := backlashTickAmount(cand, 1) * 8
				if backlashDamage == 0 {
					backlashDamage = 680
				}
				casterSess := s.backlashCasterSession(cand.CasterGUID)
				if petCaster != nil && casterSess == s {
					s.executePetSpellDamage(ctx, petCaster, targetGUID, 64085, backlashDamage, 0, true)
				} else {
					casterSess.executeSpellDamage(ctx, targetGUID, 64085, backlashDamage, 0)
				}
			}

			// Shadow Affinity (SpellAuras.cpp:1636-1646): the
			// AURA_REMOVE_BY_ENEMY_SPELL arm of Aura::HandleAuraSpecificMods —
			// when Shadow Word: Pain or Vampiric Touch is removed by an enemy
			// spell (dispel), the AURA caster's Shadow Affinity dummy aura
			// (priest family, icon 178, effect index 1) restores amount% of the
			// caster's create mana via 64103. Fires only on full removal —
			// C++ ModCharges/ModStackAmount arms the specific-mods on the
			// removal, not per charge.
			if fullyRemoved {
				s.shadowAffinityDispelRestore(ctx, cand.SpellID, cand.CasterGUID)
				s.raptureDispelProc(ctx, targetGUID, cand.SpellID, cand.CasterGUID)
			}
		} else {
			// Dispel resisted / failed
			failList = append(failList, cand.SpellID)
		}
	}

	if len(failList) > 0 {
		failPkt := buildDispelFailed(casterGUID, targetGUID, spell.ID, failList)
		_ = s.write(uint16(protocol.OpcodeSMSG_DISPEL_FAILED), failPkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DISPEL_FAILED), failPkt, s)
		}
	}

	if len(successList) > 0 {
		logPkt := buildSpellDispelLog(targetGUID, casterGUID, spell.ID, successList)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, true)
		if targetSess != nil && targetSess != s {
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, true)
		}
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, s)
		}
	}

	// Devour Magic heal (SpellEffects.cpp:2520-2530): once per cast when at
	// least one aura was dispelled — C++ casts triggered 19658 on the caster
	// (the pet for pet casts), and with the Glyph of Felhunter (56249) aura
	// the owner repeats it on itself. C++ gates on SPELLFAMILY_WARLOCK +
	// SPELLCATEGORY_DEVOUR_MAGIC (1179), not spell IDs, so the family/category
	// arm covers every rank; the ID list is the no-data fallback (no
	// Spell.dbc on disk means family/category read zero).
	if (isDevourMagicSpell(spell.ID) || (spell.SpellFamilyName == spellFamilyWarlock && spell.Category == 1179)) && len(successList) > 0 {
		healAmount := uint32(100)
		if len(spell.Effects) > 1 && spell.Effects[1].BasePoints > 0 {
			healAmount = uint32(spell.Effects[1].CalcValue())
		}
		if petCaster != nil {
			s.healDevourMagicPet(ctx, petCaster, healAmount)
		} else {
			// m_caster is the player: C++ self-casts 19658 on m_caster
			// (SpellEffects.cpp:2524), so the full EffectHeal pipeline
			// runs — not a raw Health add.
			s.applyDevourMagicSelfHeal(healAmount)
		}
	}
}

// healDevourMagicPet applies the Devour Magic heal to the casting pet via the
// triggered 19658 (SpellEffects.cpp:2522-2526) and, when the owner carries
// the Glyph of Felhunter aura (56249), to the owner as well
// (SpellEffects.cpp:2527-2529).
func (s *session) healDevourMagicPet(ctx context.Context, petCaster *creatureMotion, healAmount uint32) {
	if s.server == nil || petCaster == nil || healAmount == 0 {
		return
	}
	heal := healAmount
	// Spell::EffectHeal on the triggered 19658 runs the pet through
	// Unit::SpellHealingBonusDone (Unit.cpp:7558-7561) — the same
	// advertised-benefit arms the pet cast site bridges — not a raw Health
	// add.
	if s.server.Data != nil {
		if devourHeal, found, err := s.server.Data.Spell(19658); err == nil && found {
			effIndex := 0
			for i, eff := range devourHeal.Effects {
				if eff.Effect == spellEffectHeal || eff.Effect == spellEffectHealMechanical || eff.Effect == spellEffectHealPct {
					effIndex = i
					break
				}
			}
			if bonus := s.petGuardianDoneHealingBonus(ctx, petCaster, devourHeal, effIndex, petCaster.GUID, false, 1); bonus != 0 {
				if adjusted := int32(heal) + bonus; adjusted > 0 {
					heal = uint32(adjusted)
				} else {
					heal = 0
				}
			}
		}
	}
	s.executePetSpellHeal(ctx, petCaster, petCaster.GUID, 19658, heal)
	if s.player != nil && s.playerHasAura(56249) {
		// SpellEffects.cpp:2527-2529: the glyph arm is owner->CastSpell(owner,
		// 19658, args) — the 19658 caster is the OWNER, not the pet.
		s.applyDevourMagicSelfHeal(healAmount)
	}
}

// applyDevourMagicSelfHeal runs a player self-cast of triggered 19658
// (Devour Magic, SpellEffects.cpp:2520-2530) through the C++ EffectHeal
// pipeline: the SpellHealingBonusTaken caster-match (Unit.cpp:7746-7754)
// names the owner (the 19658 caster), the dead-target gate
// (SpellEffects.cpp:1398) drops the heal and the heal log, the CAST_SPELL
// criterion (Spell.cpp:3442) fires ahead of it, and the PreprocessSpellHit
// legs (Spell.cpp:2725-2735) follow — crit stays false like the pet leg
// (Spell.cpp:7782-7788; no crit model in Go's heal pipeline).
func (s *session) applyDevourMagicSelfHeal(healAmount uint32) {
	if s == nil || s.player == nil {
		return
	}
	heal := s.healingTakenBonus(s, s.playerGUID, 19658, healAmount, false)
	s.updateAchievementCriteria(criteriaTypeCastSpell, 19658, 1)
	if s.player.Health > 0 {
		overheal := uint32(0)
		if uint64(s.player.Health)+uint64(heal) > uint64(s.player.MaxHealth) {
			overheal = uint32(uint64(s.player.Health) + uint64(heal) - uint64(s.player.MaxHealth))
		}
		packet := buildSpellHealLog(s.playerGUID, s.playerGUID, 19658, heal, overheal, 0, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, s)
		s.startTimedAchievement(timedTypeSpellCast, 19658)
		s.updateAchievementCriteria(criteriaTypeCastSpell2, 19658, 1)
		s.startTimedAchievement(timedTypeSpellTarget, 19658)
		s.updateAchievementCriteria(criteriaTypeBeSpellTarget, 19658, 1)
		s.updateAchievementCriteria(criteriaTypeBeSpellTarget2, 19658, 1)
		s.player.Health += heal
		if s.player.Health > s.player.MaxHealth {
			s.player.Health = s.player.MaxHealth
		}
		s.removeGrievousBiteIfFullyHealed(s.playerGUID)
		s.sendPlayerUpdate()
	}
}

// stolenBuff carries a successfully stolen buff from the roll loop to the
// stealer-side apply: the candidate plus the victim aura's remaining
// duration and charge count captured before the steal decrement (C++ reads
// aura->GetDuration()/GetCharges() before mutating, Unit.cpp:3999/4032),
// plus the victim aura's caster GUID and instance pointer for the
// single-target steal dance (Unit.cpp:4015-4042), plus the victim aura's
// per-effect state for the AuraCreateInfo/SetLoadedState twin (the fresh
// stolen aura carries the victim's live effects, Unit.cpp:4017-4033).
type stolenBuff struct {
	candidate        dispelCandidate
	remainingMs      uint32
	victimMaxMs      uint32
	charges          uint8
	stealCharge      bool
	victimCasterGUID uint64
	victimAura       *activeAura
	victimFx         *victimEffectState
}

// victimEffectState snapshots the SetLoadedState inputs of the victim aura
// (Unit.cpp:4017-4033) while the victim-side lock is held: the effect mask
// (effMask), per-effect base amounts (baseDamage), per-effect live amounts
// (damage), recalculate mask, crit chance and resilience flag.
type victimEffectState struct {
	effectMask      uint8
	amounts         [3]int32
	baseAmounts     [3]int32
	recalculateMask uint8
	critChance      float32
	applyResilience bool
}

// stealVictimState reads the victim aura's current remaining duration, max
// duration and charge count; degrades to the candidate's recorded duration
// when the aura is gone. Also returns the aura's caster GUID and instance
// pointer for the single-target steal dance.
func (s *session) stealVictimState(targetSess *session, targetGUID uint64, isTargetPlayer bool, cand dispelCandidate) (remainingMs, victimMaxMs uint32, charges uint8, victimCasterGUID uint64, victimAura *activeAura, victimFx *victimEffectState) {
	if isTargetPlayer && targetSess != nil {
		targetSess.castMu.Lock()
		defer targetSess.castMu.Unlock()
		if aura := targetSess.activeAuras[cand.SpellID]; aura != nil && !aura.Stopped {
			advanceAuraDuration(aura, time.Now())
			return aura.RemainingMs, aura.DurationMs, aura.RemainingCharges, aura.CasterGUID, aura, snapshotVictimEffectState(aura)
		}
		return cand.DurationMs, 0, 0, 0, nil, nil
	}
	if s.server != nil && s.player != nil {
		key := creatureAuraKeyForPlayer(*s.player, targetGUID)
		s.server.auraMu.Lock()
		defer s.server.auraMu.Unlock()
		if aura := s.server.activeCreatureAuras[key][cand.SpellID]; aura != nil && !aura.Stopped {
			// Creature auras expire on AfterFunc timers; RemainingMs is
			// only advanced lazily, so bring it current before reading
			// (the player path above does the same). Advancing is safe:
			// expiry is timer-driven and never reads RemainingMs.
			advanceAuraDuration(aura, time.Now())
			return aura.RemainingMs, aura.DurationMs, aura.RemainingCharges, aura.CasterGUID, aura, snapshotVictimEffectState(aura)
		}
	}
	return cand.DurationMs, 0, 0, 0, nil, nil
}

// snapshotVictimEffectState copies the SetLoadedState inputs off a victim
// aura; the caller must hold the aura's lock.
func snapshotVictimEffectState(aura *activeAura) *victimEffectState {
	return &victimEffectState{
		effectMask:      aura.EffectMask,
		amounts:         aura.Amounts,
		baseAmounts:     aura.BaseAmounts,
		recalculateMask: aura.RecalculateMask,
		critChance:      aura.CritChance,
		applyResilience: aura.ApplyResilience,
	}
}

// carryStealVictimEffects mirrors the AuraCreateInfo + SetLoadedState half of
// Unit::RemoveAurasDueToSpellBySteal (Unit.cpp:4015-4033) on a freshly
// created stolen aura: the new aura carries the VICTIM aura's live effect
// mask, per-effect base and current amounts, recalculate mask, crit chance
// and resilience flag. Without this the stolen aura keeps the synthetic
// single-effect record the apply above wrote (EffectMask 0x01, one aura
// type), while C++ carries every live effect of the victim aura at its
// live amount. The merge path needs nothing: C++ merges via
// ModCharges/ModStackAmount + SetDuration only and never touches the
// victim's mask or amounts (Unit.cpp:4002-4010). A nil state (victim aura
// gone) keeps the synthetic single-effect record.
func (s *session) carryStealVictimEffects(spellID uint32, victimFx *victimEffectState) {
	if victimFx == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped {
		return
	}
	aura.EffectMask = victimFx.effectMask
	aura.Amounts = victimFx.amounts
	aura.BaseAmounts = victimFx.baseAmounts
	aura.RecalculateMask = victimFx.recalculateMask
	aura.CritChance = victimFx.critChance
	aura.ApplyResilience = victimFx.applyResilience
}

// reregisterStolenSpellMods mirrors the AuraEffect::CalculateSpellMod half
// of Aura::SetLoadedState (SpellAuraEffects.cpp:634-653, called at
// Unit.cpp:4032): the synthetic fresh-steal apply registered spell
// modifiers from the single-effect record, so drop those and re-register
// from the victim-live mask/amounts carryStealVictimEffects installed.
// C++ snapshots the pre-RecalculateAmount values, hence this runs before
// the recalculate calls.
func (s *session) reregisterStolenSpellMods(spellID uint32) {
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	stopped := aura == nil || aura.Stopped
	s.castMu.Unlock()
	if stopped {
		return
	}
	s.dropSpellMods(spellID)
	s.addSpellMods(aura)
}

// foldPeriodicPeriodNoHaste mirrors the pet/totem-caster arm of
// AuraEffect::CalculatePeriodic (SpellAuraEffects.cpp:598-613) through
// WorldObject::GetSpellModOwner (Object.cpp:2246-2258): a pet or totem
// caster's SPELLMOD_ACTIVATION_TIME mods fold through the spellmod owner —
// the pet's owner player — while the haste legs read the pet's own
// UNIT_MOD_CAST_SPEED, which has no Go model. The pet apply path makes the
// same call (pet_combat.go:2126-2132), so the amplitude stands at the modded
// DBC base — exact whenever the pet carries no haste, which Go cannot
// represent anyway.
func (s *session) foldPeriodicPeriodNoHaste(spell wotlk.Spell, eff wotlk.SpellEffect) uint32 {
	periodMs := eff.AuraPeriod
	if periodMs == 0 {
		switch eff.Aura {
		case 3, 8, 23, 24, 89:
			periodMs = 3000
		}
	}
	if periodMs == 0 {
		return 0
	}
	return uint32(max(s.applySpellMod(spell, spellModActivationTime, int32(periodMs)), 1))
}

// recalculateStolenAuraPeriod mirrors the AuraEffect::CalculatePeriodic
// recompute half of Aura::SetLoadedState (SpellAuraEffects.cpp:598-618,
// called at Unit.cpp:4032): the stolen aura's periodic amplitude starts
// from the DBC base and folds the ORIGINAL caster's current
// SPELLMOD_ACTIVATION_TIME and haste — it is not the victim aura's live
// period. A player original caster resolves via their session; a pet or
// totem original caster resolves to its owner player's session for the
// spellmod fold (GetSpellModOwner, Object.cpp:2246-2258) with the pet's own
// haste unmodeled, mirroring the pet apply path; any other caster (offline,
// plain creature, unknown) degrades to the unmodded DBC base, like C++'s
// null-caster path. Runs before rescheduleStolenAuraTick so the load-arm
// phase uses the recomputed amplitude, exactly like C++.
// stolenCasterPeriodFold mirrors the AuraEffect::CalculatePeriodic amplitude
// recompute (SpellAuraEffects.cpp:598-630) for a stolen aura: the DBC base
// amplitude folds the ORIGINAL caster's current SPELLMOD_ACTIVATION_TIME and
// haste (foldPeriodicPeriod on the caster's session); the pet/totem arm
// folds the owner player's spellmods without haste (Object.cpp:2246-2258);
// an unresolvable caster degrades to the DBC base like C++'s null-caster
// path. effectMask/auraType select the tick-driving effect — the last effect
// in the mask whose aura type matches the record's, mirroring the tick
// dispatch. Returns 0 when no periodic effect is selected or the spell is
// unknown. Callers must not hold s.castMu: the fold locks the mod-owner's
// castMu, and the mod-owner can be s itself.
func (s *session) stolenCasterPeriodFold(stSpell wotlk.Spell, spellKnown bool, effectMask uint8, auraType uint32, casterGUID uint64) uint32 {
	if !spellKnown {
		return 0
	}
	// Tick-driving effect: the last effect in the mask whose aura type
	// matches the record's, mirroring the tick dispatch.
	tickIdx := -1
	for i, eff := range stSpell.Effects {
		if effectMask&(1<<uint(i)) == 0 {
			continue
		}
		if eff.Aura == auraType {
			tickIdx = i
		}
	}
	if tickIdx < 0 {
		return 0
	}
	var casterSess *session
	if s.server != nil {
		casterSess = s.server.findSessionByGUID(casterGUID)
	}
	var periodMs uint32
	if casterSess != nil {
		periodMs = casterSess.foldPeriodicPeriod(stSpell, stSpell.Effects[tickIdx])
	} else if s.server != nil && s.player != nil {
		// Pet/totem original caster (Object.cpp:2246-2258): the spellmod
		// owner is the pet's owner player, resolved on the stealer's
		// map/instance — the victim is within steal range, so the pet is
		// on the same map. The pet's own haste has no Go model, so the
		// fold is spellmods-only, mirroring the pet apply path
		// (pet_combat.go:2126-2132). A miss falls through to the
		// null-caster path below.
		if modOwner := s.server.spellModOwnerSession(s.player.Map, s.player.InstanceID, casterGUID); modOwner != nil {
			periodMs = modOwner.foldPeriodicPeriodNoHaste(stSpell, stSpell.Effects[tickIdx])
		}
	}
	if periodMs == 0 {
		// Null-caster path (SpellAuraEffects.cpp:598-601): no spellmod or
		// haste folds — the DBC base stands, with Go's zero-period
		// fallback for periodic aura types (auraEffectParams convention).
		periodMs = stSpell.Effects[tickIdx].AuraPeriod
		if periodMs == 0 {
			switch stSpell.Effects[tickIdx].Aura {
			case 3, 8, 23, 24, 89:
				periodMs = 3000
			}
		}
	}
	return periodMs
}

// mergeStolenMaxDuration mirrors the max-duration half of Aura::RefreshTimers
// (SpellAuras.cpp:930-940) as run by Aura::ModStackAmount(+1) on the steal
// merge arm (Unit.cpp:4002-4010): m_maxDuration = Aura::CalcMaxDuration
// (SpellAuras.cpp:871-886) — the raw DBC duration (SpellInfo::GetDuration,
// no per-level scaling) with the original caster's SPELLMOD_DURATION fold.
// RefreshDuration's withMods arm is NOT taken (RefreshTimers calls it with
// the default false), so the haste-duration fold does not apply here. The
// passive no-DurationEntry carve-out is vacuous (passive auras are never
// stealable) and the combo-points term has no Go caster-combo infra.
// Returns 0 when the duration cannot be resolved — the caller keeps the
// stealer's current max. Callers must not hold s.castMu: applySpellMod locks
// the mod-owner's castMu, and the mod-owner can be s itself.
func (s *session) mergeStolenMaxDuration(stSpell wotlk.Spell, spellKnown bool, casterGUID uint64) uint32 {
	if !spellKnown || s.server == nil || s.server.Data == nil || stSpell.DurationIndex == 0 {
		return 0
	}
	base, ok, err := s.server.Data.SpellDurationBase(stSpell.DurationIndex)
	if err != nil || !ok || base <= 0 {
		return 0
	}
	maxDur := uint32(base)
	// The mod owner is the caster's GetSpellModOwner (Object.cpp:2246-2258):
	// player -> self, pet/totem -> owner player, else the null-caster path
	// (no mods). Resolved on the stealer's map/instance — the victim is
	// within steal range, so the caster is on the same map.
	if s.player != nil {
		if modOwner := s.server.spellModOwnerSession(s.player.Map, s.player.InstanceID, casterGUID); modOwner != nil {
			if modded := modOwner.applySpellMod(stSpell, spellModDuration, int32(maxDur)); modded > 0 {
				maxDur = uint32(modded)
			} else {
				maxDur = 0
			}
		}
	}
	return maxDur
}

func (s *session) recalculateStolenAuraPeriod(spellID uint32, stSpell wotlk.Spell, spellKnown bool, victimFx *victimEffectState, victimCasterGUID uint64) {
	if victimFx == nil || !spellKnown {
		return
	}
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	s.castMu.Unlock()
	if aura == nil || aura.Stopped || aura.PeriodMs == 0 {
		return
	}
	periodMs := s.stolenCasterPeriodFold(stSpell, spellKnown, victimFx.effectMask, aura.AuraType, victimCasterGUID)
	if periodMs == 0 {
		return
	}
	s.castMu.Lock()
	if live := s.activeAuras[spellID]; live != nil && !live.Stopped {
		live.PeriodMs = periodMs
	}
	s.castMu.Unlock()
}

// recalculateStolenAuraAmounts mirrors AuraEffect::RecalculateAmount as
// called by Aura::SetLoadedState (SpellAuras.cpp:1238-1257, via
// Unit.cpp:4032): recalculable effects recompute their amounts from the
// ORIGINAL caster's current stats — CalcValue with the victim's base
// amount plus the caster's live spellpower done-bonus — instead of keeping
// the victim aura's live amounts. A non-recalculable effect keeps its
// carried live amount, matching C++'s SetAmount-then-maybe-recalculate
// order. A pet/totem original caster scales by the pet's own level
// (SpellInfo.cpp:413-424) with no spellpower leg — Go has no pet spellpower
// model, matching the pet apply path. An unresolvable caster degrades to
// the null-caster CalcValue (no level scaling, no bonuses), like
// tickCalcValueCasterLevel. Residuals:
// the combo-points term (no combo-point infra on the caster session),
// ApplyEffectModifiers (Go's normal apply path has no twin either), the
// item-enchant arm (a stolen aura carries no cast item), and script
// amount handlers (no infra).
func (s *session) recalculateStolenAuraAmounts(spellID uint32, stSpell wotlk.Spell, spellKnown bool, victimFx *victimEffectState, victimCasterGUID uint64) {
	if victimFx == nil || !spellKnown || victimFx.recalculateMask == 0 {
		return
	}
	var casterSess *session
	var casterLevel uint32
	var spellPower uint32
	if s.server != nil {
		casterSess = s.server.findSessionByGUID(victimCasterGUID)
	}
	if casterSess != nil && casterSess.player != nil {
		casterLevel = uint32(casterSess.player.Level)
		spellPower = casterSess.player.SpellPower
	} else if s.player != nil {
		// Pet/totem original caster: AuraEffect::RecalculateAmount(caster)
		// (SpellAuras.cpp:1256) scales by the caster's own level
		// (SpellEffectInfo::CalcValue, SpellInfo.cpp:413-424) — the pet's
		// level, not the owner's. The spellpower done-bonus leg has no Go
		// pet model, matching the pet apply path's level-only
		// CalcValueForLevel (pet_combat.go:2103-2113).
		if m := s.findCreatureMotion(victimCasterGUID); m != nil && m.PetID != 0 && m.OwnerGUID != 0 && m.Level > 0 {
			casterLevel = uint32(m.Level)
		}
	}
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped {
		s.castMu.Unlock()
		return
	}
	periodMs := aura.PeriodMs
	// Tick-driving effect first: the last effect in the stolen mask whose
	// aura type matches the record's — its recalculated amount (if
	// recalculable) becomes the record's primary amount.
	primaryIdx := -1
	for i, eff := range stSpell.Effects {
		if victimFx.effectMask&(1<<uint(i)) == 0 {
			continue
		}
		if eff.Aura == aura.AuraType {
			primaryIdx = i
		}
	}
	for i, eff := range stSpell.Effects {
		if victimFx.effectMask&(1<<uint(i)) == 0 || victimFx.recalculateMask&(1<<uint(i)) == 0 {
			continue
		}
		recomputed := eff.CalcValueForBase(stSpell, victimFx.baseAmounts[i], casterLevel)
		// AuraEffect::CalculateAmount (SpellAuraEffects.cpp:498-507): the
		// done-bonus leg runs for periodic damage/leech/heal only, using
		// Go's established spellpower simplification (auraEffectParams)
		// with the recomputed period above.
		if spellPower > 0 && periodMs > 0 &&
			(eff.Aura == spellAuraPeriodicDamage || eff.Aura == spellAuraPeriodicLeech || eff.Aura == spellAuraPeriodicHeal) {
			recomputed += int32(math.Round(float64(spellPower) * (float64(periodMs) / 15000.0)))
		}
		aura.Amounts[i] = recomputed
		if i == primaryIdx {
			aura.Amount = uint32(recomputed)
		}
	}
	s.castMu.Unlock()
}

// stolenAuraMergesInto reports whether the stealer-side merge of
// Unit::RemoveAurasDueToSpellBySteal (Unit.cpp:4002) applies: C++ merges only
// via unitStealer->GetAura(aura->GetId(), aura->GetCasterGUID()), so both the
// spell ID and the victim aura's original caster GUID must match the
// stealer's existing aura. A zero victim caster degrades to the stealing
// player (the same fallback the fresh-apply path uses); a zero existing
// caster never matches. Go holds one aura per spell ID, so on a miss the
// steal falls through to the fresh apply, which replaces the slot — the
// C++ rank-chain term (SpellAuras.cpp:1988-2004) never lets the same spell
// stack with itself anyway.
func stolenAuraMergesInto(existing *activeAura, victimCasterGUID, playerGUID uint64) bool {
	mergeCaster := victimCasterGUID
	if mergeCaster == 0 {
		mergeCaster = playerGUID
	}
	return existing != nil && !existing.Stopped && existing.CasterGUID == mergeCaster
}

// mergeStolenAura mirrors the stealer-side merge in
// Unit::RemoveAurasDueToSpellBySteal (Unit.cpp:4002-4010): when the stealer
// already holds the stolen spell from the same original caster, the steal
// merges into it — one charge (Aura::ModCharges(+1), clamped to the spell's
// max charges) or one stack (Aura::ModStackAmount(+1), clamped to the
// spell's stack amount) — and the remaining time is reset to the capped
// stolen duration, instead of the aura being re-applied fresh. Reports
// whether a merge happened.
func (s *session) mergeStolenAura(spellID uint32, stSpell wotlk.Spell, spellKnown bool, stealCharge bool, dur uint32, victimCasterGUID uint64) bool {
	// Snapshot mergeability and the RefreshTimers inputs before mutating.
	// The caster folds (mergeStolenMaxDuration, stolenCasterPeriodFold)
	// lock the mod-owner's castMu — which can be s itself — so they must
	// run with s.castMu released.
	s.castMu.Lock()
	mergeable := stolenAuraMergesInto(s.activeAuras[spellID], victimCasterGUID, s.playerGUID)
	var effectMask uint8
	var auraType uint32
	periodic := false
	if mergeable && !stealCharge && spellKnown {
		if existing := s.activeAuras[spellID]; existing != nil {
			effectMask, auraType = existing.EffectMask, existing.AuraType
			periodic = existing.PeriodMs > 0
		}
	}
	s.castMu.Unlock()
	if !mergeable {
		return false
	}
	mergeCaster := victimCasterGUID
	if mergeCaster == 0 {
		mergeCaster = s.playerGUID
	}
	// Aura::CalcMaxCharges inputs (SpellAuras.cpp:951-962) for the steal
	// merge below: the original caster's spell-mod owner folds
	// SPELLMOD_CHARGES. s.castMu is taken below, so the fold runs unlocked
	// here; when the mod owner is this same session the no-taking fold runs
	// at the use sites instead of re-locking it.
	stealMapID, stealInstanceID := uint32(0), uint32(0)
	if s.player != nil {
		stealMapID, stealInstanceID = s.player.Map, s.player.InstanceID
	}
	stealModOwner := s.server.spellModOwnerSession(stealMapID, stealInstanceID, mergeCaster)
	stealOwnerLocked := stealModOwner != nil && stealModOwner == s
	var refreshMaxDur, refreshPeriod uint32
	if !stealCharge && spellKnown {
		refreshMaxDur = s.mergeStolenMaxDuration(stSpell, spellKnown, mergeCaster)
		if periodic {
			refreshPeriod = s.stolenCasterPeriodFold(stSpell, spellKnown, effectMask, auraType, mergeCaster)
		}
	}
	upkeep := !stealCharge && spellKnown && auraUpkeepNeeded(stSpell)

	s.castMu.Lock()
	existing := s.activeAuras[spellID]
	if !stolenAuraMergesInto(existing, victimCasterGUID, s.playerGUID) {
		s.castMu.Unlock()
		return false
	}
	if stealCharge {
		// Aura::ModCharges(+1, SpellAuras.cpp:964): increments while the
		// aura uses charges, clamped to CalcMaxCharges
		// (SpellAuras.cpp:951-962) — the DBC proc charges folded through
		// the original caster's spell-mod owner's SPELLMOD_CHARGES mods.
		maxCharges := calcAuraMaxChargesFor(stSpell, stealModOwner, stealOwnerLocked)
		if existing.RemainingCharges > 0 || maxCharges > 0 {
			charges := int32(existing.RemainingCharges) + 1
			if max := int32(maxCharges); max > 0 && charges > max {
				charges = max
			}
			existing.RemainingCharges = uint8(charges)
		}
	} else {
		// Aura::ModStackAmount(+1, SpellAuras.cpp:1030): clamped to the
		// spell's stack amount (1 when the spell is not stackable).
		maxStack := int32(existing.StackAmount)
		if maxStack == 0 {
			maxStack = 1
		}
		cur := int32(existing.StackCount)
		if cur == 0 {
			cur = 1
		}
		oldCount := uint32(cur)
		if cur++; cur > maxStack {
			cur = maxStack
		}
		existing.StackCount = uint8(cur)
		// SetStackAmount recalc (SpellAuras.cpp:1008) on the increase: the
		// stolen stack must scale the amounts up, or a later decrement
		// would divide amounts that were never multiplied.
		rescaleAuraAmountsByStack(existing, oldCount, uint32(cur))
		// The ModStackAmount refresh flag is unconditional on increase
		// (SpellAuras.cpp:1042): RefreshTimers and the charge reset run
		// even when the stack clamps at the max — SetCharges(
		// CalcMaxCharges()) (SpellAuras.cpp:1060), the original caster's
		// spell-mod owner folding SPELLMOD_CHARGES.
		existing.RemainingCharges = calcAuraMaxChargesFor(stSpell, stealModOwner, stealOwnerLocked)
		if spellKnown {
			// Aura::RefreshTimers(true) (SpellAuras.cpp:930-940): the max
			// duration is recomputed from the original caster
			// (Aura::CalcMaxDuration), the periodic amplitude is
			// recomputed and the tick timer restarts at a full amplitude
			// (AuraEffect::ResetPeriodic(true),
			// SpellAuraEffects.cpp:553), tick counters zero, and
			// RefreshDuration re-arms the 1s power-upkeep clock (m_timeCla).
			// Gated on known spell data — the no-data regime keeps the
			// stealer's live timers.
			if refreshMaxDur > 0 {
				existing.DurationMs = refreshMaxDur
			}
			oldPeriod := existing.PeriodMs
			if refreshPeriod > 0 {
				existing.PeriodMs = refreshPeriod
			} else {
				refreshPeriod = oldPeriod
			}
			existing.TickCount = 0
			if existing.TickTimer != nil {
				existing.TickTimer.Stop()
				existing.TickTimer = nil
			}
			stopAuraUpkeepTimer(existing)
		}
	}
	// oldAura->SetDuration(int32(dur)) (Unit.cpp:4009, withMods=false):
	// only the remaining time is written, never the max.
	existing.RemainingMs = dur
	existing.DurationUpdatedAt = time.Now()
	// Go's expiry timer is the twin of C++'s update-loop IsExpired check —
	// the merge rewrites the remaining time, so the old deadline must go,
	// or the aura expires on the pre-merge schedule.
	if existing.Timer != nil {
		existing.Timer.Stop()
		existing.Timer = nil
	}
	slot, positive := existing.Slot, existing.Positive
	maxDur := existing.DurationMs
	count := auraWireStackCount(existing)
	s.castMu.Unlock()
	if dur == 0 {
		// C++ oldAura->SetDuration(0) (Unit.cpp:4009): the merged aura is
		// expired and removed on the owner's next update pass (IsExpired,
		// Unit.cpp:2989). Go has no deferred removal queue, so expire it
		// now instead of sending a zero-duration update.
		s.expirePlayerAura(spellID)
		return true
	}
	if !stealCharge && spellKnown {
		if refreshPeriod > 0 {
			// AuraEffect::ResetPeriodic(true): the first tick fires after
			// a full recomputed amplitude.
			s.schedulePlayerPeriodicTickInitial(existing, refreshPeriod, periodicTickInitialDelay(stSpell, refreshPeriod))
		}
		if upkeep {
			s.scheduleAuraUpkeep(existing)
		}
		// AuraEffect::ChangeAmount -> CalculateSpellMod
		// (SpellAuraEffects.cpp:657-683): the rescaled amounts update the
		// registered modifier values.
		s.refreshSpellModValues(existing)
	}
	// Re-arm the expiry timer for the merged remaining time.
	s.castMu.Lock()
	if live := s.activeAuras[spellID]; live == existing && !live.Stopped && dur > 0 {
		live.Timer = time.AfterFunc(time.Duration(dur)*time.Millisecond, func() {
			s.expirePlayerAura(spellID)
		})
	}
	s.castMu.Unlock()
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, maxDur, dur, count)
	return true
}

// rescheduleStolenAuraTick mirrors the AuraEffect::CalculatePeriodic load arm
// (SpellAuraEffects.cpp:619-630) that Aura::SetLoadedState runs for a freshly
// stolen aura (Unit.cpp:4032): the stolen aura does not restart the periodic
// timer at a full amplitude — it keeps the victim's tick cadence, with the
// periodic timer set to the time since the victim's last tick:
// _periodicTimer = (victimMaxDuration - dur) % amplitude. The first tick on
// the stealer therefore fires after amplitude - (elapsed % amplitude), not
// after a full amplitude like a fresh apply would. A permanent victim aura
// (victimMaxMs 0) makes the stolen aura permanent too, so C++ skips the
// elapsed recompute and the construction ResetPeriodic stands: the first
// tick fires after a full RECOMPUTED amplitude, and the victim-live delay
// the fresh apply scheduled is stopped and re-armed. The merge arm is
// handled separately in mergeStolenAura: C++ ModStackAmount(+1) there runs
// Aura::RefreshTimers, which resets the periodic timer to a full
// recomputed amplitude.
// Call right after the fresh stolen-aura apply, with dur the capped stolen
// duration and victimMaxMs the SetLoadedState maxduration arg
// (aura->GetMaxDuration()).
func (s *session) rescheduleStolenAuraTick(spellID, victimMaxMs, dur uint32, stSpell wotlk.Spell) {
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped || aura.TickInFlight {
		s.castMu.Unlock()
		return
	}
	periodMs := aura.PeriodMs
	if periodMs == 0 {
		// Non-periodic: no timer exists.
		s.castMu.Unlock()
		return
	}
	if aura.TickTimer != nil {
		aura.TickTimer.Stop()
	}
	if victimMaxMs == 0 {
		// Permanent victim aura: the stolen aura is permanent too
		// (SetLoadedState maxduration arg is the victim's -1,
		// SpellAuras.h:152), so C++'s CalculatePeriodic load arm skips the
		// elapsed recompute (SpellAuraEffects.cpp:619-627) and the
		// construction ResetPeriodic stands — the first tick fires after a
		// full RECOMPUTED amplitude (SpellAuraEffects.cpp:553-563), not
		// the victim-live delay the fresh apply scheduled above (that
		// delay was computed from the victim aura's live period, folded
		// with the original caster's mods at the victim's apply time).
		// Re-arm at the recomputed period; START_PERIODIC_AT_APPLY keeps
		// its zero delay like the construction arm.
		delay := periodicTickInitialDelay(stSpell, periodMs)
		s.castMu.Unlock()
		s.schedulePlayerPeriodicTickInitial(aura, periodMs, delay)
		return
	}
	var elapsed uint32
	if victimMaxMs > dur {
		elapsed = victimMaxMs - dur
	}
	// C++ sets _periodicTimer = elapsed % amplitude and the Update loop
	// accumulates diff until it reaches the amplitude, so the first tick
	// fires after amplitude - (elapsed % amplitude) — a FULL amplitude
	// when elapsed is an exact multiple (timer 0 still needs a full
	// amplitude of accumulation, exactly like a fresh apply). The modulo
	// must not wrap that case to 0: a 0 delay would fire an immediate
	// extra tick C++ never fires (e.g. stealing a freshly applied HoT).
	delay := periodMs - elapsed%periodMs
	s.castMu.Unlock()
	s.schedulePlayerPeriodicTickInitial(aura, periodMs, delay)
}

// setAuraCharges fixes the stolen aura's charge count and max duration after
// the fresh apply (C++ Aura::SetLoadedState charges + maxduration args,
// SpellAuras.cpp:1238, called at Unit.cpp:4032) and pushes the corrected
// values to the client when either changed. The max duration is the VICTIM
// aura's max, not the capped remaining: a 30-minute buff stolen with 5
// minutes left keeps max=30m in C++ while the earlier code sent max=5m.
func (s *session) setAuraCharges(spellID uint32, charges uint8, victimMaxMs uint32) {
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped {
		s.castMu.Unlock()
		return
	}
	changed := false
	if aura.RemainingCharges != charges {
		aura.RemainingCharges = charges
		changed = true
	}
	// Skip a zero victim max: the victim aura is permanent and Go's
	// DurationMs==0 means permanent, which would freeze the expiry
	// countdown (advanceAuraDuration no-ops on DurationMs==0) while C++
	// still expires the stolen aura on m_duration=dur.
	if victimMaxMs != 0 && aura.DurationMs != victimMaxMs {
		aura.DurationMs = victimMaxMs
		changed = true
	}
	if !changed {
		s.castMu.Unlock()
		return
	}
	slot, positive := aura.Slot, aura.Positive
	maxDuration, remaining := aura.DurationMs, aura.RemainingMs
	// The wire stack-count field carries the stack amount for stackable
	// spells, else the charges (AuraApplication::BuildUpdatePacket,
	// SpellAuras.cpp:241-242).
	wireCount := auraWireStackCount(aura)
	s.castMu.Unlock()
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, maxDuration, remaining, wireCount)
}

// handleEffectSpellsteal processes SPELL_EFFECT_STEAL_BENEFICIAL_BUFF (126).
// Mirrors TrinityCore Spell::EffectStealBeneficialBuff (SpellEffects.cpp:5150-5251).
func (s *session) handleEffectSpellsteal(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s.player == nil || targetGUID == s.playerGUID || targetGUID == 0 {
		return
	}

	dispelType := uint32(eff.MiscValue)
	dispelMask := getDispelMask(dispelType)

	var targetSess *session
	if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	isTargetPlayer := (targetSess != nil && targetSess.player != nil)
	// Spellsteal only steals beneficial buffs (SpellEffects.cpp:5172); the
	// helper skips passive auras (already excluded by the dispel helpers) and
	// auras whose spell carries SPELL_ATTR4_NOT_STEALABLE.
	stealable := s.stealableAuraList(targetSess, targetGUID, isTargetPlayer, dispelMask, false)

	if len(stealable) == 0 {
		return
	}

	maxDispelled := int(eff.CalcValueForLevel(spell, uint32(s.player.Level)))
	// C++ EffectStealBeneficialBuff (SpellEffects.cpp:5201-5202) uses the
	// raw damage member (Spell.cpp:5165, CalculateDamage with no clamp): a
	// value <= 0 zero-iterates the roll loop and the empty successList arm
	// returns before SMSG_SPELLSTEALLOG is built, so there is no
	// forced-to-1.

	var successList []uint32
	var stolen []stolenBuff
	var failList []uint32

	for count := 0; count < maxDispelled && len(stealable) > 0; count++ {
		idx := rand.IntN(len(stealable))
		cand := stealable[idx]

		roll := rand.IntN(100)
		if int32(roll) < cand.Chance {
			successList = append(successList, cand.SpellID)
			// C++ reads the victim aura's duration/charges before mutating
			// it (Unit.cpp:3999/4032).
			stealCharge := false
			if s.server != nil && s.server.Data != nil {
				if sp, found, _ := s.server.Data.Spell(cand.SpellID); found {
					stealCharge = sp.AttributesEx7&spellAttr7DispelCharges != 0
				}
			}
			remaining, victimMaxMs, charges, victimCasterGUID, victimAura, victimFx := s.stealVictimState(targetSess, targetGUID, isTargetPlayer, cand)
			stolen = append(stolen, stolenBuff{candidate: cand, remainingMs: remaining, victimMaxMs: victimMaxMs, charges: charges, stealCharge: stealCharge, victimCasterGUID: victimCasterGUID, victimAura: victimAura, victimFx: victimFx})
			// C++ RemoveAurasDueToSpellBySteal (Unit.cpp:4046-4049): a
			// successful steal removes one charge
			// (SPELL_ATTR7_DISPEL_CHARGES) or one stack
			// (Aura::ModStackAmount(-1)) from the victim aura — the aura
			// survives while charges/stacks remain and stays in the roll
			// list like the C++ DecrementCharge loop.
			fullyRemoved := true
			if isTargetPlayer {
				fullyRemoved = targetSess.dispelPlayerAuraCharge(cand.SpellID)
			} else {
				fullyRemoved = s.dispelCreatureAuraCharge(creatureAuraKeyForPlayer(*s.player, targetGUID), cand.SpellID, cand.Slot)
			}
			if fullyRemoved {
				stealable[idx] = stealable[len(stealable)-1]
				stealable = stealable[:len(stealable)-1]
			}
		} else {
			failList = append(failList, cand.SpellID)
		}
	}

	// Apply stolen buffs to the stealer. C++ (Unit.cpp:3999):
	//   uint32 dur = std::min(2u * MINUTE * IN_MILLISECONDS, uint32(aura->GetDuration()));
	// A permanent victim aura reports GetDuration() -1, which wraps to
	// UINT32_MAX and clamps to 120000 — Go's permanent victim auras
	// (DurationMs==0 with a live record) take the same arm. A
	// merely-expired victim aura (remaining 0, max > 0) must NOT: C++
	// steals that at duration 0 and the stolen aura expires on the
	// stealer's next update (IsExpired, Unit.cpp:2989), never ticking.
	for _, st := range stolen {
		cand := st.candidate
		dur := st.remainingMs
		if dur > 120000 {
			dur = 120000
		}
		if dur == 0 && st.victimAura != nil && st.victimMaxMs == 0 {
			dur = 120000
		}
		expiredSteal := dur == 0
		stSpell := wotlk.Spell{ID: cand.SpellID, DispelType: cand.DispelType, Mechanic: cand.Mechanic}
		stSpellKnown := false
		if s.server != nil && s.server.Data != nil {
			if loaded, found, _ := s.server.Data.Spell(cand.SpellID); found {
				stSpell = loaded
				stSpellKnown = true
			}
		}
		if s.mergeStolenAura(cand.SpellID, stSpell, stSpellKnown, st.stealCharge, dur, st.victimCasterGUID) {
			continue
		}
		// Unit::RemoveAurasDueToSpellBySteal single-target dance
		// (Unit.cpp:4015-4042): the _AddAura dance runs on the stolen aura
		// with the ORIGINAL caster, purging that caster's other
		// single-target-with auras — but the created aura itself must not
		// stay single-target, "so stealer won't loose it on recast". C++
		// unregisters the victim aura before the dance and re-registers
		// it after (Unit.cpp:4031-4032); here it is excluded from the
		// purge by pointer and its registration is kept (a fully removed
		// victim aura unregistered itself on expiry). The stolen aura is
		// created with single-target registration skipped.
		if spellIsSingleTarget(stSpell) && st.victimCasterGUID != 0 {
			for _, e := range s.server.crossTargetSingleCastPurge(st.victimCasterGUID, stSpell, st.victimAura) {
				s.expireSingleCastEntry(e)
			}
		}
		eff := wotlk.SpellEffect{
			Effect:     6,
			Aura:       cand.AuraType,
			BasePoints: int32(cand.Amount) - 1,
		}
		stealCasterGUID := st.victimCasterGUID
		if stealCasterGUID == 0 {
			stealCasterGUID = s.playerGUID
		}
		s.applyAuraToTarget(ctx, s.playerGUID, stSpell, eff, -1, dur, cand.PeriodMs, cand.Amount, cand.SchoolMask, nil, true, stealCasterGUID, false, 0, false)
		// C++ AuraCreateInfo/SetLoadedState twin (Unit.cpp:4015-4033): the
		// fresh stolen aura carries the victim aura's live effect mask,
		// per-effect amounts, recalculate mask, crit chance and
		// resilience flag instead of the synthetic single-effect record.
		s.carryStealVictimEffects(cand.SpellID, st.victimFx)
		// C++ AuraEffect::CalculateSpellMod (SpellAuraEffects.cpp:634-653,
		// called at Unit.cpp:4032 between the amount snapshot and
		// RecalculateAmount): the synthetic apply above registered spell
		// modifiers from the single-effect record, so re-register from the
		// victim-live mask/amounts just installed — C++ snapshots the
		// pre-recalculation values, hence this runs before the recalculate
		// calls below.
		s.reregisterStolenSpellMods(cand.SpellID)
		// C++ SetLoadedState charges arg (Unit.cpp:4032): 1 for
		// ATTR7_DISPEL_CHARGES auras, the victim's charge count otherwise.
		wantCharges := st.charges
		if st.stealCharge {
			wantCharges = 1
		}
		s.setAuraCharges(cand.SpellID, wantCharges, st.victimMaxMs)
		// C++ Aura::SetLoadedState -> AuraEffect::CalculatePeriodic
		// (SpellAuraEffects.cpp:598-630, called at Unit.cpp:4032): the
		// stolen aura's amplitude is recomputed from the DBC base with the
		// ORIGINAL caster's current spellmods/haste — not carried from
		// the victim — and only then does the load arm phase the tick
		// timer off it.
		s.recalculateStolenAuraPeriod(cand.SpellID, stSpell, stSpellKnown, st.victimFx, st.victimCasterGUID)
		// C++ AuraEffect::RecalculateAmount (via SetLoadedState,
		// SpellAuras.cpp:1238-1257): recalculable effects recompute from
		// the original caster's current stats instead of keeping the
		// victim's live amounts.
		s.recalculateStolenAuraAmounts(cand.SpellID, stSpell, stSpellKnown, st.victimFx, st.victimCasterGUID)
		// C++ Aura::SetLoadedState -> AuraEffect::CalculatePeriodic load arm
		// (SpellAuraEffects.cpp:619-630, called at Unit.cpp:4032): the fresh
		// stolen aura keeps the victim's periodic tick cadence instead of
		// restarting the timer at a full amplitude.
		s.rescheduleStolenAuraTick(cand.SpellID, st.victimMaxMs, dur, stSpell)
		if expiredSteal {
			// A zero-remaining victim aura is stolen at duration 0
			// (Unit.cpp:3999); C++ still creates the stolen aura — it
			// never ticks (the load-arm tick cap) and expires on the
			// stealer's next update (IsExpired, Unit.cpp:2989). Go cannot
			// represent a zero-duration non-permanent aura (DurationMs==0
			// is permanent), so expire it now that the C++-ordered
			// SetLoadedState sequence above has run.
			s.expirePlayerAura(cand.SpellID)
		}
	}

	if len(failList) > 0 {
		failPkt := buildDispelFailed(s.playerGUID, targetGUID, spell.ID, failList)
		_ = s.write(uint16(protocol.OpcodeSMSG_DISPEL_FAILED), failPkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DISPEL_FAILED), failPkt, s)
		}
	}

	if len(successList) > 0 {
		logPkt := buildSpellstealLog(targetGUID, s.playerGUID, spell.ID, successList)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLSTEALLOG), logPkt, true)
		if targetSess != nil && targetSess != s {
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELLSTEALLOG), logPkt, true)
		}
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLSTEALLOG), logPkt, s)
		}
	}
}

// auraSpellMechanicMask is the C++ SpellInfo::GetAllEffectsMechanicMask twin
// (SpellInfo.cpp:1893-1902): the aura spell's top-level Mechanic plus every
// non-NONE effect's Mechanic, as a bit mask. The aura record only carries the
// top-level Mechanic, so per-effect mechanics resolve from the spell store;
// an unresolvable spell falls back to the top-level bit alone.
func (s *session) auraSpellMechanicMask(aura *activeAura) uint32 {
	var mask uint32
	if aura == nil {
		return mask
	}
	if aura.Mechanic != 0 {
		mask |= uint32(1) << aura.Mechanic
	}
	if s.server != nil && s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
			for _, eff := range sp.Effects {
				if eff.Effect != 0 && eff.Mechanic != 0 {
					mask |= uint32(1) << eff.Mechanic
				}
			}
		}
	}
	return mask
}

// handleEffectDispelMechanic processes SPELL_EFFECT_DISPEL_MECHANIC (108).
// Mirrors TrinityCore Spell::EffectDispelMechanic (SpellEffects.cpp:4733-4756).
func (s *session) handleEffectDispelMechanic(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	mechanic := uint32(eff.MiscValue)
	if mechanic == 0 {
		return
	}

	var targetSess *session
	if targetGUID == s.playerGUID || targetGUID == 0 {
		targetSess = s
		targetGUID = s.playerGUID
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	if targetSess != nil && targetSess.player != nil {
		// Spell::EffectDispelMechanic (SpellEffects.cpp:4749): each
		// mechanic-matching aura is dispelled only on a successful
		// Aura::CalcDispelChance roll, computed per aura
		// (SpellAuras.cpp:1218-1236). Two passes: the roll folds the aura
		// caster's spell mods (takes the caster session's lock), so it runs
		// after the target's lock is released.
		targetSess.castMu.Lock()
		offensive := !s.isFriendlyToTarget(targetGUID, targetSess)
		resistTotal := int32(0)
		if offensive {
			resistTotal = targetSess.dispelResistTotalLocked()
		}
		var matched []*activeAura
		for _, aura := range targetSess.activeAuras {
			if aura == nil || aura.Stopped {
				continue
			}
			// Owned-only scan (SpellEffects.cpp:4739): another caster's
			// persistent-area aura applications are never dispelled.
			if aura.PersistentAreaAura && aura.CasterGUID != targetSess.playerGUID {
				continue
			}
			// C++ matches the aura spell's full mechanic mask, not just the
			// top-level Mechanic the aura record carries
			// (SpellInfo::GetAllEffectsMechanicMask, SpellInfo.cpp:1893-1902).
			matchesMechanic := (targetSess.auraSpellMechanicMask(aura) & (uint32(1) << mechanic)) != 0
			if !matchesMechanic {
				// Also check if aura type corresponds to standard CC mechanics
				switch mechanic {
				case 1: // MECHANIC_CHARM
					matchesMechanic = (aura.AuraType == 6)
				case 2: // MECHANIC_DISORIENTED
					matchesMechanic = (aura.AuraType == 5)
				case 5: // MECHANIC_FEAR
					matchesMechanic = (aura.AuraType == 7)
				case 7: // MECHANIC_ROOT
					matchesMechanic = (aura.AuraType == 15)
				case 9: // MECHANIC_SILENCE
					matchesMechanic = (aura.AuraType == 14)
				case 11: // MECHANIC_SNARE
					matchesMechanic = (aura.AuraType == 130 || aura.AuraType == 31)
				case 12: // MECHANIC_STUN
					matchesMechanic = (aura.AuraType == 12)
				case 15: // MECHANIC_BLEED
					matchesMechanic = (aura.AuraType == 3 && aura.SchoolMask == 1)
				}
			}
			if matchesMechanic {
				matched = append(matched, aura)
			}
		}
		mapID, instanceID := targetSess.player.Map, targetSess.player.InstanceID
		targetSess.castMu.Unlock()

		var toRemove []uint32
		for _, aura := range matched {
			if int32(rand.IntN(100)) < s.server.auraDispelChance(mapID, instanceID, aura, resistTotal, offensive) {
				toRemove = append(toRemove, aura.SpellID)
			}
		}

		for _, spID := range toRemove {
			// Mass-dispel removal: AURA_REMOVE_BY_ENEMY_SPELL.
			targetSess.dispelPlayerAura(spID)
		}
	} else if targetGUID != 0 && s.server != nil {
		s.server.auraMu.Lock()
		key := creatureAuraKeyForPlayer(*s.player, targetGUID)
		auras := s.server.activeCreatureAuras[key]
		// Creature-side CalcDispelChance: same per-aura 100-minus-resist roll
		// as the creature dispel path (SpellEffects.cpp:4749), two passes so
		// the caster-mod fold runs without auraMu held.
		offensive := !s.isFriendlyToTarget(targetGUID, nil)
		resistTotal := int32(0)
		if offensive {
			for _, ra := range auras {
				if ra != nil && !ra.Stopped && ra.AuraType == spellAuraModDispelResist {
					resistTotal += int32(ra.Amount)
				}
			}
		}
		var matched []*activeAura
		for _, aura := range auras {
			if aura == nil || aura.Stopped {
				continue
			}
			// Owned-only scan (SpellEffects.cpp:4739), like the player leg.
			if aura.PersistentAreaAura && aura.CasterGUID != targetGUID {
				continue
			}
			if (s.auraSpellMechanicMask(aura)&(uint32(1)<<mechanic)) != 0 || (mechanic == 5 && aura.AuraType == 7) || (mechanic == 12 && aura.AuraType == 12) {
				matched = append(matched, aura)
			}
		}
		mapID, instanceID := s.player.Map, s.player.InstanceID
		s.server.auraMu.Unlock()

		var toRemove []struct {
			spellID uint32
			slot    uint8
		}
		for _, aura := range matched {
			if int32(rand.IntN(100)) < s.server.auraDispelChance(mapID, instanceID, aura, resistTotal, offensive) {
				toRemove = append(toRemove, struct {
					spellID uint32
					slot    uint8
				}{spellID: aura.SpellID, slot: aura.Slot})
			}
		}

		for _, item := range toRemove {
			// Mass-dispel removal: AURA_REMOVE_BY_ENEMY_SPELL.
			s.dispelCreatureAura(key, item.spellID, item.slot)
		}
	}
}
