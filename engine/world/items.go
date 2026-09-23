package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	itemFlagUniqueEquippable uint32 = 0x00080000
	itemClassQuiver          uint32 = 11
	defaultMaxPlayerLevel    uint32 = 80
	itemBagFamilyKeys        uint32 = 0x00000100
	itemBagFamilyCurrency    uint32 = 0x00002000
	itemSubClassPolearm      uint32 = 6
	itemSubClassStaff        uint32 = 10
)

func playerHasSpell(state *playerState, spellID uint32) bool {
	if state == nil || spellID == 0 {
		return false
	}
	for _, spell := range state.Spells {
		if spell.ID == spellID && !spell.Disabled {
			return true
		}
	}
	return false
}

func socketGemEnchantmentIDs(enchantments string) []uint32 {
	fields := strings.Fields(enchantments)
	ids := make([]uint32, 0, 3)
	for slot := 2; slot < 5; slot++ {
		index := slot * 3
		if index >= len(fields) {
			continue
		}
		id, err := strconv.ParseUint(fields[index], 10, 32)
		if err == nil && id != 0 {
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

func itemFitsEquipmentSlot(state *playerState, equipped map[int64]itemQueryData, item itemQueryData, slot int64) bool {
	dualWield, titanGrip := playerHasSpell(state, 674), playerHasSpell(state, 46917)
	var slots []int64
	switch item.InventoryType {
	case 1:
		slots = []int64{0}
	case 2:
		slots = []int64{1}
	case 3:
		slots = []int64{2}
	case 4:
		slots = []int64{3}
	case 5, 20:
		slots = []int64{4}
	case 6:
		slots = []int64{5}
	case 7:
		slots = []int64{6}
	case 8:
		slots = []int64{7}
	case 9:
		slots = []int64{8}
	case 10:
		slots = []int64{9}
	case 11:
		slots = []int64{10, 11}
	case 12:
		slots = []int64{12, 13}
	case 13:
		slots = []int64{15}
		if dualWield {
			slots = append(slots, 16)
		}
	case 14, 22, 23:
		slots = []int64{16}
	case 15, 25, 26:
		slots = []int64{17}
	case 16:
		slots = []int64{14}
	case 17, 21:
		slots = []int64{15}
		if item.InventoryType == 17 && dualWield && titanGrip {
			slots = append(slots, 16)
		}
	case 19:
		slots = []int64{18}
	case 28:
		switch item.SubClass {
		case 0:
			if state.Class == 9 {
				slots = []int64{17}
			}
		case 7:
			if state.Class == 2 {
				slots = []int64{17}
			}
		case 8:
			if state.Class == 11 {
				slots = []int64{17}
			}
		case 9:
			if state.Class == 7 {
				slots = []int64{17}
			}
		case 10:
			if state.Class == 6 {
				slots = []int64{17}
			}
		}
	}
	for _, candidate := range slots {
		if candidate != slot {
			continue
		}
		if slot == 16 {
			if item.InventoryType == 13 && item.Class == 2 && item.SubClass == 6 || (item.InventoryType == 13 || item.InventoryType == 22) && !dualWield {
				return false
			}
			if item.InventoryType == 17 && (!dualWield || !titanGrip) {
				return false
			}
			if item.InventoryType == 17 && (item.SubClass == itemSubClassPolearm || item.SubClass == itemSubClassStaff) {
				return false
			}
			if mainHand, ok := equipped[15]; ok && mainHand.InventoryType == 17 && !titanGrip {
				return false
			}
		}
		return true
	}
	return false
}

func (s *session) canUseItemTemplate(ctx context.Context, entry uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	data, err := s.loadItemQueryData(ctx, entry)
	if err != nil {
		return false
	}
	return s.canUseItemData(ctx, s.player, data)
}

func (s *session) canUseItemData(ctx context.Context, state *playerState, data itemQueryData) bool {
	if s == nil || state == nil || uint32(state.Level) < data.RequiredLevel {
		return false
	}
	if data.ScalingStatDistribution != 0 && s.server.Data != nil {
		if maxLevel, found, err := s.server.Data.ScalingStatDistributionMaxLevel(data.ScalingStatDistribution); err == nil && found && maxLevel < defaultMaxPlayerLevel && maxLevel < uint32(state.Level) {
			return false
		}
	}
	weaponSkills := [...]uint32{44, 172, 45, 46, 54, 160, 229, 43, 55, 0, 136, 0, 0, 473, 0, 173, 176, 253, 226, 228, 356}
	weaponSpells := [...]uint32{196, 197, 264, 266, 198, 199, 200, 201, 202, 0, 227, 0, 0, 0, 0, 1180, 2567, 3386, 5011, 5009, 0}
	armorSkills := [...]uint32{0, 415, 414, 413, 293, 0, 433, 0, 0, 0, 0}
	armorSpells := [...]uint32{0, 9078, 9077, 8737, 750, 0, 9116, 0, 0, 0, 0}
	itemSkill, itemSpell := uint32(0), uint32(0)
	switch data.Class {
	case itemClassWeapon:
		if data.SubClass < uint32(len(weaponSkills)) {
			itemSkill, itemSpell = weaponSkills[data.SubClass], weaponSpells[data.SubClass]
		}
	case itemClassArmor:
		if data.SubClass < uint32(len(armorSkills)) {
			itemSkill, itemSpell = armorSkills[data.SubClass], armorSpells[data.SubClass]
		}
	}
	hasSkill := func(skillID uint32) bool {
		if skillID == 0 {
			return false
		}
		for _, skill := range state.Skills {
			if uint32(skill.Skill) == skillID {
				return true
			}
		}
		return false
	}
	skillValue := func(skillID uint32) uint16 {
		for _, skill := range state.Skills {
			if uint32(skill.Skill) == skillID {
				return skill.Value
			}
		}
		return 0
	}
	maxSkill := func(skillID uint32) {
		if skillID == 0 || skillID > 0xffff || skillValue(skillID) != 0 {
			return
		}
		for i := range state.Skills {
			if uint32(state.Skills[i].Skill) == skillID {
				state.Skills[i].Value, state.Skills[i].Max = 400, 400
				if db := s.server.CharactersStore.DB; db != nil {
					_, _ = db.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, 400, 400)", state.GUID, skillID)
				}
				return
			}
		}
		state.Skills = append(state.Skills, playerSkill{Skill: uint16(skillID), Value: 400, Max: 400})
		if db := s.server.CharactersStore.DB; db != nil {
			_, _ = db.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, 400, 400)", state.GUID, skillID)
		}
	}
	if playerHasSpell(state, itemSpell) || hasSkill(itemSkill) {
		maxSkill(itemSkill)
		return true
	}
	team := playerTeam(state.Race)
	if (data.Flags2&0x01 != 0 && team != teamHorde) || (data.Flags2&0x02 != 0 && team != teamAlliance) {
		return false
	}
	classMask, raceMask := uint32(0), uint32(0)
	if state.Class > 0 {
		classMask = uint32(1) << uint(state.Class-1)
	}
	if state.Race > 0 {
		raceMask = uint32(1) << uint(state.Race-1)
	}
	if data.AllowableClass&classMask == 0 || data.AllowableRace&raceMask == 0 {
		return false
	}
	templateBypass := playerHasSpell(state, data.RequiredSpell) || hasSkill(data.RequiredSkill)
	if templateBypass {
		maxSkill(data.RequiredSkill)
	}
	if !templateBypass {
		if data.RequiredSkill != 0 {
			requiredSkillValue := skillValue(data.RequiredSkill)
			if requiredSkillValue == 0 || uint32(requiredSkillValue) < data.RequiredSkillRank {
				return false
			}
		}
		if data.RequiredSpell != 0 && !playerHasSpell(state, data.RequiredSpell) {
			return false
		}
		if data.HolidayID != 0 {
			if _, active := s.server.cachedActiveGameHolidays(ctx)[int64(data.HolidayID)]; !active {
				return false
			}
		}
		if (data.Spells[0].ID == 483 || data.Spells[0].ID == 55884) && data.Spells[1].ID > 0 && playerHasSpell(state, uint32(data.Spells[1].ID)) {
			return false
		}
	}
	if itemSkill != 0 && s.getSkillValue(itemSkill) == 0 {
		allowHeirloom := false
		if data.Quality == 7 && data.Class == itemClassArmor && !hasSkill(itemSkill) {
			switch s.player.Class {
			case 3, 7:
				allowHeirloom = itemSkill == 413
			case 1, 2:
				allowHeirloom = itemSkill == 293
			}
		}
		if !allowHeirloom {
			return false
		}
	}
	if data.RequiredReputationFaction != 0 {
		rank := uint32(3)
		for _, reputation := range state.Reputations {
			if reputation.FactionID == data.RequiredReputationFaction {
				rank = reputationRank(int64(totalReputationStanding(reputation)))
				break
			}
		}
		if rank < data.RequiredReputationRank {
			return false
		}
	}
	return true
}

const (
	equipSlotHead     uint8 = 0
	equipSlotNeck     uint8 = 1
	equipSlotShoulder uint8 = 2
	equipSlotBody     uint8 = 3
	equipSlotChest    uint8 = 4
	equipSlotWaist    uint8 = 5
	equipSlotLegs     uint8 = 6
	equipSlotFeet     uint8 = 7
	equipSlotWrists   uint8 = 8
	equipSlotHands    uint8 = 9
	equipSlotFinger1  uint8 = 10
	equipSlotFinger2  uint8 = 11
	equipSlotTrinket1 uint8 = 12
	equipSlotTrinket2 uint8 = 13
	equipSlotBack     uint8 = 14
	equipSlotMainhand uint8 = 15
	equipSlotOffhand  uint8 = 16
	equipSlotRanged   uint8 = 17
	equipSlotTabard   uint8 = 18
	equipSlotEnd      uint8 = 19

	invSlotBagStart  uint8 = 19
	invSlotBagEnd    uint8 = 23
	invSlotItemStart uint8 = 23
	invSlotItemEnd   uint8 = 39

	invSlotBag0 uint8 = 255
)

func (s *session) findFreeBackpackSlot(ctx context.Context) (uint8, bool) {
	db := s.server.CharactersStore.DB
	if db == nil {
		return 0, false
	}
	occupied := make(map[uint8]bool)
	rows, err := db.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= ? AND slot <= ?", s.playerGUID, invSlotItemStart, invSlotItemEnd-1)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sl uint8
			if err := rows.Scan(&sl); err == nil {
				occupied[sl] = true
			}
		}
	}
	for sl := invSlotItemStart; sl < invSlotItemEnd; sl++ {
		if !occupied[sl] {
			return sl, true
		}
	}
	return 0, false
}

func (s *session) isBagEmpty(ctx context.Context, bagItemGUID int64) bool {
	db := s.server.CharactersStore.DB
	if db == nil || bagItemGUID == 0 {
		return true
	}
	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_inventory WHERE guid = ? AND bag = ?", s.playerGUID, bagItemGUID).Scan(&count)
	return count == 0
}

func (s *session) swapInventoryCoordinates(ctx context.Context, itemA, bagA, slotA, itemB, bagB, slotB int64) {
	db := s.server.CharactersStore.DB
	if db == nil {
		return
	}
	if itemA != 0 && itemB != 0 {
		_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 254, slot = 254 WHERE guid = ? AND item = ?", s.playerGUID, itemA)
		_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", bagA, slotA, s.playerGUID, itemB)
		_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", bagB, slotB, s.playerGUID, itemA)
	} else if itemA != 0 {
		_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", bagB, slotB, s.playerGUID, itemA)
	} else if itemB != 0 {
		_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", bagA, slotA, s.playerGUID, itemB)
	}
}

func (s *session) despawnItem(itemGUID uint64) {
	if s == nil || itemGUID == 0 {
		return
	}
	fullGUID := itemGUID
	if fullGUID <= 0xFFFFFFFF {
		fullGUID = itemGUID | (uint64(0x4000) << 48)
	}
	updates := protocol.NewUpdateData()
	updates.AddOutOfRangeGUID(fullGUID)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
}

func (s *session) tryMergeStacks(ctx context.Context, srcItemGUID, srcBagKey int64, srcSlot uint8, dstItemGUID, dstBagKey int64, dstSlot uint8) (bool, error) {
	if srcItemGUID == 0 || dstItemGUID == 0 || srcItemGUID == dstItemGUID {
		return false, nil
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return false, nil
	}
	var srcEntry, srcCount int64
	if err := db.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", srcItemGUID).Scan(&srcEntry, &srcCount); err != nil {
		return false, nil
	}
	var dstEntry, dstCount int64
	if err := db.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", dstItemGUID).Scan(&dstEntry, &dstCount); err != nil {
		return false, nil
	}
	if srcEntry != dstEntry || srcEntry == 0 {
		return false, nil
	}
	if srcCount <= 0 {
		srcCount = 1
	}
	if dstCount <= 0 {
		dstCount = 1
	}
	var maxStack int64 = 1
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(stackable, 1) FROM item_template WHERE entry = ?", srcEntry).Scan(&maxStack)
	}
	if maxStack <= 1 || dstCount >= maxStack {
		return false, nil
	}
	freeSpace := maxStack - dstCount
	if srcCount <= freeSpace {
		newDstCount := dstCount + srcCount
		_, _ = db.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", newDstCount, dstItemGUID)
		_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, srcBagKey, srcSlot)
		_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", srcItemGUID)
		s.despawnItem(uint64(srcItemGUID))
	} else {
		newDstCount := maxStack
		newSrcCount := srcCount - freeSpace
		_, _ = db.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", newDstCount, dstItemGUID)
		_, _ = db.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", newSrcCount, srcItemGUID)
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true, nil
}

func (s *session) chooseAutoEquipSlot(ctx context.Context, invType uint8, itemEntry int64) (uint8, bool) {
	db := s.server.CharactersStore.DB
	if db == nil {
		return 255, false
	}
	switch invType {
	case 11: // Finger: slot 10 or 11
		var g1, g2 int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 10", s.playerGUID).Scan(&g1)
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 11", s.playerGUID).Scan(&g2)
		if g1 == 0 {
			return equipSlotFinger1, true
		}
		if g2 == 0 {
			return equipSlotFinger2, true
		}
		return equipSlotFinger1, true
	case 12: // Trinket: slot 12 or 13
		var g1, g2 int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 12", s.playerGUID).Scan(&g1)
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 13", s.playerGUID).Scan(&g2)
		if g1 == 0 {
			return equipSlotTrinket1, true
		}
		if g2 == 0 {
			return equipSlotTrinket2, true
		}
		return equipSlotTrinket1, true
	case 18: // Bag: slots 19..22
		for sl := invSlotBagStart; sl < invSlotBagEnd; sl++ {
			var g int64
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ?", s.playerGUID, sl).Scan(&g)
			if g == 0 {
				return sl, true
			}
		}
		return invSlotBagStart, true
	case 13: // One-Hand Weapon: slot 15 or slot 16
		var mh, oh int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 15", s.playerGUID).Scan(&mh)
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = 16", s.playerGUID).Scan(&oh)
		if mh == 0 {
			return equipSlotMainhand, true
		}
		if oh == 0 {
			return equipSlotOffhand, true
		}
		return equipSlotMainhand, true
	default:
		slot := inventoryTypeToSlot(invType)
		if slot >= equipSlotEnd && (slot < invSlotBagStart || slot >= invSlotBagEnd) {
			return 255, false
		}
		return slot, true
	}
}

func (s *session) isSlotValidForItem(invType uint8, slot uint8) bool {
	switch slot {
	case equipSlotHead:
		return invType == 1
	case equipSlotNeck:
		return invType == 2
	case equipSlotShoulder:
		return invType == 3
	case equipSlotBody:
		return invType == 4
	case equipSlotChest:
		return invType == 5 || invType == 20
	case equipSlotWaist:
		return invType == 6
	case equipSlotLegs:
		return invType == 7
	case equipSlotFeet:
		return invType == 8
	case equipSlotWrists:
		return invType == 9
	case equipSlotHands:
		return invType == 10
	case equipSlotFinger1, equipSlotFinger2:
		return invType == 11
	case equipSlotTrinket1, equipSlotTrinket2:
		return invType == 12
	case equipSlotBack:
		return invType == 16
	case equipSlotMainhand:
		return invType == 13 || invType == 17 || invType == 21
	case equipSlotOffhand:
		return invType == 13 || invType == 14 || invType == 22 || invType == 23
	case equipSlotRanged:
		return invType == 15 || invType == 25 || invType == 26
	case equipSlotTabard:
		return invType == 19
	case 19, 20, 21, 22, 67, 68, 69, 70, 71, 72, 73:
		return invType == 18
	default:
		return true
	}
}

func (s *session) handleAutoEquipItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	srcBag := payload[0]
	srcSlot := payload[1]
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	srcBagKey, ok := s.inventoryBagKey(ctx, srcBag)
	if !ok {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}
	var itemGUID, itemEntry int64
	err := db.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, srcBagKey, srcSlot).Scan(&itemGUID, &itemEntry)
	if err != nil || itemGUID == 0 || itemEntry == 0 {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}
	var invType int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&invType)
	}
	destSlot, ok := s.chooseAutoEquipSlot(ctx, uint8(invType), itemEntry)
	if !ok {
		s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(itemGUID))
		return true
	}
	// Check 2H Weapon equipping in slot 15: unequip offhand if present
	if destSlot == equipSlotMainhand && invType == 17 {
		var offhandItem int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
		if offhandItem != 0 {
			freeSlot, ok := s.findFreeBackpackSlot(ctx)
			if !ok {
				s.sendEquipError(equipErrInvFull, uint64(itemGUID))
				return true
			}
			_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
		}
	}
	var existingItemGUID int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, destSlot).Scan(&existingItemGUID)
	s.swapInventoryCoordinates(ctx, itemGUID, srcBagKey, int64(srcSlot), existingItemGUID, 0, int64(destSlot))
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	// Reference Player::_ApplyItemMods equip criteria (Player.cpp:12419).
	s.updateAchievementCriteria(criteriaTypeEquipItem, uint32(itemEntry), 1)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		if rows, qErr := s.server.WorldStore.DB.QueryContext(ctx, "SELECT Quality FROM item_template WHERE entry = ? LIMIT 1", itemEntry); qErr == nil {
			if rows.Next() {
				var quality int64
				if rows.Scan(&quality) == nil && quality >= 5 { // epic or better
					s.updateAchievementCriteria(criteriaTypeEquipEpicItem, uint32(destSlot), 1)
				}
			}
			rows.Close()
		}
	}
	s.debug("item auto-equipped", "account", s.accountName, "guid", s.playerGUID, "entry", itemEntry, "slot", destSlot)
	return true
}

func (s *session) handleAutoEquipItemSlot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 {
		return true
	}
	reader := protocol.NewReader(payload)
	rawItemGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	dstSlot, err := reader.ReadU8()
	if err != nil {
		return false
	}
	if dstSlot >= equipSlotEnd {
		return true
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}
	var srcBag, srcSlot, itemEntry int64
	err = db.QueryRowContext(ctx, `SELECT ci.bag, ci.slot, ii.itemEntry FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&srcBag, &srcSlot, &itemEntry)
	if err != nil || itemGUID == 0 {
		s.sendEquipError(equipErrItemNotFound, rawItemGUID)
		return true
	}
	var invType int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&invType)
	}
	if !s.isSlotValidForItem(uint8(invType), dstSlot) {
		s.sendEquipError(equipErrItemDoesntGoToSlot, rawItemGUID)
		return true
	}
	// Check 2H Weapon equipping in slot 15: unequip offhand if present
	if dstSlot == equipSlotMainhand && invType == 17 {
		var offhandItem int64
		_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
		if offhandItem != 0 {
			freeSlot, ok := s.findFreeBackpackSlot(ctx)
			if !ok {
				s.sendEquipError(equipErrInvFull, rawItemGUID)
				return true
			}
			_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
		}
	}
	var dstItemGUID int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, dstSlot).Scan(&dstItemGUID)
	s.swapInventoryCoordinates(ctx, itemGUID, srcBag, srcSlot, dstItemGUID, 0, int64(dstSlot))
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item slot equipped", "account", s.accountName, "guid", s.playerGUID, "item", itemGUID, "slot", dstSlot)
	return true
}

func (s *session) handleSwapInvItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	dstSlot := payload[0]
	srcSlot := payload[1]
	if srcSlot == dstSlot {
		return true
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	var srcItemGUID, dstItemGUID int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, srcSlot).Scan(&srcItemGUID)
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, dstSlot).Scan(&dstItemGUID)
	if srcItemGUID == 0 && dstItemGUID == 0 {
		return true
	}

	// If moving to an empty slot, but caller passed swapped slots (src empty, dst occupied)
	if srcItemGUID == 0 && dstItemGUID != 0 {
		srcSlot, dstSlot = dstSlot, srcSlot
		srcItemGUID, dstItemGUID = dstItemGUID, srcItemGUID
	}

	if srcItemGUID != 0 && dstItemGUID != 0 {
		if merged, _ := s.tryMergeStacks(ctx, srcItemGUID, 0, srcSlot, dstItemGUID, 0, dstSlot); merged {
			s.debug("inventory items merged", "account", s.accountName, "src", srcSlot, "dst", dstSlot)
			return true
		}
	}
	// Check bag moves: un-equipping or moving an equipped bag (19..22) or bank bag (67..73) requires it to be empty
	isBagSlot := func(sl uint8) bool {
		return (sl >= invSlotBagStart && sl < invSlotBagEnd) || (sl >= 67 && sl <= 73)
	}

	if isBagSlot(srcSlot) && srcItemGUID != 0 {
		if !s.isBagEmpty(ctx, srcItemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(srcItemGUID))
			return true
		}
	}
	if isBagSlot(dstSlot) && dstItemGUID != 0 {
		if !s.isBagEmpty(ctx, dstItemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(dstItemGUID))
			return true
		}
	}
	// If putting item into a bag slot (19..22 or 67..73), ensure it is a bag container and bank slot is purchased
	if isBagSlot(dstSlot) && srcItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", srcItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if invType != 18 {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
		if dstSlot >= 67 && dstSlot <= 73 && int(dstSlot-67) >= int(s.player.BankBagSlots) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
	}
	if isBagSlot(srcSlot) && dstItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", dstItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if invType != 18 {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
		if srcSlot >= 67 && srcSlot <= 73 && int(srcSlot-67) >= int(s.player.BankBagSlots) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
	}

	// Validate equipping: if dstSlot is equipment slot (< 19), validate srcItemGUID can go there
	if dstSlot < equipSlotEnd && srcItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", srcItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if !s.isSlotValidForItem(uint8(invType), dstSlot) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
		if dstSlot == equipSlotMainhand && invType == 17 {
			var offhandItem int64
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
			if offhandItem != 0 && offhandItem != srcItemGUID {
				freeSlot, ok := s.findFreeBackpackSlot(ctx)
				if !ok {
					s.sendEquipError(equipErrInvFull, uint64(srcItemGUID))
					return true
				}
				_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
			}
		}
	}
	// Validate equipping: if srcSlot is equipment slot (< 19) and dstItemGUID != 0, validate dstItemGUID can go there
	if srcSlot < equipSlotEnd && dstItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", dstItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if !s.isSlotValidForItem(uint8(invType), srcSlot) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
		if srcSlot == equipSlotMainhand && invType == 17 {
			var offhandItem int64
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
			if offhandItem != 0 && offhandItem != dstItemGUID {
				freeSlot, ok := s.findFreeBackpackSlot(ctx)
				if !ok {
					s.sendEquipError(equipErrInvFull, uint64(dstItemGUID))
					return true
				}
				_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
			}
		}
	}

	s.swapInventoryCoordinates(ctx, srcItemGUID, 0, int64(srcSlot), dstItemGUID, 0, int64(dstSlot))
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("inventory items swapped", "account", s.accountName, "src", srcSlot, "dst", dstSlot)
	return true
}

func (s *session) handleSwapItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	slotBagA := payload[0]
	slotSlotA := payload[1]
	slotBagB := payload[2]
	slotSlotB := payload[3]

	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}

	bagKeyA, ok1 := s.inventoryBagKey(ctx, slotBagA)
	bagKeyB, ok2 := s.inventoryBagKey(ctx, slotBagB)
	if !ok1 || !ok2 {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}

	if bagKeyA == bagKeyB && slotSlotA == slotSlotB {
		return true
	}

	var itemGUIDA, itemGUIDB int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ? LIMIT 1", s.playerGUID, bagKeyA, slotSlotA).Scan(&itemGUIDA)
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ? LIMIT 1", s.playerGUID, bagKeyB, slotSlotB).Scan(&itemGUIDB)
	if itemGUIDA == 0 && itemGUIDB == 0 {
		return true
	}

	dstBag, dstSlot, dstBagKey, dstItemGUID := slotBagA, slotSlotA, bagKeyA, itemGUIDA
	srcBag, srcSlot, srcBagKey, srcItemGUID := slotBagB, slotSlotB, bagKeyB, itemGUIDB
	// If src was empty but dst has an item, normalize so src is the slot with the item
	if srcItemGUID == 0 && dstItemGUID != 0 {
		srcBag, srcSlot, srcBagKey, srcItemGUID, dstBag, dstSlot, dstBagKey, dstItemGUID = dstBag, dstSlot, dstBagKey, dstItemGUID, srcBag, srcSlot, srcBagKey, srcItemGUID
	}

	if srcItemGUID != 0 && dstItemGUID != 0 {
		if merged, _ := s.tryMergeStacks(ctx, srcItemGUID, srcBagKey, srcSlot, dstItemGUID, dstBagKey, dstSlot); merged {
			s.debug("items merged across bags", "account", s.accountName, "srcBag", srcBag, "srcSlot", srcSlot, "dstBag", dstBag, "dstSlot", dstSlot)
			return true
		}
	}

	// Bag move checks: moving or un-equipping a bag requires it to be empty
	if srcBagKey == 0 && ((srcSlot >= invSlotBagStart && srcSlot < invSlotBagEnd) || (srcSlot >= 67 && srcSlot <= 73)) && srcItemGUID != 0 {
		if !s.isBagEmpty(ctx, srcItemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(srcItemGUID))
			return true
		}
	}
	if dstBagKey == 0 && ((dstSlot >= invSlotBagStart && dstSlot < invSlotBagEnd) || (dstSlot >= 67 && dstSlot <= 73)) && dstItemGUID != 0 {
		if !s.isBagEmpty(ctx, dstItemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(dstItemGUID))
			return true
		}
	}

	// Equipping a bag into bag slot (19..22 or 67..73)
	if dstBagKey == 0 && ((dstSlot >= invSlotBagStart && dstSlot < invSlotBagEnd) || (dstSlot >= 67 && dstSlot <= 73)) && srcItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", srcItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if invType != 18 {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
		if dstSlot >= 67 && dstSlot <= 73 && int(dstSlot-67) >= int(s.player.BankBagSlots) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
	}
	if srcBagKey == 0 && ((srcSlot >= invSlotBagStart && srcSlot < invSlotBagEnd) || (srcSlot >= 67 && srcSlot <= 73)) && dstItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", dstItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if invType != 18 {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
		if srcSlot >= 67 && srcSlot <= 73 && int(srcSlot-67) >= int(s.player.BankBagSlots) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
	}

	// If dstBagKey == 0 and dstSlot < equipSlotEnd (equipping), validate srcItemGUID
	if dstBagKey == 0 && dstSlot < equipSlotEnd && srcItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", srcItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if !s.isSlotValidForItem(uint8(invType), dstSlot) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(srcItemGUID))
			return true
		}
		if dstSlot == equipSlotMainhand && invType == 17 {
			var offhandItem int64
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
			if offhandItem != 0 && offhandItem != srcItemGUID {
				freeSlot, ok := s.findFreeBackpackSlot(ctx)
				if !ok {
					s.sendEquipError(equipErrInvFull, uint64(srcItemGUID))
					return true
				}
				_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
			}
		}
	}
	// If srcBagKey == 0 and srcSlot < equipSlotEnd (equipping from dst to src), validate dstItemGUID
	if srcBagKey == 0 && srcSlot < equipSlotEnd && dstItemGUID != 0 {
		var itemEntry int64
		_ = db.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ?", dstItemGUID).Scan(&itemEntry)
		var invType int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT InventoryType FROM item_template WHERE entry = ?", itemEntry).Scan(&invType)
		}
		if !s.isSlotValidForItem(uint8(invType), srcSlot) {
			s.sendEquipError(equipErrItemDoesntGoToSlot, uint64(dstItemGUID))
			return true
		}
		if srcSlot == equipSlotMainhand && invType == 17 {
			var offhandItem int64
			_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, equipSlotOffhand).Scan(&offhandItem)
			if offhandItem != 0 && offhandItem != dstItemGUID {
				freeSlot, ok := s.findFreeBackpackSlot(ctx)
				if !ok {
					s.sendEquipError(equipErrInvFull, uint64(dstItemGUID))
					return true
				}
				_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = 0, slot = ? WHERE guid = ? AND item = ?", freeSlot, s.playerGUID, offhandItem)
			}
		}
	}

	s.swapInventoryCoordinates(ctx, srcItemGUID, srcBagKey, int64(srcSlot), dstItemGUID, dstBagKey, int64(dstSlot))
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("items swapped across bags", "account", s.accountName, "srcBag", srcBag, "srcSlot", srcSlot, "dstBag", dstBag, "dstSlot", dstSlot)
	return true
}

func (s *session) handleDestroyItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 3 {
		return true
	}
	bag := payload[0]
	slot := payload[1]
	count := payload[2]
	db := s.server.CharactersStore.DB
	if db == nil {
		return true
	}
	bagKey, ok := s.inventoryBagKey(ctx, bag)
	if !ok {
		return true
	}
	var itemGUID, currentCount int64
	var itemEntry int64
	err := db.QueryRowContext(ctx, `SELECT ci.item, ii.count, ii.itemEntry FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, bagKey, slot).Scan(&itemGUID, &currentCount, &itemEntry)
	if err != nil || itemGUID == 0 {
		return true
	}
	if bagKey == 0 && ((slot >= invSlotBagStart && slot < invSlotBagEnd) || (slot >= 67 && slot <= 73)) {
		if !s.isBagEmpty(ctx, itemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(itemGUID))
			return true
		}
	}
	if currentCount <= int64(count) || count == 0 {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
		_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
		s.despawnItem(uint64(itemGUID))
	} else {
		_, _ = db.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", count, itemGUID)
	}
	s.refreshQuestItemCounts(ctx, uint32(itemEntry), false)
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item destroyed", "account", s.accountName, "item", itemGUID, "bag", bag, "slot", slot, "count", count)
	return true
}

func (s *session) syncEquipmentCache(ctx context.Context) {
	if !s.playerLoaded || s.player == nil || s.server.CharactersStore.DB == nil {
		return
	}
	db := s.server.CharactersStore.DB
	slots := make([]uint32, equipSlotEnd)
	enchants := make([]uint32, equipSlotEnd)
	rows, err := db.QueryContext(ctx, `SELECT ci.slot, ii.itemEntry, COALESCE(ii.enchantments, '') FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < ?`, s.playerGUID, equipSlotEnd)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var slot, entry int64
			var encStr string
			if err := rows.Scan(&slot, &entry, &encStr); err == nil && slot < int64(len(slots)) {
				slots[slot] = uint32(entry)
				if encStr != "" {
					fields := strings.Fields(encStr)
					if len(fields) > 0 {
						if encID, pErr := strconv.ParseUint(fields[0], 10, 32); pErr == nil {
							enchants[slot] = uint32(encID)
						}
					}
				}
			}
		}
	}
	parts := make([]string, equipSlotEnd*2)
	for i := 0; i < int(equipSlotEnd); i++ {
		parts[i*2] = strconv.FormatUint(uint64(slots[i]), 10)
		parts[i*2+1] = strconv.FormatUint(uint64(enchants[i]), 10)
	}
	cacheStr := strings.Join(parts, " ")
	s.player.Equipment = cacheStr
	_, _ = db.ExecContext(ctx, "UPDATE characters SET equipmentCache = ? WHERE guid = ?", cacheStr, s.playerGUID)
	_ = s.calculatePlayerStats(ctx, s.player)
}

func inventoryTypeToSlot(invType uint8) uint8 {
	switch invType {
	case 1: // Head
		return equipSlotHead
	case 2: // Neck
		return equipSlotNeck
	case 3: // Shoulders
		return equipSlotShoulder
	case 4: // Body/Shirt
		return equipSlotBody
	case 5, 20: // Chest / Robe
		return equipSlotChest
	case 6: // Waist
		return equipSlotWaist
	case 7: // Legs
		return equipSlotLegs
	case 8: // Feet
		return equipSlotFeet
	case 9: // Wrists
		return equipSlotWrists
	case 10: // Hands
		return equipSlotHands
	case 11: // Finger
		return equipSlotFinger1
	case 12: // Trinket
		return equipSlotTrinket1
	case 13, 17, 21: // Weapon, 2HWeapon, Mainhand
		return equipSlotMainhand
	case 14, 22, 23: // Shield, Offhand, Holdable
		return equipSlotOffhand
	case 15, 25, 26: // Ranged, Thrown, RangedRight
		return equipSlotRanged
	case 16: // Back/Cloak
		return equipSlotBack
	case 18: // Bag
		return invSlotBagStart
	case 19: // Tabard
		return equipSlotTabard
	default:
		return 255
	}
}

// handleItemNameQuery processes CMSG_ITEM_NAME_QUERY (0x2C4).
// Reference: WorldSession::HandleItemNameQueryOpcode (ItemHandler.cpp:812).
func (s *session) handleItemNameQuery(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	itemID, err := r.ReadU32()
	if err != nil {
		return false
	}
	_, _ = r.ReadU64() // skip guid

	var name string
	var invType uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name, InventoryType FROM item_template WHERE entry = ? LIMIT 1", itemID).Scan(&name, &invType)
	}
	if name == "" {
		return true
	}

	buf := protocol.NewBuffer(len(name) + 16)
	buf.WriteU32(itemID)
	buf.WriteCString(name)
	buf.WriteU32(invType)
	return s.write(uint16(protocol.OpcodeSMSG_ITEM_NAME_QUERY_RESPONSE), buf.Bytes(), true) == nil
}

// handleItemTextQuery processes CMSG_ITEM_TEXT_QUERY (0x243).
// Reference: WorldSession::HandleItemTextQuery (ItemHandler.cpp:1211).
func (s *session) handleItemTextQuery(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	rawItemGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}

	var text string
	var found bool
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT text FROM item_text WHERE id = (SELECT itemTextId FROM item_instance WHERE guid = ?) LIMIT 1", itemGUID).Scan(&text)
		if err == nil {
			found = true
		} else {
			var count int
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM item_instance WHERE guid = ?", itemGUID).Scan(&count)
			if count > 0 {
				found = true
				text = ""
			}
		}
	}

	buf := protocol.NewBuffer(len(text) + 16)
	if found {
		buf.WriteU8(0) // has text
		buf.WriteU64(rawItemGUID)
		buf.WriteCString(text)
	} else {
		buf.WriteU8(1) // no text
	}
	return s.write(uint16(protocol.OpcodeSMSG_ITEM_TEXT_QUERY_RESPONSE), buf.Bytes(), true) == nil
}

// handleItemRefundInfo processes CMSG_ITEM_REFUND_INFO (0x4B3).
// Reference: WorldSession::HandleItemRefundInfoRequest (ItemHandler.cpp:1169) and Player::SendRefundInfo (Player.cpp:26503).
func (s *session) handleItemRefundInfo(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	rawItemGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}

	var itemEntry, paidMoney, paidExtendedCost int64
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, iri.paidMoney, iri.paidExtendedCost
		FROM item_instance AS ii JOIN item_refund_instance AS iri ON iri.item_guid = ii.guid AND iri.player_guid = ?
		WHERE ii.guid = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&itemEntry, &paidMoney, &paidExtendedCost)
	if err != nil || itemEntry == 0 {
		return true
	}
	extendedCost := wotlk.ItemExtendedCostEntry{}
	if paidExtendedCost != 0 {
		if s.server.Data == nil {
			return true
		}
		var found bool
		extendedCost, found, err = s.server.Data.ItemExtendedCost(uint32(paidExtendedCost))
		if err != nil || !found {
			return true
		}
	}

	buf := protocol.NewBuffer(64)
	buf.WriteU64(rawItemGUID)
	buf.WriteU32(uint32(paidMoney))
	buf.WriteU32(extendedCost.HonorPoints)
	buf.WriteU32(extendedCost.ArenaPoints)
	for i := 0; i < len(extendedCost.ItemIDs); i++ {
		buf.WriteU32(extendedCost.ItemIDs[i])
		buf.WriteU32(extendedCost.ItemCounts[i])
	}
	buf.WriteU32(0)
	buf.WriteU32(7200)
	return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_INFO_RESPONSE), buf.Bytes(), true) == nil
}

// handleItemRefund processes CMSG_ITEM_REFUND (0x4B4).
// Reference: WorldSession::HandleItemRefund (ItemHandler.cpp:1186) and Player::RefundItem (Player.cpp:26574).
func (s *session) handleItemRefund(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	rawItemGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	itemGUID := int64(rawItemGUID & 0xFFFFFFFF)
	if itemGUID == 0 {
		itemGUID = int64(rawItemGUID)
	}
	if !s.playerLoaded || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}

	var itemEntry, paidMoney, paidExtendedCost int64
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, iri.paidMoney, iri.paidExtendedCost FROM item_instance AS ii
		JOIN character_inventory AS ci ON ci.item = ii.guid
		JOIN item_refund_instance AS iri ON iri.item_guid = ii.guid AND iri.player_guid = ci.guid
		WHERE ii.guid = ? AND ci.guid = ? LIMIT 1`, itemGUID, s.playerGUID).Scan(&itemEntry, &paidMoney, &paidExtendedCost)

	buf := protocol.NewBuffer(64)
	buf.WriteU64(rawItemGUID)
	if err != nil || itemEntry == 0 {
		buf.WriteU32(10) // error (expired or not refundable)
		return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
	}
	extendedCost := wotlk.ItemExtendedCostEntry{}
	if paidExtendedCost != 0 {
		if s.server.Data == nil {
			buf.WriteU32(10)
			return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
		}
		var found bool
		extendedCost, found, err = s.server.Data.ItemExtendedCost(uint32(paidExtendedCost))
		if err != nil || !found {
			buf.WriteU32(10)
			return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
		}
	}
	for i := 0; i < len(extendedCost.ItemIDs); i++ {
		if extendedCost.ItemIDs[i] == 0 || extendedCost.ItemCounts[i] == 0 {
			continue
		}
		if _, err := s.storeOrStackItem(ctx, s.playerGUID, extendedCost.ItemIDs[i], extendedCost.ItemCounts[i]); err != nil {
			buf.WriteU32(10)
			return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
		}
	}

	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ? AND guid = ?", itemGUID, s.playerGUID)
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_refund_instance WHERE item_guid = ? AND player_guid = ?", itemGUID, s.playerGUID)
	s.refreshQuestItemCounts(ctx, uint32(itemEntry), false)

	s.player.Money += uint32(paidMoney)
	s.player.TotalHonorPoints += extendedCost.HonorPoints
	s.player.ArenaPoints += extendedCost.ArenaPoints
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ?, arenaPoints = ?, totalHonorPoints = ? WHERE guid = ?", s.player.Money, s.player.ArenaPoints, s.player.TotalHonorPoints, s.playerGUID)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	buf.WriteU32(0)
	buf.WriteU32(uint32(paidMoney))
	buf.WriteU32(extendedCost.HonorPoints)
	buf.WriteU32(extendedCost.ArenaPoints)
	for i := 0; i < len(extendedCost.ItemIDs); i++ {
		buf.WriteU32(extendedCost.ItemIDs[i])
		buf.WriteU32(extendedCost.ItemCounts[i])
	}
	return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
}

const (
	equipErrOk                      = 0
	equipErrCantEquipLevelI         = 1
	equipErrItemDoesntGoToSlot      = 2
	equipErrBagFull                 = 4
	equipErrNonemptyBagOverOtherBag = 5
	equipErrCantEquipWithTwohanded  = 13
	equipErrCantDualWield           = 14
	equipErrItemDoesntGoIntoBag     = 15
	equipErrItemCantBeEquipped      = 20
	equipErrItemsCantBeSwapped      = 21
	equipErrSlotIsEmpty             = 22
	equipErrItemNotFound            = 23
	equipErrCanOnlyDoWithEmptyBags  = 31
	equipErrYouAreDead              = 38
	equipErrCantDoRightNow          = 39
	equipErrStackableCantBeWrapped  = 43
	equipErrEquippedCantBeWrapped   = 44
	equipErrWrappedCantBeWrapped    = 45
	equipErrBoundCantBeWrapped      = 46
	equipErrUniqueCantBeWrapped     = 47
	equipErrBagsCantBeWrapped       = 48
	equipErrInvFull                 = 50
	equipErrCantEquipRank           = 63
	equipErrVendorMissingTurnins    = 68
	equipErrNotEnoughHonorPoints    = 69
	equipErrNotEnoughArenaPoints    = 70
)

func (s *session) sendEquipError(errCode uint8, itemGUID uint64) {
	buf := protocol.NewBuffer(18)
	buf.WriteU8(errCode)
	if errCode != equipErrOk {
		buf.WriteU64(itemGUID)
		buf.WriteU64(0)
		buf.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_INVENTORY_CHANGE_FAILURE), buf.Bytes(), true)
}

// handleUseItem processes CMSG_USE_ITEM (0x0AB).
// Reference: WorldSession::HandleUseItemOpcode (SpellHandler.cpp:73).
func (s *session) handleUseItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	// Player cannot use items while dead (SpellHandler.cpp:194)
	if s.player.Health == 0 {
		s.sendEquipError(equipErrYouAreDead, 0)
		return true
	}
	r := protocol.NewReader(payload)
	bagIndex, err := r.ReadU8()
	if err != nil {
		return false
	}
	slot, err := r.ReadU8()
	if err != nil {
		return false
	}
	castCount, err := r.ReadU8()
	if err != nil {
		return false
	}
	spellID, err := r.ReadU32()
	if err != nil {
		return false
	}
	itemGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	glyphIndex, err := r.ReadU32()
	if err != nil {
		return false
	}
	if glyphIndex < 6 {
		s.targetGlyphSlot = uint8(glyphIndex)
	}
	_, err = r.ReadU8() // castFlags
	if err != nil {
		return false
	}

	target, _ := protocol.ReadSpellTargetData(r)

	// Validate item existence in specified bag and slot
	bagKey, ok := s.inventoryBagKey(ctx, bagIndex)
	if !ok {
		s.sendEquipError(equipErrItemNotFound, itemGUID)
		return true
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	var dbItemGUID int64
	var itemEntry int64
	var count int64
	err = cdb.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, bagKey, slot).Scan(&dbItemGUID, &itemEntry, &count)
	if err != nil || count <= 0 {
		s.sendEquipError(equipErrItemNotFound, itemGUID)
		return true
	}
	rawItemGUID := uint64(dbItemGUID)
	fullItemGUID := rawItemGUID | (uint64(0x4000) << 48)
	if itemGUID != 0 && itemGUID != rawItemGUID && itemGUID != fullItemGUID {
		s.sendEquipError(equipErrItemNotFound, itemGUID)
		return true
	}

	// Check player spell cooldown
	nowUnix := time.Now().Unix()
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID && cd.End > nowUnix {
			return true
		}
	}

	// Cast spell
	if spellID != 0 && s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			castTime := uint32(0)
			if value, ok, castErr := s.server.Data.SpellCastTime(spell.CastingTimeIndex); castErr == nil && ok && value > 0 {
				castTime = uint32(value)
			}
			if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_START), protocol.BuildSpellStart(s.playerGUID, s.playerGUID, castCount, spellID, spellCastFlagStart, castTime, target), true); err != nil {
				return false
			}
			if castTime > 0 {
				time.AfterFunc(time.Duration(castTime)*time.Millisecond, func() {
					s.updateAchievementCriteria(criteriaTypeUseItem, uint32(itemEntry), 1)
					s.startTimedAchievement(timedTypeItem, uint32(itemEntry))
					s.finishSpellCast(context.Background(), castCount, spellID, spell, target)
				})
			} else {
				s.finishSpellCast(ctx, castCount, spellID, spell, target)
			}
		}
	}

	// If consumable item (class == 0), decrement count or remove from inventory
	var class int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT class FROM item_template WHERE entry = ?", itemEntry).Scan(&class)
		if class == 0 { // ITEM_CLASS_CONSUMABLE
			if count > 1 {
				_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", dbItemGUID)
			} else {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, bagKey, slot)
				_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", dbItemGUID)
				s.despawnItem(uint64(dbItemGUID))
			}
			s.refreshQuestItemCounts(ctx, uint32(itemEntry), false)
			_ = s.sendInventoryItems(ctx)
			s.sendPlayerUpdate()
		}
	}

	return true
}

func (s *session) inventoryBagKey(ctx context.Context, bag uint8) (int64, bool) {
	if bag == 0 || bag == invSlotBag0 {
		return 0, true
	}
	actualSlot := bag
	if bag >= 1 && bag <= 4 {
		actualSlot = 18 + bag // 19..22
	} else if bag >= 5 && bag <= 11 {
		actualSlot = 62 + bag // 67..73
	} else if (bag < invSlotBagStart || bag >= invSlotBagEnd) && (bag < 67 || bag > 73) {
		return 0, false
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, false
	}
	var itemGUID int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, actualSlot).Scan(&itemGUID)
	return itemGUID, err == nil && itemGUID != 0
}

func (s *session) inventoryItemAt(ctx context.Context, bag, slot uint8) (int64, int64, int64, error) {
	bagKey, ok := s.inventoryBagKey(ctx, bag)
	if !ok {
		return 0, 0, 0, sql.ErrNoRows
	}
	var itemGUID, itemEntry, count int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, bagKey, slot).Scan(&itemGUID, &itemEntry, &count)
	return itemGUID, itemEntry, count, err
}

func (s *session) handleSplitItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 9 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	srcBag, err := reader.ReadU8()
	if err != nil {
		return false
	}
	srcSlot, err := reader.ReadU8()
	if err != nil {
		return false
	}
	dstBag, err := reader.ReadU8()
	if err != nil {
		return false
	}
	dstSlot, err := reader.ReadU8()
	if err != nil {
		return false
	}
	count, err := reader.ReadU32()
	if err != nil || count == 0 || (srcBag == dstBag && srcSlot == dstSlot) {
		return true
	}
	srcGUID, srcEntry, srcCount, err := s.inventoryItemAt(ctx, srcBag, srcSlot)
	if err != nil || srcGUID == 0 || count >= uint32(srcCount) {
		return true
	}
	dstKey, ok := s.inventoryBagKey(ctx, dstBag)
	if !ok {
		return true
	}
	db := s.server.CharactersStore.DB
	var dstGUID, dstEntry, dstCount int64
	if err := db.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, dstKey, dstSlot).Scan(&dstGUID, &dstEntry, &dstCount); err != nil && err != sql.ErrNoRows {
		return true
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return true
	}
	if dstGUID != 0 {
		if dstEntry != srcEntry {
			_ = tx.Rollback()
			return true
		}
		if _, err = tx.ExecContext(ctx, "UPDATE item_instance SET count = count + ? WHERE guid = ?", count, dstGUID); err != nil {
			_ = tx.Rollback()
			return true
		}
	} else {
		var newGUID int64
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&newGUID); err != nil || newGUID <= 0 {
			_ = tx.Rollback()
			return true
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, count) VALUES (?, ?, ?, ?)", newGUID, srcEntry, s.playerGUID, count); err != nil {
			_ = tx.Rollback()
			return true
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, dstKey, dstSlot, newGUID); err != nil {
			_ = tx.Rollback()
			return true
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", count, srcGUID); err != nil {
		_ = tx.Rollback()
		return true
	}
	if err = tx.Commit(); err != nil {
		return true
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item stack split", "account", s.accountName, "source_bag", srcBag, "source_slot", srcSlot, "destination_bag", dstBag, "destination_slot", dstSlot, "count", count)
	return true
}

func (s *session) handleAutoStoreBagItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 3 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	srcBag, srcSlot, dstBag := payload[0], payload[1], payload[2]
	itemGUID, _, _, err := s.inventoryItemAt(ctx, srcBag, srcSlot)
	if err != nil || itemGUID == 0 {
		return true
	}
	srcBagKey, _ := s.inventoryBagKey(ctx, srcBag)
	if srcBagKey == 0 && srcSlot >= invSlotBagStart && srcSlot < invSlotBagEnd {
		if !s.isBagEmpty(ctx, itemGUID) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(itemGUID))
			return true
		}
	}
	dstKey, ok := s.inventoryBagKey(ctx, dstBag)
	if !ok {
		return true
	}
	slot, ok := s.freeInventorySlot(ctx, dstKey)
	if !ok {
		s.sendEquipError(equipErrInvFull, uint64(itemGUID))
		return true
	}
	if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", dstKey, slot, s.playerGUID, itemGUID); err != nil {
		return true
	}
	if srcBagKey == 0 && srcSlot < equipSlotEnd {
		s.syncEquipmentCache(ctx)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item moved into bag", "account", s.accountName, "item", itemGUID, "bag", dstBag, "slot", slot)
	return true
}

// inventoryStoreResult contains the outcome of an item storage or stack operation.
type inventoryStoreResult struct {
	BagKey         int64
	ClientBag      uint8
	Slot           uint8
	ItemGUID       uint64
	IsStack        bool
	NewCount       uint32
	InventoryCount uint32
}

var errInventoryFull = errors.New("inventory is full")

type equippedBagInfo struct {
	slot  uint8
	guid  int64
	slots int64
}

func (s *session) getEquippedBags(ctx context.Context, playerGUID uint64) []equippedBagInfo {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	rows, err := cdb.QueryContext(ctx, "SELECT slot, item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 19 AND slot <= 22 AND item != 0 ORDER BY slot", playerGUID)
	if err != nil {
		return nil
	}
	type rawBag struct {
		slot uint8
		guid int64
	}
	var rawBags []rawBag
	for rows.Next() {
		var sl uint8
		var itemGUID int64
		if err := rows.Scan(&sl, &itemGUID); err == nil && itemGUID > 0 {
			rawBags = append(rawBags, rawBag{slot: sl, guid: itemGUID})
		}
	}
	rows.Close()

	var bags []equippedBagInfo
	for _, rb := range rawBags {
		var slots int64
		if wdb != nil {
			err := wdb.QueryRowContext(ctx, `SELECT COALESCE(ContainerSlots, 0) FROM item_template WHERE entry = (SELECT itemEntry FROM item_instance WHERE guid = ?)`, rb.guid).Scan(&slots)
			if err != nil && (isMissingColumn(err) || missingTable(err)) {
				slots = 0
			}
		}
		if slots > 0 {
			bags = append(bags, equippedBagInfo{slot: rb.slot, guid: rb.guid, slots: slots})
		}
	}
	return bags
}

func (s *session) freeInventorySlotForPlayer(ctx context.Context, playerGUID uint64, bagKey int64) (uint8, bool) {
	first, last := int64(invSlotItemStart), int64(invSlotItemEnd-1)
	if bagKey != 0 {
		first, last = 0, 35
		if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return 0, false
		}
		var slots int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(ContainerSlots, 0) FROM item_template WHERE entry = (SELECT itemEntry FROM item_instance WHERE guid = ?)`, bagKey).Scan(&slots); err != nil || slots <= 0 {
			return 0, false
		}
		if slots-1 < last {
			last = slots - 1
		}
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, false
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = ?", playerGUID, bagKey)
	if err != nil {
		return 0, false
	}
	defer rows.Close()
	used := make(map[int64]struct{})
	for rows.Next() {
		var slot int64
		if rows.Scan(&slot) == nil {
			used[slot] = struct{}{}
		}
	}
	for slot := first; slot <= last; slot++ {
		if _, exists := used[slot]; !exists {
			return uint8(slot), true
		}
	}
	return 0, false
}

func (s *session) freeInventorySlot(ctx context.Context, bagKey int64) (uint8, bool) {
	return s.freeInventorySlotForPlayer(ctx, s.playerGUID, bagKey)
}

func (s *session) findFreeInventorySlot(ctx context.Context, playerGUID uint64) (bagKey int64, clientBag uint8, slot uint8, ok bool) {
	// 1. Check backpack (bagKey = 0, clientBag = 255, slots 23..38)
	if slot, ok := s.freeInventorySlotForPlayer(ctx, playerGUID, 0); ok {
		return 0, 255, slot, true
	}
	// 2. Check equipped bags (slots 19..22)
	bags := s.getEquippedBags(ctx, playerGUID)
	for _, b := range bags {
		if freeSlot, ok := s.freeInventorySlotForPlayer(ctx, playerGUID, b.guid); ok {
			return b.guid, b.slot, freeSlot, true
		}
	}
	return 0, 0, 0, false
}

func (s *session) findStackableInventorySlot(ctx context.Context, playerGUID uint64, itemEntry uint32) (bagKey int64, clientBag uint8, slot uint8, itemGUID uint64, curCount uint32, maxStack uint32, ok bool) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	var stackable int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(stackable, 1) FROM item_template WHERE entry = ?", itemEntry).Scan(&stackable)
	if err != nil || stackable <= 1 {
		return 0, 0, 0, 0, 0, 0, false
	}
	cdb := s.server.CharactersStore.DB
	// 1. Check backpack (bag = 0, slots 23..38)
	var bpGUID, bpCount int64
	var bpSlot uint8
	err = cdb.QueryRowContext(ctx, `SELECT ci.slot, ci.item, ii.count
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot >= 23 AND ci.slot <= 38 AND ii.itemEntry = ? AND ii.count < ?
		ORDER BY ci.slot ASC LIMIT 1`, playerGUID, itemEntry, stackable).Scan(&bpSlot, &bpGUID, &bpCount)
	if err == nil && bpGUID > 0 {
		return 0, 255, bpSlot, uint64(bpGUID), uint32(bpCount), uint32(stackable), true
	}
	// 2. Check equipped bags (slots 19..22)
	bags := s.getEquippedBags(ctx, playerGUID)
	for _, b := range bags {
		var bItemGUID, bCount int64
		var bSlot uint8
		bErr := cdb.QueryRowContext(ctx, `SELECT ci.slot, ci.item, ii.count
			FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ci.bag = ? AND ii.itemEntry = ? AND ii.count < ?
			ORDER BY ci.slot ASC LIMIT 1`, playerGUID, b.guid, itemEntry, stackable).Scan(&bSlot, &bItemGUID, &bCount)
		if bErr == nil && bItemGUID > 0 {
			return b.guid, b.slot, bSlot, uint64(bItemGUID), uint32(bCount), uint32(stackable), true
		}
	}
	return 0, 0, 0, 0, 0, 0, false
}

func (s *session) storeOrStackItem(ctx context.Context, playerGUID uint64, itemEntry, count uint32) (*inventoryStoreResult, error) {
	result, err := s.storeOrStackItemCore(ctx, playerGUID, itemEntry, count)
	if err == nil && result != nil && s.player != nil && playerGUID == s.playerGUID {
		s.refreshQuestItemCounts(ctx, itemEntry, true)
	}
	return result, err
}

func (s *session) storeOrStackItemCore(ctx context.Context, playerGUID uint64, itemEntry, count uint32) (*inventoryStoreResult, error) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil, errors.New("characters database not available")
	}
	cdb := s.server.CharactersStore.DB
	if count == 0 {
		count = 1
	}

	// 1. Check if stackable and can stack onto an existing stack
	bagKey, clientBag, slot, itemGUID, curCount, maxStack, canStack := s.findStackableInventorySlot(ctx, playerGUID, itemEntry)
	if canStack && curCount < maxStack {
		space := maxStack - curCount
		if count <= space {
			newCount := curCount + count
			if _, err := cdb.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", newCount, itemGUID); err != nil {
				return nil, err
			}
			var totalCount int64
			_ = cdb.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
				JOIN item_instance AS ii ON ii.guid = ci.item
				WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, itemEntry).Scan(&totalCount)

			return &inventoryStoreResult{
				BagKey:         bagKey,
				ClientBag:      clientBag,
				Slot:           slot,
				ItemGUID:       itemGUID,
				IsStack:        true,
				NewCount:       newCount,
				InventoryCount: uint32(totalCount),
			}, nil
		} else {
			// Find free slot for remainder first to ensure everything fits before modifying
			freeBagKey, freeClientBag, freeSlot, ok := s.findFreeInventorySlot(ctx, playerGUID)
			if !ok {
				return nil, errInventoryFull
			}
			if _, err := cdb.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", maxStack, itemGUID); err != nil {
				return nil, err
			}
			remainder := count - space
			var nextGUID int64
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextGUID)
			if nextGUID <= 0 {
				nextGUID = 1
			}
			tx, err := cdb.BeginTx(ctx, nil)
			if err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO item_instance
				(guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
				VALUES (?, ?, ?, 0, ?, 0, '', 0, '', 0, 100, 0, '')`,
				nextGUID, itemEntry, playerGUID, remainder); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO character_inventory
				(guid, bag, slot, item) VALUES (?, ?, ?, ?)`,
				playerGUID, freeBagKey, freeSlot, nextGUID); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			var totalCount int64
			_ = cdb.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
				JOIN item_instance AS ii ON ii.guid = ci.item
				WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, itemEntry).Scan(&totalCount)

			return &inventoryStoreResult{
				BagKey:         freeBagKey,
				ClientBag:      freeClientBag,
				Slot:           freeSlot,
				ItemGUID:       uint64(nextGUID),
				IsStack:        false,
				NewCount:       remainder,
				InventoryCount: uint32(totalCount),
			}, nil
		}
	}

	// 2. Find a free slot in backpack or equipped bags
	freeBagKey, freeClientBag, freeSlot, ok := s.findFreeInventorySlot(ctx, playerGUID)
	if !ok {
		return nil, errInventoryFull
	}

	var nextGUID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextGUID)
	if nextGUID <= 0 {
		nextGUID = 1
	}

	var maxDurability int64 = 100
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var md int64
		err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MaxDurability, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&md)
		if err == nil && md > 0 {
			maxDurability = md
		}
	}

	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO item_instance
		(guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
		VALUES (?, ?, ?, 0, ?, 0, '', 0, '', 0, ?, 0, '')`,
		nextGUID, itemEntry, playerGUID, count, maxDurability); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO character_inventory
		(guid, bag, slot, item) VALUES (?, ?, ?, ?)`,
		playerGUID, freeBagKey, freeSlot, nextGUID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}

	var totalCount int64
	_ = cdb.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, itemEntry).Scan(&totalCount)

	return &inventoryStoreResult{
		BagKey:         freeBagKey,
		ClientBag:      freeClientBag,
		Slot:           freeSlot,
		ItemGUID:       uint64(nextGUID),
		IsStack:        false,
		NewCount:       count,
		InventoryCount: uint32(totalCount),
	}, nil
}

// handleOpenItem processes CMSG_OPEN_ITEM (0x0AC).
// Reference: WorldSession::HandleOpenItemOpcode (SpellHandler.cpp:183).
func (s *session) handleOpenItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.player.Health == 0 {
		s.sendEquipError(equipErrYouAreDead, 0)
		return true
	}
	if len(payload) < 2 {
		return true
	}
	bagIndex := payload[0]
	slot := payload[1]

	itemGUID, itemEntry, _, err := s.inventoryItemAt(ctx, bagIndex, slot)
	if err != nil || itemGUID == 0 {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var giftEntry, giftFlags uint32
		err := cdb.QueryRowContext(ctx, "SELECT entry, flags FROM character_gifts WHERE item_guid = ?", itemGUID).Scan(&giftEntry, &giftFlags)
		if err == nil && giftEntry > 0 {
			// Unwrapping: restore original entry, flags, and delete gift record
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET itemEntry = ?, flags = ? WHERE guid = ?", giftEntry, giftFlags, itemGUID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_gifts WHERE item_guid = ?", itemGUID)
			_ = s.sendInventoryItems(ctx)
			return true
		}

		// Container opening: check item_loot_template
		var lootSource *sql.DB
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			lootSource = s.server.WorldStore.DB
		} else {
			lootSource = cdb
		}

		if lootSource != nil {
			rows, qErr := lootSource.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0)
				FROM item_loot_template AS l
				LEFT JOIN item_template AS t ON t.entry = l.Item
				WHERE l.Entry = ? ORDER BY l.Item LIMIT 16`, itemEntry)
			if qErr == nil {
				loot := &activeLootState{
					TargetGUID: uint64(itemGUID),
					LootType:   1,
					Items:      make(map[uint8]lootItem),
				}
				var lSlot uint8 = 0
				for rows.Next() {
					var itemID int64
					var chance float64
					var minCount, maxCount, displayID int64
					if err := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID); err == nil {
						roll := rand.Float64() * 100.0
						if chance <= 0 || roll <= chance {
							count := uint32(minCount)
							if maxCount > minCount {
								count += uint32(rand.Intn(int(maxCount - minCount + 1)))
							}
							if count == 0 {
								count = 1
							}
							loot.Items[lSlot] = lootItem{
								Slot:          lSlot,
								ItemEntry:     uint32(itemID),
								Count:         count,
								DisplayInfoID: uint32(displayID),
							}
							lSlot++
							if lSlot >= 16 {
								break
							}
						}
					}
				}
				rows.Close()

				if len(loot.Items) > 0 {
					s.server.lootMu.Lock()
					if s.server.creatureLoot == nil {
						s.server.creatureLoot = make(map[uint64]*activeLootState)
					}
					s.server.creatureLoot[uint64(itemGUID)] = loot
					s.server.lootMu.Unlock()
					s.activeLoot = loot
					return s.sendLootResponse(loot) == nil
				}
			}
		}
	}

	// Default empty loot response if no items
	buf := protocol.NewBuffer(32)
	buf.WriteU64(uint64(itemGUID))
	buf.WriteU8(1)  // LOOT_CORPSE / LOOT_ITEM
	buf.WriteU32(0) // gold
	buf.WriteU8(0)  // item count
	_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), buf.Bytes(), true)
	return true
}

// handleReadItem processes CMSG_READ_ITEM (0x0AD).
// Reference: WorldSession::HandleReadItem (ItemHandler.cpp:340).
func (s *session) handleReadItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	bag := payload[0]
	slot := payload[1]
	itemGUID, _, _, err := s.inventoryItemAt(ctx, bag, slot)
	if err != nil || itemGUID == 0 {
		return true
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU64(uint64(itemGUID))
	_ = s.write(uint16(protocol.OpcodeSMSG_READ_ITEM_OK), buf.Bytes(), true)
	return true
}

// handlePageTextQuery processes CMSG_PAGE_TEXT_QUERY (0x05A).
// Reference: WorldSession::HandleQueryPageText (QueryHandler.cpp:277).
func (s *session) handlePageTextQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	pageID, err := r.ReadU32()
	if err != nil {
		return false
	}

	var text string
	var nextPageID uint32
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Text, NextPageID FROM page_text WHERE ID = ? LIMIT 1", pageID).Scan(&text, &nextPageID)
	}

	buf := protocol.NewBuffer(32 + len(text))
	buf.WriteU32(pageID)
	buf.WriteCString(text)
	buf.WriteU32(nextPageID)
	_ = s.write(uint16(protocol.OpcodeSMSG_PAGE_TEXT_QUERY_RESPONSE), buf.Bytes(), true)
	return true
}

// handleWrapItem processes CMSG_WRAP_ITEM (0x1D3).
// Reference: WorldSession::HandleWrapItemOpcode (ItemHandler.cpp:836).
func (s *session) handleWrapItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	giftBag := payload[0]
	giftSlot := payload[1]
	itemBag := payload[2]
	itemSlot := payload[3]

	giftGUID, giftEntry, _, err := s.inventoryItemAt(ctx, giftBag, giftSlot)
	if err != nil || giftGUID == 0 {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}
	targetGUID, targetEntry, targetCount, err := s.inventoryItemAt(ctx, itemBag, itemSlot)
	if err != nil || targetGUID == 0 {
		s.sendEquipError(equipErrItemNotFound, 0)
		return true
	}

	// Cheat check: cannot wrap gift with itself
	if giftGUID == targetGUID {
		s.sendEquipError(equipErrWrappedCantBeWrapped, uint64(targetGUID))
		return true
	}

	// Equipped items cannot be wrapped
	if itemBag == 0 && itemSlot < equipSlotEnd {
		s.sendEquipError(equipErrEquippedCantBeWrapped, uint64(targetGUID))
		return true
	}

	// Stackable items (count > 1) cannot be wrapped
	if targetCount > 1 {
		s.sendEquipError(equipErrStackableCantBeWrapped, uint64(targetGUID))
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB

		// Already wrapped check
		var existingGift uint32
		if err := cdb.QueryRowContext(ctx, "SELECT entry FROM character_gifts WHERE item_guid = ? LIMIT 1", targetGUID).Scan(&existingGift); err == nil && existingGift != 0 {
			s.sendEquipError(equipErrWrappedCantBeWrapped, uint64(targetGUID))
			return true
		}

		// Consume gift wrapper from inventory
		var wrapperCount uint32
		_ = cdb.QueryRowContext(ctx, "SELECT count FROM item_instance WHERE guid = ?", giftGUID).Scan(&wrapperCount)
		if wrapperCount > 1 {
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", giftGUID)
		} else {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?", s.playerGUID, giftBag, giftSlot)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", giftGUID)
		}
		s.refreshQuestItemCounts(ctx, uint32(giftEntry), false)

		// Record original entry in character_gifts
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_gifts (guid, item_guid, entry, flags) VALUES (?, ?, ?, 0)", s.playerGUID, targetGUID, targetEntry)

		// Map wrapped item entry
		var wrappedEntry uint32 = 5043
		switch giftEntry {
		case 5042:
			wrappedEntry = 5043
		case 5048:
			wrappedEntry = 5044
		case 17303:
			wrappedEntry = 17302
		case 17304:
			wrappedEntry = 17305
		case 17307:
			wrappedEntry = 17308
		case 21830:
			wrappedEntry = 21831
		}
		// Set itemEntry = wrappedEntry and flags |= 0x8 (ITEM_FIELD_FLAG_WRAPPED)
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET itemEntry = ?, flags = flags | 8 WHERE guid = ?", wrappedEntry, targetGUID)
		s.refreshQuestItemCounts(ctx, uint32(targetEntry), false)
		s.refreshQuestItemCounts(ctx, wrappedEntry, true)
		_ = s.sendInventoryItems(ctx)
	}
	return true
}

// handleRepairItem processes CMSG_REPAIR_ITEM (0x1F8 / 0x2A8).
// Reference: WorldSession::HandleRepairItemOpcode (NPCHandler.cpp:717).
func (s *session) handleRepairItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // npcGUID
	itemGUID, _ := r.ReadU64()
	_, _ = r.ReadU8() // guildBank

	if s.server == nil || s.server.CharactersStore == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var wdb *sql.DB
	if s.server.WorldStore != nil {
		wdb = s.server.WorldStore.DB
	}

	getMaxDurability := func(entry uint32) uint32 {
		var maxD uint32
		if wdb != nil {
			_ = wdb.QueryRowContext(ctx, "SELECT MaxDurability FROM item_template WHERE entry = ?", entry).Scan(&maxD)
		}
		if maxD == 0 && cdb != nil {
			_ = cdb.QueryRowContext(ctx, "SELECT MaxDurability FROM item_template WHERE entry = ?", entry).Scan(&maxD)
		}
		return maxD
	}

	if itemGUID != 0 {
		rawGUID := itemGUID & 0x0000FFFFFFFFFFFF
		var itemEntry, durability uint32
		err := cdb.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.durability
			FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND (ci.item = ? OR ii.guid = ?) LIMIT 1`,
			s.playerGUID, rawGUID, rawGUID).Scan(&itemEntry, &durability)
		if err != nil {
			err = cdb.QueryRowContext(ctx, "SELECT itemEntry, durability FROM item_instance WHERE guid = ?", rawGUID).Scan(&itemEntry, &durability)
			if err != nil {
				return true
			}
		}
		maxDurability := getMaxDurability(itemEntry)
		if maxDurability > durability {
			cost := (maxDurability - durability) * 10
			if s.player.Money >= cost {
				s.player.Money -= cost
				_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
				_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", maxDurability, rawGUID)
				_ = s.sendInventoryItems(ctx)
				s.sendPlayerUpdate()
			}
		}
	} else {
		type repairItem struct {
			guid uint64
			cost uint32
			maxD uint32
		}
		type rawItem struct {
			guid  uint64
			entry uint32
			curD  uint32
		}
		var rawItems []rawItem
		rows, err := cdb.QueryContext(ctx,
			`SELECT ii.guid, ii.itemEntry, ii.durability
			 FROM character_inventory AS ci
			 JOIN item_instance AS ii ON ii.guid = ci.item
			 WHERE ci.guid = ?`, s.playerGUID)
		if err == nil {
			for rows.Next() {
				var it rawItem
				if err := rows.Scan(&it.guid, &it.entry, &it.curD); err == nil {
					rawItems = append(rawItems, it)
				}
			}
			rows.Close()

			var toRepair []repairItem
			var totalCost uint32
			for _, it := range rawItems {
				maxD := getMaxDurability(it.entry)
				if maxD > it.curD {
					cost := (maxD - it.curD) * 10
					totalCost += cost
					toRepair = append(toRepair, repairItem{guid: it.guid, cost: cost, maxD: maxD})
				}
			}

			if len(toRepair) > 0 {
				if s.player.Money >= totalCost {
					s.player.Money -= totalCost
					_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
					for _, item := range toRepair {
						_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", item.maxD, item.guid)
					}
					_ = s.sendInventoryItems(ctx)
					s.sendPlayerUpdate()
				}
			}
		}
	}
	return true
}

// durabilityLossAll reduces durability on items by a percentage (e.g. 0.10 on death, 0.25 on spirit resurrect).
// If inventory is false, only equipped items (bag == 0 and slot < 19) lose durability.
// If inventory is true, both equipped and inventory/bag items lose durability.
// Reference: Player::DurabilityLossAll (Player.cpp:4890-4932).
func (s *session) durabilityLossAll(ctx context.Context, percent float64, inventory bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB

	query := `SELECT ci.item, ii.itemEntry, ii.durability
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.durability > 0`
	if !inventory {
		query += ` AND ci.bag = 0 AND ci.slot < 19`
	}

	rows, err := cdb.QueryContext(ctx, query, s.playerGUID)
	if err != nil {
		return
	}
	defer rows.Close()

	type itemLoss struct {
		guid      uint64
		itemEntry uint32
		curDur    uint32
	}
	var items []itemLoss
	for rows.Next() {
		var guid uint64
		var itemEntry, curDur uint32
		if err := rows.Scan(&guid, &itemEntry, &curDur); err == nil {
			items = append(items, itemLoss{guid: guid, itemEntry: itemEntry, curDur: curDur})
		}
	}

	maxDurCache := make(map[uint32]uint32)
	type itemDurUpdate struct {
		guid   uint64
		newDur uint32
	}
	var updates []itemDurUpdate

	for _, itm := range items {
		maxDur, ok := maxDurCache[itm.itemEntry]
		if !ok {
			_ = wdb.QueryRowContext(ctx, "SELECT MaxDurability FROM item_template WHERE entry = ?", itm.itemEntry).Scan(&maxDur)
			maxDurCache[itm.itemEntry] = maxDur
		}
		if maxDur == 0 {
			continue
		}
		loss := uint32(float64(maxDur) * percent)
		if loss < 1 {
			loss = 1
		}
		var newDur uint32
		if itm.curDur > loss {
			newDur = itm.curDur - loss
		} else {
			newDur = 0
		}
		updates = append(updates, itemDurUpdate{guid: itm.guid, newDur: newDur})
	}

	for _, up := range updates {
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", up.newDur, up.guid)
	}

	if len(updates) > 0 {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
}

// handleSocketGems processes CMSG_SOCKET_GEMS (0x347).
// Reference: WorldSession::HandleSocketOpcode (ItemHandler.cpp:947).
func (s *session) handleSocketGems(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 32 {
		return true
	}
	r := protocol.NewReader(payload)
	itemGUID, err := r.ReadU64()
	if err != nil || itemGUID == 0 {
		return true
	}
	var gemGUIDs [3]uint64
	for i := 0; i < 3; i++ {
		gemGUIDs[i], _ = r.ReadU64()
	}

	// Cheat check: cannot socket the same gem multiple times
	if (gemGUIDs[0] != 0 && (gemGUIDs[0] == gemGUIDs[1] || gemGUIDs[0] == gemGUIDs[2])) ||
		(gemGUIDs[1] != 0 && gemGUIDs[1] == gemGUIDs[2]) {
		return true
	}

	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB

	rawTargetGUID := itemGUID & 0x0000FFFFFFFFFFFF
	var targetEntry uint32
	var targetBag, targetSlot uint8
	var currentEnchants string
	err = cdb.QueryRowContext(ctx, `SELECT ii.itemEntry, ci.bag, ci.slot, COALESCE(ii.enchantments, '')
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND (ci.item = ? OR ii.guid = ?) LIMIT 1`,
		s.playerGUID, rawTargetGUID, rawTargetGUID).Scan(&targetEntry, &targetBag, &targetSlot, &currentEnchants)
	if err != nil {
		return true
	}

	var targetSockets [3]uint32
	var targetSocketBonus uint32
	if targetData, err := s.loadItemQueryData(ctx, targetEntry); err == nil {
		for i := 0; i < 3; i++ {
			targetSockets[i] = targetData.Sockets[i].Color
		}
		targetSocketBonus = targetData.SocketBonus
	} else if cdb != nil {
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(socketColor_1, 0), COALESCE(socketColor_2, 0), COALESCE(socketColor_3, 0), COALESCE(socketBonus, 0) FROM item_template WHERE entry = ?", targetEntry).Scan(&targetSockets[0], &targetSockets[1], &targetSockets[2], &targetSocketBonus)
	}

	var gemEnchants [3]uint32
	var gemColors [3]uint32
	var gemEntries [3]uint32
	for i := 0; i < 3; i++ {
		if gemGUIDs[i] == 0 {
			continue
		}
		rawGemGUID := gemGUIDs[i] & 0x0000FFFFFFFFFFFF
		var gemEntry uint32
		err := cdb.QueryRowContext(ctx, `SELECT ii.itemEntry
			FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND (ci.item = ? OR ii.guid = ?) LIMIT 1`,
			s.playerGUID, rawGemGUID, rawGemGUID).Scan(&gemEntry)
		if err != nil {
			return true
		}
		gemEntries[i] = gemEntry

		var gemPropID uint32
		if gemData, err := s.loadItemQueryData(ctx, gemEntry); err == nil {
			gemPropID = gemData.GemProperties
		} else if cdb != nil {
			_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(GemProperties, 0) FROM item_template WHERE entry = ?", gemEntry).Scan(&gemPropID)
		}

		if s.server.Data != nil {
			if gp, ok, _ := s.server.Data.GemProperties(gemPropID); ok {
				gemEnchants[i] = gp.EnchantID
				gemColors[i] = gp.Type
			}
		}
		if gemEnchants[i] == 0 && gemPropID != 0 {
			gemEnchants[i] = gemPropID
			gemColors[i] = 14 // match red/yellow/blue by default
		}
	}

	// Parse existing 36 ints from enchantments column
	fields := strings.Fields(currentEnchants)
	var enchants [36]uint32
	for i := 0; i < len(fields) && i < 36; i++ {
		if val, err := strconv.ParseUint(fields[i], 10, 32); err == nil {
			enchants[i] = uint32(val)
		}
	}

	// Slot 2: Sock 1 (index 6)
	// Slot 3: Sock 2 (index 9)
	// Slot 4: Sock 3 (index 12)
	if gemEnchants[0] != 0 {
		enchants[6] = gemEnchants[0]
	}
	if gemEnchants[1] != 0 {
		enchants[9] = gemEnchants[1]
	}
	if gemEnchants[2] != 0 {
		enchants[12] = gemEnchants[2]
	}

	// Check socket bonus match
	bonusMatches := true
	hasAnySocket := false
	for i := 0; i < 3; i++ {
		sockColor := targetSockets[i]
		if sockColor == 0 {
			continue
		}
		hasAnySocket = true
		gColor := gemColors[i]
		if gColor == 0 || (gColor&sockColor) == 0 {
			bonusMatches = false
			break
		}
	}

	var activeBonus uint32
	if hasAnySocket && bonusMatches && targetSocketBonus != 0 {
		activeBonus = targetSocketBonus
	}
	enchants[15] = activeBonus

	// Serialize updated enchantments
	encParts := make([]string, 36)
	for i := 0; i < 36; i++ {
		encParts[i] = strconv.FormatUint(uint64(enchants[i]), 10)
	}
	newEncStr := strings.Join(encParts, " ")
	_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET enchantments = ? WHERE guid = ?", newEncStr, rawTargetGUID)

	// Send SMSG_SOCKET_GEMS_RESULT (0x50B) first per TrinityCore HandleSocketOpcode
	resBuf := protocol.NewBuffer(24)
	resBuf.WriteU64(itemGUID)
	resBuf.WriteU32(enchants[6])
	resBuf.WriteU32(enchants[9])
	resBuf.WriteU32(enchants[12])
	resBuf.WriteU32(enchants[15])
	_ = s.write(uint16(protocol.OpcodeSMSG_SOCKET_GEMS_RESULT), resBuf.Bytes(), true)

	// Consume the socketed gems
	for index, gemGUID := range gemGUIDs {
		if gemGUID != 0 {
			rawGemGUID := gemGUID & 0x0000FFFFFFFFFFFF
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ? AND guid = ?", rawGemGUID, s.playerGUID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", rawGemGUID)
			s.refreshQuestItemCounts(ctx, gemEntries[index], false)
			s.despawnItem(rawGemGUID)
		}
	}

	if targetBag == 0 && targetSlot < equipSlotEnd {
		s.syncEquipmentCache(ctx)
	}
	_ = s.sendInventoryItems(ctx)
	return true
}

// handleSetAmmo processes CMSG_SET_AMMO (0x268).
// Reference: WorldSession::HandleSetAmmoOpcode (ItemHandler.cpp:772).
func (s *session) handleSetAmmo(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	itemEntry, _ := r.ReadU32()
	s.player.AmmoID = itemEntry
	_ = s.calculatePlayerStats(ctx, s.player)
	s.sendPlayerUpdate()
	return true
}

const (
	maxEquipmentSetIndex uint32 = 10
	equipmentSlotEnd     uint32 = 19
)

// handleEquipmentSetSave processes CMSG_EQUIPMENT_SET_SAVE (0x4BD).
// Reference: WorldSession::HandleEquipmentSetSave (CharacterHandler.cpp:1492).
func (s *session) handleEquipmentSetSave(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	setGuid, err := r.ReadPackedGUID()
	if err != nil {
		return false
	}
	index, err := r.ReadU32()
	if err != nil || index >= maxEquipmentSetIndex {
		return false
	}
	name, err := r.ReadCString()
	if err != nil {
		return false
	}
	iconName, err := r.ReadCString()
	if err != nil {
		return false
	}

	var items [19]uint64
	var ignoreMask uint32
	for i := uint32(0); i < equipmentSlotEnd; i++ {
		itemGuid, err := r.ReadPackedGUID()
		if err != nil {
			break
		}
		if itemGuid == 1 {
			ignoreMask |= 1 << i
			continue
		}
		items[i] = itemGuid
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		placeholders := strings.Repeat("?, ", 24) + "?"
		cols := "guid, setguid, setindex, name, iconname, ignore_mask"
		for i := 0; i < 19; i++ {
			cols += fmt.Sprintf(", item%d", i)
		}
		args := []any{s.playerGUID, setGuid, index, name, iconName, ignoreMask}
		for i := 0; i < 19; i++ {
			args = append(args, items[i])
		}
		query := fmt.Sprintf("REPLACE INTO character_equipmentsets (%s) VALUES (%s)", cols, placeholders)
		_, _ = cdb.ExecContext(ctx, query, args...)
	}

	// Send SMSG_EQUIPMENT_SET_SAVED (0x137)
	savedBuf := protocol.NewBuffer(16)
	savedBuf.WriteU32(index)
	savedBuf.WritePackedGUID(setGuid)
	_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_SAVED), savedBuf.Bytes(), true)
	return true
}

func (s *session) sendEquipmentSetList(ctx context.Context) {
	empty := func() {
		if s != nil {
			_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_LIST), []byte{0, 0, 0, 0}, true)
		}
	}
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		empty()
		return
	}
	cdb := s.server.CharactersStore.DB
	rows, err := cdb.QueryContext(ctx, `SELECT setguid, setindex, name, iconname, ignore_mask,
		item0, item1, item2, item3, item4, item5, item6, item7, item8, item9,
		item10, item11, item12, item13, item14, item15, item16, item17, item18
		FROM character_equipmentsets WHERE guid = ? ORDER BY setindex`, s.playerGUID)
	if err != nil {
		empty()
		return
	}
	defer rows.Close()

	type eqSetEntry struct {
		setGUID    uint64
		setIndex   uint32
		name       string
		iconName   string
		ignoreMask uint32
		items      [19]uint64
	}
	var sets []eqSetEntry
	for rows.Next() {
		var entry eqSetEntry
		var itemCols [19]int64
		scanArgs := []any{&entry.setGUID, &entry.setIndex, &entry.name, &entry.iconName, &entry.ignoreMask}
		for i := 0; i < 19; i++ {
			scanArgs = append(scanArgs, &itemCols[i])
		}
		if err := rows.Scan(scanArgs...); err != nil {
			continue
		}
		if entry.setIndex >= maxEquipmentSetIndex {
			continue
		}
		for i := 0; i < 19; i++ {
			if itemCols[i] > 0 {
				entry.items[i] = uint64(itemCols[i]) | (uint64(0x4000) << 48)
			}
		}
		sets = append(sets, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		empty()
		return
	}
	if err := rows.Close(); err != nil {
		empty()
		return
	}

	buf := protocol.NewBuffer(4 + len(sets)*128)
	buf.WriteU32(uint32(len(sets)))
	for _, set := range sets {
		buf.WritePackedGUID(set.setGUID)
		buf.WriteU32(set.setIndex)
		buf.WriteCString(set.name)
		buf.WriteCString(set.iconName)
		for i := uint32(0); i < 19; i++ {
			if set.ignoreMask&(1<<i) != 0 {
				buf.WritePackedGUID(1)
			} else {
				buf.WritePackedGUID(set.items[i])
			}
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_LIST), buf.Bytes(), true)
}

// handleEquipmentSetDelete processes CMSG_DELETEEQUIPMENT_SET (0x13E).
// Reference: WorldSession::HandleEquipmentSetDelete (CharacterHandler.cpp:1544).
func (s *session) handleEquipmentSetDelete(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	setGuid, err := r.ReadPackedGUID()
	if err != nil {
		return false
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_equipmentsets WHERE setguid = ? AND guid = ?", setGuid, s.playerGUID)
	}
	return true
}

// handleEquipmentSetUse processes CMSG_EQUIPMENT_SET_USE (0x4D5).
// Reference: WorldSession::HandleEquipmentSetUse (CharacterHandler.cpp:1554).
func (s *session) handleEquipmentSetUse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)

	for i := uint32(0); i < equipmentSlotEnd; i++ {
		itemGuid, err := r.ReadPackedGUID()
		if err != nil {
			break
		}
		srcbag, err := r.ReadU8()
		if err != nil {
			break
		}
		srcslot, err := r.ReadU8()
		if err != nil {
			break
		}
		if itemGuid == 1 || itemGuid == 0 {
			continue
		}
		// If item is in bags, swap to equipment slot
		if srcbag != 0 || srcslot >= uint8(equipmentSlotEnd) {
			_ = s.handleSwapItem(ctx, []byte{0, uint8(i), srcbag, srcslot})
		}
	}

	// Send SMSG_EQUIPMENT_SET_USE_RESULT (0x4D6) with 0 = success
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0) // 0 = ERR_EQUIPMENT_SET_USE_SUCCESS
	_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_USE_RESULT), buf.Bytes(), true)
	return true
}
