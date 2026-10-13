package world

// getsRecruitAFriendBonus mirrors Player::GetsRecruitAFriendBonus
// (Player.cpp:24086-24124): the player earns the recruit-a-friend bonus when
// a group member within MaxRecruitAFriendBonusDistance is linked to them in
// either direction (member recruited them, or they recruited the member).
// With forXP the member must also be at most RecruitAFriendMaxLevel and, if
// below the player's level, within RecruitAFriendMaxDifference levels; the
// whole scan only runs when the player's own level qualifies (forXP) or
// unconditionally for the reputation path (!forXP).
func (s *session) getsRecruitAFriendBonus(forXP bool) bool {
	if s == nil || s.server == nil || s.player == nil {
		return false
	}
	maxLevel := s.server.Config.RecruitAFriendMaxLevel
	if maxLevel == 0 {
		maxLevel = 60 // World.cpp:963 default when the config row is absent
	}
	if maxLevel > s.server.Config.MaxPlayerLevel {
		maxLevel = 60 // World.cpp:964-968: over-max falls back to 60
	}
	if forXP && uint32(s.player.Level) > maxLevel {
		return false
	}
	g := s.server.findGroupByID(s.groupID)
	if g == nil {
		return false
	}
	maxDiff := uint32(s.server.Config.RecruitAFriendMaxDifference)
	maxDist := s.server.Config.MaxRecruitAFriendBonusDistance
	for _, member := range g.Members {
		ms := s.server.findSessionByGUID(member.GUID)
		if ms == nil || ms.player == nil {
			continue
		}
		if !s.recruitAFriendInRange(ms, maxDist) {
			continue
		}
		if forXP {
			if uint32(ms.player.Level) > maxLevel {
				continue
			}
			if ms.player.Level < s.player.Level &&
				uint32(s.player.Level-ms.player.Level) > maxDiff {
				continue
			}
		}
		if ms.recruiterID == s.accountID || s.recruiterID == ms.accountID {
			return true
		}
	}
	return false
}

// recruitAFriendInRange mirrors Player::IsAtRecruitAFriendDistance
// (Player.cpp:24175-24183): same map instance (WorldObject::IsInMap), and
// the member within MaxRecruitAFriendBonusDistance of the member's corpse
// when dead, else of the member itself ("member (alive or dead) or his
// corpse at req. distance", Player.cpp:24100).
func (s *session) recruitAFriendInRange(ms *session, maxDist float64) bool {
	if s == nil || ms == nil || s.player == nil || ms.player == nil {
		return false
	}
	if s.player.Map != ms.player.Map || s.player.InstanceID != ms.player.InstanceID {
		return false
	}
	px, py, pz := ms.player.X, ms.player.Y, ms.player.Z
	if ms.isDeadOrGhost() && (ms.player.corpseX != 0 || ms.player.corpseY != 0 || ms.player.corpseZ != 0) {
		px, py, pz = ms.player.corpseX, ms.player.corpseY, ms.player.corpseZ
	}
	return distance3D(s.player.X, s.player.Y, s.player.Z, px, py, pz) <= maxDist
}
