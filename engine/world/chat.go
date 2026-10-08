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
	chatRaidLeader     = 0x27
	chatRaidWarning    = 0x28
	chatBattleground   = 0x2C
	chatBattleLeader   = 0x2D
	chatPartyLeader    = 0x33
	maxChatMessageType = 0x34
	languageUniversal  = uint32(0)
	languageAddon      = ^uint32(0)
)

// Default AFK/DND auto-reply messages: the enUS trinity_string defaults
// (LANG_PLAYER_AFK_DEFAULT=710 "AFK", LANG_PLAYER_DND_DEFAULT=709 "DND").
// Go carries enUS text only, so the defaults are hardcoded like other
// untranslated trinity_string references.
const (
	autoReplyAFKDefault = "AFK"
	autoReplyDNDDefault = "DND"
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
	if s.player != nil {
		s.player.Selection = selection
		s.sendPlayerUpdate()
	}
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
			// Player::Update (Player.cpp:1085-1094): expiry clears the session
			// mute AND the login row, including reason/by (LOGIN_UPD_MUTE_TIME
			// with mutetime=0, mutereason="", muteby="").
			_, _ = s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET mutetime = 0, mutereason = '', muteby = '' WHERE id = ?", s.accountID)
		}
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
	s.debug("chat packet received", "account", s.accountName, "guid", s.playerGUID, "character", s.player.Name, "type", typeID, "language", language)
	if typeID >= maxChatMessageType {
		s.debug("chat rejected", "account", s.accountName, "reason", "invalid message type", "type", typeID)
		return true
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
	// C++ (ChatHandler.cpp:231-232): messages over 255 bytes are dropped before
	// any further processing.
	if len(message) > 255 {
		s.debug("chat rejected", "account", s.accountName, "reason", "message too long")
		return true
	}
	// C++ (ChatHandler.cpp:253-271): cut at the first newline or carriage
	// return (drop when the message starts with one), then abort on nasty
	// (ASCII control, tab allowed) characters.
	if language != languageAddon {
		if pos := strings.IndexAny(message, "\r\n"); pos == 0 {
			s.debug("chat rejected", "account", s.accountName, "reason", "leading newline")
			return true
		} else if pos > 0 {
			message = message[:pos]
		}
		if strings.IndexFunc(message, func(r rune) bool { return r < 32 && r != '\t' }) >= 0 {
			s.debug("chat rejected", "account", s.accountName, "reason", "invalid characters")
			return true
		}
	}
	// C++ (ChatHandler.cpp:179-193): the mute gate, speak-time update, and
	// GM-silence aura gate all run before the warden response and command
	// parsing; addon messages are exempt from flood control. A muted player
	// therefore cannot run chat commands.
	if language != languageAddon && s.muteTime > now {
		remaining := s.muteTime - now
		if remaining < 1 {
			remaining = 1
		}
		// Reference: WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:196-202)
		// — the muted notification is LANG_WAIT_BEFORE_SPEAKING (705) with a
		// secsToTimeString ShortText duration, not raw seconds.
		s.sendNotification(fmt.Sprintf("You must wait %s before speaking again.", secsToTimeStringShort(uint64(remaining))))
		s.debug("chat rejected", "account", s.accountName, "reason", "account muted", "mute_until", s.muteTime)
		return true
	}
	if language != languageAddon && typeID != chatAFK && typeID != chatDND {
		s.updateSpeakTime()
	}
	if typeID != chatWhisper && s.hasAura(1852) {
		s.sendNotification(fmt.Sprintf("Silence is ON for %s", s.player.Name))
		s.debug("chat rejected", "account", s.accountName, "reason", "GM silence aura", "spell", 1852)
		return true
	}
	// Reference: WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:228-232) —
	// the warden Lua-check response arm runs only for guild-targeted addon
	// messages; a "_TW\t" response on any other channel is not a check
	// response.
	if typeID == chatGuild && language == languageAddon && s.warden != nil && s.warden.processLuaCheckResponse(message) {
		s.debug("chat rejected", "account", s.accountName, "reason", "warden check response")
		return true
	}
	if typeID != chatAFK && typeID != chatDND && language != languageAddon && (strings.HasPrefix(message, ".") || strings.HasPrefix(message, "!")) {
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
	if language == languageAddon && !addonChatType(typeID) {
		s.debug("chat rejected", "account", s.accountName, "reason", "invalid addon language type", "type", typeID)
		return true
	}
	if language == languageAddon && (s.server == nil || !s.server.Config.AddonChannel) {
		s.debug("chat rejected", "account", s.accountName, "reason", "addon channel disabled")
		return true
	}
	// Reference: WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:239-248) —
	// LANG_ADDON messages framed as "TrinityCore\t" are the remote admin console
	// (AddonChannelCommandHandler::ParseCommands); they run before the whisper
	// target lookup and consume the message.
	if language == languageAddon && s.parseAddonChannelCommand(ctx, message) {
		return true
	}
	languageSkillID, languageKnown := languageSkill(language)
	s.debug("chat language checked", "account", s.accountName, "language", language, "language_skill", languageSkillID, "language_known", languageKnown, "loaded_skill_count", len(s.player.Skills))
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
	// Reference: the non-addon branch of WorldSession::HandleMessagechatOpcode
	// (ChatHandler.cpp:179-181) — a player in .gm on mode sends in the
	// universal language regardless of spell effects or typed language. The
	// gmChat/.gm chat flag only feeds the SMSG_GM_MESSAGECHAT opcode and the
	// GM tag byte (ChatHandler::BuildChatPacket, Chat.cpp:271-297), never the
	// language. (The old universal-to-faction fallback block here was dead
	// code: the validation above already rejects every universal non-AFK/DND
	// message before this point.)
	if language != languageAddon && chatGMMode(s) {
		language = languageUniversal
	}
	if language != languageAddon && typeID != chatAFK && typeID != chatDND {
		if modifiedLanguage, ok := s.chatLanguageModifier(); ok {
			language = modifiedLanguage
		} else if s.twoSideChat {
			language = languageUniversal
		}
	}
	if typeID == chatWhisper && language != languageAddon {
		language = languageUniversal
	}
	if typeID == chatEmote {
		language = languageUniversal
	}
	// Reference: Channel::Say (Channel.cpp) - channel chat goes out in the
	// universal language when two-side channel interaction is on.
	if typeID == chatChannel && s.twoSideChannelInteraction() {
		language = languageUniversal
	}
	// Reference: the CHAT_MSG_GUILD/CHAT_MSG_OFFICER arms of
	// WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:425-451) — the
	// guild-presence gate wraps the chat hook, while the rank-rights gate lives
	// inside Guild::BroadcastToGuild (Guild.cpp:2135) and runs after the hook.
	// The rights check therefore moves below with the other post-hook gates.
	if (typeID == chatGuild || typeID == chatOfficer) && s.player.GuildID == 0 {
		s.debug("chat rejected", "account", s.accountName, "reason", "no guild", "type", typeID)
		return true
	}
	// Reference: WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:575-625) —
	// CHAT_MSG_AFK/CHAT_MSG_DND never broadcast a chat message; they toggle the
	// AFK/DND player flags and update the auto-reply message. The toggle runs
	// before the chat hook (mirroring OnPlayerChat-after-toggle in C++), and the
	// whole AFK arm is gated on !IsInCombat() while the DND arm has no combat gate.
	if typeID == chatAFK || typeID == chatDND {
		if typeID == chatAFK && s.isInCombat() {
			s.debug("chat rejected", "account", s.accountName, "reason", "AFK while in combat", "type", typeID)
			return true
		}
		if typeID == chatAFK {
			s.toggleChatAFK(message)
		} else {
			s.toggleChatDND(message)
		}
		s.firePlayerChatHook(ctx, typeID, language, message, targetName)
		return true
	}
	if s.firePlayerChatHook(ctx, typeID, language, message, targetName) {
		return true
	}
	var receiver *session
	if typeID == chatWhisper {
		receiver = s.server.findSessionByName(targetName)
		if receiver == nil {
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper target missing")
			s.sendChatPlayerNotFound(targetName)
			return true
		}
		// Reference: the CHAT_MSG_WHISPER arm (ChatHandler.cpp:340-394) — the
		// sender level requirement and the faction check are both skipped when
		// the receiver is a GM accepting whispers; Go has no
		// AcceptWhispers/whitelist state, so chatGMMode(receiver) is the
		// IsGameMasterAcceptingWhispers() analog. The silence-aura check runs
		// after the faction check in C++ (ChatHandler.cpp:388-393).
		if !chatGMMode(s) && !chatGMMode(receiver) && s.player != nil && uint32(s.player.Level) < s.server.Config.ChatWhisperLevelReq {
			s.sendNotification(fmt.Sprintf("You cannot whisper until you become level %d.", s.server.Config.ChatWhisperLevelReq))
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper level requirement", "required", s.server.Config.ChatWhisperLevelReq, "level", s.player.Level)
			return true
		}
		if !s.twoSideChat && !chatGMMode(receiver) && s.playerAlliance() != receiver.playerAlliance() {
			s.debug("chat rejected", "account", s.accountName, "reason", "whisper wrong faction", "receiver", receiver.playerGUID)
			s.sendChatWrongFaction()
			return true
		}
		if s.hasAura(1852) && !chatGMMode(receiver) {
			s.sendNotification(fmt.Sprintf("Silence is ON for %s", s.player.Name))
			s.debug("chat rejected", "account", s.accountName, "reason", "GM silence aura", "spell", 1852, "receiver", receiver.playerGUID)
			return true
		}
	}
	// Reference: the CHAT_MSG_PARTY/PARTY_LEADER/RAID/RAID_LEADER/RAID_WARNING
	// arms of WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:395-528)
	// and Guild::BroadcastToGuild (Guild.cpp:2133-2145) — sScriptMgr::OnPlayerChat
	// fires first (firePlayerChatHook above), then the group/guild gates silently
	// drop the message with no packet at all.
	if (typeID == chatGuild || typeID == chatOfficer) && !s.guildChatSpeakAllowed(typeID == chatOfficer) {
		s.debug("chat rejected", "account", s.accountName, "reason", "guild rights", "type", typeID)
		return true
	}
	if !s.groupChatAllowed(uint8(typeID)) {
		s.debug("chat rejected", "account", s.accountName, "reason", "group chat gate", "type", typeID)
		return true
	}
	// Reference: the CHAT_MSG_CHANNEL arm (ChatHandler.cpp:529-547) resolves the
	// channel with ChannelMgr::GetChannelForPlayerByNamePart — a
	// case-insensitive prefix over the sender's joined channels — and silently
	// drops the message when nothing matches.
	channelName := channel
	if typeID == chatChannel {
		_, resolved, ok := s.server.resolveChannelNamePart(s, channel)
		if !ok {
			s.debug("chat rejected", "account", s.accountName, "reason", "channel name not resolved", "channel", channel)
			return true
		}
		channelName = resolved
	}
	if typeID == chatChannel && !s.server.isChannelMember(s, channelName) {
		s.debug("chat rejected", "account", s.accountName, "reason", "channel membership", "channel", channelName)
		return s.sendChannelNotify(channelNotMemberNotice, channelName, nil) == nil
	}
	if typeID == chatChannel && s.server.isChannelMuted(s, channelName) {
		s.debug("chat rejected", "account", s.accountName, "reason", "channel muted", "channel", channelName)
		// Reference Channel::Say: muted members receive CHAT_MUTED_NOTICE and
		// the message is not delivered.
		return s.sendChannelNotify(channelMutedNotice, channelName, nil) == nil
	}
	s.server.broadcastChat(s, receiver, uint8(typeID), language, message, channelName)
	if typeID == chatWhisper && receiver != nil && language != languageAddon {
		s.announceChatAutoReply(receiver)
	}
	s.debug("chat accepted", "account", s.accountName, "type", typeID, "gm_chat", s.gmChat)
	return true
}

// firePlayerChatHook fires the PlayerEventChat Lua hook (the sScriptMgr::OnPlayerChat
// + Eluna OnChat analog) and reports whether a script cancelled the chat.
// Addon-language messages never reach the chat hook: C++ Eluna::OnChat routes
// LANG_ADDON to Eluna::OnAddonMessage instead (PlayerHooks.cpp:394-422), so
// they fire the addon hook below.
func (s *session) firePlayerChatHook(ctx context.Context, typeID uint32, language uint32, message string, targetName string) bool {
	if language == languageAddon {
		return s.fireAddonMessageHook(ctx, typeID, message, targetName)
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	values, hookErr := s.triggerPlayerEventValues(ctx, scripting.PlayerEventChat, s.luaPlayer(), message, typeID, language)
	if hookErr != nil {
		s.debug("lua chat hook failed", "account", s.accountName, "error", hookErr)
	}
	if luaCancelled(values) {
		s.debug("chat rejected", "account", s.accountName, "reason", "lua hook cancelled", "type", typeID)
		return true
	}
	return false
}

// fireAddonMessageHook fires the Eluna ADDON_EVENT_ON_MESSAGE server hook
// (ServerEvents 30, LuaEngine/Hooks.h:130) for LANG_ADDON chat and reports
// whether a script cancelled the message. C++ (Eluna::OnAddonMessage,
// ServerHooks.cpp:33-64) splits the message on the first '\t' into prefix
// and content (no tab: prefix is the whole message, content empty) and
// passes (event, sender, type, prefix, msg, target), where target is the
// whisper receiver's player, and nil for guild/party/raid/battleground
// targets — Go has no Lua guild/group objects, and addon messages to
// channels are rejected by addonChatType before this point. A boolean false
// from any handler cancels the message, mirroring CallAllFunctionsBool;
// unlike OnChat there is no message rewrite. For addon whispers to a
// missing player the hook does not fire at all: C++ sends the
// player-not-found notice and returns before OnChat (ChatHandler.cpp:355-359),
// and the Go whisper arm below does the same.
func (s *session) fireAddonMessageHook(ctx context.Context, typeID uint32, message string, targetName string) bool {
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	prefix := message
	// Reference: Eluna::OnAddonMessage (LuaEngine/ServerHooks.cpp:33-64) — with
	// no '\t' delimiter the content argument is pushed as Lua nil, not "".
	var content any
	if i := strings.IndexByte(message, '\t'); i >= 0 {
		prefix, content = message[:i], message[i+1:]
	}
	var target any
	if typeID == chatWhisper {
		receiver := s.server.findSessionByName(targetName)
		if receiver == nil {
			return false
		}
		if player := receiver.luaPlayer(); player != nil {
			target = player
		}
	}
	values, hookErr := s.server.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventAddonMessage, s.luaPlayer(), typeID, prefix, content, target)
	if hookErr != nil {
		s.debug("lua addon message hook failed", "account", s.accountName, "error", hookErr)
	}
	if luaCancelled(values) {
		s.debug("chat rejected", "account", s.accountName, "reason", "lua addon hook cancelled", "type", typeID)
		return true
	}
	return false
}

// parseAddonChannelCommand mirrors AddonChannelCommandHandler::ParseCommands
// (Chat.cpp:860-899): LANG_ADDON messages framed as "TrinityCore\t<op><echo4>"
// are the TrinityCore remote admin console protocol. 'p' is a ping, answered
// with an 'a' ack whisper; 'h'/'i' execute a chat command with 'o'/'f' result
// framing and sysmessage output routed through the framed whisper protocol
// (the SendSysMessage override, Chat.cpp:938-957). Like C++ this runs BEFORE
// the whisper target lookup, so it fires even when the whisper target name is
// invalid. IsHumanReadable ('h' vs 'i') has no Go analog — no command output
// varies on it — so it is parsed and ignored. Returns true when the message
// was consumed as an addon console frame.
func (s *session) parseAddonChannelCommand(ctx context.Context, msg string) bool {
	if len(msg) < 17 || !strings.HasPrefix(msg, "TrinityCore\t") {
		return false
	}
	opcode := msg[12]
	copy(s.addonCmdEcho[:], msg[13:17])
	s.addonCmdHadAck = false
	s.addonCmdFailed = false
	s.addonCmdActive = true
	defer func() { s.addonCmdActive = false }()
	switch opcode {
	case 'p': // p Ping
		s.sendAddonChannelAck()
		return true
	case 'h', 'i': // h Issue human-readable command / i Issue command
		if len(msg) <= 17 || msg[17] == 0 {
			return false
		}
		// C++ feeds the raw remainder to _ParseCommands, which requires the
		// '.'/'!' prefix (TryExecuteCommand); Go's executeCommand takes the
		// bare command, so strip one leading prefix like the chat path does.
		cmd := strings.TrimPrefix(strings.TrimPrefix(msg[17:], "."), "!")
		if s.executeCommand(ctx, cmd) {
			if !s.addonCmdHadAck {
				s.sendAddonChannelAck()
			}
			if s.addonCmdFailed {
				s.sendAddonChannelFailed()
			} else {
				s.sendAddonChannelOK()
			}
		} else if s.addonCommandNotFoundNotified(ctx) {
			// Reference: ChatHandler::_ParseCommands (Chat.cpp:153-166) —
			// unknown commands are reported only to sessions holding
			// RBAC_PERM_COMMANDS_NOTIFY_COMMAND_NOT_FOUND_ERROR; everyone
			// else pretends commands don't exist. C++ routes the
			// LANG_CMD_INVALID notice through the framed SendSysMessage and
			// then answers 'f'.
			s.sendAddonChannelSysMessage("Invalid command: " + cmd)
			s.sendAddonChannelFailed()
		}
		return true
	default:
		return false
	}
}

// addonCommandNotFoundNotified mirrors the HasPermission gate in
// ChatHandler::_ParseCommands (Chat.cpp:159), with a security-level fallback
// when the auth store is unavailable (players never see the notice).
func (s *session) addonCommandNotFoundNotified(ctx context.Context) bool {
	if s == nil {
		return false
	}
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil {
		return s.security > 0
	}
	has, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionCommandsNotifyCommandNotFoundError)
	if err != nil {
		return s.security > 0
	}
	return has
}

// sendAddonChannelReply mirrors AddonChannelCommandHandler::Send
// (Chat.cpp:902-907): a CHAT_MSG_WHISPER/LANG_ADDON packet from the player to
// themselves.
func (s *session) sendAddonChannelReply(body string) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	s.server.broadcastChat(s, s, chatWhisper, languageAddon, body, "")
}

// sendAddonChannelAck mirrors AddonChannelCommandHandler::SendAck
// (Chat.cpp:909-917): "TrinityCore\ta" + the 4 echo bytes.
func (s *session) sendAddonChannelAck() {
	s.sendAddonChannelReply("TrinityCore\ta" + string(s.addonCmdEcho[:]))
	s.addonCmdHadAck = true
}

// sendAddonChannelOK mirrors AddonChannelCommandHandler::SendOK (Chat.cpp:919-926).
func (s *session) sendAddonChannelOK() {
	s.sendAddonChannelReply("TrinityCore\to" + string(s.addonCmdEcho[:]))
}

// sendAddonChannelFailed mirrors AddonChannelCommandHandler::SendFailed
// (Chat.cpp:928-936).
func (s *session) sendAddonChannelFailed() {
	s.sendAddonChannelReply("TrinityCore\tf" + string(s.addonCmdEcho[:]))
}

// sendAddonChannelSysMessage mirrors AddonChannelCommandHandler::SendSysMessage
// (Chat.cpp:938-957): ack first if none was sent, escape '|' as '||', split on
// newlines, and send each line as "TrinityCore\tm" + echo + line.
func (s *session) sendAddonChannelSysMessage(msg string) {
	if !s.addonCmdHadAck {
		s.sendAddonChannelAck()
	}
	body := strings.ReplaceAll(msg, "|", "||")
	for _, line := range strings.Split(body, "\n") {
		s.sendAddonChannelReply("TrinityCore\tm" + string(s.addonCmdEcho[:]) + line)
	}
}

// toggleChatAFK mirrors the CHAT_MSG_AFK arm of WorldSession::HandleMessagechatOpcode
// (ChatHandler.cpp:575-601): when already AFK, an empty message removes AFK and a
// non-empty one updates the auto-reply message; when not AFK, the auto-reply is set
// (defaulting to the trinity_string 710 text) and any DND flag is cleared first.
func (s *session) toggleChatAFK(message string) {
	if s.player == nil {
		return
	}
	if s.player.PlayerFlags&playerFlagAFK != 0 {
		if message == "" {
			s.setPlayerAFK(false)
		} else {
			s.autoReplyMsg = message
		}
		return
	}
	if message == "" {
		s.autoReplyMsg = autoReplyAFKDefault
	} else {
		s.autoReplyMsg = message
	}
	if s.player.PlayerFlags&playerFlagDND != 0 {
		s.setPlayerDND(false)
	}
	s.setPlayerAFK(true)
}

// toggleChatDND mirrors the CHAT_MSG_DND arm of WorldSession::HandleMessagechatOpcode
// (ChatHandler.cpp:603-625): symmetric to toggleChatAFK with the trinity_string
// 709 default and no combat gate.
func (s *session) toggleChatDND(message string) {
	if s.player == nil {
		return
	}
	if s.player.PlayerFlags&playerFlagDND != 0 {
		if message == "" {
			s.setPlayerDND(false)
		} else {
			s.autoReplyMsg = message
		}
		return
	}
	if message == "" {
		s.autoReplyMsg = autoReplyDNDDefault
	} else {
		s.autoReplyMsg = message
	}
	if s.player.PlayerFlags&playerFlagAFK != 0 {
		s.setPlayerAFK(false)
	}
	s.setPlayerDND(true)
}

// setPlayerAFK mirrors Player::ToggleAFK (Player.cpp:1628-1635): toggles the
// PLAYER_FLAGS_AFK bit and pushes the update to the client.
func (s *session) setPlayerAFK(on bool) {
	if s.player == nil {
		return
	}
	if on {
		s.player.PlayerFlags |= playerFlagAFK
	} else {
		s.player.PlayerFlags &^= playerFlagAFK
	}
	s.sendPlayerUpdate()
}

// setPlayerDND mirrors Player::ToggleDND (Player.cpp:1637-1640).
func (s *session) setPlayerDND(on bool) {
	if s.player == nil {
		return
	}
	if on {
		s.player.PlayerFlags |= playerFlagDND
	} else {
		s.player.PlayerFlags &^= playerFlagDND
	}
	s.sendPlayerUpdate()
}

// announceChatAutoReply mirrors the AFK/DND auto-reply announcement at the end
// of Player::Whisper (Player.cpp:21050-21054): the target's auto-reply message
// is sent to the whispering player as a notification (not an addon whisper).
func (s *session) announceChatAutoReply(target *session) {
	if s == nil || target == nil || target.player == nil {
		return
	}
	if target.player.PlayerFlags&playerFlagAFK != 0 {
		// LANG_PLAYER_AFK (708).
		s.sendNotification(fmt.Sprintf("%s is AFK: %s", target.player.Name, target.autoReplyMsg))
	} else if target.player.PlayerFlags&playerFlagDND != 0 {
		// LANG_PLAYER_DND (707).
		s.sendNotification(fmt.Sprintf("%s is DND: %s", target.player.Name, target.autoReplyMsg))
	}
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
		if value.Skill == skill {
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

// groupChatAllowed mirrors the group-resolution gates of the
// CHAT_MSG_PARTY/PARTY_LEADER/RAID/RAID_LEADER/RAID_WARNING arms of
// WorldSession::HandleMessagechatOpcode (ChatHandler.cpp:395-528). Non-group
// chat types always pass; group chat silently fails (no packet) when the gates
// fail. Go has no original-group (pre-battleground) model, so the
// battleground-group exclusion applies to the session's current group; the
// original-group preference stays a documented delta, as do the
// CHAT_MSG_BATTLEGROUND/BATTLEGROUND_LEADER arms, which need the BG-group
// model that Go does not assign yet.
func (s *session) groupChatAllowed(chatType uint8) bool {
	switch chatType {
	case chatParty, chatPartyLeader, chatRaid, chatRaidLeader, chatRaidWarning:
	default:
		return true
	}
	if s == nil || s.server == nil || s.groupID == 0 {
		return false
	}
	group := s.server.findGroupByID(s.groupID)
	if group == nil {
		return false
	}
	// Reference: Group::isBGGroup — Go keys battleground groups by the
	// groupTypeBattleground bit (Group.h:213, Group.cpp).
	isBGGroup := group.GroupType&groupTypeBattleground != 0
	switch chatType {
	case chatParty, chatPartyLeader:
		// C++: without an original group, the current group must not be a
		// battleground group; party-leader chat requires the group leader.
		if isBGGroup {
			return false
		}
		return chatType != chatPartyLeader || group.isLeader(s.playerGUID)
	case chatRaid, chatRaidLeader:
		// C++: the group must be a raid group and not a battleground group;
		// raid-leader chat requires the group leader.
		if isBGGroup || !group.IsRaid {
			return false
		}
		return chatType != chatRaidLeader || group.isLeader(s.playerGUID)
	case chatRaidWarning:
		// C++: raid group (or CONFIG_CHAT_PARTY_RAID_WARNINGS), never a
		// battleground group, and the sender must be leader or assistant.
		if isBGGroup || !(group.IsRaid || s.server.Config.ChatPartyRaidWarnings) {
			return false
		}
		return group.isLeaderOrAssistant(s.playerGUID)
	}
	return false
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
		if value.worldReady.Load() && value.player != nil && strings.EqualFold(value.player.Name, name) {
			return value
		}
	}
	return nil
}

func (s *Server) findSessionByGUID(guid uint64) *session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for value := range s.sessions {
		if value.worldReady.Load() && value.player != nil && value.player.GUID == guid {
			return value
		}
	}
	return nil
}

func (s *Server) broadcastChat(source, receiver *session, chatType uint8, language uint32, message, channel string) {
	if source == nil || source.player == nil {
		return
	}
	// Reference: Guild::BroadcastToGuild (Guild.cpp:2138) — guild and officer
	// chat always go out in LANG_UNIVERSAL unless they are addon messages; the
	// sender's typed language never reaches the wire. The chat hook still sees
	// the typed language (it fires before this point in handleMessageChat).
	if (chatType == chatGuild || chatType == chatOfficer) && language != languageAddon {
		language = languageUniversal
	}
	// Party/raid chat is group-scoped (ChatHandler.cpp:395-528); hoist the
	// group lookup so the per-target filter below stays cheap.
	var chatGroup *groupState
	if chatType == chatParty || chatType == chatPartyLeader || chatType == chatRaid || chatType == chatRaidLeader || chatType == chatRaidWarning {
		chatGroup = s.findGroupByID(source.groupID)
	}
	// Reference: the CHAT_MSG_PARTY/PARTY_LEADER arm calls
	// group->BroadcastPacket(&data, false, group->GetMemberGroup(senderGUID)),
	// so party chat reaches only the sender's own subgroup (a 5-man party is
	// all subgroup 0, so delivery there is unchanged).
	var senderSubGroup uint8
	senderSubGroupKnown := false
	if chatGroup != nil && (chatType == chatParty || chatType == chatPartyLeader) {
		senderSubGroup, senderSubGroupKnown = chatGroup.memberSubGroup(source.playerGUID)
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	channelTargets := s.channelMembers(source, channel)
	for value := range s.sessions {
		if !value.authed || !value.worldReady.Load() || value.player == nil {
			continue
		}
		if receiver != nil {
			if language == languageAddon {
				if value != receiver {
					continue
				}
			} else if value != source && value != receiver {
				continue
			}
		} else if chatType == chatChannel {
			if _, ok := channelTargets[value]; !ok {
				continue
			}
			// Reference: Channel::Say (Channel.cpp) - SendToAll skips
			// listeners that ignored the speaker unless the speaker is a
			// channel moderator.
			if value != source && !s.isChannelModerator(source, channel) && s.chatIgnoredBy(value.playerGUID, source.playerGUID) {
				continue
			}
		} else if chatType == chatSay || chatType == chatYell || chatType == chatEmote {
			// Reference: Player::Say/Yell/TextEmote (Player.cpp:20975-21019) —
			// SendMessageToSetInRange with CONFIG_LISTEN_RANGE_SAY/YELL/TEXTEMOTE.
			if value.player.Map != source.player.Map || value.player.InstanceID != source.player.InstanceID {
				continue
			}
			var listenRange float64
			switch chatType {
			case chatSay:
				listenRange = s.Config.ChatListenRangeSay
			case chatYell:
				listenRange = s.Config.ChatListenRangeYell
			default:
				listenRange = s.Config.ChatListenRangeTextEmote
			}
			if listenRange > 0 && distance3D(source.player.X, source.player.Y, source.player.Z, value.player.X, value.player.Y, value.player.Z) > listenRange {
				continue
			}
			// Reference: Player::TextEmote passes ownTeamOnly =
			// !HasPermission(RBAC_PERM_TWO_SIDE_INTERACTION_CHAT).
			if chatType == chatEmote && !source.twoSideChat && value.playerAlliance() != source.playerAlliance() {
				continue
			}
		} else if chatType == chatParty || chatType == chatPartyLeader {
			if source.groupID == 0 || value.groupID != source.groupID {
				continue
			}
			if senderSubGroupKnown {
				if sub, ok := chatGroup.memberSubGroup(value.playerGUID); !ok || sub != senderSubGroup {
					continue
				}
			}
		} else if chatType == chatRaid || chatType == chatRaidLeader || chatType == chatRaidWarning {
			// Reference: the CHAT_MSG_RAID/RAID_LEADER/RAID_WARNING arms
			// broadcast to the group only (ChatHandler.cpp:470-528) — the
			// raid-group / leader / assistant gates already ran in
			// groupChatAllowed before broadcast.
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
	// Reference: Player::GetChatTag (Player.cpp:1642-1654) — CHAT_TAG_GM (0x04)
	// is stamped only when isGMChat() (PLAYER_EXTRA_GM_CHAT, the `.gm chat`
	// toggle), never for a merely visible GM (`.gm on` / PLAYER_FLAGS_GM).
	// CHAT_TAG_DEV (0x10, IsDeveloper = PLAYER_FLAGS_DEVELOPER) has no Go
	// model and is never set.
	var tag uint8
	if s.player.ExtraFlags&playerExtraGMChat != 0 || s.gmChat {
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

// sendChatPlayerNotFound mirrors WorldSession::SendPlayerNotFoundNotice
// (ChatHandler.cpp:768): SMSG_CHAT_PLAYER_NOT_FOUND carrying the name.
func (s *session) sendChatPlayerNotFound(name string) {
	buf := protocol.NewBuffer(len(name) + 1)
	buf.WriteCString(name)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAT_PLAYER_NOT_FOUND), buf.Bytes(), true)
}

// sendChatWrongFaction mirrors WorldSession::SendWrongFactionNotice
// (ChatHandler.cpp:781): empty SMSG_CHAT_WRONG_FACTION.
func (s *session) sendChatWrongFaction() {
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAT_WRONG_FACTION), []byte{}, true)
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
	if targetSess == nil || !targetSess.worldReady.Load() || targetSess.player == nil {
		return true
	}
	msg := protocol.BuildChatMessageWithOptions(chatIgnored, languageUniversal, s.playerGUID, s.playerGUID, s.player.Name, "", false, "", s.chatTag())
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), msg, true)
	return true
}
