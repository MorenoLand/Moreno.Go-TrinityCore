package world

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Eye of the Storm (EotS) Constants mirroring TrinityCore BattlegroundEY.h / BattlegroundEY.cpp.
const (
	EOTSMapID uint32 = 566

	EOTSTowerMage      uint32 = 0
	EOTSTowerDraenei   uint32 = 1
	EOTSTowerBloodElf  uint32 = 2
	EOTSTowerFelReaver uint32 = 3
	EOTSTowerMax       uint32 = 4

	EOTSTowerStateNeutral            uint32 = 0
	EOTSTowerStateControlledAlliance uint32 = 1
	EOTSTowerStateControlledHorde    uint32 = 2

	EOTSFlagStateAtCenter uint32 = 1
	EOTSFlagStateCarried  uint32 = 2
	EOTSFlagStateDropped  uint32 = 3

	EOTSFlagCenterEntry  uint32 = 184141
	EOTSFlagDroppedEntry uint32 = 184142

	EOTSSpellNetherstormFlag uint32 = 34976

	EOTSMaxResources uint32 = 1600

	// World States
	EOTSWorldStateAllianceResources uint32 = 2749
	EOTSWorldStateHordeResources    uint32 = 2750
	EOTSWorldStateMaxResources      uint32 = 2751
	EOTSWorldStateBasesAlliance     uint32 = 2752
	EOTSWorldStateBasesHorde        uint32 = 2753
	EOTSWorldStateFlagState         uint32 = 2757
	EOTSWorldStateFlagStateAlliance uint32 = 2769
	EOTSWorldStateFlagStateHorde    uint32 = 2770
)

// Flag lifecycle timings from BattlegroundEY.h / BattlegroundEY.cpp:
// BG_EY_FLAG_RESPAWN_TIME = 8s drives both the post-capture respawn
// (EventPlayerCapturedFlag) and the dropped-flag return
// (EventPlayerDroppedFlag's m_FlagsTimer arm).
const EOTSFlagRespawnTime = 8 * time.Second

// PostUpdateImpl resource-tick cadence: BG_EY_FPOINTS_TICK_TIME = 2s
// (BattlegroundEY.h:29).
const eotsPointTickIntervalMs int64 = 2000

// Honor-score tic threshold: BG_EY_NotEYWeekendHonorTicks = 260
// (BattlegroundEY.h:220), the non-weekend Startup value; Go has no
// BG-weekend model (established convention).
const eotsNotWeekendHonorTics uint32 = 260

// Per-team flag-state values broadcast on 2769/2770
// (BG_EY_FLAG_STATE_*, BattlegroundEY.h:231-234).
const (
	eotsFlagStateWaitRespawn uint32 = 1
	eotsFlagStateOnPlayer    uint32 = 2
)

var eotsTowerNames = [EOTSTowerMax]string{
	"Mage Tower",
	"Draenei Ruins",
	"Blood Elf Tower",
	"Fel Reaver Ruins",
}

// Tower WorldState icons:
// Index in eotsTowerWorldStates: 0=Neutral, 1=ControlledAlliance, 2=ControlledHorde
var eotsTowerWorldStates = [EOTSTowerMax][3]uint32{
	{2724, 2722, 2723}, // Mage Tower
	{2727, 2725, 2726}, // Draenei Ruins
	{2730, 2728, 2729}, // Blood Elf Tower
	{2733, 2731, 2732}, // Fel Reaver Ruins
}

// Banner GameObject entries:
// Neutral: 184080..184083
// Alliance: 184084..184087
// Horde: 184088..184091
func getEOTSBannerEntry(towerID uint32, state uint32) uint32 {
	if towerID >= EOTSTowerMax {
		return 0
	}
	switch state {
	case EOTSTowerStateNeutral:
		return 184080 + towerID
	case EOTSTowerStateControlledAlliance:
		return 184084 + towerID
	case EOTSTowerStateControlledHorde:
		return 184088 + towerID
	default:
		return 0
	}
}

// Flag capture points based on number of towers controlled (TrinityCore BattlegroundEY.cpp:780):
// 1 tower: 75 pts, 2 towers: 85 pts, 3 towers: 100 pts, 4 towers: 500 pts
var eotsFlagCapturePoints = [5]uint32{0, 75, 85, 100, 500}

// Continuous points per second based on number of towers controlled (TrinityCore BattlegroundEY.cpp:115):
// 0: 0, 1: 1, 2: 2, 3: 5, 4: 10
var eotsTowerTickPoints = [5]uint32{0, 1, 2, 5, 10}

type eotsTowerState struct {
	TowerID     uint32
	State       uint32
	BannerGUID  uint64
	BannerEntry uint32
	X, Y, Z     float32
}

type eotsBattlegroundState struct {
	mu                  sync.Mutex
	MapID               uint32
	AllianceResources   uint32
	HordeResources      uint32
	MaxResources        uint32
	AllianceTowersCount uint32
	HordeTowersCount    uint32
	Towers              [EOTSTowerMax]eotsTowerState
	FlagState           uint32
	FlagCarrierGUID     uint64
	FlagCenterGUID      uint64
	FlagDroppedGUID     uint64
	FlagReturnTimer     *time.Timer
	FlagRespawnTimer    *time.Timer
	PointTickAccumMs    int64     // m_PointAddingTimer countdown (PostUpdateImpl arm)
	HonorTicsAccum      [2]uint32 // m_HonorScoreTics per team (AddPoints arm)
	Winner              int8      // -1 = ongoing, 0 = Alliance, 1 = Horde
	CenterX             float32
	CenterY             float32
	CenterZ             float32
	AllianceAccumMs     int64
	HordeAccumMs        int64
}

func isEOTSGameObject(entry uint32) bool {
	if entry == EOTSFlagCenterEntry || entry == EOTSFlagDroppedEntry {
		return true
	}
	// Banners 184080..184091
	if entry >= 184080 && entry <= 184091 {
		return true
	}
	return false
}

func getEOTSTowerIDFromBannerEntry(entry uint32) (towerID uint32, ok bool) {
	if entry >= 184080 && entry <= 184083 {
		return entry - 184080, true
	}
	if entry >= 184084 && entry <= 184087 {
		return entry - 184084, true
	}
	if entry >= 184088 && entry <= 184091 {
		return entry - 184088, true
	}
	return 0, false
}

func (s *Server) getOrCreateEOTSState(mapID uint32) *eotsBattlegroundState {
	if s == nil {
		return nil
	}
	s.eotsMu.Lock()
	defer s.eotsMu.Unlock()
	if s.eotsState == nil {
		s.eotsState = make(map[uint32]*eotsBattlegroundState)
	}
	state, ok := s.eotsState[mapID]
	if !ok {
		state = &eotsBattlegroundState{
			MapID:        mapID,
			MaxResources: EOTSMaxResources,
			FlagState:    EOTSFlagStateAtCenter,
			Winner:       -1,
			CenterX:      2174.0,
			CenterY:      1569.0,
			CenterZ:      1160.0,
		}
		// Initialize the 4 towers
		towerCoords := [EOTSTowerMax][3]float32{
			{2228.4, 1330.4, 1199.0}, // Mage Tower
			{2167.3, 1332.6, 1200.0}, // Draenei Ruins
			{2135.0, 1775.0, 1188.0}, // Blood Elf Tower
			{2284.0, 1731.0, 1189.0}, // Fel Reaver Ruins
		}
		for i := uint32(0); i < EOTSTowerMax; i++ {
			state.Towers[i] = eotsTowerState{
				TowerID:     i,
				State:       EOTSTowerStateNeutral,
				BannerEntry: getEOTSBannerEntry(i, EOTSTowerStateNeutral),
				X:           towerCoords[i][0],
				Y:           towerCoords[i][1],
				Z:           towerCoords[i][2],
			}
		}
		s.eotsState[mapID] = state
	}
	return state
}

func (s *Server) handleEOTSGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return false
	}
	eots := s.getOrCreateEOTSState(sess.player.Map)
	if eots == nil {
		return false
	}

	eots.mu.Lock()
	defer eots.mu.Unlock()

	if eots.Winner >= 0 {
		return true // Match ended
	}

	team := teamForRace(sess.player.Race) // 0 = Alliance, 1 = Horde

	// 1. Center Flag Pickup
	if entry == EOTSFlagCenterEntry {
		if eots.FlagState != EOTSFlagStateAtCenter {
			return true
		}
		// Range check to center
		if distance3D(sess.player.X, sess.player.Y, sess.player.Z, eots.CenterX, eots.CenterY, eots.CenterZ) > 10.0 {
			return true
		}

		eots.FlagState = EOTSFlagStateCarried
		eots.FlagCarrierGUID = sess.playerGUID
		if eots.FlagCenterGUID != 0 {
			s.setGameObjectHidden(eots.FlagCenterGUID, true)
		}

		sess.applyAura(EOTSSpellNetherstormFlag)
		// Reference: BattlegroundEY::EventPlayerClickedOnFlag
		// (BattlegroundEY.cpp:642) removes ENTER_PVP_COMBAT auras on pickup
		// and flags the picker's team worldstate ON_PLAYER.
		sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)
		s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagState, EOTSFlagStateCarried)
		if team == 0 {
			s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateAlliance, eotsFlagStateOnPlayer)
		} else {
			s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateHorde, eotsFlagStateOnPlayer)
		}

		teamName := "Alliance"
		if team == 1 {
			teamName = "Horde"
		}
		s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("%s has taken the Netherstorm Flag for the %s!", sess.player.Name, teamName))
		return true
	}

	// 2. Dropped Flag Pickup
	if entry == EOTSFlagDroppedEntry {
		if eots.FlagState != EOTSFlagStateDropped {
			return true
		}
		// Stop return timer
		if eots.FlagReturnTimer != nil {
			eots.FlagReturnTimer.Stop()
			eots.FlagReturnTimer = nil
		}
		s.despawnDynamicGameObject(eots.FlagDroppedGUID)
		eots.FlagDroppedGUID = 0

		eots.FlagState = EOTSFlagStateCarried
		eots.FlagCarrierGUID = sess.playerGUID
		sess.applyAura(EOTSSpellNetherstormFlag)
		// Same EventPlayerClickedOnFlag arm as the center pickup:
		// ENTER_PVP_COMBAT strip + picker's team flag-state.
		sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)
		s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagState, EOTSFlagStateCarried)
		if team == 0 {
			s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateAlliance, eotsFlagStateOnPlayer)
		} else {
			s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateHorde, eotsFlagStateOnPlayer)
		}

		teamName := "Alliance"
		if team == 1 {
			teamName = "Horde"
		}
		s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("%s has picked up the Netherstorm Flag for the %s!", sess.player.Name, teamName))
		return true
	}

	// 3. Tower Banner interaction
	towerID, ok := getEOTSTowerIDFromBannerEntry(entry)
	if !ok || towerID >= EOTSTowerMax {
		return false
	}

	tower := &eots.Towers[towerID]
	if distance3D(sess.player.X, sess.player.Y, sess.player.Z, tower.X, tower.Y, tower.Z) > 10.0 {
		return true
	}

	// If player is carrying the Netherstorm flag AND this tower is controlled by player's team -> FLAG CAPTURE!
	if eots.FlagState == EOTSFlagStateCarried && eots.FlagCarrierGUID == sess.playerGUID {
		isControlledByTeam := (team == 0 && tower.State == EOTSTowerStateControlledAlliance) ||
			(team == 1 && tower.State == EOTSTowerStateControlledHorde)

		if isControlledByTeam {
			s.captureEOTSFlag(eots, sess, team)
			return true
		}
	}

	// Otherwise, capturing or contesting the tower:
	if team == 0 { // Alliance claims/assaults tower
		if tower.State != EOTSTowerStateControlledAlliance {
			if tower.State == EOTSTowerStateControlledHorde && eots.HordeTowersCount > 0 {
				eots.HordeTowersCount--
				s.broadcastWorldState(eots.MapID, EOTSWorldStateBasesHorde, eots.HordeTowersCount)
			}
			tower.State = EOTSTowerStateControlledAlliance
			eots.AllianceTowersCount++
			s.broadcastWorldState(eots.MapID, EOTSWorldStateBasesAlliance, eots.AllianceTowersCount)
			s.updateEOTSTowerBanner(eots, towerID)
			s.updateEOTSTowerWorldStates(eots, towerID)
			s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("The Alliance has captured the %s!", eotsTowerNames[towerID]))
		}
	} else { // Horde claims/assaults tower
		if tower.State != EOTSTowerStateControlledHorde {
			if tower.State == EOTSTowerStateControlledAlliance && eots.AllianceTowersCount > 0 {
				eots.AllianceTowersCount--
				s.broadcastWorldState(eots.MapID, EOTSWorldStateBasesAlliance, eots.AllianceTowersCount)
			}
			tower.State = EOTSTowerStateControlledHorde
			eots.HordeTowersCount++
			s.broadcastWorldState(eots.MapID, EOTSWorldStateBasesHorde, eots.HordeTowersCount)
			s.updateEOTSTowerBanner(eots, towerID)
			s.updateEOTSTowerWorldStates(eots, towerID)
			s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("The Horde has captured the %s!", eotsTowerNames[towerID]))
		}
	}

	return true
}

func (s *Server) captureEOTSFlag(eots *eotsBattlegroundState, sess *session, team uint32) {
	s.creditBGObjectiveCapture(sess.playerGUID, 0xFFFFFFFF) // flag capture objective
	sess.removeAura(EOTSSpellNetherstormFlag)
	// Reference: BattlegroundEY::EventPlayerCapturedFlag
	// (BattlegroundEY.cpp:784) strips ENTER_PVP_COMBAT auras on capture too.
	sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)
	eots.FlagCarrierGUID = 0
	eots.FlagState = EOTSFlagStateAtCenter

	towersHeld := eots.AllianceTowersCount
	if team == 1 {
		towersHeld = eots.HordeTowersCount
	}
	if towersHeld > 4 {
		towersHeld = 4
	}

	// Reference: EventPlayerCapturedFlag awards AddPoints(team,
	// BG_EY_FlagPoints[m_TeamPointsCount-1]) when the team holds a tower.
	s.addEOTSResources(eots, team, eotsFlagCapturePoints[towersHeld])

	teamName := "Alliance"
	if team == 1 {
		teamName = "Horde"
	}

	s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagState, EOTSFlagStateAtCenter)
	s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("%s captured the Netherstorm Flag for the %s (+%d resources)!", sess.player.Name, teamName, eotsFlagCapturePoints[towersHeld]))

	// Respawn central flag after BG_EY_FLAG_RESPAWN_TIME (8s)
	if eots.FlagRespawnTimer != nil {
		eots.FlagRespawnTimer.Stop()
	}
	eots.FlagRespawnTimer = time.AfterFunc(EOTSFlagRespawnTime, func() {
		eots.mu.Lock()
		defer eots.mu.Unlock()
		if eots.FlagCenterGUID != 0 {
			s.setGameObjectHidden(eots.FlagCenterGUID, false)
		}
		s.broadcastBattlegroundMessage(eots.MapID, "The Netherstorm Flag has reset!")
	})
}

// handleEOTSPlayerDeath drops the flag if the dying player is carrying it.
func (s *Server) handleEOTSPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != EOTSMapID {
		return
	}
	s.eotsMu.RLock()
	eots := s.eotsState[sess.player.Map]
	s.eotsMu.RUnlock()
	if eots == nil {
		return
	}

	eots.mu.Lock()
	defer eots.mu.Unlock()

	if eots.FlagCarrierGUID == sess.playerGUID {
		s.dropEOTSFlag(eots, sess)
	}
}

// handleEOTSPlayerLeave drops the flag if a leaving player is carrying it.
func (s *Server) handleEOTSPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != EOTSMapID {
		return
	}
	s.eotsMu.RLock()
	eots := s.eotsState[sess.player.Map]
	s.eotsMu.RUnlock()
	if eots == nil {
		return
	}

	eots.mu.Lock()
	defer eots.mu.Unlock()

	if eots.FlagCarrierGUID == sess.playerGUID {
		s.dropEOTSFlag(eots, sess)
	}
}

func (s *Server) dropEOTSFlag(eots *eotsBattlegroundState, sess *session) {
	sess.removeAura(EOTSSpellNetherstormFlag)
	eots.FlagCarrierGUID = 0
	eots.FlagState = EOTSFlagStateDropped

	droppedLow := s.nextDynamicGameObjectLowGUID()
	droppedGUID := gameObjectGUID(droppedLow, EOTSFlagDroppedEntry)
	eots.FlagDroppedGUID = droppedGUID

	s.spawnDynamicGameObject(&dynamicGameObjectState{
		GUID:           droppedGUID,
		LowGUID:        droppedLow,
		Entry:          EOTSFlagDroppedEntry,
		Map:            eots.MapID,
		X:              sess.player.X,
		Y:              sess.player.Y,
		Z:              sess.player.Z,
		State:          GameObjectStateReady,
		Type:           GameObjectTypeFlagDrop,
		DisplayID:      EOTSFlagDroppedEntry,
		Size:           1.0,
		IsRuntimeSpawn: true,
	})

	s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagState, EOTSFlagStateDropped)
	// Reference: BattlegroundEY::EventPlayerDroppedFlag (BattlegroundEY.cpp:607)
	// resets both per-team flag-state worldstates to WAIT_RESPAWN.
	s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateAlliance, eotsFlagStateWaitRespawn)
	s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagStateHorde, eotsFlagStateWaitRespawn)
	s.broadcastBattlegroundMessage(eots.MapID, fmt.Sprintf("The Netherstorm Flag was dropped by %s!", sess.player.Name))

	if eots.FlagReturnTimer != nil {
		eots.FlagReturnTimer.Stop()
	}
	// Dropped-flag return also runs on BG_EY_FLAG_RESPAWN_TIME (8s).
	eots.FlagReturnTimer = time.AfterFunc(EOTSFlagRespawnTime, func() {
		eots.mu.Lock()
		defer eots.mu.Unlock()
		if eots.FlagState == EOTSFlagStateDropped {
			s.despawnDynamicGameObject(eots.FlagDroppedGUID)
			eots.FlagDroppedGUID = 0
			eots.FlagState = EOTSFlagStateAtCenter
			if eots.FlagCenterGUID != 0 {
				s.setGameObjectHidden(eots.FlagCenterGUID, false)
			}
			s.broadcastWorldState(eots.MapID, EOTSWorldStateFlagState, EOTSFlagStateAtCenter)
			s.broadcastBattlegroundMessage(eots.MapID, "The Netherstorm Flag has reset to the center!")
		}
	})
}

func (s *Server) updateEOTSTowerBanner(eots *eotsBattlegroundState, towerID uint32) {
	if towerID >= EOTSTowerMax {
		return
	}
	tower := &eots.Towers[towerID]
	newEntry := getEOTSBannerEntry(towerID, tower.State)
	if newEntry == 0 {
		return
	}

	if tower.BannerGUID != 0 {
		s.despawnDynamicGameObject(tower.BannerGUID)
	}

	lowGUID := s.nextDynamicGameObjectLowGUID()
	newGUID := gameObjectGUID(lowGUID, newEntry)
	tower.BannerGUID = newGUID
	tower.BannerEntry = newEntry

	s.spawnDynamicGameObject(&dynamicGameObjectState{
		GUID:           newGUID,
		LowGUID:        lowGUID,
		Entry:          newEntry,
		Map:            eots.MapID,
		X:              tower.X,
		Y:              tower.Y,
		Z:              tower.Z,
		Orientation:    0,
		State:          GameObjectStateReady,
		Type:           GameObjectTypeGoober,
		DisplayID:      newEntry,
		Size:           1.0,
		IsRuntimeSpawn: true,
	})
}

func (s *Server) updateEOTSTowerWorldStates(eots *eotsBattlegroundState, towerID uint32) {
	if towerID >= EOTSTowerMax {
		return
	}
	tower := &eots.Towers[towerID]
	// Send 1 for active state icon, 0 for others
	for st := uint32(0); st < 3; st++ {
		val := uint32(0)
		if st == tower.State {
			val = 1
		}
		s.broadcastWorldState(eots.MapID, eotsTowerWorldStates[towerID][st], val)
	}
}

// TickResources advances continuous resource generation for elapsed milliseconds.
// Mirrors TrinityCore BattlegroundEY::PostUpdateImpl (BattlegroundEY.cpp:80-92):
// m_PointAddingTimer counts down by diff and the AddPoints arms fire every
// BG_EY_FPOINTS_TICK_TIME (2s). Wired from the world tick via updateEOTSBattles.
func (s *Server) TickEOTSResources(eots *eotsBattlegroundState, elapsedMs int64) {
	if eots == nil {
		return
	}
	eots.mu.Lock()
	defer eots.mu.Unlock()

	if eots.Winner >= 0 {
		return
	}

	eots.PointTickAccumMs -= elapsedMs
	if eots.PointTickAccumMs > 0 {
		return
	}
	eots.PointTickAccumMs = eotsPointTickIntervalMs

	if eots.AllianceTowersCount > 0 && eots.AllianceTowersCount <= 4 {
		s.addEOTSResources(eots, 0, eotsTowerTickPoints[eots.AllianceTowersCount])
	}
	if eots.HordeTowersCount > 0 && eots.HordeTowersCount <= 4 {
		s.addEOTSResources(eots, 1, eotsTowerTickPoints[eots.HordeTowersCount])
	}
}

// addEOTSResources mirrors BattlegroundEY::AddPoints (BattlegroundEY.cpp:144):
// score add, honor-score tic banking against m_HonorTics (GetBonusHonorFromKill(1)
// per team when the non-weekend 260-point threshold is reached), then the
// UpdateTeamScore arm (1600 clamp, EndBattleground on reaching max, resource
// worldstate). eots.mu is held by the caller.
func (s *Server) addEOTSResources(eots *eotsBattlegroundState, team uint32, points uint32) {
	var resources *uint32
	var honorTics *uint32
	var wsID uint32
	if team == 0 {
		resources = &eots.AllianceResources
		honorTics = &eots.HonorTicsAccum[0]
		wsID = EOTSWorldStateAllianceResources
	} else {
		resources = &eots.HordeResources
		honorTics = &eots.HonorTicsAccum[1]
		wsID = EOTSWorldStateHordeResources
	}

	*resources += points
	*honorTics += points
	if *honorTics >= eotsNotWeekendHonorTics {
		*honorTics -= eotsNotWeekendHonorTics
		s.rewardBGEndHonor(eots.MapID, team, 1)
	}

	if *resources >= eots.MaxResources {
		*resources = eots.MaxResources
		if eots.Winner < 0 {
			eots.Winner = int8(team)
			s.announceEOTSVictory(eots.MapID, team)
		}
	}
	s.broadcastWorldState(eots.MapID, wsID, *resources)
}

// updateEOTSBattles ticks every live EotS state, driving the resource point tick
// (BattlegroundEY::PostUpdateImpl's m_PointAddingTimer arm). BattlegroundMgr::Update
// (BattlegroundMgr.cpp:94) sweeps all running instances with bg->Update(diff)
// every BATTLEGROUND_OBJECTIVE_UPDATE_INTERVAL (BattlegroundMgr.h:38 = 1000ms);
// the 1s gate here mirrors that cadence, like updateArenaBattles/updateAVBattles.
func (s *Server) updateEOTSBattles(now time.Time) {
	if s == nil {
		return
	}
	if !s.eotsTickLast.IsZero() && now.Sub(s.eotsTickLast) < time.Second {
		return
	}
	var elapsedMs int64 = 1000
	if !s.eotsTickLast.IsZero() {
		elapsedMs = now.Sub(s.eotsTickLast).Milliseconds()
	}
	s.eotsTickLast = now
	s.eotsMu.RLock()
	battles := make([]*eotsBattlegroundState, 0, len(s.eotsState))
	for _, eots := range s.eotsState {
		battles = append(battles, eots)
	}
	s.eotsMu.RUnlock()
	for _, eots := range battles {
		s.TickEOTSResources(eots, elapsedMs)
	}
}

func (s *Server) announceEOTSVictory(mapID uint32, winningTeam uint32) {
	teamName := "Alliance"
	if winningTeam == 1 {
		teamName = "Horde"
	}
	msg := fmt.Sprintf("The %s wins!", teamName)
	s.broadcastBattlegroundMessage(mapID, msg)
	// Reference: BattlegroundEY::EndBattleground (BattlegroundEY.cpp:316): the
	// winning team gets GetBonusHonorFromKill(1), then BOTH teams get the
	// completion honor, ahead of Battleground::EndBattleground.
	s.rewardBGEndHonor(mapID, winningTeam, 1)
	s.rewardBGEndHonor(mapID, 0, 1)
	s.rewardBGEndHonor(mapID, 1, 1)
	s.creditBattlegroundWin(mapID, winningTeam)
}

func (s *Server) getEOTSFlagCarriers(mapID uint32) []*session {
	if s == nil || mapID != EOTSMapID {
		return nil
	}
	s.eotsMu.RLock()
	eots := s.eotsState[mapID]
	s.eotsMu.RUnlock()
	if eots == nil {
		return nil
	}

	eots.mu.Lock()
	defer eots.mu.Unlock()

	var carriers []*session
	if eots.FlagCarrierGUID != 0 {
		if sess := s.findSessionByGUID(eots.FlagCarrierGUID); sess != nil {
			carriers = append(carriers, sess)
		}
	}
	return carriers
}
