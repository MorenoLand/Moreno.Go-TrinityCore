package world

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Wintergrasp (WG) Constants mirroring TrinityCore BattlefieldWG.h / BattlefieldWG.cpp.
const (
	WGMapID  uint32 = 571  // Northrend
	WGZoneID uint32 = 4197 // Wintergrasp zone

	// WGBattleID mirrors BATTLEFIELD_BATTLEID_WG (Battlefield.h:34); the only
	// battlefield battle id registered with BattlefieldMgr.
	WGBattleID uint32 = 1

	// Durations
	WGBattleDuration        = 30 * time.Minute  // 30 minutes war
	WGNoWarDuration         = 150 * time.Minute // 2.5 hours peace
	WGSouthTowerTimePenalty = 10 * time.Minute  // 10 minutes removed when all 3 south towers fall
	WGStartGroupingDuration = 15 * time.Minute  // 15 minutes before war
	wgResurrectInterval     = 30 * time.Second  // RESURRECTION_INTERVAL (Battlefield.cpp:191-199)

	// Teams
	WGTeamAlliance uint32 = 0
	WGTeamHorde    uint32 = 1
	WGTeamNeutral  uint32 = 2

	// Wartime Ranks & Spells
	WGSpellRecruit                           uint32 = 37795
	WGSpellCorporal                          uint32 = 33280
	WGSpellLieutenant                        uint32 = 55629
	WGSpellTenacity                          uint32 = 58549
	WGSpellTenacityVehicle                   uint32 = 59911
	WGSpellTowerControl                      uint32 = 62064
	WGSpellSpiritualImmunity                 uint32 = 58729
	WGSpellGreatHonor                        uint32 = 58555
	WGSpellGreaterHonor                      uint32 = 58556
	WGSpellGreatestHonor                     uint32 = 58557
	WGSpellAllianceFlag                      uint32 = 14268
	WGSpellHordeFlag                         uint32 = 14267
	WGSpellGrabPassenger                     uint32 = 61178
	WGSpellVictoryReward                     uint32 = 56902
	WGSpellDefeatReward                      uint32 = 58494
	WGSpellEssenceOfWintergrasp              uint32 = 58045
	WGSpellAllianceControlPhaseShift         uint32 = 55774
	WGSpellHordeControlPhaseShift            uint32 = 55773
	WGSpellAllianceControlsFactoryPhaseShift uint32 = 56617
	WGSpellHordeControlsFactoryPhaseShift    uint32 = 56618
	WGSpellBuildCatapultForce                uint32 = 56663
	WGSpellBuildDemolisherForce              uint32 = 56575
	WGSpellBuildSiegeAllianceForce           uint32 = 56661
	WGSpellBuildSiegeHordeForce              uint32 = 61408

	// World States (SharedDefines.h WS_BATTLEFIELD_WG_* enum)
	WGWorldStateActive         uint32 = 3801
	WGWorldStateShowWorldState uint32 = 3710
	WGWorldStateDefender       uint32 = 3802
	WGWorldStateAttacker       uint32 = 3803
	WGWorldStateDefendedA      uint32 = 4025
	WGWorldStateDefendedH      uint32 = 4024
	WGWorldStateAttackedA      uint32 = 4023
	WGWorldStateAttackedH      uint32 = 4022
	WGWorldStateVehicleH       uint32 = 3490
	WGWorldStateMaxVehicleH    uint32 = 3491
	WGWorldStateVehicleA       uint32 = 3680
	WGWorldStateMaxVehicleA    uint32 = 3681
	WGWorldStateClock1         uint32 = 3781
	WGWorldStateClock2         uint32 = 4354

	// Workshop WorldStates
	WGWorldStateWorkshopNW       uint32 = 3700 // Broken Temple
	WGWorldStateWorkshopNE       uint32 = 3701 // Sunken Ring
	WGWorldStateWorkshopSE       uint32 = 3703 // Eastspark
	WGWorldStateWorkshopSW       uint32 = 3702 // Westspark
	WGWorldStateWorkshopKeepWest uint32 = 3698
	WGWorldStateWorkshopKeepEast uint32 = 3699

	// Workshop IDs
	WGWorkshopSE       uint8 = 0
	WGWorkshopSW       uint8 = 1
	WGWorkshopNE       uint8 = 2
	WGWorkshopNW       uint8 = 3
	WGWorkshopKeepWest uint8 = 4
	WGWorkshopKeepEast uint8 = 5
	WGMaxWorkshops           = 6

	// Tower WorldStates
	WGWorldStateTowerShadowsight uint32 = 3704 // West South Tower
	WGWorldStateTowerWintersEdge uint32 = 3705 // South South Tower
	WGWorldStateTowerFlamewatch  uint32 = 3706 // East South Tower
	WGWorldStateKeepTowerNW      uint32 = 3711
	WGWorldStateKeepTowerNE      uint32 = 3712
	WGWorldStateKeepTowerSW      uint32 = 3713
	WGWorldStateKeepTowerSE      uint32 = 3714

	// Tower IDs
	WGTowerKeepNW      uint8 = 0
	WGTowerKeepSW      uint8 = 1
	WGTowerKeepSE      uint8 = 2
	WGTowerKeepNE      uint8 = 3
	WGTowerShadowsight uint8 = 4
	WGTowerWintersEdge uint8 = 5
	WGTowerFlamewatch  uint8 = 6
	WGMaxTowers              = 7

	// Gate WorldStates
	WGWorldStateFortressGate uint32 = 3763
	WGWorldStateVaultGate    uint32 = 3773

	// GameObjects
	WGGameObjectTitanRelic        uint32 = 192829
	WGGameObjectVaultGate         uint32 = 191810
	WGGameObjectFortressGate      uint32 = 190375
	WGGameObjectKeepCollisionWall uint32 = 194323

	// Banners
	WGGameObjectBannerNE uint32 = 190475 // Sunken Ring
	WGGameObjectBannerNW uint32 = 190487 // Broken Temple
	WGGameObjectBannerSE uint32 = 194959 // Eastspark
	WGGameObjectBannerSW uint32 = 194962 // Westspark

	// Towers
	WGGameObjectKeepTower1       uint32 = 190221 // NW
	WGGameObjectKeepTower2       uint32 = 190373 // SW
	WGGameObjectKeepTower3       uint32 = 190377 // SE
	WGGameObjectKeepTower4       uint32 = 190378 // NE
	WGGameObjectTowerShadowsight uint32 = 190356 // West
	WGGameObjectTowerWintersEdge uint32 = 190357 // South
	WGGameObjectTowerFlamewatch  uint32 = 190358 // East

	// Vehicles and NPCs
	WGCreatureSiegeEngineAlliance uint32 = 28312
	WGCreatureSiegeEngineHorde    uint32 = 32627
	WGCreatureCatapult            uint32 = 27881
	WGCreatureDemolisher          uint32 = 28094
	WGCreatureTowerCannon         uint32 = 28366
	WGCreatureGoblinMechanic      uint32 = 30400
	WGCreatureGnomishEngineer     uint32 = 30499
	WGCreatureSpiritGuideAlliance uint32 = 31842
	WGCreatureSpiritGuideHorde    uint32 = 31841

	// Achievements & Quests
	WGAchievementWinWG           uint32 = 1717
	WGAchievementWinWGTimer10    uint32 = 1755
	WGAchievementTowerDestroy    uint32 = 1727
	WGQuestVictoryAlliance       uint32 = 13181
	WGQuestVictoryHorde          uint32 = 13183
	WGQuestCreditTowersDestroyed uint32 = 35074
	WGQuestCreditDefendSiege     uint32 = 31284

	// Building Destructible States (TrinityCore WintergraspGameObjectState)
	WGBuildingStateNone              uint8 = 0
	WGBuildingStateNeutralIntact     uint8 = 1
	WGBuildingStateNeutralDamaged    uint8 = 2
	WGBuildingStateNeutralDestroyed  uint8 = 3
	WGBuildingStateHordeIntact       uint8 = 4
	WGBuildingStateHordeDamaged      uint8 = 5
	WGBuildingStateHordeDestroyed    uint8 = 6
	WGBuildingStateAllianceIntact    uint8 = 7
	WGBuildingStateAllianceDamaged   uint8 = 8
	WGBuildingStateAllianceDestroyed uint8 = 9
)

// Building state calculation formula matching TrinityCore BfWGGameObjectBuilding:
// Intact:    ALLIANCE_INTACT - (teamControl * 3) -> Alliance: 7, Horde: 4
// Damaged:   ALLIANCE_DAMAGE - (teamControl * 3) -> Alliance: 8, Horde: 5
// Destroyed: ALLIANCE_DESTROY - (teamControl * 3) -> Alliance: 9, Horde: 6
func wgBuildingState(state uint8, teamControl uint32) uint8 {
	if teamControl == WGTeamNeutral {
		switch state {
		case 1:
			return WGBuildingStateNeutralDamaged
		case 2:
			return WGBuildingStateNeutralDestroyed
		default:
			return WGBuildingStateNeutralIntact
		}
	}
	base := WGBuildingStateAllianceIntact
	switch state {
	case 1:
		base = WGBuildingStateAllianceDamaged
	case 2:
		base = WGBuildingStateAllianceDestroyed
	}
	return base - uint8(teamControl*3)
}

// Workshop definition
type wgWorkshopState struct {
	ID          uint8
	WorldState  uint32
	BannerEntry uint32
	TeamControl uint32 // WGTeamAlliance, WGTeamHorde, WGTeamNeutral
	Name        string
}

// Tower definition
type wgTowerState struct {
	ID           uint8
	Entry        uint32
	WorldState   uint32
	TeamControl  uint32
	State        uint8 // 0: Intact, 1: Damaged, 2: Destroyed
	IsSouthTower bool
	Name         string
}

// Gate definition
type wgGateState struct {
	Entry      uint32
	WorldState uint32
	State      uint8 // 0: Intact, 1: Damaged, 2: Destroyed
	Name       string
}

// wgWallState mirrors the BATTLEFIELD_WG_OBJECTTYPE_WALL entries of
// WGGameObjectBuilding (BattlefieldWG.cpp:88-108): the 20 keep walls plus the
// 3 walls with passage, in spawn-array order. Walls are defender-team buildings
// with no tower warning texts; C++ emits their worldstates in the
// BuildingsInZone block of FillInitialWorldStates ahead of the towers.
type wgWallState struct {
	Entry      uint32
	WorldState uint32
	State      uint8 // 0: Intact, 1: Damaged, 2: Destroyed
}

var wgWallBuildings = [][2]uint32{
	{190219, 3749}, {190220, 3750}, {191795, 3764}, {191796, 3772},
	{191799, 3762}, {191800, 3766}, {191801, 3770}, {191802, 3751},
	{191803, 3752}, {191804, 3767}, {191806, 3769}, {191807, 3759},
	{191808, 3760}, {191809, 3761}, {190369, 3753}, {190370, 3758},
	{190371, 3754}, {190372, 3757}, {190374, 3755}, {190376, 3756},
	{191797, 3765}, {191798, 3771}, {191805, 3768},
}

// Wintergrasp battlefield state
type wgBattlegroundState struct {
	mu sync.Mutex

	MapID         uint32
	ZoneID        uint32
	IsActive      bool
	DefenderTeam  uint32 // WGTeamAlliance or WGTeamHorde
	Timer         time.Duration
	TotalDuration time.Duration
	EndTime       time.Time
	// Enabled mirrors Battlefield::m_IsEnabled (Battlefield.cpp:42, default
	// true); toggled by the .bf enable command (cs_bf.cpp
	// HandleBattlefieldEnable / ToggleBattlefield).
	Enabled bool

	Workshops         [WGMaxWorkshops]wgWorkshopState
	Towers            [WGMaxTowers]wgTowerState
	Gates             [2]wgGateState // 0: Fortress Gate, 1: Vault Gate
	Walls             []wgWallState
	RelicInteractible bool
	BrokenSouthTowers uint32

	VehiclesAlliance    uint32
	VehiclesHorde       uint32
	MaxVehiclesAlliance uint32
	MaxVehiclesHorde    uint32

	TenacityTeam  int8 // -1: neutral, 0: Alliance, 1: Horde
	TenacityStack int32

	PlayersInWar      map[uint64]uint32 // playerGUID -> team
	PlayersInQueue    map[uint64]uint32 // playerGUID -> team; mirrors Battlefield::m_PlayersInQueue[team] (Battlefield.h)
	PlayerRanks       map[uint64]uint32 // playerGUID -> rank spell ID (Recruit, Corporal, Lieutenant)
	PlayerKillsInRank map[uint64]uint32 // playerGUID -> kills in current rank towards next promotion (0..5)
	Vehicles          map[uint64]uint32 // vehicleGUID -> team

	// ReviveQueue is the Wintergrasp spirit-healer revive queue
	// (playerGUID -> spirit-guide GUID). C++ fans the queue out per
	// graveyard (BfGraveyard::m_ResurrectQueue, Battlefield.cpp:605-627);
	// Go keeps the single battlefield-level queue since it models no
	// graveyard list.
	ReviveQueue map[uint64]uint64
	// LastResurrectTimer mirrors Battlefield::m_LastResurrectTimer
	// (Battlefield.cpp:62,191-199): time until the next 30-second
	// resurrection wave; answered to CMSG_AREA_SPIRIT_HEALER_QUERY.
	LastResurrectTimer time.Duration

	StatsWonA uint32
	StatsDefA uint32
	StatsWonH uint32
	StatsDefH uint32
	Winner    int8 // -1: ongoing, 0: Alliance, 1: Horde
}

// AttackerTeam returns the opposing team from DefenderTeam.
func (wg *wgBattlegroundState) AttackerTeam() uint32 {
	if wg.DefenderTeam == WGTeamAlliance {
		return WGTeamHorde
	}
	return WGTeamAlliance
}

// initWorkshops sets up the 6 Wintergrasp workshops.
func (wg *wgBattlegroundState) initWorkshops() {
	attacker := wg.AttackerTeam()
	defender := wg.DefenderTeam

	wg.Workshops = [WGMaxWorkshops]wgWorkshopState{
		{ID: WGWorkshopSE, WorldState: WGWorldStateWorkshopSE, BannerEntry: WGGameObjectBannerSE, TeamControl: attacker, Name: "Eastspark Workshop"},
		{ID: WGWorkshopSW, WorldState: WGWorldStateWorkshopSW, BannerEntry: WGGameObjectBannerSW, TeamControl: attacker, Name: "Westspark Workshop"},
		{ID: WGWorkshopNE, WorldState: WGWorldStateWorkshopNE, BannerEntry: WGGameObjectBannerNE, TeamControl: defender, Name: "Sunken Ring Workshop"},
		{ID: WGWorkshopNW, WorldState: WGWorldStateWorkshopNW, BannerEntry: WGGameObjectBannerNW, TeamControl: defender, Name: "Broken Temple Workshop"},
		{ID: WGWorkshopKeepWest, WorldState: WGWorldStateWorkshopKeepWest, BannerEntry: 0, TeamControl: defender, Name: "Keep West Workshop"},
		{ID: WGWorkshopKeepEast, WorldState: WGWorldStateWorkshopKeepEast, BannerEntry: 0, TeamControl: defender, Name: "Keep East Workshop"},
	}
}

// initTowers sets up Keep and South towers.
func (wg *wgBattlegroundState) initTowers() {
	attacker := wg.AttackerTeam()
	defender := wg.DefenderTeam

	wg.Towers = [WGMaxTowers]wgTowerState{
		{ID: WGTowerKeepNW, Entry: WGGameObjectKeepTower1, WorldState: WGWorldStateKeepTowerNW, TeamControl: defender, State: 0, IsSouthTower: false, Name: "Northwest Keep Tower"},
		{ID: WGTowerKeepSW, Entry: WGGameObjectKeepTower2, WorldState: WGWorldStateKeepTowerSW, TeamControl: defender, State: 0, IsSouthTower: false, Name: "Southwest Keep Tower"},
		{ID: WGTowerKeepSE, Entry: WGGameObjectKeepTower3, WorldState: WGWorldStateKeepTowerSE, TeamControl: defender, State: 0, IsSouthTower: false, Name: "Southeast Keep Tower"},
		{ID: WGTowerKeepNE, Entry: WGGameObjectKeepTower4, WorldState: WGWorldStateKeepTowerNE, TeamControl: defender, State: 0, IsSouthTower: false, Name: "Northeast Keep Tower"},
		{ID: WGTowerShadowsight, Entry: WGGameObjectTowerShadowsight, WorldState: WGWorldStateTowerShadowsight, TeamControl: attacker, State: 0, IsSouthTower: true, Name: "Shadowsight Tower"},
		{ID: WGTowerWintersEdge, Entry: WGGameObjectTowerWintersEdge, WorldState: WGWorldStateTowerWintersEdge, TeamControl: attacker, State: 0, IsSouthTower: true, Name: "Winter's Edge Tower"},
		{ID: WGTowerFlamewatch, Entry: WGGameObjectTowerFlamewatch, WorldState: WGWorldStateTowerFlamewatch, TeamControl: attacker, State: 0, IsSouthTower: true, Name: "Flamewatch Tower"},
	}
}

// initGates sets up Fortress and Vault gates.
func (wg *wgBattlegroundState) initGates() {
	wg.Gates = [2]wgGateState{
		{Entry: WGGameObjectFortressGate, WorldState: WGWorldStateFortressGate, State: 0, Name: "Wintergrasp Fortress Gate"},
		{Entry: WGGameObjectVaultGate, WorldState: WGWorldStateVaultGate, State: 0, Name: "Vault of Archavon Gate"},
	}
	wg.Walls = make([]wgWallState, len(wgWallBuildings))
	for i, b := range wgWallBuildings {
		wg.Walls[i] = wgWallState{Entry: b[0], WorldState: b[1], State: 0}
	}
	wg.RelicInteractible = false
	wg.BrokenSouthTowers = 0
}

// updateVehicleLimits recalculates MaxVehiclesAlliance and MaxVehiclesHorde based on workshop control.
// Reference: BattlefieldWG::UpdateCounterVehicle (BattlefieldWG.cpp:630-650).
func (wg *wgBattlegroundState) updateVehicleLimits() {
	wg.MaxVehiclesAlliance = 0
	wg.MaxVehiclesHorde = 0
	for _, ws := range wg.Workshops {
		if ws.TeamControl == WGTeamAlliance {
			wg.MaxVehiclesAlliance += 4
		} else if ws.TeamControl == WGTeamHorde {
			wg.MaxVehiclesHorde += 4
		}
	}
}

// StartBattle initializes a wartime battle.
// Reference: BattlefieldWG::OnBattleStart (BattlefieldWG.cpp:567-629).
func (s *Server) StartWGBattle(defenderTeam uint32) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	wg.IsActive = true
	wg.DefenderTeam = defenderTeam
	wg.Timer = WGBattleDuration
	wg.TotalDuration = WGBattleDuration
	wg.EndTime = time.Now().Add(WGBattleDuration)
	wg.Winner = -1

	wg.initWorkshops()
	wg.initTowers()
	wg.initGates()
	wg.updateVehicleLimits()

	wg.VehiclesAlliance = 0
	wg.VehiclesHorde = 0
	wg.Vehicles = make(map[uint64]uint32)
	wg.PlayerRanks = make(map[uint64]uint32)
	wg.PlayerKillsInRank = make(map[uint64]uint32)

	s.broadcastWGInitWorldStates()
	s.broadcastBattlegroundMessage(wg.MapID, "The battle for Wintergrasp has begun!")
}

// EndBattle completes a wartime battle.
// Reference: BattlefieldWG::OnBattleEnd (BattlefieldWG.cpp:651-760).
func (s *Server) EndWGBattle(endByTimer bool) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if !wg.IsActive {
		return
	}

	var winningTeam uint32
	var losingTeam uint32

	if endByTimer {
		// Defenders successfully held the fortress
		winningTeam = wg.DefenderTeam
		losingTeam = wg.AttackerTeam()
		if winningTeam == WGTeamAlliance {
			wg.StatsDefA++
		} else {
			wg.StatsDefH++
		}
		wg.Winner = int8(winningTeam)
	} else {
		// Attackers breached the vault and activated the relic!
		winningTeam = wg.AttackerTeam()
		losingTeam = wg.DefenderTeam
		if winningTeam == WGTeamAlliance {
			wg.StatsWonA++
		} else {
			wg.StatsWonH++
		}
		// Attacking team becomes the new defender team!
		wg.DefenderTeam = winningTeam
		wg.Winner = int8(winningTeam)
	}

	wg.IsActive = false
	// OnBattleEnd reads the remaining battle timer before it is reset
	// (DoCompleteOrIncrementAchievement(ACHIEVEMENTS_WIN_WG_TIMER_10) when
	// !endByTimer && GetTimer() <= 10000); capture it here.
	remainingBattleTime := wg.Timer
	wg.Timer = WGNoWarDuration
	wg.TotalDuration = WGNoWarDuration
	wg.EndTime = time.Now().Add(WGNoWarDuration)

	// Reward players
	// Reference: BattlefieldWG::OnBattleEnd (BattlefieldWG.cpp:651-760).
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		pGUID := sess.playerGUID
		pTeam, inWar := wg.PlayersInWar[pGUID]
		if !inWar {
			pTeam = teamForRace(sess.player.Race)
		}

		// Remove wartime auras (BattlefieldWG::RemoveAurasFromPlayer,
		// BattlefieldWG.cpp:1049-1059)
		sess.removeAura(WGSpellRecruit)
		sess.removeAura(WGSpellCorporal)
		sess.removeAura(WGSpellLieutenant)
		sess.removeAura(WGSpellTenacity)
		sess.removeAura(WGSpellTowerControl)
		sess.removeAura(WGSpellSpiritualImmunity)
		sess.removeAura(WGSpellGreatHonor)
		sess.removeAura(WGSpellGreaterHonor)
		sess.removeAura(WGSpellGreatestHonor)

		if pTeam == winningTeam {
			sess.applyAura(WGSpellEssenceOfWintergrasp)
			sess.applyAura(WGSpellVictoryReward)
			if inWar {
				sess.completeAchievement(WGAchievementWinWG)
				if !endByTimer && remainingBattleTime <= 10*time.Second {
					sess.completeAchievement(WGAchievementWinWGTimer10)
				}
				// Complete victory quests (Player::AreaExploredOrEventHappens,
				// BattlefieldWG.cpp:719-720)
				for _, questID := range [2]uint32{WGQuestVictoryAlliance, WGQuestVictoryHorde} {
					if status, _ := sess.characterQuestStatus(context.Background(), questID); status == questStatusIncomplete {
						sess.sendQuestComplete(questID)
						sess.completeQuest(context.Background(), questID)
					}
				}
			}
			if pTeam == WGTeamAlliance {
				sess.applyAura(WGSpellAllianceControlPhaseShift)
				sess.removeAura(WGSpellHordeControlPhaseShift)
			} else {
				sess.applyAura(WGSpellHordeControlPhaseShift)
				sess.removeAura(WGSpellAllianceControlPhaseShift)
			}
		} else if pTeam == losingTeam {
			sess.removeAura(WGSpellEssenceOfWintergrasp)
			sess.applyAura(WGSpellDefeatReward)
			if !endByTimer {
				// On a relic capture every player swaps to the new defender's
				// control phase shift (BattlefieldWG.cpp:747-757); on a
				// successful defense the shifts are untouched.
				if winningTeam == WGTeamAlliance {
					sess.removeAura(WGSpellHordeControlPhaseShift)
					sess.applyAura(WGSpellAllianceControlPhaseShift)
				} else {
					sess.removeAura(WGSpellAllianceControlPhaseShift)
					sess.applyAura(WGSpellHordeControlPhaseShift)
				}
			}
		}
	}
	s.sessionsMu.RUnlock()

	// Despawn vehicles
	wg.VehiclesAlliance = 0
	wg.VehiclesHorde = 0
	wg.Vehicles = make(map[uint64]uint32)

	s.broadcastWGInitWorldStates()
	if endByTimer {
		teamName := "Alliance"
		if winningTeam == WGTeamHorde {
			teamName = "Horde"
		}
		s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("The %s has successfully defended Wintergrasp Fortress!", teamName))
	} else {
		teamName := "Alliance"
		if winningTeam == WGTeamHorde {
			teamName = "Horde"
		}
		s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("The %s has captured Wintergrasp Fortress!", teamName))
	}
}

// CanBuildVehicle validates whether a player can construct the requested vehicle.
// Reference: zone_wintergrasp.cpp:134-194 (npc_wg_demolisher_engineerAI::CanBuild / OnGossipHello).
func (wg *wgBattlegroundState) CanBuildVehicle(playerTeam uint32, rank uint32, vehicleEntry uint32) (bool, string) {
	if !wg.IsActive {
		return false, "Vehicles can only be constructed during wartime."
	}

	// Rank check
	switch vehicleEntry {
	case WGCreatureCatapult:
		if rank != WGSpellCorporal && rank != WGSpellLieutenant {
			return false, "Requires rank of Corporal or higher."
		}
	case WGCreatureDemolisher, WGCreatureSiegeEngineAlliance, WGCreatureSiegeEngineHorde:
		if rank != WGSpellLieutenant {
			return false, "Requires rank of First Lieutenant."
		}
	default:
		return false, "Unknown vehicle type."
	}

	// Capacity check
	if playerTeam == WGTeamAlliance {
		if wg.VehiclesAlliance >= wg.MaxVehiclesAlliance {
			return false, "Alliance vehicle limit reached."
		}
	} else {
		if wg.VehiclesHorde >= wg.MaxVehiclesHorde {
			return false, "Horde vehicle limit reached."
		}
	}

	return true, ""
}

// SpawnVehicle tracks a created vehicle.
// Reference: BattlefieldWG::OnCreatureCreate (BattlefieldWG.cpp:837-891).
func (s *Server) SpawnWGVehicle(vehicleGUID uint64, vehicleEntry uint32, playerTeam uint32) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if playerTeam == WGTeamAlliance {
		if wg.VehiclesAlliance >= wg.MaxVehiclesAlliance {
			return false
		}
		wg.VehiclesAlliance++
		wg.Vehicles[vehicleGUID] = playerTeam
		s.updateWGWorldState(WGWorldStateVehicleA, wg.VehiclesAlliance)
	} else {
		if wg.VehiclesHorde >= wg.MaxVehiclesHorde {
			return false
		}
		wg.VehiclesHorde++
		wg.Vehicles[vehicleGUID] = playerTeam
		s.updateWGWorldState(WGWorldStateVehicleH, wg.VehiclesHorde)
	}
	return true
}

// DestroyVehicle unregisters a destroyed or despawned vehicle.
// Reference: BattlefieldWG::OnUnitDeath / FindAndRemoveVehicleFromList (BattlefieldWG.cpp:980-1005).
func (s *Server) DestroyWGVehicle(vehicleGUID uint64) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	team, exists := wg.Vehicles[vehicleGUID]
	if !exists {
		return false
	}
	delete(wg.Vehicles, vehicleGUID)

	if team == WGTeamAlliance {
		if wg.VehiclesAlliance > 0 {
			wg.VehiclesAlliance--
		}
		s.updateWGWorldState(WGWorldStateVehicleA, wg.VehiclesAlliance)
	} else {
		if wg.VehiclesHorde > 0 {
			wg.VehiclesHorde--
		}
		s.updateWGWorldState(WGWorldStateVehicleH, wg.VehiclesHorde)
	}
	return true
}

// HandlePromotion updates player rank progression on honorable kills.
// Reference: BattlefieldWG::PromotePlayer / HandlePromotion (BattlefieldWG.cpp:1007-1047).
func (wg *wgBattlegroundState) HandlePromotion(s *Server, killerGUID uint64, killerTeam uint32) {
	if !wg.IsActive {
		return
	}

	currentRank, ok := wg.PlayerRanks[killerGUID]
	if !ok || currentRank == 0 {
		currentRank = WGSpellRecruit
		wg.PlayerRanks[killerGUID] = currentRank
	}

	switch currentRank {
	case WGSpellRecruit:
		wg.PlayerKillsInRank[killerGUID]++
		if wg.PlayerKillsInRank[killerGUID] >= 5 {
			wg.PlayerRanks[killerGUID] = WGSpellCorporal
			wg.PlayerKillsInRank[killerGUID] = 0
			if sess := s.findSessionByGUID(killerGUID); sess != nil {
				sess.removeAura(WGSpellRecruit)
				sess.applyAura(WGSpellCorporal)
			}
		}
	case WGSpellCorporal:
		wg.PlayerKillsInRank[killerGUID]++
		if wg.PlayerKillsInRank[killerGUID] >= 5 {
			wg.PlayerRanks[killerGUID] = WGSpellLieutenant
			wg.PlayerKillsInRank[killerGUID] = 0
			if sess := s.findSessionByGUID(killerGUID); sess != nil {
				sess.removeAura(WGSpellCorporal)
				sess.applyAura(WGSpellLieutenant)
			}
		}
	case WGSpellLieutenant:
		// Max rank achieved
	}
}

// DamageBuilding transitions a building or tower to damaged state.
// Reference: BfWGGameObjectBuilding::Damaged (BattlefieldWG.cpp:1453-1475).
func (s *Server) DamageWGBuilding(entry uint32) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	// Reference: BfWGGameObjectBuilding::Damaged (BattlefieldWG.cpp:1453-1475):
	// walls are defender-team buildings with no tower warning texts.
	for i := range wg.Walls {
		if wg.Walls[i].Entry == entry {
			if wg.Walls[i].State == 0 {
				wg.Walls[i].State = 1 // Damaged
				s.updateWGWorldState(wg.Walls[i].WorldState, uint32(wgBuildingState(1, wg.DefenderTeam)))
				return true
			}
			return false
		}
	}

	for i := range wg.Towers {
		if wg.Towers[i].Entry == entry {
			if wg.Towers[i].State == 0 {
				wg.Towers[i].State = 1 // Damaged
				s.updateWGWorldState(wg.Towers[i].WorldState, uint32(wgBuildingState(1, wg.Towers[i].TeamControl)))
				s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("%s is under attack!", wg.Towers[i].Name))
				return true
			}
			return false
		}
	}

	for i := range wg.Gates {
		if wg.Gates[i].Entry == entry {
			if wg.Gates[i].State == 0 {
				wg.Gates[i].State = 1
				s.updateWGWorldState(wg.Gates[i].WorldState, uint32(wgBuildingState(1, wg.DefenderTeam)))
				s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("%s is damaged!", wg.Gates[i].Name))
				return true
			}
			return false
		}
	}
	return false
}

// DestroyBuilding transitions a building or tower to destroyed state.
// If all 3 south towers are destroyed, 10 minutes are removed from the battle timer.
// If the Vault Gate is destroyed, the Titan Relic becomes interactible!
// Reference: BfWGGameObjectBuilding::Destroyed / UpdatedDestroyedTowerCount (BattlefieldWG.cpp:1204-1245, 1477-1509).
func (s *Server) DestroyWGBuilding(entry uint32) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	// Reference: BfWGGameObjectBuilding::Destroyed (BattlefieldWG.cpp:1477-1509):
	// walls are defender-team buildings with no tower warning texts and no
	// tower counters.
	for i := range wg.Walls {
		if wg.Walls[i].Entry == entry {
			if wg.Walls[i].State != 2 {
				wg.Walls[i].State = 2 // Destroyed
				s.updateWGWorldState(wg.Walls[i].WorldState, uint32(wgBuildingState(2, wg.DefenderTeam)))
				return true
			}
			return false
		}
	}

	for i := range wg.Towers {
		if wg.Towers[i].Entry == entry {
			if wg.Towers[i].State != 2 {
				wg.Towers[i].State = 2 // Destroyed
				s.updateWGWorldState(wg.Towers[i].WorldState, uint32(wgBuildingState(2, wg.Towers[i].TeamControl)))
				s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("%s has been destroyed!", wg.Towers[i].Name))

				if wg.Towers[i].IsSouthTower {
					wg.BrokenSouthTowers++

					// Update tower control buffs
					s.sessionsMu.RLock()
					for sess := range s.sessions {
						if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != wg.MapID {
							continue
						}
						pTeam := teamForRace(sess.player.Race)
						if pTeam == wg.DefenderTeam {
							sess.applyAura(WGSpellTowerControl)
						} else {
							sess.removeAura(WGSpellTowerControl)
						}
					}
					s.sessionsMu.RUnlock()

					// Tower-destroy achievement and quest credit to defenders in war.
					// Reference: BattlefieldWG::UpdatedDestroyedTowerCount
					// (BattlefieldWG.cpp:1204-1245): defenders gain a Tower Control
					// stack, KilledMonsterCredit(QUEST_CREDIT_TOWERS_DESTROYED)
					// and the tower-destroy achievement.
					for guid, team := range wg.PlayersInWar {
						if team != wg.DefenderTeam {
							continue
						}
						if sess := s.findSessionByGUID(guid); sess != nil {
							sess.completeAchievement(WGAchievementTowerDestroy)
							sess.creditQuestKills(context.Background(), WGQuestCreditTowersDestroyed, 0)
						}
					}

					// Penalty: If all 3 south towers are destroyed, subtract 10 minutes from battle timer!
					if wg.BrokenSouthTowers == 3 {
						if wg.Timer > WGSouthTowerTimePenalty {
							wg.Timer -= WGSouthTowerTimePenalty
						} else {
							wg.Timer = 0
						}
						wg.EndTime = time.Now().Add(wg.Timer)
						s.broadcastBattlegroundMessage(wg.MapID, "All southern towers destroyed! Battle time reduced by 10 minutes!")
						s.broadcastWGInitWorldStates()
					}
				}
				return true
			}
			return false
		}
	}

	for i := range wg.Gates {
		if wg.Gates[i].Entry == entry {
			if wg.Gates[i].State != 2 {
				wg.Gates[i].State = 2
				s.updateWGWorldState(wg.Gates[i].WorldState, uint32(wgBuildingState(2, wg.DefenderTeam)))
				s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("%s has been breached!", wg.Gates[i].Name))

				// If Vault Gate breached, unlock Titan's Relic!
				if entry == WGGameObjectVaultGate {
					wg.RelicInteractible = true
					s.broadcastBattlegroundMessage(wg.MapID, "The Chamber of the Titan Relic has been breached!")
				}
				return true
			}
			return false
		}
	}
	return false
}

// CaptureWorkshop transfers control of a workshop to a new faction.
// Reference: WintergraspWorkshop::GiveControlTo (BattlefieldWG.cpp:1778-1828).
func (s *Server) CaptureWGWorkshop(workshopID uint8, team uint32, capturerName string) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if int(workshopID) >= len(wg.Workshops) {
		return false
	}
	// Keep workshops cannot be captured
	if workshopID == WGWorkshopKeepWest || workshopID == WGWorkshopKeepEast {
		return false
	}

	ws := &wg.Workshops[workshopID]
	if ws.TeamControl == team {
		return true // already controlled
	}
	ws.TeamControl = team
	s.updateWGWorldState(ws.WorldState, uint32(wgBuildingState(0, team)))
	wg.updateVehicleLimits()
	s.updateWGWorldState(WGWorldStateMaxVehicleA, wg.MaxVehiclesAlliance)
	s.updateWGWorldState(WGWorldStateMaxVehicleH, wg.MaxVehiclesHorde)

	teamName := "Alliance"
	if team == WGTeamHorde {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(wg.MapID, fmt.Sprintf("%s has captured %s for the %s!", capturerName, ws.Name, teamName))
	return true
}

// UpdateTenacity computes and applies tenacity buff stacks based on team balance.
// Reference: BattlefieldWG::UpdateTenacity (BattlefieldWG.cpp:1301-1371).
func (s *Server) UpdateWGTenacity() {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	var allyCount, hordeCount uint32
	for _, team := range wg.PlayersInWar {
		if team == WGTeamAlliance {
			allyCount++
		} else if team == WGTeamHorde {
			hordeCount++
		}
	}

	var newStack int32
	if allyCount > 0 && hordeCount > 0 {
		// C++ divides the integer counts FIRST (uint32 division), then casts
		// to float: (float(hordePlayers / alliancePlayers) - 1) * 4.
		if allyCount < hordeCount {
			newStack = int32((float32(hordeCount/allyCount) - 1.0) * 4)
		} else if hordeCount < allyCount {
			newStack = int32((1.0 - float32(allyCount/hordeCount)) * 4.0)
		}
	}

	// Reference: BattlefieldWG::UpdateTenacity (BattlefieldWG.cpp:1301-1312):
	// the stored stack is the raw uncapped value; only the applied aura is
	// capped at 20.
	if newStack == wg.TenacityStack {
		return
	}
	wg.TenacityStack = newStack

	var teamToBuff int8 = -1
	absStack := newStack
	if absStack < 0 {
		absStack = -absStack
	}
	if newStack > 0 {
		teamToBuff = 0 // Alliance
	} else if newStack < 0 {
		teamToBuff = 1 // Horde
	}
	if absStack > 20 {
		absStack = 20
	}

	wg.TenacityTeam = teamToBuff

	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != wg.MapID {
			continue
		}
		pTeam := teamForRace(sess.player.Race)
		if teamToBuff >= 0 && uint32(teamToBuff) == pTeam {
			sess.applyAura(WGSpellTenacity)
			if absStack >= 15 {
				sess.applyAura(WGSpellGreatestHonor)
			} else if absStack >= 10 {
				sess.applyAura(WGSpellGreaterHonor)
			} else if absStack >= 5 {
				sess.applyAura(WGSpellGreatHonor)
			}
		} else {
			sess.removeAura(WGSpellTenacity)
			sess.removeAura(WGSpellGreatHonor)
			sess.removeAura(WGSpellGreaterHonor)
			sess.removeAura(WGSpellGreatestHonor)
		}
	}
	s.sessionsMu.RUnlock()
}

// TickWG processes periodic Wintergrasp timer countdown and state transitions.
// Reference: BattlefieldMgr::Update (BattlefieldMgr.cpp:155-164) — disabled
// battlefields are skipped entirely.
func (s *Server) TickWG(ctx context.Context, delta time.Duration) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	if !wg.Enabled {
		wg.mu.Unlock()
		return
	}
	// Battlefield::Update (Battlefield.cpp:191-199): the spirit-healer
	// resurrect timer ticks on every update, war or peace; each
	// RESURRECTION_INTERVAL every graveyard resurrects its queue.
	var queued map[uint64]uint64
	wg.LastResurrectTimer -= delta
	if wg.LastResurrectTimer <= 0 {
		wg.LastResurrectTimer = wgResurrectInterval
		if len(wg.ReviveQueue) != 0 {
			queued = wg.ReviveQueue
			wg.ReviveQueue = make(map[uint64]uint64)
		}
	}
	if wg.Timer > delta {
		wg.Timer -= delta
		wg.mu.Unlock()
	} else {
		wg.Timer = 0
		isActive := wg.IsActive
		defenderTeam := wg.DefenderTeam
		wg.mu.Unlock()

		if isActive {
			// Wartime timer expired -> Defenders win!
			s.EndWGBattle(true)
		} else {
			// Peace timer expired -> Start war!
			s.StartWGBattle(defenderTeam)
		}
	}
	if len(queued) != 0 {
		s.resurrectWGQueuedPlayers(ctx, queued)
	}
}

// inWintergraspBattlefield mirrors BattlefieldMgr::GetBattlefieldToZoneId
// (BattlefieldMgr.cpp:121-133): the battlefield answers the spirit-healer
// opcodes for the player's zone id, and only while enabled.
func (s *session) inWintergraspBattlefield() bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	if s.player.Zone != WGZoneID {
		return false
	}
	wg := s.server.getOrCreateWGState()
	wg.mu.Lock()
	enabled := wg.Enabled
	wg.mu.Unlock()
	return enabled
}

// queueWGSpiritRevive mirrors Battlefield::AddPlayerToResurrectQueue
// (Battlefield.cpp:605-616): the ghost joins the Wintergrasp revive queue.
// It reports whether the insert was fresh — BfGraveyard::AddPlayer
// (Battlefield.cpp:618-627) casts SPELL_WAITING_FOR_RESURRECT only then.
func (s *Server) queueWGSpiritRevive(playerGUID, spiritGUID uint64) bool {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	if wg.ReviveQueue == nil {
		wg.ReviveQueue = make(map[uint64]uint64)
	}
	_, dup := wg.ReviveQueue[playerGUID]
	wg.ReviveQueue[playerGUID] = spiritGUID
	wg.mu.Unlock()
	return !dup
}

// wgSpiritHealerTimeLeftMs mirrors Battlefield::SendAreaSpiritHealerQueryOpcode
// (Battlefield.cpp:635-641): SMSG_AREA_SPIRIT_HEALER_TIME carries the spirit
// guide's GUID and m_LastResurrectTimer, the milliseconds until the next
// 30-second wave.
func (s *Server) wgSpiritHealerTimeLeftMs() uint32 {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	left := wg.LastResurrectTimer
	wg.mu.Unlock()
	if left < 0 {
		left = 0
	}
	return uint32(left / time.Millisecond)
}

// resurrectWGQueuedPlayers mirrors BfGraveyard::Resurrect
// (Battlefield.cpp:697-723): the queued ghosts gain the resurrection visual,
// ResurrectPlayer(1.0), 6962 + SPELL_SPIRIT_HEAL_MANA (44535), and their
// corpses become bones. C++ also has the graveyard's spirit guide cast
// SPELL_SPIRIT_HEAL (22012) on itself; Go omits that cast — the guide
// creature is not resolved — following the BG wave-resurrect convention.
// The no-longer-a-ghost skip is Go-original: C++ has no such check, but the
// queue only ever holds ghosts.
func (s *Server) resurrectWGQueuedPlayers(ctx context.Context, queued map[uint64]uint64) {
	for pGUID := range queued {
		sess := s.findSessionByGUID(pGUID)
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.PlayerFlags&playerFlagGhost == 0 {
			continue
		}
		sess.applyAura(24171) // SPELL_RESURRECTION_VISUAL
		sess.resurrectPlayer(ctx, 1.0)
		sess.applyAura(6962)
		sess.applyAura(44535) // SPELL_SPIRIT_HEAL_MANA
		sess.removeAura(2584) // SPELL_WAITING_FOR_RESURRECT
		sess.spawnCorpseBones(ctx)
		sess.debug("wintergrasp wave resurrected ghost", "account", sess.accountName, "guid", pGUID)
	}
}

// handleWGGameObjectUse dispatches interaction with Wintergrasp objects.
// Reference: BattlefieldWG::ProcessEvent (BattlefieldWG.cpp:1247-1281).
func (s *Server) handleWGGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return true
	}
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	playerTeam := teamForRace(sess.player.Race)

	switch entry {
	case WGGameObjectTitanRelic:
		// Attackers clicking Titan Relic wins the battle!
		if !wg.RelicInteractible || !wg.IsActive {
			return true
		}
		if playerTeam != wg.AttackerTeam() {
			return true
		}
		wg.mu.Unlock()
		s.EndWGBattle(false)
		wg.mu.Lock()
		return true

	case WGGameObjectBannerNE:
		wg.mu.Unlock()
		s.CaptureWGWorkshop(WGWorkshopNE, playerTeam, sess.player.Name)
		wg.mu.Lock()
		return true

	case WGGameObjectBannerNW:
		wg.mu.Unlock()
		s.CaptureWGWorkshop(WGWorkshopNW, playerTeam, sess.player.Name)
		wg.mu.Lock()
		return true

	case WGGameObjectBannerSE:
		wg.mu.Unlock()
		s.CaptureWGWorkshop(WGWorkshopSE, playerTeam, sess.player.Name)
		wg.mu.Lock()
		return true

	case WGGameObjectBannerSW:
		wg.mu.Unlock()
		s.CaptureWGWorkshop(WGWorkshopSW, playerTeam, sess.player.Name)
		wg.mu.Lock()
		return true
	}

	return true
}

// handleWGPlayerEnter processes a player entering Wintergrasp.
// Reference: BattlefieldWG::OnPlayerEnterZone / OnPlayerJoinWar (BattlefieldWG.cpp:1061-1094, 1124-1133).
func (s *Server) handleWGPlayerEnter(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != WGMapID {
		return
	}
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	// Mirror BattlefieldMgr::HandlePlayerEnterZone (BattlefieldMgr.cpp:100):
	// players do not join a disabled battlefield.
	if !wg.Enabled {
		wg.mu.Unlock()
		return
	}

	playerTeam := teamForRace(sess.player.Race)
	wg.PlayersInWar[sess.playerGUID] = playerTeam

	if wg.IsActive {
		if _, ok := wg.PlayerRanks[sess.playerGUID]; !ok {
			wg.PlayerRanks[sess.playerGUID] = WGSpellRecruit
			wg.PlayerKillsInRank[sess.playerGUID] = 0
		}
		sess.applyAura(WGSpellRecruit)
	}

	// Apply continent/zone phase shift
	if wg.DefenderTeam == WGTeamAlliance {
		sess.applyAura(WGSpellAllianceControlPhaseShift)
	} else {
		sess.applyAura(WGSpellHordeControlPhaseShift)
	}
	wg.mu.Unlock()

	s.sendWGInitWorldStatesTo(sess)
	s.UpdateWGTenacity()
}

// handleWGPlayerLeave processes a player leaving Wintergrasp.
// Reference: BattlefieldWG::OnPlayerLeaveWar / OnPlayerLeaveZone (BattlefieldWG.cpp:1095-1123).
func (s *Server) handleWGPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	delete(wg.PlayersInWar, sess.playerGUID)

	sess.removeAura(WGSpellRecruit)
	sess.removeAura(WGSpellCorporal)
	sess.removeAura(WGSpellLieutenant)
	sess.removeAura(WGSpellTenacity)
	sess.removeAura(WGSpellTowerControl)
	sess.removeAura(WGSpellGreatHonor)
	sess.removeAura(WGSpellGreaterHonor)
	sess.removeAura(WGSpellGreatestHonor)
	wg.mu.Unlock()

	s.UpdateWGTenacity()
}

// handleWGPlayerDeath handles deaths in Wintergrasp and grants promotion credits.
// Reference: BattlefieldWG::HandleKill (BattlefieldWG.cpp:962-977).
func (s *Server) handleWGPlayerDeath(victimSess *session, killerSess *session) {
	if s == nil || victimSess == nil || victimSess.player == nil || victimSess.player.Map != WGMapID {
		return
	}
	if killerSess == nil || killerSess.player == nil {
		return
	}
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if !wg.IsActive {
		return
	}

	killerTeam := teamForRace(killerSess.player.Race)
	victimTeam := teamForRace(victimSess.player.Race)
	if killerTeam == victimTeam {
		return
	}

	// BattlefieldWG::HandlePromotion: every in-war player of the killer's
	// team within 40 (2D) yards of the victim shares the promotion credit.
	vx, vy := victimSess.player.X, victimSess.player.Y
	for guid, team := range wg.PlayersInWar {
		if team != killerTeam {
			continue
		}
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil || sess.player.Map != wg.MapID {
			continue
		}
		dx := float64(sess.player.X - vx)
		dy := float64(sess.player.Y - vy)
		if math.Sqrt(dx*dx+dy*dy) > 40.0 {
			continue
		}
		wg.HandlePromotion(s, guid, team)
	}
}

// isWGGameObject identifies Wintergrasp interactible objects.
func isWGGameObject(entry uint32) bool {
	switch entry {
	case WGGameObjectTitanRelic,
		WGGameObjectVaultGate,
		WGGameObjectFortressGate,
		WGGameObjectKeepCollisionWall,
		WGGameObjectBannerNE,
		WGGameObjectBannerNW,
		WGGameObjectBannerSE,
		WGGameObjectBannerSW,
		WGGameObjectKeepTower1,
		WGGameObjectKeepTower2,
		WGGameObjectKeepTower3,
		WGGameObjectKeepTower4,
		WGGameObjectTowerShadowsight,
		WGGameObjectTowerWintersEdge,
		WGGameObjectTowerFlamewatch:
		return true
	default:
		return false
	}
}

// getOrCreateWGState returns or initializes the singleton Wintergrasp state.
func (s *Server) getOrCreateWGState() *wgBattlegroundState {
	s.wgMu.Lock()
	defer s.wgMu.Unlock()
	if s.wgState == nil {
		wg := &wgBattlegroundState{
			MapID:              WGMapID,
			ZoneID:             WGZoneID,
			IsActive:           false,
			DefenderTeam:       WGTeamAlliance,
			Timer:              WGNoWarDuration,
			TotalDuration:      WGNoWarDuration,
			EndTime:            time.Now().Add(WGNoWarDuration),
			Enabled:            true,
			Winner:             -1,
			PlayersInWar:       make(map[uint64]uint32),
			PlayersInQueue:     make(map[uint64]uint32),
			PlayerRanks:        make(map[uint64]uint32),
			PlayerKillsInRank:  make(map[uint64]uint32),
			Vehicles:           make(map[uint64]uint32),
			ReviveQueue:        make(map[uint64]uint64),
			LastResurrectTimer: wgResurrectInterval,
		}
		wg.initWorkshops()
		wg.initTowers()
		wg.initGates()
		wg.updateVehicleLimits()
		s.wgState = wg
	}
	return s.wgState
}

// updateWGWorldState sends an SMSG_UPDATE_WORLD_STATE packet to players in Wintergrasp.
func (s *Server) updateWGWorldState(variableID, value uint32) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == WGMapID {
			sess.sendWorldState(variableID, value)
		}
	}
}

// broadcastWGInitWorldStates sends SMSG_INIT_WORLD_STATES to all players in Wintergrasp.
func (s *Server) broadcastWGInitWorldStates() {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == WGMapID {
			s.sendWGInitWorldStatesTo(sess)
		}
	}
}

// sendWGInitWorldStatesTo serializes all Wintergrasp world states into SMSG_INIT_WORLD_STATES.
// Reference: BattlefieldWG::FillInitialWorldStates (BattlefieldWG.cpp:1155-1181).
func (s *Server) sendWGInitWorldStatesTo(sess *session) {
	if sess == nil || sess.player == nil {
		return
	}
	wg := s.getOrCreateWGState()

	// Reference: BattlefieldWG::FillInitialWorldStates (BattlefieldWG.cpp:1155-1181).
	// Wire order matches C++ exactly: stats, attacker/defender, active/show,
	// the two clocks, vehicle counts, the BuildingsInZone block (walls, keep
	// towers, passage walls, south towers, gates), then the workshops in
	// vector order (NE, NW, SE, SW, Keep West, Keep East).
	worldStates := [][2]int32{
		{int32(WGWorldStateDefendedA), int32(wg.StatsDefA)},
		{int32(WGWorldStateDefendedH), int32(wg.StatsDefH)},
		{int32(WGWorldStateAttackedA), int32(wg.StatsWonA)},
		{int32(WGWorldStateAttackedH), int32(wg.StatsWonH)},
		{int32(WGWorldStateAttacker), int32(wg.AttackerTeam())},
		{int32(WGWorldStateDefender), int32(wg.DefenderTeam)},
		{int32(WGWorldStateActive), boolToInt32(!wg.IsActive)},
		{int32(WGWorldStateShowWorldState), boolToInt32(wg.IsActive)},
	}

	clockVal := int32(time.Now().Unix() + int64(wg.Timer.Seconds()))
	worldStates = append(worldStates, [2]int32{int32(WGWorldStateClock1), clockVal})
	worldStates = append(worldStates, [2]int32{int32(WGWorldStateClock2), clockVal})

	worldStates = append(worldStates,
		[2]int32{int32(WGWorldStateVehicleH), int32(wg.VehiclesHorde)},
		[2]int32{int32(WGWorldStateMaxVehicleH), int32(wg.MaxVehiclesHorde)},
		[2]int32{int32(WGWorldStateVehicleA), int32(wg.VehiclesAlliance)},
		[2]int32{int32(WGWorldStateMaxVehicleA), int32(wg.MaxVehiclesAlliance)},
	)

	// Wall states (defender-team buildings, no tower warnings)
	for _, w := range wg.Walls {
		worldStates = append(worldStates, [2]int32{int32(w.WorldState), int32(wgBuildingState(w.State, wg.DefenderTeam))})
	}

	// Tower states
	for _, tw := range wg.Towers {
		worldStates = append(worldStates, [2]int32{int32(tw.WorldState), int32(wgBuildingState(tw.State, tw.TeamControl))})
	}

	// Gate states
	for _, gt := range wg.Gates {
		worldStates = append(worldStates, [2]int32{int32(gt.WorldState), int32(wgBuildingState(gt.State, wg.DefenderTeam))})
	}

	// Workshop states
	for _, id := range [WGMaxWorkshops]uint8{WGWorkshopNE, WGWorkshopNW, WGWorkshopSE, WGWorkshopSW, WGWorkshopKeepWest, WGWorkshopKeepEast} {
		ws := &wg.Workshops[id]
		worldStates = append(worldStates, [2]int32{int32(ws.WorldState), int32(wgBuildingState(0, ws.TeamControl))})
	}

	packet := protocol.NewBuffer(16 + len(worldStates)*8)
	packet.WriteI32(int32(WGMapID))
	packet.WriteI32(int32(WGZoneID))
	packet.WriteI32(0)
	packet.WriteU16(uint16(len(worldStates)))
	for _, ws := range worldStates {
		packet.WriteI32(ws[0])
		packet.WriteI32(ws[1])
	}
	_ = sess.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), packet.Bytes(), true)
}

func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
