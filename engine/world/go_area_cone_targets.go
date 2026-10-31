package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetGOSrcArea  uint32 = 51  // TARGET_GAMEOBJECT_SRC_AREA (SharedDefines.h:1487)
	implicitTargetGODestArea uint32 = 52  // TARGET_GAMEOBJECT_DEST_AREA (SharedDefines.h:1488)
	implicitTargetGOCone     uint32 = 108 // TARGET_GAMEOBJECT_CONE (SharedDefines.h:1544)
)

// implicitTargetBlocksSecondSlot reports whether an implicit target's
// selection category consumes the effect's processed slot in
// Spell::SelectEffectImplicitTargets (Spell.cpp:866-900): NEARBY, CONE and
// AREA selections mark the effect processed, so a second slot of the same
// effect in one of those categories never runs. DEFAULT, CHANNEL, TRAJ and
// NYI slots leave the effect unprocessed, so a later slot still selects.
// Categories from the SpellImplicitTargetInfo table (SpellInfo.cpp:218-330).
func implicitTargetBlocksSecondSlot(target uint32) bool {
	switch target {
	case 2, 3, 4, 38, 40, 46, 58, // NEARBY
		24, 54, 59, 60, 104, 108, // CONE
		7, 8, 15, 16, 20, 30, 31, 33, 34, 37, 51, 52, 56, 61, 93: // AREA
		return true
	}
	return false
}

// implicitTargetSecondSlotSkipped reports whether a second slot's
// selection never runs because the first slot already consumed the
// effect's processed slot (Spell.cpp:866-900): the first slot must
// itself select in NEARBY/CONE/AREA, and the second slot must too —
// a DEFAULT, CHANNEL, TRAJ or NYI second slot still runs even when
// the first slot consumed the effect.
func implicitTargetSecondSlotSkipped(first, second uint32) bool {
	return implicitTargetBlocksSecondSlot(first) && implicitTargetBlocksSecondSlot(second)
}

// goAreaLeg is one effect's GO area/cone selection: its center, its own
// CalcRadius and its own ImplicitTargetConditions list. C++ selects per
// effect (Spell.cpp:1227/1176) and each selection adds its matches, so the
// spell's target list is the union of the legs — never the union of the
// condition rows, whose same-ElseGroup rows would AND across effects.
type goAreaLeg struct {
	cx, cy, cz float32
	radius     float64
	condRows   []conditionRow
	// effIndex is the selecting effect: a match on this leg carries only
	// this effect's AddGOTarget bit (Spell.cpp:2193), so the masked
	// variants can thread the exact per-arm mask instead of the
	// category approximation.
	effIndex int
}

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
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok {
			// SpellEffectInfo::CalcRadius (SpellInfo.cpp:551-574): the DBC
			// radius folds SPELLMOD_RADIUS via the mod owner; the max is
			// taken over the modded per-effect radii.
			if mv := s.applySpellModFloat(spell, spellModRadius, float64(value)); mv > radius {
				radius = mv
			}
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
// each effect's own ImplicitTargetConditions gate candidates, via the
// shared OBJECT_ENTRY_GUID loader/narrowing/matcher from the 40 run,
// unioned across the effect legs below.
// Every match is kept (no nearest-shrink); the MaxTargets cap mirrors the
// area path (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier +
// RandomResize). An empty area list adds no targets and never fails the
// cast — area selection has no BAD_IMPLICIT_TARGETS gate.
func (s *session) spellGOAreaTargets(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) []uint64 {
	targets, _ := s.spellGOAreaTargetsWithMasks(ctx, spell, spellID, target)
	return targets
}

// spellGOAreaTargetsWithMasks is the threaded-mask twin of
// spellGOAreaTargets: the same per-effect legs, but each match also
// records the leg effect's bit, so a GUID matched by several legs carries
// the union of exactly those legs' bits (C++ AddGOTarget merges masks on
// dup, Spell.cpp:2225). GUIDs dropped by the MaxTargets cap keep a mask
// entry but are never consulted, since the fan-out and packet filter
// iterate the capped list.
func (s *session) spellGOAreaTargetsWithMasks(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) ([]uint64, map[uint64]uint32) {
	return s.spellGOAreaTargetsWithMasksAndCenters(ctx, spell, spellID, target, nil)
}

// spellGOAreaTargetsWithMasksAndCenters is spellGOAreaTargetsWithMasks with
// per-effect arm-time destination centers for its DEST legs: C++ selects
// each area leg at its own position in the per-effect SelectSpellTargets
// loop (Spell.cpp:758-779), so a DEST leg centers on the running
// destination at its arm time (areaArmTimeDestSnapshot), not the
// spell-final one. A nil centers map, or a leg with no entry, keeps the
// spell-final destination (the previous behavior); a present-but-unstamped
// entry leaves the leg on the caster default, since its arm-time
// destination was unset and the spell-final dest must not center it.
func (s *session) spellGOAreaTargetsWithMasksAndCenters(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData, centers map[int]sessionDestSnapshot) ([]uint64, map[uint64]uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil, nil
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1809-1842): the GOBJ base mask
	// narrows to zero under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so the search contributes nothing.
	if spellSearchPlayersOnly(spell) {
		return nil, nil
	}
	// Per-effect legs (Spell.cpp:1227: SelectImplicitAreaTargets runs per
	// effect with that effect's own ImplicitTargetConditions, and each
	// selection adds its matches): the selecting slot is A when A is
	// 51/52, else B when B is 51/52 and A did not consume the effect's
	// processed slot (Spell.cpp:866-900). TARGET_CHECK_DEFAULT has no
	// faction case, but the conditions still gate: the area check ends
	// in WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316), which
	// runs IsObjectMeetToConditions on the effect's own list — DEFAULT
	// disables the faction switch, not the condition list.
	destX, destY, destZ := s.player.X, s.player.Y, s.player.Z
	if target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		destX, destY, destZ = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		// Spell.cpp:1258 CheckDst: an explicit unit target becomes the
		// destination for DEST-reference area selection.
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			destX, destY, destZ = destination.X, destination.Y, destination.Z
		}
	}
	legs := make([]goAreaLeg, 0)
	maxRadius := float64(0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		var legTarget uint32
		switch {
		case eff.ImplicitTargetA == implicitTargetGOSrcArea || eff.ImplicitTargetA == implicitTargetGODestArea:
			legTarget = eff.ImplicitTargetA
		case (eff.ImplicitTargetB == implicitTargetGOSrcArea || eff.ImplicitTargetB == implicitTargetGODestArea) && !implicitTargetBlocksSecondSlot(eff.ImplicitTargetA):
			legTarget = eff.ImplicitTargetB
		default:
			continue
		}
		radius := s.spellGOAreaRadius(spell, 1<<uint(i))
		if radius <= 0 {
			continue
		}
		condRows := s.loadImplicitTargetConditions(ctx, spellID, 1<<uint(i))
		// GetSearcherTypeMask (Spell.cpp:1809): the base mask is
		// gameobject-only for TARGET_OBJECT_TYPE_GOBJ, narrowed by
		// CONDITION_OBJECT_ENTRY_GUID rows (ConditionMgr.cpp:843).
		_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
		if !allowGO {
			continue
		}
		leg := goAreaLeg{cx: s.player.X, cy: s.player.Y, cz: s.player.Z, radius: radius, condRows: condRows, effIndex: i}
		if legTarget == implicitTargetGODestArea {
			if c, ok := centers[i]; ok {
				if c.Has {
					leg.cx, leg.cy, leg.cz = c.X, c.Y, c.Z
				}
			} else {
				leg.cx, leg.cy, leg.cz = destX, destY, destZ
			}
		}
		legs = append(legs, leg)
		if radius > maxRadius {
			maxRadius = radius
		}
	}
	if len(legs) == 0 {
		return nil, nil
	}
	targets := make([]uint64, 0)
	masks := make(map[uint64]uint32)
	seen := make(map[uint64]struct{})
	s.scanNearbyGameObjects(ctx, float32(maxRadius), func(g nearbyGameObject) {
		// GameObject::IsInRange (GameObject.cpp:2105) without the
		// display-info dimension term: 3D distance from the leg's center.
		// A match on any leg joins the list (per-effect selections union),
		// and each matching leg contributes its own effect's bit.
		var matchMask uint32
		for _, leg := range legs {
			if distance3D(g.x, g.y, g.z, leg.cx, leg.cy, leg.cz) <= leg.radius && implicitEntryGOConditionsMet(leg.condRows, g.entry, g.spawnLow) {
				matchMask |= 1 << uint(leg.effIndex)
			}
		}
		if matchMask == 0 {
			return
		}
		masks[g.guid] |= matchMask
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
	return targets, masks
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
// spellGOAreaTargets, per effect and unioned across the effect legs. Every match is kept (no nearest-shrink); the
// MaxTargets cap mirrors the cone path (Spell.cpp:1212).
func (s *session) spellGOConeTargets(ctx context.Context, spell wotlk.Spell, spellID uint32) []uint64 {
	targets, _ := s.spellGOConeTargetsWithMasks(ctx, spell, spellID)
	return targets
}

// spellGOConeTargetsWithMasks is the threaded-mask twin of
// spellGOConeTargets, as spellGOAreaTargetsWithMasks is of
// spellGOAreaTargets: each matching leg contributes its own effect's bit.
func (s *session) spellGOConeTargetsWithMasks(ctx context.Context, spell wotlk.Spell, spellID uint32) ([]uint64, map[uint64]uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil, nil
	}
	// Spell::GetSearcherTypeMask (Spell.cpp:1809-1842): the GOBJ base mask
	// narrows to zero under SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS, so the search contributes nothing.
	if spellSearchPlayersOnly(spell) {
		return nil, nil
	}
	// Per-effect legs, as in spellGOAreaTargets: SelectImplicitConeTargets
	// (Spell.cpp:1176) runs per effect with that effect's own
	// ImplicitTargetConditions (condList, Spell.cpp:1190), and each
	// selection adds its matches. The selecting slot is A when A is 108,
	// else B when B is 108 and A did not consume the effect's processed
	// slot (Spell.cpp:866-900).
	// SpellMgr.cpp:2840 — CU_CONE_BACK comes from SpellVisual[0] == 3879.
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	legs := make([]goAreaLeg, 0)
	maxRadius := float64(0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		selects := eff.ImplicitTargetA == implicitTargetGOCone ||
			(eff.ImplicitTargetB == implicitTargetGOCone && !implicitTargetBlocksSecondSlot(eff.ImplicitTargetA))
		if !selects {
			continue
		}
		radius := s.spellGOAreaRadius(spell, 1<<uint(i))
		if radius <= 0 {
			continue
		}
		condRows := s.loadImplicitTargetConditions(ctx, spellID, 1<<uint(i))
		_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
		if !allowGO {
			continue
		}
		legs = append(legs, goAreaLeg{cx: s.player.X, cy: s.player.Y, cz: s.player.Z, radius: radius, condRows: condRows, effIndex: i})
		if radius > maxRadius {
			maxRadius = radius
		}
	}
	if len(legs) == 0 {
		return nil, nil
	}
	targets := make([]uint64, 0)
	masks := make(map[uint64]uint32)
	seen := make(map[uint64]struct{})
	s.scanNearbyGameObjects(ctx, float32(maxRadius), func(g nearbyGameObject) {
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
		var matchMask uint32
		for _, leg := range legs {
			if distance3D(g.x, g.y, g.z, s.player.X, s.player.Y, s.player.Z) <= leg.radius && implicitEntryGOConditionsMet(leg.condRows, g.entry, g.spawnLow) {
				matchMask |= 1 << uint(leg.effIndex)
			}
		}
		if matchMask == 0 {
			return
		}
		masks[g.guid] |= matchMask
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
	return targets, masks
}

// creatureGOAreaRadius mirrors spellGOAreaRadius for a creature caster:
// max CalcRadius over the matched effects at the caster's level, with the
// Spell.cpp:1281 range fallback (GetMaxRange split by isHarmfulSpell). No
// SPELLMOD_RADIUS fold: a creature caster carries no spellmod owner session
// to fold through, so the raw DBC radius stands.
func (s *Server) creatureGOAreaRadius(m *creatureMotion, spell wotlk.Spell, effectMask uint32) float64 {
	radius := float64(0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 || effectMask&(1<<uint(i)) == 0 {
			continue
		}
		if value, ok, err := s.Data.SpellRadius(eff.RadiusIndex, casterLevel(m)); err == nil && ok {
			if float64(value) > radius {
				radius = float64(value)
			}
		}
	}
	if radius <= 0 {
		if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			radius = float64(rangeEntry.MaxHostile)
			if !isHarmfulSpell(spell) {
				radius = float64(rangeEntry.MaxFriendly)
			}
		}
	}
	return radius
}

// creatureGOAreaDestCenter resolves the DEST-area center for a creature
// caster: the cast's resolved destination when one exists, else the
// explicit unit target's position (CheckDst, Spell.cpp:1258), else the
// caster position (the session helper's caster fallback).
func (s *Server) creatureGOAreaDestCenter(m *creatureMotion, hasDest bool, destX, destY, destZ float32, targetGUID uint64) (float32, float32, float32) {
	if hasDest {
		return destX, destY, destZ
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil && ts.player.Map == m.Map && ts.player.InstanceID == m.InstanceID {
		return ts.player.X, ts.player.Y, ts.player.Z
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return tm.X, tm.Y, tm.Z
	}
	return m.X, m.Y, m.Z
}

// creatureGOAreaTargets ports spellGOAreaTargets (Spell::SelectImplicitAreaTargets,
// Spell.cpp:1227, + SearchAreaTargets, Spell.cpp:1881) for a creature caster:
// TARGET_GAMEOBJECT_SRC_AREA (51) / TARGET_GAMEOBJECT_DEST_AREA (52), per-effect
// legs unioned, each gated on its own ImplicitTargetConditions, scanned per leg
// around the leg's own center (the session scan centers on the caster, which
// would miss DEST-area matches far from the caster). The MaxTargets cap is the
// DBC value alone: a creature caster has no total-aura-modifier helper for the
// MOD_MAX_AFFECTED_TARGETS fold.
func (s *Server) creatureGOAreaTargets(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, hasDest bool, destX, destY, destZ float32, targetGUID uint64) []uint64 {
	targets, _ := s.creatureGOAreaTargetsWithMasks(ctx, m, spell, spellID, hasDest, destX, destY, destZ, targetGUID)
	return targets
}

// creatureGOAreaTargetsWithMasks is the threaded-mask twin of
// creatureGOAreaTargets: the same per-effect legs, but each match also
// records the leg effect's bit (Spell.cpp:2193), so a GUID matched by
// several legs carries the union of exactly those legs' bits. GUIDs
// dropped by the MaxTargets cap keep a mask entry but are never
// consulted, since the fan-out and packet filter iterate the capped list.
func (s *Server) creatureGOAreaTargetsWithMasks(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, hasDest bool, destX, destY, destZ float32, targetGUID uint64) ([]uint64, map[uint64]uint32) {
	return s.creatureGOAreaTargetsWithMasksAndCenters(ctx, m, spell, spellID, hasDest, destX, destY, destZ, targetGUID, nil)
}

// creatureGOAreaTargetsWithMasksAndCenters is
// creatureGOAreaTargetsWithMasks with per-effect arm-time destination
// centers for its DEST legs, the creature twin of
// spellGOAreaTargetsWithMasksAndCenters: C++ selects each area leg at its
// own position in the per-effect SelectSpellTargets loop
// (Spell.cpp:758-779), so a DEST leg centers on the running destination
// at its arm time (creatureAreaArmTimeDestSnapshot), not the spell-final
// one. A nil centers map, or a leg with no entry, keeps the previous
// creatureGOAreaDestCenter behavior; a present-but-unstamped entry
// leaves the leg on the caster default, since its arm-time destination
// was unset and the spell-final dest must not center it.
func (s *Server) creatureGOAreaTargetsWithMasksAndCenters(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, hasDest bool, destX, destY, destZ float32, targetGUID uint64, centers map[int]creatureDestSnapshot) ([]uint64, map[uint64]uint32) {
	if s == nil || m == nil || s.Data == nil {
		return nil, nil
	}
	if spellSearchPlayersOnly(spell) {
		return nil, nil
	}
	legs := make([]goAreaLeg, 0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		var legTarget uint32
		switch {
		case eff.ImplicitTargetA == implicitTargetGOSrcArea || eff.ImplicitTargetA == implicitTargetGODestArea:
			legTarget = eff.ImplicitTargetA
		case (eff.ImplicitTargetB == implicitTargetGOSrcArea || eff.ImplicitTargetB == implicitTargetGODestArea) && !implicitTargetBlocksSecondSlot(eff.ImplicitTargetA):
			legTarget = eff.ImplicitTargetB
		default:
			continue
		}
		radius := s.creatureGOAreaRadius(m, spell, 1<<uint(i))
		if radius <= 0 {
			continue
		}
		condRows := s.loadImplicitTargetConditionRows(ctx, spellID, 1<<uint(i))
		_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
		if !allowGO {
			continue
		}
		leg := goAreaLeg{cx: m.X, cy: m.Y, cz: m.Z, radius: radius, condRows: condRows, effIndex: i}
		if legTarget == implicitTargetGODestArea {
			if c, ok := centers[i]; ok {
				if c.Has {
					leg.cx, leg.cy, leg.cz = c.X, c.Y, c.Z
				}
			} else {
				leg.cx, leg.cy, leg.cz = s.creatureGOAreaDestCenter(m, hasDest, destX, destY, destZ, targetGUID)
			}
		}
		legs = append(legs, leg)
	}
	if len(legs) == 0 {
		return nil, nil
	}
	targets := make([]uint64, 0)
	masks := make(map[uint64]uint32)
	seen := make(map[uint64]struct{})
	for _, leg := range legs {
		s.scanNearbyGameObjectsAtServer(ctx, m.Map, m.InstanceID, leg.cx, leg.cy, float32(leg.radius), func(g nearbyGameObject) {
			if distance3D(g.x, g.y, g.z, leg.cx, leg.cy, leg.cz) > leg.radius || !implicitEntryGOConditionsMet(leg.condRows, g.entry, g.spawnLow) {
				return
			}
			masks[g.guid] |= 1 << uint(leg.effIndex)
			if _, ok := seen[g.guid]; ok {
				return
			}
			seen[g.guid] = struct{}{}
			targets = append(targets, g.guid)
		})
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(targets)) > maxTargets {
		rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
		targets = targets[:maxTargets]
	}
	return targets, masks
}

// creatureGOConeTargets ports spellGOConeTargets (Spell::SelectImplicitConeTargets,
// Spell.cpp:1176) for a creature caster: TARGET_GAMEOBJECT_CONE (108), front cone
// at M_PI/2 (WorldObjectSpellConeTargetCheck, Spell.cpp:8432), CU_CONE_BACK when
// SpellVisual[0] == 3879 (SpellMgr.cpp:2840), per-effect legs unioned on their own
// ImplicitTargetConditions, DBC MaxTargets cap alone as in creatureGOAreaTargets.
func (s *Server) creatureGOConeTargets(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32) []uint64 {
	targets, _ := s.creatureGOConeTargetsWithMasks(ctx, m, spell, spellID)
	return targets
}

// creatureGOConeTargetsWithMasks is the threaded-mask twin of
// creatureGOConeTargets, as creatureGOAreaTargetsWithMasks is of
// creatureGOAreaTargets: each matching leg contributes its own effect's bit.
func (s *Server) creatureGOConeTargetsWithMasks(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32) ([]uint64, map[uint64]uint32) {
	if s == nil || m == nil || s.Data == nil {
		return nil, nil
	}
	if spellSearchPlayersOnly(spell) {
		return nil, nil
	}
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	legs := make([]goAreaLeg, 0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		selects := eff.ImplicitTargetA == implicitTargetGOCone ||
			(eff.ImplicitTargetB == implicitTargetGOCone && !implicitTargetBlocksSecondSlot(eff.ImplicitTargetA))
		if !selects {
			continue
		}
		radius := s.creatureGOAreaRadius(m, spell, 1<<uint(i))
		if radius <= 0 {
			continue
		}
		condRows := s.loadImplicitTargetConditionRows(ctx, spellID, 1<<uint(i))
		_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
		if !allowGO {
			continue
		}
		legs = append(legs, goAreaLeg{cx: m.X, cy: m.Y, cz: m.Z, radius: radius, condRows: condRows, effIndex: i})
	}
	if len(legs) == 0 {
		return nil, nil
	}
	targets := make([]uint64, 0)
	masks := make(map[uint64]uint32)
	seen := make(map[uint64]struct{})
	for _, leg := range legs {
		s.scanNearbyGameObjectsAtServer(ctx, m.Map, m.InstanceID, m.X, m.Y, float32(leg.radius), func(g nearbyGameObject) {
			if backCone {
				if hasInArc(m.Orientation, m.X, m.Y, g.x, g.y, 2*math.Pi-math.Pi/2) {
					return
				}
			} else if !hasInArc(m.Orientation, m.X, m.Y, g.x, g.y, math.Pi/2) {
				return
			}
			if distance3D(g.x, g.y, g.z, m.X, m.Y, m.Z) > leg.radius || !implicitEntryGOConditionsMet(leg.condRows, g.entry, g.spawnLow) {
				return
			}
			masks[g.guid] |= 1 << uint(leg.effIndex)
			if _, ok := seen[g.guid]; ok {
				return
			}
			seen[g.guid] = struct{}{}
			targets = append(targets, g.guid)
		})
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(targets)) > maxTargets {
		rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
		targets = targets[:maxTargets]
	}
	return targets, masks
}
