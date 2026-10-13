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

// arenaGetChanceAgainst mirrors ArenaTeam::GetChanceAgainst (ArenaTeam.cpp:680):
// 1 / (1 + exp(ln(10) * (opponent - own) / 650)).
func arenaGetChanceAgainst(ownRating, opponentRating uint32) float64 {
	return 1.0 / (1.0 + math.Exp(math.Log(10.0)*(float64(opponentRating)-float64(ownRating))/650.0))
}

// arenaGetRatingMod mirrors ArenaTeam::GetRatingMod (ArenaTeam.cpp:713). The win
// arm is tiered on the team's own rating (<1000: winMod1 flat; 1000-1300: winMod1
// scaled by (1300-rating)/300; >=1300: winMod2), the loss arm is loseMod * -chance;
// every arm returns ceil(mod).
func arenaGetRatingMod(ownRating, opponentRating uint32, won bool, winMod1, winMod2, loseMod float64) int32 {
	chance := arenaGetChanceAgainst(ownRating, opponentRating)
	var mod float64
	if won {
		if ownRating < 1300 {
			if ownRating < 1000 {
				mod = winMod1 * (1.0 - chance)
			} else {
				mod = ((winMod1 / 2.0) + ((winMod1 / 2.0) * (1300.0 - float64(ownRating)) / 300.0)) * (1.0 - chance)
			}
		} else {
			mod = winMod2 * (1.0 - chance)
		}
	} else {
		mod = loseMod * -chance
	}
	return int32(math.Ceil(mod))
}

// arenaGetMatchmakerRatingMod mirrors ArenaTeam::GetMatchmakerRatingMod
// (ArenaTeam.cpp:687): ceil(((won ? 1 : 0) - chance) * mmMod).
func arenaGetMatchmakerRatingMod(ownMMR, opponentMMR uint32, won bool, mmMod float64) int32 {
	chance := arenaGetChanceAgainst(ownMMR, opponentMMR)
	var wonMod float64
	if won {
		wonMod = 1.0
	}
	return int32(math.Ceil((wonMod - chance) * mmMod))
}

// arenaTeamRatingLossMod keeps the old single-call shape for the deserter path:
// ArenaTeam::GetRatingMod (ArenaTeam.cpp:713) for a loss with the configured
// Arena.ArenaLoseRatingModifier (World.cpp:1224, 24.0 default).
func arenaTeamRatingLossMod(ownRating, opponentRating uint32, loseMod float64) int32 {
	return arenaGetRatingMod(ownRating, opponentRating, false, 0, 0, loseMod)
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
// Mirrors ArenaTeam::WonAgainst / LostAgainst (ArenaTeam.cpp:769-804) as driven by
// Arena::EndBattleground (Arena.cpp:193-204): the team rating change is
// GetRatingMod(own team rating, opponent team MMR, won), the MMR change is
// GetMatchmakerRatingMod(own MMR, opponent MMR, won), and each participant's
// personal change is GetRatingMod(personal rating, opponent MMR, won)
// (ArenaTeam::MemberWon/MemberLost, ArenaTeam.cpp:805-880). C++ SetArenaTeamInfoField
// unit-field updates have no Go model; PreviousOpponents is queue-only (no Go
// matchmaker). MMR persists per (guid, slot) in character_arena_stats.
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

	winMod1 := s.Config.ArenaWinRatingModifier1
	winMod2 := s.Config.ArenaWinRatingModifier2
	loseMod := s.Config.ArenaLoseRatingModifier
	mmMod := s.Config.ArenaMatchmakerRatingModifier

	// C++ GetArenaMatchmakerRating is the match-time MMR (m_ArenaTeamMMR, set at
	// match start from the queued players' average); the end-of-match
	// participants' average is its twin here.
	winSlot := arenaTeamSlotByType(winType)
	winnerMMR := s.avgArenaMatchmakerRating(ctx, winnerMembers, winSlot)
	loserMMR := s.avgArenaMatchmakerRating(ctx, loserMembers, winSlot)

	winnerChange := arenaGetRatingMod(winRating, loserMMR, true, winMod1, winMod2, loseMod)
	winnerMMRChange := arenaGetMatchmakerRatingMod(winnerMMR, loserMMR, true, mmMod)
	loserChange := arenaGetRatingMod(loseRating, winnerMMR, false, winMod1, winMod2, loseMod)
	loserMMRChange := arenaGetMatchmakerRatingMod(loserMMR, winnerMMR, false, mmMod)

	// ArenaTeam::FinishGame: the team rating floors at 0 (ArenaTeam.cpp:736-754);
	// the HIGHEST_TEAM_RATING criteria only fires when it did not floor.
	applyRatingFloor := func(rating uint32, change int32) uint32 {
		if change < 0 && uint32(-change) >= rating {
			return 0
		}
		return uint32(int32(rating) + change)
	}
	newWinRating := applyRatingFloor(winRating, winnerChange)
	newLoseRating := applyRatingFloor(loseRating, loserChange)

	// Update winning team: games, wins, rank recompute (FinishGame).
	var winRank uint32
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*)+1 FROM arena_team WHERE type = ? AND rating > ?", winType, newWinRating).Scan(&winRank)
	_, err = cdb.ExecContext(ctx, `UPDATE arena_team SET rating = ?, weekGames = weekGames + 1, weekWins = weekWins + 1,
		seasonGames = seasonGames + 1, seasonWins = seasonWins + 1, rank = ? WHERE arenaTeamId = ?`,
		newWinRating, winRank, winnerTeamID)
	if err != nil {
		return 0, 0, err
	}
	if int32(winRating)+winnerChange >= 0 {
		for _, guid := range winnerMembers {
			if sess := s.findSessionByGUID(guid); sess != nil {
				sess.setAchievementCriteria(criteriaTypeHighestTeamRating, 0, newWinRating)
			}
		}
	}

	// Update losing team.
	var loseRank uint32
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*)+1 FROM arena_team WHERE type = ? AND rating > ?", loseType, newLoseRating).Scan(&loseRank)
	_, err = cdb.ExecContext(ctx, `UPDATE arena_team SET rating = ?, weekGames = weekGames + 1,
		seasonGames = seasonGames + 1, rank = ? WHERE arenaTeamId = ?`, newLoseRating, loseRank, loserTeamID)
	if err != nil {
		return 0, 0, err
	}
	if int32(loseRating)+loserChange >= 0 {
		for _, guid := range loserMembers {
			if sess := s.findSessionByGUID(guid); sess != nil {
				sess.setAchievementCriteria(criteriaTypeHighestTeamRating, 0, newLoseRating)
			}
		}
	}

	// ArenaTeam::MemberWon: personal change is GetRatingMod(personal, opponent
	// MMR, won); MMR shifts by the team MMR change; personal stats gain games
	// and wins (ArenaTeam.cpp:852-880). ModifyPersonalRating fires
	// HIGHEST_PERSONAL_RATING for online members and floors at 0.
	for _, mGUID := range winnerMembers {
		var curPersonal uint32
		_ = cdb.QueryRowContext(ctx, "SELECT personalRating FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", winnerTeamID, mGUID).Scan(&curPersonal)
		newPersonal := applyRatingFloor(curPersonal, arenaGetRatingMod(curPersonal, loserMMR, true, winMod1, winMod2, loseMod))
		_, _ = cdb.ExecContext(ctx, `UPDATE arena_team_member SET personalRating = ?,
			weekGames = weekGames + 1, weekWins = weekWins + 1, seasonGames = seasonGames + 1, seasonWins = seasonWins + 1
			WHERE arenaTeamId = ? AND guid = ?`, newPersonal, winnerTeamID, mGUID)
		s.applyArenaMMRChange(ctx, mGUID, winSlot, winnerMMRChange)
		if sess := s.findSessionByGUID(mGUID); sess != nil {
			sess.setAchievementCriteria(criteriaTypeHighestPersonalRating, 0, newPersonal)
		}
	}

	// ArenaTeam::MemberLost: same shape, no wins (ArenaTeam.cpp:805-834).
	for _, mGUID := range loserMembers {
		var curPersonal uint32
		_ = cdb.QueryRowContext(ctx, "SELECT personalRating FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", loserTeamID, mGUID).Scan(&curPersonal)
		newPersonal := applyRatingFloor(curPersonal, arenaGetRatingMod(curPersonal, winnerMMR, false, winMod1, winMod2, loseMod))
		_, _ = cdb.ExecContext(ctx, `UPDATE arena_team_member SET personalRating = ?,
			weekGames = weekGames + 1, seasonGames = seasonGames + 1
			WHERE arenaTeamId = ? AND guid = ?`, newPersonal, loserTeamID, mGUID)
		s.applyArenaMMRChange(ctx, mGUID, winSlot, loserMMRChange)
		if sess := s.findSessionByGUID(mGUID); sess != nil {
			sess.setAchievementCriteria(criteriaTypeHighestPersonalRating, 0, newPersonal)
		}
	}

	s.broadcastArenaTeamStats(winnerTeamID, newWinRating)
	s.broadcastArenaTeamStats(loserTeamID, newLoseRating)

	return winnerChange, -loserChange, nil
}

// avgArenaMatchmakerRating averages the participants' stored MMR for the slot;
// members with no stored MMR count as Arena.ArenaStartMatchmakerRating, the
// AddMember fallback (ArenaTeam.cpp:141). C++ sets the match MMR at queue time
// (Battleground.h m_ArenaTeamMMR) from the same population.
func (s *Server) avgArenaMatchmakerRating(ctx context.Context, members []uint64, slot uint32) uint32 {
	fallback := uint32(1500)
	if s != nil && s.Config.ArenaStartMatchmakerRating > 0 {
		fallback = uint32(s.Config.ArenaStartMatchmakerRating)
	}
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || len(members) == 0 {
		return fallback
	}
	var sum uint64
	for _, guid := range members {
		var mmr uint32
		if err := s.CharactersStore.DB.QueryRowContext(ctx, "SELECT matchMakerRating FROM character_arena_stats WHERE guid = ? AND slot = ?", guid, slot).Scan(&mmr); err != nil {
			mmr = fallback
		}
		sum += uint64(mmr)
	}
	return uint32(sum / uint64(len(members)))
}

// applyArenaMMRChange mirrors ArenaTeamMember::ModifyMatchmakerRating
// (ArenaTeam.cpp:536): the member's MMR shifts by mod, floored at 0, persisted
// in character_arena_stats for the member's slot.
func (s *Server) applyArenaMMRChange(ctx context.Context, guid uint64, slot uint32, mod int32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	cdb := s.CharactersStore.DB
	fallback := uint32(1500)
	if s.Config.ArenaStartMatchmakerRating > 0 {
		fallback = uint32(s.Config.ArenaStartMatchmakerRating)
	}
	var cur uint32
	err := cdb.QueryRowContext(ctx, "SELECT matchMakerRating FROM character_arena_stats WHERE guid = ? AND slot = ?", guid, slot).Scan(&cur)
	if err != nil {
		cur = fallback
	}
	newMMR := cur
	if mod < 0 && uint32(-mod) >= newMMR {
		newMMR = 0
	} else {
		newMMR = uint32(int32(newMMR) + mod)
	}
	if err != nil {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO character_arena_stats (guid, slot, matchMakerRating) VALUES (?, ?, ?)", guid, slot, newMMR)
	} else {
		_, _ = cdb.ExecContext(ctx, "UPDATE character_arena_stats SET matchMakerRating = ? WHERE guid = ? AND slot = ?", newMMR, guid, slot)
	}
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
			if newPoints > 0 {
				sess.addKnownCurrency(sess.player, itemArenaPointsID)
			}
			sess.sendPlayerUpdate()
		}
	}

	// Reset weekGames and weekWins
	_, _ = cdb.ExecContext(ctx, "UPDATE arena_team SET weekGames = 0, weekWins = 0")
	_, _ = cdb.ExecContext(ctx, "UPDATE arena_team_member SET weekGames = 0, weekWins = 0")

	return totalGranted, nil
}
