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
// their tick-372 "ported scaling" notes were wrong. OnEffectHitTarget
// arms qualify too — IsInModifiableHook (SpellScript.cpp:421-434)
// covers EFFECT_HIT_TARGET — so 50341 (oculus) is live; the dispatch
// carries casterMaxHealth for caster-absolute arms. OnEffectLaunchTarget
// arms qualify the same way (HandleEffects runs the script hook before
// the effect handler, Spell.cpp:5165-5171, with damage already the
// rolled DBC base), so 49840 (oculus shock lance) is live via
// shockLanceDetonate. The stateful arms (49840, 34779, 60103, 47496) live in
// this Server method, not the pure table below: 49840 reads and consumes
// the target's charge aura, 34779 picks its trigger spell from the
// caster's map difficulty, and 60103 takes its Flametongue gate
// (lavaLashBoost, computed once per cast by the player-caster arm —
// creature casters never satisfy it, and the spell's Load() gate
// requires TYPEID_PLAYER anyway). 47496 takes the caster's max health
// for its pct-of-max-health damage (the ghoul's health, not the
// target's — the dispatch already threads casterMaxHealth).
func (s *Server) spellScriptOnHitDamage(spell wotlk.Spell, casterLevel uint32, casterX, casterY, victimX, victimY float32, victimHealth, victimMaxHealth, casterMaxHealth, baseDamage uint32, casterGUID, victimGUID uint64, mapID, instanceID uint32, lavaLashBoost bool) (uint32, bool) {
	switch spell.ID {
	case 49840:
		return s.shockLanceDetonate(casterGUID, victimGUID, mapID, instanceID, baseDamage)
	case 34779:
		return s.freezingCircleDamage(mapID, instanceID)
	case 47496:
		// spell_dk_ghoul_explode::HandleDamage (spell_dk.cpp:1187-1191):
		// the OnEffectLaunchTarget arm REPLACES the hit damage with
		// CountPctFromMaxHealth(EFFECT_2 CalcValue) — pct of the
		// caster's max health. Same hook position as the 49840 arm
		// (HandleEffects runs it before EffectSchoolDMG), so the
		// dispatch returns the computed value outright, not base+value.
		// CalcValueForLevel is the CalcValue(caster) twin (level clamp
		// + per-level + die roll, store.go:157). The Suicide leg
		// (EFFECT_1 OnEffectHitTarget: triggered 47496-triggered +
		// visual cast on the hit unit) stays out — no triggered-cast
		// leg in this dispatch.
		pct := spell.Effects[2].CalcValueForLevel(spell, casterLevel)
		if pct <= 0 {
			return baseDamage, false
		}
		return casterMaxHealth * uint32(pct) / 100, true
	case 60103:
		if !lavaLashBoost {
			return baseDamage, false
		}
		// spell_sha_lava_lash::HandleDummy (spell_shaman.cpp:1481-1495):
		// the dummy effect's value is the percent (25): AddPct(hitDamage,
		// damage) adds hitDamage * damage / 100. GetHitDamage() here is
		// the launch damage (baseDamage at this dispatch position), not
		// the OnHit 0 — this is an OnEffectHitTarget arm, and m_damage
		// already holds the launch accumulation at hit time
		// (TargetInfo::DoTargetSpellHit, Spell.cpp:2409-2425).
		pct := spell.Effects[1].CalcValueForLevel(spell, casterLevel)
		if pct <= 0 {
			return baseDamage, false
		}
		return baseDamage + baseDamage*uint32(pct)/100, true
	}
	return onHitDamageRewrite(spell.ID, victimHealth, victimMaxHealth, casterMaxHealth, baseDamage)
}

// shockLanceDetonate mirrors spell_oculus_shock_lance::CalcDamage
// (oculus.cpp:524-538): the OnEffectLaunchTarget arm adds the target's
// SPELL_AMBER_SHOCK_CHARGE (49836) EFFECT_0 amount — from this caster
// only (GetAuraEffect(spell, eff, casterGUID)) — to the rolled DBC base,
// then removes the charge aura (AURA_REMOVE_BY_ENEMY_SPELL). ADDITIVE:
// HandleEffects (Spell.cpp:5165) already set damage to the rolled base
// before the hook, and EffectSchoolDMG starts from that rewritten value
// (SpellEffects.cpp:322-348), so the charge adds on top of base — the
// same shape as the 50341 case in the table below. The amount is the
// single DBC effect value, not stack-scaled: Aura::ModStackAmount
// (SpellAuras.cpp:1030-1068) never rescales m_amount. Player victims
// resolve through the victim session's activeAuras under its castMu
// (the cloneCasterGUIDForUnit pattern, aura_state.go:173-205);
// re-application preserves Amounts (spells.go:37988), so the player
// leg reads the same single DBC value. Neither dispatch call site
// holds a session castMu (no castMu lock in spells.go:2271-27819 or
// the lua_player.go CastSpell binding), so locking the victim
// session here cannot invert with the caster's lock.
func (s *Server) shockLanceDetonate(casterGUID, victimGUID uint64, mapID, instanceID uint32, baseDamage uint32) (uint32, bool) {
	if s == nil || victimGUID == 0 {
		return baseDamage, false
	}
	if victim := s.findCreatureMotion(mapID, instanceID, victimGUID); victim != nil {
		key := creatureAuraKeyForMotion(victim)
		s.auraMu.Lock()
		aura := s.activeCreatureAuras[key][49836]
		s.auraMu.Unlock()
		if aura == nil || aura.EffectMask&(1<<0) == 0 || aura.CasterGUID != casterGUID {
			return baseDamage, false
		}
		amount := aura.Amounts[0]
		s.removeCreatureAura(key, 49836)
		if amount <= 0 {
			return baseDamage, true
		}
		return baseDamage + uint32(amount), true
	}
	if vs := s.findSessionByGUID(victimGUID); vs != nil && vs.player != nil {
		vs.castMu.Lock()
		aura := vs.activeAuras[49836]
		matched := aura != nil && !aura.Stopped && aura.EffectMask&(1<<0) != 0 && aura.CasterGUID == casterGUID
		amount := int32(0)
		if matched {
			amount = aura.Amounts[0]
		}
		vs.castMu.Unlock()
		if !matched {
			return baseDamage, false
		}
		vs.removeAuraWithMode(49836, auraRemoveByEnemySpell)
		if amount <= 0 {
			return baseDamage, true
		}
		return baseDamage + uint32(amount), true
	}
	return baseDamage, false
}

// freezingCircleDamage mirrors spell_freezing_circle::HandleDamage
// (spell_generic.cpp:4271-4287): the OnEffectHitTarget arm REPLACES the
// hit damage with the trigger spell's EFFECT_0 CalcValue — the Pit of
// Saron heroic (70276) / normal (69574) trigger in dungeons, the base
// trigger (34787) anywhere else. CalcValue() with no caster takes no
// per-level scaling (SpellInfo.cpp:402-425), so the Go twin is
// Effects[0].CalcValue(), not CalcValueForLevel. The difficulty comes
// from the first session on the instance (autoBalanceInstanceDifficulty);
// a dungeon with nobody on it reports normal, matching an empty
// instance's default difficulty.
func (s *Server) freezingCircleDamage(mapID, instanceID uint32) (uint32, bool) {
	if s == nil || s.Data == nil {
		return 0, false
	}
	triggerID := uint32(34787)
	if entry, found, err := s.Data.Map(mapID); err == nil && found && entry.IsDungeon() {
		dungeonDiff, _ := s.autoBalanceInstanceDifficulty(mapID, instanceID)
		if dungeonDiff == 1 { // DUNGEON_DIFFICULTY_HEROIC
			triggerID = 70276
		} else {
			triggerID = 69574
		}
	}
	trigger, found, err := s.Data.Spell(triggerID)
	if err != nil || !found {
		return 0, false
	}
	value := trigger.Effects[0].CalcValue()
	if value <= 0 {
		return 0, false
	}
	return uint32(value), true
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
// Damage (SpellEffects.cpp:788), so entries replace the base only
// where the DBC base is 0 — unverified without Spell.dbc — except
// 50341, whose nonzero base (25000, oculus.cpp:80) is kept and the
// 30% caster-max-health rewrite is added on top.
func onHitDamageRewrite(spellID uint32, victimHealth, victimMaxHealth, casterMaxHealth, baseDamage uint32) (uint32, bool) {
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
	case 50341:
		// spell_oculus_touch_the_nightmare (oculus.cpp:632-655): the
		// OnEffectHitTarget handler sets the hit damage to 30% of the
		// caster's max health. ADDITIVE: EffectSchoolDamage adds the
		// rolled DBC base on top of the rewritten damage
		// (SpellEffects.cpp:788), and effect 2's base is 25000
		// (oculus.cpp:80 tooltip), so the base is kept.
		return baseDamage + casterMaxHealth*30/100, true
	}
	return baseDamage, false
}
