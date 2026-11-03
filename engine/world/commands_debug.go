package world

import (
	"context"
	"fmt"
	"sort"
	"strconv"

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
