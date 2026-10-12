package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

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
	// Template reads backing the CreatureMethods stat surface (rank,
	// flags_extra, type_flags, family, AIName, RacialLeader, unit_flags are
	// static per entry, so the constructor snapshot is exact).
	Rank         uint32
	FlagsExtra   uint32
	TypeFlags    uint32
	Family       uint32
	UnitFlags    uint32
	AIName       string
	RacialLeader bool
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

// luaSoundTargetSession resolves the optional [Player] argument (args[1]) of
// the Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242): a
// player object with a live session gets the direct message; anything else
// (absent, non-player, or offline) falls through to the set broadcast, as
// C++ does for a null player.
func (srv *Server) luaSoundTargetSession(args []any) *session {
	if srv == nil || len(args) < 2 {
		return nil
	}
	object, ok := args[1].(*scripting.Object)
	if !ok || object == nil {
		return nil
	}
	guid, ok := objectUint64Field(object, "GUID")
	if !ok || guid == 0 {
		return nil
	}
	return srv.findSessionByGUID(guid)
}

// luaDeliverSound mirrors the delivery tail shared by LuaWorldObject::PlayMusic,
// PlayDirectSound and PlayDistanceSound (WorldObjectMethods.h:1161-1242): a
// resolved player target gets the direct write; otherwise the packet goes to
// the object's visible set — C++ SendMessageToSet(data, true), i.e. self plus
// nearby (the sendSpellStart/sendSpellMiss convention: self write plus
// broadcastToNearby skipping self; creatures and gameobjects have no session,
// so they broadcast with a nil source like the creature-text sound arm).
// The sSoundEntriesStore existence gate has no Go bridge (no SoundEntries.dbc
// model, same documented-blocked state as handleEffectPlaySound): an unknown
// id plays nothing client-side.
func (srv *Server) luaDeliverSound(self *session, opcode uint16, payload []byte, args []any) error {
	if srv == nil {
		return nil
	}
	if target := srv.luaSoundTargetSession(args); target != nil {
		return target.write(opcode, payload, true)
	}
	if self != nil {
		if err := self.write(opcode, payload, true); err != nil {
			return err
		}
		srv.broadcastToNearby(opcode, payload, self)
		return nil
	}
	srv.broadcastToNearby(opcode, payload, nil)
	return nil
}

// luaWorldObjectSoundMethods registers the three Eluna WorldObject sound
// methods on a Lua object: PlayMusic (SMSG_PLAY_MUSIC, u32 id),
// PlayDirectSound (SMSG_PLAY_SOUND, u32 id) and PlayDistanceSound
// (SMSG_PLAY_OBJECT_SOUND, u32 id + packed source GUID). sourceGUID is the
// WorldObject's GUID for the distance-sound packet; self is the source's
// session when it has one (players), nil for creatures and gameobjects.
func luaWorldObjectSoundMethods(srv *Server, self *session, sourceGUID uint64, methods map[string]scripting.ObjectMethod) {
	methods["PlayMusic"] = func(_ context.Context, args []any) ([]any, error) {
		musicID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		pkt := protocol.NewBuffer(4)
		pkt.WriteU32(musicID)
		return nil, srv.luaDeliverSound(self, uint16(protocol.OpcodeSMSG_PLAY_MUSIC), pkt.Bytes(), args)
	}
	methods["PlayDirectSound"] = func(_ context.Context, args []any) ([]any, error) {
		soundID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return nil, srv.luaDeliverSound(self, uint16(protocol.OpcodeSMSG_PLAY_SOUND), buildPlaySound(soundID), args)
	}
	methods["PlayDistanceSound"] = func(_ context.Context, args []any) ([]any, error) {
		soundID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return nil, srv.luaDeliverSound(self, uint16(protocol.OpcodeSMSG_PLAY_OBJECT_SOUND), buildPlayObjectSound(soundID, sourceGUID), args)
	}
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
	var rank, flagsExtra, typeFlags, family, unitFlags, racialLeader int64
	var aiName string
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, "+npcFlagExpr+", t.rank, t.flags_extra, t.type_flags, t.family, t.unit_flags, t.AIName, t.RacialLeader, c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", queryArgs...).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &rank, &flagsExtra, &typeFlags, &family, &unitFlags, &aiName, &racialLeader, &state.Map, &state.X, &state.Y, &state.Z)
	if err != nil && missingTable(err) {
		err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, t.npcflag, t.rank, t.flags_extra, t.type_flags, t.family, t.unit_flags, t.AIName, t.RacialLeader, c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", low, entry).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &rank, &flagsExtra, &typeFlags, &family, &unitFlags, &aiName, &racialLeader, &state.Map, &state.X, &state.Y, &state.Z)
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
	state.Rank, state.FlagsExtra, state.TypeFlags, state.Family, state.UnitFlags = uint32(rank), uint32(flagsExtra), uint32(typeFlags), uint32(family), uint32(unitFlags)
	state.AIName, state.RacialLeader = aiName, racialLeader != 0
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
	// TextEmote(message) is Unit::TextEmote (Unit.cpp:14058): the
	// plain-string emote == SMSG_MESSAGECHAT with CHAT_MSG_MONSTER_EMOTE
	// (0x10, SharedDefines.h:3178), LANG_UNIVERSAL, sender = creature
	// GUID, receiver = the gossiping (session) player — the same
	// session-player targeting the Whisper binding above uses.
	methods["TextEmote"] = func(_ context.Context, args []any) ([]any, error) {
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
		return nil, s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildChatMessage(0x10, 0, state.GUID, s.player.GUID, message, ""), true)
	}
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554, Trinity):
	// HasUnitState(UNIT_STATE_CASTING). Go creature casts are packet-only
	// (castCreatureSpell) with no cast state, so this is always false;
	// the player object carries the real arm (lua_player.go).
	methods["IsCasting"] = luaNoArgs(func() any { return false })
	// StopSpellCast is UnitMethods::StopSpellCast (UnitMethods.h:2532-2537):
	// Unit::CastStop. No-op for creatures — no cast state to interrupt.
	methods["StopSpellCast"] = func(_ context.Context, _ []any) ([]any, error) { return nil, nil }
	// GetHomePosition is CreatureMethods::GetHomePosition
	// (CreatureMethods.h:630-644): x, y, z, o from the creature's home
	// position (Creature::GetHomePosition).
	methods["GetHomePosition"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return []any{float32(0), float32(0), float32(0), float32(0)}, nil
		}
		return []any{motion.HomeX, motion.HomeY, motion.HomeZ, motion.Orientation}, nil
	}
	// SetHomePosition is CreatureMethods::SetHomePosition
	// (CreatureMethods.h:655-669): Creature::SetHomePosition(x, y, z, o).
	methods["SetHomePosition"] = func(_ context.Context, args []any) ([]any, error) {
		x, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		y, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		z, err := luaFloat32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		o, err := luaFloat32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID); motion != nil {
			motion.HomeX, motion.HomeY, motion.HomeZ = x, y, z
			motion.Orientation = o
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// PerformEmote is UnitMethods::PerformEmote (UnitMethods.h:2041-2049):
	// Unit::HandleEmoteCommand sends SMSG_EMOTE (u32 emote + u64 guid) to
	// the visibility set; creatures broadcast with a nil source like the
	// creature-text arms.
	methods["PerformEmote"] = func(_ context.Context, args []any) ([]any, error) {
		emote, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		pkt := protocol.NewBuffer(12)
		pkt.WriteU32(emote)
		pkt.WriteU64(state.GUID)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), nil)
		return nil, nil
	}
	// ClearThreatList is UnitMethods::ClearThreatList
	// (UnitMethods.h:1992-1999): ThreatManager::ClearAllThreat via
	// Unit::GetThreatManager — Go's ClearThreat twin.
	methods["ClearThreatList"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		return nil, nil
	}
	// luaAngleTarget resolves the target of the WorldObject angle methods
	// (WorldObjectMethods.h): either a WorldObject Lua object (player,
	// creature, gameobject — all carry X/Y fields) or raw x, y numbers
	// (the GetAngle(x, y) proto).
	luaAngleTarget := func(args []any) (x, y float32, guid uint64, ok bool) {
		if len(args) > 0 {
			if obj, isObj := args[0].(*scripting.Object); isObj && obj != nil {
				if fx, found := objectFloat32Field(obj, "X"); found {
					if fy, found := objectFloat32Field(obj, "Y"); found {
						g, _ := objectUint64Field(obj, "GUID")
						return fx, fy, g, true
					}
				}
				return 0, 0, 0, false
			}
		}
		if len(args) < 2 {
			return 0, 0, 0, false
		}
		x, err := luaFloat32Arg(args, 0)
		if err != nil {
			return 0, 0, 0, false
		}
		y, err = luaFloat32Arg(args, 1)
		if err != nil {
			return 0, 0, 0, false
		}
		return x, y, 0, true
	}
	// GetAngle is WorldObjectMethods::GetAngle (WorldObjectMethods.h:658-678,
	// Trinity arm): Position::GetAbsoluteAngle = NormalizeOrientation(
	// atan2(dy, dx)) (Position.h:126-131); orientation is ignored.
	methods["GetAngle"] = func(_ context.Context, args []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return nil, fmt.Errorf("target is required")
		}
		tx, ty, _, ok := luaAngleTarget(args)
		if !ok {
			return nil, fmt.Errorf("target is required")
		}
		angle := normalizeOrientation(float32(math.Atan2(float64(ty-motion.Y), float64(tx-motion.X))))
		return []any{angle}, nil
	}
	// IsInFront is WorldObjectMethods::IsInFront (WorldObjectMethods.h:1117-1126):
	// WorldObject::isInFront(target, arc) = HasInArc(arc, target)
	// (Object.cpp:1412-1415); Eluna's arc defaults to pi.
	methods["IsInFront"] = func(_ context.Context, args []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return nil, fmt.Errorf("target is required")
		}
		tx, ty, targetGUID, ok := luaAngleTarget(args)
		if !ok {
			return nil, fmt.Errorf("target is required")
		}
		if targetGUID != 0 && targetGUID == state.GUID {
			return []any{true}, nil // Position::HasInArc: always have self in arc (Position.cpp:120)
		}
		arc := float64(math.Pi)
		if len(args) > 1 {
			a, err := luaFloat32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			arc = float64(a)
		}
		return []any{hasInArc(motion.Orientation, motion.X, motion.Y, tx, ty, arc)}, nil
	}
	// IsInBack is WorldObjectMethods::IsInBack (WorldObjectMethods.h:1137-1146):
	// WorldObject::isInBack(target, arc) = !HasInArc(2*pi - arc, target)
	// (Object.cpp:1417-1420); Eluna's arc defaults to pi.
	methods["IsInBack"] = func(_ context.Context, args []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return nil, fmt.Errorf("target is required")
		}
		tx, ty, targetGUID, ok := luaAngleTarget(args)
		if !ok {
			return nil, fmt.Errorf("target is required")
		}
		if targetGUID != 0 && targetGUID == state.GUID {
			return []any{false}, nil // self is always in the frontal arc, never behind it
		}
		arc := float64(math.Pi)
		if len(args) > 1 {
			a, err := luaFloat32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			arc = float64(a)
		}
		return []any{!hasInArc(motion.Orientation, motion.X, motion.Y, tx, ty, 2*math.Pi-arc)}, nil
	}
	// GetRelativePoint is WorldObjectMethods::GetRelativePoint
	// (WorldObjectMethods.h:630-646): WorldObject::GetClosePoint(x, y, z,
	// 0, dist, relAngle) = GetNearPoint with absAngle = orientation +
	// relAngle at (combatReach + dist) (Object.cpp:3307-3311, 3272-3269).
	// The height-collision re-solve (UpdateAllowedPositionZ/IsWithinLOS)
	// has no Go model, so z stays the creature's z.
	methods["GetRelativePoint"] = func(_ context.Context, args []any) ([]any, error) {
		dist, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		rad, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return []any{float32(0), float32(0), float32(0)}, nil
		}
		r := motion.CombatReach + dist
		a := motion.Orientation + rad
		return []any{motion.X + r*float32(math.Cos(float64(a))), motion.Y + r*float32(math.Sin(float64(a))), motion.Z}, nil
	}
	// GetMap is WorldObjectMethods::GetMap: the DBC-backed Map object for
	// the creature's map/instance (NewMapObject + mapLiveFromStore, the
	// same builder behind the instance hooks' map argument).
	methods["GetMap"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{scripting.NewMapObject(state.Map, state.InstanceID, mapLiveFromStore(s.server.Data, state.Map))}, nil
	}
	// IsElite is CreatureMethods::IsElite: Creature::isElite
	// (Creature.cpp:2344-2350) — rank neither NORMAL (0) nor RARE (4),
	// and not a pet. Go pets are motions with a nonzero OwnerGUID (the
	// pet_combat.go isPet twin).
	methods["IsElite"] = luaNoArgs(func() any {
		if state.Rank == 0 || state.Rank == 4 {
			return false
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.OwnerGUID != 0 {
			return false
		}
		return true
	})
	// IsWorldBoss is CreatureMethods::IsWorldBoss: Creature::isWorldBoss
	// (Creature.cpp:2353-2358) — type_flags & CREATURE_TYPE_FLAG_BOSS_MOB
	// (0x4, SharedDefines.h:2731), not a pet.
	methods["IsWorldBoss"] = luaNoArgs(func() any {
		if state.TypeFlags&0x4 == 0 {
			return false
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.OwnerGUID != 0 {
			return false
		}
		return true
	})
	// IsGuard / IsCivilian / IsTrigger are CreatureMethods.h:98/96/97:
	// Creature::IsGuard / IsCivilian / IsTrigger read flags_extra bits
	// CREATURE_FLAG_EXTRA_GUARD (0x8000) / CIVILIAN (0x2) / TRIGGER
	// (0x80) (CreatureData.h:38-52).
	methods["IsGuard"] = luaNoArgs(func() any { return state.FlagsExtra&0x8000 != 0 })
	methods["IsCivilian"] = luaNoArgs(func() any { return state.FlagsExtra&0x2 != 0 })
	methods["IsTrigger"] = luaNoArgs(func() any { return state.FlagsExtra&0x80 != 0 })
	// IsRacialLeader is CreatureMethods.h:95: GetCreatureTemplate()->RacialLeader.
	methods["IsRacialLeader"] = luaNoArgs(func() any { return state.RacialLeader })
	// GetCreatureFamily is CreatureMethods::GetCreatureFamily: the
	// creature_template.family enum (CreatureData.h:178).
	methods["GetCreatureFamily"] = luaNoArgs(func() any { return state.Family })
	// GetAIName is CreatureMethods::GetAIName (Creature.cpp:2862-2865):
	// the creature_template.AIName column, banked on the motion at spawn.
	methods["GetAIName"] = luaNoArgs(func() any { return state.AIName })
	// GetScriptName is CreatureMethods::GetScriptName (Creature.cpp:2867-2870):
	// the script name the motion was banked with at spawn (ScriptName).
	methods["GetScriptName"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.ScriptName
		}
		return ""
	})
	// GetWanderRadius is CreatureMethods::GetWanderRadius
	// (CreatureMethods.h:424-430, Trinity): Creature::GetWanderDistance —
	// the spawn wander distance, motion.Wander.
	methods["GetWanderRadius"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return float64(motion.Wander)
		}
		return float64(0)
	})
	// SetWanderRadius is CreatureMethods::SetWanderRadius
	// (CreatureMethods.h:1062-1072, Trinity): Creature::SetWanderDistance.
	methods["SetWanderRadius"] = func(_ context.Context, args []any) ([]any, error) {
		dist, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID); motion != nil {
			motion.Wander = float64(dist)
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// GetWaypointPath is CreatureMethods::GetWaypointPath
	// (CreatureMethods.h:440-446, Trinity): the current waypoint path ID,
	// motion.PathID.
	methods["GetWaypointPath"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.PathID
		}
		return uint32(0)
	})
	// GetDefaultMovementType is CreatureMethods::GetDefaultMovementType:
	// m_defaultMovementType (Creature.h:110) — the spawn movement type,
	// motion.MoveType (1 random, 2 waypoint, 0 idle, MovementDefines.h:28-30).
	methods["GetDefaultMovementType"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.MoveType
		}
		return uint32(0)
	})
	// SetDefaultMovementType is Creature.h:110's setter — a pure field
	// write in C++, no generator switch.
	methods["SetDefaultMovementType"] = func(_ context.Context, args []any) ([]any, error) {
		moveType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID); motion != nil {
			motion.MoveType = moveType
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// IsInEvadeMode is CreatureMethods::IsInEvadeMode: motion.Evading —
	// the creature is returning to its spawn position from combat.
	methods["IsInEvadeMode"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		return motion != nil && motion.Evading
	})
	// IsTargetableForAttack is CreatureMethods::IsTargetableForAttack
	// (CreatureMethods.h:55-64): Unit::isTargetableForAttack(checkFakeDeath
	// = mustBeDead) (Unit.cpp:8504-8516) — alive, no NON_ATTACKABLE
	// (0x2) / NOT_SELECTABLE flag, no UNIT_STATE_UNATTACKABLE, and when
	// mustBeDead no UNIT_STATE_DIED (Go: the feign-death flag twin).
	// The NOT_SELECTABLE unit flag and UNIT_STATE_UNATTACKABLE have no Go
	// model, so those arms are vacuous here.
	methods["IsTargetableForAttack"] = func(_ context.Context, args []any) ([]any, error) {
		mustBeDead := false
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				mustBeDead = b
			}
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || motion.Health == 0 {
			return []any{false}, nil
		}
		if motion.UnitFlags&unitFlagNonAttackable != 0 {
			return []any{false}, nil
		}
		if mustBeDead && motion.UnitFlags2&unitFlag2FeignDeath != 0 {
			return []any{false}, nil
		}
		return []any{true}, nil
	}
	// GetNPCFlags is CreatureMethods::GetNPCFlags
	// (CreatureMethods.h:861-865): GetUInt32Value(UNIT_NPC_FLAGS), read
	// from the motion's live NPCFlags (initialized from the template
	// npcflag at spawn).
	methods["GetNPCFlags"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.NPCFlags
		}
		return state.NPCFlags
	})
	// SetNPCFlags is CreatureMethods::SetNPCFlags
	// (CreatureMethods.h:909-915): SetUInt32Value(UNIT_NPC_FLAGS, flags) —
	// the motion field plus the values-update broadcast twin.
	methods["SetNPCFlags"] = func(_ context.Context, args []any) ([]any, error) {
		flags, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion != nil {
			motion.NPCFlags = flags
		}
		s.server.motionMu.Unlock()
		if motion != nil {
			s.server.broadcastCreatureValuesUpdateInInstance(state.Map, state.InstanceID, state.GUID, map[int]uint32{unitFieldNPCFlags: flags})
		}
		return nil, nil
	}
	// IsReputationGainDisabled is CreatureMethods::IsReputationGainDisabled:
	// Creature::IsReputationGainDisabled (Creature.h:321) reads the
	// DisableReputationGain field (default false, Creature.cpp:269).
	methods["IsReputationGainDisabled"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.DisableReputationGain
		}
		return false
	})
	// SetDisableReputationGain is Creature::SetDisableReputationGain
	// (Creature.h:320).
	methods["SetDisableReputationGain"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("disable is required")
		}
		disable, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("disable must be a boolean")
		}
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID); motion != nil {
			motion.DisableReputationGain = disable
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// CanAggro is CreatureMethods::CanAggro (CreatureMethods.h:138-147,
	// Trinity): !HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC)
	// (Unit.h:1045 IsImmuneToNPC, 0x200, UnitDefines.h:133) — the live
	// motion.UnitFlags when banked, the unit_flags template snapshot
	// otherwise.
	methods["CanAggro"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.UnitFlags&unitFlagImmuneToNPC == 0
		}
		return state.UnitFlags&unitFlagImmuneToNPC == 0
	})
	// SetAggroEnabled is CreatureMethods::SetAggroEnabled
	// (CreatureMethods.h:1005-1018, Trinity): allow=true removes
	// UNIT_FLAG_IMMUNE_TO_NPC, allow=false sets it — the motion field plus
	// the values-update broadcast twin.
	methods["SetAggroEnabled"] = func(_ context.Context, args []any) ([]any, error) {
		allow := true
		if len(args) > 0 {
			var ok bool
			allow, ok = args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("allow must be a boolean")
			}
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion != nil {
			if allow {
				motion.UnitFlags &^= unitFlagImmuneToNPC
			} else {
				motion.UnitFlags |= unitFlagImmuneToNPC
			}
		}
		s.server.motionMu.Unlock()
		if motion != nil {
			s.server.broadcastCreatureValuesUpdateInInstance(state.Map, state.InstanceID, state.GUID, map[int]uint32{unitFieldFlags: motion.UnitFlags})
		}
		return nil, nil
	}
	// SetEquipmentSlots is CreatureMethods::SetEquipmentSlots
	// (CreatureMethods.h:982-994, Trinity): SetUInt32Value
	// (UNIT_VIRTUAL_ITEM_SLOT_ID + i, slot) for the three virtual item
	// slots (UpdateFields.h:116, Go index 56) — the values-update
	// broadcast is the SetUInt32Value twin.
	methods["SetEquipmentSlots"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("main_hand, off_hand and ranged are required")
		}
		slots := make([]uint32, 3)
		for i := range slots {
			slot, err := luaUint32Arg(args, i)
			if err != nil {
				return nil, err
			}
			slots[i] = slot
		}
		s.server.broadcastCreatureValuesUpdateInInstance(state.Map, state.InstanceID, state.GUID, map[int]uint32{
			unitVirtualItemSlotID:     slots[0],
			unitVirtualItemSlotID + 1: slots[1],
			unitVirtualItemSlotID + 2: slots[2],
		})
		return nil, nil
	}
	// luaTapMotion resolves the creature motion for the tap/loot-recipient
	// methods below (Creature.cpp:1328-1355 SetLootRecipient state:
	// TapPlayerGUID/TapGroupID).
	luaTapMotion := func() *creatureMotion { return s.luaCreatureMotion(state.GUID, state.Entry) }
	// IsTappedBy is CreatureMethods::IsTappedBy
	// (CreatureMethods.h:104-113, Trinity): Creature::isTappedBy
	// (Creature.cpp:1362-1375) — the player GUID matches the loot
	// recipient, or the player is in the recipient group.
	methods["IsTappedBy"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("player is required")
		}
		target, ok := args[0].(*scripting.Object)
		if !ok || target == nil {
			return nil, fmt.Errorf("player is required")
		}
		playerGUID, ok := objectUint64Field(target, "GUID")
		if !ok || playerGUID == 0 {
			return nil, fmt.Errorf("player is required")
		}
		motion := luaTapMotion()
		if motion == nil {
			return []any{false}, nil
		}
		if playerGUID == motion.TapPlayerGUID {
			return []any{true}, nil
		}
		if motion.TapGroupID == 0 {
			return []any{false}, nil
		}
		if sess := s.server.findSessionByGUID(playerGUID); sess != nil && sess.groupID != 0 && sess.groupID == motion.TapGroupID {
			return []any{true}, nil
		}
		return []any{false}, nil
	}
	// HasLootRecipient is CreatureMethods::HasLootRecipient
	// (CreatureMethods.h:118-128, Trinity): Creature::hasLootRecipient
	// (Creature.h:218) — a loot recipient player or group is set.
	methods["HasLootRecipient"] = luaNoArgs(func() any {
		motion := luaTapMotion()
		return motion != nil && (motion.TapPlayerGUID != 0 || motion.TapGroupID != 0)
	})
	// GetLootRecipient is CreatureMethods::GetLootRecipient: the tapping
	// player object, nil when untapped (Eluna::Push((Player*)nullptr)).
	methods["GetLootRecipient"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil || motion.TapPlayerGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(motion.TapPlayerGUID); player != nil {
			return []any{player}, nil
		}
		return []any{nil}, nil
	}
	// GetLootRecipientGroup is CreatureMethods::GetLootRecipientGroup:
	// the tapping group object, nil when the tap carries no group.
	methods["GetLootRecipientGroup"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil || motion.TapGroupID == 0 {
			return []any{nil}, nil
		}
		if g := groupLuaObject(s.server, s.server.findGroupByID(motion.TapGroupID)); g != nil {
			return []any{g}, nil
		}
		return []any{nil}, nil
	}
	// IsDamageEnoughForLootingAndReward is
	// CreatureMethods::IsDamageEnoughForLootingAndReward:
	// Creature::m_PlayerDamageReq at 0 (Creature.h:322-325), lowered by
	// every player-attributed hit.
	methods["IsDamageEnoughForLootingAndReward"] = luaNoArgs(func() any {
		motion := luaTapMotion()
		return motion != nil && motion.PlayerDamageReq == 0
	})
	// HasSpell is CreatureMethods::HasSpell (CreatureMethods.h:287-294):
	// Creature::HasSpell (Creature.cpp:2730-2733) scans m_spells — Go's
	// motion.Spells slice.
	methods["HasSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		motion := luaTapMotion()
		if motion == nil {
			return []any{false}, nil
		}
		for _, id := range motion.Spells {
			if id == spell {
				return []any{true}, nil
			}
		}
		return []any{false}, nil
	}
	// luaSpellCooldownEnd reads the motion's spell-cooldown expiry
	// (SpellCooldowns carries time.Time expiries, boss_ai.go:2776).
	luaSpellCooldownEnd := func(spell uint32) (time.Time, bool) {
		motion := luaTapMotion()
		if motion == nil {
			return time.Time{}, false
		}
		end, ok := motion.SpellCooldowns[spell]
		return end, ok && time.Now().Before(end)
	}
	// HasSpellCooldown is CreatureMethods::HasSpellCooldown
	// (CreatureMethods.h:321-331, Trinity):
	// GetSpellHistory()->HasCooldown(spellId).
	methods["HasSpellCooldown"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		_, active := luaSpellCooldownEnd(spell)
		return []any{active}, nil
	}
	// GetCreatureSpellCooldownDelay is
	// CreatureMethods::GetCreatureSpellCooldownDelay
	// (CreatureMethods.h:590-605, Trinity):
	// GetSpellHistory()->GetRemainingCooldown, in milliseconds (0 when no
	// cooldown; the SpellInfo-exists gate has no Go twin — the Go model
	// tracks cooldowns without spell DBC rows).
	methods["GetCreatureSpellCooldownDelay"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		end, active := luaSpellCooldownEnd(spell)
		if !active {
			return []any{uint32(0)}, nil
		}
		remaining := end.Sub(time.Now())
		if remaining < 0 {
			remaining = 0
		}
		return []any{uint32(remaining.Milliseconds())}, nil
	}
	// HasCategoryCooldown is CreatureMethods::HasCategoryCooldown
	// (CreatureMethods.h:260-279, Trinity): the spell has a nonzero
	// category and is on cooldown.
	methods["HasCategoryCooldown"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		motion := luaTapMotion()
		if motion == nil || motion.SpellCooldownCategories[spell] == 0 {
			return []any{false}, nil
		}
		_, active := luaSpellCooldownEnd(spell)
		return []any{active}, nil
	}
	// GetAITargetsCount is CreatureMethods::GetAITargetsCount
	// (CreatureMethods.h:841-853, Trinity): the threatened-by-me list size.
	methods["GetAITargetsCount"] = luaNoArgs(func() any {
		motion := luaTapMotion()
		if motion == nil || motion.ThreatMgr == nil {
			return 0
		}
		return len(motion.ThreatMgr.SortedEntries())
	})
	// GetAITargets is CreatureMethods::GetAITargets
	// (CreatureMethods.h:807-839, Trinity): the threatened-by-me victims
	// as Unit objects (players first-resolved, then creatures).
	methods["GetAITargets"] = func(ctx context.Context, _ []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil || motion.ThreatMgr == nil {
			return []any{[]any{}}, nil
		}
		targets := []any{}
		for _, entry := range motion.ThreatMgr.SortedEntries() {
			if player := s.server.findPlayer(entry.VictimGUID); player != nil {
				targets = append(targets, player)
				continue
			}
			if creature := s.luaCreature(ctx, entry.VictimGUID); creature != nil {
				targets = append(targets, creature)
			}
		}
		return []any{targets}, nil
	}
	// Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242).
	luaWorldObjectSoundMethods(s.server, nil, state.GUID, methods)
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
	state, ok := s.server.loadLuaGameObject(ctx, low, entry)
	if !ok || s.player == nil || state.Map != s.player.Map {
		return nil
	}
	state.InstanceID = s.player.InstanceID
	if s.server.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) {
		return nil
	}
	return s.server.luaGameObjectObject(state)
}

func (srv *Server) loadLuaGameObject(ctx context.Context, low, entry uint32) (luaGameObjectState, bool) {
	var state luaGameObjectState
	var mapID int64
	if err := srv.WorldStore.DB.QueryRowContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.map, g.position_x, g.position_y, g.position_z FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.guid = ? AND g.id = ?", low, entry).Scan(&state.LowGUID, &state.Entry, &state.DisplayID, &state.Name, &mapID, &state.X, &state.Y, &state.Z); err != nil {
		return state, false
	}
	state.Map, state.GUID = uint32(mapID), gameObjectGUID(state.LowGUID, state.Entry)
	state.GoState, state.LootState = 1, 1
	return state, true
}

func (srv *Server) luaGameObjectObject(state luaGameObjectState) *scripting.Object {
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetEntry"] = luaNoArgs(func() any { return state.Entry })
	methods["GetDisplayId"] = luaNoArgs(func() any { return state.DisplayID })
	methods["GetDBTableGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	methods["GetGoState"] = luaNoArgs(func() any { return state.GoState })
	methods["GetLootState"] = luaNoArgs(func() any { return state.LootState })
	methods["IsSpawned"] = luaNoArgs(func() any { return !srv.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) })
	methods["IsActive"] = luaNoArgs(func() any { return state.GoState == 0 })
	methods["GetGUID"] = luaNoArgs(func() any { return state.GUID })
	methods["GetGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	methods["GetObjectType"] = luaNoArgs(func() any { return "GameObject" })
	methods["GetMapId"] = luaNoArgs(func() any { return state.Map })
	methods["GetX"] = luaNoArgs(func() any { return state.X })
	methods["GetY"] = luaNoArgs(func() any { return state.Y })
	methods["GetZ"] = luaNoArgs(func() any { return state.Z })
	methods["RemoveFromWorld"] = func(_ context.Context, _ []any) ([]any, error) {
		srv.setGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID, true)
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
	// UseDoorOrButton is LuaGameObject::UseDoorOrButton
	// (GameObjectMethods.h): the delay arg defaults to 0, which falls back
	// to the template autoCloseTime (GameObject::UseDoorOrButton,
	// GameObject.cpp:1434).
	methods["UseDoorOrButton"] = func(_ context.Context, args []any) ([]any, error) {
		var delay uint32
		if len(args) > 0 {
			value, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			delay = value
		}
		srv.useDoorOrButton(state.Map, state.InstanceID, state.GUID, false, delay)
		return nil, nil
	}
	// IsTransport is LuaGameObject::IsTransport (GameObjectMethods.h):
	// GameObject::IsTransport (GameObject.cpp) returns true for template
	// type TRANSPORT (11) or MO_TRANSPORT (15) (SharedDefines.h:1605/1609).
	methods["IsTransport"] = luaNoArgs(func() any {
		if dyn := srv.gameObjectState(state.Map, state.InstanceID, state.GUID); dyn != nil {
			return dyn.Type == GameObjectTypeTransport || dyn.Type == GameObjectTypeMOTransport
		}
		t := srv.gameObjectTemplateType(state.Entry)
		return t == GameObjectTypeTransport || t == GameObjectTypeMOTransport
	})
	methods["Despawn"] = func(_ context.Context, _ []any) ([]any, error) {
		srv.objectsMuLockGameObject(state.Map, state.InstanceID, state.GUID)
		return nil, nil
	}
	methods["Respawn"] = func(_ context.Context, _ []any) ([]any, error) {
		srv.objectsMuUnlockGameObject(state.Map, state.InstanceID, state.GUID)
		return nil, nil
	}
	// Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242).
	luaWorldObjectSoundMethods(srv, nil, state.GUID, methods)
	return &scripting.Object{Type: "GameObject", Fields: map[string]any{"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry, "Map": state.Map, "MapId": state.Map, "X": state.X, "Y": state.Y, "Z": state.Z, "DisplayID": state.DisplayID, "GoState": state.GoState, "LootState": state.LootState, "InWorld": true}, Methods: methods}
}

func (srv *Server) objectsMuLockGameObject(mapID, instanceID uint32, guid uint64) {
	srv.setGameObjectHiddenInInstance(mapID, instanceID, guid, true)
}
func (srv *Server) objectsMuUnlockGameObject(mapID, instanceID uint32, guid uint64) {
	srv.setGameObjectHiddenInInstance(mapID, instanceID, guid, false)
}

func (s *session) objectsMuLockGameObject(mapID, instanceID uint32, guid uint64) {
	s.server.objectsMuLockGameObject(mapID, instanceID, guid)
}
func (s *session) objectsMuUnlockGameObject(mapID, instanceID uint32, guid uint64) {
	s.server.objectsMuUnlockGameObject(mapID, instanceID, guid)
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
	return s.server.luaGameObjectObject(nearest)
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
