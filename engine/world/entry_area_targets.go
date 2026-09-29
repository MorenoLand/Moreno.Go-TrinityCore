package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetSrcAreaEntry  uint32 = 7 // TARGET_UNIT_SRC_AREA_ENTRY (SharedDefines.h:1447)
	implicitTargetDestAreaEntry uint32 = 8 // TARGET_UNIT_DEST_AREA_ENTRY
)

// isEntryAreaTargetType reports the SpellImplicitTargetInfo values
// (SpellInfo.cpp:218-330) that Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) resolves with TARGET_CHECK_ENTRY.
func isEntryAreaTargetType(target uint32) bool {
	return target == implicitTargetSrcAreaEntry || target == implicitTargetDestAreaEntry
}

func isEntryAreaSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isEntryAreaTargetType(eff.ImplicitTargetA) || isEntryAreaTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// spellEntryAreaTargets ports Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) + Spell::SearchAreaTargets (Spell.cpp:1881) for
// TARGET_UNIT_SRC_AREA_ENTRY (7) / TARGET_UNIT_DEST_AREA_ENTRY (8):
// {UNIT, SRC|DEST, AREA, TARGET_CHECK_ENTRY} (SpellInfo.cpp:218-330).
//
// Center: GetSrcPos() for 7, GetDstPos() for 8. Go tracks no src position,
// so SRC defaults to the caster — the same heuristic Go applies to
// TARGET_SRC_CASTER (22) and the SRC-reference friendly area values
// (30/33); C++'s CheckSrc (Spell.cpp:6519) also defaults an unset src to
// the caster. DEST uses the resolved destination (client dest or the
// explicit unit via CheckDst, like the friendly-area path).
// Radius: CalcRadius(caster), with the Spell.cpp:1281 workaround — an
// unset radius falls back to GetMaxRange(IsPositiveEffect) (Go: the
// SpellRange MaxHostile/MaxFriendly split via isHarmfulSpell, like the 38
// path); the C++ RadiusMod multiplier has no Go equivalent.
// Selection: WorldObjectSpellAreaTargetCheck (Spell.cpp:8409) cylinder
// test (2D distance within radius and |dz| within radius), then
// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316). TARGET_CHECK_ENTRY
// has no faction case in the switch, so only the CheckTarget terms apply:
// alive (unless the spell allows dead targets, SpellInfo.cpp:1715) and not
// combat-disabled — plus the spell's ImplicitTargetConditions evaluated
// with the shared entry-condition loader. Every match is kept (no
// nearest-shrink, unlike 38); the MaxTargets cap mirrors the area path
// (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier +
// RandomResize). An empty area list adds no targets and never fails the
// cast — area selection has no BAD_IMPLICIT_TARGETS gate (unlike 38).
func (s *session) spellEntryAreaTargets(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isEntryAreaSpell(spell) || s.server.Data == nil {
		return nil
	}
	var effectMask uint32
	radius := float64(0)
	destCentered := false
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		matched := false
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if !isEntryAreaTargetType(targetType) {
				continue
			}
			matched = true
			if targetType == implicitTargetDestAreaEntry {
				destCentered = true
			}
		}
		if !matched {
			continue
		}
		effectMask |= 1 << uint(i)
		r := float64(0)
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok {
			r = float64(value)
		}
		if r <= 0 {
			if rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex); err == nil && ok {
				r = float64(rangeEntry.MaxHostile)
				if !isHarmfulSpell(spell) {
					r = float64(rangeEntry.MaxFriendly)
				}
			}
		}
		if r > radius {
			radius = r
		}
	}
	if radius <= 0 {
		return nil
	}
	centerX, centerY, centerZ := s.player.X, s.player.Y, s.player.Z
	if destCentered {
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
	allowPlayers, allowCreatures := implicitTargetEntryMask(condRows)
	allowDead := spellAllowsDeadTarget(spell)
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	s.friendlyScanCandidates(ctx, centerX, centerY, float32(radius), func(c friendlyCandidate) {
		if c.mapID != s.player.Map || c.instanceID != s.player.InstanceID {
			return
		}
		if c.isPlayer && !allowPlayers {
			return
		}
		if !c.isPlayer && !allowCreatures {
			return
		}
		if c.health == 0 && !allowDead {
			return
		}
		if spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, false) {
			return
		}
		dx, dy := float64(c.x-centerX), float64(c.y-centerY)
		if dx*dx+dy*dy > radius*radius || math.Abs(float64(c.z-centerZ)) > radius {
			return
		}
		spawnLow := uint32(0)
		if !c.isPlayer {
			spawnLow = uint32(c.guid & 0xFFFFFF)
		}
		if !implicitEntryConditionsMet(condRows, c.isPlayer, c.entry, spawnLow) {
			return
		}
		if _, ok := seen[c.guid]; ok {
			return
		}
		seen[c.guid] = struct{}{}
		targets = append(targets, c.guid)
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
