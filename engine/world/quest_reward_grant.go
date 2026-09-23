package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questRewardChoiceLimit  = 6
	inventoryRewardBag      = 0
	inventoryRewardFirst    = 23
	inventoryRewardLast     = 38
	questInventoryFull      = 4
	questFlagsDaily         = 0x00001000
	questFlagsWeekly        = 0x00008000
	questSpecialRepeatable  = 0x001
	questSpecialDungeonFind = 0x008
	questSpecialMonthly     = 0x010
	questSpecialAllowed     = 0x03F
)

var errQuestInventoryFull = errors.New("quest reward inventory is full")

type inventoryRewardRecord struct {
	Bag      int64
	Slot     uint8
	ItemGUID uint32
	Entry    uint32
	Count    uint32
}

type questItemGrant struct {
	ItemGUID       uint32
	Entry          uint32
	Count          uint32
	Bag            uint8
	Slot           uint32
	InventoryCount uint32
	Stacked        bool
}

type questRewardPersistenceState struct {
	DailyFlag     bool
	DailyStatus   bool
	DungeonFinder bool
	Weekly        bool
	Monthly       bool
	Seasonal      bool
	Rewarded      bool
	SortID        int32
	EventID       uint32
}

func (s *session) handleQuestgiverChooseReward(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	giverGUID, err := reader.ReadU64()
	if err != nil {
		return true
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return true
	}
	reward, err := reader.ReadU32()
	if err != nil {
		return true
	}
	if reward >= questRewardChoiceLimit {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reward", reward, "reason", "invalid choice")
		return true
	}
	if _, ok := s.questGiverEnder(ctx, giverGUID, questID); !ok {
		return true
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		s.debug("quest reward load failed", "account", s.accountName, "quest", questID, "error", err)
		return true
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil {
		return false
	}
	if status == questStatusIncomplete && s.canCompleteQuest(ctx, questID) {
		s.completeQuest(ctx, questID)
		status = questStatusComplete
	}
	if status != questStatusComplete && view.Detail.Flags&questAutoCompleteFlags == 0 {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reason", "quest incomplete", "status", status)
		return true
	}
	if len(view.Detail.ChoiceItems) > 0 && (reward >= uint32(len(view.Detail.ChoiceItems)) || view.Detail.ChoiceItems[reward].ID == 0) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reward", reward, "reason", "choice unavailable")
		return true
	}
	if complete, itemErr := s.hasQuestRequiredItems(ctx, view.RequiredItems); itemErr != nil {
		return false
	} else if !complete {
		return true
	}
	grants, destroyedGUIDs, err := s.commitQuestReward(ctx, view, reward)
	if errors.Is(err, errQuestInventoryFull) {
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_FAILED), buildQuestFailed(view.Detail.ID, questInventoryFull), true)
		return true
	}
	if err != nil {
		s.debug("quest reward commit failed", "account", s.accountName, "quest", questID, "error", err)
		return false
	}
	for _, fullGUID := range destroyedGUIDs {
		s.sendDestroyObject(fullGUID, false)
		s.despawnItem(fullGUID)
	}
	for _, grant := range grants {
		if err := s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(s.playerGUID, grant.Bag, grant.Slot, grant.Entry, grant.Count, grant.InventoryCount, grant.Stacked), true); err != nil {
			return false
		}
	}
	s.player.Money += view.Detail.RewardMoney
	rewardXP := s.calculateQuestRewardXP(ctx, view.Detail.ID, view.Detail.RewardXPDifficulty)
	if rewardXP > 0 {
		s.grantXP(ctx, rewardXP)
	}
	for i := 0; i < questRewardFactions; i++ {
		facID := view.Detail.RewardFactionID[i]
		facVal := view.Detail.RewardFactionValue[i]
		if facID > 0 && facVal != 0 {
			s.giveReputation(ctx, facID, facVal)
		}
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_COMPLETE), buildQuestRewardComplete(view.Detail.ID, rewardXP, view.Detail.RewardMoney, 0, view.Detail.RewardTalents, uint32(view.Detail.RewardArenaPoints)), true); err != nil {
		return false
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	if giverGUID != 0 {
		var entry uint32
		if creature := s.luaCreature(ctx, giverGUID); creature != nil {
			entry = objectUint32OrZero(creature, "Entry")
		}
		if entry == 0 {
			entry = uint32((giverGUID >> 24) & 0x00FFFFFF)
		}
		if entry == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			rawGUID := giverGUID & 0x00000000FFFFFFFF
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", rawGUID).Scan(&entry)
		}
		status := uint8(questDialogNone)
		if entry != 0 {
			if qStatus, err := s.questDialogStatus(ctx, entry); err == nil {
				status = qStatus
			}
		}
		packet := protocol.NewBuffer(9)
		packet.WriteU64(giverGUID)
		packet.WriteU8(status)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS), packet.Bytes(), true)
	}
	s.gossip = nil
	s.gossipClosed = true
	s.debug("quest rewarded", "account", s.accountName, "quest", questID, "reward", reward, "items", len(grants), "money", view.Detail.RewardMoney)
	return true
}

func (s *session) commitQuestReward(ctx context.Context, view questRewardView, choice uint32) ([]questItemGrant, []uint64, error) {
	s.server.inventoryMu.Lock()
	defer s.server.inventoryMu.Unlock()
	questState, err := s.loadQuestRewardPersistenceState(ctx, view.Detail.ID)
	if err != nil {
		return nil, nil, err
	}

	fixed := append([]questRewardItem(nil), view.Detail.RewardItems...)
	if len(view.Detail.ChoiceItems) > 0 && choice < uint32(len(view.Detail.ChoiceItems)) {
		fixed = append(fixed, view.Detail.ChoiceItems[choice])
	}
	equippedBags := s.getEquippedBags(ctx, s.playerGUID)
	stackables := make(map[uint32]int64)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		for _, it := range fixed {
			if it.ID > 0 {
				var st int64
				_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT stackable FROM item_template WHERE entry = ?", it.ID).Scan(&st)
				if st < 1 {
					st = 1
				}
				stackables[it.ID] = st
			}
		}
	}

	tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	rollback := func(cause error) ([]questItemGrant, []uint64, error) {
		_ = tx.Rollback()
		return nil, nil, cause
	}
	inventory, err := loadInventoryRewardRecords(ctx, tx, s.playerGUID)
	if err != nil {
		return rollback(err)
	}
	destroyedGUIDs, err := consumeQuestItems(ctx, tx, inventory, view.RequiredItems)
	if err != nil {
		return rollback(err)
	}
	grants, err := s.storeQuestRewardItems(ctx, tx, inventory, fixed, equippedBags, stackables)
	if err != nil {
		return rollback(err)
	}
	if view.Detail.RewardMoney != 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE characters SET money = money + ? WHERE guid = ? AND account = ?", view.Detail.RewardMoney, s.playerGUID, s.accountID); err != nil {
			return rollback(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", s.playerGUID, view.Detail.ID); err != nil {
		return rollback(err)
	}
	insertIgnore := "INSERT OR IGNORE"
	if s.server.CharactersStore.Backend != database.BackendSQLite {
		insertIgnore = "INSERT IGNORE"
	}
	if questState.DailyStatus {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_daily (guid, quest, time) VALUES (?, ?, ?)", s.playerGUID, view.Detail.ID, time.Now().Unix()); err != nil {
			return rollback(err)
		}
	} else if questState.Weekly {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_weekly (guid, quest) VALUES (?, ?)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	} else if questState.Monthly {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_monthly (guid, quest) VALUES (?, ?)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	} else if questState.Seasonal {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_seasonal (guid, quest, event) VALUES (?, ?, ?)", s.playerGUID, view.Detail.ID, questState.EventID); err != nil {
			return rollback(err)
		}
	}
	if questState.Rewarded {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_rewarded (guid, quest, active) VALUES (?, ?, 1)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	}
	questID := view.Detail.ID
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	for _, item := range view.RequiredItems {
		if item.ID != 0 && item.Quantity != 0 {
			s.adjustQuestItemCount(ctx, item.ID, item.Quantity, false)
		}
	}
	for _, item := range fixed {
		if item.ID != 0 && item.Quantity != 0 {
			s.adjustQuestItemCount(ctx, item.ID, item.Quantity, true)
		}
	}
	s.applyQuestRewardPersistenceState(questID, questState)
	s.updateAchievementCriteria(criteriaTypeCompleteQuest, questID, 1)
	s.updateAchievementCriteria(criteriaTypeQuestCount, 0, 1)
	if view.Detail.RewardMoney > 0 {
		s.updateAchievementCriteria(criteriaTypeMoneyFromQuest, 0, uint32(view.Detail.RewardMoney))
	}
	if questState.SortID > 0 {
		s.updateAchievementCriteria(criteriaTypeCompleteQuestsInZone, uint32(questState.SortID), 1)
	}
	if questState.DailyFlag {
		s.updateAchievementCriteria(criteriaTypeCompleteDailyQuest, 0, 1)
		s.updateAchievementCriteria(criteriaTypeCompleteDailyQuestDaily, 0, 1)
	}
	return grants, destroyedGUIDs, nil
}

func (s *session) loadQuestRewardPersistenceState(ctx context.Context, questID uint32) (questRewardPersistenceState, error) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return questRewardPersistenceState{}, errors.New("world database unavailable for quest reward state")
	}
	var flags uint32
	var sortID int32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Flags, QuestSortID FROM quest_template WHERE ID = ?", questID).Scan(&flags, &sortID); err != nil {
		return questRewardPersistenceState{}, err
	}
	var specialFlags uint32
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	if err != nil && err != sql.ErrNoRows {
		return questRewardPersistenceState{}, err
	}
	specialFlags &= questSpecialAllowed
	if flags&(questFlagsDaily|questFlagsWeekly) != 0 {
		specialFlags |= questSpecialRepeatable
	}
	state := questRewardPersistenceState{
		DailyFlag:     flags&questFlagsDaily != 0,
		DailyStatus:   flags&questFlagsDaily != 0 || specialFlags&questSpecialDungeonFind != 0,
		DungeonFinder: specialFlags&questSpecialDungeonFind != 0,
		Weekly:        flags&questFlagsWeekly != 0,
		Monthly:       specialFlags&questSpecialMonthly != 0,
		SortID:        sortID,
	}
	state.Seasonal = isSeasonalQuestSort(sortID) && specialFlags&questSpecialRepeatable == 0
	state.Rewarded = !state.DungeonFinder && !state.DailyFlag && (specialFlags&questSpecialRepeatable == 0 || state.Weekly || state.Monthly || state.Seasonal)
	if state.DailyStatus || state.Weekly {
		state.Monthly = false
		state.Seasonal = false
	} else if state.Monthly {
		state.Seasonal = false
	}
	if state.Seasonal {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT eventEntry FROM game_event_seasonal_questrelation WHERE questId = ?", questID)
		if err != nil {
			return questRewardPersistenceState{}, err
		}
		for rows.Next() {
			var eventID int64
			if err := rows.Scan(&eventID); err != nil {
				_ = rows.Close()
				return questRewardPersistenceState{}, err
			}
			if eventID >= 0 {
				state.EventID = uint32(uint16(eventID))
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return questRewardPersistenceState{}, err
		}
		if err := rows.Close(); err != nil {
			return questRewardPersistenceState{}, err
		}
	}
	return state, nil
}

func isSeasonalQuestSort(sortID int32) bool {
	switch sortID {
	case -22, -284, -366, -369, -370, -374, -376:
		return true
	default:
		return false
	}
}

func (s *session) applyQuestRewardPersistenceState(questID uint32, state questRewardPersistenceState) {
	if s.player == nil {
		return
	}
	if state.DailyStatus {
		if dailyTime := time.Now().Unix(); dailyTime > s.player.LastDailyQuestTime {
			s.player.LastDailyQuestTime = dailyTime
		}
		if state.DungeonFinder {
			if s.player.DungeonFinderQuests == nil {
				s.player.DungeonFinderQuests = make(map[uint32]struct{})
			}
			s.player.DungeonFinderQuests[questID] = struct{}{}
		} else {
			for index := range s.player.DailyQuests {
				if s.player.DailyQuests[index] == 0 {
					s.player.DailyQuests[index] = questID
					break
				}
			}
		}
		return
	}
	if state.Weekly {
		if s.player.WeeklyQuests == nil {
			s.player.WeeklyQuests = make(map[uint32]struct{})
		}
		s.player.WeeklyQuests[questID] = struct{}{}
	} else if state.Monthly {
		if s.player.MonthlyQuests == nil {
			s.player.MonthlyQuests = make(map[uint32]struct{})
		}
		s.player.MonthlyQuests[questID] = struct{}{}
	} else if state.Seasonal {
		if s.player.SeasonalQuests == nil {
			s.player.SeasonalQuests = make(map[uint32]map[uint32]struct{})
		}
		if s.player.SeasonalQuests[state.EventID] == nil {
			s.player.SeasonalQuests[state.EventID] = make(map[uint32]struct{})
		}
		s.player.SeasonalQuests[state.EventID][questID] = struct{}{}
	}
}

func loadInventoryRewardRecords(ctx context.Context, tx *sql.Tx, guid uint64) ([]inventoryRewardRecord, error) {
	rows, err := tx.QueryContext(ctx, "SELECT ci.bag, ci.slot, ci.item, ii.itemEntry, ii.count FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? ORDER BY ci.bag, ci.slot", guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]inventoryRewardRecord, 0)
	for rows.Next() {
		var bag, slot, itemGUID, entry, count int64
		if err := rows.Scan(&bag, &slot, &itemGUID, &entry, &count); err != nil {
			return nil, err
		}
		if bag < 0 || slot < 0 || slot > 255 || itemGUID < 0 || itemGUID > int64(^uint32(0)) || entry < 0 || entry > int64(^uint32(0)) || count <= 0 {
			continue
		}
		if count > int64(^uint32(0)) {
			count = int64(^uint32(0))
		}
		result = append(result, inventoryRewardRecord{Bag: bag, Slot: uint8(slot), ItemGUID: uint32(itemGUID), Entry: uint32(entry), Count: uint32(count)})
	}
	return result, rows.Err()
}

func consumeQuestItems(ctx context.Context, tx *sql.Tx, inventory []inventoryRewardRecord, required []questRewardItem) ([]uint64, error) {
	needed := make(map[uint32]uint64)
	for _, item := range required {
		if item.ID != 0 && item.Quantity != 0 {
			needed[item.ID] += uint64(item.Quantity)
		}
	}
	ids := make([]uint32, 0, len(needed))
	for id := range needed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var destroyed []uint64
	for _, entry := range ids {
		remaining := needed[entry]
		for index := range inventory {
			record := &inventory[index]
			if record.Entry != entry || record.Count == 0 || remaining == 0 {
				continue
			}
			remove := uint64(record.Count)
			if remove > remaining {
				remove = remaining
			}
			record.Count -= uint32(remove)
			remaining -= remove
			if record.Count == 0 {
				fullGUID := uint64(record.ItemGUID) | (uint64(0x4000) << 48)
				destroyed = append(destroyed, fullGUID)
				if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", record.ItemGUID); err != nil {
					return nil, err
				}
				if _, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", record.ItemGUID); err != nil {
					return nil, err
				}
			} else if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", record.Count, record.ItemGUID); err != nil {
				return nil, err
			}
		}
		if remaining != 0 {
			return nil, fmt.Errorf("required quest item %d is unavailable", entry)
		}
	}
	return destroyed, nil
}

type rewardSlotLocation struct {
	BagKey    int64
	ClientBag uint8
	Slot      uint8
}

func (s *session) storeQuestRewardItems(ctx context.Context, tx *sql.Tx, inventory []inventoryRewardRecord, items []questRewardItem, equippedBags []equippedBagInfo, stackables map[uint32]int64) ([]questItemGrant, error) {
	grants := make([]questItemGrant, 0, len(items))
	var nextGUID uint32
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) FROM item_instance").Scan(&nextGUID); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == 0 || item.Quantity == 0 {
			continue
		}
		stackable := stackables[item.ID]
		if stackable < 1 {
			stackable = 1
		}
		remaining := uint64(item.Quantity)
		for index := range inventory {
			record := &inventory[index]
			if record.Entry != item.ID || record.Count >= uint32(stackable) || remaining == 0 {
				continue
			}
			space := uint64(uint32(stackable) - record.Count)
			add := remaining
			if add > space {
				add = space
			}
			record.Count += uint32(add)
			remaining -= add
			if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", record.Count, record.ItemGUID); err != nil {
				return nil, err
			}
			clientBag := resolveRewardClientBag(record.Bag, equippedBags)
			grants = append(grants, questItemGrant{
				Entry:          item.ID,
				Count:          uint32(add),
				Bag:            clientBag,
				Slot:           uint32(record.Slot),
				InventoryCount: record.Count,
				Stacked:        true,
			})
		}
		for remaining != 0 {
			loc, ok := findNextRewardSlot(inventory, equippedBags)
			if !ok {
				return nil, errQuestInventoryFull
			}
			add := remaining
			if add > uint64(stackable) {
				add = uint64(stackable)
			}
			nextGUID++
			if nextGUID == 0 {
				return nil, errors.New("item guid space exhausted")
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, giftCreatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, ?, ?, 0, 0, ?, 0, '', 0, '', 0, 0, 0, NULL)", nextGUID, item.ID, s.playerGUID, add); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, loc.BagKey, loc.Slot, nextGUID); err != nil {
				return nil, err
			}
			inventory = append(inventory, inventoryRewardRecord{Bag: loc.BagKey, Slot: loc.Slot, ItemGUID: nextGUID, Entry: item.ID, Count: uint32(add)})
			remaining -= add
			grants = append(grants, questItemGrant{
				ItemGUID:       nextGUID,
				Entry:          item.ID,
				Count:          uint32(add),
				Bag:            loc.ClientBag,
				Slot:           uint32(loc.Slot),
				InventoryCount: uint32(add),
				Stacked:        false,
			})
		}
	}
	return grants, nil
}

func findNextRewardSlot(inventory []inventoryRewardRecord, equippedBags []equippedBagInfo) (rewardSlotLocation, bool) {
	occupiedBackpack := make(map[uint8]struct{})
	for _, record := range inventory {
		if record.Bag == 0 && record.Slot >= inventoryRewardFirst && record.Slot <= inventoryRewardLast && record.Count != 0 {
			occupiedBackpack[record.Slot] = struct{}{}
		}
	}
	for slot := inventoryRewardFirst; slot <= inventoryRewardLast; slot++ {
		if _, ok := occupiedBackpack[uint8(slot)]; !ok {
			return rewardSlotLocation{BagKey: 0, ClientBag: 0, Slot: uint8(slot)}, true
		}
	}

	for _, bag := range equippedBags {
		if bag.slots == 0 {
			continue
		}
		occupiedBag := make(map[uint8]struct{})
		for _, record := range inventory {
			if record.Bag == bag.guid && record.Count != 0 {
				occupiedBag[record.Slot] = struct{}{}
			}
		}
		for slot := uint8(0); slot < uint8(bag.slots); slot++ {
			if _, ok := occupiedBag[slot]; !ok {
				return rewardSlotLocation{BagKey: bag.guid, ClientBag: bag.slot, Slot: slot}, true
			}
		}
	}

	return rewardSlotLocation{}, false
}

func resolveRewardClientBag(bagKey int64, equippedBags []equippedBagInfo) uint8 {
	if bagKey == 0 {
		return 0
	}
	for _, bag := range equippedBags {
		if bag.guid == bagKey {
			return bag.slot
		}
	}
	return 0
}

func buildItemPushResult(playerGUID uint64, bag uint8, slot, entry, count, inventoryCount uint32, stacked bool) []byte {
	packet := protocol.NewBuffer(48)
	packet.WriteU64(playerGUID)
	packet.WriteU32(1)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU8(bag)
	if stacked {
		packet.WriteU32(^uint32(0))
	} else {
		packet.WriteU32(slot)
	}
	packet.WriteU32(entry)
	packet.WriteU32(0)
	packet.WriteI32(0)
	packet.WriteU32(count)
	packet.WriteU32(inventoryCount)
	return packet.Bytes()
}

func buildQuestRewardComplete(questID, xp, money, honor, talents, arena uint32) []byte {
	packet := protocol.NewBuffer(24)
	packet.WriteU32(questID)
	packet.WriteU32(xp)
	packet.WriteU32(money)
	packet.WriteU32(honor)
	packet.WriteU32(talents)
	packet.WriteU32(arena)
	return packet.Bytes()
}

func buildQuestFailed(questID, reason uint32) []byte {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(questID)
	packet.WriteU32(reason)
	return packet.Bytes()
}

func (s *session) calculateQuestRewardXP(ctx context.Context, questID, xpDiff uint32) uint32 {
	if xpDiff >= 9 {
		return 0
	}
	var qLevel int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel FROM quest_template WHERE entry = ?", questID).Scan(&qLevel)
	}
	if qLevel <= 0 {
		if s.player != nil && s.player.Level > 0 {
			qLevel = int64(s.player.Level)
		} else {
			qLevel = 1
		}
	}
	if qLevel < 1 {
		qLevel = 1
	} else if qLevel > 100 {
		qLevel = 100
	}
	entry, ok := questXPDBC[uint32(qLevel)]
	if !ok {
		return 0
	}
	return entry[xpDiff]
}

var questXPDBC = map[uint32][9]uint32{
	1:  {0, 10, 20, 40, 60, 80, 100, 120, 160},
	2:  {0, 15, 45, 85, 130, 170, 215, 255, 340},
	3:  {0, 25, 65, 125, 190, 250, 315, 380, 500},
	4:  {0, 35, 90, 180, 270, 355, 445, 540, 710},
	5:  {0, 45, 110, 225, 335, 450, 560, 670, 900},
	6:  {0, 55, 135, 270, 405, 540, 680, 810, 1080},
	7:  {0, 65, 160, 315, 475, 630, 790, 950, 1260},
	8:  {0, 70, 175, 350, 530, 700, 880, 1050, 1400},
	9:  {0, 80, 195, 390, 590, 780, 980, 1150, 1560},
	10: {0, 85, 210, 420, 630, 840, 1050, 1250, 1680},
	11: {0, 90, 220, 440, 660, 880, 1100, 1300, 1760},
	12: {0, 90, 225, 455, 680, 910, 1150, 1350, 1820},
	13: {0, 90, 230, 455, 680, 910, 1150, 1350, 1820},
	14: {0, 100, 245, 490, 740, 980, 1250, 1450, 1960},
	15: {0, 105, 270, 540, 800, 1050, 1350, 1600, 2100},
	16: {0, 115, 290, 580, 880, 1150, 1450, 1750, 2300},
	17: {0, 125, 315, 630, 950, 1250, 1600, 1900, 2500},
	18: {0, 135, 340, 680, 1000, 1350, 1700, 2050, 2700},
	19: {0, 145, 365, 730, 1100, 1450, 1800, 2200, 2900},
	20: {0, 155, 390, 780, 1150, 1550, 1950, 2350, 3100},
	21: {0, 165, 415, 830, 1250, 1650, 2050, 2500, 3300},
	22: {0, 175, 435, 870, 1300, 1750, 2200, 2600, 3500},
	23: {0, 185, 460, 920, 1400, 1850, 2300, 2750, 3700},
	24: {0, 195, 485, 970, 1450, 1950, 2400, 2900, 3900},
	25: {0, 200, 510, 1000, 1500, 2000, 2550, 3050, 4000},
	26: {0, 210, 530, 1050, 1600, 2100, 2650, 3150, 4200},
	27: {0, 220, 550, 1100, 1650, 2200, 2750, 3300, 4400},
	28: {0, 230, 570, 1150, 1700, 2300, 2850, 3400, 4600},
	29: {0, 235, 590, 1200, 1750, 2350, 2950, 3550, 4700},
	30: {0, 245, 610, 1200, 1850, 2450, 3050, 3650, 4900},
	31: {0, 250, 630, 1250, 1900, 2540, 3150, 3800, 5080},
	32: {0, 270, 670, 1350, 2000, 2710, 3350, 4050, 5420},
	33: {0, 290, 720, 1450, 2150, 2900, 3600, 4350, 5800},
	34: {0, 310, 770, 1550, 2300, 3100, 3850, 4650, 6200},
	35: {0, 330, 820, 1650, 2450, 3300, 4100, 4950, 6600},
	36: {0, 350, 870, 1750, 2600, 3500, 4350, 5250, 7000},
	37: {0, 370, 920, 1850, 2750, 3710, 4600, 5550, 7420},
	38: {0, 390, 980, 1950, 2900, 3920, 4900, 5850, 7840},
	39: {0, 410, 1030, 2050, 3100, 4140, 5150, 6200, 8280},
	40: {0, 435, 1090, 2150, 3250, 4370, 5450, 6550, 8740},
	41: {0, 455, 1140, 2250, 3400, 4590, 5700, 6850, 9180},
	42: {0, 480, 1200, 2400, 3600, 4820, 6000, 7200, 9640},
	43: {0, 505, 1260, 2500, 3750, 5050, 6300, 7550, 10100},
	44: {0, 525, 1320, 2600, 3950, 5290, 6600, 7900, 10580},
	45: {0, 550, 1380, 2750, 4150, 5540, 6900, 8300, 11080},
	46: {0, 575, 1440, 2850, 4300, 5790, 7200, 8650, 11580},
	47: {0, 600, 1510, 3000, 4500, 6040, 7550, 9050, 12080},
	48: {0, 625, 1570, 3100, 4700, 6290, 7850, 9400, 12580},
	49: {0, 655, 1630, 3250, 4900, 6550, 8150, 9800, 13100},
	50: {0, 680, 1700, 3400, 5100, 6810, 8500, 10200, 13620},
	51: {0, 705, 1760, 3500, 5300, 7070, 8800, 10600, 14140},
	52: {0, 730, 1830, 3650, 5500, 7340, 9150, 11000, 14680},
	53: {0, 760, 1900, 3800, 5700, 7610, 9500, 11400, 15220},
	54: {0, 785, 1970, 3900, 5900, 7890, 9850, 11800, 15780},
	55: {0, 815, 2040, 4050, 6100, 8170, 10200, 12250, 16340},
	56: {0, 845, 2110, 4200, 6300, 8450, 10550, 12650, 16900},
	57: {0, 870, 2180, 4350, 6500, 8730, 10900, 13050, 17460},
	58: {0, 900, 2250, 4500, 6750, 9020, 11250, 13500, 18040},
	59: {0, 930, 2320, 4650, 6950, 9310, 11600, 13950, 18620},
	60: {0, 955, 2380, 4750, 7150, 9550, 11900, 14300, 19100},
	61: {0, 970, 2400, 4850, 7300, 9800, 12200, 14700, 19600},
	62: {0, 1000, 2550, 5000, 7600, 10050, 12600, 15050, 20100},
	63: {0, 1050, 2600, 5250, 7800, 10400, 12950, 15550, 20800},
	64: {0, 1080, 2650, 5400, 8050, 10750, 13350, 16000, 21500},
	65: {0, 1100, 2700, 5500, 8250, 11000, 13750, 16500, 22000},
	66: {0, 1130, 2850, 5650, 8550, 11300, 14150, 17000, 22600},
	67: {0, 1160, 2900, 5800, 8750, 11650, 14600, 17450, 23300},
	68: {0, 1200, 3050, 6000, 9000, 12000, 14950, 17950, 24000},
	69: {0, 1230, 3100, 6150, 9250, 12300, 15400, 18450, 24600},
	70: {0, 1250, 3150, 6250, 9500, 12650, 15800, 19000, 25300},
	71: {0, 2000, 5050, 10050, 15100, 20100, 25150, 30150, 40200},
	72: {0, 2050, 5100, 10150, 15250, 20300, 25400, 30450, 40600},
	73: {0, 2050, 5150, 10250, 15400, 20500, 25650, 30750, 41000},
	74: {0, 2100, 5200, 10400, 15550, 20750, 25950, 31150, 41500},
	75: {0, 2100, 5250, 10500, 15700, 20950, 26200, 31450, 41900},
	76: {0, 2100, 5300, 10600, 15850, 21150, 26450, 31750, 42300},
	77: {0, 2150, 5350, 10700, 16050, 21400, 26750, 32100, 42800},
	78: {0, 2150, 5400, 10800, 16200, 21600, 27000, 32400, 43200},
	79: {0, 2200, 5450, 10900, 16350, 21800, 27250, 32700, 43600},
	80: {0, 2200, 5500, 11050, 16550, 22050, 27550, 33100, 44100},
}
