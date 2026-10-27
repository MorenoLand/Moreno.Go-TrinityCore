package world

import (
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

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

// bgPlayerScore mirrors BattlegroundScore (BattlegroundScore.h): the six base
// fields every score row carries, plus the objectives fields of the six C++
// subclasses (BattlegroundWS/EY/AB/IC/AV/SA headers). The fields coexist
// union-style; only the instance's own subclass block is serialized, so a WS
// row never emits AV fields. BonusHonor, DamageDone, and HealingDone have no
// Go UpdatePlayerScore callers yet and serialize as zero; the objectives
// fields likewise await their capture/assault/defend update sites.
type bgPlayerScore struct {
	KillingBlows   uint32
	Deaths         uint32
	HonorableKills uint32
	BonusHonor     uint32
	DamageDone     uint32
	HealingDone    uint32

	FlagCaptures         uint32 // WS + EY
	FlagReturns          uint32 // WS
	BasesAssaulted       uint32 // AB + IC
	BasesDefended        uint32 // AB + IC
	GraveyardsAssaulted  uint32 // AV
	GraveyardsDefended   uint32 // AV
	TowersAssaulted      uint32 // AV
	TowersDefended       uint32 // AV
	MinesCaptured        uint32 // AV
	DemolishersDestroyed uint32 // SA
	GatesDestroyed       uint32 // SA (SCORE_DESTROYED_WALL)
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

// bgKillPlayerScores snapshots one instance's score table for the non-arena
// MSG_PVP_LOG_DATA builder (buildNonArenaPvPLogDataPacket). The copy keeps
// callers off the live rows.
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

// bgScoreTypeHasObjectives reports whether bgTypeID names one of the six
// C++ BattlegroundScore subclasses (BattlegroundTypeId: AV=1, WS=2, AB=3,
// EY=7, SA=9, IC=30). Any other type has no C++ objectives block to mirror.
func bgScoreTypeHasObjectives(bgTypeID uint32) bool {
	switch bgTypeID {
	case 1, 2, 3, 7, 9, 30:
		return true
	default:
		return false
	}
}

// buildNonArenaPvPLogDataPacket ports Battleground::BuildPvPLogDataPacket
// (Battleground.cpp:1196-1221) for the non-arena arm: type byte 0, the
// ended/winner block, the row count, then one BattlegroundScore::AppendToPacket
// row per score (Battleground.cpp:44-55: guid, killing blows, honorable kills,
// deaths, bonus honor, damage done, healing done, then the subclass
// objectives block). Rows sort by GUID ascending: C++ PlayerScores is
// std::map<uint32, BattlegroundScore*> (Battleground.h:339), keyed by GUID
// counter, and Go GUIDs share the counter in their low bits. The ended block
// always writes 0: Go has no live non-arena BG status machine (see the
// STATUS_WAIT_LEAVE note in battleground_group.go), and a handler-path
// request during STATUS_IN_PROGRESS also serializes 0 in C++; the
// WAIT_LEAVE winner arm stays unported. An unknown bgTypeID returns nil:
// C++ always has a concrete subclass block, so no faithful packet exists.
func (s *Server) buildNonArenaPvPLogDataPacket(mapID, instanceID, bgTypeID uint32) []byte {
	if !bgScoreTypeHasObjectives(bgTypeID) {
		return nil
	}
	table := s.bgKillPlayerScores(mapID, instanceID)
	guids := make([]uint64, 0, len(table))
	for guid := range table {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })

	buf := protocol.NewBuffer(2 + 4 + len(guids)*40)
	buf.WriteU8(0) // type: battleground
	buf.WriteU8(0) // bg not ended (no live status machine; see above)
	buf.WriteU32(uint32(len(guids)))
	for _, guid := range guids {
		score := table[guid]
		buf.WriteU64(guid)
		buf.WriteU32(score.KillingBlows)
		buf.WriteU32(score.HonorableKills)
		buf.WriteU32(score.Deaths)
		buf.WriteU32(score.BonusHonor)
		buf.WriteU32(score.DamageDone)
		buf.WriteU32(score.HealingDone)
		switch bgTypeID {
		case 1: // AV: BattlegroundAVScore::BuildObjectivesBlock, 5 objectives
			buf.WriteU32(5)
			buf.WriteU32(score.GraveyardsAssaulted)
			buf.WriteU32(score.GraveyardsDefended)
			buf.WriteU32(score.TowersAssaulted)
			buf.WriteU32(score.TowersDefended)
			buf.WriteU32(score.MinesCaptured)
		case 2: // WS: BattlegroundWGScore::BuildObjectivesBlock, 2 objectives
			buf.WriteU32(2)
			buf.WriteU32(score.FlagCaptures)
			buf.WriteU32(score.FlagReturns)
		case 3: // AB: BattlegroundABScore::BuildObjectivesBlock, 2 objectives
			buf.WriteU32(2)
			buf.WriteU32(score.BasesAssaulted)
			buf.WriteU32(score.BasesDefended)
		case 7: // EY: BattlegroundEYScore::BuildObjectivesBlock, 1 objective
			buf.WriteU32(1)
			buf.WriteU32(score.FlagCaptures)
		case 9: // SA: BattlegroundSAScore::BuildObjectivesBlock, 2 objectives
			buf.WriteU32(2)
			buf.WriteU32(score.DemolishersDestroyed)
			buf.WriteU32(score.GatesDestroyed)
		case 30: // IC: BattlegroundICScore::BuildObjectivesBlock, 2 objectives
			buf.WriteU32(2)
			buf.WriteU32(score.BasesAssaulted)
			buf.WriteU32(score.BasesDefended)
		}
	}
	return buf.Bytes()
}
