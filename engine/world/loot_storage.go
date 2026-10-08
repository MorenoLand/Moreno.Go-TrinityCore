package world

import (
	"context"
)

// storedContainerItem mirrors StoredLootItem (Loot/LootItemStorage.h): one
// persisted row of a container (item) loot, keyed by the container item's
// instance GUID. Only the arms Go's lootItem models are kept — random
// property/suffix and follow_loot_rules have no Go fill model (loot.go
// writes 0 for the wire fields), so they are not persisted; the reopen
// re-derives the display fields from the template the same way the fill
// does.
type storedContainerItem struct {
	ItemEntry     uint32
	Count         uint32
	FreeForAll    bool
	IsBlocked     bool
	NeedsQuest    bool
	DisplayInfoID uint32
	Quality       uint32
}

// storedContainerLoot mirrors StoredLootContainer (Loot/LootItemStorage.h):
// the money plus the item rows saved for one container item instance.
type storedContainerLoot struct {
	Money uint32
	Items []storedContainerItem
}

// loadStoredContainerLoot mirrors LootItemStorage::LoadStorageFromDB
// (Loot/LootItemStorage.cpp:55): at startup it loads the persisted
// container-loot rows (item_loot_items) and money
// (item_loot_money) into the in-memory store, so a partially-looted
// container reopens with the same remainder after a server restart.
func (s *Server) loadStoredContainerLoot(ctx context.Context) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	s.lootMu.Lock()
	defer s.lootMu.Unlock()
	if s.storedContainerLoot == nil {
		s.storedContainerLoot = make(map[uint64]*storedContainerLoot)
	}
	rows, err := s.CharactersStore.QueryStatement(ctx, "CHAR_SEL_ITEMCONTAINER_ITEMS")
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("loot: stored container loot load failed", "error", err)
		}
		return
	}
	defer rows.Close()
	for rows.Next() {
		var containerID uint64
		var item storedContainerItem
		var ffa, blocked, needsQuest bool
		var followRules, counted, underThreshold bool
		var rndProp int32
		var rndSuffix uint32
		if err := rows.Scan(&containerID, &item.ItemEntry, &item.Count, &followRules, &ffa, &blocked, &counted, &underThreshold, &needsQuest, &rndProp, &rndSuffix); err != nil {
			continue
		}
		item.FreeForAll = ffa
		item.IsBlocked = blocked
		item.NeedsQuest = needsQuest
		st := s.storedContainerLoot[containerID]
		if st == nil {
			st = &storedContainerLoot{}
			s.storedContainerLoot[containerID] = st
		}
		st.Items = append(st.Items, item)
	}
	_ = rows.Close()
	mrows, err := s.CharactersStore.QueryStatement(ctx, "CHAR_SEL_ITEMCONTAINER_MONEY")
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("loot: stored container money load failed", "error", err)
		}
		return
	}
	defer mrows.Close()
	for mrows.Next() {
		var containerID uint64
		var money uint32
		if err := mrows.Scan(&containerID, &money); err != nil {
			continue
		}
		st := s.storedContainerLoot[containerID]
		if st == nil {
			st = &storedContainerLoot{}
			s.storedContainerLoot[containerID] = st
		}
		st.Money = money
	}
	_ = mrows.Close()
}

// applyStoredContainerLoot mirrors LootItemStorage::LoadStoredLoot
// (Loot/LootItemStorage.cpp:124): a container that already has persisted
// loot reopens with the stored money and item rows instead of a fresh
// roll. Returns false when nothing is stored, so the caller falls back
// to the first-open fill + store.
func (s *Server) applyStoredContainerLoot(ctx context.Context, opener *session, containerGUID uint64, loot *activeLootState) bool {
	if s == nil || opener == nil || loot == nil || containerGUID == 0 {
		return false
	}
	s.lootMu.Lock()
	st := s.storedContainerLoot[containerGUID]
	var items []storedContainerItem
	var money uint32
	if st != nil {
		items = st.Items
		money = st.Money
	}
	s.lootMu.Unlock()
	if st == nil {
		return false
	}
	loot.Money = money
	for _, stored := range items {
		data, err := opener.loadItemQueryData(ctx, stored.ItemEntry)
		if err != nil {
			continue
		}
		slot := uint8(len(loot.Items))
		loot.Items[slot] = lootItem{
			Slot:          slot,
			ItemEntry:     stored.ItemEntry,
			Count:         stored.Count,
			DisplayInfoID: data.DisplayInfoID,
			Quality:       data.Quality,
			IsBlocked:     stored.IsBlocked,
			NeedsQuest:    stored.NeedsQuest,
			FreeForAll:    stored.FreeForAll,
		}
	}
	loot.NormalSlotCount = uint8(len(loot.Items))
	loot.StoredContainerGUID = containerGUID
	return true
}

// storeNewContainerLoot mirrors LootItemStorage::AddNewStoredLoot
// (Loot/LootItemStorage.cpp:232): the first open of a container persists
// its rolled money and items so a later reopen (or a server restart)
// yields the same remainder. Currency-token rows are skipped — the
// auto-store arm already moved them into bags (ItemTemplate.h:684,
// LootItemStorage.cpp:270-273) — and rows the opener is not allowed to
// take are skipped, matching the AllowedForPlayer filter
// (LootItemStorage.cpp:267).
func (s *Server) storeNewContainerLoot(ctx context.Context, containerGUID uint64, loot *activeLootState, opener *session) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || containerGUID == 0 || loot == nil {
		return
	}
	s.lootMu.Lock()
	if s.storedContainerLoot == nil {
		s.storedContainerLoot = make(map[uint64]*storedContainerLoot)
	}
	if _, exists := s.storedContainerLoot[containerGUID]; exists {
		s.lootMu.Unlock()
		return
	}
	st := &storedContainerLoot{Money: loot.Money}
	for _, it := range loot.Items {
		if it.ItemEntry == 0 {
			continue
		}
		if opener != nil && !opener.lootItemAllowedForPlayer(ctx, it, false) {
			continue
		}
		data, err := opener.loadItemQueryData(ctx, it.ItemEntry)
		if err != nil || data.BagFamily&itemBagFamilyCurrency != 0 {
			continue
		}
		st.Items = append(st.Items, storedContainerItem{
			ItemEntry:     it.ItemEntry,
			Count:         it.Count,
			FreeForAll:    it.FreeForAll,
			IsBlocked:     it.IsBlocked,
			NeedsQuest:    it.NeedsQuest,
			DisplayInfoID: data.DisplayInfoID,
			Quality:       data.Quality,
		})
	}
	// LootItemStorage::AddNewStoredLoot's isLooted gate (Loot.h:236) +
	// Player.cpp:8705: the store fires only when gold > 0 or
	// unlootedCount > 0. unlootedCount counts opener/group-visible normal
	// and FFA rows (quest rows are never counted, Loot.cpp:176-183), so
	// the Go analog is: money, or at least one opener-eligible non-currency
	// row surviving the AllowedForPlayer filter above. Registering an
	// empty entry here would make applyStoredContainerLoot return true on
	// the next open and swallow the re-roll, where C++ (no AddNewStoredLoot
	// call at all) re-fills — e.g. a container that rolled only recipes
	// the opener already knows. C++'s canSeeItemInLootWindow group-member
	// leg (a groupmate could see a row the opener cannot) stays unmodeled:
	// Go's store filter is opener-scoped.
	if st.Money == 0 && len(st.Items) == 0 {
		s.lootMu.Unlock()
		return
	}
	s.storedContainerLoot[containerGUID] = st
	s.lootMu.Unlock()

	if st.Money > 0 {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_MONEY", containerGUID)
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_INS_ITEMCONTAINER_MONEY", containerGUID, st.Money)
	}
	_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_ITEMS", containerGUID)
	for _, it := range st.Items {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_INS_ITEMCONTAINER_ITEMS",
			containerGUID, it.ItemEntry, it.Count, false, it.FreeForAll, it.IsBlocked, false, false, it.NeedsQuest, 0, 0)
	}
	loot.StoredContainerGUID = containerGUID
}

// removeStoredContainerLootItem mirrors
// LootItemStorage::RemoveStoredLootItemForContainer
// (Loot/LootItemStorage.cpp:335): one taken row leaves the persisted
// container loot (Player::StoreLootItem, Player.cpp:25136-25137). The
// stored rows are a multimap keyed by entry, so the first row whose
// entry and count both match is the one removed.
func (s *Server) removeStoredContainerLootItem(ctx context.Context, containerGUID uint64, itemID, count uint32) {
	if s == nil || containerGUID == 0 || itemID == 0 {
		return
	}
	s.lootMu.Lock()
	if st := s.storedContainerLoot[containerGUID]; st != nil {
		for i, it := range st.Items {
			if it.ItemEntry == itemID && it.Count == count {
				st.Items = append(st.Items[:i], st.Items[i+1:]...)
				break
			}
		}
	}
	s.lootMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_ITEM", containerGUID, itemID, count)
	}
}

// removeStoredContainerLootMoney mirrors
// LootItemStorage::RemoveStoredMoneyForContainer
// (Loot/LootItemStorage.cpp:326): the money row leaves the persisted
// container loot (LootHandler.cpp:215-217).
func (s *Server) removeStoredContainerLootMoney(ctx context.Context, containerGUID uint64) {
	if s == nil || containerGUID == 0 {
		return
	}
	s.lootMu.Lock()
	if st := s.storedContainerLoot[containerGUID]; st != nil {
		st.Money = 0
	}
	s.lootMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_MONEY", containerGUID)
	}
}

// removeStoredContainerLoot mirrors
// LootItemStorage::RemoveStoredLootForContainer
// (Loot/LootItemStorage.cpp:200): the whole persisted container loot is
// dropped when the container item is destroyed (Player.cpp:12710,
// 13573, 19962, 19978).
func (s *Server) removeStoredContainerLoot(ctx context.Context, containerGUID uint64) {
	if s == nil || containerGUID == 0 {
		return
	}
	s.lootMu.Lock()
	delete(s.storedContainerLoot, containerGUID)
	s.lootMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_ITEMS", containerGUID)
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_ITEMCONTAINER_MONEY", containerGUID)
	}
}
