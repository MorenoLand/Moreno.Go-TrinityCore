package world

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	channelJoinedNotice        uint8 = 0x00
	channelLeftNotice          uint8 = 0x01
	channelYouJoinedNotice     uint8 = 0x02
	channelYouLeftNotice       uint8 = 0x03
	channelWrongPasswordNotice uint8 = 0x04
	channelNotMemberNotice     uint8 = 0x05
	channelAlreadyMemberNotice uint8 = 0x17
	channelInvalidNameNotice   uint8 = 0x1B
	channelNotInAreaNotice     uint8 = 0x20
	channelNotInLFGNotice      uint8 = 0x21
	channelFlagCustom          uint8 = 0x01
	channelFlagTrade           uint8 = 0x04
	channelFlagNotLFG          uint8 = 0x08
	channelFlagGeneral         uint8 = 0x10
	channelFlagCity            uint8 = 0x20
	channelFlagLFG             uint8 = 0x40
)

type worldChannel struct {
	ID         uint32
	Name       string
	Flags      uint8
	Password   string
	Owner      uint64
	Announce   bool
	Members    map[*session]struct{}
	Moderators map[uint64]struct{}
	Muted      map[uint64]struct{}
	Banned     map[uint64]struct{}
}

func (s *session) handleJoinChannel(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	channelID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if _, err = reader.ReadU8(); err != nil {
		return false
	}
	if _, err = reader.ReadU8(); err != nil {
		return false
	}
	name, err := reader.ReadCString()
	if err != nil {
		return false
	}
	password, err := reader.ReadCString()
	if err != nil {
		return false
	}
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return s.sendChannelNotify(channelInvalidNameNotice, name, nil) == nil
	}
	if len(name) > 31 || len(password) > 31 || strings.Contains(name, "|") {
		return true
	}
	key := s.scopedChannelKey(name)
	flags := channelFlags(channelID, name)
	// Reference: WorldSession::HandleJoinChannel (ChannelHandler.cpp:38-49) —
	// unknown built-in channel IDs are silently dropped, and
	// Player::CanJoinConstantChannelInZone (Player.cpp:5159-5171) rejects
	// zone-dependent channels (General/Trade/LocalDefense/GuildRecruitment)
	// inside arena instances, plus the guild-recruitment channel for players
	// already in a guild. Channel IDs are ChatChannels.dbc rows:
	// 1 General, 2 Trade, 3 LocalDefense, 22 WorldDefense, 23
	// GuildRecruitment, 24 LookingForGroup.
	if channelID != 0 {
		switch channelID {
		case 1, 2, 3, 22, 23, 24:
		default:
			s.debug("channel join rejected: unknown channel id", "account", s.accountName, "id", channelID)
			return true
		}
		if _, _, inArena, _ := battlegroundTypeForMap(s.player.Map); inArena &&
			(channelID == 1 || channelID == 2 || channelID == 3 || channelID == 23) {
			s.debug("channel join rejected: zone-dependent channel in arena", "account", s.accountName, "id", channelID)
			return true
		}
		if channelID == 23 && s.player.GuildID != 0 {
			s.debug("channel join rejected: guild-recruitment requires guildless", "account", s.accountName)
			return true
		}
	}
	if flags&channelFlagCity != 0 && !s.isCityZone(s.player.Zone) {
		s.debug("city channel join rejected: outside city zone", "account", s.accountName, "zone", s.player.Zone, "channel", name)
		return true
	}
	s.server.channelsMu.Lock()
	if s.server.channels == nil {
		s.server.channels = make(map[string]*worldChannel)
	}
	channel := s.server.channels[key]
	if channel == nil {
		channel = &worldChannel{
			ID:   channelID,
			Name: name,
			// Reference: Channel::Channel - custom channels announce
			// joins/leaves and hand out ownership; constant (built-in)
			// channels do neither.
			Flags:      flags,
			Password:   password,
			Owner:      s.playerGUID,
			Announce:   channelID == 0,
			Members:    make(map[*session]struct{}),
			Moderators: map[uint64]struct{}{s.playerGUID: {}},
			Muted:      make(map[uint64]struct{}),
			Banned:     make(map[uint64]struct{}),
		}
		s.server.channels[key] = channel
	}
	// Reference: Channel::JoinChannel (Channel.cpp) gate order is
	// already-member, banned, password, then the LFG restriction.
	if _, exists := channel.Members[s]; exists {
		// Reference: no error message for built-in (constant) channels.
		custom := channel.Flags&channelFlagCustom != 0
		s.server.channelsMu.Unlock()
		if !custom {
			return true
		}
		return s.sendChannelNotify(channelAlreadyMemberNotice, channel.Name, nil) == nil
	}
	if channel.Banned != nil {
		if _, banned := channel.Banned[s.playerGUID]; banned {
			s.server.channelsMu.Unlock()
			return s.sendChannelNotify(channelBannedNotice, channel.Name, nil) == nil
		}
	}
	if channel.Password != "" && channel.Password != password {
		s.server.channelsMu.Unlock()
		return s.sendChannelNotify(channelWrongPasswordNotice, channel.Name, nil) == nil
	}
	// Reference: Channel::JoinChannel - the LFG channel refuses players that
	// are in a group when Channel.RestrictedLfg is on and the account is a
	// plain player account (AccountMgr::IsPlayerAccount = security SEC_PLAYER).
	if channel.Flags&channelFlagLFG != 0 && s.server.Config.ChannelRestrictedLFG &&
		s.security == 0 && s.groupID != 0 {
		s.server.channelsMu.Unlock()
		return s.sendChannelNotify(channelNotInLFGNotice, channel.Name, nil) == nil
	}
	channel.Members[s] = struct{}{}
	if s.channels == nil {
		s.channels = make(map[string]struct{})
	}
	s.channels[key] = struct{}{}
	others := make([]*session, 0, len(channel.Members)-1)
	for member := range channel.Members {
		if member != s {
			others = append(others, member)
		}
	}
	channelName, channelFlagsValue, channelIDValue, announce := channel.Name, channel.Flags, channel.ID, channel.Announce
	s.server.channelsMu.Unlock()
	// Reference: Channel::JoinChannel (Channel.cpp) - the joined broadcast
	// is suppressed when the session holds
	// RBAC_PERM_SILENTLY_JOIN_CHANNEL.
	if announce && !s.silentlyJoinChannel() {
		for _, member := range others {
			_ = member.sendChannelNotify(channelJoinedNotice, channelName, &channelNotifyGUID{GUID: s.playerGUID})
		}
	}
	if err := s.sendChannelNotify(channelYouJoinedNotice, channelName, &channelNotifyChannel{Flags: channelFlagsValue, ID: channelIDValue}); err != nil {
		return false
	}
	s.debug("channel joined", "account", s.accountName, "channel", channelName, "id", channelIDValue)
	return true
}

func (s *session) handleLeaveChannel(payload []byte) bool {
	if !s.playerLoaded {
		return true
	}
	reader := protocol.NewReader(payload)
	channelID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	name, err := reader.ReadCString()
	if err != nil {
		return false
	}
	if channelID == 0 && name == "" {
		return true
	}
	key := s.scopedChannelKey(name)
	s.server.channelsMu.Lock()
	channel := s.server.channels[key]
	if channel == nil && channelID != 0 {
		for candidateKey, candidate := range s.server.channels {
			if candidate.ID == channelID {
				key, channel = candidateKey, candidate
				break
			}
		}
	}
	if channel == nil {
		s.server.channelsMu.Unlock()
		return s.sendChannelNotify(channelNotMemberNotice, name, nil) == nil
	}
	if _, exists := channel.Members[s]; !exists {
		s.server.channelsMu.Unlock()
		return s.sendChannelNotify(channelNotMemberNotice, channel.Name, nil) == nil
	}
	delete(channel.Members, s)
	if s.channels != nil {
		delete(s.channels, key)
	}
	others := make([]*session, 0, len(channel.Members))
	for member := range channel.Members {
		others = append(others, member)
	}
	channelName, channelFlagsValue, channelIDValue, announce := channel.Name, channel.Flags, channel.ID, channel.Announce
	// Reference: Channel::LeaveChannel - when the owner leaves a custom
	// channel with members left, the next member becomes owner+moderator.
	var newOwner *session
	var oldFlags, newFlags uint8
	if channel.Owner == s.playerGUID {
		newOwner, oldFlags, newFlags = channelTakeOwnershipLocked(channel)
	}
	if len(channel.Members) == 0 {
		delete(s.server.channels, key)
	}
	s.server.channelsMu.Unlock()
	// Reference: Channel::LeaveChannel (Channel.cpp) - the left broadcast
	// is suppressed when the session holds
	// RBAC_PERM_SILENTLY_JOIN_CHANNEL.
	if announce && !s.silentlyJoinChannel() {
		for _, member := range others {
			_ = member.sendChannelNotify(channelLeftNotice, channelName, &channelNotifyGUID{GUID: s.playerGUID})
		}
	}
	if newOwner != nil {
		for _, member := range others {
			_ = member.sendChannelNotify(channelModeChangeNotice, channelName, &channelNotifyModeChange{GUID: newOwner.playerGUID, OldFlags: oldFlags, NewFlags: newFlags})
			_ = member.sendChannelNotify(channelOwnerChangedNotice, channelName, &channelNotifyGUID{GUID: newOwner.playerGUID})
		}
	}
	if err := s.sendChannelNotify(channelYouLeftNotice, channelName, &channelNotifyChannel{Flags: channelFlagsValue, ID: channelIDValue}); err != nil {
		return false
	}
	s.debug("channel left", "account", s.accountName, "channel", channelName, "id", channelIDValue)
	return true
}

func (s *session) handleChannelList(payload []byte) bool {
	if !s.playerLoaded {
		return true
	}
	reader := protocol.NewReader(payload)
	name, err := reader.ReadCString()
	if err != nil {
		return false
	}
	key := s.scopedChannelKey(name)
	s.server.channelsMu.RLock()
	channel := s.server.channels[key]
	if channel == nil {
		s.server.channelsMu.RUnlock()
		return true
	}
	type member struct {
		guid  uint64
		flags uint8
	}
	members := make([]member, 0, len(channel.Members))
	for session := range channel.Members {
		if session.worldReady.Load() && session.player != nil {
			// Reference: Channel::List writes each member's owner/moderator/
			// muted flags; the GM see-all/visibility filter has no Go model.
			members = append(members, member{guid: session.playerGUID, flags: channel.memberFlags(session.playerGUID)})
		}
	}
	channelName, channelFlagsValue := channel.Name, channel.Flags
	s.server.channelsMu.RUnlock()
	sort.Slice(members, func(i, j int) bool { return members[i].guid < members[j].guid })
	packet := protocol.NewBuffer(64 + len(members)*9)
	packet.WriteU8(1)
	packet.WriteCString(channelName)
	packet.WriteU8(channelFlagsValue)
	packet.WriteU32(uint32(len(members)))
	for _, member := range members {
		packet.WriteU64(member.guid)
		packet.WriteU8(member.flags)
	}
	return s.write(uint16(protocol.OpcodeSMSG_CHANNEL_LIST), packet.Bytes(), true) == nil
}

func (s *session) sendChannelNotify(notice uint8, name string, extra any) error {
	return s.write(uint16(protocol.OpcodeSMSG_CHANNEL_NOTIFY), buildChannelNotify(notice, name, extra), true)
}

type channelNotifyGUID struct{ GUID uint64 }

type channelNotifyChannel struct {
	Flags uint8
	ID    uint32
}

func buildChannelNotify(notice uint8, name string, extra any) []byte {
	packet := protocol.NewBuffer(48)
	packet.WriteU8(notice)
	packet.WriteCString(name)
	switch value := extra.(type) {
	case *channelNotifyGUID:
		packet.WriteU64(value.GUID)
	case *channelNotifyName:
		packet.WriteCString(value.Name)
	case *channelNotifyModeChange:
		packet.WriteU64(value.GUID)
		packet.WriteU8(value.OldFlags)
		packet.WriteU8(value.NewFlags)
	case *channelNotifyTwoGUID:
		packet.WriteU64(value.Victim)
		packet.WriteU64(value.Moderator)
	case *channelNotifyChannel:
		if notice == channelYouLeftNotice {
			packet.WriteU32(value.ID)
			if value.ID != 0 {
				packet.WriteU8(1)
			} else {
				packet.WriteU8(0)
			}
		} else {
			packet.WriteU8(value.Flags)
			packet.WriteU32(value.ID)
			packet.WriteU32(0)
		}
	}
	return packet.Bytes()
}

func (s *Server) channelMembers(member *session, name string) map[*session]struct{} {
	key := member.scopedChannelKey(name)
	s.channelsMu.RLock()
	channel := s.channels[key]
	result := make(map[*session]struct{})
	if channel != nil {
		for member := range channel.Members {
			result[member] = struct{}{}
		}
	}
	s.channelsMu.RUnlock()
	return result
}

func (s *Server) isChannelMember(member *session, name string) bool {
	key := member.scopedChannelKey(name)
	s.channelsMu.RLock()
	channel := s.channels[key]
	ok := false
	if channel != nil {
		_, ok = channel.Members[member]
	}
	s.channelsMu.RUnlock()
	return ok
}

// isChannelMuted reports whether the speaker carries the channel mute flag.
func (s *Server) isChannelMuted(member *session, name string) bool {
	key := member.scopedChannelKey(name)
	s.channelsMu.RLock()
	channel := s.channels[key]
	muted := false
	if channel != nil {
		_, muted = channel.Muted[member.playerGUID]
	}
	s.channelsMu.RUnlock()
	return muted
}

// isChannelModerator reports whether the session is a moderator of the named
// channel (the Channel::Say moderator arm needs the speaker's flag).
func (s *Server) isChannelModerator(member *session, name string) bool {
	key := member.scopedChannelKey(name)
	s.channelsMu.RLock()
	channel := s.channels[key]
	moderator := false
	if channel != nil {
		moderator = channel.isModerator(member.playerGUID)
	}
	s.channelsMu.RUnlock()
	return moderator
}

// resolveChannelNamePart mirrors ChannelMgr::GetChannelForPlayerByNamePart
// (ChannelMgr.cpp:124-142): the channel name carried by a channel-chat packet
// is a case-insensitive prefix matched against the sender's joined channels,
// not an exact key lookup. It returns the canonical channel name and key of
// the first match (deterministic by name; C++ iterates an arbitrary-order
// pointer set).
func (s *Server) resolveChannelNamePart(member *session, namePart string) (string, string, bool) {
	if s == nil || member == nil || member.channels == nil {
		return "", "", false
	}
	part := strings.ToLower(strings.TrimSpace(namePart))
	if part == "" {
		return "", "", false
	}
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()
	bestKey, bestName := "", ""
	for key := range member.channels {
		ch := s.channels[key]
		if ch == nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(ch.Name), part) {
			if bestKey == "" || ch.Name < bestName {
				bestKey, bestName = key, ch.Name
			}
		}
	}
	if bestKey == "" {
		return "", "", false
	}
	return bestKey, bestName, true
}

// channelTakeOwnershipLocked hands a custom channel to its next member when the
// owner leaves, mirroring Channel::LeaveChannel: the first remaining member
// becomes owner and moderator. C++ iterates its member map (arbitrary order)
// preferring a visible member; Go has no invisibility model, so the lowest
// GUID wins for determinism. The caller must hold channelsMu. It returns the
// new owner session plus the old/new member flags for the mode-change
// broadcast, or nil when no transfer applies.
func channelTakeOwnershipLocked(ch *worldChannel) (newOwner *session, oldFlags, newFlags uint8) {
	if ch.Flags&channelFlagCustom == 0 || len(ch.Members) == 0 {
		return nil, 0, 0
	}
	for m := range ch.Members {
		if newOwner == nil || m.playerGUID < newOwner.playerGUID {
			newOwner = m
		}
	}
	oldFlags = ch.memberFlags(newOwner.playerGUID)
	ch.Owner = newOwner.playerGUID
	ch.Moderators[newOwner.playerGUID] = struct{}{}
	return newOwner, oldFlags, ch.memberFlags(newOwner.playerGUID)
}

func channelKey(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if separator := strings.Index(key, " - "); separator >= 0 {
		base := strings.ReplaceAll(strings.TrimSpace(key[:separator]), " ", "")
		switch base {
		case "trade", "guildrecruitment", "lookingforgroup":
			return strings.TrimSpace(key[:separator])
		}
	}
	return key
}

func (s *session) scopedChannelKey(name string) string {
	key := channelKey(name)
	if s == nil || s.player == nil || s.twoSideChannelInteraction() {
		return key
	}
	return strconv.FormatUint(uint64(playerTeam(s.player.Race)), 10) + ":" + key
}

func channelFlags(id uint32, name string) uint8 {
	if id == 0 {
		return channelFlagCustom
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if separator := strings.Index(name, " - "); separator >= 0 {
		name = name[:separator]
	}
	switch strings.ReplaceAll(name, " ", "") {
	case "trade":
		return channelFlagGeneral | channelFlagNotLFG | channelFlagTrade | channelFlagCity
	case "lookingforgroup":
		return channelFlagGeneral | channelFlagLFG | channelFlagCity
	case "guildrecruitment":
		return channelFlagGeneral | channelFlagNotLFG | channelFlagCity
	default:
		return channelFlagGeneral | channelFlagNotLFG
	}
}

func (s *Server) removeSessionChannels(member *session) {
	type departure struct {
		name     string
		announce bool
		members  []*session
		newOwner *session
		oldFlags uint8
		newFlags uint8
	}
	departures := make([]departure, 0)
	s.channelsMu.Lock()
	if s.channels == nil {
		s.channelsMu.Unlock()
		member.channels = nil
		return
	}
	for key, channel := range s.channels {
		if _, ok := channel.Members[member]; !ok {
			continue
		}
		delete(channel.Members, member)
		// Reference: Player::CleanupChannels calls LeaveChannel(send=false) -
		// the leaver gets no packet but the remaining members still see the
		// announce and the owner hand-off runs.
		var newOwner *session
		var oldFlags, newFlags uint8
		if channel.Owner == member.playerGUID {
			newOwner, oldFlags, newFlags = channelTakeOwnershipLocked(channel)
		}
		members := make([]*session, 0, len(channel.Members))
		for other := range channel.Members {
			members = append(members, other)
		}
		departures = append(departures, departure{name: channel.Name, announce: channel.Announce, members: members, newOwner: newOwner, oldFlags: oldFlags, newFlags: newFlags})
		if len(channel.Members) == 0 {
			delete(s.channels, key)
		}
	}
	s.channelsMu.Unlock()
	member.channels = nil
	for _, left := range departures {
		for _, other := range left.members {
			if left.announce {
				_ = other.sendChannelNotify(channelLeftNotice, left.name, &channelNotifyGUID{GUID: member.playerGUID})
			}
			if left.newOwner != nil {
				_ = other.sendChannelNotify(channelModeChangeNotice, left.name, &channelNotifyModeChange{GUID: left.newOwner.playerGUID, OldFlags: left.oldFlags, NewFlags: left.newFlags})
				_ = other.sendChannelNotify(channelOwnerChangedNotice, left.name, &channelNotifyGUID{GUID: left.newOwner.playerGUID})
			}
		}
	}
}

func (s *session) isCityZone(zone uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	area, found, err := s.server.Data.Area(zone)
	if err != nil || !found {
		return false
	}
	return area.Flags&wotlk.AreaFlagSlaveCapital != 0
}

func (s *session) updateLocalChannels(newZone uint32) {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return
	}
	s.player.Zone = newZone
	if !s.isCityZone(newZone) {
		// Player left city: remove from all city-only channels (Trade, GuildRecruitment)
		type departure struct {
			name     string
			flags    uint8
			id       uint32
			members  []*session
			newOwner *session
			oldFlags uint8
			newFlags uint8
		}
		departures := make([]departure, 0)
		s.server.channelsMu.Lock()
		if s.channels == nil || s.server.channels == nil {
			s.server.channelsMu.Unlock()
			return
		}
		for key := range s.channels {
			if ch := s.server.channels[key]; ch != nil && ch.Flags&channelFlagCity != 0 {
				delete(ch.Members, s)
				delete(s.channels, key)
				var newOwner *session
				var oldFlags, newFlags uint8
				if ch.Owner == s.playerGUID {
					newOwner, oldFlags, newFlags = channelTakeOwnershipLocked(ch)
				}
				members := make([]*session, 0, len(ch.Members))
				for other := range ch.Members {
					members = append(members, other)
				}
				departures = append(departures, departure{name: ch.Name, flags: ch.Flags, id: ch.ID, members: members, newOwner: newOwner, oldFlags: oldFlags, newFlags: newFlags})
				if len(ch.Members) == 0 {
					delete(s.server.channels, key)
				}
			}
		}
		s.server.channelsMu.Unlock()
		for _, left := range departures {
			_ = s.sendChannelNotify(channelYouLeftNotice, left.name, &channelNotifyChannel{Flags: left.flags, ID: left.id})
			for _, other := range left.members {
				if left.newOwner != nil {
					_ = other.sendChannelNotify(channelModeChangeNotice, left.name, &channelNotifyModeChange{GUID: left.newOwner.playerGUID, OldFlags: left.oldFlags, NewFlags: left.newFlags})
					_ = other.sendChannelNotify(channelOwnerChangedNotice, left.name, &channelNotifyGUID{GUID: left.newOwner.playerGUID})
				}
			}
		}
	}
}

// Channel command family, mirroring TrinityCore Channel.cpp/ChannelHandler.cpp
// at reference commit dcdbc0c5. Notification codes and payload layouts follow
// Channel.h (ChatNotify enum) and ChannelAppenders.h exactly: two-GUID notices
// carry the victim first, then the acting moderator.

const (
	channelNotModeratorNotice     uint8 = 0x06 // CHAT_NOT_MODERATOR_NOTICE
	channelPasswordChangedNotice  uint8 = 0x07 // CHAT_PASSWORD_CHANGED_NOTICE
	channelOwnerChangedNotice     uint8 = 0x08 // CHAT_OWNER_CHANGED_NOTICE
	channelPlayerNotFoundNotice   uint8 = 0x09 // CHAT_PLAYER_NOT_FOUND_NOTICE
	channelNotOwnerNotice         uint8 = 0x0A // CHAT_NOT_OWNER_NOTICE
	channelChannelOwnerNotice     uint8 = 0x0B // CHAT_CHANNEL_OWNER_NOTICE
	channelModeChangeNotice       uint8 = 0x0C // CHAT_MODE_CHANGE_NOTICE
	channelAnnouncementsOnNotice  uint8 = 0x0D // CHAT_ANNOUNCEMENTS_ON_NOTICE
	channelAnnouncementsOffNotice uint8 = 0x0E // CHAT_ANNOUNCEMENTS_OFF_NOTICE
	channelMutedNotice            uint8 = 0x11 // CHAT_MUTED_NOTICE
	channelPlayerKickedNotice     uint8 = 0x12 // CHAT_PLAYER_KICKED_NOTICE
	channelBannedNotice           uint8 = 0x13 // CHAT_BANNED_NOTICE
	channelPlayerBannedNotice     uint8 = 0x14 // CHAT_PLAYER_BANNED_NOTICE
	channelPlayerUnbannedNotice   uint8 = 0x15 // CHAT_PLAYER_UNBANNED_NOTICE
	channelPlayerNotBannedNotice  uint8 = 0x16 // CHAT_PLAYER_NOT_BANNED_NOTICE
	channelInviteNotice           uint8 = 0x18 // CHAT_INVITE_NOTICE
	channelInviteWrongFactionNot  uint8 = 0x19 // CHAT_INVITE_WRONG_FACTION_NOTICE
	channelPlayerInvitedNotice    uint8 = 0x1D // CHAT_PLAYER_INVITED_NOTICE
	channelPlayerInviteBannedNot  uint8 = 0x1E // CHAT_PLAYER_INVITE_BANNED_NOTICE
	channelVoiceOnNotice          uint8 = 0x22 // CHAT_VOICE_ON_NOTICE
	channelVoiceOffNotice         uint8 = 0x23 // CHAT_VOICE_OFF_NOTICE
)

// Channel member flags (Channel.h ChannelMemberFlags).
const (
	channelMemberFlagOwner     uint8 = 0x01
	channelMemberFlagModerator uint8 = 0x02
	channelMemberFlagVoiced    uint8 = 0x04
	channelMemberFlagMuted     uint8 = 0x08
)

type channelNotifyName struct{ Name string }

// channelNotifyModeChange mirrors ModeChangeAppend: GUID + old flags + new flags.
type channelNotifyModeChange struct {
	GUID     uint64
	OldFlags uint8
	NewFlags uint8
}

// channelNotifyTwoGUID is the PlayerKicked/PlayerBanned/PlayerUnbanned layout:
// victim GUID first, acting moderator second.
type channelNotifyTwoGUID struct{ Victim, Moderator uint64 }

func (c *worldChannel) memberFlags(guid uint64) uint8 {
	flags := uint8(0)
	if c.Owner == guid {
		flags |= channelMemberFlagOwner
	}
	if _, ok := c.Moderators[guid]; ok {
		flags |= channelMemberFlagModerator
	}
	if _, ok := c.Muted[guid]; ok {
		flags |= channelMemberFlagMuted
	}
	return flags
}

func (c *worldChannel) isModerator(guid uint64) bool {
	_, ok := c.Moderators[guid]
	return ok
}

func (c *worldChannel) findMemberByName(name string) *session {
	if name == "" {
		return nil
	}
	for m := range c.Members {
		if m.player != nil && strings.EqualFold(m.player.Name, name) {
			return m
		}
	}
	return nil
}

// channelCommandGuard applies the two guard steps every Channel.cpp command
// runs: the sender must be on the channel (CHAT_NOT_MEMBER_NOTICE) and must be
// a moderator (CHAT_NOT_MODERATOR_NOTICE) unless they hold
// RBAC_PERM_CHANGE_CHANNEL_NOT_MODERATOR.
func (s *session) channelCommandGuard(name string) (*worldChannel, bool) {
	s.server.channelsMu.RLock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.RUnlock()
		return nil, false
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.RUnlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return nil, false
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.RUnlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return nil, false
	}
	return ch, true
}

// channelMembersSnapshot copies the member set under lock for notifications.
func (s *Server) channelMembersSnapshot(ch *worldChannel) []*session {
	members := make([]*session, 0, len(ch.Members))
	for m := range ch.Members {
		members = append(members, m)
	}
	return members
}

// readChannelCommand parses the common "<channel>\0[<target>\0]" payload.
func readChannelCommand(payload []byte) (string, string, bool) {
	r := protocol.NewReader(payload)
	name, err := r.ReadCString()
	if err != nil {
		return "", "", false
	}
	target, _ := r.ReadCString()
	return name, target, true
}

// handleChannelPassword processes CMSG_CHANNEL_PASSWORD (0x09C).
// Reference: ChannelHandler.cpp HandleChannelPassword -> Channel::Password.
func (s *session) handleChannelPassword(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, password, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	// Reference: Channel::Password - the sender must be on the channel
	// (CHAT_NOT_MEMBER_NOTICE) and a moderator (CHAT_NOT_MODERATOR_NOTICE),
	// unless they hold RBAC_PERM_CHANGE_CHANNEL_NOT_MODERATOR.
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return true
	}
	ch.Password = password
	members := s.server.channelMembersSnapshot(ch)
	s.server.channelsMu.Unlock()
	for _, m := range members {
		_ = m.sendChannelNotify(channelPasswordChangedNotice, ch.Name, &channelNotifyGUID{GUID: s.playerGUID})
	}
	return true
}

// handleChannelSetOwner processes CMSG_CHANNEL_SET_OWNER (0x09D).
// Reference: Channel::SetOwner(player, newname) -> Channel::SetOwner(guid, true):
// the sender must be the owner (CHAT_NOT_OWNER_NOTICE) unless they hold
// RBAC_PERM_CHANGE_CHANNEL_NOT_MODERATOR, the target must be on the channel
// (cross-team targets are reported as not found unless both sides hold
// RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL), and the new owner gains moderator
// and owner flags while everyone sees a mode change broadcast followed by
// the owner-changed broadcast.
func (s *session) handleChannelSetOwner(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, targetName, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if ch.Owner != s.playerGUID && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotOwnerNotice, name, nil)
		return true
	}
	target := ch.findMemberByName(targetName)
	if target == nil {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	// Reference: cross-team targets are reported as not found unless both
	// sides hold RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL.
	if playerTeam(s.player.Race) != playerTeam(target.player.Race) &&
		!(s.twoSideChannelInteraction() && target.twoSideChannelInteraction()) {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	oldFlags := ch.memberFlags(target.playerGUID)
	ch.Owner = target.playerGUID
	ch.Moderators[target.playerGUID] = struct{}{}
	newFlags := ch.memberFlags(target.playerGUID)
	members := s.server.channelMembersSnapshot(ch)
	s.server.channelsMu.Unlock()
	for _, m := range members {
		_ = m.sendChannelNotify(channelModeChangeNotice, ch.Name, &channelNotifyModeChange{GUID: target.playerGUID, OldFlags: oldFlags, NewFlags: newFlags})
		_ = m.sendChannelNotify(channelOwnerChangedNotice, ch.Name, &channelNotifyGUID{GUID: target.playerGUID})
	}
	return true
}

// handleChannelOwner processes CMSG_CHANNEL_OWNER (0x09E).
// Reference: Channel::SendWhoOwner - members learn the owner NAME
// (ChannelOwnerAppend, ChannelAppenders.h:186: a C-string, "Nobody" for
// constant channels or no owner), everyone else is told they are not on
// the channel.
func (s *session) handleChannelOwner(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || len(payload) == 0 {
		return true
	}
	name, _, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.RLock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.RUnlock()
		return true
	}
	_, on := ch.Members[s]
	custom := ch.Flags&channelFlagCustom != 0
	owner := ch.Owner
	ownerName := ""
	if custom && owner != 0 {
		for m := range ch.Members {
			if m.playerGUID == owner && m.player != nil {
				ownerName = m.player.Name
				break
			}
		}
	} else {
		ownerName = "Nobody"
	}
	s.server.channelsMu.RUnlock()
	if !on {
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	_ = s.sendChannelNotify(channelChannelOwnerNotice, name, &channelNotifyName{Name: ownerName})
	return true
}

// channelSetMode implements Channel::SetMode for both the moderator family
// (CMSG_CHANNEL_MODERATOR/UNMODERATOR) and the mute family
// (CMSG_CHANNEL_MUTE/UNMUTE): moderator-only senders, target must be on the
// channel (cross-team targets are reported as not found unless both sides
// hold RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL), the owner cannot be demoted
// or muted by anyone else, and the change is broadcast as a mode change with
// old and new member flags.
func (s *session) channelSetMode(payload []byte, moderator, set bool) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, targetName, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return true
	}
	// Reference: making yourself the owner-moderator again is a no-op.
	if moderator && s.playerGUID == ch.Owner && strings.EqualFold(s.player.Name, targetName) {
		s.server.channelsMu.Unlock()
		return true
	}
	target := ch.findMemberByName(targetName)
	if target == nil {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	// Reference: cross-team targets are reported as not found unless both
	// sides hold RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL.
	if playerTeam(s.player.Race) != playerTeam(target.player.Race) &&
		!(s.twoSideChannelInteraction() && target.twoSideChannelInteraction()) {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	// Reference: nobody touches the owner unless they are the owner.
	if ch.Owner == target.playerGUID && ch.Owner != s.playerGUID {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotOwnerNotice, name, nil)
		return true
	}
	store := ch.Moderators
	if !moderator {
		store = ch.Muted
	}
	_, already := store[target.playerGUID]
	if already == set {
		s.server.channelsMu.Unlock()
		return true
	}
	oldFlags := ch.memberFlags(target.playerGUID)
	if set {
		store[target.playerGUID] = struct{}{}
	} else {
		delete(store, target.playerGUID)
	}
	newFlags := ch.memberFlags(target.playerGUID)
	members := s.server.channelMembersSnapshot(ch)
	s.server.channelsMu.Unlock()
	for _, m := range members {
		_ = m.sendChannelNotify(channelModeChangeNotice, ch.Name, &channelNotifyModeChange{GUID: target.playerGUID, OldFlags: oldFlags, NewFlags: newFlags})
	}
	return true
}

// handleChannelModerator processes CMSG_CHANNEL_MODERATOR (0x09F).
func (s *session) handleChannelModerator(ctx context.Context, payload []byte) bool {
	return s.channelSetMode(payload, true, true)
}

// handleChannelUnmoderator processes CMSG_CHANNEL_UNMODERATOR (0x0A0).
func (s *session) handleChannelUnmoderator(ctx context.Context, payload []byte) bool {
	return s.channelSetMode(payload, true, false)
}

// handleChannelMute processes CMSG_CHANNEL_MUTE (0x0A1).
func (s *session) handleChannelMute(ctx context.Context, payload []byte) bool {
	return s.channelSetMode(payload, false, true)
}

// handleChannelUnmute processes CMSG_CHANNEL_UNMUTE (0x0A2).
func (s *session) handleChannelUnmute(ctx context.Context, payload []byte) bool {
	return s.channelSetMode(payload, false, false)
}

// handleChannelInvite processes CMSG_CHANNEL_INVITE (0x0A3).
// Reference: Channel::Invite - member guard, target lookup, banned target,
// wrong faction (reported before the already-member check, and gated on
// RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL for both sides), already-member,
// then the invite notice to the target (skipped when the target ignored the
// inviter) and the player-invited notice back to the inviter.
func (s *session) handleChannelInvite(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, targetName, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.RLock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.RUnlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.RUnlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	s.server.channelsMu.RUnlock()

	s.server.sessionsMu.RLock()
	var target *session
	for sess := range s.server.sessions {
		if sess.player != nil && strings.EqualFold(sess.player.Name, targetName) {
			target = sess
			break
		}
	}
	s.server.sessionsMu.RUnlock()
	if target == nil || target.player.ExtraFlags&playerExtraGMInvisible != 0 {
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	s.server.channelsMu.RLock()
	ch = s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.RUnlock()
		return true
	}
	if _, banned := ch.Banned[target.playerGUID]; banned {
		s.server.channelsMu.RUnlock()
		_ = s.sendChannelNotify(channelPlayerInviteBannedNot, name, &channelNotifyName{Name: targetName})
		return true
	}
	_, alreadyMember := ch.Members[target]
	s.server.channelsMu.RUnlock()
	// Reference: the wrong-faction check runs before the already-member check
	// and requires RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL on both sides.
	if playerTeam(s.player.Race) != playerTeam(target.player.Race) &&
		!(s.twoSideChannelInteraction() && target.twoSideChannelInteraction()) {
		_ = s.sendChannelNotify(channelInviteWrongFactionNot, name, nil)
		return true
	}
	if alreadyMember {
		_ = s.sendChannelNotify(channelAlreadyMemberNotice, name, &channelNotifyGUID{GUID: target.playerGUID})
		return true
	}
	// Reference: the invite notice is withheld when the target ignored the
	// inviter; the inviter still sees the player-invited notice.
	if !s.server.chatIgnoredBy(target.playerGUID, s.playerGUID) {
		_ = target.sendChannelNotify(channelInviteNotice, name, &channelNotifyGUID{GUID: s.playerGUID})
	}
	_ = s.sendChannelNotify(channelPlayerInvitedNotice, name, &channelNotifyName{Name: targetName})
	return true
}

// twoSideChannelInteraction resolves the reference
// RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL permission (id 26) for channel use.
func (s *session) twoSideChannelInteraction() bool {
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil || !s.authed {
		return false
	}
	// The permission is checked through the shared RBAC resolver; accountID 0
	// sessions (unit tests) are treated as unprivileged.
	if s.accountID == 0 {
		return false
	}
	granted, err := accountHasPermission(context.Background(), s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionTwoSideInteractionChannel)
	return err == nil && granted
}

// silentlyJoinChannel resolves the reference RBAC_PERM_SILENTLY_JOIN_CHANNEL
// permission (id 45): Channel::JoinChannel/LeaveChannel/KickOrBan suppress
// the join/leave/kicked/banned broadcast when the acting session holds it.
func (s *session) silentlyJoinChannel() bool {
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil || !s.authed {
		return false
	}
	if s.accountID == 0 {
		return false
	}
	granted, err := accountHasPermission(context.Background(), s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionSilentlyJoinChannel)
	return err == nil && granted
}

// changeChannelNotModerator resolves the reference
// RBAC_PERM_CHANGE_CHANNEL_NOT_MODERATOR permission (id 46): lets a session
// run the moderator-gated channel commands (kick/ban/unban, mode, password,
// announcements, ownership handout) without being a channel moderator.
func (s *session) changeChannelNotModerator() bool {
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil || !s.authed {
		return false
	}
	if s.accountID == 0 {
		return false
	}
	granted, err := accountHasPermission(context.Background(), s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionChangeChannelNotModerator)
	return err == nil && granted
}

// channelKickBan implements Channel::KickOrBan for CMSG_CHANNEL_KICK (0x0A4)
// and CMSG_CHANNEL_BAN (0x0A5): member and moderator guards, target on
// channel, the owner can only be removed by the owner, then removal plus the
// kicked/banned broadcast carrying victim and acting moderator GUIDs.
func (s *session) channelKickBan(payload []byte, ban bool) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, targetName, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return true
	}
	target := ch.findMemberByName(targetName)
	if target == nil {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	if ch.Owner == target.playerGUID && ch.Owner != s.playerGUID && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotOwnerNotice, name, nil)
		return true
	}
	victimGUID := target.playerGUID
	delete(ch.Members, target)
	if target.channels != nil {
		delete(target.channels, target.scopedChannelKey(name))
	}
	if ban {
		ch.Banned[victimGUID] = struct{}{}
	}
	// Reference: Channel::KickOrBan - when the owner is removed from a custom
	// channel the acting moderator becomes the new owner.
	ownerTransferred := false
	var oldFlags, newFlags uint8
	if ch.Owner == victimGUID && ch.Flags&channelFlagCustom != 0 && len(ch.Members) > 0 {
		oldFlags = ch.memberFlags(s.playerGUID)
		ch.Owner = s.playerGUID
		ch.Moderators[s.playerGUID] = struct{}{}
		newFlags = ch.memberFlags(s.playerGUID)
		ownerTransferred = true
	}
	members := s.server.channelMembersSnapshot(ch)
	channelFlags, channelID := ch.Flags, ch.ID
	s.server.channelsMu.Unlock()
	notice := channelPlayerKickedNotice
	if ban {
		notice = channelPlayerBannedNotice
	}
	// Reference: Channel::KickOrBan (Channel.cpp) - the kicked/banned
	// broadcast is suppressed when the acting session holds
	// RBAC_PERM_SILENTLY_JOIN_CHANNEL; the removal itself always runs.
	if !s.silentlyJoinChannel() {
		_ = target.sendChannelNotify(notice, name, &channelNotifyTwoGUID{Victim: victimGUID, Moderator: s.playerGUID})
		for _, m := range members {
			_ = m.sendChannelNotify(notice, name, &channelNotifyTwoGUID{Victim: victimGUID, Moderator: s.playerGUID})
			if ownerTransferred {
				_ = m.sendChannelNotify(channelModeChangeNotice, name, &channelNotifyModeChange{GUID: s.playerGUID, OldFlags: oldFlags, NewFlags: newFlags})
				_ = m.sendChannelNotify(channelOwnerChangedNotice, name, &channelNotifyGUID{GUID: s.playerGUID})
			}
		}
	}
	_ = target.sendChannelNotify(channelYouLeftNotice, name, &channelNotifyChannel{Flags: channelFlags, ID: channelID})
	return true
}

// handleChannelKick processes CMSG_CHANNEL_KICK (0x0A4).
func (s *session) handleChannelKick(ctx context.Context, payload []byte) bool {
	return s.channelKickBan(payload, false)
}

// handleChannelBan processes CMSG_CHANNEL_BAN (0x0A5).
func (s *session) handleChannelBan(ctx context.Context, payload []byte) bool {
	return s.channelKickBan(payload, true)
}

// handleChannelUnban processes CMSG_CHANNEL_UNBAN (0x0A6).
// Reference: Channel::UnBan - moderator guards, target resolved by name even
// when offline, not-banned guard, then the unbanned broadcast.
func (s *session) handleChannelUnban(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, targetName, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return true
	}
	var targetGUID uint64
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? COLLATE NOCASE", targetName).Scan(&targetGUID)
	}
	if targetGUID == 0 {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	if _, banned := ch.Banned[targetGUID]; !banned {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotBannedNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	delete(ch.Banned, targetGUID)
	members := s.server.channelMembersSnapshot(ch)
	s.server.channelsMu.Unlock()
	for _, m := range members {
		_ = m.sendChannelNotify(channelPlayerUnbannedNotice, name, &channelNotifyTwoGUID{Victim: targetGUID, Moderator: s.playerGUID})
	}
	return true
}

// handleChannelAnnouncements processes CMSG_CHANNEL_ANNOUNCEMENTS (0x0A7).
// Reference: Channel::Announce - moderator guards, toggle, broadcast.
func (s *session) handleChannelAnnouncements(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	name, _, ok := readChannelCommand(payload)
	if !ok {
		return false
	}
	s.server.channelsMu.Lock()
	ch := s.server.channels[s.scopedChannelKey(name)]
	if ch == nil {
		s.server.channelsMu.Unlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotMemberNotice, name, nil)
		return true
	}
	if !ch.isModerator(s.playerGUID) && !s.changeChannelNotModerator() {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelNotModeratorNotice, name, nil)
		return true
	}
	ch.Announce = !ch.Announce
	notice := channelAnnouncementsOnNotice
	if !ch.Announce {
		notice = channelAnnouncementsOffNotice
	}
	members := s.server.channelMembersSnapshot(ch)
	s.server.channelsMu.Unlock()
	for _, m := range members {
		_ = m.sendChannelNotify(notice, ch.Name, &channelNotifyGUID{GUID: s.playerGUID})
	}
	return true
}

// handleChannelVoiceOn processes CMSG_CHANNEL_VOICE_ON (0x3D6).
// Reference: ChannelHandler.cpp HandleChannelVoiceOn calls Channel::Voice,
// which is an empty body in the reference - a true no-op.
func (s *session) handleChannelVoiceOn(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || len(payload) > 0 {
		if len(payload) > 0 {
			r := protocol.NewReader(payload)
			if _, err := r.ReadCString(); err != nil {
				return false
			}
		}
	}
	return true
}

// handleChannelModerate processes CMSG_CHANNEL_MODERATE (0x0A8).
// Reference: TrinityCore Opcodes.cpp CMSG_CHANNEL_MODERATE -> WorldSession::Handle_NULL.
func (s *session) handleChannelModerate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	s.debug("reference logged-in no-op", "account", s.accountName, "opcode", protocol.OpcodeCMSG_CHANNEL_MODERATE, "size", len(payload))
	return true
}

// handleGetChannelMemberCount processes CMSG_GET_CHANNEL_MEMBER_COUNT (0x3D3).
// Reference: HandleGetChannelMemberCount only answers channels the player is
// on: name, channel flags, member count.
func (s *session) handleGetChannelMemberCount(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || len(payload) == 0 {
		return true
	}
	r := protocol.NewReader(payload)
	channelName, err := r.ReadCString()
	if err != nil {
		return false
	}
	s.server.channelsMu.RLock()
	ch := s.server.channels[s.scopedChannelKey(channelName)]
	if ch == nil {
		s.server.channelsMu.RUnlock()
		return true
	}
	if _, on := ch.Members[s]; !on {
		s.server.channelsMu.RUnlock()
		return true
	}
	flags, count, name := ch.Flags, uint32(len(ch.Members)), ch.Name
	s.server.channelsMu.RUnlock()

	buf := protocol.NewBuffer(len(name) + 6)
	buf.WriteCString(name)
	buf.WriteU8(flags)
	buf.WriteU32(count)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHANNEL_MEMBER_COUNT), buf.Bytes(), true)
	return true
}

// handleDeclineChannelInvite processes CMSG_DECLINE_CHANNEL_INVITE (0x264).
func (s *session) handleDeclineChannelInvite(ctx context.Context, payload []byte) bool {
	s.debug("channel invite declined", "account", s.accountName)
	return true
}

// handleSetActiveVoiceChannel processes CMSG_SET_ACTIVE_VOICE_CHANNEL (0x3D3).
func (s *session) handleSetActiveVoiceChannel(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	if _, err := r.ReadU32(); err != nil {
		return false
	}
	if _, err := r.ReadCString(); err != nil {
		return false
	}
	return true
}

// handleVoiceSessionEnable processes CMSG_VOICE_SESSION_ENABLE (0x3AF).
func (s *session) handleVoiceSessionEnable(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	if _, err := r.ReadU8(); err != nil {
		return false
	}
	if _, err := r.ReadU8(); err != nil {
		return false
	}
	return true
}

// handleSetChannelWatch processes CMSG_SET_CHANNEL_WATCH (0x3EF).
func (s *session) handleSetChannelWatch(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	if _, err := r.ReadCString(); err != nil {
		return false
	}
	return true
}
