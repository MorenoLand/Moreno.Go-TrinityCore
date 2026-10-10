package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetGOSrcArea      uint32 = 51  // TARGET_GAMEOBJECT_SRC_AREA (SharedDefines.h:1487)
	implicitTargetGODestArea     uint32 = 52  // TARGET_GAMEOBJECT_DEST_AREA (SharedDefines.h:1488)
	implicitTargetGOCone         uint32 = 108 // TARGET_GAMEOBJECT_CONE (SharedDefines.h:1544)
)

// 23 TARGET_GAMEOBJECT_TARGET / 26 TARGET_GAMEOBJECT_ITEM_TARGET need no
// branch (see the checkpoint audit for this run): both dispatch to
// Spell::SelectImplicitTargetObjectTargets (Spell.cpp:1558), which selects
// only the client's explicit object/item target — Go's cast packet already
// reads that GUID (TARGET_FLAG_GAMEOBJECT 0x800 is in the unit wire mask)
// and the hitTargets chain passes it through.

// isGOAreaSpell reports spells carrying TARGET_GAMEOBJECT_SRC_AREA (51) or
// TARGET_GAMEOBJECT_DEST_AREA (52) on any effect: {GOBJ, SRC|DEST, AREA,
// TARGET_CHECK_DEFAULT} (SpellInfo.cpp:218-330;
// Spell::SelectImplicitAreaTargets, Spell.cpp:1227).
func isGOAreaSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == implicitTargetGOSrcArea || targetType == implicitTargetGODestArea {
				return true
			}
		}
	}
	return false
}

// isGOConeSpell reports spells carrying TARGET_GAMEOBJECT_CONE (108) on any
// effect: {GOBJ, CASTER, CONE, TARGET_CHECK_DEFAULT, DIR_FRONT}
// (SpellInfo.cpp:218-330; Spell::SelectImplicitConeTargets, Spell.cpp:1176).
func isGOConeSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == implicitTargetGOCone {
				return true
			}
		}
	}
	return false
}

// spellGOAreaRadius mirrors the radius arm of Spell::SelectImplicitAreaTargets
// (Spell.cpp:1279): max CalcRadius over the matched effects, with the
// Spell.cpp:1281 workaround — an unset radius falls back to
// GetMaxRange(IsPositiveEffect) (Go: the SpellRange MaxHostile/MaxFriendly
// split via isHarmfulSpell, like the 7/8 path). The C++ RadiusMod
// multiplier (m_spellValue) has no Go equivalent.
func (s *session) spellGOAreaRadius(spell wotlk.Spell, effectMask uint32) float64 {
	radius := float64(0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 || effectMask&(1<<uint(i)) == 0 {
			continue
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && float64(value) > radius {
			radius = float64(value)
		}
	}
	if radius <= 0 {
		if rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			radius = float64(rangeEntry.MaxHostile)
			if !isHarmfulSpell(spell) {
				radius = float64(rangeEntry.MaxFriendly)
			}
		}
	}
	return radius
}

// spellGOAreaTargets ports Spell::SelectImplicitAreaTargets (Spell.cpp:1227)
// + Spell::SearchAreaTargets (Spell.cpp:1881) for TARGET_GAMEOBJECT_SRC_AREA
// (51) / TARGET_GAMEOBJECT_DEST_AREA (52): {GOBJ, SRC|DEST, AREA,
// TARGET_CHECK_DEFAULT}.
//
// Center: GetSrcPos() for 51, GetDstPos() for 52. Go tracks no src
// position, so SRC defaults to the caster — the same heuristic as the 7
// path; C++'s CheckSrc (Spell.cpp:6519) also defaults an unset src to the
// caster. DEST uses the resolved destination (client dest, else the
// explicit unit via CheckDst, like the 7/8 path).
// Selection: WorldObjectSpellAreaTargetCheck (Spell.cpp:8409) — for
// gameobjects GameObject::IsInRange, "isInRange including the dimension of
// the GO" (GameObject.cpp:2105: an oriented GeoBox overlap test against
// GameObjectDisplayInfo). Go loads no GameObjectDisplayInfo, so the
// dimension term degrades to the C++ no-display-info fallback — a plain 3D
// distance test (IsWithinDist3d) against the GO position. Then
// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316):
// TARGET_CHECK_DEFAULT has no faction case in the switch, and
// SpellInfo::CheckTarget is always valid for gameobjects
// (SpellInfo.cpp:1707: "other types of objects - always valid"), so only
// the spell's ImplicitTargetConditions gate candidates, via the shared
// OBJECT_ENTRY_GUID loader/narrowing/matcher from the 40 run.
// Every match is kept (no nearest-shrink); the MaxTargets cap mirrors the
// area path (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier +
// RandomResize). An empty area list adds no targets and never fails the
// cast — area selection has no BAD_IMPLICIT_TARGETS gate.
func (s *session) spellGOAreaTargets(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1809-1842): the GOBJ base mask
	// narrows to zero under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so the search contributes nothing.
	if spellSearchPlayersOnly(spell) {
		return nil
	}
	effectMask := entryImplicitEffectMask(spell, implicitTargetGOSrcArea) | entryImplicitEffectMask(spell, implicitTargetGODestArea)
	radius := s.spellGOAreaRadius(spell, effectMask)
	if radius <= 0 {
		return nil
	}
	centerX, centerY, centerZ := s.player.X, s.player.Y, s.player.Z
	if entryImplicitEffectMask(spell, implicitTargetGODestArea) != 0 {
		if target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
			centerX, centerY, centerZ = target.Destination.X, target.Destination.Y, target.Destination.Z
		} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
			// Spell.cpp:1258 CheckDst: an explicit unit target becomes the
			// destination for DEST-reference area selection.
			if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
				centerX, centerY, centerZ = destination.X, destination.Y, destination.Z
			}
		}
	}
	condRows := s.loadImplicitTargetConditions(ctx, spellID, effectMask)
	// GetSearcherTypeMask (Spell.cpp:1809): the base mask is gameobject-only
	// for TARGET_OBJECT_TYPE_GOBJ, narrowed by CONDITION_OBJECT_ENTRY_GUID
	// rows (ConditionMgr.cpp:843).
	_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
	if !allowGO {
		return nil
	}
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	s.scanNearbyGameObjects(ctx, float32(radius), func(g nearbyGameObject) {
		// GameObject::IsInRange (GameObject.cpp:2105) without the
		// display-info dimension term: 3D distance from the center.
		if distance3D(g.x, g.y, g.z, centerX, centerY, centerZ) > radius {
			return
		}
		if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
			return
		}
		if _, ok := seen[g.guid]; ok {
			return
		}
		seen[g.guid] = struct{}{}
		targets = append(targets, g.guid)
	})
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		// Spell.cpp:1293 — cap to MaxAffectedTargets plus
		// SPELL_AURA_MOD_MAX_AFFECTED_TARGETS aura modifiers, then
		// Trinity::Containers::RandomResize.
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}

// spellGOConeTargets ports Spell::SelectImplicitConeTargets (Spell.cpp:1176)
// for TARGET_GAMEOBJECT_CONE (108): {GOBJ, CASTER, CONE, TARGET_CHECK_DEFAULT,
// DIR_FRONT}.
//
// coneAngle = M_PI/2 (WorldObjectSpellConeTargetCheck, Spell.cpp:8432); the
// direction gate is C++-custom-attr driven: CU_CONE_BACK (set in
// SpellMgr.cpp:2840 when SpellVisual[0] == 3879) keeps targets in the back
// cone, CU_CONE_LINE has no write site in C++ (verified: only the read at
// Spell.cpp:8439) and is a no-op, otherwise the front cone
// (Position::HasInArc, Position.cpp:120: "always have self in arc" is
// vacuous here — a GO never is the caster). The range and condition terms
// are the GO halves of WorldObjectSpellConeTargetCheck/
// WorldObjectSpellAreaTargetCheck (Spell.cpp:8432/8409) as in
// spellGOAreaTargets. Every match is kept (no nearest-shrink); the
// MaxTargets cap mirrors the cone path (Spell.cpp:1212).
func (s *session) spellGOConeTargets(ctx context.Context, spell wotlk.Spell, spellID uint32) []uint64 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1809-1842): the GOBJ base mask
	// narrows to zero under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so the search contributes nothing.
	if spellSearchPlayersOnly(spell) {
		return nil
	}
	effectMask := entryImplicitEffectMask(spell, implicitTargetGOCone)
	radius := s.spellGOAreaRadius(spell, effectMask)
	if radius <= 0 {
		return nil
	}
	// SpellMgr.cpp:2840 — CU_CONE_BACK comes from SpellVisual[0] == 3879.
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	condRows := s.loadImplicitTargetConditions(ctx, spellID, effectMask)
	_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
	if !allowGO {
		return nil
	}
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	s.scanNearbyGameObjects(ctx, float32(radius), func(g nearbyGameObject) {
		if backCone {
			// WorldObject::isInBack (Object.cpp:1417): !HasInArc(2*pi -
			// arc, target).
			if hasInArc(s.player.Orientation, s.player.X, s.player.Y, g.x, g.y, 2*math.Pi-math.Pi/2) {
				return
			}
		} else if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, g.x, g.y, math.Pi/2) {
			// WorldObject::isInFront (Object.cpp:1412): HasInArc(pi/2).
			return
		}
		if distance3D(g.x, g.y, g.z, s.player.X, s.player.Y, s.player.Z) > radius {
			return
		}
		if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
			return
		}
		if _, ok := seen[g.guid]; ok {
			return
		}
		seen[g.guid] = struct{}{}
		targets = append(targets, g.guid)
	})
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}
