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
// (CHAR_SEL_PVPSTATS_MAXID); the per-player pvpstats_players rows have no
// Go writer (scores exist per-player, but no roster insert runs at BG end).
// Best-effort, like the other character-DB arms: a missing store or a
// failed write drops the row silently.
func (s *Server) recordBattlegroundStats(mapID, winningTeam uint32) {
	if s == nil || !s.Config.BattlegroundStoreStatisticsEnable {
		return
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
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	ctx := context.Background()
	var maxID uint64
	_ = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM pvpstats_battlegrounds").Scan(&maxID)
	bgType, _, _, _ := battlegroundTypeForMap(mapID)
	bracket := s.bgBracketFor(mapID, 0)
	_, _ = db.ExecContext(ctx, "INSERT INTO pvpstats_battlegrounds (id, winner_faction, bracket_id, type, date) VALUES (?, ?, ?, ?, ?)", maxID+1, winnerFaction, bracket, bgType, time.Now().Format("2006-01-02 15:04:05"))
}
