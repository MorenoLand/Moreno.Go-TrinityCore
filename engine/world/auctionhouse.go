package world

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// TrinityCore AuctionAction enum (AuctionHouseMgr.h:50)
const (
	auctionSellItem uint32 = 0
	auctionCancel   uint32 = 1
	auctionPlaceBid uint32 = 2
)

// TrinityCore AuctionError enum (AuctionHouseMgr.h:32)
const (
	errAuctionOK                uint32 = 0
	errAuctionInventory         uint32 = 1
	errAuctionItemNotFound      uint32 = 2
	errAuctionNotEnoughMoney    uint32 = 3
	errAuctionDatabaseError     uint32 = 4
	errAuctionBidIncrement      uint32 = 7
	errAuctionCantBidOwn        uint32 = 10
	errAuctionRestrictedAccount uint32 = 13
)

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
	defaultAuctionHouseID uint32 = 1
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
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	s.expireAuctions(ctx)
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagAuctioneer)) {
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
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	s.expireAuctions(ctx)
	reader := protocol.NewReader(payload)
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
	_ = getAll

	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return true
	}

	whereClauses := []string{"ah.time > ?"}
	args := []interface{}{time.Now().Unix()}

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
		whereClauses = append(whereClauses, "it.RequiredLevel <= ?")
		args = append(args, s.player.Level)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := "SELECT COUNT(*) FROM auctionhouse AS ah LEFT JOIN item_template AS it ON it.entry = ah.item_template WHERE " + whereSQL
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

	query := `SELECT ah.id, ah.itemguid, ah.item_template, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, COALESCE(ii.count, 1)
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
		LEFT JOIN item_template AS it ON it.entry = ah.item_template
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
	packet.WriteU32(uint32(totalCount)) // Total count
	packet.WriteU32(300)                // Delay
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_RESULT), packet.Bytes(), true)
	s.debug("auction list items sent", "account", s.accountName, "count", len(auctions), "total", totalCount)
	return true
}

func (s *session) handleAuctionSellItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 24 {
		return true
	}
	reader := protocol.NewReader(payload)
	auctioneer, _ := reader.ReadU64() // auctioneer
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	itemCount, err := reader.ReadU32()
	if err != nil || itemCount == 0 {
		return false
	}
	rawItemGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}
	stackCount, err := reader.ReadU32()
	if err != nil {
		return false
	}
	bid, _ := reader.ReadU32()
	buyout, _ := reader.ReadU32()
	etime, _ := reader.ReadU32() // minutes
	if etime == 0 {
		etime = 1440 // 24 hours
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemEntry int64
	err = cdb.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ? AND owner_guid = ? LIMIT 1", itemGUID, s.playerGUID).Scan(&itemEntry)
	if err != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, errAuctionItemNotFound), true)
		return true
	}

	// Calculate deposit: TrinityCore GetAuctionDeposit formula
	// 5% of vendor SellPrice per 12 hours * stackCount, minimum 1 silver (100 copper)
	var sellPrice int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SellPrice, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&sellPrice)
	}
	timeHr := (etime / 60) / 12
	if timeHr == 0 {
		timeHr = 1
	}
	deposit := uint32(100) // 1 silver base deposit
	if sellPrice > 0 {
		calc := uint32(float64(sellPrice) * 0.05 * float64(timeHr) * float64(stackCount))
		if calc > deposit {
			deposit = calc
		}
	}

	if s.player.Money < deposit {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionSellItem, errAuctionNotEnoughMoney), true)
		return true
	}
	s.player.Money -= deposit
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
	s.refreshQuestItemCounts(ctx, uint32(itemEntry), false)
	s.despawnItem(uint64(itemGUID))
	now := time.Now().Unix()
	expire := now + int64(etime*60)
	var nextID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM auctionhouse").Scan(&nextID)
	if nextID <= 0 {
		nextID = 1
	}
	_, _ = cdb.ExecContext(ctx, `INSERT INTO auctionhouse (id, houseid, itemguid, item_template, itemCount, itemowner, buyoutprice, time, buyguid, lastbid, startbid, deposit)
		VALUES (?, 1, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`, nextID, itemGUID, itemEntry, stackCount, s.playerGUID, buyout, expire, bid, deposit)
	s.updateAchievementCriteria(criteriaTypeCreateAuction, 0, 1)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(uint32(nextID), auctionSellItem, errAuctionOK), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	s.debug("auction created", "account", s.accountName, "auction_id", nextID, "item", itemEntry, "deposit", deposit)
	return true
}

func (s *session) handleAuctionPlaceBid(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	reader := protocol.NewReader(payload)
	auctioneer, _ := reader.ReadU64()
	if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
		return true
	}
	auctionID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	price, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemGUID, itemEntry, ownerGUID, buyout, bidderGUID, lastBid, deposit, startBid, itemCount int64
	err = cdb.QueryRowContext(ctx, `SELECT ah.itemguid, ah.item_template, ah.itemowner, ah.buyoutprice, ah.buyguid, ah.lastbid, ah.deposit, ah.startbid, COALESCE(ii.count, 1)
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.id = ? LIMIT 1`, auctionID).Scan(&itemGUID, &itemEntry, &ownerGUID, &buyout, &bidderGUID, &lastBid, &deposit, &startBid, &itemCount)
	if err != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionDatabaseError), true)
		return true
	}

	// Player cannot bid on their own auction
	if ownerGUID == int64(s.playerGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionCantBidOwn), true)
		return true
	}

	// Price cannot be lower than start bid
	if price < uint32(startBid) {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionBidIncrement), true)
		return true
	}

	// Min increment check if not buyout
	isBuyout := buyout > 0 && price >= uint32(buyout)
	if !isBuyout && lastBid > 0 {
		minOutBid := uint32(lastBid) * 5 / 100
		if minOutBid == 0 {
			minOutBid = 1
		}
		if price < uint32(lastBid)+minOutBid {
			_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionBidIncrement), true)
			return true
		}
	}

	// Required money check (if previous bidder, only pay difference)
	priceToDeduct := price
	if bidderGUID == int64(s.playerGUID) && uint32(lastBid) < price {
		priceToDeduct = price - uint32(lastBid)
	}

	if s.player.Money < priceToDeduct {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionNotEnoughMoney), true)
		return true
	}
	s.player.Money -= priceToDeduct
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

	now := time.Now().Unix()

	s.setAchievementCriteria(criteriaTypeHighestAuctionBid, 0, price)
	if isBuyout {
		// Buyout success!
		s.updateAchievementCriteria(criteriaTypeWonAuctions, 0, 1)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionhouse WHERE id = ?", auctionID)

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
			var refundMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&refundMailID)
			outbidSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionOutbidded, auctionID, itemCount)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
				refundMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, outbidSubj, now+30*86400, now, lastBid)
			s.sendMailNotify(uint64(bidderGUID))
			s.notifyAuctionBidder(uint64(bidderGUID), 1, auctionID, uint32(buyout), 0, uint32(itemEntry))
		}

		// 2. Send won mail with item to buyer (immediate delivery)
		var nextMailID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
		wonSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionWon, auctionID, itemCount)
		wonBody := fmt.Sprintf("%X:%d:%d", ownerGUID, buyout, buyout)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 1, ?, ?, 0, 0, 4)",
			nextMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, s.playerGUID, wonSubj, wonBody, now+30*86400, now)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, item_template, receiver) VALUES (?, ?, ?, ?)", nextMailID, itemGUID, itemEntry, s.playerGUID)
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, itemGUID)
		s.sendMailNotify(uint64(s.playerGUID))
		s.sendAuctionBidderNotification(1, auctionID, s.playerGUID, uint32(buyout), 0, uint32(itemEntry))

		// 3. Send profit mail to seller (delayed by 1 hour)
		var sellerMailID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&sellerMailID)
		succSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionSuccessful, auctionID, itemCount)
		succBody := fmt.Sprintf("%X:%d:%d:%d:%d", s.playerGUID, buyout, buyout, deposit, consignment)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, ?, 0, 4)",
			sellerMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, succSubj, succBody, now+30*86400, now+3600, profit)

		// 4. Send auction invoice / sale pending notice mail to seller (immediate delivery, expires in 1 hour)
		var invoiceMailID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&invoiceMailID)
		pendingSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionSalePending, auctionID, itemCount)
		pendingBody := fmt.Sprintf("%X:%d:%d:%d:%d:%d:%d", s.playerGUID, buyout, buyout, deposit, consignment, 3600, 0)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, 0, 0, 4)",
			invoiceMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, pendingSubj, pendingBody, now+3600, now)
		s.sendMailNotify(uint64(ownerGUID))
		s.notifyAuctionOwner(uint64(ownerGUID), auctionID, uint32(buyout), s.playerGUID, uint32(itemEntry))

		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionPlaceBid, errAuctionOK), true)
	} else {
		// Normal Bid / Outbid previous bidder
		if bidderGUID != 0 && lastBid > 0 && bidderGUID != int64(s.playerGUID) {
			// Refund previous bidder via mail
			var refundMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&refundMailID)
			outbidSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionOutbidded, auctionID, itemCount)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
				refundMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, outbidSubj, now+30*86400, now, lastBid)
			s.sendMailNotify(uint64(bidderGUID))
			minOutBid := price * 5 / 100
			if minOutBid == 0 {
				minOutBid = 1
			}
			s.notifyAuctionBidder(uint64(bidderGUID), 1, auctionID, price, minOutBid, uint32(itemEntry))
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE auctionhouse SET buyguid = ?, lastbid = ? WHERE id = ?", s.playerGUID, price, auctionID)
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
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.itemguid, ah.item_template, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, COALESCE(ii.count, 1)
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
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
	if len(payload) >= 16 {
		r := protocol.NewReader(payload)
		_, _ = r.ReadU64() // auctioneer
		_, _ = r.ReadU32() // listFrom
		outbiddedCount, _ := r.ReadU32()
		for i := uint32(0); i < outbiddedCount && r.Remaining() >= 4; i++ {
			id, err := r.ReadU32()
			if err == nil && id > 0 {
				outbidIDs = append(outbidIDs, id)
			}
		}
	}

	now := time.Now().Unix()
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.itemguid, ah.item_template, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, COALESCE(ii.count, 1)
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.buyguid = ? AND ah.time > ?`, s.playerGUID, now)
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
		if err := cdb.QueryRowContext(ctx, `SELECT ah.id, ah.itemguid, ah.item_template, ah.itemowner, ah.buyoutprice, ah.time, ah.buyguid, ah.lastbid, ah.startbid, ah.deposit, COALESCE(ii.count, 1)
			FROM auctionhouse AS ah
			LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
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
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	reader := protocol.NewReader(payload)
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
	err = cdb.QueryRowContext(ctx, `SELECT ah.itemguid, ah.item_template, ah.itemowner, ah.buyguid, ah.lastbid, COALESCE(ii.count, 1)
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.id = ? AND ah.itemowner = ? LIMIT 1`, auctionID, s.playerGUID).Scan(&itemGUID, &itemEntry, &ownerGUID, &bidderGUID, &lastBid, &itemCount)
	if err != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(0, auctionCancel, errAuctionDatabaseError), true)
		return true
	}

	// TrinityCore: If auction has an active bidder, seller must pay the auction cut (5%)
	if bidderGUID > 0 && lastBid > 0 {
		auctionCut := uint32(lastBid) * 5 / 100
		if s.player.Money < auctionCut {
			_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionCancel, errAuctionNotEnoughMoney), true)
			return true
		}
		s.player.Money -= auctionCut
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		s.sendPlayerMoneyUpdate()
		s.sendPlayerUpdate()
	}

	_, _ = cdb.ExecContext(ctx, "DELETE FROM auctionhouse WHERE id = ?", auctionID)

	now := time.Now().Unix()

	// Mail item back to owner
	var nextMailID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&nextMailID)
	cancelSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionCanceled, auctionID, itemCount)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 1, ?, ?, 0, 0, 4)",
		nextMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, ownerGUID, cancelSubj, now+30*86400, now)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, item_template, receiver) VALUES (?, ?, ?, ?)", nextMailID, itemGUID, itemEntry, ownerGUID)
	s.sendMailNotify(uint64(ownerGUID))

	// Refund active bidder via mail (TC: SendAuctionCancelledToBidderMail)
	if bidderGUID > 0 && lastBid > 0 {
		var bidderMailID int64
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&bidderMailID)
		bidderSubj := fmt.Sprintf("%d:0:%d:%d:%d", itemEntry, auctionCancelledToBidder, auctionID, itemCount)
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 0, ?, ?, ?, 0, 4)",
			bidderMailID, mailAuctionType, mailStationeryAuction, defaultAuctionHouseID, bidderGUID, bidderSubj, now+30*86400, now, lastBid)
		s.sendMailNotify(uint64(bidderGUID))
		s.notifyAuctionBidder(uint64(bidderGUID), 1, auctionID, 0, 0, uint32(itemEntry))
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_COMMAND_RESULT), buildAuctionCommandResult(auctionID, auctionCancel, errAuctionOK), true)
	return true
}

func (s *session) expireAuctions(ctx context.Context) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	now := time.Now().Unix()
	rows, err := cdb.QueryContext(ctx, `SELECT ah.id, ah.houseid, ah.itemguid, ah.item_template, COALESCE(ii.count, 1), ah.itemowner, ah.buyoutprice, ah.buyguid, ah.lastbid, ah.deposit
		FROM auctionhouse AS ah
		LEFT JOIN item_instance AS ii ON ii.guid = ah.itemguid
		WHERE ah.time <= ?`, now)
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
		if a.bidder > 0 && a.lastBid > 0 {
			// Won by bidder
			consignment := uint32(a.lastBid) * 5 / 100
			profit := uint32(a.lastBid) + uint32(a.deposit) - consignment

			// 1. Won mail to bidder (immediate)
			var wonMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&wonMailID)
			wonSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionWon, a.id, a.count)
			wonBody := fmt.Sprintf("%X:%d:%d", a.owner, a.lastBid, a.buyout)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 1, ?, ?, 0, 0, 4)",
				wonMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.bidder, wonSubj, wonBody, now+30*86400, now)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, item_template, receiver) VALUES (?, ?, ?, ?)", wonMailID, a.itemGUID, a.itemTmpl, a.bidder)
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", a.bidder, a.itemGUID)
			s.sendMailNotify(uint64(a.bidder))
			s.notifyAuctionBidder(uint64(a.bidder), uint32(a.houseID), uint32(a.id), uint32(a.lastBid), 0, uint32(a.itemTmpl))

			// 2. Profit mail to seller (delayed by 1 hour)
			var sellerMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&sellerMailID)
			succSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionSuccessful, a.id, a.count)
			succBody := fmt.Sprintf("%X:%d:%d:%d:%d", a.bidder, a.lastBid, a.buyout, a.deposit, consignment)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, ?, 0, 4)",
				sellerMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.owner, succSubj, succBody, now+30*86400, now+3600, profit)

			// 3. Invoice mail to seller (immediate)
			var invoiceMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&invoiceMailID)
			pendingSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionSalePending, a.id, a.count)
			pendingBody := fmt.Sprintf("%X:%d:%d:%d:%d:%d:%d", a.bidder, a.lastBid, a.buyout, a.deposit, consignment, 3600, 0)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?, 0, 0, 4)",
				invoiceMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.owner, pendingSubj, pendingBody, now+3600, now)
			s.sendMailNotify(uint64(a.owner))
			s.notifyAuctionOwner(uint64(a.owner), uint32(a.id), uint32(a.lastBid), uint64(a.bidder), uint32(a.itemTmpl))
			if s.server != nil {
				if bidderSess := s.server.findSessionByGUID(uint64(a.bidder)); bidderSess != nil {
					bidderSess.updateAchievementCriteria(criteriaTypeWonAuctions, 0, 1)
				}
				if sellerSess := s.server.findSessionByGUID(uint64(a.owner)); sellerSess != nil {
					sellerSess.setAchievementCriteria(criteriaTypeHighestAuctionSold, 0, uint32(a.lastBid))
					sellerSess.updateAchievementCriteria(criteriaTypeGoldEarnedAuctions, 0, profit)
				}
			}
		} else {
			// Expired with no bids: return item to owner (deposit forfeited)
			var expMailID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM mail").Scan(&expMailID)
			expSubj := fmt.Sprintf("%d:0:%d:%d:%d", a.itemTmpl, auctionExpired, a.id, a.count)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked) VALUES (?, ?, ?, 0, ?, ?, ?, '', 1, ?, ?, 0, 0, 4)",
				expMailID, mailAuctionType, mailStationeryAuction, a.houseID, a.owner, expSubj, now+30*86400, now)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, item_template, receiver) VALUES (?, ?, ?, ?)", expMailID, a.itemGUID, a.itemTmpl, a.owner)
			s.sendMailNotify(uint64(a.owner))
			s.notifyAuctionOwner(uint64(a.owner), uint32(a.id), 0, 0, uint32(a.itemTmpl))
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

func (s *session) sendAuctionOwnerNotification(auctionID, bid uint32, bidderGUID uint64, itemEntry uint32) {
	buf := protocol.NewBuffer(32)
	buf.WriteU32(auctionID)
	buf.WriteU32(bid)
	buf.WriteU32(0)
	buf.WriteU64(bidderGUID)
	buf.WriteU32(itemEntry)
	buf.WriteU32(0)
	buf.WriteF32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_AUCTION_OWNER_NOTIFICATION), buf.Bytes(), true)
}

func (s *session) notifyAuctionBidder(bidderGUID uint64, location, auctionID, bidSum, diff, itemEntry uint32) {
	if s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(bidderGUID)
	if targetSess != nil {
		targetSess.sendAuctionBidderNotification(location, auctionID, bidderGUID, bidSum, diff, itemEntry)
	}
}

func (s *session) notifyAuctionOwner(ownerGUID uint64, auctionID, bid uint32, bidderGUID uint64, itemEntry uint32) {
	if s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(ownerGUID)
	if targetSess != nil {
		targetSess.sendAuctionOwnerNotification(auctionID, bid, bidderGUID, itemEntry)
	}
}

func (s *session) sendMailNotify(receiverGUID uint64) {
	if s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(receiverGUID)
	if targetSess != nil {
		targetSess.loadMailState(context.Background())
		if targetSess.unreadMails > 0 {
			targetSess.sendNewMailNotification(context.Background())
		}
	}
}

func writeAuctionInfo(buf *protocol.Buffer, a auctionRecord) {
	now := time.Now().Unix()
	timeLeftMs := uint32(0)
	if a.ExpireTime > now {
		timeLeftMs = uint32((a.ExpireTime - now) * 1000)
	}
	buf.WriteU32(a.ID)
	buf.WriteU32(a.ItemEntry)
	for i := 0; i < 6; i++ {
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
		return false
	}
	if len(payload) >= 8 {
		r := protocol.NewReader(payload)
		auctioneer, _ := r.ReadU64() // auctioneer GUID (reference AuctionHouseHandler.cpp:816)
		if !s.canInteractWithNPC(ctx, auctioneer, uint64(unitNPCFlagAuctioneer)) {
			return true
		}
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(0)
		return s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_PENDING_SALES), buf.Bytes(), true) == nil
	}

	now := time.Now().Unix()
	rows, err := cdb.QueryContext(ctx, `SELECT m.subject, m.body, COALESCE(c.name, 'Buyer'), m.money, m.deliver_time
		FROM mail AS m
		LEFT JOIN characters AS c ON c.guid = m.sender
		WHERE m.receiver = ? AND m.messageType = ? AND m.money > 0 AND m.deliver_time > ? ORDER BY m.deliver_time ASC LIMIT 50`, s.playerGUID, mailAuctionType, now)
	if err != nil {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(0)
		return s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_PENDING_SALES), buf.Bytes(), true) == nil
	}

	type pendingSale struct {
		itemName  string
		buyerName string
		bid       uint32
		buyout    uint32
		timeLeft  float32
	}
	var sales []pendingSale

	type rawPendingRow struct {
		subject     string
		body        string
		buyerName   string
		money       int64
		deliverTime int64
	}
	var rawRows []rawPendingRow
	for rows.Next() {
		var r rawPendingRow
		if err := rows.Scan(&r.subject, &r.body, &r.buyerName, &r.money, &r.deliverTime); err == nil {
			rawRows = append(rawRows, r)
		}
	}
	rows.Close()

	for _, r := range rawRows {
		itemName := "Item"
		if strings.Contains(r.subject, ":") {
			parts := strings.Split(r.subject, ":")
			if len(parts) > 0 {
				var itemEntry int64
				_, _ = fmt.Sscanf(parts[0], "%d", &itemEntry)
				if itemEntry > 0 {
					var name string
					if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
						_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemEntry).Scan(&name)
					}
					if name == "" && cdb != nil {
						_ = cdb.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemEntry).Scan(&name)
					}
					if name != "" {
						itemName = name
					}
				}
			}
		} else {
			trimmed := strings.TrimPrefix(r.subject, "Auction successful: ")
			if trimmed != "" {
				itemName = trimmed
			}
		}

		buyerName := r.buyerName
		if strings.Contains(r.body, ":") {
			bodyParts := strings.Split(r.body, ":")
			if len(bodyParts) > 0 {
				var buyerGUID uint64
				_, _ = fmt.Sscanf(bodyParts[0], "%X", &buyerGUID)
				if buyerGUID > 0 && cdb != nil {
					var name string
					if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", buyerGUID).Scan(&name); err == nil && name != "" {
						buyerName = name
					}
				}
			}
		}

		var timeLeft float32
		if r.deliverTime > now {
			timeLeft = float32(r.deliverTime-now) / 3600.0
		}
		sales = append(sales, pendingSale{
			itemName:  itemName,
			buyerName: buyerName,
			bid:       uint32(r.money),
			buyout:    uint32(r.money),
			timeLeft:  timeLeft,
		})
	}

	buf := protocol.NewBuffer(4 + len(sales)*64)
	buf.WriteU32(uint32(len(sales)))
	for _, ps := range sales {
		buf.WriteCString(ps.itemName)
		buf.WriteCString(ps.buyerName)
		buf.WriteU32(ps.bid)
		buf.WriteU32(ps.buyout)
		buf.WriteF32(ps.timeLeft)
	}

	return s.write(uint16(protocol.OpcodeSMSG_AUCTION_LIST_PENDING_SALES), buf.Bytes(), true) == nil
}
