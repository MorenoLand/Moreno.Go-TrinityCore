package world

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Reference: SharedDefines.h:3601 (enum TradeStatus)
const (
	tradeStatusBusy          uint32 = 0
	tradeStatusBeginTrade    uint32 = 1
	tradeStatusOpenWindow    uint32 = 2
	tradeStatusTradeCanceled uint32 = 3
	tradeStatusTradeAccept   uint32 = 4
	tradeStatusBusy2         uint32 = 5
	tradeStatusNoTarget      uint32 = 6
	tradeStatusBackToTrade   uint32 = 7
	tradeStatusTradeComplete uint32 = 8
	tradeStatusTradeRejected uint32 = 9
	tradeStatusTargetTooFar  uint32 = 10
	tradeStatusWrongFaction  uint32 = 11
	tradeStatusCloseWindow   uint32 = 12
	tradeStatusIgnoreYou     uint32 = 14
	tradeStatusYouStunned    uint32 = 15
	tradeStatusTargetStunned uint32 = 16
	tradeStatusYouDead       uint32 = 17
	tradeStatusTargetDead    uint32 = 18
	tradeStatusYouLogout     uint32 = 19
	tradeStatusTargetLogout  uint32 = 20
	tradeStatusTrialAccount  uint32 = 21
	tradeStatusWrongRealm    uint32 = 22
	tradeStatusNotOnTaplist  uint32 = 23

	tradeSlotCount       = 7
	tradeSlotTradedCount = 6
	tradeSlotNonTraded   = 6
	tradeDistance        = 11.111111
)

type tradeSlotItem struct {
	ItemGUID   uint64
	ItemEntry  uint32
	DisplayID  uint32
	StackCount uint32
	EnchantID  uint32
}

type playerTradeState struct {
	Partner  *session
	Money    uint32
	Items    map[uint8]tradeSlotItem
	Accepted bool
}

// sendTradeStatus sends SMSG_TRADE_STATUS (0x120) with matching TrinityCore structure.
// Reference: WorldSession::SendTradeStatus (TradeHandler.cpp:34).
func (s *session) sendTradeStatus(status uint32, traderGUID uint64, result uint32, isTargetResult uint8, itemLimitCategory uint32) error {
	buf := protocol.NewBuffer(16)
	buf.WriteU32(status)
	switch status {
	case tradeStatusBeginTrade:
		buf.WriteU64(traderGUID)
	case tradeStatusOpenWindow:
		buf.WriteU32(0) // tradeID
	case tradeStatusCloseWindow:
		buf.WriteU32(result)
		buf.WriteU8(isTargetResult)
		buf.WriteU32(itemLimitCategory)
	}
	return s.write(uint16(protocol.OpcodeSMSG_TRADE_STATUS), buf.Bytes(), true)
}

// handleInitiateTrade processes CMSG_INITIATE_TRADE (0x116).
// Reference: WorldSession::HandleInitiateTradeOpcode (TradeHandler.cpp:590).
func (s *session) handleInitiateTrade(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	if s.trade != nil {
		return true
	}
	if s.player.MaxHealth > 0 && s.player.Health == 0 {
		_ = s.sendTradeStatus(tradeStatusYouDead, 0, 0, 0, 0)
		return true
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		_ = s.sendTradeStatus(tradeStatusNoTarget, 0, 0, 0, 0)
		return true
	}
	if targetSess == s || targetSess.trade != nil {
		_ = s.sendTradeStatus(tradeStatusBusy, 0, 0, 0, 0)
		return true
	}
	if targetSess.player.MaxHealth > 0 && targetSess.player.Health == 0 {
		_ = s.sendTradeStatus(tradeStatusTargetDead, 0, 0, 0, 0)
		return true
	}
	if targetSess.player.Map != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, targetSess.player.X, targetSess.player.Y, targetSess.player.Z) > tradeDistance {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0)
		return true
	}
	if s.player.Race != 0 && targetSess.player.Race != 0 && teamForRace(s.player.Race) != teamForRace(targetSess.player.Race) {
		_ = s.sendTradeStatus(tradeStatusWrongFaction, 0, 0, 0, 0)
		return true
	}

	s.trade = &playerTradeState{Partner: targetSess, Items: make(map[uint8]tradeSlotItem)}
	targetSess.trade = &playerTradeState{Partner: s, Items: make(map[uint8]tradeSlotItem)}

	// Send SMSG_TRADE_STATUS (TRADE_STATUS_BEGIN_TRADE) to target
	_ = targetSess.sendTradeStatus(tradeStatusBeginTrade, s.playerGUID, 0, 0, 0)
	s.debug("trade initiated", "from", s.accountName, "to", targetSess.accountName)
	return true
}

// handleBeginTrade processes CMSG_BEGIN_TRADE (0x117).
// Reference: WorldSession::HandleBeginTradeOpcode (TradeHandler.cpp:561).
func (s *session) handleBeginTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || s.trade.Partner == nil {
		return true
	}
	partner := s.trade.Partner
	_ = s.sendTradeStatus(tradeStatusOpenWindow, 0, 0, 0, 0)
	_ = partner.sendTradeStatus(tradeStatusOpenWindow, 0, 0, 0, 0)
	s.notifyTradeUpdate()
	partner.notifyTradeUpdate()
	s.debug("trade window opened", "player1", s.accountName, "player2", partner.accountName)
	return true
}

// handleSetTradeGold processes CMSG_SET_TRADE_GOLD (0x11F).
// Reference: WorldSession::HandleSetTradeGoldOpcode (TradeHandler.cpp:711).
func (s *session) handleSetTradeGold(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	gold, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if gold > s.player.Money {
		gold = s.player.Money
	}
	s.trade.Money = gold
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
		}
		s.notifyTradeUpdate()
	}
	return true
}

// handleSetTradeItem processes CMSG_SET_TRADE_ITEM (0x11D).
// Reference: WorldSession::HandleSetTradeItemOpcode (TradeHandler.cpp:723).
func (s *session) handleSetTradeItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 3 {
		return true
	}
	tradeSlot := payload[0]
	bag := payload[1]
	slot := payload[2]
	if tradeSlot >= tradeSlotCount {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var itemGUID int64
	err := cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ? LIMIT 1", s.playerGUID, bag, slot).Scan(&itemGUID)
	if err != nil || itemGUID == 0 {
		return true
	}
	var itemEntry, count, flags int64
	var encStr sql.NullString
	_ = cdb.QueryRowContext(ctx, "SELECT itemEntry, count, flags, enchantments FROM item_instance WHERE guid = ? LIMIT 1", itemGUID).Scan(&itemEntry, &count, &flags, &encStr)
	if tradeSlot < tradeSlotTradedCount && (flags&1 != 0) {
		// Soulbound items cannot be placed in traded slots
		_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0)
		return true
	}
	var enchantID uint32
	if encStr.Valid && encStr.String != "" {
		fields := strings.Fields(encStr.String)
		if len(fields) > 0 {
			if e, err := strconv.ParseUint(fields[0], 10, 32); err == nil {
				enchantID = uint32(e)
			}
		}
	}
	var displayID uint32
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var disp int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT displayid FROM item_template WHERE entry = ?", itemEntry).Scan(&disp)
		displayID = uint32(disp)
	}
	s.trade.Items[tradeSlot] = tradeSlotItem{
		ItemGUID:   uint64(itemGUID),
		ItemEntry:  uint32(itemEntry),
		DisplayID:  displayID,
		StackCount: uint32(count),
		EnchantID:  enchantID,
	}
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
		}
		s.notifyTradeUpdate()
	}
	return true
}

// handleClearTradeItem processes CMSG_CLEAR_TRADE_ITEM (0x11E).
// Reference: WorldSession::HandleClearTradeItemOpcode (TradeHandler.cpp:780).
func (s *session) handleClearTradeItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || len(payload) < 1 {
		return true
	}
	tradeSlot := payload[0]
	delete(s.trade.Items, tradeSlot)
	if s.trade.Accepted {
		s.trade.Accepted = false
		_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
	}
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		if s.trade.Partner.trade.Accepted {
			s.trade.Partner.trade.Accepted = false
			_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
		}
		s.notifyTradeUpdate()
	}
	return true
}

// handleAcceptTrade processes CMSG_ACCEPT_TRADE (0x11A).
// Reference: WorldSession::HandleAcceptTradeOpcode (TradeHandler.cpp:337).
func (s *session) handleAcceptTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil || s.trade.Partner == nil {
		return true
	}
	partner := s.trade.Partner
	if partner.player == nil || s.player.Map != partner.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, partner.player.X, partner.player.Y, partner.player.Z) > tradeDistance {
		_ = s.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0)
		_ = partner.sendTradeStatus(tradeStatusTargetTooFar, 0, 0, 0, 0)
		s.trade = nil
		partner.trade = nil
		return true
	}

	s.trade.Accepted = true

	// Inform partner
	_ = partner.sendTradeStatus(tradeStatusTradeAccept, 0, 0, 0, 0)

	if partner.trade != nil && partner.trade.Accepted {
		// Both accepted -> execute trade
		s.completeTrade(ctx, partner)
	}
	return true
}

type tradeSlotLoc struct {
	bagKey int64
	slot   uint8
}

func (s *session) findFreeSlotsForTrade(ctx context.Context, count int) ([]tradeSlotLoc, bool) {
	if count == 0 {
		return nil, true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return nil, false
	}
	var freeLocs []tradeSlotLoc

	// 1. Check backpack (bag = 0, slots 23..38)
	usedBackpack := make(map[uint8]bool)
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var sl int64
			if rows.Scan(&sl) == nil {
				usedBackpack[uint8(sl)] = true
			}
		}
		rows.Close()
	}
	for sl := uint8(23); sl <= 38; sl++ {
		if !usedBackpack[sl] {
			freeLocs = append(freeLocs, tradeSlotLoc{bagKey: 0, slot: sl})
			if len(freeLocs) == count {
				return freeLocs, true
			}
		}
	}

	// 2. Check equipped bags (slots 19..22)
	equippedBags := s.getEquippedBags(ctx, s.playerGUID)
	for _, eb := range equippedBags {
		maxSlots := eb.slots
		if maxSlots <= 0 || maxSlots > 36 {
			continue
		}
		usedBagSlots := make(map[uint8]bool)
		bRows, bErr := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = ?", s.playerGUID, eb.guid)
		if bErr == nil {
			for bRows.Next() {
				var bsl int64
				if bRows.Scan(&bsl) == nil {
					usedBagSlots[uint8(bsl)] = true
				}
			}
			bRows.Close()
		}
		for bsl := uint8(0); bsl < uint8(maxSlots); bsl++ {
			if !usedBagSlots[bsl] {
				freeLocs = append(freeLocs, tradeSlotLoc{bagKey: eb.guid, slot: bsl})
				if len(freeLocs) == count {
					return freeLocs, true
				}
			}
		}
	}

	return freeLocs, len(freeLocs) >= count
}

// completeTrade finalizes the trade, exchanging traded items (slots 0..5) and currency.
// Reference: WorldSession::HandleAcceptTradeOpcode (TradeHandler.cpp:443-544).
func (s *session) completeTrade(ctx context.Context, partner *session) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}

	// Only slots 0..5 (TRADE_SLOT_TRADED_COUNT = 6) are traded; slot 6 is non-traded
	var sTradedItems []tradeSlotItem
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			sTradedItems = append(sTradedItems, it)
		}
	}
	var partnerTradedItems []tradeSlotItem
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := partner.trade.Items[slot]; ok {
			partnerTradedItems = append(partnerTradedItems, it)
		}
	}

	partnerSlots, ok1 := partner.findFreeSlotsForTrade(ctx, len(sTradedItems))
	sSlots, ok2 := s.findFreeSlotsForTrade(ctx, len(partnerTradedItems))
	if !ok1 || !ok2 {
		// EQUIP_ERR_BAG_FULL = 1
		if !ok1 {
			_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, 1, 0, 0)
			_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, 1, 1, 0) // isTargetResult = 1
		} else {
			_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, 1, 0, 0)
			_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, 1, 1, 0) // isTargetResult = 1
		}
		s.trade = nil
		partner.trade = nil
		return
	}

	if s.player.Money < s.trade.Money || partner.player.Money < partner.trade.Money {
		_ = s.sendTradeStatus(tradeStatusCloseWindow, 0, 0, 0, 0)
		_ = partner.sendTradeStatus(tradeStatusCloseWindow, 0, 0, 0, 0)
		s.trade = nil
		partner.trade = nil
		return
	}

	// Money transfer
	s.player.Money = s.player.Money - s.trade.Money + partner.trade.Money
	partner.player.Money = partner.player.Money - partner.trade.Money + s.trade.Money
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", partner.player.Money, partner.playerGUID)

	sTransferSlots := make(map[uint8]int, tradeSlotTradedCount)
	partnerTransferSlots := make(map[uint8]int, tradeSlotTradedCount)
	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			sTransferSlots[slot] = len(sTransferSlots)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, it.ItemGUID)
			s.despawnItem(it.ItemGUID)
			s.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, false)
		}
		if it, ok := partner.trade.Items[slot]; ok {
			partnerTransferSlots[slot] = len(partnerTransferSlots)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", partner.playerGUID, it.ItemGUID)
			partner.despawnItem(it.ItemGUID)
			partner.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, false)
		}
	}

	for slot := uint8(0); slot < tradeSlotTradedCount; slot++ {
		if it, ok := s.trade.Items[slot]; ok {
			targetLoc := partnerSlots[sTransferSlots[slot]]
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", partner.playerGUID, it.ItemGUID)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", partner.playerGUID, targetLoc.bagKey, targetLoc.slot, it.ItemGUID)
			partner.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, true)
		}
		if it, ok := partner.trade.Items[slot]; ok {
			targetLoc := sSlots[partnerTransferSlots[slot]]
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", s.playerGUID, it.ItemGUID)
			_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, targetLoc.bagKey, targetLoc.slot, it.ItemGUID)
			s.adjustQuestItemCount(ctx, it.ItemEntry, it.StackCount, true)
		}
	}

	_ = s.sendTradeStatus(tradeStatusTradeComplete, 0, 0, 0, 0)
	_ = partner.sendTradeStatus(tradeStatusTradeComplete, 0, 0, 0, 0)

	_ = s.sendInventoryItems(ctx)
	_ = partner.sendInventoryItems(ctx)
	s.sendPlayerMoneyUpdate()
	partner.sendPlayerMoneyUpdate()
	s.syncEquipmentCache(ctx)
	partner.syncEquipmentCache(ctx)
	s.sendPlayerUpdate()
	partner.sendPlayerUpdate()

	s.trade = nil
	partner.trade = nil
	s.debug("trade completed successfully", "player1", s.accountName, "player2", partner.accountName)
}

// handleUnacceptTrade processes CMSG_UNACCEPT_TRADE (0x11B).
// Reference: WorldSession::HandleUnacceptTradeOpcode (TradeHandler.cpp:552).
func (s *session) handleUnacceptTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil {
		return true
	}
	s.trade.Accepted = false
	_ = s.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
	if s.trade.Partner != nil && s.trade.Partner.trade != nil {
		s.trade.Partner.trade.Accepted = false
		_ = s.trade.Partner.sendTradeStatus(tradeStatusBackToTrade, 0, 0, 0, 0)
	}
	return true
}

// handleCancelTrade processes CMSG_CANCEL_TRADE (0x11C).
// Reference: WorldSession::HandleCancelTradeOpcode (TradeHandler.cpp:583).
func (s *session) handleCancelTrade(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.trade == nil {
		return true
	}
	partner := s.trade.Partner
	_ = s.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0)
	s.trade = nil
	if partner != nil {
		partner.trade = nil
		_ = partner.sendTradeStatus(tradeStatusTradeCanceled, 0, 0, 0, 0)
	}
	return true
}

// sendTradeStatusExtended sends SMSG_TRADE_STATUS_EXTENDED (0x121).
// Reference: WorldSession::SendUpdateTrade (TradeHandler.cpp:75).
func (s *session) sendTradeStatusExtended(traderData bool) {
	if s.trade == nil || s.trade.Partner == nil {
		return
	}
	data := s.trade
	if traderData {
		data = s.trade.Partner.trade
	}
	if data == nil {
		return
	}
	buf := protocol.NewBuffer(256)
	if traderData {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU32(0)              // tradeID
	buf.WriteU32(tradeSlotCount) // tradeSlotCount
	buf.WriteU32(tradeSlotCount) // tradeSlotCount
	buf.WriteU32(data.Money)
	buf.WriteU32(0) // spell
	for i := uint8(0); i < tradeSlotCount; i++ {
		buf.WriteU8(i)
		if it, ok := data.Items[i]; ok {
			buf.WriteU32(it.ItemEntry)
			buf.WriteU32(it.DisplayID)
			buf.WriteU32(it.StackCount)
			buf.WriteU32(0)            // wrapped
			buf.WriteU64(0)            // giftCreator
			buf.WriteU32(it.EnchantID) // permEnchant
			for j := 0; j < 3; j++ {
				buf.WriteU32(0) // gem sockets
			}
			buf.WriteU64(0) // creator
			buf.WriteU32(0) // charges
			buf.WriteU32(0) // randomPropId
			buf.WriteU32(0) // suffixFactor
			buf.WriteU32(0) // lockId
			buf.WriteU32(0) // maxDurability
			buf.WriteU32(0) // durability
		} else {
			for j := 0; j < 18; j++ {
				buf.WriteU32(0)
			}
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_TRADE_STATUS_EXTENDED), buf.Bytes(), true)
}

func (s *session) notifyTradeUpdate() {
	s.sendTradeStatusExtended(false)
	if s.trade != nil && s.trade.Partner != nil {
		s.trade.Partner.sendTradeStatusExtended(true)
	}
}
