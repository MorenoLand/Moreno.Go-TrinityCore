package world

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// ThreatManager manages a creature's threat table and victim selection heuristics.
// Reference: TrinityCore ThreatManager.h / ThreatManager.cpp.
type ThreatManager struct {
	mu             sync.Mutex
	ownerGUID      uint64
	currentVictim  uint64
	entries        map[uint64]float32
	lastClientSync time.Time
}

// NewThreatManager initializes a ThreatManager for a creature.
func NewThreatManager(ownerGUID uint64) *ThreatManager {
	return &ThreatManager{
		ownerGUID: ownerGUID,
		entries:   make(map[uint64]float32),
	}
}

// AddThreat adds threat for a victim and evaluates target switching.
// Reference: TrinityCore ThreatManager::AddThreat (ThreatManager.cpp:308-410) /
// ReselectVictim (531-585) / CompareReferencesLT (593-601) / UpdateVictim (516-528).
// The 110%/130% switch gate matches C++ exactly, including the strict-greater
// boundary (CompareReferencesLT uses `<`, so equal threat never switches):
// a candidate breaks the current victim at >110% only in melee range,
// otherwise it needs >130%.
// Documented deltas: (1) C++ AddThreat never switches - selection happens in
// UpdateVictim->ReselectVictim on the AI tick (or when there is no current
// victim); Go re-evaluates eagerly inside AddThreat, outcome-equivalent for
// the adding victim but never re-evaluating non-adding candidates. (2) C++
// walks the sorted heap, so a ranged top below 130% does not block a melee
// candidate above 110% beneath it; Go tests only the adding victim against a
// single threshold, so that steal is missed.
// Bridged: ThreatReference::AddThreat negative amounts reduce the entry
// floored at 0 (ThreatManager.cpp:40-41); a zero add on an existing entry is a
// no-op, but on a NEW victim it still creates the entry and runs the victim
// leg (ThreatManager.cpp:308-410) — the EngageWithTarget 0.0f seed
// (Unit.cpp:8429-8438) registers the acquirer with zero threat. A decrease
// never clears the 110%/130% switch gate for the adding victim. Tick-based
// re-selection of a decreased non-current-victim entry stays under delta (1).
// The melee gate is per add, combat-reach-based via inMeleeThreatRange
// (Unit::IsWithinMeleeRange -> GetMeleeRange, Unit.cpp:599-618), matching the
// C++ per-candidate test positionally.
// Documented no-bridge: FixateTarget/_fixateRef (always preferred in
// ReselectVictim); taunt-state precedence in the comparator (TAUNT > NONE >
// DETAUNT) with TauntUpdate driven by SPELL_AURA_MOD_TAUNT (Go taunt is the
// one-shot MatchUnitThreatToHighestThreat in handleEffectTaunt); the
// online/suppressed/offline ref states (ShouldBeOffline/ShouldBeSuppressed:
// CanSeeOrDetect, _IsTargetAcceptable, CanCreatureAttack, immune flags,
// melee-school immunity, confuse, breakable stun) - Go refs are removed only
// via RemoveThreat/ClearThreat; ProcessAIUpdates/JustStartedThreateningMe
// (boss hooks ride the new-victim broadcast instead).
// Out of scope for this unit: AddThreat's modifier/redirection arms
// (CalculateModifiedThreat, NO_THREAT/NO_INITIAL_AGGRO attrs, vehicle and
// misdirection redirects) belong to the HandleThreatSpells cast-threat audit;
// getThreatMultiplier already covers stance/aura SPELL_AURA_MOD_THREAT.
func (tm *ThreatManager) AddThreat(victim uint64, amount float32, inMelee bool) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// ThreatManager::AddThreat (ThreatManager.cpp:308-410): the new-target leg
	// creates the ThreatReference and applies the amount through
	// ThreatReference::AddThreat even when the amount is 0 — Unit::EngageWithTarget
	// seeds 0.0f (Unit.cpp:8429-8438), and the zero add still registers the entry
	// and runs the victim leg. ThreatReference::AddThreat (ThreatManager.cpp:40-41)
	// no-ops on 0 and floors negatives at 0.
	oldThreat, exists := tm.entries[victim]
	var newThreat float32
	if !exists {
		if amount < 0 {
			amount = 0
		}
		newThreat = amount
		tm.entries[victim] = newThreat
		// With no current victim the new ref becomes the victim outright
		// (C++ UpdateVictim); otherwise C++ runs ProcessAIUpdates (no
		// re-selection) — Go keeps its documented eager gate as delta (1).
		if tm.currentVictim == 0 {
			tm.currentVictim = victim
			return false, victim
		}
	} else {
		// ThreatReference::AddThreat: zero is a no-op on an existing entry;
		// negatives reduce, floored at zero. A decrease never clears the
		// 110%/130% switch gate for the adding victim.
		if amount == 0 {
			return false, tm.currentVictim
		}
		newThreat = oldThreat + amount
		if newThreat < 0 {
			newThreat = 0
		}
		tm.entries[victim] = newThreat
	}

	if tm.currentVictim == 0 || tm.currentVictim == victim {
		tm.currentVictim = victim
		return false, victim
	}

	currThreat := tm.entries[tm.currentVictim]
	threshold := currThreat * 1.30
	if inMelee {
		threshold = currThreat * 1.10
	}

	if newThreat > threshold {
		tm.currentVictim = victim
		return true, victim
	}
	return false, tm.currentVictim
}

// SetThreat sets absolute threat for a victim (e.g. taunt).
func (tm *ThreatManager) SetThreat(victim uint64, amount float32) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.entries[victim] = amount
	if tm.currentVictim == 0 || amount > tm.entries[tm.currentVictim] {
		tm.currentVictim = victim
		return true, victim
	}
	return false, tm.currentVictim
}

// MatchUnitThreatToHighestThreat sets the victim's threat equal to the highest threat currently on the creature.
// Reference: TrinityCore ThreatManager::MatchUnitThreatToHighestThreat (ThreatManager.cpp:419-437).
// Documented deltas: C++ skips a highest that is itself taunting or unavailable;
// Go has no taunt-state model, so the raw highest wins. C++ returns early on an
// empty list; Go matches (no seed, no victim change). C++ routes the delta
// through AddThreat with ignoreModifiers/ignoreRedirects; Go writes the entry
// directly, which is value-equivalent for the missing-or-lower victim. Go always
// reports switched; C++ re-selects through the taunt-state comparator.
// The taunt-state machinery (TauntUpdate, comparator precedence) is
// documented at AddThreat; Go has no taunt-state model.
func (tm *ThreatManager) MatchUnitThreatToHighestThreat(victim uint64) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// ThreatManager::MatchUnitThreatToHighestThreat (ThreatManager.cpp:419-437)
	// returns early on an empty list — no seed, no victim change.
	if len(tm.entries) == 0 {
		return false, tm.currentVictim
	}

	var highestThreat float32
	for _, threat := range tm.entries {
		if threat > highestThreat {
			highestThreat = threat
		}
	}
	current := tm.entries[victim]
	if highestThreat > current {
		tm.entries[victim] = highestThreat
	}
	tm.currentVictim = victim
	return true, victim
}

// RemoveThreat removes a victim from the threat table and re-evaluates top victim.
// Reference: TrinityCore ThreatManager::PurgeThreatListRef.
func (tm *ThreatManager) RemoveThreat(victim uint64) (switched bool, newVictim uint64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.entries, victim)
	if tm.currentVictim == victim {
		tm.currentVictim = 0
		var highestGUID uint64
		var highestThreat float32
		for guid, threat := range tm.entries {
			if threat > highestThreat {
				highestThreat = threat
				highestGUID = guid
			}
		}
		tm.currentVictim = highestGUID
		return true, tm.currentVictim
	}
	return false, tm.currentVictim
}

// HasVictim reports whether victim holds an entry in the threat table.
// Used by the RemoveMeFromThreatLists bridge (ThreatManager.cpp:690-697):
// removal tests entry existence, not a positive value.
func (tm *ThreatManager) HasVictim(victim uint64) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	_, ok := tm.entries[victim]
	return ok
}

// ClearThreat wipes the threat table on evade or death.
// Reference: TrinityCore ThreatManager::ClearAllThreat.
func (tm *ThreatManager) ClearThreat() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.entries = make(map[uint64]float32)
	tm.currentVictim = 0
}

// GetCurrentVictim returns the current primary threat target.
func (tm *ThreatManager) GetCurrentVictim() uint64 {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.currentVictim
}

// GetThreat returns the threat value for a victim.
func (tm *ThreatManager) GetThreat(victim uint64) float32 {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.entries[victim]
}

// IsEmpty returns true if there are no targets threatening the creature.
func (tm *ThreatManager) IsEmpty() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return len(tm.entries) == 0
}

// SortedEntries returns protocol-ready ThreatEntry slice sorted descending by threat.
// Reference: TrinityCore ThreatManager::SendThreatListToClients.
func (tm *ThreatManager) SortedEntries() []protocol.ThreatEntry {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	list := make([]protocol.ThreatEntry, 0, len(tm.entries))
	for guid, threat := range tm.entries {
		list = append(list, protocol.ThreatEntry{
			VictimGUID: guid,
			Threat:     uint32(threat * 100),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Threat > list[j].Threat
	})
	return list
}

// Broadcast helpers for Threat Packets
func (s *Server) broadcastThreatUpdate(mapID uint32, creatureGUID uint64, list []protocol.ThreatEntry) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatUpdate(creatureGUID, list)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastHighestThreatUpdate(mapID uint32, creatureGUID, highestGUID uint64, list []protocol.ThreatEntry) {
	if s == nil {
		return
	}
	payload := protocol.BuildHighestThreatUpdate(creatureGUID, highestGUID, list)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_HIGHEST_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastHighestThreatUpdateInInstance(mapID, instanceID uint32, creatureGUID, highestGUID uint64, list []protocol.ThreatEntry) {
	payload := protocol.BuildHighestThreatUpdate(creatureGUID, highestGUID, list)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_HIGHEST_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastThreatRemove(mapID uint32, creatureGUID, victimGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatRemove(creatureGUID, victimGUID)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_REMOVE), payload, nil)
}

func (s *Server) broadcastThreatRemoveInInstance(mapID, instanceID uint32, creatureGUID, victimGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatRemove(creatureGUID, victimGUID)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_THREAT_REMOVE), payload, nil)
}

// removeThreatVictimFromAllLists drops victimGUID from every other unit's
// threat table on the map/instance.
// Reference: TrinityCore ThreatManager::RemoveMeFromThreatLists
// (ThreatManager.cpp:690-697), reached on evade via CombatStop(true) ->
// CombatManager::EndAllPvECombat (CombatManager.cpp:344-350) and on death
// via Unit::setDeathState -> CombatStop (Unit.cpp:8901-8907) — the
// setDeathState pairing of RemoveMeFromThreatLists (the dead unit as
// victim in everyone else's table) with ClearAllThreat (the dead unit's
// own table). Per-victim removal sends SMSG_THREAT_REMOVE
// (SendRemoveToClients, ThreatManager.cpp:477) and re-evaluates the top
// victim (RemoveThreat / UpdateVictim).
func (s *Server) removeThreatVictimFromAllLists(mapID, instanceID uint32, victimGUID uint64) {
	if s == nil || victimGUID == 0 {
		return
	}
	var owners []uint64
	s.motionMu.Lock()
	for guid, motion := range s.motionMapLocked(mapID, instanceID) {
		if guid == victimGUID || motion == nil || motion.ThreatMgr == nil {
			continue
		}
		if motion.ThreatMgr.HasVictim(victimGUID) {
			owners = append(owners, guid)
		}
	}
	s.motionMu.Unlock()
	for _, ownerGUID := range owners {
		motion := s.findCreatureMotion(mapID, instanceID, ownerGUID)
		if motion == nil || motion.ThreatMgr == nil {
			continue
		}
		motion.ThreatMgr.RemoveThreat(victimGUID)
		s.broadcastThreatRemoveInInstance(mapID, instanceID, ownerGUID, victimGUID)
	}
}

// stopPlayerCombat mirrors the combat half of Unit::CombatStop (Unit.cpp:5809-5825)
// reached on logout via Unit::CleanupBeforeRemoveFromMap (Unit.cpp:9845) and on
// teleport via Player::TeleportTo (Player.cpp:1753 same-map unless
// TELE_TO_NOT_LEAVE_COMBAT, 1803 far-teleport unconditional): drop the player as a
// victim from every creature/pet threat table on the map/instance
// (ThreatManager::RemoveMeFromThreatLists, ThreatManager.cpp:690-697, via
// CombatManager::EndAllPvECombat, CombatManager.cpp:344-350) and stop the player's
// own attack (AttackStop + SMSG_ATTACK_STOP). The ClearAllThreat leg is vacuous —
// Go players carry no ThreatMgr — and RemoveAllAttackers needs no bridge
// (attackers hold no separate attacker list in Go); the PvP suppression leg has
// no Go model outside sanctuary handling. No Go teleportTo caller maps to a
// TELE_TO_NOT_LEAVE_COMBAT (Player.h:666) site — blink/GO/transport/boss-mechanic
// teleports have no Go analog routing through teleportTo — so the stop is
// unconditional here, matching C++ for every Go caller (incl. Hearthstone, whose
// TELE_TO_SPELL still drops combat at Player.cpp:1753).
func (s *session) stopPlayerCombat() {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	s.server.removeThreatVictimFromAllLists(s.player.Map, s.player.InstanceID, s.playerGUID)
	if s.attackTarget != 0 {
		victim := s.attackTarget
		s.attackTarget = 0
		_ = s.sendAttackStop(victim, false)
	}
}

func (s *Server) broadcastThreatClear(mapID uint32, creatureGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatClear(creatureGUID)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_CLEAR), payload, nil)
}

func (s *Server) broadcastThreatClearInInstance(mapID, instanceID uint32, creatureGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatClear(creatureGUID)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_THREAT_CLEAR), payload, nil)
}

// getThreatMultiplier calculates the session's current threat multiplier based on active stances and auras.
// Reference: TrinityCore Unit::GetTotalAuraMultiplierByMiscMask(SPELL_AURA_MOD_THREAT, mask).
func (s *session) getThreatMultiplier(schoolMask uint32) float32 {
	if s == nil || s.player == nil {
		return 1.0
	}
	mult := float32(1.0)
	s.castMu.Lock()
	defer s.castMu.Unlock()

	for _, a := range s.activeAuras {
		if a == nil || a.Stopped {
			continue
		}
		// Generic SPELL_AURA_MOD_THREAT (AuraType 10)
		if a.AuraType == 10 {
			mult *= (1.0 + float32(int32(a.Amount))/100.0)
			continue
		}
		// Specific stance / aura threat modifiers
		switch a.SpellID {
		case 71: // Warrior: Defensive Stance (+45% threat)
			mult *= 1.45
		case 2457, 2458: // Warrior: Battle / Berserker Stance (-20% threat)
			mult *= 0.80
		case 5487, 9634: // Druid: Bear Form / Dire Bear Form (+30% threat)
			mult *= 1.30
		case 25780: // Paladin: Righteous Fury (+80% threat on Holy spells, schoolMask & 0x02 != 0)
			if schoolMask&0x02 != 0 {
				mult *= 1.80
			}
		case 1038: // Paladin: Hand of Salvation (-20% threat)
			mult *= 0.80
		}
	}
	if mult < 0.1 {
		mult = 0.1
	}
	return mult
}

// isTauntSpell returns true if the spell is a Taunt ability.
// Reference: TrinityCore SPELL_EFFECT_ATTACK_ME (114) and SPELL_AURA_MOD_TAUNT (11).
func isTauntSpell(spellID uint32) bool {
	switch spellID {
	case 355, // Warrior: Taunt
		694,   // Warrior: Mocking Blow
		1161,  // Warrior: Challenging Shout
		6795,  // Druid: Growl
		5209,  // Druid: Challenging Roar
		56222, // Death Knight: Dark Command
		62124, // Paladin: Hand of Reckoning
		31789: // Paladin: Righteous Defense
		return true
	}
	return false
}

// handleEffectTaunt processes SPELL_EFFECT_ATTACK_ME (114) and Taunt spells against creature targets.
// Reference: TrinityCore Spell::EffectTaunt (SpellEffects.cpp:3131-3169).
func (s *session) handleEffectTaunt(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.server == nil || targetGUID == 0 {
		return
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()

	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Evading {
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(targetGUID)
	}
	switched, newVictim := motion.ThreatMgr.MatchUnitThreatToHighestThreat(s.playerGUID)
	if switched || newVictim != motion.TargetGUID {
		motion.TargetGUID = newVictim
		entries := motion.ThreatMgr.SortedEntries()
		s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
	}
	motion.InCombat = true
	motion.Moving = true
}

// distributeHealingThreat calculates and splits healing threat among all creatures
// currently in combat with the healer or the heal target.
// Formula mirrors TrinityCore Unit::SendHealSpellLog and Unit::DoAttack (Unit.cpp:6550-6580):
// Base threat = effectiveHeal * 0.5 * healerThreatMultiplier.
// Threat is divided equally by the number of engaged creatures.
func (s *Server) distributeHealingThreat(ctx context.Context, healerGUID, targetGUID uint64, effectiveHeal uint32) {
	if s == nil || healerGUID == 0 || effectiveHeal == 0 {
		return
	}

	healerSess := s.findSessionByGUID(healerGUID)
	if healerSess == nil || healerSess.player == nil {
		return
	}

	mult := healerSess.getThreatMultiplier(2) // Holy/Healing school mask = 2
	totalThreat := float32(effectiveHeal) * 0.5 * mult
	if totalThreat <= 0 {
		return
	}

	s.motionMu.Lock()
	defer s.motionMu.Unlock()

	var engaged []*creatureMotion
	for _, m := range s.motionMapLocked(healerSess.player.Map, healerSess.player.InstanceID) {
		if m == nil || m.Health == 0 || !m.InCombat || m.Evading {
			continue
		}
		isEngaged := false
		if m.TargetGUID == healerGUID || m.TargetGUID == targetGUID {
			isEngaged = true
		} else if m.ThreatMgr != nil {
			if m.ThreatMgr.GetThreat(healerGUID) > 0 || (targetGUID != 0 && m.ThreatMgr.GetThreat(targetGUID) > 0) {
				isEngaged = true
			}
		}
		if isEngaged {
			engaged = append(engaged, m)
		}
	}

	if len(engaged) == 0 {
		return
	}

	threatPerCreature := totalThreat / float32(len(engaged))
	for _, m := range engaged {
		if m.ThreatMgr == nil {
			m.ThreatMgr = NewThreatManager(m.GUID)
		}
		dist := distance3D(healerSess.player.X, healerSess.player.Y, healerSess.player.Z, m.X, m.Y, m.Z)
		inMelee := inMeleeThreatRange(m.CombatReach, healerSess.player.CombatReach, dist)
		switched, newVictim := m.ThreatMgr.AddThreat(healerGUID, threatPerCreature, inMelee)
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}

	if healerSess.player.UnitFlags&unitFlagInCombat == 0 {
		healerSess.player.UnitFlags |= unitFlagInCombat
		healerSess.sendPlayerUpdate()
	}
}
