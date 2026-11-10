package world

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// wp command port: wp_commandscript (cs_wp.cpp), the "wp" root with 7 arms
// (add, event, load, modify, unload, reload, show). THIRTY-NINTH and LAST of
// the ported Commands groups (cs_script_loader.cpp decl 58 / call 103; call
// order titles(102) -> wp(103)). Trinity checks permission only on the
// invoker leaf node (ChatCommand.cpp:487), so each arm gates exactly its own
// C++ permission (RBAC.h:635-642, 8 constants 767-774 in permissions.go).
// The root permission 767 is DEAD in C++ — the root is the deprecated 6-arg
// nullptr+subtable overload (ChatCommand.h:261-263 drops RBACPermissions),
// so bare ".wp" prints the syntax listing ungated (same pattern as the
// titles/ticket/reset ports); permissionCommandWp stays in permissions.go as
// documentation.
//
// The Go worldserver has a real waypoint system (creaturemotion.go:
// loadWaypoints reads waypoint_data on demand; creature_addon.path_id binds a
// spawn to a path; the live motion map can be refreshed). Arms:
//
//   - `add [pathid]`: native — INSERT INTO waypoint_data at the GM's
//     position (new path = max id + 1 when no creature selected and no id).
//   - `event add|del|mod|listid`: native waypoint_scripts table ops.
//   - `load <pathid>`: native DB (creature_addon path_id upsert + creature
//     MovementType = 2) plus live motion-map refresh, creature Say, and
//     walk-home on unload (MoveTargetedHome spline).
//   - `modify delay|action|action_chance|move_type|del|move`: partial —
//     needs a selected visual waypoint (entry 1); the wpguid link column may
//     be absent, so the path/point is resolved by position proximity (the C++
//     fallback query). `del`/`move` run the DeleteFromDB twin (live despawn
//     + multi-table cleanup, 242 on failure); `move` recreates with a new
//     GUID and leaves wpguid stale, as C++ does. The 222 search message
//     is sent on a wpguid miss (cs_wp.cpp:592); its text and the other
//     waypoint LANG texts are the acore_string enUS rows (TC
//     trinity_string lineage, rows 220-245).
//   - `unload`: native DB (DELETE creature_addon, MovementType = 0) plus live
//     motion-map refresh, walk-home spline, and creature Say.
//   - `reload <id>`: re-reads waypoint_data and replaces Points on every
//     live motion patrolling that path (ReloadPath twin); a missing path
//     is a silent no-op for live motions.
//   - `show on|off|first|last|info [pathid]`: visual waypoints are entry-1
//     creature rows (the npc-add pattern) spawned at the player's
//     phase/spawn mode; `on`/`first`/`last` mimic the selected creature's
//     display at half scale (`on` also sets level min(point, 255)) and
//     abort with 228/230 on spawn failure; `on`/`off` cleanup runs the
//     DeleteFromDB twin per visual (242 + TOOFAR trio on failure).
//
// Creature selection resolves through the live motion map — the
// getSelectedCreature twin (Chat.cpp:336-341, ObjectAccessor::
// GetCreatureOrPetOrVehicle). The selection GUID packs entry bits above the
// low 24 (ObjectGuid.h:134), so truncating it to uint32 corrupts the spawn
// guid for every entry != 0 mod 256; the old code also required a `creature`
// DB row, so pets, vehicles, temp summons and every real creature selection
// resolved as no target. The motion lookup type-checks for free (players and
// gameobjects are never creature motions) and needs no DB row, matching the
// C++ live-object resolution. LANG texts below are the acore_string enUS
// texts (TC trinity_string lineage), rendered printf-style; no in-tree
// TDB seed exists.
//
// R13: the root and all 7 arms return bool — bare/unknown subcommands
// return false (the framework shows the help listing), denial is a silent
// false (the C++ framework denies before invoking the handler), framework
// parse failures (empty/missing args, unknown inner verbs) are silent
// false, message legs return true when C++ sets SetSentErrorMessage, and
// message + FALSE when C++ returns false without it (ChatCommand.cpp:
// 296-309 shows help after those: modify no-visual, modify missing value,
// and the VP_NOTCREATED/NOTCREATED spawn failures).

// wpSelectedCreature resolves the selected creature's spawn guid and entry
// via the live motion map (castCommandSelectedCreature precedent). The
// spawn guid is the low 24 bits of the motion GUID (objectGUIDCounter);
// motion.Entry is the template entry (1 = VISUAL_WAYPOINT).
func (s *session) wpSelectedCreature(ctx context.Context) (guid, entry uint32, ok bool) {
	motion := s.findCreatureMotion(s.selection)
	if motion == nil {
		return 0, 0, false
	}
	return objectGUIDCounter(motion.GUID), motion.Entry, true
}

// playerPhaseMaskForSpawn is Player::GetPhaseMaskForSpawn
// (Player.cpp:25279-25297) for the session player: a .modify phase
// override stands as-is; a GM reads the first SPELL_AURA_PHASE aura's
// misc value, else PHASEMASK_NORMAL; a non-GM reads the current phase
// mask (phase-aura OR, else 1); the result strips the normal bit when
// another phase bit is set, falling back to it. Mirrors the Lua
// GetPhaseMaskForSpawn bridge (lua_player.go:827-852); the npc-add arm
// shares it (cs_npc.cpp:163 SaveToDB phase leg).
func (s *session) playerPhaseMaskForSpawn() uint32 {
	if s == nil || s.player == nil {
		return 1
	}
	if s.player.PhaseMaskOverrideSet {
		return s.player.PhaseMaskOverride
	}
	var phase uint32
	if s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0 {
		phase = 1
		s.castMu.Lock()
		for _, aura := range s.activeAuras {
			if aura != nil && !aura.Stopped && aura.AuraType == spellAuraPhase {
				phase = uint32(aura.MiscValue)
				break
			}
		}
		s.castMu.Unlock()
	} else {
		phase = s.currentPlayerPhaseMask()
	}
	if stripped := phase &^ 1; stripped != 0 {
		return stripped
	}
	return 1
}

// playerSpawnMode is Map::GetSpawnMode for the player's map (cs_npc.cpp:163
// SaveToDB spawnMask leg): the instance difficulty (dungeon/raid) on
// instance maps, normal elsewhere. Shared by npc-add and wp visual spawns.
func (s *session) playerSpawnMode() uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	if s.isDungeonMap(s.player.Map) {
		return uint32(s.player.DungeonDifficulty)
	}
	if raidMapID(s.server, s.player.Map) {
		return uint32(s.player.RaidDifficulty)
	}
	return 0
}

// wpRefreshMotion refreshes a live creature's motion entry after a path
// load/unload (creaturemotion.go motion map).
func (s *session) wpRefreshMotion(ctx context.Context, motion *creatureMotion, pathID uint32, moveType uint32) {
	if s.server == nil || motion == nil {
		return
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion.MoveType = moveType
	motion.PathID = pathID
	motion.NextIdx = 0
	if moveType == 2 && pathID != 0 {
		motion.Points = s.server.loadWaypoints(ctx, pathID)
	} else {
		motion.Points = nil
	}
	motion.Refreshed = time.Now()
}

// handleCmdWp dispatches the "wp" root (cs_wp.cpp:51-57).
func (s *session) handleCmdWp(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Dead root perm 767 (see header): the root has no handler, so
		// the framework shows the help listing.
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("add", sub):
		return s.handleWpAdd(ctx, rest)
	case strings.HasPrefix("event", sub):
		return s.handleWpEvent(ctx, rest)
	case strings.HasPrefix("load", sub):
		return s.handleWpLoad(ctx, rest)
	case strings.HasPrefix("modify", sub):
		return s.handleWpModify(ctx, rest)
	case strings.HasPrefix("unload", sub):
		return s.handleWpUnload(ctx)
	case strings.HasPrefix("reload", sub):
		return s.handleWpReload(ctx, rest)
	case strings.HasPrefix("show", sub):
		return s.handleWpShow(ctx, rest)
	default:
		return false
	}
}

// handleWpAdd mirrors HandleWpAddCommand (cs_wp.cpp:86).
func (s *session) handleWpAdd(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpAdd) {
		return false
	}
	if s.player == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	var pathID uint32
	if len(args) > 0 {
		pathID = uint32(cAtoi(args[0]))
	} else if guid, _, ok := s.wpSelectedCreature(ctx); ok {
		// == target->GetWaypointPath(): the creature's own path, 0 when it
		// has none — C++ does NOT start a new path here (cs_wp.cpp:97-100).
		_ = wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID)
	} else {
		var maxID uint32
		_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM waypoint_data").Scan(&maxID)
		pathID = maxID + 1
		s.sendSysMessage("New path started.")
	}
	if pathID == 0 {
		s.sendSysMessage("Current creature haven't loaded path.")
		return true
	}
	var point uint32
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(point), 0) FROM waypoint_data WHERE id = ?", pathID).Scan(&point)
	point++
	p := s.player
	if _, err := wdb.ExecContext(ctx, "INSERT INTO waypoint_data (id, point, position_x, position_y, position_z, orientation) VALUES (?, ?, ?, ?, ?, ?)",
		pathID, point, p.X, p.Y, p.Z, p.Orientation); err != nil {
		return true
	}
	s.sendSysMessage(fmt.Sprintf("PathID: %d: Waypoint %d created.", pathID, point))
	return true
}

// handleWpEvent mirrors HandleWpEventCommand (cs_wp.cpp:252): add, del, mod,
// listid on waypoint_scripts.
func (s *session) handleWpEvent(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpEvent) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	show := args[0] // C++ compares the inner verb exact-case (cs_wp.cpp:298:
	// std::string ==, no ToLower); only the outer wp dispatch is
	// case-insensitive (StringStartsWithI, ChatCommand.cpp:230).
	// C++ validates the verb (cs_wp.cpp:300-303) before touching the DB,
	// so an unknown verb is false even with no database.
	switch show {
	case "add", "del", "mod", "listid":
	default:
		return false
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	var id uint32
	if len(args) > 1 {
		id = uint32(cAtoi(args[1]))
	}
	switch show {
	case "add":
		if id == 0 {
			var maxID uint32
			_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) FROM waypoint_scripts").Scan(&maxID)
			id = maxID + 1
		} else {
			var existing int
			if wdb.QueryRowContext(ctx, "SELECT id FROM waypoint_scripts WHERE guid = ?", id).Scan(&existing) == nil {
				s.sendSysMessage(fmt.Sprintf("Wp Event: You have choosed an existing waypoint script guid: %d", id))
				return true
			}
		}
		if _, err := wdb.ExecContext(ctx, "INSERT INTO waypoint_scripts (guid) VALUES (?)", id); err == nil {
			s.sendSysMessage(fmt.Sprintf("Wp Event: New waypoint event added: %d", id))
		}
	case "del":
		// C++ checks arg presence, not the parsed id (cs_wp.cpp:389-392):
		// ".wp event del 0" queries guid 0 and reports it missing.
		if len(args) < 2 {
			s.sendSysMessage("ERROR: Waypoint script guid not present.")
			return true
		}
		id := uint32(cAtoi(args[1]))
		res, err := wdb.ExecContext(ctx, "DELETE FROM waypoint_scripts WHERE guid = ?", id)
		if err != nil {
			return true
		}
		if n, _ := res.RowsAffected(); n > 0 {
			s.sendSysMessage(fmt.Sprintf("Wp Event: Waypoint script removed: %d", id))
		} else {
			s.sendSysMessage(fmt.Sprintf("Wp Event: ERROR: you have selected a non existing script: %d", id))
		}
	case "listid":
		// C++ checks arg presence, not the parsed id (cs_wp.cpp:348-353).
		if len(args) < 2 {
			s.sendSysMessage("Wp Event: You must provide waypoint script id.")
			return true
		}
		id = uint32(cAtoi(args[1]))
		rows, err := wdb.QueryContext(ctx, "SELECT guid, delay, command, datalong, datalong2, dataint, x, y, z, o FROM waypoint_scripts WHERE id = ?", id)
		if err != nil {
			return true
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			var guid, delay, command, datalong, datalong2, dataint uint32
			var x, y, z, o float64
			if rows.Scan(&guid, &delay, &command, &datalong, &datalong2, &dataint, &x, &y, &z, &o) != nil {
				continue
			}
			// C++ prints "datatext: %s" with GetCString() on the dataint
			// field (WorldDatabase.cpp WORLD_SEL_WAYPOINT_SCRIPT_BY_ID has no
			// datatext column) — the dataint value rendered as a string.
			s.sendSysMessage(fmt.Sprintf("id: %d, guid: %d, delay: %d, command: %d, datalong: %d, datalong2: %d, datatext: %d, posx: %f, posy: %f, posz: %f, orientation: %f",
				id, guid, delay, command, datalong, datalong2, dataint, x, y, z, o))
			found = true
		}
		if !found {
			s.sendSysMessage(fmt.Sprintf("Wp Event: No waypoint scripts found on id: %d", id))
		}
	case "mod":
		return s.handleWpEventMod(ctx, args)
	default:
		return false
	}
	return true
}

// handleWpEventMod mirrors the "mod" leg of HandleWpEventCommand
// (cs_wp.cpp:413-527): sequential arg checks with the C++-exact literal
// messages (including its "vallid"/"Wypoint scipt" typos), the setid special
// branch (no existence check, updates the id column), and per-field update
// messages. Field names compare exact-case, like the C++ validity list.
func (s *session) handleWpEventMod(ctx context.Context, args []string) bool {
	wdb := s.server.WorldStore.DB
	if len(args) < 2 {
		s.sendSysMessage("ERROR: Waypoint script guid not present.")
		return true
	}
	id := uint32(cAtoi(args[1]))
	if id == 0 {
		s.sendSysMessage("ERROR: No vallid waypoint script id not present.")
		return true
	}
	if len(args) < 3 {
		s.sendSysMessage("ERROR: No argument present.")
		return true
	}
	field := args[2] // C++ compares the mod field exact-case
	// (cs_wp.cpp:440-451 validity list, 463+ branches); "DELAY" is not a
	// valid field in C++.
	if field != "setid" && field != "delay" && field != "command" && field != "datalong" &&
		field != "datalong2" && field != "dataint" && field != "posx" && field != "posy" &&
		field != "posz" && field != "orientation" {
		s.sendSysMessage("ERROR: No valid argument present.")
		return true
	}
	if len(args) < 4 {
		s.sendSysMessage("ERROR: No additional argument present.")
		return true
	}
	value := args[3]
	if field == "setid" {
		newID := uint32(cAtoi(value))
		if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_scripts SET id = ? WHERE guid = ?", newID, id); err != nil {
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Wp Event: Wypoint scipt guid: %d id changed: %d", newID, id))
		return true
	}
	var existing int
	if wdb.QueryRowContext(ctx, "SELECT id FROM waypoint_scripts WHERE guid = ?", id).Scan(&existing) != nil {
		s.sendSysMessage("ERROR: You have selected an non existing waypoint script guid.")
		return true
	}
	switch field {
	case "posx", "posy", "posz", "orientation":
		cols := map[string]string{"posx": "x", "posy": "y", "posz": "z", "orientation": "o"}
		names := map[string]string{"posx": "position_x", "posy": "position_y", "posz": "position_z", "orientation": "orientation"}
		if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_scripts SET "+cols[field]+" = ? WHERE guid = ?", value, id); err != nil {
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Waypoint script: %d %s updated.", id, names[field]))
		return true
	case "dataint":
		// C++ interpolates atoi(value) through '%u' (cs_wp.cpp:526-530), so
		// "-5" stores 4294967291; the raw string as a bound parameter would
		// clamp to 4294967295 (or error in strict mode).
		if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_scripts SET dataint = ? WHERE guid = ?", uint32(cAtoi(value)), id); err != nil {
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Waypoint script: %d dataint updated.", id))
		return true
	case "delay", "command", "datalong", "datalong2":
		if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_scripts SET "+field+" = ? WHERE guid = ?", value, id); err != nil {
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Waypoint script: %d: %s updated.", id, field))
		return true
	default:
		s.sendSysMessage("ERROR: No valid argument present.")
	}
	return true
}

// handleWpLoad mirrors HandleWpLoadCommand (cs_wp.cpp:163).
func (s *session) handleWpLoad(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpLoad) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	guid, entry, ok := s.wpSelectedCreature(ctx)
	if !ok {
		s.sendSysMessage("Select a creature.") // LANG_SELECT_CREATURE 2
		return true
	}
	if entry == 1 { // VISUAL_WAYPOINT
		s.sendSysMessage("You want to load path to a waypoint? Aren't you?")
		return true
	}
	pathID := uint32(cAtoi(args[0]))
	if pathID == 0 {
		s.sendSysMessage("No valid path number provided.")
		return true
	}
	wdb := s.server.WorldStore.DB
	var hasAddon int
	if wdb.QueryRowContext(ctx, "SELECT 1 FROM creature_addon WHERE guid = ?", guid).Scan(&hasAddon) == nil {
		_, _ = wdb.ExecContext(ctx, "UPDATE creature_addon SET path_id = ? WHERE guid = ?", pathID, guid)
	} else {
		_, _ = wdb.ExecContext(ctx, "INSERT INTO creature_addon (guid, path_id) VALUES (?, ?)", guid, pathID)
	}
	_, _ = wdb.ExecContext(ctx, "UPDATE creature SET MovementType = 2 WHERE guid = ?", guid)
	// LoadPath + SetDefaultMovementType(WAYPOINT) + Initialize
	// (cs_wp.cpp:223-226): the live motion refreshes from the selected
	// motion directly — the old DB-row lookup no-opped for pets, vehicles
	// and temp summons, which C++ loads the path on all the same.
	if motion := s.castCommandSelectedCreature(); motion != nil {
		s.wpRefreshMotion(ctx, motion, pathID, 2)
		// target->Say("Path loaded.", LANG_UNIVERSAL): the creature speaks
		// in-world (npcCreatureChat twin), not a GM-only sysmessage.
		s.npcCreatureChat(motion, 0x0C, languageUniversal, "Path loaded.", float64(s.server.Config.ChatListenRangeSay))
		return true
	}
	s.sendSysMessage("Path loaded.")
	return true
}

// handleWpUnload mirrors HandleWpUnLoadCommand (cs_wp.cpp:231).
func (s *session) handleWpUnload(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandWpUnload) {
		return false
	}
	guid, _, ok := s.wpSelectedCreature(ctx)
	if !ok {
		s.sendSysMessage("You must select target.")
		return true
	}
	wdb := s.server.WorldStore.DB
	var pathID uint32
	if wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID) != nil {
		// C++ GetCreatureAddon() is nil without a creature_addon row, and
		// unload then returns silently (cs_wp.cpp:243-244) — only an
		// existing addon row with path_id 0 reports "no loaded path".
		return true
	}
	if pathID == 0 {
		s.sendSysMessage("Target have no loaded path.")
		return true
	}
	_, _ = wdb.ExecContext(ctx, "DELETE FROM creature_addon WHERE guid = ?", guid)
	_, _ = wdb.ExecContext(ctx, "UPDATE creature SET MovementType = 0 WHERE guid = ?", guid)
	// LoadPath(0) + SetDefaultMovementType(IDLE) + MoveTargetedHome +
	// Initialize + Say (cs_wp.cpp:268-279): the creature walks home on a
	// HomeMovementGenerator spline (launchCreatureScriptSpline with the
	// home move type, which fires OnReachHome on arrival) instead of
	// standing where it was unloaded, then speaks in-world.
	if motion := s.castCommandSelectedCreature(); motion != nil {
		s.wpRefreshMotion(ctx, motion, 0, 0)
		if s.server != nil {
			s.server.motionMu.Lock()
			homeX, homeY, homeZ := motion.HomeX, motion.HomeY, motion.HomeZ
			s.server.motionMu.Unlock()
			s.server.launchCreatureScriptSpline(motion, homeX, homeY, homeZ, true, movementGeneratorHome, 0, false)
		}
		s.npcCreatureChat(motion, 0x0C, languageUniversal, "Path unloaded.", float64(s.server.Config.ChatListenRangeSay))
		return true
	}
	s.sendSysMessage("Path unloaded.")
	return true
}

// handleWpReload mirrors HandleWpReloadCommand (cs_wp.cpp:231):
// ReloadPath re-reads waypoint_data into the shared store
// (WaypointManager.cpp:85-130), so live patrollers on that path see the
// new nodes. Empty args or id 0 return false in C++ (framework help),
// not silent.
func (s *session) handleWpReload(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpReload) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	id := uint32(cAtoi(args[0]))
	if id == 0 {
		return false
	}
	wdb := s.server.WorldStore.DB
	if wdb != nil {
		var one int
		_ = wdb.QueryRowContext(ctx, "SELECT 1 FROM waypoint_data WHERE id = ? LIMIT 1", id).Scan(&one)
	}
	s.wpReloadLiveMotions(ctx, id)
	s.sendSysMessage(fmt.Sprintf("Loading Path: %d", id))
	return true
}

// wpReloadLiveMotions is the ReloadPath live leg: every live motion
// patrolling pathID on any map/instance re-reads the path (loadWaypoints
// filters invalid move_type rows like ReloadPath does) and keeps its
// current node, clamped when the new path is shorter. A missing path
// leaves live motions untouched — C++ returns early on no rows.
func (s *session) wpReloadLiveMotions(ctx context.Context, pathID uint32) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	points := s.server.loadWaypoints(ctx, pathID)
	if len(points) == 0 {
		return
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	for _, motions := range s.server.instanceCreatureMotion {
		for _, motion := range motions {
			if motion == nil || motion.PathID != pathID || motion.MoveType != 2 {
				continue
			}
			motion.Points = points
			if motion.NextIdx >= len(points) {
				motion.NextIdx = 0
			}
			motion.Refreshed = time.Now()
		}
	}
}

// wpVisualPoint resolves the selected visual waypoint (entry 1) to its
// (pathid, point): by wpguid when the column exists, else by position
// proximity (the C++ fallback, cs_wp.cpp:600-625). C++ takes the LAST
// matching row (do-while), so the proximity scan keeps the last hit too.
func (s *session) wpVisualPoint(ctx context.Context, guid uint32) (pathID, point uint32, ok bool) {
	wdb := s.server.WorldStore.DB
	if wdb.QueryRowContext(ctx, "SELECT id, point FROM waypoint_data WHERE wpguid = ?", guid).Scan(&pathID, &point) == nil {
		return pathID, point, true
	}
	// C++ sends LANG_WAYPOINT_NOTFOUNDSEARCH (222) on the wpguid miss and
	// then runs the position-proximity fallback (cs_wp.cpp:592-609).
	s.sendSysMessage(fmt.Sprintf("Creature (GUID: %d) No waypoint found - used 'wpguid'. Now trying to find it by its position...", guid))
	var x, y, z float64
	if wdb.QueryRowContext(ctx, "SELECT position_x, position_y, position_z FROM creature WHERE guid = ?", guid).Scan(&x, &y, &z) != nil {
		return 0, 0, false
	}
	rows, err := wdb.QueryContext(ctx, `SELECT id, point FROM waypoint_data
		WHERE abs(position_x - ?) <= 0.01 AND abs(position_y - ?) <= 0.01 AND abs(position_z - ?) <= 0.01`,
		x, y, z)
	if err != nil {
		return 0, 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var pid, pt uint32
		if rows.Scan(&pid, &pt) == nil {
			pathID, point, ok = pid, pt, true
		}
	}
	return pathID, point, ok
}

// handleWpModify mirrors HandleWpModifyCommand (cs_wp.cpp:541): delay,
// action, action_chance, move_type, del, move on the selected visual waypoint.
func (s *session) handleWpModify(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpModify) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	show := args[0] // C++ compares the inner verb exact-case (cs_wp.cpp:560-565:
	// std::string ==, no ToLower); only the outer wp dispatch is
	// case-insensitive.
	switch show {
	case "delay", "action", "action_chance", "move_type", "del", "move":
	default:
		// C++ returns false with no message (framework help).
		return false
	}
	guid, entry, ok := s.wpSelectedCreature(ctx)
	if !ok || entry != 1 { // VISUAL_WAYPOINT
		// C++ sends the message and returns false WITHOUT
		// SetSentErrorMessage (cs_wp.cpp:575-579), so the framework shows
		// help after it: message + false.
		s.sendSysMessage("ERROR: You must select a waypoint.")
		return false
	}
	pathID, point, ok := s.wpVisualPoint(ctx, guid)
	if !ok {
		// LANG_WAYPOINT_NOTFOUNDDBPROBLEM 223 on a failed proximity
		// fallback (cs_wp.cpp:610-614), acore_string row 223.
		s.sendSysMessage(fmt.Sprintf("No waypoint information was found for Creature (GUID: %d). Make sure 'wp show on' command was properly executed.", guid))
		return true
	}
	wdb := s.server.WorldStore.DB
	switch show {
	case "del":
		s.sendSysMessage(fmt.Sprintf("DEBUG: wp modify del, PathID: %d", pathID))
		// DeleteFromDB first (cs_wp.cpp:650-674): on failure the waypoint
		// row stays and LANG_WAYPOINT_NOTREMOVED (242) + error return.
		if !s.deleteCreatureFromDB(ctx, guid) {
			s.sendSysMessage(fmt.Sprintf("Warning: Could not delete WP from the world with ID: %d", guid)) // LANG_WAYPOINT_NOTREMOVED 242, acore_string row 242
			return true
		}
		_, _ = wdb.ExecContext(ctx, "DELETE FROM waypoint_data WHERE id = ? AND point = ?", pathID, point)
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET point = point - 1 WHERE id = ? AND point > ?", pathID, point)
		s.sendSysMessage("Waypoint removed.") // LANG_WAYPOINT_REMOVED 241
		return true
	case "move":
		if s.player == nil {
			return true
		}
		s.sendSysMessage(fmt.Sprintf("DEBUG: wp move, PathID: %d", pathID))
		// DeleteFromDB + recreate (cs_wp.cpp:677-721): a delete failure
		// reports LANG_WAYPOINT_VP_NOTCREATED (228) with
		// SetSentErrorMessage, while a failed recreate reports it WITHOUT
		// one, so the framework shows help after it: message + false.
		// The new visual gets a NEW spawn GUID at the player's
		// position/phase, and waypoint_data.wpguid is NOT updated — it
		// keeps pointing at the deleted GUID, so later modify/show arms
		// fall back to the position-proximity query.
		if !s.deleteCreatureFromDB(ctx, guid) {
			s.sendSysMessage("Could not create visual waypoint with creatureID: 1") // LANG_WAYPOINT_VP_NOTCREATED 228, acore_string row 228
			return true
		}
		p := s.player
		var newGUID uint32
		if wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&newGUID) != nil || newGUID == 0 {
			s.sendSysMessage("Could not create visual waypoint with creatureID: 1") // LANG_WAYPOINT_VP_NOTCREATED 228, acore_string row 228
			return false
		}
		if _, err := wdb.ExecContext(ctx,
			"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, 120, 0)",
			newGUID, p.Map, 1<<s.playerSpawnMode(), s.playerPhaseMaskForSpawn(), p.X, p.Y, p.Z, p.Orientation); err != nil {
			s.sendSysMessage("Could not create visual waypoint with creatureID: 1") // LANG_WAYPOINT_VP_NOTCREATED 228, acore_string row 228
			return false
		}
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE id = ? AND point = ?",
			p.X, p.Y, p.Z, p.Orientation, pathID, point)
		s.sendSysMessage("Waypoint changed.") // LANG_WAYPOINT_CHANGED 236
		return true
	}
	if len(args) < 2 {
		// LANG_WAYPOINT_ARGUMENTREQ (233) without SetSentErrorMessage
		// (cs_wp.cpp:640-644): message + help.
		s.sendSysMessage(fmt.Sprintf("Argument required for '%s'.", show)) // LANG_WAYPOINT_ARGUMENTREQ 233, acore_string row 233
		return false
	}
	if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_data SET "+show+" = ? WHERE id = ? AND point = ?", args[1], pathID, point); err != nil {
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Waypoint %s modified.", show)) // LANG_WAYPOINT_CHANGED_NO 237, acore_string row 237
	return true
}

// wpSpawnVisual spawns one visual-waypoint creature row (entry 1, the
// npc-add pattern) at the player's map/phase/spawn mode
// (cs_wp.cpp:854-857: Create + SaveToDB(map, 1 << spawnMode,
// GetPhaseMaskForSpawn)). A failed INSERT returns 0 — the callers abort
// with LANG_WAYPOINT_VP_NOTCREATED (228) / NOTCREATED (230) like C++.
// When a creature is selected (displayID != 0), the fresh visual mimics
// it live (cs_wp.cpp:868-872): its display id, half scale, and — for
// `on` only — level min(point, 255). The row carries no display/scale
// columns, so the mimic lands on the live motion materialized here
// (motionFor twin of Create + LoadFromDB); DisplayIDKnown/ScriptScaleKnown
// keep the sweep from re-affirming the template values, and LevelKnown
// does the same for the level.
func (s *session) wpSpawnVisual(ctx context.Context, x, y, z, o float64, displayID uint32, level uint32, withLevel bool) uint32 {
	wdb := s.server.WorldStore.DB
	if wdb == nil || s.player == nil || s.server == nil {
		return 0
	}
	var guid uint32
	if wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid) != nil || guid == 0 {
		return 0
	}
	if _, err := wdb.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, 120, 0)",
		guid, s.player.Map, 1<<s.playerSpawnMode(), s.playerPhaseMaskForSpawn(), x, y, z, o); err != nil {
		return 0
	}
	if displayID != 0 {
		var walkRate, runRate float64 = 1.0, 1.14286
		_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(speed_walk, 0), 1.0), COALESCE(NULLIF(speed_run, 0), 1.14286) FROM creature_template WHERE entry = 1").Scan(&walkRate, &runRate)
		motion := s.server.motionFor(ctx, guid, 1, s.player.Map, s.player.InstanceID, float32(x), float32(y), float32(z), float32(o), 0, 0, float32(walkRate), float32(runRate), 0)
		if motion != nil {
			s.server.motionMu.Lock()
			motion.DisplayID = displayID
			motion.DisplayIDKnown = true
			motion.ScriptScale = 0.5
			motion.ScriptScaleKnown = true
			if withLevel {
				motion.Level = level
				motion.LevelKnown = true
			}
			s.server.motionMu.Unlock()
		}
	}
	return guid
}

// wpSelectedDisplayID reads the selected creature's live display id for
// the show mimic (cs_wp.cpp:868-872: SetDisplayId(target->GetDisplayId()));
// 0 when nothing is selected.
func (s *session) wpSelectedDisplayID() uint32 {
	motion := s.castCommandSelectedCreature()
	if motion == nil || s.server == nil {
		return 0
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	return motion.DisplayID
}

// handleWpShow mirrors HandleWpShowCommand (cs_wp.cpp:729): on, off, first,
// last, info.
func (s *session) handleWpShow(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandWpShow) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	show := args[0] // C++ compares the inner verb exact-case (cs_wp.cpp:778,
	// 820, 918, 969, 1018: std::string ==, no ToLower); only the outer wp
	// dispatch is case-insensitive.
	// C++ pathid resolution (cs_wp.cpp:741-764): an explicit pathid wins and
	// warns when a creature is also selected; otherwise the selected
	// creature's path is used, and with neither the command errors.
	guid, _, hasTarget := s.wpSelectedCreature(ctx)
	var pathID uint32
	if len(args) > 1 {
		if hasTarget {
			s.sendSysMessage("Selected creature is ignored - provided GUID is used") // LANG_WAYPOINT_CREATSELECTED 224, acore_string row 224
		}
		pathID = uint32(cAtoi(args[1]))
	} else {
		if !hasTarget {
			s.sendSysMessage("Select a creature.") // LANG_SELECT_CREATURE 2
			return true
		}
		_ = wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID)
	}
	switch show {
	case "info":
		guid, entry, ok := s.wpSelectedCreature(ctx)
		if !ok || entry != 1 {
			s.sendSysMessage("You must select a visual waypoint.") // LANG_WAYPOINT_VP_SELECT 226, acore_string row 226
			return true
		}
		// C++ info looks up by wpguid only — no position fallback
		// (WORLD_SEL_WAYPOINT_DATA_ALL_BY_WPGUID, cs_wp.cpp:778-784).
		rows, err := wdb.QueryContext(ctx, "SELECT id, point, delay, move_type, action, action_chance FROM waypoint_data WHERE wpguid = ?", guid)
		if err != nil {
			return true
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			var pid, point, delay, moveType, action, actionChance uint32
			if rows.Scan(&pid, &point, &delay, &moveType, &action, &actionChance) != nil {
				continue
			}
			if !found {
				s.sendSysMessage("DEBUG: wp show info:")
			}
			found = true
			s.sendSysMessage(fmt.Sprintf("Show info: for current point: %d, Path ID: %d", point, pid))
			s.sendSysMessage(fmt.Sprintf("Show info: delay: %d", delay))
			s.sendSysMessage(fmt.Sprintf("Show info: Move flag: %d", moveType))
			s.sendSysMessage(fmt.Sprintf("Show info: Waypoint event: %d", action))
			s.sendSysMessage(fmt.Sprintf("Show info: Event chance: %d", actionChance))
		}
		if !found {
			// LANG_WAYPOINT_NOTFOUNDDBPROBLEM 223, acore_string row 223.
			s.sendSysMessage(fmt.Sprintf("No waypoint information was found for Creature (GUID: %d). Make sure 'wp show on' command was properly executed.", guid))
		}
	case "on":
		// C++ checks the path exists first (cs_wp.cpp:808-814).
		var one int
		if wdb.QueryRowContext(ctx, "SELECT 1 FROM waypoint_data WHERE id = ? LIMIT 1", pathID).Scan(&one) != nil {
			s.sendSysMessage("Path no found.")
			return true
		}
		s.sendSysMessage(fmt.Sprintf("DEBUG: wp on, PathID: %d", pathID))
		// Delete only this path's visuals (wpguids attached to its rows),
		// not every visual waypoint in the world (cs_wp.cpp:818-843).
		// Each DeleteFromDB failure reports LANG_WAYPOINT_NOTREMOVED (242)
		// for that guid, and any failure sends the TOOFAR trio (243-245)
		// once; the spawn loop still runs (cs_wp.cpp:818-847).
		var oldVisuals []uint32
		if vrows, err := wdb.QueryContext(ctx, "SELECT wpguid FROM waypoint_data WHERE id = ? AND wpguid <> 0", pathID); err == nil {
			for vrows.Next() {
				var g uint32
				if vrows.Scan(&g) == nil {
					oldVisuals = append(oldVisuals, g)
				}
			}
			vrows.Close()
		}
		hasError := false
		for _, g := range oldVisuals {
			if !s.deleteCreatureFromDB(ctx, g) {
				s.sendSysMessage(fmt.Sprintf("Warning: Could not delete WP from the world with ID: %d", g)) // LANG_WAYPOINT_NOTREMOVED 242, acore_string row 242
				hasError = true
			}
		}
		if hasError {
			s.sendSysMessage("This happens if the waypoint is too far away from your char.")      // LANG_WAYPOINT_TOOFAR1 243, acore_string row 243
			s.sendSysMessage("The WP is deleted from the database, but not from the world here.") // LANG_WAYPOINT_TOOFAR2 244, acore_string row 244
			s.sendSysMessage("They will disappear after a server restart.")                       // LANG_WAYPOINT_TOOFAR3 245, acore_string row 245
		}
		// No blanket wpguid reset: C++ never touches waypoint_data.wpguid
		// in `on` (cs_wp.cpp:820-874) — each re-spawned point overwrites
		// its own row's wpguid, and a mid-loop spawn failure leaves the
		// stale values behind.
		pts, err := wdb.QueryContext(ctx, "SELECT point, position_x, position_y, position_z, orientation FROM waypoint_data WHERE id = ? ORDER BY point", pathID)
		if err != nil {
			return true
		}
		defer pts.Close()
		displayID := s.wpSelectedDisplayID()
		for pts.Next() {
			var point uint32
			var x, y, z, o float64
			if pts.Scan(&point, &x, &y, &z, &o) != nil {
				continue
			}
			// A failed Create/LoadFromDB reports LANG_WAYPOINT_VP_NOTCREATED
			// (228) and aborts the whole command (cs_wp.cpp:858-867); the
			// mimic level is min(point, STRONG_MAX_LEVEL 255).
			level := point
			if level > 255 {
				level = 255
			}
			vg := s.wpSpawnVisual(ctx, x, y, z, o, displayID, level, true)
			if vg == 0 {
				// Create/LoadFromDB failure reports LANG_WAYPOINT_VP_NOTCREATED
				// WITHOUT SetSentErrorMessage (cs_wp.cpp:858-867): message + help.
				s.sendSysMessage("Could not create visual waypoint with creatureID: 1") // LANG_WAYPOINT_VP_NOTCREATED 228, acore_string row 228
				return false
			}
			_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET wpguid = ? WHERE id = ? AND point = ?", vg, pathID, point)
		}
		s.sendSysMessage("Showing the current creature's path.")
	case "first", "last":
		if show == "first" {
			s.sendSysMessage(fmt.Sprintf("DEBUG: wp first, GUID: %d", pathID))
		} else {
			s.sendSysMessage(fmt.Sprintf("DEBUG: wp last, PathID: %d", pathID))
		}
		var x, y, z, o float64
		var qerr error
		if show == "first" {
			// C++ WORLD_SEL_WAYPOINT_DATA_POS_FIRST_BY_ID requires point 1
			// (WorldDatabase.cpp:57); no point-1 row errors instead of
			// showing the lowest existing point.
			qerr = wdb.QueryRowContext(ctx, "SELECT position_x, position_y, position_z, orientation FROM waypoint_data WHERE point = 1 AND id = ?", pathID).Scan(&x, &y, &z, &o)
		} else {
			qerr = wdb.QueryRowContext(ctx, "SELECT position_x, position_y, position_z, orientation FROM waypoint_data WHERE id = ? ORDER BY point DESC LIMIT 1", pathID).Scan(&x, &y, &z, &o)
		}
		if qerr != nil {
			// LANG_WAYPOINT_NOTFOUND 220 (first) / NOTFOUNDLAST 221 (last),
			// acore_string rows 220/221; C++ passes the path id (cs_wp.cpp:928,979).
			if show == "first" {
				s.sendSysMessage(fmt.Sprintf("Creature (GUID: %d) No waypoint found.", pathID))
			} else {
				s.sendSysMessage(fmt.Sprintf("Creature (GUID: %d) Last waypoint not found.", pathID))
			}
			return true
		}
		// Create/LoadFromDB failure aborts with LANG_WAYPOINT_VP_NOTCREATED
		// (228, first) or LANG_WAYPOINT_NOTCREATED (230, last)
		// (cs_wp.cpp:909-917, 955-963); the mimic is display + half scale,
		// no level (unlike `on`).
		if s.wpSpawnVisual(ctx, x, y, z, o, s.wpSelectedDisplayID(), 0, false) == 0 {
			// Create/LoadFromDB failure reports 228 (first) / 230 (last)
			// WITHOUT SetSentErrorMessage (cs_wp.cpp:909-917, 955-963):
			// message + help.
			if show == "first" {
				s.sendSysMessage("Could not create visual waypoint with creatureID: 1") // LANG_WAYPOINT_VP_NOTCREATED 228, acore_string row 228
			} else {
				s.sendSysMessage("Could not create waypoint-creature with ID: 1") // LANG_WAYPOINT_NOTCREATED 230, acore_string row 230
			}
			return false
		}
	case "off":
		rows, err := wdb.QueryContext(ctx, "SELECT guid FROM creature WHERE id = 1")
		if err != nil {
			// C++ sends LANG_WAYPOINT_VP_NOTFOUND (227) whenever the query
			// returns no rows (cs_wp.cpp:1024-1029), error included.
			s.sendSysMessage("No visual waypoints found") // LANG_WAYPOINT_VP_NOTFOUND 227, acore_string row 227
			return true
		}
		var guids []uint32
		for rows.Next() {
			var g uint32
			if rows.Scan(&g) == nil {
				guids = append(guids, g)
			}
		}
		rows.Close()
		if len(guids) == 0 {
			s.sendSysMessage("No visual waypoints found") // LANG_WAYPOINT_VP_NOTFOUND 227, acore_string row 227
			return true
		}
		// Per-guid DeleteFromDB (cs_wp.cpp:973-1013): a failure reports
		// LANG_WAYPOINT_NOTREMOVED (242) for that guid, and any failure
		// sends the TOOFAR trio (243-245) once; the wpguid reset and the
		// ALLREMOVED message still happen.
		hasError := false
		for _, g := range guids {
			if !s.deleteCreatureFromDB(ctx, g) {
				s.sendSysMessage(fmt.Sprintf("Warning: Could not delete WP from the world with ID: %d", g)) // LANG_WAYPOINT_NOTREMOVED 242, acore_string row 242
				hasError = true
			}
		}
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET wpguid = 0")
		if hasError {
			s.sendSysMessage("This happens if the waypoint is too far away from your char.")      // LANG_WAYPOINT_TOOFAR1 243, acore_string row 243
			s.sendSysMessage("The WP is deleted from the database, but not from the world here.") // LANG_WAYPOINT_TOOFAR2 244, acore_string row 244
			s.sendSysMessage("They will disappear after a server restart.")                       // LANG_WAYPOINT_TOOFAR3 245, acore_string row 245
		}
		s.sendSysMessage("All visual waypoints removed") // LANG_WAYPOINT_VP_ALLREMOVED 229, acore_string row 229
	default:
		s.sendSysMessage("DEBUG: wpshow - no valid command found")
	}
	return true
}
