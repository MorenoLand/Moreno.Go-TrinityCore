package world

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
)

// BattlegroundIC.h BG_IC_NPCs slot ordinals (BattlegroundIC.h:357-421):
// the BG-creature registry is a slot-indexed GUID vector
// (Battleground::BgCreatures, Battleground.h:457), sized
// MAX_NORMAL_NPCS_SPAWNS + MAX_WORKSHOP_SPAWNS + MAX_DOCKS_SPAWNS +
// MAX_SPIRIT_GUIDES_SPAWNS + MAX_HANGAR_NPCS_SPAWNS
// (BattlegroundIC.cpp:48-49). Only the vehicle and boss slots the
// ported arms touch are named here.
const (
	icNPCSlotOverlordAgmar   uint32 = 0
	icNPCSlotHalfordWyrmbane uint32 = 1
	icNPCSlotSiegeEngineA    uint32 = 34
	icNPCSlotSiegeEngineH    uint32 = 35
	icNPCSlotDemolisher1A    uint32 = 36
	icNPCSlotDemolisher1H    uint32 = 40
	icNPCSlotGlaiveThrower1A uint32 = 44
	icNPCSlotGlaiveThrower1H uint32 = 46
	icNPCSlotCatapult1A      uint32 = 48
	icNPCSlotCatapult1H      uint32 = 52
)

// BG_IC_Factions (BattlegroundIC.h:25-28): 1732 Alliance, 1735 Horde.
const (
	icFactionAlliance uint32 = 1732
	icFactionHorde    uint32 = 1735
)

// UNIT_FLAG_CANNOT_SWIM (UnitDefines.h:138): no Go named const exists.
const icUnitFlagCannotSwim uint32 = 0x00004000

// BG_IC_WorkshopVehicles (BattlegroundIC.h:510-517): 4 demolishers +
// siege engine at index 4. BG_IC_DocksVehiclesGlaives (:519-522) and
// BG_IC_DocksVehiclesCatapults (:525-530).
var icWorkshopVehiclePositions = [5][4]float32{
	{751.8281, -852.732666, 12.5250978, 1.46607661},
	{761.809, -854.2274, 12.5263243, 1.46607661},
	{783.4722, -853.9601, 12.54775, 1.71042264},
	{793.055542, -852.71875, 12.5671329, 1.71042264},
	{773.680542, -884.092041, 16.8090363, 1.58824956},
}

var icDocksGlaivePositions = [2][4]float32{
	{779.3125, -342.972229, 12.2104874, 4.712389},
	{790.029541, -342.899323, 12.2128582, 4.71238},
}

var icDocksCatapultPositions = [4][4]float32{
	{757.283, -341.7795, 12.2113762, 4.729842},
	{766.947937, -342.053833, 12.2009945, 4.694},
	{800.3785, -342.607635, 12.1669979, 4.6774},
	{810.7257, -342.083344, 12.1675768, 4.66},
}

// BG_IC_NpcSpawnlocs (BattlegroundIC.h:471-472): faction boss spawns.
var icBossSpawnPositions = [2][4]float32{
	{1295.44, -765.733, 70.0541, 0.0},
	{224.983, -831.573, 60.9034, 0.0},
}

// icBGCreature is one Battleground::BgCreatures slot
// (Battleground.h:457): the creature GUID plus the spawn record, so a
// slot whose motion was pruned after death can re-spawn from the
// stored entry/position instead of needing a live motion.
type icBGCreature struct {
	Entry      uint32
	GUID       uint64
	InstanceID uint32
	X, Y, Z, O float32
	Team       uint32
}

const (
	// WORKSHOP_UPDATE_TIME / DOCKS_UPDATE_TIME
	// (BattlegroundIC.h:193-194): 180000ms vehicle respawn cadence.
	icWorkshopUpdateMs int64 = 180000
	icDocksUpdateMs    int64 = 180000
)

const (
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
	// Creatures is the Battleground::BgCreatures registry
	// (Battleground.h:457): slot -> spawn record for the BG
	// creatures the ported arms manage (workshop/docks vehicles,
	// faction bosses). The caller's ic.mu is held.
	Creatures map[uint32]icBGCreature
	// DocksTimerMs / WorkshopTimerMs are docksTimer /
	// siegeEngineWorkshopTimer (BattlegroundIC.cpp:110-166): ms
	// left until the next dead-vehicle respawn sweep while the
	// node is controlled; reset to the full update time on fire
	// and on capture (BattlegroundIC.cpp:666-667, :720-721).
	DocksTimerMs    int64
	WorkshopTimerMs int64
	StopTicker      chan struct{}
	ResourceAccumMs int64 // ms accumulated toward the next 45s resource tick
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
	created := false
	defer func() {
		if created {
			s.triggerBGEvent(mapID, scripting.BGEventOnCreate)
		}
	}()
	defer s.icMu.Unlock()
	if s.icState == nil {
		s.icState = make(map[uint32]*icBattlegroundState)
	}
	state := s.icState[mapID]
	if state == nil {
		created = true
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
			Creatures:              make(map[uint32]icBGCreature),
			DocksTimerMs:           icDocksUpdateMs,
			WorkshopTimerMs:        icWorkshopUpdateMs,
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

// resetICStateForNewMatch is the Go form of the CreateNewBattleground +
// Battleground::Reset pair (BattlegroundMgr.cpp:412,
// Battleground.cpp:935-938) for the successor-reuse case: C++ hands every
// match a fresh BG object, while Go reuses one per-map state, so a successor
// match arriving in the previous match's WaitLeave window inherits the ended
// fields instead - Winner >= 0 gates every node/gate arm, and reinforcements,
// bosses, gates, and nodes are the old match's. Runs at formation (first
// fresh accept, see rearmNonArenaBGMatchStartTimeOnFirstAccept), never at
// the end arms. The caller's ic.mu is held. IC nodes and gates carry no
// dynamic GUIDs (banners are entry constants), so initNodes plus the gate
// loop restore the creation values with nothing to despawn.
func (s *Server) resetICStateForNewMatch(ic *icBattlegroundState) {
	for i := range ic.Nodes {
		if ic.Nodes[i].CaptureTimer != nil {
			ic.Nodes[i].CaptureTimer.Stop()
			ic.Nodes[i].CaptureTimer = nil
		}
	}
	ic.initNodes()
	for i := 0; i < ICMaxGates; i++ {
		ic.Gates[i] = icGateState{
			State:     ICGateOK,
			Health:    ICGateDefaultMaxHealth,
			MaxHealth: ICGateDefaultMaxHealth,
		}
	}
	ic.AllianceReinforcements = ICMaxReinforcements
	ic.HordeReinforcements = ICMaxReinforcements
	ic.AllianceBossAlive = true
	ic.HordeBossAlive = true
	ic.VehiclesDestroyed = make(map[uint64]uint32)
	ic.Creatures = make(map[uint32]icBGCreature)
	ic.DocksTimerMs = icDocksUpdateMs
	ic.WorkshopTimerMs = icWorkshopUpdateMs
	ic.ResourceAccumMs = 0
	ic.Winner = -1
	ic.EndedAt = time.Time{}
	ic.PrematureCountDown = false
	ic.PrematureTimerMs = 0
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
	// The registry's motions live in the match instance; dropping
	// the state orphans them as manual-despawn temp summons, so
	// despawn them here (ic.mu released — despawnCreatureMotion
	// takes motionMu and fires delete events).
	creatures := make([]icBGCreature, 0, len(ic.Creatures))
	for _, rec := range ic.Creatures {
		creatures = append(creatures, rec)
	}
	ic.Creatures = make(map[uint32]icBGCreature)
	ic.mu.Unlock()
	for _, rec := range creatures {
		s.despawnCreatureMotion(context.Background(), ICMapID, rec.InstanceID, rec.GUID)
	}
	s.icMu.Lock()
	destroyed := s.icState[mapID] == ic
	if destroyed {
		delete(s.icState, mapID)
	}
	s.icMu.Unlock()
	if destroyed {
		s.triggerBGEvent(mapID, scripting.BGEventOnPreDestroy)
	}
}

// icBGInstanceID resolves the live IoC match instance from the first
// entered session on the map (sess.bgData.InstanceID), mirroring the
// bgRewardTargets membership scan. 0 when no IC player is loaded —
// no match is running, so no BG creature can spawn.
func (s *Server) icBGInstanceID() uint32 {
	if s == nil {
		return 0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.bgData.InstanceID == 0 {
			continue
		}
		entered := false
		for i := range sess.bgQueues {
			if e := &sess.bgQueues[i]; e.Active && !e.IsArena && e.MapID == ICMapID && (e.Status == BGStatusInProgress || e.Status == BGStatusWaitLeave) {
				entered = true
				break
			}
		}
		if entered {
			return sess.bgData.InstanceID
		}
	}
	return 0
}

// addICBGCreature is Battleground::AddCreature
// (Battleground.cpp:1444-1497) for the IC registry: spawns the entry
// at the slot position as a manual-despawn temp summon (C++ creates a
// plain Creature with RESPAWN_ONE_DAY; Go has no DB-backed BG spawn,
// so the temp-summon funnel is the spawn path), stamps the BG faction
// (HandleCapturedNodes calls SetFaction(BG_IC_Factions[team]) after
// every AddCreature), and stores the GUID in the slot. The caller's
// ic.mu is held. False when no match instance is live or the spawn
// fails.
func (s *Server) addICBGCreature(ctx context.Context, ic *icBattlegroundState, slot, entry uint32, pos [4]float32, team uint32) bool {
	if s == nil || ic == nil {
		return false
	}
	instanceID := s.icBGInstanceID()
	if instanceID == 0 {
		return false
	}
	summoned := s.summonTempCreature(ctx, tempSummonOrigin{Map: ICMapID, InstanceID: instanceID}, entry, pos[0], pos[1], pos[2], pos[3], 7, 0)
	if summoned == nil {
		return false
	}
	faction := icFactionAlliance
	if team == ICTeamHorde {
		faction = icFactionHorde
	}
	s.motionMu.Lock()
	summoned.Faction = faction
	summoned.FactionKnown = true
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(ICMapID, instanceID, summoned.GUID, map[int]uint32{unitFieldFaction: faction})
	ic.Creatures[slot] = icBGCreature{Entry: entry, GUID: summoned.GUID, InstanceID: instanceID, X: pos[0], Y: pos[1], Z: pos[2], O: pos[3], Team: team}
	return true
}

// getICBGCreature is Battleground::GetBGCreature
// (Battleground.cpp:1402-1415): the slot's live motion, or nil when
// the slot is empty or the motion was pruned. The caller's ic.mu is
// held.
func (s *Server) getICBGCreature(ic *icBattlegroundState, slot uint32) *creatureMotion {
	if s == nil || ic == nil {
		return nil
	}
	rec, ok := ic.Creatures[slot]
	if !ok || rec.GUID == 0 {
		return nil
	}
	return s.findCreatureMotion(ICMapID, rec.InstanceID, rec.GUID)
}

// delICBGCreature is Battleground::DelCreature
// (Battleground.cpp:1504-1520): AddObjectToRemoveList + clear the
// slot. The caller's ic.mu is held.
func (s *Server) delICBGCreature(ctx context.Context, ic *icBattlegroundState, slot uint32) {
	if s == nil || ic == nil {
		return
	}
	if rec, ok := ic.Creatures[slot]; ok && rec.GUID != 0 {
		s.despawnCreatureMotion(ctx, ICMapID, rec.InstanceID, rec.GUID)
	}
	delete(ic.Creatures, slot)
}

// respawnICBGCreature runs Creature::Respawn(true) for a registry
// slot: a live motion is left alone, a dead motion respawns in
// place (respawnCreatureNow), and a pruned motion re-spawns fresh
// from the stored record (a MANUAL_DESPAWN temp summon's corpse is
// pruned after death, so the motion may be gone by the 180s sweep).
// The caller's ic.mu is held.
func (s *Server) respawnICBGCreature(ctx context.Context, ic *icBattlegroundState, slot uint32) {
	if s == nil || ic == nil {
		return
	}
	rec, ok := ic.Creatures[slot]
	if !ok {
		return
	}
	motion := s.findCreatureMotion(ICMapID, rec.InstanceID, rec.GUID)
	if motion == nil {
		delete(ic.Creatures, slot)
		s.addICBGCreature(ctx, ic, slot, rec.Entry, [4]float32{rec.X, rec.Y, rec.Z, rec.O}, rec.Team)
		return
	}
	if motion.Health == 0 {
		spawnGUID := uint32(rec.GUID & 0x00FFFFFF)
		s.respawnCreatureNow(ctx, motion, spawnGUID, rec.Entry)
	}
}

// tickICVehicleTimers runs the PostUpdateImpl vehicle respawn arms
// (BattlegroundIC.cpp:110-166): every DOCKS_UPDATE_TIME /
// WORKSHOP_UPDATE_TIME (180s) while the node is controlled, respawn
// the faction's dead docks/workshop vehicles; the workshop siege
// engine, when alive and flagged NOT_SELECTABLE|CANNOT_SWIM|
// IMMUNE_TO_PC, loses NOT_SELECTABLE|IMMUNE_TO_PC (CANNOT_SWIM
// stays, per the sniff note), and when alive unflagged heals to
// full. The C++ loops run `u < _4` / `u < _2` — iterating only
// slots 1..3 (catapults/demolishers) and 1 (glaives), an upstream
// off-by-one that skips the last slot; replicated verbatim. The
// caller's ic.mu is held; driven from TickICResources on the 1s
// world tick.
func (s *Server) tickICVehicleTimers(ctx context.Context, ic *icBattlegroundState, elapsedMs int64) {
	if s == nil || ic == nil || elapsedMs <= 0 {
		return
	}
	docks := &ic.Nodes[ICNodeDocks]
	if docks.State == ICNodeStateControlledA || docks.State == ICNodeStateControlledH {
		if ic.DocksTimerMs <= elapsedMs {
			catapult1, glaive1 := icNPCSlotCatapult1A, icNPCSlotGlaiveThrower1A
			if docks.Faction == ICTeamHorde {
				catapult1, glaive1 = icNPCSlotCatapult1H, icNPCSlotGlaiveThrower1H
			}
			for slot := catapult1; slot < catapult1+3; slot++ {
				s.respawnICBGCreature(ctx, ic, slot)
			}
			for slot := glaive1; slot < glaive1+1; slot++ {
				s.respawnICBGCreature(ctx, ic, slot)
			}
			ic.DocksTimerMs = icDocksUpdateMs
		} else {
			ic.DocksTimerMs -= elapsedMs
		}
	}
	workshop := &ic.Nodes[ICNodeWorkshop]
	if workshop.State == ICNodeStateControlledA || workshop.State == ICNodeStateControlledH {
		if ic.WorkshopTimerMs <= elapsedMs {
			siegeSlot := icNPCSlotSiegeEngineA
			demolisher1 := icNPCSlotDemolisher1A
			if workshop.Faction == ICTeamHorde {
				siegeSlot, demolisher1 = icNPCSlotSiegeEngineH, icNPCSlotDemolisher1H
			}
			if siege := s.getICBGCreature(ic, siegeSlot); siege != nil {
				s.motionMu.Lock()
				alive, flags, health, maxHealth := siege.Health > 0, siege.UnitFlags, siege.Health, siege.MaxHealth
				s.motionMu.Unlock()
				if alive {
					if flags&(unitFlagNotSelectable|icUnitFlagCannotSwim|unitFlagImmuneToPC) != 0 {
						s.motionMu.Lock()
						siege.UnitFlags &^= unitFlagNotSelectable | unitFlagImmuneToPC
						flags = siege.UnitFlags
						s.motionMu.Unlock()
						s.broadcastCreatureValuesUpdateInInstance(ICMapID, siege.InstanceID, siege.GUID, map[int]uint32{unitFieldFlags: flags})
					} else if health < maxHealth {
						s.motionMu.Lock()
						siege.Health = maxHealth
						s.motionMu.Unlock()
						s.broadcastCreatureValuesUpdateInInstance(ICMapID, siege.InstanceID, siege.GUID, map[int]uint32{unitFieldHealth: maxHealth})
					}
				} else {
					s.respawnICBGCreature(ctx, ic, siegeSlot)
				}
			}
			for slot := demolisher1; slot < demolisher1+3; slot++ {
				s.respawnICBGCreature(ctx, ic, slot)
			}
			ic.WorkshopTimerMs = icWorkshopUpdateMs
		} else {
			ic.WorkshopTimerMs -= elapsedMs
		}
	}
}

// captureICNodeVehicles runs the Docks/Workshop arms of
// BattlegroundIC::HandleCapturedNodes (BattlegroundIC.cpp:662-791)
// with recapture=false: the caller is resolveICNodeCapture, which
// fires only from the 60s banner timer (= the PostUpdateImpl
// needChange arm, :194), never from the defend path (:475,
// recapture=true). On capture the node's respawn timer resets to
// full; unused enemy vehicles are deleted (glaives/catapults with
// no seat-0 passenger, demolishers/siege not in use); the
// capturing team's vehicles spawn at the node positions, skipping
// slots already alive; the siege engine spawns flagged
// NOT_SELECTABLE|CANNOT_SWIM (SetImmuneToPC has no engine model,
// so that leg stays out). The seaforium-bomb arm (:781-791) stays
// out: Go has no BG gameobject spawn (AddObject) helper. The
// caller's ic.mu is held.
func (s *Server) captureICNodeVehicles(ctx context.Context, ic *icBattlegroundState, nodeID uint8, team uint32) {
	if s == nil || ic == nil || team == ICTeamNeutral {
		return
	}
	switch nodeID {
	case ICNodeDocks:
		if ic.DocksTimerMs < icDocksUpdateMs {
			ic.DocksTimerMs = icDocksUpdateMs
		}
		enemyGlaive1, enemyCatapult1 := icNPCSlotGlaiveThrower1H, icNPCSlotCatapult1H
		ownGlaive1, ownCatapult1 := icNPCSlotGlaiveThrower1A, icNPCSlotCatapult1A
		if team == ICTeamHorde {
			enemyGlaive1, enemyCatapult1 = icNPCSlotGlaiveThrower1A, icNPCSlotCatapult1A
			ownGlaive1, ownCatapult1 = icNPCSlotGlaiveThrower1H, icNPCSlotCatapult1H
		}
		for slot := enemyGlaive1; slot < enemyGlaive1+1; slot++ {
			if m := s.getICBGCreature(ic, slot); m != nil {
				if kit := s.getVehicleKit(ICMapID, m.InstanceID, m.GUID); kit != nil && kit.GetPassenger(0) == 0 {
					s.delICBGCreature(ctx, ic, slot)
				}
			}
		}
		for slot := enemyCatapult1; slot < enemyCatapult1+3; slot++ {
			if m := s.getICBGCreature(ic, slot); m != nil {
				if kit := s.getVehicleKit(ICMapID, m.InstanceID, m.GUID); kit != nil && kit.GetPassenger(0) == 0 {
					s.delICBGCreature(ctx, ic, slot)
				}
			}
		}
		for i := 0; i < 2; i++ {
			slot := ownGlaive1 + uint32(i)
			if m := s.getICBGCreature(ic, slot); m != nil && m.Health > 0 {
				continue
			}
			entry := ICCreatureGlaiveThrowerA
			if team == ICTeamHorde {
				entry = ICCreatureGlaiveThrowerH
			}
			s.addICBGCreature(ctx, ic, slot, entry, icDocksGlaivePositions[i], team)
		}
		for i := 0; i < 4; i++ {
			slot := ownCatapult1 + uint32(i)
			if m := s.getICBGCreature(ic, slot); m != nil && m.Health > 0 {
				continue
			}
			s.addICBGCreature(ctx, ic, slot, ICCreatureCatapult, icDocksCatapultPositions[i], team)
		}
	case ICNodeWorkshop:
		if ic.WorkshopTimerMs < icWorkshopUpdateMs {
			ic.WorkshopTimerMs = icWorkshopUpdateMs
		}
		enemyDemolisher1, enemySiege := icNPCSlotDemolisher1H, icNPCSlotSiegeEngineH
		ownDemolisher1, ownSiege := icNPCSlotDemolisher1A, icNPCSlotSiegeEngineA
		if team == ICTeamHorde {
			enemyDemolisher1, enemySiege = icNPCSlotDemolisher1A, icNPCSlotSiegeEngineA
			ownDemolisher1, ownSiege = icNPCSlotDemolisher1H, icNPCSlotSiegeEngineH
		}
		for slot := enemyDemolisher1; slot < enemyDemolisher1+3; slot++ {
			if m := s.getICBGCreature(ic, slot); m != nil {
				if kit := s.getVehicleKit(ICMapID, m.InstanceID, m.GUID); kit != nil && !kit.IsVehicleInUse() {
					s.delICBGCreature(ctx, ic, slot)
				}
			}
		}
		for i := 0; i < 4; i++ {
			slot := ownDemolisher1 + uint32(i)
			if m := s.getICBGCreature(ic, slot); m != nil && m.Health > 0 {
				continue
			}
			s.addICBGCreature(ctx, ic, slot, ICCreatureDemolisher, icWorkshopVehiclePositions[i], team)
		}
		if m := s.getICBGCreature(ic, enemySiege); m != nil {
			if kit := s.getVehicleKit(ICMapID, m.InstanceID, m.GUID); kit != nil && !kit.IsVehicleInUse() {
				s.delICBGCreature(ctx, ic, enemySiege)
			}
		}
		if m := s.getICBGCreature(ic, ownSiege); m == nil || m.Health == 0 {
			entry := ICCreatureSiegeEngineA
			if team == ICTeamHorde {
				entry = ICCreatureSiegeEngineH
			}
			if m != nil && m.Health == 0 {
				s.delICBGCreature(ctx, ic, ownSiege)
			}
			if s.addICBGCreature(ctx, ic, ownSiege, entry, icWorkshopVehiclePositions[4], team) {
				if siege := s.getICBGCreature(ic, ownSiege); siege != nil {
					s.motionMu.Lock()
					siege.UnitFlags |= unitFlagNotSelectable | icUnitFlagCannotSwim
					flags := siege.UnitFlags
					s.motionMu.Unlock()
					s.broadcastCreatureValuesUpdateInInstance(ICMapID, siege.InstanceID, siege.GUID, map[int]uint32{unitFieldFlags: flags})
				}
			}
		}
	}
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

	// BattlegroundIC::HandleCapturedNodes (BattlegroundIC.cpp:662-791)
	// with recapture=false: the docks/workshop vehicle delete/spawn
	// arms. resolveICNodeCapture fires only from the 60s banner
	// timer, so the recapture=true defend path never reaches here.
	if nodeID == ICNodeDocks || nodeID == ICNodeWorkshop {
		s.captureICNodeVehicles(context.Background(), ic, nodeID, team)
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
		// BattlegroundIC::DestroyGate (BattlegroundIC.cpp:845-858):
		// the first destroyed gate of a keep spawns its faction
		// boss (Overlord Agmar for Horde gates, Halford Wyrmbane
		// for Alliance gates) at BG_IC_NpcSpawnlocs, when no
		// creature is currently registered in that slot. The
		// portcullis DoorOpen + keep-banner NOT_SELECTABLE clear
		// (:809-819) stay out: Go has no gameobject lookup by
		// entry to resolve the portcullis/banner GUIDs.
		if gateID <= ICHordeEastGate {
			if s.getICBGCreature(ic, icNPCSlotOverlordAgmar) == nil {
				s.addICBGCreature(context.Background(), ic, icNPCSlotOverlordAgmar, ICCreatureOverlordAgmar, icBossSpawnPositions[0], ICTeamHorde)
			}
		} else {
			if s.getICBGCreature(ic, icNPCSlotHalfordWyrmbane) == nil {
				s.addICBGCreature(context.Background(), ic, icNPCSlotHalfordWyrmbane, ICCreatureHalfordWyrmbane, icBossSpawnPositions[1], ICTeamAlliance)
			}
		}
	}
}

// handleICPlayerDeath decrements team reinforcements on death.
// Reference: BattlegroundIC::HandleKillPlayer (BattlegroundIC.cpp:405-419).
func (s *Server) handleICPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || (sess.player.Map != ICMapID && !sess.hasActiveBGMap(ICMapID)) {
		return
	}
	ic := s.getOrCreateICState(ICMapID)
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
	if s == nil || sess == nil || sess.player == nil || (sess.player.Map != ICMapID && !sess.hasActiveBGMap(ICMapID)) {
		return
	}
	ic := s.getOrCreateICState(ICMapID)
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
// gate). Called from the resurrect choke point, same membership class as the
// repop dispatch. Player.cpp:4722-4725 dispatches
// HandlePlayerResurrect on the player's battleground (InBattleground gate),
// never on the standing map, so the guard takes the membership leg like
// handleICPlayerDeath and the state resolves on ICMapID.
func (s *Server) applyICNodeAuras(sess *session) {
	if s == nil || sess == nil || sess.player == nil || (sess.player.Map != ICMapID && !sess.hasActiveBGMap(ICMapID)) {
		return
	}
	ic := s.getOrCreateICState(ICMapID)
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

	// BattlegroundIC::PostUpdateImpl (BattlegroundIC.cpp:110-166):
	// the docks/workshop 180s vehicle respawn sweep runs on the
	// same 1s cadence as this tick, ahead of the 45s resource
	// tick below — it must not wait for a resource boundary.
	s.tickICVehicleTimers(context.Background(), ic, elapsedMs)

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
	s.triggerBGEvent(ic.MapID, scripting.BGEventOnEnd, winner)
}

func (s *Server) handleICPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || (sess.player.Map != ICMapID && !sess.hasActiveBGMap(ICMapID)) {
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
