package world

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Isle of Conquest (IoC) Constants mirroring TrinityCore BattlegroundIC.h / BattlegroundIC.cpp.
const (
	ICMapID uint32 = 628

	ICMaxReinforcements uint32 = 300

	// ICWinnerHonorAmount mirrors WINNER_HONOR_AMOUNT (BattlegroundIC.h:907):
	// raw honor granted to the winning team on a boss kill.
	ICWinnerHonorAmount uint32 = 500

	ICDefaultBannerCaptureDuration = 60 * time.Second // 1 minute
	ICResourceTickDuration         = 45 * time.Second // 45 seconds

	// Teams
	ICTeamAlliance uint32 = 0
	ICTeamHorde    uint32 = 1
	ICTeamNeutral  uint32 = 2

	// Node Types (0..6)
	ICNodeRefinery   uint8 = 0
	ICNodeQuarry     uint8 = 1
	ICNodeDocks      uint8 = 2
	ICNodeHangar     uint8 = 3
	ICNodeWorkshop   uint8 = 4
	ICNodeGraveyardA uint8 = 5 // Alliance Keep Graveyard
	ICNodeGraveyardH uint8 = 6 // Horde Keep Graveyard
	ICMaxNodes             = 7

	// Node States
	ICNodeStateUncontrolled uint8 = 0
	ICNodeStateConflictA    uint8 = 1
	ICNodeStateConflictH    uint8 = 2
	ICNodeStateControlledA  uint8 = 3
	ICNodeStateControlledH  uint8 = 4
)

// BG_IC_GraveyardIds (BattlegroundIC.h:833): world_safe_locs IDs per node
// type (indices 0..6; refinery/quarry are 0 = no grave), plus the per-team
// "last resort" fallback graves (indices 7..8 = teamIndex + MAX_NODE_TYPES).
var ICGraveyardIDs = [ICMaxNodes + 2]uint32{0, 0, 1480, 1481, 1482, 1485, 1486, 1483, 1484}

const (

	// Gate States
	ICGateOK        uint8 = 1
	ICGateDamaged   uint8 = 2
	ICGateDestroyed uint8 = 3

	// Doors / Gates (0..5)
	ICHordeFrontGate    uint8 = 0
	ICHordeWestGate     uint8 = 1
	ICHordeEastGate     uint8 = 2
	ICAllianceFrontGate uint8 = 3
	ICAllianceWestGate  uint8 = 4
	ICAllianceEastGate  uint8 = 5
	ICMaxGates                = 6

	// Bosses & Creatures
	ICCreatureHalfordWyrmbane uint32 = 34924 // Alliance Boss
	ICCreatureOverlordAgmar   uint32 = 34922 // Horde Boss
	ICCreatureKeepCannon      uint32 = 34944
	ICCreatureDemolisher      uint32 = 34775
	ICCreatureSiegeEngineA    uint32 = 34776
	ICCreatureSiegeEngineH    uint32 = 35069
	ICCreatureGlaiveThrowerA  uint32 = 34802
	ICCreatureGlaiveThrowerH  uint32 = 35273
	ICCreatureCatapult        uint32 = 34793

	// Gate GameObjects
	ICGameObjectGateHordeFront    uint32 = 195494
	ICGameObjectGateHordeWest     uint32 = 195496
	ICGameObjectGateHordeEast     uint32 = 195495
	ICGameObjectGateAllianceFront uint32 = 195700
	ICGameObjectGateAllianceWest  uint32 = 195699
	ICGameObjectGateAllianceEast  uint32 = 195698

	// Banner GameObjects
	// Base banners
	ICGameObjectBannerRefinery uint32 = 195343
	ICGameObjectBannerQuarry   uint32 = 195338
	ICGameObjectBannerDocks    uint32 = 195157
	ICGameObjectBannerHangar   uint32 = 195158
	ICGameObjectBannerWorkshop uint32 = 195133
	ICGameObjectBannerKeepA    uint32 = 195396
	ICGameObjectBannerKeepH    uint32 = 195393

	// Docks Banners
	ICGameObjectBannerDocksA     uint32 = 195153
	ICGameObjectBannerDocksACont uint32 = 195154
	ICGameObjectBannerDocksH     uint32 = 195155
	ICGameObjectBannerDocksHCont uint32 = 195156

	// Hangar Banners
	ICGameObjectBannerHangarA     uint32 = 195132
	ICGameObjectBannerHangarACont uint32 = 195144
	ICGameObjectBannerHangarH     uint32 = 195130
	ICGameObjectBannerHangarHCont uint32 = 195145

	// Quarry Banners
	ICGameObjectBannerQuarryA     uint32 = 195334
	ICGameObjectBannerQuarryACont uint32 = 195335
	ICGameObjectBannerQuarryH     uint32 = 195336
	ICGameObjectBannerQuarryHCont uint32 = 195337

	// Refinery Banners
	ICGameObjectBannerRefineryA     uint32 = 195339
	ICGameObjectBannerRefineryACont uint32 = 195340
	ICGameObjectBannerRefineryH     uint32 = 195341
	ICGameObjectBannerRefineryHCont uint32 = 195342

	// Workshop Banners
	ICGameObjectBannerWorkshopA     uint32 = 195149
	ICGameObjectBannerWorkshopACont uint32 = 195150
	ICGameObjectBannerWorkshopH     uint32 = 195151
	ICGameObjectBannerWorkshopHCont uint32 = 195152

	// Graveyard A Banners
	ICGameObjectBannerGraveyardA_A     uint32 = 195396
	ICGameObjectBannerGraveyardA_ACont uint32 = 195397
	ICGameObjectBannerGraveyardA_H     uint32 = 195398
	ICGameObjectBannerGraveyardA_HCont uint32 = 195399

	// Graveyard H Banners
	ICGameObjectBannerGraveyardH_A     uint32 = 195391
	ICGameObjectBannerGraveyardH_ACont uint32 = 195392
	ICGameObjectBannerGraveyardH_H     uint32 = 195393
	ICGameObjectBannerGraveyardH_HCont uint32 = 195394

	// Bombs & Teleporters
	ICGameObjectSeaforiumBombs uint32 = 195237
	ICGameObjectHugeBombA      uint32 = 195332
	ICGameObjectHugeBombH      uint32 = 195333

	// World States
	ICWorldStateAllianceReinforcementsSet uint32 = 4221
	ICWorldStateHordeReinforcementsSet    uint32 = 4222
	ICWorldStateAllianceReinforcements    uint32 = 4226
	ICWorldStateHordeReinforcements       uint32 = 4227

	// Gate WorldStates (Closed)
	ICWorldStateGateHordeFrontClosed    uint32 = 4317
	ICWorldStateGateHordeWestClosed     uint32 = 4318
	ICWorldStateGateHordeEastClosed     uint32 = 4319
	ICWorldStateGateAllianceFrontClosed uint32 = 4328
	ICWorldStateGateAllianceWestClosed  uint32 = 4327
	ICWorldStateGateAllianceEastClosed  uint32 = 4326

	// Gate WorldStates (Open / Destroyed)
	ICWorldStateGateHordeFrontOpen    uint32 = 4322
	ICWorldStateGateHordeWestOpen     uint32 = 4321
	ICWorldStateGateHordeEastOpen     uint32 = 4320
	ICWorldStateGateAllianceFrontOpen uint32 = 4323
	ICWorldStateGateAllianceWestOpen  uint32 = 4324
	ICWorldStateGateAllianceEastOpen  uint32 = 4325

	// Node WorldStates (Uncontrolled, Conflict A, Conflict H, Controlled A, Controlled H)
	ICWorldStateDocksUncontrolled uint32 = 4301
	ICWorldStateDocksConflictA    uint32 = 4305
	ICWorldStateDocksConflictH    uint32 = 4302
	ICWorldStateDocksControlledA  uint32 = 4304
	ICWorldStateDocksControlledH  uint32 = 4303

	ICWorldStateHangarUncontrolled uint32 = 4296
	ICWorldStateHangarConflictA    uint32 = 4300
	ICWorldStateHangarConflictH    uint32 = 4297
	ICWorldStateHangarControlledA  uint32 = 4299
	ICWorldStateHangarControlledH  uint32 = 4298

	ICWorldStateQuarryUncontrolled uint32 = 4306
	ICWorldStateQuarryConflictA    uint32 = 4310
	ICWorldStateQuarryConflictH    uint32 = 4307
	ICWorldStateQuarryControlledA  uint32 = 4309
	ICWorldStateQuarryControlledH  uint32 = 4308

	ICWorldStateRefineryUncontrolled uint32 = 4311
	ICWorldStateRefineryConflictA    uint32 = 4315
	ICWorldStateRefineryConflictH    uint32 = 4312
	ICWorldStateRefineryControlledA  uint32 = 4314
	ICWorldStateRefineryControlledH  uint32 = 4313

	ICWorldStateWorkshopUncontrolled uint32 = 4294
	ICWorldStateWorkshopConflictA    uint32 = 4228
	ICWorldStateWorkshopConflictH    uint32 = 4293
	ICWorldStateWorkshopControlledA  uint32 = 4229
	ICWorldStateWorkshopControlledH  uint32 = 4230

	ICWorldStateAllianceKeepUncontrolled uint32 = 4341
	ICWorldStateAllianceKeepConflictA    uint32 = 4342
	ICWorldStateAllianceKeepConflictH    uint32 = 4343
	ICWorldStateAllianceKeepControlledA  uint32 = 4339
	ICWorldStateAllianceKeepControlledH  uint32 = 4340

	ICWorldStateHordeKeepUncontrolled uint32 = 4346
	ICWorldStateHordeKeepConflictA    uint32 = 4347
	ICWorldStateHordeKeepConflictH    uint32 = 4348
	ICWorldStateHordeKeepControlledA  uint32 = 4344
	ICWorldStateHordeKeepControlledH  uint32 = 4345

	// Spells
	ICSpellOilRefinery = 68719
	ICSpellQuarry      = 68720
	ICSpellParachute   = 66656

	// BattlegroundIC.h Spells: achievement credit spells cast triggered on the
	// killer (Mowed Down, 68357) and on the player at the back-door trigger
	// (Back Door Job, 68502).
	ICSpellDestroyedVehicleAchievement uint32 = 68357
	ICSpellBackDoorJobAchievement      uint32 = 68502

	// BattlegroundIC.h IC_OBJECTIVE_ASSAULT_BASE / IC_OBJECTIVE_DEFEND_BASE:
	// BG_OBJECTIVE_CAPTURE objective ids credited on assault/defend clicks.
	ICObjectiveAssaultBase uint32 = 245
	ICObjectiveDefendBase  uint32 = 246

	// BattlegroundIC.h RESOURCE_HONOR_AMOUNT: honor per resource tick.
	ICResourceHonorAmount uint32 = 12

	// BattlegroundIC::HandleAreaTrigger trigger ids: 5555 is the Horde
	// back-door trigger (Alliance keep), 5535 the Alliance one (Horde keep).
	ICAreaTriggerBackDoorHorde    uint32 = 5555
	ICAreaTriggerBackDoorAlliance uint32 = 5535

	ICGateDefaultMaxHealth uint32 = 120000
)

var icNodeNames = [ICMaxNodes]string{
	"the Refinery",
	"the Quarry",
	"the Docks",
	"the Hangar",
	"the Workshop",
	"the Alliance Keep Graveyard",
	"the Horde Keep Graveyard",
}

type icNodeWorldStatesInfo struct {
	Uncontrolled uint32
	ConflictA    uint32
	ConflictH    uint32
	ControlledA  uint32
	ControlledH  uint32
}

var icNodeWorldStates = [ICMaxNodes]icNodeWorldStatesInfo{
	{ICWorldStateRefineryUncontrolled, ICWorldStateRefineryConflictA, ICWorldStateRefineryConflictH, ICWorldStateRefineryControlledA, ICWorldStateRefineryControlledH},
	{ICWorldStateQuarryUncontrolled, ICWorldStateQuarryConflictA, ICWorldStateQuarryConflictH, ICWorldStateQuarryControlledA, ICWorldStateQuarryControlledH},
	{ICWorldStateDocksUncontrolled, ICWorldStateDocksConflictA, ICWorldStateDocksConflictH, ICWorldStateDocksControlledA, ICWorldStateDocksControlledH},
	{ICWorldStateHangarUncontrolled, ICWorldStateHangarConflictA, ICWorldStateHangarConflictH, ICWorldStateHangarControlledA, ICWorldStateHangarControlledH},
	{ICWorldStateWorkshopUncontrolled, ICWorldStateWorkshopConflictA, ICWorldStateWorkshopConflictH, ICWorldStateWorkshopControlledA, ICWorldStateWorkshopControlledH},
	{ICWorldStateAllianceKeepUncontrolled, ICWorldStateAllianceKeepConflictA, ICWorldStateAllianceKeepConflictH, ICWorldStateAllianceKeepControlledA, ICWorldStateAllianceKeepControlledH},
	{ICWorldStateHordeKeepUncontrolled, ICWorldStateHordeKeepConflictA, ICWorldStateHordeKeepConflictH, ICWorldStateHordeKeepControlledA, ICWorldStateHordeKeepControlledH},
}

type icGateInfo struct {
	GateID       uint8
	GameObjectId uint32
	ClosedWS     uint32
	OpenWS       uint32
	Name         string
}

var icGates = [ICMaxGates]icGateInfo{
	{ICHordeFrontGate, ICGameObjectGateHordeFront, ICWorldStateGateHordeFrontClosed, ICWorldStateGateHordeFrontOpen, "Horde Front Gate"},
	{ICHordeWestGate, ICGameObjectGateHordeWest, ICWorldStateGateHordeWestClosed, ICWorldStateGateHordeWestOpen, "Horde West Gate"},
	{ICHordeEastGate, ICGameObjectGateHordeEast, ICWorldStateGateHordeEastClosed, ICWorldStateGateHordeEastOpen, "Horde East Gate"},
	{ICAllianceFrontGate, ICGameObjectGateAllianceFront, ICWorldStateGateAllianceFrontClosed, ICWorldStateGateAllianceFrontOpen, "Alliance Front Gate"},
	{ICAllianceWestGate, ICGameObjectGateAllianceWest, ICWorldStateGateAllianceWestClosed, ICWorldStateGateAllianceWestOpen, "Alliance West Gate"},
	{ICAllianceEastGate, ICGameObjectGateAllianceEast, ICWorldStateGateAllianceEastClosed, ICWorldStateGateAllianceEastOpen, "Alliance East Gate"},
}

type icGateState struct {
	State     uint8 // 1: OK, 2: Damaged, 3: Destroyed
	Health    uint32
	MaxHealth uint32
}

type icNodeState struct {
	NodeType     uint8
	Faction      uint32 // 0: Alliance, 1: Horde, 2: Neutral
	State        uint8  // 0: Uncontrolled, 1: ConflictA, 2: ConflictH, 3: ControlledA, 4: ControlledH
	Timer        time.Duration
	CaptureTimer *time.Timer
	Banners      [4]uint32 // ControlledA, ContestedA, ControlledH, ContestedH
	// ContestTeam mirrors BattlegroundIC's nodePoint.faction during the 60s
	// contested window: the team that clicked last. C++ sets faction to the
	// contesting team on click (EventPlayerClickedOnFlag) and GetClosestGraveyard
	// keys off it, so the contesting team's ghosts use the node GY while the
	// banner is grey. ICTeamNeutral when no contest is running.
	ContestTeam uint32
}

// closestICGraveyard mirrors BattlegroundIC::GetClosestGraveyard: every node
// owned by the player's team is a candidate via its ICGraveyardIDs entry, the
// closest by 2D distance wins, and the per-team fallback grave applies when
// the team owns no node. A false return lets the generic zone path run (C++
// always returns an entry; Go falls back when the DBC rows are missing).
func (s *Server) closestICGraveyard(x, y float32, team uint32) (wotlk.WorldSafeLoc, bool) {
	var teamIndex uint32
	switch team {
	case teamAlliance:
		teamIndex = ICTeamAlliance
	case teamHorde:
		teamIndex = ICTeamHorde
	default:
		return wotlk.WorldSafeLoc{}, false
	}
	if s == nil || s.Data == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	ic := s.getOrCreateICState(ICMapID)
	if ic == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	ic.mu.Lock()
	factions := make([]uint32, ICMaxNodes)
	for i := range ic.Nodes {
		factions[i] = ic.Nodes[i].Faction
		// BattlegroundIC::EventPlayerClickedOnFlag sets nodePoint.faction to the
		// contesting team on click, and GetClosestGraveyard keys off it: while
		// the banner is contested, the contesting team's ghosts use the node GY.
		if st := ic.Nodes[i].State; (st == ICNodeStateConflictA || st == ICNodeStateConflictH) && ic.Nodes[i].ContestTeam != ICTeamNeutral {
			factions[i] = ic.Nodes[i].ContestTeam
		}
	}
	ic.mu.Unlock()

	var best wotlk.WorldSafeLoc
	bestDist := float32(999999.0)
	found := false
	for i, faction := range factions {
		if faction != teamIndex {
			continue
		}
		id := ICGraveyardIDs[i]
		if id == 0 {
			continue
		}
		loc, ok, err := s.Data.WorldSafeLoc(id)
		if err != nil || !ok {
			continue
		}
		dx, dy := loc.X-x, loc.Y-y
		if dist := dx*dx + dy*dy; !found || dist < bestDist {
			best, bestDist, found = loc, dist, true
		}
	}
	if found {
		return best, true
	}
	loc, ok, err := s.Data.WorldSafeLoc(ICGraveyardIDs[ICMaxNodes+teamIndex])
	if err != nil || !ok {
		return wotlk.WorldSafeLoc{}, false
	}
	return loc, true
}

type icBattlegroundState struct {
	mu                     sync.Mutex
	MapID                  uint32
	AllianceReinforcements uint32
	HordeReinforcements    uint32
	AllianceBossAlive      bool
	HordeBossAlive         bool
	Gates                  [ICMaxGates]icGateState
	Nodes                  [ICMaxNodes]icNodeState
	BannerCaptureDuration  time.Duration
	ResourceTickDuration   time.Duration
	Winner                 int8      // -1: ongoing, 0: Alliance, 1: Horde
	EndedAt                time.Time // EndBattleground time for the _ProcessLeave auto-remove (Battleground.cpp:506-528, TIME_TO_AUTOREMOVE = 120s)
	MatchStartTime         time.Time // Go form of C++ m_StartTime (Battleground.cpp:222, accumulated elapsed): armed at state creation, since Go has no StartBattleground/STATUS_IN_PROGRESS transition for this map; the restore slot anchors its elapsed clock here instead of the login time.
	PrematureCountDown     bool      // == m_PrematureCountDown (Battleground.cpp:359): a roster-collapse countdown is armed
	PrematureTimerMs       int64     // == m_PrematureCountDownTimer: ms left on the premature-finish countdown
	VehiclesDestroyed      map[uint64]uint32
	StopTicker             chan struct{}
	ResourceAccumMs        int64 // ms accumulated toward the next 45s resource tick
}

func isICGameObject(entry uint32) bool {
	switch entry {
	case ICGameObjectGateHordeFront,
		ICGameObjectGateHordeWest,
		ICGameObjectGateHordeEast,
		ICGameObjectGateAllianceFront,
		ICGameObjectGateAllianceWest,
		ICGameObjectGateAllianceEast,
		ICGameObjectBannerRefinery,
		ICGameObjectBannerQuarry,
		ICGameObjectBannerDocks,
		ICGameObjectBannerHangar,
		ICGameObjectBannerWorkshop,
		ICGameObjectBannerKeepA,
		ICGameObjectBannerKeepH,
		ICGameObjectBannerDocksA,
		ICGameObjectBannerDocksACont,
		ICGameObjectBannerDocksH,
		ICGameObjectBannerDocksHCont,
		ICGameObjectBannerHangarA,
		ICGameObjectBannerHangarACont,
		ICGameObjectBannerHangarH,
		ICGameObjectBannerHangarHCont,
		ICGameObjectBannerQuarryA,
		ICGameObjectBannerQuarryACont,
		ICGameObjectBannerQuarryH,
		ICGameObjectBannerQuarryHCont,
		ICGameObjectBannerRefineryA,
		ICGameObjectBannerRefineryACont,
		ICGameObjectBannerRefineryH,
		ICGameObjectBannerRefineryHCont,
		ICGameObjectBannerWorkshopA,
		ICGameObjectBannerWorkshopACont,
		ICGameObjectBannerWorkshopH,
		ICGameObjectBannerWorkshopHCont,
		ICGameObjectBannerGraveyardA_ACont,
		ICGameObjectBannerGraveyardA_H,
		ICGameObjectBannerGraveyardA_HCont,
		ICGameObjectBannerGraveyardH_A,
		ICGameObjectBannerGraveyardH_ACont,
		ICGameObjectBannerGraveyardH_HCont,
		ICGameObjectSeaforiumBombs,
		ICGameObjectHugeBombA,
		ICGameObjectHugeBombH:
		return true
	}
	return false
}

func (s *Server) getOrCreateICState(mapID uint32) *icBattlegroundState {
	if s == nil {
		return nil
	}
	s.icMu.Lock()
	defer s.icMu.Unlock()
	if s.icState == nil {
		s.icState = make(map[uint32]*icBattlegroundState)
	}
	state := s.icState[mapID]
	if state == nil {
		state = &icBattlegroundState{
			MapID:                  mapID,
			AllianceReinforcements: ICMaxReinforcements,
			HordeReinforcements:    ICMaxReinforcements,
			AllianceBossAlive:      true,
			HordeBossAlive:         true,
			BannerCaptureDuration:  ICDefaultBannerCaptureDuration,
			ResourceTickDuration:   ICResourceTickDuration,
			MatchStartTime:         time.Now(),
			Winner:                 -1,
			VehiclesDestroyed:      make(map[uint64]uint32),
			StopTicker:             make(chan struct{}),
		}

		// Initialize gates
		for i := 0; i < ICMaxGates; i++ {
			state.Gates[i] = icGateState{
				State:     ICGateOK,
				Health:    ICGateDefaultMaxHealth,
				MaxHealth: ICGateDefaultMaxHealth,
			}
		}

		// Initialize nodes
		state.initNodes()

		s.icState[mapID] = state
	}
	return state
}

// deleteICState removes the per-map IoC state once no BG player remains on
// the map (deleteBattlegroundStateOnEmpty). Node capture timers stop under
// ic.mu before the delete so no armed callback fires on the orphan; the
// delete only lands if the entry is still this state. Registry and state
// locks are never held together here, for the same reason as
// deleteWSGState.
func (s *Server) deleteICState(mapID uint32) {
	if s == nil {
		return
	}
	s.icMu.RLock()
	ic := s.icState[mapID]
	s.icMu.RUnlock()
	if ic == nil {
		return
	}
	ic.mu.Lock()
	for i := range ic.Nodes {
		if ic.Nodes[i].CaptureTimer != nil {
			ic.Nodes[i].CaptureTimer.Stop()
			ic.Nodes[i].CaptureTimer = nil
		}
	}
	ic.mu.Unlock()
	s.icMu.Lock()
	if s.icState[mapID] == ic {
		delete(s.icState, mapID)
	}
	s.icMu.Unlock()
}

func (ic *icBattlegroundState) initNodes() {
	// 0: Refinery
	ic.Nodes[ICNodeRefinery] = icNodeState{
		NodeType:    ICNodeRefinery,
		Faction:     ICTeamNeutral,
		State:       ICNodeStateUncontrolled,
		Banners:     [4]uint32{ICGameObjectBannerRefineryA, ICGameObjectBannerRefineryACont, ICGameObjectBannerRefineryH, ICGameObjectBannerRefineryHCont},
		ContestTeam: ICTeamNeutral,
	}
	// 1: Quarry
	ic.Nodes[ICNodeQuarry] = icNodeState{
		NodeType:    ICNodeQuarry,
		Faction:     ICTeamNeutral,
		State:       ICNodeStateUncontrolled,
		Banners:     [4]uint32{ICGameObjectBannerQuarryA, ICGameObjectBannerQuarryACont, ICGameObjectBannerQuarryH, ICGameObjectBannerQuarryHCont},
		ContestTeam: ICTeamNeutral,
	}
	// 2: Docks
	ic.Nodes[ICNodeDocks] = icNodeState{
		NodeType:    ICNodeDocks,
		Faction:     ICTeamNeutral,
		State:       ICNodeStateUncontrolled,
		Banners:     [4]uint32{ICGameObjectBannerDocksA, ICGameObjectBannerDocksACont, ICGameObjectBannerDocksH, ICGameObjectBannerDocksHCont},
		ContestTeam: ICTeamNeutral,
	}
	// 3: Hangar
	ic.Nodes[ICNodeHangar] = icNodeState{
		NodeType:    ICNodeHangar,
		Faction:     ICTeamNeutral,
		State:       ICNodeStateUncontrolled,
		Banners:     [4]uint32{ICGameObjectBannerHangarA, ICGameObjectBannerHangarACont, ICGameObjectBannerHangarH, ICGameObjectBannerHangarHCont},
		ContestTeam: ICTeamNeutral,
	}
	// 4: Workshop
	ic.Nodes[ICNodeWorkshop] = icNodeState{
		NodeType:    ICNodeWorkshop,
		Faction:     ICTeamNeutral,
		State:       ICNodeStateUncontrolled,
		Banners:     [4]uint32{ICGameObjectBannerWorkshopA, ICGameObjectBannerWorkshopACont, ICGameObjectBannerWorkshopH, ICGameObjectBannerWorkshopHCont},
		ContestTeam: ICTeamNeutral,
	}
	// 5: Graveyard A (Alliance Keep)
	ic.Nodes[ICNodeGraveyardA] = icNodeState{
		NodeType:    ICNodeGraveyardA,
		Faction:     ICTeamAlliance,
		State:       ICNodeStateControlledA,
		Banners:     [4]uint32{ICGameObjectBannerGraveyardA_A, ICGameObjectBannerGraveyardA_ACont, ICGameObjectBannerGraveyardA_H, ICGameObjectBannerGraveyardA_HCont},
		ContestTeam: ICTeamNeutral,
	}
	// 6: Graveyard H (Horde Keep)
	ic.Nodes[ICNodeGraveyardH] = icNodeState{
		NodeType:    ICNodeGraveyardH,
		Faction:     ICTeamHorde,
		State:       ICNodeStateControlledH,
		Banners:     [4]uint32{ICGameObjectBannerGraveyardH_A, ICGameObjectBannerGraveyardH_ACont, ICGameObjectBannerGraveyardH_H, ICGameObjectBannerGraveyardH_HCont},
		ContestTeam: ICTeamNeutral,
	}
}

// getNodeByBannerEntry finds which node corresponds to a clicked banner.
func (ic *icBattlegroundState) getNodeByBannerEntry(entry uint32) (uint8, bool) {
	switch entry {
	case ICGameObjectBannerRefinery, ICGameObjectBannerRefineryA, ICGameObjectBannerRefineryACont, ICGameObjectBannerRefineryH, ICGameObjectBannerRefineryHCont:
		return ICNodeRefinery, true
	case ICGameObjectBannerQuarry, ICGameObjectBannerQuarryA, ICGameObjectBannerQuarryACont, ICGameObjectBannerQuarryH, ICGameObjectBannerQuarryHCont:
		return ICNodeQuarry, true
	case ICGameObjectBannerDocks, ICGameObjectBannerDocksA, ICGameObjectBannerDocksACont, ICGameObjectBannerDocksH, ICGameObjectBannerDocksHCont:
		return ICNodeDocks, true
	case ICGameObjectBannerHangar, ICGameObjectBannerHangarA, ICGameObjectBannerHangarACont, ICGameObjectBannerHangarH, ICGameObjectBannerHangarHCont:
		return ICNodeHangar, true
	case ICGameObjectBannerWorkshop, ICGameObjectBannerWorkshopA, ICGameObjectBannerWorkshopACont, ICGameObjectBannerWorkshopH, ICGameObjectBannerWorkshopHCont:
		return ICNodeWorkshop, true
	case ICGameObjectBannerKeepA, ICGameObjectBannerGraveyardA_ACont, ICGameObjectBannerGraveyardA_H, ICGameObjectBannerGraveyardA_HCont:
		return ICNodeGraveyardA, true
	case ICGameObjectBannerKeepH, ICGameObjectBannerGraveyardH_A, ICGameObjectBannerGraveyardH_ACont, ICGameObjectBannerGraveyardH_HCont:
		return ICNodeGraveyardH, true
	}
	return 0, false
}

// handleICGameObjectUse handles interaction with IoC banners and bombs.
// Reference: BattlegroundIC::EventPlayerClickedOnFlag (BattlegroundIC.cpp:421-500).
func (s *Server) handleICGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return true
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return true
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return true
	}

	playerTeam := teamForRace(sess.player.Race)

	if nodeID, ok := ic.getNodeByBannerEntry(entry); ok {
		s.interactWithICBanner(ic, nodeID, playerTeam, sess)
		return true
	}

	if entry == ICGameObjectSeaforiumBombs || entry == ICGameObjectHugeBombA || entry == ICGameObjectHugeBombH {
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has picked up a Seaforium Bomb!", sess.player.Name))
		return true
	}

	return true
}

// interactWithICBanner implements the 1-minute assault/defend lifecycle for Isle of Conquest nodes.
// Reference: BattlegroundIC::EventPlayerClickedOnFlag (BattlegroundIC.cpp:421-500).
func (s *Server) interactWithICBanner(ic *icBattlegroundState, nodeID uint8, team uint32, sess *session) {
	node := &ic.Nodes[nodeID]
	nodeName := icNodeNames[nodeID]
	playerName := sess.player.Name
	teamName := "Alliance"
	if team == ICTeamHorde {
		teamName = "Horde"
	}

	// If node is already controlled by this player's team and not contested: nothing to do
	if (team == ICTeamAlliance && node.State == ICNodeStateControlledA) ||
		(team == ICTeamHorde && node.State == ICNodeStateControlledH) {
		return
	}

	// If the node is already contested by this player's team: nothing to do.
	// The 60s capture timer from the first click keeps running.
	// Reference: BattlegroundIC::EventPlayerClickedOnFlag (BattlegroundIC.cpp:430-431) —
	// nodePoint[i].faction holds the contesting team from the earlier click.
	if (team == ICTeamAlliance && node.State == ICNodeStateConflictA) ||
		(team == ICTeamHorde && node.State == ICNodeStateConflictH) {
		return
	}

	// DEFENDING: If the node is currently contested by the enemy, the original controller reclaims it
	if (team == ICTeamAlliance && node.State == ICNodeStateConflictH && node.Faction == ICTeamAlliance) ||
		(team == ICTeamHorde && node.State == ICNodeStateConflictA && node.Faction == ICTeamHorde) {
		if node.CaptureTimer != nil {
			node.CaptureTimer.Stop()
			node.CaptureTimer = nil
		}
		node.ContestTeam = ICTeamNeutral
		if team == ICTeamAlliance {
			node.State = ICNodeStateControlledA
		} else {
			node.State = ICNodeStateControlledH
		}
		s.sendICNodeWorldState(ic, nodeID)
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has defended %s for the %s!", playerName, nodeName, teamName))
		// BattlegroundIC::EventPlayerClickedOnFlag (BattlegroundIC.cpp:475-479):
		// UpdatePlayerScore(player, SCORE_BASES_DEFENDED, 1).
		s.creditBGObjectiveCapture(sess.playerGUID, ICObjectiveDefendBase)
		s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesDefended++ })
		return
	}

	// COUNTER-CONTEST ON A NEVER-CAPTURED NODE: contested by the enemy with no
	// previous owner (Faction neutral — the C++ last_entry is the neutral banner,
	// BattlegroundIC.h:893). BattlegroundIC::GetNextBanner
	// (BattlegroundIC.cpp:527-545) then returns the neutral banner, so neither
	// the assault arm (cpp:444-464) nor the defend arm (cpp:465-479) fires: the
	// 60s timer from the first click KEEPS RUNNING (it is not restarted), no
	// assault score is credited, and no broadcast goes out. The banner object is
	// swapped back to neutral while the worldstate rows stay on the previous
	// conflict (UpdateNodeWorldState matches none of the four faction banners),
	// and nodePoint.faction takes the clicking team — mirrored here by keeping
	// State and the live CaptureTimer while moving ContestTeam to the clicker.
	// The capture then resolves to the clicker at the ORIGINAL deadline: the
	// Update arm reads the current faction (BattlegroundIC.cpp:175), which is
	// why resolveICNodeCapture reads ContestTeam at fire time.
	if ((team == ICTeamAlliance && node.State == ICNodeStateConflictH) ||
		(team == ICTeamHorde && node.State == ICNodeStateConflictA)) && node.Faction == ICTeamNeutral {
		node.ContestTeam = team
		return
	}

	// ASSAULTING: Node is uncontrolled, controlled by enemy, or contested
	if node.CaptureTimer != nil {
		node.CaptureTimer.Stop()
		node.CaptureTimer = nil
	}
	node.ContestTeam = team

	if team == ICTeamAlliance {
		node.State = ICNodeStateConflictA
	} else {
		node.State = ICNodeStateConflictH
	}
	s.sendICNodeWorldState(ic, nodeID)
	s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has assaulted %s for the %s!", playerName, nodeName, teamName))
	// BattlegroundIC::EventPlayerClickedOnFlag (BattlegroundIC.cpp:457-464):
	// UpdatePlayerScore(player, SCORE_BASES_ASSAULTED, 1).
	s.creditBGObjectiveCapture(sess.playerGUID, ICObjectiveAssaultBase)
	s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.BasesAssaulted++ })

	// Start 60s capture timer. The callback resolves to the ContestTeam live
	// at fire time (see the counter-contest arm above): C++ reads
	// nodePoint.faction at expiry, not the team that armed the timer.
	node.CaptureTimer = time.AfterFunc(ic.BannerCaptureDuration, func() {
		s.resolveICNodeCapture(ic, nodeID)
	})
}

// resolveICNodeCapture finalizes ownership after 60s without defense. The
// capturing team is the ContestTeam live at fire time — C++ reads
// nodePoint.faction at expiry (BattlegroundIC.cpp:175), which a counter-click
// on a never-captured node moves without restarting the timer.
func (s *Server) resolveICNodeCapture(ic *icBattlegroundState, nodeID uint8) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	node := &ic.Nodes[nodeID]
	team := node.ContestTeam
	if team == ICTeamNeutral {
		return
	}
	node.Faction = team
	node.ContestTeam = ICTeamNeutral
	if team == ICTeamAlliance {
		node.State = ICNodeStateControlledA
	} else {
		node.State = ICNodeStateControlledH
	}
	s.sendICNodeWorldState(ic, nodeID)

	// BattlegroundIC::HandleCapturedNodes (BattlegroundIC.cpp:653-661): capturing
	// the quarry/refinery casts the buff on the capturing team and strips it from
	// the enemy team.
	if nodeID == ICNodeQuarry || nodeID == ICNodeRefinery {
		s.syncICNodeAuraTeam(ic, nodeID, team)
	}

	nodeName := icNodeNames[nodeID]
	teamName := "Alliance"
	if team == ICTeamHorde {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("The %s has taken %s!", teamName, nodeName))
}

// DamageICGate applies damage to an IoC fortress gate, transitioning to Damaged and Destroyed.
// Reference: BattlegroundIC::DestroyGate (BattlegroundIC.cpp:797-860).
func (s *Server) DamageICGate(mapID uint32, gateID uint8, damage uint32, attacker *session) {
	if s == nil || gateID >= ICMaxGates {
		return
	}
	ic := s.getOrCreateICState(mapID)
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	gate := &ic.Gates[gateID]
	if gate.State == ICGateDestroyed {
		return
	}

	if damage >= gate.Health {
		gate.Health = 0
	} else {
		gate.Health -= damage
	}

	info := icGates[gateID]

	// Damaged state (health <= 50%)
	if gate.Health <= gate.MaxHealth/2 && gate.Health > 0 && gate.State == ICGateOK {
		gate.State = ICGateDamaged
		s.broadcastBattlegroundMessage(mapID, fmt.Sprintf("%s is under attack!", info.Name))
	}

	// Destroyed state
	if gate.Health == 0 && gate.State != ICGateDestroyed {
		gate.State = ICGateDestroyed
		// Update world states: closed becomes 0, open becomes 1
		s.updateICWorldState(mapID, info.ClosedWS, 0)
		s.updateICWorldState(mapID, info.OpenWS, 1)
		s.broadcastBattlegroundMessage(mapID, fmt.Sprintf("%s has been destroyed!", info.Name))
	}
}

// handleICPlayerDeath decrements team reinforcements on death.
// Reference: BattlegroundIC::HandleKillPlayer (BattlegroundIC.cpp:405-419).
func (s *Server) handleICPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != ICMapID {
		return
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	victimTeam := teamForRace(sess.player.Race)
	if victimTeam == ICTeamAlliance {
		if ic.AllianceReinforcements > 0 {
			ic.AllianceReinforcements--
		}
		s.updateICWorldState(ic.MapID, ICWorldStateAllianceReinforcements, ic.AllianceReinforcements)
		if ic.AllianceReinforcements == 0 {
			s.endIC(ic, int8(ICTeamHorde))
		}
	} else {
		if ic.HordeReinforcements > 0 {
			ic.HordeReinforcements--
		}
		s.updateICWorldState(ic.MapID, ICWorldStateHordeReinforcements, ic.HordeReinforcements)
		if ic.HordeReinforcements == 0 {
			s.endIC(ic, int8(ICTeamAlliance))
		}
	}
}

// handleICCreatureKilled handles Boss kills (Halford Wyrmbane & Overlord Agmar) and vehicle destruction.
// Reference: BattlegroundIC::HandleKillUnit (BattlegroundIC.cpp:382-403).
func (s *Server) handleICCreatureKilled(sess *session, creatureEntry uint32) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != ICMapID {
		return
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	switch creatureEntry {
	case ICCreatureHalfordWyrmbane:
		// Alliance General killed -> Horde wins immediately!
		ic.AllianceBossAlive = false
		s.broadcastBattlegroundMessage(ic.MapID, "High Commander Halford Wyrmbane has been slain! The Horde is victorious!")
		s.grantICWinnerHonor(ic, ICTeamHorde)
		s.endIC(ic, int8(ICTeamHorde))

	case ICCreatureOverlordAgmar:
		// Horde General killed -> Alliance wins immediately!
		ic.HordeBossAlive = false
		s.broadcastBattlegroundMessage(ic.MapID, "Overlord Agmar has been slain! The Alliance is victorious!")
		s.grantICWinnerHonor(ic, ICTeamAlliance)
		s.endIC(ic, int8(ICTeamAlliance))

	case ICCreatureDemolisher, ICCreatureSiegeEngineA, ICCreatureSiegeEngineH,
		ICCreatureGlaiveThrowerA, ICCreatureGlaiveThrowerH, ICCreatureCatapult:
		ic.VehiclesDestroyed[sess.playerGUID]++
		// BattlegroundIC::HandleKillUnit (BattlegroundIC.cpp:401-404): destroying
		// a vehicle casts SPELL_DESTROYED_VEHICLE_ACHIEVEMENT (68357) triggered on
		// the killer — the "Mowed Down" credit. Triggered casts credit via the
		// CAST_SPELL criterion (spells.go:16087), so credit it directly.
		sess.updateAchievementCriteria(criteriaTypeCastSpell, ICSpellDestroyedVehicleAchievement, 1)
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has destroyed a siege vehicle!", sess.player.Name))
	}
}

// grantICWinnerHonor mirrors the RewardHonorToTeam(WINNER_HONOR_AMOUNT, team)
// arm of BattlegroundIC::HandleKillUnit (BattlegroundIC.cpp:385-396): 500 raw
// honor to every entered-slot holder for the IoC map on the winning team, ahead
// of the end-battleground arm (== C++ arm order). rewardBGEndHonor derives
// honor from kill counts via GetBonusHonorFromKill, so the flat winner amount
// goes through sess.rewardHonorPoints directly.
func (s *Server) grantICWinnerHonor(ic *icBattlegroundState, team uint32) {
	if s == nil || ic == nil {
		return
	}
	for _, sess := range s.bgRewardTargets(ic.MapID) {
		if teamForRace(sess.player.Race) == team {
			sess.rewardHonorPoints(context.Background(), ICWinnerHonorAmount)
		}
	}
}

// applyICNodeAuras mirrors BattlegroundIC::HandlePlayerResurrect
// (BattlegroundIC.cpp:63-70): on resurrection in IoC, grant the
// quarry/refinery buffs (SPELL_QUARRY=68720 / SPELL_OIL_REFINERY=68719) when
// the player's team controls that node (== C++ nodeState == controlled-for-team
// gate). Called from the resurrect choke point, same class as the
// death.go closestGraveyard IoC dispatch.
func (s *Server) applyICNodeAuras(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return
	}
	controlledState := ICNodeStateControlledH
	if teamForRace(sess.player.Race) == ICTeamAlliance {
		controlledState = ICNodeStateControlledA
	}
	ic.mu.Lock()
	quarryControlled := ic.Nodes[ICNodeQuarry].State == controlledState
	refineryControlled := ic.Nodes[ICNodeRefinery].State == controlledState
	ic.mu.Unlock()
	if quarryControlled {
		sess.applyAura(ICSpellQuarry)
	}
	if refineryControlled {
		sess.applyAura(ICSpellOilRefinery)
	}
}

// TickICResources generates passive reinforcements (+1 every 45s) for controlled Refinery and Quarry.
// Reference: BattlegroundIC::PostUpdateImpl / IC_RESOURCE_TIME (BattlegroundIC.cpp:205-219).
// Driven by updateICBattles on the world tick (BattlegroundMgr::Update sweeps
// instances every BATTLEGROUND_OBJECTIVE_UPDATE_INTERVAL = 1000ms,
// BattlegroundMgr.cpp:94 / BattlegroundMgr.h:38). C++ applies no upper clamp
// to the reinforcement counter, so neither does this.
func (s *Server) TickICResources(ic *icBattlegroundState, elapsedMs int64) {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	tickMs := int64(ic.ResourceTickDuration / time.Millisecond)
	if tickMs <= 0 {
		tickMs = int64(ICResourceTickDuration / time.Millisecond)
	}
	ic.ResourceAccumMs += elapsedMs
	if ic.ResourceAccumMs < tickMs {
		return
	}
	ic.ResourceAccumMs -= tickMs

	for _, nodeID := range []uint8{ICNodeRefinery, ICNodeQuarry} {
		node := &ic.Nodes[nodeID]
		if node.State == ICNodeStateControlledA {
			ic.AllianceReinforcements++
			s.updateICWorldState(ic.MapID, ICWorldStateAllianceReinforcements, ic.AllianceReinforcements)
			// BattlegroundIC::PostUpdateImpl (BattlegroundIC.cpp:212):
			// RewardHonorToTeam(RESOURCE_HONOR_AMOUNT, faction).
			s.grantICTeamHonor(ic, ICTeamAlliance, ICResourceHonorAmount)
		} else if node.State == ICNodeStateControlledH {
			ic.HordeReinforcements++
			s.updateICWorldState(ic.MapID, ICWorldStateHordeReinforcements, ic.HordeReinforcements)
			s.grantICTeamHonor(ic, ICTeamHorde, ICResourceHonorAmount)
		}
	}
}

// updateICBattles ticks every live IoC state, driving the 45s resource tick.
// Same 1s cadence convention as updateArenaBattles/updateAVBattles/
// updateEOTSBattles/updateSABattles; wired from the world tick in server.go.
func (s *Server) updateICBattles(now time.Time) {
	if s == nil {
		return
	}
	if !s.icTickLast.IsZero() && now.Sub(s.icTickLast) < time.Second {
		return
	}
	var elapsedMs int64 = 1000
	if !s.icTickLast.IsZero() {
		elapsedMs = now.Sub(s.icTickLast).Milliseconds()
	}
	s.icTickLast = now
	s.icMu.RLock()
	battles := make([]*icBattlegroundState, 0, len(s.icState))
	for _, ic := range s.icState {
		battles = append(battles, ic)
	}
	s.icMu.RUnlock()
	for _, ic := range battles {
		s.TickICResources(ic, elapsedMs)
	}
}

// grantICTeamHonor mirrors Battleground::RewardHonorToTeam: raw honor to every
// worldReady session on the IoC map on the given team (0 Alliance, 1 Horde).
// The per-session call is rewardHonorPoints == Player::RewardHonor(nullptr, 1,
// honor), the same tail grantICWinnerHonor uses.
func (s *Server) grantICTeamHonor(ic *icBattlegroundState, team uint32, honor uint32) {
	if s == nil || ic == nil || honor == 0 {
		return
	}
	for _, sess := range s.bgRewardTargets(ic.MapID) {
		if teamForRace(sess.player.Race) == team {
			sess.rewardHonorPoints(context.Background(), honor)
		}
	}
}

// syncICNodeAuraTeam mirrors the quarry/refinery arms of
// BattlegroundIC::HandleCapturedNodes (BattlegroundIC.cpp:653-661): the buff is
// cast on the capturing team and stripped from the enemy team at capture time.
func (s *Server) syncICNodeAuraTeam(ic *icBattlegroundState, nodeID uint8, team uint32) {
	if s == nil || ic == nil {
		return
	}
	var spellID uint32
	switch nodeID {
	case ICNodeQuarry:
		spellID = ICSpellQuarry
	case ICNodeRefinery:
		spellID = ICSpellOilRefinery
	default:
		return
	}
	for _, sess := range s.bgRewardTargets(ic.MapID) {
		if teamForRace(sess.player.Race) == team {
			sess.applyAura(spellID)
		} else {
			sess.removeAura(spellID)
		}
	}
}

// handleICBackDoorTrigger mirrors BattlegroundIC::HandleAreaTrigger
// (BattlegroundIC.cpp:270-291): trigger 5555 (Horde, at the Alliance keep) /
// 5535 (Alliance, at the Horde keep) casts SPELL_BACK_DOOR_JOB_ACHIEVEMENT
// (68502) triggered on the player when none of the enemy's three gates is
// destroyed yet. The triggered cast credits via the CAST_SPELL criterion
// (spells.go:16087), so credit it directly. STATUS_IN_PROGRESS maps to the Go
// Winner < 0 gate.
func (s *Server) handleICBackDoorTrigger(sess *session, triggerID uint32) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	team := teamForRace(sess.player.Race)
	if triggerID == ICAreaTriggerBackDoorHorde && team != ICTeamHorde {
		return
	}
	if triggerID == ICAreaTriggerBackDoorAlliance && team != ICTeamAlliance {
		return
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return
	}
	ic.mu.Lock()
	if ic.Winner >= 0 {
		ic.mu.Unlock()
		return
	}
	gates := ic.Gates
	ic.mu.Unlock()
	var check []uint8
	if team == ICTeamHorde {
		check = []uint8{ICAllianceFrontGate, ICAllianceWestGate, ICAllianceEastGate}
	} else {
		check = []uint8{ICHordeFrontGate, ICHordeWestGate, ICHordeEastGate}
	}
	for _, g := range check {
		if gates[g].State == ICGateDestroyed {
			return
		}
	}
	sess.updateAchievementCriteria(criteriaTypeCastSpell, ICSpellBackDoorJobAchievement, 1)
}

func (s *Server) endIC(ic *icBattlegroundState, winner int8) {
	if ic.Winner >= 0 {
		return
	}
	ic.Winner = winner
	ic.EndedAt = time.Now()
	s.markBGSlotsWaitLeave(ic.MapID)
	// Reference: Battleground::EndBattleground (Battleground.cpp:667) sets the
	// winner and the instance leaves STATUS_IN_PROGRESS, so the zone
	// PostUpdateImpl timer arms never tick again and the timers die with the
	// instance. Go models each IoC node timer as a time.AfterFunc, so stop
	// every armed node timer here under the caller's ic.mu (the lock is also
	// held by the timer callback, so Stop never races a running callback;
	// the ic.Winner >= 0 gate in resolveICNodeCapture stays as defense for a
	// fire that lands between the gate and Stop).
	for i := range ic.Nodes {
		if ic.Nodes[i].CaptureTimer != nil {
			ic.Nodes[i].CaptureTimer.Stop()
			ic.Nodes[i].CaptureTimer = nil
		}
	}
	// A neutral premature winner sends no win broadcast (C++
	// EndBattleground's neutral arm sets PVP_TEAM_NEUTRAL with no win text);
	// the criteria credit below still runs, since the WIN_BG /
	// COMPLETE_BATTLEGROUND arms are unconditional and creditBattlegroundWin
	// with a neutral team credits completion to all participants and the win
	// to nobody (teamForRace never returns 2).
	if winner == 2 {
		s.broadcastBattlegroundMessage(ic.MapID, "The battle for Isle of Conquest ended in a draw!")
	} else {
		winnerTeam := "Alliance"
		if winner == int8(ICTeamHorde) {
			winnerTeam = "Horde"
		}
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("The %s has won the battle for Isle of Conquest!", winnerTeam))
	}
	// Reference: Battleground::EndBattleground (Battleground.cpp:775-800): the
	// WIN_BG / COMPLETE_BATTLEGROUND criteria arms are unconditional (unlike the
	// IsRandom/Weekend-gated winner/loser honor arms, which have no reachable
	// model in Go); AB and EotS victories already credit through this helper.
	s.creditBattlegroundWin(ic.MapID, uint32(winner))
}

func (s *Server) handleICPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != ICMapID {
		return
	}
	// BattlegroundIC::RemovePlayer (BattlegroundIC.cpp:261-268) strips the
	// quarry/refinery buffs when the player leaves the battleground.
	sess.removeAura(ICSpellQuarry)
	sess.removeAura(ICSpellOilRefinery)
}

func (s *Server) sendICNodeWorldState(ic *icBattlegroundState, nodeID uint8) {
	node := &ic.Nodes[nodeID]
	info := icNodeWorldStates[nodeID]

	// Reset all 5 worldstates for this node, then set the active one to 1
	states := [5]uint32{info.Uncontrolled, info.ConflictA, info.ConflictH, info.ControlledA, info.ControlledH}
	for i, ws := range states {
		val := uint32(0)
		if uint8(i) == node.State {
			val = 1
		}
		s.updateICWorldState(ic.MapID, ws, val)
	}
}

// updateICWorldState sends an IoC world state update to the map's roster
// (bgRewardTargets), not to the map's players: C++ UpdateWorldState sends
// via SendPacketToAll over m_Players (Battleground.cpp:659-664, :568-571).
func (s *Server) updateICWorldState(mapID, variableID, value uint32) {
	for _, sess := range s.bgRewardTargets(mapID) {
		sess.sendWorldState(variableID, value)
	}
}

func (s *Server) sendICInitialWorldStates(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	sess.sendWorldState(ICWorldStateAllianceReinforcementsSet, 1)
	sess.sendWorldState(ICWorldStateHordeReinforcementsSet, 1)
	sess.sendWorldState(ICWorldStateAllianceReinforcements, ic.AllianceReinforcements)
	sess.sendWorldState(ICWorldStateHordeReinforcements, ic.HordeReinforcements)

	// Gates
	for i := 0; i < ICMaxGates; i++ {
		gate := &ic.Gates[i]
		info := icGates[i]
		if gate.State == ICGateDestroyed {
			sess.sendWorldState(info.ClosedWS, 0)
			sess.sendWorldState(info.OpenWS, 1)
		} else {
			sess.sendWorldState(info.ClosedWS, 1)
			sess.sendWorldState(info.OpenWS, 0)
		}
	}

	// Nodes
	for i := uint8(0); i < ICMaxNodes; i++ {
		node := &ic.Nodes[i]
		info := icNodeWorldStates[i]
		states := [5]uint32{info.Uncontrolled, info.ConflictA, info.ConflictH, info.ControlledA, info.ControlledH}
		for idx, ws := range states {
			val := uint32(0)
			if uint8(idx) == node.State {
				val = 1
			}
			sess.sendWorldState(ws, val)
		}
	}
}
