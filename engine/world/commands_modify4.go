package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// modify command port, chunk 2c: the modify arm reputation
// (cs_modify.cpp:711), the last of the 23 modify arms. Chunks 1, 2a and 2b
// covered morph, demorph and the other 22 arms; with this arm cs_modify.cpp
// is CLOSED.
//
// Native: the target player's standing is set to the parsed amount (numeric,
// or a reputation-rank name with an optional delta) via the SetOneFactionReputation
// mirror (clamped to [-42000, 42999], Standing = amount - BaseStanding,
// persisted to character_reputation and pushed with SMSG_SET_FACTION_STANDING).
// The faction link form (|cffffffff|Hfaction:<id>|h[name]|h|r) is parsed like
// ChatHandler::extractKeyFromLink (Chat.cpp:362): a non-link first token is
// used verbatim, the link-tail skip keeps the post-link tokens aligned with
// the C++ strtok continuation.

// modifyReputationPointsInRank mirrors ReputationMgr::PointsInRank
// (ReputationMgr.cpp:29): the standing span of each reputation rank.
var modifyReputationPointsInRank = [8]int32{36000, 3000, 3000, 3000, 6000, 12000, 21000, 1000}

// modifyReputationRankNames mirrors GetTrinityString(ReputationRankStrIndex[r])
// (ReputationMgr.h:28): the enUS rank names the C++ prefix-matches.
var modifyReputationRankNames = [8]string{
	"Hated", "Hostile", "Unfriendly", "Neutral",
	"Friendly", "Honored", "Revered", "Exalted",
}

// extractModifyFactionKey mirrors ChatHandler::extractKeyFromLink(text,
// "Hfaction") (Chat.cpp:362): leading spaces skipped, a non-link first token
// returned verbatim, a |color|Hfaction:key|h[name]|h|r link returning key with
// the link tail skipped so the remainder lines up with the C++ strtok
// continuation. A wrong link type returns an empty key.
func extractModifyFactionKey(raw string) (key, rest string) {
	raw = strings.TrimLeft(raw, " \t\b")
	if raw == "" {
		return "", ""
	}
	if raw[0] != '|' {
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) > 1 {
			return parts[0], parts[1]
		}
		return parts[0], ""
	}
	segments := strings.Split(raw, "|")
	if len(segments) < 3 || segments[1] == "" {
		return "", ""
	}
	link := segments[2]
	if !strings.HasPrefix(link, "Hfaction:") {
		return "", "" // LANG_WRONG_LINK_TYPE: null in the C++ path too
	}
	key = strings.SplitN(strings.TrimPrefix(link, "Hfaction:"), ":", 2)[0]
	if idx := strings.Index(raw, "|h|r"); idx >= 0 {
		rest = strings.TrimLeft(raw[idx+len("|h|r"):], " ")
	}
	return key, rest
}

// handleModifyReputation mirrors HandleModifyRepCommand (cs_modify.cpp:711):
// ".modify reputation <factionid> <amount|rank> [delta]" on the selected
// player or the handler's own player.
func (s *session) handleModifyReputation(ctx context.Context, args []string) {
	const syntax = "Syntax: .modify reputation <factionid> <amount|rank> [delta]"
	if s.miscDeny(ctx, permissionCommandModifyReputation) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	target := s.modifyTargetPlayer(ctx)
	if target == nil || s.modifyTargetLowerSecurity(ctx, target) {
		return
	}
	factionKey, rest := extractModifyFactionKey(strings.Join(args, " "))
	if factionKey == "" {
		s.sendSysMessage(syntax)
		return
	}
	factionID := uint32(cAtoi(factionKey))
	fields := strings.Fields(rest)
	if factionID == 0 || len(fields) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	rankTxt := fields[0]
	deltaTxt := ""
	if len(fields) > 1 {
		deltaTxt = fields[1]
	}
	amount := int32(cAtoi(rankTxt))
	if amount == 0 && rankTxt[0] != '-' && (rankTxt[0] < '0' || rankTxt[0] > '9') {
		// Rank-name leg (cs_modify.cpp:744): prefix-match the enUS rank
		// name and map it to a standing, plus an optional bounded delta.
		prefix := strings.ToLower(rankTxt)
		amount = -42000
		matched := false
		for r := 0; r < len(modifyReputationRankNames); r++ {
			if strings.HasPrefix(strings.ToLower(modifyReputationRankNames[r]), prefix) {
				matched = true
				if deltaTxt != "" {
					delta := cAtoi(deltaTxt)
					if delta < 0 || int32(delta) > modifyReputationPointsInRank[r]-1 {
						// LANG_COMMAND_FACTION_DELTA.
						s.sendSysMessage(fmt.Sprintf("Incorrect delta value. Expected parameter can be from 0 to %d.", modifyReputationPointsInRank[r]-1))
						return
					}
					amount += int32(delta)
				}
				break
			}
			amount += modifyReputationPointsInRank[r]
		}
		if !matched {
			// LANG_COMMAND_INVALID_PARAM.
			s.sendSysMessage(fmt.Sprintf("Invalid parameter '%s'.", rankTxt))
			return
		}
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Faction data is unavailable.")
		return
	}
	rep, found, err := s.server.Data.Reputation(factionID, target.player.Race, target.player.Class)
	if err != nil || !found {
		// LANG_COMMAND_FACTION_UNKNOWN.
		s.sendSysMessage(fmt.Sprintf("Unknown faction with id %d.", factionID))
		return
	}
	factionName := ""
	if file, fileErr := s.server.Data.File("Faction"); fileErr == nil && file != nil {
		if record, ok := file.Find(factionID); ok {
			factionName, _ = record.String(23) // FactionEntry.Name enUS (DBCStructure.h:671)
		}
	}
	if rep.ReputationList < 0 {
		// LANG_COMMAND_FACTION_NOREP_ERROR.
		s.sendSysMessage(fmt.Sprintf("Faction '%s' with id %d can't have reputation.", factionName, factionID))
		return
	}
	// SetOneFactionReputation mirror (ReputationMgr.cpp:374): clamp the
	// standing and store it net of the race/class base reputation.
	const reputationCap = 42999
	const reputationBottom = -42000
	standing := amount
	if standing > reputationCap {
		standing = reputationCap
	} else if standing < reputationBottom {
		standing = reputationBottom
	}
	index := -1
	for i := range target.player.Reputations {
		if target.player.Reputations[i].FactionID == factionID {
			index = i
			break
		}
	}
	if index < 0 {
		target.player.Reputations = append(target.player.Reputations, playerReputation{
			FactionID: factionID,
			ListID:    uint32(rep.ReputationList),
			Base:      rep.BaseStanding,
			Flags:     rep.DefaultFlags,
		})
		index = len(target.player.Reputations) - 1
	}
	entry := &target.player.Reputations[index]
	oldTotal := totalReputationStanding(*entry)
	entry.Standing = standing - rep.BaseStanding
	entry.Flags |= factionFlagVisible // SetVisible
	guid := s.playerGUIDOf(target)
	if chars := s.server.CharactersStore; chars != nil && chars.DB != nil {
		_, _ = chars.DB.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", guid, factionID, entry.Standing, entry.Flags)
	}
	newTotal := totalReputationStanding(*entry)
	increased := reputationRank(int64(newTotal)) > reputationRank(int64(oldTotal))
	_ = target.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState([]playerReputation{*entry}, increased), true)
	if newTotal > 0 {
		target.setAchievementCriteria(criteriaTypeKnownFactions, factionID, 1)
		target.setAchievementCriteria(criteriaTypeGainReputation, factionID, uint32(newTotal))
		if newTotal >= 9000 {
			target.updateAchievementCriteria(criteriaTypeHonoredRep, factionID, 1)
		}
		if newTotal >= 21000 {
			target.updateAchievementCriteria(criteriaTypeReveredRep, factionID, 1)
		}
		if newTotal >= 42000 {
			target.updateAchievementCriteria(criteriaTypeExaltedRep, factionID, 1)
		}
	}
	// LANG_COMMAND_MODIFY_REP: the handler's message, like the C++.
	s.sendSysMessage(fmt.Sprintf("You change %s faction's reputation (id %d) for %s to %d.", factionName, factionID, target.player.Name, newTotal))
}

// handleModifyChunk2c extends the modify dispatcher with the chunk-2c arm.
func (s *session) handleModifyChunk2c(ctx context.Context, sub string, rest []string) bool {
	if strings.HasPrefix("reputation", sub) {
		s.handleModifyReputation(ctx, rest)
		return true
	}
	return false
}
