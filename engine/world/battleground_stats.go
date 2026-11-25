package world

import (
	"context"
	"time"
)

// recordBattlegroundStats mirrors the Battleground::EndBattleground statistics
// insert (Battleground.cpp:694-710, CHAR_INS_PVPSTATS_BATTLEGROUND): the
// battleground row (id, winner_faction, bracket_id, type, date) lands in
// pvpstats_battlegrounds when CONFIG_BATTLEGROUND_STORE_STATISTICS_ENABLE
// is on. winningTeam is Go's TeamId (Alliance 0, Horde 1, Neutral 2);
// winner_faction stores C++'s PvPTeamId (Horde 0, Alliance 1, Neutral 2),
// so the Alliance/Horde legs swap. The id is MAX(id)+1, as in C++
// (CHAR_SEL_PVPSTATS_MAXID), and is returned so the per-player rows
// (recordBattlegroundPlayerStats) share it, as in C++; 0 when the row did
// not land. Best-effort, like the other character-DB arms: a missing store
// or a failed write drops the row silently.
func (s *Server) recordBattlegroundStats(mapID, winningTeam uint32) uint64 {
	if s == nil || !s.Config.BattlegroundStoreStatisticsEnable {
		return 0
	}
	var winnerFaction uint32
	switch winningTeam {
	case 0:
		winnerFaction = 1
	case 1:
		winnerFaction = 0
	case 2:
		winnerFaction = 2
	default:
		return 0
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0
	}
	db := s.CharactersStore.DB
	ctx := context.Background()
	var maxID uint64
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM pvpstats_battlegrounds").Scan(&maxID)
	bgType, _, _, _ := battlegroundTypeForMap(mapID)
	bracket := s.bgBracketFor(mapID, 0)
	_, _ = db.ExecContext(ctx, "INSERT INTO pvpstats_battlegrounds (id, winner_faction, bracket_id, type, date) VALUES (?, ?, ?, ?, ?)", maxID+1, winnerFaction, bracket, bgType, time.Now().Format("2006-01-02 15:04:05"))
	return maxID + 1
}

// recordBattlegroundPlayerStats mirrors the per-player statistics insert of
// Battleground::EndBattleground (Battleground.cpp:749-766,
// CHAR_INS_PVPSTATS_PLAYER): one pvpstats_players row per roster member,
// sharing the battleground row's id. The winner bit is team == winner, as
// in C++; the attr columns are the subclass GetAttr1-5 fields
// (BattlegroundWS.h:191-192, BattlegroundAV.h:1602-1606,
// BattlegroundAB.h:292-293, BattlegroundEY.h:375, BattlegroundSA.h:544-545,
// BattlegroundIC.h:935-936), all present on bgPlayerScore. A roster member
// with no score row reads zeroed, matching C++ (AddPlayer always creates
// the row). Runs before the base honor arms, so the stored bonus honor
// excludes the end honor, as in C++. Best-effort: a zero id (statistics
// off or no store) or a failed write drops the rows silently.
func (s *Server) recordBattlegroundPlayerStats(mapID, instanceID, bgTypeID uint32, bgStatsID uint64, winnerTeam uint32, roster []*session) {
	if s == nil || bgStatsID == 0 || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	table := s.bgKillPlayerScores(mapID, instanceID)
	db := s.CharactersStore.DB
	ctx := context.Background()
	for _, sess := range roster {
		if sess == nil || sess.player == nil {
			continue
		}
		score := table[sess.playerGUID]
		var attr [5]uint32
		switch bgTypeID {
		case 1: // AV
			attr = [5]uint32{score.GraveyardsAssaulted, score.GraveyardsDefended, score.TowersAssaulted, score.TowersDefended, score.MinesCaptured}
		case 2: // WS
			attr = [5]uint32{score.FlagCaptures, score.FlagReturns}
		case 3: // AB
			attr = [5]uint32{score.BasesAssaulted, score.BasesDefended}
		case 7: // EY
			attr = [5]uint32{score.FlagCaptures}
		case 9: // SA
			attr = [5]uint32{score.DemolishersDestroyed, score.GatesDestroyed}
		case 30: // IC
			attr = [5]uint32{score.BasesAssaulted, score.BasesDefended}
		}
		winner := teamForRace(sess.player.Race) == winnerTeam
		_, _ = db.ExecContext(ctx, "INSERT INTO pvpstats_players (battleground_id, character_guid, winner, score_killing_blows, score_deaths, score_honorable_kills, score_bonus_honor, score_damage_done, score_healing_done, attr_1, attr_2, attr_3, attr_4, attr_5) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", bgStatsID, uint32(sess.playerGUID), winner, score.KillingBlows, score.Deaths, score.HonorableKills, score.BonusHonor, score.DamageDone, score.HealingDone, attr[0], attr[1], attr[2], attr[3], attr[4])
	}
}
