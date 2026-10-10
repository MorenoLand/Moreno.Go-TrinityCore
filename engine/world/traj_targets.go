package world

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetDestTraj uint32 = 89 // TARGET_DEST_TRAJ (SharedDefines.h:1525)

	// trajectoryMissileSize mirrors TRAJECTORY_MISSILE_SIZE (Spell.h:70):
	// the missile-body width added to the target's combat reach in the
	// trajectory collision checks.
	trajectoryMissileSize float32 = 3.0

	// creatureTypeFlagCanCollideWithMissiles mirrors
	// CREATURE_TYPE_FLAG_CAN_COLLIDE_WITH_MISSILES (SharedDefines.h:2748):
	// creatures without it never collide with trajectory missiles.
	creatureTypeFlagCanCollideWithMissiles uint32 = 0x00080000
)

// spellHasTrajTarget reports whether any active effect carries the
// trajectory implicit target (TARGET_SELECT_CATEGORY_TRAJ, Spell.cpp:918).
func spellHasTrajTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetDestTraj || eff.ImplicitTargetB == implicitTargetDestTraj {
			return true
		}
	}
	return false
}

// spellMaxHostileRange mirrors SpellInfo::GetMaxRange(false) (SpellInfo.h:500)
// the way Go's own range validation reads it: the SpellRange hostile max.
func spellMaxHostileRange(store *wotlk.Store, spell wotlk.Spell) float32 {
	if store == nil {
		return 0
	}
	if entry, ok, err := store.SpellRange(spell.RangeIndex); err == nil && ok {
		return entry.MaxHostile
	}
	return 0
}

// trajCandidate is one unit on the missile's potential flight path.
type trajCandidate struct {
	guid        uint64
	x, y, z     float32
	combatReach float32
	entry       uint32 // creature template entry; 0 for players
	isPlayer    bool
}

// trajCollideFlags returns the creature_template type_flags for the given
// entries in one query, so the missile-collision loop can skip creatures
// that cannot collide with missiles.
func (s *session) trajCollideFlags(ctx context.Context, entries []uint32) map[uint32]uint32 {
	flags := make(map[uint32]uint32)
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || len(entries) == 0 {
		return flags
	}
	seen := make(map[uint32]struct{}, len(entries))
	unique := make([]uint32, 0, len(entries))
	for _, entry := range entries {
		if entry == 0 {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		unique = append(unique, entry)
	}
	if len(unique) == 0 {
		return flags
	}
	placeholders := make([]string, len(unique))
	args := make([]any, len(unique))
	for i, entry := range unique {
		placeholders[i] = "?"
		args[i] = entry
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT entry, COALESCE(type_flags, 0) FROM creature_template WHERE entry IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return flags
	}
	defer rows.Close()
	for rows.Next() {
		var entry int64
		var typeFlags int64
		if err := rows.Scan(&entry, &typeFlags); err != nil {
			continue
		}
		flags[uint32(entry)] = uint32(typeFlags)
	}
	return flags
}

// trajCandidates mirrors the WorldObjectSpellTrajTargetCheck search
// (Spell.cpp:1640-1643 + 8452-8462): every unit within dist2d of the source
// that lies in the missile corridor — the corridor is caster-anchored
// (Position::HasInLine, Position.cpp:139: the check calls
// _caster->HasInLine, so the frontal-hemisphere arc and the lateral
// |sin(angle)| * dist test both use the caster's position and orientation,
// not the spell source position), with |sin(angle)| * dist <
// combatReach + TRAJECTORY_MISSILE_SIZE — while the range arm measures the
// target's 2D distance from the spell source position (Spell.cpp:8460).
// Go has no full SpellInfo::CheckTarget, so the gate is the standing
// unit-validity approximation: alive and not combat-disabled.
func (s *session) trajCandidates(ctx context.Context, spell wotlk.Spell, srcX, srcY float32, dist2d float32) []trajCandidate {
	candidates := make([]trajCandidate, 0, 16)
	seen := make(map[uint64]struct{})
	ori := float64(s.player.Orientation)
	// The trajectory search runs under GRID_MAP_TYPE_MASK_ALL in C++
	// (Spell.cpp:1643), but the per-candidate SpellInfo::CheckTarget gate
	// (implicit, Spell.cpp:8316 -> SpellInfo.cpp:1654-1662/1712-1713) still
	// rejects non-player candidates for ONLY_TARGET_PLAYERS spells and
	// non-ghost candidates for ONLY_TARGET_GHOSTS spells.
	playersOnly := spellSearchPlayersOnly(spell)
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z, combatReach float32, unitFlags, flagsExtra, health uint32, entry uint32, isPlayer bool) {
		if guid == 0 || guid == s.playerGUID {
			return
		}
		if !isPlayer && playersOnly {
			return
		}
		if mapID != s.player.Map || instanceID != s.player.InstanceID || health == 0 {
			return
		}
		if creatureCombatDisabled(unitFlags, flagsExtra) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		dx, dy := float64(x-srcX), float64(y-srcY)
		objDist2d := math.Sqrt(dx*dx + dy*dy)
		if objDist2d > float64(dist2d) {
			return
		}
		// The HasInLine corridor (Spell.cpp:8458 → Position.cpp:139) is
		// caster-anchored: the arc test and the lateral offset both use the
		// caster's position, not the spell source position.
		cdx, cdy := float64(x-s.player.X), float64(y-s.player.Y)
		if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, x, y, math.Pi) {
			return
		}
		relAngle := math.Atan2(cdy, cdx) - ori
		casterDist2d := math.Sqrt(cdx*cdx + cdy*cdy)
		if math.Abs(math.Sin(relAngle))*casterDist2d >= float64(combatReach+trajectoryMissileSize) {
			return
		}
		seen[guid] = struct{}{}
		candidates = append(candidates, trajCandidate{guid: guid, x: x, y: y, z: z, combatReach: combatReach, entry: entry, isPlayer: isPlayer})
	}
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(s.player.Map, s.player.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	for _, motion := range motions {
		motionGUIDs[motion.GUID] = struct{}{}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.CombatReach, motion.UnitFlags, motion.FlagsExtra, motion.Health, motion.Entry, false)
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		accept(targetSession.playerGUID, targetSession.player.Map, targetSession.player.InstanceID, targetSession.player.X, targetSession.player.Y, targetSession.player.Z, targetSession.player.CombatReach, targetSession.player.UnitFlags, 0, targetSession.player.Health, 0, true)
	}
	s.server.sessionsMu.RUnlock()
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, s.player.Map, float64(srcX-dist2d), float64(srcX+dist2d), float64(srcY-dist2d), float64(srcY+dist2d))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if err := rows.Scan(&low, &entry, &mapID, &x, &y, &z, &unitFlags, &flagsExtra, &health); err != nil {
					continue
				}
				guid := creatureWorldGUID(uint32(low), uint32(entry))
				if _, hasMotion := motionGUIDs[guid]; hasMotion {
					continue
				}
				stats := s.server.loadCreatureStats(ctx, uint32(entry))
				accept(guid, uint32(mapID), s.player.InstanceID, float32(x), float32(y), float32(z), stats.CombatReach, uint32(unitFlags), uint32(flagsExtra), uint32(health), uint32(entry), false)
			}
		}
	}
	return candidates
}

// resolveTrajDestination mirrors Spell::SelectImplicitTrajTargets
// (Spell.cpp:1626-1708): it shortens the spell destination to the first
// missile-body collision along the ballistic trajectory from the source to
// the current destination. changed reports whether the destination moved;
// callers keep the previous destination otherwise (C++ only calls ModDst
// when dist2d > bestDist). Like the C++ dispatch (Spell.cpp:918-923), the
// source defaults to the caster position (CheckDst covers the destination).
func (s *session) resolveTrajDestination(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect, target protocol.SpellTargetData, dx, dy, dz float32) (float32, float32, float32, bool) {
	if target.TrajSpeed == 0 {
		return dx, dy, dz, false
	}
	sx, sy, sz := s.player.X, s.player.Y, s.player.Z
	if target.Flags&protocol.SpellTargetFlagSourceLocation != 0 {
		sx, sy, sz = target.Source.X, target.Source.Y, target.Source.Z
	}
	ddx, ddy := float64(dx-sx), float64(dy-sy)
	dist2d := float32(math.Sqrt(ddx*ddx + ddy*ddy))
	if dist2d == 0 {
		return dx, dy, dz, false
	}
	b := float32(math.Tan(float64(target.TrajElevation)))
	a := (dz - sz - dist2d*b) / (dist2d * dist2d)
	if a > -0.0001 {
		a = 0
	}
	bestDist := spellMaxHostileRange(s.server.Data, spell)
	if eff.TriggerSpell != 0 {
		// "We should check if triggered spell has greater range (which is
		// true in many cases, and initial spell has too short max range)"
		// (Spell.cpp:1651-1655); the range entry fix in SpellMgr.cpp:4933
		// is subsumed by the max() term here.
		if trigger, found, err := s.server.Data.Spell(eff.TriggerSpell); err == nil && found {
			triggerMax := spellMaxHostileRange(s.server.Data, trigger)
			if triggerMax > bestDist {
				bestDist = triggerMax
			}
		}
		if limit := dist2d; limit < 300 {
			if limit < bestDist {
				bestDist = limit
			}
		} else if bestDist > 300 {
			bestDist = 300
		}
	}
	candidates := s.trajCandidates(ctx, spell, sx, sy, dist2d)
	collideFlags := s.trajCollideFlags(ctx, trajCandidateEntries(candidates))
	// targets.sort(Trinity::ObjectDistanceOrderPred(m_caster)) (Spell.cpp:1644):
	// Object::GetDistanceOrder takes is3D = true by default (Object.cpp:1306),
	// so the sort key is the 3D squared distance from the caster — not 2D.
	sort.Slice(candidates, func(i, j int) bool {
		di := (candidates[i].x-s.player.X)*(candidates[i].x-s.player.X) + (candidates[i].y-s.player.Y)*(candidates[i].y-s.player.Y) + (candidates[i].z-s.player.Z)*(candidates[i].z-s.player.Z)
		dj := (candidates[j].x-s.player.X)*(candidates[j].x-s.player.X) + (candidates[j].y-s.player.Y)*(candidates[j].y-s.player.Y) + (candidates[j].z-s.player.Z)*(candidates[j].z-s.player.Z)
		return di < dj
	})
	ori := float64(s.player.Orientation)
	for _, c := range candidates {
		if !c.isPlayer {
			// Creatures without CREATURE_TYPE_FLAG_CAN_COLLIDE_WITH_MISSILES
			// never collide (Spell.cpp:1670-1673); players always can.
			if collideFlags[c.entry]&creatureTypeFlagCanCollideWithMissiles == 0 {
				continue
			}
		}
		// The IsOnVehicle/GetVehicle skips (Spell.cpp:1666) have no Go
		// vehicle-casting infra.
		size := c.combatReach
		if size < 1.0 {
			size = 1.0
		}
		relX, relY := float64(c.x-sx), float64(c.y-sy)
		objDist2d := float32(math.Sqrt(relX*relX + relY*relY))
		relAngle := math.Atan2(relY, relX) - ori
		horizontalDistToTraj := math.Abs(float64(objDist2d) * math.Sin(relAngle))
		sizeFactor := math.Cos(horizontalDistToTraj / float64(size) * (math.Pi / 2.0))
		distToHitPoint := float32(math.Max(float64(objDist2d)*math.Cos(relAngle)-float64(size)*sizeFactor, 0))
		height := distToHitPoint * (a*distToHitPoint + b)
		if math.Abs(float64(c.z-sz)-float64(height)) > float64(size)+float64(b)/2.0+float64(trajectoryMissileSize) {
			continue
		}
		if distToHitPoint < bestDist {
			bestDist = distToHitPoint
			break
		}
	}
	if dist2d > bestDist {
		nx := sx + float32(math.Cos(ori))*bestDist
		ny := sy + float32(math.Sin(ori))*bestDist
		nz := sz + bestDist*(a*bestDist+b)
		return nx, ny, nz, true
	}
	return dx, dy, dz, false
}

// trajCandidateEntries collects the creature entries needing type_flags
// lookups for the missile-collision pass.
func trajCandidateEntries(candidates []trajCandidate) []uint32 {
	entries := make([]uint32, 0, len(candidates))
	for _, c := range candidates {
		if !c.isPlayer {
			entries = append(entries, c.entry)
		}
	}
	return entries
}
