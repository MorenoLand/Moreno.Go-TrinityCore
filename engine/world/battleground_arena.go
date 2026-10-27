package world

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
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

	// BattlegroundStatus wire values (Battleground.h:176); written into
	// bgQueueEntry.Status which sendBattlefieldStatus puts on the wire verbatim
	BGStatusNone       uint32 = 0
	BGStatusWaitQueue  uint32 = 1
	BGStatusWaitJoin   uint32 = 2
	BGStatusInProgress uint32 = 3
	BGStatusWaitLeave  uint32 = 4

	// ArenaTimeToAutoRemove mirrors TIME_TO_AUTOREMOVE (Battleground.h:151):
	// Battleground::_ProcessLeave removes every remaining player this long after
	// the match ends (Battleground.cpp:506-528).
	ArenaTimeToAutoRemove = 120 * time.Second

	// Spells
	SpellArenaPreparation  uint32 = 32727
	SpellAllianceGoldFlag  uint32 = 32724
	SpellAllianceGreenFlag uint32 = 32725
	SpellHordeGoldFlag     uint32 = 35774
	SpellHordeGreenFlag    uint32 = 35775
	SpellLastManStanding   uint32 = 26549
	SpellShadowSight       uint32 = 34709
	SpellWarlDemonicCircle uint32 = 48018
	SpellDSWaterSpout      uint32 = 58873
	SpellDSFlush           uint32 = 57405
	SpellDSFlushKnockback  uint32 = 61698

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
	ArenaWarmupLength              = 60 * time.Second
	ArenaMatchTimeLimit            = 47 * time.Minute
	ArenaDoorRemovalDelay          = 5 * time.Second
	ArenaShadowSightRespawnDelay   = 120 * time.Second // buff respawn NA/RL/BE/DS: AddObject respawn param 120 (replaces wrong unused 60s const)
	ArenaShadowSightRespawnDelayRV = 90 * time.Second  // buff respawn RV: BattlegroundRV::StartingEventOpenDoors (BattlegroundRV.cpp:71-72)
	ArenaAutoLeaveDelay            = 120 * time.Second
	ArenaTimeLimitPointsLoss       = -16

	// Ring of Valor fire/pillar preamble (BattlegroundRV.h:84-87):	// StartingEventOpenDoors sets OPEN_FENCES + FIRST_TIMER 20133ms, the fire
	// opens, CLOSE_FIRE_TIMER 5000ms closes it, FIRE_TO_PILLAR_TIMER 20000ms
	// later the first TogglePillarCollision fires, then PILLAR_SWITCH_TIMER
	// 25000ms repeats it.
	RVPillarFirstSwitchDelay = (20133 + 5000 + 20000) * time.Millisecond
	RVPillarSwitchInterval   = 25 * time.Second

	// Dalaran Sewers waterfall / pipe-knockback timing (BattlegroundDS.h:63-75).
	// The header notes these values are not blizzlike; they are kept verbatim.
	DSWaterfallTimerMin       = 30 * time.Second
	DSWaterfallTimerMax       = 60 * time.Second
	DSWaterfallWarningTime    = 5 * time.Second
	DSWaterfallDuration       = 30 * time.Second
	DSWaterfallKnockbackEvery = 1500 * time.Millisecond
	DSPipeKnockbackFirstDelay = 5 * time.Second
	DSPipeKnockbackDelay      = 3 * time.Second
	DSPipeKnockbackTotalCount = 2

	// Dalaran Sewers event ids (BattlegroundDS.h:77-85). The knockback repeat
	// is tracked with DSKnockbackAt and the pipe knockback with DSPipeAt/
	// DSPipeCount; BG_DS_EVENT_PIPE_KNOCKBACK is never scheduled in C++
	// (commented out in StartingEventOpenDoors), so it has no Go arm either.
	DSEventWaterfallWarning   uint8 = 1
	DSEventWaterfallOn        uint8 = 2
	DSEventWaterfallOff       uint8 = 3
	DSEventWaterfallKnockback uint8 = 4
	DSEventPipeKnockback      uint8 = 5

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

	// Dalaran Sewers (617) knockback creature (BattlegroundDS.h:48-52)
	ArenaGODS_NPCWaterSpout uint32 = 28567

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

// bgStatusForArena maps the Go arena lifecycle status to the client-visible
// BattlegroundStatus wire value: a player inside an arena (warmup or battle)
// is STATUS_IN_PROGRESS in C++ terms, finished is STATUS_WAIT_LEAVE.
func bgStatusForArena(status uint32) uint32 {
	switch status {
	case ArenaStatusWaitJoin, ArenaStatusInProgress:
		return BGStatusInProgress
	case ArenaStatusWaitLeave:
		return BGStatusWaitLeave
	default:
		return BGStatusNone
	}
}

// arenaMatchQueueSlot returns the index of the session's queue slot for this
// arena instance. The slot is the one whose instance id matches the state's
// (the pop stamps the slot and the state with the same id, and the accept
// re-finds the state by it); when the state carries no instance id, the
// arena-type slot is used instead — at most one active slot exists per
// arena type (the join dup-block). C++ keys the packet slot by queue type
// (Battleground::RemovePlayerAtLeave, Battleground.cpp:887, via
// GetBattlegroundQueueIndex), never by first-active-slot: a player can
// hold several arena slots at once (one per arena type), so first-active
// flips or clears a different match's slot — a 3v3 WaitJoin invite
// overwritten when its owner's 2v2 match starts, ends, or auto-removes.
func arenaMatchQueueSlot(sess *session, arena *arenaBattlegroundState) int {
	if sess == nil || arena == nil {
		return -1
	}
	for i := range sess.bgQueues {
		e := &sess.bgQueues[i]
		if !e.Active || !e.IsArena {
			continue
		}
		if arena.InstanceID != 0 {
			if e.InstanceID == arena.InstanceID {
				return i
			}
		} else if e.ArenaType == arena.ArenaType {
			return i
		}
	}
	return -1
}

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
	CountedAlive bool
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
	BEDoorsRemoveAt     time.Time
	ShadowSightSpawned  bool
	RVPillarCollision   bool
	RVPillarSwitchTimer time.Time

	// Dalaran Sewers waterfall / pipe-knockback machine
	// (BattlegroundDS::PostUpdateImpl, BattlegroundDS.cpp:37-87).
	DSWaterfallEvent  uint8
	DSWaterfallAt     time.Time
	DSWaterfallActive bool
	DSKnockbackAt     time.Time
	DSPipeCount       uint8
	DSPipeAt          time.Time

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

// findArenaStateForPlayer resolves the arena a player belongs to by roster
// membership (PlayerTeams), not by standing map. C++ UpdatePlayerScore
// (Battleground.cpp:1225-1228) keys on the player's Battleground pointer
// and its PlayerScores table — the Unit.cpp:864 (damage) and :6320
// (healing) callers reach it through killer->GetBattleground() and
// player->GetBattleground(), which never consult the player's map — so a
// roster member dealing damage or healing while standing off the arena
// map still scores in their arena, and a map bystander with no roster
// slot scores nowhere. Scores rows are created alongside PlayerTeams in
// addPlayerToArena and deleted with them on leave, so the roster gate
// never blocks a legitimate row. Matches sort by (MapID, InstanceID) so
// a stale dual roster resolves deterministically, not by map order.
func (s *Server) findArenaStateForPlayer(guid uint64) *arenaBattlegroundState {
	if s == nil {
		return nil
	}
	s.arenaMu.RLock()
	states := make([]*arenaBattlegroundState, 0, len(s.arenaState))
	for _, state := range s.arenaState {
		states = append(states, state)
	}
	s.arenaMu.RUnlock()
	var matches []*arenaBattlegroundState
	for _, state := range states {
		if state == nil {
			continue
		}
		state.mu.RLock()
		_, inRoster := state.PlayerTeams[guid]
		state.mu.RUnlock()
		if inRoster {
			matches = append(matches, state)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].MapID != matches[j].MapID {
			return matches[i].MapID < matches[j].MapID
		}
		return matches[i].InstanceID < matches[j].InstanceID
	})
	return matches[0]
}

// arenaRosterSessions resolves the arena's PlayerTeams roster to online
// sessions, in GUID order (C++ m_Players is an ordered map). C++ _GetPlayer
// skips offline members (Battleground.cpp:724, :465, BroadcastWorker), so
// members with no worldReady session are omitted, not synthesized.
func (s *Server) arenaRosterSessions(arena *arenaBattlegroundState) []*session {
	if s == nil || arena == nil {
		return nil
	}
	arena.mu.RLock()
	guids := make([]uint64, 0, len(arena.PlayerTeams))
	for guid := range arena.PlayerTeams {
		guids = append(guids, guid)
	}
	arena.mu.RUnlock()
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	inRoster := make(map[uint64]struct{}, len(guids))
	for _, guid := range guids {
		inRoster[guid] = struct{}{}
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	sessions := make([]*session, 0, len(guids))
	for sess := range s.sessions {
		if _, ok := inRoster[sess.playerGUID]; ok && sess.worldReady.Load() && sess.player != nil {
			sessions = append(sessions, sess)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].playerGUID < sessions[j].playerGUID })
	return sessions
}

// addPlayerToArena admits a player into the arena, sets preparation auras, flags, and sends initial world states.
func (s *Server) addPlayerToArena(sess *session, arenaTeam uint8, arena *arenaBattlegroundState, freshPort bool) {
	if s == nil || sess == nil || sess.player == nil || arena == nil {
		return
	}

	arena.mu.Lock()

	guid := sess.playerGUID
	arena.PlayerTeams[guid] = arenaTeam

	if _, exists := arena.Scores[guid]; !exists {
		arena.Scores[guid] = &ArenaPlayerScore{
			GUID:         guid,
			TeamID:       arenaTeam,
			CountedAlive: true,
		}
		if arenaTeam == ArenaTeamGold {
			arena.GoldAlive++
			arena.GoldTotal++
		} else {
			arena.GreenAlive++
			arena.GreenTotal++
		}
	}

	// Battleground::AddPlayer (Battleground.cpp:1003): entering the arena
	// strips SPELL_AURA_MOUNTED auras. The aura remove leg runs the
	// Unit::Dismount mirror — display/flag clear, SMSG_DISMOUNT, collision
	// height, NOT_MOUNTED interrupt strip, pet resummon — instead of just
	// clearing the display field (which sendPlayerMountUpdate would
	// immediately re-flag from the surviving aura).
	sess.removeAurasByType(spellAuraMounted)

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

	// In preparation phase, apply Arena Preparation aura and reset powers.
	// The power reset runs on the fresh transition only: a duplicate
	// accept mid-preparation must not refill spent health and mana.
	if arena.Status == ArenaStatusWaitJoin {
		sess.applyAura(SpellArenaPreparation)
		if freshPort {
			s.resetPlayerPowers(sess)
		}
	}

	// Set queue entry state on this arena's own slot (arenaMatchQueueSlot):
	// the first-active slot may belong to another arena type the player
	// also holds, and overwriting it would destroy that invite.
	if i := arenaMatchQueueSlot(sess, arena); i >= 0 {
		sess.bgQueues[i].Status = bgStatusForArena(arena.Status)
		sess.bgQueues[i].MapID = arena.MapID
		sess.bgQueues[i].InstanceID = arena.InstanceID
		sess.bgQueues[i].ArenaType = arena.ArenaType
		sess.bgQueues[i].ArenaFaction = arenaTeam
		sess.bgQueues[i].StartTime = arena.StartTime
		sess.sendBattlefieldStatus(uint8(i))
	}
	arena.mu.Unlock()

	s.sendArenaInitialWorldStates(sess, arena)
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
// The arena is the one addPlayerToArena just admitted this session to, not a
// standing-map lookup: the sim port is no-bridge, so the admittee stands off
// the arena map and a map lookup returns nil, sending no initial states at
// all. C++ sends these on map entry via Arena::FillInitialWorldStates
// (Arena.cpp:106-111) to the entering player, whatever map they came from.
func (s *Server) sendArenaInitialWorldStates(sess *session, arena *arenaBattlegroundState) {
	if s == nil || sess == nil || sess.player == nil || arena == nil {
		return
	}

	arena.mu.RLock()
	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	mapID := arena.MapID
	arena.mu.RUnlock()

	// C++ order (BattlegroundDS.cpp:138-142, BattlegroundBE.cpp:84-88): the
	// map-specific show state is emplaced first, then Arena::FillInitialWorld-
	// States appends the alive-player counts.
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

	// Alive player counts (Arena::FillInitialWorldStates, Arena.cpp:106-111)
	sess.sendWorldState(ArenaWorldStateAlivePlayersGreen, greenAlive)
	sess.sendWorldState(ArenaWorldStateAlivePlayersGold, goldAlive)
}

// updateArenaWorldState broadcasts a world state update to the arena's
// roster. C++ UpdateWorldState sends via SendPacketToAll over m_Players
// (Battleground.cpp:659-664, :568-571), not over the map's players: a
// sim-accepted roster member holds no arena map yet, while a map
// bystander holds no roster slot.
func (s *Server) updateArenaWorldState(arena *arenaBattlegroundState, variableID, value uint32) {
	if s == nil || arena == nil {
		return
	}
	for _, sess := range s.arenaRosterSessions(arena) {
		sess.sendWorldState(variableID, value)
	}
}

// handleArenaPlayerDeath processes player deaths inside an arena match.
func (s *Server) handleArenaPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}

	// Arena::HandleKillPlayer (Arena.cpp:118-126) gates only on
	// STATUS_IN_PROGRESS, never on the victim's map: the victim is in
	// m_Players by construction. The death caller (death.go) already runs
	// off-map, so the old standing-map gate dropped every sim roster
	// death: no alive decrement, no world-state update, no win check.
	arena := s.findArenaStateForPlayer(sess.playerGUID)
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
	// C++ GetAlivePlayersCountByTeam (Battleground.cpp:1737) counts live
	// players still in m_Players, so a death already removed this player
	// from the alive count; clear the flag so the leave arm below does
	// not decrement a second time.
	if sc, ok := arena.Scores[sess.playerGUID]; ok {
		sc.CountedAlive = false
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
	if s == nil || sess == nil || sess.player == nil {
		return
	}

	// Arena::RemovePlayer (Arena.cpp:97-104) gates only on
	// STATUS_WAIT_LEAVE, never on the leaver's map: the leaver is in
	// m_Players by construction. The leave funnel (handleLeaveBattlefield)
	// runs off-map in the sim, so the old standing-map gate left the
	// leaver in PlayerTeams/Scores with a stale alive count, unstripped
	// flag/preparation auras, and no win check.
	arena := s.findArenaStateForPlayer(sess.playerGUID)
	if arena == nil {
		return
	}

	arena.mu.Lock()
	team, exists := arena.PlayerTeams[sess.playerGUID]
	status := arena.Status
	// C++ GetAlivePlayersCountByTeam (Battleground.cpp:1737) counts live
	// players still in m_Players: a leaver already dead (death arm cleared
	// CountedAlive) changes nothing on erase, and a leaver whose death
	// arm never ran (no killer) is still counted, so the flag, not
	// liveness at leave time, decides the decrement.
	if exists && status == ArenaStatusInProgress {
		if sc, ok := arena.Scores[sess.playerGUID]; ok && sc.CountedAlive {
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
	}
	// Battleground::RemovePlayerAtLeave (Battleground.cpp:818-839): the leaver is
	// erased from the player and score maps.
	delete(arena.PlayerTeams, sess.playerGUID)
	delete(arena.Scores, sess.playerGUID)
	greenAlive := arena.GreenAlive
	goldAlive := arena.GoldAlive
	arena.mu.Unlock()

	// Remove flag and prep auras
	sess.removeAura(SpellAllianceGoldFlag)
	sess.removeAura(SpellAllianceGreenFlag)
	sess.removeAura(SpellHordeGoldFlag)
	sess.removeAura(SpellHordeGreenFlag)
	sess.removeAura(SpellArenaPreparation)

	// Broadcast after the erase, to the remaining roster only: C++
	// RemovePlayerAtLeave erases m_Players before the subclass
	// RemovePlayer arm broadcasts (Battleground.cpp:826-828, :868).
	// Arena::RemovePlayer (Arena.cpp:97-101) early-returns on
	// STATUS_WAIT_LEAVE, so no broadcast fires once the match ended.
	if status != ArenaStatusWaitLeave {
		s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGreen, greenAlive)
		s.updateArenaWorldState(arena, ArenaWorldStateAlivePlayersGold, goldAlive)
	}

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

// removeShortDurationPositiveAuras implements the arena-start strip in
// Battleground.cpp:476-484 (RemoveAppliedAuras predicate): a
// non-permanent positive aura with 30s or less of live remaining
// duration goes, unless its spell is UNAFFECTED_BY_INVULNERABILITY or
// carries a MOD_INVISIBILITY effect. DurationMs == 0 is the Go
// permanent marker (scheduleAuraUpkeepLocked precedent); remaining time
// is the live DurationUpdatedAt-adjusted value, matching Aura::GetDuration.
// A spell unresolvable from DBC data is left alone: its two exclusions
// cannot be checked, so stripping it could remove an exempt aura.
func (s *session) removeShortDurationPositiveAuras() {
	if s == nil || s.player == nil {
		return
	}
	var ids []uint32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || !aura.Positive || aura.DurationMs == 0 {
			continue
		}
		remaining := aura.RemainingMs
		if !aura.DurationUpdatedAt.IsZero() {
			if elapsed := time.Since(aura.DurationUpdatedAt).Milliseconds(); elapsed > 0 {
				if uint64(elapsed) < uint64(remaining) {
					remaining -= uint32(elapsed)
				} else {
					remaining = 0
				}
			}
		}
		if remaining > 30*1000 {
			continue
		}
		if s.server == nil || s.server.Data == nil {
			continue
		}
		spell, found, _ := s.server.Data.Spell(aura.SpellID)
		if !found {
			continue
		}
		if spell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 || spellHasAura(spell, spellAuraInvisibility) {
			continue
		}
		ids = append(ids, aura.SpellID)
	}
	for _, id := range ids {
		s.removeAura(id)
	}
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
	arena.RVPillarSwitchTimer = time.Now().Add(RVPillarFirstSwitchDelay)

	// Blade's Edge (BattlegroundBE::StartingEventOpenDoors): the two starting
	// doors are removed 5s after the match begins (BattlegroundBE.cpp:49-57).
	// Go arenas spawn no gameobjects, so only the removal state is tracked.
	if arena.MapID == ArenaMapBladesEdge {
		arena.DoorsRemoved = false
		arena.BEDoorsRemoveAt = time.Now().Add(ArenaDoorRemovalDelay)
	}
	// Dalaran Sewers (BattlegroundDS::StartingEventOpenDoors): reset the pipe
	// knockback count/timer and arm the first waterfall warning. The water-
	// door toggles and water-spout triggered self-casts (58873/57405) have no
	// Go model, so the timing machine is tracked and the physical arms are
	// skipped.
	if arena.MapID == ArenaMapDalaranSewers {
		arena.DSPipeCount = 0
		arena.DSPipeAt = time.Now().Add(DSPipeKnockbackFirstDelay)
		arena.DSWaterfallEvent = DSEventWaterfallWarning
		arena.DSWaterfallAt = time.Now().Add(dsWaterfallRandomDelay())
		arena.DSWaterfallActive = false
	}
	// Nagrand Arena (BattlegroundNA::StartingEventOpenDoors) and Ruins of
	// Lordaeron (BattlegroundRL::StartingEventOpenDoors): the two starting
	// doors are removed 5s after the match begins (BattlegroundNA.cpp:56-64,
	// BattlegroundRL.cpp:56-64). Go arenas spawn no gameobjects, so only
	// the removal state is tracked.
	if arena.MapID == ArenaMapNagrand || arena.MapID == ArenaMapRuinsOfLordaeron {
		arena.DoorsRemoved = false
		time.AfterFunc(ArenaDoorRemovalDelay, func() {
			// PostUpdateImpl gate (BattlegroundNA.cpp:29-34,
			// BattlegroundRL.cpp:29-34): the REMOVE_DOORS event only fires
			// while the match is in progress.
			arena.mu.Lock()
			defer arena.mu.Unlock()
			if arena.Status == ArenaStatusInProgress {
				arena.DoorsRemoved = true
			}
		})
	}
	arena.mu.Unlock()

	s.broadcastArenaStartTextToRoster(arena, ArenaTextStartBattleHasBegun, "The arena battle has begun!")

	// The start-event-4 arm iterates GetPlayers() (Battleground.cpp:465),
	// the roster, not the map: a sim-accepted player holds no arena map yet.
	for _, sess := range s.arenaRosterSessions(arena) {
		sess.removeAura(SpellArenaPreparation)
		s.resetPlayerPowers(sess)

		// Battleground.cpp:476-484: non-GM players lose non-permanent
		// positive auras with 30s or less remaining, except
		// UNAFFECTED_BY_INVULNERABILITY spells and invisibility auras.
		if !sessionIsGameMaster(sess) {
			sess.removeShortDurationPositiveAuras()
		}

		// Remove Warlock Demonic Circle if on Dalaran Sewers
		if arena.MapID == ArenaMapDalaranSewers {
			sess.removeAura(SpellWarlDemonicCircle)
		}

		// Update queue status on this arena's own slot (arenaMatchQueueSlot),
		// not the first active arena slot, which may belong to another
		// arena type the player also holds.
		if idx := arenaMatchQueueSlot(sess, arena); idx >= 0 {
			// BattlegroundQueue::PlayerInvitedToBGUpdateAverageWaitTime
			// (BattlegroundQueue.cpp:242): the arena match start is the
			// invite point for arena queues. C++ keys arenas by the
			// rated flag (BattlegroundQueue.cpp:251-253): rated ->
			// TEAM_HORDE, else TEAM_ALLIANCE.
			if sess.bgQueues[idx].Status == BGStatusWaitQueue && !sess.bgQueues[idx].JoinTime.IsZero() {
				teamIdx := uint8(0)
				if sess.bgQueues[idx].IsRated {
					teamIdx = 1
				}
				s.recordBGQueueWait(sess.bgQueues[idx].BgTypeID, teamIdx, uint32(time.Since(sess.bgQueues[idx].JoinTime).Milliseconds()))
			}
			sess.bgQueues[idx].Status = BGStatusInProgress
			sess.bgQueues[idx].StartTime = arena.MatchStartTime
			sess.sendBattlefieldStatus(uint8(idx))
		}
	}
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

	// Reference arena end criteria: PLAY_ARENA, WIN_ARENA.
	// Killing blows stay on the scoreboard only (arena.Scores.KillingBlows);
	// the GET_KILLING_BLOWS achievement leg is per-kill via Unit::Kill
	// (creditKillingBlowCriteria), not arena-end.
	arena.mu.RLock()
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
	s.creditArenaParticipants(arena, winningMembers)

	// Victory announcement
	if winner == int8(ArenaTeamGold) {
		s.broadcastArenaMessageToRoster(arena, fmt.Sprintf("%s is victorious!", arena.GoldTeamName))
	} else if winner == int8(ArenaTeamGreen) {
		s.broadcastArenaMessageToRoster(arena, fmt.Sprintf("%s is victorious!", arena.GreenTeamName))
	} else {
		s.broadcastArenaMessageToRoster(arena, "The arena match ended in a draw!")
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
	ctx := context.Background()
	// EndBattleground iterates m_Players (Battleground.cpp:724), the roster,
	// not the map: a sim-accepted player holds no arena map yet.
	for _, sess := range s.arenaRosterSessions(arena) {
		// Battleground::EndBattleground per-player arms (Battleground.cpp:728-802):
		// remove spirit of redemption (the reference removes the linked
		// SPELL_AURA_MOD_SHAPESHIFT auras, Battleground.cpp:732-734).
		if sess.hasAuraType(spellAuraSpiritOfRedemption) {
			sess.removeTargetAurasByType(ctx, sess.playerGUID, spellAuraModShapeshift)
		}
		// ResurrectPlayer(1.0f) + SpawnCorpseBones for the dead
		// (Battleground.cpp:736-739); the living CombatStop() arm has no Go
		// model (no combat-stop API in engine/world), no-bridge.
		if sess.player.Health == 0 {
			sess.resurrectPlayer(ctx, 1.0)
			sess.spawnCorpseBones(ctx)
		}
		s.resetPlayerPowers(sess)                      // ResetAllPowers (Battleground.cpp:791)
		sess.sendClientControl(sess.playerGUID, false) // BlockMovement (Battleground.cpp:808-812)
		// The honor-bonus / arena-point reward arms (Battleground.cpp:742-786)
		// are gated by IsRandom() || BattlegroundMgr::IsBGWeekend(GetTypeID());
		// arenas are never random and no arena type is a BG weekend, so the
		// gate is false here: no bonus honor, no arena points, and the
		// random-winner flag is never set at arena end. Rated vs skirmish is
		// decided by the Arena::EndBattleground isRated() arm above.
		if _, won := winningMembers[sess.playerGUID]; won {
			sess.updateAchievementCriteria(criteriaTypeWinBG, mapID, 1) // ACHIEVEMENT_CRITERIA_TYPE_WIN_BG (Battleground.cpp:788)
		}
		sess.updateAchievementCriteria(criteriaTypeCompleteBattleground, mapID, 1) // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_BATTLEGROUND (Battleground.cpp:802)

		_ = sess.write(uint16(protocol.OpcodeMSG_PVP_LOG_DATA), logData, true)

		// Update queue entry on this arena's own slot (arenaMatchQueueSlot),
		// not the first active arena slot, which may belong to another
		// arena type the player also holds.
		if idx := arenaMatchQueueSlot(sess, arena); idx >= 0 {
			sess.bgQueues[idx].Status = BGStatusWaitLeave
			sess.sendBattlefieldStatus(uint8(idx))
		}
	}
}

// processArenaLeave ports Battleground::_ProcessLeave (Battleground.cpp:506-528):
// after ArenaTimeToAutoRemove, RemovePlayerAtLeave(guid, true, true) runs for
// every remaining player. When the roster empties, the instance is deleted from
// the manager (the m_SetDeleteThis arm of BattlegroundMgr::Update,
// BattlegroundMgr.cpp:95-125).
func (s *Server) processArenaLeave(arena *arenaBattlegroundState) {
	if s == nil || arena == nil {
		return
	}

	arena.mu.Lock()
	guids := make([]uint64, 0, len(arena.PlayerTeams))
	for guid := range arena.PlayerTeams {
		guids = append(guids, guid)
	}
	arena.mu.Unlock()

	ctx := context.Background()
	for _, guid := range guids {
		s.removeArenaPlayerAtLeave(ctx, arena, guid)
	}

	arena.mu.Lock()
	empty := len(arena.PlayerTeams) == 0
	arena.mu.Unlock()
	if empty {
		s.deleteArenaState(arena)
	}
}

// removeArenaPlayerAtLeave ports the arena arms of
// Battleground::RemovePlayerAtLeave(guid, Transport=true, SendPacket=true)
// (Battleground.cpp:810-932).
func (s *Server) removeArenaPlayerAtLeave(ctx context.Context, arena *arenaBattlegroundState, guid uint64) {
	if s == nil || arena == nil {
		return
	}

	var sess *session
	s.sessionsMu.RLock()
	for cand := range s.sessions {
		if cand.playerGUID == guid && cand.worldReady.Load() && cand.player != nil {
			sess = cand
			break
		}
	}
	s.sessionsMu.RUnlock()

	if sess != nil {
		// Battleground.cpp:826-828: strip spirit of redemption.
		if sess.hasAuraType(spellAuraSpiritOfRedemption) {
			sess.removeTargetAurasByType(ctx, guid, spellAuraModShapeshift)
		}
		// Battleground.cpp:830: strip mounted auras.
		sess.removeTargetAurasByType(ctx, guid, spellAuraMounted)
		// Battleground.cpp:832-835: resurrect the dead and spawn corpse bones.
		if sess.player.Health == 0 {
			sess.resurrectPlayer(ctx, 1.0)
			sess.spawnCorpseBones(ctx)
		}
		// Battleground.cpp:880-881: unsummon the current pet, resummon the old one.
		sess.unsummonPet(ctx, petSaveNotInSlot)
		sess.resummonTemporaryPet(ctx)
		// SendPacket=true arm (Battleground.cpp:883-887): the STATUS_NONE
		// battlefield-status packet; the cleared slot is the Go form of
		// RemoveBattlegroundQueueId, and sendBattlefieldStatus writes the
		// 12-byte none form for an inactive slot.
		// Clear this arena's own slot (arenaMatchQueueSlot): the first-active
		// slot may belong to another arena type the player also holds, and
		// clearing it would destroy that invite while leaving this match's
		// slot dangling.
		if idx := arenaMatchQueueSlot(sess, arena); idx >= 0 {
			sess.bgQueues[idx] = bgQueueEntry{}
			sess.sendBattlefieldStatus(uint8(idx))
		}
		// Battleground::RemovePlayerAtLeave trailing arms
		// (Battleground.cpp:894-920), which the voluntary-leave funnel
		// already runs but the timer sweep skipped: drop the player
		// from the team's BG raid (reads bgData, so it runs before
		// the zeroing below), clear the battleground id and team, and
		// reset the BG-map achievement criteria. Without these a
		// timer-swept player kept a stale in-BG id (death/group/guild/
		// LFG gates key off bgData.InstanceID), stayed in the BG raid,
		// and kept BG-map criteria after teleporting to the entry point.
		s.removePlayerFromBGRaid(sess)
		sess.bgData.InstanceID = 0
		sess.bgData.Team = 0
		sess.resetAchievementCriteriaByCondition(criteriaConditionBGMap, arena.MapID)
		// The team flag and preparation auras addPlayerToArena applies
		// are stripped on the funnel path (handleArenaPlayerLeave); the
		// timer sweep left them on through the entry-point teleport.
		sess.removeAura(SpellAllianceGoldFlag)
		sess.removeAura(SpellAllianceGreenFlag)
		sess.removeAura(SpellHordeGoldFlag)
		sess.removeAura(SpellHordeGreenFlag)
		sess.removeAura(SpellArenaPreparation)
		// Transport=true arm (Battleground.cpp:925-927):
		// Player::TeleportToBGEntryPoint.
		if sess.bgData.JoinMap != 0 {
			sess.teleportTo(sess.bgData.JoinMap, sess.bgData.JoinX, sess.bgData.JoinY, sess.bgData.JoinZ, sess.bgData.JoinO)
		}
	}

	// Battleground.cpp:818-839: erase the player and delete their score.
	arena.mu.Lock()
	delete(arena.PlayerTeams, guid)
	delete(arena.Scores, guid)
	arena.mu.Unlock()
}

// deleteArenaState removes a finished arena instance from the manager, the Go
// form of BattlegroundMgr::Update deleting a ToBeDeleted battleground
// (BattlegroundMgr.cpp:95-125: erase from the instance map, erase the
// client-instance-id mapping, `delete bg`). The Go registry is keyed by
// map+instance rather than instance id, and there is no client-instance-id
// mapping to erase.
func (s *Server) deleteArenaState(arena *arenaBattlegroundState) {
	if s == nil || arena == nil {
		return
	}
	s.arenaMu.Lock()
	defer s.arenaMu.Unlock()
	for key, state := range s.arenaState {
		if state == arena {
			delete(s.arenaState, key)
			return
		}
	}
}

// awardLastManStanding grants SpellLastManStanding (26549) to the solely alive participant.
// Arena::EndBattleground casts it over GetPlayers() (Arena.cpp:270), the roster,
// not the map: a sim-accepted roster member holds no arena map yet, so the old
// map gate let the sole alive participant miss the aura while a map bystander
// on the winning team could never receive it (the roster check excluded them).
func (s *Server) awardLastManStanding(arena *arenaBattlegroundState, team uint8) {
	for _, sess := range s.arenaRosterSessions(arena) {
		if sess.player.Health > 0 {
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
		return true
	}

	switch entry {
	case ArenaGOShadowSight1, ArenaGOShadowSight2:
		sess.applyAura(SpellShadowSight)
		s.setGameObjectHidden(guid, true)
		s.broadcastGameObjectDespawn(sess.player.Map, guid)
		// No C++ pickup broadcast exists in the Battlegrounds tree (the
		// GO use just casts the buff spell); the announcement itself is
		// Go-side, but its audience follows the same m_Players rule as
		// every other arena announcement, not the map's players.
		if arena := s.findArenaState(sess.player.Map, 0); arena != nil {
			s.broadcastArenaMessageToRoster(arena, fmt.Sprintf("%s has picked up Shadow Sight!", sess.player.Name))
		} else {
			s.broadcastArenaMessage(sess.player.Map, fmt.Sprintf("%s has picked up Shadow Sight!", sess.player.Name))
		}
		s.scheduleArenaShadowSightRespawn(sess.player.Map, guid)
		return true
	default:
		return true
	}
}

// scheduleArenaShadowSightRespawn arms the Shadow Sight buff respawn at the
// C++ respawn delay (120s NA/RL/BE/DS, 90s RV — see ArenaShadowSightRespawnDelay
// consts). The unhide re-enables the buff click, mirroring the C++ respawn of
// the buff gameobject; setGameObjectHidden(false) is the established respawn
// primitive (EOTS/WSG flag-return precedent). A fire landing after the match
// ended is a no-op (EOTS respawn-timer battle-over gate).
func (s *Server) scheduleArenaShadowSightRespawn(mapID uint32, guid uint64) {
	delay := ArenaShadowSightRespawnDelay
	if mapID == ArenaMapRingOfValor {
		delay = ArenaShadowSightRespawnDelayRV
	}
	time.AfterFunc(delay, func() {
		if s == nil {
			return
		}
		if arena := s.findArenaState(mapID, 0); arena != nil && arena.Winner != ArenaTeamNone {
			return
		}
		s.setGameObjectHidden(guid, false)
	})
}

// updateArenaDamageScore updates damage done for a player in the arena score table.
func (s *Server) updateArenaDamageScore(attacker *session, damage uint32) {
	if s == nil || attacker == nil || attacker.player == nil {
		return
	}
	arena := s.findArenaStateForPlayer(attacker.playerGUID)
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
	if s == nil || healer == nil || healer.player == nil {
		return
	}
	arena := s.findArenaStateForPlayer(healer.playerGUID)
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

// resolveArenaStartText resolves an arena start broadcast_text ID from the
// world DB, falling back to the hardcoded English text when the row is
// missing (Battleground::SendBroadcastText, Arena.h:25-28, Arena.cpp:68-71).
func (s *Server) resolveArenaStartText(textID uint32, fallback string) string {
	text := fallback
	if s != nil && s.WorldStore != nil && s.WorldStore.DB != nil {
		var dbText string
		if err := s.WorldStore.DB.QueryRowContext(context.Background(), "SELECT Text FROM broadcast_text WHERE ID = ?", textID).Scan(&dbText); err == nil && dbText != "" {
			text = dbText
		}
	}
	return text
}

// broadcastArenaMessageToRoster sends a system chat message to the arena's
// roster only. C++ SendBroadcastText runs BroadcastWorker over m_Players
// (Battleground.cpp:592-602), not over the map's players.
func (s *Server) broadcastArenaMessageToRoster(arena *arenaBattlegroundState, msg string) {
	if s == nil || arena == nil {
		return
	}
	for _, sess := range s.arenaRosterSessions(arena) {
		sess.sendSystemMessage(msg)
	}
}

// broadcastArenaStartTextToRoster resolves an arena start broadcast_text ID
// and sends it to the roster only.
func (s *Server) broadcastArenaStartTextToRoster(arena *arenaBattlegroundState, textID uint32, fallback string) {
	if s == nil || arena == nil {
		return
	}
	s.broadcastArenaMessageToRoster(arena, s.resolveArenaStartText(textID, fallback))
}

// handleArenaAreaTrigger dispatches arena map-script area triggers for an
// in-progress arena (status gate applied by the caller in areatrigger.go).
// Reference: BattlegroundNA/BE/RL/DS/RV::HandleAreaTrigger; Battleground::
// HandleAreaTrigger is empty, so only the arena map scripts have trigger arms.
func (s *session) handleArenaAreaTrigger(triggerID, mapID uint32) {
	if s == nil || s.player == nil {
		return
	}
	switch mapID {
	case ArenaMapDalaranSewers:
		switch triggerID {
		case 5347, 5348:
			// BattlegroundDS::HandleAreaTrigger: the pipe area strips Demonic
			// Circle; if the pipe knockback has already been performed, the
			// count resets so the player is kicked into the arena again.
			s.removeAura(SpellWarlDemonicCircle)
			if arena := s.server.findArenaState(mapID, 0); arena != nil {
				arena.mu.Lock()
				if arena.DSPipeCount >= DSPipeKnockbackTotalCount {
					arena.DSPipeCount = 0
				}
				arena.mu.Unlock()
			}
		}
	case ArenaMapNagrand:
		// 4536, 4537: swallowed buff triggers, no-op in C++ and Go.
	case ArenaMapBladesEdge:
		// 4538, 4539: swallowed buff triggers, no-op in C++ and Go.
	case ArenaMapRuinsOfLordaeron:
		// 4696, 4697: swallowed buff triggers, no-op in C++ and Go.
	case ArenaMapRingOfValor:
		// 5224, 5226, 5473, 5474: fire removed in 3.2.0, no-op in C++ and Go.
	}
}

// updateArenaBattles ticks every live arena, driving warmup countdowns, match
// start, the match time limit, and per-map hazards. BattlegroundMgr::Update
// (BattlegroundMgr.cpp:94) sweeps all running instances with bg->Update(diff)
// every BATTLEGROUND_OBJECTIVE_UPDATE_INTERVAL (BattlegroundMgr.h:38 = 1000ms);
// the 1s gate here mirrors that cadence.
func (s *Server) updateArenaBattles(now time.Time) {
	if s == nil {
		return
	}
	if !s.arenaTickLast.IsZero() && now.Sub(s.arenaTickLast) < time.Second {
		return
	}
	s.arenaTickLast = now
	s.arenaMu.RLock()
	arenas := make([]*arenaBattlegroundState, 0, len(s.arenaState))
	for _, arena := range s.arenaState {
		arenas = append(arenas, arena)
	}
	s.arenaMu.RUnlock()
	for _, arena := range arenas {
		s.updateArenaTick(arena, now)
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
	endTime := arena.EndTime
	arena.mu.Unlock()

	if status == ArenaStatusWaitJoin {
		elapsed := now.Sub(startTime)
		remaining := ArenaWarmupLength - elapsed

		if remaining <= 60*time.Second && remaining > 30*time.Second && !arena.AnnouncedOneMinute {
			arena.AnnouncedOneMinute = true
			s.broadcastArenaStartTextToRoster(arena, ArenaTextStartOneMinute, "One minute until the arena battle begins!")
		} else if remaining <= 30*time.Second && remaining > 15*time.Second && !arena.AnnouncedThirtySeconds {
			arena.AnnouncedThirtySeconds = true
			s.broadcastArenaStartTextToRoster(arena, ArenaTextStartThirtySeconds, "Thirty seconds until the arena battle begins!")
		} else if remaining <= 15*time.Second && remaining > 0 && !arena.AnnouncedFifteenSeconds {
			arena.AnnouncedFifteenSeconds = true
			s.broadcastArenaStartTextToRoster(arena, ArenaTextStartFifteenSeconds, "Fifteen seconds until the arena battle begins!")
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
				arena.RVPillarSwitchTimer = now.Add(RVPillarSwitchInterval)
			}
			arena.mu.Unlock()
		}

		// Dalaran Sewers waterfall / pipe-knockback machine
		// (BattlegroundDS::PostUpdateImpl, BattlegroundDS.cpp:37-87).
		if arena.MapID == ArenaMapDalaranSewers {
			s.updateDSWaterfall(arena, now)
		}

		// Blade's Edge door removal (BattlegroundBE::PostUpdateImpl): the
		// REMOVE_DOORS event deletes starting doors 0-1 five seconds after
		// the match begins (BattlegroundBE.cpp:49-57).
		if arena.MapID == ArenaMapBladesEdge {
			arena.mu.Lock()
			if !arena.DoorsRemoved && !arena.BEDoorsRemoveAt.IsZero() && !now.Before(arena.BEDoorsRemoveAt) {
				arena.DoorsRemoved = true
			}
			arena.mu.Unlock()
		}
	} else if status == ArenaStatusWaitLeave {
		// Battleground::_ProcessLeave (Battleground.cpp:506-528) plus the
		// m_SetDeleteThis sweep (Battleground.cpp:164-186,
		// BattlegroundMgr::Update): once the match ends, every remaining player
		// is auto-removed after TIME_TO_AUTOREMOVE, and the emptied instance is
		// deleted from the manager.
		arena.mu.Lock()
		remaining := len(arena.PlayerTeams)
		arena.mu.Unlock()
		if remaining == 0 {
			s.deleteArenaState(arena)
		} else if !endTime.IsZero() && now.Sub(endTime) >= ArenaTimeToAutoRemove {
			s.processArenaLeave(arena)
		}
	}
}

// dsWaterfallRandomDelay rolls the 30-60s waterfall re-arm delay
// (ScheduleEvent(..., BG_DS_WATERFALL_TIMER_MIN, BG_DS_WATERFALL_TIMER_MAX)).
func dsWaterfallRandomDelay() time.Duration {
	return DSWaterfallTimerMin + time.Duration(rand.Int63n(int64(DSWaterfallTimerMax-DSWaterfallTimerMin)))
}

// updateDSWaterfall drives the Dalaran Sewers waterfall / pipe-knockback
// machine from BattlegroundDS::PostUpdateImpl (BattlegroundDS.cpp:37-87) on
// the 1s arena tick. The C++ EventMap arms are reproduced in order:
// WATERFALL_WARNING (30-60s) -> 5s warning -> WATERFALL_ON (30s, knockback
// every 1.5s) -> WATERFALL_OFF -> re-arm WARNING. The DoorClose/DoorOpen water
// toggles and the water-spout triggered self-casts (58873/57405) have no Go
// model: Go arenas spawn no gameobjects and expose no server-side creature-
// cast API, so the machine tracks timing state only.
func (s *Server) updateDSWaterfall(arena *arenaBattlegroundState, now time.Time) {
	arena.mu.Lock()
	defer arena.mu.Unlock()

	// Pipe knockback loop (BattlegroundDS.cpp:78-93): first cast 5s after
	// match start, then every 3s, DSPipeKnockbackTotalCount casts total.
	if arena.DSPipeCount < DSPipeKnockbackTotalCount && !arena.DSPipeAt.IsZero() && !now.Before(arena.DSPipeAt) {
		arena.DSPipeCount++
		arena.DSPipeAt = now.Add(DSPipeKnockbackDelay)
	}

	// Waterfall knockback repeat (BG_DS_EVENT_WATERFALL_KNOCKBACK): every
	// 1.5s while the waterfall is on; cancelled at WATERFALL_OFF by clearing
	// DSWaterfallActive (BattlegroundDS.cpp:72-75, 70).
	if arena.DSWaterfallActive && !arena.DSKnockbackAt.IsZero() && !now.Before(arena.DSKnockbackAt) {
		arena.DSKnockbackAt = now.Add(DSWaterfallKnockbackEvery)
	}

	// Waterfall event machine (BattlegroundDS.cpp:44-76).
	if arena.DSWaterfallEvent == 0 || arena.DSWaterfallAt.IsZero() || now.Before(arena.DSWaterfallAt) {
		return
	}
	switch arena.DSWaterfallEvent {
	case DSEventWaterfallWarning:
		// Add the water (DoorClose WATER_2, no Go model); warn for 5s.
		arena.DSWaterfallEvent = DSEventWaterfallOn
		arena.DSWaterfallAt = now.Add(DSWaterfallWarningTime)
	case DSEventWaterfallOn:
		// Active collision and start knockback timer (DoorClose WATER_1).
		arena.DSWaterfallEvent = DSEventWaterfallOff
		arena.DSWaterfallAt = now.Add(DSWaterfallDuration)
		arena.DSWaterfallActive = true
		arena.DSKnockbackAt = now.Add(DSWaterfallKnockbackEvery)
	case DSEventWaterfallOff:
		// Remove collision and water (DoorOpen WATER_1/WATER_2).
		arena.DSWaterfallActive = false
		arena.DSWaterfallEvent = DSEventWaterfallWarning
		arena.DSWaterfallAt = now.Add(dsWaterfallRandomDelay())
	}
}
