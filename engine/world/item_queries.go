package world

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	itemStats   = 10
	itemDamages = 2
	itemSpells  = 5
	itemSockets = 3
)

type itemStatQueryData struct {
	Type  uint32
	Value int32
}

type itemDamageQueryData struct {
	Min  float32
	Max  float32
	Type uint32
}

type itemSpellQueryData struct {
	ID               int32
	Trigger          uint32
	Charges          int32
	Cooldown         int32
	Category         uint32
	CategoryCooldown int32
}

type itemSocketQueryData struct {
	Color   uint32
	Content uint32
}

type itemQueryData struct {
	Entry                     uint32
	Class                     uint32
	SubClass                  uint32
	SoundOverrideSubclass     int32
	Name                      string
	DisplayInfoID             uint32
	Quality                   uint32
	Flags                     uint32
	Flags2                    uint32
	BuyPrice                  int32
	SellPrice                 uint32
	InventoryType             uint32
	AllowableClass            uint32
	AllowableRace             uint32
	ItemLevel                 uint32
	RequiredLevel             uint32
	RequiredSkill             uint32
	RequiredSkillRank         uint32
	RequiredSpell             uint32
	RequiredHonorRank         uint32
	RequiredCityRank          uint32
	RequiredReputationFaction uint32
	RequiredReputationRank    uint32
	MaxCount                  int32
	Stackable                 int32
	ContainerSlots            uint32
	StatsCount                uint32
	Stats                     [itemStats]itemStatQueryData
	ScalingStatDistribution   uint32
	ScalingStatValue          uint32
	Damage                    [itemDamages]itemDamageQueryData
	Resistance                [7]uint32
	Delay                     uint32
	AmmoType                  uint32
	RangedModRange            float32
	Spells                    [itemSpells]itemSpellQueryData
	Bonding                   uint32
	Description               string
	PageText                  uint32
	LanguageID                uint32
	PageMaterial              uint32
	StartQuest                uint32
	LockID                    uint32
	Material                  int32
	Sheath                    uint32
	RandomProperty            int32
	RandomSuffix              int32
	Block                     uint32
	ItemSet                   uint32
	MaxDurability             uint32
	Area                      uint32
	Map                       uint32
	BagFamily                 uint32
	TotemCategory             uint32
	Sockets                   [itemSockets]itemSocketQueryData
	SocketBonus               uint32
	GemProperties             uint32
	RequiredDisenchantSkill   uint32
	ArmorDamageModifier       float32
	Duration                  uint32
	ItemLimitCategory         uint32
	HolidayID                 uint32
	// FlagsCustom mirrors item_template.flagsCustom (ItemTemplate.h:225):
	// the template custom flags (IGNORE_QUEST_STATUS, FOLLOW_LOOT_RULES)
	// the loot fill carries per row. Read live, never stored.
	FlagsCustom uint32
}

func (s *session) handleItemQuerySingle(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	entry, err := reader.ReadU32()
	if err != nil {
		s.debug("item query rejected", "account", s.accountName, "error", err)
		return false
	}
	data, err := s.loadItemQueryData(ctx, entry)
	if errors.Is(err, sql.ErrNoRows) {
		s.debug("item query unknown", "account", s.accountName, "entry", entry)
		return s.write(uint16(protocol.OpcodeSMSG_ITEM_QUERY_SINGLE_RESPONSE), buildItemQueryResponse(itemQueryData{Entry: entry}, false), true) == nil
	}
	if err != nil {
		s.debug("item query failed", "account", s.accountName, "entry", entry, "error", err)
		return false
	}
	s.debug("item query response", "account", s.accountName, "entry", entry, "name", data.Name)
	return s.write(uint16(protocol.OpcodeSMSG_ITEM_QUERY_SINGLE_RESPONSE), buildItemQueryResponse(data, true), true) == nil
}

func itemQueryColumns() []string {
	columns := []string{"class", "subclass", "SoundOverrideSubclass", "name", "displayid", "Quality", "Flags", "FlagsExtra", "BuyPrice", "SellPrice", "InventoryType", "AllowableClass", "AllowableRace", "ItemLevel", "RequiredLevel", "RequiredSkill", "RequiredSkillRank", "requiredspell", "requiredhonorrank", "RequiredCityRank", "RequiredReputationFaction", "RequiredReputationRank", "maxcount", "stackable", "ContainerSlots", "StatsCount"}
	for index := 1; index <= itemStats; index++ {
		columns = append(columns, "stat_type"+strconv.Itoa(index), "stat_value"+strconv.Itoa(index))
	}
	columns = append(columns, "ScalingStatDistribution", "ScalingStatValue", "dmg_min1", "dmg_max1", "dmg_type1", "dmg_min2", "dmg_max2", "dmg_type2", "armor", "holy_res", "fire_res", "nature_res", "frost_res", "shadow_res", "arcane_res", "delay", "ammo_type", "RangedModRange")
	for index := 1; index <= itemSpells; index++ {
		columns = append(columns, "spellid_"+strconv.Itoa(index), "spelltrigger_"+strconv.Itoa(index), "spellcharges_"+strconv.Itoa(index), "spellcooldown_"+strconv.Itoa(index), "spellcategory_"+strconv.Itoa(index), "spellcategorycooldown_"+strconv.Itoa(index))
	}
	columns = append(columns, "bonding", "description", "PageText", "LanguageID", "PageMaterial", "startquest", "lockid", "Material", "sheath", "RandomProperty", "RandomSuffix", "block", "itemset", "MaxDurability", "area", "Map", "BagFamily", "TotemCategory")
	for index := 1; index <= itemSockets; index++ {
		columns = append(columns, "socketColor_"+strconv.Itoa(index), "socketContent_"+strconv.Itoa(index))
	}
	columns = append(columns, "socketBonus", "GemProperties", "RequiredDisenchantSkill", "ArmorDamageModifier", "duration", "ItemLimitCategory", "HolidayId", "flagsCustom")
	return columns
}

func (s *session) loadItemQueryData(ctx context.Context, entry uint32) (itemQueryData, error) {
	var data itemQueryData
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return data, errors.New("world store unavailable")
	}
	var class, subclass, sound, display, quality, flags, flags2, buyPrice, sellPrice, inventoryType, allowableClass, allowableRace, itemLevel, requiredLevel, requiredSkill, requiredSkillRank, requiredSpell, requiredHonorRank, requiredCityRank, requiredReputationFaction, requiredReputationRank, maxCount, stackable, containerSlots, statsCount int64
	var name, description string
	statTypes := make([]int64, itemStats)
	statValues := make([]int64, itemStats)
	var scalingDistribution, scalingValue int64
	damageMin := make([]float64, itemDamages)
	damageMax := make([]float64, itemDamages)
	damageTypes := make([]int64, itemDamages)
	resistance := make([]int64, len(data.Resistance))
	var delay, ammoType int64
	var rangedModRange float64
	spellIDs := make([]int64, itemSpells)
	spellTriggers := make([]int64, itemSpells)
	spellCharges := make([]int64, itemSpells)
	spellCooldowns := make([]int64, itemSpells)
	spellCategories := make([]int64, itemSpells)
	spellCategoryCooldowns := make([]int64, itemSpells)
	var bonding, pageText, languageID, pageMaterial, startQuest, lockID, sheath, block, itemSet, maxDurability, area, mapID, bagFamily, totemCategory int64
	var material, randomProperty, randomSuffix int64
	socketColors := make([]int64, itemSockets)
	socketContents := make([]int64, itemSockets)
	var socketBonus, gemProperties, requiredDisenchantSkill, duration, itemLimitCategory, holidayID, flagsCustom int64
	var armorDamageModifier float64
	targets := []any{&class, &subclass, &sound, &name, &display, &quality, &flags, &flags2, &buyPrice, &sellPrice, &inventoryType, &allowableClass, &allowableRace, &itemLevel, &requiredLevel, &requiredSkill, &requiredSkillRank, &requiredSpell, &requiredHonorRank, &requiredCityRank, &requiredReputationFaction, &requiredReputationRank, &maxCount, &stackable, &containerSlots, &statsCount}
	for index := range statTypes {
		targets = append(targets, &statTypes[index], &statValues[index])
	}
	targets = append(targets, &scalingDistribution, &scalingValue)
	for index := range damageMin {
		targets = append(targets, &damageMin[index], &damageMax[index], &damageTypes[index])
	}
	for index := range resistance {
		targets = append(targets, &resistance[index])
	}
	targets = append(targets, &delay, &ammoType, &rangedModRange)
	for index := range spellIDs {
		targets = append(targets, &spellIDs[index], &spellTriggers[index], &spellCharges[index], &spellCooldowns[index], &spellCategories[index], &spellCategoryCooldowns[index])
	}
	targets = append(targets, &bonding, &description, &pageText, &languageID, &pageMaterial, &startQuest, &lockID, &material, &sheath, &randomProperty, &randomSuffix, &block, &itemSet, &maxDurability, &area, &mapID, &bagFamily, &totemCategory)
	for index := range socketColors {
		targets = append(targets, &socketColors[index], &socketContents[index])
	}
	targets = append(targets, &socketBonus, &gemProperties, &requiredDisenchantSkill, &armorDamageModifier, &duration, &itemLimitCategory, &holidayID, &flagsCustom)
	query := "SELECT " + strings.Join(itemQueryColumns(), ", ") + " FROM item_template WHERE entry = ?"
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, query, entry).Scan(targets...); err != nil {
		return data, err
	}
	data.Entry, data.Class, data.SubClass, data.SoundOverrideSubclass, data.Name = entry, uint32(class), uint32(subclass), int32(sound), name
	data.DisplayInfoID, data.Quality, data.Flags, data.Flags2, data.BuyPrice, data.SellPrice = uint32(display), uint32(quality), uint32(flags), uint32(flags2), int32(buyPrice), uint32(sellPrice)
	data.InventoryType, data.AllowableClass, data.AllowableRace, data.ItemLevel, data.RequiredLevel = uint32(inventoryType), uint32(allowableClass), uint32(allowableRace), uint32(itemLevel), uint32(requiredLevel)
	data.RequiredSkill, data.RequiredSkillRank, data.RequiredSpell, data.RequiredHonorRank, data.RequiredCityRank = uint32(requiredSkill), uint32(requiredSkillRank), uint32(requiredSpell), uint32(requiredHonorRank), uint32(requiredCityRank)
	data.RequiredReputationFaction, data.RequiredReputationRank, data.MaxCount, data.Stackable, data.ContainerSlots = uint32(requiredReputationFaction), uint32(requiredReputationRank), int32(maxCount), int32(stackable), uint32(containerSlots)
	if statsCount < 0 {
		statsCount = 0
	}
	if statsCount > itemStats {
		statsCount = itemStats
	}
	data.StatsCount = uint32(statsCount)
	for index := 0; index < int(statsCount); index++ {
		data.Stats[index] = itemStatQueryData{Type: uint32(statTypes[index]), Value: int32(statValues[index])}
	}
	data.ScalingStatDistribution, data.ScalingStatValue = uint32(scalingDistribution), uint32(scalingValue)
	for index := range data.Damage {
		data.Damage[index] = itemDamageQueryData{Min: float32(damageMin[index]), Max: float32(damageMax[index]), Type: uint32(damageTypes[index])}
	}
	for index := range data.Resistance {
		data.Resistance[index] = uint32(resistance[index])
	}
	data.Delay, data.AmmoType, data.RangedModRange = uint32(delay), uint32(ammoType), float32(rangedModRange)
	for index := range data.Spells {
		data.Spells[index] = itemSpellQueryData{ID: int32(spellIDs[index]), Trigger: uint32(spellTriggers[index]), Charges: int32(spellCharges[index]), Cooldown: int32(spellCooldowns[index]), Category: uint32(spellCategories[index]), CategoryCooldown: int32(spellCategoryCooldowns[index])}
	}
	data.Bonding, data.Description, data.PageText, data.LanguageID, data.PageMaterial = uint32(bonding), description, uint32(pageText), uint32(languageID), uint32(pageMaterial)
	data.StartQuest, data.LockID, data.Material, data.Sheath, data.RandomProperty, data.RandomSuffix = uint32(startQuest), uint32(lockID), int32(material), uint32(sheath), int32(randomProperty), int32(randomSuffix)
	data.Block, data.ItemSet, data.MaxDurability, data.Area, data.Map, data.BagFamily, data.TotemCategory = uint32(block), uint32(itemSet), uint32(maxDurability), uint32(area), uint32(mapID), uint32(bagFamily), uint32(totemCategory)
	for index := range data.Sockets {
		data.Sockets[index] = itemSocketQueryData{Color: uint32(socketColors[index]), Content: uint32(socketContents[index])}
	}
	data.SocketBonus, data.GemProperties, data.RequiredDisenchantSkill, data.ArmorDamageModifier, data.Duration, data.ItemLimitCategory, data.HolidayID = uint32(socketBonus), uint32(gemProperties), uint32(requiredDisenchantSkill), float32(armorDamageModifier), uint32(duration), uint32(itemLimitCategory), uint32(holidayID)
	data.FlagsCustom = uint32(flagsCustom)
	return data, nil
}

func buildItemQueryResponse(data itemQueryData, allow bool) []byte {
	packet := protocol.NewBuffer(512)
	entry := data.Entry
	if !allow {
		entry |= 0x80000000
	}
	packet.WriteU32(entry)
	if !allow {
		return packet.Bytes()
	}
	packet.WriteU32(data.Class)
	packet.WriteU32(data.SubClass)
	packet.WriteI32(data.SoundOverrideSubclass)
	packet.WriteCString(data.Name)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU32(data.DisplayInfoID)
	packet.WriteU32(data.Quality)
	packet.WriteU32(data.Flags)
	packet.WriteU32(data.Flags2)
	packet.WriteI32(data.BuyPrice)
	packet.WriteU32(data.SellPrice)
	packet.WriteU32(data.InventoryType)
	packet.WriteU32(data.AllowableClass)
	packet.WriteU32(data.AllowableRace)
	packet.WriteU32(data.ItemLevel)
	packet.WriteU32(data.RequiredLevel)
	packet.WriteU32(data.RequiredSkill)
	packet.WriteU32(data.RequiredSkillRank)
	packet.WriteU32(data.RequiredSpell)
	packet.WriteU32(data.RequiredHonorRank)
	packet.WriteU32(data.RequiredCityRank)
	packet.WriteU32(data.RequiredReputationFaction)
	packet.WriteU32(data.RequiredReputationRank)
	packet.WriteI32(data.MaxCount)
	packet.WriteI32(data.Stackable)
	packet.WriteU32(data.ContainerSlots)
	packet.WriteU32(data.StatsCount)
	for index := uint32(0); index < data.StatsCount && index < uint32(len(data.Stats)); index++ {
		packet.WriteU32(data.Stats[index].Type)
		packet.WriteI32(data.Stats[index].Value)
	}
	packet.WriteU32(data.ScalingStatDistribution)
	packet.WriteU32(data.ScalingStatValue)
	for _, damage := range data.Damage {
		packet.WriteF32(damage.Min)
		packet.WriteF32(damage.Max)
		packet.WriteU32(damage.Type)
	}
	for _, value := range data.Resistance {
		packet.WriteU32(value)
	}
	packet.WriteU32(data.Delay)
	packet.WriteU32(data.AmmoType)
	packet.WriteF32(data.RangedModRange)
	for _, spell := range data.Spells {
		if spell.ID > 0 {
			packet.WriteI32(spell.ID)
			packet.WriteU32(spell.Trigger)
			charges := spell.Charges
			if charges > 0 {
				charges = -charges
			}
			packet.WriteU32(uint32(charges))
			packet.WriteU32(uint32(spell.Cooldown))
			packet.WriteU32(spell.Category)
			packet.WriteU32(uint32(spell.CategoryCooldown))
			continue
		}
		packet.WriteU32(0)
		packet.WriteU32(0)
		packet.WriteU32(0)
		packet.WriteU32(^uint32(0))
		packet.WriteU32(0)
		packet.WriteU32(^uint32(0))
	}
	packet.WriteU32(data.Bonding)
	packet.WriteCString(data.Description)
	packet.WriteU32(data.PageText)
	packet.WriteU32(data.LanguageID)
	packet.WriteU32(data.PageMaterial)
	packet.WriteU32(data.StartQuest)
	packet.WriteU32(data.LockID)
	packet.WriteI32(data.Material)
	packet.WriteU32(data.Sheath)
	packet.WriteI32(data.RandomProperty)
	packet.WriteI32(data.RandomSuffix)
	packet.WriteU32(data.Block)
	packet.WriteU32(data.ItemSet)
	packet.WriteU32(data.MaxDurability)
	packet.WriteU32(data.Area)
	packet.WriteU32(data.Map)
	packet.WriteU32(data.BagFamily)
	packet.WriteU32(data.TotemCategory)
	for _, socket := range data.Sockets {
		packet.WriteU32(socket.Color)
		packet.WriteU32(socket.Content)
	}
	packet.WriteU32(data.SocketBonus)
	packet.WriteU32(data.GemProperties)
	packet.WriteU32(data.RequiredDisenchantSkill)
	packet.WriteF32(data.ArmorDamageModifier)
	packet.WriteU32(data.Duration)
	packet.WriteU32(data.ItemLimitCategory)
	packet.WriteU32(data.HolidayID)
	return packet.Bytes()
}
