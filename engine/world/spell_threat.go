package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellThreatEntry mirrors SpellThreatEntry (SpellMgr.h:340): one row of the
// world `spell_threat` table (SpellMgr.cpp:1927).
type spellThreatEntry struct {
	flatMod  float32
	pctMod   float32
	apPctMod float32
}

// loadSpellThreatEntry fetches the spell_threat row for a spell. A missing
// table or row degrades to "no entry", which is exactly C++'s fallback to
// the SpellLevel-based formula (Spell.cpp:5116-5118).
func (s *session) loadSpellThreatEntry(ctx context.Context, spellID uint32) (spellThreatEntry, bool) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return spellThreatEntry{}, false
	}
	var row spellThreatEntry
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT flatMod, pctMod, apPctMod FROM spell_threat WHERE entry = ?", spellID).
		Scan(&row.flatMod, &row.pctMod, &row.apPctMod)
	if err != nil {
		return spellThreatEntry{}, false
	}
	return row, true
}

// spellHasInitialThreat mirrors SpellInfo::HasInitialAggro (SpellInfo.cpp:1261).
func spellHasInitialThreat(spell wotlk.Spell) bool {
	return spell.AttributesEx&spellAttr1NoThreat == 0 && spell.AttributesEx3&spellAttr3NoInitialAggro == 0
}

// spellSuppressesInitialThreat mirrors the SPELL_ATTR0_CU_NO_INITIAL_THREAT
// assignment in SpellMgr::LoadSpellInfoCorrections (SpellMgr.cpp:2700-2710):
// heal/energize-class effects carry no initial threat.
func spellSuppressesInitialThreat(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		switch eff.Effect {
		case spellEffectPowerDrain, spellEffectPowerBurn, spellEffectHealMaxHealth,
			spellEffectHealthLeech, spellEffectHealPct, spellEffectEnergizePct,
			spellEffectEnergize, spellEffectHealMechanical:
			return true
		}
	}
	return false
}

// handleSpellInitialThreat mirrors Spell::HandleThreatSpells (Spell.cpp:5096-5150).
// Every cast with initial aggro adds spell_threat-row threat
// (flatMod + apPctMod * base attack power) or, without a row, the spell's
// SpellLevel, split evenly across the hit targets, before any effect-damage
// threat lands. Missed targets carry zero threat (Spell.cpp:5125); Go's
// hitTargets only ever holds hits, so that term needs no code. Positive
// spells forward the threat to every creature currently threatening the
// target (ThreatManager::ForwardThreatForAssistingMe, ThreatManager.cpp:662);
// negative spells add it to creature targets directly (ignoreModifiers, so
// no caster multiplier on this path).
func (s *session) handleSpellInitialThreat(ctx context.Context, spell wotlk.Spell, hitTargets []uint64) {
	if s == nil || s.server == nil || s.player == nil || len(hitTargets) == 0 {
		return
	}
	if !spellHasInitialThreat(spell) {
		return
	}
	var threat float32
	var pctMod float32 = 1.0
	if entry, ok := s.loadSpellThreatEntry(ctx, spell.ID); ok {
		if entry.apPctMod != 0 {
			threat += entry.apPctMod * float32(s.player.AttackPower)
		}
		threat += entry.flatMod
		pctMod = entry.pctMod
	} else if !spellSuppressesInitialThreat(spell) {
		threat += float32(spell.SpellLevel)
	}
	if threat == 0 {
		return
	}
	// Spell.cpp:5123: the defined bonus is distributed among all targets.
	threat /= float32(len(hitTargets))

	positive := !isHarmfulSpell(spell)
	// Caster-side threat modifiers (ThreatManager::CalculateModifiedThreat via
	// the attacker's own manager, ThreatManager.cpp:606) apply to the assist
	// path; the negative path passes ignoreModifiers.
	assistMult := float32(1.0)
	if positive {
		assistMult = s.getThreatMultiplier(uint32(spell.SchoolMask))
	}

	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		if positive {
			s.forwardInitialAssistThreatLocked(threat*pctMod*assistMult, targetGUID)
			continue
		}
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		if motion == nil {
			continue
		}
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(motion.GUID)
		}
		dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
		inMelee := dist <= meleeAttackRange
		switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
		if switched && newVictim != motion.TargetGUID {
			motion.TargetGUID = newVictim
			entries := motion.ThreatMgr.SortedEntries()
			s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
		} else {
			motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
		}
		motion.InCombat = true
		motion.Moving = true
	}
}

// forwardInitialAssistThreatLocked mirrors
// ThreatManager::ForwardThreatForAssistingMe (ThreatManager.cpp:662-688): the
// initial threat of a positive spell is split evenly among all creatures
// currently threatening the target. Caller holds motionMu.
func (s *session) forwardInitialAssistThreatLocked(threat float32, targetGUID uint64) {
	if threat <= 0 {
		return
	}
	var assisting []*creatureMotion
	for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if m == nil || m.GUID == targetGUID || m.ThreatMgr == nil {
			continue
		}
		if m.ThreatMgr.GetThreat(targetGUID) <= 0 {
			continue
		}
		assisting = append(assisting, m)
	}
	if len(assisting) == 0 {
		return
	}
	perTarget := threat / float32(len(assisting))
	for _, m := range assisting {
		dist := distance3D(s.player.X, s.player.Y, s.player.Z, m.X, m.Y, m.Z)
		inMelee := dist <= meleeAttackRange
		switched, newVictim := m.ThreatMgr.AddThreat(s.playerGUID, perTarget, inMelee)
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.server.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}
}
