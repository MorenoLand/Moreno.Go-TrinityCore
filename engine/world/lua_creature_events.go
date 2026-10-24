package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
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

// backArcTarget resolves the target of the creature IsInBack bridge: either
// a Lua object with X/Y/GUID fields (a creature/player object, e.g. from
// GetVictim) or raw x, y numbers — the event-surface twin of luaAngleTarget
// in lua_objects.go.
func backArcTarget(args []any) (x, y float32, guid uint64, ok bool) {
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

// summonTempCreature is the shared spawn funnel behind the creature
// SummonCreature bridge below (Creature::SummonCreature -> TempSummon::InitSummon,
// TemporarySummon.cpp:168-244): spawns a temporary creature of the given
// entry at (x, y, z, o) on the summoner's map/instance. summonType is the
// bridge's 0-based TempSummonType scheme (0 TIMED_OR_DEAD_DESPAWN ... 7
// MANUAL_DESPAWN); despawnMs arms the timer only for the timed family
// (0-3). The all-zero x/y/z close-point and o==0 orientation fallbacks,
// the instance create-block broadcast, and the
// CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19) fire on the summoner
// (Eluna::JustSummoned, CreatureHooks.cpp:165-171) all live here. Unknown
// entries return nil (C++ logs and returns null for a missing template).
// The summon is NOT auto-aggroed: C++ arms call DoZoneInCombat/AttackStart
// explicitly, so ported arms must do the same.
// Player summoners (player:SummonCreature) share this funnel with a
// nil SummonerMotion: no event-19 fire, since players carry no creature AI.
// tempSummonOrigin is the summoner snapshot summonTempCreature spawns
// from: creature summoners pass their motion (which also carries the
// event-19 fire target), player summoners pass session state with a nil
// SummonerMotion (players carry no creature AI, so no JustSummoned
// event fires for them — the C++ player-summon arms call no AI hook).
type tempSummonOrigin struct {
	Map, InstanceID uint32
	GUID            uint64
	X, Y, Z, O      float32
	Reach           float64
	SummonerMotion  *creatureMotion
}

// summonOriginFromMotion snapshots a creature summoner under motionMu.
// Callers must not hold motionMu: summonTempCreature itself locked it
// for the same read before this refactor.
func (s *Server) summonOriginFromMotion(m *creatureMotion) tempSummonOrigin {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return tempSummonOrigin{Map: m.Map, InstanceID: m.InstanceID, GUID: m.GUID, X: m.X, Y: m.Y, Z: m.Z, O: m.Orientation, Reach: float64(m.CombatReach), SummonerMotion: m}
}

func (s *Server) summonTempCreature(ctx context.Context, origin tempSummonOrigin, entry uint32, x, y, z, o float32, summonType uint8, despawnMs uint32) *creatureMotion {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return nil
	}
	var exists int64
	if err := s.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM creature_template WHERE entry = ?", entry).Scan(&exists); err != nil || exists == 0 {
		return nil
	}
	mapID, instanceID, ownerGUID := origin.Map, origin.InstanceID, origin.GUID
	sx, sy, sz, summonerO := origin.X, origin.Y, origin.Z, origin.O
	reach := origin.Reach
	// Object.cpp:2059-2062 — the C++ float overload falls back to a
	// close point (GetClosePoint(combatReach): summoner position
	// offset by combat reach in facing direction, z = summoner z;
	// the collision/ground folds have no Go model, documented) when
	// x, y, and z are all 0, and to the summoner's orientation when
	// o is 0 (anubarak's scarab arm passes the 4-arg default
	// o=0.0f intending exactly this).
	if x == 0 && y == 0 && z == 0 {
		// GetClosePoint(x,y,z,GetCombatReach()) ->
		// GetNearPoint(nullptr,x,y,z,R,O) -> GetNearPoint2D adds
		// effectiveReach (R) on top of distance2d (R): 2*R in facing
		// direction (Object.cpp:2059,3307-3311). The collision/ground
		// folds and the LoS retry loop have no Go model, documented.
		x = sx + float32(2*reach*math.Cos(float64(summonerO)))
		y = sy + float32(2*reach*math.Sin(float64(summonerO)))
		z = sz
	}
	if o == 0 {
		o = summonerO
	}
	var walkSpeed, runSpeed float64 = 1.0, 1.14286
	_ = s.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(speed_walk, 1), COALESCE(speed_run, 1) FROM creature_template WHERE entry = ?",
		entry).Scan(&walkSpeed, &runSpeed)
	low := s.nextDynamicCreatureLowGUID()
	summoned := s.motionFor(ctx, low, entry, mapID, instanceID, x, y, z, o, 0, 0, float32(walkSpeed), float32(runSpeed), 0)
	if summoned == nil {
		return nil
	}
	s.motionMu.Lock()
	summoned.OwnerGUID = ownerGUID
	summoned.IsTempSummon = true
	summoned.TempSummonType = summonType
	// InitStats (TemporarySummon.cpp:168-183) stamps m_lifetime from
	// the summon type: only the timed family (0/1/2/3) arms the timer
	// at spawn. The corpse/dead family (4/5/6) starts its timer at
	// death — unmodeled here (documented delta) — and MANUAL (7) has
	// no timer.
	if despawnMs > 0 && summonType <= 3 {
		summoned.TempSummonLifetime = time.Duration(despawnMs) * time.Millisecond
		summoned.TempSummonExpiresAt = time.Now().Add(summoned.TempSummonLifetime)
	}
	stats := s.loadCreatureStats(ctx, entry)
	spawn := creatureSpawn{
		GUID: low, RawGUID: summoned.GUID, Entry: entry, Map: mapID,
		X: x, Y: y, Z: z, Orientation: o,
		Model: stats.Model, Faction: summoned.Faction, NPCFlags: stats.NPCFlags,
		UnitFlags: stats.UnitFlags, Level: stats.Level,
		Health: stats.Health, MaxHealth: stats.MaxHealth,
		Scale: stats.Scale, Mana: stats.Mana,
		WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed),
	}
	s.motionMu.Unlock()
	// Re-create the object client-side: the create block goes to the
	// whole instance (respawnEvadedCreature pattern), not just nearby
	// players — C++ AddToWorld notifies the full map.
	s.applyCreatureMotionToSpawn(playerState{Map: mapID, InstanceID: instanceID}, &spawn)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, nil))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastToInstance(mapID, instanceID, packet.Opcode, packet.Payload.Bytes(), nil)
	}
	if origin.SummonerMotion != nil {
		s.fireCreatureSummoned(ctx, origin.SummonerMotion, scripting.CreatureEventOnJustSummonedCreature, summoned)
	}
	return summoned
}

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
	// GetOwner returns the owning player object for a pet/guardian motion
	// (Unit::GetOwner, Unit.h) or nil when the motion has no owner or the
	// owner session is gone. Lets Lua port owner-targeted arms such as
	// the ebon gargoyle's JustDied owner-aura strip; the ev4 killer arg
	// is the killer session's player, not the owner, so handlers must not
	// use it as the owner.
	methods["GetOwner"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		ownerGUID := motion.OwnerGUID
		s.motionMu.Unlock()
		if ownerGUID == 0 {
			return []any{nil}, nil
		}
		if owner := s.findSessionByGUID(ownerGUID); owner != nil {
			return []any{owner.luaPlayer()}, nil
		}
		return []any{nil}, nil
	}
	methods["IsInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		inCombat := motion.InCombat
		s.motionMu.Unlock()
		return []any{inCombat}, nil
	}
	// IsInBack(target[, arc]) is WorldObject::isInBack(target, arc) =
	// !HasInArc(2*pi - arc, target) (Object.cpp:1417-1420), the event-surface
	// twin of the session bridge (lua_objects.go:477-496); Eluna's arc
	// defaults to pi. The target is a Lua object with X/Y fields or raw
	// x, y numbers; self is never in its own back arc. NOTE the direction:
	// creature:IsInBack(victim) is victim-behind-creature; C++ gates of the
	// form victim->isInBack(me) port as victim:IsInBack(creature) via the
	// player-surface bridge (lua_player.go).
	methods["IsInBack"] = func(_ context.Context, args []any) ([]any, error) {
		tx, ty, targetGUID, ok := backArcTarget(args)
		if !ok {
			return nil, fmt.Errorf("target is required")
		}
		if targetGUID != 0 && targetGUID == motion.GUID {
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
	// IsCharmed reports the engine's charm state (creaturemotion.go
	// charmCreatureFaction/uncharmCreature, the Unit::SetCharmedBy /
	// RemoveCharmedBy twin). Lets Lua gate possess/charm-sensitive arms
	// such as the Razuvious understudy's !isPossessedByPlayer() UpdateAI
	// leg; in that fight the only charm source is priest Mind Control.
	methods["IsCharmed"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		charmed := motion.Charmed
		s.motionMu.Unlock()
		return []any{charmed}, nil
	}
	// IsSummon is Unit::IsSummon (Unit.h:837 — UNIT_MASK_SUMMON): true
	// only for creatures stamped by the summon bridge (creaturemotion
	// IsTempSummon); DB creatures and pets report false. Lets Lua gate
	// IsSummon arms such as skeram's image evade/died/split legs.
	methods["IsSummon"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		isSummon := motion.IsTempSummon
		s.motionMu.Unlock()
		return []any{isSummon}, nil
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
	// IsTargetableForAttack is Unit::isTargetableForAttack (Unit.h:1077,
	// Unit.cpp): alive, no NON_ATTACKABLE (0x2) / NOT_SELECTABLE (0x02000000)
	// unit flags, and when checkFakeDeath (C++ default true) no feign-death
	// (UNIT_STATE_DIED; Go's unitFlag2FeignDeath twin). UNIT_STATE_UNATTACKABLE
	// has no Go model, so that arm is vacuous here. Not an exact twin of the
	// session binding (lua_objects.go:646): that one defaults mustBeDead=false
	// (Eluna CreatureMethods.h) and checks NON_ATTACKABLE only, while this one
	// defaults checkFakeDeath=true (C++ Unit.h:1077) and checks 0x02000000 too.
	// Zuljin's claw-rage chain calls this handler twin on the victim.
	methods["IsTargetableForAttack"] = func(_ context.Context, args []any) ([]any, error) {
		checkFakeDeath := true
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				checkFakeDeath = b
			}
		}
		s.motionMu.Lock()
		health, flags, flags2 := motion.Health, motion.UnitFlags, motion.UnitFlags2
		s.motionMu.Unlock()
		if health == 0 {
			return []any{false}, nil
		}
		if flags&(unitFlagNonAttackable|0x02000000) != 0 {
			return []any{false}, nil
		}
		if checkFakeDeath && flags2&unitFlag2FeignDeath != 0 {
			return []any{false}, nil
		}
		return []any{true}, nil
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
	// GetStackAmount is Aura::GetStackAmount (SpellAuras.h) for handler
	// creatures: the stack count of the named aura, or 0 when the aura is
	// absent (the C++ call sites null-check the Aura* first — returning 0
	// subsumes the missing-aura leg). Only engine-applied auras
	// (activeCreatureAuras) carry stacks; the Lua AddAura presence set
	// (creatureAuras) is boolean-only, so a Lua-added aura reads 0.
	methods["GetStackAmount"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := auraKey()
		s.auraMu.Lock()
		aura := s.activeCreatureAuras[key][spell]
		s.auraMu.Unlock()
		if aura == nil {
			return []any{uint32(0)}, nil
		}
		return []any{aura.StackAmount}, nil
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
	// DoZoneInCombat is CreatureAI::DoZoneInCombat (CreatureAI.cpp:73-103):
	// on a dungeon map, the handler creature engages every alive,
	// world-ready, non-GM player on its map/instance with a zero-threat
	// seed (Unit::EngageWithTarget, Unit.cpp:8429-8438 twin: ThreatMgr
	// AddThreat(guid, 0.0, true) + InCombat). C++ legs carried: the
	// IsDungeon gate (Go has no script logger on the handler path, so the
	// C++ error log becomes a silent no-op), the empty-player-list early
	// return, the dead-player skip, and CombatManager::CanBeginCombat's
	// gates as Go models them — alive, same map/instance from the session
	// enumeration (Go has no phase model, and the world-ready state is the
	// in-world twin), GM off (playerExtraGMOn) — plus the controlled-unit
	// leg: each qualifying player's pet motions (OwnerGUID,
	// creaturemotion.go:1160) are engaged too, like C++'s m_Controlled
	// loop. The vehicle-base leg has no Go twin (no vehicle model; the
	// same gap is documented on AddThreat in threat.go). EngageWithTarget
	// sends no packets in C++ (SMSG_ATTACK_START rides Unit::Attack, not
	// the engage), so this twin seeds threat and sets InCombat without
	// broadcasting. sessionsMu is never taken under motionMu: the player
	// targets resolve first, then one motion lock applies the seeds.
	// mandokir's JustEngagedWith tail ports on this.
	methods["DoZoneInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		if entry, found, err := s.Data.Map(mapID); err != nil || !found || !entry.IsDungeon() {
			return nil, nil
		}
		var players []uint64
		s.sessionsMu.RLock()
		for sess := range s.sessions {
			p := sess.player
			if p == nil || !sess.worldReady.Load() {
				continue
			}
			if p.Map != mapID || p.InstanceID != instanceID || p.Health == 0 {
				continue
			}
			if p.ExtraFlags&playerExtraGMOn != 0 {
				continue
			}
			players = append(players, sess.playerGUID)
		}
		s.sessionsMu.RUnlock()
		if len(players) == 0 {
			return nil, nil
		}
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(mapID, instanceID, guid)
		if m != nil && m.Health > 0 {
			if m.ThreatMgr == nil {
				m.ThreatMgr = NewThreatManager(m)
			}
			for _, pg := range players {
				m.ThreatMgr.AddThreat(pg, 0.0, true)
				for _, pm := range s.motionMapLocked(mapID, instanceID) {
					if pm != nil && pm.OwnerGUID == pg && pm.Health > 0 {
						m.ThreatMgr.AddThreat(pm.GUID, 0.0, true)
					}
				}
			}
			m.InCombat = true
		}
		s.motionMu.Unlock()
		return nil, nil
	}
	// ClearThreatList is UnitMethods::ClearThreatList
	// (UnitMethods.h:1992-1999): ThreatManager::ClearAllThreat via
	// Unit::GetThreatManager — Go's ClearThreat twin. WARNING: this WIPES
	// the threat table (evade semantics); it is NOT ScriptedAI::
	// ResetThreatList — use ResetAllThreat for that. Handler-surface twin
	// of the session-bound bridge (lua_objects.go:401): nightbane's
	// rain-of-bones, zuljin's EnterPhase and jeklik's DamageTaken phase-2
	// ports call it from RegisterCreatureEvent handlers, where the
	// session bridge is unreachable and the call nil-called.
	methods["ClearThreatList"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		return nil, nil
	}
	// ResetAllThreat is ThreatManager::ResetAllThreat
	// (ThreatManager.h:150-152): ScaleThreat(0.0f) on every entry — the
	// threat VALUES go to zero but the ENTRIES stay, so the owner does
	// not evade and UpdateVictim keeps a victim list. This is what
	// ScriptedAI::ResetThreatList (ScriptedCreature.cpp:229-234) lowers
	// to — "does not delete entries, just sets their threat to zero".
	// It is NOT ClearThreatList (ClearAllThreat: wipe + evade); ports
	// that used ClearThreatList for ResetThreatList arms deviated.
	methods["ResetAllThreat"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ResetThreat()
		}
		return nil, nil
	}
	// GetThreat is ThreatManager::GetThreat (ThreatManager.h:117,260):
	// the victim's effective threat (base + temp modifier, floored at
	// zero). A nil/unknown victim reads 0, matching the C++ gate
	// pattern if (GetThreat(victim)) used by the BWL wingbuffet /
	// knockback arms.
	methods["GetThreat"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 || motion.ThreatMgr == nil {
			return []any{float32(0)}, nil
		}
		if victimGUID := luaObjectGUID(args[0]); victimGUID != 0 {
			return []any{motion.ThreatMgr.GetThreat(victimGUID)}, nil
		}
		return []any{float32(0)}, nil
	}
	// ModifyThreatByPercent is ThreatManager::ModifyThreatByPercent
	// (ThreatManager.h:143): pct == 0 no-ops, otherwise
	// ScaleThreat(victim, 0.01*(100+pct)). The target may be a
	// creature/player object or a raw GUID (luaObjectGUID, the
	// CastSpell pattern); a nil/unknown victim no-ops. A victim
	// switch propagates to motion.TargetGUID on the motion tick
	// (creaturemotion.go), like every other threat mutation.
	methods["ModifyThreatByPercent"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 || motion.ThreatMgr == nil {
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
		motion.ThreatMgr.ScaleThreat(victimGUID, float32(0.01*float64(100+pct)))
		return nil, nil
	}
	// AddThreat is ScriptedAI::AddThreat (ScriptedCreature.cpp:202-209):
	// who defaults to me, lowering to ThreatManager::AddThreat with
	// ignoreModifiers/ignoreRedirects. The target may be a creature/player
	// object or a raw GUID (luaObjectGUID); a nil/unknown victim no-ops.
	// Go's AddThreat runs its eager victim-switch gate at the 130%
	// threshold (inMelee=false, threat.go), where C++ defers selection
	// to UpdateVictim — a 1000000.0f add switches either way. The
	// who-redirect leg (3rd C++ arg, e.g. malchezaar axe) is not
	// expressible here; this bridge is self-threat only.
	methods["AddThreat"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 || motion.ThreatMgr == nil {
			return nil, nil
		}
		victimGUID := luaObjectGUID(args[0])
		if victimGUID == 0 {
			return nil, nil
		}
		amount, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		motion.ThreatMgr.AddThreat(victimGUID, amount, false)
		return nil, nil
	}
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554):
	// HasUnitState(UNIT_STATE_CASTING). Go creature casts are fire-and-forget
	// (no in-flight cast state on the creature — the 2026-10-20 caststate
	// axiom), so the handler-surface twin is the constant false twin of the
	// session binding (lua_objects.go:342). Without it, ports calling
	// target:IsCasting() on a non-player threat victim (halazzi's shock
	// arm) nil-call and abort the timer chain mid-flight.
	methods["IsCasting"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{false}, nil
	}
	// InterruptNonMeleeSpells is Unit::InterruptNonMeleeSpells
	// (Unit.cpp: interrupt all non-melee casts, withDelayed included). Go
	// creature casts are fire-and-forget with no in-flight cast state (the
	// 2026-10-20 caststate axiom, same as IsCasting above), so the
	// handler-surface twin is a no-op: without it, svala's ritual-strike
	// arm nil-called here and aborted the handler before its CastSpell.
	methods["InterruptNonMeleeSpells"] = func(_ context.Context, _ []any) ([]any, error) {
		return nil, nil
	}
	// AddLootMode is CreatureMethods::AddLootMode (CreatureMethods.h:1321):
	// Creature::AddLootMode (Creature.h:227) sets the mode bits on
	// Creature::m_LootMode. Handler-surface twin of the session binding
	// (lua_objects.go:906): yogg-saron's ev14 Val'anyr SpellHit arm calls it
	// from a RegisterCreatureEvent handler, where the session bridge is
	// unreachable and the call nil-called.
	methods["AddLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.LootMode |= uint16(mode)
		s.motionMu.Unlock()
		return nil, nil
	}
	// WorldObject sound methods (PlayMusic/PlayDirectSound/PlayDistanceSound)
	// are Eluna LuaWorldObject methods (WorldObjectMethods.h:1161-1242);
	// creatures have no session, so they broadcast with a nil source like
	// the creature-text sound arm. Handler-surface registration of the same
	// helper the session surface uses (lua_objects.go:1036): the opera
	// wolfDied and gurtogg berserk ports call creature:PlayDirectSound from
	// RegisterCreatureEvent handlers, where the session-only registration
	// nil-called and aborted the arm before its cleanup legs.
	luaWorldObjectSoundMethods(s, nil, motion.GUID, methods)
	// GetGUIDLow is ObjectMethods::GetGUIDLow: the counter half of the
	// packed GUID. Handler-surface twin of the session binding
	// (lua_objects.go:196): the blackfathom_deeps ports key their timer
	// tables on it from RegisterCreatureEvent handlers, where the
	// session-only registration nil-called.
	methods["GetGUIDLow"] = luaNoArgs(func() any { return uint32(motion.GUID & 0x00FFFFFF) })
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
	// SetMaxHealth is Unit::SetMaxHealth (Unit.cpp:9588-9612): a 0 value
	// becomes 1, UNIT_FIELD_MAXHEALTH is set, and a current health above
	// the new max is clamped down via SetHealth. The values update
	// carries both fields when the clamp fires, max only otherwise.
	methods["SetMaxHealth"] = func(_ context.Context, args []any) ([]any, error) {
		maxHealth, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if maxHealth == 0 {
			maxHealth = 1
		}
		s.motionMu.Lock()
		motion.MaxHealth = maxHealth
		updates := map[int]uint32{unitFieldMaxHealth: maxHealth}
		if motion.Health > maxHealth {
			motion.Health = maxHealth
			updates[unitFieldHealth] = maxHealth
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, updates)
		return nil, nil
	}
	// SetPower is Unit::SetPower (Unit.cpp): writes UNIT_FIELD_POWER1+type,
	// clamped to the type's max (C++ clamps via std::min against
	// GetMaxPower), then broadcasts the update field. powerType is validated
	// 0-4 (POWER_MANA..POWER_HAPPINESS); SetPowerType legs stay out (no
	// power-type bridge).
	methods["SetPower"] = func(_ context.Context, args []any) ([]any, error) {
		powerType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if powerType > 4 {
			return nil, fmt.Errorf("SetPower requires a power type 0-4, got %d", powerType)
		}
		value, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		if value > motion.MaxPowers[powerType] {
			value = motion.MaxPowers[powerType]
		}
		motion.Powers[powerType] = value
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldPower1 + int(powerType): value})
		return nil, nil
	}
	// ModifyPower is Unit::ModifyPower (Unit.cpp:7439): the signed delta is
	// added to the current power, floored at 0, then SetPower clamps to the
	// type's max. Returns the new power, matching the C++ int32 return.
	methods["ModifyPower"] = func(_ context.Context, args []any) ([]any, error) {
		powerType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if powerType > 4 {
			return nil, fmt.Errorf("ModifyPower requires a power type 0-4, got %d", powerType)
		}
		delta, err := luaInt64Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		next := int64(motion.Powers[powerType]) + delta
		if next < 0 {
			next = 0
		}
		if next > int64(motion.MaxPowers[powerType]) {
			next = int64(motion.MaxPowers[powerType])
		}
		motion.Powers[powerType] = uint32(next)
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldPower1 + int(powerType): uint32(next)})
		return []any{uint32(next)}, nil
	}
	// SetMaxPower is Unit::SetMaxPower (Unit.cpp): writes
	// UNIT_FIELD_MAXPOWER1+type and broadcasts the update field. The current
	// power is left untouched (C++ does not clamp current on max change).
	methods["SetMaxPower"] = func(_ context.Context, args []any) ([]any, error) {
		powerType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if powerType > 4 {
			return nil, fmt.Errorf("SetMaxPower requires a power type 0-4, got %d", powerType)
		}
		value, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.MaxPowers[powerType] = value
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldMaxPower1 + int(powerType): value})
		return nil, nil
	}
	// AttackStop is Unit::AttackStop (Unit.cpp:5763-5787): when the
	// creature has a current target it drops the attack — SMSG_ATTACK_STOP
	// for the (attacker, victim) pair, the visual target
	// (UNIT_FIELD_TARGET) cleared, and the no-call-assistance flag reset.
	// Threat, TargetGUID and InCombat are untouched (that teardown is
	// Unit::CombatStop, Unit.cpp:5809 — see CombatStop below). The
	// InterruptSpell(CURRENT_MELEE_SPELL) leg is vacuous in Go (melee
	// swings carry no cast state); the victim-side _removeAttacker leg
	// rides the packet, matching the evade path's broadcast-only
	// treatment. No-op when no target is set (the !m_attacking early-out).
	methods["AttackStop"] = func(ctx context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		target := motion.TargetGUID
		if target == 0 {
			s.motionMu.Unlock()
			return nil, nil
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		motion.AlreadyCallAssistance = false
		s.motionMu.Unlock()
		stopPkt := buildAttackStop(guid, target, false)
		s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
		s.clearCreatureVisualTarget(motion)
		return nil, nil
	}
	// CombatStop is Unit::CombatStop (Unit.cpp:5809-5825) for handler
	// creatures: in C++ order, InterruptNonMeleeSpells(false) when
	// includingCast (the Go cast-state gate is motion.CastingUntil, the
	// evade precedent at creaturemotion.go:2191), AttackStop (own victim
	// dropped, SMSG_ATTACK_STOP, visual target cleared, assistance flag
	// reset), RemoveAllAttackers (every attacker stops — see
	// removeCreatureAttackers), then the mutualPvP=false leg,
	// CombatManager::EndAllPvECombat (CombatManager.cpp:344-350):
	// RemoveMeFromThreatLists + ClearAllThreat + InCombat=false.
	// NOT carried: the player-only SendAttackSwingCancelAttack (this
	// method exists only on creature objects), SuppressPvPCombat (Go has
	// no PvP-suppression model outside sanctuary handling), health
	// restore / home walk / aura clear (those are EnterEvadeMode legs,
	// not CombatStop's), and no Lua hooks fire (C++ CombatStop fires
	// none — OnLeaveCombat belongs to EnterEvadeMode). The first arg is
	// the optional includingCast flag (default false, the C++ default).
	methods["CombatStop"] = func(_ context.Context, args []any) ([]any, error) {
		includingCast := false
		if len(args) >= 1 {
			if b, ok := args[0].(bool); ok {
				includingCast = b
			}
		}
		s.motionMu.Lock()
		victim := motion.TargetGUID
		if includingCast {
			motion.CastingUntil = time.Time{}
		}
		motion.TargetGUID = 0
		motion.AlreadyCallAssistance = false
		motion.AssistAt = time.Time{}
		motion.AssistVictim = 0
		motion.Assistants = nil
		motion.InCombat = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		if victim != 0 {
			stopPkt := buildAttackStop(guid, victim, false)
			s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
		}
		s.clearCreatureVisualTarget(motion)
		s.removeCreatureAttackers(mapID, instanceID, guid)
		s.removeThreatVictimFromAllLists(mapID, instanceID, guid)
		s.broadcastThreatClearInInstance(mapID, instanceID, guid)
		return nil, nil
	}
	// RemoveAllAttackers is Unit::RemoveAllAttackers (Unit.cpp:5867-5878)
	// on its own: the CombatStop attacker leg without the self teardown
	// — freya's own JustDied calls it without CombatStop, so the two
	// stay separate methods (see removeCreatureAttackers).
	methods["RemoveAllAttackers"] = func(_ context.Context, _ []any) ([]any, error) {
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.removeCreatureAttackers(mapID, instanceID, guid)
		return nil, nil
	}
	// StopMoving is Unit::StopMoving (Unit.cpp:10499-10511): the moving
	// state is cleared and the stop is broadcast (MoveSplineInit::Stop's
	// MonsterMoveStop), so clients halt the creature at its current
	// position. Threat, TargetGUID and InCombat are untouched — unlike
	// stopCreatureMotion, which is the evade-path teardown, not this
	// twin. No-op when the creature is not moving (the movespline
	// Finalized early-out).
	methods["StopMoving"] = func(ctx context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		if !motion.Moving {
			s.motionMu.Unlock()
			return nil, nil
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		x, y, z := motion.X, motion.Y, motion.Z
		motion.Moving = false
		s.motionMu.Unlock()
		s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z)
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
	// Kill is Unit::Kill (Unit.cpp:11188-11387) with the caller as both
	// killer and victim — the Creature::KillSelf twin. The death runs
	// through the spell insta-kill victim path (boss_ai.go
	// creatureInstaKillVictim): DamageTaken/Dealt hooks, tap, lootable
	// corpse flag, threat teardown, KILL/KILLED/DEATH proc auras,
	// death-aura strip, the KilledUnit branch arm and BossAI OnDied.
	// The Lua event chain (23 Reset then 4 Died, the session kill
	// funnel's order) fires here only when no player owns the victim:
	// a player-owned victim's owner path already funnels through
	// onCreatureKilled, which fires both events, so firing again
	// would double-dispatch. Already-dead callers are a no-op (the
	// victim path's Health == 0 early-out).
	methods["Kill"] = func(ctx context.Context, _ []any) ([]any, error) {
		if motion == nil {
			return nil, nil
		}
		ownerPlayer := false
		if motion.OwnerGUID != 0 {
			if owner := s.findSessionByGUID(motion.OwnerGUID); owner != nil && owner.player != nil {
				ownerPlayer = true
			}
		}
		s.creatureInstaKillVictim(ctx, motion, motion.GUID)
		if ownerPlayer || s.Features == nil || s.Features.Scripts == nil {
			return nil, nil
		}
		creature := s.luaMotionCreature(motion)
		if creature == nil {
			return nil, nil
		}
		resetArgs := []any{scripting.CreatureEventOnReset, creature}
		_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
		_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
		diedArgs := []any{scripting.CreatureEventOnDied, creature, creature}
		_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDied, diedArgs...)
		_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDied, diedArgs...)
		return nil, nil
	}
	// Teleport is Unit::NearTeleportTo (Unit.cpp:13332-13346) for creatures:
	// DisableSpline (motion.Moving cleared, the in-flight spline twin) +
	// SendTeleportPacket (MSG_MOVE_TELEPORT carrying the creature GUID to
	// nearby players, Unit.cpp:13348) + UpdatePosition (motion X/Y/Z/
	// Orientation under motionMu). UpdateObjectVisibility has no Go twin —
	// visibility is evaluated per broadcast from the motion table. The
	// orientation argument is optional and defaults to the current facing.
	methods["Teleport"] = func(ctx context.Context, args []any) ([]any, error) {
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
		s.motionMu.Lock()
		motions := s.motionMapLocked(motion.Map, motion.InstanceID)
		m, ok := motions[motion.GUID]
		if !ok {
			s.motionMu.Unlock()
			return nil, nil
		}
		o := m.Orientation
		if len(args) > 3 {
			o, err = luaFloat32Arg(args, 3)
			if err != nil {
				s.motionMu.Unlock()
				return nil, err
			}
		}
		m.X, m.Y, m.Z, m.Orientation = x, y, z, o
		m.Moving = false
		mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
		s.motionMu.Unlock()
		_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: x, Y: y, Z: z, Orientation: o})
		s.broadcastCreatureTeleportMovement(mapID, instanceID, x, y, nearby)
		return nil, nil
	}
	// SetDisableGravity is Creature::SetDisableGravity (Creature.cpp:3027):
	// flips MOVEMENTFLAG_DISABLE_GRAVITY (and clears MOVEMENTFLAG_FALLING)
	// with the C++ no-op-on-unchanged early-out, then broadcasts
	// SMSG_SPLINE_MOVE_GRAVITY_DISABLE/ENABLE (packed GUID only,
	// Creature.cpp:3040-3045). The animation-tier fold (Fly/Ground/Hover)
	// stays out — the Go engine models no animation tiers (the waypoint
	// LAND/TAKEOFF precedent, creaturemotion.go). The falling-flag clear is
	// internal: the Go engine keeps no per-creature movement flags, so the
	// bit lives on motion for idempotence and the packet is the
	// client-visible leg (the Teleport bridge precedent). C++ skips the
	// packet when the move spline is uninitialized; the Go movement model
	// has no splines, so the broadcast always goes (matching the ported
	// arms, which always pair it with a visible motion change).
	methods["SetDisableGravity"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetDisableGravity requires a bool, got no args")
		}
		disable, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetDisableGravity requires a bool")
		}
		s.motionMu.Lock()
		if motion.GravityDisabled == disable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.GravityDisabled = disable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_ENABLE
		if disable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_DISABLE
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode)
		return nil, nil
	}
	// GetZoneId/GetAreaId are Unit::GetZoneId()/GetAreaId() via
	// Map::GetZoneAndAreaId (Map.cpp:2641): the area id resolves from
	// the creature's map + position (WMO area, then the .map area grid,
	// then the map's default AreaTableID) and the zone id is the area's
	// ParentAreaID when set. The Go twin is Server.zoneAndAreaID
	// (terrain.go), the same resolution creatures use for spell-area
	// checks (checkMotionSpellCheckLocation). Both return 0 when the
	// position cannot be resolved — the C++ fail-open twin of an
	// absent area row. No phase-mask leg: the Go grid model has none.
	methods["GetZoneId"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, x, y, z := motion.Map, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		zoneID, _ := s.zoneAndAreaID(mapID, x, y, z, 0)
		return []any{zoneID}, nil
	}
	methods["GetAreaId"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, x, y, z := motion.Map, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		_, areaID := s.zoneAndAreaID(mapID, x, y, z, 0)
		return []any{areaID}, nil
	}
	// SetHover is Creature::SetHover (Creature.cpp:3105): flips
	// MOVEMENTFLAG_HOVER with the C++ no-op-on-unchanged early-out, then
	// broadcasts SMSG_SPLINE_MOVE_SET_HOVER/UNSET_HOVER (packed GUID only,
	// Creature.cpp:3121-3126). The hover-height Z fold (Unit.cpp:13789-13803,
	// GetFloatValue UNIT_FIELD_HOVERHEIGHT) stays out — the Go engine models
	// no creature hover height; the animation-tier fold stays out with it
	// (SetDisableGravity above).
	methods["SetHover"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetHover requires a bool, got no args")
		}
		enable, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetHover requires a bool")
		}
		s.motionMu.Lock()
		if motion.Hovering == enable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.Hovering = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_HOVER
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_HOVER
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode)
		return nil, nil
	}
	// SetCanFly is Creature::SetCanFly (Creature.cpp:3060): flips
	// MOVEMENTFLAG_CAN_FLY (and clears the moving-fly mask / FALLING on
	// disable) with the C++ no-op-on-unchanged early-out, then broadcasts
	// SMSG_SPLINE_MOVE_SET_FLYING/UNSET_FLYING (packed GUID only,
	// Creature.cpp:3067-3071). The animation-tier fold stays out (the
	// SetDisableGravity precedent, above).
	methods["SetCanFly"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetCanFly requires a bool, got no args")
		}
		enable, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetCanFly requires a bool")
		}
		s.motionMu.Lock()
		if motion.Flying == enable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.Flying = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_FLYING
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_FLYING
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode)
		return nil, nil
	}
	// SetCombatPulseDelay is Creature::SetCombatPulseDelay (Creature.h:270):
	// the interval in SECONDS at which the creature pulses its whole zone
	// into combat (only fires on dungeon maps). The C++ setter stores the
	// delay and re-seeds m_combatPulseTime when it is 0 or larger than the
	// new delay; the Go twin keeps the same two fields on the motion and
	// the same seeding rule. Note the C++ quirk (Creature.h:270-275):
	// m_combatPulseTime is MILLISECONDS but the setter writes the raw
	// SECONDS value into it, so the first pulse fires on the next update
	// tick after SetCombatPulseDelay(5) — the Go timer seeds the same raw
	// numeric value as milliseconds, then re-arms at delay*1000ms after
	// each pulse (Creature.cpp:866). The pulse itself runs in
	// stepCreatureCombatPulse (creaturemotion.go). There is no client
	// broadcast — C++ sends none.
	methods["SetCombatPulseDelay"] = func(_ context.Context, args []any) ([]any, error) {
		delay, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.CombatPulseDelaySecs = delay
		delayMs := time.Duration(delay) * time.Millisecond
		if motion.CombatPulseTimer == 0 || motion.CombatPulseTimer > delayMs {
			motion.CombatPulseTimer = delayMs
		}
		s.motionMu.Unlock()
		return nil, nil
	}
	// SummonCreature is Creature::SummonCreature (Creature.cpp: the
	// TempSummon::InitSummon twin, TemporarySummon.cpp:168-244): spawns a
	// temporary creature of the given entry at (x, y, z, o) on the
	// summoner's map/instance and returns it. Eluna CreatureMethods::
	// SummonCreature signature (entry, x, y, z, o, summonType,
	// despawnTimer); summonType is the C++ TempSummonType enum (0
	// TIMED_OR_DEAD_DESPAWN, 1 TIMED_OR_CORPSE_DESPAWN, 2 TIMED_DESPAWN,
	// 3 TIMED_DESPAWN_OUT_OF_COMBAT, 4 CORPSE_DESPAWN, 5 CORPSE_TIMED_DESPAWN,
	// 6 DEAD_DESPAWN, 7 MANUAL_DESPAWN) and defaults to 7; despawnTimer is
	// milliseconds, default 0. All-zero x/y/z falls back to a close point
	// 2*combatReach in facing direction (Object.cpp:2059-2060 twin, the
	// SummonGameObject fallback above); an o of 0 falls back to the
	// summoner's orientation (Object.cpp:2061-2062 twin). C++ InitStats
	// (TemporarySummon.cpp:168-183)
	// stamps the template's level/stats — the motionFor funnel does the
	// same from creature_template — and TempSummon::InitSummon fires
	// Eluna::JustSummoned (CreatureHooks.cpp:165-171), twinned here as
	// CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19) on the summoner after
	// the lock release (Lua handlers lock motionMu on demand). The summon
	// GUID allocates from the dynamic-creature range (totem precedent) and
	// never collides with DB rows. Unknown entries return nil (C++ logs
	// and returns null for a missing template). The summon is NOT
	// auto-aggroed: C++ arms call DoZoneInCombat/AttackStart explicitly,
	// so ported arms must do the same.
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
		summoned := s.summonTempCreature(ctx, s.summonOriginFromMotion(motion), entry, x, y, z, o, uint8(summonType), despawnMs)
		if summoned == nil {
			return []any{nil}, nil
		}
		return []any{s.luaMotionCreature(summoned)}, nil
	}
	// SummonGameObject is WorldObject::SummonGameObject
	// (Object.cpp:2065-2096): spawns a runtime gameobject of the given entry
	// at (x, y, z, o) on the summoner's map/instance and returns its packed
	// GUID. Eluna CreatureMethods::SummonGameObject signature
	// (entry, x, y, z, o, respawnTime); respawnTime is seconds, default 0.
	// A missing template logs and returns nil (C++ TC_LOG_ERROR + null).
	// All-zero coords fall back to a close point 2*combatReach in facing
	// direction at the summoner (Object.cpp:2095-2099 ->
	// GetClosePoint(combatReach) -> GetNearPoint2D: effectiveReach (R) on
	// top of distance2d (R); the collision/ground folds have no Go model,
	// documented). The summon
	// type is the C++ default GO_SUMMON_TIMED_OR_CORPSE_DESPAWN: a positive
	// respawnTime despawns the object when it elapses (time.AfterFunc, the
	// gobject-add-temp precedent; the respawn scheduler is unmodeled), 0
	// persists. The summoner-GO tracking leg (Unit::AddGameObject, Unit.cpp)
	// has no Lua-visible effect — Go carries OwnerGUID on the dyn state,
	// which is what the instance hooks and rituals read.
	// spawnDynamicGameObject broadcasts the create block to the map/instance
	// and fires the GO-ADD + instance-GO-CREATE hooks, twinning
	// map->AddToMap (Object.cpp:2089).
	methods["SummonGameObject"] = func(ctx context.Context, args []any) ([]any, error) {
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
		var respawnSecs uint32
		if len(args) > 5 {
			if respawnSecs, err = luaUint32Arg(args, 5); err != nil {
				return nil, err
			}
		}
		if s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{nil}, nil
		}
		tmpl, err := gobjectTemplateByEntry(ctx, s.WorldStore.DB, entry)
		if err != nil || tmpl == nil {
			return []any{nil}, nil
		}
		s.motionMu.Lock()
		mapID, instanceID, ownerGUID := motion.Map, motion.InstanceID, motion.GUID
		sx, sy, sz, so := motion.X, motion.Y, motion.Z, motion.Orientation
		reach := float64(motion.CombatReach)
		s.motionMu.Unlock()
		if x == 0 && y == 0 && z == 0 {
			// 2*R in facing direction: GetClosePoint(x,y,z,GetCombatReach())
			// -> GetNearPoint2D adds effectiveReach (R) on top of distance2d
			// (R) (Object.cpp:2095-2099,3307-3311). ang falls back to the
			// summoner orientation only in the all-zero branch (Object.cpp:
			// 2099), twinned here. Collision/ground folds unmodeled.
			x = sx + float32(2*reach*math.Cos(float64(so)))
			y = sy + float32(2*reach*math.Sin(float64(so)))
			z, o = sz, so
		}
		lowGUID := s.nextDynamicGameObjectLowGUID()
		newGUID := gameObjectGUID(lowGUID, entry)
		size := tmpl.size
		if size <= 0 {
			size = 1.0
		}
		dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: entry, OwnerGUID: ownerGUID, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, Orientation: o, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, IsRuntimeSpawn: true}
		if respawnSecs > 0 {
			dyn.DespawnTimer = time.AfterFunc(time.Duration(respawnSecs)*time.Second, func() {
				s.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
			})
		}
		s.spawnDynamicGameObject(dyn)
		return []any{newGUID}, nil
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
	// Is25ManRaid is Map::Is25ManRaid (Map.cpp:4254-4258) and ScriptedAI::Is25ManRaid
	// (ScriptedCreature.h:247): raids only, and since 25-man difficulties are 1 and 3
	// (DBCEnums.h:287-292) the test is difficulty & RAID_DIFFICULTY_MASK_25MAN (1).
	// Same instance-row read as IsHeroic above: non-instance maps default false.
	methods["Is25ManRaid"] = func(ctx context.Context, _ []any) ([]any, error) {
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
		return []any{isRaid && difficulty&1 == 1}, nil
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
	// isCreaturePassive (creaturemotion.go:617) and help-call filtering, so
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
	// SetStandState is Unit::SetStandState (Unit.cpp:10557-10570):
	// SetByteValue(UNIT_FIELD_BYTES_1, UNIT_BYTES_1_OFFSET_STAND_STATE,
	// state) (UNIT_STAND_STATE_STAND=0, SIT=1, SIT_CHAIR=2, SLEEP=3,
	// SIT_LOW_CHAIR=4, SIT_MEDIUM_CHAIR=5, SIT_HIGH_CHAIR=6, DEAD=7,
	// KNEEL=8, SUBMERGED=9, SharedDefines.h); the player-only
	// SMSG_STANDSTATE_UPDATE leg is N/A for creatures. The
	// RemoveAurasWithInterruptFlags(AURA_INTERRUPT_FLAG_NOT_SEATED) leg
	// (state != STAND) stays out: it is vacuous for every current caller
	// (feign arms RemoveAllAuras first; nefarius's barrier is stripped by
	// his explicit RemoveAura) and wiring it would require unlocking
	// motionMu around the aura-strip call.
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
	// SetRooted is Unit::SetControlled(state, UNIT_STATE_ROOT): sets or
	// clears the root leg of UNIT_STATE_NOT_MOVE (Unit.h:256) on the
	// handler creature. The engine honors motion.ScriptRooted in the
	// chase-pause gate (ChaseMovementGenerator.cpp:98-106) and the
	// waypoint-launch gate (WaypointMovementGenerator.cpp:141), so
	// feign-death / phase-change root arms (stalagg, feugen, thekal,
	// kologarn) port with real effect; melee swings still fire while
	// rooted (DoMeleeAttackIfReady gates UNIT_STATE_CASTING only,
	// UnitAI.cpp:61-63). STUNNED has no bridge (cast/melee blocking
	// needs new engine state) — a future dimension.
	methods["SetRooted"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetRooted requires a bool, got no args")
		}
		rooted, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetRooted requires a bool")
		}
		s.motionMu.Lock()
		motion.ScriptRooted = rooted
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetCombatMovement is ScriptedAI::SetCombatMovement
	// (ScriptedCreature.h:228-233): controls whether MoveChase() is used in
	// AttackStart — ranged AIs stand and cast instead of chasing the
	// victim. C++ NOTE: in-combat calls do NOTHING (the flag only affects
	// AttackStart), and the value is not reset at Reset(). The Lua twin
	// stores the inverted flag on motion; the engine honors
	// motion.NoCombatMovement in the combat-tick chase gate
	// (ChaseMovementGenerator.cpp:98-106), so ctor/Reset-time arms port
	// with real effect when called at OnEnterCombat (the ctor-equivalent
	// point — the first chase can only happen after combat starts).
	// Mid-combat Lua calls apply at the next tick (C++ would wait for the
	// next AttackStart) — ports only call it pre-combat per the C++ NOTE.
	methods["SetCombatMovement"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetCombatMovement requires a bool, got no args")
		}
		allow, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetCombatMovement requires a bool")
		}
		s.motionMu.Lock()
		motion.NoCombatMovement = !allow
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetRegenerateHealth is Creature::SetRegenerateHealth (Creature.h:286):
	// C++ flips only _regenerateHealthLock, so false disables the 2s
	// RegenerateHealth tick and true restores the default-enabled state.
	// The Lua twin stores the inverted flag on motion; the regen tick
	// honors motion.RegenHealthDisabled. Zero value = enabled (the C++
	// default, Creature.cpp:261).
	methods["SetRegenerateHealth"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetRegenerateHealth requires a bool, got no args")
		}
		regen, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetRegenerateHealth requires a bool")
		}
		s.motionMu.Lock()
		motion.RegenHealthDisabled = !regen
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetMeleeAttack declares that the creature's C++ AI never melee-swings:
	// boss-class custom UpdateAIs (muru, eredar twins, gothik, novos, sara,
	// voice-of-yogg, rimefang, julianne, krosh firehand, flame leviathan,
	// kalecgos-kj) pump the scheduler / cast without the trailing
	// DoMeleeAttackIfReady arm (UnitAI.cpp:61-83), while the base UpdateAIs
	// of ScriptedAI (ScriptedCreature.cpp:150), BossAI (:571), SmartAI
	// (SmartAI.cpp:284), CombatAI (:89), AggressorAI (:43) and ArcherAI
	// (:219) all end in DoMeleeAttackIfReady. There is no C++ setter to
	// twin — the behavior is the UpdateAI's omission of the call — so the
	// Lua twin stores the inverted flag on motion and the engine honors
	// motion.NoMeleeAttack in the combat-tick swing gate, restoring the
	// cast-only behavior with real effect. Zero value = swing allowed (the
	// ScriptedAI/BossAI default). Ports call it at OnEnterCombat (the
	// ctor-equivalent point, SetCombatMovement precedent); the flag is
	// structural and persists across the fight.
	methods["SetMeleeAttack"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetMeleeAttack requires a bool, got no args")
		}
		allow, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetMeleeAttack requires a bool")
		}
		s.motionMu.Lock()
		motion.NoMeleeAttack = !allow
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetDisplayId is Unit::SetDisplayId (Unit.cpp:10601):
	// SetUInt32Value(UNIT_FIELD_DISPLAYID, modelId), plus the
	// Creature::SetDisplayId (Creature.cpp:3210-3218) bounding-radius /
	// combat-reach folds from CreatureModelInfo, including the
	// * GetObjectScale() multiplier (the SetScale script-override /
	// template-base twin). The gender fold (Unit.cpp:10605-10606) has
	// no server read path, so it is not modeled. The pet branch
	// (IsPet: 1.0f / DEFAULT_PLAYER_COMBAT_REACH) stays out — the
	// handler surface does not serve pets. applyCreatureMotionToSpawn overlays motion.DisplayID onto
	// the spawn record (creatures.go), so morph/disguise arms (hodir
	// icicle, valithria dream-slip, ...) port with real client-visible
	// effect; the values-update broadcast pushes the new display to the
	// instance immediately, matching the C++ SetUInt32Value update path.
	// RestoreDisplayId is the Lua-side idiom SetDisplayId(nativeDisplay),
	// with the native model captured via GetDisplayId at spawn.
	methods["SetDisplayId"] = func(ctx context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		fields := map[int]uint32{unitFieldDisplayID: id}
		var boundingRadius, combatReach float64
		if s != nil && s.WorldStore != nil && s.WorldStore.DB != nil {
			var br, cr sql.NullFloat64
			if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT BoundingRadius, CombatReach FROM creature_model_info WHERE DisplayID = ?", id).Scan(&br, &cr); err == nil {
				if br.Valid && br.Float64 > 0 {
					boundingRadius = br.Float64
				}
				if cr.Valid && cr.Float64 > 0 {
					combatReach = cr.Float64
				}
			}
		}
		s.motionMu.Lock()
		motion.DisplayID = id
		motion.DisplayIDKnown = true
		// Creature::SetDisplayId (Creature.cpp:3216-3217) multiplies the
		// model-info radius/reach by the live object scale (the SetScale
		// twin: script override while known, else the template base).
		scale := motion.BaseScale
		if motion.ScriptScaleKnown {
			scale = motion.ScriptScale
		}
		if scale <= 0 {
			scale = 1
		}
		if boundingRadius > 0 {
			fields[unitFieldBoundingRadius] = math.Float32bits(float32(boundingRadius) * scale)
		}
		if combatReach > 0 {
			fields[unitFieldCombatReach] = math.Float32bits(float32(combatReach) * scale)
			motion.CombatReach = float32(combatReach) * scale
		}
		s.motionMu.Unlock()
		if s != nil {
			s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, fields)
		}
		return nil, nil
	}
	// GetDisplayId is Unit::GetDisplayId: the live UNIT_FIELD_DISPLAYID
	// (template-native model until a SetDisplayId override lands). Lua
	// ports capture this at engage/spawn to implement
	// Unit::RestoreDisplayId as SetDisplayId(nativeDisplay).
	methods["GetDisplayId"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		id := motion.DisplayID
		s.motionMu.Unlock()
		return []any{id}, nil
	}
	// GetTemplateModelId is the Lua read path for
	// Creature::GetCreatureTemplate()->Modelid1..4 (Creature.h): the
	// template model-id row backing the spawn display. C++ script arms
	// morph with SetDisplayId(me->GetCreatureTemplate()->Modelid2) — the
	// Lua surface had no template-model getter, so those arms stayed out
	// (sironas Reset, muru dark-fiend ctor). Index 1-4; returns 0 for an
	// out-of-range index or a missing template row.
	methods["GetTemplateModelId"] = func(ctx context.Context, args []any) ([]any, error) {
		idx, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if idx < 1 || idx > 4 || s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{uint32(0)}, nil
		}
		cols := [4]string{"modelid1", "modelid2", "modelid3", "modelid4"}
		var v sql.NullInt64
		if err := s.WorldStore.DB.QueryRowContext(ctx,
			"SELECT "+cols[idx-1]+" FROM creature_template WHERE entry = ?", motion.Entry).Scan(&v); err != nil || !v.Valid {
			return []any{uint32(0)}, nil
		}
		return []any{uint32(v.Int64)}, nil
	}
	// UpdateEntry is Creature::UpdateEntry (Creature.cpp:552-655): re-seats
	// the creature on a new creature_template entry — the scripted
	// transform twin (volkhan's brittle golem, the deathwhisper
	// fanatic/adherent transforms, ingvar's undead phase, ...). Returns
	// false when the template is missing or has no model (the C++
	// InitEntry false legs, Creature.cpp:454-459/494-499).
	//
	// Seated legs, in C++ order: entry + first-valid template model
	// (InitEntry, Creature.cpp:452-550: GetFirstValidModelId over
	// modelid1..4, SetDisplayId/SetNativeDisplayId, default equipment
	// when data is nil), template faction, template npcflag/unit_flags
	// (UNIT_NPC_FLAGS/UNIT_FIELD_FLAGS, keeping UNIT_FLAG_IN_COMBAT while
	// engaged)/unit_flags2/dynamicflags, attack time, SelectLevel
	// (urand(min,max)), UpdateLevelDependantStats with the previous-health
	// preservation (SetHealth clamps to the new max), armor/resistances/
	// damage from the template, template scale (the script scale override
	// is cleared — the C++ Reset/JustDied scale-restore precedent),
	// RegenHealth, walk/run speeds, sheath MELEE when no creature_addon
	// row, and InitializeReactState. The motion-map key and motion.GUID
	// keep the ORIGINAL entry embedded — C++ fixes the GUID at
	// Object::Create and only SetEntry changes (Creature.cpp:486) — so
	// client references, threat, and lookups keep resolving; the new
	// entry rides OBJECT_FIELD_ENTRY in the values-update and the spawn
	// overlay (creatures.go) so late-joining clients resolve the right
	// name/model. Lua ports register entry-persistent handlers (damage,
	// SpellHit) for BOTH entries — the C++ AI object keeps receiving
	// hooks after the transform (LastUsedScriptID leg), and Go dispatches
	// on the live motion.Entry.
	//
	// Not modeled: the guardian-stats leg (the handler surface does not
	// serve pets), the vehicle branch, trigger NOT_SELECTABLE,
	// CREATURE_FLAG_EXTRA_NO_TAUNT immunities, IgnoringCombat, melee
	// damage school, the faction-template PvP flag, and threat
	// EvaluateSuppressed. The data/updateLevel parameters stay at their
	// C++ defaults (nil/true) — every scripted call site uses them.
	methods["UpdateEntry"] = func(ctx context.Context, args []any) ([]any, error) {
		newEntry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		ok := s.seatCreatureEntry(ctx, motion, newEntry)
		return []any{ok}, nil
	}
	// SetFacingTo is Unit::SetFacingTo (Unit.cpp:13669): an in-place
	// MoveTo + SetFacing spline that turns the unit without moving it. The
	// Go twin reuses the DistractMovementGenerator facing pattern — an
	// in-place broadcastMonsterMoveInInstance with the Final_Angle arm
	// (creaturemotion.go:2458) — plus the logical motion.Orientation slot
	// write the chase worker and the values snapshot read. C++ skips the
	// turn when the unit is moving (the IsStopped gate); the Go twin
	// documents that instead of gating: a mid-spline call sets the logical
	// orientation but the client turn packet can race the spline.
	// SetFacingToObject(obj) has no direct bridge — Lua ports compute the
	// facing with math.atan2 from creature:GetX()/GetY() and the target's
	// coordinates (the Unit::GetAbsoluteAngle twin, atan2 normalized to
	// [0, 2pi) per creaturemotion.go:2430), then call this bridge. Creature::SetOrientation (the
	// raw slot write with no packet) is the slot half of this bridge.
	methods["SetFacingTo"] = func(_ context.Context, args []any) ([]any, error) {
		ori, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.Orientation = ori
		guid, mapID, instID := motion.GUID, motion.Map, motion.InstanceID
		x, y, z := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, ori, true)
		return nil, nil
	}
	// SetSpeedRate is Unit::SetSpeedRate (Unit.cpp:8821) for MOVE_RUN=1
	// only: the rate is the raw m_speed_rate multiplier on base move speed
	// (7.0 yd/s run; C++ SetSpeed's absolute form divides by that base in
	// Lua with a citation). Go has no creature MOVE_WALK/SWIM/FLIGHT
	// model — the Unit::UpdateSpeed twin (creatureRunSpeedRate) is
	// MOVE_RUN-only — so other move types are rejected. rate <= 0 clears
	// the script override, restoring the aura-fold/template rate (the C++
	// Reset arms that restore _baseSpeed/DefaultMoveSpeedRate/SPEED_RUN).
	// The normal refresh path recomputes RunSpeed, rescales in-flight
	// legs (PropagateSpeedChange) and broadcasts
	// SMSG_FORCE_RUN_SPEED_CHANGE to the instance.
	methods["SetSpeedRate"] = func(_ context.Context, args []any) ([]any, error) {
		mt, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if mt != 1 {
			return nil, fmt.Errorf("SetSpeedRate supports MOVE_RUN=1 only, got %d", mt)
		}
		rate, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		if rate <= 0 {
			motion.ScriptRunRate = 0
			motion.ScriptRunRateKnown = false
		} else {
			motion.ScriptRunRate = rate
			motion.ScriptRunRateKnown = true
		}
		s.motionMu.Unlock()
		s.refreshCreatureRunSpeed(creatureAuraKeyForMotion(motion), motion.GUID)
		return nil, nil
	}
	// SetScale is Creature::SetObjectScale (Creature.cpp:3199): sets
	// OBJECT_FIELD_SCALE_X directly. The script scale replaces the template
	// base in the RecalculateObjectScale twin (creatures.go) while known; a
	// non-positive scale clears the override (the C++ Reset/JustDied arms
	// restoring the template/default scale — solarian, sironas). The live
	// value broadcasts as a values-update; applyCreatureMotionToSpawn
	// overlays it onto the spawn record so rebuilds keep it.
	// Documented deviation: the C++ arm also re-derives
	// UNIT_FIELD_BOUNDINGRADIUS / UNIT_FIELD_COMBATREACH from the display's
	// model info; the motion carries no per-display model info, so melee
	// reach keeps the template value (solarian precedent: visual-only).
	methods["SetScale"] = func(_ context.Context, args []any) ([]any, error) {
		scale, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
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
		s.motionMu.Unlock()
		effective := float64(base) + float64(s.creatureAuraScaleSum(key))/100
		if effective < 0.01 {
			effective = 0.01
		}
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{objectFieldScale: math.Float32bits(float32(effective))})
		return nil, nil
	}
	// GetScale is Unit::GetObjectScale: the live object-scale base — the
	// script override while known, else the template scale (what the C++
	// solarian AI captures as defaultsize at construction).
	methods["GetScale"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		if motion.ScriptScaleKnown {
			return float64(motion.ScriptScale)
		}
		if motion.BaseScale > 0 {
			return float64(motion.BaseScale)
		}
		return 1.0
	})
	// SetVisible is Unit::SetVisible (Unit.cpp:8656): toggles the
	// m_serverSideVisibility SERVERSIDE_VISIBILITY_GM arm — false hides the
	// creature from everyone except GM seers (SEC_GAMEMASTER), true restores
	// normal visibility (SEC_PLAYER), followed by UpdateObjectVisibility.
	// The script override lives on motion.ScriptHidden; the nearby-creature
	// update filter (creatures.go) skips hidden motions for observers whose
	// gmVisibilityDetect is SEC_PLAYER, mirroring
	// WorldObject::CanSeeOrDetect (Object.cpp:1622-1630). Hiding broadcasts
	// SMSG_DESTROY_OBJECT to the non-GM sessions in the instance (the
	// Despawn precedent); unhiding re-creates the object via the periodic
	// nearby-creature update, the same path that re-creates respawned
	// creatures. The AI keeps running while hidden (C++ does not touch the
	// motion master or threat list).
	methods["SetVisible"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetVisible requires a bool, got no args")
		}
		visible, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetVisible requires a bool")
		}
		s.motionMu.Lock()
		motion.ScriptHidden = !visible
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		if !visible {
			destroyBuf := protocol.NewBuffer(9)
			destroyBuf.WriteU64(guid)
			destroyBuf.WriteU8(0)
			payload := destroyBuf.Bytes()
			opcode := uint16(protocol.OpcodeSMSG_DESTROY_OBJECT)
			s.sessionsMu.RLock()
			for target := range s.sessions {
				if !target.authed || !target.worldReady.Load() || target.player == nil ||
					target.player.Map != mapID || target.player.InstanceID != instanceID ||
					target.gmVisibilityDetect != 0 {
					continue
				}
				_ = target.write(opcode, payload, true)
			}
			s.sessionsMu.RUnlock()
		}
		return nil, nil
	}
	// SetFarVisible is WorldObject::SetFarVisible (Object.cpp:1049-1055):
	// sets the m_isFarVisible flag (no-op for players in C++; this surface
	// only ever receives creatures). The flag feeds the GetVisibilityRange
	// twin in the nearby-creature update filter (creatures.go): a
	// far-visible creature is visible out to MAX_VISIBILITY_DISTANCE
	// (Object.cpp:1515-1522, SIZE_OF_GRIDS) instead of the map default.
	// C++ sets only the flag — the visibility change takes effect on the
	// next visibility update, so no broadcast here (the periodic
	// nearby-creature rebuild picks it up). Backs the farvisible hunt
	// (hidden_files/farvisible_hunt.py, 2026-10-23).
	methods["SetFarVisible"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetFarVisible requires a bool, got no args")
		}
		farVisible, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetFarVisible requires a bool")
		}
		s.motionMu.Lock()
		motion.FarVisible = farVisible
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetTarget is Creature::SetTarget (Creature.cpp:3221-3226): the visual
	// UNIT_FIELD_TARGET (the GUID shown above the creature's head). It is
	// NOT the combat victim (motion.TargetGUID, driven by the threat
	// manager) — the C++ arms this backs are the boss "stare" effects
	// (malchezaar enfeeble, cthun eye beam, northrend_beasts icehowl stare,
	// kaelthas mind-control stares) plus the Reset/JustDied clears that
	// restore the empty target. Accepts a GUID number or a creature/player
	// object (luaObjectGUID); nil/0 clears, the ObjectGuid::Empty twin. The
	// live value broadcasts as a values-update; applyCreatureMotionToSpawn
	// overlays it onto the spawn record so rebuilds keep it. Backs the
	// target-gap hunt (hidden_files/target_gap_hunt.py, 2026-10-20).
	methods["SetTarget"] = func(_ context.Context, args []any) ([]any, error) {
		var guid uint64
		if len(args) > 0 && args[0] != nil {
			guid = luaObjectGUID(args[0])
		}
		s.motionMu.Lock()
		motion.ScriptTargetGUID = guid
		motion.ScriptTargetKnown = true
		mapID, instanceID, motionGUID := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, motionGUID,
			map[int]uint32{unitFieldTarget: uint32(guid), unitFieldTarget + 1: uint32(guid >> 32)})
		return nil, nil
	}
	// GetTarget is Unit::GetTarget: the live visual target GUID — the script
	// override while known, else 0 (the C++ field defaults to empty until a
	// script sets it; the combat victim is NOT reflected here).
	methods["GetTarget"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.ScriptTargetGUID
	})
	// ApplySpellImmune is Unit::ApplySpellImmune (Unit.cpp:8268): with
	// apply=true inserts (type, spellId) into m_spellImmune[op], with
	// apply=false removes the (type, spellId) rows. The spellId is only a
	// removal key — the C++ immunity evals never consult it — so the Go
	// model keeps it per row but the evals fold on (op, type) presence,
	// exactly like Unit::IsImmunedToSpell[Effect] (Unit.cpp:7852-7994).
	// Ops (SharedDefines.h:1426): 0 EFFECT (SpellEffects), 1 STATE
	// (AuraType), 2 SCHOOL (SpellSchoolMask), 3 DAMAGE (SpellSchoolMask),
	// 4 DISPEL (DispelType), 5 MECHANIC (Mechanics), 6 ID (spell id).
	// The rows are consulted by the creature-target immunity chain in
	// immunity.go at their C++ positions; immunity is server-side logic,
	// so there is no values-update broadcast.
	methods["ApplySpellImmune"] = func(_ context.Context, args []any) ([]any, error) {
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		op, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if op > 6 {
			return nil, fmt.Errorf("ApplySpellImmune supports ops 0-6, got %d", op)
		}
		immType, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		apply := false
		if len(args) >= 4 {
			if b, ok := args[3].(bool); ok {
				apply = b
			}
		}
		key := uint64(op)<<32 | uint64(immType)
		s.motionMu.Lock()
		if apply {
			if motion.ScriptSpellImmune == nil {
				motion.ScriptSpellImmune = map[uint64][]uint32{}
			}
			for _, id := range motion.ScriptSpellImmune[key] {
				if id == spellID {
					s.motionMu.Unlock()
					return nil, nil
				}
			}
			motion.ScriptSpellImmune[key] = append(motion.ScriptSpellImmune[key], spellID)
		} else if motion.ScriptSpellImmune != nil {
			ids := motion.ScriptSpellImmune[key]
			kept := ids[:0]
			for _, id := range ids {
				if id != spellID {
					kept = append(kept, id)
				}
			}
			if len(kept) == 0 {
				delete(motion.ScriptSpellImmune, key)
			} else {
				motion.ScriptSpellImmune[key] = kept
			}
		}
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
	// HasFlag is Unit::HasFlag (Unit.h: GetUInt32Value(field) & flag) for
	// the flag-backed fields the SetFlag/RemoveFlag bridges honor:
	// UNIT_FIELD_FLAGS (59) reads the live motion.UnitFlags mirror (init
	// from template at spawn; SetFlag/RemoveFlag/Mount/Dismount write
	// it), UNIT_NPC_FLAGS (82) reads motion.NPCFlags, UNIT_DYNAMIC_FLAGS
	// (79) reads motion.DynamicFlags. Other fields return false.
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flag, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		var has bool
		switch field {
		case 59:
			has = motion.UnitFlags&flag != 0
		case unitFieldNPCFlags:
			has = motion.NPCFlags&flag != 0
		case unitFieldDynamicFlags:
			has = motion.DynamicFlags&flag != 0
		}
		s.motionMu.Unlock()
		return []any{has}, nil
	}
	// IsWithinLOS is WorldObject::IsWithinLOSInMap (Object.cpp:1272-1284):
	// IsInMap(target) (same map AND instance — the C++ compares Map
	// pointers) && the eye-to-target segment clears terrain via the
	// default (aggro/assist/melee) LOS form — hasLineOfSight, i.e.
	// hasLineOfSightFlags(..., ignoreM2=false) (the M2-ignore form is
	// spell-CheckCast-only, Spell.cpp:5394). C++ adds the target's
	// collision height and shoots from the collision-height eye point
	// (players) or the hit-sphere point (creatures); the bridge uses
	// raw motion positions on both ends, the same shape as every other
	// hasLineOfSight caller in the tree (chain/area/pet/item legs).
	// The target may be a creature/player object or a raw GUID; an
	// unknown or non-live target is false (C++ IsInMap fails).
	methods["IsWithinLOS"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("IsWithinLOS requires a target")
		}
		guid := luaObjectGUID(args[0])
		if guid == 0 {
			return []any{false}, nil
		}
		s.motionMu.Lock()
		mx, my, mz := motion.X, motion.Y, motion.Z
		mmap, miid := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		var tx, ty, tz float32
		var tmap, tiid uint32
		found := false
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
			tx, ty, tz = sess.player.X, sess.player.Y, sess.player.Z
			tmap, tiid = sess.player.Map, sess.player.InstanceID
			found = true
		} else {
			s.motionMu.Lock()
			if m := s.findCreatureMotionLocked(mmap, miid, guid); m != nil {
				tx, ty, tz = m.X, m.Y, m.Z
				tmap, tiid = m.Map, m.InstanceID
				found = true
			}
			s.motionMu.Unlock()
		}
		if !found || tmap != mmap || tiid != miid {
			return []any{false}, nil
		}
		return []any{s.hasLineOfSight(mmap, mx, my, mz, tx, ty, tz)}, nil
	}
	// unitFieldFlagsValue is UNIT_FIELD_FLAGS = OBJECT_END(0x6)+0x35
	// (UpdateFields.h:117). SetFlag/RemoveFlag on it mutate motion.UnitFlags,
	// which the engine reads in help-call (creaturemotion.go:1089) and
	// chain/area target filtering — intro NOT_SELECTABLE/NON_ATTACKABLE arms
	// port with real targeting effect. UNIT_NPC_FLAGS (field 82) is honored
	// too (see below); UNIT_DYNAMIC_FLAGS (field 79) is honored too (see
	// below); other update fields have no creature backing here and are
	// ignored.
	methods["SetFlag"] = func(_ context.Context, args []any) ([]any, error) {
		field, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		flags, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var npcFlags uint32
		npcDirty := false
		var unitFlags uint32
		unitFlagsDirty := false
		var dynFlags uint32
		dynDirty := false
		var mapID, instanceID uint32
		var guid uint64
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags |= flags
			unitFlags, unitFlagsDirty = motion.UnitFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		} else if field == unitFieldNPCFlags {
			// UNIT_NPC_FLAGS (UpdateFields.h:139: OBJECT_END+0x4C = 82), the
			// live value behind the gossip/quest/vendor service flags that
			// the server-side hello gates (gossip.go, quest_details.go,
			// taxi.go) and the client service icons read. C++ SetFlag ORs
			// the bits; the values-update broadcast twin mirrors the
			// object-surface SetNPCFlags so clients see the change without
			// waiting for the next spawn rebuild.
			s.motionMu.Lock()
			motion.NPCFlags |= flags
			motion.NPCFlagsKnown = true
			npcFlags, npcDirty = motion.NPCFlags, true
			s.motionMu.Unlock()
		} else if field == unitFieldDynamicFlags {
			// UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79),
			// the live value behind the lootable/dead/tapped bits the
			// client corpse/loot gates read (motion.DynamicFlags is the
			// live slot — spawn rebuilds sync from it, so no Known flag
			// is needed). C++ SetFlag ORs the bits; the values-update
			// broadcast twin mirrors the 59/82 arms. Backs script
			// lootable/dead legs such as twin_valkyr JustDied and the
			// opera Romulo/Julianne cross-kill.
			s.motionMu.Lock()
			motion.DynamicFlags |= flags
			dynFlags, dynDirty = motion.DynamicFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		}
		if npcDirty {
			s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID,
				map[int]uint32{unitFieldNPCFlags: npcFlags})
		}
		if unitFlagsDirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldFlags: unitFlags})
		}
		if dynDirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldDynamicFlags: dynFlags})
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
		var npcFlags uint32
		npcDirty := false
		var unitFlags uint32
		unitFlagsDirty := false
		var dynFlags uint32
		dynDirty := false
		var mapID, instanceID uint32
		var guid uint64
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags &^= flags
			unitFlags, unitFlagsDirty = motion.UnitFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		} else if field == unitFieldNPCFlags {
			// UNIT_NPC_FLAGS (see SetFlag above): C++ RemoveFlag ANDNs the bits.
			s.motionMu.Lock()
			motion.NPCFlags &^= flags
			motion.NPCFlagsKnown = true
			npcFlags, npcDirty = motion.NPCFlags, true
			s.motionMu.Unlock()
		} else if field == unitFieldDynamicFlags {
			// UNIT_DYNAMIC_FLAGS (see SetFlag above): C++ RemoveFlag ANDNs
			// the bits. Backs the twin_valkyr sister-alive un-lootable leg.
			s.motionMu.Lock()
			motion.DynamicFlags &^= flags
			dynFlags, dynDirty = motion.DynamicFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		}
		if npcDirty {
			s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID,
				map[int]uint32{unitFieldNPCFlags: npcFlags})
		}
		if unitFlagsDirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldFlags: unitFlags})
		}
		if dynDirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldDynamicFlags: dynFlags})
		}
		return nil, nil
	}
	// SetNPCFlags is CreatureMethods::SetNPCFlags (CreatureMethods.h:909-915):
	// SetUInt32Value(UNIT_NPC_FLAGS, flags) — whole-value assignment, the
	// SetUInt32Value twin of the granular SetFlag/RemoveFlag(82) arms above.
	// The object surface (lua_objects.go) carries the same method for
	// session-bound creatures, but the event surface keeps a separate method
	// table, so it needs its own copy: spark_overgrind's Reset was calling a
	// nil method here before this bridge existed.
	methods["SetNPCFlags"] = func(_ context.Context, args []any) ([]any, error) {
		flags, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.NPCFlags = flags
		motion.NPCFlagsKnown = true
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID,
			map[int]uint32{unitFieldNPCFlags: flags})
		return nil, nil
	}
	// GetNPCFlags is CreatureMethods::GetNPCFlags
	// (CreatureMethods.h:861-865): GetUInt32Value(UNIT_NPC_FLAGS), read from
	// the motion's live value (initialized from the template npcflag at
	// spawn, mutated by the SetNPCFlags/SetFlag(82)/RemoveFlag(82) bridges).
	methods["GetNPCFlags"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.NPCFlags
	})
	// SetFaction is Unit::SetFaction: SetUInt32Value(UNIT_FIELD_FACTIONTEMPLATE,
	// factionId) (Object.cpp) — whole-value assignment, no range validation in
	// C++. The Go twin writes the motion record's live Faction slot and
	// broadcasts UNIT_FIELD_FACTIONTEMPLATE, exactly the charmCreatureFaction
	// pattern (creaturemotion.go:1794/1828): the faction slot feeds
	// isAttackableFaction / isHostileFaction at aggro and targeting time, so
	// faction-flip arms (nefarius throne/event, spark_overgrind gossip,
	// zum_rah areatrigger restore, ...) take real effect instead of being
	// header-documented as unbridgeable. Backs the faction-gap hunt
	// (hidden_files/faction_gap_hunt.py, 2026-10-20).
	methods["SetFaction"] = func(_ context.Context, args []any) ([]any, error) {
		faction, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.Faction = faction
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldFaction: faction})
		return nil, nil
	}
	// GetFaction is UnitMethods::GetFaction (CreatureMethods.h): reads the
	// creature's live faction-template id (the motion slot above, initialized
	// from the template faction at spawn).
	methods["GetFaction"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.Faction
	})
	// FindNearestCreature(entry, dist) is WorldObject::FindNearestCreature /
	// GetClosestCreatureWithEntry (ScriptedCreature.h): the nearest live
	// creature of entry within dist yards (3D gate with both combat reaches
	// added — the C++ IsWithinDistInMap metric; winner picked by
	// reach-adjusted distance, the C++ GetDistance i_range narrowing) in
	// the same map+instance, excluding
	// self; nil when none. Backs the creature-search arms (flame-tsunami
	// lava-blaze buff) that headers documented as bridge-blocked.
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
		srcReach := float64(motion.combatReachOrDefault())
		s.motionMu.Lock()
		var best *creatureMotion
		bestAdj := dist
		for guid, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if guid == motion.GUID || m.Entry != entry || m.Health == 0 {
				continue
			}
			dx := float64(m.X - motion.X)
			dy := float64(m.Y - motion.Y)
			dz := float64(m.Z - motion.Z)
			candReach := float64(m.combatReachOrDefault())
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist+srcReach+candReach {
				// C++ narrows i_range to GetDistance (3D minus both combat
				// reaches, floored at 0, Object.cpp:1169) — the winner is
				// the reach-adjusted nearest, not the raw-3D nearest.
				adj := d - srcReach - candReach
				if adj < 0 {
					adj = 0
				}
				if best == nil || adj < bestAdj {
					best, bestAdj = m, adj
				}
			}
		}
		s.motionMu.Unlock()
		if best == nil {
			return []any{nil}, nil
		}
		return []any{s.luaMotionCreature(best)}, nil
	}
	// FindNearestGameObject(entry, dist) is
	// WorldObject::FindNearestGameObject (Object.cpp): the nearest
	// gameobject of entry within dist yards (3D metric) in the same
	// map+instance; nil when none. DB-spawn model only (see
	// nearestGameObjectByEntry). Backs the tallhorn-stag haunch arm and the
	// 8 HEADER-ONLY FindNearestGameObject sites the 2026-10-23 triage queued
	// for re-key.
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
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		x, y, z := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		return []any{s.nearestGameObjectByEntry(ctx, mapID, instanceID, x, y, z, entry, dist32)}, nil
	}
	// GetCreaturesInGrid(entry, dist) is
	// WorldObject::GetCreatureListWithEntryInGrid: every creature of entry
	// within dist yards (2D, both combat reaches added — the C++
	// AllCreaturesOfEntryInRange metric is IsWithinDist(unit, range,
	// is3D=false), GridNotifiers.h) in the same map+instance, including
	// self (the C++ checker has no self exclusion), as a Lua array of
	// creature objects (GUID-sorted for determinism; the C++ grid-visit
	// order is unspecified). Backs mass-despawn arms such as anubarak's
	// frost-sphere/burrower cleanup.
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
		srcReach := float64(motion.combatReachOrDefault())
		s.motionMu.Lock()
		var matches []*creatureMotion
		for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if m.Entry != entry {
				continue
			}
			dx := float64(m.X - motion.X)
			dy := float64(m.Y - motion.Y)
			if d := math.Sqrt(dx*dx + dy*dy); d <= dist+srcReach+float64(m.combatReachOrDefault()) {
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
	// GetHomePosition is CreatureMethods::GetHomePosition
	// (CreatureMethods.h:630-644): x, y, z, o from the creature's HOME
	// position (Creature.h:296-297: m_homePosition) — twin of the
	// session-surface bridge (lua_objects.go) for event handlers, whose
	// wrappers close over the motion directly. The 4th value is the home
	// orientation, NOT the live facing.
	methods["GetHomePosition"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		hx, hy, hz, ho := motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO
		s.motionMu.Unlock()
		return []any{hx, hy, hz, ho}, nil
	}
	// GetCombatReach is Creature::GetCombatReach (Unit.h): the creature's
	// own combat reach (creature_model_info CombatReach, 1.5 default via
	// combatReachOrDefault), so Lua ports can twin C++ WorldObject::GetDistance
	// semantics (Object.cpp:1175-1179: GetExactDist - reach, clamped >= 0).
	methods["GetCombatReach"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		reach := motion.combatReachOrDefault()
		s.motionMu.Unlock()
		return []any{float64(reach)}, nil
	}
	// SetHomePosition is CreatureMethods::SetHomePosition
	// (CreatureMethods.h:655-669): Creature::SetHomePosition(x, y, z, o)
	// (Creature.h:294) relocates m_homePosition ONLY — the live
	// position/orientation is untouched, so the bridge must not write
	// motion.Orientation here (the old write snapped the live facing,
	// e.g. sartharion's Reset to 4.66003).
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
		motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO = x, y, z, o
		s.motionMu.Unlock()
		return nil, nil
	}
	// UpdateWorldState(fieldId, value) bridges InstanceScript::DoUpdateWorldState
	// (InstanceScript.cpp:555-567): it pushes SMSG_UPDATE_WORLD_STATE to every
	// player in the creature's instance. It does NOT touch the global persisted
	// worldstates map (worldstates.go) — that is World::setWorldState, a
	// different, server-wide store.
	methods["UpdateWorldState"] = func(_ context.Context, args []any) ([]any, error) {
		fieldID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		value, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		s.updateInstanceWorldState(mapID, instanceID, fieldID, value)
		return nil, nil
	}
	motionPowerType := motion.PowerType
	if motionPowerType >= uint32(len(motion.Powers)) {
		motionPowerType = 0
	}
	return &scripting.Object{Type: "Creature", Fields: map[string]any{
		"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry,
		"Map": state.Map, "MapId": state.Map,
		"X": state.X, "Y": state.Y, "Z": state.Z, "Orientation": motion.Orientation,
		"Health": state.Health, "MaxHealth": state.MaxHealth, "Level": state.Level,
		"Power": motion.Powers[motionPowerType], "MaxPower": motion.MaxPowers[motionPowerType],
		"PowerType":  motionPowerType,
		"InstanceId": motion.InstanceID,
		"InWorld":    true,
	}, Methods: methods}
}

// seatCreatureEntry is the shared core of the Lua creature:UpdateEntry
// bridge and the evade-respawn re-seat: Creature::UpdateEntry
// (Creature.cpp:552-655) re-seats the creature on a new
// creature_template entry. The motion-map key and motion.GUID keep the
// ORIGINAL entry embedded (C++ fixes the GUID at Object::Create;
// Creature::InitEntry only SetEntry-changes, Creature.cpp:486), so the
// caller must hold no locks; the seating takes motionMu itself. Returns
// false when the template is missing or defines no model (the C++
// InitEntry false legs).
func (s *Server) seatCreatureEntry(ctx context.Context, motion *creatureMotion, newEntry uint32) bool {
	if s == nil || motion == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return false
	}
	var model1, model2, model3, model4 int64
	var faction, npcFlag, unitFlags, unitFlags2, dynamicFlags int64
	var minLevel, maxLevel int64
	var templateScale, speedWalk, speedRun float64
	var baseAttackTime, regenHealth int64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT
		COALESCE(modelid1, 0), COALESCE(modelid2, 0), COALESCE(modelid3, 0), COALESCE(modelid4, 0),
		COALESCE(faction, 0), COALESCE(npcflag, 0), COALESCE(unit_flags, 0), COALESCE(unit_flags2, 0),
		COALESCE(dynamicflags, 0), COALESCE(minlevel, 1), COALESCE(maxlevel, 1), COALESCE(scale, 1.0),
		COALESCE(speed_walk, 0), COALESCE(speed_run, 0), COALESCE(BaseAttackTime, 2000),
		COALESCE(RegenHealth, 0)
		FROM creature_template WHERE entry = ?`, newEntry).Scan(
		&model1, &model2, &model3, &model4,
		&faction, &npcFlag, &unitFlags, &unitFlags2,
		&dynamicFlags, &minLevel, &maxLevel, &templateScale,
		&speedWalk, &speedRun, &baseAttackTime, &regenHealth)
	if err != nil {
		return false
	}
	// Creature::GetFirstValidModelId: first nonzero of modelid1..4.
	var model uint32
	for _, m := range []int64{model1, model2, model3, model4} {
		if m > 0 {
			model = uint32(m)
			break
		}
	}
	if model == 0 {
		return false
	}
	stats := s.loadCreatureStats(ctx, newEntry)
	// Creature::SelectLevel (Creature.cpp:1489-1498): urand(min, max).
	minL, maxL := minLevel, maxLevel
	if minL > maxL {
		minL, maxL = maxL, minL
	}
	if minL < 1 {
		minL = 1
	}
	level := uint32(minL)
	if maxL > minL {
		level = uint32(minL) + uint32(rand.Intn(int(maxL-minL)+1))
	}
	// CreatureModelInfo radius/reach for the new display (the SetDisplayId
	// twin's Creature.cpp:3210-3218 fold).
	var boundingRadius, combatReach float64
	var br, cr sql.NullFloat64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT BoundingRadius, CombatReach FROM creature_model_info WHERE DisplayID = ?", model).Scan(&br, &cr); err == nil {
		if br.Valid && br.Float64 > 0 {
			boundingRadius = br.Float64
		}
		if cr.Valid && cr.Float64 > 0 {
			combatReach = cr.Float64
		}
	}
	// Creature::UpdateEntry sheathes melee when no addon row exists
	// (Creature.cpp:562-563).
	lowGUID := uint32(motion.GUID & 0x00FFFFFF)
	var addonProbe int64
	hasAddon := s.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature_addon WHERE guid = ? LIMIT 1", lowGUID).Scan(&addonProbe) == nil
	reactState, reactKnown, _ := s.loadCreatureReaction(ctx, newEntry)
	// Default equipment for the new entry (C++ InitEntry with data=nil
	// calls LoadEquipment()).
	var item1, item2, item3 sql.NullInt64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT ItemID1, ItemID2, ItemID3 FROM creature_equip_template WHERE CreatureID = ? AND ID = 1", newEntry).Scan(&item1, &item2, &item3)

	fields := make(map[int]uint32)
	s.motionMu.Lock()
	prevHealth := motion.Health
	motion.Entry = newEntry
	motion.EntryOverride = true
	motion.DisplayID = model
	motion.DisplayIDKnown = true
	motion.Faction = uint32(faction)
	// Creature.cpp:576-578: the template flags apply, but IN_COMBAT is
	// kept while the creature is engaged.
	flags := uint32(unitFlags) &^ unitFlagInCombat
	if motion.InCombat {
		flags |= unitFlagInCombat
	}
	motion.UnitFlags = flags
	motion.DynamicFlags = uint32(dynamicFlags)
	motion.NPCFlags = uint32(npcFlag)
	motion.NPCFlagsKnown = true
	attackTime := uint32(baseAttackTime)
	if attackTime == 0 {
		attackTime = 2000
	}
	motion.AttackTime = attackTime
	motion.Level = level
	// UpdateLevelDependantStats + the previous-health preservation
	// (Creature.cpp:597-600): the new max applies, the current health
	// carries over clamped (a dead creature stays dead).
	motion.MaxHealth = stats.MaxHealth
	if prevHealth > 0 {
		if prevHealth > stats.MaxHealth {
			prevHealth = stats.MaxHealth
		}
		motion.Health = prevHealth
	} else {
		motion.Health = 0
	}
	motion.Armor = stats.Armor
	motion.Resistances = stats.Resistances
	motion.MinDamage = stats.MinDamage
	motion.MaxDamage = stats.MaxDamage
	// SetObjectScale(GetNativeObjectScale()) (Creature.cpp:3195-3198 via
	// InitEntry): the template scale replaces any script override.
	motion.BaseScale = stats.Scale
	motion.ScriptScaleKnown = false
	motion.RegenHealthDisabled = regenHealth == 0
	motion.Speed = creatureWalkVelocity(speedWalk)
	motion.BaseRunRate = float32(speedRun)
	if !hasAddon {
		motion.SheathState = 1 // SHEATH_STATE_MELEE (UnitDefines.h:71-77)
		motion.SheathStateKnown = true
	}
	if reactKnown {
		motion.ReactState = reactState
		motion.ReactStateKnown = true
	}
	if combatReach > 0 {
		scale := motion.BaseScale
		if scale <= 0 {
			scale = 1
		}
		motion.CombatReach = float32(combatReach) * scale
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	newHealth := motion.Health
	newCombatReach := motion.CombatReach
	auraKey := creatureAuraKeyForMotion(motion)
	s.motionMu.Unlock()

	// Unit::UpdateSpeed twin: the new template run rate feeds the aura
	// fold (the SetSpeedRate twin's refresh path).
	s.refreshCreatureRunSpeed(auraKey, guid)

	fields[objectFieldEntry] = newEntry
	fields[unitFieldDisplayID] = model
	fields[unitFieldNativeDisplayID] = model
	fields[unitFieldFaction] = uint32(faction)
	fields[unitFieldFlags] = flags
	fields[unitFieldFlags2] = uint32(unitFlags2)
	fields[unitFieldDynamicFlags] = uint32(dynamicFlags)
	fields[unitFieldNPCFlags] = uint32(npcFlag)
	fields[unitFieldLevel] = level
	fields[unitFieldHealth] = newHealth
	fields[unitFieldMaxHealth] = stats.MaxHealth
	fields[unitFieldAttackTime] = attackTime
	if boundingRadius > 0 {
		scale := float64(stats.Scale)
		if scale <= 0 {
			scale = 1
		}
		fields[unitFieldBoundingRadius] = math.Float32bits(float32(boundingRadius * scale))
	}
	if combatReach > 0 {
		fields[unitFieldCombatReach] = math.Float32bits(newCombatReach)
	}
	// OBJECT_FIELD_SCALE_X with the aura fold (the SetScale twin's
	// effective-scale formula).
	scaleBase := float64(stats.Scale)
	if scaleBase <= 0 {
		scaleBase = 1
	}
	effective := scaleBase + float64(s.creatureAuraScaleSum(auraKey))/100
	if effective < 0.01 {
		effective = 0.01
	}
	fields[objectFieldScale] = math.Float32bits(float32(effective))
	if item1.Valid && item1.Int64 > 0 {
		fields[unitVirtualItemSlotID] = uint32(item1.Int64)
	}
	if item2.Valid && item2.Int64 > 0 {
		fields[unitVirtualItemSlotID+1] = uint32(item2.Int64)
	}
	if item3.Valid && item3.Int64 > 0 {
		fields[unitVirtualItemSlotID+2] = uint32(item3.Int64)
	}
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, fields)
	return true
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

// fireCreatureHealReceived dispatches Go-side CREATURE_EVENT_ON_HEAL_RECEIVED
// (11) for the creature ahead of the health gain, mirroring the AI-hook arm
// of Unit::DealHeal (Unit.cpp:6299-6304): the victim AI's HealReceived fires
// on the pre-clamp addhealth. C++ Eluna has no heal hook, so this event is a
// Go extension, not a C++ Eluna event; it is read-only — handler returns are
// ignored, unlike DamageTaken's rewrite, because no Lua consumer adjusts heals
// and C++ HealReceived's heal-adjustment capability has no Eluna twin to
// mirror. Args are (event, creature, healer, heal); the healer is the healer
// creature object, or nil when the healer is a player (no player object is
// available at the fire site). Fires only for entry handlers; unique-handler
// dispatch is unnecessary here because no per-GUID heal consumer exists.
func (s *Server) fireCreatureHealReceived(ctx context.Context, victim, healer *creatureMotion, heal uint32) {
	if s == nil || victim == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	var healerObj any
	if healer != nil {
		healerObj = s.luaMotionCreature(healer)
	}
	s.fireCreatureLuaEvent(ctx, victim, scripting.CreatureEventOnHealReceived, healerObj, heal)
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
	// CREATURE_EVENT_ON_SUMMONED_CREATURE_DIED (21) dispatches on the
	// summoner when a temp summon dies. C++ (Eluna::SummonedCreatureDied,
	// CreatureHooks.cpp, reached from ElunaCreatureAI::SummonedCreatureDies,
	// ElunaCreatureAI.h) pushes (event, creature, summon, killer) over the
	// entry then unique bindings of the summoner. The summoner link is the
	// summon's OwnerGUID (stamped by summonTempCreature); pets and charmed
	// creatures point at player GUIDs, which findCreatureMotion never
	// resolves, so those skip cleanly. The boolean veto gates only
	// ScriptedAI::SummonedCreatureDies, whose base is empty (CreatureAI.h)
	// — a provable no-op like the JustDied/JustSummoned rulings — and is
	// discarded. Must be called with motionMu released: findCreatureMotion
	// locks it and the Lua handlers lock it on demand.
	if motion.OwnerGUID != 0 {
		if summoner := sv.findCreatureMotion(motion.Map, motion.InstanceID, motion.OwnerGUID); summoner != nil {
			if summonerObj := sv.luaMotionCreature(summoner); summonerObj != nil {
				summonDiedArgs := []any{scripting.CreatureEventOnSummonedCreatureDied, summonerObj, creature, s.luaPlayer()}
				_, _ = sv.Features.Scripts.TriggerCreatureEvent(ctx, summoner.Entry, scripting.CreatureEventOnSummonedCreatureDied, summonDiedArgs...)
				_, _ = sv.Features.Scripts.TriggerUniqueCreatureEvent(ctx, summoner.GUID, summoner.InstanceID, scripting.CreatureEventOnSummonedCreatureDied, summonDiedArgs...)
			}
		}
	}
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
