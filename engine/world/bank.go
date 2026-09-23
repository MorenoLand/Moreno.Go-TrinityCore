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

// handleBankerActivate processes CMSG_BANKER_ACTIVATE (0x1B7).
// Reference: WorldSession::HandleBankerActivateOpcode (BankHandler.cpp:46).
func (s *session) handleBankerActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

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
	bankerGUID, err := r.ReadU64()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	// Count purchased bank bag slots from characters table or player state
	purchasedCount := int(s.player.BankBagSlots)
	var dbSlots int64
	if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(bankSlots, 0) FROM characters WHERE guid = ?", s.playerGUID).Scan(&dbSlots); err == nil {
		purchasedCount = int(dbSlots)
	}

	res := protocol.NewBuffer(12)
	res.WriteU64(bankerGUID)
	if purchasedCount >= len(bankBagSlotPrices) {
		res.WriteU32(1) // ERR_BANKSLOT_FAILED_TOO_MANY
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
		return true
	}

	cost := bankBagSlotPrices[purchasedCount]
	if s.player.Money < cost {
		res.WriteU32(2) // ERR_BANKSLOT_INSUFFICIENT_FUNDS
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
		return true
	}

	s.player.Money -= cost
	newCount := uint8(purchasedCount + 1)
	s.player.BankBagSlots = newCount
	if _, err := cdb.ExecContext(ctx, "UPDATE characters SET money = ?, bankSlots = ? WHERE guid = ?", s.player.Money, newCount, s.playerGUID); err != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
	}

	res.WriteU32(0) // ERR_BANKSLOT_OK
	s.updateAchievementCriteria(criteriaTypeBuyBankSlot, 0, 1)
	_ = s.write(uint16(protocol.OpcodeSMSG_BUY_BANK_SLOT_RESULT), res.Bytes(), true)
	s.sendPlayerMoneyUpdate()
	s.sendPlayerUpdate()
	return true
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
	for sl := bankSlotStart; sl <= bankSlotEnd; sl++ {
		if !occupied[sl] {
			destSlot = sl
			break
		}
	}

	// 2. If 39..66 are full, check purchased bank bags (slots 67..73)
	if destSlot == 0xFF && s.player.BankBagSlots > 0 {
		maxBankBag := int(s.player.BankBagSlots)
		if maxBankBag > 7 {
			maxBankBag = 7
		}
		for i := 0; i < maxBankBag; i++ {
			bagSlot := 67 + i
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

	// Determine if item is in the bank (primary bank 39..66 or inside a bank bag)
	isBank := (srcBagKey == 0 && srcSlot >= bankSlotStart && srcSlot <= bankSlotEnd)
	if !isBank && srcBagKey != 0 {
		var isBankBagCount int
		_ = cdb.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 67 AND slot <= 73 AND item = ?", s.playerGUID, srcBagKey).Scan(&isBankBagCount)
		isBank = isBankBagCount > 0
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
		if destSlot == 0xFF && s.player.BankBagSlots > 0 {
			maxBankBag := int(s.player.BankBagSlots)
			if maxBankBag > 7 {
				maxBankBag = 7
			}
			for i := 0; i < maxBankBag; i++ {
				bagSlot := 67 + i
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
