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
// Reference: TrinityCore ThreatManager::AddThreat / CompareThreatLessThan.
// In melee range (< 5yd), a new target requires 110% of current victim's threat.
// At range (>= 5yd), a new target requires 130% of current victim's threat.
func (tm *ThreatManager) AddThreat(victim uint64, amount float32, inMelee bool) (switched bool, newVictim uint64) {
	if victim == 0 || amount <= 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.entries[victim] += amount
	newThreat := tm.entries[victim]

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
func (tm *ThreatManager) MatchUnitThreatToHighestThreat(victim uint64) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var highestThreat float32
	for _, threat := range tm.entries {
		if threat > highestThreat {
			highestThreat = threat
		}
	}
	current := tm.entries[victim]
	if highestThreat > current {
		tm.entries[victim] = highestThreat
	} else if highestThreat == 0 {
		tm.entries[victim] = 100.0
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
		inMelee := dist <= meleeAttackRange
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
