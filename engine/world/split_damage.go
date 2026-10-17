package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// SPELL_AURA_SPLIT_DAMAGE_PCT (81) and SPELL_AURA_SPLIT_DAMAGE_FLAT (153)
// redirect part of a victim's post-absorb damage to the aura's caster
// (Unit::CalcAbsorbResist, Unit.cpp:1977-2060): Soul Link, Blessing of
// Sacrifice. The flat leg runs before the pct leg; each aura's split is
// absorbed from the victim's running damage first, then dealt to the caster
// as a full SPELL_DIRECT_DAMAGE recursive hit (DealDamageMods zeroing, the
// immunity miss, the attacker's non-melee damage log, then the shared
// direct-damage tails — which run the nested split legs again, matching
// C++'s recursive DealDamage).

const (
	spellAuraSplitDamagePct  = 81  // SPELL_AURA_SPLIT_DAMAGE_PCT (SpellAuraDefines.h)
	spellAuraSplitDamageFlat = 153 // SPELL_AURA_SPLIT_DAMAGE_FLAT (SpellAuraDefines.h)
)

// splitDamageAura is one live split-damage effect on a damage victim.
type splitDamageAura struct {
	spellID    uint32
	casterGUID uint64
	amount     int32
	isPct      bool
}

// collectSplitDamageAuras snapshots the victim's live 153 (flat) then 81
// (pct) effects whose misc school mask covers the damage school
// (Unit.cpp:1977-2020). The C++ arm copies the aura list before iterating
// so mid-iteration removals are safe; the snapshot plays that role. Entries
// no longer applied on the victim (TargetGUID mismatch) are skipped,
// mirroring the IsAppliedOnTarget check. With several split auras live, C++
// consumes them in application order while Go's aura map iterates randomly;
// the single-aura case (every real 81/153 spell) is unaffected.
func (s *Server) collectSplitDamageAuras(victimGUID uint64, victimIsPlayer bool, key creatureAuraKey, schoolMask uint32) []splitDamageAura {
	if s == nil || s.Data == nil || schoolMask == 0 {
		return nil
	}
	var out []splitDamageAura
	// Unit.cpp:1977 runs the flat (153) leg before the pct (81) leg (:2019).
	for _, auraType := range []uint32{spellAuraSplitDamageFlat, spellAuraSplitDamagePct} {
		isPct := auraType == spellAuraSplitDamagePct
		accept := func(aura *activeAura) {
			if aura == nil || aura.Stopped || aura.AuraType != auraType {
				return
			}
			if aura.TargetGUID != 0 && aura.TargetGUID != victimGUID {
				return
			}
			if aura.CasterGUID == 0 {
				return
			}
			spell, found, err := s.Data.Spell(aura.SpellID)
			if err != nil || !found {
				return
			}
			for index, effect := range spell.Effects {
				if aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraType {
					continue
				}
				if uint32(effect.MiscValue)&schoolMask == 0 {
					continue
				}
				amount := int32(0)
				if index < len(aura.Amounts) {
					amount = aura.Amounts[index]
				}
				if amount == 0 {
					amount = int32(aura.Amount)
				}
				if amount <= 0 {
					continue
				}
				out = append(out, splitDamageAura{spellID: aura.SpellID, casterGUID: aura.CasterGUID, amount: amount, isPct: isPct})
			}
		}
		if victimIsPlayer {
			vs := s.findSessionByGUID(victimGUID)
			if vs == nil || vs.player == nil {
				return nil
			}
			vs.castMu.Lock()
			for _, aura := range vs.activeAuras {
				accept(aura)
			}
			vs.castMu.Unlock()
			continue
		}
		s.auraMu.Lock()
		for _, aura := range s.activeCreatureAuras[key] {
			accept(aura)
		}
		s.auraMu.Unlock()
	}
	return out
}

// splitDamageToCaster mirrors Unit::CalcAbsorbResist's split-damage legs
// (Unit.cpp:1977-2060): each live 153/81 effect on the victim absorbs its
// share from the running damage and redirects it to the aura's caster as a
// full SPELL_DIRECT_DAMAGE hit. Returns the victim's remaining damage. The
// split runs only when the victim is not damaging itself, and only while
// damage remains. The pct leg's CallScriptEffectSplitHandlers has no Go
// bridge (documented delta). The god-mode corner deviates: C++ runs the
// split legs in the taken pipeline before DealDamage's god arm, while Go
// hooks the split at the DealDamage-equivalent point after the god fold —
// a cheat-mode-only corner.
func (s *Server) splitDamageToCaster(ctx context.Context, victimGUID uint64, victimIsPlayer bool, key creatureAuraKey, attackerGUID uint64, damage uint32, schoolMask uint32) uint32 {
	if s == nil || damage == 0 || schoolMask == 0 || victimGUID == attackerGUID {
		return damage
	}
	auras := s.collectSplitDamageAuras(victimGUID, victimIsPlayer, key, schoolMask)
	if len(auras) == 0 {
		return damage
	}
	for _, a := range auras {
		if damage == 0 {
			break
		}
		var split uint32
		if a.isPct {
			// Unit.cpp:2033: CalculatePct(damage, amount), clamped to the
			// running damage.
			split = uint32(uint64(damage) * uint64(a.amount) / 100)
		} else if a.amount > 0 {
			// Unit.cpp:1992: the flat amount, clamped to the damage.
			split = uint32(a.amount)
		}
		if split == 0 {
			continue
		}
		if split > damage {
			split = damage
		}
		// Unit.cpp:1994/2039: the victim absorbs the split before the
		// caster leg, so immunity still spares the victim.
		damage -= split
		s.dealSplitDamageToCaster(ctx, victimGUID, victimIsPlayer, key, attackerGUID, a, split, schoolMask)
	}
	return damage
}

// dealSplitDamageToCaster deals one aura's redirected split to its caster:
// the DealDamageMods zeroing (Unit.cpp:686-694), the immunity miss
// (Unit.cpp:1995-2000, 2041-2046), the attacker's non-melee damage log
// (Unit.cpp:2006-2012, 2052-2058) and the recursive
// DealDamage(attacker, caster, split, DIRECT_DAMAGE, school, aura spell,
// durabilityLoss=false) (Unit.cpp:2014, 2060) through the shared
// direct-damage tails. The pct leg's fear-break proc
// (PROC_FLAG_TAKEN_SPELL_MAGIC_DMG_CLASS_NEG, Unit.cpp:2062) rides the
// taken proc pass of the recursive hit (documented delta on the exact
// flag mapping).
func (s *Server) dealSplitDamageToCaster(ctx context.Context, victimGUID uint64, victimIsPlayer bool, key creatureAuraKey, attackerGUID uint64, a splitDamageAura, split uint32, schoolMask uint32) {
	if s == nil || split == 0 {
		return
	}
	if a.casterGUID == victimGUID {
		return
	}
	// Unit::DealDamageMods (Unit.cpp:686-694): a dead, in-flight or evading
	// caster zeroes the split. The evade/in-flight arms have no Go
	// player-side model; the creature side resolves through the motion
	// lookup like the shield path.
	var casterHealth uint32
	casterSess := s.findSessionByGUID(a.casterGUID)
	casterIsPlayer := false
	var casterMotion *creatureMotion
	var casterKey creatureAuraKey
	if casterSess != nil && casterSess.player != nil && casterSess.player.Health > 0 {
		casterIsPlayer = true
		casterHealth = casterSess.player.Health
	} else {
		casterSess = nil
		casterKey = creatureAuraKey{Map: key.Map, InstanceID: key.InstanceID, GUID: a.casterGUID}
		casterMotion = s.findCreatureMotion(casterKey.Map, casterKey.InstanceID, casterKey.GUID)
		if casterMotion == nil || casterMotion.Health == 0 {
			return
		}
		casterHealth = casterMotion.Health
	}
	// Unit.cpp:1995-2000, 2041-2046: an immune caster takes nothing — the
	// victim still absorbed the split above, and the victim sends the miss.
	if casterIsPlayer {
		if casterSess.isImmuneToDamage(schoolMask) {
			s.sendSplitImmuneMiss(victimGUID, victimIsPlayer, a.casterGUID, a.spellID)
			return
		}
	} else if s.Data != nil {
		if spell, found, err := s.Data.Spell(a.spellID); err == nil && found {
			if creatureImmuneToDamageSpell(s, casterKey, spell, schoolMask) {
				s.sendSplitImmuneMiss(victimGUID, victimIsPlayer, a.casterGUID, a.spellID)
				return
			}
		}
	}
	// Unit.cpp:2006-2012, 2052-2058: the attacker logs the split hit on the
	// caster before the recursive DealDamage.
	s.sendSplitDamageLog(attackerGUID, a.casterGUID, a.spellID, split, casterHealth, schoolMask, key.Map, key.InstanceID)
	// Unit.cpp:2014, 2060: the recursive hit is a full DealDamage with the
	// original attacker, the aura's spell, and durabilityLoss=false.
	attackerSess := s.findSessionByGUID(attackerGUID)
	if casterIsPlayer {
		s.dealDirectDamageToPlayer(ctx, attackerGUID, attackerSess, casterSess, a.spellID, schoolMask, split)
	} else {
		s.dealDirectDamageToCreature(ctx, attackerGUID, attackerSess, casterMotion, casterKey, a.spellID, schoolMask, true, split)
	}
}

// sendSplitImmuneMiss has the victim report the immune split caster
// (Unit.cpp:1998, 2044: victim->SendSpellMiss(caster, spell, SPELL_MISS_IMMUNE)).
// Creature victims have no miss-packet path (same delta as the shield hit
// roll).
func (s *Server) sendSplitImmuneMiss(victimGUID uint64, victimIsPlayer bool, casterGUID uint64, spellID uint32) {
	if s == nil || !victimIsPlayer {
		return
	}
	if vs := s.findSessionByGUID(victimGUID); vs != nil {
		vs.sendSpellMiss(casterGUID, spellID, protocol.SpellMissImmune)
	}
}

// sendSplitDamageLog sends the attacker's SMSG_SPELLNONMELEEDAMAGELOG for the
// redirected split (Unit.cpp:2009, 2055). Unit::SendSpellNonMeleeDamageLog
// ends in SendMessageToSet on the attacker — the attacker's visibility set —
// so a player attacker writes its own copy and fans out from its position,
// and a creature attacker fans out from its motion position with no session
// leg. The attacker's map/instance comes from the victim's key (the split
// runs where the original damage landed).
func (s *Server) sendSplitDamageLog(attackerGUID, casterGUID uint64, spellID, split, casterHealth uint32, schoolMask uint32, mapID, instanceID uint32) {
	if s == nil || split == 0 {
		return
	}
	var overkill uint32
	if split > casterHealth {
		overkill = split - casterHealth
	}
	logPkt := buildSpellNonMeleeDamageLog(casterGUID, attackerGUID, spellID, split, overkill, uint8(schoolMask))
	var source *session
	var cx, cy, cz float32
	if as := s.findSessionByGUID(attackerGUID); as != nil && as.player != nil {
		source = as
		mapID, instanceID = as.player.Map, as.player.InstanceID
		cx, cy, cz = as.player.X, as.player.Y, as.player.Z
		_ = as.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
	} else if m := s.findCreatureMotion(mapID, instanceID, attackerGUID); m != nil {
		mapID, instanceID = m.Map, m.InstanceID
		cx, cy, cz = m.X, m.Y, m.Z
	} else {
		return
	}
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, mapID, instanceID, cx, cy, cz, source)
}
