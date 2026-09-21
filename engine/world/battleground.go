package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleBattlemasterHello processes CMSG_BATTLEMASTER_HELLO (0x2D7).
// Reference: WorldSession::HandleBattlemasterHelloOpcode (BattleGroundHandler.cpp:41).
func (s *session) handleBattlemasterHello(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	bmGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	return s.sendBattlefieldList(bmGUID, 0, 1) // default to Warsong Gulch or first BG
}

// handleBattlefieldList processes CMSG_BATTLEFIELD_LIST (0x23C).
// Reference: WorldSession::HandleBattlefieldListOpcode (BattleGroundHandler.cpp:338).
func (s *session) handleBattlefieldList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return false
	}
	fromWhere, _ := r.ReadU8()

	return s.sendBattlefieldList(0, fromWhere, bgTypeID)
}

func (s *session) sendBattlefieldList(bmGUID uint64, fromWhere uint8, bgTypeID uint32) bool {
	buf := protocol.NewBuffer(64)
	buf.WriteU64(bmGUID)
	buf.WriteU8(fromWhere)
	buf.WriteU32(bgTypeID)
	buf.WriteU8(0) // unk
	buf.WriteU8(0) // unk
	if s.randomBGWinner {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(0) // winHonor
	buf.WriteU32(0) // winArena
	buf.WriteU32(0) // lossHonor
	buf.WriteU8(0)  // isRandom
	buf.WriteU32(0) // count of active instances
	return s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_LIST), buf.Bytes(), true) == nil
}

// handleBattlemasterJoin processes CMSG_BATTLEMASTER_JOIN (0x2EE).
// Reference: WorldSession::HandleBattlemasterJoinOpcode (BattleGroundHandler.cpp:74).
func (s *session) handleBattlemasterJoin(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 17 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // guid
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return false
	}
	instanceID, _ := r.ReadU32()
	joinAsGroup, _ := r.ReadU8()
	_ = joinAsGroup

	// Find free queue slot
	slot := -1
	for i := 0; i < len(s.bgQueues); i++ {
		if !s.bgQueues[i].Active {
			slot = i
			break
		}
	}
	if slot == -1 {
		return true // Queues full
	}

	s.bgQueues[slot] = bgQueueEntry{
		Active:     true,
		BgTypeID:   bgTypeID,
		InstanceID: instanceID,
		JoinTime:   time.Now(),
		Status:     1, // STATUS_WAIT_QUEUE
	}

	s.sendBattlefieldStatus(uint8(slot))
	s.debug("queued for battleground", "account", s.accountName, "bg", bgTypeID, "slot", slot)
	return true
}

// handleBattlemasterJoinArena processes CMSG_BATTLEMASTER_JOIN_ARENA (0x358).
// Reference: WorldSession::HandleBattlemasterJoinArena (BattleGroundHandler.cpp:166).
func (s *session) handleBattlemasterJoinArena(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 11 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // guid
	arenaSlot, _ := r.ReadU8()
	asGroup, _ := r.ReadU8()
	isRated, _ := r.ReadU8()
	_ = asGroup

	arenaType := uint8(2)
	switch arenaSlot {
	case 0:
		arenaType = 2
	case 1:
		arenaType = 3
	case 2:
		arenaType = 5
	default:
		if arenaSlot == 2 || arenaSlot == 3 || arenaSlot == 5 {
			arenaType = arenaSlot
		}
	}

	slot := -1
	for i := 0; i < len(s.bgQueues); i++ {
		if !s.bgQueues[i].Active {
			slot = i
			break
		}
	}
	if slot == -1 {
		return true
	}

	s.bgQueues[slot] = bgQueueEntry{
		Active:       true,
		BgTypeID:     4, // BATTLEGROUND_AA (All Arenas)
		InstanceID:   0,
		JoinTime:     time.Now(),
		Status:       1, // STATUS_WAIT_QUEUE
		ArenaType:    arenaType,
		IsArena:      true,
		IsRated:      isRated != 0,
		ArenaFaction: 0,
	}

	s.sendBattlefieldStatus(uint8(slot))
	s.debug("queued for arena", "account", s.accountName, "slot", slot, "type", arenaType, "rated", isRated != 0)
	return true
}

// handleBattlefieldPort processes CMSG_BATTLEFIELD_PORT (0x2D5).
// Reference: WorldSession::HandleBattleFieldPortOpcode (BattleGroundHandler.cpp:357).
func (s *session) handleBattlefieldPort(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU8() // type
	_, _ = r.ReadU8() // unk2
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return false
	}
	_, _ = r.ReadU16() // unk
	action, _ := r.ReadU8()

	if action == 0 {
		// Leave queue
		for i := 0; i < len(s.bgQueues); i++ {
			if s.bgQueues[i].Active && s.bgQueues[i].BgTypeID == bgTypeID {
				s.bgQueues[i] = bgQueueEntry{}
				s.sendBattlefieldStatus(uint8(i))
				break
			}
		}
	}

	return true
}

// handleBattlefieldStatus processes CMSG_BATTLEFIELD_STATUS (0x2D3).
// Reference: WorldSession::HandleBattlefieldStatusOpcode (BattleGroundHandler.cpp:546).
func (s *session) handleBattlefieldStatus(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	for slot := uint8(0); slot < uint8(len(s.bgQueues)); slot++ {
		s.sendBattlefieldStatus(slot)
	}
	return true
}

func (s *session) sendBattlefieldStatus(slot uint8) {
	if int(slot) >= len(s.bgQueues) {
		return
	}
	entry := s.bgQueues[slot]
	if !entry.Active || entry.Status == 0 {
		buf := protocol.NewBuffer(12)
		buf.WriteU32(uint32(slot))
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_STATUS), buf.Bytes(), true)
		return
	}

	buf := protocol.NewBuffer(40)
	buf.WriteU32(uint32(slot))
	buf.WriteU8(entry.ArenaType)
	if entry.IsArena {
		buf.WriteU8(0x0E)
	} else {
		buf.WriteU8(0x00)
	}
	buf.WriteU32(entry.BgTypeID)
	buf.WriteU16(0x1F90)
	buf.WriteU8(10) // minLevel
	buf.WriteU8(80) // maxLevel
	buf.WriteU32(entry.InstanceID)
	if entry.IsRated {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(entry.Status) // STATUS_WAIT_QUEUE = 1
	switch entry.Status {
	case 1: // wait queue
		buf.WriteU32(120000) // average wait time ms
		timeInQueue := uint32(time.Since(entry.JoinTime).Milliseconds())
		buf.WriteU32(timeInQueue)
	case 2: // wait join
		buf.WriteU32(entry.MapID)
		buf.WriteU64(0)
		buf.WriteU32(120000) // time to remove
	case 3: // in progress
		buf.WriteU32(entry.MapID)
		buf.WriteU64(0)
		buf.WriteU32(0) // time to auto leave
		elapsed := uint32(0)
		if !entry.StartTime.IsZero() {
			elapsed = uint32(time.Since(entry.StartTime).Milliseconds())
		}
		buf.WriteU32(elapsed)
		buf.WriteU8(entry.ArenaFaction)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_STATUS), buf.Bytes(), true)
}

func (s *session) restoreBattlegroundLoginQueue(state playerState) {
	if s == nil || s.bgData.InstanceID == 0 || !isBattlegroundMap(state.Map) {
		return
	}
	bgTypeID, arenaType, isArena, ok := battlegroundTypeForMap(state.Map)
	if !ok {
		return
	}
	for index := range s.bgQueues {
		if s.bgQueues[index].Active {
			continue
		}
		s.bgQueues[index] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, InstanceID: s.bgData.InstanceID, Status: 3, ArenaType: arenaType, IsArena: isArena, MapID: state.Map, StartTime: time.Now(), ArenaFaction: uint8(s.bgData.Team)}
		return
	}
}

func battlegroundTypeForMap(mapID uint32) (uint32, uint8, bool, bool) {
	switch mapID {
	case 30:
		return 1, 0, false, true
	case 489:
		return 2, 0, false, true
	case 529:
		return 3, 0, false, true
	case 566:
		return 7, 0, false, true
	case 607:
		return 9, 0, false, true
	case 628:
		return 30, 0, false, true
	case 559, 562, 572, 617, 618:
		return 4, uint8(2), true, true
	default:
		return 0, 0, false, false
	}
}

// handleBfEntryInviteResponse processes CMSG_BATTLEFIELD_MGR_ENTRY_INVITE_RESPONSE (0x4DF).
// Reference: WorldSession::HandleBfEntryInviteResponse (BattlefieldHandler.cpp:143).
func (s *session) handleBfEntryInviteResponse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	r := protocol.NewReader(payload)
	battleID, err := r.ReadU32()
	if err != nil {
		return false
	}
	accepted, err := r.ReadU8()
	if err != nil {
		return false
	}

	if accepted != 0 {
		buf := protocol.NewBuffer(9)
		buf.WriteU32(battleID)
		buf.WriteU8(0)  // unk
		buf.WriteU32(1) // clear afk
		_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_ENTERED), buf.Bytes(), true)
	}
	s.debug("battlefield entry invite response", "battle", battleID, "accepted", accepted)
	return true
}

// handleBfQueueInviteResponse processes CMSG_BATTLEFIELD_MGR_QUEUE_INVITE_RESPONSE (0x4E2).
// Reference: WorldSession::HandleBfQueueInviteResponse.
func (s *session) handleBfQueueInviteResponse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	r := protocol.NewReader(payload)
	battleID, _ := r.ReadU32()
	accepted, _ := r.ReadU8()

	s.debug("battlefield queue invite response", "battle", battleID, "accepted", accepted)
	return true
}

// handleBfQueueExitRequest processes CMSG_BATTLEFIELD_MGR_EXIT_REQUEST (0x4E7).
// Reference: WorldSession::HandleBfQueueExitRequest (BattlefieldHandler.cpp:173).
func (s *session) handleBfQueueExitRequest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	battleID, _ := r.ReadU32()

	buf := protocol.NewBuffer(9)
	buf.WriteU32(battleID)
	buf.WriteU8(0) // reason 0 = normal exit
	buf.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_EJECTED), buf.Bytes(), true)
	s.debug("battlefield exit request", "battle", battleID)
	return true
}

// handleLeaveBattlefield processes CMSG_LEAVE_BATTLEFIELD (0x2E1).
// Reference: WorldSession::HandleBattlefieldLeaveOpcode (BattleGroundHandler.cpp:528).
func (s *session) handleLeaveBattlefield(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.server != nil {
		s.server.handleWSGPlayerLeave(s)
		s.server.handleEOTSPlayerLeave(s)
		s.server.handleSAPlayerLeave(s)
		s.server.handleICPlayerLeave(s)
		s.server.handleArenaPlayerLeave(s)
		s.server.handleWGPlayerLeave(s)
	}
	s.resetAchievementCriteriaByCondition(criteriaConditionBGMap, s.player.Map)
	for slot := 0; slot < len(s.bgQueues); slot++ {
		if s.bgQueues[slot].Active {
			s.bgQueues[slot].Active = false
			s.bgQueues[slot].Status = 0 // STATUS_NONE
			s.sendBattlefieldStatus(uint8(slot))
		}
	}
	return true
}

// handleReportPvPAfk processes CMSG_REPORT_PVP_AFK (0x3E4).
// Reference: WorldSession::HandleReportPvPAFK (BattleGroundHandler.cpp:795) and Player::ReportedAfkBy (Player.cpp:22524).
func (s *session) handleReportPvPAfk(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, _ := r.ReadU64()

	if s.server != nil && targetGUID != s.playerGUID {
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess != nil && targetSess.player != nil {
			targetSess.writeMu.Lock()
			if targetSess.afkReporters == nil {
				targetSess.afkReporters = make(map[uint64]struct{})
			}
			targetSess.afkReporters[s.playerGUID] = struct{}{}
			reportCount := len(targetSess.afkReporters)
			targetSess.writeMu.Unlock()

			s.debug("reported player for pvp afk", "account", s.accountName, "target", targetGUID, "reports", reportCount)
			if reportCount >= 3 {
				targetSess.applyAura(43680) // Idle debuff
			}
		}
	}
	return true
}

// handleBattlegroundPlayerPositions processes MSG_BATTLEGROUND_PLAYER_POSITIONS (0x2E9).
// Reference: WorldSession::HandleBattlegroundPlayerPositionsOpcode (BattlegroundHandler.cpp:262).
func (s *session) handleBattlegroundPlayerPositions(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	// Reference BattlegroundHandler.cpp:266-268:
	// Only respond if player is inside a battleground or arena instance/map
	switch s.player.Map {
	case 30, 489, 529, 566, 607, 628, 559, 562, 572, 617, 618:
		var teammates []*session
		if s.server != nil {
			s.server.sessionsMu.RLock()
			for other := range s.server.sessions {
				if other != s && other.playerLoaded && other.player != nil && other.player.Map == s.player.Map && teamForRace(other.player.Race) == teamForRace(s.player.Race) {
					teammates = append(teammates, other)
				}
			}
			s.server.sessionsMu.RUnlock()
		}

		var flagCarriers []*session
		if s.server != nil {
			flagCarriers = append(flagCarriers, s.server.getWSGFlagCarriers(s.player.Map)...)
			flagCarriers = append(flagCarriers, s.server.getEOTSFlagCarriers(s.player.Map)...)
		}

		buf := protocol.NewBuffer(8 + len(teammates)*16 + len(flagCarriers)*16)
		buf.WriteU32(uint32(len(teammates))) // numPlayerPositions
		for _, mate := range teammates {
			buf.WriteU64(mate.playerGUID)
			buf.WriteF32(mate.player.X)
			buf.WriteF32(mate.player.Y)
		}
		buf.WriteU32(uint32(len(flagCarriers))) // flagCarrierCount
		for _, carrier := range flagCarriers {
			buf.WriteU64(carrier.playerGUID)
			buf.WriteF32(carrier.player.X)
			buf.WriteF32(carrier.player.Y)
		}
		_ = s.write(uint16(protocol.OpcodeMSG_BATTLEGROUND_PLAYER_POSITIONS), buf.Bytes(), true)
	}
	return true
}
