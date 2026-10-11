package world

// Aura-driven melee-combat stat arms: AuraEffect::HandleAuraModDodgePercent
// (SpellAuraEffects.cpp:3786 — aura type 49,
// SPELL_AURA_MOD_DODGE_PERCENT), HandleAuraModBlockPercent (:3799 — aura
// 51, SPELL_AURA_MOD_BLOCK_PERCENT), HandleAuraModWeaponCritPercent (:3817
// — aura 52, SPELL_AURA_MOD_WEAPON_CRIT_PERCENT), HandleAuraModCritPct
// (:3870 — aura 290, SPELL_AURA_MOD_CRIT_PCT — the non-item arm feeds
// Player::UpdateWeaponDependentCritAuras via
// Player::UpdateAllWeaponDependentCritAuras, Player.cpp:7905), and
// HandleAuraModExpertise (:3596 — aura 240, SPELL_AURA_MOD_EXPERTISE).
//
// C++ re-runs Player::UpdateDodgePercentage / UpdateBlockPercentage /
// UpdateCritPercentage / UpdateExpertise on apply/remove/change-amount; Go
// folds all four into calculatePlayerStats (player_state.go), so the
// apply/merge/remove legs below call that instead of targeted updates.
//
// Fold semantics (all player-only in C++): 49 adds flat to the
// nondiminishing leg of UpdateDodgePercentage (StatSystem.cpp:805); 51
// adds flat to UpdateBlockPercentage (StatSystem.cpp:612), clamped at 0;
// 52 + 290 add flat to the weapon-crit base mod of
// UpdateWeaponDependentCritAuras (Player.cpp:7901-7905) — the 290 leg is
// already folded into the spell-crit chance arms of calculatePlayerStats,
// it additionally feeds weapon crit here; 240 adds raw expertise skill
// points (not rating) to GetExpertise / UpdateExpertise
// (StatSystem.cpp:880).
//
// Deltas: the C++ weapon-fit predicates on 52
// (CheckAttackFitToAuraRequirement, Player.cpp:7901) and 240
// (IsItemFitToSpellRequirements, StatSystem.cpp:880-884) have no Go model
// — the flat folds sum unconditionally. The CONFIG_STATS_LIMITS_ENABLE
// caps are not modeled (tree-wide convention).

const (
	spellAuraModDodgePercent = 49  // SPELL_AURA_MOD_DODGE_PERCENT (SpellAuraDefines.h:129)
	spellAuraModExpertise    = 240 // SPELL_AURA_MOD_EXPERTISE (SpellAuraDefines.h:320)
	// spellAuraModBlockPercent (51), spellAuraModWeaponCritPercent (52),
	// spellAuraModParryPercent (47) already exist in spells.go.
	//
	// spellAuraAddCasterHitTrigger bridges Unit::GetMeleeHitRedirectTarget
	// (Unit.cpp:6349-6363) at the auto-attack site: the victim-side 111
	// aura (Warrior Intervene 3411 / hunter-pet Intervene 53476) redirects
	// the next melee swing onto the aura's caster. See meleeHitRedirectGUID
	// in combat.go.
	spellAuraAddCasterHitTrigger = 111 // SPELL_AURA_ADD_CASTER_HIT_TRIGGER (SpellAuraDefines.h:191)
)

// auraTouchesMeleeCombat reports whether the aura record carries any of the
// melee-combat stat effects (dodge 49, block 51, weapon crit 52, crit pct
// 290, expertise 240) whose folds live inside calculatePlayerStats.
func (s *session) auraTouchesMeleeCombat(aura *activeAura) bool {
	return s.activeAuraHasEffect(aura, spellAuraModDodgePercent) ||
		s.activeAuraHasEffect(aura, spellAuraModBlockPercent) ||
		s.activeAuraHasEffect(aura, spellAuraModWeaponCritPercent) ||
		s.activeAuraHasEffect(aura, spellAuraModCritPct) ||
		s.activeAuraHasEffect(aura, spellAuraModExpertise)
}
