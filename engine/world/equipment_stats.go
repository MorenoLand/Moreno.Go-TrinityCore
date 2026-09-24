package world

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

type equippedItemStats struct {
	Slot           int64
	Armor          int64
	Block          int64
	Delay          int64
	MinDamage      float64
	MaxDamage      float64
	StatTypes      [10]int64
	StatValues     [10]int64
	Enchantments   string
	MaxDurability  uint32
	Durability     uint32
	RandomProperty int32
	ItemLevel      uint32
	Quality        uint32
	InventoryType  uint32
	RandomSuffix   uint32
	SocketColors   [3]uint32
}

const (
	itemEnchantmentTypeStat = 5
	itemModMana             = 0
	itemModHealth           = 1
	itemModSpellPower       = 45
)

func (s *session) loadEquippedItemStats(ctx context.Context, state *playerState) ([]equippedItemStats, error) {
	if s == nil || s.server == nil || state == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.slot, ii.itemEntry, COALESCE(ii.enchantments, ''), COALESCE(ii.durability, 0), COALESCE(ii.randomPropertyId, 0)
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < 19`, state.GUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	template, err := s.server.WorldStore.DB.PrepareContext(ctx, `SELECT armor, block, delay, dmg_min1, dmg_max1,
		stat_type1, stat_value1, stat_type2, stat_value2, stat_type3, stat_value3, stat_type4, stat_value4,
		stat_type5, stat_value5, stat_type6, stat_value6, stat_type7, stat_value7, stat_type8, stat_value8,
		stat_type9, stat_value9, stat_type10, stat_value10, MaxDurability, ItemLevel, Quality, InventoryType, RandomSuffix, SocketColor_1, SocketColor_2, SocketColor_3 FROM item_template WHERE entry = ?`)
	if err != nil {
		return nil, err
	}
	defer template.Close()
	var items []equippedItemStats
	for rows.Next() {
		var item equippedItemStats
		var entry uint32
		if err := rows.Scan(&item.Slot, &entry, &item.Enchantments, &item.Durability, &item.RandomProperty); err != nil {
			return nil, err
		}
		err := template.QueryRowContext(ctx, entry).Scan(&item.Armor, &item.Block, &item.Delay, &item.MinDamage, &item.MaxDamage,
			&item.StatTypes[0], &item.StatValues[0], &item.StatTypes[1], &item.StatValues[1], &item.StatTypes[2], &item.StatValues[2], &item.StatTypes[3], &item.StatValues[3],
			&item.StatTypes[4], &item.StatValues[4], &item.StatTypes[5], &item.StatValues[5], &item.StatTypes[6], &item.StatValues[6], &item.StatTypes[7], &item.StatValues[7],
			&item.StatTypes[8], &item.StatValues[8], &item.StatTypes[9], &item.StatValues[9], &item.MaxDurability, &item.ItemLevel, &item.Quality, &item.InventoryType, &item.RandomSuffix, &item.SocketColors[0], &item.SocketColors[1], &item.SocketColors[2])
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func ResolveEquippedSpellPowerEnchant(enchantment wotlk.SpellItemEnchantmentEntry, level, requiredSkillValue uint32, broken bool) uint32 {
	return ResolveEquippedItemStatEnchant(enchantment, itemModSpellPower, level, requiredSkillValue, broken)
}

func ResolveRandomSuffixSpellPowerEnchant(enchantment wotlk.SpellItemEnchantmentEntry, level, requiredSkillValue uint32, broken bool, suffixAmount uint32) uint32 {
	return ResolveRandomSuffixItemStatEnchant(enchantment, itemModSpellPower, level, requiredSkillValue, broken, suffixAmount)
}

func ResolveGemSocketEnchantActive(socketColor uint32, prismatic wotlk.SpellItemEnchantmentEntry, prismaticFound bool, requiredSkillValue uint32) bool {
	return socketColor != 0 || prismaticFound && (prismatic.RequiredSkillID == 0 || requiredSkillValue >= prismatic.RequiredSkillRank)
}

func ResolveEquippedItemStatEnchant(enchantment wotlk.SpellItemEnchantmentEntry, itemMod, level, requiredSkillValue uint32, broken bool) uint32 {
	return resolveItemStatEnchant(enchantment, itemMod, level, requiredSkillValue, broken, 0, false)
}

func ResolveRandomSuffixItemStatEnchant(enchantment wotlk.SpellItemEnchantmentEntry, itemMod, level, requiredSkillValue uint32, broken bool, suffixAmount uint32) uint32 {
	return resolveItemStatEnchant(enchantment, itemMod, level, requiredSkillValue, broken, suffixAmount, true)
}

func resolveItemStatEnchant(enchantment wotlk.SpellItemEnchantmentEntry, itemMod, level, requiredSkillValue uint32, broken bool, suffixAmount uint32, suffix bool) uint32 {
	if broken || enchantment.ConditionID != 0 || level < enchantment.MinLevel || requiredSkillValue < enchantment.RequiredSkillRank {
		return 0
	}
	var amount uint32
	for index, effect := range enchantment.Effects {
		if effect == itemEnchantmentTypeStat && enchantment.EffectArg[index] == itemMod {
			value := enchantment.EffectPointsMin[index]
			if value == 0 && suffix {
				value = suffixAmount
			}
			amount += value
		}
	}
	return amount
}

func ResolveItemSuffixFactor(data *wotlk.Store, itemLevel, quality, inventoryType, randomSuffix uint32) uint32 {
	if randomSuffix == 0 || data == nil {
		return 0
	}
	points, found, err := data.RandPropPoints(itemLevel)
	if err != nil || !found {
		return 0
	}
	index := -1
	switch inventoryType {
	case 1, 4, 5, 7, 17, 20:
		index = 0
	case 3, 6, 8, 10, 12:
		index = 1
	case 2, 9, 11, 14, 16, 23:
		index = 2
	case 13, 21, 22:
		index = 3
	case 15, 25, 26:
		index = 4
	}
	if index < 0 {
		return 0
	}
	switch quality {
	case 2:
		return points.Good[index]
	case 3:
		return points.Superior[index]
	case 4:
		return points.Epic[index]
	default:
		return 0
	}
}

func (s *session) applyPlayerItemStatEnchants(state *playerState, item equippedItemStats) error {
	if s == nil || s.server == nil || s.server.Data == nil || state == nil {
		return nil
	}
	broken := item.MaxDurability > 0 && item.Durability == 0
	fields := strings.Fields(item.Enchantments)
	for _, index := range []int{0, 3} {
		if index >= len(fields) {
			continue
		}
		enchantID, err := strconv.ParseUint(fields[index], 10, 32)
		if err != nil || enchantID == 0 {
			continue
		}
		if err := s.applyItemStatEnchantment(state, uint32(enchantID), broken, 0, false); err != nil {
			return err
		}
	}
	var prismatic wotlk.SpellItemEnchantmentEntry
	var prismaticFound bool
	if len(fields) > 18 {
		if enchantID, err := strconv.ParseUint(fields[18], 10, 32); err == nil && enchantID != 0 {
			var err error
			prismatic, prismaticFound, err = s.server.Data.SpellItemEnchantment(uint32(enchantID))
			if err != nil {
				return err
			}
		}
	}
	for socket := range item.SocketColors {
		fieldIndex := (socket + 2) * 3
		if fieldIndex >= len(fields) {
			continue
		}
		enchantID, err := strconv.ParseUint(fields[fieldIndex], 10, 32)
		if err != nil || enchantID == 0 {
			continue
		}
		if !ResolveGemSocketEnchantActive(item.SocketColors[socket], prismatic, prismaticFound, playerSkillValue(state, prismatic.RequiredSkillID)) {
			continue
		}
		if err := s.applyItemStatEnchantment(state, uint32(enchantID), broken, 0, false); err != nil {
			return err
		}
	}
	if item.RandomProperty > 0 {
		property, found, err := s.server.Data.ItemRandomProperties(uint32(item.RandomProperty))
		if err != nil {
			return err
		}
		if found {
			for _, enchantID := range property.Enchantment {
				if enchantID != 0 {
					if err := s.applyItemStatEnchantment(state, enchantID, broken, 0, false); err != nil {
						return err
					}
				}
			}
		}
	} else if item.RandomProperty < 0 {
		suffixID := uint32(-int64(item.RandomProperty))
		suffix, found, err := s.server.Data.ItemRandomSuffix(suffixID)
		if err != nil {
			return err
		}
		if found {
			factor := ResolveItemSuffixFactor(s.server.Data, item.ItemLevel, item.Quality, item.InventoryType, item.RandomSuffix)
			for index, enchantID := range suffix.Enchantment {
				if enchantID == 0 || factor == 0 {
					continue
				}
				amount := suffix.AllocationPct[index] * factor / 10000
				if err := s.applyItemStatEnchantment(state, enchantID, broken, amount, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *session) applyItemStatEnchantment(state *playerState, enchantID uint32, broken bool, suffixAmount uint32, suffix bool) error {
	entry, found, err := s.server.Data.SpellItemEnchantment(enchantID)
	if err != nil || !found {
		return err
	}
	skillValue := playerSkillValue(state, entry.RequiredSkillID)
	for _, itemMod := range []uint32{itemModMana, itemModHealth, itemModSpellPower} {
		amount := ResolveEquippedItemStatEnchant(entry, itemMod, uint32(state.Level), skillValue, broken)
		if suffix {
			amount = ResolveRandomSuffixItemStatEnchant(entry, itemMod, uint32(state.Level), skillValue, broken, suffixAmount)
		}
		if amount == 0 {
			continue
		}
		switch itemMod {
		case itemModMana:
			state.ItemManaBonus += amount
		case itemModHealth:
			state.ItemHealthBonus += amount
		case itemModSpellPower:
			state.BaseSpellPower += amount
			state.SpellPower += amount
			s.setAchievementCriteria(criteriaTypeHighestSpellpower, 0, state.SpellPower)
		}
	}
	return nil
}

func playerSkillValue(state *playerState, skillID uint32) uint32 {
	for _, skill := range state.Skills {
		if uint32(skill.Skill) == skillID {
			return uint32(skill.Value)
		}
	}
	return 0
}
