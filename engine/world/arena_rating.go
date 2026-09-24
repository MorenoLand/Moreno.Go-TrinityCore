package world

import (
	"context"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	ArenaTeamType2v2 uint32 = 2
	ArenaTeamType3v3 uint32 = 3
	ArenaTeamType5v5 uint32 = 5

	ArenaMinTeamGamesForPoints uint32 = 10
	ArenaMinPlayerGameRatio           = 0.30 // 30% participation
	ArenaMaxPersonalRatingDiff uint32 = 150
	ArenaPointsCap             uint32 = 10000
)

// CalcRatingDelta calculates the Elo rating change for an Arena match.
// Mirrors TrinityCore Battleground::GetRatingBonus and Battleground::GetRatingLoss (Battleground.cpp:780-840).
func CalcRatingDelta(winnerRating, loserRating uint32) (winnerGain, loserLoss int32) {
	diff := float64(int64(loserRating) - int64(winnerRating))
	winProb := 1.0 / (1.0 + math.Pow(10.0, diff/400.0))

	// Base K factor in 3.3.5 is 32
	const k = 32.0
	gain := int32(math.Round(k * (1.0 - winProb)))
	if gain < 1 {
		gain = 1
	}

	loss := gain
	// Under 1000 rating, rating loss is scaled down (WotLK catchup mechanism)
	if loserRating < 1000 {
		loss = int32(math.Round(float64(gain) * (float64(loserRating) / 1000.0)))
	}

	return gain, loss
}

// CalculateArenaPoints calculates the weekly Arena Points awarded for a given rating and bracket type.
// Mirrors TrinityCore ArenaTeam::FinishWeek (ArenaTeam.cpp:320-360).
func CalculateArenaPoints(rating uint32, teamType uint32) uint32 {
	if rating <= 150 {
		return 0
	}

	var points float64
	if rating <= 1500 {
		points = 0.22*float64(rating) + 14.0
	} else {
		points = 1511.26 / (1.0 + 1639.28*math.Exp(-0.00412*float64(rating)))
	}

	// Bracket multiplier: 2v2: 76%, 3v3: 88%, 5v5: 100%
	switch teamType {
	case ArenaTeamType2v2:
		points *= 0.76
	case ArenaTeamType3v3:
		points *= 0.88
	case ArenaTeamType5v5:
		points *= 1.00
	default:
		points *= 0.76
	}

	return uint32(math.Round(points))
}

// RecordArenaMatchResult applies rating and win/loss statistics to participating arena teams and members.
// Mirrors TrinityCore ArenaTeam::WonAgainst and ArenaTeam::LostAgainst (ArenaTeam.cpp:380-450).
func (s *Server) RecordArenaMatchResult(ctx context.Context, winnerTeamID, loserTeamID uint32, winnerMembers, loserMembers []uint64) (winnerGain, loserLoss int32, err error) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0, 0, nil
	}
	cdb := s.CharactersStore.DB

	var winRating, loseRating, winType, loseType uint32
	err = cdb.QueryRowContext(ctx, "SELECT rating, type FROM arena_team WHERE arenaTeamId = ?", winnerTeamID).Scan(&winRating, &winType)
	if err != nil {
		return 0, 0, err
	}
	err = cdb.QueryRowContext(ctx, "SELECT rating, type FROM arena_team WHERE arenaTeamId = ?", loserTeamID).Scan(&loseRating, &loseType)
	if err != nil {
		return 0, 0, err
	}

	gain, loss := CalcRatingDelta(winRating, loseRating)

	newWinRating := winRating + uint32(gain)
	var newLoseRating uint32
	if loseRating > uint32(loss) {
		newLoseRating = loseRating - uint32(loss)
	} else {
		newLoseRating = 0
	}

	// Update winning team
	_, err = cdb.ExecContext(ctx, `UPDATE arena_team SET rating = ?, weekGames = weekGames + 1, weekWins = weekWins + 1,
		seasonGames = seasonGames + 1, seasonWins = seasonWins + 1 WHERE arenaTeamId = ?`, newWinRating, winnerTeamID)
	// Reference ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_TEAM_RATING: absolute
	// progress for the winning team members' new rating.
	for _, guid := range winnerMembers {
		if sess := s.findSessionByGUID(guid); sess != nil {
			sess.setAchievementCriteria(criteriaTypeHighestTeamRating, 0, newWinRating)
		}
	}
	if err != nil {
		return 0, 0, err
	}

	// Update losing team
	_, err = cdb.ExecContext(ctx, `UPDATE arena_team SET rating = ?, weekGames = weekGames + 1,
		seasonGames = seasonGames + 1 WHERE arenaTeamId = ?`, newLoseRating, loserTeamID)
	if err != nil {
		return 0, 0, err
	}

	// Update winner participating members
	for _, mGUID := range winnerMembers {
		var curPersonal uint32
		_ = cdb.QueryRowContext(ctx, "SELECT personalRating FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", winnerTeamID, mGUID).Scan(&curPersonal)
		newPersonal := curPersonal + uint32(gain)
		_, _ = cdb.ExecContext(ctx, `UPDATE arena_team_member SET personalRating = personalRating + ?,
			weekGames = weekGames + 1, weekWins = weekWins + 1, seasonGames = seasonGames + 1, seasonWins = seasonWins + 1
			WHERE arenaTeamId = ? AND guid = ?`, gain, winnerTeamID, mGUID)
		if sess := s.findSessionByGUID(mGUID); sess != nil {
			sess.setAchievementCriteria(criteriaTypeHighestPersonalRating, 0, newPersonal)
		}
	}

	// Update loser participating members
	for _, mGUID := range loserMembers {
		var curPersonal uint32
		_ = cdb.QueryRowContext(ctx, "SELECT personalRating FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", loserTeamID, mGUID).Scan(&curPersonal)
		pLoss := loss
		if curPersonal < 1000 {
			pLoss = int32(math.Round(float64(gain) * (float64(curPersonal) / 1000.0)))
		}
		newPersonal := curPersonal
		if newPersonal > uint32(pLoss) {
			newPersonal -= uint32(pLoss)
		} else {
			newPersonal = 0
		}
		_, _ = cdb.ExecContext(ctx, `UPDATE arena_team_member SET personalRating = ?,
			weekGames = weekGames + 1, seasonGames = seasonGames + 1
			WHERE arenaTeamId = ? AND guid = ?`, newPersonal, loserTeamID, mGUID)
	}

	s.broadcastArenaTeamStats(winnerTeamID, newWinRating)
	s.broadcastArenaTeamStats(loserTeamID, newLoseRating)

	return gain, loss, nil
}

func (s *Server) broadcastArenaTeamStats(teamID, rating uint32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	var weekGames, weekWins, seasonGames, seasonWins, rank uint32
	err := s.CharactersStore.DB.QueryRowContext(context.Background(),
		"SELECT weekGames, weekWins, seasonGames, seasonWins, rank FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(
		&weekGames, &weekWins, &seasonGames, &seasonWins, &rank)
	if err != nil {
		return
	}

	sBuf := protocol.NewBuffer(28)
	sBuf.WriteU32(teamID)
	sBuf.WriteU32(rating)
	sBuf.WriteU32(weekGames)
	sBuf.WriteU32(weekWins)
	sBuf.WriteU32(seasonGames)
	sBuf.WriteU32(seasonWins)
	sBuf.WriteU32(rank)

	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.authed && sess.worldReady.Load() {
			_ = sess.write(uint16(protocol.OpcodeSMSG_ARENA_TEAM_STATS), sBuf.Bytes(), true)
		}
	}
}

// FlushWeeklyArenaPoints calculates and distributes weekly Arena Points to all eligible players.
// Mirrors TrinityCore ArenaTeam::FinishWeek (ArenaTeam.cpp:320-380).
func (s *Server) FlushWeeklyArenaPoints(ctx context.Context) (totalGranted uint32, err error) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0, nil
	}
	cdb := s.CharactersStore.DB

	type memberRecord struct {
		teamID         uint32
		playerGUID     uint64
		teamType       uint32
		teamRating     uint32
		teamWeekGames  uint32
		memberWeekGame uint32
		personalRating uint32
	}

	rows, err := cdb.QueryContext(ctx, `
		SELECT t.arenaTeamId, m.guid, t.type, t.rating, t.weekGames, m.weekGames, m.personalRating
		FROM arena_team_member AS m
		JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	playerMaxPoints := make(map[uint64]uint32)

	for rows.Next() {
		var rec memberRecord
		if err := rows.Scan(&rec.teamID, &rec.playerGUID, &rec.teamType, &rec.teamRating, &rec.teamWeekGames, &rec.memberWeekGame, &rec.personalRating); err != nil {
			continue
		}

		// Qualification check (TrinityCore ArenaTeam::FinishWeek):
		// 1. Team must have played at least 10 games this week
		// 2. Member must have played at least 30% of team's games
		// 3. Member personal rating must be within 150 of team rating
		if rec.teamWeekGames < ArenaMinTeamGamesForPoints {
			continue
		}
		if float64(rec.memberWeekGame) < float64(rec.teamWeekGames)*ArenaMinPlayerGameRatio {
			continue
		}
		effectiveRating := rec.personalRating
		if rec.teamRating < effectiveRating {
			effectiveRating = rec.teamRating
		}
		if rec.teamRating > rec.personalRating && rec.teamRating-rec.personalRating > ArenaMaxPersonalRatingDiff {
			continue
		}

		points := CalculateArenaPoints(effectiveRating, rec.teamType)
		if points > playerMaxPoints[rec.playerGUID] {
			playerMaxPoints[rec.playerGUID] = points
		}
	}

	// Grant arena points to characters (respecting 10,000 cap)
	for guid, pts := range playerMaxPoints {
		if pts == 0 {
			continue
		}
		var curPoints uint32
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(arenaPoints, 0) FROM characters WHERE guid = ?", guid).Scan(&curPoints)
		newPoints := curPoints + pts
		if newPoints > ArenaPointsCap {
			newPoints = ArenaPointsCap
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET arenaPoints = ? WHERE guid = ?", newPoints, guid)
		totalGranted += pts

		// Update online session if present
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
			sess.player.ArenaPoints = newPoints
			sess.sendPlayerUpdate()
		}
	}

	// Reset weekGames and weekWins
	_, _ = cdb.ExecContext(ctx, "UPDATE arena_team SET weekGames = 0, weekWins = 0")
	_, _ = cdb.ExecContext(ctx, "UPDATE arena_team_member SET weekGames = 0, weekWins = 0")

	return totalGranted, nil
}
