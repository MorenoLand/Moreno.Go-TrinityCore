package world

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// npc command port, chunk 2: the "set" sub-table (12 arms) and the remaining
// flat arms info, playemote, say, textemote, whisper, yell, tame, spawngroup,
// despawngroup, delete item, follow, follow stop, evade, showloot
// (cs_npc.cpp). Chunk 1 (commands_npc.go) covered the "add" sub-table and
// move/delete/near.
//
// Of these 26 arms, one is partial (`set movetype`: the GUID form updates
// the creature row in the world DB, the live creature store, and the
// selection form adds the live MoveType slot, but the LoadPath /
// MotionMaster-Initialize / respawn legs have no Go bridge), eighteen are wired
// (`delete item` on the npc_vendor rows, `set factionid` on the live faction
// slot plus the template row, `set flag` on the live NPC-flags slot plus the
// template row, `set level` on the live level/health slots plus the spawn
// row's curhealth, `set model` on the live display slots plus the spawn
// row's modelid, `set phase` on the live phase slot plus the spawn row's
// phaseMask, `set entry` on the seatCreatureEntry re-seat, `set allowmove`
// on the world movement toggle, `set wanderdistance` on the live Wander /
// MoveType slots plus the spawn row, `set spawntime` on the live respawn-delay
// slot plus the spawn row, `set link` on the linked_respawn row, `set data`
// on the AI/script-name message tail, `evade` on the triggerCreatureEvade
// twin, `say`/`yell`/`textemote` on the monster-chat fan-out plus the
// trailing oneshot emote, `playemote` on the live emotestate slot plus the
// values-update broadcast, `whisper` on the direct monster-whisper packet
// to the named online player) and 7 are documented-blocked: info, tame,
// spawngroup, despawngroup, follow, follow stop and showloot need the
// creature AI/motion/loot/pet bridges or the spawn-group manager, neither
// of which exists in the Go tree.
// Selected-creature resolution itself exists (castCommandSelectedCreature,
// the getSelectedCreature twin).

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
		s.handleNPCSetEntry(ctx, rest)
	case strings.HasPrefix("factionid", sub):
		s.handleNPCSetFactionID(ctx, rest)
	case strings.HasPrefix("flag", sub):
		s.handleNPCSetFlag(ctx, rest)
	case strings.HasPrefix("level", sub):
		s.handleNPCSetLevel(ctx, rest)
	case strings.HasPrefix("link", sub):
		s.handleNPCSetLink(ctx, rest)
	case strings.HasPrefix("model", sub):
		s.handleNPCSetModel(ctx, rest)
	case strings.HasPrefix("movetype", sub):
		s.handleNPCSetMoveType(ctx, rest)
	case strings.HasPrefix("phase", sub):
		s.handleNPCSetPhase(ctx, rest)
	case strings.HasPrefix("wanderdistance", sub):
		s.handleNPCSetWanderDistance(ctx, rest)
	case strings.HasPrefix("spawntime", sub):
		s.handleNPCSetSpawnTime(ctx, rest)
	case strings.HasPrefix("data", sub):
		s.handleNPCSetData(ctx, rest)
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

// handleNPCSetAllowMove mirrors HandleNpcSetAllowMovementCommand
// (cs_npc.cpp:241): it toggles the world's creature-movement flag and
// reports the new state. The flag itself is the whole model: C++ notes at
// World.h:619 that its actions are still to be implemented, so no Go
// consumer reads it yet, and the default is movement allowed (World.cpp:115),
// which the inverted creatureMoveDisabled slot preserves at zero value.
func (s *session) handleNPCSetAllowMove(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCSetAllowMove) {
		return
	}
	if s.server == nil {
		return
	}
	disabled := !s.server.creatureMoveDisabled.Load()
	s.server.creatureMoveDisabled.Store(disabled)
	if disabled {
		s.sendSysMessage("Creature movement disabled.")
	} else {
		s.sendSysMessage("Creature movement enabled.")
	}
}

// handleNPCSetMoveType mirrors HandleNpcSetMoveTypeCommand (cs_npc.cpp:716):
// ".npc set movetype [guid] <stay|random|way> [nodel]". The GUID form updates
// the creature row's MovementType in the world DB (the live creature store);
// the selection form resolves the selected creature (non-pet, the C++ gate)
// and additionally writes the live MoveType slot. The LoadPath(0) /
// MotionMaster-Initialize / JUST_DIED-respawn legs have no Go bridge in
// either form, and nodel only changes the message, as in C++ (the waypoint
// deletion itself is commented out there too).
func (s *session) handleNPCSetMoveType(ctx context.Context, args []string) {
	const syntax = "Syntax: .npc set movetype [guid] <stay|random|way> [nodel]"
	if s.miscDeny(ctx, permissionCommandNPCSetMoveType) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if moveType, title, ok := parseNPCMoveType(args[0]); ok {
		// Selection form: ".npc set movetype <stay|random|way> [nodel]".
		motion := s.castCommandSelectedCreature()
		if motion == nil || motionIsPet(motion) {
			// C++ returns false silently here; the file's convention
			// reports the missing selection instead.
			s.sendSysMessage("Select a creature first.")
			return
		}
		db := s.npcWorldDB()
		if db == nil {
			return
		}
		nodel := len(args) > 1 && strings.EqualFold(args[1], "nodel")
		s.server.motionMu.Lock()
		guid := motion.GUID
		motion.MoveType = uint32(moveType)
		s.server.motionMu.Unlock()
		_, _ = db.ExecContext(ctx, "UPDATE creature SET MovementType = ? WHERE guid = ?", moveType, uint32(guid&0x00FFFFFF))
		if nodel {
			s.sendSysMessage(fmt.Sprintf("Move type set to %s. Waypoints not deleted.", title))
		} else {
			s.sendSysMessage(fmt.Sprintf("Move type set to %s.", title))
		}
		return
	}
	if len(args) < 2 {
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
		s.handleNPCPlayEmote(ctx, rest)
	case strings.HasPrefix("say", sub):
		s.handleNPCSay(ctx, rest)
	case strings.HasPrefix("textemote", sub):
		s.handleNPCTextEmote(ctx, rest)
	case strings.HasPrefix("whisper", sub):
		s.handleNPCWhisper(ctx, rest)
	case strings.HasPrefix("yell", sub):
		s.handleNPCYell(ctx, rest)
	case strings.HasPrefix("tame", sub):
		s.npcNeedCreature(ctx, permissionCommandNPCTame, "tame")
	case strings.HasPrefix("spawngroup", sub):
		s.handleNPCSpawnGroup(ctx)
	case strings.HasPrefix("despawngroup", sub):
		s.handleNPCDespawnGroup(ctx)
	case strings.HasPrefix("follow", sub):
		s.handleNPCFollow(ctx, rest)
	case strings.HasPrefix("evade", sub):
		s.handleNPCEvade(ctx, rest)
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

// handleNPCSetEntry mirrors HandleNpcSetEntryCommand (cs_npc.cpp:256):
// ".npc set entry <entryId>". C++ returns false on a zero entry before the
// creature check, then requires the selected unit to be a creature
// (LANG_SELECT_CREATURE) and reports LANG_DONE when Creature::UpdateEntry
// succeeds, LANG_ERROR otherwise. The live leg is the seatCreatureEntry
// re-seat (the Lua UpdateEntry twin, including its values-update broadcast);
// like C++, no spawn row is rewritten, so a respawn re-derives the entry
// from the DB row.
func (s *session) handleNPCSetEntry(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetEntry) {
		return
	}
	const syntax = "Syntax: .npc set entry <entryId>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	entry, ok := npcVendorUintArg(args[0])
	if !ok || entry == 0 {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil || !s.server.seatCreatureEntry(ctx, motion, entry) {
		s.sendSysMessage("Error.")
		return
	}
	s.sendSysMessage("Done.")
}

// npcWanderMoveType is the HandleNpcSetWanderDistanceCommand movement-type
// rule (cs_npc.cpp:840): IDLE at zero distance, RANDOM above it.
func npcWanderMoveType(dist float64) uint32 {
	if dist > 0 {
		return 1
	}
	return 0
}

// handleNPCSetWanderDistance mirrors HandleNpcSetWanderDistanceCommand
// (cs_npc.cpp:840): ".npc set wanderdistance <distance>". C++ rejects a
// negative distance (LANG_BAD_VALUE) before the creature check, then
// requires the selected creature, writes the live wander distance and the
// default movement type, and persists both columns
// (WORLD_UPD_CREATURE_WANDER_DISTANCE) before reporting
// LANG_COMMAND_WANDER_DISTANCE. The MotionMaster-Initialize and
// JUST_DIED-respawn legs have no Go bridge; the live legs are the motion
// Wander slot (read by the random-movement generator) and MoveType.
func (s *session) handleNPCSetWanderDistance(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetSpawnDist) {
		return
	}
	const syntax = "Syntax: .npc set wanderdistance <distance>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	dist, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		s.sendSysMessage(syntax)
		return
	}
	if dist < 0 {
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
	moveType := npcWanderMoveType(dist)
	s.server.motionMu.Lock()
	guid := motion.GUID
	motion.Wander = dist
	motion.MoveType = moveType
	s.server.motionMu.Unlock()
	_, _ = db.ExecContext(ctx, "UPDATE creature SET wander_distance = ?, MovementType = ? WHERE guid = ?", dist, moveType, uint32(guid&0x00FFFFFF))
	s.sendSysMessage(fmt.Sprintf("Wander distance set to %g.", dist))
}

// handleNPCSetSpawnTime mirrors HandleNpcSetSpawnTimeCommand (cs_npc.cpp:882):
// ".npc set spawntime <seconds>". C++ requires the selected creature, then
// persists the spawn row (WORLD_UPD_CREATURE_SPAWN_TIME_SECS, the
// spawntimesecs column the respawn-delay reader consults) and writes the live
// respawn delay (the SetRespawnDelay twin's slot, consumed at death and
// cleared on respawn) before reporting LANG_COMMAND_SPAWNTIME.
func (s *session) handleNPCSetSpawnTime(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetSpawnTime) {
		return
	}
	const syntax = "Syntax: .npc set spawntime <seconds>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	secs, ok := npcVendorUintArg(args[0])
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
	guid := motion.GUID
	motion.RespawnDelaySec = secs
	motion.RespawnDelayKnown = true
	s.server.motionMu.Unlock()
	_, _ = db.ExecContext(ctx, "UPDATE creature SET spawntimesecs = ? WHERE guid = ?", secs, uint32(guid&0x00FFFFFF))
	s.sendSysMessage(fmt.Sprintf("Spawn time set to %d seconds.", secs))
}

// npcLinkBlocked mirrors the ObjectMgr::SetCreatureLinkedRespawn
// (ObjectMgr.cpp:1939) failure legs: the maps must match, the master's map
// must be instanceable, and the spawn masks must overlap. Every failure
// collapses to the command's "can't link" message in C++, so the caller
// needs only the boolean.
func npcLinkBlocked(masterMap, slaveMap, masterMask, slaveMask uint32, masterInstanceable bool) bool {
	return masterMap != slaveMap || !masterInstanceable || masterMask&slaveMask == 0
}

// handleNPCSetLink mirrors HandleNpcSetLinkCommand (cs_npc.cpp:1275): the
// selected creature must have a spawn row (a temp summon reports "isn't in
// creature table"), a zero linkguid unlinks (the DELETE leg of
// SetCreatureLinkedRespawn), and a live link is the REPLACE leg over
// (guid, linkedGuid, linkType 0). No in-memory store update is needed: the
// respawn tick's linked-deferral leg reads the linked_respawn row directly,
// so the row takes live effect at the next respawn check.
func (s *session) handleNPCSetLink(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetLink) {
		return
	}
	const syntax = "Syntax: .npc set link <linkguid>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	link, ok := npcVendorUintArg(args[0])
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
	guid := uint32(motion.GUID & 0x00FFFFFF)
	var masterMap, masterMask uint32
	if err := db.QueryRowContext(ctx, "SELECT map, spawnMask FROM creature WHERE guid = ?", guid).Scan(&masterMap, &masterMask); err != nil {
		s.sendSysMessage(fmt.Sprintf("Selected creature %d isn't in creature table", guid))
		return
	}
	if link == 0 {
		_, _ = db.ExecContext(ctx, "DELETE FROM linked_respawn WHERE guid = ? AND linkType = 0", guid)
		s.sendSysMessage(fmt.Sprintf("LinkGUID '%d' added to creature with DBTableGUID: '%d'", link, guid))
		return
	}
	var slaveMap, slaveMask uint32
	if err := db.QueryRowContext(ctx, "SELECT map, spawnMask FROM creature WHERE guid = ?", link).Scan(&slaveMap, &slaveMask); err != nil {
		s.sendSysMessage(fmt.Sprintf("Selected creature can't link with guid '%d'", link))
		return
	}
	instanceable := false
	if s.server != nil && s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(masterMap); err == nil && found {
			instanceable = mapInfo.InstanceType != 0
		}
	}
	if npcLinkBlocked(masterMap, slaveMap, masterMask, slaveMask, instanceable) {
		s.sendSysMessage(fmt.Sprintf("Selected creature can't link with guid '%d'", link))
		return
	}
	_, _ = db.ExecContext(ctx, "REPLACE INTO linked_respawn (guid, linkedGuid, linkType) VALUES (?, ?, 0)", guid, link)
	s.sendSysMessage(fmt.Sprintf("LinkGUID '%d' added to creature with DBTableGUID: '%d'", link, guid))
}

// npcSetDataAIOrScript mirrors the HandleNpcSetDataCommand message tail
// (cs_npc.cpp:445): the AI name wins, else the script name, else the
// no-AI-or-script text.
func npcSetDataAIOrScript(aiName, scriptName string) string {
	if aiName != "" {
		return "AI type: " + aiName
	}
	if scriptName != "" {
		return "Script Name: " + scriptName
	}
	return "No AI or Script Name Set"
}

// handleNPCSetData mirrors HandleNpcSetDataCommand (cs_npc.cpp:445). The
// SetData call itself has no Go dispatch: no creature event carries it
// (scripting runtime events 1-37 have no SetData hook), and the C++ base
// ScriptedAI::SetData is a no-op unless the script overrides it, which a Go
// Lua script cannot. The wired leg is the command's own effect: the message
// with the creature's GUID, entry, name, data pair and AI/script-name tail.
// The LANG_NPC_SETDATA wording is a plain-English recall (no in-tree seed).
func (s *session) handleNPCSetData(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetData) {
		return
	}
	const syntax = "Syntax: .npc set data <data1> <data2>"
	if len(args) != 2 {
		s.sendSysMessage(syntax)
		return
	}
	data1, ok1 := npcVendorUintArg(args[0])
	data2, ok2 := npcVendorUintArg(args[1])
	if !ok1 || !ok2 {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	guid, entry, name, aiName, scriptName := uint32(motion.GUID&0x00FFFFFF), motion.Entry, motion.Name, motion.AIName, motion.ScriptName
	s.server.motionMu.Unlock()
	s.sendSysMessage(fmt.Sprintf("Set data %d %d for creature %s (GUID %d, entry %d): %s", data1, data2, name, guid, entry, npcSetDataAIOrScript(aiName, scriptName)))
}

// npcEvadeParse parses the optional ".npc evade [why] [force]" tail: a
// numeric why is accepted and ignored (CreatureAI::_EnterEvadeMode ignores
// its why argument), "force" arms the clear-then-evade leg, anything else
// is a parser error.
func npcEvadeParse(args []string) (force bool, ok bool) {
	for _, arg := range args {
		if strings.EqualFold(arg, "force") {
			force = true
			continue
		}
		if _, good := npcVendorUintArg(arg); !good {
			return false, false
		}
	}
	return force, true
}

// handleNPCEvade mirrors HandleNpcEvadeCommand (cs_npc.cpp:1114) on the
// triggerCreatureEvade twin: a selected non-pet creature, silent when dead
// (C++ _EnterEvadeMode refuses dead creatures) or already evading without
// force (C++ IsInEvadeMode refuses the re-entry), and the force leg clears
// the evade state first so the evade re-runs. The IsAIEnabled gate is
// vacuous in Go (no AI-disable model; every motion is AI-enabled), and the
// charmed-creature owner-follow leg of C++ EnterEvadeMode is unmodeled (the
// generic reset runs instead).
func (s *session) handleNPCEvade(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCEvade) {
		return
	}
	force, ok := npcEvadeParse(args)
	if !ok {
		s.sendSysMessage("Syntax: .npc evade [why] [force]")
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil || motionIsPet(motion) {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	evading, dead := motion.Evading, motion.Health == 0
	if force {
		motion.Evading = false
	}
	s.server.motionMu.Unlock()
	if dead || (evading && !force) {
		return
	}
	s.server.triggerCreatureEvade(ctx, motion, time.Now())
}

// npcCreatureChat is the Unit::Talk fan-out (Unit.cpp:14040) for the npc
// say/yell/textemote/whisper arms: a monster chat packet built from the
// motion GUID/name, fanned out in the CONFIG_LISTEN_RANGE_* radius via the
// set-within-phase path the creature-text rows use. Whisper callers pass
// no range and write direct instead; see handleNPCWhisper.
func (s *session) npcCreatureChat(motion *creatureMotion, chatType uint8, language uint32, message string, listenRange float64) {
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.server.motionMu.Unlock()
	packet := protocol.BuildMonsterChatMessage(chatType, language, motion.GUID, motion.Name, message)
	s.server.broadcastToSetWithinFromPhase(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, mapID, instanceID, x, y, z, listenRange, phase)
}

// npcCreatureEmote is the Unit::HandleEmoteCommand fan-out (Unit.cpp):
// SMSG_EMOTE (u32 emote + u64 guid) to the visibility set, the PerformEmote
// twin's path.
func (s *session) npcCreatureEmote(motion *creatureMotion, emote uint32) {
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.server.motionMu.Unlock()
	packet := protocol.NewBuffer(12)
	packet.WriteU32(emote)
	packet.WriteU64(motion.GUID)
	s.server.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), mapID, instanceID, x, y, z, phase)
}

// npcSayOneshotEmote is the HandleNpcSayCommand trailing-emote rule
// (cs_npc.cpp:899): '?' asks, '!' exclaims, anything else talks
// (SharedDefines.h: EMOTE_ONESHOT_TALK=1, EXCLAMATION=5, QUESTION=6).
func npcSayOneshotEmote(text string) uint32 {
	switch text[len(text)-1] {
	case '?':
		return 6
	case '!':
		return 5
	default:
		return 1
	}
}

// handleNPCSay mirrors HandleNpcSayCommand (cs_npc.cpp:899): ".npc say
// <text>". Empty text returns false in C++ (the syntax arm here); the
// creature Says with LANG_UNIVERSAL, then plays the trailing oneshot emote.
func (s *session) handleNPCSay(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSay) {
		return
	}
	const syntax = "Syntax: .npc say <text>"
	text := strings.Join(args, " ")
	if text == "" {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.npcCreatureChat(motion, 0x0C, languageUniversal, text, float64(s.server.Config.ChatListenRangeSay))
	s.npcCreatureEmote(motion, npcSayOneshotEmote(text))
}

// handleNPCYell mirrors HandleNpcYellCommand (cs_npc.cpp:1003): ".npc yell
// <text>". Same shape as say, with MONSTER_YELL and the shout oneshot
// (EMOTE_ONESHOT_SHOUT=22).
func (s *session) handleNPCYell(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCYell) {
		return
	}
	const syntax = "Syntax: .npc yell <text>"
	text := strings.Join(args, " ")
	if text == "" {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.npcCreatureChat(motion, 0x0E, languageUniversal, text, float64(s.server.Config.ChatListenRangeYell))
	s.npcCreatureEmote(motion, 22)
}

// handleNPCTextEmote mirrors HandleNpcTextEmoteCommand (cs_npc.cpp:926):
// ".npc textemote <text>". Unit::TextEmote with isBossEmote=false, so
// MONSTER_EMOTE (0x10) at the text-emote listen range; no trailing emote.
func (s *session) handleNPCTextEmote(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCTextEmote) {
		return
	}
	const syntax = "Syntax: .npc textemote <text>"
	text := strings.Join(args, " ")
	if text == "" {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.npcCreatureChat(motion, 0x10, languageUniversal, text, float64(s.server.Config.ChatListenRangeTextEmote))
}

// handleNPCPlayEmote mirrors HandleNpcPlayEmoteCommand (cs_npc.cpp:662):
// ".npc playemote <emote>". SetUInt32Value(UNIT_NPC_EMOTESTATE) on the
// selected creature: the live slot plus the values-update broadcast, the
// EmoteState twin's path. C++ returns true silently; so does this arm.
func (s *session) handleNPCPlayEmote(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCPlayEmote) {
		return
	}
	const syntax = "Syntax: .npc playemote <emote>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	emote, ok := npcVendorUintArg(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	motion.EmoteState = emote
	motion.EmoteStateKnown = true
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCEmoteState: emote})
}

// handleNPCWhisper mirrors HandleNpcWhisperCommand (cs_npc.cpp:981):
// ".npc whisper <player> <text>". C++ checks empty text, then the selected
// creature, then HasLowerSecurity on the FindPlayerByName receiver: an
// offline/unknown name reports LANG_PLAYER_NOT_FOUND, and a receiver whose
// account outranks the handler fails. Unit::Whisper writes the
// MONSTER_WHISPER (0x0F) packet direct to the receiver session only.
func (s *session) handleNPCWhisper(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCWhisper) {
		return
	}
	const syntax = "Syntax: .npc whisper <player> <text>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	text := strings.Join(args[1:], " ")
	if text == "" {
		s.sendSysMessage(syntax)
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	if s.server == nil {
		return
	}
	receiver := s.server.findSessionByName(args[0])
	if receiver == nil || receiver.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	if s.security < s.accountSecurityLevel(ctx, receiver.accountID) {
		return
	}
	packet := protocol.BuildMonsterChatMessage(0x0F, languageUniversal, motion.GUID, motion.Name, text)
	_ = receiver.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
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
