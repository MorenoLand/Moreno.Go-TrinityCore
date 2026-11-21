package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const implicitTargetCorpseSrcAreaEnemy uint32 = 93 // TARGET_CORPSE_SRC_AREA_ENEMY (SpellInfo.cpp:313)

// isCorpseAreaSpell reports spells carrying TARGET_CORPSE_SRC_AREA_ENEMY
// (93) on any live effect: {CORPSE, SRC, AREA, TARGET_CHECK_ENEMY}
// (SpellInfo.cpp:218-330; Spell::SelectImplicitAreaTargets, Spell.cpp:1227).
// The B slot runs only when the A slot did not consume the effect's
// processed slot (Spell.cpp:866-900, the implicitTargetBlocksSecondSlot
// rule — 93 is an AREA target, so an AREA A slot blocks a B 93 leg).
func isCorpseAreaSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetCorpseSrcAreaEnemy ||
			(eff.ImplicitTargetB == implicitTargetCorpseSrcAreaEnemy && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB)) {
			return true
		}
	}
	return false
}

// spellCorpseAreaTargets ports Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) + Spell::SearchAreaTargets (Spell.cpp:1881) for
// TARGET_CORPSE_SRC_AREA_ENEMY (93).
//
// Center: SRC reference, so the center is the src position — Go tracks no
// src position, so SRC defaults to the caster, the same heuristic Go
// applies to the SRC-reference entry/GO area legs; C++'s CheckSrc
// (Spell.cpp:6519) also defaults an unset src to the caster.
// Radius: CalcRadius(caster), with the Spell.cpp:1281 workaround — an
// unset radius falls back to GetMaxRange(IsPositiveEffect) (Go: the
// SpellRange MaxHostile/MaxFriendly split via isHarmfulSpell, like the
// entry-area path); the C++ RadiusMod multiplier has no Go equivalent
// beyond the cast-scoped applyCastRadiusMod64 fold.
// Selection: WorldObjectSpellAreaTargetCheck (Spell.cpp:8409) cylinder
// test (2D distance within radius and |dz| within radius), then
// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316) — whose corpse
// arm (SpellInfo.cpp:1697-1710) rejects bare bones and ownerless corpses,
// and whose ENEMY faction check is SKIPPED for corpses (Spell.cpp:8341:
// the IsValidAttackTarget arm is gated on !target->IsCorpse(), with a
// TODO to restore it) — so any corpse whose owner is online passes.
// CheckTarget's dead-owner gate (SpellInfo.cpp:1715) still applies: the
// owner is dead by definition, so a spell that does not allow dead
// targets selects nothing. LOS rides the CheckEffectTarget default arm
// (Spell.cpp:7479-7481, losPosition == center), like the entry/friendly
// area paths. Per-effect ImplicitTargetConditions on a 93 leg have no Go
// evaluator (no corpse-condition twin) and are not applied — no 93 spell
// in the seed carries any. The MaxTargets cap mirrors the area path
// (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier +
// RandomResize). An empty area list adds no targets and never fails the
// cast — area selection has no BAD_IMPLICIT_TARGETS gate (unlike 38).
// Each match carries only its selecting effect's AddCorpseTarget bit
// (Spell.cpp:2281); Go's flat hitTargets list is the standing
// mixed-effect approximation, so a sibling effect's fan-out also sees
// these corpses — the same class as the area+single mixes elsewhere.
// AddCorpseTarget TimeDelay (Spell.cpp:2300-2310, Speed > 0) is not
// folded into spellDelayMomentMs — vacuous for Speed-0 resurrects,
// the L1 explicit-corpse residual.
func (s *session) spellCorpseAreaTargets(ctx context.Context, spell wotlk.Spell) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isCorpseAreaSpell(spell) || s.server.Data == nil {
		return nil
	}
	if !spellAllowsDeadTarget(spell) {
		return nil
	}
	type corpseAreaLeg struct {
		radius float64
	}
	legs := make([]corpseAreaLeg, 0)
	maxRadius := float64(0)
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		switch {
		case eff.ImplicitTargetA == implicitTargetCorpseSrcAreaEnemy:
		case eff.ImplicitTargetB == implicitTargetCorpseSrcAreaEnemy && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB):
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
		// SPELLVALUE_RADIUS_MOD (Spell.cpp:1194/1284): the cast-scoped
		// radius multiplier folds in after the CalcRadius/range-fallback arm.
		r = applyCastRadiusMod64(spell, r)
		if r <= 0 {
			continue
		}
		legs = append(legs, corpseAreaLeg{radius: r})
		if r > maxRadius {
			maxRadius = r
		}
	}
	if len(legs) == 0 {
		return nil
	}
	corpses := s.server.listCorpsesOnMap(ctx, s.player.Map, s.player.InstanceID)
	targets := make([]uint64, 0, len(corpses))
	seen := make(map[uint64]struct{})
	for _, corpse := range corpses {
		// WorldObjectSpellTargetCheck (Spell.cpp:8322-8328): a corpse
		// whose owner is offline is not a valid target at all.
		ownerSess := s.server.findSessionByGUID(corpse.OwnerGUID)
		if ownerSess == nil || ownerSess.player == nil {
			continue
		}
		matched := false
		for _, leg := range legs {
			// WorldObjectSpellAreaTargetCheck (Spell.cpp:8420): the
			// cylinder test — 2D distance within radius and |dz|
			// within radius — centered on the caster (SRC).
			dx, dy := float64(corpse.X-s.player.X), float64(corpse.Y-s.player.Y)
			if dx*dx+dy*dy > leg.radius*leg.radius || math.Abs(float64(corpse.Z-s.player.Z)) > leg.radius {
				continue
			}
			// Spell::CheckEffectTarget default arm (Spell.cpp:7479-7481):
			// area selection passes losPosition == center, so every
			// candidate needs line of sight from the leg's center.
			if !s.implicitAreaTargetLOSPasses(spell, corpse.X, corpse.Y, corpse.Z, s.player.X, s.player.Y, s.player.Z) {
				continue
			}
			matched = true
			break
		}
		if !matched {
			continue
		}
		if _, dup := seen[corpse.GUID]; dup {
			continue
		}
		seen[corpse.GUID] = struct{}{}
		targets = append(targets, corpse.GUID)
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
