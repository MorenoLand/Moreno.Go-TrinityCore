package world

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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

func (s *session) handleEmote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 || s.player.Health == 0 {
		return true
	}
	reader := protocol.NewReader(payload)
	emote, err := reader.ReadU32()
	// Reference: WorldSession::HandleEmoteOpcode (ChatHandler.cpp:631-637) —
	// the client hardcodes only EMOTE_ONESHOT_NONE (0) and EMOTE_ONESHOT_WAVE
	// (3); 17 is EMOTE_ONESHOT_KISS and is not a valid CMSG_EMOTE id.
	if err != nil || (emote != 0 && emote != 3) {
		return true
	}
	// C++ (ChatHandler.cpp:636-647): Eluna PLAYER_EVENT_ON_EMOTE (23) fires
	// from ScriptMgr::OnPlayerEmote before the native HandleEmoteCommand; the
	// hook is void (CallAllFunctions) and cannot cancel.
	s.triggerPlayerEvent(ctx, scripting.PlayerEventEmote, s.luaPlayer(), emote)
	packet := protocol.NewBuffer(12)
	packet.WriteU32(emote)
	packet.WriteU64(s.playerGUID)
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for member := range s.server.sessions {
		if !member.worldReady.Load() || member.player == nil || member.player.Map != s.player.Map {
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
	// C++ (ChatHandler.cpp:683-693): text emotes are muted like chat, with
	// the LANG_WAIT_BEFORE_SPEAKING notification (705, secsToTimeString
	// ShortText duration).
	if now := time.Now().Unix(); s.muteTime > now {
		s.sendNotification(fmt.Sprintf("You must wait %s before speaking again.", secsToTimeStringShort(uint64(s.muteTime-now))))
		s.debug("text emote rejected", "account", s.accountName, "reason", "account muted")
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
	// C++ (ChatHandler.cpp:699): Eluna PLAYER_EVENT_ON_TEXT_EMOTE (24) fires
	// from ScriptMgr::OnPlayerTextEmote right after the payload read, before
	// the EmotesText lookup; the hook is void (CallAllFunctions).
	s.triggerPlayerEvent(ctx, scripting.PlayerEventTextEmote, s.luaPlayer(), textEmote, emoteNum, targetGUID)
	file, err := s.server.Data.File("EmotesText")
	if err != nil {
		return true
	}
	entry, found := file.Find(textEmote)
	if !found {
		return true
	}
	// Reference: WorldSession::HandleTextEmoteOpcode (ChatHandler.cpp:696-698
	// then the criteria call after the broadcast) — the
	// ACHIEVEMENT_CRITERIA_TYPE_DO_EMOTE update runs only after the EmotesText
	// lookup succeeds; C++ returns early on an invalid text emote id before
	// the criteria update. The quantity stays 1: Go's additive progress model
	// has no SET-type analog, and the C++ quantity 0 is ignored there (C++
	// sets the counter to 1 via miscValue1 != 0 in AchievementMgr.cpp).
	s.updateAchievementCriteria(criteriaTypeDoEmote, textEmote, 1)
	visualEmote, err := entry.Uint32(2)
	if err != nil {
		return true
	}
	// Reference: WorldSession::HandleTextEmoteOpcode (ChatHandler.cpp:700-714)
	// — the state emotes (SLEEP=12, SIT=13, KNEEL=68) and NONE=0 skip
	// HandleEmoteCommand entirely; only the default arm sends the visual
	// SMSG_EMOTE. The dead-entity arm (UNIT_STATE_DIED) is unreachable here:
	// the handler returns early when the player is dead, like C++'s
	// !IsAlive() gate.
	sendVisual := false
	switch visualEmote {
	case 12, 13, 68:
	default:
		sendVisual = visualEmote != 0
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
	if sendVisual {
		buf := protocol.NewBuffer(12)
		buf.WriteU32(visualEmote)
		buf.WriteU64(s.playerGUID)
		visPacket = buf.Bytes()
	}

	// Reference: WorldSession::HandleTextEmoteOpcode (ChatHandler.cpp:718-723)
	// — the text emote packet goes to the sender's cell visit with
	// CONFIG_LISTEN_RANGE_TEXTEMOTE, not to the whole map (unlike the oneshot
	// CMSG_EMOTE path, which is map-wide via SendMessageToSet).
	listenRange := float64(40)
	if s.server != nil && s.server.Config.ChatListenRangeTextEmote > 0 {
		listenRange = s.server.Config.ChatListenRangeTextEmote
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for member := range s.server.sessions {
		if !member.worldReady.Load() || member.player == nil || member.player.Map != s.player.Map || member.player.InstanceID != s.player.InstanceID {
			continue
		}
		if distance3D(s.player.X, s.player.Y, s.player.Z, member.player.X, member.player.Y, member.player.Z) > listenRange {
			continue
		}
		_ = member.write(uint16(protocol.OpcodeSMSG_TEXT_EMOTE), packet.Bytes(), true)
		if len(visPacket) > 0 {
			_ = member.write(uint16(protocol.OpcodeSMSG_EMOTE), visPacket, true)
		}
	}
	return true
}
