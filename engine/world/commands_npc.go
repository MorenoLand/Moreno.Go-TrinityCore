package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"

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
		s.handleNPCAddFormation(ctx, rest)
	case strings.HasPrefix("item", sub):
		s.handleNPCAddItem(ctx, rest)
	case strings.HasPrefix("move", sub):
		s.handleNPCAddMove(ctx, rest)
	case strings.HasPrefix("temp", sub):
		s.handleNPCAddTemp(ctx, rest)
	default:
		// The "" table entry: ".npc add <entry>".
		s.handleNPCAdd(ctx, args)
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
func (s *session) handleNPCAddFormation(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCAddFormation) {
		return
	}
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .npc add formation $leaderGUID")
		return
	}
	leaderGUID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid leader GUID.")
		return
	}
	if uint16(s.selection>>48) != 0xF130 {
		s.sendSysMessage("Select a creature first.")
		return
	}
	memberLow := uint32(s.selection & 0x00FFFFFF)
	if memberLow == 0 {
		s.sendSysMessage("Select a creature first.")
		return
	}
	server := s.server
	if row, ok := server.formationRowFor(memberLow); ok {
		s.sendSysMessage(fmt.Sprintf("Selected creature is already member of group %d", row.leaderGUID))
		return
	}
	member := s.findCreatureMotion(s.selection)
	if member == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	followAngle := (math.Atan2(float64(s.player.Y-member.Y), float64(s.player.X-member.X)) - float64(s.player.Orientation)) * 180.0 / math.Pi
	followDist := math.Hypot(float64(s.player.X-member.X), float64(s.player.Y-member.Y))
	if _, err := server.WorldStore.DB.ExecContext(ctx, "INSERT INTO creature_formations (leaderGUID, memberGUID, dist, angle, groupAI) VALUES (?, ?, ?, ?, ?)", uint32(leaderGUID), memberLow, float32(followAngle), float32(followDist), 0); err != nil {
		s.sendSysMessage("Failed to store the formation row.")
		return
	}
	server.ensureFormationsLoaded(ctx)
	server.addFormationRow(memberLow, formationRow{leaderGUID: uint32(leaderGUID), dist: float32(followAngle), angle: float32(followDist)})
	s.sendSysMessage(fmt.Sprintf("Creature %d added to formation with leader %d", memberLow, uint32(leaderGUID)))
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
	id64, err := strconv.ParseUint(token, 10, 32)
	if err != nil || id64 == 0 {
		return 0, false, false
	}
	return uint32(id64), false, true
}

// npcVendorUintArg parses one of the optional uint32 vendor arguments
// (maxcount, incrtime, extendedcost): C++ parses Optional<uint32>, so a
// non-numeric token is a parser error, not atoi's 0.
func npcVendorUintArg(token string) (uint32, bool) {
	v, err := strconv.ParseUint(token, 10, 32)
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
func (s *session) handleNPCAddItem(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCAddItem) {
		return
	}
	const syntax = "Syntax: .npc add item <itemId|itemLink> [maxcount] [incrtime] [extendedcost]"
	if len(args) == 0 || len(args) > 4 {
		s.sendSysMessage(syntax)
		return
	}
	itemID, link, ok := npcItemTokenID(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	var maxcount, incrtime, extendedcost uint32
	for i, token := range args[1:] {
		v, ok := npcVendorUintArg(token)
		if !ok {
			s.sendSysMessage(syntax)
			return
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
		return
	}
	var itemName string
	if err := db.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemID).Scan(&itemName); err != nil {
		if link {
			s.sendSysMessage("You need to send an item.")
			return
		}
		s.sendSysMessage(fmt.Sprintf("Item %d does not exist.", itemID))
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	s.server.motionMu.Lock()
	vendorEntry := motion.Entry
	s.server.motionMu.Unlock()
	var npcFlag uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(npcflag, 0) FROM creature_template WHERE entry = ?", vendorEntry).Scan(&npcFlag); err != nil || npcFlag&unitNPCFlagVendor == 0 {
		s.sendSysMessage("You need to select a vendor.")
		return
	}
	if extendedcost != 0 {
		if s.server == nil || s.server.Data == nil {
			s.sendSysMessage(fmt.Sprintf("Extended cost %d does not exist.", extendedcost))
			return
		}
		if _, found, err := s.server.Data.ItemExtendedCost(extendedcost); err != nil || !found {
			s.sendSysMessage(fmt.Sprintf("Extended cost %d does not exist.", extendedcost))
			return
		}
	}
	if maxcount > 0 && incrtime == 0 {
		s.sendSysMessage(fmt.Sprintf("MaxCount != 0 (%d) but IncrTime == 0", maxcount))
		return
	}
	if maxcount == 0 && incrtime > 0 {
		s.sendSysMessage("MaxCount == 0 but IncrTime<>= 0")
		return
	}
	var exists int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM npc_vendor WHERE entry = ? AND item = ? AND ExtendedCost = ?", vendorEntry, itemID, extendedcost).Scan(&exists); err != nil || exists != 0 {
		s.sendSysMessage(fmt.Sprintf("Item %d is already in the list (extended cost %d).", itemID, extendedcost))
		return
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO npc_vendor (entry, item, maxcount, incrtime, extendedcost) VALUES (?, ?, ?, ?, ?)", vendorEntry, itemID, maxcount, incrtime, extendedcost); err != nil {
		s.sendSysMessage("Failed to store the vendor row.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("Item %d (%s) added to vendor list (maxcount %d, incrtime %d, extendedcost %d).", itemID, itemName, maxcount, incrtime, extendedcost))
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
// temp arm carries none (unlike the permanent add arm).
func (s *session) handleNPCAddTemp(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCAddTemp) {
		return
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
		}
	}
	if len(rest) != 1 {
		s.sendSysMessage(syntax)
		return
	}
	entry, ok := goParseLinkOrUint(rest[0], "creature_entry")
	if !ok {
		s.sendSysMessage(syntax)
		return
	}
	if _, ok := s.npcTemplateGate(ctx, entry); !ok {
		return
	}
	if s.server == nil || s.player == nil {
		return
	}
	p := s.player
	origin := tempSummonOrigin{Map: p.Map, InstanceID: p.InstanceID, GUID: p.GUID, X: p.X, Y: p.Y, Z: p.Z, O: p.Orientation, Reach: float64(p.CombatReach)}
	s.server.summonTempCreature(ctx, origin, entry, p.X, p.Y, p.Z, p.Orientation, summonType, 30000)
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
	guid, ok := goParseLinkOrUint(args[0], "creature")
	if !ok {
		s.sendSysMessage("Syntax: .npc add move <guid>")
		return
	}
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
	entry, ok := goParseLinkOrUint(args[0], "creature_entry")
	if !ok {
		s.sendSysMessage("Syntax: .npc add <entry>")
		return
	}
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

// handleNPCMove mirrors HandleNpcMoveCommand (cs_npc.cpp:601): the GUID
// form moves the spawn row to the handler's position, and the selection
// form (no GUID arg) moves the selected creature's row instead — C++
// returns false silently with neither, the file's convention reports the
// missing selection. The live legs ride the motion map: the moved
// creature's live position/home snap to the new row at full health (the
// DespawnOrUnsummon respawn-at-new-location outcome), and a
// differently-selected creature respawns home via npcMovementReset (no
// path reload: a respawn does not LoadPath). Pets, totems and temp
// summons carry no spawn id, so C++ reads GetCreatureData(0) and reports
// GUID-not-found for them.
func (s *session) handleNPCMove(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCMove) {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	const syntax = "Syntax: .npc move [guid]"
	var guid uint32
	selected := s.castCommandSelectedCreature()
	if len(args) == 0 {
		if selected == nil {
			s.sendSysMessage("Select a creature first.")
			return
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
			s.sendSysMessage(syntax)
			return
		}
	}
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
	if s.server != nil {
		if moved := s.npcLiveCreatureBySpawnGUID(guid); moved != nil {
			s.server.motionMu.Lock()
			moved.X, moved.Y, moved.Z, moved.Orientation = p.X, p.Y, p.Z, p.Orientation
			moved.HomeX, moved.HomeY, moved.HomeZ, moved.HomeO = p.X, p.Y, p.Z, p.Orientation
			healthChanged := moved.Health != moved.MaxHealth
			moved.Health = moved.MaxHealth
			mapID, instanceID, mguid, maxHealth := moved.Map, moved.InstanceID, moved.GUID, moved.MaxHealth
			s.server.motionMu.Unlock()
			if healthChanged {
				s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, mguid, map[int]uint32{unitFieldHealth: maxHealth})
			}
		}
		if selected != nil && uint32(selected.GUID&0x00FFFFFF) != guid {
			s.npcMovementReset(ctx, selected, false)
		}
	}
	s.sendSysMessage("Creature moved.") // LANG_COMMAND_CREATUREMOVED 271
}

// handleNPCDelete mirrors HandleNpcDeleteCommand (cs_npc.cpp:302): the
// GUID form deletes the spawn row (rows>0 -> deleted, else
// GUID-not-found), and the selection form (no GUID arg) resolves the
// selected creature — pet/totem -> LANG_SELECT_CREATURE, npcbot -> the
// bot message, temp summon -> UnSummon (live removal + deleted message),
// else the creature's spawn id down the DeleteFromDB path.
func (s *session) handleNPCDelete(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCDelete) {
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	const syntax = "Syntax: .npc delete [guid]"
	var guid uint32
	if len(args) == 0 {
		selected := s.castCommandSelectedCreature()
		if selected == nil || motionIsPet(selected) || (s.server != nil && s.server.isTotemGUID(selected.GUID)) {
			s.sendSysMessage("Select a creature first.")
			return
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
			return
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
			return
		}
		guid = uint32(selected.GUID & 0x00FFFFFF)
	} else {
		var ok bool
		guid, ok = goParseLinkOrUint(args[0], "creature")
		if !ok {
			s.sendSysMessage(syntax)
			return
		}
	}
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
// ordered creature scan on the handler's map, pure SQL. C++ binds
// distance^2 unconditionally, so a negative distance searches |distance|
// and prints the signed value; an unparseable token fails the C++ typed
// parse (syntax here).
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
		d, err := parseNPCFloat(args[0])
		if err != nil {
			s.sendSysMessage("Syntax: .npc near [distance]")
			return
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
