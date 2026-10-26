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
		if isEntryAreaTargetType(eff.ImplicitTargetA) || (isEntryAreaTargetType(eff.ImplicitTargetB) && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB)) {
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
// combat-disabled — plus each selecting effect's own ImplicitTargetConditions
// on its own leg: the selecting slot is A when A is 7/8, else B when B is
// 7/8 and A did not consume the effect's processed slot (Spell.cpp:866-900,
// the GO 51/52 rule), and each leg centers on its own reference (SRC =
// caster, DEST = resolved destination) with its own radius, so a candidate
// is kept when ANY leg's cylinder + LOS + conditions match it (never a
// single center with a merged condition set, whose same-ElseGroup rows
// would AND across effects). Every match is kept (no
// nearest-shrink, unlike 38); the MaxTargets cap mirrors the area path
// (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier +
// RandomResize). An empty area list adds no targets and never fails the
// cast — area selection has no BAD_IMPLICIT_TARGETS gate (unlike 38).
func (s *session) spellEntryAreaTargets(ctx context.Context, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isEntryAreaSpell(spell) || s.server.Data == nil {
		return nil
	}
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
	// Per-effect legs (Spell.cpp:1227: SelectImplicitAreaTargets runs per
	// effect with that effect's own ImplicitTargetConditions, and each
	// selection adds its matches): the selecting slot is A when A is
	// 7/8, else B when B is 7/8 and A did not consume the effect's
	// processed slot (Spell.cpp:866-900, the GO 51/52 rule — an A slot
	// in NEARBY/CONE/AREA marks the effect processed, so a B 7/8 leg
	// never runs for it). Each leg carries its own center (SRC =
	// caster, DEST = resolved destination), its own CalcRadius and its
	// own condition rows; the spell's list is the union of the legs,
	// never a single center with a merged condition set.
	type entryAreaLeg struct {
		cx, cy, cz     float32
		radius         float64
		condRows       []conditionRow
		allowPlayers   bool
		allowCreatures bool
	}
	legs := make([]entryAreaLeg, 0)
	maxRadius := float64(0)
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		var legTarget uint32
		switch {
		case isEntryAreaTargetType(eff.ImplicitTargetA):
			legTarget = eff.ImplicitTargetA
		case isEntryAreaTargetType(eff.ImplicitTargetB) && !implicitTargetBlocksSecondSlot(eff.ImplicitTargetA):
			legTarget = eff.ImplicitTargetB
		default:
			continue
		}
		r := float64(0)
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok {
			// SpellEffectInfo::CalcRadius (SpellInfo.cpp:551-574): the DBC
			// radius folds SPELLMOD_RADIUS via the mod owner.
			r = s.applySpellModFloat(spell, spellModRadius, float64(value))
		}
		if r <= 0 {
			if rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex); err == nil && ok {
				r = float64(rangeEntry.MaxHostile)
				if !isHarmfulSpell(spell) {
					r = float64(rangeEntry.MaxFriendly)
				}
			}
		}
		if r <= 0 {
			continue
		}
		leg := entryAreaLeg{cx: s.player.X, cy: s.player.Y, cz: s.player.Z, radius: r, condRows: s.loadImplicitTargetConditions(ctx, spellID, 1<<uint(i))}
		if legTarget == implicitTargetDestAreaEntry {
			leg.cx, leg.cy, leg.cz = destX, destY, destZ
		}
		leg.allowPlayers, leg.allowCreatures = implicitTargetEntryMask(leg.condRows)
		// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842): the UNIT base
		// mask narrows to PLAYER|CORPSE under
		// SPELL_ATTR3_ONLY_TARGET_PLAYERS / SPELL_ATTR3_ONLY_TARGET_GHOSTS,
		// so creature candidates drop out (the 38/40 precedent).
		if spellSearchPlayersOnly(spell) {
			leg.allowCreatures = false
		}
		legs = append(legs, leg)
		if r > maxRadius {
			maxRadius = r
		}
	}
	if len(legs) == 0 {
		return nil
	}
	allowDead := spellAllowsDeadTarget(spell)
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	// The scan enumerates motions and sessions map-wide, but its DB leg
	// is boxed around the scan center, so each distinct leg center gets
	// its own pass; a candidate matched on an earlier pass rides the
	// seen set, and an unmatched candidate is re-evaluated per pass.
	scanCenters := [][3]float32{{s.player.X, s.player.Y, s.player.Z}}
	hasDestCenter := false
	for _, leg := range legs {
		if leg.cx != s.player.X || leg.cy != s.player.Y || leg.cz != s.player.Z {
			hasDestCenter = true
		}
	}
	if hasDestCenter {
		scanCenters = append(scanCenters, [3]float32{destX, destY, destZ})
	}
	for _, center := range scanCenters {
		s.friendlyScanCandidates(ctx, center[0], center[1], float32(maxRadius), spell, func(c friendlyCandidate) {
			if c.mapID != s.player.Map || c.instanceID != s.player.InstanceID {
				return
			}
			if c.health == 0 && !allowDead {
				return
			}
			if spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, false) {
				return
			}
			spawnLow := uint32(0)
			if !c.isPlayer {
				spawnLow = uint32(c.guid & 0xFFFFFF)
			}
			matched := false
			for _, leg := range legs {
				if c.isPlayer && !leg.allowPlayers {
					continue
				}
				if !c.isPlayer && !leg.allowCreatures {
					continue
				}
				dx, dy := float64(c.x-leg.cx), float64(c.y-leg.cy)
				if dx*dx+dy*dy > leg.radius*leg.radius || math.Abs(float64(c.z-leg.cz)) > leg.radius {
					continue
				}
				// Spell::CheckEffectTarget default arm (Spell.cpp:7479-7481):
				// area selection passes losPosition == center, so every
				// candidate needs line of sight from the leg's center.
				if !s.implicitAreaTargetLOSPasses(spell, c.x, c.y, c.z, leg.cx, leg.cy, leg.cz) {
					continue
				}
				if implicitEntryConditionsMet(leg.condRows, c.isPlayer, c.entry, spawnLow) {
					matched = true
					break
				}
			}
			if !matched {
				return
			}
			if _, ok := seen[c.guid]; ok {
				return
			}
			seen[c.guid] = struct{}{}
			targets = append(targets, c.guid)
		})
	}
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
