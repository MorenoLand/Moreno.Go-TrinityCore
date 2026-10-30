package world

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetDestTargetEnemy  uint32 = 53 // TARGET_DEST_TARGET_ENEMY (SharedDefines.h:1489)
	implicitTargetDestTargetAny    uint32 = 63 // TARGET_DEST_TARGET_ANY (SharedDefines.h:1499)
	implicitTargetDestTargetFront  uint32 = 64 // TARGET_DEST_TARGET_FRONT (SharedDefines.h:1500)
	implicitTargetDestTargetBack   uint32 = 65
	implicitTargetDestTargetRight  uint32 = 66
	implicitTargetDestTargetLeft   uint32 = 67
	implicitTargetDestTargetFRight uint32 = 68
	implicitTargetDestTargetBRight uint32 = 69
	implicitTargetDestTargetBLeft  uint32 = 70
	implicitTargetDestTargetFLeft  uint32 = 71
	implicitTargetDestTargetRandom uint32 = 74 // TARGET_DEST_TARGET_RANDOM (SharedDefines.h:1510)
	implicitTargetDestTargetRadius uint32 = 75 // TARGET_DEST_TARGET_RADIUS (SharedDefines.h:1511)

	implicitTargetDestDynobjEnemy uint32 = 28 // TARGET_DEST_DYNOBJ_ENEMY (SharedDefines.h:1464)
	implicitTargetDestDynobjAlly  uint32 = 29 // TARGET_DEST_DYNOBJ_ALLY (SharedDefines.h:1465)
	implicitTargetDestDestFront   uint32 = 78 // TARGET_DEST_DEST_FRONT (SharedDefines.h:1514)
	implicitTargetDestDestBack    uint32 = 79
	implicitTargetDestDestRight   uint32 = 80
	implicitTargetDestDestLeft    uint32 = 81
	implicitTargetDestDestFRight  uint32 = 82
	implicitTargetDestDestBRight  uint32 = 83
	implicitTargetDestDestBLeft   uint32 = 84
	implicitTargetDestDestFLeft   uint32 = 85
	implicitTargetDestDestRandom  uint32 = 86 // TARGET_DEST_DEST_RANDOM (SharedDefines.h:1520)
	implicitTargetDestDest        uint32 = 87 // TARGET_DEST_DEST (SharedDefines.h:1521)
	implicitTargetDestDynobjNone  uint32 = 88 // TARGET_DEST_DYNOBJ_NONE (SharedDefines.h:1522)
	implicitTargetDestDestRadius  uint32 = 91 // TARGET_DEST_DEST_RADIUS (SharedDefines.h:1524)
)

// isTargetDestTarget reports TARGET_REFERENCE_TYPE_TARGET dest values:
// SelectImplicitTargetDestTargets (Spell.cpp:1433).
func isTargetDestTarget(target uint32) bool {
	switch target {
	case implicitTargetDestTargetEnemy, implicitTargetDestTargetAny,
		implicitTargetDestTargetFront, implicitTargetDestTargetBack,
		implicitTargetDestTargetRight, implicitTargetDestTargetLeft,
		implicitTargetDestTargetFRight, implicitTargetDestTargetBRight,
		implicitTargetDestTargetBLeft, implicitTargetDestTargetFLeft,
		implicitTargetDestTargetRandom, implicitTargetDestTargetRadius:
		return true
	}
	return false
}

// isDestDestTarget reports TARGET_REFERENCE_TYPE_DEST dest values:
// SelectImplicitDestDestTargets (Spell.cpp:1464).
func isDestDestTarget(target uint32) bool {
	switch target {
	case implicitTargetDestDynobjEnemy, implicitTargetDestDynobjAlly,
		implicitTargetDestDestFront, implicitTargetDestDestBack,
		implicitTargetDestDestRight, implicitTargetDestDestLeft,
		implicitTargetDestDestFRight, implicitTargetDestDestBRight,
		implicitTargetDestDestBLeft, implicitTargetDestDestFLeft,
		implicitTargetDestDestRandom, implicitTargetDestDest,
		implicitTargetDestDynobjNone, implicitTargetDestDestRadius:
		return true
	}
	return false
}

// destDirectionAngle mirrors SpellImplicitTargetInfo::CalcDirectionAngle
// (SpellInfo.cpp:102-124) for the target-dest and dest-dest families. The
// directional values share the same static DirectionType rows as the
// caster-dest family, so they delegate to casterDestDirectionAngle; the
// random/radius values carry TARGET_DIR_RANDOM.
func destDirectionAngle(target uint32) float32 {
	switch target {
	case implicitTargetDestTargetFront, implicitTargetDestDestFront:
		return casterDestDirectionAngle(implicitTargetDestCasterFront)
	case implicitTargetDestTargetBack, implicitTargetDestDestBack:
		return casterDestDirectionAngle(implicitTargetDestCasterBack)
	case implicitTargetDestTargetRight, implicitTargetDestDestRight:
		return casterDestDirectionAngle(implicitTargetDestCasterRight)
	case implicitTargetDestTargetLeft, implicitTargetDestDestLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterLeft)
	case implicitTargetDestTargetFRight, implicitTargetDestDestFRight:
		return casterDestDirectionAngle(implicitTargetDestCasterFrontRight)
	case implicitTargetDestTargetBRight, implicitTargetDestDestBRight:
		return casterDestDirectionAngle(implicitTargetDestCasterBackRight)
	case implicitTargetDestTargetBLeft, implicitTargetDestDestBLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterBackLeft)
	case implicitTargetDestTargetFLeft, implicitTargetDestDestFLeft:
		return casterDestDirectionAngle(implicitTargetDestCasterFrontLeft)
	case implicitTargetDestTargetRandom, implicitTargetDestTargetRadius,
		implicitTargetDestDestRandom, implicitTargetDestDestRadius:
		return rand.Float32() * 2 * float32(math.Pi)
	default:
		return 0
	}
}

// resolveTargetDestPosition mirrors Spell::SelectImplicitTargetDestTargets
// (Spell.cpp:1433-1462): 53/63 keep the target position; otherwise the
// target position moved by CalcRadius(nullptr) along CalcDirectionAngle (74
// scales the distance by rand_norm, 75 uses a random angle). There is no
// combat-reach floor on this path, and the angle is relative to the target's
// orientation (target->MovePositionToFirstCollision). Go has no collision or
// map height queries, so the move degrades to a plain offset and z is kept.
func resolveTargetDestPosition(store *wotlk.Store, target uint32, radiusIndex uint32, tx, ty, tz, tOrientation float32) (float32, float32, float32) {
	if target == implicitTargetDestTargetEnemy || target == implicitTargetDestTargetAny {
		return tx, ty, tz
	}
	radius := float32(0)
	if store != nil {
		// CalcRadius(nullptr): base radius only, no per-level or max terms (SpellInfo.cpp:551-574).
		if value, ok, err := store.SpellRadius(radiusIndex, 0); err == nil && ok {
			radius = value
		}
	}
	dist := radius
	if target == implicitTargetDestTargetRandom {
		dist *= rand.Float32()
	}
	angle := destDirectionAngle(target) + tOrientation
	return tx + dist*float32(math.Cos(float64(angle))), ty + dist*float32(math.Sin(float64(angle))), tz
}

// resolveDestDestPosition mirrors Spell::SelectImplicitDestDestTargets
// (Spell.cpp:1464-1499): 28/29/87/88 leave the destination unchanged (the
// C++ early return); otherwise the current destination moved by
// CalcRadius(m_caster) along CalcDirectionAngle (86 scales the distance by
// rand_norm, 91 uses a random angle). The angle is relative to the caster's
// orientation (m_caster->MovePositionToFirstCollision), the start position
// is the current destination, and there is no combat-reach floor. Collision
// degrades to a plain offset, z is kept. changed reports whether the
// destination was modified.
func resolveDestDestPosition(store *wotlk.Store, target uint32, radiusIndex uint32, level uint32, dx, dy, dz, cOrientation float32, modRadius func(float32) float32) (float32, float32, float32, bool) {
	switch target {
	case implicitTargetDestDynobjEnemy, implicitTargetDestDynobjAlly,
		implicitTargetDestDest, implicitTargetDestDynobjNone:
		return dx, dy, dz, false
	}
	radius := float32(0)
	if store != nil {
		if value, ok, err := store.SpellRadius(radiusIndex, level); err == nil && ok {
			radius = value
		}
	}
	// SpellInfo.cpp:560-563: CalcRadius folds SPELLMOD_RADIUS through the
	// caster's spell-mod owner after the per-level/max clamp.
	if modRadius != nil {
		radius = modRadius(radius)
	}
	dist := radius
	if target == implicitTargetDestDestRandom {
		dist *= rand.Float32()
	}
	angle := destDirectionAngle(target) + cOrientation
	return dx + dist*float32(math.Cos(float64(angle))), dy + dist*float32(math.Sin(float64(angle))), dz, true
}

// spellHasCasterDestTarget reports whether any active effect carries a
// caster-dest implicit target (SelectImplicitCasterDestTargets,
// Spell.cpp:1312).
func spellHasCasterDestTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isCasterDestTarget(eff.ImplicitTargetA) || isCasterDestTarget(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// spellHasDestFamilyTarget reports whether any active effect carries a
// target-dest or dest-dest implicit target.
func spellHasDestFamilyTarget(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isTargetDestTarget(eff.ImplicitTargetA) || isTargetDestTarget(eff.ImplicitTargetB) ||
			isDestDestTarget(eff.ImplicitTargetA) || isDestDestTarget(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// sessionDestSnapshot is one effect's Spell::m_destTargets slot
// (Spell.cpp:630, 789) on the session path: the running destination as it
// stood after that effect's own A/B arms — a dest-family effect later in
// the list does NOT retroactively move an earlier effect's slot, and an
// effect with no dest arm inherits the running destination from earlier
// effects (or the packet destination). Indexed by effect position; Has is
// false for skipped (Effect == 0) effects and for effects before any dest
// was present (no packet dest and no dest arm yet).
type sessionDestSnapshot struct {
	Has     bool
	X, Y, Z float32
}

// sessionEffectDestTarget returns the SpellTargetData an effect's HIT arm
// must see: its own m_destTargets slot (Spell.cpp:5159), not the
// spell-final destination resolveImplicitSpellDestination returns. A stamped
// slot carries the slot; an unstamped slot with a final destination carries
// the caster position (the Spell.cpp:630 slot init — C++'s m_targets.HasDst
// gate passes at handle time, so the arm uses the slot, not the close
// point); with no destination at all the flag is cleared so the helper
// falls to its close-point leg, matching the current no-dest behavior.
func sessionEffectDestTarget(base protocol.SpellTargetData, cx, cy, cz float32, effIndex int, snapshots []sessionDestSnapshot) protocol.SpellTargetData {
	if effIndex >= 0 && effIndex < len(snapshots) && snapshots[effIndex].Has {
		base.Flags |= protocol.SpellTargetFlagDestLocation
		base.Destination = protocol.SpellTargetLocation{X: snapshots[effIndex].X, Y: snapshots[effIndex].Y, Z: snapshots[effIndex].Z}
		return base
	}
	if base.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		base.Destination = protocol.SpellTargetLocation{X: cx, Y: cy, Z: cz}
		return base
	}
	base.Flags &^= protocol.SpellTargetFlagDestLocation
	return base
}

// sessionPersistentAreaAuraTarget returns the SpellTargetData
// spawnPersistentAreaAura must see: the LAST effect-27's own slot
// (SpellEffects.cpp:1738 handles only the last one; Spell.cpp:5159 hands
// each effect its own slot). An unstamped last-27 slot with a final
// destination centers on the caster position (the Spell.cpp:630 slot init);
// with no destination at all the base passes through unchanged so the
// unit-wire fallback inside spawnPersistentAreaAura still runs.
func sessionPersistentAreaAuraTarget(base protocol.SpellTargetData, cx, cy, cz float32, spell wotlk.Spell, snapshots []sessionDestSnapshot) protocol.SpellTargetData {
	idx := -1
	for i, eff := range spell.Effects {
		if eff.Effect == spellEffectPersistentAreaAura {
			idx = i
		}
	}
	if idx < 0 {
		return base
	}
	return sessionEffectDestTarget(base, cx, cy, cz, idx, snapshots)
}

// resolveImplicitSpellDestination mirrors the destination half of
// Spell::SelectSpellTargets (Spell.cpp:758-794) for the target-dest and
// dest-dest families, the caster-dest family (SelectImplicitCasterDestTargets,
// Spell.cpp:1312), TARGET_DEST_CASTER_FISHING (39, Spell.cpp:1359), the 89
// traj destination, the 76/106 channel destinations, and
// TARGET_DEST_NEARBY_ENTRY (46, the DEST half of
// Spell::SelectImplicitNearbyTargets, Spell.cpp:1036): it starts from the
// client-supplied destination (falling back to the caster position, like
// CheckDst in Spell.cpp:6523) and applies the per-effect resolutions in
// effect order — target-dest replaces the destination with the
// unit-target-derived position (Spell.cpp:1433), dest-dest offsets the
// current destination (Spell.cpp:1464), 46 sets it to the nearest
// entry-matched object's position. The result is written back into target
// with the dest-location flag so the area-selection and persistent-area read
// sites consume it through their existing client-dest branches. It runs once
// at cast time; the dynamic-aura tick path reuses those read sites with an
// already resolved destination and must not re-apply the offsets. ok is
// false when a 46 effect finds no object, which fails the cast with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
func (s *session) resolveImplicitSpellDestination(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) (protocol.SpellTargetData, bool) {
	resolved, _, ok := s.resolveImplicitSpellDestinationWithSnapshots(ctx, spell, spellID, target, 0)
	return resolved, ok
}

// sessionInitExplicitSeed mirrors the destination seed of
// Spell::InitExplicitTargets (Spell.cpp:700-712): when the spell's explicit
// mask needs DEST_LOCATION and the packet carries no destination, m_targets
// dst is seeded from the ORIGINAL packet object target — not the adopted
// unit the gauntlet mirrors into the target slot (a selection/self fallback
// seeds the caster, while a mask-removed wire object still seeds its own
// position, since the seed reads the untouched targets parameter) — else
// from the caster. A corpse wire target has no Go model and an
// unresolvable object falls back to the caster, like the other anchor
// fallbacks. seeded is false when the mask needs no destination; callers
// apply the seed only when no packet destination was sent, since a packet
// destination beats the seed (Spell.cpp:702).
func (s *session) sessionInitExplicitSeed(ctx context.Context, spell wotlk.Spell, wireObjectGUID uint64) (float32, float32, float32, bool) {
	if s == nil || s.player == nil || s.server == nil || !spellExplicitDestLocationNeeded(spell) {
		return 0, 0, 0, false
	}
	if wireObjectGUID != 0 {
		if uint16(wireObjectGUID>>48) == 0xF110 {
			if st := s.server.gameObjectState(s.player.Map, s.player.InstanceID, wireObjectGUID); st != nil {
				return st.X, st.Y, st.Z, true
			}
			if gx, gy, gz, ok := s.server.gameObjectAnchorPosition(ctx, wireObjectGUID, s.player.Map, s.player.InstanceID); ok {
				return gx, gy, gz, true
			}
		} else if resolved, ok := s.getCombatTarget(ctx, wireObjectGUID); ok {
			return resolved.X, resolved.Y, resolved.Z, true
		}
	}
	return s.player.X, s.player.Y, s.player.Z, true
}

// resolveImplicitSpellDestinationWithSnapshots is resolveImplicitSpellDestination
// plus the per-effect m_destTargets slots (Spell.cpp:630, 789): snapshots[i]
// is the running destination after effect i's own A/B arms. The single-target
// return is unchanged; callers that hand a destination to a HIT arm use the
// slot, never the spell-final destination.
func (s *session) resolveImplicitSpellDestinationWithSnapshots(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData, wireObjectGUID uint64) (protocol.SpellTargetData, []sessionDestSnapshot, bool) {
	if s == nil || s.player == nil || s.server == nil || (!spellHasDestFamilyTarget(spell) && !spellHasCasterDestTarget(spell) && !spellHasFishingDestTarget(spell) && !spellHasTrajTarget(spell) && !spellHasChannelDestTarget(spell) && !spellHasDestNearbyEntryTarget(spell)) {
		// No dest-family target: every slot stamps the packet destination
		// when one was sent (C++ AddDestTarget fires per effect while
		// HasDst holds). With no packet destination, a spell whose
		// explicit mask needs DEST_LOCATION still stamps the
		// InitExplicit seed (Spell.cpp:700-712, 789) — a summon or DynObj
		// effect before any dest arm centers on the seed, not the
		// caster position the unstamped fallback would give.
		snapshots := make([]sessionDestSnapshot, len(spell.Effects))
		// Spell::InitExplicitTargets (Spell.cpp:716): when the mask needs
		// no DEST_LOCATION the packet destination is removed, so it stamps
		// no slot here either — the slots stay unstamped and the returned
		// target drops the flag, matching m_targets after RemoveDst. The
		// GO packet mask (spellGoPacketTarget) and the HasDst gates of the
		// summon/knockback HIT arms (SpellEffects.cpp:4231/4552) read that
		// flag, so a surviving packet flag would misroute them; a kept
		// packet destination or an InitExplicit seed sets the flag and the
		// returned destination, like SetDst does. RemoveDst clears only the
		// mask, so the position field is left stale when the flag drops.
		dstKept := target.Flags&protocol.SpellTargetFlagDestLocation != 0 && spellExplicitDestLocationNeeded(spell)
		seedX, seedY, seedZ, seeded := float32(0), float32(0), float32(0), false
		if !dstKept {
			seedX, seedY, seedZ, seeded = s.sessionInitExplicitSeed(ctx, spell, wireObjectGUID)
		}
		if dstKept {
			for i := range snapshots {
				snapshots[i] = sessionDestSnapshot{Has: true, X: target.Destination.X, Y: target.Destination.Y, Z: target.Destination.Z}
			}
		} else if seeded {
			for i := range snapshots {
				snapshots[i] = sessionDestSnapshot{Has: true, X: seedX, Y: seedY, Z: seedZ}
			}
			target.Flags |= protocol.SpellTargetFlagDestLocation
			target.Destination = protocol.SpellTargetLocation{X: seedX, Y: seedY, Z: seedZ}
		} else {
			target.Flags &^= protocol.SpellTargetFlagDestLocation
		}
		return finalizeSessionSrcTarget(spell, target, target.Flags&protocol.SpellTargetFlagSourceLocation != 0, s.player.X, s.player.Y, s.player.Z), snapshots, true
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	packetHasDst := target.Flags&protocol.SpellTargetFlagDestLocation != 0
	packetHasSrc := target.Flags&protocol.SpellTargetFlagSourceLocation != 0
	// Spell::InitExplicitTargets (Spell.cpp:716): when the mask needs no
	// DEST_LOCATION the packet destination is removed, so the running
	// destination starts at the caster and pre-arm slots stay unstamped —
	// a dest-dest/traj arm's CheckDst (Spell.cpp:6523) falls back to the
	// caster, not the dropped packet point. Dest arms below re-set HasDst
	// via SetDst, so post-arm slots are unaffected by the gate.
	packetHasDstKept := packetHasDst && spellExplicitDestLocationNeeded(spell)
	seedX, seedY, seedZ, seedApplied := float32(0), float32(0), float32(0), false
	if !packetHasDst {
		seedX, seedY, seedZ, seedApplied = s.sessionInitExplicitSeed(ctx, spell, wireObjectGUID)
	}
	if packetHasDstKept {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if seedApplied {
		// Spell::InitExplicitTargets (Spell.cpp:700-712): the running
		// destination starts at the seed, so a dest-dest arm that leaves
		// the destination unchanged (28/29/87/88) and every effect
		// before the first dest arm see the seed, not the caster.
		x, y, z = seedX, seedY, seedZ
	}
	var unitTarget combatTarget
	hasUnitTarget := false
	// Spell::InitExplicitTargets (Spell.cpp:668-678): a wire object target
	// whose type the spell's explicit mask does not need is removed before
	// the implicit arms run, so the 53/63/64-71/74/75 arm never anchors on
	// it — the fits-mask gate rides here as well as in the cast-completion
	// explicit block.
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 && explicitWireTargetFitsMask(spell, target) {
		// A GO guid must never reach getCombatTarget: its DB leg matches
		// the low 24 bits against the creature table and would anchor the
		// destination on an unrelated creature — the same collision the
		// charge/teleport/leap-back arms already guard.
		if uint16(target.UnitGUID>>48) != 0xF110 {
			if resolved, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
				unitTarget, hasUnitTarget = resolved, true
			}
		}
	}
	// Spell::SelectImplicitTargetDestTargets (Spell.cpp:1433) anchors on
	// GetObjectTarget — any WorldObject, not just units — so a GO wire
	// target anchors the destination at the gameobject's position when the
	// spell's explicit mask takes gameobjects (C++ InitExplicitTargets,
	// Spell.cpp:668-678, drops a GO target otherwise). The position comes
	// from the GO anchor helper, never the creature DB leg; the live state
	// also carries the orientation the offset dest types need, while a
	// static row falls back to its DB orientation.
	if !hasUnitTarget && target.UnitGUID != 0 && uint16(target.UnitGUID>>48) == 0xF110 && spellExplicitObjectTargetMask(spell)&(targetFlagGameObject|targetFlagGameObjectItem) != 0 {
		if st := s.server.gameObjectState(s.player.Map, s.player.InstanceID, target.UnitGUID); st != nil {
			unitTarget, hasUnitTarget = combatTarget{GUID: target.UnitGUID, Map: s.player.Map, InstanceID: s.player.InstanceID, X: st.X, Y: st.Y, Z: st.Z, Orientation: st.Orientation}, true
		} else if gx, gy, gz, ok := s.server.gameObjectAnchorPosition(ctx, target.UnitGUID, s.player.Map, s.player.InstanceID); ok {
			var ori float64
			if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT orientation FROM gameobject WHERE guid = ?", uint32(target.UnitGUID&0xFFFFFF)).Scan(&ori)
			}
			unitTarget, hasUnitTarget = combatTarget{GUID: target.UnitGUID, Map: s.player.Map, InstanceID: s.player.InstanceID, X: gx, Y: gy, Z: gz, Orientation: float32(ori)}, true
		}
	}
	snapshots := make([]sessionDestSnapshot, len(spell.Effects))
	hasDst := packetHasDstKept || seedApplied
	// seenSrcCaster tracks whether a TARGET_SRC_CASTER (22) arm has
	// already run at an earlier (effect, slot) in SelectSpellTargets
	// order (Spell.cpp:758-779: effects in index order, TargetA
	// before TargetB): the 22 arm re-sets m_src to the live caster
	// (Spell.cpp:930-936), so a traj arm after it reads the live
	// caster, while a traj arm before it reads the pre-22 src (packet
	// src else the Read/prepare-time seed, Spell.cpp:1635).
	seenSrcCaster := false
	for effIndex, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for slotIndex, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 22 {
				seenSrcCaster = true
			}
			switch {
			case isCasterDestTarget(targetType):
				// Spell::SelectImplicitCasterDestTargets (Spell.cpp:1312-1424):
				// caster-dest implicit targets (18/32/41-44/47-50/55/72/73)
				// set the spell destination from the caster-relative position
				// on every cast — C++ m_targets.SetDst at the end of the
				// function, not just for totem summons (the totem path in
				// totems.go predates this general resolution). The
				// destination starts from the caster's position
				// (dest(*m_caster), Spell.cpp:1314), ignoring any
				// client-supplied dest; like the other families, later
				// effects overwrite earlier ones. The script
				// DestinationTargetSelect handlers are the documented
				// no-bridge leg (no ScriptMgr model).
				combatReach := s.player.CombatReach
				if combatReach <= 0 {
					combatReach = 1.5
				}
				// SpellInfo.cpp:551-574: CalcRadius(m_caster) folds SPELLMOD_RADIUS
				// through the player's spell-mod owner; resolveTargetDestPosition
				// stays unfolded (C++ passes nullptr).
				modRadius := func(r float32) float32 {
					return float32(s.applySpellModFloat(spell, spellModRadius, float64(r)))
				}
				x, y, z = resolveCasterDestPosition(s.server.Data, targetType, eff.RadiusIndex, uint32(s.player.Level), s.player.X, s.player.Y, s.player.Z, s.player.Orientation, combatReach, modRadius)
				hasDst = true
			case targetType == implicitTargetDestCasterFishing:
				// Spell::SelectImplicitCasterDestTargets (Spell.cpp:1359-1389):
				// TARGET_DEST_CASTER_FISHING (39) — the bobber point resolves
				// server-side, overwriting any client-supplied destination
				// (the pre-strip client-dest gate). The liquid-level
				// NOT_HERE/TOO_SHALLOW fail arms are no-bridge (no
				// map-height/liquid model).
				x, y, z = s.fishingDestPoint(spell)
				hasDst = true
			case isTargetDestTarget(targetType):
				// C++ asserts a non-null object target here; without a
				// resolvable unit target Go keeps the current destination.
				if !hasUnitTarget {
					continue
				}
				x, y, z = resolveTargetDestPosition(s.server.Data, targetType, eff.RadiusIndex, unitTarget.X, unitTarget.Y, unitTarget.Z, unitTarget.Orientation)
				hasDst = true
			case isDestDestTarget(targetType):
				// Spell::SelectImplicitDestDestTargets (Spell.cpp:1464):
				// CheckDst runs before the switch, so the arm sets HasDst
				// even for the unchanged 28/29/87/88 legs (ModDst runs only
				// for the offset legs) — this effect's slot and every later
				// effect's slot stamp the CheckDst caster position when
				// nothing else set one.
				hasDst = true
				// SpellInfo.cpp:551-574: CalcRadius(m_caster) folds SPELLMOD_RADIUS
				// through the player's spell-mod owner.
				modRadius := func(r float32) float32 {
					return float32(s.applySpellModFloat(spell, spellModRadius, float64(r)))
				}
				nx, ny, nz, changed := resolveDestDestPosition(s.server.Data, targetType, eff.RadiusIndex, uint32(s.player.Level), x, y, z, s.player.Orientation, modRadius)
				if changed {
					x, y, z = nx, ny, nz
				}
			case targetType == implicitTargetDestTraj:
				// Spell::SelectImplicitTrajTargets (Spell.cpp:1626):
				// shorten the destination to the first missile-body
				// collision along the ballistic trajectory. CheckDst is
				// the caster-position fallback already applied above.
				// When a 22 arm already ran, m_src is the live caster
				// at selection time, so the stamped packet/Read src
				// must not feed the arm: pass a copy with both src
				// legs cleared and let the arm fall through to the
				// live caster position.
				trajTarget := target
				if seenSrcCaster {
					trajTarget.Flags &^= protocol.SpellTargetFlagSourceLocation
					trajTarget.HasReadSrc = false
				}
				if nx, ny, nz, changed := s.resolveTrajDestination(ctx, spell, eff, trajTarget, x, y, z); changed {
					x, y, z = nx, ny, nz
					hasDst = true
				}
			case targetType == implicitTargetDestChannelTarget || targetType == implicitTargetDestChannelCaster:
				// Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032):
				// resolve the destination from the live channel. The
				// client-cast path breaks any prior channel at cast start
				// (mirroring C++ SetCurrentCastSpell in Spell::prepare, which
				// runs before SelectSpellTargets in _cast), so the C++ null
				// gate means nothing resolves here in practice.
				if nx, ny, nz, ok := s.channelDestForSpell(ctx, spell); ok {
					x, y, z = nx, ny, nz
					hasDst = true
				}
			case targetType == implicitTargetDestNearbyEntry:
				// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
				// TARGET_DEST_NEARBY_ENTRY (46) — the destination becomes the
				// nearest entry-matched object's position; no match fails the
				// cast (Spell.cpp:1111), and without conditions for this
				// effect a RequiresSpellFocus spell uses the focus
				// object's position. The scan is per effect
				// (Spell.cpp:1066), so a later effect's 46 arm re-scans
				// with its own conditions and overwrites the destination.
				// A B-slot 46 behind a NEARBY/CONE/AREA A slot never selects
				// (Spell.cpp:866-900 processed-slot rule) and must not fail
				// the cast here.
				if slotIndex == 1 && implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB) {
					continue
				}
				if nx, ny, nz, ok := s.spellEntryNearbyDestPosition(ctx, spell, spellID, effIndex); ok {
					x, y, z = nx, ny, nz
					hasDst = true
				} else {
					return target, nil, false
				}
			}
		}
		if hasDst {
			snapshots[effIndex] = sessionDestSnapshot{Has: true, X: x, Y: y, Z: z}
		}
	}
	// The returned flag is m_targets.HasDst after selection, not "a
	// dest-family spell ran": with no kept packet destination, no seed,
	// and no arm setting it (target-dest with no resolvable object,
	// traj with no shortening, channel with no live channel), C++
	// leaves the mask clear — the GO packet carries no destination, the
	// channeled BAD_IMPLICIT_TARGETS check (Spell.cpp:794) sees !HasDst,
	// and the summon/knockback HIT arms (SpellEffects.cpp:4231/4552)
	// take their !HasDst legs (close point / no knockback) instead of
	// the slot's caster position. RemoveDst clears only the mask, so
	// the position field is left stale when the flag drops; every
	// consumer gates on the flag.
	if hasDst {
		target.Flags |= protocol.SpellTargetFlagDestLocation
		target.Destination = protocol.SpellTargetLocation{X: x, Y: y, Z: z}
	} else {
		target.Flags &^= protocol.SpellTargetFlagDestLocation
	}
	return finalizeSessionSrcTarget(spell, target, packetHasSrc, s.player.X, s.player.Y, s.player.Z), snapshots, true
}

// finalizeSessionSrcTarget mirrors the SOURCE_LOCATION half of
// Spell::InitExplicitTargets (Spell.cpp:720-727) plus the implicit
// TARGET_SRC_CASTER arm (Spell.cpp:930-936) for a resolver-returned or
// GO-packet target: the final HasSrc is the mask need OR the 22 arm. A
// packet source survives only when the mask needs it — RemoveSrc clears
// the flag otherwise, and the position field is left stale when the flag
// drops, as with the destination. A needed-but-absent source is set to
// the caster (InitExplicit SetSrc), and the 22 arm always re-sets the
// source to the caster, overwriting a kept packet source. The traj arms
// do NOT take this gate: they read the source position unconditionally
// (Spell.cpp:1635 GetSrcPos, no HasSrc check) and CheckSrc is dead code
// (zero call sites), so a dropped packet source still seeds the
// trajectory — unlike the destination, where CheckDst (Spell.cpp:6523)
// resets the running destination to the caster before the dest-dest and
// traj arms run.
func finalizeSessionSrcTarget(spell wotlk.Spell, target protocol.SpellTargetData, packetHasSrc bool, casterX, casterY, casterZ float32) protocol.SpellTargetData {
	if spellHasImplicitSrcCasterTarget(spell) {
		target.Flags |= protocol.SpellTargetFlagSourceLocation
		target.Source = protocol.SpellTargetLocation{X: casterX, Y: casterY, Z: casterZ}
		return target
	}
	if spellExplicitSourceLocationNeeded(spell) {
		target.Flags |= protocol.SpellTargetFlagSourceLocation
		if !packetHasSrc {
			target.Source = protocol.SpellTargetLocation{X: casterX, Y: casterY, Z: casterZ}
		}
		return target
	}
	target.Flags &^= protocol.SpellTargetFlagSourceLocation
	return target
}

// startSrcTarget mirrors ONLY the SOURCE_LOCATION half of
// Spell::InitExplicitTargets (Spell.cpp:720-727) for an SMSG_SPELL_START
// build: SendSpellStart runs inside Spell::prepare (Spell.cpp:3194),
// after InitExplicitTargets (Spell.cpp:3039) but before SelectSpellTargets,
// so the implicit TARGET_SRC_CASTER arm (Spell.cpp:930-936) has NOT run
// yet and must not be applied here — unlike finalizeSessionSrcTarget,
// which serves the post-selection GO packet. A packet source survives
// only when the explicit mask needs SOURCE_LOCATION (RemoveSrc clears
// the flag otherwise, leaving the position field stale); a needed-but-
// absent source is set to the caster (InitExplicit SetSrc). The returned
// copy leaves the caller's target untouched, so the finish/queue/traj
// flows keep the raw packet target.
func startSrcTarget(spell wotlk.Spell, target protocol.SpellTargetData, packetHasSrc bool, casterX, casterY, casterZ float32) protocol.SpellTargetData {
	if spellExplicitSourceLocationNeeded(spell) {
		target.Flags |= protocol.SpellTargetFlagSourceLocation
		if !packetHasSrc {
			target.Source = protocol.SpellTargetLocation{X: casterX, Y: casterY, Z: casterZ}
		}
		return target
	}
	target.Flags &^= protocol.SpellTargetFlagSourceLocation
	return target
}

// startDestTarget mirrors ONLY the DEST_LOCATION half of
// Spell::InitExplicitTargets (Spell.cpp:700-716) for an SMSG_SPELL_START
// build: SendSpellStart runs inside Spell::prepare (Spell.cpp:3194),
// after InitExplicitTargets (Spell.cpp:3039) but before SelectSpellTargets,
// so no implicit dest arm has run yet and the dest-dest/traj arms must not
// be applied here — unlike the resolver, which folds them into the running
// destination. A packet destination survives only when the explicit mask
// needs DEST_LOCATION (RemoveDst clears the flag otherwise, leaving the
// position field stale); a needed-but-absent destination is seeded from
// the ORIGINAL packet object target (wireObjectGUID, captured at packet
// parse before the gauntlet rewrites target) else the caster (InitExplicit
// SetDst). The packet flag itself is the parse-time flag: the gauntlet may
// rewrite target.Flags wholesale before START, so callers pass the captured
// packetHasDst, never the live flag. The returned copy leaves the caller's
// target untouched, so the finish/queue/traj flows keep the raw packet
// target.
func (s *session) startDestTarget(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData, packetHasDst bool, wireObjectGUID uint64) protocol.SpellTargetData {
	if !spellExplicitDestLocationNeeded(spell) {
		target.Flags &^= protocol.SpellTargetFlagDestLocation
		return target
	}
	target.Flags |= protocol.SpellTargetFlagDestLocation
	if !packetHasDst {
		if x, y, z, ok := s.sessionInitExplicitSeed(ctx, spell, wireObjectGUID); ok {
			target.Destination = protocol.SpellTargetLocation{X: x, Y: y, Z: z}
		}
	}
	return target
}
