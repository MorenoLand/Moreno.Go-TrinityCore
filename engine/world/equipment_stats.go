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
	Slot          int64
	Armor         int64
	Block         int64
	Delay         int64
	MinDamage     float64
	MaxDamage     float64
	StatTypes     [10]int64
	StatValues    [10]int64
	Enchantments  string
	MaxDurability uint32
	Durability    uint32
}

const (
	itemEnchantmentTypeStat = 5
	itemModSpellPower       = 45
)

func (s *session) loadEquippedItemStats(ctx context.Context, state *playerState) ([]equippedItemStats, error) {
	if s == nil || s.server == nil || state == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil, nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.slot, ii.itemEntry, COALESCE(ii.enchantments, ''), COALESCE(ii.durability, 0)
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < 19`, state.GUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	template, err := s.server.WorldStore.DB.PrepareContext(ctx, `SELECT armor, block, delay, dmg_min1, dmg_max1,
		stat_type1, stat_value1, stat_type2, stat_value2, stat_type3, stat_value3, stat_type4, stat_value4,
		stat_type5, stat_value5, stat_type6, stat_value6, stat_type7, stat_value7, stat_type8, stat_value8,
		stat_type9, stat_value9, stat_type10, stat_value10, MaxDurability FROM item_template WHERE entry = ?`)
	if err != nil {
		return nil, err
	}
	defer template.Close()
	var items []equippedItemStats
	for rows.Next() {
		var item equippedItemStats
		var entry uint32
		if err := rows.Scan(&item.Slot, &entry, &item.Enchantments, &item.Durability); err != nil {
			return nil, err
		}
		err := template.QueryRowContext(ctx, entry).Scan(&item.Armor, &item.Block, &item.Delay, &item.MinDamage, &item.MaxDamage,
			&item.StatTypes[0], &item.StatValues[0], &item.StatTypes[1], &item.StatValues[1], &item.StatTypes[2], &item.StatValues[2], &item.StatTypes[3], &item.StatValues[3],
			&item.StatTypes[4], &item.StatValues[4], &item.StatTypes[5], &item.StatValues[5], &item.StatTypes[6], &item.StatValues[6], &item.StatTypes[7], &item.StatValues[7],
			&item.StatTypes[8], &item.StatValues[8], &item.StatTypes[9], &item.StatValues[9], &item.MaxDurability)
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
	if broken || enchantment.ConditionID != 0 || level < enchantment.MinLevel || requiredSkillValue < enchantment.RequiredSkillRank {
		return 0
	}
	var amount uint32
	for index, effect := range enchantment.Effects {
		if effect == itemEnchantmentTypeStat && enchantment.EffectArg[index] == itemModSpellPower {
			amount += enchantment.EffectPointsMin[index]
		}
	}
	return amount
}

func (s *session) applyPlayerSpellPowerEnchants(state *playerState, enchantments string, broken bool) error {
	if s == nil || s.server == nil || s.server.Data == nil || state == nil || broken {
		return nil
	}
	fields := strings.Fields(enchantments)
	for _, index := range []int{0, 3} {
		if index >= len(fields) {
			continue
		}
		enchantID, err := strconv.ParseUint(fields[index], 10, 32)
		if err != nil || enchantID == 0 {
			continue
		}
		entry, found, err := s.server.Data.SpellItemEnchantment(uint32(enchantID))
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		var skillValue uint32
		for _, skill := range state.Skills {
			if uint32(skill.Skill) == entry.RequiredSkillID {
				skillValue = uint32(skill.Value)
				break
			}
		}
		if amount := ResolveEquippedSpellPowerEnchant(entry, uint32(state.Level), skillValue, broken); amount > 0 {
			state.BaseSpellPower += amount
			state.SpellPower += amount
			s.setAchievementCriteria(criteriaTypeHighestSpellpower, 0, state.SpellPower)
		}
	}
	return nil
}
