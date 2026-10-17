package world

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	LFGRoleNone   uint8 = 0x00
	LFGRoleLeader uint8 = 0x01
	LFGRoleTank   uint8 = 0x02
	LFGRoleHealer uint8 = 0x04
	LFGRoleDamage uint8 = 0x08
	LFGRoleAny    uint8 = LFGRoleLeader | LFGRoleTank | LFGRoleHealer | LFGRoleDamage

	LFGStateNone            uint8 = 0
	LFGStateRoleCheck       uint8 = 1
	LFGStateQueued          uint8 = 2
	LFGStateProposal        uint8 = 3
	LFGStateBoot            uint8 = 4
	LFGStateDungeon         uint8 = 5
	LFGStateFinishedDungeon uint8 = 6
	LFGStateRaidBrowser     uint8 = 7

	LFGUpdateDefault            uint8 = 0
	LFGUpdateLeaderUnk1         uint8 = 1 // LFG_UPDATETYPE_LEADER_UNK1 (LFG.h:50, "FIXME: At group leave")
	LFGUpdateRoleCheckAborted   uint8 = 4
	LFGUpdateJoinQueue          uint8 = 5
	LFGUpdateRoleCheckFailed    uint8 = 6
	LFGUpdateRemovedFromQueue   uint8 = 7
	LFGUpdateProposalFailed     uint8 = 8
	LFGUpdateProposalDeclined   uint8 = 9
	LFGUpdateGroupFound         uint8 = 10
	LFGUpdateAddedToQueue       uint8 = 12
	LFGUpdateProposalBegin      uint8 = 13
	LFGUpdateStatus             uint8 = 14
	LFGUpdateGroupMemberOffline uint8 = 15

	// LFG option bits (LFGMgr.h:41): the dungeon-finder/raid-browser join
	// gate in HandleLfgJoinOpcode.
	LFGOptionEnableDungeonFinder uint32 = 0x01
	LFGOptionEnableRaidBrowser   uint32 = 0x02

	LFGRoleCheckDefault      uint32 = 0
	LFGRoleCheckFinished     uint32 = 1
	LFGRoleCheckInitializing uint32 = 2
	LFGRoleCheckMissingRole  uint32 = 3
	LFGRoleCheckWrongRoles   uint32 = 4
	LFGRoleCheckAborted      uint32 = 5
	LFGRoleCheckNoRole       uint32 = 6

	// Group composition targets for the role-check solver (LFG.h:32-34).
	lfgTanksNeeded   = 1
	lfgHealersNeeded = 1
	lfgDPSNeeded     = 3

	LFGProposalInitiating uint8 = 0
	LFGProposalFailed     uint8 = 1
	LFGProposalSuccess    uint8 = 2

	LFGAnswerPending int8 = -1
	LFGAnswerDeny    int8 = 0
	LFGAnswerAgree   int8 = 1

	LFGLockStatusInsufficientExpansion  uint32 = 1
	LFGLockStatusTooLowLevel            uint32 = 2
	LFGLockStatusTooHighLevel           uint32 = 3
	LFGLockStatusTooLowGearScore        uint32 = 4
	LFGLockStatusTooHighGearScore       uint32 = 5
	LFGLockStatusRaidLocked             uint32 = 6
	LFGLockStatusAttunementTooLowLevel  uint32 = 1001
	LFGLockStatusAttunementTooHighLevel uint32 = 1002
	LFGLockStatusQuestNotCompleted      uint32 = 1022
	LFGLockStatusMissingItem            uint32 = 1025
	LFGLockStatusNotInSeason            uint32 = 1031
	LFGLockStatusMissingAchievement     uint32 = 1034

	LFGTeleportErrorOK              uint32 = 0
	LFGTeleportErrorPlayerDead      uint32 = 1
	LFGTeleportErrorFalling         uint32 = 2
	LFGTeleportErrorInVehicle       uint32 = 3
	LFGTeleportErrorFatigue         uint32 = 4
	LFGTeleportErrorInvalidLocation uint32 = 6
	LFGTeleportErrorCharming        uint32 = 8

	LFGDungeonTypeNone    uint32 = 0
	LFGDungeonTypeDungeon uint32 = 1
	LFGDungeonTypeRaid    uint32 = 2
	LFGDungeonTypeHeroic  uint32 = 5
	LFGDungeonTypeRandom  uint32 = 6

	LFGJoinOK                  uint32 = 0
	LFGJoinNotMeetReqs         uint32 = 5
	LFGJoinPartyNotMeetReqs    uint32 = 6
	LFGJoinMixedRaidDungeon    uint32 = 7
	LFGJoinDisconnected        uint32 = 9
	LFGJoinInvalidDungeon      uint32 = 11
	LFGJoinDeserter            uint32 = 12
	LFGJoinPartyDeserter       uint32 = 13
	LFGJoinRandomCooldown      uint32 = 14
	LFGJoinPartyRandomCooldown uint32 = 15
	LFGJoinTooMuchMembers      uint32 = 16
	LFGJoinUsingBGSystem       uint32 = 17

	// LFG script auras (LFGMgr.h:51-54,61): cast by the LFGPlayerScript /
	// LFGGroupScript arms in LFGScripts.cpp.
	lfgSpellDungeonCooldown = 71328 // LFG_SPELL_DUNGEON_COOLDOWN
	lfgSpellDungeonDeserter = 71041 // LFG_SPELL_DUNGEON_DESERTER
	lfgSpellLuckOfTheDraw   = 72221 // LFG_SPELL_LUCK_OF_THE_DRAW
	lfgGroupKickVotesNeeded = 3     // LFG_GROUP_KICK_VOTES_NEEDED (LFGMgr.h:54)
	lfgTimeBoot             = 120   // LFG_TIME_BOOT (LFGMgr.h:48, seconds)
	lfgGroupMaxKicks        = 3     // LFG_GROUP_MAX_KICKS (LFGGroupData.h:28)
	lfgFlagSeasonal         = 0x4   // LFG_FLAG_SEASONAL
)

type LFGQueueEntry struct {
	GUID     uint64
	Roles    uint8
	Dungeons []uint32
	Comment  string
	QueuedAt time.Time
	State    uint8
}

type LfgRoleCheck struct {
	GroupID   uint64
	Leader    uint64
	Dungeons  []uint32
	Roles     map[uint64]uint8 // playerGUID -> roles
	State     uint32
	CreatedAt time.Time
}

type LFGProposalPlayer struct {
	Role   uint8
	Group  uint64
	Accept int8 // -1 = pending, 0 = decline, 1 = agree
}

type LFGProposal struct {
	ID         uint32
	DungeonID  uint32
	State      uint8
	Group      uint64
	Leader     uint64
	CancelTime time.Time
	Encounters uint32
	Silent     bool
	Players    map[uint64]*LFGProposalPlayer
}

type LFGManager struct {
	mu              sync.RWMutex
	soloEnable      bool
	solo            bool
	options         uint32
	queue           map[uint64]LFGQueueEntry
	roleChecks      map[uint64]*LfgRoleCheck
	proposals       map[uint32]*LFGProposal
	nextProposalID  uint32
	validateDungeon func(uint32) bool
}

func NewLFGManager(soloEnable bool) *LFGManager {
	return &LFGManager{
		soloEnable: soloEnable,
		options:    1, // "DungeonFinder.OptionsMask" default (World.cpp:1436)
		queue:      make(map[uint64]LFGQueueEntry),
		roleChecks: make(map[uint64]*LfgRoleCheck),
		proposals:  make(map[uint32]*LFGProposal),
	}
}

func (m *LFGManager) SetDungeonValidator(validate func(uint32) bool) {
	m.mu.Lock()
	m.validateDungeon = validate
	m.mu.Unlock()
}

func (m *LFGManager) OnLogin() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.soloEnable && !m.solo {
		m.solo = true
	}
}

func (m *LFGManager) Solo() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.solo
}

func (m *LFGManager) ToggleSolo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.solo = !m.solo
	return m.solo
}

func (m *LFGManager) RequiresFullGroup(players, groupSize int) bool {
	if players < 0 || groupSize < 0 {
		return true
	}
	return !m.Solo() && players != groupSize
}

func (m *LFGManager) Join(guid uint64, roles uint8, dungeons []uint32, comment string) (uint32, LFGQueueEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(dungeons) == 0 {
		return LFGJoinNotMeetReqs, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:411-414): sanitize input roles. C++ keeps
	// the LEADER bit (PLAYER_ROLE_ANY); every consumer strips it at read
	// time (LFGMgr.cpp:955/1022/1267), so the stored queue roles carry it.
	roles &= LFGRoleAny
	if roles == 0 {
		return LFGJoinNotMeetReqs, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	filtered := append([]uint32(nil), dungeons...)
	if m.validateDungeon != nil {
		valid := filtered[:0]
		for _, dungeon := range filtered {
			if m.validateDungeon(dungeon) {
				valid = append(valid, dungeon)
			}
		}
		filtered = valid
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i] < filtered[j] })
	filtered = uniqueUint32(filtered)
	if len(filtered) == 0 {
		return LFGJoinInvalidDungeon, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	entry := LFGQueueEntry{GUID: guid, Roles: roles, Dungeons: filtered, Comment: comment, QueuedAt: time.Now(), State: LFGStateQueued}
	m.queue[guid] = entry
	return LFGJoinOK, cloneLFGQueueEntry(entry)
}

func (m *LFGManager) Leave(guid uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.queue[guid]; !ok {
		return false
	}
	delete(m.queue, guid)
	return true
}

func (m *LFGManager) Status(guid uint64) (LFGQueueEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.queue[guid]
	if !ok {
		return LFGQueueEntry{GUID: guid, State: LFGStateNone}, false
	}
	return cloneLFGQueueEntry(entry), true
}

func uniqueUint32(values []uint32) []uint32 {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneLFGQueueEntry(entry LFGQueueEntry) LFGQueueEntry {
	entry.Dungeons = append([]uint32(nil), entry.Dungeons...)
	return entry
}

func (m *LFGManager) StartRoleCheck(groupID uint64, leaderGUID uint64, dungeons []uint32, leaderRoles uint8) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	rc := &LfgRoleCheck{
		GroupID:   groupID,
		Leader:    leaderGUID,
		Dungeons:  append([]uint32(nil), dungeons...),
		Roles:     make(map[uint64]uint8),
		State:     LFGRoleCheckInitializing,
		CreatedAt: time.Now(),
	}
	rc.Roles[leaderGUID] = leaderRoles
	m.roleChecks[groupID] = rc
	return rc
}

func (m *LFGManager) GetRoleCheck(groupID uint64) *LfgRoleCheck {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.roleChecks[groupID]
}

func (m *LFGManager) UpdateRoleCheck(groupID uint64, guid uint64, roles uint8) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	rc, ok := m.roleChecks[groupID]
	if !ok {
		return nil
	}
	// LFGMgr::UpdateRoleCheck (LFGMgr.cpp:808): sanitize input roles.
	roles &= LFGRoleAny
	if roles < LFGRoleTank {
		// LFGMgr.cpp:815-816: selecting no role (or leader-only) fails the
		// check outright — the answer is not stored.
		rc.State = LFGRoleCheckNoRole
		return rc
	}
	rc.Roles[guid] = roles
	allAnswered := true
	for _, r := range rc.Roles {
		if r == LFGRoleNone {
			allAnswered = false
			break
		}
	}
	if allAnswered {
		// LFGMgr.cpp:828-831: a fully-answered check resolves through
		// CheckGroupRoles; an incompatible role set fails WRONG_ROLES. The
		// solver mutates a copy of the roles map (C++ check_roles).
		work := make(map[uint64]uint8, len(rc.Roles))
		for k, v := range rc.Roles {
			work[k] = v
		}
		if lfgCheckGroupRoles(work) {
			rc.State = LFGRoleCheckFinished
		} else {
			rc.State = LFGRoleCheckWrongRoles
		}
	}
	return rc
}

// lfgCheckGroupRoles mirrors LFGMgr::CheckGroupRoles (LFGMgr.cpp:944-1002):
// every member must resolve to exactly one of tank/healer/dps with at most
// one tank, one healer and three dps, covering the whole group. Multi-role
// members are tried role-by-role (damage first, then healer, then tank —
// the C++ arm order), recursing after each removal; the leader bit never
// participates (LFGMgr.cpp:955). std::map iterates GUIDs in ascending
// order, hence the sorted key pass.
func lfgCheckGroupRoles(roles map[uint64]uint8) bool {
	if len(roles) == 0 {
		return false
	}
	guids := make([]uint64, 0, len(roles))
	for guid := range roles {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	damage, tank, healer := 0, 0, 0
	for _, guid := range guids {
		role := roles[guid] &^ LFGRoleLeader
		if role == LFGRoleNone {
			return false
		}
		if role&LFGRoleDamage != 0 {
			if role != LFGRoleDamage {
				roles[guid] -= LFGRoleDamage
				if lfgCheckGroupRoles(roles) {
					return true
				}
				roles[guid] += LFGRoleDamage
			} else if damage == lfgDPSNeeded {
				return false
			} else {
				damage++
			}
		}
		if role&LFGRoleHealer != 0 {
			if role != LFGRoleHealer {
				roles[guid] -= LFGRoleHealer
				if lfgCheckGroupRoles(roles) {
					return true
				}
				roles[guid] += LFGRoleHealer
			} else if healer == lfgHealersNeeded {
				return false
			} else {
				healer++
			}
		}
		if role&LFGRoleTank != 0 {
			if role != LFGRoleTank {
				roles[guid] -= LFGRoleTank
				if lfgCheckGroupRoles(roles) {
					return true
				}
				roles[guid] += LFGRoleTank
			} else if tank == lfgTanksNeeded {
				return false
			} else {
				tank++
			}
		}
	}
	return tank+healer+damage == len(roles)
}

func (m *LFGManager) CreateProposal(dungeonID uint32, groupID uint64, leaderGUID uint64, players map[uint64]uint8) *LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextProposalID++
	prop := &LFGProposal{
		ID:         m.nextProposalID,
		DungeonID:  dungeonID,
		State:      LFGProposalInitiating,
		Group:      groupID,
		Leader:     leaderGUID,
		CancelTime: time.Now().Add(45 * time.Second),
		Players:    make(map[uint64]*LFGProposalPlayer, len(players)),
	}
	for guid, role := range players {
		prop.Players[guid] = &LFGProposalPlayer{
			Role:   role,
			Group:  groupID,
			Accept: LFGAnswerPending,
		}
	}
	m.proposals[prop.ID] = prop
	return prop
}

func (m *LFGManager) GetProposal(proposalID uint32) *LFGProposal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.proposals[proposalID]
}

func (m *LFGManager) UpdateProposal(proposalID uint32, guid uint64, accept bool) (*LFGProposal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prop, ok := m.proposals[proposalID]
	if !ok {
		return nil, false
	}
	player, ok := prop.Players[guid]
	if !ok {
		return nil, false
	}
	if !accept {
		player.Accept = LFGAnswerDeny
		prop.State = LFGProposalFailed
		delete(m.proposals, proposalID)
		return prop, true
	}

	player.Accept = LFGAnswerAgree
	allAgree := true
	for _, p := range prop.Players {
		if p.Accept != LFGAnswerAgree {
			allAgree = false
			break
		}
	}
	// LFGMgr::UpdateProposal (LFGMgr.cpp:1228): with SoloLFG on, the
	// proposal completes on the first accept without waiting for the other
	// players' answers (!IsSoloLFG() && !allAnswered waits); otherwise all
	// must agree.
	if allAgree || m.Solo() {
		prop.State = LFGProposalSuccess
		delete(m.proposals, proposalID)
	}
	return prop, true
}

// canJoinDungeonFinder mirrors the HasPermission gate at the head of
// LFGMgr::JoinLfg's requirement chain (LFGMgr.cpp:440): an account without
// RBAC_PERM_JOIN_DUNGEON_FINDER fails with LFG_JOIN_NOT_MEET_REQS before any
// other gate. An unavailable auth store counts as unprivileged.
func (s *session) canJoinDungeonFinder() bool {
	if s == nil || s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil {
		return false
	}
	granted, err := accountHasPermission(context.Background(), s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionJoinDungeonFinder)
	return err == nil && granted
}

func (s *session) handleLFGJoin(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// HandleLfgJoinOpcode (LFGHandler.cpp:51-56): joining requires the
	// dungeon-finder or raid-browser option enabled, and a grouped non-leader
	// is silently dropped when the group is full (MAXGROUPSIZE) or not an LFG group.
	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil &&
		s.server.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return true
	}
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil && grp.LeaderGUID != s.playerGUID &&
			(len(grp.Members) == maxGroupSize || !grp.IsLFG) {
			return true
		}
	}
	// Truncated reads are dropped, never close: C++ ByteBuffer exceptions are
	// caught in WorldSession::Update as "Skipped packet" (WorldSession.cpp:425-430).
	b := protocol.NewReader(payload)
	roles, err := b.ReadU32()
	if err != nil {
		return true
	}
	if _, err := b.ReadBool(); err != nil {
		return true
	}
	if _, err := b.ReadBool(); err != nil {
		return true
	}
	count, err := b.ReadU8()
	if err != nil || count > 50 {
		return true
	}
	dungeons := make([]uint32, 0, count)
	for index := uint8(0); index < count; index++ {
		dungeon, readErr := b.ReadU32()
		if readErr != nil {
			return true
		}
		dungeons = append(dungeons, dungeon&0x00FFFFFF)
	}
	// LFGJoin::Read (LFGPackets.cpp:28-30): the needs count byte is skipped
	// and exactly 3 needs bytes follow ("hardcoded to 3 in client").
	if _, err := b.ReadU8(); err != nil {
		return true
	}
	if _, err := b.Read(3); err != nil {
		return true
	}
	comment, err := b.ReadCString()
	if err != nil {
		return true
	}
	if len(comment) > 255 {
		comment = comment[:255]
	}
	roles8 := uint8(roles)
	// HandleLfgJoinOpcode (LFGHandler.cpp:59-61): no dungeons selected is a
	// silent return — no join-result packet.
	if len(dungeons) == 0 {
		return true
	}
	// HandleLfgJoinOpcode (LFGHandler.cpp:62-67): the handler keeps every
	// DBC-known dungeon (the slot's type bits were already masked above) —
	// unsupported types are rejected downstream by the JoinLfg type loop
	// (LFGMgr.cpp:527-557) with LFG_JOIN_DUNGEON_INVALID, not dropped here.
	filtered := make([]uint32, 0, len(dungeons))
	if s.server != nil && s.server.Data != nil {
		for _, dungeonID := range dungeons {
			if _, found, derr := s.server.Data.LFGDungeon(dungeonID); derr == nil && found {
				filtered = append(filtered, dungeonID)
			}
		}
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:405): an empty post-filter set is a silent
	// return, no join-result packet.
	if len(filtered) == 0 {
		return true
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:408-414): no tank/healer/damage role is a
	// silent return, no join-result packet.
	if roles8&(LFGRoleTank|LFGRoleHealer|LFGRoleDamage) == 0 {
		return true
	}
	failJoin := func(result uint32) bool {
		if err := s.sendLFGJoinResult(result, uint32(LFGStateNone)); err != nil {
			return false
		}
		return true
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:422-428): a grouped LFG mid-dungeon
	// re-queue (isContinue) cannot change dungeons — the requested set is
	// replaced by the group's current dungeon (grp.LFGDungeonID is the
	// GetDungeon(gguid) analog, set at proposal accept like C++'s SetDungeon
	// at LFGMgr.cpp:1147) before the requirement gates run.
	isContinue := false
	joinDungeons := filtered
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil && grp.IsLFG && grp.LFGState != LFGStateFinishedDungeon {
			isContinue = true
			if grp.LFGDungeonID != 0 {
				joinDungeons = []uint32{grp.LFGDungeonID}
			}
		}
	}
	// LFGMgr::JoinLfg requirement gates (LFGMgr.cpp:440-466), solo-player arms
	// in C++ order; the group-member arms follow at LFGMgr.cpp:467-526.
	if !s.canJoinDungeonFinder() {
		// HasPermission(rbac::RBAC_PERM_JOIN_DUNGEON_FINDER) (LFGMgr.cpp:440)
		return failJoin(LFGJoinNotMeetReqs)
	}
	if s.bgData.InstanceID != 0 || s.inBattlegroundQueue() {
		// InBattleground/InArena/InBattlegroundQueue (Player.h:1906,
		// Player.cpp:23647/23554)
		return failJoin(LFGJoinUsingBGSystem)
	}
	if s.hasAura(lfgSpellDungeonDeserter) {
		return failJoin(LFGJoinDeserter)
	}
	// isContinue skips the random-cooldown gate (LFGMgr.cpp:447).
	if !isContinue && s.hasAura(lfgSpellDungeonCooldown) {
		return failJoin(LFGJoinRandomCooldown)
	}
	if s.hasAura(9454) { // Freeze debuff (LFGMgr.cpp:465)
		return failJoin(LFGJoinNotMeetReqs)
	}
	// LFGMgr::JoinLfg group-member restriction arms (LFGMgr.cpp:467-526).
	// The queue stays per-player (documented model delta), but the PARTY_*
	// validation results are bridged: the MAXGROUPSIZE gate, then per-member
	// RBAC / deserter / random-cooldown / BG / freeze checks in C++ arm
	// order. The RBAC check is a standalone if (LFGMgr.cpp:476-477), so a
	// member's deserter aura overwrites its PARTY_NOT_MEET_REQS; the member
	// BG arm carries the plain USING_BG_SYSTEM result (LFGMgr.cpp:489-490),
	// not a PARTY_* variant. The npcbot block is fork-specific and skipped.
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil {
			if len(grp.Members) > 5 { // MAXGROUPSIZE (Group.h:42)
				return failJoin(LFGJoinTooMuchMembers)
			}
			memberCount := 0
			groupResult := LFGJoinOK
			for _, m := range grp.Members {
				ms := s.server.playerSessionForGUID(m.GUID)
				if ms == nil {
					continue // GetSource() null: counted as disconnected below
				}
				if !ms.canJoinDungeonFinder() {
					groupResult = LFGJoinPartyNotMeetReqs
				}
				if ms.hasAura(lfgSpellDungeonDeserter) {
					groupResult = LFGJoinPartyDeserter
				} else if !isContinue && ms.hasAura(lfgSpellDungeonCooldown) {
					groupResult = LFGJoinPartyRandomCooldown
				} else if ms.bgData.InstanceID != 0 || ms.inBattlegroundQueue() {
					groupResult = LFGJoinUsingBGSystem
				} else if ms.hasAura(9454) { // Freeze debuff (LFGMgr.cpp:491-492)
					groupResult = LFGJoinPartyNotMeetReqs
				}
				memberCount++
				if groupResult != LFGJoinOK {
					break
				}
			}
			// LFGMgr.cpp:522-523: members without a live session count as
			// disconnected.
			if groupResult == LFGJoinOK && memberCount != len(grp.Members) {
				groupResult = LFGJoinDisconnected
			}
			if groupResult != LFGJoinOK {
				return failJoin(groupResult)
			}
		}
	}
	// LFGMgr::JoinLfg dungeon-type validation (LFGMgr.cpp:527-557): a random
	// dungeon must be queued alone; raid and non-raid dungeons cannot mix. The
	// loop stops at the first failure, like the C++ result == LFG_JOIN_OK
	// continuation condition.
	isRaid := false
	isDungeon := false
	typeFail := LFGJoinOK
	for _, dungeonID := range joinDungeons {
		dungeon, found, derr := s.server.Data.LFGDungeon(dungeonID)
		dtype := uint32(0)
		if derr == nil && found {
			dtype = dungeon.TypeID
		}
		switch dtype {
		case LFGDungeonTypeRandom:
			if len(joinDungeons) > 1 {
				typeFail = LFGJoinInvalidDungeon
				break
			}
			fallthrough
		case LFGDungeonTypeHeroic, LFGDungeonTypeDungeon:
			if isRaid {
				typeFail = LFGJoinMixedRaidDungeon
			} else {
				isDungeon = true
			}
		case LFGDungeonTypeRaid:
			if isDungeon {
				typeFail = LFGJoinMixedRaidDungeon
			} else {
				isRaid = true
			}
		default:
			typeFail = LFGJoinInvalidDungeon
		}
		if typeFail != LFGJoinOK {
			break
		}
	}
	if typeFail != LFGJoinOK {
		return failJoin(typeFail)
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:558-567): locked dungeons are dropped from
	// the requested set for every queued player — the joiner alone when
	// solo, every live group member when grouped (the C++ players set). When
	// nothing compatible remains the join fails — PARTY_NOT_MEET_REQS
	// grouped, NOT_MEET_REQS solo — and the join result carries the lock map
	// (LFGHandler.cpp:450). The random-dungeon expansion (LFGMgr.cpp:562)
	// has no Go twin: the queued set keeps the random id, resolved at
	// match time (lfgSelectedRandomDungeon), so random ids skip the filter.
	var joinMeta []lfgJoinDungeon
	if s.server != nil && s.server.Data != nil {
		for _, dungeonID := range joinDungeons {
			if dungeon, found, derr := s.server.Data.LFGDungeon(dungeonID); derr == nil && found {
				joinMeta = append(joinMeta, lfgJoinDungeon{id: dungeonID, typeID: dungeon.TypeID, mapID: uint32(dungeon.MapID), difficulty: dungeon.Difficulty})
			}
		}
	}
	compatPlayers := []uint64{s.playerGUID}
	grouped := false
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil {
			grouped = true
			compatPlayers = compatPlayers[:0]
			for _, m := range grp.Members {
				if s.server.playerSessionForGUID(m.GUID) != nil {
					compatPlayers = append(compatPlayers, m.GUID)
				}
			}
		}
	}
	if _, lockMap := s.lfgCompatibleDungeons(joinMeta, compatPlayers, isContinue); lockMap != nil {
		result := LFGJoinNotMeetReqs
		if grouped {
			result = LFGJoinPartyNotMeetReqs
		}
		return s.sendLFGJoinResultWithLocks(result, uint32(LFGStateNone), lockMap) == nil
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:577-581): raid-browser joins queue nothing —
	// the raid path ends here silently.
	if isRaid {
		return true
	}
	// LFGMgr::JoinLfg (LFGMgr.cpp:683-691): on isContinue the stored
	// selection is not overwritten (the SetSelectedDungeons skip) — the
	// player's original pick (e.g. the random id driving luck-of-the-draw)
	// survives the mid-dungeon re-queue.
	storeDungeons := joinDungeons
	if isContinue {
		if old, ok := s.server.Features.LFG.Status(s.playerGUID); ok && len(old.Dungeons) > 0 {
			storeDungeons = old.Dungeons
		}
	}
	result, entry := s.server.Features.LFG.Join(s.playerGUID, roles8, storeDungeons, comment)
	s.debug("lfg join", "account", s.accountName, "result", result, "dungeons", entry.Dungeons)
	if err := s.sendLFGJoinResult(result, uint32(LFGStateNone)); err != nil {
		return true
	}
	if result == LFGJoinOK {
		// The join-queue update carries the requested (possibly replaced)
		// set, like C++'s SendLfgUpdatePlayer(JOIN_QUEUE, dungeons).
		if isContinue {
			entry.Dungeons = joinDungeons
		}
		return s.sendLFGUpdatePlayer(LFGUpdateJoinQueue, entry) == nil
	}
	return true
}

// inBattlegroundQueue mirrors Player::InBattlegroundQueue (Player.cpp:23554):
// any active battleground/arena queue slot.
func (s *session) inBattlegroundQueue() bool {
	for i := range s.bgQueues {
		if s.bgQueues[i].Active {
			return true
		}
	}
	return false
}

func (s *session) handleLFGLeave() bool {
	if !s.playerLoaded {
		return true
	}
	// HandleLfgLeaveOpcode (LFGHandler.cpp:78): only the group leader may
	// leave the queue — a grouped non-leader's request is a silent no-op.
	if s.groupID != 0 && s.server != nil {
		if g := s.server.getGroup(s.groupID); g != nil && g.LeaderGUID != s.playerGUID {
			return true
		}
	}
	// LFGMgr::LeaveLfg (LFGMgr.cpp:714-749): the removed-from-queue update
	// goes out only from the QUEUED arm — a leave with nothing queued sends
	// nothing.
	if !s.server.Features.LFG.Leave(s.playerGUID) {
		return true
	}
	s.debug("lfg leave", "account", s.accountName)
	return s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone}) == nil
}

func (s *session) handleLFGGetStatus() bool {
	if !s.playerLoaded {
		return true
	}
	entry, _ := s.server.Features.LFG.Status(s.playerGUID)
	// HandleLfgGetStatus (LFGHandler.cpp:277): grouped players receive the
	// party update first, then the player update with dungeons cleared;
	// solo players receive the player update first, then the cleared party
	// update. The group-queue model has no Go counterpart, so the party
	// send carries the player's own queue entry (documented delta).
	cleared := entry
	cleared.Dungeons = nil
	cleared.Comment = ""
	if s.groupID != 0 {
		if err := s.sendLFGUpdateParty(LFGUpdateStatus, entry); err != nil {
			return true
		}
		return s.sendLFGUpdatePlayer(LFGUpdateStatus, cleared) == nil
	}
	if err := s.sendLFGUpdatePlayer(LFGUpdateStatus, entry); err != nil {
		return true
	}
	return s.sendLFGUpdateParty(LFGUpdateStatus, cleared) == nil
}

func (s *session) sendLFGJoinResult(result uint32, state uint32) error {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(result)
	packet.WriteU32(state)
	return s.write(uint16(protocol.OpcodeSMSG_LFG_JOIN_RESULT), packet.Bytes(), true)
}

// sendLFGJoinResultWithLocks mirrors WorldSession::SendLfgJoinResult
// (LFGHandler.cpp:438-453): the party lock block follows result+state only
// when the lock map is non-empty — JoinLfg clears it whenever a compatible
// dungeon remains (LFGMgr.cpp:571-572). Party members serialize in
// ascending guid order (std::map), dungeon entries in ascending id order.
func (s *session) sendLFGJoinResultWithLocks(result, state uint32, lockMap map[uint64]map[uint32]uint32) error {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(result)
	packet.WriteU32(state)
	if len(lockMap) > 0 {
		guids := make([]uint64, 0, len(lockMap))
		for guid := range lockMap {
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		packet.WriteU8(uint8(len(guids)))
		for _, guid := range guids {
			locks := lockMap[guid]
			packet.WriteU64(guid)
			entries := make([]uint32, 0, len(locks))
			for dungeon := range locks {
				entries = append(entries, dungeon)
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
			packet.WriteU32(uint32(len(entries)))
			for _, dungeon := range entries {
				packet.WriteU32(dungeon)
				packet.WriteU32(locks[dungeon])
			}
		}
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_JOIN_RESULT), packet.Bytes(), true)
}

func (s *session) sendLFGUpdatePlayer(updateType uint8, entry LFGQueueEntry) error {
	// SendLfgUpdatePlayer (LFGHandler.cpp:293): the queued flag derives
	// from the update type and state, it is not a constant.
	queued := false
	switch updateType {
	case LFGUpdateJoinQueue, LFGUpdateAddedToQueue:
		queued = true
	case LFGUpdateStatus:
		queued = entry.State == LFGStateQueued
	}
	packet := protocol.NewBuffer(16 + len(entry.Dungeons)*4 + len(entry.Comment))
	packet.WriteU8(updateType)
	// SendLfgUpdatePlayer (LFGHandler.cpp:310-312): the joined flag is
	// (size > 0) — the entry state plays no part.
	if len(entry.Dungeons) == 0 {
		packet.WriteU8(0)
	} else {
		packet.WriteU8(1)
		packet.WriteU8(boolToU8(queued))
		packet.WriteU8(0)
		packet.WriteU8(0)
		packet.WriteU8(uint8(len(entry.Dungeons)))
		for _, dungeon := range entry.Dungeons {
			packet.WriteU32(dungeon)
		}
		packet.WriteCString(entry.Comment)
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_UPDATE_PLAYER), packet.Bytes(), true)
}

// buildLFGUpdateParty mirrors WorldSession::SendLfgUpdateParty
// (LFGHandler.cpp:334): the join/queued flags derive from the update type
// and state, followed by the two zero bytes, three needs bytes, the
// dungeon list and the comment.
func buildLFGUpdateParty(updateType uint8, entry LFGQueueEntry) []byte {
	join := false
	queued := false
	switch updateType {
	case LFGUpdateAddedToQueue:
		queued = true
		join = true
	case LFGUpdateProposalBegin:
		join = true
	case LFGUpdateStatus:
		join = entry.State != LFGStateRoleCheck && entry.State != LFGStateNone
		queued = entry.State == LFGStateQueued
	}
	packet := protocol.NewBuffer(16 + len(entry.Dungeons)*4 + len(entry.Comment))
	packet.WriteU8(updateType)
	if len(entry.Dungeons) == 0 {
		packet.WriteU8(0)
		return packet.Bytes()
	}
	packet.WriteU8(1)
	packet.WriteU8(boolToU8(join))
	packet.WriteU8(boolToU8(queued))
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(uint8(len(entry.Dungeons)))
	for _, dungeon := range entry.Dungeons {
		packet.WriteU32(dungeon)
	}
	packet.WriteCString(entry.Comment)
	return packet.Bytes()
}

func (s *session) sendLFGUpdateParty(updateType uint8, entry LFGQueueEntry) error {
	return s.write(uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY), buildLFGUpdateParty(updateType, entry), true)
}

func (s *session) getLFGDungeonEntrance(dungeonID uint32) (mapID uint32, x, y, z, ori float32) {
	baseID := dungeonID & 0x00FFFFFF
	var dungeonType uint32
	if s.server != nil && s.server.Data != nil {
		dungeon, found, _ := s.server.Data.LFGDungeon(baseID)
		if found {
			if dungeon.MapID >= 0 {
				mapID = uint32(dungeon.MapID)
			}
			dungeonType = dungeon.TypeID
		}
	}
	var posX, posY, posZ, orientation float32
	hasRow := false
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		err := s.server.WorldStore.DB.QueryRow(
			"SELECT position_x, position_y, position_z, orientation FROM lfg_dungeon_template WHERE dungeonId = ? LIMIT 1",
			baseID,
		).Scan(&posX, &posY, &posZ, &orientation)
		hasRow = err == nil
	}
	// LFGMgr::LoadLFGDungeons (LFGMgr.cpp:260-273): with no usable
	// lfg_dungeon_template row, non-random dungeons fall back to the
	// map-entrance areatrigger (ObjectMgr::GetMapEntranceTrigger).
	if (!hasRow || (posX == 0 && posY == 0 && posZ == 0)) && dungeonType != LFGDungeonTypeRandom {
		if tx, ty, tz, to, ok := s.instanceEntranceTrigger(context.Background(), mapID); ok {
			return mapID, tx, ty, tz, to
		}
	}
	return mapID, posX, posY, posZ, orientation
}

// lfgSelectedRandomDungeon mirrors LFGMgr::selectedRandomLfgDungeon
// (LFGMgr.cpp:2250-2262): true when the player's queued LFG selection starts
// with a random or seasonal dungeon entry.
func (s *Server) lfgSelectedRandomDungeon(guid uint64) bool {
	if s == nil || s.Features == nil || s.Features.LFG == nil || s.Data == nil {
		return false
	}
	entry, ok := s.Features.LFG.Status(guid)
	if !ok || entry.State == LFGStateNone || len(entry.Dungeons) == 0 {
		return false
	}
	dungeon, found, err := s.Data.LFGDungeon(entry.Dungeons[0] & 0x00FFFFFF)
	if err != nil || !found {
		return false
	}
	return dungeon.TypeID == LFGDungeonTypeRandom || dungeon.Flags&lfgFlagSeasonal != 0
}

// lfgGroupMemberRemoved mirrors the LFGMgr-state-free arms of
// LFGGroupScript::OnRemoveMember (LFGScripts.cpp:183-228): the Dungeon
// Deserter cast on mid-dungeon leaves, the Dungeon Cooldown aura strip on
// KICK_LFG removals, the SMSG_LFG_UPDATE_PARTY ping, and the teleport-out of
// the dungeon. The per-group data writes (SetGroup/RemovePlayerFromGroup/
// RemoveGroupData) and SendLfgOfferContinue have no Go counterpart and stay
// documented no-bridge: Go keeps no per-group LFG data.
func (s *Server) lfgGroupMemberRemoved(g *groupState, target *session, method uint8) {
	if s == nil || g == nil || !g.IsLFG || target == nil || target.player == nil {
		return
	}
	if method == groupRemoveMethodLeave && g.LFGState == LFGStateDungeon &&
		len(g.Members) >= lfgGroupKickVotesNeeded {
		target.applyAura(lfgSpellDungeonDeserter)
	} else if method == groupRemoveMethodKickLFG {
		target.removeAura(lfgSpellDungeonCooldown)
	}
	_ = target.sendLFGUpdateParty(LFGUpdateLeaderUnk1, LFGQueueEntry{})
	if target.isDungeonMap(target.player.Map) {
		target.teleportToBGEntryPoint()
	}
}

// lfgPlayerBoot mirrors lfg::LfgPlayerBoot (LFGMgr.h:260-267): one active
// kick vote on an LFG group.
type lfgPlayerBoot struct {
	cancelTime int64 // unix seconds; expires when cancelTime < now
	inProgress bool
	votes      map[uint64]int8 // LFGAnswerPending/Deny/Agree per member guid
	victim     uint64
	reason     string
}

// initLFGBoot mirrors LFGMgr::InitBoot (LFGMgr.cpp:1406-1432): every group
// member starts PENDING, the victim auto-votes DENY, the kicker auto-votes
// AGREE, and every member (victim included) gets the proposal packet.
// SetVoteKick(gguid, true) is the boot's presence in lfgBoots; Go never sets
// LFG_STATE_BOOT as a group state — C++ doesn't either (only the
// m_VoteKickActive flag moves).
func (s *Server) initLFGBoot(g *groupState, kickerGUID, victimGUID uint64, reason string) {
	if s == nil || g == nil {
		return
	}
	boot := &lfgPlayerBoot{
		cancelTime: time.Now().Unix() + lfgTimeBoot,
		inProgress: true,
		votes:      make(map[uint64]int8, len(g.Members)),
		victim:     victimGUID,
		reason:     reason,
	}
	for _, m := range g.Members {
		boot.votes[m.GUID] = LFGAnswerPending
	}
	boot.votes[victimGUID] = LFGAnswerDeny
	boot.votes[kickerGUID] = LFGAnswerAgree
	s.groupsMu.Lock()
	if s.lfgBoots == nil {
		s.lfgBoots = make(map[uint64]*lfgPlayerBoot)
	}
	s.lfgBoots[g.ID] = boot
	members := append([]groupMember(nil), g.Members...)
	s.groupsMu.Unlock()
	for _, m := range members {
		if ps := s.findSessionByGUID(m.GUID); ps != nil {
			_ = ps.sendLFGBootProposalUpdate(boot)
		}
	}
}

// lfgBootVote mirrors LFGMgr::UpdateBoot (LFGMgr.cpp:1439-1496): records one
// vote, then finalizes when the outcome is decided — agreeNum reached
// LFG_GROUP_KICK_VOTES_NEEDED, or too few votes remain to reach it. The
// finalize notifies every voter except the victim, clears the vote-kick
// state, and on exactly LFG_GROUP_KICK_VOTES_NEEDED agrees removes the victim
// with GROUP_REMOVEMETHOD_KICK_LFG and spends one kick.
func (s *Server) lfgBootVote(ctx context.Context, voter *session, accept bool) {
	if s == nil || voter == nil || voter.groupID == 0 {
		return
	}
	s.groupsMu.Lock()
	g := s.groups[voter.groupID]
	boot := s.lfgBoots[voter.groupID]
	if g == nil || boot == nil || !boot.inProgress {
		s.groupsMu.Unlock()
		return
	}
	if _, ok := boot.votes[voter.playerGUID]; !ok {
		// LFGMgr::UpdateBoot (LFGMgr.cpp:1452): std::map::operator[] value-inserts
		// LFG_ANSWER_DENY (0) for a voter missing from the boot table (e.g. joined
		// the group after the vote started); the cheat check then discards the
		// vote, but the inserted entry counts in SMSG_LFG_BOOT_PROPOSAL_UPDATE.
		boot.votes[voter.playerGUID] = LFGAnswerDeny
	}
	if boot.votes[voter.playerGUID] != LFGAnswerPending { // cheat check: no double vote
		s.groupsMu.Unlock()
		return
	}
	if accept {
		boot.votes[voter.playerGUID] = LFGAnswerAgree
	} else {
		boot.votes[voter.playerGUID] = LFGAnswerDeny
	}
	var agreeNum, denyNum uint8
	for _, v := range boot.votes {
		switch v {
		case LFGAnswerAgree:
			agreeNum++
		case LFGAnswerDeny:
			denyNum++
		}
	}
	// Not enough votes yet and the needed count is still reachable: wait.
	if agreeNum < lfgGroupKickVotesNeeded && uint8(len(boot.votes))-denyNum >= lfgGroupKickVotesNeeded {
		s.groupsMu.Unlock()
		return
	}
	boot.inProgress = false
	voters := make([]uint64, 0, len(boot.votes))
	for pguid := range boot.votes {
		if pguid != boot.victim {
			voters = append(voters, pguid)
		}
	}
	kick := agreeNum == lfgGroupKickVotesNeeded
	victimGUID := boot.victim
	if kick && g.LFGKicksLeft > 0 {
		g.LFGKicksLeft--
	}
	delete(s.lfgBoots, g.ID)
	groupID := g.ID
	s.groupsMu.Unlock()
	for _, pguid := range voters {
		if ps := s.findSessionByGUID(pguid); ps != nil {
			_ = ps.sendLFGBootProposalUpdate(boot)
		}
	}
	if kick {
		// C++ guards the kick on the group still existing
		// (sGroupMgr->GetGroupByGUID); the victim session must still be in
		// this group — a member who left mid-vote is not re-removed.
		if g2 := s.findGroupByID(groupID); g2 != nil {
			if vs := s.findSessionByGUID(victimGUID); vs != nil && vs.groupID == groupID {
				_ = vs.removeFromGroup(ctx, g2, vs, groupRemoveMethodKickLFG)
			} else if vs == nil {
				s.removeGroupMemberByGUID(ctx, g2, victimGUID, groupRemoveMethodKickLFG)
			}
		}
	}
}

// updateLFGBootVotes mirrors the boot-expiry arm of LFGMgr::Update
// (LFGMgr.cpp:331-345): a vote past cancelTime ends without a kick — the
// voters (victim excluded) get the closed proposal packet and the vote-kick
// state clears. No kicks are spent on expiry.
func (s *Server) updateLFGBootVotes(now time.Time) {
	if s == nil {
		return
	}
	nowSec := now.Unix()
	s.groupsMu.Lock()
	var expired []*lfgPlayerBoot
	for id, boot := range s.lfgBoots {
		if boot.cancelTime < nowSec {
			boot.inProgress = false
			expired = append(expired, boot)
			delete(s.lfgBoots, id)
		}
	}
	s.groupsMu.Unlock()
	for _, boot := range expired {
		for pguid := range boot.votes {
			if pguid == boot.victim {
				continue
			}
			if ps := s.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGBootProposalUpdate(boot)
			}
		}
	}
}

// buildLFGBootProposalUpdate mirrors WorldSession::SendLfgBootProposalUpdate
// (LFGHandler.cpp:512-545): SMSG_LFG_BOOT_PROPOSAL_UPDATE is built per
// recipient — didVote/agree describe that recipient's own vote. The secsleft
// field keeps the C++ quirk verbatim: cancelTime is in seconds but the
// packet divides by 1000, so it always writes 0.
func buildLFGBootProposalUpdate(boot *lfgPlayerBoot, recipientGUID uint64, nowSec int64) []byte {
	playerVote := LFGAnswerPending
	if v, ok := boot.votes[recipientGUID]; ok {
		playerVote = v
	}
	var votesNum, agreeNum uint32
	for _, v := range boot.votes {
		if v != LFGAnswerPending {
			votesNum++
			if v == LFGAnswerAgree {
				agreeNum++
			}
		}
	}
	remaining := boot.cancelTime - nowSec
	if remaining < 0 {
		remaining = 0
	}
	packet := protocol.NewBuffer(1 + 1 + 1 + 8 + 4 + 4 + 4 + 4 + len(boot.reason) + 1)
	packet.WriteU8(boolToU8(boot.inProgress))
	packet.WriteU8(boolToU8(playerVote != LFGAnswerPending))
	packet.WriteU8(boolToU8(playerVote == LFGAnswerAgree))
	packet.WriteU64(boot.victim)
	packet.WriteU32(votesNum)
	packet.WriteU32(agreeNum)
	packet.WriteU32(uint32(uint8(remaining / 1000)))
	packet.WriteU32(lfgGroupKickVotesNeeded)
	packet.WriteCString(boot.reason)
	return packet.Bytes()
}

func (s *session) sendLFGBootProposalUpdate(boot *lfgPlayerBoot) error {
	return s.write(uint16(protocol.OpcodeSMSG_LFG_BOOT_PROPOSAL_UPDATE), buildLFGBootProposalUpdate(boot, s.playerGUID, time.Now().Unix()), true)
}

// canStartLFGBootVote mirrors the LFG branch of Player::CanUninviteFromGroup
// (Player.cpp:24426-24452). Any group member may open a vote — there is no
// leader check for LFG groups. Returns errPartyResultOK (0) when the vote may
// start.
func (s *session) canStartLFGBootVote(g *groupState) uint32 {
	if g.LFGKicksLeft == 0 {
		return errPartyLFGBootLimit
	}
	s.server.groupsMu.RLock()
	_, active := s.server.lfgBoots[g.ID]
	members := append([]groupMember(nil), g.Members...)
	s.server.groupsMu.RUnlock()
	if active {
		return errPartyLFGBootInProgress
	}
	if len(members) <= lfgGroupKickVotesNeeded {
		return errPartyLFGBootTooFewPlayers
	}
	if g.LFGState == LFGStateFinishedDungeon {
		return errPartyLFGBootDungeonComplete
	}
	// Group::isRollLootActive: any active roll owned by this group.
	s.server.lootMu.Lock()
	for _, roll := range s.server.groupRolls {
		if roll != nil && roll.GroupID == g.ID {
			s.server.lootMu.Unlock()
			return errPartyLFGBootLootRolls
		}
	}
	s.server.lootMu.Unlock()
	// Any member in the kicker's map instance who is in combat blocks the vote.
	for _, m := range members {
		ms := s.server.findSessionByGUID(m.GUID)
		if ms == nil || ms.player == nil || s.player == nil {
			continue
		}
		if ms.player.Map == s.player.Map && ms.player.InstanceID == s.player.InstanceID && ms.isInCombat() {
			return errPartyLFGBootInCombat
		}
	}
	return errPartyResultOK
}

// lfgBootTargetInGroup reports whether guid is a current member of g.
func lfgBootTargetInGroup(g *groupState, guid uint64) bool {
	for _, m := range g.Members {
		if m.GUID == guid {
			return true
		}
	}
	return false
}

// teleportToBGEntryPoint mirrors the out=true leg of LFGMgr::TeleportPlayer
// (LFGMgr.cpp:1529-1537) via Player::TeleportToBGEntryPoint
// (Player.cpp:1896-1906): the player returns to the battleground entry point
// recorded by setBattlegroundEntryPoint on the way in. An unset entry point
// teleports nowhere (the MAPID_INVALID leg) — this C++ tree keeps no separate
// LFG entry point and has no Stormwind fallback.
func (s *session) teleportToBGEntryPoint() {
	if s == nil || s.player == nil {
		return
	}
	if s.bgData.JoinMap == 0 {
		return
	}
	s.teleportTo(s.bgData.JoinMap, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO)
}

func (s *session) teleportToLFGDungeon(dungeonID uint32, fromOpcode bool) {
	if s.player == nil {
		return
	}
	inDungeon := false
	if s.server != nil && s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(s.player.Map); err == nil && found {
			inDungeon = mapInfo.IsDungeon()
		}
	}
	if !inDungeon {
		s.setBattlegroundEntryPoint()
	}
	s.finishTaxiFlight()

	mapID, x, y, z, ori := s.getLFGDungeonEntrance(dungeonID)
	// LFGMgr::TeleportPlayer (LFGMgr.cpp:1550-1567): with fromOpcode=false
	// the destination is an LFG group member already inside the dungeon
	// when one exists, falling back to the dungeon entrance; the opcode
	// path calls with fromOpcode=true and always uses the entrance.
	if !fromOpcode && s.groupID != 0 && s.server != nil && mapID != 0 {
		s.server.groupsMu.RLock()
		var members []groupMember
		isLFG := false
		if grp := s.server.groups[s.groupID]; grp != nil {
			isLFG = grp.IsLFG
			members = append([]groupMember(nil), grp.Members...)
		}
		s.server.groupsMu.RUnlock()
		if isLFG {
			for _, m := range members {
				if m.GUID == s.playerGUID {
					continue
				}
				if ms := s.server.findSessionByGUID(m.GUID); ms != nil && ms.player != nil && ms.player.Map == mapID {
					x, y, z, ori = ms.player.X, ms.player.Y, ms.player.Z, ms.player.Orientation
					break
				}
			}
		}
	}
	// LFGPlayerScript::OnMapChanged (LFGScripts.cpp:93-131): arriving in the
	// LFG dungeon map casts Luck of the Draw when the player queued for a
	// random dungeon (LFGMgr::selectedRandomLfgDungeon, LFGMgr.cpp:2250-2262).
	// C++ carries no fixed duration — the aura is stripped on map change —
	// and Go has no map-change hook, so a 2h practical cap stands in.
	luck := s.server.lfgSelectedRandomDungeon(s.playerGUID)
	if s.teleportTo(mapID, x, y, z, ori) && luck {
		s.applyAuraWithDuration(lfgSpellLuckOfTheDraw, 7200000)
	}
}

func (s *session) sendLFGRoleCheckUpdate(roleCheck *LfgRoleCheck) error {
	packet := protocol.NewBuffer(64)
	packet.WriteU32(roleCheck.State)
	if roleCheck.State == LFGRoleCheckInitializing {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(uint8(len(roleCheck.Dungeons)))
	for _, d := range roleCheck.Dungeons {
		packet.WriteU32(d)
	}
	packet.WriteU8(uint8(len(roleCheck.Roles)))

	// Leader first
	leaderRoles := roleCheck.Roles[roleCheck.Leader]
	packet.WriteU64(roleCheck.Leader)
	if leaderRoles > 0 {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(leaderRoles))
	leaderLevel := uint8(80)
	if s.server != nil {
		if ls := s.server.findSessionByGUID(roleCheck.Leader); ls != nil && ls.player != nil {
			leaderLevel = ls.player.Level
		}
	}
	packet.WriteU8(leaderLevel)

	// Other members
	for pguid, roles := range roleCheck.Roles {
		if pguid == roleCheck.Leader {
			continue
		}
		packet.WriteU64(pguid)
		if roles > 0 {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU32(uint32(roles))
		level := uint8(80)
		if s.server != nil {
			if ms := s.server.findSessionByGUID(pguid); ms != nil && ms.player != nil {
				level = ms.player.Level
			}
		}
		packet.WriteU8(level)
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_ROLE_CHECK_UPDATE), packet.Bytes(), true)
}

func (s *session) sendLFGProposalUpdate(proposal *LFGProposal) error {
	guid := s.playerGUID
	packet := protocol.NewBuffer(64)
	packet.WriteU32(proposal.DungeonID)
	packet.WriteU8(proposal.State)
	packet.WriteU32(proposal.ID)
	packet.WriteU32(proposal.Encounters)
	if proposal.Silent {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(uint8(len(proposal.Players)))

	for pguid, pdata := range proposal.Players {
		packet.WriteU32(uint32(pdata.Role))
		if pguid == guid {
			packet.WriteU8(1) // isSelf
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU8(0) // inDungeon
		if s.groupID != 0 && pdata.Group == s.groupID {
			packet.WriteU8(1) // sameGroup
		} else {
			packet.WriteU8(0)
		}
		if pdata.Accept != LFGAnswerPending {
			packet.WriteU8(1) // answered
		} else {
			packet.WriteU8(0)
		}
		if pdata.Accept == LFGAnswerAgree {
			packet.WriteU8(1) // accepted
		} else {
			packet.WriteU8(0)
		}
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_PROPOSAL_UPDATE), packet.Bytes(), true)
}

func (s *session) getLockedDungeons(guid uint64) map[uint32]uint32 {
	locks := make(map[uint32]uint32)
	if s.server == nil || s.server.Data == nil {
		return locks
	}

	allDungeons, err := s.server.Data.LFGDungeons()
	if err != nil || len(allDungeons) == 0 {
		return locks
	}

	targetSess := s.server.findSessionByGUID(guid)
	var level uint8 = 80
	var expansion uint8 = s.accountExpansion
	if targetSess != nil {
		if targetSess.player != nil {
			level = targetSess.player.Level
		}
		expansion = targetSess.accountExpansion
	} else if guid == s.playerGUID && s.player != nil {
		level = s.player.Level
	}

	// 1. Check bound instances from character_instance JOIN instance
	type boundKey struct {
		mapID      uint32
		difficulty uint32
	}
	boundInstances := make(map[boundKey]bool)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		now := time.Now().Unix()
		rows, err := s.server.CharactersStore.DB.Query(
			"SELECT i.map, i.difficulty, i.resettime FROM character_instance ci JOIN instance i ON ci.instance = i.id WHERE ci.guid = ?",
			guid,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mapID, diff uint32
				var resetTime int64
				if err := rows.Scan(&mapID, &diff, &resetTime); err == nil {
					if resetTime > now {
						boundInstances[boundKey{mapID: mapID, difficulty: diff}] = true
					}
				}
			}
		}
	}

	// 2. Check access requirements (e.g. min item level)
	accessReqItemLevel := make(map[boundKey]uint32)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.Query(
			"SELECT mapId, difficulty, item_level FROM access_requirement WHERE item_level > 0",
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mapID, diff, itemLvl uint32
				if err := rows.Scan(&mapID, &diff, &itemLvl); err == nil {
					accessReqItemLevel[boundKey{mapID: mapID, difficulty: diff}] = itemLvl
				}
			}
		}
	}

	// 3. Compute player's average item level
	var avgItemLevel float32
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var sumItemLevel int64
		var countItemLevel int64
		var itemGUIDs []uint64
		rows, err := s.server.CharactersStore.DB.Query(
			"SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 0 AND slot <= 18 AND slot NOT IN (6, 14, 17, 18) AND item != 0",
			guid,
		)
		if err == nil {
			for rows.Next() {
				var itemGUID uint64
				if err := rows.Scan(&itemGUID); err == nil && itemGUID != 0 {
					itemGUIDs = append(itemGUIDs, itemGUID)
				}
			}
			rows.Close()

			for _, itemGUID := range itemGUIDs {
				var itemEntry uint32
				if err := s.server.CharactersStore.DB.QueryRow("SELECT itemEntry FROM item_instance WHERE guid = ?", itemGUID).Scan(&itemEntry); err == nil && itemEntry != 0 {
					var itemLvl int64
					if err := s.server.WorldStore.DB.QueryRow("SELECT ItemLevel FROM item_template WHERE entry = ?", itemEntry).Scan(&itemLvl); err == nil && itemLvl > 0 {
						sumItemLevel += itemLvl
					}
				}
				countItemLevel++
			}
			if countItemLevel > 0 {
				avgItemLevel = float32(sumItemLevel) / float32(countItemLevel)
			}
		}
	}

	// LFGMgr::GetLockedDungeons (LFGMgr.cpp:1852): denyJoin is computed once
	// and skips every other lock arm — without RBAC_PERM_JOIN_DUNGEON_FINDER
	// all dungeons report LFG_LOCKSTATUS_RAID_LOCKED. The permission belongs
	// to the target player's session, like C++'s FindConnectedPlayer(guid).
	denyJoin := true
	if targetSess != nil {
		denyJoin = !targetSess.canJoinDungeonFinder()
	} else {
		denyJoin = !s.canJoinDungeonFinder()
	}

	for _, dungeon := range allDungeons {
		if dungeon.TypeID == LFGDungeonTypeRandom {
			continue
		}

		var lockData uint32
		if denyJoin {
			lockData = LFGLockStatusRaidLocked
		} else if dungeon.ExpansionLevel > uint32(expansion) {
			lockData = LFGLockStatusInsufficientExpansion
		} else if uint32(level) < dungeon.MinLevel {
			lockData = LFGLockStatusTooLowLevel
		} else if dungeon.MaxLevel > 0 && uint32(level) > dungeon.MaxLevel {
			lockData = LFGLockStatusTooHighLevel
		} else if dungeon.Difficulty > 0 && boundInstances[boundKey{mapID: uint32(dungeon.MapID), difficulty: dungeon.Difficulty}] {
			lockData = LFGLockStatusRaidLocked
		} else if reqILvl, ok := accessReqItemLevel[boundKey{mapID: uint32(dungeon.MapID), difficulty: dungeon.Difficulty}]; ok && reqILvl > 0 && avgItemLevel < float32(reqILvl) {
			lockData = LFGLockStatusTooLowGearScore
		}

		if lockData != 0 {
			locks[dungeon.Entry()] = lockData
		}
	}

	return locks
}

// lfgJoinDungeon carries the DBC metadata GetCompatibleDungeons needs for
// the isContinue raid-locked exception.
type lfgJoinDungeon struct {
	id         uint32
	typeID     uint32
	mapID      uint32
	difficulty uint32
}

// lfgCompatibleDungeons mirrors LFGMgr::GetCompatibleDungeons
// (LFGMgr.cpp:888-935): for every queued player, locked dungeons are dropped
// from the requested set and recorded in the per-player lock map, keyed by
// masked dungeon id like C++'s lockMap[guid][dungeonId]; when compatible
// dungeons remain the lock map is cleared (LFGMgr.cpp:933-934). The random-
// dungeon expansion (LFGMgr.cpp:562) has no Go twin — the queued set keeps
// the random id and match-time resolution (lfgSelectedRandomDungeon) — so
// random ids pass through unfiltered. The isContinue raid-locked exception
// (LFGMgr.cpp:908-926) keeps a dungeon when every checked player is saved
// to the same instance of it.
func (s *session) lfgCompatibleDungeons(dungeons []lfgJoinDungeon, playerGUIDs []uint64, isContinue bool) ([]uint32, map[uint64]map[uint32]uint32) {
	remaining := make([]uint32, 0, len(dungeons))
	for _, d := range dungeons {
		remaining = append(remaining, d.id)
	}
	inRemaining := func(id uint32) bool {
		for _, r := range remaining {
			if r == id {
				return true
			}
		}
		return false
	}
	lockMap := make(map[uint64]map[uint32]uint32)
	lockedInstance := make(map[uint32]uint32)
	sorted := append([]uint64(nil), playerGUIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, pguid := range sorted {
		if len(remaining) == 0 {
			break
		}
		locks := s.getLockedDungeons(pguid)
		for _, d := range dungeons {
			if !inRemaining(d.id) {
				continue
			}
			status, locked := locks[d.id+(d.typeID<<24)]
			if !locked {
				continue
			}
			erase := true
			if status == LFGLockStatusRaidLocked && isContinue {
				if instanceID, bound := s.boundLFGInstanceID(pguid, d.mapID, d.difficulty); bound {
					if prev, seen := lockedInstance[d.id]; !seen || prev == instanceID {
						erase = false
					}
					lockedInstance[d.id] = instanceID
				}
			}
			if erase {
				kept := remaining[:0]
				for _, r := range remaining {
					if r != d.id {
						kept = append(kept, r)
					}
				}
				remaining = kept
			}
			if lockMap[pguid] == nil {
				lockMap[pguid] = make(map[uint32]uint32)
			}
			lockMap[pguid][d.id] = status
		}
	}
	if len(remaining) > 0 {
		lockMap = nil
	}
	return remaining, lockMap
}

// boundLFGInstanceID returns the instance id the character is saved to for
// the given map/difficulty — the GetBoundInstance arm backing
// GetCompatibleDungeons' isContinue raid-locked exception (LFGMgr.cpp:913).
func (s *session) boundLFGInstanceID(guid uint64, mapID, difficulty uint32) (uint32, bool) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, false
	}
	var instanceID uint32
	err := s.server.CharactersStore.DB.QueryRow(
		"SELECT ci.instance FROM character_instance ci JOIN instance i ON ci.instance = i.id WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? AND i.resettime > ?",
		guid, mapID, difficulty, time.Now().Unix(),
	).Scan(&instanceID)
	if err != nil || instanceID == 0 {
		return 0, false
	}
	return instanceID, true
}

// handleLfdPartyLockInfoRequest processes CMSG_LFD_PARTY_LOCK_INFO_REQUEST (0x371).
// Reference: WorldSession::HandleLfdPartyLockInfoRequestOpcode (LFGHandler.cpp:115).
func (s *session) handleLfdPartyLockInfoRequest(ctx context.Context, payload []byte) bool {
	// HandleLfgPartyLockInfoRequestOpcode (LFGHandler.cpp:225): without a
	// group the reference sends nothing at all.
	if !s.playerLoaded || s.server == nil || s.groupID == 0 {
		return true
	}

	grp := s.server.getGroup(s.groupID)
	if grp == nil {
		return true
	}

	type memberLock struct {
		guid  uint64
		locks map[uint32]uint32
	}
	var otherLocks []memberLock
	for _, m := range grp.Members {
		if m.GUID == s.playerGUID {
			continue
		}
		// HandleLfgPartyLockInfoRequestOpcode (LFGHandler.cpp:233-237):
		// members with no live session (GetSource() null) are skipped —
		// GetLockedDungeons early-returns empty for them anyway.
		if s.server.findSessionByGUID(m.GUID) == nil {
			continue
		}
		locks := s.getLockedDungeons(m.GUID)
		otherLocks = append(otherLocks, memberLock{guid: m.GUID, locks: locks})
	}
	// TrinityCore stores the party lock map in a std::map keyed by guid
	// (LFGHandler.cpp:233), so members serialize in ascending guid order,
	// not group order.
	sort.Slice(otherLocks, func(i, j int) bool { return otherLocks[i].guid < otherLocks[j].guid })

	buf := protocol.NewBuffer(64)
	buf.WriteU8(uint8(len(otherLocks)))
	for _, ml := range otherLocks {
		buf.WriteU64(ml.guid)
		buf.WriteU32(uint32(len(ml.locks)))
		entries := make([]uint32, 0, len(ml.locks))
		for entry := range ml.locks {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
		for _, entry := range entries {
			buf.WriteU32(entry)
			buf.WriteU32(ml.locks[entry])
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PARTY_INFO), buf.Bytes(), true)
	return true
}

// handleLfdPlayerLockInfoRequest processes CMSG_LFD_PLAYER_LOCK_INFO_REQUEST (0x36E).
// Reference: WorldSession::HandleLfgPlayerLockInfoRequestOpcode (LFGHandler.cpp:154).
func (s *session) handleLfdPlayerLockInfoRequest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}

	var randomDungeons []wotlk.LFGDungeon
	var lockedDungeons map[uint32]uint32

	if s.server != nil && s.server.Data != nil {
		allDungeons, err := s.server.Data.LFGDungeons()
		if err == nil {
			for _, d := range allDungeons {
				if d.TypeID == LFGDungeonTypeRandom {
					if d.ExpansionLevel <= uint32(s.accountExpansion) &&
						d.MinLevel <= uint32(s.player.Level) &&
						(d.MaxLevel == 0 || uint32(s.player.Level) <= d.MaxLevel) {
						randomDungeons = append(randomDungeons, d)
					}
				}
			}
		}
		lockedDungeons = s.getLockedDungeons(s.playerGUID)
	}

	bufSize := 5 + len(randomDungeons)*22 + len(lockedDungeons)*8
	buf := protocol.NewBuffer(bufSize)
	sort.Slice(randomDungeons, func(i, j int) bool { return randomDungeons[i].Entry() < randomDungeons[j].Entry() })
	buf.WriteU8(uint8(len(randomDungeons)))
	for _, rd := range randomDungeons {
		buf.WriteU32(rd.Entry())
		s.writeLFGRandomDungeonReward(ctx, buf, rd, uint8(s.player.Level))
	}

	buf.WriteU32(uint32(len(lockedDungeons)))
	if len(lockedDungeons) > 0 {
		entries := make([]uint32, 0, len(lockedDungeons))
		for entry := range lockedDungeons {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
		for _, entry := range entries {
			buf.WriteU32(entry)
			buf.WriteU32(lockedDungeons[entry])
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PLAYER_INFO), buf.Bytes(), true)
	return true
}

// questXPRewardRaw mirrors Quest::GetXPReward (QuestDef.cpp:202-228): the raw
// XP value written by HandleLfgPlayerLockInfoRequestOpcode, without the
// RATE_XP_QUEST grant-time multiplier.
func (s *session) questXPRewardRaw(ctx context.Context, questID, xpDiff uint32) uint32 {
	// Quest::GetXPReward (QuestDef.cpp:202-228) is the raw wire value written
	// by HandleLfgPlayerLockInfoRequestOpcode: the quest/player level
	// diffFactor and RoundXPValue apply, but RATE_XP_QUEST never does — C++
	// multiplies the rate only at grant time (Player.cpp:15401), so the
	// rate-baked calculateQuestRewardXP helper must not be used here.
	// CONFIG_MIN_QUEST_SCALED_XP_RATIO has no Go config knob and defaults to
	// 0 upstream, so the floor arm stays inert.
	if xpDiff >= 9 {
		return 0
	}
	var qLevel int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel FROM quest_template WHERE entry = ?", questID).Scan(&qLevel)
	}
	playerLevel := int64(0)
	if s.player != nil {
		playerLevel = int64(s.player.Level)
	}
	if qLevel <= 0 {
		qLevel = playerLevel
	}
	if qLevel < 1 {
		qLevel = 1
	} else if qLevel > 100 {
		qLevel = 100
	}
	entry, ok := questXPDBC[uint32(qLevel)]
	if !ok {
		return 0
	}
	diffFactor := 2*(qLevel-playerLevel) + 20
	if diffFactor < 1 {
		diffFactor = 1
	} else if diffFactor > 10 {
		diffFactor = 10
	}
	return roundQuestXPValue(uint32(diffFactor) * entry[xpDiff] / 10)
}

// roundQuestXPValue mirrors Quest::RoundXPValue (QuestDef.cpp:504-514).
func roundQuestXPValue(xp uint32) uint32 {
	if xp <= 100 {
		return 5 * ((xp + 2) / 5)
	} else if xp <= 500 {
		return 10 * ((xp + 5) / 10)
	} else if xp <= 1000 {
		return 25 * ((xp + 12) / 25)
	}
	return 50 * ((xp + 25) / 50)
}

// writeLFGRandomDungeonReward serializes the per-random-dungeon reward quest
// block of SMSG_LFG_PLAYER_INFO.
// Reference: WorldSession::HandleLfgPlayerLockInfoRequestOpcode (LFGHandler.cpp:154-222).
// LFGMgr::GetRandomDungeonReward (DungeonFinding/LFGMgr.cpp) picks the first
// lfg_dungeon_rewards row for the dungeon (entry bits 0-23, rows ordered by
// maxLevel ASC) whose maxLevel reaches the player level; the block shows the
// done flag when the player can no longer earn the quest (CanRewardQuest ==
// false) and then falls back to the row's otherQuest, otherwise it writes a
// zero quest block.
func (s *session) writeLFGRandomDungeonReward(ctx context.Context, buf *protocol.Buffer, dungeon wotlk.LFGDungeon, level uint8) {
	var firstQuest, otherQuest uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		// Mirror LFGMgr::GetRandomDungeonReward (LFGMgr.cpp:1725-1737): the
		// reward pointer is assigned BEFORE the maxLevel break check, so when
		// the player out-levels every row the last (highest-maxLevel) row is
		// used instead of no reward. Rows load ordered by maxLevel ASC
		// (LFGMgr.cpp:135), so keep the last row seen and break on the first
		// row that reaches the player level.
		rows, err := s.server.WorldStore.DB.QueryContext(ctx,
			"SELECT firstQuestId, otherQuestId, maxLevel FROM lfg_dungeon_rewards WHERE dungeonId = ? ORDER BY maxLevel ASC",
			dungeon.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var maxLevel uint8
				if scanErr := rows.Scan(&firstQuest, &otherQuest, &maxLevel); scanErr != nil {
					break
				}
				if maxLevel >= level {
					break
				}
			}
		}
	}
	var detail *questDetailData
	done := false
	if firstQuest != 0 {
		if d, err := s.loadQuestDetailData(ctx, firstQuest); err == nil {
			// Player::CanRewardQuest(quest, false) (Player.cpp:15059) fails
			// once the daily is turned in; the rewarded/can-take state mirrors
			// the daily, level, skill and reputation arms. The quest-status arm
			// is skipped for DF quests (Player.cpp:15066), so an in-progress
			// daily still shows done=0 — a canTake failure from quest-log
			// status alone must not set the flag.
			done = s.isQuestRewarded(ctx, firstQuest)
			if canTake, err := s.canTakeQuest(ctx, firstQuest); err == nil && !canTake {
				if status, serr := s.characterQuestStatus(ctx, firstQuest); serr != nil || status == 0 {
					done = true
				}
			}
			dd := d
			detail = &dd
			if done {
				// LFGHandler.cpp:184: a done daily falls back to otherQuest,
				// but GetQuestTemplate on a missing/zero otherQuest yields
				// nullptr, which writes the zero block — not the first quest
				// marked done.
				detail = nil
				if otherQuest != 0 {
					if od, err := s.loadQuestDetailData(ctx, otherQuest); err == nil {
						dd = od
						detail = &dd
					}
				}
			}
		}
	}
	if detail == nil {
		buf.WriteU8(0)
		buf.WriteU32(0)
		buf.WriteU32(0)
		buf.WriteU32(0)
		buf.WriteU32(0)
		buf.WriteU8(0)
		return
	}
	if done {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	// Quest::GetRewOrReqMoney writes as uint32; a negative required money
	// wraps the same way on the wire.
	buf.WriteU32(uint32(s.questRewOrReqMoney(*detail)))
	buf.WriteU32(s.questXPRewardRaw(ctx, detail.ID, detail.RewardXPDifficulty))
	buf.WriteU32(0)
	buf.WriteU32(0)
	count := uint8(0)
	for _, item := range detail.RewardItems {
		if item.ID != 0 {
			count++
		}
	}
	buf.WriteU8(count)
	for _, item := range detail.RewardItems {
		if item.ID == 0 {
			continue
		}
		buf.WriteU32(item.ID)
		var displayID uint32
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx,
				"SELECT COALESCE(displayid, 0) FROM item_template WHERE entry = ?", item.ID).Scan(&displayID)
		}
		buf.WriteU32(displayID)
		buf.WriteU32(item.Quantity)
	}
}

// handleLfgProposalResult processes CMSG_LFG_PROPOSAL_RESULT (0x362).
// Reference: WorldSession::HandleLfgProposalResultOpcode (LFGHandler.cpp:68).
func (s *session) handleLfgProposalResult(ctx context.Context, payload []byte) bool {
	if s.playerGUID == 0 {
		return true
	}
	if len(payload) < 5 {
		return true
	}
	r := protocol.NewReader(payload)
	proposalID, _ := r.ReadU32()
	acceptVal, _ := r.ReadU8()
	accept := int8(acceptVal) > 0
	s.debug("lfg proposal result", "account", s.accountName, "proposal", proposalID, "accept", accept)

	if s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return true
	}

	lfg := s.server.Features.LFG
	proposal, updated := lfg.UpdateProposal(proposalID, s.playerGUID, accept)
	if !updated || proposal == nil {
		return true
	}

	if !accept {
		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGProposalUpdate(proposal)
				_ = ps.sendLFGUpdatePlayer(LFGUpdateProposalDeclined, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
			}
		}
		return true
	}

	if proposal.State == LFGProposalSuccess {
		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGProposalUpdate(proposal)
				_ = ps.sendLFGUpdatePlayer(LFGUpdateGroupFound, LFGQueueEntry{GUID: pguid, State: LFGStateDungeon})
				_ = ps.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
			}
		}

		if proposal.Group != 0 {
			grp := s.server.getGroup(proposal.Group)
			if grp != nil {
				s.server.groupsMu.Lock()
				grp.IsLFG = true
				grp.LFGDungeonID = proposal.DungeonID
				grp.LFGState = LFGStateDungeon
				grp.LFGKicksLeft = lfgGroupMaxKicks
				dbGroupID := grp.DBID
				s.server.groupsMu.Unlock()
				if dbGroupID == 0 {
					dbGroupID = uint32(proposal.Group)
				}
				if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
					_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "REPLACE INTO lfg_data (guid, dungeon, state) VALUES (?, ?, ?)", dbGroupID, proposal.DungeonID, LFGStateDungeon)
				}
			}
		}

		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				// LFGMgr::UpdateProposal (LFGMgr.cpp:1127-1141): players who
				// queued for a random dungeon take the Dungeon Cooldown aura
				// on proposal accept.
				if s.server.lfgSelectedRandomDungeon(pguid) {
					ps.applyAura(lfgSpellDungeonCooldown)
				}
				ps.teleportToLFGDungeon(proposal.DungeonID, false)
			}
		}
		return true
	}

	// Still pending: broadcast updated progress to players
	for pguid := range proposal.Players {
		if ps := s.server.findSessionByGUID(pguid); ps != nil {
			_ = ps.sendLFGProposalUpdate(proposal)
		}
	}
	return true
}

// handleLfgSetBootVote processes CMSG_LFG_SET_BOOT_VOTE (0x36C).
// Reference: WorldSession::HandleLfgSetBootVoteOpcode (LFGHandler.cpp:133)
// -> LFGMgr::UpdateBoot (LFGMgr.cpp:1439).
func (s *session) handleLfgSetBootVote(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 {
		return true
	}
	agree := payload[0] != 0
	s.debug("lfg boot vote", "account", s.accountName, "agree", agree)
	s.server.lfgBootVote(ctx, s, agree)
	return true
}

// handleLfgSetRoles processes CMSG_LFG_SET_ROLES (0x36A).
// Reference: WorldSession::HandleLfgSetRolesOpcode (LFGHandler.cpp:104).
func (s *session) handleLfgSetRoles(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 {
		return true
	}
	// HandleLfgSetRolesOpcode (LFGHandler.cpp:104): groupless players are a
	// silent no-op — C++ returns before touching roles or achievements.
	if s.groupID == 0 {
		return true
	}
	roles := payload[0] & (LFGRoleTank | LFGRoleHealer | LFGRoleDamage | LFGRoleLeader)
	s.debug("lfg set roles", "account", s.accountName, "roles", roles)

	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		lfg := s.server.Features.LFG
		lfg.mu.Lock()
		if entry, ok := lfg.queue[s.playerGUID]; ok {
			entry.Roles = roles & (LFGRoleTank | LFGRoleHealer | LFGRoleDamage)
			lfg.queue[s.playerGUID] = entry
		}
		lfg.mu.Unlock()

		rc := lfg.UpdateRoleCheck(s.groupID, s.playerGUID, roles)
		if rc != nil {
			grp := s.server.getGroup(s.groupID)
			if grp != nil {
				for _, m := range grp.Members {
					if ms := s.server.findSessionByGUID(m.GUID); ms != nil {
						_ = ms.sendLFGRoleCheckUpdate(rc)
					}
				}
			}
			if rc.State == LFGRoleCheckFinished {
				s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
					buildLFGUpdateParty(LFGUpdateAddedToQueue, LFGQueueEntry{Dungeons: rc.Dungeons, State: LFGStateQueued}))
			}
		}
	}

	// SMSG_LFG_ROLE_CHOSEN (0x2BB): guid (8), ready (1), roles (4)
	packet := protocol.NewBuffer(13)
	packet.WriteU64(s.playerGUID)
	if roles > 0 {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(roles))
	if s.server != nil {
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_ROLE_CHOSEN), packet.Bytes())
	}
	return true
}

// hasCharmedCreature mirrors Unit::GetCharmedGUID (UNIT_FIELD_CHARM, Unit.h:1161)
// for LFGMgr::TeleportPlayer (LFGMgr.cpp:1538): true while the player controls a
// charmed creature, i.e. a live MOD_CHARM/AOE_CHARM creature aura on the player's
// map instance cast by the player.
func (s *session) hasCharmedCreature() bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	s.server.auraMu.Lock()
	defer s.server.auraMu.Unlock()
	for key, auras := range s.server.activeCreatureAuras {
		if key.Map != s.player.Map || key.InstanceID != s.player.InstanceID {
			continue
		}
		for _, aura := range auras {
			if aura == nil {
				continue
			}
			if aura.CasterGUID == s.playerGUID && (aura.AuraType == spellAuraCharm || aura.AuraType == spellAuraAoeCharm) {
				return true
			}
		}
	}
	return false
}

// handleLfgTeleport processes CMSG_LFG_TELEPORT (0x370).
// Reference: WorldSession::HandleLfgTeleportOpcode (LFGHandler.cpp:144).
func (s *session) handleLfgTeleport(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 || s.player == nil {
		return true
	}
	out := payload[0] != 0
	s.debug("lfg teleport", "account", s.accountName, "out", out)

	// LFGMgr::TeleportPlayer (LFGMgr.cpp:1507-1527): the dungeon resolves
	// only from the player's LFG group. A solo-queued player has no group
	// and gets LFG_TELEPORTERROR_INVALID_LOCATION, so there is no queue-
	// entry fallback.
	var lfgDungeonID uint32
	if s.groupID != 0 && s.server != nil {
		grp := s.server.getGroup(s.groupID)
		if grp != nil && grp.IsLFG {
			lfgDungeonID = grp.LFGDungeonID
		}
	}

	if lfgDungeonID == 0 {
		// SMSG_LFG_TELEPORT_DENIED (0x200): error (4) -> LFG_TELEPORTERROR_INVALID_LOCATION (6)
		packet := protocol.NewBuffer(4)
		packet.WriteU32(LFGTeleportErrorInvalidLocation)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	dungeonMap, _, _, _, _ := s.getLFGDungeonEntrance(lfgDungeonID)

	if out {
		// LFGMgr::TeleportPlayer (LFGMgr.cpp:1529-1537): the out leg only
		// fires when the player is standing on the dungeon map, returning
		// via Player::TeleportToBGEntryPoint.
		if dungeonMap != 0 && s.player.Map == dungeonMap {
			s.teleportToBGEntryPoint()
		}
		return true
	}

	// Reference: LFGMgr::TeleportPlayer (LFGMgr.cpp:1529-1582)
	teleportErr := LFGTeleportErrorOK
	if s.player.Health == 0 || (s.player.PlayerFlags&playerFlagGhost != 0) {
		teleportErr = LFGTeleportErrorPlayerDead
	} else if s.isFalling {
		teleportErr = LFGTeleportErrorFalling
	} else if s.isFatigueActive() {
		teleportErr = LFGTeleportErrorFatigue
	} else if s.player.VehicleGUID != 0 {
		teleportErr = LFGTeleportErrorInVehicle
	} else if s.hasCharmedCreature() { // GetCharmedGUID (LFGMgr.cpp:1538)
		teleportErr = LFGTeleportErrorCharming
	} else if s.hasAura(9454) { // Freeze debuff (LFGMgr.cpp:1540)
		teleportErr = LFGTeleportErrorInvalidLocation
	}

	if teleportErr != LFGTeleportErrorOK {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(teleportErr)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	if dungeonMap != 0 && s.player.Map == dungeonMap {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(LFGTeleportErrorInvalidLocation)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	s.teleportToLFGDungeon(lfgDungeonID, true)
	return true
}

// handleSearchLfgJoin processes CMSG_SEARCH_LFG_JOIN (0x35E).
// Reference: WorldSession::HandleLfrJoinOpcode (LFGHandler.cpp:259).
func (s *session) handleSearchLfgJoin(ctx context.Context, payload []byte) bool {
	if len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	dungeonID, _ := r.ReadU32()
	s.debug("search lfg join", "account", s.accountName, "dungeon", dungeonID)
	return true
}

// handleSearchLfgLeave processes CMSG_SEARCH_LFG_LEAVE (0x35F).
// Reference: WorldSession::HandleLfrLeaveOpcode (LFGHandler.cpp:268).
func (s *session) handleSearchLfgLeave(ctx context.Context, payload []byte) bool {
	if len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	dungeonID, _ := r.ReadU32()
	s.debug("search lfg leave", "account", s.accountName, "dungeon", dungeonID)
	return true
}

// handleSetLfgComment processes CMSG_SET_LFG_COMMENT (0x366).
// Reference: WorldSession::HandleLfgSetCommentOpcode (LFGHandler.cpp:122).
func (s *session) handleSetLfgComment(ctx context.Context, payload []byte) bool {
	if s.playerGUID == 0 {
		return true
	}
	r := protocol.NewReader(payload)
	comment, err := r.ReadCString()
	if err != nil {
		return true
	}
	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		lfg := s.server.Features.LFG
		lfg.mu.Lock()
		// HandleLfgSetCommentOpcode -> LFGMgr::SetComment (LFGMgr.cpp:1984)
		// writes the player-data comment only; it never creates queue state,
		// so a comment from a non-queued player must not materialize a queue
		// entry (that phantom entry would make a later Leave emit a bogus
		// REMOVED_FROM_QUEUE and inflate the queue dump).
		if entry, ok := lfg.queue[s.playerGUID]; ok {
			entry.Comment = comment
			lfg.queue[s.playerGUID] = entry
		}
		lfg.mu.Unlock()
	}
	s.debug("lfg set comment", "account", s.accountName, "comment", comment)
	return true
}

// completeLFGDungeon rewards players in an LFG dungeon group and triggers criteria.
func (s *Server) completeLFGDungeon(groupID uint32, dungeonID uint32) {
	s.groupsMu.Lock()
	grp := s.groups[uint64(groupID)]
	if grp == nil || !grp.IsLFG {
		s.groupsMu.Unlock()
		return
	}
	if grp.LFGState == LFGStateFinishedDungeon {
		s.groupsMu.Unlock()
		return
	}
	grp.LFGState = LFGStateFinishedDungeon
	grp.LFGDungeonID = dungeonID
	dbGroupID := grp.DBID
	members := append([]groupMember(nil), grp.Members...)
	s.groupsMu.Unlock()
	if dbGroupID == 0 {
		dbGroupID = groupID
	}
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.DB.ExecContext(context.Background(), "REPLACE INTO lfg_data (guid, dungeon, state) VALUES (?, ?, ?)", dbGroupID, dungeonID, LFGStateFinishedDungeon)
	}
	for _, m := range members {
		if sess := s.findSessionByGUID(m.GUID); sess != nil {
			// LFGMgr::FinishDungeon (LFGMgr.cpp:1664-1673): the only LFG
			// achievement criteria the C++ server fires is
			// USE_LFD_TO_GROUP_WITH_PLAYERS, for heroic random dungeons,
			// miscValue = random players grouped with (the C++ fallback 4
			// when the premade count is unknown, which it is here). The C++
			// never fires LFG_COMPLETION or LFG_DUNGEON_REWARD.
			if s.Data != nil {
				if dungeon, found, err := s.Data.LFGDungeon(dungeonID & 0x00FFFFFF); err == nil && found &&
					(dungeon.TypeID == LFGDungeonTypeRandom || dungeon.Flags&lfgFlagSeasonal != 0) &&
					dungeon.Difficulty == 1 {
					sess.updateAchievementCriteria(criteriaTypeUseLFDToGroup, 4, 1)
				}
			}
			_ = sess.sendLFGDungeonReward(dungeonID)
		}
	}
}

// sendLFGDungeonReward sends SMSG_LFG_PLAYER_REWARD (0x1FF) to the client.
func (s *session) sendLFGDungeonReward(dungeonID uint32) error {
	packet := protocol.NewBuffer(32)
	packet.WriteU32(dungeonID)
	packet.WriteU32(dungeonID)
	packet.WriteU32(0) // copper
	packet.WriteU32(0) // xp
	packet.WriteU8(0)  // reward count
	return s.write(uint16(protocol.OpcodeSMSG_LFG_PLAYER_REWARD), packet.Bytes(), true)
}

// GetOptions mirrors LFGMgr::GetOptions (LFGMgr.h:340); the C++ default is 1
// ("DungeonFinder.OptionsMask", World.cpp:1436).
func (m *LFGManager) GetOptions() uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.options
}

// SetOptions mirrors LFGMgr::SetOptions (LFGMgr.h:342).
func (m *LFGManager) SetOptions(options uint32) {
	m.mu.Lock()
	m.options = options
	m.mu.Unlock()
}

// Clean mirrors LFGMgr::Clean (LFGMgr.cpp): the C++ clears the queued-player
// and group stores; the Go tree's whole LFG runtime state is the queue,
// role checks and proposals maps, so all three are dropped.
func (m *LFGManager) Clean() {
	m.mu.Lock()
	m.queue = make(map[uint64]LFGQueueEntry)
	m.roleChecks = make(map[uint64]*LfgRoleCheck)
	m.proposals = make(map[uint32]*LFGProposal)
	m.nextProposalID = 0
	m.mu.Unlock()
}

// DumpQueueInfo mirrors LFGMgr::DumpQueueInfo (LFGMgr.cpp) for the Go tree's
// single queue model: the C++ keeps one LfgQueue per dungeon group and dumps
// its player/group counts plus the compatible map; the Go manager keeps one
// flat player queue and has no compatible-map store, so that section reports
// size 0. (C++: "Number of Queues: %u\n" then LfgQueue::DumpQueueInfo's
// "Queued Players: %u (in group: %u) Groups: %u\n" and DumpCompatibleInfo's
// "Compatible Map size: %u\n" with the full detail listing.)
func (m *LFGManager) DumpQueueInfo(full bool) string {
	m.mu.RLock()
	players := len(m.queue)
	m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Number of Queues: 1\n")
	fmt.Fprintf(&sb, "Queued Players: %d (in group: 0) Groups: 0\n", players)
	sb.WriteString("Compatible Map size: 0\n")
	_ = full
	return sb.String()
}

// lfgStateString mirrors lfg::GetStateString (LFG.cpp:75): the English state
// words are inlined from TDB enUS recall (no in-tree trinity_string seed).
// Note the faithful quirk: LFG_STATE_BOOT has no case in the C++ switch and
// falls through to LANG_LFG_ERROR ("Error").
func lfgStateString(state uint8) string {
	switch state {
	case LFGStateNone:
		return "None"
	case LFGStateRoleCheck:
		return "Role check"
	case LFGStateQueued:
		return "Queued"
	case LFGStateProposal:
		return "Proposal"
	case LFGStateDungeon:
		return "Dungeon"
	case LFGStateFinishedDungeon:
		return "Finished dungeon"
	case LFGStateRaidBrowser:
		return "Raid browser"
	default:
		return "Error"
	}
}

// concatenateLFGDungeons mirrors lfg::ConcatenateDungeons (LFG.cpp:26): the
// selected dungeon ids joined with ", " (the Go queue already sorts them).
func concatenateLFGDungeons(dungeons []uint32) string {
	parts := make([]string, 0, len(dungeons))
	for _, d := range dungeons {
		parts = append(parts, strconv.FormatUint(uint64(d), 10))
	}
	return strings.Join(parts, ", ")
}
