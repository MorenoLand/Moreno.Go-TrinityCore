package world

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Movement flag bits (UnitDefines.h:217-279, enum MovementFlags) used by
// the .debug moveflags arm (cs_debug.cpp:1251).
const (
	debugMoveFlagForward         = 0x00000001
	debugMoveFlagBackward        = 0x00000002
	debugMoveFlagStrafeLeft      = 0x00000004
	debugMoveFlagStrafeRight     = 0x00000008
	debugMoveFlagWalking         = 0x00000100
	debugMoveFlagDisableGravity  = 0x00000400
	debugMoveFlagRoot            = 0x00000800
	debugMoveFlagFalling         = 0x00001000
	debugMoveFlagFallingFar      = 0x00002000
	debugMoveFlagSwimming        = 0x00200000
	debugMoveFlagAscending       = 0x00400000
	debugMoveFlagDescending      = 0x00800000
	debugMoveFlagCanFly          = 0x01000000
	debugMoveFlagFlying          = 0x02000000
	debugMoveFlagSplineElevation = 0x04000000
	debugMoveFlagSplineEnabled   = 0x08000000
	debugMoveFlagWaterWalking    = 0x10000000
	debugMoveFlagFallingSlow     = 0x20000000
	debugMoveFlagHover           = 0x40000000

	debugMoveFlagMaskMoving = debugMoveFlagForward | debugMoveFlagBackward |
		debugMoveFlagStrafeLeft | debugMoveFlagStrafeRight |
		debugMoveFlagFalling | debugMoveFlagFallingFar |
		debugMoveFlagAscending | debugMoveFlagDescending |
		debugMoveFlagSplineElevation

	// FlagsWithHandlers from HandleDebugMoveflagsCommand: the mask of
	// flags with player status opcodes plus walking, swimming and
	// spline-enabled. Bits outside this set are stored raw
	// (SetUnitMovementFlags twin) rather than run through a setter.
	debugMoveFlagMaskHasPlayerStatusOpcode = debugMoveFlagDisableGravity |
		debugMoveFlagRoot | debugMoveFlagCanFly | debugMoveFlagWaterWalking |
		debugMoveFlagFallingSlow | debugMoveFlagHover
	debugMoveFlagsWithHandlers = debugMoveFlagMaskHasPlayerStatusOpcode |
		debugMoveFlagWalking | debugMoveFlagSwimming | debugMoveFlagSplineEnabled
)

// debugItemValuesCount is ITEM_END (UpdateFields.h:73: OBJECT_END 6 +
// 0x3A); containers carry CONTAINER_END (138) instead.
const (
	debugItemValuesCount      = 64
	debugContainerValuesCount = 138
)

// debugAuraStateName is one AuraStateType value with its EnumUtils constant
// name (enuminfo_SharedDefines.cpp:1899): Title and Constant are identical
// for every value, so the framework's enum prefix match runs over these.
type debugAuraStateName struct {
	name  string
	value uint32
}

var debugAuraStateNames = []debugAuraStateName{
	{"AURA_STATE_NONE", 0},
	{"AURA_STATE_DEFENSE", 1},
	{"AURA_STATE_HEALTHLESS_20_PERCENT", 2},
	{"AURA_STATE_BERSERKING", 3},
	{"AURA_STATE_FROZEN", 4},
	{"AURA_STATE_JUDGEMENT", 5},
	{"AURA_STATE_HUNTER_PARRY", 7},
	{"AURA_STATE_WARRIOR_VICTORY_RUSH", 10},
	{"AURA_STATE_FAERIE_FIRE", 12},
	{"AURA_STATE_HEALTHLESS_35_PERCENT", 13},
	{"AURA_STATE_CONFLAGRATE", 14},
	{"AURA_STATE_SWIFTMEND", 15},
	{"AURA_STATE_DEADLY_POISON", 16},
	{"AURA_STATE_ENRAGE", 17},
	{"AURA_STATE_BLEEDING", 18},
	{"AURA_STATE_UNKNOWN19", 19},
	{"AURA_STATE_UNKNOWN22", 22},
	{"AURA_STATE_HEALTH_ABOVE_75_PERCENT", 23},
}

// parseDebugAuraState mirrors the framework enum parse for AuraStateType
// (ChatCommandArgs.h:126-201): a unique case-insensitive prefix of a
// constant name wins, else the token parses as the underlying number and
// must be a valid enum value (EnumUtils::IsValid).
func parseDebugAuraState(token string) (uint32, bool) {
	lower := strings.ToLower(token)
	matches := 0
	var matched uint32
	for _, entry := range debugAuraStateNames {
		if strings.HasPrefix(strings.ToLower(entry.name), lower) {
			matches++
			matched = entry.value
		}
	}
	if matches == 1 {
		return matched, true
	}
	value, ok := parseCommandUint32(token)
	if !ok {
		return 0, false
	}
	for _, entry := range debugAuraStateNames {
		if entry.value == value {
			return value, true
		}
	}
	return 0, false
}

// debugItemRow is one item_instance row joined to its inventory position,
// the Player::GetItemByGuid (Player.cpp:9994-10024) resolution behind the
// debug item arms: the item must be in the invoker's inventory (the
// character_inventory join excludes mail attachments and other owners'
// rows, which GetItemByGuid never sees).
type debugItemRow struct {
	guid             uint64
	entry            uint32
	ownerGUID        uint64
	creatorGUID      uint64
	giftCreatorGUID  uint64
	count            uint32
	duration         uint32
	charges          string
	flags            uint32
	enchantments     string
	randomPropertyID int32
	durability       uint32
	playedTime       uint32
	containedGUID    uint64 // container item guid; 0 = bag 0
}

func (s *session) debugItemRow(ctx context.Context, guid uint32) (debugItemRow, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || guid == 0 {
		return debugItemRow{}, false
	}
	var row debugItemRow
	var owner, creator, gift, contained int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.owner_guid,
		COALESCE(ii.creatorGuid, 0), COALESCE(ii.giftCreatorGuid, 0), ii.count, COALESCE(ii.duration, 0),
		COALESCE(ii.charges, ''), ii.flags, COALESCE(ii.enchantments, ''), COALESCE(ii.randomPropertyId, 0),
		COALESCE(ii.durability, 0), COALESCE(ii.playedTime, 0), COALESCE(ci.bag, 0)
		FROM item_instance AS ii JOIN character_inventory AS ci ON ci.item = ii.guid
		WHERE ii.guid = ? AND ii.owner_guid = ? LIMIT 1`, guid, s.playerGUID).Scan(
		&row.entry, &owner, &creator, &gift, &row.count, &row.duration, &row.charges,
		&row.flags, &row.enchantments, &row.randomPropertyID, &row.durability, &row.playedTime, &contained)
	if err != nil || row.entry == 0 {
		return debugItemRow{}, false
	}
	row.guid = uint64(guid)
	row.ownerGUID = uint64(owner)
	row.creatorGUID = uint64(creator)
	row.giftCreatorGUID = uint64(gift)
	row.containedGUID = uint64(contained)
	return row, true
}

// debugItemValues builds the item's values block (UpdateFields.h:35-73)
// from the DB row plus the template, the read path behind
// buildItemCreateBlockForLocationWithState (player_state.go): guid,
// type (3, or 7 for containers), entry, scale, owner, contained,
// creator, gift creator, stack count, duration, spell charges,
// flags, the 12-slot enchantment block, property seed (the suffix
// factor, only when a random suffix applies), random property id,
// durability, max durability and played time. Containers extend the
// block to CONTAINER_END with the slot count and the contents' guids.
func (s *session) debugItemValues(ctx context.Context, row debugItemRow) []uint32 {
	var maxDurability uint32
	var templateCharges [5]int32
	var suffixFactor uint32
	var containerSlots uint32
	container := false
	if data, err := s.loadItemQueryData(ctx, row.entry); err == nil {
		container = data.ContainerSlots > 0
		containerSlots = data.ContainerSlots
		maxDurability = data.MaxDurability
		for index := range data.Spells {
			templateCharges[index] = data.Spells[index].Charges
		}
		if row.randomPropertyID < 0 {
			suffixFactor = ResolveItemSuffixFactor(s.server.Data, data.ItemLevel, data.Quality, data.InventoryType, uint32(data.RandomSuffix))
		}
	}
	length := debugItemValuesCount
	if container {
		length = debugContainerValuesCount
	}
	values := make([]uint32, length)
	fullGUID := row.guid | (uint64(0x4000) << 48)
	values[0] = uint32(fullGUID)
	values[1] = uint32(fullGUID >> 32)
	if container {
		values[2] = 0x07 // TYPEMASK_OBJECT | TYPEMASK_ITEM | TYPEMASK_CONTAINER
	} else {
		values[2] = 0x03 // TYPEMASK_OBJECT | TYPEMASK_ITEM
	}
	values[3] = row.entry
	values[4] = math.Float32bits(1.0)
	values[6] = uint32(row.ownerGUID)
	values[7] = uint32(row.ownerGUID >> 32)
	containedFull := uint64(0)
	if row.containedGUID != 0 {
		containedFull = row.containedGUID | (uint64(0x4000) << 48)
	}
	values[8] = uint32(containedFull)
	values[9] = uint32(containedFull >> 32)
	values[10] = uint32(row.creatorGUID)
	values[11] = uint32(row.creatorGUID >> 32)
	values[12] = uint32(row.giftCreatorGUID)
	values[13] = uint32(row.giftCreatorGUID >> 32)
	values[14] = row.count
	values[15] = row.duration
	charges := parseItemSpellCharges(row.charges, templateCharges)
	for index, charge := range charges {
		values[16+index] = charge
	}
	values[21] = row.flags
	enchantments := make([]uint32, 36)
	for index, token := range strings.Fields(row.enchantments) {
		if index >= len(enchantments) {
			break
		}
		if value, err := strconv.ParseUint(token, 10, 32); err == nil {
			enchantments[index] = uint32(value)
		}
	}
	copy(values[22:58], enchantments)
	values[58] = suffixFactor
	values[59] = uint32(row.randomPropertyID)
	values[60] = row.durability
	values[61] = maxDurability
	values[62] = row.playedTime
	if container {
		values[64] = containerSlots
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
				"SELECT slot, item FROM character_inventory WHERE guid = ? AND bag = ?", s.playerGUID, row.guid)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var slot, item int64
					if err := rows.Scan(&slot, &item); err != nil || slot < 0 || slot >= int64(containerSlots) {
						continue
					}
					itemFull := uint64(item) | (uint64(0x4000) << 48)
					values[66+slot*2] = uint32(itemFull)
					values[67+slot*2] = uint32(itemFull >> 32)
				}
			}
		}
	}
	return values
}

// debugItemSetFieldToken replaces one space-separated token of a stored
// item field string (charges, enchantments), padding with zeroes — the
// write twin of the parseItemSpellCharges / parseItemFields reads.
func debugItemSetFieldToken(raw string, index int, value uint32) string {
	fields := strings.Fields(raw)
	for len(fields) <= index {
		fields = append(fields, "0")
	}
	fields[index] = strconv.FormatUint(uint64(value), 10)
	return strings.Join(fields, " ")
}

// handleDebugGetItemValue mirrors HandleDebugGetItemValueCommand
// (cs_debug.cpp:1023): ".debug getitemvalue <guid> <index>" prints the
// item's value at the index. The unknown-item and over-end legs return
// false with no message (C++ shape).
func (s *session) handleDebugGetItemValue(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug getitemvalue <guid> <index>"
	if len(args) != 2 {
		return false
	}
	guid, ok1 := parseCommandUint32(args[0])
	index, ok2 := parseCommandUint32(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	row, ok := s.debugItemRow(ctx, guid)
	if !ok {
		return false
	}
	values := s.debugItemValues(ctx, row)
	if int(index) >= len(values) {
		return false
	}
	s.sendSysMessage(fmt.Sprintf("Item %d: value at %d is %d", guid, index, values[index]))
	return true
}

// handleDebugSetItemValue mirrors HandleDebugSetItemValueCommand
// (cs_debug.cpp:1040): ".debug setitemvalue <guid> <index> <value>".
// Column-backed fields persist to item_instance (entry, owner, creator,
// gift creator, stack count, duration, spell charges, flags,
// enchantments, random property id, durability, played time) and the
// changed field is sent to the owner, the C++ values-update leg. Entry
// persists because Item::SaveToDB writes GetEntry() (Item.cpp:337) and
// GetTemplate() re-resolves from the entry on every read (Item.cpp:536),
// so a changed entry survives reload as the new template. The remaining
// identity and derived fields (guid, type, scale, contained, property
// seed, max durability, padding, container slots/contents) have no DB
// twin: SaveToDB writes the guid from the object's m_guid, not the
// values array (Item.cpp:326), and bag/slot positions come from
// character_inventory, while type/scale/max durability/container slots
// are template-derived and the property seed derives from the random
// property id - so the value is sent to the client (the C++ immediate
// effect) but reverts on the next inventory load.
func (s *session) handleDebugSetItemValue(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug setitemvalue <guid> <index> <value>"
	if len(args) != 3 {
		return false
	}
	guid, ok1 := parseCommandUint32(args[0])
	index, ok2 := parseCommandUint32(args[1])
	value, ok3 := parseCommandUint32(args[2])
	if !ok1 || !ok2 || !ok3 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	row, ok := s.debugItemRow(ctx, guid)
	if !ok {
		return false
	}
	values := s.debugItemValues(ctx, row)
	if int(index) >= len(values) {
		return false
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	db := s.server.CharactersStore.DB
	fields := map[int]uint32{int(index): value}
	switch {
	case index == 3:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET itemEntry = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	case index == 6 || index == 7:
		owner := uint64(values[6]) | uint64(values[7])<<32
		if index == 6 {
			owner = owner&^0xFFFFFFFF | uint64(value)
		} else {
			owner = owner&0xFFFFFFFF | uint64(value)<<32
		}
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET owner_guid = ? WHERE guid = ?", owner, row.guid); err != nil {
			return false
		}
		fields[6] = uint32(owner)
		fields[7] = uint32(owner >> 32)
	case index == 10 || index == 11:
		creator := uint64(values[10]) | uint64(values[11])<<32
		if index == 10 {
			creator = creator&^0xFFFFFFFF | uint64(value)
		} else {
			creator = creator&0xFFFFFFFF | uint64(value)<<32
		}
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET creatorGuid = ? WHERE guid = ?", creator, row.guid); err != nil {
			return false
		}
		fields[10] = uint32(creator)
		fields[11] = uint32(creator >> 32)
	case index == 12 || index == 13:
		giftCreator := uint64(values[12]) | uint64(values[13])<<32
		if index == 12 {
			giftCreator = giftCreator&^0xFFFFFFFF | uint64(value)
		} else {
			giftCreator = giftCreator&0xFFFFFFFF | uint64(value)<<32
		}
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET giftCreatorGuid = ? WHERE guid = ?", giftCreator, row.guid); err != nil {
			return false
		}
		fields[12] = uint32(giftCreator)
		fields[13] = uint32(giftCreator >> 32)
	case index == 14:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET `count` = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	case index == 15:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET duration = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	case index >= 16 && index <= 20:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET charges = ? WHERE guid = ?",
			debugItemSetFieldToken(row.charges, int(index)-16, value), row.guid); err != nil {
			return false
		}
	case index == 21:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET flags = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	case index >= 22 && index <= 57:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET enchantments = ? WHERE guid = ?",
			debugItemSetFieldToken(row.enchantments, int(index)-22, value), row.guid); err != nil {
			return false
		}
	case index == 59:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET randomPropertyId = ? WHERE guid = ?", int32(value), row.guid); err != nil {
			return false
		}
	case index == 60:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	case index == 62:
		if _, err := db.ExecContext(ctx, "UPDATE item_instance SET playedTime = ? WHERE guid = ?", value, row.guid); err != nil {
			return false
		}
	}
	s.sendItemValuesUpdate(row.guid, fields)
	return true
}

// debug inventory slot bounds (Player.h:526-529, 586-587): the state and
// check_all legs iterate PLAYER_SLOT_START..PLAYER_SLOT_END skipping the
// buyback range.
const (
	debugPlayerSlotStart  = 0
	debugPlayerSlotEnd    = 150
	debugBuybackSlotStart = 74
	debugBuybackSlotEnd   = 86
)

// debugInventoryItem is one character_inventory row joined to its
// item_instance owner/entry, the DB twin of Player::GetItemByPos: bag 0
// rows are the top-level slots, rows with bag = a container item's guid
// are that bag's contents.
type debugInventoryItem struct {
	bag   int64
	slot  int64
	guid  uint64
	entry uint32
	owner uint64
}

// debugInventoryWalk reads the target player's whole inventory in one
// query (the bank.go join shape), split into the bag-0 slot map and the
// per-container slot maps. First row wins per (bag, slot): the table keys
// one item per position, so a later duplicate is corrupt data the C++
// in-memory slots could never hold.
func (s *session) debugInventoryWalk(ctx context.Context, playerGUID uint64) (map[int64]debugInventoryItem, map[uint64]map[int64]debugInventoryItem) {
	top := make(map[int64]debugInventoryItem)
	contents := make(map[uint64]map[int64]debugInventoryItem)
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || playerGUID == 0 {
		return top, contents
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.bag, ci.slot, ci.item, ii.itemEntry, ii.owner_guid
		FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? ORDER BY ci.bag, ci.slot`, playerGUID)
	if err != nil {
		return top, contents
	}
	defer rows.Close()
	for rows.Next() {
		var item debugInventoryItem
		var guid, owner int64
		if err := rows.Scan(&item.bag, &item.slot, &guid, &item.entry, &owner); err != nil || guid <= 0 {
			continue
		}
		item.guid = uint64(guid)
		item.owner = uint64(owner)
		if item.bag == 0 {
			if _, dup := top[item.slot]; !dup {
				top[item.slot] = item
			}
			continue
		}
		bySlot, ok := contents[uint64(item.bag)]
		if !ok {
			bySlot = make(map[int64]debugInventoryItem)
			contents[uint64(item.bag)] = bySlot
		}
		if _, dup := bySlot[item.slot]; !dup {
			bySlot[item.slot] = item
		}
	}
	return top, contents
}

// debugItemBagSize is the GetBagSize twin: the template's ContainerSlots,
// 0 when the item is not a container or its template is unknown (a
// missing template can never be a bag in C++ either — the bag object
// comes from the template).
func (s *session) debugItemBagSize(ctx context.Context, entry uint32) int64 {
	if data, err := s.loadItemQueryData(ctx, entry); err == nil {
		return int64(data.ContainerSlots)
	}
	return 0
}

// handleDebugGetItemState mirrors HandleDebugGetItemStateCommand
// (cs_debug.cpp:491): ".debug getitemstate
// <unchanged|changed|new|removed|queue|check_all>". The arg is a single
// std::string token (ChatCommandArgs.h:91-103), so a missing arg, an
// unknown token, or any trailing token returns false silently. The
// target is the selected player, else the invoker (Chat.cpp:300-308).
// Go persists every item immediately, so every item is ITEM_UNCHANGED
// (Item.h:53-56) and there is no update queue: the changed/new/removed
// legs print the header only, the queue leg always prints "The player's
// updatequeue is empty", and the check_all queue legs never fire. The
// state leg walks PLAYER_SLOT_START..END skipping buyback, one level
// into each bag (GetBagSize from the template, cached per entry); the
// check_all leg validates owner on both levels and prints "All OK!"
// when clean. The slot and container legs are carried literally but
// cannot fire here: the DB slot is the single source of truth (the
// walk keys by it, so item.slot always equals its position), and a
// row's bag IS its container (a bag-0 row has no container by
// construction, a contents row's container is the bag it was read
// under) — in C++ they guard in-memory corruption the DB-backed
// model cannot hold.
func (s *session) handleDebugGetItemState(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) != 1 {
		return false
	}
	stateName := args[0]
	listQueue := stateName == "queue"
	checkAll := stateName == "check_all"
	var state uint32 // ITEM_UNCHANGED
	switch stateName {
	case "unchanged":
		state = 0
	case "changed":
		state = 1
	case "new":
		state = 2
	case "removed":
		state = 3
	case "queue", "check_all":
	default:
		return false
	}
	playerGUID := s.playerGUID
	if s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil {
			playerGUID = ts.playerGUID
		}
	}
	if playerGUID == 0 {
		return false
	}
	top, contents := s.debugInventoryWalk(ctx, playerGUID)
	bagSizes := make(map[uint32]int64)
	bagSize := func(entry uint32) int64 {
		if size, ok := bagSizes[entry]; ok {
			return size
		}
		size := s.debugItemBagSize(ctx, entry)
		bagSizes[entry] = size
		return size
	}
	itemFullGUID := func(guid uint64) uint64 { return guid | (uint64(0x4000) << 48) }
	if !listQueue && !checkAll {
		s.sendSysMessage("The player has the following " + stateName + " items: ")
		if state == 0 {
			for i := int64(debugPlayerSlotStart); i < debugPlayerSlotEnd; i++ {
				if i >= debugBuybackSlotStart && i < debugBuybackSlotEnd {
					continue
				}
				item, ok := top[i]
				if !ok {
					continue
				}
				if size := bagSize(item.entry); size > 0 {
					for j := int64(0); j < size; j++ {
						inner, ok := contents[item.guid][j]
						if !ok {
							continue
						}
						s.sendSysMessage(fmt.Sprintf("bag: 255 slot: %d %s owner: %s", inner.slot, miscGUIDToString(itemFullGUID(inner.guid)), miscGUIDToString(inner.owner)))
					}
					continue
				}
				s.sendSysMessage(fmt.Sprintf("bag: 255 slot: %d %s owner: %s", item.slot, miscGUIDToString(itemFullGUID(item.guid)), miscGUIDToString(item.owner)))
			}
		}
		return true
	}
	if listQueue {
		s.sendSysMessage("The player's updatequeue is empty")
		return true
	}
	errorFound := false
	playerGUIDString := miscGUIDToString(playerGUID)
	for i := int64(debugPlayerSlotStart); i < debugPlayerSlotEnd; i++ {
		if i >= debugBuybackSlotStart && i < debugBuybackSlotEnd {
			continue
		}
		item, ok := top[i]
		if !ok {
			continue
		}
		if item.slot != i {
			s.sendSysMessage(fmt.Sprintf("Item with slot %d and guid %d has an incorrect slot value: %d", i, item.guid, item.slot))
			errorFound = true
			continue
		}
		if item.owner != playerGUID {
			s.sendSysMessage(fmt.Sprintf("The item with slot %d %s does have non-matching owner guid %s and %s!", item.slot, miscGUIDToString(itemFullGUID(item.guid)), miscGUIDToString(item.owner), playerGUIDString))
			errorFound = true
			continue
		}
		if size := bagSize(item.entry); size > 0 {
			for j := int64(0); j < size; j++ {
				inner, ok := contents[item.guid][j]
				if !ok {
					continue
				}
				if inner.slot != j {
					s.sendSysMessage(fmt.Sprintf("The item in bag %d and slot %d (guid: %d) has an incorrect slot value: %d", item.slot, j, inner.guid, inner.slot))
					errorFound = true
					continue
				}
				if inner.owner != playerGUID {
					s.sendSysMessage(fmt.Sprintf("The item in bag %d at slot %d and %s, the owner (%s) and the player (%s) don't match!", item.slot, inner.slot, miscGUIDToString(itemFullGUID(inner.guid)), miscGUIDToString(inner.owner), playerGUIDString))
					errorFound = true
					continue
				}
			}
		}
	}
	if !errorFound {
		s.sendSysMessage("All OK!")
	}
	return true
}

// handleDebugItemExpire mirrors HandleDebugItemExpireCommand
// (cs_debug.cpp:1055): ".debug itemexpire <guid>" fires the expire hook
// and destroys the item (Item::UpdateDuration, Item.cpp:305-320: the
// OnItemExpire hook fires first, then DestroyItem, which fires
// OnItemRemove — destroyItemInstance carries the remove leg).
func (s *session) handleDebugItemExpire(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug itemexpire <guid>"
	if len(args) != 1 {
		return false
	}
	guid, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	row, ok := s.debugItemRow(ctx, guid)
	if !ok {
		return false
	}
	s.fireItemEvent(ctx, row.entry, scripting.ItemEventOnExpire, s.luaPlayer(), row.entry)
	s.destroyItemInstance(ctx, row.guid)
	return true
}

// debugUnitTarget resolves the getSelectedUnit twin (Chat.cpp:312) for the
// aura-state and moveflags arms: the selected creature, else the selected
// player, else the invoker (modifyTargetPlayer never returns nil
// in-session).
func (s *session) debugUnitTarget(ctx context.Context) (*creatureMotion, *session) {
	if motion := s.debugSelectedCreatureMotion(); motion != nil {
		return motion, nil
	}
	if target := s.modifyTargetPlayer(ctx); target != nil {
		return nil, target
	}
	return nil, nil
}

// handleDebugSetAuraState mirrors HandleDebugSetAuraStateCommand
// (cs_debug.cpp:1093): ".debug setaurastate [state] <apply>". The state
// is Optional, so the framework's retry (ChatCommand.h:77-103) makes a
// lone bool token reset all states — ".debug setaurastate 1" and
// ".debug setaurastate true" both take the omit-state branch — while a
// lone state token fails on the missing apply with a message, and any
// other bad token messages too. The reset leg clears the debug override
// mask; health-derived bits re-derive on the next read (Unit::Update,
// Unit.cpp:467-469) and aura-derived bits still come from the live auras,
// matching the C++ steady state. The ModifyAuraState passive-spell cast
// leg (Unit.cpp:5888-5905) and the remove leg's aura purge
// (Unit.cpp:5925-5933) have no Go model and stay out.
func (s *session) handleDebugSetAuraState(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug setaurastate [state] <apply>"
	if len(args) == 0 || len(args) > 2 {
		return false
	}
	if len(args) == 1 {
		if apply, ok := cheatArgBool(args[0]); ok {
			_ = apply
			motion, target := s.debugUnitTarget(ctx)
			if motion == nil && target == nil {
				s.sendSysMessage("Select a character or creature.")
				return false
			}
			if motion != nil {
				s.server.motionMu.Lock()
				motion.DebugAuraStateMask = 0
				s.server.motionMu.Unlock()
			} else {
				target.debugAuraStateMask = 0
			}
			return true
		}
		// A lone state token, or any other bad token: the framework
		// surfaces the parse error (LANG_CMDPARSER_* + SetSentErrorMessage).
		s.sendSysMessage(syntax)
		return true
	}
	state, ok1 := parseDebugAuraState(args[0])
	apply, ok2 := cheatArgBool(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	motion, target := s.debugUnitTarget(ctx)
	if motion == nil && target == nil {
		s.sendSysMessage("Select a character or creature.")
		return false
	}
	bit := uint32(1) << (state - 1)
	if motion != nil {
		s.server.motionMu.Lock()
		if apply {
			motion.DebugAuraStateMask |= bit
		} else {
			motion.DebugAuraStateMask &^= bit
		}
		s.server.motionMu.Unlock()
	} else {
		if apply {
			target.debugAuraStateMask |= bit
		} else {
			target.debugAuraStateMask &^= bit
		}
	}
	return true
}

// debugCreatureMovementFlags folds a motion's stored movement state into
// the GetUnitMovementFlags word (cs_debug.cpp:1251 display leg): the
// per-flag bools, the script-rooted bit, the spline-enabled bit while the
// creature is moving, and the raw unhandled bits stored by the set leg.
func debugCreatureMovementFlags(motion *creatureMotion) (uint32, uint32) {
	var flags uint32
	if motion.Walking {
		flags |= debugMoveFlagWalking
	}
	if motion.GravityDisabled {
		flags |= debugMoveFlagDisableGravity
	}
	if motion.ScriptRooted {
		flags |= debugMoveFlagRoot
	}
	if motion.Swimming {
		flags |= debugMoveFlagSwimming
	}
	if motion.Flying {
		flags |= debugMoveFlagCanFly
	}
	if motion.WaterWalking {
		flags |= debugMoveFlagWaterWalking
	}
	if motion.FeatherFalling {
		flags |= debugMoveFlagFallingSlow
	}
	if motion.Hovering {
		flags |= debugMoveFlagHover
	}
	if motion.Moving {
		flags |= debugMoveFlagSplineEnabled
	}
	flags |= motion.MoveFlagsRaw
	return flags, motion.MoveFlagsExtra
}

// debugPlayerMovementFlags is the GetUnitMovementFlags twin for players:
// the last client movement packet's flags (m_movementInfo), falling back
// to the aura-folded create block before the first packet.
func (s *session) debugPlayerMovementFlags(target *session) (uint32, uint32) {
	if mi, ok := target.lastMovementInfoSnapshot(); ok {
		return mi.Flags, uint32(mi.Flags2)
	}
	if target.player != nil {
		info := target.movementInfoForCreate(*target.player)
		return info.Flags, uint32(info.Flags2)
	}
	return 0, 0
}

// debugBroadcastPlayerMovement sends one MSG_MOVE_* update for the target
// player with the given flag word, the SendMessageToSet leg of the
// Player::Set* overrides (Player.cpp:26840-26925), and banks it as the
// last movement info so the display leg reads it back.
func (s *session) debugBroadcastPlayerMovement(target *session, opcode protocol.Opcode, flags, extra uint32) {
	if target == nil || target.player == nil || target.server == nil {
		return
	}
	info := target.movementInfoForCreate(*target.player)
	info.Flags = flags
	info.Flags2 = uint16(extra)
	info.Time = uint32(time.Now().UnixMilli())
	target.setLastMovementInfo(info)
	packet := protocol.NewBuffer(112)
	writeMovementInfo(packet, info)
	target.server.broadcastToVisibleSetFromSession(target, uint16(opcode), packet.Bytes())
}

// debugSetCreatureMoveflags applies the moveflags set leg to a creature:
// the handled setters run with the Lua-bridge broadcast legs
// (SetWalk/SetSwim/SetCanFly/SetHover/SetWaterWalk/SetDisableGravity plus
// the new SetFeatherFall twin), the root leg latches ScriptRooted, a
// cleared spline-enabled bit stops the motion, and the unhandled bits of
// the final word land in MoveFlagsRaw. The heartbeat leg
// (SendMovementFlagUpdate) has no creature carrier in Go — handled
// changes already went out on the spline packets, unhandled and extra
// bits are stored but not broadcast.
func (s *session) debugSetCreatureMoveflags(motion *creatureMotion, moveFlags uint32, moveFlagsExtra *uint32) {
	s.server.motionMu.Lock()
	current, _ := debugCreatureMovementFlags(motion)
	newFlags := moveFlags
	if newFlags&(debugMoveFlagDisableGravity|debugMoveFlagCanFly) != 0 {
		newFlags &^= debugMoveFlagFalling
	}
	if newFlags&debugMoveFlagRoot != 0 {
		motion.ScriptRooted = true
		newFlags &^= debugMoveFlagMaskMoving
	}
	hadSpline := current&debugMoveFlagSplineEnabled != 0
	var broadcasts []protocol.Opcode
	flip := func(currentValue, want bool, setOpcode, unsetOpcode protocol.Opcode, apply func(bool)) {
		if currentValue == want {
			return
		}
		apply(want)
		if want {
			broadcasts = append(broadcasts, setOpcode)
		} else {
			broadcasts = append(broadcasts, unsetOpcode)
		}
	}
	flip(motion.Walking, newFlags&debugMoveFlagWalking != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE, protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE,
		func(v bool) { motion.Walking = v })
	flip(motion.GravityDisabled, newFlags&debugMoveFlagDisableGravity != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_DISABLE, protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_ENABLE,
		func(v bool) { motion.GravityDisabled = v })
	flip(motion.Swimming, newFlags&debugMoveFlagSwimming != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_START_SWIM, protocol.OpcodeSMSG_SPLINE_MOVE_STOP_SWIM,
		func(v bool) { motion.Swimming = v })
	flip(motion.Flying, newFlags&debugMoveFlagCanFly != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_SET_FLYING, protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_FLYING,
		func(v bool) { motion.Flying = v })
	flip(motion.WaterWalking, newFlags&debugMoveFlagWaterWalking != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_WATER_WALK, protocol.OpcodeSMSG_SPLINE_MOVE_LAND_WALK,
		func(v bool) { motion.WaterWalking = v })
	flip(motion.FeatherFalling, newFlags&debugMoveFlagFallingSlow != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_FEATHER_FALL, protocol.OpcodeSMSG_SPLINE_MOVE_NORMAL_FALL,
		func(v bool) { motion.FeatherFalling = v })
	flip(motion.Hovering, newFlags&debugMoveFlagHover != 0,
		protocol.OpcodeSMSG_SPLINE_MOVE_SET_HOVER, protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_HOVER,
		func(v bool) { motion.Hovering = v })
	motion.MoveFlagsRaw = newFlags &^ debugMoveFlagsWithHandlers
	if moveFlagsExtra != nil {
		motion.MoveFlagsExtra = *moveFlagsExtra
	}
	mapID, instanceID, x, y, z, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.GUID
	phase := motion.PhaseMask
	stopSpline := hadSpline && newFlags&debugMoveFlagSplineEnabled == 0
	s.server.motionMu.Unlock()
	for _, opcode := range broadcasts {
		s.server.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
	}
	if stopSpline {
		s.server.stopCreatureMotionInInstance(mapID, instanceID, guid, x, y, z)
	}
}

// debugSetPlayerMoveflags applies the moveflags set leg to a player: the
// status-opcode flags send their SMSG_MOVE_* packet to the player plus
// the MSG_MOVE_* broadcast (Player::Set* overrides, Player.cpp:26840-
// 26925) on change, walking and swimming flip the stored flags with no
// self packet (Unit::SetWalk/SetSwim send none), root sends
// SMSG_FORCE_MOVE_ROOT, and the final flag word plus extra flags are
// banked on the last movement info. The heartbeat leg
// (SendMovementFlagUpdate) goes out when extra flags were passed or any
// unhandled bit changed.
func (s *session) debugSetPlayerMoveflags(target *session, moveFlags uint32, moveFlagsExtra *uint32) {
	current, currentExtra := s.debugPlayerMovementFlags(target)
	unhandled := ((moveFlags ^ current) &^ debugMoveFlagsWithHandlers) != 0
	newFlags := moveFlags
	if newFlags&(debugMoveFlagDisableGravity|debugMoveFlagCanFly) != 0 {
		newFlags &^= debugMoveFlagFalling
	}
	if newFlags&debugMoveFlagRoot != 0 {
		if current&debugMoveFlagRoot == 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
		}
		newFlags &^= debugMoveFlagMaskMoving
	}
	newExtra := currentExtra
	if moveFlagsExtra != nil {
		newExtra = *moveFlagsExtra
	}
	statusChanged := false
	if current&debugMoveFlagCanFly != newFlags&debugMoveFlagCanFly {
		if newFlags&debugMoveFlagCanFly != 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY))
		} else {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_UNSET_CAN_FLY))
		}
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY, newFlags, newExtra)
		statusChanged = true
	}
	if current&debugMoveFlagDisableGravity != newFlags&debugMoveFlagDisableGravity {
		if newFlags&debugMoveFlagDisableGravity != 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_GRAVITY_DISABLE))
		} else {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_GRAVITY_ENABLE))
		}
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_GRAVITY_CHNG, newFlags, newExtra)
		statusChanged = true
	}
	if current&debugMoveFlagWaterWalking != newFlags&debugMoveFlagWaterWalking {
		if newFlags&debugMoveFlagWaterWalking != 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
		} else {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_LAND_WALK))
		}
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_WATER_WALK, newFlags, newExtra)
		statusChanged = true
	}
	if current&debugMoveFlagFallingSlow != newFlags&debugMoveFlagFallingSlow {
		if newFlags&debugMoveFlagFallingSlow != 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_FEATHER_FALL))
		} else {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_NORMAL_FALL))
		}
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_FEATHER_FALL, newFlags, newExtra)
		statusChanged = true
	}
	if current&debugMoveFlagHover != newFlags&debugMoveFlagHover {
		if newFlags&debugMoveFlagHover != 0 {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_SET_HOVER))
		} else {
			target.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_UNSET_HOVER))
		}
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_HOVER, newFlags, newExtra)
		statusChanged = true
	}
	if moveFlagsExtra != nil || unhandled {
		s.debugBroadcastPlayerMovement(target, protocol.OpcodeMSG_MOVE_HEARTBEAT, newFlags, newExtra)
	} else if !statusChanged {
		// No packet fired (a walking/swimming flip, or nothing
		// changed): bank the new word silently, the way the C++
		// flag word sits in m_movementInfo until the next update.
		info := target.movementInfoForCreate(*target.player)
		info.Flags = newFlags
		info.Flags2 = uint16(newExtra)
		info.Time = uint32(time.Now().UnixMilli())
		target.setLastMovementInfo(info)
	}
}

// handleDebugMoveflags mirrors HandleDebugMoveflagsCommand
// (cs_debug.cpp:1251): ".debug moveflags [moveFlags] [moveFlagsExtra]".
// No arguments prints the target's flags (LANG_MOVEFLAGS_GET 1143); with
// arguments the handled setters run and the new flags print
// (LANG_MOVEFLAGS_SET 1144) — both LANG texts are unseeded in-tree, so
// the Go wording carries the same two values. The uint32 args are
// Optional, so an unparseable token messages (the Optional retry
// surfaces the parse error, ChatCommand.h:77-103) while a trailing token
// after two valid values stays silent.
func (s *session) handleDebugMoveflags(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug moveflags [moveFlags] [moveFlagsExtra]"
	var moveFlags *uint32
	var moveFlagsExtra *uint32
	switch len(args) {
	case 0:
	case 1:
		value, ok := parseCommandUint32(args[0])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		moveFlags = &value
	case 2:
		first, ok1 := parseCommandUint32(args[0])
		second, ok2 := parseCommandUint32(args[1])
		if !ok1 || !ok2 {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		moveFlags = &first
		moveFlagsExtra = &second
	default:
		// Trailing tokens after two values stay silent, but a failed
		// typed token still surfaces its parse error (Optional retry).
		if _, ok1 := parseCommandUint32(args[0]); !ok1 {
			s.sendSysMessage(syntax)
			return true
		}
		if _, ok2 := parseCommandUint32(args[1]); !ok2 {
			s.sendSysMessage(syntax)
			return true
		}
		return false
	}
	motion, target := s.debugUnitTarget(ctx)
	if motion == nil && target == nil {
		s.sendSysMessage("Select a character or creature.")
		return false
	}
	if moveFlags == nil {
		var flags, extra uint32
		if motion != nil {
			s.server.motionMu.Lock()
			flags, extra = debugCreatureMovementFlags(motion)
			s.server.motionMu.Unlock()
		} else {
			flags, extra = s.debugPlayerMovementFlags(target)
		}
		s.sendSysMessage(fmt.Sprintf("Movement flags: %d, extra flags: %d", flags, extra))
		return true
	}
	if motion != nil {
		s.debugSetCreatureMoveflags(motion, *moveFlags, moveFlagsExtra)
		s.server.motionMu.Lock()
		flags, extra := debugCreatureMovementFlags(motion)
		s.server.motionMu.Unlock()
		s.sendSysMessage(fmt.Sprintf("Movement flags: %d, extra flags: %d", flags, extra))
		return true
	}
	s.debugSetPlayerMoveflags(target, *moveFlags, moveFlagsExtra)
	flags, extra := s.debugPlayerMovementFlags(target)
	s.sendSysMessage(fmt.Sprintf("Movement flags: %d, extra flags: %d", flags, extra))
	return true
}
