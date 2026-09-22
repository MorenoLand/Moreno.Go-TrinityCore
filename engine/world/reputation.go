package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func totalReputationStanding(reputation playerReputation) int32 {
	return reputation.Base + reputation.Standing
}

const (
	factionFlagAtWar           uint8 = 0x02
	factionFlagHidden          uint8 = 0x04
	factionFlagInvisibleForced uint8 = 0x08
	factionFlagInactive        uint8 = 0x20
	factionFlagVisible         uint8 = 0x01
)

func (s *session) applyStartAllReputation(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	factions := []uint32{942, 935, 936, 1011, 970, 967, 989, 932, 934, 1038, 1077, 1106, 1104, 1090, 1098, 1156, 1073, 1105, 1119, 1091}
	if teamForRace(s.player.Race) == 0 {
		factions = append(factions, 72, 47, 69, 930, 730, 978, 54, 946, 1037, 1068, 1126, 1094, 1050)
	} else {
		factions = append(factions, 76, 68, 81, 911, 729, 941, 530, 947, 1052, 1067, 1124, 1064, 1085)
	}
	changed := make([]playerReputation, 0, len(factions))
	increased := false
	for _, factionID := range factions {
		reputation, found, err := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class)
		if err != nil || !found || reputation.ReputationList < 0 {
			continue
		}
		index := -1
		for i := range s.player.Reputations {
			if s.player.Reputations[i].FactionID == factionID {
				index = i
				break
			}
		}
		if index < 0 {
			s.player.Reputations = append(s.player.Reputations, playerReputation{FactionID: factionID, ListID: uint32(reputation.ReputationList), Base: reputation.BaseStanding, Flags: reputation.DefaultFlags})
			index = len(s.player.Reputations) - 1
		}
		oldRank := reputationRank(int64(totalReputationStanding(s.player.Reputations[index])))
		s.player.Reputations[index].ListID = uint32(reputation.ReputationList)
		s.player.Reputations[index].Standing = 42999 - reputation.BaseStanding
		s.player.Reputations[index].Flags |= factionFlagVisible
		if s.player.Reputations[index].Flags == factionFlagVisible {
			s.player.Reputations[index].Flags |= reputation.DefaultFlags
		}
		if reputationRank(int64(totalReputationStanding(s.player.Reputations[index]))) > oldRank {
			increased = true
		}
		changed = append(changed, s.player.Reputations[index])
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, s.player.Reputations[index].Standing, s.player.Reputations[index].Flags)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState(changed, increased), true)
}

func buildFactionStandingState(reputations []playerReputation, increased bool) []byte {
	packet := protocol.NewBuffer(13 + len(reputations)*8)
	packet.WriteF32(0)
	if increased {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(len(reputations)))
	for _, reputation := range reputations {
		packet.WriteU32(reputation.ListID)
		packet.WriteU32(uint32(reputation.Standing))
	}
	return packet.Bytes()
}

// handleSetWatchedFaction mirrors WorldSession::HandleSetWatchedFactionOpcode
// (CharacterHandler.cpp): read the reputation list index and store it in
// PLAYER_FIELD_WATCHED_FACTION_INDEX, pushing the field change to the owning
// client as a values update.
func (s *session) handleSetWatchedFaction(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	fact, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.player.WatchedFaction = fact
	packet, err := s.server.buildPlayerValuesUpdate(s.playerGUID, map[int]uint32{unitFieldWatchedFaction: fact})
	if err != nil {
		s.debug("watched faction values update build failed", "account", s.accountName, "error", err)
		return true
	}
	_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	return true
}

// handleSetFactionInactive mirrors WorldSession::HandleSetFactionInactiveOpcode
// plus ReputationMgr::SetInactive: toggle FACTION_FLAG_INACTIVE on the faction
// matching the reputation list ID. Hidden, forced-invisible, or not-yet-visible
// factions cannot be inactivated, and already-matching states are ignored. The
// flag is persisted immediately like the at-war toggle and only reaches the
// client through SMSG_INITIALIZE_FACTIONS on the next login, matching the
// reference where SendState carries standing but never flags.
func (s *session) handleSetFactionInactive(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	reader := protocol.NewReader(payload)
	listID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	inactive, err := reader.ReadU8()
	if err != nil {
		return false
	}
	for index := range s.player.Reputations {
		reputation := &s.player.Reputations[index]
		if reputation.ListID != listID {
			continue
		}
		if inactive != 0 {
			if reputation.Flags&(factionFlagInvisibleForced|factionFlagHidden) != 0 || reputation.Flags&factionFlagVisible == 0 {
				return true
			}
			if reputation.Flags&factionFlagInactive != 0 {
				return true
			}
			reputation.Flags |= factionFlagInactive
		} else {
			if reputation.Flags&factionFlagInactive == 0 {
				return true
			}
			reputation.Flags &^= factionFlagInactive
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_reputation SET flags = ? WHERE guid = ? AND faction = ?", reputation.Flags, s.playerGUID, reputation.FactionID)
		}
		s.debug("faction inactive state changed", "account", s.accountName, "faction", reputation.FactionID, "inactive", inactive != 0)
		return true
	}
	return true
}

func (s *session) handleSetFactionAtWar(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 5 {
		return true
	}
	reader := protocol.NewReader(payload)
	listID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	flag, err := reader.ReadU8()
	if err != nil {
		return false
	}
	for index := range s.player.Reputations {
		reputation := &s.player.Reputations[index]
		if reputation.ListID != listID {
			continue
		}
		if flag != 0 {
			reputation.Flags |= factionFlagAtWar
		} else {
			reputation.Flags &^= factionFlagAtWar
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_reputation SET flags = ? WHERE guid = ? AND faction = ?", reputation.Flags, s.playerGUID, reputation.FactionID)
		}
		s.debug("faction war state changed", "account", s.accountName, "faction", reputation.FactionID, "at_war", flag != 0)
		return true
	}
	return true
}

func (s *session) giveReputation(ctx context.Context, factionID uint32, amount int32) {
	if s.player == nil || factionID == 0 || amount == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	for i := range s.player.Reputations {
		if s.player.Reputations[i].FactionID == factionID {
			s.player.Reputations[i].Standing += amount
			_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET standing = ? WHERE guid = ? AND faction = ?", s.player.Reputations[i].Standing, s.playerGUID, factionID)
			totalStanding := totalReputationStanding(s.player.Reputations[i])
			if totalStanding > 0 {
				s.setAchievementCriteria(criteriaTypeGainReputation, factionID, uint32(totalStanding))
				if totalStanding >= 9000 {
					s.updateAchievementCriteria(criteriaTypeHonoredRep, factionID, 1)
				}
				if totalStanding >= 21000 {
					s.updateAchievementCriteria(criteriaTypeReveredRep, factionID, 1)
				}
				// Reference GAIN_EXALTED_REPUTATION: any faction at 42000+ counts.
				if totalStanding >= 42000 {
					s.updateAchievementCriteria(criteriaTypeExaltedRep, factionID, 1)
				}
			}
			return
		}
	}
	rep := playerReputation{
		FactionID: factionID,
		Standing:  amount,
		Flags:     factionFlagVisible,
	}
	s.player.Reputations = append(s.player.Reputations, rep)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_reputation (guid, faction, standing, flags) VALUES (?, ?, ?, ?)", s.playerGUID, factionID, amount, factionFlagVisible)
}
