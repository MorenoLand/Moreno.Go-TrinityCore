package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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
	wgMaxPlayersPerTeam     = 100               // Wintergrasp.PlayerMax default (World.cpp:1477)
	wgMinPlayers            = 0                 // Wintergrasp.PlayerMin default (World.cpp:1478); the queue-invite gate below reads it
	wgMinLevel              = 77                // Wintergrasp.PlayerMinLvl default (World.cpp:1479); the war-invite level leg below reads it
	wgResurrectInterval     = 30 * time.Second  // RESURRECTION_INTERVAL (Battlefield.cpp:191-199)
	wgInviteAcceptWindow    = 20 * time.Second  // m_TimeForAcceptInvite (BattlefieldWG.cpp:429)
	wgKickDelay             = 10 * time.Second  // m_PlayersWillBeKick deadline (Battlefield.cpp:89, 253)
	wgKickSweepInterval     = time.Second       // m_uiKickDontAcceptTimer / m_uiKickAfkPlayersTimer (Battlefield.cpp:60, 165-186)

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
	WGSpellRestrictedFlightArea              uint32 = 58730
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

	// Graveyard IDs in WGGraveyardId enum order (BattlefieldWG.h:152-160).
	// Note the order differs from the workshop enum above (SE, SW, NE, NW):
	// GY index 0 is the NE graveyard, not the SE one. BfGraveyard lookups
	// below use these indices directly, as C++ does.
	WGGraveyardNE       uint8 = 0
	WGGraveyardNW       uint8 = 1
	WGGraveyardSE       uint8 = 2
	WGGraveyardSW       uint8 = 3
	WGGraveyardKeep     uint8 = 4
	WGGraveyardHorde    uint8 = 5
	WGGraveyardAlliance uint8 = 6
	WGMaxGraveyards           = 7

	// Wintergrasp sub-area IDs (SharedDefines.h AreaId enum, :3834-3840)
	// for the spirit-graveyard mapping below; WGZoneID already carries
	// AREA_WINTERGRASP (4197).
	WGAreaSunkenRing      uint32 = 4538
	WGAreaBrokenTemplate  uint32 = 4539
	WGAreaFortress        uint32 = 4575
	WGAreaChilledQuagmire uint32 = 4589
	WGAreaWestpark        uint32 = 4611
	WGAreaEastpark        uint32 = 4612
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
	WGCreatureStalker             uint32 = 15214 // BATTLEFIELD_WG_NPC_STALKER (BattlefieldWG.h:178), the SendWarning/SendChat source

	// Wintergrasp stalker creature-text groups (BattlefieldWG.h:470-471):
	// the rank-up SendChat groups. CHAT_MSG_ADDON/LANG_ADDON in the
	// PromotePlayer call are placeholders (SharedDefines.h:3161:
	// CHAT_MSG_ADDON = 0xFF): SendChat replaces them with the row's
	// type, language, and range, so the row — not this call — decides
	// the packet type and audience.
	WGTextRankCorporal        uint8 = 37
	WGTextRankFirstLieutenant uint8 = 38

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

// Graveyard definition: one entry per WGGraveyard row
// (BattlefieldWG.cpp:53-64: position, WorldSafeLocs ID, gossip text ID,
// starting controller). TeamControl is the BfGraveyard::m_ControlTeam
// value; the spirit guides live in wgBattlegroundState.SpiritGuides
// (BfGraveyard::m_SpiritGuide, one guide GUID per graveyard per team),
// while the resurrect queue itself stays battlefield-level (ReviveQueue)
// with per-graveyard attribution resolved through that table.
type wgGraveyardState struct {
	ID           uint8
	SafeLocID    uint32
	GossipTextID uint32
	TeamControl  uint32 // WGTeamAlliance, WGTeamHorde, WGTeamNeutral
	Name         string
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

	Workshops  [WGMaxWorkshops]wgWorkshopState
	Graveyards [WGMaxGraveyards]wgGraveyardState
	// SpiritGuides mirrors BfGraveyard::m_SpiritGuide per graveyard
	// (Battlefield.cpp:668): [graveyard][team] guide GUIDs, set by
	// assignWGSpiritGuide; 0 means no guide assigned yet.
	SpiritGuides      [WGMaxGraveyards][2]uint64
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

	PlayersInWar      map[uint64]uint32    // playerGUID -> team
	PlayersInZone     map[uint64]uint32    // playerGUID -> team; mirrors Battlefield::m_players, the zone roster, distinct from the war roster
	PlayersInQueue    map[uint64]uint32    // playerGUID -> team; mirrors Battlefield::m_PlayersInQueue[team] (Battlefield.h)
	InvitedPlayers    map[uint64]wgInvite  // playerGUID -> team + accept deadline; mirrors Battlefield::m_InvitedPlayers (Battlefield.h:374)
	PlayersWillBeKick map[uint64]time.Time // playerGUID -> kick deadline; mirrors Battlefield::m_PlayersWillBeKick
	KickSweepTimer    time.Duration        // accumulator for the 1s wartime kick sweep (Battlefield::Update)
	StartGrouping     bool                 // mirrors Battlefield::m_StartGrouping; set at grouping, reset by EndBattle
	PlayerRanks       map[uint64]uint32    // playerGUID -> rank spell ID (Recruit, Corporal, Lieutenant)
	PlayerKillsInRank map[uint64]uint32    // playerGUID -> kills in current rank towards next promotion (0..5)
	Vehicles          map[uint64]uint32    // vehicleGUID -> team

	// ReviveQueue is the Wintergrasp spirit-healer revive queue
	// (playerGUID -> spirit-guide GUID). C++ fans the queue out per
	// graveyard (BfGraveyard::m_ResurrectQueue, Battlefield.cpp:605-627);
	// Go keeps the single battlefield-level queue, with per-graveyard
	// attribution resolved through SpiritGuides (a queued player's
	// guide identifies its graveyard, as in AddPlayerToResurrectQueue).
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

// initGraveyards sets up the 7 Wintergrasp graveyards.
// Reference: BattlefieldWG::SetupBattlefield (BattlefieldWG.cpp:471-483):
// graveyards whose WGGraveyard row starts neutral take the defending
// team between battles, the Horde and Alliance graveyards keep their
// fixed team. This is the steady between-battles state (the same values
// OnBattleEnd restores); C++'s first-boot workshop-init scramble of
// graveyard control is not reproduced, since Go's initWorkshops already
// carries the corrected OnBattleStart workshop assignment.
func (wg *wgBattlegroundState) initGraveyards() {
	defender := wg.DefenderTeam
	wg.Graveyards = [WGMaxGraveyards]wgGraveyardState{
		{ID: WGGraveyardNE, SafeLocID: 1329, GossipTextID: 20071, TeamControl: defender, Name: "NE Workshop Graveyard"},
		{ID: WGGraveyardNW, SafeLocID: 1330, GossipTextID: 20072, TeamControl: defender, Name: "NW Workshop Graveyard"},
		{ID: WGGraveyardSE, SafeLocID: 1333, GossipTextID: 20074, TeamControl: defender, Name: "SE Workshop Graveyard"},
		{ID: WGGraveyardSW, SafeLocID: 1334, GossipTextID: 20073, TeamControl: defender, Name: "SW Workshop Graveyard"},
		{ID: WGGraveyardKeep, SafeLocID: 1285, GossipTextID: 20070, TeamControl: defender, Name: "Keep Graveyard"},
		{ID: WGGraveyardHorde, SafeLocID: 1331, GossipTextID: 20075, TeamControl: WGTeamHorde, Name: "Horde Graveyard"},
		{ID: WGGraveyardAlliance, SafeLocID: 1332, GossipTextID: 20076, TeamControl: WGTeamAlliance, Name: "Alliance Graveyard"},
	}
}

// updateGraveyardsFromWorkshops mirrors the graveyard leg of
// WintergraspWorkshop::UpdateGraveyardAndWorkshop
// (BattlefieldWG.cpp:1830-1836), called from OnBattleStart: each
// capturable workshop's control flips its linked graveyard. The link
// is the workshop enum ID passed straight to GetGraveyardById
// (BattlefieldWG.cpp:1800-1801, 1818-1819), NOT the workshop's
// position in the workshop list — and the graveyard enum orders
// NE, NW, SE, SW while the workshop enum orders SE, SW, NE, NW, so
// the linkage is crossed exactly as in C++: an Eastspark (SE, id 0)
// control flips Graveyards[0], the NE graveyard. Keep workshops
// have no graveyard leg (WorkshopId >= KEEP_WEST gate). Caller
// holds wg.mu; workshop controls are already set by initWorkshops.
func (wg *wgBattlegroundState) updateGraveyardsFromWorkshops() {
	for w := uint8(0); w < WGWorkshopKeepWest; w++ {
		wg.Graveyards[w].TeamControl = wg.Workshops[w].TeamControl
	}
}

// wgSpiritGraveyardID mirrors BattlefieldWG::GetSpiritGraveyardId
// (BattlefieldWG.cpp:794-815): the area a spirit guide stands in selects
// its graveyard. The boolean reports whether the area matched: C++ logs
// an error and returns 0 (the NE graveyard) for unknown areas, but a Go
// caller that cannot resolve the terrain area must not mis-attribute a
// guide to NE, so unknown areas report false and the caller skips.
func wgSpiritGraveyardID(areaID uint32) (uint8, bool) {
	switch areaID {
	case WGAreaFortress:
		return WGGraveyardKeep, true
	case WGAreaSunkenRing:
		return WGGraveyardNE, true
	case WGAreaBrokenTemplate:
		return WGGraveyardNW, true
	case WGAreaWestpark:
		return WGGraveyardSW, true
	case WGAreaEastpark:
		return WGGraveyardSE, true
	case WGZoneID:
		return WGGraveyardAlliance, true
	case WGAreaChilledQuagmire:
		return WGGraveyardHorde, true
	}
	return WGGraveyardNE, false
}

// assignWGSpiritGuide mirrors the spirit-guide arm of
// BattlefieldWG::OnCreatureCreate (BattlefieldWG.cpp:823-832): a dwarven
// guide (31842) serves Alliance, a taunka guide (31841) Horde, and the
// area it stands in selects its graveyard (BfGraveyard::SetSpirit,
// Battlefield.cpp:660-670, stores the guide per graveyard per team).
// Go has no creature-create dispatch for DB spawns -- spirit guides never
// enter the motion system -- so assignment fires when a ghost first
// queues at the guide, which covers every guide that can ever hold a
// queue: a guide with no queue needs no attribution. Non-guide entries
// and unmapped areas leave the table untouched.
func (s *Server) assignWGSpiritGuide(spiritGUID uint64, areaID uint32) {
	var team uint32
	switch uint32((spiritGUID >> 24) & 0xFFFFFF) {
	case WGCreatureSpiritGuideAlliance:
		team = WGTeamAlliance
	case WGCreatureSpiritGuideHorde:
		team = WGTeamHorde
	default:
		return
	}
	gy, ok := wgSpiritGraveyardID(areaID)
	if !ok {
		return
	}
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	wg.SpiritGuides[gy][team] = spiritGUID
	wg.mu.Unlock()
}

// wgGraveyardForSpiritGuide resolves a queued spirit guide back to its
// graveyard -- the AddPlayerToResurrectQueue lookup
// (Battlefield.cpp:605-616), which finds the graveyard holding the guide
// (BfGraveyard::HasNpc, :760-770). Caller holds wg.mu.
func (wg *wgBattlegroundState) wgGraveyardForSpiritGuide(spiritGUID uint64) (uint8, bool) {
	for gy := uint8(0); gy < WGMaxGraveyards; gy++ {
		if wg.SpiritGuides[gy][WGTeamAlliance] == spiritGUID || wg.SpiritGuides[gy][WGTeamHorde] == spiritGUID {
			return gy, true
		}
	}
	return 0, false
}

// queuedDeadAtGraveyard collects the queued ghosts attributed to one
// graveyard, in ascending GUID order (C++ iterates the GuidSet, so the
// relocation order is GUID order). Caller holds wg.mu. Queued players
// whose guide was never assigned (unmapped area) are skipped: with no
// attribution they cannot be proven to belong to this graveyard.
func (wg *wgBattlegroundState) queuedDeadAtGraveyard(gy uint8) []uint64 {
	var out []uint64
	for playerGUID, spiritGUID := range wg.ReviveQueue {
		if atGY, ok := wg.wgGraveyardForSpiritGuide(spiritGUID); ok && atGY == gy {
			out = append(out, playerGUID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// closestWGGraveyard mirrors Battlefield::GetClosestGraveyard
// (Battlefield.cpp:579-600): among the graveyards controlled by the
// player's team, the nearest by 2D distance from the graveyard's
// WorldSafeLocs position (BfGraveyard::GetDistance, :677-681). Squared
// distances compare in the same order as C++'s GetDistance2d. No
// team-controlled graveyard, or no resolvable SafeLocs row, reports
// not-found (C++ returns nullptr).
func (s *Server) closestWGGraveyard(team uint32, x, y float32) (wotlk.WorldSafeLoc, bool) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	graves := wg.Graveyards
	wg.mu.Unlock()
	if s.Data == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	var best wotlk.WorldSafeLoc
	bestDist := float32(-1)
	for i := range graves {
		if graves[i].TeamControl != team {
			continue
		}
		loc, found, err := s.Data.WorldSafeLoc(graves[i].SafeLocID)
		if err != nil || !found {
			continue
		}
		dx, dy := loc.X-x, loc.Y-y
		if dist := dx*dx + dy*dy; bestDist < 0 || dist < bestDist {
			best, bestDist = loc, dist
		}
	}
	if bestDist < 0 {
		return wotlk.WorldSafeLoc{}, false
	}
	return best, true
}

// relocateWGDeadPlayers mirrors BfGraveyard::RelocateDeadPlayers
// (Battlefield.cpp:745-770): ghosts queued at a graveyard that just
// changed control teleport to the closest graveyard their team still
// controls, on the same map, keeping orientation. C++ computes the
// closest grave for the first queued player and reuses it for the rest
// (closestGrave cached across the loop); a player with no
// team-controlled grave is skipped without poisoning the cache, so
// later players still get a computed grave. Caller holds no lock: the
// teleport's zone-change path takes wg.mu.
func (s *Server) relocateWGDeadPlayers(playerGUIDs []uint64) {
	var grave wotlk.WorldSafeLoc
	haveGrave := false
	for _, playerGUID := range playerGUIDs {
		sess := s.findSessionByGUID(playerGUID)
		if sess == nil || sess.player == nil {
			continue
		}
		if !haveGrave {
			loc, found := s.closestWGGraveyard(teamForRace(sess.player.Race), sess.player.X, sess.player.Y)
			if !found {
				continue
			}
			grave, haveGrave = loc, true
		}
		sess.teleportTo(WGMapID, grave.X, grave.Y, grave.Z, sess.player.Orientation)
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
// Reference: Battlefield::StartBattle (Battlefield.cpp:316-335) clears
// m_PlayersInWar, then InvitePlayersInZoneToWar (Battlefield.cpp:241-262)
// and InvitePlayersInQueueToWar (:222-239) repopulate it; OnBattleStart
// (BattlefieldWG.cpp:567-629) is the WG script hook. Go previously kept
// every zone entrant in PlayersInWar across battles, so the EndBattle
// clear the head tick deferred could never land.
func (s *Server) StartWGBattle(defenderTeam uint32) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	if wg.IsActive {
		wg.mu.Unlock()
		return
	}

	wg.IsActive = true
	wg.DefenderTeam = defenderTeam
	wg.Timer = WGBattleDuration
	wg.TotalDuration = WGBattleDuration
	wg.EndTime = time.Now().Add(WGBattleDuration)
	wg.Winner = -1

	wg.initWorkshops()
	// OnBattleStart (BattlefieldWG.cpp:605-606) runs
	// UpdateGraveyardAndWorkshop per workshop; the workshop-control
	// half is initWorkshops above, this is the graveyard half.
	wg.updateGraveyardsFromWorkshops()
	wg.initTowers()
	wg.initGates()
	wg.updateVehicleLimits()

	wg.VehiclesAlliance = 0
	wg.VehiclesHorde = 0
	wg.Vehicles = make(map[uint64]uint32)
	wg.PlayerRanks = make(map[uint64]uint32)
	wg.PlayerKillsInRank = make(map[uint64]uint32)
	wg.KickSweepTimer = wgKickSweepInterval

	s.inviteWGZoneAndQueueToWar(wg)

	s.broadcastWGInitWorldStates()
	s.broadcastWGZoneMessage("The battle for Wintergrasp has begun!")
	wg.mu.Unlock()
	s.UpdateWGTenacity()
}

// wgInvite is one outstanding war invite: the team it was issued for and
// the accept deadline (GameTime + m_TimeForAcceptInvite).
type wgInvite struct {
	Team     uint32
	Deadline time.Time
}

// warAndInvitedCount counts the per-team war roster plus outstanding
// invites, the vacancy check every InvitePlayerToWar caller applies
// (Battlefield.cpp:84, 229, 251).
func (wg *wgBattlegroundState) warAndInvitedCount(team uint32) int {
	count := 0
	for _, t := range wg.PlayersInWar {
		if t == team {
			count++
		}
	}
	for _, inv := range wg.InvitedPlayers {
		if inv.Team == team {
			count++
		}
	}
	return count
}

// inviteWGPlayerToWar mirrors Battlefield::InvitePlayerToWar
// (Battlefield.cpp:260-292): already-in-war or already-invited is a
// no-op, a will-be-kick entry is cleared, and the invite lands in
// m_InvitedPlayers with a SendBfInvitePlayerToWar dialog
// (BattlefieldHandler.cpp:36-44: battleId, zoneId, accept deadline).
// The vacancy check lives in the callers, as in C++. Caller holds wg.mu.
// The guards run in C++ order ahead of the invited check: in-flight
// (the live TaxiPath flag, the in-tree flight gate used by combat.go
// and spells.go) returns with nothing changed; in-arena or
// in-battleground (an Active bgQueues entry InProgress) erases the
// player from the WG queue and returns; below the min level
// (Wintergrasp.PlayerMinLvl, 77) marks will-be-kick once and returns
// without inviting.
func (s *Server) inviteWGPlayerToWar(wg *wgBattlegroundState, sess *session, team uint32) bool {
	if sess == nil || sess.player == nil {
		return false
	}
	guid := sess.playerGUID
	if sess.player.TaxiPath != "" {
		return false
	}
	for i := range sess.bgQueues {
		if e := &sess.bgQueues[i]; e.Active && e.Status == BGStatusInProgress {
			delete(wg.PlayersInQueue, guid)
			return false
		}
	}
	if uint32(sess.player.Level) < wgMinLevel {
		if _, marked := wg.PlayersWillBeKick[guid]; !marked {
			wg.PlayersWillBeKick[guid] = time.Now().Add(wgKickDelay)
		}
		return false
	}
	if _, inWar := wg.PlayersInWar[guid]; inWar {
		return false
	}
	if _, invited := wg.InvitedPlayers[guid]; invited {
		return false
	}
	delete(wg.PlayersWillBeKick, guid)
	deadline := time.Now().Add(wgInviteAcceptWindow)
	wg.InvitedPlayers[guid] = wgInvite{Team: team, Deadline: deadline}
	packet := protocol.NewBuffer(12)
	packet.WriteU32(WGBattleID)
	packet.WriteU32(WGZoneID)
	packet.WriteU32(uint32(deadline.Unix()))
	_ = sess.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_ENTRY_INVITE), packet.Bytes(), true)
	return true
}

// joinWGWar moves a player into the war roster on invite accept.
// Caller holds wg.mu. Mirrors Battlefield::PlayerAcceptInviteToWar
// (Battlefield.cpp:390-404) minus the raid-group leg: invited erase,
// war insert, and OnPlayerJoinWar (BattlefieldWG.cpp:1061-1094):
// RemoveAurasFromPlayer's strip, the Recruit cast with the rank reset,
// and the tower-control stacks (attacker 3 - broken south towers,
// defender broken south towers). Accept does not require an outstanding
// invite in C++ either, so an uninvited accept still joins.
func (s *Server) joinWGWar(wg *wgBattlegroundState, sess *session, team uint32) {
	if sess == nil {
		return
	}
	delete(wg.InvitedPlayers, sess.playerGUID)
	wg.PlayersInWar[sess.playerGUID] = team
	// RemoveAurasFromPlayer (BattlefieldWG.cpp:1049-1059) strips the
	// wartime auras before the Recruit cast, so a rejoiner drops a stale
	// Corporal/Lieutenant rank instead of keeping it with no rank aura.
	sess.removeAura(WGSpellRecruit)
	sess.removeAura(WGSpellCorporal)
	sess.removeAura(WGSpellLieutenant)
	sess.removeAura(WGSpellTenacity)
	sess.removeAura(WGSpellTowerControl)
	sess.removeAura(WGSpellSpiritualImmunity)
	sess.removeAura(WGSpellRestrictedFlightArea)
	sess.removeAura(WGSpellEssenceOfWintergrasp)
	wg.PlayerRanks[sess.playerGUID] = WGSpellRecruit
	wg.PlayerKillsInRank[sess.playerGUID] = 0
	sess.applyAura(WGSpellRecruit)
	if team == wg.AttackerTeam() {
		if wg.BrokenSouthTowers < 3 {
			s.setWGTowerControlStacks(sess, int(3-wg.BrokenSouthTowers))
		}
	} else if wg.BrokenSouthTowers > 0 {
		s.setWGTowerControlStacks(sess, int(wg.BrokenSouthTowers))
	}
}

// wgTowerControlStacks reads the live Tower Control stack count on sess.
// A present aura whose StackCount field is still zero carries one stack:
// applyAura's fresh apply sends stack 1 without setting the field.
func wgTowerControlStacks(sess *session) int {
	if sess == nil {
		return 0
	}
	sess.castMu.Lock()
	defer sess.castMu.Unlock()
	aura := sess.activeAuras[WGSpellTowerControl]
	if aura == nil || aura.Stopped {
		return 0
	}
	if aura.StackCount == 0 {
		return 1
	}
	return int(aura.StackCount)
}

// setWGTowerControlStacks mirrors Aura::SetStackAmount for Tower Control
// (BattlefieldWG.cpp:1085/1090 join stacks, :1217/:1224 destroy steps):
// zero stacks removes the aura, otherwise a fresh apply is corrected to
// the count with the SetStackAmount rescale + client update. Caller holds
// no castMu; applyAura/removeAura take it themselves.
func (s *Server) setWGTowerControlStacks(sess *session, stacks int) {
	if sess == nil {
		return
	}
	if stacks <= 0 {
		sess.removeAura(WGSpellTowerControl)
		return
	}
	sess.applyAura(WGSpellTowerControl)
	sess.castMu.Lock()
	aura := sess.activeAuras[WGSpellTowerControl]
	if aura == nil || aura.Stopped {
		sess.castMu.Unlock()
		return
	}
	oldCount := int(aura.StackCount)
	if oldCount == 0 {
		oldCount = 1
	}
	aura.StackCount = uint8(stacks)
	rescaleAuraAmountsByStack(aura, uint32(oldCount), uint32(stacks))
	slot, positive, maxDur, dur := aura.Slot, aura.Positive, aura.DurationMs, aura.RemainingMs
	sess.castMu.Unlock()
	sess.sendAuraUpdateWithStack(slot, WGSpellTowerControl, false, positive, maxDur, dur, uint8(stacks))
}

// inviteWGZoneToWar mirrors Battlefield::InvitePlayersInZoneToWar
// (Battlefield.cpp:241-258): GUID-ordered zone-roster sweep, inviting
// while the per-team war+invited count is below m_MaxPlayer and marking
// the rest for kick after wgKickDelay. GUIDs in exclude (just kicked
// this sweep) are skipped so a kicked player is not re-invited in the
// same pass. Caller holds wg.mu.
func (s *Server) inviteWGZoneToWar(wg *wgBattlegroundState, exclude map[uint64]bool) {
	zoneGUIDs := make([]uint64, 0, len(wg.PlayersInZone))
	for guid := range wg.PlayersInZone {
		zoneGUIDs = append(zoneGUIDs, guid)
	}
	sort.Slice(zoneGUIDs, func(i, j int) bool { return zoneGUIDs[i] < zoneGUIDs[j] })
	for _, guid := range zoneGUIDs {
		if exclude[guid] {
			continue
		}
		team := wg.PlayersInZone[guid]
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil || sess.player.Map != WGMapID || sess.player.Zone != WGZoneID {
			continue
		}
		if wg.warAndInvitedCount(team) < wgMaxPlayersPerTeam {
			s.inviteWGPlayerToWar(wg, sess, team)
		} else {
			wg.PlayersWillBeKick[guid] = time.Now().Add(wgKickDelay)
		}
	}
}

// inviteWGQueueToWar mirrors Battlefield::InvitePlayersInQueueToWar
// (Battlefield.cpp:222-239): the queue sweep invites while vacant and,
// unlike the zone sweep, marks nothing on full. Caller holds wg.mu;
// the caller clears the queue afterwards, as C++ does.
func (s *Server) inviteWGQueueToWar(wg *wgBattlegroundState) {
	queueGUIDs := make([]uint64, 0, len(wg.PlayersInQueue))
	for guid := range wg.PlayersInQueue {
		queueGUIDs = append(queueGUIDs, guid)
	}
	sort.Slice(queueGUIDs, func(i, j int) bool { return queueGUIDs[i] < queueGUIDs[j] })
	for _, guid := range queueGUIDs {
		team := wg.PlayersInQueue[guid]
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil {
			continue
		}
		if wg.warAndInvitedCount(team) < wgMaxPlayersPerTeam {
			s.inviteWGPlayerToWar(wg, sess, team)
		}
	}
}

// wgQueueCount counts accepted queue members of one team. The queue map is
// team-flattened, so the count is a sweep, not a lookup.
func (wg *wgBattlegroundState) wgQueueCount(team uint32) int {
	count := 0
	for _, t := range wg.PlayersInQueue {
		if t == team {
			count++
		}
	}
	return count
}

// inviteWGPlayerToQueue mirrors Battlefield::InvitePlayerToQueue
// (Battlefield.cpp:212-218): already-queued is a no-op, and the invite is
// sent only while the own team's queue is at or below m_MinPlayer or the
// other team's queue is at or above it (with the Wintergrasp.PlayerMin
// default of 0 the second leg always holds, so the send is ungated in
// practice — the gate is kept literal, not simplified away). The packet
// is SMSG_BATTLEFIELD_MGR_QUEUE_INVITE (BattlefieldHandler.cpp:52-58:
// battleId + warmup byte). The queue insert itself happens on the
// client's accept (PlayerAcceptInviteToQueue, :366), already ported in
// battleground.go, so this only sends. Caller holds wg.mu.
func (s *Server) inviteWGPlayerToQueue(wg *wgBattlegroundState, sess *session, team uint32) {
	if sess == nil || sess.player == nil {
		return
	}
	if _, queued := wg.PlayersInQueue[sess.playerGUID]; queued {
		return
	}
	other := WGTeamHorde
	if team == WGTeamHorde {
		other = WGTeamAlliance
	}
	if wg.wgQueueCount(team) <= wgMinPlayers || wg.wgQueueCount(other) >= wgMinPlayers {
		packet := protocol.NewBuffer(5)
		packet.WriteU32(WGBattleID)
		packet.WriteU8(1)
		_ = sess.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_QUEUE_INVITE), packet.Bytes(), true)
	}
}

// inviteWGZoneToQueue mirrors Battlefield::InvitePlayersInZoneToQueue
// (Battlefield.cpp:204-210): a GUID-ordered sweep of the zone roster,
// inviting each live zone member to the queue. Caller holds wg.mu.
func (s *Server) inviteWGZoneToQueue(wg *wgBattlegroundState) {
	zoneGUIDs := make([]uint64, 0, len(wg.PlayersInZone))
	for guid := range wg.PlayersInZone {
		zoneGUIDs = append(zoneGUIDs, guid)
	}
	sort.Slice(zoneGUIDs, func(i, j int) bool { return zoneGUIDs[i] < zoneGUIDs[j] })
	for _, guid := range zoneGUIDs {
		team := wg.PlayersInZone[guid]
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil || sess.player.Map != WGMapID || sess.player.Zone != WGZoneID {
			continue
		}
		s.inviteWGPlayerToQueue(wg, sess, team)
	}
}

// broadcastWGZoneMessage sends a system chat line to every live player in
// the Wintergrasp zone, the Go stand-in for Battlefield::SendWarning
// (Battlefield.cpp:448-452: stalker creature text, zone-range chat) used
// where Go models no stalker creature. It carries every WG SendWarning
// line (grouping, battle start, fortress defend/capture, tower damage/
// destroy, workshop capture); the old broadcastBattlegroundMessage
// routing reached nobody, since its BG-queue audience requires a
// battleground queue slot on map 571 and WG players never hold one.
func (s *Server) broadcastWGZoneMessage(message string) {
	if s == nil {
		return
	}
	payload := protocol.BuildChatMessageWithOptions(chatSystem, 0, 0, 0, message, "", false, "", 0)
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == WGMapID && sess.player.Zone == WGZoneID {
			_ = sess.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload, true)
		}
	}
}

// kickWGPlayerFromBattlefield mirrors Battlefield::KickPlayerFromBattlefield
// (Battlefield.cpp:309-314): a player still in the zone is teleported to
// KickPosition (BattlefieldWG.cpp:435). Call after releasing wg.mu: the
// teleport's zone-change path takes wg.mu via handleWGPlayerLeave.
func (s *Server) kickWGPlayerFromBattlefield(sess *session) {
	if sess == nil || sess.player == nil || sess.player.Map != WGMapID || sess.player.Zone != WGZoneID {
		return
	}
	sess.teleportTo(WGMapID, 5728.117, 2714.346, 697.733, 0)
}

// inviteWGZoneAndQueueToWar repopulates the war at battle start.
// Reference: Battlefield::StartBattle (Battlefield.cpp:316-335) clears
// m_PlayersInWar, then InvitePlayersInZoneToWar and
// InvitePlayersInQueueToWar invite; players join the war only on accept,
// so the sweep records invites (and full-battlefield kick marks) instead
// of inserting into PlayersInWar directly. OnPlayerJoinWar's
// tower-control stacks land at accept via joinWGWar, not here.
func (s *Server) inviteWGZoneAndQueueToWar(wg *wgBattlegroundState) {
	wg.PlayersInWar = make(map[uint64]uint32)
	wg.InvitedPlayers = make(map[uint64]wgInvite)
	wg.PlayersWillBeKick = make(map[uint64]time.Time)
	s.inviteWGZoneToWar(wg, nil)
	s.inviteWGQueueToWar(wg)
	wg.PlayersInQueue = make(map[uint64]uint32)
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
	// Battlefield::EndBattle (Battlefield.cpp:343) resets m_StartGrouping,
	// so the next peace period's grouping sweep fires again at 15 minutes.
	wg.StartGrouping = false
	// OnBattleEnd reads the remaining battle timer before it is reset
	// (DoCompleteOrIncrementAchievement(ACHIEVEMENTS_WIN_WG_TIMER_10) when
	// !endByTimer && GetTimer() <= 10000); capture it here.
	remainingBattleTime := wg.Timer
	wg.Timer = WGNoWarDuration
	wg.TotalDuration = WGNoWarDuration
	wg.EndTime = time.Now().Add(WGNoWarDuration)

	// Reward players: OnBattleEnd rewards and aura-strips reach
	// m_PlayersInWar only (BattlefieldWG.cpp:704-731) — the old
	// all-sessions loop handed victory/defeat rewards to every player
	// of the team anywhere in the world, not just war participants.
	// Reference: BattlefieldWG::OnBattleEnd (BattlefieldWG.cpp:651-760).
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		pGUID := sess.playerGUID
		pTeam, inWar := wg.PlayersInWar[pGUID]
		if !inWar {
			continue
		}

		// Remove wartime auras (BattlefieldWG::RemoveAurasFromPlayer,
		// BattlefieldWG.cpp:1049-1059)
		sess.removeAura(WGSpellRecruit)
		sess.removeAura(WGSpellCorporal)
		sess.removeAura(WGSpellLieutenant)
		sess.removeAura(WGSpellTenacity)
		sess.removeAura(WGSpellTowerControl)
		sess.removeAura(WGSpellSpiritualImmunity)
		sess.removeAura(WGSpellRestrictedFlightArea)
		sess.removeAura(WGSpellGreatHonor)
		sess.removeAura(WGSpellGreaterHonor)
		sess.removeAura(WGSpellGreatestHonor)

		if pTeam == winningTeam {
			sess.applyAura(WGSpellEssenceOfWintergrasp)
			sess.applyAura(WGSpellVictoryReward)
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
		} else if pTeam == losingTeam {
			sess.removeAura(WGSpellEssenceOfWintergrasp)
			sess.applyAura(WGSpellDefeatReward)
		}
	}
	s.sessionsMu.RUnlock()

	// BattlefieldWG::OnBattleEnd clears m_PlayersInWar per team after the
	// aura strip above (BattlefieldWG.cpp:704-731); the zone roster
	// (PlayersInZone) survives, so the next StartBattle can repopulate
	// from it. Previously the clear was deferred because the war roster
	// doubled as the zone roster.
	// Outstanding invites and kick marks lapse with the battle: C++
	// leaves m_InvitedPlayers entries until leave-zone, but Go clears
	// them here so stale expiries cannot poison the next StartBattle's
	// war+invited count or fire a stale kick — accept is gated on
	// IsActive, so a lapsed invite can never be accepted anyway.
	wg.PlayersInWar = make(map[uint64]uint32)
	wg.InvitedPlayers = make(map[uint64]wgInvite)
	wg.PlayersWillBeKick = make(map[uint64]time.Time)

	// OnBattleEnd (BattlefieldWG.cpp:684-687): between battles every
	// zone graveyard returns to the defending team — indices below
	// GY_HORDE, i.e. the four workshop graveyards plus the keep
	// graveyard; the faction graveyards keep their team. DefenderTeam
	// was already swapped to the winner above on a relic capture,
	// matching C++ ("teams have already been swapped" at :676).
	for i := uint8(0); i < WGGraveyardHorde; i++ {
		wg.Graveyards[i].TeamControl = wg.DefenderTeam
	}

	if !endByTimer {
		// On a relic capture every player in the zone swaps to the new
		// defender's control phase shift (BattlefieldWG.cpp:747-757, the
		// m_players zone-roster loop); on a successful defense the shifts
		// are untouched. The old winner/loser arms applied the swap to
		// every session of the team world-wide, and the winner arm did so
		// even on a timer defense.
		s.sessionsMu.RLock()
		for sess := range s.sessions {
			if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != WGMapID || sess.player.Zone != WGZoneID {
				continue
			}
			if winningTeam == WGTeamAlliance {
				sess.removeAura(WGSpellHordeControlPhaseShift)
				sess.applyAura(WGSpellAllianceControlPhaseShift)
			} else {
				sess.removeAura(WGSpellAllianceControlPhaseShift)
				sess.applyAura(WGSpellHordeControlPhaseShift)
			}
		}
		s.sessionsMu.RUnlock()
	}

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
		s.broadcastWGZoneMessage(fmt.Sprintf("The %s has successfully defended Wintergrasp Fortress!", teamName))
	} else {
		teamName := "Alliance"
		if winningTeam == WGTeamHorde {
			teamName = "Horde"
		}
		s.broadcastWGZoneMessage(fmt.Sprintf("The %s has captured Wintergrasp Fortress!", teamName))
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
				// PromotePlayer's stalker SendChat (BattlefieldWG.cpp:1030):
				// routed through the row-driven creature-text path, so
				// the DB row's type/range decides the audience; the
				// sender is the promoted player (srcPlr = killer), hence
				// killerGUID as both source GUID and whisper target.
				// Instance 0 is the open-world Northrend instance.
				s.broadcastCreatureTalk(context.Background(), WGMapID, 0, killerGUID, WGCreatureStalker, sess.player.Name, WGTextRankCorporal, killerGUID)
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
				// PromotePlayer's stalker SendChat (BattlefieldWG.cpp:1042),
				// same row-driven path as the Corporal arm above.
				s.broadcastCreatureTalk(context.Background(), WGMapID, 0, killerGUID, WGCreatureStalker, sess.player.Name, WGTextRankFirstLieutenant, killerGUID)
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
				s.broadcastWGZoneMessage(fmt.Sprintf("%s is under attack!", wg.Towers[i].Name))
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
				s.broadcastWGZoneMessage(fmt.Sprintf("%s has been destroyed!", wg.Towers[i].Name))

				if wg.Towers[i].IsSouthTower {
					wg.BrokenSouthTowers++

					// Update tower control stacks (UpdatedDestroyedTowerCount,
					// BattlefieldWG.cpp:1204-1245): each south tower destroyed
					// adds one stack to defenders in war and removes one from
					// attackers, reaching m_PlayersInWar only. Same audience as
					// the credit loop below.
					for guid, pTeam := range wg.PlayersInWar {
						if sess := s.findSessionByGUID(guid); sess != nil {
							if pTeam == wg.DefenderTeam {
								s.setWGTowerControlStacks(sess, wgTowerControlStacks(sess)+1)
							} else {
								s.setWGTowerControlStacks(sess, wgTowerControlStacks(sess)-1)
							}
						}
					}

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

				// If Vault Gate breached, unlock Titan's Relic!
				if entry == WGGameObjectVaultGate {
					wg.RelicInteractible = true
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

	if int(workshopID) >= len(wg.Workshops) {
		wg.mu.Unlock()
		return false
	}
	// Keep workshops cannot be captured
	if workshopID == WGWorkshopKeepWest || workshopID == WGWorkshopKeepEast {
		wg.mu.Unlock()
		return false
	}

	ws := &wg.Workshops[workshopID]
	if ws.TeamControl == team {
		wg.mu.Unlock()
		return true // already controlled
	}
	// WintergraspWorkshop::GiveControlTo(TEAM_NEUTRAL)
	// (BattlefieldWG.cpp:1786): when a capture bar crosses the midpoint,
	// the workshop warns of the attack before any capture leg fires, and
	// the warning names the attacker as the opposite of the current
	// controller (controller Alliance -> HordeAttack text, otherwise
	// AllianceAttack). Go captures instantly on banner use, so the
	// midpoint warning collapses into the capture call, ahead of the
	// capture message below. Init captures never reach this function
	// (initWorkshops sets control directly), matching the C++ init=true
	// warning suppression.
	prevControl := ws.TeamControl
	attackerName := "Alliance"
	if prevControl == WGTeamAlliance {
		attackerName = "Horde"
	}
	s.broadcastWGZoneMessage(fmt.Sprintf("The %s is attacking %s!", attackerName, ws.Name))
	ws.TeamControl = team
	// WintergraspWorkshop::GiveControlTo (BattlefieldWG.cpp:1800-1802,
	// 1818-1820): a capture flips the linked graveyard, gated on
	// WorkshopId < KEEP_WEST, so keep workshops never reach this leg
	// (they also return false above). The index is the workshop enum
	// ID, crossed against the graveyard enum exactly as in C++ — see
	// updateGraveyardsFromWorkshops. The BfGraveyard::GiveControlTo
	// RelocateDeadPlayers leg runs below: ghosts queued at the flipped
	// graveyard (attributed through the SpiritGuides table) teleport
	// to the closest graveyard their team still controls.
	var relocate []uint64
	if workshopID < WGWorkshopKeepWest {
		wg.Graveyards[workshopID].TeamControl = team
		relocate = wg.queuedDeadAtGraveyard(workshopID)
	}
	s.updateWGWorldState(ws.WorldState, uint32(wgBuildingState(0, team)))
	wg.updateVehicleLimits()
	s.updateWGWorldState(WGWorldStateMaxVehicleA, wg.MaxVehiclesAlliance)
	s.updateWGWorldState(WGWorldStateMaxVehicleH, wg.MaxVehiclesHorde)

	teamName := "Alliance"
	if team == WGTeamHorde {
		teamName = "Horde"
	}
	s.broadcastWGZoneMessage(fmt.Sprintf("%s has captured %s for the %s!", capturerName, ws.Name, teamName))
	// Teleports run after unlock: the teleport's zone-change path takes
	// wg.mu (same ordering as the TickWG kick sweep).
	wg.mu.Unlock()
	s.relocateWGDeadPlayers(relocate)
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

	// Tenacity reaches m_PlayersInWar only (BattlefieldWG.cpp:1301+):
	// the counts above already come from PlayersInWar, but the old map
	// scan applied the buff to every same-team player on Northrend.
	for guid, pTeam := range wg.PlayersInWar {
		if sess := s.findSessionByGUID(guid); sess != nil {
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
	}
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
	// Wartime kick sweep (Battlefield::Update, Battlefield.cpp:165-186):
	// every second, kick expired invites, re-invite the zone to fill
	// freed slots, kick expired will-be-kick marks, and kick AFK war
	// members (KickAfkPlayers). C++ kicks in place during the sweep;
	// Go collects GUIDs under wg.mu and teleports after unlock, since
	// the teleport's zone-change path takes wg.mu. Expired invites are
	// erased at kick time (C++ leaves the entry until leave-zone, but
	// Go must free the slot for the same-pass re-invite); just-kicked
	// GUIDs are excluded from that re-invite.
	wg = s.getOrCreateWGState()
	wg.mu.Lock()
	var kickGUIDs []uint64
	// Peacetime grouping (Battlefield::Update, Battlefield.cpp:147-152):
	// once per peace period, when the timer drops to the 15-minute
	// grouping window, invite the whole zone roster to the queue and
	// fire OnStartGrouping (BattlefieldWG.cpp:789-792: SendWarning
	// BATTLEFIELD_WG_TEXT_START_GROUPING, id 27). The check runs after
	// the timer-expiry branch above, so a battle that just started or
	// just ended cannot also trigger it in the same tick; EndBattle
	// resets the flag and the fresh 150-minute peace timer is above
	// the window.
	grouping := false
	if !wg.IsActive && !wg.StartGrouping && wg.Timer <= WGStartGroupingDuration {
		wg.StartGrouping = true
		s.inviteWGZoneToQueue(wg)
		grouping = true
	}
	if wg.IsActive {
		wg.KickSweepTimer -= delta
		if wg.KickSweepTimer <= 0 {
			wg.KickSweepTimer = wgKickSweepInterval
			now := time.Now()
			kicked := map[uint64]bool{}
			for guid, inv := range wg.InvitedPlayers {
				if !inv.Deadline.After(now) {
					delete(wg.InvitedPlayers, guid)
					kicked[guid] = true
					kickGUIDs = append(kickGUIDs, guid)
				}
			}
			s.inviteWGZoneToWar(wg, kicked)
			for guid, deadline := range wg.PlayersWillBeKick {
				if !deadline.After(now) {
					delete(wg.PlayersWillBeKick, guid)
					if !kicked[guid] {
						kicked[guid] = true
						kickGUIDs = append(kickGUIDs, guid)
					}
				}
			}
			for guid := range wg.PlayersInWar {
				if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil && sess.player.PlayerFlags&playerFlagAFK != 0 {
					if !kicked[guid] {
						kicked[guid] = true
						kickGUIDs = append(kickGUIDs, guid)
					}
				}
			}
		}
	}
	wg.mu.Unlock()
	for _, guid := range kickGUIDs {
		if sess := s.findSessionByGUID(guid); sess != nil {
			s.kickWGPlayerFromBattlefield(sess)
		}
	}
	if grouping {
		s.broadcastWGZoneMessage("Wintergrasp grouping has begun — the battle starts in 15 minutes!")
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

// wgResurrectGuide resolves the SPELL_SPIRIT_HEAL caster and graveyard for
// one queued ghost: the graveyard is the one holding the guide the ghost
// queued at (BfGraveyard::HasNpc, Battlefield.cpp:760-770), and the caster
// is that graveyard's control-team guide (Resurrect, :711-712). A neutral
// or never-assigned control guide falls back to the queued-at guide — Go
// assigns a guide only when a ghost first queues at it, so an unassigned
// control guide has no Go GUID, but the queued-at guide stands at the same
// graveyard. The boolean reports whether the graveyard resolved; without
// it the caller centers the cast on the ghost.
func wgResurrectGuide(guides [WGMaxGraveyards][2]uint64, graves [WGMaxGraveyards]wgGraveyardState, spiritGUID uint64) (uint64, uint8, bool) {
	for gy := uint8(0); gy < WGMaxGraveyards; gy++ {
		if guides[gy][WGTeamAlliance] == spiritGUID || guides[gy][WGTeamHorde] == spiritGUID {
			if control := graves[gy].TeamControl; control == WGTeamAlliance || control == WGTeamHorde {
				if guide := guides[gy][control]; guide != 0 {
					return guide, gy, true
				}
			}
			return spiritGUID, gy, true
		}
	}
	return spiritGUID, 0, false
}

// broadcastWGSpiritHeal sends the guide's SPELL_SPIRIT_HEAL SMSG_SPELL_GO
// (Battlefield.cpp:712, triggered self-cast: GO only, no SPELL_START) to
// the guide's visibility set. Guides never enter motion, so no motion
// lookup can place them; the cast centers on the graveyard SafeLoc —
// where the guide stands — and falls back to the ghost's position when
// the graveyard or its SafeLoc row does not resolve. safeLocID comes from
// the caller's graveyard snapshot, so no lock is taken here.
func (s *Server) broadcastWGSpiritHeal(guideGUID uint64, safeLocID uint32, gyOK bool, sess *session) {
	if s == nil || sess == nil || sess.player == nil || guideGUID == 0 {
		return
	}
	mapID, instanceID := WGMapID, sess.player.InstanceID
	x, y, z := sess.player.X, sess.player.Y, sess.player.Z
	if gyOK && s.Data != nil {
		if loc, found, err := s.Data.WorldSafeLoc(safeLocID); err == nil && found {
			x, y, z = loc.X, loc.Y, loc.Z
		}
	}
	pkt := protocol.NewBuffer(16)
	pkt.WritePackedGUID(guideGUID)
	pkt.WritePackedGUID(guideGUID)
	pkt.WriteU8(1)
	pkt.WriteU32(22012) // SPELL_SPIRIT_HEAL (Battleground.h:133)
	pkt.WriteU32(0)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELL_GO), pkt.Bytes(), mapID, instanceID, x, y, z, nil)
}

// resurrectWGQueuedPlayers mirrors BfGraveyard::Resurrect
// (Battlefield.cpp:697-723): the graveyard's spirit guide casts
// SPELL_SPIRIT_HEAL (22012) on itself for each in-world queued ghost, then
// the ghost gains the resurrection visual, ResurrectPlayer(1.0),
// 6962 + SPELL_SPIRIT_HEAL_MANA (44535), and its corpse becomes bones.
// The no-longer-a-ghost skip is Go-original: C++ has no such check, but the
// queue only ever holds ghosts.
func (s *Server) resurrectWGQueuedPlayers(ctx context.Context, queued map[uint64]uint64) {
	wg := s.getOrCreateWGState()
	wg.mu.Lock()
	guides, graves := wg.SpiritGuides, wg.Graveyards
	wg.mu.Unlock()
	for pGUID, spiritGUID := range queued {
		sess := s.findSessionByGUID(pGUID)
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.PlayerFlags&playerFlagGhost == 0 {
			continue
		}
		guideGUID, gy, gyOK := wgResurrectGuide(guides, graves, spiritGUID)
		var safeLocID uint32
		if gyOK {
			safeLocID = graves[gy].SafeLocID
		}
		s.broadcastWGSpiritHeal(guideGUID, safeLocID, gyOK, sess)
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
	// Battlefield::HandlePlayerEnterZone (Battlefield.cpp:80-102) always
	// adds the player to the zone roster m_players, but in wartime only
	// invites to the war (InvitePlayerToWar); a full battlefield marks
	// the entrant for kick and invites them to the queue instead. Go
	// invites (no direct war insert, so tenacity/tower loops only see
	// accepters); in peacetime with 15 minutes or less left, the entrant
	// is invited to the queue instead (:96-97). The war invite's
	// in-flight, arena/BG, and min-level guards live in
	// inviteWGPlayerToWar and apply on this path too.
	wg.PlayersInZone[sess.playerGUID] = playerTeam
	if wg.IsActive {
		if _, inWar := wg.PlayersInWar[sess.playerGUID]; !inWar {
			if _, invited := wg.InvitedPlayers[sess.playerGUID]; !invited {
				if wg.warAndInvitedCount(playerTeam) < wgMaxPlayersPerTeam {
					s.inviteWGPlayerToWar(wg, sess, playerTeam)
				} else {
					wg.PlayersWillBeKick[sess.playerGUID] = time.Now().Add(wgKickDelay)
					s.inviteWGPlayerToQueue(wg, sess, playerTeam)
				}
			}
		}
	} else if wg.Timer <= WGStartGroupingDuration {
		s.inviteWGPlayerToQueue(wg, sess, playerTeam)
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
	// Battlefield::HandlePlayerLeaveZone (Battlefield.cpp:108-118) sends
	// SendBfLeaveMessage (BattlefieldHandler.cpp:106-116: u32 battleId,
	// u8 reason, u8(2) BattleStatus, u8(0) Relocated = 7 bytes) when a
	// wartime war member leaves the zone, before the raid-group removal
	// and OnPlayerLeaveWar; the default reason is BF_LEAVE_REASON_EXITED
	// (WorldSession.h:267, 0x08). The send sits here, after the war
	// erase and before the aura strips below, matching that order.
	_, wasInWar := wg.PlayersInWar[sess.playerGUID]
	wasActive := wg.IsActive
	delete(wg.PlayersInWar, sess.playerGUID)
	if wasInWar && wasActive {
		buf := protocol.NewBuffer(7)
		buf.WriteU32(WGBattleID)
		buf.WriteU8(8)
		buf.WriteU8(2)
		buf.WriteU8(0)
		_ = sess.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_EJECTED), buf.Bytes(), true)
	}
	delete(wg.PlayersInZone, sess.playerGUID)
	// Battlefield::HandlePlayerLeaveZone (Battlefield.cpp:126-127) erases
	// the invited and will-be-kick entries alongside the rosters.
	delete(wg.InvitedPlayers, sess.playerGUID)
	delete(wg.PlayersWillBeKick, sess.playerGUID)
	// Battlefield::HandlePlayerLeaveZone (Battlefield.cpp:129) also drops
	// the revive-queue entry; BfGraveyard::RemovePlayer (:688-694) strips
	// SPELL_WAITING_FOR_RESURRECT with it, so a ghost that leaves the zone
	// is not resurrected by the next wave and keeps no waiting aura.
	_, queued := wg.ReviveQueue[sess.playerGUID]
	if queued {
		delete(wg.ReviveQueue, sess.playerGUID)
	}

	sess.removeAura(WGSpellRecruit)
	sess.removeAura(WGSpellCorporal)
	sess.removeAura(WGSpellLieutenant)
	sess.removeAura(WGSpellTenacity)
	sess.removeAura(WGSpellTowerControl)
	sess.removeAura(WGSpellGreatHonor)
	sess.removeAura(WGSpellGreaterHonor)
	sess.removeAura(WGSpellGreatestHonor)
	if queued {
		sess.removeAura(2584) // SPELL_WAITING_FOR_RESURRECT
	}
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

	killerTeam := teamForRace(killerSess.player.Race)
	victimTeam := teamForRace(victimSess.player.Race)
	if killerTeam == victimTeam {
		return
	}

	// BattlefieldWG::HandlePromotion: every in-war player of the killer's
	// team within 40 (2D) yards of the victim shares the promotion credit.
	// PromotePlayer itself gates on war active; the skinnable arm below
	// does not, so the loop — not the whole handler — carries the gate.
	if wg.IsActive {
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

	// BattlefieldWG::HandleKill (BattlefieldWG.cpp:962-977): a slain
	// player in the zone becomes skinnable so the insignia can be
	// removed. Same bare flag set as the battleground-victim arm in
	// death.go; repop clears it there.
	victimSess.player.UnitFlags |= unitFlagSkinnable
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
			PlayersInZone:      make(map[uint64]uint32),
			PlayersInQueue:     make(map[uint64]uint32),
			InvitedPlayers:     make(map[uint64]wgInvite),
			PlayersWillBeKick:  make(map[uint64]time.Time),
			KickSweepTimer:     wgKickSweepInterval,
			PlayerRanks:        make(map[uint64]uint32),
			PlayerKillsInRank:  make(map[uint64]uint32),
			Vehicles:           make(map[uint64]uint32),
			ReviveQueue:        make(map[uint64]uint64),
			LastResurrectTimer: wgResurrectInterval,
		}
		wg.initWorkshops()
		wg.initGraveyards()
		wg.initTowers()
		wg.initGates()
		wg.updateVehicleLimits()
		s.wgState = wg
	}
	return s.wgState
}

// updateWGWorldState sends an SMSG_UPDATE_WORLD_STATE packet to players in Wintergrasp.
// Reference: Battlefield::SendUpdateWorldState (Battlefield.cpp:465-471)
// iterates m_players, the zone roster — not every player on map 571, so
// the old map gate spammed all of Northrend. The zone gate is the live
// player.Zone (recomputed on map change and zone tick).
func (s *Server) updateWGWorldState(variableID, value uint32) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == WGMapID && sess.player.Zone == WGZoneID {
			sess.sendWorldState(variableID, value)
		}
	}
}

// broadcastWGInitWorldStates sends SMSG_INIT_WORLD_STATES to all players in Wintergrasp.
// Same zone-roster audience as updateWGWorldState (Battlefield.cpp:465-471).
func (s *Server) broadcastWGInitWorldStates() {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == WGMapID && sess.player.Zone == WGZoneID {
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
