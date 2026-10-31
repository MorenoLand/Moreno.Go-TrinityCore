package world

import (
	"context"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
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
// registry (ThreatManager.cpp:727-731), consumed when the caster generates
// threat (ThreatManager.cpp:344-373). damage is the effect's CalculateDamage
// value. There is no alive check in C++; the registry key is the redirect
// spell's own id (e.g. 35079), unregistered when its aura is removed.
func (s *session) handleEffectRedirectThreat(spell wotlk.Spell, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.player == nil {
		return
	}
	damage := eff.CalcValueForLevel(spell, uint32(s.player.Level))
	if damage <= 0 {
		return
	}
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		s.registerRedirectThreat(spell.ID, targetGUID, uint32(damage))
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
	if s == nil || s.server == nil {
		return spellThreatEntry{}, false
	}
	return s.server.loadSpellThreatEntry(ctx, spellID)
}

// loadSpellThreatEntry is the Server-level spell_threat fetch, shared by the
// player path above and the creature/pet initial-threat leg below.
func (s *Server) loadSpellThreatEntry(ctx context.Context, spellID uint32) (spellThreatEntry, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return spellThreatEntry{}, false
	}
	var row spellThreatEntry
	err := s.WorldStore.DB.QueryRowContext(ctx,
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
// before this damage). A zero non-gated amount still runs AddThreat (the ref
// is created with 0 threat and the victim leg runs — Unit::EngageWithTarget
// seeds 0.0f the same way).
// spellThreatStep1Gate mirrors ThreatManager::AddThreat's step-1 early return
// (ThreatManager.cpp:311-317): SPELL_ATTR1_NO_THREAT, or
// SPELL_ATTR3_NO_INITIAL_AGGRO while the owner is not engaged. On this gate
// C++ returns before SetInCombatWith, so the victim neither gains threat nor
// enters combat — the damage itself still lands.
func spellThreatStep1Gate(spell wotlk.Spell, spellKnown, engaged bool) bool {
	return spellKnown && (spell.AttributesEx&spellAttr1NoThreat != 0 ||
		(spell.AttributesEx3&spellAttr3NoInitialAggro != 0 && !engaged))
}

// spellDamageStep1Gate loads the spell and reports AddThreat's step-1 gate
// for a damage funnel (ThreatManager.cpp:311-317), for call sites that need
// the gate before the victim's motion (and pre-damage engagement state) is
// available. engaged=false is the conservative pre-lock probe: it gates
// NO_INITIAL_AGGRO spells, which the in-block computation then refines with
// the true engagement state.
func (s *session) spellDamageStep1Gate(spellID uint32, engaged bool) bool {
	if s == nil || s.server == nil || spellID == 0 {
		return false
	}
	sp, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return false
	}
	return spellThreatStep1Gate(sp, true, engaged)
}

func (s *session) damageThreatAmount(ctx context.Context, spellID uint32, schoolMask uint32, damage float32, engaged bool) (float32, bool) {
	return s.damageThreatAmountWithModOwner(ctx, spellID, schoolMask, damage, engaged, s, nil)
}

// damageThreatAmountWithModOwner is damageThreatAmount with the SPELLMOD_THREAT
// fold keyed on the caster's spellmod owner instead of the executing session:
// WorldObject::GetSpellModOwner (Object.cpp:2246-2261) resolves an owner
// player only for pet/totem casters, so a possessed wild creature's threat
// takes no charmer spellmod. Player casters pass themselves, unchanged.
// attackerMotion, when non-nil, is the creature attacker: CalculateModifiedThreat
// (ThreatManager.cpp:606-659) takes the ATTACKER as its unit argument, so the
// school multipliers come from the attacker's own SPELL_AURA_MOD_THREAT auras
// (the creature-aura store), never the executing session's player auras.
func (s *session) damageThreatAmountWithModOwner(ctx context.Context, spellID uint32, schoolMask uint32, damage float32, engaged bool, modOwner *session, attackerMotion *creatureMotion) (float32, bool) {
	if s == nil || s.server == nil {
		return 0, false
	}
	var spell wotlk.Spell
	spellKnown := false
	if spellID != 0 {
		if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
			spell, spellKnown = sp, true
		}
	}
	// The step-1 gate runs on the zero AddThreat too (Unit.cpp:906 calls
	// AddThreat with float(damage) even when damage is 0): a gated spell
	// engages nothing, an ungated one creates the 0-ref and engages
	// ("threat implies combat", ThreatManager.cpp:366-368).
	if spellThreatStep1Gate(spell, spellKnown, engaged) {
		return 0, true
	}
	if damage <= 0 {
		return 0, false
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
		if spellKnown && modOwner != nil {
			t = modOwner.applySpellModFloat(spell, spellModThreat, t)
		}
	}
	if attackerMotion != nil {
		return float32(t) * s.server.creatureThreatMultiplier(attackerMotion.Map, attackerMotion.InstanceID, attackerMotion.GUID, schoolMask), false
	}
	return float32(t) * s.getThreatMultiplier(schoolMask), false
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

// spellHasNoThreatAttr mirrors the ThreatManager::ForwardThreatForAssistingMe
// step-1 shortcut (ThreatManager.cpp:664-665): a SPELL_ATTR1_NO_THREAT spell
// generates no assist threat at all. A missing spell record degrades to
// "has threat" — C++ always has the SpellInfo at these call sites.
func spellHasNoThreatAttr(data *wotlk.Store, spellID uint32) bool {
	if data == nil || spellID == 0 {
		return false
	}
	sp, found, err := data.Spell(spellID)
	if err != nil || !found {
		return false
	}
	return sp.AttributesEx&spellAttr1NoThreat != 0
}

// unitInCombatLocked reports whether the unit is in combat: the motionMu-held
// variant of creatureSpellTargetInCombat (boss_ai.go) for callers that already
// hold the lock. Caller holds motionMu; findSessionByGUID takes sessionsMu,
// which follows the same motionMu -> sessionsMu ordering the threat forwards
// below already use.
func (s *Server) unitInCombatLocked(mapID, instanceID uint32, guid uint64) bool {
	if s == nil {
		return false
	}
	if ts := s.findSessionByGUID(guid); ts != nil && ts.player != nil {
		return ts.attackTarget != 0 || ts.player.UnitFlags&unitFlagInCombat != 0
	}
	motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
	return motion != nil && motion.InCombat
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
// no caster multiplier on this path). Creature and pet casts run the twin
// leg handleCreatureSpellInitialThreat (below): the negative half is
// vacuous for creature casts on players — players cannot have threat lists
// (ThreatManager.cpp:156-179), which is exactly C++'s CanHaveThreatList
// skip — and creature heal forwarding (a creature as the
// ForwardThreatForAssistingMe assistant) rides the healing-threat
// distributor there. C++ takes the caster from m_originalCaster
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
			// Spell::HandleThreatSpells (Spell.cpp:5112-5113): the AP leg is
			// unitCaster->GetTotalAttackPowerValue(BASE_ATTACK) — the live
			// aura folds, not the base field.
			threat += entry.apPctMod * s.totalAttackPowerValue(false)
		}
		threat += entry.flatMod
		pctMod = entry.pctMod
	} else if !spellSuppressesInitialThreat(spell) {
		threat += float32(spell.SpellLevel)
	}
	positive := !isHarmfulSpell(spell)
	// The forward/negative legs below can newly engage the caster (the
	// CombatManager::SetInCombatWith symmetry); the client update must go out
	// after motionMu is released, mirroring the heal-threat path.
	wasInCombat := s.player.UnitFlags&unitFlagInCombat != 0
	pushCombatUpdate := func() {
		if !wasInCombat && s.player.UnitFlags&unitFlagInCombat != 0 {
			s.sendPlayerUpdate()
		}
	}
	if threat == 0 {
		// Spell::TargetInfo::DoTargetSpellHit (Spell.cpp:2762-2766): the
		// friendly-assist arm fires even when the computed initial threat is
		// zero — a 0.0f ForwardThreatForAssistingMe with ignoreModifiers=true
		// and a null spell on each in-combat friendly hit target. The zero
		// add still creates the threat refs and engages the target's
		// attackers with the caster (ThreatManager.cpp:379-410); negative
		// spells have no such arm (the C++ zero leg is friendly-only), and
		// HandleThreatSpells itself returns early on zero threat for both
		// signs (Spell.cpp:5119-5120). m_originalCaster is the caster for a
		// direct player cast (Spell.cpp:570-576), which is the session player
		// here. Missed targets are not covered: the arm lives in
		// DoTargetSpellHit, which only runs for hits.
		if positive {
			s.server.motionMu.Lock()
			for _, targetGUID := range hitTargets {
				if targetGUID == 0 {
					continue
				}
				if !s.server.unitInCombatLocked(s.player.Map, s.player.InstanceID, targetGUID) {
					continue
				}
				s.forwardInitialAssistThreatLocked(0, targetGUID)
			}
			s.server.motionMu.Unlock()
			pushCombatUpdate()
		}
		return
	}
	// Spell.cpp:5123: the defined bonus is distributed among all unique
	// targets, misses included (their shares are wasted, Spell.cpp:5125).
	totalTargets := len(hitTargets) + len(missGUIDs)
	// C++ divides by m_UniqueTargetInfo.size() (Spell.cpp:5123) and
	// wild gameobject spells cause no threat at all — GO guids riding a
	// mixed triggered cast's hit/miss lists must not dilute the divisor.
	// Launch lists are units only, so this is behavior-identical there.
	for _, guid := range hitTargets {
		if uint16(guid>>48) == 0xF110 {
			totalTargets--
		}
	}
	for _, guid := range missGUIDs {
		if uint16(guid>>48) == 0xF110 {
			totalTargets--
		}
	}
	// Same whole-list rule as the AoE-cap divisor (Spell.cpp:7761): a
	// TARGET_UNIT_CASTER sibling effect's TargetInfo dilutes the division
	// even though it rides neither the hit list nor the miss list (the
	// caster never misses itself and the single-target selection seeds no
	// list), so it is counted here when absent from both.
	if spellHasCasterTargetEffect(spell) {
		casterCounted := false
		for _, guid := range hitTargets {
			if guid == s.playerGUID {
				casterCounted = true
				break
			}
		}
		if !casterCounted {
			for _, guid := range missGUIDs {
				if guid == s.playerGUID {
					casterCounted = true
					break
				}
			}
		}
		if !casterCounted {
			totalTargets++
		}
	}
	if totalTargets <= 0 {
		return
	}
	threat /= float32(totalTargets)
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
	s.server.motionMu.Unlock()
	pushCombatUpdate()
}

// creatureBaseAttackPower mirrors Unit::GetTotalAttackPowerValue(BASE_ATTACK)
// (Unit.cpp:9507) for a wild creature caster: creature_classlevelstats
// attackpower parks on UNIT_MOD_ATTACK_POWER BASE_VALUE (Creature.cpp:1549),
// the aura-99 flat fold feeds TOTAL_VALUE and the aura-166 product feeds
// TOTAL_PCT, and a negative sum floors at zero. A missing row (unknown
// level/class) degrades to the aura folds alone, like the load-time fallback
// in combat.go. True pets use the guardian twin instead
// (petGuardianAttackPower, StatSystem.cpp:1386-1477); callers branch on it.
func (s *Server) creatureBaseAttackPower(ctx context.Context, caster *creatureMotion) float64 {
	if s == nil || caster == nil || caster.GUID == 0 {
		return 0
	}
	var ap int64
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		// The motion carries no unit class (loadCreatureStats reads it
		// from the template row), so resolve it here; this helper runs
		// only when a spell_threat row carries apPctMod, never per cast.
		var class uint8
		_ = s.WorldStore.DB.QueryRowContext(ctx, `SELECT unit_class FROM creature_template WHERE entry = ?`, caster.Entry).Scan(&class)
		_ = s.WorldStore.DB.QueryRowContext(ctx, `SELECT attackpower FROM creature_classlevelstats WHERE level = ? AND class = ?`, caster.Level, class).Scan(&ap)
	}
	key := creatureAuraKeyForMotion(caster)
	total := float64(ap+int64(s.creatureAuraAttackPowerFlat(key))) * s.creatureAuraAttackPowerPct(key)
	if total < 0 {
		return 0
	}
	return total
}

// handleCreatureSpellInitialThreat mirrors Spell::HandleThreatSpells
// (Spell.cpp:5096-5151) for a creature caster: the boss_ai and pet cast
// paths previously took no initial-threat pass at all, so a creature's buff,
// debuff or damage spell added only its effect threat (damage/heal legs)
// and never the cast threat C++ adds ahead of the HIT effects
// (_handle_immediate_phase, Spell.cpp:3718). The threat is the spell_threat
// row (flatMod + apPctMod * the caster's base attack power) or, without a
// row, the spell's SpellLevel, split evenly across ALL unique unit targets —
// hits and misses alike (Spell.cpp:5123); GO GUIDs are not TargetInfos and
// never count. Missed targets (missed, immune, evaded, reflected — the
// reflector keeps the entry with MissCondition=REFLECT, per the player
// path's AddUnitTarget note) take the zero leg, not a share. Positive spells
// forward the folded share (spell_threat-row pctMod, the mod owner's
// SPELLMOD_THREAT, then the caster's own school multipliers — the
// CalculateModifiedThreat order, ThreatManager.cpp:606-659, all
// assistant-constant so folded once) via the healing-threat distributor;
// negative spells add the raw share (ignoreModifiers=true) to creature
// targets with threat lists. A zero computed threat returns before any leg
// (Spell.cpp:5119-5120); the zero-threat friendly forward is the separate
// PreprocessSpellHit arm (Spell.cpp:2761-2765), already ported on both
// paths. Callers run this ahead of their HIT effect loops, with isMiss
// reporting their own verdict maps.
func (s *Server) handleCreatureSpellInitialThreat(ctx context.Context, caster *creatureMotion, spell wotlk.Spell, attackPower func() float64, modOwner *session, hitTargets []uint64, isMiss func(uint64) bool) {
	if s == nil || caster == nil || caster.GUID == 0 || isMiss == nil {
		return
	}
	if !spellHasInitialThreat(spell) {
		return
	}
	seen := make(map[uint64]struct{}, len(hitTargets))
	var targets []uint64
	for _, guid := range hitTargets {
		if guid == 0 || uint16(guid>>48) == 0xF110 {
			continue
		}
		if _, dup := seen[guid]; dup {
			continue
		}
		seen[guid] = struct{}{}
		targets = append(targets, guid)
	}
	if len(targets) == 0 {
		return
	}
	var threat float32
	var pctMod float32 = 1.0
	if entry, ok := s.loadSpellThreatEntry(ctx, spell.ID); ok {
		if entry.apPctMod != 0 && attackPower != nil {
			threat += entry.apPctMod * float32(attackPower())
		}
		threat += entry.flatMod
		pctMod = entry.pctMod
	} else if !spellSuppressesInitialThreat(spell) {
		threat += float32(spell.SpellLevel)
	}
	if threat == 0 {
		return
	}
	threat /= float32(len(targets))
	positive := !isHarmfulSpell(spell)
	assistThreat := float32(0)
	if positive {
		t := float64(threat * pctMod)
		t = modOwner.applySpellModFloat(spell, spellModThreat, t)
		assistThreat = float32(t) * s.creatureThreatMultiplier(caster.Map, caster.InstanceID, caster.GUID, uint32(spell.SchoolMask))
	}
	for _, targetGUID := range targets {
		share := threat
		if isMiss(targetGUID) {
			share = 0
		}
		if positive {
			forwardShare := assistThreat
			if isMiss(targetGUID) {
				forwardShare = 0
			}
			s.forwardCreatureInitialAssistThreat(caster, targetGUID, forwardShare)
			continue
		}
		s.addCreatureInitialNegativeThreat(caster, targetGUID, share)
	}
}

// forwardCreatureInitialAssistThreat runs the Spell.cpp:5139 positive leg
// for one target: ForwardThreatForAssistingMe with the pre-folded share,
// via the healing-threat distributor (same recipient set and CC split as
// the 2761 forward). C++ runs AddThreat per recipient, engaging the
// assistant with each; the recipient pre-scan (the distributor's set plus
// the threat-list gate, matching the player twin's recipient shape) decides
// whether the caster engages — a recipient-less forward engages nothing.
func (s *Server) forwardCreatureInitialAssistThreat(caster *creatureMotion, targetGUID uint64, amount float32) {
	if s == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 {
		return
	}
	hasRecipient := false
	s.motionMu.Lock()
	for _, m := range s.motionMapLocked(caster.Map, caster.InstanceID) {
		if m == nil || m.GUID == targetGUID || m.Health == 0 || !m.InCombat || m.Evading || !motionCanHaveThreatList(m) {
			continue
		}
		if m.TargetGUID == targetGUID || (m.ThreatMgr != nil && m.ThreatMgr.HasVictim(targetGUID)) {
			hasRecipient = true
			break
		}
	}
	s.motionMu.Unlock()
	s.distributeHealingThreatLocked(nil, caster.Map, caster.InstanceID, caster.X, caster.Y, caster.Z, caster.CombatReach, caster.GUID, targetGUID, amount)
	if hasRecipient {
		s.motionMu.Lock()
		if cm := s.findCreatureMotionLocked(caster.Map, caster.InstanceID, caster.GUID); cm != nil {
			cm.InCombat = true
		}
		s.motionMu.Unlock()
	}
}

// addCreatureInitialNegativeThreat runs the Spell.cpp:5143 negative leg on
// one creature target for a creature caster, in the creatureEffectThreat
// shape (lock per target, AddThreat crediting the caster, switch broadcast
// plus AI-reaction/attack-start packets on a newly engaged victim). The
// CanHaveThreatList check (ThreatManager.cpp:156-179) runs before AddThreat,
// exactly as C++ does: player targets resolve to no motion here and skip,
// and list-less creature targets (pets, player-summoned minions) skip with
// no engage — HandleThreatSpells never reaches AddThreat's engage-only
// early leg for them. (The player twin's engage-only fallback for list-less
// targets diverges from this reading; flagged, not changed this tick.)
// ignoreModifiers=true, so no pctMod, spellmod or school fold; creature
// casters carry no redirect registry (the redirect effect is player-cast
// only in Go), so the nil-session split is a no-op and ignoreRedirects=false
// is vacuous here.
func (s *Server) addCreatureInitialNegativeThreat(caster *creatureMotion, targetGUID uint64, amount float32) {
	if s == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 {
		return
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(caster.Map, caster.InstanceID, targetGUID)
	if motion == nil || !motionCanHaveThreatList(motion) {
		s.motionMu.Unlock()
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion)
	}
	var nilSess *session
	share, rSwitched, rVictim := nilSess.splitThreatRedirects(motion, amount)
	dist := distance3D(caster.X, caster.Y, caster.Z, motion.X, motion.Y, motion.Z)
	inMelee := inMeleeThreatRange(motion.CombatReach, caster.CombatReach, dist)
	switched, newVictim := motion.ThreatMgr.AddThreat(caster.GUID, share, inMelee)
	if rSwitched {
		switched, newVictim = true, rVictim
	}
	wasInCombat := motion.InCombat
	if switched && newVictim != motion.TargetGUID {
		motion.TargetGUID = newVictim
	} else {
		motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
	}
	motion.InCombat = true
	motion.Moving = true
	if cm := s.findCreatureMotionLocked(caster.Map, caster.InstanceID, caster.GUID); cm != nil {
		cm.InCombat = true
	}
	entries := motion.ThreatMgr.SortedEntries()
	mapID, instanceID := motion.Map, motion.InstanceID
	mx, my, mz, mguid := motion.X, motion.Y, motion.Z, motion.GUID
	s.motionMu.Unlock()
	if switched {
		s.broadcastHighestThreatUpdateInInstance(mapID, instanceID, mx, my, mz, mguid, newVictim, entries)
	}
	if !wasInCombat {
		s.broadcastAIReactionInInstance(mapID, instanceID, mguid, 2)
		startPkt := buildAttackStart(mguid, caster.GUID)
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, mapID, instanceID, mx, my, mz, nil)
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
	// Spell.cpp:5143-5144: the negative path skips the threat add for
	// targets that cannot have a threat list (ThreatManager::CanHaveThreatList,
	// ThreatManager.cpp:156-179 — charmed creatures keep their lists), but
	// C++ AddThreat's !CanHaveThreatList() early leg (ThreatManager.cpp:328-339)
	// still runs SetInCombatWith on both sides, so the target and the caster
	// engage; the caller pushes the client update after motionMu is released.
	if !motionCanHaveThreatList(motion) {
		motion.InCombat = true
		if s.player.UnitFlags&unitFlagInCombat == 0 {
			s.player.UnitFlags |= unitFlagInCombat
		}
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
		s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.GUID, newVictim, entries)
	} else {
		motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
	}
	motion.InCombat = true
	motion.Moving = true
	// CombatManager::SetInCombatWith symmetry (CombatManager.cpp:200-257):
	// the negative-leg AddThreat (Spell.cpp:5143) engages the caster with the
	// target as well as the target with the caster. Go has no per-pair
	// CombatManager model, so the caster side is the in-combat flag; the
	// caller pushes the client update after motionMu is released.
	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
	}
}

// addDrainGainThreat runs the HandlePeriodicManaLeechAuraTick gain arm
// (SpellAuraEffects.cpp:5489-5491): target->GetThreatManager().AddThreat(
// caster, gainedAmount * 0.5f, GetSpellInfo(), true) — a direct add on the
// drain victim's own threat list crediting the drainer, not the heal-style
// assist distribution: draining a creature makes it threaten the drainer.
// ignoreModifiers=true skips CalculateModifiedThreat; ignoreRedirects stays
// false so the caster's redirect registry consumes (splitThreatRedirects,
// ThreatManager.cpp:347-373). Step 1 (ThreatManager.cpp:308-317): NO_THREAT
// spells add nothing, and NO_INITIAL_AGGRO adds nothing while the victim is
// not engaged (motion.InCombat is the engagement proxy). The vehicle leg has
// no Go analog (no vehicle model). Victims that cannot have a threat list
// (players included) take combat state only (ThreatManager.cpp:328-339).
func (s *session) addDrainGainThreat(targetGUID uint64, auraSpellID uint32, tickSpell wotlk.Spell, tickKnown bool, gained uint32) {
	if s == nil || s.server == nil || s.player == nil || targetGUID == 0 || gained == 0 {
		return
	}
	// ThreatManager::AddThreat step 1 (ThreatManager.cpp:308-317). tickSpell
	// carries the DBC record when known; otherwise fall back to a store
	// lookup on the aura's spell id.
	noThreat, noInitialAggro := false, false
	if tickKnown {
		noThreat = tickSpell.AttributesEx&spellAttr1NoThreat != 0
		noInitialAggro = tickSpell.AttributesEx3&spellAttr3NoInitialAggro != 0
	} else if s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(auraSpellID); err == nil && found {
			noThreat = sp.AttributesEx&spellAttr1NoThreat != 0
			noInitialAggro = sp.AttributesEx3&spellAttr3NoInitialAggro != 0
		}
	}
	if noThreat {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil {
		s.server.motionMu.Unlock()
		return
	}
	if noInitialAggro && !motion.InCombat {
		s.server.motionMu.Unlock()
		return
	}
	// ThreatManager::AddThreat's SetInCombatWith leg (ThreatManager.cpp:379):
	// the add engages both sides. The !CanHaveThreatList() early leg
	// (ThreatManager.cpp:328-339) takes combat state only, no threat refs.
	if !motionCanHaveThreatList(motion) {
		motion.InCombat = true
		if s.player.UnitFlags&unitFlagInCombat == 0 {
			s.player.UnitFlags |= unitFlagInCombat
		}
		s.server.motionMu.Unlock()
		s.sendPlayerUpdate()
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion)
	}
	threat := float32(gained) * 0.5
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
	inMelee := inMeleeThreatRange(motion.CombatReach, s.player.CombatReach, dist)
	threat, rSwitched, rVictim := s.splitThreatRedirects(motion, threat)
	switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
	if rSwitched {
		switched, newVictim = true, rVictim
	}
	if switched && newVictim != motion.TargetGUID {
		motion.TargetGUID = newVictim
		entries := motion.ThreatMgr.SortedEntries()
		s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.GUID, newVictim, entries)
	} else {
		motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
	}
	motion.InCombat = true
	motion.Moving = true
	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
	}
	s.server.motionMu.Unlock()
	s.sendPlayerUpdate()
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
// Recipients under UNIT_STATE_CONTROLLED (ThreatManager.cpp:676-680) take a
// zero add and are excluded from the even-split denominator; Go reads the CC
// state from the creature's live confuse/stun/fear auras plus the fleeing
// timer, the same model as distributeHealingThreatLocked.
func (s *session) forwardInitialAssistThreatLocked(threat float32, targetGUID uint64) {
	var canBe, cannotBe []*creatureMotion
	now := time.Now()
	for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if m == nil || m.GUID == targetGUID || m.ThreatMgr == nil {
			continue
		}
		// ForwardThreatForAssistingMe iterates the target's _threatenedByMe
		// refs, which can only exist on threat-list-capable creatures
		// (ThreatManager.cpp:156-179); owned pet/summon motions never hold
		// one, but charmed creatures keep their lists and are recipients.
		if !motionCanHaveThreatList(m) {
			continue
		}
		// ThreatManager.cpp:662-688 iterates ALL _threatenedByMe pairs with no
		// threat-value filter: zero-threat refs are recipients too (AddThreat
		// creates the ref even at 0, ThreatManager.cpp:308-410; negatives
		// floor at 0 keeping the entry) — HasVictim tests entry existence,
		// not a positive value.
		if !m.ThreatMgr.HasVictim(targetGUID) {
			continue
		}
		// UNIT_STATE_CONTROLLED (Unit.h:251 — CONFUSED | STUNNED | FLEEING;
		// fear lands here via the fleeing leg) recipients take the zero add.
		key := creatureAuraKey{Map: m.Map, InstanceID: m.InstanceID, GUID: m.GUID}
		if (!m.FleeingUntil.IsZero() && now.Before(m.FleeingUntil)) ||
			s.server.creatureHasAuraType(key, spellAuraModConfuse) ||
			s.server.creatureHasAuraType(key, spellAuraModStun) ||
			s.server.creatureHasAuraType(key, spellAuraModFear) {
			cannotBe = append(cannotBe, m)
			continue
		}
		canBe = append(canBe, m)
	}
	if len(canBe) == 0 && len(cannotBe) == 0 {
		return
	}
	// CombatManager::SetInCombatWith (CombatManager.cpp:200-257) inserts into
	// both managers and updates both units' combat states: every per-recipient
	// AddThreat below engages the assistant (the session player) as well as the
	// recipient creature. Go has no per-pair CombatManager model, so the caster
	// side is the in-combat flag; the caller pushes the client update after
	// motionMu is released.
	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
	}
	addShare := func(m *creatureMotion, amount float32) {
		dist := distance3D(s.player.X, s.player.Y, s.player.Z, m.X, m.Y, m.Z)
		inMelee := inMeleeThreatRange(m.CombatReach, s.player.CombatReach, dist)
		// The per-recipient AddThreat runs the redirect leg against the
		// assistant's registry (ThreatManager.cpp:344-373, ignoreRedirects
		// stays false on the ForwardThreatForAssistingMe adds).
		share, rSwitched, rVictim := s.splitThreatRedirects(m, amount)
		switched, newVictim := m.ThreatMgr.AddThreat(s.playerGUID, share, inMelee)
		if rSwitched {
			switched, newVictim = true, rVictim
		}
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.server.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.X, m.Y, m.Z, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}
	// Targets under CC cannot gain assist threat — split evenly among the
	// rest (ThreatManager.cpp:676-684); the zero add registers the threat
	// ref and engages combat without adding threat.
	if len(canBe) > 0 {
		perTarget := threat / float32(len(canBe))
		for _, m := range canBe {
			addShare(m, perTarget)
		}
	}
	for _, m := range cannotBe {
		addShare(m, 0)
	}
}
