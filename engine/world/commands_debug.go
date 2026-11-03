package world

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

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
func (s *session) handleDebugAnim(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	const syntax = "Syntax: .debug anim <emote>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	emote, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage(syntax)
		return
	}
	if motion := s.castCommandSelectedCreature(); motion != nil {
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
func (s *session) handleDebugWorldState(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	const syntax = "Syntax: .debug worldstate <variable> <value>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	variable, err1 := strconv.ParseUint(args[0], 10, 32)
	value, err2 := strconv.ParseUint(args[1], 10, 32)
	if err1 != nil || err2 != nil {
		s.sendSysMessage(syntax)
		return
	}
	s.sendWorldState(uint32(variable), uint32(value))
}

// handleDebugDummy mirrors HandleDebugDummyCommand (cs_debug.cpp:1792):
// the arm's whole C++ body is the literal no-op notice.
func (s *session) handleDebugDummy(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	s.sendSysMessage("This command does nothing right now. Edit your local core (cs_debug.cpp) to make it do whatever you need for testing.")
}

// handleDebugObjectCount mirrors HandleDebugObjectCountCommand plus
// HandleDebugObjectCountMap (cs_debug.cpp:1715, 1770): ".debug objectcount
// [mapId]" prints, per (map, instance), the live creature count and the
// runtime gameobject count, then the top 5 creature entries by count.
func (s *session) handleDebugObjectCount(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	var mapFilter *uint32
	if len(args) > 0 {
		id, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			s.sendSysMessage("Syntax: .debug objectcount [mapId]")
			return
		}
		v := uint32(id)
		mapFilter = &v
	}
	if s.server == nil || s.player == nil {
		return
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
func (s *session) handleDebugNearGraveyard(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	if s.server == nil || s.player == nil || s.server.Data == nil {
		return
	}
	linked := len(args) > 0 && strings.ToLower(args[0]) == "linked"
	if linked {
		loc, found := s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.Zone, playerTeam(s.player.Race))
		if !found {
			s.sendSysMessage("No graveyard found.")
			return
		}
		s.sendSysMessage(fmt.Sprintf("Nearest graveyard: %d at (%f, %f, %f)", loc.ID, loc.X, loc.Y, loc.Z))
		return
	}
	file, err := s.server.Data.File("WorldSafeLocs")
	if err != nil {
		s.sendSysMessage("No graveyard found.")
		return
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
		return
	}
	s.sendSysMessage(fmt.Sprintf("Nearest graveyard: %d at (%f, %f, %f)", nearest.id, nearest.x, nearest.y, nearest.z))
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
func (s *session) handleDebugLoS(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	if s.server == nil || s.player == nil {
		return
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
func (s *session) handleDebugPlay(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	const syntax = "Syntax: .debug play cinematic|movie|sound|music <id>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return
	}
	id, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		s.sendSysMessage(syntax)
		return
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
}

// handleDebugSendSpellFail mirrors HandleDebugSendSpellFailCommand
// (cs_debug.cpp:221): ".debug send spellfail <result> [failArg1] [failArg2]"
// writes SMSG_CAST_FAILED with cast id 0 and spell 133 (Fireball) through
// the buildCastFailedParams funnel, which appends the optional extended
// params exactly as C++ appends failArg1/failArg2. DELTA: the result arg
// parses as a numeric SpellCastResult; C++'s enum-name form has no Go
// command parser. The remaining send arms are wired in handleDebugSend,
// including opcode.
func (s *session) handleDebugSendSpellFail(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	const syntax = "Syntax: .debug send spellfail <result> [failArg1] [failArg2]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	result, err := strconv.ParseUint(args[0], 10, 8)
	if err != nil {
		s.sendSysMessage(syntax)
		return
	}
	var params []uint32
	for _, arg := range args[1:] {
		v, err := strconv.ParseUint(arg, 10, 32)
		if err != nil {
			s.sendSysMessage(syntax)
			return
		}
		params = append(params, uint32(v))
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailedParams(0, 133, uint8(result), params...), true)
}

// handleDebugSend mirrors the debugSendCommandTable (cs_debug.cpp:70-82):
// spellfail, buyerror, sellerror, equiperror, channelnotify, chatmessage,
// qpartymsg, qinvalidmsg, setphaseshift, largepacket and opcode — the
// whole subtable. The debug arms
// pass nil creatures/items, so the error packets carry empty GUIDs and
// zero item/param fields exactly as C++ does; numeric args stand in for
// C++'s enum-name forms, matching the spellfail precedent. Unknown
// sub-arms report the syntax rather than silently succeeding.
func (s *session) handleDebugSend(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	const syntax = "Syntax: .debug send spellfail|buyerror|sellerror|equiperror|channelnotify|chatmessage|qpartymsg|qinvalidmsg|setphaseshift|largepacket|opcode <arg>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	switch strings.ToLower(args[0]) {
	case "spellfail":
		s.handleDebugSendSpellFail(ctx, args[1:])
	case "opcode":
		s.handleDebugSendOpcode(ctx)
	case "buyerror", "sellerror", "equiperror", "channelnotify", "chatmessage", "qpartymsg", "qinvalidmsg", "setphaseshift":
		if len(args) < 2 {
			s.sendSysMessage(syntax)
			return
		}
		value, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			s.sendSysMessage(syntax)
			return
		}
		s.debugSendNumeric(strings.ToLower(args[0]), uint32(value))
	case "largepacket":
		var sb strings.Builder
		for sb.Len() < 128000 {
			sb.WriteString("This is a dummy string to push the packet's size beyond 128000 bytes. ")
		}
		s.sendSysMessage(sb.String())
	default:
		s.sendSysMessage(syntax)
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
// opcode.txt reports a plain message where C++ returns false to the
// chat handler's generic failure; unknown types stop the token loop but
// the packet still sends, as C++ logs and breaks before sending.
func (s *session) handleDebugSendOpcode(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	raw, err := os.ReadFile("opcode.txt")
	if err != nil {
		s.sendSysMessage("opcode.txt not found.")
		return
	}
	tokens := strings.Fields(string(stripOpcodeComments(raw)))
	if len(tokens) == 0 {
		return
	}
	opcode, err := strconv.ParseUint(tokens[0], 10, 32)
	if err != nil {
		return
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
			return
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
}

// handleDebugLootRecipient mirrors HandleDebugGetLootRecipientCommand
// (cs_debug.cpp:473): ".debug lootrecipient" reports the selected
// creature's tap model (creatureMotion TapPlayerGUID/TapGroupID, the
// Creature::SetLootRecipient twin): no tap reads "no loot recipient", a
// tapped player online reads its name, a tapped player offline reads
// "offline". The GUID counter and spawn id share the npc-info precedent
// (GUID & 0x00FFFFFF); a group-only tap has no tracked player GUID, so it
// reads "offline" like an unresolvable recipient.
func (s *session) handleDebugLootRecipient(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		return
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
func (s *session) handleDebugInstanceSpawns(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	if s.server == nil || s.player == nil {
		return
	}
	const syntax = "Syntax: .debug instancespawn <groupId|explain>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	explain := strings.ToLower(args[0]) == "explain"
	var groupID uint32
	if !explain {
		id, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			s.sendSysMessage(syntax)
			return
		}
		groupID = uint32(id)
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
		return
	}
	if err := s.server.ensureInstanceSpawnGroups(ctx, s.server.WorldStore.DB); err != nil {
		s.sendSysMessage("Instance spawn group data is unavailable.")
		return
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
			return
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
		return
	}
	if len(store) == 0 {
		s.sendSysMessage(fmt.Sprintf("%s's instance script does not manage any spawn groups.", mapName))
		return
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
	if m := s.castCommandSelectedCreature(); m != nil {
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
// threat list (sorted descending, [FIXATE]/[TAUNT]/[SUPPRESSED] markers) or
// the cannot-have-list / not-engaged legs. Deltas: no threatened-by-me
// index (rebuilt by scan, so only live creature lists contribute); no
// detaunt/offline markers (no per-reference detaunt state, no online
// tracking); the offline leg of a victim row is unmodeled.
func (s *session) handleDebugThreat(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	name, guid, alive, engaged, motion := s.debugThreatTarget()
	counter := uint32(guid & 0x00FFFFFF)
	if !alive {
		engagedNote := ""
		if engaged {
			engagedNote = " (It is, however, engaged. Huh?)"
		}
		s.sendSysMessage(fmt.Sprintf("%s (GUID %d) is not alive.%s", name, counter, engagedNote))
		return
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
		return
	}
	entries := motion.ThreatMgr.DebugEntries()
	if len(entries) == 0 {
		if !engaged {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d, SpawnID %d) is not currently engaged.", name, counter, counter))
		} else {
			s.sendSysMessage(fmt.Sprintf("%s (GUID %d, SpawnID %d) seems to be engaged, but does not have a threat list??", name, counter, counter))
		}
		return
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
		}
		if e.suppressed {
			marker += " [SUPPRESSED]"
		}
		s.sendSysMessage(fmt.Sprintf("   %d.   %s   (GUID %d)  - threat %f%s", i+1, s.debugUnitName(e.victimGUID), uint32(e.victimGUID&0x00FFFFFF), e.threat, marker))
	}
	s.sendSysMessage("End of threat list.")
}

// handleDebugThreatInfo mirrors HandleDebugThreatInfoCommand
// (cs_debug.cpp:854): ".debug threatinfo" requires a selected unit and
// prints the modeled threat-manager state. Deltas: the single-school and
// multi-school modifier tables and the redirect info/registry have no Go
// model (ThreatManager tracks only the temp modifier per victim), so the
// report prints the modeled rows — fixate target, live taunts,
// suppressions and per-victim temp modifiers — and says so for the rest.
func (s *session) handleDebugThreatInfo(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	motion := s.castCommandSelectedCreature()
	var name string
	var guid uint64
	var mgr *ThreatManager
	if motion != nil {
		name, guid, mgr = motion.Name, motion.GUID, motion.ThreatMgr
	} else if s.selection != 0 {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			name, guid = ts.player.Name, ts.playerGUID
		}
	}
	if motion == nil && guid == 0 {
		s.sendSysMessage("Select a character or creature.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("Threat info for %s (%d):", name, guid))
	s.sendSysMessage(" - Single-school threat modifiers: not modeled")
	s.sendSysMessage("- Multi-school threat modifiers (0 entries): not modeled")
	s.sendSysMessage(" - No redirects being applied (redirects not modeled)")
	s.sendSysMessage(" - No redirects are registered (redirects not modeled)")
	if mgr == nil {
		return
	}
	fixate := mgr.GetFixateTarget()
	if fixate != 0 {
		s.sendSysMessage(fmt.Sprintf(" - Fixate target: %s", s.debugUnitName(fixate)))
	}
	for _, e := range mgr.DebugEntries() {
		if e.taunted {
			s.sendSysMessage(fmt.Sprintf(" - Taunted victim: %s", s.debugUnitName(e.victimGUID)))
		}
		if e.suppressed {
			s.sendSysMessage(fmt.Sprintf(" - Suppressed victim: %s", s.debugUnitName(e.victimGUID)))
		}
	}
}

// handleDebugCombat mirrors HandleDebugCombatListCommand (cs_debug.cpp:929):
// ".debug combat" prints the target's combat state line. Delta: Go has no
// combat-reference list (only the InCombat/UnitFlags bits), so no [PvP] /
// [PvE] ref rows follow the header.
func (s *session) handleDebugCombat(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	_, _, _, engaged, _ := s.debugThreatTarget()
	combatState := 0
	if engaged {
		combatState = 1
	}
	s.sendSysMessage(fmt.Sprintf("Combat refs: (Combat state: %d | Manager state: %d)", combatState, combatState))
}

// handleDebugGuidLimits mirrors HandleDebugGuidLimitsCommand plus
// HandleDebugGuidLimitsMap (cs_debug.cpp:1683, 1698): ".debug guidlimits
// [mapId]" prints, per (map, instance), the highest live creature and
// gameobject GUID counters, reusing the objectcount map iteration. Delta:
// the GUID warn/alert config levels have no Go config, so those two lines
// are omitted; counters cover live motions and runtime gameobjects only,
// not the DB spawn tables.
func (s *session) handleDebugGuidLimits(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	var mapFilter *uint32
	if len(args) > 0 {
		id, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			s.sendSysMessage("Syntax: .debug guidlimits [mapId]")
			return
		}
		v := uint32(id)
		mapFilter = &v
	}
	if s.server == nil || s.player == nil {
		return
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
}

// handleDebugWPGPS mirrors HandleWPGPSCommand (cs_debug.cpp:1311):
// ".debug wpgps" logs the invoker's position as a waypoint SQL row and
// reports that the row was written. Go has no sql.dev log file, so the row
// goes to the session debug log instead.
func (s *session) handleDebugWPGPS(ctx context.Context) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	if s.player == nil {
		return
	}
	s.debug("waypoint sql", "row", fmt.Sprintf("(@PATH, XX, %.3f, %.3f, %.5f, %.5f, 0, 0, 0, 100, 0),", s.player.X, s.player.Y, s.player.Z, s.player.Orientation))
	s.sendSysMessage("Waypoint SQL written to SQL Developer log")
}

// handleDebugTransport mirrors HandleDebugTransportCommand
// (cs_debug.cpp:1317): ".debug transport [stop|start]" on the invoker's
// continent transport. The stop/start legs ride the MovementPaused latch
// (EnableMovement twin); any other argument prints the transport's name,
// state and position. No transport reads a plain notice, the C++ false
// return's Go equivalent.
func (s *session) handleDebugTransport(ctx context.Context, args []string) {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	if s.server == nil || s.player == nil || s.player.TransportGUID == 0 {
		s.sendSysMessage("You are not on a transport.")
		return
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
		op := ""
		if len(args) > 0 {
			op = strings.ToLower(args[0])
		}
		if op == "stop" || op == "start" {
			transport.MovementPaused = op == "stop"
			paused = transport.MovementPaused
			s.server.transportMu.Unlock()
			if op == "stop" {
				s.sendSysMessage(fmt.Sprintf("Transport %s stopped", transportName))
			} else {
				s.sendSysMessage(fmt.Sprintf("Transport %s started", transportName))
			}
			return
		}
		s.server.transportMu.Unlock()
		state := "moving"
		if paused {
			state = "stopped"
		}
		s.sendSysMessage(fmt.Sprintf("Transport %s is %s", transportName, state))
		s.sendSysMessage(fmt.Sprintf("Transport position: X: %f Y: %f Z: %f O: %f", x, y, z, o))
		return
	}
	s.server.transportMu.Unlock()
	s.sendSysMessage("You are not on a transport.")
}
