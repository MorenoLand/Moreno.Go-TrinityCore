package world

import (
	"context"
	"database/sql"
	"errors"
	"math"
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

	// WorldSession::HandleBattlemasterHelloOpcode (BattleGroundHandler.cpp:41-45): the
	// creature must be interactable with UNIT_NPC_FLAG_BATTLEMASTER, or the hello is ignored.
	if !s.canInteractWithNPC(ctx, bmGUID, uint64(unitNPCFlagBattlemaster)) {
		return true
	}

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
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return false
	}
	fromWhere, _ := r.ReadU8()

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

	// The solo-only checks below (deserter, duplicate queue, free slots, freeze) live in the
	// !joinAsGroup arm of HandleBattlemasterJoinOpcode (BattleGroundHandler.cpp:131-193); the
	// group arm re-checks per member through Group::CanJoinBattlegroundQueue (Group.cpp:2024).
	if joinAsGroup != 0 {
		s.handleBattlemasterJoinGroup(bgTypeID, instanceID)
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

	s.bgQueues[slot] = bgQueueEntry{
		Active:     true,
		BgTypeID:   bgTypeID,
		InstanceID: instanceID,
		JoinTime:   time.Now(),
		Status:     BGStatusWaitQueue,
	}

	s.sendBattlefieldStatus(uint8(slot))
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

	// Member sessions in group order (leader first), mirroring the C++ GroupReference walk.
	members := make([]*session, 0, len(grp.Members))
	for _, m := range grp.Members {
		members = append(members, s.server.findSessionByGUID(m.GUID))
	}

	// CanJoinBattlegroundQueue per-member checks in C++ order (Group.cpp:2049-2087); the first
	// failing member decides err, which is then broadcast to every member.
	err := int32(bgTypeID) // success: positive values are indexes in BattlemasterList.dbc (SharedDefines.h:3689)
	leaderTeam := teamForRace(s.player.Race)
	for _, member := range members {
		switch {
		case member == nil || !member.playerLoaded || member.player == nil:
			// offline member → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2049-2051)
			err = groupJoinBattlegroundFailed
		case teamForRace(member.player.Race) != leaderTeam:
			// cross-faction → ERR_BATTLEGROUND_JOIN_TIMED_OUT (Group.cpp:2056-2058)
			err = groupJoinBattlegroundTimedOut
		case memberBGQueueIndex(member, bgTypeID) != -1:
			// member already in this queue → ERR_BATTLEGROUND_JOIN_FAILED (Group.cpp:2067-2068)
			err = groupJoinBattlegroundFailed
		case member.hasAura(deserterSpellBG):
			// deserter → ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS (Group.cpp:2076-2077)
			err = groupJoinBattlegroundDeserters
		case memberFreeBGQueueIndex(member) == -1:
			// no free slot → ERR_BATTLEGROUND_TOO_MANY_QUEUES (Group.cpp:2079-2080)
			err = groupJoinTooManyQueues
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
	for _, member := range members {
		slot := memberFreeBGQueueIndex(member)
		if slot == -1 {
			continue
		}
		member.bgQueues[slot] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, InstanceID: instanceID, JoinTime: time.Now(), Status: BGStatusWaitQueue}
		member.sendBattlefieldStatus(uint8(slot))
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

// handleBattlemasterJoinArena processes CMSG_BATTLEMASTER_JOIN_ARENA (0x358).
// Reference: WorldSession::HandleBattlemasterJoinArena (BattleGroundHandler.cpp:610).
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

	if isRated != 0 {
		// BattleGroundHandler.cpp:714-722 — a rated queue requires a real arena team:
		// GetArenaTeamId(arenaslot) + sArenaTeamMgr->GetArenaTeamById null →
		// SendNotInArenaTeamPacket(arenatype) + return. The single JOIN covers both the
		// missing-membership and the deleted-team cases (same query shape as
		// commands.go:3902, the sCharacterCache->GetCharacterArenaTeamIdByGuid mirror).
		if cdb := s.server.CharactersStore.DB; cdb != nil {
			var teamID uint32
			err := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", s.playerGUID, arenaType).Scan(&teamID)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return false
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
			s.debug("rated arena join", "account", s.accountName, "type", arenaType, "team", teamID)
		}
	}

	s.bgQueues[slot] = bgQueueEntry{
		Active:       true,
		BgTypeID:     battlegroundAA, // BATTLEGROUND_AA (All Arenas)
		InstanceID:   0,
		JoinTime:     time.Now(),
		Status:       BGStatusWaitQueue,
		ArenaType:    arenaType,
		IsArena:      true,
		IsRated:      isRated != 0,
		ArenaFaction: 0,
	}

	s.sendBattlefieldStatus(uint8(slot))
	s.debug("queued for arena", "account", s.accountName, "slot", slot, "type", arenaType, "rated", isRated != 0)
	return true
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
	_, _ = r.ReadU8() // type
	_, _ = r.ReadU8() // unk2
	bgTypeID, err := r.ReadU32()
	if err != nil {
		return false
	}
	_, _ = r.ReadU16() // unk
	action, _ := r.ReadU8()

	// Deserter demotion (BattleGroundHandler.cpp:429-439 — action==1 &&
	// ginfo.ArenaType==0 && _player->IsDeserter() == HasAura(26013) (Player.h:1913)
	// → BuildGroupJoinedBattlegroundPacket ERR_GROUP_JOIN_BATTLEGROUND_DESERTERS
	// and the accept demotes to leave; the shared leave arm below then clears the
	// slot, exactly as the C++ else branch does). The rest of the C++ action==1
	// accept path (BattleGroundHandler.cpp:448-501 — resurrect, taxi finish,
	// STATUS_IN_PROGRESS packet, queue removal, SendToBattleground teleport into
	// a live instance) has no Go counterpart: there is no BattlegroundMgr /
	// Battleground / queue world model to port the player to, so the port itself
	// is documented no-bridge rather than stubbed.
	if action == 1 {
		for i := 0; i < len(s.bgQueues); i++ {
			if s.bgQueues[i].Active && !s.bgQueues[i].IsArena && s.bgQueues[i].BgTypeID == bgTypeID && s.hasAura(deserterSpellBG) {
				buf := protocol.NewBuffer(4)
				buf.WriteI32(groupJoinBattlegroundDeserters)
				_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_JOINED_BATTLEGROUND), buf.Bytes(), true)
				s.debug("battlefield port accept demoted to leave: deserter debuff", "account", s.accountName, "bg", bgTypeID)
				action = 0
				break
			}
		}
	}

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
	case BGStatusWaitQueue:
		buf.WriteU32(120000) // average wait time ms
		timeInQueue := uint32(time.Since(entry.JoinTime).Milliseconds())
		buf.WriteU32(timeInQueue)
	case BGStatusWaitJoin:
		buf.WriteU32(entry.MapID)
		buf.WriteU64(0)
		buf.WriteU32(120000) // time to remove
	case BGStatusInProgress:
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
		s.bgQueues[index] = bgQueueEntry{Active: true, BgTypeID: bgTypeID, InstanceID: s.bgData.InstanceID, Status: BGStatusInProgress, ArenaType: arenaType, IsArena: isArena, MapID: state.Map, StartTime: time.Now(), ArenaFaction: uint8(s.bgData.Team)}
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
// and SPELL_AURA_MOD_FACTION_REPUTATION_GAIN aura modifiers first; Go has no
// reputation-gain aura model, so the flat giveReputation standing arm is the bridge.
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
	// C++ WorldSession::HandleBattlefieldLeaveOpcode (BattleGroundHandler.cpp:528)
	// denies leaving a battleground while in combat unless the BG is already in
	// STATUS_WAIT_LEAVE. Go's live-BG analog is the arena world model
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
	if s.server != nil {
		s.server.handleWSGPlayerLeave(s)
		s.server.handleEOTSPlayerLeave(s)
		s.server.handleSAPlayerLeave(s)
		s.server.handleICPlayerLeave(s)
		s.server.handleAVPlayerLeave(s)
		s.server.handleArenaPlayerLeave(s)
		s.server.handleWGPlayerLeave(s)
	}
	s.resetAchievementCriteriaByCondition(criteriaConditionBGMap, s.player.Map)
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
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, _ := r.ReadU64()

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
	now := time.Now()
	s.writeMu.Lock()
	if now.After(s.afkReportWindowEnd) {
		s.afkReportWindowEnd = now.Add(5 * time.Minute)
		s.afkReportedCount = 0
	}
	allowed := s.afkReportedCount < 15
	s.afkReportedCount++
	s.writeMu.Unlock()
	if !allowed {
		return true
	}

	// Player::ReportedAfkBy (Player.cpp:22531-22542): no duplicate reporters, and no
	// report against a target already carrying Idle (43680) or Inactive (43681). On
	// reaching CONFIG_BATTLEGROUND_REPORT_AFK reporters (default 3) the Idle debuff is
	// cast and the reporter set is cleared.
	targetSess.writeMu.Lock()
	if targetSess.afkReporters == nil {
		targetSess.afkReporters = make(map[uint64]struct{})
	}
	_, dup := targetSess.afkReporters[s.playerGUID]
	if !dup && !targetSess.hasAura(43680) && !targetSess.hasAura(43681) {
		targetSess.afkReporters[s.playerGUID] = struct{}{}
	}
	reportCount := len(targetSess.afkReporters)
	if reportCount >= 3 {
		targetSess.afkReporters = make(map[uint64]struct{})
	}
	targetSess.writeMu.Unlock()

	s.debug("reported player for pvp afk", "account", s.accountName, "target", targetGUID, "reports", reportCount)
	if reportCount >= 3 {
		targetSess.applyAura(43680) // Idle debuff
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
