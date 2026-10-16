package world

// Battleground queue matchmaker, ported from src/server/game/Battlegrounds/BattlegroundQueue.cpp.
// The pure selection arms — FillPlayersToBG, CheckPremadeMatch, CheckNormalMatch,
// CheckSkirmishForSameFaction and the rated-arena team selection in
// BattlegroundQueueUpdate — run over caller-held group slices in queue order and
// return the per-side selection pools; the server-level containers (m_QueuedGroups,
// m_QueuedPlayers) and live BG instances stay unported by design (see
// battleground_queue.go header). Team pool index 0 is alliance, 1 is horde,
// mirroring TeamId (TEAM_ALLIANCE=0, TEAM_HORDE=1) in m_SelectionPools.

// bgAbsInt32 is |a| on the signed free-slot diffs in FillPlayersToBG (cpp:568).
func bgAbsInt32(a int32) int32 {
	if a < 0 {
		return -a
	}
	return a
}

// fillPlayersToBG ports BattlegroundQueue::FillPlayersToBG (cpp:500-603): picks
// groups from the normal queues (queue order) to fill the free slots of a live
// battleground. aliFree/hordeFree are bg->GetFreeSlotsForTeam; invitationType is
// the CONFIG_BATTLEGROUND_INVITATION_TYPE value (bgQueueInvitationNoBalance/
// Balanced/Even). The even-invitation arm (cpp:508-532) trims free slots so both
// sides can invite equally; the subset-sum rebalance loop (cpp:566-601) kicks and
// re-adds groups until the sides differ by at most one player.
func fillPlayersToBG(aliFree, hordeFree int32, aliGroups, hordeGroups []*bgGroupQueueInfo, invitationType uint8) (aliPool, hordePool *bgSelectionPool) {
	aliPool = &bgSelectionPool{}
	hordePool = &bgSelectionPool{}
	aliCount := uint32(len(aliGroups))
	hordeCount := uint32(len(hordeGroups))

	// try to get even teams
	if invitationType == bgQueueInvitationEven {
		// check if the teams are even
		if hordeFree == 1 && aliFree == 1 {
			// if we are here, the teams have the same amount of players
			// then we have to allow to join the same amount of players
			hordeExtra := int32(hordeCount) - int32(aliCount)
			if hordeExtra < 0 {
				hordeExtra = 0
			}
			aliExtra := int32(aliCount) - int32(hordeCount)
			if aliExtra < 0 {
				aliExtra = 0
			}
			if aliCount != hordeCount {
				aliFree -= aliExtra
				hordeFree -= hordeExtra
				if aliFree < 0 {
					aliFree = 0
				}
				if hordeFree < 0 {
					hordeFree = 0
				}
			}
		}
	}

	pools := []*bgSelectionPool{aliPool, hordePool}
	queues := [][]*bgGroupQueueInfo{aliGroups, hordeGroups}
	frees := []int32{aliFree, hordeFree}
	var idx [2]uint32
	for t := 0; t < 2; t++ {
		for ; idx[t] < uint32(len(queues[t])) && pools[t].addGroup(queues[t][idx[t]], uint32(frees[t])); idx[t]++ {
		}
	}

	// if ofc like BG queue invitation is set in config, then we are happy
	if invitationType == bgQueueInvitationNoBalance {
		return aliPool, hordePool
	}

	// At first we need to compare free space in bg and our selection pool
	diffAli := aliFree - int32(aliPool.playerCount())
	diffHorde := hordeFree - int32(hordePool.playerCount())
	for bgAbsInt32(diffAli-diffHorde) > 1 && (hordePool.playerCount() > 0 || aliPool.playerCount() > 0) {
		// each cycle execution we need to kick at least 1 group
		if diffAli < diffHorde {
			// kick alliance group, add to pool new group if needed
			if aliPool.kickGroup(uint32(diffHorde - diffAli)) {
				var desired uint32
				if aliFree >= diffHorde {
					desired = uint32(aliFree - diffHorde)
				}
				for ; idx[0] < aliCount && aliPool.addGroup(aliGroups[idx[0]], desired); idx[0]++ {
				}
			}
			// if ali selection is already empty, then kick horde group, but if there are less horde than ali in bg - break;
			if aliPool.playerCount() == 0 {
				if aliFree <= diffHorde+1 {
					break
				}
				hordePool.kickGroup(uint32(diffHorde - diffAli))
			}
		} else {
			// kick horde group, add to pool new group if needed
			if hordePool.kickGroup(uint32(diffAli - diffHorde)) {
				var desired uint32
				if hordeFree >= diffAli {
					desired = uint32(hordeFree - diffAli)
				}
				for ; idx[1] < hordeCount && hordePool.addGroup(hordeGroups[idx[1]], desired); idx[1]++ {
				}
			}
			if hordePool.playerCount() == 0 {
				if hordeFree <= diffAli+1 {
					break
				}
				aliPool.kickGroup(uint32(diffAli - diffHorde))
			}
		}
		// count diffs after small update
		diffAli = aliFree - int32(aliPool.playerCount())
		diffHorde = hordeFree - int32(hordePool.playerCount())
	}
	return aliPool, hordePool
}

// checkNormalMatch ports BattlegroundQueue::CheckNormalMatch (cpp:663-704): builds
// per-side pools up to maxPlayers, then (unless invitationType is NoBalance) tops
// up the smaller side toward the larger one and rejects the match when the sides
// differ by more than 2. testing+isBattleground is the sBattlegroundMgr->isTesting
// debug arm that allows a 1v0 start.
func checkNormalMatch(aliGroups, hordeGroups []*bgGroupQueueInfo, minPlayers, maxPlayers uint32, invitationType uint8, testing, isBattleground bool) (aliPool, hordePool *bgSelectionPool, ok bool) {
	aliPool = &bgSelectionPool{}
	hordePool = &bgSelectionPool{}
	pools := []*bgSelectionPool{aliPool, hordePool}
	queues := [][]*bgGroupQueueInfo{aliGroups, hordeGroups}
	var idx [2]int
	for i := 0; i < 2; i++ {
		for ; idx[i] < len(queues[i]); idx[i]++ {
			if g := queues[i][idx[i]]; g.isInvitedToBGInstanceGUID == 0 {
				pools[i].addGroup(g, maxPlayers)
				if pools[i].playerCount() >= minPlayers {
					break
				}
			}
		}
	}
	// try to invite same number of players - this cycle may cause longer wait time even if there are enough players in queue, but we want ballanced bg
	j := 0
	if hordePool.playerCount() < aliPool.playerCount() {
		j = 1
	}
	if invitationType != bgQueueInvitationNoBalance &&
		hordePool.playerCount() >= minPlayers && aliPool.playerCount() >= minPlayers {
		// we will try to invite more groups to team with less players indexed by j
		idx[j]++ // this will not cause a crash, because for cycle above reached break
		for ; idx[j] < len(queues[j]); idx[j]++ {
			if g := queues[j][idx[j]]; g.isInvitedToBGInstanceGUID == 0 {
				if !pools[j].addGroup(g, pools[(j+1)%2].playerCount()) {
					break
				}
			}
		}
		// do not allow to start bg with more than 2 players more on 1 faction
		if bgAbsDiff(hordePool.playerCount(), aliPool.playerCount()) > 2 {
			return aliPool, hordePool, false
		}
	}
	// allow 1v0 if debug bg
	if testing && isBattleground && (aliPool.playerCount() > 0 || hordePool.playerCount() > 0) {
		return aliPool, hordePool, true
	}
	// return true if there are enough players in selection pools - enable to work .debug bg command correctly
	return aliPool, hordePool, aliPool.playerCount() >= minPlayers && hordePool.playerCount() >= minPlayers
}

// checkSkirmishForSameFaction ports BattlegroundQueue::CheckSkirmishForSameFaction
// (cpp:706-762): when one side's pool from checkNormalMatch already holds
// minPlayersPerTeam, drafts the other side from the same faction's normal queue
// (queue order after the last selected group) and moves the drafted groups to the
// other faction's queue, re-tagging their team. aliGroups/hordeGroups are not
// mutated; the updated queue orders come back as aliOut/hordeOut. The C++ reads
// .back() on the team's pool; that read is guarded here (C++ would be UB on an
// empty pool) — in practice the pool holds at least minPlayersPerTeam >= 1.
func checkSkirmishForSameFaction(aliPool, hordePool *bgSelectionPool, aliGroups, hordeGroups []*bgGroupQueueInfo, minPlayersPerTeam uint32) (ok bool, aliOut, hordeOut []*bgGroupQueueInfo) {
	if aliPool.playerCount() < minPlayersPerTeam && hordePool.playerCount() < minPlayersPerTeam {
		return false, aliGroups, hordeGroups
	}
	teamIndex, otherTeam := 0, 1
	var otherTeamID uint32 = teamHorde
	if hordePool.playerCount() == minPlayersPerTeam {
		teamIndex, otherTeam = 1, 0
		otherTeamID = teamAlliance
	}
	pools := []*bgSelectionPool{aliPool, hordePool}
	queues := [][]*bgGroupQueueInfo{aliGroups, hordeGroups}
	// clear other team's selection
	pools[otherTeam].init()
	// store last ginfo pointer
	if len(pools[teamIndex].selectedGroups) == 0 {
		return false, aliGroups, hordeGroups
	}
	ginfo := pools[teamIndex].selectedGroups[len(pools[teamIndex].selectedGroups)-1]
	// set itr_team to group that was added to selection pool latest
	pos := -1
	for i, g := range queues[teamIndex] {
		if g == ginfo {
			pos = i
			break
		}
	}
	if pos == -1 {
		return false, aliGroups, hordeGroups
	}
	// invite players to other selection pool
	for i := pos + 1; i < len(queues[teamIndex]); i++ {
		if g := queues[teamIndex][i]; g.isInvitedToBGInstanceGUID == 0 && !pools[otherTeam].addGroup(g, minPlayersPerTeam) {
			break
		}
	}
	if pools[otherTeam].playerCount() != minPlayersPerTeam {
		return false, aliGroups, hordeGroups
	}
	// here we have correct 2 selections and we need to change one teams team and move selection pool teams to other team's queue
	aliOut = append([]*bgGroupQueueInfo(nil), aliGroups...)
	hordeOut = append([]*bgGroupQueueInfo(nil), hordeGroups...)
	out := [][]*bgGroupQueueInfo{aliOut, hordeOut}
	for _, g := range pools[otherTeam].selectedGroups {
		// set correct team
		g.team = otherTeamID
		// add team to other queue
		out[otherTeam] = append([]*bgGroupQueueInfo{g}, out[otherTeam]...)
		// remove team from old queue
		for i := pos + 1; i < len(out[teamIndex]); i++ {
			if out[teamIndex][i] == g {
				out[teamIndex] = append(out[teamIndex][:i], out[teamIndex][i+1:]...)
				break
			}
		}
	}
	return true, out[0], out[1]
}

// checkPremadeMatch ports the pool-selection arm of BattlegroundQueue::CheckPremadeMatch
// (cpp:605-642): when both premade queues hold an uninvited group, both join the
// pools up to maxPlayersPerTeam and the normal queues top each side up to the
// smaller side's count. The demotion arm (cpp:644-658) lives in premadeShouldDemote.
func checkPremadeMatch(aliPremade, hordePremade, aliNormal, hordeNormal []*bgGroupQueueInfo, maxPlayersPerTeam uint32) (aliPool, hordePool *bgSelectionPool, ok bool) {
	aliPool = &bgSelectionPool{}
	hordePool = &bgSelectionPool{}
	// check match
	if len(aliPremade) > 0 && len(hordePremade) > 0 {
		// start premade match — if groups aren't invited
		var aliG, hordeG *bgGroupQueueInfo
		for _, g := range aliPremade {
			if g.isInvitedToBGInstanceGUID == 0 {
				aliG = g
				break
			}
		}
		for _, g := range hordePremade {
			if g.isInvitedToBGInstanceGUID == 0 {
				hordeG = g
				break
			}
		}
		if aliG != nil && hordeG != nil {
			aliPool.addGroup(aliG, maxPlayersPerTeam)
			hordePool.addGroup(hordeG, maxPlayersPerTeam)
			// add groups/players from normal queue to size of bigger group
			maxPlayers := aliPool.playerCount()
			if hordePool.playerCount() < maxPlayers {
				maxPlayers = hordePool.playerCount()
			}
			normals := [][]*bgGroupQueueInfo{aliNormal, hordeNormal}
			pools := []*bgSelectionPool{aliPool, hordePool}
			for i := 0; i < 2; i++ {
				for _, g := range normals[i] {
					// if itr can join BG and player count is less that maxPlayers, then add group to selectionpool
					if g.isInvitedToBGInstanceGUID == 0 && !pools[i].addGroup(g, maxPlayers) {
						break
					}
				}
			}
			// premade selection pools are set
			return aliPool, hordePool, true
		}
	}
	// selection pools are not set
	return aliPool, hordePool, false
}

// premadeShouldDemote ports the premade→normal demotion rule in CheckPremadeMatch
// (cpp:644-658): the front premade group moves to the normal queue when its wait
// exceeds premadeWaitMS (CONFIG_BATTLEGROUND_PREMADE_GROUP_WAIT_FOR_MATCH, 30min
// default) or it shrank below minPlayersPerTeam; invited groups never move. now
// and joinTime are game-time milliseconds.
func premadeShouldDemote(front *bgGroupQueueInfo, now, premadeWaitMS, minPlayersPerTeam uint32) bool {
	if front.isInvitedToBGInstanceGUID != 0 {
		return false
	}
	return front.joinTime < now-premadeWaitMS || uint32(len(front.players)) < minPlayersPerTeam
}

// bgArenaQueuePos locates a team inside the rated-arena premade queues:
// queue 0 = alliance premade, 1 = horde premade.
type bgArenaQueuePos struct {
	queue int
	index int
}

// selectRatedArenaMatch ports the rated-arena team selection in
// BattlegroundQueue::BattlegroundQueueUpdate (cpp:895-985): picks two opposing
// premade teams for a new rated arena. arenaRating 0 selects from the longest-
// waiting front team (cpp:895-910); teams qualify on matchmaker rating inside
// [arenaRating-maxRatingDifference, arenaRating+maxRatingDifference] or on having
// waited past ratingDiscardTimer (cpp:912-921, discardTime signed per the C++
// comment). The opponent must differ in arena team id and must not be the first
// team's previous opponent unless past prevOpponentsDiscardTimer (cpp:944-957).
// On match it wires the cross opponent ratings (cpp:964-968) and moves a team
// whose faction tag disagrees with its queue side to the other queue's front
// (cpp:971-982, because Queue::RemovePlayer would otherwise spam errors).
// now/joinTime are game-time milliseconds; aliOut/hordeOut are the updated queue
// orders (inputs are not mutated).
func selectRatedArenaMatch(aliPremade, hordePremade []*bgGroupQueueInfo, arenaRating uint32, now, maxRatingDifference, ratingDiscardTimer, prevOpponentsDiscardTimer uint32) (aTeam, hTeam *bgGroupQueueInfo, rating uint32, aliOut, hordeOut []*bgGroupQueueInfo, ok bool) {
	premade := [][]*bgGroupQueueInfo{aliPremade, hordePremade}
	rating = arenaRating
	if rating == 0 {
		var front1, front2 *bgGroupQueueInfo
		if len(premade[0]) > 0 {
			front1 = premade[0][0]
			rating = front1.arenaMatchmakerRating
		}
		if len(premade[1]) > 0 {
			front2 = premade[1][0]
			rating = front2.arenaMatchmakerRating
		}
		if front1 != nil && front2 != nil {
			if front1.joinTime < front2.joinTime {
				rating = front1.arenaMatchmakerRating
			}
		} else if front1 == nil && front2 == nil {
			return nil, nil, rating, aliPremade, hordePremade, false // queues are empty
		}
	}

	// set rating range
	var arenaMinRating uint32
	if rating > maxRatingDifference {
		arenaMinRating = rating - maxRatingDifference
	}
	arenaMaxRating := rating + maxRatingDifference
	// this has to be signed value - when the server starts, this value would be negative and thus overflow
	discardTime := int32(now - ratingDiscardTimer)
	// timer for previous opponents
	discardOpponentsTime := int32(now - prevOpponentsDiscardTimer)

	inWindow := func(g *bgGroupQueueInfo) bool {
		return g.arenaMatchmakerRating >= arenaMinRating && g.arenaMatchmakerRating <= arenaMaxRating
	}
	// we need to find 2 teams which will play next game
	var teams [2]bgArenaQueuePos
	found := 0
	team := 0
	for i := 0; i < 2; i++ {
		// take the group that joined first
		for j, g := range premade[i] {
			// if group match conditions, then add it to pool
			if g.isInvitedToBGInstanceGUID == 0 && (inWindow(g) || int32(g.joinTime) < discardTime) {
				teams[found] = bgArenaQueuePos{queue: i, index: j}
				found++
				team = i
				break
			}
		}
	}
	if found == 0 {
		return nil, nil, rating, aliPremade, hordePremade, false
	}
	if found == 1 {
		first := premade[teams[0].queue][teams[0].index]
		for j := teams[0].index; j < len(premade[team]); j++ {
			if g := premade[team][j]; g.isInvitedToBGInstanceGUID == 0 &&
				(inWindow(g) || int32(g.joinTime) < discardTime) &&
				(first.arenaTeamID != g.previousOpponentsTeamID || int32(g.joinTime) < discardOpponentsTime) &&
				first.arenaTeamID != g.arenaTeamID {
				teams[1] = bgArenaQueuePos{queue: team, index: j}
				found++
				break
			}
		}
	}
	// if we have 2 teams, then start new arena and invite players!
	if found != 2 {
		return nil, nil, rating, aliPremade, hordePremade, false
	}
	aTeam = premade[teams[0].queue][teams[0].index]
	hTeam = premade[teams[1].queue][teams[1].index]
	aTeam.opponentsTeamRating = hTeam.arenaTeamRating
	hTeam.opponentsTeamRating = aTeam.arenaTeamRating
	aTeam.opponentsMatchmakerRating = hTeam.arenaMatchmakerRating
	hTeam.opponentsMatchmakerRating = aTeam.arenaMatchmakerRating

	// now we must move team if we changed its faction to another faction queue,
	// because then we will spam log by errors in Queue::RemovePlayer
	aliOut = append([]*bgGroupQueueInfo(nil), aliPremade...)
	hordeOut = append([]*bgGroupQueueInfo(nil), hordePremade...)
	out := [][]*bgGroupQueueInfo{aliOut, hordeOut}
	if aTeam.team != teamAlliance {
		out[0] = append([]*bgGroupQueueInfo{aTeam}, out[0]...)
		out[1] = append(out[1][:teams[0].index], out[1][teams[0].index+1:]...)
	}
	if hTeam.team != teamHorde {
		out[1] = append([]*bgGroupQueueInfo{hTeam}, out[1]...)
		out[0] = append(out[0][:teams[1].index], out[0][teams[1].index+1:]...)
	}
	return aTeam, hTeam, rating, out[0], out[1], true
}

// bgQueueIsPlayerInvited ports BattlegroundQueue::IsPlayerInvited (cpp:410-416)
// over the caller's queued-players map: true when the player is queued and
// invited to bgInstanceGUID with a matching remove-invite timestamp.
func bgQueueIsPlayerInvited(queued map[uint64]*bgQueuePlayerInfo, guid uint64, bgInstanceGUID, removeTime uint32) bool {
	plInfo, ok := queued[guid]
	return ok && plInfo.groupInfo.isInvitedToBGInstanceGUID == bgInstanceGUID && plInfo.groupInfo.removeInviteTime == removeTime
}

// bgQueueGroupInfoCopy ports BattlegroundQueue::GetPlayerGroupInfoData (cpp:418-425):
// returns a copy of the queued player's group record, like the C++ struct copy.
func bgQueueGroupInfoCopy(queued map[uint64]*bgQueuePlayerInfo, guid uint64) (bgGroupQueueInfo, bool) {
	plInfo, ok := queued[guid]
	if !ok {
		return bgGroupQueueInfo{}, false
	}
	return *plInfo.groupInfo, true
}
