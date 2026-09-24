package world

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Arena Constants mirroring TrinityCore BattlegroundNA/BE/RL/DS/RV and Arena.cpp / Arena.h.
const (
	// Arena Maps
	ArenaMapNagrand          uint32 = 559
	ArenaMapBladesEdge       uint32 = 562
	ArenaMapRuinsOfLordaeron uint32 = 572
	ArenaMapDalaranSewers    uint32 = 617
	ArenaMapRingOfValor      uint32 = 618

	// Battleground Template Type IDs
	BattlegroundTypeIdNA uint32 = 4  // Nagrand Arena
	BattlegroundTypeIdBE uint32 = 5  // Blade's Edge Arena
	BattlegroundTypeIdAA uint32 = 6  // All Arenas queue
	BattlegroundTypeIdRL uint32 = 8  // Ruins of Lordaeron
	BattlegroundTypeIdDS uint32 = 10 // Dalaran Sewers
	BattlegroundTypeIdRV uint32 = 11 // The Ring of Valor

	// Arena Types
	ArenaType2v2 uint8 = 2
	ArenaType3v3 uint8 = 3
	ArenaType5v5 uint8 = 5

	// Teams (Gold / Green)
	ArenaTeamGreen uint8 = 0 // Horde / PVP_TEAM_HORDE
	ArenaTeamGold  uint8 = 1 // Alliance / PVP_TEAM_ALLIANCE
	ArenaTeamNone  int8  = -1
	ArenaTeamDraw  int8  = 2

	// Statuses
	ArenaStatusNone       uint32 = 0
	ArenaStatusWaitJoin   uint32 = 1 // Warmup / preparation
	ArenaStatusInProgress uint32 = 2 // Battle active
	ArenaStatusWaitLeave  uint32 = 3 // Finished, waiting for auto-leave

	// Spells
	SpellArenaPreparation  uint32 = 32727
	SpellAllianceGoldFlag  uint32 = 32724
	SpellAllianceGreenFlag uint32 = 32725
	SpellHordeGoldFlag     uint32 = 35774
	SpellHordeGreenFlag    uint32 = 35775
	SpellLastManStanding   uint32 = 26549
	SpellShadowSight       uint32 = 34709
	SpellWarlDemonicCircle uint32 = 48018
	SpellDSWaterSpout      uint32 = 57090

	// Broadcast Texts
	ArenaTextStartOneMinute      uint32 = 15740
	ArenaTextStartThirtySeconds  uint32 = 15741
	ArenaTextStartFifteenSeconds uint32 = 15739
	ArenaTextStartBattleHasBegun uint32 = 15742

	// WorldStates
	ArenaWorldStateAlivePlayersGreen uint32 = 3600
	ArenaWorldStateAlivePlayersGold  uint32 = 3601
	ArenaWorldStateShowNagrand       uint32 = 2577
	ArenaWorldStateShowBladesEdge    uint32 = 2547
	ArenaWorldStateShowLordaeron     uint32 = 3002
	ArenaWorldStateShowDalaran       uint32 = 3610
	ArenaWorldStateShowRingOfValor   uint32 = 3610

	// Timers & Limits
	ArenaWarmupLength        = 60 * time.Second
	ArenaMatchTimeLimit      = 45 * time.Minute
	ArenaDoorRemovalDelay    = 5 * time.Second
	ArenaShadowSightDelay    = 60 * time.Second
	ArenaAutoLeaveDelay      = 120 * time.Second
	ArenaTimeLimitPointsLoss = -16

	// Buff GameObjects (Shadow Sight)
	ArenaGOShadowSight1 uint32 = 184663
	ArenaGOShadowSight2 uint32 = 184664

	// Nagrand Arena (559) GameObjects
	ArenaGONA_Door1 uint32 = 183978
	ArenaGONA_Door2 uint32 = 183980
	ArenaGONA_Door3 uint32 = 183977
	ArenaGONA_Door4 uint32 = 183979

	// Blade's Edge Arena (562) GameObjects
	ArenaGOBE_Door1 uint32 = 183971
	ArenaGOBE_Door2 uint32 = 183973
	ArenaGOBE_Door3 uint32 = 183970
	ArenaGOBE_Door4 uint32 = 183972

	// Ruins of Lordaeron (572) GameObjects
	ArenaGORL_Door1 uint32 = 185918
	ArenaGORL_Door2 uint32 = 185917

	// Dalaran Sewers (617) GameObjects
	ArenaGODS_Door1  uint32 = 192642
	ArenaGODS_Door2  uint32 = 192643
	ArenaGODS_Water1 uint32 = 194395
	ArenaGODS_Water2 uint32 = 191877

	// Ring of Valor (618) GameObjects
	ArenaGORV_Elevator1  uint32 = 194582
	ArenaGORV_Elevator2  uint32 = 194586
	ArenaGORV_Fire1      uint32 = 192704
	ArenaGORV_Fire2      uint32 = 192705
	ArenaGORV_Firedoor1  uint32 = 192388
	ArenaGORV_Firedoor2  uint32 = 192387
	ArenaGORV_Pillar1    uint32 = 194583
	ArenaGORV_Pillar2    uint32 = 194584
	ArenaGORV_Pillar3    uint32 = 194585
	ArenaGORV_Pillar4    uint32 = 194587
	ArenaGORV_PillarCol1 uint32 = 194580
	ArenaGORV_PillarCol2 uint32 = 194579
	ArenaGORV_PillarCol3 uint32 = 194581
	ArenaGORV_PillarCol4 uint32 = 194578
	ArenaGORV_Gear1      uint32 = 192393
	ArenaGORV_Gear2      uint32 = 192394
	ArenaGORV_Pulley1    uint32 = 192389
	ArenaGORV_Pulley2    uint32 = 192390
)

// IsArenaMap returns true if the specified map ID is an arena map.
func IsArenaMap(mapID uint32) bool {
	switch mapID {
	case ArenaMapNagrand, ArenaMapBladesEdge, ArenaMapRuinsOfLordaeron, ArenaMapDalaranSewers, ArenaMapRingOfValor:
		return true
	default:
		return false
	}
}

// GetArenaStartLocation returns the official WorldSafeLocs starting coordinates for an arena map and team.
func GetArenaStartLocation(mapID uint32, team uint8) (x, y, z, o float32) {
	switch mapID {
	case ArenaMapNagrand:
		if team == ArenaTeamGold {
			return 4027.60, 2972.78, 12.07, 0.0 // LocID 929
		}
		return 4085.45, 2866.83, 12.40, math.Pi // LocID 936
	case ArenaMapBladesEdge:
		if team == ArenaTeamGold {
			return 6292.66, 288.58, 4.96, 0.0 // LocID 939
		}
		return 6184.98, 236.01, 4.98, math.Pi // LocID 940
	case ArenaMapRuinsOfLordaeron:
		if team == ArenaTeamGold {
			return 1277.87, 1744.90, 32.50, 0.0 // LocID 1258
		}
		return 1295.13, 1586.44, 32.50, math.Pi // LocID 1259
	case ArenaMapDalaranSewers:
		if team == ArenaTeamGold {
			return 1218.01, 764.80, 14.73, 0.0 // LocID 1362
		}
		return 1361.76, 817.34, 14.84, math.Pi // LocID 1363
	case ArenaMapRingOfValor:
		if team == ArenaTeamGold {
			return 763.56, -274.00, 3.55, 0.0 // LocID 1364
		}
		return 763.93, -295.01, 3.56, 0.0 // LocID 1365
	default:
		return 0, 0, 0, 0
	}
}

// isArenaGameObject checks if an entry corresponds to any arena gameobject.
func isArenaGameObject(entry uint32) bool {
	switch entry {
	case ArenaGOShadowSight1, ArenaGOShadowSight2,
		ArenaGONA_Door1, ArenaGONA_Door2, ArenaGONA_Door3, ArenaGONA_Door4,
		ArenaGOBE_Door1, ArenaGOBE_Door2, ArenaGOBE_Door3, ArenaGOBE_Door4,
		ArenaGORL_Door1, ArenaGORL_Door2,
		ArenaGODS_Door1, ArenaGODS_Door2, ArenaGODS_Water1, ArenaGODS_Water2,
		ArenaGORV_Elevator1, ArenaGORV_Elevator2, ArenaGORV_Fire1, ArenaGORV_Fire2,
		ArenaGORV_Firedoor1, ArenaGORV_Firedoor2, ArenaGORV_Pillar1, ArenaGORV_Pillar2,
		ArenaGORV_Pillar3, ArenaGORV_Pillar4, ArenaGORV_PillarCol1, ArenaGORV_PillarCol2,
		ArenaGORV_PillarCol3, ArenaGORV_PillarCol4, ArenaGORV_Gear1, ArenaGORV_Gear2,
		ArenaGORV_Pulley1, ArenaGORV_Pulley2:
		return true
	default:
		return false
	}
}

// ArenaPlayerScore tracks individual statistics for an arena participant.
type ArenaPlayerScore struct {
	GUID         uint64
	TeamID       uint8 // 0: Green, 1: Gold
	KillingBlows uint32
	DamageDone   uint32
	HealingDone  uint32
}

// ArenaTeamMatchScore tracks team ratings and delta for an arena match.
type ArenaTeamMatchScore struct {
	RatingChange     int32
	NewRating        uint32
	MatchmakerRating uint32
	TeamName         string
}

// arenaBattlegroundState encapsulates all state and mechanics for an active arena instance.
type arenaBattlegroundState struct {
	mu sync.RWMutex

	InstanceID uint32
	MapID      uint32
	TypeID     uint32
	ArenaType  uint8 // 2, 3, 5
	IsRated    bool

	GoldTeamID    uint32
	GreenTeamID   uint32
	GoldTeamName  string
	GreenTeamName string

	Status         uint32
	StartTime      time.Time
	MatchStartTime time.Time
	EndTime        time.Time
	Winner         int8 // -1: ongoing, 0: Green, 1: Gold, 2: Draw

	GreenAlive uint32
	GoldAlive  uint32
	GreenTotal uint32
	GoldTotal  uint32

	DoorsRemoved        bool
	ShadowSightSpawned  bool
	RVPillarCollision   bool
	RVPillarSwitchTimer time.Time

	AnnouncedOneMinute      bool
	AnnouncedThirtySeconds  bool
	AnnouncedFifteenSeconds bool

	Scores         map[uint64]*ArenaPlayerScore
	PlayerTeams    map[uint64]uint8 // GUID -> team (0: Green, 1: Gold)
	GoldTeamScore  ArenaTeamMatchScore
	GreenTeamScore ArenaTeamMatchScore

	StopTicker chan struct{}
}

// getOrCreateArenaState retrieves or instantiates the arena state for a map and instance ID.
func (s *Server) getOrCreateArenaState(mapID, instanceID uint32, arenaType uint8, isRated bool) *arenaBattlegroundState {
	if s == nil {
		return nil
	}
	s.arenaMu.Lock()
	defer s.arenaMu.Unlock()
	if s.arenaState == nil {
		s.arenaState = make(map[uint32]*arenaBattlegroundState)
	}

	key := (mapID << 16) | (instanceID & 0xFFFF)
	if instanceID == 0 {
		key = mapID
	}

	state := s.arenaState[key]
	if state == nil {
		typeID := BattlegroundTypeIdAA
		switch mapID {
		case ArenaMapNagrand:
			typeID = BattlegroundTypeIdNA
		case ArenaMapBladesEdge:
			typeID = BattlegroundTypeIdBE
		case ArenaMapRuinsOfLordaeron:
			typeID = BattlegroundTypeIdRL
		case ArenaMapDalaranSewers:
			typeID = BattlegroundTypeIdDS
		case ArenaMapRingOfValor:
			typeID = BattlegroundTypeIdRV
		}

		if arenaType == 0 {
			arenaType = uint8(ArenaTeamType2v2)
		}

		state = &arenaBattlegroundState{
			InstanceID:    instanceID,
			MapID:         mapID,
			TypeID:        typeID,
			ArenaType:     arenaType,
			IsRated:       isRated,
			GoldTeamName:  "Gold Team",
			GreenTeamName: "Green Team",
			Status:        ArenaStatusWaitJoin,
			StartTime:     time.Now(),
			Winner:        ArenaTeamNone,
			Scores:        make(map[uint64]*ArenaPlayerScore),
			PlayerTeams:   make(map[uint64]uint8),
			StopTicker:    make(chan struct{}),
		}
		s.arenaState[key] = state
	}
	return state
}

// findArenaState retrieves an existing arena state for a map and instance.
func (s *Server) findArenaState(mapID, instanceID uint32) *arenaBattlegroundState {
	if s == nil {
		return nil
	}
	s.arenaMu.RLock()
	defer s.arenaMu.RUnlock()
	if s.arenaState == nil {
		return nil
	}
	if instanceID != 0 {
		key := (mapID << 16) | (instanceID & 0xFFFF)
		if state := s.arenaState[key]; state != nil {
			return state
		}
	}
	if state := s.arenaState[mapID]; state != nil {
		return state
	}
	for _, state := range s.arenaState {
		if state != nil && state.MapID == mapID {
			return state
		}
	}
	return nil
}

// addPlayerToArena admits a player into the arena, sets preparation auras, flags, and sends initial world states.
func (s *Server) addPlayerToArena(sess *session, arenaTeam uint8, arena *arenaBattlegroundState) {
	if s == nil || sess == nil || sess.player == nil || arena == nil {
		return
	}

	arena.mu.Lock()

	guid := sess.playerGUID
	arena.PlayerTeams[guid] = arenaTeam

	if _, exists := arena.Scores[guid]; !exists {
		arena.Scores[guid] = &ArenaPlayerScore{
			GUID:   guid,
			TeamID: arenaTeam,
		}
		if arenaTeam == ArenaTeamGold {
			arena.GoldAlive++
			arena.GoldTotal++
		} else {
			arena.GreenAlive++
			arena.GreenTotal++
		}
	}

	// Remove mounts
	if sess.player.MountDisplayID != 0 {
		sess.player.MountDisplayID = 0
		sess.sendPlayerMountUpdate()
	}

	// Assign team flag aura
	// Alliance player on Gold gets 32724, Horde on Gold gets 35774
	// Alliance on Green gets 32725, Horde on Green gets 35775
	isHorde := sess.player.Race == 2 || sess.player.Race == 5 || sess.player.Race == 6 || sess.player.Race == 8 || sess.player.Race == 10
	if arenaTeam == ArenaTeamGold {
		if isHorde {
			sess.applyAura(SpellHordeGoldFlag)
		} else {
			sess.applyAura(SpellAllianceGoldFlag)
		}
	} else {
		if isHorde {
			sess.applyAura(SpellHordeGreenFlag)
		} else {
			sess.applyAura(SpellAllianceGreenFlag)
		}
	}

	// In preparation phase, apply Arena Preparation aura and reset powers
	if arena.Status == ArenaStatusWaitJoin {
		sess.applyAura(SpellArenaPreparation)
		s.resetPlayerPowers(sess)
	}

	// Set queue entry state
	for i := 0; i < len(sess.bgQueues); i++ {
		if sess.bgQueues[i].Active && sess.bgQueues[i].IsArena {
			sess.bgQueues[i].Status = arena.Status
			sess.bgQueues[i].MapID = arena.MapID
			sess.bgQueues[i].InstanceID = arena.InstanceID
			sess.bgQueues[i].ArenaType = arena.ArenaType
			sess.bgQueues[i].ArenaFaction = arenaTeam
			sess.bgQueues[i].StartTime = arena.StartTime
			sess.sendBattlefieldStatus(uint8(i))
			break
		}
	}
	arena.mu.Unlock()

	s.sendArenaInitialWorldStates(sess)
}

// resetPlayerPowers restores max health/mana and clears rage/runic power.
func (s *Server) resetPlayerPowers(sess *session) {
	if sess == nil || sess.player == nil {
		return
	}
	sess.player.Health = sess.player.MaxHealth
	sess.player.Powers[0] = sess.player.MaxPowers[0] // Mana
	sess.player.Powers[1] = 0                        // Rage
	sess.player.Powers[3] = sess.player.MaxPowers[3] // Energy
	sess.player.Powers[6] = 0                        // Runic Power
	sess.sendPlayerUpdate()
}

// sendArenaInitialWorldStates broadcasts initial alive counts and map display world states.
func (s *Server) sendArenaInitialWorldStates(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	arena := s.findArenaState(sess.player.Map, 0)
	if arena == nil {
		return
	}

	arena.mu.RLock()
	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	mapID := arena.MapID
	arena.mu.RUnlock()

	// Alive player counts
	sess.sendWorldState(ArenaWorldStateAlivePlayersGreen, greenAlive)
	sess.sendWorldState(ArenaWorldStateAlivePlayersGold, goldAlive)

	// Map-specific display world states
	switch mapID {
	case ArenaMapNagrand:
		sess.sendWorldState(ArenaWorldStateShowNagrand, 1)
	case ArenaMapBladesEdge:
		sess.sendWorldState(ArenaWorldStateShowBladesEdge, 1)
	case ArenaMapRuinsOfLordaeron:
		sess.sendWorldState(ArenaWorldStateShowLordaeron, 1)
	case ArenaMapDalaranSewers:
		sess.sendWorldState(ArenaWorldStateShowDalaran, 1)
	case ArenaMapRingOfValor:
		sess.sendWorldState(ArenaWorldStateShowRingOfValor, 1)
	}
}

// updateArenaWorldState broadcasts a world state update to all players on the arena map.
func (s *Server) updateArenaWorldState(arena *arenaBattlegroundState, variableID, value uint32) {
	if s == nil || arena == nil {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == arena.MapID {
			sess.sendWorldState(variableID, value)
		}
	}
}

// handleArenaPlayerDeath processes player deaths inside an arena match.
func (s *Server) handleArenaPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil || !IsArenaMap(sess.player.Map) {
		return
	}

	arena := s.findArenaState(sess.player.Map, 0)
	if arena == nil {
		return
	}

	arena.mu.Lock()
	if arena.Status != ArenaStatusInProgress {
		arena.mu.Unlock()
		return
	}

	team, exists := arena.PlayerTeams[sess.playerGUID]
	if !exists {
		arena.mu.Unlock()
		return
	}

	if team == ArenaTeamGold {
		if arena.GoldAlive > 0 {
			arena.GoldAlive--
		}
	} else {
		if arena.GreenAlive > 0 {
			arena.GreenAlive--
		}
	}
	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	arena.mu.Unlock()

	// Update alive counts
	s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGreen, greenAlive)
	s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGold, goldAlive)

	s.checkArenaWinConditions(arena)
}

// handleArenaPlayerLeave processes a player leaving an arena map or disconnecting.
func (s *Server) handleArenaPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil || !IsArenaMap(sess.player.Map) {
		return
	}

	arena := s.findArenaState(sess.player.Map, 0)
	if arena == nil {
		return
	}

	arena.mu.Lock()
	team, exists := arena.PlayerTeams[sess.playerGUID]
	if exists && arena.Status == ArenaStatusInProgress {
		if team == ArenaTeamGold {
			if arena.GoldAlive > 0 {
				arena.GoldAlive--
			}
		} else {
			if arena.GreenAlive > 0 {
				arena.GreenAlive--
			}
		}
	}
	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	arena.mu.Unlock()

	// Remove flag and prep auras
	sess.removeAura(SpellAllianceGoldFlag)
	sess.removeAura(SpellAllianceGreenFlag)
	sess.removeAura(SpellHordeGoldFlag)
	sess.removeAura(SpellHordeGreenFlag)
	sess.removeAura(SpellArenaPreparation)

	s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGreen, greenAlive)
	s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGold, goldAlive)

	s.checkArenaWinConditions(arena)
}

// checkArenaWinConditions evaluates whether an arena team has won.
func (s *Server) checkArenaWinConditions(arena *arenaBattlegroundState) {
	if s == nil || arena == nil {
		return
	}

	arena.mu.Lock()
	if arena.Status != ArenaStatusInProgress {
		arena.mu.Unlock()
		return
	}

	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	greenTotal := arena.GreenTotal
	goldTotal := arena.GoldTotal

	if goldAlive == 0 && greenTotal > 0 {
		arena.mu.Unlock()
		s.endArena(arena, int8(ArenaTeamGreen))
		return
	} else if greenAlive == 0 && goldTotal > 0 {
		arena.mu.Unlock()
		s.endArena(arena, int8(ArenaTeamGold))
		return
	} else if greenAlive == 0 && goldAlive == 0 {
		arena.mu.Unlock()
		s.endArena(arena, ArenaTeamDraw)
		return
	}
	arena.mu.Unlock()
}

// startArenaMatch transitions an arena from warmup to in-progress.
func (s *Server) startArenaMatch(arena *arenaBattlegroundState) {
	if s == nil || arena == nil {
		return
	}

	arena.mu.Lock()
	if arena.Status != ArenaStatusWaitJoin {
		arena.mu.Unlock()
		return
	}

	arena.Status = ArenaStatusInProgress
	arena.MatchStartTime = time.Now()
	arena.RVPillarSwitchTimer = time.Now().Add(20 * time.Second)
	arena.mu.Unlock()

	s.broadcastArenaMessage(arena.MapID, "The arena battle has begun!")

	// Process all players in the arena
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == arena.MapID {
			sess.removeAura(SpellArenaPreparation)
			s.resetPlayerPowers(sess)

			// Remove short-duration positive buffs (<= 30s)
			// Remove Warlock Demonic Circle if on Dalaran Sewers
			if arena.MapID == ArenaMapDalaranSewers {
				sess.removeAura(SpellWarlDemonicCircle)
			}

			// Update queue status
			for i := 0; i < len(sess.bgQueues); i++ {
				if sess.bgQueues[i].Active && sess.bgQueues[i].IsArena {
					sess.bgQueues[i].Status = ArenaStatusInProgress
					sess.bgQueues[i].StartTime = arena.MatchStartTime
					sess.sendBattlefieldStatus(uint8(i))
					break
				}
			}
		}
	}
	s.sessionsMu.RUnlock()
}

// endArena handles match conclusion, rating settlement, achievements, and PVP log data dispatch.
func (s *Server) endArena(arena *arenaBattlegroundState, winner int8) {
	if s == nil || arena == nil {
		return
	}

	arena.mu.Lock()
	if arena.Winner != ArenaTeamNone {
		arena.mu.Unlock()
		return
	}

	arena.Status = ArenaStatusWaitLeave
	arena.Winner = winner
	arena.EndTime = time.Now()

	goldTeamID := arena.GoldTeamID
	greenTeamID := arena.GreenTeamID
	isRated := arena.IsRated
	arenaType := uint32(arena.ArenaType)
	goldAlive := arena.GoldAlive
	greenAlive := arena.GreenAlive
	arena.mu.Unlock()

	// Reference arena end criteria: PLAY_ARENA, WIN_ARENA, GET_KILLING_BLOWS.
	arena.mu.RLock()
	scoreBlows := make(map[uint64]uint32, len(arena.Scores))
	for guid, score := range arena.Scores {
		scoreBlows[guid] = score.KillingBlows
	}
	winningMembers := make(map[uint64]struct{})
	if winner == int8(ArenaTeamGold) || winner == int8(ArenaTeamGreen) {
		for guid, team := range arena.PlayerTeams {
			if int8(team) == winner {
				winningMembers[guid] = struct{}{}
			}
		}
	}
	mapID := arena.MapID
	arena.mu.RUnlock()
	s.creditArenaParticipants(mapID, scoreBlows, winningMembers)

	// Victory announcement
	if winner == int8(ArenaTeamGold) {
		s.broadcastArenaMessage(arena.MapID, fmt.Sprintf("%s is victorious!", arena.GoldTeamName))
	} else if winner == int8(ArenaTeamGreen) {
		s.broadcastArenaMessage(arena.MapID, fmt.Sprintf("%s is victorious!", arena.GreenTeamName))
	} else {
		s.broadcastArenaMessage(arena.MapID, "The arena match ended in a draw!")
	}

	// Rated rating calculation and DB settlement
	if isRated && s.CharactersStore != nil && s.CharactersStore.DB != nil && goldTeamID > 0 && greenTeamID > 0 {
		ctx := context.Background()
		var goldMembers, greenMembers []uint64
		arena.mu.RLock()
		for guid, team := range arena.PlayerTeams {
			if team == ArenaTeamGold {
				goldMembers = append(goldMembers, guid)
			} else {
				greenMembers = append(greenMembers, guid)
			}
		}
		arena.mu.RUnlock()

		if winner == int8(ArenaTeamGold) {
			gain, loss, _ := s.RecordArenaMatchResult(ctx, goldTeamID, greenTeamID, goldMembers, greenMembers)
			arena.mu.Lock()
			arena.GoldTeamScore.RatingChange = gain
			arena.GreenTeamScore.RatingChange = -loss
			arena.mu.Unlock()
		} else if winner == int8(ArenaTeamGreen) {
			gain, loss, _ := s.RecordArenaMatchResult(ctx, greenTeamID, goldTeamID, greenMembers, goldMembers)
			arena.mu.Lock()
			arena.GreenTeamScore.RatingChange = gain
			arena.GoldTeamScore.RatingChange = -loss
			arena.mu.Unlock()
		} else {
			// Draw: deduct 16 points from both teams
			cdb := s.CharactersStore.DB
			_, _ = cdb.ExecContext(ctx, "UPDATE arena_team SET rating = CASE WHEN rating >= 16 THEN rating - 16 ELSE 0 END, weekGames = weekGames + 1, seasonGames = seasonGames + 1 WHERE arenaTeamId IN (?, ?)", goldTeamID, greenTeamID)
			arena.mu.Lock()
			arena.GoldTeamScore.RatingChange = ArenaTimeLimitPointsLoss
			arena.GreenTeamScore.RatingChange = ArenaTimeLimitPointsLoss
			arena.mu.Unlock()
		}
	}

	// Achievement check: Last Man Standing (Rated 5v5 and solely alive winner)
	if isRated && arenaType == ArenaTeamType5v5 {
		if winner == int8(ArenaTeamGold) && goldAlive == 1 {
			s.awardLastManStanding(arena, ArenaTeamGold)
		} else if winner == int8(ArenaTeamGreen) && greenAlive == 1 {
			s.awardLastManStanding(arena, ArenaTeamGreen)
		}
	}

	// Build and dispatch MSG_PVP_LOG_DATA to all players in the match
	logData := s.buildArenaPvPLogDataPacket(arena)
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == arena.MapID {
			_ = sess.write(uint16(protocol.OpcodeMSG_PVP_LOG_DATA), logData, true)

			// Update queue entry
			for i := 0; i < len(sess.bgQueues); i++ {
				if sess.bgQueues[i].Active && sess.bgQueues[i].IsArena {
					sess.bgQueues[i].Status = ArenaStatusWaitLeave
					sess.sendBattlefieldStatus(uint8(i))
					break
				}
			}
		}
	}
	s.sessionsMu.RUnlock()
}

// awardLastManStanding grants SpellLastManStanding (26549) to the solely alive participant.
func (s *Server) awardLastManStanding(arena *arenaBattlegroundState, team uint8) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == arena.MapID && sess.player.Health > 0 {
			if arenaTeam, ok := arena.PlayerTeams[sess.playerGUID]; ok && arenaTeam == team {
				sess.applyAura(SpellLastManStanding)
				break
			}
		}
	}
}

// buildArenaPvPLogDataPacket constructs MSG_PVP_LOG_DATA matching TrinityCore ArenaScore format.
func (s *Server) buildArenaPvPLogDataPacket(arena *arenaBattlegroundState) []byte {
	if arena == nil {
		return nil
	}
	arena.mu.RLock()
	defer arena.mu.RUnlock()

	numScores := len(arena.Scores)
	buf := protocol.NewBuffer(128 + numScores*40)

	// type: 1 for Arena
	buf.WriteU8(1)

	// Rating info blocks: Gold (Alliance = 1), Green (Horde = 0)
	// TrinityCore PVP_TEAMS_COUNT = 2 (0: Horde, 1: Alliance)
	// Horde (Green) Rating Info
	greenLoss := uint32(0)
	greenGain := uint32(0)
	if arena.GreenTeamScore.RatingChange < 0 {
		greenLoss = uint32(-arena.GreenTeamScore.RatingChange)
	} else {
		greenGain = uint32(arena.GreenTeamScore.RatingChange)
	}
	buf.WriteU32(greenLoss)
	buf.WriteU32(greenGain)
	buf.WriteU32(arena.GreenTeamScore.MatchmakerRating)

	// Alliance (Gold) Rating Info
	goldLoss := uint32(0)
	goldGain := uint32(0)
	if arena.GoldTeamScore.RatingChange < 0 {
		goldLoss = uint32(-arena.GoldTeamScore.RatingChange)
	} else {
		goldGain = uint32(arena.GoldTeamScore.RatingChange)
	}
	buf.WriteU32(goldLoss)
	buf.WriteU32(goldGain)
	buf.WriteU32(arena.GoldTeamScore.MatchmakerRating)

	// Team Info blocks (CString names)
	buf.WriteCString(arena.GreenTeamName)
	buf.WriteCString(arena.GoldTeamName)

	// Match ended flag & winner
	if arena.Status == ArenaStatusWaitLeave {
		buf.WriteU8(1) // Ended
		if arena.Winner == int8(ArenaTeamGreen) {
			buf.WriteU8(0) // Horde win
		} else if arena.Winner == int8(ArenaTeamGold) {
			buf.WriteU8(1) // Alliance win
		} else {
			buf.WriteU8(2) // Draw
		}
	} else {
		buf.WriteU8(0) // Not ended
	}

	// Player Scores
	buf.WriteU32(uint32(numScores))
	for _, sc := range arena.Scores {
		buf.WriteU64(sc.GUID)
		buf.WriteU32(sc.KillingBlows)
		buf.WriteU8(sc.TeamID)
		buf.WriteU32(sc.DamageDone)
		buf.WriteU32(sc.HealingDone)
		buf.WriteU32(0) // Objectives count (always 0 in arena)
	}

	return buf.Bytes()
}

// handleArenaGameObjectUse processes clicks on arena gameobjects (such as Shadow Sight).
func (s *Server) handleArenaGameObjectUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil || !IsArenaMap(sess.player.Map) {
		return false
	}

	switch entry {
	case ArenaGOShadowSight1, ArenaGOShadowSight2:
		sess.applyAura(SpellShadowSight)
		s.setGameObjectHidden(guid, true)
		s.broadcastGameObjectDespawn(sess.player.Map, guid)
		s.broadcastArenaMessage(sess.player.Map, fmt.Sprintf("%s has picked up Shadow Sight!", sess.player.Name))
		return true
	default:
		return false
	}
}

// updateArenaDamageScore updates damage done for a player in the arena score table.
func (s *Server) updateArenaDamageScore(attacker *session, damage uint32) {
	if s == nil || attacker == nil || attacker.player == nil || !IsArenaMap(attacker.player.Map) {
		return
	}
	arena := s.findArenaState(attacker.player.Map, 0)
	if arena == nil {
		return
	}
	arena.mu.Lock()
	defer arena.mu.Unlock()
	if sc, ok := arena.Scores[attacker.playerGUID]; ok {
		sc.DamageDone += damage
	}
}

// updateArenaHealingScore updates healing done for a player in the arena score table.
func (s *Server) updateArenaHealingScore(healer *session, healing uint32) {
	if s == nil || healer == nil || healer.player == nil || !IsArenaMap(healer.player.Map) {
		return
	}
	arena := s.findArenaState(healer.player.Map, 0)
	if arena == nil {
		return
	}
	arena.mu.Lock()
	defer arena.mu.Unlock()
	if sc, ok := arena.Scores[healer.playerGUID]; ok {
		sc.HealingDone += healing
	}
}

// broadcastArenaMessage sends a system chat message to all players on the arena map.
func (s *Server) broadcastArenaMessage(mapID uint32, msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID {
			sess.sendSystemMessage(msg)
		}
	}
}

// updateArenaTick updates countdowns, hazards, and timeout checks.
func (s *Server) updateArenaTick(arena *arenaBattlegroundState, now time.Time) {
	if s == nil || arena == nil {
		return
	}

	arena.mu.Lock()
	status := arena.Status
	startTime := arena.StartTime
	matchStartTime := arena.MatchStartTime
	arena.mu.Unlock()

	if status == ArenaStatusWaitJoin {
		elapsed := now.Sub(startTime)
		remaining := ArenaWarmupLength - elapsed

		if remaining <= 60*time.Second && remaining > 30*time.Second && !arena.AnnouncedOneMinute {
			arena.AnnouncedOneMinute = true
			s.broadcastArenaMessage(arena.MapID, "One minute until the arena battle begins!")
		} else if remaining <= 30*time.Second && remaining > 15*time.Second && !arena.AnnouncedThirtySeconds {
			arena.AnnouncedThirtySeconds = true
			s.broadcastArenaMessage(arena.MapID, "Thirty seconds until the arena battle begins!")
		} else if remaining <= 15*time.Second && remaining > 0 && !arena.AnnouncedFifteenSeconds {
			arena.AnnouncedFifteenSeconds = true
			s.broadcastArenaMessage(arena.MapID, "Fifteen seconds until the arena battle begins!")
		} else if remaining <= 0 {
			s.startArenaMatch(arena)
		}
	} else if status == ArenaStatusInProgress {
		// Check match duration timeout (45 minutes)
		if !matchStartTime.IsZero() && now.Sub(matchStartTime) >= ArenaMatchTimeLimit {
			s.endArena(arena, ArenaTeamDraw)
			return
		}

		// Ring of Valor dynamic pillar collision toggling
		if arena.MapID == ArenaMapRingOfValor {
			arena.mu.Lock()
			if now.After(arena.RVPillarSwitchTimer) {
				arena.RVPillarCollision = !arena.RVPillarCollision
				arena.RVPillarSwitchTimer = now.Add(25 * time.Second)
			}
			arena.mu.Unlock()
		}
	}
}
