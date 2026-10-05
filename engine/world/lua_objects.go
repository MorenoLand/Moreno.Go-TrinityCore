package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type luaCreatureState struct {
	GUID         uint64
	Entry        uint32
	Name         string
	DisplayID    uint32
	Health       uint32
	MaxHealth    uint32
	Level        uint32
	GossipMenuID uint32
	NPCFlags     uint32
	Map          uint32
	InstanceID   uint32
	X            float32
	Y            float32
	Z            float32
}

type luaGameObjectState struct {
	GUID       uint64
	LowGUID    uint32
	Entry      uint32
	DisplayID  uint32
	Name       string
	Map        uint32
	InstanceID uint32
	X          float32
	Y          float32
	Z          float32
	GoState    uint32
	LootState  uint32
}

func (s *session) luaCreature(ctx context.Context, guid uint64) *scripting.Object {
	if uint16(guid>>48) != 0xF130 {
		return nil
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var state luaCreatureState
	var displayID, gossipMenuID, npcFlags int64
	var selectArgs []any
	npcFlagExpr := "t.npcflag"
	if flagClause, ok := s.server.gameEventNPCFlagClause(ctx, &selectArgs); ok {
		npcFlagExpr = "(t.npcflag | " + flagClause + ")"
	}
	queryArgs := append(selectArgs, low, entry)
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, "+npcFlagExpr+", c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", queryArgs...).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &state.Map, &state.X, &state.Y, &state.Z)
	if err != nil && missingTable(err) {
		err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, t.npcflag, c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", low, entry).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &state.Map, &state.X, &state.Y, &state.Z)
	}
	if err != nil {
		if !errorsIsNoRows(err) {
			s.debug("lua creature lookup failed", "account", s.accountName, "guid", guid, "error", err)
		}
		return nil
	}
	if s.player == nil || state.Map != s.player.Map {
		return nil
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(state.Map, s.player.InstanceID, guid)
	if motion == nil || motion.Entry != entry {
		s.server.motionMu.Unlock()
		return nil
	}
	state.GUID = guid
	state.InstanceID = motion.InstanceID
	state.DisplayID, state.Health, state.MaxHealth, state.Level = uint32(displayID), motion.Health, motion.MaxHealth, motion.Level
	state.X, state.Y, state.Z = motion.X, motion.Y, motion.Z
	s.server.motionMu.Unlock()
	state.GossipMenuID, state.NPCFlags = uint32(gossipMenuID), uint32(npcFlags)
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetEntry"] = luaNoArgs(func() any { return state.Entry })
	methods["GetDisplayId"] = luaNoArgs(func() any { return state.DisplayID })
	methods["GetGUID"] = luaNoArgs(func() any { return state.GUID })
	methods["GetGUIDLow"] = luaNoArgs(func() any { return uint32(state.GUID & 0x00FFFFFF) })
	methods["GetObjectType"] = luaNoArgs(func() any { return "Creature" })
	methods["GetHealth"] = luaNoArgs(func() any { return state.Health })
	methods["GetMaxHealth"] = luaNoArgs(func() any { return state.MaxHealth })
	methods["GetHealthPct"] = luaNoArgs(func() any {
		if state.MaxHealth == 0 {
			return float32(0)
		}
		return float32(state.Health) * 100 / float32(state.MaxHealth)
	})
	methods["GetLevel"] = luaNoArgs(func() any { return state.Level })
	methods["IsDead"] = luaNoArgs(func() any { return state.Health == 0 })
	methods["IsFullHealth"] = luaNoArgs(func() any { return state.MaxHealth > 0 && state.Health >= state.MaxHealth })
	hasNPCFlag := func(flag uint32) scripting.ObjectMethod {
		return luaNoArgs(func() any { return state.NPCFlags&flag != 0 })
	}
	methods["IsGossip"] = hasNPCFlag(0x00000001)
	methods["IsQuestGiver"] = hasNPCFlag(0x00000002)
	methods["IsVendor"] = hasNPCFlag(0x00000080)
	methods["IsTrainer"] = hasNPCFlag(0x00000070)
	methods["IsTaxi"] = hasNPCFlag(0x00002000)
	methods["IsSpiritHealer"] = hasNPCFlag(0x00004000)
	methods["IsSpiritGuide"] = hasNPCFlag(0x00008000)
	methods["IsSpiritService"] = hasNPCFlag(0x0000C000)
	methods["IsInnkeeper"] = hasNPCFlag(0x00001000)
	methods["IsAuctioneer"] = hasNPCFlag(0x00200000)
	methods["IsServiceProvider"] = luaNoArgs(func() any { return state.NPCFlags&0x007FC0F2 != 0 })
	methods["IsInCombat"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		return motion != nil && motion.InCombat
	})
	methods["GetVictim"] = func(ctx context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || motion.TargetGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(motion.TargetGUID); player != nil {
			return []any{player}, nil
		}
		if creature := s.luaCreature(ctx, motion.TargetGUID); creature != nil {
			return []any{creature}, nil
		}
		return []any{nil}, nil
	}
	methods["HasAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID}
		s.server.auraMu.Lock()
		_, found := s.server.creatureAuras[key][spell]
		if !found {
			_, found = s.server.activeCreatureAuras[key][spell]
		}
		s.server.auraMu.Unlock()
		return []any{found}, nil
	}
	methods["RemoveAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.removeCreatureAura(creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID}, spell)
		return nil, nil
	}
	methods["IsAlive"] = luaNoArgs(func() any { return state.Health > 0 })
	methods["AddAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID}
		s.server.auraMu.Lock()
		if s.server.creatureAuras == nil {
			s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
		}
		if s.server.creatureAuras[key] == nil {
			s.server.creatureAuras[key] = make(map[uint32]struct{})
		}
		s.server.creatureAuras[key][spell] = struct{}{}
		s.server.auraMu.Unlock()
		return nil, nil
	}
	methods["SetHealth"] = func(_ context.Context, args []any) ([]any, error) {
		health, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion == nil || motion.Entry != state.Entry {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		if health > motion.MaxHealth {
			health = motion.MaxHealth
		}
		motion.Health = health
		mapID, instanceID, liveGUID := motion.Map, motion.InstanceID, motion.GUID
		s.server.motionMu.Unlock()
		state.Health = health
		s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, liveGUID, map[int]uint32{unitFieldHealth: health})
		return nil, nil
	}
	methods["SendBroadcastMessage"] = s.luaMessageMethod()
	// Whisper(message) mirrors the botgiver's WhisperTo helper
	// (botgiver.cpp): me->Whisper(message, LANG_UNIVERSAL, player).
	// Sends SMSG_MESSAGECHAT with CHAT_MSG_MONSTER_WHISPER (0x0F,
	// SharedDefines.h:3177), LANG_UNIVERSAL (0), sender = creature GUID,
	// receiver = the gossiping (session) player's GUID.
	methods["Whisper"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("message is required")
		}
		message, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("message must be a string")
		}
		if s == nil || s.player == nil {
			return nil, nil
		}
		return nil, s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildChatMessage(0x0F, 0, state.GUID, s.player.GUID, message, ""), true)
	}
	return &scripting.Object{Type: "Creature", Fields: map[string]any{"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry, "GossipMenuID": state.GossipMenuID, "NPCFlags": state.NPCFlags, "Map": state.Map, "MapId": state.Map, "X": state.X, "Y": state.Y, "Z": state.Z, "Health": state.Health, "MaxHealth": state.MaxHealth, "Level": state.Level, "InWorld": true}, Methods: methods}
}

func (s *session) luaCreatureMotion(guid uint64, entry uint32) *creatureMotion {
	return s.findCreatureMotion(guid)
}

func (s *session) luaGameObject(ctx context.Context, guid uint64) *scripting.Object {
	if uint16(guid>>48) != 0xF110 {
		return nil
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	state, ok := s.loadLuaGameObject(ctx, low, entry)
	if !ok || s.player == nil || state.Map != s.player.Map {
		return nil
	}
	state.InstanceID = s.player.InstanceID
	if s.server.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) {
		return nil
	}
	return s.luaGameObjectObject(state)
}

func (s *session) loadLuaGameObject(ctx context.Context, low, entry uint32) (luaGameObjectState, bool) {
	var state luaGameObjectState
	var mapID int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.map, g.position_x, g.position_y, g.position_z FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.guid = ? AND g.id = ?", low, entry).Scan(&state.LowGUID, &state.Entry, &state.DisplayID, &state.Name, &mapID, &state.X, &state.Y, &state.Z); err != nil {
		return state, false
	}
	state.Map, state.GUID = uint32(mapID), gameObjectGUID(state.LowGUID, state.Entry)
	state.GoState, state.LootState = 1, 1
	return state, true
}

func (s *session) luaGameObjectObject(state luaGameObjectState) *scripting.Object {
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetEntry"] = luaNoArgs(func() any { return state.Entry })
	methods["GetDisplayId"] = luaNoArgs(func() any { return state.DisplayID })
	methods["GetDBTableGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	methods["GetGoState"] = luaNoArgs(func() any { return state.GoState })
	methods["GetLootState"] = luaNoArgs(func() any { return state.LootState })
	methods["IsSpawned"] = luaNoArgs(func() any { return !s.server.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) })
	methods["IsActive"] = luaNoArgs(func() any { return state.GoState == 0 })
	methods["GetGUID"] = luaNoArgs(func() any { return state.GUID })
	methods["GetGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	methods["GetObjectType"] = luaNoArgs(func() any { return "GameObject" })
	methods["GetMapId"] = luaNoArgs(func() any { return state.Map })
	methods["GetX"] = luaNoArgs(func() any { return state.X })
	methods["GetY"] = luaNoArgs(func() any { return state.Y })
	methods["GetZ"] = luaNoArgs(func() any { return state.Z })
	methods["RemoveFromWorld"] = func(_ context.Context, _ []any) ([]any, error) {
		s.server.setGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID, true)
		return nil, nil
	}
	methods["SetGoState"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("state is required")
		}
		value, err := luaUint32Arg(args, 0)
		if err != nil || value > 2 {
			if err == nil {
				err = fmt.Errorf("invalid gameobject state")
			}
			return nil, err
		}
		state.GoState = value
		return nil, nil
	}
	methods["SetLootState"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("loot state is required")
		}
		value, err := luaUint32Arg(args, 0)
		if err != nil || value > 3 {
			if err == nil {
				err = fmt.Errorf("invalid loot state")
			}
			return nil, err
		}
		state.LootState = value
		return nil, nil
	}
	methods["Despawn"] = func(_ context.Context, _ []any) ([]any, error) {
		s.objectsMuLockGameObject(state.Map, state.InstanceID, state.GUID)
		return nil, nil
	}
	methods["Respawn"] = func(_ context.Context, _ []any) ([]any, error) {
		s.objectsMuUnlockGameObject(state.Map, state.InstanceID, state.GUID)
		return nil, nil
	}
	return &scripting.Object{Type: "GameObject", Fields: map[string]any{"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry, "Map": state.Map, "MapId": state.Map, "X": state.X, "Y": state.Y, "Z": state.Z, "DisplayID": state.DisplayID, "GoState": state.GoState, "LootState": state.LootState, "InWorld": true}, Methods: methods}
}

func (s *session) objectsMuLockGameObject(mapID, instanceID uint32, guid uint64) {
	s.server.setGameObjectHiddenInInstance(mapID, instanceID, guid, true)
}
func (s *session) objectsMuUnlockGameObject(mapID, instanceID uint32, guid uint64) {
	s.server.setGameObjectHiddenInInstance(mapID, instanceID, guid, false)
}

func (s *session) nearestGameObject(ctx context.Context, mapID uint32, x, y float32, distance float32) *scripting.Object {
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.position_x, g.position_y, g.position_z FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.map = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ? ORDER BY g.guid", mapID, float64(x-distance), float64(x+distance), float64(y-distance), float64(y+distance))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var nearest luaGameObjectState
	nearestDistance := float32(math.MaxFloat32)
	for rows.Next() {
		var candidate luaGameObjectState
		if err := rows.Scan(&candidate.LowGUID, &candidate.Entry, &candidate.DisplayID, &candidate.Name, &candidate.X, &candidate.Y, &candidate.Z); err != nil {
			continue
		}
		candidate.Map = mapID
		candidate.InstanceID = s.player.InstanceID
		candidate.GoState, candidate.LootState = 1, 1
		candidate.GUID = gameObjectGUID(candidate.LowGUID, candidate.Entry)
		if s.server.isGameObjectHiddenInInstance(candidate.Map, candidate.InstanceID, candidate.GUID) {
			continue
		}
		currentDistance := float32(math.Hypot(float64(candidate.X-x), float64(candidate.Y-y)))
		if currentDistance <= distance && currentDistance < nearestDistance {
			nearest, nearestDistance = candidate, currentDistance
		}
	}
	if nearest.GUID == 0 {
		return nil
	}
	return s.luaGameObjectObject(nearest)
}

func (s *Server) isGameObjectHidden(guid uint64) bool {
	s.objectsMu.RLock()
	_, hidden := s.hiddenGameObjects[guid]
	s.objectsMu.RUnlock()
	return hidden
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func maxUint64(value int64, fallback int64) int64 {
	if value < fallback {
		return fallback
	}
	return value
}
