package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Battleground per-team raid groups.
// Reference: Battleground::AddOrSetPlayerToCorrectBgGroup
// (Battlegrounds/Battleground.cpp:1033-1059), Battleground::SetBgRaid
// (Battleground.cpp:1767-1775), Player::SetBattlegroundOrBattlefieldRaid /
// Player::RemoveFromBattlegroundOrBattlefieldRaid (Player.cpp:24486-24505),
// Group::Create's BG arms (Group.cpp:160-161, 175-176, 185) and the
// Group::RemoveMember BG arms (Group.cpp:630-639, 736-739).

// bgRaidTeamIndex mirrors the GetBgRaid(team) slot select
// (Battleground.h:397): ALLIANCE -> 0, everything else -> 1. The Go port
// arm in handleBattlefieldPort stores teamForRace (0 alliance / 1 horde),
// so the team value indexes the slots directly.
func bgRaidTeamIndex(team uint16) uint8 {
	if team == 1 {
		return 1
	}
	return 0
}

// setBGGroupStash mirrors Player::SetBattlegroundOrBattlefieldRaid
// (Player.cpp:24486-24494): the player's current group becomes the
// original group, then the player links the battleground raid. The raid
// subgroup lives on the member entry itself (set at formation time).
// cur is the player's current group (nil when group-less); callers pass
// it from under the groups lock.
func (s *session) setBGGroupStash(g, cur *groupState) {
	if s == nil || g == nil {
		return
	}
	if s.groupID != g.ID {
		s.originalGroupID = s.groupID
		if cur != nil {
			if sg, ok := cur.memberSubGroup(s.playerGUID); ok {
				s.originalSubGroup = sg
			}
		}
		s.groupID = g.ID
	}
}

// addOrSetPlayerToCorrectBGGroup mirrors
// Battleground::AddOrSetPlayerToCorrectBgGroup (Battleground.cpp:1033-1059).
// The first player of a team creates the team's BG raid; later joiners are
// added to it. SetBgRaid precedes the group creation exactly like the C++
// (Battleground.cpp:1040-1041), so the Create-time BG arms see isBGGroup()
// true: GroupType becomes GROUPTYPE_BGRAID (Group.cpp:160-161) and the
// character-group DB persist is skipped (Group.cpp:185) — Go's DBID stays
// 0. Loot stays GROUP_LOOT via Create's !isLFGGroup arm (Group.cpp:175).
// PlayerAddedToBGCheckIfBGIsRunning (Battleground.cpp:1720) has no Go model:
// there is no live BG status machine, so the STATUS_WAIT_LEAVE PvP-log arm
// reads the per-map Winner state instead (bgEndedWinner, battleground_scores.go).
func (s *Server) addOrSetPlayerToCorrectBGGroup(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	instanceID := sess.bgData.InstanceID
	if instanceID == 0 {
		return
	}
	teamIdx := bgRaidTeamIndex(sess.bgData.Team)

	s.groupsMu.Lock()
	if s.bgRaids == nil {
		s.bgRaids = make(map[uint32][2]uint64)
	}
	raids := s.bgRaids[instanceID]
	g := s.groups[raids[teamIdx]]

	if g == nil {
		// First player of this team: new Group, SetBgRaid, Create.
		g = &groupState{
			ID:             newGroupID(),
			LeaderGUID:     sess.playerGUID,
			Members:        []groupMember{{GUID: sess.playerGUID, Name: sess.player.Name, SubGroup: 0}},
			LootMethod:     3,
			LootThreshold:  2,
			LooterGUID:     sess.playerGUID,
			DungeonDiff:    1,
			RaidDiff:       1,
			GroupType:      groupTypeBattlegroundRaid,
			IsRaid:         true,
			bgRaidInstance: instanceID,
			bgRaidTeam:     teamIdx,
		}
		s.groups[g.ID] = g
		raids[teamIdx] = g.ID
		s.bgRaids[instanceID] = raids
		sess.setBGGroupStash(g, s.groups[sess.groupID])
		s.groupsMu.Unlock()
		s.broadcastGroupList(g)
		return
	}

	// Raid already exists.
	memberIdx := -1
	for i, m := range g.Members {
		if m.GUID == sess.playerGUID {
			memberIdx = i
			break
		}
	}
	if memberIdx >= 0 {
		// Already a member: re-link in the recorded subgroup, like the
		// IsMember arm (Battleground.cpp:1044-1048).
		sess.setBGGroupStash(g, s.groups[sess.groupID])
		s.groupsMu.Unlock()
		s.broadcastGroupList(g)
		return
	}

	// Group::AddMember (Group.cpp:404-442): raid joiners land in the first
	// subgroup with a free slot; a joiner already in a group goes through
	// SetBattlegroundOrBattlefieldRaid (the isBGGroup arm at Group.cpp:442),
	// which stashes the current group as the original group.
	subGroup := uint8(0)
	for i := uint8(0); i < 8; i++ {
		if g.countInSubGroup(i) < maxGroupSize {
			subGroup = i
			break
		}
	}
	originalGroupID := sess.groupID
	curGroup := s.groups[originalGroupID]
	g.Members = append(g.Members, groupMember{GUID: sess.playerGUID, Name: sess.player.Name, SubGroup: subGroup})
	refreshGroupMaxEnchantingLevel(s, g)
	sess.setBGGroupStash(g, curGroup)
	// Battleground.cpp:1052-1056: a joiner who leads their original group
	// becomes the BG raid leader. setGroupLeader is the in-tree
	// Group::ChangeLeader arm (Group.cpp:806-809 flags + SendUpdate).
	makeLeader := false
	if originalGroupID != 0 {
		if og := s.groups[originalGroupID]; og != nil && og.isLeader(sess.playerGUID) {
			makeLeader = true
		}
	}
	s.groupsMu.Unlock()
	// Group::AddMember ends with SendUpdate (Group.cpp:478).
	if makeLeader {
		s.setGroupLeader(g, sess.playerGUID)
	} else {
		s.broadcastGroupList(g)
	}
}

// sendPostLeaveGroupList refreshes the session's group UI after a
// battleground-raid removal, the Go form of the SMSG_GROUP_LIST
// Group::RemoveMember sends the removed player (Group.cpp:659-663).
func (s *session) sendPostLeaveGroupList() {
	if s == nil || s.server == nil {
		return
	}
	var pkt []byte
	if g := s.server.findGroupByID(s.groupID); g != nil {
		pkt = buildGroupList(s.server, g, s.playerGUID, 0)
	} else {
		pkt = buildGroupList(s.server, &groupState{ID: 0, LeaderGUID: s.playerGUID}, s.playerGUID, 0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_GROUP_LIST), pkt, true)
}

// removePlayerFromBGRaid mirrors the Battleground::RemovePlayer raid arm
// (Battleground.cpp:894-900): drop the player from the team's BG raid; a
// raid reduced to zero members disbands and its slot is cleared
// (SetBgRaid(team, nullptr) via Group::~Group, Group.cpp:73-82).
// Group::RemoveMember's BG arms apply: the 1-member disband threshold
// (Group.cpp:630, 736-739) and the RemoveFromBattlegroundOrBattlefieldRaid
// original-group restore (Group.cpp:635).
func (s *Server) removePlayerFromBGRaid(sess *session) {
	if s == nil || sess == nil || sess.player == nil {
		return
	}
	instanceID := sess.bgData.InstanceID
	if instanceID == 0 {
		return
	}
	teamIdx := bgRaidTeamIndex(sess.bgData.Team)

	s.groupsMu.Lock()
	raids := s.bgRaids[instanceID]
	g := s.groups[raids[teamIdx]]
	if g == nil || !g.isBGGroup() {
		s.groupsMu.Unlock()
		return
	}
	idx := -1
	for i, m := range g.Members {
		if m.GUID == sess.playerGUID {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.groupsMu.Unlock()
		return
	}
	wasLeader := g.LeaderGUID == sess.playerGUID
	g.Members = append(g.Members[:idx], g.Members[idx+1:]...)
	refreshGroupMaxEnchantingLevel(s, g)
	// Player::RemoveFromBattlegroundOrBattlefieldRaid
	// (Player.cpp:24496-24505): unlink the raid, re-link the original group
	// when one was stashed.
	if sess.groupID == g.ID {
		sess.groupID = sess.originalGroupID
		sess.originalGroupID = 0
		sess.originalSubGroup = 0
	}
	disbanded := len(g.Members) == 0
	if disbanded {
		delete(s.groups, g.ID)
		raids[teamIdx] = 0
		if raids[0] == 0 && raids[1] == 0 {
			delete(s.bgRaids, instanceID)
		} else {
			s.bgRaids[instanceID] = raids
		}
	}
	var leaderCandidate uint64
	if !disbanded && wasLeader {
		// Group::RemoveMember (Group.cpp:712-720): first connected member
		// becomes leader via ChangeLeader; the full arm runs after unlock.
		for _, m := range g.Members {
			if cs := s.findSessionByGUID(m.GUID); cs != nil && cs.player != nil {
				leaderCandidate = m.GUID
				break
			}
		}
	}
	s.groupsMu.Unlock()
	if disbanded {
		return
	}
	if leaderCandidate != 0 {
		s.setGroupLeader(g, leaderCandidate)
	} else {
		s.broadcastGroupList(g)
	}
	// Group::RemoveMember (Group.cpp:659-663) sends the removed player a
	// GROUP_LIST: refresh the leaver's UI with their post-leave group
	// (the restored original group, or the empty form when group-less).
	sess.sendPostLeaveGroupList()
}
