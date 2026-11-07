package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// list command ports list_commandscript (cs_list.cpp), the TWENTY-FOURTH
// Commands group in loader call order (AddSC_lfg_commandscript() is call 86,
// this is call 87). All 7 arms report database state: `creature` and `object`
// list creature/gameobject spawns from the world database (nearest first when
// the invoker is a player); `item` finds copies of an item across inventories,
// mail, auctionhouse and guild banks from the character database;
// `auras` dumps the selected player's applied auras; `mail` lists a player's
// mail from the character database; `spawnpoints` lists creature/gameobject
// spawns on the invoker's map. Console-vs-chat LANG branches are moot (Go
// commands are always sessioned), and LANG texts are inlined from TDB enUS
// recall (no in-tree trinity_string seed), per tree convention.
//
// Fidelity notes (documented gaps, not stubs):
//   - the live-map lookup (Map::GetCreatureBySpawnIdStore /
//     GetGameObjectBySpawnIdStore) has no Go bridge, so every listed spawn is
//     reported with empty live GUID/flag fields, like the C++ rows whose map
//     has no live instance;
//   - item positions use the bag/slot constants verbatim (Player.h:
//     EQUIPMENT_SLOT_END=19, INVENTORY_SLOT_ITEM_START/END=23/39,
//     BANK_SLOT_ITEM_START/END=39/67, INVENTORY_SLOT_BAG_START/END=19/23,
//     BANK_SLOT_BAG_START/END=67/74); NULL bag (dangling container row)
//     mirrors the C++ GetUInt32-on-NULL result of 0 and matches nothing;
//   - the aura talent mark is always empty: GetTalentSpellCost has no Go model
//     (same gap as the learn port); the per-effect aura-type list is rebuilt
//     from each aura's single stored AuraType;
//   - only online players can be targeted for `auras` (no offline
//     CharacterCache name->GUID bridge in the Go tree); `mail` resolves
//     offline names/GUIDs through the character DB. No selection lists
//     the invoker's own auras (getSelectedUnit falls back to self,
//     Chat.cpp:312-321); creature selections have no aura model in the
//     Go tree (auras live on sessions only) and report honestly;
//   - `respawns` has no Go bridge at all (no live map respawn registry, no
//     spawn-group model, no IsBattlegroundOrArena-per-map state) and is
//     RBAC-gated with an honest message instead of a stub.

// handleCmdList ports the list command table (cs_list.cpp:52-64): 7 arms,
// Trinity per-level prefix matching, each arm gated on its own RBAC permission
// (ChatCommand.cpp:487 checks the invoker/leaf node only).
func (s *session) handleCmdList(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	deny := func(perm uint32) bool {
		return !s.commandAllowed(ctx, perm)
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("creature", sub):
		if deny(permissionCommandListCreature) {
			return false
		}
		return s.handleListCreatureCommand(ctx, rest)
	case strings.HasPrefix("item", sub):
		if deny(permissionCommandListItem) {
			return false
		}
		return s.handleListItemCommand(ctx, rest)
	case strings.HasPrefix("object", sub):
		if deny(permissionCommandListObject) {
			return false
		}
		return s.handleListObjectCommand(ctx, rest)
	case strings.HasPrefix("auras", sub):
		if deny(permissionCommandListAuras) {
			return false
		}
		if len(rest) > 0 {
			return false
		}
		return s.handleListAurasCommand()
	case strings.HasPrefix("mail", sub):
		if deny(permissionCommandListMail) {
			return false
		}
		return s.handleListMailCommand(ctx, rest)
	case strings.HasPrefix("spawnpoints", sub):
		if deny(permissionCommandListSpawnpoints) {
			return false
		}
		if len(rest) > 0 {
			return false
		}
		return s.handleListSpawnPointsCommand(ctx)
	case strings.HasPrefix("respawns", sub):
		if deny(permissionCommandListRespawns) {
			return false
		}
		return s.handleListRespawnsCommand(ctx, rest)
	default:
		return false
	}
}

// parseListEntryID mirrors the Variant<Hyperlink<...>, uint32> args of the
// creature/object arms (cs_list.cpp:69/347): it accepts a |H<kind>:<id>...|h
// link or a bare numeric id.
func parseListEntryID(arg, link string) (uint32, bool) {
	t := strings.TrimSpace(arg)
	if i := strings.Index(t, "H"+link+":"); i >= 0 {
		t = t[i+len("H"+link+":"):]
		j := 0
		for j < len(t) && t[j] >= '0' && t[j] <= '9' {
			j++
		}
		t = t[:j]
		n, err := strconv.ParseUint(t, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(n), true
	}
	// Bare numeric id: StringTo<uint32> base 0 (ChatCommandArgs.h:60-70),
	// so 0x/0b forms parse like the C++ Variant arm (R13 D9).
	return parseCommandUint32(t)
}

// parseListEntryIDLink parses only the |H<kind>:<id>...|h link form, for arms
// whose C++ arg is a plain Hyperlink: the item arm (cs_list.cpp:154) takes
// Hyperlink<item> with no uint32 variant, so a bare numeric id fails the
// C++ parser (ChatCommandTags.h:204-235) and must fail here too.
func parseListEntryIDLink(arg, link string) (uint32, bool) {
	t := strings.TrimSpace(arg)
	i := strings.Index(t, "H"+link+":")
	if i < 0 {
		return 0, false
	}
	t = t[i+len("H"+link+":"):]
	j := 0
	for j < len(t) && t[j] >= '0' && t[j] <= '9' {
		j++
	}
	if j == 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(t[:j], 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// listCountArg mirrors the Optional<uint32> count args: default 10, zero
// aborts the arm silently like the C++ (count == 0 -> return false), and an
// unparseable token fails the arm (the C++ prints LANG_CMDPARSER_STRING_VALUE_INVALID
// at ChatCommand.h:77-94, for which the tree's stand-in is the syntax line),
// never silently defaulting to 10.
func listCountArg(args []string) (uint32, bool) {
	if len(args) == 0 {
		return 10, true
	}
	// Base-0 StringTo<uint32> (ChatCommandArgs.h:60-70): 0x/0b counts
	// parse like the C++ Optional<uint32> arm (R13 D9).
	return parseCommandUint32(args[0])
}

// handleListCreatureCommand mirrors HandleListCreatureCommand
// (cs_list.cpp:66-132): creature_template gate, total count, then up to count
// rows ordered by distance from the invoker (cs_list.cpp:96-99). Every row
// renders with empty live fields: the live-map spawn registry
// (Map::GetCreatureBySpawnIdStore) has no Go bridge (documented gap).
func (s *session) handleListCreatureCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	entry, ok := parseListEntryID(args[0], "creature_entry")
	if !ok {
		s.sendSysMessage("Syntax: .list creature <entry|link> [count]")
		return true
	}
	count, ok := listCountArg(args[1:])
	if !ok {
		s.sendSysMessage("Syntax: .list creature <entry|link> [count]")
		return true
	}
	if len(args) > 2 {
		return false
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM creature_template WHERE entry = ? LIMIT 1", entry).Scan(&name); err != nil {
		s.sendSysMessage(fmt.Sprintf("Invalid creature ID %d.", entry)) // LANG_COMMAND_INVALIDCREATUREID (440)
		return true
	}
	if count == 0 {
		return false
	}
	var creatureCount uint64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(guid) FROM creature WHERE id = ?", entry).Scan(&creatureCount); err != nil {
		s.sendSysMessage(fmt.Sprintf("List creature error: %v", err))
		return true
	}
	rows, err := listCreatureRows(ctx, db, entry, count, s.player.X, s.player.Y, s.player.Z)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("List creature error: %v", err))
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint32
		var x, y, z float64
		var mapID uint16
		if err := rows.Scan(&guid, &x, &y, &z, &mapID); err != nil {
			continue
		}
		// LANG_CREATURE_LIST_CHAT (515); live GUID/alive fields are empty:
		// no live spawn registry in the Go tree (documented gap).
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hcreature:%d|h[%s]|h|r - x: %f y: %f z: %f mapid: %d (GUID %s %s)",
			guid, guid, name, x, y, z, mapID, "", ""))
	}
	// LANG_COMMAND_LISTCREATUREMESSAGE (441): C++ passes (creatureId,
	// creatureCount) positionally (cs_list.cpp:149) into the recalled
	// TDB text, so the id leads — the same (id, count) order as the
	// item/object twins below. Format text unseeded in-tree.
	s.sendSysMessage(fmt.Sprintf("List of creatures found: %d (entry %d)", entry, creatureCount))
	return true
}

func listCreatureRows(ctx context.Context, db *sql.DB, entry uint32, count uint32, px, py, pz float32) (*sql.Rows, error) {
	return db.QueryContext(ctx, fmt.Sprintf(`SELECT guid, position_x, position_y, position_z, map FROM creature WHERE id = ?
		ORDER BY (POW(position_x - %f, 2) + POW(position_y - %f, 2) + POW(position_z - %f, 2)) ASC LIMIT ?`, px, py, pz), entry, count)
}

// handleListObjectCommand mirrors HandleListObjectCommand
// (cs_list.cpp:251-317), the gameobject twin of the creature arm.
func (s *session) handleListObjectCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	entry, ok := parseListEntryID(args[0], "gameobject_entry")
	if !ok {
		s.sendSysMessage("Syntax: .list object <entry|link> [count]")
		return true
	}
	count, ok := listCountArg(args[1:])
	if !ok {
		s.sendSysMessage("Syntax: .list object <entry|link> [count]")
		return true
	}
	if len(args) > 2 {
		return false
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM gameobject_template WHERE entry = ? LIMIT 1", entry).Scan(&name); err != nil {
		s.sendSysMessage(fmt.Sprintf("Invalid gameobject ID %d.", entry)) // LANG_COMMAND_LISTOBJINVALIDID (437)
		return true
	}
	if count == 0 {
		return false
	}
	var objectCount uint64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(guid) FROM gameobject WHERE id = ?", entry).Scan(&objectCount); err != nil {
		s.sendSysMessage(fmt.Sprintf("List object error: %v", err))
		return true
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT guid, position_x, position_y, position_z, map FROM gameobject WHERE id = ?
		ORDER BY (POW(position_x - %f, 2) + POW(position_y - %f, 2) + POW(position_z - %f, 2)) ASC LIMIT ?`, s.player.X, s.player.Y, s.player.Z), entry, count)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("List object error: %v", err))
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint32
		var x, y, z float64
		var mapID uint16
		if err := rows.Scan(&guid, &x, &y, &z, &mapID); err != nil {
			continue
		}
		// LANG_GO_LIST_CHAT (517); live GUID/spawned fields are empty: no
		// live spawn registry in the Go tree (documented gap).
		s.sendSysMessage(fmt.Sprintf("%d - %d - |cffffffff|Hgameobject:%d|h[%s]|h|r - x: %f y: %f z: %f mapid: %d (GUID %s %s)",
			guid, entry, guid, name, x, y, z, mapID, "", ""))
	}
	// LANG_COMMAND_LISTOBJMESSAGE (439): C++ passes (gameObjectId,
	// objectCount) positionally (cs_list.cpp:428) into the recalled TDB
	// text, so the id leads. Format text unseeded in-tree.
	s.sendSysMessage(fmt.Sprintf("List of gameobjects found: %d (entry %d)", entry, objectCount))
	return true
}

// item bag/slot constants mirror Player.h: EQUIPMENT_SLOT_END=19,
// INVENTORY_SLOT_BAG_START/END=19/23, INVENTORY_SLOT_ITEM_START/END=23/39,
// BANK_SLOT_ITEM_START/END=39/67, BANK_SLOT_BAG_START/END=67/74,
// INVENTORY_SLOT_BAG_0=255.
const (
	listBag0          = 255
	listEquipEnd      = 19
	listInvBagStart   = 19
	listInvBagEnd     = 23
	listInvItemStart  = 23
	listInvItemEnd    = 39
	listBankItemStart = 39
	listBankItemEnd   = 67
	listBankBagStart  = 67
	listBankBagEnd    = 74
)

// listItemPos mirrors Player::IsEquipmentPos/IsInventoryPos/IsBankPos
// (Player.cpp:10141-10170) on the (bag, slot) pair from the inventory query.
func listItemPos(bag, slot uint32) string {
	switch {
	case bag == listBag0 && slot < listEquipEnd:
		return "[equipped]"
	case bag == listBag0 && slot >= listInvItemStart && slot < listInvItemEnd,
		bag >= listInvBagStart && bag < listInvBagEnd:
		return "[in inventory]"
	case bag == listBag0 && slot >= listBankItemStart && slot < listBankItemEnd,
		bag >= listBankBagStart && bag < listBankBagEnd:
		return "[in bank]"
	default:
		return ""
	}
}

// handleListItemCommand mirrors HandleListItemCommand (cs_list.cpp:154-345):
// item_instance copies are reported from character inventories, mail, the
// auctionhouse and guild banks, each case keeping the C++ row order and
// decrementing the remaining count exactly like the C++ does (auction does
// not decrement, matching cs_list.cpp). The entry arg is a plain
// Hyperlink<item> (no bare-number form, unlike the creature/object arms).
func (s *session) handleListItemCommand(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	itemID, ok := parseListEntryIDLink(args[0], "item")
	if !ok {
		if strings.Contains(args[0], "Hitem:") {
			s.sendSysMessage("Syntax: .list item <link> [count]")
			return true
		}
		return false
	}
	count, ok := listCountArg(args[1:])
	if !ok {
		s.sendSysMessage("Syntax: .list item <link> [count]")
		return true
	}
	if len(args) > 2 {
		return false
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if count == 0 {
		return false
	}

	var inventoryCount, mailCount, auctionCount, guildCount uint64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(itemEntry) FROM character_inventory ci INNER JOIN item_instance ii ON ii.guid = ci.item WHERE itemEntry = ?", itemID).Scan(&inventoryCount)

	rows, err := cdb.QueryContext(ctx, `SELECT ci.item, cb.slot AS bag, ci.slot, ci.guid, c.account, c.name FROM characters c
		INNER JOIN character_inventory ci ON ci.guid = c.guid
		INNER JOIN item_instance ii ON ii.guid = ci.item
		LEFT JOIN character_inventory cb ON cb.item = ci.bag WHERE ii.itemEntry = ? LIMIT ?`, itemID, count)
	inventoryRows := uint64(0)
	if err == nil {
		for rows.Next() {
			var itemGuid uint32
			var bag sql.NullInt64
			var slot uint8
			var ownerGuid, ownerAccount uint32
			var ownerName string
			if err := rows.Scan(&itemGuid, &bag, &slot, &ownerGuid, &ownerAccount, &ownerName); err != nil {
				continue
			}
			inventoryRows++
			var bagVal uint32
			if bag.Valid {
				bagVal = uint32(bag.Int64)
			}
			// LANG_ITEMLIST_SLOT (508)
			s.sendSysMessage(fmt.Sprintf("Item: %d Owner: %s OwnerGuid: %d OwnerAccount: %d %s",
				itemGuid, ownerName, ownerGuid, ownerAccount, listItemPos(bagVal, uint32(slot))))
		}
		rows.Close()
		// C++-exact: count decrements by the row count actually returned.
		count = decrementListCount(count, inventoryRows)
	}

	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(itemEntry) FROM mail_items mi INNER JOIN item_instance ii ON ii.guid = mi.item_guid WHERE itemEntry = ?", itemID).Scan(&mailCount)
	if count > 0 {
		rows, err = cdb.QueryContext(ctx, `SELECT mi.item_guid, m.sender, m.receiver, cs.account, cs.name, cr.account, cr.name
			FROM mail m INNER JOIN mail_items mi ON mi.mail_id = m.id INNER JOIN item_instance ii ON ii.guid = mi.item_guid
			INNER JOIN characters cs ON cs.guid = m.sender INNER JOIN characters cr ON cr.guid = m.receiver
			WHERE ii.itemEntry = ? LIMIT ?`, itemID, count)
		if err == nil {
			n := 0
			for rows.Next() {
				var itemGuid, itemSender, itemReceiver, itemSenderAccount, itemReceiverAccount uint32
				var itemSenderName, itemReceiverName string
				if err := rows.Scan(&itemGuid, &itemSender, &itemReceiver, &itemSenderAccount, &itemSenderName, &itemReceiverAccount, &itemReceiverName); err != nil {
					continue
				}
				n++
				// LANG_ITEMLIST_MAIL (509)
				s.sendSysMessage(fmt.Sprintf("Item: %d Sender: %s SenderGuid: %d SenderAccount: %d Receiver: %s ReceiverGuid: %d ReceiverAccount: %d %s",
					itemGuid, itemSenderName, itemSender, itemSenderAccount, itemReceiverName, itemReceiver, itemReceiverAccount, "[in mail]"))
			}
			rows.Close()
			count = decrementListCount(count, uint64(n))
		}
	}

	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(itemEntry) FROM auctionhouse ah INNER JOIN item_instance ii ON ii.guid = ah.itemguid WHERE itemEntry = ?", itemID).Scan(&auctionCount)
	if count > 0 {
		rows, err = cdb.QueryContext(ctx, `SELECT ah.itemguid, ah.itemowner, c.account, c.name FROM auctionhouse ah
			INNER JOIN characters c ON c.guid = ah.itemowner INNER JOIN item_instance ii ON ii.guid = ah.itemguid
			WHERE ii.itemEntry = ? LIMIT ?`, itemID, count)
		if err == nil {
			for rows.Next() {
				var itemGuid, owner, ownerAccount uint32
				var ownerName string
				if err := rows.Scan(&itemGuid, &owner, &ownerAccount, &ownerName); err != nil {
					continue
				}
				// LANG_ITEMLIST_AUCTION (510); no count decrement (cs_list.cpp).
				s.sendSysMessage(fmt.Sprintf("Item: %d Owner: %s OwnerGuid: %d OwnerAccount: %d %s",
					itemGuid, ownerName, owner, ownerAccount, "[in auction]"))
			}
			rows.Close()
		}
	}

	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(itemEntry) FROM guild_bank_item gbi INNER JOIN item_instance ii ON ii.guid = gbi.item_guid WHERE itemEntry = ?", itemID).Scan(&guildCount)
	rows, err = cdb.QueryContext(ctx, `SELECT gi.item_guid, gi.guildid, g.name FROM guild_bank_item gi
		INNER JOIN guild g ON g.guildid = gi.guildid INNER JOIN item_instance ii ON ii.guid = gi.item_guid
		WHERE ii.itemEntry = ? LIMIT ?`, itemID, count)
	if err == nil {
		n := 0
		for rows.Next() {
			var itemGuid, guildGuid uint32
			var guildName string
			if err := rows.Scan(&itemGuid, &guildGuid, &guildName); err != nil {
				continue
			}
			n++
			// LANG_ITEMLIST_GUILD (1118)
			s.sendSysMessage(fmt.Sprintf("Item: %d Guild: %s GuildGuid: %d %s",
				itemGuid, guildName, guildGuid, "[in guild bank]"))
		}
		rows.Close()
		count = decrementListCount(count, uint64(n))
	}

	total := inventoryCount + mailCount + auctionCount + guildCount
	if total == 0 {
		s.sendSysMessage("No item found.") // LANG_COMMAND_NOITEMFOUND (436)
		return true
	}
	// LANG_COMMAND_LISTITEMMESSAGE (438): C++ passes (itemId, total,
	// inventory, mail, auction, guild) positionally (cs_list.cpp:342)
	// into the recalled TDB text, so the id leads. Format text unseeded
	// in-tree.
	s.sendSysMessage(fmt.Sprintf("List of items found: %d (item %d, inventory: %d, mail: %d, auction: %d, guild bank: %d)",
		itemID, total, inventoryCount, mailCount, auctionCount, guildCount))
	return true
}

// decrementListCount mirrors the C++ count shrink after each case:
// count > resultCount ? count -= resultCount : count = 0.
func decrementListCount(count uint32, resultCount uint64) uint32 {
	if uint64(count) > resultCount {
		return count - uint32(resultCount)
	}
	return 0
}

// objectGUIDCounter mirrors ObjectGuid::GetCounter (ObjectGuid.h:148-153):
// GUIDs whose HighGuid carries an entry (GameObject/Transport/Unit/Pet/
// Vehicle, and default) expose only the low 24 bits as the counter — the
// entry lives in bits 24-47. GUIDs without an entry (Player/Item/
// DynamicObject/Corpse/Mo_Transport/Instance/Group, ObjectGuid.h:216-234)
// expose the full low 32 bits.
func objectGUIDCounter(guid uint64) uint32 {
	switch uint16(guid >> 48) {
	case 0x0000, 0x4000, 0xF100, 0xF101, 0x1FC0, 0x1F40, 0x1F50:
		return uint32(guid)
	default:
		return uint32(guid & 0xFFFFFF)
	}
}

// handleListAurasCommand mirrors HandleListAurasCommand (cs_list.cpp:433-478)
// for the selected unit (getSelectedUnit, Chat.cpp:312-321): no selection
// falls back to self; creature selections have no aura model in the Go
// tree (auras live on sessions only) and report honestly instead of
// listing. Detail lines carry effect mask, charges, stack, slot,
// duration and the passive mark (AttributesEx bit 0x40); the talent mark is
// always empty because GetTalentSpellCost has no Go model.
func (s *session) handleListAurasCommand() bool {
	target := s.lookupSelectedPlayerOrSelf()
	if target == nil || target.player == nil {
		s.sendSysMessage("Creature auras are unavailable: the Go tree has no creature aura model.")
		return true
	}
	ids := make([]uint32, 0, len(target.activeAuras))
	for id, a := range target.activeAuras {
		if a != nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// LANG_COMMAND_TARGET_LISTAURAS (467)
	s.sendSysMessage(fmt.Sprintf("List of auras found: %d", len(ids)))
	for _, id := range ids {
		aura := target.activeAuras[id]
		name := ""
		passive := ""
		if s.server != nil && s.server.Data != nil {
			if n, _, found, err := s.server.Data.SpellName(id); err == nil && found {
				name = n
			}
			if sp, found, err := s.server.Data.Spell(id); err == nil && found && sp.Attributes&spellAttr0Passive != 0 {
				passive = "Passive" // LANG_PASSIVE (33)
			}
		}
		link := fmt.Sprintf("|cffffffff|Hspell:%d|h[%s]|h|r", id, name)
		casterKind := "creature"
		if uint16(aura.CasterGUID>>48) == 0 {
			casterKind = "player"
		}
		// LANG_COMMAND_TARGET_AURADETAIL (468); talent mark empty (no
		// GetTalentSpellCost bridge). The caster counter is
		// ObjectGuid::GetCounter: low 24 bits for entry-carrying GUIDs
		// (creature/gameobject/pet/vehicle casters), full low 32 for
		// players — the raw low 32 would mix entry bits into the counter.
		s.sendSysMessage(fmt.Sprintf("Aura %d: %s Effect mask: %d Charges: %d Stack: %d Slot: %d Duration: %d MaxDuration: %d %s %s Caster: %s %d",
			id, link, aura.EffectMask, aura.RemainingCharges, aura.StackAmount, aura.Slot,
			int32(aura.RemainingMs), int32(aura.DurationMs), passive, "", casterKind, objectGUIDCounter(aura.CasterGUID)))
	}
	// The C++ second loop reports per-effect aura types; the Go model stores
	// one AuraType per applied aura, so effects are grouped by the aura's own
	// type (documented gap).
	type auraEffectLine struct {
		spellID uint32
		index   uint32
		amount  int32
	}
	byType := map[uint32][]auraEffectLine{}
	var types []uint32
	for _, id := range ids {
		aura := target.activeAuras[id]
		t := aura.AuraType
		if _, seen := byType[t]; !seen {
			types = append(types, t)
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) != 0 {
				byType[t] = append(byType[t], auraEffectLine{id, uint32(i), aura.Amounts[i]})
			}
		}
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	for _, t := range types {
		lines := byType[t]
		// LANG_COMMAND_TARGET_LISTAURATYPE (469)
		s.sendSysMessage(fmt.Sprintf("Aura effects: %d of type %d", len(lines), t))
		for _, l := range lines {
			// LANG_COMMAND_TARGET_AURASIMPLE (470)
			s.sendSysMessage(fmt.Sprintf("Effect %d Index %d Amount %d", l.spellID, l.index, l.amount))
		}
	}
	return true
}

// listMailPlayerLink mirrors ChatHandler::playerLink (Chat.h:119); the Go
// command path is always sessioned, so the colored link form is used.
func listMailPlayerLink(name string) string {
	return "|cffffffff|Hplayer:" + name + "|h[" + name + "]|h|r"
}

// itemQualityColors mirrors ItemQualityColors (SharedDefines.h): grey, white,
// green, blue, purple, orange, light yellow, artifact.
var itemQualityColors = []uint32{
	0xFF9D9D9D, 0xFFFFFFFF, 0xFF1EFF00, 0xFF0070DD,
	0xFFA335EE, 0xFFFF8000, 0xFFE6CC80, 0xFFE6CC80,
}

// handleListMailCommand mirrors HandleListMailCommand (cs_list.cpp:480-617):
// optional name, else selection, else self; offline names/GUIDs resolve
// through the character DB (|Hplayer link, numeric GUID, or plain name —
// the C++ PlayerIdentifier forms, ChatCommandTags.cpp:88-121). Mail rows
// come straight from the character database (CHAR_SEL_MAIL_LIST_COUNT/INFO
// in CharacterDatabase.cpp:41-43), item rows from mail_items +
// item_instance + item_template.
func (s *session) handleListMailCommand(ctx context.Context, args []string) bool {
	if len(args) > 1 {
		return false
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	// C++ PlayerIdentifier (ChatCommandTags.cpp:88-121) accepts a |Hplayer
	// link, a numeric player GUID, or a plain name; the old code only tried
	// the online name form, so link/GUID targets (including offline GUID
	// targets, whose mail is plain DB state) silently listed nothing.
	var targetGUID uint64
	var targetName string
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		first := strings.TrimSpace(args[0])
		switch {
		case strings.HasPrefix(first, "|"):
			name := normalizePlayerName(extractPlayerNameFromLink(first))
			if online := s.sessionForPlayerName(name); online != nil && online.player != nil {
				targetGUID, targetName = online.playerGUID, online.player.Name
			} else if err := cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&targetGUID); err != nil || targetGUID == 0 {
				s.sendSysMessage("Player not found.")
				return true
			} else {
				targetName = name
			}
		case isAllDigits(first):
			guid64, err := strconv.ParseUint(first, 10, 32)
			if err != nil || guid64 == 0 {
				s.sendSysMessage("Player not found.")
				return true
			}
			if online := s.server.playerSessionForGUID(guid64); online != nil && online.player != nil {
				targetGUID, targetName = online.playerGUID, online.player.Name
			} else if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid64).Scan(&targetName); err != nil || targetName == "" {
				s.sendSysMessage("Player not found.")
				return true
			} else {
				targetGUID = guid64
			}
		default:
			name := normalizePlayerName(strings.Join(args, " "))
			if online := s.sessionForPlayerName(name); online != nil && online.player != nil {
				targetGUID, targetName = online.playerGUID, online.player.Name
			} else if err := cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&targetGUID); err != nil || targetGUID == 0 {
				s.sendSysMessage("Player not found.")
				return true
			} else {
				targetName = name
			}
		}
	} else {
		if ts := s.lookupSelectedPlayer(); ts != nil && ts.player != nil {
			targetGUID, targetName = ts.playerGUID, ts.player.Name
		} else if s.player != nil {
			targetGUID, targetName = s.playerGUID, s.player.Name
		} else {
			return false
		}
	}
	var countMail uint64
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(id) FROM mail WHERE receiver = ?", targetGUID).Scan(&countMail); err != nil {
		s.sendSysMessage(fmt.Sprintf("List mail error: %v", err))
		return true
	}
	// LANG_LIST_MAIL_HEADER (1151)
	s.sendSysMessage(fmt.Sprintf("%d mail(s) for %s (guid %d)", countMail, listMailPlayerLink(targetName), targetGUID))
	s.sendSysMessage("================================================") // LANG_ACCOUNT_LIST_BAR (1012)
	rows, err := cdb.QueryContext(ctx, `SELECT id, sender, (SELECT name FROM characters WHERE guid = sender) AS sendername,
		receiver, (SELECT name FROM characters WHERE guid = receiver) AS receivername,
		subject, deliver_time, expire_time, money, has_items FROM mail WHERE receiver = ?`, targetGUID)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("List mail error: %v", err))
		return true
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var messageID, senderID, receiverID uint32
		var sender, receiver, subject string
		var deliverTime, expireTime uint32
		var money uint32
		var hasItem uint8
		if err := rows.Scan(&messageID, &senderID, &sender, &receiverID, &receiver, &subject, &deliverTime, &expireTime, &money, &hasItem); err != nil {
			continue
		}
		found = true
		gold := money / 10000
		silv := (money % 10000) / 100
		copp := (money % 10000) % 100
		// LANG_LIST_MAIL_INFO_1 (1152)
		s.sendSysMessage(fmt.Sprintf("%d: %s (%dg %ds %dc)", messageID, subject, gold, silv, copp))
		// LANG_LIST_MAIL_INFO_2 (1153)
		s.sendSysMessage(fmt.Sprintf("From: %s (%d) To: %s (%d)",
			listMailPlayerLink(sender), senderID, listMailPlayerLink(receiver), receiverID))
		// LANG_LIST_MAIL_INFO_3 (1154)
		s.sendSysMessage(fmt.Sprintf("Delivered: %s Expires: %s",
			timeToTimestampStr(time.Unix(int64(deliverTime), 0)), timeToTimestampStr(time.Unix(int64(expireTime), 0))))
		if hasItem == 1 {
			itemRows, err := cdb.QueryContext(ctx, "SELECT item_guid FROM mail_items WHERE mail_id = ?", messageID)
			if err == nil {
				for itemRows.Next() {
					var itemGUID uint32
					if err := itemRows.Scan(&itemGUID); err != nil {
						continue
					}
					var itemEntry, itemCount uint32
					if err := cdb.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", itemGUID).Scan(&itemEntry, &itemCount); err != nil {
						continue
					}
					var itemName string
					var itemQuality uint8
					wdb := s.goWorldDB()
					if wdb == nil {
						wdb = cdb
					}
					if err := wdb.QueryRowContext(ctx, "SELECT name, quality FROM item_template WHERE entry = ?", itemEntry).Scan(&itemName, &itemQuality); err != nil {
						continue
					}
					color := uint32(0xFFFFFFFF)
					if int(itemQuality) < len(itemQualityColors) {
						color = itemQualityColors[itemQuality]
					}
					itemStr := fmt.Sprintf("|c%08x|Hitem:%d:0:0:0:0:0:0:0:0:0|h[%s]|h|r", color, itemEntry, itemName)
					// LANG_LIST_MAIL_INFO_ITEM (1155)
					s.sendSysMessage(fmt.Sprintf("Item: %s (entry %d, guid %d, count %d)", itemStr, itemEntry, itemGUID, itemCount))
				}
				itemRows.Close()
			}
		}
		s.sendSysMessage("================================================") // LANG_ACCOUNT_LIST_BAR (1012)
	}
	if !found {
		s.sendSysMessage("No mail found.") // LANG_LIST_MAIL_NOT_FOUND (1156)
	}
	return true
}

// handleListSpawnPointsCommand mirrors HandleListSpawnPointsCommand
// (cs_list.cpp:467-493): every creature/gameobject spawn row on the invoker's
// map, filtered to a 5000-yard 2D radius unless the map is an instance,
// battleground or arena (Map.dbc instance type != 0).
func (s *session) handleListSpawnPointsCommand(ctx context.Context) bool {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	mapID := s.player.Map
	mapName := ""
	showAll := false
	if s.server != nil && s.server.Data != nil {
		if me, ok, err := s.server.Data.Map(mapID); err == nil && ok {
			mapName = me.MapName
			showAll = me.InstanceType != 0
		}
	}
	tail := ""
	if !showAll {
		tail = " within 5000yd"
	}
	s.sendSysMessage(fmt.Sprintf("Listing all spawn points in map %d (%s)%s:", mapID, mapName, tail))
	type spawnPoint struct {
		typ     uint32 // SpawnObjectType: 0 creature, 1 gameobject (SpawnData.h:29-34)
		spawnID uint32
		entry   uint32
		name    string
		x, y, z float64
	}
	report := func(p spawnPoint) {
		if !showAll {
			if dx, dy := p.x-float64(s.player.X), p.y-float64(s.player.Y); math.Hypot(dx, dy) >= 5000.0 {
				return
			}
		}
		s.sendSysMessage(fmt.Sprintf("Type: %d | SpawnId: %d | Entry: %d (%s) | X: %.3f | Y: %.3f | Z: %.3f",
			p.typ, p.spawnID, p.entry, p.name, p.x, p.y, p.z))
	}
	creatureRows, err := db.QueryContext(ctx, `SELECT c.guid, c.id, t.name, c.position_x, c.position_y, c.position_z
		FROM creature c INNER JOIN creature_template t ON t.entry = c.id WHERE c.map = ? ORDER BY c.guid`, mapID)
	if err == nil {
		for creatureRows.Next() {
			var p spawnPoint
			if err := creatureRows.Scan(&p.spawnID, &p.entry, &p.name, &p.x, &p.y, &p.z); err != nil {
				continue
			}
			p.typ = 0
			report(p)
		}
		creatureRows.Close()
	}
	gobjectRows, err := db.QueryContext(ctx, `SELECT g.guid, g.id, t.name, g.position_x, g.position_y, g.position_z
		FROM gameobject g INNER JOIN gameobject_template t ON t.entry = g.id WHERE g.map = ? ORDER BY g.guid`, mapID)
	if err == nil {
		for gobjectRows.Next() {
			var p spawnPoint
			if err := gobjectRows.Scan(&p.spawnID, &p.entry, &p.name, &p.x, &p.y, &p.z); err != nil {
				continue
			}
			p.typ = 1
			report(p)
		}
		gobjectRows.Close()
	}
	return true
}

// handleListRespawnsCommand is the documented-blocked respawns arm
// (cs_list.cpp:502-545, RBAC 860): it bottoms out in Map::GetRespawnInfo and
// the spawn-group registry, which have no Go model. RBAC-gated with an
// honest message, not a stub.
func (s *session) handleListRespawnsCommand(ctx context.Context, args []string) bool {
	if len(args) > 1 {
		return false
	}
	s.sendSysMessage("Respawn listing is unavailable: the Go tree has no live map respawn registry or spawn-group model.")
	return true
}
