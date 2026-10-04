package world

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Arathi Basin (AB) Constants mirroring TrinityCore BattlegroundAB.h / BattlegroundAB.cpp.
const (
	ABMapID uint32 = 529

	ABNodeStables    uint32 = 0
	ABNodeBlacksmith uint32 = 1
	ABNodeFarm       uint32 = 2
	ABNodeLumberMill uint32 = 3
	ABNodeGoldMine   uint32 = 4
	ABNodeMax        uint32 = 5

	ABNodeStateNeutral            uint32 = 0
	ABNodeStateContestedAlliance  uint32 = 1
	ABNodeStateContestedHorde     uint32 = 2
	ABNodeStateControlledAlliance uint32 = 3
	ABNodeStateControlledHorde    uint32 = 4

	ABBannerCaptureTimeDefault = 60 * time.Second

	ABMaxResources uint32 = 1600

	// World States
	ABWorldStateAllianceResources uint32 = 1776
	ABWorldStateHordeResources    uint32 = 1777
	ABWorldStateMaxResources      uint32 = 1780
	ABWorldStateBasesAlliance     uint32 = 1779
	ABWorldStateBasesHorde        uint32 = 1778

	// Per-tick award thresholds from BattlegroundAB::Startup (BattlegroundAB.cpp:606-607).
	// Go has no BG-weekend model, so the non-weekend values are used, matching the
	// convention of the other per-BG reward bridges.
	abHonorTicsThreshold uint32 = 260  // BG_AB_NotABBGWeekendHonorTicks
	abRepTicsThreshold   uint32 = 160  // BG_AB_NotABBGWeekendReputationTicks
	abNearVictoryScore   uint32 = 1400 // BG_AB_WARNING_NEAR_VICTORY_SCORE
)

var abNodeNames = [ABNodeMax]string{
	"the Stables",
	"the Blacksmith",
	"the Farm",
	"the Lumber Mill",
	"the Gold Mine",
}

// Banner GameObject entries by NodeID (0..4) and State (0..4)
// Neutral: 180087..180091
// Contested Alliance: 180100..180104
// Contested Horde: 180105..180109
// Controlled Alliance: 180110..180114
// Controlled Horde: 180115..180119
func getABBannerEntry(nodeID uint32, state uint32) uint32 {
	if nodeID >= ABNodeMax {
		return 0
	}
	switch state {
	case ABNodeStateNeutral:
		return 180087 + nodeID
	case ABNodeStateContestedAlliance:
		return 180100 + nodeID
	case ABNodeStateContestedHorde:
		return 180105 + nodeID
	case ABNodeStateControlledAlliance:
		return 180110 + nodeID
	case ABNodeStateControlledHorde:
		return 180115 + nodeID
	default:
		return 0
	}
}

// WorldState Icon mapping for each node in each state.
// Mirrors TrinityCore BattlegroundAB.h BG_AB_WorldStates: per node the state ids are
// BG_AB_OP_NODESTATES[node]+{0,2,3,0,1} for {ally-occupied, ally-contested,
// horde-contested} via the plusArray in _SendNodeUpdate, and BG_AB_OP_NODEICONS[node]
// while neutral.
var abNodeWorldStates = [ABNodeMax][5]uint32{
	// Stables: Neutral=1842(icon), ContAlly=1769, ContHorde=1770, Ally=1767, Horde=1768
	{1842, 1769, 1770, 1767, 1768},
	// Blacksmith: Neutral=1846(icon), ContAlly=1784, ContHorde=1785, Ally=1782, Horde=1783
	{1846, 1784, 1785, 1782, 1783},
	// Farm: Neutral=1845(icon), ContAlly=1774, ContHorde=1775, Ally=1772, Horde=1773
	{1845, 1774, 1775, 1772, 1773},
	// Lumber Mill: Neutral=1844(icon), ContAlly=1794, ContHorde=1795, Ally=1792, Horde=1793
	{1844, 1794, 1795, 1792, 1793},
	// Gold Mine: Neutral=1843(icon), ContAlly=1789, ContHorde=1790, Ally=1787, Horde=1788
	{1843, 1789, 1790, 1787, 1788},
}

// Resource accumulation intervals and tick points from TrinityCore BattlegroundAB.h:
// 1 base: 10 pts every 12 sec
// 2 bases: 10 pts every 9 sec
// 3 bases: 10 pts every 6 sec
// 4 bases: 10 pts every 3 sec
// 5 bases: 30 pts every 1 sec
var abTickIntervals = [6]time.Duration{
	0,
	12 * time.Second,
	9 * time.Second,
	6 * time.Second,
	3 * time.Second,
	1 * time.Second,
}

var abTickPoints = [6]uint32{0, 10, 10, 10, 10, 30}

type abNodeState struct {
	NodeID       uint32
	State        uint32
	PrevState    uint32 // Prior controlled state for defending return
	AssaultedBy  uint64 // player GUID credited with the objective capture
	CaptureTimer *time.Timer
	BannerGUID   uint64
	BannerEntry  uint32
	X            float32
	Y            float32
	Z            float32
}

type abBattlegroundState struct {
	mu                   sync.Mutex
	MapID                uint32
	AllianceResources    uint32
	HordeResources       uint32
	MaxResources         uint32
	AllianceBasesCount   uint32
	HordeBasesCount      uint32
	Nodes                [ABNodeMax]abNodeState
	CaptureDuration      time.Duration
	Winner               int8 // -1 = ongoing, 0 = Alliance, 1 = Horde
	AllianceAccumMs      int64
	HordeAccumMs         int64
	HonorTicsAccum       [2]uint32 // resource points banked toward the next trickle-honor award
	RepTicsAccum         [2]uint32 // resource points banked toward the next trickle-rep award
	NearVictoryAnnounced bool
	StopAccumulation     chan struct{}
}

func isABBanner(entry uint32) bool {
	// Neutral: 180087..180091
	if entry >= 180087 && entry <= 180091 {
		return true
	}
	// Contested Ally: 180100..180104
	if entry >= 180100 && entry <= 180104 {
		return true
	}
	// Contested Horde: 180105..180109
	if entry >= 180105 && entry <= 180109 {
		return true
	}
	// Controlled Ally: 180110..180114
	if entry >= 180110 && entry <= 180114 {
		return true
	}
	// Controlled Horde: 180115..180119
	if entry >= 180115 && entry <= 180119 {
		return true
	}
	return false
}

func getABNodeIDFromBannerEntry(entry uint32) (nodeID uint32, ok bool) {
	if entry >= 180087 && entry <= 180091 {
		return entry - 180087, true
	}
	if entry >= 180100 && entry <= 180104 {
		return entry - 180100, true
	}
	if entry >= 180105 && entry <= 180109 {
		return entry - 180105, true
	}
	if entry >= 180110 && entry <= 180114 {
		return entry - 180110, true
	}
	if entry >= 180115 && entry <= 180119 {
		return entry - 180115, true
	}
	return 0, false
}

func (s *Server) getOrCreateABState(mapID uint32) *abBattlegroundState {
	if s == nil {
		return nil
	}
	s.abMu.Lock()
	defer s.abMu.Unlock()
	if s.abState == nil {
		s.abState = make(map[uint32]*abBattlegroundState)
	}
	state, ok := s.abState[mapID]
	if !ok {
		state = &abBattlegroundState{
			MapID:           mapID,
			MaxResources:    ABMaxResources,
			CaptureDuration: ABBannerCaptureTimeDefault,
			Winner:          -1,
		}
		// Initialize the 5 nodes in neutral state
		nodeCoords := [ABNodeMax][3]float32{
			{1166.7, 1200.1, -56.7}, // Stables
			{977.0, 1046.6, -44.8},  // Blacksmith
			{806.2, 874.3, -55.5},   // Farm
			{775.7, 1206.4, 15.7},   // Lumber Mill
			{1147.0, 843.5, -110.9}, // Gold Mine
		}
		for i := uint32(0); i < ABNodeMax; i++ {
			state.Nodes[i] = abNodeState{
				NodeID:      i,
				State:       ABNodeStateNeutral,
				PrevState:   ABNodeStateNeutral,
				BannerEntry: getABBannerEntry(i, ABNodeStateNeutral),
				X:           nodeCoords[i][0],
				Y:           nodeCoords[i][1],
				Z:           nodeCoords[i][2],
			}
		}
		s.abState[mapID] = state
	}
	return state
}

// handleABBannerUse processes player interaction with an Arathi Basin banner.
// Mirrors TrinityCore BattlegroundAB::EventPlayerClickedOnFlag (BattlegroundAB.cpp:210-310).
func (s *Server) handleABBannerUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return false
	}
	ab := s.getOrCreateABState(sess.player.Map)
	if ab == nil {
		return false
	}

	nodeID, ok := getABNodeIDFromBannerEntry(entry)
	if !ok || nodeID >= ABNodeMax {
		return false
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.Winner >= 0 {
		return true // Match already finished
	}

	node := &ab.Nodes[nodeID]
	node.AssaultedBy = sess.playerGUID

	// Range check (10.0 yards standard interaction distance)
	if distance3D(sess.player.X, sess.player.Y, sess.player.Z, node.X, node.Y, node.Z) > 10.0 {
		return true
	}

	team := teamForRace(sess.player.Race) // 0 = Alliance, 1 = Horde

	// TrinityCore EventPlayerClickedOnFlag strips ENTER_PVP_COMBAT auras after the
	// banner-legitimacy gate, before the node-state dispatch.
	sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)

	switch node.State {
	case ABNodeStateNeutral:
		// Neutral node assaulted
		if team == 0 {
			node.State = ABNodeStateContestedAlliance
		} else {
			node.State = ABNodeStateContestedHorde
		}
		node.PrevState = ABNodeStateNeutral
		s.startABNodeCaptureTimer(ab, nodeID, team)
		s.updateABNodeBanner(ab, nodeID)
		s.updateABNodeWorldStates(ab, nodeID)
		s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)

	case ABNodeStateControlledAlliance:
		if team == 1 { // Horde assaults Alliance-controlled node
			node.State = ABNodeStateContestedHorde
			node.PrevState = ABNodeStateControlledAlliance
			if ab.AllianceBasesCount > 0 {
				ab.AllianceBasesCount--
				ab.AllianceAccumMs = 0
				s.broadcastWorldState(ab.MapID, ABWorldStateBasesAlliance, ab.AllianceBasesCount)
			}
			s.startABNodeCaptureTimer(ab, nodeID, team)
			s.updateABNodeBanner(ab, nodeID)
			s.updateABNodeWorldStates(ab, nodeID)
			s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
		}

	case ABNodeStateControlledHorde:
		if team == 0 { // Alliance assaults Horde-controlled node
			node.State = ABNodeStateContestedAlliance
			node.PrevState = ABNodeStateControlledHorde
			if ab.HordeBasesCount > 0 {
				ab.HordeBasesCount--
				ab.HordeAccumMs = 0
				s.broadcastWorldState(ab.MapID, ABWorldStateBasesHorde, ab.HordeBasesCount)
			}
			s.startABNodeCaptureTimer(ab, nodeID, team)
			s.updateABNodeBanner(ab, nodeID)
			s.updateABNodeWorldStates(ab, nodeID)
			s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
		}

	case ABNodeStateContestedHorde:
		if team == 0 { // Alliance defends or contest-reclaims
			if node.CaptureTimer != nil {
				node.CaptureTimer.Stop()
				node.CaptureTimer = nil
			}
			if node.PrevState == ABNodeStateControlledAlliance {
				// Defended by Alliance! Returns immediately to controlled.
				node.State = ABNodeStateControlledAlliance
				ab.AllianceBasesCount++
				ab.AllianceAccumMs = 0
				s.broadcastWorldState(ab.MapID, ABWorldStateBasesAlliance, ab.AllianceBasesCount)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABDefended(ab.MapID, sess.player.Name, nodeID, team)
			} else {
				// Re-contested for Alliance
				node.State = ABNodeStateContestedAlliance
				s.startABNodeCaptureTimer(ab, nodeID, team)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
			}
		}

	case ABNodeStateContestedAlliance:
		if team == 1 { // Horde defends or contest-reclaims
			if node.CaptureTimer != nil {
				node.CaptureTimer.Stop()
				node.CaptureTimer = nil
			}
			if node.PrevState == ABNodeStateControlledHorde {
				// Defended by Horde! Returns immediately to controlled.
				node.State = ABNodeStateControlledHorde
				ab.HordeBasesCount++
				ab.HordeAccumMs = 0
				s.broadcastWorldState(ab.MapID, ABWorldStateBasesHorde, ab.HordeBasesCount)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABDefended(ab.MapID, sess.player.Name, nodeID, team)
			} else {
				// Re-contested for Horde
				node.State = ABNodeStateContestedHorde
				s.startABNodeCaptureTimer(ab, nodeID, team)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
			}
		}
	}

	return true
}

func (s *Server) startABNodeCaptureTimer(ab *abBattlegroundState, nodeID uint32, team uint32) {
	node := &ab.Nodes[nodeID]
	if node.CaptureTimer != nil {
		node.CaptureTimer.Stop()
	}
	duration := ab.CaptureDuration
	if duration <= 0 {
		duration = ABBannerCaptureTimeDefault
	}
	node.CaptureTimer = time.AfterFunc(duration, func() {
		s.completeABNodeCapture(ab.MapID, nodeID, team)
	})
}

func (s *Server) completeABNodeCapture(mapID, nodeID uint32, team uint32) {
	s.abMu.RLock()
	ab := s.abState[mapID]
	s.abMu.RUnlock()
	if ab == nil {
		return
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.Winner >= 0 || nodeID >= ABNodeMax {
		return
	}

	node := &ab.Nodes[nodeID]
	node.CaptureTimer = nil

	if team == 0 && node.State == ABNodeStateContestedAlliance {
		node.State = ABNodeStateControlledAlliance
		node.PrevState = ABNodeStateControlledAlliance
		ab.AllianceBasesCount++
		s.creditBGObjectiveCapture(node.AssaultedBy, nodeID)
		ab.AllianceAccumMs = 0
		s.broadcastWorldState(ab.MapID, ABWorldStateBasesAlliance, ab.AllianceBasesCount)
		s.updateABNodeBanner(ab, nodeID)
		s.updateABNodeWorldStates(ab, nodeID)
		s.announceABTaken(ab.MapID, nodeID, team)
	} else if team == 1 && node.State == ABNodeStateContestedHorde {
		node.State = ABNodeStateControlledHorde
		node.PrevState = ABNodeStateControlledHorde
		ab.HordeBasesCount++
		s.creditBGObjectiveCapture(node.AssaultedBy, nodeID)
		ab.HordeAccumMs = 0
		s.broadcastWorldState(ab.MapID, ABWorldStateBasesHorde, ab.HordeBasesCount)
		s.updateABNodeBanner(ab, nodeID)
		s.updateABNodeWorldStates(ab, nodeID)
		s.announceABTaken(ab.MapID, nodeID, team)
	}
}

func (s *Server) updateABNodeBanner(ab *abBattlegroundState, nodeID uint32) {
	if nodeID >= ABNodeMax {
		return
	}
	node := &ab.Nodes[nodeID]
	newEntry := getABBannerEntry(nodeID, node.State)
	if newEntry == 0 {
		return
	}

	// Remove or despawn old banner if active
	if node.BannerGUID != 0 {
		s.despawnDynamicGameObject(node.BannerGUID)
	}

	lowGUID := s.nextDynamicGameObjectLowGUID()
	newGUID := gameObjectGUID(lowGUID, newEntry)
	node.BannerGUID = newGUID
	node.BannerEntry = newEntry

	s.spawnDynamicGameObject(&dynamicGameObjectState{
		GUID:           newGUID,
		LowGUID:        lowGUID,
		Entry:          newEntry,
		Map:            ab.MapID,
		X:              node.X,
		Y:              node.Y,
		Z:              node.Z,
		Orientation:    0,
		State:          GameObjectStateReady,
		Type:           GameObjectTypeGoober,
		DisplayID:      newEntry,
		Size:           1.0,
		IsRuntimeSpawn: true,
	})
}

func (s *Server) updateABNodeWorldStates(ab *abBattlegroundState, nodeID uint32) {
	if nodeID >= ABNodeMax {
		return
	}
	node := &ab.Nodes[nodeID]
	// Send 1 for the active state icon, 0 for all others
	// Index in abNodeWorldStates: 0=Neutral, 1=ContAlly, 2=ContHorde, 3=AllyControlled, 4=HordeControlled
	for st := uint32(0); st < 5; st++ {
		val := uint32(0)
		if st == node.State {
			val = 1
		}
		s.broadcastWorldState(ab.MapID, abNodeWorldStates[nodeID][st], val)
	}
}

func (s *Server) announceABAssault(mapID uint32, playerName string, nodeID uint32, team uint32) {
	teamName := "Alliance"
	if team == 1 {
		teamName = "Horde"
	}
	nodeName := abNodeNames[nodeID]
	msg := fmt.Sprintf("%s has assaulted %s for the %s!", playerName, nodeName, teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
}

func (s *Server) announceABDefended(mapID uint32, playerName string, nodeID uint32, team uint32) {
	teamName := "Alliance"
	if team == 1 {
		teamName = "Horde"
	}
	nodeName := abNodeNames[nodeID]
	msg := fmt.Sprintf("%s has defended %s for the %s!", playerName, nodeName, teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
}

func (s *Server) announceABTaken(mapID uint32, nodeID uint32, team uint32) {
	teamName := "Alliance"
	if team == 1 {
		teamName = "Horde"
	}
	nodeName := abNodeNames[nodeID]
	msg := fmt.Sprintf("The %s has taken %s!", teamName, nodeName)
	s.broadcastBattlegroundMessage(mapID, msg)
}

// TickResources advances resource accumulation for elapsed milliseconds.
// Mirrors TrinityCore BattlegroundAB::Update (BattlegroundAB.cpp:115-180).
func (s *Server) TickResources(ab *abBattlegroundState, elapsedMs int64) {
	if ab == nil {
		return
	}
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.Winner >= 0 {
		return
	}

	// 1. Alliance accumulation
	if ab.AllianceBasesCount > 0 && ab.AllianceBasesCount <= 5 {
		intervalMs := abTickIntervals[ab.AllianceBasesCount].Milliseconds()
		if intervalMs > 0 {
			ab.AllianceAccumMs += elapsedMs
			for ab.AllianceAccumMs >= intervalMs && ab.Winner < 0 {
				ab.AllianceAccumMs -= intervalMs
				ab.AllianceResources += abTickPoints[ab.AllianceBasesCount]
				s.abResourceTickAwards(ab, 0, abTickPoints[ab.AllianceBasesCount])
				if ab.AllianceResources >= ab.MaxResources {
					ab.AllianceResources = ab.MaxResources
					ab.Winner = 0
					s.announceABVictory(ab.MapID, 0)
				}
				s.broadcastWorldState(ab.MapID, ABWorldStateAllianceResources, ab.AllianceResources)
			}
		}
	}

	// 2. Horde accumulation
	if ab.HordeBasesCount > 0 && ab.HordeBasesCount <= 5 {
		intervalMs := abTickIntervals[ab.HordeBasesCount].Milliseconds()
		if intervalMs > 0 {
			ab.HordeAccumMs += elapsedMs
			for ab.HordeAccumMs >= intervalMs && ab.Winner < 0 {
				ab.HordeAccumMs -= intervalMs
				ab.HordeResources += abTickPoints[ab.HordeBasesCount]
				s.abResourceTickAwards(ab, 1, abTickPoints[ab.HordeBasesCount])
				if ab.HordeResources >= ab.MaxResources {
					ab.HordeResources = ab.MaxResources
					ab.Winner = 1
					s.announceABVictory(ab.MapID, 1)
				}
				s.broadcastWorldState(ab.MapID, ABWorldStateHordeResources, ab.HordeResources)
			}
		}
	}
}

// abResourceTickAwards runs the per-resource-tick award arms of TrinityCore
// BattlegroundAB::PostUpdateImpl (BattlegroundAB.cpp:140-162): trickle reputation
// (509 League of Arathor / 510 The Defilers, +10) and trickle honor
// (GetBonusHonorFromKill(1)) banked per resource point, plus the one-time
// near-victory broadcast at BG_AB_WARNING_NEAR_VICTORY_SCORE. Arm order (rep,
// honor, near-victory) matches C++.
func (s *Server) abResourceTickAwards(ab *abBattlegroundState, team uint32, points uint32) {
	ab.RepTicsAccum[team] += points
	if ab.RepTicsAccum[team] >= abRepTicsThreshold {
		faction := uint32(509)
		if team == 1 {
			faction = 510
		}
		s.rewardBGEndReputation(ab.MapID, team, faction, 10)
		ab.RepTicsAccum[team] -= abRepTicsThreshold
	}
	ab.HonorTicsAccum[team] += points
	if ab.HonorTicsAccum[team] >= abHonorTicsThreshold {
		s.rewardBGEndHonor(ab.MapID, team, 1)
		ab.HonorTicsAccum[team] -= abHonorTicsThreshold
	}
	if !ab.NearVictoryAnnounced {
		resources := ab.AllianceResources
		if team == 1 {
			resources = ab.HordeResources
		}
		if resources > abNearVictoryScore {
			ab.NearVictoryAnnounced = true
			s.announceABNearVictory(ab.MapID, team)
		}
	}
}

func (s *Server) announceABNearVictory(mapID uint32, team uint32) {
	teamName := "Alliance"
	if team == 1 {
		teamName = "Horde"
	}
	// C++ sends broadcast_text 10598/10599 + BG_AB_SOUND_NEAR_VICTORY (8456);
	// Go uses its generic BG message convention (no broadcast_text seed, no
	// PlaySound model).
	s.broadcastBattlegroundMessage(mapID, fmt.Sprintf("The %s is near victory!", teamName))
}

func (s *Server) announceABVictory(mapID uint32, winningTeam uint32) {
	teamName := "Alliance"
	if winningTeam == 1 {
		teamName = "Horde"
	}
	msg := fmt.Sprintf("The %s wins!", teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
	// Reference: BattlegroundAB::EndBattleground (BattlegroundAB.cpp:624): the
	// winning team gets GetBonusHonorFromKill(1), then BOTH teams get the
	// completion honor (even if no team wins), ahead of Battleground::EndBattleground.
	s.rewardBGEndHonor(mapID, winningTeam, 1)
	s.rewardBGEndHonor(mapID, 0, 1)
	s.rewardBGEndHonor(mapID, 1, 1)
	s.creditBattlegroundWin(mapID, winningTeam)
}
