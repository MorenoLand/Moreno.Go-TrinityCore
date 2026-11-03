package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
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
	Class        uint32 // creature_template.unit_class, backs Lua Unit::GetClassMask
	FlagsExtra   uint32
	TypeFlags    uint32
	Family       uint32
	CreatureType uint32 // creature_template.type, backs Lua Unit::GetCreatureType
	UnitFlags    uint32
	AIName       string
	RacialLeader bool
}

type luaGameObjectState struct {
	GUID        uint64
	OwnerGUID   uint64
	LowGUID     uint32
	Entry       uint32
	DisplayID   uint32
	Size        float32
	Name        string
	Map         uint32
	InstanceID  uint32
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Type        uint32
	GoState     uint32
	LootState   uint32
	PhaseMask   uint32
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
// visibility set (the sendSpellStart/sendSpellMiss convention: self write plus
// visibility-set fan-out skipping self; creatures and gameobjects have no session,
// so they broadcast with a nil source like the creature-text sound arm).
// The sSoundEntriesStore existence gate has no Go bridge (no SoundEntries.dbc
// model, same documented-blocked state as handleEffectPlaySound): an unknown
// id plays nothing client-side.
func (srv *Server) luaDeliverSound(self *session, sourceGUID uint64, opcode uint16, payload []byte, args []any) error {
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
		srv.broadcastToVisibleSetFromSession(self, opcode, payload)
		return nil
	}
	if mapID, instanceID, x, y, z, phase, ok := srv.luaSoundSourcePosition(sourceGUID); ok {
		srv.broadcastToVisibleSetFromPhase(opcode, payload, mapID, instanceID, x, y, z, phase)
		return nil
	}
	srv.broadcastToNearby(opcode, payload, nil)
	return nil
}

// luaSoundSourcePosition resolves a nil-session sound source (creature or
// gameobject) to its live position and phase for the visibility-set fan-out: creature
// motions are scanned across instances (the handler surface carries no map),
// gameobject states across both dynamic maps. Static gameobjects live in no
// runtime map, so they report not-found and the caller keeps the map-wide
// fan-out rather than dropping the sound.
func (srv *Server) luaSoundSourcePosition(sourceGUID uint64) (mapID, instanceID uint32, x, y, z float32, phase uint32, ok bool) {
	if srv == nil || sourceGUID == 0 {
		return 0, 0, 0, 0, 0, 0, false
	}
	srv.motionMu.Lock()
	for scope := range srv.instanceCreatureMotion {
		if m := srv.findCreatureMotionLocked(scope.MapID, scope.InstanceID, sourceGUID); m != nil {
			mapID, instanceID, x, y, z, phase = m.Map, m.InstanceID, m.X, m.Y, m.Z, m.PhaseMask
			ok = true
			break
		}
	}
	srv.motionMu.Unlock()
	if ok {
		return mapID, instanceID, x, y, z, phase, true
	}
	srv.objectsMu.RLock()
	defer srv.objectsMu.RUnlock()
	if st := srv.dynamicGameObjects[sourceGUID]; st != nil {
		return st.Map, st.InstanceID, st.X, st.Y, st.Z, st.PhaseMask, true
	}
	for _, byGUID := range srv.instanceGameObjects {
		if st := byGUID[sourceGUID]; st != nil {
			return st.Map, st.InstanceID, st.X, st.Y, st.Z, st.PhaseMask, true
		}
	}
	return 0, 0, 0, 0, 0, 0, false
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
		return nil, srv.luaDeliverSound(self, sourceGUID, uint16(protocol.OpcodeSMSG_PLAY_MUSIC), pkt.Bytes(), args)
	}
	methods["PlayDirectSound"] = func(_ context.Context, args []any) ([]any, error) {
		soundID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return nil, srv.luaDeliverSound(self, sourceGUID, uint16(protocol.OpcodeSMSG_PLAY_SOUND), buildPlaySound(soundID), args)
	}
	methods["PlayDistanceSound"] = func(_ context.Context, args []any) ([]any, error) {
		soundID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return nil, srv.luaDeliverSound(self, sourceGUID, uint16(protocol.OpcodeSMSG_PLAY_OBJECT_SOUND), buildPlayObjectSound(soundID, sourceGUID), args)
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
	var rank, flagsExtra, typeFlags, family, unitFlags, racialLeader, unitClass, creatureType int64
	var aiName string
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, "+npcFlagExpr+", t.rank, t.flags_extra, t.type_flags, t.unit_class, t.family, t.type, t.unit_flags, t.AIName, t.RacialLeader, c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", queryArgs...).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &rank, &flagsExtra, &typeFlags, &unitClass, &family, &creatureType, &unitFlags, &aiName, &racialLeader, &state.Map, &state.X, &state.Y, &state.Z)
	if err != nil && missingTable(err) {
		err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, t.name, COALESCE(NULLIF(c.modelid, 0), t.modelid1), t.gossip_menu_id, t.npcflag, t.rank, t.flags_extra, t.type_flags, t.unit_class, t.family, t.type, t.unit_flags, t.AIName, t.RacialLeader, c.map, c.position_x, c.position_y, c.position_z FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ? AND c.id = ?", low, entry).Scan(&state.GUID, &state.Entry, &state.Name, &displayID, &gossipMenuID, &npcFlags, &rank, &flagsExtra, &typeFlags, &unitClass, &family, &creatureType, &unitFlags, &aiName, &racialLeader, &state.Map, &state.X, &state.Y, &state.Z)
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
	npcFlagsKnown, liveNPCFlags := motion.NPCFlagsKnown, motion.NPCFlags
	s.server.motionMu.Unlock()
	state.GossipMenuID, state.NPCFlags = uint32(gossipMenuID), uint32(npcFlags)
	if npcFlagsKnown {
		// A Lua arm overrode the live NPC flags (event-surface SetNPCFlags /
		// SetFlag(82) / RemoveFlag(82)): the server-side hello gates
		// (gossip.go, quest_details.go, taxi.go) must see the live value,
		// not the DB-template snapshot, or e.g. a questgiver flag set on
		// Reset would still fail the quest-hello gate. Untouched creatures
		// keep the DB/game-event value above (a Lua override intentionally
		// replaces the game-event OR, matching C++ SetUInt32Value
		// semantics).
		state.NPCFlags = liveNPCFlags
	}
	state.Rank, state.FlagsExtra, state.TypeFlags, state.Family, state.UnitFlags = uint32(rank), uint32(flagsExtra), uint32(typeFlags), uint32(family), uint32(unitFlags)
	state.CreatureType = uint32(creatureType)
	state.Class = uint32(unitClass)
	state.AIName, state.RacialLeader = aiName, racialLeader != 0
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetEntry"] = luaNoArgs(func() any { return state.Entry })
	methods["GetDisplayId"] = luaNoArgs(func() any { return state.DisplayID })
	// GetNativeDisplayId is Unit::GetNativeDisplayId (Unit.h:1474):
	// the live motion native when the motion is up (a handler
	// SetNativeDisplayId override wins), else the spawn model this
	// snapshot was built from. DELTA: this surface's GetDisplayId
	// already returns the spawn model, so a live morph is invisible
	// here until the motion overlay path serves it.
	methods["GetNativeDisplayId"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.NativeDisplayID != 0 {
			return motion.NativeDisplayID
		}
		return state.DisplayID
	})
	// SetNativeDisplayId is Unit::SetNativeDisplayId (Unit.h:1476)
	// through the motion lock, mirroring SetHealth's write-through
	// shape; a despawned motion is a silent no-op.
	methods["SetNativeDisplayId"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion == nil || motion.Entry != state.Entry {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		motion.NativeDisplayID = id
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.server.motionMu.Unlock()
		s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNativeDisplayID: id})
		return nil, nil
	}
	// DeMorph is Unit::DeMorph (Unit.cpp:3321-3323) through the
	// shared display fold; the spawn model stands in for a zero
	// motion native.
	methods["DeMorph"] = func(ctx context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return nil, nil
		}
		native := motion.NativeDisplayID
		if native == 0 {
			native = state.DisplayID
		}
		if native == 0 {
			return nil, nil
		}
		s.server.setMotionDisplayID(ctx, motion, native)
		return nil, nil
	}
	// GetCreatureType is Unit::GetCreatureType (Unit.cpp:9124-9149)
	// for creatures: the template type snapshot.
	methods["GetCreatureType"] = luaNoArgs(func() any { return state.CreatureType })
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
	// HealthBelowPct/HealthAbovePct/CountPctFromCurHealth/
	// CountPctFromMaxHealth over the same construction snapshot
	// GetHealth/GetHealthPct read on this surface, replacing the
	// generic fallback's Fields read with builder-owned methods.
	for name, fn := range unitHealthPctMethods(func() (uint32, uint32) { return state.Health, state.MaxHealth }) {
		methods[name] = fn
	}
	methods["GetLevel"] = luaNoArgs(func() any { return state.Level })
	methods["IsDead"] = luaNoArgs(func() any { return state.Health == 0 })
	// IsDying is UnitMethods::IsDying (UnitMethods.h:316-324): Unit::isDying
	// (Unit.h:1141) reads m_deathState == JUST_DIED. For creatures that state
	// is unobservable from Lua: Creature::setDeathState (Creature.cpp:
	// 2057-2115) promotes JUST_DIED to CORPSE synchronously inside the same
	// call, before KilledUnit/JustDied hooks fire, so this always reads false.
	// The player leg (lua_player.go) reads the death-to-release window instead.
	methods["IsDying"] = luaNoArgs(func() any { return false })
	// MoveFleeing is UnitMethods::MoveFleeing (UnitMethods.h:2254-2260):
	// MotionMaster::MoveFleeing(target, time) (MotionMaster.cpp:618-633).
	// A nil/positionless target errors (the Eluna CHECKOBJ leg); a time of 0
	// arms the untimed generator leg, which flees until SetFeared(false)
	// clears it. A motion that no longer resolves is a no-op. The C++
	// generator's continuous repathing is a single 24yd spline here, the
	// critter-flee delta. The player surface stays out: player movement is
	// client-driven and Go has no player flee spline.
	methods["MoveFleeing"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("MoveFleeing requires a WorldObject target")
		}
		target, ok := args[0].(*scripting.Object)
		if !ok || target == nil {
			return nil, fmt.Errorf("MoveFleeing requires a WorldObject target")
		}
		tx, ok := objectFloat32Field(target, "X")
		if !ok {
			return nil, fmt.Errorf("MoveFleeing requires a WorldObject target with a position")
		}
		ty, ok := objectFloat32Field(target, "Y")
		if !ok {
			return nil, fmt.Errorf("MoveFleeing requires a WorldObject target with a position")
		}
		var durationMs uint32
		if len(args) >= 2 && args[1] != nil {
			d, err := luaUint32Arg(args, 1)
			if err != nil {
				return nil, err
			}
			durationMs = d
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.startCreatureFlee(motion, tx, ty, durationMs)
		}
		return nil, nil
	}
	// SetFeared is UnitMethods::SetFeared (UnitMethods.h:1964-1973) for
	// creatures: Unit::SetControlled(apply, UNIT_STATE_FLEEING) (Unit.cpp:
	// 11513-11560) via the setCreatureFeared engine leg. The player
	// surface stays out (client control + no player flee spline).
	methods["SetFeared"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetFeared requires a boolean")
			}
			apply = b
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.setCreatureFeared(motion, apply)
		}
		return nil, nil
	}
	// CallAssistance is Creature::CallAssistance (Creature.cpp:2402-2425)
	// through the engine's callCreatureAssistance funnel, the
	// session-creature twin of the handler binding. A motion that no
	// longer resolves is a no-op.
	methods["CallAssistance"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.callCreatureAssistance(ctx, motion)
		}
		return nil, nil
	}

	// CallForHelp is Creature::CallForHelp (Creature.cpp:2432-2456) via
	// the callCreatureForHelp funnel, the session-creature twin of the
	// handler binding; a motion that no longer resolves is a no-op.
	methods["CallForHelp"] = func(ctx context.Context, args []any) ([]any, error) {
		radius, err := luaGenericFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.callCreatureForHelp(ctx, motion, radius)
		}
		return nil, nil
	}
	// HasSearchedAssistance is Creature::HasSearchedAssistance
	// (Creature.h:248), the session-creature twin of the handler
	// binding; a motion that no longer resolves reads false.
	methods["HasSearchedAssistance"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.AlreadySearchedAssistance
		}
		return false
	})
	// SetNoSearchAssistance is Creature::SetNoSearchAssistance
	// (Creature.h:247) through the motion lock, mirroring
	// SetNativeDisplayId's write-through shape; a despawned motion is
	// a silent no-op. Omitted argument defaults true, the Eluna
	// CHECKVAL default.
	methods["SetNoSearchAssistance"] = func(_ context.Context, args []any) ([]any, error) {
		val := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetNoSearchAssistance requires a bool")
			}
			val = b
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion != nil && motion.Entry == state.Entry {
			motion.AlreadySearchedAssistance = val
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// GetRespawnDelay is Creature::GetRespawnDelay (Creature.h:262),
	// the session-creature twin of the handler binding: the
	// SetRespawnDelay override while known, else the
	// creature.spawntimesecs DB row.
	methods["GetRespawnDelay"] = func(ctx context.Context, _ []any) ([]any, error) {
		return []any{s.server.creatureRespawnDelaySeconds(ctx, state.Map, state.InstanceID, state.GUID)}, nil
	}
	// SetRespawnDelay is Creature::SetRespawnDelay (Creature.h:263)
	// through the motion lock, mirroring SetNativeDisplayId's
	// write-through shape; a despawned motion is a silent no-op. It
	// writes no DB row, matching C++.
	methods["SetRespawnDelay"] = func(_ context.Context, args []any) ([]any, error) {
		delay, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion != nil && motion.Entry == state.Entry {
			motion.RespawnDelaySec = delay
			motion.RespawnDelayKnown = true
		}
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// CanAssistTo is Creature::CanAssistTo (CreatureMethods.h:76-88,
	// Creature.cpp:2458-2496) via creatureCanAssistTo, the
	// session-creature twin of the handler binding. A motion that no
	// longer resolves reads false; unresolvable friend/enemy
	// arguments error, the Eluna CHECKOBJ twin.
	methods["CanAssistTo"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CanAssistTo requires a friend and an enemy Unit")
		}
		friend := s.server.resolveAssistParty(args[0], state.Map, state.InstanceID)
		if !friend.ok {
			return nil, fmt.Errorf("CanAssistTo requires a friend Unit")
		}
		enemy := s.server.resolveAssistParty(args[1], state.Map, state.InstanceID)
		if !enemy.ok {
			return nil, fmt.Errorf("CanAssistTo requires an enemy Unit")
		}
		checkfaction := true
		if len(args) >= 3 && args[2] != nil {
			b, ok := args[2].(bool)
			if !ok {
				return nil, fmt.Errorf("CanAssistTo requires a bool checkFaction")
			}
			checkfaction = b
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return []any{false}, nil
		}
		s.server.motionMu.Lock()
		faction, passive, health, evading, inCombat, unitFlags, ownerGUID, charmerGUID := motion.Faction, isCreaturePassive(motion), motion.Health, motion.Evading, motion.InCombat, motion.UnitFlags, motion.OwnerGUID, motion.CharmerGUID
		s.server.motionMu.Unlock()
		return []any{s.server.creatureCanAssistTo(faction, passive, health, evading, inCombat, unitFlags, ownerGUID, charmerGUID, friend, enemy, checkfaction)}, nil
	}
	// CanSwim is Creature::CanSwim (Creature.h:105), the
	// session-creature twin of the handler binding: template Swim or
	// pet (nonzero OwnerGUID). A motion that no longer resolves reads
	// the C++ missing-row default (true), like the stat-less motions.
	methods["CanSwim"] = luaNoArgs(func() any {
		return creatureCanSwimMotion(s.luaCreatureMotion(state.GUID, state.Entry))
	})
	// CanWalk is Creature::CanWalk (Creature.h:104), the
	// session-creature twin of the handler binding: template Ground
	// != None. A motion that no longer resolves reads the C++
	// missing-row default (true).
	methods["CanWalk"] = luaNoArgs(func() any {
		return creatureCanWalkMotion(s.luaCreatureMotion(state.GUID, state.Entry))
	})
	// CanStartAttack is Creature::CanStartAttack (Creature.cpp:1960-2004),
	// the session-creature twin of the handler binding. A motion that
	// no longer resolves reads false; an unresolvable target errors,
	// the Eluna CHECKOBJ twin.
	methods["CanStartAttack"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("CanStartAttack requires a Unit target")
		}
		target := s.server.creatureAttackTargetResolve(args[0], state.Map, state.InstanceID)
		if !target.ok {
			return nil, fmt.Errorf("CanStartAttack requires a Unit target")
		}
		force := true
		if len(args) >= 2 && args[1] != nil {
			b, ok := args[1].(bool)
			if !ok {
				return nil, fmt.Errorf("CanStartAttack requires a bool force")
			}
			force = b
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return []any{false}, nil
		}
		return []any{s.server.creatureCanStartAttack(ctx, motion, target, force)}, nil
	}
	// FleeToGetAssistance is Creature::DoFleeToGetAssistance
	// (Creature.cpp:1031-1052), the session-creature twin of the
	// handler binding; a motion that no longer resolves is a no-op.
	methods["FleeToGetAssistance"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.creatureFleeToGetAssistance(ctx, motion)
		}
		return nil, nil
	}
	// GetScriptId is Creature::GetScriptId (Creature.cpp:2872-2879),
	// the session-creature twin of the handler binding. The session
	// builder only returns live motions, so the template ScriptName
	// always resolves; see the registry comment for the numbering
	// delta vs C++'s sorted-store indices.
	methods["GetScriptId"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return scriptIDForName(motion.ScriptName)
		}
		return uint32(0)
	})
	// SetDeathState is Creature::setDeathState (Creature.cpp:2057-2153
	// over Unit::setDeathState, Unit.cpp:8901-8960) via
	// applyCreatureDeathState, the session-creature twin of the handler
	// binding; a motion that no longer resolves is a no-op.
	methods["SetDeathState"] = func(ctx context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if st > 4 {
			return nil, fmt.Errorf("SetDeathState requires a DeathState 0-4, got %d", st)
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.applyCreatureDeathState(ctx, motion, uint8(st))
		}
		return nil, nil
	}
	// IsInWater/IsUnderWater are Unit::IsInWater/IsUnderWater
	// (Unit.cpp:3276-3284) via the grid-leg liquid status, over the
	// builder's live-motion snapshot coords like its other reads.
	methods["IsInWater"] = luaNoArgs(func() any {
		if s == nil || s.server == nil {
			return false
		}
		return s.server.mapIsInWater(state.Map, state.X, state.Y, state.Z)
	})
	methods["IsUnderWater"] = luaNoArgs(func() any {
		if s == nil || s.server == nil {
			return false
		}
		return s.server.mapIsUnderWater(state.Map, state.X, state.Y, state.Z)
	})
	// GetCorpseDelay / RemoveCorpse are the session-bound twins of the
	// handler-creature bindings (lua_creature_events.go): the rank-mapped
	// corpse-decay delay, and the direct Creature::RemoveCorpse port in
	// kill.go (a motion that no longer resolves is a no-op there).
	methods["GetCorpseDelay"] = func(ctx context.Context, _ []any) ([]any, error) {
		return []any{s.server.creatureCorpseDelaySeconds(ctx, state.Entry)}, nil
	}
	methods["RemoveCorpse"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.removeCreatureCorpseNow(ctx, motion)
		}
		return nil, nil
	}
	// GetVehicleKit / IsOnVehicle are the session-creature twins of the
	// motion-creature arms in lua_creature_events.go (UnitMethods.h;
	// GetVehicle stays out, commented out in LuaFunctions.cpp).
	methods["GetVehicleKit"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{s.server.luaVehicleObject(state.Map, state.InstanceID, state.GUID)}, nil
	}
	methods["IsOnVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || motion.VehicleGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.server.luaVehicleObject(state.Map, state.InstanceID, motion.VehicleGUID)}, nil
	}
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
	methods["IsInnkeeper"] = hasNPCFlag(0x00010000) // UNIT_NPC_FLAG_INNKEEPER (UnitDefines.h:202); was 0x1000 (REPAIR)
	methods["IsAuctioneer"] = hasNPCFlag(0x00200000)
	methods["IsArmorer"] = hasNPCFlag(0x00001000)
	methods["IsBanker"] = hasNPCFlag(0x00020000)
	methods["IsBattleMaster"] = hasNPCFlag(0x00100000)
	methods["IsGuildMaster"] = hasNPCFlag(0x00040000)
	methods["IsTabardDesigner"] = hasNPCFlag(0x00080000)
	// GetClassMask/GetRaceMask are Unit::GetClassMask/GetRaceMask
	// (Unit.h:849/852): class from template unit_class, race 0 (the
	// creature race byte Creature::InitEntry never sets). IsPvPFlagged
	// is Unit::IsPvP (Unit.h:914) over the live motion bytes2 byte 1;
	// a despawned motion reads false.
	methods["GetClassMask"] = luaNoArgs(func() any { return unitClassRaceMask(state.Class) })
	methods["GetRaceMask"] = luaNoArgs(func() any { return uint32(0) })
	methods["IsPvPFlagged"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return false
		}
		return motion.PVPFlags&pvpFlagPvP != 0
	})
	// GetClassAsString is Unit::GetClassAsString (UnitMethods.h:1152):
	// ChrClasses Name[locale] for the template unit_class, nil on a
	// missing row. GetRaceAsString reads nil: the creature race byte
	// Creature::InitEntry never sets, so the C++ ChrRaces lookup for
	// race 0 finds no row either. GetControllerGUID/GetControllerGUIDS
	// are Unit::GetCharmerOrOwnerGUID (Unit.h:1170) and
	// WorldObject::GetCharmerOrOwnerOrOwnGUID (Object.cpp:2196) over
	// the live motion's charmer/owner; a motion that no longer
	// resolves reads 0 / the snapshot GUID.
	methods["GetClassAsString"] = func(_ context.Context, args []any) ([]any, error) {
		locale, err := dbcLocaleArg(args)
		if err != nil {
			return nil, err
		}
		name, err := s.server.dbcLocaleName("ChrClasses", state.Class, 4, locale)
		if err != nil {
			return nil, err
		}
		return []any{name}, nil
	}
	methods["GetRaceAsString"] = luaNoArgs(func() any { return nil })
	methods["GetControllerGUID"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return uint64(0)
		}
		return unitControllerGUID(motion.Charmed, motion.CharmerGUID, motion.OwnerGUID)
	})
	// GetCharmerGUID is Unit::GetCharmerGUID (Unit.h:1158):
	// UNIT_FIELD_CHARMEDBY, the motion's charmer slot; a despawned
	// motion reads 0 like the controller twin.
	methods["GetCharmerGUID"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return uint64(0)
		}
		return motion.CharmerGUID
	})
	// GetCharmGUID is Unit::GetCharmedGUID (Unit.h:1161) via the
	// shared instance scan for a motion charmed by this creature.
	methods["GetCharmGUID"] = luaNoArgs(func() any {
		return s.server.unitCharmedGUID(state.Map, state.InstanceID, state.GUID)
	})
	// GetCritterGUID is Unit::GetCritterGUID (Unit.h:1156): the
	// motion's critter slot, 0 unless a script set one.
	methods["GetCritterGUID"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return uint64(0)
		}
		return motion.CritterGUID
	})
	// SetCritterGUID is Unit::SetCritterGUID (UnitMethods.h:1908):
	// the motion critter-slot write, mirroring SetNativeDisplayId's
	// write-through shape; a despawned motion is a silent no-op.
	// No values broadcast: Go carries no creature UNIT_FIELD_CRITTER
	// leg, unlike the native-display field.
	methods["SetCritterGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion == nil || motion.Entry != state.Entry {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		motion.CritterGUID = guid
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// GetMinionGUID is Unit::GetMinionGUID (Unit.h:1151) via the
	// shared instance scan for an uncharmed motion owned by this
	// creature (pets and guardians included, charmed victims not).
	methods["GetMinionGUID"] = luaNoArgs(func() any {
		return s.server.unitMinionGUID(state.Map, state.InstanceID, state.GUID)
	})
	// GetOwnerGUID is Unit::GetOwnerGUID (Unit.h:1147): the
	// UNIT_FIELD_SUMMONEDBY read, here the live motion's OwnerGUID
	// (the SetOwnerGUID write target); a motion that no longer
	// resolves reads 0, the DB-spawn default.
	methods["GetOwnerGUID"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return motion.OwnerGUID
		}
		return uint64(0)
	})
	// IsStopped is Unit::IsStopped (Unit.h:1557): !HasUnitState(
	// UNIT_STATE_MOVING), here the motion's spline-active Moving
	// flag (set at launch, cleared at arrival); a motion that no
	// longer resolves is not moving, so it reads stopped.
	methods["IsStopped"] = luaNoArgs(func() any {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			return !motion.Moving
		}
		return true
	})
	// SetOwnerGUID is Unit::SetOwnerGUID (Unit.cpp:5971): the
	// UNIT_FIELD_SUMMONEDBY write, with C++'s equal-value early
	// return; a despawned motion is a silent no-op. DELTA: C++'s
	// owner-player values-update notify has no Go twin.
	methods["SetOwnerGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion == nil || motion.Entry != state.Entry || motion.OwnerGUID == guid {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		motion.OwnerGUID = guid
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// GetBaseSpellPower is Unit::GetBaseSpellPower (UnitMethods.h:743):
	// the PLAYER_FIELD_MOD_DAMAGE_DONE_POS + school field, which
	// creatures do not carry, so it reads 0. The school arg is still
	// validated like C++'s CHECKVAL, and school >= MAX_SPELL_SCHOOL
	// (7) reads nil like the player twin.
	methods["GetBaseSpellPower"] = func(_ context.Context, args []any) ([]any, error) {
		school, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if school >= 7 {
			return []any{nil}, nil
		}
		return []any{uint32(0)}, nil
	}
	// NearTeleport is Unit::NearTeleport (UnitMethods.h:2690) for
	// session creatures: the NearTeleportTo creature arm
	// (Unit.cpp:13332-13346) — DisableSpline (the in-flight spline
	// twin) + SendTeleportPacket + UpdatePosition — mirroring the
	// handler-creature Teleport twin, re-resolved at call time like
	// the session SendUnit arms. All four CHECKVALs are required, as
	// in C++; a motion that no longer resolves reads a no-op.
	methods["NearTeleport"] = func(_ context.Context, args []any) ([]any, error) {
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
		if s.server == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		motions := s.server.motionMapLocked(state.Map, state.InstanceID)
		m, ok := motions[state.GUID]
		if !ok {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		m.X, m.Y, m.Z, m.Orientation = x, y, z, o
		m.Moving = false
		mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
		phase := m.PhaseMask
		s.server.motionMu.Unlock()
		_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.server.gameTimeMilliseconds(), X: x, Y: y, Z: z, Orientation: o})
		s.server.broadcastCreatureTeleportMovement(mapID, instanceID, guid, x, y, z, nearby, phase)
		s.server.syncCreatureVehiclePassengers(m)
		return nil, nil
	}
	methods["GetControllerGUIDS"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return state.GUID
		}
		if controller := unitControllerGUID(motion.Charmed, motion.CharmerGUID, motion.OwnerGUID); controller != 0 {
			return controller
		}
		return state.GUID
	})
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
	// GetAura is Unit::GetAura (UnitMethods.h) for session creatures:
	// the engine record for the spell, or nil when absent. The Lua
	// AddAura presence set (creatureAuras) carries no record, so a
	// Lua-added aura reads nil here, the motion-creature GetAura
	// convention; see lua_aura.go.
	methods["GetAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID}
		s.server.auraMu.Lock()
		aura := s.server.activeCreatureAuras[key][spell]
		s.server.auraMu.Unlock()
		if aura == nil {
			return []any{nil}, nil
		}
		ref := &luaAuraRef{server: s.server, spellID: spell, creatureKey: key, mapID: state.Map, instanceID: state.InstanceID}
		return []any{ref.luaAuraObject()}, nil
	}
	// IsImmunedToDamage is the mask overload of Unit::IsImmunedToDamage
	// (Unit.cpp:7800-7816) for creatures: the creatureImmuneToDamage twin
	// (script-written IMMUNITY_SCHOOL / IMMUNITY_DAMAGE rows plus the live
	// aura-39/aura-40 misc folds). SPELL_SCHOOL_MASK_NORMAL = 1.
	methods["IsImmunedToDamage"] = func(_ context.Context, args []any) ([]any, error) {
		mask, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID}
		return []any{creatureImmuneToDamage(s.server, key, mask)}, nil
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
	// GetScale/SetScale are the session-creature twins of the
	// motion-creature arms (ObjectMethods.h:134/362): GetScale reads
	// the live scale twin (script override while known, else the
	// template base, else 1.0; a despawned motion reads 1.0) and
	// SetScale writes it through the same motion lock, clearing the
	// override on non-positive values and broadcasting the
	// aura-folded effective value exactly like the handler arm.
	methods["GetScale"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return float64(1)
		}
		if motion.ScriptScaleKnown {
			return float64(motion.ScriptScale)
		}
		if motion.BaseScale > 0 {
			return float64(motion.BaseScale)
		}
		return float64(1)
	})
	methods["SetScale"] = func(_ context.Context, args []any) ([]any, error) {
		scale, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID)
		if motion == nil || motion.Entry != state.Entry {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		if scale <= 0 {
			motion.ScriptScale = 0
			motion.ScriptScaleKnown = false
		} else {
			motion.ScriptScale = scale
			motion.ScriptScaleKnown = true
		}
		base := motion.BaseScale
		if motion.ScriptScaleKnown {
			base = motion.ScriptScale
		}
		if base <= 0 {
			base = 1
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		key := creatureAuraKeyForMotion(motion)
		s.server.motionMu.Unlock()
		effective := float64(base) + float64(s.server.creatureAuraScaleSum(key))/100
		if effective < 0.01 {
			effective = 0.01
		}
		s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{objectFieldScale: math.Float32bits(float32(effective))})
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
	// SendUnitSay/SendUnitYell/SendUnitWhisper/SendUnitEmote are the
	// UnitMethods twins of the handler-surface family in
	// lua_creature_events.go (UnitMethods.h:2320-2390, Unit.cpp:14048-14072),
	// with the same arg shapes, chat types and listen ranges. The phase
	// decision: a session creature is only constructed while a live motion
	// exists, and the indexed-value precedent delegates to the motion
	// backend while it resolves — so these re-resolve the motion at call
	// time and fan out over its live coords and PhaseMask. A motion that
	// has since despawned leaves the object stale (C++ would hold a dead
	// Unit pointer); the call drops silently rather than broadcasting from
	// snapshot coords under a guessed phase. Whisper's receiver with no
	// live session drops silently, the Unit::Whisper null-target arm.
	sessionChatRange := func(chatType uint8, language uint32, message string, listenRange float64) {
		if s == nil || s.server == nil {
			return
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return
		}
		s.server.motionMu.Lock()
		mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
		s.server.motionMu.Unlock()
		packet := protocol.BuildMonsterChatMessage(chatType, language, state.GUID, state.Name, message)
		s.server.broadcastToSetWithinFromPhase(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, mapID, instanceID, x, y, z, listenRange, phase)
	}
	methods["SendUnitSay"] = func(_ context.Context, args []any) ([]any, error) {
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		language, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if len(message) == 0 {
			return []any{}, nil
		}
		sessionChatRange(0x0C, language, message, s.server.Config.ChatListenRangeSay)
		return []any{}, nil
	}
	methods["SendUnitYell"] = func(_ context.Context, args []any) ([]any, error) {
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		language, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if len(message) == 0 {
			return []any{}, nil
		}
		sessionChatRange(0x0E, language, message, s.server.Config.ChatListenRangeYell)
		return []any{}, nil
	}
	methods["SendUnitEmote"] = func(_ context.Context, args []any) ([]any, error) {
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(message) == 0 {
			return []any{}, nil
		}
		chatType := uint8(0x10)
		if len(args) > 2 {
			if bossEmote, ok := args[2].(bool); ok && bossEmote {
				chatType = 0x29
			}
		}
		sessionChatRange(chatType, languageUniversal, message, s.server.Config.ChatListenRangeTextEmote)
		return []any{}, nil
	}
	methods["SendUnitWhisper"] = func(_ context.Context, args []any) ([]any, error) {
		message, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		language, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if len(args) < 3 {
			return nil, fmt.Errorf("argument 3 is required")
		}
		receiverGUID := luaObjectGUID(args[2])
		if receiverGUID == 0 {
			return nil, fmt.Errorf("argument 3 must be a player")
		}
		if len(message) == 0 {
			return []any{}, nil
		}
		if s == nil || s.server == nil {
			return []any{}, nil
		}
		receiver := s.server.findSessionByGUID(receiverGUID)
		if receiver == nil || receiver.player == nil {
			return []any{}, nil
		}
		chatType := uint8(0x0F)
		if len(args) > 3 {
			if bossWhisper, ok := args[3].(bool); ok && bossWhisper {
				chatType = 0x2A
			}
		}
		packet := protocol.BuildChatMessage(chatType, language, state.GUID, receiverGUID, message, "")
		return nil, receiver.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
	}
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554, Trinity):
	// HasUnitState(UNIT_STATE_CASTING). The session creature reads the
	// live cast window (motion.CastingUntil), the same gate the combat
	// tick uses; the player object carries its own arm (lua_player.go).
	methods["IsCasting"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return false
		}
		s.server.motionMu.Lock()
		defer s.server.motionMu.Unlock()
		return time.Now().Before(motion.CastingUntil)
	})
	// CastSpell(target, spellId[, triggered]) is Unit::CastSpell
	// (UnitMethods.h:2411-2432) through the server's creature cast path,
	// the session-creature twin of the handler binding; a nil target
	// falls back to the motion's victim and is a no-op with none, and a
	// despawned motion is a silent no-op.
	methods["CastSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CastSpell requires a target and a spell id")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			s.server.motionMu.Lock()
			targetGUID = motion.TargetGUID
			s.server.motionMu.Unlock()
		}
		if targetGUID == 0 {
			return nil, nil
		}
		triggered := false
		if len(args) >= 3 {
			if b, ok := args[2].(bool); ok {
				triggered = b
			}
		}
		if triggered {
			s.server.castCreatureSpellTriggered(ctx, motion, spellID, targetGUID)
		} else {
			s.server.castCreatureSpell(ctx, motion, spellID, targetGUID)
		}
		return nil, nil
	}
	// CastCustomSpell is the session-creature twin of the handler
	// binding (UnitMethods.h:2449-2480): sparse base-point overrides
	// via the creature funnel; castItem/originalCaster accepted and
	// ignored, same unmodeled legs as the handler surface.
	methods["CastCustomSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CastCustomSpell requires a target and a spell id")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		basePoints, err := luaCastCustomBasePoints(args)
		if err != nil {
			return nil, err
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			s.server.motionMu.Lock()
			targetGUID = motion.TargetGUID
			s.server.motionMu.Unlock()
		}
		if targetGUID == 0 {
			return nil, nil
		}
		triggered := false
		if len(args) >= 3 {
			if b, ok := args[2].(bool); ok {
				triggered = b
			}
		}
		s.server.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, targetGUID, triggered, basePoints, nil, false)
		return nil, nil
	}
	// CastSpellAoF is the session-creature twin of the handler binding
	// (UnitMethods.h:2493-2514): explicit-destination cast, triggered
	// by default.
	methods["CastSpellAoF"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 4 {
			return nil, fmt.Errorf("CastSpellAoF requires x, y, z and a spell id")
		}
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
		spellID, err := luaUint32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		triggered := true
		if len(args) >= 5 {
			if b, ok := args[4].(bool); ok {
				triggered = b
			}
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		dest := [3]float32{x, y, z}
		s.server.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, 0, triggered, nil, &dest, false)
		return nil, nil
	}
	// StopSpellCast is UnitMethods::StopSpellCast (UnitMethods.h:2532-2537):
	// Unit::CastStop (Unit.cpp:980-985) interrupts every current
	// non-melee spell except except_spellid (default 0: all). The
	// single-arm interrupt helpers compose the except leg; MELEE has
	// no Go slot. The prior no-op stub predates the pending/channel
	// cast-state model.
	methods["StopSpellCast"] = func(_ context.Context, args []any) ([]any, error) {
		var spellID uint32
		if len(args) > 0 {
			id, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			spellID = id
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		var genericID uint32
		if motion.PendingInlineSpellID != 0 {
			genericID = motion.PendingInlineSpellID
		} else {
			genericID = motion.PendingCastSpellID
		}
		channelID := uint32(0)
		if s.server.creatureChannelLive(motion) {
			channelID = motion.ChannelSpellID
		}
		var interrupted []interruptedCreatureCast
		if genericID != 0 && genericID != spellID {
			interrupted = append(interrupted, s.server.interruptCreatureGenericCast(motion)...)
		}
		if channelID != 0 && channelID != spellID {
			interrupted = append(interrupted, s.server.interruptCreatureChannelCast(motion)...)
		}
		s.server.motionMu.Unlock()
		for _, arm := range interrupted {
			s.server.sendCreatureInterruptPackets(motion, arm)
		}
		return nil, nil
	}
	// InterruptSpell is the session-creature twin of the handler
	// binding (UnitMethods.h:2547-2570): single-slot interrupt, MELEE
	// and AUTOREPEAT no-ops, delayed arg moot (no DELAYED state).
	methods["InterruptSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spellType, err := luaCastSpellTypeArg(args, 0)
		if err != nil {
			return nil, err
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		var interrupted []interruptedCreatureCast
		switch spellType {
		case 1:
			interrupted = s.server.interruptCreatureGenericCast(motion)
		case 2:
			interrupted = s.server.interruptCreatureChannelCast(motion)
		}
		s.server.motionMu.Unlock()
		for _, arm := range interrupted {
			s.server.sendCreatureInterruptPackets(motion, arm)
		}
		return nil, nil
	}
	// GetCurrentSpell is the session-creature twin of the handler
	// binding (UnitMethods.h:785-792).
	methods["GetCurrentSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spellType, err := luaCastSpellTypeArg(args, 0)
		if err != nil {
			return nil, err
		}
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return []any{nil}, nil
		}
		s.server.motionMu.Lock()
		spell := s.server.luaCreatureCurrentSpell(motion, spellType)
		s.server.motionMu.Unlock()
		return []any{spell}, nil
	}
	// InterruptNonMeleeSpells is Unit::InterruptNonMeleeSpells
	// (Unit.cpp:3212-3225): the session-creature twin of the handler
	// binding — cancels the pending generic cast and the live channel,
	// clearing the plain gate when no arm returns.
	methods["InterruptNonMeleeSpells"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		interrupted := s.server.interruptCreatureNonMeleeCasts(motion)
		if len(interrupted) == 0 {
			motion.CastingUntil = time.Time{}
			s.server.cancelPendingCreatureCast(motion)
		}
		s.server.motionMu.Unlock()
		for _, arm := range interrupted {
			s.server.sendCreatureInterruptPackets(motion, arm)
		}
		return nil, nil
	}
	// SendChatMessageToPlayer is UnitMethods::SendChatMessageToPlayer
	// (UnitMethods.h:2095-2113), shared in lua_worldobject_range.go;
	// creatures carry no chat tag, so the tag byte is 0.
	methods["SendChatMessageToPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		return s.server.luaSendChatMessageToPlayer(state.GUID, 0, args)
	}
	// IsAttackingPlayer is Unit::isAttackingPlayer (Unit.cpp:5849-5865),
	// shared in lua_worldobject_range.go; a motion that has since
	// despawned reads false, the stale-motion precedent of the
	// SendUnit* family above.
	methods["IsAttackingPlayer"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil || s.server == nil {
			return []any{false}, nil
		}
		return []any{s.server.creatureIsAttackingPlayer(motion.Map, motion.InstanceID, motion.GUID)}, nil
	}
	// GetHomePosition is CreatureMethods::GetHomePosition
	// (CreatureMethods.h:630-644): x, y, z, o from the creature's HOME
	// position (Creature::GetHomePosition, Creature.h:296-297); the 4th
	// value is the home orientation, NOT the live facing.
	methods["GetHomePosition"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return []any{float32(0), float32(0), float32(0), float32(0)}, nil
		}
		return []any{motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO}, nil
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
			motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO = x, y, z, o
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
		// HandleEmoteCommand fans out via SendMessageToSet (Unit.cpp),
		// i.e. the visibility set around the creature, not the whole map.
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil {
			s.server.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
		} else {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), nil)
		}
		return nil, nil
	}
	// ClearThreatList is UnitMethods::ClearThreatList
	// (UnitMethods.h:1992-1999): ThreatManager::ClearAllThreat via
	// Unit::GetThreatManager — Go's ClearThreat twin. WARNING: this WIPES
	// the threat table (evade semantics); it is NOT ScriptedAI::
	// ResetThreatList — use ResetAllThreat for that.
	methods["ClearThreatList"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		return nil, nil
	}
	// ResetAllThreat is the session-surface twin of the handler-surface
	// bridge (lua_creature_events.go): ThreatManager::ResetAllThreat —
	// zero threat values, keep entries, no evade. ScriptedAI::
	// ResetThreatList lowers to this, not to ClearAllThreat.
	methods["ResetAllThreat"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.ThreatMgr != nil {
			motion.ThreatMgr.ResetThreat()
		}
		return nil, nil
	}
	// GetThreat / ModifyThreatByPercent are the session-surface twins
	// of the handler-surface bridges (lua_creature_events.go):
	// ThreatManager::GetThreat and ModifyThreatByPercent
	// (ThreatManager.h:117,143).
	methods["GetThreat"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{float32(0)}, nil
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.ThreatMgr != nil {
			if victimGUID := luaObjectGUID(args[0]); victimGUID != 0 {
				return []any{motion.ThreatMgr.GetThreat(victimGUID)}, nil
			}
		}
		return []any{float32(0)}, nil
	}
	methods["ModifyThreatByPercent"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, nil
		}
		victimGUID := luaObjectGUID(args[0])
		if victimGUID == 0 {
			return nil, nil
		}
		pct, err := luaInt64Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if pct == 0 {
			return nil, nil
		}
		if motion := s.luaCreatureMotion(state.GUID, state.Entry); motion != nil && motion.ThreatMgr != nil {
			motion.ThreatMgr.ScaleThreat(victimGUID, float32(0.01*float64(100+pct)))
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
	// the creature's map/instance (NewMapObject + mapLive, the
	// same builder behind the instance hooks' map argument).
	methods["GetMap"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{scripting.NewMapObject(state.Map, state.InstanceID, s.server.mapLive(state.Map, state.InstanceID))}, nil
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
	// IsSummon is Unit::IsSummon (Unit.h:837 — UNIT_MASK_SUMMON):
	// the handler-surface twin lives in lua_creature_events.go; this
	// session binding reads the same creaturemotion IsTempSummon
	// flag, stamped only by the summon bridge.
	methods["IsSummon"] = luaNoArgs(func() any {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		return motion != nil && motion.IsTempSummon
	})
	// IsTargetableForAttack is CreatureMethods::IsTargetableForAttack
	// (CreatureMethods.h:55-64): Unit::isTargetableForAttack(checkFakeDeath
	// = mustBeDead) (Unit.cpp:8504-8516) — alive, no NON_ATTACKABLE
	// (0x2) / NOT_SELECTABLE flag, no UNIT_STATE_UNATTACKABLE, and when
	// mustBeDead no UNIT_STATE_DIED (Go: the feign-death flag twin).
	// UNIT_STATE_UNATTACKABLE has no Go model, so that arm is vacuous here.
	// NOT_SELECTABLE (0x02000000) is modeled live (vehicle.go:818 sets it on
	// players; creaturemotion.go:4031 consults creature unitFlags), but this
	// binding reads NON_ATTACKABLE only.
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
	// broadcast is the SetUInt32Value twin. A slot passed as 0xFFFFFFFF
	// is left untouched, mirroring ScriptedAI::SetEquipmentSlots'
	// EQUIP_NO_CHANGE = -1 skip semantics (ScriptedCreature.h:226,
	// ScriptedCreature.cpp:473-477: slots < 0 are never written), since
	// Lua uint32 args cannot carry -1.
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
		updates := map[int]uint32{}
		for i, slot := range slots {
			if slot == 0xFFFFFFFF {
				continue
			}
			updates[unitVirtualItemSlotID+i] = slot
		}
		if len(updates) > 0 {
			s.server.broadcastCreatureValuesUpdateInInstance(state.Map, state.InstanceID, state.GUID, updates)
		}
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
	// GetLootMode is CreatureMethods::GetLootMode (CreatureMethods.h:881-885):
	// Creature::GetLootMode (Creature.h:224) returns the m_LootMode bitmask.
	methods["GetLootMode"] = luaNoArgs(func() any {
		motion := luaTapMotion()
		if motion == nil {
			return uint32(lootModeDefault)
		}
		return uint32(motion.LootMode)
	})
	// HasLootMode is CreatureMethods::HasLootMode (CreatureMethods.h:395-401):
	// Creature::HasLootMode (Creature.h:225) tests (m_LootMode & lootMode) != 0.
	methods["HasLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		motion := luaTapMotion()
		return []any{motion != nil && motion.LootMode&uint16(mode) != 0}, nil
	}
	// mutateLootMode applies fn to the live creature motion's LootMode bitmask
	// (Creature::m_LootMode, Creature.h:482) under the motion lock.
	mutateLootMode := func(fn func(*creatureMotion)) {
		s.server.motionMu.Lock()
		if motion := s.server.findCreatureMotionLocked(state.Map, state.InstanceID, state.GUID); motion != nil {
			fn(motion)
		}
		s.server.motionMu.Unlock()
	}
	// SetLootMode is CreatureMethods::SetLootMode (CreatureMethods.h:935-942):
	// Creature::SetLootMode (Creature.h:226) assigns m_LootMode.
	methods["SetLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mutateLootMode(func(motion *creatureMotion) { motion.LootMode = uint16(mode) })
		return nil, nil
	}
	// AddLootMode is CreatureMethods::AddLootMode (CreatureMethods.h:1318-1327):
	// Creature::AddLootMode (Creature.h:227) ORs the mode in.
	methods["AddLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mutateLootMode(func(motion *creatureMotion) { motion.LootMode |= uint16(mode) })
		return nil, nil
	}
	// RemoveLootMode is CreatureMethods::RemoveLootMode (CreatureMethods.h:1305-1316):
	// Creature::RemoveLootMode (Creature.h:228) clears the mode bits.
	methods["RemoveLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mutateLootMode(func(motion *creatureMotion) { motion.LootMode &^= uint16(mode) })
		return nil, nil
	}
	// ResetLootMode is CreatureMethods::ResetLootMode (CreatureMethods.h:1297-1301):
	// Creature::ResetLootMode (Creature.h:229) restores LOOT_MODE_DEFAULT.
	methods["ResetLootMode"] = func(_ context.Context, _ []any) ([]any, error) {
		mutateLootMode(func(motion *creatureMotion) { motion.LootMode = lootModeDefault })
		return nil, nil
	}
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

	// GetAttackDistance is Creature::GetAttackDistance (Creature.cpp:2015-2058),
	// the session-creature twin of the handler binding; a motion that no
	// longer resolves reads 0.
	methods["GetAttackDistance"] = func(ctx context.Context, args []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil {
			return []any{float32(0)}, nil
		}
		targetLevel, targetDetected, ok := s.server.creatureDistanceTarget(args, motion.Map, motion.InstanceID)
		if !ok {
			return nil, fmt.Errorf("GetAttackDistance requires a Unit target")
		}
		return []any{s.server.creatureAttackDistance(ctx, motion, targetLevel, targetDetected)}, nil
	}
	// GetAggroRange is Creature::GetAggroRange (Creature.cpp:3130-3174),
	// the session-creature twin of the handler binding; non-pets read 0.
	methods["GetAggroRange"] = func(ctx context.Context, args []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil {
			return []any{float32(0)}, nil
		}
		targetLevel, targetDetected, ok := s.server.creatureDistanceTarget(args, motion.Map, motion.InstanceID)
		if !ok {
			return nil, fmt.Errorf("GetAggroRange requires a Unit target")
		}
		if motion.OwnerGUID == 0 {
			return []any{float32(0)}, nil
		}
		ownDetect := s.server.creatureAuraTypeSum(creatureAuraKeyForMotion(motion), spellAuraModDetectRange)
		return []any{creatureAggroRange(motion, targetLevel, ownDetect, targetDetected)}, nil
	}
	// SelectVictim is Creature::SelectVictim (Creature.cpp:1194-1258),
	// the session-creature twin of the handler binding.
	methods["SelectVictim"] = func(ctx context.Context, _ []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil || motion.ThreatMgr == nil || !motionCanHaveThreatList(motion) {
			return []any{nil}, nil
		}
		victimGUID := motion.ThreatMgr.GetCurrentVictim()
		if victimGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(victimGUID); player != nil {
			if sess := s.server.findSessionByGUID(victimGUID); sess != nil && sess.player != nil && sess.player.Map == motion.Map && sess.player.InstanceID == motion.InstanceID && sess.player.Health > 0 {
				return []any{player}, nil
			}
			return []any{nil}, nil
		}
		if creature := s.luaCreature(ctx, victimGUID); creature != nil {
			if target := s.server.findCreatureMotion(motion.Map, motion.InstanceID, victimGUID); target != nil && target.Health > 0 {
				return []any{creature}, nil
			}
		}
		return []any{nil}, nil
	}
	// GetAITarget is CreatureMethods::GetAITarget (CreatureMethods.h:706-799,
	// Trinity), the session-creature twin of the handler binding.
	methods["GetAITarget"] = func(ctx context.Context, args []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil {
			return []any{nil}, nil
		}
		targetType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		playerOnly := false
		if len(args) >= 2 && args[1] != nil {
			b, ok := args[1].(bool)
			if !ok {
				return nil, fmt.Errorf("GetAITarget requires a bool playerOnly")
			}
			playerOnly = b
		}
		var position uint32
		if len(args) >= 3 && args[2] != nil {
			position, err = luaUint32Arg(args, 2)
			if err != nil {
				return nil, err
			}
		}
		var dist float32
		if len(args) >= 4 && args[3] != nil {
			dist, err = luaGenericFloat32Arg(args, 3)
			if err != nil {
				return nil, err
			}
		}
		var aura int32
		if len(args) >= 5 && args[4] != nil {
			aura, err = luaInt32Arg(args, 4)
			if err != nil {
				return nil, err
			}
		}
		guid, err := s.server.selectAIThreatTarget(motion, targetType, playerOnly, position, dist, aura)
		if err != nil {
			return nil, err
		}
		if guid == 0 {
			return []any{nil}, nil
		}
		if player := s.server.findPlayer(guid); player != nil {
			return []any{player}, nil
		}
		if creature := s.luaCreature(ctx, guid); creature != nil {
			return []any{creature}, nil
		}
		return []any{nil}, nil
	}
	// GetCurrentWaypointId is CreatureMethods::GetCurrentWaypointId
	// (CreatureMethods.h:452-458, Trinity), the session-creature twin of
	// the handler binding; a motion that no longer resolves reads 0.
	methods["GetCurrentWaypointId"] = luaNoArgs(func() any {
		motion := luaTapMotion()
		if motion == nil {
			return uint32(0)
		}
		return motion.LastWaypointID
	})
	// MoveWaypoint is CreatureMethods::MoveWaypoint (CreatureMethods.h:1196-1203),
	// the session-creature twin of the handler binding.
	methods["MoveWaypoint"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion := luaTapMotion(); motion != nil {
			s.server.startCreatureWaypoint(ctx, motion)
		}
		return nil, nil
	}
	// SetWalk is Creature::SetWalk (Creature.cpp:3012-3021), the
	// session-creature twin of the handler binding.
	methods["SetWalk"] = func(_ context.Context, args []any) ([]any, error) {
		enable := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetWalk requires a bool")
			}
			enable = b
		}
		motion := luaTapMotion()
		if motion == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		if motion.Walking == enable {
			s.server.motionMu.Unlock()
			return nil, nil
		}
		motion.Walking = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		phase := motion.PhaseMask
		s.server.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
		}
		s.server.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
		return nil, nil
	}
	// SetFFA is UnitMethods::SetFFA (UnitMethods.h:1851-1890, TRINITY
	// leg), the session-creature twin of the handler binding: the
	// FFA_PVP byte flag on the motion PVPFlags slot plus the
	// controlled-unit fan-out.
	methods["SetFFA"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetFFA requires a bool")
			}
			apply = b
		}
		motion := luaTapMotion()
		if motion == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		if apply {
			motion.PVPFlags |= pvpFlagFFA
		} else {
			motion.PVPFlags &^= pvpFlagFFA
		}
		motion.PVPFlagsKnown = true
		s.server.motionMu.Unlock()
		s.server.fanOutBytes2FlagToControlled(state.Map, state.InstanceID, state.GUID, pvpFlagFFA, apply)
		return nil, nil
	}
	// SetPvP is UnitMethods::SetPvP (UnitMethods.h:1466-1474), the
	// session-creature twin of the handler binding: the PvP byte flag
	// only, no timer or contested-flag arm.
	methods["SetPvP"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetPvP requires a bool")
			}
			apply = b
		}
		motion := luaTapMotion()
		if motion == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		if apply {
			motion.PVPFlags |= pvpFlagPvP
		} else {
			motion.PVPFlags &^= pvpFlagPvP
		}
		motion.PVPFlagsKnown = true
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// GetStat is UnitMethods::GetStat (UnitMethods.h:726-734), the
	// session-creature twin of the handler binding: the live motion
	// stats array, nil on a despawned motion or stat >= MAX_STATS (5).
	methods["GetStat"] = func(_ context.Context, args []any) ([]any, error) {
		stat, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if stat >= 5 {
			return []any{nil}, nil
		}
		motion := luaTapMotion()
		if motion == nil {
			return []any{nil}, nil
		}
		return []any{float64(motion.Stats[stat])}, nil
	}
	// GetCreatorGUID is UnitMethods::GetCreatorGUID (UnitMethods.h:633-638),
	// the session-creature twin of the handler binding: the live motion
	// UNIT_FIELD_CREATEDBY, 0 on a despawned motion.
	methods["GetCreatorGUID"] = func(_ context.Context, _ []any) ([]any, error) {
		motion := luaTapMotion()
		if motion == nil {
			return []any{uint64(0)}, nil
		}
		return []any{motion.CreatedByGUID}, nil
	}
	// SetCreatorGUID is UnitMethods::SetCreatorGUID (UnitMethods.h:1790-1797),
	// the session-creature twin of the handler binding; a despawned
	// motion is a silent no-op.
	methods["SetCreatorGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		motion := luaTapMotion()
		if motion == nil {
			return nil, nil
		}
		s.server.motionMu.Lock()
		motion.CreatedByGUID = guid
		s.server.motionMu.Unlock()
		return nil, nil
	}
	// Eluna nearest/in-range family (WorldObjectMethods.h) over the
	// live motion, re-resolved at call time like the session SendUnit
	// arms: a motion that no longer resolves reads a srv-nil origin,
	// so nearest reads nil, lists read empty, and IsWithinLoS reads
	// false (the nil-player-origin convention in luaRangeOriginForPlayer).
	for name, fn := range luaRangeMethods(s.server, func() luaRangeOrigin {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return luaRangeOrigin{}
		}
		return s.server.luaRangeOriginForMotion(motion)
	}) {
		methods[name] = fn
	}
	// GetFriendlyUnitsInRange/GetUnfriendlyUnitsInRange are the Unit
	// in-range pair (UnitMethods.h:1255/1296); see
	// luaUnitFriendlyMethods in lua_worldobject_range.go.
	for name, fn := range luaUnitFriendlyMethods(s.server, func() luaRangeOrigin {
		motion := s.luaCreatureMotion(state.GUID, state.Entry)
		if motion == nil {
			return luaRangeOrigin{}
		}
		return s.server.luaRangeOriginForMotion(motion)
	}) {
		methods[name] = fn
	}
	// Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242).
	luaWorldObjectSoundMethods(s.server, nil, state.GUID, methods)
	// Indexed values (ObjectMethods.h) delegate to the motion backend
	// when the motion resolves, else read the snapshot; see
	// sessionCreatureIndexedValueBackend in lua_values.go.
	registerIndexedValueMethods(methods, sessionCreatureIndexedValueBackend(s, state))
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
	var size float64
	if err := srv.WorldStore.DB.QueryRowContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.map, g.position_x, g.position_y, g.position_z, g.orientation, t.type, t.size, COALESCE(g.phaseMask, 0) FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.guid = ? AND g.id = ?", low, entry).Scan(&state.LowGUID, &state.Entry, &state.DisplayID, &state.Name, &mapID, &state.X, &state.Y, &state.Z, &state.Orientation, &state.Type, &size, &state.PhaseMask); err != nil {
		return state, false
	}
	state.Size = float32(size)
	state.Map, state.GUID = uint32(mapID), gameObjectGUID(state.LowGUID, state.Entry)
	state.GoState, state.LootState = 1, 1
	return state, true
}

func (srv *Server) luaGameObjectObject(state luaGameObjectState) *scripting.Object {
	var goObj *scripting.Object
	methods := map[string]scripting.ObjectMethod{}
	methods["GetName"] = luaNoArgs(func() any { return state.Name })
	methods["GetEntry"] = luaNoArgs(func() any { return state.Entry })
	methods["GetDisplayId"] = luaNoArgs(func() any { return state.DisplayID })
	methods["GetDBTableGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	// GetGoState is LuaGameObject::GetGoState (GameObjectMethods.h):
	// GameObject::GetGoState. The snapshot state is only the spawn-time
	// value (loadLuaGameObject stamps GO_STATE_READY); a live dynamic
	// state wins, so a SetGoState/UseDoorOrButton on the same object
	// reads back the new state instead of the stale spawn snapshot.
	// A static DB spawn gains its live state on its first SetGoState
	// (materializeStaticGameObjectState); until then it keeps the
	// snapshot.
	methods["GetGoState"] = func(_ context.Context, _ []any) ([]any, error) {
		if dyn := srv.gameObjectState(state.Map, state.InstanceID, state.GUID); dyn != nil {
			return []any{uint32(dyn.State)}, nil
		}
		return []any{state.GoState}, nil
	}
	// GetLootState is LuaGameObject::GetLootState (GameObjectMethods.h):
	// GameObject::getLootState. A live record with a script-set loot
	// state wins (SetLootState below); otherwise the spawn snapshot
	// stands (GO_READY, the in-world value — C++ Create stamps
	// GO_NOT_READY but the Update arm flips it to READY on spawn,
	// GameObject.cpp:473/532-545).
	methods["GetLootState"] = func(_ context.Context, _ []any) ([]any, error) {
		if dyn := srv.gameObjectState(state.Map, state.InstanceID, state.GUID); dyn != nil && dyn.LootStateSet {
			return []any{uint32(dyn.LootState)}, nil
		}
		return []any{state.LootState}, nil
	}
	// GetGoType is GameObject::GetGoType (GameObject.h:160): the
	// template type (GAMEOBJECT_BYTES_1 byte 1), 0-50 per the
	// GameObjectType constants (gameobjects.go). DB-loaded GOs read
	// gameobject_template.type; runtime-spawned GOs carry dyn.Type.
	// Backs the goober gates in go_scripts.cpp:61-474 (gilded_brazier,
	// resonite_cask, blood_filled_orb, tablet_of_the_seven).
	methods["GetGoType"] = luaNoArgs(func() any { return state.Type })
	// GetScale is Object::GetScale (ObjectMethods.h:134; Object.h:87):
	// GameObject::Create stamps OBJECT_FIELD_SCALE_X from the
	// template size (GameObject.cpp:331), which loadLuaGameObject
	// already carries as state.Size. SetScale stays out: the GO
	// object is a DB snapshot with no live scale store to write
	// (the GetPhaseMask precedent in this builder).
	methods["GetScale"] = luaNoArgs(func() any { return float64(state.Size) })
	// GetOwner is GameObject::GetOwner (Object.h:429, Unit::GetOwner
	// via UNIT_FIELD_SUMMONEDBY): the owning player object, or nil
	// when the GO has no owner or the owner session is gone. DB
	// spawns carry no owner in Go (OwnerGUID 0); only runtime-spawned
	// GOs populate it (dynamicGameObjects / instanceGameObjects).
	// Backs the soulwell gossip gate (go_scripts.cpp:538-564).
	methods["GetOwner"] = func(_ context.Context, _ []any) ([]any, error) {
		if state.OwnerGUID == 0 {
			return []any{nil}, nil
		}
		if owner := srv.findSessionByGUID(state.OwnerGUID); owner != nil {
			return []any{owner.luaPlayer()}, nil
		}
		return []any{nil}, nil
	}
	// AddUse/GetUseCount are GameObject::AddUse/GetUseCount
	// (GameObject.h:202-204): the soulwell charge counter. Static DB
	// spawns have no live state object, so AddUse is a no-op for them.
	methods["AddUse"] = luaNoArgs(func() any { srv.addGameObjectUse(state.GUID); return nil })
	methods["GetUseCount"] = luaNoArgs(func() any { return srv.gameObjectUseCount(state.GUID) })
	methods["IsSpawned"] = luaNoArgs(func() any { return !srv.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) })
	// IsActive is LuaGameObject::IsActive (GameObjectMethods.h):
	// WorldObject::isActiveObject (Object.h:501) reads m_isActive,
	// NOT the GO state — the old GoState==0 read mistook
	// GO_STATE_ACTIVE (an open door) for active-object status. C++
	// sets m_isActive true for GOs only in Battlefield::AddGameObject
	// (Battlefield.cpp:827); Go has no battlefield GO activation, so
	// the banked flag (default false) is the whole model.
	methods["IsActive"] = luaNoArgs(func() any {
		if dyn := srv.gameObjectState(state.Map, state.InstanceID, state.GUID); dyn != nil {
			return dyn.Active
		}
		return false
	})
	// GetLootRecipient/GetLootRecipientGroup are LuaGameObject's pair
	// (GameObjectMethods.h): GameObject::GetLootRecipient reads
	// m_lootRecipient via FindConnectedPlayer, the group twin reads
	// m_lootRecipientGroup. Both fields are written only by
	// GameObject::SetLootRecipient (GameObject.cpp:2489), which has
	// zero call sites in this fork (every SetLootRecipient caller
	// targets a creature), so both read nil exactly — no Go store is
	// missing, and the active-loot Viewers map is not a recipient
	// (group-loot viewers are not the C++ recipient).
	methods["GetLootRecipient"] = luaNoArgs(func() any { return nil })
	methods["GetLootRecipientGroup"] = luaNoArgs(func() any { return nil })
	methods["GetGUID"] = luaNoArgs(func() any { return state.GUID })
	methods["GetGUIDLow"] = luaNoArgs(func() any { return state.LowGUID })
	methods["GetObjectType"] = luaNoArgs(func() any { return "GameObject" })
	methods["GetMapId"] = luaNoArgs(func() any { return state.Map })
	methods["GetX"] = luaNoArgs(func() any { return state.X })
	methods["GetY"] = luaNoArgs(func() any { return state.Y })
	methods["GetZ"] = luaNoArgs(func() any { return state.Z })
	// GetZoneId/GetAreaId are WorldObject::GetZoneId()/GetAreaId() via
	// Map::GetZoneAndAreaId (Map.cpp:2641), the same Server.zoneAndAreaID
	// twin the creature surface uses (lua_creature_events.go): the area
	// resolves from the GO's map + position and the zone is the area's
	// ParentAreaID when set. Backs the brewfest area dispatch
	// (go_scripts.cpp:931-1051) and the bells zone->soundId init
	// (:1282-1325). Both return 0 when the position cannot be resolved.
	methods["GetZoneId"] = func(_ context.Context, _ []any) ([]any, error) {
		zoneID, _ := srv.zoneAndAreaID(state.Map, state.X, state.Y, state.Z, 0)
		return []any{zoneID}, nil
	}
	methods["GetAreaId"] = func(_ context.Context, _ []any) ([]any, error) {
		_, areaID := srv.zoneAndAreaID(state.Map, state.X, state.Y, state.Z, 0)
		return []any{areaID}, nil
	}
	// GetPhaseMask is WorldObject::GetPhaseMask (WorldObjectMethods.h):
	// the gameobject state's stored phase mask (DB phaseMask for static
	// spawns, spawn phase for runtime spawns). There is no GO SetPhaseMask
	// twin: Go carries no live phase-change path for gameobjects (static
	// phase is spawn-time DB data, runtime phase is stamped at spawn), so
	// a set would have no live object to write; it stays out.
	methods["GetPhaseMask"] = luaNoArgs(func() any { return state.PhaseMask })
	// GetMap is WorldObject::GetMap: the DBC-backed Map object for the
	// gameobject's map/instance (NewMapObject + mapLive, the same
	// builder behind the creature surface and the instance hooks' map
	// argument). Nil when the server store is unavailable.
	methods["GetMap"] = func(_ context.Context, _ []any) ([]any, error) {
		if srv.Data == nil {
			return []any{nil}, nil
		}
		return []any{scripting.NewMapObject(state.Map, state.InstanceID, srv.mapLive(state.Map, state.InstanceID))}, nil
	}
	// IsHolidayActive is GameEventMgr::IsHolidayActive
	// (GameEventMgr.cpp:1847): true when any currently active game event
	// carries this holiday id, false for HOLIDAY_NONE. The Go twin reads
	// the cached active-holiday set (conditions.go, refreshed at most once
	// a minute). Backs the *_music holiday gates (go_scripts.cpp:931-1230).
	methods["IsHolidayActive"] = func(ctx context.Context, args []any) ([]any, error) {
		holiday, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if holiday == 0 {
			return []any{false}, nil
		}
		_, active := srv.cachedActiveGameHolidays(ctx)[int64(holiday)]
		return []any{active}, nil
	}
	// GetCreaturesInGrid(entry, dist) is
	// WorldObject::GetCreatureListWithEntryInGrid, the gameobject-surface
	// twin of the creature bridge (lua_creature_events.go): every creature
	// of entry within dist yards (2D, candidate combat reach added —
	// GridNotifiers.h; gameobjects carry no combat reach, so the source
	// reach is 0) in the same map+instance, as a Lua array of creature
	// objects (GUID-sorted for determinism). Backs the veil-skith cage
	// child search (go_scripts.cpp:691-709).
	methods["GetCreaturesInGrid"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		dist := float64(dist32)
		srv.motionMu.Lock()
		var matches []*creatureMotion
		for _, m := range srv.motionMapLocked(state.Map, state.InstanceID) {
			if m.Entry != entry {
				continue
			}
			dx := float64(m.X - state.X)
			dy := float64(m.Y - state.Y)
			if d := math.Sqrt(dx*dx + dy*dy); d <= dist+float64(m.combatReachOrDefault()) {
				matches = append(matches, m)
			}
		}
		srv.motionMu.Unlock()
		sort.Slice(matches, func(i, j int) bool { return matches[i].GUID < matches[j].GUID })
		out := make([]any, 0, len(matches))
		for _, m := range matches {
			out = append(out, srv.luaMotionCreature(m))
		}
		return []any{out}, nil
	}
	// GetPlayerListInGrid(dist) is
	// WorldObject::GetPlayerListInGrid (Object.cpp:3236): every alive
	// player within dist yards (3D, player combat reach 1.5 added —
	// GridNotifiers.h AnyPlayerInObjectRangeCheck + Object.cpp
	// _IsWithinDist; gameobjects carry no combat reach, so the source
	// reach is 0) in the same map+instance, as a Lua array of player
	// objects (GUID-sorted for determinism). Alive = not dead-or-ghost
	// (death.go isDeadOrGhost twin; ghosts carry Health=1 in Go, so a
	// Health>0 check alone would wrongly include them). Backs the
	// brewfest SHATTRATH + midsummer per-player dispatch
	// (go_scripts.cpp:999, 1099).
	methods["GetPlayerListInGrid"] = func(_ context.Context, args []any) ([]any, error) {
		dist32, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist := float64(dist32)
		srv.sessionsMu.RLock()
		var matches []*session
		for sess := range srv.sessions {
			if !sess.worldReady.Load() || sess.player == nil {
				continue
			}
			if sess.player.Map != state.Map || sess.player.InstanceID != state.InstanceID {
				continue
			}
			if sess.isDeadOrGhost() {
				continue
			}
			dx := float64(sess.player.X - state.X)
			dy := float64(sess.player.Y - state.Y)
			dz := float64(sess.player.Z - state.Z)
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist+1.5 {
				matches = append(matches, sess)
			}
		}
		srv.sessionsMu.RUnlock()
		sort.Slice(matches, func(i, j int) bool { return matches[i].playerGUID < matches[j].playerGUID })
		out := make([]any, 0, len(matches))
		for _, sess := range matches {
			if player := sess.luaPlayer(); player != nil {
				out = append(out, player)
			}
		}
		return []any{out}, nil
	}
	// FindNearestCreature(entry, dist) is the gameobject-surface twin of
	// the player/creature bridges (WorldObject::FindNearestCreature):
	// the nearest live creature of entry within dist yards (3D gate with
	// the candidate's combat reach added; gameobjects carry no combat
	// reach in the Go model, so the source reach is 0; winner by
	// reach-adjusted distance) on the GO's map+instance; nil when none.
	// Backs the ribbon-pole bunny search (go_scripts.cpp:817-826).
	methods["FindNearestCreature"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if dist32 < 0 {
			return []any{nil}, nil
		}
		dist := float64(dist32)
		srv.motionMu.Lock()
		var best *creatureMotion
		bestAdj := dist
		for _, m := range srv.motionMapLocked(state.Map, state.InstanceID) {
			if m.Entry != entry || m.Health == 0 {
				continue
			}
			dx := float64(m.X - state.X)
			dy := float64(m.Y - state.Y)
			dz := float64(m.Z - state.Z)
			candReach := float64(m.combatReachOrDefault())
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist+candReach {
				adj := d - candReach
				if adj < 0 {
					adj = 0
				}
				if best == nil || adj < bestAdj {
					best, bestAdj = m, adj
				}
			}
		}
		srv.motionMu.Unlock()
		if best == nil {
			return []any{nil}, nil
		}
		return []any{srv.luaMotionCreature(best)}, nil
	}
	// FindNearestGameObject(entry, dist) is the gameobject-surface twin of
	// the creature bridge (WorldObject::FindNearestGameObject, Object.cpp):
	// the nearest gameobject of entry within dist yards (3D metric) in the
	// same map+instance; nil when none. DB spawns plus runtime GOs (see
	// nearestGameObjectByEntry). Backs the shadowforge-brazier golem-door
	// arm (blackrock_depths.cpp:51-69).
	methods["FindNearestGameObject"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if dist32 < 0 {
			return []any{nil}, nil
		}
		return []any{srv.nearestGameObjectByEntry(ctx, state.Map, state.InstanceID, state.X, state.Y, state.Z, entry, dist32)}, nil
	}
	// Eluna nearest/in-range family (WorldObjectMethods.h, see
	// lua_worldobject_range.go): registered from the gameobject
	// surface with the GO state as origin.
	for name, fn := range luaRangeMethods(srv, func() luaRangeOrigin { return srv.luaRangeOriginForGameObject(state) }) {
		methods[name] = fn
	}
	// SummonCreature is the gameobject-surface twin of the player bridge
	// (WorldObject::SummonCreature, Object.cpp:2059-2062 fallbacks via
	// the shared summonTempCreature funnel): same (entry, x, y, z, o,
	// summonType, despawnTimer-ms) signature, same 0-7 type scheme,
	// unknown entries return nil. The origin is the GO itself (gameobjects
	// carry no combat reach, so the 0,0,0,0 close point is the GO position
	// exactly, not the player close point the player twin computes).
	// Backs the resonite-cask summon leg (go_scripts.cpp:251-260) and
	// the ethereum stasis/prison GO-position summons (:218-230, :155-198).
	methods["SummonCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		x, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		y, err := luaFloat32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		z, err := luaFloat32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		o, err := luaFloat32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		summonType := uint32(7)
		if len(args) > 5 {
			if summonType, err = luaUint32Arg(args, 5); err != nil {
				return nil, err
			}
			if summonType > 7 {
				return nil, fmt.Errorf("SummonCreature summonType must be 0-7")
			}
		}
		var despawnMs uint32
		if len(args) > 6 {
			if despawnMs, err = luaUint32Arg(args, 6); err != nil {
				return nil, err
			}
		}
		origin := tempSummonOrigin{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID, X: state.X, Y: state.Y, Z: state.Z, O: state.Orientation, Reach: 0}
		summoned := srv.summonTempCreature(ctx, origin, entry, x, y, z, o, uint8(summonType), despawnMs)
		if summoned == nil {
			return []any{nil}, nil
		}
		return []any{srv.luaMotionCreature(summoned)}, nil
	}
	// SpawnCreature is WorldObject::SpawnCreature (WorldObjectMethods.h),
	// the Eluna 1-8 spawn-type twin of SummonCreature; the shared
	// spawnCreatureFromOrigin funnel (lua_creature_events.go) documents
	// the type translation and the seconds-to-milliseconds despawn
	// conversion. Gameobjects carry no combat reach, so the close-point
	// fallback uses reach 0, matching the GO SummonCreature twin.
	methods["SpawnCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		origin := tempSummonOrigin{Map: state.Map, InstanceID: state.InstanceID, GUID: state.GUID, X: state.X, Y: state.Y, Z: state.Z, O: state.Orientation, Reach: 0}
		summoned, err := srv.spawnCreatureFromOrigin(ctx, origin, args)
		if err != nil {
			return nil, err
		}
		if summoned == nil {
			return []any{nil}, nil
		}
		return []any{srv.luaMotionCreature(summoned)}, nil
	}
	methods["RemoveFromWorld"] = func(ctx context.Context, args []any) ([]any, error) {
		// C++ GameObject::Delete removes the object outright; for a
		// runtime GO that means despawning it (destroy broadcast +
		// GO_EVENT_ON_REMOVE), while a static DB spawn has no live
		// state and is removed by hiding it in the instance.
		// LuaGameObject::RemoveFromWorld (GameObjectMethods.h) adds two
		// arms the old bridge ignored: an owner gate (a missing or
		// non-player owner returns without deleting, GameObject.cpp
		// RemoveFromWorld twin) and deleteFromDB (GameObject::DeleteFromDB
		// on the spawn id; synthetic runtime lows match no DB row, so
		// the DELETE is harmless for them). No-arg callers — all 7 live
		// Lua sites — take the unchanged path.
		if state.OwnerGUID != 0 && srv.findSessionByGUID(state.OwnerGUID) == nil {
			return nil, nil
		}
		deldb := false
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				deldb = b
			}
		}
		if deldb && srv.WorldStore != nil && srv.WorldStore.DB != nil {
			_, _ = srv.WorldStore.DB.ExecContext(ctx, "DELETE FROM gameobject WHERE guid = ?", state.LowGUID)
		}
		if srv.gameObjectState(state.Map, state.InstanceID, state.GUID) != nil {
			srv.despawnDynamicGameObjectInInstance(state.Map, state.InstanceID, state.GUID)
		} else {
			srv.setGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID, true)
		}
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
		srv.setGameObjectStateInInstance(state.Map, state.InstanceID, state.GUID, uint8(value))
		return nil, nil
	}
	// SetLootState is LuaGameObject::SetLootState (GameObjectMethods.h):
	// GameObject::SetLootState(state) — the single-arg Eluna arm (the
	// (state, unit) overload is internal; m_lootStateUnitGUID has no
	// Eluna reader). The write lands on the live record, materializing
	// a static spawn exactly like SetGoState, so a later object for the
	// same GO reads it back; the closure copy keeps the pre-materialize
	// read-back for objects without a live record. DELTA: the Eluna
	// OnLootStateChanged fire inside C++ SetLootState has no Go binding
	// (no event-11 hook exists in scripting/), so handlers never run.
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
		srv.setGameObjectLootState(state.Map, state.InstanceID, state.GUID, uint8(value))
		return nil, nil
	}
	// SetRespawnTime is LuaGameObject::SetRespawnTime
	// (GameObjectMethods.h): GameObject::SetRespawnTime(int32)
	// (GameObject.cpp:1330-1336) — respawn > 0 stores now+respawn as
	// m_respawnTime and respawn as m_respawnDelayTime, respawn <= 0
	// zeroes both. Go stores only the delay leg: the static-GO map
	// respawn tick that consumes m_respawnTime has no Go equivalent
	// (hidden GOs come back only via script/BG unhides and the
	// loot-release timer below), so the portal re-spawn arm
	// (obsidian_sanctum.cpp:309-312) stays parked. CONSUMERS of the
	// stored delay: the loot-release respawn timer (loot.go) and
	// SaveToDB's spawntimesecs, both of which read m_respawnDelayTime
	// in C++. The !spawnedByDefault visibility arm is a no-op for a
	// visible runtime GO, so it is not mirrored.
	methods["SetRespawnTime"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("respawn is required")
		}
		raw, err := luaInt64Arg(args, 0)
		if err != nil || raw > math.MaxInt32 || raw < math.MinInt32 {
			if err == nil {
				err = fmt.Errorf("respawn must be int32")
			}
			return nil, err
		}
		delay := int32(0)
		if raw > 0 {
			delay = int32(raw)
		}
		srv.setGameObjectRespawnDelay(state.Map, state.InstanceID, state.GUID, delay)
		return nil, nil
	}
	// SaveToDB is LuaGameObject::SaveToDB (GameObjectMethods.h):
	// GameObject::SaveToDB (GameObject.cpp:983-1070) — DELETE + INSERT
	// of the gameobject row for the spawn id (generated when 0), with
	// position, state, anim progress, phase, spawn mask and
	// spawntimesecs = spawnedByDefault ? respawnDelay : -respawnDelay.
	// The Go twin: a row for this guid+entry is replaced in place
	// (same guid, current dyn-or-snapshot position/state/phase,
	// rotations from the live record else the row, spawntimesecs from
	// the SetRespawnTime delay when set else the row); a runtime GO
	// with no row takes the .gobject-add auto-increment path and the
	// live record re-keys to the new spawn id (plus the closure and
	// Fields GUIDs, so the same object keeps resolving). No world DB
	// means no-op, the Item SaveToDB precedent.
	methods["SaveToDB"] = func(ctx context.Context, _ []any) ([]any, error) {
		if srv.WorldStore == nil || srv.WorldStore.DB == nil {
			return nil, nil
		}
		db := srv.WorldStore.DB
		dyn := srv.gameObjectState(state.Map, state.InstanceID, state.GUID)
		x, y, z, o := state.X, state.Y, state.Z, state.Orientation
		goState := state.GoState
		phase := state.PhaseMask
		rot := [4]float32{0, 0, 0, 1}
		anim := 255
		spawnMask := 1
		spawnSecs := int64(0)
		rowExists := false
		if dyn != nil {
			x, y, z, o = dyn.X, dyn.Y, dyn.Z, dyn.Orientation
			goState = uint32(dyn.State)
			phase = dyn.PhaseMask
			rot = dyn.ParentRotation
			anim = int(dyn.AnimProgress)
		}
		var rowMask, rowSecs, rowAnim int64
		var rowRot0, rowRot1, rowRot2, rowRot3 float64
		if err := db.QueryRowContext(ctx, `SELECT spawnMask, spawntimesecs, animprogress, rotation0, rotation1, rotation2, rotation3 FROM gameobject WHERE guid = ? AND id = ? LIMIT 1`, state.LowGUID, state.Entry).Scan(&rowMask, &rowSecs, &rowAnim, &rowRot0, &rowRot1, &rowRot2, &rowRot3); err == nil {
			rowExists = true
			spawnMask, spawnSecs = int(rowMask), rowSecs
			if dyn == nil {
				anim = int(rowAnim)
				rot = [4]float32{float32(rowRot0), float32(rowRot1), float32(rowRot2), float32(rowRot3)}
			}
		}
		if dyn != nil && dyn.RespawnDelaySet {
			if dyn.IsRuntimeSpawn {
				spawnSecs = -int64(dyn.RespawnDelaySec)
			} else {
				spawnSecs = int64(dyn.RespawnDelaySec)
			}
		}
		if rowExists {
			if _, err := db.ExecContext(ctx, `DELETE FROM gameobject WHERE guid = ?`, state.LowGUID); err != nil {
				return nil, err
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO gameobject (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, rotation0, rotation1, rotation2, rotation3, spawntimesecs, animprogress, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				state.LowGUID, state.Entry, state.Map, spawnMask, phase, x, y, z, o, rot[0], rot[1], rot[2], rot[3], spawnSecs, anim, goState); err != nil {
				return nil, err
			}
			return nil, nil
		}
		res, err := db.ExecContext(ctx, `INSERT INTO gameobject (id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, rotation0, rotation1, rotation2, rotation3, spawntimesecs, animprogress, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			state.Entry, state.Map, spawnMask, phase, x, y, z, o, rot[0], rot[1], rot[2], rot[3], spawnSecs, anim, goState)
		if err != nil {
			return nil, err
		}
		if newLow, err := res.LastInsertId(); err == nil && newLow > 0 {
			newGUID := gameObjectGUID(uint32(newLow), state.Entry)
			if dyn != nil {
				srv.rekeyDynamicGameObject(state.Map, state.InstanceID, state.GUID, newGUID)
			}
			state.GUID = newGUID
			state.LowGUID = uint32(newLow)
			if goObj != nil {
				goObj.Fields["GUID"] = newGUID
			}
		}
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
	// SetFlag/HasFlag mirror GameObject::SetFlag/HasFlag on GAMEOBJECT_FLAGS
	// (the Go values-map flags field, gameObjectFlags): C++ ORs on set and
	// AND-tests on read; other fields are ignored (creature HasFlag
	// precedent: only flag-backed fields read live). A static DB spawn
	// gains its live state on its first flag write (instance_gameobjects.go
	// materialization), and HasFlag reads the spawn flags for a GO that was
	// never touched. Backs script flag arms such as the Zul'Gurub gong
	// (NOT_SELECTABLE) and the Karazhan urn (IN_USE read leg).
	methods["SetFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if field == gameObjectFlags {
			srv.setGameObjectFlag(state.Map, state.InstanceID, state.GUID, flag)
		}
		return nil, nil
	}
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if field != gameObjectFlags {
			return []any{false}, nil
		}
		return []any{srv.gameObjectHasFlag(state.Map, state.InstanceID, state.GUID, flag)}, nil
	}
	methods["RemoveFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if field == gameObjectFlags {
			srv.clearGameObjectFlag(state.Map, state.InstanceID, state.GUID, flag)
		}
		return nil, nil
	}
	// SendCustomAnim mirrors GameObject::SendCustomAnim (GameObject.cpp):
	// the SMSG_GAMEOBJECT_CUSTOM_ANIM broadcast for this GO's instance.
	methods["SendCustomAnim"] = func(_ context.Context, args []any) ([]any, error) {
		var anim uint32
		if len(args) > 0 {
			value, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			anim = value
		}
		srv.broadcastGameObjectCustomAnimInInstance(state.Map, state.InstanceID, state.GUID, anim)
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
	// HasQuest is LuaGameObject::HasQuest (GameObjectMethods.h):
	// GameObject::hasQuest (GameObject.cpp:1197-1200) = the
	// gameobject-quest starter relation (ObjectMgr GetGOQuestRelations),
	// the GO twin of the creature HasQuest bridge (starter table only,
	// not starter-or-ender). No world DB reads false.
	methods["HasQuest"] = func(ctx context.Context, args []any) ([]any, error) {
		questID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if srv.WorldStore == nil || srv.WorldStore.DB == nil {
			return []any{false}, nil
		}
		return []any{questRelationExists(ctx, srv.WorldStore.DB, "gameobject_queststarter", state.Entry, questID)}, nil
	}
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
	// Indexed values (ObjectMethods.h); see goIndexedValueBackend in
	// lua_values.go for the dyn/static read model and write rules.
	registerIndexedValueMethods(methods, goIndexedValueBackend(srv, state))
	goObj = &scripting.Object{Type: "GameObject", Fields: map[string]any{"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry, "Map": state.Map, "MapId": state.Map, "X": state.X, "Y": state.Y, "Z": state.Z, "Orientation": state.Orientation, "DisplayID": state.DisplayID, "GoState": state.GoState, "LootState": state.LootState, "InWorld": true}, Methods: methods}
	return goObj
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

// nearestGameObjectByEntry is the entry-filtered, 3D twin of
// nearestGameObject for the creature surface's FindNearestGameObject:
// WorldObject::FindNearestGameObject (Object.cpp) searches the grid for the
// nearest gameobject of entry within range using the 3D distance metric.
// The search covers the static spawn table plus runtime GOs
// (SummonGameObject/spell/BG summons), which C++'s grid search also finds.
// Instance-hidden objects are skipped, matching the unfiltered twin.
func (srv *Server) nearestGameObjectByEntry(ctx context.Context, mapID, instanceID uint32, x, y, z float32, entry uint32, distance float32) *scripting.Object {
	rows, err := srv.WorldStore.DB.QueryContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.position_x, g.position_y, g.position_z, g.orientation, COALESCE(g.phaseMask, 0) FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.map = ? AND g.id = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ? ORDER BY g.guid", mapID, entry, float64(x-distance), float64(x+distance), float64(y-distance), float64(y+distance))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var nearest luaGameObjectState
	nearestDistance := float32(math.MaxFloat32)
	for rows.Next() {
		var candidate luaGameObjectState
		if err := rows.Scan(&candidate.LowGUID, &candidate.Entry, &candidate.DisplayID, &candidate.Name, &candidate.X, &candidate.Y, &candidate.Z, &candidate.Orientation, &candidate.PhaseMask); err != nil {
			continue
		}
		candidate.Map = mapID
		candidate.InstanceID = instanceID
		candidate.GoState, candidate.LootState = 1, 1
		candidate.GUID = gameObjectGUID(candidate.LowGUID, candidate.Entry)
		if srv.isGameObjectHiddenInInstance(candidate.Map, candidate.InstanceID, candidate.GUID) {
			continue
		}
		dx, dy, dz := float64(candidate.X-x), float64(candidate.Y-y), float64(candidate.Z-z)
		currentDistance := float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
		if currentDistance <= distance && currentDistance < nearestDistance {
			nearest, nearestDistance = candidate, currentDistance
		}
	}
	// Runtime GOs live in the dynamic/instance stores, not the spawn table,
	// so they compete with the DB best on the same 3D metric. Snapshot
	// under objectsMu, then filter hidden after release
	// (isGameObjectHiddenInInstance takes the same lock).
	type dynCandidate struct {
		state    luaGameObjectState
		distance float32
	}
	var dynCandidates []dynCandidate
	srv.objectsMu.RLock()
	collectDyn := func(dyn *dynamicGameObjectState) {
		if dyn == nil || dyn.Entry != entry || dyn.Map != mapID || dyn.InstanceID != instanceID {
			return
		}
		dx, dy, dz := float64(dyn.X-x), float64(dyn.Y-y), float64(dyn.Z-z)
		currentDistance := float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
		if currentDistance > distance {
			return
		}
		dynCandidates = append(dynCandidates, dynCandidate{state: luaGameObjectState{GUID: dyn.GUID, LowGUID: dyn.LowGUID, Entry: dyn.Entry, DisplayID: dyn.DisplayID, Map: dyn.Map, InstanceID: dyn.InstanceID, X: dyn.X, Y: dyn.Y, Z: dyn.Z, Orientation: dyn.Orientation, Type: uint32(dyn.Type), GoState: uint32(dyn.State), LootState: 1, PhaseMask: dyn.PhaseMask}, distance: currentDistance})
	}
	for _, dyn := range srv.dynamicGameObjects {
		collectDyn(dyn)
	}
	for _, dyn := range srv.instanceGameObjects[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}] {
		collectDyn(dyn)
	}
	srv.objectsMu.RUnlock()
	sort.Slice(dynCandidates, func(i, j int) bool { return dynCandidates[i].distance < dynCandidates[j].distance })
	for _, cand := range dynCandidates {
		if srv.isGameObjectHiddenInInstance(cand.state.Map, cand.state.InstanceID, cand.state.GUID) {
			continue
		}
		if cand.distance < nearestDistance {
			return srv.luaGameObjectObject(cand.state)
		}
		break
	}
	if nearest.GUID == 0 {
		return nil
	}
	return srv.luaGameObjectObject(nearest)
}

// gameObjectsInGridByEntry is the list twin of nearestGameObjectByEntry
// for the creature surface's GetGameObjectsInGrid:
// WorldObject::GetGameObjectListWithEntryInGrid (Object.cpp:3220, default
// range 250.0f): every gameobject of entry within range using the 2D
// metric (AllGameObjectsWithEntryInRange, GridNotifiers.h:1437-1454:
// IsWithinDist(go, range, false) — no combat-reach additions, unlike
// the creature list). DB-spawn model only, instance-hidden skipped,
// GUID-sorted for determinism (the C++ grid-visit order is unspecified).
func (srv *Server) gameObjectsInGridByEntry(ctx context.Context, mapID, instanceID uint32, x, y float32, entry uint32, distance float32) []*scripting.Object {
	rows, err := srv.WorldStore.DB.QueryContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.position_x, g.position_y, g.position_z, g.orientation, COALESCE(g.phaseMask, 0) FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.map = ? AND g.id = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ? ORDER BY g.guid", mapID, entry, float64(x-distance), float64(x+distance), float64(y-distance), float64(y+distance))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var matches []luaGameObjectState
	for rows.Next() {
		var candidate luaGameObjectState
		if err := rows.Scan(&candidate.LowGUID, &candidate.Entry, &candidate.DisplayID, &candidate.Name, &candidate.X, &candidate.Y, &candidate.Z, &candidate.Orientation, &candidate.PhaseMask); err != nil {
			continue
		}
		candidate.Map = mapID
		candidate.InstanceID = instanceID
		candidate.GoState, candidate.LootState = 1, 1
		candidate.GUID = gameObjectGUID(candidate.LowGUID, candidate.Entry)
		if srv.isGameObjectHiddenInInstance(candidate.Map, candidate.InstanceID, candidate.GUID) {
			continue
		}
		if currentDistance := float32(math.Hypot(float64(candidate.X-x), float64(candidate.Y-y))); currentDistance <= distance {
			matches = append(matches, candidate)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].GUID < matches[j].GUID })
	out := make([]*scripting.Object, 0, len(matches))
	for _, m := range matches {
		out = append(out, srv.luaGameObjectObject(m))
	}
	return out
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
