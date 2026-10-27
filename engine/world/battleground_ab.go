package world

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
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
	ABWorldStateWarningResources  uint32 = 1955

	// Per-tick award thresholds from BattlegroundAB::Startup (BattlegroundAB.cpp:606-607).
	// Go has no BG-weekend model, so the non-weekend values are used, matching the
	// convention of the other per-BG reward bridges.
	abHonorTicsThreshold uint32 = 260  // BG_AB_NotABBGWeekendHonorTicks
	abRepTicsThreshold   uint32 = 160  // BG_AB_NotABBGWeekendReputationTicks
	abNearVictoryScore   uint32 = 1400 // BG_AB_WARNING_NEAR_VICTORY_SCORE

	// Objective IDs credited by BattlegroundAB::UpdatePlayerScore
	// (BattlegroundAB.h:205-206) into
	// ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE: SCORE_BASES_ASSAULTED
	// -> AB_OBJECTIVE_ASSAULT_BASE, SCORE_BASES_DEFENDED ->
	// AB_OBJECTIVE_DEFEND_BASE. The credit lands on the banner click (the
	// score update), never on capture completion.
	ABObjectiveAssaultBase uint32 = 122
	ABObjectiveDefendBase  uint32 = 123

	// Quest-reward spells cast on the capturing team by
	// BattlegroundAB::_NodeOccupied (BattlegroundAB.cpp:371-380; IDs from
	// Battleground.h:96-97).
	ABSpellQuestReward4Bases uint32 = 24061
	ABSpellQuestReward5Bases uint32 = 24064
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
	Winner               int8      // -1 = ongoing, 0 = Alliance, 1 = Horde
	EndedAt              time.Time // EndBattleground time for the _ProcessLeave auto-remove (Battleground.cpp:506-528, TIME_TO_AUTOREMOVE = 120s)
	MatchStartTime       time.Time // Go form of C++ m_StartTime (Battleground.cpp:222, accumulated elapsed): armed at state creation, since Go has no StartBattleground/STATUS_IN_PROGRESS transition for this map; the restore slot anchors its elapsed clock here instead of the login time.
	PrematureCountDown   bool      // == m_PrematureCountDown (Battleground.cpp:359): a roster-collapse countdown is armed
	PrematureTimerMs     int64     // == m_PrematureCountDownTimer: ms left on the premature-finish countdown
	AllianceAccumMs      int64
	HordeAccumMs         int64
	HonorTicsAccum       [2]uint32 // resource points banked toward the next trickle-honor award
	RepTicsAccum         [2]uint32 // resource points banked toward the next trickle-rep award
	NearVictoryAnnounced bool
	// Scores500Disadvantage is the Go form of m_TeamScores500Disadvantage
	// (BattlegroundAB.h:315): the one-way latch BattlegroundAB::PostUpdateImpl
	// (BattlegroundAB.cpp:174-177) trips for the trailing team whenever the
	// other team's lead exceeds 500. Gates the resilient_victory achievement
	// script (BattlegroundAB.cpp:707-713).
	Scores500Disadvantage [2]bool
	StopAccumulation      chan struct{}
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
			MatchStartTime:  time.Now(),
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

// deleteABState removes the per-map AB state once no BG player remains on
// the map (deleteBattlegroundStateOnEmpty). Node capture timers stop under
// ab.mu before the delete so no armed callback fires on the orphan; the
// delete only lands if the entry is still this state. Registry and state
// locks are never held together here, for the same reason as
// deleteWSGState.
func (s *Server) deleteABState(mapID uint32) {
	if s == nil {
		return
	}
	s.abMu.RLock()
	ab := s.abState[mapID]
	s.abMu.RUnlock()
	if ab == nil {
		return
	}
	ab.mu.Lock()
	s.stopABNodeCaptureTimers(ab)
	ab.mu.Unlock()
	s.abMu.Lock()
	if s.abState[mapID] == ab {
		delete(s.abState, mapID)
	}
	s.abMu.Unlock()
}

// handleABBannerUse processes player interaction with an Arathi Basin banner.
// Mirrors TrinityCore BattlegroundAB::EventPlayerClickedOnFlag (BattlegroundAB.cpp:210-310).
func (s *Server) handleABBannerUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return true
	}
	ab := s.getOrCreateABState(sess.player.Map)
	if ab == nil {
		return true
	}

	nodeID, ok := getABNodeIDFromBannerEntry(entry)
	if !ok || nodeID >= ABNodeMax {
		return true
	}

	return s.clickABNode(ab, nodeID, sess)
}

// abNodeIDNearPlayer mirrors the node scan in
// BattlegroundAB::EventPlayerClickedOnFlag (BattlegroundAB.cpp:418-430): the
// clicked flagstand resolves to the first node whose banner position is
// within 10 yards of the player. The generic flagstand arm
// (useFlagstandGameObject) relies on this since flagstand entries like
// 15001-15005 carry no node ID.
func abNodeIDNearPlayer(ab *abBattlegroundState, sess *session) (uint32, bool) {
	if ab == nil || sess == nil || sess.player == nil {
		return 0, false
	}
	for nodeID := uint32(0); nodeID < ABNodeMax; nodeID++ {
		node := &ab.Nodes[nodeID]
		if distance3D(sess.player.X, sess.player.Y, sess.player.Z, node.X, node.Y, node.Z) <= 10.0 {
			return nodeID, true
		}
	}
	return 0, false
}

// clickABNode is the node-click core of BattlegroundAB::EventPlayerClickedOnFlag
// (BattlegroundAB.cpp:210-310), shared by the banner entries and the generic
// GAMEOBJECT_TYPE_FLAGSTAND arm (which resolves the node by proximity).
func (s *Server) clickABNode(ab *abBattlegroundState, nodeID uint32, sess *session) bool {
	if s == nil || ab == nil || sess == nil || sess.player == nil || nodeID >= ABNodeMax {
		return true
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
		// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_ASSAULTED)
		// (BattlegroundAB.cpp:226) -> AB_OBJECTIVE_ASSAULT_BASE.
		s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveAssaultBase)
		s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })

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
			// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_ASSAULTED)
			// (BattlegroundAB.cpp:297) -> AB_OBJECTIVE_ASSAULT_BASE.
			s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveAssaultBase)
			s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })
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
			// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_ASSAULTED)
			// (BattlegroundAB.cpp:297) -> AB_OBJECTIVE_ASSAULT_BASE.
			s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveAssaultBase)
			s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })
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
				// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_DEFENDED)
				// (BattlegroundAB.cpp:262) -> AB_OBJECTIVE_DEFEND_BASE.
				s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveDefendBase)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesDefended++ })
			} else {
				// Re-contested for Alliance
				node.State = ABNodeStateContestedAlliance
				s.startABNodeCaptureTimer(ab, nodeID, team)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
				// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_ASSAULTED)
				// (BattlegroundAB.cpp:244) -> AB_OBJECTIVE_ASSAULT_BASE.
				s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveAssaultBase)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })
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
				// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_DEFENDED)
				// (BattlegroundAB.cpp:262) -> AB_OBJECTIVE_DEFEND_BASE.
				s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveDefendBase)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesDefended++ })
			} else {
				// Re-contested for Horde
				node.State = ABNodeStateContestedHorde
				s.startABNodeCaptureTimer(ab, nodeID, team)
				s.updateABNodeBanner(ab, nodeID)
				s.updateABNodeWorldStates(ab, nodeID)
				s.announceABAssault(ab.MapID, sess.player.Name, nodeID, team)
				// BattlegroundAB::UpdatePlayerScore(SCORE_BASES_ASSAULTED)
				// (BattlegroundAB.cpp:244) -> AB_OBJECTIVE_ASSAULT_BASE.
				s.creditBGObjectiveCapture(sess.playerGUID, ABObjectiveAssaultBase)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })
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
		// No objective credit here: BattlegroundAB::UpdatePlayerScore fires the
		// AB_OBJECTIVE_ASSAULT_BASE credit on the banner click (the
		// EventPlayerClickedOnFlag arms above), never on the node-timer
		// expiry (BattlegroundAB.cpp:115-180 has no UpdatePlayerScore leg).
		ab.AllianceAccumMs = 0
		s.broadcastWorldState(ab.MapID, ABWorldStateBasesAlliance, ab.AllianceBasesCount)
		s.updateABNodeBanner(ab, nodeID)
		s.updateABNodeWorldStates(ab, nodeID)
		s.announceABTaken(ab.MapID, nodeID, team)
		s.applyABQuestRewards(ab, team)
	} else if team == 1 && node.State == ABNodeStateContestedHorde {
		node.State = ABNodeStateControlledHorde
		node.PrevState = ABNodeStateControlledHorde
		ab.HordeBasesCount++
		// No objective credit here: see the alliance arm above.
		ab.HordeAccumMs = 0
		s.broadcastWorldState(ab.MapID, ABWorldStateBasesHorde, ab.HordeBasesCount)
		s.updateABNodeBanner(ab, nodeID)
		s.updateABNodeWorldStates(ab, nodeID)
		s.announceABTaken(ab.MapID, nodeID, team)
		s.applyABQuestRewards(ab, team)
	}
}

// applyABQuestRewards ports the quest-reward arms of BattlegroundAB::_NodeOccupied
// (BattlegroundAB.cpp:371-380): when a node capture completes, the capturing team
// gets SPELL_AB_QUEST_REWARD_5_BASES (24064) at 5 captured nodes and
// SPELL_AB_QUEST_REWARD_4_BASES (24061) at 4 (Battleground.h:96-97; both arms
// fire at 5, in this order). capturedNodes counts nodes occupied by the team
// with no node-timer running (m_NodeTimers[i] == 0), so the just-completed
// node counts — its timer was cleared on entry. CastSpellOnTeam's per-player
// loop is the Go team-aura bridge (applyTeamAura); the spirit-guide spawn,
// aura-trigger creature (SPELL_HONORABLE_DEFENDER_25Y) and RelocateDeadPlayers
// arms of _NodeOccupied/_NodeDeOccupied stay unported (no Go creature/spawn
// or graveyard model), as does GetClosestGraveyard. Called with ab.mu held.
func (s *Server) applyABQuestRewards(ab *abBattlegroundState, team uint32) {
	if s == nil || ab == nil {
		return
	}
	capturedNodes := 0
	for i := range ab.Nodes {
		n := &ab.Nodes[i]
		if n.CaptureTimer == nil &&
			((team == 0 && n.State == ABNodeStateControlledAlliance) ||
				(team == 1 && n.State == ABNodeStateControlledHorde)) {
			capturedNodes++
		}
	}
	if capturedNodes >= 5 {
		s.applyTeamAura(ab.MapID, team, ABSpellQuestReward5Bases)
	}
	if capturedNodes >= 4 {
		s.applyTeamAura(ab.MapID, team, ABSpellQuestReward4Bases)
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

// sendABInitWorldStatesTo serializes the live Arathi Basin world states into
// SMSG_INIT_WORLD_STATES. Reference: BattlegroundAB::FillInitialWorldStates
// (BattlegroundAB.cpp:306-340): node icons (1842..1846, 1 while neutral),
// node state rows (1767..1795 via the per-node plusArray, 1 on the active
// row), occupied-base counts (1779/1778), resources max (1780=1600) and
// warning (1955=1400) plus the live team scores (1776/1777), and 1861=2.
// The generic static init block no longer carries map 529 — same pattern as
// the WSG/EotS live senders.
func (s *Server) sendABInitWorldStatesTo(sess *session) {
	if sess == nil || sess.player == nil {
		return
	}
	ab := s.getOrCreateABState(sess.player.Map)
	if ab == nil {
		return
	}
	ab.mu.Lock()
	defer ab.mu.Unlock()

	worldStates := make([][2]int32, 0, 32)
	// Node icons: (m_Nodes[node] == 0) ? 1 : 0
	for node := uint32(0); node < ABNodeMax; node++ {
		val := int32(0)
		if ab.Nodes[node].State == ABNodeStateNeutral {
			val = 1
		}
		worldStates = append(worldStates, [2]int32{int32(abNodeWorldStates[node][0]), val})
	}
	// Node occupied states: NODESTATES[node]+plusArray[itr] for itr 1..4.
	for node := uint32(0); node < ABNodeMax; node++ {
		for st := uint32(1); st < 5; st++ {
			val := int32(0)
			if ab.Nodes[node].State == st {
				val = 1
			}
			worldStates = append(worldStates, [2]int32{int32(abNodeWorldStates[node][st]), val})
		}
	}
	worldStates = append(worldStates,
		[2]int32{int32(ABWorldStateBasesAlliance), int32(ab.AllianceBasesCount)},
		[2]int32{int32(ABWorldStateBasesHorde), int32(ab.HordeBasesCount)},
		[2]int32{int32(ABWorldStateMaxResources), int32(ABMaxResources)},
		[2]int32{int32(ABWorldStateWarningResources), int32(abNearVictoryScore)},
		[2]int32{int32(ABWorldStateAllianceResources), int32(ab.AllianceResources)},
		[2]int32{int32(ABWorldStateHordeResources), int32(ab.HordeResources)},
		[2]int32{1861, 2},
	)

	packet := protocol.NewBuffer(16 + len(worldStates)*8)
	packet.WriteI32(int32(sess.player.Map))
	packet.WriteI32(int32(sess.player.Zone))
	packet.WriteI32(0)
	packet.WriteU16(uint16(len(worldStates)))
	for _, ws := range worldStates {
		packet.WriteI32(ws[0])
		packet.WriteI32(ws[1])
	}
	_ = sess.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), packet.Bytes(), true)
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
					ab.EndedAt = time.Now()
					s.markBGSlotsWaitLeave(ab.MapID)
					s.stopABNodeCaptureTimers(ab)
					s.announceABVictory(ab.MapID, 0)
				}
				s.broadcastWorldState(ab.MapID, ABWorldStateAllianceResources, ab.AllianceResources)
				// update achievement flags (BattlegroundAB.cpp:174-177): the
				// alliance score just ticked up, so the horde 500-point
				// disadvantage latch trips when the lead exceeds 500. One-way
				// latch — C++ clears it only in Reset (BattlegroundAB.cpp:608-609).
				if ab.AllianceResources > ab.HordeResources+500 {
					ab.Scores500Disadvantage[1] = true
				}
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
					ab.EndedAt = time.Now()
					s.markBGSlotsWaitLeave(ab.MapID)
					s.stopABNodeCaptureTimers(ab)
					s.announceABVictory(ab.MapID, 1)
				}
				s.broadcastWorldState(ab.MapID, ABWorldStateHordeResources, ab.HordeResources)
				// update achievement flags (BattlegroundAB.cpp:174-177): the
				// horde score just ticked up, so the alliance 500-point
				// disadvantage latch trips when the lead exceeds 500. One-way
				// latch — C++ clears it only in Reset (BattlegroundAB.cpp:608-609).
				if ab.HordeResources > ab.AllianceResources+500 {
					ab.Scores500Disadvantage[0] = true
				}
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

// stopABNodeCaptureTimers cancels every armed AB banner capture timer; the
// Go form of the STATUS_IN_PROGRESS gate + instance deletion in
// Battleground::EndBattleground. Called with ab.mu held (from
// TickResources), the same lock the timer callbacks take, so Stop never
// races a running callback; the ab.Winner >= 0 gate in
// completeABNodeCapture stays as defense for a fire landing between the
// gate and Stop.
func (s *Server) stopABNodeCaptureTimers(ab *abBattlegroundState) {
	for i := range ab.Nodes {
		if ab.Nodes[i].CaptureTimer != nil {
			ab.Nodes[i].CaptureTimer.Stop()
			ab.Nodes[i].CaptureTimer = nil
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

// abPrematureWinner is the node-count leg of BattlegroundAB::GetPrematureWinner
// (BattlegroundAB.cpp:613-631): the team holding more occupied bases wins a
// premature finish. Team ids match the rest of the Go AB model (0 = alliance,
// 1 = horde). The tie fallthrough in C++ calls Battleground::GetPrematureWinner
// (Battleground.cpp:344-352), the min-players-per-team roster check — wired in
// updateBattlegroundPrematureFinish as bgBasePrematureWinner on the live
// roster counts, so a tie reports the roster winner or a draw, never no
// winner.
func abPrematureWinner(ab *abBattlegroundState) (uint32, bool) {
	if ab == nil {
		return 0, false
	}
	ally, horde := 0, 0
	for i := range ab.Nodes {
		switch ab.Nodes[i].State {
		case ABNodeStateControlledAlliance:
			ally++
		case ABNodeStateControlledHorde:
			horde++
		}
	}
	if ally > horde {
		return 0, true
	}
	if horde > ally {
		return 1, true
	}
	return 0, false
}
