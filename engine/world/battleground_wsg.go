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
)

type wsgBattlegroundState struct {
	mu                  sync.Mutex
	MapID               uint32
	AllianceScore       uint32
	HordeScore          uint32
	MaxScore            uint32
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
				s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnPlayer)
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
				s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
				s.startWSGFlagRespawnTimer(wsg, sess.player.Map, 1)
				s.broadcastBattlegroundMessage(sess.player.Map, sess.player.Name+" captured the Warsong flag!")
				if wsg.AllianceScore >= wsg.MaxScore {
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
				s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnPlayer)
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

				s.rewardBGEndReputation(sess.player.Map, 1, WSGFactionWarsongOutriders, WSGReputationCapture)
				s.rewardBGEndHonor(sess.player.Map, 1, 2)

				s.broadcastWorldState(sess.player.Map, WSWorldStateHordeCaptures, wsg.HordeScore)
				// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_CAPTURES arm
				// (BattlegroundWS.cpp:779-787): WS_OBJECTIVE_CAPTURE_FLAG
				// objective credit, after the team-score worldstate like the
				// C++ UpdateTeamScore -> UpdatePlayerScore order.
				s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveCaptureFlag)
				s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
				s.startWSGFlagRespawnTimer(wsg, sess.player.Map, 0)
				s.broadcastBattlegroundMessage(sess.player.Map, sess.player.Name+" captured the Alliance flag!")
				if wsg.HordeScore >= wsg.MaxScore {
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
			s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
			// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_RETURNS arm
			// (BattlegroundWS.cpp:788-791): the return credits
			// ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE with
			// WS_OBJECTIVE_RETURN_FLAG.
			s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveReturnFlag)
			s.broadcastBattlegroundMessage(sess.player.Map, "The Alliance flag was returned to its base by "+sess.player.Name+"!")
		} else {
			// Enemy pickup
			wsg.AllianceFlagState = WSGFlagStateOnPlayer
			wsg.AllianceCarrierGUID = sess.playerGUID
			sess.applyAura(WSGSpellSilverwingFlag)
			sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellSilverwingFlagPicked)
			s.broadcastWorldState(sess.player.Map, WSWorldStateAllianceFlagState, WSGFlagStateOnPlayer)
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
			s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
			// BattlegroundWS::UpdatePlayerScore SCORE_FLAG_RETURNS arm
			// (BattlegroundWS.cpp:788-791): WS_OBJECTIVE_RETURN_FLAG objective
			// credit for the returning player.
			s.creditBGObjectiveCapture(sess.playerGUID, WSGObjectiveReturnFlag)
			s.broadcastBattlegroundMessage(sess.player.Map, "The Warsong flag was returned to its base by "+sess.player.Name+"!")
		} else {
			// Enemy pickup
			wsg.HordeFlagState = WSGFlagStateOnPlayer
			wsg.HordeCarrierGUID = sess.playerGUID
			sess.applyAura(WSGSpellWarsongFlag)
			sess.startTimedAchievement(timedTypeSpellTarget, WSGSpellWarsongFlagPicked)
			s.broadcastWorldState(sess.player.Map, WSWorldStateHordeFlagState, WSGFlagStateOnPlayer)
			s.broadcastBattlegroundMessage(sess.player.Map, "The Warsong flag was picked up by "+sess.player.Name+"!")
		}
	}

	sess.removeAurasWithInterruptFlags(auraInterruptFlagEnterPvPCombat)

	// Stalemate-debuff state machine (BattlegroundWS.cpp:155-192): re-arm or
	// tear down after this flag interaction.
	s.refreshWSGStalemate(wsg, sess.player.Map)

	return true
}

// startWSGFlagRespawnTimer mirrors BattlegroundWS::PostUpdateImpl's _flagsTimer
// countdown (BattlegroundWS.cpp:112-122): the captured team's flag state is
// BG_WS_FLAG_STATE_WAIT_RESPAWN for BG_WS_FLAG_RESPAWN_TIME, then
// RespawnFlag(team, true) returns it to base with the BG_WS_TEXT_FLAGS_PLACED
// broadcast. The caller's wsg.mu is held; team is 0 Alliance, 1 Horde.
func (s *Server) startWSGFlagRespawnTimer(wsg *wsgBattlegroundState, mapID uint32, team uint32) {
	arm := func(timer **time.Timer, state *uint32, baseGUID *uint64, worldStateID uint32) {
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
				s.broadcastWorldState(mapID, worldStateID, WSGFlagStateOnBase)
				s.broadcastBattlegroundMessage(mapID, "The flags were placed!")
			}
		})
	}
	if team == 0 {
		arm(&wsg.AllianceWaitRespawnTimer, &wsg.AllianceFlagState, &wsg.AllianceBaseGUID, WSWorldStateAllianceFlagState)
	} else {
		arm(&wsg.HordeWaitRespawnTimer, &wsg.HordeFlagState, &wsg.HordeBaseGUID, WSWorldStateHordeFlagState)
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
// carried flag object spawns at the player's feet, the worldstate flips
// to on-ground, and the flag-return timer starts. The wsg mutex must be held.
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

		s.broadcastWorldState(mapID, WSWorldStateAllianceFlagState, WSGFlagStateOnGround)
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
				s.broadcastWorldState(mapID, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
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

		s.broadcastWorldState(mapID, WSWorldStateHordeFlagState, WSGFlagStateOnGround)
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
				s.broadcastWorldState(mapID, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
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
		s.broadcastWorldState(mapID, WSWorldStateAllianceFlagState, WSGFlagStateOnBase)
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
		s.broadcastWorldState(mapID, WSWorldStateHordeFlagState, WSGFlagStateOnBase)
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

func (s *Server) broadcastBattlegroundMessage(mapID uint32, message string) {
	if s == nil {
		return
	}
	payload := protocol.BuildChatMessageWithOptions(chatSystem, 0, 0, 0, message, "", false, "", 0)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload)
}

// endWSGVictory runs the Warsong Gulch end-of-match rewards.
// Reference: BattlegroundWS::EndBattleground (BattlegroundWS.cpp:755): the
// winning team gets GetBonusHonorFromKill(m_HonorWinKills), then BOTH teams
// get GetBonusHonorFromKill(m_HonorEndKills), ahead of Battleground::EndBattleground.
// The kill counts are BG-weekend gated in C++ (3/4 on weekend, 1/2 otherwise,
// BattlegroundWS.cpp:733-743); Go has no BG-weekend model, so the non-weekend
// defaults apply.
//
// The caller's wsg.mu is held (both callers are the flag-capture arms of
// handleWSGFlagUse).
func (s *Server) endWSGVictory(wsg *wsgBattlegroundState, mapID uint32, winningTeam uint32) {
	// Reference: Battleground::EndBattleground (Battleground.cpp:667) sets the
	// winner and the instance leaves STATUS_IN_PROGRESS, so the _flagsTimer
	// arm (BattlegroundWS::PostUpdateImpl, BattlegroundWS.cpp:112-122) never
	// ticks again. Go models the respawn/return windows as time.AfterFunc, so
	// stop every armed flag timer here under the caller's wsg.mu (the lock is
	// also held by the timer callbacks, so Stop never races a running
	// callback). The winning capture arms a respawn timer just before this
	// runs, so it must be cancelled here or it would unhide the flag and
	// broadcast "The flags were placed!" into the finished battle.
	for _, timer := range [5]**time.Timer{
		&wsg.AllianceWaitRespawnTimer, &wsg.HordeWaitRespawnTimer,
		&wsg.AllianceReturnTimer, &wsg.HordeReturnTimer,
		&wsg.StalemateTimer,
	} {
		if *timer != nil {
			(*timer).Stop()
			*timer = nil
		}
	}
	// The match is over: the stalemate-debuff state machine
	// (BattlegroundWS.cpp:155-192) ends with the instance. Carrier auras are
	// already stripped by the capturing arm's removal (BattlegroundWS.cpp:303-306,
	// 320-323); only the stage bookkeeping needs clearing here.
	wsg.StalemateStage = 0
	teamName := "Alliance"
	if winningTeam == 1 {
		teamName = "Horde"
	}
	s.broadcastBattlegroundMessage(mapID, "The "+teamName+" wins!")
	s.rewardBGEndHonor(mapID, winningTeam, 1)
	s.rewardBGEndHonor(mapID, 0, 2)
	s.rewardBGEndHonor(mapID, 1, 2)
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
