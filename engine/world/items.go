package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	itemFlagUniqueEquippable uint32 = 0x00080000
	itemFlagIsProspectable   uint32 = 0x00040000 // ITEM_FLAG_IS_PROSPECTABLE (ItemTemplate.h:170)
	itemFlagIsMillable       uint32 = 0x20000000 // ITEM_FLAG_IS_MILLABLE (ItemTemplate.h:181)
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

// grantMaxSkill mirrors the Denveous-marker SetSkill(itemSkill, 0, 400, 400)
// arms in Player::CanUseItem (Player.cpp:11875, 11957): when the player has
// the item's equip spell or proficiency skill (or a required spell/skill) but
// its value is zero, the skill is maxed out and persisted to character_skills.
func (s *session) grantMaxSkill(ctx context.Context, state *playerState, skillID uint32) {
	if s == nil || state == nil || skillID == 0 || skillID > 0xffff {
		return
	}
	for i := range state.Skills {
		if uint32(state.Skills[i].Skill) == skillID {
			if state.Skills[i].Value != 0 {
				return
			}
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

// canUseItemResult mirrors Player::CanUseItem(Item*) (Player.cpp:11859) in
// InventoryResult form for the use-item path: the dead gate lives in
// handleUseItem and the level gate in its template-gates block, so this
// covers the remaining arms in C++ order — the Denveous-marker AX1 bypass
// (Player.cpp:11871), the proto arms (Player.cpp:11932: faction flags2,
// allowable class/race, Denveous-marker AX2 bypass, RequiredSkill/Rank,
// RequiredSpell, HolidayId, the 483/55884 learning arm), the
// GetSkill()-proficiency + heirloom-exception arm (Player.cpp:11984) and the
// reputation arm (Player.cpp:12008). IsBindedNotWith is vacuous here: the
// item row was read from this player's own character_inventory, so the owner
// is necessarily the player. Eluna's OnCanUseItem has no Go fire site.
func (s *session) canUseItemResult(ctx context.Context, itemClass, itemSubClass, itemTplFlags2 uint32, allowableClass, allowableRace int64, reqSkill, reqSkillRank, reqSpell, holidayID, quality, reqRepFaction, reqRepRank uint32, itemSpellIDs [5]int64) uint8 {
	if s == nil || s.player == nil {
		return equipErrItemNotFound
	}
	// Item::GetSpell/GetSkill (Item.cpp:545-588) — class/subclass tables, the
	// same rows as the canUseItemData path above.
	weaponSkills := [...]uint32{44, 172, 45, 46, 54, 160, 229, 43, 55, 0, 136, 0, 0, 473, 0, 173, 176, 253, 226, 228, 356}
	weaponSpells := [...]uint32{196, 197, 264, 266, 198, 199, 200, 201, 202, 0, 227, 0, 0, 0, 0, 1180, 2567, 3386, 5011, 5009, 0}
	armorSkills := [...]uint32{0, 415, 414, 413, 293, 0, 433, 0, 0, 0, 0}
	armorSpells := [...]uint32{0, 9078, 9077, 8737, 750, 0, 9116, 0, 0, 0, 0}
	itemSkill, itemSpell := uint32(0), uint32(0)
	switch itemClass {
	case itemClassWeapon:
		if itemSubClass < uint32(len(weaponSkills)) {
			itemSkill, itemSpell = weaponSkills[itemSubClass], weaponSpells[itemSubClass]
		}
	case itemClassArmor:
		if itemSubClass < uint32(len(armorSkills)) {
			itemSkill, itemSpell = armorSkills[itemSubClass], armorSpells[itemSubClass]
		}
	}
	hasSkill := func(skillID uint32) bool {
		if skillID == 0 {
			return false
		}
		for _, skill := range s.player.Skills {
			if uint32(skill.Skill) == skillID {
				return true
			}
		}
		return false
	}
	// Denveous's Marker AX1 (Player.cpp:11871-11880) — the equip spell or
	// proficiency skill is known: max a zero skill out and accept outright.
	if playerHasSpell(s.player, itemSpell) || hasSkill(itemSkill) {
		s.grantMaxSkill(ctx, s.player, itemSkill)
		return equipErrOk
	}
	team := playerTeam(s.player.Race)
	if (itemTplFlags2&0x01 != 0 && team != teamHorde) || (itemTplFlags2&0x02 != 0 && team != teamAlliance) {
		return equipErrYouCanNeverUseThatItem
	}
	classMask, raceMask := uint32(0), uint32(0)
	if s.player.Class > 0 {
		classMask = uint32(1) << uint(s.player.Class-1)
	}
	if s.player.Race > 0 {
		raceMask = uint32(1) << uint(s.player.Race-1)
	}
	if uint32(allowableClass)&classMask == 0 || uint32(allowableRace)&raceMask == 0 {
		return equipErrYouCanNeverUseThatItem
	}
	// Denveous's Marker AX2 (Player.cpp:11953-11963).
	if playerHasSpell(s.player, reqSpell) || hasSkill(reqSkill) {
		s.grantMaxSkill(ctx, s.player, reqSkill)
		return equipErrOk
	}
	if reqSkill != 0 {
		skillVal := playerSkillTotalValue(s.player, reqSkill)
		if skillVal == 0 {
			return equipErrNoRequiredProficiency
		} else if skillVal < int32(reqSkillRank) {
			return equipErrCantEquipSkill
		}
	}
	if reqSpell != 0 && !playerHasSpell(s.player, reqSpell) {
		return equipErrNoRequiredProficiency
	}
	if holidayID != 0 {
		if _, active := s.server.cachedActiveGameHolidays(ctx)[int64(holidayID)]; !active {
			return equipErrCantDoRightNow
		}
	}
	// learning (recipes, mounts, pets, etc.): C++ returns EQUIP_ERR_NONE (59)
	// when the taught spell is already known — a denial, not EQUIP_ERR_OK.
	if (itemSpellIDs[0] == 483 || itemSpellIDs[0] == 55884) && itemSpellIDs[1] > 0 && playerHasSpell(s.player, uint32(itemSpellIDs[1])) {
		return equipErrNone
	}
	if itemSkill != 0 {
		allowEquip := false
		if quality == 7 /* ITEM_QUALITY_HEIRLOOM */ && itemClass == itemClassArmor && !hasSkill(itemSkill) {
			switch s.player.Class {
			case 3, 7:
				allowEquip = itemSkill == 413
			case 1, 2:
				allowEquip = itemSkill == 293
			}
		}
		if !allowEquip && playerSkillTotalValue(s.player, itemSkill) == 0 {
			return equipErrNoRequiredProficiency
		}
	}
	if reqRepFaction != 0 {
		rank := uint32(3)
		for _, reputation := range s.player.Reputations {
			if reputation.FactionID == reqRepFaction {
				rank = reputationRank(int64(totalReputationStanding(reputation)))
				break
			}
		}
		if rank < reqRepRank {
			return equipErrCantEquipReputation
		}
	}
	return equipErrOk
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
		s.grantMaxSkill(ctx, state, skillID)
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

	invSlotBagStart     uint8 = 19
	invSlotBagEnd       uint8 = 23
	invSlotItemStart    uint8 = 23
	invSlotItemEnd      uint8 = 39
	invSlotKeyringStart uint8 = 86
	invSlotKeyringEnd   uint8 = 118

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

func (s *session) itemIsBag(ctx context.Context, itemGUID uint64) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var slots int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(ContainerSlots, 0) FROM item_template WHERE entry = (SELECT itemEntry FROM item_instance WHERE guid = ?)`, itemGUID).Scan(&slots); err != nil || slots <= 0 {
		return false
	}
	return true
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
	// Player::SwapItem/StoreItem equipment transitions (Player.cpp:12419):
	// itemA moved (bagA,slotA)->(bagB,slotB), itemB moved (bagB,slotB)->(bagA,slotA).
	if itemA != 0 {
		s.updateItemEquipSpellsOnMove(ctx, itemA, bagA, slotA, bagB, slotB)
	}
	if itemB != 0 {
		s.updateItemEquipSpellsOnMove(ctx, itemB, bagB, slotB, bagA, slotA)
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

	// Bag-lift checks (Player::CanUnequipItem, Player.cpp:11626): a non-empty bag
	// may leave its bag slot only when the C++ swap term holds — dst is a bag
	// position, or the item at dst is an empty bag (Player.cpp:13170); the
	// destination side is symmetric (Player.cpp:13207). The !IsBagPos(src) arm
	// is folded away: these branches only run for bag positions.
	srcIsBagPos := srcBagKey == 0 && ((srcSlot >= invSlotBagStart && srcSlot < invSlotBagEnd) || (srcSlot >= 67 && srcSlot <= 73))
	dstIsBagPos := dstBagKey == 0 && ((dstSlot >= invSlotBagStart && dstSlot < invSlotBagEnd) || (dstSlot >= 67 && dstSlot <= 73))
	dstIsEmptyBag := dstItemGUID != 0 && s.itemIsBag(ctx, uint64(dstItemGUID)) && s.isBagEmpty(ctx, dstItemGUID)
	srcIsEmptyBag := srcItemGUID != 0 && s.itemIsBag(ctx, uint64(srcItemGUID)) && s.isBagEmpty(ctx, srcItemGUID)
	if srcIsBagPos && srcItemGUID != 0 && !dstIsBagPos && !dstIsEmptyBag && s.itemIsNonemptyBag(ctx, uint64(srcItemGUID)) {
		s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(srcItemGUID))
		return true
	}
	if dstIsBagPos && dstItemGUID != 0 && !srcIsBagPos && !srcIsEmptyBag && s.itemIsNonemptyBag(ctx, uint64(dstItemGUID)) {
		s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(dstItemGUID))
		return true
	}

	// Moving a non-empty bag into a specific slot that is not a bag position
	// is rejected (Player::CanStoreItem_InSpecificSlot, Player.cpp:10518);
	// the same code guards the in-bag path (Player::CanStoreItem_InBag),
	// where the destination can never be a bag position. The full-swap case
	// checks both move directions in C++ order (src->dst, then dst->src).
	if srcItemGUID != 0 && !(dstBagKey == 0 && ((dstSlot >= invSlotBagStart && dstSlot < invSlotBagEnd) || (dstSlot >= 67 && dstSlot <= 73))) && s.itemIsNonemptyBag(ctx, uint64(srcItemGUID)) {
		s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(srcItemGUID))
		return true
	}
	if dstItemGUID != 0 && !(srcBagKey == 0 && ((srcSlot >= invSlotBagStart && srcSlot < invSlotBagEnd) || (srcSlot >= 67 && srcSlot <= 73))) && s.itemIsNonemptyBag(ctx, uint64(dstItemGUID)) {
		s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(dstItemGUID))
		return true
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
	removedCount := uint32(count)
	if count == 0 || currentCount <= int64(count) {
		removedCount = uint32(currentCount)
	}
	s.adjustQuestItemCount(ctx, uint32(itemEntry), removedCount, false)
	// Player::RemoveItem -> _ApplyItemMods(false) (Player.cpp:12419): a
	// destroyed equipped item sheds its equip spells before the row goes.
	if bagKey == 0 && slot < equipSlotEnd && (currentCount <= int64(count) || count == 0) {
		s.applyItemEquipSpells(ctx, itemGUID, uint32(itemEntry), false)
	}
	if currentCount <= int64(count) || count == 0 {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
		_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
		s.despawnItem(uint64(itemGUID))
	} else {
		_, _ = db.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", count, itemGUID)
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	s.debug("item destroyed", "account", s.accountName, "item", itemGUID, "bag", bag, "slot", slot, "count", count)
	return true
}

func (s *session) consumeInventoryItemByGUID(ctx context.Context, itemGUID uint64, count uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || itemGUID == 0 || count == 0 {
		return false
	}
	db := s.server.CharactersStore.DB
	var itemEntry, currentCount int64
	if err := db.QueryRowContext(ctx, "SELECT ii.itemEntry, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ? LIMIT 1", s.playerGUID, int64(itemGUID)).Scan(&itemEntry, &currentCount); err != nil || itemEntry <= 0 || currentCount < int64(count) {
		return false
	}
	s.adjustQuestItemCount(ctx, uint32(itemEntry), count, false)
	if currentCount == int64(count) {
		if _, err := db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, int64(itemGUID)); err != nil {
			return false
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", int64(itemGUID)); err != nil {
			return false
		}
		s.despawnItem(itemGUID)
	} else if _, err := db.ExecContext(ctx, "UPDATE item_instance SET count = count - ? WHERE guid = ?", count, int64(itemGUID)); err != nil {
		return false
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
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
	cacheStr := strings.Join(parts, " ") + " "
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
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(ii.text, '') FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&text)
		found = err == nil
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
	// WorldSession::HandleItemRefundInfoRequest (ItemHandler.cpp:1169-1179)
	// resolves the item with Player::GetItemByGuid — the item must be in the
	// player's possession (inventory/bank/equipped); a bare refund row for an
	// item the player no longer holds gets no answer. Same
	// character_inventory JOIN as handleItemRefund.
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, iri.paidMoney, iri.paidExtendedCost
		FROM item_instance AS ii JOIN item_refund_instance AS iri ON iri.item_guid = ii.guid AND iri.player_guid = ?
		JOIN character_inventory AS ci ON ci.item = ii.guid AND ci.guid = ?
		WHERE ii.guid = ? LIMIT 1`, s.playerGUID, s.playerGUID, itemGUID).Scan(&itemEntry, &paidMoney, &paidExtendedCost)
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

	// WorldSession::HandleItemRefund (ItemHandler.cpp:1186-1204): an item
	// currently being disenchanted is silently ignored, exactly like a
	// missing item.
	if loot := s.activeLoot; loot != nil && loot.TargetGUID == rawItemGUID {
		return true
	}

	var itemEntry, itemCount, paidMoney, paidExtendedCost int64
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.count, iri.paidMoney, iri.paidExtendedCost FROM item_instance AS ii
		JOIN character_inventory AS ci ON ci.item = ii.guid
		JOIN item_refund_instance AS iri ON iri.item_guid = ii.guid AND iri.player_guid = ci.guid
		WHERE ii.guid = ? AND ci.guid = ? LIMIT 1`, itemGUID, s.playerGUID).Scan(&itemEntry, &itemCount, &paidMoney, &paidExtendedCost)
	// Player::RefundItem (Player.cpp:26576-26580): a missing or non-refundable
	// item (no refund row, or a traded-away recipient) is a silent return —
	// no SMSG_ITEM_REFUND_RESULT reaches the client.
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
		// Player.cpp:26599-26605: a missing extended-cost entry only logs and
		// returns — the client gets no result packet.
		if err != nil || !found {
			return true
		}
	}
	// Player.cpp:26607-26623: the extended-cost item space is feasibility-
	// checked for ALL requirements before anything is granted — a failure
	// answers SMSG_ITEM_REFUND_RESULT error 10 with zero side effects. The
	// old code granted items first, so a mid-loop failure left a partial
	// grant with no refund recorded.
	type refundGrant struct{ entry, count uint32 }
	var grants []refundGrant
	for i := 0; i < len(extendedCost.ItemIDs); i++ {
		if extendedCost.ItemIDs[i] == 0 || extendedCost.ItemCounts[i] == 0 {
			continue
		}
		if s.canStoreNewItem(ctx, s.playerGUID, extendedCost.ItemIDs[i], extendedCost.ItemCounts[i]) != equipErrOk {
			buf := protocol.NewBuffer(64)
			buf.WriteU64(rawItemGUID)
			buf.WriteU32(10) // error
			return s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) == nil
		}
		grants = append(grants, refundGrant{entry: extendedCost.ItemIDs[i], count: extendedCost.ItemCounts[i]})
	}

	// Player.cpp:26625-26635: the success result is sent BEFORE the refund
	// data is cleared, the item destroyed and the costs granted back.
	buf := protocol.NewBuffer(64)
	buf.WriteU64(rawItemGUID)
	buf.WriteU32(0)
	buf.WriteU32(uint32(paidMoney))
	buf.WriteU32(extendedCost.HonorPoints)
	buf.WriteU32(extendedCost.ArenaPoints)
	for i := 0; i < len(extendedCost.ItemIDs); i++ {
		buf.WriteU32(extendedCost.ItemIDs[i])
		buf.WriteU32(extendedCost.ItemCounts[i])
	}
	if s.write(uint16(protocol.OpcodeSMSG_ITEM_REFUND_RESULT), buf.Bytes(), true) != nil {
		return false
	}

	// Player::RefundItem tail (Player.cpp:26639-26686): SetNotRefundable,
	// DestroyItem, then the extended-cost items (each with
	// SendNewItem(it, count, received=true, created=false, broadcast=true)),
	// money, honor and arena grants.
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ? AND guid = ?", itemGUID, s.playerGUID)
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_refund_instance WHERE item_guid = ? AND player_guid = ?", itemGUID, s.playerGUID)
	s.adjustQuestItemCount(ctx, uint32(itemEntry), uint32(itemCount), false)

	for _, g := range grants {
		res, storeErr := s.storeOrStackItem(ctx, s.playerGUID, g.entry, g.count)
		if storeErr != nil || res == nil {
			continue
		}
		slotForPush := uint32(res.Slot)
		if res.IsStack {
			slotForPush = 0xFFFFFFFF
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(s.playerGUID, res.ClientBag, slotForPush, g.entry, g.count, res.InventoryCount, res.IsStack), true)
	}

	s.player.Money += uint32(paidMoney)
	s.player.TotalHonorPoints += extendedCost.HonorPoints
	s.player.ArenaPoints += extendedCost.ArenaPoints
	if s.player.TotalHonorPoints > 0 {
		s.addKnownCurrency(s.player, itemHonorPointsID)
	}
	if s.player.ArenaPoints > 0 {
		s.addKnownCurrency(s.player, itemArenaPointsID)
	}
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ?, arenaPoints = ?, totalHonorPoints = ? WHERE guid = ?", s.player.Money, s.player.ArenaPoints, s.player.TotalHonorPoints, s.playerGUID)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	return true
}

const (
	equipErrOk              = 0
	equipErrCantEquipLevelI = 1
	// C++ EQUIP_ERR_CANT_EQUIP_SKILL (ItemDefines.h:28); the pre-existing
	// equipErrItemDoesntGoToSlot const shares this value by a stale mislabel.
	equipErrCantEquipSkill                    = 2
	equipErrItemDoesntGoToSlot                = 2
	equipErrNoRequiredProficiency             = 8  // C++ EQUIP_ERR_NO_REQUIRED_PROFICIENCY (ItemDefines.h:34)
	equipErrYouCanNeverUseThatItem            = 10 // C++ EQUIP_ERR_YOU_CAN_NEVER_USE_THAT_ITEM (ItemDefines.h:36)
	equipErrDontOwnThatItem                   = 32 // C++ EQUIP_ERR_DONT_OWN_THAT_ITEM (ItemDefines.h:58)
	equipErrCantEquipReputation               = 64 // C++ EQUIP_ERR_CANT_EQUIP_REPUTATION (ItemDefines.h:90)
	equipErrBagFull                           = 4
	equipErrNonemptyBagOverOtherBag           = 5
	equipErrCantEquipWithTwohanded            = 13
	equipErrCantDualWield                     = 14
	equipErrCantCarryMoreOfThis               = 17
	equipErrItemDoesntGoIntoBag               = 15
	equipErrItemCantBeEquipped                = 20
	equipErrItemsCantBeSwapped                = 21
	equipErrSlotIsEmpty                       = 22
	equipErrItemNotFound                      = 23
	equipErrNotEnoughMoney                    = 29
	equipErrCanOnlyDoWithEmptyBags            = 31
	equipErrItemLocked                        = 36 // C++ EQUIP_ERR_ITEM_LOCKED (ItemDefines.h:62)
	equipErrYouAreDead                        = 38
	equipErrCantDoRightNow                    = 39
	equipErrStackableCantBeWrapped            = 43
	equipErrEquippedCantBeWrapped             = 44
	equipErrWrappedCantBeWrapped              = 45
	equipErrBoundCantBeWrapped                = 46
	equipErrUniqueCantBeWrapped               = 47
	equipErrBagsCantBeWrapped                 = 48
	equipErrInvFull                           = 50
	equipErrCouldntSplitItems                 = 27 // C++ EQUIP_ERR_COULDNT_SPLIT_ITEMS (ItemDefines.h:53)
	equipErrAlreadyLooted                     = 49 // C++ EQUIP_ERR_ALREADY_LOOTED (ItemDefines.h:75)
	equipErrTooMuchGold                       = 77
	equipErrItemMaxLimitCategoryCountExceeded = 84
	equipErrCantEquipRank                     = 63
	equipErrVendorMissingTurnins              = 68
	equipErrNotEnoughHonorPoints              = 69
	equipErrNotEnoughArenaPoints              = 70
	// equipErrNone is C++ EQUIP_ERR_NONE (59): Eluna::OnUse's tail
	// (ItemHooks.cpp) sends it raw in SMSG_INVENTORY_CHANGE_FAILURE when a
	// Lua handler blocks the cast, to un-stick the grayed item client-side.
	equipErrNone                = 59
	equipErrNotInCombat         = 60 // C++ EQUIP_ERR_NOT_IN_COMBAT (ItemDefines.h:86)
	equipErrNotDuringArenaMatch = 78 // C++ EQUIP_ERR_NOT_DURING_ARENA_MATCH (ItemDefines.h:103)
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
// takeCastItemSpellCharges mirrors Spell::TakeCastItem (Spell.cpp:4711-4781):
// items whose template spells carry charges decrement the instance charges
// toward zero (abs(charges) - 1 per use); expendable items (negative template
// charges) are destroyed once the last charge is spent. Returns true when the
// item was destroyed. The TRIGGERED_IGNORE_CAST_ITEM gate is vacuous here —
// spendCastItemCharges runs only from finishSpellCast, which serves only
// player-initiated (TRIGGERED_NONE) casts.
func (s *session) takeCastItemSpellCharges(ctx context.Context, dbItemGUID, itemEntry, bagKey int64, slot uint8) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	var spellIDs, tplCharges [5]int64
	var stackable int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT spellid_1, spellid_2, spellid_3, spellid_4, spellid_5,
		spellcharges_1, spellcharges_2, spellcharges_3, spellcharges_4, spellcharges_5,
		stackable
		FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(
		&spellIDs[0], &spellIDs[1], &spellIDs[2], &spellIDs[3], &spellIDs[4],
		&tplCharges[0], &tplCharges[1], &tplCharges[2], &tplCharges[3], &tplCharges[4],
		&stackable); err != nil {
		return false
	}
	var defaults [5]int32
	for i := range defaults {
		defaults[i] = int32(tplCharges[i])
	}
	var rawCharges string
	if err := cdb.QueryRowContext(ctx, `SELECT COALESCE(charges, '') FROM item_instance WHERE guid = ? LIMIT 1`, dbItemGUID).Scan(&rawCharges); err != nil {
		return false
	}
	charges := parseItemSpellCharges(rawCharges, defaults)
	expendable := false
	withoutCharges := false
	chargeUsed := false
	for i := 0; i < 5; i++ {
		if spellIDs[i] <= 0 || tplCharges[i] == 0 {
			continue
		}
		if tplCharges[i] < 0 {
			expendable = true
		}
		c := int32(charges[i])
		if c != 0 {
			// abs(charges) less at 1 after use (Spell.cpp:4751)
			if c > 0 {
				c--
			} else {
				c++
			}
			// Spell.cpp:4752 — SetSpellCharges is gated on Stackable == 1:
			// instance charges on non-stackable items always read the
			// template defaults.
			if stackable == 1 {
				charges[i] = uint32(c)
			}
			chargeUsed = true
		}
		// all charges used (Spell.cpp:4760). The destroy arm does not share
		// the Stackable gate: an expendable non-stackable item is still
		// destroyed once the local count reaches 0.
		withoutCharges = (c == 0)
	}
	if chargeUsed {
		if stackable == 1 {
			var sb strings.Builder
			for i, charge := range charges {
				if i > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(strconv.FormatInt(int64(int32(charge)), 10))
			}
			_, _ = cdb.ExecContext(ctx, `UPDATE item_instance SET charges = ? WHERE guid = ?`, sb.String(), dbItemGUID)
		}
		// Spell.cpp:4753 — SetState(ITEM_CHANGED, player) fires whenever a
		// charge was spent, even when no persistence happens.
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
	if expendable && withoutCharges {
		var count int64
		_ = cdb.QueryRowContext(ctx, `SELECT count FROM item_instance WHERE guid = ? LIMIT 1`, dbItemGUID).Scan(&count)
		if count > 1 {
			_, _ = cdb.ExecContext(ctx, `UPDATE item_instance SET count = count - 1 WHERE guid = ?`, dbItemGUID)
		} else {
			_, _ = cdb.ExecContext(ctx, `DELETE FROM character_inventory WHERE guid = ? AND bag = ? AND slot = ?`, s.playerGUID, bagKey, slot)
			_, _ = cdb.ExecContext(ctx, `DELETE FROM item_instance WHERE guid = ?`, dbItemGUID)
			s.despawnItem(uint64(dbItemGUID))
		}
		s.adjustQuestItemCount(ctx, uint32(itemEntry), 1, false)
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
		return true
	}
	return false
}

// castItemIsSpellReagent mirrors the TakeReagents cast-item-as-reagent arm
// (Spell.cpp:5062-5081): when the cast item's own template entry is one of
// the spell's reagents, TakeReagents nulls m_CastItem, so the completion-time
// TakeCastItem never runs on it — the item is consumed as a reagent instead.
func castItemIsSpellReagent(spell wotlk.Spell, itemEntry uint32) bool {
	for _, r := range spell.Reagent {
		if r > 0 && uint32(r) == itemEntry {
			return true
		}
	}
	return false
}

// spendCastItemCharges is the completion-time Spell::TakeCastItem
// (Spell.cpp:4711-4781) for item casts, called from finishSpellCast: on the
// immediate branch it runs after _handle_finish_phase (Spell.cpp:3616-3619),
// on the delayed branch at delay start after SendSpellGo (Spell.cpp:3473-3479).
// The bag/slot are re-resolved from character_inventory at completion — C++
// operates on the live m_CastItem pointer, not the cast-start slot — and an
// item moved or gone since cast start spends nothing.
func (s *session) spendCastItemCharges(ctx context.Context, spell wotlk.Spell, castItemGUID uint64, castItemEntry uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	if castItemGUID == 0 || castItemIsSpellReagent(spell, castItemEntry) {
		return
	}
	var bag, slot int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT bag, slot FROM character_inventory WHERE guid = ? AND item = ? LIMIT 1`, s.playerGUID, int64(castItemGUID)).Scan(&bag, &slot); err != nil {
		return
	}
	s.takeCastItemSpellCharges(ctx, int64(castItemGUID), int64(castItemEntry), bag, uint8(slot))
}

// resolveItemUseSpell mirrors Player::CastItemUseSpell's spell selection
// (Player.cpp:8232-8303): the packet's spellId is ignored — the cast spell
// comes from the item template. The special learning case runs first
// (Spells[0] == 483/55884): the spell itself is cast, carrying the taught
// spell (Spells[1]) as its SPELLVALUE_BASE_POINT0 override, returned
// alongside for the effect-0 base points. Otherwise the first template
// spell with SpellTrigger == ITEM_SPELLTRIGGER_ON_USE casts. When no
// template spell qualifies, the enchantment leg runs: the first of the 12
// enchantment slots whose SpellItemEnchantment entry carries an
// ITEM_ENCHANTMENT_TYPE_USE_SPELL effect casts its EffectArg. Corrupt rows
// (spell missing from the DBC) are skipped like C++'s continue arms.
// Returns 0 when nothing casts.
func (s *session) resolveItemUseSpell(ctx context.Context, itemEntry uint32, dbItemGUID int64) (uint32, *int32) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return 0, nil
	}
	var spellIDs, spellTriggers [5]int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT spellid_1, spelltrigger_1, spellid_2, spelltrigger_2, spellid_3, spelltrigger_3, spellid_4, spelltrigger_4, spellid_5, spelltrigger_5 FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(
		&spellIDs[0], &spellTriggers[0], &spellIDs[1], &spellTriggers[1], &spellIDs[2], &spellTriggers[2],
		&spellIDs[3], &spellTriggers[3], &spellIDs[4], &spellTriggers[4]); err != nil {
		return 0, nil
	}
	// Special learning case (Player.cpp:8235-8250).
	if spellIDs[0] == 483 || spellIDs[0] == 55884 {
		bp := int32(spellIDs[1]) - 1
		return uint32(spellIDs[0]), &bp
	}
	spellKnown := func(id uint32) bool {
		_, found, err := s.server.Data.Spell(id)
		return err == nil && found
	}
	// Item spells cast at use (Player.cpp:8252-8276).
	for i := 0; i < 5; i++ {
		if spellIDs[i] > 0 && spellTriggers[i] == itemSpellTriggerOnUse && spellKnown(uint32(spellIDs[i])) {
			return uint32(spellIDs[i]), nil
		}
	}
	// Item enchantment spells cast at use (Player.cpp:8278-8302).
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, nil
	}
	var enchantments string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(enchantments, '') FROM item_instance WHERE guid = ? LIMIT 1`, dbItemGUID).Scan(&enchantments); err != nil {
		return 0, nil
	}
	fields := strings.Fields(enchantments)
	for slot := 0; slot < 12; slot++ {
		idx := slot * 3
		if idx >= len(fields) {
			break
		}
		enchantID, err := strconv.ParseUint(fields[idx], 10, 32)
		if err != nil || enchantID == 0 {
			continue
		}
		enchant, found, err := s.server.Data.SpellItemEnchantment(uint32(enchantID))
		if err != nil || !found {
			continue
		}
		for e := range enchant.Effects {
			if enchant.Effects[e] == itemEnchantTypeUseSpell && enchant.EffectArg[e] != 0 && spellKnown(enchant.EffectArg[e]) {
				return enchant.EffectArg[e], nil
			}
		}
	}
	return 0, nil
}

// applyItemEquipSpells mirrors Player::ApplyItemEquipSpell plus the spell half
// of Player::ApplyEquipSpell (Player.cpp:7960-8022): equipping an item casts
// each template spell with ITEM_SPELLTRIGGER_ON_EQUIP as a triggered cast
// with the item as cast item (CastSpellExtraArgs(Item*), SpellDefines.h:165);
// removing the item strips exactly those auras via RemoveAurasDueToItemSpell,
// walking ALL template spells rather than just the ON_EQUIP ones ("un-apply
// all spells, not only at-equipped", Player.cpp:8019). The apply leg's
// CheckShapeshift gate (Player.cpp:7993-7996) rides checkShapeshiftCast; the
// form_change re-evaluation arm (Player.cpp:7998-8004, UpdateEquipSpellsAt-
// FormChange) has no Go hook yet. The entry-ID binding (GetSpellInfo lookup,
// Player.cpp:7977-7980) skips corrupt rows like the C++ continue arms.
func (s *session) applyItemEquipSpells(ctx context.Context, dbItemGUID int64, itemEntry uint32, apply bool) {
	if s == nil || s.player == nil || dbItemGUID == 0 || itemEntry == 0 {
		return
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return
	}
	var spellIDs, spellTriggers [5]int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT spellid_1, spelltrigger_1, spellid_2, spelltrigger_2, spellid_3, spelltrigger_3, spellid_4, spelltrigger_4, spellid_5, spelltrigger_5 FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(
		&spellIDs[0], &spellTriggers[0], &spellIDs[1], &spellTriggers[1], &spellIDs[2], &spellTriggers[2],
		&spellIDs[3], &spellTriggers[3], &spellIDs[4], &spellTriggers[4]); err != nil {
		return
	}
	for i := 0; i < maxItemProtoSpells; i++ {
		if spellIDs[i] <= 0 {
			continue
		}
		// wrong triggering type (Player.cpp:7971-7972): only ON_EQUIP
		// spells cast on equip; the removal leg takes every template spell.
		if apply && spellTriggers[i] != itemSpellTriggerOnEquip {
			continue
		}
		spell, found, err := s.server.Data.Spell(uint32(spellIDs[i]))
		if err != nil || !found {
			continue
		}
		if apply {
			// Cannot be used in this stance/form (Player.cpp:7993-7996).
			if s.checkShapeshiftCast(spell) != 0 {
				continue
			}
			s.castSpellDirectWithItem(ctx, uint32(spellIDs[i]), s.playerGUID, uint64(dbItemGUID))
		} else {
			s.removeAurasDueToItemSpell(uint32(spellIDs[i]), uint64(dbItemGUID))
		}
	}
}

// applyLoginItemEquipSpells mirrors the equip-spell leg of Player::_ApplyAllItemMods
// (Player.cpp:8385-8401): at login, after the DB auras are loaded and before
// MSG_SET_DUNGEON_DIFFICULTY/SMSG_LOGIN_VERIFY_WORLD (HandlePlayerLogin order:
// LoadFromDB -> SendDungeonDifficulty -> LoginVerifyWorld, CharacterHandler.cpp),
// every equipped item re-applies its ON_EQUIP spells. The cast merges with the
// DB-loaded aura (Aura::TryRefreshStackOrCreate refresh arm), which is how login
// resets equip-aura durations to full. The IsBroken skip rides the
// MaxDurability/durability check; Unit::CanUseAttackType is vacuous at login (a
// freshly loaded player never carries UNIT_FLAG_DISARMED/UNIT_FLAG2_DISARM_*).
// Item sets (AddItemsSetItem) and enchantments (ApplyEnchantment) are separate
// _ApplyAllItemMods legs, not part of this unit.
func (s *session) applyLoginItemEquipSpells(ctx context.Context) {
	if s == nil || s.player == nil {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	type equippedItem struct {
		guid, entry, durability int64
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
		`SELECT ii.guid, ii.itemEntry, COALESCE(ii.durability, 0)
		FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < ? ORDER BY ci.slot`,
		s.playerGUID, int64(inventorySlotBagEnd))
	if err != nil {
		return
	}
	var items []equippedItem
	for rows.Next() {
		var it equippedItem
		if rows.Scan(&it.guid, &it.entry, &it.durability) != nil || it.guid == 0 || it.entry <= 0 {
			continue
		}
		items = append(items, it)
	}
	rows.Close()
	for _, it := range items {
		// Player.cpp:8392-8393: broken items shed their equip spells at login.
		if tmpl, ok := s.server.getItemStoreTemplateInfo(ctx, uint32(it.entry)); ok && tmpl.MaxDurability > 0 && it.durability == 0 {
			continue
		}
		s.applyItemEquipSpells(ctx, it.guid, uint32(it.entry), true)
	}
}

// updateItemEquipSpellsOnMove mirrors the _ApplyItemMods(true/false) pair
// Player::SwapItem/StoreItem run around equipment transitions
// (Player::_ApplyItemMods, Player.cpp:12419): an item leaving an equipment
// slot (bag 0, slot < 19) sheds its equip spells, an item entering one gains
// them. Called after the coordinate swap commits so the remove leg cannot
// see a stale position.
func (s *session) updateItemEquipSpellsOnMove(ctx context.Context, itemGUID, fromBag, fromSlot, toBag, toSlot int64) {
	if s == nil || itemGUID == 0 || (fromBag == toBag && fromSlot == toSlot) {
		return
	}
	wasEquipped := fromBag == 0 && fromSlot < int64(equipSlotEnd)
	isEquipped := toBag == 0 && toSlot < int64(equipSlotEnd)
	if !wasEquipped && !isEquipped {
		return
	}
	var itemEntry int64
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ? LIMIT 1", itemGUID).Scan(&itemEntry)
	if itemEntry <= 0 {
		return
	}
	// C++ SwapItem/StoreItem order: the old position's mods leave before the
	// new position's apply.
	if wasEquipped {
		s.applyItemEquipSpells(ctx, itemGUID, uint32(itemEntry), false)
	}
	if isEquipped {
		s.applyItemEquipSpells(ctx, itemGUID, uint32(itemEntry), true)
	}
}

// cast-item-as-reagent block (Spell.cpp:5066-5076): the reagent count grows
// by one when the cast item is expendable (negative template SpellCharges
// on some spell slot) and this use spends its last charge (abs(charges) <
// 2) — the cast item is used up and does not count as a reagent.
func (s *session) castItemReagentTakesExtra(ctx context.Context, dbItemGUID uint64, itemEntry uint32) bool {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	var tplCharges [5]int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT spellcharges_1, spellcharges_2, spellcharges_3, spellcharges_4, spellcharges_5
		FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(
		&tplCharges[0], &tplCharges[1], &tplCharges[2], &tplCharges[3], &tplCharges[4]); err != nil {
		return false
	}
	var defaults [5]int32
	for i := range defaults {
		defaults[i] = int32(tplCharges[i])
	}
	var rawCharges string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(charges, '') FROM item_instance WHERE guid = ? LIMIT 1`, dbItemGUID).Scan(&rawCharges); err != nil {
		return false
	}
	charges := parseItemSpellCharges(rawCharges, defaults)
	for i := range tplCharges {
		if tplCharges[i] >= 0 {
			continue
		}
		c := int32(charges[i])
		if c < 0 {
			c = -c
		}
		if c < 2 {
			return true
		}
	}
	return false
}

// checkCastItemCharges mirrors the cast-item charge arm of Spell::CheckItems
// (Spell.cpp:6690-6696): any template spell slot carrying charges
// (SpellCharges != 0) whose instance charges read 0 means the item is spent.
// Returns false when the cast must be rejected. The HasItemCount and
// null-proto arms have no bridge here: the item was resolved from the
// player's own inventory slot above, and a missing item_template row means
// the DB is incomplete (fail-open per this file's convention).
func (s *session) checkCastItemCharges(ctx context.Context, dbItemGUID int64, itemEntry int64) bool {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	var tplCharges [5]int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT spellcharges_1, spellcharges_2, spellcharges_3, spellcharges_4, spellcharges_5
		FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(
		&tplCharges[0], &tplCharges[1], &tplCharges[2], &tplCharges[3], &tplCharges[4]); err != nil {
		return true
	}
	var defaults [5]int32
	for i := range defaults {
		defaults[i] = int32(tplCharges[i])
	}
	var rawCharges string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(charges, '') FROM item_instance WHERE guid = ? LIMIT 1`, dbItemGUID).Scan(&rawCharges); err != nil {
		return true
	}
	charges := parseItemSpellCharges(rawCharges, defaults)
	for i := 0; i < 5; i++ {
		if tplCharges[i] != 0 && int32(charges[i]) == 0 {
			return false
		}
	}
	return true
}

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
	spellID, err := r.ReadU32() // packet spellId: read for protocol, ignored for casting (Player::CastItemUseSpell resolves server-side)
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
	if glyphIndex >= 6 {
		// SpellHandler.cpp:95-99 — glyphIndex >= MAX_GLYPH_SLOT_INDEX (6) is a
		// hard EQUIP_ERR_ITEM_NOT_FOUND, not a silent ignore. C++ reads the
		// whole payload first, so the check sits after the castFlags read.
		s.sendEquipError(equipErrItemNotFound, itemGUID)
		return true
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
	var itemInstanceFlags int64
	err = cdb.QueryRowContext(ctx, `SELECT ci.item, ii.itemEntry, ii.count, ii.flags
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = ? AND ci.slot = ? LIMIT 1`, s.playerGUID, bagKey, slot).Scan(&dbItemGUID, &itemEntry, &count, &itemInstanceFlags)
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

	// Item-template gates (SpellHandler.cpp:127-172). When the template row is
	// missing the arms are skipped — C++ would already have errored at the
	// !proto check above, so a missing row here means the DB is incomplete,
	// not a bypass.
	var itemClass, itemSubClass, itemInvType, itemTplFlags, itemTplFlags2, bonding, reqLevel int64
	var allowableClass, allowableRace, reqSkill, reqSkillRank, reqSpell, holidayID, quality, reqRepFaction, reqRepRank int64
	var itemSpellIDs [5]int64
	tplOK := false
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		tplOK = s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT Class, SubClass, InventoryType, Flags, Flags2, Bonding, RequiredLevel, AllowableClass, AllowableRace, RequiredSkill, RequiredSkillRank, RequiredSpell, HolidayId, Quality, RequiredReputationFaction, RequiredReputationRank, spellid_1, spellid_2, spellid_3, spellid_4, spellid_5 FROM item_template WHERE entry = ? LIMIT 1`, itemEntry).Scan(&itemClass, &itemSubClass, &itemInvType, &itemTplFlags, &itemTplFlags2, &bonding, &reqLevel, &allowableClass, &allowableRace, &reqSkill, &reqSkillRank, &reqSpell, &holidayID, &quality, &reqRepFaction, &reqRepRank, &itemSpellIDs[0], &itemSpellIDs[1], &itemSpellIDs[2], &itemSpellIDs[3], &itemSpellIDs[4]) == nil
	}
	if tplOK {
		// SpellHandler.cpp:127-132 — item classes with an equip slot can only
		// be used from equipped state. Go models equipped items at bag 0,
		// slots 0..18 (the client's bagIndex 255 arm resolves to bagKey 0).
		if itemInvType != 0 /* INVTYPE_NON_EQUIP (ItemTemplate.h:261) */ && !(bagKey == 0 && slot < 19) {
			s.sendEquipError(equipErrItemNotFound, fullItemGUID)
			return true
		}
		// Player::CanUseItem (Player.cpp:11936) — the level gate.
		if s.player.Level < uint8(reqLevel) {
			s.sendEquipError(equipErrCantEquipLevelI, fullItemGUID)
			return true
		}
		// SpellHandler.cpp:135 — Player::CanUseItem (Player.cpp:11859) in
		// result form; the Denveous-marker AX1/AX2 bypasses plus the
		// faction/class/race/skill/spell/holiday/learning/reputation arms.
		if msg := s.canUseItemResult(ctx, uint32(itemClass), uint32(itemSubClass), uint32(itemTplFlags2), allowableClass, allowableRace, uint32(reqSkill), uint32(reqSkillRank), uint32(reqSpell), uint32(holidayID), uint32(quality), uint32(reqRepFaction), uint32(reqRepRank), itemSpellIDs); msg != equipErrOk {
			s.sendEquipError(msg, fullItemGUID)
			return true
		}
		// SpellHandler.cpp:137-149 — arena restrictions. InArena's Go proxy is
		// an active arena queue entry past the wait queue (arena_team.go
		// documents the established/in-progress leg mapping).
		inArena := false
		for i := range s.bgQueues {
			if e := &s.bgQueues[i]; e.Active && e.IsArena && e.Status == BGStatusInProgress {
				inArena = true
				break
			}
		}
		// ITEM_FLAG_IGNORE_DEFAULT_ARENA_RESTRICTIONS 0x200000 (ItemTemplate.h:173),
		// ITEM_FLAG_NOT_USEABLE_IN_ARENA 0x4000000 (ItemTemplate.h:178).
		if inArena && ((itemClass == itemClassConsumable && itemTplFlags&0x200000 == 0) || itemTplFlags&0x4000000 != 0) {
			s.sendEquipError(equipErrNotDuringArenaMatch, fullItemGUID)
			return true
		}
		// SpellHandler.cpp:151-162 — in-combat items whose any spell cannot be
		// used in combat refuse with EQUIP_ERR_NOT_IN_COMBAT.
		if s.isInCombat() && s.server != nil && s.server.Data != nil {
			for _, tplSpellID := range itemSpellIDs {
				if tplSpellID == 0 {
					continue
				}
				if spell, found, sErr := s.server.Data.Spell(uint32(tplSpellID)); sErr == nil && found && spell.Attributes&spellAttr0CantUsedInCombat == 0 {
					s.sendEquipError(equipErrNotInCombat, fullItemGUID)
					return true
				}
			}
		}
		// SpellHandler.cpp:164-172 — BIND_WHEN_PICKED_UP (1), BIND_WHEN_USE
		// (3), BIND_QUEST_ITEM (4) (ItemTemplate.h:99-102) soulbind on first
		// use. C++ does this before the targets read; Go does it before the
		// cooldown/cast arms, same effective order.
		if (bonding == 1 || bonding == 3 || bonding == 4) && itemInstanceFlags&int64(itemInstanceFlagSoulbound) == 0 {
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET flags = flags | ? WHERE guid = ?", uint32(itemInstanceFlagSoulbound), rawItemGUID)
		}
	}

	// Spell::CheckCast cooldown block (Spell.cpp:5187-5221) lives on the
	// spell-found path below (it needs the spell's category), after the
	// Eluna hooks — matching C++ SpellHandler.cpp order (OnItemUse at
	// SpellHandler.cpp:176 precedes the cast and its CheckCast).

	// Eluna::OnUse (ItemHooks.cpp:55) fires ITEM_EVENT_ON_USE (2) via
	// ScriptMgr::OnItemUse (SpellHandler.cpp:176) after the item validation
	// and bonding gates, then the item gossip hello hook
	// (GOSSIP_EVENT_ON_HELLO, GossipHooks.cpp:81) whenever the item still
	// exists — the hello fires even when the use hook already cancelled,
	// exactly like Eluna::OnUse's OnItemUse-then-OnItemGossip order. A Lua
	// false return from either blocks the cast; C++ then sends the raw
	// EQUIP_ERR_NONE failure packet (the stuck-item hack at the tail of
	// Eluna::OnUse) and skips the cast.
	cancelUse := s.fireItemUseHook(ctx, rawItemGUID)
	if s.fireItemGossipHelloHook(ctx, rawItemGUID) {
		cancelUse = true
	}
	if cancelUse {
		s.sendEquipError(equipErrNone, fullItemGUID)
		return true
	}

	// Cast spell. Player::CastItemUseSpell (Player.cpp:8232-8303) resolves the
	// cast spell server-side from the item template — the packet's spellId is
	// read but never used for casting. The learning case returns its
	// SPELLVALUE_BASE_POINT0 override (the taught spell) alongside.
	spellID, learnBasePoint := s.resolveItemUseSpell(ctx, uint32(itemEntry), dbItemGUID)
	if spellID != 0 && s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			if learnBasePoint != nil {
				// Player::CastItemUseSpell (Player.cpp:8245): the learning
				// spell (483/55884) carries the taught spell in
				// SPELLVALUE_BASE_POINT0. Go's EffectLearnSpell reads
				// eff.BasePoints+1, so the override lands on this cast's
				// spell copy (Store.Spell returns a value).
				spell.Effects[0].BasePoints = *learnBasePoint
			}
			// Spell::CheckCast cooldown block (Spell.cpp:5187-5221) on the
			// m_CastItem path: SpellHistory::IsReady/HasCooldown
			// (SpellHistory.cpp:190-200/473-487) fails with
			// SPELL_FAILED_NOT_READY when the spell or its category is
			// cooling down. The category arm reads the category entry
			// alone — SpellHistory::Update (SpellHistory.cpp:141-155)
			// erases category and spell entries independently, so the
			// spell arm's End must not gate it. Runs after the Eluna
			// hooks, matching C++ SpellHandler.cpp order (OnItemUse at
			// SpellHandler.cpp:176 precedes the cast and its CheckCast).
			nowUnix := time.Now().Unix()
			for _, cd := range s.player.Cooldowns {
				if cd.Spell == spellID && cd.End > nowUnix {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
					s.debug("item cast rejected", "account", s.accountName, "spell", spellID, "reason", "spell on cooldown")
					return true
				}
			}
			if categoryID := spell.Category; categoryID != 0 {
				for _, cd := range s.player.Cooldowns {
					if cd.Category == categoryID && cd.CategoryEnd > nowUnix {
						_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
						s.debug("item cast rejected", "account", s.accountName, "spell", spellID, "reason", "category cooldown active", "category", categoryID)
						return true
					}
				}
			}
			// Spell::CheckCast potion leg (Spell.cpp:5195-5198): with a
			// banked m_lastPotionId (set at SendSpellCooldown above), a
			// further potion (Item::IsPotion, Item.h:177) or
			// cooldown-started-on-event spell
			// (SpellInfo::IsCooldownStartedOnEvent, SpellInfo.cpp:1159)
			// item cast fails SPELL_FAILED_NOT_READY. C++ runs this in
			// Spell::CheckCast with m_CastItem set; Go's item casts never
			// pass through handleCastSpell's gates, so the arm lives on
			// the m_CastItem path here. The !IsIgnoringCooldowns() arm is
			// vacuous: handleUseItem serves only client CMSG_USE_ITEM
			// casts, never triggered ones.
			if s.lastPotionId != 0 && (s.server.isPotionItem(ctx, uint32(itemEntry)) || s.server.spellIsCooldownStartedOnEvent(spell)) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
				return true
			}
			// Cast-item charge arm of Spell::CheckItems (Spell.cpp:6690-6696):
			// a template spell slot carrying charges whose instance charges
			// read 0 rejects the cast with SPELL_FAILED_NO_CHARGES_REMAIN
			// before SMSG_SPELL_START. Runs ahead of the consumable arm,
			// matching C++ CheckItems relative order.
			if !s.checkCastItemCharges(ctx, dbItemGUID, itemEntry) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNoChargesRemain), true)
				s.debug("item cast rejected", "account", s.accountName, "spell", spellID, "reason", "no charges remain")
				return true
			}
			// Consumable full-health/full-power arm of Spell::CheckItems
			// (Spell.cpp:6704-6751): when the cast item is a consumable and
			// the spell targets a unit, effects are tried in order — the
			// first HEAL effect against a non-full-health target or
			// ENERGIZE effect against a non-full power target clears the
			// pending failure (the Rejuvenation Potion pattern), while an
			// effect that finds a full target only sets the failure reason
			// and continues. TARGET_UNIT_PET effects are skipped: there the
			// C++ target is the caster, not the pet. Only the
			// caster-as-target leg is bridged — other unit targets have no
			// Go model to read health/power from. Runs before SMSG_SPELL_START,
			// matching C++ CheckCast order (CheckItems precedes prepare's START).
			if tplOK && itemClass == itemClassConsumable && target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 && target.UnitGUID == s.playerGUID {
				failReason := uint8(0)
				for _, eff := range spell.Effects {
					if eff.ImplicitTargetA == spellImplicitTargetUnitPet {
						continue
					}
					if eff.Effect == spellEffectHeal {
						if s.player.Health >= s.player.MaxHealth {
							failReason = spellFailedAlreadyAtFullHealth
							continue
						}
						failReason = 0
						break
					}
					if eff.Effect == spellEffectEnergize {
						if eff.MiscValue < 0 || eff.MiscValue >= 7 /* MAX_POWERS (SharedDefines.h:302) */ {
							failReason = spellFailedAlreadyAtFullPower
							continue
						}
						power := int(eff.MiscValue)
						if s.player.Powers[power] >= s.player.MaxPowers[power] {
							failReason = spellFailedAlreadyAtFullPower
							continue
						}
						failReason = 0
						break
					}
				}
				if failReason != 0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, failReason), true)
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "consumable target at full health/power", "failReason", failReason)
					return true
				}
			}
			// Spell::prepare disabled-spell gate (Spell.cpp:3074-3080) via
			// DisableMgr::IsDisabledFor(DISABLE_TYPE_SPELL, id, caster):
			// item casts are TRIGGERED_NONE prepares
			// (Player::CastItemUseSpell, Player.cpp:8232), so the gate
			// applies here too, ahead of the in-progress gate.
			if s.spellDisabledForCaster(ctx, spellID) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedSpellUnavailable), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "spell disabled", "failure", spellFailedSpellUnavailable)
				return true
			}
			// Spell::prepare server-side gate (Spell.cpp:3082-3087): a
			// cast-bar cast already in progress blocks the item cast with
			// SPELL_FAILED_SPELL_IN_PROGRESS (105). The TRIGGERED_NONE
			// item prepare carries no TRIGGERED_IGNORE_CAST_IN_PROGRESS,
			// so the gate is live on this path; the auto-shot exception
			// rides the same helper as the client cast path.
			if s.genericCastInProgress() && !s.autoShotNonBlockingCast(spellID) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, 105), true) // SPELL_FAILED_SPELL_IN_PROGRESS = 105
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "another spell cast is in progress")
				return true
			}
			// Spell::CheckCast GCD arm (Spell.cpp:5227-5228): the
			// TRIGGERED_NONE item prepare runs the strict CheckCast, so an
			// active global cooldown fails the cast with
			// SPELL_FAILED_NOT_READY (SPELL_FAILED_DONT_REPORT for
			// DISABLED_WHILE_ACTIVE spells), ahead of SMSG_SPELL_START —
			// matching C++ CheckCast-before-START order.
			if s.isGCDActive(spell) {
				reason := spellFailedNotReady
				if spell.Attributes&spellAttr0DisabledWhileActive != 0 {
					reason = spellFailedDontReport
				}
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castCount, spellID, reason), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "global cooldown active")
				return true
			}
			castTime := uint32(0)
			if value, ok, castErr := s.server.Data.SpellCastTime(spell.CastingTimeIndex); castErr == nil && ok && value > 0 {
				castTime = uint32(value)
			}
			// Spell::SendSpellStart (Spell.cpp:4253-4259): item casts carry the
			// cast item's GUID as CasterGUID (0x4000 high bits on the wire;
			// castItemGUID is the raw instance guid) with the player as
			// CasterUnit; flags/powers follow the same arms as the client path.
			itemGUID := rawItemGUID | (uint64(0x4000) << 48)
			startFlags := spellStartCastFlags(spell)
			if err := s.sendSpellStart(protocol.BuildSpellStartWithPower(itemGUID, s.playerGUID, castCount, spellID, startFlags, castTime, target, s.spellStartRemainingPower(spell, startFlags), s.spellStartAmmoData(ctx, startFlags))); err != nil {
				return false
			}
			// Spell::prepare (Spell.cpp:3188-3196) sends SMSG_SPELL_START
			// before TriggerGlobalCooldown: item casts are TRIGGERED_NONE
			// prepares (Player::CastItemUseSpell, Player.cpp:8232), so they
			// set the category GCD exactly like client-initiated casts.
			s.triggerGlobalCooldown(spell)
			if castTime > 0 {
				time.AfterFunc(time.Duration(castTime)*time.Millisecond, func() {
					s.finishSpellCast(context.Background(), castCount, spellID, spell, target, rawItemGUID, uint32(itemEntry), nil)
				})
			} else {
				s.finishSpellCast(ctx, castCount, spellID, spell, target, rawItemGUID, uint32(itemEntry), nil)
			}
		}
	}

	// Spell::TakeCastItem (Spell.cpp:4711-4781) runs at cast COMPLETION,
	// not here: on the immediate branch C++ spends the charge after
	// _handle_finish_phase (Spell.cpp:3616-3619), and on the delayed branch
	// at delay start after SendSpellGo (Spell.cpp:3473-3479) — an
	// interrupted or failed cast never reaches either point. The spend
	// lives in finishSpellCast's completion sites (spells.go) via
	// spendCastItemCharges; the CheckItems no-charges pre-check above stays
	// at prepare, matching C++ CheckCast order.
	// There is no C++ counterpart to a cast-start consumable decrement:
	// charge-less consumables are consumed through the reagent arm
	// (their spell lists the item itself as a reagent) in takeSpellReagents
	// at completion.
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
	if err != nil || srcGUID == 0 {
		return true
	}
	// Player::SplitItem (Player.cpp:13047): an item whose loot was generated
	// (Item::m_lootGenerated) cannot be split — the split would strand the
	// stored remainder on the wrong stack. Go tracks generation per
	// container instance GUID (generatedContainerLoot).
	if s.server.containerLootGenerated(uint64(srcGUID)) {
		s.sendEquipError(equipErrCouldntSplitItems, uint64(srcGUID))
		return true
	}
	if count >= uint32(srcCount) {
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
		newGUID := int64(s.server.generateItemGUID())
		if newGUID <= 0 {
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
	// ExtraItemGUIDs holds additional new stacks beyond the primary ItemGUID
	// when a grant was split across multiple stacks (C++ CanStoreNewItem
	// multi-position placement).
	ExtraItemGUIDs []uint64
	// StackFillGUID/StackFillAmount record a merge into a pre-existing partial
	// pile, so rollback can undo it.
	StackFillGUID   uint64
	StackFillAmount uint32
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

// freeSlotRef is one free inventory position: the character_inventory bag key
// (0 = backpack, else the bag item's guid), the client-visible bag number
// (255 = backpack, else the 19-22 equip slot) and the slot within the bag.
type freeSlotRef struct {
	bagKey    int64
	clientBag uint8
	slot      uint8
}

// collectFreeInventorySlots gathers up to need free inventory positions in
// C++ CanStoreNewItem preference order: backpack slots first (lowest first),
// then equipped bags. It returns fewer than need when the inventory cannot
// hold that many new stacks.
func (s *session) collectFreeInventorySlots(ctx context.Context, playerGUID uint64, need uint32) []freeSlotRef {
	out := make([]freeSlotRef, 0, need)
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return out
	}
	cdb := s.server.CharactersStore.DB
	usedIn := func(bagKey int64) map[int64]struct{} {
		used := make(map[int64]struct{})
		rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = ?", playerGUID, bagKey)
		if err != nil {
			return used
		}
		defer rows.Close()
		for rows.Next() {
			var slot int64
			if rows.Scan(&slot) == nil {
				used[slot] = struct{}{}
			}
		}
		return used
	}
	used := usedIn(0)
	for slot := int64(invSlotItemStart); slot < int64(invSlotItemEnd) && uint32(len(out)) < need; slot++ {
		if _, ok := used[slot]; !ok {
			out = append(out, freeSlotRef{bagKey: 0, clientBag: 255, slot: uint8(slot)})
		}
	}
	for _, b := range s.getEquippedBags(ctx, playerGUID) {
		if uint32(len(out)) >= need {
			break
		}
		if b.slots <= 0 {
			continue
		}
		usedBag := usedIn(b.guid)
		for slot := int64(0); slot < b.slots && uint32(len(out)) < need; slot++ {
			if _, ok := usedBag[slot]; !ok {
				out = append(out, freeSlotRef{bagKey: b.guid, clientBag: b.slot, slot: uint8(slot)})
			}
		}
	}
	return out
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
	if count == 0 {
		count = 1
	}
	result, err := s.storeOrStackItemCore(ctx, playerGUID, itemEntry, count)
	if err == nil && result != nil && s.player != nil && playerGUID == s.playerGUID {
		s.adjustQuestItemCount(ctx, itemEntry, count, true)
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

	// Player::CanStoreNewItem: stackable grants merge into an existing partial
	// pile first, then split across as many new maxStack-bounded stacks as
	// needed — never a single over-full row.
	maxStack := uint32(1)
	var maxDurability int64 = 100
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var stackable int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(stackable, 1) FROM item_template WHERE entry = ?", itemEntry).Scan(&stackable); err == nil && stackable > 1 {
			maxStack = uint32(stackable)
		}
		var md int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MaxDurability, 0) FROM item_template WHERE entry = ?", itemEntry).Scan(&md); err == nil && md > 0 {
			maxDurability = md
		}
	}

	// 1. Merge into an existing partial stack first.
	bagKey, clientBag, slot, itemGUID, curCount, pileMax, canStack := s.findStackableInventorySlot(ctx, playerGUID, itemEntry)
	if canStack && curCount < pileMax {
		space := pileMax - curCount
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
				BagKey:          bagKey,
				ClientBag:       clientBag,
				Slot:            slot,
				ItemGUID:        itemGUID,
				IsStack:         true,
				NewCount:        newCount,
				InventoryCount:  uint32(totalCount),
				StackFillGUID:   itemGUID,
				StackFillAmount: count,
			}, nil
		}
	}

	// 2. Split the remainder into maxStack-bounded stacks. Feasibility is
	// checked before mutating anything, like CanStoreNewItem.
	var fillGUID uint64
	var fillAmount uint32
	remaining := count
	if canStack && curCount < pileMax {
		fillAmount = pileMax - curCount
		fillGUID = itemGUID
		remaining -= fillAmount
	}
	var newStacks []uint32
	for r := remaining; r > 0; {
		take := maxStack
		if take > r {
			take = r
		}
		newStacks = append(newStacks, take)
		r -= take
	}
	slots := s.collectFreeInventorySlots(ctx, playerGUID, uint32(len(newStacks)))
	if uint32(len(slots)) < uint32(len(newStacks)) {
		return nil, errInventoryFull
	}

	tx, err := cdb.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if fillGUID != 0 {
		if _, err = tx.ExecContext(ctx, "UPDATE item_instance SET count = count + ? WHERE guid = ?", fillAmount, fillGUID); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	var baseGUID int64
	if reserved, ok := s.server.generateItemGUIDRange(uint64(len(newStacks))); ok {
		baseGUID = int64(reserved)
	} else {
		// Counter overflow: fall back to the legacy MAX(guid) read so the
		// stacks still land, matching the old behavior exactly.
		_ = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) FROM item_instance").Scan(&baseGUID)
	}
	if baseGUID < 0 {
		baseGUID = 0
	}
	guids := make([]uint64, 0, len(newStacks))
	for i, stackCount := range newStacks {
		nextGUID := uint64(baseGUID) + uint64(i) + 1
		if _, err = tx.ExecContext(ctx, `INSERT INTO item_instance
			(guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text)
			VALUES (?, ?, ?, 0, ?, 0, '', 0, '', 0, ?, 0, '')`,
			nextGUID, itemEntry, playerGUID, stackCount, maxDurability); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO character_inventory
			(guid, bag, slot, item) VALUES (?, ?, ?, ?)`,
			playerGUID, slots[i].bagKey, slots[i].slot, nextGUID); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		guids = append(guids, nextGUID)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}

	var totalCount int64
	_ = cdb.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, itemEntry).Scan(&totalCount)

	res := &inventoryStoreResult{
		BagKey:          slots[0].bagKey,
		ClientBag:       slots[0].clientBag,
		Slot:            slots[0].slot,
		ItemGUID:        guids[0],
		IsStack:         false,
		NewCount:        newStacks[0],
		InventoryCount:  uint32(totalCount),
		StackFillGUID:   fillGUID,
		StackFillAmount: fillAmount,
	}
	if len(guids) > 1 {
		res.ExtraItemGUIDs = guids[1:]
	}
	return res, nil
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

	// SpellHandler.cpp:221-229 — only items flagged lootable (ITEM_FLAG_HAS_LOOT
	// 0x4, ItemTemplate.h:154) or wrapped (a character_gifts row, the async
	// HandleOpenWrappedItemCallback arm) can be opened at all.
	wrapped := false
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var giftCount int64
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_gifts WHERE item_guid = ?", itemGUID).Scan(&giftCount)
		wrapped = giftCount > 0
	}
	var tplFlags, lockID int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Flags, LockID FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&tplFlags, &lockID)
	}
	if tplFlags&0x4 == 0 && !wrapped {
		s.sendEquipError(equipErrCantDoRightNow, uint64(itemGUID))
		return true
	}
	// SpellHandler.cpp:231-250 — locked items refuse with EQUIP_ERR_ITEM_LOCKED.
	// Go has no Lock.dbc model and no per-item unlocked state, so any LockID
	// refuses; the unknown-lock vs not-unlocked nuance is a documented delta.
	if lockID != 0 {
		s.sendEquipError(equipErrItemLocked, uint64(itemGUID))
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

		// Container opening (Player::SendLoot item arm, Player.cpp:8665-8710):
		// OWNER_PERMISSION, per-item stored loot, money rolled from the item
		// template BEFORE the template fill (Player.cpp:8699), and the
		// default arm's personal FillLoot(item_loot_template) — the
		// disenchant/prospect/mill arms never apply here (Go has no
		// EffectDisenchant/EffectProspecting/EffectMilling handlers).
		var lootSource *sql.DB
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			lootSource = s.server.WorldStore.DB
		} else {
			lootSource = cdb
		}

		if lootSource != nil {
			loot := &activeLootState{
				TargetGUID: uint64(itemGUID),
				MapID:      s.player.Map,
				InstanceID: s.player.InstanceID,
				LootType:   1,
				Items:      make(map[uint8]lootItem),
				// Loot::FillLoot personal arm (Loot.cpp:214-232): the item
				// fill passes personal=true, so quest rows belong to the
				// opener alone (viewerQuestLootList pins on this).
				QuestPersonalGUID: s.playerGUID,
				// The window's target is the opener's own item instance:
				// takes skip the creature gates and the release runs the
				// DoLootRelease item arm (LootHandler.cpp:60-66/321-349).
				LootItemGUID: uint64(itemGUID),
			}
			// Player.cpp:8699: generateMoneyLoot(MinMoneyLoot, MaxMoneyLoot)
			// runs before the template fill; the fill's noEmptyError arm is
			// (gold != 0), so money alone still opens a window.
			// Player.cpp:8683: a container that already has persisted loot
			// reopens it from the store (LoadStoredLoot) instead of
			// rolling fresh.
			if s.server.applyStoredContainerLoot(ctx, s, uint64(itemGUID), loot) {
				// Stored remainder reopened; no fresh roll, no re-store.
			} else {
				var minMoney, maxMoney int64
				_ = lootSource.QueryRowContext(ctx, "SELECT MinMoneyLoot, MaxMoneyLoot FROM item_template WHERE entry = ? LIMIT 1", itemEntry).Scan(&minMoney, &maxMoney)
				loot.Money = generateMoneyLootValue(minMoney, maxMoney)
				s.server.fillLootTemplate(ctx, lootSource, "item_loot_template", int64(itemEntry), lootModeDefault, loot)
				// FillNotNormalLootFor (Loot.cpp:246-266) auto-stores
				// currency-token rows straight into the opener's bags.
				s.server.autoStoreLootCurrencyTokens(ctx, loot, s)
				// Player.cpp:8703-8706: the default (container) arm persists
				// the rolled loot so a later reopen yields the same
				// remainder. Tokens are already gone from the fill above.
				s.server.storeNewContainerLoot(ctx, uint64(itemGUID), loot, s)
			}
			// Player.cpp:8683-8685: the item's loot is generated from here on
			// (Item::m_lootGenerated), even for an empty roll — the
			// container's stack can no longer be split (Player::SplitItem,
			// Player.cpp:13047).
			s.server.markContainerLootGenerated(uint64(itemGUID))
			loot.addViewer(s)

			if len(loot.Items) > 0 || len(loot.QuestItems) > 0 || loot.Money > 0 {
				s.server.lootMu.Lock()
				if s.server.creatureLoot == nil {
					s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
				}
				s.server.creatureLoot[loot.objectKey()] = loot
				s.server.lootMu.Unlock()
				s.activeLoot = loot
				return s.sendLootResponse(ctx, loot) == nil
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
		s.adjustQuestItemCount(ctx, uint32(giftEntry), 1, false)

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
		s.adjustQuestItemCount(ctx, uint32(targetEntry), 1, false)
		s.adjustQuestItemCount(ctx, wrappedEntry, 1, true)
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
	npcGUID, _ := r.ReadU64()
	itemGUID, _ := r.ReadU64()
	guildBank, _ := r.ReadU8()

	// NPCHandler.cpp:717-731 (HandleRepairItemOpcode): repairs require an
	// interactable repair NPC (UNIT_NPC_FLAG_REPAIR), otherwise silent drop.
	if !s.canInteractWithNPC(ctx, npcGUID, uint64(unitNPCFlagRepair)) {
		return true
	}

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
			repaired := false
			if guildBank != 0 {
				// Player::DurabilityRepair (Player.cpp:5084): guild-bank
				// repairs draw from the guild bank; failure leaves the item
				// unrepaired — no personal-money fallback.
				if s.player.GuildID != 0 && s.guildBankWithdrawMoneyForRepair(ctx, s.player.GuildID, cost) {
					repaired = true
				}
			} else if s.player.Money >= cost {
				s.player.Money -= cost
				_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
				repaired = true
			}
			if repaired {
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
				if guildBank != 0 {
					// Player::DurabilityRepairAll -> Player::DurabilityRepair
					// (Player.cpp:5084): each item draws from the guild bank
					// independently; items the guild cannot cover stay damaged.
					repaired := false
					if s.player.GuildID != 0 {
						for _, item := range toRepair {
							if s.guildBankWithdrawMoneyForRepair(ctx, s.player.GuildID, item.cost) {
								_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", item.maxD, item.guid)
								repaired = true
							}
						}
					}
					if repaired {
						_ = s.sendInventoryItems(ctx)
						s.sendPlayerUpdate()
					}
				} else if s.player.Money >= totalCost {
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

// durabilityPointsLossAll mirrors Player::DurabilityPointsLossAll
// (Player.cpp:4934-4958): a flat point loss on every equipped item, or on
// every item when inventory is true. Negative points restore durability,
// clamped at max. Follows the Go durability convention (DB-backed + inventory
// refresh; the _ApplyItemMods stat strip on the 0-crossing and the
// SPELL_AURA_PREVENT_DURABILITY_LOSS gate are documented no-bridge, same as
// durabilityLossAll/rollDurabilityLossOnHit).
func (s *session) durabilityPointsLossAll(ctx context.Context, points int32, inventory bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB

	query := `SELECT ci.item, ii.itemEntry, ii.durability
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ?`
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
		// Player::DurabilityPointsLoss (Player.cpp:4960-4991): clamped at
		// zero and at max durability.
		nd := int64(itm.curDur) - int64(points)
		if nd < 0 {
			nd = 0
		}
		if nd > int64(maxDur) {
			nd = int64(maxDur)
		}
		if uint32(nd) != itm.curDur {
			updates = append(updates, itemDurUpdate{guid: itm.guid, newDur: uint32(nd)})
		}
	}

	for _, up := range updates {
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", up.newDur, up.guid)
	}

	if len(updates) > 0 {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
}

// durabilityLossChanceOnHit folds TrinityCore's DurabilityLossChance.Damage
// config (World.cpp:619, default 0.5): roll_chance_f(rate) is a percent roll,
// so 0.5 means a 0.5% chance per hit. There is no Go rate config (runes.go
// precedent), so the default is folded as a constant.
const durabilityLossChanceOnHit = 0.005

// rollDurabilityLossOnHit mirrors the two random-durability arms of
// Unit::DealDamage (Unit.cpp:906-913 HIT TAKEN on a player victim, 925-931 HIT
// DONE by a player attacker): on a non-lethal hit with damage > 0 the side
// rolls DurabilityLossChance.Damage and loses 1 durability point on a random
// equipment slot (urand(0, EQUIPMENT_SLOT_END-1)). The two arms roll
// independently — call once per side.
// Documented no-bridge arms: SPELL_AURA_PREVENT_DURABILITY_LOSS (aura 402 has
// no Go aura-type model) and the _ApplyItemMods stat strip on the 0-crossing
// (Go's durability model is DB-backed with a full inventory refresh, the
// durabilityLossAll precedent — stats are not re-derived on durability
// changes).
func (s *session) rollDurabilityLossOnHit(ctx context.Context, damage uint32) {
	if s == nil || s.player == nil || damage == 0 || ctx == nil {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	// roll_chance_f(rate): percent roll against DurabilityLossChance.Damage.
	if rand.Float64() >= durabilityLossChanceOnHit {
		return
	}
	// DurabilityPointLossForEquipSlot: GetItemByPos(INVENTORY_SLOT_BAG_0,
	// slot) — a random equipment slot 0..EQUIPMENT_SLOT_END-1; no item in
	// the slot is a silent no-op.
	slot := uint32(rand.Intn(int(equipmentSlotEnd)))
	cdb := s.server.CharactersStore.DB
	var itemGUID uint64
	var curDur uint32
	err := cdb.QueryRowContext(ctx, `SELECT ci.item, ii.durability
		FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot = ? LIMIT 1`,
		s.playerGUID, slot).Scan(&itemGUID, &curDur)
	if err != nil || curDur == 0 {
		return
	}
	// DurabilityPointsLoss(item, 1): clamped at zero; an unchanged value
	// (already zero) writes nothing.
	newDur := curDur - 1
	_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", newDur, itemGUID)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
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
			s.adjustQuestItemCount(ctx, gemEntries[index], 1, false)
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

// equipmentSetNextGUID is the server-global equipment-set GUID counter,
// mirroring ObjectMgr::_equipmentSetGuid (ObjectMgr.cpp:7356), seeded from
// MAX(setguid)+1 on first use.
var equipmentSetNextGUID uint64 = 1

func newEquipmentSetGUID() uint64 {
	return atomic.AddUint64(&equipmentSetNextGUID, 1) - 1
}

func reserveEquipmentSetGUID(id uint64) {
	for {
		next := atomic.LoadUint64(&equipmentSetNextGUID)
		if next > id {
			return
		}
		if atomic.CompareAndSwapUint64(&equipmentSetNextGUID, next, id+1) {
			return
		}
	}
}

// handleEquipmentSetSave processes CMSG_EQUIPMENT_SET_SAVE (0x4BD).
// Reference: WorldSession::HandleEquipmentSetSave (CharacterHandler.cpp:1492),
// Player::SetEquipmentSet (Player.cpp:25995), ObjectMgr::GenerateEquipmentSetGuid
// (ObjectMgr.cpp:7356).
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
	if err != nil {
		return false
	}
	if index >= maxEquipmentSetIndex {
		// C++ silently drops an out-of-range set index (HandleEquipmentSetSave,
		// CharacterHandler.cpp:1498): no disconnect, no packet.
		return true
	}
	name, err := r.ReadCString()
	if err != nil {
		return false
	}
	iconName, err := r.ReadCString()
	if err != nil {
		return false
	}

	var sentItems [19]uint64
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
		sentItems[i] = itemGuid
	}

	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB

	// Cheat check from HandleEquipmentSetSave: a slot is only saved when the
	// referenced item is actually equipped there
	// (_player->GetItemByPos(INVENTORY_SLOT_BAG_0, i) GUID match); anything
	// else is dropped, leaving the slot empty like C++'s continue.
	equipped := make(map[uint32]uint64, equipmentSlotEnd)
	if rows, err := cdb.QueryContext(ctx, "SELECT slot, item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot < ?", s.playerGUID, equipmentSlotEnd); err == nil {
		for rows.Next() {
			var slot uint32
			var item uint64
			if err := rows.Scan(&slot, &item); err == nil {
				equipped[slot] = item
			}
		}
		rows.Close()
	}

	var items [19]uint64
	for i := uint32(0); i < equipmentSlotEnd; i++ {
		if sentItems[i] == 0 {
			continue
		}
		if eq, ok := equipped[i]; ok && eq == sentItems[i]&0xFFFFFFFF {
			items[i] = sentItems[i] & 0xFFFFFFFF
		}
	}

	// Player::SetEquipmentSet semantics: a nonzero GUID must name an existing
	// set of this player, otherwise the save is refused; a zero GUID means a
	// new set, whose GUID the server generates (GenerateEquipmentSetGuid) and
	// reports back in SMSG_EQUIPMENT_SET_SAVED. Updates send no packet.
	isNew := setGuid == 0
	if isNew {
		var maxGUID uint64
		if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(setguid), 0) FROM character_equipmentsets").Scan(&maxGUID); err == nil {
			reserveEquipmentSetGUID(maxGUID)
		}
		setGuid = newEquipmentSetGUID()
	} else {
		var exists bool
		// A nonzero GUID must name an existing set of this player; C++
		// logs an error and drops the save otherwise (no disconnect).
		if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM character_equipmentsets WHERE guid = ? AND setguid = ?", s.playerGUID, setGuid).Scan(&exists); err != nil {
			return true
		}
	}

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

	if isNew {
		// Send SMSG_EQUIPMENT_SET_SAVED (0x137) with the server-generated GUID
		savedBuf := protocol.NewBuffer(16)
		savedBuf.WriteU32(index)
		savedBuf.WritePackedGUID(setGuid)
		_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_SAVED), savedBuf.Bytes(), true)
	}
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

	inCombat := s.player.UnitFlags&unitFlagInCombat != 0

	for i := uint32(0); i < equipmentSlotEnd; i++ {
		itemGuid, err := r.ReadPackedGUID()
		if err != nil {
			break
		}
		// srcbag/srcslot are still read for protocol framing but the item is
		// located by GUID, not by the client's coordinates (CharacterHandler.cpp:1574).
		if _, err := r.ReadU8(); err != nil {
			break
		}
		if _, err := r.ReadU8(); err != nil {
			break
		}
		// Slots set to "ignored" (raw value 1) must not be unequipped (CharacterHandler.cpp:1569).
		if itemGuid == 1 {
			continue
		}
		// Only weapons may be swapped in combat (CharacterHandler.cpp:1572).
		if inCombat && i != uint32(equipSlotMainhand) && i != uint32(equipSlotOffhand) && i != uint32(equipSlotRanged) {
			continue
		}
		if itemGuid == 0 {
			// Empty slot: unequip the worn item into the bags (CharacterHandler.cpp:1576-1593).
			s.equipmentSetUnequipSlot(ctx, i)
			continue
		}
		// C++ ignores srcbag/srcslot: the item is located by GUID across the
		// whole inventory (GetItemByGuid) and swapped from its actual position
		// (CharacterHandler.cpp:1574-1595).
		s.equipmentSetSwapToSlot(ctx, i, itemGuid)
	}

	// Send SMSG_EQUIPMENT_SET_USE_RESULT (0x4D6) with 0 = success
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0) // 0 = ERR_EQUIPMENT_SET_USE_SUCCESS
	_ = s.write(uint16(protocol.OpcodeSMSG_EQUIPMENT_SET_USE_RESULT), buf.Bytes(), true)
	return true
}

// equipmentSetSwapToSlot moves the set piece named by the packed GUID into
// equipment slot (WorldSession::HandleEquipmentSetUse,
// CharacterHandler.cpp:1574-1595: Item* item = _player->GetItemByGuid
// (itemGuid) — the client's srcbag/srcslot are not used; an unknown GUID
// takes the unequip arm; an item already at dstpos is skipped; otherwise
// CanEquipItem then SwapItem(item->GetPos(), dstpos)).
func (s *session) equipmentSetSwapToSlot(ctx context.Context, slot uint32, itemGuid uint64) {
	db := s.server.CharactersStore.DB
	if db == nil {
		return
	}
	counter := int64(itemGuid & 0xFFFFFFFF)
	var bagKey, itemSlot int64
	if err := db.QueryRowContext(ctx, "SELECT bag, slot FROM character_inventory WHERE guid = ? AND item = ? LIMIT 1", s.playerGUID, counter).Scan(&bagKey, &itemSlot); err != nil {
		// Unknown GUID (C++ GetItemByGuid returns null): the unequip arm
		// (CharacterHandler.cpp:1576-1593).
		s.equipmentSetUnequipSlot(ctx, slot)
		return
	}
	if bagKey == 0 && uint32(itemSlot) == slot {
		return
	}
	bagByte, ok := s.equipmentSetBagByte(ctx, bagKey)
	if !ok {
		return
	}
	// CanEquipItem(i, dstpos) is approximated by handleSwapItem's
	// isSlotValidForItem gate; SwapItem maps onto the coordinate swap.
	_ = s.handleSwapItem(ctx, []byte{0, uint8(slot), bagByte, uint8(itemSlot)})
}

// equipmentSetBagByte maps a character_inventory bag key (0 for the main
// inventory, otherwise the container's item GUID) back to the packet-style
// bag byte that handleSwapItem's inventoryBagKey translation expects.
func (s *session) equipmentSetBagByte(ctx context.Context, bagKey int64) (uint8, bool) {
	if bagKey == 0 {
		return 0, true
	}
	db := s.server.CharactersStore.DB
	if db == nil {
		return 0, false
	}
	var slot int64
	if err := db.QueryRowContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0 AND item = ? LIMIT 1", s.playerGUID, bagKey).Scan(&slot); err != nil {
		return 0, false
	}
	switch {
	case slot >= 19 && slot <= 22:
		return uint8(slot - 18), true
	case slot >= 67 && slot <= 73:
		return uint8(slot - 62), true
	}
	return 0, false
}

// equipmentSetUnequipSlot moves the item worn in equipment slot into the
// inventory (WorldSession::HandleEquipmentSetUse, CharacterHandler.cpp:1576-
// 1593: CanStoreItem NULL_BAG/NULL_SLOT, then CanUnequipItem(dstpos) — always
// OK for equipment slots since bags cannot be equipped — then RemoveItem +
// StoreItem; a full inventory sends the equip error).
func (s *session) equipmentSetUnequipSlot(ctx context.Context, slot uint32) {
	db := s.server.CharactersStore.DB
	if db == nil {
		return
	}
	var itemGUID int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, slot).Scan(&itemGUID)
	if itemGUID == 0 {
		return
	}
	// CanStoreItem(NULL_BAG, NULL_SLOT) searches the backpack first, then the
	// equipped bags — findFreeInventorySlot covers both (findFreeBackpackSlot
	// stopped at the backpack and reported full too early).
	freeBagKey, _, freeSlot, ok := s.findFreeInventorySlot(ctx, s.playerGUID)
	if !ok {
		s.sendEquipError(equipErrInvFull, uint64(itemGUID))
		return
	}
	_, _ = db.ExecContext(ctx, "UPDATE character_inventory SET bag = ?, slot = ? WHERE guid = ? AND item = ?", freeBagKey, freeSlot, s.playerGUID, itemGUID)
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
}
