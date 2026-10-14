package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	bankSlotPrice1 uint32 = 1000    // 10s
	bankSlotPrice2 uint32 = 10000   // 1g
	bankSlotPrice3 uint32 = 100000  // 10g
	bankSlotPrice4 uint32 = 250000  // 25g
	bankSlotPrice5 uint32 = 500000  // 50g
	bankSlotPrice6 uint32 = 1000000 // 100g
	bankSlotPrice7 uint32 = 2500000 // 250g

	bankSlotStart    uint8 = 39
	bankSlotEnd      uint8 = 66 // 28 bank item slots (39..66)
	bankBagSlotStart uint8 = 67
	bankBagSlotEnd   uint8 = 74
	bagSlotStart     uint8 = 23
	bagSlotEnd       uint8 = 38 // 16 backpack slots (23..38)

	// Container subclasses from ItemTemplate.h:334-343 / 485-486.
	itemSubClassContainer               uint32 = 0
	itemSubClassSoulContainer           uint32 = 1
	itemSubClassHerbContainer           uint32 = 2
	itemSubClassEnchantingContainer     uint32 = 3
	itemSubClassEngineeringContainer    uint32 = 4
	itemSubClassGemContainer            uint32 = 5
	itemSubClassMiningContainer         uint32 = 6
	itemSubClassLeatherworkingContainer uint32 = 7
	itemSubClassInscriptionContainer    uint32 = 8
	itemSubClassQuiver                  uint32 = 2 // ITEM_CLASS_QUIVER subclass
	itemSubClassAmmoPouch               uint32 = 3 // ITEM_CLASS_QUIVER subclass

	// BAG_FAMILY_MASK from ItemTemplate.h:230-248 (Keys/CurrencyTokens live in guild.go).
	bagFamilyMaskArrows             uint32 = 0x00000001
	bagFamilyMaskBullets            uint32 = 0x00000002
	bagFamilyMaskSoulShards         uint32 = 0x00000004
	bagFamilyMaskLeatherworkingSupp uint32 = 0x00000008
	bagFamilyMaskInscriptionSupp    uint32 = 0x00000010
	bagFamilyMaskHerbs              uint32 = 0x00000020
	bagFamilyMaskEnchantingSupp     uint32 = 0x00000040
	bagFamilyMaskEngineeringSupp    uint32 = 0x00000080
	bagFamilyMaskGems               uint32 = 0x00000200
	bagFamilyMaskMiningSupp         uint32 = 0x00000400
)

var bankBagSlotPrices = []uint32{
	bankSlotPrice1,
	bankSlotPrice2,
	bankSlotPrice3,
	bankSlotPrice4,
	bankSlotPrice5,
	bankSlotPrice6,
	bankSlotPrice7,
}

// BuyBankSlot result codes mirror BuyBankSlotResultCode (Player.h:113-116):
// note the non-obvious order — OK is 3, not 0.
const (
	bankSlotResultFailedTooMany     uint32 = 0 // ERR_BANKSLOT_FAILED_TOO_MANY
	bankSlotResultInsufficientFunds uint32 = 1 // ERR_BANKSLOT_INSUFFICIENT_FUNDS
	bankSlotResultNotBanker         uint32 = 2 // ERR_BANKSLOT_NOTBANKER
	bankSlotResultOK                uint32 = 3 // ERR_BANKSLOT_OK
)

// canUseBank mirrors WorldSession::CanUseBank (BankHandler.cpp:32-44): a zero
// banker GUID falls back to the banker the player last talked to
// (m_currentBankerGUID, latched by SendShowBank). The ".bank" command path
// (banker GUID == own GUID == current banker) bypasses the interact check;
// everything else needs an interactable UNIT_NPC_FLAG_BANKER creature.
func (s *session) canUseBank(ctx context.Context, bankerGUID uint64) bool {
	if bankerGUID == 0 {
		bankerGUID = s.currentBankerGUID
	}
	if bankerGUID != 0 && bankerGUID == s.playerGUID && bankerGUID == s.currentBankerGUID {
		return true
	}
	if bankerGUID == 0 {
		return false
	}
	return s.canInteractWithNPC(ctx, bankerGUID, uint64(unitNPCFlagBanker))
}

// handleBankerActivate processes CMSG_BANKER_ACTIVATE (0x1B7).
// Reference: WorldSession::HandleBankerActivateOpcode (BankHandler.cpp:46).
func (s *session) handleBankerActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	// NPC::Hello::Read (NPCPackets.cpp:20-23) does `_worldPacket >> Unit`;
	// ByteBuffer's operator>>(ObjectGuid&) reads a raw 8-byte GUID
	// (ObjectGuid.cpp:76-80), not a packed one.
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	// BankHandler.cpp:46-53 (HandleBankerActivateOpcode): the NPC must be
	// interactable as a banker (UNIT_NPC_FLAG_BANKER), otherwise silent drop.
	if !s.canInteractWithNPC(ctx, bankerGUID, uint64(unitNPCFlagBanker)) {
		return true
	}
	// BankHandler.cpp:56-57 — activating the banker strips a faked death.
	s.removeFakeDeath()

	// SendShowBank (BankHandler.cpp:183-188) latches the banker GUID
	// (m_currentBankerGUID) before sending SMSG_SHOW_BANK, which carries only
	// the banker GUID (8 bytes).
	s.currentBankerGUID = bankerGUID
	res := protocol.NewBuffer(8)
	res.WriteU64(bankerGUID)
	return s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), res.Bytes(), true) == nil
}

// handleBuyBankSlot processes CMSG_BUY_BANK_SLOT (0x1B9).
// Reference: WorldSession::HandleBuyBankSlotOpcode (BankHandler.cpp:138).
func (s *session) handleBuyBankSlot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	// Bank::BuyBankSlot::Read (BankPackets.cpp:32-35) does
	// `_worldPacket >> Banker` — a raw 8-byte GUID read
	// (ObjectGuid.cpp:76-80), not packed.
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	// BankHandler.cpp:141-146: the banker must be usable first, otherwise the
	// result packet answers ERR_BANKSLOT_NOTBANKER (not a silent drop).
	res := protocol.NewBuffer(4)
	if !s.canUseBank(ctx, bankerGUID) {
		res.WriteU32(bankSlotResultNotBanker)
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
		return true
	}

	// Count purchased bank bag slots from characters table or player state
	purchasedCount := int(s.player.BankBagSlots)
	var dbSlots int64
	if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(bankSlots, 0) FROM characters WHERE guid = ?", s.playerGUID).Scan(&dbSlots); err == nil {
		purchasedCount = int(dbSlots)
	}

	// C++ looks up BankBagSlotPrices[owned+1]; a missing row means too many.
	// Go's price table has one row per purchasable slot, so owned >= len is
	// the same gate.
	if purchasedCount >= len(bankBagSlotPrices) {
		res.WriteU32(bankSlotResultFailedTooMany)
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
		return true
	}

	cost := bankBagSlotPrices[purchasedCount]
	if s.player.Money < cost {
		res.WriteU32(bankSlotResultInsufficientFunds)
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
		return true
	}

	s.player.Money -= cost
	s.questMoneyChanged(ctx)
	newCount := uint8(purchasedCount + 1)
	s.player.BankBagSlots = newCount
	if _, err := cdb.ExecContext(ctx, "UPDATE characters SET money = ?, bankSlots = ? WHERE guid = ?", s.player.Money, newCount, s.playerGUID); err != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	}

	// SMSG_BUY_BANK_SLOT_RESULT carries only the u32 result (BankPackets.h:60-67);
	// the banker GUID is not echoed back.
	res.WriteU32(bankSlotResultOK)
	s.updateAchievementCriteria(criteriaTypeBuyBankSlot, 0, 1)
	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	return true
}

// itemCanGoIntoBag ports ItemCanGoIntoBag (Item.cpp:181-244): a plain
// container (ITEM_CLASS_CONTAINER / ITEM_SUBCLASS_CONTAINER) accepts anything
// (including empty bags); specialized containers accept only items carrying
// the matching BagFamily mask; quivers/ammo pouches accept arrows/bullets
// family items.
func itemCanGoIntoBag(bagClass, bagSubClass uint32, itemBagFamily uint32) bool {
	switch bagClass {
	case itemClassContainer:
		switch bagSubClass {
		case itemSubClassContainer:
			return true
		case itemSubClassSoulContainer:
			return itemBagFamily&bagFamilyMaskSoulShards != 0
		case itemSubClassHerbContainer:
			return itemBagFamily&bagFamilyMaskHerbs != 0
		case itemSubClassEnchantingContainer:
			return itemBagFamily&bagFamilyMaskEnchantingSupp != 0
		case itemSubClassEngineeringContainer:
			return itemBagFamily&bagFamilyMaskEngineeringSupp != 0
		case itemSubClassGemContainer:
			return itemBagFamily&bagFamilyMaskGems != 0
		case itemSubClassMiningContainer:
			return itemBagFamily&bagFamilyMaskMiningSupp != 0
		case itemSubClassLeatherworkingContainer:
			return itemBagFamily&bagFamilyMaskLeatherworkingSupp != 0
		case itemSubClassInscriptionContainer:
			return itemBagFamily&bagFamilyMaskInscriptionSupp != 0
		default:
			return false
		}
	case itemClassQuiver:
		switch bagSubClass {
		case itemSubClassQuiver:
			return itemBagFamily&bagFamilyMaskArrows != 0
		case itemSubClassAmmoPouch:
			return itemBagFamily&bagFamilyMaskBullets != 0
		default:
			return false
		}
	}
	return false
}

// findFreeBankBagSlot ports the free-place bank-bag legs of Player::CanBankItem's
// auto path (Player.cpp:11783-11830). The merge legs are moot (Go moves whole
// items), so this runs the two free-place legs: for items with a BagFamily,
// specialized family-matching bank bags are tried first, then plain
// containers; items with no BagFamily only consider plain containers.
// CanStoreItem_InBag's non_specialized gate (Player.cpp:10597) rejects
// plain containers on the specialized pass and specialized bags on the plain
// pass, and ItemCanGoIntoBag rejects family-mismatched bags (a sword never
// lands in a soul bag). Empty bags nest in plain containers per ItemCanGoIntoBag's
// plain arm.
func (s *session) findFreeBankBagSlot(ctx context.Context, itemEntry uint32) (int64, uint8, bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil ||
		s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, 0, false
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	var itemBagFamily uint32
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(BagFamily, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&itemBagFamily)

	maxBankBag := int(s.player.BankBagSlots)
	if maxBankBag > 7 {
		maxBankBag = 7
	}
	type bankBag struct {
		guid     int64
		class    uint32
		subclass uint32
	}
	bags := make([]bankBag, 0, maxBankBag)
	for i := 0; i < maxBankBag; i++ {
		var bagGUID int64
		_ = cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1", s.playerGUID, 67+i).Scan(&bagGUID)
		if bagGUID == 0 {
			continue
		}
		var class, subclass uint32
		if err := wdb.QueryRowContext(ctx, `SELECT COALESCE(class, 0), COALESCE(subclass, 0) FROM item_template WHERE entry = (SELECT itemEntry FROM item_instance WHERE guid = ?)`, bagGUID).Scan(&class, &subclass); err != nil {
			continue
		}
		bags = append(bags, bankBag{guid: bagGUID, class: class, subclass: subclass})
	}
	isPlain := func(b bankBag) bool {
		return b.class == itemClassContainer && b.subclass == itemSubClassContainer
	}
	// Pass 1 (Player.cpp:11786-11796): BagFamily items try specialized
	// family-matching bags first.
	if itemBagFamily != 0 {
		for _, b := range bags {
			if isPlain(b) || !itemCanGoIntoBag(b.class, b.subclass, itemBagFamily) {
				continue
			}
			if slot, ok := s.freeInventorySlot(ctx, b.guid); ok {
				return b.guid, slot, true
			}
		}
	}
	// Pass 2 (Player.cpp:11822-11830): plain containers.
	for _, b := range bags {
		if !isPlain(b) {
			continue
		}
		if slot, ok := s.freeInventorySlot(ctx, b.guid); ok {
			return b.guid, slot, true
		}
	}
	return 0, 0, false
}

// handleAutoBankItem processes CMSG_AUTOBANK_ITEM (0x283).
// Reference: WorldSession::HandleAutoBankItemOpcode (BankHandler.cpp:62).
func (s *session) handleAutoBankItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	srcBag, err := r.ReadU8()
	if err != nil {
		return false
	}
	srcSlot, err := r.ReadU8()
	if err != nil {
		return false
	}

	// BankHandler.cpp:66-70: silent drop without an interactable banker —
	// the bank window is a banker-scoped session, not a free action.
	if !s.canUseBank(ctx, 0) {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	srcBagKey, ok := s.inventoryBagKey(ctx, srcBag)
	if !ok {
		return true
	}

	var itemGUID, itemEntry, itemCount int64
	err = cdb.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? AND ci.item != 0 LIMIT 1`, s.playerGUID, srcBagKey, srcSlot).Scan(&itemGUID, &itemEntry, &itemCount)
	if err != nil || itemGUID == 0 || itemEntry <= 0 || itemCount <= 0 {
		return true
	}

	// Player::CanBankItem's auto path reaches CanStoreItem_InInventorySlots
	// (Player.cpp:10654), whose top arm rejects a non-empty bag with
	// EQUIP_ERR_CAN_ONLY_DO_WITH_EMPTY_BAGS before any slot scan.
	if s.itemIsNonemptyBag(ctx, uint64(itemGUID)) {
		s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(itemGUID))
		return true
	}

	// AUDIT vs Player::CanBankItem (Player.cpp:11671) auto path
	// (NULL_BAG/NULL_SLOT): the remaining validation arms have no Go
	// counterpart — m_lootGenerated (Go has no loot-on-item model),
	// IsBindedNotWith (Go tracks no binding state), currency-token slots
	// (Go has no hidden currency bag), and CanTakeMoreSimilarItems
	// (stack-merge is unmodeled tree-wide; Go moves whole items).
	// CanStoreItem_InInventorySlots treats the moved item's own slot as
	// empty (Player.cpp:10663-10667), so a bank-src item CAN resolve dest
	// to its own slot — the dest==own-pos EQUIP_ERR_NONE guard
	// (BankHandler.cpp:79-83) is ported below, not a no-op.
	// 1. Find free bank slot among 39..66
	occupied := make(map[uint8]bool)
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= ? AND slot <= ?", s.playerGUID, bankSlotStart, bankSlotEnd)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sl uint8
			if rows.Scan(&sl) == nil {
				occupied[sl] = true
			}
		}
	}

	var destBagKey int64 = 0
	var destSlot uint8 = 0xFF
	// The moved item's own slot scans as empty (Player.cpp:10663-10667).
	if srcBagKey == 0 && srcSlot >= bankSlotStart && srcSlot <= bankSlotEnd {
		delete(occupied, srcSlot)
	}
	for sl := bankSlotStart; sl <= bankSlotEnd; sl++ {
		if !occupied[sl] {
			destSlot = sl
			break
		}
	}

	// dest==own-pos guard (BankHandler.cpp:79-83): with the own slot
	// scanning as empty the search can resolve to it — answer
	// EQUIP_ERR_NONE instead of moving.
	if destBagKey == 0 && srcBagKey == 0 && destSlot == srcSlot && srcSlot >= bankSlotStart && srcSlot <= bankSlotEnd {
		s.sendEquipError(equipErrNone, uint64(itemGUID))
		return true
	}

	// 2. If 39..66 are full, check purchased bank bags (slots 67..73) with
	// CanBankItem's auto-path bank-bag legs (Player.cpp:11783-11830):
	// family-matching specialized bags first for BagFamily items, then plain
	// containers. ItemCanGoIntoBag's plain arm accepts anything, so empty
	// bags may nest in plain-container bank bags (bffff2c8's !srcIsBag gate
	// was wrong — C++ has no bag-in-bag rejection on this path).
	if destSlot == 0xFF {
		if bagKey, slot, ok := s.findFreeBankBagSlot(ctx, uint32(itemEntry)); ok {
			destBagKey = bagKey
			destSlot = slot
		}
	}

	if destSlot == 0xFF {
		s.sendEquipError(equipErrBankFull, uint64(itemGUID))
		return true // Bank full
	}

	if _, err := cdb.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", destBagKey, destSlot, s.playerGUID, itemGUID); err != nil {
		return false
	}
	s.adjustQuestItemCount(ctx, uint32(itemEntry), uint32(itemCount), false)
	if srcBagKey == 0 && srcSlot < equipSlotEnd {
		s.syncEquipmentCache(ctx)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}

// handleAutoStoreBankItem processes CMSG_AUTOSTORE_BANK_ITEM (0x282).
// Reference: WorldSession::HandleAutoStoreBankItemOpcode (BankHandler.cpp:95).
func (s *session) handleAutoStoreBankItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	srcBag, err := r.ReadU8()
	if err != nil {
		return false
	}
	srcSlot, err := r.ReadU8()
	if err != nil {
		return false
	}

	// BankHandler.cpp:99-103: silent drop without an interactable banker.
	if !s.canUseBank(ctx, 0) {
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	srcBagKey, ok := s.inventoryBagKey(ctx, srcBag)
	if !ok {
		return true
	}

	var itemGUID, itemEntry, itemCount int64
	err = cdb.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? AND ci.item != 0 LIMIT 1`, s.playerGUID, srcBagKey, srcSlot).Scan(&itemGUID, &itemEntry, &itemCount)
	if err != nil || itemGUID == 0 || itemEntry <= 0 || itemCount <= 0 {
		return true
	}

	// Player::IsBankPos (Player.cpp:10163-10172): bank item slots 39..66 AND
	// bank bag slots 67..73 count as bank (arm 2), plus anything inside a
	// bank bag (arm 3).
	isBank := (srcBagKey == 0 && srcSlot >= bankSlotStart && srcSlot < bankBagSlotEnd)
	if !isBank && srcBagKey != 0 {
		var isBankBagCount int
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 67 AND slot <= 73 AND item = ?", s.playerGUID, srcBagKey).Scan(&isBankBagCount)
		isBank = isBankBagCount > 0
	}

	// Bank -> inventory auto-store hits CanStoreItem's IsNotEmptyBag term
	// (Player.cpp:11088, EQUIP_ERR_NONEMPTY_BAG_OVER_OTHER_BAG); the
	// inventory -> bank path reaches the same item through
	// CanStoreItem_InInventorySlots (Player.cpp:10654,
	// EQUIP_ERR_CAN_ONLY_DO_WITH_EMPTY_BAGS). Both fire before any
	// free-slot scan, so they are checked before the branch bodies.
	if s.itemIsNonemptyBag(ctx, uint64(itemGUID)) {
		if isBank {
			s.sendEquipError(equipErrNonemptyBagOverOtherBag, uint64(itemGUID))
		} else {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(itemGUID))
		}
		return true
	}
	if isBank {
		// Move from bank to backpack (slots 23..38), or into equipped bags (19..22)
		occupied := make(map[uint8]bool)
		rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= ? AND slot <= ?", s.playerGUID, bagSlotStart, bagSlotEnd)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sl uint8
				if rows.Scan(&sl) == nil {
					occupied[sl] = true
				}
			}
		}
		var destBagKey int64 = 0
		var destSlot uint8 = 0xFF
		for sl := bagSlotStart; sl <= bagSlotEnd; sl++ {
			if !occupied[sl] {
				destSlot = sl
				break
			}
		}
		if destSlot == 0xFF {
			// Check equipped bags 19..22
			for bagSlot := int(invSlotBagStart); bagSlot < int(invSlotBagEnd); bagSlot++ {
				var bagGUID int64
				_ = cdb.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? AND item != 0 LIMIT 1", s.playerGUID, bagSlot).Scan(&bagGUID)
				if bagGUID == 0 {
					continue
				}
				freeSlot, ok := s.freeInventorySlot(ctx, bagGUID)
				if ok {
					destBagKey = bagGUID
					destSlot = freeSlot
					break
				}
			}
		}
		if destSlot == 0xFF {
			s.sendEquipError(equipErrInvFull, uint64(itemGUID))
			return true // Inventory full
		}
		if _, err := cdb.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", destBagKey, destSlot, s.playerGUID, itemGUID); err != nil {
			return false
		}
	} else {
		// Move from player bags to bank
		occupied := make(map[uint8]bool)
		rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= ? AND slot <= ?", s.playerGUID, bankSlotStart, bankSlotEnd)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sl uint8
				if rows.Scan(&sl) == nil {
					occupied[sl] = true
				}
			}
		}
		var destBagKey int64 = 0
		var destSlot uint8 = 0xFF
		for sl := bankSlotStart; sl <= bankSlotEnd; sl++ {
			if !occupied[sl] {
				destSlot = sl
				break
			}
		}
		// Bank-bag fallback with CanBankItem's auto-path legs
		// (Player.cpp:11783-11830): family-matching specialized bags first
		// for BagFamily items, then plain containers; ItemCanGoIntoBag's
		// plain arm accepts anything, so empty bags may nest.
		if destSlot == 0xFF {
			if bagKey, slot, ok := s.findFreeBankBagSlot(ctx, uint32(itemEntry)); ok {
				destBagKey = bagKey
				destSlot = slot
			}
		}
		if destSlot == 0xFF {
			s.sendEquipError(equipErrBankFull, uint64(itemGUID))
			return true // Bank full
		}
		if _, err := cdb.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", destBagKey, destSlot, s.playerGUID, itemGUID); err != nil {
			return false
		}
	}
	if isBank {
		s.adjustQuestItemCount(ctx, uint32(itemEntry), uint32(itemCount), true)
	} else {
		s.adjustQuestItemCount(ctx, uint32(itemEntry), uint32(itemCount), false)
	}

	if srcBagKey == 0 && srcSlot < equipSlotEnd {
		s.syncEquipmentCache(ctx)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}
