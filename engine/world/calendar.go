package world

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Calendar invite status constants matching CalendarInviteStatus
// (CalendarMgr.h:80-89).
const (
	CalendarStatusInvited     = 0
	CalendarStatusAccepted    = 1
	CalendarStatusDeclined    = 2
	CalendarStatusConfirmed   = 3
	CalendarStatusOut         = 4
	CalendarStatusStandby     = 5
	CalendarStatusSignedUp    = 6
	CalendarStatusNotSignedUp = 7
	CalendarStatusTentative   = 8
	CalendarStatusRemoved     = 9
)

// Calendar rank constants matching CalendarModerationRank
// (CalendarMgr.h:47-52): PLAYER, MODERATOR, OWNER — C++ has no creator rank.
const (
	CalendarRankPlayer    = 0
	CalendarRankModerator = 1
	CalendarRankOwner     = 2
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

// Calendar send type constants matching CalendarSendEventType
// (CalendarMgr.h:55-60): GET, ADD, COPY only.
const (
	CalendarSendTypeGet  = 0
	CalendarSendTypeAdd  = 1
	CalendarSendTypeCopy = 2
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

// calendarStdEastOffset returns the server-local standard (DST-blind) UTC
// offset in seconds east of UTC — the Go equivalent of the POSIX `timezone`
// global that LocalTimeToUTCTime adds (Util.cpp:73-80). It is parsed from the
// POSIX TZ string ($TZ when set, else the footer of the zoneinfo file
// time.Local was loaded from), so it is exact for every zone — including
// negative-DST zones like Europe/Dublin where the DST offset sits below the
// standard one, and long-DST zones like America/New_York where DST covers
// most of the year. A wrong offset here shifts every stored eventtime by
// the DST delta (verified against mktime(tm_isdst=0) semantics).
var calendarStdEastOffset = sync.OnceValue(func() int {
	if time.Local == time.UTC {
		return 0
	}
	if east, ok := calendarPOSIXStdEast(); ok {
		return east
	}
	// Last resort: the offset in effect just before the year's first
	// forward (offset-increasing) transition, i.e. the pre-DST offset.
	year := time.Now().Year()
	t := time.Date(year, time.January, 1, 12, 0, 0, 0, time.Local)
	_, prev := t.Zone()
	end := t.AddDate(1, 0, 0)
	for tt := t.Add(24 * time.Hour); tt.Before(end); tt = tt.Add(24 * time.Hour) {
		_, off := tt.Zone()
		if off > prev {
			return prev
		}
		prev = off
	}
	_, off := t.Zone()
	return off
})

// calendarPOSIXStdEast resolves the POSIX TZ string for the local zone and
// extracts its standard offset.
func calendarPOSIXStdEast() (int, bool) {
	tz := os.Getenv("TZ")
	var posix string
	switch {
	case tz == "":
		posix = calendarTZifPOSIX("/etc/localtime")
	case strings.HasPrefix(tz, ":"):
		posix = calendarTZifPOSIX(tz[1:])
	case strings.Contains(tz, "/"):
		if strings.HasPrefix(tz, "/") {
			posix = calendarTZifPOSIX(tz)
		} else {
			posix = calendarTZifPOSIX("/usr/share/zoneinfo/" + tz)
		}
	default:
		posix = tz
	}
	return parsePOSIXStdOffset(posix)
}

// calendarTZifPOSIX reads the POSIX TZ string from a TZif file footer
// (present in v2+ zoneinfo files: newline, TZ string, newline at EOF).
func calendarTZifPOSIX(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	s := strings.TrimRight(string(data), "\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if s == "" || (s[0] != '<' && !isASCIILetter(s[0])) {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return ""
		}
	}
	return s
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// parsePOSIXStdOffset parses the std offset out of a POSIX TZ string
// ("STDoffset[DST[offset]][,rules]") and returns seconds east of UTC. The
// POSIX offset uses the inverted sign convention (it is added to local time
// to get UTC), so the sign is negated.
func parsePOSIXStdOffset(posix string) (int, bool) {
	i := 0
	if i < len(posix) && posix[i] == '<' {
		j := strings.IndexByte(posix, '>')
		if j < 0 {
			return 0, false
		}
		i = j + 1
	} else {
		for i < len(posix) && isASCIILetter(posix[i]) {
			i++
		}
		if i == 0 {
			return 0, false
		}
	}
	neg := false
	if i < len(posix) && (posix[i] == '+' || posix[i] == '-') {
		neg = posix[i] == '-'
		i++
	}
	readNum := func() (int, bool) {
		start := i
		for i < len(posix) && posix[i] >= '0' && posix[i] <= '9' {
			i++
		}
		if start == i {
			return 0, false
		}
		n := 0
		for _, c := range posix[start:i] {
			n = n*10 + int(c-'0')
		}
		return n, true
	}
	hh, ok := readNum()
	if !ok {
		return 0, false
	}
	mm, ss := 0, 0
	if i < len(posix) && posix[i] == ':' {
		i++
		if mm, ok = readNum(); !ok {
			return 0, false
		}
		if i < len(posix) && posix[i] == ':' {
			i++
			if ss, ok = readNum(); !ok {
				return 0, false
			}
		}
	}
	secs := hh*3600 + mm*60 + ss
	if neg {
		secs = -secs
	}
	return -secs, true
}

// calendarPackedToEventTime mirrors the C++ event-time pipeline
// LocalTimeToUTCTime(ReadPackedTime(p)) (CalendarHandler.cpp:240, Util.cpp:73):
// ReadPackedTime zero-initializes tm (tm_isdst=0, ByteBuffer.cpp:92-104),
// so mktime interprets the wall time with the DST-blind standard offset:
// ReadPackedTime = W - east_std (W = wall clock as a UTC epoch). Then
// LocalTimeToUTCTime adds the POSIX `timezone` global (seconds west of UTC,
// i.e. -east_std, DST-blind): eventtime = W - 2*east_std. A DST-aware decode
// (t.Unix() - east_effective) diverges by twice the DST shift in summer.
// The result is what CalendarEvent stores in the eventtime column and what
// AppendPackedTime re-packs on every send path (CalendarMgr.cpp:523+).
func calendarPackedToEventTime(packed uint32) int64 {
	t := unpackCalendarPackedTime(packed)
	_, eff := t.Zone()
	wall := t.Unix() + int64(eff)
	return wall - 2*int64(calendarStdEastOffset())
}

// calendarPackedRawEpoch mirrors ByteBuffer::ReadPackedTime
// (ByteBuffer.cpp:92-104) alone: W - east_std. unkPackedTime is stored
// unconverted (CalendarHandler.cpp:291), so the time2 column holds this.
func calendarPackedRawEpoch(packed uint32) int64 {
	t := unpackCalendarPackedTime(packed)
	_, eff := t.Zone()
	return t.Unix() + int64(eff) - int64(calendarStdEastOffset())
}

// calendarEventInPast mirrors the "prevent events in the past" gate in
// HandleCalendarAddEvent/UpdateEvent/CopyEvent (CalendarHandler.cpp:244):
// C++ compares the LocalTimeToUTCTime-converted packed time against now
// minus the 86400s hack, not the raw client value.
func calendarEventInPast(packed uint32) bool {
	return calendarPackedToEventTime(packed) < time.Now().Unix()-86400
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

// boolToU8 maps a boolean wire arm to its u8 encoding.
func boolToU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// calendarGuildMemberSessions returns every online session whose player is in
// the given guild — the Guild::BroadcastPacket arm used by
// CalendarMgr::SendCalendarEventInviteAlert (CalendarMgr.cpp:581-598) and
// CalendarMgr::SendPacketToAllEventRelatives (CalendarMgr.cpp:700-712).
func calendarGuildMemberSessions(srv *Server, guildID uint32) []*session {
	var targets []*session
	if srv == nil || guildID == 0 {
		return targets
	}
	srv.sessionsMu.RLock()
	for sess := range srv.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.GuildID == guildID {
			targets = append(targets, sess)
		}
	}
	srv.sessionsMu.RUnlock()
	return targets
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
	for _, sess := range calendarGuildMemberSessions(srv, eventGuild) {
		add(sess)
	}
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
		return true
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

		// 2. Events the player is invited to, or guild events/announcements
		// of the player's guild (CalendarMgr::GetPlayerEvents,
		// CalendarMgr.cpp:403-425: invitee rows plus every event whose guild
		// id is the player's guild — creator is not a criterion there).
		evRows, err := cdb.QueryContext(ctx,
			`SELECT id, title, type, dungeon, flags, eventtime, creator FROM calendar_events
			 WHERE id IN (SELECT event FROM calendar_invites WHERE invitee = ?)
			 UNION
			 SELECT e.id, e.title, e.type, e.dungeon, e.flags, e.eventtime, e.creator
			 FROM calendar_events e JOIN guild_member gm ON gm.guid = e.creator
			 WHERE (e.flags & ?) != 0 AND gm.guildid = ? AND ? != 0`,
			s.playerGUID,
			calendarFlagGuildEvent|calendarFlagWithoutInvites, s.player.GuildID, s.player.GuildID)
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
					// CalendarHandler.cpp:109: u32(resetTime - currTime), so a
					// past reset wraps instead of clamping to 0.
					lock.expireTime = int32(resetTime - currTime.Unix())
					lockouts = append(lockouts, lock)
				}
			}
		}

		// 4. Raid global resets
		resetRows, err := cdb.QueryContext(ctx,
			`SELECT mapid, resettime FROM instance_reset`)
		if err == nil {
			defer resetRows.Close()
			sentMaps := make(map[int32]struct{})
			for resetRows.Next() {
				var r calRaidReset
				var resetTime int64
				if err := resetRows.Scan(&r.mapID, &resetTime); err != nil {
					continue
				}
				// CalendarHandler.cpp:130-146: one entry per map, raids only;
				// the duration is int32(resetTime - currTime), negative when
				// the reset already passed.
				if _, seen := sentMaps[r.mapID]; seen {
					continue
				}
				isRaid := false
				if s.server != nil && s.server.Data != nil && r.mapID >= 0 {
					if entry, ok, err := s.server.Data.Map(uint32(r.mapID)); err == nil && ok {
						isRaid = entry.IsRaid()
					}
				}
				if !isRaid {
					continue
				}
				sentMaps[r.mapID] = struct{}{}
				r.duration = int32(resetTime - currTime.Unix())
				r.offset = 0
				raidResets = append(raidResets, r)
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

	// Holidays: CalendarHandler.cpp:149-175 — sGameEventMgr->modifiedHolidays
	// is the sorted id list of holidays carrying rows in the world
	// `holiday_dates` table (GameEventMgr::LoadHolidayDates,
	// GameEventMgr.cpp:965-1010): each row overrides HolidaysEntry.Date[date_id]
	// (and Duration[0] when nonzero). Holidays.dbc supplies the remaining
	// fields; a holiday_dates row for an unknown DBC id is skipped like C++.
	type holidayDateOverride struct {
		dateID   uint8
		date     uint32
		duration uint32
	}
	holidayOrder := []uint32{}
	holidayOverrides := map[uint32][]holidayDateOverride{}
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		if hrows, herr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT id, date_id, date_value, holiday_duration FROM holiday_dates`); herr == nil {
			defer hrows.Close()
			for hrows.Next() {
				var hid, dateValue, dur uint32
				var did uint8
				if herr := hrows.Scan(&hid, &did, &dateValue, &dur); herr != nil || did >= 26 {
					continue
				}
				if _, seen := holidayOverrides[hid]; !seen {
					holidayOrder = append(holidayOrder, hid)
				}
				holidayOverrides[hid] = append(holidayOverrides[hid], holidayDateOverride{dateID: did, date: dateValue, duration: dur})
			}
		}
	}
	sort.Slice(holidayOrder, func(i, j int) bool { return holidayOrder[i] < holidayOrder[j] })

	holidayBuf := protocol.NewBuffer(64 * len(holidayOrder))
	holidayCount := uint32(0)
	if len(holidayOrder) > 0 && s.server != nil && s.server.Data != nil {
		if hfile, herr := s.server.Data.File("Holidays"); herr == nil {
			for _, hid := range holidayOrder {
				rec, ok := hfile.Find(hid)
				if !ok {
					continue
				}
				region, _ := rec.Uint32(37)
				looping, _ := rec.Uint32(38)
				priority, _ := rec.Uint32(52)
				filterType, _ := rec.Int32(53)
				texture, _ := rec.String(51)
				var dates [26]uint32
				for j := range dates {
					dates[j], _ = rec.Uint32(11 + j)
				}
				var durations [10]uint32
				for j := range durations {
					durations[j], _ = rec.Uint32(1 + j)
				}
				var calFlags [10]uint32
				for j := range calFlags {
					calFlags[j], _ = rec.Uint32(39 + j)
				}
				for _, ov := range holidayOverrides[hid] {
					dates[ov.dateID] = ov.date
					if ov.duration != 0 {
						durations[0] = ov.duration
					}
				}
				holidayBuf.WriteU32(hid)
				holidayBuf.WriteU32(region)
				holidayBuf.WriteU32(looping)
				holidayBuf.WriteU32(priority)
				holidayBuf.WriteU32(uint32(filterType))
				for _, d := range dates {
					holidayBuf.WriteU32(d)
				}
				for _, d := range durations {
					holidayBuf.WriteU32(d)
				}
				for _, f := range calFlags {
					holidayBuf.WriteU32(f)
				}
				holidayBuf.WriteCString(texture)
				holidayCount++
			}
		}
	}
	buf.WriteU32(holidayCount)
	buf.Write(holidayBuf.Bytes())

	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_CALENDAR), buf.Bytes(), true) == nil
}

// handleCalendarGetNumPending processes CMSG_CALENDAR_GET_NUM_PENDING (0x447).
// Reference: WorldSession::HandleCalendarGetNumPending (CalendarHandler.cpp:665).
func (s *session) handleCalendarGetNumPending(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	var count uint32 = 0
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		// CalendarMgr::GetPlayerNumPending (CalendarMgr.cpp:439-456): an
		// invite counts when its status is INVITED, NOT_SIGNED_UP, or
		// TENTATIVE.
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM calendar_invites WHERE invitee = ? AND status IN (?, ?, ?)", s.playerGUID,
			CalendarStatusInvited, CalendarStatusNotSignedUp, CalendarStatusTentative).Scan(&count)
	}
	buf := protocol.NewBuffer(4)
	buf.WriteU32(count)
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_NUM_PENDING), buf.Bytes(), true) == nil
}

// handleCalendarGetEvent processes CMSG_CALENDAR_GET_EVENT (0x42A).
// Reference: WorldSession::HandleCalendarGetEvent (CalendarHandler.cpp:181-192):
// the event goes back with CALENDAR_SENDTYPE_GET; an unknown event answers
// CALENDAR_ERROR_EVENT_INVALID.
func (s *session) handleCalendarGetEvent(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
	}
	pkt, ok := s.buildCalendarSendEvent(ctx, eventID, CalendarSendTypeGet)
	if !ok {
		return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
	}
	return s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_EVENT), pkt, true) == nil
}

// buildCalendarSendEvent serializes SMSG_CALENDAR_SEND_EVENT for an event
// (CalendarMgr::SendCalendarEvent, CalendarMgr.cpp:606-651): u8 send type,
// packed creator, u64 id, title, description, u8 type, u8(0) repeatable,
// u32(100) max invites, i32 dungeon, u32 flags, packed event time, packed
// lock time, u32 guild id, then the invite list (packed invitee, u8 level,
// u8 status, u8 rank, u8 guild-invite arm, u64 invite id, packed status
// time, text). ok is false when the event row is missing.
func (s *session) buildCalendarSendEvent(ctx context.Context, eventID uint64, sendType uint8) (pkt []byte, ok bool) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil, false
	}
	cdb := s.server.CharactersStore.DB

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

	var evType32 uint32
	err := cdb.QueryRowContext(ctx,
		"SELECT creator, title, description, type, dungeon, flags, eventtime, time2 FROM calendar_events WHERE id = ?",
		eventID).Scan(&creator, &title, &description, &evType32, &dungeon, &flags, &eventTime, &lockDate)
	if err != nil {
		return nil, false
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

	buf := protocol.NewBuffer(128 + len(title) + len(description) + len(invites)*48)
	buf.WriteU8(sendType)
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

	return buf.Bytes(), true
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
	// CalendarHandler.cpp:201: recvData >> minLevel >> maxLevel >> minRank —
	// a truncated packet throws and the packet is skipped, not zero-filled.
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	minLevel, _ := r.ReadU32()
	maxLevel, _ := r.ReadU32()
	minRank, _ := r.ReadU32()

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

	// CalendarHandler.cpp:204: GetGuildById(0) is null, so a guildless
	// player gets no SMSG at all, not an empty list.
	if s.player.GuildID == 0 {
		return true
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

	// CalendarHandler.cpp:216-217: GetArenaTeamById(0) is null — nothing sent.
	if teamID == 0 {
		return true
	}

	type memberInfo struct {
		guid uint64
	}
	var members []memberInfo

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
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
// buildCalendarInvitePacket serializes SMSG_CALENDAR_EVENT_INVITE
// (CalendarMgr::SendCalendarEventInvite, CalendarMgr.cpp:481-514): packed
// invitee GUID, u64 event id, u64 invite id, u8 level, u8 status,
// u8 hasStatusTime (set unless the status time is the 946684800 default),
// the packed status time when set, then u8(sender != invitee).
func buildCalendarInvitePacket(inviteeGUID, eventID, inviteID uint64, level, status uint8, statusTime int64, senderGUID uint64) []byte {
	buf := protocol.NewBuffer(32)
	buf.WritePackedGUID(inviteeGUID)
	buf.WriteU64(eventID)
	buf.WriteU64(inviteID)
	buf.WriteU8(level)
	buf.WriteU8(status)
	hasStatusTime := statusTime != calendarDefaultResponseTime
	buf.WriteU8(boolToU8(hasStatusTime))
	if hasStatusTime {
		buf.WritePackedTime(time.Unix(statusTime, 0))
	}
	buf.WriteU8(boolToU8(senderGUID != inviteeGUID))
	return buf.Bytes()
}

// buildCalendarInviteAlertPacket serializes SMSG_CALENDAR_EVENT_INVITE_ALERT
// (CalendarMgr::SendCalendarEventInviteAlert, CalendarMgr.cpp:581-604): u64
// event id, title, packed event time, u32 flags, u32 type, i32 dungeon,
// u64 invite id, u8 status, u8 rank, packed creator GUID, packed sender GUID.
func buildCalendarInviteAlertPacket(eventID uint64, title string, eventTime int64, flags uint32, eventType uint32, dungeonID int32, inviteID uint64, status, rank uint8, creatorGUID, senderGUID uint64) []byte {
	buf := protocol.NewBuffer(64 + len(title))
	buf.WriteU64(eventID)
	buf.WriteCString(title)
	buf.WritePackedTime(time.Unix(eventTime, 0))
	buf.WriteU32(flags)
	buf.WriteU32(eventType)
	buf.WriteI32(dungeonID)
	buf.WriteU64(inviteID)
	buf.WriteU8(status)
	buf.WriteU8(rank)
	buf.WritePackedGUID(creatorGUID)
	buf.WritePackedGUID(senderGUID)
	return buf.Bytes()
}

// calendarExecer is satisfied by *sql.DB and *sql.Tx, so the same insert
// loop runs immediately or inside a transaction.
type calendarExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Reference: WorldSession::HandleCalendarAddEvent (CalendarHandler.cpp:221-268)
// & WorldPackets::Calendar::CalendarAddEvent::Read (CalendarPackets.cpp:123-137):
// past-time gate (with the -86400s hack) answers CALENDAR_ERROR_EVENT_PASSED,
// guild events/announcements require guild membership, then the per-guild
// (100) / per-player (30) caps and the 5s creation cooldown gate; the
// client invite list is capped at CALENDAR_MAX_INVITES (100); guild
// announcements store no invites at all (AddInvite is a no-op for them).
func (s *session) handleCalendarAddEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
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

		// CalendarHandler.cpp:313-340: the client invite list is read inside
		// try/catch AFTER the gates — a ByteBufferException deletes the
		// not-yet-added event and rethrows, so no event is created.
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
				return true
			}
			status, err := r.ReadU8()
			if err != nil {
				return true
			}
			moderator, err := r.ReadU8()
			if err != nil {
				return true
			}
			rawInvites = append(rawInvites, rawInvite{guid: invGuid, status: status, moderator: moderator})
		}

		var nextID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_events").Scan(&nextID)
		// C++ stores eventtime as LocalTimeToUTCTime(ReadPackedTime(...))
		// (CalendarHandler.cpp:240) and time2 as the raw ReadPackedTime epoch
		// (unkPackedTime is never converted, CalendarHandler.cpp:291), so the
		// columns hold time_t, not packed bits — every send path re-packs
		// via AppendPackedTime.
		eventTime := uint32(calendarPackedToEventTime(packedEventTime))
		lockDate := uint32(calendarPackedRawEpoch(packedLockDate))
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nextID, s.playerGUID, title, description, eventType, dungeonID, eventTime, flags, lockDate)

		// Guild announcements carry a single Empty-GUID NOT_SIGNED_UP invite
		// that CalendarMgr::AddInvite neither stores nor broadcasts
		// (CalendarMgr.cpp:147-160), so no invites are inserted for them.
		if flags&calendarFlagWithoutInvites == 0 {
			// The client-sent invite rows are stored as sent
			// (CalendarHandler.cpp:321-340): the client includes the creator
			// in the list itself, so C++ synthesizes no creator invite and
			// never rewrites the client-sent status/rank. Empty-GUID entries
			// are dropped: C++ would store them as invitee-0 rows, but no
			// client ever sends one and only forged packets produce them.
			type pendingInvite struct {
				guid, id          uint64
				status, moderator uint8
				level             uint8
				sess              *session
			}
			var nextInviteID uint64 = 1
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
			var pending []pendingInvite
			for _, inv := range rawInvites {
				if inv.guid == 0 {
					continue
				}
				var invLevel uint8
				inviteSess := s.server.findSessionByGUID(inv.guid)
				if inviteSess != nil && inviteSess.player != nil {
					invLevel = inviteSess.player.Level
				} else {
					_ = cdb.QueryRowContext(ctx,
						"SELECT level FROM characters WHERE guid = ?", inv.guid).Scan(&invLevel)
				}
				// CalendarMgr::GetFreeInviteId (CalendarMgr.cpp:355-363) hands
				// out ++_maxInviteId per invite, so the first invite takes
				// MAX(id)+1: increment after assigning, not before.
				pending = append(pending, pendingInvite{guid: inv.guid, id: nextInviteID, status: inv.status, moderator: inv.moderator, level: invLevel, sess: inviteSess})
				nextInviteID++
			}

			// CalendarHandler.cpp:328-344: when the client sent more than one
			// invite the INSERTs ride one CharacterDatabaseTransaction; a lone
			// invite executes immediately (AddInvite with a null trans).
			// The condition counts the client-sent entries, before the
			// empty-GUID drop.
			insertInvites := func(exCtx context.Context, ex calendarExecer) error {
				for _, p := range pending {
					if _, err := ex.ExecContext(exCtx,
						`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
						 VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
						p.id, nextID, p.guid, s.playerGUID, p.status, calendarDefaultResponseTime, p.moderator); err != nil {
						return err
					}
				}
				return nil
			}
			if len(rawInvites) > 1 {
				_ = s.server.CharactersStore.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
					return insertInvites(txCtx, tx)
				})
			} else {
				_ = insertInvites(ctx, cdb)
			}

			for _, p := range pending {
				// CalendarMgr::AddInvite during HandleCalendarAddEvent: the new
				// event is not in _events yet (AddEvent runs after the invite
				// loop), so SendCalendarEventInvite takes the pre-invite arm
				// (CalendarMgr.cpp:498-502) and the INVITE packet goes to the
				// sender (creator) only — never to the event relatives.
				invPkt := buildCalendarInvitePacket(p.guid, nextID, p.id, p.level, p.status, calendarDefaultResponseTime, s.playerGUID)
				_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invPkt, true)

				// SendCalendarEventInviteAlert (CalendarMgr.cpp:151-154):
				// non-guild events alert the invitee direct; guild events
				// alert only the creator-invitee, as a guild broadcast.
				alertPkt := buildCalendarInviteAlertPacket(nextID, title, int64(eventTime), flags, uint32(eventType), dungeonID, p.id, p.status, p.moderator, s.playerGUID, s.playerGUID)
				if flags&calendarFlagGuildEvent == 0 {
					if p.sess != nil {
						_ = p.sess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertPkt, true)
					}
				} else if p.guid == s.playerGUID {
					for _, t := range calendarGuildMemberSessions(s.server, s.player.GuildID) {
						_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertPkt, true)
					}
				}
			}
		}

		if flags&calendarFlagWithoutInvites != 0 && flags&calendarFlagGuildEvent == 0 {
			// CalendarMgr::AddInvite (CalendarMgr.cpp:151-154) skips the
			// announcement's Empty-GUID invite alert when the announcement
			// also carries the guild-event flag; without it the alert
			// broadcasts to the guild (SendCalendarEventInviteAlert,
			// CalendarMgr.cpp:581-598).
			alertPkt := buildCalendarInviteAlertPacket(nextID, title, int64(eventTime), flags, uint32(eventType), dungeonID, 0, CalendarStatusNotSignedUp, CalendarRankPlayer, s.playerGUID, s.playerGUID)
			for _, t := range calendarGuildMemberSessions(s.server, s.player.GuildID) {
				_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertPkt, true)
			}
		}

		// CalendarMgr::AddEvent (CalendarMgr.cpp:140-145) ends the add path
		// by sending the new event back to the creator
		// (CalendarHandler.cpp:348, CALENDAR_SENDTYPE_ADD).
		if pkt, ok := s.buildCalendarSendEvent(ctx, nextID, CalendarSendTypeAdd); ok {
			_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_EVENT), pkt, true)
		}
	}
	return true
}

// handleCalendarUpdateEvent processes CMSG_CALENDAR_UPDATE_EVENT (0x42E).
// Reference: WorldSession::HandleCalendarUpdateEvent (CalendarHandler.cpp:270-302)
// & WorldPackets::Calendar::CalendarUpdateEvent::Read (CalendarPackets.cpp:139-152):
// the past-time gate returns silently (no error packet), and an unknown
// event answers CALENDAR_ERROR_EVENT_INVALID.
func (s *session) handleCalendarUpdateEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
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

		// CalendarHandler.cpp:351-367: no creator/moderator gate — any
		// logged-in player can update any existing event.
		_, _ = cdb.ExecContext(ctx,
			`UPDATE calendar_events SET title = ?, description = ?, type = ?, dungeon = ?, eventtime = ?, flags = ?, time2 = ?
			 WHERE id = ?`,
			title, description, eventType, dungeonID, uint32(calendarPackedToEventTime(packedEventTime)), flags,
			uint32(calendarPackedRawEpoch(packedLockDate)), eventID)

		// SendCalendarEventUpdateAlert (CalendarMgr.cpp:515-533): u8(1), u64 event
		// id, packed old event time, u32 flags, packed event time, u8 type,
		// i32 dungeon, title, description, u8(0) repeatable, u32(100)
		// maxInvites, u32(0); to every event relative.
		updBuf := protocol.NewBuffer(64 + len(title) + len(description))
		updBuf.WriteU8(1) // unk
		updBuf.WriteU64(eventID)
		updBuf.WritePackedTime(time.Unix(int64(oldEventTime), 0))
		updBuf.WriteU32(flags)
		updBuf.WritePackedTime(time.Unix(int64(calendarPackedToEventTime(packedEventTime)), 0))
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
	return true
}

// handleCalendarRemoveEvent processes CMSG_CALENDAR_REMOVE_EVENT (0x42F).
// Reference: WorldSession::HandleCalendarRemoveEvent (CalendarHandler.cpp:304-308).
func (s *session) handleCalendarRemoveEvent(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
	}
	_, _ = r.ReadU64() // moderatorID
	_, _ = r.ReadU8()  // isSignUp

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB

		// WorldSession::HandleCalendarRemoveEvent (CalendarHandler.cpp:409)
		// passes the request straight to CalendarMgr::RemoveEvent
		// (CalendarMgr.cpp:217): no creator/moderator gate — a missing event
		// answers CALENDAR_ERROR_EVENT_INVALID and anything else is removed.
		var exists int64
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM calendar_events WHERE id = ?", eventID).Scan(&exists)
		if exists == 0 {
			return s.sendCalendarCommandResult(CalendarErrorEventInvalid)
		}
		s.calendarRemoveEventFully(ctx, cdb, eventID, s.playerGUID)
	}
	return true
}

// calendarRemoveEventFully mirrors CalendarMgr::RemoveEvent
// (CalendarMgr.cpp:217-262): the SMSG_CALENDAR_EVENT_REMOVED_ALERT goes to
// every event relative before anything is deleted, every invitee except the
// remover gets a calendar mail (MailDraft with MAIL_CHECK_MASK_COPIED;
// subject "removerGUID:title", body packed event time), then the invites and
// the event rows are deleted.
func (s *session) calendarRemoveEventFully(ctx context.Context, cdb *sql.DB, eventID uint64, removerGUID uint64) {
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

	// CalendarMgr::RemoveEvent (CalendarMgr.cpp:217-262): when an event is
	// deleted, every invitee except the remover gets a calendar mail
	// (MailDraft(subject, body) with MAIL_CHECK_MASK_COPIED). Subject is
	// removerGUID:title, body is the packed event time as a decimal string
	// (CalendarEvent::BuildCalendarMailSubject/BuildCalendarMailBody).
	// MailSender(CalendarEvent*) -> MAIL_CALENDAR (5), sender = event id,
	// MAIL_STATIONERY_DEFAULT; the 30-day expiry arm applies.
	// CalendarMgr::RemoveEvent (CalendarMgr.cpp:186-209) appends the
	// per-invitee calendar mail rows, the invite deletes and the event delete
	// to a single CharacterDatabaseTransaction committed once at the end.
	// The mail notifies stay outside the transaction: they are in-memory
	// only, and the tx body may replay on deadlock.
	var mailInvitees []uint64
	if mailRows, mailErr := cdb.QueryContext(ctx, "SELECT invitee FROM calendar_invites WHERE event = ?", eventID); mailErr == nil {
		for mailRows.Next() {
			var inviteeGUID uint64
			if err := mailRows.Scan(&inviteeGUID); err != nil || inviteeGUID == removerGUID {
				continue
			}
			mailInvitees = append(mailInvitees, inviteeGUID)
		}
		mailRows.Close()
	}
	now := time.Now().Unix()
	mailBody := strconv.FormatUint(uint64(protocol.PackTime(time.Unix(int64(evTime), 0))), 10)
	subject := strconv.FormatUint(removerGUID, 10) + ":" + evTitle
	expire := now + mailSendExpireDelay(false, 0)
	_ = s.server.CharactersStore.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		for _, inviteeGUID := range mailInvitees {
			nextMailID := s.server.generateMailID()
			if _, err := tx.ExecContext(txCtx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
				VALUES (?, 5, 41, 0, ?, ?, ?, ?, 0, ?, ?, 0, 0, 4)`,
				nextMailID, uint32(eventID), inviteeGUID, subject, mailBody, expire, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(txCtx, "DELETE FROM calendar_invites WHERE event = ?", eventID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(txCtx, "DELETE FROM calendar_events WHERE id = ?", eventID); err != nil {
			return err
		}
		return nil
	})
	for _, inviteeGUID := range mailInvitees {
		s.sendMailNotify(inviteeGUID)
	}
}

// removePlayerGuildEventsAndSignups mirrors
// CalendarMgr::RemovePlayerGuildEventsAndSignups (CalendarMgr.cpp:295), which
// Guild::HandleLeaveMember (Guild.cpp:1558) calls on both the leave and the
// disband paths: guild events and announcements created by the player go
// through the full RemoveEvent path, and the player's own invites to guild
// events of the given guild are dropped with an invite-remove broadcast
// (SendCalendarEventInviteRemove, CalendarMgr.cpp:559; the remove-ALERT arm
// only fires for non-guild events so it stays silent here).
func (s *session) removePlayerGuildEventsAndSignups(ctx context.Context, playerGUID uint64, guildID uint32) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	var createdIDs []uint64
	if rows, err := cdb.QueryContext(ctx, `SELECT id FROM calendar_events WHERE creator = ?
		AND (flags & ?) != 0`, playerGUID, calendarFlagGuildEvent|calendarFlagWithoutInvites); err == nil {
		for rows.Next() {
			var eventID uint64
			if err := rows.Scan(&eventID); err == nil {
				createdIDs = append(createdIDs, eventID)
			}
		}
		rows.Close()
	}
	for _, eventID := range createdIDs {
		s.calendarRemoveEventFully(ctx, cdb, eventID, playerGUID)
	}
	type guildInvite struct {
		inviteID uint64
		eventID  uint64
		invitee  uint64
		evFlags  uint32
	}
	var invites []guildInvite
	if rows, err := cdb.QueryContext(ctx, `SELECT i.id, i.event, i.invitee, e.flags FROM calendar_invites i
		JOIN calendar_events e ON e.id = i.event
		WHERE i.invitee = ? AND (e.flags & ?) != 0
		AND e.creator IN (SELECT guid FROM guild_member WHERE guildid = ?)`,
		playerGUID, calendarFlagGuildEvent, guildID); err == nil {
		for rows.Next() {
			var inv guildInvite
			if err := rows.Scan(&inv.inviteID, &inv.eventID, &inv.invitee, &inv.evFlags); err == nil {
				invites = append(invites, inv)
			}
		}
		rows.Close()
	}
	for _, inv := range invites {
		remBuf := protocol.NewBuffer(24)
		remBuf.WritePackedGUID(inv.invitee)
		remBuf.WriteU64(inv.eventID)
		remBuf.WriteU32(inv.evFlags)
		remBuf.WriteU8(1) // FIXME
		for _, t := range calendarEventRelativeSessions(ctx, s.server, inv.eventID) {
			_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_REMOVED), remBuf.Bytes(), true)
		}
		_, _ = cdb.ExecContext(ctx, "DELETE FROM calendar_invites WHERE id = ?", inv.inviteID)
	}
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
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
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
		var evTitle string
		var evType uint32
		var evDungeon int32
		if err := cdb.QueryRowContext(ctx,
			"SELECT creator, flags, title, type, dungeon FROM calendar_events WHERE id = ?", eventID).Scan(&creator, &flags, &evTitle, &evType, &evDungeon); err != nil {
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
		newEventTime := int64(calendarPackedToEventTime(packedEventTime))
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO calendar_events (id, creator, title, description, type, dungeon, eventtime, flags, time2)
			 SELECT ?, creator, title, description, type, dungeon, ?, flags, time2 FROM calendar_events WHERE id = ?`,
			nextID, uint32(newEventTime), eventID)

		// CalendarMgr::AddEvent on the copy path (CalendarHandler.cpp:489)
		// sends the copied event to its (original) creator with
		// CALENDAR_SENDTYPE_COPY BEFORE the invite loop (:491-499);
		// SendCalendarEvent skips players who are offline
		// (CalendarMgr.cpp:607-609).
		if pkt, ok := s.buildCalendarSendEvent(ctx, nextID, CalendarSendTypeCopy); ok {
			if target := s.server.findSessionByGUID(creator); target != nil {
				_ = target.write(uint16(protocol.OpcodeSMSG_CALENDAR_SEND_EVENT), pkt, true)
			}
		}

		rows, err := cdb.QueryContext(ctx, "SELECT invitee, sender, status, statustime, rank, text FROM calendar_invites WHERE event = ?", eventID)
		if err == nil {
			type copyInvite struct {
				invitee, sender, status, statustime, rank int64
				text                                      string
				id                                        uint64
			}
			var copies []copyInvite
			// CalendarMgr::GetFreeInviteId (CalendarMgr.cpp:355-363) hands
			// out ++_maxInviteId per invite, so the first copy takes
			// MAX(id)+1: increment after assigning, not before.
			var nextInviteID uint64 = 1
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
			for rows.Next() {
				var c copyInvite
				if rows.Scan(&c.invitee, &c.sender, &c.status, &c.statustime, &c.rank, &c.text) == nil {
					c.id = nextInviteID
					nextInviteID++
					copies = append(copies, c)
				}
			}
			rows.Close()

			// CalendarHandler.cpp:491-499: when the copied event carries more
			// than one invite the UpdateInvite calls ride a single
			// CharacterDatabaseTransaction; a lone invite executes
			// immediately (AddInvite with a null trans). The transaction is
			// driven manually (no deadlock replay): the broadcasts below
			// are network side effects that must not repeat.
			var tx *sql.Tx
			if len(copies) > 1 {
				if btx, berr := cdb.BeginTx(ctx, nil); berr == nil {
					tx = btx
					defer func() { _ = tx.Rollback() }()
				}
			}
			ex := calendarExecer(cdb)
			if tx != nil {
				ex = tx
			}

			var eventGuild uint32
			if flags&(calendarFlagGuildEvent|calendarFlagWithoutInvites) != 0 {
				eventGuild = calendarCreatorGuildID(ctx, cdb, creator)
			}

			for i, c := range copies {
				// CalendarMgr::AddInvite on the copy path
				// (CalendarHandler.cpp:491-499): the copied event is already
				// in _events (AddEvent ran first), so SendCalendarEventInvite
				// broadcasts to every event relative unless the invitee is
				// the creator (CalendarMgr.cpp:505-508). The broadcast runs
				// before the _invites push_back, so the relatives set must be
				// read before this invite's insert: invitee i only hears the
				// INVITE packets of invites 0..i-1. The tx's uncommitted rows
				// are invisible to the DB-backed relatives read, so fold the
				// connected sessions of the already-copied invitees in by
				// hand, with the same guild-event skip
				// SendPacketToAllEventRelatives applies
				// (CalendarMgr.cpp:700-712).
				copyTargets := calendarEventRelativeSessions(ctx, s.server, nextID)
				seen := make(map[*session]struct{}, len(copyTargets)+i)
				for _, t := range copyTargets {
					seen[t] = struct{}{}
				}
				for _, prev := range copies[:i] {
					prevSess := s.server.findSessionByGUID(uint64(prev.invitee))
					if prevSess == nil || prevSess.player == nil {
						continue
					}
					if _, ok := seen[prevSess]; ok {
						continue
					}
					if flags&calendarFlagGuildEvent != 0 && eventGuild != 0 && prevSess.player.GuildID == eventGuild {
						continue
					}
					seen[prevSess] = struct{}{}
					copyTargets = append(copyTargets, prevSess)
				}

				_, _ = ex.ExecContext(ctx,
					"INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
					c.id, nextID, c.invitee, c.sender, c.status, c.statustime, c.rank, c.text)

				var invLevel uint8
				invSess := s.server.findSessionByGUID(uint64(c.invitee))
				if invSess != nil && invSess.player != nil {
					invLevel = invSess.player.Level
				} else {
					_ = cdb.QueryRowContext(ctx,
						"SELECT level FROM characters WHERE guid = ?", c.invitee).Scan(&invLevel)
				}
				if uint64(c.invitee) != creator {
					invPkt := buildCalendarInvitePacket(uint64(c.invitee), nextID, c.id, invLevel, uint8(c.status), c.statustime, uint64(c.sender))
					for _, t := range copyTargets {
						_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invPkt, true)
					}
				}

				// SendCalendarEventInviteAlert (CalendarMgr.cpp:151-154):
				// non-guild events alert the invitee direct; guild events
				// and announcements alert only the creator-invitee, as a
				// guild broadcast.
				if flags&calendarFlagGuildEvent == 0 || uint64(c.invitee) == creator {
					alertPkt := buildCalendarInviteAlertPacket(nextID, evTitle, newEventTime, flags, evType, evDungeon, c.id, uint8(c.status), uint8(c.rank), creator, uint64(c.sender))
					if flags&(calendarFlagGuildEvent|calendarFlagWithoutInvites) != 0 {
						for _, t := range calendarGuildMemberSessions(s.server, calendarCreatorGuildID(ctx, cdb, creator)) {
							_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertPkt, true)
						}
					} else if invSess != nil {
						_ = invSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertPkt, true)
					}
				}
			}
			if tx != nil {
				_ = tx.Commit()
			}
		}

	}
	return true
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
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	eventID, err := r.ReadU64()
	if err != nil {
		return true
	}
	inviteID, _ := r.ReadU64()
	name, _ := r.ReadCString()
	isPreInvite, _ := r.ReadU8()
	isGuildEvent, _ := r.ReadU8()

	// normalizePlayerName (ObjectMgr.cpp:141-156) fails on empty or
	// non-UTF8 names, and the handler returns silently
	// (CalendarHandler.cpp:525-526); the normalized name is what the
	// lookup and the IGNORING_YOU_S echo use.
	if !utf8.ValidString(name) {
		return true
	}
	name = normalizePlayerName(name)
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
		invBuf.WriteU8(0)                                    // hasStatusTime
		invBuf.WriteU8(boolToU8(s.playerGUID != targetGUID)) // sender != invitee
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
			return true
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

		// CalendarMgr::AddInvite (CalendarMgr.cpp:147-160) broadcasts
		// SendCalendarEventInvite BEFORE storing the invite (the _invites
		// push_back and UpdateInvite come after), so the new invitee is
		// not yet in the relatives set and never receives the INVITE
		// packet — only the ALERT below. Read the relatives before the
		// insert.
		inviteTargets := calendarEventRelativeSessions(ctx, s.server, eventID)

		_, _ = cdb.ExecContext(ctx,
			"INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text) VALUES (?, ?, ?, ?, ?, ?, ?, '')",
			nextInviteID, eventID, targetGUID, s.playerGUID, CalendarStatusInvited, calendarDefaultResponseTime, CalendarRankPlayer)

		// CalendarMgr::AddInvite (CalendarMgr.cpp:147-160) via SendCalendarEventInvite
		// (CalendarMgr.cpp:481-514): the invite packet goes to every event
		// relative and is skipped entirely when the invitee is the event
		// creator.
		if evCreator != targetGUID {
			invBuf := protocol.NewBuffer(32)
			invBuf.WritePackedGUID(targetGUID)
			invBuf.WriteU64(eventID)
			invBuf.WriteU64(nextInviteID)
			invBuf.WriteU8(targetLevel)
			invBuf.WriteU8(CalendarStatusInvited)
			invBuf.WriteU8(0)                                    // hasStatusTime (new invites carry 946684800)
			invBuf.WriteU8(boolToU8(s.playerGUID != targetGUID)) // sender != invitee
			for _, t := range inviteTargets {
				_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invBuf.Bytes(), true)
			}
		}

		// SendCalendarEventInviteAlert (CalendarMgr.cpp:581-598): guild events
		// are broadcast to every online guild member (only the creator-invitee
		// case reaches this arm); otherwise the alert goes to the connected
		// invitee direct.
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
		if evFlags&calendarFlagGuildEvent != 0 {
			if targetGUID == evCreator {
				for _, t := range calendarGuildMemberSessions(s.server, calendarCreatorGuildID(ctx, cdb, evCreator)) {
					_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
				}
			}
		} else if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil {
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
		}
	}
	return true
}

// handleCalendarEventRSVP processes CMSG_CALENDAR_EVENT_RSVP (0x432).
// Reference: WorldSession::HandleCalendarEventRsvp (CalendarHandler.cpp:608-628):
// RSVPing any status other than REMOVED on a locked event answers
// CALENDAR_ERROR_EVENT_LOCKED; an unknown event answers
// CALENDAR_ERROR_EVENT_INVALID; an unknown invite answers
// CALENDAR_ERROR_NO_INVITE.
func (s *session) handleCalendarEventRSVP(ctx context.Context, payload []byte) bool {
	if len(payload) < 20 {
		return true
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
		var invitee uint64
		if err := cdb.QueryRowContext(ctx,
			"SELECT rank, invitee FROM calendar_invites WHERE id = ?", inviteID).Scan(&rank, &invitee); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}

		now := time.Now().Unix()
		_, _ = cdb.ExecContext(ctx,
			"UPDATE calendar_invites SET status = ?, statustime = ? WHERE id = ?",
			status, now, inviteID)

		// SendCalendarEventStatus (CalendarMgr.cpp:535-547) carries the updated
		// status, the invite's rank, and the just-set status time — addressed
		// to the invite's invitee, not necessarily the responder.
		sendCalendarEventStatusToRelatives(s.server, calendarEventRelativeSessions(ctx, s.server, eventID),
			invitee, eventID, int64(evTime), evFlags, uint8(status), rank, now)

		// CalendarHandler.cpp:663 — the RSVP clears the pending action on
		// the responder's own client.
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_CLEAR_PENDING_ACTION), nil, true)
	}
	return true
}

// handleCalendarEventRemoveInvite processes CMSG_CALENDAR_EVENT_REMOVE_INVITE (0x433).
// Reference: WorldSession::HandleCalendarEventRemoveInvite (CalendarHandler.cpp:640-665):
// removing the event creator answers CALENDAR_ERROR_DELETE_CREATOR_FAILED;
// an unknown event answers CALENDAR_ERROR_NO_INVITE (not EVENT_INVALID);
// CalendarMgr::RemoveInvite (CalendarMgr.cpp:216) only ever deletes the
// invite matching both the invite id and the event id.
func (s *session) handleCalendarEventRemoveInvite(ctx context.Context, payload []byte) bool {
	if len(payload) < 24 {
		return true
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID()
	inviteID, _ := r.ReadU64()
	_, _ = r.ReadU64() // moderatorID
	eventID, _ := r.ReadU64()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var creator uint64
		var evFlags uint32
		var evTime uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT creator, flags, eventtime FROM calendar_events WHERE id = ?", eventID).Scan(&creator, &evFlags, &evTime); err != nil {
			return s.sendCalendarCommandResult(CalendarErrorNoInvite)
		}
		if creator == inviteeGUID {
			return s.sendCalendarCommandResult(CalendarErrorDeleteCreatorFailed)
		}

		// CalendarMgr::RemoveInvite (CalendarMgr.cpp:225-248) broadcasts
		// SendCalendarEventInviteRemove BEFORE erasing the invite from the
		// in-memory store, so the removed invitee still receives it when
		// online. Read the relatives before the delete. When the invite
		// isn't found it returns silently — no broadcast.
		removeTargets := calendarEventRelativeSessions(ctx, s.server, eventID)
		// CalendarMgr::RemoveInvite (CalendarMgr.cpp:238-240) alerts and
		// broadcasts the invite ROW's invitee GUID, not the packet's; read
		// it before the delete. The rows==0 silent return below covers a
		// missing row.
		var rowInvitee uint64
		_ = cdb.QueryRowContext(ctx,
			"SELECT invitee FROM calendar_invites WHERE id = ? AND event = ?", inviteID, eventID).Scan(&rowInvitee)
		res, _ := cdb.ExecContext(ctx,
			"DELETE FROM calendar_invites WHERE id = ? AND event = ?", inviteID, eventID)
		if rows, _ := res.RowsAffected(); rows == 0 {
			return true
		}

		// CalendarMgr::RemoveInvite (CalendarMgr.cpp:238) sends
		// SendCalendarEventInviteRemoveAlert (CalendarMgr.cpp:652-660) direct
		// to the removed invitee for non-guild events: u64 event id, packed
		// event time, u32 flags, u8 status.
		if evFlags&calendarFlagGuildEvent == 0 {
			if invSess := s.server.findSessionByGUID(rowInvitee); invSess != nil {
				alertBuf := protocol.NewBuffer(16)
				alertBuf.WriteU64(eventID)
				alertBuf.WritePackedTime(time.Unix(int64(evTime), 0))
				alertBuf.WriteU32(evFlags)
				alertBuf.WriteU8(CalendarStatusRemoved)
				_ = invSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_REMOVED_ALERT), alertBuf.Bytes(), true)
			}
		}

		// SendCalendarEventInviteRemove (CalendarMgr.cpp:559-568): the event's
		// flags (not 0), trailing u8(1), broadcast to every event relative.
		remBuf := protocol.NewBuffer(24)
		remBuf.WritePackedGUID(rowInvitee)
		remBuf.WriteU64(eventID)
		remBuf.WriteU32(evFlags)
		remBuf.WriteU8(1) // FIXME (CalendarMgr.cpp:565)
		for _, t := range removeTargets {
			_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_REMOVED), remBuf.Bytes(), true)
		}
	}
	return true
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
		return true
	}
	r := protocol.NewReader(payload)
	inviteeGUID, _ := r.ReadPackedGUID() // invitee: C++ :724 clears pending on the packet-parsed GUID
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

		// CalendarHandler.cpp:724 — the moderator-driven status change clears
		// the pending action on the packet-parsed invitee's client.
		if invSess := s.server.findSessionByGUID(inviteeGUID); invSess != nil {
			_ = invSess.write(uint16(protocol.OpcodeSMSG_CALENDAR_CLEAR_PENDING_ACTION), nil, true)
		}
	}
	return true
}

// handleCalendarEventModeratorStatus processes CMSG_CALENDAR_EVENT_MODERATOR_STATUS (0x435).
// Reference: WorldSession::HandleCalendarEventModeratorStatus (CalendarHandler.cpp:730):
// an unknown event answers CALENDAR_ERROR_EVENT_INVALID, an unknown invite
// answers CALENDAR_ERROR_NO_INVITE; the invite's rank is updated by invite id
// only; SMSG_CALENDAR_EVENT_MODERATOR_STATUS_ALERT goes to every event relative.
func (s *session) handleCalendarEventModeratorStatus(ctx context.Context, payload []byte) bool {
	if len(payload) < 25 {
		return true
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
	return true
}

// handleCalendarEventSignup processes CMSG_CALENDAR_EVENT_SIGNUP (0x4BA).
// Reference: WorldSession::HandleCalendarEventSignup (CalendarHandler.cpp:604):
// signing up for a guild event of another guild answers
// CALENDAR_ERROR_GUILD_PLAYER_NOT_IN_GUILD; an unknown event answers
// CALENDAR_ERROR_EVENT_INVALID. The signup flows through
// CalendarMgr::AddInvite (CalendarMgr.cpp:147-160): SMSG_CALENDAR_EVENT_INVITE
// to every event relative (suppressed when the signer is the creator;
// SendCalendarEventInvite, CalendarMgr.cpp:481-514), the
// SMSG_CALENDAR_EVENT_INVITE_ALERT arm (direct to self for non-guild events,
// guild broadcast only when the signer is the creator for guild events;
// CalendarMgr.cpp:581-598), and SendCalendarClearPendingAction
// (CalendarMgr.cpp:666-673). C++ sends no SMSG_CALENDAR_EVENT_STATUS here.
func (s *session) handleCalendarEventSignup(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return true
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
		var evTitle string
		var evType uint32
		var evDungeon int32
		if err := cdb.QueryRowContext(ctx,
			"SELECT flags, creator, eventtime, title, type, dungeon FROM calendar_events WHERE id = ?", eventID).Scan(&evFlags, &evCreator, &evTime, &evTitle, &evType, &evDungeon); err != nil {
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
		isGuildEvent := evFlags&calendarFlagGuildEvent != 0
		// IsGuildAnnouncement() is _flags & CALENDAR_FLAG_WITHOUT_INVITES
		// (CalendarMgr.h:257).
		isAnnouncement := evFlags&calendarFlagWithoutInvites != 0
		var nextInviteID uint64 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM calendar_invites").Scan(&nextInviteID)
		statTime := time.Now().Unix()

		// AddInvite: guild announcements send no invite packet; the packet
		// goes to every event relative and is skipped entirely when the
		// signer is the event creator (the relatives set is read before the
		// insert, matching AddInvite's broadcast-before-store order). The
		// signup's sender == invitee, so the trailing byte is 0 and the
		// non-default status time is always present.
		if !isAnnouncement && evCreator != s.playerGUID {
			invBuf := protocol.NewBuffer(48)
			invBuf.WritePackedGUID(s.playerGUID)
			invBuf.WriteU64(eventID)
			invBuf.WriteU64(nextInviteID)
			invBuf.WriteU8(uint8(s.player.Level))
			invBuf.WriteU8(uint8(status))
			invBuf.WriteU8(1) // hasStatusTime (signup stamps now, never 946684800)
			invBuf.WritePackedTime(time.Unix(statTime, 0))
			invBuf.WriteU8(0) // sender != invitee (false for signups)
			for _, t := range calendarEventRelativeSessions(ctx, s.server, eventID) {
				_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE), invBuf.Bytes(), true)
			}
		}

		// AddInvite never stores invites for guild announcements.
		if !isAnnouncement {
			_, _ = cdb.ExecContext(ctx,
				`INSERT INTO calendar_invites (id, event, invitee, sender, status, statustime, rank, text)
				 VALUES (?, ?, ?, ?, ?, ?, 0, '')`,
				nextInviteID, eventID, s.playerGUID, s.playerGUID, status, statTime)
		}

		// AddInvite alert arm: fired for non-guild events, and for guild
		// events only when the signer is the creator. Guild events and
		// announcements broadcast to the whole guild; otherwise the
		// connected invitee gets it direct (the signer is online).
		if !isGuildEvent || s.playerGUID == evCreator {
			alertBuf := protocol.NewBuffer(64 + len(evTitle))
			alertBuf.WriteU64(eventID)
			alertBuf.WriteCString(evTitle)
			alertBuf.WritePackedTime(time.Unix(int64(evTime), 0))
			alertBuf.WriteU32(evFlags)
			alertBuf.WriteU32(evType)
			alertBuf.WriteI32(evDungeon)
			alertBuf.WriteU64(nextInviteID)
			alertBuf.WriteU8(uint8(status))
			alertBuf.WriteU8(CalendarRankPlayer)
			alertBuf.WritePackedGUID(evCreator)
			alertBuf.WritePackedGUID(s.playerGUID)
			if isGuildEvent || isAnnouncement {
				for _, t := range calendarGuildMemberSessions(s.server, calendarCreatorGuildID(ctx, cdb, evCreator)) {
					_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
				}
			} else {
				_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_INVITE_ALERT), alertBuf.Bytes(), true)
			}
		}

		// CalendarMgr::SendCalendarClearPendingAction.
		_ = s.write(uint16(protocol.OpcodeSMSG_CALENDAR_CLEAR_PENDING_ACTION), nil, true)
	}
	return true
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
