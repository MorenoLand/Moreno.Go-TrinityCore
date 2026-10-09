package world

import (
	"context"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// redirectThreatTarget mirrors one flattened entry of
// ThreatManager::_redirectInfo (ThreatManager.cpp:829-845): the redirect
// victim plus its percent share of the caster's generated threat.
type redirectThreatTarget struct {
	spellID uint32
	victim  uint64
	pct     uint32
}

// registerRedirectThreat mirrors ThreatManager::RegisterRedirectThreat
// (ThreatManager.cpp:727-731): (spellId, victim) -> pct on the caster's
// registry, followed by the UpdateRedirectInfo flatten.
func (s *session) registerRedirectThreat(spellID uint32, victimGUID uint64, pct uint32) {
	if s == nil || victimGUID == 0 || pct == 0 {
		return
	}
	s.redirectMu.Lock()
	defer s.redirectMu.Unlock()
	if s.redirectThreatRegistry == nil {
		s.redirectThreatRegistry = make(map[uint32]map[uint64]uint32)
	}
	victims := s.redirectThreatRegistry[spellID]
	if victims == nil {
		victims = make(map[uint64]uint32)
		s.redirectThreatRegistry[spellID] = victims
	}
	victims[victimGUID] = pct
	s.flattenRedirectThreatInfoLocked()
}

// unregisterRedirectThreat mirrors ThreatManager::UnregisterRedirectThreat(uint32)
// (ThreatManager.cpp:733-740).
func (s *session) unregisterRedirectThreat(spellID uint32) {
	if s == nil {
		return
	}
	s.redirectMu.Lock()
	defer s.redirectMu.Unlock()
	if s.redirectThreatRegistry == nil {
		return
	}
	delete(s.redirectThreatRegistry, spellID)
	s.flattenRedirectThreatInfoLocked()
}

// unregisterRedirectThreatVictim mirrors
// ThreatManager::UnregisterRedirectThreat(uint32, ObjectGuid)
// (ThreatManager.cpp:742-753): drops one victim under a spell id, e.g. the
// Vigilance (50720) aura remove unregistering (59665, warrior) from the
// vigilance target's manager (spell_warrior.cpp:1128).
func (s *session) unregisterRedirectThreatVictim(spellID uint32, victimGUID uint64) {
	if s == nil {
		return
	}
	s.redirectMu.Lock()
	defer s.redirectMu.Unlock()
	victims, ok := s.redirectThreatRegistry[spellID]
	if !ok {
		return
	}
	delete(victims, victimGUID)
	if len(victims) == 0 {
		delete(s.redirectThreatRegistry, spellID)
	}
	s.flattenRedirectThreatInfoLocked()
}

// flattenRedirectThreatInfoLocked mirrors ThreatManager::UpdateRedirectInfo
// (ThreatManager.cpp:829-845): every (spell, victim) pair contributes
// min(100-total, pct), the total capped at 100. C++ iterates std::map
// (key-sorted); Go sorts by spell id then victim for determinism.
// Caller holds redirectMu.
func (s *session) flattenRedirectThreatInfoLocked() {
	s.redirectThreatInfo = s.redirectThreatInfo[:0]
	if len(s.redirectThreatRegistry) == 0 {
		return
	}
	spellIDs := make([]uint32, 0, len(s.redirectThreatRegistry))
	for spellID := range s.redirectThreatRegistry {
		spellIDs = append(spellIDs, spellID)
	}
	sort.Slice(spellIDs, func(i, j int) bool { return spellIDs[i] < spellIDs[j] })
	var total uint32
	for _, spellID := range spellIDs {
		victims := s.redirectThreatRegistry[spellID]
		guids := make([]uint64, 0, len(victims))
		for guid := range victims {
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		for _, guid := range guids {
			thisPct := victims[guid]
			if total < 100 && thisPct > 100-total {
				thisPct = 100 - total
			}
			if thisPct == 0 {
				continue
			}
			s.redirectThreatInfo = append(s.redirectThreatInfo, redirectThreatTarget{spellID: spellID, victim: guid, pct: thisPct})
			total += thisPct
			if total == 100 {
				return
			}
		}
	}
}

// splitThreatRedirects mirrors the redirect leg of ThreatManager::AddThreat
// (ThreatManager.cpp:344-373) for threat the session's player generates on
// motion's threat list. Each flattened entry whose victim resolves takes
// CalculatePct(origAmount, pct) of the ORIGINAL amount onto the victim's own
// entry (the recursive AddThreat(redirTarget, amountRedirected, spell, true,
// true): no threat modifiers, no nested redirect — Go's direct ThreatManager
// add is exactly that). Victim lookup is the threat-list entry first, then
// the world (C++ _myThreatListEntries, then ObjectAccessor); a victim that
// resolves nowhere keeps its share with the caster (the C++ `if (redirTarget)`
// gate only subtracts on resolution). Only positive amounts redirect.
// Caller holds motionMu. Returns the remainder for the caster's own entry,
// plus whether any add switched the victim and the resulting victim.
func (s *session) splitThreatRedirects(motion *creatureMotion, amount float32) (remaining float32, switched bool, newVictim uint64) {
	remaining = amount
	if s == nil || s.server == nil || motion == nil || motion.ThreatMgr == nil || amount <= 0 {
		return remaining, false, motion.ThreatMgr.GetCurrentVictim()
	}
	s.redirectMu.Lock()
	info := make([]redirectThreatTarget, len(s.redirectThreatInfo))
	copy(info, s.redirectThreatInfo)
	s.redirectMu.Unlock()
	if len(info) == 0 {
		return remaining, false, motion.ThreatMgr.GetCurrentVictim()
	}
	for _, r := range info {
		share := amount * float32(r.pct) / 100.0
		if share <= 0 {
			continue
		}
		var victimReach float32
		var vx, vy, vz float32
		resolved := false
		if ts := s.server.findSessionByGUID(r.victim); ts != nil && ts.player != nil {
			victimReach = ts.player.CombatReach
			vx, vy, vz = ts.player.X, ts.player.Y, ts.player.Z
			resolved = true
		} else if vm := s.server.findCreatureMotionLocked(motion.Map, motion.InstanceID, r.victim); vm != nil {
			victimReach = vm.CombatReach
			vx, vy, vz = vm.X, vm.Y, vm.Z
			resolved = true
		}
		if !resolved {
			continue
		}
		dist := distance3D(motion.X, motion.Y, motion.Z, vx, vy, vz)
		inMelee := inMeleeThreatRange(motion.CombatReach, victimReach, dist)
		if sw, nv := motion.ThreatMgr.AddThreat(r.victim, share, inMelee); sw {
			switched, newVictim = true, nv
		}
		remaining -= share
	}
	if !switched {
		newVictim = motion.ThreatMgr.GetCurrentVictim()
	}
	return remaining, switched, newVictim
}

// handleEffectRedirectThreat processes SPELL_EFFECT_REDIRECT_THREAT (130).
// Reference: Spell::EffectRedirectThreat (SpellEffects.cpp:5411-5421) runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per unit target, registering
// (spellId, unitTarget, damage) on the caster's ThreatManager redirect
// registry (ThreatManager.cpp:727-729). damage is the effect's BasePoints+1
// (100 for the Misdirection/Tricks of the Trade/Vigilance redirect spells in
// DBC). There is no alive check in C++; the registry key is the redirect
// spell's own id (e.g. 35079), unregistered when its aura is removed.
func (s *session) handleEffectRedirectThreat(spellID uint32, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.player == nil {
		return
	}
	damage := eff.BasePoints + 1
	if damage <= 0 {
		return
	}
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		s.registerRedirectThreat(spellID, targetGUID, uint32(damage))
	}
}

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

// damageThreatAmount folds ThreatManager::CalculateModifiedThreat
// (ThreatManager.cpp:606-659) onto a damaging spell's DealDamage threat.
// Unit::DealDamage (Unit.cpp:900-907) calls AddThreat(attacker, damage,
// spellProto) with the default args (ignoreModifiers=false,
// ignoreRedirects=false), so a damaging spell's threat folds the
// spell_threat-row pctMod, the attacker's SPELLMOD_THREAT spell mods, then
// the attacker's own school threat multipliers — in that C++ order. (The
// redirect consumption, also a default-false leg, is applied separately by
// splitThreatRedirects at each damage site.) Damage from melee swings and
// auto-shots passes a null spell, which skips the row and mod legs, so those
// funnels keep folding only the physical-school multiplier. Damage spells
// carrying SPELL_ATTR1_NO_THREAT add nothing at all (AddThreat's step-1
// early return, ThreatManager.cpp:311-314), and SPELL_ATTR3_NO_INITIAL_AGGRO
// adds nothing while the target is not yet engaged
// (ThreatManager.cpp:315-317; engaged is the victim's combat state as seen
// before this damage). A zero return skips the threat add entirely.
func (s *session) damageThreatAmount(ctx context.Context, spellID uint32, schoolMask uint32, damage float32, engaged bool) float32 {
	if s == nil || s.server == nil || damage <= 0 {
		return 0
	}
	var spell wotlk.Spell
	spellKnown := false
	if spellID != 0 {
		if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
			spell, spellKnown = sp, true
		}
	}
	if spellKnown {
		if spell.AttributesEx&spellAttr1NoThreat != 0 {
			return 0
		}
		if spell.AttributesEx3&spellAttr3NoInitialAggro != 0 && !engaged {
			return 0
		}
	}
	// ThreatManager::CalculateModifiedThreat (ThreatManager.cpp:606-659):
	// the spell_threat-row pctMod applies even though its flatMod/apPctMod
	// arms stay on the cast-threat path, then the attacker's SPELLMOD_THREAT
	// mods, then the attacker's own school multipliers — the C++ order.
	t := float64(damage)
	if spellID != 0 {
		if entry, ok := s.loadSpellThreatEntry(ctx, spellID); ok && entry.pctMod != 1.0 {
			t *= float64(entry.pctMod)
		}
		if spellKnown {
			t = s.applySpellModFloat(spell, spellModThreat, t)
		}
	}
	return float32(t) * s.getThreatMultiplier(schoolMask)
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

// handleSpellInitialThreat mirrors Spell::HandleThreatSpells (Spell.cpp:5096-5151).
// Every cast with initial aggro adds spell_threat-row threat
// (flatMod + apPctMod * base attack power) or, without a row, the spell's
// SpellLevel, split evenly across ALL unique targets — hits and misses alike
// (Spell.cpp:5123) — before any effect-damage threat lands; missed shares are
// wasted, and missed targets carry zero threat (Spell.cpp:5125) but still
// take the zero-threat add: positive spells forward it to every creature
// currently threatening the missed target
// (ThreatManager::ForwardThreatForAssistingMe, ThreatManager.cpp:662), and
// negative spells AddThreat the caster at 0 on the missed target's list,
// which still creates the ref and engages combat (ThreatManager.cpp:308-410).
// Positive spells forward the threat to every creature currently threatening
// the target (ThreatManager::ForwardThreatForAssistingMe, ThreatManager.cpp:662);
// negative spells add it to creature targets directly (ignoreModifiers, so
// no caster multiplier on this path). Only player casts route through this
// path: C++ runs HandleThreatSpells for any unit caster, but Go creature and
// pet casts resolve instantly (executePetSpellWithOptions) with no
// initial-threat pass. The negative half is vacuous for creature casts on
// players — players cannot have threat lists (ThreatManager.cpp:156-179),
// which is exactly C++'s CanHaveThreatList skip; creature-on-creature casts
// and creature heal forwarding (a creature as the ForwardThreatForAssistingMe
// assistant) have no Go analog. C++ takes the caster from m_originalCaster
// when set (Spell.cpp:5098); Go always uses the session player.
func (s *session) handleSpellInitialThreat(ctx context.Context, spell wotlk.Spell, hitTargets []uint64, missGUIDs []uint64) {
	if s == nil || s.server == nil || s.player == nil || (len(hitTargets) == 0 && len(missGUIDs) == 0) {
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
	// Spell.cpp:5123: the defined bonus is distributed among all unique
	// targets, misses included (their shares are wasted, Spell.cpp:5125).
	totalTargets := len(hitTargets) + len(missGUIDs)
	if totalTargets <= 0 {
		return
	}
	threat /= float32(totalTargets)

	positive := !isHarmfulSpell(spell)
	// The positive path runs through ForwardThreatForAssistingMe without
	// ignoreModifiers, so each recipient's AddThreat folds
	// ThreatManager::CalculateModifiedThreat (ThreatManager.cpp:606-659):
	// spell_threat-row pctMod, the assistant's SPELLMOD_THREAT spell mods,
	// then the assistant's own school threat multipliers (the C++ order) —
	// all assistant-constant, so folded once here.
	assistThreat := float32(0)
	if positive {
		t := float64(threat * pctMod)
		t = s.applySpellModFloat(spell, spellModThreat, t)
		assistThreat = float32(t) * s.getThreatMultiplier(uint32(spell.SchoolMask))
	}

	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	// Missed targets are not in hitTargets, so dedupe them here (C++
	// iterates m_UniqueTargetInfo, which holds each target once).
	hitSet := make(map[uint64]struct{}, len(hitTargets))
	for _, g := range hitTargets {
		hitSet[g] = struct{}{}
	}
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		if positive {
			s.forwardInitialAssistThreatLocked(assistThreat, targetGUID)
			continue
		}
		s.addInitialNegativeThreatLocked(targetGUID, threat)
	}
	for _, missGUID := range missGUIDs {
		if missGUID == 0 {
			continue
		}
		if _, dup := hitSet[missGUID]; dup {
			continue
		}
		// Spell.cpp:5128-5130: a missed target takes a zero-threat add.
		if positive {
			s.forwardInitialAssistThreatLocked(0, missGUID)
			continue
		}
		s.addInitialNegativeThreatLocked(missGUID, 0)
	}
}

// addInitialNegativeThreatLocked runs the Spell.cpp:5143 negative leg on one
// creature motion: the CanHaveThreatList skip (ThreatManager.cpp:156-179 —
// pets, totems, triggers, player-summoned minions, npcbots; in Go, owned
// creature motions with OwnerGUID != 0 are the modeled members of that set),
// then AddThreat(unitCaster, threatToAdd, m_spellInfo, true)
// (Spell.cpp:5143): ignoreModifiers=true so neither the spell_threat-row
// pctMod, SPELLMOD_THREAT spell mods, nor school multipliers apply, while
// ignoreRedirects stays false and the caster's redirect registry still
// consumes (splitThreatRedirects; a no-op at amount <= 0 per the C++
// amount > 0 gate, ThreatManager.cpp:347). A zero add still creates the
// threat ref and engages combat (ThreatManager.cpp:308-410). The vehicle
// redirect leg (ThreatManager.cpp:316-322) has no Go analog — Go models no
// vehicle threat. Caller holds motionMu.
func (s *session) addInitialNegativeThreatLocked(targetGUID uint64, amount float32) {
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil {
		return
	}
	// Spell.cpp:5143-5144: the negative path skips targets that cannot
	// have a threat list (ThreatManager::CanHaveThreatList,
	// ThreatManager.cpp:156-179 — charmed creatures keep their lists).
	if !motionCanHaveThreatList(motion) {
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion)
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
	inMelee := inMeleeThreatRange(motion.CombatReach, s.player.CombatReach, dist)
	amount, rSwitched, rVictim := s.splitThreatRedirects(motion, amount)
	switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, amount, inMelee)
	if rSwitched {
		switched, newVictim = true, rVictim
	}
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

// forwardInitialAssistThreatLocked mirrors
// ThreatManager::ForwardThreatForAssistingMe (ThreatManager.cpp:662-688): the
// initial threat of a positive spell is split evenly among all creatures
// currently threatening the target. C++ carries no zero-threat early return —
// a missed target's zero add still splits (perTarget 0) and runs AddThreat,
// creating the refs. The caller folds CalculateModifiedThreat
// (ThreatManager.cpp:606-659) — spell_threat-row pctMod, the assistant's
// SPELLMOD_THREAT spell mods, then the assistant's own school multipliers —
// since spell and assistant are constant across recipients. Caller holds
// motionMu.
// Documented deltas: creatures under UNIT_STATE_CONTROLLED are excluded from
// the even split in C++ and receive a zero-threat add instead
// (ThreatManager.cpp:676-680); Go has no CC-state model, so every recipient
// takes the full share. C++ builds its recipient set from the target's
// _threatenedByMe refs, which include zero-threat engaged creatures; Go's
// scan requires GetThreat > 0.
func (s *session) forwardInitialAssistThreatLocked(threat float32, targetGUID uint64) {
	var assisting []*creatureMotion
	for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if m == nil || m.GUID == targetGUID || m.ThreatMgr == nil {
			continue
		}
		// ForwardThreatForAssistingMe iterates the target's _threatenedByMe
		// refs, which can only exist on threat-list-capable creatures
		// (ThreatManager.cpp:156-179); owned pet/summon motions never hold one.
		if m.OwnerGUID != 0 {
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
		inMelee := inMeleeThreatRange(m.CombatReach, s.player.CombatReach, dist)
		switched, newVictim := m.ThreatMgr.AddThreat(s.playerGUID, perTarget, inMelee)
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.server.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}
}
