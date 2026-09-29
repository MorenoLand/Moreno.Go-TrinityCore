package world

import (
	"context"
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

// calcDispelChanceLocked calculates dispel chance assuming castMu is already held.
func (s *session) calcDispelChanceLocked(offensive bool) int32 {
	resistChance := int32(0)
	if offensive {
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
				spell, found, err := s.server.Data.Spell(pSpell.ID)
				if err == nil && found {
					for _, eff := range spell.Effects {
						if eff.Aura == spellAuraModDispelResist {
							resistChance += eff.BasePoints + 1
						}
					}
				}
			}
		}
	}
	if resistChance < 0 {
		resistChance = 0
	} else if resistChance > 100 {
		resistChance = 100
	}
	return 100 - resistChance
}

// calcDispelChance mirrors TrinityCore Aura::CalcDispelChance (SpellAuras.cpp:1218-1236).
func (s *session) calcDispelChance(offensive bool) int32 {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.calcDispelChanceLocked(offensive)
}

// getDispellableAuraListForPlayer returns the list of auras that can be dispelled from a player.
// Mirrors TrinityCore Unit::GetDispellableAuraList (Unit.cpp:4588-4628).
func (s *session) getDispellableAuraListForPlayer(targetSess *session, dispelMask uint32) []dispelCandidate {
	if targetSess == nil || targetSess.player == nil {
		return nil
	}

	// If target is affected by Unholy Blight (50536), cannot dispel diseases (Unit.cpp:4591)
	if (dispelMask&(1<<DispelDisease) != 0) && targetSess.hasAura(spellUnholyBlight) {
		dispelMask &^= (1 << DispelDisease)
	}

	isFriendly := s.isFriendlyToPlayer(targetSess)

	targetSess.castMu.Lock()
	defer targetSess.castMu.Unlock()

	var candidates []dispelCandidate
	for spellID, aura := range targetSess.activeAuras {
		if aura == nil || aura.Stopped {
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

		// Friendly target: remove harmful debuffs (!aura.Positive)
		// Hostile target: remove beneficial buffs (aura.Positive)
		// Mirrors Unit.cpp:4608-4612
		if isFriendly == aura.Positive {
			continue
		}

		// Calculate dispel chance: 100 - resistChance (SpellAuras.cpp:1218-1236)
		chance := targetSess.calcDispelChanceLocked(!isFriendly)
		if chance < 0 {
			chance = 0
		}

		candidates = append(candidates, dispelCandidate{
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
			SchoolMask: aura.SchoolMask,
		})
	}
	return candidates
}

// getDispellableAuraListForCreature returns dispellable auras on a creature.
func (s *session) getDispellableAuraListForCreature(creatureGUID uint64, dispelMask uint32) []dispelCandidate {
	if s.server == nil || s.player == nil {
		return nil
	}
	s.server.auraMu.Lock()
	defer s.server.auraMu.Unlock()

	auras := s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, creatureGUID)]
	if len(auras) == 0 {
		return nil
	}

	isFriendly := false
	if s.player != nil && s.player.PetGUID != 0 && s.player.PetGUID == creatureGUID {
		isFriendly = true
	}

	var candidates []dispelCandidate
	for spellID, aura := range auras {
		if aura == nil || aura.Stopped {
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

		if isFriendly == aura.Positive {
			continue
		}

		chance := int32(100)
		candidates = append(candidates, dispelCandidate{
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
			SchoolMask: aura.SchoolMask,
		})
	}
	return candidates
}

// stealableAuraList builds the Spell::EffectStealBeneficialBuff (SpellEffects.cpp:5172)
// steal candidate list from the dispel-mask candidate list: positive auras only,
// skipping auras whose spell carries SPELL_ATTR4_NOT_STEALABLE. Passive auras are
// already excluded by the dispel helpers (the spellAttributePassive gate).
func (s *session) stealableAuraList(targetSess *session, targetGUID uint64, isTargetPlayer bool, dispelMask uint32) []dispelCandidate {
	var candidates []dispelCandidate
	if isTargetPlayer {
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask)
	} else {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask)
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
// SPELL_FAILED_BAD_TARGETS; a target carrying no stealable aura under the same skip
// list as EffectStealBeneficialBuff fails with SPELL_FAILED_NOTHING_TO_STEAL.
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
		if len(s.stealableAuraList(targetSess, targetGUID, isTargetPlayer, dispelMask)) == 0 {
			return spellFailedNothingToSteal
		}
	}
	return 0
}

// checkDispelPreCast mirrors TrinityCore Spell::CheckCast (Spell.cpp:5520-5565).
// Returns spellFailedNothingToDispel (86) if the spell only dispels and target has nothing to dispel.
func (s *session) checkDispelPreCast(spell wotlk.Spell, targetGUID uint64) uint8 {
	hasNonDispelEffect := false
	hasAreaDispel := false
	dispelMask := uint32(0)

	for _, eff := range spell.Effects {
		if eff.Effect == 38 { // SPELL_EFFECT_DISPEL
			if eff.ImplicitTargetA == 18 || eff.ImplicitTargetA == 24 || eff.ImplicitTargetA == 28 || eff.RadiusIndex > 0 {
				hasAreaDispel = true
				break
			}
			dispelMask |= getDispelMask(uint32(eff.MiscValue))
		} else if eff.Effect != 0 {
			hasNonDispelEffect = true
			break
		}
	}

	if hasNonDispelEffect || hasAreaDispel || dispelMask == 0 {
		return 0
	}

	// Target resolution
	var targetSess *session
	if targetGUID == s.playerGUID || targetGUID == 0 {
		targetSess = s
		targetGUID = s.playerGUID
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	var candidates []dispelCandidate
	if targetSess != nil && targetSess.player != nil {
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask)
	} else if targetGUID != 0 {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask)
	}

	if len(candidates) == 0 {
		return spellFailedNothingToDispel
	}
	return 0
}

func isDevourMagicSpell(spellID uint32) bool {
	switch spellID {
	case 19505, 19731, 19734, 19736, 27276, 27277:
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
			if aura.RemainingCharges > 1 {
				aura.RemainingCharges--
				advanceAuraDuration(aura, time.Now())
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
	var maxDuration, remaining, count uint32
	if aura != nil {
		slot, positive = aura.Slot, aura.Positive
		maxDuration, remaining = aura.DurationMs, aura.RemainingMs
		if dispelCharges {
			count = uint32(aura.RemainingCharges)
		} else {
			count = uint32(aura.StackCount)
		}
	}
	ts.castMu.Unlock()

	if aura == nil {
		return true
	}
	// A zero StackCount reads as a single stack (the auraUpdateRecords wire
	// default), so ModStackAmount(-1) on it removes the aura.
	if count <= 1 {
		ts.expirePlayerAura(spellID)
		return true
	}
	// Charges ride the stack-count field of the aura update (player_auras.go).
	ts.sendAuraUpdateWithStack(slot, spellID, false, positive, maxDuration, remaining, uint8(count))
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
			if aura.RemainingCharges > 1 {
				aura.RemainingCharges--
			}
		} else if aura.StackCount > 1 {
			oldCount := uint32(aura.StackCount)
			aura.StackCount--
			rescaleAuraAmountsByStack(aura, oldCount, uint32(aura.StackCount))
		}
	}
	var positive bool
	var count uint32
	if aura != nil {
		positive = aura.Positive
		if dispelCharges {
			count = uint32(aura.RemainingCharges)
		} else {
			count = uint32(aura.StackCount)
		}
	}
	s.server.auraMu.Unlock()

	if aura == nil || count <= 1 {
		s.expireCreatureAura(key, spellID, slot)
		return true
	}
	updatePkt := protocol.BuildAuraUpdateWithStack(key.GUID, s.playerGUID, slot, spellID, false, positive, 0, 0, 1, uint8(count))
	s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, nil)
	return false
}

func (s *session) handleEffectDispel(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s.player == nil {
		return
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
		candidates = s.getDispellableAuraListForPlayer(targetSess, dispelMask)
	} else {
		candidates = s.getDispellableAuraListForCreature(targetGUID, dispelMask)
	}

	if len(candidates) == 0 {
		return
	}

	maxDispelled := int(eff.BasePoints + 1)
	if maxDispelled <= 0 {
		maxDispelled = 1
	}

	var successList []uint32
	var failList []uint32

	for count := 0; count < maxDispelled && len(candidates) > 0; count++ {
		idx := rand.IntN(len(candidates))
		cand := candidates[idx]

		roll := rand.IntN(100)
		if int32(roll) < cand.Chance {
			// Dispel success
			successList = append(successList, cand.SpellID)
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

			// Devour Magic self-heal (SpellEffects.cpp:2520-2530)
			if isDevourMagicSpell(spell.ID) {
				healAmount := uint32(100)
				if len(spell.Effects) > 1 && spell.Effects[1].BasePoints > 0 {
					healAmount = uint32(spell.Effects[1].BasePoints + 1)
				}
				s.player.Health += healAmount
				if s.player.Health > s.player.MaxHealth {
					s.player.Health = s.player.MaxHealth
				}
				s.sendPlayerUpdate()
			}

			// Dispel Backfire / Backlash Mechanics (SpellEffects.cpp:2470-2485)
			// Unstable Affliction: deals 9 * tick damage (cand.Amount * 9) and silences dispeller for 5 seconds (spell 31117).
			if isUnstableAfflictionSpell(cand.SpellID) {
				backlashDamage := cand.Amount * 9
				if backlashDamage == 0 {
					backlashDamage = 1800
				}
				s.executeSpellDamage(ctx, s.playerGUID, 31117, backlashDamage, 0)

				silenceSpell := wotlk.Spell{
					ID:         31117,
					DispelType: DispelMagic,
					Mechanic:   9, // MECHANIC_SILENCE
				}
				silenceEff := wotlk.SpellEffect{
					Effect: 6,  // SPELL_EFFECT_APPLY_AURA
					Aura:   18, // SPELL_AURA_MOD_SILENCE
				}
				s.applyAuraToTarget(ctx, s.playerGUID, silenceSpell, silenceEff, 5000, 0, 0, 32, nil, false, s.playerGUID)
			}

			// Vampiric Touch: deals 2 * tick damage (cand.Amount * 2) to the dispeller (spell 64085).
			if isVampiricTouchSpell(cand.SpellID) {
				backlashDamage := cand.Amount * 2
				if backlashDamage == 0 {
					backlashDamage = 680
				}
				s.executeSpellDamage(ctx, s.playerGUID, 64085, backlashDamage, 0)
			}
		} else {
			// Dispel resisted / failed
			failList = append(failList, cand.SpellID)
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
		logPkt := buildSpellDispelLog(targetGUID, s.playerGUID, spell.ID, successList)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, true)
		if targetSess != nil && targetSess != s {
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, true)
		}
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLDISPELLOG), logPkt, s)
		}
	}
}

// stolenBuff carries a successfully stolen buff from the roll loop to the
// stealer-side apply: the candidate plus the victim aura's remaining
// duration and charge count captured before the steal decrement (C++ reads
// aura->GetDuration()/GetCharges() before mutating, Unit.cpp:3999/4032),
// plus the victim aura's caster GUID and instance pointer for the
// single-target steal dance (Unit.cpp:4015-4042).
type stolenBuff struct {
	candidate        dispelCandidate
	remainingMs      uint32
	charges          uint8
	stealCharge      bool
	victimCasterGUID uint64
	victimAura       *activeAura
}

// stealVictimState reads the victim aura's current remaining duration and
// charge count; degrades to the candidate's recorded duration when the aura
// is gone. Also returns the aura's caster GUID and instance pointer for the
// single-target steal dance.
func (s *session) stealVictimState(targetSess *session, targetGUID uint64, isTargetPlayer bool, cand dispelCandidate) (uint32, uint8, uint64, *activeAura) {
	if isTargetPlayer && targetSess != nil {
		targetSess.castMu.Lock()
		defer targetSess.castMu.Unlock()
		if aura := targetSess.activeAuras[cand.SpellID]; aura != nil && !aura.Stopped {
			advanceAuraDuration(aura, time.Now())
			return aura.RemainingMs, aura.RemainingCharges, aura.CasterGUID, aura
		}
		return cand.DurationMs, 0, 0, nil
	}
	if s.server != nil && s.player != nil {
		key := creatureAuraKeyForPlayer(*s.player, targetGUID)
		s.server.auraMu.Lock()
		defer s.server.auraMu.Unlock()
		if aura := s.server.activeCreatureAuras[key][cand.SpellID]; aura != nil && !aura.Stopped {
			return aura.RemainingMs, aura.RemainingCharges, aura.CasterGUID, aura
		}
	}
	return cand.DurationMs, 0, 0, nil
}

// mergeStolenAura mirrors the stealer-side merge in
// Unit::RemoveAurasDueToSpellBySteal (Unit.cpp:4002-4010): when the stealer
// already holds the stolen spell, the steal merges into it — one charge
// (Aura::ModCharges(+1), clamped to the spell's max charges) or one stack
// (Aura::ModStackAmount(+1), clamped to the spell's stack amount) — and the
// duration is reset to the capped stolen duration, instead of the aura being
// re-applied fresh. Reports whether a merge happened. Go holds one aura per
// spell ID, so the C++ caster-GUID match is vacuous.
func (s *session) mergeStolenAura(spellID uint32, stSpell wotlk.Spell, stealCharge bool, dur uint32) bool {
	s.castMu.Lock()
	existing := s.activeAuras[spellID]
	if existing == nil || existing.Stopped {
		s.castMu.Unlock()
		return false
	}
	if stealCharge {
		// Aura::ModCharges(+1, SpellAuras.cpp:964): increments while the
		// aura uses charges, clamped to CalcMaxCharges (the spell's proc
		// charges; SPELLMOD_CHARGES has no Go infra).
		if existing.RemainingCharges > 0 || stSpell.ProcCharges > 0 {
			charges := int32(existing.RemainingCharges) + 1
			if max := int32(stSpell.ProcCharges); max > 0 && charges > max {
				charges = max
			}
			existing.RemainingCharges = uint8(charges)
		}
	} else {
		// Aura::ModStackAmount(+1, SpellAuras.cpp:1030): clamped to the
		// spell's stack amount (1 when the spell is not stackable); an
		// increase refreshes the charge count — the SetDuration below
		// replaces the timer refresh.
		maxStack := int32(existing.StackAmount)
		if maxStack == 0 {
			maxStack = 1
		}
		cur := int32(existing.StackCount)
		if cur == 0 {
			cur = 1
		}
		oldCount := uint32(cur)
		increased := cur < maxStack
		if cur++; cur > maxStack {
			cur = maxStack
		}
		existing.StackCount = uint8(cur)
		// SetStackAmount recalc (SpellAuras.cpp:1008) on the increase: the
		// stolen stack must scale the amounts up, or a later decrement
		// would divide amounts that were never multiplied.
		rescaleAuraAmountsByStack(existing, oldCount, uint32(cur))
		if increased {
			existing.RemainingCharges = uint8(stSpell.ProcCharges)
		}
	}
	// oldAura->SetDuration(int32(dur)), Unit.cpp:4009.
	existing.DurationMs = dur
	existing.RemainingMs = dur
	existing.DurationUpdatedAt = time.Now()
	slot, positive := existing.Slot, existing.Positive
	var count uint8
	if stealCharge {
		count = existing.RemainingCharges
	} else {
		count = existing.StackCount
		if count == 0 {
			count = 1
		}
	}
	s.castMu.Unlock()
	// Charges ride the stack-count field of the aura update
	// (player_auras.go).
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, dur, dur, count)
	return true
}

// setAuraCharges fixes the stolen aura's charge count after the fresh apply
// (C++ SetLoadedState charges arg, Unit.cpp:4032) and pushes the corrected
// count to the client when it changed.
func (s *session) setAuraCharges(spellID uint32, charges uint8) {
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped || aura.RemainingCharges == charges {
		s.castMu.Unlock()
		return
	}
	aura.RemainingCharges = charges
	slot, positive := aura.Slot, aura.Positive
	maxDuration, remaining := aura.DurationMs, aura.RemainingMs
	s.castMu.Unlock()
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, maxDuration, remaining, charges)
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
	stealable := s.stealableAuraList(targetSess, targetGUID, isTargetPlayer, dispelMask)

	if len(stealable) == 0 {
		return
	}

	maxDispelled := int(eff.BasePoints + 1)
	if maxDispelled <= 0 {
		maxDispelled = 1
	}

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
			remaining, charges, victimCasterGUID, victimAura := s.stealVictimState(targetSess, targetGUID, isTargetPlayer, cand)
			stolen = append(stolen, stolenBuff{candidate: cand, remainingMs: remaining, charges: charges, stealCharge: stealCharge, victimCasterGUID: victimCasterGUID, victimAura: victimAura})
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

	// Apply stolen buffs to the stealer (duration capped at 2 minutes /
	// 120000ms per TC, Unit.cpp:3999).
	for _, st := range stolen {
		cand := st.candidate
		dur := st.remainingMs
		if dur == 0 || dur > 120000 {
			dur = 120000
		}
		stSpell := wotlk.Spell{ID: cand.SpellID, DispelType: cand.DispelType, Mechanic: cand.Mechanic}
		if s.server != nil && s.server.Data != nil {
			if loaded, found, _ := s.server.Data.Spell(cand.SpellID); found {
				stSpell = loaded
			}
		}
		if s.mergeStolenAura(cand.SpellID, stSpell, st.stealCharge, dur) {
			continue
		}
		// Unit::RemoveAurasDueToSpellBySteal single-target dance
		// (Unit.cpp:4015-4042): the _AddAura dance runs on the stolen aura
		// with the ORIGINAL caster, purging that caster's other
		// single-target-with auras — but the created aura itself must not
		// stay single-target, "so stealer won't loose it on recast". The
		// victim aura was already unregistered from the original caster's
		// list before the dance in C++; here it is excluded by pointer when
		// it survived the steal decrement (a fully removed victim aura
		// unregistered itself on expiry). The stolen aura is created with
		// single-target registration skipped.
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
		s.applyAuraToTarget(ctx, s.playerGUID, stSpell, eff, dur, cand.PeriodMs, cand.Amount, cand.SchoolMask, nil, true, stealCasterGUID)
		// C++ SetLoadedState charges arg (Unit.cpp:4032): 1 for
		// ATTR7_DISPEL_CHARGES auras, the victim's charge count otherwise.
		wantCharges := st.charges
		if st.stealCharge {
			wantCharges = 1
		}
		s.setAuraCharges(cand.SpellID, wantCharges)
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
		targetSess.castMu.Lock()
		var toRemove []uint32
		for spellID, aura := range targetSess.activeAuras {
			if aura == nil || aura.Stopped {
				continue
			}
			matchesMechanic := (aura.Mechanic == mechanic)
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
				toRemove = append(toRemove, spellID)
			}
		}
		targetSess.castMu.Unlock()

		for _, spID := range toRemove {
			targetSess.expirePlayerAura(spID)
		}
	} else if targetGUID != 0 && s.server != nil {
		s.server.auraMu.Lock()
		key := creatureAuraKeyForPlayer(*s.player, targetGUID)
		auras := s.server.activeCreatureAuras[key]
		var toRemove []struct {
			spellID uint32
			slot    uint8
		}
		for spellID, aura := range auras {
			if aura == nil || aura.Stopped {
				continue
			}
			if aura.Mechanic == mechanic || (mechanic == 5 && aura.AuraType == 7) || (mechanic == 12 && aura.AuraType == 12) {
				toRemove = append(toRemove, struct {
					spellID uint32
					slot    uint8
				}{spellID: spellID, slot: aura.Slot})
			}
		}
		s.server.auraMu.Unlock()

		for _, item := range toRemove {
			s.expireCreatureAura(key, item.spellID, item.slot)
		}
	}
}
