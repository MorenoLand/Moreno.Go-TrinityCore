package world

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Alterac Valley (AV) Constants mirroring TrinityCore BattlegroundAV.h / BattlegroundAV.cpp.
const (
	AVMapID uint32 = 30

	AVMaxReinforcements uint32 = 600
	AVNearLoseThreshold uint32 = 120

	AVReinforcementsCaptain uint32 = 100
	AVReinforcementsTower   uint32 = 75

	AVDefaultCaptureDuration     = 240 * time.Second // 4 minutes
	AVDefaultMineTickDuration    = 45 * time.Second  // 45 seconds
	AVDefaultMineReclaimDuration = 20 * time.Minute  // 20 minutes (AV_MINE_RECLAIM_TIMER)
	AVFirstCapDuration           = 300 * time.Second // 5 minutes (BG_AV_SNOWFALL_FIRSTCAP)

	// Kill/rep rewards mirroring BattlegroundAV.h.
	AVHonorKillsBoss    uint32 = 4   // BG_AV_KILL_BOSS
	AVRepBoss           uint32 = 350 // BG_AV_REP_BOSS
	AVHonorKillsCaptain uint32 = 3   // BG_AV_KILL_CAPTAIN
	AVRepCaptain        uint32 = 125 // BG_AV_REP_CAPTAIN
	AVHonorKillsTower   uint32 = 3   // BG_AV_KILL_TOWER
	AVRepTower          uint32 = 12  // BG_AV_REP_TOWER

	// Captain buff spells mirroring BG_AV_BUFF (BattlegroundAV.h:1330-1334).
	AVBuffArmor    uint32 = 21163 // AV_BUFF_ARMOR
	AVBuffACaptain uint32 = 23693 // AV_BUFF_A_CAPTAIN: the buff the Alliance captain casts
	AVBuffHCaptain uint32 = 22751 // AV_BUFF_H_CAPTAIN: the buff the Horde captain casts

	// Teams
	AVTeamNeutral  uint8 = 0
	AVTeamAlliance uint8 = 1
	AVTeamHorde    uint8 = 2

	// Node States
	AVNodeStateControlled        uint8 = 0
	AVNodeStateContestedAlliance uint8 = 1
	AVNodeStateContestedHorde    uint8 = 2
	AVNodeStateDestroyed         uint8 = 3 // Towers only

	// Nodes (Graveyards: 0..6, Towers/Bunkers: 7..14)
	AVNodeFirstAidStation    uint32 = 0
	AVNodeStormpikeGrave     uint32 = 1
	AVNodeStoneheartGrave    uint32 = 2
	AVNodeSnowfallGrave      uint32 = 3
	AVNodeIcebloodGrave      uint32 = 4
	AVNodeFrostwolfGrave     uint32 = 5
	AVNodeFrostwolfHut       uint32 = 6
	AVNodeDunBaldarSouth     uint32 = 7
	AVNodeDunBaldarNorth     uint32 = 8
	AVNodeIcewingBunker      uint32 = 9
	AVNodeStoneheartBunker   uint32 = 10
	AVNodeIcebloodTower      uint32 = 11
	AVNodeTowerPoint         uint32 = 12
	AVNodeFrostwolfEastTower uint32 = 13
	AVNodeFrostwolfWestTower uint32 = 14
	AVNodeMax                uint32 = 15

	// Mines
	AVNorthMine uint32 = 0 // Irondeep
	AVSouthMine uint32 = 1 // Coldtooth

	// Creatures
	AVCreatureVanndar   uint32 = 11948 // Alliance General
	AVCreatureDrekThar  uint32 = 11946 // Horde General
	AVCreatureBalinda   uint32 = 11949 // Alliance Captain
	AVCreatureGalvangar uint32 = 11947 // Horde Captain

	// Mine Creatures
	AVCreatureIrondeepBoss1  uint32 = 13099
	AVCreatureIrondeepBoss2  uint32 = 13096
	AVCreatureIrondeepBoss3  uint32 = 13097
	AVCreatureColdtoothBoss1 uint32 = 13098
	AVCreatureColdtoothBoss2 uint32 = 13094
	AVCreatureColdtoothBoss3 uint32 = 13095

	// World States
	AVWorldStateAllianceScore     uint32 = 3127
	AVWorldStateHordeScore        uint32 = 3128
	AVWorldStateShowHordeScore    uint32 = 3133
	AVWorldStateShowAllianceScore uint32 = 3134
	AVWorldStateSnowfallNeutral   uint32 = 1966

	// GameObjects (Banners)
	AVObjectBannerA        uint32 = 178925
	AVObjectBannerH        uint32 = 178943
	AVObjectBannerContA    uint32 = 178940
	AVObjectBannerContH    uint32 = 179435
	AVObjectBannerSnowfall uint32 = 180418
	AVObjectMineSupplyN    uint32 = 178785
	AVObjectMineSupplyS    uint32 = 178784
)

var avNodeNames = [AVNodeMax]string{
	"Stormpike First Aid Station",
	"Stormpike Graveyard",
	"Stonehearth Graveyard",
	"Snowfall Graveyard",
	"Iceblood Graveyard",
	"Frostwolf Graveyard",
	"Frostwolf Relief Hut",
	"Dun Baldar South Bunker",
	"Dun Baldar North Bunker",
	"Icewing Bunker",
	"Stonehearth Bunker",
	"Iceblood Tower",
	"Tower Point",
	"East Frostwolf Tower",
	"West Frostwolf Tower",
}

var avNodeCoords = [AVNodeMax][3]float32{
	{638.592, -32.422, 46.061},
	{669.007, -294.078, 30.291},
	{77.801, -404.7, 46.755},
	{-202.581, -112.73, 78.488},
	{-611.962, -396.17, 60.835},
	{-1082.45, -346.823, 54.922},
	{-1402.21, -307.431, 89.442},
	{553.779, -78.657, 51.938},
	{674.001, -143.125, 63.662},
	{203.281, -360.366, 56.387},
	{-152.437, -441.758, 40.398},
	{-571.88, -262.777, 75.009},
	{-768.907, -363.71, 90.895},
	{-1302.9, -316.981, 113.867},
	{-1297.5, -266.767, 114.15},
}

// WorldState mapping matching TrinityCore BGAVNodeInfo:
// {AllianceControl, AllianceAssault/Defend, HordeControl/Destroyed, HordeAssault}
type avNodeWorldStateInfo struct {
	AllianceControl uint32
	AllianceAssault uint32
	HordeControl    uint32
	HordeAssault    uint32
	Destroyed       uint32
}

var avNodeWorldStates = [AVNodeMax]avNodeWorldStateInfo{
	{1325, 1326, 1327, 1328, 0},    // First Aid Station
	{1333, 1335, 1334, 1336, 0},    // Stormpike Graveyard
	{1302, 1304, 1301, 1303, 0},    // Stoneheart Graveyard
	{1341, 1343, 1342, 1344, 0},    // Snowfall Graveyard
	{1346, 1348, 1347, 1349, 0},    // Iceblood Graveyard
	{1337, 1339, 1338, 1340, 0},    // Frostwolf Graveyard
	{1329, 1331, 1330, 1332, 0},    // Frostwolf Hut
	{1361, 1375, 1370, 1378, 1370}, // Dun Baldar South Bunker
	{1362, 1374, 1371, 1379, 1371}, // Dun Baldar North Bunker
	{1363, 1376, 1372, 1380, 1372}, // Icewing Bunker
	{1364, 1377, 1373, 1381, 1373}, // Stoneheart Bunker
	{1368, 1390, 1385, 1395, 1368}, // Iceblood Tower
	{1367, 1389, 1384, 1394, 1367}, // Tower Point
	{1366, 1388, 1383, 1393, 1366}, // Frostwolf East Tower
	{1365, 1387, 1382, 1392, 1365}, // Frostwolf West Tower
}

// Mine WorldStates: [MineID][Alliance=0, Neutral=1, Horde=2]
var avMineWorldStates = [2][3]uint32{
	{1358, 1360, 1359}, // North Mine (Irondeep)
	{1355, 1357, 1356}, // South Mine (Coldtooth)
}

type avNodeState struct {
	NodeID       uint32
	IsTower      bool
	Owner        uint8 // 0: Neutral, 1: Alliance, 2: Horde
	State        uint8 // 0: Controlled, 1: ContestedAlliance, 2: ContestedHorde, 3: Destroyed
	PrevOwner    uint8 // Owner before contest
	CaptureTimer *time.Timer
	BannerGUID   uint64
	BannerEntry  uint32
	X, Y, Z      float32
}

type avMineState struct {
	MineID         uint32
	Owner          uint8 // 0: Neutral, 1: Alliance, 2: Horde
	ReclaimTimerMs int64 // == m_Mine_Reclaim_Timer: counts down to neutral reclaim
	LastTick       time.Time
}

type avBattlegroundState struct {
	mu                     sync.Mutex
	MapID                  uint32
	AllianceReinforcements uint32
	HordeReinforcements    uint32
	AllianceCaptainAlive   bool
	HordeCaptainAlive      bool
	CaptainBuffTimerMs     [2]int64 // == m_CaptainBuffTimer[2]: countdown-by-diff to the next team buff cast
	Nodes                  [AVNodeMax]avNodeState
	Mines                  [2]avMineState
	CaptureDuration        time.Duration
	MineTickDuration       time.Duration
	MineTickElapsedMs      int64 // == m_Mine_Timer accumulation toward the 45s score tick
	Winner                 int8  // -1: ongoing, 0: Alliance, 1: Horde
	StopTicker             chan struct{}
}

func isAVGameObject(entry uint32) bool {
	switch entry {
	case AVObjectBannerA, 178365,
		AVObjectBannerH, 178364,
		AVObjectBannerContA, 179286,
		AVObjectBannerContH, 179287,
		AVObjectBannerSnowfall,
		178927, 178955, 179446, 179436,
		AVObjectMineSupplyN, AVObjectMineSupplyS:
		return true
	}
	return false
}

func isAVMineBoss(entry uint32) (uint32, bool) {
	switch entry {
	case AVCreatureIrondeepBoss1, AVCreatureIrondeepBoss2, AVCreatureIrondeepBoss3:
		return AVNorthMine, true
	case AVCreatureColdtoothBoss1, AVCreatureColdtoothBoss2, AVCreatureColdtoothBoss3:
		return AVSouthMine, true
	}
	return 0, false
}

func getAVNodeInitialOwner(nodeID uint32) uint8 {
	switch nodeID {
	case AVNodeFirstAidStation, AVNodeStormpikeGrave, AVNodeStoneheartGrave,
		AVNodeDunBaldarSouth, AVNodeDunBaldarNorth, AVNodeIcewingBunker, AVNodeStoneheartBunker:
		return AVTeamAlliance
	case AVNodeSnowfallGrave:
		return AVTeamNeutral
	default:
		return AVTeamHorde
	}
}

func (s *Server) getOrCreateAVState(mapID uint32) *avBattlegroundState {
	if s == nil {
		return nil
	}
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	if s.avState == nil {
		s.avState = make(map[uint32]*avBattlegroundState)
	}
	state := s.avState[mapID]
	if state == nil {
		state = &avBattlegroundState{
			MapID:                  mapID,
			AllianceReinforcements: AVMaxReinforcements,
			HordeReinforcements:    AVMaxReinforcements,
			AllianceCaptainAlive:   true,
			HordeCaptainAlive:      true,
			CaptureDuration:        AVDefaultCaptureDuration,
			MineTickDuration:       AVDefaultMineTickDuration,
			Winner:                 -1,
			StopTicker:             make(chan struct{}),
		}
		// Initialize nodes
		for i := uint32(0); i < AVNodeMax; i++ {
			initOwner := getAVNodeInitialOwner(i)
			isTower := i >= AVNodeDunBaldarSouth
			bannerEntry := AVObjectBannerA
			if initOwner == AVTeamHorde {
				bannerEntry = AVObjectBannerH
			} else if initOwner == AVTeamNeutral {
				bannerEntry = AVObjectBannerSnowfall
			}
			state.Nodes[i] = avNodeState{
				NodeID:      i,
				IsTower:     isTower,
				Owner:       initOwner,
				State:       AVNodeStateControlled,
				PrevOwner:   AVTeamNeutral, // == C++ InitNode's PrevOwner = 0 (BattlegroundAV.cpp:1457)
				BannerEntry: bannerEntry,
				X:           avNodeCoords[i][0],
				Y:           avNodeCoords[i][1],
				Z:           avNodeCoords[i][2],
			}
		}
		// Initialize mines
		state.Mines[AVNorthMine] = avMineState{MineID: AVNorthMine, Owner: AVTeamNeutral}
		state.Mines[AVSouthMine] = avMineState{MineID: AVSouthMine, Owner: AVTeamNeutral}

		s.avState[mapID] = state
	}
	return state
}

// handleAVGameObjectUse processes player interaction with AV banners and mine supplies.
func (s *Server) handleAVGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return false
	}
	av := s.getOrCreateAVState(sess.player.Map)
	if av == nil {
		return false
	}

	av.mu.Lock()
	defer av.mu.Unlock()

	if av.Winner >= 0 {
		return true
	}

	// Mine supply check
	if entry == AVObjectMineSupplyN {
		s.changeAVMineOwner(av, AVNorthMine, uint8(teamForRace(sess.player.Race)+1))
		return true
	}
	if entry == AVObjectMineSupplyS {
		s.changeAVMineOwner(av, AVSouthMine, uint8(teamForRace(sess.player.Race)+1))
		return true
	}

	// Find closest node to player
	nodeID := uint32(AVNodeMax)
	minDist := float64(1000000.0)
	for i := uint32(0); i < AVNodeMax; i++ {
		d := distance3D(sess.player.X, sess.player.Y, sess.player.Z, av.Nodes[i].X, av.Nodes[i].Y, av.Nodes[i].Z)
		if d < minDist {
			minDist = d
			nodeID = i
		}
	}
	if nodeID >= AVNodeMax || minDist > 10.0 {
		return true
	}

	node := &av.Nodes[nodeID]
	if node.State == AVNodeStateDestroyed {
		return true // Destroyed tower cannot be interacted with
	}

	playerTeam := uint8(teamForRace(sess.player.Race) + 1) // 1 = Alliance, 2 = Horde

	if node.IsTower {
		// Tower interaction
		switch node.State {
		case AVNodeStateControlled:
			if node.Owner != playerTeam {
				// Assault enemy tower
				if playerTeam == AVTeamAlliance {
					node.State = AVNodeStateContestedAlliance
				} else {
					node.State = AVNodeStateContestedHorde
				}
				// Reference: BattlegroundAV::AssaultNode (BattlegroundAV.cpp:1437):
				// the capture timer is BG_AV_SNOWFALL_FIRSTCAP when the
				// pre-assault previous owner was neutral, else BG_AV_CAPTIME.
				firstCap := node.PrevOwner == AVTeamNeutral
				node.PrevOwner = node.Owner
				s.startAVNodeCaptureTimer(av, nodeID, playerTeam, firstCap)
				s.updateAVNodeBanner(av, nodeID)
				s.updateAVNodeWorldStates(av, nodeID)
				s.announceAVAssault(av.MapID, sess.accountName, nodeID, playerTeam)
			}
		case AVNodeStateContestedAlliance:
			if playerTeam == AVTeamHorde && node.PrevOwner == AVTeamHorde {
				// Horde defends tower assaulted by Alliance
				if node.CaptureTimer != nil {
					node.CaptureTimer.Stop()
					node.CaptureTimer = nil
				}
				// Reference: BattlegroundAV::DefendNode (BattlegroundAV.cpp:1467):
				// PrevOwner = Owner, and C++'s Owner is the assaulter at this point.
				node.PrevOwner = AVTeamAlliance
				node.State = AVNodeStateControlled
				node.Owner = AVTeamHorde
				s.updateAVNodeBanner(av, nodeID)
				s.updateAVNodeWorldStates(av, nodeID)
				s.announceAVDefended(av.MapID, sess.accountName, nodeID, playerTeam)
			}
		case AVNodeStateContestedHorde:
			if playerTeam == AVTeamAlliance && node.PrevOwner == AVTeamAlliance {
				// Alliance defends bunker assaulted by Horde
				if node.CaptureTimer != nil {
					node.CaptureTimer.Stop()
					node.CaptureTimer = nil
				}
				// Reference: BattlegroundAV::DefendNode (BattlegroundAV.cpp:1467):
				// PrevOwner = Owner, and C++'s Owner is the assaulter at this point.
				node.PrevOwner = AVTeamHorde
				node.State = AVNodeStateControlled
				node.Owner = AVTeamAlliance
				s.updateAVNodeBanner(av, nodeID)
				s.updateAVNodeWorldStates(av, nodeID)
				s.announceAVDefended(av.MapID, sess.accountName, nodeID, playerTeam)
			}
		}
	} else {
		// Graveyard interaction
		switch node.State {
		case AVNodeStateControlled:
			if node.Owner != playerTeam {
				// Assault enemy/neutral graveyard
				if playerTeam == AVTeamAlliance {
					node.State = AVNodeStateContestedAlliance
				} else {
					node.State = AVNodeStateContestedHorde
				}
				// Reference: BattlegroundAV::AssaultNode (BattlegroundAV.cpp:1437):
				// the capture timer is BG_AV_SNOWFALL_FIRSTCAP when the
				// pre-assault previous owner was neutral, else BG_AV_CAPTIME.
				firstCap := node.PrevOwner == AVTeamNeutral
				node.PrevOwner = node.Owner
				s.startAVNodeCaptureTimer(av, nodeID, playerTeam, firstCap)
				s.updateAVNodeBanner(av, nodeID)
				s.updateAVNodeWorldStates(av, nodeID)
				s.announceAVAssault(av.MapID, sess.accountName, nodeID, playerTeam)
			}
		case AVNodeStateContestedAlliance:
			if playerTeam == AVTeamHorde {
				if node.PrevOwner == AVTeamHorde {
					// Horde defends
					if node.CaptureTimer != nil {
						node.CaptureTimer.Stop()
						node.CaptureTimer = nil
					}
					// Reference: BattlegroundAV::DefendNode (BattlegroundAV.cpp:1467):
					// PrevOwner = Owner, and C++'s Owner is the assaulter at this point.
					node.PrevOwner = AVTeamAlliance
					node.State = AVNodeStateControlled
					node.Owner = AVTeamHorde
					s.updateAVNodeBanner(av, nodeID)
					s.updateAVNodeWorldStates(av, nodeID)
					s.announceAVDefended(av.MapID, sess.accountName, nodeID, playerTeam)
				} else {
					// Horde re-assaults
					if node.CaptureTimer != nil {
						node.CaptureTimer.Stop()
					}
					node.State = AVNodeStateContestedHorde
					// Reference: BattlegroundAV::AssaultNode (BattlegroundAV.cpp:1414):
					// on a re-assault of an already-assaulted node, C++ reads
					// PrevOwner before assignment, and it holds the first
					// assaulter (a real team, never neutral), so the timer is
					// always BG_AV_CAPTIME, never BG_AV_SNOWFALL_FIRSTCAP.
					s.startAVNodeCaptureTimer(av, nodeID, playerTeam, false)
					s.updateAVNodeBanner(av, nodeID)
					s.updateAVNodeWorldStates(av, nodeID)
					s.announceAVAssault(av.MapID, sess.accountName, nodeID, playerTeam)
				}
			}
		case AVNodeStateContestedHorde:
			if playerTeam == AVTeamAlliance {
				if node.PrevOwner == AVTeamAlliance {
					// Alliance defends
					if node.CaptureTimer != nil {
						node.CaptureTimer.Stop()
						node.CaptureTimer = nil
					}
					// Reference: BattlegroundAV::DefendNode (BattlegroundAV.cpp:1467):
					// PrevOwner = Owner, and C++'s Owner is the assaulter at this point.
					node.PrevOwner = AVTeamHorde
					node.State = AVNodeStateControlled
					node.Owner = AVTeamAlliance
					s.updateAVNodeBanner(av, nodeID)
					s.updateAVNodeWorldStates(av, nodeID)
					s.announceAVDefended(av.MapID, sess.accountName, nodeID, playerTeam)
				} else {
					// Alliance re-assaults
					if node.CaptureTimer != nil {
						node.CaptureTimer.Stop()
					}
					node.State = AVNodeStateContestedAlliance
					// Reference: BattlegroundAV::AssaultNode (BattlegroundAV.cpp:1414):
					// on a re-assault of an already-assaulted node, C++ reads
					// PrevOwner before assignment, and it holds the first
					// assaulter (a real team, never neutral), so the timer is
					// always BG_AV_CAPTIME, never BG_AV_SNOWFALL_FIRSTCAP.
					s.startAVNodeCaptureTimer(av, nodeID, playerTeam, false)
					s.updateAVNodeBanner(av, nodeID)
					s.updateAVNodeWorldStates(av, nodeID)
					s.announceAVAssault(av.MapID, sess.accountName, nodeID, playerTeam)
				}
			}
		}
	}
	return true
}

func (s *Server) startAVNodeCaptureTimer(av *avBattlegroundState, nodeID uint32, capturingTeam uint8, firstCap bool) {
	node := &av.Nodes[nodeID]
	if node.CaptureTimer != nil {
		node.CaptureTimer.Stop()
	}
	duration := av.CaptureDuration
	if duration <= 0 {
		duration = AVDefaultCaptureDuration
	}
	// Reference: BattlegroundAV::AssaultNode (BattlegroundAV.cpp:1437): the
	// timer is BG_AV_SNOWFALL_FIRSTCAP when the pre-assault previous owner was
	// neutral, else BG_AV_CAPTIME.
	if firstCap {
		duration = AVFirstCapDuration
	}

	node.CaptureTimer = time.AfterFunc(duration, func() {
		av.mu.Lock()
		defer av.mu.Unlock()

		if av.Winner >= 0 {
			return
		}

		if node.IsTower {
			// Tower burn timer expired -> tower is destroyed!
			node.State = AVNodeStateDestroyed
			loserTeam := node.PrevOwner
			s.updateAVScore(av, loserTeam, -int32(AVReinforcementsTower))
			// Reference: BattlegroundAV::EventPlayerDestroyedPoint (BattlegroundAV.cpp:565):
			// the destroying (assaulting) team gains rep + bonus honor.
			capturingIdx := uint32(capturingTeam - 1)
			capturingFaction := uint32(730) // Stormpike Guard
			if capturingTeam == AVTeamHorde {
				capturingFaction = 729 // Frostwolf Clan
			}
			s.rewardBGEndReputation(av.MapID, capturingIdx, capturingFaction, AVRepTower)
			s.rewardBGEndHonor(av.MapID, capturingIdx, AVHonorKillsTower)
			s.updateAVNodeBanner(av, nodeID)
			s.updateAVNodeWorldStates(av, nodeID)
			nodeName := avNodeNames[nodeID]
			msg := fmt.Sprintf("%s was destroyed!", nodeName)
			s.broadcastBattlegroundMessage(av.MapID, msg)
		} else {
			// Graveyard timer expired -> becomes fully controlled by capturing team
			node.State = AVNodeStateControlled
			node.Owner = capturingTeam
			s.updateAVNodeBanner(av, nodeID)
			s.updateAVNodeWorldStates(av, nodeID)
			teamName := "Alliance"
			if capturingTeam == AVTeamHorde {
				teamName = "Horde"
			}
			nodeName := avNodeNames[nodeID]
			msg := fmt.Sprintf("%s was taken by the %s!", nodeName, teamName)
			s.broadcastBattlegroundMessage(av.MapID, msg)
		}
	})
}

func (s *Server) updateAVNodeBanner(av *avBattlegroundState, nodeID uint32) {
	node := &av.Nodes[nodeID]
	if node.BannerGUID != 0 {
		s.despawnDynamicGameObject(node.BannerGUID)
		node.BannerGUID = 0
	}
	if node.State == AVNodeStateDestroyed {
		return // No banner for destroyed tower
	}

	newEntry := AVObjectBannerA
	switch node.State {
	case AVNodeStateControlled:
		if node.Owner == AVTeamHorde {
			newEntry = AVObjectBannerH
		} else if node.Owner == AVTeamNeutral {
			newEntry = AVObjectBannerSnowfall
		}
	case AVNodeStateContestedAlliance:
		newEntry = AVObjectBannerContA
	case AVNodeStateContestedHorde:
		newEntry = AVObjectBannerContH
	}
	node.BannerEntry = newEntry

	lowGUID := s.nextDynamicGameObjectLowGUID()
	guid := gameObjectGUID(lowGUID, newEntry)
	node.BannerGUID = guid

	s.spawnDynamicGameObject(&dynamicGameObjectState{
		GUID:           guid,
		LowGUID:        lowGUID,
		Entry:          newEntry,
		Map:            av.MapID,
		X:              node.X,
		Y:              node.Y,
		Z:              node.Z,
		State:          GameObjectStateReady,
		Type:           GameObjectTypeGoober,
		DisplayID:      newEntry,
		Size:           1.0,
		IsRuntimeSpawn: true,
	})
}

func (s *Server) updateAVNodeWorldStates(av *avBattlegroundState, nodeID uint32) {
	if nodeID >= AVNodeMax {
		return
	}
	node := &av.Nodes[nodeID]
	info := avNodeWorldStates[nodeID]

	// Reset all 4 worldstates to 0
	s.broadcastWorldState(av.MapID, info.AllianceControl, 0)
	s.broadcastWorldState(av.MapID, info.AllianceAssault, 0)
	s.broadcastWorldState(av.MapID, info.HordeControl, 0)
	s.broadcastWorldState(av.MapID, info.HordeAssault, 0)

	if node.NodeID == AVNodeSnowfallGrave && node.Owner == AVTeamNeutral && node.State == AVNodeStateControlled {
		s.broadcastWorldState(av.MapID, AVWorldStateSnowfallNeutral, 1)
		return
	} else if node.NodeID == AVNodeSnowfallGrave {
		s.broadcastWorldState(av.MapID, AVWorldStateSnowfallNeutral, 0)
	}

	switch node.State {
	case AVNodeStateControlled:
		if node.Owner == AVTeamAlliance {
			s.broadcastWorldState(av.MapID, info.AllianceControl, 1)
		} else if node.Owner == AVTeamHorde {
			s.broadcastWorldState(av.MapID, info.HordeControl, 1)
		}
	case AVNodeStateContestedAlliance:
		s.broadcastWorldState(av.MapID, info.AllianceAssault, 1)
	case AVNodeStateContestedHorde:
		s.broadcastWorldState(av.MapID, info.HordeAssault, 1)
	case AVNodeStateDestroyed:
		if info.Destroyed != 0 {
			s.broadcastWorldState(av.MapID, info.Destroyed, 1)
		}
	}
}

func (s *Server) changeAVMineOwner(av *avBattlegroundState, mineID uint32, newOwner uint8) {
	if mineID > AVSouthMine {
		return
	}
	mine := &av.Mines[mineID]
	if mine.Owner == newOwner {
		return
	}
	mine.Owner = newOwner
	// Reference: BattlegroundAV::ChangeMineOwner (BattlegroundAV.cpp:687):
	// m_Mine_Reclaim_Timer[mine] = AV_MINE_RECLAIM_TIMER (1200000ms) only when
	// the new owner is a faction. The same-owner early return above precedes
	// this, matching C++ arm order (re-capturing an already-owned mine does
	// not extend the reclaim timer).
	if newOwner == AVTeamAlliance || newOwner == AVTeamHorde {
		mine.ReclaimTimerMs = AVDefaultMineReclaimDuration.Milliseconds()
	} else {
		mine.ReclaimTimerMs = 0
	}
	// Update WorldStates
	for ownerIdx := uint32(0); ownerIdx < 3; ownerIdx++ {
		val := uint32(0)
		if ownerIdx == 0 && newOwner == AVTeamAlliance {
			val = 1
		} else if ownerIdx == 1 && newOwner == AVTeamNeutral {
			val = 1
		} else if ownerIdx == 2 && newOwner == AVTeamHorde {
			val = 1
		}
		s.broadcastWorldState(av.MapID, avMineWorldStates[mineID][ownerIdx], val)
	}
	teamName := "Neutral"
	if newOwner == AVTeamAlliance {
		teamName = "Alliance"
	} else if newOwner == AVTeamHorde {
		teamName = "Horde"
	}
	mineName := "Irondeep Mine"
	if mineID == AVSouthMine {
		mineName = "Coldtooth Mine"
	}
	msg := fmt.Sprintf("%s was claimed by the %s!", mineName, teamName)
	s.broadcastBattlegroundMessage(av.MapID, msg)
}

// updateAVScore updates team reinforcements.
// A negative delta removes reinforcements; positive adds them.
// If reinforcements reach 0, the team loses and opposing team wins.
// Reference: BattlegroundAV::UpdateScore (BattlegroundAV.cpp:264-285).
func (s *Server) updateAVScore(av *avBattlegroundState, team uint8, delta int32) {
	if av == nil || av.Winner >= 0 {
		return
	}
	if team == AVTeamAlliance {
		current := int32(av.AllianceReinforcements) + delta
		if current < 0 {
			current = 0
		} else if current > int32(AVMaxReinforcements) {
			current = int32(AVMaxReinforcements)
		}
		av.AllianceReinforcements = uint32(current)
		s.broadcastWorldState(av.MapID, AVWorldStateAllianceScore, av.AllianceReinforcements)
		if av.AllianceReinforcements == 0 {
			s.endAV(av, 1) // Horde wins
		}
	} else if team == AVTeamHorde {
		current := int32(av.HordeReinforcements) + delta
		if current < 0 {
			current = 0
		} else if current > int32(AVMaxReinforcements) {
			current = int32(AVMaxReinforcements)
		}
		av.HordeReinforcements = uint32(current)
		s.broadcastWorldState(av.MapID, AVWorldStateHordeScore, av.HordeReinforcements)
		if av.HordeReinforcements == 0 {
			s.endAV(av, 0) // Alliance wins
		}
	}
}

// handleAVPlayerDeath handles reinforcement decrement (-1) on player death in AV.
// Reference: BattlegroundAV::HandleKillPlayer (BattlegroundAV.cpp:74-81).
func (s *Server) handleAVPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != AVMapID {
		return
	}
	av := s.getOrCreateAVState(sess.player.Map)
	if av == nil {
		return
	}
	av.mu.Lock()
	defer av.mu.Unlock()

	team := uint8(teamForRace(sess.player.Race) + 1)
	s.updateAVScore(av, team, -1)
}

// handleAVCreatureKilled handles General, Captain, and Mine Boss kills in AV.
// Reference: BattlegroundAV::HandleKillUnit (BattlegroundAV.cpp:83-158).
func (s *Server) handleAVCreatureKilled(sess *session, creatureEntry uint32) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != AVMapID {
		return
	}
	av := s.getOrCreateAVState(sess.player.Map)
	if av == nil {
		return
	}
	av.mu.Lock()
	defer av.mu.Unlock()

	if av.Winner >= 0 {
		return
	}

	killerTeam := uint8(teamForRace(sess.player.Race) + 1)

	switch creatureEntry {
	case AVCreatureVanndar:
		// Alliance General killed -> Horde wins!
		// Reference: BattlegroundAV::HandleKillUnit (BattlegroundAV.cpp:97):
		// RewardReputationToTeam(729, BG_AV_REP_BOSS, HORDE) +
		// RewardHonorToTeam(GetBonusHonorFromKill(BG_AV_KILL_BOSS), HORDE),
		// then EndBattleground(HORDE). CastSpellOnTeam(23658) has no bridge
		// (no team-wide quest-completion spell model).
		s.rewardBGEndReputation(av.MapID, 1, 729, AVRepBoss)
		s.rewardBGEndHonor(av.MapID, 1, AVHonorKillsBoss)
		s.broadcastBattlegroundMessage(av.MapID, "General Vanndar Stormpike has been slain! The Horde is victorious!")
		s.endAV(av, 1)

	case AVCreatureDrekThar:
		// Horde General killed -> Alliance wins!
		// Reference: BattlegroundAV::HandleKillUnit (BattlegroundAV.cpp:106):
		// RewardReputationToTeam(730, BG_AV_REP_BOSS, ALLIANCE) +
		// RewardHonorToTeam(GetBonusHonorFromKill(BG_AV_KILL_BOSS), ALLIANCE),
		// then EndBattleground(ALLIANCE).
		s.rewardBGEndReputation(av.MapID, 0, 730, AVRepBoss)
		s.rewardBGEndHonor(av.MapID, 0, AVHonorKillsBoss)
		s.broadcastBattlegroundMessage(av.MapID, "General Drek'Thar has been slain! The Alliance is victorious!")
		s.endAV(av, 0)

	case AVCreatureBalinda:
		if av.AllianceCaptainAlive {
			av.AllianceCaptainAlive = false
			// Reference: BattlegroundAV::HandleKillUnit (BattlegroundAV.cpp:113):
			// RewardReputationToTeam(729, BG_AV_REP_CAPTAIN, HORDE) +
			// RewardHonorToTeam(GetBonusHonorFromKill(BG_AV_KILL_CAPTAIN), HORDE)
			// ahead of UpdateScore(ALLIANCE, -BG_AV_RES_CAPTAIN).
			s.rewardBGEndReputation(av.MapID, 1, 729, AVRepCaptain)
			s.rewardBGEndHonor(av.MapID, 1, AVHonorKillsCaptain)
			s.updateAVScore(av, AVTeamAlliance, -int32(AVReinforcementsCaptain))
			s.broadcastBattlegroundMessage(av.MapID, "Captain Balinda Stonehearth has been slain! Alliance loses 100 reinforcements!")
		}

	case AVCreatureGalvangar:
		if av.HordeCaptainAlive {
			av.HordeCaptainAlive = false
			// Reference: BattlegroundAV::HandleKillUnit (BattlegroundAV.cpp:133):
			// RewardReputationToTeam(730, BG_AV_REP_CAPTAIN, ALLIANCE) +
			// RewardHonorToTeam(GetBonusHonorFromKill(BG_AV_KILL_CAPTAIN), ALLIANCE)
			// ahead of UpdateScore(HORDE, -BG_AV_RES_CAPTAIN).
			s.rewardBGEndReputation(av.MapID, 0, 730, AVRepCaptain)
			s.rewardBGEndHonor(av.MapID, 0, AVHonorKillsCaptain)
			s.updateAVScore(av, AVTeamHorde, -int32(AVReinforcementsCaptain))
			s.broadcastBattlegroundMessage(av.MapID, "Captain Galvangar has been slain! Horde loses 100 reinforcements!")
		}

	default:
		if mineID, ok := isAVMineBoss(creatureEntry); ok {
			s.changeAVMineOwner(av, mineID, killerTeam)
		}
	}
}

// TickAVMines processes mine reinforcement ticks and neutral reclaims.
// Reference: BattlegroundAV::PostUpdateImpl (BattlegroundAV.cpp:390-407):
// m_Mine_Timer -= diff, per owned mine UpdateScore(owner, 1) when the 45s
// AV_MINE_TICK_TIMER elapses (reset afterwards), and the per-mine
// m_Mine_Reclaim_Timer countdown -> ChangeMineOwner(mine, AV_NEUTRAL_TEAM)
// on expiry.
func (s *Server) TickAVMines(av *avBattlegroundState, elapsedMs int64) {
	if av == nil {
		return
	}
	av.mu.Lock()
	defer av.mu.Unlock()

	if av.Winner >= 0 {
		return
	}

	tickMs := av.MineTickDuration.Milliseconds()
	if tickMs <= 0 {
		tickMs = AVDefaultMineTickDuration.Milliseconds()
	}
	av.MineTickElapsedMs += elapsedMs

	for i := uint32(0); i < 2; i++ {
		mine := &av.Mines[i]
		if mine.Owner != AVTeamAlliance && mine.Owner != AVTeamHorde {
			continue
		}
		if av.MineTickElapsedMs >= tickMs {
			s.updateAVScore(av, mine.Owner, 1)
		}
		if mine.ReclaimTimerMs > elapsedMs {
			mine.ReclaimTimerMs -= elapsedMs
		} else {
			s.changeAVMineOwner(av, i, AVTeamNeutral)
		}
	}
	if av.MineTickElapsedMs >= tickMs {
		av.MineTickElapsedMs = 0
	}
}

// tickAVCaptainBuffs mirrors the captain-buff arms of BattlegroundAV::PostUpdateImpl
// (BattlegroundAV.cpp:367-388): while the captain lives, m_CaptainBuffTimer ticks
// down; on expiry CastSpellOnTeam applies the captain buff to the whole team and the
// timer rearms to 120000 + urand(0,4)*60000 ("2minutes (thats the duration of the
// buff itself) + 0-4minutes"). C++ drives this from the per-BG Update on the
// BattlegroundMgr 1s sweep; here it runs from updateAVBattles on the same 1s
// cadence, ahead of the mine arms like the C++ arm order.
func (s *Server) tickAVCaptainBuffs(av *avBattlegroundState, elapsedMs int64) {
	if av == nil {
		return
	}
	av.mu.Lock()
	defer av.mu.Unlock()

	if av.Winner >= 0 {
		return
	}

	buffSpells := [2]uint32{AVBuffACaptain, AVBuffHCaptain}
	for i := 0; i < 2; i++ {
		alive := av.AllianceCaptainAlive
		if i == 1 {
			alive = av.HordeCaptainAlive
		}
		if !alive {
			continue
		}
		if av.CaptainBuffTimerMs[i] > elapsedMs {
			av.CaptainBuffTimerMs[i] -= elapsedMs
			continue
		}
		s.applyAVTeamAura(av.MapID, uint32(i), buffSpells[i], 120000)
		av.CaptainBuffTimerMs[i] = 120000 + int64(rand.Intn(5))*60000
	}
}

// applyAVTeamAura applies an aura to every worldReady session on the AV map on the
// given team (0 Alliance, 1 Horde): the Go analog of Battleground::CastSpellOnTeam's
// per-team player loop (Battleground.cpp:619-624), using the same session selection
// as rewardBGEndReputation.
func (s *Server) applyAVTeamAura(mapID, team, spellID uint32, durationMs uint32) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID && teamForRace(sess.player.Race) == team {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		sess.applyAuraWithDuration(spellID, durationMs)
	}
}

// handleAVPlayerLeave strips the AV armor/captain buffs when a player leaves the
// battleground, mirroring BattlegroundAV::RemovePlayer (BattlegroundAV.cpp:497-504).
func (s *Server) handleAVPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	s.avMu.RLock()
	av := s.avState[sess.player.Map]
	s.avMu.RUnlock()
	if av == nil {
		return
	}
	sess.removeAura(AVBuffArmor)
	sess.removeAura(AVBuffACaptain)
	sess.removeAura(AVBuffHCaptain)
}

// updateAVBattles ticks every live AV state, driving the captain buff timers, the
// mine reinforcement tick and the mine neutral-reclaim timers. BattlegroundMgr::Update
// (BattlegroundMgr.cpp:94) sweeps all running instances with bg->Update(diff)
// every BATTLEGROUND_OBJECTIVE_UPDATE_INTERVAL (BattlegroundMgr.h:38 = 1000ms);
// the 1s gate here mirrors that cadence, like updateArenaBattles.
func (s *Server) updateAVBattles(now time.Time) {
	if s == nil {
		return
	}
	if !s.avTickLast.IsZero() && now.Sub(s.avTickLast) < time.Second {
		return
	}
	var elapsedMs int64 = 1000
	if !s.avTickLast.IsZero() {
		elapsedMs = now.Sub(s.avTickLast).Milliseconds()
	}
	s.avTickLast = now
	s.avMu.RLock()
	battles := make([]*avBattlegroundState, 0, len(s.avState))
	for _, av := range s.avState {
		battles = append(battles, av)
	}
	s.avMu.RUnlock()
	for _, av := range battles {
		s.tickAVCaptainBuffs(av, elapsedMs)
		s.TickAVMines(av, elapsedMs)
	}
}

func (s *Server) endAV(av *avBattlegroundState, winner int8) {
	if av.Winner >= 0 {
		return
	}
	av.Winner = winner
	winnerTeam := "Alliance"
	if winner == 1 {
		winnerTeam = "Horde"
	}
	s.broadcastBattlegroundMessage(av.MapID, fmt.Sprintf("The %s has won the battle for Alterac Valley!", winnerTeam))
	// Reference: BattlegroundAV::EndBattleground (BattlegroundAV.cpp:453): bonus
	// kills for surviving towers (BattlegroundAV.h:45-49: 2 kills / 12 rep per
	// tower, 2 kills / 125 rep per living captain) plus Stormpike (730) /
	// Frostwolf (729) reputation, ahead of Battleground::EndBattleground.
	// The C++ arms are:
	//   towers (DUNBALDAR_SOUTH..FROSTWOLF_WTOWER) in POINT_CONTROLED state:
	//     alliance-owned: rep[alliance] += 12, kills[alliance] += 2
	//     horde-owned:    rep[alliance] += 2,  kills[horde]    += 2
	//     (the rep[alliance] += 2 arm for horde-owned towers is a genuine
	//     upstream quirk, reproduced here for 1:1 fidelity)
	//   living captains: kills[team] += 2, rep[team] += 125.
	var kills, rep [2]uint32
	for i := uint32(AVNodeDunBaldarSouth); i <= uint32(AVNodeFrostwolfWestTower); i++ {
		node := &av.Nodes[i]
		if node.State != AVNodeStateControlled {
			continue
		}
		if node.Owner == AVTeamAlliance {
			rep[0] += 12
			kills[0] += 2
		} else {
			rep[0] += 2
			kills[1] += 2
		}
	}
	if av.AllianceCaptainAlive {
		kills[0] += 2
		rep[0] += 125
	}
	if av.HordeCaptainAlive {
		kills[1] += 2
		rep[1] += 125
	}
	factions := [2]uint32{730, 729} // Stormpike Guard, Frostwolf Clan
	for team := uint32(0); team < 2; team++ {
		if rep[team] != 0 {
			s.rewardBGEndReputation(av.MapID, team, factions[team], rep[team])
		}
		if kills[team] != 0 {
			s.rewardBGEndHonor(av.MapID, team, kills[team])
		}
	}
}

func (s *Server) sendAVInitialWorldStates(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	av := s.getOrCreateAVState(sess.player.Map)
	if av == nil {
		return
	}
	av.mu.Lock()
	defer av.mu.Unlock()

	sess.sendWorldState(AVWorldStateShowAllianceScore, 1)
	sess.sendWorldState(AVWorldStateShowHordeScore, 1)
	sess.sendWorldState(AVWorldStateAllianceScore, av.AllianceReinforcements)
	sess.sendWorldState(AVWorldStateHordeScore, av.HordeReinforcements)

	for i := uint32(0); i < AVNodeMax; i++ {
		node := &av.Nodes[i]
		info := avNodeWorldStates[i]
		if node.NodeID == AVNodeSnowfallGrave && node.Owner == AVTeamNeutral && node.State == AVNodeStateControlled {
			sess.sendWorldState(AVWorldStateSnowfallNeutral, 1)
		} else {
			switch node.State {
			case AVNodeStateControlled:
				if node.Owner == AVTeamAlliance {
					sess.sendWorldState(info.AllianceControl, 1)
				} else if node.Owner == AVTeamHorde {
					sess.sendWorldState(info.HordeControl, 1)
				}
			case AVNodeStateContestedAlliance:
				sess.sendWorldState(info.AllianceAssault, 1)
			case AVNodeStateContestedHorde:
				sess.sendWorldState(info.HordeAssault, 1)
			case AVNodeStateDestroyed:
				if info.Destroyed != 0 {
					sess.sendWorldState(info.Destroyed, 1)
				}
			}
		}
	}

	for i := uint32(0); i < 2; i++ {
		mine := &av.Mines[i]
		for ownerIdx := uint32(0); ownerIdx < 3; ownerIdx++ {
			val := uint32(0)
			if ownerIdx == 0 && mine.Owner == AVTeamAlliance {
				val = 1
			} else if ownerIdx == 1 && mine.Owner == AVTeamNeutral {
				val = 1
			} else if ownerIdx == 2 && mine.Owner == AVTeamHorde {
				val = 1
			}
			sess.sendWorldState(avMineWorldStates[i][ownerIdx], val)
		}
	}
}

func (s *Server) announceAVAssault(mapID uint32, playerName string, nodeID uint32, team uint8) {
	teamName := "Alliance"
	if team == AVTeamHorde {
		teamName = "Horde"
	}
	nodeName := avNodeNames[nodeID]
	msg := fmt.Sprintf("%s has assaulted %s for the %s!", playerName, nodeName, teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
}

func (s *Server) announceAVDefended(mapID uint32, playerName string, nodeID uint32, team uint8) {
	teamName := "Alliance"
	if team == AVTeamHorde {
		teamName = "Horde"
	}
	nodeName := avNodeNames[nodeID]
	msg := fmt.Sprintf("%s has defended %s for the %s!", playerName, nodeName, teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
}

func (s *session) sendWorldState(variableID, value uint32) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU32(variableID)
	buf.WriteU32(value)
	_ = s.write(uint16(protocol.OpcodeSMSG_UPDATE_WORLD_STATE), buf.Bytes(), true)
}
