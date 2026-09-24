package world

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Wintergrasp (WG) Constants mirroring TrinityCore BattlefieldWG.h / BattlefieldWG.cpp.
const (
	WGMapID  uint32 = 571  // Northrend
	WGZoneID uint32 = 4197 // Wintergrasp zone

	// Durations
	WGBattleDuration        = 30 * time.Minute  // 30 minutes war
	WGNoWarDuration         = 150 * time.Minute // 2.5 hours peace
	WGSouthTowerTimePenalty = 10 * time.Minute  // 10 minutes removed when all 3 south towers fall
	WGStartGroupingDuration = 15 * time.Minute  // 15 minutes before war

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

	// World States
	WGWorldStateActive         uint32 = 3801
	WGWorldStateShowWorldState uint32 = 3801
	WGWorldStateDefender       uint32 = 3803
	WGWorldStateAttacker       uint32 = 3802
	WGWorldStateDefendedA      uint32 = 3710
	WGWorldStateDefendedH      uint32 = 3774
	WGWorldStateAttackedA      uint32 = 3709
	WGWorldStateAttackedH      uint32 = 3775
	WGWorldStateVehicleH       uint32 = 3680
	WGWorldStateMaxVehicleH    uint32 = 3681
	WGWorldStateVehicleA       uint32 = 3490
	WGWorldStateMaxVehicleA    uint32 = 3491
	WGWorldStateClock1         uint32 = 3781
	WGWorldStateClock2         uint32 = 4354

	// Workshop WorldStates
	WGWorldStateWorkshopNW       uint32 = 3700 // Broken Temple
	WGWorldStateWorkshopNE       uint32 = 3701 // Sunken Ring
	WGWorldStateWorkshopSE       uint32 = 3702 // Eastspark
	WGWorldStateWorkshopSW       uint32 = 3703 // Westspark
	WGWorldStateWorkshopKeepWest uint32 = 3707
	WGWorldStateWorkshopKeepEast uint32 = 3708

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

	Workshops         [WGMaxWorkshops]wgWorkshopState
	Towers            [WGMaxTowers]wgTowerState
	Gates             [2]wgGateState // 0: Fortress Gate, 1: Vault Gate
	RelicInteractible bool
	BrokenSouthTowers uint32

	VehiclesAlliance    uint32
	VehiclesHorde       uint32
	MaxVehiclesAlliance uint32
	MaxVehiclesHorde    uint32

	TenacityTeam  int8 // -1: neutral, 0: Alliance, 1: Horde
	TenacityStack uint32

	PlayersInWar      map[uint64]uint32 // playerGUID -> team
	PlayerRanks       map[uint64]uint32 // playerGUID -> rank spell ID (Recruit, Corporal, Lieutenant)
	PlayerKillsInRank map[uint64]uint32 // playerGUID -> kills in current rank towards next promotion (0..5)
	Vehicles          map[uint64]uint32 // vehicleGUID -> team

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
	wg.Timer = WGNoWarDuration
	wg.TotalDuration = WGNoWarDuration
	wg.EndTime = time.Now().Add(WGNoWarDuration)

	// Reward players
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

		// Remove wartime auras
		sess.removeAura(WGSpellRecruit)
		sess.removeAura(WGSpellCorporal)
		sess.removeAura(WGSpellLieutenant)
		sess.removeAura(WGSpellTenacity)
		sess.removeAura(WGSpellTowerControl)

		if pTeam == winningTeam {
			sess.applyAura(WGSpellEssenceOfWintergrasp)
			sess.applyAura(WGSpellVictoryReward)
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
			if pTeam == WGTeamAlliance {
				sess.removeAura(WGSpellAllianceControlPhaseShift)
			} else {
				sess.removeAura(WGSpellHordeControlPhaseShift)
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
		if allyCount < hordeCount {
			newStack = int32((float32(hordeCount)/float32(allyCount) - 1.0) * 4.0)
		} else if hordeCount < allyCount {
			newStack = int32((1.0 - float32(allyCount)/float32(hordeCount)) * 4.0)
		}
	}

	if newStack == int32(wg.TenacityStack) && ((newStack > 0 && wg.TenacityTeam == 0) || (newStack < 0 && wg.TenacityTeam == 1) || (newStack == 0 && wg.TenacityTeam == -1)) {
		return
	}

	var teamToBuff int8 = -1
	absStack := uint32(0)
	if newStack > 0 {
		teamToBuff = 0 // Alliance
		absStack = uint32(newStack)
	} else if newStack < 0 {
		teamToBuff = 1 // Horde
		absStack = uint32(-newStack)
	}
	if absStack > 20 {
		absStack = 20
	}

	wg.TenacityTeam = teamToBuff
	wg.TenacityStack = absStack

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
func (s *Server) TickWG(delta time.Duration) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	if wg.Timer > delta {
		wg.Timer -= delta
		wg.mu.Unlock()
		return
	}
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

// handleWGGameObjectUse dispatches interaction with Wintergrasp objects.
// Reference: BattlefieldWG::ProcessEvent (BattlefieldWG.cpp:1247-1281).
func (s *Server) handleWGGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return false
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

	wg.HandlePromotion(s, killerSess.playerGUID, killerTeam)
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
			MapID:             WGMapID,
			ZoneID:            WGZoneID,
			IsActive:          false,
			DefenderTeam:      WGTeamAlliance,
			Timer:             WGNoWarDuration,
			TotalDuration:     WGNoWarDuration,
			EndTime:           time.Now().Add(WGNoWarDuration),
			Winner:            -1,
			PlayersInWar:      make(map[uint64]uint32),
			PlayerRanks:       make(map[uint64]uint32),
			PlayerKillsInRank: make(map[uint64]uint32),
			Vehicles:          make(map[uint64]uint32),
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

	worldStates := [][2]int32{
		{int32(WGWorldStateDefendedA), int32(wg.StatsDefA)},
		{int32(WGWorldStateDefendedH), int32(wg.StatsDefH)},
		{int32(WGWorldStateAttackedA), int32(wg.StatsWonA)},
		{int32(WGWorldStateAttackedH), int32(wg.StatsWonH)},
		{int32(WGWorldStateAttacker), int32(wg.AttackerTeam())},
		{int32(WGWorldStateDefender), int32(wg.DefenderTeam)},
		{int32(WGWorldStateActive), boolToInt32(!wg.IsActive)},
		{int32(WGWorldStateShowWorldState), boolToInt32(wg.IsActive)},
		{int32(WGWorldStateVehicleH), int32(wg.VehiclesHorde)},
		{int32(WGWorldStateMaxVehicleH), int32(wg.MaxVehiclesHorde)},
		{int32(WGWorldStateVehicleA), int32(wg.VehiclesAlliance)},
		{int32(WGWorldStateMaxVehicleA), int32(wg.MaxVehiclesAlliance)},
	}

	clockVal := int32(time.Now().Unix() + int64(wg.Timer.Seconds()))
	worldStates = append(worldStates, [2]int32{int32(WGWorldStateClock1), clockVal})
	worldStates = append(worldStates, [2]int32{int32(WGWorldStateClock2), clockVal})

	// Workshop states
	for _, ws := range wg.Workshops {
		worldStates = append(worldStates, [2]int32{int32(ws.WorldState), int32(wgBuildingState(0, ws.TeamControl))})
	}

	// Tower states
	for _, tw := range wg.Towers {
		worldStates = append(worldStates, [2]int32{int32(tw.WorldState), int32(wgBuildingState(tw.State, tw.TeamControl))})
	}

	// Gate states
	for _, gt := range wg.Gates {
		worldStates = append(worldStates, [2]int32{int32(gt.WorldState), int32(wgBuildingState(gt.State, wg.DefenderTeam))})
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
