package world

import (
	"context"
	"math"
	"math/rand"

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
			eff.ImplicitTargetA == implicitTargetDestNearbyEntry || (eff.ImplicitTargetB == implicitTargetDestNearbyEntry && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB)) {
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
		for slotIndex, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
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
				// A B-slot 46 behind a NEARBY/CONE/AREA A slot never selects
				// (Spell.cpp:866-900 processed-slot rule); without this guard
				// the arm re-scanned and could fail a cast C++ never fails.
				if slotIndex == 1 && implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB) {
					continue
				}
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

// creatureEntryNearbyTarget ports the UNIT half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_UNIT_NEARBY_ENTRY (38) for a creature caster, mirroring the
// session twin spellEntryNearbyTarget: the single nearest unit within
// GetMaxRange(IsPositive()) passing SpellInfo::CheckTarget and the FIRST
// 38 effect's implicit-target conditions (Spell.cpp:885-900 merge model,
// as for the session twin; later effects with different lists select
// separately in C++, a multi-target residual Go's single-target list
// cannot carry). The faction switch in WorldObjectSpellTargetCheck
// (Spell.cpp:8316) has no TARGET_CHECK_ENTRY case, so only the CheckTarget
// terms apply: alive unless the spell allows dead targets
// (SpellInfo.cpp:1715), not combat-disabled. There is no focus fallback
// for UNIT (the Spell.cpp:1065 emergency case covers GOBJ and DEST only).
// Candidates are player sessions, creature motions (the caster motion
// itself included — C++ visits the caster's own grid cell, as in the 46
// twin) and static DB creature spawns without motions. Distance is
// GetDistance (Object.cpp:1175): 3D distance minus the candidate's combat
// reach, floored at 0 — static spawns carry no reach term. The pick then
// takes the session twin's CheckEffectTarget LOS gate
// (implicitUnitTargetLOSPassesForMotion, Spell.cpp:2098/7395): a
// LOS-blocked pick fails instead of rescanning, the session precedent.
// No match returns false and the caller fails the cast with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
func (s *Server) creatureEntryNearbyTarget(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (uint64, bool) {
	if s == nil || m == nil {
		return 0, false
	}
	maxRange := s.creatureEntryNearbyObjectRange(m, spell)
	if maxRange <= 0 {
		return 0, false
	}
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, firstEntryImplicitEffectMask(spell, implicitTargetNearbyEntry))
	allowPlayers, allowCreatures := implicitTargetEntryMask(condRows)
	// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842): the UNIT base mask
	// narrows to PLAYER|CORPSE under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so creature candidates drop out
	// (corpses have no Go model).
	if spellSearchPlayersOnly(spell) {
		allowCreatures = false
	}
	allowDead := spellAllowsDeadTarget(spell)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	consider := func(guid uint64, x, y, z, reach float32) {
		dist := float64(distance3D(x, y, z, m.X, m.Y, m.Z)) - float64(reach)
		if dist < 0 {
			dist = 0
		}
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist, bestGUID, bestX, bestY, bestZ = dist, guid, x, y, z
		}
	}
	if allowPlayers {
		s.sessionsMu.RLock()
		for targetSession := range s.sessions {
			if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
				continue
			}
			p := targetSession.player
			if p.Map != m.Map || p.InstanceID != m.InstanceID {
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
			consider(targetSession.playerGUID, p.X, p.Y, p.Z, p.CombatReach)
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
			consider(cand.GUID, cand.X, cand.Y, cand.Z, cand.CombatReach)
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
					consider(guid, float32(x), float32(y), float32(z), 0)
				}
			}
		}
	}
	if bestGUID == 0 {
		return 0, false
	}
	if !s.implicitUnitTargetLOSPassesForMotion(spell, m, bestGUID, bestX, bestY, bestZ) {
		return 0, false
	}
	return bestGUID, true
}

// creatureEntryNearbyGOTarget ports the GOBJ half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_GAMEOBJECT_NEARBY_ENTRY (40) for a creature caster, mirroring the
// session twin spellEntryNearbyGOTarget: the single nearest gameobject
// within GetMaxRange(IsPositive()) passing the FIRST 40 effect's
// implicit-target conditions (Spell.cpp:885-900 merge model).
// SpellInfo::CheckTarget is always valid for gameobjects
// (SpellInfo.cpp:1707), so only range and the entry conditions gate the
// search. The emergency fallback (Spell.cpp:1065): with no conditions and
// RequiresSpellFocus set, the focus object is the target. No match
// returns false and the caller fails the cast with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
func (s *Server) creatureEntryNearbyGOTarget(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (uint64, bool) {
	if s == nil || m == nil {
		return 0, false
	}
	maxRange := s.creatureEntryNearbyObjectRange(m, spell)
	if maxRange <= 0 {
		return 0, false
	}
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, firstEntryImplicitEffectMask(spell, implicitTargetGONearbyEntry))
	if len(condRows) == 0 && spell.RequiresSpellFocus != 0 {
		// Spell.cpp:1071: focus object or cast failure.
		if guid, _, _, _, ok := s.motionSpellFocusObject(ctx, m, spell); ok {
			return guid, true
		}
		return 0, false
	}
	_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
	if !allowGO {
		return 0, false
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1809-1842): the GOBJ base mask
	// narrows to zero under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so the search contributes nothing.
	if spellSearchPlayersOnly(spell) {
		return 0, false
	}
	bestGUID := uint64(0)
	bestDist := maxRange
	s.scanNearbyGameObjectsAtServer(ctx, m.Map, m.InstanceID, m.X, m.Y, float32(maxRange), func(g nearbyGameObject) {
		if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
			return
		}
		dist := float64(distance3D(g.x, g.y, g.z, m.X, m.Y, m.Z))
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist, bestGUID = dist, g.guid
		}
	})
	if bestGUID == 0 {
		return 0, false
	}
	return bestGUID, true
}

// creatureHostileNearbyTarget ports the UNIT half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_UNIT_NEARBY_ENEMY (2) for a creature caster, mirroring the
// session twin spellHostileNearbyTarget: the single nearest hostile unit
// within GetMaxRange(false) — the spell's hostile max range, folded through
// SPELLMOD_RANGE via the mod-owner session (SpellInfo.cpp:3070-3072) —
// passing SpellInfo::CheckTarget and TARGET_CHECK_ENEMY (Spell.cpp:8323-8328:
// totem rejection + IsValidAttackTarget). The caster itself never selects
// (IsValidAttackTarget fails for self). Players gate on isAttackableFaction
// (the area-enemy twin's gate) and creatures on factionTemplatesHostile
// (the creature-vs-creature verdict); static DB spawns without motions join
// via the template-faction query (the area-enemy twin's leg). Distance is
// GetDistance (Object.cpp:1175): 3D distance minus the candidate's combat
// reach, floored at 0, strict less-than shrinking to the nearest match
// (WorldObjectSpellNearbyTargetCheck, Spell.cpp:8388). The pick takes the
// session twin's CheckEffectTarget LOS gate
// (implicitUnitTargetLOSPassesForMotion, Spell.cpp:2098/7395): a LOS-blocked
// pick fails instead of rescanning. No match returns false and the caller
// fails the cast with SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
// The ImplicitTargetConditions arm (base WorldObjectSpellTargetCheck
// tail, Spell.cpp:8380-8387) is wired through the entry-subset evaluator
// (implicitEntryConditionsMet, the session twin's arm): candidates failing
// the first selecting effect's CONDITION_OBJECT_ENTRY_GUID rows never
// shrink the nearest range, matching WorldObjectSpellNearbyTargetCheck
// (Spell.cpp:8394-8399). Non-entry condition types have no candidate-side
// Go evaluation and are skipped (the shared entry/chain-twin gap).
func (s *Server) creatureHostileNearbyTarget(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (uint64, bool) {
	if s == nil || m == nil || s.Data == nil {
		return 0, false
	}
	rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok || rangeEntry.MaxHostile <= 0 {
		return 0, false
	}
	maxRange := spellDBCMaxRange(false, rangeEntry)
	if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
		maxRange = modOwner.applySpellModFloat(spell, spellModRange, maxRange)
	}
	if maxRange <= 0 {
		return 0, false
	}
	// Spell.cpp:1066: the selecting effect's ImplicitTargetConditions,
	// first selecting effect's list under Go's one-target-per-spell model
	// (the 38 first-effect model, the session twin's arm).
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, firstEntryImplicitEffectMask(spell, implicitTargetNearbyEnemy))
	playersOnly := spellSearchPlayersOnly(spell)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	consider := func(guid uint64, x, y, z, reach float32) {
		dist := float64(distance3D(x, y, z, m.X, m.Y, m.Z)) - float64(reach)
		if dist < 0 {
			dist = 0
		}
		if dist < bestDist {
			bestDist, bestGUID, bestX, bestY, bestZ = dist, guid, x, y, z
		}
	}
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		p := targetSession.player
		if p.Map != m.Map || p.InstanceID != m.InstanceID || p.Health == 0 {
			continue
		}
		if sessionIsGameMaster(targetSession) {
			continue
		}
		if spellTargetUnitBlocked(spell, p.UnitFlags, 0, false) {
			continue
		}
		if !implicitEntryConditionsMet(condRows, true, 0, 0) {
			continue
		}
		candidate := playerPos{Map: p.Map, InstanceID: p.InstanceID, X: p.X, Y: p.Y, Z: p.Z, GUID: targetSession.playerGUID, Race: p.Race, Class: p.Class, Level: p.Level, FactionTemplate: targetSession.effectiveFactionTemplate(p.Race), Reputations: playerReputationMap(p.Reputations), Sess: targetSession}
		if !s.isAttackableFaction(m.Faction, candidate) {
			continue
		}
		consider(targetSession.playerGUID, p.X, p.Y, p.Z, p.CombatReach)
	}
	s.sessionsMu.RUnlock()
	if !playersOnly {
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
			if cand.GUID == m.GUID || cand.Map != m.Map || cand.InstanceID != m.InstanceID || cand.Health == 0 {
				continue
			}
			if s.isTotemGUID(cand.GUID) || spellTargetUnitBlocked(spell, cand.UnitFlags, cand.FlagsExtra, false) {
				continue
			}
			if !implicitEntryConditionsMet(condRows, false, cand.Entry, uint32(cand.GUID&0xFFFFFF)) {
				continue
			}
			if !s.factionTemplatesHostile(m.Faction, cand.Faction) {
				continue
			}
			consider(cand.GUID, cand.X, cand.Y, cand.Z, cand.CombatReach)
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, m.Map, float64(m.X)-maxRange, float64(m.X)+maxRange, float64(m.Y)-maxRange, float64(m.Y)+maxRange)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var low, entry, rowMap, faction, unitFlags, flagsExtra, health int64
					var x, y, z float64
					if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) != nil {
						continue
					}
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; hasMotion {
						continue
					}
					if guid == m.GUID || uint32(rowMap) != m.Map || health <= 0 {
						continue
					}
					if s.isTotemGUID(guid) || spellTargetUnitBlocked(spell, uint32(unitFlags), uint32(flagsExtra), false) {
						continue
					}
					if !implicitEntryConditionsMet(condRows, false, uint32(entry), uint32(low)) {
						continue
					}
					if !s.factionTemplatesHostile(m.Faction, uint32(faction)) {
						continue
					}
					consider(guid, float32(x), float32(y), float32(z), 0)
				}
			}
		}
	}
	if bestGUID == 0 {
		return 0, false
	}
	if !s.implicitUnitTargetLOSPassesForMotion(spell, m, bestGUID, bestX, bestY, bestZ) {
		return 0, false
	}
	return bestGUID, true
}

// creatureFriendlyNearbyTarget ports Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) + Spell::SearchNearbyTarget (Spell.cpp:1869) for
// TARGET_UNIT_NEARBY_PARTY (3) / TARGET_UNIT_NEARBY_ALLY (4) /
// TARGET_UNIT_NEARBY_RAID (58) for a creature caster: the single nearest
// unit within GetMaxRange(true) passing IsValidAssistTarget
// (Object.cpp:3087: alive, not combat-disabled, non-hostile, never a GM
// except the caster itself) and the party/raid scope of
// Unit::IsInPartyWith / Unit::IsInRaidWith (Unit.cpp:12126/12155) for
// the 3/58 legs. The caster itself is a candidate (assist-to-self, and
// IsInPartyWith/IsInRaidWith return true for self), so it wins at
// distance 0 whenever it passes. The ImplicitTargetConditions arm (base
// WorldObjectSpellTargetCheck tail, Spell.cpp:8380-8387) is wired through
// the entry-subset evaluator (implicitEntryConditionsMet) on the first
// selecting effect's rows, the session twin's arm; a failing nearer
// candidate never shrinks the nearest range (Spell.cpp:8394-8399).
// Party/raid scope resolves through the charmer-or-owner (motion
// OwnerGUID, CharmOwnerGUID fallback, the hostile twin's pattern):
// a player owner contributes its group snapshot, a creature owner or
// ownerless caster contributes same-faction unit-unit membership
// (Unit.cpp:12126 tail), and the TREAT_AS_RAID_UNIT player-creature
// arm has no Go template-flag model and stays unmodeled. Totem motions
// are refused (TARGET_CHECK_ALLY/PARTY/RAID reject totems, Spell.cpp:8333).
// No match returns false and the caller fails the cast
// (Spell.cpp:1111).
func (s *Server) creatureFriendlyNearbyTarget(ctx context.Context, m *creatureMotion, spell wotlk.Spell) (uint64, bool) {
	if s == nil || m == nil || s.Data == nil {
		return 0, false
	}
	rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok || rangeEntry.MaxFriendly <= 0 {
		return 0, false
	}
	maxRange := spellDBCMaxRange(true, rangeEntry)
	if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
		maxRange = modOwner.applySpellModFloat(spell, spellModRange, maxRange)
	}
	if maxRange <= 0 {
		return 0, false
	}
	checks := make(map[uint8]struct{})
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for slot, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if slot == 1 && implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB) {
				continue
			}
			switch targetType {
			case implicitTargetNearbyParty:
				checks[friendlyCheckParty] = struct{}{}
			case implicitTargetNearbyAlly:
				checks[friendlyCheckAlly] = struct{}{}
			case implicitTargetNearbyRaid:
				checks[friendlyCheckRaid] = struct{}{}
			}
		}
	}
	if len(checks) == 0 {
		return 0, false
	}
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, firstFriendlyNearbyEffectMask(spell))
	// Party/raid scope (Unit.cpp:12126/12155): the caster's effective
	// referer is its charmer-or-owner when that is a player, else the
	// caster creature itself (faction). A candidate's effective referer
	// is itself for players, its player owner for owned creatures, else
	// the creature itself (faction). Same referer wins; two players
	// consult the group snapshot; mixed player/creature pairs fail
	// (TREAT_AS_RAID_UNIT unmodeled); two creatures compare factions.
	casterOwnerGUID := m.OwnerGUID
	if casterOwnerGUID == 0 {
		casterOwnerGUID = m.CharmOwnerGUID
	}
	casterIsPlayerReferer := false
	var casterParty, casterRaid map[uint64]struct{}
	casterInGroup := false
	if casterOwnerGUID != 0 {
		if ownerSess := s.findSessionByGUID(casterOwnerGUID); ownerSess != nil && ownerSess.player != nil {
			_, casterParty, casterRaid, casterInGroup = groupMembershipSnapshot(s, ownerSess.groupID, casterOwnerGUID)
			casterIsPlayerReferer = true
		}
	}
	type referer struct {
		isPlayer bool
		guid     uint64
		faction  uint32
	}
	casterReferer := referer{isPlayer: casterIsPlayerReferer, guid: casterOwnerGUID, faction: m.Faction}
	candidateReferer := func(guid uint64, isPlayer bool, ownerGUID uint64, faction uint32) referer {
		if isPlayer {
			return referer{isPlayer: true, guid: guid}
		}
		if ownerGUID != 0 {
			if ownerSess := s.findSessionByGUID(ownerGUID); ownerSess != nil && ownerSess.player != nil {
				return referer{isPlayer: true, guid: ownerGUID}
			}
		}
		return referer{faction: faction}
	}
	inScope := func(check uint8, cand referer) bool {
		if check == friendlyCheckAlly {
			return true
		}
		if cand == casterReferer {
			return true
		}
		if !casterReferer.isPlayer || !cand.isPlayer {
			return false
		}
		if !casterInGroup {
			return false
		}
		if check == friendlyCheckParty {
			_, ok := casterParty[cand.guid]
			return ok
		}
		_, ok := casterRaid[cand.guid]
		return ok
	}
	scopeOK := func(cand referer) bool {
		_, wantAlly := checks[friendlyCheckAlly]
		_, wantParty := checks[friendlyCheckParty]
		_, wantRaid := checks[friendlyCheckRaid]
		return (wantAlly && inScope(friendlyCheckAlly, cand)) || (wantParty && inScope(friendlyCheckParty, cand)) || (wantRaid && inScope(friendlyCheckRaid, cand))
	}
	playersOnly := spellSearchPlayersOnly(spell)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	consider := func(guid uint64, x, y, z, reach float32) {
		dist := float64(distance3D(x, y, z, m.X, m.Y, m.Z)) - float64(reach)
		if dist < 0 {
			dist = 0
		}
		if dist < bestDist {
			bestDist, bestGUID, bestX, bestY, bestZ = dist, guid, x, y, z
		}
	}
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		p := targetSession.player
		if p.Map != m.Map || p.InstanceID != m.InstanceID || p.Health == 0 {
			continue
		}
		if sessionIsGameMaster(targetSession) {
			continue
		}
		if spellTargetUnitBlocked(spell, p.UnitFlags, 0, true) {
			continue
		}
		candidate := playerPos{Map: p.Map, InstanceID: p.InstanceID, X: p.X, Y: p.Y, Z: p.Z, GUID: targetSession.playerGUID, Race: p.Race, Class: p.Class, Level: p.Level, FactionTemplate: targetSession.effectiveFactionTemplate(p.Race), Reputations: playerReputationMap(p.Reputations), Sess: targetSession}
		if s.isAttackableFaction(m.Faction, candidate) {
			continue
		}
		if !implicitEntryConditionsMet(condRows, true, 0, 0) {
			continue
		}
		if !scopeOK(candidateReferer(targetSession.playerGUID, true, 0, 0)) {
			continue
		}
		consider(targetSession.playerGUID, p.X, p.Y, p.Z, p.CombatReach)
	}
	s.sessionsMu.RUnlock()
	if !playersOnly {
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
			if cand.Map != m.Map || cand.InstanceID != m.InstanceID || cand.Health == 0 {
				continue
			}
			if s.isTotemGUID(cand.GUID) || spellTargetUnitBlocked(spell, cand.UnitFlags, cand.FlagsExtra, true) {
				continue
			}
			if !implicitEntryConditionsMet(condRows, false, cand.Entry, uint32(cand.GUID&0xFFFFFF)) {
				continue
			}
			if s.factionTemplatesHostile(m.Faction, cand.Faction) {
				continue
			}
			owner := cand.OwnerGUID
			if owner == 0 {
				owner = cand.CharmOwnerGUID
			}
			if !scopeOK(candidateReferer(cand.GUID, false, owner, cand.Faction)) {
				continue
			}
			consider(cand.GUID, cand.X, cand.Y, cand.Z, cand.CombatReach)
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, m.Map, float64(m.X)-maxRange, float64(m.X)+maxRange, float64(m.Y)-maxRange, float64(m.Y)+maxRange)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var low, entry, rowMap, faction, unitFlags, flagsExtra, health int64
					var x, y, z float64
					if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) != nil {
						continue
					}
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; hasMotion {
						continue
					}
					if uint32(rowMap) != m.Map || health <= 0 {
						continue
					}
					if s.isTotemGUID(guid) || spellTargetUnitBlocked(spell, uint32(unitFlags), uint32(flagsExtra), true) {
						continue
					}
					if !implicitEntryConditionsMet(condRows, false, uint32(entry), uint32(low)) {
						continue
					}
					if s.factionTemplatesHostile(m.Faction, uint32(faction)) {
						continue
					}
					if !scopeOK(candidateReferer(guid, false, 0, uint32(faction))) {
						continue
					}
					consider(guid, float32(x), float32(y), float32(z), 0)
				}
			}
		}
	}
	if bestGUID == 0 {
		return 0, false
	}
	if !s.implicitUnitTargetLOSPassesForMotion(spell, m, bestGUID, bestX, bestY, bestZ) {
		return 0, false
	}
	return bestGUID, true
}

// creatureFriendlyConeTargets ports Spell::SelectImplicitConeTargets
// (Spell.cpp:1176) for TARGET_UNIT_CONE_ALLY (59) / TARGET_UNIT_CONE_ENTRY
// (60) for a creature caster, mirroring the session twin
// spellFriendlyConeTargets: coneAngle M_PI/2 (Spell.cpp:8432), the cylinder
// range test (2D + |dz|, Spell.cpp:8409), the caster always in its own cone
// (Position::HasInArc, Position.cpp:120), and per-selecting-effect radius
// (CalcRadius, SpellInfo.cpp:551-574, SPELLMOD_RADIUS via the mod owner;
// zero-radius fallback per effect via GetMaxRange(IsPositiveEffect),
// Spell.cpp:1192). 59 uses TARGET_CHECK_ALLY, 60 uses TARGET_CHECK_ENTRY;
// the first selecting effect's ImplicitTargetConditions filter their own
// leg only through the entry-subset evaluator (firstFriendlyConeEffect,
// the session twin's arm); a 60 leg with no rows still selects any valid
// unit in the cone (nil _condSrcInfo, Spell.cpp:8377-8378). A B-slot 59/60
// behind a NEARBY/CONE/AREA A slot never selects (Spell.cpp:866-900).
// Ally leg gates mirror the creature friendly-nearby twin
// (IsValidAssistTarget, Object.cpp:3087: alive, assist flag arm,
// non-hostile, GM refused, totem refused per Spell.cpp:8333); the entry
// leg carries no totem exclusion and no faction gate (ENTRY has no
// faction-switch case, Spell.cpp:8316; session twin precedent). Every
// candidate needs line of sight from the caster (Spell.cpp:7483-7492, the
// motion LOS twin; the caster itself is exempt). MaxTargets caps the
// merged list (Spell.cpp:1207-1211, RandomResize; no aura term — no motion
// aura model, the entry-area twin's convention).
func (s *Server) creatureFriendlyConeTargets(ctx context.Context, m *creatureMotion, spell wotlk.Spell) ([]uint64, map[uint64]uint8) {
	if s == nil || m == nil || s.Data == nil {
		return nil, nil
	}
	modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID)
	var maxRangePositive, maxRangeNegative float32
	if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
		maxRangePositive = float32(spellDBCMaxRange(true, rangeEntry))
		maxRangeNegative = float32(spellDBCMaxRange(false, rangeEntry))
		if modOwner != nil {
			maxRangePositive = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangePositive)))
			maxRangeNegative = float32(modOwner.applySpellModFloat(spell, spellModRange, float64(maxRangeNegative)))
		}
	}
	radius := float32(0)
	checks := make(map[uint8]struct{})
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		var coneTarget uint32
		switch {
		case isFriendlyConeTargetType(eff.ImplicitTargetA):
			coneTarget = eff.ImplicitTargetA
		case isFriendlyConeTargetType(eff.ImplicitTargetB) && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB):
			coneTarget = eff.ImplicitTargetB
		default:
			continue
		}
		switch coneTarget {
		case implicitTargetConeAlly:
			checks[friendlyCheckAlly] = struct{}{}
		case implicitTargetConeEntry:
			checks[friendlyCheckEntry] = struct{}{}
		}
		if value, ok, err := s.Data.SpellRadius(eff.RadiusIndex, uint32(m.Level)); err == nil && ok && value > 0 {
			if modOwner != nil {
				value = float32(modOwner.applySpellModFloat(spell, spellModRadius, float64(value)))
			}
			if value > radius {
				radius = value
			}
		} else {
			effMaxRange := maxRangeNegative
			if spell.IsPositiveEffect(i) {
				effMaxRange = maxRangePositive
			}
			if effMaxRange > radius {
				radius = effMaxRange
			}
		}
	}
	if radius <= 0 || len(checks) == 0 {
		return nil, nil
	}
	condMask, firstConeTarget := firstFriendlyConeEffect(spell)
	condRows := s.loadImplicitTargetConditionRows(ctx, spell.ID, condMask)
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	_, wantAlly := checks[friendlyCheckAlly]
	_, wantEntry := checks[friendlyCheckEntry]
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	legBits := make(map[uint64]uint8)
	accept := func(guid uint64, x, y, z float32, unitFlags, flagsExtra uint32, health uint32, hostile, totem, isPlayer, isGM bool, entry, spawnLow uint32) {
		if guid == 0 || health == 0 || spellTargetUnitBlocked(spell, unitFlags, flagsExtra, true) {
			return
		}
		dx, dy := float64(x-m.X), float64(y-m.Y)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(z-m.Z)) > float64(radius) {
			return
		}
		if guid != m.GUID {
			if backCone {
				if hasInArc(m.Orientation, m.X, m.Y, x, y, 2*math.Pi-math.Pi/2) {
					return
				}
			} else if !hasInArc(m.Orientation, m.X, m.Y, x, y, math.Pi/2) {
				return
			}
		}
		condMet := implicitEntryConditionsMet(condRows, isPlayer, entry, spawnLow)
		passEntry := wantEntry && (firstConeTarget != implicitTargetConeEntry || condMet)
		passAlly := false
		if wantAlly && (firstConeTarget != implicitTargetConeAlly || condMet) {
			// TARGET_CHECK_ALLY rejects totems (Spell.cpp:8333) and GMs
			// (Object.cpp:3104; the caster is a creature, so no self
			// exemption applies); TARGET_CHECK_ENTRY carries neither
			// exclusion, so both gates sit on the ally path only.
			passAlly = !hostile && !isGM && !totem
		}
		if !passEntry && !passAlly {
			return
		}
		if !s.implicitUnitTargetLOSPassesForMotion(spell, m, guid, x, y, z) {
			return
		}
		if _, dup := seen[guid]; dup {
			return
		}
		seen[guid] = struct{}{}
		targets = append(targets, guid)
		// Per-leg bits for the pet path's check-gated legs (the
		// petAreaAllyCheck family): a target selected by both legs
		// carries both bits, so each effect's leg sees only its own.
		var bits uint8
		if passEntry {
			bits |= petAreaAllyCheckConeEntry
		}
		if passAlly {
			bits |= petAreaAllyCheckConeAlly
		}
		legBits[guid] = bits
	}
	playersOnly := spellSearchPlayersOnly(spell)
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		p := targetSession.player
		if p.Map != m.Map || p.InstanceID != m.InstanceID {
			continue
		}
		candidate := playerPos{Map: p.Map, InstanceID: p.InstanceID, X: p.X, Y: p.Y, Z: p.Z, GUID: targetSession.playerGUID, Race: p.Race, Class: p.Class, Level: p.Level, FactionTemplate: targetSession.effectiveFactionTemplate(p.Race), Reputations: playerReputationMap(p.Reputations), Sess: targetSession}
		accept(targetSession.playerGUID, p.X, p.Y, p.Z, p.UnitFlags, 0, p.Health, s.isAttackableFaction(m.Faction, candidate), s.isTotemGUID(targetSession.playerGUID), true, sessionIsGameMaster(targetSession), 0, 0)
	}
	s.sessionsMu.RUnlock()
	if !playersOnly {
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
			accept(cand.GUID, cand.X, cand.Y, cand.Z, cand.UnitFlags, cand.FlagsExtra, cand.Health, s.factionTemplatesHostile(m.Faction, cand.Faction), s.isTotemGUID(cand.GUID), false, false, cand.Entry, uint32(cand.GUID&0xFFFFFF))
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, m.Map, float64(m.X)-float64(radius), float64(m.X)+float64(radius), float64(m.Y)-float64(radius), float64(m.Y)+float64(radius))
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var low, entry, rowMap, faction, unitFlags, flagsExtra, health int64
					var x, y, z float64
					if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) != nil {
						continue
					}
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; hasMotion {
						continue
					}
					if uint32(rowMap) != m.Map {
						continue
					}
					accept(guid, float32(x), float32(y), float32(z), uint32(unitFlags), uint32(flagsExtra), uint32(health), s.factionTemplatesHostile(m.Faction, uint32(faction)), s.isTotemGUID(guid), false, false, uint32(entry), uint32(low))
				}
			}
		}
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(targets)) > maxTargets {
		rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
		targets = targets[:maxTargets]
		kept := make(map[uint64]struct{}, len(targets))
		for _, guid := range targets {
			kept[guid] = struct{}{}
		}
		for guid := range legBits {
			if _, ok := kept[guid]; !ok {
				delete(legBits, guid)
			}
		}
	}
	return targets, legBits
}
