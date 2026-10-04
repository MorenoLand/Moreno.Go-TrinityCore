package world

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Calendar invite status constants matching TrinityCore 3.3.5.
const (
	CalendarStatusInvited     = 0
	CalendarStatusAccepted    = 1
	CalendarStatusDeclined    = 2
	CalendarStatusConfirmed   = 3
	CalendarStatusSignedUp    = 4
	CalendarStatusNotSignedUp = 5
	CalendarStatusTentative   = 6
	CalendarStatusRemoved     = 9
)

// Calendar rank constants matching TrinityCore 3.3.5.
const (
	CalendarRankPlayer    = 0
	CalendarRankModerator = 1
	CalendarRankCreator   = 2
)

// Calendar flag bits matching TrinityCore 3.3.5 CalendarFlags (CalendarMgr.h:37-45).
const (
	calendarFlagGuildEvent     = 0x400
	calendarFlagWithoutInvites = 0x040
	calendarFlagInvitesLocked  = 0x010
)

// Calendar limits matching CalendarLimits (CalendarMgr.h:130-136).
const (
	calendarMaxInvites              = 100
	calendarMaxEvents               = 30
	calendarMaxGuildEvents          = 100
	calendarCreateEventCooldownSecs = 5
)

// calendarDefaultResponseTime is the 946684800 (01/01/2000 00:00:00) default
// response time CalendarHandler.cpp stamps on freshly created invites; the
// SMSG_CALENDAR_EVENT_INVITE hasStatusTime arm keys off it
// (CalendarMgr::SendCalendarEventInvite, CalendarMgr.cpp:481).
const calendarDefaultResponseTime = 946684800

// Calendar send type constants.
const (
	CalendarSendTypeGet    = 0
	CalendarSendTypeAdd    = 1
	CalendarSendTypeCopy   = 2
	CalendarSendTypeUpdate = 3
)

// Calendar error codes matching the real CalendarError enum (CalendarMgr.h:92-128).
// Values are sparse (15, 18, 23, 30-35 unused); clients key behavior off these.
const (
	CalendarOk                          = 0
	CalendarErrorGuildEventsExceeded    = 1
	CalendarErrorEventsExceeded         = 2
	CalendarErrorSelfInvitesExceeded    = 3
	CalendarErrorOtherInvitesExceeded   = 4
	CalendarErrorPermissions            = 5
	CalendarErrorEventInvalid           = 6
	CalendarErrorNotInvited             = 7
	CalendarErrorInternal               = 8
	CalendarErrorGuildPlayerNotInGuild  = 9
	CalendarErrorAlreadyInvitedToEventS = 10
	CalendarErrorPlayerNotFound         = 11
	CalendarErrorNotAllied              = 12
	CalendarErrorIgnoringYouS           = 13
	CalendarErrorInvitesExceeded        = 14
	CalendarErrorInvalidDate            = 16
	CalendarErrorInvalidTime            = 17
	CalendarErrorNeedsTitle             = 19
	CalendarErrorEventPassed            = 20
	CalendarErrorEventLocked            = 21
	CalendarErrorDeleteCreatorFailed    = 22
	CalendarErrorSystemDisabled         = 24
	CalendarErrorRestrictedAccount      = 25
	CalendarErrorArenaEventsExceeded    = 26
	CalendarErrorRestrictedLevel        = 27
	CalendarErrorUserSquelched          = 28
	CalendarErrorNoInvite               = 29
	CalendarErrorEventWrongServer       = 36
	CalendarErrorInviteWrongServer      = 37
	CalendarErrorNoGuildInvites         = 38
	CalendarErrorInvalidSignup          = 39
	CalendarErrorNoModerator            = 40
)

// calendarCommandResultParamErrors are the only CalendarError values whose
// SMSG_CALENDAR_COMMAND_RESULT carries a trailing name parameter
// (CalendarMgr::SendCalendarCommandResult, CalendarMgr.cpp:675-694).
func calendarCommandResultHasParam(err uint32) bool {
	return err == CalendarErrorOtherInvitesExceeded ||
		err == CalendarErrorAlreadyInvitedToEventS ||
		err == CalendarErrorIgnoringYouS
}

// sendCalendarCommandResult serializes and dispatches SMSG_CALENDAR_COMMAND_RESULT (0x43D).
// Reference: CalendarMgr::SendCalendarCommandResult (CalendarMgr.cpp:675-694):
// u32(0) always, u8(0), then the name parameter as a cstring only for the
// three _S errors (else a single u8(0)), then u32(err).
func (s *session) sendCalendarCommandResult(err uint32, name ...string) bool {
	buf := protocol.NewBuffer(16)
	buf.WriteU32(0)
	buf.WriteU8(0)
	if calendarCommandResultHasParam(err) && len(name) > 0 {
		buf.WriteCString(name[0])
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(err)
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_COMMAND_RESULT), buf.Bytes(), true) == nil
}

// unpackCalendarPackedTime decodes the client's packed 4-byte time format
// (ByteBuffer::AppendPackedTime) into local time; the unix value is what
// LocalTimeToUTCTime produces on the packed value in CalendarHandler.cpp.
func unpackCalendarPackedTime(packed uint32) time.Time {
	return time.Date(
		2000+int(packed>>24),
		time.Month(1+((packed>>20)&0xF)),
		1+int((packed>>14)&0x3F),
		int((packed>>6)&0x1F),
		int(packed&0x3F),
		0, 0, time.Local,
	)
}

// calendarEventInPast mirrors the "prevent events in the past" gate in
// HandleCalendarAddEvent/UpdateEvent/CopyEvent (CalendarHandler.cpp:238):
// the converted packed time must not be older than now minus the 86400s hack.
func calendarEventInPast(packed uint32) bool {
	return unpackCalendarPackedTime(packed).Unix() < time.Now().Unix()-86400
}

// calendarCreatorGuildID returns the guild of an event's creator, which is
// where CalendarEvent::GetGuildId comes from for guild events and guild
// announcements (CalendarHandler.cpp:262-265 sets it from the creator).
func calendarCreatorGuildID(ctx context.Context, cdb *sql.DB, creator uint64) uint32 {
	var guildID uint32
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(guildid, 0) FROM guild_member WHERE guid = ?", creator).Scan(&guildID)
	return guildID
}

// calendarCreateGateError enforces the shared add/copy creation gates
// (CalendarHandler.cpp:246-277 and :352-380): guild events and announcements
// require guild membership, then the per-guild (100) or per-player (30)
// event caps answer their errors; CalendarOk means creation may proceed.
func calendarCreateGateError(ctx context.Context, cdb *sql.DB, playerGUID uint64, playerGuildID uint32, flags uint32) uint32 {
	if flags&(calendarFlagGuildEvent|calendarFlagWithoutInvites) != 0 {
		if playerGuildID == 0 {
			return CalendarErrorGuildPlayerNotInGuild
		}
		var count int
		_ = cdb.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM calendar_events e
			 JOIN guild_member gm ON gm.guid = e.creator
			 WHERE (e.flags & ?) != 0 AND gm.guildid = ?`,
			calendarFlagGuildEvent|calendarFlagWithoutInvites, playerGuildID).Scan(&count)
		if count >= calendarMaxGuildEvents {
			return CalendarErrorGuildEventsExceeded
		}
		return CalendarOk
	}
	var count int
	_ = cdb.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM calendar_events WHERE creator = ? AND (flags & ?) = 0`,
		playerGUID, calendarFlagGuildEvent|calendarFlagWithoutInvites).Scan(&count)
	if count >= calendarMaxEvents {
		return CalendarErrorEventsExceeded
	}
	return CalendarOk
}

// calendarTakeCreateCooldown mirrors the CALENDAR_CREATE_EVENT_COOLDOWN gate
// (CalendarHandler.cpp:279-283, :382-386): while the session cooldown is in
// the future creation is rejected; otherwise the cooldown is armed 5s out.
func calendarTakeCreateCooldown(s *session) bool {
	now := time.Now().Unix()
	if s.calendarEventCooldown > now {
		return false
	}
	s.calendarEventCooldown = now + calendarCreateEventCooldownSecs
	return true
}

// calendarEventRelativeSessions implements CalendarMgr::SendPacketToAllEventRelatives
// (CalendarMgr.cpp:700-712): guild events and guild announcements go to every
// online guild member; every connected invitee outside the event's guild gets a
// direct copy; non-guild events go to all connected invitees (including the
// acting player when invited).
func calendarEventRelativeSessions(ctx context.Context, srv *Server, eventID uint64) []*session {
	if srv == nil || srv.CharactersStore == nil || srv.CharactersStore.DB == nil {
		return nil
	}
	cdb := srv.CharactersStore.DB
	var evFlags uint32
	var evCreator uint64
	if err := cdb.QueryRowContext(ctx,
		"SELECT flags, creator FROM calendar_events WHERE id = ?", eventID).Scan(&evFlags, &evCreator); err != nil {
		return nil
	}
	var eventGuild uint32
	if evFlags&(calendarFlagGuildEvent|calendarFlagWithoutInvites) != 0 {
		eventGuild = calendarCreatorGuildID(ctx, cdb, evCreator)
	}
	seen := make(map[*session]struct{})
	var targets []*session
	add := func(sess *session) {
		if sess == nil {
			return
		}
		if _, ok := seen[sess]; !ok {
			seen[sess] = struct{}{}
			targets = append(targets, sess)
		}
	}
	srv.sessionsMu.RLock()
	for sess := range srv.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if eventGuild != 0 && sess.player.GuildID == eventGuild {
			add(sess)
		}
	}
	srv.sessionsMu.RUnlock()
	invRows, err := cdb.QueryContext(ctx, "SELECT invitee FROM calendar_invites WHERE event = ?", eventID)
	if err != nil {
		return targets
	}
	defer invRows.Close()
	for invRows.Next() {
		var inviteeGUID uint64
		if err := invRows.Scan(&inviteeGUID); err != nil {
			continue
		}
		sess := srv.findSessionByGUID(inviteeGUID)
		if sess == nil {
			continue
		}
		// Same-guild invitees of a guild event already received the guild
		// broadcast; announcements broadcast to guild AND direct every invitee.
		if evFlags&calendarFlagGuildEvent != 0 && eventGuild != 0 && sess.player.GuildID == eventGuild {
			continue
		}
		add(sess)
	}
	return targets
}

// sendCalendarEventStatusToRelatives serializes SMSG_CALENDAR_EVENT_STATUS to
// every event relative (CalendarMgr::SendCalendarEventStatus, CalendarMgr.cpp:535-547):
// packed invitee GUID, u64 event id, packed event time, u32 flags, u8 status,
// u8 rank, packed status time.
func sendCalendarEventStatusToRelatives(srv *Server, targets []*session, inviteeGUID, eventID uint64, eventTime int64, flags uint32, status, rank uint8, statusTime int64) {
	buf := protocol.NewBuffer(48)
	buf.WritePackedGUID(inviteeGUID)
	buf.WriteU64(eventID)
	buf.WritePackedTime(time.Unix(eventTime, 0))
	buf.WriteU32(flags)
	buf.WriteU8(status)
	buf.WriteU8(rank)
	buf.WritePackedTime(time.Unix(statusTime, 0))
	for _, t := range targets {
		_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_STATUS), buf.Bytes(), true)
	}
}

// sendCalendarModeratorStatusToRelatives serializes SMSG_CALENDAR_EVENT_MODERATOR_STATUS_ALERT
// to every event relative (CalendarMgr::SendCalendarEventModeratorStatusAlert,
// CalendarMgr.cpp:570-579): packed invitee GUID, u64 event id, u8 rank, u8(1).
func sendCalendarModeratorStatusToRelatives(srv *Server, targets []*session, inviteeGUID, eventID uint64, rank uint8) {
	buf := protocol.NewBuffer(24)
	buf.WritePackedGUID(inviteeGUID)
	buf.WriteU64(eventID)
	buf.WriteU8(rank)
	buf.WriteU8(1) // Unk boolean - Display to client?
	for _, t := range targets {
		_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_MODERATOR_STATUS_ALERT), buf.Bytes(), true)
	}
}

// handleCalendarGetCalendar processes CMSG_CALENDAR_GET_CALENDAR (0x429).
// Reference: WorldSession::HandleCalendarGetCalendar (CalendarHandler.cpp:57-158)
// & WorldPackets::Calendar::CalendarSendCalendar::Write (CalendarPackets.cpp:239-267).
func (s *session) handleCalendarGetCalendar(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	currTime := time.Now()

	type calInvite struct {
		eventID    uint64
		inviteID   uint64
		status     uint8
		moderator  uint8
		inviteType uint8
		inviter    uint64
	}

	type calEvent struct {
		id        uint64
		title     string
		eventType uint32
		dungeon   int32
		flags     uint32
		eventTime uint32
		creator   uint64
	}

	type calLockout struct {
		mapID        int32
		difficultyID uint32
		expireTime   int32
		instanceID   uint64
	}

	type calRaidReset struct {
		mapID    int32
		duration int32
		offset   int32
	}

	var invites []calInvite
	var events []calEvent
	var lockouts []calLockout
	var raidResets []calRaidReset

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB

		// 1. Invites for the player
		invRows, err := cdb.QueryContext(ctx,
			`SELECT i.event, i.id, i.sender, i.status, i.rank, COALESCE(e.creator, i.sender), COALESCE(e.flags, 0)
			 FROM calendar_invites i
			 LEFT JOIN calendar_events e ON e.id = i.event
			 WHERE i.invitee = ?`,
			s.playerGUID)
		if err == nil {
			defer invRows.Close()
			for invRows.Next() {
				var inv calInvite
				var flags uint32
				var senderGUID uint64
				if err := invRows.Scan(&inv.eventID, &inv.inviteID, &senderGUID, &inv.status, &inv.moderator, &inv.inviter, &flags); err == nil {
					if (flags & calendarFlagGuildEvent) != 0 {
						inv.inviteType = 1
					}
					invites = append(invites, inv)
				}
			}
		}

		// 2. Events created by player or where player is invited
		evRows, err := cdb.QueryContext(ctx,
			`SELECT id, title, type, dungeon, flags, eventtime, creator FROM calendar_events
			 WHERE creator = ? OR id IN (SELECT event FROM calendar_invites WHERE invitee = ?)`,
			s.playerGUID, s.playerGUID)
		if err == nil {
			defer evRows.Close()
			for evRows.Next() {
				var ev calEvent
				if err := evRows.Scan(&ev.id, &ev.title, &ev.eventType, &ev.dungeon, &ev.flags, &ev.eventTime, &ev.creator); err == nil {
					events = append(events, ev)
				}
			}
		}

		// 3. Raid lockouts (bound permanent instances)
		lockRows, err := cdb.QueryContext(ctx,
			`SELECT i.map, i.difficulty, i.resettime, i.id
			 FROM character_instance ci
			 JOIN instance i ON i.id = ci.instance
			 WHERE ci.guid = ? AND ci.permanent = 1`,
			s.playerGUID)
		if err == nil {
			defer lockRows.Close()
			for lockRows.Next() {
				var lock calLockout
				var resetTime int64
				if err := lockRows.Scan(&lock.mapID, &lock.difficultyID, &resetTime, &lock.instanceID); err == nil {
					nowUnix := currTime.Unix()
					if resetTime > nowUnix {
						lock.expireTime = int32(resetTime - nowUnix)
					}
					lockouts = append(lockouts, lock)
				}
			}
		}

		// 4. Raid global resets
		resetRows, err := cdb.QueryContext(ctx,
			`SELECT mapid, resettime FROM instance_reset`)
		if err == nil {
			defer resetRows.Close()
			for resetRows.Next() {
				var r calRaidReset
				var resetTime int64
				if err := resetRows.Scan(&r.mapID, &resetTime); err == nil {
					nowUnix := currTime.Unix()
					if resetTime > nowUnix {
						r.duration = int32(resetTime - nowUnix)
					}
					r.offset = 0
					raidResets = append(raidResets, r)
				}
			}
		}
	}

	buf := protocol.NewBuffer(256 + len(invites)*32 + len(events)*64 + len(lockouts)*24 + len(raidResets)*12)

	// Invites list
	buf.WriteU32(uint32(len(invites)))
	for _, inv := range invites {
		buf.WriteU64(inv.eventID)
		buf.WriteU64(inv.inviteID)
		buf.WriteU8(inv.status)
		buf.WriteU8(inv.moderator)
		buf.WriteU8(inv.inviteType)
		buf.WritePackedGUID(inv.inviter)
	}

	// Events list
	buf.WriteU32(uint32(len(events)))
	for _, ev := range events {
		buf.WriteU64(ev.id)
		buf.WriteCString(ev.title)
		buf.WriteU32(ev.eventType)
		buf.WritePackedTime(time.Unix(int64(ev.eventTime), 0))
		buf.WriteU32(ev.flags)
		buf.WriteI32(ev.dungeon)
		buf.WritePackedGUID(ev.creator)
	}

	// Time synchronization
	buf.WriteU32(uint32(currTime.Unix()))
	buf.WritePackedTime(currTime)

	// Raid Lockouts
	buf.WriteU32(uint32(len(lockouts)))
	for _, lock := range lockouts {
		buf.WriteI32(lock.mapID)
		buf.WriteU32(lock.difficultyID)
		buf.WriteI32(lock.expireTime)
		buf.WriteU64(lock.instanceID)
	}

	// RaidOrigin constant (28.12.2005 07:00 UTC)
	buf.WriteU32(1135753200)

	// Raid Resets
	buf.WriteU32(uint32(len(raidResets)))
	for _, r := range raidResets {
		buf.WriteI32(r.mapID)
		buf.WriteI32(r.duration)
		buf.WriteI32(r.offset)
	}

	// Holidays count
	buf.WriteU32(0)

	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_CALENDAR), buf.Bytes(), true) == nil
}

// handleCalendarGetNumPending processes CMSG_CALENDAR_GET_NUM_PENDING (0x447).
// Reference: WorldSession::HandleCalendarGetNumPending (CalendarHandler.cpp:665).
func (s *session) handleCalendarGetNumPending(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	var count uint32 = 0
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM calendar_invites WHERE invitee = ? AND status = 0", s.playerGUID).Scan(&count)
	}
	buf := protocol.NewBuffer(4)
	buf.WriteU32(count)
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_NUM_PENDING), buf.Bytes(), true) == nil
}

// handleCalendarGetEvent processes CMSG_CALENDAR_GET_EVENT (0x42A).
// Reference: WorldSession::HandleCalendarGetEvent (CalendarHandler.cpp:160-168)
// & WorldPackets::Calendar::CalendarSendEvent::Write (CalendarPackets.cpp:269-289).
func (s *session) handleCalendarGetEvent(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
	}

	type eventInvitee struct {
		invitee    uint64
		id         uint64
		status     uint8
		rank       uint8
		inviteType uint8
		statusTime int64
		text       string
		level      uint8
	}

	title := ""
	description := ""
	var creator uint64
	var eventType uint8
	var dungeon int32 = -1
	var flags uint32
	var eventTime uint32
	var lockDate uint32
	var guildID uint32
	var invites []eventInvitee

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evType32 uint32
		err := cdb.QueryRowContext(ctx,
			"SELECT creator, title, description, type, dungeon, flags, eventtime, time2 FROM calendar_events WHERE id = ?",
			eventID).Scan(&creator, &title, &description, &evType32, &dungeon, &flags, &eventTime, &lockDate)
		if err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		eventType = uint8(evType32)

		if (flags&(calendarFlagGuildEvent|calendarFlagWithoutInvites)) != 0 && creator != 0 {
			_ = cdb.QueryRowContext(ctx,
				"SELECT COALESCE(guildid, 0) FROM guild_member WHERE guid = ?", creator).Scan(&guildID)
		}

		invRows, err := cdb.QueryContext(ctx,
			`SELECT i.invitee, i.id, i.status, i.rank, i.statustime, i.text, COALESCE(c.level, 1), COALESCE(gm.guildid, 0)
			 FROM calendar_invites i
			 LEFT JOIN characters c ON c.guid = i.invitee
			 LEFT JOIN guild_member gm ON gm.guid = i.invitee
			 WHERE i.event = ?`, eventID)
		if err == nil {
			defer invRows.Close()
			for invRows.Next() {
				var inv eventInvitee
				var inviteeGuildID uint32
				if err := invRows.Scan(&inv.invitee, &inv.id, &inv.status, &inv.rank, &inv.statusTime, &inv.text, &inv.level, &inviteeGuildID); err == nil {
					if (flags&calendarFlagGuildEvent) != 0 && guildID == inviteeGuildID {
						inv.inviteType = 1
					}
					invites = append(invites, inv)
				}
			}
		}
	} else {
		return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
	}

	buf := protocol.NewBuffer(128 + len(title) + len(description) + len(invites)*48)
	buf.WriteU8(CalendarSendTypeGet)
	buf.WritePackedGUID(creator)
	buf.WriteU64(eventID)
	buf.WriteCString(title)
	buf.WriteCString(description)
	buf.WriteU8(eventType)
	buf.WriteU8(0)    // repeatable
	buf.WriteU32(100) // maxInvites
	buf.WriteI32(dungeon)
	buf.WriteU32(flags)
	if eventTime > 0 {
		buf.WritePackedTime(time.Unix(int64(eventTime), 0))
	} else {
		buf.WritePackedTime(time.Now())
	}
	if lockDate > 0 {
		buf.WritePackedTime(time.Unix(int64(lockDate), 0))
	} else {
		buf.WritePackedTime(time.Now())
	}
	buf.WriteU32(guildID)
	buf.WriteU32(uint32(len(invites)))
	for _, inv := range invites {
		buf.WritePackedGUID(inv.invitee)
		buf.WriteU8(inv.level)
		buf.WriteU8(inv.status)
		buf.WriteU8(inv.rank)
		buf.WriteU8(inv.inviteType)
		buf.WriteU64(inv.id)
		if inv.statusTime > 0 {
			buf.WritePackedTime(time.Unix(inv.statusTime, 0))
		} else {
			buf.WritePackedTime(time.Now())
		}
		buf.WriteCString(inv.text)
	}

	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_EVENT), buf.Bytes(), true) == nil
}

// handleCalendarGuildFilter processes CMSG_CALENDAR_GUILD_FILTER (0x42B).
// Reference: WorldSession::HandleCalendarGuildFilter (CalendarHandler.cpp:194-208)
// & Guild::MassInviteToEvent (Guild.cpp:2162-2192): the requester is never
// listed, more than CALENDAR_MAX_INVITES (100) qualifying members answers
// CALENDAR_ERROR_INVITES_EXCEEDED instead of a list, and the per-member
// trailing byte is u8(0) (unk).
func (s *session) handleCalendarGuildFilter(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	var minLevel, maxLevel, minRank uint32
	if len(payload) >= 12 {
		r := protocol.NewReader(payload)
		minLevel, _ = r.ReadU32()
		maxLevel, _ = r.ReadU32()
		minRank, _ = r.ReadU32()
	}

	type memberInfo struct {
		guid uint64
	}
	var members []memberInfo

	if s.player.GuildID > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
			`SELECT c.guid FROM characters c
			 JOIN guild_member gm ON gm.guid = c.guid
			 WHERE gm.guildid = ? AND c.guid != ? AND c.level >= ? AND c.level <= ? AND gm.rank <= ?`,
			s.player.GuildID, s.playerGUID, minLevel, maxLevel, minRank)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m memberInfo
				if err := rows.Scan(&m.guid); err == nil {
					members = append(members, m)
				}
			}
		}
	}

	if len(members) > calendarMaxInvites {
		return s.sendCalendarCommandResult(CalendarErrorInvitesExceeded)
	}

	buf := protocol.NewBuffer(4 + len(members)*10)
	buf.WriteU32(uint32(len(members)))
	for _, m := range members {
		buf.WritePackedGUID(m.guid)
		buf.WriteU8(0) // unk
	}
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_FILTER_GUILD), buf.Bytes(), true) == nil
}

// handleCalendarArenaTeam processes CMSG_CALENDAR_ARENA_TEAM (0x42C).
// Reference: WorldSession::HandleCalendarArenaTeam (CalendarHandler.cpp:210-219)
// & ArenaTeam::MassInviteToEvent (ArenaTeam.cpp:582-595): unknown team id
// sends nothing, the requester is excluded, and the per-member trailing
// byte is u8(0) (unk).
func (s *session) handleCalendarArenaTeam(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	teamID, _ := r.ReadU32()

	type memberInfo struct {
		guid uint64
	}
	var members []memberInfo

	if teamID > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var teamExists int
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM arena_team WHERE arenateamid = ?", teamID).Scan(&teamExists)
		if teamExists == 0 {
			return true
		}
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
			`SELECT c.guid FROM characters c
			 JOIN arena_team_member atm ON atm.guid = c.guid
			 WHERE atm.arenateamid = ? AND c.guid != ?`, teamID, s.playerGUID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m memberInfo
				if err := rows.Scan(&m.guid); err == nil {
					members = append(members, m)
				}
			}
		}
	}

	buf := protocol.NewBuffer(4 + len(members)*10)
	buf.WriteU32(uint32(len(members)))
	for _, m := range members {
		buf.WritePackedGUID(m.guid)
		buf.WriteU8(0) // unk
	}
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_ARENA_TEAM), buf.Bytes(), true) == nil
}

// handleCalendarAddEvent processes CMSG_CALENDAR_ADD_EVENT (0x42D).
// Reference: WorldSession::HandleCalendarAddEvent (CalendarHandler.cpp:221-268)
// & WorldPackets::Calendar::CalendarAddEvent::Read (CalendarPackets.cpp:123-137):
// past-time gate (with the -86400s hack) answers CALENDAR_ERROR_EVENT_PASSED,
// guild events/announcements require guild membership, then the per-guild
// (100) / per-player (30) caps and the 5s creation cooldown gate; the
// client invite list is capped at CALENDAR_MAX_INVITES (100); guild
// announcements store no invites at all (AddInvite is a no-op for them).
func (s *session) handleCalendarAddEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	title, _ := r.ReadCString()
	description, _ := r.ReadCString()
	eventType, _ := r.ReadU8()
	_, _ = r.ReadU8()  // repeatable
	_, _ = r.ReadU32() // maxInvites
	dungeonID, _ := r.ReadI32()
	packedEventTime, _ := r.ReadU32()
	packedLockDate, _ := r.ReadU32()
	flags, _ := r.ReadU32()

	inviteCount, _ := r.ReadU32()
	type rawInvite struct {
		guid      uint64
		status    uint8
		moderator uint8
	}
	var rawInvites []rawInvite
	for i := uint32(0); i < inviteCount && i < calendarMaxInvites; i++ {
		invGuid, err := r.ReadPackedGUID()
		if err != nil {
			break
		}
		status, _ := r.ReadU8()
		moderator, _ := r.ReadU8()
		rawInvites = append(rawInvites, rawInvite{guid: invGuid, status: status, moderator: moderator})
	}

	if calendarEventInPast(packedEventTime) {
		return s.sendCalendarCommandResult(CalendarErrorEventPassed)
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		if errCode := calendarCreateGateError(ctx, cdb, s.playerGUID, s.player.GuildID, flags); errCode != CalendarOk {
			return s.sendCalendarCommandResult(errCode)
		}
		if !calendarTakeCreateCooldown(s) {
			return s.sendCalendarCommandResult(CalendarErrorInternal)
		}

		var nextID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_events").Scan(&nextID)
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nextID, s.playerGUID, title, description, eventType, dungeonID, packedEventTime, flags, packedLockDate)

		// Guild announcements carry a single Empty-GUID NOT_SIGNED_UP invite
		// that CalendarMgr::AddInvite neither stores nor broadcasts
		// (CalendarMgr.cpp:147-160), so no invites are inserted for them.
		if flags&calendarFlagWithoutInvites == 0 {
			// Insert creator invite
			var nextInviteID uint64 = 1
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
			_, _ = cdb.ExecContext(ctx,
				`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
				 VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
				nextInviteID, nextID, s.playerGUID, s.playerGUID, CalendarStatusAccepted, time.Now().Unix(), CalendarRankCreator)

			// Insert additional invites
			for _, inv := range rawInvites {
				if inv.guid == s.playerGUID || inv.guid == 0 {
					continue
				}
				nextInviteID++
				_, _ = cdb.ExecContext(ctx,
					`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
					 VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
					nextInviteID, nextID, inv.guid, s.playerGUID, inv.status, calendarDefaultResponseTime, inv.moderator)

				// Send SMSG_CALENDAR_EVENT_INVITE_ALERT to online invitee
				if otherSess := s.server.findSessionByGUID(inv.guid); otherSess != nil {
					alertBuf := protocol.NewBuffer(64 + len(title))
					alertBuf.WriteU64(nextID)
					alertBuf.WriteCString(title)
					alertBuf.WritePackedTime(time.Unix(int64(packedEventTime), 0))
					alertBuf.WriteU32(flags)
					alertBuf.WriteU32(uint32(eventType))
					alertBuf.WriteI32(dungeonID)
					alertBuf.WriteU64(nextInviteID)
					alertBuf.WriteU8(inv.status)
					alertBuf.WriteU8(inv.moderator)
					alertBuf.WritePackedGUID(s.playerGUID)
					alertBuf.WritePackedGUID(s.playerGUID)
					_ = otherSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
				}
			}
		}
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarUpdateEvent processes CMSG_CALENDAR_UPDATE_EVENT (0x42E).
// Reference: WorldSession::HandleCalendarUpdateEvent (CalendarHandler.cpp:270-302)
// & WorldPackets::Calendar::CalendarUpdateEvent::Read (CalendarPackets.cpp:139-152):
// the past-time gate returns silently (no error packet), and an unknown
// event answers CALENDAR_ERROR_EVENT_INVALID.
func (s *session) handleCalendarUpdateEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	title, _ := r.ReadCString()
	description, _ := r.ReadCString()
	eventType, _ := r.ReadU8()
	_, _ = r.ReadU8()  // repeatable
	_, _ = r.ReadU32() // maxInvites
	dungeonID, _ := r.ReadI32()
	packedEventTime, _ := r.ReadU32()
	packedLockDate, _ := r.ReadU32()
	flags, _ := r.ReadU32()

	if calendarEventInPast(packedEventTime) {
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var exists int
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM calendar_events WHERE id = ?", eventID).Scan(&exists)
		if exists == 0 {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}

		var oldEventTime uint32
		_ = cdb.QueryRowContext(ctx, "SELECT eventtime FROM calendar_events WHERE id = ?", eventID).Scan(&oldEventTime)

		_, _ = cdb.ExecContext(ctx,
			`UPDATE calendar_events SET title = ?, description = ?, type = ?, dungeon = ?, eventtime = ?, flags = ?, time2 = ?
			 WHERE id = ? AND (creator = ? OR id IN (SELECT event FROM calendar_invites WHERE invitee = ? AND rank IN (1, 2)))`,
			title, description, eventType, dungeonID, packedEventTime, flags, packedLockDate, eventID, s.playerGUID, s.playerGUID)

		// SendCalendarEventUpdateAlert (CalendarMgr.cpp:515-533): u8(1), u64 event
		// id, packed old event time, u32 flags, packed event time, u8 type,
		// i32 dungeon, title, description, u8(0) repeatable, u32(100)
		// maxInvites, u32(0); to every event relative.
		updBuf := protocol.NewBuffer(64 + len(title) + len(description))
		updBuf.WriteU8(1) // unk
		updBuf.WriteU64(eventID)
		updBuf.WritePackedTime(time.Unix(int64(oldEventTime), 0))
		updBuf.WriteU32(flags)
		updBuf.WritePackedTime(time.Unix(int64(packedEventTime), 0))
		updBuf.WriteU8(eventType)
		updBuf.WriteI32(dungeonID)
		updBuf.WriteCString(title)
		updBuf.WriteCString(description)
		updBuf.WriteU8(0)    // repeatable
		updBuf.WriteU32(100) // maxInvites
		updBuf.WriteU32(0)   // unk
		for _, t := range calendarEventRelativeSessions(ctx, s.server, eventID) {
			_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_UPDATED_ALERT), updBuf.Bytes(), true)
		}
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarRemoveEvent processes CMSG_CALENDAR_REMOVE_EVENT (0x42F).
// Reference: WorldSession::HandleCalendarRemoveEvent (CalendarHandler.cpp:304-308).
func (s *session) handleCalendarRemoveEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	_, _ = r.ReadU8()  // isSignUp

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evTime uint32
		var evTitle string
		_ = cdb.QueryRowContext(ctx, "SELECT eventtime, title FROM calendar_events WHERE id = ?", eventID).Scan(&evTime, &evTitle)

		// SendCalendarEventRemovedAlert (CalendarMgr.cpp:549-557): u8(1), u64
		// event id, packed event time; to every event relative, before the
		// invites and the event are deleted.
		remBuf := protocol.NewBuffer(16)
		remBuf.WriteU8(1) // FIXME: If true does not SignalEvent(EVENT_CALENDAR_ACTION_PENDING)
		remBuf.WriteU64(eventID)
		remBuf.WritePackedTime(time.Unix(int64(evTime), 0))
		for _, t := range calendarEventRelativeSessions(ctx, s.server, eventID) {
			_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_REMOVED_ALERT), remBuf.Bytes(), true)
		}

		// CalendarMgr::RemoveEvent (CalendarMgr.cpp:177-217): when an event is
		// deleted, every invitee except the remover gets a calendar mail
		// (MailDraft(subject, body) with MAIL_CHECK_MASK_COPIED). Subject is
		// removerGUID:title, body is the packed event time as a decimal string
		// (CalendarEvent::BuildCalendarMailSubject/BuildCalendarMailBody).
		// MailSender(CalendarEvent*) -> MAIL_CALENDAR (5), sender = event id,
		// MAIL_STATIONERY_DEFAULT; the 30-day expiry arm applies.
		now := time.Now().Unix()
		mailBody := strconv.FormatUint(uint64(protocol.PackTime(time.Unix(int64(evTime), 0))), 10)
		mailRows, mailErr := cdb.QueryContext(ctx, "SELECT invitee FROM calendar_invites WHERE event = ?", eventID)
		if mailErr == nil {
			defer mailRows.Close()
			for mailRows.Next() {
				var inviteeGUID uint64
				if err := mailRows.Scan(&inviteeGUID); err != nil || inviteeGUID == s.playerGUID {
					continue
				}
				var nextMailID int64
				_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
				if nextMailID <= 0 {
					nextMailID = 1
				}
				subject := strconv.FormatUint(s.playerGUID, 10) + ":" + evTitle
				_, _ = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
					VALUES (?, 5, 41, 0, ?, ?, ?, ?, 0, ?, ?, 0, 0, 4)`,
					nextMailID, uint32(eventID), inviteeGUID, subject, mailBody, now+mailSendExpireDelay(false, 0), now)
				s.sendMailNotify(inviteeGUID)
			}
		}

		_, _ = cdb.ExecContext(ctx, "DELETE FROM calendar_events WHERE id = ? AND (creator = ? OR id IN (SELECT event FROM calendar_invites WHERE invitee = ? AND rank = 2))", eventID, s.playerGUID, s.playerGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM calendar_invites WHERE event = ?", eventID)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarCopyEvent processes CMSG_CALENDAR_COPY_EVENT (0x430).
// Reference: WorldSession::HandleCalendarCopyEvent (CalendarHandler.cpp:310-389):
// the past-time gate answers CALENDAR_ERROR_EVENT_PASSED; guild events and
// announcements may only be copied by a member of the event's guild,
// anything else only by the event's creator
// (CALENDAR_ERROR_EVENT_INVALID); the per-guild/per-player caps and the 5s
// creation cooldown gate before the copy; the copy keeps the original
// creator (CalendarEvent copy ctor, CalendarMgr.h:201-213).
func (s *session) handleCalendarCopyEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	packedEventTime, _ := r.ReadU32()

	if calendarEventInPast(packedEventTime) {
		return s.sendCalendarCommandResult(CalendarErrorEventPassed)
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var creator uint64
		var flags uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT creator, flags FROM calendar_events WHERE id = ?", eventID).Scan(&creator, &flags); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}

		if flags&(calendarFlagGuildEvent|calendarFlagWithoutInvites) != 0 {
			if calendarCreatorGuildID(ctx, cdb, creator) != s.player.GuildID {
				return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
			}
		} else if creator != s.playerGUID {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}

		if errCode := calendarCreateGateError(ctx, cdb, s.playerGUID, s.player.GuildID, flags); errCode != CalendarOk {
			return s.sendCalendarCommandResult(errCode)
		}
		if !calendarTakeCreateCooldown(s) {
			return s.sendCalendarCommandResult(CalendarErrorInternal)
		}

		var nextID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_events").Scan(&nextID)
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 SELECT ?, creator, title, description, type, dungeon, ?, flags, time2 FROM calendar_events WHERE id = ?`,
			nextID, packedEventTime, eventID)

		rows, err := cdb.QueryContext(ctx, "SELECT invitee, sender, status, statustime, rank, text FROM calendar_invites WHERE event = ?", eventID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var invitee, sender, status, statustime, rank int64
				var text string
				if rows.Scan(&invitee, &sender, &status, &statustime, &rank, &text) == nil {
					var nextInviteID uint64 = 1
					_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
					_, _ = cdb.ExecContext(ctx,
						"INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
						nextInviteID, nextID, invitee, sender, status, statustime, rank, text)
				}
			}
		}
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventInvite processes CMSG_CALENDAR_EVENT_INVITE (0x431).
// Reference: WorldSession::HandleCalendarEventInvite (CalendarHandler.cpp:391-450):
// empty names are dropped silently; unknown names answer
// CALENDAR_ERROR_PLAYER_NOT_FOUND; cross-faction invites answer
// CALENDAR_ERROR_NOT_ALLIED unless AllowTwoSide.Interaction.Calendar; an
// invitee ignoring the inviter answers CALENDAR_ERROR_IGNORING_YOU_S with
// the name; on an existing event, inviting a same-guild member to a guild
// event answers CALENDAR_ERROR_NO_GUILD_INVITES, and a missing event
// answers CALENDAR_ERROR_EVENT_INVALID; a pre-invite (the event does not
// exist yet) answers CALENDAR_ERROR_NO_GUILD_INVITES when a guild-context
// invite targets a guildmate, else the SMSG_CALENDAR_EVENT_INVITE goes
// straight back to the sender with the packet's invite id and event 0
// (CalendarMgr::SendCalendarEventInvite pre-invite arm, CalendarMgr.cpp:481).
func (s *session) handleCalendarEventInvite(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	inviteID, _ := r.ReadU64()
	name, _ := r.ReadCString()
	isPreInvite, _ := r.ReadU8()
	isGuildEvent, _ := r.ReadU8()

	if name == "" {
		return true
	}

	// sendCalendarEventInviteToSender serializes SMSG_CALENDAR_EVENT_INVITE
	// (CalendarMgr::SendCalendarEventInvite, CalendarMgr.cpp:481-514):
	// packed invitee, event id, invite id, level, status, hasStatusTime
	// (never set here: new invites carry the default 946684800 response
	// time), then sender != invitee.
	sendCalendarEventInviteToSender := func(targetGUID uint64, targetLevel uint8, event, invite uint64) bool {
		invBuf := protocol.NewBuffer(32)
		invBuf.WritePackedGUID(targetGUID)
		invBuf.WriteU64(event)
		invBuf.WriteU64(invite)
		invBuf.WriteU8(targetLevel)
		invBuf.WriteU8(CalendarStatusInvited)
		invBuf.WriteU8(0) // hasStatusTime
		invBuf.WriteU8(1) // sender != invitee
		return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invBuf.Bytes(), true) == nil
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var targetGUID uint64
		var targetRace uint8
		var targetLevel uint8
		if err := cdb.QueryRowContext(ctx,
			"SELECT guid, race, level FROM characters WHERE name = ? COLLATE NOCASE", name).
			Scan(&targetGUID, &targetRace, &targetLevel); err != nil || targetGUID == 0 {
			return s.sendCalendarCommandResult(CalendarErrorPlayerNotFound)
		}
		var inviteeGuildID uint32
		_ = cdb.QueryRowContext(ctx,
			"SELECT COALESCE(guildid, 0) FROM guild_member WHERE guid = ?", targetGUID).Scan(&inviteeGuildID)

		if !s.server.Config.AllowTwoSideInteractionCalendar &&
			teamForRace(s.player.Race) != teamForRace(targetRace) {
			return s.sendCalendarCommandResult(CalendarErrorNotAllied)
		}

		var ignoreFlags uint8
		_ = cdb.QueryRowContext(ctx,
			"SELECT flags FROM character_social WHERE guid = ? AND friend = ?",
			targetGUID, s.playerGUID).Scan(&ignoreFlags)
		if ignoreFlags&socialFlagIgnored != 0 {
			return s.sendCalendarCommandResult(CalendarErrorIgnoringYouS, name)
		}

		if isPreInvite != 0 {
			if isGuildEvent != 0 && inviteeGuildID == s.player.GuildID {
				return s.sendCalendarCommandResult(CalendarErrorNoGuildInvites)
			}
			sendCalendarEventInviteToSender(targetGUID, targetLevel, 0, inviteID)
			return s.sendCalendarCommandResult(CalendarOk)
		}

		var evFlags uint32
		var evCreator uint64
		if err := cdb.QueryRowContext(ctx,
			"SELECT flags, creator FROM calendar_events WHERE id = ?", eventID).Scan(&evFlags, &evCreator); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}

		if evFlags&calendarFlagGuildEvent != 0 &&
			calendarCreatorGuildID(ctx, cdb, evCreator) == inviteeGuildID {
			return s.sendCalendarCommandResult(CalendarErrorNoGuildInvites)
		}

		var nextInviteID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
		_, _ = cdb.ExecContext(ctx,
			"INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text) VALUES (?, ?, ?, ?, ?, ?, ?, '')",
			nextInviteID, eventID, targetGUID, s.playerGUID, CalendarStatusInvited, calendarDefaultResponseTime, CalendarRankPlayer)

		sendCalendarEventInviteToSender(targetGUID, targetLevel, eventID, nextInviteID)

		// Send SMSG_CALENDAR_EVENT_INVITE_ALERT to target player if online
		if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil {
			var evTitle string
			var evType uint32
			var evDungeon int32
			var evTime uint32
			_ = cdb.QueryRowContext(ctx, "SELECT title, type, dungeon, eventtime FROM calendar_events WHERE id = ?", eventID).
				Scan(&evTitle, &evType, &evDungeon, &evTime)

			alertBuf := protocol.NewBuffer(64 + len(evTitle))
			alertBuf.WriteU64(eventID)
			alertBuf.WriteCString(evTitle)
			alertBuf.WritePackedTime(time.Unix(int64(evTime), 0))
			alertBuf.WriteU32(evFlags)
			alertBuf.WriteU32(evType)
			alertBuf.WriteI32(evDungeon)
			alertBuf.WriteU64(nextInviteID)
			alertBuf.WriteU8(CalendarStatusInvited)
			alertBuf.WriteU8(CalendarRankPlayer)
			alertBuf.WritePackedGUID(evCreator)
			alertBuf.WritePackedGUID(s.playerGUID)
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
		}
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventRSVP processes CMSG_CALENDAR_EVENT_RSVP (0x432).
// Reference: WorldSession::HandleCalendarEventRsvp (CalendarHandler.cpp:608-628):
// RSVPing any status other than REMOVED on a locked event answers
// CALENDAR_ERROR_EVENT_LOCKED; an unknown event answers
// CALENDAR_ERROR_EVENT_INVALID; an unknown invite answers
// CALENDAR_ERROR_NO_INVITE.
func (s *session) handleCalendarEventRSVP(ctx context.Context, payload []byte) bool {
	if len(payload) < 20 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	status, _ := r.ReadU32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evFlags uint32
		var evTime uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT flags, eventtime FROM calendar_events WHERE id = ?", eventID).Scan(&evFlags, &evTime); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		if status != CalendarStatusRemoved && evFlags&calendarFlagInvitesLocked != 0 {
			return s.sendCalendarCommandResult(CalendarErrorEventLocked)
		}
		var rank uint8
		if err := cdb.QueryRowContext(ctx,
			"SELECT rank FROM calendar_invites WHERE id = ?", inviteID).Scan(&rank); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}

		now := time.Now().Unix()
		_, _ = cdb.ExecContext(ctx,
			"UPDATE calendar_invites SET status = ?, statustime = ? WHERE id = ?",
			status, now, inviteID)

		// SendCalendarEventStatus (CalendarMgr.cpp:535-547) carries the updated
		// status, the invite's rank, and the just-set status time.
		sendCalendarEventStatusToRelatives(s.server, calendarEventRelativeSessions(ctx, s.server, eventID),
			s.playerGUID, eventID, int64(evTime), evFlags, uint8(status), rank, now)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventRemoveInvite processes CMSG_CALENDAR_EVENT_REMOVE_INVITE (0x433).
// Reference: WorldSession::HandleCalendarEventRemoveInvite (CalendarHandler.cpp:640-665):
// removing the event creator answers CALENDAR_ERROR_DELETE_CREATOR_FAILED;
// an unknown event answers CALENDAR_ERROR_NO_INVITE (not EVENT_INVALID);
// CalendarMgr::RemoveInvite (CalendarMgr.cpp:216) only ever deletes the
// invite matching both the invite id and the event id.
func (s *session) handleCalendarEventRemoveInvite(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	eventID, _ := r.ReadU64()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var creator uint64
		if err := cdb.QueryRowContext(ctx,
			"SELECT creator FROM calendar_events WHERE id = ?", eventID).Scan(&creator); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}
		if creator == inviteeGUID {
			return s.sendCalendarCommandResult(CalendarErrorDeleteCreatorFailed)
		}

		_, _ = cdb.ExecContext(ctx,
			"DELETE FROM calendar_invites WHERE id = ? AND event = ?", inviteID, eventID)

		// Send SMSG_CALENDAR_EVENT_INVITE_REMOVED
		remBuf := protocol.NewBuffer(24)
		remBuf.WritePackedGUID(inviteeGUID)
		remBuf.WriteU64(eventID)
		remBuf.WriteU32(0) // flags
		remBuf.WriteU8(0)  // clearPending
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_REMOVED), remBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventStatus processes CMSG_CALENDAR_EVENT_STATUS (0x434).
// Reference: WorldSession::HandleCalendarEventStatus (CalendarHandler.cpp:696):
// an unknown event answers CALENDAR_ERROR_EVENT_INVALID, an unknown invite
// answers CALENDAR_ERROR_NO_INVITE; the invite's status is updated by invite id
// only and the status time is left untouched (the C++ SetStatusTime call is
// commented out: "not sure if we should set response time when moderator
// changes invite status"); the SMSG_CALENDAR_EVENT_STATUS goes to every event
// relative, then SendCalendarClearPendingAction targets the invitee.
func (s *session) handleCalendarEventStatus(ctx context.Context, payload []byte) bool {
	if len(payload) < 25 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadPackedGUID() // invitee (the invite's DB row is authoritative)
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	status, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evTime uint32
		var evFlags uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT eventtime, flags FROM calendar_events WHERE id = ?", eventID).Scan(&evTime, &evFlags); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		var invInvitee uint64
		var rank uint8
		var statusTime int64
		if err := cdb.QueryRowContext(ctx,
			"SELECT invitee, rank, statustime FROM calendar_invites WHERE id = ?", inviteID).
			Scan(&invInvitee, &rank, &statusTime); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}

		_, _ = cdb.ExecContext(ctx,
			"UPDATE calendar_invites SET status = ? WHERE id = ?",
			status, inviteID)

		sendCalendarEventStatusToRelatives(s.server, calendarEventRelativeSessions(ctx, s.server, eventID),
			invInvitee, eventID, int64(evTime), evFlags, status, rank, statusTime)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventModeratorStatus processes CMSG_CALENDAR_EVENT_MODERATOR_STATUS (0x435).
// Reference: WorldSession::HandleCalendarEventModeratorStatus (CalendarHandler.cpp:730):
// an unknown event answers CALENDAR_ERROR_EVENT_INVALID, an unknown invite
// answers CALENDAR_ERROR_NO_INVITE; the invite's rank is updated by invite id
// only; SMSG_CALENDAR_EVENT_MODERATOR_STATUS_ALERT goes to every event relative.
func (s *session) handleCalendarEventModeratorStatus(ctx context.Context, payload []byte) bool {
	if len(payload) < 25 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadPackedGUID() // invitee
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	rank, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var eventExists int
		_ = cdb.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM calendar_events WHERE id = ?", eventID).Scan(&eventExists)
		if eventExists == 0 {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		var invInvitee uint64
		if err := cdb.QueryRowContext(ctx,
			"SELECT invitee FROM calendar_invites WHERE id = ?", inviteID).Scan(&invInvitee); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}

		_, _ = cdb.ExecContext(ctx,
			"UPDATE calendar_invites SET rank = ? WHERE id = ?",
			rank, inviteID)

		sendCalendarModeratorStatusToRelatives(s.server, calendarEventRelativeSessions(ctx, s.server, eventID),
			invInvitee, eventID, rank)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarEventSignup processes CMSG_CALENDAR_EVENT_SIGNUP (0x4BA).
// Reference: WorldSession::HandleCalendarEventSignup (CalendarHandler.cpp:576-602):
// signing up for a guild event of another guild answers
// CALENDAR_ERROR_GUILD_PLAYER_NOT_IN_GUILD; an unknown event answers
// CALENDAR_ERROR_EVENT_INVALID.
func (s *session) handleCalendarEventSignup(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return s.sendCalendarCommandResult(CalendarErrorInternal)
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, _ := r.ReadU64()
	tentative, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evFlags uint32
		var evCreator uint64
		var evTime uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT flags, creator, eventtime FROM calendar_events WHERE id = ?", eventID).Scan(&evFlags, &evCreator, &evTime); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		if evFlags&calendarFlagGuildEvent != 0 &&
			calendarCreatorGuildID(ctx, cdb, evCreator) != s.player.GuildID {
			return s.sendCalendarCommandResult(CalendarErrorGuildPlayerNotInGuild)
		}

		status := CalendarStatusSignedUp
		if tentative != 0 {
			status = CalendarStatusTentative
		}
		var nextInviteID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
		statTime := time.Now().Unix()
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
			 VALUES (?, ?, ?, ?, ?, ?, 0, '')`,
			nextInviteID, eventID, s.playerGUID, s.playerGUID, status, statTime)

		sendCalendarEventStatusToRelatives(s.server, calendarEventRelativeSessions(ctx, s.server, eventID),
			s.playerGUID, eventID, int64(evTime), evFlags, uint8(status), CalendarRankPlayer, statTime)
	}
	return s.sendCalendarCommandResult(CalendarOk)
}

// handleCalendarComplain processes CMSG_CALENDAR_COMPLAIN (0x446).
// Reference: WorldSession::HandleCalendarComplain (CalendarHandler.cpp:760).
func (s *session) handleCalendarComplain(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, _ := r.ReadU64()
	complainGUID, _ := r.ReadU64()
	s.debug("calendar complain", "account", s.accountName, "event", eventID, "guid", complainGUID)
	return true
}
