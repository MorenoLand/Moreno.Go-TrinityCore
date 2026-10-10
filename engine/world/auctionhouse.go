package world

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// TrinityCore AuctionAction enum (AuctionHouseMgr.h:50)
const (
	auctionSellItem uint32 = 0
	auctionCancel   uint32 = 1
	auctionPlaceBid uint32 = 2
)

// TrinityCore AuctionError enum (AuctionHouseMgr.h:37)
const (
	errAuctionOK                uint32 = 0
	errAuctionInventory         uint32 = 1
	errAuctionDatabaseError     uint32 = 2
	errAuctionNotEnoughMoney    uint32 = 3
	errAuctionItemNotFound      uint32 = 4
	errAuctionBidIncrement      uint32 = 7
	errAuctionCantBidOwn        uint32 = 10
	errAuctionRestrictedAccount uint32 = 13
)

// TrinityCore MAX_AUCTION_ITEMS (AuctionHouseMgr.h:34): 160 item slots per
// CMSG_AUCTION_SELL_ITEM (4x 36-slot bags + 16-slot backpack).
const maxAuctionItems uint32 = 160

// TrinityCore MAX_GETALL_RETURN (AuctionHouseMgr.h:35): cap on auctions
// returned by a CMSG_AUCTION_LIST_ITEMS getAll scan.
const maxGetAllReturn uint32 = 55000

// auctionOutBid mirrors AuctionEntry::GetAuctionOutBid
// (AuctionHouseMgr.cpp:879-884): the minimum outbid increment is 5% of the
// current bid, at least 1 copper.
func auctionOutBid(bid uint32) uint32 {
	outbid := bid * 5 / 100
	if outbid == 0 {
		outbid = 1
	}
	return outbid
}

// auctionDeposit ports AuctionHouseMgr::GetAuctionDeposit
// (AuctionHouseMgr.cpp:89-118). The C++ multiplier is DepositRate*0.03 from
// AuctionHouse.dbc; that DBC data is absent on this VM, so the neutral-house
// effective multiplier 0.05 is kept (the DepositRate term stays blocked on
// DBC data, not the rate config).
func auctionDeposit(sellPrice int64, timeHr, count uint32, depositRate float64) uint32 {
	const depositMultiplier = 0.05
	rate := float32(depositRate)
	// C++ GetAuctionDeposit (AuctionHouseMgr.cpp:94): no vendor sell price
	// answers the minimum deposit. AH_MINIMUM_DEPOSIT = 100
	// (AuctionHouseMgr.cpp:42).
	if sellPrice <= 0 {
		return uint32(100 * rate)
	}
	deposit := uint32(float32(sellPrice) * depositMultiplier * rate)
	remainderBase := float32(sellPrice)*depositMultiplier*rate - float32(deposit)
	deposit *= timeHr * count
	i := count
	for i > 0 && remainderBase*float32(i) != float32(uint32(remainderBase*float32(i))) {
		i--
	}
	if i > 0 {
		deposit += uint32(remainderBase * float32(i) * float32(timeHr))
	}
	if minDeposit := uint32(100 * rate); deposit < minDeposit {
		return minDeposit
	}
	return deposit
}

// auctionCharExists mirrors the C++ (player || accId) receiver-exists gates
// (SendAuctionSuccessfulMail AuctionHouseMgr.cpp:230, SendAuctionWonMail :164,
// SendAuctionSalePendingMail :199, SendAuctionExpiredMail :242): a connected
// session, or a characters row with a nonzero account —
// sCharacterCache->GetCharacterAccountIdByGuid (CharacterCache.cpp:233) returns
// 0 for unknown guids. (The auction-bot IsBotChar term has no Go model.)
func (s *session) auctionCharExists(ctx context.Context, guid uint64) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	if s.server.findSessionByGUID(guid) != nil {
		return true
	}
	var accID uint32
	return s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accID) == nil && accID != 0
}

// TrinityCore MailAuctionAnswers enum (AuctionHouseMgr.h:57)
const (
	auctionOutbidded         uint32 = 0
	auctionWon               uint32 = 1
	auctionSuccessful        uint32 = 2
	auctionExpired           uint32 = 3
	auctionCancelledToBidder uint32 = 4
	auctionCanceled          uint32 = 5
	auctionSalePending       uint32 = 6
)

const (
	mailStationeryAuction uint32 = 62
	mailAuctionType       uint8  = 2
	// C++ AuctionHouseIds (AuctionHouseMgr.h:70-72): the house-agnostic
	// model matches CONFIG_ALLOW_TWO_SIDE_INTERACTION_AUCTION, whose
	// auctions all land in the neutral house (7); this is also the
	// sender id C++ writes on auction mails
	// (MailSender(AuctionEntry*) -> GetHouseId(), Mail.cpp:66-67) and
	// the house id on MSG_AUCTION_HELLO.
	defaultAuctionHouseID uint32 = 7
	unitNPCFlagAuctioneer uint32 = 0x00200000
)

type auctionRecord struct {
	ID         uint32
	ItemGUID   uint64
	ItemEntry  uint32
	ItemCount  uint32
	Owner      uint64
	StartBid   uint32
	Buyout     uint32
	Bidder     uint64
	Bid        uint32
	ExpireTime int64
	Deposit    uint32
}

func (s *session) handleAuctionHello(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	s.expireAuctions(ctx)
	reader := protocol.NewReader(payload)
	// CMSG_AUCTION_HELLO carries the auctioneer as a raw 8-byte GUID
	// (AuctionHouseHandler.cpp:42: recvData >> guid — ByteBuffer's
	// operator>>(ObjectGuid&) reads a raw uint64, not a packed GUID).
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	// C++ WorldSession::SendAuctionHello (AuctionHouseHandler.cpp:57-66):
	// below CONFIG_AUCTION_LEVEL_REQ ("LevelReq.Auction", default 1,
	// World.cpp:683) the LANG_AUCTION_REQ (6607) notification fires and the
	// window never opens. (The faction->house-entry lookup and the
	// feign-death strip have no Go model: the AH is house-agnostic.)
	auctionLevelReq := uint32(1)
	if s.server != nil {
		auctionLevelReq = s.server.Config.AuctionLevelReq
	}
	if uint32(s.player.Level) < auctionLevelReq {
		s.sendNotification(fmt.Sprintf("You must reach level %d to use the auction house.", auctionLevelReq))
		return true
	}
	packet := protocol.NewBuffer(13)
	packet.WriteU64(guid)
	packet.WriteU32(defaultAuctionHouseID) // Neutral / Standard AH ID
	packet.WriteU8(1)                      // Enabled
	_ = s.write(uint16(protocol.OpcodeMSG_AUCTION_HELLO), packet.Bytes(), true)
	s.debug("auction hello handled", "account", s.accountName, "auctioneer", guid)
	return true
}

func (s *session) handleAuctionListItems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	s.expireAuctions(ctx)
	reader := protocol.NewReader(payload)
	// Raw 8-byte NPC GUID (AuctionHouseHandler.cpp:747: recvData >> guid).
	auctioneer, _ := reader.ReadU64()
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	listFrom, _ := reader.ReadU32()
	searchedName, _ := reader.ReadCString()

	var levelMin, levelMax, usable, getAll uint8
	var auctionSlotID, auctionMainCategory, auctionSubCategory, quality uint32 = 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF
	if reader.Remaining() >= 2 {
		levelMin, _ = reader.ReadU8()
		levelMax, _ = reader.ReadU8()
	}
	if reader.Remaining() >= 12 {
		auctionSlotID, _ = reader.ReadU32()
		auctionMainCategory, _ = reader.ReadU32()
		auctionSubCategory, _ = reader.ReadU32()
	}
	if reader.Remaining() >= 4 {
		quality, _ = reader.ReadU32()
	}
	if reader.Remaining() >= 1 {
		usable, _ = reader.ReadU8()
	}
	if reader.Remaining() >= 1 {
		getAll, _ = reader.ReadU8()
	}

	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return true
	}

	now := time.Now().Unix()
	cfg := s.server.Config

	// C++ HandleAuctionListItems (AuctionHouseHandler.cpp:794-797): the
	// searched name is converted with Utf8toWStr before any list work; an
	// invalid UTF-8 name aborts the handler silently (no result packet).
	if !utf8.ValidString(searchedName) {
		return true
	}

	// C++ HandleAuctionListItems -> AuctionHouseObject::BuildListAuctionItems
	// (AuctionHouseMgr.cpp:701-740): a getAll request scans the whole house,
	// capped at MAX_GETALL_RETURN, skipping expired auctions and auctions
	// whose item is gone; the scan is throttled per player by
	// Auction.GetAllScanDelay (0 disables the branch, like C++ ignoring getAll
	// when CONFIG_AUCTION_GETALL_DELAY == 0).
	if getAll != 0 && cfg.AuctionGetAllDelay != 0 {
		s.server.auctionGetAllMu.Lock()
		throttleTime, found := s.server.auctionGetAllThrottle[s.playerGUID]
		if !found || throttleTime <= now {
			s.server.auctionGetAllThrottle[s.playerGUID] = now + int64(cfg.AuctionGetAllDelay)
			s.server.auctionGetAllMu.Unlock()
			auctions := scanAuctionRows(ctx, cdb, `SELECT ah.id, ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, ii.count
				FROM auctionhouse AS ah
				INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
				WHERE ah.time > ?
				ORDER BY ah.id LIMIT ?`, now, maxGetAllReturn)
			packet := protocol.NewBuffer(12 + len(auctions)*120)
			packet.WriteU32(uint32(len(auctions)))
			for _, a := range auctions {
				writeAuctionInfo(packet, a)
			}
			packet.WriteU32(uint32(len(auctions)))
			packet.WriteU32(uint32(cfg.AuctionSearchDelay))
			_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_RESULT), packet.Bytes(), true)
			s.debug("auction list items getall sent", "account", s.accountName, "count", len(auctions))
			return true
		}
		s.server.auctionGetAllMu.Unlock()
	}

	whereClauses := []string{"ah.time > ?"}
	args := []interface{}{now}

	if searchedName != "" {
		whereClauses = append(whereClauses, "UPPER(it.name) LIKE UPPER(?)")
		args = append(args, "%"+searchedName+"%")
	}
	if levelMin > 0 {
		whereClauses = append(whereClauses, "it.RequiredLevel >= ?")
		args = append(args, levelMin)
	}
	if levelMax > 0 {
		whereClauses = append(whereClauses, "it.RequiredLevel <= ?")
		args = append(args, levelMax)
	}
	if auctionSlotID != 0xFFFFFFFF {
		whereClauses = append(whereClauses, "(it.InventoryType = ? OR (? = 5 AND it.InventoryType = 20))")
		args = append(args, auctionSlotID, auctionSlotID)
	}
	if auctionMainCategory != 0xFFFFFFFF {
		whereClauses = append(whereClauses, "it.class = ?")
		args = append(args, auctionMainCategory)
	}
	if auctionSubCategory != 0xFFFFFFFF {
		whereClauses = append(whereClauses, "it.subclass = ?")
		args = append(args, auctionSubCategory)
	}
	if quality != 0xFFFFFFFF {
		whereClauses = append(whereClauses, "it.Quality = ?")
		args = append(args, quality)
	}
	if usable != 0 {
		// C++ Player::CanUseItem (Player.cpp:11943-11951): after the level
		// check the faction Flags2 term and the class/race mask terms gate
		// usability. FlagsExtra is the item_template Flags2 column.
		whereClauses = append(whereClauses, "it.RequiredLevel <= ?")
		args = append(args, s.player.Level)
		whereClauses = append(whereClauses, "(it.AllowableClass & ?) != 0")
		args = append(args, playerCreateMask(s.player.Class))
		whereClauses = append(whereClauses, "(it.AllowableRace & ?) != 0")
		args = append(args, playerCreateMask(s.player.Race))
		switch playerTeam(s.player.Race) {
		case teamHorde:
			whereClauses = append(whereClauses, "(it.FlagsExtra & 2) = 0")
		case teamAlliance:
			whereClauses = append(whereClauses, "(it.FlagsExtra & 1) = 0")
		}
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// C++ BuildListAuctionItems (AuctionHouseMgr.cpp:710-713): auctions
	// whose item row is gone are skipped — the item entry/count come from
	// the item_instance join, exactly like C++ CHAR_SEL_AUCTIONS
	// (WorldDatabase.cpp) which selects itemEntry/count from the join.
	countQuery := "SELECT COUNT(*) FROM auctionhouse AS ah INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid LEFT JOIN item_template AS it ON it.entry = ii.itemEntry WHERE " + whereSQL
	var totalCount int64
	if err := cdb.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM auctionhouse WHERE time > ?", time.Now().Unix()).Scan(&totalCount)
		whereSQL = "ah.time > ?"
		args = []interface{}{time.Now().Unix()}
		if searchedName != "" {
			whereSQL += " AND UPPER(it.name) LIKE UPPER(?)"
			args = append(args, "%"+searchedName+"%")
		}
	}

	query := `SELECT ah.id, ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, ii.count
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		LEFT JOIN item_template AS it ON it.entry = ii.itemEntry
		WHERE ` + whereSQL + ` ORDER BY ah.id LIMIT 50 OFFSET ?`
	argsWithOffset := append(args, listFrom)
	rows, err := cdb.QueryContext(ctx, query, argsWithOffset...)
	if err != nil {
		return true
	}
	defer rows.Close()
	var auctions []auctionRecord
	for rows.Next() {
		var id, iGuid, iTmpl, owner, buyout, expTime, bidder, lastBid, startBid, deposit, count int64
		if err := rows.Scan(&id, &iGuid, &iTmpl, &owner, &buyout, &expTime, &bidder, &lastBid, &startBid, &deposit, &count); err == nil {
			auctions = append(auctions, auctionRecord{
				ID:         uint32(id),
				ItemGUID:   uint64(iGuid),
				ItemEntry:  uint32(iTmpl),
				ItemCount:  uint32(count),
				Owner:      uint64(owner),
				Buyout:     uint32(buyout),
				ExpireTime: expTime,
				Bidder:     uint64(bidder),
				Bid:        uint32(lastBid),
				StartBid:   uint32(startBid),
				Deposit:    uint32(deposit),
			})
		}
	}
	packet := protocol.NewBuffer(12 + len(auctions)*120)
	packet.WriteU32(uint32(len(auctions))) // Count
	for _, a := range auctions {
		writeAuctionInfo(packet, a)
	}
	packet.WriteU32(uint32(totalCount))             // Total count
	packet.WriteU32(uint32(cfg.AuctionSearchDelay)) // Auction.SearchDelay
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_RESULT), packet.Bytes(), true)
	s.debug("auction list items sent", "account", s.accountName, "count", len(auctions), "total", totalCount)
	return true
}

func (s *session) handleAuctionSellItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	// Raw 8-byte auctioneer GUID (AuctionHouseHandler.cpp:122: recvData >> auctioneer).
	// The NPC interact check sits AFTER the max-money gate in C++
	// (AuctionHouseHandler.cpp:164-169), not up front — see below.
	auctioneer, _ := reader.ReadU64()
	itemCount, err := reader.ReadU32()
	if err != nil {
		return false
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:132-138): more than
	// MAX_AUCTION_ITEMS answers ERR_AUCTION_DATABASE_ERROR with auction id 0.
	if itemCount > maxAuctionItems {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, errAuctionDatabaseError), true)
		return true
	}
	itemGUIDs := make([]int64, 0, itemCount)
	stackCounts := make([]uint32, 0, itemCount)
	for i := uint32(0); i < itemCount; i++ {
		// Raw 8-byte item GUIDs (AuctionHouseHandler.cpp:139: recvData >> itemGUIDs[i]).
		rawItemGUID, rerr := reader.ReadU64()
		if rerr != nil {
			return false
		}
		itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
		if itemGUID == 0 {
			itemGUID = int64(rawItemGUID)
		}
		stackCount, rerr := reader.ReadU32()
		if rerr != nil {
			return false
		}
		// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:144-150): a zero
		// guid, a zero count, or a count above 1000 is silently dropped.
		if itemGUID == 0 || stackCount == 0 || stackCount > 1000 {
			return true
		}
		itemGUIDs = append(itemGUIDs, itemGUID)
		stackCounts = append(stackCounts, stackCount)
	}
	bid, _ := reader.ReadU32()
	buyout, _ := reader.ReadU32()
	etime, _ := reader.ReadU32() // minutes
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:154-155): a zero
	// bid or zero duration is silently dropped.
	if bid == 0 || etime == 0 {
		return true
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:157-162): a bid or
	// buyout above MAX_MONEY_AMOUNT answers ERR_AUCTION_DATABASE_ERROR.
	if bid > maxMoneyAmount || buyout > maxMoneyAmount {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, errAuctionDatabaseError), true)
		return true
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:164-169): the NPC
	// interact check fires here, AFTER the max-money gate — an over-limit
	// bid answers ERR_AUCTION_DATABASE_ERROR even when the player is
	// nowhere near an auctioneer. (The faction->house-entry lookup at
	// :171-177 has no Go model: the AH is house-agnostic.)
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:175-184): the client
	// duration is in minutes; only 12/24/48 hours are accepted, anything else
	// is silently dropped.
	switch etime {
	case 720, 1440, 2880:
	default:
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	sellFail := func(result uint32) bool {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, result), true)
		return true
	}
	var itemEntry int64
	var finalCount uint32
	itemCounts := make([]int64, len(itemGUIDs))
	for j, itemGUID := range itemGUIDs {
		var entry, have, flags, duration int64
		qerr := cdb.QueryRowContext(ctx, "SELECT itemEntry, count, flags, duration FROM item_instance WHERE guid = ? AND owner_guid = ? LIMIT 1", itemGUID, s.playerGUID).Scan(&entry, &have, &flags, &duration)
		// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:199-204): a
		// missing item answers ERR_AUCTION_ITEM_NOT_FOUND with auction id 0.
		if qerr != nil {
			return sellFail(errAuctionItemNotFound)
		}
		if j == 0 {
			itemEntry = entry
		}
		var listed int
		// sAuctionMgr->GetAItem (AuctionHouseHandler.cpp:207): an item that is
		// already on auction cannot be listed again.
		alreadyListed := cdb.QueryRowContext(ctx, "SELECT 1 FROM auctionhouse WHERE itemguid = ? LIMIT 1", itemGUID).Scan(&listed) == nil
		// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:207-212): an
		// already-listed item, a soulbound item (the representable slice of
		// Item::CanBeTraded — Go has no loot/bag/enchant state), a temporary
		// item, a short stack, or a mixed entry all answer
		// ERR_AUCTION_DATABASE_ERROR with auction id 0.
		if alreadyListed || flags&1 != 0 || duration != 0 || have < int64(stackCounts[j]) || entry != itemEntry {
			return sellFail(errAuctionDatabaseError)
		}
		itemCounts[j] = have
		finalCount += stackCounts[j]
	}
	// item->GetTemplate()->HasFlag(ITEM_FLAG_CONJURED)
	// (AuctionHouseHandler.cpp:208, ITEM_FLAG_CONJURED = 0x2,
	// ItemTemplate.h:153): all listed items share one entry, so one template
	// lookup covers the loop.
	var tmplFlags int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(Flags, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&tmplFlags)
	}
	if tmplFlags&2 != 0 {
		return sellFail(errAuctionDatabaseError)
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:218-222): a zero
	// total count answers ERR_AUCTION_DATABASE_ERROR.
	if finalCount == 0 {
		return sellFail(errAuctionDatabaseError)
	}
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:225-234): listing the
	// same item guid twice is a cheat attempt and answers
	// ERR_AUCTION_DATABASE_ERROR.
	seenGUID := make(map[int64]struct{}, len(itemGUIDs))
	for _, itemGUID := range itemGUIDs {
		if _, dup := seenGUID[itemGUID]; dup {
			return sellFail(errAuctionDatabaseError)
		}
		seenGUID[itemGUID] = struct{}{}
	}
	// Item::GetMaxStackCount (Item.h:119) via
	// ItemTemplate::GetMaxStackSize (ItemTemplate.h:686-689): a stackable of
	// 2147483647 or <= 0 means effectively uncapped.
	var stackable int64 = 1
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(stackable, 1) FROM item_template WHERE entry = ?", itemEntry).Scan(&stackable)
	}
	maxStack := stackable
	if stackable == 2147483647 || stackable <= 0 {
		maxStack = 0x7FFFFFFF - 1
	}
	if maxStack < int64(finalCount) {
		return sellFail(errAuctionDatabaseError)
	}

	var sellPrice int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SellPrice, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&sellPrice)
	}
	// C++ HandleAuctionSellItem passes etime in seconds to GetAuctionDeposit
	// (AuctionHouseHandler.cpp:257); C++ timeHr is (((time/60)/60)/12), and
	// Go's etime is in minutes, so (etime/60)/12 evaluates identically on the
	// 720/1440/2880 minutes the switch above accepts.
	timeHr := (etime / 60) / 12
	deposit := auctionDeposit(sellPrice, timeHr, finalCount, s.server.Config.AuctionDepositRate)

	if s.player.Money < deposit {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, errAuctionNotEnoughMoney), true)
		return true
	}
	s.player.Money -= deposit
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	auctionItemGUID := itemGUIDs[0]
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:296-341): a single
	// item sold whole is moved onto the auction; otherwise the stacks are
	// cloned into one merged item (AuctionHouseHandler.cpp:343-425).
	if itemCount != 1 || itemCounts[0] != int64(stackCounts[0]) {
		newGUID := int64(s.server.generateItemGUID())
		if newGUID <= 0 {
			return sellFail(errAuctionDatabaseError)
		}
		if _, err = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, count) VALUES (?, ?, ?, ?)", newGUID, itemEntry, s.playerGUID, finalCount); err != nil {
			return sellFail(errAuctionDatabaseError)
		}
		for j, itemGUID := range itemGUIDs {
			if itemCounts[j] == int64(stackCounts[j]) {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_refund_instance WHERE item_guid = ?", itemGUID)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
				s.despawnItem(uint64(itemGUID))
			} else {
				_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", stackCounts[j], itemGUID)
			}
		}
		auctionItemGUID = newGUID
	} else {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, auctionItemGUID)
		s.despawnItem(uint64(auctionItemGUID))
		// Player::MoveItemFromInventory (Player.cpp:12588-12595): posting to
		// the auction runs Item::SetNotRefundable — flag cleared, refund row
		// deleted.
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET flags = flags & ? WHERE guid = ?", ^int64(itemInstanceFlagRefundable), auctionItemGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM item_refund_instance WHERE item_guid = ?", auctionItemGUID)
	}
	s.adjustQuestItemCount(ctx, uint32(itemEntry), finalCount, false)
	now := time.Now().Unix()
	// C++ HandleAuctionSellItem (AuctionHouseHandler.cpp:254): auctionTime =
	// uint32(etime_seconds * Rate.Auction.Time); the auction's expire time is
	// the current time plus that scaled duration. Go's etime is in minutes.
	auctionTime := uint32(float32(etime*60) * float32(s.server.Config.AuctionTimeRate))
	expire := now + int64(auctionTime)
	var nextID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM auctionhouse").Scan(&nextID)
	// C++ AuctionEntry::SaveToDB (AuctionHouseMgr.cpp:899-912): the auction
	// row carries no item columns — entry and count resolve from the
	// item_instance join (CHAR_SEL_AUCTIONS); Flags starts at
	// AUCTION_ENTRY_FLAG_NONE. Column order matches the C++ bind order:
	// Id, houseId, itemGUIDLow, owner, buyout, expire_time, bidder, bid,
	// startbid, deposit, Flags.
	_, _ = cdb.ExecContext(ctx, `INSERT INTO auctionhouse (id, houseid, itemguid, itemowner, buyoutprice, time, buyguid, lastbid, startbid, deposit, Flags)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, 0)`, nextID, defaultAuctionHouseID, auctionItemGUID, s.playerGUID, buyout, expire, bid, deposit)
	s.updateAchievementCriteria(criteriaTypeCreateAuction, 0, 1)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(uint32(nextID), auctionSellItem, errAuctionOK), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	s.debug("auction created", "account", s.accountName, "auction_id", nextID, "item", itemEntry, "deposit", deposit)
	return true
}

func (s *session) handleAuctionPlaceBid(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	// Raw 8-byte auctioneer GUID (AuctionHouseHandler.cpp:432: recvData >> auctioneer).
	auctioneer, _ := reader.ReadU64()
	auctionID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	price, err := reader.ReadU32()
	if err != nil {
		return false
	}
	// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:436): cheater guard,
	// silently dropped.
	if auctionID == 0 || price == 0 {
		return true
	}
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemGUID, itemEntry, ownerGUID, buyout, bidderGUID, lastBid, deposit, startBid, itemCount, houseID int64
	err = cdb.QueryRowContext(ctx, `SELECT ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.buyguid, ah.lastbid, ah.deposit, ah.startbid, ii.count, ah.houseid
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.id = ? LIMIT 1`, auctionID).Scan(&itemGUID, &itemEntry, &ownerGUID, &buyout, &bidderGUID, &lastBid, &deposit, &startBid, &itemCount, &houseID)
	if err != nil {
		// C++ answers ERR_AUCTION_BID_OWN with a zero auction id when the
		// auction is missing (AuctionHouseHandler.cpp:459).
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionPlaceBid, errAuctionCantBidOwn), true)
		return true
	}

	// Player cannot bid on their own auction
	if ownerGUID == int64(s.playerGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionPlaceBid, errAuctionCantBidOwn), true)
		return true
	}

	// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:462-470): the
	// owner's offline characters on the same account cannot bid either.
	if s.server.findSessionByGUID(uint64(ownerGUID)) == nil {
		var ownerAccount uint32
		if qerr := cdb.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", ownerGUID).Scan(&ownerAccount); qerr == nil && ownerAccount == s.accountID {
			_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionPlaceBid, errAuctionCantBidOwn), true)
			return true
		}
	}

	// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:473): cheating,
	// silently dropped.
	if price <= uint32(lastBid) || price < uint32(startBid) {
		return true
	}

	// Min increment check if not buyout (C++ AuctionHouseHandler.cpp:476-482),
	// silently dropped — the client tests it.
	isBuyout := buyout > 0 && price >= uint32(buyout)
	if !isBuyout && price < uint32(lastBid)+auctionOutBid(uint32(lastBid)) {
		return true
	}

	// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:484-489): the money
	// check is on the full price and is silently dropped — the client tests it.
	if s.player.Money < price {
		return true
	}

	now := time.Now().Unix()
	// C++ AuctionHouseMgr::SendAuctionSuccessfulMail (AuctionHouseMgr.cpp:230) passes
	// CONFIG_MAIL_DELIVERY_DELAY as the successful-mail deliver_delay, and
	// SendAuctionSalePendingMail (AuctionHouseMgr.cpp:199,203) reads the same config
	// for the invoice body/timePacker fields (worldserver.conf "MailDeliveryDelay", default 3600).
	mailDelay := int64(s.server.Config.MailDeliveryDelay)

	s.setAchievementCriteria(criteriaTypeHighestAuctionBid, 0, price)
	if isBuyout {
		// Buyout success!
		// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:522-526): the
		// buyer pays the buyout, or only the buyout-minus-bid difference when
		// topping up their own bid.
		if bidderGUID == int64(s.playerGUID) {
			s.player.Money -= uint32(buyout) - uint32(lastBid)
		} else {
			s.player.Money -= uint32(buyout)
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

		_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionhouse WHERE id = ?", auctionID)
		// C++ AuctionEntry::DeleteFromDB (AuctionHouseMgr.cpp:890-896):
		// the auction's bidder rows die with it.
		_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionbidders WHERE id = ?", auctionID)

		// Consignment cut (5%) and profit (bid + deposit - cut)
		consignment := uint32(buyout) * 5 / 100
		profit := uint32(buyout) + uint32(deposit) - consignment
		if s.server != nil {
			if sellerSess := s.server.findSessionByGUID(uint64(ownerGUID)); sellerSess != nil {
				sellerSess.setAchievementCriteria(criteriaTypeHighestAuctionSold, 0, uint32(buyout))
				sellerSess.updateAchievementCriteria(criteriaTypeGoldEarnedAuctions, 0, profit)
			}
		}

		// 1. If previous bidder existed and was not current buyer, refund them
		if bidderGUID > 0 && bidderGUID != int64(s.playerGUID) && lastBid > 0 {
			refundMailID := s.server.generateMailID()
			outbidSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionOutbidded, auctionID, itemCount)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
				refundMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, outbidSubj, now+30*86400, now, lastBid)
			s.sendMailNotify(uint64(bidderGUID))
			// C++ SendAuctionOutbiddedMail (AuctionHouseMgr.cpp:263-280): the
			// notification carries the auction house id, the NEW bidder's
			// guid, the new price, and the outbid increment of the OLD bid
			// (evaluated before auction->bid is updated).
			s.notifyAuctionBidder(uint64(bidderGUID), uint64(s.playerGUID), uint32(houseID), auctionID, uint32(buyout), auctionOutBid(uint32(lastBid)), uint32(itemEntry))
		}

		// C++ HandleAuctionPlaceBid buyout (AuctionHouseHandler.cpp:539-569): the
		// mails go out invoice -> profit -> won, each existence-gated —
		// SendAuctionSalePendingMail and SendAuctionSuccessfulMail only when
		// the owner exists (AuctionHouseMgr.cpp:199,230), SendAuctionWonMail
		// only when the item exists (:120-122). The buyer is the connected
		// session here, so the bidder gate (:164) always holds.

		// 1. Send auction invoice / sale pending notice mail to seller (immediate delivery, expires in MailDeliveryDelay)
		if s.auctionCharExists(ctx, uint64(ownerGUID)) {
			invoiceMailID := s.server.generateMailID()
			pendingSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionSalePending, auctionID, itemCount)
			// C++ AuctionHouseMgr::SendAuctionSalePendingMail (AuctionHouseMgr.cpp:199,203):
			// the trailing eta field is timePacker.read<uint32>() after
			// AppendPackedTime(now + CONFIG_MAIL_DELIVERY_DELAY).
			pendingEta := protocol.PackTime(time.Unix(now+mailDelay, 0))
			pendingBody := fmt.Sprintf("%X:%d:%d:%d:%d:%d:%d", s.playerGUID, buyout, buyout, deposit, consignment, mailDelay, pendingEta)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, 0, 0, 4)",
				// C++ Mail.cpp:203-204: auction mail without items and money expires after CONFIG_MAIL_DELIVERY_DELAY;
				// the moneyDelay/eta body fields come from AuctionHouseMgr::SendAuctionSalePendingMail
				// (AuctionHouseMgr.cpp:199,203), same config.
				invoiceMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, pendingSubj, pendingBody, now+mailDelay, now)
			s.sendMailNotify(uint64(ownerGUID))
		}

		// 2. Send profit mail to seller (delayed by MailDeliveryDelay, default 1 hour)
		if s.auctionCharExists(ctx, uint64(ownerGUID)) {
			sellerMailID := s.server.generateMailID()
			succSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionSuccessful, auctionID, itemCount)
			succBody := fmt.Sprintf("%X:%d:%d:%d:%d", s.playerGUID, buyout, buyout, deposit, consignment)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, ?, 0, 4)",
				// C++ Mail.cpp:197,215: expire_time = deliver_time + expire_delay (30d: money but no items/COD -> else branch);
				// deliver_delay comes from AuctionHouseMgr::SendAuctionSuccessfulMail (AuctionHouseMgr.cpp:230),
				// which passes CONFIG_MAIL_DELIVERY_DELAY ("MailDeliveryDelay", default 3600).
				sellerMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, succSubj, succBody, now+mailDelay+30*86400, now+mailDelay, profit)
			s.sendMailNotify(uint64(ownerGUID))
			s.notifyAuctionOwner(uint64(ownerGUID), auctionID, uint32(buyout), uint32(itemEntry))
		}

		// 3. Send won mail with item to buyer (immediate delivery)
		var itemProbe int
		if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM item_instance WHERE guid = ? LIMIT 1", itemGUID).Scan(&itemProbe); err == nil {
			nextMailID := s.server.generateMailID()
			wonSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionWon, auctionID, itemCount)
			wonBody := fmt.Sprintf("%X:%d:%d", ownerGUID, buyout, buyout)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 1, ?, ?, 0, 0, 4)",
				nextMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, s.playerGUID, wonSubj, wonBody, now+30*86400, now)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", nextMailID, itemGUID, s.playerGUID)
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, itemGUID)
			s.sendMailNotify(uint64(s.playerGUID))
			// C++ SendAuctionWonMail (AuctionHouseMgr.cpp:170-174): the
			// connected winning bidder gets the bidder notification (zeroed
			// sums) and the won-auctions achievement; the notification restores
			// the shape the 18:41 run removed with misaligned args.
			s.sendAuctionBidderNotification(uint32(houseID), auctionID, uint64(s.playerGUID), 0, 0, uint32(itemEntry))
			s.updateAchievementCriteria(criteriaTypeWonAuctions, 0, 1)
		}

		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionOK), true)
	} else {
		// Normal Bid / Outbid previous bidder
		// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:505-511): a
		// bidder raising their own bid pays only the difference.
		if bidderGUID == int64(s.playerGUID) {
			s.player.Money -= price - uint32(lastBid)
		} else {
			s.player.Money -= price
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

		if bidderGUID != 0 && lastBid > 0 && bidderGUID != int64(s.playerGUID) {
			// Refund previous bidder via mail
			refundMailID := s.server.generateMailID()
			outbidSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionOutbidded, auctionID, itemCount)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
				refundMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, outbidSubj, now+30*86400, now, lastBid)
			s.sendMailNotify(uint64(bidderGUID))
			// C++ SendAuctionOutbiddedMail (AuctionHouseMgr.cpp:263-280): the
			// notification carries the auction house id, the NEW bidder's
			// guid, the new price, and the outbid increment of the OLD bid
			// (evaluated before auction->bid is updated).
			s.notifyAuctionBidder(uint64(bidderGUID), uint64(s.playerGUID), uint32(houseID), auctionID, price, auctionOutBid(uint32(lastBid)), uint32(itemEntry))
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE auctionhouse SET buyguid = ?, lastbid = ? WHERE id = ?", s.playerGUID, price, auctionID)
		// C++ HandleAuctionPlaceBid (AuctionHouseHandler.cpp:526-535): the
		// bidder joins the auction's bidder set once, persisted in
		// auctionbidders; the IGNORE insert is the no-op-if-present guard
		// for the in-memory set check.
		insertIgnore := "INSERT OR IGNORE"
		if s.server.CharactersStore.Backend != database.BackendSQLite {
			insertIgnore = "INSERT IGNORE"
		}
		_, _ = cdb.ExecContext(ctx, insertIgnore+" INTO auctionbidders (id, bidderguid) VALUES (?, ?)", auctionID, s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionOK), true)
	}
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	s.debug("auction bid placed", "account", s.accountName, "auction_id", auctionID, "price", price)
	return true
}

func (s *session) handleAuctionListOwnerItems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.expireAuctions(ctx)
	// C++ HandleAuctionListOwnerItems (AuctionHouseHandler.cpp:708-727):
	// the auctioneer GUID comes first in the packet and the NPC interact
	// check runs before anything else.
	if len(payload) < 1 {
		return true
	}
	// Raw 8-byte auctioneer GUID (AuctionHouseHandler.cpp:712: recvData >> guid).
	auctioneer, _ := protocol.NewReader(payload).ReadU64()
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, ii.count
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.itemowner = ? AND ah.time > ?`, s.playerGUID, time.Now().Unix())
	if err != nil {
		return true
	}
	defer rows.Close()
	var auctions []auctionRecord
	for rows.Next() {
		var id, iGuid, iTmpl, owner, buyout, expTime, bidder, lastBid, startBid, deposit, count int64
		if err := rows.Scan(&id, &iGuid, &iTmpl, &owner, &buyout, &expTime, &bidder, &lastBid, &startBid, &deposit, &count); err == nil {
			auctions = append(auctions, auctionRecord{
				ID:         uint32(id),
				ItemGUID:   uint64(iGuid),
				ItemEntry:  uint32(iTmpl),
				ItemCount:  uint32(count),
				Owner:      uint64(owner),
				Buyout:     uint32(buyout),
				ExpireTime: expTime,
				Bidder:     uint64(bidder),
				Bid:        uint32(lastBid),
				StartBid:   uint32(startBid),
				Deposit:    uint32(deposit),
			})
		}
	}
	packet := protocol.NewBuffer(12 + len(auctions)*120)
	packet.WriteU32(uint32(len(auctions)))
	for _, a := range auctions {
		writeAuctionInfo(packet, a)
	}
	packet.WriteU32(uint32(len(auctions)))
	packet.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_OWNER_LIST_RESULT), packet.Bytes(), true)
	return true
}

func (s *session) handleAuctionListBidderItems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.expireAuctions(ctx)
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var outbidIDs []uint32
	if len(payload) >= 1 {
		r := protocol.NewReader(payload)
		// Raw 8-byte auctioneer GUID (AuctionHouseHandler.cpp:657: recvData >> guid).
		auctioneer, _ := r.ReadU64()
		_, _ = r.ReadU32() // listFrom, unused
		outbiddedCount, _ := r.ReadU32()
		// C++ HandleAuctionListBidderItems (AuctionHouseHandler.cpp:659-663):
		// a bad packet size zeroes the outbidded count instead of trusting it.
		// The constant 16 is C++'s own: 8-byte raw GUID + u32 listFrom + u32
		// outbiddedCount; int64 arithmetic keeps short packets on the
		// zero-the-count path instead of underflowing.
		if int64(outbiddedCount)*4 != int64(len(payload))-16 {
			outbiddedCount = 0
		}
		if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
			return true
		}
		for i := uint32(0); i < outbiddedCount; i++ {
			id, err := r.ReadU32()
			if err == nil && id > 0 {
				outbidIDs = append(outbidIDs, id)
			}
		}
	} else {
		return true
	}

	now := time.Now().Unix()
	// C++ AuctionHouseObject::BuildListBidderItems (AuctionHouseMgr.cpp:671-
	// 680): the bidder list covers every auction the player has bid on (the
	// auctionbidders set), not just ones where they are the current top
	// bidder.
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, ii.count
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.id IN (SELECT id FROM auctionbidders WHERE bidderguid = ?) AND ah.time > ?`, s.playerGUID, now)
	if err != nil {
		return true
	}
	defer rows.Close()
	var auctions []auctionRecord
	seenIDs := make(map[uint32]bool)
	for rows.Next() {
		var id, iGuid, iTmpl, owner, buyout, expTime, bidder, lastBid, startBid, deposit, count int64
		if err := rows.Scan(&id, &iGuid, &iTmpl, &owner, &buyout, &expTime, &bidder, &lastBid, &startBid, &deposit, &count); err == nil {
			seenIDs[uint32(id)] = true
			auctions = append(auctions, auctionRecord{
				ID:         uint32(id),
				ItemGUID:   uint64(iGuid),
				ItemEntry:  uint32(iTmpl),
				ItemCount:  uint32(count),
				Owner:      uint64(owner),
				Buyout:     uint32(buyout),
				ExpireTime: expTime,
				Bidder:     uint64(bidder),
				Bid:        uint32(lastBid),
				StartBid:   uint32(startBid),
				Deposit:    uint32(deposit),
			})
		}
	}

	// Also append outbidded auctions if requested by client
	for _, oid := range outbidIDs {
		if seenIDs[oid] {
			continue
		}
		var id, iGuid, iTmpl, owner, buyout, expTime, bidder, lastBid, startBid, deposit, count int64
		if err := cdb.QueryRowContext(ctx, `SELECT ah.id, ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, ii.count
			FROM auctionhouse AS ah
			INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
			WHERE ah.id = ? AND ah.time > ? LIMIT 1`, oid, now).Scan(&id, &iGuid, &iTmpl, &owner, &buyout, &expTime, &bidder, &lastBid, &startBid, &deposit, &count); err == nil {
			seenIDs[uint32(id)] = true
			auctions = append(auctions, auctionRecord{
				ID:         uint32(id),
				ItemGUID:   uint64(iGuid),
				ItemEntry:  uint32(iTmpl),
				ItemCount:  uint32(count),
				Owner:      uint64(owner),
				Buyout:     uint32(buyout),
				ExpireTime: expTime,
				Bidder:     uint64(bidder),
				Bid:        uint32(lastBid),
				StartBid:   uint32(startBid),
				Deposit:    uint32(deposit),
			})
		}
	}

	packet := protocol.NewBuffer(12 + len(auctions)*120)
	packet.WriteU32(uint32(len(auctions)))
	for _, a := range auctions {
		writeAuctionInfo(packet, a)
	}
	packet.WriteU32(uint32(len(auctions)))
	packet.WriteU32(300)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_BIDDER_LIST_RESULT), packet.Bytes(), true)
	return true
}

func (s *session) handleAuctionRemoveItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	// Raw 8-byte auctioneer GUID (AuctionHouseHandler.cpp:580: recvData >> auctioneer).
	auctioneer, _ := reader.ReadU64()
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	auctionID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemGUID, itemEntry, ownerGUID, bidderGUID, lastBid, itemCount int64
	err = cdb.QueryRowContext(ctx, `SELECT ah.itemguid, ii.itemEntry, ah.itemowner, ah.buyguid, ah.lastbid, ii.count
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.id = ? AND ah.itemowner = ? LIMIT 1`, auctionID, s.playerGUID).Scan(&itemGUID, &itemEntry, &ownerGUID, &bidderGUID, &lastBid, &itemCount)
	if err != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionCancel, errAuctionDatabaseError), true)
		return true
	}

	// C++ HandleAuctionRemoveItem (AuctionHouseHandler.cpp:575-633): the
	// auctioned item must be in the item map — a missing item answers
	// ERR_AUCTION_DATABASE_ERROR with auction id 0 and the auction is kept.
	var itemProbe int
	if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM item_instance WHERE guid = ? LIMIT 1", itemGUID).Scan(&itemProbe); err != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionCancel, errAuctionDatabaseError), true)
		return true
	}

	now := time.Now().Unix()

	// C++ HandleAuctionRemoveItem (AuctionHouseHandler.cpp:599-610): with an
	// active bidder the seller pays the 5% auction cut; insufficient money
	// silently aborts the cancel — no command result is sent, and the
	// bidder-refund mail goes out before the cut is taken.
	if bidderGUID > 0 && lastBid > 0 {
		auctionCut := uint32(lastBid) * 5 / 100
		if s.player.Money < auctionCut {
			return true
		}
		bidderMailID := s.server.generateMailID()
		bidderSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionCancelledToBidder, auctionID, itemCount)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
			bidderMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, bidderSubj, now+30*86400, now, lastBid)
		s.sendMailNotify(uint64(bidderGUID))
		s.player.Money -= auctionCut
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		s.sendPlayerMoneyUpdate()
		s.sendPlayerUpdate()
	}

	// Mail item back to owner
	nextMailID := s.server.generateMailID()
	cancelSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionCanceled, auctionID, itemCount)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 1, ?, ?, 0, 0, 4)",
		nextMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, cancelSubj, now+30*86400, now)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", nextMailID, itemGUID, ownerGUID)
	s.sendMailNotify(uint64(ownerGUID))

	_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionhouse WHERE id = ?", auctionID)
	// C++ AuctionEntry::DeleteFromDB (AuctionHouseMgr.cpp:890-896):
	// the auction's bidder rows die with it.
	_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionbidders WHERE id = ?", auctionID)

	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionCancel, errAuctionOK), true)
	return true
}

func (s *session) expireAuctions(ctx context.Context) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	now := time.Now().Unix()
	mailDelay := int64(s.server.Config.MailDeliveryDelay)
	// C++ AuctionHouseObject::Update (AuctionHouseMgr.cpp:622-629) drops
	// expired entries from the per-player getAll throttle map each sweep.
	if s.server != nil {
		s.server.auctionGetAllMu.Lock()
		for guid, throttleTime := range s.server.auctionGetAllThrottle {
			if throttleTime <= now {
				delete(s.server.auctionGetAllThrottle, guid)
			}
		}
		s.server.auctionGetAllMu.Unlock()
	}
	// C++ AuctionHouseObject::Update (AuctionHouseMgr.cpp:630-636): an
	// auction whose expire_time is within 60 seconds of now is settled on
	// this sweep — expire_time > curTime + 60 is the skip filter, so the
	// sweep horizon is now + 60. Like C++ CHAR_SEL_AUCTIONS, the item
	// entry/count resolve from the item_instance join.
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.houseid, ah.itemguid, ii.itemEntry, ii.count, ah.itemowner, ah.buyoutprice, ah.buyguid, ah.lastbid, ah.deposit
		FROM auctionhouse AS ah
		INNER JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.time <= ?`, now+60)
	if err != nil {
		return
	}
	type expiredRecord struct {
		id, houseID, itemGUID, itemTmpl, count, owner, buyout, bidder, lastBid, deposit int64
	}
	var expired []expiredRecord
	for rows.Next() {
		var r expiredRecord
		if err := rows.Scan(&r.id, &r.houseID, &r.itemGUID, &r.itemTmpl, &r.count, &r.owner, &r.buyout, &r.bidder, &r.lastBid, &r.deposit); err == nil {
			expired = append(expired, r)
		}
	}
	rows.Close()

	for _, a := range expired {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionhouse WHERE id = ?", a.id)
		// C++ AuctionEntry::DeleteFromDB (AuctionHouseMgr.cpp:890-896):
		// the auction's bidder rows die with it.
		_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionbidders WHERE id = ?", a.id)
		if a.bidder > 0 && a.lastBid > 0 {
			// Won by bidder. C++ AuctionHouseObject::Update (AuctionHouseMgr.cpp:649-657)
			// sends SendAuctionSuccessfulMail first, then SendAuctionWonMail — the
			// sale-pending/invoice mail (SendAuctionSalePendingMail) fires only on
			// player buyout (AuctionHouseHandler.cpp:559) and the bot-buyer path.
			consignment := uint32(a.lastBid) * 5 / 100
			profit := uint32(a.lastBid) + uint32(a.deposit) - consignment

			// 1. Profit mail to seller (delayed by MailDeliveryDelay, default 1 hour),
			// gated on the owner existing (SendAuctionSuccessfulMail, AuctionHouseMgr.cpp:230).
			if s.auctionCharExists(ctx, uint64(a.owner)) {
				sellerMailID := s.server.generateMailID()
				succSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionSuccessful, a.id, a.count)
				succBody := fmt.Sprintf("%X:%d:%d:%d:%d", a.bidder, a.lastBid, a.buyout, a.deposit, consignment)
				_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, ?, 0, 4)",
					// C++ Mail.cpp:197,215: expire_time = deliver_time + expire_delay (30d: money but no items/COD -> else branch);
					// deliver_delay comes from AuctionHouseMgr::SendAuctionSuccessfulMail (AuctionHouseMgr.cpp:230),
					// which passes CONFIG_MAIL_DELIVERY_DELAY ("MailDeliveryDelay", default 3600).
					sellerMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.owner, succSubj, succBody, now+mailDelay+30*86400, now+mailDelay, profit)
				s.sendMailNotify(uint64(a.owner))
				// C++ :224-230: the owner notification and the gold-earned /
				// highest-sold criteria fire only when the owner is connected.
				if sellerSess := s.server.findSessionByGUID(uint64(a.owner)); sellerSess != nil {
					s.notifyAuctionOwner(uint64(a.owner), uint32(a.id), uint32(a.lastBid), uint32(a.itemTmpl))
					sellerSess.setAchievementCriteria(criteriaTypeHighestAuctionSold, 0, uint32(a.lastBid))
					sellerSess.updateAchievementCriteria(criteriaTypeGoldEarnedAuctions, 0, profit)
				}
			}

			// 2. Won mail with the item to the bidder (immediate delivery),
			// gated on the item existing (SendAuctionWonMail, AuctionHouseMgr.cpp:120-122)
			// and the bidder existing (:164: (bidder || bidderAccId)); when the
			// bidder is gone the item row is deleted instead (:186:
			// RemoveAItem(itemGUIDLow, true, &trans) -> ITEM_REMOVED SaveToDB).
			var itemProbe int
			if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM item_instance WHERE guid = ? LIMIT 1", a.itemGUID).Scan(&itemProbe); err == nil {
				if s.auctionCharExists(ctx, uint64(a.bidder)) {
					wonMailID := s.server.generateMailID()
					wonSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionWon, a.id, a.count)
					wonBody := fmt.Sprintf("%X:%d:%d", a.owner, a.lastBid, a.buyout)
					_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 1, ?, ?, 0, 0, 4)",
						wonMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.bidder, wonSubj, wonBody, now+30*86400, now)
					_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", wonMailID, a.itemGUID, a.bidder)
					_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", a.bidder, a.itemGUID)
					s.sendMailNotify(uint64(a.bidder))
					// C++ :170-174: the connected winning bidder gets the
					// bidder notification (zeroed sums) and the won-auctions
					// achievement; this restores the notification the 18:41 run
					// removed with misaligned args.
					if bidderSess := s.server.findSessionByGUID(uint64(a.bidder)); bidderSess != nil {
						bidderSess.sendAuctionBidderNotification(uint32(a.houseID), uint32(a.id), uint64(a.bidder), 0, 0, uint32(a.itemTmpl))
						bidderSess.updateAchievementCriteria(criteriaTypeWonAuctions, 0, 1)
					}
				} else {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", a.itemGUID)
				}
			}
		} else {
			// Expired with no bids: return item to owner (deposit forfeited).
			// C++ SendAuctionExpiredMail (AuctionHouseMgr.cpp:235-260): a missing
			// auctioned item returns silently — no mail is sent (the auction row,
			// already deleted above, is still cleared in C++ via DeleteFromDB).
			var itemProbe int
			if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM item_instance WHERE guid = ? LIMIT 1", a.itemGUID).Scan(&itemProbe); err != nil {
				continue
			}
			// C++ :242-257: the item goes back by mail only when the owner is
			// connected or has an account; otherwise the item row is deleted.
			// (The auction-bot IsBotChar term has no Go model.)
			var ownerProbe int
			if s.server.findSessionByGUID(uint64(a.owner)) == nil &&
				cdb.QueryRowContext(ctx, "SELECT 1 FROM characters WHERE guid = ? LIMIT 1", a.owner).Scan(&ownerProbe) != nil {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", a.itemGUID)
				continue
			}
			expMailID := s.server.generateMailID()
			expSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionExpired, a.id, a.count)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 1, ?, ?, 0, 0, 4)",
				expMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.owner, expSubj, now+30*86400, now)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", expMailID, a.itemGUID, a.owner)
			s.sendMailNotify(uint64(a.owner))
			s.notifyAuctionOwner(uint64(a.owner), uint32(a.id), 0, uint32(a.itemTmpl))
		}
	}
}

func (s *session) sendAuctionBidderNotification(location, auctionID uint32, bidderGUID uint64, bidSum, diff, itemEntry uint32) {
	buf := protocol.NewBuffer(28)
	buf.WriteU32(location)
	buf.WriteU32(auctionID)
	buf.WriteU64(bidderGUID)
	buf.WriteU32(bidSum)
	buf.WriteU32(diff)
	buf.WriteU32(itemEntry)
	buf.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_BIDDER_NOTIFICATION), buf.Bytes(), true)
}

func (s *session) sendAuctionOwnerNotification(auctionID, bid uint32, itemEntry uint32) {
	buf := protocol.NewBuffer(32)
	buf.WriteU32(auctionID)
	buf.WriteU32(bid)
	buf.WriteU32(0)
	// C++ WorldSession::SendAuctionOwnerNotification
	// (AuctionHouseHandler.cpp:103-116): the guid slot is always zero —
	// "unk (bidder guid?)".
	buf.WriteU64(0)
	buf.WriteU32(itemEntry)
	buf.WriteU32(0)
	buf.WriteF32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_OWNER_NOTIFICATION), buf.Bytes(), true)
}

func (s *session) notifyAuctionBidder(recipientGUID, newBidderGUID uint64, location, auctionID, bidSum, diff, itemEntry uint32) {
	if s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(recipientGUID)
	if targetSess != nil {
		targetSess.sendAuctionBidderNotification(location, auctionID, newBidderGUID, bidSum, diff, itemEntry)
	}
}

func (s *session) notifyAuctionOwner(ownerGUID uint64, auctionID, bid uint32, itemEntry uint32) {
	if s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(ownerGUID)
	if targetSess != nil {
		targetSess.sendAuctionOwnerNotification(auctionID, bid, itemEntry)
	}
}

// notifyMailAvailable is the Server half of the mail-arrival notify used by
// the mail sweep and send paths: the online receiver's unread count is
// refreshed and SMSG_RECEIVED_MAIL fires when something is actually waiting.
func (srv *Server) notifyMailAvailable(receiverGUID uint64) {
	if srv == nil {
		return
	}
	targetSess := srv.findSessionByGUID(receiverGUID)
	if targetSess != nil {
		targetSess.loadMailState(context.Background())
		if targetSess.unreadMails > 0 {
			targetSess.sendNewMailNotification(context.Background())
		}
	}
}

func (s *session) sendMailNotify(receiverGUID uint64) {
	if s.server == nil {
		return
	}
	s.server.notifyMailAvailable(receiverGUID)
}

// scanAuctionRows runs an auctionhouse row query and converts the rows to
// auctionRecord values, skipping rows that fail to scan.
func scanAuctionRows(ctx context.Context, db *sql.DB, query string, args ...interface{}) []auctionRecord {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var auctions []auctionRecord
	for rows.Next() {
		var id, iGuid, iTmpl, owner, buyout, expTime, bidder, lastBid, startBid, deposit, count int64
		if err := rows.Scan(&id, &iGuid, &iTmpl, &owner, &buyout, &expTime, &bidder, &lastBid, &startBid, &deposit, &count); err == nil {
			auctions = append(auctions, auctionRecord{
				ID:         uint32(id),
				ItemGUID:   uint64(iGuid),
				ItemEntry:  uint32(iTmpl),
				ItemCount:  uint32(count),
				Owner:      uint64(owner),
				Buyout:     uint32(buyout),
				ExpireTime: expTime,
				Bidder:     uint64(bidder),
				Bid:        uint32(lastBid),
				StartBid:   uint32(startBid),
				Deposit:    uint32(deposit),
			})
		}
	}
	return auctions
}

func writeAuctionInfo(buf *protocol.Buffer, a auctionRecord) {
	now := time.Now().Unix()
	timeLeftMs := uint32(0)
	if a.ExpireTime > now {
		timeLeftMs = uint32((a.ExpireTime - now) * 1000)
	}
	buf.WriteU32(a.ID)
	buf.WriteU32(a.ItemEntry)
	// C++ AuctionEntry::BuildAuctionInfo (AuctionHouseMgr.cpp:849): the
	// enchantment block is MAX_INSPECTED_ENCHANTMENT_SLOT = 7 triples
	// (ItemDefines.h:153), not 6 — one slot short shifts every later field
	// by 12 bytes on the wire. The Go item model keeps no enchantment
	// data, so the seven triples stay zeroed.
	for i := 0; i < 7; i++ {
		buf.WriteU32(0)
		buf.WriteU32(0)
		buf.WriteU32(0)
	}
	buf.WriteI32(0) // RandomPropertyId
	buf.WriteU32(0) // SuffixFactor
	buf.WriteU32(a.ItemCount)
	buf.WriteU32(0) // SpellCharges
	buf.WriteU32(0) // ItemFlags
	buf.WriteU64(a.Owner)
	buf.WriteU32(a.StartBid)
	minOutBid := uint32(0)
	if a.Bid > 0 {
		minOutBid = a.Bid * 5 / 100
		if minOutBid == 0 {
			minOutBid = 1
		}
	}
	buf.WriteU32(minOutBid)
	buf.WriteU32(a.Buyout)
	buf.WriteU32(timeLeftMs)
	buf.WriteU64(a.Bidder)
	buf.WriteU32(a.Bid)
}

func buildAuctionCommandResult(auctionID, action, result uint32) []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU32(auctionID)
	buf.WriteU32(action)
	buf.WriteU32(result)
	return buf.Bytes()
}

// handleAuctionListPendingSales processes CMSG_AUCTION_LIST_PENDING_SALES (0x48F).
// Reference: WorldSession::HandleAuctionListPendingSales (AuctionHouseHandler.cpp:812).
func (s *session) handleAuctionListPendingSales(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// WorldSession::HandleAuctionListPendingSales (AuctionHouseHandler.cpp:812-830):
	// the handler only skips the 8-byte auctioneer GUID (read_skip<uint64>)
	// and always answers a zero count — the per-sale loop in C++ is
	// commented out, so no sale data is ever sent, and there is NO NPC
	// interact check on this path (a prior Go implementation gated on
	// canInteractWithNPC, an invented restriction that dropped the answer
	// whenever the player was away from an auctioneer).
	// (A prior Go implementation parsed pending-sale mails here; that was
	// invented behavior with no C++ arm, removed for fidelity.)
	buf := protocol.NewBuffer(4)
	buf.WriteU32(0)
	return s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_PENDING_SALES), buf.Bytes(), true) == nil
}
