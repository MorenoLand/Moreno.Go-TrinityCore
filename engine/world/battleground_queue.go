package world

// Battleground queue membership structures, ported from
// src/server/game/Battlegrounds/BattlegroundQueue.h. The queue wait-time ring
// (m_WaitTimes) already lives in battleground.go; this file covers the queued
// player/group records, the pure arms of BattlegroundQueue::AddGroup (ginfo
// construction, queue index math, per-player wiring) and the per-player erase
// arm of BattlegroundQueue::RemovePlayer, plus the invite-selection pool
// (BattlegroundQueue.cpp:64-124).
//
// PORTED: BattlegroundQueue::AddGroup's arena-join world announce
// (BattlegroundQueue.cpp:166-170) — the Arena.QueueAnnouncer.Enable config
// (engine/config), the arena_team name+rating lookup (the
// sArenaTeamMgr->GetArenaTeamById analog) and broadcastMessageChatAll (the
// sWorld->SendWorldText analog) all exist; wired in battleground.go for both
// the solo and group rated-join paths. The 718/719 enUS texts are recalled
// from TDB (unseeded in-tree — verify when trinity_string is seeded).
// PORTED: the rated-exit world announce
// (BattlegroundQueue.cpp:360-362) — the solo ginfo empties when the leaver
// goes, so handleBattlefieldPort's leave-queue arm announces for
// !ArenaGroupJoin entries, and the group last-leaver detection rides the
// leave-arm roster scan (uninvited groups) and the arena expire-arm roster
// scan (invited groups, same-pop instance id) in battleground.go.
// NOT ported (unported dependencies): the m_QueuedGroups push
// (BattlegroundQueue.cpp:198) and the queue-status announce
// (BattlegroundQueue.cpp:201-235 — CONFIG_BATTLEGROUND_QUEUE_ANNOUNCER_ENABLE/_PLAYERONLY):
// qAlliance/qHorde count every queued group in the bracket's normal queues
// and need the server-level m_QueuedGroups containers the per-session sim
// deliberately replaces — no dead container is added. BattlegroundQueue::RemovePlayer's
// container arms
// (BattlegroundQueue.cpp:288-407): the m_QueuedPlayers lookup (:294-303), the
// bracket search (:310-331 — brackets scanned high to low, premade+normal slots
// of the group's own team stepped by PVP_TEAMS_COUNT), the m_QueuedPlayers
// erase (:357), the invited-count decrease (:352-354 — needs a live BG
// instance from sBattlegroundMgr; the Go sim is per-session), the
// MemberLost/OfflineMemberLost rated arm
// (:365-376 — the loss mechanism exists (arena_rating.go's MemberLost shape,
// applyArenaMMRChange) but the arm's input, the opponent's matchmaker rating
// (group->OpponentsMatchmakerRating, wired by the rated matchmaker the
// per-session sim never runs), is unfabricable; the BattleGroundHandler :496-507
// twin looks up GetArenaTeamById(ginfo.Team) and misses — see battleground.go),
// the empty-group container
// erase (:380-384). The recursive rated-group removal with its STATUS_NONE
// status packet (:389-406) is PORTED in battleground.go's leave-queue arm via
// the group roster (findGroupByID) and per-member slot clears. All of the rest need the
// server-level m_QueuedPlayers/m_QueuedGroups containers that the per-session
// sim (battleground.go handleBattlemasterJoin) deliberately replaces — no dead
// container is added here. BGQueueInviteEvent/BGQueueRemoveEvent
// (BattlegroundQueue.cpp:471-476, 1018-1103) are already bridged in Go by the
// timers in simulateBGQueueInvite (battleground.go); the arena legs of those
// events are mirrored by simulateArenaQueueInvite, which also carries the
// BattlegroundMgr::CreateNewBattleground pop arm (BattlegroundQueue.cpp:983)
// and the Arena::AddPlayer admit on port accept.

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

// bgQueueGroupIndex ports the queue-slot computation in BattlegroundQueue::AddGroup
// (BattlegroundQueue.cpp:156-160): non-rated, non-premade groups land in the normal
// slots (PVP_TEAMS_COUNT == 2 == bgQueueNormalAlliance), rated and premade groups in
// the premade slots; Horde takes the +1 slot of its pair. team is the C++ Team enum
// value (teamAlliance/teamHorde), as stored on bgGroupQueueInfo.team.
func bgQueueGroupIndex(isRated, isPremade bool, team uint32) uint8 {
	index := uint8(0)
	if !isRated && !isPremade {
		index += bgQueueNormalAlliance
	}
	if team == teamHorde {
		index++
	}
	return index
}

// newBGGroupQueueInfo ports the ginfo construction in BattlegroundQueue::AddGroup
// (BattlegroundQueue.cpp:138-151) plus the per-player wiring (BattlegroundQueue.cpp:173-194):
// every member GUID gets a bgQueuePlayerInfo whose LastOnlineTime equals the join
// time, with its GroupInfo back-pointer set, mirrored into the record's players map
// (the C++ m_QueuedPlayers map itself is server-level and stays unported — see the
// file header). leaderTeam is leader->GetTeam() (BattlegroundQueue.cpp:146), the C++
// Team enum value (teamAlliance/teamHorde); opponents ratings start at 0
// (BattlegroundQueue.cpp:150-151). The m_QueuedGroups push (BattlegroundQueue.cpp:198)
// and the queue-status announce (:201-235) stay unported — see the file header;
// the arena-join world announce (:166-170) is PORTED in battleground.go's solo and
// group rated-join paths.
func newBGGroupQueueInfo(bgTypeID uint32, arenaType uint8, arenaTeamID uint32, isRated bool, arenaRating, matchmakerRating, previousOpponentsTeamID uint32, leaderTeam uint32, memberGUIDs []uint64) *bgGroupQueueInfo {
	now := gameTimeMS()
	ginfo := &bgGroupQueueInfo{
		team:                      leaderTeam,
		bgTypeID:                  bgTypeID,
		isRated:                   isRated,
		arenaType:                 arenaType,
		arenaTeamID:               arenaTeamID,
		joinTime:                  now,
		removeInviteTime:          0,
		isInvitedToBGInstanceGUID: 0,
		arenaTeamRating:           arenaRating,
		arenaMatchmakerRating:     matchmakerRating,
		opponentsTeamRating:       0,
		opponentsMatchmakerRating: 0,
		previousOpponentsTeamID:   previousOpponentsTeamID,
		players:                   make(map[uint64]*bgQueuePlayerInfo, len(memberGUIDs)),
	}
	for _, guid := range memberGUIDs {
		plInfo := &bgQueuePlayerInfo{lastOnlineTime: now, groupInfo: ginfo}
		ginfo.players[guid] = plInfo
	}
	return ginfo
}

// removeQueuedPlayer ports the per-player erase arm of BattlegroundQueue::RemovePlayer
// (BattlegroundQueue.cpp:347-349): the player's record leaves the group's player map.
// The surrounding container arms — the bracket search (:310-331), the m_QueuedPlayers
// erase (:357), and the empty-group delete (:380-384) — need the server-level queue
// containers and stay documented-unported in the file header.
func (g *bgGroupQueueInfo) removeQueuedPlayer(guid uint64) {
	delete(g.players, guid)
}

// bgSelectionPool is the Go form of BattlegroundQueue::SelectionPool
// (BattlegroundQueue.h:113-126): the per-side group list the matchmaker builds when
// filling a battleground, with the running head-count of selected players.
//
// MATCHMAKER (BattlegroundQueue.cpp:500-1000): the pure selection arms below port
// FillPlayersToBG, CheckPremadeMatch, CheckNormalMatch, CheckSkirmishForSameFaction
// and the rated-arena team selection in BattlegroundQueueUpdate as free functions
// over caller-held group slices (queue order). They have no live caller yet — the
// per-session sim (battleground.go handleBattlemasterJoin) never cross-matches
// players — and the BattlegroundQueueUpdate orchestration itself stays unported:
// it needs live BG instances (sBattlegroundMgr free-slot queue, CreateNewBattleground,
// StartBattleground), as does InviteGroupToBG (cpp:432-498 — bg instance ids,
// invited counters, arena-team ids, per-player invite events). BattlegroundQueue::UpdateEvents
// (cpp:764-767, m_events.Update) is covered on the Go side by the per-session timers
// in simulateBGQueueInvite (battleground.go), same as BGQueueInviteEvent/BGQueueRemoveEvent.
// GetPlayersInQueue (cpp:427-430) is a SelectionPool counter read and adds nothing.
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
