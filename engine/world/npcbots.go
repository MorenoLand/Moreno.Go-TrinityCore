package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

const (
	BotInventorySize          = 18
	BotRoleTank               = uint16(0x001)
	BotRoleDPS                = uint16(0x002)
	BotRoleHeal               = uint16(0x004)
	BotRoleRanged             = uint16(0x008)
	BotClassBlademaster       = uint8(12)
	BotClassObsidianDestroyer = uint8(13)
	BotClassArchmage          = uint8(14)
	BotClassDreadlord         = uint8(15)
	BotClassSpellbreaker      = uint8(16)
	BotClassDarkRanger        = uint8(17)
)

type BotAssignResult uint16

const (
	BotAddDisabled         BotAssignResult = 0x001
	BotAddAlreadyHave      BotAssignResult = 0x002
	BotAddMaxExceeded      BotAssignResult = 0x004
	BotAddMaxClassExceeded BotAssignResult = 0x008
	BotAddCannotAfford     BotAssignResult = 0x010
	BotAddInstanceLimit    BotAssignResult = 0x020
	BotAddBusy             BotAssignResult = 0x040
	BotAddNotAvailable     BotAssignResult = 0x080
	BotAddSuccess          BotAssignResult = 0x100
)

type NpcBotUpdateType uint8

const (
	NpcBotUpdateOwner NpcBotUpdateType = iota + 1
	NpcBotUpdateRoles
	NpcBotUpdateSpec
	NpcBotUpdateDisabledSpells
	NpcBotUpdateFaction
	NpcBotUpdateEquips
	NpcBotUpdateErase
)

type NpcBotData struct {
	Entry          uint32
	Owner          uint32
	Roles          uint16
	Spec           uint8
	Faction        uint32
	Equips         [BotInventorySize]uint32
	DisabledSpells []uint32
}

type NpcBotAppearanceData struct {
	Entry     uint32
	Gender    uint8
	Skin      uint8
	Face      uint8
	Hair      uint8
	HairColor uint8
	Features  uint8
}

type NpcBotExtras struct {
	Entry uint32
	Class uint8
	Race  uint8
}

type NPCBotManager struct {
	characters *database.Store
	world      *database.Store
	config     config.NPCBotConfig
	mu         sync.RWMutex
	loaded     bool
	bots       map[uint32]NpcBotData
	appearance map[uint32]NpcBotAppearanceData
	extras     map[uint32]NpcBotExtras
}

func NewNPCBotManager(characters, world *database.Store, cfg config.NPCBotConfig) *NPCBotManager {
	return &NPCBotManager{characters: characters, world: world, config: cfg, bots: make(map[uint32]NpcBotData), appearance: make(map[uint32]NpcBotAppearanceData), extras: make(map[uint32]NpcBotExtras)}
}

func (m *NPCBotManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	if m.loaded {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	if !m.config.Enable {
		m.mu.Lock()
		m.loaded = true
		m.mu.Unlock()
		return nil
	}
	appearance, err := loadNpcBotAppearance(ctx, m.world)
	if err != nil {
		return err
	}
	extras, err := loadNpcBotExtras(ctx, m.world)
	if err != nil {
		return err
	}
	bots, err := loadNpcBotData(ctx, m.characters)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.appearance = appearance
	m.extras = extras
	m.bots = bots
	m.loaded = true
	m.mu.Unlock()
	return nil
}

func (m *NPCBotManager) Loaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded
}

func (m *NPCBotManager) Config() config.NPCBotConfig {
	return m.config
}

func (m *NPCBotManager) Get(entry uint32) (NpcBotData, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.bots[entry]
	if !ok {
		return NpcBotData{}, false
	}
	data.DisabledSpells = append([]uint32(nil), data.DisabledSpells...)
	return data, true
}

func (m *NPCBotManager) Appearance(entry uint32) (NpcBotAppearanceData, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.appearance[entry]
	return data, ok
}

func (m *NPCBotManager) Extras(entry uint32) (NpcBotExtras, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.extras[entry]
	return data, ok
}

func (m *NPCBotManager) Snapshot() []NpcBotData {
	m.mu.RLock()
	result := make([]NpcBotData, 0, len(m.bots))
	for _, data := range m.bots {
		data.DisabledSpells = append([]uint32(nil), data.DisabledSpells...)
		result = append(result, data)
	}
	m.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].Entry < result[j].Entry })
	return result
}

func (m *NPCBotManager) Add(ctx context.Context, entry uint32, roles uint16, spec uint8, faction uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.bots[entry]; exists {
		return fmt.Errorf("npcbot %d already exists", entry)
	}
	if _, err := m.characters.ExecStatement(ctx, "CHAR_INS_NPCBOT", entry, roles, spec, faction); err != nil {
		return err
	}
	m.bots[entry] = NpcBotData{Entry: entry, Roles: roles, Spec: spec, Faction: faction}
	return nil
}

func (m *NPCBotManager) Update(ctx context.Context, entry uint32, kind NpcBotUpdateType, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.bots[entry]
	if !ok {
		return fmt.Errorf("npcbot %d not found", entry)
	}
	switch kind {
	case NpcBotUpdateOwner:
		owner, err := asUint32(value)
		if err != nil {
			return err
		}
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_OWNER", owner, entry); err != nil {
			return err
		}
		data.Owner = owner
	case NpcBotUpdateRoles:
		roles, err := asUint16(value)
		if err != nil {
			return err
		}
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_ROLES", roles, entry); err != nil {
			return err
		}
		data.Roles = roles
	case NpcBotUpdateSpec:
		spec, err := asUint8(value)
		if err != nil {
			return err
		}
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_SPEC", spec, entry); err != nil {
			return err
		}
		data.Spec = spec
	case NpcBotUpdateFaction:
		faction, err := asUint32(value)
		if err != nil {
			return err
		}
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_FACTION", faction, entry); err != nil {
			return err
		}
		data.Faction = faction
	case NpcBotUpdateDisabledSpells:
		spells, err := asSpellList(value)
		if err != nil {
			return err
		}
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_DISABLED_SPELLS", formatSpellList(spells), entry); err != nil {
			return err
		}
		data.DisabledSpells = spells
	case NpcBotUpdateEquips:
		equips, err := asEquips(value)
		if err != nil {
			return err
		}
		args := make([]any, 0, BotInventorySize+1)
		for _, equip := range equips {
			args = append(args, equip)
		}
		args = append(args, entry)
		if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_EQUIP", args...); err != nil {
			return err
		}
		data.Equips = equips
	case NpcBotUpdateErase:
		if _, err := m.characters.ExecStatement(ctx, "CHAR_DEL_NPCBOT", entry); err != nil {
			return err
		}
		delete(m.bots, entry)
		return nil
	default:
		return fmt.Errorf("unsupported npcbot update type %d", kind)
	}
	m.bots[entry] = data
	return nil
}

func (m *NPCBotManager) UpdateOwnerAll(ctx context.Context, previousOwner, owner uint32) error {
	if _, err := m.characters.ExecStatement(ctx, "CHAR_UPD_NPCBOT_OWNER_ALL", owner, previousOwner); err != nil {
		return err
	}
	m.mu.Lock()
	for entry, data := range m.bots {
		if data.Owner == previousOwner {
			data.Owner = owner
			m.bots[entry] = data
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *NPCBotManager) CountByOwner(owner uint32) uint8 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded {
		return 0
	}
	count := uint8(0)
	for _, data := range m.bots {
		if data.Owner == owner {
			count++
		}
	}
	return count
}

func (m *NPCBotManager) CountByRole(owner uint32, roles uint16) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, data := range m.bots {
		if data.Owner == owner && data.Roles&roles != 0 {
			count++
		}
	}
	return count
}

func (m *NPCBotManager) ClassMask(owner uint32) uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var mask uint32
	for entry, data := range m.bots {
		if data.Owner != owner {
			continue
		}
		if extra, ok := m.extras[entry]; ok && extra.Class > 0 && extra.Class <= 32 {
			mask |= uint32(1) << (extra.Class - 1)
		}
	}
	return mask
}

func (m *NPCBotManager) IsClassEnabled(class uint8) bool {
	switch class {
	case BotClassBlademaster:
		return m.config.BlademasterEnable
	case BotClassObsidianDestroyer:
		return m.config.ObsidianDestroyerEnable
	case BotClassArchmage:
		return m.config.ArchmageEnable
	case BotClassDreadlord:
		return m.config.DreadlordEnable
	case BotClassSpellbreaker:
		return m.config.SpellBreakerEnable
	case BotClassDarkRanger:
		return m.config.DarkRangerEnable
	default:
		return true
	}
}

func (m *NPCBotManager) CanAssign(owner, entry uint32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.config.Enable {
		return false
	}
	data, ok := m.bots[entry]
	if !ok || data.Owner != 0 {
		return false
	}
	if extra, ok := m.extras[entry]; ok {
		if !m.IsClassEnabled(extra.Class) {
			return false
		}
		if m.config.MaxBotsPerClass > 0 {
			count := uint32(0)
			for botEntry, bot := range m.bots {
				if bot.Owner == owner {
					if botExtra, exists := m.extras[botEntry]; exists && botExtra.Class == extra.Class {
						count++
					}
				}
			}
			if count >= m.config.MaxBotsPerClass {
				return false
			}
		}
	}
	if m.config.MaxBots == 0 {
		return true
	}
	count := uint32(0)
	for _, bot := range m.bots {
		if bot.Owner == owner {
			count++
		}
	}
	return count < m.config.MaxBots
}

func (m *NPCBotManager) Assign(ctx context.Context, owner, entry uint32) error {
	if !m.CanAssign(owner, entry) {
		return errors.New("npcbot cannot be assigned to owner")
	}
	return m.Update(ctx, entry, NpcBotUpdateOwner, owner)
}

func (m *NPCBotManager) Recruit(ctx context.Context, owner, entry uint32) (BotAssignResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.config.Enable {
		return BotAddDisabled, nil
	}
	data, ok := m.bots[entry]
	if !ok {
		return BotAddNotAvailable, nil
	}
	if data.Owner == owner && owner != 0 {
		return BotAddAlreadyHave, nil
	}
	if data.Owner != 0 {
		return BotAddNotAvailable, nil
	}
	extra, ok := m.extras[entry]
	if !ok {
		return BotAddNotAvailable, nil
	}
	if !m.classEnabled(extra.Class) {
		return BotAddNotAvailable, nil
	}
	var owned uint32
	var classOwned uint32
	for botEntry, bot := range m.bots {
		if bot.Owner != owner {
			continue
		}
		owned++
		if botExtra, exists := m.extras[botEntry]; exists && botExtra.Class == extra.Class {
			classOwned++
		}
	}
	if m.config.MaxBots > 0 && owned >= m.config.MaxBots {
		return BotAddMaxExceeded, nil
	}
	if m.config.MaxBotsPerClass > 0 && classOwned >= m.config.MaxBotsPerClass {
		return BotAddMaxClassExceeded, nil
	}
	var level, money int64
	if err := m.characters.DB.QueryRowContext(ctx, "SELECT level, money FROM characters WHERE guid = ?", owner).Scan(&level, &money); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BotAddNotAvailable, nil
		}
		return 0, err
	}
	cost := NpcBotCost(uint8(level), extra.Class, m.config.Cost)
	if uint64(money) < cost {
		return BotAddCannotAfford, nil
	}
	tx, err := m.characters.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE characters SET money = money - ? WHERE guid = ? AND money >= ?", cost, owner, cost)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		_ = tx.Rollback()
		if err != nil {
			return 0, err
		}
		return BotAddCannotAfford, nil
	}
	result, err = tx.ExecContext(ctx, "UPDATE characters_npcbot SET owner = ? WHERE entry = ? AND owner = 0", owner, entry)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		_ = tx.Rollback()
		if err != nil {
			return 0, err
		}
		return BotAddNotAvailable, nil
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	data.Owner = owner
	m.bots[entry] = data
	return BotAddSuccess, nil
}

func (m *NPCBotManager) classEnabled(class uint8) bool {
	switch class {
	case BotClassBlademaster:
		return m.config.BlademasterEnable
	case BotClassObsidianDestroyer:
		return m.config.ObsidianDestroyerEnable
	case BotClassArchmage:
		return m.config.ArchmageEnable
	case BotClassDreadlord:
		return m.config.DreadlordEnable
	case BotClassSpellbreaker:
		return m.config.SpellBreakerEnable
	case BotClassDarkRanger:
		return m.config.DarkRangerEnable
	default:
		return true
	}
}

func NpcBotCost(level, class uint8, base uint64) uint64 {
	var cost uint64
	switch {
	case level < 10:
		cost = base / 5000
	case level < 20:
		cost = base / 100
	case level < 30:
		cost = base / 20
	case level < 40:
		cost = base / 5
	default:
		cost = base * uint64(level) / 80
	}
	switch class {
	case BotClassBlademaster, BotClassArchmage, BotClassSpellbreaker:
		cost *= 2
	case BotClassObsidianDestroyer, BotClassDreadlord, BotClassDarkRanger:
		cost *= 5
	}
	return cost
}

func loadNpcBotAppearance(ctx context.Context, store *database.Store) (map[uint32]NpcBotAppearanceData, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT entry, gender, skin, face, hair, haircolor, features FROM creature_template_npcbot_appearance")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uint32]NpcBotAppearanceData)
	for rows.Next() {
		var entry, gender, skin, face, hair, hairColor, features int64
		if err := rows.Scan(&entry, &gender, &skin, &face, &hair, &hairColor, &features); err != nil {
			return nil, err
		}
		result[uint32(entry)] = NpcBotAppearanceData{Entry: uint32(entry), Gender: uint8(gender), Skin: uint8(skin), Face: uint8(face), Hair: uint8(hair), HairColor: uint8(hairColor), Features: uint8(features)}
	}
	return result, rows.Err()
}

func loadNpcBotExtras(ctx context.Context, store *database.Store) (map[uint32]NpcBotExtras, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT entry, class, race FROM creature_template_npcbot_extras")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uint32]NpcBotExtras)
	for rows.Next() {
		var entry, class, race int64
		if err := rows.Scan(&entry, &class, &race); err != nil {
			return nil, err
		}
		result[uint32(entry)] = NpcBotExtras{Entry: uint32(entry), Class: uint8(class), Race: uint8(race)}
	}
	return result, rows.Err()
}

func loadNpcBotData(ctx context.Context, store *database.Store) (map[uint32]NpcBotData, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT entry, owner, roles, spec, faction, equipMhEx, equipOhEx, equipRhEx, equipHead, equipShoulders, equipChest, equipWaist, equipLegs, equipFeet, equipWrist, equipHands, equipBack, equipBody, equipFinger1, equipFinger2, equipTrinket1, equipTrinket2, equipNeck, spells_disabled FROM characters_npcbot")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uint32]NpcBotData)
	for rows.Next() {
		var entry, owner, roles, spec, faction int64
		var equips [BotInventorySize]int64
		var disabled sql.NullString
		args := []any{&entry, &owner, &roles, &spec, &faction}
		for i := range equips {
			args = append(args, &equips[i])
		}
		args = append(args, &disabled)
		if err := rows.Scan(args...); err != nil {
			return nil, err
		}
		data := NpcBotData{Entry: uint32(entry), Owner: uint32(owner), Roles: uint16(roles), Spec: uint8(spec), Faction: uint32(faction)}
		for i, equip := range equips {
			data.Equips[i] = uint32(equip)
		}
		if disabled.Valid {
			data.DisabledSpells = parseSpellList(disabled.String)
		}
		result[data.Entry] = data
	}
	return result, rows.Err()
}

func parseSpellList(value string) []uint32 {
	values := strings.Fields(value)
	result := make([]uint32, 0, len(values))
	for _, value := range values {
		spell, err := strconv.ParseUint(value, 10, 32)
		if err == nil {
			result = append(result, uint32(spell))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func formatSpellList(spells []uint32) string {
	spells = append([]uint32(nil), spells...)
	sort.Slice(spells, func(i, j int) bool { return spells[i] < spells[j] })
	var builder strings.Builder
	for _, spell := range spells {
		builder.WriteString(strconv.FormatUint(uint64(spell), 10))
		builder.WriteByte(' ')
	}
	return builder.String()
}

func asUint8(value any) (uint8, error) {
	n, err := asUint64(value)
	if err != nil || n > 255 {
		return 0, fmt.Errorf("value is not uint8")
	}
	return uint8(n), nil
}

func asUint16(value any) (uint16, error) {
	n, err := asUint64(value)
	if err != nil || n > 65535 {
		return 0, fmt.Errorf("value is not uint16")
	}
	return uint16(n), nil
}

func asUint32(value any) (uint32, error) {
	n, err := asUint64(value)
	if err != nil || n > 4294967295 {
		return 0, fmt.Errorf("value is not uint32")
	}
	return uint32(n), nil
}

func asUint64(value any) (uint64, error) {
	switch value := value.(type) {
	case uint8:
		return uint64(value), nil
	case uint16:
		return uint64(value), nil
	case uint32:
		return uint64(value), nil
	case uint64:
		return value, nil
	case int:
		if value >= 0 {
			return uint64(value), nil
		}
	case int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case string:
		return strconv.ParseUint(value, 10, 64)
	}
	return 0, fmt.Errorf("value is not unsigned integer")
}

func asSpellList(value any) ([]uint32, error) {
	switch value := value.(type) {
	case []uint32:
		return parseSpellList(formatSpellList(value)), nil
	case string:
		return parseSpellList(value), nil
	default:
		return nil, fmt.Errorf("value is not a spell list")
	}
}

func asEquips(value any) ([BotInventorySize]uint32, error) {
	var result [BotInventorySize]uint32
	switch value := value.(type) {
	case [BotInventorySize]uint32:
		return value, nil
	case []uint32:
		if len(value) != BotInventorySize {
			return result, fmt.Errorf("equipment must contain %d entries", BotInventorySize)
		}
		copy(result[:], value)
		return result, nil
	default:
		return result, fmt.Errorf("value is not equipment")
	}
}
