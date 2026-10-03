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
// C++ permission (RBAC.h:635-642, 8 constants 767-774 in permissions.go);
// the root permission 767 covers the bare ".wp".
//
// The Go worldserver has a real waypoint system (creaturemotion.go:
// loadWaypoints reads waypoint_data on demand; creature_addon.path_id binds a
// spawn to a path; the live motion map can be refreshed). Arms:
//
//   - `add [pathid]`: native — INSERT INTO waypoint_data at the GM's
//     position (new path = max id + 1 when no creature selected and no id).
//   - `event add|del|mod|listid`: native waypoint_scripts table ops.
//   - `load <pathid>`: native DB (creature_addon path_id upsert + creature
//     MovementType = 2) plus live motion-map refresh.
//   - `modify delay|action|action_chance|move_type|del|move`: partial —
//     needs a selected visual waypoint (entry 1); the wpguid link column may
//     be absent, so the path/point is resolved by position proximity (the C++
//     fallback query). The visual-creature respawn legs use plain creature
//     table rows.
//   - `unload`: native DB (DELETE creature_addon, MovementType = 0) plus live
//     motion-map refresh.
//   - `reload <id>`: native probe — Go reads waypoint_data on demand.
//   - `show on|off|first|last|info [pathid]`: partial-native — visual
//     waypoints are entry-1 creature rows (the npc-add pattern); `off`
//     removes all entry-1 rows.
//
// Creature selection uses the low 32 bits of s.selection (the spawn guid),
// like the C++ getSelectedCreature. LANG texts are inlined from the TDB
// enUS recall (no in-tree trinity_string seed).

// wpSelectedCreature resolves the selected creature's spawn guid and entry.
func (s *session) wpSelectedCreature(ctx context.Context) (guid, entry uint32, ok bool) {
	if s.selection == 0 || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, 0, false
	}
	guid = uint32(s.selection)
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", guid).Scan(&entry); err != nil {
		return 0, 0, false
	}
	return guid, entry, true
}

// wpRefreshMotion refreshes a live creature's motion entry after a path
// load/unload (creaturemotion.go motion map).
func (s *session) wpRefreshMotion(ctx context.Context, guid uint32, pathID uint32, moveType uint32) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var entry, mapID uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map FROM creature WHERE guid = ?", guid).Scan(&entry, &mapID); err != nil {
		return
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(mapID, 0, uint64(guid))
	if motion == nil {
		return
	}
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
func (s *session) handleCmdWp(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandWp) {
			return
		}
		s.sendSysMessage("Syntax: .wp add|event|load|modify|unload|reload|show")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("add", sub):
		s.handleWpAdd(ctx, rest)
	case strings.HasPrefix("event", sub):
		s.handleWpEvent(ctx, rest)
	case strings.HasPrefix("load", sub):
		s.handleWpLoad(ctx, rest)
	case strings.HasPrefix("modify", sub):
		s.handleWpModify(ctx, rest)
	case strings.HasPrefix("unload", sub):
		s.handleWpUnload(ctx)
	case strings.HasPrefix("reload", sub):
		s.handleWpReload(ctx, rest)
	case strings.HasPrefix("show", sub):
		s.handleWpShow(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .wp add|event|load|modify|unload|reload|show")
	}
}

// handleWpAdd mirrors HandleWpAddCommand (cs_wp.cpp:86).
func (s *session) handleWpAdd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpAdd) {
		return
	}
	if s.player == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return
	}
	var pathID uint32
	if len(args) > 0 {
		pathID = uint32(cAtoi(args[0]))
	} else if guid, _, ok := s.wpSelectedCreature(ctx); ok {
		_ = wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID)
		if pathID == 0 {
			var maxID uint32
			_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM waypoint_data").Scan(&maxID)
			pathID = maxID + 1
			s.sendSysMessage("New path started.")
		}
	} else {
		var maxID uint32
		_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM waypoint_data").Scan(&maxID)
		pathID = maxID + 1
		s.sendSysMessage("New path started.")
	}
	if pathID == 0 {
		s.sendSysMessage("Current creature haven't loaded path.")
		return
	}
	var point uint32
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(point), 0) FROM waypoint_data WHERE id = ?", pathID).Scan(&point)
	point++
	p := s.player
	if _, err := wdb.ExecContext(ctx, "INSERT INTO waypoint_data (id, point, position_x, position_y, position_z, orientation) VALUES (?, ?, ?, ?, ?, ?)",
		pathID, point, p.X, p.Y, p.Z, p.Orientation); err != nil {
		return
	}
	s.sendSysMessage(fmt.Sprintf("PathID: %d: Waypoint %d created.", pathID, point))
}

// handleWpEvent mirrors HandleWpEventCommand (cs_wp.cpp:252): add, del, mod,
// listid on waypoint_scripts.
func (s *session) handleWpEvent(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpEvent) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .wp event add|del|mod|listid")
		return
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return
	}
	show := strings.ToLower(args[0])
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
				return
			}
		}
		if _, err := wdb.ExecContext(ctx, "INSERT INTO waypoint_scripts (guid) VALUES (?)", id); err == nil {
			s.sendSysMessage(fmt.Sprintf("Wp Event: New waypoint event added: %d", id))
		}
	case "del":
		if id == 0 {
			s.sendSysMessage("ERROR: Waypoint script guid not present.")
			return
		}
		res, err := wdb.ExecContext(ctx, "DELETE FROM waypoint_scripts WHERE guid = ?", id)
		if err != nil {
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			s.sendSysMessage(fmt.Sprintf("Wp Event: Waypoint script removed: %d", id))
		} else {
			s.sendSysMessage(fmt.Sprintf("Wp Event: ERROR: you have selected a non existing script: %d", id))
		}
	case "listid":
		if id == 0 {
			s.sendSysMessage("Wp Event: You must provide waypoint script id.")
			return
		}
		rows, err := wdb.QueryContext(ctx, "SELECT guid, delay, command, datalong, datalong2, dataint, x, y, z, o FROM waypoint_scripts WHERE id = ?", id)
		if err != nil {
			return
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			var guid, delay, command, datalong, datalong2, dataint uint32
			var x, y, z, o float64
			if rows.Scan(&guid, &delay, &command, &datalong, &datalong2, &dataint, &x, &y, &z, &o) != nil {
				continue
			}
			s.sendSysMessage(fmt.Sprintf("id: %d, guid: %d, delay: %d, command: %d, datalong: %d, datalong2: %d, dataint: %d, posx: %f, posy: %f, posz: %f, orientation: %f",
				id, guid, delay, command, datalong, datalong2, dataint, x, y, z, o))
			found = true
		}
		if !found {
			s.sendSysMessage(fmt.Sprintf("Wp Event: No waypoint scripts found on id: %d", id))
		}
	case "mod":
		s.handleWpEventMod(ctx, args)
	default:
		s.sendSysMessage("Syntax: .wp event add|del|mod|listid")
	}
}

// handleWpEventMod mirrors the "mod" leg of HandleWpEventCommand.
func (s *session) handleWpEventMod(ctx context.Context, args []string) {
	wdb := s.server.WorldStore.DB
	if len(args) < 4 {
		s.sendSysMessage("Syntax: .wp event mod <id> <setid|delay|command|datalong|datalong2|dataint|posx|posy|posz|orientation> <value>")
		return
	}
	id := uint32(cAtoi(args[1]))
	if id == 0 {
		s.sendSysMessage("ERROR: No valid waypoint script id present.")
		return
	}
	field := strings.ToLower(args[2])
	columns := map[string]string{
		"setid": "id", "delay": "delay", "command": "command", "datalong": "datalong",
		"datalong2": "datalong2", "dataint": "dataint", "posx": "x", "posy": "y",
		"posz": "z", "orientation": "o",
	}
	col, ok := columns[field]
	if !ok {
		s.sendSysMessage("ERROR: No valid argument present.")
		return
	}
	var existing int
	if wdb.QueryRowContext(ctx, "SELECT id FROM waypoint_scripts WHERE guid = ?", id).Scan(&existing) != nil {
		s.sendSysMessage("ERROR: You have selected an non existing waypoint script guid.")
		return
	}
	if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_scripts SET "+col+" = ? WHERE guid = ?", args[3], id); err != nil {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Waypoint script: %d: %s updated.", id, field))
}

// handleWpLoad mirrors HandleWpLoadCommand (cs_wp.cpp:163).
func (s *session) handleWpLoad(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpLoad) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .wp load <pathid>")
		return
	}
	guid, entry, ok := s.wpSelectedCreature(ctx)
	if !ok {
		s.sendSysMessage("Select a creature.") // LANG_SELECT_CREATURE 199
		return
	}
	if entry == 1 { // VISUAL_WAYPOINT
		s.sendSysMessage("You want to load path to a waypoint? Aren't you?")
		return
	}
	pathID := uint32(cAtoi(args[0]))
	if pathID == 0 {
		s.sendSysMessage("No valid path number provided.")
		return
	}
	wdb := s.server.WorldStore.DB
	var hasAddon int
	if wdb.QueryRowContext(ctx, "SELECT 1 FROM creature_addon WHERE guid = ?", guid).Scan(&hasAddon) == nil {
		_, _ = wdb.ExecContext(ctx, "UPDATE creature_addon SET path_id = ? WHERE guid = ?", pathID, guid)
	} else {
		_, _ = wdb.ExecContext(ctx, "INSERT INTO creature_addon (guid, path_id) VALUES (?, ?)", guid, pathID)
	}
	_, _ = wdb.ExecContext(ctx, "UPDATE creature SET MovementType = 2 WHERE guid = ?", guid)
	s.wpRefreshMotion(ctx, guid, pathID, 2)
	s.sendSysMessage("Path loaded.")
}

// handleWpUnload mirrors HandleWpUnLoadCommand (cs_wp.cpp:231).
func (s *session) handleWpUnload(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandWpUnload) {
		return
	}
	guid, _, ok := s.wpSelectedCreature(ctx)
	if !ok {
		s.sendSysMessage("You must select target.")
		return
	}
	wdb := s.server.WorldStore.DB
	var pathID uint32
	hasPath := wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID) == nil && pathID != 0
	if !hasPath {
		s.sendSysMessage("Target have no loaded path.")
		return
	}
	_, _ = wdb.ExecContext(ctx, "DELETE FROM creature_addon WHERE guid = ?", guid)
	_, _ = wdb.ExecContext(ctx, "UPDATE creature SET MovementType = 0 WHERE guid = ?", guid)
	s.wpRefreshMotion(ctx, guid, 0, 0)
	s.sendSysMessage("Path unloaded.")
}

// handleWpReload mirrors HandleWpReloadCommand (cs_wp.cpp:219): Go reads
// waypoint_data on demand, so the probe is the reload.
func (s *session) handleWpReload(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpReload) {
		return
	}
	if len(args) == 0 {
		return
	}
	id := uint32(cAtoi(args[0]))
	if id == 0 {
		return
	}
	wdb := s.server.WorldStore.DB
	if wdb != nil {
		var one int
		_ = wdb.QueryRowContext(ctx, "SELECT 1 FROM waypoint_data WHERE id = ? LIMIT 1", id).Scan(&one)
	}
	s.sendSysMessage(fmt.Sprintf("Loading Path: %d", id))
}

// wpVisualPoint resolves the selected visual waypoint (entry 1) to its
// (pathid, point): by wpguid when the column exists, else by position
// proximity (the C++ fallback, cs_wp.cpp:600-625).
func (s *session) wpVisualPoint(ctx context.Context, guid uint32) (pathID, point uint32, ok bool) {
	wdb := s.server.WorldStore.DB
	if wdb.QueryRowContext(ctx, "SELECT id, point FROM waypoint_data WHERE wpguid = ?", guid).Scan(&pathID, &point) == nil {
		return pathID, point, true
	}
	var x, y, z float64
	if wdb.QueryRowContext(ctx, "SELECT position_x, position_y, position_z FROM creature WHERE guid = ?", guid).Scan(&x, &y, &z) != nil {
		return 0, 0, false
	}
	if wdb.QueryRowContext(ctx, `SELECT id, point FROM waypoint_data
		WHERE abs(position_x - ?) <= 0.01 AND abs(position_y - ?) <= 0.01 AND abs(position_z - ?) <= 0.01 LIMIT 1`,
		x, y, z).Scan(&pathID, &point) == nil {
		return pathID, point, true
	}
	return 0, 0, false
}

// handleWpModify mirrors HandleWpModifyCommand (cs_wp.cpp:541): delay,
// action, action_chance, move_type, del, move on the selected visual waypoint.
func (s *session) handleWpModify(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpModify) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .wp modify delay|action|action_chance|move_type|del|move [value]")
		return
	}
	show := strings.ToLower(args[0])
	switch show {
	case "delay", "action", "action_chance", "move_type", "del", "move":
	default:
		return
	}
	guid, entry, ok := s.wpSelectedCreature(ctx)
	if !ok || entry != 1 { // VISUAL_WAYPOINT
		s.sendSysMessage("ERROR: You must select a waypoint.")
		return
	}
	pathID, point, ok := s.wpVisualPoint(ctx, guid)
	if !ok {
		s.sendSysMessage(fmt.Sprintf("Waypoint %d not found in DB.", guid))
		return
	}
	wdb := s.server.WorldStore.DB
	switch show {
	case "del":
		_, _ = wdb.ExecContext(ctx, "DELETE FROM waypoint_data WHERE id = ? AND point = ?", pathID, point)
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET point = point - 1 WHERE id = ? AND point > ?", pathID, point)
		_, _ = wdb.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", guid)
		s.sendSysMessage("Waypoint removed.")
		return
	case "move":
		if s.player == nil {
			return
		}
		p := s.player
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE id = ? AND point = ?",
			p.X, p.Y, p.Z, p.Orientation, pathID, point)
		_, _ = wdb.ExecContext(ctx, "UPDATE creature SET position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			p.X, p.Y, p.Z, p.Orientation, guid)
		s.sendSysMessage("Waypoint changed.")
		return
	}
	if len(args) < 2 {
		s.sendSysMessage(fmt.Sprintf("Argument required for %s.", show))
		return
	}
	if _, err := wdb.ExecContext(ctx, "UPDATE waypoint_data SET "+show+" = ? WHERE id = ? AND point = ?", args[1], pathID, point); err != nil {
		return
	}
	s.sendSysMessage(fmt.Sprintf("Waypoint %s updated.", show))
}

// wpSpawnVisual spawns one visual-waypoint creature row (entry 1, the
// npc-add pattern).
func (s *session) wpSpawnVisual(ctx context.Context, x, y, z, o float64) uint32 {
	wdb := s.server.WorldStore.DB
	if wdb == nil || s.player == nil {
		return 0
	}
	var guid uint32
	if wdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid) != nil || guid == 0 {
		return 0
	}
	if _, err := wdb.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, 1, ?, 1, 1, ?, ?, ?, ?, 120, 0)",
		guid, s.player.Map, x, y, z, o); err != nil {
		return 0
	}
	return guid
}

// handleWpShow mirrors HandleWpShowCommand (cs_wp.cpp:729): on, off, first,
// last, info.
func (s *session) handleWpShow(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandWpShow) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .wp show on|off|first|last|info [pathid]")
		return
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return
	}
	show := strings.ToLower(args[0])
	var pathID uint32
	if len(args) > 1 {
		pathID = uint32(cAtoi(args[1]))
	} else if guid, _, ok := s.wpSelectedCreature(ctx); ok {
		_ = wdb.QueryRowContext(ctx, "SELECT path_id FROM creature_addon WHERE guid = ?", guid).Scan(&pathID)
	}
	switch show {
	case "info":
		guid, entry, ok := s.wpSelectedCreature(ctx)
		if !ok || entry != 1 {
			s.sendSysMessage("Select a visual waypoint.")
			return
		}
		pid, point, ok := s.wpVisualPoint(ctx, guid)
		if !ok {
			s.sendSysMessage(fmt.Sprintf("Waypoint %d not found in DB.", guid))
			return
		}
		var delay, moveType, action, actionChance uint32
		_ = wdb.QueryRowContext(ctx, "SELECT delay, move_type, action, action_chance FROM waypoint_data WHERE id = ? AND point = ?", pid, point).Scan(&delay, &moveType, &action, &actionChance)
		s.sendSysMessage(fmt.Sprintf("Show info: for current point: %d, Path ID: %d", point, pid))
		s.sendSysMessage(fmt.Sprintf("Show info: delay: %d", delay))
		s.sendSysMessage(fmt.Sprintf("Show info: Move flag: %d", moveType))
		s.sendSysMessage(fmt.Sprintf("Show info: Waypoint event: %d", action))
		s.sendSysMessage(fmt.Sprintf("Show info: Event chance: %d", actionChance))
	case "on":
		if pathID == 0 {
			s.sendSysMessage("Select a creature.")
			return
		}
		// Delete existing visuals for this path first.
		rows, err := wdb.QueryContext(ctx, "SELECT guid FROM creature WHERE id = 1")
		if err == nil {
			for rows.Next() {
				var g uint32
				if rows.Scan(&g) == nil {
					_, _ = wdb.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", g)
				}
			}
			rows.Close()
		}
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET wpguid = 0")
		pts, err := wdb.QueryContext(ctx, "SELECT point, position_x, position_y, position_z, orientation FROM waypoint_data WHERE id = ? ORDER BY point", pathID)
		if err != nil {
			return
		}
		defer pts.Close()
		shown := false
		for pts.Next() {
			var point uint32
			var x, y, z, o float64
			if pts.Scan(&point, &x, &y, &z, &o) != nil {
				continue
			}
			if vg := s.wpSpawnVisual(ctx, x, y, z, o); vg != 0 {
				_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET wpguid = ? WHERE id = ? AND point = ?", vg, pathID, point)
				shown = true
			}
		}
		if !shown {
			s.sendSysMessage("Path not found.")
			return
		}
		s.sendSysMessage("Showing the current creature's path.")
	case "first", "last":
		if pathID == 0 {
			s.sendSysMessage("Select a creature.")
			return
		}
		order := "ORDER BY point ASC"
		if show == "last" {
			order = "ORDER BY point DESC"
		}
		var x, y, z, o float64
		if wdb.QueryRowContext(ctx, "SELECT position_x, position_y, position_z, orientation FROM waypoint_data WHERE id = ? "+order+" LIMIT 1", pathID).Scan(&x, &y, &z, &o) != nil {
			s.sendSysMessage(fmt.Sprintf("Waypoint %d not found.", pathID))
			return
		}
		s.wpSpawnVisual(ctx, x, y, z, o)
	case "off":
		rows, err := wdb.QueryContext(ctx, "SELECT guid FROM creature WHERE id = 1")
		if err != nil {
			return
		}
		for rows.Next() {
			var g uint32
			if rows.Scan(&g) == nil {
				_, _ = wdb.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", g)
			}
		}
		rows.Close()
		_, _ = wdb.ExecContext(ctx, "UPDATE waypoint_data SET wpguid = 0")
		s.sendSysMessage("All visual waypoints removed.")
	default:
		s.sendSysMessage("Syntax: .wp show on|off|first|last|info [pathid]")
	}
}
