package world

import (
	"context"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	memberFlagAssistant        uint8  = 0x01
	memberFlagMainTank         uint8  = 0x02
	memberFlagMainAssist       uint8  = 0x04
	groupTypeBattleground      uint8  = 0x01
	groupTypeRaid              uint8  = 0x02
	groupTypeBattlegroundRaid  uint8  = 0x03
	groupUpdateFlagPetCurPower uint32 = protocol.GroupUpdateFlagPetCurrentPower
)

// groupState holds all state for a 5-man or raid group.
// Mirrors TrinityCore's Group class (Groups/Group.h).
type groupState struct {
	ID            uint64
	DBID          uint32
	LFGState      uint8
	LeaderGUID    uint64
	Members       []groupMember // ordered; first entry is leader
	LootMethod    uint8         // 0=Free, 1=RR, 2=MasterLoot, 3=GroupLoot, 4=NeedBeforeGreed
	MasterLooter  uint64
	LootThreshold uint8  // item quality threshold (default 2 = uncommon)
	LooterGUID    uint64 // current round-robin looter GUID
	DungeonDiff   uint8
	RaidDiff      uint8
	GroupType     uint8
	IsRaid        bool
	IsLFG         bool
	LFGDungeonID  uint32
	TargetIcons   [8]uint64 // raid target icons, index=icon, value=target GUID
	counter       uint32
	// MaxEnchantingLevel mirrors Group::m_maxEnchantingLevel: the highest
	// enchanting skill among members. Group::GroupLoot/NeedBeforeGreed gate
	// the disenchant roll option on m_maxEnchantingLevel >=
	// item->RequiredDisenchantSkill (Group.cpp:1146/1296).
	MaxEnchantingLevel uint16
}

// dungeonMapID mirrors the DBC lookup behind (*session).isDungeonMap for
// callers that only hold the Server.
func dungeonMapID(srv *Server, mapID uint32) bool {
	if srv != nil && srv.Data != nil {
		if m, ok, err := srv.Data.Map(mapID); ok && err == nil {
			return m.IsDungeon()
		}
	}
	// Fallback when DBC is not loaded (continents: 0 Eastern Kingdoms, 1 Kalimdor, 530 Outland, 571 Northrend)
	return mapID != 0 && mapID != 1 && mapID != 530 && mapID != 571
}

// updateLooter mirrors Group::UpdateLooterGuid (Group.cpp:1962) with
// ifneed=true: the current looter keeps the role while they are still at
// Player::IsAtGroupRewardDistance (Player.cpp:24160) of the looted object
// (same map and instance, dungeon-always, else within MaxGroupXPDistance /
// CONFIG_GROUP_XP_DISTANCE, default 74) — the role advances to the next
// in-range member only when the current looter left range or is gone, and
// clears when nobody is in range (Group.cpp:2017-2020). FREE_FOR_ALL never
// rotates (Group.cpp:1965-1966). Timing delta: C++ runs this at kill
// (Unit.cpp:11224) while Go runs it at loot open, so a corpse that is never
// opened never advances the role; like the rest of the group state, the
// role is kept in memory only (no groups.looterGuid write).
func (g *groupState) updateLooter(srv *Server, mapID, instanceID uint32, x, y, z float32) {
	if g.LootMethod == 0 || len(g.Members) == 0 {
		return
	}
	atRewardDistance := func(sess *session) bool {
		return sess != nil && sess.player != nil && sess.player.Map == mapID &&
			sess.player.InstanceID == instanceID &&
			(dungeonMapID(srv, mapID) || distance3D(sess.player.X, sess.player.Y, sess.player.Z, x, y, z) <= srv.Config.MaxGroupXPDistance)
	}
	// ifneed arm (Group.cpp:1972-1978): keep the current looter.
	if atRewardDistance(srv.findSessionByGUID(g.LooterGUID)) {
		return
	}
	currIdx := -1
	for i, m := range g.Members {
		if m.GUID == g.LooterGUID {
			currIdx = i
			break
		}
	}
	for step := 1; step <= len(g.Members); step++ {
		idx := (currIdx + step) % len(g.Members)
		mGUID := g.Members[idx].GUID
		if atRewardDistance(srv.findSessionByGUID(mGUID)) {
			g.LooterGUID = mGUID
			return
		}
	}
	g.LooterGUID = 0
}

func (g *groupState) isLeader(guid uint64) bool {
	return g.LeaderGUID == guid
}

// refreshGroupMaxEnchantingLevel mirrors the m_maxEnchantingLevel maintenance
// in Group::AddMember (Group.cpp:556-557, raised on join) and
// Group::RemoveMember (Group.cpp:2411-2417, recomputed over members on leave).
func refreshGroupMaxEnchantingLevel(srv *Server, g *groupState) {
	var maxLvl uint16
	for _, m := range g.Members {
		if sess := srv.findSessionByGUID(m.GUID); sess != nil && sess.player != nil {
			if v := playerSkillTotalValue(sess.player, skillEnchanting); v > int32(maxLvl) {
				maxLvl = uint16(v)
			}
		}
	}
	g.MaxEnchantingLevel = maxLvl
}

func UpdatePlayerGroupLeaderFlag(flags uint32, leader bool) uint32 {
	if leader {
		return flags | playerFlagGroupLeader
	}
	return flags &^ playerFlagGroupLeader
}

func (g *groupState) isAssistant(guid uint64) bool {
	for _, m := range g.Members {
		if m.GUID == guid && (m.Flags&memberFlagAssistant != 0) {
			return true
		}
	}
	return false
}

func (g *groupState) isLeaderOrAssistant(guid uint64) bool {
	return g.isLeader(guid) || g.isAssistant(guid)
}

func (g *groupState) memberSubGroup(guid uint64) (uint8, bool) {
	for _, m := range g.Members {
		if m.GUID == guid {
			return m.SubGroup, true
		}
	}
	return 0, false
}

func (g *groupState) countInSubGroup(subGroup uint8) int {
	cnt := 0
	for _, m := range g.Members {
		if m.SubGroup == subGroup {
			cnt++
		}
	}
	return cnt
}

// groupMember mirrors Group::MemberSlot.
type groupMember struct {
	GUID     uint64
	Name     string
	SubGroup uint8 // 0-7 for raids, always 0 for 5-man
	Flags    uint8 // member flags (assistant etc)
	Roles    uint8 // LFG roles (unused at group level)
}

func (s *session) loadPlayerGroup(ctx context.Context, guid uint64) {
	if s != nil && s.player != nil {
		s.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(s.player.PlayerFlags, false)
	}
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	db := s.server.CharactersStore.DB
	var dbGroupID int64
	if err := db.QueryRowContext(ctx, "SELECT guid FROM group_member WHERE memberGuid = ? LIMIT 1", guid).Scan(&dbGroupID); err != nil {
		return
	}
	if group := s.server.findGroupByDBID(uint64(dbGroupID)); group != nil {
		s.groupID = group.ID
		if s.player != nil {
			s.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(s.player.PlayerFlags, group.isLeader(guid))
			s.player.DungeonDifficulty = group.DungeonDiff
			s.player.RaidDifficulty = group.RaidDiff
		}
		return
	}
	var leaderGUID, lootMethod, looterGUID, lootThreshold, groupType, dungeonDiff, raidDiff, masterLooterGUID int64
	var icons [8]int64
	err := db.QueryRowContext(ctx, "SELECT leaderGuid, lootMethod, looterGuid, lootThreshold, icon1, icon2, icon3, icon4, icon5, icon6, icon7, icon8, groupType, difficulty, raidDifficulty, masterLooterGuid FROM `groups` WHERE guid = ?", dbGroupID).Scan(&leaderGUID, &lootMethod, &looterGUID, &lootThreshold, &icons[0], &icons[1], &icons[2], &icons[3], &icons[4], &icons[5], &icons[6], &icons[7], &groupType, &dungeonDiff, &raidDiff, &masterLooterGUID)
	if err != nil {
		return
	}
	if dungeonDiff < 0 || dungeonDiff >= 3 {
		dungeonDiff = 0
	}
	if raidDiff < 0 || raidDiff >= 4 {
		raidDiff = 0
	}
	var lfgDungeonID, lfgState int64
	if uint8(groupType)&0x08 != 0 {
		_ = db.QueryRowContext(ctx, "SELECT dungeon, state FROM lfg_data WHERE guid = ?", dbGroupID).Scan(&lfgDungeonID, &lfgState)
	}
	rows, err := db.QueryContext(ctx, "SELECT gm.memberGuid, gm.memberFlags, gm.subgroup, gm.roles, c.name FROM group_member gm JOIN characters c ON c.guid = gm.memberGuid WHERE gm.guid = ? ORDER BY gm.memberGuid", dbGroupID)
	if err != nil {
		return
	}
	members := make([]groupMember, 0, maxGroupSize)
	for rows.Next() {
		var memberGUID, memberFlags, subgroup, roles int64
		var name string
		if err := rows.Scan(&memberGUID, &memberFlags, &subgroup, &roles, &name); err != nil || name == "" {
			continue
		}
		members = append(members, groupMember{GUID: uint64(memberGUID), Name: name, SubGroup: uint8(subgroup), Flags: uint8(memberFlags), Roles: uint8(roles)})
	}
	rows.Close()
	if len(members) < 2 {
		return
	}
	runtimeID := int64(0)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups AS g WHERE g.guid <= ? AND g.guid IN (SELECT gm.guid FROM group_member gm JOIN characters c ON c.guid = gm.memberGuid GROUP BY gm.guid HAVING COUNT(*) > 1)`, dbGroupID).Scan(&runtimeID)
	if runtimeID <= 0 {
		runtimeID = int64(newGroupID())
	} else {
		reserveGroupID(uint64(runtimeID))
	}
	g := &groupState{ID: uint64(runtimeID), DBID: uint32(dbGroupID), LFGState: uint8(lfgState), LeaderGUID: uint64(leaderGUID), Members: members, LootMethod: uint8(lootMethod), LooterGUID: uint64(looterGUID), LootThreshold: uint8(lootThreshold), MasterLooter: uint64(masterLooterGUID), DungeonDiff: uint8(dungeonDiff), RaidDiff: uint8(raidDiff), GroupType: uint8(groupType), IsRaid: uint8(groupType)&0x02 != 0, IsLFG: uint8(groupType)&0x08 != 0, LFGDungeonID: uint32(lfgDungeonID)}
	for index, icon := range icons {
		g.TargetIcons[index] = uint64(icon)
	}
	s.server.groupsMu.Lock()
	if s.server.groups == nil {
		s.server.groups = make(map[uint64]*groupState)
	}
	s.server.groups[g.ID] = g
	s.server.groupsMu.Unlock()
	s.groupID = g.ID
	if s.player != nil {
		s.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(s.player.PlayerFlags, g.isLeader(guid))
		s.player.DungeonDifficulty = g.DungeonDiff
		s.player.RaidDifficulty = g.RaidDiff
	}
}

func (s *session) sendLoadedGroup() {
	if s == nil || s.server == nil || s.groupID == 0 {
		return
	}
	if group := s.server.findGroupByID(s.groupID); group != nil {
		s.server.broadcastGroupList(group)
	}
}

// groupNextID is a monotonic group ID counter.
var groupNextID uint64 = 1

func newGroupID() uint64 {
	return atomic.AddUint64(&groupNextID, 1) - 1
}

func reserveGroupID(id uint64) {
	for {
		next := atomic.LoadUint64(&groupNextID)
		if next > id {
			return
		}
		if atomic.CompareAndSwapUint64(&groupNextID, next, id+1) {
			return
		}
	}
}

// PartyOperation enum, mirrors TrinityCore's PartyOperation.
const (
	partyOpInvite   uint32 = 0
	partyOpUninvite uint32 = 1
	partyOpLeave    uint32 = 2
)

// PartyResult enum, mirrors TrinityCore's PartyResult.
const (
	errPartyResultOK        uint32 = 0
	errBadPlayerNameS       uint32 = 1
	errTargetNotInGroup     uint32 = 2
	errGroupFull            uint32 = 3
	errAlreadyInGroupS      uint32 = 4
	errNotLeader            uint32 = 5
	errPlayerWrongFaction   uint32 = 7
	errIgnoringYouS         uint32 = 8
	errTargetNotInInstanceS uint32 = 11
	errInviteRestricted     uint32 = 13
)

// MaxGroupSize (5-man). Raids can have up to 40.
const maxGroupSize = 5
const maxRaidSize = 40

// -----------------------------------------------------------------
// Wire helpers
// -----------------------------------------------------------------

// buildPartyCommandResult mirrors WorldSession::SendPartyResult.
// SMSG_PARTY_COMMAND_RESULT: uint32 operation, cstring member, uint32 result, uint32 val
func buildPartyCommandResult(operation uint32, member string, result uint32) []byte {
	b := protocol.NewBuffer(4 + len(member) + 1 + 4 + 4)
	b.WriteU32(operation)
	b.WriteCString(member)
	b.WriteU32(result)
	b.WriteU32(0) // LFD cooldown val
	return b.Bytes()
}

// buildGroupList sends SMSG_GROUP_LIST to a specific member, excluding themselves.
// Mirrors Group::SendUpdate (Group.cpp:1755).
// groupType: 0=party, 1=BG, 2=raid
func buildGroupList(srv *Server, g *groupState, forGUID uint64, counter uint32) []byte {
	// Find the member slot for the recipient.
	var slot *groupMember
	for i := range g.Members {
		if g.Members[i].GUID == forGUID {
			slot = &g.Members[i]
			break
		}
	}
	membersCount := len(g.Members) - 1 // exclude self
	if membersCount < 0 {
		membersCount = 0
	}

	groupType := g.GroupType
	if groupType == 0 && g.IsRaid {
		groupType = groupTypeRaid
	}
	if g.IsLFG {
		groupType |= 0x08
	}

	subGroup := uint8(0)
	flags := uint8(0)
	roles := uint8(0)
	if slot != nil {
		subGroup = slot.SubGroup
		flags = slot.Flags
		roles = slot.Roles
	}

	b := protocol.NewBuffer(64 + membersCount*24)
	b.WriteU8(groupType)
	b.WriteU8(subGroup)
	b.WriteU8(flags)
	b.WriteU8(roles)
	if groupType&0x08 != 0 {
		status := uint8(0)
		if g.LFGState == LFGStateFinishedDungeon {
			status = 2
		}
		b.WriteU8(status)
		b.WriteU32(g.LFGDungeonID)
	}
	b.WriteU64(groupGUID(g.ID))
	b.WriteU32(counter)
	b.WriteU32(uint32(membersCount))
	for _, m := range g.Members {
		if m.GUID == forGUID {
			continue
		}
		b.WriteCString(m.Name)
		b.WriteU64(m.GUID)
		status := uint8(0)
		if sess := srv.findSessionByGUID(m.GUID); sess != nil && sess.worldReady.Load() && sess.logoutAt.IsZero() {
			status = 1
		}
		if groupType == groupTypeBattleground || groupType == groupTypeBattlegroundRaid {
			status |= 0x02
		}
		b.WriteU8(status)
		b.WriteU8(m.SubGroup)
		b.WriteU8(m.Flags)
		b.WriteU8(m.Roles)
	}
	b.WriteU64(g.LeaderGUID)
	if membersCount > 0 {
		b.WriteU8(g.LootMethod)
		if g.LootMethod == 2 {
			b.WriteU64(g.MasterLooter)
		} else {
			b.WriteU64(0)
		}
		b.WriteU8(g.LootThreshold)
		b.WriteU8(g.DungeonDiff)
		b.WriteU8(g.RaidDiff)
		if g.RaidDiff >= 2 {
			b.WriteU8(1)
		} else {
			b.WriteU8(0)
		}
	}
	return b.Bytes()
}

func groupGUID(low uint64) uint64 { return low | (uint64(0x1F50) << 48) }

// buildGroupInvite builds SMSG_GROUP_INVITE sent to the invited player.
// flag: 1 = valid invite, 0 = already in group notification.
// Mirrors GroupHandler.cpp:149 and GroupHandler.cpp:207.
func buildGroupInvite(flag uint8, inviterName string) []byte {
	b := protocol.NewBuffer(2 + len(inviterName) + 8)
	b.WriteU8(flag)
	b.WriteCString(inviterName)
	b.WriteU32(0) // unk
	b.WriteU8(0)  // count
	b.WriteU32(0) // unk
	return b.Bytes()
}

// -----------------------------------------------------------------
// Server helpers
// -----------------------------------------------------------------

func (s *Server) findGroupByID(id uint64) *groupState {
	s.groupsMu.RLock()
	defer s.groupsMu.RUnlock()
	return s.groups[id]
}

func (s *Server) sendGroupPetCurrentPower(ownerGUID uint64, current uint32) {
	if s == nil || ownerGUID == 0 {
		return
	}
	owner := s.findSessionByGUID(ownerGUID)
	if owner == nil || owner.player == nil || owner.groupID == 0 {
		return
	}
	s.groupsMu.RLock()
	group := s.groups[owner.groupID]
	if group == nil {
		s.groupsMu.RUnlock()
		return
	}
	members := append([]groupMember(nil), group.Members...)
	s.groupsMu.RUnlock()
	packet := protocol.NewBuffer(24)
	packet.WritePackedGUID(ownerGUID)
	packet.WriteU32(groupUpdateFlagPetCurPower)
	packet.WriteU16(uint16(current))
	for _, member := range members {
		if member.GUID == ownerGUID {
			continue
		}
		target := s.findSessionByGUID(member.GUID)
		if target == nil || target.player == nil || target.player.Map == owner.player.Map && distance3D(target.player.X, target.player.Y, target.player.Z, owner.player.X, owner.player.Y, owner.player.Z) <= 100.0 {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_PARTY_MEMBER_STATS), packet.Bytes(), true)
	}
}

func (s *Server) findGroupByDBID(id uint64) *groupState {
	s.groupsMu.RLock()
	defer s.groupsMu.RUnlock()
	for _, group := range s.groups {
		if group != nil && uint64(group.DBID) == id {
			return group
		}
	}
	return nil
}

func (s *Server) getGroup(id uint64) *groupState {
	return s.findGroupByID(id)
}

func (s *Server) removeSessionFromGroup(member *session) {
	if s == nil || member == nil || member.groupID == 0 {
		return
	}
	g := s.findGroupByID(member.groupID)
	if g == nil {
		member.groupID = 0
		member.pendingGroupLeader = 0
		return
	}
	s.groupsMu.Lock()
	groupObj := groupLuaObject(g)
	index := -1
	for i, value := range g.Members {
		if value.GUID == member.playerGUID {
			index = i
			break
		}
	}
	if index >= 0 {
		g.Members = append(g.Members[:index], g.Members[index+1:]...)
	}
	member.groupID = 0
	member.pendingGroupLeader = 0
	s.onPlayerLeaveGroupRolls(member.playerGUID, g.ID)
	disbanded := len(g.Members) == 0
	if disbanded {
		delete(s.groups, g.ID)
		s.groupsMu.Unlock()
		// C++ Group::RemoveMember fires OnGroupRemoveMember (Group.cpp:569)
		// and disbands a 1-or-fewer-member group, which fires OnGroupDisband.
		s.triggerGroupEvent(scripting.GroupEventOnMemberRemove, groupObj, member.playerGUID, uint8(groupRemoveMethodDefault))
		s.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
		return
	}
	// C++ Group::RemoveMember picks a new leader via ChangeLeader when the
	// removed member was the leader, which fires OnGroupChangeLeader
	// (Group.cpp:765).
	oldLeader := g.LeaderGUID
	var newLeader uint64
	leaderChanged := false
	if g.LeaderGUID == member.playerGUID {
		newLeader = g.Members[0].GUID
		g.LeaderGUID = newLeader
		leaderChanged = true
	}
	s.groupsMu.Unlock()
	s.triggerGroupEvent(scripting.GroupEventOnMemberRemove, groupObj, member.playerGUID, uint8(groupRemoveMethodDefault))
	if leaderChanged {
		s.triggerGroupEvent(scripting.GroupEventOnLeaderChange, groupObj, newLeader, oldLeader)
	}
	s.broadcastGroupList(g)
}

func (s *Server) broadcastGroupList(g *groupState) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.groupID == g.ID && sess.worldReady.Load() {
			counter := atomic.AddUint32(&g.counter, 1) - 1
			pkt := buildGroupList(s, g, sess.playerGUID, counter)
			_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), pkt, true)
		}
	}
}

func (s *Server) broadcastToGroup(groupID uint64, opcode uint16, payload []byte) {
	if groupID == 0 {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.groupID == groupID && sess.worldReady.Load() {
			_ = sess.write(opcode, payload, true)
		}
	}
}

func (s *Server) getGroupSessions(groupID uint64) []*session {
	if groupID == 0 {
		return nil
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	var list []*session
	for sess := range s.sessions {
		if sess.groupID == groupID {
			list = append(list, sess)
		}
	}
	return list
}

// -----------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------

// handleGroupInvite processes CMSG_GROUP_INVITE (0x06E).
// TrinityCore: WorldSession::HandleGroupInviteOpcode.
func (s *session) handleGroupInvite(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	memberName, err := r.ReadCString()
	if err != nil || memberName == "" {
		return s.sendPartyResult(partyOpInvite, memberName, errBadPlayerNameS)
	}
	// skip uint32 unk
	_, _ = r.ReadU32()

	// Can't invite yourself
	if toLower(memberName) == toLower(s.player.Name) {
		return s.sendPartyResult(partyOpInvite, memberName, errBadPlayerNameS)
	}

	invitedSess := s.server.findSessionByName(memberName)
	if invitedSess == nil || invitedSess.player == nil {
		return s.sendPartyResult(partyOpInvite, memberName, errBadPlayerNameS)
	}

	// Restrict invite to GMs (GroupHandler.cpp:103 — GM.AllowInvite, default false)
	if !s.server.Config.GMAllowInvite && s.security == 0 && invitedSess.security > 0 {
		return s.sendPartyResult(partyOpInvite, memberName, errBadPlayerNameS)
	}

	// Can't group with the opposite faction (AllowTwoSide.Interaction.Group, default false)
	if s.security == 0 && !s.server.Config.AllowTwoSideInteractionGroup &&
		playerTeam(s.player.Race) != playerTeam(invitedSess.player.Race) {
		return s.sendPartyResult(partyOpInvite, memberName, errPlayerWrongFaction)
	}

	// Both inside different instances of the same map
	if s.player.InstanceID != 0 && invitedSess.player.InstanceID != 0 &&
		s.player.InstanceID != invitedSess.player.InstanceID && s.player.Map == invitedSess.player.Map {
		return s.sendPartyResult(partyOpInvite, memberName, errTargetNotInInstanceS)
	}

	// The invited player ignored the inviter
	if s.server.chatIgnoredBy(invitedSess.playerGUID, s.playerGUID) {
		return s.sendPartyResult(partyOpInvite, memberName, errIgnoringYouS)
	}

	// Party level requirement (World.cpp:680, default 1), waived when the
	// invited player lists the inviter as a friend
	if !s.server.socialHasFriend(invitedSess.playerGUID, s.playerGUID) &&
		s.server.Config.PartyLevelReq > 0 && uint32(s.player.Level) < s.server.Config.PartyLevelReq {
		return s.sendPartyResult(partyOpInvite, memberName, errInviteRestricted)
	}

	// Invited player already in a group or has a pending invite
	if invitedSess.groupID != 0 || invitedSess.pendingGroupLeader != 0 {
		_ = s.sendPartyResult(partyOpInvite, memberName, errAlreadyInGroupS)
		if invitedSess.groupID != 0 {
			_ = invitedSess.write(uint16(protocol.OpcodeSMSG_GROUP_INVITE), buildGroupInvite(0, s.player.Name), true)
		}
		return true
	}

	// Inviting player must be leader or assistant if already in a group
	if s.groupID != 0 {
		g := s.server.findGroupByID(s.groupID)
		if g == nil {
			s.groupID = 0
		} else if !g.isLeaderOrAssistant(s.playerGUID) {
			return s.sendPartyResult(partyOpInvite, "", errNotLeader)
		} else if len(g.Members) >= maxGroupSize {
			return s.sendPartyResult(partyOpInvite, "", errGroupFull)
		}
	}

	// Set the pending invite on the target player
	invitedSess.pendingGroupLeader = s.playerGUID

	// Eluna GROUP_EVENT_ON_MEMBER_INVITE (2): C++ Group::AddInvite fires
	// OnGroupInviteMember (Group.cpp:351). When the inviter has no group yet,
	// C++ builds an ephemeral Group and fires once for the leader (via
	// AddLeaderInvite) and once for the invitee; with an existing group it
	// fires only for the invitee.
	if s.groupID != 0 {
		if g := s.server.findGroupByID(s.groupID); g != nil {
			s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, groupLuaObject(g), invitedSess.playerGUID)
		}
	} else {
		ephemeral := scripting.NewGroupObject(groupGUID(0), s.playerGUID, []uint64{s.playerGUID})
		s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, ephemeral, s.playerGUID)
		s.server.triggerGroupEvent(scripting.GroupEventOnMemberInvite, ephemeral, invitedSess.playerGUID)
		// LFGGroupScript::OnInviteMember (LFGScripts.cpp:231-245): a queued
		// leader forming a new group via invite leaves the LFG queue.
		if s.server.Features != nil && s.server.Features.LFG != nil &&
			s.server.Features.LFG.Leave(s.playerGUID) {
			_ = s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone})
		}
	}

	// Send SMSG_GROUP_INVITE to invited player
	_ = invitedSess.write(uint16(protocol.OpcodeSMSG_GROUP_INVITE), buildGroupInvite(1, s.player.Name), true)
	// Tell inviter that invite was sent OK
	return s.sendPartyResult(partyOpInvite, memberName, errPartyResultOK)
}

// handleGroupAccept processes CMSG_GROUP_ACCEPT (0x072).
// TrinityCore: WorldSession::HandleGroupAcceptOpcode.
func (s *session) handleGroupAccept(_ context.Context, _ []byte) bool {
	if !s.playerLoaded || s.player == nil || s.pendingGroupLeader == 0 {
		return false
	}
	leaderGUID := s.pendingGroupLeader
	s.pendingGroupLeader = 0

	// Can't accept your own invite
	if leaderGUID == s.playerGUID {
		return false
	}

	leaderSess := s.server.findSessionByGUID(leaderGUID)

	srv := s.server
	srv.groupsMu.Lock()

	var g *groupState
	if leaderSess != nil && leaderSess.groupID != 0 {
		g = srv.groups[leaderSess.groupID]
	}

	if g == nil {
		// Forming a new group needs the leader present; joining an
		// existing group does not (HandleGroupAcceptOpcode)
		if leaderSess == nil || leaderSess.player == nil {
			srv.groupsMu.Unlock()
			return false
		}
		// Create new group
		g = &groupState{
			ID:            newGroupID(),
			DBID:          0,
			LeaderGUID:    leaderGUID,
			LootMethod:    3, // Group Loot default in retail / TrinityCore
			LootThreshold: 2, // uncommon
			LooterGUID:    leaderGUID,
			DungeonDiff:   1,
			RaidDiff:      1,
		}
		g.Members = append(g.Members, groupMember{GUID: leaderGUID, Name: leaderSess.player.Name})
		refreshGroupMaxEnchantingLevel(srv, g)
		srv.groups[g.ID] = g
		leaderSess.groupID = g.ID
	}

	if len(g.Members) >= maxGroupSize {
		srv.groupsMu.Unlock()
		_ = s.sendPartyResult(partyOpInvite, "", errGroupFull)
		return false
	}

	g.Members = append(g.Members, groupMember{GUID: s.playerGUID, Name: s.player.Name})
	refreshGroupMaxEnchantingLevel(srv, g)
	s.groupID = g.ID
	groupObj := groupLuaObject(g)
	srv.groupsMu.Unlock()

	// LFGGroupScript::OnAddMember (LFGScripts.cpp:143-181): a queued player
	// joining a group leaves the LFG queue. The per-group bookkeeping
	// (SetGroup/AddPlayerToGroup/SetLeader) has no Go counterpart — Go keeps
	// no per-group LFG data.
	if srv.Features != nil && srv.Features.LFG != nil &&
		srv.Features.LFG.Leave(s.playerGUID) {
		_ = s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone})
	}

	srv.broadcastGroupList(g)
	// Eluna GROUP_EVENT_ON_MEMBER_ADD (1): C++ Group::AddMember fires
	// OnGroupAddMember after SendUpdate (Group.cpp:477). The leader joined via
	// AddLeaderInvite at invite time and never passes through AddMember, so
	// only the accepting member fires here.
	srv.triggerGroupEvent(scripting.GroupEventOnMemberAdd, groupObj, s.playerGUID)
	return true
}

// handleGroupDecline processes CMSG_GROUP_DECLINE (0x073).
// TrinityCore: WorldSession::HandleGroupDeclineOpcode.
func (s *session) handleGroupDecline(_ context.Context, _ []byte) bool {
	if !s.playerLoaded || s.pendingGroupLeader == 0 {
		return false
	}
	leaderGUID := s.pendingGroupLeader
	s.pendingGroupLeader = 0

	leaderSess := s.server.findSessionByGUID(leaderGUID)
	if leaderSess == nil || leaderSess.player == nil {
		return false
	}
	// SMSG_GROUP_DECLINE: player name cstring
	name := ""
	if s.player != nil {
		name = s.player.Name
	}
	b := protocol.NewBuffer(len(name) + 1)
	b.WriteCString(name)
	_ = leaderSess.write(uint16(protocol.OpcodeSMSG_GROUP_DECLINE), b.Bytes(), true)
	return true
}

// handleGroupUninvite processes CMSG_GROUP_UNINVITE (0x075) - by name.
// TrinityCore: WorldSession::HandleGroupUninviteOpcode.
func (s *session) handleGroupUninvite(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	name, err := r.ReadCString()
	if err != nil || toLower(name) == toLower(s.player.Name) {
		return false
	}

	g := s.server.findGroupByID(s.groupID)
	if g == nil || !g.isLeaderOrAssistant(s.playerGUID) {
		return s.sendPartyResult(partyOpUninvite, "", errNotLeader)
	}

	target := s.server.findSessionByName(name)
	if target == nil {
		return s.sendPartyResult(partyOpUninvite, name, errTargetNotInGroup)
	}
	if target.playerGUID == g.LeaderGUID {
		return s.sendPartyResult(partyOpUninvite, "", errNotLeader)
	}
	// A pending invite issued by this leader can be withdrawn
	if target.pendingGroupLeader == s.playerGUID {
		target.pendingGroupLeader = 0
		return true
	}
	return s.removeFromGroup(g, target, groupRemoveMethodKick)
}

// handleGroupUninviteGUID processes CMSG_GROUP_UNINVITE_GUID (0x076).
// TrinityCore: WorldSession::HandleGroupUninviteGuidOpcode.
func (s *session) handleGroupUninviteGUID(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil || guid == s.playerGUID {
		return false
	}

	g := s.server.findGroupByID(s.groupID)
	if g == nil || !g.isLeaderOrAssistant(s.playerGUID) {
		return s.sendPartyResult(partyOpUninvite, "", errNotLeader)
	}

	target := s.server.findSessionByGUID(guid)
	if target == nil {
		return s.sendPartyResult(partyOpUninvite, "", errTargetNotInGroup)
	}
	if target.playerGUID == g.LeaderGUID {
		return s.sendPartyResult(partyOpUninvite, "", errNotLeader)
	}
	// A pending invite issued by this leader can be withdrawn
	if target.pendingGroupLeader == s.playerGUID {
		target.pendingGroupLeader = 0
		return true
	}
	return s.removeFromGroup(g, target, groupRemoveMethodKick)
}

// removeFromGroup removes target from the group, dissolving if the group
// goes to 1 member. method is the Eluna-visible RemoveMethod for
// GROUP_EVENT_ON_MEMBER_REMOVE (SharedDefines.h:3640-3643):
// GROUP_REMOVEMETHOD_KICK from the uninvite handlers (GroupHandler.cpp:323
// and :368 via Player::RemoveFromGroup), GROUP_REMOVEMETHOD_DEFAULT from the
// GM .group remove command (cs_group.cpp:284). C++ Group::RemoveMember fires
// OnGroupRemoveMember before the removal (Group.cpp:569) and OnGroupDisband
// when its tail disbands the group; the Lua group object is snapshotted at
// the C++ fire point and the hooks fire after the lock is released.
func (s *session) removeFromGroup(g *groupState, target *session, method uint8) bool {
	srv := s.server
	srv.groupsMu.Lock()
	groupObj := groupLuaObject(g)
	// Remove the target member
	for i, m := range g.Members {
		if m.GUID == target.playerGUID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	refreshGroupMaxEnchantingLevel(srv, g)
	target.groupID = 0
	target.pendingGroupLeader = 0
	srv.onPlayerLeaveGroupRolls(target.playerGUID, g.ID)

	// Send SMSG_GROUP_UNINVITE to the kicked player
	_ = target.write(uint16(protocol.OpcodeSMSG_GROUP_UNINVITE), nil, true)

	disbanded := len(g.Members) <= 1
	if disbanded {
		// Disband the group
		delete(srv.groups, g.ID)
		if len(g.Members) == 1 {
			if last := srv.findSessionByGUID(g.Members[0].GUID); last != nil {
				last.groupID = 0
				// SMSG_GROUP_DESTROYED
				_ = last.write(uint16(protocol.OpcodeSMSG_GROUP_DESTROYED), nil, true)
				// Also send empty group list to clear UI
				emptyList := buildGroupList(srv, &groupState{ID: g.ID, LeaderGUID: g.Members[0].GUID}, g.Members[0].GUID, 0)
				_ = last.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), emptyList, true)
			}
		}
	}
	srv.groupsMu.Unlock()

	// LFGGroupScript::OnRemoveMember native arms (LFGScripts.cpp:183-228).
	srv.lfgGroupMemberRemoved(g, target, method)

	srv.triggerGroupEvent(scripting.GroupEventOnMemberRemove, groupObj, target.playerGUID, method)
	if disbanded {
		srv.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
		return true
	}
	srv.broadcastGroupList(g)
	return true
}

// removeGroupMemberByGUID removes a member by guid, mirroring
// Group::RemoveMember (Group.cpp:564): online members ride the normal
// session detach path, offline members are dropped from the slot list.
// CHAR_DEL_GROUP_MEMBER is the C++ _removeMember DB persist.
func (s *Server) removeGroupMemberByGUID(ctx context.Context, g *groupState, guid uint64) {
	if sess := s.findSessionByGUID(guid); sess != nil {
		// GM .group remove: cs_group.cpp:284 calls RemoveMember with the
		// default method (GROUP_REMOVEMETHOD_DEFAULT).
		sess.removeFromGroup(g, sess, groupRemoveMethodDefault)
		if s.CharactersStore != nil && s.CharactersStore.DB != nil {
			_, _ = s.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_DEL_GROUP_MEMBER"), guid)
		}
		return
	}
	s.groupsMu.Lock()
	groupObj := groupLuaObject(g)
	idx := -1
	for i, m := range g.Members {
		if m.GUID == guid {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.groupsMu.Unlock()
		return
	}
	g.Members = append(g.Members[:idx], g.Members[idx+1:]...)
	refreshGroupMaxEnchantingLevel(s, g)
	disbanded := len(g.Members) <= 1
	if disbanded {
		// Dissolve like Group::RemoveMember does for a 1-member group.
		delete(s.groups, g.ID)
	}
	s.groupsMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_DEL_GROUP_MEMBER"), guid)
	}
	s.triggerGroupEvent(scripting.GroupEventOnMemberRemove, groupObj, guid, uint8(groupRemoveMethodDefault))
	if disbanded {
		s.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
		return
	}
	s.broadcastGroupList(g)
}

// handleGroupSetLeader processes CMSG_GROUP_SET_LEADER (0x078).
// TrinityCore: WorldSession::HandleGroupSetLeaderOpcode.
func (s *session) handleGroupSetLeader(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil {
		return false
	}

	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil || g.LeaderGUID != s.playerGUID {
		srv.groupsMu.Unlock()
		return false
	}
	srv.groupsMu.Unlock()
	return srv.setGroupLeader(g, guid)
}

// setGroupLeader changes the group leader, moving the new leader to the front
// of the member list, and broadcasts SMSG_GROUP_SET_LEADER plus the refreshed
// group list. The new leader must be a member (and, mirroring
// Group::ChangeLeader in Group.cpp:752, online). The instance-binding rewrite
// in ChangeLeader has no Go bridge.
func (s *Server) setGroupLeader(g *groupState, guid uint64) bool {
	s.groupsMu.Lock()
	// Check target is a member
	found := false
	for _, m := range g.Members {
		if m.GUID == guid {
			found = true
			break
		}
	}
	// ChangeLeader refuses offline players
	newLeader := s.findSessionByGUID(guid)
	if !found || newLeader == nil || newLeader.player == nil {
		s.groupsMu.Unlock()
		return false
	}
	groupObj := groupLuaObject(g)
	oldLeader := g.LeaderGUID
	g.LeaderGUID = guid

	// Move new leader to front of members list
	for i, m := range g.Members {
		if m.GUID == guid {
			g.Members[0], g.Members[i] = g.Members[i], g.Members[0]
			break
		}
	}
	s.groupsMu.Unlock()

	// Eluna GROUP_EVENT_ON_LEADER_CHANGE (4): C++ Group::ChangeLeader fires
	// OnGroupChangeLeader(newLeaderGuid, m_leaderGuid) after the member-slot
	// and offline checks (Group.cpp:765).
	s.triggerGroupEvent(scripting.GroupEventOnLeaderChange, groupObj, guid, oldLeader)

	// SMSG_GROUP_SET_LEADER: cstring name
	name := newLeader.player.Name
	b := protocol.NewBuffer(len(name) + 1)
	b.WriteCString(name)
	pkt := b.Bytes()
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.groupID == g.ID {
			_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_SET_LEADER), pkt, true)
		}
	}
	s.sessionsMu.RUnlock()
	s.broadcastGroupList(g)
	return true
}

// disbandGroup dissolves the whole group: every online member session is
// detached and notified, the persisted member/leader rows are removed, and
// the in-memory group is dropped. Mirrors Group::Disband as used by
// HandleGroupDisbandCommand (cs_group.cpp:246).
func (s *Server) disbandGroup(ctx context.Context, g *groupState) {
	s.groupsMu.Lock()
	groupObj := groupLuaObject(g)
	members := make([]uint64, len(g.Members))
	for i, m := range g.Members {
		members[i] = m.GUID
	}
	delete(s.groups, g.ID)
	s.groupsMu.Unlock()
	// Eluna GROUP_EVENT_ON_DISBAND (5): C++ Group::Disband fires
	// OnGroupDisband first, before the per-member detach (Group.cpp:859).
	s.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
	for _, guid := range members {
		sess := s.findSessionByGUID(guid)
		if sess == nil {
			continue
		}
		sess.groupID = 0
		_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_DESTROYED), nil, true)
		empty := buildGroupList(s, &groupState{ID: g.ID, LeaderGUID: guid}, guid, 0)
		_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), empty, true)
	}
	if s.CharactersStore != nil {
		if s.CharactersStore.DB != nil {
			_, _ = s.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_DEL_GROUP_MEMBER_ALL"), g.DBID)
			_, _ = s.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_DEL_GROUP"), g.DBID)
		}
	}
}

// handleGroupDisband processes CMSG_GROUP_DISBAND (0x07B).
// TrinityCore: WorldSession::HandleGroupDisbandOpcode.
func (s *session) handleGroupDisband(_ context.Context, _ []byte) bool {
	if !s.playerLoaded {
		return false
	}
	if s.pendingGroupLeader != 0 {
		// Cancel a pending invite we initiated (not a real TC case but safe)
		s.pendingGroupLeader = 0
		return true
	}
	if s.groupID == 0 {
		return false
	}

	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil {
		s.groupID = 0
		srv.groupsMu.Unlock()
		return false
	}

	name := ""
	if s.player != nil {
		name = s.player.Name
	}

	if g.IsLFG {
		s.updateAchievementCriteria(criteriaTypeLFGAbandon, 0, 1)
	}

	if g.LeaderGUID == s.playerGUID {
		// Leader disbands entire group
		groupObj := groupLuaObject(g)
		members := make([]uint64, len(g.Members))
		for i, m := range g.Members {
			members[i] = m.GUID
		}
		delete(srv.groups, g.ID)
		srv.groupsMu.Unlock()
		for _, guid := range members {
			sess := srv.findSessionByGUID(guid)
			if sess == nil {
				continue
			}
			sess.groupID = 0
			_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_DESTROYED), nil, true)
			empty := buildGroupList(srv, &groupState{ID: g.ID, LeaderGUID: guid}, guid, 0)
			_ = sess.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), empty, true)
		}
		// Eluna GROUP_EVENT_ON_DISBAND (5): C++ Group::Disband fires
		// OnGroupDisband first (Group.cpp:859).
		srv.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
	} else {
		// Non-leader leaves: C++ HandleGroupDisbandOpcode calls
		// Player::RemoveFromGroup(GROUP_REMOVEMETHOD_LEAVE) (GroupHandler.cpp:424).
		groupObj := groupLuaObject(g)
		for i, m := range g.Members {
			if m.GUID == s.playerGUID {
				g.Members = append(g.Members[:i], g.Members[i+1:]...)
				break
			}
		}
		s.groupID = 0
		srv.onPlayerLeaveGroupRolls(s.playerGUID, g.ID)
		disbanded := len(g.Members) <= 1
		var lastGUID uint64
		if disbanded {
			if len(g.Members) == 1 {
				lastGUID = g.Members[0].GUID
			}
			delete(srv.groups, g.ID)
			srv.groupsMu.Unlock()
			if lastGUID != 0 {
				if last := srv.findSessionByGUID(lastGUID); last != nil {
					last.groupID = 0
					_ = last.write(uint16(protocol.OpcodeSMSG_GROUP_DESTROYED), nil, true)
					emptyG := buildGroupList(srv, &groupState{ID: g.ID, LeaderGUID: lastGUID}, lastGUID, 0)
					_ = last.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), emptyG, true)
				}
			}
		} else {
			srv.groupsMu.Unlock()
			srv.broadcastGroupList(g)
		}
		// C++ Group::RemoveMember fires OnGroupRemoveMember (Group.cpp:569);
		// its tail disbands the group (firing OnGroupDisband) when the
		// member count drops to 1 or fewer.
		// LFGGroupScript::OnRemoveMember native arms (LFGScripts.cpp:183-228).
		srv.lfgGroupMemberRemoved(g, s, groupRemoveMethodLeave)
		srv.triggerGroupEvent(scripting.GroupEventOnMemberRemove, groupObj, s.playerGUID, uint8(groupRemoveMethodLeave))
		if disbanded {
			srv.triggerGroupEvent(scripting.GroupEventOnDisband, groupObj)
		}
	}

	_ = s.sendPartyResult(partyOpLeave, name, errPartyResultOK)
	return true
}

// handleLootMethod processes CMSG_LOOT_METHOD (0x09A).
// TrinityCore: WorldSession::HandleLootMethodOpcode.
func (s *session) handleLootMethod(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	lootMethod, err := r.ReadU32()
	if err != nil {
		return false
	}
	masterLooter, err := r.ReadU64()
	if err != nil {
		return false
	}
	lootThreshold, err := r.ReadU32()
	if err != nil {
		return false
	}

	if lootMethod > 4 {
		return false
	}
	if lootThreshold < 2 || lootThreshold > 6 {
		return false
	}

	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil || g.LeaderGUID != s.playerGUID {
		srv.groupsMu.Unlock()
		return false
	}
	if g.IsLFG { // C++ HandleLootMethodOpcode: isLFGGroup() -> return
		srv.groupsMu.Unlock()
		return false
	}
	if lootMethod == 2 { // MASTER_LOOT: master looter must be a group member
		masterIsMember := false
		for _, m := range g.Members {
			if m.GUID == masterLooter {
				masterIsMember = true
				break
			}
		}
		if !masterIsMember {
			srv.groupsMu.Unlock()
			return false
		}
	}
	g.LootMethod = uint8(lootMethod)
	g.MasterLooter = masterLooter
	g.LootThreshold = uint8(lootThreshold)
	srv.groupsMu.Unlock()
	srv.broadcastGroupList(g)
	return true
}

// handleMinimapPing processes MSG_MINIMAP_PING (0x1D5).
// TrinityCore: WorldSession::HandleMinimapPingOpcode.
// Sends a map ping to all group members.
func (s *session) handleMinimapPing(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	x, err := r.ReadF32()
	if err != nil {
		return false
	}
	y, err := r.ReadF32()
	if err != nil {
		return false
	}

	buf := protocol.NewBuffer(16)
	buf.WriteU64(s.playerGUID)
	buf.WriteF32(x)
	buf.WriteF32(y)
	pkt := buf.Bytes()

	srv := s.server
	srv.sessionsMu.RLock()
	for sess := range srv.sessions {
		if sess.groupID == s.groupID && sess != s {
			_ = sess.write(uint16(protocol.OpcodeMSG_MINIMAP_PING), pkt, true)
		}
	}
	srv.sessionsMu.RUnlock()
	return true
}

// handleRaidTargetUpdate processes MSG_RAID_TARGET_UPDATE (0x321).
// TrinityCore: WorldSession::HandleRaidTargetUpdateOpcode.
func (s *session) handleRaidTargetUpdate(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	x, err := r.ReadU8()
	if err != nil {
		return false
	}

	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil {
		srv.groupsMu.Unlock()
		return false
	}

	if x == 0xFF {
		// Query — Group::SendTargetIconList: u8(1) then (u8 index, u64 guid)
		// per non-empty slot, sent to the requester only.
		var icons [8]uint64
		copy(icons[:], g.TargetIcons[:])
		srv.groupsMu.Unlock()
		b := protocol.NewBuffer(2 + 8*9)
		b.WriteU8(1)
		for i, iconGUID := range icons {
			if iconGUID == 0 {
				continue
			}
			b.WriteU8(uint8(i))
			b.WriteU64(iconGUID)
		}
		_ = s.write(uint16(protocol.OpcodeMSG_RAID_TARGET_UPDATE), b.Bytes(), true)
		return true
	}

	// Raid groups: leader or assistant only (GroupHandler.cpp).
	raidGateOK := !g.IsRaid || g.isLeaderOrAssistant(s.playerGUID)
	srv.groupsMu.Unlock()
	if !raidGateOK {
		return false
	}

	guid, err := r.ReadU64()
	if err != nil {
		return false
	}

	// Player targets must resolve to a connected, non-hostile player
	// (HandleRaidTargetUpdateOpcode's guid.IsPlayer() arm). HIGHGUID_PLAYER is
	// 0, so clear high bits mark a player GUID — the same test C++ uses; an
	// empty GUID qualifies and misses the lookup, matching the C++ silent
	// return. Hostility is the playerTeam analog (FFA/duel edges unmodeled).
	if uint16(guid>>48) == 0 {
		target := srv.findSessionByGUID(guid)
		hostile := target == nil || target.player == nil || s.player == nil
		if !hostile {
			tTeam, sTeam := playerTeam(target.player.Race), playerTeam(s.player.Race)
			hostile = tTeam != 0 && sTeam != 0 && tTeam != sTeam
		}
		if hostile {
			return false
		}
	}

	srv.groupsMu.Lock()
	g = srv.groups[s.groupID]
	if g == nil || x >= targetIconCount {
		srv.groupsMu.Unlock()
		return false
	}
	// Group::SetTargetIcon: clear the GUID from any other slot first (each
	// clear broadcasts its own packet), then set and broadcast.
	type iconUpdate struct {
		icon uint8
		guid uint64
	}
	var updates []iconUpdate
	if guid != 0 {
		for i := range g.TargetIcons {
			if uint8(i) != x && g.TargetIcons[i] == guid {
				g.TargetIcons[i] = 0
				updates = append(updates, iconUpdate{uint8(i), 0})
			}
		}
	}
	g.TargetIcons[x] = guid
	updates = append(updates, iconUpdate{x, guid})
	srv.groupsMu.Unlock()
	for _, u := range updates {
		b := protocol.NewBuffer(18)
		b.WriteU8(0)
		b.WriteU64(s.playerGUID)
		b.WriteU8(u.icon)
		b.WriteU64(u.guid)
		srv.broadcastToGroup(s.groupID, uint16(protocol.OpcodeMSG_RAID_TARGET_UPDATE), b.Bytes())
	}
	return true
}

// targetIconCount mirrors TARGETICONCOUNT (Group.h:45).
const targetIconCount = 8

// handleGroupRaidConvert processes CMSG_GROUP_RAID_CONVERT (0x28E).
// TrinityCore: WorldSession::HandleGroupRaidConvertOpcode.
func (s *session) handleGroupRaidConvert(_ context.Context, _ []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil || g.LeaderGUID != s.playerGUID || len(g.Members) < 2 {
		srv.groupsMu.Unlock()
		return false
	}
	g.IsRaid = true
	g.GroupType |= 0x02
	srv.groupsMu.Unlock()
	_ = s.sendPartyResult(partyOpInvite, "", errPartyResultOK)
	srv.broadcastGroupList(g)
	return true
}

// handlePartyAssignment processes MSG_PARTY_ASSIGNMENT (0x38E).
// Sets main assist / main tank flags.
// TrinityCore: WorldSession::HandlePartyAssignmentOpcode.
func (s *session) handlePartyAssignment(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 {
		return false
	}
	r := protocol.NewReader(payload)
	assignment, err := r.ReadU8()
	if err != nil {
		return false
	}
	applyByte, err := r.ReadU8()
	if err != nil {
		return false
	}
	guid, err := r.ReadU64()
	if err != nil {
		return false
	}
	apply := applyByte != 0

	srv := s.server
	srv.groupsMu.Lock()
	g := srv.groups[s.groupID]
	if g == nil || !g.IsRaid || !g.isLeaderOrAssistant(s.playerGUID) {
		srv.groupsMu.Unlock()
		return false
	}
	const (
		assignMainAssist = 0
		assignMainTank   = 1
	)
	clearFlag := uint8(0)
	setFlag := uint8(0)
	switch assignment {
	case assignMainAssist:
		clearFlag = memberFlagMainAssist
		setFlag = memberFlagMainAssist
	case assignMainTank:
		clearFlag = memberFlagMainTank
		setFlag = memberFlagMainTank
	default:
		srv.groupsMu.Unlock()
		srv.broadcastGroupList(g)
		return true
	}
	// Clear flag from all members first
	for i := range g.Members {
		g.Members[i].Flags &^= clearFlag
	}
	if apply {
		for i := range g.Members {
			if g.Members[i].GUID == guid {
				g.Members[i].Flags |= setFlag
				break
			}
		}
	}
	srv.groupsMu.Unlock()
	srv.broadcastGroupList(g)
	return true
}

// handleReadyCheck processes MSG_RAID_READY_CHECK (0x322).
// TrinityCore: WorldSession::HandleRaidReadyCheckOpcode (GroupHandler.cpp:687).
func (s *session) handleReadyCheck(_ context.Context, payload []byte) bool {
	if !s.playerLoaded || s.groupID == 0 || s.server == nil {
		return false
	}
	srv := s.server
	srv.groupsMu.RLock()
	g := srv.groups[s.groupID]
	srv.groupsMu.RUnlock()
	if g == nil {
		return false
	}

	r := protocol.NewReader(payload)
	if len(payload) == 0 {
		// Request — must be leader or assistant
		if !g.isLeaderOrAssistant(s.playerGUID) {
			return false
		}
		b := protocol.NewBuffer(8)
		b.WriteU64(s.playerGUID)
		pkt := b.Bytes()
		srv.broadcastToGroup(s.groupID, uint16(protocol.OpcodeMSG_RAID_READY_CHECK), pkt)

		// Offline ready check: send not-ready (0) for any offline members to leaders/assistants
		srv.sessionsMu.RLock()
		for _, m := range g.Members {
			if srv.findSessionByGUID(m.GUID) == nil {
				bOffline := protocol.NewBuffer(9)
				bOffline.WriteU64(m.GUID)
				bOffline.WriteU8(0) // 0 = not ready
				for sess := range srv.sessions {
					if sess.groupID == s.groupID && g.isLeaderOrAssistant(sess.playerGUID) {
						_ = sess.write(uint16(protocol.OpcodeMSG_RAID_READY_CHECK_CONFIRM), bOffline.Bytes(), true)
					}
				}
			}
		}
		srv.sessionsMu.RUnlock()
	} else {
		// Answer from a group member
		state, err := r.ReadU8()
		if err != nil {
			return false
		}
		b := protocol.NewBuffer(9)
		b.WriteU64(s.playerGUID)
		b.WriteU8(state)
		pkt := b.Bytes()
		// Broadcast the reply to leader and assistants (Group::BroadcastReadyCheck)
		srv.sessionsMu.RLock()
		for sess := range srv.sessions {
			if sess.groupID == s.groupID && g.isLeaderOrAssistant(sess.playerGUID) {
				_ = sess.write(uint16(protocol.OpcodeMSG_RAID_READY_CHECK_CONFIRM), pkt, true)
			}
		}
		srv.sessionsMu.RUnlock()
	}
	return true
}

// handleRaidReadyCheckFinished processes MSG_RAID_READY_CHECK_FINISHED (0x3C6).
// Reference: WorldSession::HandleRaidReadyCheckFinishedOpcode (GroupHandler.cpp:722)
// is a deliberate no-op (the whole body is commented out): the server never sends
// MSG_RAID_READY_CHECK_FINISHED — the client ends the check when it has received
// all MSG_RAID_READY_CHECK_CONFIRM replies. Broadcasting it here would end the
// ready-check display early, diverging from C++.
func (s *session) handleRaidReadyCheckFinished(_ context.Context, _ []byte) bool {
	return true
}

// sendPartyResult is a helper to write SMSG_PARTY_COMMAND_RESULT.
func (s *session) sendPartyResult(op uint32, member string, result uint32) bool {
	return s.write(uint16(protocol.OpcodeSMSG_PARTY_COMMAND_RESULT), buildPartyCommandResult(op, member, result), true) == nil
}

// toLower is a simple ASCII lowercase helper.
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// randomRollMax is the maximum roll value (matching TrinityCore).
const randomRollMax = 10000

// handleRandomRoll processes MSG_RANDOM_ROLL.
// TrinityCore: WorldSession::HandleRandomRollOpcode.
func (s *session) handleRandomRoll(_ context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	r := protocol.NewReader(payload)
	minimum, err := r.ReadU32()
	if err != nil {
		return false
	}
	maximum, err := r.ReadU32()
	if err != nil {
		return false
	}
	if minimum > maximum || maximum > randomRollMax {
		return false
	}
	rolled := minimum + uint32(rand.Intn(int(maximum-minimum)+1))

	// SMSG_RANDOMIZE_CHAR_NAME uses MSG_RANDOM_ROLL opcode in 3.3.5a.
	b := protocol.NewBuffer(20)
	b.WriteU32(minimum)
	b.WriteU32(maximum)
	b.WriteU32(rolled)
	b.WriteU64(s.playerGUID)
	pkt := b.Bytes()

	_ = s.write(uint16(protocol.OpcodeMSG_RANDOM_ROLL), pkt, true)
	// Broadcast to group members if in a group
	if s.groupID != 0 {
		srv := s.server
		srv.sessionsMu.RLock()
		for sess := range srv.sessions {
			if sess.groupID == s.groupID && sess != s {
				_ = sess.write(uint16(protocol.OpcodeMSG_RANDOM_ROLL), pkt, true)
			}
		}
		srv.sessionsMu.RUnlock()
	}
	return true
}

// handleGroupAssistantLeader processes CMSG_GROUP_ASSISTANT_LEADER (0x28F).
// Reference: WorldSession::HandleGroupAssistantLeaderOpcode (GroupHandler.cpp:633).
func (s *session) handleGroupAssistantLeader(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 || s.groupID == 0 || s.server == nil {
		return true
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil {
		return true
	}
	apply, err := r.ReadU8()
	if err != nil {
		return true
	}

	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	if grp == nil || !grp.isLeader(s.playerGUID) || !grp.IsRaid {
		s.server.groupsMu.Unlock()
		return true
	}

	changed := false
	for i := range grp.Members {
		if grp.Members[i].GUID == guid {
			if apply != 0 {
				grp.Members[i].Flags |= memberFlagAssistant
			} else {
				grp.Members[i].Flags &^= memberFlagAssistant
			}
			changed = true
			break
		}
	}
	s.server.groupsMu.Unlock()

	if changed {
		s.server.broadcastGroupList(grp)
	}
	return true
}

// handleGroupChangeSubGroup processes CMSG_GROUP_CHANGE_SUB_GROUP (0x27E).
// Reference: WorldSession::HandleGroupChangeSubGroupOpcode (GroupHandler.cpp:600).
func (s *session) handleGroupChangeSubGroup(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 || s.groupID == 0 || s.server == nil {
		return true
	}
	r := protocol.NewReader(payload)
	name, err := r.ReadCString()
	if err != nil {
		return true
	}
	groupNr, err := r.ReadU8()
	if err != nil || groupNr >= 8 { // MAX_RAID_SUBGROUPS = 8
		return true
	}

	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	if grp == nil || !grp.IsRaid || !grp.isLeaderOrAssistant(s.playerGUID) {
		s.server.groupsMu.Unlock()
		return true
	}

	if grp.countInSubGroup(groupNr) >= 5 {
		s.server.groupsMu.Unlock()
		return true
	}

	var found bool
	for i := range grp.Members {
		if strings.EqualFold(grp.Members[i].Name, name) {
			if grp.Members[i].SubGroup != groupNr {
				grp.Members[i].SubGroup = groupNr
				found = true
			}
			break
		}
	}
	s.server.groupsMu.Unlock()

	if found {
		s.server.broadcastGroupList(grp)
	}
	return true
}

// handleResetInstances processes CMSG_RESET_INSTANCES (0x31D).
// Reference: WorldSession::HandleResetInstancesOpcode (MiscHandler.cpp:1255).
func (s *session) handleResetInstances(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}

	if s.groupID != 0 && s.server != nil {
		grp := s.server.findGroupByID(s.groupID)
		if grp != nil {
			if grp.LeaderGUID != s.playerGUID {
				// Non-leader in group cannot reset instances (matches TC HandleResetInstancesOpcode)
				return true
			}
			// Check if any group member is currently inside an instance/dungeon
			groupSessions := s.server.getGroupSessions(s.groupID)
			for _, memSess := range groupSessions {
				if memSess != nil && memSess.playerLoaded && memSess.player != nil {
					if s.isDungeonMap(memSess.player.Map) {
						// Group member is inside the instance: fail reset
						failBuf := protocol.NewBuffer(8)
						failBuf.WriteU32(0) // reason 0: players inside instance
						failBuf.WriteU32(memSess.player.Map)
						_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_RESET_FAILED), failBuf.Bytes(), true)

						notifyBuf := protocol.NewBuffer(4)
						notifyBuf.WriteU32(memSess.player.Map)
						_ = memSess.write(uint16(protocol.OpcodeSMSG_RESET_FAILED_NOTIFY), notifyBuf.Bytes(), true)
						return true
					}
				}
			}

			// Clean up non-permanent instance bindings for group members in DB
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				cdb := s.server.CharactersStore.DB
				for _, mem := range grp.Members {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_instance WHERE guid = ? AND permanent = 0", mem.GUID)
				}
			}

			// Broadcast SMSG_INSTANCE_RESET to all group members
			resetBuf := protocol.NewBuffer(4)
			resetBuf.WriteU32(0)
			s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_INSTANCE_RESET), resetBuf.Bytes())
			return true
		}
	}

	// Solo player path
	if s.isDungeonMap(s.player.Map) {
		failBuf := protocol.NewBuffer(8)
		failBuf.WriteU32(0) // reason 0: players inside instance
		failBuf.WriteU32(s.player.Map)
		_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_RESET_FAILED), failBuf.Bytes(), true)

		notifyBuf := protocol.NewBuffer(4)
		notifyBuf.WriteU32(s.player.Map)
		_ = s.write(uint16(protocol.OpcodeSMSG_RESET_FAILED_NOTIFY), notifyBuf.Bytes(), true)
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_instance WHERE guid = ? AND permanent = 0", s.playerGUID)
	}

	resetBuf := protocol.NewBuffer(4)
	resetBuf.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_RESET), resetBuf.Bytes(), true)
	return true
}

// handleSetDungeonDifficulty processes MSG_SET_DUNGEON_DIFFICULTY (0x329).
// Reference: WorldSession::HandleSetDungeonDifficultyOpcode (MiscHandler.cpp:1268).
// Protocol: Player::SendDungeonDifficulty (Player.cpp:20615):
// uint32 difficulty, uint32 1, uint32 isInGroup
func (s *session) handleSetDungeonDifficulty(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	mode, err := r.ReadU32()
	if err != nil {
		return false
	}
	if mode >= 2 { // MAX_DUNGEON_DIFFICULTY = 2 (0=Normal, 1=Heroic)
		mode = 0
	}
	s.player.DungeonDifficulty = uint8(mode)

	isInGroup := uint32(0)
	if s.groupID != 0 {
		isInGroup = 1
	}

	buf := protocol.NewBuffer(12)
	buf.WriteU32(mode)
	buf.WriteU32(1)
	buf.WriteU32(isInGroup)

	if s.groupID != 0 && s.server != nil {
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY), buf.Bytes())
	} else {
		_ = s.write(uint16(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY), buf.Bytes(), true)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY), buildInstanceDifficulty(mode), true)
	return true
}

// handleSetRaidDifficulty processes MSG_SET_RAID_DIFFICULTY (0x4EB).
// Reference: WorldSession::HandleSetRaidDifficultyOpcode (MiscHandler.cpp:1323).
// Protocol: Player::SendRaidDifficulty (Player.cpp:20625):
// uint32 difficulty, uint32 1, uint32 isInGroup
func (s *session) handleSetRaidDifficulty(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	mode, err := r.ReadU32()
	if err != nil {
		return false
	}
	if mode >= 4 { // MAX_RAID_DIFFICULTY = 4 (0=10N, 1=25N, 2=10H, 3=25H)
		mode = 0
	}
	s.player.RaidDifficulty = uint8(mode)

	isInGroup := uint32(0)
	if s.groupID != 0 {
		isInGroup = 1
	}

	buf := protocol.NewBuffer(12)
	buf.WriteU32(mode)
	buf.WriteU32(1)
	buf.WriteU32(isInGroup)

	if s.groupID != 0 && s.server != nil {
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeMSG_SET_RAID_DIFFICULTY), buf.Bytes())
	} else {
		_ = s.write(uint16(protocol.OpcodeMSG_SET_RAID_DIFFICULTY), buf.Bytes(), true)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY), buildInstanceDifficulty(mode), true)
	return true
}

func (s *session) isDungeonMap(mapID uint32) bool {
	var srv *Server
	if s != nil {
		srv = s.server
	}
	return dungeonMapID(srv, mapID)
}

func (s *session) setPendingBind(instanceID uint64, mapID, diff, timer uint32) {
	if s == nil {
		return
	}
	s.pendingBindMu.Lock()
	s.pendingBindInstanceID = instanceID
	s.pendingBindMapID = mapID
	s.pendingBindDiff = diff
	s.pendingBindTimer = timer
	s.pendingBindMu.Unlock()
}

func (s *session) takePendingBind() (uint64, uint32, uint32, bool) {
	if s == nil {
		return 0, 0, 0, false
	}
	s.pendingBindMu.Lock()
	defer s.pendingBindMu.Unlock()
	if s.pendingBindInstanceID == 0 {
		return 0, 0, 0, false
	}
	instanceID, mapID, difficulty := s.pendingBindInstanceID, s.pendingBindMapID, s.pendingBindDiff
	s.pendingBindInstanceID, s.pendingBindMapID, s.pendingBindDiff, s.pendingBindTimer = 0, 0, 0, 0
	return instanceID, mapID, difficulty, true
}

func (s *Server) updatePendingInstanceBinds(ctx context.Context, elapsed time.Duration) {
	if s == nil || elapsed <= 0 {
		return
	}
	elapsedMs := uint32(elapsed / time.Millisecond)
	if elapsedMs == 0 {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		sess.pendingBindMu.Lock()
		instanceID := sess.pendingBindInstanceID
		if instanceID == 0 {
			sess.pendingBindMu.Unlock()
			continue
		}
		if !sess.playerLoaded || sess.player == nil {
			sess.pendingBindMu.Unlock()
			sess.setPendingBind(0, 0, 0, 0)
			continue
		}
		if elapsedMs < sess.pendingBindTimer {
			sess.pendingBindTimer -= elapsedMs
			sess.pendingBindMu.Unlock()
			continue
		}
		sess.pendingBindTimer = 0
		sess.pendingBindMu.Unlock()
		if uint64(sess.player.InstanceID) != instanceID {
			sess.setPendingBind(0, 0, 0, 0)
			continue
		}
		_ = sess.handleInstanceLockResponse(ctx, []byte{1})
	}
}

func (s *session) sendInstanceLockWarningQuery(timeRemainingMs, completedEncounterMask uint32, extend uint8) {
	buf := protocol.NewBuffer(9)
	buf.WriteU32(timeRemainingMs)
	buf.WriteU32(completedEncounterMask)
	buf.WriteU8(extend)
	_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_LOCK_WARNING_QUERY), buf.Bytes(), true)
}

func (s *session) sendCalendarRaidLockout(mapID, difficulty uint32, resetTimeSec uint32, instanceID uint64, add bool) {
	currTime := time.Now()
	op := uint16(protocol.OpcodeSMSG_CALENDAR_RAID_LOCKOUT_REMOVED)
	cap := 20
	if add {
		op = uint16(protocol.OpcodeSMSG_CALENDAR_RAID_LOCKOUT_ADDED)
		cap = 24
	}
	buf := protocol.NewBuffer(cap)
	if add {
		buf.WritePackedTime(currTime)
	}
	buf.WriteU32(mapID)
	buf.WriteU32(difficulty)
	buf.WriteU32(resetTimeSec)
	buf.WriteU64(instanceID)
	_ = s.write(op, buf.Bytes(), true)
}

func (s *session) sendCalendarRaidLockoutUpdated(mapID, difficulty uint32, resetTimeSec uint32) {
	currTime := time.Now()
	buf := protocol.NewBuffer(20)
	buf.WritePackedTime(currTime)
	buf.WriteU32(mapID)
	buf.WriteU32(difficulty)
	buf.WriteU32(0) // time delta
	buf.WriteU32(resetTimeSec)
	_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_RAID_LOCKOUT_UPDATED), buf.Bytes(), true)
}

// handleInstanceLockResponse processes CMSG_INSTANCE_LOCK_RESPONSE (0x13F).
// Reference: WorldSession::HandleInstanceLockResponse (MiscHandler.cpp:1525).
func (s *session) handleInstanceLockResponse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}

	instanceID, mapID, difficulty, hasPendingBind := s.takePendingBind()
	if !hasPendingBind {
		return true
	}

	accept := payload[0]
	if accept != 0 {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_INSTANCE_SAVE_CREATED), buf.Bytes(), true)

		if !s.isMapAdmissionGM() {
			var resetTime uint32 = 7 * 86400
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				cdb := s.server.CharactersStore.DB
				var rt int64
				if err := cdb.QueryRowContext(ctx, "SELECT resettime FROM instance WHERE id = ?", instanceID).Scan(&rt); err == nil {
					now := time.Now().Unix()
					if rt > now {
						resetTime = uint32(rt - now)
					}
				}
				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, instanceID)
				_, _ = cdb.ExecContext(ctx, "INSERT INTO character_instance (guid, instance, permanent, extendState) VALUES (?, ?, 1, 0)", s.playerGUID, instanceID)
			}
			s.sendCalendarRaidLockout(mapID, difficulty, resetTime, instanceID, true)
		}
	} else {
		s.repopAtGraveyard(ctx)
	}

	return true
}

// handleSetSavedInstanceExtend processes CMSG_SET_SAVED_INSTANCE_EXTEND (0x292).
// Reference: WorldSession::HandleSetSavedInstanceExtend (CalendarHandler.cpp:786).
func (s *session) handleSetSavedInstanceExtend(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	mapID, err := r.ReadU32()
	if err != nil {
		return true
	}
	difficulty, err := r.ReadU32()
	if err != nil {
		return true
	}
	toggleExtend, err := r.ReadU8()
	if err != nil {
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var extendState int = 0
		if toggleExtend != 0 {
			extendState = 2 // EXTEND_STATE_EXTENDED
		}
		_, _ = cdb.ExecContext(ctx, `UPDATE character_instance SET extendState = ? 
			WHERE guid = ? AND instance IN (SELECT id FROM instance WHERE map = ? AND difficulty = ?)`,
			extendState, s.playerGUID, mapID, difficulty)

		var resetTime int64
		_ = cdb.QueryRowContext(ctx, `SELECT i.resettime 
			FROM instance i JOIN character_instance ci ON ci.instance = i.id 
			WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ?`,
			s.playerGUID, mapID, difficulty).Scan(&resetTime)

		rem := uint32(0)
		now := time.Now().Unix()
		if resetTime > now {
			rem = uint32(resetTime - now)
		}

		s.sendCalendarRaidLockoutUpdated(mapID, difficulty, rem)
		_ = s.handleRequestRaidInfo(ctx)
	}
	return true
}

const (
	groupUpdateFlagStatus      uint32 = protocol.GroupUpdateFlagStatus
	groupUpdateFlagCurHP       uint32 = protocol.GroupUpdateFlagCurrentHealth
	groupUpdateFlagMaxHP       uint32 = protocol.GroupUpdateFlagMaximumHealth
	groupUpdateFlagPowerType   uint32 = protocol.GroupUpdateFlagPowerType
	groupUpdateFlagCurPower    uint32 = protocol.GroupUpdateFlagCurrentPower
	groupUpdateFlagMaxPower    uint32 = protocol.GroupUpdateFlagMaximumPower
	groupUpdateFlagLevel       uint32 = protocol.GroupUpdateFlagLevel
	groupUpdateFlagZone        uint32 = protocol.GroupUpdateFlagZone
	groupUpdateFlagPosition    uint32 = protocol.GroupUpdateFlagPosition
	groupUpdateFlagAuras       uint32 = protocol.GroupUpdateFlagAuras
	groupUpdateFlagPetGUID     uint32 = protocol.GroupUpdateFlagPetGUID
	groupUpdateFlagPetName     uint32 = protocol.GroupUpdateFlagPetName
	groupUpdateFlagPetModel    uint32 = protocol.GroupUpdateFlagPetModelID
	groupUpdateFlagPetCurHP    uint32 = protocol.GroupUpdateFlagPetCurrentHealth
	groupUpdateFlagPetMaxHP    uint32 = protocol.GroupUpdateFlagPetMaximumHealth
	groupUpdateFlagPetPower    uint32 = protocol.GroupUpdateFlagPetPowerType
	groupUpdateFlagPetMaxPower uint32 = protocol.GroupUpdateFlagPetMaximumPower
	groupUpdateFlagPetAuras    uint32 = protocol.GroupUpdateFlagPetAuras
	groupUpdateFlagVehicleSeat uint32 = protocol.GroupUpdateFlagVehicleSeat
)

const (
	memberStatusOffline uint16 = 0x0000
	memberStatusOnline  uint16 = 0x0001
	memberStatusPvP     uint16 = 0x0002
	memberStatusDead    uint16 = 0x0004
	memberStatusGhost   uint16 = 0x0008
	memberStatusPvPFFA  uint16 = 0x0010
	memberStatusAFK     uint16 = 0x0020
	memberStatusDND     uint16 = 0x0040
)

type groupAuraEntry = protocol.PartyMemberAura

type groupPetStats struct {
	guid         uint64
	name         string
	model        uint16
	health       uint32
	maxHealth    uint32
	powerType    uint8
	currentPower uint16
	maxPower     uint16
	auraMask     uint64
	auras        [64]groupAuraEntry
}

func groupAuraEntries(auras []*activeAura, targetGUID uint64) (uint64, [64]groupAuraEntry) {
	var mask uint64
	var entries [64]groupAuraEntry
	for _, aura := range auras {
		if !clientVisibleAura(aura) {
			continue
		}
		flags := aura.EffectMask & 0x07
		if flags == 0 {
			flags = 0x01
		}
		if aura.CasterGUID == targetGUID {
			flags |= protocol.AuraFlagCaster
		}
		if aura.Positive {
			flags |= protocol.AuraFlagPositive
		} else {
			flags |= protocol.AuraFlagNegative
		}
		if aura.DurationMs > 0 && !aura.HideDuration {
			flags |= protocol.AuraFlagDuration
		}
		entries[aura.Slot] = groupAuraEntry{SpellID: aura.SpellID, Flags: flags}
		mask |= uint64(1) << aura.Slot
	}
	return mask, entries
}

func (s *session) groupPetStats(ctx context.Context, target *session) groupPetStats {
	var result groupPetStats
	if s == nil || s.server == nil || target == nil || target.player == nil || target.player.PetGUID == 0 {
		return result
	}
	result.guid = target.player.PetGUID
	var entry, level, petType, health, mana, model int64
	petNumber := target.activePetNumber()
	if petNumber != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT entry, level, COALESCE(PetType, 0), COALESCE(curhealth, 0), COALESCE(curmana, 0), COALESCE(modelid, 0), COALESCE(name, '') FROM character_pet WHERE id = ? AND owner = ?", petNumber, target.playerGUID).Scan(&entry, &level, &petType, &health, &mana, &model, &result.name)
	}
	hasMotion := false
	s.server.motionMu.Lock()
	if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, result.guid); motion != nil {
		hasMotion = true
		entry, level, health, petType = int64(motion.Entry), int64(motion.Level), int64(motion.Health), int64(motion.PetType)
		result.health, result.maxHealth = motion.Health, motion.MaxHealth
		result.powerType = uint8(motion.PowerType)
		if motion.PowerType < uint32(len(motion.Powers)) {
			result.currentPower = uint16(motion.Powers[motion.PowerType])
			result.maxPower = uint16(motion.MaxPowers[motion.PowerType])
		}
	}
	s.server.motionMu.Unlock()
	result.model = uint16(model)
	if !hasMotion && health > 0 {
		result.health = uint32(health)
	}
	if !hasMotion && entry > 0 {
		_, maxHealth, _, maxMana := target.getPetStats(ctx, uint32(entry), uint32(maxUint32(uint32(level), 1)), uint8(petType))
		result.maxHealth, result.maxPower = maxHealth, uint16(maxMana)
		if petType == int64(petTypeHunter) {
			result.powerType, result.maxPower = 2, uint16(petFocusMax)
		}
		result.currentPower = uint16(mana)
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry > 0 && (result.name == "" || result.model == 0) {
		var templateName string
		var displayID int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name, COALESCE(NULLIF(modelid1, 0), NULLIF(modelid2, 0), NULLIF(modelid3, 0), NULLIF(modelid4, 0), 0) FROM creature_template WHERE entry = ?", entry).Scan(&templateName, &displayID)
		if result.name == "" {
			result.name = templateName
		}
		if result.model == 0 {
			result.model = uint16(displayID)
		}
	}
	var petAuras []*activeAura
	s.server.auraMu.Lock()
	for _, aura := range s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*target.player, result.guid)] {
		if aura != nil {
			copy := *aura
			petAuras = append(petAuras, &copy)
		}
	}
	s.server.auraMu.Unlock()
	result.auraMask, result.auras = groupAuraEntries(petAuras, result.guid)
	return result
}

// handleRequestPartyMemberStats processes CMSG_REQUEST_PARTY_MEMBER_STATS (0x27F).
// Reference: WorldSession::HandleRequestPartyMemberStatsOpcode (GroupHandler.cpp:920),
// and GroupHandler::SendPartyMemberStats (GroupHandler.cpp:752).
func (s *session) handleRequestPartyMemberStats(ctx context.Context, payload []byte) bool {
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
		payload := protocol.BuildPartyMemberStatsFull(protocol.PartyMemberStatsFull{GUID: targetGUID, UpdateFlags: groupUpdateFlagStatus, Status: memberStatusOffline})
		_ = s.write(uint16(protocol.OpcodeSMSG_PARTY_MEMBER_STATS_FULL), payload, true)
		return true
	}

	tp := targetSess.player
	powerType := playerPowerType(tp)
	mask := groupUpdateFlagStatus | groupUpdateFlagCurHP | groupUpdateFlagMaxHP |
		groupUpdateFlagCurPower | groupUpdateFlagMaxPower |
		groupUpdateFlagLevel | groupUpdateFlagZone | groupUpdateFlagPosition |
		groupUpdateFlagAuras | groupUpdateFlagPetName | groupUpdateFlagPetModel | groupUpdateFlagPetAuras

	if powerType != 0 { // 0 = POWER_MANA
		mask |= groupUpdateFlagPowerType
	}
	pet := s.groupPetStats(ctx, targetSess)
	if pet.guid != 0 {
		mask |= groupUpdateFlagPetGUID | groupUpdateFlagPetCurHP | groupUpdateFlagPetMaxHP |
			groupUpdateFlagPetPower | groupUpdateFlagPetCurPower | groupUpdateFlagPetMaxPower
	}
	var vehicleSeat uint32
	if tp.VehicleGUID != 0 {
		if kit := s.server.getVehicleKit(tp.Map, tp.InstanceID, tp.VehicleGUID); kit != nil {
			if _, seat, _ := kit.GetSeatForPassenger(targetGUID); seat != nil {
				mask |= groupUpdateFlagVehicleSeat
				vehicleSeat = seat.ID
			}
		}
	}

	var status uint16 = memberStatusOnline
	if tp.Health == 0 {
		if tp.PlayerFlags&playerFlagGhost != 0 {
			status |= memberStatusGhost
		} else {
			status |= memberStatusDead
		}
	}
	if tp.PlayerFlags&0x02 != 0 {
		status |= memberStatusPvP
	}
	if tp.PVPFlags&0x04 != 0 {
		status |= memberStatusPvPFFA
	}
	if tp.PlayerFlags&playerFlagAFK != 0 {
		status |= memberStatusAFK
	}
	if tp.PlayerFlags&playerFlagDND != 0 {
		status |= memberStatusDND
	}

	curPower := uint16(0)
	maxPower := uint16(0)
	if int(powerType) < len(tp.Powers) {
		curPower = uint16(tp.Powers[powerType])
		maxPower = uint16(tp.MaxPowers[powerType])
	}
	auraMask, auras := groupAuraEntries(targetSess.loadedAuras(), targetGUID)
	response := protocol.BuildPartyMemberStatsFull(protocol.PartyMemberStatsFull{GUID: targetGUID, UpdateFlags: mask, Status: status, Health: tp.Health, MaximumHealth: tp.MaxHealth, PowerType: powerType, CurrentPower: curPower, MaximumPower: maxPower, Level: uint16(tp.Level), Zone: uint16(tp.Zone), X: uint16(tp.X), Y: uint16(tp.Y), AuraMask: auraMask, Auras: auras, PetGUID: pet.guid, PetName: pet.name, PetModelID: pet.model, PetHealth: pet.health, PetMaximumHealth: pet.maxHealth, PetPowerType: pet.powerType, PetCurrentPower: pet.currentPower, PetMaximumPower: pet.maxPower, PetAuraMask: pet.auraMask, PetAuras: pet.auras, VehicleSeatID: vehicleSeat})
	return s.write(uint16(protocol.OpcodeSMSG_PARTY_MEMBER_STATS_FULL), response, true) == nil
}
