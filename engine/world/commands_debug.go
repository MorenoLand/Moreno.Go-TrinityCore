package world

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type debugObjectCountRow struct {
	mapID       uint32
	instanceID  uint32
	mapName     string
	creatures   int
	gameobjects int
	top         []debugObjectEntryCount
}

type debugObjectEntryCount struct {
	entry uint32
	count int
}

// handleDebugAnim mirrors HandleDebugAnimCommand (cs_debug.cpp:1069):
// ".debug anim <emote>". getSelectedUnit (Chat.cpp:312) is the selected
// unit, else the handler's own player; Unit::HandleEmoteCommand sends
// SMSG_EMOTE (u32 emote + u64 guid) to the visibility set, which is the
// npcCreatureEmote fan-out for creatures and the PerformEmote twin
// (self write + visible-set fan-out) for players.
func (s *session) handleDebugAnim(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug anim <emote>"
	if len(args) != 1 {
		return false
	}
	emote, ok := parseCommandUint32(args[0])
	if !ok || !debugEmoteValid(emote) {
		// Unparseable/out-of-set enum token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	if motion := s.debugSelectedCreatureMotion(); motion != nil {
		s.npcCreatureEmote(motion, uint32(emote))
	} else if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			s.debugPlayerEmote(ts, uint32(emote))
		} else {
			s.debugPlayerEmote(s, uint32(emote))
		}
	} else {
		s.debugPlayerEmote(s, uint32(emote))
	}
	// C++ prints the emote's constant name ("Playing emote %s"); Go has
	// no emote-name table, so the numeric id stands in.
	s.sendSysMessage(fmt.Sprintf("Playing emote %d", emote))
	return true
}

func (s *session) debugPlayerEmote(ts *session, emote uint32) {
	if ts == nil || ts.server == nil {
		return
	}
	pkt := protocol.NewBuffer(12)
	pkt.WriteU32(emote)
	pkt.WriteU64(ts.playerGUID)
	_ = ts.write(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), true)
	ts.server.broadcastToVisibleSetFromSession(ts, uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes())
}

// handleDebugWorldState mirrors HandleDebugUpdateWorldStateCommand
// (cs_debug.cpp:426): ".debug worldstate <variable> <value>" sends the
// SMSG_UPDATE_WORLD_STATE pair to the handler's own player via the
// existing sendWorldState packet helper (battleground_av.go).
func (s *session) handleDebugWorldState(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug worldstate <variable> <value>"
	if len(args) != 2 {
		return false
	}
	variable, ok1 := parseCommandUint32(args[0])
	value, ok2 := parseCommandUint32(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	s.sendWorldState(variable, value)
	return true
}

// handleDebugDummy mirrors HandleDebugDummyCommand (cs_debug.cpp:1792):
// the arm's whole C++ body is the literal no-op notice.
func (s *session) handleDebugDummy(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	s.sendSysMessage("This command does nothing right now. Edit your local core (cs_debug.cpp) to make it do whatever you need for testing.")
	return true
}

// handleDebugObjectCount mirrors HandleDebugObjectCountCommand plus
// HandleDebugObjectCountMap (cs_debug.cpp:1715, 1770): ".debug objectcount
// [mapId]" prints, per (map, instance), the live creature count and the
// runtime gameobject count, then the top 5 creature entries by count.
func (s *session) handleDebugObjectCount(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	var mapFilter *uint32
	if len(args) > 1 {
		return false
	}
	if len(args) > 0 {
		id, ok := parseCommandUint32(args[0])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid map ID.")
			return true
		}
		v := id
		mapFilter = &v
	}
	if s.server == nil || s.player == nil {
		return false
	}
	rows := s.server.debugObjectCountRows(mapFilter, s.player.Map, s.player.InstanceID)
	for _, c := range rows {
		s.sendSysMessage(fmt.Sprintf("Map Id: %d Name: '%s' Instance Id: %d Creatures: %d GameObjects: %d",
			c.mapID, c.mapName, c.instanceID, c.creatures, c.gameobjects))
		s.sendSysMessage("Top Creatures count:")
		for _, e := range c.top {
			s.sendSysMessage(fmt.Sprintf("Entry: %d Count: %d", e.entry, e.count))
		}
	}
	return true
}

// debugObjectCountRows is the countable core of handleDebugObjectCount:
// live creature motions per (map, instance) plus runtime gameobjects,
// with the top 5 creature entries per map. The invoker's own map always
// gets a row even when it currently holds no live motions or runtime GOs.
func (s *Server) debugObjectCountRows(mapFilter *uint32, selfMap, selfInstance uint32) []debugObjectCountRow {
	type mapCount struct {
		mapID       uint32
		instanceID  uint32
		creatures   int
		gameobjects int
		entries     map[uint32]int
	}
	counts := map[instanceAdmissionKey]*mapCount{}
	get := func(mapID, instanceID uint32) *mapCount {
		key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
		if counts[key] == nil {
			counts[key] = &mapCount{mapID: mapID, instanceID: instanceID, entries: map[uint32]int{}}
		}
		return counts[key]
	}
	s.motionMu.Lock()
	for key, motions := range s.instanceCreatureMotion {
		if mapFilter != nil && key.MapID != *mapFilter {
			continue
		}
		c := get(key.MapID, key.InstanceID)
		for _, motion := range motions {
			if motion == nil {
				continue
			}
			c.creatures++
			c.entries[motion.Entry]++
		}
	}
	s.motionMu.Unlock()
	s.objectsMu.RLock()
	collectGO := func(state *dynamicGameObjectState) {
		if state == nil {
			return
		}
		if mapFilter != nil && state.Map != *mapFilter {
			return
		}
		get(state.Map, state.InstanceID).gameobjects++
	}
	for _, state := range s.dynamicGameObjects {
		collectGO(state)
	}
	for _, byGUID := range s.instanceGameObjects {
		for _, state := range byGUID {
			collectGO(state)
		}
	}
	s.objectsMu.RUnlock()
	if mapFilter == nil || *mapFilter == selfMap {
		get(selfMap, selfInstance)
	}
	keys := make([]instanceAdmissionKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].MapID != keys[j].MapID {
			return keys[i].MapID < keys[j].MapID
		}
		return keys[i].InstanceID < keys[j].InstanceID
	})
	rows := make([]debugObjectCountRow, 0, len(counts))
	for _, key := range keys {
		c := counts[key]
		row := debugObjectCountRow{mapID: c.mapID, instanceID: c.instanceID, creatures: c.creatures, gameobjects: c.gameobjects}
		if s.Data != nil {
			if entry, found, err := s.Data.Map(c.mapID); err == nil && found {
				row.mapName = entry.MapName
			}
		}
		// C++ sorts by count through a std::set, which collapses equal
		// counts; Go keeps every entry, ordered count desc then entry asc
		// for a deterministic top 5.
		top := make([]debugObjectEntryCount, 0, len(c.entries))
		for entry, count := range c.entries {
			top = append(top, debugObjectEntryCount{entry: entry, count: count})
		}
		sort.Slice(top, func(i, j int) bool {
			if top[i].count != top[j].count {
				return top[i].count > top[j].count
			}
			return top[i].entry < top[j].entry
		})
		if len(top) > 5 {
			top = top[:5]
		}
		row.top = top
		rows = append(rows, row)
	}
	return rows
}

// handleDebugNearGraveyard mirrors HandleDebugNearGraveyard
// (cs_debug.cpp:1490): ".debug neargraveyard [linked]". The plain form
// scans every WorldSafeLocs.dbc row on the invoker's map for the nearest
// by squared distance (the DBC-record iteration of C++'s row loop); the
// linked form rides Server.closestGraveyard, the ObjectMgr::GetClosestGraveyard
// twin (zone links, faction filter, default-graveyard fallback). The
// battleground/battlefield GetClosestGraveyard legs have no Go model, so
// the linked form always takes the ObjectMgr fallback. The LANG texts
// (82/83) are unseeded in-tree, so plain prose stands in.
func (s *session) handleDebugNearGraveyard(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) > 1 || (len(args) == 1 && !strings.EqualFold(args[0], "linked")) {
		return false
	}
	if s.server == nil || s.player == nil || s.server.Data == nil {
		return false
	}
	linked := len(args) > 0 && strings.EqualFold(args[0], "linked")
	if linked {
		loc, found := s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.Zone, playerTeam(s.player.Race))
		if !found {
			s.sendSysMessage("No graveyard found.")
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Nearest graveyard: %d at (%f, %f, %f)", loc.ID, loc.X, loc.Y, loc.Z))
		return true
	}
	file, err := s.server.Data.File("WorldSafeLocs")
	if err != nil {
		s.sendSysMessage("No graveyard found.")
		return true
	}
	type safeLoc struct {
		id      uint32
		x, y, z float32
	}
	var nearest *safeLoc
	var nearestDist float32
	for i := 0; i < file.Records(); i++ {
		record, err := file.Record(i)
		if err != nil {
			continue
		}
		id, err1 := record.Uint32(0)
		continent, err2 := record.Uint32(1)
		x, err3 := record.Float32(2)
		y, err4 := record.Float32(3)
		z, err5 := record.Float32(4)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || continent != s.player.Map {
			continue
		}
		dx, dy, dz := x-s.player.X, y-s.player.Y, z-s.player.Z
		dist := dx*dx + dy*dy + dz*dz
		if nearest == nil || dist < nearestDist {
			loc := safeLoc{id: id, x: x, y: y, z: z}
			nearest = &loc
			nearestDist = dist
		}
	}
	if nearest == nil {
		s.sendSysMessage("No graveyard found.")
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Nearest graveyard: %d at (%f, %f, %f)", nearest.id, nearest.x, nearest.y, nearest.z))
	return true
}

// debugEmoteValues is the Emote membership set (SharedDefines.h:1935):
// 175 named values from 0 to 476 with gaps. C++ parses the enum arg by
// membership (EnumUtils::IsValid), so a numeric outside this set fails.
var debugEmoteValues = []uint32{0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 33, 34, 35, 36, 37, 38, 39, 43, 44, 45, 48, 50, 51, 53, 54, 60, 61, 64, 65, 66, 68, 69, 70, 71, 92, 93, 94, 113, 133, 153, 173, 193, 213, 214, 233, 234, 253, 254, 273, 274, 275, 293, 313, 333, 353, 373, 374, 375, 376, 377, 378, 379, 380, 381, 382, 383, 384, 385, 386, 387, 388, 389, 390, 391, 392, 393, 394, 395, 396, 397, 398, 399, 400, 401, 402, 403, 404, 405, 406, 407, 408, 409, 410, 411, 412, 413, 415, 416, 417, 418, 419, 420, 421, 422, 423, 424, 425, 426, 427, 428, 429, 430, 431, 432, 433, 434, 435, 436, 437, 438, 439, 440, 441, 442, 443, 444, 445, 446, 447, 448, 449, 450, 451, 452, 453, 454, 455, 456, 457, 458, 459, 460, 461, 462, 463, 465, 466, 467, 468, 469, 470, 471, 473, 474, 475, 476}

func debugEmoteValid(v uint32) bool {
	i := sort.Search(len(debugEmoteValues), func(i int) bool { return debugEmoteValues[i] >= v })
	return i < len(debugEmoteValues) && debugEmoteValues[i] == v
}

// debugSendEnumValid is the EnumUtils::IsValid membership check for the
// debug send arms (ChatCommandArgs.h:184-200): buyerror/sellerror/
// equiperror (ItemDefines.h), channelnotify (Channel.h:31), chatmessage
// (SharedDefines.h MAX_CHAT_MSG_TYPE 0x34), qpartymsg (QuestShareMessages)
// and qinvalidmsg (QuestFailedReason, QuestDef.h:55-79). Numeric-only is
// the documented delta; an
// out-of-range numeric fails where C++ rejects it.
func debugSendEnumValid(arm string, value uint32) bool {
	switch arm {
	case "buyerror":
		switch value {
		case 0, 1, 2, 4, 5, 7, 8, 11, 12:
			return true
		}
	case "sellerror":
		return value >= 1 && value <= 7
	case "equiperror":
		return value <= 73 || (value >= 75 && value <= 82) || (value >= 84 && value <= 89)
	case "channelnotify":
		return value <= 35
	case "chatmessage":
		return value <= 51
	case "qpartymsg":
		return value <= 11
	case "qinvalidmsg":
		switch value {
		case 0, 1, 6, 7, 12, 13, 16, 18, 21, 23, 26, 27, 29:
			return true
		}
	}
	return false
}

// debugSelectedCreatureMotion resolves the selected creature across
// instances, the getSelectedUnit twin (Chat.cpp:312) for creature
// targets: C++ resolves globally, while the session-scoped finder is
// map-bound.
func (s *session) debugSelectedCreatureMotion() *creatureMotion {
	if s == nil || s.server == nil || s.selection == 0 {
		return nil
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	for scope := range s.server.instanceCreatureMotion {
		if m := s.server.findCreatureMotionLocked(scope.MapID, scope.InstanceID, s.selection); m != nil {
			return m
		}
	}
	return nil
}

// debugSelectedUnit resolves the getSelectedUnit twin (Chat.cpp:312) for
// the debug arms: the selected creature (scanned across instances, since
// C++ resolves globally and the session-scoped finder is map-bound), else
// the selected player, else the invoker. The returned tuple is the unit's
// name, GUID, position, map and instance.
func (s *session) debugSelectedUnit() (name string, guid uint64, x, y, z float32, mapID, instanceID uint32) {
	if s == nil || s.player == nil {
		return "", 0, 0, 0, 0, 0, 0
	}
	if s.selection != 0 && s.server != nil {
		s.server.motionMu.Lock()
		for scope := range s.server.instanceCreatureMotion {
			if m := s.server.findCreatureMotionLocked(scope.MapID, scope.InstanceID, s.selection); m != nil {
				s.server.motionMu.Unlock()
				return m.Name, m.GUID, m.X, m.Y, m.Z, m.Map, m.InstanceID
			}
		}
		s.server.motionMu.Unlock()
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts.player.Name, ts.playerGUID, ts.player.X, ts.player.Y, ts.player.Z, ts.player.Map, ts.player.InstanceID
		}
	}
	return s.player.Name, s.playerGUID, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.InstanceID
}

// handleDebugLoS mirrors HandleDebugLoSCommand (cs_debug.cpp:1079):
// ".debug los" prints the VMAP line, the gameobject line and the combined
// verdict for the invoker -> selected unit. The VMAP leg rides
// Server.hasLineOfSight (the terrain-model check); Go has no gameobject
// collision LOS model, so the GObj leg always reads clear and the combined
// verdict follows the VMAP leg. Cross-map/cross-instance pairs read
// obstructed on every line (C++ IsInMap gate, Object.cpp:1272). Positions
// are raw: C++ adds collision heights and hit-sphere points, which Go's
// LOS callers (totem legs) also skip.
func (s *session) handleDebugLoS(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if s.server == nil || s.player == nil {
		return true
	}
	name, _, ux, uy, uz, uMap, uInst := s.debugSelectedUnit()
	sameMap := uMap == s.player.Map && uInst == s.player.InstanceID
	vmapClear := sameMap && s.server.hasLineOfSight(s.player.Map, s.player.X, s.player.Y, s.player.Z, ux, uy, uz)
	gobjClear := sameMap
	clear := func(v bool) string {
		if v {
			return "clear"
		}
		return "obstructed"
	}
	s.sendSysMessage(fmt.Sprintf("Checking LoS %s -> %s:", s.player.Name, name))
	s.sendSysMessage(fmt.Sprintf("    VMAP LoS: %s", clear(vmapClear)))
	s.sendSysMessage(fmt.Sprintf("    GObj LoS: %s", clear(gobjClear)))
	not := "not "
	if vmapClear && gobjClear {
		not = ""
	}
	s.sendSysMessage(fmt.Sprintf("%s is %sin line of sight of %s.", name, not, s.player.Name))
	return true
}

// handleDebugPlay mirrors the debugPlayCommandTable (cs_debug.cpp:73-79):
// ".debug play cinematic|movie|sound|music <id>". Cinematic and movie ride
// the SendCinematicStart/SendMovieStart packet twins (lua_player.go); the
// cinematic flyby-waypoint dump and first-camera activation have no Go
// model. Sound follows HandleDebugPlaySoundCommand (cs_debug.cpp:186):
// a set target (s.selection, the CMSG_SET_SELECTION twin) takes the
// distance leg (SMSG_PLAY_OBJECT_SOUND, unit GUID), otherwise the direct
// leg (SMSG_PLAY_SOUND); both write to the invoker only, as C++ passes the
// handler's player as the recipient. Music follows
// HandleDebugPlayMusicCommand (cs_debug.cpp:209): SMSG_PLAY_MUSIC to the
// invoker. DELTA (shared with .misc playall and the Lua sound methods):
// the CinematicSequences/Movie/SoundEntries DBC existence gates have no Go
// model, so unknown ids are sent and play nothing client-side instead of
// reporting not-exist; the "You hear sound" wording is a prose recall of
// the unseeded LANG text.
func (s *session) handleDebugPlay(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug play cinematic|movie|sound|music <id>"
	if len(args) != 2 {
		return false
	}
	id, ok := parseCommandUint32(args[1])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	switch strings.ToLower(args[0]) {
	case "cinematic":
		pkt := protocol.NewBuffer(4)
		pkt.WriteU32(uint32(id))
		_ = s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), pkt.Bytes(), true)
	case "movie":
		if s.player != nil {
			s.player.Movie = uint32(id)
		}
		pkt := protocol.NewBuffer(4)
		pkt.WriteU32(uint32(id))
		_ = s.write(uint16(protocol.OpcodeSMSG_TRIGGER_MOVIE), pkt.Bytes(), true)
	case "sound":
		_, unitGUID, _, _, _, _, _ := s.debugSelectedUnit()
		if s.selection != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_PLAY_OBJECT_SOUND), buildPlayObjectSound(uint32(id), unitGUID), true)
		} else {
			_ = s.write(uint16(protocol.OpcodeSMSG_PLAY_SOUND), buildPlaySound(uint32(id)), true)
		}
		s.sendSysMessage(fmt.Sprintf("You hear sound %d", id))
	case "music":
		pkt := protocol.NewBuffer(4)
		pkt.WriteU32(uint32(id))
		_ = s.write(uint16(protocol.OpcodeSMSG_PLAY_MUSIC), pkt.Bytes(), true)
		s.sendSysMessage(fmt.Sprintf("You hear sound %d", id))
	default:
		s.sendSysMessage(syntax)
	}
	return true
}

// handleDebugASANMemoryLeak mirrors HandleDebugMemoryLeak
// (cs_debug.cpp:1657) and handleDebugASANOutOfBounds mirrors
// HandleDebugOutOfBounds (:1646): both bodies are #ifdef ASAN, so a
// non-ASAN build returns true with no output. Go has no ASAN build,
// so the leaves are silent no-ops. Console::Yes in C++, so no
// player is required and the leaf path bypasses handleCmdDebug.
func (s *session) handleDebugASANMemoryLeak(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) > 0 {
		return false
	}
	return true
}

func (s *session) handleDebugASANOutOfBounds(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) > 0 {
		return false
	}
	return true
}

// handleDebugWardenForce mirrors HandleDebugWardenForce
// (cs_debug.cpp:1666): an empty check list returns false, which the
// C++ framework answers with the node's help; Go has no Warden, so
// a non-empty list always reports the system disabled. Check ids
// parse as uint16 (ChatCommandArgs.h:62); a bad token is a framework
// parse failure (LANG_CMDPARSER_* + SetSentErrorMessage), so it
// messages and returns true.
func (s *session) handleDebugWardenForce(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		v, ok := parseCommandUint32(arg)
		if !ok || v > 0xFFFF {
			// Unparseable/out-of-range typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid check ID.")
			return true
		}
	}
	s.sendSysMessage("Warden system is not enabled")
	return true
}

// handleDebugSendSpellFail mirrors HandleDebugSendSpellFailCommand
// (cs_debug.cpp:221): ".debug send spellfail <result> [failArg1] [failArg2]"
// writes SMSG_CAST_FAILED with cast id 0 and spell 133 (Fireball) through
// the buildCastFailedParams funnel, which appends the optional extended
// params exactly as C++ appends failArg1/failArg2. DELTA: the result arg
// parses as a numeric SpellCastResult; C++'s enum-name form has no Go
// command parser. The remaining send arms are wired in handleDebugSend,
// including opcode.
func (s *session) handleDebugSendSpellFail(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug send spellfail <result> [failArg1] [failArg2]"
	if len(args) == 0 || len(args) > 3 {
		return false
	}
	result, ok := parseCommandUint32(args[0])
	if !ok || (result > 187 && result != 255) {
		// Unparseable/out-of-set enum token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	var params []uint32
	for _, arg := range args[1:] {
		v, ok := parseCommandUint32(arg)
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		params = append(params, v)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailedParams(0, 133, uint8(result), params...), true)
	return true
}

// handleDebugSend mirrors the debugSendCommandTable (cs_debug.cpp:70-82):
// spellfail, buyerror, sellerror, equiperror, channelnotify, chatmessage,
// qpartymsg, qinvalidmsg, setphaseshift, largepacket and opcode — the
// whole subtable. The debug arms
// pass nil creatures/items, so the error packets carry empty GUIDs and
// zero item/param fields exactly as C++ does; numeric args stand in for
// C++'s enum-name forms, matching the spellfail precedent. Unknown
// sub-arms report the syntax rather than silently succeeding.
func (s *session) handleDebugSend(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug send spellfail|buyerror|sellerror|equiperror|channelnotify|chatmessage|qpartymsg|qinvalidmsg|setphaseshift|largepacket|opcode <arg>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	switch strings.ToLower(args[0]) {
	case "spellfail":
		return s.handleDebugSendSpellFail(ctx, args[1:])
	case "opcode":
		if len(args) != 1 {
			return false
		}
		return s.handleDebugSendOpcode(ctx)
	case "buyerror", "sellerror", "equiperror", "channelnotify", "chatmessage", "qpartymsg", "qinvalidmsg":
		if len(args) != 2 {
			return false
		}
		value, ok := parseCommandUint32(args[1])
		if !ok || !debugSendEnumValid(strings.ToLower(args[0]), value) {
			// Unparseable/out-of-set enum token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		s.debugSendNumeric(strings.ToLower(args[0]), value)
		return true
	case "setphaseshift":
		if len(args) != 2 {
			return false
		}
		value, ok := parseCommandUint32(args[1])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		s.debugSendNumeric("setphaseshift", value)
		return true
	case "largepacket":
		if len(args) != 1 {
			return false
		}
		var sb strings.Builder
		for sb.Len() < 128000 {
			sb.WriteString("This is a dummy string to push the packet's size beyond 128000 bytes. ")
		}
		s.sendSysMessage(sb.String())
		return true
	default:
		s.sendSysMessage(syntax)
		return true
	}
}

// debugSendNumeric writes one numeric-arg send arm. The packet layouts
// are the Player::Send* twins with the debug call's nil arguments:
// SendBuyError (Player.cpp:13635) writes creature GUID 0, item 0 and the
// result byte (param 0 omitted); SendSellError (Player.cpp:13646) writes
// creature GUID 0, item GUID 0 and the result byte; SendEquipError
// (Player.cpp:13592) writes the result byte plus empty item GUIDs, the
// bag-type byte and, for the level/limit-category arms, a zero template
// field (no item, so no template lookup) and, for the autoequip-bind
// confirm arm, zero guid/slot/container. This writes directly rather
// than through sendEquipError so the debug packet is never swallowed by
// the session's equip-error suppression, as C++ sends it unconditionally.
func (s *session) debugSendNumeric(arm string, value uint32) {
	switch arm {
	case "buyerror":
		_ = s.write(uint16(protocol.OpcodeSMSG_BUY_FAILED), buildBuyFailed(0, 0, uint8(value)), true)
	case "sellerror":
		_ = s.write(uint16(protocol.OpcodeSMSG_SELL_ITEM), buildSellResult(0, 0, uint8(value)), true)
	case "equiperror":
		buf := protocol.NewBuffer(34)
		buf.WriteU8(uint8(value))
		if value != 0 {
			buf.WriteU64(0)
			buf.WriteU64(0)
			buf.WriteU8(0)
			switch value {
			case 1, 87:
				buf.WriteU32(0)
			case 81:
				buf.WriteU64(0)
				buf.WriteU32(0)
				buf.WriteU64(0)
			case 84, 85, 89:
				buf.WriteU32(0)
			}
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_INVENTORY_CHANGE_FAILURE), buf.Bytes(), true)
	case "channelnotify":
		buf := protocol.NewBuffer(15)
		buf.WriteU8(uint8(value))
		buf.WriteCString("test")
		buf.WriteU32(0)
		buf.WriteU32(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_CHANNEL_NOTIFY), buf.Bytes(), true)
	case "chatmessage":
		packet := protocol.BuildChatMessage(uint8(value), languageUniversal, s.playerGUID, s.playerGUID, "testtest", "chan")
		_ = s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
	case "qpartymsg":
		s.sendPushToPartyResponse(s.playerGUID, uint8(value))
	case "qinvalidmsg":
		s.sendQuestGiverQuestInvalid(value)
	case "setphaseshift":
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_PHASE_SHIFT), buildPhaseShift(value), true)
	}
}

// stripOpcodeComments removes the // line and /* */ block comments that
// HandleDebugSendOpcodeCommand strips from opcode.txt before tokenizing,
// keeping every other byte so the token stream matches C++'s.
func stripOpcodeComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] == '/' && i+1 < len(data) {
			if data[i+1] == '*' {
				end := i + 2
				for end+1 < len(data) && !(data[end] == '*' && data[end+1] == '/') {
					end++
				}
				if end+1 < len(data) {
					i = end + 2
				} else {
					i = len(data)
				}
				continue
			}
			if data[i+1] == '/' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
				continue
			}
		}
		out = append(out, data[i])
		i++
	}
	return out
}

// handleDebugSendOpcode mirrors HandleDebugSendOpcodeCommand
// (cs_debug.cpp:261): ".debug send opcode" replays the server-side
// opcode.txt file — first token the opcode, then type/value pairs
// (uint8/uint16/uint32/uint64/float/string) or guid/position tokens
// (appitsguid/appmyguid/appgoguid/goguid/myguid/itsguid/itspos/mypos) —
// to the selected player, or the invoker when the selection is not a
// player. The selected unit falls back to the invoker when nothing is
// selected, as getSelectedUnit does. The nearest gameobject within one
// grid (SIZE_OF_GRIDS) supplies the goguid legs; a miss reports
// OBJNOTFOUND and sends nothing, as C++ does. DELTAS: a missing
// opcode.txt returns false silently, the C++ false return; unknown types
// stop the token loop but the packet still sends, as C++ logs and breaks
// before sending.
func (s *session) handleDebugSendOpcode(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	raw, err := os.ReadFile("opcode.txt")
	if err != nil {
		return false
	}
	tokens := strings.Fields(string(stripOpcodeComments(raw)))
	if len(tokens) == 0 {
		return false
	}
	opcode, err := strconv.ParseUint(tokens[0], 10, 32)
	if err != nil {
		return false
	}
	unitName, unitGUID, ux, uy, uz, _, _ := s.debugSelectedUnit()
	target := s
	playerGUID := s.playerGUID
	if s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil {
			target = ts
			playerGUID = ts.playerGUID
		}
	}
	var goGUID uint64
	needGO := false
	for _, tok := range tokens[1:] {
		if tok == "appgoguid" || tok == "goguid" {
			needGO = true
		}
	}
	if needGO && s.player != nil {
		best := float32(-1)
		s.scanNearbyGameObjects(ctx, 533.3333, func(g nearbyGameObject) {
			dx, dy, dz := g.x-s.player.X, g.y-s.player.Y, g.z-s.player.Z
			d := dx*dx + dy*dy + dz*dz
			if best < 0 || d < best {
				best = d
				goGUID = g.guid
			}
		})
		if goGUID == 0 {
			s.sendSysMessage("Gameobject (GUID: 0) could not be found.")
			return true
		}
	}
	buf := protocol.NewBuffer(64)
	i := 1
	for i < len(tokens) {
		typ := tokens[i]
		i++
		next := func() (string, bool) {
			if i >= len(tokens) {
				return "", false
			}
			v := tokens[i]
			i++
			return v, true
		}
		switch typ {
		case "uint8", "uint16", "uint32", "uint64", "float", "string":
			v, ok := next()
			if !ok {
				break
			}
			switch typ {
			case "uint8":
				n, err := strconv.ParseUint(v, 10, 16)
				if err != nil {
					break
				}
				buf.WriteU8(uint8(n))
			case "uint16":
				n, err := strconv.ParseUint(v, 10, 16)
				if err != nil {
					break
				}
				buf.WriteU16(uint16(n))
			case "uint32":
				n, err := strconv.ParseUint(v, 10, 32)
				if err != nil {
					break
				}
				buf.WriteU32(uint32(n))
			case "uint64":
				n, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					break
				}
				buf.WriteU64(n)
			case "float":
				n, err := strconv.ParseFloat(v, 32)
				if err != nil {
					break
				}
				buf.WriteF32(float32(n))
			case "string":
				buf.WriteCString(v)
			}
		case "appitsguid":
			buf.WritePackedGUID(unitGUID)
		case "appmyguid":
			buf.WritePackedGUID(playerGUID)
		case "appgoguid":
			buf.WritePackedGUID(goGUID)
		case "goguid":
			buf.WriteU64(goGUID)
		case "myguid":
			buf.WriteU64(playerGUID)
		case "itsguid":
			buf.WriteU64(unitGUID)
		case "itspos":
			buf.WriteF32(ux)
			buf.WriteF32(uy)
			buf.WriteF32(uz)
		case "mypos":
			if s.player != nil {
				buf.WriteF32(s.player.X)
				buf.WriteF32(s.player.Y)
				buf.WriteF32(s.player.Z)
			} else {
				buf.WriteF32(0)
				buf.WriteF32(0)
				buf.WriteF32(0)
			}
		default:
			i = len(tokens)
		}
	}
	_ = target.write(uint16(opcode), buf.Bytes(), true)
	s.sendSysMessage(fmt.Sprintf("Opcode %d sent to %s.", opcode, unitName))
	return true
}

// handleDebugLootRecipient mirrors HandleDebugGetLootRecipientCommand
// (cs_debug.cpp:473): ".debug lootrecipient" reports the selected
// creature's tap model (creatureMotion TapPlayerGUID/TapGroupID, the
// Creature::SetLootRecipient twin): no tap reads "no loot recipient", a
// tapped player online reads its name, a tapped player offline reads
// "offline". The GUID counter and spawn id share the npc-info precedent
// (GUID & 0x00FFFFFF); a group-only tap has no tracked player GUID, so it
// reads "offline" like an unresolvable recipient.
func (s *session) handleDebugLootRecipient(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		return false
	}
	counter := uint32(motion.GUID & 0x00FFFFFF)
	recipient := "no loot recipient"
	if motion.TapPlayerGUID != 0 || motion.TapGroupID != 0 {
		recipient = "offline"
		if ts := s.server.playerSessionForGUID(motion.TapPlayerGUID); ts != nil && ts.player != nil {
			recipient = ts.player.Name
		}
	}
	s.sendSysMessage(fmt.Sprintf("Loot recipient for creature %s (GUID %d, SpawnID %d) is %s", motion.Name, counter, counter, recipient))
	return true
}

// debugInstanceSpawnTuple is one InstanceSpawnGroupInfo row as
// HandleDebugInstanceSpawns (cs_debug.cpp:1540) stores it: the spawn/block
// flag pair collapsed to isSpawn, the boss-state id, and the state mask.
type debugInstanceSpawnTuple struct {
	isSpawn     bool
	bossStateID uint8
	bossStates  uint8
}

// handleDebugInstanceSpawns mirrors HandleDebugInstanceSpawns
// (cs_debug.cpp:1525): ".debug instancespawn <groupId|explain>". Rows come
// from the instance_spawn_groups cache (instance_boss_states.go) for the
// invoker's map, boss states from the per-instance boss-state store
// (unset reads TO_BE_DECIDED), group names from the spawn-group manager,
// and the active flag from Map::IsSpawnGroupActive's twin. C++-exact
// quirks kept: rows carrying neither flag are skipped, tuples sort as a
// std::set (isSpawn false first, then boss id, then mask), and the
// allowed-spawn line names the boss-state id, not the actual state, as
// the C++ text does. Deltas: Go has no per-instance script object, so the
// "has no instance script" and "manages no groups" gates collapse into the
// empty-rows message; a missing world DB reports unavailable rather than
// the C++ script gates.
func (s *session) handleDebugInstanceSpawns(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if s.server == nil || s.player == nil {
		return false
	}
	const syntax = "Syntax: .debug instancespawn <groupId|explain>"
	if len(args) != 1 {
		return false
	}
	explain := strings.EqualFold(args[0], "explain")
	var groupID uint32
	if !explain {
		id, ok := parseCommandUint32(args[0])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		groupID = id
	}
	mapID, instanceID := s.player.Map, s.player.InstanceID
	mapName := fmt.Sprintf("Map %d", mapID)
	if s.server.Data != nil {
		if entry, found, err := s.server.Data.Map(mapID); err == nil && found && entry.MapName != "" {
			mapName = entry.MapName
		}
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Instance spawn group data is unavailable.")
		return true
	}
	if err := s.server.ensureInstanceSpawnGroups(ctx, s.server.WorldStore.DB); err != nil {
		s.sendSysMessage("Instance spawn group data is unavailable.")
		return true
	}
	s.server.instanceSpawnGroupMu.RLock()
	rows := append([]instanceSpawnGroupInfo(nil), s.server.instanceSpawnGroups[mapID]...)
	s.server.instanceSpawnGroupMu.RUnlock()
	s.server.spawnGroupMu.RLock()
	templates := make(map[uint32]*spawnGroupTemplate, len(s.server.spawnGroupTemplates))
	for id, tmpl := range s.server.spawnGroupTemplates {
		templates[id] = tmpl
	}
	s.server.spawnGroupMu.RUnlock()
	if groupID != 0 {
		if templates[groupID] == nil {
			s.sendSysMessage(fmt.Sprintf("There is no spawn group with ID %d.", groupID))
			return true
		}
	}
	store := map[uint32][]debugInstanceSpawnTuple{}
	for _, info := range rows {
		if groupID != 0 && info.SpawnGroupID != groupID {
			continue
		}
		var isSpawn bool
		switch {
		case info.Flags&instanceSpawnGroupFlagBlockSpawn != 0:
			isSpawn = false
		case info.Flags&instanceSpawnGroupFlagActivateSpawn != 0:
			isSpawn = true
		default:
			continue
		}
		store[info.SpawnGroupID] = append(store[info.SpawnGroupID], debugInstanceSpawnTuple{isSpawn: isSpawn, bossStateID: info.BossStateID, bossStates: info.BossStates})
	}
	if groupID != 0 && len(store[groupID]) == 0 {
		s.sendSysMessage(fmt.Sprintf("%s's instance script does not manage group '%s'.", mapName, templates[groupID].Name))
		return true
	}
	if len(store) == 0 {
		s.sendSysMessage(fmt.Sprintf("%s's instance script does not manage any spawn groups.", mapName))
		return true
	}
	ids := make([]uint32, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if groupID == 0 {
		s.sendSysMessage(fmt.Sprintf("Spawn groups managed by %s (%d):", mapName, mapID))
	}
	for _, id := range ids {
		tmpl := templates[id]
		if tmpl == nil {
			continue
		}
		if !explain {
			active := ""
			if !s.server.spawnGroupIsActive(mapID, instanceID, tmpl) {
				active = "not "
			}
			s.sendSysMessage(fmt.Sprintf(" - '%s' (%d) is %sactive", tmpl.Name, id, active))
			continue
		}
		s.sendSysMessage(fmt.Sprintf(" |-- '%s' (%d)", tmpl.Name, id))
		tuples := store[id]
		sort.Slice(tuples, func(i, j int) bool {
			if tuples[i].isSpawn != tuples[j].isSpawn {
				return !tuples[i].isSpawn
			}
			if tuples[i].bossStateID != tuples[j].bossStateID {
				return tuples[i].bossStateID < tuples[j].bossStateID
			}
			return tuples[i].bossStates < tuples[j].bossStates
		})
		isBlocked, isSpawned := false, false
		for _, tuple := range tuples {
			actualState := s.server.instanceBossState(mapID, instanceID, uint32(tuple.bossStateID))
			if uint32(1)<<actualState&uint32(tuple.bossStates) != 0 {
				if tuple.isSpawn {
					isSpawned = true
					if isBlocked {
						s.sendSysMessage(fmt.Sprintf(" | |-- '%s' would be allowed to spawn by boss state %d being %s, but this is overruled", tmpl.Name, tuple.bossStateID, encounterStateName(actualState)))
					} else {
						s.sendSysMessage(fmt.Sprintf(" | |-- '%s' is allowed to spawn because boss state %d is %s.", tmpl.Name, tuple.bossStateID, encounterStateName(tuple.bossStateID)))
					}
				} else {
					isBlocked = true
					s.sendSysMessage(fmt.Sprintf(" | |-- '%s' is blocked from spawning because boss state %d is %s.", tmpl.Name, tuple.bossStateID, encounterStateName(actualState)))
				}
			} else {
				verb := "blocked from spawning"
				if tuple.isSpawn {
					verb = "allowed to spawn"
				}
				s.sendSysMessage(fmt.Sprintf(" | |-- '%s' could've been %s if boss state %d matched mask 0x%02x; but it is %s -> 0x%02x, which does not match.", tmpl.Name, verb, tuple.bossStateID, tuple.bossStates, encounterStateName(actualState), uint32(1)<<actualState))
			}
		}
		switch {
		case isBlocked:
			s.sendSysMessage(fmt.Sprintf(" | |=> '%s' is not active due to a blocking rule being matched", tmpl.Name))
		case isSpawned:
			s.sendSysMessage(fmt.Sprintf(" | |=> '%s' is active due to a spawn rule being matched", tmpl.Name))
		default:
			s.sendSysMessage(fmt.Sprintf(" | |=> '%s' is not active due to none of its rules being matched", tmpl.Name))
		}
	}
	return true
}

// debugThreatTarget resolves the HandleDebugThreatListCommand target
// (cs_debug.cpp:769): the selected unit, else the invoker. A selected
// creature rides its motion; a selected player (or no selection) rides the
// player session. The engaged flag is the motion InCombat bit for
// creatures and the UNIT_FLAG_IN_COMBAT bit for players.
func (s *session) debugThreatTarget() (name string, guid uint64, alive, engaged bool, motion *creatureMotion) {
	if s == nil || s.player == nil {
		return "", 0, false, false, nil
	}
	if m := s.debugSelectedCreatureMotion(); m != nil {
		return m.Name, m.GUID, m.Health != 0, m.InCombat, m
	}
	if s.selection != 0 {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts.player.Name, ts.playerGUID, ts.player.Health != 0, ts.player.UnitFlags&unitFlagInCombat != 0, nil
		}
	}
	return s.player.Name, s.playerGUID, s.player.Health != 0, s.player.UnitFlags&unitFlagInCombat != 0, nil
}

// debugUnitName resolves a threat victim's display name: live creature
// motions first, then online players, else the bare GUID text of the C++
// fallback (ThreatReference rows print the victim name; an offline victim
// has no resolvable name in the Go session model).
func (s *session) debugUnitName(guid uint64) string {
	if s == nil || s.server == nil {
		return fmt.Sprintf("%d", guid)
	}
	s.server.motionMu.Lock()
	for _, motions := range s.server.instanceCreatureMotion {
		if m := motions[guid]; m != nil {
			name := m.Name
			s.server.motionMu.Unlock()
			return name
		}
	}
	s.server.motionMu.Unlock()
	if ts := s.server.playerSessionForGUID(guid); ts != nil && ts.player != nil {
		return ts.player.Name
	}
	return fmt.Sprintf("%d", guid)
}

// debugThreatenedByMe scans every live creature threat list for entries on
// the target: the ThreatManager::_threatenedByMe reverse list of
// HandleDebugThreatListCommand has no Go index, so the debug report rebuilds
// it from the forward lists. Owner name, counter and threat come from the
// owning motion's snapshot.
func (s *session) debugThreatenedByMe(targetGUID uint64) []debugThreatenedRow {
	type owner struct {
		name    string
		counter uint32
		threat  float32
	}
	if s == nil || s.server == nil {
		return nil
	}
	var owners []owner
	s.server.motionMu.Lock()
	for _, motions := range s.server.instanceCreatureMotion {
		for _, m := range motions {
			if m == nil || m.ThreatMgr == nil {
				continue
			}
			for _, e := range m.ThreatMgr.DebugEntries() {
				if e.victimGUID == targetGUID {
					owners = append(owners, owner{m.Name, uint32(m.GUID & 0x00FFFFFF), e.threat})
					break
				}
			}
		}
	}
	s.server.motionMu.Unlock()
	rows := make([]debugThreatenedRow, 0, len(owners))
	for _, o := range owners {
		rows = append(rows, debugThreatenedRow{name: o.name, counter: o.counter, threat: o.threat})
	}
	return rows
}

type debugThreatenedRow struct {
	name    string
	counter uint32
	threat  float32
}

// handleDebugThreat mirrors HandleDebugThreatListCommand (cs_debug.cpp:769):
// ".debug threat" prints the target's threatened-by-me list, then its
// threat list (sorted descending, [FIXATE]/[TAUNT]/[DETAUNT]/
// [SUPPRESSED]/[OFFLINE] markers) or the cannot-have-list / not-engaged
// legs. Deltas: no threatened-by-me index (rebuilt by scan, so only live
// creature lists contribute).
func (s *session) handleDebugThreat(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	name, guid, alive, engaged, motion := s.debugThreatTarget()
	counter := uint32(guid & 0x00FFFFFF)
	if !alive {
		engagedNote := ""
		if engaged {
			engagedNote = " (It is, however, engaged. Huh?)"
		}
		s.sendSysMessage(fmt.Sprintf("%s (GUID %d) is not alive.%s", name, counter, engagedNote))
		return true
	}
	threatened := s.debugThreatenedByMe(guid)
	if len(threatened) == 0 {
		s.sendSysMessage(fmt.Sprintf("%s (GUID %d) does not threaten any units.", name, counter))
	} else {
		s.sendSysMessage(fmt.Sprintf("List of units threatened by %s (GUID %d)", name, counter))
		for i, row := range threatened {
			s.sendSysMessage(fmt.Sprintf("   %d.   %s   (GUID %d, SpawnID %d)  - threat %f", i+1, row.name, row.counter, row.counter, row.threat))
		}
		s.sendSysMessage("End of threatened-by-me list.")
	}
	if motion == nil || !motionCanHaveThreatList(motion) {
		if engaged {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d) is currently engaged. (This unit cannot have a threat list.)", name, counter))
		} else {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d) is not currently engaged. (This unit cannot have a threat list.)", name, counter))
		}
		return true
	}
	entries := motion.ThreatMgr.DebugEntries()
	if len(entries) == 0 {
		if !engaged {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d, SpawnID %d) is not currently engaged.", name, counter, counter))
		} else {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d, SpawnID %d) seems to be engaged, but does not have a threat list??", name, counter, counter))
		}
		return true
	}
	fixate := motion.ThreatMgr.GetFixateTarget()
	if engaged {
		s.sendSysMessage(fmt.Sprintf("Threat list of %s (GUID %d, SpawnID %d):", name, counter, counter))
	} else {
		s.sendSysMessage(fmt.Sprintf("%s (GUID %d, SpawnID %d) is not engaged, but still has a threat list? Well, here it is:", name, counter, counter))
	}
	for i, e := range entries {
		marker := ""
		if e.victimGUID == fixate {
			marker = " [FIXATE]"
		} else if e.taunted {
			marker = " [TAUNT]"
		} else if e.detaunted {
			marker = " [DETAUNT]"
		}
		if e.suppressed {
			marker += " [SUPPRESSED]"
		}
		if e.offline {
			marker += " [OFFLINE]"
		}
		s.sendSysMessage(fmt.Sprintf("   %d.   %s   (GUID %d)  - threat %f%s", i+1, s.debugUnitName(e.victimGUID), uint32(e.victimGUID&0x00FFFFFF), e.threat, marker))
	}
	s.sendSysMessage("End of threat list.")
	return true
}

// handleDebugThreatInfo mirrors HandleDebugThreatInfoCommand
// (cs_debug.cpp:854): ".debug threatinfo" reports the selected unit, else
// the invoker (getSelectedUnit, Chat.cpp:312 — the no-selection error leg
// is unreachable in-game), and prints the modeled threat-manager state.
// Deltas: the single-school and
// multi-school modifier tables and the redirect info/registry have no Go
// model (ThreatManager tracks only the temp modifier per victim), so the
// report prints the modeled rows — fixate target, live taunts,
// suppressions, offline flags and per-victim temp modifiers — and
// says so for the rest.
func (s *session) handleDebugThreatInfo(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	name, guid, _, _, motion := s.debugThreatTarget()
	if guid == 0 {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	var mgr *ThreatManager
	if motion != nil {
		mgr = motion.ThreatMgr
	}
	s.sendSysMessage(fmt.Sprintf("Threat info for %s (%d):", name, guid))
	s.sendSysMessage(" - Single-school threat modifiers: not modeled")
	s.sendSysMessage("- Multi-school threat modifiers (0 entries): not modeled")
	s.sendSysMessage(" - No redirects being applied (redirects not modeled)")
	s.sendSysMessage(" - No redirects are registered (redirects not modeled)")
	if mgr == nil {
		return true
	}
	fixate := mgr.GetFixateTarget()
	if fixate != 0 {
		s.sendSysMessage(fmt.Sprintf(" - Fixate target: %s", s.debugUnitName(fixate)))
	}
	for _, e := range mgr.DebugEntries() {
		if e.taunted {
			s.sendSysMessage(fmt.Sprintf(" - Taunted victim: %s", s.debugUnitName(e.victimGUID)))
		}
		if e.detaunted {
			s.sendSysMessage(fmt.Sprintf(" - Detaunted victim: %s", s.debugUnitName(e.victimGUID)))
		}
		if e.suppressed {
			s.sendSysMessage(fmt.Sprintf(" - Suppressed victim: %s", s.debugUnitName(e.victimGUID)))
		}
		if e.offline {
			s.sendSysMessage(fmt.Sprintf(" - Offline victim: %s", s.debugUnitName(e.victimGUID)))
		}
	}
	return true
}

// handleDebugCombat mirrors HandleDebugCombatListCommand (cs_debug.cpp:929):
// ".debug combat" prints the target's combat state line. Delta: Go has no
// combat-reference list (only the InCombat/UnitFlags bits), so no [PvP] /
// [PvE] ref rows follow the header.
func (s *session) handleDebugCombat(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	_, _, _, engaged, _ := s.debugThreatTarget()
	combatState := 0
	if engaged {
		combatState = 1
	}
	s.sendSysMessage(fmt.Sprintf("Combat refs: (Combat state: %d | Manager state: %d)", combatState, combatState))
	return true
}

// handleDebugGuidLimits mirrors HandleDebugGuidLimitsCommand plus
// HandleDebugGuidLimitsMap (cs_debug.cpp:1683, 1698): ".debug guidlimits
// [mapId]" prints, per (map, instance), the highest live creature and
// gameobject GUID counters, reusing the objectcount map iteration. Delta:
// the GUID warn/alert config levels have no Go config, so those two lines
// are omitted; counters cover live motions and runtime gameobjects only,
// not the DB spawn tables.
func (s *session) handleDebugGuidLimits(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	var mapFilter *uint32
	if len(args) > 1 {
		return false
	}
	if len(args) > 0 {
		id, ok := parseCommandUint32(args[0])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid map ID.")
			return true
		}
		v := id
		mapFilter = &v
	}
	if s.server == nil || s.player == nil {
		return false
	}
	type guidMax struct {
		mapID      uint32
		instanceID uint32
		creature   uint64
		gameobject uint64
	}
	maxima := map[instanceAdmissionKey]*guidMax{}
	get := func(mapID, instanceID uint32) *guidMax {
		key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
		if maxima[key] == nil {
			maxima[key] = &guidMax{mapID: mapID, instanceID: instanceID}
		}
		return maxima[key]
	}
	s.server.motionMu.Lock()
	for key, motions := range s.server.instanceCreatureMotion {
		if mapFilter != nil && key.MapID != *mapFilter {
			continue
		}
		m := get(key.MapID, key.InstanceID)
		for guid := range motions {
			if c := uint64(uint32(guid & 0x00FFFFFF)); c > m.creature {
				m.creature = c
			}
		}
	}
	s.server.motionMu.Unlock()
	s.server.objectsMu.RLock()
	collectGO := func(state *dynamicGameObjectState) {
		if state == nil {
			return
		}
		if mapFilter != nil && state.Map != *mapFilter {
			return
		}
		m := get(state.Map, state.InstanceID)
		if c := uint64(uint32(state.GUID & 0x00FFFFFF)); c > m.gameobject {
			m.gameobject = c
		}
	}
	for _, state := range s.server.dynamicGameObjects {
		collectGO(state)
	}
	for _, byGUID := range s.server.instanceGameObjects {
		for _, state := range byGUID {
			collectGO(state)
		}
	}
	s.server.objectsMu.RUnlock()
	if mapFilter == nil || *mapFilter == s.player.Map {
		get(s.player.Map, s.player.InstanceID)
	}
	keys := make([]instanceAdmissionKey, 0, len(maxima))
	for key := range maxima {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].MapID != keys[j].MapID {
			return keys[i].MapID < keys[j].MapID
		}
		return keys[i].InstanceID < keys[j].InstanceID
	})
	for _, key := range keys {
		m := maxima[key]
		mapName := ""
		if s.server.Data != nil {
			if entry, found, err := s.server.Data.Map(m.mapID); err == nil && found {
				mapName = entry.MapName
			}
		}
		s.sendSysMessage(fmt.Sprintf("Map Id: %d Name: '%s' Instance Id: %d Highest Guid Creature: %d GameObject: %d",
			m.mapID, mapName, m.instanceID, m.creature, m.gameobject))
	}
	return true
}

// handleDebugWPGPS mirrors HandleWPGPSCommand (cs_debug.cpp:1311):
// ".debug wpgps" logs the invoker's position as a waypoint SQL row and
// reports that the row was written. Go has no sql.dev log file, so the row
// goes to the session debug log instead.
func (s *session) handleDebugWPGPS(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if s.player == nil {
		return true
	}
	s.debug("waypoint sql", "row", fmt.Sprintf("(@PATH, XX, %.3f, %.3f, %.5f, %.5f, 0, 0, 0, 100, 0),", s.player.X, s.player.Y, s.player.Z, s.player.Orientation))
	s.sendSysMessage("Waypoint SQL written to SQL Developer log")
	return true
}

// handleDebugTransport mirrors HandleDebugTransportCommand
// (cs_debug.cpp:1317): ".debug transport <stop|start|other>" on the
// invoker's continent transport; the operation arg is required, so a bare
// ".debug transport" fails instead of printing info. The stop/start legs
// ride the MovementPaused latch (EnableMovement twin); any other argument
// prints the transport's name, state and position. Delta: C++ defers the
// stop to the next stop frame (Transport _pendingStop), Go pauses at once.
// No transport returns false silently, the C++ false return (no message).
func (s *session) handleDebugTransport(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if len(args) != 1 {
		return false
	}
	if s.server == nil || s.player == nil || s.player.TransportGUID == 0 {
		return false
	}
	var transport *continentTransport
	s.server.transportMu.Lock()
	for _, t := range s.server.transports {
		if t == nil {
			continue
		}
		if s.player.TransportGUID == uint64(t.Spawn.GUID) || s.player.TransportGUID == transportGUID(t.Spawn.GUID) || s.player.TransportGUID == gameObjectGUID(t.Spawn.GUID, t.Spawn.Entry) {
			transport = t
			break
		}
	}
	if transport != nil {
		transportName, paused, x, y, z, o := transport.Name, transport.MovementPaused, transport.Spawn.X, transport.Spawn.Y, transport.Spawn.Z, transport.Spawn.Orientation
		op := strings.ToLower(args[0])
		if op == "stop" || op == "start" {
			transport.MovementPaused = op == "stop"
			paused = transport.MovementPaused
			s.server.transportMu.Unlock()
			if op == "stop" {
				s.sendSysMessage(fmt.Sprintf("Transport %s stopped", transportName))
			} else {
				s.sendSysMessage(fmt.Sprintf("Transport %s started", transportName))
			}
			return true
		}
		s.server.transportMu.Unlock()
		state := "moving"
		if paused {
			state = "stopped"
		}
		s.sendSysMessage(fmt.Sprintf("Transport %s is %s", transportName, state))
		s.sendSysMessage(fmt.Sprintf("Transport position: X: %f Y: %f Z: %f O: %f", x, y, z, o))
		return true
	}
	s.server.transportMu.Unlock()
	return false
}

// handleDebugBattleground mirrors HandleDebugBattlegroundCommand
// (cs_debug.cpp:757): BattlegroundMgr::ToggleTesting flips m_Testing and
// broadcasts the ON/OFF world text (SendWorldText). The LANG 739/740 texts
// are unseeded in-tree, so plain prose stands in like the other unseeded
// LANG_* notices; the queue consumers have no Go twin yet (see the
// bgTesting field note in server.go).
func (s *session) handleDebugBattleground(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if s.server == nil {
		return true
	}
	s.server.bgTestingMu.Lock()
	s.server.bgTesting = !s.server.bgTesting
	on := s.server.bgTesting
	s.server.bgTestingMu.Unlock()
	if on {
		s.server.broadcastMessageChatAll("Battleground debug mode enabled.")
	} else {
		s.server.broadcastMessageChatAll("Battleground debug mode disabled.")
	}
	return true
}

// handleDebugArena mirrors HandleDebugArenaCommand (cs_debug.cpp:763):
// BattlegroundMgr::ToggleArenaTesting flips m_ArenaTesting and broadcasts
// the ON/OFF world text (SendWorldText). LANG 737/738 prose and the
// missing queue consumer match the battleground arm above.
func (s *session) handleDebugArena(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	if s.server == nil {
		return true
	}
	s.server.bgTestingMu.Lock()
	s.server.arenaTesting = !s.server.arenaTesting
	on := s.server.arenaTesting
	s.server.bgTestingMu.Unlock()
	if on {
		s.server.broadcastMessageChatAll("Arena debug mode enabled.")
	} else {
		s.server.broadcastMessageChatAll("Arena debug mode disabled.")
	}
	return true
}

// Quest-reset world states (SharedDefines.h:3867-3872).
const (
	worldStateWeeklyQuestResetTime  uint32 = 20002
	worldStateMonthlyQuestResetTime uint32 = 20007
	worldStateDailyQuestResetTime   uint32 = 20008
)

// nextWeeklyQuestResetTime mirrors World::GetNextWeeklyResetTime
// (World.cpp:3263-3273): the next daily reset, advanced to the weekly
// reset weekday. CONFIG_WEEKLY_QUEST_RESET_TIME_WDAY defaults to 3
// (Wednesday, World.cpp:989); Go keeps no such config field, so 3 is
// fixed. C++ tm_wday and Go Weekday both count Sunday as 0.
func nextWeeklyQuestResetTime(now time.Time) time.Time {
	next := nextDailyQuestResetTime(now)
	wday := int(next.Weekday())
	target := 3
	if target < wday {
		wday -= 7
	}
	return next.Add(time.Duration(target-wday) * 24 * time.Hour)
}

// nextMonthlyQuestResetTime mirrors World::GetNextMonthlyResetTime
// (World.cpp:3299-3309): the next daily reset, moved to the 1st of the
// next month unless it already falls on the 1st. Go's time.Date
// normalizes the December overflow exactly as C++ mktime does.
func nextMonthlyQuestResetTime(now time.Time) time.Time {
	next := nextDailyQuestResetTime(now)
	if next.Day() == 1 {
		return next
	}
	return time.Date(next.Year(), next.Month()+1, 1, next.Hour(), next.Minute(), next.Second(), 0, next.Location())
}

// debugQuestResetTimeText mirrors TimeToHumanReadable (Util.cpp:285-292):
// localtime strftime "%c". Go has no %c verb, so the closest fixed layout
// stands in.
func debugQuestResetTimeText(t time.Time) string {
	return t.Format("Mon Jan 2 15:04:05 2006")
}

// handleDebugQuestReset mirrors HandleDebugQuestResetCommand
// (cs_debug.cpp:1454): the upper-cased argument selects daily, weekly,
// monthly or all three; anything else fails. Each leg runs its
// World::Reset*Quests twin: the character_queststatus_* table is emptied,
// online sessions clear their in-memory quest sets, and the next reset
// time is stored as a world state and reported. The quest-pool reselect
// (sQuestPoolMgr->ChangeDailyQuests and kin) has no Go model, and the
// dungeon-finder quest clear inside ResetDailyQuestStatus has none
// either; both stay unmodeled, not fabricated.
func (s *session) handleDebugQuestReset(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug questreset <all|daily|weekly|monthly>"
	if len(args) != 1 {
		return false
	}
	var daily, weekly, monthly bool
	switch strings.ToUpper(args[0]) {
	case "ALL":
		daily, weekly, monthly = true, true, true
	case "DAILY":
		daily = true
	case "WEEKLY":
		weekly = true
	case "MONTHLY":
		monthly = true
	default:
		return false
	}
	if daily {
		s.debugResetDailyQuests(ctx)
	}
	if weekly {
		s.debugResetWeeklyQuests(ctx)
	}
	if monthly {
		s.debugResetMonthlyQuests(ctx)
	}
	return true
}

// debugResetQuestSessions collects the online sessions whose in-memory
// quest state a reset leg clears, mirroring World::Reset*Quests' session
// loop (World.cpp:3245-3247 and kin).
func (s *session) debugResetQuestSessions() []*session {
	if s.server == nil {
		return nil
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	targets := make([]*session, 0, len(s.server.sessions))
	for sess := range s.server.sessions {
		if sess != nil && sess.player != nil {
			targets = append(targets, sess)
		}
	}
	return targets
}

func (s *session) debugResetDailyQuests(ctx context.Context) {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_queststatus_daily")
	}
	for _, sess := range s.debugResetQuestSessions() {
		sess.player.DailyQuests = [playerDailyQuestsCount]uint32{}
		sess.player.LastDailyQuestTime = 0
	}
	next := nextDailyQuestResetTime(time.Now())
	if s.server != nil {
		s.server.setWorldState(ctx, worldStateDailyQuestResetTime, uint64(next.Unix()))
	}
	s.sendSysMessage(fmt.Sprintf("Daily quests have been reset. Next scheduled reset: %s", debugQuestResetTimeText(next)))
}

func (s *session) debugResetWeeklyQuests(ctx context.Context) {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_queststatus_weekly")
	}
	for _, sess := range s.debugResetQuestSessions() {
		sess.player.WeeklyQuests = make(map[uint32]struct{})
	}
	next := nextWeeklyQuestResetTime(time.Now())
	if s.server != nil {
		s.server.setWorldState(ctx, worldStateWeeklyQuestResetTime, uint64(next.Unix()))
	}
	s.sendSysMessage(fmt.Sprintf("Weekly quests have been reset. Next scheduled reset: %s", debugQuestResetTimeText(next)))
}

func (s *session) debugResetMonthlyQuests(ctx context.Context) {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_queststatus_monthly")
	}
	for _, sess := range s.debugResetQuestSessions() {
		sess.player.MonthlyQuests = make(map[uint32]struct{})
	}
	next := nextMonthlyQuestResetTime(time.Now())
	if s.server != nil {
		s.server.setWorldState(ctx, worldStateMonthlyQuestResetTime, uint64(next.Unix()))
	}
	s.sendSysMessage(fmt.Sprintf("Monthly quests have been reset. Next scheduled reset: %s", debugQuestResetTimeText(next)))
}

// handleDebugRaidReset mirrors HandleDebugRaidResetCommand
// (cs_debug.cpp:1405): the map/difficulty validation order and messages
// are C++-exact (DBCEnums.h:294-295: MAX_DUNGEON_DIFFICULTY 3,
// MAX_RAID_DIFFICULTY 4; DUNGEON_DIFFICULTY_NORMAL 0), and each reset leg
// runs the scheduler's resetInstanceGlobal twin (the _ResetOrWarnAll DB
// legs: expired-bind, group-bind and instance deletes plus the extend
// demote, respawn wipe and player invalidation). The next-reset
// computation and warning schedule ride the scheduler's own tick, not
// this command, matching Go's read-only resettime stance.
func (s *session) handleDebugRaidReset(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug raidreset <mapId> [difficulty]"
	if len(args) == 0 || len(args) > 2 || s.server == nil || s.server.Data == nil {
		return false
	}
	mapID, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	difficulty := -1
	if len(args) == 2 {
		parsed, ok := parseCppBase0Int(args[1])
		if !ok || parsed < 0 || parsed > 255 {
			// Unparseable/out-of-range typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		difficulty = int(parsed)
	}
	entry, found, err := s.server.Data.Map(mapID)
	if err != nil || !found {
		s.sendSysMessage("Invalid map specified.")
		return true
	}
	if !entry.IsDungeon() {
		s.sendSysMessage(fmt.Sprintf("'%s' is not a dungeon map.", entry.MapName))
		return true
	}
	if difficulty >= 0 {
		if difficulty >= 4 {
			s.sendSysMessage(fmt.Sprintf("Invalid difficulty %d - specify in range [0,%d).", difficulty, 4))
			return true
		}
		if _, found, err := s.server.Data.MapDifficulty(mapID, uint32(difficulty)); err != nil || !found {
			s.sendSysMessage(fmt.Sprintf("Difficulty %d is not valid for '%s'.", difficulty, entry.MapName))
			return true
		}
	}
	if difficulty < 0 {
		s.sendSysMessage(fmt.Sprintf("Resetting all difficulties for '%s'.", entry.MapName))
		start, end := uint32(1), uint32(3)
		if entry.IsRaid() {
			start, end = 0, 4
		}
		for diff := start; diff < end; diff++ {
			if _, found, err := s.server.Data.MapDifficulty(mapID, diff); err == nil && found {
				s.sendSysMessage(fmt.Sprintf("Resetting difficulty %d for '%s'.", diff, entry.MapName))
				s.server.resetInstanceGlobal(ctx, instanceResetKey{MapID: mapID, Difficulty: diff})
			}
		}
		return true
	}
	if entry.IsNonRaidDungeon() && difficulty == 0 {
		s.sendSysMessage(fmt.Sprintf("'%s' does not have any permanent saves for difficulty %d.", entry.MapName, difficulty))
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Resetting difficulty %d for '%s'.", difficulty, entry.MapName))
	s.server.resetInstanceGlobal(ctx, instanceResetKey{MapID: mapID, Difficulty: uint32(difficulty)})
	return true
}

// debugClosePoint mirrors the spawnvehicle position leg
// (cs_debug.cpp:977-978): GetClosePoint(x, y, z, combatReach) resolves
// through GetNearPoint to a point combatReach away along the player's
// orientation (Object.cpp:3307-3311).
func (s *session) debugClosePoint() (x, y, z, o float32) {
	p := s.player
	reach := float64(p.CombatReach)
	return p.X + float32(math.Cos(float64(p.Orientation))*reach),
		p.Y + float32(math.Sin(float64(p.Orientation))*reach),
		p.Z, p.Orientation
}

// handleDebugSpawnVehicle mirrors HandleDebugSpawnVehicleCommand
// (cs_debug.cpp:975): without an id the entry is summoned at the close
// point as a manual-despawn temp summon (the Player::SummonCreature
// default, Object.h:412); with an id the creature template and the
// Vehicle.dbc entry gate first, then the summon's kit is replaced with
// the given vehicle id (C++ Create takes the id outright, overriding the
// template's VehicleId). Unknown entries fail silently, the summon
// funnel's precedent; the C++ id form creates a full creature while Go
// creates a temp summon, so no DB row backs the spawn.
func (s *session) handleDebugSpawnVehicle(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug spawnvehicle <entry> [vehicleId]"
	if len(args) == 0 || len(args) > 2 {
		return false
	}
	if s.server == nil || s.player == nil {
		return true
	}
	entry, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	vehicleID := uint32(0)
	hasVehicleID := false
	if len(args) == 2 {
		vid, ok := parseCommandUint32(args[1])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		vehicleID = vid
		hasVehicleID = true
	}
	if hasVehicleID {
		if _, ok := s.npcTemplateGate(ctx, entry); !ok {
			return false
		}
		if s.server.Data == nil {
			return false
		}
		if _, ok, err := s.server.Data.Vehicle(vehicleID); err != nil || !ok {
			return false
		}
	}
	x, y, z, o := s.debugClosePoint()
	origin := tempSummonOrigin{Map: s.player.Map, InstanceID: s.player.InstanceID, GUID: s.player.GUID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, O: s.player.Orientation, Reach: float64(s.player.CombatReach)}
	summoned := s.server.summonTempCreature(ctx, origin, entry, x, y, z, o, 7, 0)
	if summoned == nil {
		return false
	}
	if hasVehicleID {
		s.server.createVehicleKit(summoned.Map, summoned.InstanceID, summoned.GUID, vehicleID, entry, false)
	}
	return true
}

// handleDebugEnterVehicle mirrors HandleDebugEnterVehicleCommand
// (cs_debug.cpp:949): the selected unit must be a vehicle (a live kit on
// the selection's map/instance). Entry 0 boards the handler's player via
// the session enterVehicle leg; otherwise the nearest creature of the
// entry within 20 yards of the player boards via the accessory boarding
// pattern (kit.AddPassenger plus the motion's VehicleGUID/VehicleSeat
// slots and the OnAddPassenger fire). C++ fails when its cell searcher
// hits the vehicle itself first; Go skips the vehicle in the scan, so a
// same-entry vehicle never blocks the board. A missing vehicle, a
// missing passenger, or an unparseable seat fails silently, the C++
// false-return legs.
func (s *session) handleDebugEnterVehicle(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	const syntax = "Syntax: .debug entervehicle <entry> [seatId]"
	if len(args) == 0 || len(args) > 2 {
		return false
	}
	if s.server == nil || s.player == nil {
		return true
	}
	if s.selection == 0 {
		return false
	}
	entry, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage(syntax)
		return true
	}
	seatID := int8(-1)
	if len(args) == 2 {
		parsed, ok := parseCppBase0Int(args[1])
		if !ok || parsed < -128 || parsed > 127 {
			// Unparseable/out-of-range typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage(syntax)
			return true
		}
		seatID = int8(parsed)
	}
	vehicleGUID := s.selection
	if s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehicleGUID) == nil {
		return false
	}
	if entry == 0 {
		s.enterVehicle(ctx, vehicleGUID, seatID)
		s.sendSysMessage(fmt.Sprintf("Unit %d entered vehicle %d", entry, seatID))
		return true
	}
	passenger := s.debugVehiclePassenger(entry, vehicleGUID)
	if passenger == nil {
		return false
	}
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, vehicleGUID)
	if kit == nil {
		return false
	}
	assigned, _, ok := kit.AddPassenger(passenger.GUID, seatID, passenger.UnitFlags&0x02000000 != 0)
	if ok {
		s.server.motionMu.Lock()
		passenger.VehicleGUID = vehicleGUID
		passenger.VehicleSeat = assigned
		passenger.Moving = false
		s.server.motionMu.Unlock()
		if base := s.findCreatureMotion(vehicleGUID); base != nil {
			s.server.triggerVehicleEvent(scripting.VehicleEventOnAddPassenger, s.server.luaMotionCreature(base), s.server.luaMotionCreature(passenger), assigned)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Unit %d entered vehicle %d", entry, seatID))
	return true
}

// debugVehiclePassenger mirrors the entervehicle passenger search
// (cs_debug.cpp:957-963): AllCreaturesOfEntryInRange over the player's
// map/instance, entry match, within 20 yards (the check's range; the
// 30-yard cell-visit radius has no Go twin since the scan is map-wide).
func (s *session) debugVehiclePassenger(entry uint32, vehicleGUID uint64) *creatureMotion {
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(s.player.Map, s.player.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	for _, motion := range motions {
		if motion.GUID == vehicleGUID || motion.Entry != entry {
			continue
		}
		if distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z) <= 20 {
			return motion
		}
	}
	return nil
}
