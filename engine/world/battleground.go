package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleBattlemasterHello processes CMSG_BATTLEMASTER_HELLO (0x2D7).
// Reference: WorldSession::HandleBattlemasterHelloOpcode (BattleGroundHandler.cpp:41).
func (s *session) handleBattlemasterHello(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// BattleGroundHandler.cpp:41: `recvData >> guid` reads a packed GUID
	// (ObjectGuid::ReadFromPacket); a raw 8-byte read misparsed it — packed
	// payloads with a full mask are 9 bytes and start with the mask byte, so
	// raw reads yielded garbage guids, the interact gate below never passed,
	// and every hello no-op'd (same bug class as the 06:39 ReportPvPAFK fix).
	// Truncated packed payloads throw in C++, so a read error reports nothing.
	r := protocol.NewReader(payload)
	bmGUID, err := r.ReadPackedGUID()
	if err != nil {
		return true
	}

	// WorldSession::HandleBattlemasterHelloOpcode (BattleGroundHandler.cpp:41-45): the
	// creature must be interactable with UNIT_NPC_FLAG_BATTLEMASTER, or the hello is ignored.
	if !s.canInteractWithNPC(ctx, bmGUID, uint64(unitNPCFlagBattlemaster)) {
		return true
	}

	// BattleGroundHandler.cpp:47-48: the battlemaster pauses its movement and
	// re-bases its home position on hello.
	s.pauseCreatureMotionForHello(bmGUID)

	// BattleGroundHandler.cpp:47 — the battleground type is derived server-side from the
	// battlemaster's creature entry (BattlegroundMgr::GetBattleMasterBG: battlemaster_entry.entry
	// → bg_template), never from the client. The previous hardcoded bgTypeId 1 was Go-original.
	bgTypeID := s.battlemasterBGType(ctx, uint32(bmGUID>>24)&0x00FFFFFF)
	if bgTypeID == battlegroundTypeNone {
		// == GetBattleMasterBG returning BATTLEGROUND_TYPE_NONE: the level gate below denies it
		// (GetBattlegroundTemplate null), so the list is never sent.
		return true
	}

	// Player::GetBGAccessByLevel (Player.cpp:23656-23672): a missing battleground_template row
	// (== GetBattlegroundTemplate null) or a level outside [MinLvl, MaxLvl] answers with
	// SendNotification(LANG_YOUR_BG_LEVEL_REQ_ERROR) and no list. trinity_string 715 is not
	// seeded in this repo's world.sql, so the arm returns without the message — the same
	// documented delta class as the arena unit's LANG_ARENA_DISABLED.
	if !s.bgAccessByLevel(ctx, bgTypeID) {
		return true
	}

	return s.sendBattlefieldList(bmGUID, 0, bgTypeID)
}

// battlegroundTypeNone mirrors BATTLEGROUND_TYPE_NONE (SharedDefines.h:3509).
const battlegroundTypeNone = uint32(0)

// Battleground desertion types (Battleground.h:52-56), recorded by
// CHAR_INS_DESERTER_TRACK into battleground_deserters.
const (
	bgDesertionTypeLeaveBG      uint8 = 0 // player leaves the BG
	bgDesertionTypeOffline      uint8 = 1 // player is kicked from BG because offline
	bgDesertionTypeLeaveQueue   uint8 = 2 // player is invited to join and refuses to do it
	bgDesertionTypeNoEnter      uint8 = 3 // player is invited to join and does nothing (time expires)
	bgDesertionTypeInviteLogout uint8 = 4 // player is invited to join and logs out
)

// bgWaitSampleCount mirrors COUNT_OF_PLAYERS_TO_AVERAGE_WAIT_TIME
// (BattlegroundQueue.h:31).
const bgWaitSampleCount = 10

// bgQueueWaitRing is the Go form of BattlegroundQueue's m_WaitTimes ring
// (BattlegroundQueue.h:133): the last bgWaitSampleCount join->invite wait
// times with a running sum. C++ keys [team][bracket]; Go keys by
// battleground type id + team (the bracket id is PvPDifficulty.dbc RangeIndex
// data with no in-tree source, so the bracket dimension is not modeled).
type bgQueueWaitRing struct {
	samples [bgWaitSampleCount]uint32
	next    int
	count   int
	sum     uint64
}

// bgWaitKey is the Go form of the m_WaitTimes [team][bracket] key
// (BattlegroundQueue.h:133) without the bracket dimension: battleground type
// id plus the C++ team_index (TEAM_ALLIANCE = 0, TEAM_HORDE = 1).
type bgWaitKey struct {
	bgTypeID uint32
	team     uint8
}

// bgWaitTeamIndex ports the team_index selection shared by
// PlayerInvitedToBGUpdateAverageWaitTime and GetAverageQueueWaitTime
// (BattlegroundQueue.cpp:244-254, 269-278): TEAM_ALLIANCE (0) by default;
// non-arena queues key by the queued group's team, arenas by the rated flag.
func bgWaitTeamIndex(isArena, isRated bool, team uint32) uint8 {
	if isArena {
		if isRated {
			return 1 // TEAM_HORDE for rated arenas (BattlegroundQueue.cpp:252)
		}
		return 0
	}
	if team == 1 { // TEAM_HORDE
		return 1
	}
	return 0
}

// recordBGQueueWait ports BattlegroundQueue::PlayerInvitedToBGUpdateAverageWaitTime
// (BattlegroundQueue.cpp:242): the new wait overwrites the oldest sample and
// the running sum is kept exact.
func (s *Server) recordBGQueueWait(bgTypeID uint32, team uint8, waitMs uint32) {
	s.bgWaitMu.Lock()
	defer s.bgWaitMu.Unlock()
	if s.bgWaitTimes == nil {
		s.bgWaitTimes = make(map[bgWaitKey]*bgQueueWaitRing)
	}
	key := bgWaitKey{bgTypeID: bgTypeID, team: team}
	ring := s.bgWaitTimes[key]
	if ring == nil {
		ring = &bgQueueWaitRing{}
		s.bgWaitTimes[key] = ring
	}
	ring.sum -= uint64(ring.samples[ring.next])
	ring.samples[ring.next] = waitMs
	ring.sum += uint64(waitMs)
	ring.next = (ring.next + 1) % bgWaitSampleCount
	if ring.count < bgWaitSampleCount {
		ring.count++
	}
}

// averageBGQueueWait ports BattlegroundQueue::GetAverageQueueWaitTime
// (BattlegroundQueue.cpp:266-285): 0 while fewer than bgWaitSampleCount
// samples exist ("not available"), then the ring mean.
func (s *Server) averageBGQueueWait(bgTypeID uint32, team uint8) uint32 {
	s.bgWaitMu.Lock()
	defer s.bgWaitMu.Unlock()
	ring := s.bgWaitTimes[bgWaitKey{bgTypeID: bgTypeID, team: team}]
	if ring == nil || ring.count < bgWaitSampleCount {
		return 0
	}
	return uint32(ring.sum / bgWaitSampleCount)
}

// trackBGDeserter records a battleground desertion row — the Go form of
// CHAR_INS_DESERTER_TRACK ("INSERT INTO battleground_deserters (guid, type,
// datetime) VALUES (?, ?, NOW())", CharacterDatabase.cpp:596). It fires only
// when Battleground.TrackDeserters.Enable is set; the C++ default is false
// (World.cpp:1193), so this stays silent unless the operator opts in.
func (s *session) trackBGDeserter(ctx context.Context, desertionType uint8) {
	if s == nil || s.server == nil || !s.server.Config.BattlegroundTrackDeserters {
		return
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
		"INSERT INTO battleground_deserters (guid, type, datetime) VALUES (?, ?, CURRENT_TIMESTAMP)",
		s.playerGUID, desertionType)
}

// battlemasterBGType mirrors BattlegroundMgr::LoadBattleMastersEntry/GetBattleMasterBG
// (BattlegroundMgr.cpp:872-915, BattlegroundMgr.h:127-133): entry → bg_template from the
// battlemaster_entry table, validated against the BattlemasterList DBC (the loader skips rows
// whose bg_template has no DBC entry). Unknown entries map to BATTLEGROUND_TYPE_NONE.
func (s *session) battlemasterBGType(ctx context.Context, entry uint32) uint32 {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return battlegroundTypeNone
	}
	var bgTypeID uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT bg_template FROM battlemaster_entry WHERE entry = ?`, entry).Scan(&bgTypeID); err != nil {
		return battlegroundTypeNone
	}
	file, fileErr := s.server.Data.File("BattlemasterList")
	if fileErr != nil {
		return battlegroundTypeNone
	}
	if _, found := file.Find(bgTypeID); !found {
		return battlegroundTypeNone
	}
	return bgTypeID
}

// bgAccessByLevel mirrors Player::GetBGAccessByLevel (Player.cpp:23656-23672): the player's
// level is capped at the max player level (DEFAULT_MAX_LEVEL in C++) and must fall inside the
// battleground_template row's [MinLvl, MaxLvl]; a missing row (== GetBattlegroundTemplate null)
// denies access.
func (s *session) bgAccessByLevel(ctx context.Context, bgTypeID uint32) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.player == nil {
		return false
	}
	var minLvl, maxLvl uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT MinLvl, MaxLvl FROM battleground_template WHERE ID = ?`, bgTypeID).Scan(&minLvl, &maxLvl); err != nil {
		return false
	}
	maxLevel := s.server.Config.MaxPlayerLevel
	if maxLevel == 0 {
		maxLevel = 80
	}
	level := uint32(s.player.Level)
	if level > maxLevel {
		level = maxLevel
	}
	return level >= minLvl && level <= maxLvl
}

// handleBattlefieldList processes CMSG_BATTLEFIELD_LIST (0x23C).
// Reference: WorldSession::HandleBattlefieldListOpcode (BattleGroundHandler.cpp:338).
func (s *session) handleBattlefieldList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 6 {
		return true
	}
	r := protocol.NewReader(payload)
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return true
	}
	fromWhere, err := r.ReadU8()
	if err != nil {
		return true
	}
	// BattleGroundHandler.cpp:340-342: canGainXP is read (players with locked
	// XP have their own bg queue on retail) but never used downstream; a short
	// packet throws before the battlemaster lookup, so the read is still
	// error-checked — truncated packets answer nothing.
	if _, err := r.ReadU8(); err != nil {
		return true
	}

	// sBattlemasterListStore.LookupEntry(bgTypeId) (BattleGroundHandler.cpp:347):
	// an invalid bgTypeId is answered with silence.
	if s.server == nil || s.server.Data == nil {
		return true
	}
	file, fileErr := s.server.Data.File("BattlemasterList")
	if fileErr != nil {
		return true
	}
	if _, found := file.Find(bgTypeID); !found {
		return true
	}

	return s.sendBattlefieldList(0, fromWhere, bgTypeID)
}

func (s *session) sendBattlefieldList(bmGUID uint64, fromWhere uint8, bgTypeID uint32) bool {
	// Reference: BattlegroundMgr::BuildBattlegroundListPacket (BattlegroundMgr.cpp:638-659) —
	// the winner/loser reward kills come from the 6 Battleground.Reward* configs
	// (World.cpp:1445-1450: RewardWinnerHonorFirst=30, RewardWinnerHonorLast=15,
	// RewardWinnerArenaFirst=25, RewardWinnerArenaLast=0, RewardLoserHonorFirst=5,
	// RewardLoserHonorLast=5) selected on Player::GetRandomWinner, and the honor
	// rewards scale with the player's level via Trinity::Honor::hk_honor_at_level
	// (Formulas.h:34-44: ceil(level*1.55*kills)). The random-reward repeat block
	// re-writes the same computed values.
	var winHonor, winArena, lossHonor uint32
	if s != nil && s.server != nil && s.player != nil {
		cfg := &s.server.Config
		winnerKills := cfg.BGRewardWinnerHonorFirst
		winnerArena := cfg.BGRewardWinnerArenaFirst
		loserKills := cfg.BGRewardLoserHonorFirst
		if s.randomBGWinner {
			winnerKills = cfg.BGRewardWinnerHonorLast
			winnerArena = cfg.BGRewardWinnerArenaLast
			loserKills = cfg.BGRewardLoserHonorLast
		}
		winHonor = uint32(math.Ceil(float64(s.player.Level) * 1.55 * float64(winnerKills)))
		winArena = winnerArena
		lossHonor = uint32(math.Ceil(float64(s.player.Level) * 1.55 * float64(loserKills)))
	}
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
	buf.WriteU32(winHonor)
	buf.WriteU32(winArena)
	buf.WriteU32(lossHonor)
	isRandom := bgTypeID == battlegroundRB
	if isRandom {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	if isRandom {
		if s.randomBGWinner {
			buf.WriteU8(1)
		} else {
			buf.WriteU8(0)
		}
		buf.WriteU32(winHonor)
		buf.WriteU32(winArena)
		buf.WriteU32(lossHonor)
	}
	buf.WriteU32(0) // count of active instances
	return s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_LIST), buf.Bytes(), true) == nil
}

// handleBattlemasterJoin processes CMSG_BATTLEMASTER_JOIN (0x2EE).
// Reference: WorldSession::HandleBattlemasterJoinOpcode (BattleGroundHandler.cpp:74).
func (s *session) handleBattlemasterJoin(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// BattleGroundHandler.cpp:76: `recvData >> guid` reads a packed GUID; a
	// raw 8-byte read misaligned every field after it (the guid is only
	// logged in C++, but bgTypeId/instanceId/joinAsGroup then ran on the
	// packed bytes), so real-client joins silently failed the BattlemasterList
	// lookup below. Truncated packets throw in C++ → drop with the
	// connection open (WorldSession.cpp:427-432), never close it.
	r := protocol.NewReader(payload)
	if _, err := r.ReadPackedGUID(); err != nil {
		return true
	}
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return true
	}
	instanceID, err := r.ReadU32()
	if err != nil {
		return true
	}
	joinAsGroup, err := r.ReadU8()
	if err != nil {
		return true
	}

	// sBattlemasterListStore.LookupEntry(bgTypeId_) (BattleGroundHandler.cpp:84-88):
	// an invalid bgTypeId is answered with silence (error log on the C++ side).
	if s.server == nil || s.server.Data == nil {
		return true
	}
	file, fileErr := s.server.Data.File("BattlemasterList")
	if fileErr != nil {
		return true
	}
	if _, found := file.Find(bgTypeID); !found {
		s.debug("invalid bgtype in battlemaster join", "account", s.accountName, "bg", bgTypeID)
		return true
	}

	// BattleGroundHandler.cpp:90-94 — DisableMgr::IsDisabledFor(DISABLE_TYPE_BATTLEGROUND, bgTypeId)
	// → PSendSysMessage(LANG_BG_DISABLED) + return. Gate bridged via the arena unit's
	// battlegroundDisabled helper; the message text (trinity_string 747) is not seeded anywhere in
	// this repo (world.sql creates the table with zero rows), so no message is sent on the disabled
	// arm — same documented delta as the arena unit.
	if s.battlegroundDisabled(ctx, bgTypeID) {
		s.debug("battlemaster join rejected: battleground disabled", "account", s.accountName, "bg", bgTypeID)
		return true
	}

	// BattleGroundHandler.cpp:104-105 — _player->InBattleground() → silent return.
	// Player::InBattleground is m_bgData.bgInstanceID != 0 (Player.h:1906), mirrored by
	// s.bgData.InstanceID (spells.go:2000 precedent); the gate covers both the solo and
	// the group arms since it sits before the joinAsGroup split.
	if s.bgData.InstanceID != 0 {
		return true
	}

	// BattleGroundHandler.cpp:121-124 — no PvPDifficulty bracket for the
	// requester's level → silent return (both arms).
	if bgBracketIndex(uint32(s.player.Level)) < 0 {
		s.debug("battlemaster join rejected: no pvp bracket for level", "account", s.accountName, "level", s.player.Level)
		return true
	}

	// The solo-only checks below (deserter, duplicate queue, free slots, freeze) live in the
	// !joinAsGroup arm of HandleBattlemasterJoinOpcode (BattleGroundHandler.cpp:131-193); the
	// group arm re-checks per member through Group::CanJoinBattlegroundQueue (Group.cpp:2024).
	if joinAsGroup != 0 {
		s.handleBattlemasterJoinGroup(bgTypeID, instanceID)
		return true
	}

	// BattleGroundHandler.cpp:126-132 — player is using the dungeon finder →
	// ERR_LFG_CANT_USE_BATTLEGROUND (-13), ahead of the RBAC arm.
	if isLFGUsing(s, s.playerGUID) {
		s.sendJoinedBGResult(groupJoinBattlegroundLFGCantUse)
		return true
	}

	// BattleGroundHandler.cpp:133-139 — !CanJoinToBattleground(bg) → ERR_BATTLEGROUND_JOIN_TIMED_OUT.
	// Player::CanJoinToBattleground (Player.cpp:22503) gates on RBAC_PERM_JOIN_RANDOM_BG for
	// random queues and RBAC_PERM_JOIN_NORMAL_BG otherwise (battlemaster joins are never
	// arenas).
	if bgTypeID == battlegroundRB {
		if !s.canJoinRandomBG {
			s.sendJoinedBGResult(groupJoinBattlegroundTimedOut)
			return true
		}
	} else if !s.canJoinNormalBG {
		s.sendJoinedBGResult(groupJoinBattlegroundTimedOut)
		return true
	}

	// BattleGroundHandler.cpp:131-137 — _player->IsDeserter() → BuildGroupJoinedBattlegroundPacket
	// ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS; the packet is int32(-2) only, slot-for-slot vs
	// BattlegroundMgr::BuildGroupJoinedBattlegroundPacket:239-244 (the u64 arm fires only for
	// ERR_BATTLEGROUND_JOIN_TIMED_OUT/JOIN_FAILED).
	if s.hasAura(deserterSpellBG) {
		buf := protocol.NewBuffer(4)
		buf.WriteI32(groupJoinBattlegroundDeserters)
		_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), buf.Bytes(), true)
		return true
	}

	// BattleGroundHandler.cpp:149-163 — the random-queue exclusion: already in the random
	// queue → ERR_IN_RANDOM_BG (-14); queued elsewhere and asking for random →
	// ERR_IN_NON_RANDOM_BG (-15). Both packets are int32-only (BattlegroundMgr.cpp:239-244).
	if s.inRandomBGQueue() {
		s.sendJoinedBGResult(groupJoinInRandomBG)
		return true
	}
	if bgTypeID == battlegroundRB && s.inNonArenaBGQueue() {
		s.sendJoinedBGResult(groupJoinInNonRandomBG)
		return true
	}

	// Duplicate-queue protection: player is already in this queue (C++ WorldSession::HandleBattlemasterJoinOpcode
	// — GetBattlegroundQueueIndex(bgQueueTypeId) < PLAYER_MAX_BATTLEGROUND_QUEUES → silent return).
	for i := 0; i < len(s.bgQueues); i++ {
		if s.bgQueues[i].Active && !s.bgQueues[i].IsArena && s.bgQueues[i].BgTypeID == bgTypeID {
			return true
		}
	}

	// Check if has free queue slots (BattleGroundHandler.cpp:183-190 — HasFreeBattlegroundQueueId
	// miss → ERR_BATTLEGROUND_TOO_MANY_QUEUES; the packet is int32(-4) only, no u64 arm).
	slot := -1
	for i := 0; i < len(s.bgQueues); i++ {
		if !s.bgQueues[i].Active {
			slot = i
			break
		}
	}
	if slot == -1 {
		buf := protocol.NewBuffer(4)
		buf.WriteI32(groupJoinTooManyQueues)
		_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), buf.Bytes(), true)
		return true
	}

	// Freeze debuff (BattleGroundHandler.cpp:192-193 — HasAura(9454) → silent return).
	if s.hasAura(freezeAuraSpellID) {
		return true
	}

	joinTime := time.Now()
	s.bgQueues[slot] = bgQueueEntry{
		Active:     true,
		BgTypeID:   bgTypeID,
		InstanceID: instanceID,
		JoinTime:   joinTime,
		Status:     BGStatusWaitQueue,
	}

	s.sendBattlefieldStatus(uint8(slot))
	// No live matchmaker: simulate the queue pop (see simulateBGQueueInvite).
	s.simulateBGQueueInvite(slot, joinTime, 0)
	s.debug("queued for battleground", "account", s.accountName, "bg", bgTypeID, "slot", slot)
	return true
}

// groupJoinBattlegroundTimedOut mirrors ERR_BATTLEGROUND_JOIN_TIMED_OUT
// (SharedDefines.h:3701 — "%s was unavailable to join the queue.").
const groupJoinBattlegroundTimedOut = int32(-11)

// groupJoinBattlegroundFailed mirrors ERR_BATTLEGROUND_JOIN_FAILED
// (SharedDefines.h:3702 — "Join as a group failed").
const groupJoinBattlegroundFailed = int32(-12)

// groupJoinBattlegroundLFGCantUse mirrors ERR_LFG_CANT_USE_BATTLEGROUND
// (SharedDefines.h:3703 — "You cannot queue for a battleground or arena while
// using the dungeon system.").
const groupJoinBattlegroundLFGCantUse = int32(-13)

// groupJoinBattlegroundNone mirrors ERR_BATTLEGROUND_NONE (SharedDefines.h:3691 —
// "not show anything"): the over-MaxGroupSize answer.
const groupJoinBattlegroundNone = int32(-1)

// groupJoinBattlegroundRangeIndex mirrors ERR_BATTLEGROUND_JOIN_RANGE_INDEX
// (SharedDefines.h:3700 — "Cannot join the queue unless all members of your
// party are in the same battleground level range.").
const groupJoinBattlegroundRangeIndex = int32(-10)

// bgBracketIndex mirrors the PvPDifficulty.dbc bracket lookup behind
// GetBattlegroundBracketByLevel: WotLK battlegrounds use the standard
// 10-level brackets 10-19 … 70-79 plus 80. A level outside every bracket
// (GetBattlegroundBracketByLevel returns null) yields -1.
func bgBracketIndex(level uint32) int {
	if level < 10 || level > 80 {
		return -1
	}
	return int((level - 1) / 10)
}

// battlemasterMaxGroupSize reads the BattlemasterList.dbc MaxGroupSize field
// (DBCStructure.h:303 — field 28) behind Group::CanJoinBattlegroundQueue's
// memberscount > bgEntry->MaxGroupSize arm (Group.cpp:2035-2037).
func battlemasterMaxGroupSize(srv *Server, bgTypeID uint32) (uint32, bool) {
	if srv == nil || srv.Data == nil {
		return 0, false
	}
	file, err := srv.Data.File("BattlemasterList")
	if err != nil {
		return 0, false
	}
	rec, found := file.Find(bgTypeID)
	if !found {
		return 0, false
	}
	size, err := rec.Uint32(28)
	if err != nil {
		return 0, false
	}
	return size, true
}

// bgSimulatedInviteDelay is the stand-in for the C++ matchmaker
// (BattlegroundQueue::Update → InviteGroupToBG): Go keeps no live
// battleground world or cross-player matchmaking, so a queued entry is
// "popped" after this delay with a synthetic instance id, flipping it to
// STATUS_WAIT_JOIN and sending the invite dialog.
const bgSimulatedInviteDelay = 15 * time.Second

// bgInviteAcceptWindow mirrors INVITE_ACCEPT_WAIT_TIME (Battleground.h:150):
// the invite expires this long after the pop, freeing the queue slot.
const bgInviteAcceptWindow = 60 * time.Second

// bgInvitationRemindTime mirrors INVITATION_REMIND_TIME (Battleground.h:149):
// BGQueueInviteEvent (BattlegroundQueue.cpp:471-476, 1018-1043) re-sends the
// invite dialog this long after the pop if the player still hasn't responded.
const bgInvitationRemindTime = 20 * time.Second

// simulateBGQueueInvite arms the queue-pop simulation for one queued slot:
// after bgSimulatedInviteDelay the entry flips to STATUS_WAIT_JOIN with the
// given instance id (0 → a fresh synthetic id is minted at fire time) and
// the client gets the invite via SMSG_BATTLEFIELD_STATUS. The join-time
// guard drops the invite if the slot was left or reused meanwhile.
func (s *session) simulateBGQueueInvite(slot int, joinTime time.Time, instanceID uint32) {
	if s == nil || slot < 0 || slot >= len(s.bgQueues) {
		return
	}
	entry := &s.bgQueues[slot]
	if !entry.Active || entry.IsArena || entry.Status != BGStatusWaitQueue {
		return
	}
	bgTypeID := entry.BgTypeID
	time.AfterFunc(bgSimulatedInviteDelay, func() {
		if s == nil || !s.playerLoaded || s.player == nil {
			return
		}
		if slot < 0 || slot >= len(s.bgQueues) {
			return
		}
		e := &s.bgQueues[slot]
		// Slot left or reused: the join time no longer matches.
		if !e.Active || e.IsArena || e.Status != BGStatusWaitQueue || e.BgTypeID != bgTypeID || !e.JoinTime.Equal(joinTime) {
			return
		}
		if instanceID == 0 {
			instanceID = uint32(time.Now().Unix())
		}
		// The C++ pop's IsInvitedToBGInstanceGUID: a fresh instance id (no
		// live BG world backs it — the port itself stays no-bridge,
		// battleground.go handleBattlefieldPort).
		e.InstanceID = instanceID
		e.MapID = battlegroundMapForType(bgTypeID)
		e.Status = BGStatusWaitJoin
		s.sendBattlefieldStatus(uint8(slot))
		s.debug("simulated battleground invite", "account", s.accountName, "bg", bgTypeID, "slot", slot, "instance", instanceID)
		// BGQueueInviteEvent::Execute (BattlegroundQueue.cpp:1018-1043):
		// INVITATION_REMIND_TIME after the pop, if the player is still
		// invited and hasn't responded, the invite dialog goes out again.
		// The slot/instance/join-time guard is the Go form of
		// IsPlayerInvited's instance+remove-time match: it drops the re-send
		// if the slot was left, reused, or the invite already expired.
		// (The re-sent packet carries the full accept window, not C++'s
		// INVITE_ACCEPT_WAIT_TIME - INVITATION_REMIND_TIME: threading a
		// remaining-time would need pop-time state Go doesn't keep.)
		remindID := instanceID
		time.AfterFunc(bgInvitationRemindTime, func() {
			if s == nil || !s.playerLoaded || s.player == nil {
				return
			}
			if slot < 0 || slot >= len(s.bgQueues) {
				return
			}
			rem := &s.bgQueues[slot]
			if !rem.Active || rem.IsArena || rem.Status != BGStatusWaitJoin || rem.BgTypeID != bgTypeID || !rem.JoinTime.Equal(joinTime) || rem.InstanceID != remindID {
				return
			}
			s.sendBattlefieldStatus(uint8(slot))
			s.debug("simulated battleground invite re-sent", "account", s.accountName, "bg", bgTypeID, "slot", slot)
		})
		// BGQueueRemoveEvent::Execute (BattlegroundQueue.cpp:1060-1103): the
		// invite expires bgInviteAcceptWindow after the pop — the slot is
		// freed, a STATUS_NONE packet goes out, and the no-enter deserter
		// type is tracked. The slot/instance/join-time guard is the Go form
		// of IsPlayerInvited's remove-time match.
		inviteID := instanceID
		time.AfterFunc(bgInviteAcceptWindow, func() {
			if s == nil || !s.playerLoaded || s.player == nil {
				return
			}
			if slot < 0 || slot >= len(s.bgQueues) {
				return
			}
			exp := &s.bgQueues[slot]
			if !exp.Active || exp.IsArena || exp.Status != BGStatusWaitJoin || exp.BgTypeID != bgTypeID || !exp.JoinTime.Equal(joinTime) || exp.InstanceID != inviteID {
				return
			}
			s.bgQueues[slot] = bgQueueEntry{}
			s.sendBattlefieldStatus(uint8(slot))
			s.trackBGDeserter(context.Background(), bgDesertionTypeNoEnter)
			s.debug("simulated battleground invite expired", "account", s.accountName, "bg", bgTypeID, "slot", slot)
		})
	})
}

// simulateArenaQueueInvite arms the arena queue-pop simulation for one queued
// arena slot, mirroring simulateBGQueueInvite for the arena legs of the C++
// matchmaker: the per-session sim stands in for the opposing-team matchmaking
// the C++ Update() performs (rated: BattlegroundQueue.cpp:955-1010; skirmish
// similarly needs a second team). At pop time it mirrors
// sBattlegroundMgr->CreateNewBattleground (BattlegroundQueue.cpp:983) by
// getting/creating the arena state, then invites the player
// (BattlegroundQueue::InviteGroupToBG, BattlegroundQueue.cpp:432 — the
// single-player sim takes no side; the arena team is the wire value the
// port-accept arm computes). The remind and expire timers mirror
// BGQueueInviteEvent::Execute (BattlegroundQueue.cpp:1018-1043) and
// BGQueueRemoveEvent::Execute (BattlegroundQueue.cpp:1060-1103); the expire
// arm skips the no-enter deserter tracking — the C++ arm gates it on
// bg->isBattleground(), which excludes arenas.
func (s *session) simulateArenaQueueInvite(slot int, joinTime time.Time, instanceID uint32) {
	if s == nil || slot < 0 || slot >= len(s.bgQueues) {
		return
	}
	entry := &s.bgQueues[slot]
	if !entry.Active || !entry.IsArena || entry.Status != BGStatusWaitQueue {
		return
	}
	time.AfterFunc(bgSimulatedInviteDelay, func() {
		if s == nil || !s.playerLoaded || s.player == nil {
			return
		}
		if slot < 0 || slot >= len(s.bgQueues) {
			return
		}
		e := &s.bgQueues[slot]
		// Slot left or reused: the join time no longer matches.
		if !e.Active || !e.IsArena || e.Status != BGStatusWaitQueue || e.BgTypeID != battlegroundAA || !e.JoinTime.Equal(joinTime) {
			return
		}
		if instanceID == 0 {
			instanceID = uint32(time.Now().Unix())
		}
		// The C++ pop's IsInvitedToBGInstanceGUID: a fresh instance id (the
		// arena state created here is the Go form of the C++ pop's new
		// Battleground object).
		e.InstanceID = instanceID
		e.MapID = battlegroundMapForType(battlegroundAA)
		e.Status = BGStatusWaitJoin
		s.server.getOrCreateArenaState(e.MapID, e.InstanceID, e.ArenaType, e.IsRated)
		s.sendBattlefieldStatus(uint8(slot))
		s.debug("simulated arena invite", "account", s.accountName, "slot", slot, "type", e.ArenaType, "rated", e.IsRated, "instance", instanceID)
		// BGQueueInviteEvent::Execute (BattlegroundQueue.cpp:1018-1043):
		// INVITATION_REMIND_TIME after the pop, if the player is still
		// invited and hasn't responded, the invite dialog goes out again.
		remindID := instanceID
		time.AfterFunc(bgInvitationRemindTime, func() {
			if s == nil || !s.playerLoaded || s.player == nil {
				return
			}
			if slot < 0 || slot >= len(s.bgQueues) {
				return
			}
			rem := &s.bgQueues[slot]
			if !rem.Active || !rem.IsArena || rem.Status != BGStatusWaitJoin || rem.BgTypeID != battlegroundAA || !rem.JoinTime.Equal(joinTime) || rem.InstanceID != remindID {
				return
			}
			s.sendBattlefieldStatus(uint8(slot))
			s.debug("simulated arena invite re-sent", "account", s.accountName, "slot", slot)
		})
		// BGQueueRemoveEvent::Execute (BattlegroundQueue.cpp:1060-1103): the
		// invite expires INVITE_ACCEPT_WAIT_TIME after the pop — the slot is
		// freed and a STATUS_NONE packet goes out. No no-enter deserter
		// tracking: the C++ arm gates it on bg->isBattleground(), which
		// excludes arenas.
		inviteID := instanceID
		time.AfterFunc(bgInviteAcceptWindow, func() {
			if s == nil || !s.playerLoaded || s.player == nil {
				return
			}
			if slot < 0 || slot >= len(s.bgQueues) {
				return
			}
			exp := &s.bgQueues[slot]
			if !exp.Active || !exp.IsArena || exp.Status != BGStatusWaitJoin || exp.BgTypeID != battlegroundAA || !exp.JoinTime.Equal(joinTime) || exp.InstanceID != inviteID {
				return
			}
			// BGQueueRemoveEvent::Execute (BattlegroundQueue.cpp:1090) calls RemovePlayer on
			// invite expiry, and RemovePlayer fires the rated-exit world announce when the group
			// empties (BattlegroundQueue.cpp:357-362). Solo joins empty their group here; group
			// last-leaver detection needs the shared queue container (leave-arm convention).
			announceExit := !exp.ArenaGroupJoin && exp.IsRated && exp.ArenaType != 0 && exp.ArenaTeamID != 0 && s.server.Config.ArenaQueueAnnouncerEnable
			teamID, teamRating, arenaType := exp.ArenaTeamID, exp.ArenaTeamRating, exp.ArenaType
			s.bgQueues[slot] = bgQueueEntry{}
			s.sendBattlefieldStatus(uint8(slot))
			if announceExit {
				if name, _, ok := arenaTeamAnnounceInfo(context.Background(), s, teamID); ok {
					s.server.broadcastMessageChatAll(fmt.Sprintf(arenaQueueAnnounceExitText, name, arenaType, arenaType, teamRating))
				}
			}
			s.debug("simulated arena invite expired", "account", s.accountName, "slot", slot)
		})
	})
}

// handleBattlemasterJoinGroup processes the joinAsGroup arm of CMSG_BATTLEMASTER_JOIN.
// Reference: WorldSession::HandleBattlemasterJoinOpcode, group branch
// (BattleGroundHandler.cpp:212-258) and Group::CanJoinBattlegroundQueue (Group.cpp:2024).
func (s *session) handleBattlemasterJoinGroup(bgTypeID, instanceID uint32) {
	// grp = _player->GetGroup(); no group or non-leader join: silent return == C++
	// (BattleGroundHandler.cpp:213-217).
	if s.groupID == 0 {
		return
	}
	grp := s.server.findGroupByID(s.groupID)
	if grp == nil || grp.LeaderGUID != s.playerGUID {
		return
	}

	// Group::CanJoinBattlegroundQueue (Group.cpp:2027): LFG group → ERR_LFG_CANT_USE_BATTLEGROUND.
	if grp.IsLFG {
		s.sendGroupJoinBGResult(grp.Members, groupJoinBattlegroundLFGCantUse)
		return
	}

	// Group::CanJoinBattlegroundQueue (Group.cpp:2035-2037): more members
	// than the BattlemasterList MaxGroupSize → ERR_BATTLEGROUND_NONE
	// ("not show anything"), broadcast to every member.
	if maxSize, ok := battlemasterMaxGroupSize(s.server, bgTypeID); ok && uint32(len(grp.Members)) > maxSize {
		s.sendGroupJoinBGResult(grp.Members, groupJoinBattlegroundNone)
		return
	}

	// Member sessions in group order (leader first), mirroring the C++ GroupReference walk.
	members := make([]*session, 0, len(grp.Members))
	for _, m := range grp.Members {
		members = append(members, s.server.findSessionByGUID(m.GUID))
	}

	// CanJoinBattlegroundQueue per-member checks in C++ order (Group.cpp:2049-2087); the first
	// failing member decides err, which is then broadcast to every member.
	err := int32(bgTypeID) // success: positive values are indexes in BattlemasterList.dbc (SharedDefines.h:3689)
	leaderTeam := teamForRace(s.player.Race)
	leaderBracket := bgBracketIndex(uint32(s.player.Level))
	for _, member := range members {
		switch {
		case member == nil || !member.playerLoaded || member.player == nil:
			// offline member → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2049-2051)
			err = groupJoinBattlegroundFailed
		case (bgTypeID == battlegroundRB && !member.canJoinRandomBG) || (bgTypeID != battlegroundRB && !member.canJoinNormalBG):
			// rbac permissions → ERR_BATTLEGROUND_JOIN_TIMED_OUT (Group.cpp:2052-2054;
			// Player::CanJoinToBattleground, Player.cpp:22503)
			err = groupJoinBattlegroundTimedOut
		case teamForRace(member.player.Race) != leaderTeam:
			// cross-faction → ERR_BATTLEGROUND_JOIN_TIMED_OUT (Group.cpp:2056-2058)
			err = groupJoinBattlegroundTimedOut
		case bgBracketIndex(uint32(member.player.Level)) != leaderBracket:
			// not in the same battleground level bracket →
			// ERR_BATTLEGROUND_JOIN_RANGE_INDEX (Group.cpp:2059-2062)
			err = groupJoinBattlegroundRangeIndex
		case memberBGQueueIndex(member, bgTypeID) != -1:
			// member already in this queue → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2067-2068)
			err = groupJoinBattlegroundFailed
		case bgTypeID != battlegroundAA && member.inRandomBGQueue():
			// member in the random queue → ERR_IN_RANDOM_BG (Group.cpp:2080; skipped for
			// arena queues — GetTypeID() == BATTLEGROUND_AA)
			err = groupJoinInRandomBG
		case bgTypeID == battlegroundRB && member.inNonArenaBGQueue():
			// joining random while the member is queued elsewhere → ERR_IN_NON_RANDOM_BG
			// (Group.cpp:2082-2083)
			err = groupJoinInNonRandomBG
		case bgTypeID != battlegroundAA && member.hasAura(deserterSpellBG):
			// deserter → ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS (Group.cpp:2086; skipped
			// for arena queues — GetTypeID() == BATTLEGROUND_AA)
			err = groupJoinBattlegroundDeserters
		case memberFreeBGQueueIndex(member) == -1:
			// no free slot → ERR_BATTLEGROUND_TOO_MANY_QUEUES (Group.cpp:2079-2080)
			err = groupJoinTooManyQueues
		case isLFGUsing(s, member.playerGUID):
			// member is using the dungeon finder → ERR_LFG_CANT_USE_BATTLEGROUND
			// (Group.cpp:2084-2085)
			err = groupJoinBattlegroundLFGCantUse
		case member.hasAura(freezeAuraSpellID):
			// freeze → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2085-2086)
			err = groupJoinBattlegroundFailed
		}
		if err <= 0 {
			break
		}
	}

	if err <= 0 {
		// err <= 0 → BuildGroupJoinedBattlegroundPacket(err) to every member
		// (BattleGroundHandler.cpp:234-242).
		s.sendGroupJoinBGResult(grp.Members, err)
		return
	}

	// err > 0: queue every member — AddBattlegroundQueueId slot assignment +
	// BuildBattlegroundStatusPacket(STATUS_WAIT_QUEUE) + BuildGroupJoinedBattlegroundPacket(err)
	// to each member (BattleGroundHandler.cpp:243-258). avgTime comes from
	// GetAverageQueueWaitTime; Go keeps no queue wait stats, so sendBattlefieldStatus answers
	// the hardcoded average — same documented delta as the solo arm.
	// One shared synthetic instance id for the simulated pop, like the
	// C++ GroupQueueInfo's single IsInvitedToBGInstanceGUID.
	simInstanceID := uint32(time.Now().Unix())
	for _, member := range members {
		slot := memberFreeBGQueueIndex(member)
		if slot == -1 {
			continue
		}
		joinTime := time.Now()
		member.bgQueues[slot] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, InstanceID: instanceID, JoinTime: joinTime, Status: BGStatusWaitQueue}
		member.sendBattlefieldStatus(uint8(slot))
		// No live matchmaker: simulate the queue pop (see simulateBGQueueInvite).
		member.simulateBGQueueInvite(slot, joinTime, simInstanceID)
	}
	s.sendGroupJoinBGResult(grp.Members, err)
	s.debug("group queued for battleground", "account", s.accountName, "bg", bgTypeID, "members", len(members))
}

// sendGroupJoinBGResult mirrors BattlegroundMgr::BuildGroupJoinedBattlegroundPacket
// (BattlegroundMgr.cpp:239): SMSG_GROUP_JOINED_BATTLEGROUND carries int32(result); the u64
// arm fires only for ERR_BATTLEGROUND_JOIN_TIMED_OUT/JOIN_FAILED. The packet goes to every
// online group member, mirroring the C++ per-member SendDirectMessage fan-out.
func (s *session) sendGroupJoinBGResult(members []groupMember, result int32) {
	buf := protocol.NewBuffer(12)
	buf.WriteI32(result)
	if result == groupJoinBattlegroundTimedOut || result == groupJoinBattlegroundFailed {
		buf.WriteU64(0) // player guid — C++ writes a zero GUID here (BattlegroundMgr.cpp:244)
	}
	pkt := buf.Bytes()
	for _, m := range members {
		if member := s.server.findSessionByGUID(m.GUID); member != nil {
			_ = member.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), pkt, true)
		}
	}
}

// sendJoinedBGResult mirrors BattlegroundMgr::BuildGroupJoinedBattlegroundPacket
// (BattlegroundMgr.cpp:239-244) for a single session: SMSG_GROUP_JOINED_BATTLEGROUND
// carries int32(result), with the u64(0) arm only for ERR_BATTLEGROUND_JOIN_TIMED_OUT
// (-11) and ERR_BATTLEGROUND_JOIN_FAILED (-12).
func (s *session) sendJoinedBGResult(result int32) {
	buf := protocol.NewBuffer(12)
	buf.WriteI32(result)
	if result == groupJoinBattlegroundTimedOut || result == groupJoinBattlegroundFailed {
		buf.WriteU64(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), buf.Bytes(), true)
}

// inRandomBGQueue mirrors GetBattlegroundQueueIndex(BGQueueTypeId(BATTLEGROUND_RB, 0)) <
// PLAYER_MAX_BATTLEGROUND_QUEUES (BattleGroundHandler.cpp:149-156, Group.cpp:2081):
// the player holds an active random-battleground queue slot.
func (s *session) inRandomBGQueue() bool {
	for i := 0; i < len(s.bgQueues); i++ {
		if s.bgQueues[i].Active && !s.bgQueues[i].IsArena && s.bgQueues[i].BgTypeID == battlegroundRB {
			return true
		}
	}
	return false
}

// inNonArenaBGQueue mirrors Player::InBattlegroundQueue(true) (Player.h:1912 — the true
// arm ignores arenas): the player holds any active non-arena battleground queue slot.
func (s *session) inNonArenaBGQueue() bool {
	for i := 0; i < len(s.bgQueues); i++ {
		if s.bgQueues[i].Active && !s.bgQueues[i].IsArena {
			return true
		}
	}
	return false
}

// memberBGQueueIndex mirrors the duplicate-queue arm of CanJoinBattlegroundQueue
// (Group.cpp:2067): the index of the member's active non-arena queue for bgTypeID, -1 if none.
func memberBGQueueIndex(member *session, bgTypeID uint32) int {
	for i := 0; i < len(member.bgQueues); i++ {
		if member.bgQueues[i].Active && !member.bgQueues[i].IsArena && member.bgQueues[i].BgTypeID == bgTypeID {
			return i
		}
	}
	return -1
}

// memberFreeBGQueueIndex mirrors Player::HasFreeBattlegroundQueueId: the first inactive
// queue slot of the member, -1 when the member is queued for the maximum.
func memberFreeBGQueueIndex(member *session) int {
	for i := 0; i < len(member.bgQueues); i++ {
		if !member.bgQueues[i].Active {
			return i
		}
	}
	return -1
}

// battlegroundAA mirrors BATTLEGROUND_AA (SharedDefines.h:3515 — BattlemasterList.dbc index 6, All Arenas).
const battlegroundAA = uint32(6)

// battlegroundRB mirrors BATTLEGROUND_RB (SharedDefines.h:3522 — BattlemasterList.dbc index 32,
// Random Battleground): Player::CanJoinToBattleground keys the RBAC permission on it
// (Player.cpp:22503) and the join handlers exclude random/non-random queue mixing on it.
const battlegroundRB = uint32(32)

// groupJoinInRandomBG mirrors ERR_IN_RANDOM_BG (SharedDefines.h:3704 — "Can't do that while
// in a Random Battleground queue.").
const groupJoinInRandomBG = int32(-14)

// groupJoinInNonRandomBG mirrors ERR_IN_NON_RANDOM_BG (SharedDefines.h:3705 — "Can't queue for
// Random Battleground while in another Battleground queue.").
const groupJoinInNonRandomBG = int32(-15)

// battlegroundDisabled mirrors DisableMgr::IsDisabledFor(DISABLE_TYPE_BATTLEGROUND, entry)
// (DisableMgr.cpp:401-405): for battlegrounds, mere presence of the row in the disables table
// disables it. Same shape as questDisabled (commands_quest.go:78).
func (s *session) battlegroundDisabled(ctx context.Context, entry uint32) bool {
	db := s.server.WorldStore.DB
	if db == nil {
		return false
	}
	var one int
	return db.QueryRowContext(ctx, "SELECT 1 FROM disables WHERE sourceType = ? AND entry = ?", disableTypeBattleground, entry).Scan(&one) == nil
}

// arenaQueueAnnounceJoinText / arenaQueueAnnounceExitText are the enUS
// formats of LANG_ARENA_QUEUE_ANNOUNCE_WORLD_JOIN (718) and
// LANG_ARENA_QUEUE_ANNOUNCE_WORLD_EXIT (719), recalled from TDB — no
// trinity_string seed exists in-tree, so verify the prose against TDB when one
// is seeded. The argument shape is C++-exact: team name, arenaType, arenaType,
// arenaTeamRating (BattlegroundQueue.cpp:166-170, 360-362).
const (
	arenaQueueAnnounceJoinText = "Arena team %s has joined the %dv%d queue with rating %d."
	arenaQueueAnnounceExitText = "Arena team %s has left the %dv%d queue with rating %d."
)

// arenaTeamAnnounceInfo resolves the arena-team name and join-time rating for
// the queue announce arms (BattlegroundQueue.cpp:166-170, 360-362): a
// GetArenaTeamById miss → not ok, silent like C++. The rating clamps to 1,
// mirroring BattleGroundHandler.cpp:721-722 (arenaRating <= 0 → 1).
func arenaTeamAnnounceInfo(ctx context.Context, s *session, teamID uint32) (string, uint32, bool) {
	if s == nil || s.server == nil || teamID == 0 {
		return "", 0, false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return "", 0, false
	}
	var name string
	var rating uint32
	if err := cdb.QueryRowContext(ctx, "SELECT name, rating FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&name, &rating); err != nil {
		return "", 0, false
	}
	if rating == 0 {
		rating = 1
	}
	return name, rating, true
}

// handleBattlemasterJoinArena processes CMSG_BATTLEMASTER_JOIN_ARENA (0x358).
// Reference: WorldSession::HandleBattlemasterJoinArena (BattleGroundHandler.cpp:610).
func (s *session) handleBattlemasterJoinArena(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// BattleGroundHandler.cpp:615: `recvData >> guid` reads a packed GUID; a
	// raw 8-byte read misparsed it, so the GetNPCIfCanInteractWith gate below
	// never passed and every arena join no-op'd (same bug class as the
	// battlemaster-hello fix). Truncated packets throw in C++ → drop.
	r := protocol.NewReader(payload)
	guid, err := r.ReadPackedGUID()
	if err != nil {
		return true
	}
	arenaSlot, err := r.ReadU8()
	if err != nil {
		return true
	}
	asGroup, err := r.ReadU8()
	if err != nil {
		return true
	}
	isRated, err := r.ReadU8()
	if err != nil {
		return true
	}

	// BattleGroundHandler.cpp:618-619 — ignore if already in a battleground
	// (the "or BG queue" half of the comment is the queue checks below; the code
	// gates only InBattleground). Player::InBattleground is m_bgData.bgInstanceID != 0
	// (Player.h:1906), mirrored by s.bgData.InstanceID (spells.go:2000 precedent).
	if s.bgData.InstanceID != 0 {
		return true
	}

	// BattleGroundHandler.cpp:621-623 — GetNPCIfCanInteractWith(guid,
	// UNIT_NPC_FLAG_BATTLEMASTER) miss → silent return.
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagBattlemaster)) {
		return true
	}

	arenaType := uint8(2)
	switch arenaSlot {
	case 0:
		arenaType = 2
	case 1:
		arenaType = 3
	case 2:
		arenaType = 5
	default:
		// BattleGroundHandler.cpp:646-647 — unknown arena slot: error log + silent return.
		s.debug("unknown arena slot in join arena", "account", s.accountName, "slot", arenaSlot)
		return true
	}

	// BattleGroundHandler.cpp:657-661 — DisableMgr::IsDisabledFor(DISABLE_TYPE_BATTLEGROUND, BATTLEGROUND_AA)
	// → PSendSysMessage(LANG_ARENA_DISABLED) + return. The gate bridges the rejection; the message text itself
	// (trinity_string 748) is not seeded anywhere in this repo (world.sql creates the table with zero rows),
	// so no message is sent on the disabled arm — documented delta.
	if s.battlegroundDisabled(ctx, battlegroundAA) {
		s.debug("arena join rejected: battleground disabled", "account", s.accountName)
		return true
	}

	// BattleGroundHandler.cpp:663-665 — no PvPDifficulty bracket for the level →
	// silent return (both arms).
	if bgBracketIndex(uint32(s.player.Level)) < 0 {
		s.debug("arena join rejected: no pvp bracket for level", "account", s.accountName, "level", s.player.Level)
		return true
	}

	// BattleGroundHandler.cpp:699-709 — the asGroup arm computes err through
	// Group::CanJoinBattlegroundQueue instead of the solo checks below.
	if asGroup != 0 {
		return s.handleBattlemasterJoinArenaGroup(ctx, arenaType, arenaSlot, isRated != 0)
	}

	// BattleGroundHandler.cpp:672-678 — player is using the dungeon finder →
	// ERR_LFG_CANT_USE_BATTLEGROUND (-13), ahead of the RBAC arm.
	if isLFGUsing(s, s.playerGUID) {
		s.sendJoinedBGResult(groupJoinBattlegroundLFGCantUse)
		return true
	}

	// BattleGroundHandler.cpp:680-686 — !CanJoinToBattleground(bg) → ERR_BATTLEGROUND_JOIN_FAILED.
	// The AA template isArena(), so Player::CanJoinToBattleground (Player.cpp:22503) gates on
	// RBAC_PERM_JOIN_ARENAS.
	if !s.canJoinArenas {
		s.sendJoinedBGResult(groupJoinBattlegroundFailed)
		return true
	}

	// Duplicate-queue protection: player is already in this arena queue (C++ WorldSession::HandleBattlemasterJoinArena
	// — GetBattlegroundQueueIndex(bgQueueTypeId) < PLAYER_MAX_BATTLEGROUND_QUEUES → silent return).
	for i := 0; i < len(s.bgQueues); i++ {
		if s.bgQueues[i].Active && s.bgQueues[i].IsArena && s.bgQueues[i].ArenaType == arenaType {
			return true
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

	var ratedTeamID uint32
	if isRated != 0 {
		// BattleGroundHandler.cpp:714-722 — a rated queue requires a real arena team:
		// GetArenaTeamId(arenaslot) + sArenaTeamMgr->GetArenaTeamById null →
		// SendNotInArenaTeamPacket(arenatype) + return. The single JOIN covers both the
		// missing-membership and the deleted-team cases (same query shape as
		// commands.go:3902, the sCharacterCache->GetCharacterArenaTeamIdByGuid mirror).
		if cdb := s.server.CharactersStore.DB; cdb != nil {
			err := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", s.playerGUID, arenaType).Scan(&ratedTeamID)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return true
				}
				// WorldSession::SendNotInArenaTeamPacket (ArenaTeamHandler.cpp:414):
				// SMSG_ARENA_ERROR (0x376), u32(0) + u8(type) — "You are not in a %uv%u arena team".
				buf := protocol.NewBuffer(5)
				buf.WriteU32(0)
				buf.WriteU8(arenaType)
				_ = s.write(uint16(protocol.OpcodeSMSG_ARENA_ERROR), buf.Bytes(), true)
				s.debug("rated arena join rejected: no arena team", "account", s.accountName, "type", arenaType)
				return true
			}
			s.debug("rated arena join", "account", s.accountName, "type", arenaType, "team", ratedTeamID)
		}
	}

	// BattlegroundQueue::AddGroup's arena-join world announce (cpp:166-170)
	// reads ginfo->ArenaTeamRating; resolve the team's name and join-time
	// rating here (BattleGroundHandler.cpp:711-723 builds the solo ginfo from
	// the joiner's team) so the queue entry carries both.
	var ratedTeamName string
	var ratedTeamRating uint32
	if isRated != 0 && ratedTeamID != 0 {
		if name, rating, ok := arenaTeamAnnounceInfo(ctx, s, ratedTeamID); ok {
			ratedTeamName, ratedTeamRating = name, rating
		}
	}

	joinTime := time.Now()
	s.bgQueues[slot] = bgQueueEntry{
		Active:          true,
		BgTypeID:        battlegroundAA, // BATTLEGROUND_AA (All Arenas)
		InstanceID:      0,
		JoinTime:        joinTime,
		Status:          BGStatusWaitQueue,
		ArenaType:       arenaType,
		IsArena:         true,
		IsRated:         isRated != 0,
		ArenaFaction:    0,
		ArenaTeamID:     ratedTeamID,
		ArenaTeamRating: ratedTeamRating,
	}

	s.sendBattlefieldStatus(uint8(slot))
	// BattlegroundQueue::AddGroup (cpp:166-170): the arena-join world announce
	// fires when the group is added — gated on Arena.QueueAnnouncer.Enable
	// (C++ default false), broadcast via broadcastMessageChatAll (the
	// sWorld->SendWorldText analog).
	if ratedTeamName != "" && s.server.Config.ArenaQueueAnnouncerEnable {
		s.server.broadcastMessageChatAll(fmt.Sprintf(arenaQueueAnnounceJoinText, ratedTeamName, arenaType, arenaType, ratedTeamRating))
	}
	// No live matchmaker: simulate the arena queue pop (see simulateArenaQueueInvite).
	s.simulateArenaQueueInvite(slot, joinTime, 0)
	s.debug("queued for arena", "account", s.accountName, "slot", slot, "type", arenaType, "rated", isRated != 0)
	return true
}

// arenaTeamPartySize mirrors ERR_ARENA_TEAM_PARTY_SIZE
// (SharedDefines.h:3693 — "Incorrect party size for this arena.").
const arenaTeamPartySize = int32(-3)

// isLFGUsing bridges Player::isUsingLfg (GetState(guid) != LFG_STATE_NONE).
// Go's LFGManager tracks the queued map only, so the queued state is covered.
func isLFGUsing(s *session, guid uint64) bool {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return false
	}
	_, using := s.server.Features.LFG.Status(guid)
	return using
}

// handleBattlemasterJoinArenaGroup processes the asGroup arm of CMSG_BATTLEMASTER_JOIN_ARENA.
// Reference: WorldSession::HandleBattlemasterJoinArena, group branch
// (BattleGroundHandler.cpp:700-788) with Group::CanJoinBattlegroundQueue (Group.cpp:2024)
// specialized for the arena variant (MinPlayerCount == MaxPlayerCount == arenatype, isRated,
// arenaSlot — so RB/non-random/random queue arms are skipped for BATTLEGROUND_AA and the
// deserter arm is skipped: Group.cpp:2084 only checks deserters for non-arena queues).
func (s *session) handleBattlemasterJoinArenaGroup(ctx context.Context, arenaType uint8, arenaSlot uint8, isRated bool) bool {
	// grp = _player->GetGroup(); no group or non-leader join: silent return == C++
	// (BattleGroundHandler.cpp:701-705).
	if s.groupID == 0 {
		return true
	}
	grp := s.server.findGroupByID(s.groupID)
	if grp == nil || grp.LeaderGUID != s.playerGUID {
		return true
	}

	// Group::CanJoinBattlegroundQueue (Group.cpp:2027): LFG group → ERR_LFG_CANT_USE_BATTLEGROUND.
	// The leader's arena team id (BattleGroundHandler.cpp:711 — the requester's
	// GetArenaTeamId(arenaslot)) is hoisted to function scope: the queue-entry
	// stamping and the AddGroup world announce below need it past the err check.
	var leaderTeamID uint32
	err := int32(battlegroundAA) // success: positive values are indexes in BattlemasterList.dbc (SharedDefines.h:3689)
	if grp.IsLFG {
		err = groupJoinBattlegroundLFGCantUse
	} else {
		// Member sessions in group order (leader first), mirroring the C++ GroupReference walk.
		members := make([]*session, 0, len(grp.Members))
		for _, m := range grp.Members {
			members = append(members, s.server.findSessionByGUID(m.GUID))
		}

		// The rated team-id arm (Group.cpp:2071-2073) compares each member against the
		// reference player's team; the reference is the group leader here.
		leaderBracket := bgBracketIndex(uint32(s.player.Level))
		if isRated {
			if cdb := s.server.CharactersStore.DB; cdb != nil {
				dbErr := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", s.playerGUID, arenaType).Scan(&leaderTeamID)
				if dbErr != nil && !errors.Is(dbErr, sql.ErrNoRows) {
					return true
				}
			}
		}

		// CanJoinBattlegroundQueue per-member checks in C++ order (Group.cpp:2049-2087),
		// arena-variant subset; the first failing member decides err.
		leaderTeam := teamForRace(s.player.Race)
		for _, member := range members {
			switch {
			case member == nil || !member.playerLoaded || member.player == nil:
				// offline member → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2049-2051)
				err = groupJoinBattlegroundFailed
			case !member.canJoinArenas:
				// rbac permissions → ERR_BATTLEGROUND_JOIN_TIMED_OUT (Group.cpp:2052-2054;
				// Player::CanJoinToBattleground on the arena template keys
				// RBAC_PERM_JOIN_ARENAS, Player.cpp:22503)
				err = groupJoinBattlegroundTimedOut
			case teamForRace(member.player.Race) != leaderTeam:
				// cross-faction → ERR_BATTLEGROUND_JOIN_TIMED_OUT (Group.cpp:2056-2058);
				// the RBAC CanJoinToBattleground arm between them has no Go RBAC model —
				// the battlemaster group-arm documented delta.
				err = groupJoinBattlegroundTimedOut
			case bgBracketIndex(uint32(member.player.Level)) != leaderBracket:
				// member not in the leader's level bracket → ERR_BATTLEGROUND_JOIN_RANGE_INDEX
				// (Group.cpp:2059-2062)
				err = groupJoinBattlegroundRangeIndex
			case isRated && s.arenaTeamIDOfMember(ctx, member, arenaType) != leaderTeamID:
				// rated arena team id mismatch → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2071-2073)
				err = groupJoinBattlegroundFailed
			case memberArenaQueueIndex(member, arenaType) != -1:
				// member already in this arena queue → ERR_BATTLEGROUND_JOIN_FAILED
				// (Group.cpp:2066-2068 — InBattlegroundQueueForBattlegroundQueueType)
				err = groupJoinBattlegroundFailed
			case memberFreeBGQueueIndex(member) == -1:
				// no free slot → ERR_BATTLEGROUND_TOO_MANY_QUEUES (Group.cpp:2079-2080)
				err = groupJoinTooManyQueues
			case isLFGUsing(s, member.playerGUID):
				// member is using the dungeon finder → ERR_LFG_CANT_USE_BATTLEGROUND
				// (Group.cpp:2081-2083)
				err = groupJoinBattlegroundLFGCantUse
			case member.hasAura(freezeAuraSpellID):
				// freeze → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2085-2086)
				err = groupJoinBattlegroundFailed
			}
			if err <= 0 {
				break
			}
		}

		// Arena party-size gate (Group.cpp:2089-2091): MinPlayerCount == arenatype.
		if err > 0 && uint8(len(grp.Members)) != arenaType {
			err = arenaTeamPartySize
		}
	}

	if isRated {
		// BattleGroundHandler.cpp:714-732 — a rated queue requires a real arena team for
		// the leader even when err <= 0: GetArenaTeamId(arenaslot) + GetArenaTeamById null →
		// SendNotInArenaTeamPacket(arenatype) + return. arenaRating clamps to 1,
		// matchmakerRating/previousOpponents feed bgQueue.AddGroup and ScheduleQueueUpdate,
		// which have no Go queue model — documented delta.
		if cdb := s.server.CharactersStore.DB; cdb != nil {
			var teamID uint32
			dbErr := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", s.playerGUID, arenaType).Scan(&teamID)
			if dbErr != nil {
				if !errors.Is(dbErr, sql.ErrNoRows) {
					return true
				}
				buf := protocol.NewBuffer(5)
				buf.WriteU32(0)
				buf.WriteU8(arenaType)
				_ = s.write(uint16(protocol.OpcodeSMSG_ARENA_ERROR), buf.Bytes(), true)
				s.debug("rated arena group join rejected: no arena team", "account", s.accountName, "type", arenaType)
				return true
			}
		}
	}

	if err <= 0 {
		// err <= 0 → BuildGroupJoinedBattlegroundPacket(err) to every online member
		// (BattleGroundHandler.cpp:737-745).
		s.sendGroupJoinBGResult(grp.Members, err)
		return true
	}

	// err > 0: per-member slot assignment + BuildBattlegroundStatusPacket(STATUS_WAIT_QUEUE)
	// + BuildGroupJoinedBattlegroundPacket(err) (BattleGroundHandler.cpp:747-773). avgTime
	// comes from GetAverageQueueWaitTime; Go keeps no queue wait stats, so
	// sendBattlefieldStatus answers the hardcoded average — same documented delta as the
	// battlemaster group arm. bg->SetRated(isRated) has no Go template model; the rating
	// rides on the queue entry, the solo-arm convention.
	// One shared synthetic instance id for the simulated pop, like the
	// C++ GroupQueueInfo's single IsInvitedToBGInstanceGUID — the group
	// lands in the same arena instance.
	// BattlegroundQueue::AddGroup's arena-join world announce (cpp:166-170)
	// reads ginfo->ArenaTeamRating; the single group ginfo is built from the
	// leader's team (BattleGroundHandler.cpp:711-723), so resolve the name and
	// join-time rating once here and stamp every member entry with it.
	var groupTeamName string
	var groupTeamRating uint32
	if isRated && leaderTeamID != 0 {
		if name, rating, ok := arenaTeamAnnounceInfo(ctx, s, leaderTeamID); ok {
			groupTeamName, groupTeamRating = name, rating
		}
	}
	simInstanceID := uint32(time.Now().Unix())
	for _, m := range grp.Members {
		member := s.server.findSessionByGUID(m.GUID)
		if member == nil || !member.playerLoaded || member.player == nil {
			continue
		}
		slot := memberFreeBGQueueIndex(member)
		if slot == -1 {
			continue
		}
		joinTime := time.Now()
		member.bgQueues[slot] = bgQueueEntry{
			Active:          true,
			BgTypeID:        battlegroundAA, // BATTLEGROUND_AA (All Arenas)
			InstanceID:      0,
			JoinTime:        joinTime,
			Status:          BGStatusWaitQueue,
			ArenaType:       arenaType,
			IsArena:         true,
			IsRated:         isRated,
			ArenaFaction:    0,
			ArenaTeamID:     leaderTeamID,
			ArenaTeamRating: groupTeamRating,
			ArenaGroupJoin:  true,
		}
		member.sendBattlefieldStatus(uint8(slot))
		// No live matchmaker: simulate the arena queue pop (see simulateArenaQueueInvite).
		member.simulateArenaQueueInvite(slot, joinTime, simInstanceID)
	}
	s.sendGroupJoinBGResult(grp.Members, err)
	// BattlegroundQueue::AddGroup (cpp:166-170): the arena-join world announce
	// fires once per group (the single ginfo), gated on
	// Arena.QueueAnnouncer.Enable (C++ default false).
	if groupTeamName != "" && s.server.Config.ArenaQueueAnnouncerEnable {
		s.server.broadcastMessageChatAll(fmt.Sprintf(arenaQueueAnnounceJoinText, groupTeamName, arenaType, arenaType, groupTeamRating))
	}
	s.debug("group queued for arena", "account", s.accountName, "type", arenaType, "rated", isRated, "members", len(grp.Members))
	return true
}

// arenaTeamIDOfMember mirrors the rated team-id arm of Group::CanJoinBattlegroundQueue
// (Group.cpp:2071-2073): the member's arena team id for the slot's type, 0 when the
// member is in no team of that type or the DB read fails.
func (s *session) arenaTeamIDOfMember(ctx context.Context, member *session, arenaType uint8) uint32 {
	if member == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var teamID uint32
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", member.playerGUID, arenaType).Scan(&teamID); err != nil {
		return 0
	}
	return teamID
}

// memberArenaQueueIndex mirrors the duplicate-queue arm of CanJoinBattlegroundQueue
// (Group.cpp:2066 — InBattlegroundQueueForBattlegroundQueueType) for the arena variant:
// the index of the member's active arena queue for arenaType, -1 if none.
func memberArenaQueueIndex(member *session, arenaType uint8) int {
	for i := 0; i < len(member.bgQueues); i++ {
		if member.bgQueues[i].Active && member.bgQueues[i].IsArena && member.bgQueues[i].ArenaType == arenaType {
			return i
		}
	}
	return -1
}

// groupJoinBattlegroundDeserters mirrors ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS
// (SharedDefines.h:3692 — "You cannot join the battleground yet because you or
// one of your party members is flagged as a Deserter.").
const groupJoinBattlegroundDeserters = int32(-2)

// groupJoinTooManyQueues mirrors ERR_BATTLEGROUND_TOO_MANY_QUEUES
// (SharedDefines.h:3694 — "You can only be queued for 2 battles at once").
const groupJoinTooManyQueues = int32(-4)

// handleBattlefieldPort processes CMSG_BATTLEFIELD_PORT (0x2D5).
// Reference: WorldSession::HandleBattleFieldPortOpcode (BattleGroundHandler.cpp:357).
func (s *session) handleBattlefieldPort(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	// BattleGroundHandler.cpp:359: type is the arena type for arena queues;
	// it selects the queue via BGQueueTypeId(bgTypeId, type)
	// (BattleGroundHandler.cpp:382) — QUEUE_2v2/3v3/5v5 for arenas.
	arenaType, _ := r.ReadU8()
	_, _ = r.ReadU8() // unk2
	// len(payload) >= 9 is guarded above, so this read cannot fail; a
	// short read drops the packet with the connection open
	// (WorldSession.cpp:432), never closes it.
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return true
	}
	_, _ = r.ReadU16() // unk
	action, _ := r.ReadU8()

	// sBattlemasterListStore.LookupEntry(bgTypeId_) (BattleGroundHandler.cpp:368-372):
	// an invalid bgTypeId is answered with silence.
	if s.server == nil || s.server.Data == nil {
		return true
	}
	if file, fileErr := s.server.Data.File("BattlemasterList"); fileErr == nil {
		if _, found := file.Find(bgTypeID); !found {
			return true
		}
	}

	// _player->InBattlegroundQueue() (BattleGroundHandler.cpp:374-379) — no active
	// slot for this battleground is answered with silence. The found slot doubles as
	// the GroupQueueInfo the C++ leg reads (BattleGroundHandler.cpp:383-387).
	// BattlegroundMgr::BGQueueTypeId(bgTypeId, type) (BattlegroundMgr.cpp:737):
	// arena queues are keyed per arena type, so an arena slot only matches when
	// its ArenaType equals the packet's type byte — a player queued for 2v2 and
	// 3v3 at once accepts/leaves the right queue. Non-arena queues ignore type,
	// == C++ (only the BATTLEGROUND_AA branch switches on arenaType).
	slot := -1
	for i := 0; i < len(s.bgQueues); i++ {
		e := &s.bgQueues[i]
		if !e.Active || e.BgTypeID != bgTypeID {
			continue
		}
		if e.IsArena && e.ArenaType != arenaType {
			continue
		}
		slot = i
		break
	}
	if slot == -1 {
		return true
	}
	entry := &s.bgQueues[slot]

	// !ginfo.IsInvitedToBGInstanceGUID && action == 1 (BattleGroundHandler.cpp:396-401):
	// a port accept with no invite instance is answered with silence. This is also the
	// Go form of the "cheating?" IsInvitedForBattlegroundQueueType gate
	// (BattleGroundHandler.cpp:453-454).
	if action != 0 && entry.InstanceID == 0 {
		return true
	}

	// BattleGroundHandler.cpp:421-423 — no PvPDifficulty bracket for the
	// player's level → silent return.
	if bgBracketIndex(uint32(s.player.Level)) < 0 {
		return true
	}

	// Deserter demotion (BattleGroundHandler.cpp:429-439 — action==1 &&
	// ginfo.ArenaType==0 && _player->IsDeserter() == HasAura(26013) (Player.h:1913)
	// → BuildGroupJoinedBattlegroundPacket ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS
	// and the accept demotes to leave; the shared leave arm below then clears the
	// slot, exactly as the C++ else branch does).
	if action == 1 && !entry.IsArena && s.hasAura(deserterSpellBG) {
		buf := protocol.NewBuffer(4)
		buf.WriteI32(groupJoinBattlegroundDeserters)
		_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), buf.Bytes(), true)
		s.debug("battlefield port accept demoted to leave: deserter debuff", "account", s.accountName, "bg", bgTypeID)
		action = 0
	}

	// Level gate (BattleGroundHandler.cpp:441-448): a player who leveled past the
	// battleground's max level while queued does not port — the accept demotes to
	// leave. MaxLvl comes from battleground_template (the Go mirror of the BG
	// template's GetMaxLevel, same table bgAccessByLevel reads); a missing row
	// leaves the demote unevaluated.
	if action == 1 && !entry.IsArena && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var maxLvl uint32
		if qerr := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT MaxLvl FROM battleground_template WHERE ID = ?`, bgTypeID).Scan(&maxLvl); qerr == nil && maxLvl != 0 && uint32(s.player.Level) > maxLvl {
			s.debug("battlefield port accept demoted to leave: level above bg max", "account", s.accountName, "bg", bgTypeID)
			action = 0
		}
	}

	if action != 0 {
		// Freeze debuff (BattleGroundHandler.cpp:450-451 — HasAura(9454) → silent return).
		if s.hasAura(freezeAuraSpellID) {
			return true
		}

		// !_player->InBattleground() → SetBattlegroundEntryPoint
		// (BattleGroundHandler.cpp:456-457); InBattleground is
		// m_bgData.bgInstanceID != 0 (Player.h:1906), so a player porting
		// from one battleground into another keeps the original entry
		// point. setBattlegroundEntryPoint (battleground_entry.go) is the
		// faithful SetBattlegroundEntryPoint port: taxi-path store,
		// dungeon → graveyard, mount-spell capture.
		if s.bgData.InstanceID == 0 {
			s.setBattlegroundEntryPoint()
		}

		// !_player->IsAlive() → ResurrectPlayer(1.0f) + SpawnCorpseBones()
		// (BattleGroundHandler.cpp:461-465).
		if s.isDeadOrGhost() {
			s.resurrectPlayer(ctx, 1.0)
			s.spawnCorpseBones(ctx)
		}

		// Stop taxi flight at port (BattleGroundHandler.cpp:465-466).
		s.finishTaxiFlight()

		// BattlegroundQueue::PlayerInvitedToBGUpdateAverageWaitTime
		// (BattlegroundQueue.cpp:242): the port accept is Go's invite point (Go
		// models no STATUS_WAIT_JOIN state), so the join->port wait feeds the
		// average-wait ring read back by sendBattlefieldStatus. The team index
		// follows the shared selection (BattlegroundQueue.cpp:244-254):
		// non-arena keys by the queued player's team, arenas by the rated flag.
		if entry.Status == BGStatusWaitQueue && !entry.JoinTime.IsZero() {
			s.server.recordBGQueueWait(entry.BgTypeID, bgWaitTeamIndex(entry.IsArena, entry.IsRated, teamForRace(s.player.Race)), uint32(time.Since(entry.JoinTime).Milliseconds()))
		}

		// STATUS_IN_PROGRESS leg (BattleGroundHandler.cpp:468-482): the slot data
		// C++ keeps across the port — SetBattlegroundId (instance) and SetBGTeam
		// (ginfo.Team) — then the BuildBattlegroundStatusPacket. The
		// RemovePlayerAtLeave leg and the SendToBattleground teleport
		// (BattleGroundHandler.cpp:472-486) have no Go world model: there is no
		// BattlegroundMgr / live Battleground instance to port the player into, so
		// the port itself is documented no-bridge rather than stubbed.
		team := teamForRace(s.player.Race)
		entry.Status = BGStatusInProgress
		entry.StartTime = time.Now()
		entry.MapID = battlegroundMapForType(bgTypeID)
		// C++ SetBGTeam(team) stores arenaFaction = team == ALLIANCE ? 1 : 0
		// (Player.cpp:22458-22461); teamForRace is 0 alliance / 1 horde, so the
		// wire byte is inverted here.
		entry.ArenaFaction = 1 - uint8(team)
		s.bgData.InstanceID = entry.InstanceID
		s.bgData.Team = uint16(team)
		// Battleground::AddPlayer (Battleground.cpp:980-1029) ends with
		// AddOrSetPlayerToCorrectBgGroup (Battleground.cpp:1028): the
		// entrant joins their team's BG raid (created on first join).
		// PlayerAddedToBGCheckIfBGIsRunning has no Go model (no live BG
		// status machine), so only the group arm is wired here.
		s.server.addOrSetPlayerToCorrectBGGroup(s)
		s.sendBattlefieldStatus(uint8(slot))
		s.debug("battlefield port accept", "account", s.accountName, "bg", bgTypeID, "instance", entry.InstanceID)
		// Arena::AddPlayer (Arena.cpp:74): on the port accept, admit the
		// player into the arena state the pop created. entry.ArenaFaction
		// carries the C++ SetBGTeam wire value (1 alliance / 0 horde ==
		// ArenaTeamGold / ArenaTeamGreen), which addPlayerToArena takes as
		// the arena team. The state lookup re-finds the pop-created state
		// (getOrCreateArenaState is idempotent).
		if entry.IsArena {
			arena := s.server.getOrCreateArenaState(entry.MapID, entry.InstanceID, entry.ArenaType, entry.IsRated)
			s.server.addPlayerToArena(s, entry.ArenaFaction, arena)
		}
	} else {
		// Arena leave gate (BattleGroundHandler.cpp:493-494): leaving an arena past
		// STATUS_WAIT_QUEUE is answered with silence.
		if entry.IsArena && entry.Status > BGStatusWaitQueue {
			return true
		}
		// BattleGroundHandler.cpp:496-507: the rated-arena loss arm never fires
		// in C++ — it resolves the arena team via
		// GetArenaTeamById(ginfo.Team), but GroupQueueInfo::Team is the
		// faction side (BattlegroundQueue.h:43, set from leader->GetTeam()
		// at BattlegroundQueue.cpp:146), not the arena-team id (that is
		// GroupQueueInfo::ArenaTeamId, BattlegroundQueue.h:47), so the
		// lookup misses and MemberLost is never called. No rating loss is
		// applied here, C++-faithful.
		// BattleGroundHandler.cpp:515-521: track a player who refuses the BG
		// invite by leaving the queue. Battleground::isBattleground() excludes
		// arenas, and entry.InstanceID != 0 is Go's "invited to a live
		// instance" (the C++ bg reference falls back to the template when
		// uninvited, whose STATUS_NONE fails the status gate). The live
		// instance's own IN_PROGRESS/WAIT_JOIN read has no Go model; an
		// invite implies one of those states.
		if !entry.IsArena && entry.InstanceID != 0 {
			s.trackBGDeserter(ctx, bgDesertionTypeLeaveQueue)
		}
		// BattlegroundQueue::RemovePlayer (BattlegroundQueue.cpp:389-406):
		// leaving a rated arena queue removes the whole group — recursive,
		// uninvited only. Each removed member's slot is cleared and a
		// STATUS_NONE packet goes out (the BuildBattlegroundStatusPacket
		// STATUS_NONE arm); entry.InstanceID == 0 is the Go form of
		// !IsInvitedToBGInstanceGUID. Invited groups keep the rest queued,
		// and solo joins have no group to cascade.
		groupEmptied := !entry.ArenaGroupJoin
		if entry.IsArena && entry.IsRated && entry.ArenaGroupJoin && entry.InstanceID == 0 {
			if grp := s.server.findGroupByID(s.groupID); grp != nil {
				groupEmptied = true
				for _, m := range grp.Members {
					if m.GUID == s.playerGUID {
						continue
					}
					member := s.server.findSessionByGUID(m.GUID)
					if member == nil || !member.playerLoaded || member.player == nil {
						continue
					}
					for i := range member.bgQueues {
						e := &member.bgQueues[i]
						if e.Active && e.IsArena && e.IsRated && e.ArenaGroupJoin && e.InstanceID == 0 && e.Status == BGStatusWaitQueue && e.ArenaType == entry.ArenaType && e.ArenaTeamID == entry.ArenaTeamID {
							member.bgQueues[i] = bgQueueEntry{}
							member.sendBattlefieldStatus(uint8(i))
						}
					}
				}
			}
		}
		// BattlegroundQueue::RemovePlayer (BattlegroundQueue.cpp:360-362): the
		// rated-exit world announce — it fires when the last player leaves a
		// rated arena queue (group->Players.empty() after the erase). Solo
		// joins always empty their ginfo; group joins empty it via the
		// recursive removal above (uninvited only — invited groups keep the
		// rest queued, so no announce). Gated on Arena.QueueAnnouncer.Enable
		// (C++ default false); the GetArenaTeamById miss → silent guard rides
		// in arenaTeamAnnounceInfo.
		if entry.IsArena && entry.IsRated && entry.ArenaType != 0 && entry.ArenaTeamID != 0 && groupEmptied && s.server.Config.ArenaQueueAnnouncerEnable {
			if name, _, ok := arenaTeamAnnounceInfo(ctx, s, entry.ArenaTeamID); ok {
				s.server.broadcastMessageChatAll(fmt.Sprintf(arenaQueueAnnounceExitText, name, entry.ArenaType, entry.ArenaType, entry.ArenaTeamRating))
			}
		}
		// Leave queue
		s.bgQueues[slot] = bgQueueEntry{}
		s.sendBattlefieldStatus(uint8(slot))
	}

	return true
}

// handleBattlefieldStatus processes CMSG_BATTLEFIELD_STATUS (0x2D3).
// Reference: WorldSession::HandleBattlefieldStatusOpcode (BattleGroundHandler.cpp:546).
func (s *session) handleBattlefieldStatus(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	for slot := uint8(0); slot < uint8(len(s.bgQueues)); slot++ {
		// C++ sends no packet for slots with no queue type
		// (BattleGroundHandler.cpp:554-556).
		if !s.bgQueues[slot].Active {
			continue
		}
		s.sendBattlefieldStatus(slot)
	}
	return true
}

func (s *session) sendBattlefieldStatus(slot uint8) {
	if int(slot) >= len(s.bgQueues) {
		return
	}
	entry := s.bgQueues[slot]
	if !entry.Active || entry.Status == BGStatusNone {
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
	// C++ reads the BG's level range per packet (BattlegroundMgr.cpp:210-211):
	// the template range for queued packets; live instances carry the bracket
	// range instead (Battleground.cpp:1792), which has no Go model (no
	// PvPDifficulty data), so the template range stands for all statuses.
	minLvl, maxLvl := bgTemplateLevels(s.server, entry.BgTypeID)
	buf.WriteU8(minLvl)
	buf.WriteU8(maxLvl)
	buf.WriteU32(entry.InstanceID)
	if entry.Status == BGStatusWaitQueue {
		// The queued packet is built from the BG template
		// (BattleGroundHandler.cpp:580-588), and templates are never rated —
		// SetRated runs only for live instances
		// (BattlegroundMgr::CreateNewBattleground) — so the wire byte is 0.
		buf.WriteU8(0)
	} else if entry.IsRated {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(entry.Status) // STATUS_WAIT_QUEUE = 1
	switch entry.Status {
	case BGStatusWaitQueue:
		// BattlegroundQueue::GetAverageQueueWaitTime (BattlegroundQueue.cpp:266):
		// 0 until bgWaitSampleCount invites have been seen, then the ring mean.
		// The previous hardcoded 120000 was Go-original. The team index follows
		// the shared selection (BattlegroundQueue.cpp:269-278): non-arena keys
		// by the queued player's team, arenas by the rated flag.
		buf.WriteU32(s.server.averageBGQueueWait(entry.BgTypeID, bgWaitTeamIndex(entry.IsArena, entry.IsRated, teamForRace(s.player.Race))))
		timeInQueue := uint32(time.Since(entry.JoinTime).Milliseconds())
		buf.WriteU32(timeInQueue)
	case BGStatusWaitJoin:
		buf.WriteU32(entry.MapID)
		buf.WriteU64(0)
		// time to remove: C++ sends getMSTimeDiff(now, ginfo.RemoveInviteTime)
		// (BattleGroundHandler.cpp:585), with RemoveInviteTime = invite +
		// INVITE_ACCEPT_WAIT_TIME (Battleground.h:150). The simulated
		// invite fires this status immediately, so the full accept window
		// stands.
		buf.WriteU32(uint32(bgInviteAcceptWindow.Milliseconds())) // time to remove
	case BGStatusInProgress:
		buf.WriteU32(entry.MapID)
		buf.WriteU64(0)
		buf.WriteU32(0) // time to auto leave
		elapsed := uint32(0)
		if !entry.StartTime.IsZero() {
			elapsed = uint32(time.Since(entry.StartTime).Milliseconds())
		}
		buf.WriteU32(elapsed)
		// C++: arenaFaction == ALLIANCE ? 1 : 0 (BattlegroundMgr.cpp:225 ==
		// Player::SetBGTeam, Player.cpp:22458-22461). All ArenaFaction writers
		// store this wire value (1 alliance, 0 horde).
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
		// bgData.Team is the teamForRace id (0 alliance / 1 horde); the status
		// packet's arena-faction byte is the C++ wire value (1 alliance /
		// 0 horde, Player::SetBGTeam).
		s.bgQueues[index] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, InstanceID: s.bgData.InstanceID, Status: BGStatusInProgress, ArenaType: arenaType, IsArena: isArena, MapID: state.Map, StartTime: time.Now(), ArenaFaction: 1 - uint8(s.bgData.Team)}
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

// battlegroundMapForType inverts battlegroundTypeForMap: the map a battleground
// instance runs on, == Battleground::GetMapId for the accept status packet
// (BattleGroundHandler.cpp:468). Arena type 4 covers five maps; 559 (Nagrand
// Arena) stands in since Go has no live instance to read the map from.
func battlegroundMapForType(bgTypeID uint32) uint32 {
	switch bgTypeID {
	case 1:
		return 30
	case 2:
		return 489
	case 3:
		return 529
	case 7:
		return 566
	case 9:
		return 607
	case 30:
		return 628
	case 4:
		return 559
	default:
		return 0
	}
}

// bgTemplateLevels returns the battleground_template MinLvl/MaxLvl pair ==
// Battleground::GetMinLevel/GetMaxLevel for the status packet
// (BattlegroundMgr.cpp:210-211). The table is unseeded in this repo's
// world.sql, so the 10/80 fallback applies (bgMaxLevelForMap convention).
func bgTemplateLevels(srv *Server, bgTypeID uint32) (uint8, uint8) {
	if srv == nil || srv.WorldStore == nil || srv.WorldStore.DB == nil {
		return 10, 80
	}
	var minLvl, maxLvl uint32
	if err := srv.WorldStore.DB.QueryRowContext(context.Background(), `SELECT MinLvl, MaxLvl FROM battleground_template WHERE ID = ?`, bgTypeID).Scan(&minLvl, &maxLvl); err != nil || maxLvl == 0 {
		return 10, 80
	}
	return uint8(minLvl), uint8(maxLvl)
}

// bgMaxLevelForMap returns the BG's m_LevelMax == Battleground::GetMaxLevel,
// read from battleground_template (MaxLvl) keyed by battlegroundTypeForMap's
// type ID. The table is unseeded in this repo's world.sql, so the 80 fallback
// == DEFAULT_MAX_LEVEL applies (same convention as bgAccessByLevel's
// MaxPlayerLevel cap).
func bgMaxLevelForMap(s *Server, mapID uint32) uint32 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 80
	}
	bgTypeID, _, _, ok := battlegroundTypeForMap(mapID)
	if !ok {
		return 80
	}
	var minLvl, maxLvl uint32
	if err := s.WorldStore.DB.QueryRowContext(context.Background(), `SELECT MinLvl, MaxLvl FROM battleground_template WHERE ID = ?`, bgTypeID).Scan(&minLvl, &maxLvl); err != nil || maxLvl == 0 {
		return 80
	}
	return maxLvl
}

// rewardBGEndHonor mirrors Battleground::RewardHonorToTeam (Battleground.cpp:633)
// fed by GetBonusHonorFromKill (Battleground.cpp:806): ceil(min(maxLevel,80)*1.55*kills)
// honor to every worldReady session on the BG map on the given team (0 Alliance,
// 1 Horde). C++ iterates the live instance's m_Players; Go's ambient BG model
// treats all worldReady sessions on the map as participants, the same proxy
// creditBattlegroundWin uses (achievements.go:1455). The per-session call is
// rewardHonorPoints == Player::RewardHonor(nullptr, 1, honor), the same tail
// Battleground::UpdatePlayerScore reaches for SCORE_BONUS_HONOR (Battleground.cpp:1231).
func (s *Server) rewardBGEndHonor(mapID, team uint32, kills uint32) {
	if s == nil || kills == 0 {
		return
	}
	honor := uint32(math.Ceil(float64(min(bgMaxLevelForMap(s, mapID), 80)) * 1.55 * float64(kills)))
	if honor == 0 {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID && teamForRace(sess.player.Race) == team {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		sess.rewardHonorPoints(context.Background(), honor)
	}
}

// rewardBGEndReputation mirrors Battleground::RewardReputationToTeam's standing arm
// (Battleground.cpp:640): rep standing to every worldReady session on the BG map
// on the given team (0 Alliance, 1 Horde). C++ adds the SPELL_AURA_MOD_REPUTATION_GAIN
// and SPELL_AURA_MOD_FACTION_REPUTATION_GAIN aura modifiers first; Go applies the
// same fold inside giveReputation (reputationGainPctMod).
func (s *Server) rewardBGEndReputation(mapID, team uint32, factionID uint32, rep uint32) {
	if s == nil || factionID == 0 || rep == 0 {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID && teamForRace(sess.player.Race) == team {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		sess.giveReputation(context.Background(), factionID, int32(rep))
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
		return true
	}
	accepted, err := r.ReadU8()
	if err != nil {
		return true
	}

	if accepted != 0 {
		// Battlefield::PlayerAcceptInviteToWar (Battlefield.cpp:390): the
		// IsWarTime() gate, the m_PlayersInWar insert
		// (AddOrSetPlayerToCorrectBfGroup), SendBfEntered, and the AFK clear.
		// The invited-set erase has no Go model (no invite tracking); the
		// OnPlayerJoinWar script hook is engine-only.
		if s.server != nil && battleID == WGBattleID {
			wg := s.server.getOrCreateWGState()
			wg.mu.Lock()
			if wg.Enabled && wg.IsActive {
				afk := s.player.PlayerFlags&playerFlagAFK != 0
				wg.PlayersInWar[s.playerGUID] = teamForRace(s.player.Race)
				wg.mu.Unlock()
				// SendBfEntered (BattlefieldHandler.cpp:100): u32 battleId,
				// u8(1), u8(1), u8(isAFK) — 7 bytes; the AFK byte is read
				// before the ToggleAFK clear below.
				buf := protocol.NewBuffer(7)
				buf.WriteU32(battleID)
				buf.WriteU8(1)
				buf.WriteU8(1)
				if afk {
					buf.WriteU8(1)
				} else {
					buf.WriteU8(0)
				}
				_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_ENTERED), buf.Bytes(), true)
				if afk {
					s.setPlayerAFK(false)
				}
			} else {
				wg.mu.Unlock()
			}
		}
	} else if s.server != nil && battleID == WGBattleID && s.player.Zone == WGZoneID {
		// HandleBfEntryInviteResponse decline arm (BattlefieldHandler.cpp:151):
		// a decliner still inside the zone is kicked from the battlefield.
		// KickPosition (BattlefieldWG.cpp:435): 5728.117, 2714.346, 697.733 on
		// the WG map.
		s.teleportTo(WGMapID, 5728.117, 2714.346, 697.733, 0)
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

	if accepted != 0 && s.server != nil && battleID == WGBattleID {
		// Battlefield::PlayerAcceptInviteToQueue (Battlefield.cpp:366): insert
		// the player into the queue, then answer with SendBfQueueInviteResponse
		// (BattlefieldHandler.cpp:73) — u32 battleId, u32 zoneId, u8(1
		// accepted), u8(1 not-full), u8(1 warmup) = 11 bytes.
		wg := s.server.getOrCreateWGState()
		wg.mu.Lock()
		if wg.PlayersInQueue == nil {
			wg.PlayersInQueue = make(map[uint64]uint32)
		}
		wg.PlayersInQueue[s.playerGUID] = teamForRace(s.player.Race)
		wg.mu.Unlock()
		buf := protocol.NewBuffer(11)
		buf.WriteU32(battleID)
		buf.WriteU32(WGZoneID)
		buf.WriteU8(1)
		buf.WriteU8(1)
		buf.WriteU8(1)
		_ = s.write(uint16(protocol.OpcodeSMSG_BATTLEFIELD_MGR_QUEUE_REQUEST_RESPONSE), buf.Bytes(), true)
	}

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

	// Battlefield::AskToLeaveQueue (Battlefield.cpp:374): erase from the
	// queue. C++ sends no packet here — the old fabricated
	// SMSG_BATTLEFIELD_MGR_EJECTED reply (also a wrong 9-byte shape; C++'s
	// SendBfLeaveMessage is 7 bytes) is dropped.
	if s.server != nil && battleID == WGBattleID {
		wg := s.server.getOrCreateWGState()
		wg.mu.Lock()
		delete(wg.PlayersInQueue, s.playerGUID)
		wg.mu.Unlock()
	}
	s.debug("battlefield exit request", "battle", battleID)
	return true
}

// handleLeaveBattlefield processes CMSG_LEAVE_BATTLEFIELD (0x2E1).
// Reference: WorldSession::HandleBattlefieldLeaveOpcode (BattleGroundHandler.cpp:528).
// fromLogout marks the completeLogoutWithPacket call: C++ LogoutPlayer never calls
// Player::LeaveBattleground (offline removal is the separate untracked OFFLINE site,
// Battleground.cpp:271), so the leave-BG tracking below must not fire on logout.
// activeBattlegroundMap is the Go analog of
// Player::GetBattleground()->GetMapId(): the battleground the player is
// currently in, per the active queue slot. The port leg itself is no-bridge
// (no live Battleground instance; the player's own map never becomes the
// BG's), so the BG's map lives on the slot — battlegroundMapForType at
// accept, the restored map at login.
func (s *session) activeBattlegroundMap() uint32 {
	if s == nil {
		return 0
	}
	for i := range s.bgQueues {
		e := &s.bgQueues[i]
		if e.Active && e.InstanceID != 0 && e.MapID != 0 {
			return e.MapID
		}
	}
	return 0
}

func (s *session) handleLeaveBattlefield(ctx context.Context, payload []byte, fromLogout bool, skipEntryTeleport bool) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// BattleGroundHandler.cpp:531-534: the opcode handler skips 8 bytes
	// (u8 unk1, u8 unk2, u32 bgTypeId, u16 unk3) before the combat gate; a
	// short packet throws and nothing happens. Internal callers pass nil and
	// skip this arm.
	if payload != nil && len(payload) < 8 {
		return true
	}
	// C++ WorldSession::HandleBattlefieldLeaveOpcode (BattleGroundHandler.cpp:528)
	// denies leaving a battleground while in combat unless the BG is already in
	// STATUS_WAIT_LEAVE, and it does so BEFORE _player->LeaveBattleground() — so a
	// combat-deny casts no deserter and tracks nothing (Player.cpp:22479-22499 live
	// inside LeaveBattleground). The check therefore runs ahead of the deserter
	// arms below. Go's live-BG analog is the arena world model
	// (battleground_arena.go): a registered participant of a live arena whose
	// status is not ArenaStatusWaitLeave cannot leave while in combat
	// (Unit::IsInCombat == session.isInCombat, combat.go:1924). Queue-leave is
	// never gated: C++'s port action==0 arm has no combat check.
	if s.server != nil && IsArenaMap(s.player.Map) && s.isInCombat() {
		if arena := s.server.findArenaState(s.player.Map, 0); arena != nil {
			arena.mu.Lock()
			_, inArena := arena.PlayerTeams[s.playerGUID]
			waitLeave := arena.Status == ArenaStatusWaitLeave
			arena.mu.Unlock()
			if inArena && !waitLeave {
				return true
			}
		}
	}
	// Player::LeaveBattleground (Player.cpp:22490-22499): leaving a live
	// battleground while inside it is tracked. Battleground::isBattleground()
	// excludes arenas; Go keeps no live BG instances, so an active non-arena
	// slot with Status == BGStatusInProgress is the ported-in state (the port
	// arm sets it on accept, battleground.go).
	// Player::LeaveBattleground (Player.cpp:22475-22485): leaving a live
	// battleground marks the leaver deserter — spell 26013 cast triggered —
	// unless they are a game master or Battleground.CastDeserter is off.
	// A far-teleport leave defers the cast until the teleport completes
	// (DELAYED_SPELL_CAST_DESERTER, Player.cpp:1918): Go arms
	// pendingDeserterCast and fires it in completeWorldPort, the
	// ProcessDelayedOperations analog for the far-teleport semaphore.
	if !fromLogout && s.server != nil && s.server.Config.BattlegroundCastDeserter && !sessionIsGameMaster(s) {
		for i := range s.bgQueues {
			e := &s.bgQueues[i]
			if e.Active && !e.IsArena && (e.Status == BGStatusInProgress || e.Status == BGStatusWaitJoin) {
				if skipEntryTeleport {
					s.pendingDeserterCast = true
				} else {
					s.castSpellDirect(ctx, deserterSpellBG, s.playerGUID)
				}
				break
			}
		}
	}
	if !fromLogout {
		for i := range s.bgQueues {
			e := &s.bgQueues[i]
			if e.Active && !e.IsArena && (e.Status == BGStatusInProgress || e.Status == BGStatusWaitJoin) {
				s.trackBGDeserter(ctx, bgDesertionTypeLeaveBG)
				break
			}
		}
	}
	if s.server != nil {
		s.server.handleWSGPlayerLeave(s)
		s.server.handleEOTSPlayerLeave(s)
		s.server.handleSAPlayerLeave(s)
		s.server.handleICPlayerLeave(s)
		s.server.handleAVPlayerLeave(s)
		s.server.handleArenaPlayerLeave(s)
		s.server.handleWGPlayerLeave(s)
		// Battleground::RemovePlayerAtLeave (Battleground.cpp:840) dequeues the
		// player from the resurrect queue on BG leave: drop the revive-queue
		// entry and strip SPELL_WAITING_FOR_RESURRECT.
		s.server.spiritWaveMu.Lock()
		_, queued := s.server.spiritReviveQueue[s.playerGUID]
		if queued {
			delete(s.server.spiritReviveQueue, s.playerGUID)
		}
		s.server.spiritWaveMu.Unlock()
		if queued {
			s.removeAura(2584) // SPELL_WAITING_FOR_RESURRECT
		}
	}
	// Battleground::RemovePlayerAtLeave (Battleground.cpp:916-927): the
	// trailing player arms run only on the voluntary-leave path. C++
	// LogoutPlayer never calls LeaveBattleground (offline removal is the
	// separate untracked OFFLINE site, Battleground.cpp:271), and
	// Player::LeaveBattleground itself gates on GetBattleground() != null
	// (Player.cpp:22471); Player::InBattleground is m_bgData.bgInstanceID !=
	// 0 (Player.h:1906), mirrored by s.bgData.InstanceID.
	wasInBattleground := !fromLogout && s.bgData.InstanceID != 0
	if wasInBattleground {
		// Battleground::RemovePlayer (Battleground.cpp:894-900): drop the
		// player from the team's BG raid; the last member out disbands the
		// raid and clears its slot. Runs before the InstanceID zeroing
		// below, which the removal reads.
		s.server.removePlayerFromBGRaid(s)
		// Battleground.cpp:918-920: SetBattlegroundId(0,
		// BATTLEGROUND_TYPE_NONE) + SetBGTeam(0). The bgTypeID=NONE term has
		// no Go field, and SetBGTeam's arena-faction reset dies with the
		// queue slots deactivated below. The stale instance id previously
		// kept gating corpse lootability (death.go), the group/guild BG
		// gates (group.go, guild.go) and the BG-only spell checks
		// (spells.go) after the player had already left.
		s.bgData.InstanceID = 0
		s.bgData.Team = 0
		// Battleground.cpp:826-828: the C++ removes SPELL_AURA_MOD_SHAPESHIFT
		// when the player carries spirit of redemption - mirrored exactly,
		// like the arena-end twin (battleground_arena.go).
		if s.hasAuraType(spellAuraSpiritOfRedemption) {
			s.removeAurasByType(spellAuraModShapeshift)
		}
		// Battleground.cpp:832-835: resurrect on exit, spawning corpse bones.
		// The SPELL_AURA_MOUNTED strip (Battleground.cpp:830) is deliberately
		// not mirrored: C++ restores the mount after the teleport via
		// DELAYED_BG_MOUNT_RESTORE, which has no Go twin, so stripping would
		// dismount where C++ leaves the player mounted.
		if s.player.Health == 0 {
			s.resurrectPlayer(ctx, 1.0)
			s.spawnCorpseBones(ctx)
		}
	}
	s.resetAchievementCriteriaByCondition(criteriaConditionBGMap, s.player.Map)
	// The teleportToEntryPoint=false arm: Player::TeleportTo (Player.cpp:1814)
	// calls LeaveBattleground(false) on a far teleport away from the BG, so
	// the player lands at the far destination instead of being bounced to
	// the BG entry point. Battleground::RemovePlayerAtLeave gates the
	// TeleportToBGEntryPoint arm on its teleportToEntryPoint parameter
	// (Battleground.cpp:925-927).
	if wasInBattleground && !skipEntryTeleport && s.bgData.JoinMap != 0 {
		// Battleground.cpp:925-927 with Transport=true:
		// Player::TeleportToBGEntryPoint (Player.cpp:1896-1906); the JoinMap
		// gate mirrors the arena-end twin (battleground_arena.go) - C++
		// returns false with no teleport when the entry point is invalid.
		s.teleportTo(s.bgData.JoinMap, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO)
	}
	for slot := 0; slot < len(s.bgQueues); slot++ {
		if s.bgQueues[slot].Active {
			s.bgQueues[slot].Active = false
			s.bgQueues[slot].Status = BGStatusNone
			s.sendBattlefieldStatus(uint8(slot))
		}
	}
	return true
}

// handleReportPvPAfk processes CMSG_REPORT_PVP_AFK (0x3E4).
// Reference: WorldSession::HandleReportPvPAFK (BattleGroundHandler.cpp:795) and Player::ReportedAfkBy (Player.cpp:22524).
func (s *session) handleReportPvPAfk(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// BattleGroundHandler.cpp:797: `ObjectGuid playerGuid; recvData >>
	// playerGuid` reads a packed GUID (ByteBuffer::operator>> on ObjectGuid
	// calls ReadPackedGuid); a raw 8-byte read misparses it — packed payloads
	// with a full mask are 9 bytes and start with the mask byte, so raw reads
	// yielded garbage guids and every AFK report no-op'd. Truncated packed
	// payloads throw in C++, so a read error reports nothing.
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadPackedGUID()
	if err != nil {
		return true
	}

	if s.server == nil {
		return true
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		return true
	}

	// Player::ReportedAfkBy (Player.cpp:22526-22529): the report only counts when both
	// players share the same live battleground, that battleground is in progress, and
	// both are on the same team. Go has no live battleground instance model outside
	// arenas, so the gates are mapped onto the live arena state; reports on non-arena
	// maps are silent no-ops, as they would be in C++ with GetBattleground() null.
	arena := s.server.findArenaState(s.player.Map, 0)
	if arena == nil {
		return true
	}
	arena.mu.Lock()
	reporterTeam, reporterIn := arena.PlayerTeams[s.playerGUID]
	targetTeam, targetIn := arena.PlayerTeams[targetGUID]
	gates := reporterIn && targetIn && reporterTeam == targetTeam && arena.Status == ArenaStatusInProgress
	arena.mu.Unlock()
	if !gates {
		return true
	}

	// Player::CanReportAfkDueToLimit (Player.cpp:22514-22521): a player can complain
	// about 15 people per 5 minutes. The window resets on a 5-minute timer
	// (Player::UpdateAfkReport, Player.cpp:20715-20721), evaluated lazily here.
	// The budget is consumed only for a fresh, non-debuffed report: C++ short-circuits
	// CanReportAfkDueToLimit() after the duplicate/aura arms (Player.cpp:22532), so
	// duplicate or already-debuffed reports cost the reporter nothing.
	now := time.Now()
	s.writeMu.Lock()
	if now.After(s.afkReportWindowEnd) {
		s.afkReportWindowEnd = now.Add(5 * time.Minute)
		s.afkReportedCount = 0
	}
	s.writeMu.Unlock()

	// Player::ReportedAfkBy (Player.cpp:22531-22542): no duplicate reporters, and no
	// report against a target already carrying Idle (43680) or Inactive (43681). On
	// reaching CONFIG_BATTLEGROUND_REPORT_AFK reporters (default 3) the Idle debuff is
	// cast and the reporter set is cleared.
	targetSess.writeMu.Lock()
	if targetSess.afkReporters == nil {
		targetSess.afkReporters = make(map[uint64]struct{})
	}
	_, dup := targetSess.afkReporters[s.playerGUID]
	fresh := !dup && !targetSess.hasAura(43680) && !targetSess.hasAura(43681)
	existing := len(targetSess.afkReporters)
	targetSess.writeMu.Unlock()

	allowed := false
	if fresh {
		s.writeMu.Lock()
		allowed = s.afkReportedCount < 15
		s.afkReportedCount++
		s.writeMu.Unlock()
	}

	reportCount := existing
	if fresh && allowed {
		targetSess.writeMu.Lock()
		targetSess.afkReporters[s.playerGUID] = struct{}{}
		reportCount = len(targetSess.afkReporters)
		if reportCount >= 3 {
			targetSess.afkReporters = make(map[uint64]struct{})
		}
		targetSess.writeMu.Unlock()
	}

	s.debug("reported player for pvp afk", "account", s.accountName, "target", targetGUID, "reports", reportCount)
	if reportCount >= 3 {
		targetSess.applyAura(43680) // Idle debuff
	}
	return true
}

// handleBattlegroundPlayerPositions processes MSG_BATTLEGROUND_PLAYER_POSITIONS (0x2E9).
// Reference: WorldSession::HandleBattlegroundPlayerPositionsOpcode (BattlegroundHandler.cpp:262).
func (s *session) handleBattlegroundPlayerPositions(ctx context.Context, payload []byte) bool {
	// STATUS_LOGGEDIN (Opcodes.cpp:876): pre-player packets are dropped with the
	// connection open (WorldSession.cpp:427-432), never closed.
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Reference BattlegroundHandler.cpp:266-268:
	// Only respond if player is inside a battleground or arena instance/map
	switch s.player.Map {
	case 30, 489, 529, 566, 607, 628, 559, 562, 572, 617, 618:
		// Reference BattleGroundHandler.cpp:274-286:
		// The two flag-picker queries run in order: GetFlagPickerGUID(TEAM_ALLIANCE)
		// then GetFlagPickerGUID(TEAM_HORDE), each counted only when the player is
		// still in the world (== ObjectAccessor::FindPlayer non-null). WSG answers
		// per team (BattlegroundWS.h:211); EOTS ignores the team arg and returns the
		// single flag keeper for both queries (BattlegroundEY.h:392), so the same
		// carrier is sent twice — matching the C++ wire output.
		var flagCarriers []*session
		if s.server != nil {
			flagCarriers = append(flagCarriers, s.server.getWSGFlagCarriers(s.player.Map)...)
			eotsCarriers := s.server.getEOTSFlagCarriers(s.player.Map)
			flagCarriers = append(flagCarriers, eotsCarriers...)
			flagCarriers = append(flagCarriers, eotsCarriers...)
		}

		// Reference BattleGroundHandler.cpp:288-294:
		// numPlayerPositions is hardcoded 0 — the per-player guid/x/y loop is
		// commented out in C++; player positions are never sent.
		buf := protocol.NewBuffer(8 + len(flagCarriers)*16)
		buf.WriteU32(0)                         // numPlayerPositions
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

// spellRecentlyDroppedFlag is SPELL_RECENTLY_DROPPED_FLAG (18012): the
// still-has-recently-held-flag debuff that blocks battleground object use
// (Player::CanUseBattlegroundObject, Player.cpp:24706).
const spellRecentlyDroppedFlag = 18012

// canUseBattlegroundObject mirrors Player::CanUseBattlegroundObject
// (Player.cpp:24691-24708). With a null gameobject target the faction check
// is skipped (the C++ null comment); otherwise the GO's
// gameobject_template_addon faction must be friendly to the caster's
// faction template (FactionTemplateEntry::IsFriendlyTo, via the friends-list
// bridge), and the caster must not be damage-immune, must not carry the
// recently-dropped-flag debuff, and must be alive. Missing DBC rows skip
// their check, matching C++'s null-pointer passes.
func (s *session) canUseBattlegroundObject(goGUID uint64) bool {
	if s == nil || s.player == nil || s.server == nil {
		return true // unknown-data-is-permissive (terrain.go convention)
	}
	if goGUID != 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && s.server.Data != nil {
		entry := uint32((goGUID >> 24) & 0xFFFFFF)
		var faction int64
		if err := s.server.WorldStore.DB.QueryRowContext(context.Background(),
			"SELECT COALESCE(faction, 0) FROM gameobject_template_addon WHERE entry = ? LIMIT 1", entry).Scan(&faction); err == nil && faction != 0 {
			playerFaction := s.effectiveFactionTemplate(s.player.Race)
			_, pfound, perr := s.server.Data.FactionTemplate(playerFaction)
			_, gfound, gerr := s.server.Data.FactionTemplate(uint32(faction))
			if pfound && gfound && perr == nil && gerr == nil {
				caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: playerFaction, Reputations: playerReputationMap(s.player.Reputations), Sess: s}
				if !s.server.isFriendlyFaction(uint32(faction), caster) {
					return false
				}
			}
		}
	}
	if s.isTotalImmune() {
		return false
	}
	if s.hasAura(spellRecentlyDroppedFlag) {
		return false
	}
	return s.player.Health > 0
}
