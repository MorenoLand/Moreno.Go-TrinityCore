package wotlk

// Spell positivity bits. Mirrors SpellInfo::_InitializeSpellPositivity and
// the DBC-visible unconditional arms of _isPositiveEffectImpl
// (src/server/game/Spells/SpellInfo.cpp:3385-3847), which the server
// evaluates once per spell at load to fill the SPELL_ATTR0_CU_NEGATIVE_EFF*
// custom attributes (src/server/game/Spells/SpellInfo.h:190-203).
// SpellInfo::IsPositiveEffect (SpellInfo.cpp:1213-1222) then reads them;
// the no-damage proc arm (Spell.cpp:2447-2457) sweeps that read over the
// effect mask. Go computes the bits once per spell in Store.Spell and
// memoizes them in the Store, so every consumer sees the load-time value.
//
// Modeled: empty slots, passive spells, SPELL_ATTR0_NEGATIVE_1, the
// whole-spell pre-scan (heal/learn/skill-step/heal-pct positive;
// same-target instakill negative; stealth/unattackable positive;
// school-heal-absorb/empathy/mod-damage-from-caster/prevents-fleeing
// negative), the unconditional per-effect negative/positive lists, the
// dispel misc-value negative arm, the unconditional aura negative arms,
// the triggered-spell recursion (cycle-guarded like the C++ visited set),
// and the same-target sibling-debuff cross rule. Not modeled (left
// positive, the C++ tail default): the spell-family flag/ID exception
// tables, the mechanic IMMUNE_SHIELD arm, the SPELL_ATTR1_UNK11 target
// sweep, every _isPositiveTarget/bp-dependent arm, and spell_custom_attr
// DB bits (Go has no spell_custom_attr model).

const (
	spellAttr0Passive   uint32 = 0x00000040 // SPELL_ATTR0_PASSIVE (SharedDefines.h:418)
	spellAttr0Negative1 uint32 = 0x04000000 // SPELL_ATTR0_NEGATIVE_1 (SharedDefines.h:438)

	spellAttr0CuNegativeEff0 uint32 = 0x00001000 // SPELL_ATTR0_CU_NEGATIVE_EFF0 (SpellInfo.h:190)
)

const (
	spellEffectInstakill            = 1   // SPELL_EFFECT_INSTAKILL (SharedDefines.h:812)
	spellEffectSchoolDamage         = 2   // SPELL_EFFECT_SCHOOL_DAMAGE (SharedDefines.h:813)
	spellEffectApplyAura            = 6   // SPELL_EFFECT_APPLY_AURA (SharedDefines.h:817)
	spellEffectEnvironmentalDamage  = 7   // SPELL_EFFECT_ENVIRONMENTAL_DAMAGE (SharedDefines.h:819)
	spellEffectPowerDrain           = 8   // SPELL_EFFECT_POWER_DRAIN (SharedDefines.h:820)
	spellEffectHealthLeech          = 9   // SPELL_EFFECT_HEALTH_LEECH (SharedDefines.h:821)
	spellEffectHeal                 = 10  // SPELL_EFFECT_HEAL (SharedDefines.h:821)
	spellEffectWeaponDamageNoschool = 17  // SPELL_EFFECT_WEAPON_DAMAGE_NOSCHOOL (SharedDefines.h:828)
	spellEffectPersistentAreaAura   = 27  // SPELL_EFFECT_PERSISTENT_AREA_AURA (SharedDefines.h:838)
	spellEffectEnergize             = 30  // SPELL_EFFECT_ENERGIZE (SharedDefines.h:842)
	spellEffectWeaponPercentDamage  = 31  // SPELL_EFFECT_WEAPON_PERCENT_DAMAGE (SharedDefines.h:843)
	spellEffectApplyAreaAuraParty   = 35  // SPELL_EFFECT_APPLY_AREA_AURA_PARTY (SharedDefines.h:846)
	spellEffectLearnSpell           = 36  // SPELL_EFFECT_LEARN_SPELL (SharedDefines.h:847)
	spellEffectDispel               = 38  // SPELL_EFFECT_DISPEL (SharedDefines.h:849)
	spellEffectSkillStep            = 44  // SPELL_EFFECT_SKILL_STEP (SharedDefines.h:855)
	spellEffectTameCreature         = 55  // SPELL_EFFECT_TAMECREATURE (SharedDefines.h:866)
	spellEffectWeaponDamage         = 58  // SPELL_EFFECT_WEAPON_DAMAGE (SharedDefines.h:869)
	spellEffectApplyAreaAuraRaid    = 65  // SPELL_EFFECT_APPLY_AREA_AURA_RAID (SharedDefines.h:876)
	spellEffectHealMaxHealth        = 67  // SPELL_EFFECT_HEAL_MAX_HEALTH (SharedDefines.h:878)
	spellEffectInterruptCast        = 68  // SPELL_EFFECT_INTERRUPT_CAST (SharedDefines.h:879)
	spellEffectDistract             = 69  // SPELL_EFFECT_DISTRACT (SharedDefines.h:880)
	spellEffectPickpocket           = 71  // SPELL_EFFECT_PICKPOCKET (SharedDefines.h:882)
	spellEffectHealMechanical       = 75  // SPELL_EFFECT_HEAL_MECHANICAL (SharedDefines.h:886)
	spellEffectGameobjectDamage     = 87  // SPELL_EFFECT_GAMEOBJECT_DAMAGE (SharedDefines.h:898)
	spellEffectDurabilityDamage     = 111 // SPELL_EFFECT_DURABILITY_DAMAGE (SharedDefines.h:922)
	spellEffectDurabilityDamagePct  = 115 // SPELL_EFFECT_DURABILITY_DAMAGE_PCT (SharedDefines.h:926)
	spellEffectApplyAreaAuraPet     = 119 // SPELL_EFFECT_APPLY_AREA_AURA_PET (SharedDefines.h:930)
	spellEffectNormalizedWeaponDmg  = 121 // SPELL_EFFECT_NORMALIZED_WEAPON_DMG (SharedDefines.h:932)
	spellEffectStealBeneficialBuff  = 126 // SPELL_EFFECT_STEAL_BENEFICIAL_BUFF (SharedDefines.h:937)
	spellEffectApplyAreaAuraFriend  = 128 // SPELL_EFFECT_APPLY_AREA_AURA_FRIEND (SharedDefines.h:939)
	spellEffectApplyAreaAuraEnemy   = 129 // SPELL_EFFECT_APPLY_AREA_AURA_ENEMY (SharedDefines.h:940)
	spellEffectHealPct              = 136 // SPELL_EFFECT_HEAL_PCT (SharedDefines.h:947)
	spellEffectEnergizePct          = 137 // SPELL_EFFECT_ENERGIZE_PCT (SharedDefines.h:948)
	spellEffectApplyAreaAuraOwner   = 143 // SPELL_EFFECT_APPLY_AREA_AURA_OWNER (SharedDefines.h:954)

	spellAuraPeriodicDamage                 = 3   // SPELL_AURA_PERIODIC_DAMAGE (SpellAuraDefines.h:83)
	spellAuraDummy                          = 4   // SPELL_AURA_DUMMY (SpellAuraDefines.h:84)
	spellAuraModConfuse                     = 5   // SPELL_AURA_MOD_CONFUSE (SpellAuraDefines.h:85)
	spellAuraModFear                        = 7   // SPELL_AURA_MOD_FEAR (SpellAuraDefines.h:87)
	spellAuraModAttackspeed                 = 9   // SPELL_AURA_MOD_ATTACKSPEED (SpellAuraDefines.h:89)
	spellAuraModTaunt                       = 11  // SPELL_AURA_MOD_TAUNT (SpellAuraDefines.h:91)
	spellAuraModStun                        = 12  // SPELL_AURA_MOD_STUN (SpellAuraDefines.h:92)
	spellAuraModStealth                     = 16  // SPELL_AURA_MOD_STEALTH (SpellAuraDefines.h:96)
	spellAuraModRoot                        = 26  // SPELL_AURA_MOD_ROOT (SpellAuraDefines.h:106)
	spellAuraModSilence                     = 27  // SPELL_AURA_MOD_SILENCE (SpellAuraDefines.h:107)
	spellAuraModDecreaseSpeed               = 33  // SPELL_AURA_MOD_DECREASE_SPEED (SpellAuraDefines.h:113)
	spellAuraPeriodicLeech                  = 53  // SPELL_AURA_PERIODIC_LEECH (SpellAuraDefines.h:133)
	spellAuraTransform                      = 56  // SPELL_AURA_TRANSFORM (SpellAuraDefines.h:136)
	spellAuraPeriodicManaLeech              = 64  // SPELL_AURA_PERIODIC_MANA_LEECH (SpellAuraDefines.h:144)
	spellAuraModStalked                     = 68  // SPELL_AURA_MOD_STALKED (SpellAuraDefines.h:148)
	spellAuraChannelDeathItem               = 86  // SPELL_AURA_CHANNEL_DEATH_ITEM (SpellAuraDefines.h:166)
	spellAuraPeriodicDamagePercent          = 89  // SPELL_AURA_PERIODIC_DAMAGE_PERCENT (SpellAuraDefines.h:169)
	spellAuraPreventsFleeing                = 92  // SPELL_AURA_PREVENTS_FLEEING (SpellAuraDefines.h:172)
	spellAuraModUnattackable                = 93  // SPELL_AURA_MOD_UNATTACKABLE (SpellAuraDefines.h:173)
	spellAuraGhost                          = 95  // SPELL_AURA_GHOST (SpellAuraDefines.h:175)
	spellAuraEmpathy                        = 121 // SPELL_AURA_EMPATHY (SpellAuraDefines.h:201)
	spellAuraRangedAttackPowerAttackerBonus = 127 // SPELL_AURA_RANGED_ATTACK_POWER_ATTACKER_BONUS (SpellAuraDefines.h:207)
	spellAuraMeleeAttackPowerAttackerBonus  = 165 // SPELL_AURA_MELEE_ATTACK_POWER_ATTACKER_BONUS (SpellAuraDefines.h:245)
	spellAuraModDetaunt                     = 221 // SPELL_AURA_MOD_DETAUNT (SpellAuraDefines.h:301)
	spellAuraModDamageFromCaster            = 271 // SPELL_AURA_MOD_DAMAGE_FROM_CASTER (SpellAuraDefines.h:351)
	spellAuraSchoolHealAbsorb               = 301 // SPELL_AURA_SCHOOL_HEAL_ABSORB (SpellAuraDefines.h:381)
	spellAuraPreventResurrection            = 314 // SPELL_AURA_PREVENT_RESURRECTION (SpellAuraDefines.h:394)

	dispelStealth      = 5 // DISPEL_STEALTH (SharedDefines.h:1409)
	dispelInvisibility = 6 // DISPEL_INVISIBILITY (SharedDefines.h:1410)
	dispelEnrage       = 9 // DISPEL_ENRAGE (SharedDefines.h:1413)
)

// IsPositiveEffect mirrors SpellInfo::IsPositiveEffect
// (src/server/game/Spells/SpellInfo.cpp:1213-1222): the effect is positive
// unless its CU_NEGATIVE_EFF bit was set at load.
func (spell Spell) IsPositiveEffect(effIndex int) bool {
	if effIndex < 0 || effIndex >= len(spell.Effects) {
		return false
	}
	return spell.AttributesCu&(spellAttr0CuNegativeEff0<<uint(effIndex)) == 0
}

// initializeSpellPositivity mirrors SpellInfo::_InitializeSpellPositivity
// (SpellInfo.cpp:3848-3886): set the CU_NEGATIVE_EFF bit for every effect
// the impl reports negative, then apply the same-target sibling-debuff
// cross rule. lookup resolves triggered spells; visiting is the shared
// cycle guard (the C++ visited set).
func initializeSpellPositivity(spell *Spell, lookup func(uint32) (Spell, bool), visiting map[[2]uint32]bool) {
	for i := range spell.Effects {
		if !isPositiveEffectImpl(spell, i, lookup, visiting) {
			spell.AttributesCu |= spellAttr0CuNegativeEff0 << uint(i)
		}
	}
	for i := range spell.Effects {
		if spell.Effects[i].Effect == 0 || !spell.IsPositiveEffect(i) {
			continue
		}
		switch spell.Effects[i].Aura {
		case spellAuraDummy, spellAuraModStun, spellAuraModFear, spellAuraModTaunt,
			spellAuraTransform, spellAuraModAttackspeed, spellAuraModDecreaseSpeed:
			for j := i + 1; j < len(spell.Effects); j++ {
				if !spell.IsPositiveEffect(j) &&
					spell.Effects[i].ImplicitTargetA == spell.Effects[j].ImplicitTargetA &&
					spell.Effects[i].ImplicitTargetB == spell.Effects[j].ImplicitTargetB {
					spell.AttributesCu |= spellAttr0CuNegativeEff0 << uint(i)
				}
			}
		}
	}
}

// isPositiveEffectImpl mirrors the DBC-visible unconditional arms of
// _isPositiveEffectImpl (SpellInfo.cpp:3385-3847) in C++ evaluation order.
func isPositiveEffectImpl(spell *Spell, effIndex int, lookup func(uint32) (Spell, bool), visiting map[[2]uint32]bool) bool {
	effect := spell.Effects[effIndex]
	if effect.Effect == 0 {
		return true
	}
	if spell.Attributes&spellAttr0Passive != 0 {
		return true
	}
	if spell.Attributes&spellAttr0Negative1 != 0 {
		return false
	}
	visiting[[2]uint32{spell.ID, uint32(effIndex)}] = true

	if decided, positive := prescanEffectPositivity(spell, effIndex); decided {
		return positive
	}

	switch effect.Effect {
	case spellEffectWeaponDamage, spellEffectWeaponDamageNoschool, spellEffectNormalizedWeaponDmg,
		spellEffectWeaponPercentDamage, spellEffectSchoolDamage, spellEffectEnvironmentalDamage,
		spellEffectHealthLeech, spellEffectInstakill, spellEffectPowerDrain,
		spellEffectStealBeneficialBuff, spellEffectInterruptCast, spellEffectPickpocket,
		spellEffectGameobjectDamage, spellEffectDurabilityDamage, spellEffectDurabilityDamagePct,
		spellEffectApplyAreaAuraEnemy, spellEffectTameCreature, spellEffectDistract:
		return false
	case spellEffectEnergize, spellEffectEnergizePct, spellEffectHealPct,
		spellEffectHealMaxHealth, spellEffectHealMechanical:
		return true
	case spellEffectDispel:
		switch effect.MiscValue {
		case dispelStealth, dispelInvisibility, dispelEnrage:
			return false
		}
	}

	if isAuraEffect(effect) {
		switch effect.Aura {
		case spellAuraModConfuse, spellAuraChannelDeathItem, spellAuraModRoot,
			spellAuraModSilence, spellAuraModDetaunt, spellAuraGhost,
			spellAuraPeriodicLeech, spellAuraPeriodicManaLeech, spellAuraModStalked,
			spellAuraPreventResurrection, spellAuraPeriodicDamage,
			spellAuraPeriodicDamagePercent, spellAuraMeleeAttackPowerAttackerBonus,
			spellAuraRangedAttackPowerAttackerBonus:
			return false
		}
	}

	if effect.Aura == 0 && effect.TriggerSpell != 0 {
		if triggered, ok := lookup(effect.TriggerSpell); ok {
			for i := range triggered.Effects {
				if triggered.Effects[i].Effect == 0 {
					continue
				}
				if visiting[[2]uint32{triggered.ID, uint32(i)}] {
					continue
				}
				if !isPositiveEffectImpl(&triggered, i, lookup, visiting) {
					return false
				}
			}
		}
	}

	return true
}

// prescanEffectPositivity mirrors the "effects which determine positivity of
// whole spell" pre-scan (SpellInfo.cpp:3495-3533): the first matching effect
// in index order decides for the queried effect.
func prescanEffectPositivity(spell *Spell, effIndex int) (bool, bool) {
	for i := range spell.Effects {
		switch spell.Effects[i].Effect {
		case spellEffectHeal, spellEffectLearnSpell, spellEffectSkillStep, spellEffectHealPct:
			return true, true
		case spellEffectInstakill:
			if i != effIndex &&
				spell.Effects[i].ImplicitTargetA == spell.Effects[effIndex].ImplicitTargetA &&
				spell.Effects[i].ImplicitTargetB == spell.Effects[effIndex].ImplicitTargetB {
				return true, false
			}
		}
		if isAuraEffect(spell.Effects[i]) {
			switch spell.Effects[i].Aura {
			case spellAuraModStealth, spellAuraModUnattackable:
				return true, true
			case spellAuraSchoolHealAbsorb, spellAuraEmpathy, spellAuraModDamageFromCaster,
				spellAuraPreventsFleeing:
				return true, false
			}
		}
	}
	return false, false
}

// isAuraEffect mirrors SpellEffectInfo::IsAura (SpellInfo.cpp:368-371):
// a unit-owned or persistent area aura effect with a set aura name.
func isAuraEffect(effect SpellEffect) bool {
	switch effect.Effect {
	case spellEffectApplyAura, spellEffectPersistentAreaAura,
		spellEffectApplyAreaAuraParty, spellEffectApplyAreaAuraRaid,
		spellEffectApplyAreaAuraFriend, spellEffectApplyAreaAuraEnemy,
		spellEffectApplyAreaAuraPet, spellEffectApplyAreaAuraOwner:
		return effect.Aura != 0
	}
	return false
}
