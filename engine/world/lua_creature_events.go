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
// MANUAL_DESPAWN); despawnMs arms the timer for the timed family (0-3)
// and stamps the corpse timer's duration for CORPSE_TIMED_DESPAWN (5). The all-zero x/y/z close-point and o==0 orientation fallbacks,
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
	// Phase, when HasPhase, overrides the summoner/session phase below:
	// PerformIngameSpawn carries an explicit phase argument, while
	// origin-based spawns keep the summoner phase.
	Phase    uint32
	HasPhase bool
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
	summoned.CreatedByGUID = ownerGUID // TempSummon::InitStats (TemporarySummon.cpp:356): creator is the owner
	summoned.IsTempSummon = true
	summoned.TempSummonType = summonType
	// InitStats (TemporarySummon.cpp:168-183) stamps m_lifetime from
	// the summon duration for every type: the timed family (0/1/2/3)
	// arms the timer at spawn, CORPSE_TIMED_DESPAWN (5) carries the
	// duration for the corpse timer checkDeadTempSummon starts at
	// death, and CORPSE_DESPAWN (4) / DEAD_DESPAWN (6) / MANUAL (7)
	// carry no live timer.
	if despawnMs > 0 {
		summoned.TempSummonLifetime = time.Duration(despawnMs) * time.Millisecond
		if summonType <= 3 {
			summoned.TempSummonExpiresAt = time.Now().Add(summoned.TempSummonLifetime)
		}
	}
	stats := s.loadCreatureStats(ctx, entry)
	spawn := creatureSpawn{
		GUID: low, RawGUID: summoned.GUID, Entry: entry, Map: mapID,
		X: x, Y: y, Z: z, Orientation: o,
		Model: stats.Model, Faction: summoned.Faction, NPCFlags: stats.NPCFlags,
		UnitFlags: stats.UnitFlags, Level: stats.Level,
		Health: stats.Health, MaxHealth: stats.MaxHealth,
		Scale: stats.Scale, Mana: stats.Mana,
		FlagsExtra: stats.FlagsExtra, TypeFlags: stats.TypeFlags,
		WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed),
	}
	s.motionMu.Unlock()
	// Temp-summon phase (WorldObject::GetPhaseMaskForSpawn): a creature
	// summoner passes its own motion phase, a player summoner its
	// session phase; other summoners keep the PHASEMASK_NORMAL
	// fallback in the totem phase helper.
	if origin.SummonerMotion != nil {
		summoned.PhaseMask = origin.SummonerMotion.PhaseMask
	} else if summoner := s.findSessionByGUID(origin.GUID); summoner != nil {
		summoned.PhaseMask = summoner.currentPlayerPhaseMask()
	}
	if origin.HasPhase {
		summoned.PhaseMask = origin.Phase
	}
	spawn.PhaseMask = summoned.PhaseMask
	// Re-create the object client-side: the create block goes to the
	// creature's visibility set (WorldObject::BuildUpdate fans out "for
	// all visible players", Object.cpp:3620; C++ AddToWorld itself sends
	// no packet — Creature.cpp:283-304).
	s.applyCreatureMotionToSpawn(playerState{Map: mapID, InstanceID: instanceID}, &spawn)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, nil))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastCreatureCreateToVisibleSet(packet.Opcode, packet.Payload.Bytes(), mapID, instanceID, spawn.X, spawn.Y, spawn.Z, &spawn, nil)
	}
	if origin.SummonerMotion != nil {
		s.fireCreatureSummoned(ctx, origin.SummonerMotion, scripting.CreatureEventOnJustSummonedCreature, summoned)
		// Eluna::OnSummoned (CreatureHooks.cpp:86-94): the summoned
		// creature's own IsSummonedBy hook (event 22) fires on the summon
		// path with the summoner as the extra argument — C++ calls
		// IsSummonedBy on the new AI for every summon, so creature summons
		// (group summons included) reach ev22 here, not just player pets
		// via the pet funnel. The fireCreaturePetSummoned comment's
		// pet-only wording is stale as of this fire site.
		s.fireCreaturePetSummoned(ctx, summoned, s.luaMotionCreature(origin.SummonerMotion))
	} else if summoner := s.findSessionByGUID(origin.GUID); summoner != nil {
		if player := luaPlayerOrNil(summoner); player != nil {
			s.fireCreaturePetSummoned(ctx, summoned, player)
		}
	}
	return summoned
}

// spawnCreatureFromOrigin is the WorldObject::SpawnCreature
// (WorldObjectMethods.h) twin behind the SpawnCreature bridge on the
// creature, player, and gameobject surfaces: it shares summonTempCreature
// with SummonCreature but takes the Eluna SpawnCreature signature
// (entry, x, y, z, o, spawnType = 8, despawnTimer = 0). The Trinity branch
// of SpawnCreature accepts only types 1-8 and luaL_argerrors otherwise
// (the 9/10 OOC-or-corpse/dead types exist only in the MaNGOS branch), so
// types outside 1-8 error here too; the Eluna type translates to the
// bridge's 0-based scheme by subtracting 1 (8 MANUAL_DESPAWN -> 7). The
// despawn timer is seconds in C++ (Seconds(despawnTimer) at the
// SummonCreature call), while summonTempCreature takes milliseconds, so
// it is scaled by 1000 and capped at the uint32 millisecond ceiling.
func (s *Server) spawnCreatureFromOrigin(ctx context.Context, origin tempSummonOrigin, args []any) (*creatureMotion, error) {
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
	spawnType := uint32(8)
	if len(args) > 5 {
		if spawnType, err = luaUint32Arg(args, 5); err != nil {
			return nil, err
		}
		if spawnType < 1 || spawnType > 8 {
			return nil, fmt.Errorf("valid SpawnType expected")
		}
	}
	var despawnMs uint32
	if len(args) > 6 {
		despawnSecs, err := luaUint32Arg(args, 6)
		if err != nil {
			return nil, err
		}
		if despawnSecs > 4294967 {
			despawnMs = 4294967295
		} else {
			despawnMs = despawnSecs * 1000
		}
	}
	return s.summonTempCreature(ctx, origin, entry, x, y, z, o, uint8(spawnType-1), despawnMs), nil
}

// unitIsStandState is Unit::IsStandState (Unit.cpp:10551-10555) over
// Unit::IsSitState (Unit.cpp:10542-10549): true unless the stand
// state is a sit state (1, 2, 4, 5, 6), SLEEP (3), or KNEEL (8).
func unitIsStandState(st uint8) bool {
	switch st {
	case 1, 2, 3, 4, 5, 6, 8:
		return false
	default:
		return true
	}
}

// setMotionDisplayID is the shared Unit::SetDisplayId /
// Creature::SetDisplayId fold behind the handler SetDisplayId bridge
// and the DeMorph arms: UNIT_FIELD_DISPLAYID plus the
// CreatureModelInfo bounding-radius / combat-reach folds scaled by the
// live object scale, broadcast to the instance. The gender fold
// (Unit.cpp:10605-10606) has no server read path, so it is not
// modeled; the pet branch stays out, as on the bridge.
func (s *Server) setMotionDisplayID(ctx context.Context, motion *creatureMotion, id uint32) {
	if s == nil || motion == nil {
		return
	}
	fields := map[int]uint32{unitFieldDisplayID: id}
	var boundingRadius, combatReach float64
	if s.WorldStore != nil && s.WorldStore.DB != nil {
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
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, fields)
}

// luaCastSpellTypeArg parses a CurrentSpellTypes arg (UnitMethods.h:785):
// 0 MELEE, 1 GENERIC, 2 CHANNELED, 3 AUTOREPEAT; anything else errors,
// matching Eluna's luaL_argerror for type >= CURRENT_MAX_SPELL.
func luaCastSpellTypeArg(args []any, index int) (int, error) {
	v, err := luaUint32Arg(args, index)
	if err != nil {
		return 0, err
	}
	if v > 3 {
		return 0, fmt.Errorf("argument %d must be a valid CurrentSpellTypes", index+1)
	}
	return int(v), nil
}

// luaCastCustomBasePoints parses CastCustomSpell's bp0/bp1/bp2 args
// (args 3-5): only non-nil args override, matching CastSpellExtraArgs'
// AddSpellMod-per-supplied-point semantics; the sparse map leaves
// unlisted effects at their DBC values.
func luaCastCustomBasePoints(args []any) (map[int]int32, error) {
	var out map[int]int32
	for i := 0; i < 3; i++ {
		idx := 3 + i
		if len(args) <= idx || args[idx] == nil {
			continue
		}
		bp, err := luaInt32Arg(args, idx)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = map[int]int32{}
		}
		out[i] = bp
	}
	return out, nil
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
	// GetName is Unit::GetName (Object.h:338) over the live motion:
	// the generic fallback reads the construction-time Fields snapshot,
	// so a SetName below would be invisible to a later GetName on the
	// same object without this builder-owned read under motionMu.
	// DELTA: chat packets built from the snapshot state.Name (the
	// motionChatRange closure) and already-sent query responses keep
	// the old name; only GetName reads live.
	methods["GetName"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.Name}, nil
	}
	// SetName is Unit::SetName (UnitMethods.h:1501, Object.h:338): the
	// Eluna binding gates empty names (length > 0 only) and the C++
	// write is memory-only, no DB write, so this writes motion.Name
	// under motionMu and nothing else.
	methods["SetName"] = func(_ context.Context, args []any) ([]any, error) {
		name, err := luaStringArg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(name) == 0 {
			return nil, nil
		}
		s.motionMu.Lock()
		motion.Name = name
		s.motionMu.Unlock()
		return nil, nil
	}
	// IsArmorer/IsBanker/IsBattleMaster/IsGuildMaster/IsTabardDesigner
	// plus the gossip/quest/vendor/trainer/taxi/spirit/innkeeper/
	// auctioneer/service-provider set are the Unit.h:1006-1021
	// UNIT_NPC_FLAGS reads over the live motion flags, mirroring the
	// session surface's masks (IsTrainer 0x70 and IsServiceProvider
	// 0x007FC0F2 are supersets of the C++-exact 0x10 / 0x003FE090;
	// see the recensus detail note). GetClassMask is Unit::GetClassMask (Unit.h:852) over the
	// motion's template unit_class; GetRaceMask reads 0, the creature
	// race byte Creature::InitEntry never sets. IsPvPFlagged is
	// Unit::IsPvP (Unit.h:914) over the motion bytes2 byte 1, whose
	// PvP bit SetPvP sets and FFA bit SetFFA sets.
	npcFlag := func(flag uint32) scripting.ObjectMethod {
		return luaNoArgs(func() any {
			s.motionMu.Lock()
			defer s.motionMu.Unlock()
			return motion.NPCFlags&flag != 0
		})
	}
	methods["IsArmorer"] = npcFlag(0x00001000)
	methods["IsBanker"] = npcFlag(0x00020000)
	methods["IsBattleMaster"] = npcFlag(0x00100000)
	methods["IsGuildMaster"] = npcFlag(0x00040000)
	methods["IsTabardDesigner"] = npcFlag(0x00080000)
	methods["IsGossip"] = npcFlag(0x00000001)
	methods["IsQuestGiver"] = npcFlag(0x00000002)
	methods["IsVendor"] = npcFlag(0x00000080)
	methods["IsTrainer"] = npcFlag(0x00000070)
	methods["IsTaxi"] = npcFlag(0x00002000)
	methods["IsSpiritHealer"] = npcFlag(0x00004000)
	methods["IsSpiritGuide"] = npcFlag(0x00008000)
	methods["IsSpiritService"] = npcFlag(0x0000C000)
	methods["IsInnkeeper"] = npcFlag(0x00010000)
	methods["IsAuctioneer"] = npcFlag(0x00200000)
	methods["IsServiceProvider"] = npcFlag(0x007FC0F2)
	methods["GetClassMask"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return unitClassRaceMask(motion.UnitClass)
	})
	methods["GetRaceMask"] = luaNoArgs(func() any { return uint32(0) })
	// GetShapeshiftForm is Unit::GetShapeshiftForm (Unit.h): creatures
	// carry no shapeshift form in the Go model, so this reads FORM_NONE
	// (0, SpellAuraDefines.h:409) — the C++ default for a unit whose
	// form was never set. The player-surface twin reads the live state.
	methods["GetShapeshiftForm"] = luaNoArgs(func() any { return uint32(0) })
	methods["IsPvPFlagged"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.PVPFlags&pvpFlagPvP != 0
	})
	// HealthBelowPct/HealthAbovePct/CountPctFromCurHealth/
	// CountPctFromMaxHealth over the live motion under motionMu.
	// This surface previously had no health methods of its own and
	// relied on the generic fallback's construction-time Fields
	// snapshot, so a stored handler creature went stale.
	for name, fn := range unitHealthPctMethods(func() (uint32, uint32) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.Health, motion.MaxHealth
	}) {
		methods[name] = fn
	}
	// IsStopped is Unit::IsStopped (Unit.h:1557): !HasUnitState(
	// UNIT_STATE_MOVING), here the motion's spline-active Moving
	// flag (set at launch, cleared at arrival).
	methods["IsStopped"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{!motion.Moving}, nil
	}
	// IsMoving is Unit::isMoving (Unit.h:1641):
	// HasUnitMovementFlag(MOVEMENTFLAG_MASK_MOVING). The Go twin is
	// the live spline flight (motion.Moving && now before MoveEnds,
	// the formation.go:307 idiom); a stamp past MoveEnds is never
	// read (creaturemotion.go:187-188). DELTA: falling / knockback
	// have no creature model, so they read as not-moving; the
	// player surface already carries its own IsMoving
	// (lua_player.go:1732) from the session movement flags.
	methods["IsMoving"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.Moving && time.Now().Before(motion.MoveEnds)}, nil
	}
	// GetClassAsString/GetRaceAsString/GetControllerGUID/
	// GetControllerGUIDS mirror the session-creature twins in
	// lua_objects.go: ChrClasses Name[locale] for the motion's
	// template unit_class, nil race (the race byte is never set),
	// and the Unit.h:1170 / Object.cpp:2196 controller GUIDs over
	// the motion's charmer/owner, else the motion's own GUID.
	methods["GetClassAsString"] = func(_ context.Context, args []any) ([]any, error) {
		locale, err := dbcLocaleArg(args)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		class := motion.UnitClass
		s.motionMu.Unlock()
		name, err := s.dbcLocaleName("ChrClasses", class, 4, locale)
		if err != nil {
			return nil, err
		}
		return []any{name}, nil
	}
	methods["GetRaceAsString"] = luaNoArgs(func() any { return nil })
	methods["GetControllerGUID"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return unitControllerGUID(motion.Charmed, motion.CharmerGUID, motion.OwnerGUID)
	})
	methods["GetControllerGUIDS"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		if controller := unitControllerGUID(motion.Charmed, motion.CharmerGUID, motion.OwnerGUID); controller != 0 {
			return controller
		}
		return motion.GUID
	})
	// GetCharmerGUID is Unit::GetCharmerGUID (Unit.h:1158):
	// UNIT_FIELD_CHARMEDBY, the motion's charmer slot.
	methods["GetCharmerGUID"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.CharmerGUID
	})
	// GetCharmGUID is Unit::GetCharmedGUID (Unit.h:1161) via the
	// shared instance scan for a motion charmed by this creature.
	methods["GetCharmGUID"] = luaNoArgs(func() any {
		return s.unitCharmedGUID(motion.Map, motion.InstanceID, motion.GUID)
	})
	// GetCritterGUID is Unit::GetCritterGUID (Unit.h:1156): the
	// motion's critter slot, 0 unless a script set one.
	methods["GetCritterGUID"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.CritterGUID
	})
	// SetCritterGUID is Unit::SetCritterGUID (UnitMethods.h:1908):
	// the motion critter-slot write. No values broadcast: Go
	// carries no creature UNIT_FIELD_CRITTER leg.
	methods["SetCritterGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.motionMu.Lock()
		motion.CritterGUID = guid
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetMinionGUID is LuaUnit::GetPetGUID (LuaFunctions.cpp:283
	// registers GetMinionGUID to it, not to Unit::GetMinionGUID):
	// the m_SummonSlot[PET] read, here the live motion's PetGUID.
	methods["GetMinionGUID"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.PetGUID
	})
	// GetPetGUID is Unit::GetPetGUID (UnitMethods.h:680): the
	// m_SummonSlot[PET] read, same slot as GetMinionGUID above.
	methods["GetPetGUID"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.PetGUID
	})
	// SetPetGUID is Unit::SetPetGUID (UnitMethods.h:1806): the
	// m_SummonSlot[PET] write. LuaFunctions.cpp:331 registers
	// SetMinionGUID to this same C++ function, so both keys share
	// this closure. No pet object is created or dismissed here,
	// matching the C++ slot write.
	setPetGUID := func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.motionMu.Lock()
		motion.PetGUID = guid
		s.motionMu.Unlock()
		return nil, nil
	}
	methods["SetPetGUID"] = setPetGUID
	methods["SetMinionGUID"] = setPetGUID
	// SetOwnerGUID is Unit::SetOwnerGUID (Unit.cpp:5971): the
	// UNIT_FIELD_SUMMONEDBY write, with C++'s equal-value early
	// return. DELTA: C++'s owner-player values-update notify has
	// no Go twin.
	methods["SetOwnerGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.motionMu.Lock()
		if motion.OwnerGUID != guid {
			motion.OwnerGUID = guid
		}
		s.motionMu.Unlock()
		return nil, nil
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
	// GetOwnerGUID is Unit::GetOwnerGUID (Unit.h:1147): the
	// UNIT_FIELD_SUMMONEDBY read, the live motion's OwnerGUID
	// (the GetOwner precedent reads the same field).
	methods["GetOwnerGUID"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.OwnerGUID}, nil
	}
	methods["IsInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		inCombat := motion.InCombat
		s.motionMu.Unlock()
		return []any{inCombat}, nil
	}
	// IsInWater/IsUnderWater are Unit::IsInWater/IsUnderWater
	// (Unit.cpp:3276-3284) via the grid-leg liquid status, the
	// handler-creature twins of the player methods in lua_player.go:
	// live motion coords under motionMu, same vmap-only limitation.
	methods["IsInWater"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, x, y, z := motion.Map, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		return []any{s.mapIsInWater(mapID, x, y, z)}, nil
	}
	methods["IsUnderWater"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, x, y, z := motion.Map, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		return []any{s.mapIsUnderWater(mapID, x, y, z)}, nil
	}
	// SendUnitSay/SendUnitYell/SendUnitWhisper/SendUnitEmote are
	// UnitMethods (UnitMethods.h:2320-2390, Trinity): Unit::Say/Yell/
	// Whisper/TextEmote (Unit.cpp:14048-14072) over the live motion
	// coords. Empty messages are dropped by the Eluna binding itself.
	// Say/Yell/Emote fan out in the CONFIG_LISTEN_RANGE_* radius via
	// the same set-within-phase path the Talk rows use; Whisper writes
	// direct to the receiver session only (the Unit::Whisper
	// null-target arm: a receiver with no live session drops
	// silently). The Emote receiver and the Say/Yell target do not gate
	// delivery in C++ (Talk visits the range set; the target only feeds
	// the packet locale, and Go is enUS-only), so they are accepted
	// and ignored here. The session-creature surface keeps its own
	// gossip-targeted Whisper/TextEmote shorthands (lua_objects.go);
	// this is the handler-surface Eluna family, which was missing.
	motionChatRange := func(chatType uint8, language uint32, message string, listenRange float64) {
		s.motionMu.Lock()
		mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
		s.motionMu.Unlock()
		packet := protocol.BuildMonsterChatMessage(chatType, language, state.GUID, state.Name, message)
		s.broadcastToSetWithinFromPhase(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, mapID, instanceID, x, y, z, listenRange, phase)
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
		motionChatRange(0x0C, language, message, s.Config.ChatListenRangeSay)
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
		motionChatRange(0x0E, language, message, s.Config.ChatListenRangeYell)
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
		motionChatRange(chatType, languageUniversal, message, s.Config.ChatListenRangeTextEmote)
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
		receiver := s.findSessionByGUID(receiverGUID)
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
	// SendChatMessageToPlayer is UnitMethods::SendChatMessageToPlayer
	// (UnitMethods.h:2095-2113), shared in lua_worldobject_range.go;
	// creatures carry no chat tag, so the tag byte is 0.
	methods["SendChatMessageToPlayer"] = func(_ context.Context, args []any) ([]any, error) {
		return s.luaSendChatMessageToPlayer(state.GUID, 0, args)
	}
	// IsAttackingPlayer is Unit::isAttackingPlayer (Unit.cpp:5849-5865),
	// shared in lua_worldobject_range.go; the map/instance/GUID triple
	// is copied under motionMu like the motionChatRange arm above.
	methods["IsAttackingPlayer"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		return []any{s.creatureIsAttackingPlayer(mapID, instanceID, guid)}, nil
	}
	// SetData/GetData are the CreatureAI::SetData virtual-dispatch twin
	// (CreatureAI.h: the per-AI override interprets its own type numbers;
	// Go handlers are not AI objects, so there is no virtual table to
	// dispatch through — the store below is the transport and each
	// handler interprets its own types, e.g. the outhouse bunny reads
	// type 1 as gender, zone_grizzly_hills.cpp:296). Absent types read
	// 0, the C++ member default. Backs the go_amberpine_outhouse select
	// arm (go_scripts.cpp:624) and the bunny gender-sound arm.
	methods["SetData"] = func(_ context.Context, args []any) ([]any, error) {
		typ, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		data, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		if motion.ScriptData == nil {
			motion.ScriptData = map[uint32]uint32{}
		}
		motion.ScriptData[typ] = data
		s.motionMu.Unlock()
		return nil, nil
	}
	methods["GetData"] = func(_ context.Context, args []any) ([]any, error) {
		typ, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		data := motion.ScriptData[typ]
		s.motionMu.Unlock()
		return []any{data}, nil
	}
	// DoAction is the CreatureAI::DoAction dispatch twin (CreatureAI.h):
	// the C++ call runs the AI's virtual override for the action id, and
	// Lua-ported AIs have no AI object — so this fires the Go-side
	// CREATURE_EVENT_ON_DO_ACTION (38) hook with (event, creature,
	// action) on the entry then unique bindings, the void-hook twin of
	// the other creature events (C++ DoAction returns void, so the hook
	// returns are discarded). Aura scripts call it on the target creature
	// exactly as the C++ scripts call target->AI()->DoAction(action).
	methods["DoAction"] = func(ctx context.Context, args []any) ([]any, error) {
		action, err := luaInt32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s == nil || s.Features == nil || s.Features.Scripts == nil {
			return nil, nil
		}
		creature := s.luaMotionCreature(motion)
		if creature == nil {
			return nil, nil
		}
		doArgs := []any{scripting.CreatureEventOnDoAction, creature, action}
		_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDoAction, doArgs...)
		_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDoAction, doArgs...)
		return nil, nil
	}
	// SetGUID is the CreatureAI::SetGUID dispatch twin (CreatureAI.h):
	// the C++ call runs the AI's virtual override for the id with the
	// sender's GUID, and Lua-ported AIs have no AI object — so this
	// fires the Go-side CREATURE_EVENT_ON_SET_GUID (40) hook with
	// (event, creature, guid, id) on the entry then unique bindings,
	// the void-hook twin of the other creature events (C++ SetGUID
	// returns void, so the hook returns are discarded). The guid is
	// the low half (GetGUIDLow convention): packed GUIDs lose bits
	// through Lua numbers, so callers pass creature:GetGUIDLow() and
	// receivers resolve it with GetCreatureByGUID. Live user is the
	// Simon game (npc_simon_bunny.lua cluster-press transport); the
	// ICC orb-controller relay it was built for
	// (icecrown_citadel.cpp:680/729/821/833) is dead in C++ — the
	// 38008 trigger runs boss_blood_council_controller, which has no
	// SetGUID override — and its Lua arms were removed 2026-11-10
	// (npc_icc_orb_controller.lua dead-AI verdict).
	methods["SetGUID"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("SetGUID requires guid and id arguments")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, err
		}
		id, err := luaInt32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if s == nil || s.Features == nil || s.Features.Scripts == nil {
			return nil, nil
		}
		creature := s.luaMotionCreature(motion)
		if creature == nil {
			return nil, nil
		}
		setArgs := []any{scripting.CreatureEventOnSetGUID, creature, guid, id}
		_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnSetGUID, setArgs...)
		_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnSetGUID, setArgs...)
		return nil, nil
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
	// CastCustomSpell(target, spellId[, triggered[, bp0[, bp1[,
	// bp2[, castItem[, originalCaster]]]]]]) is Unit::CastSpell with
	// CastSpellExtraArgs (UnitMethods.h:2449-2480): the supplied base
	// points override only their effects (sparse map), triggered sets
	// TRIGGERED_FULL_MASK via the triggered twin. The castItem leg is
	// unmodeled on the creature path (no creature item model) and the
	// originalCaster leg has no Go model on either path, so both args
	// are accepted and ignored — the cast itself still fires.
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
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			s.motionMu.Lock()
			targetGUID = motion.TargetGUID
			s.motionMu.Unlock()
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
		s.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, targetGUID, triggered, basePoints, nil, false, 0, 0)
		return nil, nil
	}
	// CastSpellWithRadiusMod(target, spellId, triggered, radiusMod) is
	// Unit::CastSpell with CastSpellExtraArgs carrying AddSpellMod(
	// SPELLVALUE_RADIUS_MOD, radiusMod) (Spell.cpp:7881: RadiusMod =
	// value/10000; applied at Spell.cpp:1194/1284 as radius *=
	// RadiusMod). The radiusMod arg is the raw C++ int32 mod value
	// (10000 = 1.0x, unchanged radius); a non-positive mod leaves the
	// radius unmodified. The target is the explicit unit target —
	// self-cast for the self-centered area casts this serves (the
	// DoCastAOE convention); the area selection itself runs off the
	// caster position.
	methods["CastSpellWithRadiusMod"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 4 {
			return nil, fmt.Errorf("CastSpellWithRadiusMod requires a target, a spell id, triggered and a radius mod")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		triggered := false
		if b, ok := args[2].(bool); ok {
			triggered = b
		}
		mod, err := luaInt64Arg(args, 3)
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
		s.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, targetGUID, triggered, nil, nil, false, float32(mod)/10000, 0)
		return nil, nil
	}
	// CastSpellWithMaxTargets(target, spellId, triggered, maxTargets) is
	// Unit::CastSpell with CastSpellExtraArgs carrying AddSpellMod(
	// SPELLVALUE_MAX_TARGETS, maxTargets) (Spell.cpp:7883-7885): the
	// per-cast MaxAffectedTargets override (SpellInfo.cpp:853 default,
	// Spell.cpp:1207/1293 RandomResize cap), threaded as the local
	// spell copy's MaxTargets in castCreatureSpellInternalWithBasePoints
	// — the CastSpellWithRadiusMod twin. A non-positive maxTargets leaves
	// the DBC cap unmodified. Backs the negative-energy periodic port
	// (spell_generic.cpp:2344-2367), whose tick ramp is the only
	// deviation from the default periodic-trigger cast.
	methods["CastSpellWithMaxTargets"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 4 {
			return nil, fmt.Errorf("CastSpellWithMaxTargets requires a target, a spell id, triggered and a max targets value")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		triggered := false
		if b, ok := args[2].(bool); ok {
			triggered = b
		}
		maxTargets, err := luaInt64Arg(args, 3)
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
		s.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, targetGUID, triggered, nil, nil, false, 0, int32(maxTargets))
		return nil, nil
	}
	// CastSpellAoF(x, y, z, spellId[, triggered]) is Unit::CastSpell at
	// a Position (UnitMethods.h:2493-2514, Trinity: CastSpellExtraArgs
	// with TRIGGERED_FULL_MASK when triggered, default true). The
	// explicit destination replaces the server-side dest resolution in
	// the creature funnel and rides the deferred twin fire; a
	// non-triggered cast-time cast sends its START snapshot from the
	// resolver, not the explicit point (documented delta).
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
		dest := [3]float32{x, y, z}
		s.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, 0, triggered, nil, &dest, false, 0, 0)
		return nil, nil
	}
	// SetChannelObjectGuid(target) is Unit::SetChannelObjectGuid
	// (Unit.h:1360): SetGuidValue(UNIT_FIELD_CHANNEL_OBJECT, guid) —
	// retargets the caster's live channel object and broadcasts the
	// 64-bit field (low/high slots, the commands.go channelGUID
	// precedent). The motion twin is ChannelTargetGUID, read by
	// creatureChannelUnitTarget / creatureChannelDest for later
	// TARGET_UNIT_CHANNEL_TARGET / TARGET_DEST_CHANNEL_TARGET
	// resolution; the channel stamp at cast start is overwritten here
	// mid-cast, matching the C++ OnHit legs (laser barrage /
	// spinning up) that rewrite it per hit target. A nil/zero target
	// clears the channel object, matching C++'s guid clear.
	methods["SetChannelObjectGuid"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetChannelObjectGuid requires a target")
		}
		guid := luaObjectGUID(args[0])
		s.motionMu.Lock()
		motion.ChannelTargetGUID = guid
		mapID, instanceID := motion.Map, motion.InstanceID
		x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateAtInInstance(mapID, instanceID, motion.GUID, map[int]uint32{
			unitFieldChannelObject:     uint32(guid),
			unitFieldChannelObject + 1: uint32(guid >> 32),
		}, x, y, z, phase)
		return nil, nil
	}
	// InterruptSpell(spellType[, delayed]) is Unit::InterruptSpell
	// (UnitMethods.h:2547-2570, Unit.cpp:3140-3172): MELEE is a no-op
	// (no melee spell slot in Go, the lua_creature_events.go:2094
	// precedent), GENERIC cancels only the pending cast, CHANNELED
	// cancels only the live channel, AUTOREPEAT has no creature model.
	// The delayed arg is accepted but moot — Go has no SPELL_STATE_DELAYED
	// (pushback only extends the timer), so both values interrupt.
	// IsInterruptable is vacuous for creatures (synchronous casts).
	methods["InterruptSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spellType, err := luaCastSpellTypeArg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		var interrupted []interruptedCreatureCast
		switch spellType {
		case 1:
			interrupted = s.interruptCreatureGenericCast(motion)
		case 2:
			interrupted = s.interruptCreatureChannelCast(motion)
		}
		s.motionMu.Unlock()
		for _, arm := range interrupted {
			s.sendCreatureInterruptPackets(motion, arm)
		}
		return nil, nil
	}
	// StopSpellCast(spellId) is UnitMethods::StopSpellCast
	// (UnitMethods.h:2532-2537): Unit::CastStop (Unit.cpp:980-985)
	// interrupts every current non-melee spell except except_spellid
	// (default 0: all), composed from the single-arm helpers.
	methods["StopSpellCast"] = func(_ context.Context, args []any) ([]any, error) {
		var spellID uint32
		if len(args) > 0 {
			id, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			spellID = id
		}
		s.motionMu.Lock()
		var genericID uint32
		if motion.PendingInlineSpellID != 0 {
			genericID = motion.PendingInlineSpellID
		} else {
			genericID = motion.PendingCastSpellID
		}
		channelID := uint32(0)
		if s.creatureChannelLive(motion) {
			channelID = motion.ChannelSpellID
		}
		var interrupted []interruptedCreatureCast
		if genericID != 0 && genericID != spellID {
			interrupted = append(interrupted, s.interruptCreatureGenericCast(motion)...)
		}
		if channelID != 0 && channelID != spellID {
			interrupted = append(interrupted, s.interruptCreatureChannelCast(motion)...)
		}
		s.motionMu.Unlock()
		for _, arm := range interrupted {
			s.sendCreatureInterruptPackets(motion, arm)
		}
		return nil, nil
	}
	// GetCurrentSpell(spellType) is Unit::GetCurrentSpell
	// (UnitMethods.h:785-792): the live Spell object for the slot, nil
	// when the slot is empty. MELEE and AUTOREPEAT read nil (no slots
	// in Go); GENERIC reads the pending twin/inline spell (a post-GO
	// CastingUntil stamp alone is not a current spell, matching the
	// interrupt arms); CHANNELED reads the live channel only.
	methods["GetCurrentSpell"] = func(_ context.Context, args []any) ([]any, error) {
		spellType, err := luaCastSpellTypeArg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		spell := s.luaCreatureCurrentSpell(motion, spellType)
		s.motionMu.Unlock()
		return []any{spell}, nil
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
		s.broadcastCreatureTalk(ctx, motion.Map, motion.InstanceID, motion.GUID, motion.Entry, motion.Name, uint8(textID), whisperTarget)
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
	// GetStandState is Unit::GetStandState (Unit.h:921): the
	// UNIT_FIELD_BYTES_1 stand-state byte, which SetStandState writes
	// via motion.StandState (default 0 STAND when never set).
	// unitIsStandState is the shared Unit::IsStandState /
	// Unit::IsSitState predicate (Unit.cpp:10542-10555), used by
	// both the creature and player builders.
	methods["GetStandState"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		st := motion.StandState
		s.motionMu.Unlock()
		return []any{st}, nil
	}
	// IsStandState is Unit::IsStandState (Unit.cpp:10551-10555):
	// not a sit state (SIT/SIT_CHAIR/SIT_LOW_CHAIR/SIT_MEDIUM_CHAIR/
	// SIT_HIGH_CHAIR = 1/2/4/5/6) and neither SLEEP (3) nor KNEEL (8).
	methods["IsStandState"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		st := motion.StandState
		s.motionMu.Unlock()
		return []any{unitIsStandState(st)}, nil
	}
	// IsRooted is Unit::IsRooted (UnitMethods.h, Trinity arm):
	// UNIT_STATE_ROOT or MOVEMENTFLAG_ROOT. Go's twins are
	// motion.ScriptRooted (SetRooted) and the SPELL_AURA_MOD_ROOT
	// caster-aura effects; a bare movement-flag root has no Go
	// model, but SetControlled sets both in C++, so the script and
	// aura legs cover the reachable states.
	methods["IsRooted"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		rooted := motion.ScriptRooted
		s.motionMu.Unlock()
		if !rooted {
			rooted = len(s.creatureCasterAuraEffectsByType(motion, spellAuraModRoot)) > 0
		}
		return []any{rooted}, nil
	}
	// GetCorpseDelay is Creature::GetCorpseDelay (CreatureMethods.h:615,
	// Creature.h:94): the rank-mapped corpse-decay delay in seconds, the
	// same value creatureCorpseDelaySeconds feeds the corpse-removal
	// sweep. SmartScript SetCorpseDelay overrides are not modeled in Go
	// (no per-motion field); the default rank value is what the sweep
	// uses too.
	methods["GetCorpseDelay"] = func(ctx context.Context, _ []any) ([]any, error) {
		return []any{s.creatureCorpseDelaySeconds(ctx, motion.Entry)}, nil
	}
	// RemoveCorpse is Creature::RemoveCorpse (CreatureMethods.h:1187,
	// Creature.cpp:380-440 with default setSpawnTime=true): the direct
	// removal port in kill.go, firing ev26 with the respawn push.
	methods["RemoveCorpse"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.removeCreatureCorpseNow(ctx, motion)
		return nil, nil
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
	// IsHostileTo(target) is WorldObject::IsHostileTo (Object.cpp:2842-2845):
	// GetReactionTo(target) <= REP_HOSTILE. The target may be a player
	// object, a creature object, or a raw GUID. A wild creature vs a player
	// runs GetFactionReactionTo (Object.cpp:2795-2840) via
	// creatureHostileToPlayer; vs a creature the template term alone
	// decides via creatureHostileToCreature. Unresolvable targets read not
	// hostile.
	methods["IsHostileTo"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{false}, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 || targetGUID == motion.GUID {
			return []any{false}, nil
		}
		if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
			return []any{s.creatureHostileToPlayer(motion, ts)}, nil
		}
		if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); tm != nil {
			return []any{s.creatureHostileToCreature(motion, tm)}, nil
		}
		return []any{false}, nil
	}
	// IsFriendlyTo(target) is WorldObject::IsFriendlyTo (Object.cpp:2847-2850):
	// GetReactionTo(target) >= REP_FRIENDLY, via the existing
	// creatureFriendlyToTarget helper (boss_ai.go): player targets resolve
	// through their live session's faction state, creature targets through
	// a synthetic playerPos carrying the target's faction template. Self
	// is always friendly (Object.cpp:2711); unresolvable targets read not
	// friendly.
	methods["IsFriendlyTo"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{false}, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return []any{false}, nil
		}
		return []any{s.creatureFriendlyToTarget(motion, targetGUID)}, nil
	}
	// IsValidAttackTarget(target) is WorldObject::IsValidAttackTarget
	// (Object.cpp:2930) restricted to its hostility verdict via
	// creatureExplicitAttackPasses: the dead, GM, untargetable, visibility
	// and immunity arms have no twin here (the spell-path twin documents
	// them as outcome-equivalent for casts, which does not hold for
	// sighting/engage gates) — ports must compose those arms themselves
	// from the grid's alive-only filter, IsGM and IsTargetableForAttack.
	// Unresolvable creature targets fail open, like the spell-path twin.
	methods["IsValidAttackTarget"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{false}, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return []any{false}, nil
		}
		return []any{s.creatureExplicitAttackPasses(motion, nil, targetGUID)}, nil
	}
	// IsEngagedBy(target) is Unit::IsEngagedBy (Unit.h:1032): CanHaveThreatList
	// ? IsThreatenedBy(target) : IsInCombatWith(target). Wild creatures
	// carry threat lists, so the entry-presence twin decides; the
	// IsInCombatWith fallback for list-less owners is not modeled (a
	// creature's ThreatMgr is created on its first engage, so a nil manager
	// is never engaged).
	methods["IsEngagedBy"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 || motion.ThreatMgr == nil {
			return []any{false}, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return []any{false}, nil
		}
		return []any{motion.ThreatMgr.HasThreatEntry(targetGUID)}, nil
	}
	// EngageWithTarget(target) is Unit::EngageWithTarget (Unit.cpp:8429-8438)
	// via the engine aggro funnel: a 0.0f threat seed, victim set, combat
	// entered and ATTACK_START broadcast (triggerCreatureAggro also fires
	// the engager's event-1 hook, which is a no-op for handlers the
	// engager does not register). Creature victims funnel through the
	// same path since the AttackStart creature-victim port: the victim
	// motion seeds the engager's map/instance and the combat tick
	// resolves creature victims to a session-less target view. An evading
	// or passive engager is skipped by the funnel; C++ has no such gate.
	methods["EngageWithTarget"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, nil
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return nil, nil
		}
		if s.findSessionByGUID(targetGUID) == nil && s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID) == nil {
			return nil, nil
		}
		s.triggerCreatureAggro(ctx, motion.GUID, targetGUID)
		return nil, nil
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
	// HasAuraType(auraType) is Unit::HasAuraType (Unit.cpp) for handler
	// creatures: whether the creature carries a live (unstopped) aura of
	// the given SPELL_AURA_* type, via the creatureHasAuraType live fold.
	methods["HasAuraType"] = func(_ context.Context, args []any) ([]any, error) {
		auraType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.creatureHasAuraType(auraKey(), auraType)}, nil
	}
	// RemoveAurasWithMechanic(mask) is Unit::RemoveAurasWithMechanic
	// (Unit.cpp:4217-4232) for handler creatures: removes every live
	// aura whose GetAllEffectsMechanicMask intersects the mask, via
	// the removeCreatureAurasWithMechanic fold. Backs AfterHit ports
	// such as Mana Burn (8129), which strips FEAR and POLYMORPH
	// auras from the hit target.
	methods["RemoveAurasWithMechanic"] = func(_ context.Context, args []any) ([]any, error) {
		mask, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.removeCreatureAurasWithMechanic(auraKey(), mask)
		return nil, nil
	}
	// GetAura is Unit::GetAura (UnitMethods.h) for handler creatures:
	// the engine record for the spell, or nil when absent. The Lua
	// AddAura presence set (creatureAuras) carries no record, so a
	// Lua-added aura reads nil here, the GetDuration/GetStackAmount
	// 0-read convention for the same set; see lua_aura.go.
	methods["GetAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := auraKey()
		s.auraMu.Lock()
		aura := s.activeCreatureAuras[key][spell]
		s.auraMu.Unlock()
		if aura == nil {
			return []any{nil}, nil
		}
		ref := &luaAuraRef{server: s, spellID: spell, creatureKey: key, mapID: motion.Map, instanceID: motion.InstanceID}
		return []any{ref.luaAuraObject()}, nil
	}
	// GetZ is WorldObject::GetPositionZ (Object.h) over the live
	// motion: the player and GameObject surfaces already carry it
	// (lua_player.go, lua_objects.go) and the creature surface was
	// the gap — the nitro boosts backfire (54621) Z-fall check
	// reads the aura target's height on either surface.
	methods["GetZ"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.Z}, nil
	}
	// GetDummyAuraEffect(family, iconID[, effIndex]) is
	// Unit::GetDummyAuraEffect (Unit.cpp:4540-4543 -> 4510-4522)
	// for handler creatures: the live amount of the first DUMMY
	// aura matching family+icon at the effect index, or nil when
	// the creature carries none — the C++ null AuraEffect* twin,
	// the player-surface convention. Default effect 0.
	methods["GetDummyAuraEffect"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return []any{nil}, nil
		}
		family, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		iconID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		effIndex := 0
		if len(args) > 2 && args[2] != nil {
			v, err := luaInt64Arg(args, 2)
			if err != nil {
				return nil, err
			}
			effIndex = int(v)
		}
		if amount, ok := s.creatureDummyAuraAmountByIconEffIndex(auraKey(), family, iconID, effIndex); ok {
			return []any{amount}, nil
		}
		return []any{nil}, nil
	}
	// GetAuraEffect(spellID[, effIndex]) is Unit::GetAuraEffect
	// (Unit.cpp:4494-4496) for handler creatures: the live amount
	// of the named aura's effect, or nil when the creature does
	// not carry it. Default effect 0. Backs the Maelstrom Weapon
	// (53817) T10 4P bonus read.
	methods["GetAuraEffect"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{nil}, nil
		}
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		effIndex := 0
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			effIndex = int(v)
		}
		key := auraKey()
		s.auraMu.Lock()
		aura := s.activeCreatureAuras[key][spell]
		s.auraMu.Unlock()
		if aura == nil || effIndex < 0 || effIndex >= len(aura.Amounts) || aura.EffectMask&(1<<uint(effIndex)) == 0 {
			return []any{nil}, nil
		}
		return []any{aura.Amounts[effIndex]}, nil
	}
	// HasAuraEffect(spellID[, effIndex]) is Unit::HasAuraEffect
	// (Unit.cpp:4630-4642) for handler creatures: whether the creature
	// carries the named aura with an effect at the given index
	// (default 0) — the GetAuraEffect nil twin above. The C++
	// caster-GUID filter (third arg) is unmodeled; no in-tree Lua
	// caller passes one.
	methods["HasAuraEffect"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{false}, nil
		}
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		effIndex := 0
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			effIndex = int(v)
		}
		key := auraKey()
		s.auraMu.Lock()
		aura := s.activeCreatureAuras[key][spell]
		s.auraMu.Unlock()
		if aura == nil || effIndex < 0 || effIndex >= len(aura.Amounts) || aura.EffectMask&(1<<uint(effIndex)) == 0 {
			return []any{false}, nil
		}
		return []any{true}, nil
	}
	// HasAuraWithMechanic(mechanicMask) is Unit::HasAuraWithMechanic
	// (Unit.cpp:4727-4740) for handler creatures: whether the creature
	// carries any live aura whose spell-level mechanic falls inside
	// the mask (1 << mechanic, SharedDefines.h; stun is 1 << 12 =
	// 4096). The player twin backs the druid absorb gates; this twin
	// keeps the creature object at parity.
	methods["HasAuraWithMechanic"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{false}, nil
		}
		mask, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.creatureHasAuraMechanic(motion, mask)}, nil
	}
	// GetAuraEffectOfRankedSpell(spellID[, effIndex]) is
	// Unit::GetAuraEffectOfRankedSpell (Unit.cpp:4498-4508) for
	// handler creatures: the live amount of the effect on any
	// rank of the queried spell, or nil when no rank is present.
	// Default effect 0.
	methods["GetAuraEffectOfRankedSpell"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{nil}, nil
		}
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		effIndex := 0
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			effIndex = int(v)
		}
		key := auraKey()
		s.auraMu.Lock()
		auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
		for _, aura := range s.activeCreatureAuras[key] {
			auras = append(auras, aura)
		}
		s.auraMu.Unlock()
		if amount, ok := s.rankedAuraEffectAmount(auras, spell, effIndex); ok {
			return []any{amount}, nil
		}
		return []any{nil}, nil
	}
	// GetAuraEffectSpellIdByFamily(auraType, familyName, flag0, flag1,
	// flag2 [, casterGUID]) is the spell-id half of
	// Unit::GetAuraEffect(auraType, familyName, f0, f1, f2, casterGUID)
	// (Unit.cpp:4524-4545) for handler creatures: the spell id of the
	// first live aura effect of the given type whose spell matches
	// the family name and flag96 mask, cast by the given caster when
	// a nonzero caster GUID is passed — or nil when no live effect
	// matches. The player-object twin (lua_player.go) backs the
	// thrill-of-the-hunt Explosive Shot leg.
	methods["GetAuraEffectSpellIdByFamily"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 5 {
			return []any{nil}, nil
		}
		auraType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		family, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var flags [3]uint32
		for i := 0; i < 3; i++ {
			flags[i], err = luaUint32Arg(args, 2+i)
			if err != nil {
				return nil, err
			}
		}
		var casterGUID uint64
		if len(args) > 5 && args[5] != nil {
			v, err := luaInt64Arg(args, 5)
			if err != nil {
				return nil, err
			}
			if v > 0 {
				casterGUID = uint64(v)
			}
		}
		key := auraKey()
		s.auraMu.Lock()
		auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
		for _, aura := range s.activeCreatureAuras[key] {
			auras = append(auras, aura)
		}
		s.auraMu.Unlock()
		if spellID, ok := s.familyAuraEffectSpellID(auras, auraType, family, flags[0], flags[1], flags[2], casterGUID); ok {
			return []any{spellID}, nil
		}
		return []any{nil}, nil
	}
	// IsPolymorphed is Unit::IsPolymorphed (Unit.cpp:10572): a live
	// SPELL_AURA_TRANSFORM (56) aura, via the creatureIsPolymorphed twin.
	methods["IsPolymorphed"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{s.creatureIsPolymorphed(motion)}, nil
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
		return []any{creatureImmuneToDamage(s, auraKey(), mask)}, nil
	}
	// RemoveAura(spell[, mode]) is the handler-creature twin of the
	// player method above: the optional AuraRemoveMode rides
	// removeCreatureAuraWithMode instead of the default-mode helper.
	methods["RemoveAura"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		mode := auraRemoveByDefault
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			if v < 0 || v > 5 {
				return nil, fmt.Errorf("invalid aura remove mode")
			}
			mode = auraRemoveMode(v)
		}
		s.removeCreatureAuraWithMode(auraKey(), spell, mode)
		return nil, nil
	}
	// RemoveAuraFromStack(spellId[, casterGUID]) is Unit::RemoveAuraFromStack
	// (Unit.cpp:3917) on the creature surface: one stack comes off the named
	// aura (Aura::ModStackAmount(-1), SpellAuras.cpp:1030) and the aura is
	// removed when the last stack goes; returns true when the aura is gone
	// (absent included). The C++ caster-GUID argument is accepted and
	// ignored: the Go aura store holds one aura per spell per unit, so there
	// is no same-spell second instance to disambiguate. Backs the gaseous
	// bloat periodic arm (boss_professor_putricide.cpp:815-826).
	methods["RemoveAuraFromStack"] = func(_ context.Context, args []any) ([]any, error) {
		spell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.removeCreatureAuraFromStack(auraKey(), spell)}, nil
	}
	// RemoveMovementImpairingAuras is Unit::RemoveMovementImpairingAuras
	// (Unit.cpp:4193-4213) on the creature surface: withRoot=true drops
	// root-mechanic auras (plus the stalked/stun extra the creature
	// helper carries for Demonic Empowerment), withRoot=false drops
	// snare-mechanic auras only. The per-effect snare amount-zeroing
	// leg has no model — same documented residual as the helper.
	methods["RemoveMovementImpairingAuras"] = func(_ context.Context, args []any) ([]any, error) {
		withRoot := false
		if len(args) >= 1 {
			if b, ok := args[0].(bool); ok {
				withRoot = b
			}
		}
		s.removeCreatureMovementImpairingAuras(motion, withRoot)
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
	// GetDuration is Aura::GetDuration (SpellAuras.h) for handler
	// creatures: the live remaining duration in ms of the named aura,
	// or 0 when the aura is absent (the C++ call sites null-check the
	// Aura* first — returning 0 subsumes the missing-aura leg, the
	// GetStackAmount convention). RemainingMs is the duration at last
	// refresh and DurationUpdatedAt the refresh instant (creature aura
	// application sets both), so the live remaining is RemainingMs
	// minus elapsed since that instant, the spells.go:7233 /
	// boss_ai.go:2937 pattern. Only engine-applied auras
	// (activeCreatureAuras) carry durations; the Lua AddAura presence
	// set (creatureAuras) is boolean-only, so a Lua-added aura reads 0.
	methods["GetDuration"] = func(_ context.Context, args []any) ([]any, error) {
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
		remaining := int64(aura.RemainingMs)
		if !aura.DurationUpdatedAt.IsZero() {
			if elapsed := time.Since(aura.DurationUpdatedAt).Milliseconds(); elapsed > 0 {
				if uint64(elapsed) < uint64(remaining) {
					remaining -= elapsed
				} else {
					remaining = 0
				}
			}
		}
		if remaining < 0 {
			remaining = 0
		}
		return []any{uint32(remaining)}, nil
	}
	// RemoveAllAuras is Unit::RemoveAllAuras for handler creatures: the
	// full strip via clearCreatureAuras (the evade-leg twin — timers
	// stopped, both aura maps cleared). Freywinn's tranquility arm.
	methods["RemoveAllAuras"] = func(_ context.Context, _ []any) ([]any, error) {
		s.clearCreatureAuras(auraKey())
		return nil, nil
	}
	// HasBreakableByDamageCrowdControlAura is Unit::HasBreakableByDamageCrowdControlAura
	// (Unit.cpp:673-683) for the creature itself: the shared breakableCCAuraPresent
	// scan over its live engine-applied auras. The Lua AddAura presence set carries
	// no aura type, so a Lua-added aura never triggers the gate, the GetStackAmount
	// convention. The excludeCasterChannel leg is vacuous in Go (no creature
	// channeled-spell state, creaturemotion.go:1651-1667).
	methods["HasBreakableByDamageCrowdControlAura"] = func(_ context.Context, _ []any) ([]any, error) {
		key := auraKey()
		s.auraMu.Lock()
		snapshot := make(map[uint32]*activeAura, len(s.activeCreatureAuras[key]))
		for spellID, aura := range s.activeCreatureAuras[key] {
			snapshot[spellID] = aura
		}
		s.auraMu.Unlock()
		return []any{breakableCCAuraPresent(snapshot)}, nil
	}
	// RemoveAurasByType is Unit::RemoveAurasByType (Unit.cpp) for handler creatures:
	// every live engine-applied aura whose stored AuraType matches is removed via
	// the removeCreatureAura funnel. Lua-added presence-only auras carry no type
	// and are never matched. The caster-GUID / except-Aura legs take their C++
	// defaults (any caster, no exception).
	methods["RemoveAurasByType"] = func(_ context.Context, args []any) ([]any, error) {
		auraType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		key := auraKey()
		s.auraMu.Lock()
		var toRemove []uint32
		for spellID, aura := range s.activeCreatureAuras[key] {
			if aura == nil || aura.Stopped {
				continue
			}
			if aura.AuraType == auraType {
				toRemove = append(toRemove, spellID)
			}
		}
		s.auraMu.Unlock()
		for _, spellID := range toRemove {
			s.removeCreatureAura(key, spellID)
		}
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
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, mapID, instanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
		return []any{true}, nil
	}
	// Attack(target[, meleeAttack]) is the handler-creature twin of
	// the player binding (UnitMethods.h:22-27, Unit.cpp:5650-5760):
	// the shared luaUnitAttack gates and engage tail, over the live
	// motion (the AttackStop twin above owns the stop arm).
	methods["Attack"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("Attack requires a target")
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return nil, fmt.Errorf("Attack requires a target unit")
		}
		meleeAttack := luaOptionalBoolArg(args, 1, false)
		return []any{luaUnitAttack(ctx, luaUnitCreatureRef(s, motion), targetGUID, meleeAttack)}, nil
	}
	// DealDamage(target, damage[, durabilityloss[, school[, spell]]])
	// is the handler-creature twin of the player binding
	// (UnitMethods.h:2724-2857) through luaUnitDealDamage.
	methods["DealDamage"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("DealDamage requires a target and damage")
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return nil, fmt.Errorf("DealDamage requires a target unit")
		}
		damage, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		durabilityLoss := luaOptionalBoolArg(args, 2, true)
		school := uint32(7)
		if len(args) >= 4 {
			school, err = luaUint32Arg(args, 3)
			if err != nil {
				return nil, err
			}
		}
		if school > 7 {
			return nil, fmt.Errorf("DealDamage requires a valid SpellSchool (0-7)")
		}
		var spellID uint32
		if len(args) >= 5 {
			spellID, err = luaUint32Arg(args, 4)
			if err != nil {
				return nil, err
			}
		}
		luaUnitDealDamage(ctx, luaUnitCreatureRef(s, motion), targetGUID, damage, durabilityLoss, school, spellID)
		return nil, nil
	}
	// DealHeal(target, spell, amount[, critical]) is the
	// handler-creature twin of the player binding
	// (UnitMethods.h:2863-2892, Unit::HealBySpell Unit.cpp:6561)
	// through luaUnitDealHeal.
	methods["DealHeal"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("DealHeal requires a target, a spell and an amount")
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 {
			return nil, fmt.Errorf("DealHeal requires a target unit")
		}
		spellID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		amount, err := luaUint32Arg(args, 2)
		if err != nil {
			return nil, err
		}
		critical := luaOptionalBoolArg(args, 3, false)
		luaUnitDealHeal(ctx, luaUnitCreatureRef(s, motion), targetGUID, spellID, amount, critical)
		return nil, nil
	}
	// SetNoCallAssistance is Creature::SetNoCallAssistance
	// (Creature.h:246): it writes m_AlreadyCallAssistance directly, so
	// true means the creature never calls for assistance and false
	// re-opens the once-per-engagement call. The sticky half
	// (motion.NoCallAssistance) holds a Reset-ported true across the
	// engine's evade clear, which runs after Go's Lua event-23 fire
	// while C++ AI Reset runs after CombatStop.
	methods["SetNoCallAssistance"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetNoCallAssistance requires a bool, got no args")
		}
		noCall, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetNoCallAssistance requires a bool")
		}
		s.motionMu.Lock()
		motion.AlreadyCallAssistance = noCall
		motion.NoCallAssistance = noCall
		s.motionMu.Unlock()
		return nil, nil
	}
	// CallAssistance is Creature::CallAssistance (Creature.cpp:2402-2425)
	// through the engine's callCreatureAssistance funnel: once per
	// engagement, same-faction aggressive unengaged creatures within the
	// family-assistance radius are gathered and engage after the
	// configured delay. The funnel re-checks its own gates, so a
	// second call in the same engagement is a no-op there, matching
	// C++'s m_AlreadyCallAssistance early return.
	methods["CallAssistance"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.callCreatureAssistance(ctx, motion)
		return nil, nil
	}

	// CallForHelp is Creature::CallForHelp (Creature.cpp:2432-2456) via
	// the callCreatureForHelp funnel: immediate (undelayed) engage of
	// in-radius CanAssistTo creatures, unlike CallAssistance's delayed
	// family funnel. CHECKVAL requires the radius; radius <= 0 no-ops.
	methods["CallForHelp"] = func(ctx context.Context, args []any) ([]any, error) {
		radius, err := luaGenericFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.callCreatureForHelp(ctx, motion, radius)
		return nil, nil
	}
	// HasSearchedAssistance is Creature::HasSearchedAssistance
	// (Creature.h:248): the m_AlreadySearchedAssistance read. Only
	// DoFleeToGetAssistance and Lua SetNoSearchAssistance set it in
	// C++; Go's flee-to-assistance arm is unported, so Lua writes and
	// the 0.66 spline multiplier are its live readers.
	methods["HasSearchedAssistance"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.AlreadySearchedAssistance
	})
	// SetNoSearchAssistance is Creature::SetNoSearchAssistance
	// (Creature.h:247): it writes m_AlreadySearchedAssistance directly,
	// so true disables searching and false re-opens it. The Eluna
	// CHECKVAL default is true when omitted; a mistyped argument
	// errors, as CHECKVAL does.
	methods["SetNoSearchAssistance"] = func(_ context.Context, args []any) ([]any, error) {
		val := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetNoSearchAssistance requires a bool")
			}
			val = b
		}
		s.motionMu.Lock()
		motion.AlreadySearchedAssistance = val
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetRespawnDelay is Creature::GetRespawnDelay (Creature.h:262):
	// the live m_respawnDelay field — the SetRespawnDelay override
	// while known, else the creature.spawntimesecs DB row. A
	// despawned motion reads 0 through the DB-miss arm, the
	// temp-summon value.
	methods["GetRespawnDelay"] = func(ctx context.Context, _ []any) ([]any, error) {
		return []any{s.creatureRespawnDelaySeconds(ctx, motion.Map, motion.InstanceID, motion.GUID)}, nil
	}
	// SetRespawnDelay is Creature::SetRespawnDelay (Creature.h:263):
	// the m_respawnDelay write, consumed by the respawn schedulers at
	// death and cleared on respawn, when C++ re-inits from the DB
	// row. It writes no DB row, matching C++.
	methods["SetRespawnDelay"] = func(_ context.Context, args []any) ([]any, error) {
		delay, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.RespawnDelaySec = delay
		motion.RespawnDelayKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// CanAssistTo is Creature::CanAssistTo (CreatureMethods.h:76-88,
	// Creature.cpp:2458-2496) via creatureCanAssistTo: the assistant
	// gates (aggressive, alive, not evading, flags, unengaged, free),
	// the same-faction-or-friendly friend arm, and the hostile-to-enemy
	// arm. Friend/enemy resolve on the assistant's map/instance;
	// unresolvable parties error, the Eluna CHECKOBJ twin. Sessions
	// resolve before any motion lock: sessionsMu is never taken under
	// motionMu.
	methods["CanAssistTo"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("CanAssistTo requires a friend and an enemy Unit")
		}
		friend := s.resolveAssistParty(args[0], motion.Map, motion.InstanceID)
		if !friend.ok {
			return nil, fmt.Errorf("CanAssistTo requires a friend Unit")
		}
		enemy := s.resolveAssistParty(args[1], motion.Map, motion.InstanceID)
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
		s.motionMu.Lock()
		faction, passive, health, evading, inCombat, unitFlags, ownerGUID, charmerGUID := motion.Faction, isCreaturePassive(motion), motion.Health, motion.Evading, motion.InCombat, motion.UnitFlags, motion.OwnerGUID, motion.CharmerGUID
		s.motionMu.Unlock()
		return []any{s.creatureCanAssistTo(faction, passive, health, evading, inCombat, unitFlags, ownerGUID, charmerGUID, friend, enemy, checkfaction)}, nil
	}
	// CanSwim is Creature::CanSwim (Creature.h:105):
	// GetMovementTemplate().IsSwimAllowed() || IsPet(). The template
	// row rides loadCreatureStats (creature_template_movement, with
	// the C++ missing-row defaults); pets are motions with a nonzero
	// OwnerGUID; the spawn-override table has no Go load, so override
	// creatures read their template row.
	methods["CanSwim"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		swim := creatureCanSwimMotion(motion)
		s.motionMu.Unlock()
		return []any{swim}, nil
	}
	// CanWalk is Creature::CanWalk (Creature.h:104):
	// GetMovementTemplate().IsGroundAllowed(), i.e. Ground != None
	// (CreatureData.h:126). Same template-row / spawn-override deltas
	// as CanSwim; stat-less motions (pets, totems) read the C++
	// default (Ground=Run, allowed).
	methods["CanWalk"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		walk := creatureCanWalkMotion(motion)
		s.motionMu.Unlock()
		return []any{walk}, nil
	}
	// CanStartAttack is Creature::CanStartAttack (Creature.cpp:1960-2004)
	// via creatureCanStartAttack: civilian, immunity pairing,
	// non-combat-pet, non-flyer Z, then (force=false) _IsTargetAcceptable
	// + neutral/dist, then CanCreatureAttack, gray aggro, LOS. The
	// Eluna force argument defaults true (CreatureMethods.h:377-386);
	// the target resolves as a player session or a live creature motion
	// on the caller's map/instance, and an unresolvable target errors,
	// the Eluna CHECKOBJ twin. See creatureCanStartAttack for the arms
	// Go cannot mirror.
	methods["CanStartAttack"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("CanStartAttack requires a Unit target")
		}
		target := s.creatureAttackTargetResolve(args[0], motion.Map, motion.InstanceID)
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
		return []any{s.creatureCanStartAttack(ctx, motion, target, force)}, nil
	}
	// FleeToGetAssistance is Creature::DoFleeToGetAssistance
	// (Creature.cpp:1031-1052) via creatureFleeToGetAssistance: the
	// victim + prevents-fleeing gates, the nearest-assist search with
	// SetNoSearchAssistance(true), the untimed flee leg when no ally is
	// found, and a real move-toward-ally leg (with the arrival
	// CallAssistance + distract arms) when one is.
	methods["FleeToGetAssistance"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.creatureFleeToGetAssistance(ctx, motion)
		return nil, nil
	}
	// GetScriptId is Creature::GetScriptId (Creature.cpp:2872-2879):
	// the spawn row's scriptId when nonzero, else the template ScriptID.
	// Both resolve through the script-name registry (scriptIDForName):
	// Go motions carry the template ScriptName only, so the spawn
	// override arm collapses into the template arm — a spawn-level
	// ScriptName row has no Go load. See the registry comment for the
	// numbering delta vs C++'s sorted-store indices.
	methods["GetScriptId"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		id := scriptIDForName(motion.ScriptName)
		s.motionMu.Unlock()
		return []any{id}, nil
	}
	// SetDeathState is Creature::setDeathState (Creature.cpp:2057-2153
	// over Unit::setDeathState, Unit.cpp:8901-8960) via
	// applyCreatureDeathState: the state range follows the DeathState
	// enum (Unit.h:200-207), and the JUST_DIED -> CORPSE and
	// JUST_RESPAWNED -> ALIVE promotions run inside the call, so
	// IsDying still reads false afterwards, exactly as in C++.
	methods["SetDeathState"] = func(ctx context.Context, args []any) ([]any, error) {
		st, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if st > 4 {
			return nil, fmt.Errorf("SetDeathState requires a DeathState 0-4, got %d", st)
		}
		s.applyCreatureDeathState(ctx, motion, uint8(st))
		return nil, nil
	}
	// ResetPlayerDamageReq is Creature::ResetPlayerDamageReq
	// (Creature.h:324): m_PlayerDamageReq = GetHealth() / 2. Go models
	// the field as motion.PlayerDamageReq, consumed by the kill-path
	// loot-eligibility gates (kill.go), so this is a live write, not a
	// latch. It reads the motion's current health, so it must follow
	// any SetHealth in the same handler to match C++ call order.
	methods["ResetPlayerDamageReq"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		motion.PlayerDamageReq = motion.Health / 2
		s.motionMu.Unlock()
		return nil, nil
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
			wasInCombat := m.InCombat
			if m.ThreatMgr == nil {
				m.ThreatMgr = NewThreatManager(m)
			}
			for _, pg := range players {
				s.creatureAddThreat(m, pg, 0.0, true, s.findSessionByGUID(pg), nil)
				for _, pm := range s.motionMapLocked(mapID, instanceID) {
					if pm != nil && pm.OwnerGUID == pg && pm.Health > 0 {
						s.creatureAddThreat(m, pm.GUID, 0.0, true, nil, pm)
					}
				}
			}
			// CreatureAI::DoZoneInCombat (CreatureAI.cpp:75-104) engages via
			// EngageWithTarget -> AddThreat, so a fresh zone engage dismounts
			// and re-homes (Creature::AtEngage, Creature.cpp:3435-3436,
			// 3446-3447); later players add no second engage.
			if !wasInCombat {
				atEngageDismount(m)
				atEngageSetHomePosition(m)
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
	// expressible here; this bridge is self-threat only. The eager
	// switch hides from the step tick (ThreatManager.Update compares
	// its reselect against the already-switched currentVictim, so it
	// never re-fires), so the bridge syncs TargetGUID and broadcasts
	// HIGHEST_THREAT_UPDATE + ATTACK_START itself on a switch, on the
	// direct-set ports' gates (wasInCombat, non-passive); both
	// broadcasts flush after motionMu drops, as the visible-set
	// broadcast reads motionMu through its seer lookup.
	methods["AddThreat"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
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
		s.motionMu.Lock()
		wasInCombat := motion.InCombat
		passive := isCreaturePassive(motion)
		var switched bool
		var newVictim uint64
		var entries []protocol.ThreatEntry
		var switchStart pendingAttackStart
		var mapID, instanceID uint32
		var x, y, z float32
		var phase uint32
		if motion.ThreatMgr != nil {
			victimSess := s.findSessionByGUID(victimGUID)
			var victimMotion *creatureMotion
			if victimSess == nil {
				victimMotion = s.findCreatureMotionLocked(motion.Map, motion.InstanceID, victimGUID)
			}
			switched, newVictim = s.creatureAddThreat(motion, victimGUID, amount, false, victimSess, victimMotion)
			if switched && newVictim != motion.TargetGUID {
				motion.TargetGUID = newVictim
				entries = motion.ThreatMgr.SortedEntriesAvailable()
				mapID, instanceID, x, y, z, phase = motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
				if wasInCombat && !passive {
					switchStart = pendingAttackStart{creature: motion.GUID, victim: newVictim, mapID: mapID, instanceID: instanceID, x: x, y: y, z: z, phase: phase}
				}
			}
		}
		s.motionMu.Unlock()
		if entries != nil {
			s.broadcastHighestThreatUpdateInInstance(mapID, instanceID, x, y, z, motion.GUID, newVictim, entries, phase)
		}
		s.broadcastPendingAttackStarts(switchStart)
		return nil, nil
	}
	// FixateTarget is ThreatManager::FixateTarget (ThreatManager.cpp:494-506):
	// a script-fixated victim already on the threat list becomes the
	// preferred victim; a nil target (or a target off the list) clears the
	// fixate, which is the ClearFixate twin — the Go form is FixateTarget(0).
	// The argument is a creature/player object or a raw GUID; luaObjectGUID
	// yields 0 for nil, so creature:FixateTarget(nil) clears. A missing
	// ThreatManager means no list exists, so the fixate would clear in C++
	// too and the call is a no-op.
	methods["FixateTarget"] = func(_ context.Context, args []any) ([]any, error) {
		var victimGUID uint64
		if len(args) > 0 {
			victimGUID = luaObjectGUID(args[0])
		}
		s.motionMu.Lock()
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.FixateTarget(victimGUID)
		}
		s.motionMu.Unlock()
		return nil, nil
	}
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554):
	// HasUnitState(UNIT_STATE_CASTING). Go creature casts are fire-and-forget
	// (no in-flight cast state on the creature — the 2026-10-20 caststate
	// axiom), so the handler-surface twin is the constant false twin of the
	// session binding (lua_objects.go:342). Without it, ports calling
	// target:IsCasting() on a non-player threat victim (halazzi's shock
	// arm) nil-call and abort the timer chain mid-flight.
	// IsCasting is UnitMethods::IsCasting (UnitMethods.h:546-554):
	// HasUnitState(UNIT_STATE_CASTING), here the live CastingUntil
	// window — the 2026-10-20 "no cast state" axiom is stale since the
	// pending/channel cast-state model landed.
	methods["IsCasting"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{time.Now().Before(motion.CastingUntil)}, nil
	}
	// HasUnitState is UnitMethods::HasUnitState (UnitMethods.h:562-573):
	// Unit::HasUnitState, the (m_state & f) != 0 any-bit read; the
	// handler-surface twin of the session binding (lua_objects.go).
	// Live leg plus motion script mask live in lua_unit_state.go.
	methods["HasUnitState"] = func(_ context.Context, args []any) ([]any, error) {
		state, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{s.creatureUnitStateHas(motion, state)}, nil
	}
	// AddUnitState is UnitMethods::AddUnitState (UnitMethods.h:2651-2662):
	// Unit::AddUnitState, the m_state |= f write; the ROOT bit also sets
	// the ScriptRooted gate slot (lua_unit_state.go header).
	methods["AddUnitState"] = func(_ context.Context, args []any) ([]any, error) {
		state, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.creatureUnitStateAdd(motion, state)
		return nil, nil
	}
	// ClearUnitState is UnitMethods::ClearUnitState (UnitMethods.h:2668-2681):
	// Unit::ClearUnitState, the m_state &= ~f write; the ROOT bit also
	// clears the ScriptRooted gate slot. A live-held leg still reads
	// true (lua_unit_state.go).
	methods["ClearUnitState"] = func(_ context.Context, args []any) ([]any, error) {
		state, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.creatureUnitStateClear(motion, state)
		return nil, nil
	}
	// InterruptNonMeleeSpells is Unit::InterruptNonMeleeSpells
	// (Unit.cpp:3212-3225): cancels the pending generic cast and the
	// live channel, broadcasting each arm's cancel packets; with no arm
	// the plain CastingUntil gate clears, the CombatStop precedent.
	// (The prior no-op stub predates the pending/channel model; it
	// existed so svala's ritual-strike arm would not nil-call.)
	methods["InterruptNonMeleeSpells"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		interrupted := s.interruptCreatureNonMeleeCasts(motion)
		if len(interrupted) == 0 {
			motion.CastingUntil = time.Time{}
			s.cancelPendingCreatureCast(motion)
		}
		s.motionMu.Unlock()
		for _, arm := range interrupted {
			s.sendCreatureInterruptPackets(motion, arm)
		}
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
	// GetLootMode is CreatureMethods::GetLootMode (CreatureMethods.h:881):
	// Creature::GetLootMode (Creature.h:224) returns the m_LootMode
	// bitmask. Handler-surface twin of the session binding
	// (lua_objects.go:1018); the motion is the live creature here, so
	// no lookup is needed.
	methods["GetLootMode"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		mode := motion.LootMode
		s.motionMu.Unlock()
		return []any{uint32(mode)}, nil
	}
	// HasLootMode is CreatureMethods::HasLootMode (CreatureMethods.h:395):
	// Creature::HasLootMode (Creature.h:225) tests
	// (m_LootMode & lootMode) != 0. Handler-surface twin of the session
	// binding (lua_objects.go:1027).
	methods["HasLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		has := motion.LootMode&uint16(mode) != 0
		s.motionMu.Unlock()
		return []any{has}, nil
	}
	// SetLootMode is CreatureMethods::SetLootMode (CreatureMethods.h:935):
	// Creature::SetLootMode (Creature.h:226) assigns m_LootMode.
	// Handler-surface twin of the session binding (lua_objects.go:1046).
	methods["SetLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.LootMode = uint16(mode)
		s.motionMu.Unlock()
		return nil, nil
	}
	// RemoveLootMode is CreatureMethods::RemoveLootMode
	// (CreatureMethods.h:1305): Creature::RemoveLootMode (Creature.h:228)
	// clears the mode bits. Handler-surface twin of the session binding
	// (lua_objects.go:1066).
	methods["RemoveLootMode"] = func(_ context.Context, args []any) ([]any, error) {
		mode, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.LootMode &^= uint16(mode)
		s.motionMu.Unlock()
		return nil, nil
	}
	// ResetLootMode is CreatureMethods::ResetLootMode
	// (CreatureMethods.h:1297): Creature::ResetLootMode (Creature.h:229)
	// restores LOOT_MODE_DEFAULT. Handler-surface twin of the session
	// binding (lua_objects.go:1076).
	methods["ResetLootMode"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		motion.LootMode = lootModeDefault
		s.motionMu.Unlock()
		return nil, nil
	}
	// IsTappedBy is CreatureMethods::IsTappedBy (CreatureMethods.h:104):
	// Creature::isTappedBy (Creature.cpp:1362-1375) — the player GUID
	// matches the loot recipient, or the player is in the recipient
	// group. Handler-surface twin of the session binding
	// (lua_objects.go:950); the tap fields live on the motion, and the
	// group leg reads the player session exactly like the session twin.
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
		s.motionMu.Lock()
		tapPlayer, tapGroup := motion.TapPlayerGUID, motion.TapGroupID
		s.motionMu.Unlock()
		if playerGUID == tapPlayer {
			return []any{true}, nil
		}
		if tapGroup == 0 {
			return []any{false}, nil
		}
		if sess := s.findSessionByGUID(playerGUID); sess != nil && sess.groupID != 0 && sess.groupID == tapGroup {
			return []any{true}, nil
		}
		return []any{false}, nil
	}
	// IsEscorted is Creature::IsEscorted (Creature.cpp:3446's
	// AI()->IsEscorted() leg): ScriptedEscortAI::IsEscorted()
	// (ScriptedEscortAI.h:58) is the escort-player GUID non-empty;
	// the base CreatureAI returns false.
	methods["IsEscorted"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		escorted := motion.IsEscorted()
		s.motionMu.Unlock()
		return []any{escorted}, nil
	}
	// StartEscort is the ScriptedEscortAI::Start producer: it arms the
	// escort-player GUID the IsEscorted gate reads. The waypoint walk
	// itself rides the MoveWaypoint bridge + the OnReachWP (6) arrival
	// event, paused/resumed via SetEscortPaused; the EscortAI phase
	// machine (WaypointReached scripts) lives in Lua ports. The Start
	// respawn-queue leg (ScriptedEscortAI.cpp:280-285) rides along:
	// queueEscortRespawnTime queues the respawn record + row for
	// escort-quest NPCs
	// (CONFIG_RESPAWN_DYNAMIC_ESCORTNPC + SPAWNGROUP_FLAG_ESCORTQUESTNPC),
	// ordered before the engaged/already-escorting returns in C++.
	methods["StartEscort"] = func(ctx context.Context, args []any) ([]any, error) {
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
		s.motionMu.Lock()
		motion.EscortPlayerGUID = playerGUID
		s.motionMu.Unlock()
		s.queueEscortRespawnTime(ctx, motion)
		return nil, nil
	}
	// GetEscortPlayer is ScriptedEscortAI::GetPlayerForEscort: the
	// escort player object, nil when no escort runs or the player no
	// longer resolves (ObjectAccessor::GetPlayer). The legoso
	// healing-surge arm (zone_bloodmyst_isle.cpp:452-464) consumes it;
	// the resolution is the GetLootRecipient precedent (findPlayer).
	methods["GetEscortPlayer"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		playerGUID := motion.EscortPlayerGUID
		s.motionMu.Unlock()
		if playerGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(playerGUID); player != nil {
			return []any{player}, nil
		}
		return []any{nil}, nil
	}
	// StopEscort clears the escort-player GUID when the escort ends
	// (quest complete/fail, waypoint end): the escort AI object dies
	// with the escort in C++, so in Go the state needs an explicit
	// clear — respawn also clears it (kill.go). It also halts the
	// waypoint generator: EscortAI::UpdateAI at the last node runs
	// MoveIdle and drops STATE_ESCORT_ESCORTING (ScriptedEscortAI.cpp),
	// while Go paths repeat by default (creaturemotion.go), so without
	// the halt an ended escort would loop back to node 0 — the Thrall
	// Old Hillsbrad waypoint-108 end rides this.
	methods["StopEscort"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		motion.EscortPlayerGUID = 0
		motion.EscortPaused = false
		motion.MoveType = 0
		motion.Moving = false
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetEscortPaused is EscortAI::SetEscortPaused
	// (ScriptedEscortAI.cpp:353-372): pausing sets STATE_ESCORT_PAUSED
	// and pauses the waypoint generator; resuming clears the state and
	// the generator continues with the next waypoint. The C++ call is a
	// no-op while no escort runs, so the bridge ignores the set unless
	// an escort player is armed. The Go arm is the waypoint launch gate
	// in stepCreatureMotion (a spline already in flight still arrives).
	methods["SetEscortPaused"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("paused is required")
		}
		paused, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("paused must be a boolean")
		}
		s.motionMu.Lock()
		if motion.EscortPlayerGUID != 0 {
			motion.EscortPaused = paused
		}
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetLootRecipient is CreatureMethods::GetLootRecipient: the tapping
	// player object, nil when untapped (Eluna::Push((Player*)nullptr)).
	// Handler-surface twin of the session binding (lua_objects.go:990).
	methods["GetLootRecipient"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		tapPlayer := motion.TapPlayerGUID
		s.motionMu.Unlock()
		if tapPlayer == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(tapPlayer); player != nil {
			return []any{player}, nil
		}
		return []any{nil}, nil
	}
	// GetLootRecipientGroup is CreatureMethods::GetLootRecipientGroup:
	// the tapping group object, nil when the tap carries no group.
	// Handler-surface twin of the session binding (lua_objects.go:1002).
	methods["GetLootRecipientGroup"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		tapGroup := motion.TapGroupID
		s.motionMu.Unlock()
		if tapGroup == 0 {
			return []any{nil}, nil
		}
		if g := groupLuaObject(s, s.findGroupByID(tapGroup)); g != nil {
			return []any{g}, nil
		}
		return []any{nil}, nil
	}
	// IsDamageEnoughForLootingAndReward is
	// CreatureMethods::IsDamageEnoughForLootingAndReward:
	// Creature::m_PlayerDamageReq at 0 (Creature.h:322-325), lowered by
	// every player-attributed hit. Handler-surface twin of the session
	// binding (lua_objects.go:1010).
	methods["IsDamageEnoughForLootingAndReward"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		enough := motion.PlayerDamageReq == 0
		s.motionMu.Unlock()
		return []any{enough}, nil
	}
	// GetScriptName is CreatureMethods::GetScriptName (Creature.cpp:2867):
	// the script name the motion was banked with at spawn (ScriptName).
	// Handler-surface twin of the session binding (lua_objects.go:712),
	// which reads the same motion field through a lookup.
	methods["GetScriptName"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		name := motion.ScriptName
		s.motionMu.Unlock()
		return []any{name}, nil
	}
	// IsWorldBoss is CreatureMethods::IsWorldBoss:
	// Creature::isWorldBoss (Creature.cpp:2353-2358) — type_flags &
	// CREATURE_TYPE_FLAG_BOSS_MOB (0x4, SharedDefines.h:2731), not a pet.
	// Handler-surface twin of the session binding (lua_objects.go:689);
	// pets are motions with a nonzero OwnerGUID (the pet_combat.go
	// isPet twin).
	methods["IsWorldBoss"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		boss := motion.TypeFlags&0x4 != 0 && motion.OwnerGUID == 0
		s.motionMu.Unlock()
		return []any{boss}, nil
	}
	// IsElite is CreatureMethods::IsElite (CreatureMethods.h:191):
	// Creature::isElite (Creature.cpp:2344-2350) — not a pet, and rank
	// neither CREATURE_ELITE_NORMAL (0) nor CREATURE_ELITE_RARE (4)
	// (SharedDefines.h:2764-2771). Handler-surface twin of the session
	// binding (lua_objects.go:670); pets are motions with a nonzero
	// OwnerGUID (the pet_combat.go isPet twin).
	methods["IsElite"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		elite := motion.OwnerGUID == 0 && motion.Rank != 0 && motion.Rank != 4
		s.motionMu.Unlock()
		return []any{elite}, nil
	}
	// IsGuard / IsCivilian / IsTrigger are CreatureMethods.h:207/219/352:
	// Creature::IsGuard / IsCivilian / IsTrigger read flags_extra bits
	// CREATURE_FLAG_EXTRA_GUARD (0x8000) / CIVILIAN (0x2) / TRIGGER
	// (0x80) (CreatureData.h:38-52). Handler-surface twins of the
	// session bindings (lua_objects.go:696-702), reading the motion's
	// live FlagsExtra (banked from the template at spawn).
	methods["IsGuard"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		guard := motion.FlagsExtra&0x8000 != 0
		s.motionMu.Unlock()
		return []any{guard}, nil
	}
	methods["IsCivilian"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		civilian := motion.FlagsExtra&0x2 != 0
		s.motionMu.Unlock()
		return []any{civilian}, nil
	}
	methods["IsTrigger"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		trigger := motion.FlagsExtra&0x80 != 0
		s.motionMu.Unlock()
		return []any{trigger}, nil
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
	// GetArmor is Unit::GetArmor (Unit.h:860: GetResistance(SPELL_SCHOOL_NORMAL)):
	// the stored base armor from the creature motion, before the aura fold
	// the damage path applies (liveCreatureArmor, creature_auras.go).
	methods["GetArmor"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		armor := motion.Armor
		s.motionMu.Unlock()
		return []any{armor}, nil
	}
	// GetResistance is Unit::GetResistance(SpellSchools) (Unit.h:863:
	// UNIT_FIELD_RESISTANCES+school): the stored template resistances
	// from the creature motion (lua_creature_events.go:6529); Go has no
	// creature resistance aura fold, so this is the stored value, the
	// same limitation class as GetArmor above. School is the
	// SharedDefines.h enum (0 NORMAL, 1 HOLY, 2 FIRE, 3 NATURE,
	// 4 FROST, 5 SHADOW, 6 ARCANE). Backs the sapphiron
	// SpellHitTarget frost-resistance leg.
	methods["GetResistance"] = func(_ context.Context, args []any) ([]any, error) {
		school, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if school > 6 {
			return nil, fmt.Errorf("GetResistance requires a spell school 0-6, got %d", school)
		}
		s.motionMu.Lock()
		resistance := motion.Resistances[school]
		s.motionMu.Unlock()
		return []any{resistance}, nil
	}
	// SetArmor is Unit::SetArmor (Unit.h:861: SetResistance(SPELL_SCHOOL_NORMAL)):
	// writes the stored base armor and broadcasts UNIT_FIELD_RESISTANCES,
	// the same field the aura-change twin (recomputeCreatureArmorOnAuraChange)
	// refreshes. Lets Lua port SetArmor arms such as solarian's phase-4
	// WV_ARMOR set and Reset restore.
	methods["SetArmor"] = func(_ context.Context, args []any) ([]any, error) {
		armor, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.Armor = armor
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldResistances: armor})
		return nil, nil
	}
	// GetPower is Unit::GetPower (UnitMethods.h) over the live motion:
	// the Powers array the SetPower bridge writes, read under motionMu.
	// Validated 0-4 like SetPower (POWER_MANA..POWER_HAPPINESS); the
	// gunship-cannon energy read (POWER_ENERGY=3) rides this.
	methods["GetPower"] = func(_ context.Context, args []any) ([]any, error) {
		powerType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if powerType > 4 {
			return nil, fmt.Errorf("GetPower requires a power type 0-4, got %d", powerType)
		}
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.Powers[powerType]}, nil
	}
	// SetPower is Unit::SetPower (Unit.cpp): writes UNIT_FIELD_POWER1+type,
	// clamped to the type's max (C++ clamps via std::min against
	// GetMaxPower), then broadcasts the update field. powerType is validated
	// 0-4 (POWER_MANA..POWER_HAPPINESS).
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
	// SetPowerType is Unit::SetPowerType (Unit.cpp:5523-5569): the early
	// return on an unchanged type, then the UNIT_FIELD_BYTES_0 power-type
	// byte, then Creature::UpdateMaxPower (StatSystem.cpp:1030-1041) over
	// GetCreatePowerValue (Unit.cpp:9679-9701): ENERGY 100, RAGE 1000,
	// FOCUS/HAPPINESS 0 for a non-pet creature; MANA keeps the banked
	// spawn mana. The current-power legs follow the C++ switch: RAGE
	// resets to 0, FOCUS fills to max, ENERGY/MANA keep current. The
	// group/pet update-flag legs are player/pet-only. No bytes-0
	// broadcast: creature update fields carry no bytes-0 slot
	// (SetCombatReach precedent). Backs the setdemand hunt (toc rogue
	// Reset ENERGY + full).
	methods["SetPowerType"] = func(_ context.Context, args []any) ([]any, error) {
		powerType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if powerType > 4 {
			return nil, fmt.Errorf("SetPowerType requires a power type 0-4, got %d", powerType)
		}
		s.motionMu.Lock()
		if motion.PowerType == powerType {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.PowerType = powerType
		updates := map[int]uint32{}
		switch powerType {
		case 3: // POWER_ENERGY
			motion.MaxPowers[powerType] = 100
			motion.BaseMaxPowers[powerType] = 100
			updates[unitFieldMaxPower1+int(powerType)] = 100
		case 1: // POWER_RAGE
			motion.MaxPowers[powerType] = 1000
			motion.BaseMaxPowers[powerType] = 1000
			updates[unitFieldMaxPower1+int(powerType)] = 1000
			motion.Powers[powerType] = 0
			updates[unitFieldPower1+int(powerType)] = 0
		case 2: // POWER_FOCUS
			motion.MaxPowers[powerType] = 0
			motion.BaseMaxPowers[powerType] = 0
			updates[unitFieldMaxPower1+int(powerType)] = 0
			motion.Powers[powerType] = 0
			updates[unitFieldPower1+int(powerType)] = 0
		case 4: // POWER_HAPPINESS
			motion.MaxPowers[powerType] = 0
			motion.BaseMaxPowers[powerType] = 0
			updates[unitFieldMaxPower1+int(powerType)] = 0
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		if len(updates) > 0 {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, updates)
		}
		return nil, nil
	}
	// GetMaxPower reads the live max for the creature's current power
	// type, or for the given type when passed one (the SetPower type
	// convention, 0-4). The generic fallback (object.go) reads the
	// event-time snapshot, so it goes stale after a mid-handler
	// SetPowerType; this live read lets a port fill a new type to its
	// max in the same handler (toc rogue SetFullPower). No-arg reads
	// match the fallback's current-type value on every existing call
	// site (none changes type mid-handler).
	methods["GetMaxPower"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		powerType := motion.PowerType
		if powerType >= uint32(len(motion.MaxPowers)) {
			powerType = 0
		}
		s.motionMu.Unlock()
		if len(args) > 0 {
			var err error
			powerType, err = luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			if powerType > 4 {
				return nil, fmt.Errorf("GetMaxPower requires a power type 0-4, got %d", powerType)
			}
		}
		s.motionMu.Lock()
		maxPower := motion.MaxPowers[powerType]
		s.motionMu.Unlock()
		return []any{maxPower}, nil
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
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, mapID, instanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
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
		var interrupted []interruptedCreatureCast
		if includingCast {
			// Unit::CombatStop(true) -> InterruptNonMeleeSpells ->
			// Spell::cancel, so the arms broadcast their cancel
			// packets after the lock drops, ahead of AttackStop.
			interrupted = s.interruptCreatureNonMeleeCasts(motion)
			if len(interrupted) == 0 {
				motion.CastingUntil = time.Time{}
				s.cancelPendingCreatureCast(motion)
			}
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
		for _, arm := range interrupted {
			s.sendCreatureInterruptPackets(motion, arm)
		}
		if victim != 0 {
			stopPkt := buildAttackStop(guid, victim, false)
			s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, mapID, instanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
		}
		s.clearCreatureVisualTarget(motion)
		s.removeCreatureAttackers(mapID, instanceID, guid)
		s.removeThreatVictimFromAllLists(mapID, instanceID, guid)
		s.broadcastThreatClearInInstance(mapID, instanceID, motion.X, motion.Y, motion.Z, guid, motion.PhaseMask)
		return nil, nil
	}
	// ClearInCombat is Unit::ClearInCombat (UnitMethods.h:2521-2524,
	// Unit.h:1052) -> CombatManager::EndAllCombat (CombatManager.h:126,
	// CombatManager.cpp:344-353): EndAllPvECombat + EndAllPvPCombat.
	// Unlike CombatStop above, C++ clears NO attack state here: no
	// AttackStop, no interrupt, no target drop, no evade legs. The
	// carried legs are the flag clear, the own threat wipe
	// (ClearAllThreat) and the cross-list removal
	// (RemoveMeFromThreatLists, via removeThreatVictimFromAllLists),
	// plus the threat-clear broadcast CombatStop already uses.
	// DELTAS: the per-ref both-sides flag update
	// (CombatReference::EndCombat -> UpdateOwnerCombatState) has no Go
	// ref model, so attackers keep a stale InCombat until their own
	// teardown — the same accepted delta as CombatStop's attacker leg;
	// the JustExitedCombat AI hook and the Eluna OnPlayerLeaveCombat
	// hook have no twin here (CombatStop fires none either); PvP refs
	// have no Go model outside sanctuary handling, so the PvP leg
	// collapses into the same flag + threat teardown.
	methods["ClearInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		motion.InCombat = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
		s.motionMu.Unlock()
		s.removeThreatVictimFromAllLists(mapID, instanceID, guid)
		s.broadcastThreatClearInInstance(mapID, instanceID, x, y, z, guid, phase)
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
		phase := motion.PhaseMask
		transportGUID := motion.TransportGUID
		x, y, z := motion.X, motion.Y, motion.Z
		motion.Moving = false
		s.motionMu.Unlock()
		s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z, phase, transportGUID)
		return nil, nil
	}
	// MoveStop is UnitMethods::MoveStop (UnitMethods.h:2131-2136) =
	// Unit::StopMoving, the twin directly above. MoveClear and
	// MoveExpire are the TRINITY legs of UnitMethods.h:2142-2172 =
	// MotionMaster::Clear(), and MoveIdle is UnitMethods.h:2174-2178
	// = MotionMaster::MoveIdle(). Go has no movement-generator stack
	// and nothing resumes motion after a halt, so all four collapse
	// to the StopMoving twin: halt at the current position, no-op
	// when not moving. Threat, TargetGUID and InCombat are untouched
	// by every leg (the StopMoving comment above). MoveChase,
	// MoveFollow and MoveJump ride the chase/follow slots and the
	// parabolic launch below; the four halt legs never clear those
	// slots — C++ MoveStop (Unit::StopMoving) likewise leaves generators
	// armed, while MoveClear/MoveIdle would remove them, the standing
	// collapse delta. MoveTo, MoveRandom, MoveConfused and MoveHome
	// ride the script-spline launch below.
	methods["MoveStop"] = methods["StopMoving"]
	methods["MoveClear"] = methods["StopMoving"]
	methods["MoveExpire"] = methods["StopMoving"]
	methods["MoveIdle"] = methods["StopMoving"]
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
	// A TempSummon additionally fires event 20
	// (SummonedCreatureDespawn) on its summoner creature below,
	// the UnSummon leg; see the inline note at the fire site.
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
		isTempSummon, ownerGUID := m.IsTempSummon, m.OwnerGUID
		x, y, z := m.X, m.Y, m.Z
		s.motionMu.Unlock()
		// CleanupBeforeRemoveFromMap (Unit.cpp:9826-9829) cancels a
		// cast in flight before RemoveFromWorld, ahead of destroy.
		s.interruptCreatureCastsOnRemove(m)
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(guid)
		destroyBuf.WriteU8(0)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), mapID, instanceID, x, y, z, m.PhaseMask)
		// DespawnOrUnsummon on a TempSummon runs UnSummon
		// (TemporarySummon.cpp:274-284) FIRST, firing
		// SummonedCreatureDespawn on the summoner creature's AI;
		// the Eluna twin is event 20 on the summoner (the
		// expireTempSummonGuardian precedent). Player owners have
		// no creature hook, so the motion lookup skips them, and
		// non-temp creatures carry no summoner leg. MotionMu is
		// released above, as fireCreatureSummoned requires.
		if isTempSummon {
			if ownerMotion := s.findCreatureMotion(mapID, instanceID, ownerGUID); ownerMotion != nil {
				s.fireCreatureSummoned(ctx, ownerMotion, scripting.CreatureEventOnSummonedCreatureDespawn, m)
			}
		}
		// Eluna WORLD_EVENT_ON_DELETE_CREATURE (31): DespawnOrUnsummon
		// (Creature.cpp:2271) runs ForcedDespawn (Creature.cpp:2219;
		// delay 0 queues the delete at :2267 in both respawn modes)
		// or, for a TempSummon, UnSummon (TemporarySummon.cpp:298) —
		// both end at AddObjectToRemoveList, so the delete event fires
		// here even for DB creatures that later respawn as a new object.
		// The helper fires 31 then the RemoveFromWorld leg 37, in C++
		// order (previously this leg fired 37 before 31, and 20 last).
		s.fireServerDeleteCreature(ctx, m)
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
	// Respawn is Creature::Respawn (Creature.cpp:2156-2215, force=true
	// path): a dead creature returns to life at full health. The Go twin
	// mirrors the instance-respawn restoration in kill.go
	// (processCreatureRespawns): full health, home position, combat/loot/
	// tap state cleared, loot state cleared, values broadcast, then
	// fireCreatureSpawned (event-23 OnReset pre-fire, then event 5) so
	// the creature's Reset arm runs as C++ ai->Reset() does. A live
	// creature is a no-op: C++ force-respawn on an alive creature only
	// cycles it through JUST_DIED and skips the DEAD-gated restore
	// block. Pending scheduled respawns for the GUID are dropped so
	// the timer cannot double-fire the restore. The transformed-entry
	// revert (m_originalEntry) is not modeled: Go tracks no original
	// entry apart from the current one.
	methods["Respawn"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		m, ok := s.motionMapLocked(motion.Map, motion.InstanceID)[motion.GUID]
		if !ok || m.Health > 0 {
			s.motionMu.Unlock()
			return nil, nil
		}
		maxHealth := m.MaxHealth
		if maxHealth == 0 {
			maxHealth = m.BaseMaxHealth
		}
		if maxHealth == 0 {
			maxHealth = 1
		}
		m.Health, m.MaxHealth = maxHealth, maxHealth
		m.X, m.Y, m.Z = m.HomeX, m.HomeY, m.HomeZ
		m.Orientation = m.HomeO
		m.InCombat, m.Evading, m.TargetGUID, m.Moving = false, false, 0, false
		m.Looted = false
		m.TapPlayerGUID, m.TapGroupID = 0, 0
		m.DynamicFlags &^= unitDynFlagTapped | unitDynFlagLootable
		m.PlayerDamageReq = maxHealth / 2
		dbGUID := uint32(m.GUID & 0x00FFFFFF)
		delete(s.creatureRespawns, dbGUID)
		if respawns := s.instanceCreatureRespawns[instanceAdmissionKey{MapID: m.Map, InstanceID: m.InstanceID}]; respawns != nil {
			delete(respawns, dbGUID)
			if len(respawns) == 0 {
				delete(s.instanceCreatureRespawns, instanceAdmissionKey{MapID: m.Map, InstanceID: m.InstanceID})
			}
		}
		mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
		s.motionMu.Unlock()
		s.deleteCreatureRespawnRow(ctx, dbGUID, mapID, instanceID)
		s.clearLootState(mapID, instanceID, guid)
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldHealth: maxHealth, unitFieldDynamicFlags: 0})
		s.fireCreatureSpawned(ctx, m)
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
	// victim path's Health == 0 early-out). An optional first argument
	// names the killer creature (object or GUID, the ev14 caster idiom):
	// Unit::Kill(victim, killer) with the receiver as victim — the killer's
	// charmer/owner player takes the tap through creatureInstaKillVictim.
	// An unresolvable killer falls back to the caller (the old KillSelf
	// behavior); player killers have no motion twin and fall back the same way.
	methods["Kill"] = func(ctx context.Context, args []any) ([]any, error) {
		if motion == nil {
			return nil, nil
		}
		ownerPlayer := false
		if motion.OwnerGUID != 0 {
			if owner := s.findSessionByGUID(motion.OwnerGUID); owner != nil && owner.player != nil {
				ownerPlayer = true
			}
		}
		killer := motion
		if len(args) >= 1 {
			if killerGUID := luaObjectGUID(args[0]); killerGUID != 0 {
				if km := s.findCreatureMotion(motion.Map, motion.InstanceID, killerGUID); km != nil {
					killer = km
				}
			}
		}
		s.creatureInstaKillVictim(ctx, killer, motion.GUID)
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
		killerObj := s.luaMotionCreature(killer)
		if killerObj == nil {
			killerObj = creature
		}
		diedArgs := []any{scripting.CreatureEventOnDied, creature, killerObj}
		_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDied, diedArgs...)
		_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDied, diedArgs...)
		return nil, nil
	}
	// IsVehicle is Unit::IsVehicle (Unit.h:842): the UNIT_MASK_VEHICLE
	// bit, set in CreateVehicleKit and cleared in RemoveVehicleKit
	// (Unit.cpp:12047-12070) — a live vehicle kit is the exact twin, the
	// same predicate creatureCasterCanApplyResilience already uses
	// (resilience.go). Creature kits are created by the player-vehicle
	// spell path and by the Creature.cpp:1657 spawn twin (motionFor
	// fresh-motion branch, combat-target and aggro materialization).
	methods["IsVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil {
			return []any{false}, nil
		}
		return []any{s.getVehicleKit(motion.Map, motion.InstanceID, motion.GUID) != nil}, nil
	}
	// GetVehicleKit / IsOnVehicle are the motion-creature twins of the
	// player arms in lua_player.go (UnitMethods.h). GetVehicle,
	// GetVehicleBase and GetTransSeat ride the same motion stamp
	// (VehicleGUID / VehicleSeat) the IsOnVehicle arm reads.
	methods["GetVehicleKit"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil {
			return []any{nil}, nil
		}
		return []any{s.luaVehicleObject(motion.Map, motion.InstanceID, motion.GUID)}, nil
	}
	methods["IsOnVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.VehicleGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.luaVehicleObject(motion.Map, motion.InstanceID, motion.VehicleGUID)}, nil
	}
	methods["GetVehicle"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.VehicleGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.luaVehicleObject(motion.Map, motion.InstanceID, motion.VehicleGUID)}, nil
	}
	methods["GetVehicleBase"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.VehicleGUID == 0 {
			return []any{nil}, nil
		}
		return []any{s.luaUnitObjectForGUID(motion.Map, motion.InstanceID, motion.VehicleGUID)}, nil
	}
	methods["GetTransSeat"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.VehicleGUID == 0 {
			return []any{-1}, nil
		}
		return []any{int(motion.VehicleSeat)}, nil
	}
	// GetAllMinionsByEntry(entry) is the motion-creature twin of the
	// player arm in lua_player.go (Unit::GetAllMinionsByEntry,
	// Unit.cpp:6184-6194; see minionsByEntry in pet_combat.go): every
	// summoned creature of entry owned by this creature, in the same
	// map+instance, as a Lua array of creature objects (GUID-sorted).
	methods["GetAllMinionsByEntry"] = func(_ context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if motion == nil {
			return []any{[]any{}}, nil
		}
		s.motionMu.Lock()
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		out := make([]any, 0)
		for _, m := range s.minionsByEntry(mapID, instanceID, guid, entry) {
			out = append(out, s.luaMotionCreature(m))
		}
		return []any{out}, nil
	}
	// GetAttackers is the motion-creature twin of the player arm in
	// lua_player.go (Unit::getAttackers, Unit.h; see attackersOf in
	// pet_combat.go): the units with this creature as victim, as a
	// Lua array of unit objects (GUID-sorted).
	methods["GetAttackers"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil {
			return []any{[]any{}}, nil
		}
		s.motionMu.Lock()
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		out := make([]any, 0)
		for _, attackerGUID := range s.attackersOf(mapID, instanceID, guid) {
			if obj := s.luaUnitObjectForGUID(mapID, instanceID, attackerGUID); obj != nil {
				out = append(out, obj)
			}
		}
		return []any{out}, nil
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
		phase := m.PhaseMask
		s.motionMu.Unlock()
		_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: x, Y: y, Z: z, Orientation: o})
		s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, x, y, z, nearby, phase)
		s.syncCreatureVehiclePassengers(m)
		return nil, nil
	}
	// NearTeleport aliases Teleport: the handler Teleport above is
	// already Unit::NearTeleportTo (Unit.cpp:13332-13346) under the
	// Go-side name. DELTA: C++ NearTeleport CHECKVALs the orientation
	// (a missing o errors); the Teleport twin defaults it to the
	// current facing, and the alias inherits that leniency.
	methods["NearTeleport"] = methods["Teleport"]
	// ExitVehicle is Unit::ExitVehicle (Unit.cpp:13215-13234, the
	// _ExitVehicle twin) for creatures: with an explicit position it
	// unseats the passenger at those coordinates (the Kologarn
	// stone-grip leg passes the scripted exit position); with no
	// arguments it unseats at the passenger's current position — the
	// base-relocated position, the C++ default (base position with the
	// passenger's orientation). The kit removal rides
	// luaVehicleRemovePassenger (ev6 on a creature base, the
	// Vehicle::RemovePassenger twin); the position write rides the
	// Teleport broadcast above, so nearby clients see the exit at
	// once. RemovePendingEventsForPassenger has no Go model (vehicle
	// events are synchronous). A creature that is not seated is a
	// no-op, matching C++'s early return on a missing vehicle.
	methods["ExitVehicle"] = func(ctx context.Context, args []any) ([]any, error) {
		if motion == nil || motion.VehicleGUID == 0 {
			return nil, nil
		}
		s.motionMu.Lock()
		m, ok := s.motionMapLocked(motion.Map, motion.InstanceID)[motion.GUID]
		if !ok || m.VehicleGUID == 0 {
			s.motionMu.Unlock()
			return nil, nil
		}
		baseGUID := m.VehicleGUID
		x, y, z, o := m.X, m.Y, m.Z, m.Orientation
		if len(args) > 0 {
			var err error
			x, err = luaFloat32Arg(args, 0)
			if err != nil {
				s.motionMu.Unlock()
				return nil, err
			}
		}
		if len(args) > 1 {
			var err error
			y, err = luaFloat32Arg(args, 1)
			if err != nil {
				s.motionMu.Unlock()
				return nil, err
			}
		}
		if len(args) > 2 {
			var err error
			z, err = luaFloat32Arg(args, 2)
			if err != nil {
				s.motionMu.Unlock()
				return nil, err
			}
		}
		if len(args) > 3 {
			var err error
			o, err = luaFloat32Arg(args, 3)
			if err != nil {
				s.motionMu.Unlock()
				return nil, err
			}
		}
		mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
		phase := m.PhaseMask
		s.motionMu.Unlock()
		s.luaVehicleRemovePassenger(ctx, &luaVehicleRef{server: s, mapID: mapID, instanceID: instanceID, baseGUID: baseGUID}, guid)
		s.motionMu.Lock()
		if m2, ok := s.motionMapLocked(mapID, instanceID)[guid]; ok {
			m2.X, m2.Y, m2.Z, m2.Orientation = x, y, z, o
			m2.Moving = false
		}
		s.motionMu.Unlock()
		_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: x, Y: y, Z: z, Orientation: o})
		s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, x, y, z, nearby, phase)
		s.syncCreatureVehiclePassengers(m)
		return nil, nil
	}
	// Relocate is Object::Relocate (Object.h): a pure position write
	// with no packet — the client learns the new coordinates from the
	// next movement broadcast (the Kologarn stone-grip leg's MoveFall
	// packet). The orientation argument is optional and defaults to
	// the current facing, the Teleport precedent.
	methods["Relocate"] = func(_ context.Context, args []any) ([]any, error) {
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
		m, ok := s.motionMapLocked(motion.Map, motion.InstanceID)[motion.GUID]
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
		s.motionMu.Unlock()
		return nil, nil
	}
	// MoveFall is MotionMaster::MoveFall (MotionMaster.cpp:919-955)
	// for creatures: AddUnitMovementFlag(MOVEMENTFLAG_FALLING) plus a
	// fall spline to the ground height. Go has no creature spline
	// sender and no vmap height query, so the spline-to-ground half
	// stays out; the client-visible leg is the fall-mode packet —
	// SMSG_SPLINE_MOVE_NORMAL_FALL, the SetFeatherFall(false) leg's
	// opcode — sent fire-and-forget. MOVEMENTFLAG_FALLING is transient
	// in C++ (cleared on landing), so no latch is kept on the motion.
	// Rooted units do not fall in C++ (MotionMaster.cpp:937-939); the
	// ScriptRooted gate is the Go twin, and the stunned half stays out
	// (no creature stun-state read on the motion).
	methods["MoveFall"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil {
			return nil, nil
		}
		s.motionMu.Lock()
		m, ok := s.motionMapLocked(motion.Map, motion.InstanceID)[motion.GUID]
		if !ok || m.ScriptRooted {
			s.motionMu.Unlock()
			return nil, nil
		}
		mapID, instanceID, x, y, guid := m.Map, m.InstanceID, m.X, m.Y, m.GUID
		phase := m.PhaseMask
		s.motionMu.Unlock()
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, protocol.OpcodeSMSG_SPLINE_MOVE_NORMAL_FALL, phase)
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
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_ENABLE
		if disable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_DISABLE
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
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
	// GetPhaseMask/SetPhaseMask are WorldObject::GetPhaseMask/SetPhaseMask
	// (WorldObjectMethods.h, WotLK branch): the motion's stored phase mask.
	// SetPhaseMask takes the mask and an optional update flag (default true
	// in C++); Go broadcasts are phase-filtered at send time, so the stored
	// mask governs all future broadcasts, but no immediate create/destroy
	// fan-out runs for the update leg (C++ UpdateObjectVisibility) — Go
	// keeps no persistent per-player visibility set to refresh.
	methods["GetPhaseMask"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		return []any{phase}, nil
	}
	methods["SetPhaseMask"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetPhaseMask requires a phase mask")
		}
		mask, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.PhaseMask = mask
		motion.PhaseMaskKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetMap is WorldObject::GetMap (WorldObjectMethods.h): the DBC-backed
	// Map object for the creature's map/instance (NewMapObject + mapLive,
	// the same builder behind the player, gameobject, and instance-hook
	// surfaces). The session-creature twin (lua_objects.go) has carried it
	// since the WorldObjectMethods census; the motion builder never did,
	// so every handler-creature creature:GetMap() call (the instanceDataOf
	// idiom, ~148 sites) indexed to nil and died on the call.
	methods["GetMap"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		return []any{scripting.NewMapObject(mapID, instanceID, s.mapLive(mapID, instanceID))}, nil
	}
	// GetRelativePoint is WorldObjectMethods::GetRelativePoint
	// (WorldObjectMethods.h:630-646): WorldObject::GetClosePoint(x, y, z,
	// 0, dist, relAngle) = GetNearPoint with absAngle = orientation +
	// relAngle at (combatReach + dist) (Object.cpp:3307-3311, 3272-3269).
	// The height-collision re-solve (UpdateAllowedPositionZ/IsWithinLOS)
	// has no Go model, so z stays the creature's z. Same twin as the
	// session-creature surface (lua_objects.go); the motion builder never
	// had it, so king_dred's raptor-call summon point died on the call.
	methods["GetRelativePoint"] = func(_ context.Context, args []any) ([]any, error) {
		dist, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		rad, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		reach, orientation, x, y, z := motion.CombatReach, motion.Orientation, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		r := reach + dist
		a := orientation + rad
		return []any{x + r*float32(math.Cos(float64(a))), y + r*float32(math.Sin(float64(a))), z}, nil
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
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_HOVER
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_HOVER
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
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
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_UNSET_FLYING
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_FLYING
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
		return nil, nil
	}

	// SetWalk is Creature::SetWalk (Creature.cpp:3012-3021): Unit::SetWalk
	// flips the persistent MOVEMENTFLAG_WALKING (no-op when unchanged)
	// and broadcasts SMSG_SPLINE_MOVE_SET_WALK_MODE / SET_RUN_MODE
	// (packed GUID only). Go stores the flag on the motion and folds it
	// into creatureSplineVelocity, so later splines run at walk speed
	// until cleared — the persistent state C++ keeps, distinct from the
	// per-spline walk velocity of waypoint/wander legs.
	methods["SetWalk"] = func(_ context.Context, args []any) ([]any, error) {
		enable := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetWalk requires a bool")
			}
			enable = b
		}
		s.motionMu.Lock()
		if motion.Walking == enable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.Walking = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
		return nil, nil
	}
	// SetSwim is Unit::SetSwim (Unit.cpp:13728): flips
	// MOVEMENTFLAG_SWIMMING with the C++ no-op-on-unchanged early-out.
	// Unlike Creature::SetHover, C++ SetSwim itself sends no packet (a
	// flag flip only); the Go twin broadcasts SMSG_SPLINE_MOVE_START_SWIM/
	// STOP_SWIM (packed GUID only, via broadcastCreatureSplineMove) so
	// clients observe the change, the same broadcast-leg approach as
	// SetHover/SetCanFly.
	methods["SetSwim"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("SetSwim requires a bool, got no args")
		}
		enable, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("SetSwim requires a bool")
		}
		s.motionMu.Lock()
		if motion.Swimming == enable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.Swimming = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_STOP_SWIM
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_START_SWIM
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
		return nil, nil
	}
	// SetWaterWalk is Unit::SetWaterWalk (UnitMethods.h:1822,
	// Unit.cpp:13755): flips MOVEMENTFLAG_WATERWALKING with the C++
	// no-op-on-unchanged early-out. Like SetSwim, C++ itself sends no
	// packet for the flip, so the Go twin broadcasts
	// SMSG_SPLINE_MOVE_WATER_WALK / LAND_WALK (packed GUID only, via
	// broadcastCreatureSplineMove) so clients observe the change; the
	// latch follows the SetCanFly/SetSwim motion-bool pattern and no
	// creature movement-flag emission folds it (the sibling slots are
	// broadcast-leg only too).
	methods["SetWaterWalk"] = func(_ context.Context, args []any) ([]any, error) {
		enable := true
		if len(args) >= 1 && args[0] != nil {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetWaterWalk requires a bool")
			}
			enable = b
		}
		s.motionMu.Lock()
		if motion.WaterWalking == enable {
			s.motionMu.Unlock()
			return nil, nil
		}
		motion.WaterWalking = enable
		mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
		phase := motion.PhaseMask
		s.motionMu.Unlock()
		opcode := protocol.OpcodeSMSG_SPLINE_MOVE_LAND_WALK
		if enable {
			opcode = protocol.OpcodeSMSG_SPLINE_MOVE_WATER_WALK
		}
		s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
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
	// SummonCreatureGroup is WorldObject::SummonCreatureGroup
	// (Object.cpp:2132-2151): looks up (GetEntry(), SUMMONER_TYPE_CREATURE,
	// group) in creature_summon_groups (ObjectMgr::LoadTempSummonData,
	// ObjectMgr.cpp:1996) and summons every row via SummonCreature with the
	// row's position, summonType and summonTime (milliseconds), collecting
	// the TempSummon*s into the out-param list. The per-call world-DB query
	// mirrors summonTempCreature's own per-call creature_template check —
	// no startup loader or cache; the group rows live only in TDB, not in
	// the C++ tree. Returns the summoned creatures as a Lua array (the
	// &list sites — Kel'Thuzad guardians/minions, Yogg clouds, Valithria
	// archmages — iterate it); a missing table or no rows yields an empty
	// table, the C++ `if (!data) return;` twin (Object.cpp:2136-2140),
	// required because the repo's world.db placeholder carries no such
	// table. Rows whose template is unknown are skipped by the funnel
	// (nil return), exactly C++ logging null for a missing template.
	methods["SummonCreatureGroup"] = func(ctx context.Context, args []any) ([]any, error) {
		groupID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		out := []any{}
		if s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{out}, nil
		}
		s.motionMu.Lock()
		summonerEntry := motion.Entry
		s.motionMu.Unlock()
		rows, err := s.WorldStore.DB.QueryContext(ctx,
			"SELECT entry, position_x, position_y, position_z, orientation, summonType, summonTime FROM creature_summon_groups WHERE summonerId = ? AND summonerType = 0 AND groupId = ?",
			summonerEntry, groupID)
		if err != nil {
			return []any{out}, nil
		}
		defer rows.Close()
		origin := s.summonOriginFromMotion(motion)
		for rows.Next() {
			var entry int64
			var x, y, z, o float64
			var summonType, summonTime int64
			if err := rows.Scan(&entry, &x, &y, &z, &o, &summonType, &summonTime); err != nil {
				continue
			}
			if summoned := s.summonTempCreature(ctx, origin, uint32(entry), float32(x), float32(y), float32(z), float32(o), uint8(summonType), uint32(summonTime)); summoned != nil {
				out = append(out, s.luaMotionCreature(summoned))
			}
		}
		return []any{out}, nil
	}
	// SpawnCreature is WorldObject::SpawnCreature (WorldObjectMethods.h),
	// the Eluna 1-8 spawn-type twin of SummonCreature; the shared
	// spawnCreatureFromOrigin funnel above documents the type translation
	// and the seconds-to-milliseconds despawn conversion.
	methods["SpawnCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		summoned, err := s.spawnCreatureFromOrigin(ctx, s.summonOriginFromMotion(motion), args)
		if err != nil {
			return nil, err
		}
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
		phaseMask := motion.PhaseMask
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
		dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: entry, OwnerGUID: ownerGUID, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, Orientation: o, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, PhaseMask: phaseMask, IsRuntimeSpawn: true}
		if respawnSecs > 0 {
			dyn.DespawnTimer = time.AfterFunc(time.Duration(respawnSecs)*time.Second, func() {
				s.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
			})
		}
		s.spawnDynamicGameObject(dyn)
		return []any{newGUID}, nil
	}
	// GetGameObjectByGUID is ObjectAccessor::GetGameObject (ObjectAccessor.h)
	// by packed GUID — the creature-surface twin of the player bridge
	// (lua_player.go): unlike GetGameObject above it is not keyed on an
	// owner spell, so it resolves runtime-spawned objects the creature
	// summoned (serverLuaGameObject reads the dynamic registries first,
	// then the DB-saved rows). Nil when no such object is live. Backs the
	// legoso explosives-detonate arms (zone_bloodmyst_isle.cpp: the
	// summoned draenei-explosives GOs are removed by GUID).
	methods["GetGameObjectByGUID"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{nil}, nil
		}
		return []any{s.serverLuaGameObject(ctx, guid)}, nil
	}
	// GetCreatureByGUID is ObjectAccessor::GetCreature (ObjectAccessor.h)
	// by low GUID — the creature-surface twin of GetGameObjectByGUID:
	// Lua GUIDs travel as the counter half (GetGUIDLow convention; packed
	// GUIDs lose bits through Lua numbers), so the lookup scans the
	// map+instance motions for a matching low half, the same scan
	// GetCreaturesInGrid runs under motionMu. Nil when no such creature
	// is live. Built for the ICC orb-controller relay
	// (icecrown_citadel.cpp:729-749); that relay is dead in C++
	// (npc_icc_orb_controller.lua verdict) and the live users are the
	// ZF pyramid wave adds (instance_zulfarrak.lua) and the
	// Quel'Danas thalorien/morlen lookups
	// (zone_isle_of_queldanas.lua).
	methods["GetCreatureByGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, err
		}
		if s == nil {
			return []any{nil}, nil
		}
		low := guid & 0x00FFFFFF
		s.motionMu.Lock()
		var target *creatureMotion
		for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if m != nil && m.GUID&0x00FFFFFF == low {
				target = m
				break
			}
		}
		s.motionMu.Unlock()
		if target == nil {
			return []any{nil}, nil
		}
		return []any{s.luaMotionCreature(target)}, nil
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
	// IsGuardian is Unit::IsGuardian (Unit.h:838 — the UNIT_MASK_GUARDIAN
	// bit of m_unitTypeMask, minted by Map::SummonCreature's mask rule):
	// the motion flag stamped by the spell-summon sites that resolve
	// SummonProperties (summonPropertiesIsGuardian); DB spawns, pets
	// and Lua SummonCreature summons read false.
	methods["IsGuardian"] = luaNoArgs(func() any { return motion.IsGuardian })
	// IsPet is Unit::IsPet (Unit.h:839 — the UNIT_MASK_PET bit of
	// m_unitTypeMask): the motionIsPet twin (creature_auras.go:572,
	// PetID != 0). Backs the masters-call DoCheckCast port
	// (spell_hunter.cpp:838-869).
	methods["IsPet"] = luaNoArgs(func() any { return motionIsPet(motion) })
	// GetGameObject is Unit::GetGameObject(spellId) (Unit.cpp:5199): the
	// owned gameobject summoned by that spell, or nil — the creature
	// twin of the player bridge (the demonic-circle arms are
	// player-only, but the C++ method is Unit-level).
	methods["GetGameObject"] = func(ctx context.Context, args []any) ([]any, error) {
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		guid := s.gameObjectByOwnerSpell(motion.GUID, spellID)
		if guid == 0 {
			return []any{nil}, nil
		}
		return []any{s.serverLuaGameObject(ctx, guid)}, nil
	}
	// RemoveGameObject is Unit::RemoveGameObject(spellid, del)
	// (Unit.cpp:5263): despawns every owned gameobject of that spell id.
	methods["RemoveGameObject"] = func(_ context.Context, args []any) ([]any, error) {
		spellID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.removeGameObjectsByOwnerSpell(motion.GUID, spellID)
		return nil, nil
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
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), pkt.Bytes(), motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
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

	// GetAttackDistance is Creature::GetAttackDistance (Creature.cpp:2015-2058)
	// via creatureAttackDistance: the level/reach/aura formula, without the
	// TurretAI/ArcherAI +m_CombatDistance arm C++ adds only in CanStartAttack.
	// The target resolves as a player session or a live creature motion on
	// the caller's map/instance; an unresolvable target errors, the Eluna
	// CHECKOBJ twin.
	methods["GetAttackDistance"] = func(ctx context.Context, args []any) ([]any, error) {
		targetLevel, targetDetected, ok := s.creatureDistanceTarget(args, motion.Map, motion.InstanceID)
		if !ok {
			return nil, fmt.Errorf("GetAttackDistance requires a Unit target")
		}
		return []any{s.creatureAttackDistance(ctx, motion, targetLevel, targetDetected)}, nil
	}
	// GetAggroRange is Creature::GetAggroRange (Creature.cpp:3130-3174),
	// the pet-only variant via creatureAggroRange; non-pets read 0, the
	// C++ Default arm. Go approximates IsPet with a nonzero OwnerGUID.
	methods["GetAggroRange"] = func(ctx context.Context, args []any) ([]any, error) {
		targetLevel, targetDetected, ok := s.creatureDistanceTarget(args, motion.Map, motion.InstanceID)
		if !ok {
			return nil, fmt.Errorf("GetAggroRange requires a Unit target")
		}
		s.motionMu.Lock()
		isPet := motion.OwnerGUID != 0
		s.motionMu.Unlock()
		if !isPet {
			return []any{float32(0)}, nil
		}
		ownDetect := s.creatureAuraTypeSum(creatureAuraKeyForMotion(motion), spellAuraModDetectRange)
		return []any{creatureAggroRange(motion, targetLevel, ownDetect, targetDetected)}, nil
	}
	// SelectVictim is Creature::SelectVictim (Creature.cpp:1194-1258): the
	// threat-list current victim when the creature can have a threat list,
	// else nil (the pet getAttackerForHelper leg has no Go twin). The
	// _IsTargetAcceptable/CanCreatureAttack re-checks are NOT replicated:
	// the victim must resolve on the caller's map/instance and be alive
	// (the DIED arm); the vehicle, permanent-invisibility evade, and
	// SetInFront arms stay out, and a failed gate reads nil without
	// entering evade mode, since a Lua read must not evade the creature.
	methods["SelectVictim"] = func(_ context.Context, _ []any) ([]any, error) {
		if motion == nil || motion.ThreatMgr == nil || !motionCanHaveThreatList(motion) {
			return []any{nil}, nil
		}
		victimGUID := motion.ThreatMgr.GetCurrentVictim()
		if victimGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(victimGUID); player != nil {
			if sess := s.findSessionByGUID(victimGUID); sess != nil && sess.player != nil && sess.player.Map == motion.Map && sess.player.InstanceID == motion.InstanceID && sess.player.Health > 0 {
				return []any{player}, nil
			}
			return []any{nil}, nil
		}
		if target := s.findCreatureMotion(motion.Map, motion.InstanceID, victimGUID); target != nil && target.Health > 0 {
			return []any{s.luaMotionCreature(target)}, nil
		}
		return []any{nil}, nil
	}
	// GetAITarget is CreatureMethods::GetAITarget (CreatureMethods.h:706-799,
	// Trinity): the threat-list selection over the filtered victim list —
	// RANDOM=0, TOPAGGRO=1, BOTTOMAGGRO=2, NEAREST=3, FARTHEST=4 — with the
	// playerOnly, position, distance (positive = within, negative =
	// beyond), and aura (positive = has, negative = lacks) filters. An
	// empty list or out-of-range position reads nil; an unknown target
	// type errors, the luaL_argerror twin.
	methods["GetAITarget"] = func(_ context.Context, args []any) ([]any, error) {
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
		guid, err := s.selectAIThreatTarget(motion, targetType, playerOnly, position, dist, aura)
		if err != nil {
			return nil, err
		}
		if guid == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(guid); player != nil {
			return []any{player}, nil
		}
		if target := s.findCreatureMotion(motion.Map, motion.InstanceID, guid); target != nil {
			return []any{s.luaMotionCreature(target)}, nil
		}
		return []any{nil}, nil
	}
	// GetCurrentWaypointId is CreatureMethods::GetCurrentWaypointId
	// (CreatureMethods.h:452-458, Trinity): the first of
	// GetCurrentWaypointInfo, the node id the waypoint generator last
	// reported on arrival (WaypointMovementGenerator.cpp:264); 0 before
	// the first arrival or for pathless creatures, the C++ (0,0) default.
	// The path-end (0,0) reset (WaypointMovementGenerator.cpp:313) stays
	// out: Go paths loop, so the last arrival stands.
	methods["GetCurrentWaypointId"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.LastWaypointID
	})
	// MoveWaypoint is CreatureMethods::MoveWaypoint (CreatureMethods.h:1196-1203):
	// MovePath(GetWaypointPath(), true) via startCreatureWaypoint — the
	// assigned path re-arms from its first node after the DoInitialize 1s
	// hold; pathless or empty paths no-op.
	methods["MoveWaypoint"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.startCreatureWaypoint(ctx, motion)
		return nil, nil
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
	// GetReactState is Creature::GetReactState (UnitDefines.h:
	// REACT_PASSIVE=0, REACT_DEFENSIVE=1, REACT_AGGRESSIVE=2 — matches
	// creatureReact* in creaturemotion.go): the read twin of
	// SetReactState above, for AI branches that test the current state
	// (kelthuzad minion JustEngagedWith). DB-spawned motions carry the
	// template reaction (loadCreatureReaction, combat.go) and scripted
	// SetReactState stamps ReactStateKnown, so the read is reliable for
	// registered handlers; an unset motion reads 0 (Go zero value).
	methods["GetReactState"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		st := motion.ReactState
		s.motionMu.Unlock()
		return st
	})
	// HasLootRecipient is Creature::hasLootRecipient (Creature.h:218):
	// a loot-recipient player or group is set. The tap fields live on
	// the motion (creaturemotion.go:307, the Creature::SetLootRecipient
	// state), and kill.go honors them when granting loot, so this is
	// the guard for the SetLootRecipient leg below.
	methods["HasLootRecipient"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		tapped := motion.TapPlayerGUID != 0 || motion.TapGroupID != 0
		s.motionMu.Unlock()
		return tapped
	})
	// SetLootRecipient is Creature::SetLootRecipient (Creature.cpp:1328-1355,
	// Creature.h:221, withGroup defaults true): nil clears the recipient
	// and sheds LOOTABLE|TAPPED; a player sets TapPlayerGUID plus the
	// player's group when withGroup holds (optional second arg, default
	// true). A non-player arg is a no-op, matching C++'s early return
	// for non-player non-vehicle units (the vehicle charmer-owner chain
	// is unmodeled). Backs boss_valithria_dreamwalker HealReceived.
	methods["SetLootRecipient"] = func(_ context.Context, args []any) ([]any, error) {
		withGroup := true
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				withGroup = b
			}
		}
		if len(args) == 0 || args[0] == nil {
			s.motionMu.Lock()
			motion.TapPlayerGUID, motion.TapGroupID = 0, 0
			motion.DynamicFlags &^= unitDynFlagTapped | unitDynFlagLootable
			dynFlags := motion.DynamicFlags
			mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldDynamicFlags: dynFlags})
			return nil, nil
		}
		playerGUID := luaObjectGUID(args[0])
		if playerGUID == 0 {
			return nil, nil
		}
		sess := s.findSessionByGUID(playerGUID)
		if sess == nil {
			return nil, nil
		}
		s.motionMu.Lock()
		motion.TapPlayerGUID = playerGUID
		if withGroup {
			motion.TapGroupID = sess.groupID
		}
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
	// ApplyDamagePctModifier is Unit::ApplyStatPctModifier
	// (Unit.cpp:9237-9255) for UNIT_MOD_DAMAGE_MAINHAND / TOTAL_PCT:
	// AddPct multiplies m_auraPctModifiersGroup[mod][TOTAL_PCT] by
	// (1+pct/100), and UpdateUnitMod recomputes the weapon-damage
	// base from it. The Zul'Gurub phase-transform hacks ride it:
	// arlokk/marli +35 on transform, thekal +40, each rolled back
	// on transform-back / phase-gated Reset by the inverse
	// (100/(1+pct/100)-100), which restores the group to 1.0.
	// The swing path scales the rolled base by the live group;
	// spell damage is untouched (the C++ mod covers weapon damage
	// only). A zero group means unset and starts at 1.0.
	methods["ApplyDamagePctModifier"] = func(_ context.Context, args []any) ([]any, error) {
		pct, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		group := motion.DamageMainhandPct
		if group == 0 {
			group = 1.0
		}
		motion.DamageMainhandPct = group * (1.0 + float64(pct)/100.0)
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
	// GetNativeDisplayId is Unit::GetNativeDisplayId (Unit.h:1474):
	// the live UNIT_FIELD_NATIVEDISPLAYID, init from the template
	// model at spawn and re-seated by seatCreatureEntry on entry
	// change; a SetNativeDisplayId override replaces it without
	// touching the live display, as C++ SetUInt32Value does.
	methods["GetNativeDisplayId"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		id := motion.NativeDisplayID
		s.motionMu.Unlock()
		return []any{id}, nil
	}
	// SetNativeDisplayId is Unit::SetNativeDisplayId (Unit.h:1476):
	// the native field only, broadcast to the instance. DELTA: the
	// spawn create block still reads spawn.Model for the native
	// field (the pre-existing DisplayIDKnown overlay conflates the
	// two), so late joiners see the spawn model until a values
	// update lands; live observers get the broadcast value.
	methods["SetNativeDisplayId"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.NativeDisplayID = id
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		if s != nil {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNativeDisplayID: id})
		}
		return nil, nil
	}
	// DeMorph is Unit::DeMorph (Unit.cpp:3321-3323): SetDisplayId of
	// the native display, through the shared fold. A zero native
	// (totem/pet motions, which carry no template model) is a
	// silent no-op; the handler surface does not serve pets.
	methods["DeMorph"] = func(ctx context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		native := motion.NativeDisplayID
		s.motionMu.Unlock()
		if native == 0 {
			return nil, nil
		}
		s.setMotionDisplayID(ctx, motion, native)
		return nil, nil
	}
	// GetCreatureType is Unit::GetCreatureType (Unit.cpp:9124-9149)
	// for creatures: the template type (the player shapeshift/race
	// arms live on the player surface).
	methods["GetCreatureType"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		typ := motion.CreatureType
		s.motionMu.Unlock()
		return []any{typ}, nil
	}
	// IsTameable is CreatureTemplate::IsTameable (Creature.cpp): the
	// template must be a beast with a family and the TAMABLE_PET type
	// flag; exotic pets additionally need the caller's
	// CanTameExoticPets flag (Player::CanTameExoticPets, Player.h:1826).
	// The exotic flag arrives as the single bool arg, mirroring the
	// C++ call target->IsTameable(caster->CanTameExoticPets()) — the
	// tame-beast CheckCast twin (spell_hunter.cpp:1446). A missing
	// template row fails closed (not tameable).
	methods["IsTameable"] = func(ctx context.Context, args []any) ([]any, error) {
		exotic := false
		if len(args) >= 1 {
			if b, ok := args[0].(bool); ok {
				exotic = b
			}
		}
		if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{false}, nil
		}
		var ctype, family, typeFlags int64
		if err := s.WorldStore.DB.QueryRowContext(ctx,
			"SELECT COALESCE(type, 0), COALESCE(family, 0), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?", motion.Entry).Scan(&ctype, &family, &typeFlags); err != nil {
			return []any{false}, nil
		}
		return []any{creatureTameable(ctype, family, typeFlags, exotic)}, nil
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
	//
	// SetEntry is Object::SetEntry (Object.h:85): a bare
	// SetUInt32Value(OBJECT_FIELD_ENTRY, entry) — no template reload,
	// no level roll, no stat change, unlike Creature::UpdateEntry
	// (the seatCreatureEntry twin above). The live motion.Entry flips
	// with EntryOverride latched so grid sweeps keep the new entry,
	// and the values-update carries the new entry to clients.
	methods["SetEntry"] = func(ctx context.Context, args []any) ([]any, error) {
		newEntry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.Entry = newEntry
		motion.EntryOverride = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{objectFieldEntry: newEntry})
		return nil, nil
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
		phase := motion.PhaseMask
		transportGUID := motion.TransportGUID
		x, y, z := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, ori, true, phase, transportGUID)
		return nil, nil
	}
	// SetFacing is UnitMethods::SetFacing (UnitMethods.h:1766-1771):
	// SetFacingTo(o) verbatim, so it is the SetFacingTo bridge below
	// with the same in-place turn packet and the same mid-spline
	// race delta (C++ skips the turn when the unit is moving; Go
	// sets the logical orientation and the client packet races).
	methods["SetFacing"] = func(_ context.Context, args []any) ([]any, error) {
		ori, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.Orientation = ori
		guid, mapID, instID := motion.GUID, motion.Map, motion.InstanceID
		phase := motion.PhaseMask
		transportGUID := motion.TransportGUID
		x, y, z := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, ori, true, phase, transportGUID)
		return nil, nil
	}
	// SetFacingToObject is UnitMethods::SetFacingToObject
	// (UnitMethods.h:1778-1783): Unit::SetFacingToObject computes the
	// absolute angle to the target (Unit.cpp:13677, the GetAbsoluteAngle
	// twin: atan2 normalized to [0, 2pi)) and turns in place, so this
	// bridge resolves the target's X/Y through the object fields
	// (creature/player/GO objects all carry them) and rides the same
	// SetFacingTo turn packet. An argument without X/Y fields errors
	// (the Eluna CHECKOBJ leg); the C++ IsStopped gate is not gated,
	// same delta as SetFacingTo.
	methods["SetFacingToObject"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target")
		}
		target, ok := args[0].(*scripting.Object)
		if !ok || target == nil {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target")
		}
		tx, ok := objectFloat32Field(target, "X")
		if !ok {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target with a position")
		}
		ty, ok := objectFloat32Field(target, "Y")
		if !ok {
			return nil, fmt.Errorf("SetFacingToObject requires a WorldObject target with a position")
		}
		s.motionMu.Lock()
		angle := float32(math.Atan2(float64(ty-motion.Y), float64(tx-motion.X)))
		if angle < 0 {
			angle += 2 * math.Pi
		}
		motion.Orientation = angle
		guid, mapID, instID := motion.GUID, motion.Map, motion.InstanceID
		phase := motion.PhaseMask
		transportGUID := motion.TransportGUID
		x, y, z := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, angle, true, phase, transportGUID)
		return nil, nil
	}
	// IsDying is UnitMethods::IsDying (UnitMethods.h:316-324): Unit::isDying
	// (Unit.h:1141) reads m_deathState == JUST_DIED. For creatures that state
	// is unobservable from Lua: Creature::setDeathState (Creature.cpp:
	// 2057-2115) promotes JUST_DIED to CORPSE synchronously inside the same
	// call, before KilledUnit/JustDied hooks fire, so a handler creature
	// always reads false. The player leg (lua_player.go) reads the
	// death-to-release window instead.
	methods["IsDying"] = luaNoArgs(func() any { return false })
	// MoveFleeing is UnitMethods::MoveFleeing (UnitMethods.h:2254-2260):
	// MotionMaster::MoveFleeing(target, time) (MotionMaster.cpp:618-633).
	// A nil/positionless target errors (the Eluna CHECKOBJ leg); a time of 0
	// arms the untimed generator leg, which flees until SetFeared(false)
	// clears it. The C++ generator's continuous repathing is a single 24yd
	// spline here, the critter-flee delta. The player surface stays out:
	// player movement is client-driven and Go has no player flee spline.
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
		s.startCreatureFlee(motion, tx, ty, durationMs)
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
		s.setCreatureFeared(motion, apply)
		return nil, nil
	}
	// MoveTo is UnitMethods::MoveTo (UnitMethods.h:2267-2283) =
	// MotionMaster::MovePoint(id, x, y, z, genPath) (MotionMaster.cpp:635),
	// via the shared script-spline launch. The id is recorded on the
	// motion (ScriptMovePointID/dest) so the prologue landing gate can
	// fire the MovementInform arrival (POINT_MOTION_TYPE, event 28).
	// genPath is accepted and ignored — Go has no PathGenerator, so the
	// spline is straight-line like every other script launch. The player
	// surface stays out (client-driven movement).
	methods["MoveTo"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
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
		s.launchCreatureScriptSpline(motion, x, y, z, false, movementGeneratorPoint, id, true)
		return nil, nil
	}
	// MoveLand is MotionMaster::MoveLand (MotionMaster.cpp:674-684) =
	// a GenericMovementGenerator (EFFECT_MOTION_TYPE 16) with the
	// ground animation tier: the landing twin of MoveTo, so the
	// arrival gate fires MovementInform(EFFECT_MOTION_TYPE, id) on
	// event 28 (creaturemotion.go:5723), exactly as C++ MovementInform
	// receives it. Backs the razorscale LAND->GROUND descent
	// (boss_razorscale.cpp:456-462). The player surface stays out
	// (client-driven movement).
	methods["MoveLand"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
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
		s.launchCreatureScriptSpline(motion, x, y, z, false, movementGeneratorEffect, id, true)
		return nil, nil
	}
	// MoveTakeoff is MotionMaster::MoveTakeoff
	// (MotionMaster.cpp:686-696) = a GenericMovementGenerator
	// (EFFECT_MOTION_TYPE 16) with the hover animation tier: the
	// takeoff twin of MoveTo, so the arrival gate fires
	// MovementInform(EFFECT_MOTION_TYPE, id) on event 28
	// (creaturemotion.go:5723). Backs the razorscale
	// RESUME_AIR_PHASE lift (boss_razorscale.cpp:631-640). The
	// player surface stays out (client-driven movement).
	methods["MoveTakeoff"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
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
		s.launchCreatureScriptSpline(motion, x, y, z, false, movementGeneratorEffect, id, true)
		return nil, nil
	}
	// MoveCharge is MotionMaster::MoveCharge (MotionMaster.cpp:698-720)
	// = a PointMovementGenerator (POINT_MOTION_TYPE 8), the same
	// generator type MovePoint runs, so it shares the script-spline
	// launch and the arrival gate fires MovementInform
	// (POINT_MOTION_TYPE, id) on event 28 (creaturemotion.go:5723),
	// exactly as C++ MovementInform receives it. The C++ charge
	// speed rides launchCreatureScriptSplineSpeed (the spline-chain
	// speed-taking core); the spline itself is straight-line like
	// every other script launch — Go has no PathGenerator. Arg
	// order follows the Lua MoveTo family (id first), not the C++
	// (x, y, z, speed, id) order. Backs the flame leviathan
	// ACTION_MOVE_TO_CENTER_POSITION leg (boss_flame_leviathan.cpp
	// :557-565) and the blood-prince-council kinetic-bomb jump. The
	// player surface stays out (client-driven movement).
	methods["MoveCharge"] = func(_ context.Context, args []any) ([]any, error) {
		id, err := luaUint32Arg(args, 0)
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
		speed, err := luaFloat32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		s.launchCreatureScriptSplineSpeed(motion, x, y, z, false, movementGeneratorPoint, id, true, speed)
		return nil, nil
	}
	// MoveAlongSplineChain is MotionMaster::MoveAlongSplineChain
	// (MotionMaster.cpp:172-194) = the SplineChainMovementGenerator
	// (SplineChainMovementGenerator.cpp): walk the DB chain
	// (entry, chainId) link by link (spline_chain.go), firing
	// MovementInform(SPLINE_CHAIN_MOTION_TYPE 17, pointId) on event
	// 28 at the final landing. The optional 4th arg overrides the
	// lookup entry — the MoveInfiniteOnSpawn twin
	// (npc_arthas.cpp): the infinite chains are keyed by
	// NPC_INFINITE_ADVERSARY whatever spawned them. Returns false
	// when no such chain exists (the C++ error-and-return), so callers
	// can fall back; a running chain is superseded by any fresh
	// non-chain Move* launch or evade and then never informs.
	methods["MoveAlongSplineChain"] = func(ctx context.Context, args []any) ([]any, error) {
		pointID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		chainID, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		walk := false
		if len(args) >= 3 {
			b, ok := args[2].(bool)
			if !ok {
				return nil, fmt.Errorf("MoveAlongSplineChain requires a bool walk flag")
			}
			walk = b
		}
		var entryOverride uint32
		if len(args) >= 4 {
			entryOverride, err = luaUint32Arg(args, 3)
			if err != nil {
				return nil, err
			}
		}
		return []any{s.moveAlongSplineChain(ctx, motion, pointID, uint16(chainID), walk, entryOverride)}, nil
	}
	// MoveRandom is UnitMethods::MoveRandom (UnitMethods.h:2178-2198) =
	// MotionMaster::MoveRandom(radius): a RandomMovementGenerator whose
	// reference is the creature's current position
	// (RandomMovementGenerator.cpp:83) and whose wander distance is the
	// radius (or the template wander distance when 0). Go launches a
	// single walk spline at frand(0, radius) around the current
	// position — the generator's stepping/pause cycle is not repeated,
	// so the tick resumes wander, waypoint, or pursuit once it lands.
	// A radius of 0 falls back to the template wander distance, the
	// DoInitialize arm above.
	methods["MoveRandom"] = func(_ context.Context, args []any) ([]any, error) {
		radius, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if radius == 0 {
			s.motionMu.Lock()
			radius = float32(motion.Wander)
			s.motionMu.Unlock()
		}
		angle := rand.Float64() * 2 * math.Pi
		dist := rand.Float64() * float64(radius)
		s.motionMu.Lock()
		fromX, fromY, fromZ := motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		s.launchCreatureScriptSpline(motion, fromX+float32(dist*math.Cos(angle)), fromY+float32(dist*math.Sin(angle)), fromZ, true, movementGeneratorRandom, 0, false)
		return nil, nil
	}
	// MoveHome is UnitMethods::MoveHome (UnitMethods.h:2199-2206) =
	// MotionMaster::MoveTargetedHome (MotionMaster.cpp:551-573): Clear()
	// then a HomeMovementGenerator, whose finalize fires
	// CREATURE_EVENT_ON_REACH_HOME (24) — the early-gate arm in
	// stepCreatureMotion fires it when the script home spline lands.
	// A charmed or owned creature takes C++'s follow arm instead, so
	// those creatures no-op here (their pet-follow leg already keeps
	// them near the owner).
	// The player surface stays out.
	methods["MoveHome"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		owned := motion.CharmerGUID != 0 || motion.OwnerGUID != 0
		homeX, homeY, homeZ := motion.HomeX, motion.HomeY, motion.HomeZ
		s.motionMu.Unlock()
		if owned {
			return nil, nil
		}
		// MotionMaster::MoveTargetedHome (MotionMaster.cpp:551-573) opens
		// with Clear(): the ACTIVE chase/follow slots drop with the
		// engagement.
		s.clearScriptChaseFollow(motion)
		s.launchCreatureScriptSpline(motion, homeX, homeY, homeZ, false, movementGeneratorHome, 0, true)
		return nil, nil
	}
	// EnterEvadeMode is CreatureAI::EnterEvadeMode (CreatureAI.cpp:294-316,
	// via Eluna::EnterEvadeMode, CreatureHooks.cpp:202) over the
	// triggerCreatureEvade leash twin (creaturemotion.go:4601), which the
	// file already audits leg-by-leg against the C++ body: On_Reset (23)
	// then On_LeaveCombat (2) ahead of the base evade, threat clear,
	// CombatStop, aura clear, health restore, tap reset, cooldown clear,
	// and the home walk firing On_ReachHome (24). The C++ guards ride
	// along: already-evading and dead motions no-op (_EnterEvadeMode
	// returns false on IsInEvadeMode / !IsAlive). No return value,
	// matching the C++ void.
	methods["EnterEvadeMode"] = func(ctx context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		dead := motion.Health == 0
		evading := motion.Evading
		s.motionMu.Unlock()
		if dead || evading {
			return nil, nil
		}
		s.triggerCreatureEvade(ctx, motion, time.Now())
		return nil, nil
	}
	// MoveConfused is UnitMethods::MoveConfused (UnitMethods.h:2240-2248)
	// = MotionMaster::MoveConfused: the ConfusedMovementGenerator
	// starts without the SetControlled side effects (no CastStop, no
	// attack stop, no target clear — those belong to SetConfused).
	// Re-launching while already confused re-arms the hop, the
	// MotionMaster::Add replace arm.
	methods["MoveConfused"] = func(_ context.Context, args []any) ([]any, error) {
		s.motionMu.Lock()
		motion.Confused = true
		s.motionMu.Unlock()
		s.launchCreatureConfusedSpline(motion)
		return nil, nil
	}
	// SetConfused is UnitMethods::SetConfused (UnitMethods.h:1946-1958)
	// for creatures: Unit::SetControlled(apply, UNIT_STATE_CONFUSED)
	// (Unit.cpp:11484-11560) via the setCreatureConfused engine leg.
	// The player surface stays out (client control + no player
	// confused spline).
	methods["SetConfused"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetConfused requires a boolean")
			}
			apply = b
		}
		s.setCreatureConfused(motion, apply)
		return nil, nil
	}
	// MoveChase is UnitMethods::MoveChase (UnitMethods.h:2230-2238) =
	// MotionMaster::MoveChase(target, dist, angle) (MotionMaster.cpp:594-602):
	// an ACTIVE-slot ChaseMovementGenerator (MotionMaster early-returns
	// on a missing target or self, so those no-op here too). The call
	// only arms the slot — ChaseMovementGenerator pauses while
	// HasLostTarget (owner victim != target, ChaseMovementGenerator.cpp:105-113),
	// so a chase to a non-victim, or out of combat, moves nothing; when
	// the chase target IS the combat victim, the tick's pursuit leg
	// honors the ChaseRange(dist) stop and ChaseAngle(angle) near point.
	// MoveFollow clears the slot (same ACTIVE slot). The player surface
	// stays out (client-driven movement).
	methods["MoveChase"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("MoveChase requires a target")
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 || targetGUID == motion.GUID {
			return nil, nil
		}
		var dist, angle float32
		if len(args) >= 2 {
			var err error
			if dist, err = luaFloat32Arg(args, 1); err != nil {
				return nil, err
			}
		}
		if len(args) >= 3 {
			var err error
			if angle, err = luaFloat32Arg(args, 2); err != nil {
				return nil, err
			}
		}
		s.motionMu.Lock()
		motion.ScriptFollowTargetGUID = 0
		motion.ScriptFollowDist = 0
		motion.ScriptFollowAngle = 0
		motion.ScriptChaseTargetGUID = targetGUID
		motion.ScriptChaseDist = dist
		motion.ScriptChaseAngle = angle
		s.motionMu.Unlock()
		return nil, nil
	}
	// MoveFollow is UnitMethods::MoveFollow (UnitMethods.h:2214-2221) =
	// MotionMaster::MoveFollow(target, dist, angle) (MotionMaster.cpp:584-592):
	// an ACTIVE-slot FollowMovementGenerator (same missing-target/self
	// no-op as MoveChase). The slot arms the wander section's follow leg,
	// which repaths to the target's near point while the owner sits
	// outside range + tolerance; in combat the chase leg runs instead.
	// MoveChase clears the slot (same ACTIVE slot). The player surface
	// stays out.
	methods["MoveFollow"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("MoveFollow requires a target")
		}
		targetGUID := luaObjectGUID(args[0])
		if targetGUID == 0 || targetGUID == motion.GUID {
			return nil, nil
		}
		var dist, angle float32
		if len(args) >= 2 {
			var err error
			if dist, err = luaFloat32Arg(args, 1); err != nil {
				return nil, err
			}
		}
		if len(args) >= 3 {
			var err error
			if angle, err = luaFloat32Arg(args, 2); err != nil {
				return nil, err
			}
		}
		s.motionMu.Lock()
		motion.ScriptChaseTargetGUID = 0
		motion.ScriptChaseDist = 0
		motion.ScriptChaseAngle = 0
		motion.ScriptFollowTargetGUID = targetGUID
		motion.ScriptFollowDist = dist
		motion.ScriptFollowAngle = angle
		motion.ScriptFollowInformed = false
		s.motionMu.Unlock()
		return nil, nil
	}
	// MoveJump is UnitMethods::MoveJump (UnitMethods.h:2293-2310) =
	// MotionMaster::MoveJump(pos, speedXY, speedZ, id) (MotionMaster.cpp:780-824):
	// Eluna passes (pos, zSpeed, maxHeight, id) straight through, so the
	// 4th Lua arg drives horizontal speed and the 5th drives the vertical
	// profile — see launchCreatureJumpSpline. The jump takes the ACTIVE
	// slot, clearing any chase/follow slot. The player surface stays out.
	methods["MoveJump"] = func(_ context.Context, args []any) ([]any, error) {
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
		zSpeed, err := luaFloat32Arg(args, 3)
		if err != nil {
			return nil, err
		}
		maxHeight, err := luaFloat32Arg(args, 4)
		if err != nil {
			return nil, err
		}
		var id uint32
		if len(args) >= 6 {
			if id, err = luaUint32Arg(args, 5); err != nil {
				return nil, err
			}
		}
		s.launchCreatureJumpSpline(motion, x, y, z, zSpeed, maxHeight, id)
		return nil, nil
	}
	// GetMovementType is UnitMethods::GetMovementType
	// (UnitMethods.h:1430-1444) = MotionMaster:->
	// GetCurrentMovementGeneratorType, via the creatureMovementType
	// derivation (script stamp in flight, else the live engine legs).
	methods["GetMovementType"] = func(_ context.Context, _ []any) ([]any, error) {
		return []any{s.creatureMovementType(motion)}, nil
	}
	// SetSanctuary is UnitMethods::SetSanctuary (UnitMethods.h:1892-1908)
	// for creatures: the UNIT_BYTE2_FLAG_SANCTUARY (0x08, UnitDefines.h:106)
	// byte flag on UNIT_FIELD_BYTES_2 byte 1 rides the motion PVPFlags
	// slot (the SetSheath precedent: folded into spawn.Bytes2 by
	// applyCreatureMotionToSpawn, no immediate update packet) plus the
	// CombatStop() arm (Unit.cpp:5809, includingCast=false default):
	// own victim dropped with SMSG_ATTACK_STOP, visual target cleared,
	// attackers removed, threat cleared, InCombat false. DELTAS: the
	// CombatStopWithPets leg is unmodeled (no pet attack-stop primitive
	// that does not halt motion), same delta as the player twin; the
	// false leg only clears the flag, no CombatStop, per C++.
	methods["SetSanctuary"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetSanctuary requires a boolean")
			}
			apply = b
		}
		s.motionMu.Lock()
		if apply {
			motion.PVPFlags |= pvpFlagSanctuary
		} else {
			motion.PVPFlags &^= pvpFlagSanctuary
		}
		motion.PVPFlagsKnown = true
		var victim uint64
		var mapID, instanceID uint32
		var guid uint64
		var x, y, z float32
		var phase uint32
		if apply {
			victim = motion.TargetGUID
			motion.TargetGUID = 0
			motion.AlreadyCallAssistance = false
			motion.AssistAt = time.Time{}
			motion.AssistVictim = 0
			motion.Assistants = nil
			motion.InCombat = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			x, y, z = motion.X, motion.Y, motion.Z
			phase = motion.PhaseMask
		}
		s.motionMu.Unlock()
		if apply {
			if victim != 0 {
				stopPkt := buildAttackStop(guid, victim, false)
				s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, mapID, instanceID, x, y, z, phase)
			}
			s.clearCreatureVisualTarget(motion)
			s.removeCreatureAttackers(mapID, instanceID, guid)
			s.removeThreatVictimFromAllLists(mapID, instanceID, guid)
			s.broadcastThreatClearInInstance(mapID, instanceID, x, y, z, guid, phase)
		}
		return nil, nil
	}
	// SetFFA is UnitMethods::SetFFA (UnitMethods.h:1851-1890, TRINITY
	// leg, WotLK only): the UNIT_BYTE2_FLAG_FFA_PVP (0x04,
	// UnitDefines.h:105) byte flag on UNIT_FIELD_BYTES_2 byte 1 rides
	// the motion PVPFlags slot (folded into spawn.Bytes2 by
	// applyCreatureMotionToSpawn, no immediate update packet, the
	// SetSanctuary precedent), plus the controlled-unit fan-out in
	// fanOutBytes2FlagToControlled. The player SetFFA twin is
	// excluded in this fork (PlayerMethods.h:2086, !TRINITY guard);
	// this Unit arm applies to players as Units.
	methods["SetFFA"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetFFA requires a boolean")
			}
			apply = b
		}
		s.motionMu.Lock()
		if apply {
			motion.PVPFlags |= pvpFlagFFA
		} else {
			motion.PVPFlags &^= pvpFlagFFA
		}
		motion.PVPFlagsKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.fanOutBytes2FlagToControlled(mapID, instanceID, guid, pvpFlagFFA, apply)
		return nil, nil
	}
	// SetPvP is UnitMethods::SetPvP (UnitMethods.h:1466-1474): Unit::SetPvP
	// (Unit.cpp:12254) sets or clears only the UNIT_BYTE2_FLAG_PVP
	// (0x01) byte flag on UNIT_FIELD_BYTES_2 byte 1 — no timer, no
	// contested-flag, no controlled-unit arm (those belong to
	// Player::UpdatePvP, a separate path).
	methods["SetPvP"] = func(_ context.Context, args []any) ([]any, error) {
		apply := true
		if len(args) >= 1 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("SetPvP requires a boolean")
			}
			apply = b
		}
		s.motionMu.Lock()
		if apply {
			motion.PVPFlags |= pvpFlagPvP
		} else {
			motion.PVPFlags &^= pvpFlagPvP
		}
		motion.PVPFlagsKnown = true
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetStat is UnitMethods::GetStat (UnitMethods.h:726-734): Unit::GetStat
	// (Unit.h:858) reads UNIT_FIELD_STAT0+stat directly, so this reads
	// the motion stats array — the same source the update values emit.
	// stat >= MAX_STATS (5) reads nil: Eluna returns 1 with nothing
	// pushed, which Go cannot express, so nil is the twin.
	methods["GetStat"] = func(_ context.Context, args []any) ([]any, error) {
		stat, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if stat >= 5 {
			return []any{nil}, nil
		}
		s.motionMu.Lock()
		value := motion.Stats[stat]
		s.motionMu.Unlock()
		return []any{float64(value)}, nil
	}
	// GetCreatorGUID is UnitMethods::GetCreatorGUID (UnitMethods.h:633-638):
	// Unit::GetCreatorGUID (Unit.h:1149) reads UNIT_FIELD_CREATEDBY.
	methods["GetCreatorGUID"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		id := motion.CreatedByGUID
		s.motionMu.Unlock()
		return []any{id}, nil
	}
	// GetSummonerUnit is TempSummon::GetSummonerUnit
	// (TemporarySummon.cpp:48-53): GetSummoner() resolves m_summonerGUID
	// (set from the owner in the TempSummon ctor :38) through
	// ObjectAccessor to a live unit on the map, nil when the summoner
	// is gone or there is none. Go reads the same GUID from
	// UNIT_FIELD_CREATEDBY (motion.CreatedByGUID, stamped by the summon
	// bridge at :139) and resolves it the GetVictim way — an online
	// player first, else a live creature motion on the caller's map and
	// instance — returning nil when neither resolves, the Go form of
	// the C++ nullptr. Callers nil-guard it, matching the C++ sites.
	methods["GetSummonerUnit"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		summonerGUID := motion.CreatedByGUID
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		if summonerGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(summonerGUID); player != nil {
			return []any{player}, nil
		}
		if target := s.findCreatureMotion(mapID, instanceID, summonerGUID); target != nil {
			return []any{s.luaMotionCreature(target)}, nil
		}
		return []any{nil}, nil
	}
	// SetCreatorGUID is UnitMethods::SetCreatorGUID (UnitMethods.h:1790-1797):
	// Unit::SetCreatorGUID (Unit.h:1150) writes UNIT_FIELD_CREATEDBY.
	methods["SetCreatorGUID"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		guid, err := luaUint64(args[0])
		if err != nil {
			return nil, fmt.Errorf("argument 1 must be uint64")
		}
		s.motionMu.Lock()
		motion.CreatedByGUID = guid
		s.motionMu.Unlock()
		return nil, nil
	}
	// SetTempSummonType is TempSummon::SetTempSummonType (TemporarySummon.h):
	// rewrites the summon's despawn family on the 0-7 TempSummonType scheme
	// (0 TIMED_OR_DEAD_DESPAWN ... 7 MANUAL_DESPAWN, lua_creature_events.go
	// SummonCreature comment). The field is read by checkTempSummonExpiry
	// (timed family) and checkDeadTempSummon (types 1/4 unsummon at corpse
	// at once, type 5 runs the corpse timer); setting 4 on a live add makes
	// its corpse vanish at once instead of lingering for the prune. Values
	// above 7 error, mirroring the SummonCreature gate.
	methods["SetTempSummonType"] = func(_ context.Context, args []any) ([]any, error) {
		summonType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if summonType > 7 {
			return nil, fmt.Errorf("SetTempSummonType type must be 0-7")
		}
		s.motionMu.Lock()
		motion.TempSummonType = uint8(summonType)
		s.motionMu.Unlock()
		return nil, nil
	}
	// GetSpeed is UnitMethods::GetSpeed (UnitMethods.h:1392-1404):
	// Unit::GetSpeed (Unit.cpp:8811-8814) = rate * baseMoveSpeed[type].
	// MOVE_WALK=0 reads the motion walk speed (template walk speed,
	// 2.5 fallback), MOVE_RUN=1 the engine run speed (base 7.0 times
	// the creatureRunSpeedRate fold, 7.0 fallback), MOVE_FLIGHT=6 the
	// Creature::Initialize flight speed (1.0 * 7.0, creaturemotion.go:507).
	// Other move types have no Go creature model and error, mirroring
	// the SetSpeedRate rejection.
	methods["GetSpeed"] = func(_ context.Context, args []any) ([]any, error) {
		mt, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		walk, run := motion.Speed, motion.RunSpeed
		s.motionMu.Unlock()
		switch mt {
		case 0:
			if walk <= 0 {
				walk = creatureBaseWalkSpeed
			}
			return []any{walk}, nil
		case 1:
			if run <= 0 {
				run = creatureBaseRunSpeed
			}
			return []any{run}, nil
		case 6:
			return []any{float32(creatureBaseFlightSpeed)}, nil
		default:
			return nil, fmt.Errorf("GetSpeed supports MOVE_WALK=0, MOVE_RUN=1, MOVE_FLIGHT=6 only, got %d", mt)
		}
	}
	// SetSpeed is UnitMethods::SetSpeed (UnitMethods.h:1532-1546) for
	// creatures: the Trinity arm calls SetSpeedRate(type, rate) with
	// the rate form (the forced flag is voided in C++), so this is the
	// SetSpeedRate bridge below with the third argument ignored:
	// MOVE_RUN=1 only, rate <= 0 clears the script override.
	methods["SetSpeed"] = func(_ context.Context, args []any) ([]any, error) {
		mt, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if mt != 1 {
			return nil, fmt.Errorf("SetSpeed supports MOVE_RUN=1 only, got %d", mt)
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
	// SMSG_SPLINE_SET_RUN_SPEED to the creature's visibility set.
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
	// GetMountId is Unit::GetMountID (Unit.h:933): the
	// UNIT_FIELD_MOUNTDISPLAYID read over the live motion Mount slot
	// (Mount/Dismount write it); unmounted creatures read 0.
	methods["GetMountId"] = luaNoArgs(func() any {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return motion.Mount
	})
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
		motion.UnitFlagsKnown = true
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
		motion.UnitFlagsKnown = true
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
	// it), UNIT_FIELD_FLAGS_2 (60) reads motion.UnitFlags2,
	// UNIT_NPC_FLAGS (82) reads motion.NPCFlags, UNIT_DYNAMIC_FLAGS
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
		case unitFieldFlags2:
			has = motion.UnitFlags2&flag != 0
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
	// port with real targeting effect. UNIT_FIELD_FLAGS_2 (field 60) is
	// honored too (see below); UNIT_NPC_FLAGS (field 82) is honored
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
		var flags2 uint32
		flags2Dirty := false
		var dynFlags uint32
		dynDirty := false
		var mapID, instanceID uint32
		var guid uint64
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags |= flags
			motion.UnitFlagsKnown = true
			unitFlags, unitFlagsDirty = motion.UnitFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		} else if field == unitFieldFlags2 {
			// UNIT_FIELD_FLAGS_2 (UpdateFields.h:118: OBJECT_END+0x36 = 60).
			// C++ SetFlag ORs the bits; the values-update broadcast twin
			// mirrors the 59 arm. Backs the falling-dragon feign-death leg
			// (zone_storm_peaks.cpp:55795 sets UNIT_FLAG2_FEIGN_DEATH
			// 0x00000001 here); IsTargetableForAttack reads the same
			// motion.UnitFlags2 mirror, so the flag has real targeting
			// effect.
			s.motionMu.Lock()
			motion.UnitFlags2 |= flags
			flags2, flags2Dirty = motion.UnitFlags2, true
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
			// client corpse/loot gates read. The sweep re-affirms the DB
			// template value every tick, so the write latches
			// DynamicFlagsKnown exactly like the 59/82 arms (the old
			// "no Known flag is needed" claim was wrong — the sweep, not
			// the spawn rebuild, owns the wipe). C++ SetFlag ORs the
			// bits; the values-update broadcast twin mirrors the 59/82
			// arms. Backs script lootable/dead legs such as twin_valkyr
			// JustDied and the opera Romulo/Julianne cross-kill.
			s.motionMu.Lock()
			motion.DynamicFlags |= flags
			motion.DynamicFlagsKnown = true
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
		if flags2Dirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldFlags2: flags2})
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
		var flags2 uint32
		flags2Dirty := false
		var dynFlags uint32
		dynDirty := false
		var mapID, instanceID uint32
		var guid uint64
		if field == 59 {
			s.motionMu.Lock()
			motion.UnitFlags &^= flags
			motion.UnitFlagsKnown = true
			unitFlags, unitFlagsDirty = motion.UnitFlags, true
			mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
			s.motionMu.Unlock()
		} else if field == unitFieldFlags2 {
			// UNIT_FIELD_FLAGS_2 (see SetFlag above): C++ RemoveFlag ANDNs
			// the bits. Backs the falling-dragon feign-death leg.
			s.motionMu.Lock()
			motion.UnitFlags2 &^= flags
			flags2, flags2Dirty = motion.UnitFlags2, true
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
			motion.DynamicFlagsKnown = true
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
		if flags2Dirty {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
				map[int]uint32{unitFieldFlags2: flags2})
		}
		return nil, nil
	}
	// SetImmuneToAll is Unit::SetImmuneToAll (Unit.cpp:8440) via the
	// Creature override (Creature.h:124: keepCombat =
	// HasReactState(REACT_PASSIVE)): set/clear UNIT_FLAG_IMMUNE_TO_PC
	// (0x100) | UNIT_FLAG_IMMUNE_TO_NPC (0x200) on UNIT_FIELD_FLAGS (59),
	// the motion.UnitFlags mirror the SetFlag/RemoveFlag bridges write,
	// with the same values-update broadcast. On apply, a non-passive
	// creature ends all combat (m_combatManager.EndAllCombat) — the
	// ClearInCombat teardown (threat wipe + cross-list removal +
	// broadcast). DELTA: ValidateAttackersAndOwnTarget has no Go model;
	// attackers keep a stale InCombat until their own teardown, the
	// accepted ClearInCombat delta. Backs the legoso sironas-detonate
	// arm (zone_bloodmyst_isle.cpp: SetImmuneToAll(false)).
	methods["SetImmuneToAll"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("apply is required")
		}
		apply, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("apply must be a boolean")
		}
		const immuneFlags = uint32(0x100 | 0x200)
		s.motionMu.Lock()
		if apply {
			motion.UnitFlags |= immuneFlags
		} else {
			motion.UnitFlags &^= immuneFlags
		}
		motion.UnitFlagsKnown = true
		unitFlags := motion.UnitFlags
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
		endCombat := apply && !isCreaturePassive(motion)
		if endCombat {
			motion.InCombat = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldFlags: unitFlags})
		if endCombat {
			s.removeThreatVictimFromAllLists(mapID, instanceID, guid)
			s.broadcastThreatClearInInstance(mapID, instanceID, x, y, z, guid, phase)
		}
		return nil, nil
	}
	// Get/Set*Value are Object::Get/Set*Value (ObjectMethods.h) over
	// the motion slots; see lua_values.go.
	registerIndexedValueMethods(methods, motionIndexedValueBackend(s, motion))
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
		motion.FactionKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldFaction: faction})
		return nil, nil
	}
	// SetLevel is Unit::SetLevel (Unit.cpp:9539): writes UNIT_FIELD_LEVEL;
	// the player group-update leg has no creature analogue, and no stat
	// recompute runs (creature stats come from the template, not the
	// level field). Lets Lua port the JustSummoned level legs (kiljaeden,
	// shirrak, nazan, alar) so summons fight at the summoner's level,
	// which the combat rolls (combat.go) read from motion.Level; the
	// spawn record carries motion.Level (creatures.go), like Faction.
	methods["SetLevel"] = func(_ context.Context, args []any) ([]any, error) {
		level, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if level > 255 {
			return nil, fmt.Errorf("SetLevel requires a level 0-255, got %d", level)
		}
		s.motionMu.Lock()
		motion.Level = level
		motion.LevelKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid,
			map[int]uint32{unitFieldLevel: level})
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
	// FindNearestCreature(entry, dist[, alive]) is
	// WorldObject::FindNearestCreature /
	// GetClosestCreatureWithEntry (ScriptedCreature.h): the nearest
	// creature of entry within dist yards (3D gate with both combat reaches
	// added — the C++ IsWithinDistInMap metric; winner picked by
	// reach-adjusted distance, the C++ GetDistance i_range narrowing) in
	// the same map+instance, excluding self; nil when none. The
	// optional third arg is the C++ alive parameter (default true);
	// false includes dead creatures, the player-surface twin of the
	// demon-broiled-surprise CheckCast leg. Backs the creature-search
	// arms (flame-tsunami lava-blaze buff) that headers documented as
	// bridge-blocked.
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
		alive := luaOptionalBoolArg(args, 2, true)
		srcReach := float64(motion.combatReachOrDefault())
		s.motionMu.Lock()
		var best *creatureMotion
		bestAdj := dist
		for guid, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
			if guid == motion.GUID || m.Entry != entry || (alive && m.Health == 0) {
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
	// map+instance; nil when none. DB spawns plus runtime GOs (see
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
	// Eluna nearest/in-range family (WorldObjectMethods.h):
	// GetNearestPlayer/GetNearestCreature/GetNearestGameObject/
	// GetPlayersInRange/GetCreaturesInRange/GetGameObjectsInRange/
	// GetNearObject/GetNearObjects over the WorldObjectInRangeCheck
	// (see lua_worldobject_range.go). Registered from the creature
	// surface with the live motion as origin.
	for name, fn := range luaRangeMethods(s, func() luaRangeOrigin { return s.luaRangeOriginForMotion(motion) }) {
		methods[name] = fn
	}
	// GetFriendlyUnitsInRange/GetUnfriendlyUnitsInRange are the Unit
	// in-range pair (UnitMethods.h:1255/1296); see
	// luaUnitFriendlyMethods in lua_worldobject_range.go.
	for name, fn := range luaUnitFriendlyMethods(s, func() luaRangeOrigin { return s.luaRangeOriginForMotion(motion) }) {
		methods[name] = fn
	}
	// GetGameObjectsInGrid(entry, dist) is
	// WorldObject::GetGameObjectListWithEntryInGrid (Object.cpp:3220):
	// every gameobject of entry within dist yards (2D metric, no
	// reach additions) in the same map+instance, as a Lua array of
	// gameobject objects (GUID-sorted for determinism). DB-spawn
	// model only (see gameObjectsInGridByEntry). Backs the kalecgos
	// portal-despawn and marli egg-respawn arms.
	methods["GetGameObjectsInGrid"] = func(ctx context.Context, args []any) ([]any, error) {
		entry, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist32, err := luaFloat32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if dist32 < 0 {
			return []any{[]any{}}, nil
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		x, y := motion.X, motion.Y
		s.motionMu.Unlock()
		out := make([]any, 0)
		for _, goObj := range s.gameObjectsInGridByEntry(ctx, mapID, instanceID, x, y, entry, dist32) {
			out = append(out, goObj)
		}
		return []any{out}, nil
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
	// GetPlayersInGrid(dist) is the creature-surface twin of the
	// gameobject bridge (lua_objects.go): every alive player within dist
	// yards (3D, source combat reach + 1.5 player reach added — the
	// GridNotifiers.h AnyPlayerInObjectRangeCheck + Object.cpp
	// _IsWithinDist metric the GO twin uses, with the source reach
	// taken from the creature instead of 0) in the same map+instance,
	// as a Lua array of player objects (GUID-sorted for determinism).
	// Alive = not dead-or-ghost (death.go isDeadOrGhost twin). Backs
	// the midsummer ribbon-pole checkNearbyPlayers
	// (npcs_special.cpp:494-503).
	methods["GetPlayersInGrid"] = func(_ context.Context, args []any) ([]any, error) {
		dist32, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		dist := float64(dist32)
		srcReach := float64(motion.combatReachOrDefault())
		s.sessionsMu.RLock()
		var matches []*session
		for sess := range s.sessions {
			if !sess.worldReady.Load() || sess.player == nil {
				continue
			}
			if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
				continue
			}
			if sess.isDeadOrGhost() {
				continue
			}
			dx := float64(sess.player.X - motion.X)
			dy := float64(sess.player.Y - motion.Y)
			dz := float64(sess.player.Z - motion.Z)
			if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d <= dist+srcReach+1.5 {
				matches = append(matches, sess)
			}
		}
		s.sessionsMu.RUnlock()
		sort.Slice(matches, func(i, j int) bool { return matches[i].playerGUID < matches[j].playerGUID })
		out := make([]any, 0, len(matches))
		for _, sess := range matches {
			if player := sess.luaPlayer(); player != nil {
				out = append(out, player)
			}
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
	// SetCombatReach is Unit::SetFloatValue(UNIT_FIELD_COMBATREACH, v)
	// (Unit.cpp): the raw combat-reach slot write, twin of GetCombatReach
	// above — motion.CombatReach is the live slot the distance, melee
	// range and aggro calcs read (combatReachOrDefault, 1.5 default).
	// Backs the setdemand hunt (felmyst Reset 10, kiljaeden Reset 12);
	// UNIT_FIELD_BOUNDINGRADIUS has no creature motion slot, so those
	// arms stay out.
	methods["SetCombatReach"] = func(_ context.Context, args []any) ([]any, error) {
		reach, err := luaFloat32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		motion.CombatReach = reach
		s.motionMu.Unlock()
		return nil, nil
	}
	// IsWithinMeleeRange is Unit::IsWithinMeleeRange (Unit.h:798) =
	// IsWithinMeleeRangeAt(GetPosition(), obj) (Unit.cpp:599-612): same map
	// and phase, 3D distance <= GetMeleeRange(obj) = max(self reach +
	// target reach + 4.0/3.0, NOMINAL_MELEE_RANGE 5.0) (Unit.cpp:614-618),
	// via the combat.go calcMeleeRange twin. The target resolves through
	// combatTargetByGUID (live position + reach, 1.5 default), so a nil,
	// off-map, or unresolvable target reads false, matching C++'s null /
	// IsInMap false legs; phase itself has no object field and is unmodeled.
	methods["IsWithinMeleeRange"] = func(ctx context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("target is required")
		}
		target, ok := args[0].(*scripting.Object)
		if !ok || target == nil {
			return []any{false}, nil
		}
		guid, _ := objectUint64Field(target, "GUID")
		if guid == 0 {
			return []any{false}, nil
		}
		s.motionMu.Lock()
		mapID, instanceID, sx, sy, sz, selfReach := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.combatReachOrDefault()
		s.motionMu.Unlock()
		resolved, ok := s.combatTargetByGUID(ctx, mapID, instanceID, guid)
		if !ok || resolved.Map != mapID {
			return []any{false}, nil
		}
		dx := float64(sx - resolved.X)
		dy := float64(sy - resolved.Y)
		dz := float64(sz - resolved.Z)
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		return []any{inMeleeThreatRange(selfReach, resolved.CombatReach, dist)}, nil
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
	// SetBossState(bossID, state) bridges InstanceScript::SetBossState
	// (InstanceScript.cpp:340-390) over the live instance boss-state
	// store (instance_boss_states.go): the creature's map/instance
	// pair keys the store, the same way UpdateWorldState above reaches
	// instance-scoped state. Team-less caller: team-flagged spawn
	// rows stay blocked per setInstanceBossState's documented
	// behavior. State is the EncounterState enum (InstanceScript.h:68-76):
	// NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3, SPECIAL 4,
	// TO_BE_DECIDED 5. No return value, matching the C++ void.
	methods["SetBossState"] = func(ctx context.Context, args []any) ([]any, error) {
		bossID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		state, err := luaUint32Arg(args, 1)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		s.setInstanceBossState(ctx, mapID, instanceID, bossID, uint8(state), 0, false)
		return nil, nil
	}
	// GetBossState(bossID) bridges InstanceScript::GetBossState
	// (InstanceScript.h:236-237): the stored state, or TO_BE_DECIDED (5)
	// for a boss never set in this instance — C++-exact, since
	// instanceBossState returns TO_BE_DECIDED for an unset boss.
	methods["GetBossState"] = func(_ context.Context, args []any) ([]any, error) {
		bossID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		return []any{s.instanceBossState(mapID, instanceID, bossID)}, nil
	}
	// SetGuidData(dataID) is the population leg of the instance guid
	// store (instance_guid_store.go): it latches the calling
	// creature's GUID under dataID in its instance, the Lua mirror of
	// the C++ instance script's OnCreatureCreate latching
	// (InstanceScript::AddObject create leg, InstanceScript.cpp:294-297;
	// e.g. instance_icecrown_citadel.cpp:255-261 latches the blood
	// princes). Boss Lua files call it from a CREATURE_EVENT_ON_SPAWN
	// (5) handler, which motionFor fires on first spawn and on
	// respawn. No return value, matching the C++ void assignment.
	methods["SetGuidData"] = func(_ context.Context, args []any) ([]any, error) {
		dataID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.setInstanceGuidData(mapID, instanceID, dataID, guid)
		return nil, nil
	}
	// GetGuidData(dataID) bridges InstanceScript::GetGuidData
	// (InstanceScript.cpp:108-113) over the instance guid store: the
	// stored GUID, or 0 for an unlatched type — the Go form of
	// ObjectGuid::Empty.
	methods["GetGuidData"] = func(_ context.Context, args []any) ([]any, error) {
		dataID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		return []any{s.instanceGuidData(mapID, instanceID, dataID)}, nil
	}
	// GetGuidDataLow(dataID) is the numeric twin of GetGuidData for
	// MovementInform id comparisons: the low 32 bits of the stored
	// GUID as a Lua number — the C++ ObjectGuid::GetCounter() mirror
	// (boss_bronjahm.cpp:247, boss_ichoron.cpp:277 compare it against
	// the uint32 id the Go engine passes to ev28). A plain
	// GetGuidData(...) % 2^32 cannot work: the stored GUID arrives as
	// UInt64 userdata whose metatable defines no __mod, so the modulo
	// raises and the handler aborts.
	methods["GetGuidDataLow"] = func(_ context.Context, args []any) ([]any, error) {
		dataID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		return []any{uint32(s.instanceGuidData(mapID, instanceID, dataID))}, nil
	}
	// GetInstanceCreature(dataID) bridges InstanceScript::GetCreature
	// (InstanceScript.cpp:117-120) over the instance guid store: the
	// creature object for the stored GUID, or nil when the type is
	// unlatched or the creature is not on the map — the Go form of
	// the C++ nullptr. Callers nil-guard it, matching the C++ sites
	// (e.g. boss_illidari_council.cpp:157-160).
	methods["GetInstanceCreature"] = func(_ context.Context, args []any) ([]any, error) {
		dataID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		target := s.instanceCreatureByGuidData(mapID, instanceID, dataID)
		if target == nil {
			return []any{nil}, nil
		}
		return []any{s.luaMotionCreature(target)}, nil
	}
	// SendEncounterUnit(frameType, unit, param1, param2) bridges
	// InstanceScript::SendEncounterUnit (InstanceScript.cpp:703-736)
	// over sendEncounterUnit (instance_encounter_frame.go): the
	// encounter-frame packet for the creature's instance. frameType is
	// the EncounterFrame enum (InstanceScript.h:57-64): ENGAGE 0,
	// DISENGAGE 1, UPDATE_PRIORITY 2, ADD_TIMER 3, ENABLE_OBJECTIVE 4,
	// UPDATE_OBJECTIVE 5, DISABLE_OBJECTIVE 6, UNK7 7. unit is the
	// frame's creature — a Lua creature/player object or a bare GUID —
	// and defaults to the calling creature (the `me` at nearly every
	// C++ site); an explicit nil unit sends nothing for the
	// GUID-carrying types, C++-exact (the C++ early return). param1
	// and param2 are optional uint8s defaulting to 0; the sender
	// writes only the bytes each frame type carries. No return value,
	// matching the C++ void.
	methods["SendEncounterUnit"] = func(_ context.Context, args []any) ([]any, error) {
		frameType, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		unitGUID := motion.GUID
		if len(args) > 1 && args[1] != nil {
			unitGUID = luaObjectGUID(args[1])
		}
		var param1, param2 uint8
		if len(args) > 2 {
			p1, err := luaUint32Arg(args, 2)
			if err != nil {
				return nil, err
			}
			param1 = uint8(p1)
		}
		if len(args) > 3 {
			p2, err := luaUint32Arg(args, 3)
			if err != nil {
				return nil, err
			}
			param2 = uint8(p2)
		}
		s.motionMu.Lock()
		mapID, instanceID := motion.Map, motion.InstanceID
		s.motionMu.Unlock()
		s.sendEncounterUnit(mapID, instanceID, unitGUID, frameType, param1, param2)
		return nil, nil
	}
	// HasQuest is CreatureMethods::HasQuest (CreatureMethods.h):
	// Creature::hasQuest (Creature.cpp:1851-1854) = the creature-quest
	// starter relation (ObjectMgr.cpp:8439 loads creature_queststarter
	// into _creatureQuestRelations), NOT starter-or-ender: the Go
	// session helper creatureHasQuest unions both, so this bridge
	// queries the starter table only. No world DB reads false.
	methods["HasQuest"] = func(ctx context.Context, args []any) ([]any, error) {
		questID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{false}, nil
		}
		s.motionMu.Lock()
		entry := motion.Entry
		s.motionMu.Unlock()
		return []any{questRelationExists(ctx, s.WorldStore.DB, "creature_queststarter", entry, questID)}, nil
	}
	// CanCompleteQuest is CreatureMethods::CanCompleteQuest
	// (CreatureMethods.h): Creature::hasInvolvedQuest
	// (Creature.cpp:1856-1859) = the creature-quest ender relation
	// (ObjectMgr.cpp:8453 loads creature_questender into
	// _creatureQuestInvolvedRelations), despite the Eluna name
	// suggesting completion in general. No world DB reads false.
	methods["CanCompleteQuest"] = func(ctx context.Context, args []any) ([]any, error) {
		questID, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if s.WorldStore == nil || s.WorldStore.DB == nil {
			return []any{false}, nil
		}
		s.motionMu.Lock()
		entry := motion.Entry
		s.motionMu.Unlock()
		return []any{questRelationExists(ctx, s.WorldStore.DB, "creature_questender", entry, questID)}, nil
	}
	// CanFly is Creature::CanFly (Creature.h:106) =
	// GetMovementTemplate().IsFlightAllowed() || IsFlying(): the
	// spawn-seeded template flag (motion.CanFly) or the live
	// MOVEMENTFLAG_CAN_FLY flag (motion.Flying, the SetCanFly slot).
	methods["CanFly"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		defer s.motionMu.Unlock()
		return []any{motion.CanFly || motion.Flying}, nil
	}
	// IsInAccessiblePlaceFor is Unit::IsInAccessiblePlaceFor
	// (UnitMethods.h:104, Unit.cpp:3268): receiver in water ? target
	// CanSwim : (target CanWalk || target CanFly), over the live
	// motion coords under motionMu like the IsInWater twin. The
	// target resolves through accessiblePlaceTarget (CHECKOBJ<Creature>
	// twin: a player, bare-number, or unresolvable target errors, and
	// a missing arg errors); the receiver's own swim/walk flags are
	// never consulted for the target.
	methods["IsInAccessiblePlaceFor"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("IsInAccessiblePlaceFor requires a Creature target")
		}
		s.motionMu.Lock()
		mapID, instanceID, x, y, z := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z
		s.motionMu.Unlock()
		swim, walk, fly, err := s.accessiblePlaceTarget(args[0], mapID, instanceID)
		if err != nil {
			return nil, err
		}
		if s.mapIsInWater(mapID, x, y, z) {
			return []any{swim}, nil
		}
		return []any{walk || fly}, nil
	}
	// SetInCombatWithZone is CreatureMethods::SetInCombatWithZone
	// (CreatureMethods.h:1043-1052): under TRINITY it is exactly
	// AI()->DoZoneInCombat(), the DoZoneInCombat bridge above, so
	// this is that method under its Eluna name.
	methods["SetInCombatWithZone"] = methods["DoZoneInCombat"]
	// AttackStart is CreatureMethods::AttackStart
	// (CreatureMethods.h): AI()->AttackStart(target), the engage
	// funnel the EngageWithTarget bridge above implements, so this
	// is that method under its Eluna name, sharing its creature-victim
	// resolution and evading/passive gates.
	methods["AttackStart"] = methods["EngageWithTarget"]
	// DespawnOrUnsummon is CreatureMethods::DespawnOrUnsummon
	// (CreatureMethods.h): Creature::DespawnOrUnsummon
	// (Creature.cpp) = TempSummon UnSummon, else ForcedDespawn.
	// The motion Despawn bridge already implements that split
	// (temp-summon event-20 leg, respawn scheduling), so ms == 0
	// calls it directly and ms > 0 schedules it via time.AfterFunc,
	// the ForcedDespawnDelayEvent leg (ForcedDespawn with a delay
	// queues the same delete). Eluna takes no forceRespawnTimer
	// argument, so the C++ default of 0 applies.
	methods["DespawnOrUnsummon"] = func(ctx context.Context, args []any) ([]any, error) {
		var ms uint32
		if len(args) > 0 {
			// Eluna defaults msTimeToDespawn to 0 only when omitted;
			// a present but mistyped argument errors, as CHECKVAL does.
			var err error
			ms, err = luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
		}
		despawn := methods["Despawn"]
		if ms == 0 {
			return despawn(ctx, nil)
		}
		time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
			_, _ = despawn(context.Background(), nil)
		})
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
	motion.NativeDisplayID = model
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
	motion.MoveGround = stats.MoveGround
	motion.MoveSwim = stats.MoveSwim
	motion.MoveKnown = true
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

// fireServerDeleteCreature fires Eluna WORLD_EVENT_ON_DELETE_CREATURE (31)
// for a permanently deleted creature: Map::AddObjectToRemoveList
// (Map.cpp:3545-3553, args (event, creature)) queues the delete, and the
// remove-list processing that follows reaches Creature::RemoveFromWorld
// (Map.cpp:3636 RemoveFromMap -> Creature.cpp:314, creature event 37) —
// so a delete-queued creature fires 31 at queue time and 37 when it
// leaves the world, in that order. Go deletes synchronously, so this
// fires both at the deletion point, 31 then 37, from the motion snapshot
// (valid after the map deletion). Callers are the verified
// AddObjectToRemoveList legs only — never the prune path (the
// RemoveFromWorld twin, ev37 only), the evade hide path, or the
// transport-unload registry cleanup (C++ twin unverified).
func (s *Server) fireServerDeleteCreature(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	if _, err := s.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventDeleteCreature, creature); err != nil {
		s.debug("lua server delete creature event failed", "event", scripting.ServerEventDeleteCreature, "error", err)
	}
	// RemoveFromWorld leg of the same delete: the remove-list twin above.
	// The veto return is discarded (CallAllFunctions, CreatureHooks.cpp).
	s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnRemove)
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
// creature object, the healer player object when the healer is a player
// (resolved from healerGUID), or nil when neither resolves. Fires only for entry handlers; unique-handler
// dispatch is unnecessary here because no per-GUID heal consumer exists.
func (s *Server) fireCreatureHealReceived(ctx context.Context, victim, healer *creatureMotion, healerGUID uint64, heal uint32) {
	if s == nil || victim == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	var healerObj any
	if healer != nil {
		healerObj = s.luaMotionCreature(healer)
	} else if healerGUID != 0 {
		healerObj = s.findPlayer(healerGUID)
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

// fireCreatureMoveInLOSCreature is the creature-victim twin of
// fireCreatureMoveInLOS: same entry+unique passes and veto semantics,
// with who built from the victim motion (luaMotionCreature) instead of a
// session's luaPlayer. C++ CreatureUnitRelocationWorker passes any unit to
// Eluna MIOS, so a scanner sighting a creature victim fires the same hook
// with a creature who. Placement stays the engage branch, mirroring the
// player scan; passive scanners never reach it.
func (s *Server) fireCreatureMoveInLOSCreature(ctx context.Context, motion, victim *creatureMotion) bool {
	if s == nil || motion == nil || victim == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	creature := s.luaMotionCreature(motion)
	who := s.luaMotionCreature(victim)
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

// fireCreatureDiedByCreature is the server-side twin of session.fireCreatureDied
// for creatures killed outside the session funnel (inline creature-spell,
// damage-shield/split, reflect self-trip kills): C++ (Eluna::JustDied,
// CreatureHooks.cpp:143-152) fires On_Reset (23) then ON_DIED (4) with
// (event, creature, killer), and SummonedCreatureDies (21) on the summoner
// when the victim is a temp summon. The session twin pushes s.luaPlayer()
// as killer because every funneled killer is player-owned; here the killer
// is the creature motion, or nil when the attacker motion is gone.
func (s *Server) fireCreatureDiedByCreature(ctx context.Context, motion, killer *creatureMotion) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	var killerObj *scripting.Object
	if killer != nil {
		killerObj = s.luaMotionCreature(killer)
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	diedArgs := []any{scripting.CreatureEventOnDied, creature, killerObj}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDied, diedArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDied, diedArgs...)
	if motion.OwnerGUID != 0 {
		if summoner := s.findCreatureMotion(motion.Map, motion.InstanceID, motion.OwnerGUID); summoner != nil {
			if summonerObj := s.luaMotionCreature(summoner); summonerObj != nil {
				summonDiedArgs := []any{scripting.CreatureEventOnSummonedCreatureDied, summonerObj, creature, killerObj}
				_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, summoner.Entry, scripting.CreatureEventOnSummonedCreatureDied, summonDiedArgs...)
				_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, summoner.GUID, summoner.InstanceID, scripting.CreatureEventOnSummonedCreatureDied, summonDiedArgs...)
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
	// Creature::Update JustAppeared tick (Creature.cpp:699-704): the
	// compat-mode vehicleKit Reset() re-applies ApplyAllImmunities
	// before AI()->JustAppeared() — the On_Reset/OnSpawn pair below.
	// applyVehicleImmunities documents why the re-apply is ungated.
	s.applyVehicleImmunities(motion)
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

// fireCreatureSpellClick dispatches the Go-side CREATURE_EVENT_ON_SPELL_CLICK (39)
// hook on the clicked creature. C++ (Unit::HandleSpellClick, Unit.cpp:12982-13070)
// calls CreatureAI::OnSpellClick(clicker, spellClickHandled) after the click-spell
// rows, with handled true when any row cast; Eluna has no spell-click hook, so this
// is a Go-side extension on the free slot 39 (the DoAction-38 precedent), fired
// from the spell-click funnel with (event, creature, clicker, handled). C++
// OnSpellClick returns void, so returns are discarded. Only creatures fire it:
// pets resolve no Creature (Unit.cpp ToCreature is null for pets), matching the
// C++ `creature && IsAIEnabled` gate. Must be called with motionMu released:
// the Lua handlers' object methods lock it on demand.
func (s *Server) fireCreatureSpellClick(ctx context.Context, motion *creatureMotion, clicker any, handled bool) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	args := []any{scripting.CreatureEventOnSpellClick, creature, clicker, handled}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnSpellClick, args...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnSpellClick, args...)
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
// chain C++ Eluna-driven pets take, and from summonTempCreature for
// creature/player summons (the C++ summon path calls IsSummonedBy on the
// new AI for every summon). Must be called with motionMu released:
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
		// Script numbers arrive as float64 via luaValue, so a bare
		// uint32 assertion always misses and HasFlag used to read false.
		flag, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		return []any{uint32(flags)&flag != 0}, nil
	}
	methods["IsDaily"] = luaNoArgs(func() any { return uint32(flags)&0x1000 != 0 })
	// IsRepeatable is Quest::IsRepeatable (QuestDef.h:284): SpecialFlags &
	// QUEST_SPECIAL_FLAGS_REPEATABLE (0x001) from quest_template_addon, not
	// the quest_template Flags daily/weekly bits.
	methods["IsRepeatable"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s == nil {
			return []any{false}, nil
		}
		return []any{s.questIsRepeatable(ctx, questID)}, nil
	}
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
