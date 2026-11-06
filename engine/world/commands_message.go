package world

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// message command ports message_commandscript (cs_message.cpp), the
// TWENTY-FIFTH Commands group in loader call order (AddSC_list_commandscript()
// is call 87, this is call 88). The C++ file is a single column-0 class
// ("message_commandscript"); sole-source verified (hits only cs_message.cpp +
// cs_script_loader.cpp decl 43 / call 88). Of the 8 arms, 6 are native and 2
// are documented-blocked:
//
//   - `channel set ownership` is ported: the ChatChannels.dbc + AreaTable.dbc
//     substring scans, the pkt=false GetChannel lookup (builtin by
//     (channelId, zoneId), custom by lowercased name, never creating), the
//     live _ownershipEnabled flip, the CHAR_UPD_CHANNEL_OWNERSHIP row update
//     and the LANG enable/disable line all land in
//     handleChannelSetOwnershipCommand.
//   - `whispers` is half-ported: the accept-whispers flag has a Go model
//     (playerExtraAcceptWhispers, persisted via persistExtraFlags, Lua
//     SetAcceptWhispers live), so the status/on/off arms are ported; the
//     `remove` sub-arm stays blocked — the per-player whisper white list
//     has no Go model, so there is nothing to remove a name from.
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

// splitWorldText mirrors WorldWorldTextBuilder (World.cpp:2640-2672): the
// formatted text is truncated to 2047 chars (vsnprintf(str, 2048, ...))
// and split on '\n' with strtok semantics — consecutive, leading, and
// trailing newlines produce no empty lines — so SendWorldText/SendGMText
// emit one CHAT_MSG_SYSTEM packet per line.
func splitWorldText(msg string) []string {
	if len(msg) > 2047 {
		msg = msg[:2047]
	}
	var lines []string
	for _, line := range strings.Split(msg, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// broadcastMessageChatAll sends a CHAT_MSG_SYSTEM line to every in-game
// session, mirroring sWorld->SendWorldText (which builds CHAT_MSG_SYSTEM and
// broadcasts globally).
func (s *Server) broadcastMessageChatAll(msg string) {
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
	for _, line := range splitWorldText(msg) {
		payload := protocol.BuildSystemChatMessage(line)
		for _, target := range targets {
			_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload, true)
		}
	}
}

// broadcastToTeam mirrors sWorld->SendGlobalMessage(&data, nullptr, team): a
// packet to every in-game session of the given team (0 = Alliance, 1 = Horde).
func (s *Server) broadcastToTeam(opcode uint16, payload []byte, team uint32) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil && teamForRace(sess.player.Race) == team {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		_ = target.write(opcode, payload, true)
	}
}

// broadcastMessageChatGM mirrors sWorld->SendGMText: a CHAT_MSG_SYSTEM line
// to sessions holding RBAC_PERM_RECEIVE_GLOBAL_GM_TEXTMESSAGE (44), matching
// the sendGlobalGMMessage filter.
func (s *Server) broadcastMessageChatGM(ctx context.Context, msg string) {
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
	for _, line := range splitWorldText(msg) {
		payload := protocol.BuildSystemChatMessage(line)
		for _, target := range targets {
			_ = target.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), payload, true)
		}
	}
}

// broadcastServerMessageAll mirrors World::SendServerMessage
// (World.cpp:3074) with player == nullptr: the SMSG_CHAT_SERVER_MESSAGE
// packet (ChatPackets.cpp:33-39) goes to every in-game session, ==
// SendGlobalMessage's player-in-world filter (World.cpp:2590-2606).
func (s *Server) broadcastServerMessageAll(messageID int32, stringParam string) {
	if s == nil {
		return
	}
	payload := protocol.BuildChatServerMessage(messageID, stringParam)
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		_ = target.write(uint16(protocol.OpcodeSMSG_CHAT_SERVER_MESSAGE), payload, true)
	}
}

// broadcastZoneUnderAttack mirrors the team-filtered arm of
// World::SendGlobalMessage (World.cpp:2584-2606) as used by
// Creature::SendZoneUnderAttackMessage (Creature.cpp:2706-2712): the
// SMSG_ZONE_UNDER_ATTACK packet (area id, uint32) goes to every in-game
// session on the opposite team of the killer.
func (s *Server) broadcastZoneUnderAttack(killerTeam uint32, areaID uint32) {
	if s == nil || areaID == 0 {
		return
	}
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, areaID)
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.worldReady.Load() && sess.player != nil && teamForRace(sess.player.Race) != killerTeam {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		_ = target.write(uint16(protocol.OpcodeSMSG_ZONE_UNDER_ATTACK), payload, true)
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
			s.handleChannelSetOwnershipCommand(ctx, args[2:])
		default:
			s.sendSysMessage(syntax)
		}
	default:
		s.sendSysMessage(syntax)
	}
}

// handleChannelSetOwnershipCommand ports HandleChannelSetOwnership
// (cs_message.cpp:62-121, RBAC 465): the ChatChannels.dbc substring scan
// yields the builtin channel id (0 = custom), the AreaTable.dbc substring
// scan yields the zone, and ChannelMgr::GetChannel(..., pkt=false) returns
// the live channel without creating one or sending a not-on packet —
// builtin by (channelId, zoneId) with GLOBAL/CITY_ONLY forcing zone 0,
// custom by lowercased name. The live channel's _ownershipEnabled flag
// flips when found; the characters `channels` row update runs
// unconditionally (CHAR_UPD_CHANNEL_OWNERSHIP: UPDATE channels SET
// ownership = ? WHERE name LIKE ? — LIKE verbatim, no team filter), and
// the LANG enable/disable line is always sent. The bool token follows
// StringTo<bool> (StringConvert.h:146-166, case-insensitive), the same set
// handleCmdWhispers accepts; anything else, or a missing arg, is syntax
// (the C++ typed dispatcher fails the command).
func (s *session) handleChannelSetOwnershipCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .channel set ownership <channel> <on|off>"
	if len(args) != 2 {
		s.sendSysMessage(syntax)
		return
	}
	channelName := args[0]
	var grant bool
	switch strings.ToLower(args[1]) {
	case "1", "y", "on", "yes", "true":
		grant = true
	case "0", "n", "off", "no", "false":
		grant = false
	default:
		s.sendSysMessage(syntax)
		return
	}
	// Reference: cs_message.cpp:64-91 — both scans take the FIRST row, in
	// ascending id order, whose template/area name contains the channel
	// name case-insensitively (StringContainsStringI); unmatched scans
	// leave channelId 0 / zoneEntry null.
	var channelID uint32
	var dbcTemplate string
	var dbcFlags uint32
	var zoneName string
	zoneFound := false
	if s.server != nil && s.server.Data != nil {
		if entries, err := s.server.Data.ChatChannels(); err == nil {
			for _, e := range entries {
				if goContainsFold(e.Name, channelName) {
					channelID, dbcTemplate, dbcFlags = e.ID, e.Name, e.Flags
					break
				}
			}
		}
		if areas, err := s.server.Data.AreaNames(); err == nil {
			for _, a := range areas {
				if goContainsFold(a.Name, channelName) {
					zoneName, zoneFound = a.Name, true
					break
				}
			}
		}
	}
	// Reference: ChannelMgr::GetChannel (ChannelMgr.cpp:204-249) with
	// pkt=false — the live channel only, never created. The Go channels
	// map is keyed by scoped name, so the builtin lookup reconstructs
	// the Channel::GetChannelName form (Channel.cpp:88-103: GLOBAL keeps
	// the template, CITY_ONLY formats "City", zone-dependent channels
	// format the zone name); a zone-dependent channel with no zone match
	// keys (channelId, 0), which has no Go counterpart — zone channels
	// always carry " - Zone" in name and key — so no live channel flips,
	// exactly like C++ finding no (channelId, 0) entry.
	if s.server != nil {
		s.server.channelsMu.Lock()
		var channel *worldChannel
		if channelID != 0 {
			var name string
			switch {
			case dbcFlags&channelDBCFlagGlobal != 0:
				name = dbcTemplate
			case dbcFlags&channelDBCFlagCityOnly != 0:
				name = fmt.Sprintf(dbcTemplate, channelCityName)
			case zoneFound:
				name = fmt.Sprintf(dbcTemplate, zoneName)
			}
			if name != "" {
				if ch := s.server.channels[s.scopedChannelKey(name)]; ch != nil && ch.ID == channelID {
					channel = ch
				}
			}
		} else if ch := s.server.channels[s.scopedChannelKey(channelName)]; ch != nil && ch.Flags&channelFlagCustom != 0 {
			channel = ch
		}
		if channel != nil {
			channel.OwnershipEnabled = grant
		}
		s.server.channelsMu.Unlock()
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		ownership := 0
		if grant {
			ownership = 1
		}
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE channels SET ownership = ? WHERE name LIKE ?", ownership, channelName); err != nil && s.server.Logger != nil {
			s.server.Logger.Warn("channel ownership update failed", "channel", channelName, "error", err)
		}
	}
	if grant {
		s.sendSysMessage(fmt.Sprintf(messageEnableOwnership, channelName))
	} else {
		s.sendSysMessage(fmt.Sprintf(messageDisableOwnership, channelName))
	}
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
// (cs_message.cpp:123-211): per-arm RBAC first — the C++ framework's
// IsInvokerVisible gates invocation before the handler runs
// (ChatCommand.cpp:295-309), so an unauthorized user never sees the
// handler's syntax text — then non-empty tail, then in-game player. The
// tail is re-joined from the dispatcher's fields, so interior/trailing
// whitespace is collapsed; the C++ Tail is verbatim (ChatCommandTags.h:116),
// and recovering it needs raw-tail plumbing through executeCommand/
// dispatchCommand (deferred: the dispatcher tokenizes with strings.Fields).
func (s *session) messageTailGate(ctx context.Context, syntax string, perm uint32, args []string) (string, bool) {
	if s.messageDeny(ctx, perm) {
		return "", false
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return "", false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
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

// serverMessageString is the SERVER_MSG_STRING id (World.h:45-52).
const serverMessageString int32 = 3

// handleCmdAnnounceCommand mirrors HandleAnnounceCommand (cs_message.cpp:152-161,
// RBAC 462): SendServerMessage(SERVER_MSG_STRING, LANG_SYSTEMMESSAGE % text)
// emits SMSG_CHAT_SERVER_MESSAGE (World.cpp:3074), not CHAT_MSG_SYSTEM.
func (s *session) handleCmdAnnounceCommand(ctx context.Context, args []string) {
	msg, ok := s.messageTailGate(ctx, "Syntax: .announce <text>", permissionCommandAnnounce, args)
	if !ok {
		return
	}
	s.server.broadcastServerMessageAll(serverMessageString, fmt.Sprintf(messageSystemMessage, msg))
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
// RBAC 471). The no-arg status arm and the bool on/off arms are ported on
// the playerExtraAcceptWhispers flag (persisted like the .gm flag flips);
// the off arm's ClearWhisperWhiteList is vacuous — no whitelist exists in
// the Go tree. The `remove` sub-arm stays blocked on the missing whitelist
// model and reports honestly instead of pretending to remove a name; with
// no name it prints syntax (C++ returns false, cs_message.cpp:236-237). Bool
// tokens follow StringTo<bool> (StringConvert.h:146-166, case-insensitive):
// 1/y/on/yes/true and 0/n/off/no/false; anything else is LANG_USE_BOL.
func (s *session) handleCmdWhispers(ctx context.Context, args []string) {
	if s.messageDeny(ctx, permissionCommandWhispers) {
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if len(args) == 0 {
		onOff := "off" // LANG_OFF
		if s.player.ExtraFlags&playerExtraAcceptWhispers != 0 {
			onOff = "on" // LANG_ON
		}
		s.sendSysMessage(fmt.Sprintf("Whisper accepting: %s", onOff)) // LANG_COMMAND_WHISPERACCEPTING
		return
	}
	switch strings.ToLower(args[0]) {
	case "1", "y", "on", "yes", "true":
		s.player.ExtraFlags |= playerExtraAcceptWhispers
		s.persistExtraFlags()
		s.sendSysMessage("Whisper accepting on.") // LANG_COMMAND_WHISPERON
	case "0", "n", "off", "no", "false":
		s.player.ExtraFlags &^= playerExtraAcceptWhispers
		s.persistExtraFlags()
		s.sendSysMessage("Whisper accepting off.") // LANG_COMMAND_WHISPEROFF
	case "remove":
		// C++ returns false with no name (cs_message.cpp:236-237), so the
		// framework shows the leaf usage; the removal itself stays blocked
		// on the missing whitelist model.
		if len(args) < 2 {
			s.sendSysMessage("Syntax: .whispers [on|off|remove [player]]")
			return
		}
		s.sendSysMessage("Whisper white list removal is unavailable: the Go tree has no whisper white list model.")
	default:
		s.sendSysMessage("Use on/off.") // LANG_USE_BOL 259
	}
}

// worldChatRaceIcons maps race id to the Achievement_Character_* icon token
// used by Custom/cs_world_chat.cpp GetNameLink; gender picks Male/Female.
var worldChatRaceIcons = map[uint8]string{
	1: "Human", 2: "Orc", 3: "Dwarf", 4: "Nightelf", 5: "Undead",
	6: "Tauren", 7: "Gnome", 8: "Troll", 10: "Bloodelf", 11: "Draenei",
}

// worldChatClassColors maps class id to the GetNameLink class color.
var worldChatClassColors = map[uint8]string{
	1: "|cffC79C6E", 2: "|cffF58CBA", 3: "|cffABD473", 4: "|cffFFF569",
	5: "|cffFFFFFF", 6: "|cffC41F3B", 7: "|cff0070DE", 8: "|cff69CCF0",
	9: "|cff9482C9", 11: "|cffFF7D0A",
}

// worldChatNameLink mirrors Custom/cs_world_chat.cpp GetNameLink: the player
// link with the class-colored name and the race icon. Races outside the C++
// switch leave the RACE_ICON global untouched there (whatever a previous call
// left, which is unknowable); Go renders it empty, the honest equivalent.
func worldChatNameLink(name string, race, class, gender uint8) string {
	icon := ""
	if token, ok := worldChatRaceIcons[race]; ok {
		sex := "Male"
		if gender == 1 { // GENDER_FEMALE
			sex = "Female"
		}
		icon = "|TInterface/ICONS/Achievement_Character_" + token + "_" + sex + ":15|t"
	}
	return "|Hplayer:" + name + "|h|cffFFFFFF[" + worldChatClassColors[class] + name + "|r|h|cffFFFFFF] [|r" + icon + "|h|cffFFFFFF]|r"
}

// handleCmdWorldChat mirrors HandleWorldChatCommand (Custom/cs_world_chat.cpp:176-188,
// RBAC_PERM_COMMAND_SAVE 525) under its .chat/.c/.world aliases:
// empty args shows usage (C++ returns false, which shows the help text);
// the censor arm ("unixmad") reports to GMs via SendGMText(17000) and mutes
// the session one second (mutetimecensor); the played-time gate blocks
// accounts under one second of played time (Playedtimetochat) with the
// secsToTimeString remainder; the broadcast is the [World] name-link line via
// SendGlobalText (CHAT_MSG_SYSTEM to every session). The SEC_MODERATOR..
// SEC_CONSOLE switch in _SendWorldChat builds the identical message on every
// arm, so there is no branch to port. The per-command antispam mute write
// (mutetimeantispam = 0) is a session no-op in C++ (m_muteTime = now is not
// > now) and its login-DB statement binds no account id, so it is not
// ported. The FACTION_SPECIFIC branch is compiled out (FACTION_SPECIFIC 0),
// leaving the global broadcast. The 1024-char snprintf truncation is kept.
func (s *session) handleCmdWorldChat(ctx context.Context, args []string) bool {
	msg, ok := s.messageTailGate(ctx, "Syntax: .chat $message", permissionCommandSave, args)
	if !ok {
		return true
	}
	s.updatePlayedTime(time.Now())
	now := time.Now().Unix()
	if strings.Contains(msg, "unixmad") {
		// trinity_string 17000's format is seeded by the C++ file's own
		// header comment (cs_world_chat.cpp:9):
		// 'Player |cff00ff00%s|r Chat Blocked : |cffff0000%s'
		// (SendGMText(17000, name, msg), cs_world_chat.cpp:117).
		s.server.broadcastMessageChatGM(ctx, fmt.Sprintf("Player |cff00ff00%s|r Chat Blocked : |cffff0000%s", s.player.Name, msg))
		s.sendSysMessage("That word is not allowed on the server.")
		s.muteTime = now + 1
		return true
	}
	if s.player.TotalPlayedTime <= 1 {
		s.sendNotification("You must wait until you've played " + secsToTimeStringFull(uint64(1-s.player.TotalPlayedTime)) + " seconds to use world chat!")
		return true
	}
	line := "|h|cffFFFFFF[|r|cffC67171World|r|h|cffFFFFFF]|r " + worldChatNameLink(s.player.Name, s.player.Race, s.player.Class, s.player.Gender) + "|r:  " + msg + " |r"
	if len(line) > 1023 {
		line = line[:1023]
	}
	s.server.broadcastMessageChatAll(line)
	return true
}
