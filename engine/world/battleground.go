package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
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
	if s.server == nil || s.player == nil {
		return false
	}
	tmpl := s.server.bgTemplateForType(ctx, bgTypeID)
	if tmpl == nil {
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
	return level >= uint32(tmpl.minLevel) && level <= uint32(tmpl.maxLevel)
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
	// BattlegroundMgr::BuildBattlegroundListPacket (BattlegroundMgr.cpp:677-698):
	// arenas write a bare 0; battlegrounds write the claimed client ids for
	// the requester's bracket (m_ClientBattlegroundIds[type][bracket]) — the
	// same set the simulated pop claims into, so a popped instance appears
	// in the list exactly as a C++ CreateNewBattleground instance would
	// until its BG is deleted. Go never deletes (no BG lifecycle past the
	// invite dialog), so a row may outlive its pop — the recorded
	// never-release delta of the claim map, not a new one. The bracket key
	// is the same bgBracketIndex the pop claim uses, so list and claim
	// cannot drift apart.
	var listIDs []uint32
	if bgTypeID != battlegroundAA && s != nil && s.player != nil {
		if idx := bgBracketIndex(uint32(s.player.Level)); idx >= 0 {
			listIDs = s.server.bgClientInstanceIDs(bgTypeID, uint8(idx))
		}
	}
	buf.WriteU32(uint32(len(listIDs)))
	for _, id := range listIDs {
		buf.WriteU32(id)
	}
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
	// BattleGroundHandler.cpp:85: instance id, 0 if First Available selected.
	// C++ spends it only on resolving bg/template (Handler:114-119); it is
	// never stored in the queue entry (AddGroup sets IsInvitedToBGInstanceGUID
	// to 0, BattlegroundQueue.cpp:143), so it is read for alignment only and
	// the entry below stores 0 until the simulate invite fire sets the
	// invited instance id. The join-time WAIT_QUEUE packet's live client
	// id (builder, BattlegroundMgr.cpp:209) needs the bgDataStore lookup
	// Go has no twin of (battleground_mgr.go header), and Go's own
	// battlefield list now advertises claimed ids, but the join
	// packet still needs a live-instance resolution — so 0, the
	// template fallback for an unresolvable id, stands for the join
	// packet; echoing the raw packet id would invent a live instance.
	if _, err := r.ReadU32(); err != nil {
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
		s.handleBattlemasterJoinGroup(bgTypeID)
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
	// InstanceID stays 0 here: BattlegroundQueue::AddGroup sets
	// IsInvitedToBGInstanceGUID to 0 (BattlegroundQueue.cpp:143) and only
	// InviteGroupToBG sets it, at invite time (BattlegroundQueue.cpp:438) —
	// the simulate invite fire below is Go's InviteGroupToBG. Storing the
	// join packet's instance id here let a pre-invite port accept pass the
	// invited gate below, where C++ answers it with silence.
	s.bgQueues[slot] = bgQueueEntry{
		Active:   true,
		BgTypeID: bgTypeID,
		JoinTime: joinTime,
		Status:   BGStatusWaitQueue,
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

// resolveRandomBGType is the Go form of the GetRandomBG call in
// BattlegroundMgr::CreateNewBattleground (BattlegroundMgr.cpp:353): the
// queued random type resolves to one concrete battleground, or
// BATTLEGROUND_TYPE_NONE (0) when no template/DBC/candidate exists —
// 0 doubles as the entry's unresolved marker, so the pop falls back to
// the queued type's map. The roll is the weightedRandomIndex convention
// (getRandomBG, battleground_mgr.go).
func (s *session) resolveRandomBGType() uint32 {
	if s == nil || s.server == nil {
		return battlegroundTypeNone
	}
	return s.server.getRandomBG(context.Background(), battlegroundRB, rand.Float64())
}

// simulateBGQueueInvite arms the queue-pop simulation for one queued slot:
// after bgSimulatedInviteDelay the entry flips to STATUS_WAIT_JOIN with the
// given instance id (0 → a fresh synthetic id is minted at fire time) and
// the client gets the invite via SMSG_BATTLEFIELD_STATUS. The join-time
// guard drops the invite if the slot was left or reused meanwhile.
// A random-BG slot resolves here, at arm time, not at fire time: the
// group-join path arms one pop per member for a single shared instance,
// and C++ invites the whole group to the one battleground
// CreateNewBattleground returns — per-member fire-time rolls would split
// the group across maps. A pre-set ResolvedBgTypeID (the group-shared
// pick) wins over a fresh roll; a template/DBC reload inside the 15s sim
// window is the accepted delta against C++'s pop-time resolution.
func (s *session) simulateBGQueueInvite(slot int, joinTime time.Time, instanceID uint32) {
	if s == nil || slot < 0 || slot >= len(s.bgQueues) {
		return
	}
	entry := &s.bgQueues[slot]
	if !entry.Active || entry.IsArena || entry.Status != BGStatusWaitQueue {
		return
	}
	bgTypeID := entry.BgTypeID
	if entry.ResolvedBgTypeID == 0 && bgTypeID == battlegroundRB {
		entry.ResolvedBgTypeID = s.resolveRandomBGType()
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
		if !e.Active || e.IsArena || e.Status != BGStatusWaitQueue || e.BgTypeID != bgTypeID || !e.JoinTime.Equal(joinTime) {
			return
		}
		if instanceID == 0 {
			instanceID = s.server.mintBGInstanceID()
		}
		// The C++ pop's IsInvitedToBGInstanceGUID: a fresh instance id (no
		// live BG world backs it — the port itself stays no-bridge,
		// battleground.go handleBattlefieldPort).
		e.InstanceID = instanceID
		// BattlegroundMgr::CreateClientVisibleInstanceId
		// (BattlegroundMgr.cpp:325-348): the live instance's
		// display-only client id, claimed at pop time under the queued
		// type and the player's bracket — the sim-internal id above
		// is not a client id and must never reach the wire.
		bracketID := uint8(0)
		if idx := bgBracketIndex(uint32(s.player.Level)); idx >= 0 {
			bracketID = uint8(idx)
		}
		e.ClientInstanceID = s.server.claimBGClientInstanceID(bgTypeID, bracketID, instanceID)
		// The invite dialog carries the live instance's map
		// (Battleground::GetMapId): the resolved type for a random queue,
		// the queued type otherwise — an unresolved random queue keeps
		// the old map-0 behavior (battlegroundMapForType has no RB row).
		if e.ResolvedBgTypeID != 0 {
			e.MapID = battlegroundMapForType(e.ResolvedBgTypeID)
		} else {
			e.MapID = battlegroundMapForType(bgTypeID)
		}
		e.Status = BGStatusWaitJoin
		// BattlegroundQueue::PlayerInvitedToBGUpdateAverageWaitTime
		// (BattlegroundQueue.cpp:238, called per player from InviteGroupToBG:463):
		// the invite feeds the average-wait ring, not the port accept — the
		// accept-time arm is gone (a WaitQueue accept cannot pass the
		// invited gate), so recording only at accept left the ring empty
		// and every queued packet reporting a 0 average wait. Keyed by the
		// queued type (RB stays 32, the queue the average is read back
		// from), not the resolved type.
		if s.server != nil && !joinTime.IsZero() {
			s.server.recordBGQueueWait(bgTypeID, bgWaitTeamIndex(false, false, teamForRace(s.player.Race)), uint32(time.Since(joinTime).Milliseconds()))
		}
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
			instanceID = s.server.mintBGInstanceID()
		}
		// The C++ pop's IsInvitedToBGInstanceGUID: a fresh instance id (the
		// arena state created here is the Go form of the C++ pop's new
		// Battleground object).
		e.InstanceID = instanceID
		e.MapID = battlegroundMapForType(battlegroundAA)
		e.Status = BGStatusWaitJoin
		// Same invite-time recording as simulateBGQueueInvite
		// (BattlegroundQueue.cpp:463): arenas key by the rated flag
		// (BattlegroundQueue.cpp:251-253).
		if s.server != nil && !joinTime.IsZero() {
			s.server.recordBGQueueWait(battlegroundAA, bgWaitTeamIndex(true, e.IsRated, 0), uint32(time.Since(joinTime).Milliseconds()))
		}
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
func (s *session) handleBattlemasterJoinGroup(bgTypeID uint32) {
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
	// GetAverageQueueWaitTime; sendBattlefieldStatus answers it from the
	// average-wait ring the simulate invite fire feeds.
	// One shared synthetic instance id for the simulated pop, like the
	// C++ GroupQueueInfo's single IsInvitedToBGInstanceGUID — and one
	// shared random-BG resolution for the same reason: C++ invites the
	// group to the single battleground CreateNewBattleground returns.
	sharedResolved := uint32(0)
	if bgTypeID == battlegroundRB {
		sharedResolved = s.resolveRandomBGType()
	}
	simInstanceID := s.server.mintBGInstanceID()
	for _, member := range members {
		slot := memberFreeBGQueueIndex(member)
		if slot == -1 {
			continue
		}
		joinTime := time.Now()
		// InstanceID stays 0 until the simulate invite fire: AddGroup sets
		// IsInvitedToBGInstanceGUID to 0 (BattlegroundQueue.cpp:143), same
		// as the solo arm above.
		member.bgQueues[slot] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, ResolvedBgTypeID: sharedResolved, JoinTime: joinTime, Status: BGStatusWaitQueue}
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
	// comes from GetAverageQueueWaitTime; sendBattlefieldStatus answers it from the
	// average-wait ring the simulate invite fire feeds. bg->SetRated(isRated) has no Go
	// template model; the rating rides on the queue entry, the solo-arm convention.
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
	simInstanceID := s.server.mintBGInstanceID()
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
	// (BattleGroundHandler.cpp:453-454). Joins store InstanceID 0 (AddGroup,
	// BattlegroundQueue.cpp:143) and only the simulate invite fire sets it, so
	// this gate is now exact: a pre-invite accept can no longer pass it.
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
	// leave. C++ reads bg->GetMaxLevel(), the LIVE instance's max level, so a
	// resolved random queue gates on the resolved type's template, not the
	// queued RB template; a missing row leaves the demote unevaluated.
	if action == 1 && !entry.IsArena && s.player != nil {
		gateType := bgTypeID
		if entry.ResolvedBgTypeID != 0 {
			gateType = entry.ResolvedBgTypeID
		}
		if tmpl := s.server.bgTemplateForType(ctx, gateType); tmpl != nil && uint32(s.player.Level) > uint32(tmpl.maxLevel) {
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

		// The invite fire in simulateBGQueueInvite/simulateArenaQueueInvite is
		// the sole recording point for BattlegroundQueue::PlayerInvitedToBGUpdateAverageWaitTime
		// (BattlegroundQueue.cpp:238, per player from InviteGroupToBG:463). No
		// accept-time arm exists: a WaitQueue accept cannot pass the invited
		// gate above (InstanceID is 0 until the fire flips the slot to
		// WaitJoin), so the old pre-invite-accept recording arm was dead.

		// STATUS_IN_PROGRESS leg (BattleGroundHandler.cpp:468-482): the slot data
		// C++ keeps across the port — SetBattlegroundId (instance) and SetBGTeam
		// (ginfo.Team) — then the BuildBattlegroundStatusPacket. The
		// SendToBattleground teleport (BattleGroundHandler.cpp:472-486) has no Go
		// world model: there is no BattlegroundMgr / live Battleground instance
		// to port the player into, so the port itself is documented no-bridge
		// rather than stubbed. The RemovePlayerAtLeave leg (BattleGroundHandler.cpp:
		// 476-479) IS ported below, as the BG-jump arm.
		//
		// BG-jump (BattleGroundHandler.cpp:476-479): when the player is already
		// inside a battleground (GetBattleground() != null) and accepts another
		// invite, C++ runs currentBg->RemovePlayerAtLeave(guid, false, true) on
		// the OLD battleground before SetBattlegroundId switches to the new one.
		// Transport=false is the whole point: no TeleportToBGEntryPoint, and the
		// immediate SendToBattleground carries the player to the new BG instead.
		// RemovePlayerAtLeave is NOT LeaveBattleground, so no deserter is cast
		// or tracked here — that is why jumping "shouldn't add deserter debuff"
		// (BattleGroundHandler.cpp:477). A WaitJoin old slot is not a jump: the
		// player is invited but not yet in that BG (GetBattleground() is null),
		// so only InProgress and WaitLeave old slots are removed; WaitLeave
		// counts because the ended match still holds the player until the
		// auto-remove runs. The internal arms run here, before the overwrite
		// below (the raid removal reads the old bgData instance), while the
		// old STATUS_NONE packets go out after the new IN_PROGRESS packet,
		// matching the C++ packet order. Stay-outs: ScheduleQueueUpdate /
		// AddToBGFreeSlotQueue (sim pops are timer-driven, no scheduler),
		// the PlayerLeft team packet (Go has no SendPacketToTeam twin;
		// broadcastToMap would leak it to the enemy team), and the
		// empty-BG state delete (the player.Map-based emptiness proxy cannot
		// prove the old map empty, so deleting would drop a live match).
		type bgJumpOldSlot struct {
			index      int
			instanceID uint32
			mapID      uint32
		}
		var jumpOld []bgJumpOldSlot
		for i := range s.bgQueues {
			if i == slot {
				continue
			}
			e := &s.bgQueues[i]
			if !e.Active || e.InstanceID == 0 || (e.Status != BGStatusInProgress && e.Status != BGStatusWaitLeave) {
				continue
			}
			jumpOld = append(jumpOld, bgJumpOldSlot{index: i, instanceID: e.InstanceID, mapID: e.MapID})
		}
		if len(jumpOld) > 0 {
			// Battleground::RemovePlayer (Battleground.cpp:894-900) subclass
			// arms, via the same per-map leave funnel handleLeaveBattlefield
			// uses; they key off player.Map and no-op for a jumper who never
			// landed on the old BG map, exactly as the leave path does.
			s.server.handleWSGPlayerLeave(s)
			s.server.handleEOTSPlayerLeave(s)
			s.server.handleSAPlayerLeave(s)
			s.server.handleICPlayerLeave(s)
			s.server.handleAVPlayerLeave(s)
			s.server.handleArenaPlayerLeave(s)
			s.server.handleWGPlayerLeave(s)
			// Battleground::RemovePlayerAtLeave (Battleground.cpp:840):
			// drop the resurrect-queue entry and SPELL_WAITING_FOR_RESURRECT.
			if s.server != nil {
				s.server.spiritWaveMu.Lock()
				_, queued := s.server.spiritReviveQueue[s.playerGUID]
				if queued {
					delete(s.server.spiritReviveQueue, s.playerGUID)
				}
				s.server.spiritWaveMu.Unlock()
				if queued {
					s.removeAura(2584)
				}
			}
			// Battleground.cpp:826-828: spirit-of-redemption shapeshift strip.
			// The SPELL_AURA_MOUNTED strip (Battleground.cpp:830) is deliberately
			// not mirrored, as in handleLeaveBattlefield: C++ restores the mount
			// via DELAYED_BG_MOUNT_RESTORE, which has no Go twin.
			if s.hasAuraType(spellAuraSpiritOfRedemption) {
				s.removeAurasByType(spellAuraModShapeshift)
			}
			// Battleground.cpp:894-900: drop the player from the old BG raid
			// and restore the original group. Reads the still-old bgData, so
			// it must run before the overwrite below; the new raid arm then
			// stashes the restored group instead of the old raid.
			s.server.removePlayerFromBGRaid(s)
			for _, old := range jumpOld {
				// Battleground.cpp:833-837: delete the old score row, keyed
				// the same way handleLeaveBattlefield keys it.
				s.server.deleteBGScoreRow(s.player.Map, old.instanceID, s.playerGUID)
				// Battleground.cpp:918-921: reset BG-map achievement criteria
				// for the OLD map, which the slot still carries.
				if old.mapID != 0 {
					s.resetAchievementCriteriaByCondition(criteriaConditionBGMap, old.mapID)
				}
				s.bgQueues[old.index].Active = false
				s.bgQueues[old.index].Status = BGStatusNone
			}
		}
		team := teamForRace(s.player.Race)
		entry.Status = BGStatusInProgress
		entry.StartTime = time.Now()
		// The in-progress packet carries the live instance's map
		// (Battleground::GetMapId, BattleGroundHandler.cpp:468): the
		// resolved type for a random queue, else the queued type — the
		// pop-time MapID is not trusted here, matching the C++ arm that
		// rebuilds the packet from the instance, and keeping
		// activeBattlegroundMap on the real map after accept.
		if entry.ResolvedBgTypeID != 0 {
			entry.MapID = battlegroundMapForType(entry.ResolvedBgTypeID)
		} else {
			entry.MapID = battlegroundMapForType(bgTypeID)
		}
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
		// RemovePlayerAtLeave's SendPacket=true arm (Battleground.cpp:886-891):
		// the old slot's STATUS_NONE packet, after the new IN_PROGRESS packet
		// above — the C++ packet order (BattleGroundHandler.cpp:468 then :478).
		// The slots were already deactivated above, so this writes the
		// STATUS_NONE form (slot + uint64 0).
		for _, old := range jumpOld {
			s.sendBattlefieldStatus(uint8(old.index))
		}
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
	// BattlegroundMgr::BuildBattlegroundStatusPacket (BattlegroundMgr.cpp:209)
	// writes bg->GetClientInstanceID(), not GetInstanceID(): a queued
	// slot has none claimed yet (0, the template value); a WaitJoin or
	// InProgress slot carries the fire-claimed id; arenas carry 0.
	buf.WriteU32(entry.ClientInstanceID)
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
	// BattleGroundHandler.cpp:562-572 + Battleground.cpp:667-810: C++ never
	// sends STATUS_WAIT_LEAVE in this packet — an ended match still reports
	// STATUS_IN_PROGRESS, with Time1 = GetEndTime() (remaining auto-remove).
	// A Go non-arena WaitLeave slot therefore serializes as InProgress;
	// arena slots keep their own status machine and are untouched.
	wireStatus := entry.Status
	if wireStatus == BGStatusWaitLeave && !entry.IsArena {
		wireStatus = BGStatusInProgress
	}
	buf.WriteU32(wireStatus) // STATUS_WAIT_QUEUE = 1
	switch wireStatus {
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
		// BattlegroundMgr.cpp:225 Time1 is GetEndTime(): 0 while the match
		// runs, the remaining auto-remove countdown once it has ended
		// (EndBattleground sets TIME_TO_AUTOREMOVE, _ProcessLeave decrements
		// it). A running match and an unpeekable state both read 0.
		buf.WriteU32(s.server.bgAutoRemoveRemaining(entry.MapID))
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
		// ClientInstanceID stays 0 here: character_battleground_data
		// persists only the internal id, and C++ re-reads the client id
		// from the live battleground object, which has no Go twin.
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
// (BattlegroundMgr.cpp:210-211), read from the loaded template store. The
// table is unseeded in this repo's world.sql, so the 10/80 fallback applies
// (bgMaxLevelForMap convention); a loader-skipped row (MinLvl/MaxLvl zero
// or inverted) likewise falls back, since C++ creates no template for it.
func bgTemplateLevels(srv *Server, bgTypeID uint32) (uint8, uint8) {
	if tmpl := srv.bgTemplateForType(context.Background(), bgTypeID); tmpl != nil {
		return tmpl.minLevel, tmpl.maxLevel
	}
	return 10, 80
}

// bgMaxLevelForMap returns the BG's m_LevelMax == Battleground::GetMaxLevel,
// read from the loaded template store (MaxLvl) keyed by
// battlegroundTypeForMap's type ID. The table is unseeded in this repo's
// world.sql, so the 80 fallback == DEFAULT_MAX_LEVEL applies (same
// convention as bgAccessByLevel's MaxPlayerLevel cap).
func bgMaxLevelForMap(s *Server, mapID uint32) uint32 {
	if s == nil {
		return 80
	}
	bgTypeID, _, _, ok := battlegroundTypeForMap(mapID)
	if !ok {
		return 80
	}
	if tmpl := s.bgTemplateForType(context.Background(), bgTypeID); tmpl != nil {
		return uint32(tmpl.maxLevel)
	}
	return 80
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
	// Capture the BG the leaver is in before any arm below zeroes
	// InstanceID or deactivates the queue slots: Battleground::Update
	// (Battleground.cpp:164-186) deletes an empty BG at any status, so the
	// last player out of a Go per-map state deletes it — the next entrant
	// recreates it pristine via getOrCreate, the CreateBattleground +
	// Reset() pair (BattlegroundMgr.cpp:412). The map lives on the queue
	// slot (activeBattlegroundMap); player.Map is the fallback when no
	// slot is active. Arenas delete through deleteArenaState instead, so
	// the dispatcher at the end only handles the six non-arena maps.
	leaveBGMap := uint32(0)
	if s.bgData.InstanceID != 0 {
		leaveBGMap = s.activeBattlegroundMap()
		if leaveBGMap == 0 {
			leaveBGMap = s.player.Map
		}
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
	// BattleGroundHandler.cpp:531-540: leaving a battleground while in combat
	// is denied unless the BG is already STATUS_WAIT_LEAVE, and the denial
	// runs before Player::LeaveBattleground, so it casts no deserter and
	// tracks nothing. The arena arm above covers arena maps; a non-arena
	// ported-in slot (Status InProgress) is the same gate for battlegrounds
	// — WaitLeave slots are exempt, and WaitJoin slots are not yet in a BG
	// (C++ GetBattleground() is null there), so neither is gated. The opcode
	// handler is the only C++ site with this gate, so internal callers (nil
	// payload: logout, GM command, areatrigger) bypass it.
	if payload != nil && s.isInCombat() {
		for i := range s.bgQueues {
			e := &s.bgQueues[i]
			if e.Active && !e.IsArena && e.InstanceID != 0 && e.Status == BGStatusInProgress {
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
	// Battleground::RemovePlayerAtLeave (Battleground.cpp:833-837)
	// deletes the leaver's PlayerScores row. Read before the
	// wasInBattleground block zeroes InstanceID; the logout arm runs
	// here too, since Go has no offline queue to defer it through
	// (C++ holds the row up to 5 minutes in m_OfflineQueue, then
	// RemovePlayerAtLeave deletes it — battleground.go's untracked
	// OFFLINE site). A rejoin always mints a fresh instance id, so
	// the deleted row's key is dead either way.
	if s.server != nil {
		s.server.deleteBGScoreRow(s.player.Map, s.bgData.InstanceID, s.playerGUID)
	}
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
	// Last player out: delete the per-map state so the next match starts
	// pristine (see the capture above). Runs for the logout path too — Go
	// has no offline queue holding the BG open, and the score row is
	// already deleted on logout.
	if leaveBGMap != 0 && s.server != nil {
		s.server.deleteBattlegroundStateOnEmpty(s, leaveBGMap)
	}
	return true
}

// deleteBattlegroundStateOnEmpty deletes a non-arena per-map BG state once
// no BG player remains on the map, the Go form of Battleground::Update
// deleting an empty BG (Battleground.cpp:164-186, "BG is empty — if there
// are no players invited, delete BG"). The count is the codebase's
// participant proxy — worldReady sessions on the map with a live BG
// instance id — so a GM-teleported bystander on the map without an
// instance never blocks the delete, and an invited-but-not-entered player
// does not hold it open (C++ counts invited players; Go has no invited
// count, so that narrow window is a known delta). The per-map state model
// conflates concurrent instances on one map, so the count does too. A
// state deleted here is recreated pristine by the next getOrCreate, which
// is the CreateBattleground + Reset() pair (BattlegroundMgr.cpp:412).
func (s *Server) deleteBattlegroundStateOnEmpty(leaver *session, mapID uint32) {
	if s == nil || leaver == nil {
		return
	}
	s.sessionsMu.RLock()
	remaining := false
	for sess := range s.sessions {
		if sess == leaver {
			continue
		}
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID && sess.bgData.InstanceID != 0 {
			remaining = true
			break
		}
	}
	s.sessionsMu.RUnlock()
	if remaining {
		return
	}
	switch mapID {
	case WSGMapID:
		s.deleteWSGState(mapID)
	case ABMapID:
		s.deleteABState(mapID)
	case AVMapID:
		s.deleteAVState(mapID)
	case EOTSMapID:
		s.deleteEOTSState(mapID)
	case SAMapID:
		s.deleteSAState(mapID)
	case ICMapID:
		s.deleteICState(mapID)
	}
}

// bgWaitLeaveAutoRemove mirrors TIME_TO_AUTOREMOVE (Battleground.h:151):
// _ProcessLeave removes stayers 120 seconds after the match ends.
const bgWaitLeaveAutoRemove = 120 * time.Second

// bgAutoRemoveRemaining returns the milliseconds until _ProcessLeave
// auto-removes stayers of an ended match on mapID (Battleground::GetEndTime,
// Battleground.cpp:512-515, set to TIME_TO_AUTOREMOVE at EndBattleground:717
// and decremented by _ProcessLeave). It returns 0 for a running match, for
// a map with no state, and for an ended state with a zero EndedAt — the
// updateBattlegroundWaitLeave due-immediately convention for a stamp-less
// end arm. The peek creates no state.
func (s *Server) bgAutoRemoveRemaining(mapID uint32) uint32 {
	if s == nil {
		return 0
	}
	var winner int8 = -1
	var endedAt time.Time
	switch mapID {
	case AVMapID:
		s.objectsMu.RLock()
		state := s.avState[mapID]
		s.objectsMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	case WSGMapID:
		s.wsgMu.RLock()
		state := s.wsgState[mapID]
		s.wsgMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	case ABMapID:
		s.abMu.RLock()
		state := s.abState[mapID]
		s.abMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	case EOTSMapID:
		s.eotsMu.RLock()
		state := s.eotsState[mapID]
		s.eotsMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	case SAMapID:
		s.saMu.RLock()
		state := s.saState[mapID]
		s.saMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	case ICMapID:
		s.icMu.RLock()
		state := s.icState[mapID]
		s.icMu.RUnlock()
		if state == nil {
			return 0
		}
		state.mu.Lock()
		winner, endedAt = state.Winner, state.EndedAt
		state.mu.Unlock()
	default:
		return 0
	}
	if winner < 0 || endedAt.IsZero() {
		return 0
	}
	remaining := bgWaitLeaveAutoRemove - time.Since(endedAt)
	if remaining < 0 {
		return 0
	}
	return uint32(remaining.Milliseconds())
}

// markBGSlotsWaitLeave flips every live participant slot on mapID to
// BGStatusWaitLeave, the Go form of Battleground::EndBattleground setting
// STATUS_WAIT_LEAVE (Battleground.cpp:715). Player::LeaveBattleground
// (Player.cpp:22475-22499) casts deserter and tracks desertion only while
// the BG is STATUS_IN_PROGRESS or STATUS_WAIT_JOIN, so without this flip a
// player who stays to the end and then leaves — voluntarily or by the
// auto-remove below — would wrongly take deserter. Called from each
// per-map end arm with that map's state mu held; it takes no BG mu, only
// sessionsMu, following the endArena slot-flip precedent. No status packet
// is sent here: the end arms broadcast the result already, and the leave
// funnel sends STATUS_NONE on removal.
func (s *Server) markBGSlotsWaitLeave(mapID uint32) {
	if s == nil || mapID == 0 {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.bgData.InstanceID == 0 {
			continue
		}
		for i := range sess.bgQueues {
			if e := &sess.bgQueues[i]; e.Active && !e.IsArena {
				e.Status = BGStatusWaitLeave
			}
		}
	}
}

// updateBattlegroundWaitLeave ports Battleground::_ProcessLeave
// (Battleground.cpp:506-528) for the six non-arena maps: once a match has
// been ended for TIME_TO_AUTOREMOVE, every remaining player is removed via
// the voluntary-leave funnel (handleLeaveBattlefield, the RemovePlayerAtLeave
// + TeleportToBGEntryPoint pair with Transport=true, SendPacket=true), which
// deletes the stayer's score row, resurrects, teleports to the entry point,
// and lets the last leave delete the per-map state. Arenas already port
// this in updateArenaTick (processArenaLeave); Wintergrasp is a Battlefield,
// not a Battleground, and keeps its own timers. The sweep peeks the
// registries (never getOrCreate, so a map with no BG creates no state) and
// runs on the world tick's 1s cadence beside the other per-map battle
// ticks. A Winner>=0 state with a zero EndedAt (an end arm added without
// the stamp) is treated as due rather than left to persist indefinitely.
// The AV peek uses objectsMu, the creation lock, per the deleteAVState
// comment; the other maps peek under their own registry mu. No registry
// or state mu is held while the leave funnel runs.
func (s *Server) updateBattlegroundWaitLeave(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	if !s.bgWaitLeaveTickLast.IsZero() && now.Sub(s.bgWaitLeaveTickLast) < time.Second {
		return
	}
	s.bgWaitLeaveTickLast = now
	due := make(map[uint32]bool)
	consider := func(mapID uint32, winner int8, endedAt time.Time) {
		if winner >= 0 && (endedAt.IsZero() || now.Sub(endedAt) >= bgWaitLeaveAutoRemove) {
			due[mapID] = true
		}
	}
	s.wsgMu.RLock()
	wsg := s.wsgState[WSGMapID]
	s.wsgMu.RUnlock()
	if wsg != nil {
		wsg.mu.Lock()
		consider(WSGMapID, wsg.Winner, wsg.EndedAt)
		wsg.mu.Unlock()
	}
	s.abMu.RLock()
	ab := s.abState[ABMapID]
	s.abMu.RUnlock()
	if ab != nil {
		ab.mu.Lock()
		consider(ABMapID, ab.Winner, ab.EndedAt)
		ab.mu.Unlock()
	}
	s.objectsMu.RLock()
	av := s.avState[AVMapID]
	s.objectsMu.RUnlock()
	if av != nil {
		av.mu.Lock()
		consider(AVMapID, av.Winner, av.EndedAt)
		av.mu.Unlock()
	}
	s.eotsMu.RLock()
	eots := s.eotsState[EOTSMapID]
	s.eotsMu.RUnlock()
	if eots != nil {
		eots.mu.Lock()
		consider(EOTSMapID, eots.Winner, eots.EndedAt)
		eots.mu.Unlock()
	}
	s.saMu.RLock()
	sa := s.saState[SAMapID]
	s.saMu.RUnlock()
	if sa != nil {
		sa.mu.Lock()
		consider(SAMapID, sa.Winner, sa.EndedAt)
		sa.mu.Unlock()
	}
	s.icMu.RLock()
	ic := s.icState[ICMapID]
	s.icMu.RUnlock()
	if ic != nil {
		ic.mu.Lock()
		consider(ICMapID, ic.Winner, ic.EndedAt)
		ic.mu.Unlock()
	}
	if len(due) == 0 {
		return
	}
	s.sessionsMu.RLock()
	stayers := make([]*session, 0)
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.bgData.InstanceID == 0 || !due[sess.player.Map] {
			continue
		}
		stayers = append(stayers, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range stayers {
		sess.handleLeaveBattlefield(ctx, nil, false, false)
	}
}

// bgPrematureFinishTime mirrors BattlegroundMgr::GetPrematureFinishTime
// (BattlegroundMgr.cpp:867), the CONFIG_BATTLEGROUND_PREMATURE_FINISH_TIMER
// default (World.cpp:1206: 5 * MINUTE * IN_MILLISECONDS). C++ arms
// _ProcessProgress only when the config is nonzero; Go has no config load
// for it, so the default applies unconditionally.
const bgPrematureFinishTime = 5 * time.Minute

// bgPrematureMinPlayers returns the battleground_template MinPlayersPerTeam
// for mapID: Battleground::GetMinPlayersPerTeam (Battleground.cpp:208),
// copied onto the instance at creation (BattlegroundMgr.cpp:505), read from
// the loaded template store via bgTemplateForType. While the table is
// unseeded (this repo's world.sql) or holds no row for the type, the
// shipped-template fallback below applies.
func (s *Server) bgPrematureMinPlayers(mapID uint32) int {
	if s != nil {
		if bgTypeID, _, _, ok := battlegroundTypeForMap(mapID); ok {
			if tmpl := s.bgTemplateForType(context.Background(), bgTypeID); tmpl != nil {
				return int(tmpl.minPlayersPerTeam)
			}
		}
	}
	return bgPrematureMinPlayersFallback(mapID)
}

// bgPrematureMinPlayersFallback is the shipped-template MinPlayersPerTeam
// table, used only while battleground_template has no row for the type.
// AV's 20 is the documented minimum; the others are the shipped-template
// values; SA/IC are the least certain and the first a seeded table corrects.
func bgPrematureMinPlayersFallback(mapID uint32) int {
	switch mapID {
	case AVMapID:
		return 20
	case WSGMapID:
		return 5
	case ABMapID:
		return 6
	case EOTSMapID:
		return 6
	case SAMapID:
		return 10
	case ICMapID:
		return 20
	default:
		return 0
	}
}

// bgRosterCounts counts live BG participants on mapID per team, the Go form
// of Battleground::GetPlayersCountByTeam (Battleground.cpp:208, 347): a
// worldReady session on the map with a live BG instance id, split by race
// team (teamForRace, as rewardBGEndHonor does). Invited-but-not-entered
// players are not counted — Go has no invited count, the same narrow window
// the last-leave delete already carries.
func (s *Server) bgRosterCounts(mapID uint32) (ally, horde int) {
	if s == nil {
		return 0, 0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.bgData.InstanceID == 0 {
			continue
		}
		if teamForRace(sess.player.Race) == 0 {
			ally++
		} else {
			horde++
		}
	}
	return ally, horde
}

// bgBasePrematureWinner is Battleground::GetPrematureWinner
// (Battleground.cpp:344-352): the team still at or above the minimum wins;
// when neither team is, C++ returns 0, the neutral value EndBattleground
// maps to PVP_TEAM_NEUTRAL — Go 2, the draw value bgEndedWinner already
// maps to PvPTeamId neutral.
func bgBasePrematureWinner(ally, horde, min int) int8 {
	if ally >= min {
		return 0
	}
	if horde >= min {
		return 1
	}
	return 2
}

// prematureCountdownStep is one Battleground::_ProcessProgress tick
// (Battleground.cpp:355-388) for a single instance. trigger is the Update
// arm (Battleground.cpp:208): the premature config is nonzero and either
// team is below the minimum. When trigger is false an armed countdown is
// cancelled (flag only, matching C++: the timer value is left stale). When
// trigger is true and no countdown runs, it starts at the full premature
// time; when the timer runs below diff the match expires. Otherwise the
// timer decrements and, unless the new time crosses no boundary, announces
// on minute boundaries above a minute and 15-second boundaries below it —
// the message value is the OLD timer (C++ sends m_PrematureCountDownTimer
// before assigning newtime), so "5 mins" fires as the timer crosses from
// 5:00 to 4:59, not at 4:00. The caller holds the per-map state mu; the
// returned message is empty unless a boundary fired.
func prematureCountdownStep(active *bool, timerMs *int64, diffMs int64, trigger bool) (expired bool, message string) {
	if !trigger {
		if *active {
			*active = false
		}
		return false, ""
	}
	if !*active {
		*active = true
		*timerMs = int64(bgPrematureFinishTime / time.Millisecond)
		return false, ""
	}
	if *timerMs < diffMs {
		*active = false
		return true, ""
	}
	newtime := *timerMs - diffMs
	if newtime > int64(time.Minute/time.Millisecond) {
		if newtime/(int64(time.Minute/time.Millisecond)) != *timerMs/(int64(time.Minute/time.Millisecond)) {
			message = fmt.Sprintf("Not enough players. This game will close in %d mins.", *timerMs/(int64(time.Minute/time.Millisecond)))
		}
	} else {
		if newtime/(15*int64(time.Second/time.Millisecond)) != *timerMs/(15*int64(time.Second/time.Millisecond)) {
			message = fmt.Sprintf("Not enough players. This game will close in %d seconds.", *timerMs/int64(time.Second/time.Millisecond))
		}
	}
	*timerMs = newtime
	return false, message
}

// finishWSGPremature ends a WSG match on premature expiry: a decided winner
// takes the victory arm, a draw the tie arm (endWSGTie is the C++
// EndBattleground neutral arm). The caller's wsg.mu is held.
func (s *Server) finishWSGPremature(wsg *wsgBattlegroundState, mapID uint32, winner int8) {
	if winner == 2 {
		s.endWSGTie(wsg, mapID)
		return
	}
	s.endWSGVictory(wsg, mapID, uint32(winner))
}

// finishABPremature ends an AB match on premature expiry. AB has no
// dedicated end function: the resource-max arm (TickResources) sets Winner,
// stamps EndedAt, flips slots to WAIT_LEAVE, stops node timers and announces
// victory (announceABVictory carries the winner + completion honor from
// BattlegroundAB::EndBattleground, BattlegroundAB.cpp:624). The draw arm
// mirrors the same EndBattleground completion honor for both teams with no
// winner honor, since a neutral winner pays neither side's win bonus. The
// caller's ab.mu is held.
func (s *Server) finishABPremature(ab *abBattlegroundState, winner int8) {
	ab.Winner = winner
	ab.EndedAt = time.Now()
	s.markBGSlotsWaitLeave(ab.MapID)
	s.stopABNodeCaptureTimers(ab)
	if winner == 2 {
		s.broadcastBattlegroundMessage(ab.MapID, "The battle ended in a draw!")
		s.rewardBGEndHonor(ab.MapID, 0, 1)
		s.rewardBGEndHonor(ab.MapID, 1, 1)
		return
	}
	s.announceABVictory(ab.MapID, uint32(winner))
}

// finishAVPremature ends an AV match on premature expiry via endAV, whose
// tower/captain bonuses run for any winner (BattlegroundAV::EndBattleground,
// BattlegroundAV.cpp:453, computes them before the base-class call) and
// whose draw message arm covers the neutral winner. The caller's av.mu is
// held.
func (s *Server) finishAVPremature(av *avBattlegroundState, winner int8) {
	s.endAV(av, winner)
}

// finishEOTSPremature ends an EotS match on premature expiry. Like AB, EotS
// has no dedicated end function: the resource-max arm (addEOTSResources)
// sets Winner, stamps EndedAt, flips slots, stops the flag timers and
// announces victory (announceEOTSVictory carries the winner + completion
// honor from BattlegroundEY::EndBattleground, BattlegroundEY.cpp:316). The
// draw arm mirrors the completion honor for both teams with no winner
// honor. The caller's eots.mu is held.
func (s *Server) finishEOTSPremature(eots *eotsBattlegroundState, winner int8) {
	eots.Winner = winner
	eots.EndedAt = time.Now()
	s.markBGSlotsWaitLeave(eots.MapID)
	if eots.FlagRespawnTimer != nil {
		eots.FlagRespawnTimer.Stop()
		eots.FlagRespawnTimer = nil
	}
	if eots.FlagReturnTimer != nil {
		eots.FlagReturnTimer.Stop()
		eots.FlagReturnTimer = nil
	}
	if winner == 2 {
		s.broadcastBattlegroundMessage(eots.MapID, "The battle ended in a draw!")
		s.rewardBGEndHonor(eots.MapID, 0, 1)
		s.rewardBGEndHonor(eots.MapID, 1, 1)
		return
	}
	s.announceEOTSVictory(eots.MapID, uint32(winner))
}

// finishSAPremature ends an SA match on premature expiry via endSA, which
// already handles the draw winner (2). The caller's sa.mu is held.
func (s *Server) finishSAPremature(sa *saBattlegroundState, winner int8) {
	s.endSA(sa, winner)
}

// finishICPremature ends an IoC match on premature expiry via endIC, whose
// draw message arm covers the neutral winner. The caller's ic.mu is held.
func (s *Server) finishICPremature(ic *icBattlegroundState, winner int8) {
	s.endIC(ic, winner)
}

// updateBattlegroundPrematureFinish ports the STATUS_IN_PROGRESS arm of
// Battleground::Update (Battleground.cpp:205-211) plus _ProcessProgress
// (Battleground.cpp:355-388) for the six non-arena maps: when either team's
// live roster drops below the minimum, a 5-minute countdown starts,
// announces on minute/15-second boundaries, and on expiry ends the match
// with the per-map GetPrematureWinner (score/node/reinforcement legs, base
// roster leg on ties); when both teams are back at or above the minimum
// an armed countdown cancels. Arenas are excluded by the C++ arm itself
// (the arena branch ends at 47 minutes instead); Wintergrasp is a
// Battlefield, not a Battleground. The sweep peeks the registries (never
// getOrCreate) and runs on the world tick's 1s cadence beside
// updateBattlegroundWaitLeave; the per-map Winner >= 0 gate keeps ended
// matches out, and the AV peek uses objectsMu, the creation lock. SA is
// additionally gated on an in-progress round (RoundOne/RoundTwo): C++ only
// runs _ProcessProgress in STATUS_IN_PROGRESS, and SA is the one Go map
// with a warmup/finished status to gate on. The other five Go maps have no
// WAIT_JOIN/IN_PROGRESS distinction (Winner -1 spans warmup and play), so
// their countdown may start during warmup — a roster at queue-full rarely
// trips it there. No registry or state mu is held across the roster count;
// the end arms run under the state mu, the same ordering their normal
// callers already use.
func (s *Server) updateBattlegroundPrematureFinish(now time.Time) {
	if s == nil {
		return
	}
	if !s.bgPrematureTickLast.IsZero() && now.Sub(s.bgPrematureTickLast) < time.Second {
		return
	}
	diffMs := int64(0)
	if !s.bgPrematureTickLast.IsZero() {
		diffMs = now.Sub(s.bgPrematureTickLast).Milliseconds()
	}
	s.bgPrematureTickLast = now

	// WSG: score leg, base leg on ties.
	s.wsgMu.RLock()
	wsg := s.wsgState[WSGMapID]
	s.wsgMu.RUnlock()
	if wsg != nil {
		wsg.mu.Lock()
		ended := wsg.Winner >= 0
		wsg.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(WSGMapID)
			min := s.bgPrematureMinPlayers(WSGMapID)
			wsg.mu.Lock()
			if wsg.Winner < 0 {
				expired, msg := prematureCountdownStep(&wsg.PrematureCountDown, &wsg.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(WSGMapID, msg)
				}
				if expired {
					winner := int8(0)
					if w, ok := wsgPrematureWinner(wsg); ok {
						winner = int8(w)
					} else {
						winner = bgBasePrematureWinner(ally, horde, min)
					}
					s.finishWSGPremature(wsg, WSGMapID, winner)
				}
			}
			wsg.mu.Unlock()
		}
	}

	// AB: node-count leg, base leg on ties.
	s.abMu.RLock()
	ab := s.abState[ABMapID]
	s.abMu.RUnlock()
	if ab != nil {
		ab.mu.Lock()
		ended := ab.Winner >= 0
		ab.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(ABMapID)
			min := s.bgPrematureMinPlayers(ABMapID)
			ab.mu.Lock()
			if ab.Winner < 0 {
				expired, msg := prematureCountdownStep(&ab.PrematureCountDown, &ab.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(ABMapID, msg)
				}
				if expired {
					winner := int8(0)
					if w, ok := abPrematureWinner(ab); ok {
						winner = int8(w)
					} else {
						winner = bgBasePrematureWinner(ally, horde, min)
					}
					s.finishABPremature(ab, winner)
				}
			}
			ab.mu.Unlock()
		}
	}

	// AV: reinforcement (m_Team_Scores) leg, base leg on ties.
	s.objectsMu.RLock()
	av := s.avState[AVMapID]
	s.objectsMu.RUnlock()
	if av != nil {
		av.mu.Lock()
		ended := av.Winner >= 0
		av.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(AVMapID)
			min := s.bgPrematureMinPlayers(AVMapID)
			av.mu.Lock()
			if av.Winner < 0 {
				expired, msg := prematureCountdownStep(&av.PrematureCountDown, &av.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(AVMapID, msg)
				}
				if expired {
					winner := int8(0)
					if w, ok := avPrematureWinner(av); ok {
						winner = int8(w)
					} else {
						winner = bgBasePrematureWinner(ally, horde, min)
					}
					s.finishAVPremature(av, winner)
				}
			}
			av.mu.Unlock()
		}
	}

	// EotS: victory-point leg, base leg on ties.
	s.eotsMu.RLock()
	eots := s.eotsState[EOTSMapID]
	s.eotsMu.RUnlock()
	if eots != nil {
		eots.mu.Lock()
		ended := eots.Winner >= 0
		eots.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(EOTSMapID)
			min := s.bgPrematureMinPlayers(EOTSMapID)
			eots.mu.Lock()
			if eots.Winner < 0 {
				expired, msg := prematureCountdownStep(&eots.PrematureCountDown, &eots.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(EOTSMapID, msg)
				}
				if expired {
					winner := int8(0)
					if w, ok := eotsPrematureWinner(eots); ok {
						winner = int8(w)
					} else {
						winner = bgBasePrematureWinner(ally, horde, min)
					}
					s.finishEOTSPremature(eots, winner)
				}
			}
			eots.mu.Unlock()
		}
	}

	// SA: no per-map override in C++, base roster leg only, and only while
	// a round is in progress.
	s.saMu.RLock()
	sa := s.saState[SAMapID]
	s.saMu.RUnlock()
	if sa != nil {
		sa.mu.Lock()
		ended := sa.Winner >= 0 || (sa.Status != SAStatusRoundOne && sa.Status != SAStatusRoundTwo)
		sa.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(SAMapID)
			min := s.bgPrematureMinPlayers(SAMapID)
			sa.mu.Lock()
			if sa.Winner < 0 && (sa.Status == SAStatusRoundOne || sa.Status == SAStatusRoundTwo) {
				expired, msg := prematureCountdownStep(&sa.PrematureCountDown, &sa.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(SAMapID, msg)
				}
				if expired {
					s.finishSAPremature(sa, bgBasePrematureWinner(ally, horde, min))
				}
			}
			sa.mu.Unlock()
		}
	}

	// IoC: no per-map override in C++, base roster leg only.
	s.icMu.RLock()
	ic := s.icState[ICMapID]
	s.icMu.RUnlock()
	if ic != nil {
		ic.mu.Lock()
		ended := ic.Winner >= 0
		ic.mu.Unlock()
		if !ended {
			ally, horde := s.bgRosterCounts(ICMapID)
			min := s.bgPrematureMinPlayers(ICMapID)
			ic.mu.Lock()
			if ic.Winner < 0 {
				expired, msg := prematureCountdownStep(&ic.PrematureCountDown, &ic.PrematureTimerMs, diffMs, ally < min || horde < min)
				if msg != "" {
					s.broadcastBattlegroundMessage(ICMapID, msg)
				}
				if expired {
					s.finishICPremature(ic, bgBasePrematureWinner(ally, horde, min))
				}
			}
			ic.mu.Unlock()
		}
	}
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
