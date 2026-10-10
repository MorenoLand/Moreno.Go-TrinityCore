package world

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
	guildEventPromotion        uint8  = 0
	guildEventDemotion         uint8  = 1
	guildEventJoined           uint8  = 3
	guildEventLeft             uint8  = 4
	guildEventRemoved          uint8  = 5
	guildEventLeaderChanged    uint8  = 7
	guildEventDisbanded        uint8  = 8
	guildEventRankUpdated      uint8  = 10
	guildEventRankDeleted      uint8  = 11
	guildEventSignedOn         uint8  = 12
	guildEventSignedOff        uint8  = 13
	guildEventBankTabPurchased uint8  = 15
	guildEventBankTabUpdated   uint8  = 16
	guildEventBankMoneySet     uint8  = 17
	guildEventMotd             uint8  = 2
	grRightEmpty               uint32 = 0x00000040
	guildRightSetMOTD          uint32 = 0x00001040
	guildRightModifyGuildInfo  uint32 = 0x00010040
	guildRightEPNote           uint32 = 0x00002040
	guildRightEOffNote         uint32 = 0x00008040
	guildRightViewOfficerNote  uint32 = 0x00004000
	guildRightInvite           uint32 = 0x00000050
	guildRightRemove           uint32 = 0x00000060
	guildRightPromote          uint32 = 0x000000C0
	guildRightDemote           uint32 = 0x00000140
	guildRightWithdrawRepair   uint32 = 0x00040000
	guildRightWithdrawGold     uint32 = 0x00080000
	guildRightGChatListen      uint32 = 0x00000041
	guildRightGChatSpeak       uint32 = 0x00000042
	guildRightAll              uint32 = 0x001DF1FF
	guildRanksMaxCount         int    = 10
	guildRanksMinCount         int    = 5
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

// charterTypeGuild mirrors GUILD_CHARTER_TYPE (SharedDefines.h:3790-3798).
const charterTypeGuild uint8 = 9

// petitionExecer is satisfied by *sql.DB and *sql.Tx.
type petitionExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// removeGuildCharterPetitions bridges Player::RemovePetitionsAndSigns(guid,
// GUILD_CHARTER_TYPE) (Player.cpp:21455-21459) via PetitionMgr's three
// type-gated DB arms (PetitionMgr.cpp:145-216): CHAR_DEL_PETITION_SIGNATURE
// (the player's signatures on guild charters),
// CHAR_DEL_PETITION_BY_OWNER_AND_TYPE (charters the player owns), and
// CHAR_DEL_PETITION_SIGNATURE_BY_OWNER_AND_TYPE (signatures on those owned
// charters). Errors are best-effort like the other petition bridges.
func removeGuildCharterPetitions(ctx context.Context, exec petitionExecer, guid uint64) {
	_, _ = exec.ExecContext(ctx, "DELETE FROM petition_sign WHERE playerguid = ? AND type = ?", guid, charterTypeGuild)
	_, _ = exec.ExecContext(ctx, "DELETE FROM petition WHERE ownerguid = ? AND type = ?", guid, charterTypeGuild)
	_, _ = exec.ExecContext(ctx, "DELETE FROM petition_sign WHERE ownerguid = ? AND type = ?", guid, charterTypeGuild)
}

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

// Guild emblem errors mirroring TrinityCore Guild.h:209-216 (GuildEmblemError).
const (
	guildEmblemSuccess        uint32 = 0 // ERR_GUILDEMBLEM_SUCCESS
	guildEmblemInvalidColors  uint32 = 1 // ERR_GUILDEMBLEM_INVALID_TABARD_COLORS (client-side only; never sent)
	guildEmblemNoGuild        uint32 = 2 // ERR_GUILDEMBLEM_NOGUILD
	guildEmblemNotGuildMaster uint32 = 3 // ERR_GUILDEMBLEM_NOTGUILDMASTER
	guildEmblemNotEnoughMoney uint32 = 4 // ERR_GUILDEMBLEM_NOTENOUGHMONEY
	guildEmblemInvalidVendor  uint32 = 5 // ERR_GUILDEMBLEM_INVALIDVENDOR
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

	guildBankMaxTabs                 uint8  = 6
	guildBankMaxSlots                uint8  = 98
	guildBankMoneyLogsTab            uint8  = 100
	guildBankRightFull               uint8  = 0xFF
	guildWithdrawSlotUnlimited       uint32 = 0xFFFFFFFF
	guildEquipErrItemCantStack       uint8  = 19
	guildEquipErrCantDropSoulbound   uint8  = 24
	guildEquipErrDontOwnThatItem     uint8  = 32
	guildEquipErrBankFull            uint8  = 51
	guildEquipErrItemDoesntGoIntoBag uint8  = 15
	bagFamilyMaskKeys                uint32 = 0x00000100 // BAG_FAMILY_MASK_KEYS (ItemTemplate.h:240)
	bagFamilyMaskCurrencyTokens      uint32 = 0x00002000 // BAG_FAMILY_MASK_CURRENCY_TOKENS (ItemTemplate.h:254)
)

// Guild event log types mirroring TrinityCore Guild.h:200-205.
const (
	guildEventLogInvitePlayer   uint8 = 1
	guildEventLogJoinGuild      uint8 = 2
	guildEventLogPromotePlayer  uint8 = 3
	guildEventLogDemotePlayer   uint8 = 4
	guildEventLogUninvitePlayer uint8 = 5
	guildEventLogLeaveGuild     uint8 = 6
)

// GUILD_BANK_MONEY_LIMIT mirroring TrinityCore Guild.h:60.
const guildBankMoneyLimit uint64 = 0x7FFFFFFFFFFFF

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
	_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), guildEventPayload(guildEventMotd, 0, motd), true)
	s.sendGuildBankTabsInfo(ctx)
	_ = s.handleGuildRoster(ctx)
	s.broadcastGuildMemberLogin()
}

func (s *session) ensureStartingGuild(ctx context.Context, playerGUID uint64) error {
	if s == nil || s.server == nil || !s.server.Config.StartingGuildEnable || s.server.Config.StartingGuildID == 0 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingGuild uint32
	if err := tx.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", playerGUID).Scan(&existingGuild); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var guildID uint32
	if err := tx.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE guildid = ? LIMIT 1", s.server.Config.StartingGuildID).Scan(&guildID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	var rank sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT MAX(rid) FROM guild_rank WHERE guildid = ?", guildID).Scan(&rank); err != nil {
		return err
	}
	// Guild::AddMember (Guild.cpp:2209) runs Player::RemovePetitionsAndSigns
	// before inserting the member; the starting-guild script reaches
	// AddMember on both its OnCreate and OnLogin arms.
	removeGuildCharterPetitions(ctx, tx, playerGUID)
	memberRank := uint32(4)
	if rank.Valid {
		memberRank = uint32(rank.Int64)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO guild_member (guildid, guid, rank, pnote, offnote) VALUES (?, ?, ?, '', '')", guildID, playerGUID, memberRank); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.player != nil && s.playerGUID == playerGUID {
		s.player.GuildID = guildID
		s.player.GuildRank = uint8(memberRank)
	}
	return nil
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
		if err := cdb.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = 0 AND rid = ?", guildID, rank).Scan(&rights, &limit); err != nil {
			remaining = 0
		} else {
			var withdrawn int64
			_ = cdb.QueryRowContext(ctx, "SELECT tab0 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&withdrawn)
			remaining = guildBankRemainingSlots(uint32(rank), uint32(rights), uint32(limit), uint32(withdrawn))
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

func guildBankRemainingSlots(rank, rights, slotsPerDay, withdrawn uint32) int32 {
	if rank == 0 {
		return -1
	}
	remaining := int32(slotsPerDay) - int32(withdrawn)
	if rights&1 == 0 || remaining <= 0 {
		return 0
	}
	return remaining
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

// broadcastGuildBankTabPurchased mirrors Guild::HandleBuyBankTab
// (Guild.cpp:1460): GE_BANK_TAB_PURCHASED is broadcast to every guild
// member. The packet carries no GUID for this event type
// (GuildPackets.cpp:130-140), so the payload is just the event byte.
func (s *session) broadcastGuildBankTabPurchased(guildID uint32) {
	if s == nil || s.server == nil {
		return
	}

	event := guildEventPayload(guildEventBankTabPurchased, 0)
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
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

// broadcastGuildBankMoneySet mirrors Guild::HandleMemberDepositMoney /
// HandleMemberWithdrawMoney (Guild.cpp:1719/1757): GE_BANK_MONEY_SET is
// broadcast to every guild member with the new bank money as a %016llX hex
// string, so clients refresh the bank money display.
func (s *session) broadcastGuildBankMoneySet(guildID uint32, newBankMoney int64) {
	if s == nil || s.server == nil {
		return
	}

	event := guildEventPayload(guildEventBankMoneySet, 0, fmt.Sprintf("%016X", uint64(newBankMoney)))
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
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
	return s.sendGuildQueryResponse(ctx, guildID)
}

// sendGuildQueryResponse mirrors Guild::HandleQuery (Guild.cpp:1279): the
// SMSG_GUILD_QUERY_RESPONSE with name, emblem fields, ranks and rank count.
// HandleSetEmblem calls it right after a successful emblem save (Guild.cpp:1357).
func (s *session) sendGuildQueryResponse(ctx context.Context, guildID uint32) bool {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var name string
	var emblemStyle, emblemColor, borderStyle, borderColor, bgColor int64
	err := cdb.QueryRowContext(ctx, "SELECT name, EmblemStyle, EmblemColor, BorderStyle, BorderColor, BackgroundColor FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&name, &emblemStyle, &emblemColor, &borderStyle, &borderColor, &bgColor)
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
				if memberSession := s.server.findSessionByGUID(uint64(mGuid)); memberSession != nil && memberSession.worldReady.Load() && memberSession.player != nil {
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
	// Guild::HandleInviteMember (Guild.cpp:1464): the invitee must be a known
	// (online) player, otherwise SendCommandResult(INVITE,
	// ERR_GUILD_PLAYER_NOT_FOUND_S, name).
	var targetGUID int64
	err = cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", targetName).Scan(&targetGUID)
	targetSess := s.server.findSessionByGUID(uint64(targetGUID))
	if err != nil || targetGUID == 0 || targetSess == nil || targetSess.player == nil {
		s.sendGuildCommandResult(guildCmdInvite, targetName, errGuildPlayerNotFoundS)
		return true
	}
	// Do not show invitations from ignored players (Guild.cpp:1473).
	if s.server.chatIgnoredBy(uint64(targetGUID), s.playerGUID) {
		return true
	}
	// CONFIG_ALLOW_TWO_SIDE_INTERACTION_GUILD (default false, Guild.cpp:1476).
	if !s.server.Config.AllowTwoSideInteractionGuild &&
		teamForRace(s.player.Race) != teamForRace(targetSess.player.Race) {
		s.sendGuildCommandResult(guildCmdInvite, targetName, errGuildNotAllied)
		return true
	}
	// Invited player cannot be in another guild (Guild.cpp:1481).
	if targetSess.player.GuildID != 0 {
		s.sendGuildCommandResult(guildCmdInvite, targetName, errAlreadyInGuildS)
		return true
	}
	// Invited player cannot be invited (Guild.cpp:1487).
	if targetSess.guildInvitedID != 0 {
		s.sendGuildCommandResult(guildCmdInvite, targetName, errAlreadyInvitedToGuildS)
		return true
	}
	// Inviting player must have rights to invite (Guild.cpp:1493), via the
	// exact _HasRankRight form (Guild.cpp:2388): denied iff the masked
	// rights equal GR_RIGHT_EMPTY.
	var rights uint32
	if err := cdb.QueryRowContext(ctx, `SELECT gr.rights FROM guild_member AS gm
		JOIN guild_rank AS gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&rights); err != nil || rights&guildRightInvite == grRightEmpty {
		s.sendGuildCommandResult(guildCmdInvite, "", errGuildPermissions)
		return true
	}
	// Success result is sent before the invite packet (Guild.cpp:1498).
	s.sendGuildCommandResult(guildCmdInvite, targetName, errGuildCommandSuccess)

	targetSess.guildInvitedID = uint32(guildID)
	targetSess.guildInviterGUID = s.playerGUID

	invBuf := protocol.NewBuffer(128)
	invBuf.WriteCString(s.player.Name)
	invBuf.WriteCString(guildName)
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_GUILD_INVITE), invBuf.Bytes(), true)
	// Guild::HandleInviteMember (Guild.cpp:1507):
	// _LogEvent(GUILD_EVENT_LOG_INVITE_PLAYER, inviter, invitee)
	s.logGuildEvent(ctx, uint32(guildID), guildEventLogInvitePlayer, s.playerGUID, uint64(targetGUID), 0)
	s.debug("guild invite sent", "from", s.player.Name, "to", targetName, "guild", guildName)
	return true
}

func (s *session) handleGuildAccept(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.guildInvitedID == 0 {
		return true
	}
	// WorldSession::HandleGuildAcceptOpcode (GuildHandler.cpp:60-64): the
	// handler only reaches Guild::HandleAcceptMember when the player is not
	// already in a guild — accepting while guilded is a silent no-op and the
	// pending invitation stays set.
	if s.player.GuildID != 0 {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	guildID := s.guildInvitedID
	// sGuildMgr->GetGuildById(GetPlayer()->GetGuildIdInvited()) returning null
	// (the guild was disbanded while the invitation was pending) is a silent
	// no-op; Go must not create a member row for a dead guild.
	var exists int
	if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&exists); err != nil || exists == 0 {
		return true
	}
	// Guild::HandleAcceptMember (Guild.cpp:1520-1522): the accept is a silent
	// no-op when the acceptor's team differs from the leader's team unless
	// CONFIG_ALLOW_TWO_SIDE_INTERACTION_GUILD is set; the pending invitation
	// stays set. CharacterCache::GetCharacterTeamByGuid returns 0 for an
	// unknown guid (CharacterCache.cpp:224-229) and teamForRace defaults to 0
	// the same way.
	if !s.server.Config.AllowTwoSideInteractionGuild {
		var leaderGUID int64
		var leaderRace uint8
		if err := cdb.QueryRowContext(ctx, "SELECT leaderguid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&leaderGUID); err == nil {
			_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ? LIMIT 1", leaderGUID).Scan(&leaderRace)
		}
		if teamForRace(s.player.Race) != teamForRace(leaderRace) {
			return true
		}
	}
	// Guild::AddMember (Guild.cpp:2195-2204): Player::RemovePetitionsAndSigns
	// (GUILD_CHARTER_TYPE) runs before the member row is created, so joining
	// clears the acceptor's charter signatures and owned charters.
	removeGuildCharterPetitions(ctx, cdb, s.playerGUID)
	// Guild::AddMember (Guild.cpp:2214-2216): rankId == GUILD_RANK_NONE ->
	// _GetLowestRankId() = m_ranks.size()-1. Rank ids stay sequential 0..n-1
	// (HandleAddNewRank appends, HandleRemoveLowestRank drops the tail), so
	// MAX(rid) is the lowest rank; the old hardcoded 4 assigned the wrong
	// rank in guilds that added ranks.
	lowestRank := uint32(4)
	var maxRid sql.NullInt64
	if err := cdb.QueryRowContext(ctx, "SELECT MAX(rid) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRid); err == nil && maxRid.Valid {
		lowestRank = uint32(maxRid.Int64)
	}
	s.guildInvitedID = 0
	s.guildInviterGUID = 0
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO guild_member (guildid, guid, rank, pnote, offnote) VALUES (?, ?, ?, '', '')", guildID, s.playerGUID, lowestRank)
	s.player.GuildID = guildID
	s.player.GuildRank = uint8(lowestRank)
	s.sendPlayerUpdate()

	// Guild::AddMember (Guild.cpp:2228): SendLoginInfo runs right after the
	// member is in place — MOTD event, bank tabs info, the roster reply, and
	// the SIGNED_ON broadcast — before the join log event.
	s.sendGuildLoginInfo(ctx)

	// Guild::AddMember (Guild.cpp:2268):
	// _LogEvent(GUILD_EVENT_LOG_JOIN_GUILD, lowguid)
	s.logGuildEvent(ctx, guildID, guildEventLogJoinGuild, s.playerGUID, 0, 0)

	// Guild::AddMember (Guild.cpp:2269): _BroadcastEvent(GE_JOINED, guid, name)
	// reaches every online guild member via BroadcastPacket, including the
	// newly added member — the acceptor's GuildID was set above, so the
	// sessions loop below includes it. The packet bytes are unchanged
	// ([3, 1, name, guid], guid appended for this type per
	// GuildPackets.cpp:130).
	event := guildEventPayload(guildEventJoined, s.playerGUID, s.player.Name)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()

	// ScriptMgr::OnGuildAddMember (Guild.cpp:2272): fires after the member is
	// stored and broadcast, with the new member's player and rank.
	s.fireGuildEvent(ctx, scripting.GuildEventOnAddMember, s.luaGuildObject(ctx, uint32(guildID)), s.luaPlayer(), lowestRank)
	return true
}

func (s *session) handleGuildDecline(ctx context.Context) bool {
	s.guildInvitedID = 0
	s.guildInviterGUID = 0
	if s.player != nil {
		s.player.GuildID = 0
	}
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
	var guildID, leaderGUID int64
	var guildName string
	_ = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid, g.name FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID, &guildName)
	// WorldSession::HandleGuildLeaveOpcode (GuildHandler.cpp:113): with no
	// guild the handler never calls Guild::HandleLeaveMember — silent no-op.
	if guildID == 0 {
		return true
	}
	// Guild::HandleLeaveMember (Guild.cpp:1558):
	// RemovePlayerGuildEventsAndSignups runs on both the leave and the
	// disband path.
	s.removePlayerGuildEventsAndSignups(ctx, s.playerGUID, uint32(guildID))
	// Guild::HandleLeaveMember (Guild.cpp:1529): the leader cannot leave while
	// other members remain; a lone leader disbands the guild instead of leaving.
	if uint64(leaderGUID) == s.playerGUID {
		var members int64
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_member WHERE guildid = ?", guildID).Scan(&members)
		if members > 1 {
			// SendCommandResult(session, GUILD_COMMAND_QUIT, ERR_GUILD_LEADER_LEAVE)
			s.sendGuildCommandResult(guildCmdQuit, "", errGuildLeaderLeave)
			return true
		}
		// ScriptMgr::OnGuildDisband (Guild.cpp:1144) fires before guild data is
		// removed; Disband then DeleteMember()s the lone leader with
		// isDisbanding=true, firing OnGuildRemoveMember (Guild.cpp:1148-1154,
		// 2318). The fire sites sit before execGuildDisband, like C++ firing
		// before the data removal.
		disbandGuildObj := s.luaGuildObject(ctx, uint32(guildID))
		s.fireGuildEvent(ctx, scripting.GuildEventOnDisband, disbandGuildObj)
		s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, disbandGuildObj, s.luaPlayer(), true)

		execGuildDisband(ctx, cdb, guildID)
		s.player.GuildID = 0
		s.player.GuildRank = 0
		s.sendPlayerUpdate()
		// Guild::Disband (Guild.cpp:1146) _BroadcastEvent(GE_DISBANDED,
		// ObjectGuid::Empty) reaches all online members before the member
		// rows are deleted, so the lone leader receives it; no command
		// result is sent on this path (Guild.cpp:1540). The packet bytes
		// are unchanged ([8, 0], no guid appended per GuildPackets.cpp:130).
		_ = s.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), guildEventPayload(guildEventDisbanded, 0), true)
		s.debug("guild disbanded on leader leave", "guild_id", guildID)
		return true
	}
	// ScriptMgr::OnGuildRemoveMember (Guild.cpp:2318): fires in DeleteMember
	// before the member is erased from the roster and the database.
	s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, s.luaGuildObject(ctx, uint32(guildID)), s.luaPlayer(), false)

	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ?", s.playerGUID)
	s.player.GuildID = 0
	s.player.GuildRank = 0
	s.sendPlayerUpdate()
	// Guild::HandleLeaveMember (Guild.cpp:1552):
	// _LogEvent(GUILD_EVENT_LOG_LEAVE_GUILD, player)
	s.logGuildEvent(ctx, uint32(guildID), guildEventLogLeaveGuild, s.playerGUID, 0, 0)
	// Guild::HandleLeaveMember (Guild.cpp:1553):
	// _BroadcastEvent(GE_LEFT, player->GetGUID(), player->GetName()) reaches
	// every online guild member via BroadcastPacket, not just the leaver;
	// DeleteMember (Guild.cpp:2321-2328) erases the member and clears the
	// online player's guild state BEFORE the broadcast, so the leaver never
	// receives it — the leaver's already-cleared GuildID excludes it here,
	// the same erase-before-broadcast shape. The packet bytes are unchanged
	// ([4, 1, name, guid], guid appended for this type per
	// GuildPackets.cpp:130). The leaver receives only the command result
	// below, exactly as in C++.
	event := guildEventPayload(guildEventLeft, s.playerGUID, s.player.Name)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	// SendCommandResult(session, GUILD_COMMAND_QUIT, ERR_GUILD_COMMAND_SUCCESS, m_name)
	s.sendGuildCommandResult(guildCmdQuit, guildName, errGuildCommandSuccess)
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
	var curMotd string
	var rights uint32
	if err := cdb.QueryRowContext(ctx, `SELECT gm.guildid, g.motd, gr.rights FROM guild_member AS gm
		JOIN guild AS g ON g.guildid = gm.guildid
		JOIN guild_rank AS gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &curMotd, &rights); err != nil || guildID == 0 {
		return true
	}
	// Guild::HandleSetMOTD (Guild.cpp:1301): an unchanged motd is a silent
	// no-op — no DB write, no broadcast.
	if curMotd == motd {
		return true
	}
	// The setter must hold GR_RIGHT_SETMOTD (Guild.cpp:1307), via the exact
	// _HasRankRight form (Guild.cpp:2388); denial sends EDIT_MOTD + PERMISSIONS.
	if rights&guildRightSetMOTD == grRightEmpty {
		s.sendGuildCommandResult(guildCmdEditMotd, "", errGuildPermissions)
		return true
	}
	// ScriptMgr::OnGuildMOTDChanged (Guild.cpp:1313): fires after the motd
	// changes, before the DB write and the broadcast.
	s.fireGuildEvent(ctx, scripting.GuildEventOnMotdChange, s.luaGuildObject(ctx, uint32(guildID)), motd)

	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET motd = ? WHERE guildid = ?", motd, guildID)

	// _BroadcastEvent(GE_MOTD, ObjectGuid::Empty, motd) (Guild.cpp:1320):
	// type 2 with one string param, reaching every online guild member.
	event := guildEventPayload(guildEventMotd, 0, motd)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
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
// Reference: Guild::HandleUpdateMemberRank (Guild.cpp:1595-1651, demote=false),
// WorldSession::HandleGuildPromoteOpcode (GuildHandler.cpp:95).
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

	// WorldSession::HandleGuildPromoteOpcode (GuildHandler.cpp:95-101):
	// guildless player -> HandleUpdateMemberRank never reached, silent no-op.
	var guildID, myRank int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid, rank FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &myRank)
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleUpdateMemberRank (Guild.cpp:1598-1600): the rights gate is
	// checked FIRST, via the exact _HasRankRight form (Guild.cpp:2388);
	// denial sends PROMOTE + PERMISSIONS with an empty param (Guild.h:627).
	if s.guildRankRightsMasked(ctx, guildRightPromote) == grRightEmpty {
		s.sendGuildCommandResult(guildCmdPromote, "", errGuildPermissions)
		return true
	}

	// The promoted player must be a member of the guild (Guild.cpp:1601);
	// a miss is fully silent.
	var targetGUID, targetRank int64
	var memberName string
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank, c.name FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, targetName).Scan(&targetGUID, &targetRank, &memberName)
	if err != nil || targetGUID == 0 {
		return true
	}

	// Player cannot promote himself (Guild.cpp:1604-1609).
	if uint64(targetGUID) == s.playerGUID {
		s.sendGuildCommandResult(guildCmdPromote, "", errGuildNameInvalid)
		return true
	}

	// Allow to promote only to lower rank than member's rank
	// (Guild.cpp:1630-1637: Member::IsRankNotLower(rankId+1), i.e.
	// m_rankId <= rankId+1, -> ERR_GUILD_RANK_TOO_HIGH_S with the name).
	if targetRank <= myRank+1 {
		s.sendGuildCommandResult(guildCmdPromote, memberName, errGuildRankTooHighS)
		return true
	}

	newRank := targetRank - 1
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = ? WHERE guid = ? AND guildid = ?", newRank, targetGUID, guildID)

	// Guild::Member::ChangeRank (Guild.cpp:577-588): the online target's
	// PLAYER_GUILDRANK updates alongside the DB row (player->SetRank).
	if targetSess := s.server.findSessionByGUID(uint64(targetGUID)); targetSess != nil && targetSess.player != nil {
		targetSess.player.GuildRank = uint8(newRank)
		targetSess.sendPlayerUpdate()
	}

	// Guild::HandleUpdateMemberRank (Guild.cpp:1644):
	// _LogEvent(GUILD_EVENT_LOG_PROMOTE_PLAYER, player, member, newRankId)
	s.logGuildEvent(ctx, uint32(guildID), guildEventLogPromotePlayer, s.playerGUID, uint64(targetGUID), uint8(newRank))

	// _BroadcastEvent(GE_PROMOTION, ObjectGuid::Empty, playerName, memberName,
	// rankName) (Guild.cpp:1645): 3 string params to every online guild member;
	// GuildEvent::Write appends no guid for this type (GuildPackets.cpp:130).
	var rankName string
	_ = cdb.QueryRowContext(ctx, "SELECT rname FROM guild_rank WHERE guildid = ? AND rid = ? LIMIT 1", guildID, newRank).Scan(&rankName)
	event := guildEventPayload(guildEventPromotion, 0, s.player.Name, memberName, rankName)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	return true
}

// handleGuildDemote processes CMSG_GUILD_DEMOTE (0x08C).
// Reference: Guild::HandleUpdateMemberRank (Guild.cpp:1595-1651, demote=true),
// WorldSession::HandleGuildDemoteOpcode (GuildHandler.cpp:104).
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

	// WorldSession::HandleGuildDemoteOpcode (GuildHandler.cpp:104-109):
	// guildless player -> HandleUpdateMemberRank never reached, silent no-op.
	var guildID, myRank int64
	err = cdb.QueryRowContext(ctx, "SELECT guildid, rank FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID, &myRank)
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleUpdateMemberRank (Guild.cpp:1598-1600): the rights gate is
	// checked FIRST, via the exact _HasRankRight form (Guild.cpp:2388);
	// denial sends DEMOTE + PERMISSIONS with an empty param (Guild.h:627).
	if s.guildRankRightsMasked(ctx, guildRightDemote) == grRightEmpty {
		s.sendGuildCommandResult(guildCmdDemote, "", errGuildPermissions)
		return true
	}

	// The demoted player must be a member of the guild (Guild.cpp:1601);
	// a miss is fully silent.
	var targetGUID, targetRank int64
	var memberName string
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank, c.name FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, targetName).Scan(&targetGUID, &targetRank, &memberName)
	if err != nil || targetGUID == 0 {
		return true
	}

	// Player cannot demote himself (Guild.cpp:1604-1609).
	if uint64(targetGUID) == s.playerGUID {
		s.sendGuildCommandResult(guildCmdDemote, "", errGuildNameInvalid)
		return true
	}

	// Player can demote only lower rank members
	// (Guild.cpp:1616-1622: Member::IsRankNotLower(rankId), i.e.
	// m_rankId <= rankId, -> ERR_GUILD_RANK_TOO_HIGH_S with the name).
	if targetRank <= myRank {
		s.sendGuildCommandResult(guildCmdDemote, memberName, errGuildRankTooHighS)
		return true
	}

	// Lowest rank cannot be demoted
	// (Guild.cpp:1624-1629: GetRankId() >= _GetLowestRankId() ->
	// ERR_GUILD_RANK_TOO_LOW_S with the name).
	var maxRank int64 = 4
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(rid), 4) FROM guild_rank WHERE guildid = ?", guildID).Scan(&maxRank)
	if targetRank >= maxRank {
		s.sendGuildCommandResult(guildCmdDemote, memberName, errGuildRankTooLowS)
		return true
	}

	newRank := targetRank + 1
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = ? WHERE guid = ? AND guildid = ?", newRank, targetGUID, guildID)

	// Guild::Member::ChangeRank (Guild.cpp:577-588): the online target's
	// PLAYER_GUILDRANK updates alongside the DB row (player->SetRank).
	if targetSess := s.server.findSessionByGUID(uint64(targetGUID)); targetSess != nil && targetSess.player != nil {
		targetSess.player.GuildRank = uint8(newRank)
		targetSess.sendPlayerUpdate()
	}

	// Guild::HandleUpdateMemberRank (Guild.cpp:1644):
	// _LogEvent(GUILD_EVENT_LOG_DEMOTE_PLAYER, player, member, newRankId)
	s.logGuildEvent(ctx, uint32(guildID), guildEventLogDemotePlayer, s.playerGUID, uint64(targetGUID), uint8(newRank))

	// _BroadcastEvent(GE_DEMOTION, ObjectGuid::Empty, playerName, memberName,
	// rankName) (Guild.cpp:1645): 3 string params to every online guild member;
	// GuildEvent::Write appends no guid for this type (GuildPackets.cpp:130).
	var rankName string
	_ = cdb.QueryRowContext(ctx, "SELECT rname FROM guild_rank WHERE guildid = ? AND rid = ? LIMIT 1", guildID, newRank).Scan(&rankName)
	event := guildEventPayload(guildEventDemotion, 0, s.player.Name, memberName, rankName)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	return true
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
	if err != nil || guildID == 0 {
		return true
	}
	// Guild::HandleSetLeader (Guild.cpp:1367): only the leader can assign a
	// new leader; a guilded non-leader gets GUILD_COMMAND_CHANGE_LEADER +
	// ERR_GUILD_PERMISSIONS instead of the silent return the Go arm had.
	if uint64(leaderGUID) != s.playerGUID {
		s.sendGuildCommandResult(guildCmdChangeLeader, "", errGuildPermissions)
		return true
	}

	var newLeaderGUID int64
	var newLeaderName string
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, c.name FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, newMasterName).Scan(&newLeaderGUID, &newLeaderName)
	if err != nil || newLeaderGUID == 0 || uint64(newLeaderGUID) == s.playerGUID {
		return true
	}

	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET leaderguid = ? WHERE guildid = ?", newLeaderGUID, guildID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = 1 WHERE guid = ? AND guildid = ?", s.playerGUID, guildID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member SET rank = 0 WHERE guid = ? AND guildid = ?", newLeaderGUID, guildID)

	// Guild::HandleSetLeader (Guild.cpp:1373): the old leader drops to
	// GR_OFFICER (Guild.h:72) through Member::ChangeRank, which also
	// player->SetRank's the connected player — the new leader takes rank 0.
	s.player.GuildRank = 1
	s.sendPlayerUpdate()
	if newLeaderSess := s.server.findSessionByGUID(uint64(newLeaderGUID)); newLeaderSess != nil && newLeaderSess.player != nil {
		newLeaderSess.player.GuildRank = 0
		newLeaderSess.sendPlayerUpdate()
	}

	// Guild::HandleSetLeader (Guild.cpp:1379):
	// _BroadcastEvent(GE_LEADER_CHANGED, ObjectGuid::Empty, player->GetName(),
	// pNewLeader->GetName()) reaches every online guild member via
	// BroadcastPacket, not just the actor; the packet bytes are unchanged
	// ([7, 2, oldName, newName], no guid for this type per
	// GuildPackets.cpp:130). The new leader's DB-stored name is used, as in
	// C++ (GetMember's actual name, not the client packet's).
	event := guildEventPayload(guildEventLeaderChanged, 0, s.player.Name, newLeaderName)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()

	// Guild::HandleSetLeader (Guild.cpp:1363) sends no roster — the client
	// re-requests it via CMSG_GUILD_ROSTER.
	return true
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
	// WorldSession::HandleGuildRemoveOpcode (GuildHandler.cpp:51): with no
	// guild HandleRemoveMember is never reached — silent no-op.
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleRemoveMember (Guild.cpp:1564): the remover must hold
	// GR_RIGHT_REMOVE (Guild.h:88, 0x60), checked before the member lookup;
	// denial sends GUILD_COMMAND_REMOVE + ERR_GUILD_PERMISSIONS, no name.
	if s.guildRankRightsMasked(ctx, guildRightRemove) == grRightEmpty {
		s.sendGuildCommandResult(guildCmdRemove, "", errGuildPermissions)
		return true
	}

	var targetGUID, targetRank int64
	err = cdb.QueryRowContext(ctx, `SELECT gm.guid, gm.rank FROM guild_member gm
		JOIN characters c ON c.guid = gm.guid
		WHERE gm.guildid = ? AND UPPER(c.name) = UPPER(?) LIMIT 1`, guildID, removee).Scan(&targetGUID, &targetRank)
	// GetMember miss is silent — no command result (Guild.cpp:1567).
	if err != nil || targetGUID == 0 {
		return true
	}

	// Guild masters cannot be removed (Guild.cpp:1570).
	if targetRank == 0 {
		s.sendGuildCommandResult(guildCmdRemove, "", errGuildLeaderLeave)
		return true
	}

	// Do not allow removing a player of the same rank or higher
	// (Member::IsRankNotLower, Guild.h:324) — memberMe null also fails
	// (Guild.cpp:1578); both send ERR_GUILD_RANK_TOO_HIGH_S with the name.
	if targetRank <= myRank {
		s.sendGuildCommandResult(guildCmdRemove, removee, errGuildRankTooHighS)
		return true
	}

	// ScriptMgr::OnGuildRemoveMember (Guild.cpp:2318): fires in DeleteMember
	// before the member is erased from the roster and the database; the
	// removed player may be offline (Eluna pushes nil).
	var targetPlayer *scripting.Object
	if targetSess := s.server.findSessionByGUID(uint64(targetGUID)); targetSess != nil {
		targetPlayer = targetSess.luaPlayer()
	}
	s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, s.luaGuildObject(ctx, uint32(guildID)), targetPlayer, false)

	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guid = ? AND guildid = ?", targetGUID, guildID)

	// Guild::HandleRemoveMember (Guild.cpp:1582-1584): DeleteMember runs
	// first — the member is erased from the roster and, when the removed
	// player is online, player->SetInGuild(0) + player->SetRank(0). Clear
	// the removed session's guild state before the broadcast so it is
	// excluded from it, the same way C++ erases the member before
	// _BroadcastEvent (Guild.cpp:1589, BroadcastPacket iterates only the
	// remaining members).
	if targetSess := s.server.findSessionByGUID(uint64(targetGUID)); targetSess != nil &&
		targetSess.player != nil && targetSess.player.GuildID == uint32(guildID) {
		targetSess.player.GuildID = 0
		targetSess.player.GuildRank = 0
		targetSess.sendPlayerUpdate()
	}

	// Guild::HandleRemoveMember (Guild.cpp:1587):
	// _LogEvent(GUILD_EVENT_LOG_UNINVITE_PLAYER, player, member)
	s.logGuildEvent(ctx, uint32(guildID), guildEventLogUninvitePlayer, s.playerGUID, uint64(targetGUID), 0)

	// Guild::HandleRemoveMember (Guild.cpp:1589):
	// _BroadcastEvent(GE_REMOVED, ObjectGuid::Empty, name, player->GetName())
	// reaches every online guild member via BroadcastPacket, not just the
	// remover; GuildEvent::Write appends no guid for this type
	// (GuildPackets.cpp:130).
	event := guildEventPayload(guildEventRemoved, 0, removee, s.player.Name)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()

	// Guild::HandleRemoveMember (Guild.cpp:1564) sends no roster — the
	// client re-requests it via CMSG_GUILD_ROSTER.
	return true
}

// execGuildDisband deletes every row belonging to a guild, mirroring the
// table deletions in Guild::Disband (Guild.cpp:1139).
func execGuildDisband(ctx context.Context, cdb *sql.DB, guildID int64) {
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_member WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_rank WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_tab WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_item WHERE guildid = ?", guildID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_eventlog WHERE guildid = ?", guildID)
	// Guild::Disband (Guild.cpp:1183): CHAR_DEL_GUILD_EVENTLOGS
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_eventlog WHERE guildid = ?", guildID)
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

	// ScriptMgr::OnGuildDisband (Guild.cpp:1144) fires before guild data is
	// removed; Disband then DeleteMember()s every member with
	// isDisbanding=true, firing OnGuildRemoveMember per member
	// (Guild.cpp:1148-1154, 2318) — offline members reach the hook as nil,
	// like Eluna's Push of a null Player*. The fire sites sit before the row
	// deletions, like C++ firing before the data removal.
	disbandGuild := s.luaGuildObject(ctx, uint32(guildID))
	s.fireGuildEvent(ctx, scripting.GuildEventOnDisband, disbandGuild)
	var disbandMembers []int64
	if rows, err := cdb.QueryContext(ctx, "SELECT guid FROM guild_member WHERE guildid = ?", guildID); err == nil && rows != nil {
		for rows.Next() {
			var memberGUID int64
			if err := rows.Scan(&memberGUID); err == nil {
				disbandMembers = append(disbandMembers, memberGUID)
			}
		}
		rows.Close()
	}
	for _, memberGUID := range disbandMembers {
		var memberPlayer *scripting.Object
		if memberSess := s.server.findSessionByGUID(uint64(memberGUID)); memberSess != nil {
			memberPlayer = memberSess.luaPlayer()
		}
		s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, disbandGuild, memberPlayer, true)
	}

	// Guild::Disband (Guild.cpp:1141-1190) via the shared GM helper: GE_DISBANDED
	// reaches every online member first, the rows are deleted, then every
	// online member's session guild state is cleared. The lone-leader arm in
	// handleGuildLeave is the only disband path where the actor must be
	// excluded.
	s.disbandGuildByID(ctx, uint32(guildID))

	s.debug("guild disbanded", "guild_id", guildID)
	return true
}

// handleGuildAddRank processes CMSG_GUILD_ADD_RANK (0x232).
// Reference: WorldSession::HandleGuildAddRankOpcode (GuildHandler.cpp:181),
// Guild::HandleAddNewRank (Guild.cpp:1649-1658).
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
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleAddNewRank (Guild.cpp:1651-1654): size >=
	// GUILD_RANKS_MAX_COUNT (10) -> silent return; only the leader can add
	// a rank, and a non-leader is fully silent (no command result).
	var rankCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_rank WHERE guildid = ?", guildID).Scan(&rankCount)
	if rankCount >= int64(guildRanksMaxCount) || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	// Guild::_CreateRank (Guild.cpp:2447-2468): newRankId = _GetRanksSize();
	// the default rights are GR_RIGHT_GCHATLISTEN | GR_RIGHT_GCHATSPEAK
	// (0x43), and CreateMissingTabsIfNeeded inserts empty bank-right rows
	// for the new rank on every purchased tab (Guild.cpp:276-297).
	newRid := rankCount
	_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_rank (guildid, rid, rname, rights, BankMoneyPerDay) VALUES (?, ?, ?, ?, 0)",
		guildID, newRid, rankName, guildRightGChatListen|guildRightGChatSpeak)
	var purchasedTabs int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs)
	for tab := int64(0); tab < purchasedTabs; tab++ {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_bank_right (guildid, TabId, rid, gbright, SlotPerDay) VALUES (?, ?, ?, 0, 0)",
			guildID, tab, newRid)
	}

	// _BroadcastEvent(GE_RANK_UPDATED, ObjectGuid::Empty, newRankId, name,
	// newSize) (Guild.cpp:1657): 3 string params to every online guild
	// member; C++ sends no roster here.
	event := guildEventPayload(guildEventRankUpdated, 0, fmt.Sprintf("%d", newRid), rankName, fmt.Sprintf("%d", newRid+1))
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	return true
}

// handleGuildDelRank processes CMSG_GUILD_DEL_RANK (0x233).
// Reference: WorldSession::HandleGuildDeleteRank (GuildHandler.cpp:189),
// Guild::HandleRemoveLowestRank (Guild.cpp:1660-1663) and
// Guild::HandleRemoveRank (Guild.cpp:1669-1691).
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
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleRemoveRank (Guild.cpp:1671-1673): cannot remove a rank if
	// the total count is at the client minimum (GUILD_RANKS_MIN_COUNT = 5),
	// or if the actor is not the leader — all fully silent.
	var rankCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_rank WHERE guildid = ?", guildID).Scan(&rankCount)
	if rankCount <= int64(guildRanksMinCount) || uint64(leaderGUID) != s.playerGUID {
		return true
	}

	// CHAR_DEL_GUILD_BANK_RIGHTS_FOR_RANK ("DELETE FROM guild_bank_right
	// WHERE guildid = ? AND rid = ?") + CHAR_DEL_GUILD_LOWEST_RANK
	// ("DELETE FROM guild_rank WHERE guildid = ? AND rid >= ?",
	// CharacterDatabase.cpp:179/191). Members are NOT reassigned — the
	// client blocks deleting a rank that still has members.
	lowestRank := rankCount - 1
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_bank_right WHERE guildid = ? AND rid = ?", guildID, lowestRank)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_rank WHERE guildid = ? AND rid >= ?", guildID, lowestRank)

	// _BroadcastEvent(GE_RANK_DELETED, ObjectGuid::Empty, newSize)
	// (Guild.cpp:1690): 1 string param to every online guild member; C++
	// sends no roster here.
	event := guildEventPayload(guildEventRankDeleted, 0, fmt.Sprintf("%d", lowestRank))
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	return true
}

// handleGuildRank processes CMSG_GUILD_RANK (0x231).
// Reference: WorldSession::HandleGuildSetRankPermissions (GuildHandler.cpp:166),
// Guild::HandleSetRankInfo (Guild.cpp:1414-1431), GuildSetRankPermissions::Read
// (GuildPackets.cpp:181-193: RankID u32, Flags u32, RankName cstr,
// WithdrawGoldLimit u32, then GUILD_BANK_MAX_TABS x (TabFlags u32,
// TabWithdrawItemLimit u32) — the flags are dwords on the wire, truncated
// to uint8 by the handler (GuildHandler.cpp:172:
// uint8(packet.TabFlags[tabId]))).
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
	tabFlags := make([]uint32, guildBankMaxTabs)
	tabLimits := make([]uint32, guildBankMaxTabs)
	for i := range tabFlags {
		f, ferr := r.ReadU32()
		if ferr != nil {
			return false
		}
		tabFlags[i] = f
		l, lerr := r.ReadU32()
		if lerr != nil {
			return false
		}
		tabLimits[i] = l
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var guildID, leaderGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleSetRankInfo (Guild.cpp:1416-1417): only the leader can
	// modify ranks; denial sends GUILD_COMMAND_CHANGE_RANK (16) +
	// ERR_GUILD_PERMISSIONS — unlike add/remove, this one is NOT silent.
	if uint64(leaderGUID) != s.playerGUID {
		s.sendGuildCommandResult(guildCmdChangeRank, "", errGuildPermissions)
		return true
	}

	// RankInfo::SetRights (Guild.cpp:315-318): rank 0 (guildmaster) keeps
	// GR_RIGHT_ALL no matter what the packet carries.
	if rankID == 0 {
		rights = guildRightAll
	}
	res, execErr := cdb.ExecContext(ctx, "UPDATE guild_rank SET rname = ?, rights = ?, BankMoneyPerDay = ? WHERE guildid = ? AND rid = ?",
		rankName, rights, goldLimit, guildID, rankID)
	if execErr != nil {
		return true
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return true // GetRankInfo miss -> silent (Guild.cpp:1418)
	}

	// Guild::_SetRankBankTabRightsAndSlots (Guild.cpp:2541-2548): only tabs
	// below the purchased count are written; CHAR_INS_GUILD_BANK_RIGHT is an
	// upsert; rank 0 forces the guildmaster values (Guild.cpp:351-352).
	var purchasedTabs int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs)
	for tab := int64(0); tab < purchasedTabs && tab < int64(guildBankMaxTabs); tab++ {
		// GuildHandler.cpp:172 truncates the packet's u32 flag to uint8
		// before GuildBankRightsAndSlots stores it (CHAR_INS_GUILD_BANK_RIGHT
		// writes it as int8).
		gbright := uint32(uint8(tabFlags[tab]))
		slots := tabLimits[tab]
		if rankID == 0 {
			gbright = uint32(guildBankRightFull)
			slots = guildWithdrawSlotUnlimited
		}
		_, _ = cdb.ExecContext(ctx, `INSERT INTO guild_bank_right (guildid, TabId, rid, gbright, SlotPerDay) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(guildid, TabId, rid) DO UPDATE SET gbright = excluded.gbright, SlotPerDay = excluded.SlotPerDay`,
			guildID, tab, rankID, gbright, slots)
	}

	// _BroadcastEvent(GE_RANK_UPDATED, ObjectGuid::Empty, rankId, name, size)
	// (Guild.cpp:1428): 3 string params to every online guild member; C++
	// sends no roster here.
	var rankCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_rank WHERE guildid = ?", guildID).Scan(&rankCount)
	event := guildEventPayload(guildEventRankUpdated, 0, fmt.Sprintf("%d", rankID), rankName, fmt.Sprintf("%d", rankCount))
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()
	return true
}

// handleGuildSetPublicNote processes CMSG_GUILD_SET_PUBLIC_NOTE (0x234).
// Reference: WorldSession::HandleGuildSetPublicNoteOpcode (GuildHandler.cpp:146).
// guildRankRightsMasked returns the setter's guild-rank rights masked with
// right, via the exact Guild::_HasRankRight form (Guild.cpp:2388: denied
// iff (rights & right) == GR_RIGHT_EMPTY). A missing rank row masks to
// GR_RIGHT_EMPTY (denied), matching the GetMember-null arm of _HasRankRight
// callers.
func (s *session) guildRankRightsMasked(ctx context.Context, right uint32) uint32 {
	var rights uint32
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return grRightEmpty
	}
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT gr.rights FROM guild_member AS gm
		JOIN guild_rank AS gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&rights); err != nil {
		return grRightEmpty
	}
	return rights & right
}

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

	// Guild::HandleSetMemberNote (Guild.cpp:1398): the setter must hold
	// GR_RIGHT_EPNOTE / GR_RIGHT_EOFFNOTE (Guild.h:92/94) — rights are
	// checked before the member lookup, via the exact _HasRankRight form
	// (Guild.cpp:2388); denial sends GUILD_COMMAND_PUBLIC_NOTE + PERMISSIONS
	// (Guild.cpp:1401), even for officer notes.
	// Guild.cpp:1401: public note = officer=false -> GR_RIGHT_EPNOTE.
	if s.guildRankRightsMasked(ctx, guildRightEPNote) == grRightEmpty {
		s.sendGuildCommandResult(guildCmdPublicNote, "", errGuildPermissions)
		return true
	}

	// Guild.cpp:1403: else if (Member* member = GetMember(name)) — the note
	// is set only when the target is a guild member; C++ then re-sends the
	// roster to the setter (HandleRoster). The rows-affected check mirrors
	// the GetMember null gate.
	res, _ := cdb.ExecContext(ctx, `UPDATE guild_member SET pnote = ?
		WHERE guildid = ? AND guid = (SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1)`, note, guildID, targetName)
	if n, _ := res.RowsAffected(); n > 0 {
		return s.handleGuildRoster(ctx)
	}
	return true
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

	// Guild.cpp:1398: officer=true -> GR_RIGHT_EOFFNOTE; denial carries
	// GUILD_COMMAND_PUBLIC_NOTE (Guild.cpp:1401), not a separate command.
	if s.guildRankRightsMasked(ctx, guildRightEOffNote) == grRightEmpty {
		s.sendGuildCommandResult(guildCmdPublicNote, "", errGuildPermissions)
		return true
	}

	res, _ := cdb.ExecContext(ctx, `UPDATE guild_member SET offnote = ?
		WHERE guildid = ? AND guid = (SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1)`, note, guildID, targetName)
	if n, _ := res.RowsAffected(); n > 0 {
		return s.handleGuildRoster(ctx)
	}
	return true
}

// handleGuildInfoText processes CMSG_GUILD_INFO_TEXT (0x2FC).
// Reference: WorldSession::HandleGuildUpdateInfoText (GuildHandler.cpp:197) ->
// Guild::HandleSetInfo (Guild.cpp:1324).
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
	var curInfo string
	var rights uint32
	if err := cdb.QueryRowContext(ctx, `SELECT gm.guildid, g.info, gr.rights FROM guild_member AS gm
		JOIN guild AS g ON g.guildid = gm.guildid
		JOIN guild_rank AS gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &curInfo, &rights); err != nil || guildID == 0 {
		return true
	}

	// Guild::HandleSetInfo (Guild.cpp:1326): unchanged info is a silent
	// no-op — no DB write.
	if curInfo == infoText {
		return true
	}
	// The setter must hold GR_RIGHT_MODIFY_GUILD_INFO (Guild.cpp:1330), via
	// the exact _HasRankRight form (Guild.cpp:2388); unlike the MOTD arm,
	// denial is fully silent — HandleSetInfo sends no command result.
	if rights&guildRightModifyGuildInfo == grRightEmpty {
		return true
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET info = ? WHERE guildid = ?", infoText, guildID)

	// ScriptMgr::OnGuildInfoChanged (Guild.cpp:1334): fires after the info
	// changes and before the DB save — the UPDATE above is the save, and the
	// fired string is the same either way.
	s.fireGuildEvent(ctx, scripting.GuildEventOnInfoChange, s.luaGuildObject(ctx, uint32(guildID)), infoText)
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
// Reference: WorldSession::HandleGuildEventLogQueryOpcode (GuildHandler.cpp:230:
// no guild -> no packet) -> Guild::SendEventLog (Guild.cpp:1803) ->
// EventLogEntry::WritePacket (Guild.cpp:187).
func (s *session) handleGuildEventLogQuery(ctx context.Context, payload []byte) bool {
	cdb := s.server.CharactersStore.DB
	var guildID int64
	if s.player != nil {
		guildID = int64(s.player.GuildID)
	}
	if cdb == nil || guildID == 0 {
		return true
	}

	// Guild::SendEventLog iterates the in-memory LogHolder, which is ordered
	// oldest-first (Guild.h: "The first element is the oldest entry": DB rows
	// are loaded ORDER BY TimeStamp DESC, LogGuid DESC then emplace_front'd,
	// and new events are emplace_back'd), so the packet lists oldest entries
	// first. LIMIT 100 mirrors GUILD_EVENTLOG_MAX_RECORDS
	// (SharedDefines.h:3248), the in-memory cap per guild.
	rows, err := cdb.QueryContext(ctx, "SELECT EventType, PlayerGuid1, PlayerGuid2, NewRank, TimeStamp FROM guild_eventlog WHERE guildid = ? ORDER BY TimeStamp ASC, LogGuid ASC LIMIT 100", guildID)
	if err != nil {
		return true
	}

	type eventLogRecord struct {
		eventType       uint8
		playerGUID      uint64
		otherGUID       uint64
		rankID          uint8
		transactionDate uint32
	}
	var entries []eventLogRecord
	now := uint32(time.Now().Unix())

	// Player GUIDs are stored as counters (ObjectGuid::LowType); writing the
	// counter as the raw u64 is exact since player high bits are 0x0000.
	for rows.Next() {
		var et uint8
		var pGuid1, pGuid2 uint32
		var rankID uint8
		var ts uint64
		if err := rows.Scan(&et, &pGuid1, &pGuid2, &rankID, &ts); err == nil {
			entries = append(entries, eventLogRecord{
				eventType:       et,
				playerGUID:      uint64(pGuid1),
				otherGUID:       uint64(pGuid2),
				rankID:          rankID,
				transactionDate: now - uint32(ts),
			})
		}
	}
	rows.Close()

	buf := protocol.NewBuffer(1 + len(entries)*23)
	buf.WriteU8(uint8(len(entries)))

	// GuildEventLogQueryResults::Write (GuildPackets.cpp:145): per entry
	// TransactionType u8, PlayerGUID u64, OtherGUID u64 only when the type is
	// not JOIN_GUILD or LEAVE_GUILD, RankID u8 only for PROMOTE/DEMOTE, then
	// TransactionDate u32 as the plain now - ts wrap.
	for _, e := range entries {
		buf.WriteU8(e.eventType)
		buf.WriteU64(e.playerGUID)
		if e.eventType != guildEventLogJoinGuild && e.eventType != guildEventLogLeaveGuild {
			buf.WriteU64(e.otherGUID)
		}
		if e.eventType == guildEventLogPromotePlayer || e.eventType == guildEventLogDemotePlayer {
			buf.WriteU8(e.rankID)
		}
		buf.WriteU32(e.transactionDate)
	}

	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_EVENT_LOG_QUERY), buf.Bytes(), true)
	return true
}

// handleGuildPermissions processes MSG_GUILD_PERMISSIONS (0x3FD).
// Reference: WorldSession::HandleGuildPermissionsQuery (GuildHandler.cpp:244).
func (s *session) handleGuildPermissions(ctx context.Context, payload []byte) bool {
	s.sendGuildPermissions(ctx)
	return true
}

// sendGuildPermissions mirrors Guild::SendPermissions (Guild.cpp:1854). It
// is also re-sent after a bank tab purchase (Guild.cpp:1461) to force the
// client to update permissions.
func (s *session) sendGuildPermissions(ctx context.Context) {
	if !s.playerLoaded || s.player == nil {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	// C++ GetMember miss -> silent return, no packet (Guild.cpp:1856).
	var guildID, rankID, rights, goldLimit int64
	if err := cdb.QueryRowContext(ctx, `SELECT gm.guildid, gr.rid, gr.rights, gr.BankMoneyPerDay
		FROM guild_member gm
		JOIN guild_rank gr ON gr.guildid = gm.guildid AND gr.rid = gm.rank
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &rankID, &rights, &goldLimit); err != nil {
		return
	}
	var numTabs int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&numTabs)

	buf := protocol.NewBuffer(20 + int(guildBankMaxTabs)*8)
	buf.WriteU32(uint32(rankID))
	buf.WriteU32(uint32(rights))
	buf.WriteU32(uint32(goldLimit))
	buf.WriteU8(uint8(numTabs))
	for tab := uint8(0); tab < guildBankMaxTabs; tab++ {
		// C++ _GetRankBankTabRights (Guild.cpp:2579): rights for all six tabs,
		// missing row -> 0; C++ _GetMemberRemainingSlots (Guild.cpp:2586):
		// guildmaster -> unlimited, no view right -> 0, else
		// slotsPerDay - withdrawn clamped at 0 (unbought tabs give 0).
		var tabFlags int64
		_ = cdb.QueryRowContext(ctx, `SELECT gbright FROM guild_bank_right
			WHERE guildid = ? AND TabId = ? AND rid = ? LIMIT 1`, guildID, tab, rankID).Scan(&tabFlags)
		var withdrawItemLimit int32
		if rankID == 0 || tab < uint8(numTabs) {
			withdrawItemLimit = s.guildBankWithdrawalsRemaining(ctx, guildID, tab)
		}
		buf.WriteI32(int32(tabFlags))
		buf.WriteI32(withdrawItemLimit)
	}
	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_PERMISSIONS), buf.Bytes(), true)
}

// handleInspectArenaTeams processes MSG_INSPECT_ARENA_TEAMS (0x377).
// Reference: WorldSession::HandleInspectArenaTeamsOpcode (ArenaTeamHandler.cpp:33)
// and ArenaTeam::Inspect (ArenaTeam.cpp:504).
func (s *session) handleInspectArenaTeams(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	// ObjectAccessor::FindPlayer - target must be online (ArenaTeamHandler.cpp:44-46)
	targetSession := s.server.findSessionByGUID(targetGUID)
	if targetSession == nil || !targetSession.worldReady.Load() || targetSession.player == nil {
		return true
	}
	target := targetSession.player

	// IsWithinDistInMap(player, INSPECT_DISTANCE = 28.0) (ArenaTeamHandler.cpp:48)
	if target.Map != s.player.Map {
		return true
	}
	if !inspectWithinDistance(s.player, target) {
		return true
	}

	// IsValidAttackTarget arm (ArenaTeamHandler.cpp:51)
	if inspectBlockedByHostility(s.player, target) {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	// For each arena team the target is a member of, ArenaTeam::Inspect sends one
	// packet (ArenaTeam.cpp:511-520); the membership join encodes the GetMember
	// miss arm, which silently skips the team.
	rows, err := cdb.QueryContext(ctx, "SELECT t.arenaTeamId, t.type, t.rating, t.seasonGames, t.seasonWins, m.seasonGames, m.personalRating FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ?", targetGUID)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var teamID, aType, rating, tSeasonGames, tSeasonWins, mSeasonGames, mPersonalRating uint32
		if rows.Scan(&teamID, &aType, &rating, &tSeasonGames, &tSeasonWins, &mSeasonGames, &mPersonalRating) != nil {
			continue
		}
		buf := protocol.NewBuffer(8 + 1 + 4*6)
		buf.WriteU64(targetGUID)                       // player guid
		buf.WriteU8(uint8(arenaTeamSlotByType(aType))) // slot (0...2)
		buf.WriteU32(teamID)                           // arena team id
		buf.WriteU32(rating)                           // rating
		buf.WriteU32(tSeasonGames)                     // season played
		buf.WriteU32(tSeasonWins)                      // season wins
		buf.WriteU32(mSeasonGames)                     // played (member's games)
		buf.WriteU32(mPersonalRating)                  // personal rating
		_ = s.write(uint16(protocol.OpcodeMSG_INSPECT_ARENA_TEAMS), buf.Bytes(), true)
	}
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

	// !player + IsWithinDistInMap(INSPECT_DISTANCE) + IsValidAttackTarget
	// gates (MiscHandler.cpp:1044-1051); C++ answers nothing when any fails.
	targetSession := s.server.findSessionByGUID(targetGUID)
	if targetSession == nil || !targetSession.worldReady.Load() || targetSession.player == nil {
		return true
	}
	target := targetSession.player
	if target.Map != s.player.Map {
		return true
	}
	if !inspectWithinDistance(s.player, target) {
		return true
	}
	if inspectBlockedByHostility(s.player, target) {
		return true
	}

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
// Reference: WorldSession::HandlePVPLogDataOpcode (BattleGroundHandler.cpp:306)
// — both gates are silent: Battleground* bg = _player->GetBattleground();
// if (!bg) return; then "Prevent players from sending BuildPvpLogDataPacket
// in an arena except for when sent in BattleGround::EndBattleGround":
// if (bg->isArena()) return.
// Go has no live non-arena Battleground instance model, so no session maps
// to the in-BG arm and the null-BG gate covers every non-arena session;
// arena sessions map to the isArena gate. The arena packet itself is pushed
// server-side by endArena (== Battleground::EndBattleGround's
// BuildPvPLogDataPacket arm), and buildArenaPvPLogDataPacket matches the
// C++ arena layout slot-for-slot (type 1, per-team rating blocks green
// first, per-team name blocks, ended+winner, count, then per-score
// guid/killingblows/teamid/damage/healing/zero-objectives).
func (s *session) handlePvpLogData(ctx context.Context, payload []byte) bool {
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

	// Reference: WorldSession::HandleGuildBankActivate (GuildHandler.cpp:
	// 256-258): no interactable guild-bank gameobject -> silent return,
	// before the guild check.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
		return true
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
	// The FullUpdate flag exists in the client packet (GuildPackets.h:415)
	// but is deliberately ignored below: WorldSession::HandleGuildBankQueryTab
	// (GuildHandler.cpp:276) passes `true` to SendBankTabData unconditionally
	// (the "HACK" comment there — the client doesn't query the full tab
	// content if it already received a bank list in this session).
	if len(payload) >= 10 {
		if _, err = r.ReadU8(); err != nil {
			return false
		}
	}

	// Reference: WorldSession::HandleGuildBankQueryTab (GuildHandler.cpp:276):
	// tab data is sent only when the banker gameobject is interactable.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
		return true
	}

	// Reference: Guild::SendBankTabData (Guild.cpp:1837-1841) and
	// Guild::_SendBankContent (Guild.cpp:2780-2786): an unpurchased tab id
	// is silently ignored, and the tab content is sent only to members
	// holding GUILD_BANK_RIGHT_VIEW_TAB (0x01) on the tab. A player with no
	// guild gets nothing at all (WorldSession::HandleGuildBankQueryTab,
	// GuildHandler.cpp:276-282, only calls into the guild object when
	// GetGuild() is non-null); DB errors fail closed the same way.
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	var guildID int64
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID); err != nil || guildID == 0 {
		return true
	}
	var purchasedTabs int64
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs); err != nil {
		return true
	}
	if int64(tabID) >= purchasedTabs || !s.checkGuildBankRights(ctx, uint32(guildID), tabID, false) {
		return true
	}

	return s.sendGuildBankList(ctx, bankerGUID, tabID, true)
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

	payload, ok := buildGuildBankListPayload(ctx, cdb, uint32(guildID), bankMoney, tabID, fullUpdate,
		s.guildBankWithdrawalsRemaining(ctx, guildID, tabID), nil)
	if !ok {
		return true
	}
	return s.write(uint16(protocol.OpcodeSMSG_GUILD_BANK_LIST), payload, true) == nil
}

// buildGuildBankListPayload assembles the SMSG_GUILD_BANK_LIST body shared by
// the session-targeted sender and the guild-wide broadcast; the caller
// supplies the recipient-specific WithdrawalsRemaining. Guild::_SendBankList
// (Guild.cpp:2844-2904): on a full update the list carries every occupied
// slot (empties skipped); on a partial update (sendAllSlots=false) it
// carries exactly the given slots, emptied ones as ItemID=0, and nothing
// at all when no slots are given.
func buildGuildBankListPayload(ctx context.Context, cdb *sql.DB, guildID uint32, bankMoney int64, tabID uint8, fullUpdate bool, withdrawalsRemaining int32, slots []uint8) ([]byte, bool) {
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
	buf.WriteI32(withdrawalsRemaining)
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

	itemBySlot := make(map[uint8]guildBankSlotItem, len(items))
	for _, it := range items {
		itemBySlot[it.Slot] = it
	}
	// The per-item trailer matches GuildBankItemInfo (GuildPackets.cpp):
	// Flags, RandomPropertiesID, Count, EnchantmentID, Charges and the
	// socket-enchant list are only present when ItemID != 0; an emptied
	// slot is sent as Slot + ItemID=0 only.
	writeBankItem := func(slot uint8, it guildBankSlotItem) {
		buf.WriteU8(slot)
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
	if fullUpdate {
		buf.WriteU8(uint8(len(items)))
		for _, it := range items {
			writeBankItem(it.Slot, it)
		}
	} else if slots != nil {
		buf.WriteU8(uint8(len(slots)))
		for _, slot := range slots {
			writeBankItem(slot, itemBySlot[slot])
		}
	} else {
		buf.WriteU8(0)
	}

	return buf.Bytes(), true
}

// broadcastGuildBankList mirrors Guild::_SendBankList with a null session
// (Guild.cpp:2826, 2922-2941): the post-move content update goes to every
// online guild member holding GUILD_BANK_RIGHT_VIEW_TAB on the tab. The
// packet is built once and only the per-member WithdrawalsRemaining field
// (bytes 9-12: after the 8-byte money and 1-byte tab) is rewritten per
// recipient, as in C++. The list is partial (fullUpdate=false): it carries
// exactly the given changed slots, emptied ones as ItemID=0
// (Guild::_SendBankContentUpdate, Guild.cpp:2794-2826).
func (s *session) broadcastGuildBankList(ctx context.Context, guildID uint32, tabID uint8, fullUpdate bool, slots []uint8) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || guildID == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB
	var bankMoney int64
	if err := cdb.QueryRowContext(ctx, "SELECT BankMoney FROM guild WHERE guildid = ?", guildID).Scan(&bankMoney); err != nil {
		return
	}
	s.server.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.server.sessions))
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
			continue
		}
		targets = append(targets, target)
	}
	s.server.sessionsMu.RUnlock()
	var payload []byte
	for _, target := range targets {
		if !guildBankHasTabView(ctx, cdb, guildID, target.playerGUID, tabID) {
			continue
		}
		if payload == nil {
			p, ok := buildGuildBankListPayload(ctx, cdb, guildID, bankMoney, tabID, fullUpdate, 0, slots)
			if !ok {
				return
			}
			payload = p
		}
		frame := make([]byte, len(payload))
		copy(frame, payload)
		binary.LittleEndian.PutUint32(frame[9:13], uint32(guildBankWithdrawalsRemainingFor(ctx, cdb, int64(guildID), tabID, target.playerGUID)))
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_BANK_LIST), frame, true)
	}
}

// guildBankHasTabView mirrors the GUILD_BANK_RIGHT_VIEW_TAB (0x01) term of
// Guild::_MemberHasTabRights (Guild.cpp:2626-2636); the guildmaster (rank 0)
// holds all rights.
func guildBankHasTabView(ctx context.Context, q guildMoveQueryer, guildID uint32, playerGUID uint64, tabID uint8) bool {
	var rank int64
	if err := q.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", playerGUID, guildID).Scan(&rank); err != nil {
		return false
	}
	if rank == 0 {
		return true
	}
	var gbright int64
	if err := q.QueryRowContext(ctx, "SELECT gbright FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright); err != nil {
		return false
	}
	return gbright&0x01 != 0
}

func (s *session) logGuildBankEvent(ctx context.Context, guildID uint32, tabID uint8, eventType uint8, playerGUID uint64, itemOrMoney uint32, stackCount uint32, destTabID uint8) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || guildID == 0 {
		return
	}
	// Guild::_LogBankEvent (Guild.cpp:2656-2661): tab ids past the bank are
	// dropped, and moves within the same tab are not logged.
	if tabID > guildBankMaxTabs {
		return
	}
	if eventType == guildBankLogMoveItem && tabID == destTabID {
		return
	}
	cdb := s.server.CharactersStore.DB

	// Money events are stored under the money-logs tab, but the hook sees
	// GUILD_BANK_MAX_TABS (Guild.cpp:2663-2673, 2683).
	hookTabID := tabID
	dbTabID := tabID
	if eventType == guildBankLogDepositMoney || eventType == guildBankLogWithdrawMoney || eventType == guildBankLogRepairMoney {
		hookTabID = guildBankMaxTabs
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
	// sScriptMgr->OnGuildBankEvent fires after the log write
	// (Guild.cpp:2683): (event, guild, eventType, tabId, playerGUIDLow,
	// itemOrMoney, itemStackCount, destTabId).
	s.fireGuildEvent(ctx, scripting.GuildEventOnBankEvent, s.luaGuildObject(ctx, guildID), eventType, hookTabID, uint32(playerGUID), itemOrMoney, uint16(stackCount), destTabID)
}

// logGuildEvent mirrors Guild::_LogEvent (Guild.cpp:2639) plus
// LogHolder<EventLogEntry>::AddEvent (Guild.cpp:141-152): it appends a row to
// guild_eventlog, keeping at most GUILD_EVENTLOG_MAX_RECORDS (100,
// SharedDefines.h:3248) newest rows per guild. C++ wraps the per-guild
// LogGuid modulo 100 and deletes the replaced row; Go uses ever-increasing
// LogGuids (same convention as logGuildBankEvent) — the query path orders by
// TimeStamp, LogGuid, which reproduces the in-memory oldest-first list, and
// the wire packet never carries LogGuid.
func (s *session) logGuildEvent(ctx context.Context, guildID uint32, eventType uint8, playerGUID1, playerGUID2 uint64, newRank uint8) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || guildID == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB

	var maxLogGuid uint32
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(LogGuid), 0) FROM guild_eventlog WHERE guildid = ?", guildID).Scan(&maxLogGuid)
	nextLogGuid := maxLogGuid + 1
	now := uint32(time.Now().Unix())

	_, _ = cdb.ExecContext(ctx, `INSERT INTO guild_eventlog (guildid, LogGuid, EventType, PlayerGuid1, PlayerGuid2, NewRank, TimeStamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		guildID, nextLogGuid, eventType, uint32(playerGUID1), uint32(playerGUID2), newRank, now)

	// Keep up to 100 logs per guild
	rows, err := cdb.QueryContext(ctx, "SELECT LogGuid FROM guild_eventlog WHERE guildid = ? ORDER BY TimeStamp DESC, LogGuid DESC", guildID)
	if err == nil {
		var logGuids []uint32
		for rows.Next() {
			var lg uint32
			if err := rows.Scan(&lg); err == nil {
				logGuids = append(logGuids, lg)
			}
		}
		rows.Close()
		if len(logGuids) > 100 {
			for _, lg := range logGuids[100:] {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM guild_eventlog WHERE guildid = ? AND LogGuid = ?", guildID, lg)
			}
		}
	}
	// sScriptMgr->OnGuildEvent fires after the log write (Guild.cpp:2650):
	// (event, guild, eventType, plrGUIDLow1, plrGUIDLow2, newRank).
	s.fireGuildEvent(ctx, scripting.GuildEventOnEvent, s.luaGuildObject(ctx, guildID), eventType, uint32(playerGUID1), uint32(playerGUID2), newRank)
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

type guildWithdrawExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// guildBankWithdrawalsRemaining mirrors Guild::_GetMemberRemainingSlots
// (Guild.cpp:2586): the guildmaster reports (int32)GUILD_WITHDRAW_SLOT_UNLIMITED
// (-1); other ranks report the tab's daily slot allowance minus today's
// withdrawals when the rank may view the tab, floored at 0.
func (s *session) guildBankWithdrawalsRemaining(ctx context.Context, guildID int64, tabID uint8) int32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	return guildBankWithdrawalsRemainingFor(ctx, s.server.CharactersStore.DB, guildID, tabID, s.playerGUID)
}

func guildBankWithdrawalsRemainingFor(ctx context.Context, cdb *sql.DB, guildID int64, tabID uint8, playerGUID uint64) int32 {
	var rank int64
	if err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", playerGUID, guildID).Scan(&rank); err != nil {
		return 0
	}
	if rank == 0 {
		return -1
	}
	var gbright, slotPerDay int64
	if err := cdb.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright, &slotPerDay); err != nil || gbright&0x01 == 0 {
		return 0
	}
	tabCol := fmt.Sprintf("tab%d", tabID)
	var withdrawn uint32
	_ = cdb.QueryRowContext(ctx, "SELECT "+tabCol+" FROM guild_member_withdraw WHERE guid = ?", playerGUID).Scan(&withdrawn)
	if remaining := int32(uint32(slotPerDay) - withdrawn); remaining > 0 {
		return remaining
	}
	return 0
}

func guildConsumeBankWithdraw(ctx context.Context, q guildWithdrawExecutor, playerGUID uint64, guildID uint32, tabID uint8) bool {
	if tabID >= guildBankMaxTabs {
		return false
	}
	var rank uint32
	err := q.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", playerGUID, guildID).Scan(&rank)
	if err != nil {
		return false
	}
	if rank == 0 {
		return true
	}
	var gbright, slotPerDay uint32
	err = q.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright, &slotPerDay)
	if err != nil || gbright&0x01 == 0 || slotPerDay == 0 {
		return false
	}
	if slotPerDay != 0xFFFFFFFF {
		tabCol := fmt.Sprintf("tab%d", tabID)
		var exists int
		_ = q.QueryRowContext(ctx, "SELECT 1 FROM guild_member_withdraw WHERE guid = ?", playerGUID).Scan(&exists)
		if exists == 0 {
			if _, err := q.ExecContext(ctx, "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, 0, 0, 0, 0, 0, 0, 0)", playerGUID); err != nil {
				return false
			}
		}
		var currentWithdrawn uint32
		_ = q.QueryRowContext(ctx, "SELECT "+tabCol+" FROM guild_member_withdraw WHERE guid = ?", playerGUID).Scan(&currentWithdrawn)
		if currentWithdrawn >= slotPerDay {
			return false
		}
		if _, err := q.ExecContext(ctx, "UPDATE guild_member_withdraw SET "+tabCol+" = "+tabCol+" + 1 WHERE guid = ?", playerGUID); err != nil {
			return false
		}
	}
	return true
}

// guildCheckBankWithdraw mirrors the check portion of
// BankMoveItemData::HasWithdrawRights (Guild.cpp:864-876) without consuming
// the slot: guildmaster (rank 0) passes, other ranks need the tab's view
// right (0x01) and at least one remaining daily withdraw slot on the tab.
// Used for same-tab bank rearrangements, where _MoveItems step 4 still
// applies but RemoveItem never decrements (Guild.cpp:888-894).
func guildCheckBankWithdraw(ctx context.Context, q guildWithdrawExecutor, playerGUID uint64, guildID uint32, tabID uint8) bool {
	if tabID >= guildBankMaxTabs {
		return false
	}
	var rank uint32
	err := q.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", playerGUID, guildID).Scan(&rank)
	if err != nil {
		return false
	}
	if rank == 0 {
		return true
	}
	var gbright, slotPerDay uint32
	err = q.QueryRowContext(ctx, "SELECT gbright, SlotPerDay FROM guild_bank_right WHERE guildid = ? AND TabId = ? AND rid = ?", guildID, tabID, rank).Scan(&gbright, &slotPerDay)
	if err != nil || gbright&0x01 == 0 || slotPerDay == 0 {
		return false
	}
	if slotPerDay == 0xFFFFFFFF {
		return true
	}
	tabCol := fmt.Sprintf("tab%d", tabID)
	var currentWithdrawn uint32
	_ = q.QueryRowContext(ctx, "SELECT "+tabCol+" FROM guild_member_withdraw WHERE guid = ?", playerGUID).Scan(&currentWithdrawn)
	return currentWithdrawn < slotPerDay
}

// checkGuildBankMoneyWithdraw mirrors the silent pre-transfer validation of
// Guild::HandleMemberWithdrawMoney (Guild.cpp:1724-1746): member-miss, the
// withdraw-rights gate and the daily allowance check
// (Guild::_GetMemberRemainingMoney, Guild.cpp:2600). It performs no writes;
// the allowance is consumed only after the player money cap
// (Player::ModifyMoney, Player.cpp:22821) has passed, so a cap failure never
// eats into the daily allowance.
func (s *session) checkGuildBankMoneyWithdraw(ctx context.Context, guildID uint32, amount uint32) bool {
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
	// Guild::_GetMemberRemainingMoney (Guild.cpp:2600): non-guildmaster
	// ranks need GR_RIGHT_WITHDRAW_REPAIR|GR_RIGHT_WITHDRAW_GOLD before the
	// daily allowance is even consulted.
	var rights uint32
	if err := cdb.QueryRowContext(ctx, "SELECT rights FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&rights); err != nil || rights&(guildRightWithdrawRepair|guildRightWithdrawGold) == 0 {
		return false
	}
	var bankMoneyPerDay uint32
	err = cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&bankMoneyPerDay)
	if err != nil || bankMoneyPerDay == 0 {
		return false
	}
	if bankMoneyPerDay != 0xFFFFFFFF {
		var currentWithdrawn uint32
		_ = cdb.QueryRowContext(ctx, "SELECT money FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&currentWithdrawn)
		if currentWithdrawn+amount > bankMoneyPerDay {
			return false
		}
	}
	return true
}

// consumeGuildBankMoneyWithdraw mirrors Guild::Member::UpdateBankWithdrawValue
// (Guild.cpp:669) for the money column: once the transfer is approved it
// records the withdrawn amount against the member's daily allowance. The
// guildmaster and unlimited (0xFFFFFFFF) ranks have no allowance to track.
func (s *session) consumeGuildBankMoneyWithdraw(ctx context.Context, guildID uint32, amount uint32) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	var rank uint32
	if err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, guildID).Scan(&rank); err != nil || rank == 0 {
		return
	}
	var bankMoneyPerDay uint32
	if err := cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&bankMoneyPerDay); err != nil || bankMoneyPerDay == 0xFFFFFFFF {
		return
	}
	var exists int
	_ = cdb.QueryRowContext(ctx, "SELECT 1 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&exists)
	if exists == 0 {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, 0, 0, 0, 0, 0, 0, 0)", s.playerGUID)
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE guild_member_withdraw SET money = money + ? WHERE guid = ?", amount, s.playerGUID)
}

// guildBankWithdrawMoneyForRepair mirrors Guild::HandleMemberWithdrawMoney
// (Guild.cpp:1724) with repair=true: the repair cost is drawn from the guild
// bank and consumes the member's daily money-withdraw allowance, but no money
// is credited to the player, and the bank event is logged as
// GUILD_BANK_LOG_REPAIR_MONEY (6). On any failure it returns false and the
// caller leaves the item unrepaired (Player::DurabilityRepair,
// Player.cpp:5084 — no personal-money fallback when the guild bank is used).
func (s *session) guildBankWithdrawMoneyForRepair(ctx context.Context, guildID uint32, amount uint32) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	// Guild.cpp:1727: clamp to the player money cap before every check.
	if amount > maxMoneyAmount {
		amount = maxMoneyAmount
	}
	cdb := s.server.CharactersStore.DB
	var bankMoney int64
	if err := cdb.QueryRowContext(ctx, "SELECT BankMoney FROM guild WHERE guildid = ?", guildID).Scan(&bankMoney); err != nil || uint64(bankMoney) < uint64(amount) {
		return false
	}
	var rank uint32
	if err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, guildID).Scan(&rank); err != nil {
		return false
	}
	// Guild::_GetMemberRemainingMoney (Guild.cpp:2600): the guildmaster rank is
	// unlimited; other ranks need GR_RIGHT_WITHDRAW_REPAIR|GR_RIGHT_WITHDRAW_GOLD
	// and a positive (per-day allowance - today's withdrawals) difference.
	if rank != 0 {
		var rights uint32
		if err := cdb.QueryRowContext(ctx, "SELECT rights FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&rights); err != nil || rights&(guildRightWithdrawRepair|guildRightWithdrawGold) == 0 {
			return false
		}
		var perDay, withdrawn uint32
		_ = cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", guildID, rank).Scan(&perDay)
		_ = cdb.QueryRowContext(ctx, "SELECT money FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&withdrawn)
		// Same allowance form as checkGuildBankMoneyWithdraw:
		// withdrawn+amount > perDay denies; C++ _GetMemberRemainingMoney
		// (Guild.cpp:2600) clamps non-positive remainders to 0.
		if withdrawn+amount > perDay {
			return false
		}
		// ScriptMgr::OnGuildMemberWitdrawMoney (Guild.cpp:1742, repair=true):
		// fires after validation and before the allowance update and bank
		// debit; a handler's numeric return rewrites the amount
		// (GuildHooks.cpp OnMemberWitdrawMoney), so the allowance, debit,
		// log, and broadcast below all run on the rewritten amount.
		amount = s.fireGuildMoneyEvent(ctx, scripting.GuildEventOnMoneyWithdraw, s.luaGuildObject(ctx, guildID), s.luaPlayer(), amount, true)
		var exists int
		_ = cdb.QueryRowContext(ctx, "SELECT 1 FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&exists)
		if exists == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, 0, 0, 0, 0, 0, 0, 0)", s.playerGUID)
		}
		// Guild.cpp:1755: the repair arm still consumes the daily allowance.
		_, _ = cdb.ExecContext(ctx, "UPDATE guild_member_withdraw SET money = money + ? WHERE guid = ?", amount, s.playerGUID)
	}
	// Guild.cpp:1749-1758: no money is credited to the player; the bank is
	// debited and a REPAIR_MONEY event is logged.
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET BankMoney = BankMoney - ? WHERE guildid = ?", amount, guildID)
	s.logGuildBankEvent(ctx, guildID, 0, guildBankLogRepairMoney, s.playerGUID, amount, 0, 0)
	s.broadcastGuildBankMoneySet(guildID, bankMoney-int64(amount))
	return true
}

// handleGuildBankSwapItems processes CMSG_GUILD_BANK_SWAP_ITEMS (0x3E9).
// Reference: WorldSession::HandleGuildBankSwapItems (GuildHandler.cpp:320).
func (s *session) handleGuildBankSwapItems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	// Reference: WorldSession::HandleGuildBankSwapItems (GuildHandler.cpp:
	// 307-308): no interactable guild-bank gameobject -> silent return,
	// before the guild lookup.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
		return true
	}
	bankOnly, err := r.ReadU8()
	if err != nil {
		return false
	}

	guildID := s.player.GuildID
	if guildID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	var purchasedTabs int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs); err != nil || purchasedTabs <= 0 {
		return true
	}
	var bankTab uint8
	// Guild::_MoveItems sends _SendBankContentUpdate only on the success
	// path (Guild.cpp:2683-2733); the handler emits no bank-list refresh on
	// a failed move, so the trailing sendGuildBankList below is gated on
	// guildMoveItem's moved flag (GuildHandler.cpp:306-335).
	movedItems := false
	// Guild::_SendBankContentUpdate (Guild.cpp:2797-2823): a bank->bank
	// move across tabs emits a second list message for the src tab, after
	// the dest-tab message. Track that extra tab here.
	var extraListTab uint8
	sendExtraList := false
	// The trailing content-update lists are partial: each carries exactly
	// the changed bank slots (emptied ones as ItemID=0), in ascending
	// slot order like C++'s SlotIds set (Guild::_SendBankContentUpdate,
	// Guild.cpp:2794-2826; _SendBankList, Guild.cpp:2888-2904).
	var contentSlots []uint8
	var extraContentSlots []uint8
	if bankOnly != 0 {
		bankTab, err = r.ReadU8()
		if err != nil {
			return false
		}
		bankSlot, err := r.ReadU8()
		if err != nil {
			return false
		}
		if _, err = r.ReadU32(); err != nil {
			return false
		}
		bankTab1, err := r.ReadU8()
		if err != nil {
			return false
		}
		bankSlot1, err := r.ReadU8()
		if err != nil {
			return false
		}
		if _, err = r.ReadU32(); err != nil {
			return false
		}
		if _, err = r.ReadU8(); err != nil { // AutoStore is consumed but unused for BankOnly
			return false
		}
		bankItemCount, err := r.ReadU32()
		if err != nil {
			return false
		}

		if int64(bankTab) >= purchasedTabs || int64(bankTab1) >= purchasedTabs || bankSlot >= guildBankMaxSlots || bankSlot1 >= guildBankMaxSlots {
			return true
		}

		// The _MoveItems rights checks (dest DEPOSIT_ITEM, source withdraw
		// slots — Guild::_MoveItems steps 3-4, Guild.cpp:2683-2690) live
		// inside guildMoveItem and fail silently, C++-exact; no upfront
		// rights block or error feedback exists on this path.
		source := guildMoveLocation{Bank: true, Tab: bankTab1, Slot: bankSlot1}
		destination := guildMoveLocation{Bank: true, Tab: bankTab, Slot: bankSlot}
		outcome, moved := s.guildMoveItem(ctx, guildID, source, &destination, false, bankItemCount)
		movedItems = moved
		if moved {
			// Same tab: one message for the src tab carries the src and
			// dest slots; cross-tab: the dest-tab message carries only
			// the dest slot, the src tab gets its own message below.
			if bankTab == bankTab1 && bankSlot1 != bankSlot {
				contentSlots = []uint8{bankSlot1, bankSlot}
				if bankSlot1 > bankSlot {
					contentSlots[0], contentSlots[1] = contentSlots[1], contentSlots[0]
				}
			} else {
				contentSlots = []uint8{bankSlot}
			}
		}
		if moved && bankTab != bankTab1 {
			// Cross-tab: the src tab gets its own list message after the
			// dest-tab one (Guild.cpp:2812-2823).
			sendExtraList = true
			extraListTab = bankTab1
			extraContentSlots = []uint8{bankSlot1}
			s.logGuildBankEvent(ctx, guildID, bankTab1, guildBankLogMoveItem, s.playerGUID, outcome.Source.Entry, outcome.Count, bankTab)
			if outcome.Swapped {
				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogMoveItem, s.playerGUID, outcome.Destination.Entry, outcome.Destination.Count, bankTab1)
			}
		}
	} else {
		bankTab, err = r.ReadU8()
		if err != nil {
			return false
		}
		bankSlot, err := r.ReadU8()
		if err != nil {
			return false
		}
		if _, err = r.ReadU32(); err != nil {
			return false
		}
		autoStoreByte, err := r.ReadU8()
		if err != nil {
			return false
		}
		autoStore := autoStoreByte != 0

		if int64(bankTab) >= purchasedTabs || bankSlot >= guildBankMaxSlots && bankSlot != 0xFF {
			return true
		}

		var containerSlot, containerItemSlot, toSlot uint8
		var splitCount uint32
		if autoStore {
			if _, err = r.ReadU32(); err != nil { // BankItemCount is parsed but the handler uses split amount zero for AutoStore
				return false
			}
			if toSlot, err = r.ReadU8(); err != nil {
				return false
			}
			if _, err = r.ReadU32(); err != nil { // StackCount is parsed but ignored by the handler for AutoStore
				return false
			}
		} else {
			if containerSlot, err = r.ReadU8(); err != nil {
				return false
			}
			if containerItemSlot, err = r.ReadU8(); err != nil {
				return false
			}
			if toSlot, err = r.ReadU8(); err != nil {
				return false
			}
			if splitCount, err = r.ReadU32(); err != nil {
				return false
			}
		}

		if autoStore || toSlot != 0 {
			// Bank -> Player Inventory (Withdraw). Guild::_MoveItems step 3
			// (Guild.cpp:2683-2690) needs no dest rights check here —
			// PlayerMoveItemData inherits the base HasStoreRights
			// (Guild.h:550), which returns true — and step 4's source-tab
			// gate is the withdraw-slots check
			// (BankMoveItemData::HasWithdrawRights, Guild.cpp:864-876),
			// enforced silently by guildMoveItem's consumeWithdraw below.
			// C++ is silent on this path, so no upfront rights block or
			// command-result feedback exists here.
			source := guildMoveLocation{Bank: true, Tab: bankTab, Slot: bankSlot}
			autoTarget := autoStore || containerSlot == 0xFF && containerItemSlot == 0xFF
			var destination *guildMoveLocation
			if !autoTarget {
				bagKey, ok := s.inventoryBagKey(ctx, containerSlot)
				if !ok {
					return true
				}
				destination = &guildMoveLocation{Bag: bagKey, Slot: containerItemSlot, BagSlot: containerSlot}
			} else {
				destination = &guildMoveLocation{BagSlot: 0xFF, Slot: 0xFF}
			}
			outcome, moved := s.guildMoveItem(ctx, guildID, source, destination, autoTarget, splitCount)
			movedItems = moved
			if moved {
				// B->C: only the emptied/reduced src bank slot changes.
				contentSlots = []uint8{bankSlot}
				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogWithdrawItem, s.playerGUID, outcome.Source.Entry, outcome.Count, 0)
				if outcome.Swapped {
					s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogDepositItem, s.playerGUID, outcome.Destination.Entry, outcome.Destination.Count, 0)
				}
				_ = s.sendInventoryItems(ctx)
				s.sendPlayerUpdate()
			}
		} else {
			// Player Inventory -> Bank (Deposit). The dest-tab
			// DEPOSIT_ITEM gate (Guild::_MoveItems step 3,
			// BankMoveItemData::HasStoreRights, Guild.cpp:855-862) lives
			// inside guildMoveItem and fails silently, C++-exact; no
			// upfront rights block or command-result feedback exists on
			// this path.
			bagKey, ok := s.inventoryBagKey(ctx, containerSlot)
			if !ok {
				return true
			}
			source := guildMoveLocation{Bag: bagKey, Slot: containerItemSlot, BagSlot: containerSlot}
			destination := guildMoveLocation{Bank: true, Tab: bankTab, Slot: bankSlot}
			outcome, moved := s.guildMoveItem(ctx, guildID, source, &destination, false, splitCount)
			movedItems = moved
			if moved {
				// C->B: only the dest bank slot changes.
				contentSlots = []uint8{bankSlot}
				s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogDepositItem, s.playerGUID, outcome.Source.Entry, outcome.Count, 0)
				if outcome.Swapped {
					s.logGuildBankEvent(ctx, guildID, bankTab, guildBankLogWithdrawItem, s.playerGUID, outcome.Destination.Entry, outcome.Destination.Count, 0)
				}
				_ = s.sendInventoryItems(ctx)
				s.sendPlayerUpdate()
			}
		}
	}
	if movedItems {
		// The dest-tab message precedes the src-tab one on cross-tab
		// moves (Guild.cpp:2812-2823). Guild::_SendBankContentUpdate
		// (Guild.cpp:2794-2826) fans the refresh out to every online
		// member with VIEW_TAB on the tab, not just the acting session.
		s.broadcastGuildBankList(ctx, guildID, bankTab, false, contentSlots)
		if sendExtraList {
			s.broadcastGuildBankList(ctx, guildID, extraListTab, false, extraContentSlots)
		}
	}
	return true
}

type guildMoveQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type guildMoveLocation struct {
	Bank bool
	Tab  uint8
	Bag  int64
	// BagSlot is the C++ bag slot id (PlayerMoveItemData::GetContainer)
	// for the GUILD_EVENT_ON_ITEM_MOVE hook; the Bag key alone cannot
	// recover it (inventoryBagKey maps slots to bag item GUIDs).
	BagSlot uint8
	Slot    uint8
}

type guildMoveItem struct {
	GUID  uint64
	Entry uint32
	Count uint32
	Flags uint32
}

type guildMovePlacement struct {
	Location     guildMoveLocation
	ExistingGUID uint64
	Count        uint32
}

type guildPlayerMoveSlot struct {
	Location guildMoveLocation
	Family   uint32
	Item     guildMoveItem
	Occupied bool
}

type guildMoveOutcome struct {
	Source      guildMoveItem
	Destination guildMoveItem
	SourceLoc   guildMoveLocation
	DestLoc     guildMoveLocation
	Count       uint32
	Swapped     bool
}

func guildReadMoveItem(ctx context.Context, q guildMoveQueryer, guildID, playerGUID uint64, loc guildMoveLocation) (guildMoveItem, error) {
	var guid, entry, count, flags int64
	var err error
	if loc.Bank {
		err = q.QueryRowContext(ctx, `SELECT gbi.item_guid, ii.itemEntry, ii.count, ii.flags
			FROM guild_bank_item gbi JOIN item_instance ii ON ii.guid = gbi.item_guid
			WHERE gbi.guildid = ? AND gbi.TabId = ? AND gbi.SlotId = ? LIMIT 1`, guildID, loc.Tab, loc.Slot).Scan(&guid, &entry, &count, &flags)
	} else {
		err = q.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count, ii.flags
			FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, playerGUID, loc.Bag, loc.Slot).Scan(&guid, &entry, &count, &flags)
	}
	if err != nil {
		return guildMoveItem{}, err
	}
	if guid <= 0 || entry <= 0 || count <= 0 {
		return guildMoveItem{}, sql.ErrNoRows
	}
	return guildMoveItem{GUID: uint64(guid), Entry: uint32(entry), Count: uint32(count), Flags: uint32(flags)}, nil
}

func guildClearMoveLocation(ctx context.Context, tx *sql.Tx, guildID, playerGUID uint64, loc guildMoveLocation) error {
	if loc.Bank {
		_, err := tx.ExecContext(ctx, "DELETE FROM guild_bank_item WHERE guildid = ? AND TabId = ? AND SlotId = ?", guildID, loc.Tab, loc.Slot)
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", playerGUID, loc.Bag, loc.Slot)
	return err
}

func guildInsertMoveLocation(ctx context.Context, tx *sql.Tx, guildID, playerGUID uint64, loc guildMoveLocation, itemGUID uint64) error {
	if loc.Bank {
		_, err := tx.ExecContext(ctx, "INSERT INTO guild_bank_item (guildid, TabId, SlotId, item_guid) VALUES (?, ?, ?, ?)", guildID, loc.Tab, loc.Slot, itemGUID)
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", playerGUID, loc.Bag, loc.Slot, itemGUID)
	return err
}

func guildMoveOwner(loc guildMoveLocation, playerGUID uint64) uint64 {
	if loc.Bank {
		return 0
	}
	return playerGUID
}

func (s *session) guildMoveMaxStack(ctx context.Context, entry uint32) uint32 {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 1
	}
	var maxStack int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(stackable, 1) FROM item_template WHERE entry = ?", entry).Scan(&maxStack); err != nil || maxStack < 1 {
		return 1
	}
	return uint32(maxStack)
}

func (s *session) guildCloneMoveItem(ctx context.Context, tx *sql.Tx, sourceGUID uint64, count uint32, ownerGUID uint64) (uint64, error) {
	var entry, creator, giftCreator, duration, flags int64
	if err := tx.QueryRowContext(ctx, "SELECT itemEntry, creatorGuid, giftCreatorGuid, duration, flags FROM item_instance WHERE guid = ?", sourceGUID).Scan(&entry, &creator, &giftCreator, &duration, &flags); err != nil {
		return 0, err
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, fmt.Errorf("world database unavailable for item clone")
	}
	var maxDurability, charge1, charge2, charge3, charge4, charge5 int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(MaxDurability, 0), COALESCE(spellcharges_1, 0), COALESCE(spellcharges_2, 0),
		COALESCE(spellcharges_3, 0), COALESCE(spellcharges_4, 0), COALESCE(spellcharges_5, 0) FROM item_template WHERE entry = ?`, entry).Scan(&maxDurability, &charge1, &charge2, &charge3, &charge4, &charge5); err != nil {
		return 0, err
	}
	nextGUID := int64(s.server.generateItemGUID())
	if nextGUID <= 0 {
		return 0, fmt.Errorf("item guid space exhausted")
	}
	charges := fmt.Sprintf("%d %d %d %d %d ", charge1, charge2, charge3, charge4, charge5)
	flags &^= int64(itemInstanceFlagRefundable | itemInstanceFlagBOPTradeable)
	if _, err := tx.ExecContext(ctx, `INSERT INTO item_instance
		(guid, itemEntry, owner_guid, creatorGuid, giftCreatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, '')`, nextGUID, entry, ownerGUID, creator, giftCreator, count, duration, charges, flags, strings.Repeat("0 ", 36), maxDurability); err != nil {
		return 0, err
	}
	return uint64(nextGUID), nil
}

func (s *session) guildBankMovePlan(ctx context.Context, q guildMoveQueryer, guildID uint64, destTab, destSlot uint8, sourceLoc guildMoveLocation, source guildMoveItem, moveCount, maxStack uint32, full bool) ([]guildMovePlacement, uint8) {
	rows, err := q.QueryContext(ctx, `SELECT gbi.SlotId, gbi.item_guid, ii.itemEntry, ii.count
		FROM guild_bank_item gbi JOIN item_instance ii ON ii.guid = gbi.item_guid
		WHERE gbi.guildid = ? AND gbi.TabId = ? ORDER BY gbi.SlotId`, guildID, destTab)
	if err != nil {
		return nil, guildEquipErrBankFull
	}
	slots := make(map[uint8]guildMoveItem)
	for rows.Next() {
		var slot uint8
		var guid, entry, count int64
		if rows.Scan(&slot, &guid, &entry, &count) == nil && guid > 0 && entry > 0 && count > 0 {
			slots[slot] = guildMoveItem{GUID: uint64(guid), Entry: uint32(entry), Count: uint32(count)}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, guildEquipErrBankFull
	}
	rows.Close()
	if sourceLoc.Bank && sourceLoc.Tab == destTab && full {
		delete(slots, sourceLoc.Slot)
	}
	remaining := moveCount
	placements := make([]guildMovePlacement, 0, 3)
	add := func(slot uint8, existing uint64, n uint32) {
		placements = append(placements, guildMovePlacement{Location: guildMoveLocation{Bank: true, Tab: destTab, Slot: slot}, ExistingGUID: existing, Count: n})
		remaining -= n
	}
	if destSlot != 0xFF {
		if dest, ok := slots[destSlot]; ok {
			if dest.Entry != source.Entry || dest.Count >= maxStack || maxStack <= 1 {
				return nil, guildEquipErrItemCantStack
			}
			space := maxStack - dest.Count
			n := remaining
			if n > space {
				n = space
			}
			add(destSlot, dest.GUID, n)
		} else {
			n := remaining
			if n > maxStack {
				n = maxStack
			}
			add(destSlot, 0, n)
		}
	}
	for slot := uint8(0); slot < guildBankMaxSlots && remaining > 0; slot++ {
		if slot == destSlot {
			continue
		}
		if dest, ok := slots[slot]; ok && dest.GUID != source.GUID && dest.Entry == source.Entry && dest.Count < maxStack && maxStack > 1 {
			space := maxStack - dest.Count
			n := remaining
			if n > space {
				n = space
			}
			add(slot, dest.GUID, n)
		}
	}
	for slot := uint8(0); slot < guildBankMaxSlots && remaining > 0; slot++ {
		if slot == destSlot {
			continue
		}
		if _, occupied := slots[slot]; occupied {
			continue
		}
		n := remaining
		if n > maxStack {
			n = maxStack
		}
		add(slot, 0, n)
	}
	if remaining != 0 {
		return nil, guildEquipErrBankFull
	}
	return placements, 0
}

func guildBagAccepts(itemFamily, bagFamily uint32) bool {
	return bagFamily == 0 || itemFamily != 0 && itemFamily&bagFamily != 0
}

func (s *session) guildPlayerMoveSlots(ctx context.Context, q guildMoveQueryer, playerGUID uint64, itemEntry uint32) ([]guildPlayerMoveSlot, uint8) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, equipErrItemDoesntGoIntoBag
	}
	var itemFamily int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(BagFamily, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&itemFamily); err != nil {
		return nil, equipErrItemDoesntGoIntoBag
	}
	type container struct {
		key, first, slots int64
		family            uint32
	}
	containers := []container{
		{key: 0, first: int64(bagSlotStart), slots: int64(bagSlotEnd - bagSlotStart + 1)},
		// C++ auto-store searches the keyring and currencytoken ranges first
		// (Player.cpp:10947); the family masks gate them to keys and currency
		// tokens (CanStoreItem_InSpecificSlot, Player.cpp:10524-10529).
		{key: 0, first: int64(invSlotKeyringStart), slots: int64(invSlotKeyringEnd - invSlotKeyringStart), family: bagFamilyMaskKeys},
		{key: 0, first: int64(currencyTokenSlotStart), slots: int64(currencyTokenSlotEnd - currencyTokenSlotStart), family: bagFamilyMaskCurrencyTokens},
	}
	for _, bag := range s.getEquippedBags(ctx, playerGUID) {
		var family int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(t.BagFamily, 0) FROM item_instance ii
			JOIN item_template t ON t.entry = ii.itemEntry WHERE ii.guid = ?`, bag.guid).Scan(&family); err != nil {
			continue
		}
		containers = append(containers, container{key: bag.guid, slots: bag.slots, family: uint32(family)})
	}
	rows, err := q.QueryContext(ctx, `SELECT ci.bag, ci.slot, ci.item, ii.itemEntry, ii.count, ii.flags
		FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ((ci.bag = 0 AND ci.slot >= ? AND ci.slot <= ?) OR (ci.bag = 0 AND ci.slot >= ? AND ci.slot < ?) OR (ci.bag = 0 AND ci.slot >= ? AND ci.slot < ?) OR ci.bag IN
		(SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 19 AND slot <= 22))`, playerGUID, bagSlotStart, bagSlotEnd, invSlotKeyringStart, invSlotKeyringEnd, currencyTokenSlotStart, currencyTokenSlotEnd, playerGUID)
	if err != nil {
		return nil, equipErrInvFull
	}
	type key struct{ bag, slot int64 }
	items := make(map[key]guildMoveItem)
	for rows.Next() {
		var bag, slot, guid, entry, count, flags int64
		if rows.Scan(&bag, &slot, &guid, &entry, &count, &flags) == nil && guid > 0 && entry > 0 && count > 0 {
			items[key{bag: bag, slot: slot}] = guildMoveItem{GUID: uint64(guid), Entry: uint32(entry), Count: uint32(count), Flags: uint32(flags)}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, equipErrInvFull
	}
	rows.Close()
	slots := make([]guildPlayerMoveSlot, 0, 16)
	for _, bag := range containers {
		for slot := bag.first; slot < bag.first+bag.slots; slot++ {
			it, occupied := items[key{bag: bag.key, slot: slot}]
			loc := guildMoveLocation{Bag: bag.key, Slot: uint8(slot)}
			if !guildBagAccepts(uint32(itemFamily), bag.family) {
				continue
			}
			slots = append(slots, guildPlayerMoveSlot{Location: loc, Family: bag.family, Item: it, Occupied: occupied})
		}
	}
	return slots, 0
}

// guildPlayerSimilarItemCap mirrors the MaxCount term of
// Player::CanTakeMoreSimilarItems (Player.cpp:10404-10459) as invoked from
// Player::CanStoreItem (Player.cpp:10731-10742) before any destination
// planning: withdrawing more of an entry than the MaxCount headroom allows
// stores only the headroom; zero headroom fails the whole move with
// EQUIP_ERR_CANT_CARRY_MORE_OF_THIS (ItemDefines.h:43, = 17) instead of
// planning. The count is character-inventory-wide (GetItemCount with
// inBankAlso=true, Player.cpp:9914), so it is a bag-agnostic sum over
// character_inventory; the source item lives in the guild bank, not
// character_inventory, so no skip-GUID exclusion is needed (C++ skips
// pItem, the bank item). Socketed-gem counts (Player.cpp:9924-9930) have no
// Go model. The ItemLimitCategory sub-term (Player.cpp:10440-10457) needs
// ItemLimitCategory.dbc, absent from this server, so it is not modeled.
func (s *session) guildPlayerSimilarItemCap(ctx context.Context, q guildMoveQueryer, playerGUID uint64, entry, count uint32) (uint32, uint8) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return count, 0
	}
	var maxCount, limitCategory int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(MaxCount, 0), COALESCE(ItemLimitCategory, 0) FROM item_template WHERE entry = ?`, entry).Scan(&maxCount, &limitCategory); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// C++ answers EQUIP_ERR_CANT_CARRY_MORE_OF_THIS for a missing
			// template (Player.cpp:10406-10411).
			return 0, equipErrCantCarryMoreOfThis
		}
		// A transport error is deferred to the planner's own template
		// query rather than failing the move here.
		return count, 0
	}
	_ = limitCategory // ItemLimitCategory.dbc absent; sub-term not modeled (see above).
	if maxCount <= 0 || maxCount == 2147483647 {
		return count, 0
	}
	var owned int64
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, entry).Scan(&owned); err != nil {
		return count, 0
	}
	headroom := maxCount - owned
	if headroom <= 0 {
		return 0, equipErrCantCarryMoreOfThis
	}
	if uint64(count) > uint64(headroom) {
		return uint32(headroom), 0
	}
	return count, 0
}

func (s *session) guildPlayerMovePlan(ctx context.Context, q guildMoveQueryer, playerGUID uint64, entry, count, maxStack uint32, target *guildMoveLocation) ([]guildMovePlacement, uint8) {
	slots, errCode := s.guildPlayerMoveSlots(ctx, q, playerGUID, entry)
	if errCode != 0 {
		return nil, errCode
	}
	remaining := count
	placements := make([]guildMovePlacement, 0, 3)
	used := make(map[guildMoveLocation]bool)
	add := func(slot guildPlayerMoveSlot, existing uint64, n uint32) {
		placements = append(placements, guildMovePlacement{Location: slot.Location, ExistingGUID: existing, Count: n})
		used[slot.Location] = true
		remaining -= n
	}
	if target != nil {
		found := false
		for _, slot := range slots {
			if slot.Location != *target {
				continue
			}
			found = true
			if slot.Occupied {
				if slot.Item.Entry != entry || slot.Item.Count >= maxStack || maxStack <= 1 {
					return nil, guildEquipErrItemCantStack
				}
				space := maxStack - slot.Item.Count
				n := remaining
				if n > space {
					n = space
				}
				add(slot, slot.Item.GUID, n)
			} else {
				n := remaining
				if n > maxStack {
					n = maxStack
				}
				add(slot, 0, n)
			}
			break
		}
		if !found {
			return nil, guildEquipErrItemDoesntGoIntoBag
		}
	}
	// Player::CanStoreItem auto-store (NULL_BAG/NULL_SLOT, Player.cpp:10701)
	// search order. guildPlayerMoveSlots already dropped containers whose
	// family rejects the item, so a non-empty keyring/currency/specialized
	// group implies a bag-family item; the family gating makes the merge pass
	// over the keyring/currency ranges equivalent to C++ (a same-entry merge
	// target there can only be a key or currency token). Merge phase:
	// keyring + currencytoken ranges, then backpack, then specialized bags,
	// then plain bags (Player.cpp:10945-11024). Free-slot phase: for family
	// items the dedicated range (keyring for keys, currencytoken for currency
	// tokens) comes first, then specialized bags, then the backpack, then
	// plain bags; otherwise the backpack comes first (Player.cpp:11027-11120).
	var keyring, currency, backpack, specialized, plain []guildPlayerMoveSlot
	for _, slot := range slots {
		switch {
		case slot.Location.Bag == 0 && slot.Location.Slot >= invSlotKeyringStart && slot.Location.Slot < invSlotKeyringEnd:
			keyring = append(keyring, slot)
		case slot.Location.Bag == 0 && slot.Location.Slot >= currencyTokenSlotStart && slot.Location.Slot < currencyTokenSlotEnd:
			currency = append(currency, slot)
		case slot.Location.Bag == 0:
			backpack = append(backpack, slot)
		case slot.Family != 0:
			specialized = append(specialized, slot)
		default:
			plain = append(plain, slot)
		}
	}
	mergeOrder := make([]guildPlayerMoveSlot, 0, len(slots))
	mergeOrder = append(mergeOrder, keyring...)
	mergeOrder = append(mergeOrder, currency...)
	mergeOrder = append(mergeOrder, backpack...)
	mergeOrder = append(mergeOrder, specialized...)
	mergeOrder = append(mergeOrder, plain...)
	freeOrder := make([]guildPlayerMoveSlot, 0, len(slots))
	if len(keyring)+len(currency)+len(specialized) > 0 {
		freeOrder = append(freeOrder, keyring...)
		freeOrder = append(freeOrder, currency...)
		freeOrder = append(freeOrder, specialized...)
	}
	freeOrder = append(freeOrder, backpack...)
	freeOrder = append(freeOrder, plain...)
	for _, slot := range mergeOrder {
		if remaining == 0 {
			break
		}
		if used[slot.Location] || !slot.Occupied || slot.Item.Entry != entry || slot.Item.Count >= maxStack || maxStack <= 1 {
			continue
		}
		space := maxStack - slot.Item.Count
		n := remaining
		if n > space {
			n = space
		}
		add(slot, slot.Item.GUID, n)
	}
	for _, slot := range freeOrder {
		if remaining == 0 {
			break
		}
		if used[slot.Location] || slot.Occupied {
			continue
		}
		n := remaining
		if n > maxStack {
			n = maxStack
		}
		add(slot, 0, n)
	}
	if remaining != 0 {
		return nil, equipErrInvFull
	}
	return placements, 0
}

func guildMoveClear(ctx context.Context, tx *sql.Tx, guildID, playerGUID uint64, loc guildMoveLocation) error {
	return guildClearMoveLocation(ctx, tx, guildID, playerGUID, loc)
}

func (s *session) guildApplyMovePlan(ctx context.Context, tx *sql.Tx, guildID, playerGUID uint64, sourceLoc guildMoveLocation, source guildMoveItem, moveCount uint32, full bool, placements []guildMovePlacement) error {
	if full {
		if err := guildMoveClear(ctx, tx, guildID, playerGUID, sourceLoc); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", moveCount, source.GUID); err != nil {
		return err
	}
	lastNew := -1
	if full {
		for i, placement := range placements {
			if placement.ExistingGUID == 0 {
				lastNew = i
			}
		}
	}
	for i, placement := range placements {
		if placement.ExistingGUID != 0 {
			if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = count + ? WHERE guid = ?", placement.Count, placement.ExistingGUID); err != nil {
				return err
			}
			continue
		}
		var guid uint64
		if i == lastNew {
			guid = source.GUID
			// Like guildCloneMoveItem above (and C++ MoveItemFromInventory ->
			// SetNotRefundable), a full-stack move out of player inventory
			// clears the refundable/BoP-tradeable bits and drops the refund row.
			if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = ?, owner_guid = ?, flags = flags & ? WHERE guid = ?", placement.Count, guildMoveOwner(placement.Location, playerGUID), ^int64(itemInstanceFlagRefundable|itemInstanceFlagBOPTradeable), guid); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM item_refund_instance WHERE item_guid = ?", guid); err != nil {
				return err
			}
		} else {
			var err error
			guid, err = s.guildCloneMoveItem(ctx, tx, source.GUID, placement.Count, guildMoveOwner(placement.Location, playerGUID))
			if err != nil {
				return err
			}
		}
		if err := guildInsertMoveLocation(ctx, tx, guildID, playerGUID, placement.Location, guid); err != nil {
			return err
		}
	}
	if full && lastNew < 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", source.GUID)
		return err
	}
	return nil
}

func (s *session) guildMoveCanSwap(ctx context.Context, sourceLoc, destLoc guildMoveLocation, source, destination guildMoveItem) uint8 {
	if sourceLoc.Bank && destLoc.Bank {
		if source.Flags&itemInstanceFlagSoulbound != 0 || destination.Flags&itemInstanceFlagSoulbound != 0 {
			return guildEquipErrCantDropSoulbound
		}
		return 0
	}
	if destLoc.Bank && source.Flags&itemInstanceFlagSoulbound != 0 || sourceLoc.Bank && destination.Flags&itemInstanceFlagSoulbound != 0 {
		return guildEquipErrCantDropSoulbound
	}
	if !destLoc.Bank {
		if !s.guildItemFitsPlayerBag(ctx, destLoc.Bag, source.Entry) {
			return guildEquipErrItemDoesntGoIntoBag
		}
	}
	if !sourceLoc.Bank {
		if !s.guildItemFitsPlayerBag(ctx, sourceLoc.Bag, destination.Entry) {
			return guildEquipErrItemDoesntGoIntoBag
		}
	}
	if !destLoc.Bank && s.guildItemIsNonemptyBag(ctx, destLoc, destination) {
		return equipErrCanOnlyDoWithEmptyBags
	}
	return 0
}

func (s *session) guildItemFitsPlayerBag(ctx context.Context, bagKey int64, entry uint32) bool {
	if bagKey == 0 {
		return true
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var itemFamily, bagFamily int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(BagFamily, 0) FROM item_template WHERE entry = ?", entry).Scan(&itemFamily); err != nil {
		return false
	}
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(t.BagFamily, 0) FROM item_instance ii
		JOIN item_template t ON t.entry = ii.itemEntry WHERE ii.guid = ?`, bagKey).Scan(&bagFamily); err != nil {
		return false
	}
	return guildBagAccepts(uint32(itemFamily), uint32(bagFamily))
}

func (s *session) guildItemIsNonemptyBag(ctx context.Context, loc guildMoveLocation, item guildMoveItem) bool {
	if loc.Bank || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var slots int64
	if s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(ContainerSlots, 0) FROM item_template WHERE entry = ?", item.Entry).Scan(&slots) != nil || slots <= 0 {
		return false
	}
	return !s.isBagEmpty(ctx, int64(item.GUID))
}

func (s *session) guildPlayerLocationValid(ctx context.Context, playerGUID uint64, loc guildMoveLocation) bool {
	if loc.Bag == 0 {
		return loc.Slot >= bagSlotStart && loc.Slot <= bagSlotEnd
	}
	var slots int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(t.ContainerSlots, 0) FROM character_inventory ci
		JOIN item_instance ii ON ii.guid = ci.item JOIN item_template t ON t.entry = ii.itemEntry
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.item = ? AND ci.slot >= 19 AND ci.slot <= 22 LIMIT 1`, playerGUID, loc.Bag).Scan(&slots); err != nil {
		return false
	}
	return loc.Slot < uint8(slots)
}

func guildSameMoveLocation(a, b guildMoveLocation) bool {
	return a.Bank == b.Bank && a.Tab == b.Tab && a.Bag == b.Bag && a.Slot == b.Slot
}

func guildFullItemGUID(guid uint64) uint64 { return guid | (uint64(0x4000) << 48) }

func (s *session) guildMoveItem(ctx context.Context, guildID uint32, sourceLoc guildMoveLocation, destination *guildMoveLocation, autoStore bool, requestedCount uint32) (guildMoveOutcome, bool) {
	cdb := s.server.CharactersStore.DB
	s.server.inventoryMu.Lock()
	defer s.server.inventoryMu.Unlock()
	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		return guildMoveOutcome{}, false
	}
	rollback := func(code uint8, itemGUID uint64) (guildMoveOutcome, bool) {
		_ = tx.Rollback()
		if code != 0 {
			s.sendEquipError(code, guildFullItemGUID(itemGUID))
		}
		return guildMoveOutcome{}, false
	}
	source, err := guildReadMoveItem(ctx, tx, uint64(guildID), s.playerGUID, sourceLoc)
	if err != nil {
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	if !sourceLoc.Bank && !s.guildPlayerLocationValid(ctx, s.playerGUID, sourceLoc) {
		_ = tx.Rollback()
		s.sendEquipError(equipErrOk, 0)
		return guildMoveOutcome{}, false
	}
	if destination != nil && !destination.Bank && !autoStore && !s.guildPlayerLocationValid(ctx, s.playerGUID, *destination) {
		_ = tx.Rollback()
		s.sendEquipError(equipErrOk, 0)
		return guildMoveOutcome{}, false
	}
	if destination != nil && !autoStore && guildSameMoveLocation(sourceLoc, *destination) {
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	if requestedCount > source.Count {
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	moveCount, full := requestedCount, false
	if moveCount == 0 || moveCount == source.Count {
		moveCount, full = source.Count, true
	}
	if !sourceLoc.Bank {
		if s.guildItemIsNonemptyBag(ctx, sourceLoc, source) {
			return rollback(equipErrCanOnlyDoWithEmptyBags, source.GUID)
		}
	}
	// PlayerMoveItemData::InitItem (Guild.cpp:810-812) rejects the item
	// before _MoveItems' rights steps with EQUIP_ERR_ITEMS_CANT_BE_SWAPPED
	// (21) when it cannot be traded — a soulbound player item is never
	// tradable (Item::CanBeTraded, Item.cpp:720-743) — so the deposit path
	// emits 21 here, not the EQUIP_ERR_CANT_DROP_SOULBOUND (24) that
	// BankMoveItemData::CanStore (Guild.cpp:1023-1029) emits for the
	// bank->bank direction. The 21 arm is unreachable there: soulbound
	// items cannot be deposited, so the bank side never holds them.
	if destination != nil && destination.Bank && source.Flags&itemInstanceFlagSoulbound != 0 {
		code := guildEquipErrCantDropSoulbound
		if !sourceLoc.Bank {
			code = equipErrItemsCantBeSwapped
		}
		return rollback(code, source.GUID)
	}
	maxStack := s.guildMoveMaxStack(ctx, source.Entry)
	// Guild::_MoveItems step 3 (Guild.cpp:2683-2690) runs before step 4's
	// withdraw-slot check and before any store planning: the deposit arm
	// (player -> bank) needs GUILD_BANK_RIGHT_DEPOSIT_ITEM
	// (VIEW_TAB|PUT_ITEM, Guild.h:181) on the dest tab
	// (BankMoveItemData::HasStoreRights, Guild.cpp:855-862; pOther is the
	// player, so the same-tab skip does not apply). Silent on failure —
	// HandleGuildBankSwapItems emits no command result on this path.
	if destination != nil && destination.Bank && !sourceLoc.Bank && !s.checkGuildBankRights(ctx, guildID, destination.Tab, true) {
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	var destItem guildMoveItem
	destExists := false
	if destination != nil && !autoStore {
		if it, readErr := guildReadMoveItem(ctx, tx, uint64(guildID), s.playerGUID, *destination); readErr == nil {
			destItem, destExists = it, true
		} else if readErr != sql.ErrNoRows {
			_ = tx.Rollback()
			return guildMoveOutcome{}, false
		}
	}
	consumeWithdraw := func(swapping bool) bool {
		if destination == nil {
			return true
		}
		if sourceLoc.Bank && (!destination.Bank || sourceLoc.Tab != destination.Tab) && !guildConsumeBankWithdraw(ctx, tx, s.playerGUID, guildID, sourceLoc.Tab) {
			return false
		}
		if swapping && destination.Bank && (!sourceLoc.Bank || sourceLoc.Tab != destination.Tab) && !guildConsumeBankWithdraw(ctx, tx, s.playerGUID, guildID, destination.Tab) {
			return false
		}
		// Guild::_MoveItems step 4 (Guild.cpp:2683-2690):
		// BankMoveItemData::HasWithdrawRights (Guild.cpp:864-876) has no
		// same-tab skip, so even a same-tab bank rearrangement requires
		// remaining withdraw slots != 0. RemoveItem only decrements when the
		// item leaves the tab (Guild.cpp:888-894), so this arm checks without
		// consuming. Silent on failure — _MoveItems returns with no feedback.
		if sourceLoc.Bank && destination.Bank && sourceLoc.Tab == destination.Tab && !guildCheckBankWithdraw(ctx, tx, s.playerGUID, guildID, sourceLoc.Tab) {
			return false
		}
		return true
	}
	var placements []guildMovePlacement
	var storeErr uint8
	if destination != nil && destination.Bank {
		placements, storeErr = s.guildBankMovePlan(ctx, tx, uint64(guildID), destination.Tab, destination.Slot, sourceLoc, source, moveCount, maxStack, full)
	} else {
		// Item::IsBindedNotWith (Item.cpp:1039-1051) runs inside
		// Player::CanStoreItem (Player.cpp:10714-10720) before destination
		// planning, so a soulbound bank item is rejected on the withdraw
		// path. Bank items load with ObjectGuid::Empty owner (Guild.cpp:399;
		// Go deposits set owner_guid = 0), so a soulbound bank item is never
		// owned by the withdrawing player — the BOP-tradeable allowed-looter
		// exemption has no Go model. Guild::_DoItemsMove gates the equip
		// error on sendError (Guild.cpp:2736): the split path (Guild.cpp:2709)
		// and the swap fallback (Guild.cpp:2729) pass true and emit, while
		// the non-split merge attempt (Guild.cpp:2714) passes false and is
		// silent.
		if sourceLoc.Bank && source.Flags&itemInstanceFlagSoulbound != 0 {
			emit := requestedCount != 0 && requestedCount != source.Count
			if destination != nil && !autoStore && destExists && full {
				emit = true
			}
			if emit {
				return rollback(guildEquipErrDontOwnThatItem, source.GUID)
			}
			_ = tx.Rollback()
			return guildMoveOutcome{}, false
		}
		// Player::CanStoreItem (Player.cpp:10731-10742) runs the
		// CanTakeMoreSimilarItems unique/max-count cap before any
		// destination planning. Guild::_DoItemsMove runs the store with
		// sendError=false on this path (Guild.cpp:2714), so a zero-headroom
		// failure is silent — no command result, no equip error.
		capped, capErr := s.guildPlayerSimilarItemCap(ctx, tx, s.playerGUID, source.Entry, moveCount)
		if capErr != 0 {
			_ = tx.Rollback()
			return guildMoveOutcome{}, false
		}
		if capped < moveCount {
			// Partial store: the source keeps the remainder (Go split
			// semantics). C++ truncates the detached bank item to the
			// stored count instead (Player::_StoreItem, Player.cpp:11290),
			// destroying the excess — a data-loss edge not replicated.
			moveCount, full = capped, false
		}
		var target *guildMoveLocation
		if destination != nil && !autoStore {
			target = destination
		}
		placements, storeErr = s.guildPlayerMovePlan(ctx, tx, s.playerGUID, source.Entry, moveCount, maxStack, target)
	}
	if storeErr != 0 && destination != nil && destExists && full {
		// Guild::_MoveItems step 3 (Guild.cpp:2683-2690) ran above for the
		// player->bank direction; the cross-tab bank case is re-checked
		// below, and the swap-arm opposite-direction rights are the
		// silent withdraw-slots check
		// (BankMoveItemData::HasWithdrawRights, Guild.cpp:864-876),
		// enforced by consumeWithdraw(true) below.
		// Guild::_MoveItems step 3 (Guild.cpp:2683-2690): a cross-tab bank
		// swap needs GUILD_BANK_RIGHT_DEPOSIT_ITEM (VIEW_TAB|PUT_ITEM,
		// Guild.h:181) on the dest tab
		// (BankMoveItemData::HasStoreRights, Guild.cpp:855-862; the
		// same-tab skip lives there too). Silent on failure — C++ returns
		// with no feedback on this path.
		if destination.Bank && sourceLoc.Bank && sourceLoc.Tab != destination.Tab && !s.checkGuildBankRights(ctx, guildID, destination.Tab, true) {
			return rollback(0, source.GUID)
		}
		// Guild::_MoveItems step 6.2.2 (Guild.cpp:2722-2729): the swap
		// fallback checks the opposite direction too — the dest item is
		// stored back in the source tab, so
		// BankMoveItemData::HasStoreRights (Guild.cpp:855-862) needs
		// GUILD_BANK_RIGHT_DEPOSIT_ITEM (VIEW_TAB|PUT_ITEM, Guild.h:181)
		// on the src tab (skipped only for same-tab bank swaps, like
		// step 3). Silent on failure — C++ returns with no feedback on
		// this path.
		if sourceLoc.Bank && (!destination.Bank || sourceLoc.Tab != destination.Tab) && !s.checkGuildBankRights(ctx, guildID, sourceLoc.Tab, true) {
			return rollback(0, source.GUID)
		}
		if swapErr := s.guildMoveCanSwap(ctx, sourceLoc, *destination, source, destItem); swapErr != 0 {
			return rollback(swapErr, source.GUID)
		}
		if !sourceLoc.Bank && s.guildItemIsNonemptyBag(ctx, sourceLoc, source) || !destination.Bank && s.guildItemIsNonemptyBag(ctx, *destination, destItem) {
			return rollback(equipErrCanOnlyDoWithEmptyBags, source.GUID)
		}
		if !consumeWithdraw(true) {
			_ = tx.Rollback()
			// Guild::_MoveItems steps 3-4 return silently on rights failure; C++
			// never emits ERR_GUILD_WITHDRAW_LIMIT on this path.
			return guildMoveOutcome{}, false
		}
		if err := guildSwapMoveItems(ctx, tx, uint64(guildID), s.playerGUID, sourceLoc, *destination, source, destItem); err != nil {
			return rollback(0, source.GUID)
		}
		// Guild::_MoveItems (Guild.cpp:2751-2754): pDest->LogAction(pSrc)
		// then pSrc->LogAction(pDest) fire the OnItemMove hook before the
		// store transaction commits.
		s.fireGuildItemMove(ctx, guildID, sourceLoc, *destination, source, moveCount)
		s.fireGuildItemMove(ctx, guildID, *destination, sourceLoc, destItem, destItem.Count)
		if err := tx.Commit(); err != nil {
			return guildMoveOutcome{}, false
		}
		if !sourceLoc.Bank {
			s.adjustQuestItemCount(ctx, source.Entry, source.Count, false)
		}
		if !destination.Bank {
			s.adjustQuestItemCount(ctx, destItem.Entry, destItem.Count, false)
			s.adjustQuestItemCount(ctx, source.Entry, moveCount, true)
		}
		if !sourceLoc.Bank {
			s.adjustQuestItemCount(ctx, destItem.Entry, destItem.Count, true)
		}
		return guildMoveOutcome{Source: source, Destination: destItem, SourceLoc: sourceLoc, DestLoc: *destination, Count: moveCount, Swapped: true}, true
	}
	if storeErr != 0 {
		// Guild::_DoItemsMove (Guild.cpp:2736) takes a sendError flag: the
		// non-split merge attempt (Guild.cpp:2714) passes false, so a store
		// failure is silent — no equip error packet. The split path
		// (Guild.cpp:2709) and the swap fallback (Guild.cpp:2729) pass true
		// and do emit the equip error.
		if requestedCount != 0 && requestedCount != source.Count {
			return rollback(storeErr, source.GUID)
		}
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	// Guild::_MoveItems step 3 (Guild.cpp:2683-2690) runs before step 4's
	// withdraw-slot check: a cross-tab bank move needs
	// GUILD_BANK_RIGHT_DEPOSIT_ITEM (VIEW_TAB|PUT_ITEM, Guild.h:181) on the
	// dest tab (BankMoveItemData::HasStoreRights, Guild.cpp:855-862; the
	// same-tab skip lives there too). Silent on failure — C++ returns with
	// no feedback on this path.
	if destination != nil && destination.Bank && sourceLoc.Bank && sourceLoc.Tab != destination.Tab && !s.checkGuildBankRights(ctx, guildID, destination.Tab, true) {
		_ = tx.Rollback()
		return guildMoveOutcome{}, false
	}
	if !consumeWithdraw(false) {
		_ = tx.Rollback()
		// Guild::_MoveItems steps 3-4 return silently on rights failure; C++
		// never emits ERR_GUILD_WITHDRAW_LIMIT on this path.
		return guildMoveOutcome{}, false
	}
	if err := s.guildApplyMovePlan(ctx, tx, uint64(guildID), s.playerGUID, sourceLoc, source, moveCount, full, placements); err != nil {
		return rollback(0, source.GUID)
	}
	// Guild::_MoveItems (Guild.cpp:2751): pDest->LogAction(pSrc) fires the
	// OnItemMove hook before the store transaction commits.
	destLoc := guildMoveLocation{BagSlot: 0xFF, Slot: 0xFF}
	if destination != nil {
		destLoc = *destination
	}
	s.fireGuildItemMove(ctx, guildID, sourceLoc, destLoc, source, moveCount)
	if err := tx.Commit(); err != nil {
		return guildMoveOutcome{}, false
	}
	if !sourceLoc.Bank && full {
		s.adjustQuestItemCount(ctx, source.Entry, source.Count, false)
	}
	if destination != nil && !destination.Bank {
		s.adjustQuestItemCount(ctx, source.Entry, moveCount, true)
	}
	return guildMoveOutcome{Source: source, SourceLoc: sourceLoc, Count: moveCount, DestLoc: destLoc}, true
}

func guildSwapMoveItems(ctx context.Context, tx *sql.Tx, guildID, playerGUID uint64, sourceLoc, destination guildMoveLocation, source, dest guildMoveItem) error {
	if err := guildMoveClear(ctx, tx, guildID, playerGUID, sourceLoc); err != nil {
		return err
	}
	if err := guildMoveClear(ctx, tx, guildID, playerGUID, destination); err != nil {
		return err
	}
	if err := guildInsertMoveLocation(ctx, tx, guildID, playerGUID, destination, source.GUID); err != nil {
		return err
	}
	if err := guildInsertMoveLocation(ctx, tx, guildID, playerGUID, sourceLoc, dest.GUID); err != nil {
		return err
	}
	ownerSource, ownerDest := guildMoveOwner(destination, playerGUID), guildMoveOwner(sourceLoc, playerGUID)
	if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", ownerSource, source.GUID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", ownerDest, dest.GUID)
	return err
}

var guildBankTabPrices = []uint32{
	100 * 10000,  // Tab 0 (first bought tab): 100 gold
	250 * 10000,  // Tab 1: 250 gold
	500 * 10000,  // Tab 2: 500 gold
	1000 * 10000, // Tab 3: 1000 gold
	2500 * 10000, // Tab 4: 2500 gold
	5000 * 10000, // Tab 5: 5000 gold
}

// maxGuildBankTabTextLen mirrors MAX_GUILD_BANK_TAB_TEXT_LEN (Guild.cpp:41).
const maxGuildBankTabTextLen = 500

// truncateBankTabText mirrors utf8truncate(m_text, MAX_GUILD_BANK_TAB_TEXT_LEN)
// in Guild::BankTab::SetText (Guild.cpp:450-459): the 500-unit limit is counted
// in UTF-16 code units (wstr.resize), not bytes or runes, and invalid UTF-8
// clears the text outright (utf8truncate's catch clears utf8str). The dangling
// lead-surrogate case (a pair straddling exactly unit 500) throws in the C++
// checked conversion, so it clears there too.
func truncateBankTabText(text string) string {
	if !utf8.ValidString(text) {
		return ""
	}
	units := utf16.Encode([]rune(text))
	if len(units) <= maxGuildBankTabTextLen {
		return text
	}
	cut := units[:maxGuildBankTabTextLen]
	if cut[len(cut)-1] >= 0xD800 && cut[len(cut)-1] <= 0xDBFF {
		return ""
	}
	return string(utf16.Decode(cut))
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

	// Reference: WorldSession::HandleGuildBankBuyTab (GuildHandler.cpp:344):
	// the banker-interact gate runs before the guild lookup.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
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

	if int(tabID) >= len(guildBankTabPrices) {
		return true
	}
	// Reference: Guild::HandleBuyBankTab (Guild.cpp:1434-1460): only the
	// next unpurchased tab may be bought, and never past the tab cap.
	var purchasedTabs int64
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs); err != nil {
		return true
	}
	if purchasedTabs >= int64(guildBankMaxTabs) || int64(tabID) != purchasedTabs {
		return true
	}
	cost := guildBankTabPrices[tabID]
	if s.player.Money < cost {
		return true
	}

	s.player.Money -= cost
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

	// Reference: Guild::_CreateNewBankTab (Guild.cpp:2404-2425): the tab-row
	// delete+insert and all per-rank rights inserts run inside one
	// CharacterDatabase transaction, and the tab row carries only the
	// (guildid, TabId) columns == CHAR_INS_GUILD_BANK_TAB — TabName/TabIcon
	// stay at their '' schema defaults and TabText stays NULL until set.
	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		return true
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO guild_bank_tab (guildid, TabId) VALUES (?, ?)", guildID, tabID); err != nil {
		return true
	}

	// Reference: RankInfo::CreateMissingTabsIfNeeded (Guild.cpp:276-300)
	// via GuildBankRightsAndSlots::SetGuildMasterValues (Guild.h:259-263):
	// fresh rights rows start at rights 0/slots 0; only the guildmaster
	// rank (rid 0 == GR_GUILDMASTER) gets FULL rights + unlimited slots.
	var rankIDs []uint32
	if rankRows, err := tx.QueryContext(ctx, "SELECT rid FROM guild_rank WHERE guildid = ?", guildID); err == nil {
		for rankRows.Next() {
			var rid uint32
			if err := rankRows.Scan(&rid); err == nil {
				rankIDs = append(rankIDs, rid)
			}
		}
		rankRows.Close()
	}
	for _, rid := range rankIDs {
		rights := uint8(0)
		slots := uint32(0)
		if rid == 0 {
			rights = guildBankRightFull
			slots = guildWithdrawSlotUnlimited
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO guild_bank_right (guildid, TabId, rid, gbright, SlotPerDay) VALUES (?, ?, ?, ?, ?)",
			guildID, tabID, rid, rights, slots); err != nil {
			return true
		}
	}
	if err := tx.Commit(); err != nil {
		return true
	}

	// Reference: Guild::HandleBuyBankTab (Guild.cpp:1460-1461): broadcast
	// GE_BANK_TAB_PURCHASED to the guild, then re-send permissions to the
	// buyer to force the client to update them.
	s.broadcastGuildBankTabPurchased(uint32(guildID))
	s.sendGuildPermissions(ctx)

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

	// Reference: WorldSession::HandleGuildBankUpdateTab (GuildHandler.cpp:
	// 354): an empty Name or Icon skips the whole handler, before the
	// banker-interact gate.
	if name == "" || icon == "" {
		return true
	}

	// Reference: WorldSession::HandleGuildBankUpdateTab (GuildHandler.cpp:
	// 355): the banker-interact gate runs before the guild lookup.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
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

	// Reference: Guild::HandleSetBankTabInfo (Guild.cpp:1384-1396): a
	// GetBankTab miss (unpurchased tab) logs an error and returns before the
	// broadcast, so the UPDATE touching zero rows must not broadcast; C++
	// broadcasts unconditionally once the tab exists (SetInfo no-ops the DB
	// write when name+icon are unchanged but the event still fires).
	res, execErr := cdb.ExecContext(ctx, "UPDATE guild_bank_tab SET TabName = ?, TabIcon = ? WHERE guildid = ? AND TabId = ?", name, icon, guildID, tabID)
	if execErr != nil {
		return true
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return true
	}

	// Reference: Guild::HandleSetBankTabInfo (Guild.cpp:1384-1396):
	// SetInfo followed by _BroadcastEvent(GE_BANK_TAB_UPDATED (16),
	// Empty, to_string(tabId), tab->GetName(), tab->GetIcon()) to all
	// online members (no guid appended for this type per
	// GuildPackets.cpp:130). The Go-invented trailing re-send of the
	// bank list to the actor is dropped.
	event := guildEventPayload(guildEventBankTabUpdated, 0, fmt.Sprintf("%d", tabID), name, icon)
	s.server.sessionsMu.RLock()
	for target := range s.server.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_GUILD_EVENT), event, true)
	}
	s.server.sessionsMu.RUnlock()

	return true
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
	if err != nil {
		return false
	}
	// Reference: WorldSession::HandleGuildBankDepositMoney (GuildHandler.cpp:
	// 289-290): the banker-interact gate runs before the money checks.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
		return true
	}
	if amount == 0 || s.player.Money < amount {
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
	if err != nil || guildID == 0 {
		return true
	}

	// ScriptMgr::OnGuildMemberDepositMoney (Guild.cpp:1697): fires after
	// validation and before the money transfer — C++ fires before the
	// bank-full check, so the check and the transfer both use the rewritten
	// amount (GuildHooks.cpp OnMemberDepositMoney).
	amount = s.fireGuildMoneyEvent(ctx, scripting.GuildEventOnMoneyDeposit, s.luaGuildObject(ctx, uint32(guildID)), s.luaPlayer(), amount)

	// Guild::HandleMemberDepositMoney (Guild.cpp:1699): refuse deposits that
	// would overflow the bank money cap. C++ promotes m_bankMoney to uint64
	// for the comparison (GUILD_BANK_MONEY_LIMIT is uint64), so the bare
	// uint64 form below is the exact mirror.
	if uint64(bankMoney) > guildBankMoneyLimit-uint64(amount) {
		s.sendGuildCommandResult(guildCmdMoveItem, "", errGuildBankFull)
		return true
	}

	s.player.Money -= amount
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET BankMoney = BankMoney + ? WHERE guildid = ?", amount, guildID)
	s.logGuildBankEvent(ctx, uint32(guildID), 0, guildBankLogDepositMoney, s.playerGUID, amount, 0, 0)
	s.broadcastGuildBankMoneySet(uint32(guildID), bankMoney+int64(amount))

	return true
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
	// Reference: WorldSession::HandleGuildBankWithdrawMoney (GuildHandler.cpp:
	// 300): the packet.Money check runs first, then the banker-interact gate.
	if !s.canInteractWithGameObject(ctx, bankerGUID, gameObjectTypeGuildBank) {
		return true
	}

	// Guild::HandleMemberWithdrawMoney (Guild.cpp:1727): clamp to the
	// player money cap before every downstream check.
	if amount > maxMoneyAmount {
		amount = maxMoneyAmount
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

	// Guild::HandleMemberWithdrawMoney (Guild.cpp:1729-1736): the member-miss
	// and daily-limit arms all return false silently — the session handler
	// (GuildHandler.cpp:295-303) ignores the return value, so no command
	// result is ever sent. ERR_GUILD_WITHDRAW_LIMIT (25) is defined
	// (Guild.h:145) but never emitted by the C++ server.
	if !s.checkGuildBankMoneyWithdraw(ctx, uint32(guildID), amount) {
		return true
	}

	// Player::ModifyMoney (Player.cpp:22821): a withdraw that would push the
	// player past MAX_MONEY_AMOUNT fails with EQUIP_ERR_TOO_MUCH_GOLD — only
	// after the daily-limit check above has passed, matching the C++ order
	// (HandleMemberWithdrawMoney checks the limit silently first, then calls
	// ModifyMoney), and the daily allowance is consumed only on the success
	// path.
	if s.player.Money >= maxMoneyAmount-amount {
		s.sendEquipError(equipErrTooMuchGold, 0)
		return true
	}

	// ScriptMgr::OnGuildMemberWitdrawMoney (Guild.cpp:1742): fires after all
	// validation (bank money, member, daily allowance, player cap above) and
	// before the money transfer; a handler's numeric return rewrites the
	// transferred amount (GuildHooks.cpp OnMemberWitdrawMoney), so every use
	// below — allowance consumption, player credit, bank debit, log,
	// broadcast — runs on the rewritten amount.
	amount = s.fireGuildMoneyEvent(ctx, scripting.GuildEventOnMoneyWithdraw, s.luaGuildObject(ctx, uint32(guildID)), s.luaPlayer(), amount, false)

	s.consumeGuildBankMoneyWithdraw(ctx, uint32(guildID), amount)

	s.player.Money += amount
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET BankMoney = BankMoney - ? WHERE guildid = ?", amount, guildID)
	s.logGuildBankEvent(ctx, uint32(guildID), 0, guildBankLogWithdrawMoney, s.playerGUID, amount, 0, 0)
	s.broadcastGuildBankMoneySet(uint32(guildID), bankMoney-int64(amount))

	return true
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
	// Guild::SendBankLog (Guild.cpp:1817) only answers when tabId <
	// _GetPurchasedTabsSize() or tabId == GUILD_BANK_MAX_TABS (money log);
	// anything else gets no packet at all, and HandleGuildBankLogQuery only
	// reaches SendBankLog when the player is in a guild.
	if tabID > guildBankMaxTabs {
		return true
	}
	if cdb == nil || guildID == 0 {
		return true
	}
	if tabID != guildBankMaxTabs {
		var purchasedTabs int64
		if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_bank_tab WHERE guildid = ?", guildID).Scan(&purchasedTabs); err != nil || int64(tabID) >= purchasedTabs {
			return true
		}
	}

	dbTabID := tabID
	if tabID == guildBankMaxTabs {
		dbTabID = guildBankMoneyLogsTab
	}

	// C++ iterates the in-memory LogHolder, which is ordered oldest-first
	// (Guild.h: "The first element is the oldest entry": DB rows are loaded
	// ORDER BY TimeStamp DESC, LogGuid DESC then emplace_front'd, and new
	// events are emplace_back'd), so the packet lists oldest entries first.
	rows, err := cdb.QueryContext(ctx, "SELECT EventType, PlayerGuid, ItemOrMoney, ItemStackCount, DestTabId, TimeStamp FROM guild_bank_eventlog WHERE guildid = ? AND TabId = ? ORDER BY TimeStamp ASC, LogGuid ASC LIMIT 25", guildID, dbTabID)
	if err != nil {
		return true
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
			entries = append(entries, logRecord{
				eventType:      et,
				playerGUID:     uint64(pGuid),
				itemOrMoney:    iom,
				itemStackCount: cnt,
				destTabID:      dt,
				timeOffset:     now - ts,
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
// handleGuildBankMoneyWithdrawn processes MSG_GUILD_BANK_MONEY_WITHDRAWN.
// Reference: WorldSession::HandleGuildBankMoneyWithdrawn (GuildHandler.cpp:238)
// via Guild::SendMoneyInfo -> Guild::_GetMemberRemainingMoney (Guild.cpp).
// The guildmaster reports (int32)GUILD_WITHDRAW_MONEY_UNLIMITED (-1); other
// ranks report their daily money allowance minus today's withdrawals only
// when the rank holds GR_RIGHT_WITHDRAW_REPAIR or GR_RIGHT_WITHDRAW_GOLD,
// and only when the (uint32 wraparound then int32) difference is > 0.
func (s *session) handleGuildBankMoneyWithdrawn(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	buf := protocol.NewBuffer(8)
	buf.WriteI32(s.guildBankMoneyRemaining(ctx))
	_ = s.write(uint16(protocol.OpcodeMSG_GUILD_BANK_MONEY_WITHDRAWN), buf.Bytes(), true)
	return true
}

// guildBankMoneyRemaining mirrors Guild::_GetMemberRemainingMoney
// (Guild.cpp): the guildmaster reports (int32)GUILD_WITHDRAW_MONEY_UNLIMITED
// (-1); other ranks report their daily money allowance minus today's
// withdrawals only when the rank holds GR_RIGHT_WITHDRAW_REPAIR or
// GR_RIGHT_WITHDRAW_GOLD, and only when the (uint32 wraparound then int32)
// difference is > 0 — otherwise 0.
func (s *session) guildBankMoneyRemaining(ctx context.Context) int32 {
	cdb := s.server.CharactersStore.DB
	if cdb == nil || s.player.GuildID == 0 {
		return 0
	}
	var rank uint32
	if err := cdb.QueryRowContext(ctx, "SELECT rank FROM guild_member WHERE guid = ? AND guildid = ?", s.playerGUID, s.player.GuildID).Scan(&rank); err != nil {
		return 0
	}
	if rank == 0 {
		return -1
	}
	var rights uint32
	if err := cdb.QueryRowContext(ctx, "SELECT rights FROM guild_rank WHERE guildid = ? AND rid = ?", s.player.GuildID, rank).Scan(&rights); err != nil || rights&(guildRightWithdrawRepair|guildRightWithdrawGold) == 0 {
		return 0
	}
	var perDay, withdrawn uint32
	_ = cdb.QueryRowContext(ctx, "SELECT BankMoneyPerDay FROM guild_rank WHERE guildid = ? AND rid = ?", s.player.GuildID, rank).Scan(&perDay)
	_ = cdb.QueryRowContext(ctx, "SELECT money FROM guild_member_withdraw WHERE guid = ?", s.playerGUID).Scan(&withdrawn)
	if r := int32(perDay - withdrawn); r > 0 {
		return r
	}
	return 0
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

	// Reference: WorldSession::HandleGuildBankTextQuery (GuildHandler.cpp:
	// 368) + Guild::SendBankTabText (Guild.cpp:1848-1850): a guildless
	// player gets no packet (the guild lookup fails), and a missing bank
	// tab sends nothing — the packet is only built when GetBankTab
	// returns the tab.
	cdb := s.server.CharactersStore.DB
	if cdb == nil || s.player == nil {
		return true
	}
	var guildID int64
	if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID); err != nil || guildID <= 0 {
		return true
	}
	var text sql.NullString
	if err := cdb.QueryRowContext(ctx, "SELECT TabText FROM guild_bank_tab WHERE guildid = ? AND TabId = ? LIMIT 1", guildID, tabID).Scan(&text); err != nil {
		return true
	}
	tabText := ""
	if text.Valid {
		tabText = text.String
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

	// Reference: Guild::BankTab::SetText (Guild.cpp:450-459): utf8truncate
	// to MAX_GUILD_BANK_TAB_TEXT_LEN before persisting and broadcasting.
	tabText = truncateBankTabText(tabText)

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var guildID int64
		if err := cdb.QueryRowContext(ctx, "SELECT guildid FROM guild_member WHERE guid = ? LIMIT 1", s.playerGUID).Scan(&guildID); err == nil && guildID > 0 {
			// Reference: Guild::SetBankTabText (Guild.cpp:2379-2383):
			// SetText persists the text, then SendText(this, nullptr)
			// broadcasts the GuildBankTextQueryResult (MSG_QUERY_GUILD_BANK_TEXT,
			// tab + text, no guid) to all online guild members. A GetBankTab
			// miss writes nothing and broadcasts nothing.
			if res, execErr := cdb.ExecContext(ctx, "UPDATE guild_bank_tab SET TabText = ? WHERE guildid = ? AND TabId = ?", tabText, guildID, tabID); execErr == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					buf := protocol.NewBuffer(64 + len(tabText))
					buf.WriteU8(tabID)
					buf.WriteCString(tabText)
					event := buf.Bytes()
					s.server.sessionsMu.RLock()
					for target := range s.server.sessions {
						if !target.worldReady.Load() || target.player == nil || target.player.GuildID != uint32(guildID) {
							continue
						}
						_ = target.write(uint16(protocol.OpcodeMSG_QUERY_GUILD_BANK_TEXT), event, true)
					}
					s.server.sessionsMu.RUnlock()
				}
			}
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
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:52 — `recvData >> guidNPC` reads a raw 8-byte
	// GUID (operator>>(ObjectGuid&) is a raw uint64 read, ObjectGuid.cpp:76).
	npcGUID, _ := r.ReadU64()
	_, _ = r.ReadU32() // 0
	_, _ = r.ReadU64() // 0
	name, err := r.ReadCString()
	if err != nil || name == "" {
		return true
	}

	// PetitionsHandler.cpp:85-100: the seller must be an interactable
	// petitioner NPC (UNIT_NPC_FLAG_PETITIONER); C++ only then branches on
	// IsTabardDesigner for the guild path. The arena-charter branch has no
	// Go model, so a petitioner that is not a tabard designer sells nothing
	// here.
	if !s.canInteractWithNPC(ctx, npcGUID, uint64(unitNPCFlagPetitioner)) {
		return true
	}
	if !s.creatureHasNPCFlag(ctx, npcGUID, unitNPCFlagTabardDesigner) {
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

	// PetitionsHandler.cpp:146-158: guild-name-exists and reserved/invalid
	// name gates. IsReservedName has no Go bridge (standing delta, same as
	// the rename path); the exists check rides the guild table.
	var nameTaken int64
	_ = cdb.QueryRowContext(ctx, "SELECT guildid FROM guild WHERE UPPER(name) = UPPER(?) LIMIT 1", name).Scan(&nameTaken)
	if nameTaken > 0 {
		s.sendGuildCommandResult(guildCmdCreate, name, errGuildNameExists)
		return true
	}
	// ObjectMgr::IsValidCharterName (ObjectMgr.cpp:8624): bridgeable length
	// arms (MIN_CHARTER_NAME=2, MAX_CHARTER_NAME=24, counted in runes) ->
	// ERR_GUILD_NAME_INVALID. The strict-mask charset check has no Go bridge
	// (standing delta).
	if rn := []rune(name); len(rn) > 24 || len(rn) < 2 {
		s.sendGuildCommandResult(guildCmdCreate, name, errGuildNameInvalid)
		return true
	}
	// PetitionsHandler.cpp:166-171: the charter must exist in item_template,
	// otherwise BUY_ERR_CANT_FIND_ITEM — checked before the money arm.
	// A missing item_template table means the bridge data is not loaded;
	// the check is skipped rather than failing the purchase.
	var templateEntry int64
	if terr := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE entry = ? LIMIT 1", guildCharterItemID).Scan(&templateEntry); terr != nil && !missingTable(terr) && !isMissingColumn(terr) {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(npcGUID, guildCharterItemID, buyErrCantFindItem), true)
		return true
	}

	cost := uint32(guildCharterCost)
	if s.server != nil {
		cost = s.server.Config.GuildCharterCost
	}
	if s.player.Money < cost {
		// PetitionsHandler.cpp:175: SendBuyError(BUY_ERR_NOT_ENOUGHT_MONEY).
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(npcGUID, guildCharterItemID, buyErrNotEnoughMoney), true)
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

	nextItemGUID := int64(s.server.generateItemGUID())
	if nextItemGUID <= 0 {
		nextItemGUID = 1
	}

	petitionGUID := uint64(nextItemGUID)
	s.player.Money -= cost
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	// PetitionsHandler.cpp:192: charter->SetUInt32Value(ITEM_FIELD_ENCHANTMENT_1_1,
	// charter->GetGUID().GetCounter()) — the petition id (the item GUID
	// counter) is stored in enchantment slot 0 (values[22:58]); the follow-up
	// sendInventoryItems below re-sends the create block with it parsed from
	// here, matching the C++ SendNewItem state.
	enchantments := fmt.Sprintf("%d 0 0", nextItemGUID)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, 5863, ?, ?, 1, 0, '', 0, ?, 0, 0, 0, '')", nextItemGUID, s.playerGUID, s.playerGUID, enchantments)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, 0, ?, ?)", s.playerGUID, freeSlot, nextItemGUID)
	// PetitionsHandler.cpp:197-204: buying a new charter invalidates the
	// owner's previous petition of the same type (RemovePetition). The
	// REPLACE covers the petition row (PK is ownerguid+type); the old
	// petition's signature rows are removed explicitly.
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE ownerguid = ? AND type = 9", s.playerGUID)
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
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:223 — `recvData >> petitionGuid`: raw 8-byte GUID.
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
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // guild GUID (client echo; the reference answers the petition GUID low32)
	// PetitionsHandler.cpp:266 — `recvData >> petitionguid`: raw 8-byte GUID.
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

	// SendPetitionQueryOpcode (PetitionsHandler.cpp:272): field 1 is the
	// petition GUID's low 32 bits (not the client-echoed guild GUID), and
	// the guild arm writes (needed, needed, 0) followed by the zero block
	// (u32x4, u16, u32x3), ten empty strings, u32(0), then
	// u32(type != GUILD_CHARTER_TYPE) == 0 for guild charters.
	needed := uint32(9)
	if s.server != nil {
		needed = s.server.Config.MinPetitionSigns
		if needed > 9 {
			needed = 9
		}
	}
	buf := protocol.NewBuffer(128 + len(name))
	buf.WriteU32(uint32(petitionGUID & 0xFFFFFFFF))
	buf.WriteU64(uint64(ownerGUID))
	buf.WriteCString(name)
	buf.WriteU8(0)
	buf.WriteU32(needed)
	buf.WriteU32(needed)
	buf.WriteU32(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	buf.WriteU16(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	buf.WriteU32(0)
	for i := 0; i < 10; i++ {
		buf.WriteU8(0)
	}
	buf.WriteU32(0)
	buf.WriteU32(0) // type != GUILD_CHARTER_TYPE -> 0 (guild only)
	_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_QUERY_RESPONSE), buf.Bytes(), true)
	return true
}

// handlePetitionSign processes CMSG_PETITION_SIGN (0x1C0).
// Reference: WorldSession::HandleSignPetition (PetitionsHandler.cpp:383).
func (s *session) handlePetitionSign(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:386-387 — `recvData >> petitionGuid` is a raw
	// 8-byte GUID; the u8 unk tail carries no data (Go ignores the tail).
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

	// PetitionsHandler.cpp:401: the owner cannot sign their own petition —
	// silent return, no result packet is sent on this arm.
	if uint64(ownerGUID) == s.playerGUID {
		return true
	}

	// Cross-faction check (PetitionsHandler.cpp:404): skipped when
	// AllowTwoSide.Interaction.Guild is enabled.
	var ownerRace int64
	_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ?", ownerGUID).Scan(&ownerRace)
	if s.server != nil && !s.server.Config.AllowTwoSideInteractionGuild &&
		s.player.Race != 0 && ownerRace != 0 && teamForRace(s.player.Race) != teamForRace(uint8(ownerRace)) {
		s.sendGuildCommandResult(guildCmdCreate, "", errGuildNotAllied)
		return true
	}

	// Already-in-guild / invited arms (PetitionsHandler.cpp:430-438): the
	// reference answers guild command results, not sign results.
	if s.player.GuildID != 0 {
		s.sendGuildCommandResult(guildCmdInvite, s.player.Name, errAlreadyInGuildS)
		return true
	}
	if s.guildInvitedID != 0 {
		s.sendGuildCommandResult(guildCmdInvite, s.player.Name, errAlreadyInvitedToGuildS)
		return true
	}

	// PetitionsHandler.cpp:441: a guild charter (type 9) accepts at most
	// 9 signatures; further signs are silently dropped.
	var signCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM petition_sign WHERE petitionguid = ?", petitionGUID).Scan(&signCount)
	if signCount+1 > 9 {
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
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:592 — `recvData >> petitionguid`: raw 8-byte GUID.
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

	// PetitionsHandler.cpp:596: the owner must actually hold the charter
	// item — turning in a petition row without the item is rejected.
	var hasCharter int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, petitionGUID).Scan(&hasCharter)
	if hasCharter == 0 {
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

	// Guild::Create (Guild.cpp:1130, 1135): AddMember fires OnGuildAddMember
	// for the leader, then OnGuildCreate fires on the created guild — before
	// the signature members are added (PetitionsHandler.cpp:701).
	createGuild := s.luaGuildObject(ctx, uint32(newGuildID))
	s.fireGuildEvent(ctx, scripting.GuildEventOnAddMember, createGuild, s.luaPlayer(), uint32(0))
	s.fireGuildEvent(ctx, scripting.GuildEventOnCreate, createGuild, s.luaPlayer(), guildName)

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
		var signerPlayer *scripting.Object
		if s.server != nil {
			if signerSess := s.server.findSessionByGUID(uint64(signerGUID)); signerSess != nil && signerSess.worldReady.Load() {
				signerPlayer = signerSess.luaPlayer()
				if signerPlayer != nil {
					signerSess.player.GuildID = uint32(newGuildID)
					signerSess.player.GuildRank = 4
					signerSess.sendPlayerUpdate()
				}
			}
		}
		// PetitionsHandler.cpp:701: each signature member is AddMember()ed,
		// firing OnGuildAddMember (Guild.cpp:2272); offline signers reach the
		// hook as nil, like Eluna's Push of a null Player*.
		s.fireGuildEvent(ctx, scripting.GuildEventOnAddMember, createGuild, signerPlayer, uint32(4))
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
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // junk
	// PetitionsHandler.cpp:530-532 — `recvData >> petitionGuid >> offererGuid`:
	// raw 8-byte GUIDs.
	petitionGUID, _ := r.ReadU64()
	targetGUID, _ := r.ReadU64()

	if s.server != nil {
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess == nil || !targetSess.worldReady.Load() {
			return true
		}

		// Cross-faction check (PetitionsHandler.cpp:541): skipped when
		// AllowTwoSide.Interaction.Guild is enabled.
		if s.server != nil && !s.server.Config.AllowTwoSideInteractionGuild &&
			s.player.Race != 0 && targetSess.player.Race != 0 && teamForRace(s.player.Race) != teamForRace(targetSess.player.Race) {
			s.sendGuildCommandResult(guildCmdCreate, "", errGuildNotAllied)
			return true
		}

		// Target already in guild (PetitionsHandler.cpp:576): the reference
		// reports the offerer's name, and also covers the invited arm.
		if targetSess.player.GuildID != 0 {
			s.sendGuildCommandResult(guildCmdInvite, s.player.Name, errAlreadyInGuildS)
			return true
		}
		if targetSess.guildInvitedID != 0 {
			s.sendGuildCommandResult(guildCmdInvite, s.player.Name, errAlreadyInvitedToGuildS)
			return true
		}

		s.sendPetitionShowSignatures(targetSess, petitionGUID)
	}
	return true
}

// handlePetitionShowList processes CMSG_PETITION_SHOWLIST (0x1BB).
// Reference: WorldSession::HandlePetitionShowListOpcode (PetitionsHandler.cpp:408).
func (s *session) handlePetitionShowList(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:745 — `recvData >> guid`: raw 8-byte GUID.
	npcGUID, _ := r.ReadU64()

	// PetitionsHandler.cpp:743-749: the list is served only by an
	// interactable petitioner NPC. Go lists only the guild charter row;
	// the C++ arena-charter rows for non-tabard petitioners have no Go
	// model (documented delta).
	if !s.canInteractWithNPC(ctx, npcGUID, uint64(unitNPCFlagPetitioner)) {
		return true
	}

	reqSigns := uint32(9)
	if s.server != nil && s.server.Config.MinPetitionSigns > 0 {
		reqSigns = s.server.Config.MinPetitionSigns
	} else if s.server != nil && s.server.Config.MinPetitionSigns == 0 {
		reqSigns = 0
	}
	charterCost := uint32(guildCharterCost)
	if s.server != nil {
		charterCost = s.server.Config.GuildCharterCost
	}

	buf := protocol.NewBuffer(24)
	buf.WriteU64(npcGUID)
	buf.WriteU8(1)                   // count = 1 petition item
	buf.WriteU32(1)                  // index = 1
	buf.WriteU32(guildCharterItemID) // 5863 Guild Charter
	buf.WriteU32(CHARTER_DISPLAY_ID) // 16161
	buf.WriteU32(charterCost)        // Guild.CharterCost copper
	buf.WriteU32(0)                  // 0
	buf.WriteU32(reqSigns)           // required signs
	_ = s.write(uint16(protocol.OpcodeSMSG_PETITION_SHOWLIST), buf.Bytes(), true)
	return true
}

// handlePetitionDecline processes MSG_PETITION_DECLINE (0x1C2).
// Reference: WorldSession::HandleDeclinePetition (PetitionsHandler.cpp:493).
func (s *session) handlePetitionDecline(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:496 — `recvData >> petitionguid`: raw 8-byte GUID.
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

	// PetitionsHandler.cpp:493: the decline is only forwarded to the
	// online owner — the decliner gets no echo.
	return true
}

// handlePetitionRename processes MSG_PETITION_RENAME (0x1C1).
// Reference: WorldSession::HandlePetitionRenameGuild (PetitionsHandler.cpp:322).
func (s *session) handlePetitionRename(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	r := protocol.NewReader(payload)
	// PetitionsHandler.cpp:325-326 — `recvData >> petitionGuid`: raw 8-byte
	// GUID, followed by the name string.
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

	// PetitionsHandler.cpp:330: the renamer must hold the charter item.
	var hasCharter int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, petitionGUID).Scan(&hasCharter)
	if hasCharter == 0 {
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
// Reference: WorldSession::HandleTabardVendorActivateOpcode (NPCHandler.cpp:58).
func (s *session) handleTabardVendorActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	vendorGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	// NPCHandler.cpp:62-67: the NPC must be interactable as a tabard
	// designer (UNIT_NPC_FLAG_TABARDDESIGNER), otherwise silent drop.
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagTabardDesigner)) {
		return true
	}

	buf := protocol.NewBuffer(8)
	buf.WriteU64(vendorGUID)
	_ = s.write(uint16(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE), buf.Bytes(), true)
	return true
}

// handleSaveGuildEmblem processes MSG_SAVE_GUILD_EMBLEM (0x1FB).
// Reference: WorldSession::HandleSaveGuildEmblemOpcode (GuildHandler.cpp:205)
// and Guild::HandleSetEmblem (Guild.cpp:1341): guildless -> ERR_GUILDEMBLEM_NOGUILD,
// non-leader -> ERR_GUILDEMBLEM_NOTGUILDMASTER, insufficient money ->
// ERR_GUILDEMBLEM_NOTENOUGHMONEY; on success the emblem is saved,
// ERR_GUILDEMBLEM_SUCCESS is sent, and HandleQuery follows (Guild.cpp:1357).
func (s *session) handleSaveGuildEmblem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 28 {
		return true
	}
	r := protocol.NewReader(payload)
	vendorGUID, _ := r.ReadU64()
	style, _ := r.ReadU32()
	color, _ := r.ReadU32()
	bStyle, _ := r.ReadU32()
	bColor, _ := r.ReadU32()
	bgColor, _ := r.ReadU32()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	// WorldSession::HandleSaveGuildEmblemOpcode (GuildHandler.cpp:220-228):
	// saving requires an interactable tabard-designer NPC
	// (UNIT_NPC_FLAG_TABARDDESIGNER 0x80000); the gate runs before the
	// guild check and reports ERR_GUILDEMBLEM_INVALIDVENDOR.
	if !s.canInteractWithNPC(ctx, vendorGUID, 0x80000) {
		s.sendSaveGuildEmblemResult(guildEmblemInvalidVendor)
		return true
	}

	var guildID, leaderGUID int64
	err := cdb.QueryRowContext(ctx, `SELECT g.guildid, g.leaderguid FROM guild g
		JOIN guild_member gm ON gm.guildid = g.guildid
		WHERE gm.guid = ? LIMIT 1`, s.playerGUID).Scan(&guildID, &leaderGUID)
	if err != nil || guildID == 0 {
		s.sendSaveGuildEmblemResult(guildEmblemNoGuild)
		return true
	}
	if uint64(leaderGUID) != s.playerGUID {
		s.sendSaveGuildEmblemResult(guildEmblemNotGuildMaster)
		return true
	}

	const emblemPrice = 100000 // EMBLEM_PRICE = 10 * GOLD (Guild.cpp:43)
	if s.player.Money < emblemPrice {
		s.sendSaveGuildEmblemResult(guildEmblemNotEnoughMoney)
		return true
	}

	s.player.Money -= emblemPrice
	s.sendPlayerMoneyUpdate()
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE guild SET EmblemStyle = ?, EmblemColor = ?, BorderStyle = ?, BorderColor = ?, BackgroundColor = ? WHERE guildid = ?",
		style, color, bStyle, bColor, bgColor, guildID)

	s.sendSaveGuildEmblemResult(guildEmblemSuccess)
	s.sendGuildQueryResponse(ctx, uint32(guildID))
	return true
}

// sendSaveGuildEmblemResult mirrors Guild::SendSaveEmblemResult (Guild.cpp:117):
// PlayerSaveGuildEmblem::Write (GuildPackets.cpp:454) emits int32(Error).
func (s *session) sendSaveGuildEmblemResult(errCode uint32) {
	buf := protocol.NewBuffer(4)
	buf.WriteI32(int32(errCode))
	_ = s.write(uint16(protocol.OpcodeMSG_SAVE_GUILD_EMBLEM), buf.Bytes(), true)
}

const CHARTER_DISPLAY_ID = 16161
