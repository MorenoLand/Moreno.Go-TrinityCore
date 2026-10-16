package world

// Battleground queue membership structures, ported from
// src/server/game/Battlegrounds/BattlegroundQueue.h. The queue wait-time ring
// (m_WaitTimes) already lives in battleground.go; this file covers the queued
// player/group records and the invite-selection pool (BattlegroundQueue.cpp:64-124).
//
// NOT ported (unported dependencies): BattlegroundQueue::AddGroup
// (BattlegroundQueue.cpp:133-238) walks Group*/Player* and calls into the
// world announcer config, ChatHandler and the BG template registry — no Go
// forms of those exist; BattlegroundQueue::RemovePlayer (BattlegroundQueue.cpp:288)
// mutates the queued containers plus the invite events, which ride on the same
// Group*/Player* machinery; BGQueueInviteEvent/BGQueueRemoveEvent
// (BattlegroundQueue.cpp:471-476, 1018-1103) are already bridged in Go by the
// timers in simulateBGQueueInvite (battleground.go).

// bgQueuePlayerInfo is the Go form of PlayerQueueInfo (BattlegroundQueue.h:33-38):
// per-player data for someone waiting in the battleground queue.
type bgQueuePlayerInfo struct {
	lastOnlineTime uint32            // for tracking and removing offline players from queue after 5 minutes
	groupInfo      *bgGroupQueueInfo // pointer to the associated group queue info
}

// bgGroupQueueInfo is the Go form of GroupQueueInfo (BattlegroundQueue.h:40-58):
// per-group data for a group waiting in the battleground queue (also used for solo joins).
type bgGroupQueueInfo struct {
	players                   map[uint64]*bgQueuePlayerInfo // player queue info by guid
	team                      uint32                        // ALLIANCE/HORDE (teamAlliance/teamHorde)
	bgTypeID                  uint32                        // battleground type id
	isRated                   bool                          // rated
	arenaType                 uint8                         // 2v2, 3v3, 5v5 or 0 when BG
	arenaTeamID               uint32                        // team id if rated match
	joinTime                  uint32                        // time when group was added
	removeInviteTime          uint32                        // time when we will remove invite for players in group
	isInvitedToBGInstanceGUID uint32                        // was invited to certain BG
	arenaTeamRating           uint32                        // if rated match, inited to the rating of the team
	arenaMatchmakerRating     uint32                        // if rated match, inited to the rating of the team
	opponentsTeamRating       uint32                        // for rated arena matches
	opponentsMatchmakerRating uint32                        // for rated arena matches
	previousOpponentsTeamID   uint32                        // excluded from the current queue until the timer is met
}

// BG queue group-type indices (BattlegroundQueue.h:58-65): the second dimension of the
// C++ m_QueuedGroups array — premade groups/rated arena teams vs normal groups, per faction.
const (
	bgQueuePremadeAlliance uint8 = 0
	bgQueuePremadeHorde    uint8 = 1
	bgQueueNormalAlliance  uint8 = 2
	bgQueueNormalHorde     uint8 = 3
)

// bgQueueGroupTypesCount mirrors BG_QUEUE_GROUP_TYPES_COUNT (BattlegroundQueue.h:65).
const bgQueueGroupTypesCount = 4

// BG queue invitation types (BattlegroundQueue.h:67-72): how the matchmaker balances
// team sizes when filling a battleground.
const (
	bgQueueInvitationNoBalance uint8 = 0 // no balance: N+M vs N players
	bgQueueInvitationBalanced  uint8 = 1 // teams balanced: N+1 vs N players
	bgQueueInvitationEven      uint8 = 2 // teams even: N vs N players
)

// bgSelectionPool is the Go form of BattlegroundQueue::SelectionPool
// (BattlegroundQueue.h:113-126): the per-side group list the matchmaker builds when
// filling a battleground, with the running head-count of selected players.
type bgSelectionPool struct {
	selectedGroups []*bgGroupQueueInfo
	players        uint32
}

// playerCount mirrors SelectionPool::GetPlayerCount (BattlegroundQueue.h:120).
func (p *bgSelectionPool) playerCount() uint32 {
	return p.players
}

// init ports BattlegroundQueue::SelectionPool::Init (BattlegroundQueue.cpp:70-73):
// clears the selection from the previous run.
func (p *bgSelectionPool) init() {
	p.selectedGroups = nil
	p.players = 0
}

// addGroup ports BattlegroundQueue::SelectionPool::AddGroup (BattlegroundQueue.cpp:113-124):
// a group already invited to an instance is never selected again, and a group larger than
// the desired head-count stays out. Returns true when the pool can still take players
// (or the group was added), false when the pool is full.
func (p *bgSelectionPool) addGroup(ginfo *bgGroupQueueInfo, desiredCount uint32) bool {
	size := uint32(len(ginfo.players))
	// if group is larger than desired count - don't allow to add it to pool
	if ginfo.isInvitedToBGInstanceGUID == 0 && desiredCount >= p.players+size {
		p.selectedGroups = append(p.selectedGroups, ginfo)
		p.players += size
		return true
	}
	if p.players < desiredCount {
		return true
	}
	return false
}

// kickGroup ports BattlegroundQueue::SelectionPool::KickGroup (BattlegroundQueue.cpp:80-102):
// removes the largest group — or the last group whose size is within 1 of the target —
// and updates the player count. Returns true when the caller should try adding another
// group, false when the pool is fine (kicked a smaller-enough group) or empty.
func (p *bgSelectionPool) kickGroup(size uint32) bool {
	// find maxgroup or LAST group with size == size and kick it
	found := false
	kick := 0
	for i, g := range p.selectedGroups {
		n := uint32(len(g.players))
		if bgAbsDiff(n, size) <= 1 {
			kick = i
			found = true
		} else if !found && n >= uint32(len(p.selectedGroups[kick].players)) {
			kick = i
		}
	}
	// if pool is empty, do nothing
	if p.playerCount() > 0 {
		ginfo := p.selectedGroups[kick]
		p.selectedGroups = append(p.selectedGroups[:kick], p.selectedGroups[kick+1:]...)
		p.players -= uint32(len(ginfo.players))
		// return false if we kicked smaller group or there are enough players in selection pool
		if uint32(len(ginfo.players)) <= size+1 {
			return false
		}
	}
	return true
}

// bgAbsDiff is the |a - b| used by KickGroup's size window (BattlegroundQueue.cpp:86).
func bgAbsDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
