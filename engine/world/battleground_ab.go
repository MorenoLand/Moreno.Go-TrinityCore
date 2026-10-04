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
	ABWorldStateMaxResources      uint32 = 1778
	ABWorldStateBasesAlliance     uint32 = 1779
	ABWorldStateBasesHorde        uint32 = 1780
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
// Mirrors TrinityCore BattlegroundAB.h BG_AB_WorldStates.
var abNodeWorldStates = [ABNodeMax][5]uint32{
	// Stables: Ally=1767, ContAlly=1768, ContHorde=1769, Horde=1770, Neutral=1771
	{1771, 1768, 1769, 1767, 1770},
	// Blacksmith: Ally=1772, ContAlly=1773, ContHorde=1774, Horde=1775, Neutral=1781
	{1781, 1773, 1774, 1772, 1775},
	// Farm: Ally=1782, ContAlly=1783, ContHorde=1784, Horde=1785, Neutral=1786
	{1786, 1783, 1784, 1782, 1785},
	// Lumber Mill: Ally=1787, ContAlly=1788, ContHorde=1789, Horde=1790, Neutral=1791
	{1791, 1788, 1789, 1787, 1790},
	// Gold Mine: Ally=1792, ContAlly=1793, ContHorde=1794, Horde=1795, Neutral=1796
	{1796, 1793, 1794, 1792, 1795},
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
	mu                 sync.Mutex
	MapID              uint32
	AllianceResources  uint32
	HordeResources     uint32
	MaxResources       uint32
	AllianceBasesCount uint32
	HordeBasesCount    uint32
	Nodes              [ABNodeMax]abNodeState
	CaptureDuration    time.Duration
	Winner             int8 // -1 = ongoing, 0 = Alliance, 1 = Horde
	AllianceAccumMs    int64
	HordeAccumMs       int64
	StopAccumulation   chan struct{}
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
