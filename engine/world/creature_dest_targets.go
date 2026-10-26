package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellHasCreatureDestTarget reports whether any active effect carries a
// dest-family implicit target that the creature path resolves: the
// caster-dest family (SelectImplicitCasterDestTargets, Spell.cpp:1312-1424),
// the target-dest family (SelectImplicitTargetDestTargets, 1433-1462), the
// dest-dest family (SelectImplicitDestDestTargets, 1464-1499), and the traj
// target (SelectImplicitTrajTargets, 1626 — seed only, see below).
func spellHasCreatureDestTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isCasterDestTarget(eff.ImplicitTargetA) || isCasterDestTarget(eff.ImplicitTargetB) ||
			isTargetDestTarget(eff.ImplicitTargetA) || isTargetDestTarget(eff.ImplicitTargetB) ||
			isDestDestTarget(eff.ImplicitTargetA) || isDestDestTarget(eff.ImplicitTargetB) ||
			eff.ImplicitTargetA == implicitTargetDestTraj || eff.ImplicitTargetB == implicitTargetDestTraj ||
			eff.ImplicitTargetA == implicitTargetDestNearbyEntry || eff.ImplicitTargetB == implicitTargetDestNearbyEntry {
			return true
		}
	}
	return false
}

// resolveCreatureSpellDestination mirrors the dest-family arms of
// Spell::SelectEffectImplicitTargets (Spell.cpp:900-975) for a creature
// caster, anchored on the motion instead of the session: the caster-dest
// arms resolve from the motion's position and orientation, the target-dest
// arms from the explicit unit target's position, and the dest-dest arms
// offset the running destination. Creature casts carry no client
// destination, so the seed is the motion position — the CheckDst fallback
// (Spell.cpp:6523) that C++ applies before the traj and dest-dest arms
// (Spell.cpp:920, 1470), and SendSpellGo writes m_targets into the packet
// (Spell.cpp:4338), so the resolved destination rides the GO packet the
// same way.
//
// Later effects overwrite earlier ones and each effect's A arm runs before
// its B arm, matching the player path and the C++ per-effect order.
// ok is false when the spell carries no bridgeable dest-family target;
// destFailed is true when a 46 arm found no match.
//
// Documented no-bridge: TARGET_DEST_TRAJ (89) beyond the seed —
// SelectImplicitTrajTargets early-returns on m_speed == 0 (Spell.cpp:1628)
// and creature AI casts never set a trajectory (Unit::CastSpell builds
// default targets); TARGET_DEST_CASTER_FISHING (39, player bobber model);
// the 76/106 channel destinations (motions hold channel duration but no
// channel target).
//
// TARGET_DEST_NEARBY_ENTRY (46) resolves per effect through
// creatureEntryNearbyDestPosition below: each effect scans with its own
// ImplicitTargetConditions list (Spell.cpp:1066) and a no-match on any
// 46 arm fails the cast (Spell.cpp:1111, SendCastResult
// SPELL_FAILED_BAD_IMPLICIT_TARGETS plus finish(false)), reported via
// the destFailed return so call sites abort before the GO packet — not
// the seed-dest fallback the target-dest arms use without a resolvable
// unit target.
func (s *Server) resolveCreatureSpellDestination(ctx context.Context, m *creatureMotion, spell wotlk.Spell, targetGUID uint64) (float32, float32, float32, bool, bool) {
	if s == nil || m == nil {
		return 0, 0, 0, false, false
	}
	if !spellHasCreatureDestTarget(spell) {
		return 0, 0, 0, false, false
	}
	x, y, z := m.X, m.Y, m.Z
	combatReach := m.CombatReach
	if combatReach <= 0 {
		combatReach = 1.5
	}
	// SpellInfo.cpp:551-574: CalcRadius(m_caster) folds SPELLMOD_RADIUS
	// through GetSpellModOwner (Object.cpp:2246-2261) — a pet or totem
	// caster resolves to its owner player's session; wild creatures fold
	// nothing.
	var modRadius func(float32) float32
	if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
		modRadius = func(r float32) float32 {
			return float32(modOwner.applySpellModFloat(spell, spellModRadius, float64(r)))
		}
	}
	// SelectImplicitTargetDestTargets anchors on the wire object target
	// (Spell.cpp:1433-1436); on this path that is the explicit unit target
	// (Unit::CastSpell sets it before _cast).
	var unitX, unitY, unitZ, unitO float32
	hasUnitTarget := false
	if targetGUID != 0 {
		if ux, uy, uz, _, _, found := s.aiThreatUnitPos(m, targetGUID, nil, true); found {
			unitX, unitY, unitZ = ux, uy, uz
			unitO = s.creatureDestTargetOrientation(m, targetGUID)
			hasUnitTarget = true
		}
	}
	// TARGET_DEST_NEARBY_ENTRY (46) scans per effect in the loop below:
	// each effect's own ImplicitTargetConditions list (Spell.cpp:1066)
	// gates its scan, cached across the effect's two slots. C++ merges
	// effects sharing one conditions pointer into a single selection
	// (Spell.cpp:885-900); rescanning an identical list yields the
	// identical nearest position, so the per-effect scan is
	// outcome-equivalent there and exact where the lists differ.
	scannedEff := -1
	var nearbyX, nearbyY, nearbyZ float32
	nearbyFound := false
	for effIndex, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch {
			case isCasterDestTarget(targetType):
				x, y, z = resolveCasterDestPosition(s.Data, targetType, eff.RadiusIndex, m.Level, m.X, m.Y, m.Z, m.Orientation, combatReach, modRadius)
			case isTargetDestTarget(targetType):
				// C++ asserts a non-null object target here
				// (Spell.cpp:1434); without a resolvable unit target Go
				// keeps the current destination, the same fallback the
				// player path documents.
				if !hasUnitTarget {
					continue
				}
				x, y, z = resolveTargetDestPosition(s.Data, targetType, eff.RadiusIndex, unitX, unitY, unitZ, unitO)
			case isDestDestTarget(targetType):
				if nx, ny, nz, changed := resolveDestDestPosition(s.Data, targetType, eff.RadiusIndex, m.Level, x, y, z, m.Orientation, modRadius); changed {
					x, y, z = nx, ny, nz
				}
			case targetType == implicitTargetDestNearbyEntry:
				if effIndex != scannedEff {
					nearbyX, nearbyY, nearbyZ, nearbyFound = s.creatureEntryNearbyDestPosition(ctx, m, spell, effIndex)
					scannedEff = effIndex
				}
				if nearbyFound {
					x, y, z = nearbyX, nearbyY, nearbyZ
				} else {
					return x, y, z, true, true
				}
			}
		}
	}
	return x, y, z, true, false
}

// creatureDestTargetOrientation returns the orientation of the explicit
// target for target-dest anchoring: the non-trivial arms move the
// destination along the target's orientation
// (target->MovePositionToFirstCollision, Spell.cpp:1456).
func (s *Server) creatureDestTargetOrientation(m *creatureMotion, targetGUID uint64) float32 {
	if sess := s.findSessionByGUID(targetGUID); sess != nil && sess.player != nil {
		return sess.player.Orientation
	}
	if m == nil {
		return 0
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if mm := s.motionMapLocked(m.Map, m.InstanceID); mm != nil {
		if tm, ok := mm[targetGUID]; ok && tm != nil {
			return tm.Orientation
		}
	}
	return 0
}

// creatureEntryNearbyObjectRange mirrors the TARGET_CHECK_ENTRY arm of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1052) for a creature caster:
// range = GetMaxRange(IsPositive()), folded through SPELLMOD_RANGE via the
// mod-owner session (SpellInfo.cpp:3060-3072, Object.cpp:2246-2268) — the
// pet/totem owner's session, nil for wild casters (the creature-area
// geometry precedent).
func (s *Server) creatureEntryNearbyObjectRange(m *creatureMotion, spell wotlk.Spell) float64 {
	if s == nil || s.Data == nil || m == nil {
		return 0
	}
	rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok {
		return 0
	}
	maxRange := spellDBCMaxRange(spellIsPositive(spell), rangeEntry)
	if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
		maxRange = modOwner.applySpellModFloat(spell, spellModRange, maxRange)
	}
	return maxRange
}

// creatureEntryNearbyDestPosition ports the DEST half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_DEST_NEARBY_ENTRY (46) for a creature caster, mirroring the session
// twin spellEntryNearbyDestPosition: the destination becomes the position of
// the nearest world object within GetMaxRange(IsPositive()) passing
// SpellInfo::CheckTarget and the effect's own implicit-target conditions
// (Spell.cpp:1066: Effects[effIndex].ImplicitTargetConditions, loaded by
// the single-effect mask — never the union over all 46 effects, whose
// ElseGroup rows would AND together across effects).
// Candidates are player sessions, creature motions (the caster motion
// itself included — C++ visits the caster's own grid cell, and the session
// twin likewise includes the casting session), static DB creature spawns
// without motions (joining every instance of the map, the area-sweep
// precedent), and gameobjects (runtime + static rows). The ENTRY faction
// switch has no case (Spell.cpp:8316), so only the CheckTarget terms gate
// units: alive unless the spell allows dead targets (SpellInfo.cpp:1715),
// not combat-disabled (spellTargetUnitBlocked, assist=false like the
// session twin); corpses have no Go model and never match. With no
// conditions for this effect and RequiresSpellFocus set, the focus
// object's position is used (Spell.cpp:1085). No match returns false.
func (s *Server) creatureEntryNearbyDestPosition(ctx context.Context, m *creatureMotion, spell wotlk.Spell, effIndex int) (float32, float32, float32, bool) {
	if s == nil || m == nil {
		return 0, 0, 0, false
	}
	maxRange := s.creatureEntryNearbyObjectRange(m, spell)
	if maxRange <= 0 {
		return 0, 0, 0, false
	}
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, 1<<uint(effIndex))
	if len(condRows) == 0 && spell.RequiresSpellFocus != 0 {
		if x, y, z, ok := s.motionSpellFocusPosition(ctx, m, spell); ok {
			return x, y, z, true
		}
		return 0, 0, 0, false
	}
	allowPlayers, allowCreatures, allowGO := implicitTargetEntryObjectMask(condRows, true, true, true)
	// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842): the DEST base mask
	// narrows to PLAYER|CORPSE under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so creature and gameobject candidates
	// drop out (corpses have no Go model).
	if spellSearchPlayersOnly(spell) {
		allowCreatures, allowGO = false, false
	}
	allowDead := spellAllowsDeadTarget(spell)
	bestX, bestY, bestZ := float32(0), float32(0), float32(0)
	bestDist := maxRange
	found := false
	consider := func(x, y, z float32) {
		dist := distance3D(x, y, z, m.X, m.Y, m.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist, bestX, bestY, bestZ = dist, x, y, z
			found = true
		}
	}
	if allowPlayers || allowCreatures {
		s.sessionsMu.RLock()
		for targetSession := range s.sessions {
			if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
				continue
			}
			p := targetSession.player
			if p.Map != m.Map || p.InstanceID != m.InstanceID || !allowPlayers {
				continue
			}
			if p.Health == 0 && !allowDead {
				continue
			}
			if spellTargetUnitBlocked(spell, p.UnitFlags, 0, false) {
				continue
			}
			if !implicitEntryConditionsMet(condRows, true, 0, 0) {
				continue
			}
			consider(p.X, p.Y, p.Z)
		}
		s.sessionsMu.RUnlock()
	}
	if allowCreatures {
		s.motionMu.Lock()
		motionMap := s.motionMapLocked(m.Map, m.InstanceID)
		candidates := make([]*creatureMotion, 0, len(motionMap))
		motionGUIDs := make(map[uint64]struct{}, len(motionMap))
		for _, cand := range motionMap {
			if cand != nil {
				candidates = append(candidates, cand)
				motionGUIDs[cand.GUID] = struct{}{}
			}
		}
		s.motionMu.Unlock()
		for _, cand := range candidates {
			if cand.Map != m.Map || cand.InstanceID != m.InstanceID {
				continue
			}
			if cand.Health == 0 && !allowDead {
				continue
			}
			if spellTargetUnitBlocked(spell, cand.UnitFlags, cand.FlagsExtra, false) {
				continue
			}
			if !implicitEntryConditionsMet(condRows, false, cand.Entry, uint32(cand.GUID&0xFFFFFF)) {
				continue
			}
			consider(cand.X, cand.Y, cand.Z)
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, m.Map, float64(m.X)-maxRange, float64(m.X)+maxRange, float64(m.Y)-maxRange, float64(m.Y)+maxRange)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var low, entry, rowMap, unitFlags, flagsExtra, health int64
					var x, y, z float64
					if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &unitFlags, &flagsExtra, &health) != nil {
						continue
					}
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; hasMotion {
						continue
					}
					if uint32(rowMap) != m.Map {
						continue
					}
					if health <= 0 && !allowDead {
						continue
					}
					if spellTargetUnitBlocked(spell, uint32(unitFlags), uint32(flagsExtra), false) {
						continue
					}
					if !implicitEntryConditionsMet(condRows, false, uint32(entry), uint32(low)) {
						continue
					}
					consider(float32(x), float32(y), float32(z))
				}
			}
		}
	}
	if allowGO {
		s.scanNearbyGameObjectsAtServer(ctx, m.Map, m.InstanceID, m.X, m.Y, float32(maxRange), func(g nearbyGameObject) {
			if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
				return
			}
			consider(g.x, g.y, g.z)
		})
	}
	if !found {
		return 0, 0, 0, false
	}
	return bestX, bestY, bestZ, true
}
