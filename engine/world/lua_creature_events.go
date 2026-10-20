package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// luaMotionCreature builds a scripting creature object from a live motion
// snapshot. Unlike the session-bound luaCreature (which resolves spawn rows
// from the world DB), this is server-level so the combat-lifecycle triggers
// (aggro, evade, death) can hand Lua a usable creature without a session.
// The generic object methods (GetName, GetEntry, GetGUID, GetHealth,
// GetMaxHealth, GetLevel, GetX/Y/Z, IsAlive, IsDead, ...) read these fields,
// so Eluna-style handlers get the standard read surface for free.
func (s *Server) luaMotionCreature(motion *creatureMotion) *scripting.Object {
	if s == nil || motion == nil {
		return nil
	}
	state := luaCreatureState{
		GUID:      motion.GUID,
		Entry:     motion.Entry,
		Name:      motion.Name,
		Health:    motion.Health,
		MaxHealth: motion.MaxHealth,
		Level:     motion.Level,
		Map:       motion.Map,
		X:         motion.X,
		Y:         motion.Y,
		Z:         motion.Z,
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetVictim"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		targetGUID := motion.TargetGUID
		s.motionMu.Unlock()
		if targetGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(targetGUID); player != nil {
			return []any{player}, nil
		}
		if target := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); target != nil {
			return []any{s.luaMotionCreature(target)}, nil
		}
		return []any{nil}, nil
	}
	methods["IsInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		inCombat := motion.InCombat
		s.motionMu.Unlock()
		return []any{inCombat}, nil
	}
	// CastSpell(target, spellId[, triggered]) casts a creature spell through
	// the server's creature cast path (SMSG_SPELL_GO broadcast, the same path
	// the Go BossAI implementations use). Eluna argument order. The target
	// may be a creature/player object, a raw GUID, or nil — nil falls back to
	// the creature's current victim and is a no-op when it has none.
	methods["CastSpell"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CastSpell requires a target and a spell id")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			s.motionMu.Lock()
			targetGUID = motion.TargetGUID
			s.motionMu.Unlock()
		}
		if targetGUID == 0 {
			return nil, nil
		}
		// Eluna's third arg selects the triggered cast
		// (TRIGGERED_IGNORE_POWER_AND_REAGENT_COST, Spell.cpp:3449) —
		// a triggered cast skips the CheckPower/TakePower legs.
		triggered := false
		if len(args) >= 3 {
			if b, ok := args[2].(bool); ok {
				triggered = b
			}
		}
		if triggered {
			s.castCreatureSpellTriggered(ctx, motion, spellID, targetGUID)
		} else {
			s.castCreatureSpell(ctx, motion, spellID, targetGUID)
		}
		return nil, nil
	}
	// Talk(textId[, target]) mirrors the C++ CreatureAI::Talk(textId,
	// whisperTarget) used by boss scripts: it plays the creature_text row for
	// (entry, textId) through the same broadcastCreatureTalk path the Go
	// BossAI implementations use. The optional target (a player object or
	// GUID) bridges CreatureAI::Talk's whisperTarget arm — whisper-type rows
	// (CHAT_MSG_MONSTER_WHISPER / RAID_BOSS_WHISPER) route to that player
	// only, exactly like SendChatPacket.
	methods["Talk"] = func(ctx context.Context, args []any) ([]any, error) {
		textID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		var whisperTarget uint64
		if len(args) >= 2 {
			whisperTarget = luaObjectGUID(args[1])
		}
		s.broadcastCreatureTalk(ctx, motion.Map, motion.GUID, motion.Entry, motion.Name, uint8(textID), whisperTarget)
		return nil, nil
	}
	// Eluna WorldObject sound methods (WorldObjectMethods.h:1161-1242).
	luaWorldObjectSoundMethods(s, nil, motion.GUID, methods)
	// Eluna event handlers call creature/victim methods that the
	// session-bound luaCreature also exposes; without them every
	// RegisterCreatureEvent handler calling e.g. creature:IsInEvadeMode()
	// or victim:GetObjectType() nil-calls and aborts mid-arm. The Go
	// backing state is the same (motion fields, the server aura maps),
	// so these are exact twins of the lua_objects.go bindings, not new
	// semantics.
	// GetObjectType mirrors the session-bound binding: a motion creature
	// is always a Creature (players arrive as luaPlayer objects).
	methods["GetObjectType"] = luaNoArgs(func() any { return "Creature" })
	// IsInEvadeMode is CreatureMethods::IsInEvadeMode: motion.Evading —
	// the creature is returning to its spawn position from combat
	// (session-bound twin at lua_objects.go).
	methods["IsInEvadeMode"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		evading := motion.Evading
		s.motionMu.Unlock()
		return []any{evading}, nil
	}
	auraKey := func() creatureAuraKey {
		return creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}
	}
	// HasAura / RemoveAura / AddAura mirror the session-bound creature
	// bindings: the Go aura model is keyed by creatureAuraKey, which the
	// motion carries, so no session is needed.
	methods["HasAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := auraKey()
		s.auraMu.Lock()
		_, found := s.creatureAuras[key][spell]
		if !found {
			_, found = s.activeCreatureAuras[key][spell]
		}
		s.auraMu.Unlock()
		return []any{found}, nil
	}
	methods["RemoveAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.removeCreatureAura(auraKey(), spell)
		return nil, nil
	}
	// RemoveAllAuras is Unit::RemoveAllAuras for handler creatures: the
	// full strip via clearCreatureAuras (the evade-leg twin — timers
	// stopped, both aura maps cleared). Freywinn's tranquility arm.
	methods["RemoveAllAuras"] = func(_ context.Context, _ []any) ([]any, error) {
		s.clearCreatureAuras(auraKey())
		return nil, nil
	}
	// AttackStop is Unit::AttackStop (Unit.cpp:5763) for handler creatures:
	// the current victim is dropped (TargetGUID=0), SMSG_ATTACK_STOP goes
	// to the instance, and the no-call-assistance flag resets (the
	// combat-stop-only reset). Threat is untouched — C++ keeps the threat
	// list; re-engagement rides the normal assist/aggro paths. The melee
	// interrupt arm is vacuous (creature swings are tick-driven, no melee
	// spell slot). The thekal zealot feign-death arm.
	methods["AttackStop"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		victim := motion.TargetGUID
		if victim == 0 {
			s.motionMu.Unlock()
			return []any{false}, nil
		}
		motion.TargetGUID = 0
		motion.AlreadyCallAssistance = false
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		stopPkt := buildAttackStop(guid, victim, false)
		s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
		return []any{true}, nil
	}
	methods["AddAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := auraKey()
		s.auraMu.Lock()
		if s.creatureAuras == nil {
			s.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
		}
		if s.creatureAuras[key] == nil {
			s.creatureAuras[key] = make(map[uint32]struct{})
		}
		s.creatureAuras[key][spell] = struct{}{}
		s.auraMu.Unlock()
		return nil, nil
	}
	// SetHealth mirrors the session-bound binding: clamp, write the
	// motion, broadcast the values update.
	methods["SetHealth"] = func(_ context.Context, args []any) ([]any, error) {
		health, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		if health > motion.MaxHealth {
			health = motion.MaxHealth
		}
		motion.Health = health
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldHealth: health})
		return nil, nil
	}
	// Despawn is Lua Creature::DespawnOrUnsummon() (Creature.cpp): the
	// creature leaves the world immediately — motion removed, a
	// SMSG_DESTROY_OBJECT broadcast (the despawnAtEvade precedent,
	// boss_ai.go), and Eluna CREATURE_EVENT_ON_REMOVE (37) fired
	// (Creature::RemoveFromWorld, Creature.cpp:314). DB-spawned
	// creatures respawn on their normal spawntimesecs delay through the
	// standard death-respawn path (the curhealth=0 row keeps the spawn
	// sweep from recreating them early); summoned creatures have no
	// creature row and leave no respawn record, so they are gone
	// permanently — matching C++ DespawnOrUnsummon on a summon.
	methods["Despawn"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		motions := s.motionMapLocked(motion.Map, motion.InstanceID)
		m, ok := motions[motion.GUID]
		if !ok {
			s.motionMu.Unlock()
			return nil, nil
		}
		delete(motions, motion.GUID)
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		maxHealth := m.MaxHealth
		x, y, z := m.X, m.Y, m.Z
		s.motionMu.Unlock()
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(guid)
		destroyBuf.WriteU8(0)
		s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), nil)
		s.fireCreatureLuaEvent(ctx, m, scripting.CreatureEventOnRemove)
		if maxHealth == 0 {
			maxHealth = 1
		}
		dbGUID := uint32(guid & 0x00FFFFFF)
		dbSpawned := false
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			var one int
			dbSpawned = s.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature WHERE guid = ?", dbGUID).Scan(&one) == nil
		}
		if !dbSpawned {
			return nil, nil
		}
		if instanceID == 0 {
			_, _ = s.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = 0 WHERE guid = ?", dbGUID)
			s.scheduleCreatureRespawn(ctx, dbGUID, maxHealth, time.Now())
		} else {
			s.scheduleInstanceCreatureRespawn(ctx, combatTarget{GUID: guid, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, MaxHealth: maxHealth}, maxHealth, time.Now())
		}
		return nil, nil
	}
	// IsHeroic is ScriptedAI::_isHeroic (ScriptedCreature.cpp:129): the
	// map spawn mode fixed when the instance was created. The Go
	// instance table stores exactly that difficulty per instance id
	// (worldport_instance.go writes it at creation), so this reads the
	// row instead of approximating from a session. Non-instance maps
	// (InstanceID 0) and missing rows default to false, like C++ normal
	// mode. Formula is Map::IsHeroic (Map.cpp:4249-4252): raids heroic
	// at difficulty >= 2 (10-man heroic), dungeons at >= 1.
	methods["IsHeroic"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion.InstanceID == 0 {
			return []any{false}, nil
		}
		var difficulty int64
		if s.CharactersStore != nil && s.CharactersStore.DB != nil {
			_ = s.CharactersStore.DB.QueryRowContext(ctx,
				"SELECT difficulty FROM instance WHERE id = ?", motion.InstanceID).Scan(&difficulty)
		}
		isRaid := false
		if s.Data != nil {
			if entry, found, err := s.Data.Map(motion.Map); err == nil && found {
				isRaid = entry.IsRaid()
			}
		}
		if isRaid {
			return []any{difficulty >= 2}, nil
		}
		return []any{difficulty >= 1}, nil
	}
	// SetEquipmentSlots is CreatureMethods::SetEquipmentSlots
	// (CreatureMethods.h:982-994, Trinity): SetUInt32Value
	// (UNIT_VIRTUAL_ITEM_SLOT_ID + i, slot) for the three virtual item
	// slots (UpdateFields.h:116, Go index 56) — the values-update
	// broadcast is the SetUInt32Value twin. A slot passed as 0xFFFFFFFF
	// is left untouched, mirroring ScriptedAI::SetEquipmentSlots'
	// EQUIP_NO_CHANGE = -1 skip semantics (ScriptedCreature.h:226,
	// ScriptedCreature.cpp:473-477: slots < 0 are never written), since
	// Lua uint32 args cannot carry -1. Exact twin of the session-bound
	// binding (lua_objects.go:747-777): commits bf5e8ec5/4030673b ported
	// handler arms (kirtonos onTransform, mr_smite onSpawn/onReset)
	// against the session-only binding, which nil-calls from this surface
	// (objectIndex pushes nil for unknown methods; ProtectedCall then
	// aborts the arm mid-flight), so the handler twin exists.
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
			s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, updates)
		}
		return nil, nil
	}
	// PerformEmote is UnitMethods::PerformEmote (UnitMethods.h:2041-2049):
	// Unit::HandleEmoteCommand sends SMSG_EMOTE (u32 emote + u64 guid) to
	// the visibility set; creatures broadcast with a nil source like the
	// creature-text arms. Exact twin of the session-bound binding
	// (lua_objects.go:375): the mordresh OOC_3 and fel-orc Reset-emote
	// arms ported in 4030673b nil-call from this surface without it (see
	// SetEquipmentSlots above), killing their timer chains mid-arm.
	methods["PerformEmote"] = func(_ context.Context, args []any) ([]any, error) {
		emote, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		pkt := protocol.NewBuffer(12)
		pkt.WriteU32(emote)
		pkt.WriteU64(motion.GUID)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), nil)
		return nil, nil
	}
	// GetAITargets is CreatureMethods::GetAITargets
	// (CreatureMethods.h:807-839, Trinity): the threatened-by-me victims
	// as Unit objects (players first-resolved, then creatures). Exact twin
	// of the session-bound binding (lua_objects.go:994-1021): creature
	// targets resolve through the server creature-motion index here
	// (the GetVictim arm above). RegisterCreatureEvent handlers nil-call
	// on this name without it (objectIndex pushes nil for unknown methods;
	// ProtectedCall then aborts the arm mid-flight), the same class fixed
	// for SetEquipmentSlots/PerformEmote above.
	methods["GetAITargets"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.ThreatMgr == nil {
			return []any{[]any{}}, nil
		}
		targets := []any{}
		for _, entry := range motion.ThreatMgr.SortedEntries() {
			if player := s.findPlayer(entry.VictimGUID); player != nil {
				targets = append(targets, player)
				continue
			}
			if target := s.findCreatureMotion(motion.Map, motion.InstanceID, entry.VictimGUID); target != nil {
				targets = append(targets, s.luaMotionCreature(target))
			}
		}
		return []any{targets}, nil
	}
	// SetReactState is CreatureMethods::SetReactState: Creature::SetReactState
	// (UnitDefines.h: REACT_PASSIVE=0, REACT_DEFENSIVE=1, REACT_AGGRESSIVE=2 —
	// matches creatureReact* in creaturemotion.go). The engine honors it in
	// isCreaturePassive (creaturemotion.go:586) and help-call filtering, so
	// phase/intro react arms (midnight, baltharus clone, ...) port with
	// real effect instead of a Lua-side latch.
	methods["SetReactState"] = func(_ context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.ReactState = uint8(st)
		motion.ReactStateKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// EmoteState is Creature::SetUInt32Value(UNIT_NPC_EMOTESTATE, state):
	// the persistent emote-state update field the client renders
	// (SharedDefines.h: EMOTE_STATE_SUBMERGED=373, EMOTE_STATE_STUN=64,
	// EMOTE_ONESHOT_NONE=0 clears). applyCreatureMotionToSpawn overlays
	// motion.EmoteState onto the spawn record (creatures.go), so phase and
	// intro emotestate arms (ragnaros submerge, akama stun, ...) port with
	// real effect; one-shot emotes stay on PerformEmote.
	methods["EmoteState"] = func(_ context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.EmoteState = st
		motion.EmoteStateKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetStandState is Creature::SetStandState: SetByteValue(UNIT_FIELD_BYTES_1,
	// UNIT_BYTES_1_OFFSET_STAND_STATE, state) (Unit.cpp: UNIT_STAND_STATE_STAND=0,
	// SIT=1, SIT_CHAIR=2, SLEEP=3, SIT_LOW_CHAIR=4, SIT_MEDIUM_CHAIR=5,
	// SIT_HIGH_CHAIR=6, DEAD=7, KNEEL=8, SUBMERGED=9, SharedDefines.h).
	// applyCreatureMotionToSpawn overlays motion.StandState onto the spawn
	// record's Bytes1 byte 0 (creatures.go), so feign-death / intro /
	// throne-sitting arms (stalagg, feugen, nefarius, ...) port with real
	// client-visible effect; the other Bytes1 bytes are preserved.
	methods["SetStandState"] = func(_ context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if st > 9 {
			return nil, fmt.Errorf("SetStandState requires a UNIT_STAND_STATE 0-9, got %d", st)
		}
		s.motionMu.Lock()
		motion.StandState = uint8(st)
		motion.StandStateKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetSheath is Unit::SetSheath: SetByteValue(UNIT_FIELD_BYTES_2,
	// UNIT_BYTES_2_OFFSET_SHEATH_STATE, sheathed) (Unit.h:903; Creature does
	// not override, only Player does — the virtual-item legs are
	// player-side, so the creature twin is the byte write alone). Sheath
	// states: SHEATH_STATE_UNARMED=0, MELEE=1, RANGED=2 (UnitDefines.h:71-77).
	// applyCreatureMotionToSpawn overlays motion.SheathState onto the spawn
	// record's Bytes2 byte 0 (creatures.go), so melee/ranged-visual arms
	// (weegli shoot, algalon evade unarm, ...) port with real client-visible
	// effect; the other Bytes2 bytes are preserved.
	methods["SetSheath"] = func(_ context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if st > 2 {
			return nil, fmt.Errorf("SetSheath requires a SHEATH_STATE 0-2, got %d", st)
		}
		s.motionMu.Lock()
		motion.SheathState = uint8(st)
		motion.SheathStateKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// Mount is Unit::Mount (Unit.cpp:8310): SetUInt32Value(
	// UNIT_FIELD_MOUNTDISPLAYID, mount) when the id is nonzero, then
	// SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_MOUNT 0x08000000,
	// UnitDefines.h:151). applyCreatureMotionToSpawn overlays motion.Mount
	// onto the spawn record (creatures.go), so intro / phase mount arms
	// (mandokir Reset MODEL_OHGAN_MOUNT 15271, ...) port with real
	// client-visible effect.
	methods["Mount"] = func(_ context.Context, args []any) ([]any, error) {
		displayID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		if displayID != 0 {
			motion.Mount = displayID
		}
		motion.UnitFlags |= 0x08000000 // UNIT_FLAG_MOUNT
		motion.MountKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// Dismount is Unit::Dismount (Unit.cpp:8365): early-return when not
	// mounted (the UNIT_FLAG_MOUNT check is IsMounted), then clear
	// UNIT_FIELD_MOUNTDISPLAYID and the flag. MountKnown forces the 0
	// clear into the update mask, mirroring the EmoteState bridge.
	methods["Dismount"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		if motion.UnitFlags&0x08000000 == 0 { // not IsMounted
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.Mount = 0
		motion.UnitFlags &^= 0x08000000
		motion.MountKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// IsMounted is Unit::IsMounted (Unit.cpp): HasFlag(UNIT_FIELD_FLAGS,
	// UNIT_FLAG_MOUNT 0x08000000). motion.UnitFlags is the live flag
	// mirror (init from template at spawn; SetFlag/RemoveFlag/Mount/
	// Dismount write it), so the query matches the Dismount bridge's
	// early-return gate.
	methods["IsMounted"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		mounted := motion.UnitFlags&0x08000000 != 0
		s.motionMu.Unlock()
		return []any{mounted}, nil
	}
	// unitFieldFlagsValue is UNIT_FIELD_FLAGS = OBJECT_END(0x6)+0x35
	// (UpdateFields.h:117). SetFlag/RemoveFlag on it mutate motion.UnitFlags,
	// which the engine reads in help-call (creaturemotion.go:1089) and
	// chain/area target filtering — intro NOT_SELECTABLE/NON_ATTACKABLE arms
	// port with real targeting effect. Other update fields have no creature
	// backing here and are ignored, matching the no-op stance of the
	// unbridged NPC/dynamic-flag arms.
	methods["SetFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flags, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags |= flags
			s.motionMu.Unlock()
		}
		return nil, nil
	}
	methods["RemoveFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flags, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags &^= flags
			s.motionMu.Unlock()
		}
		return nil, nil
	}
	// FindNearestCreature(entry, dist) is WorldObject::FindNearestCreature /
	// GetClosestCreatureWithEntry (ObjectAccessor): the nearest live creature
	// of entry within dist yards (3D, the C++ IsWithinDistInMap metric) in the
	// same map+instance, excluding self; nil when none. Backs the
	// creature-search arms (flame-tsunami lava-blaze buff, anubarak
	// sphere/burrower cleanup) that headers documented as bridge-blocked.
	methods["FindNearestCreature"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		dist := float64(dist32)
		s.motionMu.Lock()
		var best *creatureMotion
		bestD := dist
		for guid, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if guid == motion.GUID || m.Entry != entry || m.Health == 0 {
				continue
			}
			dx := float64(m.X - motion.X)
			dy := float64(m.Y - motion.Y)
			dz := float64(m.Z - motion.Z)
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist && (best == nil || d < bestD) {
				best, bestD = m, d
			}
		}
		s.motionMu.Unlock()
		if best == nil {
			return []any{nil}, nil
		}
		return []any{s.luaMotionCreature(best)}, nil
	}
	// GetCreaturesInGrid(entry, dist) is
	// WorldObject::GetCreatureListWithEntryInGrid: every creature of entry
	// within dist yards (3D) in the same map+instance, excluding self, as a
	// Lua array of creature objects (GUID-sorted for determinism; the C++
	// grid-visit order is unspecified). Backs mass-despawn arms such as
	// anubarak's frost-sphere/burrower cleanup.
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
		s.motionMu.Lock()
		var matches []*creatureMotion
		for guid, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if guid == motion.GUID || m.Entry != entry {
				continue
			}
			dx := float64(m.X - motion.X)
			dy := float64(m.Y - motion.Y)
			dz := float64(m.Z - motion.Z)
			if math.Sqrt(dx*dx+dy*dy+dz*dz) <= dist {
				matches = append(matches, m)
			}
		}
		s.motionMu.Unlock()
		sort.Slice(matches, func(i, j int) bool { return matches[i].GUID < matches[j].GUID })
		out := make([]any, 0, len(matches))
		for _, m := range matches {
			out = append(out, s.luaMotionCreature(m))
		}
		return []any{out}, nil
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
		s.motionMu.Lock()
		motion.HomeX, motion.HomeY, motion.HomeZ = x, y, z
		motion.Orientation = o
		s.motionMu.Unlock()
		return nil, nil
	}
	return &scripting.Object{Type: "Creature", Fields: map[string]any{
		"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry,
		"Map": state.Map, "MapId": state.Map,
		"X": state.X, "Y": state.Y, "Z": state.Z, "Orientation": motion.Orientation,
		"Health": state.Health, "MaxHealth": state.MaxHealth, "Level": state.Level,
		"InstanceId": motion.InstanceID,
		"InWorld":    true,
	}, Methods: methods}
}

// luaObjectGUID resolves a Lua-passed target to a GUID: a creature/player
// object contributes its GUID field, a bare number is taken as the GUID,
// anything else (including nil) yields 0.
func luaObjectGUID(value any) uint64 {
	if obj, ok := value.(*scripting.Object); ok && obj != nil {
		if guid, ok := obj.Fields["GUID"].(uint64); ok {
			return guid
		}
		return 0
	}
	if guid, err := luaUint64(value); err == nil {
		return guid
	}
	return 0
}

// fireCreatureLuaEvent dispatches an Eluna RegisterCreatureEvent hook for the
// creature's entry. Eluna argument order: (event, creature, ...) with the
// per-event extra arguments appended after the creature. Never fires when
// the scripting runtime is disabled or the creature is gone; Lua errors are
// logged by the runtime, never propagated. Returns true when any handler
// returns boolean true, matching Eluna's CallAllFunctionsBool
// (lua_isboolean-checked) OR-semantics, so START_HOOK_WITH_RETVAL call
// sites can veto the default action; no bindings or a disabled runtime
// behaves like C++'s RETVAL=false.
func (s *Server) fireCreatureLuaEvent(ctx context.Context, motion *creatureMotion, event int, extra ...any) bool {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return false
	}
	args := make([]any, 0, len(extra)+2)
	args = append(args, event, creature)
	args = append(args, extra...)
	results, _ := s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, event, args...)
	return luaHookVeto(results)
}

// luaHookVeto ORs handler return values the way Eluna's CallAllFunctionsBool
// does: only an actual boolean true flips the result (HookHelpers.h checks
// lua_isboolean before lua_toboolean), so a handler returning 1, "yes" or an
// object does not veto.
func luaHookVeto(results []any) bool {
	for _, r := range results {
		if b, ok := r.(bool); ok && b {
			return true
		}
	}
	return false
}

// fireCreatureDamageTaken dispatches Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9)
// for the creature before damage is applied. C++ (Eluna::DamageTaken,
// CreatureHooks.cpp:114-141, fired from Unit::DealDamage, Unit.cpp:697-702)
// passes (event, creature, attacker, damage); each handler may return a
// boolean veto and, as the second return, a replacement damage. The boolean
// gates only the empty ScriptedAI::DamageTaken base (UnitAI.h, the sole
// implementation), a provable no-op like the SpellHit ruling, so Go consumes
// only the damage rewrite. A numeric second return rewrites damage AND the
// argument the next handler sees (the damageIndex/ReplaceArgument loop); the
// final damage is the last numeric rewrite. Non-numeric, negative, or >
// MaxUint32 returns are ignored, mirroring lua_isnumber and
// CHECKVAL<uint32>'s range errors (LuaEngine.cpp:770-788); fractional values
// truncate toward zero like C++'s static_cast<unsigned int>. Both binding
// families fire: entry handlers first, then the unique handlers for this
// creature's GUID/instance, matching SetupStack's merged call list
// (HookHelpers.h:37-39) with the rewritten damage threaded through both
// passes. Returns the damage to apply.
func (s *Server) fireCreatureDamageTaken(ctx context.Context, motion *creatureMotion, attacker *scripting.Object, damage uint32) uint32 {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return damage
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return damage
	}
	args := []any{scripting.CreatureEventOnDamageTaken, creature, attacker, damage}
	// The rewritten damage is threaded through the update closure, so the
	// per-handler return pairs need no post-processing.
	update := func(returns []any) {
		if len(returns) < 2 {
			return
		}
		if n, ok := returns[1].(float64); ok && n >= 0 && n <= 4294967295 {
			damage = uint32(n)
			args[3] = damage
		}
	}
	_, _ = s.Features.Scripts.TriggerCreatureEvent2Updated(ctx, motion.Entry, scripting.CreatureEventOnDamageTaken, args, update)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent2Updated(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDamageTaken, args, update)
	return damage
}

// fireCreatureCorpseRemoved dispatches Eluna CREATURE_EVENT_ON_CORPSE_REMOVED
// (26) with the pending respawn delay. C++ (Eluna::CorpseRemoved,
// CreatureHooks.cpp:236-265, fired from Creature::RemoveCorpse via
// ai->CorpseRemoved, Creature.cpp:393/429) passes (event, creature,
// respawnDelay); each handler may return a boolean veto and, as the second
// return, a replacement delay. The boolean gates only ScriptedAI::
// CorpseRemoved, which has no override — the sole implementation is the empty
// CreatureAI base (CreatureAI.h:190), a provable no-op like the SpellHit
// ruling, so Go consumes only the delay rewrite. A numeric second return
// rewrites the delay AND the argument the next handler sees (the
// respawnDelayIndex/ReplaceArgument loop); the final delay is the last
// numeric rewrite. Non-numeric, negative, or > MaxUint32 returns are ignored,
// mirroring lua_isnumber and CHECKVAL<uint32>'s range errors
// (LuaEngine.cpp:770-788); fractional values truncate toward zero like C++'s
// static_cast<unsigned int>. Both binding families fire: entry handlers
// first, then the unique handlers for this creature's GUID/instance, matching
// SetupStack's merged call list (HookHelpers.h:37-39) with the rewritten delay
// threaded through both passes. Returns the delay to use.
func (s *Server) fireCreatureCorpseRemoved(ctx context.Context, motion *creatureMotion, delay uint32) uint32 {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return delay
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return delay
	}
	args := []any{scripting.CreatureEventOnCorpseRemoved, creature, delay}
	// The rewritten delay is threaded through the update closure, so the
	// per-handler return pairs need no post-processing.
	update := func(returns []any) {
		if len(returns) < 2 {
			return
		}
		if n, ok := returns[1].(float64); ok && n >= 0 && n <= 4294967295 {
			delay = uint32(n)
			args[2] = delay
		}
	}
	_, _ = s.Features.Scripts.TriggerCreatureEvent2Updated(ctx, motion.Entry, scripting.CreatureEventOnCorpseRemoved, args, update)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent2Updated(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnCorpseRemoved, args, update)
	return delay
}

// fireCreatureMoveInLOS dispatches Eluna CREATURE_EVENT_ON_MOVE_IN_LOS (27)
// when the creature is about to aggro a player it has sighted. C++
// (Eluna::MoveInLineOfSight, CreatureHooks.cpp:268-274, reached from
// ElunaCreatureAI::MoveInLineOfSight, ElunaCreatureAI.h:212-216, which the
// grid relocation worker drives via CreatureUnitRelocationWorker,
// GridNotifiers.cpp:129-141, whenever a detectable unit relocates into the
// creature's sight range) passes (event, creature, unit) through
// CallAllFunctionsBool: a boolean true from any handler vetoes the "normal
// action", which for ElunaCreatureAI is ScriptedAI::MoveInLineOfSight — the
// default aggro engage. Go's fire site is the per-tick aggro scan's engage
// branch (stepCreatureMotion): the hook fires where the default aggro would
// happen, and a veto skips exactly that engage, mirroring C++'s
// `if (!sEluna->MoveInLineOfSight(me, who)) ScriptedAI::MoveInLineOfSight
// (who)`. Both binding families fire — entry handlers first, then the
// unique handlers for this creature's GUID/instance — matching SetupStack's
// merged call list (HookHelpers.h:37-39); the veto ORs across both passes
// like CallAllFunctionsBool. No handlers or a disabled runtime behaves like
// C++'s RETVAL=false default. Returns true when the aggro must be skipped.
func (s *Server) fireCreatureMoveInLOS(ctx context.Context, motion *creatureMotion, sess *session) bool {
	if s == nil || motion == nil || sess == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	creature := s.luaMotionCreature(motion)
	who := sess.luaPlayer()
	if creature == nil || who == nil {
		return false
	}
	args := []any{scripting.CreatureEventOnMoveInLOS, creature, who}
	results, _ := s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnMoveInLOS, args...)
	uniqueResults, _ := s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnMoveInLOS, args...)
	return luaHookVeto(results) || luaHookVeto(uniqueResults)
}

// fireCreatureDied dispatches Eluna CREATURE_EVENT_ON_DIED (4) for a
// creature killed through the session kill funnel. C++
// (Eluna::JustDied, CreatureHooks.cpp:143-152, reached from
// ElunaCreatureAI::JustDied, ElunaCreatureAI.h:108-113) calls On_Reset
// first — CREATURE_EVENT_ON_RESET (23) is a void hook (START_HOOK at
// CreatureHooks.cpp:277-282), so its returns are discarded — then fires
// ON_DIED with (event, creature, killer). Both binding families fire,
// entry then unique, matching SetupStack's merged call list
// (HookHelpers.h:37-39). The boolean veto gates only
// ScriptedAI::JustDied: the empty CreatureAI base for plain creatures and
// BossAI::_JustDied (summon despawn + instance DONE) for bosses — Go has
// no summon-tracking model to gate and instance encounter clearing already
// runs unconditionally in the kill paths, so the veto is a provable no-op
// like the SpellHit ruling and is discarded. The killer is the session's
// luaPlayer: every Go kill path (melee, spell, pet) funnels through
// session.onCreatureKilled; C++ would push the pet unit on pet kills, a
// nuance Go's funnel has no model for.
func (s *session) fireCreatureDied(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	sv := s.server
	creature := sv.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = sv.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = sv.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	diedArgs := []any{scripting.CreatureEventOnDied, creature, s.luaPlayer()}
	_, _ = sv.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDied, diedArgs...)
	_, _ = sv.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDied, diedArgs...)
}

// fireCreatureTargetDied dispatches Eluna CREATURE_EVENT_ON_TARGET_DIED (3)
// when a creature kills another unit. C++ (Eluna::KilledUnit,
// CreatureHooks.cpp:156-163, reached via ElunaCreatureAI::KilledUnit,
// ElunaCreatureAI.h:117-121, from Unit::Kill, Unit.cpp:11324-11391) passes
// (event, creature, victim). Both binding families fire — entry handlers
// first, then the unique handlers for this creature's GUID/instance —
// matching SetupStack's merged call list (HookHelpers.h:37-39). The boolean
// veto gates only ScriptedAI::KilledUnit, whose sole implementation is the
// empty CreatureAI base (CreatureAI.h:133), a provable no-op like the
// SpellHit/JustDied rulings, so Go discards it.
func (s *Server) fireCreatureTargetDied(ctx context.Context, killer *creatureMotion, victim *scripting.Object) {
	if s == nil || killer == nil || victim == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(killer)
	if creature == nil {
		return
	}
	args := []any{scripting.CreatureEventOnTargetDied, creature, victim}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, killer.Entry, scripting.CreatureEventOnTargetDied, args...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, killer.GUID, killer.InstanceID, scripting.CreatureEventOnTargetDied, args...)
}

// livePetMotion returns the session's active pet motion when it is alive.
// C++ Unit::Kill's pet arm (Unit.cpp:11324-11335) notifies the killer's pet
// — player->GetPet() gated on alive, controlled (hunter/summon pet,
// Pet.h:53) and AI-enabled; Go pets are always player-bound motions driven
// by the pet tick, so a live motion found by PetGUID satisfies the gate.
func (s *session) livePetMotion() *creatureMotion {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 {
		return nil
	}
	motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil || motion.Health == 0 {
		return nil
	}
	return motion
}

// fireCreatureSpawned dispatches Eluna CREATURE_EVENT_ON_SPAWN (5) when a
// respawn timer restores a creature. C++ (Eluna::JustRespawned,
// CreatureHooks.cpp:210-216, reached in Trinity from
// ElunaCreatureAI::JustAppeared, ElunaCreatureAI.h:169-175, which the
// creature grid-update drives via m_triggerJustAppeared, Creature.
// cpp:699-704) calls On_Reset first — CREATURE_EVENT_ON_RESET (23) is a
// void hook (START_HOOK at CreatureHooks.cpp:277-282), returns discarded —
// then fires ON_SPAWN with (event, creature). Both binding families fire,
// entry then unique, matching SetupStack's merged call list
// (HookHelpers.h:37-39). The boolean veto gates only
// CreatureAI::JustAppeared's TempSummon follow-on-spawn terms (CreatureAI.
// cpp:189-205) — Go has no TempSummon model, so it is a provable no-op and
// is discarded. Go fires this only on the respawn path; C++ also fires it
// on first grid activation of a fresh spawn, which has no Go fire site.
func (s *Server) fireCreatureSpawned(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	spawnArgs := []any{scripting.CreatureEventOnSpawn, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnSpawn, spawnArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnSpawn, spawnArgs...)
}

// fireCreatureSummoned dispatches the Eluna summon hooks on the summoner
// creature: CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19) when a summon is
// created and CREATURE_EVENT_ON_SUMMONED_CREATURE_DESPAWN (20) when it is
// despawned. C++ (Eluna::JustSummoned / Eluna::SummonedCreatureDespawn,
// CreatureHooks.cpp:165-181, reached from ElunaCreatureAI::JustSummoned /
// SummonedCreatureDespawn, ElunaCreatureAI.h:122-134, fired by
// TempSummon::InitSummon / TempSummon::UnSummon,
// TemporarySummon.cpp:229-244/274-284) passes (event, creature, summon)
// through CallAllFunctionsBool. The boolean veto gates only
// ScriptedAI::JustSummoned / ScriptedAI::SummonedCreatureDespawn:
// ElunaCreatureAI derives from ScriptedAI (ElunaCreatureAI.h:19) and neither
// ScriptedAI nor CreatureAI overrides those two (the only overrides are
// BossAI/WorldBossAI's, which ElunaCreatureAI does not derive from), so both
// resolve to the empty CreatureAI bases (CreatureAI.h:136/139) — a provable
// no-op, and the veto is discarded, matching the JustDied/KilledUnit/
// SpellHit/ReachHome rulings. Both binding families fire, entry then unique,
// matching SetupStack's merged call list (HookHelpers.h:37-39). Must be
// called with motionMu released: the Lua handlers' object methods lock it on
// demand.
func (s *Server) fireCreatureSummoned(ctx context.Context, owner *creatureMotion, event int, summon *creatureMotion) {
	if s == nil || owner == nil || summon == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(owner)
	summoned := s.luaMotionCreature(summon)
	if creature == nil || summoned == nil {
		return
	}
	args := []any{event, creature, summoned}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, owner.Entry, event, args...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, owner.GUID, owner.InstanceID, event, args...)
}

// fireCreaturePetSummoned dispatches Eluna CREATURE_EVENT_ON_SUMMONED (22)
// on the summoned creature itself, with the summoner as the extra argument.
// C++ (Eluna::OnSummoned, CreatureHooks.cpp:86-94, reached from
// ElunaCreatureAI::IsSummonedBy, ElunaCreatureAI.h:246-253) pushes (event,
// creature, summoner) through CallAllFunctionsBool over entry then unique
// bindings. The boolean veto gates only ScriptedAI::IsSummonedBy, whose base
// is empty (CreatureAI.h) — a provable no-op like the JustDied/KilledUnit
// rulings — so Go discards it. Fires from the direct-construction pet
// funnel (registerPetMotion), which bypasses the AddToWorld/IsSummonedBy
// chain C++ Eluna-driven pets take. Must be called with motionMu released:
// Lua handler object methods lock it on demand.
func (s *Server) fireCreaturePetSummoned(ctx context.Context, motion *creatureMotion, summoner any) {
	if s == nil || motion == nil || summoner == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	args := []any{scripting.CreatureEventOnSummoned, creature, summoner}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnSummoned, args...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnSummoned, args...)
}

// firePetEnterCombat dispatches Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1)
// when a pet transitions from out-of-combat to engaged, with the victim as
// the extra argument (Eluna::EnterCombat, CreatureHooks.cpp:105-113, entry
// then unique bindings). C++ fires it for binding-carrying pets via
// ElunaCreatureAI::JustEngagedWith; the boolean veto gates only the empty
// ScriptedAI::JustEngagedWith base, so Go discards it. Call after the
// InCombat mutation, with motionMu released.
func (s *Server) firePetEnterCombat(ctx context.Context, motion *creatureMotion, targetGUID uint64) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	args := []any{scripting.CreatureEventOnEnterCombat, creature, s.luaUnitObjectForGUID(motion.Map, motion.InstanceID, targetGUID)}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnEnterCombat, args...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnEnterCombat, args...)
}

// firePetLeaveCombat dispatches the Eluna EnterEvadeMode pair —
// CREATURE_EVENT_ON_RESET (23) then CREATURE_EVENT_ON_LEAVE_COMBAT (2) —
// when a live pet disengages (Eluna::EnterEvadeMode, CreatureHooks.cpp:199-208,
// entry then unique bindings). The event-2 veto gates the evade itself in
// C++; Go's pet disengages carry no evade machine to gate, so both vetoes
// are provable no-ops and discarded, like the evade-veto ruling at
// creaturemotion.go:2025's inline twin. Call after the InCombat clear, with
// motionMu released.
func (s *Server) firePetLeaveCombat(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	leaveArgs := []any{scripting.CreatureEventOnLeaveCombat, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnLeaveCombat, leaveArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnLeaveCombat, leaveArgs...)
}

// luaQuest builds the Eluna Quest userdata surface used as the quest
// argument of the quest hooks. It mirrors the GetQuest global's quest
// object in engine/scripting/globals.go (ID/Name/Title/Level/MinLevel/Flags
// plus the GetId/GetLevel/GetMinLevel/GetFlags/GetNextQuestId/
// GetNextQuestInChain/GetPrevQuestId/GetType/HasFlag/IsDaily/IsRepeatable
// methods); a missing quest_template row yields a bare ID-only object so the
// hook still fires with the correct argument count (the GetQuest global
// pushes nil instead).
func (s *session) luaQuest(ctx context.Context, questID uint32) *scripting.Object {
	var id, level, minLevel, flags, nextID, chainNextID, prevID, questType int64
	var title string
	if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT ID, COALESCE(LogTitle, ''), COALESCE(QuestLevel, 0), COALESCE(MinLevel, 0), COALESCE(Flags, 0), COALESCE(RewardNextQuest, 0), COALESCE(NextQuestID, 0), COALESCE(PrevQuestId, 0), COALESCE(Type, 0) FROM quest_template WHERE ID = ?`, questID).Scan(&id, &title, &level, &minLevel, &flags, &chainNextID, &nextID, &prevID, &questType)
	}
	if id == 0 {
		id = int64(questID)
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetId"] = luaNoArgs(func() any { return uint32(id) })
	methods["GetLevel"] = luaNoArgs(func() any { return uint32(level) })
	methods["GetMinLevel"] = luaNoArgs(func() any { return uint32(minLevel) })
	methods["GetFlags"] = luaNoArgs(func() any { return uint32(flags) })
	methods["GetNextQuestId"] = luaNoArgs(func() any { return int32(nextID) })           // quest_template.NextQuestID (QuestDef.cpp:179)
	methods["GetNextQuestInChain"] = luaNoArgs(func() any { return int32(chainNextID) }) // quest_template.RewardNextQuest (QuestDef.h:253)
	methods["GetPrevQuestId"] = luaNoArgs(func() any { return int32(prevID) })
	methods["GetType"] = luaNoArgs(func() any { return uint32(questType) })
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		var flag uint32
		if len(args) > 0 {
			if n, ok := args[0].(uint32); ok {
				flag = n
			}
		}
		return []any{uint32(flags)&flag != 0}, nil
	}
	methods["IsDaily"] = luaNoArgs(func() any { return uint32(flags)&0x1000 != 0 })
	methods["IsRepeatable"] = luaNoArgs(func() any { return uint32(flags)&0x9000 != 0 })
	return &scripting.Object{Type: "Quest", Fields: map[string]any{
		"ID": uint32(id), "Name": title, "Title": title,
		"Level": uint32(level), "MinLevel": uint32(minLevel), "Flags": uint32(flags),
	}, Methods: methods}
}

// fireCreatureQuestHook dispatches an Eluna RegisterCreatureEvent quest hook
// for the creature questgiver: ON_QUEST_ACCEPT (31), ON_QUEST_REWARD (34) and
// ON_DIALOG_STATUS (35). C++ fires these only from the TYPEID_UNIT arms
// (Player.cpp:15119, QuestHandler.cpp:332, Player.cpp:16293); gameobject and
// item givers route to the GameObjectEvents and ItemQuestEvents families, so
// non-creature GUIDs are skipped here. Unlike the combat hooks, the quest
// hooks push the player first — Eluna argument order is (event, player,
// creature, quest[, opt]) per CreatureHooks.cpp. The return values of these
// hooks are discarded by every C++ call site, so Go discards them too.
// Reference: Eluna::OnQuestAccept / OnQuestReward / GetDialogStatus.
func (s *session) fireCreatureQuestHook(ctx context.Context, giverGUID uint64, event int, extra ...any) {
	if s == nil || s.server == nil || uint16(giverGUID>>48) != 0xF130 {
		return
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	motion := s.findCreatureMotion(giverGUID)
	if motion == nil {
		return
	}
	creature := s.server.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	args := make([]any, 0, len(extra)+3)
	args = append(args, event, s.luaPlayer(), creature)
	args = append(args, extra...)
	_, _ = s.server.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, event, args...)
}
