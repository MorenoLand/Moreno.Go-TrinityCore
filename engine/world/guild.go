package world

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type guildMemberInfo struct {
	GUID        uint64
	Name        string
	RankID      int32
	Level       uint8
	ClassID     uint8
	Gender      uint8
	AreaID      int32
	LastSave    float32
	Note        string
	OfficerNote string
	Status      uint8
}

const (
	guildEventJoined          uint8  = 3
	guildEventLeft            uint8  = 4
	guildEventSignedOn        uint8  = 12
	guildEventSignedOff       uint8  = 13
	guildRightViewOfficerNote uint32 = 0x00004000
)

func guildEventPayload(eventType uint8, guid uint64, params ...string) []byte {
	count := len(params)
	for count > 0 && params[count-1] == "" {
		count--
	}
	buf := protocol.NewBuffer(10 + len(params)*8)
	buf.WriteU8(eventType)
	buf.WriteU8(uint8(count))
	for _, param := range params[:count] {
		buf.WriteCString(param)
	}
	switch eventType {
	case guildEventJoined, guildEventLeft, guildEventSignedOn, guildEventSignedOff:
		buf.WriteU64(guid)
	}
	return buf.Bytes()
}

// Petition result codes mirroring TrinityCore PetitionMgr.h:30-42.
const (
	petitionTurnOk                 uint32 = 0
	petitionTurnAlreadyInGuild     uint32 = 2
	petitionTurnNeedMoreSignatures uint32 = 4

	petitionSignOk             uint32 = 0
	petitionSignAlreadySigned  uint32 = 1
	petitionSignAlreadyInGuild uint32 = 2
	petitionSignCantSignOwn    uint32 = 3
	petitionSignNotServer      uint32 = 4
)

// Guild command types mirroring TrinityCore Guild.h:103-121.
const (
	guildCmdCreate       uint32 = 0
	guildCmdInvite       uint32 = 1
	guildCmdQuit         uint32 = 3
	guildCmdRoster       uint32 = 5
	guildCmdPromote      uint32 = 6
	guildCmdDemote       uint32 = 7
	guildCmdRemove       uint32 = 8
	guildCmdChangeLeader uint32 = 10
	guildCmdEditMotd     uint32 = 11
	guildCmdGuildChat    uint32 = 13
	guildCmdFounder      uint32 = 14
	guildCmdChangeRank   uint32 = 16
	guildCmdPublicNote   uint32 = 19
	guildCmdViewTab      uint32 = 21
	guildCmdMoveItem     uint32 = 22
	guildCmdRepair       uint32 = 25
)

// Guild command errors mirroring TrinityCore Guild.h:123-149.
const (
	errGuildCommandSuccess    uint32 = 0
	errGuildInternal          uint32 = 1
	errAlreadyInGuild         uint32 = 2
	errAlreadyInGuildS        uint32 = 3
	errInvitedToGuild         uint32 = 4
	errAlreadyInvitedToGuildS uint32 = 5
	errGuildNameInvalid       uint32 = 6
	errGuildNameExists        uint32 = 7
	errGuildLeaderLeave       uint32 = 8
	errGuildPermissions       uint32 = 8
	errGuildPlayerNotInGuild  uint32 = 9
	errGuildPlayerNotInGuildS uint32 = 10
	errGuildPlayerNotFoundS   uint32 = 11
	errGuildNotAllied         uint32 = 12
	errGuildRankTooHighS      uint32 = 13
	errGuildRankTooLowS       uint32 = 14
	errGuildRanksLocked       uint32 = 17
	errGuildRankInUse         uint32 = 18
	errGuildIgnoringYouS      uint32 = 19
	errGuildWithdrawLimit     uint32 = 25
	errGuildNotEnoughMoney    uint32 = 26
	errGuildBankFull          uint32 = 28
	errGuildItemNotFound      uint32 = 29
)

// Guild bank event log types mirroring TrinityCore Guild.h:185-196.
const (
	guildBankLogDepositItem   uint8 = 1
	guildBankLogWithdrawItem  uint8 = 2
	guildBankLogMoveItem      uint8 = 3
	guildBankLogDepositMoney  uint8 = 4
	guildBankLogWithdrawMoney uint8 = 5
	guildBankLogRepairMoney   uint8 = 6
	guildBankLogMoveItem2     uint8 = 7
	guildBankLogBuySlot       uint8 = 9

	guildBankMaxTabs      uint8 = 6
	guildBankMoneyLogsTab uint8 = 100
)

func (s *session) sendGuildCommandResult(cmdType uint32, param string, errCode uint32) {
	buf := protocol.NewBuffer(12 + len(param))
	buf.WriteI32(int32(cmdType))
	buf.WriteCString(param)
	buf.WriteI32(int32(errCode))
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_COMMAND_RESULT), buf.Bytes(), true)
}

func (s *session) sendGuildLoginInfo(ctx context.Context) {
	if s == nil || s.player == nil || s.player.GuildID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var motd string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT motd FROM guild WHERE guildid = ? LIMIT 1", s.player.GuildID).Scan(&motd); err != nil {
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), guildEventPayload(2, 0, motd), true)
	s.sendGuildBankTabsInfo(ctx)
	_ = s.handleGuildRoster(ctx)
	s.broadcastGuildMemberLogin()
}

func (s *session) sendGuildBankTabsInfo(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	var guildID, bankMoney, rank int64
	if err := cdb.QueryRowContext(ctx, "SELECT g.guildid, g.BankMoney, gm.rank FROM guild_member AS gm JOIN guild AS g ON g.guildid = gm.guildid WHERE gm.guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &bankMoney, &rank); err != nil || guildID == 0 {
		return
	}
	remaining := int32(-1)
	if rank != 0 {
		var rights, limit int64
		if err := cdb.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = 0 AND rid = ?", guildID, rank).Scan(&rights, &limit); err != nil || rights&1 == 0 {
			remaining = 0
		} else if limit != int64(^uint32(0)) {
			var withdrawn int64
			_ = cdb.QueryRowContext(ctx, "SELECT tab0 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&withdrawn)
			if limit <= withdrawn {
				remaining = 0
			} else {
				remaining = int32(limit - withdrawn)
			}
		}
	}
	buf := protocol.NewBuffer(15)
	buf.WriteU64(uint64(bankMoney))
	buf.WriteU8(0)
	buf.WriteI32(remaining)
	buf.WriteU8(0)
	buf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_BANK_LIST), buf.Bytes(), true)
}

func (s *session) broadcastGuildMemberLogout() {
	if s == nil || s.player == nil || s.player.GuildID == 0 || s.server == nil {
		return
	}
	event := guildEventPayload(guildEventSignedOff, s.playerGUID, s.player.Name)
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if target == s || !target.worldReady.Load() || target.player == nil || target.player.GuildID != s.player.GuildID {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
}

func (s *session) broadcastGuildMemberLogin() {
	if s == nil || s.player == nil || s.player.GuildID == 0 || s.server == nil {
		return
	}
	event := guildEventPayload(guildEventSignedOn, s.playerGUID, s.player.Name)
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != s.player.GuildID {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
}

type guildRankInfo struct {
	RankID            uint32
	Name              string
	Rights            uint32
	GoldLimit         uint32
	TabFlags          [6]uint32
	TabWithdrawLimits [6]uint32
}

func (s *session) handleGuildQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	guildID, err := reader.ReadU32()
	if err != nil || guildID == 0 {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var name string
	var emblemStyle, emblemColor, borderStyle, borderColor, bgColor int64
	err = cdb.QueryRowContext(ctx, "SELECT name, EmblemStyle, EmblemColor, BorderStyle, BorderColor, BackgroundColor FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name, &emblemStyle, &emblemColor, &borderStyle, &borderColor, &bgColor)
	if err != nil {
		return true
	}
	rankRows, err := cdb.QueryContext(ctx, "SELECT rid, rname FROM guild_rank WHERE guildid = ? ORDER BY rid", guildID)
	var ranks []string
	if err == nil {
		defer rankRows.Close()
		for rankRows.Next() {
			var rid int
			var rname string
			if scanErr := rankRows.Scan(&rid, &rname); scanErr == nil {
				ranks = append(ranks, rname)
			}
		}
	}
	if len(ranks) == 0 {
		ranks = []string{"Guild Master", "Officer", "Veteran", "Member", "Initiate"}
	}
	buf := protocol.NewBuffer(256)
	buf.WriteU32(guildID)
	buf.WriteCString(name)
	for _, r := range ranks {
		buf.WriteCString(r)
	}
	buf.WriteU32(uint32(emblemStyle))
	buf.WriteU32(uint32(emblemColor))
	buf.WriteU32(uint32(borderStyle))
	buf.WriteU32(uint32(borderColor))
	buf.WriteU32(uint32(bgColor))
	buf.WriteU32(uint32(len(ranks)))
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_QUERY_RESPONSE), buf.Bytes(), true)
	s.debug("guild query response sent", "guild_id", guildID, "name", name)
	return true
}

func (s *session) handleGuildRoster(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		// Not in guild
		s.sendGuildCommandResult(guildCmdRoster, "", errGuildPlayerNotInGuild)
		return true
	}
	var motd, info string
	_ = cdb.QueryRowContext(ctx, "SELECT motd, info FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&motd, &info)

	var ranks []guildRankInfo
	rankRows, err := cdb.QueryContext(ctx, "SELECT rid, rname, rights, BankMoneyPerDay FROM guild_rank WHERE guildid = ? ORDER BY rid", guildID)
	if err == nil {
		for rankRows.Next() {
			var rid, rights, goldLimit int64
			var rname string
			if scanErr := rankRows.Scan(&rid, &rname, &rights, &goldLimit); scanErr == nil {
				ranks = append(ranks, guildRankInfo{
					RankID:    uint32(rid),
					Name:      rname,
					Rights:    uint32(rights),
					GoldLimit: uint32(goldLimit),
				})
			}
		}
		rankRows.Close()
	}
	if rightsRows, rightsErr := cdb.QueryContext(ctx, "SELECT rid, TabId, gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? ORDER BY rid, TabId", guildID); rightsErr == nil {
		for rightsRows.Next() {
			var rid, tabID, flags, limit int64
			if rightsRows.Scan(&rid, &tabID, &flags, &limit) != nil || tabID < 0 || tabID >= 6 {
				continue
			}
			for index := range ranks {
				if ranks[index].RankID == uint32(rid) {
					ranks[index].TabFlags[tabID] = uint32(flags)
					ranks[index].TabWithdrawLimits[tabID] = uint32(limit)
					break
				}
			}
		}
		rightsRows.Close()
	}
	if len(ranks) == 0 {
		ranks = []guildRankInfo{
			{RankID: 0, Name: "Guild Master", Rights: 0xFFFFFFFF, GoldLimit: 1000000},
			{RankID: 1, Name: "Officer", Rights: 0x000000FF, GoldLimit: 500000},
			{RankID: 2, Name: "Veteran", Rights: 0x00000040, GoldLimit: 100000},
			{RankID: 3, Name: "Member", Rights: 0x00000040, GoldLimit: 50000},
			{RankID: 4, Name: "Initiate", Rights: 0x00000040, GoldLimit: 0},
		}
	}
	viewOfficerNote := false
	for _, rank := range ranks {
		if rank.RankID == uint32(s.player.GuildRank) {
			viewOfficerNote = rank.Rights&guildRightViewOfficerNote != 0
			break
		}
	}

	var members []guildMemberInfo
	memRows, err := cdb.QueryContext(ctx, `SELECT gm.guid, c.name, gm.rank, c.level, c.class, c.gender, c.zone, c.logout_time, gm.pnote, gm.offnote
		FROM guild_member AS gm
		JOIN characters AS c ON c.guid = gm.guid
		WHERE gm.guildid = ? ORDER BY gm.guid`, guildID)
	if err == nil {
		defer memRows.Close()
		for memRows.Next() {
			var mGuid, rank, lvl, cls, gnd, zone, logoutTime int64
			var mName, pNote, offNote string
			if scanErr := memRows.Scan(&mGuid, &mName, &rank, &lvl, &cls, &gnd, &zone, &logoutTime, &pNote, &offNote); scanErr == nil {
				status := uint8(0)
				if memberSession := s.server.findSessionByGUID(uint64(mGuid)); memberSession != nil && memberSession.player != nil {
					zone = int64(memberSession.player.Zone)
					status = 1
					if memberSession.player.PlayerFlags&playerFlagAFK != 0 {
						status |= 2
					}
					if memberSession.player.PlayerFlags&playerFlagDND != 0 {
						status |= 4
					}
				}
				lastSave := float32(0)
				if status == 0 && logoutTime > 0 {
					elapsed := time.Now().Unix() - logoutTime
					if elapsed > 0 {
						lastSave = float32(elapsed) / 86400
					}
				}
				if !viewOfficerNote {
					offNote = ""
				}
				members = append(members, guildMemberInfo{
					GUID:        uint64(mGuid),
					Name:        mName,
					RankID:      int32(rank),
					Level:       uint8(lvl),
					ClassID:     uint8(cls),
					Gender:      uint8(gnd),
					AreaID:      int32(zone),
					LastSave:    lastSave,
					Note:        pNote,
					OfficerNote: offNote,
					Status:      status,
				})
			}
		}
	}

	buf := protocol.NewBuffer(512 + len(members)*64)
	buf.WriteU32(uint32(len(members)))
	buf.WriteCString(motd)
	buf.WriteCString(info)
	buf.WriteU32(uint32(len(ranks)))
	for _, r := range ranks {
		buf.WriteU32(r.Rights)
		buf.WriteU32(r.GoldLimit)
		for i := 0; i < 6; i++ {
			buf.WriteU32(r.TabFlags[i])
			buf.WriteU32(r.TabWithdrawLimits[i])
		}
	}
	for _, m := range members {
		buf.WriteU64(m.GUID)
		buf.WriteU8(m.Status)
		buf.WriteCString(m.Name)
		buf.WriteI32(m.RankID)
		buf.WriteU8(m.Level)
		buf.WriteU8(m.ClassID)
		buf.WriteU8(m.Gender)
		buf.WriteI32(m.AreaID)
		if m.Status == 0 {
			buf.WriteF32(m.LastSave)
		}
		buf.WriteCString(m.Note)
		buf.WriteCString(m.OfficerNote)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_ROSTER), buf.Bytes(), true)
	return true
}

func (s *session) handleGuildInvite(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	reader := protocol.NewReader(payload)
	targetName, err := reader.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	var guildName string
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.name FROM guild_member AS gm
		JOIN guild AS g ON g.guildid = gm.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &guildName)
	if err != nil || guildID == 0 {
		return true
	}
	var targetGUID int64
	err = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", targetName).Scan(&targetGUID)
	if err != nil || targetGUID == 0 {
		return true
	}
	targetSess := s.server.findSessionByGUID(uint64(targetGUID))
	if targetSess == nil || targetSess.player == nil {
		return true
	}
	targetSess.guildInvitedID = uint32(guildID)
	targetSess.guildInviterGUID = s.playerGUID

	invBuf := protocol.NewBuffer(128)
	invBuf.WriteCString(s.player.Name)
	invBuf.WriteCString(guildName)
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_GUILD_INVITE), invBuf.Bytes(), true)
	s.debug("guild invite sent", "from", s.player.Name, "to", targetName, "guild", guildName)
	return true
}

func (s *session) handleGuildAccept(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.guildInvitedID == 0 {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	guildID := s.guildInvitedID
	s.guildInvitedID = 0
	s.guildInviterGUID = 0
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO guild_member (guildid, guid, rank, pnote, offnote) VALUES (?, ?, 4, '', '')", guildID, s.playerGUID)
	s.player.GuildID = guildID
	s.player.GuildRank = 4
	s.sendPlayerUpdate()

	// Broadcast join event
	eventBuf := protocol.NewBuffer(64)
	eventBuf.WriteU8(3) // GE_JOINED
	eventBuf.WriteU8(1) // Param count
	eventBuf.WriteCString(s.player.Name)
	eventBuf.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)

	return s.handleGuildRoster(ctx)
}

func (s *session) handleGuildDecline(ctx context.Context) bool {
	if s.guildInviterGUID != 0 && s.server != nil && s.player != nil {
		inviterSess := s.server.findSessionByGUID(s.guildInviterGUID)
		if inviterSess != nil && inviterSess.worldReady.Load() {
			eventBuf := protocol.NewBuffer(64)
			eventBuf.WriteU8(2) // GE_DECLINED
			eventBuf.WriteU8(1)
			eventBuf.WriteCString(s.player.Name)
			_ = inviterSess.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)
		}
	}
	s.guildInvitedID = 0
	s.guildInviterGUID = 0
	return true
}

func (s *session) handleGuildLeave(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ?", s.playerGUID)
	s.player.GuildID = 0
	s.player.GuildRank = 0
	s.sendPlayerUpdate()
	eventBuf := protocol.NewBuffer(64)
	eventBuf.WriteU8(4) // GE_LEFT
	eventBuf.WriteU8(1)
	eventBuf.WriteCString(s.player.Name)
	eventBuf.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)
	return true
}

func (s *session) handleGuildMotd(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	motd, err := reader.ReadCString()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET motd = ? WHERE guildid = ?", motd, guildID)

	eventBuf := protocol.NewBuffer(128)
	eventBuf.WriteU8(5) // GE_MOTD
	eventBuf.WriteU8(1)
	eventBuf.WriteCString(motd)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)
	return true
}

// handleGuildCreate processes CMSG_GUILD_CREATE (0x081).
// Reference: WorldSession::HandleGuildCreateOpcode (GuildHandler.cpp:38).
func (s *session) handleGuildCreate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	guildName, err := r.ReadCString()
	if err != nil || guildName == "" {
		return true
	}
	s.debug("guild create rejected", "account", s.accountName, "name", guildName)
	return true
}

// handleGuildInfo processes CMSG_GUILD_INFO (0x087).
// Reference: WorldSession::HandleGuildInfoOpcode (GuildHandler.cpp:77).
func (s *session) handleGuildInfo(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	var name string
	var createdDate int64
	err = cdb.QueryRowContext(ctx, "SELECT name, createdate FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name, &createdDate)
	if err != nil {
		return true
	}

	var memberCount, accountCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*), COUNT(DISTINCT c.account) FROM guild_member gm JOIN characters c ON c.guid = gm.guid WHERE gm.guildid = ?", guildID).Scan(&memberCount, &accountCount)

	buf := protocol.NewBuffer(128)
	buf.WriteCString(name)
	buf.WriteU32(uint32(createdDate))
	buf.WriteI32(int32(memberCount))
	buf.WriteI32(int32(accountCount))
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_INFO), buf.Bytes(), true)
	s.debug("guild info sent", "guild", name, "members", memberCount)
	return true
}

// handleGuildPromote processes CMSG_GUILD_PROMOTE (0x08B).
// Reference: WorldSession::HandleGuildPromoteOpcode (GuildHandler.cpp:95).
func (s *session) handleGuildPromote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	targetName, err := r.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, myRank int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid, rank FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &myRank)
	if err != nil || guildID == 0 {
		return true
	}

	var targetGUID, targetRank int64
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, targetName).Scan(&targetGUID, &targetRank)
	if err != nil || targetGUID == 0 {
		return true
	}

	if targetRank <= myRank+1 || targetRank <= 1 {
		return true
	}

	newRank := targetRank - 1
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = ? WHERE guid = ? AND guildid = ?", newRank, targetGUID, guildID)

	eventBuf := protocol.NewBuffer(128)
	eventBuf.WriteU8(0) // GE_PROMOTION
	eventBuf.WriteU8(2)
	eventBuf.WriteCString(s.player.Name)
	eventBuf.WriteCString(targetName)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)

	return s.handleGuildRoster(ctx)
}

// handleGuildDemote processes CMSG_GUILD_DEMOTE (0x08C).
// Reference: WorldSession::HandleGuildDemoteOpcode (GuildHandler.cpp:104).
func (s *session) handleGuildDemote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	targetName, err := r.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, myRank int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid, rank FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &myRank)
	if err != nil || guildID == 0 {
		return true
	}

	var targetGUID, targetRank int64
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, targetName).Scan(&targetGUID, &targetRank)
	if err != nil || targetGUID == 0 {
		return true
	}

	var maxRank int64 = 4
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(rid), 4) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRank)

	if targetRank <= myRank || targetRank >= maxRank {
		return true
	}

	newRank := targetRank + 1
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = ? WHERE guid = ? AND guildid = ?", newRank, targetGUID, guildID)

	eventBuf := protocol.NewBuffer(128)
	eventBuf.WriteU8(1) // GE_DEMOTION
	eventBuf.WriteU8(2)
	eventBuf.WriteCString(s.player.Name)
	eventBuf.WriteCString(targetName)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)

	return s.handleGuildRoster(ctx)
}

// handleGuildLeader processes CMSG_GUILD_LEADER (0x090).
// Reference: WorldSession::HandleGuildSetGuildMaster (GuildHandler.cpp:129).
func (s *session) handleGuildLeader(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	newMasterName, err := r.ReadCString()
	if err != nil || newMasterName == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	var newLeaderGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, newMasterName).Scan(&newLeaderGUID)
	if err != nil || newLeaderGUID == 0 || uint64(newLeaderGUID) == s.playerGUID {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET leaderguid = ? WHERE guildid = ?", newLeaderGUID, guildID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = 1 WHERE guid = ? AND guildid = ?", s.playerGUID, guildID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = 0 WHERE guid = ? AND guildid = ?", newLeaderGUID, guildID)

	eventBuf := protocol.NewBuffer(128)
	eventBuf.WriteU8(7) // GE_LEADER_CHANGED
	eventBuf.WriteU8(2)
	eventBuf.WriteCString(s.player.Name)
	eventBuf.WriteCString(newMasterName)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)

	return s.handleGuildRoster(ctx)
}

// handleGuildRemove processes CMSG_GUILD_REMOVE (0x08E).
// Reference: WorldSession::HandleGuildRemoveOpcode (GuildHandler.cpp:51).
func (s *session) handleGuildRemove(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	removee, err := r.ReadCString()
	if err != nil || removee == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, myRank int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid, rank FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &myRank)
	if err != nil || guildID == 0 {
		return true
	}

	var targetGUID, targetRank int64
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, removee).Scan(&targetGUID, &targetRank)
	if err != nil || targetGUID == 0 {
		return true
	}

	if targetRank <= myRank {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ? AND guildid = ?", targetGUID, guildID)

	eventBuf := protocol.NewBuffer(128)
	eventBuf.WriteU8(5) // GE_REMOVED
	eventBuf.WriteU8(2)
	eventBuf.WriteCString(removee)
	eventBuf.WriteCString(s.player.Name)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)

	return s.handleGuildRoster(ctx)
}

// handleGuildDisband processes CMSG_GUILD_DISBAND (0x08F).
// Reference: WorldSession::HandleGuildDelete (GuildHandler.cpp:121).
func (s *session) handleGuildDisband(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err := cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_rank WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_tab WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_item WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_eventlog WHERE guildid = ?", guildID)

	eventBuf := protocol.NewBuffer(32)
	eventBuf.WriteU8(8) // GE_DISBANDED
	eventBuf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)
	s.debug("guild disbanded", "guild_id", guildID)
	return true
}

// handleGuildAddRank processes CMSG_GUILD_ADD_RANK (0x232).
// Reference: WorldSession::HandleGuildAddRankOpcode (GuildHandler.cpp:181).
func (s *session) handleGuildAddRank(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	rankName, err := r.ReadCString()
	if err != nil || rankName == "" {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	var maxRid sql.NullInt64
	_ = cdb.QueryRowContext(ctx, "SELECT MAX(rid) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRid)
	newRid := 0
	if maxRid.Valid {
		newRid = int(maxRid.Int64) + 1
	}
	if newRid > 9 {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, ?, ?, 0x00000040, 0)", guildID, newRid, rankName)
	return s.handleGuildRoster(ctx)
}

// handleGuildDelRank processes CMSG_GUILD_DEL_RANK (0x233).
// Reference: WorldSession::HandleGuildDeleteRank (GuildHandler.cpp:189).
func (s *session) handleGuildDelRank(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err := cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	var maxRid sql.NullInt64
	_ = cdb.QueryRowContext(ctx, "SELECT MAX(rid) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRid)
	if !maxRid.Valid || maxRid.Int64 <= 1 {
		return true
	}

	lowestRank := maxRid.Int64
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, lowestRank)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = ? WHERE guildid = ? AND rank = ?", lowestRank-1, guildID, lowestRank)

	return s.handleGuildRoster(ctx)
}

// handleGuildRank processes CMSG_GUILD_RANK (0x231).
// Reference: WorldSession::HandleGuildSetRankPermissions (GuildHandler.cpp:166).
func (s *session) handleGuildRank(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	rankID, err := r.ReadU32()
	if err != nil {
		return false
	}
	rights, err := r.ReadU32()
	if err != nil {
		return false
	}
	rankName, err := r.ReadCString()
	if err != nil {
		return false
	}
	goldLimit, _ := r.ReadU32()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE guild_rank SET rname = ?, rights = ?, BankMoneyPerDay = ? WHERE guildid = ? AND rid = ?", rankName, rights, goldLimit, guildID, rankID)
	return s.handleGuildRoster(ctx)
}

// handleGuildSetPublicNote processes CMSG_GUILD_SET_PUBLIC_NOTE (0x234).
// Reference: WorldSession::HandleGuildSetPublicNoteOpcode (GuildHandler.cpp:146).
func (s *session) handleGuildSetPublicNote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	targetName, err := r.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	note, _ := r.ReadCString()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	_, _ = cdb.ExecContext(ctx, `UPDATE guild_member SET pnote = ?
		WHERE guildid = ? AND guid = (SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1)`, note, guildID, targetName)

	return s.handleGuildRoster(ctx)
}

// handleGuildSetOfficerNote processes CMSG_GUILD_SET_OFFICER_NOTE (0x235).
// Reference: WorldSession::HandleGuildSetOfficerNoteOpcode (GuildHandler.cpp:156).
func (s *session) handleGuildSetOfficerNote(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	targetName, err := r.ReadCString()
	if err != nil || targetName == "" {
		return false
	}
	note, _ := r.ReadCString()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	_, _ = cdb.ExecContext(ctx, `UPDATE guild_member SET offnote = ?
		WHERE guildid = ? AND guid = (SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1)`, note, guildID, targetName)

	return s.handleGuildRoster(ctx)
}

// handleGuildInfoText processes CMSG_GUILD_INFO_TEXT (0x2FC).
// Reference: WorldSession::HandleGuildUpdateInfoText (GuildHandler.cpp:197).
func (s *session) handleGuildInfoText(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	infoText, err := r.ReadCString()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET info = ? WHERE guildid = ?", infoText, guildID)
	return true
}

func (s *Server) getGuildName(ctx context.Context, guildID uint32) string {
	if guildID == 0 || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return ""
	}
	var name string
	_ = s.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name)
	return name
}

// handleGuildEventLogQuery processes MSG_GUILD_EVENT_LOG_QUERY (0x3FF).
// Reference: WorldSession::HandleGuildEventLogQueryOpcode (GuildHandler.cpp:111).
func (s *session) handleGuildEventLogQuery(ctx context.Context, payload []byte) bool {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0) // count = 0 entries
	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_EVENT_LOG_QUERY), buf.Bytes(), true)
	return true
}

// handleGuildPermissions processes MSG_GUILD_PERMISSIONS (0x3FD).
// Reference: WorldSession::HandleGuildPermissions (GuildHandler.cpp:89).
func (s *session) handleGuildPermissions(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	var rankID, rights, goldLimit int64
	rankID = 0
	rights = 0xFFFFFFFF
	goldLimit = 1000000

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		_ = cdb.QueryRowContext(ctx, `SELECT gr.rid, gr.rights, gr.BankMoneyPerDay
			FROM guild_member gm
			JOIN guild_rank gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
			WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&rankID, &rights, &goldLimit)
	}

	buf := protocol.NewBuffer(16 + 6*8)
	buf.WriteU32(uint32(rankID))
	buf.WriteU32(uint32(rights))
	buf.WriteU32(uint32(goldLimit))
	buf.WriteU8(6) // 6 tabs
	for i := 0; i < 6; i++ {
		buf.WriteU32(0xFFFFFFFF) // full rights
		buf.WriteU32(1000)       // slot limit
	}
	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_PERMISSIONS), buf.Bytes(), true)
	return true
}

// handleInspectArenaTeams processes MSG_INSPECT_ARENA_TEAMS (0x377).
// Reference: WorldSession::HandleInspectArenaTeamsOpcode (ArenaTeamHandler.cpp:333).
func (s *session) handleInspectArenaTeams(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, _ := r.ReadU64()

	buf := protocol.NewBuffer(8 + 3*24)
	buf.WriteU64(targetGUID)
	for slot := 0; slot < 3; slot++ {
		buf.WriteU32(0) // arenaTeamID
		buf.WriteU32(0) // rating
		buf.WriteU32(0) // seasonGames
		buf.WriteU32(0) // seasonWins
		buf.WriteU32(0) // played
		buf.WriteU32(0) // personalRating
	}
	_ = s.write(uint16(protocol.OpcodeMSG_INSPECT_ARENA_TEAMS), buf.Bytes(), true)
	return true
}

// handleInspectHonorStats processes MSG_INSPECT_HONOR_STATS (0x2D6).
// Reference: WorldSession::HandleInspectHonorStatsOpcode (MiscHandler.cpp:1060).
func (s *session) handleInspectHonorStats(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, _ := r.ReadU64()

	var honorPoints uint8
	var killsToday, todayContrib, yestContrib, lifetimeHK uint32
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		charLow := uint32(targetGUID & 0x00FFFFFF)
		var hp, tk, hk, thp, yhp int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT totalHonorPoints, todayKills, totalKills, todayHonorPoints, yesterdayHonorPoints FROM characters WHERE guid = ?`, charLow).Scan(&hp, &tk, &hk, &thp, &yhp); err == nil {
			honorPoints = uint8(hp)
			killsToday = uint32(tk)
			lifetimeHK = uint32(hk)
			todayContrib = uint32(thp)
			yestContrib = uint32(yhp)
		}
	}

	// TrinityCore MiscHandler.cpp:1060: data(MSG_INSPECT_HONOR_STATS, 8+1+4*4)
	buf := protocol.NewBuffer(8 + 1 + 4*4)
	buf.WriteU64(targetGUID)
	buf.WriteU8(honorPoints)
	buf.WriteU32(killsToday)
	buf.WriteU32(todayContrib)
	buf.WriteU32(yestContrib)
	buf.WriteU32(lifetimeHK)
	_ = s.write(uint16(protocol.OpcodeMSG_INSPECT_HONOR_STATS), buf.Bytes(), true)
	return true
}

// handlePvpLogData processes MSG_PVP_LOG_DATA (0x2E0).
// Reference: WorldSession::HandlePVPLogDataOpcode (BattlegroundHandler.cpp:211).
func (s *session) handlePvpLogData(ctx context.Context, payload []byte) bool {
	if s.server != nil && s.player != nil && IsArenaMap(s.player.Map) {
		if arena := s.server.findArenaState(s.player.Map, 0); arena != nil {
			pkt := s.server.buildArenaPvPLogDataPacket(arena)
			if len(pkt) > 0 {
				_ = s.write(uint16(protocol.OpcodeMSG_PVP_LOG_DATA), pkt, true)
				return true
			}
		}
	}
	buf := protocol.NewBuffer(16)
	buf.WriteU8(0)  // arena (0)
	buf.WriteU32(0) // count (0)
	buf.WriteU8(0)  // winner
	_ = s.write(uint16(protocol.OpcodeMSG_PVP_LOG_DATA), buf.Bytes(), true)
	return true
}

type guildBankTabInfo struct {
	ID   uint8
	Name string
	Icon string
	Text string
}

type guildBankSlotItem struct {
	Slot  uint8
	Entry uint32
	Count int32
}

// handleGuildBankerActivate processes CMSG_GUILD_BANKER_ACTIVATE (0x3E6).
// Reference: WorldSession::HandleGuildBankActivate (GuildHandler.cpp:251).
func (s *session) handleGuildBankerActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	fullUpdate := uint8(1)
	if len(payload) >= 9 {
		fullUpdate, _ = r.ReadU8()
	}

	return s.sendGuildBankList(ctx, bankerGUID, 0, fullUpdate != 0)
}

// handleGuildBankQueryTab processes CMSG_GUILD_BANK_QUERY_TAB (0x3E7).
// Reference: WorldSession::HandleGuildBankQueryTab (GuildHandler.cpp:271).
func (s *session) handleGuildBankQueryTab(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	tabID, err := r.ReadU8()
	if err != nil {
		return false
	}
	fullUpdate := uint8(0)
	if len(payload) >= 10 {
		fullUpdate, _ = r.ReadU8()
	}

	return s.sendGuildBankList(ctx, bankerGUID, tabID, fullUpdate != 0)
}

func (s *session) sendGuildBankList(ctx context.Context, bankerGUID uint64, tabID uint8, fullUpdate bool) bool {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, bankMoney int64
	err := cdb.QueryRowContext(ctx, `SELECT g.guildid, g.BankMoney FROM guild_member AS gm
		JOIN guild AS g ON g.guildid = gm.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &bankMoney)
	if err != nil || guildID == 0 {
		s.sendGuildCommandResult(guildCmdViewTab, "", errGuildPlayerNotInGuild)
		return true
	}

	var tabs []guildBankTabInfo
	rows, err := cdb.QueryContext(ctx, "SELECT TabId, TabName, TabIcon, COALESCE(TabText, '') FROM guild_bank_tab WHERE guildid = ? ORDER BY TabId", guildID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t guildBankTabInfo
			var tid int
			if scanErr := rows.Scan(&tid, &t.Name, &t.Icon, &t.Text); scanErr == nil {
				t.ID = uint8(tid)
				tabs = append(tabs, t)
			}
		}
	}
	if len(tabs) == 0 {
		tabs = []guildBankTabInfo{
			{ID: 0, Name: "General", Icon: "INV_Misc_Bag_08", Text: ""},
		}
	}

	var items []guildBankSlotItem
	itemRows, err := cdb.QueryContext(ctx, `SELECT gbi.SlotId, ii.itemEntry, ii.count
		FROM guild_bank_item gbi
		JOIN item_instance ii ON ii.guid = gbi.item_guid
		WHERE gbi.guildid = ? AND gbi.TabId = ?`, guildID, tabID)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var it guildBankSlotItem
			var slot, entry, count int64
			if scanErr := itemRows.Scan(&slot, &entry, &count); scanErr == nil {
				it.Slot = uint8(slot)
				it.Entry = uint32(entry)
				it.Count = int32(count)
				items = append(items, it)
			}
		}
	}

	buf := protocol.NewBuffer(256 + len(tabs)*64 + len(items)*32)
	buf.WriteU64(uint64(bankMoney))
	buf.WriteU8(tabID)
	buf.WriteI32(1000000) // WithdrawalsRemaining
	if fullUpdate {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}

	if tabID == 0 && fullUpdate {
		buf.WriteU8(uint8(len(tabs)))
		for _, tab := range tabs {
			buf.WriteCString(tab.Name)
			buf.WriteCString(tab.Icon)
		}
	}

	buf.WriteU8(uint8(len(items)))
	for _, it := range items {
		buf.WriteU8(it.Slot)
		buf.WriteU32(it.Entry)
		if it.Entry != 0 {
			buf.WriteI32(0) // Flags
			buf.WriteI32(0) // RandomPropertiesID
			buf.WriteI32(it.Count)
			buf.WriteI32(0) // EnchantmentID
			buf.WriteU8(0)  // Charges
			buf.WriteU8(0)  // SocketEnchant count
		}
	}

	return s.write(uint16(protocol.OpcodeSMSG_GUILD_BANK_LIST), buf.Bytes(), true) == nil
}

func (s *session) logGuildBankEvent(ctx context.Context, guildID uint32, tabID uint8, eventType uint8, playerGUID uint64, itemOrMoney uint32, stackCount uint32, destTabID uint8) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || guildID == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB

	dbTabID := tabID
	if eventType == guildBankLogDepositMoney || eventType == guildBankLogWithdrawMoney || eventType == guildBankLogRepairMoney {
		dbTabID = guildBankMoneyLogsTab
	}

	var maxLogGuid uint32
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(LogGuid), 0) FROM guild_bank_eventlog WHERE guildid = ? AND TabId = ?", guildID, dbTabID).Scan(&maxLogGuid)
	nextLogGuid := maxLogGuid + 1
	now := uint32(time.Now().Unix())

	_, _ = cdb.ExecContext(ctx, `INSERT INTO guild_bank_eventlog (guildid, LogGuid, TabId, EventType, PlayerGuid, ItemOrMoney, ItemStackCount, DestTabId, TimeStamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		guildID, nextLogGuid, dbTabID, eventType, uint32(playerGUID), itemOrMoney, stackCount, destTabID, now)

	// Keep up to 25 logs per tab
	rows, err := cdb.QueryContext(ctx, "SELECT LogGuid FROM guild_bank_eventlog WHERE guildid = ? AND TabId = ? ORDER BY TimeStamp DESC, LogGuid DESC", guildID, dbTabID)
	if err == nil {
		var logGuids []uint32
		for rows.Next() {
			var lg uint32
			if err := rows.Scan(&lg); err == nil {
				logGuids = append(logGuids, lg)
			}
		}
		rows.Close()
		if len(logGuids) > 25 {
			for _, lg := range logGuids[25:] {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_eventlog WHERE guildid = ? AND TabId = ? AND LogGuid = ?", guildID, dbTabID, lg)
			}
		}
	}
}

func (s *session) checkGuildBankRights(ctx context.Context, guildID uint32, tabID uint8, checkDeposit bool) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	var rank uint32
	err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, guildID).Scan(&rank)
	if err != nil {
		return false
	}
	if rank == 0 {
		return true // Guild Master has all rights
	}
	var gbright uint32
	err = cdb.QueryRowContext(ctx, "SELECT gbright FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright)
	if err != nil {
		return false
	}
	if gbright&0x01 == 0 {
		return false
	}
	if checkDeposit && (gbright&0x02 == 0) {
		return false
	}
	return true
}

func (s *session) checkAndConsumeGuildBankWithdraw(ctx context.Context, guildID uint32, tabID uint8) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || tabID >= guildBankMaxTabs {
		return false
	}
	cdb := s.server.CharactersStore.DB
	var rank uint32
	err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, guildID).Scan(&rank)
	if err != nil {
		return false
	}
	if rank == 0 {
		return true
	}
	var gbright, slotPerDay uint32
	err = cdb.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright, &slotPerDay)
	if err != nil || gbright&0x01 == 0 || slotPerDay == 0 {
		return false
	}
	if slotPerDay != 0xFFFFFFFF {
		tabCol := fmt.Sprintf("tab%d", tabID)
		var exists int
		_ = cdb.QueryRowContext(ctx, "SELECT 1 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&exists)
		if exists == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, 0, 0, 0, 0, 0, 0, 0)", s.playerGUID)
		}
		var currentWithdrawn uint32
		_ = cdb.QueryRowContext(ctx, "SELECT "+tabCol+" FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&currentWithdrawn)
		if currentWithdrawn >= slotPerDay {
			return false
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE guild_member_withdraw SET "+tabCol+" = "+tabCol+" + 1 WHERE guid = ?", s.playerGUID)
	}
	return true
}

func (s *session) checkAndConsumeGuildBankMoneyWithdraw(ctx context.Context, guildID uint32, amount uint32) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	var rank uint32
	err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, guildID).Scan(&rank)
	if err != nil {
		return false
	}
	if rank == 0 {
		return true
	}
	var bankMoneyPerDay uint32
	err = cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&bankMoneyPerDay)
	if err != nil || bankMoneyPerDay == 0 {
		return false
	}
	if bankMoneyPerDay != 0xFFFFFFFF {
		var exists int
		_ = cdb.QueryRowContext(ctx, "SELECT 1 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&exists)
		if exists == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, 0, 0, 0, 0, 0, 0, 0)", s.playerGUID)
		}
		var currentWithdrawn uint32
		_ = cdb.QueryRowContext(ctx, "SELECT money FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&currentWithdrawn)
		if currentWithdrawn+amount > bankMoneyPerDay {
			return false
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE guild_member_withdraw SET money = money + ? WHERE guid = ?", amount, s.playerGUID)
	}
	return true
}

// handleGuildBankSwapItems processes CMSG_GUILD_BANK_SWAP_ITEMS (0x3E9).
// Reference: WorldSession::HandleGuildBankSwapItems (GuildHandler.cpp:320).
func (s *session) handleGuildBankSwapItems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, _ := r.ReadU64()
	bankOnly, _ := r.ReadU8()

	guildID := s.player.GuildID
	if guildID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB

	var bankTab uint8
	if bankOnly != 0 {
		bankTab, _ = r.ReadU8()
		bankSlot, _ := r.ReadU8()
		_, _ = r.ReadU32() // itemID
		bankTab1, _ := r.ReadU8()
		bankSlot1, _ := r.ReadU8()
		_, _ = r.ReadU32() // itemID1

		if bankTab >= guildBankMaxTabs || bankTab1 >= guildBankMaxTabs {
			return true
		}

		// Permissions check
		if !s.checkGuildBankRights(ctx, guildID, bankTab, false) || !s.checkGuildBankRights(ctx, guildID, bankTab1, false) {
			s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildPermissions)
			return true
		}
		if bankTab != bankTab1 {
			if !s.checkGuildBankRights(ctx, guildID, bankTab1, true) {
				s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildPermissions)
				return true
			}
			if !s.checkAndConsumeGuildBankWithdraw(ctx, guildID, bankTab) {
				s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildWithdrawLimit)
				return true
			}
		}

		var item1, item2 uint64
		_ = cdb.QueryRowContext(ctx, "SELECT item_guid FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, bankTab, bankSlot).Scan(&item1)
		_ = cdb.QueryRowContext(ctx, "SELECT item_guid FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, bankTab1, bankSlot1).Scan(&item2)

		_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_item WHERE guildid = ? AND ((TabId = ? AND SlotId = ?) OR (TabId = ? AND SlotId = ?))", guildID, bankTab, bankSlot, bankTab1, bankSlot1)
		if item1 != 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_bank_item (guildid, TabId, SlotId, item_guid) VALUES (?, ?, ?, ?)", guildID, bankTab1, bankSlot1, item1)
			if bankTab != bankTab1 {
				var itemEntry, count uint32
				_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", item1).Scan(&itemEntry, &count)
				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogMoveItem, s.playerGUID, itemEntry, count, bankTab1)
			}
		}
		if item2 != 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_bank_item (guildid, TabId, SlotId, item_guid) VALUES (?, ?, ?, ?)", guildID, bankTab, bankSlot, item2)
			if bankTab != bankTab1 {
				var itemEntry2, count2 uint32
				_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", item2).Scan(&itemEntry2, &count2)
				s.logGuildBankEvent(ctx, guildID, bankTab1, guildBankLogMoveItem, s.playerGUID, itemEntry2, count2, bankTab)
			}
		}
	} else {
		bankTab, _ = r.ReadU8()
		bankSlot, _ := r.ReadU8()
		_, _ = r.ReadU32() // itemID
		autoStore, _ := r.ReadU8()

		if bankTab >= guildBankMaxTabs {
			return true
		}

		var containerSlot, containerItemSlot, toSlot uint8
		if autoStore != 0 {
			_, _ = r.ReadU32() // bankItemCount
			toSlot, _ = r.ReadU8()
			_, _ = r.ReadU32() // stackCount
			containerSlot = 0
			containerItemSlot = 23
		} else {
			containerSlot, _ = r.ReadU8()
			containerItemSlot, _ = r.ReadU8()
			toSlot, _ = r.ReadU8()
			_, _ = r.ReadU32() // stackCount
		}

		if toSlot != 0 {
			// Bank -> Player Inventory (Withdraw)
			if !s.checkGuildBankRights(ctx, guildID, bankTab, false) {
				s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildPermissions)
				return true
			}
			if !s.checkAndConsumeGuildBankWithdraw(ctx, guildID, bankTab) {
				s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildWithdrawLimit)
				return true
			}

			var bankItemGUID uint64
			err := cdb.QueryRowContext(ctx, "SELECT item_guid FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, bankTab, bankSlot).Scan(&bankItemGUID)
			if err == nil && bankItemGUID > 0 {
				var existingInvItem uint64
				_ = cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, containerSlot, containerItemSlot).Scan(&existingInvItem)

				var itemEntry, count uint32
				_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", bankItemGUID).Scan(&itemEntry, &count)
				var existingItemEntry, existingItemCount uint32
				if existingInvItem > 0 {
					_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", existingInvItem).Scan(&existingItemEntry, &existingItemCount)
				}

				_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, bankTab, bankSlot)
				if existingInvItem > 0 && existingItemEntry > 0 && existingItemCount > 0 {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, containerSlot, containerItemSlot)
					s.adjustQuestItemCount(ctx, existingItemEntry, existingItemCount, false)
				}
				_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, containerSlot, containerItemSlot, bankItemGUID)
				_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, bankItemGUID)
				if itemEntry > 0 && count > 0 {
					s.adjustQuestItemCount(ctx, itemEntry, count, true)
				}

				if existingInvItem > 0 {
					_, _ = cdb.ExecContext(ctx, "REPLACE INTO guild_bank_item (guildid, TabId, SlotId, item_guid) VALUES (?, ?, ?, ?)", guildID, bankTab, bankSlot, existingInvItem)
					_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = 0 WHERE guid = ?", existingInvItem)
				}

				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogWithdrawItem, s.playerGUID, itemEntry, count, 0)
				_ = s.sendInventoryItems(ctx)
				s.sendPlayerUpdate()
			}
		} else {
			// Player Inventory -> Bank (Deposit)
			if !s.checkGuildBankRights(ctx, guildID, bankTab, true) {
				s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildPermissions)
				return true
			}

			var invItemGUID uint64
			err := cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, containerSlot, containerItemSlot).Scan(&invItemGUID)
			if err == nil && invItemGUID > 0 {
				var existingBankItem uint64
				_ = cdb.QueryRowContext(ctx, "SELECT item_guid FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, bankTab, bankSlot).Scan(&existingBankItem)

				var itemEntry, count uint32
				_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", invItemGUID).Scan(&itemEntry, &count)
				var existingItemEntry, existingItemCount uint32
				if existingBankItem > 0 {
					_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", existingBankItem).Scan(&existingItemEntry, &existingItemCount)
				}

				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, containerSlot, containerItemSlot)
				if itemEntry > 0 && count > 0 {
					s.adjustQuestItemCount(ctx, itemEntry, count, false)
				}
				_, _ = cdb.ExecContext(ctx, "REPLACE INTO guild_bank_item (guildid, TabId, SlotId, item_guid) VALUES (?, ?, ?, ?)", guildID, bankTab, bankSlot, invItemGUID)
				_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = 0 WHERE guid = ?", invItemGUID)

				if existingBankItem > 0 {
					_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, containerSlot, containerItemSlot, existingBankItem)
					_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, existingBankItem)
					if existingItemEntry > 0 && existingItemCount > 0 {
						s.adjustQuestItemCount(ctx, existingItemEntry, existingItemCount, true)
					}
				}

				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogDepositItem, s.playerGUID, itemEntry, count, 0)
				_ = s.sendInventoryItems(ctx)
				s.sendPlayerUpdate()
			}
		}
	}
	s.sendGuildBankList(ctx, bankerGUID, bankTab, false)
	return true
}

var guildBankTabPrices = []uint32{
	100 * 10000,  // Tab 0 (first bought tab): 100 gold
	250 * 10000,  // Tab 1: 250 gold
	500 * 10000,  // Tab 2: 500 gold
	1000 * 10000, // Tab 3: 1000 gold
	2500 * 10000, // Tab 4: 2500 gold
	5000 * 10000, // Tab 5: 5000 gold
}

// handleGuildBankBuyTab processes CMSG_GUILD_BANK_BUY_TAB (0x3EA).
// Reference: WorldSession::HandleGuildBankBuyTab (GuildHandler.cpp:340).
func (s *session) handleGuildBankBuyTab(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	tabID, err := r.ReadU8()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	if int(tabID) >= len(guildBankTabPrices) {
		return true
	}
	cost := guildBankTabPrices[tabID]
	if s.player.Money < cost {
		return true
	}

	s.player.Money -= cost
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO guild_bank_tab (guildid, TabId, TabName, TabIcon, TabText) VALUES (?, ?, ?, 'INV_Misc_Bag_08', '')",
		guildID, tabID, "Tab "+string(rune('1'+tabID)))

	s.logGuildBankEvent(ctx, uint32(guildID), tabID, guildBankLogBuySlot, s.playerGUID, 0, 0, 0)
	s.sendPlayerUpdate()
	return s.sendGuildBankList(ctx, bankerGUID, tabID, true)
}

// handleGuildBankUpdateTab processes CMSG_GUILD_BANK_UPDATE_TAB (0x3EB).
// Reference: WorldSession::HandleGuildBankUpdateTab (GuildHandler.cpp:349).
func (s *session) handleGuildBankUpdateTab(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 10 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	tabID, err := r.ReadU8()
	if err != nil {
		return false
	}
	name, err := r.ReadCString()
	if err != nil {
		return false
	}
	icon, err := r.ReadCString()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE guild_bank_tab SET TabName = ?, TabIcon = ? WHERE guildid = ? AND TabId = ?", name, icon, guildID, tabID)

	return s.sendGuildBankList(ctx, bankerGUID, tabID, true)
}

// handleGuildBankDepositMoney processes CMSG_GUILD_BANK_DEPOSIT_MONEY (0x3EC).
// Reference: WorldSession::HandleGuildBankDepositMoney (GuildHandler.cpp:284).
func (s *session) handleGuildBankDepositMoney(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	amount, err := r.ReadU32()
	if err != nil || amount == 0 || s.player.Money < amount {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID)
	if err != nil || guildID == 0 {
		return true
	}

	s.player.Money -= amount
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET BankMoney = BankMoney + ? WHERE guildid = ?", amount, guildID)
	s.logGuildBankEvent(ctx, uint32(guildID), 0, guildBankLogDepositMoney, s.playerGUID, amount, 0, 0)

	return s.sendGuildBankList(ctx, bankerGUID, 0, false)
}

// handleGuildBankWithdrawMoney processes CMSG_GUILD_BANK_WITHDRAW_MONEY (0x3ED).
// Reference: WorldSession::HandleGuildBankWithdrawMoney (GuildHandler.cpp:295).
func (s *session) handleGuildBankWithdrawMoney(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	amount, err := r.ReadU32()
	if err != nil || amount == 0 {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var guildID, bankMoney int64
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.BankMoney FROM guild_member gm
		JOIN guild g ON g.guildid = gm.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &bankMoney)
	if err != nil || guildID == 0 || uint64(bankMoney) < uint64(amount) {
		return true
	}

	if !s.checkAndConsumeGuildBankMoneyWithdraw(ctx, uint32(guildID), amount) {
		s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildWithdrawLimit)
		return true
	}

	s.player.Money += amount
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET BankMoney = BankMoney - ? WHERE guildid = ?", amount, guildID)
	s.logGuildBankEvent(ctx, uint32(guildID), 0, guildBankLogWithdrawMoney, s.playerGUID, amount, 0, 0)

	return s.sendGuildBankList(ctx, bankerGUID, 0, false)
}

// handleGuildBankLogQuery processes MSG_GUILD_BANK_LOG_QUERY (0x3EE).
// Reference: WorldSession::HandleGuildBankLogQuery (GuildHandler.cpp:360).
func (s *session) handleGuildBankLogQuery(ctx context.Context, payload []byte) bool {
	tabID := uint8(0)
	if len(payload) >= 1 {
		r := protocol.NewReader(payload)
		tabID, _ = r.ReadU8()
	}

	cdb := s.server.CharactersStore.DB
	var guildID int64
	if s.player != nil {
		guildID = int64(s.player.GuildID)
	}
	if cdb == nil || guildID == 0 || tabID > guildBankMaxTabs {
		buf := protocol.NewBuffer(2)
		buf.WriteU8(tabID)
		buf.WriteU8(0) // 0 entries
		return s.write(uint16(protocol.OpcodeMSG_GUILD_BANK_LOG_QUERY), buf.Bytes(), true) == nil
	}

	dbTabID := tabID
	if tabID == guildBankMaxTabs {
		dbTabID = guildBankMoneyLogsTab
	}

	rows, err := cdb.QueryContext(ctx, "SELECT EventType, PlayerGuid, ItemOrMoney, ItemStackCount, DestTabId, TimeStamp FROM guild_bank_eventlog WHERE guildid = ? AND TabId = ? ORDER BY TimeStamp DESC, LogGuid DESC LIMIT 25", guildID, dbTabID)
	if err != nil {
		buf := protocol.NewBuffer(2)
		buf.WriteU8(tabID)
		buf.WriteU8(0)
		return s.write(uint16(protocol.OpcodeMSG_GUILD_BANK_LOG_QUERY), buf.Bytes(), true) == nil
	}

	type logRecord struct {
		eventType      uint8
		playerGUID     uint64
		itemOrMoney    uint32
		itemStackCount uint32
		destTabID      uint8
		timeOffset     uint32
	}
	var entries []logRecord
	now := uint32(time.Now().Unix())

	for rows.Next() {
		var et uint8
		var pGuid uint32
		var iom uint32
		var cnt uint32
		var dt uint8
		var ts uint32
		if err := rows.Scan(&et, &pGuid, &iom, &cnt, &dt, &ts); err == nil {
			toff := uint32(0)
			if now > ts {
				toff = now - ts
			}
			entries = append(entries, logRecord{
				eventType:      et,
				playerGUID:     uint64(pGuid),
				itemOrMoney:    iom,
				itemStackCount: cnt,
				destTabID:      dt,
				timeOffset:     toff,
			})
		}
	}
	rows.Close()

	buf := protocol.NewBuffer(2 + len(entries)*20)
	buf.WriteU8(tabID)
	buf.WriteU8(uint8(len(entries)))

	for _, e := range entries {
		buf.WriteU8(e.eventType)
		buf.WriteU64(e.playerGUID)
		switch e.eventType {
		case guildBankLogDepositItem, guildBankLogWithdrawItem:
			buf.WriteU32(e.itemOrMoney)
			buf.WriteU32(e.itemStackCount)
		case guildBankLogMoveItem, guildBankLogMoveItem2:
			buf.WriteU32(e.itemOrMoney)
			buf.WriteU32(e.itemStackCount)
			buf.WriteU8(e.destTabID)
		default:
			buf.WriteU32(e.itemOrMoney)
		}
		buf.WriteU32(e.timeOffset)
	}

	return s.write(uint16(protocol.OpcodeMSG_GUILD_BANK_LOG_QUERY), buf.Bytes(), true) == nil
}

// handleGuildBankMoneyWithdrawn processes MSG_GUILD_BANK_MONEY_WITHDRAWN (0x3FE).
// Reference: WorldSession::HandleGuildBankMoneyWithdrawn (GuildHandler.cpp:238).
func (s *session) handleGuildBankMoneyWithdrawn(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	remaining := int32(0)
	cdb := s.server.CharactersStore.DB
	if cdb != nil && s.player.GuildID != 0 {
		var rank uint32
		if err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, s.player.GuildID).Scan(&rank); err == nil {
			if rank == 0 {
				var bankMoney int64
				_ = cdb.QueryRowContext(ctx, "SELECT BankMoney FROM guild WHERE guildid = ?", s.player.GuildID).Scan(&bankMoney)
				remaining = int32(bankMoney)
			} else {
				var bankMoneyPerDay uint32
				_ = cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", s.player.GuildID, rank).Scan(&bankMoneyPerDay)
				if bankMoneyPerDay == 0xFFFFFFFF {
					var bankMoney int64
					_ = cdb.QueryRowContext(ctx, "SELECT BankMoney FROM guild WHERE guildid = ?", s.player.GuildID).Scan(&bankMoney)
					remaining = int32(bankMoney)
				} else if bankMoneyPerDay > 0 {
					var withdrawn uint32
					_ = cdb.QueryRowContext(ctx, "SELECT money FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&withdrawn)
					if bankMoneyPerDay > withdrawn {
						remaining = int32(bankMoneyPerDay - withdrawn)
					}
				}
			}
		}
	}
	buf := protocol.NewBuffer(8)
	buf.WriteI32(remaining)
	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_BANK_MONEY_WITHDRAWN), buf.Bytes(), true)
	return true
}

// handleQueryGuildBankText processes MSG_QUERY_GUILD_BANK_TEXT (0x40A).
// Reference: WorldSession::HandleGuildBankTextQuery (GuildHandler.cpp:368).
func (s *session) handleQueryGuildBankText(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	tabID, err := r.ReadU8()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	tabText := ""
	if cdb != nil {
		var guildID int64
		if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID); err == nil && guildID > 0 {
			var text sql.NullString
			_ = cdb.QueryRowContext(ctx, "SELECT TabText FROM guild_bank_tab WHERE guildid = ? AND TabId = ? LIMIT 1", guildID, tabID).Scan(&text)
			if text.Valid {
				tabText = text.String
			}
		}
	}

	buf := protocol.NewBuffer(64 + len(tabText))
	buf.WriteU8(tabID)
	buf.WriteCString(tabText)
	_ = s.write(uint16(protocol.OpcodeMSG_QUERY_GUILD_BANK_TEXT), buf.Bytes(), true)
	return true
}

// handleSetGuildBankText processes CMSG_SET_GUILD_BANK_TEXT (0x40B).
// Reference: WorldSession::HandleGuildBankSetTabText (GuildHandler.cpp:376).
func (s *session) handleSetGuildBankText(ctx context.Context, payload []byte) bool {
	if len(payload) < 2 {
		return true
	}
	r := protocol.NewReader(payload)
	tabID, err := r.ReadU8()
	if err != nil {
		return false
	}
	tabText, err := r.ReadCString()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var guildID int64
		if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID); err == nil && guildID > 0 {
			_, _ = cdb.ExecContext(ctx, "UPDATE guild_bank_tab SET TabText = ? WHERE guildid = ? AND TabId = ?", tabText, guildID, tabID)
		}
	}
	return true
}

const (
	guildCharterItemID = 5863
	guildCharterCost   = 1000 // 10s
)

// handlePetitionBuy processes CMSG_PETITION_BUY (0x1B6).
// Reference: WorldSession::HandlePetitionBuyOpcode (PetitionsHandler.cpp:48).
func (s *session) handlePetitionBuy(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // npc GUID
	_, _ = r.ReadU32() // 0
	_, _ = r.ReadU64() // 0
	name, err := r.ReadCString()
	if err != nil || name == "" {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	// Check if player already in guild
	var existingGuild int64
	_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&existingGuild)
	if existingGuild != 0 {
		return true
	}

	cost := uint32(1000) // 10 silver guild charter cost
	if s.player.Money < cost {
		return true
	}

	usedSlots := make(map[uint8]bool)
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var sl int64
			if rows.Scan(&sl) == nil {
				usedSlots[uint8(sl)] = true
			}
		}
		rows.Close()
	}
	freeSlot := uint8(0xFF)
	for sl := uint8(23); sl <= 38; sl++ {
		if !usedSlots[sl] {
			freeSlot = sl
			break
		}
	}
	if freeSlot == 0xFF {
		return true
	}

	var nextItemGUID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextItemGUID)
	if nextItemGUID <= 0 {
		nextItemGUID = 1
	}

	petitionGUID := uint64(nextItemGUID)
	s.player.Money -= cost
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, 5863, ?, ?, 1, 0, '', 0, '', 0, 0, 0, '')", nextItemGUID, s.playerGUID, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, 0, ?, ?)", s.playerGUID, freeSlot, nextItemGUID)
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO petition (ownerguid, petitionguid, name, type) VALUES (?, ?, ?, 9)",
		s.playerGUID, petitionGUID, name)

	_ = s.sendItemCreate(uint64(nextItemGUID), 5863, 1, 0, freeSlot)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}

// sendPetitionShowSignatures sends SMSG_PETITION_SHOW_SIGNATURES (0x1BF).
// Reference: WorldSession::SendPetitionSigns (PetitionsHandler.cpp:243).
func (s *session) sendPetitionShowSignatures(target *session, petitionGUID uint64) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || target == nil || target.player == nil || !target.worldReady.Load() {
		return
	}
	cdb := s.server.CharactersStore.DB

	var ownerGUID, petitionType int64
	if err := cdb.QueryRow("SELECT ownerguid, type FROM petition WHERE petitionguid = ? LIMIT 1", petitionGUID).Scan(&ownerGUID, &petitionType); err != nil || petitionType == 9 && target.player.GuildID != 0 {
		return
	}

	rows, err := cdb.Query("SELECT playerguid FROM petition_sign WHERE petitionguid = ?", petitionGUID)
	if err != nil {
		return
	}
	defer rows.Close()
	var signs []uint64
	for rows.Next() {
		var pguid int64
		if err := rows.Scan(&pguid); err != nil {
			return
		}
		signs = append(signs, uint64(pguid))
	}
	if rows.Err() != nil {
		return
	}

	buf := protocol.NewBuffer(32 + len(signs)*12)
	buf.WriteU64(petitionGUID)
	buf.WriteU64(uint64(ownerGUID))
	buf.WriteU32(uint32(petitionGUID & 0xFFFFFFFF))
	buf.WriteU8(uint8(len(signs)))
	for _, sg := range signs {
		buf.WriteU64(sg)
		buf.WriteU32(0)
	}
	_ = target.write(uint16(protocol.OpcodeSMSG_PETITION_SHOW_SIGNATURES), buf.Bytes(), true)
}

// handlePetitionShowSignatures processes CMSG_PETITION_SHOW_SIGNATURES (0x1BE).
// Reference: WorldSession::HandlePetitionShowSignatures (PetitionsHandler.cpp:220).
func (s *session) handlePetitionShowSignatures(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petitionGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	s.sendPetitionShowSignatures(s, petitionGUID)
	return true
}

// handlePetitionQuery processes CMSG_PETITION_QUERY (0x1C6).
// Reference: WorldSession::HandleQueryPetition (PetitionsHandler.cpp:261).
func (s *session) handlePetitionQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	guildGUID, _ := r.ReadU32()
	petitionGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var ownerGUID int64
	var name string
	var pType int64
	err = cdb.QueryRowContext(ctx, "SELECT ownerguid, name, type FROM petition WHERE petitionguid = ? LIMIT 1", petitionGUID).Scan(&ownerGUID, &name, &pType)
	if err != nil {
		return true
	}

	buf := protocol.NewBuffer(128 + len(name))
	buf.WriteU32(guildGUID)
	buf.WriteU64(uint64(ownerGUID))
	buf.WriteCString(name)
	buf.WriteU8(0)
	buf.WriteU32(9) // 9 signs needed for guild
	buf.WriteU32(9)
	buf.WriteU32(0)
	for i := 0; i < 10; i++ {
		buf.WriteCString("")
	}
	buf.WriteU32(uint32(pType))
	buf.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_QUERY_RESPONSE), buf.Bytes(), true)
	return true
}

// handlePetitionSign processes CMSG_PETITION_SIGN (0x1C0).
// Reference: WorldSession::HandleSignPetition (PetitionsHandler.cpp:383).
func (s *session) handlePetitionSign(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petitionGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var ownerGUID int64
	var pType int64
	err = cdb.QueryRowContext(ctx, "SELECT ownerguid, type FROM petition WHERE petitionguid = ? LIMIT 1", petitionGUID).Scan(&ownerGUID, &pType)
	if err != nil {
		return true
	}

	// Cannot sign own petition
	if uint64(ownerGUID) == s.playerGUID {
		buf := protocol.NewBuffer(20)
		buf.WriteU64(petitionGUID)
		buf.WriteU64(s.playerGUID)
		buf.WriteU32(petitionSignCantSignOwn)
		_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
		return true
	}

	// Cross-faction check
	var ownerRace int64
	_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ?", ownerGUID).Scan(&ownerRace)
	if s.player.Race != 0 && ownerRace != 0 && teamForRace(s.player.Race) != teamForRace(uint8(ownerRace)) {
		s.sendGuildCommandResult(guildCmdCreate, "", errGuildNotAllied)
		return true
	}

	// Already in guild check
	if s.player.GuildID != 0 {
		buf := protocol.NewBuffer(20)
		buf.WriteU64(petitionGUID)
		buf.WriteU64(s.playerGUID)
		buf.WriteU32(petitionSignAlreadyInGuild)
		_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
		return true
	}

	// Already signed check (by character or account)
	var alreadySigned int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM petition_sign WHERE petitionguid = ? AND (playerguid = ? OR player_account = ?)", petitionGUID, s.playerGUID, s.accountID).Scan(&alreadySigned)
	if alreadySigned > 0 {
		buf := protocol.NewBuffer(20)
		buf.WriteU64(petitionGUID)
		buf.WriteU64(s.playerGUID)
		buf.WriteU32(petitionSignAlreadySigned)
		_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
		if s.server != nil {
			if ownerSess := s.server.findSessionByGUID(uint64(ownerGUID)); ownerSess != nil {
				_ = ownerSess.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
			}
		}
		return true
	}

	_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO petition_sign (ownerguid, petitionguid, playerguid, player_account, type) VALUES (?, ?, ?, ?, ?)",
		ownerGUID, petitionGUID, s.playerGUID, s.accountID, pType)

	buf := protocol.NewBuffer(20)
	buf.WriteU64(petitionGUID)
	buf.WriteU64(s.playerGUID)
	buf.WriteU32(petitionSignOk)
	_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
	if s.server != nil {
		if ownerSess := s.server.findSessionByGUID(uint64(ownerGUID)); ownerSess != nil {
			_ = ownerSess.write(uint16(protocol.OpcodeSMSG_PETITION_SIGN_RESULTS), buf.Bytes(), true)
		}
	}
	return true
}

// handleTurnInPetition processes CMSG_TURN_IN_PETITION (0x1C4).
// Reference: WorldSession::HandleTurnInPetitionOpcode (PetitionsHandler.cpp:589).
func (s *session) handleTurnInPetition(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petitionGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var ownerGUID int64
	var guildName string
	err = cdb.QueryRowContext(ctx, "SELECT ownerguid, name FROM petition WHERE petitionguid = ? LIMIT 1", petitionGUID).Scan(&ownerGUID, &guildName)
	if err != nil || ownerGUID == 0 || guildName == "" {
		return true
	}

	// Only owner can turn in petition
	if uint64(ownerGUID) != s.playerGUID {
		return true
	}

	// Owner already in guild
	if s.player.GuildID != 0 {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(petitionTurnAlreadyInGuild)
		_ = s.write(uint16(protocol.OpcodeSMSG_TURN_IN_PETITION_RESULTS), buf.Bytes(), true)
		return true
	}

	// Guild name already exists
	var existingGuildID int64
	_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE UPPER(name) = UPPER(?) LIMIT 1", guildName).Scan(&existingGuildID)
	if existingGuildID > 0 {
		s.sendGuildCommandResult(guildCmdCreate, guildName, errGuildNameExists)
		return true
	}

	// Minimum required signatures
	minSigns := int64(9)
	if s.server != nil && s.server.Config.MinPetitionSigns > 0 {
		minSigns = int64(s.server.Config.MinPetitionSigns)
	} else if s.server != nil && s.server.Config.MinPetitionSigns == 0 {
		minSigns = 0
	}
	var signCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM petition_sign WHERE petitionguid = ?", petitionGUID).Scan(&signCount)
	if signCount < minSigns {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(petitionTurnNeedMoreSignatures)
		_ = s.write(uint16(protocol.OpcodeSMSG_TURN_IN_PETITION_RESULTS), buf.Bytes(), true)
		return true
	}

	var maxGuildID sql.NullInt64
	_ = cdb.QueryRowContext(ctx, "SELECT MAX(guildid) FROM guild").Scan(&maxGuildID)
	newGuildID := int64(1)
	if maxGuildID.Valid {
		newGuildID = maxGuildID.Int64 + 1
	}

	// Create guild
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild (guildid, name, leaderguid, createdate) VALUES (?, ?, ?, ?)",
		newGuildID, guildName, ownerGUID, timeNow())

	// Default ranks
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, 0, 'Guild Master', 4294967295, 1000000)", newGuildID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, 1, 'Officer', 255, 500000)", newGuildID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, 2, 'Veteran', 67, 100000)", newGuildID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, 3, 'Member', 67, 50000)", newGuildID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, 4, 'Initiate', 67, 0)", newGuildID)

	// Add leader
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member (guildid, guid, rank, pnote, offnote) VALUES (?, ?, 0, '', '')", newGuildID, ownerGUID)
	s.player.GuildID = uint32(newGuildID)
	s.player.GuildRank = 0

	// Add signers
	var signers []int64
	rows, _ := cdb.QueryContext(ctx, "SELECT playerguid FROM petition_sign WHERE petitionguid = ?", petitionGUID)
	if rows != nil {
		for rows.Next() {
			var signerGUID int64
			if err := rows.Scan(&signerGUID); err == nil && signerGUID != ownerGUID {
				signers = append(signers, signerGUID)
			}
		}
		rows.Close()
	}
	for _, signerGUID := range signers {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member (guildid, guid, rank, pnote, offnote) VALUES (?, ?, 4, '', '')", newGuildID, signerGUID)
		if s.server != nil {
			if signerSess := s.server.findSessionByGUID(uint64(signerGUID)); signerSess != nil && signerSess.worldReady.Load() {
				signerSess.player.GuildID = uint32(newGuildID)
				signerSess.player.GuildRank = 4
				signerSess.sendPlayerUpdate()
			}
		}
	}

	// Clean up petition and charter item
	_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", ownerGUID, petitionGUID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", petitionGUID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition WHERE petitionguid = ?", petitionGUID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE petitionguid = ?", petitionGUID)

	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.sendGuildCommandResult(guildCmdCreate, guildName, errGuildCommandSuccess)

	buf := protocol.NewBuffer(4)
	buf.WriteU32(petitionTurnOk)
	_ = s.write(uint16(protocol.OpcodeSMSG_TURN_IN_PETITION_RESULTS), buf.Bytes(), true)
	return true
}

// handleOfferPetition processes CMSG_OFFER_PETITION (0x1C3).
// Reference: WorldSession::HandleOfferPetitionOpcode (PetitionsHandler.cpp:514).
func (s *session) handleOfferPetition(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 20 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // junk
	petitionGUID, _ := r.ReadU64()
	targetGUID, _ := r.ReadU64()

	if s.server != nil {
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess == nil || !targetSess.worldReady.Load() {
			return true
		}

		// Cross-faction check
		if s.player.Race != 0 && targetSess.player.Race != 0 && teamForRace(s.player.Race) != teamForRace(targetSess.player.Race) {
			s.sendGuildCommandResult(guildCmdCreate, "", errGuildNotAllied)
			return true
		}

		// Target already in guild
		if targetSess.player.GuildID != 0 {
			s.sendGuildCommandResult(guildCmdInvite, targetSess.player.Name, errAlreadyInGuildS)
			return true
		}

		s.sendPetitionShowSignatures(targetSess, petitionGUID)
	}
	return true
}

// handlePetitionShowList processes CMSG_PETITION_SHOWLIST (0x1BB).
// Reference: WorldSession::HandlePetitionShowListOpcode (PetitionsHandler.cpp:408).
func (s *session) handlePetitionShowList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, _ := r.ReadU64()

	reqSigns := uint32(9)
	if s.server != nil && s.server.Config.MinPetitionSigns > 0 {
		reqSigns = s.server.Config.MinPetitionSigns
	} else if s.server != nil && s.server.Config.MinPetitionSigns == 0 {
		reqSigns = 0
	}

	buf := protocol.NewBuffer(24)
	buf.WriteU64(npcGUID)
	buf.WriteU8(1)                   // count = 1 petition item
	buf.WriteU32(1)                  // index = 1
	buf.WriteU32(guildCharterItemID) // 5863 Guild Charter
	buf.WriteU32(CHARTER_DISPLAY_ID) // 16161
	buf.WriteU32(guildCharterCost)   // 1000 copper
	buf.WriteU32(0)                  // 0
	buf.WriteU32(reqSigns)           // required signs
	_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SHOWLIST), buf.Bytes(), true)
	return true
}

// handlePetitionDecline processes MSG_PETITION_DECLINE (0x1C2).
// Reference: WorldSession::HandleDeclinePetition (PetitionsHandler.cpp:493).
func (s *session) handlePetitionDecline(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petitionGUID, _ := r.ReadU64()

	cdb := s.server.CharactersStore.DB
	if cdb != nil && petitionGUID > 0 {
		var ownerGUID int64
		_ = cdb.QueryRowContext(ctx, "SELECT ownerguid FROM petition WHERE petitionguid = ? LIMIT 1", petitionGUID).Scan(&ownerGUID)
		if ownerGUID > 0 && s.server != nil {
			if ownerSess := s.server.findSessionByGUID(uint64(ownerGUID)); ownerSess != nil {
				buf := protocol.NewBuffer(8)
				buf.WriteU64(s.playerGUID)
				_ = ownerSess.write(uint16(protocol.OpcodeMSG_PETITION_DECLINE), buf.Bytes(), true)
			}
		}
	}

	buf := protocol.NewBuffer(8)
	buf.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeMSG_PETITION_DECLINE), buf.Bytes(), true)
	return true
}

// handlePetitionRename processes MSG_PETITION_RENAME (0x1C1).
// Reference: WorldSession::HandlePetitionRenameGuild (PetitionsHandler.cpp:322).
func (s *session) handlePetitionRename(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	petitionGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	newName, err := r.ReadCString()
	if err != nil || newName == "" {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	if len(newName) < 2 || len(newName) > 24 {
		s.sendGuildCommandResult(guildCmdCreate, newName, errGuildNameInvalid)
		return true
	}

	var existingGuildID int64
	_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE UPPER(name) = UPPER(?) LIMIT 1", newName).Scan(&existingGuildID)
	if existingGuildID > 0 {
		s.sendGuildCommandResult(guildCmdCreate, newName, errGuildNameExists)
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE petition SET name = ? WHERE petitionguid = ? AND ownerguid = ?", newName, petitionGUID, s.playerGUID)

	buf := protocol.NewBuffer(16 + len(newName))
	buf.WriteU64(petitionGUID)
	buf.WriteCString(newName)
	_ = s.write(uint16(protocol.OpcodeMSG_PETITION_RENAME), buf.Bytes(), true)
	return true
}

// handleTabardVendorActivate processes MSG_TABARDVENDOR_ACTIVATE (0x1FA).
// Reference: WorldSession::HandleTabardVendorActivateOpcode (NPCHandler.cpp:665).
func (s *session) handleTabardVendorActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	vendorGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	buf := protocol.NewBuffer(8)
	buf.WriteU64(vendorGUID)
	_ = s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), buf.Bytes(), true)
	return true
}

// handleSaveGuildEmblem processes MSG_SAVE_GUILD_EMBLEM (0x1FB).
// Reference: WorldSession::HandleSaveGuildEmblemOpcode (GuildHandler.cpp:205).
func (s *session) handleSaveGuildEmblem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 28 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // vendor
	style, _ := r.ReadU32()
	color, _ := r.ReadU32()
	bStyle, _ := r.ReadU32()
	bColor, _ := r.ReadU32()
	bgColor, _ := r.ReadU32()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err := cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	const tabardCost = 100000 // 10 gold
	if s.player.Money < tabardCost {
		return true
	}

	s.player.Money -= tabardCost
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET EmblemStyle = ?, EmblemColor = ?, BorderStyle = ?, BorderColor = ?, BackgroundColor = ? WHERE guildid = ?",
		style, color, bStyle, bColor, bgColor, guildID)

	res := protocol.NewBuffer(4)
	res.WriteU32(0) // ERR_GUILDEMBLEM_SUCCESS
	_ = s.write(uint16(protocol.OpcodeMSG_SAVE_GUILD_EMBLEM), res.Bytes(), true)

	// GE_TABARDCHANGE = 9
	eventBuf := protocol.NewBuffer(8)
	eventBuf.WriteU8(9)
	eventBuf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), eventBuf.Bytes(), true)
	return true
}

const CHARTER_DISPLAY_ID = 16161
