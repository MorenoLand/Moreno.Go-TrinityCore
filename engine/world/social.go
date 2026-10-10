package world

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// FriendsResult mirrors TrinityCore's FriendsResult enum.
// From SocialMgr.h.
const (
	friendsResultDBError        uint8 = 0x00
	friendsResultListFull       uint8 = 0x01
	friendsResultOnline         uint8 = 0x02
	friendsResultOffline        uint8 = 0x03
	friendsResultNotFound       uint8 = 0x04
	friendsResultRemoved        uint8 = 0x05
	friendsResultAddedOnline    uint8 = 0x06
	friendsResultAddedOffline   uint8 = 0x07
	friendsResultAlready        uint8 = 0x08
	friendsResultSelf           uint8 = 0x09
	friendsResultEnemy          uint8 = 0x0A
	friendsResultIgnoreFull     uint8 = 0x0B
	friendsResultIgnoreSelf     uint8 = 0x0C
	friendsResultIgnoreNotFound uint8 = 0x0D
	friendsResultIgnoreAlready  uint8 = 0x0E
	friendsResultIgnoreAdded    uint8 = 0x0F
	friendsResultIgnoreRemoved  uint8 = 0x10
)

// FriendStatus mirrors TrinityCore's FriendStatus enum.
const (
	friendStatusOffline uint8 = 0
	friendStatusOnline  uint8 = 1
	friendStatusAFK     uint8 = 2
	friendStatusDND     uint8 = 4
	friendStatusRAF     uint8 = 8
)

// Social contact flags from SocialMgr.h.
const (
	socialFlagFriend  uint8 = 0x01
	socialFlagIgnored uint8 = 0x02
	socialFlagMuted   uint8 = 0x04
)

// Max social list sizes from SocialMgr.h.
const (
	socialFriendLimit uint32 = 50
	socialIgnoreLimit uint32 = 50
)

// -----------------------------------------------------------------
// SMSG_CONTACT_LIST (0x067) builder
// TrinityCore: PlayerSocial::SendSocialList
// flags: 0x1=friends, 0x2=ignored, 0x4=muted
// -----------------------------------------------------------------
// socialHasFriend reports whether ownerGUID lists friendGUID with the friend
// flag — the PlayerSocial::HasFriend arm used by the group-invite level gate.
func (s *Server) socialHasFriend(ownerGUID, friendGUID uint64) bool {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return false
	}
	var flags int64
	if err := s.CharactersStore.DB.QueryRowContext(context.Background(), "SELECT flags FROM character_social WHERE guid = ? AND friend = ? LIMIT 1", ownerGUID, friendGUID).Scan(&flags); err != nil {
		return false
	}
	return uint64(flags)&uint64(socialFlagFriend) != 0
}

func (s *Server) friendStatus(viewer *session, guid uint64) (uint8, uint32, uint32, uint32) {
	if s == nil || viewer == nil || viewer.player == nil {
		return friendStatusOffline, 0, 0, 0
	}
	friendSess := s.findSessionByGUID(guid)
	if friendSess == nil || !friendSess.worldReady.Load() || friendSess.player == nil {
		return friendStatusOffline, 0, 0, 0
	}
	if !s.canFriendSee(viewer, friendSess) {
		return friendStatusOffline, 0, 0, 0
	}
	status := friendStatusOnline
	if friendSess.player.PlayerFlags&playerFlagDND != 0 {
		status = friendStatusDND
	} else if friendSess.player.PlayerFlags&playerFlagAFK != 0 {
		status = friendStatusAFK
	} else if (friendSess.recruiterID != 0 && friendSess.recruiterID == viewer.accountID) ||
		(viewer.recruiterID != 0 && viewer.recruiterID == friendSess.accountID) {
		// SocialMgr::GetFriendInfo (SocialMgr.cpp:234): the friend is marked
		// recruit-a-friend linked when the accounts are recruiter-linked in
		// either direction. Applies to the plain-online branch only, like C++.
		status |= friendStatusRAF
	}
	return status, uint32(friendSess.player.Zone), uint32(friendSess.player.Level), uint32(friendSess.player.Class)
}

func (s *Server) canFriendSee(viewer, target *session) bool {
	if s == nil || viewer == nil || target == nil || viewer.player == nil || target.player == nil {
		return false
	}
	if viewer.player.GUID == target.player.GUID {
		return true
	}
	if !viewer.whoSeeAllSecurityLevels && int(target.security) > s.Config.GMInWhoListLevel {
		return false
	}
	if playerTeam(viewer.player.Race) != playerTeam(target.player.Race) && !viewer.twoSideWhoList {
		return false
	}
	return target.player.ExtraFlags&playerExtraGMInvisible == 0 || (viewer.security != 0 && target.security <= viewer.security)
}

// isVisibleGloballyFor mirrors Player::IsVisibleGloballyFor (Player.cpp:22599):
// self is always visible, a GM-visible target is visible to everyone, and an
// invisible target is visible only to a GM viewer of equal-or-higher security.
func isVisibleGloballyFor(viewer, target *session) bool {
	if viewer == nil || target == nil || viewer.player == nil || target.player == nil {
		return false
	}
	if viewer.player.GUID == target.player.GUID {
		return true
	}
	if target.player.ExtraFlags&playerExtraGMInvisible == 0 {
		return true
	}
	if viewer.security != 0 {
		return target.security <= viewer.security
	}
	return false
}

// truncateSocialNote mirrors utf8truncate(note, 48) in
// PlayerSocial::SetFriendNote (SocialMgr.cpp:131): the note is limited to 48
// Unicode code points, not bytes.
func truncateSocialNote(note string) string {
	if runes := []rune(note); len(runes) > 48 {
		return string(runes[:48])
	}
	return note
}

// upsertSocialContact mirrors PlayerSocial::AddToSocialList (SocialMgr.cpp:50):
// the flag bit is ORed into any existing row (never clobbering other flags)
// and the row is created otherwise. REPLACE INTO is valid on both SQLite and
// MySQL, unlike the SQLite-only ON CONFLICT / INSERT OR REPLACE pair this
// replaces.
func upsertSocialContact(ctx context.Context, cdb *sql.DB, guid, friendGUID uint64, flagBit uint8, note string, keepNote bool) error {
	var oldFlags uint8
	var oldNote string
	_ = cdb.QueryRowContext(ctx, "SELECT flags, note FROM character_social WHERE guid = ? AND friend = ?", guid, friendGUID).Scan(&oldFlags, &oldNote)
	if keepNote {
		note = oldNote
	}
	_, err := cdb.ExecContext(ctx, "REPLACE INTO character_social (guid, friend, flags, note) VALUES (?, ?, ?, ?)", guid, friendGUID, oldFlags|flagBit, note)
	return err
}

func (s *session) sendContactList(ctx context.Context, flags uint32) error {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		b := protocol.NewBuffer(8)
		b.WriteU32(flags)
		b.WriteU32(0)
		return s.write(uint16(protocol.OpcodeSMSG_CONTACT_LIST), b.Bytes(), true)
	}
	rows, err := cdb.QueryContext(ctx,
		"SELECT cs.friend, cs.flags, cs.note FROM character_social cs JOIN characters c ON c.guid = cs.friend WHERE cs.guid = ? AND (c.deleteInfos_Name IS NULL OR c.deleteInfos_Name = '') ORDER BY cs.friend LIMIT 255",
		s.playerGUID)
	if err != nil && isMissingColumn(err) {
		rows, err = cdb.QueryContext(ctx, "SELECT friend, flags, note FROM character_social WHERE guid = ? ORDER BY friend LIMIT 255", s.playerGUID)
	}
	if err != nil {
		if missingTable(err) || errors.Is(err, sql.ErrNoRows) {
			b := protocol.NewBuffer(8)
			b.WriteU32(flags)
			b.WriteU32(0)
			return s.write(uint16(protocol.OpcodeSMSG_CONTACT_LIST), b.Bytes(), true)
		}
		return err
	}
	defer rows.Close()

	type contactEntry struct {
		GUID  uint64
		Flags uint8
		Note  string
	}
	var contacts []contactEntry
	friendCount, ignoreCount := uint32(0), uint32(0)
	for rows.Next() {
		var friendGUID uint64
		var f uint8
		var note string
		if err := rows.Scan(&friendGUID, &f, &note); err != nil {
			continue
		}
		if (uint32(f) & flags) == 0 {
			continue
		}
		if f&socialFlagFriend != 0 {
			if friendCount >= socialFriendLimit {
				continue
			}
			friendCount++
		}
		if f&socialFlagIgnored != 0 {
			if ignoreCount >= socialIgnoreLimit {
				continue
			}
			ignoreCount++
		}
		contacts = append(contacts, contactEntry{GUID: friendGUID, Flags: f, Note: note})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	b := protocol.NewBuffer(8 + len(contacts)*30)
	b.WriteU32(flags)
	b.WriteU32(uint32(len(contacts)))
	for _, c := range contacts {
		b.WriteU64(c.GUID)
		b.WriteU32(uint32(c.Flags))
		b.WriteCString(c.Note)
		if c.Flags&socialFlagFriend != 0 {
			status, zone, level, class := s.server.friendStatus(s, c.GUID)
			b.WriteU8(status)
			if status != friendStatusOffline {
				b.WriteU32(zone)
				b.WriteU32(level)
				b.WriteU32(class)
			}
		}
	}
	return s.write(uint16(protocol.OpcodeSMSG_CONTACT_LIST), b.Bytes(), true)
}

// sendFriendStatus sends SMSG_FRIEND_STATUS (0x068) to this session.
// TrinityCore: SocialMgr::SendFriendStatus.
func (s *session) sendFriendStatus(result uint8, friendGUID uint64, note string) error {
	b := protocol.NewBuffer(14)
	b.WriteU8(result)
	b.WriteU64(friendGUID)

	switch result {
	case friendsResultAddedOffline, friendsResultAddedOnline:
		b.WriteCString(note)
	}

	switch result {
	case friendsResultAddedOnline, friendsResultOnline:
		status, zone, level, class := s.server.friendStatus(s, friendGUID)
		b.WriteU8(status)
		b.WriteU32(zone)
		b.WriteU32(level)
		b.WriteU32(class)
	}

	return s.write(uint16(protocol.OpcodeSMSG_FRIEND_STATUS), b.Bytes(), true)
}

// broadcastFriendStatus sends SMSG_FRIEND_STATUS to all online players who have playerGUID on their friends list.
// Reference: SocialMgr::BroadcastToFriendListers (SocialMgr.cpp:287).
func (s *Server) broadcastFriendStatus(playerGUID uint64, result uint8, zone, level, class uint32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rows, err := s.CharactersStore.DB.QueryContext(ctx,
		"SELECT guid FROM character_social WHERE friend = ? AND (flags & 1) != 0", playerGUID)
	if err != nil {
		return
	}
	defer rows.Close()
	target := s.findSessionByGUID(playerGUID)
	if target == nil || target.player == nil {
		return
	}

	var recipientGUIDs []uint64
	for rows.Next() {
		var g uint64
		if rows.Scan(&g) == nil {
			recipientGUIDs = append(recipientGUIDs, g)
		}
	}

	if len(recipientGUIDs) == 0 {
		return
	}

	// SocialMgr::SendFriendStatus (SocialMgr.cpp:245): the packet is built ONCE
	// via GetFriendInfo(subject, subjectGUID, fi) and the identical payload goes
	// to every friend lister; per-recipient status recomputation would inject a
	// FRIEND_STATUS_RAF bit that C++ only ever sets for direct (non-broadcast)
	// packets. The subject is always visible to itself (ObjectAccessor always
	// resolves it), so the shared status is the subject's real DND/AFK/online
	// state without RAF, independent of the worldReady gate used for viewers.
	status := friendStatusOnline
	if target.player.PlayerFlags&playerFlagDND != 0 {
		status = friendStatusDND
	} else if target.player.PlayerFlags&playerFlagAFK != 0 {
		status = friendStatusAFK
	}
	b := protocol.NewBuffer(22)
	b.WriteU8(result)
	b.WriteU64(playerGUID)
	if result == friendsResultOnline {
		b.WriteU8(status)
		b.WriteU32(zone)
		b.WriteU32(level)
		b.WriteU32(class)
	}
	payload := b.Bytes()

	for _, recipient := range recipientGUIDs {
		sess := s.findSessionByGUID(recipient)
		if sess == nil || !sess.worldReady.Load() || !s.canFriendSee(sess, target) {
			continue
		}
		_ = sess.write(uint16(protocol.OpcodeSMSG_FRIEND_STATUS), payload, true)
	}
}

// -----------------------------------------------------------------
// handleContactList processes CMSG_CONTACT_LIST (0x066).
// TrinityCore: WorldSession::HandleContactListOpcode.
// -----------------------------------------------------------------
func (s *session) handleContactList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	r := protocol.NewReader(payload)
	flags, _ := r.ReadU32()
	// TrinityCore: HandleContactListOpcode passes flags through verbatim to
	// PlayerSocial::SendSocialList (SocialMgr.cpp:124) — no zero default.
	return s.sendContactList(ctx, flags) == nil
}

// -----------------------------------------------------------------
// handleAddFriend processes CMSG_ADD_FRIEND (0x069).
// TrinityCore: WorldSession::HandleAddFriendOpcode.
// -----------------------------------------------------------------
func (s *session) handleAddFriend(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	friendName, err := r.ReadCString()
	if err != nil || friendName == "" {
		return false
	}
	friendNote, _ := r.ReadCString()
	// C++ normalizePlayerName (ObjectMgr.cpp:141-158) fails on empty or
	// invalid-UTF-8 names, and the handler returns silently with no packet
	// (SocialHandler.cpp:51). The empty case is covered above; invalid
	// UTF-8 must not fall through to a FRIEND_NOT_FOUND answer.
	if !utf8.ValidString(friendName) {
		return true
	}
	// TrinityCore: HandleAddFriendOpcode normalizes the name before the lookup.
	friendName = normalizePlayerName(friendName)

	// Can't friend yourself. TrinityCore answers FRIEND_SELF with the
	// requester's own GUID (SocialHandler.cpp:66-67 -> SendFriendStatus
	// carries friendCharacterInfo->Guid, which is the player's own GUID).
	if toLower(friendName) == toLower(s.player.Name) {
		_ = s.sendFriendStatus(friendsResultSelf, s.playerGUID, "")
		return true
	}

	// Look up GUID by name from DB
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		_ = s.sendFriendStatus(friendsResultNotFound, 0, "")
		return true
	}
	var friendGUID uint64
	var friendRace uint8
	err = cdb.QueryRowContext(ctx, "SELECT guid, race FROM characters WHERE name = ? LIMIT 1", friendName).Scan(&friendGUID, &friendRace)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
			_ = s.sendFriendStatus(friendsResultNotFound, 0, "")
			return true
		}
		return false
	}
	if friendGUID == s.playerGUID {
		_ = s.sendFriendStatus(friendsResultSelf, s.playerGUID, "")
		return true
	}

	// TrinityCore: without RBAC_PERM_ALLOW_GM_FRIEND the target must be a
	// plain player account, checked on the online session or, when offline,
	// via the account's realm security (AccountMgr::GetSecurityAsync arm).
	if !s.allowGMFriend {
		var targetSecurity uint8
		if friendSess := s.server.findSessionByGUID(friendGUID); friendSess != nil {
			targetSecurity = friendSess.security
		} else if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
			var accountID uint32
			if cdb.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", friendGUID).Scan(&accountID) == nil {
				var sec int64
				if s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(SecurityLevel), 0) FROM account_access WHERE AccountID = ? AND RealmID IN (-1, ?)", accountID, s.server.RealmID).Scan(&sec) == nil && sec > 0 && sec <= 255 {
					targetSecurity = uint8(sec)
				}
			}
		}
		if targetSecurity != 0 {
			_ = s.sendFriendStatus(friendsResultNotFound, 0, "")
			return true
		}
	}

	// TrinityCore: cross-faction target without RBAC_PERM_TWO_SIDE_ADD_FRIEND
	// is FRIEND_ENEMY (SocialHandler.cpp:68).
	if playerTeam(friendRace) != playerTeam(s.player.Race) && !s.twoSideAddFriend {
		_ = s.sendFriendStatus(friendsResultEnemy, friendGUID, "")
		return true
	}

	// Check if already friend
	var count int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_social WHERE guid = ? AND friend = ? AND flags & 1 != 0", s.playerGUID, friendGUID).Scan(&count)
	if count > 0 {
		_ = s.sendFriendStatus(friendsResultAlready, friendGUID, "")
		return true
	}

	// Check friend list limit
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_social WHERE guid = ? AND flags & 1 != 0", s.playerGUID).Scan(&count)
	if uint32(count) >= socialFriendLimit {
		_ = s.sendFriendStatus(friendsResultListFull, friendGUID, "")
		return true
	}

	// Online or offline? TrinityCore reports FRIEND_ADDED_ONLINE only when
	// the target is online AND globally visible to the requester
	// (Player::IsVisibleGloballyFor).
	result := friendsResultAddedOffline
	if friendSess := s.server.findSessionByGUID(friendGUID); friendSess != nil && isVisibleGloballyFor(s, friendSess) {
		result = friendsResultAddedOnline
	}

	// TrinityCore: AddToSocialList ORs the flag into any existing row, then
	// SetFriendNote stores the (48-code-point truncated) note.
	friendNote = truncateSocialNote(friendNote)
	if err := upsertSocialContact(ctx, cdb, s.playerGUID, friendGUID, socialFlagFriend, friendNote, false); err != nil {
		return false
	}

	_ = s.sendFriendStatus(result, friendGUID, friendNote)
	return true
}

// -----------------------------------------------------------------
// handleDelFriend processes CMSG_DEL_FRIEND (0x06A).
// TrinityCore: WorldSession::HandleDelFriendOpcode.
// -----------------------------------------------------------------
func (s *session) handleDelFriend(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	r := protocol.NewReader(payload)
	friendGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}
	// Clear friend flag; delete if no other flags remain
	_, _ = cdb.ExecContext(ctx,
		"UPDATE character_social SET flags = flags & ~1 WHERE guid = ? AND friend = ?",
		s.playerGUID, friendGUID)
	_, _ = cdb.ExecContext(ctx,
		"DELETE FROM character_social WHERE guid = ? AND friend = ? AND flags = 0",
		s.playerGUID, friendGUID)
	_ = s.sendFriendStatus(friendsResultRemoved, friendGUID, "")
	return true
}

// -----------------------------------------------------------------
// handleAddIgnore processes CMSG_ADD_IGNORE (0x06C).
// TrinityCore: WorldSession::HandleAddIgnoreOpcode.
// -----------------------------------------------------------------
func (s *session) handleAddIgnore(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	ignoreName, err := r.ReadCString()
	if err != nil || ignoreName == "" {
		return false
	}
	// Same normalize-failure silent return as the add-friend arm
	// (SocialHandler.cpp:133).
	if !utf8.ValidString(ignoreName) {
		return true
	}
	// TrinityCore: HandleAddIgnoreOpcode normalizes the name before the lookup.
	ignoreName = normalizePlayerName(ignoreName)

	if toLower(ignoreName) == toLower(s.player.Name) {
		_ = s.sendFriendStatus(friendsResultIgnoreSelf, s.playerGUID, "")
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		_ = s.sendFriendStatus(friendsResultIgnoreNotFound, 0, "")
		return true
	}
	var ignoreGUID uint64
	err = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", ignoreName).Scan(&ignoreGUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
			_ = s.sendFriendStatus(friendsResultIgnoreNotFound, 0, "")
			return true
		}
		return false
	}
	if ignoreGUID == s.playerGUID {
		_ = s.sendFriendStatus(friendsResultIgnoreSelf, s.playerGUID, "")
		return true
	}

	var count int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_social WHERE guid = ? AND friend = ? AND flags & 2 != 0", s.playerGUID, ignoreGUID).Scan(&count)
	if count > 0 {
		_ = s.sendFriendStatus(friendsResultIgnoreAlready, ignoreGUID, "")
		return true
	}

	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_social WHERE guid = ? AND flags & 2 != 0", s.playerGUID).Scan(&count)
	if uint32(count) >= socialIgnoreLimit {
		_ = s.sendFriendStatus(friendsResultIgnoreFull, ignoreGUID, "")
		return true
	}

	// TrinityCore: AddToSocialList ORs SOCIAL_FLAG_IGNORED into any existing
	// row without touching the note.
	if err := upsertSocialContact(ctx, cdb, s.playerGUID, ignoreGUID, socialFlagIgnored, "", true); err != nil {
		return false
	}
	_ = s.sendFriendStatus(friendsResultIgnoreAdded, ignoreGUID, "")
	return true
}

// -----------------------------------------------------------------
// handleDelIgnore processes CMSG_DEL_IGNORE (0x06D).
// TrinityCore: WorldSession::HandleDelIgnoreOpcode.
// -----------------------------------------------------------------
func (s *session) handleDelIgnore(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	r := protocol.NewReader(payload)
	ignoreGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}
	_, _ = cdb.ExecContext(ctx,
		"UPDATE character_social SET flags = flags & ~2 WHERE guid = ? AND friend = ?",
		s.playerGUID, ignoreGUID)
	_, _ = cdb.ExecContext(ctx,
		"DELETE FROM character_social WHERE guid = ? AND friend = ? AND flags = 0",
		s.playerGUID, ignoreGUID)
	_ = s.sendFriendStatus(friendsResultIgnoreRemoved, ignoreGUID, "")
	return true
}

// -----------------------------------------------------------------
// handleSetContactNotes processes CMSG_SET_CONTACT_NOTES (0x1C6 in 3.3.5).
// TrinityCore: WorldSession::HandleSetContactNotesOpcode.
// -----------------------------------------------------------------
func (s *session) handleSetContactNotes(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded {
		return false
	}
	r := protocol.NewReader(payload)
	friendGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	note, _ := r.ReadCString()
	// TrinityCore: SetFriendNote truncates to 48 code points (utf8truncate).
	note = truncateSocialNote(note)
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}
	_, _ = cdb.ExecContext(ctx,
		"UPDATE character_social SET note = ? WHERE guid = ? AND friend = ?",
		note, s.playerGUID, friendGUID)
	return true
}
