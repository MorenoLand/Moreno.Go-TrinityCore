package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// misc command ports, chunk 2: misc_commandscript (cs_misc.cpp) rows 92-125
// in C++ table order (gps through mailbox; the unaura/unbindsight rows were
// ported in chunk 1, commands_misc.go). Of these 32 arms, 28 are native or
// partially native and 4 are documented-blocked:
//
//   - `gps` is wired: cell/grid math, DBC map/zone/area names, VMap
//     tile presence, terrain height, and liquid status (cs_misc.cpp:183-302).
//     Deltas: zone coords fall back to world coords (no WorldMapArea
//     store), MMap reads 0 (no navmesh model), transport line omitted.
//   - `itemmove` is wired: IsValidPos bag-0 slot validation (Player.cpp:
//     10185) plus the SwapItem move/swap/merge legs over Go's
//     character_inventory (bag key 0 = INVENTORY_SLOT_BAG_0) via the
//     packet-path coordinate helper (cs_misc.cpp:704-722). Deltas: no
//     CanUnequipItem / CanEquipItem / CanStoreItem / CanBankItem gates, no
//     trade/loot-generated guards, no equipped-item duration packet.
//   - `linkgrave` / `neargrave` are wired: WorldSafeLocs.dbc existence,
//     the AreaTable parent-area zone gate, graveyard_zone link insert /
//     duplicate check (ObjectMgr::AddGraveyardLink), and the nearest-link
//     pick reusing closestGraveyardCandidate (cs_misc.cpp:977-1078).
//     Deltas: neargrave resolves the zone from the player's stored zone
//     (rooted, as repopAtGraveyard does) rather than the C++ map-position
//     zone lookup, and far-graveyard order follows the DB row order rather
//     than C++ multimap insertion order.
//   - `maxskill` / `setskill` are wired: the live playerSkill slice
//     (UpdateWeaponsSkillsToMaxSkillsForLevel / SetSkill, Player.cpp:
//     6087-6228) with SkillLine.dbc profession/riding gates and the Lua
//     SetSkill apply legs (persist, rewarded spells, achievement criteria,
//     bonus-aura reapply on add). Deltas: the UpdateSkillEnchantments
//     legs and the defense UpdateDefenseBonusesMod leg have no Go model.
//   - `movegens` is wired: the selected unit's live generator from Go's
//     single-active-generator model (creatureMovementType for creatures,
//     the taxi path for players) instead of C++'s MotionMaster stack
//     (cs_misc.cpp:2016-2298). Deltas: one active line, not the full
//     stack; point destinations read the snapped spline position.
//   - `mute` / `unmute` are wired: the session muteTime gate (chat.go),
//     the login-DB mutetime/mutereason/muteby columns (LOGIN_UPD_MUTE_TIME)
//     and the account_muted history insert (LOGIN_INS_ACCOUNT_MUTE) all
//     live, as does the Lua Mute precedent (lua_player.go).
//   - `mutehistory` is wired: LOGIN_SEL_ACCOUNT_MUTE_INFO over the
//     account_muted table, via the accountIDByName lookup.
//   - `possess` is wired: the GM casts possess (530, triggered) on the
//     selected unit, and the funnel's MOD_POSSESS apply arm runs
//     charmCreatureFaction with the possess leg (POSSESSED flag, client
//     control, possess bar). Delta: no selection is a silent false
//     (R13), as in C++.
//   - `unpossess` is wired: Unit::RemoveCharmAuras on the selected unit
//     (else the GM) removes the MOD_CHARM / MOD_POSSESS_PET / MOD_POSSESS /
//     AOE_CHARM auras in C++ order; the creature remove funnel runs
//     uncharmCreature plus the client-control release. Delta: a selection
//     that resolves to neither a player session nor a same-map creature
//     falls back to self.
//   - `pvpstats` is wired: with Battleground.StoreStatistics.Enable on,
//     the CHAR_SEL_PVPSTATS_FACTIONS_OVERALL query over
//     pvpstats_battlegrounds (last 7 calendar days, winner_faction ASC),
//     walking the rows C++-exact (first row -> Horde, second -> Alliance,
//     fewer than two rows -> silent). Rows are recorded by
//     recordBattlegroundStats at every BG end (creditBattlegroundWin for
//     AB/EotS/IoC/SA/WSG, endAV for AV). Deltas: LANG wordings are
//     plain-English recalls (no in-tree seed); per-player
//     pvpstats_players rows have no writer; AV wins still get no WIN_BG
//     criteria credit (pre-existing gap, untouched).
//   - `recall` is wired: Player::SaveRecallPosition snapshots the login
//     position and every non-flight .go teleport (cs_go.cpp DoTeleport),
//     and Recall teleports the target there (Player.h:1989-1990).
//     Deltas: no IsBeingTeleported gate (Go teleports are synchronous),
//     unset recall is a silent no-op.
//   - `respawn` is wired: the selected dead creature revives through the
//     due-path revive block, and the no-target leg sweeps the player's grid
//     (pending records + dead live motions, pets excluded;
//     cs_misc.cpp:1801-1841). Delta: no grid index, so the sweep matches on
//     computed grid coords rather than the C++ grid-object visit.
//   - `wchange` is bridged via Server.setZoneWeather (Weather::SetWeather,
//     Weather.cpp); the Map::SetZoneWeather zone override rides
//     Server.setZoneWeatherOverride (weather.go) and the Lua Map
//     surface (map:SetZoneWeather); the SmartScript overrideWeather
//     action dispatch itself still has no Go engine.
//   - `mailbox` is wired: SendShowMailBox with the player's own GUID,
//     the exact lua_player.go SMSG_SHOW_MAILBOX packet, no gate (as in C++).
//   - `playall` is wired: buildPlaySound (trainers.go) broadcast to every
//     in-world session (the broadcastMessageChatAll filter); the
//     SoundEntries.dbc existence gate stays unmodeled (same documented gap
//     as the spell play-sound effect), so unknown ids are sent, not rejected.
//
// Console-vs-chat LANG branches are moot (Go commands are always sessioned);
// GetNameLink/playerLink have no Go bridge, so plain names are used. LANG
// texts are inlined from TDB enUS recall (no in-tree trinity_string seed),
// per tree convention.

// Inlined enUS texts (Language.h ids; no in-tree trinity_string seed).
const (
	misc2NoSelection        = "No selection."                                  // LANG_NO_SELECTION 200
	misc2ObjectGUID         = "Object GUID: %s"                                // LANG_OBJECT_GUID 201
	misc2KickSelf           = "You cannot kick yourself."                      // LANG_COMMAND_KICKSELF 281
	misc2KickMessage        = "Player %s has been kicked."                     // LANG_COMMAND_KICKMESSAGE 282
	misc2BadValue           = "Bad value."                                     // LANG_BAD_VALUE 168
	misc2ExploreArea        = "Area explored."                                 // LANG_EXPLORE_AREA 5064
	misc2HideArea           = "Area hidden."                                   // LANG_HIDE_AREA (custom; no LANG id)
	misc2PlayerSaved        = "Player saved."                                  // LANG_PLAYER_SAVED 76
	misc2PlayersSaved       = "All players saved."                             // LANG_PLAYERS_SAVED 77
	misc2Unfreezing         = "Unfreezing %s."                                 // LANG_COMMAND_UNFREEZE 5003
	misc2NoFrozen           = "No frozen players."                             // LANG_COMMAND_NO_FROZEN_PLAYERS 5004
	misc2ListFreeze         = "Frozen players:"                                // LANG_COMMAND_LIST_FREEZE 5005
	misc2PermaFrozen        = "%s (permanent)"                                 // LANG_COMMAND_PERMA_FROZEN_PLAYER 5006
	misc2TempFrozen         = "%s (%d seconds left)"                           // LANG_COMMAND_TEMP_FROZEN_PLAYER 5019
	misc2RepairedYou        = "You have repaired %s's items."                  // LANG_YOU_REPAIR_ITEMS 11010
	misc2RepairedTarget     = "Your items have been repaired by %s."           // LANG_YOUR_ITEMS_REPAIRED 11011
	misc2GraveNoExist       = "Graveyard %d does not exist."                   // LANG_COMMAND_GRAVEYARDNOEXIST 449
	misc2GraveAlreadyLinked = "Graveyard %d is already linked to zone %d."     // LANG_COMMAND_GRAVEYARDALRLINKED 450
	misc2GraveLinked        = "Graveyard %d linked to zone %d."                // LANG_COMMAND_GRAVEYARDLINKED 451
	misc2GraveWrongZone     = "Graveyard %d cannot be linked to zone %d."      // LANG_COMMAND_GRAVEYARDWRONGZONE 452
	misc2GraveError         = "Graveyard %d has no link for this zone."        // LANG_COMMAND_GRAVEYARDERROR 454
	misc2GraveNoTeam        = "no team"                                        // LANG_COMMAND_GRAVEYARD_NOTEAM 455
	misc2GraveAny           = "any"                                            // LANG_COMMAND_GRAVEYARD_ANY 456
	misc2GraveAlliance      = "Alliance"                                       // LANG_COMMAND_GRAVEYARD_ALLIANCE 457
	misc2GraveHorde         = "Horde"                                          // LANG_COMMAND_GRAVEYARD_HORDE 458
	misc2GraveNearest       = "Nearest graveyard is %d (team %s) for zone %d." // LANG_COMMAND_GRAVEYARDNEAREST 459
	misc2ZoneNoGraveyards   = "Zone %d has no graveyards."                     // LANG_ZONENOGRAVEYARDS 460
	misc2ZoneNoGraveFaction = "Zone %d has no graveyards for faction %s."      // LANG_ZONENOGRAFACTION 461
	misc2GraveNoData        = "Graveyard data is unavailable."
)

// misc2SessionForAccount finds an online session holding the given account
// (the C++ HandleMuteCommand "find only player from same account" leg:
// FindSession(accountId) when the named character itself is offline).
func (s *session) misc2SessionForAccount(accountID uint32) *session {
	if s.server == nil || accountID == 0 {
		return nil
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for sess := range s.server.sessions {
		if sess != nil && sess.player != nil && sess.accountID == accountID {
			return sess
		}
	}
	return nil
}

// misc2AccountForGUID resolves a character GUID to its account id, or 0.
func (s *session) misc2AccountForGUID(ctx context.Context, guid uint64) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var id uint32
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&id)
	return id
}

// misc2ParseMuteArgs parses .mute arguments: [.mute <player>] <minutes>
// [reason...]. A numeric first token means the target comes from the
// selection (miscResolvePlayerTarget's no-name path); otherwise the first
// token is the player name and the second is the minutes. Status is 0 on
// success, 1 for a syntax error, 2 for an unparseable minutes value.
// reasonStart is the args index where the verbatim Tail reason begins
// (-1 when no reason tokens follow); the caller reads it through
// commandTailAfter so interior whitespace survives (cs_misc.cpp:1841).
func misc2ParseMuteArgs(args []string) (nameArgs []string, minutes uint32, reasonStart int, status int) {
	reasonStart = -1
	if len(args) == 0 {
		return nil, 0, reasonStart, 1
	}
	if mins, err := strconv.ParseUint(args[0], 10, 32); err == nil {
		if len(args) > 1 {
			reasonStart = 1
		}
		return nil, uint32(mins), reasonStart, 0
	}
	if len(args) < 2 {
		return nil, 0, reasonStart, 1
	}
	nameArgs = args[:1]
	mins, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return nil, 0, reasonStart, 2
	}
	if len(args) > 2 {
		reasonStart = 2
	}
	return nameArgs, uint32(mins), reasonStart, 0
}

// saveRecallPosition mirrors Player::SaveRecallPosition (Player.h:1989):
// WorldRelocate snapshots the player's current map and position.
func (s *session) saveRecallPosition() {
	if s == nil || s.player == nil {
		return
	}
	s.player.RecallMap, s.player.RecallX, s.player.RecallY, s.player.RecallZ, s.player.RecallO, s.player.RecallSet = s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, true
}

// gpsGridCoord mirrors Trinity::ComputeGridCoord (GridDefines.h:186):
// CENTER_GRID_ID 32, CENTER_GRID_OFFSET SIZE_OF_GRIDS/2, SIZE_OF_GRIDS
// 533.3333. The respawn sweep shares it (C++ gridId = y*64+x).
func gpsGridCoord(x, y float32) (int, int) {
	size := float64(float32(533.3333))
	gx := int((float64(x)-size/2)/size + 32 + 0.5)
	gy := int((float64(y)-size/2)/size + 32 + 0.5)
	return gx, gy
}

// gpsCellCoord mirrors Trinity::ComputeCellCoord (GridDefines.h:191) plus
// the Cell split (CellImpl.h): 512 cells per map, cell size
// SIZE_OF_GRIDS/8, center cell id 256. Returns grid and cell coords.
func gpsCellCoord(x, y float32) (gridX, gridY, cellX, cellY int) {
	cellSize := float64(float32(533.3333)) / 8
	cx := int((float64(x)-cellSize/2)/cellSize + 256 + 0.5)
	cy := int((float64(y)-cellSize/2)/cellSize + 256 + 0.5)
	return cx / 8, cy / 8, cx % 8, cy % 8
}

// gpsPhaseMask reads a motion/GO phase mask the way the totem phase helper
// does: 0 is the unset sentinel and reads as PHASEMASK_NORMAL (1).
func gpsPhaseMask(mask uint32) uint32 {
	if mask == 0 {
		return 1
	}
	return mask
}

// gpsSelectedTarget mirrors the no-args leg of HandleGPSCommand: the
// selected unit (a player session or a live creature motion), else the
// select-a-character-or-creature refusal.
func (s *session) gpsSelectedTarget() (mapID, instanceID, phase uint32, x, y, z, o float32, ok bool) {
	if s.selection != 0 {
		if sess := s.server.playerSessionForGUID(s.selection); sess != nil && sess.player != nil {
			return sess.player.Map, sess.player.InstanceID, sess.currentPlayerPhaseMask(), sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, true
		}
		if motion := s.findCreatureMotion(s.selection); motion != nil {
			return motion.Map, motion.InstanceID, gpsPhaseMask(motion.PhaseMask), motion.X, motion.Y, motion.Z, motion.Orientation, true
		}
	}
	// C++ getSelectedUnit() falls back to self (Chat.cpp:312-320): a
	// selection that resolves to no unit reports the invoker's position.
	if s.player != nil {
		return s.player.Map, s.player.InstanceID, s.currentPlayerPhaseMask(), s.player.X, s.player.Y, s.player.Z, s.player.Orientation, true
	}
	s.sendSysMessage("Select a character or creature.") // LANG_SELECT_CHAR_OR_CREATURE 1
	return 0, 0, 0, 0, 0, 0, 0, false
}

// gpsLinkTagData extracts the tag and data of a chat hyperlink: the full
// |c...|H<tag>:<data>|h...|h|r form first, then the bare |H<tag>:<data>|
// form.
func gpsLinkTagData(arg string) (string, string) {
	if info := parseSingleHyperlink(arg); info.ok {
		return info.tag, info.data
	}
	if strings.HasPrefix(arg, "|H") {
		rest := strings.TrimPrefix(arg, "|H")
		if idx := strings.IndexByte(rest, '|'); idx >= 0 {
			rest = rest[:idx]
		}
		if tag, data, found := strings.Cut(rest, ":"); found {
			return tag, data
		}
	}
	return "", ""
}

// gpsLiveGameObject mirrors ChatHandler::GetObjectFromPlayerMapByDbGuid:
// the live dynamic/instance gameobject with this spawn GUID in the GM's
// map/instance, if any.
func (s *session) gpsLiveGameObject(spawnGUID uint32) *dynamicGameObjectState {
	if s == nil || s.server == nil || s.player == nil {
		return nil
	}
	s.server.objectsMu.RLock()
	defer s.server.objectsMu.RUnlock()
	for _, dyn := range s.server.dynamicGameObjects {
		if dyn != nil && dyn.LowGUID == spawnGUID && dyn.Map == s.player.Map && dyn.InstanceID == s.player.InstanceID {
			return dyn
		}
	}
	for _, states := range s.server.instanceGameObjects {
		for _, dyn := range states {
			if dyn != nil && dyn.LowGUID == spawnGUID && dyn.Map == s.player.Map && dyn.InstanceID == s.player.InstanceID {
				return dyn
			}
		}
	}
	return nil
}

// gpsHyperlinkTarget mirrors the args leg of HandleGPSCommand
// (extractLowGuidFromLink, Chat.cpp:598): a |Hplayer:name|,
// |Hcreature:guid| or |Hgameobject:guid| link resolves to the online
// player, the live creature, or the live gameobject in the GM's
// map/instance; unknown kinds and unresolvable targets fail the way C++
// does (not-found message for a known kind, silent for a bad link).
func (s *session) gpsHyperlinkTarget(arg string) (mapID, instanceID, phase uint32, x, y, z, o float32, ok, messaged bool) {
	tag, data := gpsLinkTagData(arg)
	switch tag {
	case "player":
		name := normalizePlayerName(data)
		if name == "" {
			return 0, 0, 0, 0, 0, 0, 0, false, false
		}
		if sess := s.sessionForPlayerName(name); sess != nil && sess.player != nil {
			return sess.player.Map, sess.player.InstanceID, sess.currentPlayerPhaseMask(), sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, true, false
		}
		s.sendSysMessage(miscPlayerNotFound) // LANG_PLAYER_NOT_FOUND 499
		return 0, 0, 0, 0, 0, 0, 0, false, true
	case "creature":
		guid, err := strconv.ParseUint(data, 10, 32)
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, false, false
		}
		if motion := s.npcLiveCreatureBySpawnGUID(uint32(guid)); motion != nil {
			return motion.Map, motion.InstanceID, gpsPhaseMask(motion.PhaseMask), motion.X, motion.Y, motion.Z, motion.Orientation, true, false
		}
		s.sendSysMessage("No creature found.") // LANG_COMMAND_NOCREATUREFOUND 447 recall
		return 0, 0, 0, 0, 0, 0, 0, false, true
	case "gameobject":
		guid, err := strconv.ParseUint(data, 10, 32)
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, false, false
		}
		if dyn := s.gpsLiveGameObject(uint32(guid)); dyn != nil {
			return dyn.Map, dyn.InstanceID, gpsPhaseMask(dyn.PhaseMask), dyn.X, dyn.Y, dyn.Z, dyn.Orientation, true, false
		}
		s.sendSysMessage("No gameobject found.") // LANG_COMMAND_NOGAMEOBJECTFOUND 448 recall
		return 0, 0, 0, 0, 0, 0, 0, false, true
	}
	return 0, 0, 0, 0, 0, 0, 0, false, false
}

// handleCmdGPS mirrors HandleGPSCommand (cs_misc.cpp:183-302, RBAC 505):
// map/zone/area names come from the DBC stores, grid and cell coords from
// the GridDefines math, ground/floor Z from the terrain height model,
// VMap presence from the loaded VMap tile, and liquid status from the
// liquid model. Deltas: zone coords fall back to world coords (no
// WorldMapArea store — the C++ not-listed fallback), MMap reads 0 (no
// navmesh model), the transport line is omitted (no moTransport
// map/entry/name leg), and outdoors rides the zone area flags (the
// non-WMO fallback twin). LANG texts are TDB enUS recalls (no in-tree
// trinity_string seed), per tree convention.
func (s *session) handleCmdGPS(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandGPS) {
		return false
	}
	if s.player == nil || s.server == nil {
		return false
	}
	var mapID, instanceID, phase uint32
	var x, y, z, o float32
	if len(args) > 0 && args[0] != "" {
		var ok, messaged bool
		mapID, instanceID, phase, x, y, z, o, ok, messaged = s.gpsHyperlinkTarget(args[0])
		if !ok {
			if messaged {
				return true
			}
			return false
		}
	} else {
		var ok bool
		mapID, instanceID, phase, x, y, z, o, ok = s.gpsSelectedTarget()
		if !ok {
			return true
		}
	}
	zoneID, areaID := s.server.zoneAndAreaID(mapID, x, y, z, 0)
	mapName, zoneName, areaName := "Unknown", "Unknown", "Unknown" // LANG_UNKNOWN recall
	if s.server.Data != nil {
		if entry, found, err := s.server.Data.Map(mapID); err == nil && found {
			mapName = entry.MapName
		}
		if area, found, err := s.server.Data.Area(zoneID); err == nil && found {
			zoneName = area.Name
		}
		if area, found, err := s.server.Data.Area(areaID); err == nil && found {
			areaName = area.Name
		}
	}
	gridX, gridY := gpsGridCoord(x, y)
	cellGridX, cellGridY, cellX, cellY := gpsCellCoord(x, y)
	haveMap := uint32(0)
	if _, ok := s.server.mapGridHeight(mapID, x, y); ok {
		haveMap = 1
	}
	haveVMap := uint32(0)
	if len(s.server.terrainTile(mapID, 63-gridX, 63-gridY)) > 0 {
		haveVMap = 1
	}
	if haveVMap == 1 {
		outdoors := true
		if s.server.Data != nil {
			if area, found, err := s.server.Data.Area(zoneID); err == nil && found {
				outdoors = area.Flags&(areaFlagInside|areaFlagOutside) != areaFlagInside
			}
		}
		if outdoors {
			s.sendSysMessage("You are outdoors.") // LANG_GPS_POSITION_OUTDOORS 5042 recall
		} else {
			s.sendSysMessage("You are indoors.") // LANG_GPS_POSITION_INDOORS 5043 recall
		}
	} else {
		s.sendSysMessage("No VMap data for this position.") // LANG_GPS_NO_VMAP 5044 recall
	}
	s.sendSysMessage(fmt.Sprintf(`Map: %d (%s) Zone: %d (%s) Area: %d (%s) Phase: %d
Position: %f %f %f %f`, mapID, mapName, zoneID, zoneName, areaID, areaName, phase, x, y, z, o)) // LANG_MAP_POSITION 101 recall
	groundZ, groundOK := s.server.mapHeight(mapID, x, y, terrainMaxHeight, true, terrainDefaultHeightSearch)
	if !groundOK {
		groundZ = terrainInvalidHeight
	}
	floorZ, floorOK := s.server.mapHeight(mapID, x, y, z, true, terrainDefaultHeightSearch)
	if !floorOK {
		floorZ = terrainInvalidHeight
	}
	s.sendSysMessage(fmt.Sprintf("Grid: %d %d Cell: %d %d Instance: %d Zone: %f %f Ground: %f Floor: %f Map: %d VMap: %d MMap: %d", cellGridX, cellGridY, cellX, cellY, instanceID, x, y, groundZ, floorZ, haveMap, haveVMap, 0)) // LANG_GRID_POSITION 178 recall
	if status, data := s.server.mapLiquidStatus(mapID, x, y, z, liquidTypeAll, terrainDefaultCollisionHeight); status != liquidStatusNoWater {
		s.sendSysMessage(fmt.Sprintf("Liquid level: %f, depth: %f, entry: %d, type: %d, status: %d", data.level, data.depthLevel, data.entry, data.typeFlags, status)) // LANG_LIQUID_STATUS 175 recall
	}
	return true
}

// handleCmdGUID mirrors HandleGUIDCommand (cs_misc.cpp:680-694, RBAC 506):
// print the selected unit's GUID.
func (s *session) handleCmdGUID(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandGUID) {
		return false
	}
	if s.selection == 0 {
		s.sendSysMessage(misc2NoSelection)
		return true
	}
	s.sendSysMessage(fmt.Sprintf(misc2ObjectGUID, miscGUIDToString(s.selection)))
	return true
}

// miscGUIDToString mirrors ObjectGuid::ToString (ObjectGuid.cpp:48-58):
// "GUID Full: 0x%016x Type: <name>[ Entry: n | Pet number: n ] Low: <n>".
// The entry leg applies to entry-carrying highs (GameObject/Transport/Unit/
// Pet/Vehicle, default); the counter is the low 24 bits there and the full
// low 32 bits for Player/Item/DynamicObject/Corpse/Mo_Transport/Instance/
// Group (objectGUIDCounter, ObjectGuid.h:148-153).
func miscGUIDToString(guid uint64) string {
	high := uint16(guid >> 48)
	var typeName string
	switch high {
	case 0x4000:
		typeName = "Item"
	case 0x0000:
		typeName = "Player"
	case 0xF110:
		typeName = "Gameobject"
	case 0xF120:
		typeName = "Transport"
	case 0xF130:
		typeName = "Creature"
	case 0xF140:
		typeName = "Pet"
	case 0xF150:
		typeName = "Vehicle"
	case 0xF100:
		typeName = "DynObject"
	case 0xF101:
		typeName = "Corpse"
	case 0x1FC0:
		typeName = "MoTransport"
	case 0x1F40:
		typeName = "InstanceID"
	case 0x1F50:
		typeName = "Group"
	default:
		typeName = "<unknown>"
	}
	middle := ""
	switch high {
	case 0x0000, 0x4000, 0xF100, 0xF101, 0x1FC0, 0x1F40, 0x1F50:
		// No entry leg (ObjectGuid.h:216-234).
	default:
		entry := uint32(guid>>24) & 0xFFFFFF
		if high == 0xF140 {
			middle = fmt.Sprintf(" Pet number: %d ", entry)
		} else {
			middle = fmt.Sprintf(" Entry: %d ", entry)
		}
	}
	return fmt.Sprintf("GUID Full: 0x%016x Type: %s%s Low: %d", guid, typeName, middle, objectGUIDCounter(guid))
}

// handleCmdHelp2 mirrors HandleHelpCommand (cs_misc.cpp:695-703, RBAC 507):
// delegate to the tree's help printer (SendCommandHelpFor equivalent).
func (s *session) handleCmdHelp2(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandHelp) {
		return false
	}
	s.handleCmdHelp(ctx, args)
	return true
}

// misc2ExploredBit resolves an area id to its explored-zone bit offset and
// mask via the AreaTable DBC (the AreaTableEntry::AreaBit math in
// HandleShowAreaCommand/HandleHideAreaCommand, cs_misc.cpp:1079-1136).
func (s *session) misc2ExploredBit(areaID uint32) (offset int, mask uint32, ok bool) {
	if s.server == nil || s.server.Data == nil {
		return 0, 0, false
	}
	areaBit, _, found, err := s.server.Data.AreaTableInfo(areaID)
	if err != nil || !found || areaBit < 0 {
		return 0, 0, false
	}
	bit := uint32(areaBit)
	offset = int(bit / 32)
	if offset >= playerExploredZonesCount {
		return 0, 0, false
	}
	return offset, uint32(1) << (bit % 32), true
}

// handleCmdShowArea mirrors HandleShowAreaCommand (cs_misc.cpp:1079-1112,
// RBAC 527): set the area's explored bit on the selected player.
func (s *session) handleCmdShowArea(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandShowArea) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	areaID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return false
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		s.sendSysMessage(miscNoCharSelected)
		return true
	}
	offset, mask, ok := s.misc2ExploredBit(uint32(areaID))
	if !ok {
		s.sendSysMessage(misc2BadValue)
		return true
	}
	target.player.ExploredZones[offset] |= mask
	target.persistExploredZones(ctx)
	target.sendPlayerValuesUpdate(map[int]uint32{playerExploredZonesStart + offset: target.player.ExploredZones[offset]})
	s.sendSysMessage(misc2ExploreArea)
	return true
}

// handleCmdHideArea mirrors HandleHideAreaCommand (cs_misc.cpp:1113-1145,
// RBAC 508): XOR-toggle the area's explored bit on the selected player —
// hiding an unexplored area explores it (cs_misc.cpp:1140).
func (s *session) handleCmdHideArea(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandHideArea) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	areaID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return false
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		s.sendSysMessage(miscNoCharSelected)
		return true
	}
	offset, mask, ok := s.misc2ExploredBit(uint32(areaID))
	if !ok {
		s.sendSysMessage(misc2BadValue)
		return true
	}
	target.player.ExploredZones[offset] ^= mask
	target.persistExploredZones(ctx)
	target.sendPlayerValuesUpdate(map[int]uint32{playerExploredZonesStart + offset: target.player.ExploredZones[offset]})
	s.sendSysMessage(misc2HideArea)
	return true
}

// misc2ValidBag0Pos mirrors Player::IsValidPos (Player.cpp:10185) for
// INVENTORY_SLOT_BAG_0: equipment (0-18), bag slots (19-22), backpack
// (23-38), bank main (39-66), bank bags (67-73) and keyring (86-117) are
// valid; buyback (74-85) and currency (118-149) are not. With explicit_pos
// false, NULL_SLOT (255) is valid, as in C++.
func misc2ValidBag0Pos(slot uint8, explicit bool) bool {
	if slot == 255 && !explicit {
		return true
	}
	switch {
	case slot < 19:
		return true
	case slot >= 19 && slot < 23:
		return true
	case slot >= 23 && slot < 39:
		return true
	case slot >= 39 && slot < 67:
		return true
	case slot >= 67 && slot < 74:
		return true
	case slot >= 86 && slot < 118:
		return true
	}
	return false
}

// handleCmdItemMove mirrors HandleItemMoveCommand (cs_misc.cpp:704-722, RBAC
// 509): move or swap the invoker's own bag-0 item between two slots. Go's
// character_inventory uses bag key 0 for INVENTORY_SLOT_BAG_0, so the slot
// pair maps straight onto the packet-path coordinate helper (which also
// runs the equip-spell transition legs); the merge leg reuses
// tryMergeStacks, and the empty-destination bank-boundary quest leg mirrors
// handleSwapInvItem. C++ silences apply: equal slots, invalid slots and an
// empty source are silent no-ops, and a NULL_SLOT destination (valid only
// via the explicit_pos=false leg) matches no position leg, so it no-ops.
// Deltas: no CanUnequipItem / CanEquipItem / CanStoreItem / CanBankItem
// gates, no trade or loot-generated guards, no equipped-item duration
// packet on equip destinations.
func (s *session) handleCmdItemMove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandItemMove) {
		return false
	}
	if s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	if len(args) < 2 {
		return false
	}
	srcSlot, srcErr := strconv.ParseUint(args[0], 10, 8)
	dstSlot, dstErr := strconv.ParseUint(args[1], 10, 8)
	if srcErr != nil || dstErr != nil {
		return false
	}
	if srcSlot == dstSlot {
		return true
	}
	if !misc2ValidBag0Pos(uint8(srcSlot), true) {
		return false
	}
	if !misc2ValidBag0Pos(uint8(dstSlot), false) {
		return false
	}
	if dstSlot == 255 {
		return true
	}
	db := s.server.CharactersStore.DB
	var srcItem int64
	if err := db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, srcSlot).Scan(&srcItem); err != nil || srcItem == 0 {
		return true
	}
	if s.player.Health == 0 {
		s.sendEquipError(equipErrYouAreDead, uint64(srcItem))
		return true
	}
	var dstItem int64
	_ = db.QueryRowContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot = ? LIMIT 1", s.playerGUID, dstSlot).Scan(&dstItem)
	if dstItem != 0 {
		if s.itemIsNonemptyBag(ctx, uint64(dstItem)) &&
			!(srcSlot >= 19 && srcSlot < 23 || srcSlot >= 67 && srcSlot < 74) &&
			!(s.itemIsBag(ctx, uint64(srcItem)) && s.isBagEmpty(ctx, srcItem)) {
			s.sendEquipError(equipErrCanOnlyDoWithEmptyBags, uint64(dstItem))
			return true
		}
		if !s.itemIsBag(ctx, uint64(srcItem)) && !s.itemIsBag(ctx, uint64(dstItem)) {
			if merged, _ := s.tryMergeStacks(ctx, srcItem, 0, uint8(srcSlot), dstItem, 0, uint8(dstSlot)); merged {
				return true
			}
		}
	}
	s.swapInventoryCoordinates(ctx, srcItem, 0, int64(srcSlot), dstItem, 0, int64(dstSlot))
	if dstItem == 0 && dstSlot >= 23 {
		srcBank := srcSlot >= 39 && srcSlot < 74
		dstBank := dstSlot >= 39 && dstSlot < 74
		if srcBank || dstBank {
			var moveEntry, moveCount int64
			if db.QueryRowContext(ctx, "SELECT itemEntry, count FROM item_instance WHERE guid = ?", srcItem).Scan(&moveEntry, &moveCount) == nil && moveEntry != 0 {
				if dstBank {
					s.adjustQuestItemCount(ctx, uint32(moveEntry), uint32(moveCount), false)
				} else if srcBank {
					s.adjustQuestItemCount(ctx, uint32(moveEntry), uint32(moveCount), true)
				}
			}
		}
	}
	s.syncEquipmentCache(ctx)
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	if dstSlot < 19 {
		s.fireEquipHook(ctx, srcItem, 255, uint8(dstSlot))
	}
	return true
}

// handleCmdKickPlayer mirrors HandleKickPlayerCommand (cs_misc.cpp:880-915,
// RBAC 510): disconnect the target player. Documented fidelity gap (not a
// stub): CONFIG_SHOW_KICK_IN_WORLD has no Go config field, so the world
// broadcast leg is skipped and the direct message is always used.
func (s *session) handleCmdKickPlayer(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandKick) {
		return false
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target == nil {
		// C++ extractPlayerTarget (Chat.cpp:657-706) fails for offline
		// targets: PLAYER_NOT_FOUND, no kick message.
		s.sendSysMessage(miscPlayerNotFound)
		return true
	}
	if target == s {
		s.sendSysMessage(misc2KickSelf)
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	reason := "No reason."
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}
	_ = reason
	s.sendSysMessage(fmt.Sprintf(misc2KickMessage, miscPlayerLink(name)))
	if target != nil {
		s.kickSession(target)
	}
	return true
}

// misc2GraveTeamArg parses the optional horde/alliance team token at
// args[idx] (cs_misc.cpp linkgrave/neargrave): absent means team 0 (any),
// an unrecognized token fails silently as in C++ (return false, no message).
func misc2GraveTeamArg(args []string, idx int) (uint32, bool) {
	if len(args) <= idx {
		return 0, true
	}
	switch strings.ToLower(args[idx]) {
	case "horde":
		return teamHorde, true
	case "alliance":
		return teamAlliance, true
	}
	return 0, false
}

// misc2GraveTeamName renders a graveyard_zone link faction the way
// HandleNearGraveCommand does (LANG_COMMAND_GRAVEYARD_* 455-458).
func misc2GraveTeamName(team uint32) string {
	switch team {
	case 0:
		return misc2GraveAny
	case teamAlliance:
		return misc2GraveAlliance
	case teamHorde:
		return misc2GraveHorde
	}
	return misc2GraveNoTeam
}

// handleCmdLinkGrave mirrors HandleLinkGraveCommand (cs_misc.cpp:977-1018,
// RBAC 511): link a WorldSafeLocs graveyard to the invoker's current zone
// for a team. The C++ zone gate is kept exactly: the area must exist and be
// a top-level zone (ParentAreaID == 0). The duplicate check mirrors
// ObjectMgr::AddGraveyardLink's FindGraveyardData(id, zone) test (team is
// not part of the key), and the insert uses WORLD_INS_GRAVEYARD_ZONE's
// (ID, GhostZone, Faction) shape. Go's closestGraveyard reads
// graveyard_zone from the world DB on every call, so no in-memory store
// update is needed for the link to take effect.
func (s *session) handleCmdLinkGrave(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandLinkGrave) {
		return false
	}
	if s.player == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	if len(args) == 0 {
		return false
	}
	graveyardID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return false
	}
	team, ok := misc2GraveTeamArg(args, 1)
	if !ok {
		return false
	}
	if _, found, err := s.server.Data.WorldSafeLoc(uint32(graveyardID)); err != nil || !found {
		s.sendSysMessage(fmt.Sprintf(misc2GraveNoExist, graveyardID))
		return true
	}
	zoneID := s.player.Zone
	area, found, err := s.server.Data.Area(zoneID)
	if err != nil || !found || area.ParentAreaID != 0 {
		s.sendSysMessage(fmt.Sprintf(misc2GraveWrongZone, graveyardID, zoneID))
		return true
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage(misc2GraveNoData)
		return true
	}
	db := s.server.WorldStore.DB
	var existing int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM graveyard_zone WHERE ID = ? AND GhostZone = ?", graveyardID, zoneID).Scan(&existing)
	if existing > 0 {
		s.sendSysMessage(fmt.Sprintf(misc2GraveAlreadyLinked, graveyardID, zoneID))
		return true
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO graveyard_zone (ID, GhostZone, Faction) VALUES (?, ?, ?)", graveyardID, zoneID, team); err != nil {
		s.sendSysMessage(misc2GraveNoData)
		return true
	}
	s.sendSysMessage(fmt.Sprintf(misc2GraveLinked, graveyardID, zoneID))
	return true
}

// handleCmdListFreeze mirrors HandleListFreezeCommand (cs_misc.cpp:2473-2508,
// RBAC 512): list frozen players from the character_aura table
// (CHAR_SEL_CHARACTER_AURA_FROZEN). Documented fidelity gap (not a stub): the
// C++ re-save of online frozen players (SaveToDB to refresh remainTime) is
// skipped; remainTime is reported as stored.
func (s *session) handleCmdListFreeze(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandListFreeze) {
		return false
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		s.sendSysMessage(misc2NoFrozen)
		return true
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx,
		"SELECT c.name, ca.remainTime FROM character_aura ca JOIN characters c ON c.guid = ca.guid WHERE ca.spell = ?", freezeAuraSpellID)
	if err != nil {
		s.sendSysMessage(misc2NoFrozen)
		return true
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var name string
		var remainMs int64
		if err := rows.Scan(&name, &remainMs); err != nil {
			continue
		}
		if remainMs < 0 {
			lines = append(lines, fmt.Sprintf(misc2PermaFrozen, name))
		} else {
			lines = append(lines, fmt.Sprintf(misc2TempFrozen, name, remainMs/1000))
		}
	}
	if len(lines) == 0 {
		s.sendSysMessage(misc2NoFrozen)
		return true
	}
	s.sendSysMessage(misc2ListFreeze)
	for _, l := range lines {
		s.sendSysMessage(l)
	}
	return true
}

// misc2PersistSkill is the character_skills REPLACE twin of the Lua
// SetSkill bridge's persist closure (lua_player.go): best-effort, like
// the other command DB writes in this file.
func misc2PersistSkill(ctx context.Context, target *session, sk playerSkill) {
	if target == nil || target.player == nil || target.server == nil || target.server.CharactersStore == nil || target.server.CharactersStore.DB == nil {
		return
	}
	_, _ = target.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", target.player.GUID, sk.Skill, sk.Value, sk.Max)
}

// misc2IsProfessionOrRidingSkill is the IsProfessionOrRidingSkill twin
// (SpellMgr.h:567) over the SkillLine DBC; without a DBC store only the
// four fixed ids (129 first aid, 185 cooking, 356 fishing, 762 riding)
// are recognized, since primary professions need the category column.
func misc2IsProfessionOrRidingSkill(s *session, skillID uint32) bool {
	if s != nil && s.server != nil && s.server.Data != nil {
		if prof, err := s.server.Data.IsProfessionOrRidingSkill(skillID); err == nil {
			return prof
		}
	}
	switch skillID {
	case 129, 185, 356, 762:
		return true
	}
	return false
}

// misc2SkillIndex is the SkillStatusMap lookup twin: the slice index of
// the skill, or -1 when the player does not know it.
func misc2SkillIndex(skills []playerSkill, skillID uint32) int {
	for i := range skills {
		if uint32(skills[i].Skill) == skillID {
			return i
		}
	}
	return -1
}

// misc2SkillLineName reads SkillLine.dbc field 3 (DisplayName enUS,
// DBCStructure.h:1316), or "" when the store or record is unavailable.
func (s *session) misc2SkillLineName(skillID uint32) string {
	if s == nil || s.server == nil || s.server.Data == nil {
		return ""
	}
	file, err := s.server.Data.File("SkillLine")
	if err != nil || file == nil {
		return ""
	}
	rec, found := file.Find(skillID)
	if !found {
		return ""
	}
	name, _ := rec.String(3)
	return name
}

// misc2ParseSkillArg parses the .setskill skill token: a numeric id or a
// |Hskill:id| hyperlink (the Variant<Hyperlink<skill>, uint32> arg).
func misc2ParseSkillArg(arg string) (uint32, bool) {
	if tag, data := gpsLinkTagData(arg); tag == "skill" {
		id, err := strconv.ParseUint(data, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(id), true
	}
	id, err := strconv.ParseUint(arg, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(id), true
}

// handleCmdMaxSkill mirrors HandleMaxSkillCommand (cs_misc.cpp:1363-1377,
// RBAC 513): every known non-profession, non-riding skill whose max
// exceeds 1 takes its max as its value (Player.cpp:6087-6112), over the
// selected player or self. Documented fidelity gap (not a stub): the
// SKILL_DEFENSE (95) UpdateDefenseBonusesMod leg has no Go defense-bonus
// model (aura_skill.go delta), and no values-update broadcast is sent —
// the Lua SetSkill bridge persists the same way.
func (s *session) handleCmdMaxSkill(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMaxSkill) {
		return false
	}
	target := s.guildSelectionOrSelf()
	if target == nil || target.player == nil {
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED
		return true
	}
	for i := range target.player.Skills {
		sk := &target.player.Skills[i]
		if misc2IsProfessionOrRidingSkill(s, uint32(sk.Skill)) {
			continue
		}
		if sk.Max > 1 {
			sk.Value = sk.Max
			misc2PersistSkill(ctx, target, *sk)
		}
	}
	return true
}

// misc2MovegenChaseTarget resolves the chase generator's target the way
// creatureMovementType derives it: the script chase slot first, else the
// combat victim while in combat.
func misc2MovegenChaseTarget(motion *creatureMotion) uint64 {
	if motion == nil {
		return 0
	}
	if motion.ScriptChaseTargetGUID != 0 {
		return motion.ScriptChaseTargetGUID
	}
	if motion.InCombat && motion.TargetGUID != 0 {
		return motion.TargetGUID
	}
	return 0
}

// misc2MovegenTargetLine renders a chase/follow target line in the C++
// shape (LANG_MOVEGENS_CHASE_PLAYER / CHASE_CREATURE and the follow
// twins): the target's name and GUID counter, or the null-target line
// when the generator carries no target.
func misc2MovegenTargetLine(ctx context.Context, s *session, verb string, targetGUID uint64) string {
	if targetGUID == 0 {
		return verb + ": no target"
	}
	counter := uint32(targetGUID & 0x00FFFFFF)
	if s != nil && s.server != nil {
		if target := s.server.playerSessionForGUID(targetGUID); target != nil && target.player != nil {
			return fmt.Sprintf("%s player %s (GUID: %d)", verb, target.player.Name, counter)
		}
	}
	if motion := s.findCreatureMotion(targetGUID); motion != nil {
		return fmt.Sprintf("%s creature %s (GUID: %d)", verb, s.npcbotTemplateName(ctx, motion.Entry), counter)
	}
	return fmt.Sprintf("%s unknown target (GUID: %d)", verb, counter)
}

// handleCmdMovegens mirrors HandleMovegensCommand (cs_misc.cpp:2016-2298,
// RBAC 514): the selected unit's live movement generator. Go keeps a
// single active generator per creature (creatureMovementType), so the
// report lists that one line instead of C++'s MotionMaster stack; the
// header and the chase/follow target lines keep the C++ shape.
// Documented fidelity gaps (not stubs): the point destination reads the
// snapped spline position (Go snaps at launch, so it equals the spline
// destination), and a player reports only Idle or Flight (taxi path) —
// Go has no player chase/follow generator model.
func (s *session) handleCmdMovegens(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMovegens) {
		return false
	}
	if s.selection == 0 {
		s.sendSysMessage("Select a character or creature.") // LANG_SELECT_CHAR_OR_CREATURE
		return true
	}
	if s.server != nil {
		if target := s.server.playerSessionForGUID(s.selection); target != nil && target.player != nil {
			s.sendSysMessage(fmt.Sprintf("Movement generators for Player %d:", uint32(target.player.GUID&0x00FFFFFF)))
			if target.player.TaxiPath != "" {
				s.sendSysMessage("Flight")
			} else {
				s.sendSysMessage("Idle")
			}
			return true
		}
	}
	motion := s.findCreatureMotion(s.selection)
	if motion == nil {
		s.sendSysMessage("Select a character or creature.") // LANG_SELECT_CHAR_OR_CREATURE
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Movement generators for Creature %d:", uint32(motion.GUID&0x00FFFFFF)))
	switch genType := s.server.creatureMovementType(motion); genType {
	case movementGeneratorIdle:
		s.sendSysMessage("Idle")
	case movementGeneratorRandom:
		s.sendSysMessage("Random")
	case movementGeneratorWaypoint:
		s.sendSysMessage("Waypoint")
	case movementGeneratorConfused:
		s.sendSysMessage("Confused")
	case movementGeneratorChase:
		s.sendSysMessage(misc2MovegenTargetLine(ctx, s, "Chase", misc2MovegenChaseTarget(motion)))
	case movementGeneratorFollow:
		s.sendSysMessage(misc2MovegenTargetLine(ctx, s, "Follow", motion.ScriptFollowTargetGUID))
	case movementGeneratorHome:
		s.sendSysMessage(fmt.Sprintf("Home: %.2f, %.2f, %.2f", motion.HomeX, motion.HomeY, motion.HomeZ))
	case movementGeneratorFlight:
		s.sendSysMessage("Flight")
	case movementGeneratorPoint:
		s.sendSysMessage(fmt.Sprintf("Point: %.2f, %.2f, %.2f", motion.X, motion.Y, motion.Z))
	case movementGeneratorFleeing, movementGeneratorTimedFleeing:
		s.sendSysMessage("Fleeing")
	case movementGeneratorDistract:
		s.sendSysMessage("Distracted")
	default:
		s.sendSysMessage(fmt.Sprintf("Unknown movement generator type: %d", genType))
	}
	return true
}

// handleCmdMute mirrors HandleMuteCommand (cs_misc.cpp:1841-1915, RBAC 515):
// the online target's session muteTime takes effect at once (now +
// minutes, in seconds, matching the chat.go CanSpeak gate); an offline
// target stores the negative minutes value, which normalizeLoginMuteTime
// converts to now + minutes at next login. The login DB gets
// LOGIN_UPD_MUTE_TIME (mutetime/mutereason/muteby) plus the
// LOGIN_INS_ACCOUNT_MUTE history row. Documented fidelity gap (not a
// stub): the CONFIG_SHOW_MUTE_IN_WORLD world broadcast has no Go config
// field, so it is skipped (same convention as the kick broadcast gap).
func (s *session) handleCmdMute(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandMute) {
		return false
	}
	nameArgs, minutes, reasonStart, status := misc2ParseMuteArgs(args)
	if status == 1 {
		return false
	}
	if status == 2 {
		return false
	}
	reason := "No reason."
	if reasonStart >= 0 {
		if tail := s.commandTailAfter(args, reasonStart); tail != "" {
			reason = tail
		}
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, nameArgs)
	if !ok {
		return true
	}
	if target == nil {
		if accountID := s.misc2AccountForGUID(ctx, guid); accountID != 0 {
			target = s.misc2SessionForAccount(accountID)
		}
	}
	// C++ HasLowerSecurity(..., true) (cs_misc.cpp:1865): strong — equal
	// security also fails, no RBAC-47 bypass.
	if s.characterTargetLowerSecurityStrong(ctx, characterTarget{online: target, guid: guid, name: name}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	var accountID uint32
	if target != nil {
		accountID = target.accountID
	} else {
		accountID = s.misc2AccountForGUID(ctx, guid)
	}
	muteBy := "Console"
	if s.player != nil && s.player.Name != "" {
		muteBy = s.player.Name
	}
	now := time.Now().Unix()
	var dbMuteTime int64
	if target != nil {
		dbMuteTime = now + int64(minutes)*60
		target.muteTime = dbMuteTime
	} else {
		dbMuteTime = -int64(minutes) * 60
	}
	if db := s.authDB(); db != nil && accountID != 0 {
		_, _ = db.ExecContext(ctx, "UPDATE account SET mutetime = ?, mutereason = ?, muteby = ? WHERE id = ?", dbMuteTime, reason, muteBy, accountID)
		_, _ = db.ExecContext(ctx, "INSERT INTO account_muted VALUES (?, unixepoch(), ?, ?, ?)", accountID, minutes, muteBy, reason)
	}
	if target != nil {
		target.sendSysMessage(fmt.Sprintf("Your chat is disabled for %d minutes by %s. Reason: %s.", minutes, muteBy, reason))
		s.sendSysMessage(fmt.Sprintf("You disabled chat for %s for %d minutes. Reason: %s.", miscPlayerLink(name), minutes, reason))
	} else {
		s.sendSysMessage(fmt.Sprintf("Chat for %s will be disabled for %d minutes on next login. Reason: %s.", miscPlayerLink(name), minutes, reason))
	}
	return true
}

// handleCmdMuteHistory mirrors HandleMuteHistoryCommand (cs_misc.cpp:1965-2015,
// RBAC 632) plus HandleMuteHistoryHelper: resolve the account name (the
// C++ Utf8ToUpperOnlyLatin leg rides accountIDByName's UPPER() match),
// then print LOGIN_SEL_ACCOUNT_MUTE_INFO rows oldest first, dates in the
// C++ "%Y-%m-%d %I:%M%p" shape. A missing table or empty history both
// report no history (the table is best-effort in the Go auth DB).
func (s *session) handleCmdMuteHistory(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandMuteHistory) {
		return false
	}
	// The C++ framework consumes the whole arg string (ChatCommand.h:
	// ConsumeFromOffset returns nullopt on leftover args), so extra
	// tokens fail the parse and print help — they are never ignored.
	if len(args) != 1 || args[0] == "" {
		return false
	}
	accountID := s.accountIDByName(ctx, args[0])
	if accountID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return true
	}
	db := s.authDB()
	if db == nil {
		s.sendSysMessage(fmt.Sprintf("No mute history for account %s.", args[0]))
		return true
	}
	rows, err := db.QueryContext(ctx, "SELECT mutedate, mutetime, COALESCE(mutereason, ''), COALESCE(mutedby, '') FROM account_muted WHERE guid = ? ORDER BY mutedate ASC", accountID)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("No mute history for account %s.", args[0]))
		return true
	}
	defer rows.Close()
	type muteRow struct {
		date    int64
		minutes uint32
		reason  string
		mutedBy string
	}
	var history []muteRow
	for rows.Next() {
		var r muteRow
		if err := rows.Scan(&r.date, &r.minutes, &r.reason, &r.mutedBy); err == nil {
			history = append(history, r)
		}
	}
	if len(history) == 0 {
		s.sendSysMessage(fmt.Sprintf("No mute history for account %s.", args[0]))
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Mute history for account %s:", args[0]))
	for _, r := range history {
		s.sendSysMessage(fmt.Sprintf("%s - %d minutes - %s - %s", time.Unix(r.date, 0).Format("2006-01-02 03:04PM"), r.minutes, r.reason, r.mutedBy))
	}
	return true
}

// handleCmdNearGrave mirrors HandleNearGraveCommand (cs_misc.cpp:1019-1078,
// RBAC 516): report the nearest graveyard linked to the invoker's zone for
// a team, with the link's own faction in the report (FindGraveyardData's
// team, not the requested team). The nearest pick reuses
// closestGraveyardCandidate, the ObjectMgr::GetClosestGraveyard twin, over
// the zone's graveyard_zone links. C++ silences and branches are kept: an
// empty link set on a non-battleground map falls through to the default
// graveyard, which has no link for this zone and reports GRAVEYARDERROR
// (team 0 has no default, so it reports ZONENOGRAVEYARDS instead); a
// battleground map with no links, or links all filtered out by faction,
// reports ZONENOGRAVEYARDS (team 0) or ZONENOGRAFACTION. Deltas: the link
// set is read for the player's stored zone (rooted, as repopAtGraveyard
// does) rather than the C++ map-position zone lookup, and the far
// graveyard follows DB row order (ORDER BY ID) rather than C++ multimap
// insertion order.
func (s *session) handleCmdNearGrave(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandNearGrave) {
		return false
	}
	if s.player == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	team, ok := misc2GraveTeamArg(args, 0)
	if !ok {
		return false
	}
	zoneID := s.player.Zone
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage(misc2GraveNoData)
		return true
	}
	rootZone := s.server.rootAreaID(zoneID)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ID, Faction FROM graveyard_zone WHERE GhostZone = ? ORDER BY ID", rootZone)
	if err != nil {
		s.sendSysMessage(misc2GraveNoData)
		return true
	}
	defer rows.Close()
	type graveLink struct {
		id      uint32
		faction uint32
	}
	var links []graveLink
	for rows.Next() {
		var link graveLink
		if err := rows.Scan(&link.id, &link.faction); err != nil {
			continue
		}
		links = append(links, link)
	}
	mapEntry, hasMapEntry, _ := s.server.Data.Map(s.player.Map)
	isBG := hasMapEntry && teleIsBattlegroundOrArena(mapEntry)
	if len(links) == 0 {
		if team == 0 || isBG {
			if team == 0 {
				s.sendSysMessage(fmt.Sprintf(misc2ZoneNoGraveyards, zoneID))
			} else {
				s.sendSysMessage(fmt.Sprintf(misc2ZoneNoGraveFaction, zoneID, misc2GraveTeamName(team)))
			}
			return true
		}
		defaultID := defaultGraveyardAlliance
		if team == teamHorde {
			defaultID = defaultGraveyardHorde
		}
		s.sendSysMessage(fmt.Sprintf(misc2GraveError, defaultID))
		return true
	}
	var locations []wotlk.WorldSafeLoc
	factionByID := make(map[uint32]uint32, len(links))
	for _, link := range links {
		if link.faction != 0 && team != 0 && link.faction != team {
			continue
		}
		loc, found, err := s.server.Data.WorldSafeLoc(link.id)
		if err != nil || !found {
			continue
		}
		locations = append(locations, loc)
		factionByID[link.id] = link.faction
	}
	if len(locations) == 0 {
		if team == 0 {
			s.sendSysMessage(fmt.Sprintf(misc2ZoneNoGraveyards, zoneID))
		} else {
			s.sendSysMessage(fmt.Sprintf(misc2ZoneNoGraveFaction, zoneID, misc2GraveTeamName(team)))
		}
		return true
	}
	chosen, found := closestGraveyardCandidate(locations, s.player.Map, mapEntry, hasMapEntry, s.player.X, s.player.Y, s.player.Z)
	if !found {
		s.sendSysMessage(fmt.Sprintf(misc2ZoneNoGraveyards, zoneID))
		return true
	}
	s.sendSysMessage(fmt.Sprintf(misc2GraveNearest, chosen.ID, misc2GraveTeamName(factionByID[chosen.ID]), zoneID))
	return true
}

// handleCmdPInfo mirrors HandlePInfoCommand (cs_misc.cpp:1430-1801, RBAC
// 517): print character and account info in the C++ I-XXI order. The
// login-DB legs (ban/mute/last login/IP/OS/emails, RBAC 48 personal-data
// gate) and the mail count are live queries now; race/class/area names
// still have no DBC-name bridge, so ids are printed. Deltas: no IP-location
// country suffix (no IP-location bridge); the account-ban type prints
// "Unknown", mirroring the C++ quirk where banType is only set to
// "Character" on the character-ban fallback (cs_misc.cpp:1654-1659).
func (s *session) handleCmdPInfo(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandPInfo) {
		return false
	}
	// C++ tries atoul(args) -> GetCharacterNameByGuid first (cs_misc.cpp:1445-1450),
	// so a bare numeric GUID resolves offline targets too; a name that
	// is not a known guid falls through to the normal target resolution.
	resolveArgs := args
	if len(args) > 0 {
		if n := cAtoi(args[0]); n > 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			var guidName string
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", n).Scan(&guidName); err == nil && guidName != "" {
				resolveArgs = []string{guidName}
			}
		}
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, resolveArgs)
	if !ok {
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{online: target, guid: guid, name: name}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	online := target != nil && target.player != nil
	var accountID uint32
	var level, race, class, gender uint8
	var money, mapID, zoneID, totalTime uint32
	var alive bool
	var latency uint32
	var phase uint32
	isGM := false
	haveBase := online
	if online {
		p := target.player
		accountID = target.accountID
		level, race, class, gender = p.Level, p.Race, p.Class, p.Gender
		money = p.Money
		mapID, zoneID = p.Map, p.Zone
		totalTime = p.TotalPlayedTime
		alive = p.Health > 0
		latency = target.latency.Load()
		phase = target.currentPlayerPhaseMask()
		isGM = sessionIsGameMaster(target)
	} else if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var health, playerFlags uint32
		// CHAR_SEL_CHAR_PINFO (CharacterDatabase.cpp:412).
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT totaltime, level, money, account, race, class, map, zone, gender, health, playerFlags FROM characters WHERE guid = ?",
			guid).Scan(&totalTime, &level, &money, &accountID, &race, &class, &mapID, &zoneID, &gender, &health, &playerFlags); err != nil {
			return false // C++ returns false on no PINFO row, it does not print a level-0 row
		}
		alive = health > 0 && playerFlags&playerFlagGhost == 0
		haveBase = true
	}
	if !haveBase {
		return false
	}

	// LOGIN_SEL_PINFO (LoginDatabase.cpp:87): username, email, reg_mail,
	// last_ip, last_login, mutetime, mutereason, muteby, failed_logins,
	// locked, OS. The personal-data fields stay "Unauthorized" unless the
	// invoker holds RBAC 48 and outranks the target (cs_misc.cpp:1595-1614);
	// the DB mutetime overwrites the live session value, as in C++.
	userName, eMail, regMail, lastIP, lastLogin, osName := "error", "error", "error", "error", "error", "unknown"
	muteReason, muteBy := "No reason", "Unknown"
	var muteTime int64 = -1
	var failedLogins uint32
	var locked uint8
	if online {
		muteTime = target.muteTime
	}
	targetSecurity := s.accountSecurityLevel(ctx, accountID)
	if db := s.authDB(); db != nil && accountID != 0 {
		var dbUser, dbEmail, dbRegMail, dbLastIP, dbLastLogin, dbMuteReason, dbMuteBy, dbOS sql.NullString
		var dbMuteTime, dbFailed, dbLocked sql.NullInt64
		if err := db.QueryRowContext(ctx,
			"SELECT username, email, reg_mail, last_ip, last_login, mutetime, mutereason, muteby, failed_logins, locked, OS FROM account WHERE id = ?",
			accountID).Scan(&dbUser, &dbEmail, &dbRegMail, &dbLastIP, &dbLastLogin, &dbMuteTime, &dbMuteReason, &dbMuteBy, &dbFailed, &dbLocked, &dbOS); err == nil {
			userName = dbUser.String
			muteTime = dbMuteTime.Int64
			muteReason = dbMuteReason.String
			if muteReason == "" {
				muteReason = "No reason"
			}
			muteBy = dbMuteBy.String
			if muteBy == "" {
				muteBy = "Unknown"
			}
			failedLogins = uint32(dbFailed.Int64)
			locked = uint8(dbLocked.Int64)
			osName = dbOS.String
			if osName == "" {
				osName = "unknown"
			}
			if s.commandAllowed(ctx, permissionCommandsPInfoCheckPersonalData) && s.security >= targetSecurity {
				eMail = dbEmail.String
				regMail = dbRegMail.String
				lastIP = dbLastIP.String
				lastLogin = dbLastLogin.String
			} else {
				eMail, regMail, lastIP, lastLogin = "Unauthorized", "Unauthorized", "Unauthorized", "Unauthorized"
			}
		}
	}

	// Ban legs (cs_misc.cpp:1645-1661): account_banned first (active,
	// oldest bandate), else character_banned. banTime is 0 for a permanent
	// ban (bandate == unbandate), else the unban timestamp; -1 means no ban.
	banTime := int64(-1)
	banType, banReason, bannedBy := "Unknown", "No reason", "Unknown"
	if db := s.authDB(); db != nil && accountID != 0 {
		var unban, permanent uint32
		var by, reason string
		if err := db.QueryRowContext(ctx,
			"SELECT unbandate, bandate = unbandate, bannedby, banreason FROM account_banned WHERE id = ? AND active ORDER BY bandate ASC LIMIT 1",
			accountID).Scan(&unban, &permanent, &by, &reason); err == nil {
			if permanent != 0 {
				banTime = 0
			} else {
				banTime = int64(unban)
			}
			banReason, bannedBy = reason, by
		}
	}
	if banTime < 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var unban, permanent uint32
		var by, reason string
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT unbandate, bandate = unbandate, bannedby, banreason FROM character_banned WHERE guid = ? AND active ORDER BY bandate ASC LIMIT 1",
			guid).Scan(&unban, &permanent, &by, &reason); err == nil {
			banType = "Character"
			if permanent != 0 {
				banTime = 0
			} else {
				banTime = int64(unban)
			}
			banReason, bannedBy = reason, by
		}
	}

	// CHAR_SEL_PINFO_XP (CharacterDatabase.cpp:417): current xp plus the
	// guild-member guid (non-zero when the character is in a guild).
	var xp uint32
	var guildMemberGUID uint32
	if online {
		xp = target.player.XP
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var dbXP, dbGuildGUID uint32
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT a.xp, b.guid FROM characters a LEFT JOIN guild_member b ON a.guid = b.guid WHERE a.guid = ?",
			guid).Scan(&dbXP, &dbGuildGUID); err == nil {
			xp = dbXP
			guildMemberGUID = dbGuildGUID
		}
	}
	var guildID uint32
	var guildName, guildRank, guildNote, guildOffNote string
	var guildRankID uint8
	if guildMemberGUID != 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		// CHAR_SEL_GUILD_MEMBER_EXTENDED (CharacterDatabase.cpp:106).
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT g.guildid, g.name, gr.rname, gr.rid, gm.pnote, gm.offnote FROM guild g JOIN guild_member gm ON g.guildid = gm.guildid JOIN guild_rank gr ON g.guildid = gr.guildid AND gm.`rank` = gr.rid WHERE gm.guid = ?",
			guid).Scan(&guildID, &guildName, &guildRank, &guildRankID, &guildNote, &guildOffNote)
	}

	// Output I. LANG_PINFO_PLAYER
	offlineMark := ""
	if !online {
		offlineMark = "Offline "
	}
	s.sendSysMessage(fmt.Sprintf("Player %s%s (guid: %d)", offlineMark, miscPlayerLink(name), guid))
	// Output II. LANG_PINFO_GM_ACTIVE
	if online && isGM {
		s.sendSysMessage("** GM Mode active, Phase: -1")
	}
	// Output III. LANG_PINFO_BANNED
	if banTime >= 0 {
		banLeft := "Permanently"
		if banTime > 0 {
			if remain := banTime - time.Now().Unix(); remain > 0 {
				banLeft = secsToTimeStringShort(uint64(remain))
			} else {
				banLeft = secsToTimeStringShort(0)
			}
		}
		s.sendSysMessage(fmt.Sprintf("** Banned: (Type: %s, Reason: %s, Time: %s, By: %s)", banType, banReason, banLeft, bannedBy))
	}
	// Output IV. LANG_PINFO_MUTED
	if muteTime > 0 {
		muteLeft := secsToTimeStringShort(0)
		if remain := muteTime - time.Now().Unix(); remain > 0 {
			muteLeft = secsToTimeStringShort(uint64(remain))
		}
		s.sendSysMessage(fmt.Sprintf("** Muted: (Reason: %s, Time: %s, By: %s)", muteReason, muteLeft, muteBy))
	}
	// Output V. LANG_PINFO_ACC_ACCOUNT
	s.sendSysMessage(fmt.Sprintf("Account: %s (id: %d), GM level: %d", userName, accountID, targetSecurity))
	// Output VI. LANG_PINFO_ACC_LASTLOGIN
	s.sendSysMessage(fmt.Sprintf("Last Login: %s (Failed Logins: %d)", lastLogin, failedLogins))
	// Output VII. LANG_PINFO_ACC_OS
	s.sendSysMessage(fmt.Sprintf("Uses OS: %s - Latency: %d ms", osName, latency))
	// Output VIII. LANG_PINFO_ACC_REGMAILS
	s.sendSysMessage(fmt.Sprintf("Registration Email: %s - Email: %s", regMail, eMail))
	// Output IX. LANG_PINFO_ACC_IP
	lockedStr := "no"
	if locked != 0 {
		lockedStr = "yes"
	}
	s.sendSysMessage(fmt.Sprintf("Last IP: %s (Locked: %s)", lastIP, lockedStr))
	// Output X. LANG_PINFO_CHR_LEVEL
	maxLevel := uint32(defaultMaxPlayerLevel)
	if s.server != nil && s.server.Config.MaxPlayerLevel != 0 {
		maxLevel = s.server.Config.MaxPlayerLevel
	}
	if uint32(level) != maxLevel {
		xpTotal := uint32(0)
		if s.server != nil {
			xpTotal = s.server.xpForLevel(ctx, uint32(level))
		}
		xpLeft := uint32(0)
		if xpTotal > xp {
			xpLeft = xpTotal - xp
		}
		s.sendSysMessage(fmt.Sprintf("Level: %d (%d/%d XP (%d XP left))", level, xp, xpTotal, xpLeft))
	} else {
		s.sendSysMessage(fmt.Sprintf("Level: %d", level))
	}
	// Output XI. LANG_PINFO_CHR_RACE
	genderStr := "Male"
	if gender != 0 {
		genderStr = "Female"
	}
	s.sendSysMessage(fmt.Sprintf("Race: %s %d, Class: %d", genderStr, race, class))
	// Output XII. LANG_PINFO_CHR_ALIVE
	aliveStr := "no"
	if alive {
		aliveStr = "yes"
	}
	s.sendSysMessage(fmt.Sprintf("Alive: %s", aliveStr))
	// Output XIII. LANG_PINFO_CHR_PHASE
	if online && !isGM {
		s.sendSysMessage(fmt.Sprintf("Phase: %d", phase))
	}
	// Output XIV. LANG_PINFO_CHR_MONEY
	s.sendSysMessage(fmt.Sprintf("Money: %dg%ds%dc", money/10000, (money%10000)/100, money%100))
	// Output XV. LANG_PINFO_CHR_MAP
	s.sendSysMessage(fmt.Sprintf("Map: %d, Zone: %d", mapID, zoneID))
	// Output XVI.-XIX. guild lines
	if guildName != "" {
		s.sendSysMessage(fmt.Sprintf("Guild: %s (Id: %d)", guildName, guildID))
		s.sendSysMessage(fmt.Sprintf("Rank: %s (%d)", guildRank, guildRankID))
		if guildNote != "" {
			s.sendSysMessage(fmt.Sprintf("Note: %s", guildNote))
		}
		if guildOffNote != "" {
			s.sendSysMessage(fmt.Sprintf("O. Note: %s", guildOffNote))
		}
	}
	// Output XX. LANG_PINFO_CHR_PLAYEDTIME
	s.sendSysMessage(fmt.Sprintf("Played time: %s", secsToTimeStringShort(uint64(totalTime))))
	// Output XXI. LANG_PINFO_CHR_MAILS
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var readMail, totalMail sql.NullInt64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT SUM(CASE WHEN (checked & 1) THEN 1 ELSE 0 END), COUNT(*) FROM mail WHERE receiver = ?",
			guid).Scan(&readMail, &totalMail); err == nil && totalMail.Int64 >= 1 {
			s.sendSysMessage(fmt.Sprintf("Mails: %d Read/%d Total", readMail.Int64, totalMail.Int64))
		}
	}
	return true
}

// handleCmdPlayAll mirrors HandlePlayAllCommand (cs_misc.cpp:2509-2523, RBAC
// 518): buildPlaySound (trainers.go) broadcast to every in-world session
// (the broadcastMessageChatAll filter, matching World::SendGlobalMessage's
// player-in-world filter). Documented fidelity gap (not a stub): the
// sSoundEntriesStore existence gate has no Go bridge, so unknown sound ids
// are sent rather than rejected with LANG_SOUND_NOT_EXIST.
func (s *session) handleCmdPlayAll(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandPlayAll) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	soundID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return false
	}
	payload := buildPlaySound(uint32(soundID))
	if s.server != nil {
		s.server.sessionsMu.RLock()
		targets := make([]*session, 0, len(s.server.sessions))
		for sess := range s.server.sessions {
			if sess != nil && sess.worldReady.Load() && sess.player != nil {
				targets = append(targets, sess)
			}
		}
		s.server.sessionsMu.RUnlock()
		for _, target := range targets {
			_ = target.write(uint16(protocol.OpcodeSMSG_PLAY_SOUND), payload, true)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Played sound %d to all players.", soundID))
	return true
}

// handleCmdPossess mirrors HandlePossessCommand (cs_misc.cpp:2524-2533, RBAC
// 519): the GM casts possess (530, triggered) on the selected unit; the
// funnel's MOD_POSSESS apply arm runs charmCreatureFaction with the possess
// leg. Delta: no selection is a silent false (R13), as in C++.
func (s *session) handleCmdPossess(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandPossess) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if s.selection == 0 {
		return false
	}
	s.castSpellDirect(ctx, 530, s.selection)
	return true
}

// handleCmdPvPstats mirrors HandlePvPstatsCommand (cs_misc.cpp:130-157, RBAC
// 797): with Battleground.StoreStatistics.Enable on, the
// CHAR_SEL_PVPSTATS_FACTIONS_OVERALL query over pvpstats_battlegrounds,
// walking the rows C++-exact (first row -> Horde, second -> Alliance;
// fewer than two rows returns false silently). DATEDIFF(NOW(), date) < 7
// counts calendar days, so the filter is date(date) within the last 6
// calendar days plus today (future rows included, as in C++). LANG
// wordings are plain-English recalls (no in-tree seed).
func (s *session) handleCmdPvPstats(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandPvPstats) {
		return false
	}
	if s.server == nil || !s.server.Config.BattlegroundStoreStatisticsEnable {
		s.sendSysMessage("Battleground statistics are disabled.")
		return true
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT winner_faction, COUNT(*) AS count FROM pvpstats_battlegrounds WHERE date(date) >= date('now', '-6 days') GROUP BY winner_faction ORDER BY winner_faction ASC")
	if err != nil {
		return false
	}
	defer rows.Close()
	if !rows.Next() {
		return false
	}
	var winnerFaction uint32
	var hordeVictories uint32
	if err := rows.Scan(&winnerFaction, &hordeVictories); err != nil {
		return false
	}
	if !rows.Next() {
		return false
	}
	var allianceVictories uint32
	if err := rows.Scan(&winnerFaction, &allianceVictories); err != nil {
		return false
	}
	s.sendSysMessage(fmt.Sprintf("PvP statistics: %d Alliance victories, %d Horde victories in the last 7 days.", allianceVictories, hordeVictories))
	return true
}

// handleCmdRecall mirrors HandleRecallCommand (cs_misc.cpp:825-847, RBAC
// 520): finish the target's taxi flight, then TeleportTo the saved recall
// location (Player::Recall, Player.h:1990). The IsBeingTeleported refusal
// has no Go bridge (Go teleports are synchronous, so the window never
// opens); an unset recall location is a silent no-op (C++ TeleportTo on
// the default WorldLocation fails with no message).
func (s *session) handleCmdRecall(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandRecall) {
		return false
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target == nil || target.player == nil {
		s.sendSysMessage(miscPlayerNotFound)
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	if target.isInFlight() {
		target.finishTaxiFlight()
	}
	if !target.player.RecallSet {
		return true
	}
	target.teleportTo(target.player.RecallMap, target.player.RecallX, target.player.RecallY, target.player.RecallZ, target.player.RecallO)
	return true
}

// handleCmdRepairItems mirrors HandleRepairitemsCommand (cs_misc.cpp:2298-2316,
// RBAC 521): repair all of the target's items at no cost (the GM
// DurabilityRepairAll(false, 0, false) path — no cost, no discount). Max
// durability comes from item_template; the repair is a direct
// item_instance.durability update, per the items.go repair pattern.
func (s *session) handleCmdRepairItems(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandRepairItems) {
		return false
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target == nil {
		// C++ extractPlayerTarget (cs_misc.cpp:2301-2302) fails for offline
		// targets: PLAYER_NOT_FOUND, nothing repaired.
		s.sendSysMessage(miscPlayerNotFound)
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	// Player::DurabilityRepairAll (Player.cpp:4996-5006) covers equipped,
	// backpack and bag items only: bag 0 slots 0-38 and bags 19-22 slots
	// 0-35. Bank, buyback and keys are not repaired.
	rows, err := cdb.QueryContext(ctx,
		`SELECT ii.guid, ii.itemEntry, ii.durability FROM character_inventory ci
		 JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ?
		 AND ((ci.bag = 0 AND ci.slot < 39) OR (ci.bag >= 19 AND ci.bag < 23 AND ci.slot < 36))`, guid)
	if err != nil {
		return true
	}
	defer rows.Close()
	type itemRow struct {
		guid, entry, dur uint32
	}
	var items []itemRow
	for rows.Next() {
		var r itemRow
		if err := rows.Scan(&r.guid, &r.entry, &r.dur); err == nil {
			items = append(items, r)
		}
	}
	rows.Close()
	var wdb = cdb
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		wdb = s.server.WorldStore.DB
	}
	repaired := false
	for _, it := range items {
		var maxD uint32
		_ = wdb.QueryRowContext(ctx, "SELECT MaxDurability FROM item_template WHERE entry = ?", it.entry).Scan(&maxD)
		if maxD == 0 || maxD <= it.dur {
			continue
		}
		if _, err := cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", maxD, it.guid); err == nil {
			repaired = true
		}
	}
	if repaired && target != nil {
		_ = target.sendInventoryItems(ctx)
		target.sendPlayerUpdate()
	}
	s.sendSysMessage(fmt.Sprintf(misc2RepairedYou, miscPlayerLink(name)))
	if target != nil && target != s {
		target.sendSysMessage(fmt.Sprintf(misc2RepairedTarget, miscPlayerLink(s.player.Name)))
	}
	return true
}

// handleCmdRespawn mirrors HandleRespawnCommand (cs_misc.cpp:1801-1840,
// RBAC 522). A selection that resolves to a creature takes the selected
// leg (pets refused, dead creatures revived, live ones left alone); any
// other selection falls through to the grid sweep, as C++ does when
// getSelectedCreature returns null.
func (s *session) handleCmdRespawn(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandRespawn) {
		return false
	}
	if s.player == nil || s.server == nil {
		return false
	}
	if s.selection != 0 {
		if motion := s.castCommandSelectedCreature(); motion != nil {
			// C++ refuses only pets on the selected leg (cs_misc.cpp:1807-1812);
			// guardians and charmed creatures respawn like any other.
			if motionIsPet(motion) {
				s.sendSysMessage("Select a creature.") // LANG_SELECT_CREATURE recall
				return true
			}
			if motion.Health <= 0 {
				s.server.respawnCreatureNow(ctx, motion, uint32(motion.GUID&0x00FFFFFF), uint32(motion.GUID>>24)&0x00FFFFFF)
			}
			return true
		}
	}
	// Grid sweep (cs_misc.cpp:1826-1839). Leg 1 is RespawnDo over grid
	// objects inside the player's grid activation range — the map
	// visibility range (Object.cpp:1518; 100 yards on continents, 170 in
	// instances) — and covers creatures AND gameobjects. Leg 2 is the
	// map's respawn infos in the player's grid cell, which keeps the
	// grid-coord match. Go has no gameobject respawn-time registry, so
	// the GO half of leg 1 is the hidden-state sweep below and leg 2 has
	// no GO counterpart. Documented delta: battleground/arena maps use
	// 533 in C++; this path does not distinguish them.
	activationRange := float64(100)
	if s.player.InstanceID != 0 {
		activationRange = 170
	}
	inRange := func(x, y float32) bool {
		dx := float64(x - s.player.X)
		dy := float64(y - s.player.Y)
		return dx*dx+dy*dy <= activationRange*activationRange
	}
	gridX, gridY := gpsGridCoord(s.player.X, s.player.Y)
	s.server.motionMu.Lock()
	var records []creatureRespawn
	for _, record := range s.server.creatureRespawns {
		if record.Map == s.player.Map && record.InstanceID == s.player.InstanceID {
			if rx, ry := gpsGridCoord(record.X, record.Y); rx == gridX && ry == gridY {
				records = append(records, record)
			}
		}
	}
	for _, respawns := range s.server.instanceCreatureRespawns {
		for _, record := range respawns {
			if record.Map == s.player.Map && record.InstanceID == s.player.InstanceID {
				if rx, ry := gpsGridCoord(record.X, record.Y); rx == gridX && ry == gridY {
					records = append(records, record)
				}
			}
		}
	}
	var motions []*creatureMotion
	if live := s.server.motionMapLocked(s.player.Map, s.player.InstanceID); live != nil {
		for _, motion := range live {
			// Sweep legs have no owner filter in C++ (cs_misc.cpp:1826-1833):
			// RespawnDo visits every dead creature in range.
			if motion != nil && motion.Health <= 0 && inRange(motion.X, motion.Y) {
				motions = append(motions, motion)
			}
		}
	}
	s.server.motionMu.Unlock()
	// GO leg of RespawnDo (GridNotifiers.h:651-659): GameObject::Respawn
	// (GameObject.cpp:1338-1344) fires only for spawned-by-default objects
	// with a live respawn time. In Go that is a hidden static gameobject
	// whose script-set delay or DB spawntimesecs is positive — the
	// fully-looted chest/goober the loot path hid.
	var goRespawns []dynamicGameObjectState
	for _, st := range s.server.gameObjectStatesInInstance(s.player.Map, s.player.InstanceID) {
		if st.IsRuntimeSpawn || !s.server.isGameObjectHiddenInInstance(st.Map, st.InstanceID, st.GUID) || !inRange(st.X, st.Y) {
			continue
		}
		var delay int64
		if st.RespawnDelaySet {
			delay = int64(st.RespawnDelaySec)
		} else if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(spawntimesecs, 0) FROM gameobject WHERE guid = ? AND id = ? LIMIT 1", st.LowGUID, st.Entry).Scan(&delay)
		}
		if delay <= 0 {
			continue
		}
		goRespawns = append(goRespawns, st)
	}
	for _, record := range records {
		motion := s.server.findCreatureMotion(record.Map, record.InstanceID, creatureWorldGUID(record.GUID, record.Entry))
		s.server.respawnCreatureNow(ctx, motion, record.GUID, record.Entry)
	}
	for _, motion := range motions {
		s.server.respawnCreatureNow(ctx, motion, uint32(motion.GUID&0x00FFFFFF), uint32(motion.GUID>>24)&0x00FFFFFF)
	}
	for _, st := range goRespawns {
		s.server.setGameObjectHiddenInInstance(st.Map, st.InstanceID, st.GUID, false)
	}
	return true
}

// respawnCreatureNow mirrors the world-path revive block of
// processCreatureRespawns (kill.go): a dead motion returns to its pending
// record's health/position, or to full health at home with no record; the
// record and its DB row die, the loot state clears, and the spawn hooks
// fire. A live motion is a no-op (C++ respawns only the dead); a nil motion
// still applies the record's DB half (the due path's reseat/clear/
// broadcast with no live incarnation). A failed curhealth update keeps the
// record pending instead of dropping it, unlike the due path's
// collection-time delete.
func (s *Server) respawnCreatureNow(ctx context.Context, motion *creatureMotion, spawnGUID, entry uint32) bool {
	if s == nil || spawnGUID == 0 {
		return false
	}
	if motion != nil && motion.Health > 0 {
		return false
	}
	var record creatureRespawn
	found := false
	var instanceKey instanceAdmissionKey
	inInstance := false
	s.motionMu.Lock()
	if rec, ok := s.creatureRespawns[spawnGUID]; ok {
		record, found = rec, true
	} else {
		for key, respawns := range s.instanceCreatureRespawns {
			if rec, ok := respawns[spawnGUID]; ok {
				record, found, instanceKey, inInstance = rec, true, key, true
				break
			}
		}
	}
	s.motionMu.Unlock()
	var health uint32
	var hx, hy, hz float32
	var broadcastMap, broadcastInstance uint32
	if found {
		health, hx, hy, hz = record.Health, record.X, record.Y, record.Z
		broadcastMap, broadcastInstance = record.Map, record.InstanceID
	} else if motion != nil {
		health, hx, hy, hz = motion.MaxHealth, motion.HomeX, motion.HomeY, motion.HomeZ
		broadcastMap, broadcastInstance = motion.Map, motion.InstanceID
	} else {
		return false
	}
	if health == 0 {
		health = 1
	}
	if found && s.WorldStore != nil && s.WorldStore.DB != nil {
		if _, err := s.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = ? WHERE guid = ?", health, spawnGUID); err != nil {
			return false
		}
		s.deleteCreatureRespawnRow(ctx, spawnGUID, record.Map, record.InstanceID)
	}
	if found {
		s.motionMu.Lock()
		if inInstance {
			if respawns := s.instanceCreatureRespawns[instanceKey]; respawns != nil {
				delete(respawns, spawnGUID)
				if len(respawns) == 0 {
					delete(s.instanceCreatureRespawns, instanceKey)
				}
			}
		} else {
			delete(s.creatureRespawns, spawnGUID)
		}
		s.motionMu.Unlock()
	}
	rawGUID := creatureWorldGUID(spawnGUID, entry)
	if motion != nil {
		s.motionMu.Lock()
		motion.Health = health
		motion.MaxHealth = health
		motion.BaseMaxHealth = health
		motion.X, motion.Y, motion.Z = hx, hy, hz
		motion.InCombat, motion.TargetGUID, motion.Moving = false, 0, false
		motion.Looted = false
		motion.RespawnDelayKnown = false
		motion.RespawnDelaySec = 0
		motion.AlreadySearchedAssistance = false
		motion.TapPlayerGUID, motion.TapGroupID = 0, 0
		motion.PlayerDamageReq = health / 2
		s.motionMu.Unlock()
	}
	s.reseatRespawnedEntry(ctx, motion, entry)
	s.clearLootState(broadcastMap, broadcastInstance, rawGUID)
	s.broadcastCreatureValuesUpdateInInstance(broadcastMap, broadcastInstance, rawGUID, map[int]uint32{unitFieldHealth: health, unitFieldDynamicFlags: 0})
	if motion != nil {
		s.fireCreatureSpawned(ctx, motion)
		if motion.BossAI != nil {
			motion.BossAI.OnReset(ctx, s, motion)
		}
	}
	return true
}

// handleCmdRevive2 mirrors HandleReviveCommand (cs_misc.cpp:634-653, RBAC
// 523): resurrect the target player. The old xyz-only handleCmdRevive stub in
// commands.go is replaced by this full C++ port. Partial (documented
// fidelity gap, not a stub): the online path is native (ResurrectPlayer +
// SpawnCorpseBones + SaveToDB); the offline path
// (Player::OfflineResurrect) has no Go bridge.
func (s *session) handleCmdRevive2(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandRevive) {
		return false
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target != nil {
		// C++ checks the TARGET's permission (cs_misc.cpp:644), and is
		// silent on success — no "Revived" message.
		full := target.commandAllowed(ctx, permissionResurrectWithFullHPS)
		pct := float32(0.5)
		if full {
			pct = 1.0
		}
		target.resurrectPlayer(ctx, pct)
		target.spawnCorpseBones(ctx)
		_ = target.savePlayerState(ctx, 1, false)
		return true
	}
	s.sendSysMessage(fmt.Sprintf("The offline revive of %s is not available: offline resurrection has no Go bridge yet.", miscPlayerLink(name)))
	_ = guid
	return true
}

// handleCmdSaveAll mirrors HandleSaveAllCommand (cs_misc.cpp:872-879, RBAC
// 524): save every online player (ObjectAccessor::SaveAllPlayers).
func (s *session) handleCmdSaveAll(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandSaveAll) {
		return false
	}
	if s.server == nil {
		return false
	}
	s.server.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.server.sessions))
	for sess := range s.server.sessions {
		sessions = append(sessions, sess)
	}
	s.server.sessionsMu.RUnlock()
	for _, sess := range sessions {
		_ = sess.savePlayerState(ctx, 1, false)
	}
	s.sendSysMessage(misc2PlayersSaved)
	return true
}

// handleCmdSave2 mirrors HandleSaveCommand (cs_misc.cpp:848-871, RBAC 525):
// the without-delay branch saves the selected player (or self); the
// throttled branch saves the invoker only (cs_misc.cpp:867-869). The old
// position-only handleCmdSave stub in commands.go is replaced by this full
// C++ port. Documented fidelity gap (not a stub): CONFIG_INTERVAL_SAVE has
// no Go config field, so the 20-second save throttle is skipped and the
// save is unconditional outside the without-delay permission path.
func (s *session) handleCmdSave2(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandSave) {
		return false
	}
	target := s.server.playerSessionForGUID(s.selection)
	if target == nil || target.player == nil {
		target = s
	}
	if s.commandAllowed(ctx, permissionCommandsSaveWithoutDelay) {
		if err := target.savePlayerState(ctx, 1, false); err != nil {
			s.sendSysMessage("Failed to save character: " + err.Error())
			return true
		}
		s.sendSysMessage(misc2PlayerSaved)
		return true
	}
	if err := s.savePlayerState(ctx, 1, false); err != nil {
		s.sendSysMessage("Failed to save character: " + err.Error())
	}
	return true
}

// handleCmdSetSkill mirrors HandleSetSkillCommand (cs_misc.cpp:1378-1429,
// RBAC 526): the skill id takes a numeric id or a |Hskill:id| hyperlink
// (the Variant<Hyperlink<skill>, uint32> arg), the target is the selected
// player or self, and an unknown SkillLine id is refused. A known skill
// keeps its step and takes the new value/max; a new skill enters at step 1
// with max defaulting to the level, and a known skill's max defaults to its
// pure max. Level 0 or level above max is a silent no-op, as in C++; a
// new skill with no free slot still prints the set message (the
// Player::SetSkill no-op does not suppress the handler's message).
// The apply leg follows the Lua SetSkill bridge (lua_player.go): persist,
// learn rewarded spells, reach/learn achievement criteria, and the
// bonus-aura reapply on the add arm only. Documented fidelity gaps (not
// stubs): the UpdateSkillEnchantments legs have no Go model, and the
// LANG_SET_SKILL wording is a plain-English recall.
func (s *session) handleCmdSetSkill(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandSetSkill) {
		return false
	}
	if len(args) < 2 || len(args) > 3 {
		return false
	}
	skillID, ok := misc2ParseSkillArg(args[0])
	if !ok {
		return false
	}
	level, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return false
	}
	var maxArg uint32
	hasMaxArg := false
	if len(args) > 2 {
		parsedMax, parseErr := strconv.ParseUint(args[2], 10, 32)
		if parseErr != nil {
			return false
		}
		maxArg = uint32(parsedMax)
		hasMaxArg = true
	}
	target := s.guildSelectionOrSelf()
	if target == nil || target.player == nil {
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED
		return true
	}
	// C++ validates against sSkillLineStore unconditionally; with no
	// DBC store to check against, refuse rather than inventing a skill
	// entry (the nil-WorldStore -> ITEMIDINVALID precedent in additem).
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage(fmt.Sprintf("Invalid skill ID %d.", skillID))
		return true
	}
	if _, found, err := s.server.Data.SkillLineCategory(skillID); err == nil && !found {
		s.sendSysMessage(fmt.Sprintf("Invalid skill ID %d.", skillID))
		return true
	}
	index := misc2SkillIndex(target.player.Skills, skillID)
	targetHasSkill := target.playerSkillTotalValue(skillID) != 0
	var max uint16
	switch {
	case hasMaxArg:
		max = uint16(maxArg)
	case targetHasSkill && index >= 0:
		max = target.player.Skills[index].Max
	default:
		max = uint16(level)
	}
	if level == 0 || level > uint64(max) {
		return false
	}
	applied := true
	if index >= 0 {
		step := uint16(1)
		if targetHasSkill {
			step = target.player.Skills[index].Step
		}
		target.player.Skills[index].Step = step
		target.player.Skills[index].Value = uint16(level)
		target.player.Skills[index].Max = max
		misc2PersistSkill(ctx, target, target.player.Skills[index])
	} else if len(target.player.Skills) >= playerMaxSkills {
		// C++ Player::SetSkill finds no free slot and no-ops, but the
		// handler still prints LANG_SET_SKILL unconditionally
		// (cs_misc.cpp:1426-1427) — so the message below still fires
		// while the rewarded-spell/criteria legs stay skipped.
		applied = false
	} else {
		target.player.Skills = append(target.player.Skills, playerSkill{Skill: uint16(skillID), Step: 1, Value: uint16(level), Max: max})
		misc2PersistSkill(ctx, target, target.player.Skills[len(target.player.Skills)-1])
		target.reapplySkillBonusAuras(skillID)
	}
	if applied {
		target.learnSkillRewardedSpells(ctx, skillID, uint32(level))
		target.setAchievementCriteria(criteriaTypeReachSkillLevel, skillID, uint32(level))
		target.setAchievementCriteria(criteriaTypeLearnSkillLevel, skillID, uint32(max))
	}
	if name := s.misc2SkillLineName(skillID); name != "" {
		s.sendSysMessage(fmt.Sprintf("Skill %d (%s) set to %d (max %d) for %s.", skillID, name, level, max, target.player.Name))
	} else {
		s.sendSysMessage(fmt.Sprintf("Skill %d set to %d (max %d) for %s.", skillID, level, max, target.player.Name))
	}
	return true
}

// handleCmdSummon mirrors HandleSummonCommand (cs_misc.cpp:486-599, RBAC
// 528): teleport the target player to the GM's close point (Object.cpp:3307)
// keeping the target's orientation, and set the target's phase to the GM's.
// Documented fidelity gaps (not stubs): the battleground gate has no Go
// bridge (no BG model), and the GetNearPoint LoS sweep and ground clamp
// have none either, so the first close point stands. The instance gate
// below ports cs_misc.cpp:540-560; the offline leg
// (Player::SavePositionInDB) is native via the characters table except the
// zone column (see commands_tele.go).
func (s *session) handleCmdSummon(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandSummon) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target == s || guid == s.playerGUID {
		s.sendSysMessage(miscCantTeleportSelf)
		return true
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	if target != nil && target.player != nil {
		if target.isBeingTeleported() {
			s.sendSysMessage(fmt.Sprintf(miscIsTeleported, miscPlayerLink(name)))
			return true
		}
		if s.isDungeonMap(s.player.Map) {
			gmMapID, gmInstance := s.player.Map, s.player.InstanceID
			targetMapID, targetInstance := target.player.Map, target.player.InstanceID
			leaderOnGMInstance := false
			if s.server != nil {
				if group := s.server.groupOfGUID(target.playerGUID); group != nil {
					if leader := s.server.findSessionByGUID(group.LeaderGUID); leader != nil && leader.player != nil && leader.player.Map == gmMapID && leader.player.InstanceID == gmInstance {
						leaderOnGMInstance = true
					}
				}
			}
			if !leaderOnGMInstance && (targetMapID != gmMapID || targetInstance != gmInstance) {
				s.sendSysMessage(miscCannotSummonToInst)
				return true
			}
			if targetMapID == gmMapID && targetInstance != gmInstance {
				s.sendSysMessage(fmt.Sprintf("%s cannot be summoned from an instance to an instance.", miscPlayerLink(name)))
				return true
			}
		}
		s.sendSysMessage(fmt.Sprintf(miscSummoning, miscPlayerLink(name), ""))
		// needReportToTarget (Chat.cpp:754-758, commands_tele.go
		// precedent): the target is told only when the invoker is
		// visible globally for them — an invisible GM summoning leaks
		// no presence. The invoker != target leg is already enforced
		// above (miscCantTeleportSelf).
		if isVisibleGloballyFor(target, s) {
			target.sendSysMessage(fmt.Sprintf(miscSummonedBy, miscPlayerLink(s.player.Name)))
		}
		// C++ stops the GM's flight (not the target's) or saves the GM's
		// recall position (cs_misc.cpp:570-574, mirrored as written).
		if s.isInFlight() {
			s.finishTaxiFlight()
		} else {
			s.saveRecallPosition()
		}
		cx, cy := miscClosePoint(s.player.X, s.player.Y, s.player.Orientation, miscPlayerCombatReach(s.player), miscPlayerCombatReach(target.player), 0)
		gmPhase := s.currentPlayerPhaseMask()
		if target.teleportTo(s.player.Map, cx, cy, s.player.Z, target.player.Orientation) {
			miscSetPhaseMask(target, gmPhase)
		}
		return true
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, guid)
	}
	s.sendSysMessage(fmt.Sprintf(miscSummoning, miscPlayerLink(name), miscOfflineSuffix))
	return true
}

// handleCmdUnFreeze mirrors HandleUnFreezeCommand (cs_misc.cpp:2416-2472,
// RBAC 531): remove the freeze aura (9454) from the target player. The online
// path is native (RemoveAurasDueToSpell); the offline path is native via the
// character_aura table (CHAR_DEL_CHAR_AURA_FROZEN).
func (s *session) handleCmdUnFreeze(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandUnFreeze) {
		return false
	}
	var name string
	var target *session
	if len(args) > 0 && args[0] != "" {
		name = normalizePlayerName(args[0])
		target = s.sessionForPlayerName(name)
	} else {
		// C++ no-name leg uses getSelectedPlayer() (cs_misc.cpp:2429-2433):
		// no selection falls back to self.
		target = s.miscSelectedPlayerStrict()
		if target != nil && target.player != nil {
			name = target.player.Name
		}
	}
	if target != nil && target.player != nil {
		s.sendSysMessage(fmt.Sprintf(misc2Unfreezing, miscPlayerLink(name)))
		target.removeAura(freezeAuraSpellID)
		return true
	}
	if name != "" && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var guid uint64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&guid); err == nil && guid != 0 {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
				"DELETE FROM character_aura WHERE guid = ? AND spell = ?", guid, freezeAuraSpellID)
			s.sendSysMessage(fmt.Sprintf(misc2Unfreezing, miscPlayerLink(name)))
			return true
		}
	}
	s.sendSysMessage(miscFreezeWrong)
	return true
}

// handleCmdUnmute mirrors HandleUnmuteCommand (cs_misc.cpp:1916-1964, RBAC
// 532): an online target that CanSpeak already is refused with the
// already-enabled message; otherwise the session muteTime clears and the
// login DB row resets (mutetime 0, reason/by blank). History rows are left
// in place, as in C++ (unmute never deletes account_muted rows).
func (s *session) handleCmdUnmute(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandUnMute) {
		return false
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return true
	}
	if target == nil {
		if accountID := s.misc2AccountForGUID(ctx, guid); accountID != 0 {
			target = s.misc2SessionForAccount(accountID)
		}
	}
	// C++ HasLowerSecurity(..., true) (cs_misc.cpp:1932): strong — equal
	// security also fails, no RBAC-47 bypass.
	if s.characterTargetLowerSecurityStrong(ctx, characterTarget{online: target, guid: guid, name: name}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	var accountID uint32
	if target != nil {
		accountID = target.accountID
	} else {
		accountID = s.misc2AccountForGUID(ctx, guid)
	}
	if target != nil {
		if !(target.muteTime > time.Now().Unix()) {
			s.sendSysMessage("Chat is already enabled.")
			return true
		}
		target.muteTime = 0
	}
	if db := s.authDB(); db != nil && accountID != 0 {
		_, _ = db.ExecContext(ctx, "UPDATE account SET mutetime = ?, mutereason = ?, muteby = ? WHERE id = ?", 0, "", "", accountID)
	}
	if target != nil {
		target.sendSysMessage("Your chat is enabled.")
	}
	s.sendSysMessage(fmt.Sprintf("You enabled chat for %s.", miscPlayerLink(name)))
	return true
}

// charmAuraTypes is Unit::RemoveCharmAuras (Unit.cpp:6525-6531) in C++
// order: MOD_CHARM, MOD_POSSESS_PET, MOD_POSSESS, AOE_CHARM.
var charmAuraTypes = []uint32{spellAuraCharm, spellAuraModPossessPet, spellAuraModPossess, spellAuraAoeCharm}

// handleCmdUnPossess mirrors HandleUnPossessCommand (cs_misc.cpp:2534-2545,
// RBAC 533): RemoveCharmAuras on the selected unit, else the GM. A
// creature selection runs the creature remove funnel (uncharmCreature plus
// the client-control release); a player selection runs the session remove
// funnel. Delta: a selection resolving to neither falls back to self.
func (s *session) handleCmdUnPossess(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandUnPossess) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if s.selection != 0 {
		if target := s.miscSelectedPlayerSession(); target != nil {
			for _, auraType := range charmAuraTypes {
				target.removeAurasByType(auraType)
			}
			return true
		}
		if motion := s.castCommandSelectedCreature(); motion != nil && s.server != nil {
			key := creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}
			for _, auraType := range charmAuraTypes {
				s.server.removeCreatureAurasByType(key, auraType)
			}
			return true
		}
	}
	for _, auraType := range charmAuraTypes {
		s.removeAurasByType(auraType)
	}
	return true
}

// handleCmdUnstuck mirrors HandleUnstuckCommand (cs_misc.cpp:916-976, RBAC
// 534): teleport the target out of a stuck spot. Partial (documented fidelity
// gaps, not stubs): the no-permission self path (cast spell 7355 "Stuck")
// has no spell-cast bridge (applyAura is not a cast and the spell's
// unstuck effect is unmodeled); the startzone leg (GetStartPosition) has
// no race/class start-position bridge. The inn leg (homebind teleport)
// and the graveyard leg (RepopAtGraveyard via repopAtGraveyard) are
// native, and the C++ flight/combat refusal is enforced (a sys message
// stands in for the SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW cast result).
func (s *session) handleCmdUnstuck(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandUnstuck) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	if !s.commandAllowed(ctx, permissionCommandsUseUnstuckWithArgs) {
		s.sendSysMessage("The self-unstuck cast (spell 7355) is not available: spell casting has no Go bridge on this path.")
		return true
	}
	if len(args) == 0 {
		return false
	}
	target, _, name, ok := s.miscResolvePlayerTarget(ctx, args[:1])
	if !ok {
		return true
	}
	if target == nil || target.player == nil {
		// C++ extractPlayerTarget fails for offline targets (cs_misc.cpp:937-938).
		s.sendSysMessage(miscPlayerNotFound)
		return true
	}
	// C++ HandleUnstuckCommand (cs_misc.cpp:916-976) has no HasLowerSecurity gate.

	if target.inFlight || target.attackTarget != 0 || target.player.UnitFlags&unitFlagInCombat != 0 {
		s.sendSysMessage(fmt.Sprintf("%s cannot be unstuck while in flight or combat.", miscPlayerLink(name)))
		return true
	}
	loc := "inn"
	if len(args) > 1 {
		// C++ compares the raw token (cs_misc.cpp:956-966): "INN" is not "inn".
		loc = args[1]
	}
	switch loc {
	case "inn":
		target.teleportTo(target.player.HomebindMap, target.player.HomebindX, target.player.HomebindY, target.player.HomebindZ, target.player.Orientation)
	case "graveyard":
		target.repopAtGraveyard(ctx)
	case "startzone":
		s.sendSysMessage(fmt.Sprintf("The startzone leg of .unstuck for %s is not available: race/class start positions have no Go bridge yet.", miscPlayerLink(name)))
	default:
		return false
	}
	return true
}

// handleCmdChangeWeather mirrors HandleChangeWeather (cs_misc.cpp:1337-1362,
// RBAC 535): with weather disabled it answers LANG_WEATHER_DISABLED; the
// player's zone needs a game_weather row (GetOrGenerateZoneDefaultWeather
// returns null otherwise, LANG_NO_WEATHER); the zone weather is then set to
// the given type and grade via Weather::SetWeather, which no-ops on an
// identical pair and otherwise broadcasts to the zone.
// miscWeatherTypeNames is the EnumUtils search map for WeatherType
// (SharedDefines.h:3146-3154, enuminfo_SharedDefines.cpp:2756-2769): the
// constant names, matched case-insensitively by unique prefix
// (ChatCommandArgs.h:128-200). A bare "rain" does not match — only the
// "WEATHER_TYPE_*" constants do.
var miscWeatherTypeNames = []struct {
	name  string
	value weatherType
}{
	{"WEATHER_TYPE_FINE", weatherFine},
	{"WEATHER_TYPE_RAIN", weatherRain},
	{"WEATHER_TYPE_SNOW", weatherSnow},
	{"WEATHER_TYPE_STORM", weatherStorm},
	{"WEATHER_TYPE_THUNDERS", weatherThunders},
	{"WEATHER_TYPE_BLACKRAIN", weatherBlackRain},
}

// miscParseWeatherType mirrors the enum ArgInfo for WeatherType: a
// case-insensitive unique-prefix name match first, then a numeric parse
// that must be a valid enum value (0, 1, 2, 3, 86, 90 — not 0-5).
func miscParseWeatherType(token string) (weatherType, bool) {
	var match weatherType
	matched := false
	for _, entry := range miscWeatherTypeNames {
		if len(token) > len(entry.name) || !strings.EqualFold(token, entry.name[:len(token)]) {
			continue
		}
		if matched {
			return 0, false // ambiguous prefix
		}
		match, matched = entry.value, true
	}
	if matched {
		return match, true
	}
	val, err := strconv.ParseUint(token, 10, 32)
	if err != nil {
		return 0, false
	}
	switch weatherType(val) {
	case weatherFine, weatherRain, weatherSnow, weatherStorm, weatherThunders, weatherBlackRain:
		return weatherType(val), true
	}
	return 0, false
}

func (s *session) handleCmdChangeWeather(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandChangeWeather) {
		return false
	}
	// The C++ framework parses the (WeatherType, float) args before the
	// handler runs (ChatCommandArgs.h), so a bad arg is a syntax failure
	// even with weather disabled, and the float parse rejects non-finite
	// values (std::isfinite, ChatCommandArgs.h:69) — only then does
	// HandleChangeWeather check CONFIG_WEATHER (cs_misc.cpp:1337).
	if len(args) < 2 {
		return false
	}
	wtype, ok := miscParseWeatherType(args[0])
	if !ok {
		return false
	}
	intensity, err := strconv.ParseFloat(args[1], 32)
	if err != nil || math.IsNaN(intensity) || math.IsInf(intensity, 0) {
		return false
	}
	if s.server == nil || !s.server.Config.WeatherEnabled {
		s.sendSysMessage("Weather is disabled by config.")
		return true
	}
	if s.player == nil {
		return false
	}
	weather := s.server.getOrGenerateZoneWeather(ctx, s.player.Zone)
	if weather == nil {
		s.sendSysMessage("There is no weather in this zone.")
		return true
	}
	s.server.setZoneWeather(ctx, s.player.Zone, wtype, float32(intensity))
	return true
}

// handleCmdMailBox mirrors HandleMailBoxCommand (cs_misc.cpp:2566-2574, RBAC
// 777): SendShowMailBox with the player's own GUID — the exact
// lua_player.go SMSG_SHOW_MAILBOX packet, with no interactability gate
// (unlike the mailbox opcode path, which checks canOpenMailBox).
func (s *session) handleCmdMailBox(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMailBox) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	mailbox := protocol.NewBuffer(8)
	mailbox.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SHOW_MAILBOX), mailbox.Bytes(), true)
	return true
}
