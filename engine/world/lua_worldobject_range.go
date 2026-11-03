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

// Eluna WorldObject nearest/in-range family (WorldObjectMethods.h
// GetNearestPlayer/GetNearestCreature/GetNearestGameObject/
// GetPlayersInRange/GetCreaturesInRange/GetGameObjectsInRange/
// GetNearObject/GetNearObjects) over the WorldObjectInRangeCheck
// (ElunaUtility.cpp:49-111) semantics:
//
//   - Defaults: range SIZE_OF_GRIDS (533.33333); player/creature legs
//     take dead = 1 (alive only), gameobject legs take dead = 0 (the
//     checker ctor default — GO methods pass no dead arg), near legs
//     take dead = 1. hostile: 0 both, 1 hostile, 2 friendly; dead:
//     0 both, 1 alive, 2 dead (any other value behaves like 2, the
//     C++ (dead == 1) != IsAlive() comparison).
//   - Self is excluded (the checker GUID gate); the C++ grid lists
//     have no self exclusion, but this checker does.
//   - Gate is IsWithinDistInMap (Object.cpp:1238): same map, phase
//     overlap, and 3D distance <= range + own combat reach + target
//     combat reach. The nearest winner is the reach-adjusted nearest
//     (GetDistance, Object.cpp:1169: raw minus both reaches, floored
//     at 0), the checker's i_range narrowing.
//   - Dead/hostile gates apply to the candidate's unit: for a
//     gameobject candidate that is its owner (GO::GetOwner), and
//     when a GO has no owner both gates are skipped (the C++
//     `if (target)` guard) — an ownerless GO passes dead/hostile
//     filters regardless.
//   - A gameobject origin with no owner has no unit in C++ either;
//     there the checker falls back to faction template 14. Go has
//     no template-14 model, so an ownerless GO origin reads
//     not-hostile (hostile = 1 excludes, hostile = 2 includes).
//   - Phase masks normalize 0 to PHASEMASK_NORMAL (1), the in-tree
//     convention for unset spawn phases; the gate is mask overlap.
//   - List order is GUID-sorted for determinism (the C++ grid-visit
//     order is unspecified); nearest ties keep the first GUID-sorted
//     candidate (C++ LastSearcher keeps the last grid visit).
//   - Candidates are players (live sessions), creature motions, and
//     gameobjects (DB spawns plus runtime/instance GOs, hidden ones
//     skipped, the nearestGameObjectByEntry model). Corpses,
//     dynamic objects and other types have no Go object model, so a
//     GetNearObject type mask naming only those matches nothing.
//   - A negative range matches nothing (the Go twin convention;
//     C++ would compare against a negative bound).
//   - Zero Lua scripts called this family before this tick —
//     engine-correctness, same class as the prior event axes.

const luaRangeDefaultRange = 533.33333

const (
	luaRangeTypeUnit       = 0x0008
	luaRangeTypePlayer     = 0x0010
	luaRangeTypeGameObject = 0x0020
)

type luaRangeKind int

const (
	luaRangeKindPlayer luaRangeKind = iota
	luaRangeKindCreature
	luaRangeKindGameObject
)

// luaRangeUnit is a resolved unit for the dead/hostile gates: either
// side of the IsHostileTo comparison, or a candidate's own unit.
type luaRangeUnit struct {
	isPlayer bool
	sess     *session
	motion   *creatureMotion
	faction  uint32
	alive    bool
}

type luaRangeOrigin struct {
	srv      *Server
	mapID    uint32
	instance uint32
	x, y, z  float32
	reach    float32
	phase    uint32
	selfGUID uint64
	// unit is the origin's unit for hostility, or nil for an
	// ownerless gameobject origin (reads not-hostile).
	unit *luaRangeUnit
}

type luaRangeCandidate struct {
	kind    luaRangeKind
	guid    uint64
	entry   uint32
	x, y, z float32
	reach   float32
	phase   uint32
	sess    *session
	motion  *creatureMotion
	goState luaGameObjectState
	owner   uint64
}

func luaRangePhase(mask uint32) uint32 {
	if mask == 0 {
		return 1
	}
	return mask
}

func (srv *Server) luaRangeOriginForMotion(motion *creatureMotion) luaRangeOrigin {
	origin := luaRangeOrigin{srv: srv, reach: 1.5}
	if srv == nil || motion == nil {
		return origin
	}
	srv.motionMu.Lock()
	origin.mapID, origin.instance = motion.Map, motion.InstanceID
	origin.x, origin.y, origin.z = motion.X, motion.Y, motion.Z
	origin.reach = float32(motion.combatReachOrDefault())
	origin.phase = luaRangePhase(motion.PhaseMask)
	origin.selfGUID = motion.GUID
	origin.unit = &luaRangeUnit{motion: motion, faction: motion.Faction, alive: motion.Health > 0}
	srv.motionMu.Unlock()
	return origin
}

func (s *session) luaRangeOriginForPlayer() luaRangeOrigin {
	origin := luaRangeOrigin{reach: 1.5}
	if s == nil || s.player == nil {
		return origin
	}
	origin.srv = s.server
	origin.mapID, origin.instance = s.player.Map, s.player.InstanceID
	origin.x, origin.y, origin.z = s.player.X, s.player.Y, s.player.Z
	if s.player.CombatReach > 0 {
		origin.reach = s.player.CombatReach
	}
	origin.phase = luaRangePhase(s.currentPlayerPhaseMask())
	origin.selfGUID = s.playerGUID
	origin.unit = &luaRangeUnit{isPlayer: true, sess: s, faction: s.effectiveFactionTemplate(s.player.Race), alive: !s.isDeadOrGhost()}
	return origin
}

func (srv *Server) luaRangeOriginForGameObject(state luaGameObjectState) luaRangeOrigin {
	origin := luaRangeOrigin{srv: srv, mapID: state.Map, instance: state.InstanceID, x: state.X, y: state.Y, z: state.Z, phase: luaRangePhase(state.PhaseMask), selfGUID: state.GUID}
	if srv == nil {
		return origin
	}
	if sess := srv.findSessionByGUID(state.OwnerGUID); state.OwnerGUID != 0 && sess != nil && sess.player != nil {
		origin.unit = &luaRangeUnit{isPlayer: true, sess: sess, faction: sess.effectiveFactionTemplate(sess.player.Race), alive: !sess.isDeadOrGhost()}
	} else if motion := srv.findCreatureMotion(state.Map, state.InstanceID, state.OwnerGUID); state.OwnerGUID != 0 && motion != nil {
		origin.unit = &luaRangeUnit{motion: motion, faction: motion.Faction, alive: motion.Health > 0}
	}
	return origin
}

// luaRangeUnitFor resolves a candidate's gate unit: players and
// creatures are their own unit; a gameobject's unit is its owner,
// or nothing when it has no resolvable owner.
func (srv *Server) luaRangeUnitFor(cand luaRangeCandidate) (luaRangeUnit, bool) {
	switch cand.kind {
	case luaRangeKindPlayer:
		if cand.sess == nil || cand.sess.player == nil {
			return luaRangeUnit{}, false
		}
		return luaRangeUnit{isPlayer: true, sess: cand.sess, faction: cand.sess.effectiveFactionTemplate(cand.sess.player.Race), alive: !cand.sess.isDeadOrGhost()}, true
	case luaRangeKindCreature:
		if cand.motion == nil {
			return luaRangeUnit{}, false
		}
		return luaRangeUnit{motion: cand.motion, faction: cand.motion.Faction, alive: cand.motion.Health > 0}, true
	default:
		if cand.owner == 0 {
			return luaRangeUnit{}, false
		}
		if sess := srv.findSessionByGUID(cand.owner); sess != nil && sess.player != nil {
			return luaRangeUnit{isPlayer: true, sess: sess, faction: sess.effectiveFactionTemplate(sess.player.Race), alive: !sess.isDeadOrGhost()}, true
		}
		if motion := srv.findCreatureMotion(cand.goState.Map, cand.goState.InstanceID, cand.owner); motion != nil {
			return luaRangeUnit{motion: motion, faction: motion.Faction, alive: motion.Health > 0}, true
		}
		return luaRangeUnit{}, false
	}
}

// luaRangeHostileTo mirrors WorldObject::IsHostileTo between the
// origin unit and a candidate unit. An ownerless GO origin reads
// not-hostile (see the file header for the template-14 delta).
func (srv *Server) luaRangeHostileTo(origin, target luaRangeUnit) bool {
	if origin.motion != nil {
		if target.isPlayer {
			return srv.creatureHostileToPlayer(origin.motion, target.sess)
		}
		return srv.creatureHostileToCreature(origin.motion, target.motion)
	}
	if origin.isPlayer {
		return srv.playerHostileToUnit(origin.sess, target.faction)
	}
	return false
}

func (origin luaRangeOrigin) passes(srv *Server, cand luaRangeCandidate, rangeVal float64, entry, hostile, dead uint32, typeMask uint16) (float64, bool) {
	if cand.guid == 0 || cand.guid == origin.selfGUID {
		return 0, false
	}
	if typeMask != 0 {
		var bits uint16
		switch cand.kind {
		case luaRangeKindPlayer:
			bits = luaRangeTypeUnit | luaRangeTypePlayer
		case luaRangeKindCreature:
			bits = luaRangeTypeUnit
		default:
			bits = luaRangeTypeGameObject
		}
		if typeMask&bits == 0 {
			return 0, false
		}
	}
	if entry != 0 && cand.entry != entry {
		return 0, false
	}
	if luaRangePhase(origin.phase)&luaRangePhase(cand.phase) == 0 {
		return 0, false
	}
	dx := float64(cand.x - origin.x)
	dy := float64(cand.y - origin.y)
	dz := float64(cand.z - origin.z)
	raw := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if raw > rangeVal+float64(origin.reach)+float64(cand.reach) {
		return 0, false
	}
	adjusted := raw - float64(origin.reach) - float64(cand.reach)
	if adjusted < 0 {
		adjusted = 0
	}
	if target, ok := srv.luaRangeUnitFor(cand); ok {
		if dead != 0 && target.alive != (dead == 1) {
			return 0, false
		}
		if hostile != 0 && origin.unit != nil && (hostile == 1) != srv.luaRangeHostileTo(*origin.unit, target) {
			return 0, false
		}
		if hostile != 0 && origin.unit == nil {
			// Ownerless GO origin reads not-hostile: hostile = 1
			// excludes, hostile = 2 includes.
			if hostile == 1 {
				return 0, false
			}
		}
	}
	return adjusted, true
}

func (srv *Server) luaRangePlayers(origin luaRangeOrigin) []luaRangeCandidate {
	if srv == nil {
		return nil
	}
	srv.sessionsMu.RLock()
	var out []luaRangeCandidate
	for sess := range srv.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.Map != origin.mapID || sess.player.InstanceID != origin.instance {
			continue
		}
		reach := float32(1.5)
		if sess.player.CombatReach > 0 {
			reach = sess.player.CombatReach
		}
		out = append(out, luaRangeCandidate{kind: luaRangeKindPlayer, guid: sess.playerGUID, x: sess.player.X, y: sess.player.Y, z: sess.player.Z, reach: reach, sess: sess})
	}
	srv.sessionsMu.RUnlock()
	for i := range out {
		out[i].phase = luaRangePhase(out[i].sess.currentPlayerPhaseMask())
	}
	return out
}

func (srv *Server) luaRangeCreatures(origin luaRangeOrigin) []luaRangeCandidate {
	if srv == nil {
		return nil
	}
	srv.motionMu.Lock()
	var out []luaRangeCandidate
	for _, m := range srv.motionMapLocked(origin.mapID, origin.instance) {
		if m == nil {
			continue
		}
		out = append(out, luaRangeCandidate{kind: luaRangeKindCreature, guid: m.GUID, entry: m.Entry, x: m.X, y: m.Y, z: m.Z, reach: float32(m.combatReachOrDefault()), phase: luaRangePhase(m.PhaseMask), motion: m})
	}
	srv.motionMu.Unlock()
	return out
}

// luaRangeGameObjects collects DB spawns plus runtime/instance GOs
// inside the range bounding box (a 3D-range candidate is always
// inside the x/y box), instance-hidden ones skipped.
func (srv *Server) luaRangeGameObjects(ctx context.Context, origin luaRangeOrigin, rangeVal float32) []luaRangeCandidate {
	if srv == nil {
		return nil
	}
	var out []luaRangeCandidate
	rows, err := srv.WorldStore.DB.QueryContext(ctx, "SELECT g.guid, g.id, t.displayId, t.name, g.position_x, g.position_y, g.position_z, g.orientation, COALESCE(g.phaseMask, 0) FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id WHERE g.map = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ? ORDER BY g.guid", origin.mapID, float64(origin.x-rangeVal), float64(origin.x+rangeVal), float64(origin.y-rangeVal), float64(origin.y+rangeVal))
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var state luaGameObjectState
			if err := rows.Scan(&state.LowGUID, &state.Entry, &state.DisplayID, &state.Name, &state.X, &state.Y, &state.Z, &state.Orientation, &state.PhaseMask); err != nil {
				continue
			}
			state.Map, state.InstanceID = origin.mapID, origin.instance
			state.GoState, state.LootState = 1, 1
			state.GUID = gameObjectGUID(state.LowGUID, state.Entry)
			if srv.isGameObjectHiddenInInstance(state.Map, state.InstanceID, state.GUID) {
				continue
			}
			out = append(out, luaRangeCandidate{kind: luaRangeKindGameObject, guid: state.GUID, entry: state.Entry, x: state.X, y: state.Y, z: state.Z, phase: luaRangePhase(state.PhaseMask), goState: state})
		}
	}
	srv.objectsMu.RLock()
	var dyn []luaRangeCandidate
	// The DB leg above already emitted every static spawn in range;
	// a materialized static GO (live record from a SetGoState or a
	// getOrLoad path) must not appear a second time via the dyn leg.
	seen := make(map[uint64]struct{}, len(out))
	for _, cand := range out {
		seen[cand.guid] = struct{}{}
	}
	collectDyn := func(d *dynamicGameObjectState) {
		if d == nil || d.Map != origin.mapID || d.InstanceID != origin.instance {
			return
		}
		if _, dup := seen[d.GUID]; dup {
			return
		}
		if d.X < origin.x-rangeVal || d.X > origin.x+rangeVal || d.Y < origin.y-rangeVal || d.Y > origin.y+rangeVal {
			return
		}
		state := luaGameObjectState{GUID: d.GUID, LowGUID: d.LowGUID, Entry: d.Entry, OwnerGUID: d.OwnerGUID, DisplayID: d.DisplayID, Map: d.Map, InstanceID: d.InstanceID, X: d.X, Y: d.Y, Z: d.Z, Orientation: d.Orientation, Type: uint32(d.Type), GoState: uint32(d.State), LootState: 1, PhaseMask: d.PhaseMask}
		dyn = append(dyn, luaRangeCandidate{kind: luaRangeKindGameObject, guid: d.GUID, entry: d.Entry, x: d.X, y: d.Y, z: d.Z, phase: luaRangePhase(d.PhaseMask), goState: state, owner: d.OwnerGUID})
	}
	for _, d := range srv.dynamicGameObjects {
		collectDyn(d)
	}
	for _, d := range srv.instanceGameObjects[instanceAdmissionKey{MapID: origin.mapID, InstanceID: origin.instance}] {
		collectDyn(d)
	}
	srv.objectsMu.RUnlock()
	for _, cand := range dyn {
		if srv.isGameObjectHiddenInInstance(cand.goState.Map, cand.goState.InstanceID, cand.goState.GUID) {
			continue
		}
		out = append(out, cand)
	}
	return out
}

func (srv *Server) luaRangeCollect(ctx context.Context, origin luaRangeOrigin, kinds []luaRangeKind, rangeVal float32) []luaRangeCandidate {
	var out []luaRangeCandidate
	for _, kind := range kinds {
		switch kind {
		case luaRangeKindPlayer:
			out = append(out, srv.luaRangePlayers(origin)...)
		case luaRangeKindCreature:
			out = append(out, srv.luaRangeCreatures(origin)...)
		case luaRangeKindGameObject:
			out = append(out, srv.luaRangeGameObjects(ctx, origin, rangeVal)...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].guid < out[j].guid })
	return out
}

func (srv *Server) luaRangeObject(cand luaRangeCandidate) *scripting.Object {
	switch cand.kind {
	case luaRangeKindPlayer:
		if cand.sess == nil {
			return nil
		}
		return cand.sess.luaPlayer()
	case luaRangeKindCreature:
		return srv.luaMotionCreature(cand.motion)
	default:
		return srv.luaGameObjectObject(cand.goState)
	}
}

func (srv *Server) luaRangeNearest(ctx context.Context, origin luaRangeOrigin, kinds []luaRangeKind, rangeVal float32, entry, hostile, dead uint32, typeMask uint16) *scripting.Object {
	if srv == nil || origin.srv == nil || rangeVal < 0 {
		return nil
	}
	var best *luaRangeCandidate
	bestAdj := float64(rangeVal)
	for _, cand := range srv.luaRangeCollect(ctx, origin, kinds, rangeVal) {
		cand := cand
		adjusted, ok := origin.passes(srv, cand, float64(rangeVal), entry, hostile, dead, typeMask)
		if !ok || adjusted > bestAdj {
			continue
		}
		if best == nil || adjusted < bestAdj {
			best, bestAdj = &cand, adjusted
		}
	}
	if best == nil {
		return nil
	}
	return srv.luaRangeObject(*best)
}

func (srv *Server) luaRangeList(ctx context.Context, origin luaRangeOrigin, kinds []luaRangeKind, rangeVal float32, entry, hostile, dead uint32, typeMask uint16) []any {
	out := make([]any, 0)
	if srv == nil || origin.srv == nil || rangeVal < 0 {
		return out
	}
	for _, cand := range srv.luaRangeCollect(ctx, origin, kinds, rangeVal) {
		if _, ok := origin.passes(srv, cand, float64(rangeVal), entry, hostile, dead, typeMask); !ok {
			continue
		}
		if obj := srv.luaRangeObject(cand); obj != nil {
			out = append(out, obj)
		}
	}
	return out
}

// luaRangeFriendlyTo mirrors WorldObject::IsFriendlyTo
// (Object.cpp:2847-2850, GetReactionTo >= REP_FRIENDLY) between the
// origin unit and a candidate unit. A motion origin reuses
// creatureFriendlyToTarget (player targets through their live session,
// creature targets through the faction-template reaction); a player
// origin uses the session twins (team-based player-vs-player with the
// duel arm, faction-reaction player-vs-creature). An origin with no
// unit reads not-friendly.
func (srv *Server) luaRangeFriendlyTo(origin luaRangeUnit, cand luaRangeCandidate) bool {
	if origin.motion != nil {
		return srv.creatureFriendlyToTarget(origin.motion, cand.guid)
	}
	if origin.isPlayer && origin.sess != nil {
		if cand.kind == luaRangeKindPlayer && cand.sess != nil {
			return origin.sess.isFriendlyToPlayer(cand.sess)
		}
		return origin.sess.isFriendlyToCreatureTarget(cand.guid)
	}
	return false
}

// luaUnitFriendlyMethods builds the Eluna Unit in-range pair
// (UnitMethods.h:1255/1296) for one unit surface — deliberately
// separate from luaRangeMethods, which the gameobject surface also
// shares (a gameobject is not a unit). GetFriendlyUnitsInRange is
// AnyFriendlyUnitInObjectRangeCheck (GridNotifiers.h:966): alive
// units in range passing IsFriendlyTo, self excluded; the unfriendly
// twin is AnyUnfriendlyUnitInObjectRangeCheck (:916): alive units in
// range failing IsFriendlyTo, so neutrals count as unfriendly. Both
// default to SIZE_OF_GRIDS (533.33333) and return a Lua array of unit
// objects, GUID-sorted like luaRangeList. DELTAS: the range gate is
// the shared 3D + both-reaches gate — the friendly C++ arm uses a
// double vertical cylinder instead, so a unit far above/below within
// 3D range is included here where C++ excludes it; the player-origin
// player-vs-player friendly read is team-based (duel arm included),
// not the full C++ reaction fold.
func luaUnitFriendlyMethods(srv *Server, origin func() luaRangeOrigin) map[string]scripting.ObjectMethod {
	list := func(friendly bool) scripting.ObjectMethod {
		return func(ctx context.Context, args []any) ([]any, error) {
			rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
			if err != nil {
				return nil, err
			}
			o := origin()
			out := make([]any, 0)
			if srv == nil || o.srv == nil || rangeVal < 0 || o.unit == nil {
				return []any{out}, nil
			}
			for _, cand := range srv.luaRangeCollect(ctx, o, []luaRangeKind{luaRangeKindPlayer, luaRangeKindCreature}, rangeVal) {
				if _, ok := o.passes(srv, cand, float64(rangeVal), 0, 0, 1, 0); !ok {
					continue
				}
				if srv.luaRangeFriendlyTo(*o.unit, cand) != friendly {
					continue
				}
				if obj := srv.luaRangeObject(cand); obj != nil {
					out = append(out, obj)
				}
			}
			return []any{out}, nil
		}
	}
	return map[string]scripting.ObjectMethod{
		"GetFriendlyUnitsInRange":   list(true),
		"GetUnfriendlyUnitsInRange": list(false),
	}
}

func luaOptionalFloat32Arg(args []any, index int, def float32) (float32, error) {
	if index >= len(args) || args[index] == nil {
		return def, nil
	}
	return luaFloat32Arg(args, index)
}

func luaOptionalUint32Arg(args []any, index int, def uint32) (uint32, error) {
	if index >= len(args) || args[index] == nil {
		return def, nil
	}
	return luaUint32Arg(args, index)
}

func luaOptionalUint16Arg(args []any, index int, def uint16) (uint16, error) {
	value, err := luaOptionalUint32Arg(args, index, uint32(def))
	if err != nil {
		return 0, err
	}
	return uint16(value), nil
}

// luaRangeMethods builds the Eluna nearest/in-range method set for
// one surface; origin snapshots the caller's live position at call
// time. The player surface replaces its range-only
// GetNearestGameObject bridge with the Eluna (range, entry, hostile)
// form — the old 2D metric had zero Lua callers.
func luaRangeMethods(srv *Server, origin func() luaRangeOrigin) map[string]scripting.ObjectMethod {
	methods := map[string]scripting.ObjectMethod{}
	methods["GetNearestPlayer"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 2, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeNearest(ctx, origin(), []luaRangeKind{luaRangeKindPlayer}, rangeVal, 0, hostile, dead, 0)}, nil
	}
	methods["GetNearestCreature"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 3, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeNearest(ctx, origin(), []luaRangeKind{luaRangeKindCreature}, rangeVal, entry, hostile, dead, 0)}, nil
	}
	methods["GetNearestGameObject"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeNearest(ctx, origin(), []luaRangeKind{luaRangeKindGameObject}, rangeVal, entry, hostile, 0, 0)}, nil
	}
	methods["GetPlayersInRange"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 2, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeList(ctx, origin(), []luaRangeKind{luaRangeKindPlayer}, rangeVal, 0, hostile, dead, 0)}, nil
	}
	methods["GetCreaturesInRange"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 3, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeList(ctx, origin(), []luaRangeKind{luaRangeKindCreature}, rangeVal, entry, hostile, dead, 0)}, nil
	}
	methods["GetGameObjectsInRange"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeList(ctx, origin(), []luaRangeKind{luaRangeKindGameObject}, rangeVal, entry, hostile, 0, 0)}, nil
	}
	allKinds := []luaRangeKind{luaRangeKindPlayer, luaRangeKindCreature, luaRangeKindGameObject}
	methods["GetNearObject"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		typeMask, err := luaOptionalUint16Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 3, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 4, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeNearest(ctx, origin(), allKinds, rangeVal, entry, hostile, dead, typeMask)}, nil
	}
	methods["GetNearObjects"] = func(ctx context.Context, args []any) ([]any, error) {
		rangeVal, err := luaOptionalFloat32Arg(args, 0, luaRangeDefaultRange)
		if err != nil {
			return nil, err
		}
		typeMask, err := luaOptionalUint16Arg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		entry, err := luaOptionalUint32Arg(args, 2, 0)
		if err != nil {
			return nil, err
		}
		hostile, err := luaOptionalUint32Arg(args, 3, 0)
		if err != nil {
			return nil, err
		}
		dead, err := luaOptionalUint32Arg(args, 4, 1)
		if err != nil {
			return nil, err
		}
		return []any{srv.luaRangeList(ctx, origin(), allKinds, rangeVal, entry, hostile, dead, typeMask)}, nil
	}
	// IsWithinLoS is WorldObject::IsWithinLOS (WorldObjectMethods.h:932,
	// Object.cpp:1252/1272): the object form runs IsWithinLOSInMap (a
	// target on another map reads false, following the generic IsInMap
	// map-equality precedent), the xyz form runs IsWithinLOS. Both
	// raycast through Server.hasLineOfSight, the default M2-kept VMap
	// form matching C++'s default ignoreFlags. DELTAS: no collision-
	// height offset on either end (C++ adds GetCollisionHeight and
	// hit-sphere points), no dynamic-tree check, no phase-mask check —
	// the raw-position model the spell LOS arms use. A target object
	// without position fields reads 0,0,0, the objectFloat convention.
	// Zero Lua callers — engine-correctness, like the range family.
	methods["IsWithinLoS"] = func(_ context.Context, args []any) ([]any, error) {
		o := origin()
		if o.srv == nil {
			return []any{false}, nil
		}
		if len(args) > 0 {
			if target, ok := args[0].(*scripting.Object); ok && target != nil {
				targetMap, _ := objectUint32Field(target, "MapId")
				if _, ok := objectUint32Field(target, "MapId"); !ok {
					targetMap, _ = objectUint32Field(target, "Map")
				}
				if targetMap != o.mapID {
					return []any{false}, nil
				}
				tx, _ := objectFloat32Field(target, "X")
				ty, _ := objectFloat32Field(target, "Y")
				tz, _ := objectFloat32Field(target, "Z")
				return []any{o.srv.hasLineOfSight(o.mapID, o.x, o.y, o.z, tx, ty, tz)}, nil
			}
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
		return []any{o.srv.hasLineOfSight(o.mapID, o.x, o.y, o.z, x, y, z)}, nil
	}
	return methods
}

// luaSendChatMessageToPlayer is UnitMethods::SendChatMessageToPlayer
// (UnitMethods.h:2095-2113): ChatHandler::BuildChatPacket to the target
// player's session, shared by the player, session-creature and
// handler-creature surfaces. Type and language carry the C++ range gates
// (MAX_CHAT_MSG_TYPE 0x34, SharedDefines.h:3216; LANGUAGES_COUNT 19,
// SharedDefines.h:789); the target must be a Player object like C++
// CHECKOBJ<Player>, and a target with no live session drops silently,
// the Unit::Whisper null-target precedent. DELTAS: the packet uses the
// shared BuildChatMessage builder, so monster-type senderName and the
// GM-message opcode legs are unmodeled (same delta as SendUnitWhisper);
// a channel type writes an empty channel name (C++ asserts non-empty).
func (s *Server) luaSendChatMessageToPlayer(senderGUID uint64, senderChatTag uint8, args []any) ([]any, error) {
	chatType, err := luaUint32Arg(args, 0)
	if err != nil {
		return nil, err
	}
	if chatType >= 0x34 {
		return nil, fmt.Errorf("valid ChatMsg expected")
	}
	lang, err := luaUint32Arg(args, 1)
	if err != nil {
		return nil, err
	}
	if lang >= 19 {
		return nil, fmt.Errorf("valid Language expected")
	}
	message, err := luaStringArg(args, 2)
	if err != nil {
		return nil, err
	}
	if len(args) < 4 {
		return nil, fmt.Errorf("argument 4 is required")
	}
	obj, ok := args[3].(*scripting.Object)
	if !ok || obj == nil || obj.Type != "Player" {
		return nil, fmt.Errorf("argument 4 must be a player")
	}
	targetGUID, ok := objectUint64Field(obj, "GUID")
	if !ok || targetGUID == 0 {
		return nil, fmt.Errorf("argument 4 must be a player")
	}
	if s == nil {
		return nil, nil
	}
	receiver := s.findSessionByGUID(targetGUID)
	if receiver == nil || receiver.player == nil {
		return nil, nil
	}
	packet := protocol.BuildChatMessageWithOptions(uint8(chatType), lang, senderGUID, targetGUID, message, "", false, "", senderChatTag)
	return nil, receiver.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
}

// attackingPlayerGraphRow is one motion-map row for the
// IsAttackingPlayer walk: the controller links plus the Controlled-vs-
// SummonSlot discriminator (motionIsPet).
type attackingPlayerGraphRow struct {
	charmer uint64
	owner   uint64
	charmed bool
	isPet   bool
}

// creatureIsAttackingPlayer is Unit::isAttackingPlayer (Unit.cpp:5849-5865)
// for a creature motion. C++'s UNIT_STATE_ATTACK_PLAYER is not a general
// "attacking a player" flag: it is set only by Player::SetContestedPvP
// (Player.cpp:20725-20747) on the player and its m_Controlled units, and
// cleared by ResetContestedPvP/AtDisengage — so Go reads the contested-PvP
// flag Go already models (threat.go SetContestedPvP twin) instead of the
// victim. A motion is flagged when its controller is flagged: charmed
// motions via CharmerGUID, pets (PetID != 0) via OwnerGUID; the walk
// follows those controller edges up and the charmer/owner edges down
// into controlled units and summons, mirroring the C++ recursion into
// m_Controlled and m_SummonSlot. DELTAS: a summon of a flagged creature
// reads via its summoner where C++ reads false (Go's flat OwnerGUID
// record does not separate SummonSlot from Controlled on the downward
// leg, and the upward leg has no summon edge, so only the downward
// reach differs); charm chains through a creature charmer over-read
// (C++ flags direct controlled units only); a controlled unit's flag
// lingering after its owner's contested expiry (C++ clears the owner
// only) reads false here, since Go reads the owner's live state; a
// player charmed by the queried creature is unreachable (players have
// no motion record).
func (s *Server) creatureIsAttackingPlayer(mapID, instanceID uint32, guid uint64) bool {
	if s == nil || guid == 0 {
		return false
	}
	rows := map[uint64]attackingPlayerGraphRow{}
	children := map[uint64][]uint64{}
	s.motionMu.Lock()
	for key, motion := range s.motionMapLocked(mapID, instanceID) {
		if motion == nil {
			continue
		}
		rows[key] = attackingPlayerGraphRow{charmer: motion.CharmerGUID, owner: motion.OwnerGUID, charmed: motion.Charmed, isPet: motionIsPet(motion)}
		if motion.CharmerGUID != 0 {
			children[motion.CharmerGUID] = append(children[motion.CharmerGUID], key)
		}
		if motion.OwnerGUID != 0 {
			children[motion.OwnerGUID] = append(children[motion.OwnerGUID], key)
		}
	}
	s.motionMu.Unlock()
	visited := map[uint64]bool{guid: true}
	queue := []uint64{guid}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if row, ok := rows[current]; ok {
			if row.charmed && row.charmer != 0 && !visited[row.charmer] {
				visited[row.charmer] = true
				queue = append(queue, row.charmer)
			}
			if row.isPet && row.owner != 0 && !visited[row.owner] {
				visited[row.owner] = true
				queue = append(queue, row.owner)
			}
		}
		for _, child := range children[current] {
			if !visited[child] {
				visited[child] = true
				queue = append(queue, child)
			}
		}
	}
	now := time.Now()
	for candidate := range visited {
		if _, ok := rows[candidate]; ok {
			continue
		}
		if sess := s.findSessionByGUID(candidate); sess != nil && sess.contestedPvPActiveQuery(now) {
			return true
		}
	}
	return false
}
