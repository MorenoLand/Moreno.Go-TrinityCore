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
// creature store in the Go tree), six are wired (`delete item` on the
// npc_vendor rows, `set factionid` on the live faction slot plus the
// template row, `set flag` on the live NPC-flags slot plus the template
// row, `set level` on the live level/health slots plus the spawn row's
// curhealth, `set model` on the live display slots plus the spawn row's
// modelid, `set phase` on the live phase slot plus the spawn row's
// phaseMask) and 19 are documented-blocked: every other arm needs the
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
		s.handleNPCSetFlag(ctx, rest)
	case strings.HasPrefix("level", sub):
		s.handleNPCSetLevel(ctx, rest)
	case strings.HasPrefix("link", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCSetLink, "set link")
	case strings.HasPrefix("model", sub):
		s.handleNPCSetModel(ctx, rest)
	case strings.HasPrefix("movetype", sub):
		s.handleNPCSetMoveType(ctx, rest)
	case strings.HasPrefix("phase", sub):
		s.handleNPCSetPhase(ctx, rest)
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

// handleNPCSetFlag mirrors HandleNpcSetFlagCommand (cs_npc.cpp:419):
// ".npc set flag <npcFlags>". C++ requires the selected creature
// (LANG_SELECT_CREATURE), then REPLACES UNIT_NPC_FLAGS with the given
// value (SetUInt32Value, not an OR) and persists it with
// WORLD_UPD_CREATURE_NPCFLAG (UPDATE creature_template SET npcflag
// WHERE entry), reporting LANG_VALUE_SAVED_REJOIN on success. The live
// write sets NPCFlagsKnown so the motion->spawn overlay keeps the live
// value on re-stream (the SetNPCFlags twin).
func (s *session) handleNPCSetFlag(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetFlag) {
		return
	}
	const syntax = "Syntax: .npc set flag <npcFlags>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	flags, ok := npcVendorUintArg(args[0])
	if !ok {
		s.sendSysMessage(syntax)
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
	motion.NPCFlags = flags
	motion.NPCFlagsKnown = true
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCFlags: flags})
	_, _ = db.ExecContext(ctx, "UPDATE creature_template SET npcflag = ? WHERE entry = ?", flags, entry)
	s.sendSysMessage("Value saved, please rejoin.")
}

// npcSetLevelHealth is the HandleNpcSetLevelCommand health rule
// (cs_npc.cpp:277): SetMaxHealth and SetHealth both take 100 + 30*lvl.
func npcSetLevelHealth(lvl uint32) uint32 { return 100 + 30*lvl }

// handleNPCSetLevel mirrors HandleNpcSetLevelCommand (cs_npc.cpp:277):
// ".npc set level <lvl>". C++ gates lvl to 1..CONFIG_MAX_PLAYER_LEVEL+3
// (LANG_BAD_VALUE), then requires a selected non-pet creature
// (LANG_SELECT_CREATURE), sets MaxHealth and Health to 100+30*lvl and
// Level, then SaveToDB. Level itself has no creature-row column, so the
// persistent leg is the spawn row's curhealth; a respawn re-derives the
// level from the template, and a temp summon gains no spawn row from the
// targeted UPDATE (C++'s full SaveToDB would mint one).
func (s *session) handleNPCSetLevel(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetLevel) {
		return
	}
	const syntax = "Syntax: .npc set level <lvl>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	lvl, ok := npcVendorUintArg(args[0])
	if !ok || lvl > 255 {
		s.sendSysMessage(syntax)
		return
	}
	maxLevel := uint32(80)
	if s.server != nil && s.server.Config.MaxPlayerLevel != 0 {
		maxLevel = s.server.Config.MaxPlayerLevel
	}
	if lvl < 1 || lvl > maxLevel+3 {
		s.sendSysMessage(misc2BadValue)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil || motionIsPet(motion) {
		s.sendSysMessage("Select a creature first.")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	health := npcSetLevelHealth(lvl)
	s.server.motionMu.Lock()
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	motion.Level = lvl
	motion.MaxHealth = health
	motion.Health = health
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldLevel: lvl, unitFieldHealth: health, unitFieldMaxHealth: health})
	_, _ = db.ExecContext(ctx, "UPDATE creature SET curhealth = ? WHERE guid = ?", health, uint32(guid&0x00FFFFFF))
}

// npcStoredModelID is the Creature::SaveToDB displayid rule
// (Creature.cpp:1408-1416): the spawn row stores 0 when the native
// display equals one of the template's models, else the display id.
func npcStoredModelID(display, m1, m2, m3, m4 uint32) uint32 {
	if display == m1 || display == m2 || display == m3 || display == m4 {
		return 0
	}
	return display
}

// handleNPCSetModel mirrors HandleNpcSetModelCommand (cs_npc.cpp:678):
// ".npc set model <displayId>". C++ requires a selected non-pet
// creature (LANG_SELECT_CREATURE), then gates the id on the
// CreatureDisplayInfo.dbc row (LANG_COMMAND_INVALID_PARAM), sets
// DisplayId and NativeDisplayId live, then SaveToDB (the spawn row's
// modelid, 0 when the display equals a template model). The gender leg
// of Unit::SetDisplayId has no Go creature slot and stays unmodeled.
func (s *session) handleNPCSetModel(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetModel) {
		return
	}
	const syntax = "Syntax: .npc set model <displayId>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	display, ok := npcVendorUintArg(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil || motionIsPet(motion) {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if !s.mountDisplayValid(display) {
		s.sendSysMessage(fmt.Sprintf("Invalid parameter '%d'.", display))
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	s.server.motionMu.Lock()
	entry, mapID, instanceID, guid := motion.Entry, motion.Map, motion.InstanceID, motion.GUID
	motion.DisplayID = display
	motion.DisplayIDKnown = true
	motion.NativeDisplayID = display
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldDisplayID: display, unitFieldNativeDisplayID: display})
	var m1, m2, m3, m4 uint32
	_ = db.QueryRowContext(ctx, "SELECT modelid1, modelid2, modelid3, modelid4 FROM creature_template WHERE entry = ?", entry).Scan(&m1, &m2, &m3, &m4)
	_, _ = db.ExecContext(ctx, "UPDATE creature SET modelid = ? WHERE guid = ?", npcStoredModelID(display, m1, m2, m3, m4), uint32(guid&0x00FFFFFF))
}

// handleNPCSetPhase mirrors HandleNpcSetPhaseCommand (cs_npc.cpp:814):
// ".npc set phase <phasemask>". C++ rejects a zero mask BEFORE the
// creature check (LANG_BAD_VALUE), then requires the selected creature
// (LANG_SELECT_CREATURE), writes the live phase mask, and runs SaveToDB
// only for non-pets (the spawn row's phaseMask). No visibility fan-out,
// the modify-phase delta.
func (s *session) handleNPCSetPhase(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetPhase) {
		return
	}
	const syntax = "Syntax: .npc set phase <phasemask>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	mask, ok := npcVendorUintArg(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	if mask == 0 {
		s.sendSysMessage(misc2BadValue)
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
	motion.PhaseMask = mask
	s.server.motionMu.Unlock()
	if !motionIsPet(motion) {
		_, _ = db.ExecContext(ctx, "UPDATE creature SET phaseMask = ? WHERE guid = ?", mask, uint32(motion.GUID&0x00FFFFFF))
	}
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
