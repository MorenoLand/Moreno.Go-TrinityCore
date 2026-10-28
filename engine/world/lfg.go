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
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
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
	LFGTeleportErrorFatigue         uint32 = 0 // LFGMgr.h:91 = 0 ("Original - 4"): the fatigue arm assigns 0, so the != OK check never fires — no SMSG_LFG_TELEPORT_DENIED, teleport proceeds.
	LFGTeleportErrorInvalidLocation uint32 = 6
	LFGTeleportErrorCharming        uint32 = 8

	LFGDungeonTypeNone    uint32 = 0
	LFGDungeonTypeDungeon uint32 = 1
	LFGDungeonTypeRaid    uint32 = 2
	LFGDungeonTypeHeroic  uint32 = 5
	LFGDungeonTypeRandom  uint32 = 6

	LFGJoinOK                  uint32 = 0
	LFGJoinFailed              uint32 = 1
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
	lfgSpellDungeonCooldown = 71328            // LFG_SPELL_DUNGEON_COOLDOWN
	lfgSpellDungeonDeserter = 71041            // LFG_SPELL_DUNGEON_DESERTER
	lfgSpellLuckOfTheDraw   = 72221            // LFG_SPELL_LUCK_OF_THE_DRAW
	lfgGroupKickVotesNeeded = 3                // LFG_GROUP_KICK_VOTES_NEEDED (LFGMgr.h:54)
	lfgTimeBoot             = 120              // LFG_TIME_BOOT (LFGMgr.h:48, seconds)
	lfgTimeRoleCheck        = 45               // LFG_TIME_ROLECHECK (LFGMgr.h:47, seconds)
	lfgGroupMaxKicks        = 3                // LFG_GROUP_MAX_KICKS (LFGGroupData.h:28)
	lfgQueueUpdateInterval  = 15 * time.Second // LFG_QUEUEUPDATE_INTERVAL (LFGMgr.h:50)
	lfgFlagSeasonal         = 0x4              // LFG_FLAG_SEASONAL
)

type LFGQueueEntry struct {
	GUID     uint64
	Roles    uint8
	Dungeons []uint32
	Comment  string
	QueuedAt time.Time
	State    uint8
}

type LFGGroupQueueEntry struct {
	GroupID  uint64
	Dungeons []uint32
	Roles    map[uint64]uint8
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
	CancelAt  time.Time
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

// lfgWaitTime mirrors LfgWaitTime (LFGQueue.h:70-75): the running-average
// wait for one dungeon, starting at -1 with zero samples.
type lfgWaitTime struct {
	time   int32
	number uint32
}

type LFGManager struct {
	mu              sync.RWMutex
	soloEnable      bool
	solo            bool
	options         uint32
	queue           map[uint64]LFGQueueEntry
	comments        map[uint64]string
	roleChecks      map[uint64]*LfgRoleCheck
	proposals       map[uint32]*LFGProposal
	groupQueue      map[uint64]*LFGGroupQueueEntry
	waitAvg         map[uint32]lfgWaitTime
	waitTank        map[uint32]lfgWaitTime
	waitHealer      map[uint32]lfgWaitTime
	waitDps         map[uint32]lfgWaitTime
	nextProposalID  uint32
	validateDungeon func(uint32) bool
}

func NewLFGManager(soloEnable bool) *LFGManager {
	return &LFGManager{
		soloEnable: soloEnable,
		options:    1, // "DungeonFinder.OptionsMask" default (World.cpp:1436)
		queue:      make(map[uint64]LFGQueueEntry),
		comments:   make(map[uint64]string),
		roleChecks: make(map[uint64]*LfgRoleCheck),
		proposals:  make(map[uint32]*LFGProposal),
		groupQueue: make(map[uint64]*LFGGroupQueueEntry),
		waitAvg:    make(map[uint32]lfgWaitTime),
		waitTank:   make(map[uint32]lfgWaitTime),
		waitHealer: make(map[uint32]lfgWaitTime),
		waitDps:    make(map[uint32]lfgWaitTime),
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

// FormSoloProposal mirrors the net SoloLFG effect of the
// LFGQueue::CheckCompatibility arms (LFGQueue.cpp:425, 523): with SoloLFG on,
// the "underfilled group always compatible" early-out is skipped and the
// "Enough players?" numPlayers != MAXGROUPSIZE gate is skipped, so a solo
// queued player is matchable alone and a proposal forms with fewer than 5
// players. the solo path forms the proposal directly from the queued entry; grouped queues match through FindGroups below, not here.
func (m *LFGManager) FormSoloProposal(guid uint64) *LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.solo {
		return nil
	}
	entry, ok := m.queue[guid]
	if !ok || entry.State != LFGStateQueued || len(entry.Dungeons) == 0 {
		return nil
	}
	m.nextProposalID++
	prop := &LFGProposal{
		ID:         m.nextProposalID,
		DungeonID:  entry.Dungeons[0],
		State:      LFGProposalInitiating,
		Group:      0,
		Leader:     guid,
		CancelTime: time.Now().Add(45 * time.Second),
		Players:    map[uint64]*LFGProposalPlayer{guid: {Role: entry.Roles, Group: 0, Accept: LFGAnswerPending}},
	}
	m.proposals[prop.ID] = prop
	entry.State = LFGStateProposal
	m.queue[guid] = entry
	return prop
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

// Leave drops a queued entry. Every other state keeps its entry: C++
// LeaveLfg only removes from the queue on the QUEUED arm (LFGMgr.cpp:720);
// the DUNGEON/FINISHED arm only flips state, the PROPOSAL arm is handled by
// the proposal arms, and the ROLECHECK arm aborts the check. Callers that
// need a state flip use SetState.
func (m *LFGManager) Leave(guid uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.leaveLocked(guid)
}

func (m *LFGManager) leaveLocked(guid uint64) bool {
	entry, ok := m.queue[guid]
	if !ok || entry.State != LFGStateQueued {
		return false
	}
	delete(m.queue, guid)
	return true
}

// SetState mirrors LfgPlayerData::SetState (LFGPlayerData.cpp:38-50):
// DUNGEON flips state only, preserving roles and selected dungeons, while
// NONE and FINISHED clear roles, selected dungeons and the comment. Go
// keeps no separate PlayersStore, so a NONE entry is deleted outright —
// every consumer treats a missing entry exactly like a NONE entry with
// cleared fields. Other states create the entry on demand, like C++'s
// PlayersStore operator[].
func (m *LFGManager) SetState(guid uint64, state uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setStateLocked(guid, state)
}

func (m *LFGManager) setStateLocked(guid uint64, state uint8) {
	if state == LFGStateNone {
		delete(m.queue, guid)
		delete(m.comments, guid)
		return
	}
	entry, ok := m.queue[guid]
	if !ok {
		entry = LFGQueueEntry{GUID: guid}
	}
	entry.State = state
	if state == LFGStateFinishedDungeon {
		entry.Roles = 0
		entry.Dungeons = nil
		entry.Comment = ""
		delete(m.comments, guid)
	}
	m.queue[guid] = entry
}

// SetupGroupMember mirrors LFGMgr::SetupGroupMember (LFGMgr.cpp:2240-2248):
// the member takes the group's single dungeon as their whole selection,
// then takes the group's state; NONE and FINISHED clear the selection
// again through SetState, so the call order (dungeons first, state
// second) is load-bearing. The C++ SetGroup/AddPlayerToGroup legs have
// no Go counterpart — group membership lives in groupState.Members, not
// in the LFG manager.
func (m *LFGManager) SetupGroupMember(guid uint64, dungeon uint32, state uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.queue[guid]
	if !ok {
		entry = LFGQueueEntry{GUID: guid}
	}
	entry.Dungeons = []uint32{dungeon}
	m.queue[guid] = entry
	m.setStateLocked(guid, state)
}

// LeaveQueuedGroup mirrors the gstate == QUEUED arm of
// LFGGroupScript::OnAddMember (LFGScripts.cpp:143-181) as reached through
// the group QUEUED arm of LFGMgr::LeaveLfg (LFGMgr.cpp:726-748): a queued
// group that gains a member by invite leaves the queue. It returns the
// GUIDs of the group's pre-existing members (the joiner, excluded by the
// caller, is not yet group LFG membership in C++ — OnAddMember runs the
// leave arms before SetGroup/AddPlayerToGroup). QUEUED member entries flip
// to restoreState when restore is set (a mid-dungeon group that re-queued:
// C++ restores OldState DUNGEON/FINISHED) and to NONE otherwise; members
// in any other state keep it, since QueueGroup is what put them in QUEUED.
func (m *LFGManager) LeaveQueuedGroup(groupID uint64, exclude uint64, restore bool, restoreState uint8) []uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.groupQueue[groupID]
	if !ok || entry.State != LFGStateQueued {
		return nil
	}
	delete(m.groupQueue, groupID)
	affected := make([]uint64, 0, len(entry.Roles))
	for guid := range entry.Roles {
		if guid == exclude {
			continue
		}
		affected = append(affected, guid)
		if cur, ok := m.queue[guid]; ok && cur.State == LFGStateQueued {
			if restore {
				m.setStateLocked(guid, restoreState)
			} else {
				m.setStateLocked(guid, LFGStateNone)
			}
		}
	}
	sort.Slice(affected, func(i, j int) bool { return affected[i] < affected[j] })
	return affected
}

// QueueGroup mirrors the FINISHED arm of LFGMgr::UpdateRoleCheck (LFGMgr.cpp:868-872): the finished group itself enters the queue (AddQueueData keyed by the group guid), and every member is marked queued with the role the check assigned (the SetState/SetRoles arms, LFGMgr.cpp:858-864). The per-member entries let Leave and status reads work per player; FindGroups treats the group entry as the unit and never double-counts its members as solos.
func (m *LFGManager) QueueGroup(groupID, leaderGUID uint64, dungeons []uint32, roles map[uint64]uint8, comment string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.groupQueue == nil {
		m.groupQueue = make(map[uint64]*LFGGroupQueueEntry)
	}
	now := time.Now()
	copiedRoles := make(map[uint64]uint8, len(roles))
	for guid, role := range roles {
		copiedRoles[guid] = role
	}
	m.groupQueue[groupID] = &LFGGroupQueueEntry{
		GroupID:  groupID,
		Dungeons: append([]uint32(nil), dungeons...),
		Roles:    copiedRoles,
		QueuedAt: now,
		State:    LFGStateQueued,
	}
	for guid, role := range copiedRoles {
		entry := LFGQueueEntry{GUID: guid, Roles: role, Dungeons: append([]uint32(nil), dungeons...), QueuedAt: now, State: LFGStateQueued}
		if guid == leaderGUID {
			entry.Comment = comment
		}
		m.queue[guid] = entry
	}
}

// LeaveGroup drops a queued group entry; the caller drops the members' per-player entries.
func (m *LFGManager) LeaveGroup(groupID uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groupQueue[groupID]; !ok {
		return false
	}
	delete(m.groupQueue, groupID)
	return true
}

func (m *LFGManager) deleteGroupEntriesContainingLocked(guid uint64) {
	for groupID, entry := range m.groupQueue {
		if _, ok := entry.Roles[guid]; ok {
			delete(m.groupQueue, groupID)
		}
	}
}

type lfgMatchUnit struct {
	groupID  uint64
	guids    []uint64
	roles    map[uint64]uint8
	dungeons []uint32
}

func lfgIntersectDungeons(a, b []uint32) []uint32 {
	set := make(map[uint32]bool, len(b))
	for _, d := range b {
		set[d] = true
	}
	var out []uint32
	for _, d := range a {
		if set[d] {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// FindGroups mirrors LFGQueue::FindGroups + FindNewGroups + CheckCompatibility (LFGQueue.cpp:293-360, 373-560) over Go's queue: group entries and solo entries are the units, and a combination matches when it holds at most one LFG group, its dungeon sets intersect, and its merged roles resolve through lfgCheckGroupRoles. Non-solo queues need exactly 5 players (MAXGROUPSIZE); with SoloLFG on, the "enough players" gate is skipped, so a single compatible unit matches alone. Differences from C++: no ignore check (Go keeps no ignore store), the proposal dungeon is the smallest common dungeon instead of a random one, and the leader is the smallest leader-bit guid instead of a random one. Matched units are marked PROPOSAL, not removed, so proposal decline/expiry keep working off the existing queue arms.
func (m *LFGManager) FindGroups() []*LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	var groupIDs []uint64
	for groupID := range m.groupQueue {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	memberOfGroup := make(map[uint64]bool)
	var units []*lfgMatchUnit
	for _, groupID := range groupIDs {
		entry := m.groupQueue[groupID]
		if entry.State != LFGStateQueued {
			continue
		}
		var guids []uint64
		roles := make(map[uint64]uint8, len(entry.Roles))
		for guid, role := range entry.Roles {
			guids = append(guids, guid)
			roles[guid] = role
			memberOfGroup[guid] = true
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		units = append(units, &lfgMatchUnit{groupID: groupID, guids: guids, roles: roles, dungeons: append([]uint32(nil), entry.Dungeons...)})
	}
	var soloGUIDs []uint64
	for guid, entry := range m.queue {
		if entry.State == LFGStateQueued && !memberOfGroup[guid] {
			soloGUIDs = append(soloGUIDs, guid)
		}
	}
	sort.Slice(soloGUIDs, func(i, j int) bool { return soloGUIDs[i] < soloGUIDs[j] })
	for _, guid := range soloGUIDs {
		entry := m.queue[guid]
		units = append(units, &lfgMatchUnit{guids: []uint64{guid}, roles: map[uint64]uint8{guid: entry.Roles}, dungeons: append([]uint32(nil), entry.Dungeons...)})
	}
	used := make([]bool, len(units))
	var proposals []*LFGProposal
	for seed := range units {
		if used[seed] {
			continue
		}
		combo := m.findLFGCombo(units, used, seed)
		if combo == nil {
			continue
		}
		var dungeons []uint32
		merged := make(map[uint64]uint8)
		ownerGroup := make(map[uint64]uint64)
		proposalGroup := uint64(0)
		for i, idx := range combo {
			unit := units[idx]
			if i == 0 {
				dungeons = append([]uint32(nil), unit.dungeons...)
			} else {
				dungeons = lfgIntersectDungeons(dungeons, unit.dungeons)
			}
			if unit.groupID != 0 {
				proposalGroup = unit.groupID
			}
			for _, guid := range unit.guids {
				merged[guid] = unit.roles[guid]
				ownerGroup[guid] = unit.groupID
			}
		}
		if len(dungeons) == 0 {
			continue
		}
		assigned := make(map[uint64]uint8, len(merged))
		for guid, role := range merged {
			assigned[guid] = role
		}
		if !lfgCheckGroupRoles(assigned) {
			continue
		}
		var allGUIDs []uint64
		for guid := range merged {
			allGUIDs = append(allGUIDs, guid)
		}
		sort.Slice(allGUIDs, func(i, j int) bool { return allGUIDs[i] < allGUIDs[j] })
		leader := uint64(0)
		for _, guid := range allGUIDs {
			if assigned[guid]&LFGRoleLeader != 0 {
				leader = guid
				break
			}
		}
		if leader == 0 {
			leader = allGUIDs[0]
		}
		m.nextProposalID++
		prop := &LFGProposal{
			ID:         m.nextProposalID,
			DungeonID:  dungeons[0],
			State:      LFGProposalInitiating,
			Group:      proposalGroup,
			Leader:     leader,
			CancelTime: time.Now().Add(45 * time.Second),
			Players:    make(map[uint64]*LFGProposalPlayer, len(merged)),
		}
		for _, guid := range allGUIDs {
			prop.Players[guid] = &LFGProposalPlayer{Role: assigned[guid], Group: ownerGroup[guid], Accept: LFGAnswerPending}
		}
		m.proposals[prop.ID] = prop
		for _, idx := range combo {
			used[idx] = true
			unit := units[idx]
			if unit.groupID != 0 {
				if entry, ok := m.groupQueue[unit.groupID]; ok {
					entry.State = LFGStateProposal
				}
			} else if entry, ok := m.queue[unit.guids[0]]; ok {
				entry.State = LFGStateProposal
				m.queue[unit.guids[0]] = entry
			}
		}
		proposals = append(proposals, prop)
	}
	return proposals
}

// findLFGCombo searches increasing-index combinations starting at seed for a set satisfying the size, group-count and dungeon gates; role resolution is re-checked by the caller on the full merged set.
func (m *LFGManager) findLFGCombo(units []*lfgMatchUnit, used []bool, seed int) []int {
	var search func(chosen []int, total, groups int, dungeons []uint32) []int
	search = func(chosen []int, total, groups int, dungeons []uint32) []int {
		if total > 5 || groups > 1 {
			return nil
		}
		if total == 5 {
			merged := make(map[uint64]uint8)
			for _, idx := range chosen {
				for guid, role := range units[idx].roles {
					merged[guid] = role
				}
			}
			if lfgCheckGroupRoles(merged) {
				return append([]int(nil), chosen...)
			}
			return nil
		}
		if m.solo && total > 0 {
			merged := make(map[uint64]uint8)
			for _, idx := range chosen {
				for guid, role := range units[idx].roles {
					merged[guid] = role
				}
			}
			if lfgCheckGroupRoles(merged) {
				return append([]int(nil), chosen...)
			}
		}
		last := seed - 1
		if len(chosen) > 0 {
			last = chosen[len(chosen)-1]
		}
		for i := last + 1; i < len(units); i++ {
			if used[i] {
				continue
			}
			unit := units[i]
			if total+len(unit.guids) > 5 {
				continue
			}
			nextGroups := groups
			if unit.groupID != 0 {
				nextGroups++
			}
			if nextGroups > 1 {
				continue
			}
			var nextDungeons []uint32
			if len(chosen) == 0 {
				nextDungeons = append([]uint32(nil), unit.dungeons...)
			} else {
				nextDungeons = lfgIntersectDungeons(dungeons, unit.dungeons)
				if len(nextDungeons) == 0 {
					continue
				}
			}
			if combo := search(append(chosen, i), total+len(unit.guids), nextGroups, nextDungeons); combo != nil {
				return combo
			}
		}
		return nil
	}
	return search([]int{}, 0, 0, nil)
}

// SetComment mirrors LFGMgr::SetComment (LFGMgr.cpp:1984-1988): the comment
// lives in the per-player store, created on demand, so it persists for
// non-queued players and survives LeaveLfg (C++ never clears PlayersStore on
// leave). The live queue entry is updated too so later updates built from it
// carry the new comment, like C++'s GetComment reads.
func (m *LFGManager) SetComment(guid uint64, comment string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.comments == nil {
		m.comments = make(map[uint64]string)
	}
	m.comments[guid] = comment
	if entry, ok := m.queue[guid]; ok {
		entry.Comment = comment
		m.queue[guid] = entry
	}
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

// StartRoleCheck mirrors the grouped branch of LFGMgr::JoinLfg
// (LFGMgr.cpp:594-602, 620-627): the check is keyed by the group, every
// live member starts unanswered (roles 0), the joiner is the check leader
// (not necessarily the group leader), and the check expires 45s after
// creation (LFGMgr.cpp:597). The joiner's roles are sanitized like C++
// sanitizes the join input (LFGMgr.cpp:411-414); the remaining members
// answer through UpdateRoleCheck, which is what moves the check out of
// INITIALIZING.
func (m *LFGManager) StartRoleCheck(groupID uint64, leaderGUID uint64, dungeons []uint32, leaderRoles uint8, members []uint64) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	rc := &LfgRoleCheck{
		GroupID:   groupID,
		Leader:    leaderGUID,
		Dungeons:  append([]uint32(nil), dungeons...),
		Roles:     make(map[uint64]uint8),
		State:     LFGRoleCheckInitializing,
		CreatedAt: now,
		CancelAt:  now.Add(time.Duration(lfgTimeRoleCheck) * time.Second),
	}
	for _, guid := range members {
		rc.Roles[guid] = LFGRoleNone
	}
	rc.Roles[leaderGUID] = leaderRoles & LFGRoleAny
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
	// LFGMgr.cpp:868-878: a finished check and a failed check are both
	// erased from the store once resolved; the returned pointer stays
	// valid for the caller's terminal notifies.
	if rc.State == LFGRoleCheckFinished || rc.State == LFGRoleCheckWrongRoles || rc.State == LFGRoleCheckNoRole {
		delete(m.roleChecks, groupID)
	}
	return rc
}

// ExpireRoleChecks mirrors the role-check arm of LFGMgr::Update
// (LFGMgr.cpp:301-321): a check at or past cancelTime fails as
// MISSING_ROLE and is erased. The per-member RestoreState calls have no
// Go counterpart to apply: a grouped join queues nothing until the check
// finishes, so erasing the check is the whole state restore. The caller
// sends the per-member role-check updates and the leader's FAILED join
// result; unlike the UpdateRoleCheck failure arm, expiry sends no
// ROLECHECK_FAILED party update (C++ sends none on this arm).
func (m *LFGManager) ExpireRoleChecks(now time.Time) []*LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []*LfgRoleCheck
	for groupID, rc := range m.roleChecks {
		if rc.CancelAt.After(now) {
			continue
		}
		rc.State = LFGRoleCheckMissingRole
		expired = append(expired, rc)
		delete(m.roleChecks, groupID)
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].GroupID < expired[j].GroupID })
	return expired
}

// AbortRoleCheck mirrors the ROLECHECK arm of LFGMgr::LeaveLfg
// (LFGMgr.cpp:763-766): leaving during a live check aborts it via
// UpdateRoleCheck with no player, which sets ABORTED and erases the
// check. The caller sends the terminal notifies; no RoleChosen packet
// goes out on this arm (C++ sendRoleChosen requires a player guid).
func (m *LFGManager) AbortRoleCheck(groupID uint64) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	rc, ok := m.roleChecks[groupID]
	if !ok {
		return nil
	}
	rc.State = LFGRoleCheckAborted
	delete(m.roleChecks, groupID)
	return rc
}

// ExpireProposals mirrors the proposal arm of LFGMgr::Update
// (LFGMgr.cpp:324-328) through RemoveProposal with PROPOSAL_FAILED
// (LFGMgr.cpp:1306-1345): pending answers become DENY, the proposal
// fails and is erased, and the queue loses every denied player plus
// every agreeing player in a denied player's group (the C++ group
// queue-entry removal, per player in Go's per-player queue). Agreeing
// players outside a denied group keep their queue entry, re-marked
// QUEUED (C++ re-adds them to the queue on this arm).
func (m *LFGManager) ExpireProposals(now time.Time) []*LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []*LFGProposal
	for id, prop := range m.proposals {
		if !prop.CancelTime.Before(now) {
			continue
		}
		prop.State = LFGProposalFailed
		deniedGroups := make(map[uint64]bool)
		for _, player := range prop.Players {
			if player.Accept == LFGAnswerPending {
				player.Accept = LFGAnswerDeny
			}
			if player.Accept == LFGAnswerDeny && player.Group != 0 {
				deniedGroups[player.Group] = true
			}
		}
		for guid, player := range prop.Players {
			if player.Accept == LFGAnswerDeny || deniedGroups[player.Group] {
				delete(m.queue, guid)
				continue
			}
			if entry, ok := m.queue[guid]; ok {
				entry.State = LFGStateQueued
				m.queue[guid] = entry
			}
		}
		for groupID, entry := range m.groupQueue {
			if deniedGroups[groupID] {
				delete(m.groupQueue, groupID)
				continue
			}
			for guid := range prop.Players {
				if _, ok := entry.Roles[guid]; ok {
					entry.State = LFGStateQueued
					break
				}
			}
		}
		expired = append(expired, prop)
		delete(m.proposals, id)
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].ID < expired[j].ID })
	return expired
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

// declineProposalLocked runs the LFGMgr::RemoveProposal decline arm
// (LFGMgr.cpp:1306-1380) for one player: the decliner is marked DENY, the
// denied group leaves the queue, agreeing players outside a denied group
// are re-queued, and the proposal is dropped. The caller holds m.mu, and
// the proposal must still be in m.proposals. Pending players are NOT
// marked deny on this arm (only the FAILED arm does that).
func (m *LFGManager) declineProposalLocked(prop *LFGProposal, guid uint64) {
	player := prop.Players[guid]
	player.Accept = LFGAnswerDeny
	prop.State = LFGProposalFailed
	// LFGMgr::RemoveProposal (LFGMgr.cpp:1318-1332): the decliner is
	// removed from the queue.
	delete(m.queue, guid)
	// LFGMgr::RemoveProposal (LFGMgr.cpp:1306-1345): the denied player's
	// whole group leaves the queue, not just the decliner.
	if player.Group != 0 {
		for pguid, p := range prop.Players {
			if p.Group == player.Group {
				delete(m.queue, pguid)
			}
		}
		delete(m.groupQueue, player.Group)
	}
	m.deleteGroupEntriesContainingLocked(guid)
	// LFGMgr::RemoveProposal (LFGMgr.cpp:1366-1376): agreeing players
	// outside a denied group are re-queued, mirroring ExpireProposals.
	deniedGroups := make(map[uint64]bool)
	for _, p := range prop.Players {
		if p.Accept == LFGAnswerDeny && p.Group != 0 {
			deniedGroups[p.Group] = true
		}
	}
	for pguid, p := range prop.Players {
		if p.Accept == LFGAnswerDeny || deniedGroups[p.Group] {
			continue
		}
		if entry, ok := m.queue[pguid]; ok {
			entry.State = LFGStateQueued
			m.queue[pguid] = entry
		}
	}
	for _, entry := range m.groupQueue {
		for pguid := range prop.Players {
			if _, ok := entry.Roles[pguid]; ok {
				entry.State = LFGStateQueued
				break
			}
		}
	}
	delete(m.proposals, prop.ID)
}

// DeclineProposalForPlayer runs the LeaveLfg PROPOSAL arm
// (LFGMgr.cpp:754-779) for a player who leaves or is kicked mid-proposal:
// the proposal containing the player is found, the player is marked DENY,
// and the proposal is removed with the DECLINED notify data. It returns
// the removed proposal for the caller to notify, or nil when the player
// is in no proposal.
func (m *LFGManager) DeclineProposalForPlayer(guid uint64) *LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, prop := range m.proposals {
		if _, ok := prop.Players[guid]; ok {
			m.declineProposalLocked(prop, guid)
			return prop
		}
	}
	return nil
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
		m.declineProposalLocked(prop, guid)
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
	// must agree. m.solo is read directly: the manager lock is already held
	// here, and Solo() would take a read lock — a self-deadlock on the
	// first accept of any proposal.
	if allAgree || m.solo {
		prop.State = LFGProposalSuccess
		// LFGMgr::UpdateProposal (LFGMgr.cpp:1240-1283): success records
		// each member's queue wait into the per-dungeon stores before the
		// queue entries are dropped. The dungeon is the member's first
		// selected dungeon (the C++ set's begin), the join time the group
		// entry's for grouped members, and the role the stored queue role
		// with the leader bit stripped; a single role selects its store,
		// anything else the average. The C++ wait divides a time_t second
		// difference by IN_MILLISECONDS, so waits under ~16.7 minutes
		// record as 0 — replicated verbatim.
		nowUnix := time.Now().Unix()
		for pguid, player := range prop.Players {
			dungeonID := prop.DungeonID
			joinUnix := nowUnix
			role := player.Role
			if entry, ok := m.queue[pguid]; ok {
				if d, ok := lfgMinDungeon(entry.Dungeons); ok {
					dungeonID = d
				}
				joinUnix = entry.QueuedAt.Unix()
				role = entry.Roles
			}
			if player.Group != 0 {
				if gentry, ok := m.groupQueue[player.Group]; ok {
					joinUnix = gentry.QueuedAt.Unix()
					if _, ok := m.queue[pguid]; !ok {
						if d, ok := lfgMinDungeon(gentry.Dungeons); ok {
							dungeonID = d
						}
					}
				}
			}
			waitTime := int32((nowUnix - joinUnix) / 1000)
			switch role &^ LFGRoleLeader {
			case LFGRoleDamage:
				m.updateLFGWaitTime(m.waitDps, waitTime, dungeonID)
			case LFGRoleHealer:
				m.updateLFGWaitTime(m.waitHealer, waitTime, dungeonID)
			case LFGRoleTank:
				m.updateLFGWaitTime(m.waitTank, waitTime, dungeonID)
			default:
				m.updateLFGWaitTime(m.waitAvg, waitTime, dungeonID)
			}
		}
		// LFGMgr::UpdateProposal (LFGMgr.cpp:1291-1293): players leave the
		// queue when the proposal succeeds, but their PlayersStore entry
		// survives as DUNGEON with roles and selected dungeons intact
		// (LFGPlayerData::SetState clears nothing on DUNGEON) — only the
		// queue units go, so GetLfgStatus still reports the dungeon and
		// the random-dungeon cooldown arm still sees the selection.
		for pguid := range prop.Players {
			m.setStateLocked(pguid, LFGStateDungeon)
			m.deleteGroupEntriesContainingLocked(pguid)
		}
		delete(m.proposals, proposalID)
	}
	return prop, true
}

// updateLFGWaitTime mirrors LFGQueue::UpdateWaitTimeAvg/Tank/Healer/Dps
// (LFGQueue.cpp:205-232): the running average over all recorded waits.
func (m *LFGManager) updateLFGWaitTime(store map[uint32]lfgWaitTime, waitTime int32, dungeonID uint32) {
	wt := store[dungeonID]
	old := wt.number
	wt.number++
	wt.time = (wt.time*int32(old) + waitTime) / int32(wt.number)
	store[dungeonID] = wt
}

func lfgWaitTimeOf(store map[uint32]lfgWaitTime, dungeonID uint32) int32 {
	if wt, ok := store[dungeonID]; ok {
		return wt.time
	}
	return -1
}

// lfgQueueWaitTime mirrors the role switch in LFGQueue::UpdateQueueTimers
// (LFGQueue.cpp:609-625): a single role selects its store, anything else
// (none or mixed) the average.
func lfgQueueWaitTime(role uint8, avg, tank, healer, dps int32) int32 {
	switch role {
	case LFGRoleNone:
		return -1
	case LFGRoleTank:
		return tank
	case LFGRoleHealer:
		return healer
	case LFGRoleDamage:
		return dps
	default:
		return avg
	}
}

func lfgMinDungeon(dungeons []uint32) (uint32, bool) {
	if len(dungeons) == 0 {
		return 0, false
	}
	min := dungeons[0]
	for _, d := range dungeons[1:] {
		if d < min {
			min = d
		}
	}
	return min, true
}

// lfgQueueStatus is one LfgQueueStatusData (LFGMgr.h:178-192) for a single
// queue unit: a group entry plus its members, or one solo player.
type lfgQueueStatus struct {
	dungeonID  uint32
	waitTime   int32
	waitAvg    int32
	waitTank   int32
	waitHealer int32
	waitDps    int32
	queuedTime uint32
	recipients []uint64
}

// QueueStatuses mirrors LFGQueue::UpdateQueueTimers (LFGQueue.cpp:589-640):
// one status per queue unit — a group entry sent to every member, a solo
// entry to its player — with the first selected dungeon, the queued
// seconds, and the role-derived wait. Queue data survives a pending
// proposal until success, so proposal members still get a status; post-
// success DUNGEON/FINISHED entries are PlayersStore state, not queue units,
// and get none. The bestCompatible refresh
// (LFGQueue.cpp:631-632) has no twin: Go's matcher recomputes combos
// every tick and keeps no cached compatible string to refresh.
func (m *LFGManager) QueueStatuses(now time.Time) []lfgQueueStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	grouped := make(map[uint64]bool)
	var groupIDs []uint64
	for groupID, entry := range m.groupQueue {
		groupIDs = append(groupIDs, groupID)
		for guid := range entry.Roles {
			grouped[guid] = true
		}
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	var out []lfgQueueStatus
	for _, groupID := range groupIDs {
		entry := m.groupQueue[groupID]
		dungeonID, ok := lfgMinDungeon(entry.Dungeons)
		if !ok {
			continue
		}
		role := uint8(0)
		var guids []uint64
		for guid, r := range entry.Roles {
			role |= r
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		role &^= LFGRoleLeader
		queued := now.Unix() - entry.QueuedAt.Unix()
		if queued < 0 {
			queued = 0
		}
		out = append(out, lfgQueueStatus{
			dungeonID:  dungeonID,
			waitTime:   lfgQueueWaitTime(role, lfgWaitTimeOf(m.waitAvg, dungeonID), lfgWaitTimeOf(m.waitTank, dungeonID), lfgWaitTimeOf(m.waitHealer, dungeonID), lfgWaitTimeOf(m.waitDps, dungeonID)),
			waitAvg:    lfgWaitTimeOf(m.waitAvg, dungeonID),
			waitTank:   lfgWaitTimeOf(m.waitTank, dungeonID),
			waitHealer: lfgWaitTimeOf(m.waitHealer, dungeonID),
			waitDps:    lfgWaitTimeOf(m.waitDps, dungeonID),
			queuedTime: uint32(queued),
			recipients: guids,
		})
	}
	var soloGUIDs []uint64
	for guid, entry := range m.queue {
		if grouped[guid] {
			continue
		}
		if entry.State != LFGStateQueued && entry.State != LFGStateProposal {
			continue
		}
		soloGUIDs = append(soloGUIDs, guid)
	}
	sort.Slice(soloGUIDs, func(i, j int) bool { return soloGUIDs[i] < soloGUIDs[j] })
	for _, guid := range soloGUIDs {
		entry := m.queue[guid]
		dungeonID, ok := lfgMinDungeon(entry.Dungeons)
		if !ok {
			continue
		}
		queued := now.Unix() - entry.QueuedAt.Unix()
		if queued < 0 {
			queued = 0
		}
		role := entry.Roles &^ LFGRoleLeader
		out = append(out, lfgQueueStatus{
			dungeonID:  dungeonID,
			waitTime:   lfgQueueWaitTime(role, lfgWaitTimeOf(m.waitAvg, dungeonID), lfgWaitTimeOf(m.waitTank, dungeonID), lfgWaitTimeOf(m.waitHealer, dungeonID), lfgWaitTimeOf(m.waitDps, dungeonID)),
			waitAvg:    lfgWaitTimeOf(m.waitAvg, dungeonID),
			waitTank:   lfgWaitTimeOf(m.waitTank, dungeonID),
			waitHealer: lfgWaitTimeOf(m.waitHealer, dungeonID),
			waitDps:    lfgWaitTimeOf(m.waitDps, dungeonID),
			queuedTime: uint32(queued),
			recipients: []uint64{guid},
		})
	}
	return out
}

// sendLFGQueueStatus mirrors WorldSession::SendLfgQueueStatus
// (LFGHandler.cpp:455-478): the packet writes the average wait before
// the role wait, and the needs bytes are the LfgQueueData ctor constants,
// not queue-derived counts.
func (s *session) sendLFGQueueStatus(status lfgQueueStatus) error {
	packet := protocol.NewBuffer(4 + 4 + 4 + 4 + 4 + 4 + 1 + 1 + 1 + 4)
	packet.WriteU32(status.dungeonID)
	packet.WriteI32(status.waitAvg)
	packet.WriteI32(status.waitTime)
	packet.WriteI32(status.waitTank)
	packet.WriteI32(status.waitHealer)
	packet.WriteI32(status.waitDps)
	packet.WriteU8(lfgTanksNeeded)
	packet.WriteU8(lfgHealersNeeded)
	packet.WriteU8(lfgDPSNeeded)
	packet.WriteU32(status.queuedTime)
	return s.write(uint16(protocol.OpcodeSMSG_LFG_QUEUE_STATUS), packet.Bytes(), true)
}

// updateLFGQueueTimers mirrors the queue-update arm of LFGMgr::Update
// (LFGMgr.cpp:382-391): once the timer exceeds the interval it resets
// and every queued player gets the status packet, otherwise the timer
// just accumulates the tick diff. The finder+browser-disabled skip is
// the LFGMgr::Update head return (LFGMgr.cpp:294-296).
func (s *Server) updateLFGQueueTimers(now time.Time, diff time.Duration) {
	if s == nil || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	if s.lfgQueueTimer > lfgQueueUpdateInterval {
		s.lfgQueueTimer = 0
		for _, status := range s.Features.LFG.QueueStatuses(now) {
			for _, guid := range status.recipients {
				if ms := s.findSessionByGUID(guid); ms != nil {
					_ = ms.sendLFGQueueStatus(status)
				}
			}
		}
	} else {
		s.lfgQueueTimer += diff
	}
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
	// LFGMgr::JoinLfg (LFGMgr.cpp:422-429): a grouped LFG mid-dungeon
	// re-queue (isContinue) cannot change dungeons — the requested set is
	// replaced by the group's current dungeon (grp.LFGDungeonID is the
	// GetDungeon(gguid) analog, set at proposal accept like C++'s SetDungeon
	// at LFGMgr.cpp:1147). The replacement is unconditional: a zero group
	// dungeon flows into the type loop as LFG_JOIN_DUNGEON_INVALID.
	isContinue := false
	joinDungeons := filtered
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil && grp.IsLFG && grp.LFGState != LFGStateFinishedDungeon {
			isContinue = true
			joinDungeons = []uint32{grp.LFGDungeonID}
		}
	}
	// LFGMgr::JoinLfg takes dungeons as LfgDungeonSet (std::set): the set is
	// deduped and sorted from the start, so the type loop (LFGMgr.cpp:527-557)
	// sees each dungeon once in ascending order — two identical random slots
	// collapse to one instead of failing LFG_JOIN_DUNGEON_INVALID, and the
	// first failing dungeon is the lowest id.
	deduped := append([]uint32(nil), joinDungeons...)
	sort.Slice(deduped, func(i, j int) bool { return deduped[i] < deduped[j] })
	joinDungeons = uniqueUint32(deduped)
	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		// LFGMgr::JoinLfg (LFGMgr.cpp:431-437): a re-join removes the existing
		// queue entry BEFORE the requirement gates run — a re-join that then
		// fails a gate leaves the player out of the queue (old queue time
		// lost). The removal is silent: C++ emits no update packet for it.
		s.server.Features.LFG.Leave(s.playerGUID)
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
	// LFGMgr::JoinLfg (LFGMgr.cpp:583-589): the comment is stored before the
	// raid early-return — raid-browser joins queue nothing but the comment
	// persists in the player store.
	s.server.Features.LFG.SetComment(s.playerGUID, comment)
	if isRaid {
		return true
	}
	// LFGMgr::JoinLfg grouped branch (LFGMgr.cpp:594-683): a grouped join
	// opens a role check instead of queueing the joiner solo — the check
	// is keyed by the group, every live member starts unanswered, each
	// member gets the JOIN_QUEUE party update, and the joiner's own
	// answer is recorded at once (UpdateRoleCheck). No join result and
	// no player update go out on the initializing path: C++ sends those
	// only on role-check failure (leader) or queueing (members).
	if s.groupID != 0 && s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil {
			memberGUIDs := make([]uint64, 0, len(grp.Members))
			for _, m := range grp.Members {
				memberGUIDs = append(memberGUIDs, m.GUID)
			}
			rc := s.server.Features.LFG.StartRoleCheck(s.groupID, s.playerGUID, joinDungeons, roles8, memberGUIDs)
			for _, guid := range memberGUIDs {
				if ms := s.server.findSessionByGUID(guid); ms != nil {
					_ = ms.sendLFGUpdateParty(LFGUpdateJoinQueue, LFGQueueEntry{Dungeons: joinDungeons, Comment: comment})
				}
			}
			preState := rc.State
			rc = s.server.Features.LFG.UpdateRoleCheck(s.groupID, s.playerGUID, roles8)
			if rc != nil {
				s.sendRoleCheckAnswerNotifies(rc, s.playerGUID, roles8, preState)
				if rc.State == LFGRoleCheckFinished {
					multi := len(rc.Roles) > 1
					if multi {
						// Multi-member group: the finished group itself enters
						// the queue (AddQueueData, LFGMgr.cpp:868-872).
						s.server.Features.LFG.QueueGroup(s.groupID, s.playerGUID, rc.Dungeons, rc.Roles, comment)
					} else {
						// Single-member group: the joiner queues solo.
						_, _ = s.server.Features.LFG.Join(s.playerGUID, roles8, joinDungeons, comment)
					}
					s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
						buildLFGUpdateParty(LFGUpdateAddedToQueue, LFGQueueEntry{Dungeons: rc.Dungeons, State: LFGStateQueued}))
					if multi {
						// The matcher runs at once, like the C++ update tick.
						for _, prop := range s.server.Features.LFG.FindGroups() {
							s.server.deliverLFGProposal(prop)
						}
					}
				}
			}
			return true
		}
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
		if err := s.sendLFGUpdatePlayer(LFGUpdateJoinQueue, entry); err != nil {
			return true
		}
		// SoloLFG: the C++ queue matcher forms proposals from underfilled
		// queue sets on its update tick (LFGQueue.cpp:425, 523 arms); the solo proposal is formed directly from the
		// queued entry. Grouped joins never reach this arm — they branch
		// to the role check above.
		if s.groupID == 0 {
			if prop := s.server.Features.LFG.FormSoloProposal(s.playerGUID); prop != nil {
				if err := s.sendLFGProposalUpdate(prop); err != nil {
					return true
				}
			}
		}
		return true
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
	// LFGMgr::LeaveLfg (LFGMgr.cpp:714-751): the QUEUED arm acts on the group
	// when the leaver is grouped — every member leaves the queue and each
	// member receives the REMOVED_FROM_QUEUE party update. (The queue stays
	// per-player in Go, so "the group is queued" means any member holds an
	// entry.) A leave with nothing queued sends nothing. The mid-dungeon arm
	// (newState = oldState for DUNGEON/FINISHED_DUNGEON) needs no code: Go
	// never resets the group's LFG state on leave.
	lfg := s.server.Features.LFG
	if s.groupID != 0 && s.server != nil {
		// LFGMgr::LeaveLfg (LFGMgr.cpp:763-766): the ROLECHECK arm
		// aborts a live check instead of leaving the queue — members
		// hold no queue entry while a check runs, so the queue-leave
		// loop below would otherwise silently no-op. No RoleChosen
		// packet goes out (C++ sends it only for a player answer);
		// every member gets the ABORTED role-check update, the leader
		// the FAILED join result, and the group the ROLECHECK_FAILED
		// party update (the UpdateRoleCheck default arm, LFGMgr.cpp:858-866).
		if rc := lfg.AbortRoleCheck(s.groupID); rc != nil {
			guids := make([]uint64, 0, len(rc.Roles))
			for guid := range rc.Roles {
				guids = append(guids, guid)
			}
			sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
			for _, guid := range guids {
				if ms := s.server.findSessionByGUID(guid); ms != nil {
					_ = ms.sendLFGRoleCheckUpdate(rc)
				}
			}
			if ls := s.server.findSessionByGUID(rc.Leader); ls != nil {
				_ = ls.sendLFGJoinResult(LFGJoinFailed, LFGRoleCheckAborted)
			}
			s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
				buildLFGUpdateParty(LFGUpdateRoleCheckFailed, LFGQueueEntry{}))
			return true
		}
		// LFGMgr::LeaveLfg (LFGMgr.cpp:726-748): the QUEUED arm restores
		// a mid-dungeon re-queued group to its dungeon state — Go's
		// group state never leaves DUNGEON on re-queue, so an LFG group
		// in DUNGEON/FINISHED with a dungeon is the restore case, and
		// only QUEUED member entries flip; members still in DUNGEON keep
		// their state and, with nothing queued, no packet goes out (the
		// DUNGEON arm, LFGMgr.cpp:783-786, guid == gguid).
		restore := false
		var restoreState uint8
		if g := s.server.getGroup(s.groupID); g != nil && g.IsLFG && g.LFGDungeonID != 0 &&
			(g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon) {
			restore = true
			restoreState = g.LFGState
		}
		removed := false
		for _, ms := range s.server.getGroupSessions(s.groupID) {
			if restore {
				if entry, ok := lfg.Status(ms.playerGUID); ok && entry.State == LFGStateQueued {
					lfg.SetState(ms.playerGUID, restoreState)
					removed = true
					continue
				}
			}
			if lfg.Leave(ms.playerGUID) {
				removed = true
			}
		}
		if lfg.LeaveGroup(s.groupID) {
			removed = true
		}
		if !removed {
			return true
		}
		s.debug("lfg leave", "account", s.accountName)
		for _, ms := range s.server.getGroupSessions(s.groupID) {
			_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: ms.playerGUID, State: LFGStateNone})
		}
		return true
	}
	if entry, ok := lfg.Status(s.playerGUID); ok && (entry.State == LFGStateDungeon || entry.State == LFGStateFinishedDungeon) {
		// LFGMgr::LeaveLfg (LFGMgr.cpp:783-786): the DUNGEON/FINISHED arm
		// clears a solo player's state and sends nothing.
		lfg.SetState(s.playerGUID, LFGStateNone)
		return true
	}
	if !lfg.Leave(s.playerGUID) {
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
	// LFGMgr::GetLfgStatus (LFGMgr.cpp:2202-2206) builds the update with the
	// 4-arg ctor whose comment defaults to "" — both packets carry an empty
	// comment, only the second has dungeons cleared.
	noComment := entry
	noComment.Comment = ""
	cleared := entry
	cleared.Dungeons = nil
	cleared.Comment = ""
	if s.groupID != 0 {
		if err := s.sendLFGUpdateParty(LFGUpdateStatus, noComment); err != nil {
			return true
		}
		return s.sendLFGUpdatePlayer(LFGUpdateStatus, cleared) == nil
	}
	if err := s.sendLFGUpdatePlayer(LFGUpdateStatus, noComment); err != nil {
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

// sendLFGOfferContinue mirrors WorldSession::SendLfgOfferContinue
// (LFGHandler.cpp:612-620): SMSG_LFG_OFFER_CONTINUE carries only the
// dungeon entry, sent to the group leader when a member leaves a
// non-finished LFG dungeon.
func (s *session) sendLFGOfferContinue(dungeonID uint32) error {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(dungeonID)
	return s.write(uint16(protocol.OpcodeSMSG_LFG_OFFER_CONTINUE), packet.Bytes(), true)
}

// sendLfgLfrList mirrors WorldSession::SendLfgLfrList (LFGHandler.cpp:596-605):
// SMSG_LFG_UPDATE_SEARCH with a single update byte; false clears the client's
// search list.
func (s *session) sendLfgLfrList(update bool) error {
	return s.write(uint16(protocol.OpcodeSMSG_LFG_UPDATE_SEARCH), []byte{boolToU8(update)}, true)
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

// lfgGroupMemberRemoved mirrors LFGGroupScript::OnRemoveMember
// (LFGScripts.cpp:183-228): the LeaveLfg arms for the removed player, the
// Dungeon Deserter cast on mid-dungeon leaves, the Dungeon Cooldown aura
// strip on KICK_LFG removals, the SMSG_LFG_UPDATE_PARTY ping, the
// teleport-out of the dungeon, and the SMSG_LFG_OFFER_CONTINUE to the
// leader on non-FINISHED removals. The KICK arm (InitBoot) never reaches here:
// Group::RemoveMember returns before removal for LFG+KICK (Group.cpp:588),
// and Go starts the boot at the uninvite handlers and the Lua bridge
// instead. The per-group data writes (SetGroup/RemovePlayerFromGroup) have
// no Go counterpart — group membership lives in groupState.Members.
func (s *Server) lfgGroupMemberRemoved(g *groupState, target *session, method uint8) {
	if s == nil || g == nil || !g.IsLFG || target == nil || target.player == nil {
		return
	}
	if s.Features != nil && s.Features.LFG != nil &&
		s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	// LFGMgr::LeaveLfg (LFGMgr.cpp:714-800) switches on the removed
	// player's own entry state and runs before the general-path arms.
	// Go tracks PROPOSAL on player entries, not on groupState, so the
	// OnRemoveMember PROPOSAL check (group state + DEFAULT method) is
	// proxied on the removed player's entry: a grouped member's entry is
	// PROPOSAL exactly while their group has an active proposal.
	if s.Features != nil && s.Features.LFG != nil {
		if entry, ok := s.Features.LFG.Status(target.playerGUID); ok {
			switch entry.State {
			case LFGStateProposal:
				if method == groupRemoveMethodDefault {
					return
				}
				// LEAVE/KICK_LFG during a proposal reach LeaveLfg's
				// PROPOSAL arm (LFGMgr.cpp:754-779): the leaver is
				// marked DENY and the proposal is removed declined;
				// the general arms below still run, as in C++.
				if prop := s.Features.LFG.DeclineProposalForPlayer(target.playerGUID); prop != nil {
					s.sendLFGProposalDeclined(prop)
				}
			case LFGStateQueued:
				// LeaveLfg QUEUED arm: the whole queued group leaves
				// the queue. Exclude is 0 because C++ runs LeaveLfg
				// before RemovePlayerFromGroup, so the removed player
				// is still group LFG membership and flips + gets the
				// REMOVED send like everyone else. Restore mirrors the
				// OnAddMember arm: Go's group state never leaves
				// DUNGEON on re-queue, so it is the OldState.
				restore := g.IsLFG && g.LFGDungeonID != 0 &&
					(g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon)
				affected := s.Features.LFG.LeaveQueuedGroup(g.ID, 0, restore, g.LFGState)
				inAffected := false
				for _, guid := range affected {
					if guid == target.playerGUID {
						inAffected = true
					}
					if ms := s.findSessionByGUID(guid); ms != nil {
						_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
					}
				}
				if !inAffected {
					// Not in the queue entry's role map: C++ flips
					// every group player, so flip the removed player
					// directly instead of leaving a stale QUEUED entry.
					if restore {
						s.Features.LFG.SetState(target.playerGUID, g.LFGState)
					} else {
						s.Features.LFG.SetState(target.playerGUID, LFGStateNone)
					}
					_ = target.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
				}
			case LFGStateDungeon, LFGStateFinishedDungeon:
				// LeaveLfg DUNGEON/FINISHED arm: the removed player
				// (not the group, not a disconnect — logout never
				// reaches this hook) drops to NONE; Go's SetState
				// (NONE) deletes the entry, which is C++'s
				// clear-roles/dungeons/comment.
				s.Features.LFG.SetState(target.playerGUID, LFGStateNone)
			}
		}
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
	// LFGScripts.cpp:224-227: a non-FINISHED LFG group offers the leader
	// the chance to continue — the state is the group state from before
	// the LeaveLfg arms (g.LFGState is untouched by them), and the leader
	// is the pre-removal one (ChangeLeader runs after the hook in C++
	// Group::RemoveMember and in Go removeFromGroup alike). The dungeon
	// is GetDungeon(gguid, false), the group's raw LFGDungeonID.
	if g.LFGState != LFGStateFinishedDungeon {
		if leader := s.findSessionByGUID(g.LeaderGUID); leader != nil {
			_ = leader.sendLFGOfferContinue(g.LFGDungeonID)
		}
	}
}

// lfgGroupMemberRemovedOffline runs the session-less subset of
// LFGGroupScript::OnRemoveMember (LFGScripts.cpp:183-228) for victims
// removed while offline: Group::RemoveMember (Group.cpp:569) fires the
// script before any online check, but Go's removeGroupMemberByGUID offline
// branch previously ran only the Eluna event, so an offline-kicked member
// of an LFG group kept a stale DUNGEON/FINISHED entry, a stale group-queue
// membership, or a live proposal — the desync LFGPlayerScript::OnLogin
// (LFGScripts.cpp:55-78) repairs in C++ only when the stored group differs
// from the live group, which Go's login never sees because the row is
// already deleted. The victim-session arms are skipped, matching the C++
// FindPlayer guards: no Dungeon Deserter cast, no Dungeon Cooldown strip,
// no victim update-party ping, no dungeon teleport. The leader offer still
// goes out (leader-session-guarded in C++ too). A KICK on an LFG group
// runs nothing here: the C++ script KICK arm returns before LeaveLfg
// (Group.cpp:588-589, InitBoot instead), and Go's Lua bridge already
// diverts LFG kicks to a boot vote before reaching the offline path.
func (s *Server) lfgGroupMemberRemovedOffline(g *groupState, guid uint64, method uint8) {
	if s == nil || g == nil || !g.IsLFG || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	if method == groupRemoveMethodKick {
		return
	}
	lfg := s.Features.LFG
	// LFGMgr::LeaveLfg ROLECHECK arm (LFGMgr.cpp:763-766): a live check
	// containing the victim aborts. Go sets no per-player ROLECHECK
	// entry, so the check is keyed on the group and the victim's
	// membership in its role map; the abort restores the pre-check state
	// in C++, and Go never overwrote the entries, so the entry switch
	// below is skipped on this arm. Notifies mirror handleLFGLeave's
	// abort arm; the offline victim gets none of them.
	abortedCheck := false
	if rc := lfg.GetRoleCheck(g.ID); rc != nil {
		if _, ok := rc.Roles[guid]; ok {
			if aborted := lfg.AbortRoleCheck(g.ID); aborted != nil {
				abortedCheck = true
				guids := make([]uint64, 0, len(aborted.Roles))
				for mguid := range aborted.Roles {
					guids = append(guids, mguid)
				}
				sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
				for _, mguid := range guids {
					if ms := s.findSessionByGUID(mguid); ms != nil {
						_ = ms.sendLFGRoleCheckUpdate(aborted)
					}
				}
				if ls := s.findSessionByGUID(aborted.Leader); ls != nil {
					_ = ls.sendLFGJoinResult(LFGJoinFailed, LFGRoleCheckAborted)
				}
				s.broadcastToGroup(g.ID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
					buildLFGUpdateParty(LFGUpdateRoleCheckFailed, LFGQueueEntry{}))
			}
		}
	}
	if !abortedCheck {
		if entry, ok := lfg.Status(guid); ok {
			switch entry.State {
			case LFGStateProposal:
				if method == groupRemoveMethodDefault {
					return
				}
				// LEAVE/KICK_LFG during a proposal reach LeaveLfg's
				// PROPOSAL arm (LFGMgr.cpp:754-779); the decline
				// notify skips nil sessions, so the offline denier
				// sends nothing while online members still hear it.
				if prop := lfg.DeclineProposalForPlayer(guid); prop != nil {
					s.sendLFGProposalDeclined(prop)
				}
			case LFGStateQueued:
				// LeaveLfg QUEUED arm, same shape as the online hook:
				// the whole queued group leaves the queue, members
				// flip to the restore state or NONE, and online
				// members get the REMOVED send. Exclude is 0 because
				// C++ runs LeaveLfg before RemovePlayerFromGroup.
				restore := g.IsLFG && g.LFGDungeonID != 0 &&
					(g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon)
				affected := lfg.LeaveQueuedGroup(g.ID, 0, restore, g.LFGState)
				inAffected := false
				for _, aguid := range affected {
					if aguid == guid {
						inAffected = true
					}
					if ms := s.findSessionByGUID(aguid); ms != nil {
						_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
					}
				}
				if !inAffected {
					// Not in the queue entry's role map: C++ flips
					// every group player, so flip the victim directly
					// instead of leaving a stale QUEUED entry.
					if restore {
						lfg.SetState(guid, g.LFGState)
					} else {
						lfg.SetState(guid, LFGStateNone)
					}
				}
			case LFGStateDungeon, LFGStateFinishedDungeon:
				// LeaveLfg DUNGEON/FINISHED arm: the removed player
				// (not a disconnect — logout never reaches this hook)
				// drops to NONE.
				lfg.SetState(guid, LFGStateNone)
			}
		}
	}
	// LFGScripts.cpp:224-227: a non-FINISHED LFG group offers the leader
	// the chance to continue; the state is the pre-removal group state
	// (g.LFGState is untouched by the LeaveLfg arms above).
	if g.LFGState != LFGStateFinishedDungeon {
		if leader := s.findSessionByGUID(g.LeaderGUID); leader != nil {
			_ = leader.sendLFGOfferContinue(g.LFGDungeonID)
		}
	}
}

// lfgOnPlayerLogout mirrors LFGPlayerScript::OnLogout (LFGScripts.cpp:41-53),
// which the old unconditional Leave in triggerLogout did not: a grouped player
// on a clean logout is a C++ no-op (the grouped branch requires
// PlayerDisconnected), while a grouped disconnect runs LeaveLfg(guid, true)
// and a groupless logout runs LeaveLfg(guid) with disconnected=false — the
// groupless branch never checks PlayerDisconnected, so a groupless DUNGEON
// entry clears on disconnect too. triggerLogout runs before
// removeSessionFromGroup, so groupID here is still the live group, matching
// C++ player->GetGroup() at hook time. Logout never fires OnRemoveMember, so
// there is no leader offer-continue arm here, unlike the removal hooks.
func (s *Server) lfgOnPlayerLogout(sess *session, disconnected bool) {
	if s == nil || sess == nil || s.Features == nil || s.Features.LFG == nil {
		return
	}
	lfg := s.Features.LFG
	if lfg.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	if sess.groupID == 0 {
		_ = sess.sendLfgLfrList(false)
		entry, ok := lfg.Status(sess.playerGUID)
		if !ok {
			return
		}
		switch entry.State {
		case LFGStateQueued:
			lfg.Leave(sess.playerGUID)
		case LFGStateProposal:
			if prop := lfg.DeclineProposalForPlayer(sess.playerGUID); prop != nil {
				s.sendLFGProposalDeclined(prop)
			}
		case LFGStateDungeon, LFGStateFinishedDungeon:
			// LeaveLfg DUNGEON/FINISHED arm with disconnected=false: the
			// groupless player drops to NONE on clean logout and on
			// disconnect alike.
			lfg.SetState(sess.playerGUID, LFGStateNone)
		}
		return
	}
	if !disconnected {
		return
	}
	g := s.getGroup(sess.groupID)
	if g == nil || !g.IsLFG {
		lfg.Leave(sess.playerGUID)
		return
	}
	// LFGMgr::LeaveLfg ROLECHECK arm (LFGMgr.cpp:763-766): a live check
	// containing the disconnecting player aborts; notifies mirror the
	// offline-removal arm, and the disconnecting player gets none of them.
	if rc := lfg.GetRoleCheck(g.ID); rc != nil {
		if _, ok := rc.Roles[sess.playerGUID]; ok {
			if aborted := lfg.AbortRoleCheck(g.ID); aborted != nil {
				guids := make([]uint64, 0, len(aborted.Roles))
				for mguid := range aborted.Roles {
					guids = append(guids, mguid)
				}
				sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
				for _, mguid := range guids {
					if ms := s.findSessionByGUID(mguid); ms != nil {
						_ = ms.sendLFGRoleCheckUpdate(aborted)
					}
				}
				if ls := s.findSessionByGUID(aborted.Leader); ls != nil {
					_ = ls.sendLFGJoinResult(LFGJoinFailed, LFGRoleCheckAborted)
				}
				s.broadcastToGroup(g.ID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
					buildLFGUpdateParty(LFGUpdateRoleCheckFailed, LFGQueueEntry{}))
			}
		}
	}
	entry, ok := lfg.Status(sess.playerGUID)
	if !ok {
		return
	}
	switch entry.State {
	case LFGStateProposal:
		if prop := lfg.DeclineProposalForPlayer(sess.playerGUID); prop != nil {
			s.sendLFGProposalDeclined(prop)
		}
	case LFGStateQueued:
		restore := g.IsLFG && g.LFGDungeonID != 0 &&
			(g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon)
		affected := lfg.LeaveQueuedGroup(g.ID, 0, restore, g.LFGState)
		if affected == nil {
			// No queued group entry: the player entry is a solo queue
			// entry, dropped like the groupless QUEUED arm.
			lfg.Leave(sess.playerGUID)
			return
		}
		inAffected := false
		for _, aguid := range affected {
			if aguid == sess.playerGUID {
				inAffected = true
			}
			if ms := s.findSessionByGUID(aguid); ms != nil {
				_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
			}
		}
		if !inAffected {
			if restore {
				lfg.SetState(sess.playerGUID, g.LFGState)
			} else {
				lfg.SetState(sess.playerGUID, LFGStateNone)
			}
		}
	case LFGStateDungeon, LFGStateFinishedDungeon:
		// LeaveLfg DUNGEON/FINISHED arm with disconnected=true: the
		// disconnecting player keeps the dungeon state.
	}
}

// lfgGroupDisbanded mirrors LFGMgr::RemoveGroupData (LFGMgr.cpp:2010-2030)
// as fired by Group::Disband's OnGroupDisband: every listed member's player
// entry drops to NONE with a REMOVED_FROM_QUEUE update-party, unless the
// member is mid-proposal (the C++ group-state PROPOSAL skip, proxied per
// member — Go tracks PROPOSAL on player entries, not on groupState). A
// group with no LFG footprint (no member entry and no dungeon/finished
// group state) has no C++ GroupsStore row, so RemoveGroupData returns
// early and nothing is sent.
func (s *Server) lfgGroupDisbanded(g *groupState, memberGUIDs []uint64) {
	if s == nil || g == nil || !g.IsLFG || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	footprint := g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon
	if !footprint {
		for _, guid := range memberGUIDs {
			if _, ok := s.Features.LFG.Status(guid); ok {
				footprint = true
				break
			}
		}
	}
	if !footprint {
		return
	}
	for _, guid := range memberGUIDs {
		if entry, ok := s.Features.LFG.Status(guid); ok && entry.State == LFGStateProposal {
			continue
		}
		s.Features.LFG.SetState(guid, LFGStateNone)
		if ms := s.findSessionByGUID(guid); ms != nil {
			_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
		}
	}
}

// lfgGroupQueued reports whether the group holds a QUEUED group-queue entry,
// the group-state proxy for the QUEUED arm of LFGMgr::LeaveLfg (LFGMgr.cpp:723-751)
// when the caller must not drop the entry speculatively (LeaveQueuedGroup deletes it).
func (m *LFGManager) lfgGroupQueued(groupID uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.groupQueue[groupID]
	return ok && entry.State == LFGStateQueued
}

// lfgInDungeonMap mirrors LFGMgr::inLfgDungeonMap (LFGMgr.cpp:2266-2277) for a
// live group: the group's dungeon (GetDungeon(gguid, true), the masked id)
// resolves to a dungeon whose map and difficulty match the player's current
// map and difficulty setting. The downscaled-instance edge (a map created at
// a lower difficulty than the player's setting) has no Go model — the
// instance difficulty is not stored on the player — so the setting is the
// comparison, matching the non-downscaled common case.
func (s *session) lfgInDungeonMap(g *groupState) bool {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil || g == nil || !g.IsLFG || g.LFGDungeonID == 0 {
		return false
	}
	dungeon, found, err := s.server.Data.LFGDungeon(g.LFGDungeonID & 0x00FFFFFF)
	if err != nil || !found || dungeon.MapID < 0 {
		return false
	}
	if uint32(dungeon.MapID) != s.player.Map {
		return false
	}
	diff := uint32(s.player.DungeonDifficulty)
	if entry, found, err := s.server.Data.Map(s.player.Map); err == nil && found && entry.IsRaid() {
		diff = uint32(s.player.RaidDifficulty)
	}
	return dungeon.Difficulty == diff
}

// lfgEntryInDungeonMap is the groupless analog of inLfgDungeonMap: a player
// with no live group but a surviving DUNGEON entry (a group disbanded while
// they were offline leaves their entry untouched — disband clears listed
// members only) is "in" the dungeon when the entry's first dungeon resolves
// to the current map at the player's difficulty. It is the Go shape of the
// C++ desync the groupless arm guards (LFGScripts.cpp:93-100): LFG data
// still names a dungeon, but no valid group exists. FINISHED entries carry
// no dungeons (SetState clears them), so they never qualify here.
func (s *session) lfgEntryInDungeonMap(entry LFGQueueEntry) bool {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return false
	}
	if entry.State != LFGStateDungeon || len(entry.Dungeons) == 0 {
		return false
	}
	dungeon, found, err := s.server.Data.LFGDungeon(entry.Dungeons[0] & 0x00FFFFFF)
	if err != nil || !found || dungeon.MapID < 0 {
		return false
	}
	if uint32(dungeon.MapID) != s.player.Map {
		return false
	}
	diff := uint32(s.player.DungeonDifficulty)
	if mapEntry, found, err := s.server.Data.Map(s.player.Map); err == nil && found && mapEntry.IsRaid() {
		diff = uint32(s.player.RaidDifficulty)
	}
	return dungeon.Difficulty == diff
}

// leaveLFGPlayerForMapChange runs LFGMgr::LeaveLfg (LFGMgr.cpp:714-788) for a
// groupless player at a map change: QUEUED drops the queue entry with the
// player REMOVED send, PROPOSAL declines via the proposal's decline notify,
// DUNGEON/FINISHED clears the state silently; NONE is a no-op. A groupless
// player holds no group-keyed role check, so the ROLECHECK arm cannot fire.
func (s *session) leaveLFGPlayerForMapChange() {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return
	}
	lfg := s.server.Features.LFG
	entry, ok := lfg.Status(s.playerGUID)
	if !ok {
		return
	}
	switch entry.State {
	case LFGStateProposal:
		if prop := lfg.DeclineProposalForPlayer(s.playerGUID); prop != nil {
			s.server.sendLFGProposalDeclined(prop)
		}
	case LFGStateQueued:
		if lfg.Leave(s.playerGUID) {
			_ = s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone})
		}
	case LFGStateDungeon, LFGStateFinishedDungeon:
		lfg.SetState(s.playerGUID, LFGStateNone)
	}
}

// leaveLFGGroupForMapChange runs the group-guid arm of LFGMgr::LeaveLfg
// (LFGMgr.cpp:714-788) before the map-change disband: ROLECHECK aborts the
// live check with the handleLFGLeave abort notifies, PROPOSAL declines via
// the group leader with the decline notify, QUEUED drops the group queue
// entry with REMOVED sends; DUNGEON/FINISHED/NONE are no-ops for a group
// guid (the DUNGEON arm only clears a lone player guid).
func (s *session) leaveLFGGroupForMapChange(g *groupState, leaderGUID uint64) {
	if s == nil || s.server == nil || g == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return
	}
	lfg := s.server.Features.LFG
	if rc := lfg.AbortRoleCheck(g.ID); rc != nil {
		guids := make([]uint64, 0, len(rc.Roles))
		for guid := range rc.Roles {
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		for _, guid := range guids {
			if ms := s.server.findSessionByGUID(guid); ms != nil {
				_ = ms.sendLFGRoleCheckUpdate(rc)
			}
		}
		if ls := s.server.findSessionByGUID(rc.Leader); ls != nil {
			_ = ls.sendLFGJoinResult(LFGJoinFailed, LFGRoleCheckAborted)
		}
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
			buildLFGUpdateParty(LFGUpdateRoleCheckFailed, LFGQueueEntry{}))
		return
	}
	if prop := lfg.DeclineProposalForPlayer(leaderGUID); prop != nil {
		s.server.sendLFGProposalDeclined(prop)
		return
	}
	if !lfg.lfgGroupQueued(g.ID) {
		return
	}
	restore := g.IsLFG && g.LFGDungeonID != 0 &&
		(g.LFGState == LFGStateDungeon || g.LFGState == LFGStateFinishedDungeon)
	affected := lfg.LeaveQueuedGroup(g.ID, 0, restore, g.LFGState)
	inAffected := false
	for _, guid := range affected {
		if guid == s.playerGUID {
			inAffected = true
		}
		if ms := s.server.findSessionByGUID(guid); ms != nil {
			_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
		}
	}
	if !inAffected {
		if restore {
			lfg.SetState(s.playerGUID, g.LFGState)
		} else {
			lfg.SetState(s.playerGUID, LFGStateNone)
		}
		_ = s.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
	}
}

// lfgOnMapChanged mirrors LFGPlayerScript::OnMapChanged (LFGScripts.cpp:93-131)
// at Go's map-change completion points: the tail of completeWorldPort and the
// login enter-world path in handlePlayerLogin. allowTeleport is false at
// login: the groupless homebind redirect needs placement-stage handling, not
// a teleport in the middle of the login packet flow, so login runs only the
// LeaveLfg + aura legs (a documented residual, not a silent skip).
//
// The in-dungeon branch's name-query sends have no Go twin and are skipped
// deliberately: the client already learns group members' names from
// SMSG_GROUP_LIST and requests names on demand (handleNameQuery), so the
// proactive send is redundant. The single-member disband arm is gated to
// LFG groups: C++'s count-1 survivors are exactly the LFG/BG/BF
// RemoveMember exceptions (Group.cpp:725-739), Go's BG groups never survive
// at 1 (the shared removeFromGroup disband), and a non-LFG 1-member Go list
// is a logout-detach artifact whose offline members are still DB members —
// disbanding it would dissolve a group C++ keeps. For LFG groups the list
// is the membership (DBID-0 by design, offline members never rejoin), so a
// 1-member LFG group is real, and the solo-proposal group is the reachable
// case: its sole member leaving the dungeon map disbands it.
func (s *session) lfgOnMapChanged(ctx context.Context, allowTeleport bool) {
	if s == nil || s.server == nil || s.player == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return
	}
	lfg := s.server.Features.LFG
	var grp *groupState
	var memberGUIDs []uint64
	var leaderGUID uint64
	var isLFG bool
	if s.groupID != 0 {
		s.server.groupsMu.RLock()
		if g := s.server.groups[s.groupID]; g != nil {
			grp = g
			leaderGUID = g.LeaderGUID
			isLFG = g.IsLFG
			for _, m := range g.Members {
				memberGUIDs = append(memberGUIDs, m.GUID)
			}
		}
		s.server.groupsMu.RUnlock()
	}
	if grp != nil && s.lfgInDungeonMap(grp) {
		if s.server.lfgSelectedRandomDungeon(s.playerGUID) {
			s.applyAuraWithDuration(lfgSpellLuckOfTheDraw, 7200000)
		}
		return
	}
	if grp == nil {
		if entry, ok := lfg.Status(s.playerGUID); ok && s.lfgEntryInDungeonMap(entry) {
			s.leaveLFGPlayerForMapChange()
			s.removeAura(lfgSpellLuckOfTheDraw)
			if allowTeleport {
				s.teleportTo(s.player.HomebindMap, s.player.HomebindX, s.player.HomebindY, s.player.HomebindZ, 0)
			}
			return
		}
	}
	s.removeAura(lfgSpellLuckOfTheDraw)
	if grp == nil || !isLFG || len(memberGUIDs) != 1 || memberGUIDs[0] != s.playerGUID || s.playerLoading {
		return
	}
	s.leaveLFGGroupForMapChange(grp, leaderGUID)
	s.server.groupsMu.RLock()
	stillGrouped := s.server.groups[grp.ID] == grp
	s.server.groupsMu.RUnlock()
	if stillGrouped {
		s.server.disbandGroup(ctx, grp, false)
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
	if s.Features != nil && s.Features.LFG != nil &&
		s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
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
	if s.Features != nil && s.Features.LFG != nil &&
		s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
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

// updateLFGRoleCheckExpiry mirrors the role-check arm of LFGMgr::Update
// (LFGMgr.cpp:301-321): an unanswered check past cancelTime fails as
// MISSING_ROLE — every member gets the role-check update carrying that
// state and the leader the FAILED join result. No ROLECHECK_FAILED party
// update goes out on this arm (C++ sends none here), and the whole sweep
// is skipped while the dungeon finder and raid browser are both disabled
// (the LFGMgr::Update early return, LFGMgr.cpp:294-296).
func (s *Server) updateLFGRoleCheckExpiry(now time.Time) {
	if s == nil || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	for _, rc := range s.Features.LFG.ExpireRoleChecks(now) {
		guids := make([]uint64, 0, len(rc.Roles))
		for guid := range rc.Roles {
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		for _, guid := range guids {
			if ms := s.findSessionByGUID(guid); ms != nil {
				_ = ms.sendLFGRoleCheckUpdate(rc)
				if guid == rc.Leader {
					_ = ms.sendLFGJoinResult(LFGJoinFailed, LFGRoleCheckMissingRole)
				}
			}
		}
	}
}

// updateLFGProposalExpiry mirrors the proposal arm of LFGMgr::Update
// (LFGMgr.cpp:324-328) through RemoveProposal with PROPOSAL_FAILED:
// every member gets the FAILED proposal update, denied players (and
// agreeing players in a denied player's group) get the failure update
// and lose their queue entry, and agreeing players outside a denied
// group get the ADDED_TO_QUEUE re-add (C++ re-queues them on this arm).
func (s *Server) updateLFGProposalExpiry(now time.Time) {
	if s == nil || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	for _, prop := range s.Features.LFG.ExpireProposals(now) {
		deniedGroups := make(map[uint64]bool)
		for _, player := range prop.Players {
			if player.Accept == LFGAnswerDeny && player.Group != 0 {
				deniedGroups[player.Group] = true
			}
		}
		guids := make([]uint64, 0, len(prop.Players))
		for guid := range prop.Players {
			guids = append(guids, guid)
		}
		sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
		for _, guid := range guids {
			player := prop.Players[guid]
			ms := s.findSessionByGUID(guid)
			if ms == nil {
				continue
			}
			_ = ms.sendLFGProposalUpdate(prop)
			switch {
			case player.Accept == LFGAnswerDeny:
				if player.Group != 0 {
					_ = ms.sendLFGUpdateParty(LFGUpdateProposalFailed, LFGQueueEntry{})
				} else {
					_ = ms.sendLFGUpdatePlayer(LFGUpdateProposalFailed, LFGQueueEntry{GUID: guid, State: LFGStateNone})
				}
			case player.Group != 0 && deniedGroups[player.Group]:
				_ = ms.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
			default:
				entry := LFGQueueEntry{Dungeons: []uint32{prop.DungeonID}, State: LFGStateQueued}
				if cur, ok := s.Features.LFG.Status(guid); ok {
					entry = cur
				}
				if player.Group != 0 {
					_ = ms.sendLFGUpdateParty(LFGUpdateAddedToQueue, entry)
				} else {
					_ = ms.sendLFGUpdatePlayer(LFGUpdateAddedToQueue, entry)
				}
			}
		}
	}
}

// updateLFGFindGroups mirrors the FindGroups arm of LFGMgr::Update (LFGMgr.cpp:352-356): each update tick forms proposals from the queued groups and players.
func (s *Server) updateLFGFindGroups() {
	if s == nil || s.Features == nil || s.Features.LFG == nil {
		return
	}
	if s.Features.LFG.GetOptions()&(LFGOptionEnableDungeonFinder|LFGOptionEnableRaidBrowser) == 0 {
		return
	}
	for _, prop := range s.Features.LFG.FindGroups() {
		s.deliverLFGProposal(prop)
	}
}

// deliverLFGProposal mirrors the proposal-notify arm of LFGMgr::Update (LFGMgr.cpp:360-373): every proposal member gets the proposal update, grouped members the PROPOSAL_BEGIN party update and solo members the player update.
func (s *Server) deliverLFGProposal(prop *LFGProposal) {
	if s == nil || prop == nil {
		return
	}
	guids := make([]uint64, 0, len(prop.Players))
	for guid := range prop.Players {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	for _, guid := range guids {
		ms := s.findSessionByGUID(guid)
		if ms == nil {
			continue
		}
		_ = ms.sendLFGProposalUpdate(prop)
		entry := LFGQueueEntry{Dungeons: []uint32{prop.DungeonID}, State: LFGStateProposal}
		if cur, ok := s.Features.LFG.Status(guid); ok {
			entry = cur
		}
		if prop.Players[guid].Group != 0 {
			_ = ms.sendLFGUpdateParty(LFGUpdateProposalBegin, entry)
		} else {
			_ = ms.sendLFGUpdatePlayer(LFGUpdateProposalBegin, entry)
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

func (s *session) teleportToLFGDungeon(dungeonID uint32, fromOpcode bool) bool {
	if s.player == nil {
		return false
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
	ok := s.teleportTo(mapID, x, y, z, ori)
	if ok && luck {
		s.applyAuraWithDuration(lfgSpellLuckOfTheDraw, 7200000)
	}
	return ok
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

// sendRoleCheckAnswerNotifies mirrors the per-member tail of
// LFGMgr::UpdateRoleCheck (LFGMgr.cpp:838-852): while a check is in
// progress every member gets SMSG_LFG_ROLE_CHOSEN for the answerer
// (sendRoleChosen = the check state before this answer was not DEFAULT)
// followed by SMSG_LFG_ROLE_CHECK_UPDATE. The caller handles the
// terminal arms (FINISHED queueing, failure notifies).
func (s *session) sendRoleCheckAnswerNotifies(rc *LfgRoleCheck, answererGUID uint64, answerRoles uint8, preState uint32) {
	if rc == nil || s.server == nil {
		return
	}
	if preState != LFGRoleCheckDefault {
		// LFGMgr::SendLfgRoleChosen (LFGMgr.cpp:2094-2098):
		// SMSG_LFG_ROLE_CHOSEN (guid, ready, roles) unicast to each
		// role-check member's session, not a group broadcast.
		packet := protocol.NewBuffer(13)
		packet.WriteU64(answererGUID)
		if answerRoles > 0 {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU32(uint32(answerRoles))
		pkt := packet.Bytes()
		for memberGUID := range rc.Roles {
			if ms := s.server.findSessionByGUID(memberGUID); ms != nil {
				_ = ms.write(uint16(protocol.OpcodeSMSG_LFG_ROLE_CHOSEN), pkt, true)
			}
		}
	}
	for memberGUID := range rc.Roles {
		if ms := s.server.findSessionByGUID(memberGUID); ms != nil {
			_ = ms.sendLFGRoleCheckUpdate(rc)
		}
	}
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
	var race, dungeonDifficulty, raidDifficulty uint8
	if targetSess != nil {
		if targetSess.player != nil {
			level = targetSess.player.Level
			race = targetSess.player.Race
			dungeonDifficulty = targetSess.player.DungeonDifficulty
			raidDifficulty = targetSess.player.RaidDifficulty
		}
		expansion = targetSess.accountExpansion
	} else if guid == s.playerGUID && s.player != nil {
		level = s.player.Level
		race = s.player.Race
		dungeonDifficulty = s.player.DungeonDifficulty
		raidDifficulty = s.player.RaidDifficulty
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

	// 2. Access requirements (LFGMgr.cpp:1878-1896): the full row is read —
	// item_level, achievement, quest_A/H, item/item2 — keyed by
	// (map, difficulty) like C++'s GetAccessRequirement.
	type accessRequirement struct {
		itemLevel   uint32
		achievement uint32
		questA      uint32
		questH      uint32
		item        uint32
		item2       uint32
	}
	accessReqs := make(map[boundKey]accessRequirement)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.Query(
			"SELECT mapId, difficulty, item_level, achievement, quest_A, quest_H, item, item2 FROM access_requirement",
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mapID, diff uint32
				var ar accessRequirement
				if err := rows.Scan(&mapID, &diff, &ar.itemLevel, &ar.achievement, &ar.questA, &ar.questH, &ar.item, &ar.item2); err == nil {
					accessReqs[boundKey{mapID: mapID, difficulty: diff}] = ar
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

	// DisableMgr::IsDisabledFor(DISABLE_TYPE_MAP/DISABLE_TYPE_LFG_MAP, map,
	// player) (DisableMgr.cpp:373-392): the disables-table flags are
	// difficulty-mode bits (DUNGEON_STATUSFLAG_* = 1<<difficulty,
	// SharedDefines.h:3730-3736); the target's own dungeon/raid difficulty
	// selects the bit. GetDownscaledMapDifficultyData has no Go twin
	// (documented delta).
	type disableKey struct {
		sourceType uint32
		entry      uint32
	}
	disableFlags := make(map[disableKey]uint32)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.Query(
			"SELECT sourceType, entry, flags FROM disables WHERE sourceType IN (2, 8)",
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sourceType, entry, flags uint32
				if err := rows.Scan(&sourceType, &entry, &flags); err == nil {
					k := disableKey{sourceType: sourceType, entry: entry}
					disableFlags[k] |= flags
				}
			}
		}
	}
	mapDisabled := func(sourceType uint32, mapID uint32, isRaid bool) bool {
		flags, ok := disableFlags[disableKey{sourceType: sourceType, entry: mapID}]
		if !ok {
			return false
		}
		diff := dungeonDifficulty
		if isRaid {
			diff = raidDifficulty
		}
		return flags&(1<<diff) != 0
	}

	charStore := s.server.CharactersStore
	hasAchievement := func(achievementID uint32) bool {
		if charStore == nil || charStore.DB == nil {
			return false
		}
		var one int
		return charStore.DB.QueryRow("SELECT 1 FROM character_achievement WHERE guid = ? AND achievement = ?", guid, achievementID).Scan(&one) == nil
	}
	questRewarded := func(questID uint32) bool {
		if charStore == nil || charStore.DB == nil {
			return false
		}
		var n int64
		if err := charStore.DB.QueryRow("SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ? AND active = 1", guid, questID).Scan(&n); err == nil {
			return n > 0
		}
		return false
	}
	hasItem := func(entry uint32) bool {
		if charStore == nil || charStore.DB == nil {
			return false
		}
		var owned int64
		if err := charStore.DB.QueryRow(`SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci
			JOIN item_instance ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ii.itemEntry = ?`, guid, entry).Scan(&owned); err == nil {
			return owned > 0
		}
		return false
	}
	isAlliance := isAllianceRace(race)

	// LFGMgr::GetLockedDungeons (LFGMgr.cpp:1840-1905) arm order: denyJoin,
	// expansion, DISABLE_TYPE_MAP, DISABLE_TYPE_LFG_MAP, bound instance,
	// min/max level, seasonal, access requirements. The iteration covers
	// every dungeon including randoms (LFGMgr.cpp:272 fills
	// CachedDungeonMapStore[0] with all ids).
	for _, dungeon := range allDungeons {
		mapID := uint32(dungeon.MapID)
		bk := boundKey{mapID: mapID, difficulty: dungeon.Difficulty}
		isRaid := dungeon.TypeID == LFGDungeonTypeRaid

		var lockData uint32
		if denyJoin {
			lockData = LFGLockStatusRaidLocked
		} else if dungeon.ExpansionLevel > uint32(expansion) {
			lockData = LFGLockStatusInsufficientExpansion
		} else if mapDisabled(2, mapID, isRaid) {
			lockData = LFGLockStatusRaidLocked
		} else if mapDisabled(8, mapID, isRaid) {
			lockData = LFGLockStatusRaidLocked
		} else if dungeon.Difficulty > 0 && boundInstances[bk] {
			lockData = LFGLockStatusRaidLocked
		} else if uint32(level) < dungeon.MinLevel {
			lockData = LFGLockStatusTooLowLevel
		} else if dungeon.MaxLevel > 0 && uint32(level) > dungeon.MaxLevel {
			lockData = LFGLockStatusTooHighLevel
		} else if dungeon.Flags&lfgFlagSeasonal != 0 && !s.server.lfgSeasonActive(dungeon.ID) {
			lockData = LFGLockStatusNotInSeason
		} else if ar, ok := accessReqs[bk]; ok {
			if ar.itemLevel > 0 && avgItemLevel < float32(ar.itemLevel) {
				lockData = LFGLockStatusTooLowGearScore
			} else if ar.achievement != 0 && !hasAchievement(ar.achievement) {
				lockData = LFGLockStatusMissingAchievement
			} else if isAlliance && ar.questA != 0 && !questRewarded(ar.questA) {
				lockData = LFGLockStatusQuestNotCompleted
			} else if !isAlliance && ar.questH != 0 && !questRewarded(ar.questH) {
				lockData = LFGLockStatusQuestNotCompleted
			} else if ar.item != 0 && !hasItem(ar.item) && (ar.item2 == 0 || !hasItem(ar.item2)) {
				lockData = LFGLockStatusMissingItem
			} else if ar.item == 0 && ar.item2 != 0 && !hasItem(ar.item2) {
				lockData = LFGLockStatusMissingItem
			}
		}

		if lockData != 0 {
			locks[dungeon.Entry()] = lockData
		}
	}

	return locks
}

// lfgSeasonActive mirrors LFGMgr::IsSeasonActive (LFGMgr.cpp:2208-2221):
// seasonal dungeons 285-288 are active only while their holiday runs
// (holiday ids from SharedDefines.h:2787-2802).
func (s *Server) lfgSeasonActive(dungeonID uint32) bool {
	var holiday int64
	switch dungeonID {
	case 285: // The Headless Horseman
		holiday = 324 // HOLIDAY_HALLOWS_END
	case 286: // The Frost Lord Ahune
		holiday = 341 // HOLIDAY_FIRE_FESTIVAL
	case 287: // Coren Direbrew
		holiday = 372 // HOLIDAY_BREWFEST
	case 288: // The Crown Chemical Co.
		holiday = 423 // HOLIDAY_LOVE_IS_IN_THE_AIR
	default:
		return false
	}
	_, ok := s.cachedActiveGameHolidays(context.Background())[holiday]
	return ok
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
				// LFGMgr::GetRandomAndSeasonalDungeons (LFGMgr.cpp:2288-2297):
				// seasonal dungeons join the list while their season is active.
				if d.TypeID == LFGDungeonTypeRandom ||
					(d.Flags&lfgFlagSeasonal != 0 && s.server.lfgSeasonActive(d.ID)) {
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
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel FROM quest_template WHERE ID = ?", questID).Scan(&qLevel)
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
// sendLFGProposalDeclined sends the LFGMgr::RemoveProposal notify arm with
// PROPOSAL_DECLINED (LFGMgr.cpp:1306-1380), mirroring the
// updateLFGProposalExpiry FAILED arm: deniers get the decline update, their
// group-mates the REMOVED update, and agreeing players outside a denied
// group the ADDED_TO_QUEUE re-add — each on the party channel when grouped,
// the player channel when solo. Pending players are NOT marked deny on this
// arm (only the FAILED arm does that), so a pending group-mate of a denier
// lands in the REMOVED case via deniedGroups, not the decline case. The
// proposal must already have been declined and removed from the manager.
func (s *Server) sendLFGProposalDeclined(proposal *LFGProposal) {
	if s == nil || proposal == nil {
		return
	}
	deniedGroups := make(map[uint64]bool)
	for _, player := range proposal.Players {
		if player.Accept == LFGAnswerDeny && player.Group != 0 {
			deniedGroups[player.Group] = true
		}
	}
	guids := make([]uint64, 0, len(proposal.Players))
	for guid := range proposal.Players {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	for _, pguid := range guids {
		player := proposal.Players[pguid]
		ps := s.findSessionByGUID(pguid)
		if ps == nil {
			continue
		}
		_ = ps.sendLFGProposalUpdate(proposal)
		switch {
		case player.Accept == LFGAnswerDeny:
			if player.Group != 0 {
				_ = ps.sendLFGUpdateParty(LFGUpdateProposalDeclined, LFGQueueEntry{})
			} else {
				_ = ps.sendLFGUpdatePlayer(LFGUpdateProposalDeclined, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
			}
		case player.Group != 0 && deniedGroups[player.Group]:
			_ = ps.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{})
		default:
			entry := LFGQueueEntry{Dungeons: []uint32{proposal.DungeonID}, State: LFGStateQueued}
			if cur, ok := s.Features.LFG.Status(pguid); ok {
				entry = cur
			}
			if player.Group != 0 {
				_ = ps.sendLFGUpdateParty(LFGUpdateAddedToQueue, entry)
			} else {
				_ = ps.sendLFGUpdatePlayer(LFGUpdateAddedToQueue, entry)
			}
		}
	}
}

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
		s.server.sendLFGProposalDeclined(proposal)
		return true
	}

	if proposal.State == LFGProposalSuccess {
		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGProposalUpdate(proposal)
				// LFGMgr::UpdateProposal (LFGMgr.cpp:1243-1266): a grouped
				// member gets the GROUP_FOUND party update, a solo member
				// the player update; every member then gets REMOVED on
				// both channels. Grouped members also record their source
				// group size for the FinishDungeon achievement count
				// (LFGMgr.cpp:1286-1289); the sends run before MakeNewGroup
				// moves anyone, so groupID is still the source group.
				if proposal.Players[pguid].Group != 0 {
					_ = ps.sendLFGUpdateParty(LFGUpdateGroupFound, LFGQueueEntry{GUID: pguid, State: LFGStateDungeon})
					if ps.groupID != 0 {
						s.server.groupsMu.RLock()
						if grp := s.server.groups[ps.groupID]; grp != nil {
							ps.lfgPartyMembersAtJoin = uint8(len(grp.Members))
						}
						s.server.groupsMu.RUnlock()
					}
				} else {
					_ = ps.sendLFGUpdatePlayer(LFGUpdateGroupFound, LFGQueueEntry{GUID: pguid, State: LFGStateDungeon})
				}
				_ = ps.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
				_ = ps.sendLFGUpdateParty(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
			}
		}

		// LFGMgr::MakeNewGroup (LFGMgr.cpp:1011-1151): ONE group for the
		// whole proposal — the first connected player in leader, tank,
		// healer, dps order creates it (Group::Create), every later
		// player is removed from any other group (Group::RemoveMember)
		// and added (Group::AddMember, roles via SetLfgRoles). The old
		// arms broke both halves: the Group != 0 arm set LFG flags on
		// the existing group but added no proposal player to it, and
		// the Group == 0 arm created one single-member group per
		// player instead of one shared group. A grouped proposal rides
		// the existing group; a missing group falls through to
		// creation, like C++'s proposal.group ? GetGroupByGUID : new.
		var diff uint8
		if dungeon, found, derr := s.server.Data.LFGDungeon(proposal.DungeonID); derr == nil && found {
			diff = uint8(dungeon.Difficulty)
		}
		ordered := make([]uint64, 0, len(proposal.Players))
		if _, ok := proposal.Players[proposal.Leader]; ok {
			ordered = append(ordered, proposal.Leader)
		}
		for _, role := range []uint8{LFGRoleTank, LFGRoleHealer, LFGRoleDamage} {
			for pguid, pdata := range proposal.Players {
				if pguid != proposal.Leader && pdata.Role&^LFGRoleLeader == role {
					ordered = append(ordered, pguid)
				}
			}
		}
		for pguid := range proposal.Players {
			seen := false
			for _, g := range ordered {
				if g == pguid {
					seen = true
					break
				}
			}
			if !seen {
				ordered = append(ordered, pguid)
			}
		}
		var grp *groupState
		if proposal.Group != 0 {
			grp = s.server.getGroup(proposal.Group)
		}
		for _, pguid := range ordered {
			pdata := proposal.Players[pguid]
			ps := s.server.findSessionByGUID(pguid)
			if ps == nil || ps.player == nil {
				continue
			}
			if grp == nil {
				s.server.groupsMu.Lock()
				grp = &groupState{
					ID:            newGroupID(),
					LeaderGUID:    pguid,
					LootMethod:    3,
					LootThreshold: 2,
					LooterGUID:    pguid,
					DungeonDiff:   diff,
					IsLFG:         true,
					LFGDungeonID:  proposal.DungeonID,
					LFGState:      LFGStateDungeon,
					LFGKicksLeft:  lfgGroupMaxKicks,
					Members:       []groupMember{{GUID: pguid, Name: ps.player.Name, Roles: pdata.Role}},
				}
				s.server.groups[grp.ID] = grp
				ps.groupID = grp.ID
				// Group::Create (Group.cpp:158) sets PLAYER_FLAGS_GROUP_LEADER
				// on the leader at creation.
				ps.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(ps.player.PlayerFlags, true)
				s.server.groupsMu.Unlock()
				continue
			}
			if ps.groupID != 0 && ps.groupID != grp.ID {
				if old := s.server.getGroup(ps.groupID); old != nil {
					_ = ps.removeFromGroup(ctx, old, ps, groupRemoveMethodDefault)
				} else {
					ps.groupID = 0
				}
			}
			s.server.groupsMu.Lock()
			added := true
			for i := range grp.Members {
				if grp.Members[i].GUID == pguid {
					grp.Members[i].Roles = pdata.Role
					added = false
					break
				}
			}
			if added {
				grp.Members = append(grp.Members, groupMember{GUID: pguid, Name: ps.player.Name, Roles: pdata.Role})
				refreshGroupMaxEnchantingLevel(s.server, grp)
				ps.groupID = grp.ID
				if pguid == grp.LeaderGUID {
					ps.player.PlayerFlags = UpdatePlayerGroupLeaderFlag(ps.player.PlayerFlags, true)
				}
			}
			dbID := grp.DBID
			s.server.groupsMu.Unlock()
			// Group::AddMember (Group.cpp:465) inserts the member row
			// for every non-BG join; DBID 0 (BG raids, LFG groups by
			// design) skips it, matching the invite-accept arm.
			if added && dbID != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_INS_GROUP_MEMBER"), dbID, pguid, 0, 0, 0)
			}
		}
		if grp != nil {
			s.server.groupsMu.Lock()
			grp.IsLFG = true
			grp.LFGDungeonID = proposal.DungeonID
			grp.LFGState = LFGStateDungeon
			grp.LFGKicksLeft = lfgGroupMaxKicks
			grp.DungeonDiff = diff
			dbGroupID := grp.DBID
			s.server.groupsMu.Unlock()
			// The lfg_data write keeps the old Group != 0 arm's shape:
			// a DB-loaded group writes under its DB id, a runtime
			// group under the proposal group id; a freshly created
			// group (Group == 0, DBID 0) writes nothing, preserving
			// the deliberate DBID-0 design.
			if proposal.Group != 0 {
				if dbGroupID == 0 {
					dbGroupID = uint32(proposal.Group)
				}
				if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
					_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "REPLACE INTO lfg_data (guid, dungeon, state) VALUES (?, ?, ?)", dbGroupID, proposal.DungeonID, LFGStateDungeon)
				}
			}
			// MakeNewGroup ends with grp->SendUpdate(): one list
			// broadcast for the whole proposal, not one per member.
			s.server.broadcastGroupList(grp)
		}

		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				// LFGMgr::UpdateProposal (LFGMgr.cpp:1127-1141): players who
				// queued for a random dungeon take the Dungeon Cooldown aura
				// on proposal accept.
				if s.server.lfgSelectedRandomDungeon(pguid) {
					ps.applyAura(lfgSpellDungeonCooldown)
				}
				// LFGMgr::TeleportPlayer (LFGMgr.cpp:1543-1548): a player
				// already on the dungeon's map is not teleported (the
				// call errors instead of moving them); the cooldown above
				// still applies, since MakeNewGroup grants it before the
				// teleport pass. The !isNew playersToTeleport skip is
				// vacuous here: every Go-formed proposal is isNew (solo
				// proposals are new groups, grouped proposals have no
				// live former), so it needs an IsNew field only when a
				// queue matcher lands.
				if ps.player != nil {
					if mapID, _, _, _, _ := ps.getLFGDungeonEntrance(proposal.DungeonID); mapID != 0 && ps.player.Map == mapID {
						continue
					}
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

		// LFGMgr::UpdateRoleCheck (LFGMgr.cpp:811) computes sendRoleChosen
		// from the check's state BEFORE this answer is applied: the packet
		// only goes out while a role check is actually in progress. With no
		// check running C++ sends nothing here.
		preState := uint32(LFGRoleCheckDefault)
		if existing := lfg.GetRoleCheck(s.groupID); existing != nil {
			preState = existing.State
		}
		rc := lfg.UpdateRoleCheck(s.groupID, s.playerGUID, roles)
		if rc != nil {
			s.sendRoleCheckAnswerNotifies(rc, s.playerGUID, roles, preState)
			if rc.State == LFGRoleCheckFinished {
				s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
					buildLFGUpdateParty(LFGUpdateAddedToQueue, LFGQueueEntry{Dungeons: rc.Dungeons, State: LFGStateQueued}))
			} else if rc.State != LFGRoleCheckInitializing {
				// LFGMgr::UpdateRoleCheck default arm (LFGMgr.cpp:858-866):
				// the check leader gets the FAILED join result and every
				// member gets the ROLECHECK_FAILED party update.
				if ls := s.server.findSessionByGUID(rc.Leader); ls != nil {
					_ = ls.sendLFGJoinResult(LFGJoinFailed, rc.State)
				}
				s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY),
					buildLFGUpdateParty(LFGUpdateRoleCheckFailed, LFGQueueEntry{}))
			}
		}
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

	// LFGMgr::TeleportPlayer (LFGMgr.cpp:1572-1578): a failed TeleportTo
	// reports LFG_TELEPORTERROR_INVALID_LOCATION.
	if !s.teleportToLFGDungeon(lfgDungeonID, true) {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(LFGTeleportErrorInvalidLocation)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
	}
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
		// HandleLfgSetCommentOpcode -> LFGMgr::SetComment (LFGMgr.cpp:1984):
		// PlayersStore[guid] creates the entry, so the comment persists for
		// non-queued players and survives LeaveLfg; it never materializes
		// queue state.
		s.server.Features.LFG.SetComment(s.playerGUID, comment)
	}
	s.debug("lfg set comment", "account", s.accountName, "comment", comment)
	return true
}

// completeLFGDungeon rewards players in an LFG dungeon group and triggers criteria.
// mapID is the instance map the encounter completed on (InstanceScript's
// currMap); the per-player reward arms check it before paying out.
func (s *Server) completeLFGDungeon(groupID uint32, dungeonID uint32, mapID uint32) {
	s.groupsMu.Lock()
	grp := s.groups[uint64(groupID)]
	if grp == nil || !grp.IsLFG {
		s.groupsMu.Unlock()
		return
	}
	// LFGMgr::FinishDungeon head (LFGMgr.cpp:1595-1601): the group must
	// be queued for this dungeon — GetDungeon masks to the low 24 bits,
	// as does the stored id here — or the finish is a no-op and the
	// stored dungeon is left untouched.
	if grp.LFGDungeonID&0x00FFFFFF != dungeonID&0x00FFFFFF {
		s.groupsMu.Unlock()
		return
	}
	if grp.LFGState == LFGStateFinishedDungeon {
		s.groupsMu.Unlock()
		return
	}
	grp.LFGState = LFGStateFinishedDungeon
	// GetDungeon(gguid, false) is the group's raw stored dungeon, not the
	// finished dungeon id — capture it before the overwrite below, since
	// SendLfgPlayerReward sends it as sdungeonEntry.
	storedGroupDungeon := grp.LFGDungeonID
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
	// LFGMgr::FinishDungeon (LFGMgr.cpp:1661-1663): the dungeonDone map
	// check reads the finished dungeon's DBC map, not the selected one.
	var dungeonDoneMap uint32
	dungeonDoneOK := false
	if s.Data != nil {
		if dungeonDone, found, err := s.Data.LFGDungeon(dungeonID & 0x00FFFFFF); err == nil && found {
			dungeonDoneMap = uint32(dungeonDone.MapID)
			dungeonDoneOK = true
		}
	}
	for _, m := range members {
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1615-1626): a player already
		// FINISHED is skipped entirely, and every other member flips to
		// FINISHED — which clears roles and selected dungeons
		// (LFGPlayerData::SetState) — before any reward is computed, so
		// the first selected dungeon is captured before the flip.
		var rDungeonID uint32
		if s.Features != nil && s.Features.LFG != nil {
			entry, ok := s.Features.LFG.Status(m.GUID)
			if ok && entry.State == LFGStateFinishedDungeon {
				continue
			}
			if ok && len(entry.Dungeons) > 0 {
				rDungeonID = entry.Dungeons[0]
			}
			s.Features.LFG.SetState(m.GUID, LFGStateFinishedDungeon)
		}
		sess := s.findSessionByGUID(m.GUID)
		if sess == nil || sess.player == nil {
			continue
		}
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1631-1643): rewards only for
		// a random or seasonal selected dungeon; a specific-dungeon
		// selection (or no selection at all) pays nothing.
		if s.Data == nil {
			continue
		}
		rDungeon, found, err := s.Data.LFGDungeon(rDungeonID & 0x00FFFFFF)
		if err != nil || !found {
			continue
		}
		if rDungeon.TypeID != LFGDungeonTypeRandom && rDungeon.Flags&lfgFlagSeasonal == 0 {
			continue
		}
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1650-1651): a player on a
		// different map than the completed instance gets no reward.
		if sess.player.Map != mapID {
			continue
		}
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1653): the dungeon cooldown
		// aura comes off before the dungeon-map check, so a player in
		// the instance but on the wrong dungeon map still loses it.
		sess.removeAura(lfgSpellDungeonCooldown)
		if !dungeonDoneOK || sess.player.Map != dungeonDoneMap {
			continue
		}
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1664-1673): the only LFG
		// achievement criteria the C++ server fires is
		// USE_LFD_TO_GROUP_WITH_PLAYERS, for heroic random dungeons,
		// miscValue = 5 minus the party size at join recorded at
		// proposal success, or the fallback 4 when that count is
		// unknown (zero). The C++ never fires LFG_COMPLETION or
		// LFG_DUNGEON_REWARD. The difficulty gate reads the selected
		// random dungeon, not the finished one.
		if rDungeon.Difficulty == 1 {
			misc := uint32(4)
			if sess.lfgPartyMembersAtJoin != 0 {
				misc = 5 - uint32(sess.lfgPartyMembersAtJoin)
			}
			sess.updateAchievementCriteria(criteriaTypeUseLFDToGroup, misc, 1)
		}
		detail, done, ok := sess.lfgFinishDungeonReward(context.Background(), rDungeon.ID, sess.player.Level)
		if !ok {
			continue
		}
		// LFGMgr::FinishDungeon (LFGMgr.cpp:1686-1696): the grant runs
		// before the reward packet, and it never suppresses the packet —
		// a full inventory mails the overflow in C++ instead of failing
		// the grant, so a Go grant error is logged and the packet still
		// goes out (the overflow-mail arm is a documented residual).
		sess.rewardDungeonFinderQuest(context.Background(), detail.ID)
		_ = sess.sendLFGDungeonReward(context.Background(), rDungeon.Entry(), storedGroupDungeon, done, detail)
	}
}

// lfgFinishDungeonReward mirrors the FinishDungeon reward-quest selection
// (LFGMgr.cpp:1672-1696): the lfg_dungeon_rewards row for the selected
// random dungeon whose maxLevel first reaches the player level (an
// out-leveled player keeps the last, highest-maxLevel row, per the
// GetRandomDungeonReward assign-before-break quirk), firstQuest unless
// the player can no longer earn it, in which case otherQuest with
// done set. ok is false when no quest template resolves, matching the
// C++ continue arms. The grant itself lives in rewardDungeonFinderQuest,
// called by the finish loop before the reward packet, mirroring the
// C++ RewardQuest-then-SendLfgPlayerReward order.
func (s *session) lfgFinishDungeonReward(ctx context.Context, dungeonID uint32, level uint8) (questDetailData, bool, bool) {
	var firstQuest, otherQuest uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx,
			"SELECT firstQuestId, otherQuestId, maxLevel FROM lfg_dungeon_rewards WHERE dungeonId = ? ORDER BY maxLevel ASC",
			dungeonID)
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
	if firstQuest == 0 {
		return questDetailData{}, false, false
	}
	detail, err := s.loadQuestDetailData(ctx, firstQuest)
	if err != nil {
		return questDetailData{}, false, false
	}
	// Player::CanRewardQuest (Player.cpp:15059) fails once the daily is
	// turned in; same derivation as writeLFGRandomDungeonReward.
	done := s.isQuestRewarded(ctx, firstQuest)
	if canTake, err := s.canTakeQuest(ctx, firstQuest); err == nil && !canTake {
		if status, serr := s.characterQuestStatus(ctx, firstQuest); serr != nil || status == 0 {
			done = true
		}
	}
	if done {
		if otherQuest == 0 {
			return questDetailData{}, false, false
		}
		other, err := s.loadQuestDetailData(ctx, otherQuest)
		if err != nil {
			return questDetailData{}, false, false
		}
		return other, true, true
	}
	return detail, false, true
}

// rewardDungeonFinderQuest mirrors the RewardQuest half of
// LFGMgr::FinishDungeon (LFGMgr.cpp:1686-1696): RewardQuest(quest, 0,
// nullptr, false) on a quest the player never accepted. There is no
// quest-log gate to bypass — commitQuestReward never checked log state,
// only the turn-in packet did — so the log-free property falls out of
// calling commit directly, as rewardTrackingQuest already does for the
// TRACKING arm. The DF options skip the claimed-window check (the
// otherQuest fallback grants even when recorded for the day) and the
// strict turn-in destroy (C++ DestroyItemCount never fails). Giver 0
// skips the single questgiver-status packet and the turn-in chain in
// applyQuestRewardGrants, matching the nullptr giver; announce=false
// skips the quest-complete packet.
func (s *session) rewardDungeonFinderQuest(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil {
		return
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		s.debug("dungeon-finder reward load failed", "account", s.accountName, "quest", questID, "error", err)
		return
	}
	grants, destroyedGUIDs, err := s.commitQuestRewardOptions(ctx, view, 0, true, true)
	if err != nil {
		s.debug("dungeon-finder reward grant failed", "account", s.accountName, "quest", questID, "error", err)
		return
	}
	s.applyQuestRewardGrants(ctx, view, questID, 0, 0, grants, destroyedGUIDs, false)
}

// sendLFGDungeonReward sends SMSG_LFG_PLAYER_REWARD (0x1FF) to the client,
// mirroring WorldSession::SendLfgPlayerReward (LFGHandler.cpp:478-510):
// random dungeon entry, finished dungeon entry, done flag, a constant 1,
// the quest's reward-or-required money and XP reward, two zero words,
// then the quest's non-empty reward items with display ids and counts.
// A zero random or finished entry, like a missing quest, sends nothing.
func (s *session) sendLFGDungeonReward(ctx context.Context, rdungeonEntry, sdungeonEntry uint32, done bool, detail questDetailData) error {
	if rdungeonEntry == 0 || sdungeonEntry == 0 || detail.ID == 0 {
		return nil
	}
	packet := protocol.NewBuffer(64)
	packet.WriteU32(rdungeonEntry)
	packet.WriteU32(sdungeonEntry)
	if done {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(1)
	packet.WriteU32(uint32(s.questRewOrReqMoney(detail)))
	packet.WriteU32(s.questXPRewardRaw(ctx, detail.ID, detail.RewardXPDifficulty))
	packet.WriteU32(0)
	packet.WriteU32(0)
	var count uint8
	for _, item := range detail.RewardItems {
		if item.ID != 0 {
			count++
		}
	}
	packet.WriteU8(count)
	for _, item := range detail.RewardItems {
		if item.ID == 0 {
			continue
		}
		packet.WriteU32(item.ID)
		packet.WriteU32(item.DisplayID)
		packet.WriteU32(item.Quantity)
	}
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
