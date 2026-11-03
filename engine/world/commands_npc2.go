package world

import (
	"context"
	"fmt"
	"strings"
)

// npc command port, chunk 2: the "set" sub-table (12 arms) and the remaining
// flat arms info, playemote, say, textemote, whisper, yell, tame, spawngroup,
// despawngroup, delete item, follow, follow stop, evade, showloot
// (cs_npc.cpp). Chunk 1 (commands_npc.go) covered the "add" sub-table and
// move/delete/near.
//
// Of these 26 arms, one is partial-native (`set movetype` with an explicit
// GUID: the creature row is updated in the world DB, which is the live
// creature store in the Go tree), two are wired (`delete item` on the
// npc_vendor rows, `set factionid` on the live faction slot plus the
// template row) and 23 are documented-blocked: every other arm needs the
// creature AI/motion/loot/pet bridges or the spawn-group manager, none of
// which exist in the Go tree. Selected-creature resolution itself exists
// (castCommandSelectedCreature, the getSelectedCreature twin).

// handleNPCSetTable dispatches the "set" sub-table (cs_npc.cpp:72-86).
// The "set" node is a bare SubCommandEntry container (ChatCommand.h:248-251)
// with no permission of its own, and RBAC_PERM_COMMAND_NPC_SET (RBAC.h:448,
// 580) is referenced by no handler in C++ — so a bare ".npc set" prints the
// help listing with no gate, same as the mmap container-root fix.
func (s *session) handleNPCSetTable(ctx context.Context, args []string) {
	const syntax = "Syntax: .npc set allowmove|entry|factionid|flag|level|link|model|movetype|phase|wanderdistance|spawntime|data <args>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("allowmove", sub):
		s.handleNPCSetAllowMove(ctx)
	case strings.HasPrefix("entry", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetEntry, "set entry")
	case strings.HasPrefix("factionid", sub):
		s.handleNPCSetFactionID(ctx, rest)
	case strings.HasPrefix("flag", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetFlag, "set flag")
	case strings.HasPrefix("level", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetLevel, "set level")
	case strings.HasPrefix("link", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetLink, "set link")
	case strings.HasPrefix("model", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetModel, "set model")
	case strings.HasPrefix("movetype", sub):
		s.handleNPCSetMoveType(ctx, rest)
	case strings.HasPrefix("phase", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetPhase, "set phase")
	case strings.HasPrefix("wanderdistance", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetSpawnDist, "set wanderdistance")
	case strings.HasPrefix("spawntime", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetSpawnTime, "set spawntime")
	case strings.HasPrefix("data", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetData, "set data")
	default:
		s.sendSysMessage(syntax)
	}
}

// npcNeedCreature is the shared documented-blocked tail for the npc arms that
// need the selected live creature (ChatHandler::getSelectedCreature): command
// selections resolve only to online player sessions.
func (s *session) npcNeedCreature(ctx context.Context, perm uint32, what string) {
	if s.miscDeny(ctx, perm) {
		return
	}
	s.sendSysMessage(fmt.Sprintf("npc %s is not supported: selected-unit creature targets have no Go bridge.", what))
}

// handleNPCSetAllowMove is documented-blocked (cs_npc.cpp:231): it toggles
// the world's creature-movement flag, but the Go tree has no creature
// movement model or allowMovement config.
func (s *session) handleNPCSetAllowMove(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCSetAllowMove) {
		return
	}
	s.sendSysMessage("npc set allowmove is not supported: creature movement has no Go bridge.")
}

// handleNPCSetMoveType mirrors HandleNpcSetMoveTypeCommand (cs_npc.cpp:688)
// for the GUID form: ".npc set movetype <guid> <stay|random|way> [nodel]".
// The creature row's MovementType is updated in the world DB (the live
// creature store); the live motion-master/AI legs have no Go bridge. The
// selection form is blocked like the other set arms.
func (s *session) handleNPCSetMoveType(ctx context.Context, args []string) {
	const syntax = "Syntax: .npc set movetype <guid> <stay|random|way> [nodel]"
	if s.miscDeny(ctx, permissionCommandNPCSetMoveType) {
		return
	}
	if len(args) < 2 {
		// Without a GUID the arm needs the selected creature.
		s.sendSysMessage("npc set movetype needs a creature GUID: selected-unit creature targets have no Go bridge.")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	guid := uint32(cAtoi(args[0]))
	moveType, title, ok := parseNPCMoveType(args[1])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	nodel := len(args) > 2 && strings.EqualFold(args[2], "nodel")
	var exists uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE guid = ?", guid).Scan(&exists); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return
	}
	if _, err := db.ExecContext(ctx, "UPDATE creature SET MovementType = ? WHERE guid = ?", moveType, guid); err != nil {
		return
	}
	// The waypoint-deletion leg is commented out in C++ too; nodel only
	// changes the message. LANG_MOVE_TYPE_SET 257 / _NODEL 258.
	if nodel {
		s.sendSysMessage(fmt.Sprintf("Move type set to %s. Waypoints not deleted.", title))
	} else {
		s.sendSysMessage(fmt.Sprintf("Move type set to %s.", title))
	}
}

// parseNPCMoveType maps stay|random|way to the MovementGeneratorType values
// (MovementDefines.h: stay=IDLE 0, random=RANDOM 1, way=WAYPOINT 2).
func parseNPCMoveType(arg string) (uint8, string, bool) {
	switch strings.ToLower(arg) {
	case "stay":
		return 0, "Idle", true
	case "random":
		return 1, "Random", true
	case "way":
		return 2, "Waypoint", true
	}
	return 0, "", false
}

// handleNPCChunk2 extends the npc dispatcher with the chunk-2 arms. It
// returns true when the sub-command was handled.
func (s *session) handleNPCChunk2(ctx context.Context, sub string, rest []string) bool {
	switch {
	case strings.HasPrefix("set", sub):
		s.handleNPCSetTable(ctx, rest)
	case strings.HasPrefix("info", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCInfo, "info")
	case strings.HasPrefix("playemote", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCPlayEmote, "playemote")
	case strings.HasPrefix("say", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSay, "say")
	case strings.HasPrefix("textemote", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCTextEmote, "textemote")
	case strings.HasPrefix("whisper", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCWhisper, "whisper")
	case strings.HasPrefix("yell", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCYell, "yell")
	case strings.HasPrefix("tame", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCTame, "tame")
	case strings.HasPrefix("spawngroup", sub):
		s.handleNPCSpawnGroup(ctx)
	case strings.HasPrefix("despawngroup", sub):
		s.handleNPCDespawnGroup(ctx)
	case strings.HasPrefix("follow", sub):
		s.handleNPCFollow(ctx, rest)
	case strings.HasPrefix("evade", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCEvade, "evade")
	case strings.HasPrefix("showloot", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCShowLoot, "showloot")
	default:
		return false
	}
	return true
}

// npcDeleteSub handles the two-level "delete item" arm (cs_npc.cpp:108:
// { "delete item", HandleNpcDeleteVendorItemCommand, ... }); plain "delete"
// falls through to the chunk-1 guid arm.
func (s *session) npcDeleteSub(ctx context.Context, rest []string) {
	if len(rest) > 0 && strings.HasPrefix("item", strings.ToLower(rest[0])) {
		s.handleNPCDeleteItem(ctx, rest[1:])
		return
	}
	s.handleNPCDelete(ctx, rest)
}

// handleNPCDeleteItem mirrors HandleNpcDeleteVendorItemCommand
// (cs_npc.cpp:349): ".npc delete item <itemId|itemLink>". C++ checks the
// vendor first (a missing selection or a non-vendor reports
// LANG_COMMAND_VENDORSELECTION), then the nil item
// (LANG_COMMAND_NEEDITEMSEND). The live vendor flag is the motion value
// when a Lua arm overrode it (NPCFlagsKnown, the session-snapshot rule)
// and the template value otherwise. Removal is the WORLD_DEL_NPC_VENDOR
// delete over every row for (entry, item), the luaVendorRemoveItem
// statement; zero rows removed reports LANG_ITEM_NOT_IN_LIST, success
// reports LANG_ITEM_DELETED_FROM_LIST. The numeric-id parser error runs
// before the body in C++, so it precedes the vendor check here too, while
// the link-to-nothing nil-item message keeps its post-vendor position.
func (s *session) handleNPCDeleteItem(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCDeleteItem) {
		return
	}
	const syntax = "Syntax: .npc delete item <itemId|itemLink>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	itemID, link, ok := npcItemTokenID(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	var itemName string
	itemFound := db.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemID).Scan(&itemName) == nil
	if !link && !itemFound {
		s.sendSysMessage(fmt.Sprintf("Item %d does not exist.", itemID))
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("You need to select a vendor.")
		return
	}
	s.server.motionMu.Lock()
	entry, npcKnown, liveFlags := motion.Entry, motion.NPCFlagsKnown, motion.NPCFlags
	s.server.motionMu.Unlock()
	flags := liveFlags
	if !npcKnown {
		if err := db.QueryRowContext(ctx, "SELECT COALESCE(npcflag, 0) FROM creature_template WHERE entry = ?", entry).Scan(&flags); err != nil {
			s.sendSysMessage("You need to select a vendor.")
			return
		}
	}
	if flags&unitNPCFlagVendor == 0 {
		s.sendSysMessage("You need to select a vendor.")
		return
	}
	if link && !itemFound {
		s.sendSysMessage("You need to send an item.")
		return
	}
	res, err := db.ExecContext(ctx, "DELETE FROM npc_vendor WHERE entry = ? AND item = ?", entry, itemID)
	if err != nil {
		return
	}
	removed, _ := res.RowsAffected()
	if removed == 0 {
		s.sendSysMessage(fmt.Sprintf("Item %d is not in the list.", itemID))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Item %d (%s) deleted from vendor list.", itemID, itemName))
}

// handleNPCSetFactionID mirrors HandleNpcSetFactionIdCommand (cs_npc.cpp:381):
// ".npc set factionid <factionId>". C++ validates the FactionTemplate.dbc
// row BEFORE the creature check (LANG_WRONG_FACTION), then requires the
// selected creature (LANG_SELECT_CREATURE). The live write is the creature
// SetFaction leg (motion Faction slot + values-update broadcast, the
// modify-faction pattern); the persistent leg is the
// WORLD_UPD_CREATURE_FACTION update of creature_template for the entry,
// which Go spawns read from the DB, so no in-memory template mutation is
// needed. Silent on success, as in C++.
func (s *session) handleNPCSetFactionID(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetFactionID) {
		return
	}
	const syntax = "Syntax: .npc set factionid <factionId>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	faction, ok := npcVendorUintArg(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	if _, found := s.factionTemplateEntry(faction); !found {
		s.sendSysMessage(fmt.Sprintf("Wrong faction %d.", faction))
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	s.server.motionMu.Lock()
	entry, mapID, instanceID, guid := motion.Entry, motion.Map, motion.InstanceID, motion.GUID
	motion.Faction = faction
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldFaction: faction})
	_, _ = db.ExecContext(ctx, "UPDATE creature_template SET faction = ? WHERE entry = ?", faction, entry)
}

// handleNPCFollow handles "follow" and the two-level "follow stop" arm
// (cs_npc.cpp:109-110, gated by RBAC_PERM_COMMAND_NPC_FOLLOW 578 and
// RBAC_PERM_COMMAND_NPC_FOLLOW_STOP 579 respectively).
func (s *session) handleNPCFollow(ctx context.Context, rest []string) {
	if len(rest) > 0 && strings.HasPrefix("stop", strings.ToLower(rest[0])) {
		s.npcNeedCreature(ctx, permissionCommandNPCFollowStop, "follow stop")
		return
	}
	s.npcNeedCreature(ctx, permissionCommandNPCFollow, "follow")
}

// handleNPCSpawnGroup is documented-blocked (cs_npc.cpp:1331): spawn groups
// need the Map::SpawnGroupSpawn bridge. This also closes the spawngroup/
// despawngroup deferral left by the gobject port.
func (s *session) handleNPCSpawnGroup(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCSpawnGroup) {
		return
	}
	s.sendSysMessage("npc spawngroup is not supported: the spawn-group manager has no Go bridge.")
}

// handleNPCDespawnGroup is documented-blocked (cs_npc.cpp:1362): same bridge
// gap as spawngroup.
func (s *session) handleNPCDespawnGroup(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCDespawnGroup) {
		return
	}
	s.sendSysMessage("npc despawngroup is not supported: the spawn-group manager has no Go bridge.")
}
