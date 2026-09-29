package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// checkSpellTotemRequirements mirrors the totem requirement gates in
// Spell::CheckCast (Spell.cpp:6823-6856), which run after the reagent check:
// spells may require specific totem items in inventory (Spell.dbc fields
// 50-51, Totem) and/or items whose item-template TotemCategory is compatible
// with a required category (Spell.dbc fields 222-223, RequiredTotemCategoryID).
// Returns 0 when the cast may proceed, otherwise the SPELL_FAILED_* reason.
func (s *session) checkSpellTotemRequirements(ctx context.Context, spell wotlk.Spell) uint8 {
	// check totem-item requirements (items presence in inventory)
	totems := 2
	for i := 0; i < 2; i++ {
		if spell.Totem[i] != 0 {
			if s.playerItemCount(ctx, spell.Totem[i]) > 0 {
				totems--
				continue
			}
		} else {
			totems--
		}
	}
	if totems != 0 {
		return spellFailedTotems
	}

	// Check items for TotemCategory (items presence in inventory)
	categories := 2
	for i := 0; i < 2; i++ {
		if spell.RequiredTotemCategory[i] != 0 {
			if s.playerHasItemTotemCategory(ctx, spell.RequiredTotemCategory[i]) {
				categories--
				continue
			}
		} else {
			categories--
		}
	}
	if categories != 0 {
		return spellFailedTotemCategory
	}
	return 0
}

// playerItemCount mirrors Player::HasItemCount (Inventory.cpp) for the
// presence check: total count of the item entry across the player's inventory.
func (s *session) playerItemCount(ctx context.Context, itemID uint32) int64 {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var have int64
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count),0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, int64(itemID)).Scan(&have)
	return have
}

// playerHasItemTotemCategory mirrors Player::HasItemTotemCategory
// (Player.cpp:10478): any usable inventory item whose item-template
// TotemCategory is compatible with the required category.
func (s *session) playerHasItemTotemCategory(ctx context.Context, categoryID uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return false
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT DISTINCT ii.itemEntry FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ?`, s.playerGUID)
	if err != nil {
		return false
	}
	var entries []int64
	for rows.Next() {
		var entry int64
		if err := rows.Scan(&entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	rows.Close()
	for _, entry := range entries {
		data, err := s.loadItemQueryData(ctx, uint32(entry))
		if err != nil || data.TotemCategory == 0 {
			continue
		}
		if s.totemCategoryCompatibleWith(data.TotemCategory, categoryID) {
			return true
		}
	}
	return false
}

// totemCategoryCompatibleWith mirrors IsTotemCategoryCompatiableWith
// (DBCStores.cpp:756): same TotemCategoryType, and the item mask must cover
// every bit of the required mask.
func (s *session) totemCategoryCompatibleWith(itemCategoryID, requiredCategoryID uint32) bool {
	if requiredCategoryID == 0 {
		return true
	}
	if itemCategoryID == 0 {
		return false
	}
	itemCat, itemFound, itemErr := s.server.Data.TotemCategory(itemCategoryID)
	reqCat, reqFound, reqErr := s.server.Data.TotemCategory(requiredCategoryID)
	if itemErr != nil || reqErr != nil || !itemFound || !reqFound {
		return false
	}
	if itemCat.TotemCategoryType != reqCat.TotemCategoryType {
		return false
	}
	return (itemCat.TotemCategoryMask & reqCat.TotemCategoryMask) == reqCat.TotemCategoryMask
}
