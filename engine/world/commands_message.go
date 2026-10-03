package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// message command ports message_commandscript (cs_message.cpp), the
// TWENTY-FIFTH Commands group in loader call order (AddSC_list_commandscript()
// is call 87, this is call 88). The C++ file is a single column-0 class
// ("message_commandscript"); sole-source verified (hits only cs_message.cpp +
// cs_script_loader.cpp decl 43 / call 88). Of the 8 arms, 6 are native and 2
// are documented-blocked:
//
//   - `channel set ownership` has no Go bridge at all: the C++ bottoms out in
//     the ChatChannels.dbc substring lookup, ChannelMgr::GetChannel and the
//     characters `channels` table (CHAR_UPD_CHANNEL_OWNERSHIP); the Go tree
//     has no ChatChannels.dbc accessor, no channels persist table, and no
//     ownership flag on worldChannel.
//   - `whispers` has no Go bridge at all: AcceptWhispers and the per-player
//     whisper white list have no Go model (the gm port already records the
//     SetAcceptWhispers gap).
//
// Console-vs-chat LANG branches are moot (Go commands are always sessioned,
// so the C++ "Console" name fallback is unreachable), and LANG texts are
// inlined from TDB enUS recall (no in-tree trinity_string seed), per tree
// convention.

// Inlined enUS texts (Language.h ids; no in-tree trinity_string seed).
const (
	messageAnnounceColor    = "|cffff0000[Server Announcement by |r%s|cffff0000]|r:|cffffffff %s|r" // LANG_ANNOUNCE_COLOR 787
	messageGMAnnounceColor  = "|cffff0000[Staff Announcement by |r%s|cffff0000]|r:|cffffffff %s|r"  // LANG_GM_ANNOUNCE_COLOR 6615
	messageSystemMessage    = "|cffff0000[System]|r %s"                                             // LANG_SYSTEMMESSAGE 3
	messageGMBroadcast      = "|cffff0000[Staff Announcement]|r %s"                                 // LANG_GM_BROADCAST 6613
	messageGlobalNotify     = "|cffff0000[Server Notice]: |r"                                       // LANG_GLOBAL_NOTIFY 100
	messageGMNotify         = "|cffff0000[Staff Notice]: |r"                                        // LANG_GM_NOTIFY 6614
	messageEnableOwnership  = "Ownership of channel '%s' granted."                                  // LANG_CHANNEL_ENABLE_OWNERSHIP 5022
	messageDisableOwnership = "Ownership of channel '%s' revoked."                                  // LANG_CHANNEL_DISABLE_OWNERSHIP 5023
)

// broadcastMessageChatAll sends a CHAT_MSG_SYSTEM line to every in-game
// session, mirroring sWorld->SendWorldText (which builds CHAT_MSG_SYSTEM and
// broadcasts globally).
func (s *Server) broadcastMessageChatAll(msg string) {
	if s == nil {
		return
	}
	payload := protocol.BuildSystemChatMessage(msg)
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload, true)
	}
}

// broadcastMessageChatGM mirrors sWorld->SendGMText: a CHAT_MSG_SYSTEM line
// to sessions holding RBAC_PERM_RECEIVE_GLOBAL_GM_TEXTMESSAGE (44), matching
// the sendGlobalGMMessage filter.
func (s *Server) broadcastMessageChatGM(ctx context.Context, msg string) {
	if s == nil {
		return
	}
	payload := protocol.BuildSystemChatMessage(msg)
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if !sess.commandAllowed(ctx, permissionReceiveGlobalGMTextMessage) {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload, true)
	}
}

// broadcastNotificationAll mirrors the SMSG_NOTIFICATION global loop in
// HandleNotifyCommand (cs_message.cpp:183-195).
func (s *Server) broadcastNotificationAll(msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		target.sendNotification(msg)
	}
}

// broadcastNotificationGM mirrors HandleGMNotifyCommand (cs_message.cpp:198-211):
// SMSG_NOTIFICATION to the RBAC_PERM_RECEIVE_GLOBAL_GM_TEXTMESSAGE audience.
func (s *Server) broadcastNotificationGM(ctx context.Context, msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if !sess.commandAllowed(ctx, permissionReceiveGlobalGMTextMessage) {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		target.sendNotification(msg)
	}
}

// handleCmdChannel ports the "channel set ownership" table entry
// (cs_message.cpp:47, sole arm): two-level dispatch mirroring the gobject
// "set phase"/"set state" pattern.
func (s *session) handleCmdChannel(ctx context.Context, args []string) {
	const syntax = "Syntax: .channel set ownership <channel> <on|off>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	deny := func(perm uint32) bool {
		if !s.commandAllowed(ctx, perm) {
			s.sendNotification("You do not have permission to use that command.")
			return true
		}
		return false
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "set":
		if len(args) < 2 {
			s.sendSysMessage(syntax)
			return
		}
		switch lvl := strings.ToLower(args[1]); {
		case strings.HasPrefix("ownership", lvl):
			if deny(permissionCommandChannelSetOwnership) {
				return
			}
			s.handleChannelSetOwnershipCommand(args[2:])
		default:
			s.sendSysMessage(syntax)
		}
	default:
		s.sendSysMessage(syntax)
	}
}

// handleChannelSetOwnershipCommand is the documented-blocked channel set
// ownership arm (cs_message.cpp:76-121, RBAC 465): it bottoms out in the
// ChatChannels.dbc substring lookup, ChannelMgr::GetChannel and the
// characters `channels` table (CHAR_UPD_CHANNEL_OWNERSHIP), none of which has
// a Go model. RBAC-gated with an honest message, not a stub.
func (s *session) handleChannelSetOwnershipCommand(args []string) {
	s.sendSysMessage("Channel ownership control is unavailable: the Go tree has no channels persist table or ChatChannels.dbc lookup model.")
}

// messageDeny reports a per-arm RBAC denial the tree-standard way.
func (s *session) messageDeny(ctx context.Context, perm uint32) bool {
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}
	return false
}

// messageTailGate applies the shared gates of the six broadcast arms
// (cs_message.cpp:123-211): non-empty tail, in-game player, per-arm RBAC.
func (s *session) messageTailGate(ctx context.Context, syntax string, perm uint32, args []string) (string, bool) {
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return "", false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return "", false
	}
	if s.messageDeny(ctx, perm) {
		return "", false
	}
	return strings.Join(args, " "), true
}

// handleCmdNameAnnounceCommand mirrors HandleNameAnnounceCommand
// (cs_message.cpp:123-135, RBAC 469): the announcer's name in
// LANG_ANNOUNCE_COLOR, broadcast to everyone. The C++ "Console" fallback is
// unreachable: Go commands are always sessioned.
func (s *session) handleCmdNameAnnounceCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .nameannounce <text>", permissionCommandNameAnnounce, args)
	if !ok {
		return
	}
	s.server.broadcastMessageChatAll(fmt.Sprintf(messageAnnounceColor, s.player.Name, msg))
}

// handleCmdGMNameAnnounceCommand mirrors HandleGMNameAnnounceCommand
// (cs_message.cpp:137-149, RBAC 467): LANG_GM_ANNOUNCE_COLOR to GMs only.
func (s *session) handleCmdGMNameAnnounceCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .gmnameannounce <text>", permissionCommandGMNameAnnounce, args)
	if !ok {
		return
	}
	s.server.broadcastMessageChatGM(ctx, fmt.Sprintf(messageGMAnnounceColor, s.player.Name, msg))
}

// handleCmdAnnounceCommand mirrors HandleAnnounceCommand (cs_message.cpp:152-161,
// RBAC 462): SendServerMessage(SERVER_MSG_STRING, LANG_SYSTEMMESSAGE % text).
func (s *session) handleCmdAnnounceCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .announce <text>", permissionCommandAnnounce, args)
	if !ok {
		return
	}
	s.server.broadcastMessageChatAll(fmt.Sprintf(messageSystemMessage, msg))
}

// handleCmdGMAnnounceCommand mirrors HandleGMAnnounceCommand (cs_message.cpp:164-172,
// RBAC 466): SendGMText(LANG_GM_BROADCAST).
func (s *session) handleCmdGMAnnounceCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .gmannounce <text>", permissionCommandGMAnnounce, args)
	if !ok {
		return
	}
	s.server.broadcastMessageChatGM(ctx, fmt.Sprintf(messageGMBroadcast, msg))
}

// handleCmdNotifyCommand mirrors HandleNotifyCommand (cs_message.cpp:175-195,
// RBAC 470): SMSG_NOTIFICATION to everyone with LANG_GLOBAL_NOTIFY + text.
func (s *session) handleCmdNotifyCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .notify <text>", permissionCommandNotify, args)
	if !ok {
		return
	}
	s.server.broadcastNotificationAll(messageGlobalNotify + msg)
}

// handleCmdGMNotifyCommand mirrors HandleGMNotifyCommand (cs_message.cpp:198-211,
// RBAC 468): SMSG_NOTIFICATION to GMs with LANG_GM_NOTIFY + text.
func (s *session) handleCmdGMNotifyCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .gmnotify <text>", permissionCommandGMNotify, args)
	if !ok {
		return
	}
	s.server.broadcastNotificationGM(ctx, messageGMNotify+msg)
}

// handleCmdWhispers mirrors HandleWhispersCommand (cs_message.cpp:214-256,
// RBAC 471): the documented-blocked whispers arm. AcceptWhispers and the
// per-player whisper white list have no Go model, so the arm reports the
// missing bridge honestly after the RBAC gate, not a stub.
func (s *session) handleCmdWhispers(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.messageDeny(ctx, permissionCommandWhispers) {
		return
	}
	s.sendSysMessage("Whisper control is unavailable: the Go tree has no accept-whispers flag or whisper white list model.")
}
