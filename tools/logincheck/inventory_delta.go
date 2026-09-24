package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type inventoryRowSnapshot struct {
	ItemGUID           int64
	OwnerGUID          int64
	Bag                int64
	Slot               int64
	ItemEntry          int64
	Count              int64
	ItemTemplateValid  bool
	Digest             string
	DigestWithoutCount string
}

func captureInventoryRow(table string, columns []string, values []any) (string, inventoryRowSnapshot, bool) {
	row := inventoryRowSnapshot{}
	for index, column := range columns {
		switch column {
		case "guid":
			if table == "character_inventory" {
				row.OwnerGUID = snapshotInt64(values[index])
			} else {
				row.ItemGUID = snapshotInt64(values[index])
			}
		case "bag":
			row.Bag = snapshotInt64(values[index])
		case "slot":
			row.Slot = snapshotInt64(values[index])
		case "item":
			row.ItemGUID = snapshotInt64(values[index])
		case "itemEntry":
			row.ItemEntry = snapshotInt64(values[index])
		case "owner_guid":
			row.OwnerGUID = snapshotInt64(values[index])
		case "count":
			row.Count = snapshotInt64(values[index])
		}
	}
	if table == "character_inventory" {
		row.Digest = inventoryRowDigest(columns, values, "")
		return fmt.Sprintf("%d:%d:%d:%d", row.OwnerGUID, row.Bag, row.Slot, row.ItemGUID), row, true
	}
	if table == "inventory_item_instances" && row.ItemGUID > 0 {
		row.Digest = inventoryRowDigest(columns, values, "")
		row.DigestWithoutCount = inventoryRowDigest(columns, values, "count")
		return fmt.Sprintf("%d", row.ItemGUID), row, true
	}
	return "", inventoryRowSnapshot{}, false
}

func inventoryRowDigest(columns []string, values []any, excludedColumn string) string {
	hash := sha256.New()
	for index, column := range columns {
		if column == excludedColumn {
			continue
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%T\x00%v\x00", column, values[index], values[index])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func inventoryRowsByGUID(rows map[string]inventoryRowSnapshot, table string) (map[int64]inventoryRowSnapshot, error) {
	if rows == nil {
		return nil, fmt.Errorf("%s snapshot has no keyed row evidence", table)
	}
	result := make(map[int64]inventoryRowSnapshot, len(rows))
	for _, row := range rows {
		if row.ItemGUID <= 0 {
			if table == "character_inventory" {
				continue
			}
			return nil, fmt.Errorf("%s snapshot contains an invalid item GUID", table)
		}
		if _, exists := result[row.ItemGUID]; exists {
			return nil, fmt.Errorf("%s snapshot repeats item GUID %d", table, row.ItemGUID)
		}
		result[row.ItemGUID] = row
	}
	return result, nil
}

func danglingInventoryLinksUnchanged(before, after map[string]inventoryRowSnapshot) bool {
	for key, row := range before {
		if row.ItemGUID != 0 {
			continue
		}
		current, exists := after[key]
		if !exists || current != row {
			return false
		}
	}
	for key, row := range after {
		if row.ItemGUID != 0 {
			continue
		}
		if previous, exists := before[key]; !exists || previous != row {
			return false
		}
	}
	return true
}

func validateInventoryStateDelta(before, after map[string]characterTableSnapshot, allowPetFeedProgress bool) error {
	beforeInventory, hasBeforeInventory := before["character_inventory"]
	afterInventory, hasAfterInventory := after["character_inventory"]
	beforeInstances, hasBeforeInstances := before["inventory_item_instances"]
	afterInstances, hasAfterInstances := after["inventory_item_instances"]
	if !hasBeforeInventory && !hasAfterInventory && !hasBeforeInstances && !hasAfterInstances {
		return nil
	}
	if !hasBeforeInventory || !hasAfterInventory || !hasBeforeInstances || !hasAfterInstances {
		return fmt.Errorf("inventory state delta is missing a before/after table snapshot")
	}
	if !danglingInventoryLinksUnchanged(beforeInventory.InventoryRows, afterInventory.InventoryRows) {
		return fmt.Errorf("inventory reference without an item GUID changed outside the source inner-join loader")
	}
	linksBefore, err := inventoryRowsByGUID(beforeInventory.InventoryRows, "character_inventory")
	if err != nil {
		return err
	}
	linksAfter, err := inventoryRowsByGUID(afterInventory.InventoryRows, "character_inventory")
	if err != nil {
		return err
	}
	itemsBefore, err := inventoryRowsByGUID(beforeInstances.InventoryRows, "inventory_item_instances")
	if err != nil {
		return err
	}
	itemsAfter, err := inventoryRowsByGUID(afterInstances.InventoryRows, "inventory_item_instances")
	if err != nil {
		return err
	}
	removed := make(map[int64]struct{})
	feedUsed := false
	for itemGUID, beforeLink := range linksBefore {
		afterLink, linked := linksAfter[itemGUID]
		beforeItem, hasBeforeItem := itemsBefore[itemGUID]
		afterItem, hasAfterItem := itemsAfter[itemGUID]
		if linked {
			if beforeLink.OwnerGUID != afterLink.OwnerGUID || beforeLink.Bag != afterLink.Bag || beforeLink.Slot != afterLink.Slot || beforeLink.Digest != afterLink.Digest {
				return fmt.Errorf("inventory link for item GUID %d changed", itemGUID)
			}
			if !hasBeforeItem && !hasAfterItem {
				continue
			}
			if !hasBeforeItem || !hasAfterItem {
				return fmt.Errorf("inventory item GUID %d lost its instance row", itemGUID)
			}
			if beforeItem.Digest != afterItem.Digest {
				if allowPetFeedProgress && !feedUsed && beforeItem.Count > 1 && afterItem.Count == beforeItem.Count-1 && beforeItem.DigestWithoutCount == afterItem.DigestWithoutCount {
					feedUsed = true
					continue
				}
				return fmt.Errorf("inventory instance row for item GUID %d changed outside one-unit pet feeding", itemGUID)
			}
			continue
		}
		if hasAfterItem {
			return fmt.Errorf("inventory link for item GUID %d was removed while its instance remained", itemGUID)
		}
		if beforeLink.Bag == 0 && beforeLink.Slot >= 74 && beforeLink.Slot <= 85 {
			removed[itemGUID] = struct{}{}
			continue
		}
		if allowPetFeedProgress && !feedUsed && hasBeforeItem && beforeItem.Count == 1 {
			feedUsed = true
			removed[itemGUID] = struct{}{}
			continue
		}
		if hasBeforeItem && (beforeItem.ItemEntry <= 0 || beforeItem.Count <= 0 || !beforeItem.ItemTemplateValid) {
			removed[itemGUID] = struct{}{}
			continue
		}
		return fmt.Errorf("valid inventory item GUID %d was deleted outside buyback cleanup or pet feeding", itemGUID)
	}
	for itemGUID := range linksAfter {
		if _, existed := linksBefore[itemGUID]; !existed {
			return fmt.Errorf("new inventory link for item GUID %d was created", itemGUID)
		}
	}
	for itemGUID := range itemsBefore {
		if _, exists := itemsAfter[itemGUID]; exists {
			continue
		}
		if _, allowed := removed[itemGUID]; !allowed {
			return fmt.Errorf("inventory instance row for item GUID %d was deleted without its link", itemGUID)
		}
	}
	for itemGUID := range itemsAfter {
		if _, existed := itemsBefore[itemGUID]; !existed {
			return fmt.Errorf("new inventory instance row for item GUID %d was created", itemGUID)
		}
	}
	return nil
}
