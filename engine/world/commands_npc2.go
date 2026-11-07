package world

import (
	"context"
	"fmt"
	"math"
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
// All 26 arms are wired (`set movetype` on the live MoveType slot plus the
// LoadPath(0) / MotionMaster-Initialize / JUST_DIED-respawn legs, the GUID
// form running the same live legs for a loaded creature and messaging only
// for an offline spawn row, `delete item` on the npc_vendor rows, `set factionid` on the live faction
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
// to the named online player, `follow`/`follow stop` on the script-follow
// ACTIVE slot, `info` on the live motion slots plus the template/spawn rows,
// `tame` on the createTamedPetFromEntry pet-creation path, `showloot` on
// the creatureLoot store, `spawngroup`/`despawngroup` on the spawn-group
// manager).
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
	// TryExecuteCommand (ChatCommand.cpp:246-300): a partial token matching
	// more than one visible subcommand reports LANG_SUBCMD_AMBIGUOUS and
	// lists the candidates; an exact match wins. The root resolve does
	// this at the npc level already; the set sub-table did not.
	setSubs := []string{"allowmove", "entry", "factionid", "flag", "level", "link", "model", "movetype", "phase", "wanderdistance", "spawntime", "data"}
	exact := false
	var matches []string
	for _, name := range setSubs {
		if name == sub {
			exact = true
			break
		}
		if strings.HasPrefix(name, sub) {
			matches = append(matches, name)
		}
	}
	if !exact {
		if len(matches) != 1 {
			if len(matches) > 1 {
				s.sendSysMessage(fmt.Sprintf("Ambiguous subcommand %q: %s", args[0], strings.Join(matches, ", ")))
				return
			}
			s.sendSysMessage(syntax)
			return
		}
	}
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
// ".npc set movetype [guid] <stay|random|way> [nodel]". The selection form
// resolves the selected creature (non-pet, the C++ gate), writes the live
// MoveType slot, then runs npcMovementReset: LoadPath(0) when nodel is
// absent (the waypoint path
// id reset to 0, re-reading waypoint_data id 0), MotionMaster-Initialize
// (the in-flight default-generator state cleared; the step tick re-derives
// the generator from the MoveType slot), and the JUST_DIED-respawn leg for
// an alive creature (home snap at full health), before the SaveToDB twin
// (MovementType plus the post-respawn position). The GUID form runs the
// same live legs for a creature loaded in the GM's map/instance
// (GetCreatureFromPlayerMapByDbGuid, no pet gate on this form); with no
// live creature C++ performs no SaveToDB, so an existing spawn row is only
// validated and messaged, never updated. Nodel skips the LoadPath(0) leg
// (cs_npc.cpp:716: LoadPath runs only when doNotDelete is false) and
// changes the message; the waypoint deletion itself is commented out in
// C++ too.
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
		nodel := false
		if len(args) > 1 {
			if len(args) > 2 || !strings.EqualFold(args[1], "nodel") {
				s.sendSysMessage(syntax)
				return
			}
			nodel = true
		}
		s.server.motionMu.Lock()
		guid := motion.GUID
		motion.MoveType = uint32(moveType)
		s.server.motionMu.Unlock()
		s.npcMovementReset(ctx, motion, !nodel)
		s.server.motionMu.Lock()
		homeX, homeY, homeZ, homeO := motion.X, motion.Y, motion.Z, motion.Orientation
		s.server.motionMu.Unlock()
		_, _ = db.ExecContext(ctx, "UPDATE creature SET MovementType = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?", moveType, homeX, homeY, homeZ, homeO, uint32(guid&0x00FFFFFF))
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
	guid, ok := goParseLinkOrUint(args[0], "creature")
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	moveType, title, ok := parseNPCMoveType(args[1])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	nodel := false
	if len(args) > 2 {
		if len(args) > 3 || !strings.EqualFold(args[2], "nodel") {
			s.sendSysMessage(syntax)
			return
		}
		nodel = true
	}
	// A creature loaded in the GM's map/instance runs the live legs
	// (ChatHandler::GetCreatureFromPlayerMapByDbGuid); unlike the
	// selection form, C++ applies no pet gate on this form.
	if live := s.npcLiveCreatureBySpawnGUID(guid); live != nil {
		s.server.motionMu.Lock()
		live.MoveType = uint32(moveType)
		s.server.motionMu.Unlock()
		s.npcMovementReset(ctx, live, !nodel)
		s.server.motionMu.Lock()
		homeX, homeY, homeZ, homeO := live.X, live.Y, live.Z, live.Orientation
		s.server.motionMu.Unlock()
		_, _ = db.ExecContext(ctx, "UPDATE creature SET MovementType = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?", moveType, homeX, homeY, homeZ, homeO, guid)
		if nodel {
			s.sendSysMessage(fmt.Sprintf("Move type set to %s. Waypoints not deleted.", title))
		} else {
			s.sendSysMessage(fmt.Sprintf("Move type set to %s.", title))
		}
		return
	}
	var exists uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE guid = ?", guid).Scan(&exists); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return
	}
	// No live creature means no SaveToDB in C++: the type change is lost
	// and only the message is printed, so the row is left untouched.
	// The waypoint-deletion leg is commented out in C++ too; nodel only
	// changes the message. LANG_MOVE_TYPE_SET 257 / _NODEL 258.
	if nodel {
		s.sendSysMessage(fmt.Sprintf("Move type set to %s. Waypoints not deleted.", title))
	} else {
		s.sendSysMessage(fmt.Sprintf("Move type set to %s.", title))
	}
}

// npcMovementReset mirrors the MotionMaster-Initialize + JUST_DIED-respawn
// tail shared by HandleNpcSetMoveTypeCommand (cs_npc.cpp:716) and
// HandleNpcSetWanderDistanceCommand (cs_npc.cpp:840). Initialize clears the
// in-flight default-generator state (the step tick re-derives the generator
// from the MoveType slot each tick, so clearing the flight state is the
// whole Initialize leg); the respawn leg relocates an alive creature to its
// respawn position at full health (Creature::RemoveCorpse relocates to
// GetRespawnPosition), broadcast as a home spline plus a health values
// update on the evade-home precedent. Dead creatures keep their position
// (C++ resets their generator at respawn instead). Script chase/follow
// slots survive: they are Go's Lua surface, not C++ generator state, and
// the tick re-checks them ahead of wander/waypoint. Transport passengers
// skip the snap (C++ relocates them through the transport instead); the
// movement clear still applies.
func (s *session) npcMovementReset(ctx context.Context, motion *creatureMotion, resetPath bool) {
	if s == nil || s.server == nil || motion == nil {
		return
	}
	pathLoaded := false
	var points []waypointPoint
	if resetPath {
		if db := s.npcWorldDB(); db != nil {
			points = s.server.loadWaypoints(ctx, 0)
			pathLoaded = true
		}
	}
	s.server.motionMu.Lock()
	oldX, oldY, oldZ := motion.X, motion.Y, motion.Z
	homeX, homeY, homeZ, homeO := motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO
	oldHealth, maxHealth := motion.Health, motion.MaxHealth
	alive := motion.Health > 0
	transport := motion.TransportGUID != 0
	mapID, instanceID, guid, phase := motion.Map, motion.InstanceID, motion.GUID, motion.PhaseMask
	speed := creatureSplineVelocity(motion, false)
	if pathLoaded {
		motion.PathID = 0
		motion.Points = points
	}
	motion.NextIdx = 0
	motion.Moving = false
	motion.MoveEnds = time.Time{}
	motion.WaitUntil = time.Time{}
	snapped := false
	if alive && !transport {
		motion.X, motion.Y, motion.Z = homeX, homeY, homeZ
		motion.Orientation = homeO
		motion.Health = maxHealth
		snapped = true
	}
	s.server.motionMu.Unlock()
	if !snapped {
		return
	}
	if oldHealth != maxHealth {
		s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldHealth: maxHealth})
	}
	homeDist := float32(math.Hypot(float64(homeX-oldX), float64(homeY-oldY)))
	if homeDist > 0.5 && speed >= 0.001 {
		s.server.broadcastMonsterMoveInInstance(mapID, instanceID, guid, oldX, oldY, oldZ, homeX, homeY, homeZ, splineDurationMs(float64(homeDist), speed), false, 0, false, phase, 0)
	}
}

// npcLiveCreatureBySpawnGUID mirrors
// ChatHandler::GetCreatureFromPlayerMapByDbGuid: the live creature with
// this spawn GUID in the GM's map/instance, if any.
func (s *session) npcLiveCreatureBySpawnGUID(spawnGUID uint32) *creatureMotion {
	if s == nil || s.server == nil || s.player == nil {
		return nil
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	for _, motion := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if motion != nil && uint32(motion.GUID&0x00FFFFFF) == spawnGUID {
			return motion
		}
	}
	return nil
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
		s.handleNPCInfo(ctx)
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
		s.handleNPCTame(ctx)
	case strings.HasPrefix("spawngroup", sub):
		s.handleNPCSpawnGroup(ctx, rest)
	case strings.HasPrefix("despawngroup", sub):
		s.handleNPCDespawnGroup(ctx, rest)
	case strings.HasPrefix("follow", sub):
		s.handleNPCFollow(ctx, rest)
	case strings.HasPrefix("evade", sub):
		s.handleNPCEvade(ctx, rest)
	case strings.HasPrefix("showloot", sub):
		s.handleNPCShowLoot(ctx, rest)
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
	// WORLD_UPD_CREATURE_FACTION binds the faction as uint16
	// (cs_npc.cpp:405); the live slot stays full uint32 in both trees.
	_, _ = db.ExecContext(ctx, "UPDATE creature_template SET faction = ? WHERE entry = ?", uint16(faction), entry)
}

// npcSetFlagNames is the EnumUtils<NPCFlags> name table
// (enuminfo_UnitDefines.cpp): constant first, title second. The C++
// ArgInfo enum parse (ChatCommandArgs.h:128-210) matches a token by
// case-insensitive exact match, else unique prefix, over both spellings —
// so "UNIT_NPC_FLAG_VENDOR" sets VENDOR, "can" sets REPAIR ("can repair"
// is the only title with that prefix), but bare "vendor" matches nothing
// and "unit_npc_flag_vendor" is ambiguous across the five vendor flags.
// A numeric value is accepted only when EnumUtils::IsValid holds, which
// for this flags enum means exactly one listed flag (ToIndex is a plain
// switch: 0 and combinations such as 0x81 are parser errors).
var npcSetFlagNames = []struct {
	constant string
	title    string
	value    uint32
}{
	{"UNIT_NPC_FLAG_GOSSIP", "has gossip menu", 0x00000001},
	{"UNIT_NPC_FLAG_QUESTGIVER", "is quest giver", 0x00000002},
	{"UNIT_NPC_FLAG_UNK1", "UNIT_NPC_FLAG_UNK1", 0x00000004},
	{"UNIT_NPC_FLAG_UNK2", "UNIT_NPC_FLAG_UNK2", 0x00000008},
	{"UNIT_NPC_FLAG_TRAINER", "is trainer", 0x00000010},
	{"UNIT_NPC_FLAG_TRAINER_CLASS", "is class trainer", 0x00000020},
	{"UNIT_NPC_FLAG_TRAINER_PROFESSION", "is profession trainer", 0x00000040},
	{"UNIT_NPC_FLAG_VENDOR", "is vendor (generic)", 0x00000080},
	{"UNIT_NPC_FLAG_VENDOR_AMMO", "is vendor (ammo)", 0x00000100},
	{"UNIT_NPC_FLAG_VENDOR_FOOD", "is vendor (food)", 0x00000200},
	{"UNIT_NPC_FLAG_VENDOR_POISON", "is vendor (poison)", 0x00000400},
	{"UNIT_NPC_FLAG_VENDOR_REAGENT", "is vendor (reagents)", 0x00000800},
	{"UNIT_NPC_FLAG_REPAIR", "can repair", 0x00001000},
	{"UNIT_NPC_FLAG_FLIGHTMASTER", "is flight master", 0x00002000},
	{"UNIT_NPC_FLAG_SPIRITHEALER", "is spirit healer", 0x00004000},
	{"UNIT_NPC_FLAG_SPIRITGUIDE", "is spirit guide", 0x00008000},
	{"UNIT_NPC_FLAG_INNKEEPER", "is innkeeper", 0x00010000},
	{"UNIT_NPC_FLAG_BANKER", "is banker", 0x00020000},
	{"UNIT_NPC_FLAG_PETITIONER", "handles guild/arena petitions", 0x00040000},
	{"UNIT_NPC_FLAG_TABARDDESIGNER", "is guild tabard designer", 0x00080000},
	{"UNIT_NPC_FLAG_BATTLEMASTER", "is battlemaster", 0x00100000},
	{"UNIT_NPC_FLAG_AUCTIONEER", "is auctioneer", 0x00200000},
	{"UNIT_NPC_FLAG_STABLEMASTER", "is stable master", 0x00400000},
	{"UNIT_NPC_FLAG_GUILD_BANKER", "is guild banker", 0x00800000},
	{"UNIT_NPC_FLAG_SPELLCLICK", "has spell click enabled", 0x01000000},
	{"UNIT_NPC_FLAG_PLAYER_VEHICLE", "is player vehicle", 0x02000000},
	{"UNIT_NPC_FLAG_MAILBOX", "is mailbox", 0x04000000},
}

func npcSetFlagParse(token string) (uint32, bool) {
	for _, f := range npcSetFlagNames {
		if strings.EqualFold(token, f.constant) || strings.EqualFold(token, f.title) {
			return f.value, true
		}
	}
	var matched uint32
	found := false
	for _, f := range npcSetFlagNames {
		for _, key := range []string{f.constant, f.title} {
			if strings.HasPrefix(strings.ToLower(key), strings.ToLower(token)) {
				if found && matched != f.value {
					return 0, false
				}
				matched, found = f.value, true
			}
		}
	}
	if found {
		return matched, true
	}
	if v, err := strconv.ParseUint(token, 0, 32); err == nil {
		for _, f := range npcSetFlagNames {
			if uint32(v) == f.value {
				return uint32(v), true
			}
		}
	}
	return 0, false
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
	flags, ok := npcSetFlagParse(args[0])
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
// ".npc set entry <entryId|entryLink>" (CreatureEntry is a hyperlink
// variant). C++ returns false on a zero entry before the
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
	entry, ok := goParseLinkOrUint(args[0], "creature_entry")
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
// JUST_DIED-respawn legs ride npcMovementReset (no path reload: unlike
// movetype, C++ does not call LoadPath here); C++ performs no SaveToDB on
// this arm, so the row update stays at the two columns.
func (s *session) handleNPCSetWanderDistance(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSetSpawnDist) {
		return
	}
	const syntax = "Syntax: .npc set wanderdistance <distance>"
	if len(args) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	dist, err := parseNPCFloat(args[0])
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
	s.npcMovementReset(ctx, motion, false)
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

// npcEvadeParse parses the optional ".npc evade [why] [force]" tail in
// the C++ fixed order (cs_npc.cpp:1114: Optional why, then Optional
// force): a numeric why is accepted and ignored
// (CreatureAI::_EnterEvadeMode ignores its why argument), "force" arms
// the clear-then-evade leg, anything else is a parser error.
// npcEvadeReasonNames is the EnumUtils<CreatureAI::EvadeReason> table
// (enuminfo_CreatureAI.cpp): constant and title are the same five
// strings. The C++ ArgInfo enum parse (ChatCommandArgs.h:128-210)
// matches by case-insensitive exact match, else unique prefix, and a
// numeric value only when EnumUtils::IsValid holds — here exactly 0-4
// (ToIndex is a plain switch). "evade_reason_boundary" evades, 99 is a
// parser error.
var npcEvadeReasonNames = []string{
	"EVADE_REASON_NO_HOSTILES",
	"EVADE_REASON_BOUNDARY",
	"EVADE_REASON_NO_PATH",
	"EVADE_REASON_SEQUENCE_BREAK",
	"EVADE_REASON_OTHER",
}

func npcEvadeReasonParse(token string) (uint32, bool) {
	for i, name := range npcEvadeReasonNames {
		if strings.EqualFold(token, name) {
			return uint32(i), true
		}
	}
	var matched uint32
	found := false
	for i, name := range npcEvadeReasonNames {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(token)) {
			if found && matched != uint32(i) {
				return 0, false
			}
			matched, found = uint32(i), true
		}
	}
	if found {
		return matched, true
	}
	if v, err := strconv.ParseUint(token, 0, 32); err == nil && v <= 4 {
		return uint32(v), true
	}
	return 0, false
}

func npcEvadeParse(args []string) (force bool, ok bool) {
	i := 0
	if i < len(args) {
		if _, good := npcEvadeReasonParse(args[i]); good {
			i++
		}
	}
	if i < len(args) && strings.EqualFold(args[i], "force") {
		force = true
		i++
	}
	return force, i == len(args)
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
	text := s.commandTail(args)
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
	text := s.commandTail(args)
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
	text := s.commandTail(args)
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
// handleNPCWhisper mirrors HandleNpcWhisperCommand (cs_npc.cpp:981): the
// receiver is a Hyperlink<player> variant, so a |Hplayer| link resolves by
// its name; a wrong-type link fails the C++ typed parse (syntax here).
func (s *session) handleNPCWhisper(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCWhisper) {
		return
	}
	const syntax = "Syntax: .npc whisper <player> <text>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	text := s.commandTailAfter(args, 1)
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
	name := extractPlayerNameFromLink(args[0])
	if name == "" {
		s.sendSysMessage(syntax)
		return
	}
	receiver := s.server.findSessionByName(name)
	if receiver == nil || receiver.player == nil {
		s.sendSysMessage("Player not found.")
		return
	}
	// HasLowerSecurity (Chat.cpp:54-102, strong=false): a handler holding
	// RBAC_PERM_CHECK_FOR_LOWER_SECURITY (47) bypasses the check entirely
	// unless CONFIG_GM_LOWER_SECURITY is on, and a failed check sends
	// LANG_YOURS_SECURITY_IS_LOW.
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: receiver, guid: receiver.playerGUID, name: receiver.player.Name}) {
		s.sendSysMessage("Your security level is too low.")
		return
	}
	packet := protocol.BuildMonsterChatMessage(0x0F, languageUniversal, motion.GUID, motion.Name, text)
	_ = receiver.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
}

// handleNPCFollow handles "follow" and the two-level "follow stop" arm
// (cs_npc.cpp:109-110): BOTH arms are gated by
// RBAC_PERM_COMMAND_NPC_FOLLOW (578) — RBAC_PERM_COMMAND_NPC_FOLLOW_STOP
// (579) is defined in RBAC.h:447 but referenced nowhere in the C++ tree.
// A token after "follow" that matches no subcommand invokes the follow
// node with a tail its handler takes no args for, so C++ shows the follow
// help instead of following.
// script-follow ACTIVE slot exactly as the Lua MoveFollow binding does
// (chase slot cleared, same slot): PET_FOLLOW_DIST 1.0 (PetDefines.h:84)
// and Unit::GetFollowAngle pi/2 (Unit.h:1655) — the temp-summon
// m_followAngle leg has no Go slot, so summoned creatures read the same
// default. The wander section's stepScriptFollow leg then repaths the
// creature to the GM's near point; in combat the chase leg runs instead
// (the step tick's standing follow/combat precedence), and a target that
// leaves the world clears the slot. "follow stop" clears the slot only
// when it targets the GM player (the C++ generator-target predicate);
// otherwise it reports the creature is not following you, as in C++.
func (s *session) handleNPCFollow(ctx context.Context, rest []string) {
	const syntax = "Syntax: .npc follow [stop]"
	if len(rest) > 0 && strings.HasPrefix("stop", strings.ToLower(rest[0])) {
		if s.miscDeny(ctx, permissionCommandNPCFollow) {
			return
		}
		if len(rest) != 1 {
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
		name := motion.Name
		following := motion.ScriptFollowTargetGUID != 0 && motion.ScriptFollowTargetGUID == s.playerGUID
		if following {
			motion.ScriptFollowTargetGUID = 0
			motion.ScriptFollowDist = 0
			motion.ScriptFollowAngle = 0
		}
		s.server.motionMu.Unlock()
		if !following {
			s.sendSysMessage(fmt.Sprintf("%s is not following you.", name))
			return
		}
		s.sendSysMessage(fmt.Sprintf("%s is no longer following you.", name))
		return
	}
	if len(rest) > 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.miscDeny(ctx, permissionCommandNPCFollow) {
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
	motion.ScriptChaseTargetGUID = 0
	motion.ScriptChaseDist = 0
	motion.ScriptChaseAngle = 0
	motion.ScriptFollowTargetGUID = s.playerGUID
	motion.ScriptFollowDist = 1.0
	motion.ScriptFollowAngle = float32(math.Pi / 2)
	name := motion.Name
	s.server.motionMu.Unlock()
	s.sendSysMessage(fmt.Sprintf("%s is now following you.", name))
}

// npcSecsToTimeString mirrors secsToTimeString with TimeFormat::ShortText
// (Util.cpp:97-180): day/hour/minute/second segments concatenated with
// d/h/m/s suffixes, zero rendering as "0s".
func npcSecsToTimeString(secs uint64) string {
	days, hours, minutes, rem := secs/86400, secs%86400/3600, secs%3600/60, secs%60
	out := ""
	if days != 0 {
		out += fmt.Sprintf("%dd", days)
	}
	if hours != 0 {
		out += fmt.Sprintf("%dh", hours)
	}
	if minutes != 0 {
		out += fmt.Sprintf("%dm", minutes)
	}
	if rem != 0 || out == "" {
		out += fmt.Sprintf("%ds", rem)
	}
	return out
}

// npcInfoFlagBit pairs a mask bit with the C++ enumerator suffix printed
// for it by the info dump's flag-expansion loops.
type npcInfoFlagBit struct {
	bit  uint32
	name string
}

// npcUnitFlagNames mirrors UnitFlags (UnitDefines.h:122-155) in bit order.
var npcUnitFlagNames = []npcInfoFlagBit{
	{0x00000001, "SERVER_CONTROLLED"}, {0x00000002, "NON_ATTACKABLE"},
	{0x00000004, "REMOVE_CLIENT_CONTROL"}, {0x00000008, "PLAYER_CONTROLLED"},
	{0x00000010, "RENAME"}, {0x00000020, "PREPARATION"}, {0x00000040, "UNK_6"},
	{0x00000080, "NOT_ATTACKABLE_1"}, {0x00000100, "IMMUNE_TO_PC"},
	{0x00000200, "IMMUNE_TO_NPC"}, {0x00000400, "LOOTING"},
	{0x00000800, "PET_IN_COMBAT"}, {0x00001000, "PVP"}, {0x00002000, "SILENCED"},
	{0x00004000, "CANNOT_SWIM"}, {0x00008000, "SWIMMING"},
	{0x00010000, "NON_ATTACKABLE_2"}, {0x00020000, "PACIFIED"},
	{0x00040000, "STUNNED"}, {0x00080000, "IN_COMBAT"}, {0x00100000, "TAXI_FLIGHT"},
	{0x00200000, "DISARMED"}, {0x00400000, "CONFUSED"}, {0x00800000, "FLEEING"},
	{0x01000000, "POSSESSED"}, {0x02000000, "NOT_SELECTABLE"},
	{0x04000000, "SKINNABLE"}, {0x08000000, "MOUNT"}, {0x10000000, "UNK_28"},
	{0x20000000, "UNK_29"}, {0x40000000, "SHEATHE"}, {0x80000000, "IMMUNE"},
}

// npcFlagsExtraNames mirrors CreatureFlagsExtra (CreatureData.h:37-68).
var npcFlagsExtraNames = []npcInfoFlagBit{
	{0x00000001, "INSTANCE_BIND"}, {0x00000002, "CIVILIAN"}, {0x00000004, "NO_PARRY"},
	{0x00000008, "NO_PARRY_HASTEN"}, {0x00000010, "NO_BLOCK"}, {0x00000020, "NO_CRUSH"},
	{0x00000040, "NO_XP_AT_KILL"}, {0x00000080, "TRIGGER"}, {0x00000100, "NO_TAUNT"},
	{0x00000200, "NO_MOVE_FLAGS_UPDATE"}, {0x00000400, "GHOST_VISIBILITY"},
	{0x00000800, "USE_OFFHAND_ATTACK"}, {0x00001000, "NO_SELL_VENDOR"},
	{0x00002000, "NO_COMBAT"}, {0x00004000, "WORLDEVENT"}, {0x00008000, "GUARD"},
	{0x00010000, "UNUSED_16"}, {0x00020000, "NO_CRIT"}, {0x00040000, "NO_SKILLGAIN"},
	{0x00080000, "TAUNT_DIMINISH"}, {0x00100000, "ALL_DIMINISH"},
	{0x00200000, "NO_PLAYER_DAMAGE_REQ"}, {0x00400000, "UNUSED_22"},
	{0x00800000, "UNUSED_23"}, {0x01000000, "UNUSED_24"}, {0x02000000, "UNUSED_25"},
	{0x04000000, "NPCBOT"}, {0x08000000, "NPCBOT_PET"}, {0x10000000, "DUNGEON_BOSS"},
	{0x20000000, "IGNORE_PATHFINDING"}, {0x40000000, "IMMUNITY_KNOCKBACK"},
	{0x80000000, "UNUSED_31"},
}

// npcFlagNames mirrors NPCFlags (UnitDefines.h:183-212).
var npcFlagNames = []npcInfoFlagBit{
	{0x00000001, "GOSSIP"}, {0x00000002, "QUESTGIVER"}, {0x00000004, "UNK1"},
	{0x00000008, "UNK2"}, {0x00000010, "TRAINER"}, {0x00000020, "TRAINER_CLASS"},
	{0x00000040, "TRAINER_PROFESSION"}, {0x00000080, "VENDOR"},
	{0x00000100, "VENDOR_AMMO"}, {0x00000200, "VENDOR_FOOD"},
	{0x00000400, "VENDOR_POISON"}, {0x00000800, "VENDOR_REAGENT"},
	{0x00001000, "REPAIR"}, {0x00002000, "FLIGHTMASTER"}, {0x00004000, "SPIRITHEALER"},
	{0x00008000, "SPIRITGUIDE"}, {0x00010000, "INNKEEPER"}, {0x00020000, "BANKER"},
	{0x00040000, "PETITIONER"}, {0x00080000, "TABARDDESIGNER"},
	{0x00100000, "BATTLEMASTER"}, {0x00200000, "AUCTIONEER"},
	{0x00400000, "STABLEMASTER"}, {0x00800000, "GUILD_BANKER"},
	{0x01000000, "SPELLCLICK"}, {0x02000000, "PLAYER_VEHICLE"}, {0x04000000, "MAILBOX"},
}

// npcMechanicNames mirrors Mechanics (SharedDefines.h:1355-1389) by id;
// the info dump's immune loop tests bit (id-1) and prints the id.
var npcMechanicNames = []string{
	"", "CHARM", "DISORIENTED", "DISARM", "DISTRACT", "FEAR", "GRIP", "ROOT",
	"SLOW_ATTACK", "SILENCE", "SLEEP", "SNARE", "STUN", "FREEZE", "KNOCKOUT",
	"BLEED", "BANDAGE", "POLYMORPH", "BANISH", "SHIELD", "SHACKLE", "MOUNT",
	"INFECTED", "TURN", "HORROR", "INVULNERABILITY", "INTERRUPT", "DAZE",
	"DISCOVERY", "IMMUNE_SHIELD", "SAPPED", "ENRAGED",
}

// npcInfoFlagLines renders the "* NAME (0xX)" expansion lines for the
// set bits of mask, in table order.
func (s *session) npcInfoFlagLines(mask uint32, table []npcInfoFlagBit) {
	for _, flag := range table {
		if mask&flag.bit != 0 {
			s.sendSysMessage(fmt.Sprintf("* %s (0x%X)", flag.name, flag.bit))
		}
	}
}

// handleNPCInfo mirrors HandleNpcInfoCommand (cs_npc.cpp:482) in C++ line
// order: char, spawn group (non-zero groups only), compatibility mode, level, equipment, health, movement
// data, unit flags (+ expansions), flags, respawn times, loot, dungeon
// id, phasemask, armor, position, AI info, react state, flags extra (+
// expansions), npc flags (+ expansions), mechanic immune (+ expansions).
// LANG wordings are plain-English recalls (no in-tree seed). DELTAS: the
// AI-type line has none either (no C++ AI class model — ScriptName is the Go AI
// identity); equipment ids have no Go slot and read 0; the movement
// Chase/Random legs read the C++ default-ctor values (Run/Walk — Go loads
// only Ground/Swim/Flight); the template Rooted bit has no Go slot, so
// only the script-rooted latch appends ", Rooted"; compatibility mode
// reads false (no Go compatibility-mode model); the current respawn
// delay reads 0 (no remaining-time model for dead creatures — selected
// motions are live); the spawn id is the GUID counter, exact for DB
// creatures; a missing template/spawn row leaves its fields at 0.
func (s *session) handleNPCInfo(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCInfo) {
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
	snap := *motion
	s.server.motionMu.Unlock()

	guidCounter := uint32(snap.GUID & 0x00FFFFFF)
	var lootID, pickpocketLootID, skinLootID, mechanicImmuneMask, spawnTimeSecs uint32
	if db := s.npcWorldDB(); db != nil {
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(lootid, 0), COALESCE(pickpocketloot, 0), COALESCE(skinloot, 0), COALESCE(MechanicImmuneMask, 0) FROM creature_template WHERE entry = ?", snap.Entry).Scan(&lootID, &pickpocketLootID, &skinLootID, &mechanicImmuneMask)
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(spawntimesecs, 0) FROM creature WHERE guid = ?", guidCounter).Scan(&spawnTimeSecs)
	}
	defRespawn := spawnTimeSecs
	if snap.RespawnDelayKnown {
		defRespawn = snap.RespawnDelaySec
	}
	createHealth := snap.BaseMaxHealth
	if createHealth == 0 {
		createHealth = snap.MaxHealth
	}
	phaseMask := snap.PhaseMask
	if phaseMask == 0 {
		phaseMask = 1
	}
	ground := "Run"
	swim := true
	flight := "None"
	if snap.MoveKnown {
		switch snap.MoveGround {
		case 0:
			ground = "None"
		case 2:
			ground = "Hover"
		}
		swim = snap.MoveSwim
		if snap.CanFly {
			flight = "CanFly"
		} else if snap.GravityDisabled {
			flight = "DisableGravity"
		}
	}
	movement := fmt.Sprintf("Ground: %s, Swim: %t, Flight: %s, Chase: Run, Random: Walk", ground, swim, flight)
	if snap.ScriptRooted {
		movement += ", Rooted"
	}
	reactState := snap.ReactState
	if !snap.ReactStateKnown {
		reactState = creatureReactState(snap.CreatureType, snap.NPCFlags, snap.FlagsExtra, snap.AIName)
	}
	reactName := "<Invalid react state>"
	switch reactState {
	case 0:
		reactName = "PASSIVE"
	case 1:
		reactName = "DEFENSIVE"
	case 2:
		reactName = "AGGRESSIVE"
	}

	s.sendSysMessage(fmt.Sprintf("NPC %s: spawn id %d, GUID %d, entry %d, faction %d, npc flags 0x%X, display id %d, native display id %d", snap.Name, guidCounter, guidCounter, snap.Entry, snap.Faction, snap.NPCFlags, snap.DisplayID, snap.NativeDisplayID))
	if db := s.npcWorldDB(); db != nil {
		if groupID, tmpl, active := s.server.spawnGroupOfMember(ctx, db, spawnTypeCreature, guidCounter, snap.Map, snap.InstanceID); groupID != 0 && tmpl != nil {
			s.sendSysMessage(fmt.Sprintf("Spawn group: %s (id %d, flags 0x%X, active %t)", tmpl.Name, groupID, tmpl.Flags, active))
		}
	}
	s.sendSysMessage("Respawn compatibility mode: false")
	s.sendSysMessage(fmt.Sprintf("Level: %d", snap.Level))
	s.sendSysMessage("Equipment: current 0, original 0")
	s.sendSysMessage(fmt.Sprintf("Health: create %d, max %d, current %d", createHealth, snap.MaxHealth, snap.Health))
	s.sendSysMessage(fmt.Sprintf("Movement: %s", movement))
	s.sendSysMessage(fmt.Sprintf("Unit flags: 0x%X", snap.UnitFlags))
	s.npcInfoFlagLines(snap.UnitFlags, npcUnitFlagNames)
	s.sendSysMessage(fmt.Sprintf("Flags: flags2 0x%X, dynamic flags 0x%X, faction %d", snap.UnitFlags2, snap.DynamicFlags, snap.Faction))
	s.sendSysMessage(fmt.Sprintf("Respawn: default %s, current %s", npcSecsToTimeString(uint64(defRespawn)), npcSecsToTimeString(0)))
	s.sendSysMessage(fmt.Sprintf("Loot: %d, pickpocket loot: %d, skin loot: %d", lootID, pickpocketLootID, skinLootID))
	s.sendSysMessage(fmt.Sprintf("Instance id: %d", snap.InstanceID))
	s.sendSysMessage(fmt.Sprintf("Phase mask: %d", phaseMask))
	s.sendSysMessage(fmt.Sprintf("Armor: %d", snap.Armor))
	s.sendSysMessage(fmt.Sprintf("Position: %.2f, %.2f, %.2f", snap.X, snap.Y, snap.Z))
	s.sendSysMessage(fmt.Sprintf("AI: %s, script: %s", snap.AIName, snap.ScriptName))
	s.sendSysMessage(fmt.Sprintf("React state: %s", reactName))
	s.sendSysMessage(fmt.Sprintf("Flags extra: 0x%X", snap.FlagsExtra))
	s.npcInfoFlagLines(snap.FlagsExtra, npcFlagsExtraNames)
	s.npcInfoFlagLines(snap.NPCFlags, npcFlagNames)
	s.sendSysMessage(fmt.Sprintf("Mechanic immune mask: 0x%X", mechanicImmuneMask))
	for id := uint32(1); id < uint32(len(npcMechanicNames)); id++ {
		if mechanicImmuneMask&(1<<(id-1)) != 0 {
			s.sendSysMessage(fmt.Sprintf("* %s (0x%X)", npcMechanicNames[id], id))
		}
	}
}

// handleNPCSpawnGroup mirrors HandleNpcSpawnGroup (cs_npc.cpp:1356) on the
// spawn-group manager: group id plus the force/ignorerespawn flags, the
// bad-group message on an unknown, system, cross-map or memberless group,
// and the spawn count on success. LANG wordings are plain-English recalls
// (no in-tree seed); the despawn count line is literal C++ text.
func (s *session) handleNPCSpawnGroup(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCSpawnGroup) {
		return
	}
	groupID, force, ignoreRespawn, ok := parseSpawnGroupArgs(args, true)
	if !ok {
		s.sendSysMessage("Syntax: .npc spawngroup <groupId> [force] [ignorerespawn]")
		return
	}
	if s.server == nil {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	count, valid := s.server.spawnGroupSpawn(ctx, db, s.player.Map, s.player.InstanceID, groupID, ignoreRespawn, force)
	if !valid {
		s.sendSysMessage(fmt.Sprintf("Spawn group %d is not valid.", groupID))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Spawned %d objects.", count))
}

// handleNPCDespawnGroup mirrors HandleNpcDespawnGroup (cs_npc.cpp:1397) on
// the spawn-group manager: group id plus the removerespawntime flag, the
// bad-group message on an unknown, system, cross-map or memberless group,
// and the despawn count on success.
func (s *session) handleNPCDespawnGroup(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCDespawnGroup) {
		return
	}
	groupID, removeRespawnTime, _, ok := parseSpawnGroupArgs(args, false)
	if !ok {
		s.sendSysMessage("Syntax: .npc despawngroup <groupId> [removerespawntime]")
		return
	}
	if s.server == nil {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	count, valid := s.server.spawnGroupDespawn(ctx, db, s.player.Map, s.player.InstanceID, groupID, removeRespawnTime)
	if !valid {
		s.sendSysMessage(fmt.Sprintf("Spawn group %d is not valid.", groupID))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Despawned a total of %d objects.", count))
}

// handleNPCTame mirrors HandleNpcTameCommand (cs_npc.cpp:1048): the selected
// non-pet creature becomes the player's pet. Unlike the spell tame
// (handleTameCreature), the command has no hunter-class gate — only the
// selected-creature, no-current-pet and IsTameable gates — and unlike the
// spell it does NOT despawn the original creature (the command has no
// DespawnOrUnsummon leg; SpellEffects.cpp:2991 does). Creation rides
// createTamedPetFromEntry (character_pet INSERT as current, pet_spell rows,
// family passives, spawnPet's AddToMap + SetMinion + PetSpellInitialize
// twins) with spell 0 and REACT_DEFENSIVE, the command's SetReactState leg;
// the InitTamedPet stable refusal surfaces as the non-tameable message.
// DELTAS: spawnPet places the pet at the follow offset (combatReach plus
// follow distance at orientation + pi/2), not the command's
// GetClosePoint(combatReach, CONTACT_DISTANCE 0.5) plus orientation pi -
// player orientation; the level-1-then-level visual effect is not
// reproduced (spawnPet broadcasts the final level only); InitTalentForLevel
// rides the pet_spell rows, not a separate talent init.
func (s *session) handleNPCTame(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCTame) {
		return
	}
	if s.player == nil || s.server == nil {
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil || motionIsPet(motion) {
		s.sendSysMessage("Select a creature first.")
		return
	}
	s.server.motionMu.Lock()
	entry, creatureLevel := motion.Entry, motion.Level
	s.server.motionMu.Unlock()
	if s.player.PetGUID != 0 {
		s.sendSysMessage("You already have a pet.")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	var ctype, family, typeFlags int64
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(type, 0), COALESCE(family, 0), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?", entry).Scan(&ctype, &family, &typeFlags); err != nil || !creatureTameable(ctype, family, typeFlags, s.canTameExoticPets()) {
		s.sendSysMessage(fmt.Sprintf("Creature %d is not tameable.", entry))
		return
	}
	if s.hasStabledHunterPet(ctx) {
		s.sendSysMessage(fmt.Sprintf("Creature %d is not tameable.", entry))
		return
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	// std::max<uint8>(playerLevel-5, creatureLevel) (cs_npc.cpp:1095):
	// the uint8 subtraction wraps below level 5, so a level-1..4 player
	// tames at level 252..254, not at the creature's level.
	playerLevel := uint32(s.player.Level)
	petLevel := creatureLevel
	if wrapped := uint32(uint8(playerLevel - 5)); wrapped > petLevel {
		petLevel = wrapped
	}
	// CreateTamedPetFrom returning null reports LANG_CREATURE_NON_TAMEABLE
	// (cs_npc.cpp:1088-1093); the Go creation fails on the character_pet
	// INSERT.
	if !s.createTamedPetFromEntry(ctx, s.server.CharactersStore.DB, 0, entry, petLevel) {
		s.sendSysMessage(fmt.Sprintf("Creature %d is not tameable.", entry))
		return
	}
}

// npcShowLootEntry mirrors _ShowLootEntry (cs_npc.cpp:1138): the count,
// quality-colored item link and name for one loot row; an unknown template
// renders "Unknown item" in the POOR (grey) color, as in C++.
func (s *session) npcShowLootEntry(ctx context.Context, itemID, count, quality uint32) {
	db := s.npcWorldDB()
	name := "Unknown item"
	if db != nil {
		var itemName string
		if err := db.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemID).Scan(&itemName); err == nil && itemName != "" {
			name = itemName
		}
	}
	color := itemQualityColors[0]
	if int(quality) < len(itemQualityColors) {
		color = itemQualityColors[quality]
	}
	s.sendSysMessage(fmt.Sprintf("%d x |c%08X|Hitem:%d|h[%s]|h|r", count, color, itemID, name))
}

// handleNPCShowLoot mirrors HandleNpcShowLootCommand (cs_npc.cpp:1170): the
// selected non-pet creature must be dead with non-empty loot from the
// creatureLoot store (standard-GUID fallback, as in the loot sparkle leg).
// The header, money line and standard/quest sections follow the C++ order;
// the "all" form drops free-for-all rows from the standard section and
// lists them in their own section instead of the per-player split. DELTAS:
// taken rows are already deleted from the Go Items map, so the section
// counts are unlooted counts, not C++'s items.size(); the per-player
// quest/FFA/conditional maps have no Go twin, so the "all" quest section
// lists the shared quest rows and the conditions filter is unmodeled; the
// locale leg is enUS-only (item_template Name1, as in the npc vendor arms).
func (s *session) handleNPCShowLoot(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCShowLoot) {
		return
	}
	const syntax = "Syntax: .npc showloot [all]"
	if len(args) > 1 {
		s.sendSysMessage(syntax)
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
	snap := *motion
	s.server.motionMu.Unlock()
	if snap.Health != 0 {
		s.sendSysMessage(fmt.Sprintf("%s is not dead or has no loot.", snap.Name))
		return
	}
	s.server.lootMu.Lock()
	loot := s.server.creatureLoot[lootObjectKey{MapID: snap.Map, InstanceID: snap.InstanceID, GUID: snap.GUID}]
	if loot == nil {
		if std := creatureWorldGUID(uint32(snap.GUID&0xFFFFFF), snap.Entry); std != snap.GUID {
			loot = s.server.creatureLoot[lootObjectKey{MapID: snap.Map, InstanceID: snap.InstanceID, GUID: std}]
		}
	}
	var money uint32
	var items, questItems []lootItem
	if loot != nil {
		money = loot.Money
		items = sortedLootItems(loot.Items)
		questItems = sortedLootItems(loot.QuestItems)
	}
	s.server.lootMu.Unlock()
	// Loot::empty (Loot.h:235) is items.empty() && gold == 0 — quest_items
	// are excluded, so loot that is only quest items reports as no loot.
	if loot == nil || (money == 0 && len(items) == 0) {
		s.sendSysMessage(fmt.Sprintf("%s is not dead or has no loot.", snap.Name))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Loot for %s (Entry: %d):", snap.Name, snap.Entry))
	s.sendSysMessage(fmt.Sprintf("Money: %dg %ds %dc", money/10000, money%10000/100, money%100))
	all := len(args) > 0 && strings.EqualFold(args[0], "all")
	standard := items
	ffa := make([]lootItem, 0)
	if all {
		standard = make([]lootItem, 0, len(items))
		for _, item := range items {
			if item.FreeForAll {
				ffa = append(ffa, item)
			} else {
				standard = append(standard, item)
			}
		}
	}
	s.sendSysMessage(fmt.Sprintf("Standard items (%d):", len(standard)))
	for _, item := range standard {
		s.npcShowLootEntry(ctx, item.ItemEntry, item.Count, item.Quality)
	}
	s.sendSysMessage(fmt.Sprintf("Quest items (%d):", len(questItems)))
	for _, item := range questItems {
		s.npcShowLootEntry(ctx, item.ItemEntry, item.Count, item.Quality)
	}
	if all && len(ffa) > 0 {
		s.sendSysMessage(fmt.Sprintf("Free-for-all items (%d):", len(ffa)))
		for _, item := range ffa {
			s.npcShowLootEntry(ctx, item.ItemEntry, item.Count, item.Quality)
		}
	}
}
