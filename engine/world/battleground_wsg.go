package world

import (
	"context"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Warsong Gulch (WSG) Constants mirroring TrinityCore BattlegroundWS.h / BattlegroundWS.cpp.
const (
	WSGMapID uint32 = 489

	WSGAllianceFlagBaseEntry    uint32 = 179830
	WSGHordeFlagBaseEntry       uint32 = 179831
	WSGAllianceFlagDroppedEntry uint32 = 179785
	WSGHordeFlagDroppedEntry    uint32 = 179786

	WSGSpellSilverwingFlag uint32 = 23335 // Carried by Horde
	WSGSpellWarsongFlag    uint32 = 23333 // Carried by Alliance

	// Stalemate debuff (BattlegroundWS.h:154-155): when both flags are kept
	// simultaneously, carriers gain Focused Assault after 10 minutes and
	// Brutal Assault after 15 total (BattlegroundWS.cpp:158-192).
	WSGSpellFocusedAssault uint32 = 46392
	WSGSpellBrutalAssault  uint32 = 46393

	wsgStalemateStage1Delay = 10 * time.Minute
	wsgStalemateStage2Delay = 5 * time.Minute // stage 2 fires at 15 minutes total

	WSGFlagStateOnBase   uint32 = 1
	WSGFlagStateOnPlayer uint32 = 2
	WSGFlagStateOnGround uint32 = 3
	// WSGFlagStateWaitRespawn is Go-internal: C++ BG_WS_FLAG_STATE_WAIT_RESPAWN=1
	// collides with Go's OnBase=1 worldstate convention, so the capture-respawn
	// window gets its own internal state while still broadcasting 1.
	WSGFlagStateWaitRespawn uint32 = 4

	WSGFlagRespawnTime = 23 * time.Second // BG_WS_FLAG_RESPAWN_TIME (BattlegroundWS.h:27)
	WSGFlagDropTime    = 10 * time.Second // BG_WS_FLAG_DROP_TIME (BattlegroundWS.h:28)

	WSGSpellSilverwingFlagPicked uint32 = 61265 // BG_WS_SPELL_SILVERWING_FLAG_PICKED (fake spell, timed-achievement event)
	WSGSpellWarsongFlagPicked    uint32 = 61266 // BG_WS_SPELL_WARSONG_FLAG_PICKED

	WSGFactionSilverwingSentinels uint32 = 890 // Alliance WSG reputation faction
	WSGFactionWarsongOutriders    uint32 = 889 // Horde WSG reputation faction
	WSGReputationCapture          uint32 = 35  // m_ReputationCapture non-weekend (BattlegroundWS.cpp:740); Go has no BG-weekend model

	// BG-objective ids credited by BattlegroundWS::UpdatePlayerScore
	// (BattlegroundWS.cpp:779-793): flag captures credit WS_OBJECTIVE_CAPTURE_FLAG,
	// flag returns credit WS_OBJECTIVE_RETURN_FLAG (BattlegroundWS.h:160-161).
	WSGObjectiveCaptureFlag uint32 = 42
	WSGObjectiveReturnFlag  uint32 = 44

	WSWorldStateAllianceCaptures  uint32 = 1581
	WSWorldStateHordeCaptures     uint32 = 1582
	WSWorldStateMaxCaptures       uint32 = 1601
	WSWorldStateHordeFlagState    uint32 = 2338
	WSWorldStateAllianceFlagState uint32 = 2339
	// BG_WS_FLAG_UNK_ALLIANCE/HORDE (BattlegroundWS.h:75-76): 1 while the flag
	// is carried, uint32(-1) while it lies on the ground, 0 otherwise —
	// updated by the pickup/drop/capture arms, not the flag-state worldstates.
	WSWorldStateFlagUnkAlliance uint32 = 1545
	WSWorldStateFlagUnkHorde    uint32 = 1546
	// BG_WS_STATE_TIMER / BG_WS_STATE_TIMER_ACTIVE (BattlegroundWS.h:83-84):
	// the 25-minute match clock driven by PostUpdateImpl (cpp:87-110).
	WSWorldStateTimer       uint32 = 4248
	WSWorldStateTimerActive uint32 = 4247
)

type wsgBattlegroundState struct {
	mu                  sync.Mutex
	MapID               uint32
	AllianceScore       uint32
	HordeScore          uint32
	MaxScore            uint32
	Winner              int8 // -1 = ongoing, 0 = Alliance, 1 = Horde, 2 = draw; set by endWSGVictory/endWSGTie, read by the MSG_PVP_LOG_DATA ended block (battleground_scores.go)
	AllianceFlagState   uint32
	HordeFlagState      uint32
	AllianceCarrierGUID uint64
	HordeCarrierGUID    uint64
	AllianceBaseGUID    uint64
	HordeBaseGUID       uint64
	AllianceDroppedGUID uint64
	HordeDroppedGUID    uint64
	AllianceReturnTimer *time.Timer
	HordeReturnTimer    *time.Timer
	// WaitRespawnTimer arms mirror BattlegroundWS::PostUpdateImpl's _flagsTimer
	// countdown (BattlegroundWS.cpp:112-122): the captured flag returns to its
	// base BG_WS_FLAG_RESPAWN_TIME after a capture via RespawnFlag(team, true).
	AllianceWaitRespawnTimer *time.Timer
	HordeWaitRespawnTimer    *time.Timer
	// StalemateTimer/StalemateStage are the Go form of _flagSpellForceTimer +
	// _flagDebuffState (BattlegroundWS.h:274, BattlegroundWS.cpp:155-192): the
	// one-shot timer armed while both flags are kept, escalating 0 -> 1
	// (Focused Assault) at 10 minutes -> 2 (Brutal Assault) at 15.
	StalemateTimer *time.Timer
	StalemateStage uint8
	// LastFlagCaptureTeam mirrors _lastFlagCaptureTeam (BattlegroundWS.h:267):
	// written by SetLastFlagCapture on every capture (BattlegroundWS.cpp:352),
	// zeroed by Reset (cpp:744-745), and the tiebreak source for the 27-minute
	// timeout end (cpp:97-99). Go teams: 0 = alliance, 1 = horde.
	LastFlagCaptureTeam uint32
	// MinutesElapsed mirrors _minutesElapsed (BattlegroundWS.h:275);
	// PostUpdateImpl increments it once per minute past the 3-minute mark
	// (BattlegroundWS.cpp:106-109).
	MinutesElapsed uint8
	// MatchStartTime / MatchMinuteTimer are the Go form of GetStartTime() for
	// the timeout arm (PostUpdateImpl, BattlegroundWS.cpp:87-110): Go has no
	// StartingEventCloseDoors or STATUS_IN_PROGRESS, so the clock arms lazily
	// on the first flag interaction of the match.
	MatchStartTime   time.Time
	MatchMinuteTimer *time.Timer
}

func isWSGFlag(entry uint32) bool {
	switch entry {
	case WSGAllianceFlagBaseEntry, WSGHordeFlagBaseEntry, WSGAllianceFlagDroppedEntry, WSGHordeFlagDroppedEntry:
		return true
	}
	return false
}

func (s *Server) getOrCreateWSGState(mapID uint32) *wsgBattlegroundState {
	if s == nil {
		return nil
	}
	s.wsgMu.Lock()
	defer s.wsgMu.Unlock()
	if s.wsgState == nil {
		s.wsgState = make(map[uint32]*wsgBattlegroundState)
	}
	state, ok := s.wsgState[mapID]
	if !ok {
		state = &wsgBattlegroundState{
			MapID:             mapID,
			AllianceScore:     0,
			HordeScore:        0,
			MaxScore:          3,
			Winner:            -1,
			AllianceFlagState: WSGFlagStateOnBase,
			HordeFlagState:    WSGFlagStateOnBase,
		}
		s.wsgState[mapID] = state
	}
	return state
}

func (s *Server) handleWSGFlagUse(ctx context.Context, sess *session, guid uint64, entry uint32) bool {
	if s == nil || sess == nil || sess.player == nil {
		return true
	}
	wsg := s.getOrCreateWSGState(sess.player.Map)
	if wsg == nil {
		return true
	}

	wsg.mu.Lock()
	defer wsg.mu.Unlock()

	team := teamForRace(sess.player.Race) // 0 = Alliance, 1 = Horde

	switch entry {
	case WSGAllianceFlagBaseEntry:
		// Enemy Horde clicks Alliance base flag -> Pickup
		if team == 1 {
			if wsg.AllianceFlagState == WSGFlagStateOnBase {
				wsg.AllianceFlagState = WSGFlagStateOnPlayer
				wsg.AllianceCarrierGUID = sess.playerGUID
				wsg.AllianceBaseGUID = guid

				sess.applyAura(WSGSpellSilverwingFlag)
				sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellSilverwingFlagPicked)
				s.setGameObjectHidden(guid, true)
				s.broadcastGameObjectDespawn(sess.player.Map, guid)
				// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:480):
				// UpdateFlagState(HORDE, BG_WS_FLAG_STATE_ON_PLAYER) — the
				// worldstate is indexed by the carrying team, not the flag.
				s.wsgUpdateFlagState(sess.player.Map, 1, WSGFlagStateOnPlayer)
				// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:478):
				// UpdateWorldState(BG_WS_FLAG_UNK_ALLIANCE, 1) on pickup.
				s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkAlliance, 1)
				s.broadcastBattlegroundMessage(sess.player.Map, "The Alliance flag was picked up by "+sess.player.Name+"!")
			}
		} else if team == 0 {
			// Alliance player touches own base flag while carrying Horde flag -> Capture!
			if wsg.HordeCarrierGUID == sess.playerGUID && wsg.AllianceFlagState == WSGFlagStateOnBase {
				sess.removeAura(WSGSpellWarsongFlag)
				// BattlegroundWS::EventPlayerCapturedFlag (BattlegroundWS.cpp:303-306):
				// the stalemate debuff leaves with the capturing keeper.
				sess.removeAura(WSGSpellFocusedAssault)
				sess.removeAura(WSGSpellBrutalAssault)
				wsg.HordeCarrierGUID = 0
				wsg.HordeFlagState = WSGFlagStateWaitRespawn
				wsg.AllianceScore++
				// BattlegroundWS::EventPlayerCapturedFlag
				// (BattlegroundWS.cpp:352): SetLastFlagCapture(player->GetTeam())
				// — the tiebreak source for the 27-minute timeout end.
				wsg.LastFlagCaptureTeam = 0

				s.rewardBGEndReputation(sess.player.Map, 0, WSGFactionSilverwingSentinels, WSGReputationCapture)
				s.rewardBGEndHonor(sess.player.Map, 0, 2)

				s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceCaptures, wsg.AllianceScore)
				// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_CAPTURES arm
				// (BattlegroundWS.cpp:779-787): the capture credits
				// ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE with
				// WS_OBJECTIVE_CAPTURE_FLAG (C++ calls UpdateTeamScore first,
				// then UpdatePlayerScore — this credit follows the captures
				// worldstate broadcast, mirroring that order).
				s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveCaptureFlag)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.FlagCaptures++ })
				// BattlegroundWS::EventPlayerCapturedFlag (BattlegroundWS.cpp:346):
				// UpdateFlagState(player->GetTeam(), 1) — capturer's team index.
				s.wsgUpdateFlagState(sess.player.Map, 0, WSGFlagStateOnBase)
				s.startWSGFlagRespawnTimer(wsg, sess.player.Map, 1)
				s.broadcastBattlegroundMessage(sess.player.Map, sess.player.Name+" captured the Warsong flag!")
				if wsg.AllianceScore >= wsg.MaxScore {
					// EventPlayerCapturedFlag's winner arm (BattlegroundWS.cpp:361-365):
					// clear the flag worldstates before EndBattleground.
					s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkAlliance, 0)
					s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkHorde, 0)
					s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
					s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
					s.endWSGVictory(wsg, sess.player.Map, 0)
				}
			}
		}

	case WSGHordeFlagBaseEntry:
		// Enemy Alliance clicks Horde base flag -> Pickup
		if team == 0 {
			if wsg.HordeFlagState == WSGFlagStateOnBase {
				wsg.HordeFlagState = WSGFlagStateOnPlayer
				wsg.HordeCarrierGUID = sess.playerGUID
				wsg.HordeBaseGUID = guid

				sess.applyAura(WSGSpellWarsongFlag)
				sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellWarsongFlagPicked)
				s.setGameObjectHidden(guid, true)
				s.broadcastGameObjectDespawn(sess.player.Map, guid)
				// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:498):
				// UpdateFlagState(ALLIANCE, BG_WS_FLAG_STATE_ON_PLAYER) — the
				// worldstate is indexed by the carrying team, not the flag.
				s.wsgUpdateFlagState(sess.player.Map, 0, WSGFlagStateOnPlayer)
				// BattlegroundWS::EventPlayerClickedOnFlag: UpdateWorldState
				// (BG_WS_FLAG_UNK_HORDE, 1) on pickup (BattlegroundWS.cpp:494).
				s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkHorde, 1)
				s.broadcastBattlegroundMessage(sess.player.Map, "The Warsong flag was picked up by "+sess.player.Name+"!")
			}
		} else if team == 1 {
			// Horde player touches own base flag while carrying Alliance flag -> Capture!
			if wsg.AllianceCarrierGUID == sess.playerGUID && wsg.HordeFlagState == WSGFlagStateOnBase {
				sess.removeAura(WSGSpellSilverwingFlag)
				// BattlegroundWS::EventPlayerCapturedFlag (BattlegroundWS.cpp:320-323):
				// the stalemate debuff leaves with the capturing keeper.
				sess.removeAura(WSGSpellFocusedAssault)
				sess.removeAura(WSGSpellBrutalAssault)
				wsg.AllianceCarrierGUID = 0
				wsg.AllianceFlagState = WSGFlagStateWaitRespawn
				wsg.HordeScore++
				// BattlegroundWS::EventPlayerCapturedFlag
				// (BattlegroundWS.cpp:352): SetLastFlagCapture(player->GetTeam()).
				wsg.LastFlagCaptureTeam = 1

				s.rewardBGEndReputation(sess.player.Map, 1, WSGFactionWarsongOutriders, WSGReputationCapture)
				s.rewardBGEndHonor(sess.player.Map, 1, 2)

				s.broadcastWorldState(sess.player.Map, WSWorldStateHordeCaptures, wsg.HordeScore)
				// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_CAPTURES arm
				// (BattlegroundWS.cpp:779-787): WS_OBJECTIVE_CAPTURE_FLAG
				// objective credit, after the team-score worldstate like the
				// C++ UpdateTeamScore -> UpdatePlayerScore order.
				s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveCaptureFlag)
				s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.FlagCaptures++ })
				// BattlegroundWS::EventPlayerCapturedFlag (BattlegroundWS.cpp:346):
				// UpdateFlagState(player->GetTeam(), 1) — capturer's team index.
				s.wsgUpdateFlagState(sess.player.Map, 1, WSGFlagStateOnBase)
				s.startWSGFlagRespawnTimer(wsg, sess.player.Map, 0)
				s.broadcastBattlegroundMessage(sess.player.Map, sess.player.Name+" captured the Alliance flag!")
				if wsg.HordeScore >= wsg.MaxScore {
					// EventPlayerCapturedFlag's winner arm (BattlegroundWS.cpp:361-365).
					s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkAlliance, 0)
					s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkHorde, 0)
					s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
					s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
					s.endWSGVictory(wsg, sess.player.Map, 1)
				}
			}
		}

	case WSGAllianceFlagDroppedEntry:
		s.despawnDynamicGameObject(guid)
		if wsg.AllianceReturnTimer != nil {
			wsg.AllianceReturnTimer.Stop()
			wsg.AllianceReturnTimer = nil
		}
		if team == 0 {
			// Friendly return
			wsg.AllianceFlagState = WSGFlagStateOnBase
			if wsg.AllianceBaseGUID != 0 {
				s.setGameObjectHidden(wsg.AllianceBaseGUID, false)
			}
			// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:513):
			// UpdateFlagState(HORDE, BG_WS_FLAG_STATE_WAIT_RESPAWN) — the team
			// that had carried the flag, not the flag's own team.
			s.wsgUpdateFlagState(sess.player.Map, 1, WSGFlagStateOnBase)
			// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_RETURNS arm
			// (BattlegroundWS.cpp:788-791): the return credits
			// ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE with
			// WS_OBJECTIVE_RETURN_FLAG.
			s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveReturnFlag)
			s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.FlagReturns++ })
			s.broadcastBattlegroundMessage(sess.player.Map, "The Alliance flag was returned to its base by "+sess.player.Name+"!")
		} else {
			// Enemy pickup
			wsg.AllianceFlagState = WSGFlagStateOnPlayer
			wsg.AllianceCarrierGUID = sess.playerGUID
			sess.applyAura(WSGSpellSilverwingFlag)
			sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellSilverwingFlagPicked)
			// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:528):
			// UpdateFlagState(HORDE, BG_WS_FLAG_STATE_ON_PLAYER) — the
			// worldstate is indexed by the carrying team, not the flag.
			s.wsgUpdateFlagState(sess.player.Map, 1, WSGFlagStateOnPlayer)
			// EventPlayerClickedOnFlag, ground-pickup leg (BattlegroundWS.cpp:526):
			// UpdateWorldState(BG_WS_FLAG_UNK_ALLIANCE, 1).
			s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkAlliance, 1)
			s.broadcastBattlegroundMessage(sess.player.Map, "The Alliance flag was picked up by "+sess.player.Name+"!")
		}

	case WSGHordeFlagDroppedEntry:
		s.despawnDynamicGameObject(guid)
		if wsg.HordeReturnTimer != nil {
			wsg.HordeReturnTimer.Stop()
			wsg.HordeReturnTimer = nil
		}
		if team == 1 {
			// Friendly return
			wsg.HordeFlagState = WSGFlagStateOnBase
			if wsg.HordeBaseGUID != 0 {
				s.setGameObjectHidden(wsg.HordeBaseGUID, false)
			}
			// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:546):
			// UpdateFlagState(ALLIANCE, BG_WS_FLAG_STATE_WAIT_RESPAWN) — the
			// team that had carried the flag, not the flag's own team.
			s.wsgUpdateFlagState(sess.player.Map, 0, WSGFlagStateOnBase)
			// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_RETURNS arm
			// (BattlegroundWS.cpp:788-791): WS_OBJECTIVE_RETURN_FLAG objective
			// credit for the returning player.
			s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveReturnFlag)
			s.addBGObjectiveScore(sess, func(score *bgPlayerScore) { score.FlagReturns++ })
			s.broadcastBattlegroundMessage(sess.player.Map, "The Warsong flag was returned to its base by "+sess.player.Name+"!")
		} else {
			// Enemy pickup
			wsg.HordeFlagState = WSGFlagStateOnPlayer
			wsg.HordeCarrierGUID = sess.playerGUID
			sess.applyAura(WSGSpellWarsongFlag)
			sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellWarsongFlagPicked)
			// BattlegroundWS::EventPlayerClickedOnFlag (BattlegroundWS.cpp:561):
			// UpdateFlagState(ALLIANCE, BG_WS_FLAG_STATE_ON_PLAYER) — the
			// worldstate is indexed by the carrying team, not the flag.
			s.wsgUpdateFlagState(sess.player.Map, 0, WSGFlagStateOnPlayer)
			// EventPlayerClickedOnFlag, ground-pickup leg (BattlegroundWS.cpp:565):
			// UpdateWorldState(BG_WS_FLAG_UNK_HORDE, 1).
			s.broadcastWorldState(sess.player.Map, WSWorldStateFlagUnkHorde, 1)
			s.broadcastBattlegroundMessage(sess.player.Map, "The Warsong flag was picked up by "+sess.player.Name+"!")
		}
	}

	sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)

	// Stalemate-debuff state machine (BattlegroundWS.cpp:155-192): re-arm or
	// tear down after this flag interaction.
	s.refreshWSGStalemate(wsg, sess.player.Map)

	// Match clock (BattlegroundWS::PostUpdateImpl timer arms, cpp:87-110):
	// the 25-minute countdown anchors on the first flag interaction of the
	// match (Go has no StartingEventCloseDoors/STATUS_IN_PROGRESS).
	s.armWSGMatchTimer(wsg, sess.player.Map)

	return true
}

// startWSGFlagRespawnTimer mirrors BattlegroundWS::PostUpdateImpl's _flagsTimer
// countdown (BattlegroundWS.cpp:112-122): the captured team's flag state is
// BG_WS_FLAG_STATE_WAIT_RESPAWN for BG_WS_FLAG_RESPAWN_TIME, then
// RespawnFlag(team, true) returns it to base with the BG_WS_TEXT_FLAGS_PLACED
// broadcast. The caller's wsg.mu is held; team is 0 Alliance, 1 Horde.
func (s *Server) startWSGFlagRespawnTimer(wsg *wsgBattlegroundState, mapID uint32, team uint32) {
	arm := func(timer **time.Timer, state *uint32, baseGUID *uint64) {
		if *timer != nil {
			(*timer).Stop()
		}
		*timer = time.AfterFunc(WSGFlagRespawnTime, func() {
			wsg.mu.Lock()
			defer wsg.mu.Unlock()
			*timer = nil
			if *state == WSGFlagStateWaitRespawn {
				*state = WSGFlagStateOnBase
				if *baseGUID != 0 {
					s.setGameObjectHidden(*baseGUID, false)
				}
				// BattlegroundWS::RespawnFlag (cpp:242-262) sends no worldstate —
				// the capture arm's capturer-team=1 value stands.
				s.broadcastBattlegroundMessage(mapID, "The flags were placed!")
			}
		})
	}
	if team == 0 {
		arm(&wsg.AllianceWaitRespawnTimer, &wsg.AllianceFlagState, &wsg.AllianceBaseGUID)
	} else {
		arm(&wsg.HordeWaitRespawnTimer, &wsg.HordeFlagState, &wsg.HordeBaseGUID)
	}
}

func (s *Server) handleWSGPlayerDeath(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	s.wsgMu.RLock()
	wsg := s.wsgState[sess.player.Map]
	s.wsgMu.RUnlock()
	if wsg == nil {
		return
	}

	wsg.mu.Lock()
	defer wsg.mu.Unlock()

	s.dropWSGCarriedFlag(wsg, sess, sess.player.Map, sess.player.X, sess.player.Y, sess.player.Z)
}

// dropWSGCarriedFlag drops the Warsong flag carried by sess, mirroring
// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:289): the
// carried flag object spawns at the player's feet, the dropper's team flag
// worldstate (2338/2339) flips to 1 per UpdateFlagState (cpp:448), and the
// flag-return timer starts. The wsg mutex must be held.
func (s *Server) dropWSGCarriedFlag(wsg *wsgBattlegroundState, sess *session, mapID uint32, x, y, z float32) {
	if wsg == nil || sess == nil {
		return
	}
	if wsg.AllianceCarrierGUID == sess.playerGUID {
		sess.removeAura(WSGSpellSilverwingFlag)
		// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:418-421):
		// the stalemate debuff leaves with the dropper.
		sess.removeAura(WSGSpellFocusedAssault)
		sess.removeAura(WSGSpellBrutalAssault)
		wsg.AllianceCarrierGUID = 0
		wsg.AllianceFlagState = WSGFlagStateOnGround

		droppedLow := s.nextDynamicGameObjectLowGUID()
		droppedGUID := gameObjectGUID(droppedLow, WSGAllianceFlagDroppedEntry)
		wsg.AllianceDroppedGUID = droppedGUID

		s.spawnDynamicGameObject(&dynamicGameObjectState{
			GUID:           droppedGUID,
			LowGUID:        droppedLow,
			Entry:          WSGAllianceFlagDroppedEntry,
			Map:            mapID,
			X:              x,
			Y:              y,
			Z:              z,
			State:          GameObjectStateReady,
			Type:           GameObjectTypeFlagDrop,
			DisplayID:      WSGAllianceFlagDroppedEntry,
			Size:           1.0,
			IsRuntimeSpawn: true,
		})

		// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:448):
		// UpdateFlagState(player->GetTeam(), 1) — the dropper's team index,
		// value 1 (C++ never sends 3/on-ground on the wire).
		s.wsgUpdateFlagState(mapID, teamForRace(sess.player.Race), WSGFlagStateOnBase)
		// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:449-457):
		// UpdateWorldState(BG_WS_FLAG_UNK_ALLIANCE, uint32(-1)) on drop.
		s.broadcastWorldState(mapID, WSWorldStateFlagUnkAlliance, 0xFFFFFFFF)
		s.broadcastBattlegroundMessage(mapID, "The Alliance flag was dropped by "+sess.player.Name+"!")

		if wsg.AllianceReturnTimer != nil {
			wsg.AllianceReturnTimer.Stop()
		}
		wsg.AllianceReturnTimer = time.AfterFunc(WSGFlagDropTime, func() {
			wsg.mu.Lock()
			defer wsg.mu.Unlock()
			if wsg.AllianceFlagState == WSGFlagStateOnGround {
				s.despawnDynamicGameObject(wsg.AllianceDroppedGUID)
				wsg.AllianceDroppedGUID = 0
				wsg.AllianceFlagState = WSGFlagStateOnBase
				if wsg.AllianceBaseGUID != 0 {
					s.setGameObjectHidden(wsg.AllianceBaseGUID, false)
				}
				// BattlegroundWS::RespawnFlagAfterDrop (cpp:266-286) sends no
				// worldstate — the drop arm's carrier-team=1 value stands.
				s.broadcastBattlegroundMessage(mapID, "The Alliance flag was returned to its base!")
			}
		})
	}

	if wsg.HordeCarrierGUID == sess.playerGUID {
		sess.removeAura(WSGSpellWarsongFlag)
		// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:432-435):
		// the stalemate debuff leaves with the dropper.
		sess.removeAura(WSGSpellFocusedAssault)
		sess.removeAura(WSGSpellBrutalAssault)
		wsg.HordeCarrierGUID = 0
		wsg.HordeFlagState = WSGFlagStateOnGround

		droppedLow := s.nextDynamicGameObjectLowGUID()
		droppedGUID := gameObjectGUID(droppedLow, WSGHordeFlagDroppedEntry)
		wsg.HordeDroppedGUID = droppedGUID

		s.spawnDynamicGameObject(&dynamicGameObjectState{
			GUID:           droppedGUID,
			LowGUID:        droppedLow,
			Entry:          WSGHordeFlagDroppedEntry,
			Map:            mapID,
			X:              x,
			Y:              y,
			Z:              z,
			State:          GameObjectStateReady,
			Type:           GameObjectTypeFlagDrop,
			DisplayID:      WSGHordeFlagDroppedEntry,
			Size:           1.0,
			IsRuntimeSpawn: true,
		})

		// BattlegroundWS::EventPlayerDroppedFlag (BattlegroundWS.cpp:448):
		// UpdateFlagState(player->GetTeam(), 1) — the dropper's team index,
		// value 1 (C++ never sends 3/on-ground on the wire).
		s.wsgUpdateFlagState(mapID, teamForRace(sess.player.Race), WSGFlagStateOnBase)
		// BattlegroundWS::EventPlayerDroppedFlag: UpdateWorldState
		// (BG_WS_FLAG_UNK_HORDE, uint32(-1)) on drop (BattlegroundWS.cpp:449-457).
		s.broadcastWorldState(mapID, WSWorldStateFlagUnkHorde, 0xFFFFFFFF)
		s.broadcastBattlegroundMessage(mapID, "The Warsong flag was dropped by "+sess.player.Name+"!")

		if wsg.HordeReturnTimer != nil {
			wsg.HordeReturnTimer.Stop()
		}
		wsg.HordeReturnTimer = time.AfterFunc(WSGFlagDropTime, func() {
			wsg.mu.Lock()
			defer wsg.mu.Unlock()
			if wsg.HordeFlagState == WSGFlagStateOnGround {
				s.despawnDynamicGameObject(wsg.HordeDroppedGUID)
				wsg.HordeDroppedGUID = 0
				wsg.HordeFlagState = WSGFlagStateOnBase
				if wsg.HordeBaseGUID != 0 {
					s.setGameObjectHidden(wsg.HordeBaseGUID, false)
				}
				// BattlegroundWS::RespawnFlagAfterDrop (cpp:266-286) sends no
				// worldstate — the drop arm's carrier-team=1 value stands.
				s.broadcastBattlegroundMessage(mapID, "The Warsong flag was returned to its base!")
			}
		})
	}

	// Stalemate-debuff state machine (BattlegroundWS.cpp:155-192): the drop
	// ends any both-kept window.
	s.refreshWSGStalemate(wsg, mapID)
}

func (s *Server) handleWSGPlayerLeave(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	s.wsgMu.RLock()
	wsg := s.wsgState[sess.player.Map]
	s.wsgMu.RUnlock()
	if wsg == nil {
		return
	}

	wsg.mu.Lock()
	defer wsg.mu.Unlock()

	mapID := sess.player.Map
	team := teamForRace(sess.player.Race) // 0 = Alliance, 1 = Horde
	if wsg.AllianceCarrierGUID == sess.playerGUID {
		sess.removeAura(WSGSpellSilverwingFlag)
		// BattlegroundWS::RemovePlayer -> EventPlayerDroppedFlag: the stalemate
		// debuff leaves with the leaving keeper (BattlegroundWS.cpp:418-421).
		sess.removeAura(WSGSpellFocusedAssault)
		sess.removeAura(WSGSpellBrutalAssault)
		wsg.AllianceCarrierGUID = 0
		wsg.AllianceFlagState = WSGFlagStateOnBase
		if wsg.AllianceBaseGUID != 0 {
			s.setGameObjectHidden(wsg.AllianceBaseGUID, false)
		}
		// Return-to-base is the Go standing simplification (C++
		// BattlegroundWS::RemovePlayer drops via EventPlayerDroppedFlag);
		// the worldstate follows the UpdateFlagState(player->GetTeam(), 1)
		// carrier-team convention (BattlegroundWS.cpp:448).
		s.wsgUpdateFlagState(mapID, team, WSGFlagStateOnBase)
		s.broadcastBattlegroundMessage(mapID, "The Alliance flag was returned to its base!")
	}

	if wsg.HordeCarrierGUID == sess.playerGUID {
		sess.removeAura(WSGSpellWarsongFlag)
		// BattlegroundWS::RemovePlayer -> EventPlayerDroppedFlag: the stalemate
		// debuff leaves with the leaving keeper (BattlegroundWS.cpp:432-435).
		sess.removeAura(WSGSpellFocusedAssault)
		sess.removeAura(WSGSpellBrutalAssault)
		wsg.HordeCarrierGUID = 0
		wsg.HordeFlagState = WSGFlagStateOnBase
		if wsg.HordeBaseGUID != 0 {
			s.setGameObjectHidden(wsg.HordeBaseGUID, false)
		}
		s.wsgUpdateFlagState(mapID, team, WSGFlagStateOnBase)
		s.broadcastBattlegroundMessage(mapID, "The Warsong flag was returned to its base!")
	}

	// Stalemate-debuff state machine (BattlegroundWS.cpp:155-192): the leave
	// ends any both-kept window.
	s.refreshWSGStalemate(wsg, mapID)
}

func (s *Server) getWSGFlagCarriers(mapID uint32) []*session {
	if s == nil {
		return nil
	}
	s.wsgMu.RLock()
	wsg := s.wsgState[mapID]
	s.wsgMu.RUnlock()
	if wsg == nil {
		return nil
	}

	wsg.mu.Lock()
	allyCarrier := wsg.AllianceCarrierGUID
	hordeCarrier := wsg.HordeCarrierGUID
	wsg.mu.Unlock()

	var carriers []*session
	if allyCarrier != 0 {
		if sess := s.findSessionByGUID(allyCarrier); sess != nil && sess.player != nil && sess.player.Map == mapID {
			carriers = append(carriers, sess)
		}
	}
	if hordeCarrier != 0 {
		if sess := s.findSessionByGUID(hordeCarrier); sess != nil && sess.player != nil && sess.player.Map == mapID {
			carriers = append(carriers, sess)
		}
	}
	return carriers
}

func (s *Server) broadcastWorldState(mapID uint32, variableID, value uint32) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU32(variableID)
	buf.WriteU32(value)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_UPDATE_WORLD_STATE), buf.Bytes())
}

// updateInstanceWorldState is the instance-scoped twin of broadcastWorldState:
// it mirrors InstanceScript::DoUpdateWorldState (InstanceScript.cpp:555-567),
// which sends SMSG_UPDATE_WORLD_STATE only to the players of that instance,
// not to the whole map.
func (s *Server) updateInstanceWorldState(mapID, instanceID, variableID, value uint32) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU32(variableID)
	buf.WriteU32(value)
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil ||
			target.player.Map != mapID || target.player.InstanceID != instanceID {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_UPDATE_WORLD_STATE), buf.Bytes(), true)
	}
}

// wsgUpdateFlagState mirrors BattlegroundWS::UpdateFlagState
// (BattlegroundWS.cpp:602-608): the flag worldstates 2338/2339 are indexed by
// the ACTING player's team (2339 Alliance, 2338 Horde), NOT by the flag's own
// team — C++ passes player->GetTeam() at every call site (pickup, drop,
// return, capture). Values are 2 while the acting team carries a flag
// (BG_WS_FLAG_STATE_ON_PLAYER) and literal 1 for every other transition
// (drop/return/capture pass BG_WS_FLAG_STATE_WAIT_RESPAWN=1); Go's
// WSGFlagStateOnBase is numerically 1 and carries that wire value. team is
// 0 = Alliance, 1 = Horde.
func (s *Server) wsgUpdateFlagState(mapID uint32, team uint32, value uint32) {
	if team == 0 {
		s.broadcastWorldState(mapID, WSWorldStateAllianceFlagState, value)
	} else {
		s.broadcastWorldState(mapID, WSWorldStateHordeFlagState, value)
	}
}

func (s *Server) broadcastBattlegroundMessage(mapID uint32, message string) {
	if s == nil {
		return
	}
	payload := protocol.BuildChatMessageWithOptions(chatSystem, 0, 0, 0, message, "", false, "", 0)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload)
}

// sendWSGInitWorldStatesTo serializes the live WSG world states into
// SMSG_INIT_WORLD_STATES. Reference: BattlegroundWS::FillInitialWorldStates
// (BattlegroundWS.cpp:820-858): live capture scores (1581/1582), the
// flag-unk states (1545/1546: 1 while carried, -1 on the ground, 0
// otherwise), the max captures (1601), the timer states (4247 active + 4248
// minutes remaining while the match clock runs), and the flag states
// (2338/2339: 2 while carried, 1 otherwise). 1547=2 is preserved from the
// previous static init block — the C++ header comments it out
// (BattlegroundWS.h:77) and FillInitialWorldStates never sends it, but it was
// already on the wire for this map.
func (s *Server) sendWSGInitWorldStatesTo(sess *session) {
	if sess == nil || sess.player == nil {
		return
	}
	wsg := s.getOrCreateWSGState(sess.player.Map)
	if wsg == nil {
		return
	}
	wsg.mu.Lock()
	defer wsg.mu.Unlock()

	unk := func(flagState uint32) int32 {
		switch flagState {
		case WSGFlagStateOnPlayer:
			return 1
		case WSGFlagStateOnGround:
			return -1
		default:
			return 0
		}
	}
	flagShown := func(flagState uint32) int32 {
		if flagState == WSGFlagStateOnPlayer {
			return 2
		}
		return 1
	}

	worldStates := [][2]int32{
		{int32(WSWorldStateAllianceCaptures), int32(wsg.AllianceScore)},
		{int32(WSWorldStateHordeCaptures), int32(wsg.HordeScore)},
		{int32(WSWorldStateFlagUnkAlliance), unk(wsg.AllianceFlagState)},
		{int32(WSWorldStateFlagUnkHorde), unk(wsg.HordeFlagState)},
		{1547, 2},
		{int32(WSWorldStateMaxCaptures), int32(wsg.MaxScore)},
	}
	if wsg.MatchMinuteTimer != nil {
		worldStates = append(worldStates,
			[2]int32{int32(WSWorldStateTimerActive), 1},
			[2]int32{int32(WSWorldStateTimer), int32(25) - int32(wsg.MinutesElapsed)},
		)
	} else {
		worldStates = append(worldStates, [2]int32{int32(WSWorldStateTimerActive), 0})
	}
	worldStates = append(worldStates,
		[2]int32{int32(WSWorldStateHordeFlagState), flagShown(wsg.HordeFlagState)},
		[2]int32{int32(WSWorldStateAllianceFlagState), flagShown(wsg.AllianceFlagState)},
	)

	packet := protocol.NewBuffer(16 + len(worldStates)*8)
	packet.WriteI32(int32(sess.player.Map))
	packet.WriteI32(int32(sess.player.Zone))
	packet.WriteI32(0)
	packet.WriteU16(uint16(len(worldStates)))
	for _, ws := range worldStates {
		packet.WriteI32(ws[0])
		packet.WriteI32(ws[1])
	}
	_ = sess.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), packet.Bytes(), true)
}

// endWSGVictory runs the Warsong Gulch end-of-match rewards.
// Reference: BattlegroundWS::EndBattleground (BattlegroundWS.cpp:755): the
// winning team gets GetBonusHonorFromKill(m_HonorWinKills), then BOTH teams
// get GetBonusHonorFromKill(m_HonorEndKills), ahead of Battleground::EndBattleground.
// The kill counts are BG-weekend gated in C++ (3/4 on weekend, 1/2 otherwise,
// BattlegroundWS.cpp:733-743); Go has no BG-weekend model, so the non-weekend
// defaults apply.
//
// The caller's wsg.mu is held (the flag-capture arms of handleWSGFlagUse and
// endWSGTimeout).
func (s *Server) endWSGVictory(wsg *wsgBattlegroundState, mapID uint32, winningTeam uint32) {
	s.stopWSGMatchTimers(wsg)
	// EventPlayerCapturedFlag's winner arm (BattlegroundWS.cpp:366): the match
	// clock stops showing once the battle ends.
	s.broadcastWorldState(mapID, WSWorldStateTimerActive, 0)
	// The match is over: the stalemate-debuff state machine
	// (BattlegroundWS.cpp:155-192) ends with the instance. Carrier auras are
	// already stripped by the capturing arm's removal (BattlegroundWS.cpp:303-306,
	// 320-323); only the stage bookkeeping needs clearing here.
	wsg.StalemateStage = 0
	wsg.Winner = int8(winningTeam)
	teamName := "Alliance"
	if winningTeam == 1 {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(mapID, "The "+teamName+" wins!")
	s.rewardBGEndHonor(mapID, winningTeam, 1)
	s.rewardBGEndHonor(mapID, 0, 2)
	s.rewardBGEndHonor(mapID, 1, 2)
}

// stopWSGMatchTimers stops every armed WSG timer under the caller's wsg.mu.
// Reference: Battleground::EndBattleground (Battleground.cpp:667) sets the
// winner and the instance leaves STATUS_IN_PROGRESS, so the PostUpdateImpl
// arms (BattlegroundWS.cpp:87-192) never tick again. Go models the windows as
// time.AfterFunc, so each must be stopped explicitly (the lock is also held by
// the timer callbacks, so Stop never races a running callback). The winning
// capture arms a respawn timer just before this runs, so it must be cancelled
// here or it would unhide the flag and broadcast "The flags were placed!" into
// the finished battle.
func (s *Server) stopWSGMatchTimers(wsg *wsgBattlegroundState) {
	for _, timer := range [6]**time.Timer{
		&wsg.AllianceWaitRespawnTimer, &wsg.HordeWaitRespawnTimer,
		&wsg.AllianceReturnTimer, &wsg.HordeReturnTimer,
		&wsg.StalemateTimer,
		&wsg.MatchMinuteTimer,
	} {
		if *timer != nil {
			(*timer).Stop()
			*timer = nil
		}
	}
}

// endWSGTimeout is the 27-minute timeout arm of BattlegroundWS::PostUpdateImpl
// (BattlegroundWS.cpp:87-104). The caller's wsg.mu is held (fired from
// wsgMatchMinute).
func (s *Server) endWSGTimeout(wsg *wsgBattlegroundState, mapID uint32) {
	ally, horde := wsg.AllianceScore, wsg.HordeScore
	switch {
	case ally == 0 && horde == 0:
		// No one scored - result is tie (cpp:91-92: EndBattleground(0)).
		s.endWSGTie(wsg, mapID)
	case ally == 0:
		// Horde has more points and thus wins (cpp:93-95).
		s.endWSGVictory(wsg, mapID, 1)
	case horde == 0:
		// Alliance has > 0, Horde has 0, alliance wins (cpp:96-97).
		s.endWSGVictory(wsg, mapID, 0)
	case horde == ally:
		// Team score equal, winner is team that scored the last flag
		// (cpp:97-99: EndBattleground(_lastFlagCaptureTeam)).
		s.endWSGVictory(wsg, mapID, wsg.LastFlagCaptureTeam)
	case horde > ally:
		// Last but not least, check who has the higher score (cpp:100-103).
		s.endWSGVictory(wsg, mapID, 1)
	default:
		s.endWSGVictory(wsg, mapID, 0)
	}
}

// endWSGTie is the C++ EndBattleground(0) neutral-winner arm
// (Battleground.cpp:667-688: SetWinner(PVP_TEAM_NEUTRAL), no win broadcast).
// The win bonus is gated on winner==ALLIANCE/HORDE so a tie pays only the
// end honor to both teams (BattlegroundWS::EndBattleground, cpp:755+).
// The caller's wsg.mu is held.
func (s *Server) endWSGTie(wsg *wsgBattlegroundState, mapID uint32) {
	s.stopWSGMatchTimers(wsg)
	s.broadcastWorldState(mapID, WSWorldStateTimerActive, 0)
	wsg.StalemateStage = 0
	wsg.Winner = 2
	s.rewardBGEndHonor(mapID, 0, 2)
	s.rewardBGEndHonor(mapID, 1, 2)
}

// armWSGMatchTimer starts the 25-minute match clock on the first flag
// interaction of a match. Reference: BattlegroundWS::StartingEventCloseDoors
// (BattlegroundWS.cpp:213-214: UpdateWorldState(BG_WS_STATE_TIMER_ACTIVE, 1),
// UpdateWorldState(BG_WS_STATE_TIMER, 25)). Go has no doors phase or
// STATUS_IN_PROGRESS, so the clock anchors lazily here. The caller's wsg.mu
// is held (handleWSGFlagUse).
func (s *Server) armWSGMatchTimer(wsg *wsgBattlegroundState, mapID uint32) {
	if !wsg.MatchStartTime.IsZero() {
		return
	}
	wsg.MatchStartTime = time.Now()
	s.broadcastWorldState(mapID, WSWorldStateTimerActive, 1)
	s.broadcastWorldState(mapID, WSWorldStateTimer, 25)
	wsg.MatchMinuteTimer = time.AfterFunc(time.Minute, func() {
		s.wsgMatchMinute(wsg, mapID, 1)
	})
}

// wsgMatchMinute is the event-driven Go form of the timer arms of
// BattlegroundWS::PostUpdateImpl (BattlegroundWS.cpp:87-110). C++ shows
// 25-1=24 once 3 minutes have elapsed (cpp:106-109: ++_minutesElapsed,
// UpdateWorldState(BG_WS_STATE_TIMER, 25 - _minutesElapsed)) and ends the
// match at 27 minutes (cpp:87-104). The minute chain reproduces that cadence
// exactly: ticks 1-2 are silent, ticks 3-26 broadcast 27-minute, tick 27 ends
// the match. The callback takes wsg.mu.
func (s *Server) wsgMatchMinute(wsg *wsgBattlegroundState, mapID uint32, minute uint8) {
	wsg.mu.Lock()
	defer wsg.mu.Unlock()
	wsg.MatchMinuteTimer = nil
	if minute >= 27 {
		s.endWSGTimeout(wsg, mapID)
		return
	}
	if minute >= 3 {
		wsg.MinutesElapsed = minute - 2
		s.broadcastWorldState(mapID, WSWorldStateTimer, uint32(27-minute))
	}
	wsg.MatchMinuteTimer = time.AfterFunc(time.Minute, func() {
		s.wsgMatchMinute(wsg, mapID, minute+1)
	})
}

// wsgPrematureWinner is the score-compare leg of
// BattlegroundWS::GetPrematureWinner (BattlegroundWS.cpp:861-869): the team
// ahead on flag captures wins a premature finish. Team ids match the rest of
// the Go WSG model (0 = alliance, 1 = horde). The tie fallthrough in C++ calls
// Battleground::GetPrematureWinner (Battleground.cpp:344-352), the
// min-players-per-team roster check — Go has no live BG-instance roster, so
// that base arm stays unported and a tie reports no winner. No live caller
// yet: Go has no premature-finish countdown
// (Battleground::_ProcessProgress / m_PrematureCountDown).
func wsgPrematureWinner(wsg *wsgBattlegroundState) (uint32, bool) {
	if wsg == nil {
		return 0, false
	}
	if wsg.AllianceScore > wsg.HordeScore {
		return 0, true
	}
	if wsg.HordeScore > wsg.AllianceScore {
		return 1, true
	}
	return 0, false
}

// refreshWSGStalemate is the event-driven Go form of the stalemate-debuff arm
// of BattlegroundWS::PostUpdateImpl (BattlegroundWS.cpp:158-192). C++ ticks
// _flagSpellForceTimer while both flags are kept and escalates _flagDebuffState
// at 10/15 minutes; Go arms a one-shot timer at each both-kept transition and
// re-checks the both-kept condition at fire time (a mid-window flap cancels
// and re-arms from zero on the next transition, matching the C++ reset in the
// else arm, BattlegroundWS.cpp:184-192). Call with wsg.mu held after any
// flag-state change.
func (s *Server) refreshWSGStalemate(wsg *wsgBattlegroundState, mapID uint32) {
	if wsg == nil {
		return
	}
	bothKept := wsg.AllianceFlagState == WSGFlagStateOnPlayer && wsg.HordeFlagState == WSGFlagStateOnPlayer
	if bothKept {
		if wsg.StalemateTimer == nil && wsg.StalemateStage == 0 {
			wsg.StalemateTimer = time.AfterFunc(wsgStalemateStage1Delay, func() {
				s.wsgStalemateStage(wsg, mapID, 1)
			})
		}
		return
	}
	if wsg.StalemateTimer != nil {
		wsg.StalemateTimer.Stop()
		wsg.StalemateTimer = nil
	}
	if wsg.StalemateStage != 0 {
		wsg.StalemateStage = 0
		s.removeWSGAssaultAuras(wsg)
	}
}

// wsgStalemateStage fires the stalemate timer: stage 1 casts Focused Assault on
// both keepers after 10 minutes (BattlegroundWS.cpp:161-168) and re-arms for
// stage 2; stage 2 swaps to Brutal Assault after 15 minutes total
// (BattlegroundWS.cpp:169-182). Missing keepers are skipped, like C++'s
// FindPlayer null arms.
func (s *Server) wsgStalemateStage(wsg *wsgBattlegroundState, mapID uint32, stage uint8) {
	wsg.mu.Lock()
	defer wsg.mu.Unlock()
	wsg.StalemateTimer = nil
	if wsg.AllianceFlagState != WSGFlagStateOnPlayer || wsg.HordeFlagState != WSGFlagStateOnPlayer {
		wsg.StalemateStage = 0
		return
	}
	carriers := [2]uint64{wsg.AllianceCarrierGUID, wsg.HordeCarrierGUID}
	if stage == 1 && wsg.StalemateStage == 0 {
		for _, guid := range carriers {
			if carrier := s.findSessionByGUID(guid); carrier != nil {
				carrier.applyAura(WSGSpellFocusedAssault)
			}
		}
		wsg.StalemateStage = 1
		wsg.StalemateTimer = time.AfterFunc(wsgStalemateStage2Delay, func() {
			s.wsgStalemateStage(wsg, mapID, 2)
		})
		return
	}
	if stage == 2 && wsg.StalemateStage == 1 {
		for _, guid := range carriers {
			if carrier := s.findSessionByGUID(guid); carrier != nil {
				carrier.removeAura(WSGSpellFocusedAssault)
				carrier.applyAura(WSGSpellBrutalAssault)
			}
		}
		wsg.StalemateStage = 2
	}
}

// removeWSGAssaultAuras strips the stalemate auras from both flag carriers,
// the Go form of the aura-removal arms in EventPlayerDroppedFlag
// (BattlegroundWS.cpp:418-421, 432-435), EventPlayerCapturedFlag
// (BattlegroundWS.cpp:303-306, 320-323), and the PostUpdateImpl else arm
// (BattlegroundWS.cpp:184-192).
func (s *Server) removeWSGAssaultAuras(wsg *wsgBattlegroundState) {
	for _, guid := range [2]uint64{wsg.AllianceCarrierGUID, wsg.HordeCarrierGUID} {
		if guid == 0 {
			continue
		}
		if carrier := s.findSessionByGUID(guid); carrier != nil {
			carrier.removeAura(WSGSpellFocusedAssault)
			carrier.removeAura(WSGSpellBrutalAssault)
		}
	}
}
