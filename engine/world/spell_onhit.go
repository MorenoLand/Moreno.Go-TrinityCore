package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// spellScriptOnHitDamage is the pre-damage SpellScript OnHit dispatch:
// C++ runs the OnHit handlers in Spell::PreprocessTarget, before the
// effect loop applies the hit damage (Spell.cpp:2377), so a handler's
// SetHitDamage rewrites the DBC base damage while the taken/crit/
// resist/absorb legs below still run on the rewritten value. Go has no
// SpellScript registry, so the dispatch is a per-spell table consulted
// by the creature-caster damage arms (creatureEffectSpellDamage and
// creatureEffectSpellDamageCreature) right after damage := baseDamage,
// the position of C++'s SetHitDamage. The player-caster leg rides the
// same dispatch from the session SCHOOL_DAMAGE arm
// (castSpellDirectWithOverrides in spells.go). Only absolute-value
// arms live here: at OnHit time GetHitDamage() is always 0
// (TargetInfo::Damage defaults 0, Spell.h:681; PreprocessSpellHit
// never writes m_damage, Spell.cpp:2710-2839), so any arm that
// multiplies GetHitDamage() is a C++ no-op and is NOT in the table:
// loken 52961 (GetHitDamage() * distance), krystallus/gruul shatter
// 50811/33671 (GetHitDamage() * falloff) — both removed 2026-11-09,
// their tick-372 "ported scaling" notes were wrong.
func (s *Server) spellScriptOnHitDamage(spell wotlk.Spell, casterLevel uint32, casterX, casterY, victimX, victimY float32, victimHealth, victimMaxHealth, baseDamage uint32) (uint32, bool) {
	return onHitDamageRewrite(spell.ID, victimHealth, victimMaxHealth, baseDamage)
}

// spellScriptOnHitLatch runs the SpellScript OnHit arms that set a
// creature ScriptData latch on a lethal hit. Go's fireSpellHit twin
// now fires pre-damage (boss_ai.go pass-1 position), but the C++
// latch condition (GetHitDamage() >= target health at OnHit) is a
// no-op there — GetHitDamage() is 0 at OnHit — so the latch still
// cannot be set from the twin; callers invoke this from the lethal
// branch of the creature-caster damage arm, once the final damage is
// known and before the kill-credit legs run (grantCreatureKillReward
// reads the latch at criteria time). DELIBERATE DIVERGENCE from
// C++-as-written: spell_razorscale_flame_breath::CheckDamage
// (boss_razorscale.cpp:1589-1600) is an OnHit arm testing
// GetHitDamage() >= target health, but GetHitDamage() is 0 at OnHit
// (Spell.cpp:2377; see spellScriptOnHitDamage above), so C++ NEVER
// sets the latch and the achievement is unobtainable there. Go sets
// it on a lethal hit anyway (the designed behaviour) so the
// achievement can be earned; do not "fix" this back to the C++
// no-op without a user call.
func (s *Server) spellScriptOnHitLatch(spell wotlk.Spell, victim *creatureMotion) {
	if s == nil || victim == nil {
		return
	}
	switch spell.ID {
	case 63317, 64021:
		// spell_razorscale_flame_breath::CheckDamage
		// (boss_razorscale.cpp:1589-1600): a flame-breath hit that
		// kills a Dark Rune Guardian sets the guardian's
		// DATA_IRON_DWARF_MEDIUM_RARE latch (29232924). The caller
		// only invokes this on a lethal hit, matching the C++
		// condition's intent (GetHitDamage() >= target health);
		// the guardian-entry gate is the C++
		// GetEntry() != NPC_DARK_RUNE_GUARDIAN early return.
		if victim.Entry != 33388 {
			return
		}
		s.motionMu.Lock()
		if victim.ScriptData == nil {
			victim.ScriptData = map[uint32]uint32{}
		}
		victim.ScriptData[29232924] = 1
		s.motionMu.Unlock()
	}
}

// onHitDamageRewrite is the pure per-spell rewrite table behind
// spellScriptOnHitDamage: (rewritten damage, true) when the spell has
// an OnHit SetHitDamage arm, (baseDamage, false) otherwise. Only
// absolute-value arms qualify (see spellScriptOnHitDamage above for
// why GetHitDamage()-relative arms are C++ no-ops). C++
// EffectSchoolDamage ADDS the rolled DBC base on top of the rewritten
// Damage (SpellEffects.cpp:788), so these entries replace the base
// only where the DBC base is 0 — unverified without Spell.dbc.
func onHitDamageRewrite(spellID uint32, victimHealth, victimMaxHealth, baseDamage uint32) (uint32, bool) {
	switch spellID {
	case 48292:
		// spell_dark_slash (boss_ymiron.cpp:315-334): the OnHit handler
		// sets the hit damage to ceil(victim health / 2) before the
		// effect loop; health is pre-damage here, matching
		// GetHitUnit()->GetHealth() at OnHit time.
		return (victimHealth + 1) / 2, true
	case 33666, 38795:
		// spell_murmur_sonic_boom_effect (boss_murmur.cpp:192-211):
		// damage = 80% of the target's max health
		// (CountPctFromMaxHealth(80) = float(maxHealth)*80/100,
		// Util.h:66-69). The class header binds both the normal (33666)
		// and heroic (38795) effect ids.
		return uint32(float32(victimMaxHealth) * 80.0 / 100.0), true
	case 48642:
		// spell_skadi_launch_harpoon::HandleDamageCalc
		// (boss_skadi.cpp:797-801): damage = 35% of the hit unit's
		// max health (target->CountPctFromMaxHealth(35), an
		// ABSOLUTE value, not GetHitDamage()-relative — unlike the
		// bone-storm / tympanic / bone-slice / koralon / defile /
		// cosmic-smash arms, which are C++ no-ops at OnHit).
		// The FilterTargets leg (keep only Grauf when >=2 targets,
		// :791-795) stays out: no Go target-select hook for this
		// spell, so the rewrite applies to whichever targets the
		// engine selects.
		return uint32(float32(victimMaxHealth) * 35.0 / 100.0), true
	}
	return baseDamage, false
}
