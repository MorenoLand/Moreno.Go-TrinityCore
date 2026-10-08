package world

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
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

// BuyItemFromVendorSlot bag/slot arms (Player.cpp:21909-21910, ItemHandler.cpp:537-575):
// NULL_BAG/NULL_SLOT mark "no specific bag", MAX_BAG_SIZE bounds the bag slot.
const (
	vendorNullBag    uint8 = 0xFF
	vendorNullSlot   uint8 = 0xFF
	vendorMaxBagSize uint8 = 36
)

const (
	buyErrCantFindItem      = 0
	buyErrItemAlreadySold   = 1
	buyErrNotEnoughMoney    = 2
	buyErrCantCarryMore     = 8
	buyErrReputationRequire = 12
)

// SellResult (ItemDefines.h:132) for SMSG_SELL_ITEM error arms.
const (
	sellErrCantFindItem           = 1
	sellErrCantSellItem           = 2
	sellErrCantFindVendor         = 3
	sellErrCantSellToThisMerchant = 7
)

type vendorInventoryStack struct {
	GUID  int64
	Bag   int64
	Slot  int64
	Count uint64
}

type vendorStockKey struct {
	// Creature::m_vendorItemCounts is keyed by itemId ONLY (Creature.cpp:2893:
	// itr->itemId == vItem->item) — two npc_vendor rows selling the same item
	// entry share one restock counter, even across ExtendedCost variants. Slot
	// and ExtendedCost stay in the helper signatures for call-site convenience
	// but do not participate in the key.
	VendorGUID uint64
	Item       uint32
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
	key := vendorStockKey{VendorGUID: vendorGUID, Item: item}
	stock := s.vendorStock[key]
	if stock == nil {
		stock = &vendorStockState{Current: maxCount, Updated: time.Now()}
		s.vendorStock[key] = stock
	}
	if stock.Current > maxCount {
		stock.Current = maxCount
	}
	if increment > 0 && stock.Current < maxCount {
		// Creature::GetVendorItemCurrentCount (Creature.cpp:2907):
		// lastIncrementTime is reset to the current time (ptime), not to the
		// last increment boundary — the next restock step lands a full
		// incrtime after the query that observed the increment.
		now := time.Now()
		steps := int32(now.Sub(stock.Updated) / increment)
		if steps > 0 {
			stock.Current += steps * int32(buyCount)
			if stock.Current > maxCount {
				stock.Current = maxCount
			}
			stock.Updated = now
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
	stock := s.vendorStock[vendorStockKey{VendorGUID: vendorGUID, Item: item}]
	if stock == nil || stock.Current < int32(amount) {
		return 0, false
	}
	stock.Current -= int32(amount)
	// Creature::UpdateVendorItemCurrentCount (Creature.cpp:2944): a purchase
	// resets lastIncrementTime to now — the restock clock restarts from the
	// purchase, it does not continue from the pre-purchase boundary.
	stock.Updated = time.Now()
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
	// Go-only rollback path (C++ has no equivalent): mirror the purchase arm
	// and restart the clock from the rollback, keeping consume/restore symmetric.
	if stock := s.vendorStock[vendorStockKey{VendorGUID: vendorGUID, Item: item}]; stock != nil {
		stock.Current += int32(amount)
		stock.Updated = time.Now()
	}
	s.vendorMu.Unlock()
}

func (s *session) handleListInventory(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	// ItemHandler.cpp:610-611: dead players get no vendor list.
	if s.isDeadOrGhost() {
		return true
	}
	reader := protocol.NewReader(payload)
	vendorGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	return s.sendVendorList(ctx, vendorGUID)
}

// expandedVendorRow is one npc_vendor row after reference expansion
// (ObjectMgr::LoadVendors / LoadReferenceVendor, ObjectMgr.cpp:9365-9402): a
// negative item id splices the referenced vendor's rows inline at that slot
// position, recursively, in slot order.
type expandedVendorRow struct {
	item, maxCount, incrTime, extCost int64
}

func (s *session) expandedVendorRows(ctx context.Context, vendorEntry uint32) []expandedVendorRow {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil
	}
	// ObjectMgr::IsVendorItemValid (ObjectMgr.cpp:9617) drops rows at load
	// time: maxcount>0 requires incrtime>0, maxcount==0 requires incrtime==0,
	// a non-zero ExtendedCost must exist in the DBC, and duplicate
	// (item, ExtendedCost) pairs are ignored. Dropped rows never enter the
	// vendor vector, so they consume no client slot — enforcing the rules
	// here keeps the list and buy paths slot-identical to C++.
	extCostOK := make(map[int64]bool)
	checkExtCost := func(ec int64) bool {
		if ok, seen := extCostOK[ec]; seen {
			return ok
		}
		ok := false
		if s.server.Data != nil {
			_, found, err := s.server.Data.ItemExtendedCost(uint32(ec))
			ok = err == nil && found
		}
		extCostOK[ec] = ok
		return ok
	}
	seenPair := make(map[uint64]bool)
	validRow := func(r expandedVendorRow) bool {
		if r.item <= 0 || r.maxCount < 0 || (r.maxCount > 0 && r.incrTime == 0) || (r.maxCount == 0 && r.incrTime > 0) {
			return false
		}
		if r.extCost != 0 && !checkExtCost(r.extCost) {
			return false
		}
		key := uint64(uint32(r.item))<<32 | uint64(uint32(r.extCost))
		if seenPair[key] {
			return false
		}
		seenPair[key] = true
		return true
	}
	visited := make(map[uint32]bool)
	var expand func(entry uint32) []expandedVendorRow
	expand = func(entry uint32) []expandedVendorRow {
		if visited[entry] {
			return nil
		}
		visited[entry] = true
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT item, maxcount, incrtime, ExtendedCost FROM npc_vendor WHERE entry = ? ORDER BY slot`, entry)
		if err != nil {
			return nil
		}
		var out []expandedVendorRow
		for rows.Next() {
			var r expandedVendorRow
			if err := rows.Scan(&r.item, &r.maxCount, &r.incrTime, &r.extCost); err != nil {
				continue
			}
			if r.item < 0 {
				out = append(out, expand(uint32(-r.item))...)
				continue
			}
			if !validRow(r) {
				continue
			}
			out = append(out, r)
		}
		_ = rows.Close()
		return out
	}
	return expand(vendorEntry)
}

// vendorTemplateInfo carries the item_template columns the vendor list and
// buy paths need, batch-loaded so reference-expanded rows don't cost a query
// per item.
type vendorTemplateInfo struct {
	display, buyPrice, maxDur, buyCount, flagsExtra, allowableClass, bonding, reqRepFaction, reqRepRank int64
}

func vendorRowItemEntries(rows []expandedVendorRow) []int64 {
	items := make([]int64, 0, len(rows))
	for _, r := range rows {
		items = append(items, r.item)
	}
	return items
}

func (s *session) vendorTemplateMap(ctx context.Context, items []int64) map[int64]vendorTemplateInfo {
	out := make(map[int64]vendorTemplateInfo)
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || len(items) == 0 {
		return out
	}
	uniq := make([]int64, 0, len(items))
	seen := make(map[int64]bool)
	for _, it := range items {
		if it <= 0 || seen[it] {
			continue
		}
		seen[it] = true
		uniq = append(uniq, it)
	}
	for start := 0; start < len(uniq); start += 200 {
		end := start + 200
		if end > len(uniq) {
			end = len(uniq)
		}
		placeholders := strings.Repeat("?,", end-start)
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]interface{}, 0, end-start)
		for _, it := range uniq[start:end] {
			args = append(args, it)
		}
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT entry, COALESCE(displayid, 0), COALESCE(BuyPrice, 0), COALESCE(MaxDurability, 0), COALESCE(BuyCount, 1), COALESCE(FlagsExtra, 0), COALESCE(AllowableClass, -1), COALESCE(Bonding, 0), COALESCE(RequiredReputationFaction, 0), COALESCE(RequiredReputationRank, 0) FROM item_template WHERE entry IN (`+placeholders+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var entry int64
			var t vendorTemplateInfo
			if err := rows.Scan(&entry, &t.display, &t.buyPrice, &t.maxDur, &t.buyCount, &t.flagsExtra, &t.allowableClass, &t.bonding, &t.reqRepFaction, &t.reqRepRank); err == nil && entry > 0 {
				out[entry] = t
			}
		}
		_ = rows.Close()
	}
	return out
}

func (s *session) sendVendorList(ctx context.Context, vendorGUID uint64) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		return true
	}
	creatureEntry := uint32((vendorGUID >> 24) & 0xFFFFFF)
	// ObjectMgr::LoadVendors (ObjectMgr.cpp:9404): rows load in slot order with
	// reference rows (negative item) expanded inline; rows without an item
	// template are dropped at load (IsVendorItemValid) and never consume a
	// client slot — the skipped-by-filter rows below DO consume one, matching
	// the C++ vector index (SendListInventory sends slot+1 for every vector
	// element, ItemHandler.cpp:680).
	rowsData := s.expandedVendorRows(ctx, creatureEntry)
	templates := s.vendorTemplateMap(ctx, vendorRowItemEntries(rowsData))
	var items []vendorItemRecord
	var itemSlot uint32 = 1
	isGM := s.player != nil && (s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0)
	for _, row := range rowsData {
		tmpl, ok := templates[row.item]
		if !ok {
			continue
		}
		slot := itemSlot
		itemSlot++
		item, maxCount, incrTime, extCost := row.item, row.maxCount, row.incrTime, row.extCost
		display, buyPrice, maxDur, buyCount, flagsExtra := tmpl.display, tmpl.buyPrice, tmpl.maxDur, tmpl.buyCount, tmpl.flagsExtra
		allowableClass, bonding := tmpl.allowableClass, tmpl.bonding
		// VendorItem::IsGoldRequired (Creature.cpp:89): an ExtendedCost row
		// costs no gold unless the template sets
		// ITEM_FLAG2_DONT_IGNORE_BUY_PRICE. Row validity itself (bad
		// ExtendedCost, maxcount/incrtime rules, duplicate pairs) is enforced
		// at load in expandedVendorRows per ObjectMgr::IsVendorItemValid.
		if extCost != 0 && flagsExtra&int64(itemFlag2DontIgnoreBuyPrice) == 0 {
			buyPrice = 0
		}
		// ItemHandler.cpp:652-666 (SendListInventory): hide bind-on-pickup
		// items unusable by the player's class and wrong-faction items from
		// non-GMs — the buy path re-checks the same gates.
		if !s.vendorItemListable(uint32(allowableClass), uint32(bonding), uint32(flagsExtra)) {
			continue
		}
		if meets, err := s.meetVendorItemConditions(ctx, creatureEntry, uint32(item), vendorGUID); err != nil || !meets {
			continue
		}
		if buyPrice > 0 {
			// SendListInventory (ItemHandler.cpp): int32 price = item->IsGoldRequired(itemTemplate)
			//   ? uint32(floor(itemTemplate->BuyPrice * discountMod)) : 0 — discount floored on the
			//   unit price in the list, unlike the buy path which floors it on the total.
			buyPrice = int64(math.Floor(float64(float32(buyPrice) * s.vendorReputationPriceDiscount(ctx, creatureEntry))))
		}
		if buyCount <= 0 {
			buyCount = 1
		}
		inStock := s.server.currentVendorStockForGUID(vendorGUID, uint32(item), slot, uint32(extCost), int32(maxCount), time.Duration(incrTime)*time.Second, uint32(buyCount))
		if inStock == 0 && !isGM {
			continue
		}
		items = append(items, vendorItemRecord{
			Slot:          slot,
			ItemEntry:     uint32(item),
			DisplayInfoID: uint32(display),
			MaxCount:      inStock,
			BuyPrice:      uint32(buyPrice),
			MaxDurability: uint32(maxDur),
			BuyCount:      uint32(buyCount),
			ExtendedCost:  uint32(extCost),
		})
		// ItemHandler.cpp:695: MAX_VENDOR_ITEMS (150) caps the listed count.
		if len(items) >= 150 {
			break
		}
	}
	packet := protocol.NewBuffer(8 + 2 + len(items)*32)
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
	if len(items) == 0 {
		// C++ SendListInventory (ItemHandler.cpp:633-640, 695-699) appends a uint8 error
		// code ("Vendor has no inventory") after a zero count
		packet.WriteU8(0)
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
	return s.processBuyItem(ctx, vendorGUID, itemEntry, slot, count, vendorNullBag, vendorNullSlot)
}

func (s *session) handleBuyItemInSlot(ctx context.Context, payload []byte) bool {
	// CMSG_BUY_ITEM_IN_SLOT (ItemHandler.cpp:537-575): vendorguid, item, vendorslot,
	// bagguid, bagslot, count. The client slot stays 1-based here — processBuyItem
	// applies the OFFSET slot-1 lookup, matching the plain-buy path.
	if !s.playerLoaded || s.player == nil || len(payload) < 26 {
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
	bagGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	bagSlot, err := reader.ReadU8()
	if err != nil {
		return false
	}
	count, err := reader.ReadU8()
	if err != nil || count == 0 {
		count = 1
	}
	// Player.cpp:21909-21910: a bag slot beyond MAX_BAG_SIZE (and not NULL_SLOT)
	// is a cheating attempt — no reply.
	if bagSlot > vendorMaxBagSize && bagSlot != vendorNullSlot {
		return true
	}
	// ItemHandler.cpp:568-570: an unresolvable bag guid is a cheating attempt — no reply.
	bag, ok := s.vendorBuyBagSlot(ctx, bagGUID)
	if !ok {
		return true
	}
	return s.processBuyItem(ctx, vendorGUID, itemEntry, slot, uint32(count), bag, bagSlot)
}

// vendorBuyBagSlot resolves a CMSG_BUY_ITEM_IN_SLOT bag guid to the bag's equip slot
// (19-22) or 0 for the backpack (ItemHandler.cpp:552-567).
func (s *session) vendorBuyBagSlot(ctx context.Context, bagGUID uint64) (uint8, bool) {
	if bagGUID == s.playerGUID {
		return 0, true
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return vendorNullBag, false
	}
	var slot int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= ? AND slot < ? AND item = ?", s.playerGUID, invSlotBagStart, invSlotBagEnd, bagGUID).Scan(&slot); err != nil {
		return vendorNullBag, false
	}
	return uint8(slot), true
}

func (s *session) processBuyItem(ctx context.Context, vendorGUID uint64, itemEntry, slot, count uint32, bag, bagSlot uint8) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	// Player.cpp:21912-21913: dead players cannot buy — no reply.
	if s.isDeadOrGhost() {
		return true
	}
	if !s.canInteractWithNPC(ctx, uint64(vendorGUID), uint64(unitNPCFlagVendor)) {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(uint64(vendorGUID), itemEntry, 5), true)
		return true
	}
	vendorEntry := uint32((vendorGUID >> 24) & 0xFFFFFF)
	if meets, err := s.meetVendorItemConditions(ctx, vendorEntry, itemEntry, vendorGUID); err != nil || !meets {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		return true
	}
	// Player.cpp:21965-21976: the client slot (1-based) indexes the vendor's
	// loaded item vector — reference-expanded, template-validated rows only,
	// exactly the sequence sendVendorList numbers.
	rowsData := s.expandedVendorRows(ctx, vendorEntry)
	templates := s.vendorTemplateMap(ctx, vendorRowItemEntries(rowsData))
	validRows := make([]expandedVendorRow, 0, len(rowsData))
	for _, r := range rowsData {
		if _, ok := templates[r.item]; ok {
			validRows = append(validRows, r)
		}
	}
	// ItemHandler.cpp:585-589 / 546-549: "client expects count starting at 1 ...
	// if (slot > 0) --slot; else return; // cheating" — a 0 slot is a
	// cheating attempt answered with silence, not BUY_ERR_CANT_FIND_ITEM.
	if slot == 0 {
		return true
	}
	if int(slot) > len(validRows) {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		return true
	}
	row := validRows[slot-1]
	tmpl := templates[row.item]
	if uint32(row.item) != itemEntry {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrCantFindItem), true)
		return true
	}
	maxCount, incrTime, extCost := row.maxCount, row.incrTime, row.extCost
	buyPrice, buyCount, flagsExtra, allowableClass, bonding := tmpl.buyPrice, tmpl.buyCount, tmpl.flagsExtra, tmpl.allowableClass, tmpl.bonding
	requiredReputationFaction, requiredReputationRank := tmpl.reqRepFaction, tmpl.reqRepRank
	if buyCount <= 0 {
		buyCount = 1
	}
	amount := uint64(count) * uint64(buyCount)
	if amount > uint64(^uint32(0)) {
		return true
	}
	accessResult, allowed := s.vendorItemAccess(uint32(allowableClass), uint32(bonding), uint32(flagsExtra))
	if !allowed {
		if accessResult >= 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, uint8(accessResult)), true)
		}
		return true
	}
	// Player.cpp:22063-22085: price = pProto->BuyPrice * count; then
	// price = uint32(floor(price * GetReputationPriceDiscount(creature))) — the discount is floored
	// on the TOTAL, not the unit price, so multi-count buys can differ by whole coppers from the
	// floored-unit form. IsGoldRequired gates the whole block; the MAX_MONEY_AMOUNT cheat clamp on
	// count is deliberately not replicated (Go drops the overflowing buy silently).
	goldRequired := extCost == 0 || flagsExtra&int64(itemFlag2DontIgnoreBuyPrice) != 0
	var totalCost uint32
	if goldRequired && buyPrice > 0 {
		if uint64(buyPrice) > uint64(^uint32(0))/uint64(count) {
			return true
		}
		totalCost = uint32(math.Floor(float64(float32(uint32(buyPrice)*count) * s.vendorReputationPriceDiscount(ctx, vendorEntry))))
	}
	remainingStock := int32(-1)
	if maxCount > 0 {
		current := s.server.currentVendorStockForGUID(vendorGUID, itemEntry, slot, uint32(extCost), int32(maxCount), time.Duration(incrTime)*time.Second, uint32(buyCount))
		if current < int32(amount) {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrItemAlreadySold), true)
			return true
		}
	}
	// Player.cpp:22012-22016: the reputation gate runs after the stock check —
	// BUY_ERR_ITEM_ALREADY_SOLD fires before BUY_ERR_REPUTATION_REQUIRE.
	if requiredReputationFaction != 0 && s.vendorReputationRank(ctx, uint32(requiredReputationFaction)) < uint32(requiredReputationRank) {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrReputationRequire), true)
		return true
	}
	extendedCost, extendedCostResult, ok := s.vendorExtendedCost(ctx, uint32(extCost), count)
	if !ok {
		if extendedCostResult != equipErrOk {
			s.sendEquipError(extendedCostResult, 0)
		} else {
			// Player.cpp:22003-22007: a wrong ExtendedCost id only logs
			// (TC_LOG_ERROR) and returns false — no error packet reaches the client.
			s.debug("vendor item has wrong ExtendedCost id", "account", s.accountName, "item", itemEntry, "extendedCost", extCost)
		}
		return true
	}
	if s.player.Money < totalCost {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, itemEntry, buyErrNotEnoughMoney), true)
		return true
	}
	// Player.cpp:22085-22095: buying into an equipment slot requires a single item —
	// the EQUIP_ERR_ITEM_CANT_BE_EQUIPPED gate runs after the money check. Placement
	// into the requested bag/slot itself is unmodeled (the item auto-stores), so the
	// EQUIP_ERR_ITEM_DOESNT_GO_TO_SLOT arm is unreachable after the bagslot sanity
	// check in handleBuyItemInSlot.
	if bag == 0 && bagSlot < uint8(equipSlotEnd) && uint64(buyCount)*uint64(count) != 1 {
		s.sendEquipError(equipErrItemCantBeEquipped, 0)
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
		// Player.cpp:21888-21896 — the refundable arm has no amount gate: any
		// ExtendedCost + ITEM_FLAG_ITEM_PURCHASE_RECORD + maxstack-1 buy flags the
		// created item refundable (the template/stackable gates live inside
		// recordVendorRefund).
		if extendedCost.ID != 0 {
			s.recordVendorRefund(ctx, cdb, res.ItemGUID, itemEntry, totalCost, uint32(extCost))
		}
	}
	newCount := vendorPacketStock(remainingStock)
	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_ITEM), buildBuySucceeded(vendorGUID, slot, newCount, count), true)
	// Player::_StoreOrEquipNewItem (Player.cpp:21885-21892): every vendor
	// purchase answers SMSG_ITEM_PUSH_RESULT via SendNewItem(it,
	// BuyCount*count, received=true, created=false, sendChatMessage=false)
	// — received=1 "from npc", the (1,0,0) header buildItemPushResult
	// writes; the slot is 0xFFFFFFFF when the items stacked onto an
	// existing pile (item->GetCount() != count arm), matching the loot
	// path's slotForPush convention.
	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(s.playerGUID, res.ClientBag, slotForPush, itemEntry, uint32(amount), res.InventoryCount, res.IsStack), true)
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
	// Player.cpp:21892 — it->SetFlag(ITEM_FIELD_FLAGS, ITEM_FIELD_FLAG_REFUNDABLE):
	// the bit is client-visible ("Refundable") and is what the sell silent-return,
	// mail, guild-bank and load-time arms key off; the refund row alone never set it.
	_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET flags = flags | ? WHERE guid = ?", int64(itemInstanceFlagRefundable), itemGUID)
}

// vendorItemListable mirrors the SendListInventory visibility filters
// (ItemHandler.cpp:652-666): bind-on-pickup items unusable by the player's
// class, wrong-faction items, and (via the caller's stock check) sold-out
// stock are hidden from non-GMs. Unlike the buy path (Player.cpp:21941),
// both faction checks here are bitmask HasFlag checks — the buy path's
// exact-equality quirk does not apply to the list.
func (s *session) vendorItemListable(allowableClass, bonding, flagsExtra uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	if s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0 {
		return true
	}
	if bonding == 1 && (s.player.Class == 0 || allowableClass&(uint32(1)<<(s.player.Class-1)) == 0) {
		return false
	}
	team := teamForRace(s.player.Race)
	if flagsExtra&0x00000001 != 0 && team != 1 {
		return false
	}
	if flagsExtra&0x00000002 != 0 && team != 0 {
		return false
	}
	return true
}

func (s *session) vendorItemAccess(allowableClass, bonding, flagsExtra uint32) (int, bool) {
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
		// Player.cpp:21941: the Alliance-faction buy check is an exact
		// Flags2 equality (==), not a bitmask — an item carrying
		// FACTION_ALLIANCE alongside other Flags2 bits stays buyable
		// cross-faction in C++, so the client-side silent drop only fires
		// when Flags2 is exactly 0x2.
		if flagsExtra == 0x00000002 && team != 0 {
			return -1, false
		}
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

// vendorReputationPriceDiscount mirrors Player::GetReputationPriceDiscount (Player.cpp:23679-23689):
// float return, 1.0f below or at REP_NEUTRAL, else 1.0f - 0.05f*(rank - REP_NEUTRAL).
func (s *session) vendorReputationPriceDiscount(ctx context.Context, vendorEntry uint32) float32 {
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
	return 1 - 0.05*float32(rank-3)
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
	var itemEntry int64
	_ = cdb.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", result.ItemGUID).Scan(&itemEntry)
	// Undo a partial merge into a pre-existing pile first (C++ never placed
	// anything on the failed path — CanStoreNewItem is feasibility-only).
	if result.StackFillGUID != 0 && result.StackFillAmount > 0 {
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ? AND count >= ?", result.StackFillAmount, result.StackFillGUID, result.StackFillAmount)
	}
	if !result.IsStack {
		// Delete every new stack the grant created.
		for _, g := range append([]uint64{result.ItemGUID}, result.ExtraItemGUIDs...) {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, g)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", g)
		}
	}
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

// vendorRefusesSale reports whether the vendor's template carries
// CREATURE_FLAG_EXTRA_NO_SELL_VENDOR (0x1000, CreatureData.h:49):
// players can't sell items to this vendor (ItemHandler.cpp:391-395).
func (s *session) vendorRefusesSale(ctx context.Context, vendorGUID uint64) bool {
	if uint16(vendorGUID>>48) != 0xF130 || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	low := uint32(vendorGUID & 0x00FFFFFF)
	entry := uint32((vendorGUID >> 24) & 0x00FFFFFF)
	var flagsExtra int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT t.flags_extra FROM creature AS c
		JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?`, low, entry).Scan(&flagsExtra); err != nil {
		return false
	}
	return flagsExtra&0x1000 != 0
}

// sendSellError mirrors Player::SendSellError (Player.cpp:13646): SMSG_SELL_ITEM
// carries the vendor GUID (0 when the vendor itself is the problem), the item
// GUID, and the SellResult byte. Success sends result 0, which C++ never emits
// (HandleSellItemOpcode sends nothing on success) and the client ignores.
func (s *session) sendSellError(vendorGUID, itemGUID uint64, result uint8) {
	_ = s.write(uint16(protocol.OpcodeSMSG_SELL_ITEM), buildSellResult(vendorGUID, itemGUID, result), true)
}

func (s *session) handleSellItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 20 {
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
	// ItemHandler.cpp:379 — empty item GUID is silently ignored.
	if rawItemGUID == 0 {
		return true
	}
	// CMSG_SELL_ITEM carries count as uint32 (ItemHandler.cpp:376).
	count, err := reader.ReadU32()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return true
	}
	// ItemHandler.cpp:384-388 — vendor not found or not interactable.
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		s.sendSellError(0, rawItemGUID, sellErrCantFindVendor)
		return true
	}
	// ItemHandler.cpp:391-395 — CREATURE_FLAG_EXTRA_NO_SELL_VENDOR (0x1000).
	if s.vendorRefusesSale(ctx, vendorGUID) {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellToThisMerchant)
		return true
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}
	var itemEntry, currentCount, ownerGUID int64
	err = cdb.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.count, COALESCE(ii.owner_guid, 0) FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&itemEntry, &currentCount, &ownerGUID)
	// ItemHandler.cpp:476 — the item was not found.
	if err != nil || itemEntry == 0 {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantFindItem)
		return true
	}
	// ItemHandler.cpp:404-408 — prevent selling an item owned by someone else.
	if ownerGUID != 0 && uint64(ownerGUID) != s.playerGUID {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellItem)
		return true
	}
	// ItemHandler.cpp:411-415 — prevent selling a non-empty bag.
	var bagContents int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_inventory WHERE bag = ?", itemGUID).Scan(&bagContents)
	if bagContents > 0 {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellItem)
		return true
	}
	// ItemHandler.cpp:418-422 — prevent selling the currently looted item.
	if s.activeLoot != nil && s.activeLoot.TargetGUID == rawItemGUID {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellItem)
		return true
	}
	// ItemHandler.cpp:423-426 — a still-refundable item is silently ignored
	// (the client sends both CMSG_SELL_ITEM and CMSG_REFUND_ITEM on right-click).
	var refundable int64
	if cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM item_refund_instance WHERE item_guid = ? AND player_guid = ?", itemGUID, s.playerGUID).Scan(&refundable) == nil && refundable > 0 {
		return true
	}
	// ItemHandler.cpp:428-440 — count == 0 is the auto-sell-whole-stack case;
	// asking for more than the stack holds is rejected.
	if count == 0 {
		count = uint32(currentCount)
	}
	if uint64(count) > uint64(currentCount) {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellItem)
		return true
	}
	var sellPrice int64
	_ = wdb.QueryRowContext(ctx, "SELECT SellPrice FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&sellPrice)
	// ItemHandler.cpp:472-474 — SellPrice <= 0 means the merchant doesn't want it.
	if sellPrice <= 0 {
		s.sendSellError(vendorGUID, rawItemGUID, sellErrCantSellItem)
		return true
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
		// Item::CloneItem semantics: the split-off stack copies every field of
		// the source item — only the guid and the split count differ.
		_, _ = cdb.ExecContext(ctx, `INSERT INTO item_instance
			(guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
			SELECT ?, itemEntry, owner_guid, creatorGuid, ?, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text
			FROM item_instance WHERE guid = ?`, bbItemGUID, count, itemGUID)
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
	// ItemHandler.cpp:497-502 — vendor not found or not interactable.
	if !s.canInteractWithNPC(ctx, vendorGUID, uint64(unitNPCFlagVendor)) {
		s.sendSellError(0, 0, sellErrCantFindVendor)
		return true
	}
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}

	// Player::GetItemFromBuyBackSlot (Player.cpp:13550-13557): slots outside
	// [BUYBACK_SLOT_START, BUYBACK_SLOT_END) return nullptr — the raw client
	// slot is not a 0-based index.
	if slot < 74 || slot > 85 {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, 0, buyErrCantFindItem), true)
		return true
	}
	eslot := int(slot) - 74
	// ItemHandler.cpp:531-532 — empty buyback slot.
	if s.buyback[eslot] == nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, 0, buyErrCantFindItem), true)
		return true
	}

	entry := s.buyback[eslot]
	if s.player.Money < entry.Price {
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, entry.ItemEntry, 2), true) // BUY_ERR_NOT_ENOUGHT_MONEY
		return true
	}

	merged, abort := func() (bool, bool) {
		// ItemHandler.cpp:517-533: the SAME item object is stored back into the
		// inventory (Player::StoreItem(dest, pItem, true)) — enchantments,
		// durability and all other fields survive the round trip. A stackable
		// buyback item merges into an existing partial pile when it fits fully,
		// per the CanStoreItem merge arm.
		cdb := s.server.CharactersStore.DB
		if cdb == nil {
			s.sendEquipError(equipErrInvFull, entry.ItemGUID)
			return false, true
		}
		var bbRowGUID int64
		_ = cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, 74+eslot).Scan(&bbRowGUID)
		if bbRowGUID <= 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(vendorGUID, 0, buyErrCantFindItem), true)
			return false, true
		}
		if _, _, _, pileGUID, pileCount, pileMax, ok := s.findStackableInventorySlot(ctx, s.playerGUID, entry.ItemEntry); ok && pileCount+entry.Count <= pileMax {
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count + ? WHERE guid = ?", entry.Count, pileGUID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, 74+eslot)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", bbRowGUID)
			return true, false
		}
		freeBagKey, _, freeSlot, ok := s.findFreeInventorySlot(ctx, s.playerGUID)
		if !ok {
			s.sendEquipError(equipErrInvFull, entry.ItemGUID)
			return false, true
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND bag = 0 AND slot = ? AND item = ?",
			freeBagKey, freeSlot, s.playerGUID, 74+eslot, bbRowGUID)
		return false, false
	}()
	if abort {
		return true
	}

	oldItemGUID := entry.ItemGUID
	s.buyback[eslot] = nil
	// Player::RemoveItemFromBuyBackSlot (Player.cpp:13587-13588): the freed
	// slot becomes the current buyback slot only when the current slot is
	// occupied — an already-free pointer does not move.
	if s.currentBuybackSlot < 12 && s.buyback[s.currentBuybackSlot] != nil {
		s.currentBuybackSlot = uint8(eslot)
	}
	s.player.Money -= entry.Price
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	}
	s.adjustQuestItemCount(ctx, entry.ItemEntry, entry.Count, true)

	// A merged-away buyback item no longer exists client-side.
	if merged {
		s.sendDestroyObject(oldItemGUID, false)
		s.despawnItem(oldItemGUID)
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_ITEM), buildBuySucceeded(vendorGUID, entry.ItemEntry, entry.Count, entry.Count), true)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("buyback item purchased", "account", s.accountName, "item", entry.ItemEntry, "merged", merged)
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
