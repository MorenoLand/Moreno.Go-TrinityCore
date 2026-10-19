package world

import (
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
			eff.ImplicitTargetA == implicitTargetDestTraj || eff.ImplicitTargetB == implicitTargetDestTraj {
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
// ok is false when the spell carries no bridgeable dest-family target.
//
// Documented no-bridge: TARGET_DEST_TRAJ (89) beyond the seed —
// SelectImplicitTrajTargets early-returns on m_speed == 0 (Spell.cpp:1628)
// and creature AI casts never set a trajectory (Unit::CastSpell builds
// default targets); TARGET_DEST_CASTER_FISHING (39, player bobber model);
// the 76/106 channel destinations (motions hold channel duration but no
// channel target); TARGET_DEST_NEARBY_ENTRY (46, needs a motion-anchored
// entry-object candidate scan — the session twin's scan is
// session-anchored).
func (s *Server) resolveCreatureSpellDestination(m *creatureMotion, spell wotlk.Spell, targetGUID uint64) (float32, float32, float32, bool) {
	if s == nil || m == nil {
		return 0, 0, 0, false
	}
	if !spellHasCreatureDestTarget(spell) {
		return 0, 0, 0, false
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
	for _, eff := range spell.Effects {
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
			}
		}
	}
	return x, y, z, true
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
