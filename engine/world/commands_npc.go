package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// npc command port: npc_commandscript (cs_npc.cpp), the "npc" root with the
// "add" and "set" sub-tables plus 16 flat arms. TWENTY-NINTH of 39 Commands
// groups (cs_script_loader.cpp decl 47 / call 92; call order re-verified this
// run: modify(91) -> npc(92)). Trinity checks permission only on the invoker
// leaf node (ChatCommand.cpp:487), so each arm gates exactly its own C++
// permission (RBAC.h:439-468/704/723-724/732).
//
// This file is chunk 1: the "add" sub-table (formation, item, move, temp,
// "") plus the flat arms move, delete and near. Chunk 2 covers the "set"
// sub-table and the remaining flat arms (info, playemote, say, textemote,
// whisper, yell, tame, spawngroup, despawngroup, delete item, follow,
// follow stop, evade, showloot).
//
// Creature visibility in the Go world is DB-driven (buildNearbyCreatureUpdates
// reads the world `creature` table live), so the DB legs of these arms are
// the real spawn/move/delete operations; the live-Creature legs ride the
// motion map where a twin exists (move/delete respawn legs) and are noted
// per arm.
//
// Console-vs-chat branches are moot (Go commands are always sessioned).
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// npcWorldDB returns the world database handle or nil.
func (s *session) npcWorldDB() *sql.DB {
	if s.server == nil || s.server.WorldStore == nil {
		return nil
	}
	return s.server.WorldStore.DB
}

// npcTemplateGate mirrors the sObjectMgr->GetCreatureTemplate gate shared by
// the npc arms: the entry must exist in creature_template. It also reports
// the template's flags_extra for the npcbot gate.
func (s *session) npcTemplateGate(ctx context.Context, entry uint32) (flagsExtra uint32, ok bool) {
	db := s.npcWorldDB()
	if db == nil {
		return 0, false
	}
	var flags sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(flags_extra, 0) FROM creature_template WHERE entry = ?", entry).Scan(&flags); err != nil {
		return 0, false
	}
	if flags.Valid {
		flagsExtra = uint32(flags.Int64)
	}
	return flagsExtra, true
}

// npcbotSpawnBlocked mirrors the NPCBots gate in HandleNpcAddCommand
// (cs_npc.cpp:121-132): npcbot entries must go through '.npcbot spawn'
// instead.
func (s *session) npcbotSpawnBlocked(entry uint32, flagsExtra uint32) bool {
	if flagsExtra&npcbotEntryMask != 0 {
		s.sendSysMessage(fmt.Sprintf("You tried to spawn creature %d, which is part of NPCBots mod. To spawn bots use '.npcbot spawn' instead.", entry))
		return true
	}
	return false
}

// npcbotMoveBlocked mirrors the NPCBots gate in HandleNpcMoveCommand
// (cs_npc.cpp:625-631): npcbot entries must go through '.npcbot move'
// instead. The message differs from the add-arm wording.
func (s *session) npcbotMoveBlocked(guid, entry uint32, flagsExtra uint32) bool {
	if flagsExtra&npcbotEntryMask != 0 {
		s.sendSysMessage(fmt.Sprintf("creature %d (id %d) is a part of NPCBots mod. Use '.npcbot move' instead", guid, entry))
		return true
	}
	return false
}

// npcbotEntryMask is CREATURE_FLAG_EXTRA_NPCBOT|_NPCBOT_PET
// (CreatureData.h:63-64).
const npcbotEntryMask = 0x04000000 | 0x08000000

// handleCmdNPC dispatches the "npc" root (cs_npc.cpp:89-116). Chunk 1 covers
// the "add" sub-table and the flat arms move/delete/near; the rest lands in
// chunk 2. R13: the bool reports C++ handler success — denial and parse
// failures return false silently (dispatch shows the node's help listing),
// message legs return true — and propagates every arm's bool through the
// add/set/chunk dispatchers.
func (s *session) handleCmdNPC(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .npc add|set|move|delete|near|info|playemote|say|textemote|whisper|yell|tame|spawngroup|despawngroup|follow|evade|showloot"
	if len(args) == 0 {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("add", sub):
		return s.handleNPCAddTable(ctx, rest)
	case strings.HasPrefix("move", sub):
		return s.handleNPCMove(ctx, rest)
	case strings.HasPrefix("delete", sub):
		return s.npcDeleteSub(ctx, rest)
	case strings.HasPrefix("near", sub):
		return s.handleNPCNear(ctx, rest)
	default:
		return s.handleNPCChunk2(ctx, sub, rest)
	}
}

// handleNPCAddTable dispatches the "add" sub-table (cs_npc.cpp:61-71).
func (s *session) handleNPCAddTable(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .npc add [formation|item|move|temp] <args> | .npc add <entry>"
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("formation", sub):
		return s.handleNPCAddFormation(ctx, rest)
	case strings.HasPrefix("item", sub):
		return s.handleNPCAddItem(ctx, rest)
	case strings.HasPrefix("move", sub):
		return s.handleNPCAddMove(ctx, rest)
	case strings.HasPrefix("temp", sub):
		return s.handleNPCAddTemp(ctx, rest)
	default:
		// The "" table entry: ".npc add <entry>".
		return s.handleNPCAdd(ctx, args)
	}
}

// handleNPCAddFormation mirrors HandleNpcAddFormationCommand (cs_npc.cpp):
// the selected creature becomes a formation member under leaderGUID. Two
// C++ quirks are replicated for 1:1 behavior: the binds put followAngle
// into the `dist` column and followDist into `angle`
// (WorldDatabase.cpp WORLD_INS_CREATURE_FORMATION column order vs the
// setFloat(2, followAngle)/setFloat(3, followDist) binds), and the angle is
// stored in degrees while the consumer reads radians. groupAI is stored 0
// exactly like C++, so the row stays inert until groupAI gains
// FLAG_IDLE_IN_FORMATION (0x200) — LeaderStartedMoving skips members
// without it, in both trees.
func (s *session) handleNPCAddFormation(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCAddFormation) {
		return false
	}
	if len(args) != 1 {
		return false
	}
	leaderGUID, err := strconv.ParseUint(args[0], 0, 32)
	if err != nil {
		return false
	}
	if uint16(s.selection>>48) != 0xF130 {
		s.sendSysMessage("Select a creature first.")
		return true
	}
	memberLow := uint32(s.selection & 0x00FFFFFF)
	if memberLow == 0 {
		s.sendSysMessage("Select a creature first.")
		return true
	}
	server := s.server
	// C++ checks the selected creature, then GetFormation() on the live
	// creature (cs_npc triage L9): the live-motion check precedes the
	// loaded-row check, so a creature with no live motion is not reported
	// as already grouped from a stale row.
	member := s.findCreatureMotion(s.selection)
	if member == nil {
		s.sendSysMessage("Select a creature first.")
		return true
	}
	if row, ok := server.formationRowFor(memberLow); ok {
		s.sendSysMessage(fmt.Sprintf("Selected creature is already member of group %d", row.leaderGUID))
		return false
	}
	followAngle := (math.Atan2(float64(s.player.Y-member.Y), float64(s.player.X-member.X)) - float64(s.player.Orientation)) * 180.0 / math.Pi
	followDist := math.Hypot(float64(s.player.X-member.X), float64(s.player.Y-member.Y))
	if _, err := server.WorldStore.DB.ExecContext(ctx, "INSERT INTO creature_formations (leaderGUID, memberGUID, dist, angle, groupAI) VALUES (?, ?, ?, ?, ?)", uint32(leaderGUID), memberLow, float32(followAngle), float32(followDist), 0); err != nil {
		s.sendSysMessage("Failed to store the formation row.")
		return true
	}
	server.ensureFormationsLoaded(ctx)
	server.addFormationRow(memberLow, formationRow{leaderGUID: uint32(leaderGUID), dist: float32(followAngle), angle: float32(followDist)})
	s.sendSysMessage(fmt.Sprintf("Creature %d added to formation with leader %d", memberLow, uint32(leaderGUID)))
	return true
}

// npcItemTokenID parses a C++ ItemTemplate command argument
// (ChatCommandArgs.cpp:69: numeric id or |Hitem| link). It reports whether
// the token was a link: a link resolving to no template reaches the handler
// body as a nil item, while a numeric id with no template is a parser error,
// so the caller needs the distinction.
func npcItemTokenID(token string) (uint32, bool, bool) {
	if id := miscExtractLinkID(token, "Hitem"); id != 0 {
		return id, true, true
	}
	id64, err := strconv.ParseUint(token, 0, 32)
	if err != nil || id64 == 0 {
		return 0, false, false
	}
	return uint32(id64), false, true
}

// npcVendorUintArg parses one of the optional uint32 vendor arguments
// (maxcount, incrtime, extendedcost): C++ parses Optional<uint32>, so a
// non-numeric token is a parser error, not atoi's 0. StringTo<T>(token, 0)
// is base 0 (ChatCommandArgs.h:54-72): 0x10 is 16, 0b101 is 5.
func npcVendorUintArg(token string) (uint32, bool) {
	v, err := strconv.ParseUint(token, 0, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// handleNPCAddItem mirrors HandleNpcAddVendorItemCommand (cs_npc.cpp:179):
// ".npc add item <itemId|itemLink> [maxcount] [incrtime] [extendedcost]".
// The vendor is the selected creature (castCommandSelectedCreature, the
// getSelectedCreature twin); a missing selection reports
// LANG_SELECT_CREATURE. The gates run in IsVendorItemValid order
// (ObjectMgr.cpp:9617): template, vendor flag, item template, ExtendedCost
// DBC row, maxcount/incrtime coherence, duplicate (item, ExtendedCost)
// pair — then the WORLD_INS_NPC_VENDOR insert (entry, item, maxcount,
// incrtime, extendedcost, no slot, the luaAddVendorItem statement). Go
// loads vendor rows from the DB on every list/buy call, so the row is
// immediately visible with no cache to refresh. Success reports
// LANG_ITEM_ADDED_TO_LIST (id, name, maxcount, incrtime, extendedcost).
func (s *session) handleNPCAddItem(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCAddItem) {
		return false
	}
	const syntax = "Syntax: .npc add item <itemId|itemLink> [maxcount] [incrtime] [extendedcost]"
	if len(args) == 0 || len(args) > 4 {
		return false
	}
	itemID, link, ok := npcItemTokenID(args[0])
	if !ok {
		return false
	}
	var maxcount, incrtime, extendedcost uint32
	for i, token := range args[1:] {
		v, ok := npcVendorUintArg(token)
		if !ok {
			return false
		}
		switch i {
		case 0:
			maxcount = v
		case 1:
			incrtime = v
		case 2:
			extendedcost = v
		}
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	// C++ checks the nil-item leg before the vendor selection
	// (cs_npc.cpp:179-192): a link resolving to no template reaches the
	// body as a nil item, so its LANG_COMMAND_NEEDITEMSEND reports
	// before the vendor gate. A numeric id with no template is a parser
	// error in C++ and stays pre-vendor here.
	var itemName string
	itemMissing := false
	if err := db.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemID).Scan(&itemName); err != nil {
		if !link {
			return false
		}
		itemMissing = true
	}
	if itemMissing {
		s.sendSysMessage("You need to send an item.")
		return true
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return true
	}
	s.server.motionMu.Lock()
	vendorEntry := motion.Entry
	s.server.motionMu.Unlock()
	var npcFlag uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(npcflag, 0) FROM creature_template WHERE entry = ?", vendorEntry).Scan(&npcFlag); err != nil || npcFlag&unitNPCFlagVendor == 0 {
		s.sendSysMessage("You need to select a vendor.")
		return true
	}
	if extendedcost != 0 {
		if s.server == nil || s.server.Data == nil {
			s.sendSysMessage(fmt.Sprintf("Extended cost %d does not exist.", extendedcost))
			return true
		}
		if _, found, err := s.server.Data.ItemExtendedCost(extendedcost); err != nil || !found {
			s.sendSysMessage(fmt.Sprintf("Extended cost %d does not exist.", extendedcost))
			return true
		}
	}
	if maxcount > 0 && incrtime == 0 {
		s.sendSysMessage(fmt.Sprintf("MaxCount != 0 (%d) but IncrTime == 0", maxcount))
		return true
	}
	if maxcount == 0 && incrtime > 0 {
		s.sendSysMessage("MaxCount == 0 but IncrTime<>= 0")
		return true
	}
	var exists int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM npc_vendor WHERE entry = ? AND item = ? AND ExtendedCost = ?", vendorEntry, itemID, extendedcost).Scan(&exists); err != nil || exists != 0 {
		s.sendSysMessage(fmt.Sprintf("Item %d is already in the list (extended cost %d).", itemID, extendedcost))
		return true
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO npc_vendor (entry, item, maxcount, incrtime, extendedcost) VALUES (?, ?, ?, ?, ?)", vendorEntry, itemID, maxcount, incrtime, extendedcost); err != nil {
		s.sendSysMessage("Failed to store the vendor row.")
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Item %d (%s) added to vendor list (maxcount %d, incrtime %d, extendedcost %d).", itemID, itemName, maxcount, incrtime, extendedcost))
	return true
}

// handleNPCAddTemp mirrors HandleNpcAddTempSpawnCommand (cs_npc.cpp:1025):
// ".npc add temp [loot|noloot] <entry>" summons the entry at the invoker's
// position through the summonTempCreature funnel (the player SummonCreature
// bridge): loot selects TEMPSUMMON_CORPSE_TIMED_DESPAWN (5), noloot and
// the bare form select TEMPSUMMON_CORPSE_DESPAWN (4), both with C++'s 30s
// duration (carried for the corpse timer on type 5, no live timer on
// type 4). An unknown template fails silently, the handleNPCAdd
// precedent; success is silent as in C++. No DB row is written, so the
// summon is temporary by construction, and no npcbot gate runs: C++'s
// temp arm carries none (unlike the permanent add arm). The bool reports
// C++ handler success: a non-loot/noloot first token and the unknown
// template both return false silently (cs_npc.cpp:1025-1038), so
// dispatchCommand shows the node's help listing (ChatCommand.cpp:296-309);
// success returns true.
func (s *session) handleNPCAddTemp(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCAddTemp) {
		return false
	}
	const syntax = "Syntax: .npc add temp [loot|noloot] <entry>"
	summonType := uint8(4)
	rest := args
	if len(rest) > 0 {
		switch strings.ToLower(rest[0]) {
		case "loot":
			summonType = 5
			rest = rest[1:]
		case "noloot":
			rest = rest[1:]
		default:
			// A first token that is neither loot nor noloot makes
			// the C++ handler return false silently (the parser
			// backtracks the optional lootStr onto the entry, so a
			// lone unknown token stays a parse failure here).
			if len(rest) > 1 {
				return false
			}
		}
	}
	if len(rest) != 1 {
		return false
	}
	entry, ok := goParseLinkOrUint(rest[0], "creature_entry")
	if !ok {
		return false
	}
	if _, ok := s.npcTemplateGate(ctx, entry); !ok {
		return false
	}
	if s.server == nil || s.player == nil {
		return true
	}
	p := s.player
	origin := tempSummonOrigin{Map: p.Map, InstanceID: p.InstanceID, GUID: p.GUID, X: p.X, Y: p.Y, Z: p.Z, O: p.Orientation, Reach: float64(p.CombatReach)}
	s.server.summonTempCreature(ctx, origin, entry, p.X, p.Y, p.Z, p.Orientation, summonType, 30000)
	return true
}

// handleNPCAddMove mirrors HandleNpcAddMoveCommand (cs_npc.cpp:213): the
// creature row must exist; movement_type becomes WAYPOINT (2). Pure DB leg,
// fully native.
func (s *session) handleNPCAddMove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCAddMove) {
		return false
	}
	if len(args) != 1 {
		return false
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	guid, ok := goParseLinkOrUint(args[0], "creature")
	if !ok {
		return false
	}
	var exists uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE guid = ?", guid).Scan(&exists); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return true
	}
	if _, err := db.ExecContext(ctx, "UPDATE creature SET MovementType = 2 WHERE guid = ?", guid); err != nil {
		return true
	}
	s.sendSysMessage("Waypoint added.") // LANG_WAYPOINT_ADDED 234
	return true
}

// handleNPCAdd mirrors HandleNpcAddCommand (cs_npc.cpp:117): the entry must
// exist in creature_template and must not be an npcbot entry. The live spawn
// legs (Creature::Create/AddToMap/grid) have no Go bridge, but creature
// visibility is DB-driven, so the creature row insert IS the spawn.
func (s *session) handleNPCAdd(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCAdd) {
		return false
	}
	if len(args) != 1 {
		return false
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	entry, ok := goParseLinkOrUint(args[0], "creature_entry")
	if !ok {
		return false
	}
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok {
		return false // C++: silent false on unknown template
	}
	if s.npcbotSpawnBlocked(entry, flagsExtra) {
		return true
	}
	var guid uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid); err != nil {
		return true
	}
	p := s.player
	// SaveToDB (cs_npc.cpp:163): spawnMask is 1 << Map::GetSpawnMode and
	// phaseMask is GetPhaseMaskForSpawn (Player.cpp:25279) — a GM reads
	// the first SPELL_AURA_PHASE aura's misc value, else PHASEMASK_NORMAL;
	// the result strips the normal bit when another phase bit is set. The
	// transport leg (CreateNPCPassenger + transport offsets) has no Go
	// bridge and stays open.
	spawnMode := s.playerSpawnMode()
	phaseMask := s.playerPhaseMaskForSpawn()
	// spawntimesecs default mirrors the world DB column default for fresh
	// rows; the live-spawn respawn-delay legs have no Go bridge.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 120, 0)",
		guid, entry, p.Map, 1<<spawnMode, phaseMask, p.X, p.Y, p.Z, p.Orientation); err != nil {
		return true
	}
	// C++ reports success silently.
	return true
}

// handleNPCMove mirrors HandleNpcMoveCommand (cs_npc.cpp:601): the GUID
// form moves the spawn row to the handler's position, and the selection
// form (no GUID arg) moves the selected creature's row instead. The live
// legs ride the DespawnOrUnsummon(0s, 1s) twin (despawnAtEvade): the moved
// creature disappears and respawns at the new row after 1s at full health,
// and a differently-selected creature is despawned too and respawns at its
// own home (no path reload: a respawn does not LoadPath). A selected pet,
// temp summon or totem has no spawn row, so it is left live — the
// despawn twin re-reads the spawn row and would delete it instead.
// Pets, totems and temp summons carry no spawn id, so C++ reads
// GetCreatureData(0) and reports GUID-not-found for them. The bool reports C++ handler success: with
// neither a GUID nor a selection C++ returns false silently
// (cs_npc.cpp:607-608), so dispatchCommand shows the node's help listing
// (ChatCommand.cpp:296-309); the GUID-not-found, wrong-map and npcbot
// paths print a message and return true.
func (s *session) handleNPCMove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCMove) {
		return false
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	const syntax = "Syntax: .npc move [guid]"
	if len(args) > 1 {
		return false
	}
	var guid uint32
	selected := s.castCommandSelectedCreature()
	if len(args) == 0 {
		if selected == nil {
			return false
		}
		if motionIsPet(selected) || selected.IsTempSummon || (s.server != nil && s.server.isTotemGUID(selected.GUID)) {
			guid = 0
		} else {
			guid = uint32(selected.GUID & 0x00FFFFFF)
		}
	} else {
		var ok bool
		guid, ok = goParseLinkOrUint(args[0], "creature")
		if !ok {
			return false
		}
	}
	var rowMap uint32
	var entry uint32
	if err := db.QueryRowContext(ctx, "SELECT map, id FROM creature WHERE guid = ?", guid).Scan(&rowMap, &entry); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return true
	}
	if rowMap != s.player.Map {
		s.sendSysMessage(fmt.Sprintf("Creature #%d is on a different map.", guid)) // LANG_COMMAND_CREATUREATSAMEMAP 272
		return true
	}
	if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok {
		if s.npcbotMoveBlocked(guid, entry, flagsExtra) {
			return true
		}
	}
	p := s.player
	if _, err := db.ExecContext(ctx, "UPDATE creature SET position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
		p.X, p.Y, p.Z, p.Orientation, guid); err != nil {
		return true
	}
	if s.server != nil {
		// DespawnOrUnsummon(0s, 1s) (cs_npc.cpp:650-651): the moved
		// creature disappears for 1s and the respawn leg re-reads the
		// updated spawn row, so it re-materializes at the new position
		// at full health; a differently-selected creature is despawned
		// too and respawns at its own unchanged home. The despawnAtEvade
		// machinery is the DespawnOrUnsummon(0s, delay) twin.
		if moved := s.npcLiveCreatureBySpawnGUID(guid); moved != nil && !s.server.isTotemGUID(moved.GUID) {
			s.server.despawnAtEvade(ctx, moved, time.Second)
		}
		if selected != nil && uint32(selected.GUID&0x00FFFFFF) != guid {
			if selSpawn := s.npcLiveCreatureBySpawnGUID(uint32(selected.GUID & 0x00FFFFFF)); selSpawn != nil && !s.server.isTotemGUID(selSpawn.GUID) {
				s.server.despawnAtEvade(ctx, selSpawn, time.Second)
			}
		}
	}
	s.sendSysMessage("Creature moved.") // LANG_COMMAND_CREATUREMOVED 271
	return true
}

// handleNPCDelete mirrors HandleNpcDeleteCommand (cs_npc.cpp:302): the
// GUID form deletes the spawn row (rows>0 -> deleted, else
// GUID-not-found), and the selection form (no GUID arg) resolves the
// selected creature — pet/totem -> LANG_SELECT_CREATURE, npcbot -> the
// bot message, temp summon -> UnSummon (live removal + deleted message),
// else the creature's spawn id down the DeleteFromDB path.
func (s *session) handleNPCDelete(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCDelete) {
		return false
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	const syntax = "Syntax: .npc delete [guid]"
	if len(args) > 1 {
		return false
	}
	var guid uint32
	if len(args) == 0 {
		selected := s.castCommandSelectedCreature()
		if selected == nil || motionIsPet(selected) || (s.server != nil && s.server.isTotemGUID(selected.GUID)) {
			s.sendSysMessage("Select a creature first.")
			return true
		}
		var entry uint32
		var tempSummon bool
		if s.server != nil {
			s.server.motionMu.Lock()
			entry, tempSummon = selected.Entry, selected.IsTempSummon
			s.server.motionMu.Unlock()
		}
		if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok && flagsExtra&npcbotEntryMask != 0 {
			s.sendSysMessage("Selected creature has botAI assigned, use '.npcbot delete' instead")
			return true
		}
		if tempSummon {
			// TempSummon::UnSummon twin: leave the world now, firing
			// the OnRemove event like RemoveFromWorld does.
			if s.server != nil {
				s.server.motionMu.Lock()
				mapID, instanceID, x, y, z, phase := selected.Map, selected.InstanceID, selected.X, selected.Y, selected.Z, selected.PhaseMask
				delete(s.server.motionMapLocked(selected.Map, selected.InstanceID), selected.GUID)
				s.server.motionMu.Unlock()
				destroyBuf := protocol.NewBuffer(9)
				destroyBuf.WriteU64(selected.GUID)
				destroyBuf.WriteU8(0)
				s.server.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), mapID, instanceID, x, y, z, phase)
				s.server.fireCreatureLuaEvent(ctx, selected, scripting.CreatureEventOnRemove)
			}
			s.sendSysMessage("Creature deleted.") // LANG_COMMAND_DELCREATMESSAGE 270
			return true
		}
		guid = uint32(selected.GUID & 0x00FFFFFF)
	} else {
		var ok bool
		guid, ok = goParseLinkOrUint(args[0], "creature")
		if !ok {
			return false
		}
	}
	// Creature::DeleteFromDB (Creature.cpp:1861): the row must exist first,
	// then every live creature with that spawn id on every map with the
	// row's map id is despawned, and the creature, spawn_group,
	// creature_addon, game_event_creature, game_event_model_equip and
	// linked_respawn rows are deleted. The respawn-time removal has no Go
	// model and stays open.
	if !s.deleteCreatureFromDB(ctx, guid) {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return true
	}
	s.sendSysMessage("Creature deleted.") // LANG_COMMAND_DELCREATMESSAGE 270
	return true
}

// npcDespawnLiveSpawn is the DeleteFromDB live leg (Creature.cpp:1861):
// every live creature with the spawn id, on every instance of the map,
// leaves the world (destroy broadcast + OnRemove event, the temp-summon
// UnSummon twin). Pets, temp summons and totems carry no spawn id and
// never match.
func (s *session) npcDespawnLiveSpawn(ctx context.Context, guid, mapID uint32) {
	if s.server == nil {
		return
	}
	type doomed struct {
		motion                   *creatureMotion
		mapID, instanceID, phase uint32
		x, y, z                  float32
	}
	var dead []doomed
	s.server.motionMu.Lock()
	for key, motions := range s.server.instanceCreatureMotion {
		if key.MapID != mapID {
			continue
		}
		for _, motion := range motions {
			if motion == nil || uint32(motion.GUID&0x00FFFFFF) != guid {
				continue
			}
			if motionIsPet(motion) || motion.IsTempSummon || s.server.isTotemGUID(motion.GUID) {
				continue
			}
			dead = append(dead, doomed{motion: motion, mapID: motion.Map, instanceID: motion.InstanceID, x: motion.X, y: motion.Y, z: motion.Z, phase: motion.PhaseMask})
			delete(motions, motion.GUID)
		}
	}
	s.server.motionMu.Unlock()
	for _, d := range dead {
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(d.motion.GUID)
		destroyBuf.WriteU8(0)
		s.server.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), d.mapID, d.instanceID, d.x, d.y, d.z, d.phase)
		s.server.fireCreatureLuaEvent(ctx, d.motion, scripting.CreatureEventOnRemove)
	}
}

// deleteCreatureFromDB is the Creature::DeleteFromDB twin
// (Creature.cpp:1861-1933): false when the spawn has no creature row
// (GetCreatureData nil); on success every live creature with that spawn
// id is despawned (npcDespawnLiveSpawn) and the creature, spawn-group,
// creature_addon, game-event and linked-respawn rows are deleted. The
// respawn-time removal has no Go model. The game_event_model_equip leg
// (WORLD_DEL_GAME_EVENT_MODEL_EQUIP) and both linked_respawn legs
// (guid + linkedGuid, linkType 0/1 and 0/3, SpawnData.h:109-112) were
// missing from the npc-delete port; the wp arms share this helper.
func (s *session) deleteCreatureFromDB(ctx context.Context, guid uint32) bool {
	db := s.npcWorldDB()
	if db == nil {
		return false
	}
	var rowMap uint32
	if err := db.QueryRowContext(ctx, "SELECT map FROM creature WHERE guid = ?", guid).Scan(&rowMap); err != nil {
		return false
	}
	if s.server != nil {
		s.npcDespawnLiveSpawn(ctx, guid, rowMap)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", guid); err != nil {
		return false
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM spawn_group WHERE spawnType = 0 AND spawnId = ?", guid)
	_, _ = db.ExecContext(ctx, "DELETE FROM creature_addon WHERE guid = ?", guid)
	_, _ = db.ExecContext(ctx, "DELETE FROM game_event_creature WHERE guid = ?", guid)
	_, _ = db.ExecContext(ctx, "DELETE FROM game_event_model_equip WHERE guid = ?", guid)
	_, _ = db.ExecContext(ctx, "DELETE FROM linked_respawn WHERE guid = ? AND linkType IN (0, 1)", guid)
	_, _ = db.ExecContext(ctx, "DELETE FROM linked_respawn WHERE linkedGuid = ? AND linkType IN (0, 3)", guid)
	return true
}

// handleNPCNear mirrors HandleNpcNearCommand (cs_npc.cpp:568): a distance-
// ordered creature scan on the handler's map, pure SQL. C++ binds
// distance^2 unconditionally, so a negative distance searches |distance|
// and prints the signed value; an unparseable token fails the C++ typed
// parse (syntax here).
func (s *session) handleNPCNear(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNPCNear) {
		return false
	}
	db := s.npcWorldDB()
	if db == nil {
		return true
	}
	distance := 10.0
	if len(args) > 1 {
		return false
	}
	if len(args) > 0 {
		d, err := parseNPCFloat(args[0])
		if err != nil {
			return false
		}
		distance = d
	}
	p := s.player
	rows, err := db.QueryContext(ctx,
		`SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, c.map, t.name
		 FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		 WHERE c.map = ? AND (POW(c.position_x - ?, 2) + POW(c.position_y - ?, 2) + POW(c.position_z - ?, 2)) <= ?
		 ORDER BY (POW(c.position_x - ?, 2) + POW(c.position_y - ?, 2) + POW(c.position_z - ?, 2))`,
		p.Map, p.X, p.Y, p.Z, distance*distance, p.X, p.Y, p.Z)
	if err != nil {
		return true
	}
	defer rows.Close()
	var count uint32
	for rows.Next() {
		var guid, entry, mapID uint32
		var x, y, z float64
		var name string
		if err := rows.Scan(&guid, &entry, &x, &y, &z, &mapID, &name); err != nil {
			continue
		}
		// LANG_CREATURE_LIST_CHAT 515 (same shape as the list port).
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hcreature_entry:%d|h[%s]|h|r - x: %f y: %f z: %f mapid: %d (GUID %s %s)",
			guid, entry, name, x, y, z, mapID, "", ""))
		count++
	}
	// LANG_COMMAND_NEAR_NPC_MESSAGE 556.
	s.sendSysMessage(fmt.Sprintf("%d creatures found within %g yards.", count, distance))
	return true
}

// parseNPCFloat mirrors the C++ float ArgInfo (ChatCommandArgs.h:54-72):
// StringTo<float>(token, 0) is base-0 strtod with the whole string
// consumed and non-finite values rejected — so 0x10 reads 16 and 0x10.8
// reads 16.5, while 10abc, inf, nan and 1_000 are parser errors. Go's
// Sscanf accepted the junk and rejected the hex; ParseFloat accepts inf,
// nan and underscores and rejects bare hex, so the hex leg is normalized
// to a p-exponent form first and the rest is rejected explicitly.
func parseNPCFloat(arg string) (float64, error) {
	if arg == "" || strings.Contains(arg, "_") {
		return 0, strconv.ErrSyntax
	}
	lower := strings.ToLower(arg)
	if strings.Contains(lower, "inf") || strings.Contains(lower, "nan") {
		return 0, strconv.ErrSyntax
	}
	s := arg
	trimmed := strings.TrimPrefix(strings.TrimPrefix(lower, "+"), "-")
	if strings.HasPrefix(trimmed, "0x") && !strings.ContainsAny(trimmed, "pP") {
		s = arg + "p0"
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, strconv.ErrSyntax
	}
	return f, nil
}
