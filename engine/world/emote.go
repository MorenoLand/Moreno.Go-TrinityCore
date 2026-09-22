package world

import (
	"context"
	"database/sql"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *session) handleStandStateChange(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	state, err := reader.ReadU32()
	if err != nil || state > 3 {
		return true
	}
	s.player.StandState = uint8(state)
	s.server.broadcastPlayerValuesUpdateFromSession(s, map[int]uint32{unitFieldBytes1: state})
	s.debug("stand state changed", "account", s.accountName, "state", state)
	return true
}

func (s *session) handleEmote(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 || s.player.Health == 0 {
		return true
	}
	reader := protocol.NewReader(payload)
	emote, err := reader.ReadU32()
	if err != nil || (emote != 0 && emote != 17) {
		return true
	}
	packet := protocol.NewBuffer(12)
	packet.WriteU32(emote)
	packet.WriteU64(s.playerGUID)
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for member := range s.server.sessions {
		if !member.playerLoaded || member.player == nil || member.player.Map != s.player.Map {
			continue
		}
		_ = member.write(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), true)
	}
	s.debug("emote sent", "account", s.accountName, "emote", emote)
	return true
}

func (s *session) handleTextEmote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.player.Health == 0 || len(payload) < 16 || s.server.Data == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	textEmote, err := reader.ReadU32()
	if err != nil {
		return false
	}
	emoteNum, err := reader.ReadU32()
	if err != nil {
		return false
	}
	targetGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	file, err := s.server.Data.File("EmotesText")
	if err != nil {
		return true
	}
	entry, found := file.Find(textEmote)
	s.updateAchievementCriteria(criteriaTypeDoEmote, textEmote, 1)
	if !found {
		return true
	}
	visualEmote, err := entry.Uint32(2)
	if err != nil {
		return true
	}
	targetName := ""
	if targetGUID != 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		entryID := uint32((targetGUID >> 24) & 0x00FFFFFF)
		var name sql.NullString
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name FROM creature_template WHERE entry = ?", entryID).Scan(&name)
		if name.Valid {
			targetName = name.String
		}
		if targetGUID>>48 == 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", uint32(targetGUID&0x00FFFFFF)).Scan(&name)
			if name.Valid {
				targetName = name.String
			}
		}
	}
	packet := protocol.NewBuffer(32 + len(targetName) + 1)
	packet.WriteU64(s.playerGUID)
	packet.WriteU32(textEmote)
	packet.WriteU32(emoteNum)
	packet.WriteU32(uint32(len(targetName)))
	if len(targetName) > 1 {
		packet.WriteCString(targetName)
	} else {
		packet.WriteU8(0)
	}

	var visPacket []byte
	if visualEmote > 0 {
		buf := protocol.NewBuffer(12)
		buf.WriteU32(visualEmote)
		buf.WriteU64(s.playerGUID)
		visPacket = buf.Bytes()
	}

	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for member := range s.server.sessions {
		if !member.playerLoaded || member.player == nil || member.player.Map != s.player.Map {
			continue
		}
		_ = member.write(uint16(protocol.OpcodeSMSG_TEXT_EMOTE), packet.Bytes(), true)
		if len(visPacket) > 0 {
			_ = member.write(uint16(protocol.OpcodeSMSG_EMOTE), visPacket, true)
		}
	}
	return true
}
