package world

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type vendorItemRecord struct {
	Slot          uint32
	ItemEntry     uint32
	DisplayInfoID uint32
	MaxCount      int32
	BuyPrice      uint32
	MaxDurability uint32
	BuyCount      uint32
	ExtendedCost  uint32
}

const itemFlag2DontIgnoreBuyPrice uint32 = 0x00000004

const (
	buyErrCantFindItem      = 0
	buyErrItemAlreadySold   = 1
	buyErrNotEnoughMoney    = 2
	buyErrCantCarryMore     = 8
	buyErrReputationRequire = 12
)

type vendorInventoryStack struct {
	GUID  int64
	Bag   int64
	Slot  int64
	Count uint64
}

type vendorStockKey struct {
	VendorGUID   uint64
	Item         uint32
	Slot         uint32
	ExtendedCost uint32
}

type vendorStockState struct {
	Current int32
	Updated time.Time
}

func (s *Server) currentVendorStock(vendor, item uint32, maxCount int32, increment time.Duration, buyCount uint32) int32 {
	return s.currentVendorStockForGUID(uint64(vendor), item, 0, 0, maxCount, increment, buyCount)
}

func (s *Server) currentVendorStockFor(vendor, item, slot, extendedCost uint32, maxCount int32, increment time.Duration, buyCount uint32) int32 {
	return s.currentVendorStockForGUID(uint64(vendor), item, slot, extendedCost, maxCount, increment, buyCount)
}

func (s *Server) currentVendorStockForGUID(vendorGUID uint64, item, slot, extendedCost uint32, maxCount int32, increment time.Duration, buyCount uint32) int32 {
	if maxCount <= 0 {
		return -1
	}
	if buyCount == 0 {
		buyCount = 1
	}
	s.vendorMu.Lock()
	defer s.vendorMu.Unlock()
	if s.vendorStock == nil {
		s.vendorStock = make(map[vendorStockKey]*vendorStockState)
	}
	key := vendorStockKey{VendorGUID: vendorGUID, Item: item, Slot: slot, ExtendedCost: extendedCost}
	stock := s.vendorStock[key]
	if stock == nil {
		stock = &vendorStockState{Current: maxCount, Updated: time.Now()}
		s.vendorStock[key] = stock
	}
	if stock.Current > maxCount {
		stock.Current = maxCount
	}
	if increment > 0 && stock.Current < maxCount {
		steps := int32(time.Since(stock.Updated) / increment)
		if steps > 0 {
			stock.Current += steps * int32(buyCount)
			if stock.Current > maxCount {
				stock.Current = maxCount
			}
			stock.Updated = stock.Updated.Add(time.Duration(steps) * increment)
		}
	}
	return stock.Current
}

func (s *Server) consumeVendorStock(vendor, item uint32, amount uint32) (int32, bool) {
	return s.consumeVendorStockForGUID(uint64(vendor), item, 0, 0, amount)
}

func (s *Server) consumeVendorStockFor(vendor, item, slot, extendedCost, amount uint32) (int32, bool) {
	return s.consumeVendorStockForGUID(uint64(vendor), item, slot, extendedCost, amount)
}

func (s *Server) consumeVendorStockForGUID(vendorGUID uint64, item, slot, extendedCost, amount uint32) (int32, bool) {
	s.vendorMu.Lock()
	defer s.vendorMu.Unlock()
	stock := s.vendorStock[vendorStockKey{VendorGUID: vendorGUID, Item: item, Slot: slot, ExtendedCost: extendedCost}]
	if stock == nil || stock.Current < int32(amount) {
		return 0, false
	}
	stock.Current -= int32(amount)
	return stock.Current, true
}

func (s *Server) restoreVendorStock(vendor, item uint32, amount uint32) {
	s.restoreVendorStockForGUID(uint64(vendor), item, 0, 0, amount)
}

func (s *Server) restoreVendorStockFor(vendor, item, slot, extendedCost, amount uint32) {
	s.restoreVendorStockForGUID(uint64(vendor), item, slot, extendedCost, amount)
}

func (s *Server) restoreVendorStockForGUID(vendorGUID uint64, item, slot, extendedCost, amount uint32) {
	s.vendorMu.Lock()
	if stock := s.vendorStock[vendorStockKey{VendorGUID: vendorGUID, Item: item, Slot: slot, ExtendedCost: extendedCost}]; stock != nil {
		stock.Current += int32(amount)
	}
	s.vendorMu.Unlock()
}

func (s *session) handleListInventory(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	vendorGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	return s.sendVendorList(ctx, vendorGUID)
}

func (s *session) sendVendorList(ctx context.Context, vendorGUID uint64) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		return true
	}
	creatureEntry := uint32((vendorGUID >> 24) & 0xFFFFFF)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT v.slot, v.item, v.maxcount, v.incrtime, v.ExtendedCost,
		COALESCE(t.displayid, 0), COALESCE(t.BuyPrice, 0), COALESCE(t.MaxDurability, 0), COALESCE(t.BuyCount, 1), COALESCE(t.FlagsExtra, 0)
		FROM npc_vendor AS v
		LEFT JOIN item_template AS t ON t.entry = v.item
		WHERE v.entry = ? ORDER BY v.slot LIMIT 150`, creatureEntry)
	if err != nil {
		return true
	}
	type vendorRow struct {
		slot, item, maxCount, incrTime, extCost, display, buyPrice, maxDur, buyCount, flagsExtra int64
	}
	rowsData := make([]vendorRow, 0, 150)
	for rows.Next() {
		var row vendorRow
		if err := rows.Scan(&row.slot, &row.item, &row.maxCount, &row.incrTime, &row.extCost, &row.display, &row.buyPrice, &row.maxDur, &row.buyCount, &row.flagsExtra); err == nil {
			rowsData = append(rowsData, row)
		}
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return true
	}
	var items []vendorItemRecord
	var fallbackSlot uint32 = 1
	isGM := s.player != nil && (s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0)
	for _, row := range rowsData {
		item, maxCount, incrTime, extCost, display, buyPrice, maxDur, buyCount, flagsExtra := row.item, row.maxCount, row.incrTime, row.extCost, row.display, row.buyPrice, row.maxDur, row.buyCount, row.flagsExtra
		itemSlot := fallbackSlot
		fallbackSlot++
		if extCost != 0 {
			if s.server.Data == nil {
				continue
			}
			if _, found, err := s.server.Data.ItemExtendedCost(uint32(extCost)); err != nil || !found {
				continue
			}
			if flagsExtra&int64(itemFlag2DontIgnoreBuyPrice) == 0 {
				buyPrice = 0
			}
		}
		if meets, err := s.meetVendorItemConditions(ctx, creatureEntry, uint32(item)); err != nil || !meets {
			continue
		}
		if buyPrice > 0 {
			buyPrice = int64(math.Floor(float64(buyPrice) * s.vendorReputationPriceDiscount(ctx, creatureEntry)))
		}
		if buyCount <= 0 {
			buyCount = 1
		}
		inStock := s.server.currentVendorStockForGUID(vendorGUID, uint32(item), itemSlot, uint32(extCost), int32(maxCount), time.Duration(incrTime)*time.Second, uint32(buyCount))
		if inStock == 0 && !isGM {
			continue
		}
		items = append(items, vendorItemRecord{
			Slot:          itemSlot,
			ItemEntry:     uint32(item),
			DisplayInfoID: uint32(display),
			MaxCount:      inStock,
			BuyPrice:      uint32(buyPrice),
			MaxDurability: uint32(maxDur),
			BuyCount:      uint32(buyCount),
			ExtendedCost:  uint32(extCost),
		})
	}
	packet := protocol.NewBuffer(8 + 1 + len(items)*32)
	packet.WriteU64(vendorGUID)
	packet.WriteU8(uint8(len(items)))
	for _, it := range items {
		packet.WriteU32(it.Slot)
		packet.WriteU32(it.ItemEntry)
		packet.WriteU32(it.DisplayInfoID)
		packet.WriteU32(vendorPacketStock(it.MaxCount))
		packet.WriteU32(it.BuyPrice)
		packet.WriteU32(it.MaxDurability)
		packet.WriteU32(it.BuyCount)
		packet.WriteU32(it.ExtendedCost)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_LIST_INVENTORY), packet.Bytes(), true)
	s.debug("vendor list sent", "account", s.accountName, "vendor", vendorGUID, "items", len(items))
	return true
}

func vendorStockValue(maxCount int64) int32 {
	if maxCount <= 0 {
		return -1
	}
	if maxCount > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(maxCount)
}

func vendorPacketStock(current int32) uint32 {
	if current < 0 {
		return ^uint32(0)
	}
	if current == 0 {
		return 0
	}
	return uint32(current)
}

func (s *session) handleBuyItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 14 {
		return true
	}
	reader := protocol.NewReader(payload)
	vendorGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	itemEntry, err := reader.ReadU32()
	if err != nil {
		return false
	}
	slot, err := reader.ReadU32()
	if err != nil {
		return false
	}
	count, err := reader.ReadU32()
	if err != nil || count == 0 {
		count = 1
	}
	return s.processBuyItem(ctx, vendorGUID, itemEntry, slot, count)
}

func (s *session) handleBuyItemInSlot(ctx context.Context, payload []byte) bool {
	return s.handleBuyItem(ctx, payload)
}

func (s *session) processBuyItem(ctx context.Context, vendorGUID uint64, itemEntry, slot, count uint32) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, uint64(vendorGUID), uint64(unitNPCFlagVendor)) {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(uint64(vendorGUID), itemEntry, 5), true)
		return true
	}
	vendorEntry := uint32((vendorGUID >> 24) & 0xFFFFFF)
	if meets, err := s.meetVendorItemConditions(ctx, vendorEntry, itemEntry); err != nil || !meets {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		return true
	}
	var dbItemEntry, maxCount, incrTime, extCost, buyPrice, buyCount, flagsExtra, allowableClass, bonding, requiredReputationFaction, requiredReputationRank int64
	var queryErr error
	if slot == 0 || slot > 150 {
		queryErr = sql.ErrNoRows
	} else {
		queryErr = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT v.item, v.maxcount, v.incrtime, v.ExtendedCost, t.BuyPrice, t.BuyCount, COALESCE(t.FlagsExtra, 0), COALESCE(t.AllowableClass, -1), COALESCE(t.Bonding, 0), COALESCE(t.RequiredReputationFaction, 0), COALESCE(t.RequiredReputationRank, 0) FROM npc_vendor AS v JOIN item_template AS t ON t.entry = v.item WHERE v.entry = ? ORDER BY v.slot, v.item LIMIT 1 OFFSET ?", vendorEntry, slot-1).Scan(&dbItemEntry, &maxCount, &incrTime, &extCost, &buyPrice, &buyCount, &flagsExtra, &allowableClass, &bonding, &requiredReputationFaction, &requiredReputationRank)
	}
	if queryErr != nil || uint32(dbItemEntry) != itemEntry {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		return true
	}
	if buyCount <= 0 {
		buyCount = 1
	}
	amount := uint64(count) * uint64(buyCount)
	if amount > uint64(^uint32(0)) {
		return true
	}
	accessResult, allowed := s.vendorItemAccess(uint32(allowableClass), uint32(bonding), uint32(flagsExtra), uint32(requiredReputationFaction), uint32(requiredReputationRank), ctx)
	if !allowed {
		if accessResult >= 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, uint8(accessResult)), true)
		}
		return true
	}
	if extCost != 0 && flagsExtra&int64(itemFlag2DontIgnoreBuyPrice) == 0 {
		buyPrice = 0
	}
	if buyPrice > 0 {
		buyPrice = int64(math.Floor(float64(buyPrice) * s.vendorReputationPriceDiscount(ctx, vendorEntry)))
	}
	if uint64(buyPrice) > uint64(^uint32(0))/uint64(count) {
		return true
	}
	totalCost := uint32(buyPrice) * count
	remainingStock := int32(-1)
	if maxCount > 0 {
		current := s.server.currentVendorStockForGUID(vendorGUID, itemEntry, slot, uint32(extCost), int32(maxCount), time.Duration(incrTime)*time.Second, uint32(buyCount))
		if current < int32(amount) {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrItemAlreadySold), true)
			return true
		}
	}
	extendedCost, extendedCostResult, ok := s.vendorExtendedCost(ctx, uint32(extCost), count)
	if !ok {
		if extendedCostResult != equipErrOk {
			s.sendEquipError(extendedCostResult, 0)
		} else {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		}
		return true
	}
	if s.player.Money < totalCost {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrNotEnoughMoney), true)
		return true
	}
	if maxCount > 0 {
		var ok bool
		remainingStock, ok = s.server.consumeVendorStockForGUID(vendorGUID, itemEntry, slot, uint32(extCost), uint32(amount))
		if !ok {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrItemAlreadySold), true)
			return true
		}
	}
	res, err := s.storeOrStackItem(ctx, s.playerGUID, itemEntry, uint32(amount))
	if err != nil {
		if maxCount > 0 {
			s.server.restoreVendorStockForGUID(vendorGUID, itemEntry, slot, uint32(extCost), uint32(amount))
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantCarryMore), true)
		return true
	}
	if extendedCost.ID != 0 {
		if err := s.destroyVendorExtendedCostItems(ctx, extendedCost, count); err != nil {
			s.rollbackVendorStoredItem(ctx, res, uint32(amount))
			if maxCount > 0 {
				s.server.restoreVendorStockForGUID(vendorGUID, itemEntry, slot, uint32(extCost), uint32(amount))
			}
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
			return true
		}
	}
	s.player.Money -= totalCost
	if extendedCost.ID != 0 {
		s.player.TotalHonorPoints -= extendedCost.HonorPoints * count
		s.player.ArenaPoints -= extendedCost.ArenaPoints * count
	}
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ?, arenaPoints = ?, totalHonorPoints = ? WHERE guid = ?", s.player.Money, s.player.ArenaPoints, s.player.TotalHonorPoints, s.playerGUID)
		if extendedCost.ID != 0 && uint32(amount) == 1 {
			s.recordVendorRefund(ctx, cdb, res.ItemGUID, itemEntry, totalCost, uint32(extCost))
		}
	}
	newCount := vendorPacketStock(remainingStock)
	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_ITEM), buildBuySucceeded(vendorGUID, slot, newCount, count), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item bought from vendor", "account", s.accountName, "item", itemEntry, "count", count, "cost", totalCost, "extended_cost", extCost, "slot", res.Slot, "bag", res.ClientBag, "stacked", res.IsStack)
	return true
}

func (s *session) recordVendorRefund(ctx context.Context, cdb *sql.DB, itemGUID uint64, itemEntry, paidMoney, extendedCost uint32) {
	if cdb == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var flags, stackable int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(Flags, 0), COALESCE(stackable, 1) FROM item_template WHERE entry = ?", itemEntry).Scan(&flags, &stackable); err != nil || uint32(flags)&0x00001000 == 0 || stackable != 1 {
		return
	}
	_, _ = cdb.ExecContext(ctx, "REPLACE INTO item_refund_instance (item_guid, player_guid, paidMoney, paidExtendedCost) VALUES (?, ?, ?, ?)", itemGUID, s.playerGUID, paidMoney, extendedCost)
}

func (s *session) vendorItemAccess(allowableClass, bonding, flagsExtra, requiredFaction, requiredRank uint32, ctx context.Context) (int, bool) {
	isGM := s != nil && s.player != nil && (s.security > 0 || s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0)
	if !isGM && bonding == 1 && s.player != nil {
		if s.player.Class == 0 || allowableClass&(uint32(1)<<(s.player.Class-1)) == 0 {
			return buyErrCantFindItem, false
		}
	}
	if !isGM && s.player != nil {
		team := teamForRace(s.player.Race)
		if flagsExtra&0x00000001 != 0 && team != 1 {
			return -1, false
		}
		if flagsExtra&0x00000002 != 0 && team != 0 {
			return -1, false
		}
	}
	if requiredFaction != 0 && s.vendorReputationRank(ctx, requiredFaction) < requiredRank {
		return buyErrReputationRequire, false
	}
	return buyErrCantFindItem, true
}

func (s *session) vendorReputationRank(ctx context.Context, factionID uint32) uint32 {
	if s.player != nil {
		for _, reputation := range s.player.Reputations {
			if reputation.FactionID == factionID {
				return reputationRank(int64(totalReputationStanding(reputation)))
			}
		}
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var standing int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT standing FROM character_reputation WHERE guid = ? AND faction = ?", s.playerGUID, factionID).Scan(&standing); err != nil {
		return 0
	}
	if s.server.Data != nil {
		if reputation, found, err := s.server.Data.Reputation(factionID, s.player.Race, s.player.Class); err == nil && found {
			standing += int64(reputation.BaseStanding)
		}
	}
	return reputationRank(standing)
}

func (s *session) vendorReputationPriceDiscount(ctx context.Context, vendorEntry uint32) float64 {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return 1
	}
	var faction int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT faction FROM creature_template WHERE entry = ?", vendorEntry).Scan(&faction); err != nil || faction <= 0 {
		return 1
	}
	template, found, err := s.server.Data.FactionTemplate(uint32(faction))
	if err != nil || !found || template.Faction == 0 {
		return 1
	}
	rank := s.vendorReputationRank(ctx, template.Faction)
	if rank <= 3 {
		return 1
	}
	return 1 - 0.05*float64(rank-3)
}

func (s *session) vendorExtendedCost(ctx context.Context, id, count uint32) (wotlk.ItemExtendedCostEntry, uint8, bool) {
	if id == 0 {
		return wotlk.ItemExtendedCostEntry{}, equipErrOk, true
	}
	if s.server == nil || s.server.Data == nil {
		return wotlk.ItemExtendedCostEntry{}, equipErrOk, false
	}
	entry, found, err := s.server.Data.ItemExtendedCost(id)
	if err != nil || !found {
		return wotlk.ItemExtendedCostEntry{}, equipErrOk, false
	}
	if uint64(entry.HonorPoints)*uint64(count) > uint64(s.player.TotalHonorPoints) {
		return entry, equipErrNotEnoughHonorPoints, false
	}
	if uint64(entry.ArenaPoints)*uint64(count) > uint64(s.player.ArenaPoints) {
		return entry, equipErrNotEnoughArenaPoints, false
	}
	for i := 0; i < len(entry.ItemIDs); i++ {
		if entry.ItemIDs[i] == 0 || entry.ItemCounts[i] == 0 {
			continue
		}
		required := uint64(entry.ItemCounts[i]) * uint64(count)
		available, err := s.vendorInventoryItemCount(ctx, entry.ItemIDs[i])
		if err != nil || available < required {
			return entry, equipErrVendorMissingTurnins, false
		}
	}
	if entry.RequiredArenaRating != 0 && s.maxPersonalArenaRating(ctx, entry.ArenaBracket) < entry.RequiredArenaRating {
		return entry, equipErrCantEquipRank, false
	}
	return entry, equipErrOk, true
}

func (s *session) vendorInventoryItemCount(ctx context.Context, itemEntry uint32) (uint64, error) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, errors.New("characters database not available")
	}
	var count int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0)
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ((ci.bag = 0 AND (ci.slot BETWEEN 0 AND ? OR (ci.slot >= ? AND ci.slot < ?))) OR ci.bag IN
			(SELECT bag.item FROM character_inventory AS bag WHERE bag.guid = ? AND bag.bag = 0 AND bag.slot BETWEEN 19 AND 22))
		AND ii.itemEntry = ?`, s.playerGUID, invSlotItemEnd-1, invSlotKeyringStart, invSlotKeyringEnd, s.playerGUID, itemEntry).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, errors.New("negative inventory count")
	}
	return uint64(count), nil
}

func (s *session) destroyVendorExtendedCostItems(ctx context.Context, entry wotlk.ItemExtendedCostEntry, count uint32) error {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return errors.New("characters database not available")
	}
	tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i := 0; i < len(entry.ItemIDs); i++ {
		if entry.ItemIDs[i] == 0 || entry.ItemCounts[i] == 0 {
			continue
		}
		required := uint64(entry.ItemCounts[i]) * uint64(count)
		if required > uint64(^uint32(0)) {
			_ = tx.Rollback()
			return errors.New("extended item cost overflow")
		}
		if err := s.destroyVendorInventoryItemCountTx(ctx, tx, entry.ItemIDs[i], uint32(required)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for i, itemEntry := range entry.ItemIDs {
		if itemEntry != 0 && entry.ItemCounts[i] != 0 {
			s.adjustQuestItemCount(ctx, itemEntry, entry.ItemCounts[i]*count, false)
		}
	}
	return nil
}

func (s *session) destroyVendorInventoryItemCountTx(ctx context.Context, tx *sql.Tx, itemEntry, count uint32) error {
	if count == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT ci.item, ci.bag, ci.slot, ii.count
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ((ci.bag = 0 AND (ci.slot BETWEEN 0 AND ? OR (ci.slot >= ? AND ci.slot < ?))) OR ci.bag IN
			(SELECT bag.item FROM character_inventory AS bag WHERE bag.guid = ? AND bag.bag = 0 AND bag.slot BETWEEN 19 AND 22))
		AND ii.itemEntry = ? ORDER BY ci.bag, ci.slot`, s.playerGUID, invSlotItemEnd-1, invSlotKeyringStart, invSlotKeyringEnd, s.playerGUID, itemEntry)
	if err != nil {
		return err
	}
	stacks := make([]vendorInventoryStack, 0)
	for rows.Next() {
		var stack vendorInventoryStack
		if err := rows.Scan(&stack.GUID, &stack.Bag, &stack.Slot, &stack.Count); err != nil {
			_ = rows.Close()
			return err
		}
		stacks = append(stacks, stack)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	remaining := uint64(count)
	for _, stack := range stacks {
		if remaining == 0 {
			break
		}
		used := stack.Count
		if used > remaining {
			used = remaining
		}
		if used == stack.Count {
			if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, stack.GUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", stack.GUID); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", used, stack.GUID); err != nil {
			return err
		}
		remaining -= used
	}
	if remaining != 0 {
		return errors.New("extended item cost inventory changed")
	}
	return nil
}

func (s *session) rollbackVendorStoredItem(ctx context.Context, result *inventoryStoreResult, count uint32) {
	if result == nil || count == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	if result.IsStack {
		var itemEntry int64
		_ = cdb.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", result.ItemGUID).Scan(&itemEntry)
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ? AND count >= ?", count, result.ItemGUID, count)
		if itemEntry > 0 {
			s.adjustQuestItemCount(ctx, uint32(itemEntry), count, false)
		}
		return
	}
	var itemEntry int64
	_ = cdb.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", result.ItemGUID).Scan(&itemEntry)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, result.ItemGUID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", result.ItemGUID)
	if itemEntry > 0 {
		s.adjustQuestItemCount(ctx, uint32(itemEntry), count, false)
	}
}

func (s *session) maxPersonalArenaRating(ctx context.Context, minSlot uint32) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT t.type, t.rating, m.personalRating
		FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ?`, s.playerGUID)
	if err != nil {
		return 0
	}
	defer rows.Close()
	var maxRating uint32
	for rows.Next() {
		var teamType, teamRating, personalRating uint32
		if rows.Scan(&teamType, &teamRating, &personalRating) != nil {
			continue
		}
		slot := uint32(3)
		switch teamType {
		case 2:
			slot = 0
		case 3:
			slot = 1
		case 5:
			slot = 2
		}
		if slot < minSlot {
			continue
		}
		if teamRating < personalRating {
			personalRating = teamRating
		}
		if personalRating > maxRating {
			maxRating = personalRating
		}
	}
	return maxRating
}

func (s *session) handleSellItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 17 {
		return true
	}
	reader := protocol.NewReader(payload)
	vendorGUID, err := reader.ReadU64()
	if err != nil {
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
	count, err := reader.ReadU8()
	if err != nil || count == 0 {
		count = 1
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		return true
	}
	var itemEntry, currentCount int64
	err = cdb.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.count FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&itemEntry, &currentCount)
	if err != nil || itemEntry == 0 {
		return true
	}
	var sellPrice int64
	_ = wdb.QueryRowContext(ctx, "SELECT SellPrice FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&sellPrice)
	if sellPrice <= 0 {
		sellPrice = 1
	}
	earned := uint32(sellPrice) * uint32(count)
	s.player.Money += earned
	s.updateAchievementCriteria(criteriaTypeMoneyFromVendor, 0, earned)
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

	bbItemGUID := uint64(itemGUID)
	if currentCount <= int64(count) {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", count, itemGUID)
	} else {
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", count, itemGUID)
		var newGUID int64
		if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&newGUID); err == nil && newGUID > 0 {
			bbItemGUID = uint64(newGUID)
		} else {
			bbItemGUID = uint64(time.Now().UnixNano() & 0x7FFFFFFF)
		}
		_, _ = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, count) VALUES (?, ?, ?, ?)", bbItemGUID, itemEntry, s.playerGUID, count)
	}
	s.adjustQuestItemCount(ctx, uint32(itemEntry), uint32(count), false)

	// TrinityCore: Player::AddItemToBuyBackSlot (Player.cpp:13495)
	// Assign buyback slot (0..11 corresponding to BUYBACK_SLOT_START 74 .. BUYBACK_SLOT_END 86)
	slot := int(s.currentBuybackSlot)
	if slot >= 12 || s.buyback[slot] != nil {
		oldestSlot := 0
		oldestTime := uint32(0xFFFFFFFF)
		for i := 0; i < 12; i++ {
			if s.buyback[i] == nil {
				oldestSlot = i
				break
			}
			if s.buyback[i].Timestamp < oldestTime {
				oldestTime = s.buyback[i].Timestamp
				oldestSlot = i
			}
		}
		slot = oldestSlot
	}

	// If overwriting an existing buyback item, despawn the evicted item
	if s.buyback[slot] != nil {
		evictedGUID := s.buyback[slot].ItemGUID
		s.sendDestroyObject(evictedGUID, false)
		s.despawnItem(evictedGUID)
	}
	var evictedDBGUID int64
	if err := cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, 74+slot).Scan(&evictedDBGUID); err == nil {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, 74+slot)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", evictedDBGUID)
	}

	fullBBGUID := bbItemGUID | (uint64(0x4000) << 48)
	s.buyback[slot] = &buybackSlot{
		ItemGUID:  fullBBGUID,
		ItemEntry: uint32(itemEntry),
		Count:     uint32(count),
		Price:     earned,
		Timestamp: uint32(time.Now().Unix()),
	}
	if s.currentBuybackSlot < 11 {
		s.currentBuybackSlot++
	}
	_, _ = cdb.ExecContext(ctx, "INSERT OR REPLACE INTO character_inventory (guid, bag, slot, item) VALUES (?, 0, ?, ?)", s.playerGUID, 74+slot, bbItemGUID)

	s.syncEquipmentCache(ctx)
	_ = s.write(uint16(protocol.OpcodeSMSG_SELL_ITEM), buildSellResult(vendorGUID, rawItemGUID, 0), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item sold to vendor", "account", s.accountName, "item", itemEntry, "guid", itemGUID, "count", count, "earned", earned, "buybackSlot", slot)
	return true
}

// handleBuybackItem processes CMSG_BUYBACK_ITEM (0x290).
// Reference: WorldSession::HandleBuybackItem (ItemHandler.cpp:490).
func (s *session) handleBuybackItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	vendorGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		return true
	}
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}

	eslot := int(slot)
	if eslot >= 74 && eslot <= 85 {
		eslot -= 74
	}
	if eslot < 0 || eslot >= 12 || s.buyback[eslot] == nil {
		return true
	}

	entry := s.buyback[eslot]
	if s.player.Money < entry.Price {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, entry.ItemEntry, 2), true) // BUY_ERR_NOT_ENOUGHT_MONEY
		return true
	}

	res, err := s.storeOrStackItemCore(ctx, s.playerGUID, entry.ItemEntry, entry.Count)
	if err != nil {
		s.sendEquipError(equipErrInvFull, entry.ItemGUID)
		return true // Inventory full
	}

	oldItemGUID := entry.ItemGUID
	s.buyback[eslot] = nil
	s.currentBuybackSlot = uint8(eslot)
	s.player.Money -= entry.Price
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, 74+eslot)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", int64(oldItemGUID&0xFFFFFFFF))
	}
	s.adjustQuestItemCount(ctx, entry.ItemEntry, entry.Count, true)

	// Destroy temporary buyback item if stored GUID is different
	newFullGUID := uint64(res.ItemGUID) | (uint64(0x4000) << 48)
	if oldItemGUID != newFullGUID {
		s.sendDestroyObject(oldItemGUID, false)
		s.despawnItem(oldItemGUID)
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_ITEM), buildBuySucceeded(vendorGUID, entry.ItemEntry, entry.Count, entry.Count), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("buyback item purchased", "account", s.accountName, "item", entry.ItemEntry, "slot", res.Slot, "bag", res.ClientBag, "stacked", res.IsStack)
	return true
}

func (s *session) loadBuybackState(ctx context.Context, guid uint64) {
	for index := range s.buyback {
		s.buyback[index] = nil
	}
	s.currentBuybackSlot = 0
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.slot, ii.guid, ii.itemEntry, ii.count
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot BETWEEN 74 AND 85 ORDER BY ci.slot`, guid)
	if err != nil {
		return
	}
	for rows.Next() {
		var slot, itemGUID, itemEntry, count int64
		if rows.Scan(&slot, &itemGUID, &itemEntry, &count) != nil || slot < 74 || slot > 85 || itemGUID <= 0 || itemEntry <= 0 {
			continue
		}
		price := int64(0)
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT SellPrice FROM item_template WHERE entry = ?", itemEntry).Scan(&price)
		}
		if price < 0 {
			price = 0
		}
		eslot := int(slot - 74)
		s.buyback[eslot] = &buybackSlot{ItemGUID: uint64(itemGUID) | (uint64(0x4000) << 48), ItemEntry: uint32(itemEntry), Count: uint32(count), Price: uint32(price) * uint32(count), Timestamp: uint32(time.Now().Unix())}
	}
	rows.Close()
	if rows.Err() != nil {
		return
	}
	for index := range s.buyback {
		if s.buyback[index] == nil {
			s.currentBuybackSlot = uint8(index)
			return
		}
	}
	s.currentBuybackSlot = 11
}

func buildBuyFailed(vendorGUID uint64, itemEntry uint32, result uint8) []byte {
	buf := protocol.NewBuffer(13)
	buf.WriteU64(vendorGUID)
	buf.WriteU32(itemEntry)
	buf.WriteU8(result)
	return buf.Bytes()
}

func buildBuySucceeded(vendorGUID uint64, slot, newCount, buyCount uint32) []byte {
	buf := protocol.NewBuffer(20)
	buf.WriteU64(vendorGUID)
	buf.WriteU32(slot)
	buf.WriteU32(newCount)
	buf.WriteU32(buyCount)
	return buf.Bytes()
}

func buildSellResult(vendorGUID, itemGUID uint64, result uint8) []byte {
	buf := protocol.NewBuffer(17)
	buf.WriteU64(vendorGUID)
	buf.WriteU64(itemGUID)
	buf.WriteU8(result)
	return buf.Bytes()
}
