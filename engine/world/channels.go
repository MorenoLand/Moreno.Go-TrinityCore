package world

import (
	"context"
	"fmt"
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

// CHANNEL_DBC_FLAG_* from Channel.h:93-102 — the ChatChannels.dbc flag bits
// driving Player::UpdateLocalChannels and
// Player::CanJoinConstantChannelInZone.
const (
	channelDBCFlagInitial   uint32 = 0x00001
	channelDBCFlagZoneDep   uint32 = 0x00002
	channelDBCFlagGlobal    uint32 = 0x00004
	channelDBCFlagTrade     uint32 = 0x00008
	channelDBCFlagCityOnly  uint32 = 0x00010
	channelDBCFlagCityOnly2 uint32 = 0x00020
	channelDBCFlagDefense   uint32 = 0x10000
	channelDBCFlagGuildReq  uint32 = 0x20000
	channelDBCFlagLFG       uint32 = 0x40000
)

// channelCityName is the LANG_CHANNEL_CITY trinity_string (819) that
// Channel::GetChannelName (Channel.cpp:96) formats into CITY_ONLY channel
// names: "Trade - City", "GuildRecruitment - City", "LookingForGroup - City".
const channelCityName = "City"

// chatChannelZoneAllowed mirrors Player::CanJoinConstantChannelInZone
// (Player.cpp:5159-5171) driven by the DBC row flags: zone-dependent rows
// are refused inside arena instances, CITY_ONLY rows outside cities, and
// GUILD_REQ rows for players already in a guild.
func chatChannelZoneAllowed(dbcFlags, areaFlags uint32, inGuild bool) bool {
	if dbcFlags&channelDBCFlagZoneDep != 0 && areaFlags&wotlk.AreaFlagArenaInstance != 0 {
		return false
	}
	if dbcFlags&channelDBCFlagCityOnly != 0 && areaFlags&wotlk.AreaFlagSlaveCapital == 0 {
		return false
	}
	if dbcFlags&channelDBCFlagGuildReq != 0 && inGuild {
		return false
	}
	return true
}

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
	// Reference: Player::CanJoinConstantChannelInZone (Player.cpp:5159-5171) —
	// zone-dependent channels (General/Trade/LocalDefense/GuildRecruitment)
	// are refused inside arena instances, city-only channels
	// (Trade/GuildRecruitment/LFG) outside cities, and GuildRecruitment for
	// players already in a guild; unknown channel IDs are refused.
	// ChannelHandler.cpp gates both join (:46-49) and leave (:112-115) on it,
	// silently in both cases.
	if channelID != 0 && (s.player == nil || !s.constantChannelZoneAllowed(channelID)) {
		s.debug("channel join rejected: zone gate", "account", s.accountName, "id", channelID)
		return true
	}
	s.server.channelsMu.Lock()
	if s.server.channels == nil {
		s.server.channels = make(map[string]*worldChannel)
	}
	channel := s.server.channels[key]
	fresh := channel == nil
	if fresh {
		channel = &worldChannel{
			ID:   channelID,
			Name: name,
			// Reference: Channel::Channel - custom channels announce
			// joins/leaves and hand out ownership; constant (built-in)
			// channels do neither, and their members carry MEMBER_FLAG_NONE.
			Flags:      flags,
			Password:   password,
			Announce:   channelID == 0,
			Members:    make(map[*session]struct{}),
			Moderators: make(map[uint64]struct{}),
			Muted:      make(map[uint64]struct{}),
			Banned:     make(map[uint64]struct{}),
		}
		if channelID == 0 {
			channel.Owner = s.playerGUID
			channel.Moderators[s.playerGUID] = struct{}{}
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
		// Reference: PlayerAlreadyMemberAppend (ChannelAppenders.h) carries the
		// joiner's GUID in the notice payload.
		return s.sendChannelNotify(channelAlreadyMemberNotice, channel.Name, &channelNotifyGUID{GUID: s.playerGUID}) == nil
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
	addChannelKeyLocked(s, key)
	others := make([]*session, 0, len(channel.Members)-1)
	for member := range channel.Members {
		if member != s {
			others = append(others, member)
		}
	}
	channelName, channelFlagsValue, channelIDValue, announce := channel.Name, channel.Flags, channel.ID, channel.Announce
	joinerFlags, numPlayers := channel.memberFlags(s.playerGUID), uint32(len(channel.Members))
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
	// Reference: Channel::JoinNotify (Channel.cpp:822-840) — constant channels
	// broadcast SMSG_USERLIST_ADD to all-but-one, custom channels
	// SMSG_USERLIST_UPDATE to all (the joiner is already in the member set,
	// so they receive their own update on custom channels).
	s.broadcastUserlist(channelIDValue != 0, s.playerGUID, joinerFlags, channelFlagsValue, numPlayers, channelName, others)
	// Reference: Channel::JoinChannel (Channel.cpp) - the first join of a
	// fresh custom channel grants ownership through SetOwner(guid, false),
	// which broadcasts CHAT_MODE_CHANGE_NOTICE (old flags MEMBER_FLAG_NONE,
	// new flags owner+moderator) to all members.
	if fresh && channelIDValue == 0 {
		_ = s.sendChannelNotify(channelModeChangeNotice, channelName, &channelNotifyModeChange{GUID: s.playerGUID, OldFlags: 0, NewFlags: joinerFlags})
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
	// Reference: ChannelHandler.cpp:112-115 — constant-channel leaves are
	// zone-gated exactly like joins (silent drop when the player may not be
	// on the channel in their current zone).
	if channelID != 0 && !s.constantChannelZoneAllowed(channelID) {
		s.debug("channel leave rejected: zone gate", "account", s.accountName, "id", channelID)
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
	removeChannelKeyLocked(s, key)
	others := make([]*session, 0, len(channel.Members))
	for member := range channel.Members {
		others = append(others, member)
	}
	channelName, channelFlagsValue, channelIDValue, announce := channel.Name, channel.Flags, channel.ID, channel.Announce
	numPlayers := uint32(len(channel.Members))
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
	// Reference: Channel::LeaveNotify (Channel.cpp:842-860) — SMSG_USERLIST_REMOVE
	// to all-but-one (constant) or all (custom); the leaver is already erased
	// from the member set in both cases, so the recipients are the remaining
	// members either way.
	removePkt := protocol.NewBuffer(8 + 1 + 4 + len(channelName) + 1)
	removePkt.WriteU64(s.playerGUID)
	removePkt.WriteU8(channelFlagsValue)
	removePkt.WriteU32(numPlayers)
	removePkt.WriteCString(channelName)
	for _, member := range others {
		_ = member.write(uint16(protocol.OpcodeSMSG_USERLIST_REMOVE), removePkt.Bytes(), true)
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
	// Reference: Channel::List (Channel.cpp) answers CHAT_NOT_MEMBER_NOTICE
	// when the requester is not on the channel.
	if _, on := channel.Members[s]; !on {
		s.server.channelsMu.RUnlock()
		return s.sendChannelNotify(channelNotMemberNotice, name, nil) == nil
	}
	members := make([]member, 0, len(channel.Members))
	for session := range channel.Members {
		if !session.worldReady.Load() || session.player == nil {
			continue
		}
		// Reference: Channel::List (Channel.cpp) - members above the
		// GM-in-who-list security level stay hidden from viewers without the
		// who-see-all permission, and GM-invisible members stay hidden per
		// Player::IsVisibleGloballyFor.
		if (s.whoSeeAllSecurityLevels || int(session.security) <= s.server.Config.GMInWhoListLevel) &&
			isVisibleGloballyFor(s, session) {
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

// broadcastUserlist emits the roster live-update for a channel join:
// SMSG_USERLIST_ADD on constant channels (to all-but-one) or
// SMSG_USERLIST_UPDATE on custom channels (to all, joiner included).
// Payload per Channel::JoinNotify (Channel.cpp:822-840): joiner GUID, joiner
// member flags, channel flags, member count (joiner included), channel name.
func (s *session) broadcastUserlist(constant bool, joinerGUID uint64, joinerFlags, channelFlags uint8, numPlayers uint32, name string, others []*session) {
	packet := protocol.NewBuffer(8 + 1 + 1 + 4 + len(name) + 1)
	packet.WriteU64(joinerGUID)
	packet.WriteU8(joinerFlags)
	packet.WriteU8(channelFlags)
	packet.WriteU32(numPlayers)
	packet.WriteCString(name)
	payload := packet.Bytes()
	var opcode protocol.Opcode
	if constant {
		opcode = protocol.OpcodeSMSG_USERLIST_ADD
	} else {
		opcode = protocol.OpcodeSMSG_USERLIST_UPDATE
	}
	for _, member := range others {
		_ = member.write(uint16(opcode), payload, true)
	}
	if !constant && s != nil {
		_ = s.write(uint16(opcode), payload, true)
	}
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
	// Reference: ChannelMgr::GetChannelForPlayerByNamePart walks the player's
	// joined-channel list (a std::list in join order) and returns the FIRST
	// channel whose lowercased name starts with the lowercased prefix.
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()
	order := member.channelOrder
	if len(order) == 0 {
		for key := range member.channels {
			order = append(order, key)
		}
	}
	for _, key := range order {
		ch := s.channels[key]
		if ch == nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(ch.Name), part) {
			return key, ch.Name, true
		}
	}
	return "", "", false
}

// addChannelKeyLocked records a channel join in the session's channel list in
// join order (Player::GetJoinedChannels is join-ordered in the reference).
func addChannelKeyLocked(member *session, key string) {
	if member.channels == nil {
		member.channels = make(map[string]struct{})
	}
	if _, ok := member.channels[key]; ok {
		return
	}
	member.channels[key] = struct{}{}
	member.channelOrder = append(member.channelOrder, key)
}

// removeChannelKeyLocked drops a channel from the session's channel list,
// keeping the join-order slice in sync.
func removeChannelKeyLocked(member *session, key string) {
	if member.channels != nil {
		delete(member.channels, key)
	}
	for i, k := range member.channelOrder {
		if k == key {
			member.channelOrder = append(member.channelOrder[:i], member.channelOrder[i+1:]...)
			break
		}
	}
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
		// Reference: Channel.h documents the LFG runtime flags as
		// 0x50 = 0x40 | 0x10 — CHANNEL_FLAG_CITY comes only from
		// CHANNEL_DBC_FLAG_CITY_ONLY2 (Channel.cpp:54-55), which the LFG DBC
		// row lacks. City gating for LFG rides its LFG bit (see the join
		// gate and updateLocalChannels), matching CanJoinConstantChannelInZone
		// (Player.cpp:5164-5165, CITY_ONLY covers Trade/GuildRecruitment/LFG).
		return channelFlagGeneral | channelFlagLFG
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
		member.channelOrder = nil
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
	member.channelOrder = nil
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

// constantChannelZoneAllowed mirrors Player::CanJoinConstantChannelInZone
// (Player.cpp:5159-5171): zone-dependent channels (General/Trade/LocalDefense/
// GuildRecruitment) are refused inside arena instances, city-only channels
// (Trade/GuildRecruitment/LFG per CHANNEL_DBC_FLAG_CITY_ONLY) outside cities,
// and GuildRecruitment for players already in a guild; unknown channel IDs
// are refused. ChannelHandler.cpp gates both join (:46-49) and leave
// (:112-115) on it, silently in both cases.
func (s *session) constantChannelZoneAllowed(channelID uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	switch channelID {
	case 1, 2, 3, 22, 23, 24:
	default:
		return false
	}
	if _, _, inArena, _ := battlegroundTypeForMap(s.player.Map); inArena &&
		(channelID == 1 || channelID == 2 || channelID == 3 || channelID == 23) {
		return false
	}
	if channelID == 23 && s.player.GuildID != 0 {
		return false
	}
	if (channelID == 2 || channelID == 23 || channelID == 24) && !s.isCityZone(s.player.Zone) {
		return false
	}
	return true
}

func (s *session) updateLocalChannels(newZone uint32) {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return
	}
	s.player.Zone = newZone
	// Reference: Player::UpdateLocalChannels (Player.cpp:5199-5202) — during
	// initial login the client drives the built-in channel joins itself, so
	// the server skips the whole walk; it runs on every later zone change
	// (Player::UpdateZone, Player.cpp:7272: "recent client version not send
	// leave/join channel packets for built-in local channels"). The Go login
	// flow has no teleported-far window (no IsBeingTeleportedFar analog), so
	// playerLoading alone gates it.
	if s.playerLoading {
		return
	}
	s.syncLocalChannels(newZone)
}

// channelZoneEvent is one membership change produced by the zone-change walk;
// notices go out after the channel lock is released.
type channelZoneEvent struct {
	join      bool
	channel   *worldChannel
	key       string
	notice    uint8 // banned / not-in-LFG denial notice for failed joins
	announce  bool
	sendLeave bool // C++ LeaveChannel(this, send): false skips the YouLeft notice
	members   []*session
	newOwner  *session
	oldFlags  uint8
	newFlags  uint8
}

// syncLocalChannels ports Player::UpdateLocalChannels (Player.cpp:5210-5269):
// one walk over the ChatChannels.dbc rows. For each row it locates the
// player's joined channel with that DBC id (usedChannel), then either joins
// the zone's system channel — replacing the old zone's channel without a
// leave notice when the name changed (General/LocalDefense:
// sendRemove=false, the client already replaced it), skipping when already
// on it (city channels keep their names; the WorldDefense re-join is a
// silent no-op) — or removes the player from the channel when the zone no
// longer allows it (leaving a city, entering an arena instance, joining a
// guild). The old city-exit-only removal is subsumed by the walk's
// !CanJoin arm, which additionally covers arena and guild gates.
func (s *session) syncLocalChannels(newZone uint32) {
	if s.server.Data == nil {
		return
	}
	entries, err := s.server.Data.ChatChannels()
	if err != nil || len(entries) == 0 {
		return
	}
	// Reference: sAreaTableStore.LookupEntry(newZone) null arm — without a
	// zone entry there is nothing to key the zone channels by.
	area, found, aerr := s.server.Data.Area(newZone)
	if aerr != nil || !found {
		return
	}
	inGuild := s.player.GuildID != 0

	var events []channelZoneEvent

	s.server.channelsMu.Lock()
	if s.channels == nil || s.server.channels == nil {
		s.server.channelsMu.Unlock()
		return
	}
	for _, entry := range entries {
		// Reference: the usedChannel scan — the player's joined channel
		// whose DBC id matches this row.
		var usedKey string
		var used *worldChannel
		for _, key := range s.channelOrder {
			if ch := s.server.channels[key]; ch != nil && ch.ID == entry.ID {
				usedKey, used = key, ch
				break
			}
		}
		if !chatChannelZoneAllowed(entry.Flags, area.Flags, inGuild) {
			// Reference: removeChannel = usedChannel (sendRemove=true).
			if used != nil {
				events = append(events, s.removeZoneChannelLocked(usedKey, used, true))
			}
			continue
		}
		var joinName string
		switch {
		case entry.Flags&channelDBCFlagGlobal != 0:
			// WorldDefense: single channel, no zone in the name; the
			// re-join of a member is a silent no-op (Channel::JoinChannel
			// IsOn arm).
			if used != nil {
				continue
			}
			joinName = entry.Name
		case entry.Flags&channelDBCFlagCityOnly != 0:
			// Already on the channel, as city channel names are not changing.
			if used != nil {
				continue
			}
			joinName = fmt.Sprintf(entry.Name, channelCityName)
		default:
			joinName = fmt.Sprintf(entry.Name, area.Name)
			if used != nil && used.Name == joinName {
				continue // joinChannel == usedChannel
			}
		}
		key := s.scopedChannelKey(joinName)
		channel := s.server.channels[key]
		if channel == nil {
			channel = &worldChannel{
				ID:    entry.ID,
				Name:  joinName,
				Flags: channelFlags(entry.ID, joinName),
				// Built-in channels have no ownership model in C++
				// (_ownershipEnabled=false); the Go tree tracks the first
				// joiner as owner like the client-driven join path does.
				Owner:      s.playerGUID,
				Announce:   false,
				Members:    make(map[*session]struct{}),
				Moderators: make(map[uint64]struct{}),
				Muted:      make(map[uint64]struct{}),
				Banned:     make(map[uint64]struct{}),
			}
			s.server.channels[key] = channel
		}
		// Reference: Channel::JoinChannel (Channel.cpp) gate order —
		// already-member is silent for constant channels, then banned, then
		// the LFG restriction (constant channels carry no password).
		if _, ok := channel.Members[s]; ok {
			continue
		}
		if _, banned := channel.Banned[s.playerGUID]; banned {
			events = append(events, channelZoneEvent{channel: channel, notice: channelBannedNotice})
			continue
		}
		if channel.Flags&channelFlagLFG != 0 && s.server.Config.ChannelRestrictedLFG &&
			s.security == 0 && s.groupID != 0 {
			events = append(events, channelZoneEvent{channel: channel, notice: channelNotInLFGNotice})
			continue
		}
		channel.Members[s] = struct{}{}
		addChannelKeyLocked(s, key)
		members := make([]*session, 0, len(channel.Members)-1)
		for other := range channel.Members {
			if other != s {
				members = append(members, other)
			}
		}
		events = append(events, channelZoneEvent{join: true, channel: channel, key: key, members: members})
		if used != nil && used != channel {
			// Reference: removeChannel = usedChannel, sendRemove=false —
			// the client already replaced the channel, so no leave notice.
			events = append(events, s.removeZoneChannelLocked(usedKey, used, false))
		}
	}
	s.server.channelsMu.Unlock()

	for _, ev := range events {
		switch {
		case ev.notice != 0:
			_ = s.sendChannelNotify(ev.notice, ev.channel.Name, nil)
		case ev.join:
			channel := ev.channel
			if err := s.sendChannelNotify(channelYouJoinedNotice, channel.Name, &channelNotifyChannel{Flags: channel.Flags, ID: channel.ID}); err != nil {
				continue
			}
			// Reference: Channel::JoinNotify (Channel.cpp:822-840) —
			// constant channels broadcast SMSG_USERLIST_ADD to all-but-one.
			s.broadcastUserlist(true, s.playerGUID, channel.memberFlags(s.playerGUID), channel.Flags, uint32(len(channel.Members)), channel.Name, ev.members)
		default:
			channel := ev.channel
			if ev.announce && !s.silentlyJoinChannel() {
				for _, other := range ev.members {
					_ = other.sendChannelNotify(channelLeftNotice, channel.Name, &channelNotifyGUID{GUID: s.playerGUID})
				}
			}
			if ev.newOwner != nil {
				for _, other := range ev.members {
					_ = other.sendChannelNotify(channelModeChangeNotice, channel.Name, &channelNotifyModeChange{GUID: ev.newOwner.playerGUID, OldFlags: ev.oldFlags, NewFlags: ev.newFlags})
					_ = other.sendChannelNotify(channelOwnerChangedNotice, channel.Name, &channelNotifyGUID{GUID: ev.newOwner.playerGUID})
				}
			}
			if ev.sendLeave {
				_ = s.sendChannelNotify(channelYouLeftNotice, channel.Name, &channelNotifyChannel{Flags: channel.Flags, ID: channel.ID})
			}
			// Reference: Channel::LeaveNotify (Channel.cpp:842-860) —
			// SMSG_USERLIST_REMOVE to the remaining members.
			removePkt := protocol.NewBuffer(8 + 1 + 4 + len(channel.Name) + 1)
			removePkt.WriteU64(s.playerGUID)
			removePkt.WriteU8(channel.Flags)
			removePkt.WriteU32(uint32(len(channel.Members)))
			removePkt.WriteCString(channel.Name)
			for _, other := range ev.members {
				_ = other.write(uint16(protocol.OpcodeSMSG_USERLIST_REMOVE), removePkt.Bytes(), true)
			}
		}
	}
}

// removeZoneChannelLocked erases the session from a zone-change walk channel:
// member removal, join-order list cleanup, ownership hand-off when the owner
// leaves, and channel deletion when empty — the shared tail of
// Channel::LeaveChannel for the walk. The caller emits the notices after
// unlocking. sendLeave selects the C++ LeaveChannel(this, send) notice arm.
func (s *session) removeZoneChannelLocked(key string, channel *worldChannel, sendLeave bool) channelZoneEvent {
	delete(channel.Members, s)
	removeChannelKeyLocked(s, key)
	var newOwner *session
	var oldFlags, newFlags uint8
	if channel.Owner == s.playerGUID {
		newOwner, oldFlags, newFlags = channelTakeOwnershipLocked(channel)
	}
	members := make([]*session, 0, len(channel.Members))
	for other := range channel.Members {
		members = append(members, other)
	}
	if len(channel.Members) == 0 {
		delete(s.server.channels, key)
	}
	return channelZoneEvent{
		channel:   channel,
		key:       key,
		announce:  channel.Announce,
		sendLeave: sendLeave,
		members:   members,
		newOwner:  newOwner,
		oldFlags:  oldFlags,
		newFlags:  newFlags,
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
	removeChannelKeyLocked(target, target.scopedChannelKey(name))
	// Reference: Channel::KickOrBan - the banned broadcast fires only from
	// the first arm (ban && !IsBanned(victim)); re-banning an already-banned
	// on-channel member falls through to the kicked broadcast.
	alreadyBanned := false
	if ban {
		_, alreadyBanned = ch.Banned[victimGUID]
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
	if ban && !alreadyBanned {
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
// Reference: Channel::UnBan - the target is resolved by
// ObjectAccessor::FindConnectedPlayerByName (online players only), and both
// the not-online and not-banned cases answer CHAT_PLAYER_NOT_FOUND_NOTICE;
// PlayerNotBannedAppend exists in the reference but is never sent by any
// Channel code path, so Go must not emit it either.
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
	s.server.sessionsMu.RLock()
	for sess := range s.server.sessions {
		if sess.player != nil && strings.EqualFold(sess.player.Name, targetName) {
			targetGUID = sess.playerGUID
			break
		}
	}
	s.server.sessionsMu.RUnlock()
	if targetGUID == 0 {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
		return true
	}
	if _, banned := ch.Banned[targetGUID]; !banned {
		s.server.channelsMu.Unlock()
		_ = s.sendChannelNotify(channelPlayerNotFoundNotice, name, &channelNotifyName{Name: targetName})
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

// handleDeclineChannelInvite processes CMSG_DECLINE_CHANNEL_INVITE (0x410).
// Reference: WorldSession::HandleChannelDeclineInvite (ChatHandler.cpp:763):
// the C++ body is a pure no-op (debug log only), so the Go handler is one too.
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
