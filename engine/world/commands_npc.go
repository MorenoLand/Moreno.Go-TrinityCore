package world

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
// the real spawn/move/delete operations; the live-Creature legs (respawn,
// motion, grid) have no Go bridge and are noted per arm.
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
// chunk 2.
func (s *session) handleCmdNPC(ctx context.Context, args []string) {
	const syntax = "Syntax: .npc add|set|move|delete|near|info|playemote|say|textemote|whisper|yell|tame|spawngroup|despawngroup|follow|evade|showloot"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("add", sub):
		s.handleNPCAddTable(ctx, rest)
	case strings.HasPrefix("move", sub):
		s.handleNPCMove(ctx, rest)
	case strings.HasPrefix("delete", sub):
		s.npcDeleteSub(ctx, rest)
	case strings.HasPrefix("near", sub):
		s.handleNPCNear(ctx, rest)
	default:
		if s.handleNPCChunk2(ctx, sub, rest) {
			return
		}
		s.sendSysMessage(syntax)
	}
}

// handleNPCAddTable dispatches the "add" sub-table (cs_npc.cpp:61-71).
func (s *session) handleNPCAddTable(ctx context.Context, args []string) {
	const syntax = "Syntax: .npc add [formation|item|move|temp] <args> | .npc add <entry>"
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandNPCAdd) {
			return
		}
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("formation", sub):
		s.handleNPCAddFormation(ctx)
	case strings.HasPrefix("item", sub):
		s.handleNPCAddItem(ctx)
	case strings.HasPrefix("move", sub):
		s.handleNPCAddMove(ctx, rest)
	case strings.HasPrefix("temp", sub):
		s.handleNPCAddTemp(ctx)
	default:
		// The "" table entry: ".npc add <entry>".
		s.handleNPCAdd(ctx, args)
	}
}

// handleNPCAddFormation is documented-blocked (cs_npc.cpp:1220): formations
// need the selected live creature plus the FormationMgr bridge.
func (s *session) handleNPCAddFormation(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCAddFormation) {
		return
	}
	s.sendSysMessage("npc add formation is not supported: live creature selection and the formation manager have no Go bridge.")
}

// handleNPCAddItem is documented-blocked (cs_npc.cpp:177): vendor-list edits
// need the selected live vendor creature.
func (s *session) handleNPCAddItem(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCAddItem) {
		return
	}
	s.sendSysMessage("npc add item is not supported: selected-unit creature targets have no Go bridge.")
}

// handleNPCAddTemp is documented-blocked (cs_npc.cpp:1025): temp summons need
// the live creature summon bridge.
func (s *session) handleNPCAddTemp(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCAddTemp) {
		return
	}
	s.sendSysMessage("npc add temp is not supported: live creature summons have no Go bridge.")
}

// handleNPCAddMove mirrors HandleNpcAddMoveCommand (cs_npc.cpp:213): the
// creature row must exist; movement_type becomes WAYPOINT (2). Pure DB leg,
// fully native.
func (s *session) handleNPCAddMove(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCAddMove) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .npc add move <guid>")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	guid := uint32(cAtoi(args[0]))
	var exists uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE guid = ?", guid).Scan(&exists); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return
	}
	if _, err := db.ExecContext(ctx, "UPDATE creature SET MovementType = 2 WHERE guid = ?", guid); err != nil {
		return
	}
	s.sendSysMessage("Waypoint added.") // LANG_WAYPOINT_ADDED 234
}

// handleNPCAdd mirrors HandleNpcAddCommand (cs_npc.cpp:117): the entry must
// exist in creature_template and must not be an npcbot entry. The live spawn
// legs (Creature::Create/AddToMap/grid) have no Go bridge, but creature
// visibility is DB-driven, so the creature row insert IS the spawn.
func (s *session) handleNPCAdd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCAdd) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .npc add <entry>")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	entry := uint32(cAtoi(args[0]))
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok {
		return // C++: silent false on unknown template
	}
	if s.npcbotSpawnBlocked(entry, flagsExtra) {
		return
	}
	var guid uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid); err != nil {
		return
	}
	p := s.player
	// spawntimesecs default mirrors the world DB column default for fresh
	// rows; the live-spawn respawn-delay legs have no Go bridge.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, ?, ?, 1, 1, ?, ?, ?, ?, 120, 0)",
		guid, entry, p.Map, p.X, p.Y, p.Z, p.Orientation); err != nil {
		return
	}
	// C++ reports success silently.
}

// handleNPCMove mirrors HandleNpcMoveCommand (cs_npc.cpp:601) for the guid
// form: the row must exist on the handler's map; position becomes the
// handler's position. The selection form and the live respawn legs have no Go
// bridge.
func (s *session) handleNPCMove(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCMove) {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("npc move needs a creature GUID: selected-unit creature targets have no Go bridge.")
		return
	}
	guid := uint32(cAtoi(args[0]))
	var rowMap uint32
	var entry uint32
	if err := db.QueryRowContext(ctx, "SELECT map, id FROM creature WHERE guid = ?", guid).Scan(&rowMap, &entry); err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
		return
	}
	if rowMap != s.player.Map {
		s.sendSysMessage(fmt.Sprintf("Creature #%d is on a different map.", guid)) // LANG_COMMAND_CREATUREATSAMEMAP 272
		return
	}
	if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok {
		if s.npcbotMoveBlocked(guid, entry, flagsExtra) {
			return
		}
	}
	p := s.player
	if _, err := db.ExecContext(ctx, "UPDATE creature SET position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
		p.X, p.Y, p.Z, p.Orientation, guid); err != nil {
		return
	}
	s.sendSysMessage("Creature moved.") // LANG_COMMAND_CREATUREMOVED 271
}

// handleNPCDelete mirrors HandleNpcDeleteCommand (cs_npc.cpp:297) for the
// guid form: DELETE FROM creature. The selection form and the live UnSummon
// leg have no Go bridge.
func (s *session) handleNPCDelete(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCDelete) {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("npc delete needs a creature GUID: selected-unit creature targets have no Go bridge.")
		return
	}
	guid := uint32(cAtoi(args[0]))
	res, err := db.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", guid)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.sendSysMessage("Creature deleted.") // LANG_COMMAND_DELCREATMESSAGE 270
	} else {
		s.sendSysMessage(fmt.Sprintf("Creature with GUID %d not found.", guid)) // LANG_COMMAND_CREATGUIDNOTFOUND 287
	}
}

// handleNPCNear mirrors HandleNpcNearCommand (cs_npc.cpp:568): a distance-
// ordered creature scan on the handler's map, pure SQL, fully native.
func (s *session) handleNPCNear(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCNear) {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	distance := 10.0
	if len(args) > 0 {
		if d, err := parseNPCFloat(args[0]); err == nil && d > 0 {
			distance = d
		}
	}
	p := s.player
	rows, err := db.QueryContext(ctx,
		`SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, c.map, t.name
		 FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		 WHERE c.map = ? AND (POW(c.position_x - ?, 2) + POW(c.position_y - ?, 2) + POW(c.position_z - ?, 2)) <= ?
		 ORDER BY (POW(c.position_x - ?, 2) + POW(c.position_y - ?, 2) + POW(c.position_z - ?, 2))`,
		p.Map, p.X, p.Y, p.Z, distance*distance, p.X, p.Y, p.Z)
	if err != nil {
		return
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
}

func parseNPCFloat(arg string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(arg, "%f", &f)
	return f, err
}
