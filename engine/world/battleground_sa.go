package world

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Strand of the Ancients (SotA) Constants mirroring TrinityCore BattlegroundSA.h / BattlegroundSA.cpp.
const (
	SAMapID uint32 = 607

	SAWarmupLength       = 120 * time.Second // 2 minutes warmup
	SARoundLength        = 600 * time.Second // 10 minutes round
	SASecondWarmupLength = 60 * time.Second  // 1 minute second warmup
	SABoatStart          = 60 * time.Second  // 1 minute boat start

	// Teams
	SATeamAlliance uint32 = 0
	SATeamHorde    uint32 = 1
	SATeamNone     uint32 = 2

	// Statuses / Phases
	SAStatusNotStarted   uint8 = 0
	SAStatusWarmup       uint8 = 1
	SAStatusRoundOne     uint8 = 2
	SAStatusSecondWarmup uint8 = 3
	SAStatusRoundTwo     uint8 = 4
	SAStatusFinished     uint8 = 5

	// Gate States
	SAGateOK        uint8 = 1
	SAGateDamaged   uint8 = 2
	SAGateDestroyed uint8 = 3

	// Gates (0..5)
	SAGateGreen   uint8 = 0
	SAGateYellow  uint8 = 1
	SAGateBlue    uint8 = 2
	SAGateRed     uint8 = 3
	SAGatePurple  uint8 = 4
	SAGateAncient uint8 = 5
	SAMaxGates          = 6

	// Graveyards (0..4)
	SABeachGY             uint8 = 0
	SADefenderLastGY      uint8 = 1
	SARightCapturableGY   uint8 = 2
	SALeftCapturableGY    uint8 = 3
	SACentralCapturableGY uint8 = 4
	SAMaxGY                     = 5

	// GameObjects
	SAGameObjectGateGreen   uint32 = 190722 // Gate of the Green Emerald
	SAGameObjectGateYellow  uint32 = 190727 // Gate of the Yellow Moon
	SAGameObjectGateBlue    uint32 = 190724 // Gate of the Blue Sapphire
	SAGameObjectGateRed     uint32 = 190726 // Gate of the Red Sun
	SAGameObjectGatePurple  uint32 = 190723 // Gate of the Purple Amethyst
	SAGameObjectGateAncient uint32 = 192549 // Chamber of Ancient Relics
	SAGameObjectTitanRelic  uint32 = 192834 // Titan Relic
	SAGameObjectTitanRelic2 uint32 = 192829 // Titan Relic alternate entry

	// Graveyard Flags
	// Banners: Right (191306 A / 191305 H), Left (191308 A / 191307 H), Central (191310 A / 191309 H)
	SAGameObjectFlagRightA   uint32 = 191306
	SAGameObjectFlagRightH   uint32 = 191305
	SAGameObjectFlagLeftA    uint32 = 191308
	SAGameObjectFlagLeftH    uint32 = 191307
	SAGameObjectFlagCentralA uint32 = 191310
	SAGameObjectFlagCentralH uint32 = 191309

	// Bombs
	SAGameObjectBomb uint32 = 190753 // Seaforium Bomb

	// Creatures
	SACreatureDemolisher       uint32 = 28781 // Demolisher SA
	SACreatureCannon           uint32 = 27894 // Anti-Personnel Cannon
	SACreatureRiggerSparklight uint32 = 29260 // Used Demolisher Salesman (East)
	SACreatureGorgrilRigspark  uint32 = 29262 // Used Demolisher Salesman (West)
	SACreatureKanrethad        uint32 = 29

	// World States
	SAWorldStateTimerMins            uint32 = 3559
	SAWorldStateTimerSecTens         uint32 = 3560
	SAWorldStateTimerSecDecs         uint32 = 3561
	SAWorldStateAllyAttacks          uint32 = 4352
	SAWorldStateHordeAttacks         uint32 = 4353
	SAWorldStatePurpleGate           uint32 = 3614
	SAWorldStateRedGate              uint32 = 3617
	SAWorldStateBlueGate             uint32 = 3620
	SAWorldStateGreenGate            uint32 = 3623
	SAWorldStateYellowGate           uint32 = 3638
	SAWorldStateAncientGate          uint32 = 3849
	SAWorldStateLeftGYAlliance       uint32 = 3635
	SAWorldStateRightGYAlliance      uint32 = 3636
	SAWorldStateCenterGYAlliance     uint32 = 3637
	SAWorldStateRightAttTokenAll     uint32 = 3627
	SAWorldStateLeftAttTokenAll      uint32 = 3626
	SAWorldStateLeftAttTokenHrd      uint32 = 3629
	SAWorldStateRightAttTokenHrd     uint32 = 3628
	SAWorldStateHordeDefenceToken    uint32 = 3631
	SAWorldStateAllianceDefenceToken uint32 = 3630
	SAWorldStateRightGYHorde         uint32 = 3632
	SAWorldStateLeftGYHorde          uint32 = 3633
	SAWorldStateCenterGYHorde        uint32 = 3634
	SAWorldStateBonusTimer           uint32 = 3571
	SAWorldStateEnableTimer          uint32 = 3564

	// Sounds
	SASoundWallAttackedAlliance  uint32 = 15912
	SASoundWallAttackedHorde     uint32 = 15911
	SASoundWallDestroyedAlliance uint32 = 15909
	SASoundWallDestroyedHorde    uint32 = 15910
	SASoundVictoryAlliance       uint32 = 15907
	SASoundVictoryHorde          uint32 = 15906

	// Default Gate Health
	SAGateDefaultMaxHealth uint32 = 100000
)

type saGateInfo struct {
	GateID       uint8
	GameObjectId uint32
	WorldState   uint32
	Name         string
	DamagedMsg   string
	DestroyedMsg string
}

var saGates = [SAMaxGates]saGateInfo{
	{SAGateGreen, SAGameObjectGateGreen, SAWorldStateGreenGate, "Gate of the Green Emerald", "Gate of the Green Emerald is under attack!", "Gate of the Green Emerald has been destroyed!"},
	{SAGateYellow, SAGameObjectGateYellow, SAWorldStateYellowGate, "Gate of the Yellow Moon", "Gate of the Yellow Moon is under attack!", "Gate of the Yellow Moon has been destroyed!"},
	{SAGateBlue, SAGameObjectGateBlue, SAWorldStateBlueGate, "Gate of the Blue Sapphire", "Gate of the Blue Sapphire is under attack!", "Gate of the Blue Sapphire has been destroyed!"},
	{SAGateRed, SAGameObjectGateRed, SAWorldStateRedGate, "Gate of the Red Sun", "Gate of the Red Sun is under attack!", "Gate of the Red Sun has been destroyed!"},
	{SAGatePurple, SAGameObjectGatePurple, SAWorldStatePurpleGate, "Gate of the Purple Amethyst", "Gate of the Purple Amethyst is under attack!", "Gate of the Purple Amethyst has been destroyed!"},
	{SAGateAncient, SAGameObjectGateAncient, SAWorldStateAncientGate, "Chamber of Ancient Relics", "Chamber of Ancient Relics is under attack!", "Chamber of Ancient Relics has been breached!"},
}

type saGateState struct {
	State     uint8 // 1: OK, 2: Damaged, 3: Destroyed
	Health    uint32
	MaxHealth uint32
}

type saRoundScore struct {
	Winner uint32        // 0: Alliance, 1: Horde
	Time   time.Duration // Time taken to reach relic (or SARoundLength if reached cap)
}

type saBattlegroundState struct {
	mu                   sync.Mutex
	MapID                uint32
	Status               uint8
	Attackers            uint32 // 0: Alliance, 1: Horde
	TotalTime            time.Duration
	EndRoundTimer        time.Duration
	TimerEnabled         bool
	WarmupLength         time.Duration
	RoundLength          time.Duration
	SecondWarmupLength   time.Duration
	Gates                [SAMaxGates]saGateState
	Graveyards           [SAMaxGY]uint32 // Owner: 0: Alliance, 1: Horde
	RoundScores          [2]saRoundScore
	DemolishersAlive     [2]bool
	GateDestroyed        bool
	Winner               int8 // -1: ongoing, 0: Alliance, 1: Horde, 2: Draw
	DemolishersDestroyed map[uint64]uint32
	GatesDestroyed       map[uint64]uint32
	StopTicker           chan struct{}
	// RoundTwoWarnTimer models BattlegroundSA::PostUpdateImpl's InitSecondRound /
	// UpdateWaitTimer arm: 5s into the second warmup the "round 2 starts in one
	// minute" broadcast fires (BG_SA_TEXT_ROUND_TWO_START_ONE_MINUTE); at 30s the
	// half-minute broadcast fires (BG_SA_TEXT_ROUND_TWO_START_HALF_MINUTE).
	RoundTwoWarnTimer     time.Duration
	RoundTwoWarned        bool
	RoundTwoHalfMinWarned bool
}

func isSAGameObject(entry uint32) bool {
	switch entry {
	case SAGameObjectGateGreen,
		SAGameObjectGateYellow,
		SAGameObjectGateBlue,
		SAGameObjectGateRed,
		SAGameObjectGatePurple,
		SAGameObjectGateAncient,
		SAGameObjectTitanRelic,
		SAGameObjectTitanRelic2,
		SAGameObjectFlagRightA,
		SAGameObjectFlagRightH,
		SAGameObjectFlagLeftA,
		SAGameObjectFlagLeftH,
		SAGameObjectFlagCentralA,
		SAGameObjectFlagCentralH,
		SAGameObjectBomb:
		return true
	}
	return false
}

func (s *Server) getOrCreateSAState(mapID uint32) *saBattlegroundState {
	if s == nil {
		return nil
	}
	s.saMu.Lock()
	defer s.saMu.Unlock()
	if s.saState == nil {
		s.saState = make(map[uint32]*saBattlegroundState)
	}
	state := s.saState[mapID]
	if state == nil {
		// Reference: BattlegroundSA::Reset (BattlegroundSA.cpp:78): the first
		// attackers are picked with urand(0, 1); rounds after that swap sides.
		attackers := SATeamAlliance
		if rand.Intn(2) == 1 {
			attackers = SATeamHorde
		}
		state = &saBattlegroundState{
			MapID:                mapID,
			Status:               SAStatusWarmup,
			Attackers:            attackers,
			WarmupLength:         SAWarmupLength,
			RoundLength:          SARoundLength,
			SecondWarmupLength:   SASecondWarmupLength,
			EndRoundTimer:        SARoundLength,
			Winner:               -1,
			DemolishersDestroyed: make(map[uint64]uint32),
			GatesDestroyed:       make(map[uint64]uint32),
			StopTicker:           make(chan struct{}),
		}
		state.DemolishersAlive[0] = true
		state.DemolishersAlive[1] = true
		state.resetObjects()
		s.saState[mapID] = state
	}
	return state
}

// resetObjects resets gates, graveyards, and demolishers for a new round.
// Reference: BattlegroundSA::ResetObjs (BattlegroundSA.cpp:99-292).
func (sa *saBattlegroundState) resetObjects() {
	for i := 0; i < SAMaxGates; i++ {
		sa.Gates[i] = saGateState{
			State:     SAGateOK,
			Health:    SAGateDefaultMaxHealth,
			MaxHealth: SAGateDefaultMaxHealth,
		}
	}
	defenders := sa.defenders()
	sa.Graveyards[SABeachGY] = sa.Attackers
	sa.Graveyards[SADefenderLastGY] = defenders
	sa.Graveyards[SARightCapturableGY] = defenders
	sa.Graveyards[SALeftCapturableGY] = defenders
	sa.Graveyards[SACentralCapturableGY] = defenders
}

func (sa *saBattlegroundState) defenders() uint32 {
	if sa.Attackers == SATeamAlliance {
		return SATeamHorde
	}
	return SATeamAlliance
}

// canInteractWithObject enforces TrinityCore gate-unlocking prerequisites:
// - Left and Right flags require Green Gate OR Blue Gate destroyed.
// - Central flag requires Red Gate OR Purple Gate destroyed.
// - Titan Relic requires Yellow Gate AND Ancient Gate destroyed.
// Reference: BattlegroundSA::CanInteractWithObject (BattlegroundSA.cpp:737-760).
func (sa *saBattlegroundState) canInteractWithObject(entry uint32) bool {
	greenDestroyed := sa.Gates[SAGateGreen].State == SAGateDestroyed
	blueDestroyed := sa.Gates[SAGateBlue].State == SAGateDestroyed
	redDestroyed := sa.Gates[SAGateRed].State == SAGateDestroyed
	purpleDestroyed := sa.Gates[SAGatePurple].State == SAGateDestroyed
	yellowDestroyed := sa.Gates[SAGateYellow].State == SAGateDestroyed
	ancientDestroyed := sa.Gates[SAGateAncient].State == SAGateDestroyed

	switch entry {
	case SAGameObjectFlagLeftA, SAGameObjectFlagLeftH,
		SAGameObjectFlagRightA, SAGameObjectFlagRightH:
		return greenDestroyed || blueDestroyed

	case SAGameObjectFlagCentralA, SAGameObjectFlagCentralH:
		return redDestroyed || purpleDestroyed

	case SAGameObjectTitanRelic, SAGameObjectTitanRelic2:
		// Reference: BattlegroundSA::CanInteractWithObject (BattlegroundSA.cpp:737-760):
		// the TITAN_RELIC case falls through into CENTRAL_FLAG and then LEFT/RIGHT_FLAG,
		// so the relic needs yellow AND ancient destroyed plus red-or-purple plus
		// green-or-blue destroyed.
		return yellowDestroyed && ancientDestroyed && (redDestroyed || purpleDestroyed) && (greenDestroyed || blueDestroyed)

	default:
		return true
	}
}

// handleSAGameObjectUse handles player interaction with flags and the Titan Relic.
// Reference: BattlegroundSA::EventPlayerClickedOnFlag / TitanRelicActivated (BattlegroundSA.cpp:780-958).
func (s *Server) handleSAGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return false
	}
	sa := s.getOrCreateSAState(sess.player.Map)
	if sa == nil {
		return false
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if sa.Winner >= 0 {
		return true
	}

	playerTeam := teamForRace(sess.player.Race)

	switch entry {
	case SAGameObjectFlagLeftA, SAGameObjectFlagLeftH:
		if !sa.canInteractWithObject(entry) {
			return true
		}
		if playerTeam == sa.Attackers && sa.Graveyards[SALeftCapturableGY] != playerTeam {
			sa.captureGraveyard(s, SALeftCapturableGY, playerTeam, sess.player.Name)
		}

	case SAGameObjectFlagRightA, SAGameObjectFlagRightH:
		if !sa.canInteractWithObject(entry) {
			return true
		}
		if playerTeam == sa.Attackers && sa.Graveyards[SARightCapturableGY] != playerTeam {
			sa.captureGraveyard(s, SARightCapturableGY, playerTeam, sess.player.Name)
		}

	case SAGameObjectFlagCentralA, SAGameObjectFlagCentralH:
		if !sa.canInteractWithObject(entry) {
			return true
		}
		if playerTeam == sa.Attackers && sa.Graveyards[SACentralCapturableGY] != playerTeam {
			sa.captureGraveyard(s, SACentralCapturableGY, playerTeam, sess.player.Name)
		}

	case SAGameObjectTitanRelic, SAGameObjectTitanRelic2:
		if !sa.canInteractWithObject(entry) {
			return true
		}
		if playerTeam != sa.Attackers {
			return true
		}
		if sa.Status == SAStatusRoundOne || sa.Status == SAStatusRoundTwo {
			sa.activateTitanRelic(s, sess)
		}

	case SAGameObjectBomb:
		// Seaforium bomb pickup - can be picked up by attackers
		if playerTeam == sa.Attackers {
			s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("%s has picked up a Seaforium Bomb!", sess.player.Name))
		}
	}

	return true
}

// captureGraveyard processes capturing of West, East, or Central GY by attackers.
// Reference: BattlegroundSA::CaptureGraveyard (BattlegroundSA.cpp:804-891).
func (sa *saBattlegroundState) captureGraveyard(s *Server, gyID uint8, team uint32, playerName string) {
	sa.Graveyards[gyID] = team
	teamName := "Alliance"
	if team == SATeamHorde {
		teamName = "Horde"
	}

	switch gyID {
	case SALeftCapturableGY:
		s.updateSAWorldState(sa.MapID, SAWorldStateLeftGYAlliance, boolToUint32(team == SATeamAlliance))
		s.updateSAWorldState(sa.MapID, SAWorldStateLeftGYHorde, boolToUint32(team == SATeamHorde))
		s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("%s has captured the West Graveyard for the %s!", playerName, teamName))
		s.broadcastBattlegroundMessage(sa.MapID, "Gorgril Rigspark arrives to reinforce the attackers with additional Demolishers!")

	case SARightCapturableGY:
		s.updateSAWorldState(sa.MapID, SAWorldStateRightGYAlliance, boolToUint32(team == SATeamAlliance))
		s.updateSAWorldState(sa.MapID, SAWorldStateRightGYHorde, boolToUint32(team == SATeamHorde))
		s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("%s has captured the East Graveyard for the %s!", playerName, teamName))
		s.broadcastBattlegroundMessage(sa.MapID, "Rigger Sparklight arrives to reinforce the attackers with additional Demolishers!")

	case SACentralCapturableGY:
		s.updateSAWorldState(sa.MapID, SAWorldStateCenterGYAlliance, boolToUint32(team == SATeamAlliance))
		s.updateSAWorldState(sa.MapID, SAWorldStateCenterGYHorde, boolToUint32(team == SATeamHorde))
		s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("%s has captured the South Graveyard for the %s!", playerName, teamName))
	}
}

// activateTitanRelic handles round conclusion upon clicking the Chamber relic.
// Reference: BattlegroundSA::TitanRelicActivated (BattlegroundSA.cpp:893-958).
func (sa *saBattlegroundState) activateTitanRelic(s *Server, sess *session) {
	teamName := "Alliance"
	if sess != nil && teamForRace(sess.player.Race) == SATeamHorde {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("The %s has captured the Titan Relic!", teamName))

	if sa.Status == SAStatusRoundOne {
		sa.RoundScores[0].Winner = sa.Attackers
		sa.RoundScores[0].Time = sa.TotalTime

		// Swap attacker/defender roles
		sa.Attackers = sa.defenders()
		sa.Status = SAStatusSecondWarmup
		sa.TotalTime = 0
		sa.TimerEnabled = false
		// Reference: BattlegroundSA::TitanRelicActivated round-one arm
		// (BattlegroundSA.cpp:893-958): UpdateWaitTimer = 5000, SignaledRoundTwo =
		// SignaledRoundTwoHalfMin = false, InitSecondRound = true.
		sa.RoundTwoWarnTimer = 5 * time.Second
		sa.RoundTwoWarned = false
		sa.RoundTwoHalfMinWarned = false
		sa.resetObjects()
		s.sendSAAllWorldStates(sa)

		s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("Round 1 finished! Round 2 will begin shortly. The %s will now attack!", sa.attackerTeamName()))
	} else if sa.Status == SAStatusRoundTwo {
		sa.RoundScores[1].Winner = sa.Attackers
		sa.RoundScores[1].Time = sa.TotalTime

		// Determine winner based on time to capture
		if sa.RoundScores[0].Time == sa.RoundScores[1].Time {
			s.endSA(sa, 2) // Draw
		} else if sa.RoundScores[0].Time < sa.RoundScores[1].Time {
			s.endSA(sa, int8(sa.RoundScores[0].Winner))
		} else {
			s.endSA(sa, int8(sa.RoundScores[1].Winner))
		}
	}
}

func (sa *saBattlegroundState) attackerTeamName() string {
	if sa.Attackers == SATeamAlliance {
		return "Alliance"
	}
	return "Horde"
}

// DamageGate applies damage to a SotA gate, handling damaged threshold and destruction.
// Reference: BattlegroundSA::ProcessEvent / DestroyGate (BattlegroundSA.cpp:550-638).
func (s *Server) DamageGate(mapID uint32, gateID uint8, damage uint32, attacker *session) {
	if s == nil || gateID >= SAMaxGates {
		return
	}
	sa := s.getOrCreateSAState(mapID)
	if sa == nil {
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if sa.Winner >= 0 || (sa.Status != SAStatusRoundOne && sa.Status != SAStatusRoundTwo) {
		return
	}

	gate := &sa.Gates[gateID]
	if gate.State == SAGateDestroyed {
		return
	}

	if damage >= gate.Health {
		gate.Health = 0
	} else {
		gate.Health -= damage
	}

	info := saGates[gateID]

	// Damaged state (health <= 50%)
	if gate.Health <= gate.MaxHealth/2 && gate.Health > 0 && gate.State == SAGateOK {
		gate.State = SAGateDamaged
		s.updateSAWorldState(mapID, info.WorldState, uint32(SAGateDamaged))
		s.broadcastBattlegroundMessage(mapID, info.DamagedMsg)
	}

	// Destroyed state
	if gate.Health == 0 && gate.State != SAGateDestroyed {
		gate.State = SAGateDestroyed
		sa.GateDestroyed = true
		s.updateSAWorldState(mapID, info.WorldState, uint32(SAGateDestroyed))
		s.broadcastBattlegroundMessage(mapID, info.DestroyedMsg)

		if attacker != nil {
			sa.GatesDestroyed[attacker.playerGUID]++
		}
	}
}

// TickSA updates Strand of the Ancients timers, handles round transitions, and checks expirations.
// Reference: BattlegroundSA::PostUpdateImpl (BattlegroundSA.cpp:319-438).
func (s *Server) TickSA(sa *saBattlegroundState, delta time.Duration) {
	if sa == nil {
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if sa.Winner >= 0 {
		return
	}

	sa.TotalTime += delta

	switch sa.Status {
	case SAStatusWarmup:
		sa.EndRoundTimer = sa.RoundLength
		if sa.TotalTime >= sa.WarmupLength {
			sa.Status = SAStatusRoundOne
			sa.TotalTime = 0
			sa.TimerEnabled = true
			s.broadcastBattlegroundMessage(sa.MapID, "Round 1 has begun! Destroy the gates and reach the Titan Relic!")
			s.sendSAAllWorldStates(sa)
		}

	case SAStatusSecondWarmup:
		// Round 2 timer is capped to Round 1 time if Round 1 attacker succeeded!
		if sa.RoundScores[0].Time < sa.RoundLength {
			sa.EndRoundTimer = sa.RoundScores[0].Time
		} else {
			sa.EndRoundTimer = sa.RoundLength
		}

		// Reference: BattlegroundSA::PostUpdateImpl's InitSecondRound/UpdateWaitTimer
		// arm plus the 30s SignaledRoundTwoHalfMin arm (BattlegroundSA.cpp:319-438).
		if sa.RoundTwoWarnTimer > 0 {
			sa.RoundTwoWarnTimer -= delta
			if sa.RoundTwoWarnTimer <= 0 && !sa.RoundTwoWarned {
				sa.RoundTwoWarned = true
				s.broadcastBattlegroundMessage(sa.MapID, "Round 2 will begin in one minute!")
			}
		}
		if !sa.RoundTwoHalfMinWarned && sa.TotalTime >= 30*time.Second {
			sa.RoundTwoHalfMinWarned = true
			s.broadcastBattlegroundMessage(sa.MapID, "Round 2 will begin in 30 seconds!")
		}

		if sa.TotalTime >= sa.SecondWarmupLength {
			sa.Status = SAStatusRoundTwo
			sa.TotalTime = 0
			sa.TimerEnabled = true
			s.broadcastBattlegroundMessage(sa.MapID, "Round 2 has begun! You must beat the first round's time to claim victory!")
			s.sendSAAllWorldStates(sa)
		}

	case SAStatusRoundOne:
		s.sendSATime(sa)
		// Check round 1 timer expiration (Defenders held for full 10m)
		if sa.TotalTime >= sa.RoundLength {
			// Reference: BattlegroundSA::PostUpdateImpl round-one expiry arm
			// (BattlegroundSA.cpp:319-438): RoundScores[0].winner = Attackers (the
			// round-one attackers, who failed to reach the relic in time), then the
			// roles swap and the second warmup starts with the 5s round-two warning
			// timer armed (UpdateWaitTimer = 5000, InitSecondRound = true).
			sa.RoundScores[0].Winner = sa.Attackers
			sa.RoundScores[0].Time = sa.RoundLength

			sa.Attackers = sa.defenders()
			sa.Status = SAStatusSecondWarmup
			sa.TotalTime = 0
			sa.TimerEnabled = false
			sa.RoundTwoWarnTimer = 5 * time.Second
			sa.RoundTwoWarned = false
			sa.RoundTwoHalfMinWarned = false
			sa.resetObjects()
			s.sendSAAllWorldStates(sa)

			s.broadcastBattlegroundMessage(sa.MapID, "Round 1 has ended! The defenders held the keep. Round 2 will begin shortly!")
		}

	case SAStatusRoundTwo:
		s.sendSATime(sa)
		// Check round 2 timer expiration
		if sa.TotalTime >= sa.EndRoundTimer {
			sa.RoundScores[1].Winner = sa.defenders()
			sa.RoundScores[1].Time = sa.RoundLength

			// Winner determination
			if sa.RoundScores[0].Time == sa.RoundScores[1].Time {
				s.endSA(sa, 2) // Draw
			} else if sa.RoundScores[0].Time < sa.RoundScores[1].Time {
				s.endSA(sa, int8(sa.RoundScores[0].Winner))
			} else {
				s.endSA(sa, int8(sa.RoundScores[1].Winner))
			}
		}
	}
}

// sendSATime broadcasts the remaining round timer in minutes, tens of seconds, and single seconds.
// Reference: BattlegroundSA::SendTime (BattlegroundSA.cpp:729-735).
func (s *Server) sendSATime(sa *saBattlegroundState) {
	var remaining time.Duration
	if sa.TotalTime < sa.EndRoundTimer {
		remaining = sa.EndRoundTimer - sa.TotalTime
	} else {
		remaining = 0
	}

	totalSec := uint32(remaining.Seconds())
	mins := totalSec / 60
	tens := (totalSec % 60) / 10
	decs := (totalSec % 60) % 10

	s.updateSAWorldState(sa.MapID, SAWorldStateTimerMins, mins)
	s.updateSAWorldState(sa.MapID, SAWorldStateTimerSecTens, tens)
	s.updateSAWorldState(sa.MapID, SAWorldStateTimerSecDecs, decs)
}

func (s *Server) endSA(sa *saBattlegroundState, winner int8) {
	if sa.Winner >= 0 {
		return
	}
	sa.Winner = winner
	sa.Status = SAStatusFinished
	sa.TimerEnabled = false
	s.updateSAWorldState(sa.MapID, SAWorldStateEnableTimer, 0)

	switch winner {
	case 0:
		s.broadcastBattlegroundMessage(sa.MapID, "The Alliance is victorious in Strand of the Ancients!")
	case 1:
		s.broadcastBattlegroundMessage(sa.MapID, "The Horde is victorious in Strand of the Ancients!")
	default:
		s.broadcastBattlegroundMessage(sa.MapID, "The battle ended in a draw!")
	}

	// Reference: BattlegroundSA::EndBattleground (BattlegroundSA.cpp:965): the
	// winning team gets GetBonusHonorFromKill(1), then BOTH teams get the
	// completion honor GetBonusHonorFromKill(2), ahead of Battleground::EndBattleground.
	// The WIN_BG / COMPLETE_BATTLEGROUND criteria arms of the base-class
	// EndBattleground are covered by creditBattlegroundWin (same pattern as the
	// AB/EotS/IoC victories). On a draw there is no winning team to credit:
	// C++'s draw path calls EndBattleground(0), which collides with TEAM_ALLIANCE=0
	// and grants Alliance the winner honor and the Alliance-wins announcement as
	// an upstream quirk; Go keeps the draw a true draw instead of replicating it.
	if winner == 0 || winner == 1 {
		s.creditBattlegroundWin(sa.MapID, uint32(winner))
		s.rewardBGEndHonor(sa.MapID, uint32(winner), 1)
	}
	s.rewardBGEndHonor(sa.MapID, 0, 2)
	s.rewardBGEndHonor(sa.MapID, 1, 2)
}

// handleSACreatureKilled tracks Demolisher kills and marks AllVehiclesAlive false.
// Reference: BattlegroundSA::HandleKillUnit (BattlegroundSA.cpp:641-648).
func (s *Server) handleSACreatureKilled(sess *session, creatureEntry uint32) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != SAMapID {
		return
	}
	sa := s.getOrCreateSAState(sess.player.Map)
	if sa == nil {
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if creatureEntry == SACreatureDemolisher {
		sa.DemolishersDestroyed[sess.playerGUID]++
		sa.DemolishersAlive[sa.Attackers] = false
		s.broadcastBattlegroundMessage(sa.MapID, fmt.Sprintf("%s has destroyed a Demolisher!", sess.player.Name))
	}
}

// handleSAPlayerDeath processes player death in SotA.
func (s *Server) handleSAPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != SAMapID {
		return
	}
	// Player deaths do not deduct reinforcements in SotA, but can trigger stats/achievements
}

// handleSAPlayerLeave processes player departure from SotA.
func (s *Server) handleSAPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || sess.player.Map != SAMapID {
		return
	}
}

// updateSABattles ticks every live SA state, driving the warmup/round timers and
// round transitions (BattlegroundSA::PostUpdateImpl). BattlegroundMgr::Update
// (BattlegroundMgr.cpp:94) sweeps all running instances with bg->Update(diff)
// every BATTLEGROUND_OBJECTIVE_UPDATE_INTERVAL (BattlegroundMgr.h:38 = 1000ms);
// the 1s gate here mirrors that cadence, like
// updateArenaBattles/updateAVBattles/updateEOTSBattles.
func (s *Server) updateSABattles(now time.Time) {
	if s == nil {
		return
	}
	if !s.saTickLast.IsZero() && now.Sub(s.saTickLast) < time.Second {
		return
	}
	var elapsed time.Duration = time.Second
	if !s.saTickLast.IsZero() {
		elapsed = now.Sub(s.saTickLast)
	}
	s.saTickLast = now
	s.saMu.RLock()
	battles := make([]*saBattlegroundState, 0, len(s.saState))
	for _, sa := range s.saState {
		battles = append(battles, sa)
	}
	s.saMu.RUnlock()
	for _, sa := range battles {
		s.TickSA(sa, elapsed)
	}
}

// SAGraveyardIDs mirrors BG_SA_GYEntries (BattlegroundSA.h:466): beach,
// defender-last, right capturable, left capturable, central capturable.
var SAGraveyardIDs = [SAMaxGY]uint32{1350, 1349, 1347, 1346, 1348}

// closestSAGraveyard mirrors BattlegroundSA::GetClosestGraveyard
// (BattlegroundSA.cpp:693-727): attackers fall back to the beach graveyard,
// defenders to the defender-last graveyard, and either side prefers the nearest
// capturable graveyard it owns, by 3D exact distance like C++'s GetExactDistSq.
func (s *Server) closestSAGraveyard(x, y, z float32, team uint32) (wotlk.WorldSafeLoc, bool) {
	var teamIndex uint32
	switch team {
	case teamAlliance:
		teamIndex = SATeamAlliance
	case teamHorde:
		teamIndex = SATeamHorde
	default:
		return wotlk.WorldSafeLoc{}, false
	}
	if s == nil || s.Data == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	sa := s.getOrCreateSAState(SAMapID)
	if sa == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	sa.mu.Lock()
	attackers := sa.Attackers
	owners := sa.Graveyards
	sa.mu.Unlock()

	base := SABeachGY
	if teamIndex != attackers {
		base = SADefenderLastGY
	}

	var best wotlk.WorldSafeLoc
	bestDist := float32(999999.0)
	found := false
	consider := func(id uint32) {
		if id == 0 {
			return
		}
		loc, ok, err := s.Data.WorldSafeLoc(id)
		if err != nil || !ok {
			return
		}
		dx, dy, dz := loc.X-x, loc.Y-y, loc.Z-z
		if dist := dx*dx + dy*dy + dz*dz; !found || dist < bestDist {
			best, bestDist, found = loc, dist, true
		}
	}
	consider(SAGraveyardIDs[base])
	for i := SARightCapturableGY; i < SAMaxGY; i++ {
		if owners[i] != teamIndex {
			continue
		}
		consider(SAGraveyardIDs[i])
	}
	if found {
		return best, true
	}
	return wotlk.WorldSafeLoc{}, false
}

func (s *Server) updateSAWorldState(mapID, variableID, value uint32) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID {
			sess.sendWorldState(variableID, value)
		}
	}
}

func (s *Server) sendSAAllWorldStates(sa *saBattlegroundState) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == sa.MapID {
			s.sendSAInitialWorldStatesToSession(sa, sess)
		}
	}
}

func (s *Server) sendSAInitialWorldStates(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	sa := s.getOrCreateSAState(sess.player.Map)
	if sa == nil {
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()
	s.sendSAInitialWorldStatesToSession(sa, sess)
}

func (s *Server) sendSAInitialWorldStatesToSession(sa *saBattlegroundState, sess *session) {
	allyAttacks := boolToUint32(sa.Attackers == SATeamAlliance)
	hordeAttacks := boolToUint32(sa.Attackers == SATeamHorde)

	// Gates
	for i := 0; i < SAMaxGates; i++ {
		sess.sendWorldState(saGates[i].WorldState, uint32(sa.Gates[i].State))
	}

	// Roles & Tokens
	sess.sendWorldState(SAWorldStateAllyAttacks, allyAttacks)
	sess.sendWorldState(SAWorldStateHordeAttacks, hordeAttacks)
	sess.sendWorldState(SAWorldStateRightAttTokenAll, allyAttacks)
	sess.sendWorldState(SAWorldStateLeftAttTokenAll, allyAttacks)
	sess.sendWorldState(SAWorldStateRightAttTokenHrd, hordeAttacks)
	sess.sendWorldState(SAWorldStateLeftAttTokenHrd, hordeAttacks)
	sess.sendWorldState(SAWorldStateHordeDefenceToken, allyAttacks)
	sess.sendWorldState(SAWorldStateAllianceDefenceToken, hordeAttacks)

	// Graveyards
	sess.sendWorldState(SAWorldStateRightGYHorde, boolToUint32(sa.Graveyards[SARightCapturableGY] == SATeamHorde))
	sess.sendWorldState(SAWorldStateLeftGYHorde, boolToUint32(sa.Graveyards[SALeftCapturableGY] == SATeamHorde))
	sess.sendWorldState(SAWorldStateCenterGYHorde, boolToUint32(sa.Graveyards[SACentralCapturableGY] == SATeamHorde))
	sess.sendWorldState(SAWorldStateRightGYAlliance, boolToUint32(sa.Graveyards[SARightCapturableGY] == SATeamAlliance))
	sess.sendWorldState(SAWorldStateLeftGYAlliance, boolToUint32(sa.Graveyards[SALeftCapturableGY] == SATeamAlliance))
	sess.sendWorldState(SAWorldStateCenterGYAlliance, boolToUint32(sa.Graveyards[SACentralCapturableGY] == SATeamAlliance))

	// Timers
	sess.sendWorldState(SAWorldStateEnableTimer, boolToUint32(sa.TimerEnabled))
	sess.sendWorldState(SAWorldStateBonusTimer, 0)
}

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
