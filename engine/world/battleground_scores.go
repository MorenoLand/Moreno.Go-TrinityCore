package world

// bgScoreKey identifies one battleground instance's score table, mirroring
// Battleground::PlayerScores (Battleground.h), which lives on the BG instance
// rather than on the map: the killer's bgData.InstanceID is the instance key
// (battleground.go:1552 assigns it from the queue entry, :2144 clears it on
// leave), and the victim's map disambiguates reused instance ids across BG
// types. Arena matches keep their own ArenaPlayerScore table
// (battleground_arena.go), so this store covers non-arena battlegrounds only.
type bgScoreKey struct {
	MapID      uint32
	InstanceID uint32
}

// bgPlayerScore mirrors the base BattlegroundScore fields that
// Battleground::HandleKillPlayer touches (Battleground.cpp:1662-1694):
// SCORE_DEATHS, SCORE_HONORABLE_KILLS, SCORE_KILLING_BLOWS. BonusHonor,
// DamageDone, and HealingDone belong to other UpdatePlayerScore callers and
// are not modeled here yet.
type bgPlayerScore struct {
	KillingBlows   uint32
	Deaths         uint32
	HonorableKills uint32
}

// recordBGKillPlayerScores ports the base leg of
// Battleground::HandleKillPlayer (Battleground.cpp:1662-1694) for non-arena
// battlegrounds. The victim always takes +1 deaths, even on suicide; a
// non-suicide killer takes +1 honorable kills and +1 killing blows, and every
// other same-team BG participant at group-reward distance from the victim
// takes +1 honorable kills (C++ iterates m_Players, not the killer's group:
// creditedPlayer == killer is the only skip). IsAtGroupRewardDistance
// (Player.cpp:24160-24170) needs the same map and, for non-dungeon maps,
// raw position distance within CONFIG_GROUP_XP_DISTANCE; battleground maps
// are not dungeons (Map.dbc instance type battleground), so no dungeon arm
// applies. C++ UpdatePlayerScore no-ops for players absent from PlayerScores;
// Go creates the row on demand, the per-map getOrCreate convention.
func (s *Server) recordBGKillPlayerScores(victim, killer *session) {
	if s == nil || victim == nil || victim.player == nil {
		return
	}
	instanceID := victim.bgData.InstanceID
	if killer != nil {
		instanceID = killer.bgData.InstanceID
	}
	key := bgScoreKey{MapID: victim.player.Map, InstanceID: instanceID}

	s.sessionsMu.RLock()
	members := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil {
			members = append(members, sess)
		}
	}
	s.sessionsMu.RUnlock()

	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	if s.bgScores == nil {
		s.bgScores = make(map[bgScoreKey]map[uint64]*bgPlayerScore)
	}
	table := s.bgScores[key]
	if table == nil {
		table = make(map[uint64]*bgPlayerScore)
		s.bgScores[key] = table
	}
	score := table[victim.playerGUID]
	if score == nil {
		score = &bgPlayerScore{}
		table[victim.playerGUID] = score
	}
	score.Deaths++

	if killer == nil || killer.player == nil || killer == victim {
		return
	}
	killerScore := table[killer.playerGUID]
	if killerScore == nil {
		killerScore = &bgPlayerScore{}
		table[killer.playerGUID] = killerScore
	}
	killerScore.HonorableKills++
	killerScore.KillingBlows++

	killerTeam := teamForRace(killer.player.Race)
	for _, member := range members {
		if member == killer || member.player == nil || member.bgData.InstanceID != instanceID || member.player.Map != victim.player.Map {
			continue
		}
		if teamForRace(member.player.Race) != killerTeam {
			continue
		}
		if distance3D(member.player.X, member.player.Y, member.player.Z, victim.player.X, victim.player.Y, victim.player.Z) > s.Config.MaxGroupXPDistance {
			continue
		}
		memberScore := table[member.playerGUID]
		if memberScore == nil {
			memberScore = &bgPlayerScore{}
			table[member.playerGUID] = memberScore
		}
		memberScore.HonorableKills++
	}
}

// bgKillPlayerScores snapshots one instance's score table for a future
// consumer (the non-arena MSG_PVP_LOG_DATA builder, which currently reports
// no packet for lack of a score model — guild.go handlePvpLogData). The copy
// keeps callers off the live rows.
func (s *Server) bgKillPlayerScores(mapID, instanceID uint32) map[uint64]bgPlayerScore {
	if s == nil {
		return nil
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	table := s.bgScores[bgScoreKey{MapID: mapID, InstanceID: instanceID}]
	if len(table) == 0 {
		return nil
	}
	out := make(map[uint64]bgPlayerScore, len(table))
	for guid, score := range table {
		if score != nil {
			out[guid] = *score
		}
	}
	return out
}
