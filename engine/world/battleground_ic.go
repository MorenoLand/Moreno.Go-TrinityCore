package world

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Isle of Conquest (IoC) Constants mirroring TrinityCore BattlegroundIC.h / BattlegroundIC.cpp.
const (
	ICMapID uint32 = 628

	ICMaxReinforcements uint32 = 300

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
	Winner                 int8 // -1: ongoing, 0: Alliance, 1: Horde
	VehiclesDestroyed      map[uint64]uint32
	StopTicker             chan struct{}
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

func (ic *icBattlegroundState) initNodes() {
	// 0: Refinery
	ic.Nodes[ICNodeRefinery] = icNodeState{
		NodeType: ICNodeRefinery,
		Faction:  ICTeamNeutral,
		State:    ICNodeStateUncontrolled,
		Banners:  [4]uint32{ICGameObjectBannerRefineryA, ICGameObjectBannerRefineryACont, ICGameObjectBannerRefineryH, ICGameObjectBannerRefineryHCont},
	}
	// 1: Quarry
	ic.Nodes[ICNodeQuarry] = icNodeState{
		NodeType: ICNodeQuarry,
		Faction:  ICTeamNeutral,
		State:    ICNodeStateUncontrolled,
		Banners:  [4]uint32{ICGameObjectBannerQuarryA, ICGameObjectBannerQuarryACont, ICGameObjectBannerQuarryH, ICGameObjectBannerQuarryHCont},
	}
	// 2: Docks
	ic.Nodes[ICNodeDocks] = icNodeState{
		NodeType: ICNodeDocks,
		Faction:  ICTeamNeutral,
		State:    ICNodeStateUncontrolled,
		Banners:  [4]uint32{ICGameObjectBannerDocksA, ICGameObjectBannerDocksACont, ICGameObjectBannerDocksH, ICGameObjectBannerDocksHCont},
	}
	// 3: Hangar
	ic.Nodes[ICNodeHangar] = icNodeState{
		NodeType: ICNodeHangar,
		Faction:  ICTeamNeutral,
		State:    ICNodeStateUncontrolled,
		Banners:  [4]uint32{ICGameObjectBannerHangarA, ICGameObjectBannerHangarACont, ICGameObjectBannerHangarH, ICGameObjectBannerHangarHCont},
	}
	// 4: Workshop
	ic.Nodes[ICNodeWorkshop] = icNodeState{
		NodeType: ICNodeWorkshop,
		Faction:  ICTeamNeutral,
		State:    ICNodeStateUncontrolled,
		Banners:  [4]uint32{ICGameObjectBannerWorkshopA, ICGameObjectBannerWorkshopACont, ICGameObjectBannerWorkshopH, ICGameObjectBannerWorkshopHCont},
	}
	// 5: Graveyard A (Alliance Keep)
	ic.Nodes[ICNodeGraveyardA] = icNodeState{
		NodeType: ICNodeGraveyardA,
		Faction:  ICTeamAlliance,
		State:    ICNodeStateControlledA,
		Banners:  [4]uint32{ICGameObjectBannerGraveyardA_A, ICGameObjectBannerGraveyardA_ACont, ICGameObjectBannerGraveyardA_H, ICGameObjectBannerGraveyardA_HCont},
	}
	// 6: Graveyard H (Horde Keep)
	ic.Nodes[ICNodeGraveyardH] = icNodeState{
		NodeType: ICNodeGraveyardH,
		Faction:  ICTeamHorde,
		State:    ICNodeStateControlledH,
		Banners:  [4]uint32{ICGameObjectBannerGraveyardH_A, ICGameObjectBannerGraveyardH_ACont, ICGameObjectBannerGraveyardH_H, ICGameObjectBannerGraveyardH_HCont},
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
		return false
	}
	ic := s.getOrCreateICState(sess.player.Map)
	if ic == nil {
		return false
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return true
	}

	playerTeam := teamForRace(sess.player.Race)

	if nodeID, ok := ic.getNodeByBannerEntry(entry); ok {
		s.interactWithICBanner(ic, nodeID, playerTeam, sess.player.Name)
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
func (s *Server) interactWithICBanner(ic *icBattlegroundState, nodeID uint8, team uint32, playerName string) {
	node := &ic.Nodes[nodeID]
	nodeName := icNodeNames[nodeID]
	teamName := "Alliance"
	if team == ICTeamHorde {
		teamName = "Horde"
	}

	// If node is already controlled by this player's team and not contested: nothing to do
	if (team == ICTeamAlliance && node.State == ICNodeStateControlledA) ||
		(team == ICTeamHorde && node.State == ICNodeStateControlledH) {
		return
	}

	// DEFENDING: If the node is currently contested by the enemy, the original controller reclaims it
	if (team == ICTeamAlliance && node.State == ICNodeStateConflictH && node.Faction == ICTeamAlliance) ||
		(team == ICTeamHorde && node.State == ICNodeStateConflictA && node.Faction == ICTeamHorde) {
		if node.CaptureTimer != nil {
			node.CaptureTimer.Stop()
			node.CaptureTimer = nil
		}
		if team == ICTeamAlliance {
			node.State = ICNodeStateControlledA
		} else {
			node.State = ICNodeStateControlledH
		}
		s.sendICNodeWorldState(ic, nodeID)
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has defended %s for the %s!", playerName, nodeName, teamName))
		return
	}

	// ASSAULTING: Node is uncontrolled, controlled by enemy, or contested
	if node.CaptureTimer != nil {
		node.CaptureTimer.Stop()
		node.CaptureTimer = nil
	}

	if team == ICTeamAlliance {
		node.State = ICNodeStateConflictA
	} else {
		node.State = ICNodeStateConflictH
	}
	s.sendICNodeWorldState(ic, nodeID)
	s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has assaulted %s for the %s!", playerName, nodeName, teamName))

	// Start 60s capture timer
	node.CaptureTimer = time.AfterFunc(ic.BannerCaptureDuration, func() {
		s.resolveICNodeCapture(ic, nodeID, team)
	})
}

// resolveICNodeCapture finalizes ownership after 60s without defense.
func (s *Server) resolveICNodeCapture(ic *icBattlegroundState, nodeID uint8, team uint32) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	node := &ic.Nodes[nodeID]
	node.Faction = team
	if team == ICTeamAlliance {
		node.State = ICNodeStateControlledA
	} else {
		node.State = ICNodeStateControlledH
	}
	s.sendICNodeWorldState(ic, nodeID)

	nodeName := icNodeNames[nodeID]
	teamName := "Alliance"
	if team == ICTeamHorde {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("The %s has taken %s!", teamName, nodeName))
}

// DamageICGate applies damage to an IoC fortress gate, transitioning to Damaged and Destroyed.
// Reference: BattlegroundIC::DestroyGate (BattlegroundIC.cpp:520-560).
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
// Reference: BattlegroundIC::HandleKillPlayer (BattlegroundIC.cpp:406-420).
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
// Reference: BattlegroundIC::HandleKillUnit (BattlegroundIC.cpp:382-405).
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
		s.endIC(ic, int8(ICTeamHorde))

	case ICCreatureOverlordAgmar:
		// Horde General killed -> Alliance wins immediately!
		ic.HordeBossAlive = false
		s.broadcastBattlegroundMessage(ic.MapID, "Overlord Agmar has been slain! The Alliance is victorious!")
		s.endIC(ic, int8(ICTeamAlliance))

	case ICCreatureDemolisher, ICCreatureSiegeEngineA, ICCreatureSiegeEngineH,
		ICCreatureGlaiveThrowerA, ICCreatureGlaiveThrowerH, ICCreatureCatapult:
		ic.VehiclesDestroyed[sess.playerGUID]++
		s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("%s has destroyed a siege vehicle!", sess.player.Name))
	}
}

// TickICResources generates passive reinforcements (+1 every 45s) for controlled Refinery and Quarry.
// Reference: BattlegroundIC::PostUpdateImpl / IC_RESOURCE_TIME (BattlegroundIC.cpp:205-219).
func (s *Server) TickICResources(ic *icBattlegroundState) {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.Winner >= 0 {
		return
	}

	for _, nodeID := range []uint8{ICNodeRefinery, ICNodeQuarry} {
		node := &ic.Nodes[nodeID]
		if node.State == ICNodeStateControlledA {
			if ic.AllianceReinforcements < ICMaxReinforcements {
				ic.AllianceReinforcements++
				s.updateICWorldState(ic.MapID, ICWorldStateAllianceReinforcements, ic.AllianceReinforcements)
			}
		} else if node.State == ICNodeStateControlledH {
			if ic.HordeReinforcements < ICMaxReinforcements {
				ic.HordeReinforcements++
				s.updateICWorldState(ic.MapID, ICWorldStateHordeReinforcements, ic.HordeReinforcements)
			}
		}
	}
}

func (s *Server) endIC(ic *icBattlegroundState, winner int8) {
	if ic.Winner >= 0 {
		return
	}
	ic.Winner = winner
	winnerTeam := "Alliance"
	if winner == int8(ICTeamHorde) {
		winnerTeam = "Horde"
	}
	s.broadcastBattlegroundMessage(ic.MapID, fmt.Sprintf("The %s has won the battle for Isle of Conquest!", winnerTeam))
}

func (s *Server) handleICPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != ICMapID {
		return
	}
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

func (s *Server) updateICWorldState(mapID, variableID, value uint32) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID {
			sess.sendWorldState(variableID, value)
		}
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
