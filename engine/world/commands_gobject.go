// Package world: gobject command script.
//
// This file ports AddSC_gobject_commandscript (cs_gobject.cpp, SIXTEENTH of the
// 39 Commands groups in cs_script_loader.cpp: decl 34 / call 79). The whole
// 630-line file is one `gobject_commandscript` class whose table registers 13
// entries: activate, delete, info, move, near, target, turn, spawngroup,
// despawngroup (both owned by HandleNpcSpawnGroup/HandleNpcDespawnGroup in
// cs_npc.cpp:1356/1397 but registered here too), "add temp", "add",
// "set phase", "set state". The old handleCmdGObject stub in commands.go
// ("GameObject command %s accepted.") is replaced by this sub-dispatcher.
//
// LANG texts are inlined from TDB enUS (the tree carries no trinity_string
// seed); the LANG id is cited on each message.
package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleCmdGObject mirrors gobject_commandscript::GetCommands (cs_gobject.cpp:58):
// per-arm RBAC gates (RBAC.h:256-267, 725-726) and Trinity per-level prefix
// matching, including the two-level "add temp" / "set phase" / "set state"
// nested commands.
func (s *session) handleCmdGObject(ctx context.Context, args []string) {
	const syntax = "Syntax: .gobject activate <guid> | .gobject add <entry> [spawntime] | .gobject add temp <entry> [spawntime] | .gobject delete <guid> | .gobject info [guid] <entry|guid> | .gobject move <guid> [x y z] | .gobject near [dist] | .gobject target [entry|name] | .gobject turn <guid> [oz] [oy] [ox] | .gobject spawngroup <groupId> [force] [ignorerespawn] | .gobject despawngroup <groupId> [removerespawntime] | .gobject set phase <guid> <phaseMask> | .gobject set state <guid> <type> [state]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	deny := func(perm uint32) bool {
		if !s.commandAllowed(ctx, perm) {
			s.sendNotification("You do not have permission to use that command.")
			return true
		}
		return false
	}
	needArgs := func(n int) bool {
		if len(args) < n+1 {
			s.sendSysMessage(syntax)
			return false
		}
		return true
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "activate":
		if deny(permissionCommandGObjectActivate) || !needArgs(1) {
			return
		}
		s.handleGObjectActivate(ctx, args[1:])
	case "delete":
		if deny(permissionCommandGObjectDelete) || !needArgs(1) {
			return
		}
		s.handleGObjectDelete(ctx, args[1:])
	case "info":
		if deny(permissionCommandGObjectInfo) || !needArgs(1) {
			return
		}
		s.handleGObjectInfo(ctx, args[1:])
	case "move":
		if deny(permissionCommandGObjectMove) || !needArgs(1) {
			return
		}
		s.handleGObjectMove(ctx, args[1:])
	case "near":
		if deny(permissionCommandGObjectNear) {
			return
		}
		s.handleGObjectNear(ctx, args[1:])
	case "target":
		if deny(permissionCommandGObjectTarget) {
			return
		}
		s.handleGObjectTarget(ctx, args[1:])
	case "turn":
		if deny(permissionCommandGObjectTurn) || !needArgs(1) {
			return
		}
		s.handleGObjectTurn(ctx, args[1:])
	case "spawngroup":
		if deny(permissionCommandGObjectSpawnGroup) || !needArgs(1) {
			return
		}
		s.sendSysMessage("gobject spawngroup is not supported: the spawn-group manager has no Go bridge.")
	case "despawngroup":
		if deny(permissionCommandGObjectDespawnGroup) || !needArgs(1) {
			return
		}
		s.sendSysMessage("gobject despawngroup is not supported: the spawn-group manager has no Go bridge.")
	case "add":
		if len(args) > 1 && strings.HasPrefix("temp", strings.ToLower(args[1])) {
			if deny(permissionCommandGObjectAddTemp) {
				return
			}
			s.handleGObjectAddTemp(ctx, args[2:])
			return
		}
		if deny(permissionCommandGObjectAdd) || !needArgs(1) {
			return
		}
		s.handleGObjectAdd(ctx, args[1:])
	case "set":
		if !needArgs(2) {
			return
		}
		switch lvl := strings.ToLower(args[1]); {
		case strings.HasPrefix("phase", lvl):
			if deny(permissionCommandGObjectSetPhase) {
				return
			}
			s.handleGObjectSetPhase(ctx, args[2:])
		case strings.HasPrefix("state", lvl):
			if deny(permissionCommandGObjectSetState) {
				return
			}
			s.handleGObjectSetState(ctx, args[2:])
		default:
			s.sendSysMessage(syntax)
		}
	default:
		s.sendSysMessage(syntax)
	}
}

// gobjectSpawnRow is the DB-facing subset of the gameobject table columns the
// gobject arms read or write.
type gobjectSpawnRow struct {
	guid       uint32
	entry      uint32
	mapID      uint32
	x, y, z    float32
	orientation float32
	rot0, rot1, rot2, rot3 float32
	phaseMask  uint32
}

// gobjectSpawnByGUID mirrors ChatHandler::GetObjectFromPlayerMapByDbGuid for
// the command layer: it loads the spawn row and rejects anything not on the
// player's map (the C++ lookup returns null for cross-map spawns).
func (s *session) gobjectSpawnByGUID(ctx context.Context, db *sql.DB, spawnID uint32) (*gobjectSpawnRow, bool) {
	var row gobjectSpawnRow
	err := db.QueryRowContext(ctx, `SELECT guid, id, map, position_x, position_y, position_z, orientation, rotation0, rotation1, rotation2, rotation3, phaseMask FROM gameobject WHERE guid = ?`, spawnID).
		Scan(&row.guid, &row.entry, &row.mapID, &row.x, &row.y, &row.z, &row.orientation, &row.rot0, &row.rot1, &row.rot2, &row.rot3, &row.phaseMask)
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) could not be found.", spawnID)) // LANG_COMMAND_OBJNOTFOUND (273)
		return nil, false
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return nil, false
	}
	if row.mapID != s.player.Map {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) could not be found.", spawnID)) // LANG_COMMAND_OBJNOTFOUND (273)
		return nil, false
	}
	return &row, true
}

// gobjectTemplateRow mirrors the gameobject_template columns the arms need.
type gobjectTemplateRow struct {
	entry     uint32
	goType    uint8
	displayID uint32
	name      string
	size      float32
	aiName    string
	script    string
	data1     uint32
}

func gobjectTemplateByEntry(ctx context.Context, db *sql.DB, entry uint32) (*gobjectTemplateRow, error) {
	var t gobjectTemplateRow
	err := db.QueryRowContext(ctx, `SELECT entry, type, displayId, name, size, COALESCE(AIName, ''), COALESCE(ScriptName, ''), COALESCE(data1, 0) FROM gameobject_template WHERE entry = ?`, entry).
		Scan(&t.entry, &t.goType, &t.displayID, &t.name, &t.size, &t.aiName, &t.script, &t.data1)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// parseGObjectEntry mirrors the GameObjectEntry variant (cs_gobject.cpp:51):
// a hyperlink |Hgameobject_entry:id| or a bare entry id.
func parseGObjectEntry(arg string) (uint32, bool) {
	if strings.HasPrefix(arg, "|Hgameobject_entry:") {
		rest := strings.TrimPrefix(arg, "|Hgameobject_entry:")
		if i := strings.Index(rest, "|"); i >= 0 {
			rest = rest[:i]
		}
		if v, err := strconv.ParseUint(rest, 10, 32); err == nil {
			return uint32(v), true
		}
		return 0, false
	}
	v, err := strconv.ParseUint(arg, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// gobjectQuatFromEulerZYX mirrors QuaternionData::fromEulerAnglesZYX
// (cs_gobject.cpp:139, 165).
func gobjectQuatFromEulerZYX(yaw, pitch, roll float64) (x, y, z, w float32) {
	cy, sy := math.Cos(yaw/2), math.Sin(yaw/2)
	cp, sp := math.Cos(pitch/2), math.Sin(pitch/2)
	cr, sr := math.Cos(roll/2), math.Sin(roll/2)
	return float32(sr*cp*cy - cr*sp*sy),
		float32(cr*sp*cy + sr*cp*sy),
		float32(cr*cp*sy - sr*sp*cy),
		float32(cr*cp*cy + sr*sp*sy)
}

// gobjectEulerFromQuat mirrors QuaternionData::toEulerAnglesZYX
// (cs_gobject.cpp:565).
func gobjectEulerFromQuat(x, y, z, w float32) (yaw, pitch, roll float32) {
	fx, fy, fz, fw := float64(x), float64(y), float64(z), float64(w)
	yaw = float32(math.Atan2(2*(fw*fz+fx*fy), 1-2*(fy*fy+fz*fz)))
	sinp := 2 * (fw*fy - fz*fx)
	if sinp > 1 {
		sinp = 1
	} else if sinp < -1 {
		sinp = -1
	}
	pitch = float32(math.Asin(sinp))
	roll = float32(math.Atan2(2*(fw*fx+fy*fz), 1-2*(fx*fx+fy*fy)))
	return yaw, pitch, roll
}

// gobjectRefreshSpawn mirrors the C++ Delete()+LoadFromDB() respawn trick
// (cs_gobject.cpp:351-361, 396-406): the 3.3.5a client caches deleted objects,
// so the object is despawned and re-broadcast as a fresh create block.
func (s *session) gobjectRefreshSpawn(ctx context.Context, db *sql.DB, spawnID uint32) {
	row, ok := s.gobjectSpawnByGUID(ctx, db, spawnID)
	if !ok {
		return
	}
	packed := gameObjectGUID(row.guid, row.entry)
	s.server.broadcastGameObjectDespawn(row.mapID, packed)
	tmpl, err := gobjectTemplateByEntry(ctx, db, row.entry)
	if err != nil {
		return
	}
	spawn := gameObjectSpawn{
		GUID:        row.guid,
		Entry:       row.entry,
		Map:         row.mapID,
		X:           row.x,
		Y:           row.y,
		Z:           row.z,
		Orientation: row.orientation,
		RotationX:   row.rot0,
		RotationY:   row.rot1,
		RotationZ:   row.rot2,
		RotationW:   row.rot3,
		State:       GameObjectStateReady,
		Type:        tmpl.goType,
		DisplayID:   tmpl.displayID,
		Size:        tmpl.size,
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.server.broadcastToMap(row.mapID, packet.Opcode, packet.Payload.Bytes())
	}
}

// handleGObjectActivate mirrors HandleGameObjectActivateCommand
// (cs_gobject.cpp:84): the spawn must exist on the player's map
// (LANG_COMMAND_OBJNOTFOUND = 273 otherwise). The live object is materialized
// through the dynamic-state registry (the same path handleGameObjectUse takes
// for player clicks) and flipped to GO_READY.
// Fidelity gap: GameObject::UseDoorOrButton (door auto-close timing from the
// template, button press/loot-window packets) has no Go bridge beyond the
// state flip pushed by broadcastGameObjectResetState.
func (s *session) handleGObjectActivate(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	dyn, err := s.server.getOrLoadGameObjectState(ctx, gameObjectGUID(row.guid, row.entry), row.guid, row.entry, row.mapID, s.player.InstanceID)
	if err != nil || dyn == nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) could not be found.", row.guid)) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	s.server.setGameObjectState(dyn.GUID, GameObjectStateReady)
	s.server.broadcastGameObjectResetState(row.mapID, dyn.GUID)
	s.sendSysMessage("Object activated!")
}

// handleGObjectDelete mirrors HandleGameObjectDeleteCommand
// (cs_gobject.cpp:301): an owner-attached live spawn is despawned first
// (the C++ owner->RemoveGameObject path; a non-player owner blocks the delete
// with LANG_COMMAND_DELOBJREFERCREATURE = 274), then GameObject::DeleteFromDB
// (LANG_COMMAND_DELOBJMESSAGE = 275, LANG_COMMAND_OBJNOTFOUND = 273).
func (s *session) handleGObjectDelete(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	packed := gameObjectGUID(row.guid, row.entry)
	if dyn := s.server.gameObjectState(row.mapID, s.player.InstanceID, packed); dyn != nil && dyn.OwnerGUID != 0 {
		if dyn.OwnerGUID != s.playerGUID {
			s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) is in use by another object (GUID low %d).", row.guid, uint32(dyn.OwnerGUID))) // LANG_COMMAND_DELOBJREFERCREATURE (274)
			return
		}
		s.server.despawnDynamicGameObject(packed)
	}
	s.server.broadcastGameObjectDespawn(row.mapID, packed)
	res, err := db.ExecContext(ctx, "DELETE FROM gameobject WHERE guid = ?", row.guid)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject delete error: %v", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) could not be found.", row.guid)) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %d) removed", row.guid)) // LANG_COMMAND_DELOBJMESSAGE (275)
}

// handleGObjectInfo mirrors HandleGameObjectInfoCommand (cs_gobject.cpp:454):
// "info [guid] <entry|guid>" — the bare-number form is an entry (the C++
// Variant resolves a bare uint32 to the entry alternative); the guid form
// loads the spawn row for location/rotation lines.
func (s *session) handleGObjectInfo(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	var entry uint32
	var spawn *gobjectSpawnRow
	rest := args
	if strings.HasPrefix("guid", strings.ToLower(args[0])) {
		if len(args) < 2 {
			s.sendSysMessage("Syntax: .gobject info [guid] <entry|guid>")
			return
		}
		spawnID, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[1])) // LANG_COMMAND_OBJNOTFOUND (273)
			return
		}
		var ok bool
		spawn, ok = s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
		if !ok {
			return
		}
		entry = spawn.entry
		rest = args[2:]
		_ = rest
	} else {
		var ok bool
		entry, ok = parseGObjectEntry(args[0])
		if !ok || entry == 0 {
			s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %s) does not exist.", args[0])) // LANG_GAMEOBJECT_NOT_EXIST (522)
			return
		}
	}
	tmpl, err := gobjectTemplateByEntry(ctx, db, entry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %d) does not exist.", entry)) // LANG_GAMEOBJECT_NOT_EXIST (522)
			return
		}
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return
	}
	lootID := uint32(0)
	if tmpl.goType == GameObjectTypeChest || tmpl.goType == GameObjectTypeFishingHole {
		lootID = tmpl.data1
	}
	if spawn != nil {
		s.sendSysMessage(fmt.Sprintf("SpawnID: %d Position: %f %f %f", spawn.guid, spawn.x, spawn.y, spawn.z)) // LANG_SPAWNINFO_SPAWNID_LOCATION (5073)
		yaw, pitch, roll := gobjectEulerFromQuat(spawn.rot0, spawn.rot1, spawn.rot2, spawn.rot3)
		s.sendSysMessage(fmt.Sprintf("Rotation: %f %f %f", yaw, pitch, roll)) // LANG_SPAWNINFO_ROTATION (5074)
		var flags, faction int64
		if err := db.QueryRowContext(ctx, "SELECT COALESCE(flags, 0), COALESCE(faction, 0) FROM gameobject_template_addon WHERE entry = ?", entry).Scan(&flags, &faction); err == nil && (flags != 0 || faction != 0) {
			s.sendSysMessage(fmt.Sprintf("Faction: %d Flags: %d", faction, flags)) // LANG_GOINFO_ADDON (85)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Entry: %d", tmpl.entry))        // LANG_GOINFO_ENTRY (5024)
	s.sendSysMessage(fmt.Sprintf("Type: %d", tmpl.goType))        // LANG_GOINFO_TYPE (5025)
	s.sendSysMessage(fmt.Sprintf("LootID: %d", lootID))          // LANG_GOINFO_LOOTID (5028)
	s.sendSysMessage(fmt.Sprintf("DisplayID: %d", tmpl.displayID)) // LANG_GOINFO_DISPLAYID (5026)
	s.sendSysMessage(fmt.Sprintf("Name: %s", tmpl.name))          // LANG_GOINFO_NAME (5027)
	s.sendSysMessage(fmt.Sprintf("Size: %f", tmpl.size))          // LANG_GOINFO_SIZE (84)
	s.sendSysMessage(fmt.Sprintf("AIName: %s ScriptName: %s", tmpl.aiName, tmpl.script)) // LANG_OBJECTINFO_AIINFO (5031)
	// Fidelity gaps: LANG_GOINFO_MODEL (86, GameObjectDisplayInfo.dbc has no Go
	// bridge), LANG_OBJECTINFO_AITYPE (5083, no GameObjectAI model),
	// LANG_SPAWNINFO_GROUP_ID (5070) / LANG_SPAWNINFO_COMPATIBILITY_MODE (5071)
	// (no spawn-group / respawn-compatibility model).
}

// handleGObjectMove mirrors HandleGameObjectMoveCommand (cs_gobject.cpp:372):
// explicit xyz or the player's position, then SaveToDB plus the delete/reload
// respawn trick (here: despawn + fresh create broadcast).
// Fidelity gap: MapManager::IsValidMapCoord has no Go bridge, so coordinates
// are stored as given without the C++ validity gate.
func (s *session) handleGObjectMove(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	if len(args) >= 4 {
		coords := make([]float32, 3)
		for i := 0; i < 3; i++ {
			v, err := strconv.ParseFloat(args[1+i], 32)
			if err != nil {
				s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
				return
			}
			coords[i] = float32(v)
		}
		x, y, z = coords[0], coords[1], coords[2]
	}
	if _, err := db.ExecContext(ctx, "UPDATE gameobject SET position_x = ?, position_y = ?, position_z = ? WHERE guid = ?", x, y, z, row.guid); err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject move error: %v", err))
		return
	}
	s.gobjectRefreshSpawn(ctx, db, row.guid)
	tmpl, _ := gobjectTemplateByEntry(ctx, db, row.entry)
	name := ""
	if tmpl != nil {
		name = tmpl.name
	}
	s.sendSysMessage(fmt.Sprintf("Move object (GUID: %d Name: %s Entry: %d)", row.guid, name, row.entry)) // LANG_COMMAND_MOVEOBJMESSAGE (277)
}

// handleGObjectNear mirrors HandleGameObjectNearCommand (cs_gobject.cpp:429):
// WORLD_SEL_GAMEOBJECT_NEAREST against the gameobject table ordered by
// distance, default radius 10 (LANG_GO_LIST_CHAT = 517 per row,
// LANG_COMMAND_NEAROBJMESSAGE = 581 summary).
func (s *session) handleGObjectNear(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	distance := 10.0
	if len(args) > 0 {
		v, err := strconv.ParseFloat(args[0], 64)
		if err != nil || v < 0 {
			s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
			return
		}
		distance = v
	}
	rows, err := db.QueryContext(ctx, `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, g.map, t.name,
		((g.position_x - ?) * (g.position_x - ?) + (g.position_y - ?) * (g.position_y - ?) + (g.position_z - ?) * (g.position_z - ?)) AS dist2
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.map = ? AND ((g.position_x - ?) * (g.position_x - ?) + (g.position_y - ?) * (g.position_y - ?) + (g.position_z - ?) * (g.position_z - ?)) <= ?
		ORDER BY dist2`,
		s.player.X, s.player.X, s.player.Y, s.player.Y, s.player.Z, s.player.Z,
		s.player.Map,
		s.player.X, s.player.X, s.player.Y, s.player.Y, s.player.Z, s.player.Z, distance*distance)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return
	}
	defer rows.Close()
	var count uint32
	for rows.Next() {
		var guid, entry uint32
		var x, y, z float32
		var mapID uint32
		var name string
		var dist2 float64
		if err := rows.Scan(&guid, &entry, &x, &y, &z, &mapID, &name, &dist2); err != nil {
			continue
		}
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hgameobject:%d|h[%s]|h|r [%f, %f, %f] Map: %d (%s %s)", guid, guid, name, x, y, z, mapID, "", "")) // LANG_GO_LIST_CHAT (517)
		count++
	}
	s.sendSysMessage(fmt.Sprintf("Search in %d found %d gameobjects.", int(distance), count)) // LANG_COMMAND_NEAROBJMESSAGE (581)
}

// handleGObjectTarget mirrors HandleGameObjectTargetCommand (cs_gobject.cpp:187):
// nearest spawn by entry id, by template-name substring, or (bare form) the
// nearest spawns filtered to active game events. Pool filtering is skipped —
// the Go tree has no PoolMgr bridge (documented gap) — and the live-respawn
// lines are emitted only when a dynamic state exists for the spawn.
// The detail line is LANG_GAMEOBJECT_DETAIL (524); empty results give
// LANG_COMMAND_TARGETOBJNOTFOUND (266); the event clause mirrors the C++
// eventFilter ("eventEntry IS NULL OR eventEntry IN (<active>)").
func (s *session) handleGObjectTarget(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	distExpr := "((g.position_x - ?) * (g.position_x - ?) + (g.position_y - ?) * (g.position_y - ?) + (g.position_z - ?) * (g.position_z - ?))"
	type targetRow struct {
		guid, entry uint32
		x, y, z, o  float32
		mapID       uint32
		phase       uint32
	}
	runQuery := func(query string, qargs ...any) []targetRow {
		rows, err := db.QueryContext(ctx, query, qargs...)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var out []targetRow
		for rows.Next() {
			var r targetRow
			if err := rows.Scan(&r.guid, &r.entry, &r.x, &r.y, &r.z, &r.o, &r.mapID, &r.phase); err != nil {
				continue
			}
			out = append(out, r)
		}
		return out
	}
	base := "SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, g.orientation, g.map, g.phaseMask FROM gameobject AS g "
	px, py, pz := float64(s.player.X), float64(s.player.Y), float64(s.player.Z)
	var found []targetRow
	switch {
	case len(args) == 0:
		eventClause := "AND (geg.eventEntry IS NULL"
		active := s.server.activeEventList(ctx)
		if len(active) > 0 {
			ids := make([]string, len(active))
			for i, id := range active {
				ids[i] = strconv.FormatInt(id, 10)
			}
			eventClause += " OR geg.eventEntry IN (" + strings.Join(ids, ",") + "))"
		} else {
			eventClause += ")"
		}
		found = runQuery(base+"LEFT OUTER JOIN game_event_gameobject AS geg ON geg.guid = g.guid WHERE g.map = ? "+eventClause+" ORDER BY "+distExpr+" ASC LIMIT 10",
			px, px, py, py, pz, pz, s.player.Map)
	case func() bool { _, ok := parseGObjectEntry(args[0]); return ok }():
		entry, _ := parseGObjectEntry(args[0])
		found = runQuery(base+"WHERE g.map = ? AND g.id = ? ORDER BY "+distExpr+" ASC LIMIT 1",
			px, px, py, py, pz, pz, s.player.Map, entry)
	default:
		// The C++ passes the name straight into LIKE '%<name>%', so user
		// % and _ keep their SQL wildcard meaning; a bound parameter needs
		// no EscapeString.
		found = runQuery("SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, g.orientation, g.map, g.phaseMask FROM gameobject AS g LEFT JOIN gameobject_template AS t ON t.entry = g.id WHERE g.map = ? AND t.name LIKE ? ORDER BY "+distExpr+" ASC LIMIT 1",
			s.player.Map, "%"+args[0]+"%", px, px, py, py, pz, pz)
	}
	if len(found) == 0 {
		s.sendSysMessage("Cannot find any gameobject.") // LANG_COMMAND_TARGETOBJNOTFOUND (266)
		return
	}
	r := found[0]
	tmpl, err := gobjectTemplateByEntry(ctx, db, r.entry)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %d) does not exist.", r.entry)) // LANG_GAMEOBJECT_NOT_EXIST (522)
		return
	}
	s.sendSysMessage(fmt.Sprintf("GUID: %d Name: %s SpawnId: %d Entry: %d Position: %f %f %f Map: %d Orientation: %f Phase: %d",
		r.guid, tmpl.name, r.guid, r.entry, r.x, r.y, r.z, r.mapID, r.o, r.phase)) // LANG_GAMEOBJECT_DETAIL (524)
	if dyn := s.server.gameObjectState(r.mapID, s.player.InstanceID, gameObjectGUID(r.guid, r.entry)); dyn != nil {
		s.sendSysMessage(fmt.Sprintf("Default respawn: %s Current respawn: %s",
			secsToTimeStringShort(0), secsToTimeStringShort(0))) // LANG_COMMAND_RAWPAWNTIMES (582)
	}
	// Fidelity gaps: pool membership (no PoolMgr bridge), and live respawn
	// timers (the dynamic state carries no respawn-delay model), so the
	// respawn line reports zeros when a live state exists at all.
}

// handleGObjectTurn mirrors HandleGameObjectTurnCommand (cs_gobject.cpp:333):
// oz defaults to the player's orientation, oy/ox default to 0; the rotation is
// stored via SetLocalRotationAngles semantics (Euler ZYX -> quaternion columns)
// plus SaveToDB, then the delete/reload respawn trick.
func (s *session) handleGObjectTurn(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	oz := float64(s.player.Orientation)
	oy, ox := 0.0, 0.0
	vals := []*float64{&oz, &oy, &ox}
	for i := 0; i < len(vals) && i+1 < len(args); i++ {
		v, err := strconv.ParseFloat(args[1+i], 64)
		if err != nil {
			s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
			return
		}
		*vals[i] = v
	}
	rotX, rotY, rotZ, rotW := gobjectQuatFromEulerZYX(oz, oy, ox)
	if _, err := db.ExecContext(ctx, "UPDATE gameobject SET orientation = ?, rotation0 = ?, rotation1 = ?, rotation2 = ?, rotation3 = ? WHERE guid = ?",
		float32(oz), rotX, rotY, rotZ, rotW, row.guid); err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject turn error: %v", err))
		return
	}
	s.gobjectRefreshSpawn(ctx, db, row.guid)
	tmpl, _ := gobjectTemplateByEntry(ctx, db, row.entry)
	name := ""
	if tmpl != nil {
		name = tmpl.name
	}
	s.sendSysMessage(fmt.Sprintf("Turn object (GUID: %d Name: %s Entry: %d)", row.guid, name, row.entry)) // LANG_COMMAND_TURNOBJMESSAGE (276)
}

// handleGObjectAdd mirrors HandleGameObjectAddCommand (cs_gobject.cpp:106):
// template validation (LANG_GAMEOBJECT_NOT_EXIST = 522), spawn at the
// player's position/orientation with phaseMask 1, SaveToDB, then the
// delete/reload trick so the object is live.
// Fidelity gaps: the GameObjectDisplayInfo.dbc validity check (no DBC bridge),
// the player's real phase mask for spawn (no PhaseMask on the Go player
// state), and AddGameobjectToGrid (no server-side grid model).
func (s *session) handleGObjectAdd(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	entry, ok := parseGObjectEntry(args[0])
	if !ok || entry == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %s) does not exist.", args[0])) // LANG_GAMEOBJECT_NOT_EXIST (522)
		return
	}
	tmpl, err := gobjectTemplateByEntry(ctx, db, entry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %d) does not exist.", entry)) // LANG_GAMEOBJECT_NOT_EXIST (522)
			return
		}
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return
	}
	var spawnTimeSecs int64
	if len(args) > 1 {
		v, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
			return
		}
		spawnTimeSecs = v
	}
	rotX, rotY, rotZ, rotW := gobjectQuatFromEulerZYX(float64(s.player.Orientation), 0, 0)
	res, err := db.ExecContext(ctx, `INSERT INTO gameobject (id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, rotation0, rotation1, rotation2, rotation3, spawntimesecs, animprogress, state)
		VALUES (?, ?, 1, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, 255, 1)`,
		entry, s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, rotX, rotY, rotZ, rotW, spawnTimeSecs)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject add error: %v", err))
		return
	}
	spawnID, _ := res.LastInsertId()
	s.gobjectRefreshSpawn(ctx, db, uint32(spawnID))
	s.sendSysMessage(fmt.Sprintf("Add gameobject (Entry: %d Name: %s SpawnId: %d) at (%f, %f, %f)", entry, tmpl.name, spawnID, s.player.X, s.player.Y, s.player.Z)) // LANG_GAMEOBJECT_ADD (525)
}

// handleGObjectAddTemp mirrors HandleGameObjectAddTempCommand (cs_gobject.cpp:168):
// Player::SummonGameObject via the dynamic-spawn registry, default 300s.
// Fidelity gap: the auto-despawn timer is a Go time.AfterFunc rather than the
// engine's respawn scheduler.
func (s *session) handleGObjectAddTemp(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .gobject add temp <entry> [spawntime]")
		return
	}
	entry, ok := parseGObjectEntry(args[0])
	if !ok || entry == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %s) does not exist.", args[0])) // LANG_GAMEOBJECT_NOT_EXIST (522)
		return
	}
	tmpl, err := gobjectTemplateByEntry(ctx, db, entry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.sendSysMessage(fmt.Sprintf("Gameobject (Entry: %d) does not exist.", entry)) // LANG_GAMEOBJECT_NOT_EXIST (522)
			return
		}
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return
	}
	spawnSecs := int64(300)
	if len(args) > 1 {
		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || v < 0 {
			s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
			return
		}
		spawnSecs = v
	}
	lowGUID := s.server.nextDynamicGameObjectLowGUID()
	dyn := &dynamicGameObjectState{
		GUID:         gameObjectGUID(lowGUID, entry),
		LowGUID:      lowGUID,
		Entry:        entry,
		Map:          s.player.Map,
		InstanceID:   s.player.InstanceID,
		X:            s.player.X,
		Y:            s.player.Y,
		Z:            s.player.Z,
		Orientation:  s.player.Orientation,
		State:        GameObjectStateReady,
		Type:         tmpl.goType,
		DisplayID:    tmpl.displayID,
		Size:         tmpl.size,
		ParentRotation: [4]float32{0, 0, 0, 1},
		IsRuntimeSpawn: true,
	}
	dyn.DespawnTimer = time.AfterFunc(time.Duration(spawnSecs)*time.Second, func() {
		s.server.despawnDynamicGameObject(dyn.GUID)
	})
	s.server.spawnDynamicGameObject(dyn)
}

// handleGObjectSetPhase mirrors HandleGameObjectSetPhaseCommand
// (cs_gobject.cpp:412): zero phaseMask is rejected (LANG_BAD_VALUE = 44),
// then SetPhaseMask(true) + SaveToDB (the phase is persisted and the spawn
// re-broadcast).
func (s *session) handleGObjectSetPhase(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .gobject set phase <guid> <phaseMask>")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	phaseMask, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil || phaseMask == 0 {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
		return
	}
	if _, err := db.ExecContext(ctx, "UPDATE gameobject SET phaseMask = ? WHERE guid = ?", uint32(phaseMask), row.guid); err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject set phase error: %v", err))
		return
	}
	s.gobjectRefreshSpawn(ctx, db, row.guid)
}

// handleGObjectSetState mirrors HandleGameObjectSetStateCommand
// (cs_gobject.cpp:589): -1 despawns with the despawn animation, -2 fails
// silently (C++ returns false with no message), 0-3 set the state byte,
// 4 broadcasts SMSG_GAMEOBJECT_CUSTOM_ANIM.
// Fidelity gap: GAMEOBJECT_BYTES_1 byte offsets other than the state byte
// have no Go update-fields bridge; the live state flip is pushed through the
// dynamic-state registry.
func (s *session) handleGObjectSetState(ctx context.Context, args []string) {
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .gobject set state <guid> <type> [state]")
		return
	}
	spawnID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || spawnID == 0 {
		s.sendSysMessage(fmt.Sprintf("Gameobject (GUID: %s) could not be found.", args[0])) // LANG_COMMAND_OBJNOTFOUND (273)
		return
	}
	row, ok := s.gobjectSpawnByGUID(ctx, db, uint32(spawnID))
	if !ok {
		return
	}
	objectType, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
		return
	}
	packed := gameObjectGUID(row.guid, row.entry)
	if objectType < 0 {
		if objectType == -1 {
			s.server.broadcastGameObjectDespawn(row.mapID, packed)
		}
		return
	}
	if len(args) < 3 {
		return
	}
	objectState, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil {
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
		return
	}
	if objectType == 4 {
		s.server.broadcastGameObjectCustomAnim(row.mapID, packed, uint32(objectState))
	} else if objectType < 4 {
		dyn, err := s.server.getOrLoadGameObjectState(ctx, packed, row.guid, row.entry, row.mapID, s.player.InstanceID)
		if err == nil && dyn != nil {
			s.server.setGameObjectState(packed, uint8(objectState))
			s.server.broadcastGameObjectResetState(row.mapID, packed)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Set gobject type %d state %d", objectType, objectState))
}
