package world

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	chatSystem         = 0x00
	chatSay            = 0x01
	chatParty          = 0x02
	chatRaid           = 0x03
	chatGuild          = 0x04
	chatOfficer        = 0x05
	chatYell           = 0x06
	chatWhisper        = 0x07
	chatEmote          = 0x0A
	chatChannel        = 0x11
	chatWhisperInform  = 0x09
	chatAFK            = 0x17
	chatDND            = 0x18
	chatIgnored        = 0x19
	chatBattleground   = 0x2C
	chatBattleLeader   = 0x2D
	chatPartyLeader    = 0x33
	maxChatMessageType = 0x34
	languageUniversal  = uint32(0)
	languageAddon      = ^uint32(0)
)

func (s *session) handleSetSelection(payload []byte) bool {
	if !s.playerLoaded {
		return true
	}
	b := protocol.NewReader(payload)
	selection, err := b.ReadU64()
	if err != nil {
		s.debug("selection rejected", "account", s.accountName, "error", err)
		return false
	}
	s.selection = selection
	return true
}

func (s *session) handleMessageChat(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		s.debug("chat ignored", "account", s.accountName, "reason", "player not loaded")
		return true
	}
	now := time.Now().Unix()
	if s.muteTime > 0 && s.muteTime <= now {
		s.muteTime = 0
		if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil && s.accountID != 0 {
			_, _ = s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET mutetime = 0 WHERE id = ?", s.accountID)
		}
	}
	if s.muteTime > now {
		remaining := s.muteTime - now
		if remaining < 1 {
			remaining = 1
		}
		s.sendNotification(fmt.Sprintf("You must wait %d seconds before speaking again.", remaining))
		s.debug("chat rejected", "account", s.accountName, "reason", "account muted", "mute_until", s.muteTime)
		return true
	}
	b := protocol.NewReader(payload)
	typeID, err := b.ReadU32()
	if err != nil {
		s.debug("chat rejected", "account", s.accountName, "reason", "malformed type", "error", err)
		return true
	}
	language, err := b.ReadU32()
	if err != nil {
		s.debug("chat rejected", "account", s.accountName, "reason", "malformed language", "error", err)
		return true
	}
	languageSkillID, languageKnown := languageSkill(language)
	s.debug("chat packet received", "account", s.accountName, "guid", s.playerGUID, "character", s.player.Name, "type", typeID, "language", language, "language_skill", languageSkillID, "language_known", languageKnown, "loaded_skill_count", len(s.player.Skills))
	if typeID >= maxChatMessageType {
		s.debug("chat rejected", "account", s.accountName, "reason", "invalid message type", "type", typeID)
		return true
	}
	if language == languageAddon && !addonChatType(typeID) {
		s.debug("chat rejected", "account", s.accountName, "reason", "invalid addon language type", "type", typeID)
		return true
	}
	if language != languageAddon && typeID != chatAFK && typeID != chatDND {
		if language == languageUniversal {
			s.sendNotification("Unknown language")
			s.debug("chat rejected", "account", s.accountName, "reason", "universal language")
			return true
		}
		skill, known := languageSkill(language)
		if !known {
			s.sendNotification("Unknown language")
			s.debug("chat rejected", "account", s.accountName, "reason", "unknown language", "language", language)
			return true
		}
		if skill != 0 && !s.hasLanguageSkill(skill) && !s.hasLanguageAura(language) {
			s.sendNotification("You don't know that language")
			s.debug("chat rejected", "account", s.accountName, "reason", "language not learned", "language", language, "skill", skill)
			return true
		}
	}
	if language != languageAddon && typeID != chatAFK && typeID != chatDND {
		s.updateSpeakTime()
	}
	var targetName, channel, message string
	switch uint8(typeID) {
	case chatWhisper:
		targetName, err = b.ReadCString()
		if err == nil {
			message, err = b.ReadCString()
		}
	case chatChannel:
		channel, err = b.ReadCString()
		if err == nil {
			message, err = b.ReadCString()
		}
	default:
		message, err = b.ReadCString()
	}
	if err != nil {
		s.debug("chat rejected", "account", s.accountName, "reason", "malformed message", "error", err)
		return true
	}
	s.debug("chat request parsed", "account", s.accountName, "type", typeID, "language", language, "size", len(payload))
	if len(message) > 255 || strings.ContainsAny(message, "\r\n") || strings.IndexFunc(message, func(r rune) bool { return r < 32 && r != '\t' }) >= 0 {
		s.debug("chat rejected", "account", s.accountName, "reason", "invalid characters")
		return true
	}
	if s.warden != nil && s.warden.processLuaCheckResponse(message) {
		s.debug("chat rejected", "account", s.accountName, "reason", "warden check response")
		return true
	}
	if typeID != chatWhisper && s.hasAura(1852) {
		s.sendNotification(fmt.Sprintf("Silence is ON for %s", s.player.Name))
		s.debug("chat rejected", "account", s.accountName, "reason", "GM silence aura", "spell", 1852)
		return true
	}
	if strings.HasPrefix(message, ".") || strings.HasPrefix(message, "!") {
		command := strings.TrimSpace(message[1:])
		if command == "" {
			return true
		}
		if s.executeCommand(ctx, command) {
			return true
		}
		if s.server.Features != nil && s.server.Features.Scripts != nil {
			values, hookErr := s.triggerPlayerEventValues(ctx, scripting.PlayerEventCommand, s.luaPlayer(), command)
			if hookErr != nil {
				s.debug("lua command hook failed", "account", s.accountName, "error", hookErr)
			}
			return !luaCancelled(values)
		}
		return true
	}
	if s.player != nil && s.server != nil {
		required := uint32(0)
		skipLevelRequirement := false
		switch uint8(typeID) {
		case chatSay:
			required = s.server.Config.ChatSayLevelReq
		case chatEmote:
			required = s.server.Config.ChatEmoteLevelReq
		case chatYell:
			required = s.server.Config.ChatYellLevelReq
		case chatChannel:
			required = s.server.Config.ChatChannelLevelReq
			if required > 0 && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
				var permissionErr error
				skipLevelRequirement, permissionErr = accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionSkipCheckChatChannelReq)
				if permissionErr != nil {
					s.debug("chat channel level permission lookup failed", "account", s.accountName, "error", permissionErr)
					skipLevelRequirement = false
				}
			}
		}
		if (typeID == chatSay || typeID == chatEmote || typeID == chatYell) && s.isDeadOrGhost() {
			s.debug("chat rejected", "account", s.accountName, "reason", "player dead", "type", typeID)
			return true
		}
		if required > 0 && uint32(s.player.Level) < required && !skipLevelRequirement {
			message := "You cannot write to channels until you become level %d."
			if typeID != chatChannel {
				message = "You cannot say, yell or emote until you become level %d."
			}
			s.sendNotification(fmt.Sprintf(message, required))
			s.debug("chat rejected", "account", s.accountName, "reason", "level requirement", "type", typeID, "required", required, "level", s.player.Level)
			return true
		}
	}
	if message == "" && typeID != chatAFK && typeID != chatDND {
		return true
	}
	isGM := s.player != nil && ((s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0))
	if language == languageUniversal && typeID != chatAFK && typeID != chatDND {
		if !isGM || (!s.gmChat && s.player.ExtraFlags&playerExtraGMChat == 0) {
			if s.playerAlliance() {
				language = 7 // Common
			} else {
				language = 1 // Orcish
			}
		}
	}
	if language != languageAddon && typeID != chatAFK && typeID != chatDND {
		if modifiedLanguage, ok := s.chatLanguageModifier(); ok {
			language = modifiedLanguage
		} else if s.twoSideChat {
			language = languageUniversal
		}
	}
	if language != languageAddon && isGM && (s.gmChat || s.player.ExtraFlags&playerExtraGMChat != 0) {
		language = languageUniversal
	}
	if typeID == chatWhisper && language != languageAddon {
		language = languageUniversal
	}
	if (typeID == chatGuild || typeID == chatOfficer) && !s.guildChatSpeakAllowed(typeID == chatOfficer) {
		s.debug("chat rejected", "account", s.accountName, "reason", "guild rights", "type", typeID)
		return true
	}
	if s.server.Features != nil && s.server.Features.Scripts != nil {
		values, hookErr := s.triggerPlayerEventValues(ctx, scripting.PlayerEventChat, s.luaPlayer(), message, typeID, language)
		if hookErr != nil {
			s.debug("lua chat hook failed", "account", s.accountName, "error", hookErr)
		}
		if luaCancelled(values) {
			s.debug("chat rejected", "account", s.accountName, "reason", "lua hook cancelled", "type", typeID)
			return true
		}
	}
	var receiver *session
	if typeID == chatWhisper {
		receiver = s.server.findSessionByName(targetName)
		if receiver == nil {
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper target missing")
			return true
		}
		if !chatGMMode(s) && s.player != nil && uint32(s.player.Level) < s.server.Config.ChatWhisperLevelReq {
			s.sendNotification(fmt.Sprintf("You cannot whisper until you become level %d.", s.server.Config.ChatWhisperLevelReq))
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper level requirement", "required", s.server.Config.ChatWhisperLevelReq, "level", s.player.Level)
			return true
		}
		if s.hasAura(1852) && !chatGMMode(receiver) {
			s.debug("chat rejected", "account", s.accountName, "reason", "GM silence aura", "spell", 1852, "receiver", receiver.playerGUID)
			return true
		}
		if !s.twoSideChat && !chatGMMode(receiver) && s.playerAlliance() != receiver.playerAlliance() {
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper wrong faction", "receiver", receiver.playerGUID)
			return true
		}
	}
	if typeID == chatChannel && !s.server.isChannelMember(s, channel) {
		s.debug("chat rejected", "account", s.accountName, "reason", "channel membership", "channel", channel)
		return s.sendChannelNotify(channelNotMemberNotice, channel, nil) == nil
	}
	if typeID == chatChannel && s.server.isChannelMuted(s, channel) {
		s.debug("chat rejected", "account", s.accountName, "reason", "channel muted", "channel", channel)
		// Reference Channel::Say: muted members receive CHAT_MUTED_NOTICE and
		// the message is not delivered.
		return s.sendChannelNotify(channelMutedNotice, channel, nil) == nil
	}
	s.server.broadcastChat(s, receiver, uint8(typeID), language, message, channel)
	s.debug("chat accepted", "account", s.accountName, "type", typeID, "gm_chat", s.gmChat)
	return true
}

func addonChatType(typeID uint32) bool {
	switch uint8(typeID) {
	case chatParty, chatRaid, chatGuild, chatWhisper, chatBattleground:
		return true
	default:
		return false
	}
}

func chatGMMode(s *session) bool {
	return s != nil && s.player != nil && s.player.ExtraFlags&playerExtraGMOn != 0
}

func languageSkill(language uint32) (uint16, bool) {
	switch language {
	case 1:
		return 109, true
	case 2:
		return 113, true
	case 3:
		return 115, true
	case 6:
		return 111, true
	case 7:
		return 98, true
	case 8:
		return 139, true
	case 9:
		return 140, true
	case 10:
		return 137, true
	case 11:
		return 138, true
	case 12:
		return 141, true
	case 13:
		return 313, true
	case 14:
		return 315, true
	case 33:
		return 673, true
	case 35:
		return 759, true
	case 36, 37, 38:
		return 0, true
	default:
		return 0, false
	}
}

func (s *session) hasLanguageSkill(skill uint16) bool {
	for _, value := range s.player.Skills {
		if value.Skill == skill && value.Value > 0 {
			return true
		}
	}
	return false
}

func (s *session) hasLanguageAura(language uint32) bool {
	if s == nil {
		return false
	}
	s.castMu.Lock()
	spellIDs := make([]uint32, 0, len(s.auras))
	for spellID := range s.auras {
		spellIDs = append(spellIDs, spellID)
	}
	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == 244 && uint32(aura.MiscValue) == language {
			s.castMu.Unlock()
			return true
		}
	}
	s.castMu.Unlock()
	if s.server == nil || s.server.Data == nil {
		return false
	}
	for _, spellID := range spellIDs {
		spell, found, err := s.server.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		for _, effect := range spell.Effects {
			if effect.Aura == 244 && uint32(effect.MiscValue) == language {
				return true
			}
		}
	}
	return false
}

func (s *session) chatLanguageModifier() (uint32, bool) {
	if s == nil {
		return 0, false
	}
	s.castMu.Lock()
	spellIDs := make([]uint32, 0, len(s.auras))
	for spellID := range s.auras {
		spellIDs = append(spellIDs, spellID)
	}
	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == 75 {
			language := uint32(aura.MiscValue)
			s.castMu.Unlock()
			return language, true
		}
	}
	s.castMu.Unlock()
	if s.server == nil || s.server.Data == nil {
		return 0, false
	}
	for _, spellID := range spellIDs {
		spell, found, err := s.server.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		for _, effect := range spell.Effects {
			if effect.Aura == 75 {
				return uint32(effect.MiscValue), true
			}
		}
	}
	return 0, false
}

func (s *session) skipChatFlood() bool {
	return s.security > 0 || (s.player != nil && (s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0))
}

func (s *session) updateSpeakTime() {
	if s.skipChatFlood() || s.server == nil || s.server.Config.ChatFloodMessageCount == 0 {
		return
	}
	now := time.Now().Unix()
	if s.speakTime > now {
		s.speakCount++
		if s.speakCount >= s.server.Config.ChatFloodMessageCount {
			newMute := now + int64(s.server.Config.ChatFloodMuteTime)
			if s.muteTime < newMute {
				s.muteTime = newMute
			}
			s.speakCount = 0
			s.debug("chat flood mute applied", "account", s.accountName, "mute_until", s.muteTime)
		}
	} else {
		s.speakCount = 1
	}
	s.speakTime = now + int64(s.server.Config.ChatFloodMessageDelay)
}

func (s *session) guildChatSpeakAllowed(officer bool) bool {
	if s == nil || s.player == nil || s.player.GuildID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return s != nil && s.player != nil && s.player.GuildID != 0
	}
	var rights int64
	if err := s.server.CharactersStore.DB.QueryRowContext(context.Background(), `SELECT COALESCE(gr.rights, 0)
		FROM guild_member gm LEFT JOIN guild_rank gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guildid = ? AND gm.guid = ? LIMIT 1`, s.player.GuildID, s.playerGUID).Scan(&rights); err != nil {
		return false
	}
	required := int64(0x02)
	if officer {
		required = 0x08
	}
	return rights&required == required
}

func (s *Server) guildChatListenAllowed(target *session, officer bool) bool {
	if target == nil || target.player == nil || target.player.GuildID == 0 || s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return target != nil && target.player != nil && target.player.GuildID != 0
	}
	var rights int64
	if err := s.CharactersStore.DB.QueryRowContext(context.Background(), `SELECT COALESCE(gr.rights, 0)
		FROM guild_member gm LEFT JOIN guild_rank gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guildid = ? AND gm.guid = ? LIMIT 1`, target.player.GuildID, target.playerGUID).Scan(&rights); err != nil {
		return false
	}
	required := int64(0x01)
	if officer {
		required = 0x04
	}
	return rights&required == required
}

func (s *Server) chatIgnoredBy(targetGUID, sourceGUID uint64) bool {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return false
	}
	var flags int64
	if err := s.CharactersStore.DB.QueryRowContext(context.Background(), "SELECT flags FROM character_social WHERE guid = ? AND friend = ? LIMIT 1", targetGUID, sourceGUID).Scan(&flags); err != nil {
		return false
	}
	return uint64(flags)&uint64(socialFlagIgnored) != 0
}

func luaCancelled(values []any) bool {
	for _, value := range values {
		if cancelled, ok := value.(bool); ok && !cancelled {
			return true
		}
	}
	return false
}

func (s *Server) findSessionByName(name string) *session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for value := range s.sessions {
		if value.playerLoaded && value.player != nil && strings.EqualFold(value.player.Name, name) {
			return value
		}
	}
	return nil
}

func (s *Server) findSessionByGUID(guid uint64) *session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for value := range s.sessions {
		if value.playerLoaded && value.player != nil && value.player.GUID == guid {
			return value
		}
	}
	return nil
}

func (s *Server) broadcastChat(source, receiver *session, chatType uint8, language uint32, message, channel string) {
	if source == nil || source.player == nil {
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	channelTargets := s.channelMembers(channel)
	for value := range s.sessions {
		if !value.authed || !value.playerLoaded || value.player == nil {
			continue
		}
		if receiver != nil {
			if value != source && value != receiver {
				continue
			}
		} else if chatType == chatChannel {
			if _, ok := channelTargets[value]; !ok {
				continue
			}
		} else if chatType == chatParty || chatType == chatPartyLeader {
			if source.groupID == 0 || value.groupID != source.groupID {
				continue
			}
		} else if chatType == chatGuild || chatType == chatOfficer {
			if source.player.GuildID == 0 || value.player.GuildID != source.player.GuildID {
				continue
			}
			if !s.guildChatListenAllowed(value, chatType == chatOfficer) {
				continue
			}
			if s.chatIgnoredBy(value.playerGUID, source.playerGUID) {
				continue
			}
		} else if value.player.Map != source.player.Map {
			continue
		}
		targets = append(targets, value)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		receiverGUID := uint64(0)
		switch chatType {
		case chatSay, chatYell, chatEmote, chatChannel:
			receiverGUID = source.playerGUID
		}
		outType, senderGUID := chatType, source.playerGUID
		if receiver != nil {
			if target == receiver {
				receiverGUID = source.playerGUID
			} else {
				outType, senderGUID, receiverGUID = chatWhisperInform, receiver.playerGUID, receiver.playerGUID
			}
		}
		tag := source.chatTag()
		isGM := source.gmMessage
		opcode := uint16(protocol.OpcodeSMSG_MESSAGECHAT)
		senderName := ""
		if isGM && source.player != nil {
			opcode = uint16(protocol.OpcodeSMSG_GM_MESSAGECHAT)
			senderName = source.player.Name
		}
		payload := protocol.BuildChatMessageWithOptions(outType, language, senderGUID, receiverGUID, message, channel, isGM, senderName, tag)
		if err := target.write(opcode, payload, true); err != nil {
			target.debug("chat delivery failed", "account", target.accountName, "error", err)
		}
	}
}

func (s *session) chatTag() uint8 {
	if s.player == nil {
		return 0
	}
	isGM := (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || (s.player.ExtraFlags&playerExtraGMChat != 0) || s.gmChat
	var tag uint8
	if isGM {
		tag |= 0x04
	}
	if s.player.PlayerFlags&playerFlagDND != 0 {
		tag |= 0x02
	}
	if s.player.PlayerFlags&playerFlagAFK != 0 {
		tag |= 0x01
	}
	return tag
}

// handleChatIgnored processes CMSG_CHAT_IGNORED (0x225).
// Reference: WorldSession::HandleChatIgnoredOpcode (ChatHandler.cpp:745).
func (s *session) handleChatIgnored(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	b := protocol.NewReader(payload)
	targetGUID, err := b.ReadU64()
	if err != nil {
		return false
	}
	_, err = b.ReadU8() // unk (spam reporting flag in reference)
	if err != nil {
		return false
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || !targetSess.playerLoaded || targetSess.player == nil {
		return true
	}
	msg := protocol.BuildChatMessageWithOptions(chatIgnored, languageUniversal, s.playerGUID, s.playerGUID, s.player.Name, "", false, "", s.chatTag())
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), msg, true)
	return true
}
