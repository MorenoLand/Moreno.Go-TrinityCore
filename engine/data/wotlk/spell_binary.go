package wotlk

// Binary-spell custom attribute. Mirrors the SPELL_ATTR0_CU_BINARY_SPELL arm
// of SpellMgr::LoadSpellInfoCustomAttributes
// (src/server/game/Spells/SpellMgr.cpp:2755-2825), which the server evaluates
// once per spell at load. Go computes the bit once per spell in Store.Spell
// and memoizes it in the Store, so every consumer sees the load-time value.
//
// The fork's second binary pass (SpellMgr.cpp:2874-2910, "omit spells
// triggering other spells") is a provable no-op there: it skips every spell
// that already carries the flag and then clears the flag on spells that do
// not carry it. It is deliberately not modeled.

const (
	spellAttr3IgnoreHitResult             uint32 = 0x00040000 // SPELL_ATTR3_IGNORE_HIT_RESULT (SharedDefines.h)
	spellAttr0UnaffectedByInvulnerability uint32 = 0x20000000 // SPELL_ATTR0_UNAFFECTED_BY_INVULNERABILITY (SharedDefines.h)

	spellAttr0CuBinarySpell uint32 = 0x00100000 // SPELL_ATTR0_CU_BINARY_SPELL (SpellInfo.h:198)

	spellEffectTriggerSpell          = 64  // SPELL_EFFECT_TRIGGER_SPELL (SharedDefines.h:875)
	spellEffectTriggerSpellWithValue = 142 // SPELL_EFFECT_TRIGGER_SPELL_WITH_VALUE (SharedDefines.h:953)

	spellAuraModPossess           = 2   // SPELL_AURA_MOD_POSSESS (SpellAuraDefines.h:82)
	spellAuraModCharm             = 6   // SPELL_AURA_MOD_CHARM (SpellAuraDefines.h:86)
	spellAuraAoeCharm             = 177 // SPELL_AURA_AOE_CHARM (SpellAuraDefines.h:257)
	spellAuraPeriodicHealthFunnel = 62  // SPELL_AURA_PERIODIC_HEALTH_FUNNEL (SpellAuraDefines.h:142)
	spellAuraPeriodicDummy        = 226 // SPELL_AURA_PERIODIC_DUMMY (SpellAuraDefines.h:306)

	spellFamilyMage    = 3 // SPELLFAMILY_MAGE (SharedDefines.h:3584)
	spellFamilyWarlock = 5 // SPELLFAMILY_WARLOCK (SharedDefines.h:3586)
)

// initializeSpellBinaryFlag mirrors the binary-spell arm of
// SpellMgr::LoadSpellInfoCustomAttributes (SpellMgr.cpp:2755-2825): spells
// ignoring the hit result are never binary; otherwise any effect that is not
// a damage/trigger effect and not a periodic/dummy/leech aura effect sets the
// flag, unless the effect has no value and is neither interrupt/crowd-control
// nor explicitly flagged (with the Sindragosa Frost Breath, Frostbolt,
// Frost Fever and Haunt exceptions). The SPELL_ATTR0_CU_AURA_CC read inside
// the check sees only the aura-name-derived CC bits: those are set earlier in
// the same per-spell load iteration (SpellMgr.cpp:2663-2672), while the
// family-derived CC bits (warrior shouts, druid roars) are set after the
// binary arm (SpellMgr.cpp:2886-2902) and are not visible to it.
func initializeSpellBinaryFlag(spell *Spell) {
	if spell.AttributesEx3&spellAttr3IgnoreHitResult != 0 {
		return
	}
	auraCC := false
	for j := range spell.Effects {
		switch spell.Effects[j].Aura {
		case spellAuraModPossess, spellAuraModConfuse, spellAuraModCharm,
			spellAuraAoeCharm, spellAuraModFear, spellAuraModStun:
			auraCC = true
		}
	}
	setFlag := false
	for j := range spell.Effects {
		eff := spell.Effects[j]
		if eff.Effect == 0 {
			continue
		}
		switch eff.Effect {
		case spellEffectSchoolDamage, spellEffectWeaponDamage,
			spellEffectWeaponDamageNoschool, spellEffectNormalizedWeaponDmg,
			spellEffectWeaponPercentDamage, spellEffectTriggerSpell,
			spellEffectTriggerSpellWithValue:
			continue
		case spellEffectPersistentAreaAura, spellEffectApplyAura,
			spellEffectApplyAreaAuraParty, spellEffectApplyAreaAuraRaid,
			spellEffectApplyAreaAuraFriend, spellEffectApplyAreaAuraEnemy,
			spellEffectApplyAreaAuraPet, spellEffectApplyAreaAuraOwner:
			switch eff.Aura {
			case spellAuraPeriodicDamage, spellAuraPeriodicDamagePercent,
				spellAuraDummy, spellAuraPeriodicLeech,
				spellAuraPeriodicHealthFunnel, spellAuraPeriodicDummy:
				continue
			}
			if binaryEffectSetsFlag(spell, eff, auraCC) {
				setFlag = true
			}
		default:
			if binaryEffectSetsFlag(spell, eff, auraCC) {
				setFlag = true
			}
		}
	}
	if setFlag {
		spell.AttributesCu |= spellAttr0CuBinarySpell
	}
}

// binaryEffectSetsFlag mirrors the default arm of the binary-spell effect
// switch (SpellMgr.cpp:2807-2822).
func binaryEffectSetsFlag(spell *Spell, eff SpellEffect, auraCC bool) bool {
	// No value and not interrupt cast or crowd control without
	// SPELL_ATTR0_UNAFFECTED_BY_INVULNERABILITY flag
	if eff.CalcValue() == 0 &&
		!((eff.Effect == spellEffectInterruptCast || auraCC) &&
			spell.Attributes&spellAttr0UnaffectedByInvulnerability == 0) {
		return false
	}
	// Sindragosa Frost Breath
	switch spell.ID {
	case 69649, 71056, 71057, 71058, 73061, 73062, 73063, 73064:
		return false
	}
	// Frostbolt
	if spell.SpellFamilyName == spellFamilyMage && spell.SpellFamilyFlags[0]&0x20 != 0 {
		return false
	}
	// Frost Fever
	if spell.ID == 55095 {
		return false
	}
	// Haunt
	if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[1]&0x40000 != 0 {
		return false
	}
	return true
}
