package world

import (
	"context"
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
)

// Calendar send type constants.
const (
	CalendarSendTypeGet    = 0
	CalendarSendTypeAdd    = 1
	CalendarSendTypeCopy   = 2
	CalendarSendTypeUpdate = 3
)

// Calendar error codes matching TrinityCore 3.3.5 CalendarError enum.
const (
	CalendarOk                     = 0
	CalendarErrorGuildEventsLimit  = 1
	CalendarErrorEventsExceeded    = 2
	CalendarErrorSelfInvitesLimit  = 3
	CalendarErrorOtherInvitesLimit = 4
	CalendarErrorNoGuildInvites    = 5
	CalendarErrorInvitesExceeded   = 6
	CalendarErrorEventPassed       = 7
	CalendarErrorEventLocked       = 8
	CalendarErrorDeleteCreatorFail = 9
	CalendarErrorSystemDisabled    = 10
	CalendarErrorRestrictedAccount = 11
	CalendarErrorArenaLimit        = 12
	CalendarErrorRestrictedLevel   = 13
	CalendarErrorUserSquelched     = 14
	CalendarErrorNoInvite          = 15
	CalendarErrorWrongEventType    = 16
	CalendarErrorEventStarted      = 17
	CalendarErrorEventInvalid      = 18
	CalendarErrorCalendarNotFound  = 19
	CalendarErrorEventNotFound     = 20
	CalendarErrorInviteNotFound    = 21
	CalendarErrorPlayerNotFound    = 22
	CalendarErrorNotAllied         = 23
	CalendarErrorIgnoringYou       = 24
	CalendarErrorInvitesExceededS  = 25
	CalendarErrorAlreadyInvited    = 26
	CalendarErrorInternal          = 27
	CalendarErrorPlayerNotFound2   = 28
)

// sendCalendarCommandResult serializes and dispatches SMSG_CALENDAR_COMMAND_RESULT (0x43D).
// Reference: WorldPackets::Calendar::CalendarCommandResult::Write (CalendarPackets.cpp:384-392).
func (s *session) sendCalendarCommandResult(cmd uint32, err uint32, name ...string) bool {
	targetName := ""
	if len(name) > 0 {
		targetName = name[0]
	}
	buf := protocol.NewBuffer(16 + len(targetName))
	buf.WriteU32(cmd)
	buf.WriteCString("")
	buf.WriteCString(targetName)
	buf.WriteU32(err)
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_COMMAND_RESULT), buf.Bytes(), true) == nil
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
			return s.sendCalendarCommandResult(0, CalendarErrorEventInvalid)
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
		return s.sendCalendarCommandResult(0, CalendarErrorEventInvalid)
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
// Reference: WorldSession::HandleCalendarGuildFilter (CalendarHandler.cpp:170-177).
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
		guid  uint64
		level uint8
	}
	var members []memberInfo

	if s.player.GuildID > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
			`SELECT c.guid, c.level FROM characters c
			 JOIN guild_member gm ON gm.guid = c.guid
			 WHERE gm.guildid = ? AND c.level >= ? AND c.level <= ? AND gm.rank <= ?`,
			s.player.GuildID, minLevel, maxLevel, minRank)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m memberInfo
				if err := rows.Scan(&m.guid, &m.level); err == nil {
					members = append(members, m)
				}
			}
		}
	}

	buf := protocol.NewBuffer(4 + len(members)*10)
	buf.WriteU32(uint32(len(members)))
	for _, m := range members {
		buf.WritePackedGUID(m.guid)
		buf.WriteU8(m.level)
	}
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_FILTER_GUILD), buf.Bytes(), true) == nil
}

// handleCalendarArenaTeam processes CMSG_CALENDAR_ARENA_TEAM (0x42C).
// Reference: WorldSession::HandleCalendarArenaTeam (CalendarHandler.cpp:179-185).
func (s *session) handleCalendarArenaTeam(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	teamID, _ := r.ReadU32()

	type memberInfo struct {
		guid  uint64
		level uint8
	}
	var members []memberInfo

	if teamID > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
			`SELECT c.guid, c.level FROM characters c
			 JOIN arena_team_member atm ON atm.guid = c.guid
			 WHERE atm.arenateamid = ?`, teamID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m memberInfo
				if err := rows.Scan(&m.guid, &m.level); err == nil {
					members = append(members, m)
				}
			}
		}
	}

	buf := protocol.NewBuffer(4 + len(members)*10)
	buf.WriteU32(uint32(len(members)))
	for _, m := range members {
		buf.WritePackedGUID(m.guid)
		buf.WriteU8(m.level)
	}
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_ARENA_TEAM), buf.Bytes(), true) == nil
}

// handleCalendarAddEvent processes CMSG_CALENDAR_ADD_EVENT (0x42D).
// Reference: WorldSession::HandleCalendarAddEvent (CalendarHandler.cpp:187-268)
// & WorldPackets::Calendar::CalendarAddEvent::Read (CalendarPackets.cpp:123-137).
func (s *session) handleCalendarAddEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return s.sendCalendarCommandResult(0, CalendarErrorInternal)
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
	for i := uint32(0); i < inviteCount; i++ {
		invGuid, err := r.ReadPackedGUID()
		if err != nil {
			break
		}
		status, _ := r.ReadU8()
		moderator, _ := r.ReadU8()
		rawInvites = append(rawInvites, rawInvite{guid: invGuid, status: status, moderator: moderator})
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var nextID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_events").Scan(&nextID)
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nextID, s.playerGUID, title, description, eventType, dungeonID, packedEventTime, flags, packedLockDate)

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
				nextInviteID, nextID, inv.guid, s.playerGUID, inv.status, time.Now().Unix(), inv.moderator)

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
	return s.sendCalendarCommandResult(0, CalendarOk)
}

// handleCalendarUpdateEvent processes CMSG_CALENDAR_UPDATE_EVENT (0x42E).
// Reference: WorldSession::HandleCalendarUpdateEvent (CalendarHandler.cpp:270-302)
// & WorldPackets::Calendar::CalendarUpdateEvent::Read (CalendarPackets.cpp:139-152).
func (s *session) handleCalendarUpdateEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return s.sendCalendarCommandResult(1, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(1, CalendarErrorInternal)
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

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var oldEventTime uint32
		_ = cdb.QueryRowContext(ctx, "SELECT eventtime FROM calendar_events WHERE id = ?", eventID).Scan(&oldEventTime)

		_, _ = cdb.ExecContext(ctx,
			`UPDATE calendar_events SET title = ?, description = ?, type = ?, dungeon = ?, eventtime = ?, flags = ?, time2 = ?
			 WHERE id = ? AND (creator = ? OR id IN (SELECT event FROM calendar_invites WHERE invitee = ? AND rank IN (1, 2)))`,
			title, description, eventType, dungeonID, packedEventTime, flags, packedLockDate, eventID, s.playerGUID, s.playerGUID)

		// Alert connected invitees about event update
		invRows, err := cdb.QueryContext(ctx, "SELECT invitee FROM calendar_invites WHERE event = ?", eventID)
		if err == nil {
			defer invRows.Close()
			for invRows.Next() {
				var inviteeGUID uint64
				if err := invRows.Scan(&inviteeGUID); err == nil && inviteeGUID != s.playerGUID {
					if otherSess := s.server.findSessionByGUID(inviteeGUID); otherSess != nil {
						updBuf := protocol.NewBuffer(64 + len(title) + len(description))
						updBuf.WriteU8(0) // clearPending
						updBuf.WriteU64(eventID)
						updBuf.WritePackedTime(time.Unix(int64(oldEventTime), 0))
						updBuf.WriteU32(flags)
						updBuf.WritePackedTime(time.Unix(int64(packedEventTime), 0))
						updBuf.WriteU8(eventType)
						updBuf.WriteU32(uint32(dungeonID))
						updBuf.WriteCString(title)
						updBuf.WriteCString(description)
						updBuf.WriteU8(0)    // repeatable
						updBuf.WriteU32(100) // maxInvites
						updBuf.WritePackedTime(time.Unix(int64(packedLockDate), 0))
						_ = otherSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_UPDATED_ALERT), updBuf.Bytes(), true)
					}
				}
			}
		}
	}
	return s.sendCalendarCommandResult(1, CalendarOk)
}

// handleCalendarRemoveEvent processes CMSG_CALENDAR_REMOVE_EVENT (0x42F).
// Reference: WorldSession::HandleCalendarRemoveEvent (CalendarHandler.cpp:304-308).
func (s *session) handleCalendarRemoveEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return s.sendCalendarCommandResult(2, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(2, CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	_, _ = r.ReadU8()  // isSignUp

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var evTime uint32
		var evTitle string
		_ = cdb.QueryRowContext(ctx, "SELECT eventtime, title FROM calendar_events WHERE id = ?", eventID).Scan(&evTime, &evTitle)

		// Alert connected invitees before deleting
		invRows, err := cdb.QueryContext(ctx, "SELECT invitee FROM calendar_invites WHERE event = ?", eventID)
		if err == nil {
			defer invRows.Close()
			for invRows.Next() {
				var inviteeGUID uint64
				if err := invRows.Scan(&inviteeGUID); err == nil && inviteeGUID != s.playerGUID {
					if otherSess := s.server.findSessionByGUID(inviteeGUID); otherSess != nil {
						remBuf := protocol.NewBuffer(16)
						remBuf.WriteU8(0) // clearPending
						remBuf.WriteU64(eventID)
						remBuf.WritePackedTime(time.Unix(int64(evTime), 0))
						_ = otherSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_REMOVED_ALERT), remBuf.Bytes(), true)
					}
				}
			}
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
	return s.sendCalendarCommandResult(2, CalendarOk)
}

// handleCalendarCopyEvent processes CMSG_CALENDAR_COPY_EVENT (0x430).
// Reference: WorldSession::HandleCalendarCopyEvent (CalendarHandler.cpp:310-389).
func (s *session) handleCalendarCopyEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return s.sendCalendarCommandResult(3, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(3, CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	packedEventTime, _ := r.ReadU32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var nextID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_events").Scan(&nextID)
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 SELECT ?, ?, title, description, type, dungeon, ?, flags, time2 FROM calendar_events WHERE id = ?`,
			nextID, s.playerGUID, packedEventTime, eventID)

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
	return s.sendCalendarCommandResult(3, CalendarOk)
}

// handleCalendarEventInvite processes CMSG_CALENDAR_EVENT_INVITE (0x431).
// Reference: WorldSession::HandleCalendarEventInvite (CalendarHandler.cpp:391-450).
func (s *session) handleCalendarEventInvite(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return s.sendCalendarCommandResult(4, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return s.sendCalendarCommandResult(4, CalendarErrorInternal)
	}
	_, _ = r.ReadU64() // moderatorID
	name, _ := r.ReadCString()
	creating, _ := r.ReadU8()
	_ = creating

	if name == "" {
		return s.sendCalendarCommandResult(4, CalendarErrorPlayerNotFound)
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var targetGUID uint64
		var targetLevel uint8 = 1
		err := cdb.QueryRowContext(ctx, "SELECT guid, level FROM characters WHERE name = ? COLLATE NOCASE", name).Scan(&targetGUID, &targetLevel)
		if err != nil || targetGUID == 0 {
			return s.sendCalendarCommandResult(4, CalendarErrorPlayerNotFound)
		}
		var nextInviteID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
		_, _ = cdb.ExecContext(ctx,
			"INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text) VALUES (?, ?, ?, ?, ?, ?, ?, '')",
			nextInviteID, eventID, targetGUID, s.playerGUID, CalendarStatusInvited, time.Now().Unix(), CalendarRankPlayer)

		// Send SMSG_CALENDAR_EVENT_INVITE to the inviter
		invBuf := protocol.NewBuffer(32)
		invBuf.WritePackedGUID(targetGUID)
		invBuf.WriteU64(eventID)
		invBuf.WriteU64(nextInviteID)
		invBuf.WriteU8(targetLevel)
		invBuf.WriteU8(CalendarStatusInvited)
		invBuf.WriteU8(0) // type
		invBuf.WriteU8(0) // clearPending
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invBuf.Bytes(), true)

		// Send SMSG_CALENDAR_EVENT_INVITE_ALERT to target player if online
		if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil {
			var evTitle string
			var evFlags, evType uint32
			var evDungeon int32
			var evTime uint32
			var evCreator uint64
			_ = cdb.QueryRowContext(ctx, "SELECT title, flags, type, dungeon, eventtime, creator FROM calendar_events WHERE id = ?", eventID).
				Scan(&evTitle, &evFlags, &evType, &evDungeon, &evTime, &evCreator)

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
	return s.sendCalendarCommandResult(4, CalendarOk)
}

// handleCalendarEventRSVP processes CMSG_CALENDAR_EVENT_RSVP (0x432).
// Reference: WorldSession::HandleCalendarEventRsvp (CalendarHandler.cpp:630).
func (s *session) handleCalendarEventRSVP(ctx context.Context, payload []byte) bool {
	if len(payload) < 20 {
		return s.sendCalendarCommandResult(5, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	status, _ := r.ReadU32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		now := time.Now().Unix()
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE calendar_invites SET status = ?, statustime = ? WHERE (id = ? OR (event = ? AND invitee = ?))",
			status, now, inviteID, eventID, s.playerGUID)

		// Send SMSG_CALENDAR_EVENT_STATUS
		statBuf := protocol.NewBuffer(32)
		statBuf.WritePackedGUID(s.playerGUID)
		statBuf.WriteU64(eventID)
		statBuf.WritePackedTime(time.Now())
		statBuf.WriteU32(0) // flags
		statBuf.WriteU8(uint8(status))
		statBuf.WriteU8(0) // clearPending
		statBuf.WritePackedTime(time.Unix(now, 0))
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_STATUS), statBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(5, CalendarOk)
}

// handleCalendarEventRemoveInvite processes CMSG_CALENDAR_EVENT_REMOVE_INVITE (0x433).
// Reference: WorldSession::HandleCalendarEventRemoveInvite (CalendarHandler.cpp:667).
func (s *session) handleCalendarEventRemoveInvite(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return s.sendCalendarCommandResult(6, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	eventID, _ := r.ReadU64()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"DELETE FROM calendar_invites WHERE id = ? OR (event = ? AND (invitee = ? OR ? = 0))",
			inviteID, eventID, inviteeGUID, inviteeGUID)

		// Send SMSG_CALENDAR_EVENT_INVITE_REMOVED
		remBuf := protocol.NewBuffer(24)
		remBuf.WritePackedGUID(inviteeGUID)
		remBuf.WriteU64(eventID)
		remBuf.WriteU32(0) // flags
		remBuf.WriteU8(0)  // clearPending
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_REMOVED), remBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(6, CalendarOk)
}

// handleCalendarEventStatus processes CMSG_CALENDAR_EVENT_STATUS (0x434).
// Reference: WorldSession::HandleCalendarEventStatus (CalendarHandler.cpp:696).
func (s *session) handleCalendarEventStatus(ctx context.Context, payload []byte) bool {
	if len(payload) < 25 {
		return s.sendCalendarCommandResult(7, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID()
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	status, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE calendar_invites SET status = ? WHERE id = ? OR (event = ? AND invitee = ?)",
			status, inviteID, eventID, inviteeGUID)

		statBuf := protocol.NewBuffer(32)
		statBuf.WritePackedGUID(inviteeGUID)
		statBuf.WriteU64(eventID)
		statBuf.WritePackedTime(time.Now())
		statBuf.WriteU32(0) // flags
		statBuf.WriteU8(status)
		statBuf.WriteU8(0) // clearPending
		statBuf.WritePackedTime(time.Now())
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_STATUS), statBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(7, CalendarOk)
}

// handleCalendarEventModeratorStatus processes CMSG_CALENDAR_EVENT_MODERATOR_STATUS (0x435).
// Reference: WorldSession::HandleCalendarEventModeratorStatus (CalendarHandler.cpp:730).
func (s *session) handleCalendarEventModeratorStatus(ctx context.Context, payload []byte) bool {
	if len(payload) < 25 {
		return s.sendCalendarCommandResult(8, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID()
	eventID, _ := r.ReadU64()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	rank, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE calendar_invites SET rank = ? WHERE id = ? OR (event = ? AND invitee = ?)",
			rank, inviteID, eventID, inviteeGUID)

		modBuf := protocol.NewBuffer(24)
		modBuf.WritePackedGUID(inviteeGUID)
		modBuf.WriteU64(eventID)
		modBuf.WriteU8(rank)
		modBuf.WriteU8(0) // clearPending
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_MODERATOR_STATUS_ALERT), modBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(8, CalendarOk)
}

// handleCalendarEventSignup processes CMSG_CALENDAR_EVENT_SIGNUP (0x4BA).
// Reference: WorldSession::HandleCalendarEventSignup (CalendarHandler.cpp:604).
func (s *session) handleCalendarEventSignup(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return s.sendCalendarCommandResult(9, CalendarErrorInternal)
	}
	r := protocol.NewReader(payload)
	eventID, _ := r.ReadU64()
	tentative, _ := r.ReadU8()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		status := CalendarStatusSignedUp
		if tentative != 0 {
			status = CalendarStatusTentative
		}
		var nextInviteID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
			 VALUES (?, ?, ?, ?, ?, ?, 0, '')`,
			nextInviteID, eventID, s.playerGUID, s.playerGUID, status, time.Now().Unix())

		statBuf := protocol.NewBuffer(32)
		statBuf.WritePackedGUID(s.playerGUID)
		statBuf.WriteU64(eventID)
		statBuf.WritePackedTime(time.Now())
		statBuf.WriteU32(0) // flags
		statBuf.WriteU8(uint8(status))
		statBuf.WriteU8(0) // clearPending
		statBuf.WritePackedTime(time.Now())
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_STATUS), statBuf.Bytes(), true)
	}
	return s.sendCalendarCommandResult(9, CalendarOk)
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
