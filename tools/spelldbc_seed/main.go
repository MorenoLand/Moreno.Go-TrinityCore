// Command spelldbc_seed generates the minimal embedded Spell.dbc seed
// (engine/data/wotlk/seeddbc/) that unblocks the Lua CastSpell /
// GO-summon legs until real client DBC data is deployed. Every row is
// Wowhead-verified per field (see spelldbc_strip_unitB.md); rows flagged
// INFERRED are derived from in-tree C++/Go constraints, not client DBC
// rows, and must be replaced when real data ships. Disk DBCs always win:
// the seed is consulted only when dbc.Open fails (seed.go).
//
// Field layout is per engine/data/wotlk/store.go (0-based; field 0 =
// ID). Spell.dbc rows are 234 fields (real WotLK width, DBCfmt.h:117).
// Aux tables use seed-local IDs (90001+, 91001+, 92001+, 93001+) so no
// row asserts a false real DBC ID, except SpellDuration/SpellRadius row
// 1, which Unit A verified against the real layout.
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func writeWDBC(path string, fieldCount int, rows [][]uint32) error {
	recordSize := uint32(fieldCount * 4)
	var buf []byte
	hdr := make([]byte, 20)
	copy(hdr[0:4], "WDBC")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(rows)))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(fieldCount))
	binary.LittleEndian.PutUint32(hdr[12:16], recordSize)
	binary.LittleEndian.PutUint32(hdr[16:20], 1)
	buf = append(buf, hdr...)
	for _, row := range rows {
		rec := make([]byte, recordSize)
		for i, v := range row {
			if i >= fieldCount {
				return fmt.Errorf("row has %d fields, exceeds %d", len(row), fieldCount)
			}
			binary.LittleEndian.PutUint32(rec[i*4:], v)
		}
		buf = append(buf, rec...)
	}
	buf = append(buf, 0)
	return os.WriteFile(path, buf, 0644)
}

func f32(v float32) uint32 { return math.Float32bits(v) }

const (
	effInstakill               = 1 // SPELL_EFFECT_INSTAKILL (SharedDefines.h:812)
	effSchoolDamage            = 2
	effDummy                   = 3
	effTeleportUnits           = 5  // SPELL_EFFECT_TELEPORT_UNITS (SharedDefines.h:816)
	effTeleportUnitsFaceCaster = 43 // SPELL_EFFECT_TELEPORT_UNITS_FACE_CASTER (SharedDefines.h:854)
	effApplyAura               = 6
	effQuestComplete           = 16 // SPELL_EFFECT_QUEST_COMPLETE (SharedDefines.h:827)
	effHealthLeech             = 9
	effHeal                    = 10 // SPELL_EFFECT_HEAL (SharedDefines.h:821)
	effLearnSpell              = 36 // SPELL_EFFECT_LEARN_SPELL (SharedDefines.h:847)
	effDispel                  = 38 // SPELL_EFFECT_DISPEL (SharedDefines.h:849)
	effSummon                  = 28
	effPersistentAreaAura      = 27 // SPELL_EFFECT_PERSISTENT_AREA_AURA (SharedDefines.h)
	effEnergize                = 30
	effWeaponPercentDamage     = 31 // SPELL_EFFECT_WEAPON_PERCENT_DAMAGE (SharedDefines.h:843)
	effTriggerMiss             = 32
	effSendEvent               = 61
	effThreat                  = 63 // SPELL_EFFECT_THREAT (SharedDefines.h)
	effTriggerSpell            = 64
	effScriptEffect            = 77
	effActivateObj             = 86
	effCharge                  = 96
	effPowerBurn               = 62  // SPELL_EFFECT_POWER_BURN (SharedDefines.h:873)
	effSkinning                = 95  // SPELL_EFFECT_SKINNING (SharedDefines.h:906)
	effSanctuary               = 79  // SPELL_EFFECT_SANCTUARY (SharedDefines.h:890)
	effSkill                   = 118 // SPELL_EFFECT_SKILL (SharedDefines.h:929)
	effKnockBack               = 98
	effAttackMe                = 114 // SPELL_EFFECT_ATTACK_ME (SharedDefines.h:925)
	effPullTowards             = 124 // SPELL_EFFECT_PULL_TOWARDS (SharedDefines.h:935)
	effKillCredit2             = 134 // SPELL_EFFECT_KILL_CREDIT2 (SharedDefines.h:945)
	effWeaponDamage            = 58
	effNormalizedWeaponDmg     = 121 // SPELL_EFFECT_NORMALIZED_WEAPON_DMG (SharedDefines.h:932)
	effHealMaxHealth           = 67  // SPELL_EFFECT_HEAL_MAX_HEALTH
	effInterruptCast           = 68  // SPELL_EFFECT_INTERRUPT_CAST (SharedDefines.h:879)
	effSummonObjectWild        = 76  // SPELL_EFFECT_SUMMON_OBJECT_WILD (SharedDefines.h:887)
	effDispelMechanic          = 108 // SPELL_EFFECT_DISPEL_MECHANIC (SharedDefines.h:919)
	effForceCast               = 140 // SPELL_EFFECT_FORCE_CAST (SharedDefines.h:951)
	effHealPct                 = 136 // SPELL_EFFECT_HEAL_PCT (SharedDefines.h:947)
	effEnergizePct             = 137 // SPELL_EFFECT_ENERGIZE_PCT (SharedDefines.h:948)
	effApplyAreaAuraOwner      = 143 // SPELL_EFFECT_APPLY_AREA_AURA_OWNER (SharedDefines.h:954)
	effApplyAreaAuraParty      = 35  // SPELL_EFFECT_APPLY_AREA_AURA_PARTY (SharedDefines.h:846)
	effApplyAreaAuraEnemy      = 129 // SPELL_EFFECT_APPLY_AREA_AURA_ENEMY (SharedDefines.h:940)
	effApplyAreaAuraFriend     = 128 // SPELL_EFFECT_APPLY_AREA_AURA_FRIEND (SharedDefines.h:939)
	effCreateItem              = 24  // SPELL_EFFECT_CREATE_ITEM (SharedDefines.h:835)
	effAddExtraAttacks         = 19  // SPELL_EFFECT_ADD_EXTRA_ATTACKS (SharedDefines.h:830)
	effSummonPet               = 56  // SPELL_EFFECT_SUMMON_PET (SharedDefines.h:867)
	effSpawn                   = 46  // SPELL_EFFECT_SPAWN (SharedDefines.h:857; clientside-only, spells.go:19747)
	auraPeriodicDmg            = 3
	auraDummy                  = 4
	auraModConfuse             = 5 // SPELL_AURA_MOD_CONFUSE (SpellAuraDefines.h:85)
	auraModCharm               = 6 // SPELL_AURA_MOD_CHARM (SpellAuraDefines.h:86)
	auraStun                   = 12
	auraModTaunt               = 11  // SPELL_AURA_MOD_TAUNT (SpellAuraDefines.h:91)
	auraMechanicImmunityMask   = 147 // SPELL_AURA_MECHANIC_IMMUNITY_MASK (SpellAuraDefines.h:227)
	auraModDamageTaken         = 14  // SPELL_AURA_MOD_DAMAGE_TAKEN (SpellAuraDefines.h:94)
	auraDamageShield           = 15  // SPELL_AURA_DAMAGE_SHIELD (SpellAuraDefines.h:95)
	auraReflectSpells          = 28  // SPELL_AURA_REFLECT_SPELLS (SpellAuraDefines.h:108)
	auraModPercentStat         = 80  // SPELL_AURA_MOD_PERCENT_STAT (SpellAuraDefines.h:160)
	auraSchoolAbsorb           = 69  // SPELL_AURA_SCHOOL_ABSORB (SpellAuraDefines.h:149)
	auraModDamageDone          = 13  // SPELL_AURA_MOD_DAMAGE_DONE (SpellAuraDefines.h:93) — NOT auraModDmgDone=79 (that is MOD_DAMAGE_PERCENT_DONE, SpellAuraDefines.h:159; pre-existing misnomer, left alone)
	auraModInvisibility        = 18  // SPELL_AURA_MOD_INVISIBILITY
	auraObsModHealth           = 20  // SPELL_AURA_OBS_MOD_HEALTH (SpellAuraDefines.h:100)
	auraObsModPower            = 21  // SPELL_AURA_OBS_MOD_POWER (SpellAuraDefines.h:101)
	auraModResistance          = 22  // SPELL_AURA_MOD_RESISTANCE (SpellAuraDefines.h:102)
	auraPeriodicTrig           = 23
	auraProcTriggerSpell       = 42 // SPELL_AURA_PROC_TRIGGER_SPELL (SpellAuraDefines.h:122)
	auraProcTriggerDmg         = 43 // SPELL_AURA_PROC_TRIGGER_DAMAGE (SpellAuraDefines.h:123)
	auraRoot                   = 26
	auraSilence                = 27
	auraModStat                = 29  // SPELL_AURA_MOD_STAT (SpellAuraDefines.h:109)
	auraModIncreaseSpeed       = 31  // SPELL_AURA_MOD_INCREASE_SPEED (SpellAuraDefines.h)
	auraModSpeedAlways         = 129 // SPELL_AURA_MOD_SPEED_ALWAYS (SpellAuraDefines.h:209)
	auraDecSpeed               = 33
	auraModIncreaseHealth      = 34 // SPELL_AURA_MOD_INCREASE_HEALTH (SpellAuraDefines.h:114)
	auraEffImmunity            = 37
	auraTransform              = 56
	auraMechanicImmunity       = 77 // SPELL_AURA_MECHANIC_IMMUNITY (SpellAuraDefines.h:157)
	auraModScale               = 61
	auraSchoolImmunity         = 39  // SPELL_AURA_SCHOOL_IMMUNITY (SpellAuraDefines.h:119)
	auraModCastingSpeed        = 65  // SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (SpellAuraDefines.h:145)
	auraDetectAmore            = 170 // SPELL_AURA_DETECT_AMORE (SpellAuraDefines.h:250)
	auraModStalked             = 68  // SPELL_AURA_MOD_STALKED (SpellAuraDefines.h:148)
	auraModDisarm              = 67  // SPELL_AURA_MOD_DISARM (SpellAuraDefines.h:147)
	auraModDmgDone             = 79
	auraModRegen               = 84 // SPELL_AURA_MOD_REGEN (SpellAuraDefines.h:164)
	auraModDmgTaken            = 87
	auraPeriodicDmgPct         = 89 // SPELL_AURA_PERIODIC_DAMAGE_PERCENT (SpellAuraDefines.h:169)
	auraModHitChance           = 54 // SPELL_AURA_MOD_HIT_CHANCE (SpellAuraDefines.h:134)
	auraPeriodicLeech          = 53 // SPELL_AURA_PERIODIC_LEECH (SpellAuraDefines.h:133)
	auraPeriodicManaLeech      = 64 // SPELL_AURA_PERIODIC_MANA_LEECH (SpellAuraDefines.h:144)
	auraModResistPct           = 101
	auraModAttackPower         = 99  // SPELL_AURA_MOD_ATTACK_POWER (SpellAuraDefines.h:179)
	auraFeatherFall            = 105 // SPELL_AURA_FEATHER_FALL (SpellAuraDefines.h:185)
	auraModMeleeHaste          = 138
	auraModCastSpeedNotStack   = 65  // SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (SpellAuraDefines.h:145)
	auraForceReaction          = 139 // SPELL_AURA_FORCE_REACTION (SpellAuraDefines.h:219)
	auraModHealingPct          = 118 // SPELL_AURA_MOD_HEALING_PCT (SpellAuraDefines.h:198)
	auraMeleeSlow              = 193 // SPELL_AURA_MELEE_SLOW (SpellAuraDefines.h:273)
	auraRangedApAttackerBonus  = 127 // SPELL_AURA_RANGED_ATTACK_POWER_ATTACKER_BONUS (SpellAuraDefines.h:207)
	auraMeleeApAttackerBonus   = 165 // SPELL_AURA_MELEE_ATTACK_POWER_ATTACKER_BONUS (SpellAuraDefines.h:245)
	auraModTotalStatPct        = 137 // SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE (SpellAuraDefines.h:217)
	auraModDetaunt             = 221 // SPELL_AURA_MOD_DETAUNT (SpellAuraDefines.h:301)
	auraRaidProcFromCharge     = 223 // SPELL_AURA_RAID_PROC_FROM_CHARGE (SpellAuraDefines.h:303)
	auraPeriodicDummy          = 226
	auraPeriodicTrigWithValue  = 227 // SPELL_AURA_PERIODIC_TRIGGER_SPELL_WITH_VALUE (SpellAuraDefines.h:307)
	auraControlVehicle         = 236 // SPELL_AURA_CONTROL_VEHICLE (SpellAuraDefines.h:301)
	auraCloneCaster            = 247 // SPELL_AURA_CLONE_CASTER (SpellAuraDefines.h)
	auraPhase                  = 261 // SPELL_AURA_PHASE (SpellAuraDefines.h)
	auraAllowOnlyAbility       = 263 // SPELL_AURA_ALLOW_ONLY_ABILITY (SpellAuraDefines.h:343; wago.tools names it DISABLE_CASTING_EXCEPT_ABILITIES)
	auraInitializeImages       = 279 // SPELL_AURA_INITIALIZE_IMAGES (SpellAuraDefines.h)
	auraLinked                 = 284
	auraPreventRegen           = 294
	auraReducePushback         = 149 // SPELL_AURA_REDUCE_PUSHBACK (SpellAuraDefines.h:229)
	auraModAoeDamageAvoidance  = 229 // SPELL_AURA_MOD_AOE_DAMAGE_AVOIDANCE (SpellAuraDefines.h:309)
	auraModFear                = 7   // SPELL_AURA_MOD_FEAR (SpellAuraDefines.h:87)
	auraModDodgePercent        = 49  // SPELL_AURA_MOD_DODGE_PERCENT (SpellAuraDefines.h:129)
	auraModPowerCostSchoolPct  = 72  // SPELL_AURA_MOD_POWER_COST_SCHOOL_PCT (SpellAuraDefines.h:152)
	auraModMeleeRangedHaste    = 192 // SPELL_AURA_MOD_MELEE_RANGED_HASTE (SpellAuraDefines.h:272)
	schoolNone                 = 0   // SPELL_SCHOOL_MASK_NONE (SharedDefines.h:323)
	schoolPhysical             = 1
	schoolHoly                 = 2
	schoolFire                 = 4
	schoolNature               = 8
	schoolFrost                = 16
	schoolFrostfire            = 20 // Frost (16) + Fire (4), wago SpellMisc 47610
	schoolSpellfire            = 68 // Arcane (64) + Fire (4), wago SpellMisc 62997
	schoolShadow               = 32
	schoolArcane               = 64
	targetCaster               = 1
	targetEnemy                = 6
	targetSrcAreaEnemy         = 15
	targetDestDB               = 17  // TARGET_DEST_DB
	targetDestCaster           = 18  // TARGET_DEST_CASTER (SharedDefines.h:1455)
	targetUnitCasterAreaParty  = 20  // TARGET_UNIT_CASTER_AREA_PARTY (SharedDefines.h:1456)
	targetUnitTargetAlly       = 21  // TARGET_UNIT_TARGET_ALLY (SharedDefines.h:1457)
	targetUnitDestAreaEnemy    = 16  // TARGET_UNIT_DEST_AREA_ENEMY
	targetDestCasterBackRight  = 42  // TARGET_DEST_CASTER_BACK_RIGHT (SharedDefines.h:1478)
	targetDestCasterFrontRight = 41  // TARGET_DEST_CASTER_FRONT_RIGHT (SharedDefines.h:1477)
	targetDestCasterSummon     = 32  // TARGET_DEST_CASTER_SUMMON
	targetDestCasterRadius     = 73  // TARGET_DEST_CASTER_RADIUS
	targetUnitTargetAny        = 25  // TARGET_UNIT_TARGET_ANY (SharedDefines.h:1461)
	targetUnitConeEnemy        = 24  // TARGET_UNIT_CONE_ENEMY_24 (SharedDefines.h:1460)
	targetDestTargetAny        = 63  // TARGET_DEST_TARGET_ANY (SharedDefines.h:1499)
	targetDestDest             = 87  // TARGET_DEST_DEST (SharedDefines.h:1523)
	targetDestTargetEnemy      = 53  // TARGET_DEST_TARGET_ENEMY
	targetSrcCaster            = 22  // TARGET_SRC_CASTER (SharedDefines.h:1459)
	targetDestDynobjEnemy      = 28  // TARGET_DEST_DYNOBJ_ENEMY (SharedDefines.h:1464)
	targetDestChannelTarget    = 76  // TARGET_DEST_CHANNEL_TARGET (SpellInfo.cpp:296)
	targetUnitConeEnemy104     = 104 // TARGET_UNIT_CONE_ENEMY_104 (SharedDefines.h:1540)
	targetUnitConeEntry        = 60  // TARGET_UNIT_CONE_ENTRY (SharedDefines.h:1496)
	targetUnitMaster           = 27  // TARGET_UNIT_MASTER (SharedDefines.h:1463)
	targetUnitSrcAreaAlly      = 30  // TARGET_UNIT_SRC_AREA_ALLY (SharedDefines.h:1467)
	targetUnitSrcAreaEntry     = 7   // TARGET_UNIT_SRC_AREA_ENTRY (SharedDefines.h:1448)
	targetUnitDestAreaEntry    = 8   // TARGET_UNIT_DEST_AREA_ENTRY (SharedDefines.h:1449)
	targetUnitNearbyEntry      = 38  // TARGET_UNIT_NEARBY_ENTRY (SharedDefines.h:1474)
	targetDestNearbyEntry      = 46  // TARGET_DEST_NEARBY_ENTRY (SharedDefines.h:1482)
	targetGONearbyEntry        = 40
	auraAddPctModifier         = 108 // SPELL_AURA_ADD_PCT_MODIFIER (SpellAuraDefines.h:188)
	spellModDamage             = 0   // SPELLMOD_DAMAGE (SpellMgr.h)
	spellModEffect1            = 3   // SPELLMOD_EFFECT1 (SpellMgr.h)
	familyShaman               = 11  // SPELLFAMILY_SHAMAN (SharedDefines.h)
	familyMage                 = 3   // SPELLFAMILY_MAGE (SpellClassOptions SpellClassSet 3, wago 3.4.0.43659)
	familyHunter               = 9   // SPELLFAMILY_HUNTER (SharedDefines.h:3590)
	familyDeathKnight          = 15  // SPELLFAMILY_DEATHKNIGHT (SharedDefines.h:3596)
	familyWarlock              = 5   // SPELLFAMILY_WARLOCK (SharedDefines.h:3586)
	familyWarrior              = 4   // SPELLFAMILY_WARRIOR (SharedDefines.h:3585)
)

type effect struct {
	effect         uint32
	dieSides       int32
	basePoints     int32
	mechanic       uint32
	targetA        uint32
	targetB        uint32
	radiusIdx      uint32
	aura           uint32
	auraPeriod     uint32
	misc           int32
	miscB          int32
	trigger        uint32
	itemType       uint32    // EffectItemType (store.go field 107+i)
	chainTargets   uint32    // EffectChainTargets (store.go field 104+i)
	chainAmplitude float32   // EffectChainAmplitude (store.go fields 216-218)
	mask           [3]uint32 // EffectSpellClassMask (store.go fields 122-130)
}

type spell struct {
	id          uint32
	dispel      uint32
	mechanic    uint32
	castTimeIdx uint32
	durIdx      uint32
	rangeIdx    uint32
	maxTargets  uint32
	school      uint32
	manaCost    uint32    // Spell.dbc field 42 (store.go:1070)
	manaCostPct uint32    // Spell.dbc field 204 (store.go:1073)
	family      uint32    // SpellFamilyName (store.go field 208)
	familyFlags [3]uint32 // SpellFamilyFlags (store.go fields 209-211)
	effects     [3]effect
}

func (s spell) row() []uint32 {
	r := make([]uint32, 234)
	r[0] = s.id
	r[2] = s.dispel
	r[3] = s.mechanic
	r[28] = s.castTimeIdx
	r[40] = s.durIdx
	r[46] = s.rangeIdx
	r[208] = s.family
	for i, f := range s.familyFlags {
		r[209+i] = f
	}
	r[212] = s.maxTargets
	r[225] = s.school
	r[42] = s.manaCost
	r[204] = s.manaCostPct
	for i, e := range s.effects {
		if e.effect == 0 {
			continue
		}
		r[71+i] = e.effect
		r[74+i] = uint32(e.dieSides)
		r[80+i] = uint32(e.basePoints)
		r[83+i] = e.mechanic
		r[86+i] = e.targetA
		r[89+i] = e.targetB
		r[92+i] = e.radiusIdx
		r[95+i] = e.aura
		r[98+i] = e.auraPeriod
		r[110+i] = uint32(e.misc)
		r[113+i] = uint32(e.miscB)
		r[116+i] = e.trigger
		r[107+i] = e.itemType
		r[104+i] = e.chainTargets
		r[216+i] = f32(e.chainAmplitude)
		for j := range e.mask {
			r[122+3*i+j] = e.mask[j]
		}
	}
	return r
}

var spells = []spell{
	// Mar'li hatch chain (boss_marli.cpp; 24082/24083 Wowhead WotLK,
	// 24081 Wowhead Classic; 24083 MiscValue 8, not 1 — Unit B).
	{id: 24081, durIdx: 90006, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 1, targetA: targetCaster, radiusIdx: 91009, misc: 15041, miscB: 64},
	}},
	{id: 24082, rangeIdx: 93008, school: schoolShadow, effects: [3]effect{
		{effect: effActivateObj, basePoints: 1, targetA: targetGONearbyEntry, misc: 8},
	}},
	{id: 24083, rangeIdx: 93008, maxTargets: 4, school: schoolShadow, effects: [3]effect{
		{effect: effActivateObj, basePoints: 1, targetA: targetGONearbyEntry, radiusIdx: 1, misc: 8},
	}},
	// Mar'li fight casts (boss_marli.cpp DoCast/DoCastVictim lines).
	{id: 22911, durIdx: 90001, rangeIdx: 93003, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, basePoints: 1, targetA: targetEnemy},
		{effect: effWeaponDamage, basePoints: 75, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraStun, basePoints: 1, targetA: targetEnemy},
	}},
	{id: 24686, durIdx: 90002, rangeIdx: 93002, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 1, targetA: targetEnemy},
	}},
	{id: 24110, durIdx: 90003, rangeIdx: 93005, school: schoolPhysical, mechanic: 7, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, basePoints: 1, targetA: targetEnemy, radiusIdx: 91002},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -100, targetA: targetEnemy, radiusIdx: 91002},
		{effect: effApplyAura, aura: auraSilence, basePoints: 1, targetA: targetEnemy, radiusIdx: 91002},
	}},
	{id: 24099, durIdx: 90004, rangeIdx: 93007, school: schoolNature, dispel: 4, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 375, targetA: targetEnemy, radiusIdx: 91004},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 289, auraPeriod: 2000, targetA: targetEnemy, radiusIdx: 91004},
	}},
	// 24084 TRANSFORM MiscValue 0: transform target entry unknown, Go
	// does not consume it (flagged unknown, gates no arm).
	{id: 24084, rangeIdx: 93001, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, basePoints: 1, targetA: targetCaster},
	}},
	{id: 24312, durIdx: 90008, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 10, misc: 1, targetA: targetCaster},
		{effect: effApplyAura, aura: auraModScale, basePoints: 20, targetA: targetCaster},
	}},
	// Vezax set. TargetA 15 on 62692/62659/62661/63420/49838 and 6 on
	// 63322 are INFERRED from cast usage + target-enum semantics.
	// 62692 eff2 EFFECT_IMMUNITY MiscValue 0: immunized effect
	// unidentified (flagged unknown, gates no arm).
	{id: 62692, rangeIdx: 93001, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPreventRegen, misc: 1, targetA: targetSrcAreaEnemy, radiusIdx: 91007},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -20, targetA: targetSrcAreaEnemy, radiusIdx: 91007},
		{effect: effApplyAura, aura: auraEffImmunity, targetA: targetSrcAreaEnemy, radiusIdx: 91007},
	}},
	{id: 62662, durIdx: 90004, rangeIdx: 93001, castTimeIdx: 92002, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -55, targetA: targetCaster},
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 100, misc: 1, targetA: targetCaster},
		{effect: effApplyAura, aura: auraModScale, basePoints: 35, targetA: targetCaster},
	}},
	{id: 62660, rangeIdx: 93007, school: schoolShadow, effects: [3]effect{
		{effect: effTriggerMiss, trigger: 62659, targetA: targetEnemy},
		{effect: effTriggerMiss, trigger: 62659, targetA: targetEnemy},
	}},
	{id: 62659, rangeIdx: 93008, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 11310, targetA: targetSrcAreaEnemy, radiusIdx: 91002},
		{effect: effKnockBack, basePoints: 175, misc: 200, targetA: targetSrcAreaEnemy, radiusIdx: 91002},
	}},
	{id: 62661, durIdx: 90004, rangeIdx: 93001, castTimeIdx: 92001, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 13875, targetA: targetSrcAreaEnemy, radiusIdx: 91008},
		{effect: effApplyAura, aura: auraModResistPct, basePoints: -75, misc: 1, targetA: targetSrcAreaEnemy, radiusIdx: 91008},
	}},
	{id: 63276, durIdx: 90004, rangeIdx: 93007, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDummy, basePoints: 5000, auraPeriod: 1000, targetA: targetEnemy},
	}},
	// 26662 eff2 LINKED TriggerSpell 0: no linked spell identified
	// (flagged unknown, gates no arm).
	{id: 26662, durIdx: 90006, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 500, misc: 127, targetA: targetCaster},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 150, targetA: targetCaster},
		{effect: effApplyAura, aura: auraLinked, targetA: targetCaster},
	}},
	// 63081/63145 MiscValueB 64 (SummonProperties row 64 is REAL now,
	// wago.tools 3.4.0.43659, 2026-11-15 — see SummonProperties note);
	// MiscValue NPC entries Wowhead-verified.
	{id: 63081, rangeIdx: 93006, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 1, targetA: targetCaster, radiusIdx: 91005, misc: 33488, miscB: 64},
	}},
	{id: 63364, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: -99, misc: 127, targetA: targetCaster},
		{effect: effSendEvent, basePoints: 1, misc: 9735, targetA: targetCaster},
	}},
	{id: 63145, rangeIdx: 93001, castTimeIdx: 92001, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 1, targetA: targetCaster, misc: 33524, miscB: 64},
	}},
	{id: 63319, rangeIdx: 93001, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 1, targetA: targetCaster},
	}},
	{id: 63420, durIdx: 90007, rangeIdx: 93001, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 750, targetA: targetSrcAreaEnemy, radiusIdx: 91007},
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: 10, misc: 32, targetA: targetSrcAreaEnemy, radiusIdx: 91007},
	}},
	{id: 63323, durIdx: 90005, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, trigger: 63322, auraPeriod: 2000, targetA: targetCaster},
	}},
	// 63278 eff0 is SPELL_EFFECT_DUMMY (3), not an aura — bare "DUMMY"
	// in the Unit B spec, unlike the explicit APPLY_AURA rows.
	{id: 63278, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, basePoints: 1, targetA: targetEnemy},
		{effect: effHealthLeech, basePoints: 1, targetA: targetEnemy, radiusIdx: 91003},
	}},
	{id: 63322, durIdx: 90009, rangeIdx: 93001, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, misc: 127, basePoints: 50, targetA: targetEnemy, radiusIdx: 91001},
	}},
	{id: 63337, rangeIdx: 93004, school: schoolShadow, effects: [3]effect{
		{effect: effEnergize, basePoints: 1, targetA: targetEnemy},
	}},
	{id: 63338, rangeIdx: 93004, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1, targetA: targetEnemy},
	}},
	// Oculus set. 49838 eff2 DUMMY BP 51162: script-parameter meaning
	// unknown (flagged unknown, gates no arm).
	{id: 49838, durIdx: 90004, rangeIdx: 93001, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, misc: 127, basePoints: 1, targetA: targetSrcAreaEnemy, radiusIdx: 91006},
		{effect: effTriggerSpell, trigger: 30787, targetA: targetSrcAreaEnemy},
		{effect: effDummy, basePoints: 51162, targetA: targetSrcAreaEnemy},
	}},
	{id: 49836, durIdx: 90005, rangeIdx: 93008, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, misc: 127, basePoints: 6525, targetA: targetEnemy},
	}},
	{id: 30787, durIdx: 90010, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	// 68415 Corrupted Rage (boss_general_vezax.cpp:68, :244-251 —
	// CheckShamanisticRage casts it on shamans who know Shamanistic
	// Rage 30823; Lua leg live, boss_general_vezax.lua:448-461).
	// REAL CLIENT DATA (wago.tools DB2, build 3.4.0.43659, read
	// 2026-11-13 — 68415 added in Classic 3.4.0 per Wowhead, so
	// this IS the spell's client data; 3.3.5.12340 is unhosted
	// anywhere since wow.tools retired May 2025): SpellMisc row
	// 621919: DurationIndex 21, RangeIndex 13, SchoolMask 0x20
	// (Shadow), CastingTimeIndex 1 (instant). SpellDuration ID 21:
	// Duration -1, MaxDuration -1 (permanent — kills the 15s
	// hypothesis; aura is removed by script, not by expiry).
	// SpellClassOptions row 66466: family 11 (Shaman), mask 0.
	// SpellEffect rows 981024/981025: both APPLY_AURA (6),
	// aura 108 ADD_PCT_MODIFIER, DieSides 1, target 6, BP -91 /
	// 99 (DBC stores value-1; CalcValue +1 -> -90 / +100, matching
	// the Wowhead tooltip). eff0 MiscValue 0 SPELLMOD_DAMAGE,
	// mask [448, 524288, 16] = [0x1C0, 0x80000, 0x10]: LHW 0x80
	// + Chain Heal 0x100 + Healing Wave 0x40 (word 0, per
	// spell_shaman.cpp), Earthliving 0x80000 (word 1 — the 6
	// Earthliving ranks read [0, 524288, 0] on the same build),
	// Riptide 0x10 (word 2, per spell_shaman.cpp:356). eff1
	// MiscValue 3 SPELLMOD_EFFECT1, mask [0, 0, 0] — EMPTY, not
	// the shared heal mask: C++ IsAffected skips the flag check
	// on an empty mask (SpellInfo.cpp IsAffected), and Go
	// spellAffectedBySpellFamilyMask does the same, so eff1
	// applies +100% to effect 0 of EVERY Shaman spell —
	// Shamanistic Rage 30823's mana-regen effect included.
	// DELIBERATE DEVIATIONS from the 3.4.0 rows (behavior, not
	// data): targetA stays 1 (caster/self) — the C++ leg casts
	// with an explicit self target; real implicit target 6 would
	// re-resolve through Go's implicit-target legs. RangeIndex
	// stays synthetic 93008 (50000 yd, same value as real 13);
	// cast time stays default 0 (instant); attributes unwritten.
	{id: 68415, durIdx: 21, rangeIdx: 93008, school: schoolShadow, family: familyShaman, effects: [3]effect{
		{effect: effApplyAura, aura: auraAddPctModifier, dieSides: 1, basePoints: -91, misc: spellModDamage, targetA: targetCaster, mask: [3]uint32{0x1C0, 0x80000, 0x10}},
		{effect: effApplyAura, aura: auraAddPctModifier, dieSides: 1, basePoints: 99, misc: spellModEffect1, targetA: targetCaster, mask: [3]uint32{0, 0, 0}},
	}},

	// MEMORY_* summon spells (boss_argent_challenge.cpp memorySpellId
	// order, :615-641) — wago.tools 3.4.0.43659 SpellMisc/SpellEffect/
	// SpellDuration rows, 2026-11-13 14:55 verdict: all 25 share
	// DurationIndex 6, RangeIndex 13, SchoolMask 0x1, CastingTimeIndex
	// 1 (instant); effect 28 SUMMON, BP 0, DieSides 1, MiscValue =
	// memory entry, MiscValueB 64, TargetA 18 (TARGET_DEST_CASTER),
	// TargetB 0, RadiusIndex 0. RangeIndex stays synthetic 93008
	// (50000 yd, same value as real 13, 68415 precedent); cast time
	// stays default 0 (instant = real 1); family stays 0 (no
	// SpellClassOptions row read). 66545 Summon Memory is seeded
	// below (RadiusIndex 28 = 50000/0/50000, real SpellRadius row).
	{id: 66715, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35052, miscB: 64, targetA: targetDestCaster}}},
	{id: 66704, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35041, miscB: 64, targetA: targetDestCaster}}},
	{id: 66697, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35033, miscB: 64, targetA: targetDestCaster}}},
	{id: 66709, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35046, miscB: 64, targetA: targetDestCaster}}},
	{id: 66706, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35043, miscB: 64, targetA: targetDestCaster}}},
	{id: 66710, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35047, miscB: 64, targetA: targetDestCaster}}},
	{id: 66707, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35044, miscB: 64, targetA: targetDestCaster}}},
	{id: 66702, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35039, miscB: 64, targetA: targetDestCaster}}},
	{id: 66698, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35034, miscB: 64, targetA: targetDestCaster}}},
	{id: 66712, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35049, miscB: 64, targetA: targetDestCaster}}},
	{id: 66694, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35030, miscB: 64, targetA: targetDestCaster}}},
	{id: 66543, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 34942, miscB: 64, targetA: targetDestCaster}}},
	{id: 66713, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35050, miscB: 64, targetA: targetDestCaster}}},
	{id: 66705, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35042, miscB: 64, targetA: targetDestCaster}}},
	{id: 66708, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35045, miscB: 64, targetA: targetDestCaster}}},
	{id: 66700, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35037, miscB: 64, targetA: targetDestCaster}}},
	{id: 66695, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35031, miscB: 64, targetA: targetDestCaster}}},
	{id: 66701, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35038, miscB: 64, targetA: targetDestCaster}}},
	{id: 66692, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35029, miscB: 64, targetA: targetDestCaster}}},
	{id: 66711, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35048, miscB: 64, targetA: targetDestCaster}}},
	{id: 66696, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35032, miscB: 64, targetA: targetDestCaster}}},
	{id: 66691, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35028, miscB: 64, targetA: targetDestCaster}}},
	{id: 66703, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35040, miscB: 64, targetA: targetDestCaster}}},
	{id: 66699, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35036, miscB: 64, targetA: targetDestCaster}}},
	{id: 66714, durIdx: 6, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 35051, miscB: 64, targetA: targetDestCaster}}},

	// 66545 Summon Memory (boss_argent_challenge.cpp:77, :646-684) —
	// wago.tools 3.4.0.43659 SpellMisc/SpellEffect rows, 2026-11-13
	// 14:55 verdict + SpellRadius row 28 read 15:52: DurationIndex 0
	// (no SpellDuration row), RangeIndex 1, SchoolMask 0x1,
	// CastingTimeIndex 1 (instant); effect 77 SCRIPT_EFFECT, BP 0,
	// DieSides 0, Misc 0/0, TargetA 22 (TARGET_SRC_CASTER), TargetB
	// 15 (TARGET_UNIT_SRC_AREA_ENEMY), RadiusIndex 28 (real SpellRadius
	// 28 = 50000/0/50000, seeded below). RangeIndex stays synthetic
	// 93001 (0 yd = real 1's value); cast time stays default 0
	// (instant = real 1). The seed row() now writes targetB (field
	// 89+i, store.go:1168) — Go's area legs read ImplicitTargetB
	// (creature_area_targets.go:63), so without it the ev7 filter
	// would see no area-enemy list.
	{id: 66545, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effScriptEffect, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 28}}},

	// 72585 Soul Missile (icecrown_citadel.cpp:91, :1479-1492) —
	// wago.tools 3.4.0.43659 SpellMisc/SpellEffect rows, 2026-11-13
	// 16:15 verdict: DurationIndex 0 (no SpellDuration row),
	// RangeIndex 1, SchoolMask 0x60 (shadow + arcane),
	// CastingTimeIndex 1 (instant); single effect 3 DUMMY, BP 0,
	// DieSides 0, Misc 0/0, TargetA 18 (TARGET_DEST_CASTER),
	// TargetB 0 (NONE), RadiusIndex 0 (no SpellRadius row).
	// RangeIndex stays synthetic 93001 (0 yd = real 1's value);
	// cast time stays default 0 (instant = real 1). The C++
	// RelocateDest +200z leg (spell_icc_soul_missile) has no Go
	// handler — the visual dest stays at caster pos.
	{id: 72585, rangeIdx: 93001, school: schoolShadow + schoolArcane, effects: [3]effect{{effect: effDummy, targetA: targetDestCaster}}},

	// 50415 Death Spell (oculus.h:83; boss_urom.cpp:284-289 JustDied
	// DoCastSelf + boss_varos.cpp:160-165 JustDied DoCast, both
	// triggered) — wago.tools 3.4.0.43659 Spell/SpellEffect/SpellMisc
	// rows, 2026-11-13 22:11 verdict: single effect 28 SUMMON,
	// MiscValue 28012 (Amber Drake entry), MiscValueB 64
	// (SummonProperties 64, seeded REAL row, wago 2026-11-15), TargetA 42
	// (TARGET_DEST_CASTER_BACK_RIGHT), TargetB 0 (NONE), RadiusIndex
	// 36, DieSides 1, BasePoints 0; DurationIndex 21 (infinite,
	// seeded row), RangeIndex 1, SchoolMask 0x1 (physical),
	// CastingTimeIndex 1 (instant); Attributes_0 0x100 not encoded
	// (seed has no attributes field). RangeIndex stays synthetic
	// 93001 (0 yd = real 1's value); cast time stays default 0
	// (instant = real 1). The summon needs a creature_template
	// 28012 row in the deployment world DB (summonTempCreature
	// returns nil without one).
	{id: 50415, durIdx: 21, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, targetA: targetDestCasterBackRight, radiusIdx: 36, misc: 28012, miscB: 64}}},

	// 52267 Despawn Horse (chapter1.cpp:706; npc_dark_rider_of_acherus
	// EVENT_DESPAWN_HORSE DoCast, triggered) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-13 22:35 verdict: single
	// effect 77 SCRIPT_EFFECT, TargetA 25 (TARGET_UNIT_TARGET_ANY),
	// TargetB 63 (TARGET_DEST_TARGET_ANY), RadiusIndex 0, DieSides 0,
	// BasePoints 0, Misc 0/0; DurationIndex 0 (no SpellDuration row),
	// RangeIndex 13 (Anywhere - Unlimited, 0-50000 yd), SchoolMask 0x1
	// (physical), CastingTimeIndex 1 (instant); Attributes_0 0x20000180
	// not encoded (seed has no attributes field). RangeIndex stays
	// synthetic 93008 (0-50000 yd = real 13's value); cast time stays
	// default 0 (instant = real 1). Effect 77 is what the
	// spell_gen_despawn_self handler keys on (spell_generic.lua); the
	// TargetA 25 / TargetB 63 legs are live in Go (chain_targets.go:360,
	// target_dest_targets.go:14).
	{id: 52267, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effScriptEffect, targetA: targetUnitTargetAny, targetB: targetDestTargetAny}}},

	// Mimiron seat spells 63313 SEAT_2 / 63316 SEAT_5 / 63344 SEAT_6 /
	// 63345 SEAT_7 (boss_mimiron.cpp activation chains + JustEngagedWith
	// SEAT_6 + INTRO_2 SEAT_7) — wago.tools 3.4.0.47659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-13 22:51 verdict: two
	// effects each, both effect 6 APPLY_AURA; eff0 aura 236
	// CONTROL_VEHICLE, DieSides 1, BasePoints = seat-1 (1/4/5/6),
	// TargetA 25 (TARGET_UNIT_TARGET_ANY), TargetB 0, RadiusIndex 0;
	// eff1 aura 4 DUMMY, BP 0, TargetA 1 (TARGET_UNIT_CASTER);
	// DurationIndex 21 (infinite, seeded row), RangeIndex 152 (real
	// SpellRange 152 = 0-150 yd, seeded below), SchoolMask 0x40
	// (arcane), CastingTimeIndex 1 (instant); Attributes not encoded
	// (seed has no attributes field). Cast time stays default 0
	// (instant = real 1). Seeding wakes the Go creature-caster
	// CONTROL_VEHICLE boarding leg (spells.go:40344, seat = amount-1
	// = BP); the aura-leave exit half is still unbridged, so a boarded
	// caster stays seated when the aura leaves.
	{id: 63313, durIdx: 21, rangeIdx: 152, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, basePoints: 1, aura: auraControlVehicle, targetA: targetUnitTargetAny},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	{id: 63316, durIdx: 21, rangeIdx: 152, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, basePoints: 4, aura: auraControlVehicle, targetA: targetUnitTargetAny},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	{id: 63344, durIdx: 21, rangeIdx: 152, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, basePoints: 5, aura: auraControlVehicle, targetA: targetUnitTargetAny},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	{id: 63345, durIdx: 21, rangeIdx: 152, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, basePoints: 6, aura: auraControlVehicle, targetA: targetUnitTargetAny},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},

	// 59335 Explode Frost Wyrm (zone_icecrown.cpp:763-775; skytalon
	// SpellHit -> DoCast(me, 54690)) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 2 SCHOOL_DAMAGE, DieSides 1, BasePoints 499999,
	// TargetA 22 (TARGET_SRC_CASTER), TargetB 7
	// (TARGET_UNIT_SRC_AREA_ENTRY), RadiusIndex 14, Misc 0/0;
	// DurationIndex 0 (no SpellDuration row), RangeIndex 74
	// (Ranged Weapon, 0-30 yd, Flags 0x2, seeded real row below),
	// SchoolMask 0x1 (physical), CastingTimeIndex 1 (instant);
	// Attributes_0 0x100 not encoded (seed has no attributes field).
	// Cast time stays default 0 (instant = real 1). The
	// src-area-entry leg is live in Go (entry_area_targets.go:13,
	// creature_area_targets.go:344, spells.go:11518).
	{id: 59335, rangeIdx: 74, school: schoolPhysical, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1, basePoints: 499999, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 14}}},

	// 59319 Ride Frost Wyrm (zone_icecrown.cpp:763-775; skytalon
	// SpellHit -> DoCastAOE(59375) + FLY_AWAY 100ms) — wago.tools
	// 3.4.0.43659 Spell/SpellEffect/SpellMisc rows, 2026-11-14
	// verdict: single effect 6 APPLY_AURA, aura 236 CONTROL_VEHICLE,
	// DieSides 0, BasePoints 0, TargetA 25 (TARGET_UNIT_TARGET_ANY),
	// TargetB 0, RadiusIndex 0, Misc 0/0; DurationIndex 21 (infinite,
	// seeded row), RangeIndex 13 (Anywhere - Unlimited, 0-50000 yd),
	// SchoolMask 0x1 (physical), CastingTimeIndex 1 (instant).
	// RangeIndex stays synthetic 93008 (0-50000 yd = real 13's value);
	// cast time stays default 0 (instant = real 1). The boarding leg
	// is live in Go (spells.go:40344, seat = amount-1; BP 0 rides the
	// any-usable-seat fallback).
	{id: 59319, durIdx: 21, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effApplyAura, aura: auraControlVehicle, targetA: targetUnitTargetAny}}},

	// 54690 Cosmetic - Immolation (Whole Body) 6 Sec
	// (zone_icecrown.cpp:767-768; skytalon SpellHit 59335 ->
	// DoCast(me, 54690)) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 6 APPLY_AURA, aura 4 DUMMY, DieSides 1, BasePoints -1,
	// TargetA 1 (TARGET_UNIT_CASTER), TargetB 0, RadiusIndex 0, Misc
	// 0/0; DurationIndex 32 (6000 ms, seeded real row below),
	// RangeIndex 4 (Medium Range, 0-30 yd, seeded real row below),
	// SchoolMask 0x4 (fire), CastingTimeIndex 1 (instant). Cast time
	// stays default 0 (instant = real 1).
	{id: 54690, durIdx: 32, rangeIdx: 4, school: schoolFire, effects: [3]effect{{effect: effApplyAura, dieSides: 1, basePoints: -1, aura: auraDummy, targetA: targetCaster}}},

	// 59375 Ping Explosion Bunny (zone_icecrown.cpp:770-772; skytalon
	// SpellHit 59319 -> DoCastAOE(59375)) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 3 DUMMY, DieSides 0, BasePoints 0, TargetA 38
	// (TARGET_UNIT_NEARBY_ENTRY), TargetB 0, RadiusIndex 0, Misc 0/0;
	// DurationIndex 0 (no SpellDuration row), RangeIndex 3 (Twenty
	// yards, 0-20 yd), SchoolMask 0x1 (physical), CastingTimeIndex 1
	// (instant). RangeIndex stays synthetic 93005 (0-20 yd = real 3's
	// value); cast time stays default 0 (instant = real 1). Misc 0
	// means the nearby-entry leg carries no entry filter in the DBC
	// row (Go targetCheckEntry, chain_targets.go:356).
	{id: 59375, rangeIdx: 93005, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetUnitNearbyEntry}}},

	// 63049 Mounted Melee Victory (zone_icecrown.cpp:75-86; valiant
	// DamageTaken lethal + player -> self-cast kill credit) —
	// wago.tools 3.4.0.43659 Spell/SpellEffect/SpellMisc rows,
	// 2026-11-14 verdict: single effect 134 KILL_CREDIT2, DieSides 1,
	// BasePoints -1, TargetA 1 (TARGET_UNIT_CASTER), TargetB 0,
	// RadiusIndex 0, MiscValue 38595 (kill-credit creature entry),
	// MiscValueB 0; DurationIndex 0 (no SpellDuration row), RangeIndex
	// 13 (Anywhere - Unlimited, 0-50000 yd), SchoolMask 0x1
	// (physical), CastingTimeIndex 1 (instant). RangeIndex stays
	// synthetic 93008 (0-50000 yd = real 13's value); cast time stays
	// default 0 (instant = real 1). The group kill-credit leg is live
	// in Go (boss_ai.go:4057, creatureEffectKillCredit :8650,
	// MiscValue = credit entry).
	{id: 63049, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{{effect: effKillCredit2, dieSides: 1, basePoints: -1, targetA: targetCaster, misc: 38595}}},

	// 65096 Elevator Knockback (boss_mimiron.cpp:171,541; VX001 step 4
	// worldtrigger self-cast) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 6 APPLY_AURA, aura 23 PERIODIC_TRIGGER_SPELL, DieSides 1,
	// BasePoints -1, TargetA 1 (TARGET_UNIT_CASTER), TargetB 0,
	// RadiusIndex 0 (no SpellRadius row), AuraPeriod 2000,
	// TriggerSpell 65100; DurationIndex 32 (6000 ms, seeded real row),
	// RangeIndex 1 (Self Only, 0-0 yd = synthetic 93001's value),
	// SchoolMask 0x1 (physical), CastingTimeIndex 1 (instant, default
	// 0). Attributes_0 0x20000100 / Attributes_1 0x10000000 not encoded
	// (seed has no attributes field). Trigger spell 65100 seeded below
	// (verdict 2026-11-14) — the chain is live in code.
	// The periodic-trigger leg is live in Go (spells.go:42157).
	{id: 65096, durIdx: 32, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effApplyAura, dieSides: 1, basePoints: -1, aura: auraPeriodicTrig, auraPeriod: 2000, trigger: 65100, targetA: targetCaster}}},

	// 65100 Elevator Knockback (trigger of 65096's periodic aura;
	// boss_mimiron.cpp:171,541) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 98 KNOCK_BACK, DieSides 1, BasePoints 99, TargetA 22
	// (TARGET_SRC_CASTER), TargetB 15 (TARGET_UNIT_SRC_AREA_ENEMY),
	// RadiusIndex 13 (10 yd, seeded real row below), MiscValue 200,
	// MiscValueB 0; DurationIndex 0 (no SpellDuration row, by design),
	// RangeIndex 1 (Self Only, 0-0 yd = synthetic 93001's value),
	// SchoolMask 0x1 (physical), CastingTimeIndex 1 (instant, default
	// 0). Attributes_0 0x20000100 not encoded (seed has no attributes
	// field). The knockback leg is live in Go (spells.go:18835,
	// handleEffectKnockBack: MiscValue*0.1 = speedXY, rolled
	// BasePoints*0.1 = speedZ; 62659 precedent).
	{id: 65100, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effKnockBack, dieSides: 1, basePoints: 99, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, misc: 200}}},

	// 15786 Earthbind Totem (zone_nagrand.cpp:99/:287, captive
	// escort totem cast) — wago.tools 3.4.0.43659 Spell/SpellEffect/
	// SpellMisc rows, 2026-11-14 verdict: single effect 28 SUMMON,
	// DieSides 1, BasePoints 4 (CalcValue +1 = 5, matching Wowhead
	// "Value: 5"), TargetA 41 (TARGET_DEST_CASTER_FRONT_RIGHT),
	// TargetB 0, RadiusIndex 0 (no SpellRadius row), MiscValue 2630
	// (Earthbind Totem entry, TDB 26091), MiscValueB 81
	// (SummonProperties 81 — real row seeded below, wago.tools
	// 3.4.0.43659: Control 1/Guardian, Faction 0, Title 4/TOTEM,
	// Slot 2, Flags_0 0x100002); DurationIndex 22 (45000 ms, seeded
	// real row below), RangeIndex 1 (Self Only, 0-0 yd = synthetic
	// 93001's value), SchoolMask 0x8 (nature), CastingTimeIndex 1
	// (instant, default 0). The 50% slow is the totem creature's
	// own aura, not an effect of this spell. Creature path live in
	// Go (boss_ai.go creatureEffectSummonType -> summonTempCreature
	// -> ev19, entry-2630 skip in npc_nagrand_captive.lua).
	{id: 15786, durIdx: 22, rangeIdx: 93001, school: schoolNature, effects: [3]effect{{effect: effSummon, dieSides: 1, basePoints: 4, targetA: targetDestCasterFrontRight, misc: 2630, miscB: 81}}},

	// Felblood Kael'thas phoenix spells (boss_felblood_kaelthas.cpp
	// enum :62-78) — wago.tools 3.4.0.43659 Spell/SpellEffect/
	// SpellMisc rows, 2026-11-14 verdict. All RangeIndex 1 (Self Only,
	// 0-0 yd = synthetic 93001's value); CastingTimeIndex 1 = instant
	// (default 0) except 44196 (index 5, 2000 ms, seeded real row
	// below). Attributes_0 not encoded (seed has no attributes field).
	// 44194 Phoenix: single effect 28 SUMMON, DieSides 1, BasePoints 0,
	// TargetA 73 (TARGET_DEST_CASTER_RADIUS), RadiusIndex 9 (20 yd,
	// seeded real row below), MiscValue 24674 (NPC_PHOENIX 24674),
	// MiscValueB 64; DurationIndex 21 (-1, seeded real row).
	{id: 44194, durIdx: 21, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, basePoints: 0, targetA: targetDestCasterRadius, radiusIdx: 9, misc: 24674, miscB: 64}}},
	// 44196 Rebirth: eff0 2 SCHOOL_DAMAGE, BasePoints 1749, DieSides
	// 501, TargetA 18 / TargetB 16 (TARGET_UNIT_DEST_AREA_ENEMY),
	// RadiusIndex 18 (15 yd, seeded real row below); eff1 98
	// KNOCK_BACK, BasePoints 249, DieSides 1, same targets/radius,
	// MiscValue 400. DurationIndex 0 (no SpellDuration row, by design).
	{id: 44196, castTimeIdx: 5, rangeIdx: 93001, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 501, basePoints: 1749, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 18},
		{effect: effKnockBack, dieSides: 1, basePoints: 249, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 18, misc: 400},
	}},
	// 44197 Burn: single effect 6 APPLY_AURA, aura 23
	// PERIODIC_TRIGGER_SPELL, DieSides 0, BasePoints 0, TargetA 1,
	// AuraPeriod 2000, TriggerSpell 44198 (seeded below — verdict
	// 2026-11-14, the chain is live in code); DurationIndex 21.
	{id: 44197, durIdx: 21, rangeIdx: 93001, school: schoolFire, effects: [3]effect{{effect: effApplyAura, dieSides: 0, basePoints: 0, aura: auraPeriodicTrig, auraPeriod: 2000, trigger: 44198, targetA: targetCaster}}},
	// 44198 Burn (trigger of 44197's periodic aura;
	// boss_felblood_kaelthas.cpp:75 SPELL_BURN trigger) — wago.tools
	// 3.4.0.43659 Spell/SpellEffect/SpellMisc rows, 2026-11-14
	// verdict: single effect 2 SCHOOL_DAMAGE, BasePoints 1749,
	// DieSides 501, TargetA 22 (TARGET_SRC_CASTER), TargetB 15
	// (TARGET_UNIT_SRC_AREA_ENEMY), RadiusIndex 14 (8 yd, seeded
	// real row); DurationIndex 0 (no SpellDuration row, by design),
	// RangeIndex 1 (Self Only, 0-0 yd = synthetic 93001's value),
	// SchoolMask 0x4 (fire), CastingTimeIndex 1 (instant, default
	// 0). Attributes_0 0x0 not encoded (seed has no attributes
	// field). The school-damage leg is live in Go (44196/44199
	// same shape).
	{id: 44198, rangeIdx: 93001, school: schoolFire, effects: [3]effect{{effect: effSchoolDamage, dieSides: 501, basePoints: 1749, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 14}}},
	// 44199 Ember Blast: eff0 2 SCHOOL_DAMAGE, BasePoints 1749,
	// DieSides 501, TargetA 18 / TargetB 16, RadiusIndex 14 (8 yd,
	// seeded real row); eff1 98 KNOCK_BACK, BasePoints 49, DieSides 1,
	// same targets/radius, MiscValue 100; eff2 6 APPLY_AURA, aura 18
	// MOD_INVISIBILITY, BasePoints 9998, DieSides 1, TargetA 1,
	// MiscValue 4. DurationIndex 21.
	{id: 44199, durIdx: 21, rangeIdx: 93001, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 501, basePoints: 1749, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 14},
		{effect: effKnockBack, dieSides: 1, basePoints: 49, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 14, misc: 100},
		{effect: effApplyAura, dieSides: 1, basePoints: 9998, aura: auraModInvisibility, targetA: targetCaster, misc: 4},
	}},
	// 17683 Full Heal: single effect 67 HEAL_MAX_HEALTH, DieSides 1,
	// BasePoints 0, TargetA 1; DurationIndex 0 (no row, by design).
	{id: 17683, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effHealMaxHealth, dieSides: 1, basePoints: 0, targetA: targetCaster}}},

	// 31331 Feral Spirit (hyjalAI.h:39 SPELL_SUMMON_DIRE_WOLF; the
	// DB2 name is "Feral Spirit", description "Summons two dire wolves
	// to assist Thrall") — wago.tools 3.4.0.43659 Spell/SpellEffect/
	// SpellMisc rows, 2026-11-14 verdict: single effect 28 SUMMON,
	// DieSides 1, BasePoints 1 (CalcValue +1 = 2 wolves, matching the
	// description), MiscValue 17854 (dire wolf entry), MiscValueB 64
	// (SummonProperties 64, seeded REAL row, wago 2026-11-15 — on the Go
	// multi-summon list, go_origin_summon.go), TargetA 18
	// (TARGET_DEST_CASTER), TargetB 0, RadiusIndex 0; DurationIndex 3
	// (60000 ms, seeded real row below), RangeIndex 12 (Interact
	// Range, 0-5 yd = synthetic 93002's value), SchoolMask 0x1
	// (physical), CastingTimeIndex 1 (instant, default 0). No
	// SpellClassOptions row. The summon needs a creature_template
	// 17854 row in the deployment world DB.
	{id: 31331, durIdx: 3, rangeIdx: 93002, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, basePoints: 1, targetA: targetDestCaster, misc: 17854, miscB: 64}}},
	// 31263 Pyroblast (hyjalAI.h SPELL_PYROBLAST; Jaina's UpdateAI
	// cast) — wago.tools 3.4.0.43659 Spell/SpellEffect/SpellMisc/
	// SpellClassOptions rows, 2026-11-14 verdict: eff0 2 SCHOOL_DAMAGE,
	// BasePoints 4624, DieSides 751, TargetA 6 (TARGET_UNIT_TARGET_ENEMY),
	// RadiusIndex 0; eff1 6 APPLY_AURA, aura 3 PERIODIC_DAMAGE,
	// BasePoints 499, DieSides 1, AuraPeriod 3000, TargetA 53
	// (TARGET_DEST_TARGET_ENEMY), TargetB 16
	// (TARGET_UNIT_DEST_AREA_ENEMY), RadiusIndex 8 (5 yd, seeded real
	// row below); DurationIndex 105 (9000 ms, seeded real row below),
	// RangeIndex 5 (Long Range, 0-40 yd, seeded real row below),
	// SchoolMask 0x4 (fire), CastingTimeIndex 6 (5000 ms, seeded real
	// row below); SpellClassOptions family 3 (Mage), mask 0. Attributes
	// not encoded (seed has no attributes field).
	{id: 31263, durIdx: 105, rangeIdx: 5, castTimeIdx: 6, school: schoolFire, family: familyMage, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 751, basePoints: 4624, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 499, auraPeriod: 3000, targetA: targetDestTargetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 8},
	}},

	// 31260 Brilliance Aura (hyjalAI.h:32 SPELL_BRILLIANCE_AURA;
	// Jaina's aura, "database must handle via creature_addon") —
	// wago.tools 3.4.0.43659 Spell/SpellEffect/SpellMisc rows,
	// 2026-11-14 verdict: eff0 6 APPLY_AURA, aura 4 DUMMY, DieSides 1,
	// BasePoints 0, TargetA 1 (TARGET_UNIT_CASTER); eff1 6 APPLY_AURA,
	// aura 23 PERIODIC_TRIGGER_SPELL, AuraPeriod 4000, TriggerSpell
	// 31308 (seeded below from wago 3.4.0.43659 — the periodic-trigger
	// leg now resolves it), TargetA 1; DurationIndex 21
	// (infinite, seeded row), RangeIndex 1 (Self Only, 0-0 yd =
	// synthetic 93001's value), SchoolMask 0x40 (arcane),
	// CastingTimeIndex 1 (instant, default 0). No SpellClassOptions
	// row (family 0). Attributes not encoded (seed has no attributes
	// field).
	{id: 31260, durIdx: 21, rangeIdx: 93001, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, aura: auraDummy, targetA: targetCaster},
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 4000, trigger: 31308, targetA: targetCaster},
	}},
	// 31308 Brilliance Aura (31260's periodic trigger; hyjalAI.h:32
	// SPELL_BRILLIANCE_AURA eff1 trigger) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc/SpellName rows, 2026-11-14 verdict:
	// single effect 6 APPLY_AURA, aura 137 MOD_TOTAL_STAT_PERCENTAGE,
	// BasePoints 99, DieSides 1, MiscValue 4 (Spirit), TargetA 22
	// (TARGET_SRC_CASTER), TargetB 30 (TARGET_UNIT_SRC_AREA_ALLY),
	// RadiusIndex 10 (30 yd, seeded real row below); DurationIndex 28
	// (5000 ms, seeded real row below), RangeIndex 1 (Self Only, 0-0
	// yd, seeded real row below), SchoolMask 0x40 (arcane),
	// CastingTimeIndex 1 (instant, default 0). No SpellClassOptions
	// row (family 0). EffectChainAmplitude 1 / GroupSizeBasePoints-
	// Coefficient 1 / PvpMultiplier 1 not encoded (seed has no fields
	// for them). Attributes not encoded (seed has no attributes field).
	{id: 31308, durIdx: 28, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraModTotalStatPct, dieSides: 1, basePoints: 99, targetA: targetSrcCaster, targetB: targetUnitSrcAreaAlly, radiusIdx: 10, misc: 4},
	}},
	// 31264 Summon Water Elementals (hyjalAI.h:35
	// SPELL_SUMMON_ELEMENTALS; Jaina's summon) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 28 SUMMON, EffectAmplitude -3 (not encoded — the seed has
	// no amplitude field; C++ EffectSummonType does not use amplitude
	// for the count), BasePoints 2, DieSides 1 (CalcValue +1 = 3
	// elementals, matching "$s1 powerful water elementals"),
	// MiscValue 18001 (water elemental entry), MiscValueB 61
	// (SummonProperties 61 — real row seeded below, wago.tools
	// 3.4.0.43659: Control 1 Guardian, Faction 0, Title 2, Slot 0,
	// Flags_0 0x2), TargetA 32
	// (TARGET_DEST_CASTER_SUMMON), RadiusIndex 15 (3 yd, seeded real
	// row below); DurationIndex 23 (90000 ms, seeded real row below),
	// RangeIndex 1 (Self Only, 0-0 yd = synthetic 93001's value),
	// SchoolMask 0x1 (physical), CastingTimeIndex 3 (500 ms, seeded
	// real row below). No SpellClassOptions row (family 0).
	{id: 31264, durIdx: 23, rangeIdx: 93001, castTimeIdx: 3, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, basePoints: 2, targetA: targetDestCasterSummon, radiusIdx: 15, misc: 18001, miscB: 61}}},
	// 31266 Blizzard (hyjalAI.h:33 SPELL_BLIZZARD; Jaina's UpdateAI
	// cast) — wago.tools 3.4.0.43659 Spell/SpellEffect/SpellMisc rows,
	// 2026-11-14 verdict: eff0 27 PERSISTENT_AREA_AURA, aura 3
	// PERIODIC_DAMAGE, BasePoints 949, DieSides 101, AuraPeriod 2000,
	// TargetA 16 (TARGET_UNIT_DEST_AREA_ENEMY), RadiusIndex 13 (10 yd,
	// seeded real row); eff1 27 PERSISTENT_AREA_AURA, aura 31
	// MOD_INCREASE_SPEED, BasePoints -71, DieSides 1, TargetA 0
	// (NONE), RadiusIndex 13; DurationIndex 1 (10000 ms, real row 1 —
	// the old fixture value 300000 was invented for 24081's assumed
	// DurationIndex; 24081 is absent from wago 3.4.0.43659 entirely —
	// no Spell/SpellName/SpellMisc/SpellEffect/SpellClassOptions row,
	// hotfixes on or off, checked 2026-11-14 — so no real DurationIndex
	// exists to seed and 24081 keeps riding synthetic 90006),
	// RangeIndex 4 (Medium Range,
	// 0-30 yd, seeded real row), SchoolMask 0x10 (frost),
	// CastingTimeIndex 4 (1000 ms, seeded real row below). No
	// SpellClassOptions row (family 0).
	{id: 31266, durIdx: 1, rangeIdx: 4, castTimeIdx: 4, school: schoolFrost, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, dieSides: 101, basePoints: 949, auraPeriod: 2000, targetA: targetUnitDestAreaEnemy, radiusIdx: 13},
		{effect: effPersistentAreaAura, aura: auraModIncreaseSpeed, dieSides: 1, basePoints: -71, radiusIdx: 13},
	}},
	// 52996 Duel / 52994 Duel Victory / 52991 Duel Flag / 7267 Grovel
	// (chapter1.cpp:521-527 Spells_DKI; npc_death_knight_initiate duel
	// machine) -- INFERRED rows, derived from in-tree C++/Go
	// constraints, not client DBC rows; replace with real DBC rows
	// when client data ships. No wago.tools verdict was obtainable
	// for these four ids this tick (search surfaced no spell rows).
	// 52996 is the gossip-arm player cast on the initiate whose hit
	// fires the ev14 SpellHit latch; 52994 is the creature's
	// triggered victory cast on the dueler; 52991 is the player's
	// triggered self-cast that summons the duel flag (the flag-GO
	// summon leg has no Go handler, so the row is a DUMMY self
	// target and RemoveGameObject rides as a no-op); 7267 Grovel is
	// the kneel/beg self-cast the Go engine already treats as
	// visual-only (combat.go:4025). All four ride single DUMMY
	// effects: 52996/52994 TargetA 25 (TARGET_UNIT_TARGET_ANY) at
	// synthetic range 93005 (0-20 yd -- the gossip cast happens at
	// melee range); 52991/7267 TargetA 1 (TARGET_UNIT_CASTER) at
	// 93001 (self only). School physical (0x1) is the inferred
	// default; cast time stays default 0 (instant).
	{id: 52996, rangeIdx: 93005, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetUnitTargetAny}}},
	{id: 52994, rangeIdx: 93005, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetUnitTargetAny}}},
	{id: 52991, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	{id: 7267, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	// 52693 Realm of Shadows (chapter1.cpp:778 SPELL_REALM_OF_SHADOWS;
	// npc_salanar_the_horseman gossip arm) -- INFERRED row, derived
	// from in-tree C++/Go constraints, not a client DBC row; replace
	// with a real DBC row when client data ships. Self-cast DUMMY
	// like 7267 Grovel: TargetA 1 (TARGET_UNIT_CASTER), range 93001.
	{id: 52693, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	// 52517 Gothik collect (chapter1.cpp:985; npc_dkc1_gothik
	// MoveInLineOfSight arm) -- INFERRED row, derived from in-tree
	// C++/Go constraints, not a client DBC row; replace with a real
	// DBC row when client data ships. The ghoul casts it on its
	// owner (the player), triggered, within 10yd: TargetA 25
	// (TARGET_UNIT_TARGET_ANY) at synthetic range 93005 (0-20 yd),
	// like 52994 Duel Victory.
	{id: 52517, rangeIdx: 93005, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetUnitTargetAny}}},
	// 17472 Death Pact 3 (boss_baron_rivendare.cpp:159-163;
	// npc_summoned_skeleton SpellHit arm) -- INFERRED row, derived
	// from in-tree C++/Go constraints, not a client DBC row; replace
	// with a real DBC row when client data ships. The skeleton
	// self-casts it, triggered, when hit by 17471: TargetA 1
	// (TARGET_UNIT_CASTER), range 93001 (self only), like 7267.
	{id: 17472, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	// 38576 Depth Charge Trap (npc_depth_chargeAI MoveInLineOfSight,
	// zone_azshara.cpp:348-360; depth-charge ev27 leg) — wago.tools
	// 3.4.0.43659 SpellEffect row (2026-11-14 verdict) + Wowhead
	// WotLK spell page: single effect 98 KNOCK_BACK, DieSides 1,
	// BasePoints 224 (Wowhead "Value: 225"), TargetA 24
	// (TARGET_UNIT_CONE_ENEMY_24), TargetB 0, RadiusIndex 13
	// (10 yd, seeded real row below), MiscValue 15, MiscValueB 0;
	// DurationIndex 0 (no SpellDuration row, by design), RangeIndex 1
	// (Self Only, 0-0 yd, seeded real row below), SchoolMask 0x1
	// (physical), CastingTimeIndex 0 (instant, default). The knockback
	// leg is live in Go (boss_ai.go:3763 creatureEffectKnockBack;
	// MiscValue*0.1 = speedXY, rolled BasePoints*0.1 = speedZ; 65100
	// precedent). C++ casts on every MIOS fire with a player in range
	// (no cast latch) — the per-tick ev27 refire matches that shape,
	// and the cone gate bounds it to players in the charge's front
	// arc (bossConeEffectTargetsFiltered); the knockback itself pushes
	// the player out of the 5yd gate within a tick or two.
	{id: 38576, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effKnockBack, dieSides: 1, basePoints: 224, targetA: targetUnitConeEnemy, radiusIdx: 13, misc: 15}}},

	// Player interrupt spells (Shade of Aran SpellHit arm,
	// boss_shade_of_aran.cpp:481-501 + creature:HasSpellEffect bridge):
	// every row below is a wago.tools 3.4.0.43659 Spell/SpellEffect/
	// SpellMisc verdict (2026-11-14), all TargetA 6
	// (TARGET_UNIT_TARGET_ENEMY), TargetB 0, RadiusIndex 0, Misc 0/0
	// unless stated. CastingTimeIndex 1 (instant, SpellCastTimes 1:
	// Base 0) rides the seed default castTimeIdx 0 (38576 precedent).
	// DurationIndex 28/32 rows already seeded; 31/35/39 added below.
	// RangeIndex 4 row already seeded; 2/34 added below.
	// CORRECTIONS from the same verdict: Earth Shock player ranks are
	// NOT interrupt spells in this build (effects 6 APPLY_AURA 138 +
	// 2 SCHOOL_DAMAGE, no effect 68; ranks 8042/8044/8045/8046/10412/
	// 10413/10414/25454/49230/49231; 8047-8049 do not exist; 8050/8052/
	// 8053 are Flame Shock) — do NOT seed them as interrupts. NOT YET
	// SEEDED (SpellMisc rows not in the verdict — follow-up wago read):
	// Kick Rank 5 (38768, 45356), Shield Bash Rank 4 (29704), Spell
	// Lock Rank 2 trigger (19647 seeded with trigger 0; the silence
	// trigger ID needs a follow-up read). Trigger targets 29703
	// (Shield Bash 72) and 24259 (Spell Lock 19244) are unseeded — the trigger arm
	// no-ops on unknown spells (spellKnown gate), same as any
	// unseeded trigger in the seed.
	// Shield Bash 72 (Rank 1): durIdx 32 (6000), rangeIdx 2 (5 yd),
	// school 0x1; eff0 68 INTERRUPT_CAST Die 1 Base -1; eff1 64
	// TRIGGER_SPELL Die 1 Base -1 Trigger 29703.
	{id: 72, durIdx: 32, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effTriggerSpell, dieSides: 1, basePoints: -1, targetA: targetEnemy, trigger: 29703}}},
	// Shield Bash 1671 (Rank 2): durIdx 32, rangeIdx 2, school 0x1;
	// eff0 68 Die 1 Base -1; eff1 2 SCHOOL_DAMAGE Die 1 Base 17.
	{id: 1671, durIdx: 32, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 17, targetA: targetEnemy}}},
	// Shield Bash 1672 (Rank 3): durIdx 32, rangeIdx 2, school 0x1;
	// eff0 68 Die 1 Base -1; eff1 2 SCHOOL_DAMAGE Die 1 Base 44.
	{id: 1672, durIdx: 32, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 44, targetA: targetEnemy}}},
	// Kick 1766 (Rank 1): durIdx 28 (5000), rangeIdx 2, school 0x1;
	// eff0 68 INTERRUPT_CAST Die 0 Base 0 (single effect).
	{id: 1766, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 0, basePoints: 0, targetA: targetEnemy}}},
	// Kick 1767 (Rank 2): durIdx 28, rangeIdx 2, school 0x1; eff0 68
	// Die 0 Base 0; eff1 2 SCHOOL_DAMAGE Die 1 Base 29.
	{id: 1767, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 0, basePoints: 0, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 29, targetA: targetEnemy}}},
	// Kick 1768 (Rank 3): durIdx 28, rangeIdx 2, school 0x1; eff0 68
	// Die 0 Base 0; eff1 2 SCHOOL_DAMAGE Die 1 Base 44.
	{id: 1768, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 0, basePoints: 0, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 44, targetA: targetEnemy}}},
	// Kick 1769 (Rank 4): durIdx 28, rangeIdx 2, school 0x1; eff0 68
	// Die 1 Base -1; eff1 2 SCHOOL_DAMAGE Die 1 Base 79.
	{id: 1769, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 79, targetA: targetEnemy}}},
	// Pummel 6552 (Rank 1): durIdx 35 (4000), rangeIdx 2, school 0x1;
	// eff0 68 INTERRUPT_CAST Die 0 Base 0 (single effect).
	{id: 6552, durIdx: 35, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 0, basePoints: 0, targetA: targetEnemy}}},
	// Pummel 6554 (Rank 2): durIdx 35, rangeIdx 2, school 0x1; eff0 68
	// Die 0 Base 0; eff1 2 SCHOOL_DAMAGE Die 1 Base 49.
	{id: 6554, durIdx: 35, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effInterruptCast, dieSides: 0, basePoints: 0, targetA: targetEnemy}, {effect: effSchoolDamage, dieSides: 1, basePoints: 49, targetA: targetEnemy}}},
	// Counterspell 2139 (single rank): durIdx 31 (8000), rangeIdx 4
	// (30 yd), school 0x40; eff0 68 Die 1 Base -1 (single effect).
	{id: 2139, durIdx: 31, rangeIdx: 4, school: schoolArcane, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}}},
	// Spell Lock 19244 (Rank 1): durIdx 28 (5000), rangeIdx 4,
	// school 0x20; eff0 68 Die 1 Base -1; eff1 64 TRIGGER_SPELL
	// Die 1 Base -1 Trigger 24259.
	{id: 19244, durIdx: 28, rangeIdx: 4, school: schoolShadow, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effTriggerSpell, dieSides: 1, basePoints: -1, targetA: targetEnemy, trigger: 24259}}},
	// Spell Lock 19647 (Rank 2): durIdx 32 (6000), rangeIdx 4,
	// school 0x20; eff0 68 Die 1 Base -1; eff1 64 TRIGGER_SPELL
	// Die 0 Base 0, trigger 0 — the trigger ID was not in the
	// wago read (3c75dc00 verdict), so the trigger arm no-ops on
	// the spellKnown gate until a follow-up read fills it; the
	// interrupt arm (eff0) is live.
	{id: 19647, durIdx: 32, rangeIdx: 4, school: schoolShadow, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effTriggerSpell, dieSides: 0, basePoints: 0, targetA: targetEnemy, trigger: 0}}},
	// Mind Freeze 47528 (single rank): durIdx 35 (4000), rangeIdx 2,
	// school 0x10; eff0 68 Die 1 Base -1 (single effect).
	{id: 47528, durIdx: 35, rangeIdx: 2, school: schoolFrost, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}}},
	// Wind Shear 57994 (single rank): durIdx 39 (2000), rangeIdx 34
	// (25 yd), school 0x8; eff0 68 Die 1 Base -1; eff1 63 THREAT
	// Die 1 Base -1.
	{id: 57994, durIdx: 39, rangeIdx: 34, school: schoolNature, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetEnemy}, {effect: effThreat, dieSides: 1, basePoints: -1, targetA: targetEnemy}}},

	// Shade of Aran normal-cast bolts (boss_shade_of_aran.lua:96-99
	// SPELL_FROSTBOLT/FIREBALL/ARCMISSLE/CHAINSOFICE; the 1s pump
	// draws from these schools) — wago.tools 3.4.0.43659
	// Spell/SpellMisc/SpellEffect/SpellPower/SpellLevels rows,
	// browser task c0a20af1 PARTIAL verdict 2026-11-14 (4 of 16
	// spells read; the site blocked the other 12 — those 12 + cannon
	// 42442 landed in the e7682859 verdict below, build 3.4.0.47659).
	// All four below
	// are DifficultyID 0; TargetB 0, RadiusIndex 0, Misc 0/0 unless
	// stated. No SpellClassOptions row was read for any of the four
	// (family stays 0); SpellLevel (70, 30 on 29991) is not encoded
	// — the seed has no SpellLevel field. ManaCost now real, so the
	// pump's non-triggered casts drain Aran's mana toward the <20%
	// drink trigger (boss_ai.go:608-622; triggered casts skip
	// TakePower :509).
	// 29954 Frostbolt: castTimeIdx 5 (2000), durIdx 35 (4000),
	// rangeIdx 6 (Vision Range 0-100, seeded real row below),
	// school 0x10; ManaCost 2500, PowerCostPct 0; eff0 2
	// SCHOOL_DAMAGE Die 1001 Base 3499 TargetA 6; eff1 6 APPLY_AURA
	// aura 33 MOD_DECREASE_SPEED Die 1 Base -66 TargetA 6,
	// Mechanic 11.
	{id: 29954, castTimeIdx: 5, durIdx: 35, rangeIdx: 6, school: schoolFrost, manaCost: 2500, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1001, basePoints: 3499, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -66, mechanic: 11, targetA: targetEnemy},
	}},
	// 29953 Fireball: castTimeIdx 5 (2000), durIdx 0, rangeIdx 6,
	// school 0x4; ManaCost 2500; single effect 2 SCHOOL_DAMAGE
	// Die 1381 Base 3909 TargetA 6.
	{id: 29953, castTimeIdx: 5, rangeIdx: 6, school: schoolFire, manaCost: 2500, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1381, basePoints: 3909, targetA: targetEnemy}}},
	// 29955 Arcane Missiles: castTimeIdx 4 (1000), durIdx 28 (5000),
	// rangeIdx 6, school 0x40; ManaCost 2850; eff0 6 APPLY_AURA
	// aura 23 PERIODIC_TRIGGER_SPELL AuraPeriod 1000 Trigger 29956
	// TargetA 6; eff1 77 SCRIPT_EFFECT TargetA 6; eff2 6 APPLY_AURA
	// aura 4 DUMMY TargetA 1. The DifficultyID 175 SpellMisc row
	// (castTimeIdx 10, otherwise identical) is not seeded — the
	// seed is difficulty-0 only. Trigger 29956 is unseeded (not in
	// the verdict) — the periodic-trigger leg no-ops on the
	// spellKnown gate until a follow-up read fills it.
	{id: 29955, castTimeIdx: 4, durIdx: 28, rangeIdx: 6, school: schoolArcane, manaCost: 2850, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, trigger: 29956, targetA: targetEnemy},
		{effect: effScriptEffect, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	// 29991 Chains of Ice: castTimeIdx 1 (instant, rides the seed
	// default 0 — 38576 precedent), durIdx 1 (10000), rangeIdx 4
	// (30 yd), school 0x10; ManaCost 350; single effect 6 APPLY_AURA
	// aura 26 MOD_ROOT Die 1 Base -1 TargetA 6, Mechanic 0 as
	// stored. SpellLevel is 30, not 70.
	{id: 29991, durIdx: 1, rangeIdx: 4, school: schoolFrost, manaCost: 350, effects: [3]effect{{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, targetA: targetEnemy}}},
	// Remaining 12 Aran spells + cannon 42442 — wago.tools 3.4.0.47659
	// Spell/SpellMisc/SpellEffect/SpellPower rows, browser task
	// e7682859 verdict 2026-11-14 (all 13 read; build 47659, NOT the
	// 43659 build the first 4 used). DifficultyID 0 only (the 175
	// rows are not seeded). TARGET NUMBER TRANSLATION (wago 3.4.0
	// renumbered three targets vs TC 3.3.5 SharedDefines.h — the Go
	// engine is 3.3.5-numbered, target_dest_targets.go:37): wago 50
	// TARGET_DEST_DEST -> 87 (29951 TargetA); wago 28
	// TARGET_UNIT_DEST_AREA_ENEMY -> 16 (29951 TargetB); wago 7
	// TARGET_UNIT_CASTER_AREA_PARTY -> 20 (42442 TargetB). All other
	// target numbers agree between builds. RADIUS: radiusIdx = wago
	// RadiusIndex_0 (repo convention — 31266/38576/65100 precedent);
	// RadiusIndex_1 is nonzero on 29961 (40), 29963 eff0/eff2 (12),
	// 29978 (12) and is recorded in the per-spell comments, not
	// seeded — follow-up read 54197700 settles whether the 3.3.5
	// field holds _0 or _1. AUX ROWS STILL MISSING (follow-up read
	// 54197700): SpellDuration 18, SpellRange 36/11/13,
	// SpellCastTimes 7, SpellRadius 17/19/27/38/12/30 (+40 if the
	// radius question resolves to _1). DO NOT COMMIT the regen until
	// those rows land — a missing aux row reads as 0 (store.go).
	// ManaCost is SpellPower.ManaCost (SpellMisc has no cost column
	// in 3.4.0); no SpellPower row = cost 0.
	// 30035 Mass Slow: castTimeIdx 1 (instant, default 0), durIdx 1
	// (10000), rangeIdx 1 (Self Only), school 0x40; ManaCost 1300;
	// eff0 6 APPLY_AURA aura 193 MELEE_SLOW Die 1 Base -36 TargetA
	// 22 TargetB 15 RadiusIndex_0 17; eff1 6 APPLY_AURA aura 33
	// MOD_DECREASE_SPEED Die 1 Base -61 TargetA 22/15 RadiusIndex_0
	// 19.
	{id: 30035, durIdx: 1, rangeIdx: 1, school: schoolArcane, manaCost: 1300, effects: [3]effect{
		{effect: effApplyAura, aura: auraMeleeSlow, dieSides: 1, basePoints: -36, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 17},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -61, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 19},
	}},
	// 29946 Flame Wreath: castTimeIdx 1 (instant, default 0), durIdx
	// 18 (row pending), rangeIdx 36 (row pending), school 0x4; no
	// SpellPower row (cost 0); eff0 27 PERSISTENT_AREA_AURA aura 23
	// PERIODIC_TRIGGER_SPELL AuraPeriod 500 Trigger 30023 TargetA 63
	// TargetB 16 RadiusIndex_0 15; eff1 3 DUMMY Die 1 BasePoints
	// 29946 TargetA 1 — the 29946 base value is verdict-literal
	// (equals the spell ID; the DUMMY leg ignores basePoints).
	{id: 29946, durIdx: 18, rangeIdx: 36, school: schoolFire, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicTrig, auraPeriod: 500, trigger: 30023, targetA: targetDestTargetAny, targetB: targetUnitDestAreaEnemy, radiusIdx: 15},
		{effect: effDummy, dieSides: 1, basePoints: 29946, targetA: targetCaster},
	}},
	// 29961 AoE Counterspell: castTimeIdx 1 (instant, default 0),
	// durIdx 1 (10000), rangeIdx 11 (row pending), school 0x40; no
	// SpellPower row (cost 0); single effect 68 INTERRUPT_CAST Die 1
	// Base -1 TargetA 22 TargetB 15 RadiusIndex_0 0 (RadiusIndex_1
	// 40 — see the radius note above; the ev14 HasSpellEffect arm
	// keys on effect 68, not the radius).
	{id: 29961, durIdx: 1, rangeIdx: 11, school: schoolArcane, effects: [3]effect{{effect: effInterruptCast, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy}}},
	// 32265 Magnetic Pull (PLAYERPULL): castTimeIdx 1 (instant,
	// default 0), durIdx 0, rangeIdx 36 (row pending), school 0x1;
	// no SpellPower row (cost 0); single effect 124 PULL_TOWARDS
	// MiscValue 125 TargetA 22 TargetB 15 RadiusIndex_0 27.
	{id: 32265, rangeIdx: 36, school: schoolPhysical, effects: [3]effect{{effect: effPullTowards, misc: 125, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 27}}},
	// 29973 Arcane Explosion: castTimeIdx 7 (row pending), durIdx 0,
	// rangeIdx 1 (Self Only), school 0x40; ManaCost 1000; eff0 2
	// SCHOOL_DAMAGE Die 2001 Base 8999 TargetA 22 TargetB 15
	// RadiusIndex_0 38 (BonusCoefficient 1, not encoded — the seed
	// has no bonus-coefficient field); eff1 98 KNOCK_BACK Die 41
	// Base 179 MiscValue 180 TargetA 22/15 RadiusIndex_0 38.
	{id: 29973, castTimeIdx: 7, rangeIdx: 1, school: schoolArcane, manaCost: 1000, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 2001, basePoints: 8999, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 38},
		{effect: effKnockBack, dieSides: 41, basePoints: 179, misc: 180, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 38},
	}},
	// 29967 Blink Center: castTimeIdx 1 (instant, default 0), durIdx
	// 0, rangeIdx 13 (row pending), school 0x8; no SpellPower row
	// (cost 0); single effect 5 TELEPORT_UNITS TargetA 1 TargetB 46
	// (wago shows no name for effect 5 — the TC number is
	// SPELL_EFFECT_TELEPORT_UNITS, SharedDefines.h:816).
	{id: 29967, rangeIdx: 13, school: schoolNature, effects: [3]effect{{effect: effTeleportUnits, targetA: targetCaster, targetB: targetDestNearbyEntry}}},
	// 29951 Circular Blizzard: castTimeIdx 1 (instant, default 0),
	// durIdx 1 (10000), rangeIdx 1 (Self Only), school 0x10; no
	// SpellPower row (cost 0); eff0 27 PERSISTENT_AREA_AURA aura 3
	// PERIODIC_DAMAGE AuraPeriod 2000 Die 375 Base 1312 TargetA 87
	// (translated from wago 50) TargetB 16 (translated from wago 28)
	// RadiusIndex_0 17; eff1 27 PERSISTENT_AREA_AURA aura 33
	// MOD_DECREASE_SPEED Die 1 Base -66 Mechanic 11 TargetA 87/16
	// RadiusIndex_0 17.
	{id: 29951, durIdx: 1, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, auraPeriod: 2000, dieSides: 375, basePoints: 1312, targetA: targetDestDest, targetB: targetUnitDestAreaEnemy, radiusIdx: 17},
		{effect: effPersistentAreaAura, aura: auraDecSpeed, dieSides: 1, basePoints: -66, mechanic: 11, targetA: targetDestDest, targetB: targetUnitDestAreaEnemy, radiusIdx: 17},
	}},
	// 29963 Mass Polymorph: castTimeIdx 5 (2000), durIdx 1 (10000),
	// rangeIdx 4 (30 yd), school 0x1; ManaCost 0, PowerCostPct 10;
	// eff0 6 APPLY_AURA aura 5 MOD_CONFUSE Die 1 Base -1 TargetA 22
	// TargetB 15 RadiusIndex_0 0 (RadiusIndex_1 12 — see radius
	// note); eff1 6 APPLY_AURA aura 56 TRANSFORM MiscValue 1933
	// TargetA 22/15 RadiusIndex_0 12; eff2 6 APPLY_AURA aura 20
	// OBS_MOD_HEALTH Die 1 Base 6 AuraPeriod 1000 TargetA 22/15
	// RadiusIndex_0 0 (RadiusIndex_1 12 — see radius note).
	{id: 29963, castTimeIdx: 5, durIdx: 1, rangeIdx: 4, school: schoolPhysical, manaCostPct: 10, effects: [3]effect{
		{effect: effApplyAura, aura: auraModConfuse, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy},
		{effect: effApplyAura, aura: auraTransform, misc: 1933, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 12},
		{effect: effApplyAura, aura: auraObsModHealth, dieSides: 1, basePoints: 6, auraPeriod: 1000, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy},
	}},
	// 29975 Conjure: castTimeIdx 5 (2000), durIdx 0, rangeIdx 1
	// (Self Only), school 0x40; ManaCost 885; single effect 3 DUMMY
	// TargetA 1.
	{id: 29975, castTimeIdx: 5, rangeIdx: 1, school: schoolArcane, manaCost: 885, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	// 30024 Drink: castTimeIdx 1 (instant, default 0), durIdx 28
	// (5000), rangeIdx 1 (Self Only), school 0x1; no SpellPower row
	// (cost 0); single effect 6 APPLY_AURA aura 21 OBS_MOD_POWER Die
	// 1 Base 19 AuraPeriod 1000 TargetA 1 (EffectAmplitude 1, not
	// encoded — the seed has no amplitude field). The drink aura's
	// periodic mana refill still has no tick executor in Go
	// (boss_shade_of_aran.lua header) — this seed only makes the
	// spell exist; the Lua drink machine is unchanged.
	{id: 30024, durIdx: 28, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effApplyAura, aura: auraObsModPower, dieSides: 1, basePoints: 19, auraPeriod: 1000, targetA: targetCaster}}},
	// 32453 Potion: castTimeIdx 3 (500), durIdx 0, rangeIdx 1 (Self
	// Only), school 0x2; no SpellPower row (cost 0); single effect
	// 30 ENERGIZE Die 1 Base 31999 TargetA 1.
	{id: 32453, castTimeIdx: 3, rangeIdx: 1, school: schoolHoly, effects: [3]effect{{effect: effEnergize, dieSides: 1, basePoints: 31999, targetA: targetCaster}}},
	// 29978 AoE Pyroblast (SHADOW_PYRO): castTimeIdx 3 (500), durIdx
	// 0, rangeIdx 6 (Vision Range 0-100), school 0x4; no SpellPower
	// row (cost 0); single effect 2 SCHOOL_DAMAGE Die 1 Base 6799
	// TargetA 22 TargetB 15 RadiusIndex_0 0 (RadiusIndex_1 12 — see
	// radius note; BonusCoefficient 1, not encoded).
	{id: 29978, castTimeIdx: 3, rangeIdx: 6, school: schoolFire, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1, basePoints: 6799, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy}}},
	// 42442 Cannon fire (Valiance Keep cannon template spell):
	// castTimeIdx 1 (instant, default 0), durIdx 0, rangeIdx 1 (Self
	// Only), school 0x1; no SpellPower row (cost 0); single effect 3
	// DUMMY TargetA 22 TargetB 20 (translated from wago 7)
	// RadiusIndex_0 30. Speed 40 in SpellMisc is not encoded — the
	// seed has no speed field.
	// Grand-champion / wyrmrest seed set (wago.tools 3.4.0.43659,
	// 2026-11-14): all rows Diff 0 (normal 10-man) where the spell has
	// multiple difficulties. CastTimeIndex 1 (instant) is default 0 and
	// is omitted; DurationIndex 0 likewise. BonusCoefficient is not
	// encoded (seed has no bonus field; 29978 precedent) and no seeded
	// spell here has a nonzero EffectAmplitude. 66045 Haste was
	// re-read independently (d3978dfa, 02:30 UTC): aura 193/61/31
	// confirmed verbatim — the MELEE_SLOW +99 base is genuine DBC
	// data for this custom NPC spell, seeded as read. 67709
	// Eviscerate: wago 3.4.0.43659 exposes no PointsPerCombo column
	// (36 columns, no combo column; EffectPointsPerResource 0), so
	// no combo-point value is encoded.
	// 63010 Charge: castTimeIdx 1, durIdx 0, rangeIdx 95 (8-25 yd),
	// school 0x1; eff0 2 SCHOOL_DAMAGE Die 1 Base 8499 TargetA 6; eff1
	// 96 CHARGE Base -1 TargetA 6; eff2 77 SCRIPT_EFFECT TargetA 6.
	{id: 63010, rangeIdx: 95, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 8499, targetA: targetEnemy},
		{effect: effCharge, dieSides: 1, basePoints: -1, targetA: targetEnemy},
		{effect: effScriptEffect, targetA: targetEnemy},
	}},
	// 65147 Shield-Breaker: castTimeIdx 2 (250), durIdx 0, rangeIdx 9
	// (10-30 yd), school 0x1; eff0 2 SCHOOL_DAMAGE Die 1 Base 1999
	// TargetA 6; eff1 77 SCRIPT_EFFECT TargetA 6.
	{id: 65147, castTimeIdx: 2, rangeIdx: 9, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 1999, targetA: targetEnemy},
		{effect: effScriptEffect, targetA: targetEnemy},
	}},
	// 49256 Wyrmrest Defender Mount: castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 7 (0-10 yd), school 0x1; eff0 6 APPLY_AURA aura 236
	// CONTROL_VEHICLE TargetA 25; eff1 6 APPLY_AURA aura 4 DUMMY
	// TargetA 1. ActiveIconFileDataID 134153 not encoded (no field).
	{id: 49256, durIdx: 21, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraControlVehicle, targetA: targetUnitTargetAny},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster},
	}},
	// 52421 Wyrmrest Defender low-health emote: castTimeIdx 1, durIdx
	// 0, rangeIdx 1 (Self Only), school 0x1; single effect 77
	// SCRIPT_EFFECT Die 1 Base 13357 TargetA 1.
	{id: 52421, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effScriptEffect, dieSides: 1, basePoints: 13357, targetA: targetCaster}}},
	// 49263 Renew: castTimeIdx 1, durIdx 8 (15000), rangeIdx 1 (Self
	// Only), school 0x1; single effect 6 APPLY_AURA aura 20
	// OBS_MOD_HEALTH Die 1 Base 9 AuraPeriod 3000 TargetA 1
	// (BonusCoefficient 0.25, not encoded).
	{id: 49263, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effApplyAura, aura: auraObsModHealth, dieSides: 1, basePoints: 9, auraPeriod: 3000, targetA: targetCaster}}},
	// 61839 Polymorph: Sheep: castTimeIdx 1, durIdx 28 (5000),
	// rangeIdx 6 (Vision 0-100), school 0x40; eff0 6 APPLY_AURA aura
	// 4 DUMMY TargetA 25; eff1 64 TRIGGER_SPELL TriggerSpell 61834
	// TargetA 25 (61834 itself unseeded — trigger dormant).
	{id: 61839, durIdx: 28, rangeIdx: 6, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetUnitTargetAny},
		{effect: effTriggerSpell, trigger: 61834, targetA: targetUnitTargetAny},
	}},
	// 51347 Teleport Visual Only: castTimeIdx 1, durIdx 0, rangeIdx 1
	// (Self Only), school 0x40; single effect 3 DUMMY TargetA 1.
	{id: 51347, rangeIdx: 1, school: schoolArcane, effects: [3]effect{{effect: effDummy, targetA: targetCaster}}},
	// 61995 Improved Blink: castTimeIdx 1, durIdx 35 (4000), rangeIdx
	// 1 (Self Only), school 0x40; single effect 6 APPLY_AURA aura 4
	// DUMMY Die 1 Base -1 MiscValue 14 SpellClassMask_0 65536 TargetA 1.
	{id: 61995, durIdx: 35, rangeIdx: 1, school: schoolArcane, effects: [3]effect{{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, misc: 14, targetA: targetCaster, mask: [3]uint32{65536, 0, 0}}}},
	// 51017 Summon Arcane Beam: castTimeIdx 1, durIdx 1 (10000),
	// rangeIdx 1 (Self Only), school 0x1; single effect 28 SUMMON Die
	// 1 MiscValue 28239 MiscValueB 64 (SummonProperties row 64 is REAL
	// now, wago.tools 3.4.0.43659, 2026-11-15 — see SummonProperties
	// note) RadiusIndex_0 8 TargetA 73.
	{id: 51017, durIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 28239, miscB: 64, radiusIdx: 8, targetA: targetDestCasterRadius}}},
	// 66042 Fireball (grand champion mage): castTimeIdx 19 (2500),
	// durIdx 0, rangeIdx 5 (Long 0-40), school 0x4; single effect 2
	// SCHOOL_DAMAGE Die 403 Base 3298 TargetA 6 (BonusCoefficient 1,
	// not encoded).
	{id: 66042, castTimeIdx: 19, rangeIdx: 5, school: schoolFire, effects: [3]effect{{effect: effSchoolDamage, dieSides: 403, basePoints: 3298, targetA: targetEnemy}}},
	// 66043 Polymorph (grand champion mage): castTimeIdx 16 (1500),
	// durIdx 39 (2000), rangeIdx 4 (Medium 0-30), school 0x40; eff0 6
	// APPLY_AURA aura 5 MOD_CONFUSE Die 1 Base -1 TargetA 6; eff1 6
	// APPLY_AURA aura 56 TRANSFORM MiscValue 1933 TargetA 6; eff2 108
	// DISPEL_MECHANIC MiscValue 21 TargetA 6.
	{id: 66043, castTimeIdx: 16, durIdx: 39, rangeIdx: 4, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraModConfuse, dieSides: 1, basePoints: -1, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraTransform, misc: 1933, targetA: targetEnemy},
		{effect: effDispelMechanic, misc: 21, targetA: targetEnemy},
	}},
	// 66044 Blast Wave (grand champion mage): castTimeIdx 1, durIdx 35
	// (4000), rangeIdx 1 (Self Only), school 0x4; eff0 2
	// SCHOOL_DAMAGE Die 451 Base 2774 TargetA 22/15 RadiusIndex_0 13;
	// eff1 6 APPLY_AURA aura 33 MOD_DECREASE_SPEED Die 1 Base -51
	// TargetA 22/15 RadiusIndex_0 13; eff2 98 KNOCK_BACK Die 1 Base 79
	// MiscValue 100 TargetA 22/15 RadiusIndex_0 13 (BonusCoefficient
	// 1 on eff0, not encoded).
	{id: 66044, durIdx: 35, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 451, basePoints: 2774, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -51, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13},
		{effect: effKnockBack, dieSides: 1, basePoints: 79, misc: 100, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13},
	}},
	// 67528 Healing Wave (grand champion shaman): castTimeIdx 14
	// (3000), durIdx 0, rangeIdx 5 (Long 0-40), school 0x8; single
	// effect 10 HEAL Die 2301 Base 18849 TargetA 21 (BonusCoefficient
	// 1, not encoded).
	{id: 67528, castTimeIdx: 14, rangeIdx: 5, school: schoolNature, effects: [3]effect{{effect: effHeal, dieSides: 2301, basePoints: 18849, targetA: targetUnitTargetAlly}}},
	// 67529 Chain Lightning (grand champion shaman): castTimeIdx 16
	// (1500), durIdx 0, rangeIdx 4 (Medium 0-30), school 0x8; single
	// effect 2 SCHOOL_DAMAGE Die 575 Base 4712 TargetA 6 ChainTargets
	// 3 ChainAmplitude 0.7 (float32 0.69999998808 as read).
	{id: 67529, castTimeIdx: 16, rangeIdx: 4, school: schoolNature, effects: [3]effect{{effect: effSchoolDamage, dieSides: 575, basePoints: 4712, targetA: targetEnemy, chainTargets: 3, chainAmplitude: 0.7}}},
	// 67530 Earth Shield (grand champion shaman): castTimeIdx 1,
	// durIdx 4 (120000), rangeIdx 5 (Long 0-40), school 0x8; single
	// effect 6 APPLY_AURA aura 4 DUMMY Die 1 Base -1 TriggerSpell
	// 67537 TargetA 21 (BonusCoefficient 1, not encoded; 67537
	// unseeded — trigger dormant).
	{id: 67530, durIdx: 4, rangeIdx: 5, school: schoolNature, effects: [3]effect{{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, trigger: 67537, targetA: targetUnitTargetAlly}}},
	// 67534 Hex of Mending (grand champion shaman): castTimeIdx 16
	// (1500), durIdx 1 (10000), rangeIdx 4 (Medium 0-30), school 0x8;
	// single effect 6 APPLY_AURA aura 4 DUMMY TriggerSpell 67535
	// TargetA 6 (67535 unseeded — trigger dormant).
	{id: 67534, castTimeIdx: 16, durIdx: 1, rangeIdx: 4, school: schoolNature, effects: [3]effect{{effect: effApplyAura, aura: auraDummy, trigger: 67535, targetA: targetEnemy}}},
	// 65868 Shoot (grand champion hunter): castTimeIdx 90 (1700),
	// durIdx 0, rangeIdx 36 (Longer 0-45), school 0x1; single effect 2
	// SCHOOL_DAMAGE Die 1951 Base 5524 TargetA 6.
	{id: 65868, castTimeIdx: 90, rangeIdx: 36, school: schoolPhysical, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1951, basePoints: 5524, targetA: targetEnemy}}},
	// 66081 Multi-Shot (grand champion hunter): castTimeIdx 1, durIdx
	// 0, rangeIdx 54 (5-30 yd), school 0x1; single effect 31
	// WEAPON_PERCENT_DAMAGE Die 1 Base 99 TargetA 6 ChainTargets 3.
	{id: 66081, rangeIdx: 54, school: schoolPhysical, effects: [3]effect{{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 99, targetA: targetEnemy, chainTargets: 3}}},
	// 66083 Lightning Arrows (grand champion hunter): castTimeIdx 1,
	// durIdx 27 (3000), rangeIdx 1 (Self Only), school 0x8; single
	// effect 6 APPLY_AURA aura 23 PERIODIC_TRIGGER_SPELL Die 1 Base
	// -1 AuraPeriod 500 TriggerSpell 66084 TargetA 1 (66084 unseeded
	// — trigger dormant).
	{id: 66083, durIdx: 27, rangeIdx: 1, school: schoolNature, effects: [3]effect{{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 500, trigger: 66084, targetA: targetCaster}}},
	// 67701 Poison Bottle (grand champion rogue): castTimeIdx 1,
	// durIdx 0, rangeIdx 4 (Medium 0-30), school 0x8; single effect 32
	// TRIGGER_MISSILE Die 1 Base -1 TriggerSpell 67594 TargetA 53
	// (67594 unseeded — trigger dormant).
	{id: 67701, rangeIdx: 4, school: schoolNature, effects: [3]effect{{effect: effTriggerMiss, dieSides: 1, basePoints: -1, trigger: 67594, targetA: targetDestTargetEnemy}}},
	// 67706 Fan of Knives (grand champion rogue): castTimeIdx 1,
	// durIdx 0, rangeIdx 1 (Self Only), school 0x1; eff0 31
	// WEAPON_PERCENT_DAMAGE Die 1 Base 99 TargetA 18/16
	// RadiusIndex_0 14 RadiusIndex_1 14 (seed stores _0 only); eff1 77
	// SCRIPT_EFFECT TargetA 18/16 RadiusIndex_0 14.
	{id: 67706, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 99, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 14},
		{effect: effScriptEffect, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 14},
	}},
	// 66045 Haste (grand champion mage): castTimeIdx 1, durIdx 1
	// (10000), rangeIdx 5 (Long 0-40), school 0x40; eff0 6 APPLY_AURA
	// aura 193 MELEE_SLOW Die 1 Base 99 TargetA 21; eff1 6 APPLY_AURA
	// aura 61 MOD_SCALE Die 1 Base 29 TargetA 21; eff2 6 APPLY_AURA
	// aura 31 MOD_INCREASE_SPEED Die 1 Base 99 TargetA 21.
	{id: 66045, durIdx: 1, rangeIdx: 5, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraMeleeSlow, dieSides: 1, basePoints: 99, targetA: targetUnitTargetAlly},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 29, targetA: targetUnitTargetAlly},
		{effect: effApplyAura, aura: auraModIncreaseSpeed, dieSides: 1, basePoints: 99, targetA: targetUnitTargetAlly},
	}},
	// 67709 Eviscerate (grand champion rogue): castTimeIdx 1, durIdx 0,
	// rangeIdx 2 (Combat 0-5), school 0x1; single effect 2
	// SCHOOL_DAMAGE Die 1151 Base 9424 TargetA 6.
	{id: 67709, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1151, basePoints: 9424, targetA: targetEnemy}}},
	{id: 42442, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetSrcCaster, targetB: targetUnitCasterAreaParty, radiusIdx: 30}}},
	// 57496 Insanity (herald volazj): castTimeIdx 6 (5000), durIdx 0,
	// rangeIdx 1 (Self Only), school 0x1; single effect 3 DUMMY
	// TargetA 22/15 RadiusIndex_0 28 ChainAmplitude 1.
	{id: 57496, castTimeIdx: 6, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effDummy, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 28, chainAmplitude: 1}}},
	// 57507 Summon Twisted Visage Spawn Effect (herald volazj):
	// castTimeIdx 1, durIdx 21 (-1), rangeIdx 13 (Anywhere), school
	// 0x20; eff0 6 APPLY_AURA aura 247 CLONE_CASTER TargetA 25; eff1
	// 6 APPLY_AURA aura 279 INITIALIZE_IMAGES TargetA 25.
	{id: 57507, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraCloneCaster, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraInitializeImages, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 57508 Insanity (phasing 1): castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 1, school 0x20; single effect 6 APPLY_AURA aura 261
	// PHASE MiscValue 16 MiscValueB 173 TargetA 1.
	{id: 57508, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{{effect: effApplyAura, aura: auraPhase, misc: 16, miscB: 173, targetA: targetCaster, chainAmplitude: 1}}},
	// 57509 Insanity (phasing 2): castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 1, school 0x20; single effect 6 APPLY_AURA aura 261
	// PHASE MiscValue 32 MiscValueB 174 TargetA 1.
	{id: 57509, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{{effect: effApplyAura, aura: auraPhase, misc: 32, miscB: 174, targetA: targetCaster, chainAmplitude: 1}}},
	// 57510 Insanity (phasing 3): castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 1, school 0x20; single effect 6 APPLY_AURA aura 261
	// PHASE MiscValue 64 MiscValueB 175 TargetA 1.
	{id: 57510, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{{effect: effApplyAura, aura: auraPhase, misc: 64, miscB: 175, targetA: targetCaster, chainAmplitude: 1}}},
	// 57511 Insanity (phasing 4): castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 1, school 0x20; single effect 6 APPLY_AURA aura 261
	// PHASE MiscValue 128 MiscValueB 176 TargetA 1.
	{id: 57511, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{{effect: effApplyAura, aura: auraPhase, misc: 128, miscB: 176, targetA: targetCaster, chainAmplitude: 1}}},
	// 57512 Insanity (phasing 5): castTimeIdx 1, durIdx 21 (-1),
	// rangeIdx 1, school 0x20; single effect 6 APPLY_AURA aura 261
	// PHASE MiscValue 256 MiscValueB 177 TargetA 1.
	{id: 57512, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{{effect: effApplyAura, aura: auraPhase, misc: 256, miscB: 177, targetA: targetCaster, chainAmplitude: 1}}},
	// 57561 Insanity Channel Visual (herald volazj): castTimeIdx 1,
	// durIdx 21 (-1), rangeIdx 1, school 0x1; single effect 6
	// APPLY_AURA aura 4 DUMMY TargetA 1.
	{id: 57561, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1}}},
	// 41342 Shared Rule (lady malande): castTimeIdx 1 (instant),
	// durIdx 0 (no duration row), rangeIdx 13 (Anywhere - Unlimited,
	// 50000), school 0x1; single effect 2 SCHOOL_DAMAGE DieSides 1
	// BasePoints 0 TargetA 38 TARGET_UNIT_NEARBY_ENTRY ChainAmplitude 1
	// (wago.tools 3.4.0.43659, browser task eb1fb12e, 2026-11-15 03:39 UTC).
	{id: 41342, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{{effect: effSchoolDamage, dieSides: 1, basePoints: 0, targetA: targetUnitNearbyEntry, chainAmplitude: 1}}},
	// Frost-seed batch (wago.tools 3.4.0.43659, hotfixes OFF, browser
	// task 3a7b6385 verdict 2026-11-15): max-rank + low-rank mage
	// frost ranks that feed the Viscidus freeze machine
	// (boss_viscidus.cpp:156 frost-school SpellHit gate, now bridged
	// by creature:GetSpellSchoolMask) + the Viscidus self-auras.
	// ManaCost is SpellPower.ManaCost — no SpellPower row was in the
	// verdict, so cost 0 (Aran precedent). RealPointsPerLevel is not
	// carried by the seed struct (all 154 prior rows omit it).
	// CastTimeIndex 1 = instant (41342 precedent, wago-literal).
	// 42841 is Frostbolt Rank 15, NOT Rank 16 (42842 is Rank 16,
	// seeded below in the follow-up batch). 42920 is Arcane
	// Explosion Rank 9, NOT Blizzard (42921 is Arcane Explosion
	// Rank 10); the mage Blizzard Rank 8 ID is 42937 damage /
	// 42939 channel (seeded below in the follow-up batch). 31266
	// is already seeded above
	// (verified against this same verdict: eff0 27/3/101/949/2000/
	// 16/13, eff1 27/31/-71/13, durIdx 1, rangeIdx 4, castTimeIdx 4
	// — byte-identical to the verdict, no change).
	// 116 Frostbolt Rank 1: castTimeIdx 16 (1500), durIdx 28
	// (5000), rangeIdx 4 (30 yd), school 0x10; family Mage (3),
	// flags 32/0/0/0; eff0 6 APPLY_AURA aura 33 MOD_DECREASE_SPEED
	// Die 1 Base -41 Mechanic 11 TargetA 6; eff1 2 SCHOOL_DAMAGE
	// Die 3 Base 17 TargetA 6 (18-20 dmg); eff2 6 APPLY_AURA aura
	// 118 MOD_HEALING_PCT Die 1 Base -1 Misc 127 TargetA 6.
	{id: 116, castTimeIdx: 16, durIdx: 28, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{32, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -41, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 3, basePoints: 17, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 25304 Frostbolt Rank 11: castTimeIdx 14 (3000), durIdx 105
	// (9000), rangeIdx 4; eff1 2 SCHOOL_DAMAGE Die 41 Base 514
	// (515-555 dmg); eff0/eff2 identical to 116.
	{id: 25304, castTimeIdx: 14, durIdx: 105, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{32, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -41, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 41, basePoints: 514, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 42841 Frostbolt Rank 15: castTimeIdx 14, durIdx 105,
	// rangeIdx 4; eff1 2 SCHOOL_DAMAGE Die 57 Base 701 (702-758).
	{id: 42841, castTimeIdx: 14, durIdx: 105, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{32, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -41, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 57, basePoints: 701, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 122 Frost Nova Rank 1: castTimeIdx 1, durIdx 31 (8000),
	// rangeIdx 1 (Self); flags 64/0/0/0; eff0 2 SCHOOL_DAMAGE Die
	// 3 Base 18 TargetA 22 TargetB 15 RadiusIndex_0 13 (10 yd)
	// (19-21 dmg); eff1 6 APPLY_AURA aura 26 MOD_ROOT Die 1 Base
	// -1 Mechanic 7 TargetA 22/15 RadiusIndex_0 13.
	{id: 122, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{64, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 3, basePoints: 18, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, mechanic: 7, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 27088 Frost Nova Rank 5: eff0 Die 31 Base 229 (230-260).
	{id: 27088, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{64, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 31, basePoints: 229, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, mechanic: 7, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 42917 Frost Nova Rank 6: eff0 Die 51 Base 364 (365-415).
	{id: 42917, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{64, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 51, basePoints: 364, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, mechanic: 7, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 120 Cone of Cold Rank 1: castTimeIdx 1, durIdx 31, rangeIdx
	// 1; flags 512/0/0/0; eff0 6 APPLY_AURA aura 33 Die 1 Base -51
	// Mechanic 11 TargetA 104 RadiusIndex_0 13 (-50%); eff1 2
	// SCHOOL_DAMAGE Die 11 Base 97 TargetA 104 RadiusIndex_0 13
	// (98-108 dmg); eff2 6 APPLY_AURA aura 118 Die 1 Base -1
	// Misc 127 TargetA 104 RadiusIndex_0 13.
	{id: 120, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{512, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -51, mechanic: 11, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 11, basePoints: 97, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 27087 Cone of Cold Rank 6: eff1 Die 39 Base 409 (410-448).
	{id: 27087, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{512, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -51, mechanic: 11, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 39, basePoints: 409, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 42931 Cone of Cold Rank 8: eff1 Die 67 Base 706 (707-773).
	{id: 42931, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{512, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -51, mechanic: 11, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 67, basePoints: 706, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetUnitConeEnemy104, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 27085 Blizzard Rank 7: castTimeIdx 1, durIdx 31, rangeIdx 4;
	// flags 524416/0/0/0; eff0 27 PERSISTENT_AREA_AURA aura 4
	// DUMMY Die 1 Base 183 TargetA 28 RadiusIndex_0 14 (8 yd,
	// RadiusIndex_1 14 recorded, seed carries _0 per convention);
	// eff1 6 APPLY_AURA aura 23 PERIODIC_TRIGGER_SPELL
	// AuraPeriod 1000 Base -1 Die 1 Trigger 42198 TargetA 1.
	// Trigger 42198 is seeded below in the follow-up batch
	// (browser task 1e814993 verdict).
	{id: 27085, castTimeIdx: 1, durIdx: 31, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{524416, 0, 0}, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraDummy, dieSides: 1, basePoints: 183, targetA: targetDestDynobjEnemy, radiusIdx: 14, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, dieSides: 1, basePoints: -1, trigger: 42198, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 47610 Frostfire Bolt Rank 2: castTimeIdx 14, durIdx 105,
	// rangeIdx 5 (40 yd); school 0x14 (Frost+Fire — counts as
	// frost under the viscidus gate's mask & 0x10); flags
	// 0/4096/8/0; eff0 6 APPLY_AURA aura 33 Die 1 Base -41
	// Mechanic 11 TargetA 6; eff1 2 SCHOOL_DAMAGE Die 117 Base
	// 721 TargetA 6 (722-838 dmg); eff2 6 APPLY_AURA aura 3
	// PERIODIC_DAMAGE AuraPeriod 3000 Die 1 Base 29 TargetA 6.
	{id: 47610, castTimeIdx: 14, durIdx: 105, rangeIdx: 5, school: schoolFrostfire, family: familyMage, familyFlags: [3]uint32{0, 4096, 8}, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -41, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 117, basePoints: 721, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 1, basePoints: 29, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 25937 Viscidus Freeze: castTimeIdx 1, durIdx 8 (15000),
	// rangeIdx 1 (Self), school 0x10; no SpellClassOptions row
	// (family/flags 0); eff0 6 APPLY_AURA aura 12 MOD_STUN Die 0
	// Base 0 TargetA 1; eff1 6 APPLY_AURA aura 4 DUMMY Die 0
	// Base 0 TargetA 1.
	{id: 25937, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 26034 Viscidus Slowed: castTimeIdx 1, durIdx 21
	// (indefinite), rangeIdx 1, school 0x1 (Physical — the slow
	// auras are physical-school, not frost); no SpellClassOptions
	// row; eff0 6 APPLY_AURA aura 138 MOD_MELEE_HASTE Die 1 Base
	// -19 TargetA 1 (-18%); eff1 6 APPLY_AURA aura 33 Die 1 Base
	// -16 TargetA 1 (-15%).
	{id: 26034, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: -19, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -16, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 26036 Viscidus Slowed More: eff0 Base -44 (-43%), eff1
	// Base -31 (-30%); otherwise identical to 26034.
	{id: 26036, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: -44, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -31, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 25938 Viscidus Explode Trigger: castTimeIdx 1, durIdx 0
	// (none), rangeIdx 1, school 0x1; no SpellClassOptions row;
	// single effect 3 DUMMY TargetA 1 (the explode leg itself
	// stays out — glob machine unported — so this row only
	// documents the trigger; no cast site consumes it yet).
	{id: 25938, castTimeIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// Follow-up batch (wago.tools 3.4.0.43659, hotfixes OFF, browser
	// task 1e814993 verdict 2026-11-15 04:04 EST): 31075 Burning
	// Straw (Strawman fire-school SpellHit leg, bosses_opera.cpp:375
	// — 1-in-10 triggered self-cast, dormant until this row), 42842
	// Frostbolt Rank 16, 42198 Blizzard Rank 7 damage + 42937/42939
	// Blizzard Rank 8 damage/channel (Rank 8 ID determined this
	// verdict: 42937 damage, 42939 channel triggering 42937).
	// ChainAmplitude 1 on every defined effect of all 20 frost /
	// follow-up rows (wago.tools 3.4.0.43659 SpellEffect CSV,
	// browser task c2753d33 verdict 2026-11-15; DifficultyID 0
	// only; chainTargets 0, so the field is inert at runtime).
	// RealPointsPerLevel not carried by the seed struct.
	// 31075 Burning Straw: castTimeIdx 1, durIdx 32, rangeIdx 1,
	// school 0x4 (Fire); no SpellClassOptions row (family/flags 0),
	// no SpellPower row (manaCost 0); eff0 6 APPLY_AURA aura 5
	// MOD_CONFUSE Die 0 Base 0 TargetA 1 (TARGET_UNIT_CASTER).
	{id: 31075, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraModConfuse, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 42842 Frostbolt Rank 16: castTimeIdx 14, durIdx 105,
	// rangeIdx 4; flags 32/0/0/0; SpellPower ManaCost 0,
	// PowerCostPct 11 (manaCostPct 11, 29963 precedent); eff0/eff2
	// identical to 42841; eff1 2 SCHOOL_DAMAGE Die 63 Base 798
	// (799-861 dmg).
	{id: 42842, castTimeIdx: 14, durIdx: 105, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{32, 0, 0}, manaCostPct: 11, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -41, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 63, basePoints: 798, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 42198 Blizzard Rank 7 damage (per-wave component; channel
	// 27085 eff1 triggers it, AuraPeriod 1000): castTimeIdx 1,
	// durIdx 0, rangeIdx 135; flags 524416/0/0/0; no SpellPower
	// row; eff0 2 SCHOOL_DAMAGE Die 1 Base 272 TargetA 76
	// TARGET_DEST_CHANNEL_TARGET TargetB 16 RadiusIndex_0 14.
	{id: 42198, castTimeIdx: 1, rangeIdx: 135, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{524416, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 272, targetA: targetDestChannelTarget, targetB: targetUnitDestAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
	}},
	// 42937 Blizzard Rank 8 damage: as 42198, eff0 Base 349.
	{id: 42937, castTimeIdx: 1, rangeIdx: 135, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{524416, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 349, targetA: targetDestChannelTarget, targetB: targetUnitDestAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
	}},
	// 42939 Blizzard Rank 8 channel: castTimeIdx 1, durIdx 31,
	// rangeIdx 4; flags 524416/0/0/0; SpellPower ManaCost 0,
	// PowerCostPct 74 (manaCostPct 74); eff0 27
	// PERSISTENT_AREA_AURA aura 4 DUMMY Die 1 Base 297 TargetA 28
	// RadiusIndex_0 14; eff1 6 APPLY_AURA aura 23
	// PERIODIC_TRIGGER_SPELL AuraPeriod 1000 Base -1 Die 1
	// Trigger 42937 TargetA 1 (mirrors 27085/42198 Rank 7 pair).
	{id: 42939, castTimeIdx: 1, durIdx: 31, rangeIdx: 4, school: schoolFrost, family: familyMage, familyFlags: [3]uint32{524416, 0, 0}, manaCostPct: 74, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraDummy, dieSides: 1, basePoints: 297, targetA: targetDestDynobjEnemy, radiusIdx: 14, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, dieSides: 1, basePoints: -1, trigger: 42937, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 49519 Heroic Image Channel (wago.tools 3.4.0.43659,
	// hotfixes OFF, browser verdict 2026-11-15 05:16 EST):
	// castTimeIdx 1 (instant), durIdx 9 (30000 ms), rangeIdx 13
	// (Anywhere - Unlimited, 0-50000); school 0x1 Physical; no
	// SpellPower row (no mana cost), no SpellCooldowns row, no
	// SpellCategories row (dispel/mechanic 0), no SpellClassOptions
	// row (family/flags 0); single eff0 6 APPLY_AURA aura 4 DUMMY
	// Die 0 Base 0 TargetA 38 TARGET_UNIT_NEARBY_ENTRY,
	// chainAmplitude 1. NOT carried by the seed struct (same as
	// all seeded rows): SpellMisc Attributes 0x100/0x4404/0x4/0/
	// 0x80, SpellAuraOptions ProcChance 101, SpellTargetRestrictions
	// ConeDegrees 60. Cast by the eligor image
	// (npc_commander_eligor_dawnbringer.lua:236); the DUMMY aura
	// has no handler, so the cast stays dormant until one lands.
	{id: 49519, castTimeIdx: 1, durIdx: 9, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 50476/50495/50496 Summon Menagerie (wago.tools 3.4.0.43659,
	// browser verdict 2026-11-15 05:43 EST): castTimeIdx 15 (4000 ms),
	// durIdx 0 (no SpellDuration row), rangeIdx 5 (Long Range, 0-40),
	// school 0x40 Arcane; single eff0 5 TELEPORT_UNITS Die 1 Base -1
	// TargetA 1 TARGET_UNIT_CASTER TargetB 17 TARGET_DEST_DB,
	// chainAmplitude 1. NOT carried by the seed struct (same as all
	// seeded rows): SpellEffect Pos_facing (0.0035 / 4.2588 / 4.2588 —
	// the DB-dest facing), SpellMisc Attributes all 0,
	// SpellAuraOptions ProcChance 101. The teleport dest rides
	// TARGET_DEST_DB (spell_target_position in the deployment world
	// DB); the ev14 SpellHit trigger fires on the hit regardless, so
	// the platform cycle advances even when the dest row is absent.
	{id: 50476, castTimeIdx: 15, rangeIdx: 5, school: schoolArcane, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, basePoints: -1, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	{id: 50495, castTimeIdx: 15, rangeIdx: 5, school: schoolArcane, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, basePoints: -1, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	{id: 50496, castTimeIdx: 15, rangeIdx: 5, school: schoolArcane, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, basePoints: -1, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 51112 Teleport (wago.tools 3.4.0.43659, 2026-11-15 05:43 EST):
	// castTimeIdx 4 (1000 ms), durIdx 0, rangeIdx 1 (Self Only, 0-0),
	// school 0x40 Arcane; single eff0 5 TELEPORT_UNITS Die 0 Base 0
	// TargetA 1 TargetB 17, chainAmplitude 1. Pos_facing 4.7997 not
	// carried (see 50476 note); Attributes all 0, ProcChance 101 not
	// carried. Teleports the caster to the center of The Oculus.
	{id: 51112, castTimeIdx: 4, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effTeleportUnits, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 51110 Empowered Arcane Explosion (wago.tools 3.4.0.43659,
	// 2026-11-15 05:43 EST): castTimeIdx 170 (8000 ms), durIdx 0,
	// rangeIdx 1 (Self Only), school 0x40 Arcane, dispel 1; single
	// eff0 2 SCHOOL_DAMAGE Die 1 Base 8999 TargetA 22
	// TARGET_SRC_CASTER TargetB 15 TARGET_UNIT_SRC_AREA_ENEMY
	// RadiusIndex_0 33 (99 yd), chainAmplitude 1. NOT carried:
	// SpellMisc Attributes_0 136, SpellCooldowns StartRecoveryTime
	// 1500, BonusCoefficient 1, ProcChance 101.
	{id: 51110, castTimeIdx: 170, rangeIdx: 1, school: schoolArcane, dispel: 1, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 8999, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 33, chainAmplitude: 1},
	}},
	// 51121 Time Bomb (wago.tools 3.4.0.43659, 2026-11-15 05:43 EST):
	// castTimeIdx 4 (1000 ms), durIdx 32 (6000 ms), rangeIdx 5
	// (Long Range), school 0x40 Arcane; eff0 2 SCHOOL_DAMAGE Die 401
	// Base 3799 TargetA 6 TARGET_UNIT_TARGET_ENEMY, chainAmplitude 1;
	// eff1 6 APPLY_AURA aura 23 (PERIODIC_TRIGGER_SPELL per C++
	// SpellAuraDefines.h:103 — the browser verdict's PERIODIC_DUMMY
	// label is wrong; 226 is PERIODIC_DUMMY) AuraPeriod 6000 Die 1
	// Base 0 TargetA 6, chainAmplitude 1. The delayed AoE leg rides
	// that aura with TriggerSpell 0 in the DBC, so it has no Go model
	// — the direct damage is the carried leg. NOT carried: SpellMisc
	// Attributes_3 256, BonusCoefficient 1, ProcChance 101.
	{id: 51121, castTimeIdx: 4, durIdx: 32, rangeIdx: 5, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 401, basePoints: 3799, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 6000, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 51602 Evocation (wago.tools 3.4.0.43659, 2026-11-15 05:43 EST;
	// wago SpellName is "Evocation", the boss_urom.lua SPELL_EVOCATE
	// label is the C++ enum name): castTimeIdx 1 (instant), durIdx 21
	// (-1), rangeIdx 1 (Self Only), school 0x40 Arcane; single eff0
	// 6 APPLY_AURA aura 4 DUMMY Die 1 Base 0 TargetA 1,
	// chainAmplitude 1. NOT carried: SpellMisc Attributes
	// 256/268435520/0/268435456, ProcChance 101.
	{id: 51602, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 51103 Frostbomb (wago.tools 3.4.0.43659, 2026-11-15 05:43 EST):
	// castTimeIdx 16 (1500 ms), durIdx 3 (60000 ms), rangeIdx 5
	// (Long Range), school 0x10 Frost, manaCost 90 (SpellPower row);
	// eff0 2 SCHOOL_DAMAGE Die 1 Base 649 TargetA 53
	// TARGET_DEST_TARGET_ENEMY TargetB 16 TARGET_UNIT_DEST_AREA_ENEMY
	// RadiusIndex_0 18 (15 yd); eff1 27 PERSISTENT_AREA_AURA aura 3
	// PERIODIC_DAMAGE AuraPeriod 1000 Die 1 Base 649 TargetA 53
	// TargetB 28 TARGET_DEST_DYNOBJ_ENEMY RadiusIndex_0 18; eff2 27
	// PERSISTENT_AREA_AURA aura 33 MOD_DECREASE_SPEED Die 1 Base -36
	// TargetA 53 TargetB 28 RadiusIndex_0 18; chainAmplitude 1 on
	// all three. NOT carried: SpellMisc Attributes_0 65536 /
	// Attributes_5 1073741824, BonusCoefficient 1, ProcChance 101.
	{id: 51103, castTimeIdx: 16, durIdx: 3, rangeIdx: 5, school: schoolFrost, manaCost: 90, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 649, targetA: targetDestTargetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, auraPeriod: 1000, dieSides: 1, basePoints: 649, targetA: targetDestTargetEnemy, targetB: targetDestDynobjEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraDecSpeed, dieSides: 1, basePoints: -36, targetA: targetDestTargetEnemy, targetB: targetDestDynobjEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 58025 Frost Buffet (wago.tools 3.4.0.43659, 2026-11-15 05:43
	// EST): castTimeIdx 1 (instant), durIdx 28 (5000 ms), rangeIdx 13
	// (Anywhere - Unlimited, 0-50000), school 0x10 Frost; single
	// eff0 6 APPLY_AURA aura 14 MOD_DAMAGE_TAKEN Die 1 Base 49
	// MiscValue_0 16 (Frost school mask) TargetA 6,
	// chainAmplitude 1. NOT carried: SpellAuraOptions
	// CumulativeAura 50, SpellMisc Attributes, ProcChance 101.
	{id: 58025, castTimeIdx: 1, durIdx: 28, rangeIdx: 13, school: schoolFrost, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 49, targetA: targetEnemy, misc: 16, chainAmplitude: 1},
	}},
	// 8613 Skinning (wago.tools 3.4.0.43659, 2026-11-15; Wowhead
	// WotLK cross-check: Skin Mob Value 1 + Learn Skill (Skinning)
	// Value 1, Range 5 yards (Interact), Cast time 1.5 s, School
	// Physical): castTimeIdx 16 (1500 ms), durIdx 0, RangeIndex 12
	// carried as synthetic 93002 (0-5 yd; real SpellRange 12 row
	// unverified), school 0x1; eff0 95 SKINNING Die 1 Base 0
	// TargetA 25 TARGET_UNIT_TARGET_ANY (EffectRealPointsPerLevel 5,
	// not encoded — the generator carries no such field and no Go
	// consumer reads the skinning value); eff1 118 SKILL Die 1 Base
	// 0 MiscValue_0 393 (Skinning) TargetA 0 NONE (BonusCoefficient
	// 1, not encoded). NOT carried: SpellMisc Attributes_0 0x10 /
	// Attributes_1 0x400. Unblocks the_beast ev14: HasSpellEffect
	// (8613, 95) now resolves.
	{id: 8613, castTimeIdx: 16, rangeIdx: 93002, school: schoolPhysical, effects: [3]effect{
		{effect: effSkinning, dieSides: 1, targetA: targetUnitTargetAny},
		{effect: effSkill, dieSides: 1, misc: 393},
	}},
	// 16710 Pip is Quickwit (wago.tools 3.4.0.43659, 2026-11-15;
	// Wowhead WotLK: "Pip is Quickwit", Summon (Pip Quickwit)
	// Radius 5 yd, Instant): castTimeIdx 1 (instant), durIdx 21
	// (-1), RangeIndex 12 (5 yd) carried as synthetic 93002 (see
	// 8613 note), school 0x1 Physical; single eff0 28 SUMMON Die 1
	// Base 0 MiscValue_0 10776 (Finkle Einhorn, TDB
	// creature_template 10776, level 60, faction 35, SmartAI)
	// MiscValueB 64 RadiusIndex_0 8 (5 yd) TargetA 32
	// TARGET_DEST_CASTER_SUMMON. C++ EffectSummon: 64 is on the
	// multi-summon list (SpellEffects.cpp) so count = damage-or-1
	// = 1; WILD + Flags 0x100 + Title 0 -> default TempSummon
	// branch, not guardian (see SummonProperties 64 note). NOT
	// carried: SpellMisc Attributes_0 0x800100.
	{id: 16710, castTimeIdx: 1, durIdx: 21, rangeIdx: 93002, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, misc: 10776, miscB: 64, radiusIdx: 8, targetA: targetDestCasterSummon},
	}},
	// 49549 Ice Beam (wago.tools 3.4.0.43659 SpellEffect rows
	// 953964/953965, 2026-11-15; Wowhead WotLK cross-check: Frost
	// damage + snare, cast 1.5 s, range 40 yd, duration 6 s, dispel
	// Magic, cost none): castTimeIdx 16 (1500 ms, real row),
	// durIdx 32 (6000 ms, real row), rangeIdx 5 (Long Range 0-40,
	// real row), school 0x10 Frost, dispel 1; eff0 2 SCHOOL_DAMAGE
	// Die 1547 Base 4380 TargetA 6 TARGET_UNIT_TARGET_ENEMY
	// chainAmplitude 1 (Wowhead value 4381 = base + 1); eff1 6
	// APPLY_AURA aura 33 MOD_DECREASE_SPEED Die 1 Base -61
	// Mechanic 11 (Snared) TargetA 6 chainAmplitude 1 (Wowhead
	// -60% = base + 1). No third effect exists (SpellEffect table
	// returns exactly 2 rows; Wowhead lists 2). NOT carried:
	// EffectBonusCoefficient 1/0, EffectMechanic 0 eff0,
	// SpellMisc Attributes, ProcChance 101. NOTE: wago Spell table
	// for this build exposes no SchoolMask/CastTime/Range/Duration
	// columns, so those indices are carried from the Wowhead
	// human-readable equivalents, not wago numerics. Unblocks one
	// of two varos ev15 blockers; the captain trigger machine
	// (51002 Send Event 12229, unseeded; no event_scripts executor)
	// still stays out.
	{id: 49549, castTimeIdx: 16, durIdx: 32, rangeIdx: 5, school: schoolFrost, dispel: 1, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1547, basePoints: 4380, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -61, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 53456 Impale (SpellName "Impale", not "Impale Aura"; the C++
	// SPELL_IMPALE_AURA constant keeps the Aura name in code) —
	// wago.tools 3.4.0.43659 SpellEffect row 958241 + SpellMisc
	// 605213 (2026-11-15 browser read): eff0 6 APPLY_AURA aura 23
	// PERIODIC_TRIGGER_SPELL Die 1 Base -1 TargetA 1
	// TARGET_UNIT_CASTER auraPeriod 10000 trigger 53457
	// chainAmplitude 1 (65096 precedent); castTimeIdx 1 (instant,
	// real row), durIdx 21 (-1 infinite, real row — 68415
	// precedent; the 45b49c2d Wowhead-derived durIdx 0 is
	// superseded by the verbatim SpellMisc read), rangeIdx 135
	// (Tower 100, 0-100, real row — SpellRange 135 added below;
	// the 45b49c2d rangeIdx 6 was the same numeric range under the
	// wrong index). NOT carried: EffectBonusCoefficient 0,
	// EffectMechanic 0, EffectAttributes 0x0. Removes one of two
	// anub_arak ev14 blockers; the SpellHit trigger cast itself
	// (boss's own EVENT_SUBMERGE, unported phase machine) still
	// stays out.
	{id: 53456, castTimeIdx: 1, durIdx: 21, rangeIdx: 135, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, targetA: targetCaster, auraPeriod: 10000, trigger: 53457, chainAmplitude: 1},
	}},
	// 53421 Submerge (wago.tools 3.4.0.43659 SpellEffect rows
	// 957978/957979 + SpellMisc 605029, 2026-11-15 browser read):
	// eff0 6 APPLY_AURA aura 12 MOD_STUN TargetA 1
	// TARGET_UNIT_CASTER; eff1 6 APPLY_AURA aura 56 TRANSFORM
	// MiscValue_0 24417 TargetA 1; castTimeIdx 5 (2000ms, real
	// row), durIdx 21 (-1 infinite, real row, 68415 precedent),
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical.
	// Dispel not carried: wago SpellMisc exposes no dispel column
	// in this build, left at the zero default. Removes the second
	// anub_arak ev14 unseeded-spell blocker; the SpellHit trigger
	// cast itself (boss's own EVENT_SUBMERGE, unported phase
	// machine) still stays out.
	{id: 53421, castTimeIdx: 5, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, misc: 24417, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 53599 Summon Darter / 53614 Summon Guardian / 53615 Summon
	// Venomancer (boss_anubarak.cpp:44-47, trigger-cast :219-263
	// inside the unported submerge phase machine) — wago.tools
	// 3.4.0.43659 SpellEffect rows 957930/958189/958190 + SpellMisc
	// 604995/605175/605176 (2026-11-15 browser read): single eff0
	// 28 SUMMON Die 1 Base 0 MiscValue = pet entry (29213/29216/
	// 29217) MiscValueB 64 TargetA 18 TARGET_DEST_CASTER;
	// DurationIndex 21 (-1, 68415 precedent), RangeIndex 1 (Self
	// Only), CastingTimeIndex 1 (instant), school 0x1 Physical.
	// Attributes_0 0x100 not encoded (seed has no attributes
	// field). Removes three of the four anub_arak ev4 unseeded-
	// spell blockers; the summon trigger cast itself (world
	// trigger inside the unported submerge machine) still stays out.
	{id: 53599, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 29213, miscB: 64, targetA: targetDestCaster}}},
	{id: 53614, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 29216, miscB: 64, targetA: targetDestCaster}}},
	{id: 53615, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 29217, miscB: 64, targetA: targetDestCaster}}},
	// 53609 Summon Assassin (boss_anubarak.cpp:45, trigger-cast
	// :230-231) — wago.tools 3.4.0.43659 SpellEffect row 958113 +
	// SpellMisc 605126 (2026-11-15 browser read): single eff0 140
	// FORCE_CAST Die 0 Base 0 TargetA 22 TARGET_SRC_CASTER
	// TargetB 7 TARGET_UNIT_SRC_AREA_ENTRY RadiusIndex 28
	// TriggerSpell 53610; DurationIndex 0 (no duration, left at
	// the zero default), RangeIndex 1 (Self Only), CastingTimeIndex
	// 1 (instant), school 0x1 Physical. Attributes_0 0x100 /
	// Attributes_2 0x4 not encoded (seed has no attributes field).
	// effForceCast constant added above (engine twin
	// spells.go:449, handler reads eff.TriggerSpell). The
	// triggered 53610 (SUMMON 29214) is seeded below from its own
	// wago read; the trigger cast itself (world trigger inside
	// the unported submerge machine) still stays out.
	{id: 53609, castTimeIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effForceCast, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 28, trigger: 53610}}},
	// 53610 Summon Assassin trigger (the spell 53609 force-casts) —
	// wago.tools 3.4.0.43659 SpellEffect row 958114 + SpellMisc
	// 605127 (2026-11-15 browser reads): single eff0 28 SUMMON
	// Die 1 Base 0 MiscValue 29214 (Nerubian Assassin)
	// MiscValueB 64 TargetA 18 TARGET_DEST_CASTER;
	// DurationIndex 21 (-1, 68415 precedent), RangeIndex 1 (Self
	// Only), CastingTimeIndex 1 (instant), school 0x1 Physical.
	// Attributes_0 0x100 not encoded (seed has no attributes
	// field). Completes the 53609 -> 53610 chain, so all four
	// anub_arak ev4 pet summon spells now resolve in the seed.
	{id: 53610, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{{effect: effSummon, dieSides: 1, misc: 29214, miscB: 64, targetA: targetDestCaster}}},
	// 2676 Pulverize (zone_terokkar_forest.cpp:47 enum, :153-157
	// DoCast(me) on a 9s timer; Lua npc_unkor_the_ruthless.lua:116
	// self-cast 9s) — wago.tools 3.4.0.43659 SpellEffect rows
	// 680971/870416 + SpellMisc 315891 + SpellCategories 71116
	// (2026-11-15 browser read): eff0 2 SCHOOL_DAMAGE Die 11
	// Base 69 TargetA 22 TARGET_SRC_CASTER TargetB 15
	// TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex 8 (5yd, real row)
	// chainAmplitude 1; eff1 68 INTERRUPT_CAST Die 0 Base 0
	// TargetA 22 TargetB 15 RadiusIndex 8 Mechanic 26
	// chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 39 (2000ms, real row — 57994 precedent), rangeIdx
	// 1 (Self Only, real row), school 0x1 Physical. NOT carried:
	// Attributes_0 0x90010 (seed has no attributes field);
	// ManaCost not reported by the wago read (left 0,
	// 49549/53421/53456 precedent). Both effects have live Go
	// handlers (school damage + creatureEffectInterruptCast,
	// boss_ai.go:3328). Removes the last unseeded-spell blocker
	// for npc_unkor_the_ruthless.lua (single-missing-id file).
	{id: 2676, castTimeIdx: 1, durIdx: 39, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 11, basePoints: 69, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effInterruptCast, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, mechanic: 26, chainAmplitude: 1},
	}},
	// 2767 Shadow Word: Pain (boss_interrogator_vishas.cpp:34
	// enum, :102-104 DoCastVictim on {5s,15s}; Lua
	// boss_interrogator_vishas.lua:96 victim cast {5s,15s}) —
	// wago.tools 3.4.0.43659 SpellEffect row 680965 + SpellMisc
	// 315885 + SpellCategories 71115 (2026-11-15 browser read):
	// eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE Die 1 Base 54
	// TargetA 6 TARGET_UNIT_TARGET_ENEMY TargetB 0 auraPeriod
	// 3000 chainAmplitude 1 (54+1 = 55/tick); castTimeIdx 1
	// (instant, real row), durIdx 85 (18s, real row added below
	// — ms from the Wowhead 18s display, wago SpellDuration
	// re-read pending), rangeIdx 4 (Medium Range 30yd, real
	// row), school 0x20 Shadow, dispel 1 (Magic). NOT carried:
	// Attributes_0/2/4/6 (seed has no attributes field);
	// ManaCost not reported by the wago read (left 0).
	// Removes the last unseeded-spell blocker for
	// boss_interrogator_vishas.lua (single-missing-id file).
	{id: 2767, castTimeIdx: 1, durIdx: 85, rangeIdx: 4, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 54, targetA: targetEnemy, auraPeriod: 3000, chainAmplitude: 1},
	}},
	// 1557 Full Speed / 1834 Gathering Speed / 1881 Summon
	// Hive'Zara Hatchling (boss_buru.cpp:34-38 enum; :154/158
	// self-casts, :229 triggered hatchling summon; Lua
	// boss_buru.lua:139/147/293) — wago.tools 3.4.0.43659
	// SpellEffect rows 680182/681033/679928 + SpellMisc
	// 315179/315947/314952 (2026-11-15 browser read): 1557
	// single eff0 6 APPLY_AURA aura 129 MOD_SPEED_ALWAYS
	// Base 299 Die 1 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1 (299+1 = 300% run speed); 1834
	// single eff0 6 APPLY_AURA aura 129 Base 24 Die 1
	// TargetA 1 chainAmplitude 1 (24+1 = 25%); 1881 single
	// eff0 28 SUMMON Die 1 Base 0 MiscValue 15521
	// (Hive'Zara Hatchling) MiscValueB 64 TargetA 32
	// TARGET_DEST_CASTER_SUMMON RadiusIndex 8 (5yd).
	// All three: castTimeIdx 1 (instant), rangeIdx 1 (Self
	// Only), school 0x1 Physical; 1557/1881 durIdx 21 (-1,
	// 68415 precedent), 1834 durIdx 25 (180000ms, 3 min,
	// real row added below). Dispel n/a (Wowhead), mechanic
	// 0. auraModSpeedAlways=129 constant added above
	// (engine twin movement_speed.go:16, run-speed set
	// includes 129). Removes the last unseeded-spell
	// blockers for boss_buru.lua (single-missing-id file in
	// the unseeded-live-cast census); all three effects
	// have live Go handlers.
	{id: 1557, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModSpeedAlways, dieSides: 1, basePoints: 299, targetA: targetCaster, chainAmplitude: 1},
	}},
	{id: 1834, castTimeIdx: 1, durIdx: 25, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModSpeedAlways, dieSides: 1, basePoints: 24, targetA: targetCaster, chainAmplitude: 1},
	}},
	{id: 1881, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, misc: 15521, miscB: 64, targetA: targetDestCasterSummon, radiusIdx: 8},
	}},
	// 15573 Fireblast (boss_ambassador_flamelash.cpp:24 enum, :70-72
	// DoCastVictim on a 2s init / 7s re-arm timer; Lua
	// boss_ambassador_flamelash.lua:63 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 691926 + SpellMisc 325212 +
	// SpellCategories 74493 (2026-11-15 browser read): single eff0
	// 6 APPLY_AURA aura 42 PROC_TRIGGER_SPELL Die 1 Base 0
	// TriggerSpell 15574 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 21 (-1, 68415 precedent), rangeIdx 1 (Self Only,
	// real row), school 0x4 Fire, dispel 1 (Magic). NOT carried:
	// EffectBonusCoefficient 0.1 (seed has no coefficient field);
	// SpellAuraOptions ProcChance 25 / ProcCategoryRecovery 4000
	// (seed has no SpellAuraOptions table) and the trigger spell
	// 15574 is now seeded below (follow-up wago read 2026-11-15). Removes the last
	// unseeded-spell blocker for boss_ambassador_flamelash.lua
	// (single-missing-id file in the unseeded-live-cast census).
	{id: 15573, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolFire, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, trigger: 15574, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 8269 Frenzy (boss_vectus.cpp:32 enum, :66/:95 DoCast(me) on
	// a 24s timer; Lua boss_vectus.lua:106/:172 self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows 686250/686251/686252
	// + SpellMisc 320508 + SpellCategories 72752 (2026-11-15
	// browser read): eff0 6 APPLY_AURA aura 13 MOD_DAMAGE_DONE
	// Die 1 Base 49 MiscValue 1 (Physical school mask) TargetA 1;
	// eff1 aura 138 MOD_MELEE_HASTE Base 59 TargetA 1; eff2 aura
	// 61 MOD_SCALE Base 14 TargetA 1; all chainAmplitude 1
	// (49+1 = 50% damage, 59+1 = 60% haste, 14+1 = 15% size per
	// Wowhead); castTimeIdx 1 (instant, real row), durIdx 4
	// (120000ms, 2 min, real row — 67530 precedent), rangeIdx 1
	// (Self Only, real row), school 0x1 Physical, dispel 0.
	// NOT carried: eff0 EffectRealPointsPerLevel 2 (seed has no
	// per-level field). auraModDamageDone=13 constant added above
	// (engine twin spells.go:689). Removes the last unseeded-spell
	// blocker for boss_vectus.lua (single-missing-id file).
	{id: 8269, castTimeIdx: 1, durIdx: 4, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageDone, dieSides: 1, basePoints: 49, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 59, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 14, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12470 Fire Nova (boss_mordresh_fire_eye.cpp:33 enum,
	// :117-119 DoCast(me) on {8s,12s} init / {11s,16s} re-arm;
	// Lua boss_mordresh_fire_eye.lua:155 self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 689767 + SpellMisc 323384 +
	// SpellCategories 73913 (2026-11-15 browser read): single
	// eff0 2 SCHOOL_DAMAGE Die 23 Base 147 TargetA 22
	// TARGET_SRC_CASTER TargetB 15 TARGET_UNIT_SRC_AREA_ENEMY
	// RadiusIndex 13 (10yd, real row) chainAmplitude 1 (147+1 ..
	// 147+23 = 148-170 damage per Wowhead); castTimeIdx 5
	// (2000ms, real row — 44196 precedent), durIdx 0, rangeIdx 1
	// (Self Only, real row), school 0x4 Fire, manaCost 30
	// (Wowhead). NOT carried: EffectBonusCoefficient 1 (neutral;
	// seed has no coefficient field). Removes the last
	// unseeded-spell blocker for boss_mordresh_fire_eye.lua
	// (single-missing-id file).
	{id: 12470, castTimeIdx: 5, rangeIdx: 1, school: schoolFire, manaCost: 30, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 23, basePoints: 147, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 31285 Vampiric Aura Heal (boss_anetheron.cpp:30 enum, :277
	// ValidateSpellInfo, :290 triggered CastSpell with
	// SPELLVALUE_BASE_POINT0 = damage * 3; Lua
	// boss_anetheron.lua:263 CastCustomSpell triggered) —
	// wago.tools 3.4.0.47659 SpellEffect row 849875 + SpellMisc
	// 448740 + SpellCategories 113032 (2026-11-15 browser read):
	// single eff0 10 HEAL Die 1 Base 0 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1 (the heal value comes
	// from the custom-cast base point, damage * 3);
	// castTimeIdx 1 (instant, real row), durIdx 0, rangeIdx 1
	// (Self Only, real row), school 0x20 Shadow, dispel 0.
	// NOT carried: EffectBonusCoefficient 1 (neutral; seed has no
	// coefficient field); SpellAuraOptions ProcChance 100 (seed
	// has no SpellAuraOptions table). The 38196 Vampiric Aura
	// itself is now seeded below (follow-up read 2026-11-15) — this row removes
	// the heal-side blocker only. Removes the last unseeded-spell
	// blocker for boss_anetheron.lua (single-missing-id file).
	{id: 31285, castTimeIdx: 1, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effHeal, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 14894 Learn Smelt (boss_tomb_of_seven.cpp:28 enum, :75
	// player->CastSpell(player, 14894, false) on gossip select;
	// Lua boss_gloomrel.lua:70 player cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 691854 + SpellMisc 325150
	// (2026-11-15 browser read): single eff0 36 LEARN_SPELL
	// Die 0 Base 0 TriggerSpell 14891 (Smelt Dark Iron)
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0, rangeIdx 6
	// (Vision Range 100yd, real row), school 0x1 Physical.
	// No SpellCategories row exists for 14894 (wago FK search
	// empty), so dispel stays 0. effLearnSpell=36 constant added
	// above (engine twin spells.go:395, handler :5096/:18410 —
	// teaches by TriggerSpell ID, so 14891 needs no seed row).
	// Removes the last unseeded-spell blocker for
	// boss_gloomrel.lua (single-missing-id file).
	{id: 14894, castTimeIdx: 1, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effLearnSpell, trigger: 14891, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 1106 Shadow Bolt (boss_bloodmage_thalnos.cpp:32 enum, :91-92
	// DoCastVictim on a 2s init / 2s re-arm timer; Lua
	// boss_bloodmage_thalnos.lua:111 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 680338 + SpellMisc 315311
	// (2026-11-15 browser read): single eff0 2 SCHOOL_DAMAGE Die
	// 21 Base 141 TargetA 6 TARGET_UNIT_TARGET_ENEMY
	// chainAmplitude 1 (141+1 .. 141+21 damage); castTimeIdx 14
	// (3000ms, real row — 67528 precedent), durIdx 0, rangeIdx 4
	// (Medium Range 0-30yd, real row — 54690 precedent), school
	// 0x20 Shadow. NOT carried: EffectBonusCoefficient 0.857 /
	// EffectRealPointsPerLevel 1.6 / Speed 20 (seed has no
	// coefficient or speed field); no Dispel column in this
	// build's SpellMisc, so dispel stays 0. Thalnos still casts
	// 8814 Flame Spike + 16079 Fire Nova unseeded (follow-up read).
	{id: 1106, castTimeIdx: 14, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 21, basePoints: 141, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 8053 Flame Shock (boss_bloodmage_thalnos.cpp:31 enum, :87-88
	// DoCastVictim on a 10s init / {10s,15s} re-arm timer; Lua
	// boss_bloodmage_thalnos.lua:93 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows 686555/686556 + SpellMisc
	// 320746 (2026-11-15 browser read): eff0 2 SCHOOL_DAMAGE
	// Die 1 Base 85 TargetA 6; eff1 6 APPLY_AURA aura 3
	// PERIODIC_DAMAGE auraPeriod 3000 Die 1 Base 23 TargetA 6
	// (85 direct + 23 per 3s tick); castTimeIdx 1 (instant, real
	// row), durIdx 85 (18000ms, real row — 2767 precedent, same
	// DurationIndex), rangeIdx 34 (0-25yd, real row — 57994
	// precedent), school 0x4 Fire. NOT carried:
	// EffectBonusCoefficient 0.214/0.1 / RealPointsPerLevel 1.7
	// (seed has no coefficient field); no Dispel column in this
	// build's SpellMisc, so dispel stays 0. Engine twins:
	// effSchoolDamage + auraPeriodicDmg live handlers.
	{id: 8053, castTimeIdx: 1, durIdx: 85, rangeIdx: 34, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 85, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 23, auraPeriod: 3000, targetA: targetEnemy},
	}},
	// 8814 Flame Spike (boss_bloodmage_thalnos.cpp:33 enum, :95-96
	// DoCastVictim on an 8s init / 30s re-arm timer; Lua
	// boss_bloodmage_thalnos.lua:129 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows 687694/687695 + SpellMisc
	// 321667 (2026-11-15 browser read): eff0 2 SCHOOL_DAMAGE
	// Die 15 Base 92 TargetA 16 TARGET_UNIT_DEST_AREA_ENEMY
	// RadiusIndex 8 (5yd, real row) chainAmplitude 1 (92+1 ..
	// 92+15 direct); eff1 27 PERSISTENT_AREA_AURA aura 3
	// PERIODIC_DAMAGE auraPeriod 3000 Die 1 Base 49 TargetA 16
	// RadiusIndex 8 chainAmplitude 1 (49 per 3s tick in the
	// ground zone); castTimeIdx 14 (3000ms, real row — 67528
	// precedent), durIdx 105 (9000ms, real row — 31263
	// precedent), rangeIdx 4 (Medium Range 0-30yd, real row —
	// 54690 precedent), school 0x4 Fire. NOT carried:
	// EffectBonusCoefficient 1 / RealPointsPerLevel 6/1.6 (seed
	// has no coefficient field); no Dispel column in this
	// build's SpellMisc, so dispel stays 0. Engine twin:
	// effPersistentAreaAura live in the seed (e.g. :794/:1020).
	{id: 8814, castTimeIdx: 14, durIdx: 105, rangeIdx: 4, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 15, basePoints: 92, targetA: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 49, auraPeriod: 3000, targetA: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 16079 Fire Nova (boss_bloodmage_thalnos.cpp:34 enum, :99-100
	// DoCastVictim on a 40s init / 40s re-arm timer; Lua
	// boss_bloodmage_thalnos.lua:147 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 693277 + SpellMisc 326330
	// (2026-11-15 browser read): single eff0 2 SCHOOL_DAMAGE
	// Die 47 Base 294 TargetA 22 TARGET_SRC_CASTER TargetB 15
	// TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex 13 (10yd, real
	// row) chainAmplitude 1 (294+1 .. 294+47 to all enemies
	// around the caster); castTimeIdx 1 (instant, real row),
	// durIdx 0, rangeIdx 1 (Self Only, real row), school 0x4
	// Fire. NOT carried: EffectBonusCoefficient 1 (neutral;
	// seed has no coefficient field); no Dispel column in this
	// build's SpellMisc, so dispel stays 0. Removes the last
	// unseeded-spell blocker for boss_bloodmage_thalnos.lua
	// (all four timer casts 1106/8053/8814/16079 now seeded).
	{id: 16079, castTimeIdx: 1, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 47, basePoints: 294, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 15574 Fire Blast (boss_ambassador_flamelash.cpp:24 enum,
	// :70-72 DoCastVictim 15573 on a 2s init / 7s re-arm timer; the
	// 15573 aura 42 PROC_TRIGGER_SPELL fires this spell on proc;
	// C++ names only 15573 — 15574 is the aura's TriggerSpell per
	// wago) — wago.tools 3.4.0.47659 SpellEffect row 691927 +
	// SpellMisc 325213 + SpellCategories 74494 + SpellClassOptions
	// 45137 (2026-11-15 browser read): single eff0 2
	// SCHOOL_DAMAGE Die 41 Base 219 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1 (220-260 damage);
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration
	// row 0 exists in this build), rangeIdx 3 (Twenty yards, real
	// row added below), school 0x4 Fire, dispel 0
	// (SpellCategories 74494). NOT carried: EffectBonusCoefficient
	// 0 / SpellClassSet 3 Mage + class mask 131074 (seed has no
	// class fields); no SpellPower row (no mana cost), no
	// SpellAuraOptions row. Engine twin: effSchoolDamage live
	// handler; proc-trigger aura 42 fires it (procs.go:1287).
	// Completes the 15573 proc leg for
	// boss_ambassador_flamelash.lua.
	{id: 15574, castTimeIdx: 1, rangeIdx: 3, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 41, basePoints: 219, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 38196 Vampiric Aura (boss_anetheron.cpp:29 enum; Lua
	// boss_anetheron.lua:117 self-cast, triggered, 5s init ->
	// {10s,20s} re-arm) — wago.tools 3.4.0.47659 SpellEffect row
	// 858697 + SpellMisc 455942 + SpellCategories 115683
	// (2026-11-15 browser read): single eff0 6 APPLY_AURA aura 4
	// DUMMY Die 0 Base 0 TargetA 22 TARGET_SRC_CASTER TargetB 30
	// TARGET_UNIT_SRC_AREA_ALLY RadiusIndex 20 (25yd, real row
	// added below) chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 28 (5000ms, real row — 31308 precedent),
	// rangeIdx 1 (Self Only, real row), school 0x20 Shadow,
	// dispel 0 (SpellCategories 115683). NOT carried:
	// SpellAuraOptions ProcChance 100 / ProcTypeMask 0x14 (seed
	// has no SpellAuraOptions table) — the RegisterAuraEvent proc
	// leg (boss_anetheron.lua:251-256, heal 31285) stays DORMANT
	// on that ground; the timer self-cast arm now resolves. No
	// SpellPower row (no mana cost), no SpellClassOptions row.
	// Engine twin: auraDummy live handler. Removes the timer-arm
	// blocker for boss_anetheron.lua.
	{id: 38196, castTimeIdx: 1, durIdx: 28, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetSrcCaster, targetB: targetUnitSrcAreaAlly, radiusIdx: 20, chainAmplitude: 1},
	}},
	// 6533 Throw Net (boss_gelihast.cpp:24 enum, :50-53 DoCastVictim
	// on a 2s/4s init / 4s/7s re-arm timer; Lua
	// boss_gelihast.lua:32 victim cast) — wago.tools 3.4.0.47659
	// SpellEffect rows 686197/686198 + SpellMisc 320465
	// (2026-11-15 browser read, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 26 MOD_ROOT Die 0 Base 0 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY; eff1 64 TRIGGER_SPELL trigger
	// 11920 Die 1 TargetA 6; castTimeIdx 1 (instant, real row),
	// durIdx 39 (2000ms, real row — 57994 precedent), rangeIdx 3
	// (Twenty yards, real row — 15574 precedent), school 0x1
	// Physical. NOT carried: DifficultyID 173/174 rows (durIdx 1,
	// eff1 Base 1) — seed takes the DifficultyID 0 row per
	// convention; no Dispel column in this build's SpellMisc, so
	// dispel stays 0. Engine twins: auraRoot live (cc_break.go:104,
	// lua_unit_state.go:170), effTriggerSpell live (spells.go:385).
	// 11920 (the triggered spell) seeded below — follow-up read landed.
	// Removes the last unseeded-spell blocker for
	// boss_gelihast.lua (single-missing-id file).
	{id: 6533, castTimeIdx: 1, durIdx: 39, rangeIdx: 3, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, targetA: targetEnemy},
		{effect: effTriggerSpell, dieSides: 1, trigger: 11920, targetA: targetEnemy},
	}},
	// 11920 Net (triggered by 6533 Throw Net eff1, boss_gelihast.cpp:24;
	// Lua boss_gelihast.lua:32) — wago.tools 3.4.0.47659 SpellEffect row
	// 688535 + SpellMisc 322355 (2026-11-15 browser read, DifficultyID 0
	// rows): eff0 6 APPLY_AURA aura 4 DUMMY Die 0 Base 0 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY; castTimeIdx 1 (instant, real row),
	// durIdx 18 (20000ms, real row — 29946 precedent), rangeIdx 13
	// (Anywhere - Unlimited 0-50000yd, real row — 29967 precedent),
	// school 0x1 Physical, dispel 0 (SpellCategories 73520, DispelType 0).
	// NOT carried: no SpellPower row, no SpellAuraOptions row (no proc
	// chance/mask); EffectBonusCoefficient 0 / RealPointsPerLevel 0
	// (seed has no coefficient fields). Engine twin: auraDummy live
	// handler. The 6533 trigger leg now resolves end-to-end.
	{id: 11920, castTimeIdx: 1, durIdx: 18, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetEnemy},
	}},
	// 10101 Knock Away (boss_highlord_omokk.cpp:25 enum, :84
	// DoCastVictim; Lua boss_highlord_omokk.lua:44 victim cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows 688723/688724/688725
	// + SpellMisc 322511 (2026-11-15 browser read): eff0 58
	// WEAPON_DAMAGE Die 1 Base 29 TargetA 6; eff1 98 KNOCK_BACK
	// Die 75 Base 112 Misc 75 TargetA 6; eff2 6 APPLY_AURA aura 12
	// MOD_STUN Die 0 Base 0 TargetA 6; castTimeIdx 1 (instant, real
	// row), durIdx 0 (no SpellDuration row 0 exists in this build —
	// the stun aura takes duration 0, store.go:2284-2287), rangeIdx
	// 7 (Ten yards, real row — 49256 precedent), school 0x1
	// Physical. NOT carried: EffectBonusCoefficient 1 (seed has no
	// coefficient field); no Dispel column in this build's
	// SpellMisc, so dispel stays 0. Engine twins:
	// effWeaponDamage (spells.go:233, handled :15028/:15037),
	// effKnockBack (spells.go:454, handled :18835/:18882),
	// auraStun live (cc_break.go:104). Removes the last
	// unseeded-spell blocker for boss_highlord_omokk.lua
	// (single-missing-id file).
	{id: 10101, castTimeIdx: 1, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 29, targetA: targetEnemy},
		{effect: effKnockBack, dieSides: 75, basePoints: 112, misc: 75, targetA: targetEnemy},
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy},
	}},
	// 12795 Frenzy (boss_glutton.cpp:33 enum, :86 DoCast(me)
	// self-cast; Lua boss_glutton.lua:55 self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows 691492/691493 + SpellMisc 324830
	// (2026-11-15 browser read): eff0 6 APPLY_AURA aura 13
	// MOD_DAMAGE_DONE Die 1 Base 0 Misc 1 RealPointsPerLevel 1
	// TargetA 1 TARGET_UNIT_CASTER; eff1 6 APPLY_AURA aura 138
	// MOD_MELEE_HASTE Die 1 Base 29 TargetA 1 (30% haste);
	// castTimeIdx 1 (instant, real row), durIdx 4 (120000ms, real
	// row — 67530 precedent), rangeIdx 1 (Self Only, real row),
	// school 0x1 Physical. NOT carried: RealPointsPerLevel 1 (seed
	// has no per-level field); no Dispel column in this build's
	// SpellMisc, so dispel stays 0. Engine twins: auraModDamageDone
	// (spells.go:689, 8269 precedent), auraModMeleeHaste (8269
	// precedent). Removes the last unseeded-spell blocker for
	// boss_glutton.lua (single-missing-id file).
	{id: 12795, castTimeIdx: 1, durIdx: 4, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageDone, dieSides: 1, misc: 1, targetA: targetCaster},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 29, targetA: targetCaster},
	}},
	// 15531 Frost Nova (boss_amnennar_the_coldbringer.cpp:35 enum,
	// :114 DoCast(me) self-cast; Lua
	// boss_amnennar_the_coldbringer.lua:75 self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows 692387/692388 + SpellMisc 325606
	// (2026-11-15 browser read): eff0 2 SCHOOL_DAMAGE Die 19 Base
	// 110 TargetA 22 TARGET_SRC_CASTER TargetB 15
	// TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex_1 13 (10yd, real row)
	// chainAmplitude 1 (111-129 damage); eff1 6 APPLY_AURA aura 26
	// MOD_ROOT Die 1 Base -1 TargetA 22 TargetB 15 RadiusIndex_0
	// 13 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 31 (8000ms, real row — 2139 precedent), rangeIdx 1
	// (Self Only, real row), school 0x10 Frost. NOT carried:
	// EffectBonusCoefficient 1 (seed has no coefficient field); no
	// Dispel column in this build's SpellMisc, so dispel stays 0.
	// Engine twins: effSchoolDamage + auraRoot live. Removes the
	// last unseeded-spell blocker for
	// boss_amnennar_the_coldbringer.lua (single-missing-id file).
	{id: 15531, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 19, basePoints: 110, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 17234 Shadow Shock (boss_illucia_barov.cpp:82 DoCast random
	// target (SelectTarget Random, 0, 100) SPELL_SHADOWSHOCK,
	// triggered; Lua boss_illucia_barov.lua:110 target cast,
	// triggered) — wago.tools 3.4.0.47659 SpellEffect row 694112 +
	// SpellMisc 326988 + SpellCategories 75100 (2026-11-15 browser
	// read, DifficultyID 0 rows): single eff0 2 SCHOOL_DAMAGE Die
	// 33 Base 209 TargetA 6 TARGET_UNIT_TARGET_ENEMY
	// chainAmplitude 1 (210-242 damage); castTimeIdx 1 (instant,
	// real row), durIdx 0 (no SpellDuration row 0 exists in this
	// build), rangeIdx 3 (Twenty yards, real row — 15574
	// precedent), school 0x20 Shadow, dispel 0 (SpellCategories
	// 75100). NOT carried: EffectBonusCoefficient 1 (seed has no
	// coefficient field); no SpellAuraOptions row, no SpellPower
	// row. Engine twin: effSchoolDamage live handler. Removes the
	// last unseeded-spell blocker for boss_illucia_barov.lua
	// (single-missing-id file).
	{id: 17234, castTimeIdx: 1, rangeIdx: 3, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 33, basePoints: 209, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16791 Furious Anger (boss_magistrate_barthilas.cpp:115
	// DoCast(me, SPELL_FURIOUS_ANGER, false) self-cast
	// non-triggered; Lua boss_magistrate_barthilas.lua:112
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect rows
	// 693618/693619/693620 + SpellMisc 326612 (2026-11-15 browser
	// read, DifficultyID 0 rows): eff0 6 APPLY_AURA aura 13
	// MOD_DAMAGE_DONE Die 1 Base 14 Misc 1 TargetA 1
	// TARGET_UNIT_CASTER; eff1 6 APPLY_AURA aura 138
	// MOD_MELEE_HASTE Die 1 Base 3 TargetA 1; eff2 6 APPLY_AURA
	// aura 61 MOD_SCALE Die 1 Base 4 TargetA 1; castTimeIdx 1
	// (instant, real row), durIdx 3 (60000ms, real row — 31331
	// precedent), rangeIdx 1 (Self Only, real row), school 0x1
	// Physical. NOT carried: SpellAuraOptions ProcChance 101 /
	// ProcTypeMask 0 (seed has no SpellAuraOptions table);
	// EffectBonusCoefficient 0/1 (seed has no coefficient field);
	// no SpellCategories row, so dispel stays 0. Engine twins:
	// auraModDamageDone spells.go:689, auraModMeleeHaste
	// aura_haste.go:44 + creature_auras.go:1735, auraModScale
	// aura_unit.go:44-50 (8269 Frenzy precedent for all three).
	// Removes the last unseeded-spell blocker for
	// boss_magistrate_barthilas.lua (single-missing-id file).
	{id: 16791, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageDone, dieSides: 1, basePoints: 14, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 3, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 4, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 17470 Ravenous Claw (boss_timmy_the_cruel.cpp:89
	// DoCastVictim(SPELL_RAVENOUSCLAW); Lua
	// boss_timmy_the_cruel.lua:83 victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 694808 + SpellMisc 327536 +
	// SpellCategories 75307 (2026-11-15 browser read,
	// DifficultyID 0 rows): single eff0 9 HEALTH_LEECH Die 151
	// Base 924 TargetA 6 TARGET_UNIT_TARGET_ENEMY; castTimeIdx 1
	// (instant, real row), durIdx 0 (no SpellDuration row 0
	// exists in this build), rangeIdx 2 (Combat Range 0-5yd, real
	// row), school 0x1 Physical, dispel 0 (SpellCategories
	// 75307). NOT carried: EffectAmplitude 1 (seed has no
	// amplitude field, precedent main.go:762/:1083);
	// EffectBonusCoefficient 0 (seed has no coefficient field);
	// no SpellAuraOptions row. Engine twin: effHealthLeech
	// spells.go:18601 case + :18638 handleEffectHealthLeech.
	// Removes the last unseeded-spell blocker for
	// boss_timmy_the_cruel.lua (single-missing-id file).
	{id: 17470, castTimeIdx: 1, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effHealthLeech, dieSides: 151, basePoints: 924, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 23261 Summon Darkreaver's Fallen Charger
	// (boss_death_knight_darkreaver.cpp:50 DoCast(me, 23261, true)
	// triggered self-cast; Lua
	// boss_death_knight_darkreaver.lua:33 triggered self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row 699983 + SpellMisc
	// 331708 (2026-11-15 browser read, DifficultyID 0 rows):
	// single eff0 28 SUMMON Die 1 Base 0 Misc 14568 MiscB 64
	// TargetA 32 TARGET_DEST_CASTER_SUMMON RadiusIndex_0 15 (3yd,
	// real row); castTimeIdx 1 (instant, real row), durIdx 367
	// (7200000ms, real row added below), rangeIdx 12 (Interact
	// Range 0-5yd, real row added below), school 0x1 Physical.
	// NOT carried: EffectBonusCoefficient 1 (seed has no
	// coefficient field); no SpellCategories row, so dispel stays
	// 0; no SpellAuraOptions row. SummonProperties 64 real row
	// pre-existing in the seed. Engine twin: effSummon live
	// handler (50415 precedent). Removes the last unseeded-spell
	// blocker for boss_death_knight_darkreaver.lua
	// (single-missing-id file).
	{id: 23261, castTimeIdx: 1, durIdx: 367, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestCasterSummon, radiusIdx: 15, misc: 14568, miscB: 64, chainAmplitude: 1},
	}},
	// 6421 Unlock (shadowfang_keep.cpp:55 enum, :93 DoCast(me) at
	// waypoint 11, Ash only; Lua npc_shadowfang_prisoner.lua:66
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row 685924 +
	// SpellMisc 320228 (2026-11-15 browser read, DifficultyID 0
	// rows): single eff0 3 DUMMY Die 0 Base 0 TargetA 1
	// TARGET_UNIT_CASTER; castTimeIdx 15 (4000ms, real row),
	// durIdx 0, rangeIdx 1 (Self Only, real row), school 0x1
	// Physical. NOT carried: no SpellCategories row, so dispel
	// stays 0; no SpellPower row. Engine twin: effDummy live
	// handler (spells.go:375). Removes the last unseeded-spell
	// blocker for npc_shadowfang_prisoner.lua (single-missing-id
	// file).
	{id: 6421, castTimeIdx: 15, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetCaster},
	}},
	// 7154 Dark Offering (shadowfang_keep.cpp:56 enum, :182/:184
	// DoCast(pFriend/me); Lua npc_arugal_voidwalker.lua:66
	// CastSpell(friend or creature)) — wago.tools 3.4.0.47659
	// SpellEffect row 685943 + SpellMisc 320242 (2026-11-15
	// browser read, DifficultyID 0 rows): single eff0 10 HEAL Die
	// 1 Base 199 TargetA 21 TARGET_UNIT_TARGET_ALLY; castTimeIdx 1
	// (instant, real row), durIdx 0, rangeIdx 7 (10yd, real row),
	// school 0x20 Shadow. NOT carried: no SpellCategories row, so
	// dispel stays 0; no SpellPower row; Attributes 0x10/0x80000
	// (seed carries no attribute fields). Engine twin: effHeal live
	// (boss_ai.go:3931, creaturemotion.go:8291). Removes the last
	// unseeded-spell blocker for npc_arugal_voidwalker.lua
	// (single-missing-id file).
	{id: 7154, castTimeIdx: 1, rangeIdx: 7, school: schoolShadow, effects: [3]effect{
		{effect: effHeal, dieSides: 1, basePoints: 199, targetA: targetUnitTargetAlly},
	}},
	// 7737 Drink (zone_tirisfal_glades.cpp:45 enum, :104
	// DoCastSelf; Lua npc_calvin_montague.lua:195 self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row 685872 + SpellMisc
	// 320182 + SpellCategories 72638 (2026-11-15 browser read,
	// DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 84
	// MOD_REGEN Die 1 Base 9 TargetA 1 TARGET_UNIT_CASTER,
	// EffectAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 8 (15000ms, real row), rangeIdx 1 (Self Only, real
	// row), school 0x1 Physical, dispel 0 (SpellCategories
	// category 11, DispelType 0). NOT carried: SpellCategories
	// category 11 (seed has no category field); EffectAmplitude 1
	// (seed carries no amplitude field — auraPeriod 0 per wago);
	// no SpellPower row. Engine twin: auraModRegen live
	// (spells.go:38723, consumed server.go:1992). Removes the last
	// unseeded-spell blocker for npc_calvin_montague.lua
	// (single-missing-id file).
	{id: 7737, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModRegen, dieSides: 1, basePoints: 9, targetA: targetCaster},
	}},
	// 7978 Dynamite (zone_azuremyst_isle.cpp:191 enum, :259
	// DoCastVictim; Lua npc_engineer_spark_overgrind.lua:133
	// victim cast) — wago.tools 3.4.0.47659 SpellEffect row 685369
	// + SpellMisc 319737 (2026-11-15 browser read, DifficultyID 0
	// rows): single eff0 2 SCHOOL_DAMAGE Die 9 Base 25 TargetA 16
	// TARGET_UNIT_DEST_AREA_ENEMY RadiusIndex_0 8 (5yd, real row);
	// castTimeIdx 4 (1000ms, real row), durIdx 0, rangeIdx 4
	// (30yd, real row), school 0x4 Fire. NOT carried:
	// EffectBonusCoefficient 1 (seed has no coefficient field);
	// Speed 14 (seed has no speed field); no SpellCategories row,
	// so dispel stays 0; no SpellPower row. Engine twin:
	// effSchoolDamage live (spells.go:405, boss_ai.go:1709).
	// Removes the last unseeded-spell blocker for
	// npc_engineer_spark_overgrind.lua (single-missing-id file).
	{id: 7978, castTimeIdx: 4, rangeIdx: 4, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 9, basePoints: 25, targetA: targetUnitDestAreaEnemy, radiusIdx: 8},
	}},
	// 47751 Spark (boss_anomalus.cpp:27 enum, :182 DoCast(target,
	// SPELL_SPARK) random-target 5s timer; Lua boss_anomalus.lua:188
	// randomPlayer cast) — wago.tools 3.4.0.47659 SpellEffect row
	// 952592 + SpellMisc 601068 + SpellPower 235741 + SpellCategories
	// 125646 (2026-11-15 browser read, DifficultyID 0 rows): single
	// eff0 2 SCHOOL_DAMAGE Die 451 Base 2024 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY, chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 0, rangeIdx 5 (40yd, real row),
	// school 0x40 Arcane, manaCost 90 (SpellPower row), dispel 0
	// (SpellCategories DispelType 0). NOT carried: Speed 12 (seed
	// has no speed field); SpellAuraOptions ProcChance 101 (seed has
	// no proc table). Engine twin: effSchoolDamage live
	// (spells.go:405, boss_ai.go:1709). Removes the last
	// unseeded-spell blocker for boss_anomalus.lua
	// (single-missing-id file).
	{id: 47751, castTimeIdx: 1, rangeIdx: 5, school: schoolArcane, manaCost: 90, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 451, basePoints: 2024, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 52087 Web Wrap (wago name; C++ SPELL_WEB_WRAP_WRAPPED,
	// boss_krikthir_the_gatewatcher.cpp:75 enum, :1021
	// target->CastSpell(target, SPELL_WEB_WRAP_WRAPPED, true) in the
	// web-wrap SpellScript; Lua boss_krikthir_the_gatewatcher.lua:59
	// triggered self-cast on webWrapRemove) — wago.tools 3.4.0.47659
	// SpellEffect rows 979898/979899/979900 + SpellMisc 621293
	// (2026-11-15 browser read, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 12 MOD_STUN TargetA 1 TARGET_UNIT_CASTER,
	// eff1 6 APPLY_AURA aura 56 TRANSFORM Misc 32785 TargetA 1,
	// eff2 28 SUMMON Misc 28619 MiscB 64 TargetA 18
	// TARGET_DEST_CASTER, all chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 1 (10000ms, real row), rangeIdx 5
	// (40yd, real row), school 0x1 Physical. NOT carried: no
	// SpellPower row; no SpellCategories row, so dispel stays 0;
	// SpellAuraOptions ProcChance 101 (seed has no proc table).
	// Engine twins: auraStun live, auraTransform 56 live
	// (spells.go:711, shapeshift.go:422), effSummon live (:17480).
	// Removes the last unseeded-spell blocker for
	// boss_krikthir_the_gatewatcher.lua (single-missing-id file).
	// 52086 Web Wrap needs no seed (RegisterAuraEvent only, no
	// CastSpell site).
	{id: 52087, castTimeIdx: 1, durIdx: 1, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, targetA: targetCaster, misc: 32785, chainAmplitude: 1},
		{effect: effSummon, targetA: targetDestCaster, misc: 28619, miscB: 64, chainAmplitude: 1},
	}},
	// 33814 Infernal Rain (wago name; C++ SPELL_METEOR,
	// hyjal_trash.cpp:29 enum, :482 trigger->CastSpell(me,
	// SPELL_METEOR, true); Lua hyjal_trash.lua:509 trigger cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows 852482/852483 +
	// SpellMisc 450435 (2026-11-15 browser read, DifficultyID 0
	// rows): eff0 3 DUMMY RadiusIndex_0 29 (6yd, real row added
	// below) TargetA 8 TARGET_UNIT_DEST_AREA_ENTRY, eff1 32
	// TRIGGER_MISSILE TriggerSpell 33637 RadiusIndex_0 10 (30yd,
	// real row) TargetA 8, both chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 0, rangeIdx 13 (50000yd, real
	// row), school 0x1 Physical. NOT carried: Speed 20 (seed has
	// no speed field); no SpellPower row; no SpellCategories row,
	// so dispel stays 0; SpellAuraOptions ProcChance 101 (seed has
	// no proc table). Engine twins: effDummy live (spells.go:375),
	// TRIGGER_MISSILE 32 live (spells.go:387/:16544). FOLLOW-UP:
	// 33637 (eff1 trigger) unseeded, needs its own wago read.
	// Removes the last unseeded-spell blocker for hyjal_trash.lua
	// (single-missing-id file).
	{id: 33814, castTimeIdx: 1, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetUnitDestAreaEntry, radiusIdx: 29, chainAmplitude: 1},
		{effect: effTriggerMiss, targetA: targetUnitDestAreaEntry, radiusIdx: 10, trigger: 33637, chainAmplitude: 1},
	}},
	// 29908 Astral Bite (wago name; C++ SPELL_MANA_BITE,
	// karazhan.cpp:630 enum, :653 DoCast(me, SPELL_MANA_BITE) in
	// Reset; Lua npc_mana_feeder.lua:23 self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row 847736 + SpellMisc 447072 +
	// SpellCategories 112236 (2026-11-15 browser read, DifficultyID
	// 0 rows): single eff0 6 APPLY_AURA aura 42 PROC_TRIGGER_SPELL
	// TriggerSpell 29907 TargetA 1 TARGET_UNIT_CASTER,
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx
	// 21 (infinite, real row), rangeIdx 1 (Self Only, real row),
	// school 0x40 Arcane, dispel 0 (SpellCategories DispelType 0).
	// NOT carried: no SpellPower row; SpellAuraOptions ProcChance
	// 100 / ProcTypeMask 0x14 (seed has no proc table — the
	// anetheron 15574 precedent). Engine twin: aura42 live
	// (procs.go:1287). FOLLOW-UP: 29907 (proc trigger) unseeded,
	// needs its own wago read. Removes the last unseeded-spell
	// blocker for npc_mana_feeder.lua (single-missing-id file).
	{id: 29908, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, targetA: targetCaster, trigger: 29907, chainAmplitude: 1},
	}},
	// 39646 Shadow Inferno Damage (wago name; C++
	// SPELL_SHADOW_INFERNO_DAMAGE, black_temple.cpp:37 enum; the
	// spell_illidari_nightlord_shadow_inferno OnPeriodic
	// (black_temple.cpp:254-274) casts it triggered on the aura
	// owner with BP0 = tick * amount; Lua
	// spell_black_temple.lua:26 CastCustomSpell(target, 39646,
	// true, bp)) — wago.tools 3.4.0.47659 SpellEffect row 860931
	// + SpellMisc 456765 + SpellCategories 116380 (2026-11-15
	// browser read 43953a23, DifficultyID 0 rows): single eff0 2
	// SCHOOL_DAMAGE Die 1 Base 0 TargetA 22 TARGET_SRC_CASTER
	// TargetB 15 TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex_0 9 (20yd,
	// real row), chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 0 (no SpellDuration row), rangeIdx 1 (Self Only,
	// real row), school 0x20 Shadow, dispel 0 (SpellCategories
	// DispelType 0). NOT carried: no SpellPower row; no
	// SpellAuraOptions row; Speed 0. Engine twin: effSchoolDamage
	// live (spells.go:405). Removes the last unseeded-spell blocker
	// for spell_black_temple.lua (single-missing-id file).
	{id: 39646, castTimeIdx: 1, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 37090 Highborne Aura (wago name; C++ SPELL_HIGHBORNE_AURA,
	// zone_undercity.cpp:59 enum, :316 DoCast(me,
	// SPELL_HIGHBORNE_AURA) on a one-shot 17500ms timer; Lua
	// npc_highborne_lamenter.lua:66 const, one-shot CreateLuaEvent
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect rows
	// 857735/857736 + SpellMisc 455017 (2026-11-15 browser read
	// 43953a23, DifficultyID 0 rows): eff0 6 APPLY_AURA aura 4
	// DUMMY TargetA 1 TARGET_UNIT_CASTER, eff1 46 SPAWN TargetA 0,
	// both chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 21 (infinite, real row), rangeIdx 1 (Self Only, real
	// row), school 0x1 Physical. NOT carried: no SpellCategories
	// row, so dispel stays 0; no SpellPower row; SpellAuraOptions
	// ProcChance 101 (seed has no proc table). Engine twins:
	// auraDummy live; effSpawn 46 is clientside-only (spells.go:
	// 19747 unused list) — carried verbatim, no server-side
	// effect. Removes the last unseeded-spell blocker for
	// npc_highborne_lamenter.lua (single-missing-id file).
	{id: 37090, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
		{effect: effSpawn, chainAmplitude: 1},
	}},
	// 23059 Create Thorium Brotherhood Contract DND (wago name; C++
	// SPELL_CREATE_THORIUM_BROTHERHOOD_CONTRACT_DND,
	// blackrock_depths.cpp:439 enum; player self-casts it on
	// gossip select (Lokhtos contract arm); Lua
	// npc_lokhtos_darkbargainer.lua:35 const, non-triggered
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row 699977 +
	// SpellMisc 331702 (2026-11-15 browser read 43953a23,
	// DifficultyID 0 rows): single eff0 24 CREATE_ITEM ItemType
	// 18628 TargetA 0, chainAmplitude 1; castTimeIdx 4 (1000ms,
	// real row), durIdx 0 (no SpellDuration row), rangeIdx 12
	// (Interact 5yd, real row), school 0x1 Physical. New seed
	// field: effect.itemType (store.go field 107+i) —
	// handleEffectCreateItem reads eff.ItemType and creates nothing
	// when it is 0, so the row would be a no-op without it. NOT
	// carried: EffectBonusCoefficient 1 (seed has no bonus-coefficient
	// field); no SpellCategories row, so dispel stays 0; no
	// SpellPower/SpellAuraOptions rows. Engine twin:
	// handleEffectCreateItem live (spells.go:18360/:29780).
	// Removes the last unseeded-spell blocker for
	// npc_lokhtos_darkbargainer.lua (single-missing-id file).
	{id: 23059, castTimeIdx: 4, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effCreateItem, itemType: 18628, chainAmplitude: 1},
	}},
	// 17370 Soul Freed (wago name; C++ SPELL_SOUL_FREED,
	// stratholme.cpp:104 enum, :164 summoned->CastSpell(summoned,
	// SPELL_SOUL_FREED, false) in JustSummoned; Lua
	// npc_restless_soul.lua:64 const, freed-soul self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row 694101 + SpellMisc
	// 326977 (2026-11-15 browser read 43953a23, DifficultyID 0
	// rows): single eff0 3 DUMMY TargetA 1 TARGET_UNIT_CASTER,
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 0
	// (no SpellDuration row), rangeIdx 1 (Self Only, real row),
	// school 0x1 Physical. NOT carried: EffectBonusCoefficient 1
	// (no field); no SpellCategories row, so dispel stays 0; no
	// SpellPower/SpellAuraOptions rows. Engine twin: effDummy live
	// (spells.go:375). Removes the last unseeded-spell blocker for
	// npc_restless_soul.lua (single-missing-id file).
	{id: 17370, castTimeIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 44335 Energy Feedback (wago name; C++ SPELL_ENERGY_FEEDBACK,
	// boss_vexallus.cpp:188 enum; JustDied killer->CastSpell(killer,
	// SPELL_ENERGY_FEEDBACK, true); Lua npc_pure_energy.lua:38
	// triggered killer self-cast) — wago.tools 3.4.0.47659
	// SpellEffect rows 865344/865345/865346 + SpellMisc 460442
	// (2026-11-15 browser read bb5173c6, DifficultyID 0 rows):
	// eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE Die 1 Base 299
	// AuraPeriod 1000 TargetA 1 TARGET_UNIT_CASTER, eff1 6
	// APPLY_AURA aura 79 MOD_DAMAGE_PERCENT_DONE Die 1 Base 49
	// Misc 127 TargetA 1, eff2 6 APPLY_AURA aura 61 MOD_SCALE Die 1
	// Base 29 TargetA 1, all chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 9 (30000ms, real row), rangeIdx 13
	// (Anywhere 50000yd, real row), school 0x40 Arcane. NOT carried:
	// no SpellPower row; no SpellCategories row, so dispel stays 0;
	// SpellAuraOptions ProcChance 101 (seed has no proc table);
	// Speed 0; Attributes (no fields). Engine twins: auraPeriodicDmg
	// live, auraModDmgDone 79 live (aura_damage.go:119,
	// creature_auras.go:460), auraModScale 61 live
	// (aura_unit.go:44-50). Removes the last unseeded-spell blocker
	// for npc_pure_energy.lua (single-missing-id file).
	{id: 44335, castTimeIdx: 1, durIdx: 9, rangeIdx: 13, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 299, auraPeriod: 1000, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 49, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 29, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 14872 Drunken Rage (wago name; C++ SPELL_DRUNKEN_RAGE,
	// blackrock_depths.cpp:505 enum; Lua npc_rocknot.lua:90
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect rows
	// 692018/692019 + SpellMisc 325291 + SpellCategories 74509
	// (2026-11-15 browser read bb5173c6, DifficultyID 0 rows):
	// eff0 6 APPLY_AURA aura 138 MOD_MELEE_HASTE Die 1 Base 49
	// TargetA 1 TARGET_UNIT_CASTER, eff1 6 APPLY_AURA aura 13
	// MOD_DAMAGE_DONE Die 1 Base 49 Misc 1 TargetA 1, both
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 8
	// (15000ms, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (SpellCategories DispelType 0).
	// NOT carried: SpellCategories Category 21 (seed carries no
	// category field, 7737 precedent); EffectBonusCoefficient 1 (no
	// field); no SpellPower/SpellAuraOptions rows. Engine twins:
	// auraModMeleeHaste 138 live (aura_haste.go:44),
	// auraModDamageDone 13 live (spells.go:689). Removes the last
	// unseeded-spell blocker for npc_rocknot.lua
	// (single-missing-id file).
	{id: 14872, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 49, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageDone, dieSides: 1, basePoints: 49, misc: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 22120 Charge (wago name; C++ SPELL_CHARGE, boss_broggok.cpp:45
	// enum; Lua npc_fel_orc_neophyte.lua:125 victim-cast on the
	// 5s-init/20s pump) — wago.tools 3.4.0.47659 SpellEffect rows
	// 699502/699503 + SpellMisc 331339 (2026-11-15 browser read
	// bb5173c6, DifficultyID 0 rows): eff0 96 CHARGE TargetA 6
	// TARGET_UNIT_TARGET_ENEMY, eff1 58 WEAPON_DAMAGE Die 1 Base 74
	// TargetA 6, both chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 0 (no SpellDuration row), rangeIdx 74
	// (Ranged Weapon 30yd, real row), school 0x1 Physical. NOT
	// carried: Speed 27 (no field); SpellPower row all zeros
	// (PowerType 1, ManaCost 0); EffectBonusCoefficient 1 (no
	// field); no SpellCategories/SpellAuraOptions rows, so dispel
	// stays 0. Engine twins: effCharge 96 live
	// (handleEffectCharge, spells.go:17816), effWeaponDamage live
	// (spells.go:233/:15028). Removes the last unseeded-spell
	// blocker for npc_fel_orc_neophyte.lua (single-missing-id file).
	{id: 22120, castTimeIdx: 1, rangeIdx: 74, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effWeaponDamage, dieSides: 1, basePoints: 74, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 30621 Kidney Shot (wago name; C++ SPELL_KIDNEY_SHOT,
	// boss_vazruden_the_herald.cpp:55 enum; Lua
	// npc_hellfire_sentry.lua:89 victim-cast on the {3s,7s}-init/20s
	// pump) — wago.tools 3.4.0.47659 SpellEffect row 849261 +
	// SpellMisc 448405 + SpellCategories 112829 (2026-11-15 browser
	// read bb5173c6, DifficultyID 0 rows): single eff0 6 APPLY_AURA
	// aura 12 MOD_STUN Die 1 Base 0 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY, chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 27 (3000ms, real row), rangeIdx 2
	// (Combat Range 5yd, real row), school 0x1 Physical, mechanic
	// 12 (SpellCategories Mechanic 12 = MECHANIC_STUN, feeds
	// diminishing.go:337), dispel 0 (SpellCategories DispelType 0).
	// NOT carried: SpellCategories Category 270 / DefenseType 2 /
	// PreventionType 0x2 / StartRecoveryCategory 133 (no fields);
	// SpellPower row all zeros (PowerType 3, ManaCost 0);
	// SpellAuraOptions ProcChance 101 (no proc table). Engine twin:
	// auraStun live (cc_break.go:104). Removes the last
	// unseeded-spell blocker for npc_hellfire_sentry.lua
	// (single-missing-id file).
	{id: 30621, castTimeIdx: 1, durIdx: 27, rangeIdx: 2, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 33637 Infernal (wago name; trigger of 33814 Infernal Rain eff1
	// TRIGGER_MISSILE, hyjal_trash.cpp:482; Lua hyjal_trash.lua:509)
	// — wago.tools 3.4.0.47659 SpellEffect rows 851909/851910 +
	// SpellMisc 450413 (2026-11-15 browser read bb5173c6,
	// DifficultyID 0 rows): eff0 2 SCHOOL_DAMAGE Die 1 Base 149
	// TargetA 16 TARGET_UNIT_DEST_AREA_ENEMY RadiusIndex_0 9 (20yd,
	// real row), eff1 6 APPLY_AURA aura 12 MOD_STUN TargetA 16
	// RadiusIndex_0 9 EffectMechanic 12, both chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 28 (5000ms, real
	// row), rangeIdx 13 (Anywhere 50000yd, real row), school 0x1
	// Physical. NOT carried: no SpellPower row; no SpellCategories
	// row, so dispel stays 0; SpellAuraOptions ProcChance 101 (no
	// proc table); Speed 0. Engine twins: effSchoolDamage live
	// (spells.go:405), auraStun live (cc_break.go:104). Removes the
	// last unseeded-spell blocker for the 33814 trigger leg.
	{id: 33637, castTimeIdx: 1, durIdx: 28, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 149, targetA: targetUnitDestAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, mechanic: 12, targetA: targetUnitDestAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 29907 Astral Bite (wago name; proc trigger of 29908 Astral Bite
	// eff0 PROC_TRIGGER_SPELL, karazhan.cpp:653; Lua
	// karazhan.lua:23 mana-feeder leg) — wago.tools 3.4.0.47659
	// SpellEffect row 847735 + SpellMisc 447004 + SpellCategories
	// 112235 (2026-11-15 browser read bb5173c6, DifficultyID 0
	// rows): single eff0 62 POWER_BURN Die 41 Base 254 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY, chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 0 (no SpellDuration row),
	// rangeIdx 7 (Ten yards, real row), school 0x40 Arcane, dispel
	// 0 (SpellCategories DispelType 0), mechanic 0. New seed const
	// effPowerBurn=62. NOT carried: Amplitude 1 (seed does not
	// encode EffectAmplitude, 17470 precedent); EffectBonusCoefficient
	// 1 (no field); SpellCategories Category 0 / DefenseType 1 /
	// PreventionType 0x1 (no fields); SpellAuraOptions ProcChance 101
	// (no proc table). Engine twin: effPowerBurn 62 live
	// (spells.go:16321/:29468). Removes the last unseeded-spell
	// blocker for the 29908 proc leg.
	{id: 29907, castTimeIdx: 1, rangeIdx: 7, school: schoolArcane, effects: [3]effect{
		{effect: effPowerBurn, dieSides: 41, basePoints: 254, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 58109 Crates Kill Credit (wago name; C++ SPELL_CRATES_KILL_CREDIT,
	// instance_culling_of_stratholme.cpp:79 enum; Lua
	// instance_culling_of_stratholme.lua:518 triggered cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// b476939d, DifficultyID 0 rows): single eff0 134 KILL_CREDIT
	// TargetA 22 TARGET_SRC_CASTER TargetB 7 TARGET_UNIT_SRC_AREA_ENTRY
	// RadiusIndex_0 28 (50000yd, real row) Misc 30996, chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration row),
	// rangeIdx 13 (50000yd, real row), school 0x1 Physical, dispel 0
	// (no SpellCategories row). NOT carried: SpellAuraOptions ProcChance
	// 101 (seed has no proc table); no SpellPower row. Engine twin:
	// effKillCredit2 134 live (spells.go:17749,
	// handleEffectKillCreditGroup). Removes the last unseeded-spell
	// blocker for instance_culling_of_stratholme.lua (single-missing-id
	// file).
	{id: 58109, castTimeIdx: 1, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effKillCredit2, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 28, misc: 30996, chainAmplitude: 1},
	}},
	// 31387 Time Rift Channel (wago name; C++ SPELL_RIFT_CHANNEL,
	// instance_the_black_morass.cpp:37 enum; Lua
	// instance_the_black_morass.lua:184 rift channel cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// b476939d, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 4
	// DUMMY TargetA 25 TARGET_UNIT_TARGET_ANY; castTimeIdx 1 (instant,
	// real row), durIdx 21 (infinite, real row), rangeIdx 13 (50000yd,
	// real row), school 0x20 Shadow, dispel 0 (no SpellCategories row).
	// NOT carried: no SpellPower row; no SpellAuraOptions row. Engine
	// twin: auraDummy live. Removes the last unseeded-spell blocker for
	// instance_the_black_morass.lua (single-missing-id file).
	{id: 31387, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetUnitTargetAny},
	}},
	// 26143 Mind Flay (wago name; C++ SPELL_MIND_FLAY, boss_cthun.cpp:90
	// enum, :950 DoCast(target, SPELL_MIND_FLAY); Lua
	// npc_eye_tentacle.lua:67 random-target cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc + SpellCategories
	// (2026-11-15 browser read b476939d, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 3 PERIODIC_DAMAGE Die 1 Base 749 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY period 1000 chainTargets 1, eff1 6
	// APPLY_AURA aura 33 MOD_DECREASE_SPEED Die 1 Base -31 TargetA 6;
	// castTimeIdx 1 (instant, real row), durIdx 1 (10000ms, real row),
	// rangeIdx 13 (50000yd, real row), school 0x20 Shadow, dispel 0 /
	// mechanic 11 (SpellCategories DispelType 0 / Mechanic 11,
	// Category 0 / DefenseType 1 not carried — seed has no fields).
	// NOT carried: EffectBonusCoefficient 0.1 (no field); no
	// SpellPower row. Engine twins: auraPeriodicDmg 3 live,
	// auraDecSpeed 33 live (spells.go:34046, movement_speed.go:342).
	// Removes the last unseeded-spell blocker for npc_eye_tentacle.lua
	// (single-missing-id file).
	{id: 26143, castTimeIdx: 1, durIdx: 1, rangeIdx: 13, school: schoolShadow, mechanic: 11, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 749, targetA: targetEnemy, auraPeriod: 1000, chainTargets: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -31, targetA: targetEnemy},
	}},
	// 44326 Pure Energy Passive (wago name; C++ SPELL_PURE_ENERGY_PASSIVE,
	// boss_vexallus.cpp:189 enum, :208 RemoveAurasDueToSpell in JustDied;
	// Lua npc_pure_energy.lua:40 RemoveAura) — wago.tools 3.4.0.47659
	// SpellEffect row 865246 + SpellMisc 460439 (2026-11-15 browser read
	// eaa0d19d, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 42
	// PROC_TRIGGER_SPELL Die 1 Base -1 TriggerSpell 36294 TargetA 1
	// TARGET_UNIT_CASTER, chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 21 (infinite, real row), rangeIdx 1 (Self Only,
	// real row), school 0x1 Physical, dispel 0 (no SpellCategories row).
	// NOT carried: no SpellPower row; EffectBonusCoefficient 0 (no
	// field). Engine twin: aura42 live (procs.go:1287; 29908
	// precedent). FOLLOW-UP: 36294 (proc trigger) unseeded, needs its
	// own wago read. Removes the last unseeded-spell blocker for
	// npc_pure_energy.lua (single-missing-id file).
	{id: 44326, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: -1, targetA: targetCaster, trigger: 36294, chainAmplitude: 1},
	}},
	// 26232 Transform C'Thun -> Phase 2 (wago name; C++ SPELL_TRANSFORM,
	// boss_cthun.cpp:99 enum, :631 DoCast(me, SPELL_TRANSFORM, false),
	// :525 RemoveAurasDueToSpell; Lua boss_cthun.lua:109 RemoveAura) —
	// wago.tools 3.4.0.47659 SpellEffect row 702664 + SpellMisc 333817
	// (2026-11-15 browser read eaa0d19d, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 56 TRANSFORM Misc 15809 TargetA 1
	// TARGET_UNIT_CASTER, chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 21 (infinite, real row), rangeIdx 6 (Vision
	// Range, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// EffectBonusCoefficient 1 (no field). Engine twin: auraTransform
	// 56 live (spells.go:711, shapeshift.go:422). Removes the last
	// unseeded-spell blocker for boss_cthun.lua (single-missing-id file).
	{id: 26232, castTimeIdx: 1, durIdx: 21, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, targetA: targetCaster, misc: 15809, chainAmplitude: 1},
	}},
	// 39886 Give Southfury Moonstone (wago name; C++
	// SPELL_GIVE_SOUTHFURY_MOONSTONE, zone_azshara.cpp:41 enum, :168
	// CastSpell(player, ..., true); Lua npc_depth_charge.lua:137
	// triggered player cast) — wago.tools 3.4.0.47659 SpellEffect row
	// 859710 + SpellMisc 456143 (2026-11-15 browser read eaa0d19d,
	// DifficultyID 0 rows): single eff0 24 CREATE_ITEM ItemType 32364
	// TargetA 25 TARGET_UNIT_TARGET_ANY, chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration
	// row), rangeIdx 36 (Longer Range, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row. Engine twin: effCreateItem 24 live
	// (spells.go:52289 handleEffectCreateItem). Removes the last
	// unseeded-spell blocker for npc_depth_charge.lua
	// (single-missing-id file).
	{id: 39886, castTimeIdx: 1, rangeIdx: 36, school: schoolPhysical, effects: [3]effect{
		{effect: effCreateItem, targetA: targetUnitTargetAny, itemType: 32364, chainAmplitude: 1},
	}},
	// 30298 Tree Disguise (wago name; C++ SPELL_TREE_DISGUISE,
	// zone_azuremyst_isle.cpp:451 enum, :581 HasAura check; Lua
	// npc_geezle.lua:153 quest-status + HasAura gate) — wago.tools
	// 3.4.0.47659 SpellEffect rows 847496-847498 + SpellMisc 447431
	// (2026-11-15 browser read eaa0d19d, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 56 TRANSFORM Misc 17315 TargetA 1
	// TARGET_UNIT_CASTER, eff1 6 APPLY_AURA aura 26 MOD_ROOT TargetA
	// 1, eff2 61 SEND_EVENT Misc 10675 TargetA 0 (NONE), all
	// chainAmplitude 1; castTimeIdx 14 (3000ms, real row), durIdx 23
	// (90000ms, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row. Engine twins: auraTransform 56 live,
	// auraRoot 26 live (cc_break.go:104), effSendEvent 61 live
	// (spells.go:17855). Removes the last unseeded-spell blocker for
	// npc_geezle.lua (single-missing-id file).
	{id: 30298, castTimeIdx: 14, durIdx: 23, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, targetA: targetCaster, misc: 17315, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, targetA: targetCaster, chainAmplitude: 1},
		{effect: effSendEvent, misc: 10675, chainAmplitude: 1},
	}},
	// 18377 Kodo Kombobulator (wago name; C++ SPELL_KODO_KOMBO_DESPAWN_BUFF,
	// zone_desolace.cpp:52 enum; Lua npc_aged_dying_ancient_kodo.lua:107
	// triggered self-cast, HasAura-gated) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read b476939d,
	// DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 23
	// PERIODIC_TRIGGER_SPELL TriggerSpell 18269 Die 1 TargetA 1
	// TARGET_UNIT_CASTER period 300000; castTimeIdx 1 (instant, real
	// row), durIdx 5 (300000ms, real row added below), rangeIdx 1
	// (Self Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row; no
	// SpellAuraOptions row. Engine twin: auraPeriodicTrig 23 live
	// (spells.go:44959, spell_dummy.go:230). FOLLOW-UP 18269 (periodic
	// trigger) seeded below from its own wago read (e6260189).
	// Removes the last unseeded-spell blocker for
	// npc_aged_dying_ancient_kodo.lua (single-missing-id file).
	{id: 18377, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, targetA: targetCaster, auraPeriod: 300000, trigger: 18269},
	}},
	// 18269 Kodo Kombobulator (wago name; periodic trigger of 18377
	// Kodo Kombobulator, zone_desolace.cpp:52; Lua
	// npc_aged_dying_ancient_kodo.lua:107) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read e6260189,
	// DifficultyID 0 rows): single eff0 3 DUMMY aura 0 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 0, rangeIdx 1 (Self Only, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; no SpellAuraOptions row. Engine twin: effDummy
	// live (spells.go:375). NOTE: 36294 (44326 proc trigger) has NO
	// ROWS in 3.4.0.47659 (all tables, hotfixes on+off) — unseedable
	// from this build; stays unseeded.
	{id: 18269, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 18172 Kodo Kombobulator (wago name; C++ SPELL_KODO_KOMBO_PLAYER_BUFF,
	// zone_desolace.cpp:51 enum; Lua npc_aged_dying_ancient_kodo.lua:67
	// AddAura/HasAura/RemoveAura) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-15 browser read 900295ba, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 4 DUMMY TargetA 21
	// TARGET_UNIT_TARGET_ALLY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 5 (300000ms, real row), rangeIdx 6 (Vision Range
	// 100yd, real row), school 0x1 Physical, dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; no SpellAuraOptions row.
	// Engine twins: auraDummy 4 live, target 21 live
	// (chain_targets.go:350, friendly_area_targets.go:45). Removes the
	// last unseeded-spell blocker for npc_aged_dying_ancient_kodo.lua
	// (single-missing-id file).
	{id: 18172, castTimeIdx: 1, durIdx: 5, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 26476 Digestive Acid (wago name; C++ SPELL_DIGESTIVE_ACID,
	// boss_cthun.cpp:117 enum; Lua npc_eye_tentacle.lua:66 HasAura gate
	// on the pick) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 900295ba, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE Die 1 Base 149 TargetA 25
	// TARGET_UNIT_TARGET_ANY period 5000 chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row), rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x8 Nature, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 / CumulativeAura 99 (no proc table,
	// 58109 precedent). Engine twins: auraPeriodicDmg 3 live (44335/26143
	// precedent), target 25 live (spells.go:5343, chain_targets.go:360).
	// Removes the last unseeded-spell blocker for npc_eye_tentacle.lua
	// (single-missing-id file).
	{id: 26476, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 149, targetA: targetUnitTargetAny, auraPeriod: 5000, chainAmplitude: 1},
	}},
	// 25148 Sleep Visual DND (wago name; C++ SPELL_SLEEP_VISUAL,
	// zone_darkshore.cpp:51 enum; Lua npc_kerlonian.lua:66 CastSpell +
	// RemoveAura) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 900295ba, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 4 DUMMY TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21 (-1
	// infinite, real row), rangeIdx 13 (Anywhere - Unlimited, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; no SpellAuraOptions row. Engine twin: auraDummy 4
	// live. Removes the last unseeded-spell blocker for npc_kerlonian.lua
	// (single-missing-id file).
	{id: 25148, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 18953 Ranshalla Waiting (wago name; C++ SPELL_LIGHT_TORCH,
	// zone_winterspring.cpp:80 enum; Lua npc_ranshalla.lua:66 CastSpell)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15
	// browser read 900295ba, DifficultyID 0 rows): eff0 6 APPLY_AURA aura
	// 26 MOD_ROOT TargetA 1 TARGET_UNIT_CASTER, eff1 6 APPLY_AURA aura 23
	// PERIODIC_TRIGGER_SPELL TriggerSpell 18954 TargetA 1 period 300000,
	// both chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; no SpellAuraOptions row. Engine twins: auraRoot 26
	// live (cc_break.go:104), auraPeriodicTrig 23 live (spells.go:44959,
	// spell_dummy.go:230). FOLLOW-UP 18954 (periodic trigger) unseeded,
	// needs own wago read. Removes the last unseeded-spell blocker for
	// npc_ranshalla.lua (single-missing-id file).
	{id: 18953, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, targetA: targetCaster, auraPeriod: 300000, trigger: 18954, chainAmplitude: 1},
	}},
	// 20514 Ruul Snowhoof Shapechange (DND) (wago name; C++ RUUL_SHAPECHANGE,
	// zone_ashenvale.cpp:49 enum; Lua npc_ruul_snowhoof.lua:64 RemoveAura)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 57f8c4bb, DifficultyID 0 rows): single eff0 6 APPLY_AURA
	// aura 56 TRANSFORM misc 12819 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 4 (1000ms, real row), durIdx 21 (-1
	// infinite, real row), rangeIdx 1 (Self Only, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; no SpellAuraOptions row; bonus coefficient 1 (no
	// field, 26143 precedent). Engine twin: auraTransform 56 live
	// (spells.go:711, 26232 precedent). Removes the last unseeded-spell
	// blocker for npc_ruul_snowhoof.lua (single-missing-id file).
	{id: 20514, castTimeIdx: 4, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, misc: 12819, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 55761 Air Revenant Entrance (wago name; C++ SPELL_JAINA_SPAWNIN,
	// zone_orgrimmar.cpp:524 enum; Lua npc_thrall_herald_of_war.lua:79
	// CastSpell on Jaina) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 57f8c4bb, DifficultyID 0 rows):
	// single eff0 46 SPAWN TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0, rangeIdx 1 (Self Only,
	// real row), school 0x1 Physical, dispel 0 (no SpellCategories row).
	// NOT carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: effSpawn 46
	// clientside-only (spells.go:19747, 37090 precedent). Removes the last
	// unseeded-spell blocker for npc_thrall_herald_of_war.lua
	// (single-missing-id file).
	{id: 55761, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSpawn, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 42486 Ooze Channel Credit (wago name; C++ SPELL_OOZE_CHANNEL_CREDIT,
	// zone_dustwallow_marsh.cpp:94 enum; Lua zone_dustwallow_marsh.lua:10
	// triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 57f8c4bb, DifficultyID 0 rows):
	// single eff0 134 KILL_CREDIT misc 23797 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 0, rangeIdx 13 (Anywhere - Unlimited, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; no SpellAuraOptions row. Engine twin:
	// effKillCredit2 134 live (spells.go:17749, 58109 precedent). Removes
	// the last unseeded-spell blocker for zone_dustwallow_marsh.lua
	// (single-missing-id file).
	{id: 42486, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effKillCredit2, misc: 23797, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 53435 Teleport to CoT Stratholme Phase 4 (wago name; C++
	// SPELL_TELEPORT_PLAYER, culling_of_stratholme.cpp:275 enum; Lua
	// culling_of_stratholme.lua:133 CastSpell) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read 57f8c4bb,
	// DifficultyID 0 rows): single eff0 77 SCRIPT_EFFECT TargetA 25
	// TARGET_UNIT_TARGET_ANY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 0, rangeIdx 4 (Medium Range 30yd, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twin: effScriptEffect 77 live
	// (spells.go:18207, 66545 precedent). Removes the last unseeded-spell
	// blocker for culling_of_stratholme.lua (single-missing-id file).
	{id: 53435, castTimeIdx: 1, durIdx: 0, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effScriptEffect, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 66657 Parachute (wago name; no C++ enum hit; Lua
	// isle_of_conquest.lua:29 CastSpell) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-15 browser read 21bf7fe7,
	// DifficultyID 0 rows): eff0 6 APPLY_AURA aura 105 FEATHER_FALL
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1, eff1 6 APPLY_AURA
	// aura 77 MECHANIC_IMMUNITY Die 1 Base -1 MiscA 23 TargetA 1
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21 (-1
	// infinite, real row), rangeIdx 1 (Self Only, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: auraFeatherFall 105 live
	// (spells.go:637, movement.go:1437), auraMechanicImmunity 77 live
	// (spells.go:712, immunity.go:1368; twin reads eff.MiscValue, so the
	// mechanic rides misc: 23 — no effect mechanic field set). Removes
	// the last unseeded-spell blocker for isle_of_conquest.lua
	// (single-missing-id file).
	{id: 66657, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraFeatherFall, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunity, dieSides: 1, basePoints: -1, misc: 23, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 17534 Healing Potion (wago name; C++ SPELL_HEALING_POTION,
	// zone_howling_fjord.cpp:39 enum; Lua npc_apothecary_hanes.lua:70
	// CastSpell) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 21bf7fe7, DifficultyID 0 rows): single
	// eff0 10 HEAL Die 701 Base 1049 TargetA 1 TARGET_UNIT_CASTER
	// Amplitude 1 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 0, rangeIdx 1 (Self Only, real row), school 0x1 Physical,
	// dispel 0 (SpellCategories DispelType 0). NOT carried: no SpellPower
	// row; no SpellAuraOptions row; SpellCategories Category 4 /
	// DefenseType 1 (no category/defense fields in the seed struct,
	// 30621 cat270 precedent); Amplitude 1 (no amplitude field, 29907
	// precedent). Engine twin: effHeal 10 live. Removes the last
	// unseeded-spell blocker for npc_apothecary_hanes.lua
	// (single-missing-id file).
	{id: 17534, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effHeal, dieSides: 701, basePoints: 1049, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 45631 Cosmetic - Enslave Chains Self (wago name; C++
	// SPELL_COSMETIC_ENSLAVE_CHAINS_SELF, zone_borean_tundra.cpp:1090
	// enum; Lua npc_beryl_sorcerer.lua:59 CastSpell) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// 21bf7fe7, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 4
	// DUMMY TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row), rangeIdx 1
	// (Self Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109 precedent).
	// Engine twin: auraDummy 4 live. Removes the last unseeded-spell
	// blocker for npc_beryl_sorcerer.lua (single-missing-id file).
	{id: 45631, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 26134 Eye Beam (wago SpellName 3.4.0.47659; C++ SPELL_GREEN_BEAM,
	// boss_cthun.cpp:85 enum; Lua npc_giant_eye_tentacle.lua:16
	// CastSpell) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 91b3e4be, DifficultyID 0 rows): single
	// eff0 2 SCHOOL_DAMAGE Die 751 Base 2624 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY ChainAmplitude 1.5 ChainTargets 60;
	// castTimeIdx 5 (2000ms, real row), durIdx 0, rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x8 Nature, dispel 0 /
	// mechanic 0 (SpellCategories DispelType 0 / Mechanic 0;
	// DefenseType 1 / PreventionType 0x1 have no seed fields, 30621
	// cat270 precedent). NOT carried: BonusCoefficient 1 (no field,
	// 26143 precedent); no SpellPower / SpellAuraOptions rows. The
	// seed carries no name field, so the wago "Eye Beam" vs C++
	// "Green Beam" naming gap is comment-only. Engine twin:
	// effSchoolDamage 2 live. Removes the last unseeded-spell blocker
	// for npc_giant_eye_tentacle.lua (single-missing-id file).
	{id: 26134, castTimeIdx: 5, durIdx: 0, rangeIdx: 13, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 751, basePoints: 2624, targetA: targetEnemy, chainTargets: 60, chainAmplitude: 1.5},
	}},
	// 70227 Empowered Blood (wago name; C++ SPELL_EMPOWERED_BLOOD,
	// icecrown_citadel.h:53 enum / .cpp:1166; Lua
	// go_empowering_blood_orb.lua:56 player CastSpell) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15 browser
	// read 91b3e4be, DifficultyID 0 rows): eff0 6 APPLY_AURA aura 79
	// MOD_DAMAGE_PERCENT_DONE Die 1 Base 99 MiscA 127 TargetA 25
	// TARGET_UNIT_TARGET_ANY chainAmplitude 1, eff1 6 APPLY_AURA aura
	// 4 DUMMY TargetA 38 TARGET_UNIT_NEARBY_ENTRY chainAmplitude 1
	// (MiscValue 0 as read), eff2 6 APPLY_AURA aura 65
	// MOD_CASTING_SPEED_NOT_STACK Die 1 Base 99999 TargetA 25
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 9
	// (30000ms, real row), rangeIdx 3 (Twenty yards, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101
	// (no proc table, 58109 precedent). New const
	// auraModCastingSpeed 65 (SpellAuraDefines.h:145). Engine twins:
	// auraModDmgDone 79 / auraDummy 4 / auraModCastingSpeed 65 live
	// (aura_damage.go:119, aura_haste.go:186), target 38 live
	// (chain_targets.go:82). Removes the last unseeded-spell blocker
	// for go_empowering_blood_orb.lua (single-missing-id file).
	{id: 70227, castTimeIdx: 1, durIdx: 9, rangeIdx: 3, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 99, misc: 127, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDummy, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModCastingSpeed, dieSides: 1, basePoints: 99999, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 70860 Frozen Throne Teleport (wago name; C++ FROZEN_THRONE_
	// TELEPORT, icecrown_citadel.h:67 enum; Lua
	// go_icecrown_teleport.lua:107 player CastSpell) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15 browser
	// read 91b3e4be, DifficultyID 0 rows): eff0 3 DUMMY TargetA 46
	// TARGET_DEST_NEARBY_ENTRY chainAmplitude 1, eff1 5
	// TELEPORT_UNITS (wago shows the effect-5 name blank) Die 1
	// TargetA 25 TARGET_UNIT_TARGET_ANY TargetB 87 TARGET_DEST_DEST
	// chainAmplitude 1, eff2 6 APPLY_AURA aura 12 MOD_STUN TargetA
	// 25 chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx
	// 39 (2000ms, real row), rangeIdx 13 (Anywhere - Unlimited, real
	// row), school 0x10 Frost, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101
	// (no proc table, 58109 precedent). Engine twins: effDummy 3 /
	// effTeleportUnits 5 (spells.go:431/:17364) / auraStun 12 live,
	// targets 46/87 live (creature_dest_targets.go:31/:466).
	// Removes the last unseeded-spell blocker for
	// go_icecrown_teleport.lua (single-missing-id file).
	{id: 70860, castTimeIdx: 1, durIdx: 39, rangeIdx: 13, school: schoolFrost, effects: [3]effect{
		{effect: effDummy, targetA: targetDestNearbyEntry, chainAmplitude: 1},
		{effect: effTeleportUnits, dieSides: 1, targetA: targetUnitTargetAny, targetB: targetDestDest, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 72915 Arthas Teleporter Ceremony (wago name; C++
	// SPELL_ARTHAS_TELEPORTER_CEREMONY, icecrown_citadel.h:45 enum;
	// Lua instance_icecrown_citadel.lua:174) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read 91b3e4be,
	// DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 4 DUMMY
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row),
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical, dispel
	// 0 (no SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent). Engine twin: auraDummy 4 live. Removes the last
	// unseeded-spell blocker for instance_icecrown_citadel.lua
	// (single-missing-id file).
	{id: 72915, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 58854 Resurrection (wago name; C++ SPELL_RESURRECTION,
	// zone_storm_peaks.cpp:449 enum; Lua
	// npc_brann_bronzebeard_keystone.lua:49 const, :127 self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 618c701b, DifficultyID 0 rows): single eff0 3 DUMMY
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 0, rangeIdx 1 (Self Only, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: effDummy 3 live.
	// Removes the last unseeded-spell blocker for
	// npc_brann_bronzebeard_keystone.lua (single-missing-id file).
	{id: 58854, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 47184 Mrfloppy (wago name; C++ SPELL_MRFLOPPY,
	// zone_grizzly_hills.cpp:43 enum; Lua npc_emily.lua:140 const,
	// :265 CastSpell on Mrfloppy) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-15 browser read 618c701b,
	// DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 236
	// CONTROL_VEHICLE TargetA 38 TARGET_UNIT_NEARBY_ENTRY
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 13 (Anywhere - Unlimited,
	// real row), school 0x1 Physical, dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent). Engine twin:
	// auraControlVehicle 236 live (spells.go:647, vehicle.go:2699),
	// target 38 live. Removes the last unseeded-spell blocker for
	// npc_emily.lua (single-missing-id file).
	{id: 47184, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraControlVehicle, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 69424 Icicle Summon (wago name; C++ SPELL_ICICLE_SUMMON,
	// pit_of_saron.h:107 enum; pit_of_saron.cpp:49 triggered self-cast;
	// Lua instance_pit_of_saron.lua:122 const, :187 triggered CastSpell)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15
	// browser read 618c701b, DifficultyID 0 rows): eff0 28 SUMMON
	// MiscValue 36847 MiscValueB 1362 TargetA 18 TARGET_DEST_CASTER
	// chainAmplitude 1, eff1 27 PERSISTENT_AREA_AURA aura 4 DUMMY
	// TargetA 18 RadiusIndex 36 chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 31 (8000ms, real row), rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x1 Physical, dispel 0
	// (no SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent). SummonProperties 1362 (Control 0 / Faction 0 /
	// Title 0 / Slot 0 / Flags 0x800) seeded as a real row below
	// (wago.tools 3.4.0.47659 SummonProperties read, browser
	// 7f680372); the engine reads it via eff.MiscValueB
	// (spells.go:6401). Engine twins: effSummon 28 / effPersistent-
	// AreaAura 27 live, auraDummy 4 live, target 18 live. Removes
	// the last unseeded-spell blocker for instance_pit_of_saron.lua
	// (single-missing-id file).
	{id: 69424, castTimeIdx: 1, durIdx: 31, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, targetA: targetDestCaster, misc: 36847, miscB: 1362, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraDummy, targetA: targetDestCaster, radiusIdx: 36, chainAmplitude: 1},
	}},
	// 70460 Coldflame Jets (wago name; C++ SPELL_COLDFLAME_JETS,
	// icecrown_citadel.cpp:85 enum; Lua npc_frost_freeze_trap.lua:15
	// const, :5 DoCastSelf non-triggered 22s re-arm) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser
	// read, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 227
	// PERIODIC_TRIGGER_SPELL_WITH_VALUE period 250 basePoints 9999
	// dieSides 1 trigger 70461 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 5 (2000ms, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 1 (Self Only, real row),
	// school 0x10 Frost, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin:
	// auraPeriodicTriggerSpellWithValue 227 live (spells.go:38227,
	// handled :21104), target 1 live. Removes the last
	// unseeded-spell blocker for npc_frost_freeze_trap.lua
	// (single-missing-id file).
	{id: 70460, castTimeIdx: 5, durIdx: 21, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrigWithValue, auraPeriod: 250, basePoints: 9999, dieSides: 1, trigger: 70461, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64668 Magnetic Field (wago name; C++ SPELL_MAGNETIC_FIELD,
	// boss_mimiron.cpp:162 enum; Lua npc_mimiron_assault_bot.lua:8
	// const, :42 30s re-arm) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-15 browser read, DifficultyID 0 rows):
	// eff0 6 APPLY_AURA aura 26 MOD_ROOT TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1, eff1 6 APPLY_AURA
	// aura 87 MOD_DAMAGE_PERCENT_TAKEN basePoints 29 dieSides 1
	// misc 127 TargetA 6 chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 32 (6000ms, real row), rangeIdx 4 (Medium
	// Range, real row), school 0x40 Arcane, dispel 1 (MAGIC,
	// SpellCategories row), mechanic 7 (ROOT, SpellCategories row).
	// NOT carried: no SpellPower row; SpellAuraOptions ProcChance 101
	// (no proc table, 58109 precedent). Engine twins: auraRoot 26 /
	// auraModDmgTaken 87 live, target 6 live. Removes the last
	// unseeded-spell blocker for npc_mimiron_assault_bot.lua
	// (single-missing-id file).
	{id: 64668, dispel: 1, mechanic: 7, castTimeIdx: 1, durIdx: 32, rangeIdx: 4, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, mechanic: 7, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: 29, dieSides: 1, mechanic: 7, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 63767 Bomb Bot (wago name — NOT "Bomb Bot Aura"; C++
	// SPELL_BOMB_BOT_AURA, boss_mimiron.cpp:165 enum; Lua
	// npc_mimiron_bomb_bot.lua:24 const, creature_template_addon aura)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-15 browser read, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 54 MOD_HIT_CHANCE basePoints -101 dieSides 1
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1, eff1 6 APPLY_AURA
	// aura 42 PROC_TRIGGER_SPELL trigger 63801 TargetA 1
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 0,
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical, dispel 0
	// (no SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 100 (not 101; no proc table, 58109
	// precedent). Engine twins: auraModHitChance 54 live
	// (spellAuraModHitChance, spells.go:619, used :34533),
	// auraProcTriggerSpell 42 live, target 1 live. Removes the last
	// unseeded-spell blocker for npc_mimiron_bomb_bot.lua
	// (single-missing-id file).
	{id: 63767, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModHitChance, basePoints: -101, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, trigger: 63801, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64562 Summon Flames Spread Trigger (wago name; C++
	// SPELL_SUMMON_FLAMES_SPREAD_TRIGGER, boss_mimiron.cpp:132 enum;
	// Lua npc_mimiron_flames.lua:16 const, :66 DoCastAOE one-shot) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read, DifficultyID 0 rows): single eff0 6 APPLY_AURA
	// aura 23 PERIODIC_TRIGGER_SPELL period 1000 basePoints -1
	// dieSides 1 trigger 64564 TargetA 22 TARGET_SRC_CASTER TargetB
	// 15 TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex 28 chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 36 (1000ms, real row
	// added below), rangeIdx 13 (Anywhere - Unlimited, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: auraPeriodicTrig 23
	// live, targets 22/15 live, radius 28 pre-existing (50000).
	// Removes the last unseeded-spell blocker for
	// npc_mimiron_flames.lua (single-missing-id file).
	{id: 64562, castTimeIdx: 1, durIdx: 36, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, basePoints: -1, dieSides: 1, trigger: 64564, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 28, chainAmplitude: 1},
	}},
	// 45532 Freed Warsong Peon (wago name; C++
	// SPELL_FREED_WARSONG_PEON, zone_borean_tundra.cpp:117 enum; Lua
	// npc_nerubar_victim.lua:35 const, :16 player triggered cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 44ba3f77, DifficultyID 0 rows): single eff0 28
	// SUMMON misc 25270 miscB 64 TargetA 53 TARGET_DEST_TARGET_ENEMY
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 25
	// (180000ms, real row), rangeIdx 37 (Extended Range 0-50yd, real
	// row added below), school 0x1 Physical, dispel 8 (ROOT,
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent); CumulativeAura 3 (no field). Engine twins:
	// effSummon 28 live (spells.go:6395, miscB=SummonProperties id),
	// target 53 live (target_dest_targets.go:13), SummonProperties 64
	// real row. Removes the last unseeded-spell blocker for
	// npc_nerubar_victim.lua (single-missing-id file).
	{id: 45532, dispel: 8, castTimeIdx: 1, durIdx: 25, rangeIdx: 37, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, targetA: targetDestTargetEnemy, misc: 25270, miscB: 64, chainAmplitude: 1},
	}},
	// 46104 Trapped! (wago name, exclamation mark; C++ SPELL_TRAPPED,
	// zone_borean_tundra.cpp:188 enum; Lua
	// npc_nesingwary_trapper.lua:58 const, :29 Phase 8 triggered
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 44ba3f77, DifficultyID 0 rows): single
	// eff0 1 INSTAKILL TargetA 25 TARGET_UNIT_TARGET_ANY
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 0,
	// rangeIdx 12 (Interact Range 0-5yd, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effInstakill 1 live
	// (positivity.go:34, spells.go:24311), target 25 live. Removes the
	// last unseeded-spell blocker for npc_nesingwary_trapper.lua
	// (single-missing-id file).
	{id: 46104, castTimeIdx: 1, durIdx: 0, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effInstakill, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 68424 Wilfred Portal (wago name; C++ SPELL_WILFRED_PORTAL,
	// trial_of_the_crusader.h:81 enum; Lua
	// npc_open_portal_target_toc.lua:31 const, :20 non-triggered cast
	// + DespawnOrUnsummon) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 44ba3f77, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 4 DUMMY TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 21 (-1 infinite, real row), rangeIdx 1 (Self
	// Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent). Engine twin: auraDummy 4 live, target 1 live.
	// Removes the last unseeded-spell blocker for
	// npc_open_portal_target_toc.lua (single-missing-id file).
	{id: 68424, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 56142 Summon Power Spark (wago name — NOT "Summon Power Park";
	// C++ SPELL_SUMMON_POWER_PARK, boss_malygos.cpp:108 enum; Lua
	// npc_portal_eoe.lua:31 const, :8 triggered cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser
	// read 44ba3f77, DifficultyID 0 rows): single eff0 28 SUMMON misc
	// 30084 miscB 1362 dieSides 1 TargetA 18 TARGET_DEST_CASTER
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 23
	// (90000ms, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; SpellAuraOptions ProcChance 101 (no proc
	// table, 58109 precedent). Engine twins: effSummon 28 live,
	// target 18 live, SummonProperties 1362 real row (e33ebcb8).
	// Removes the last unseeded-spell blocker for npc_portal_eoe.lua
	// (single-missing-id file).
	{id: 56142, castTimeIdx: 1, durIdx: 23, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestCaster, misc: 30084, miscB: 1362, chainAmplitude: 1},
	}},
	// 55852 Power Spark Death (wago name; C++ SPELL_POWER_SPARK_DEATH,
	// boss_malygos.cpp:109 enum; Lua npc_power_spark.lua:26 const, :31
	// JustDied triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-15 browser read 555b443f, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 23 PERIODIC_TRIGGER_SPELL
	// period 1000 basePoints -1 dieSides 1 trigger 55849 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 3 (60000ms, real row), rangeIdx 1 (Self Only,
	// real row), school 0x1 Physical, dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent). Engine twin:
	// auraPeriodicTrig 23 live, target 1 live. Removes the last
	// unseeded-spell blocker for npc_power_spark.lua
	// (single-missing-id file).
	{id: 55852, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, basePoints: -1, dieSides: 1, trigger: 55849, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 21343 Snowball (wago name; C++ SPELL_SNOWBALL,
	// childrens_week.cpp:130 enum; Lua npc_snowfall_glade_playmate.lua:34
	// const, :151/:158 non-triggered casts) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read 555b443f,
	// DifficultyID 0 rows): single eff0 3 DUMMY TargetA 25
	// TARGET_UNIT_TARGET_ANY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 0 (no SpellDuration row), rangeIdx 4 (Medium
	// Range, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row; Category 1197 NOT carried, no field, 30621
	// precedent). NOT carried: no SpellPower row; no SpellAuraOptions
	// row; Speed 25 (no speed field). Engine twins: effDummy 3 live,
	// target 25 live. Removes the last unseeded-spell blocker for
	// npc_snowfall_glade_playmate.lua (single-missing-id file).
	{id: 21343, castTimeIdx: 1, durIdx: 0, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 55708 Storm Visual (wago name; C++ STORM_VISUAL,
	// zone_zuldrak.cpp:905 enum; Lua npc_storm_cloud.lua:150 const, :161
	// triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 555b443f, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 4 DUMMY TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 21 (-1 infinite, real row), rangeIdx 1 (Self
	// Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent). Engine twin: auraDummy 4 live, target 1 live.
	// Removes the last unseeded-spell blocker for npc_storm_cloud.lua
	// (single-missing-id file).
	{id: 55708, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 43289 Summon Freed Mist Whisper Scout (wago name; C++
	// SPELL_SUMMON_FREED_MIST_WHISPER_SCOUT, zone_howling_fjord.cpp:530
	// enum; Lua npc_riven_widow_cocoon.lua:31 const, :57 player
	// triggered cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 555b443f, DifficultyID 0
	// rows): single eff0 28 SUMMON misc 24211 miscB 66 TargetA 63
	// TARGET_DEST_TARGET_ANY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 3 (60000ms, real row), rangeIdx 13 (Anywhere -
	// Unlimited, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent). Engine twins: effSummon 28 live (spells.go:6395,
	// miscB=SummonProperties id), target 63 live
	// (target_dest_targets.go:14), SummonProperties 66 real row added
	// below (wago.tools 3.4.0.47659 SummonProperties read, browser
	// 3ce851bf: Control 0 / Faction 0 / Title 0 / Slot 0, Flags 0x1).
	// Removes the last unseeded-spell blocker for
	// npc_riven_widow_cocoon.lua (single-missing-id file).
	{id: 43289, castTimeIdx: 1, durIdx: 3, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, targetA: targetDestTargetAny, misc: 24211, miscB: 66, chainAmplitude: 1},
	}},
	// 73036 Essence of the Captured (wago name; C++
	// SPELL_ESSENCE_OF_CAPTURED_1, halls_of_reflection.cpp:2412 enum;
	// Lua npc_uther_quel_delar.lua:116 triggered self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// 7accea68, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 23
	// PERIODIC_TRIGGER_SPELL period 330 basePoints -1 dieSides 1 trigger
	// 73035 TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 35 (4000ms, real row), rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109 precedent).
	// Engine twin: auraPeriodicTrig 23 live, target 1 live. Removes the
	// last unseeded-spell blocker for npc_uther_quel_delar.lua
	// (single-missing-id file).
	{id: 73036, castTimeIdx: 1, durIdx: 35, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 330, basePoints: -1, dieSides: 1, trigger: 73035, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 57804 Control Crystal Activation (wago name; C++
	// SPELL_CRYSTAL_ACTIVATION, violet_hold.cpp:136 enum; Lua
	// npc_violet_hold_activation_crystal.lua:41 player triggered cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 7accea68, DifficultyID 0 rows): single eff0 61
	// SEND_EVENT misc 20001 TargetA 0 TargetB 0 chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration row),
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOTE: wago labels effect 61 "SCRIPT_EFFECT"
	// but SharedDefines.h:872 is SPELL_EFFECT_SEND_EVENT 61 (SCRIPT_EFFECT
	// is 77, :888); the number is authoritative, seeded as effSendEvent.
	// NOT carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: spellEffectSendEvent 61
	// live (spells.go:384/:17855). Removes the last unseeded-spell blocker
	// for npc_violet_hold_activation_crystal.lua (single-missing-id file).
	{id: 57804, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSendEvent, misc: 20001, chainAmplitude: 1},
	}},
	// 22011 Spirit Heal Channel (wago name; C++ SPELL_CHANNEL_SPIRIT_HEAL,
	// zone_wintergrasp.cpp:66 enum; Lua npc_wg_spirit_guide.lua:11) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 7accea68, DifficultyID 0 rows): single eff0 6 APPLY_AURA
	// aura 23 PERIODIC_TRIGGER_SPELL period 30000 trigger 22012 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 9 (30000ms, real row), rangeIdx 1 (Self Only, real
	// row), school 0x2 Holy, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; no SpellAuraOptions row. Engine twin:
	// auraPeriodicTrig 23 live, target 1 live. Removes the last
	// unseeded-spell blocker for npc_wg_spirit_guide.lua
	// (single-missing-id file).
	{id: 22011, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolHoly, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 30000, trigger: 22012, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12544 Frost Armor (wago name; C++ SPELL_FROST_ARMOR,
	// zone_wintergrasp.cpp:261 enum; Lua npc_wg_queue.lua:77
	// non-triggered re-arm) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-15 browser read 7accea68, DifficultyID 0 rows):
	// eff0 6 APPLY_AURA aura 22 MOD_RESISTANCE dieSides 1 basePoints 11
	// misc 1 TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; eff1 6
	// APPLY_AURA aura 42 PROC_TRIGGER_SPELL trigger 6136 TargetA 1
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 30
	// (1800000ms, real row, browser c857c690), rangeIdx 1 (Self Only, real
	// row), school 0x10 Frost, dispel 1 (SpellCategories row). NOT carried:
	// eff0 EffectRealPointsPerLevel 12 (seed has no field); SpellPower
	// CostPerLevel 5 (seed has no field); SpellCategories Category 0 /
	// DefenseSkill 1 / Prevention 1 / StartRecovery 133 (no fields, 30621
	// precedent); SpellAuraOptions ProcChance 100 (no proc table, 58109
	// precedent). Engine twins: auraModResistance 22 live
	// (SpellAuraDefines.h:102), auraProcTriggerSpell 42 live, target 1
	// live. Removes the last unseeded-spell blocker for npc_wg_queue.lua
	// (single-missing-id file).
	{id: 12544, dispel: 1, manaCost: 10, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effApplyAura, aura: auraModResistance, dieSides: 1, basePoints: 11, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, trigger: 6136, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 75416 Rally (wago name; C++ SPELL_RALLY, ruby_sanctum.cpp:53 enum;
	// Lua ruby_sanctum.lua:22 triggered self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// a121c15b, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 79
	// MOD_DAMAGE_PERCENT_DONE dieSides 1 basePoints 24 misc 127 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 1 (10000ms, real row), rangeIdx 1 (Self Only, real
	// row), school 0x1 Physical, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: auraModDmgDone 79 live
	// (aura_damage.go:119), target 1 live. Removes the last
	// unseeded-spell blocker for ruby_sanctum.lua (single-missing-id
	// file).
	{id: 75416, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 24, misc: 127, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64619 Water Spray (wago name; C++ SPELL_WATER_SPRAY,
	// boss_mimiron.cpp:153 enum; Lua spell_mimiron_fire_search.lua:27
	// triggered on hit unit) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-15 browser read a121c15b, DifficultyID 0 rows):
	// eff0 3 DUMMY TargetA 60 TARGET_UNIT_CONE_ENTRY RadiusIndex 18;
	// eff1 98 KNOCK_BACK dieSides 1 basePoints 199 misc 100 TargetA 24
	// TARGET_UNIT_CONE_ENEMY_24 RadiusIndex 18; eff2 2 SCHOOL_DAMAGE
	// dieSides 2301 basePoints 18849 TargetA 24 RadiusIndex 18;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration
	// row), rangeIdx 1 (Self Only, real row), school 0x10 Frost, dispel
	// 0 (no SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109 precedent).
	// Engine twins: effDummy 3 / effKnockBack 98 / effSchoolDamage 2
	// live, targets 60/24 live (implicitTargetConeEntry 60,
	// friendly_nearby_cone_targets.go:19). Removes the last
	// unseeded-spell blocker for spell_mimiron_fire_search.lua
	// (single-missing-id file).
	{id: 64619, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effDummy, targetA: targetUnitConeEntry, radiusIdx: 18, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 199, misc: 100, targetA: targetUnitConeEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 2301, basePoints: 18849, targetA: targetUnitConeEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 63666 Napalm Shell (wago name; C++ SPELL_NAPALM_SHELL,
	// boss_mimiron.cpp:88 enum; Lua spell_mimiron_napalm_shell.lua:21) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15
	// browser read a121c15b, DifficultyID 0 rows): eff0 2 SCHOOL_DAMAGE
	// dieSides 921 basePoints 7539 TargetA 53 TARGET_DEST_TARGET_ENEMY
	// TargetB 16 TARGET_UNIT_DEST_AREA_ENEMY RadiusIndex 8; eff1 6
	// APPLY_AURA aura 3 PERIODIC_DAMAGE period 1000 dieSides 1
	// basePoints 3999 TargetA 53 TargetB 16 RadiusIndex 8; castTimeIdx
	// 5 (2000ms, real row), durIdx 31 (8000ms, real row), rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x4 Fire, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109 precedent).
	// Engine twins: effSchoolDamage 2 / auraPeriodicDmg 3 live, targets
	// 53/16 live. Removes the last unseeded-spell blocker for
	// spell_mimiron_napalm_shell.lua (single-missing-id file).
	{id: 63666, castTimeIdx: 5, durIdx: 31, rangeIdx: 13, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 921, basePoints: 7539, targetA: targetDestTargetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 1000, dieSides: 1, basePoints: 3999, targetA: targetDestTargetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 62997 Plasma Blast (wago name; C++ SPELL_PLASMA_BLAST,
	// boss_mimiron.cpp:89 enum; Lua spell_mimiron_plasma_blast.lua:17
	// non-triggered) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read a121c15b, DifficultyID 0 rows):
	// single eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE period 1000
	// dieSides 1 basePoints 16999 TargetA 6 TARGET_UNIT_TARGET_ENEMY
	// chainAmplitude 1; castTimeIdx 14 (3000ms, real row), durIdx 32
	// (6000ms, real row), rangeIdx 6 (Vision Range, real row), school
	// 0x44 Spellfire (Arcane 64 + Fire 4), dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent). Engine twin:
	// auraPeriodicDmg 3 live, target 6 live. Removes the last
	// unseeded-spell blocker for spell_mimiron_plasma_blast.lua
	// (single-missing-id file).
	{id: 62997, castTimeIdx: 14, durIdx: 32, rangeIdx: 6, school: schoolSpellfire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 1000, dieSides: 1, basePoints: 16999, targetA: targetEnemy, chainAmplitude: 1},
	}},

	// 64427 Summon Scrap Bot (wago name; C++ SPELL_SUMMON_ASSAULT_BOT,
	// boss_mimiron.cpp:161 enum; Lua spell_mimiron_summon_assault_bot.lua:18
	// non-triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 51c466ff, DifficultyID 0 rows):
	// single eff0 28 SUMMON dieSides 1 basePoints 0 TargetA 18
	// TARGET_DEST_CASTER misc 34057 miscB 64 (SummonProperties 64, real
	// row) chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effSummon 28 / targetDestCaster 18
	// live. Removes the last unseeded-spell blocker for
	// spell_mimiron_summon_assault_bot.lua (single-missing-id file).
	{id: 64427, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestCaster, misc: 34057, miscB: 64, chainAmplitude: 1},
	}},
	// 64426 Summon Scrap Bot dummy aura (wago name; C++
	// SPELL_SUMMON_ASSAULT_BOT_DUMMY, boss_mimiron.cpp:160 enum; Lua
	// spell_mimiron_summon_assault_bot_target.lua:6 triggered self-cast on
	// hit unit; C++ spell script boss_mimiron.cpp:2434-2481) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// 51c466ff, DifficultyID 0 rows): single eff0 6 APPLY_AURA aura 4
	// DUMMY TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 31 (8000ms, real row), rangeIdx 1 (Self
	// Only, real row), school 0x1 Physical, dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent). Engine twin: auraDummy 4
	// live. Removes the last unseeded-spell blocker for
	// spell_mimiron_summon_assault_bot_target.lua (single-missing-id file).
	{id: 64426, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64622 Summon Fire Bot (wago name; C++ SPELL_SUMMON_FIRE_BOT,
	// boss_mimiron.cpp:147 enum; Lua spell_mimiron_summon_fire_bot.lua:18
	// non-triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 51c466ff, DifficultyID 0 rows):
	// single eff0 28 SUMMON dieSides 1 basePoints 0 TargetA 18
	// TARGET_DEST_CASTER misc 34147 miscB 64 (SummonProperties 64, real
	// row) chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effSummon 28 / targetDestCaster 18
	// live. Removes the last unseeded-spell blocker for
	// spell_mimiron_summon_fire_bot.lua (single-missing-id file).
	{id: 64622, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestCaster, misc: 34147, miscB: 64, chainAmplitude: 1},
	}},
	// 64621 Summon Fire Bot dummy aura (wago name; C++
	// SPELL_SUMMON_FIRE_BOT_DUMMY, boss_mimiron.cpp:148 enum; Lua
	// spell_mimiron_summon_fire_bot_target.lua:6 triggered self-cast on
	// hit unit) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 51c466ff, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 4 DUMMY TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 31
	// (8000ms, real row), rangeIdx 1 (Self Only, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twin: auraDummy 4 live. Removes the last
	// unseeded-spell blocker for spell_mimiron_summon_fire_bot_target.lua
	// (single-missing-id file).
	{id: 64621, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},

	// 64627 Summon Frost Bomb (wago name; C++ SPELL_SUMMON_FROST_BOMB,
	// boss_mimiron.cpp:142 enum, cast by VX-001; Lua
	// spell_mimiron_summon_frost_bomb_target.lua:20) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15 browser read
	// 21144a37, DifficultyID 0 rows): eff0 28 SUMMON dieSides 1
	// basePoints 0 TargetA 63 TARGET_DEST_TARGET_ANY misc 34149 miscB 64
	// (SummonProperties 64, real row) chainAmplitude 1; eff1 3 DUMMY
	// TargetA 25 TARGET_UNIT_TARGET_ANY chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row), rangeIdx 13
	// (Anywhere - Unlimited, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row; SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent). Engine twins:
	// effSummon 28 / effDummy 3 / targets 63/25 live. Removes the last
	// unseeded-spell blocker for spell_mimiron_summon_frost_bomb_target.lua
	// (single-missing-id file).
	{id: 64627, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestTargetAny, misc: 34149, miscB: 64, chainAmplitude: 1},
		{effect: effDummy, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 63819 Summon Junk Bot (wago name; C++ SPELL_SUMMON_JUNK_BOT,
	// boss_mimiron.cpp:155 enum; Lua spell_mimiron_summon_junk_bot.lua:18
	// non-triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 21144a37, DifficultyID 0 rows):
	// single eff0 28 SUMMON dieSides 1 basePoints 0 TargetA 18
	// TARGET_DEST_CASTER misc 33855 miscB 64 (SummonProperties 64, real
	// row) chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 21
	// (-1 infinite, real row), rangeIdx 1 (Self Only, real row), school
	// 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effSummon 28 / targetDestCaster 18
	// live. Removes the last unseeded-spell blocker for
	// spell_mimiron_summon_junk_bot.lua (single-missing-id file).
	{id: 63819, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, targetA: targetDestCaster, misc: 33855, miscB: 64, chainAmplitude: 1},
	}},
	// 64398 Summon Junk Bot Dummy (C++ SPELL_SUMMON_JUNK_BOT_DUMMY,
	// boss_mimiron.cpp:157 enum; Lua spell_mimiron_summon_junk_bot_target.lua:16
	// triggered self-cast on hit unit; C++ spell script boss_mimiron.cpp:2689;
	// wago "Summon Scrap Bot" name NOT verified — SpellName lookup returned
	// no entries) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 21144a37, DifficultyID 0 rows): single eff0
	// 6 APPLY_AURA aura 4 DUMMY TargetA 1 TARGET_UNIT_CASTER chainAmplitude
	// 1; castTimeIdx 1 (instant, real row), durIdx 31 (8000ms, real row),
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: no SpellPower row; SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent). Engine twin:
	// auraDummy 4 live. Removes the last unseeded-spell blocker for
	// spell_mimiron_summon_junk_bot_target.lua (single-missing-id file).
	{id: 64398, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 46378 Windsoul Credit (wago name; C++ SPELL_WINDSOUL_CREDT — enum
	// typo, zone_borean_tundra.cpp:1666; Lua zone_borean_tundra.lua:21
	// non-triggered nil-target cast) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-15 browser read 21144a37, DifficultyID 0
	// rows): single eff0 134 KILL_CREDIT2 TargetA 27 TARGET_UNIT_MASTER
	// misc 25987 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 0 (no SpellDuration row), rangeIdx 13 (Anywhere - Unlimited,
	// real row), school 0x1 Physical, dispel 0 (no SpellCategories row).
	// NOT carried: no SpellPower row; SpellAuraOptions ProcChance 101 (no
	// proc table, 58109 precedent). Engine twin: effKillCredit2 134 live
	// (spells.go:17749 handleEffectKillCreditGroup). Removes the last
	// unseeded-spell blocker for zone_borean_tundra.lua (single-missing-id
	// file).
	{id: 46378, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effKillCredit2, targetA: targetUnitMaster, misc: 25987, chainAmplitude: 1},
	}},

	// 34603 Sunfury Disguise (wago name; C++ SPELL_SUNFURY_DISGUISE,
	// zone_netherstorm.cpp:60 enum; Lua npc_commander_dawnforge.lua:53) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15
	// browser read 60cdf9af, DifficultyID 0 rows): eff0 6 APPLY_AURA aura
	// 139 FORCE_REACTION dieSides 1 basePoints 3 misc 968 TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; eff1 6 APPLY_AURA aura 56
	// TRANSFORM dieSides 0 basePoints 0 misc 18850 TargetA 1
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 6
	// (600000ms, real row), rangeIdx 12 (Interact Range, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: auraTransform 56 live (prior
	// in-tree note); auraForceReaction 139 has NO Go model
	// (conditions.go:1746, explicit_target_faction.go:921) — the eff0 leg
	// stays dormant data until one lands. Removes the last unseeded-spell
	// blocker for npc_commander_dawnforge.lua (single-missing-id file).
	{id: 34603, castTimeIdx: 1, durIdx: 6, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraForceReaction, dieSides: 1, basePoints: 3, misc: 968, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, misc: 18850, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12491 Healing Wave (wago name; C++ SPELL_HEALING_WAVE,
	// zone_nagrand.cpp:56 enum, also SPELL_KUR_HEALING_WAVE :244 same id;
	// Lua npc_earthmender_wilda.lua:63 non-triggered) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// 60cdf9af, DifficultyID 0 rows): single eff0 10 HEAL dieSides 57
	// basePoints 357 TargetA 21 TARGET_UNIT_TARGET_ALLY chainAmplitude 1;
	// castTimeIdx 14 (3000ms, real row), durIdx 0 (no SpellDuration row),
	// rangeIdx 5 (Long Range, real row), school 0x8 Nature, dispel 0 /
	// mechanic 0 (SpellCategories row exists but DispelType 0, Mechanic
	// 0). SpellPower ManaCost 145 carried (12544 precedent); no
	// SpellAuraOptions row, no ProcChance. Engine twin: effHeal 10 live.
	// Removes the last unseeded-spell blocker for
	// npc_earthmender_wilda.lua (single-missing-id file).
	{id: 12491, castTimeIdx: 14, durIdx: 0, rangeIdx: 5, school: schoolNature, manaCost: 145, effects: [3]effect{
		{effect: effHeal, dieSides: 57, basePoints: 357, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 38775 Force of Neltharaku (wago name; C++ SPELL_FORCE_OF_NELTHARAKU,
	// zone_shadowmoon_valley.cpp:345 enum; Lua
	// npc_enslaved_netherwing_drake.lua:44) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-15 browser read 60cdf9af,
	// DifficultyID 0 rows): eff0 134 KILL_CREDIT2 TargetA 21
	// TARGET_UNIT_TARGET_ALLY misc 22316 chainAmplitude 1; eff1 6
	// APPLY_AURA aura 193 MELEE_SLOW dieSides 1 basePoints 14 TargetA 21
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 3
	// (60000ms, real row), rangeIdx 13 (Anywhere - Unlimited, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effKillCredit2 134 live
	// (spells.go:17749), auraMeleeSlow 193 recognized (aura_haste.go
	// type-193 rival scan). Removes the last unseeded-spell blocker for
	// npc_enslaved_netherwing_drake.lua (single-missing-id file).
	{id: 38775, castTimeIdx: 1, durIdx: 3, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effKillCredit2, targetA: targetUnitTargetAlly, misc: 22316, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMeleeSlow, dieSides: 1, basePoints: 14, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 37277 Summon Infernal (wago name; C++ SPELL_SUMMON_INFERNAL,
	// zone_shadowmoon_valley.cpp:61 enum; Lua npc_invis_infernal_caster.lua:33)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 60cdf9af, DifficultyID 0 rows): single eff0 3 DUMMY
	// TargetA 25 TARGET_UNIT_TARGET_ANY RadiusIndex 20 (25yd, real row)
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 0 (no
	// SpellDuration row), rangeIdx 34 (Twenty-Five yards, real row),
	// school 0x1 Physical, dispel 0 (no SpellCategories row). NOT carried:
	// no SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twins: effDummy 3 / target 25 live.
	// Removes the last unseeded-spell blocker for
	// npc_invis_infernal_caster.lua (single-missing-id file).
	{id: 37277, castTimeIdx: 1, durIdx: 0, rangeIdx: 34, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetUnitTargetAny, radiusIdx: 20, chainAmplitude: 1},
	}},

	// 37408 Oscillation Field (wago name; C++ SPELL_OSCILLATION_FIELD,
	// zone_blades_edge_mountains.cpp:927 enum; Lua
	// npc_oscillating_frequency_scanner_master_bunny.lua:35
	// RegisterSpellEvent ev5, dormant until Spell.dbc ships 37408) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 4374951a, DifficultyID 0 rows): single eff0 6
	// APPLY_AURA aura 4 DUMMY TargetA 22 TARGET_SRC_CASTER TargetB 7
	// TARGET_UNIT_SRC_AREA_ENTRY RadiusIndex 23 (50yd, real row)
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 27
	// (3000ms, real row), rangeIdx 5 (Long Range, real row), school 0x1
	// Physical, dispel 0 (no SpellCategories row). NOT carried: no
	// SpellPower row; SpellAuraOptions ProcChance 101 (no proc table,
	// 58109 precedent). Engine twin: auraDummy 4 live. Removes the last
	// unseeded-spell blocker for
	// npc_oscillating_frequency_scanner_master_bunny.lua
	// (single-missing-id file).
	{id: 37408, castTimeIdx: 1, durIdx: 27, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 23, chainAmplitude: 1},
	}},
	// 36462 Shadowmoon Tuber (wago name; C++ SPELL_SHADOWMOON_TUBER,
	// zone_shadowmoon_valley.cpp:1535 enum; Lua
	// npc_shadowmoon_tuber_node.lua:22) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read 4374951a,
	// DifficultyID 0 rows): single eff0 76 SUMMON_OBJECT_WILD TargetA 18
	// TARGET_DEST_CASTER misc 184691 chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 3 (60000ms, real row), rangeIdx 1 (Self
	// Only, real row), school 0x1 Physical, dispel 0 (no SpellCategories
	// row). NOT carried: no SpellPower row; SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent). Engine twin:
	// effSummonObjectWild 76 live (spells.go:19388). Removes the last
	// unseeded-spell blocker for npc_shadowmoon_tuber_node.lua
	// (single-missing-id file).
	{id: 36462, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummonObjectWild, targetA: targetDestCaster, misc: 184691, chainAmplitude: 1},
	}},
	// 33494 Mark of Malice Triggered (wago name; C++
	// SPELL_MARK_OF_MALICE_TRIGGERED, shadow_labyrinth.cpp:26 enum; Lua
	// shadow_labyrinth.lua:17, DORMANT: no Spell.dbc record) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15 browser read
	// 4374951a, DifficultyID 0 rows): single eff0 2 SCHOOL_DAMAGE
	// dieSides 701 basePoints 3149 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 0 (no
	// SpellDuration row), rangeIdx 1 (Self Only, real row), school 0x1
	// Physical, dispel 0 / mechanic 0 (SpellCategories row exists but
	// Dispel 0, Mechanic 0). NOT carried: no SpellPower row;
	// SpellAuraOptions ProcChance 101 (no proc table, 58109 precedent).
	// Engine twin: effSchoolDamage 2 live. Removes the last
	// unseeded-spell blocker for shadow_labyrinth.lua (single-missing-id
	// file).
	{id: 33494, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 701, basePoints: 3149, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 37284 Scalding Water (wago name; C++ SPELL_SCALDINGWATER,
	// instance_serpent_shrine.cpp:40 enum; Lua
	// instance_serpent_shrine.lua:150) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-15 browser read 4374951a,
	// DifficultyID 0 rows): eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE
	// period 3000 dieSides 1 basePoints 499 TargetA 1 TARGET_UNIT_CASTER
	// chainAmplitude 1; eff1 2 SCHOOL_DAMAGE dieSides 1 basePoints 499
	// TargetA 1 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 21 (-1 infinite, real row), rangeIdx 1 (Self Only, real
	// row), school 0x4 Fire, dispel 0 (no SpellCategories row). NOT
	// carried: no SpellPower row; NO SpellAuraOptions row, no ProcChance.
	// Diff176 rows are a separate difficulty, excluded (not a hotfix
	// diff). Engine twins: auraPeriodicDmg 3 / effSchoolDamage 2 live.
	// Removes the last unseeded-spell blocker for
	// instance_serpent_shrine.lua (single-missing-id file).
	{id: 37284, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 1, basePoints: 499, targetA: targetCaster, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 1, basePoints: 499, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 39291 Remove Kargath's Executioner (wago name; C++
	// SPELL_REMOVE_KARGATH_EXECUTIONER, shattered_halls.h:85 enum; Lua
	// instance_shattered_halls.lua:109) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-15 browser read 10fc9e9f,
	// DifficultyID 0 rows): single eff0 77 SCRIPT_EFFECT TargetA 1
	// TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 0 (no SpellDuration row), rangeIdx 1 (Self Only, real
	// row), school 0x1 Physical, dispel 0 (no SpellCategories row). NOT
	// carried: SpellPower not read in this verdict (cost 0); no
	// SpellClassOptions row; SpellAuraOptions ProcChance 101 (no proc
	// table, 58109 precedent); Attributes all 0x0 not encoded (seed has
	// no attributes field). Engine twin: effScriptEffect 77 live.
	// Removes the last unseeded-spell blocker for
	// instance_shattered_halls.lua (single-missing-id file).
	{id: 39291, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effScriptEffect, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 30478 Hemorrhage (wago name; C++ SPELL_HEMORRHAGE,
	// boss_nethekurse.cpp:55 enum; Lua npc_fel_orc_convert.lua:54, 3s
	// init then 15s non-triggered pump) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-15 browser read 10fc9e9f,
	// DifficultyID 0 rows): eff0 58 WEAPON_DAMAGE dieSides 1
	// basePoints 99 TargetA 6 TARGET_UNIT_TARGET_ENEMY chainAmplitude
	// 1; eff1 6 APPLY_AURA aura 14 MOD_DAMAGE_TAKEN dieSides 1
	// basePoints 14 misc 1 TargetA 6 chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 1 (10000ms, real row), rangeIdx 2
	// (Combat Range, real row), school 0x1 Physical, dispel 0 /
	// mechanic 0 (SpellCategories row exists: Category 1152,
	// DefenseType 2 Melee, PreventionType 0x2 — no struct fields for
	// those). NOT carried: SpellPower not read in this verdict (cost
	// 0); no SpellClassOptions row; SpellAuraOptions ProcChance 101
	// (no proc table, 58109 precedent); Attributes_0 0x80410 not
	// encoded (seed has no attributes field). Engine twins:
	// effWeaponDamage 58 / auraModDamageTaken 14 live. Removes the
	// last unseeded-spell blocker for npc_fel_orc_convert.lua
	// (single-missing-id file).
	{id: 30478, castTimeIdx: 1, durIdx: 1, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 99, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 14, targetA: targetEnemy, misc: 1, chainAmplitude: 1},
	}},
	// 58818 Orphan Out (wago name; C++ SPELL_ORPHAN_OUT,
	// childrens_week.cpp:131 enum; Lua at_bring_your_orphan_to.lua:32)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 10fc9e9f, DifficultyID 0 rows): single eff0 143
	// APPLY_AREA_AURA_OWNER aura 4 DUMMY TargetA 1 TARGET_UNIT_CASTER
	// RadiusIndex 12 (100yd, real row) chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row), rangeIdx
	// 1 (Self Only, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: SpellPower not read in this
	// verdict (cost 0); no SpellClassOptions row; SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent); Attributes_0
	// 0x100 / Attributes_1 0x10000800 / Attributes_6 0x400 not encoded
	// (seed has no attributes field). Engine twin:
	// effApplyAreaAuraOwner 143 live (spells.go:38742). Removes the
	// last unseeded-spell blocker for at_bring_your_orphan_to.lua and
	// npc_cw_area_trigger.lua (single-missing-id files).
	{id: 58818, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAreaAuraOwner, aura: auraDummy, targetA: targetCaster, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 38067 Guard's Mark (wago name; C++ SPELL_GUARDS_MARK,
	// npcs_special.cpp:49 enum; Lua npc_air_force_bots.lua:21) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-15
	// browser read 10fc9e9f, DifficultyID 0 rows): eff0 6 APPLY_AURA
	// aura 68 MOD_STALKED dieSides 1 basePoints -1 TargetA 25
	// TARGET_UNIT_TARGET_ANY chainAmplitude 1; eff1 6 APPLY_AURA aura
	// 127 RANGED_ATTACK_POWER_ATTACKER_BONUS dieSides 1 basePoints 109
	// TargetA 6 chainAmplitude 1; eff2 6 APPLY_AURA aura 165
	// MELEE_ATTACK_POWER_ATTACKER_BONUS dieSides 0 basePoints 0
	// TargetA 6 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 9 (30000ms, real row), rangeIdx 6 (Vision Range, real
	// row), school 0x40 Arcane, family Hunter (SpellClassSet 9, mask
	// 1024), dispel 0 / mechanic 0 (SpellCategories row exists:
	// StartRecoveryCategory 133 — no struct field). NOT carried:
	// SpellPower not read in this verdict (cost 0); SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent); EffectAmplitude
	// 1 on eff0 (float32, no seed-struct field — store.go:1184/1188
	// field 101+i); auraPeriod 0 on all three effects; Attributes_0
	// 0x4010000 / Attributes_1 0x400 / Attributes_3 0x30001 not encoded
	// (seed has no attributes field). Engine twins: aura 68/127/165
	// named in spells.go; effApplyAura 6 live. Removes the last
	// unseeded-spell blocker for npc_air_force_bots.lua
	// (single-missing-id file).
	{id: 38067, castTimeIdx: 1, durIdx: 9, rangeIdx: 6, school: schoolArcane, family: familyHunter, familyFlags: [3]uint32{1024, 0, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraModStalked, auraPeriod: 0, dieSides: 1, basePoints: -1, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRangedApAttackerBonus, dieSides: 1, basePoints: 109, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMeleeApAttackerBonus, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 26522 Lunar Fortune (wago name; C++ SPELL_LUNAR_FORTUNE,
	// npcs_special.cpp:1761 enum; Lua world/npc_firework.lua:129,
	// non-triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row
	// + SpellMisc (2026-11-15 browser read 5857f249, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 34 MOD_INCREASE_HEALTH
	// dieSides 1 basePoints 249 TargetA 22 TARGET_SRC_CASTER
	// RadiusIndex 9 (20yd, real row) chainAmplitude 1; misc 0
	// TargetB 8 TARGET_UNIT_DEST_AREA_ENTRY chainAmplitude 1;
	// (9ba490eb re-read 2026-11-15 corrects the 5857f249 read: the 9 is
	// EffectRadiusIndex_1, not EffectMiscValue_0 — 5857f249 also
	// misread BonusCoefficientFromAP/PvpMultiplier on 50052/69735;
	// area pattern matches the 37408 radius precedent.)
	// castTimeIdx 1 (instant, real row), durIdx 30 (1800000ms, real
	// row), rangeIdx 3 (Twenty yards, real row), school 0x1 Physical,
	// dispel 0 (no SpellCategories row). NOT carried: NO
	// SpellAuraOptions row, no SpellPower row; EffectAmplitude 0 /
	// auraPeriod 0 (zero-value); Attributes not encoded (seed has no
	// attributes field). Engine twin: auraModIncreaseHealth 34 live
	// (aura_stats.go / creature_auras.go). Removes the last
	// unseeded-spell blocker for world/npc_firework.lua
	// (single-missing-id file).
	{id: 26522, castTimeIdx: 1, durIdx: 30, rangeIdx: 3, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModIncreaseHealth, dieSides: 1, basePoints: 249, targetA: targetSrcCaster, targetB: targetUnitDestAreaEntry, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 50514 Summon Gargoyle (wago name; C++ SPELL_DK_SUMMON_GARGOYLE_2,
	// pet_dk.cpp:34 enum; Lua world/npc_pet_dk_ebon_gargoyle.lua:62,
	// owner:RemoveAura) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 5857f249, DifficultyID 0
	// rows): single eff0 6 APPLY_AURA aura 23 PERIODIC_TRIGGER_SPELL
	// auraPeriod 1000 dieSides 1 basePoints 50514 (verbatim as shown;
	// cross-checked in foreign-key view and table) trigger 50524
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 18 (20000ms, real row), rangeIdx 1
	// (Self Only, real row), school 0x20 Shadow, family 15
	// SPELLFAMILY_DEATHKNIGHT mask {0,128,0,0} (SpellClassOptions row
	// read 9ba490eb 2026-11-15; the 5857f249 read did not cover that
	// table), dispel 0 (no
	// SpellCategories row). NOT carried: SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent); SpellPower ManaPerSecond
	// 30 / PowerType 6 (no seed-struct field; manaCost 0); EffectAmplitude
	// 0 (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twin: auraPeriodicTrig 23 live (spells.go:38224).
	// Removes the last unseeded-spell blocker for
	// world/npc_pet_dk_ebon_gargoyle.lua (single-missing-id file).
	{id: 50514, castTimeIdx: 1, durIdx: 18, rangeIdx: 1, school: schoolShadow, family: familyDeathKnight, familyFlags: [3]uint32{0, 128, 0}, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, dieSides: 1, basePoints: 50514, trigger: 50524, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50052 Ethereal Pet onSummon (wago name; C++
	// SPELL_ETHEREAL_ONSUMMON, pet_generic.cpp:129 enum, spell script
	// spell_generic.cpp:1693; Lua world/npc_pet_gen_soul_trader.lua:44,
	// non-triggered owner cast) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-15 browser read 5857f249, DifficultyID
	// 0 rows): single eff0 77 SCRIPT_EFFECT TargetA 25
	// TARGET_UNIT_TARGET_ANY chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 0 (no SpellDuration row), rangeIdx 6 (Vision
	// Range, real row), school 0x1 Physical, dispel 0 (no
	// SpellCategories row). NOT carried: SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent); no SpellPower row;
	// EffectAmplitude 0 / auraPeriod 0 (zero-value); Attributes not
	// encoded (seed has no attributes field). Engine twin:
	// effScriptEffect 77 live. Removes the last unseeded-spell blocker
	// for world/npc_pet_gen_soul_trader.lua (single-missing-id file).
	{id: 50052, castTimeIdx: 1, durIdx: 0, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effScriptEffect, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 69735 Lich Pet OnSummon (wago name; C++ SPELL_LICH_ONSUMMON,
	// pet_generic.cpp:155 enum, spell script spell_generic.cpp:1921;
	// Lua world/npc_pet_lich.lua:42, non-triggered owner cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 5857f249, DifficultyID 0 rows): single eff0 77
	// SCRIPT_EFFECT TargetA 27 TARGET_UNIT_MASTER chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration
	// row), rangeIdx 6 (Vision Range, real row), school 0x1 Physical,
	// dispel 0 (no SpellCategories row). NOT carried: SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent); no SpellPower
	// row; EffectAmplitude 0 / auraPeriod 0 (zero-value); Attributes
	// not encoded (seed has no attributes field). Engine twin:
	// effScriptEffect 77 live. Removes the last unseeded-spell blocker
	// for world/npc_pet_lich.lua (single-missing-id file).
	{id: 69735, castTimeIdx: 1, durIdx: 0, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effScriptEffect, targetA: targetUnitMaster, chainAmplitude: 1},
	}},
	// 36213 Angered Earth (wago name; C++ SPELL_SHAMAN_ANGEREDEARTH,
	// pet_shaman.cpp:28 enum; Lua world/npc_pet_shaman_earth_elemental.lua:36/76,
	// non-triggered victim cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-15 browser read 2f3bd931, DifficultyID 0
	// rows): eff0 114 ATTACK_ME dieSides 1 TargetA 22 TARGET_SRC_CASTER
	// TargetB 15 TARGET_UNIT_SRC_AREA_ENEMY RadiusIndex 18 (15yd, real
	// row) chainAmplitude 1; eff1 63 THREAT dieSides 1 basePoints 1099
	// TargetA 22 TargetB 15 RadiusIndex 18 chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration row),
	// rangeIdx 1 (Self Only, real row), school 0x1 Physical, family 5
	// SPELLFAMILY_WARLOCK mask {0,0,0} (SpellClassOptions row; zero
	// mask, no familyFlags carried), mechanic 16 (SpellCategories
	// Mechanic 16), dispel 0 (DispelType 0). NOT carried: no
	// SpellAuraOptions row; no SpellPower row; eff1 RealPointsPerLevel
	// 25 (no seed-struct field); SpellCategories DefenseType 2 /
	// PreventionType 0x2 / Category 0 (no struct fields);
	// EffectAmplitude 0 (zero-value); Attributes not encoded (seed has
	// no attributes field). Engine twins: spellEffectAttackMe 114 live
	// (immunity.go), spellEffectThreat 63 live (spells.go:382). Removes
	// the last unseeded-spell blocker for
	// world/npc_pet_shaman_earth_elemental.lua (single-missing-id file;
	// last seedable single — 59046/65296 unseedable).
	{id: 36213, mechanic: 16, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, family: familyWarlock, effects: [3]effect{
		{effect: effAttackMe, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effThreat, dieSides: 1, basePoints: 1099, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 35759 Felfire Shock (wago name; C++ SPELL_FELFIRE_SHOCK,
	// boss_wrath_scryer_soccothrates.cpp:55 enum; Lua
	// arcatraz/boss_wrath_scryer_soccothrates.lua:134, timer-driven
	// victim cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 2f3bd931, DifficultyID 0 rows): eff0 2
	// SCHOOL_DAMAGE dieSides 161 basePoints 1519 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; eff1 6 APPLY_AURA aura
	// 3 PERIODIC_DAMAGE auraPeriod 3000 dieSides 1 basePoints 982
	// TargetA 6 chainAmplitude 1; castTimeIdx 19 (2500ms, real row),
	// durIdx 29 (12000ms, real SpellDuration row 29, added below),
	// rangeIdx 3 (Twenty yards, real row), school 0x4 Fire, dispel 1
	// (SpellCategories DispelType 1 MAGIC), mechanic 0. NOT carried:
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent); eff0 BonusCoefficient 1 / eff1 BonusCoefficient 0.1
	// (no seed-struct field); no SpellClassOptions row (no family);
	// SpellCategories Category 19 / DefenseType 1 / StartRecoveryCategory
	// 133 (no struct fields); EffectAmplitude 0 (zero-value);
	// Attributes not encoded (seed has no attributes field). Engine
	// twin: auraPeriodicDmg 3 live (spells.go), effSchoolDamage 2 live.
	{id: 35759, dispel: 1, castTimeIdx: 19, durIdx: 29, rangeIdx: 3, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 161, basePoints: 1519, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 1, basePoints: 982, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 36512 Knock Away (wago name; C++ SPELL_KNOCK_AWAY,
	// boss_wrath_scryer_soccothrates.cpp:56 enum; Lua
	// arcatraz/boss_wrath_scryer_soccothrates.lua:135, timer-driven
	// area cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-15 browser read 2f3bd931, DifficultyID 0 rows): eff0 31
	// WEAPON_PERCENT_DAMAGE dieSides 1 basePoints 49 TargetA 22
	// TargetB 15 RadiusIndex 13 (10yd, real row) chainAmplitude 1;
	// eff1 98 KNOCK_BACK dieSides 27 basePoints 116 misc 265 TargetA
	// 22 TargetB 15 RadiusIndex 13 chainAmplitude 1; eff2 6 APPLY_AURA
	// aura 26 MOD_ROOT TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 555 (4500ms, real
	// SpellDuration row 555, added below), rangeIdx 7 (Ten yards, real
	// row), school 0x1 Physical, dispel 0 / mechanic 0 (no
	// SpellCategories row). NOT carried: SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent); no SpellClassOptions row
	// (no family); EffectAmplitude 0 (zero-value); Attributes not
	// encoded (seed has no attributes field). Engine twins:
	// spellEffectKnockBack 98 live with handler (spells.go:18835),
	// spellEffectWeaponPercentDamage 31 live, auraRoot 26 live. With
	// 35759, removes arcatraz/boss_wrath_scryer_soccothrates.lua from
	// the 2-missing-id list.
	{id: 36512, castTimeIdx: 1, durIdx: 555, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 49, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 27, basePoints: 116, misc: 265, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 41253 Greater Invisibility (wago name; C++
	// SPELL_GREATER_INVISIBILITY, black_temple.cpp:33 enum; Lua
	// blacktemple/npc_angered_soul_fragment.lua:33, self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-15
	// browser read 2f3bd931, DifficultyID 0 rows): single eff0 6
	// APPLY_AURA aura 18 MOD_INVISIBILITY dieSides 1 basePoints 299
	// TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 21 (-1 infinite, real row), rangeIdx
	// 1 (Self Only, real row), school 0x40 Arcane, family 3
	// SPELLFAMILY_MAGE mask {0,0,0} (SpellClassOptions row; zero mask,
	// no familyFlags carried), dispel 6 (SpellCategories DispelType 6
	// INVISIBILITY), mechanic 0. NOT carried: SpellAuraOptions
	// ProcChance 101 (no proc table, 58109 precedent); eff0
	// BonusCoefficient 1 (no seed-struct field); SpellCategories
	// DefenseType 1 / PreventionType 0x1 (no struct fields);
	// EffectAmplitude 0 (zero-value); Attributes not encoded (seed has
	// no attributes field). Engine twin: spellAuraModInvisibility 18
	// live (stealth.go). Removes the last unseeded-spell blocker for
	// blacktemple/npc_angered_soul_fragment.lua (single-missing-id file).
	{id: 41253, dispel: 6, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, family: familyMage, effects: [3]effect{
		{effect: effApplyAura, aura: auraModInvisibility, dieSides: 1, basePoints: 299, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 34794 Arcane Resonance (wago name; C++ SPELL_ARCANE_RESONANCE,
	// boss_commander_sarannis.cpp:37 enum; Lua
	// botanica/boss_commander_sarannis.lua:158, TRIGGERED victim cast)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read bb66d46a, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 14 MOD_DAMAGE_TAKEN dieSides 1 basePoints
	// 1199 misc 64 (Arcane school mask) TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 9 (30000ms, real row), rangeIdx 7
	// (Ten yards, real row), school 0x40 Arcane, dispel 1
	// (SpellCategories DispelType 1 MAGIC), mechanic 0. NOT carried:
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent); CumulativeAura 3 (no seed-struct field); no
	// SpellClassOptions row (no family); SpellCategories DefenseType 1
	// / PreventionType 0x1 (no struct fields); BonusCoefficientFromAP /
	// PvpMultiplier as read (no seed-struct fields); EffectAmplitude 0
	// (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twin: auraModDamageTaken 14 live (spells.go).
	{id: 34794, dispel: 1, castTimeIdx: 1, durIdx: 9, rangeIdx: 7, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 1199, misc: 64, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34799 Arcane Devastation (wago name; C++ SPELL_ARCANE_DEVASTATION,
	// boss_commander_sarannis.cpp:38 enum; Lua
	// botanica/boss_commander_sarannis.lua:168, TRIGGERED victim cast)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read bb66d46a, DifficultyID 0 rows): eff0 31
	// WEAPON_PERCENT_DAMAGE dieSides 1 basePoints 49 TargetA 6
	// chainAmplitude 1; NO EffectIndex 1 row — slot 1 stays empty (the
	// {} middle entry reproduces the real DBC gap); eff2 6 APPLY_AURA
	// aura 4 DUMMY TargetA 6 chainAmplitude 1; castTimeIdx 1 (instant,
	// real row), durIdx 327 (500ms, real SpellDuration row 327, added
	// below), rangeIdx 2 (Combat Range, real row), school 0x40 Arcane,
	// dispel 0 / mechanic 0 (DispelType 0). NOT carried:
	// SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent); eff0 BonusCoefficient 1 (no seed-struct field);
	// BonusCoefficientFromAP / PvpMultiplier / ResourceCoefficient /
	// GroupSizeBasePointsCoefficient as read (no seed-struct fields);
	// no SpellClassOptions row (no family); SpellCategories DefenseType
	// 2 / PreventionType 0x2 (no struct fields); EffectAmplitude 0
	// (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twins: spellEffectWeaponPercentDamage 31 live,
	// auraDummy 4 live (spells.go:666). With 34794, removes
	// botanica/boss_commander_sarannis.lua from the 2-missing-id list.
	{id: 34799, castTimeIdx: 1, durIdx: 327, rangeIdx: 2, school: schoolArcane, effects: [3]effect{
		{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 49, targetA: targetEnemy, chainAmplitude: 1},
		{},
		{effect: effApplyAura, aura: auraDummy, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34697 Allergic Reaction (wago name; C++ SPELL_ALLERGIC_REACTION,
	// boss_laj.cpp:31 enum; Lua botanica/boss_laj.lua:170,
	// non-triggered victim cast) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read bb66d46a, DifficultyID
	// 0 rows): eff0 6 APPLY_AURA aura 14 MOD_DAMAGE_TAKEN dieSides 1
	// basePoints 499 misc 127 (all-schools mask) TargetA 6
	// chainAmplitude 1; eff1 6 APPLY_AURA aura 3 PERIODIC_DAMAGE
	// auraPeriod 3000 dieSides 95 basePoints 902 TargetA 6
	// chainAmplitude 1; eff2 6 APPLY_AURA aura 42 PROC_TRIGGER_SPELL
	// dieSides 1 trigger 34700 TargetA 6 chainAmplitude 1;
	// castTimeIdx 16 (1500ms, real row), durIdx 85 (18000ms, real row),
	// rangeIdx 137 (Eight yards, real SpellRange row 137, added below),
	// school 0x8 Nature, dispel 3 (SpellCategories DispelType 3
	// DISEASE), mechanic 0. NOT carried: SpellAuraOptions ProcChance
	// 101 (no proc table, 58109 precedent); ProcTypeMask_0 0x80080000
	// (no proc table); no SpellClassOptions row (no family);
	// SpellCategories Category 36 / DefenseType 1 / PreventionType 0x1
	// (no struct fields); BonusCoefficientFromAP / PvpMultiplier as
	// read (no seed-struct fields); EffectAmplitude 0 (zero-value);
	// Attributes not encoded (seed has no attributes field). Engine
	// twins: auraModDamageTaken 14 / auraPeriodicDmg 3 /
	// auraProcTriggerSpell 42 live (spells.go). Trigger 34700 is the
	// proc target, not seeded (50514 trigger-50524 precedent).
	{id: 34697, dispel: 3, castTimeIdx: 16, durIdx: 85, rangeIdx: 137, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 499, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 95, basePoints: 902, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, trigger: 34700, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34673 Teleport Self (wago name; C++ SPELL_TELEPORT_SELF,
	// boss_laj.cpp:32 enum; Lua botanica/boss_laj.lua:180,
	// non-triggered self-cast) — wago.tools 3.4.0.47659 SpellEffect row
	// + SpellMisc (2026-11-16 browser read bb66d46a, DifficultyID 0
	// rows): single eff0 5 TELEPORT_UNITS dieSides 1 TargetA 1
	// TARGET_UNIT_CASTER TargetB 17 TARGET_DEST_DB chainAmplitude 1;
	// castTimeIdx 1 (instant, real row), durIdx 0 (no SpellDuration
	// row), rangeIdx 13 (Anywhere - Unlimited, real row), school 0x1
	// Physical, dispel 0 / mechanic 0 (no SpellCategories row). NOT
	// carried: SpellAuraOptions ProcChance 101 (no proc table, 58109
	// precedent); EffectPos_facing 0.017453 (no seed-struct field); no
	// SpellClassOptions row (no family); EffectAmplitude 0
	// (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twin: spellEffectTeleportUnits 5 live with
	// handler (spells.go:17364). With 34697, removes
	// botanica/boss_laj.lua from the 2-missing-id list.
	{id: 34673, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 12040 Shadow Shield (wago name; C++ SPELL_SHADOWSHIELD,
	// boss_darkmaster_gandling.cpp:44 enum; Lua
	// eastern_kingdoms/boss_darkmaster_gandling.lua:182, self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 53d71537, DifficultyID 0 rows): eff0 6 APPLY_AURA
	// aura 69 SCHOOL_ABSORB dieSides 1 basePoints 199 misc 127 (0x7f
	// all-schools) TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1;
	// eff1 6 APPLY_AURA aura 15 DAMAGE_SHIELD dieSides 1 basePoints 9
	// TargetA 1 chainAmplitude 1; castTimeIdx 1 (instant, real row),
	// durIdx 9 (30000ms, real row), rangeIdx 5 (Long Range, real row),
	// school 0x20 Shadow, dispel 1 (SpellCategories DispelType 1
	// MAGIC), mechanic 0, manaCost 10 (SpellPower ManaCost 10, 12544
	// precedent). NOT carried: no SpellAuraOptions row; no
	// SpellClassOptions row (no family); eff0 RealPointsPerLevel 10 /
	// eff1 RealPointsPerLevel 0.5 (no seed-struct field); SpellPower
	// ManaCostPerLevel 8 (no seed-struct field); SpellCategories
	// DefenseType 1 / PreventionType 0x1 (no struct fields);
	// EffectAmplitude 0 (zero-value); Attributes not encoded (seed has
	// no attributes field). Engine twins: SpellAuraSchoolAbsorb 69
	// live (absorption.go:15), spellAuraDamageShield 15 live
	// (damage_shield.go).
	{id: 12040, dispel: 1, manaCost: 10, castTimeIdx: 1, durIdx: 9, rangeIdx: 5, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraSchoolAbsorb, dieSides: 1, basePoints: 199, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDamageShield, dieSides: 1, basePoints: 9, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 17950 Shadow Portal (wago name; C++ SPELL_SHADOW_PORTAL,
	// boss_darkmaster_gandling.cpp:46 enum; Lua
	// eastern_kingdoms/boss_darkmaster_gandling.lua:216, TRIGGERED
	// target cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 53d71537, DifficultyID 0 rows): single
	// eff0 3 DUMMY dieSides 1 TargetA 6 TARGET_UNIT_TARGET_ENEMY
	// chainAmplitude 1; castTimeIdx 4 (1000ms, real row), durIdx 0 (no
	// SpellDuration row), rangeIdx 4 (Medium Range, real row), school
	// 0x1 Physical, dispel 0 / mechanic 0 (no SpellCategories row).
	// NOT carried: no SpellAuraOptions row; no SpellPower row; no
	// SpellClassOptions row (no family); eff0 BonusCoefficient 1 (no
	// seed-struct field); EffectAmplitude 0 (zero-value); Attributes
	// not encoded (seed has no attributes field). Engine twin:
	// spellEffectDummy 3 live (spells.go:375). The 6-room picker is a
	// C++ SpellScript (boss_darkmaster_gandling.cpp:144); the DUMMY
	// effect is the seedable leg.
	{id: 17950, castTimeIdx: 4, durIdx: 0, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16509 Rend (wago name; C++ SPELL_REND,
	// boss_doctor_theolen_krastinov.cpp:36 enum; Lua
	// eastern_kingdoms/boss_doctor_theolen_krastinov.lua, TRIGGERED
	// victim cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 53d71537, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 3 PERIODIC_DAMAGE auraPeriod 3000
	// dieSides 1 basePoints 26 mechanic 15 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 8 (15000ms, real row), rangeIdx 2
	// (Combat Range, real row), school 0x1 Physical, mechanic 15
	// (SpellCategories Mechanic 15), dispel 0. NOT carried: no
	// SpellAuraOptions row; no SpellClassOptions row (no family);
	// SpellPower ManaCost 0 (zero-value); SpellCategories DefenseType
	// 2 / PreventionType 0x2 (no struct fields); EffectAmplitude 0
	// (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twin: auraPeriodicDmg 3 live (spells.go).
	// Per-effect mechanic carried on the 982/1356 auraDecSpeed
	// precedent.
	{id: 16509, mechanic: 15, castTimeIdx: 1, durIdx: 8, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 1, basePoints: 26, mechanic: 15, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 18103 Backhand (wago name; C++ SPELL_BACKHAND,
	// boss_doctor_theolen_krastinov.cpp:37 enum; Lua
	// eastern_kingdoms/boss_doctor_theolen_krastinov.lua, TRIGGERED
	// victim cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 53d71537, DifficultyID 0 rows): single
	// eff0 6 APPLY_AURA aura 12 MOD_STUN TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 39 (2000ms, real row), rangeIdx 12
	// (Interact Range, real row), school 0x1 Physical, mechanic 12
	// (SpellCategories Mechanic 12), dispel 0. NOT carried: no
	// SpellAuraOptions row; no SpellPower row; no SpellClassOptions row
	// (no family); SpellCategories DefenseType 2 / PreventionType 0x2
	// (no struct fields); EffectAmplitude 0 (zero-value); Attributes
	// not encoded (seed has no attributes field). Engine twin:
	// spellAuraModStun 12 live (spells.go:718). With 16509, removes
	// eastern_kingdoms/boss_doctor_theolen_krastinov.lua from the
	// 2-missing-id list (gandling likewise for 12040/17950).
	{id: 18103, mechanic: 12, castTimeIdx: 1, durIdx: 39, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 15636 Avatar of Flame (wago name; C++ SPELL_AVATAROFFLAME,
	// boss_emperor_dagran_thaurissan.cpp:33 enum; Lua
	// eastern_kingdoms/boss_emperor_dagran_thaurissan.lua:114,
	// victim-cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read d7c7d05e, DifficultyID 0 rows): eff0 6
	// APPLY_AURA aura 39 SCHOOL_IMMUNITY misc 126 (0x7e, all schools
	// but Physical) TargetA 1 TARGET_UNIT_CASTER chainAmplitude 1;
	// eff1 6 APPLY_AURA aura 61 MOD_SCALE dieSides 1 basePoints 59
	// TargetA 1 chainAmplitude 1; eff2 6 APPLY_AURA aura 42
	// PROC_TRIGGER_SPELL dieSides 1 trigger 15643 TargetA 1
	// chainAmplitude 1; castTimeIdx 1 (instant, real row), durIdx 1
	// (10000ms, real row), rangeIdx 1 (Self Only, real row), school
	// 0x4 Fire, dispel 0 / mechanic 0 (no SpellCategories row). NOT
	// carried: SpellAuraOptions ProcChance 100 / ProcTypeMask_0 0x14
	// (no proc table; ProcChance here is 100, not the usual 101);
	// no SpellPower row; no SpellClassOptions row (no family);
	// BonusCoefficient 1 on all three effects (no seed-struct field);
	// eff1 BonusCoefficientFromAP 1 / PvpMultiplier 0 /
	// ResourceCoefficient 1 / GroupSize 0 as read (no seed-struct
	// fields); EffectAmplitude 0 (zero-value); Attributes not encoded
	// (seed has no attributes field). Engine twins:
	// spellAuraSchoolImmunity 39 live (player_auras.go),
	// spellAuraModScale 61 live (spells.go:733),
	// auraProcTriggerSpell 42 live (procs.go:1287). Trigger 15643 is
	// the proc target, not seeded (50514 trigger-50524 precedent).
	{id: 15636, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraSchoolImmunity, misc: 126, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 59, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, trigger: 15643, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 17492 Hand of Thaurissan (wago name; C++
	// SPELL_HANDOFTHAURISSAN, boss_emperor_dagran_thaurissan.cpp:32
	// enum; Lua eastern_kingdoms/boss_emperor_dagran_thaurissan.lua:106,
	// target cast) — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read d7c7d05e, DifficultyID 0 rows): eff0 2
	// SCHOOL_DAMAGE dieSides 1 basePoints 239 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; eff1 6 APPLY_AURA aura
	// 12 MOD_STUN mechanic 12 TargetA 6 chainAmplitude 1; castTimeIdx
	// 1 (instant, real row), durIdx 28 (5000ms, real row), rangeIdx 4
	// (Medium Range, real row), school 0x4 Fire, family 3
	// SPELLFAMILY_MAGE mask {131074,0,0} (SpellClassOptions row),
	// dispel 0 / mechanic 0 (spell-level; Categories Mechanic 0).
	// NOT carried: no SpellAuraOptions row; no SpellPower row;
	// SpellCategories Category 19 / DefenseType 1 / PreventionType
	// 0x1 (no struct fields); eff1 BonusCoefficient 1 (no seed-struct
	// field); EffectAmplitude 0 (zero-value); Attributes not encoded
	// (seed has no attributes field). Engine twins: effSchoolDamage 2
	// live, spellAuraModStun 12 live (spells.go:718). Per-effect
	// mechanic carried on the 982/1356 auraDecSpeed precedent.
	{id: 17492, castTimeIdx: 1, durIdx: 28, rangeIdx: 4, school: schoolFire, family: familyMage, familyFlags: [3]uint32{131074, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 239, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, mechanic: 12, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16495 Fatal Bite (wago name; C++ SPELL_FATAL_BITE,
	// boss_gizrul_the_slavener.cpp:26 enum; Lua
	// eastern_kingdoms/boss_gizrul_the_slavener.lua:93, victim-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read d7c7d05e, DifficultyID 0 rows): single eff0 9
	// HEALTH_LEECH dieSides 1 basePoints 99 TargetA 6
	// TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; castTimeIdx 1
	// (instant, real row), durIdx 0 (no SpellDuration row), rangeIdx 2
	// (Combat Range, real row), school 0x1 Physical, dispel 0 /
	// mechanic 0 (Categories Mechanic 0). NOT carried: no
	// SpellAuraOptions row; no SpellClassOptions row (no family);
	// SpellPower ManaCost 0 (zero-value); EffectAmplitude 2 /
	// RealPointsPerLevel 16 (no seed-struct fields);
	// BonusCoefficient 1 (no seed-struct field); SpellCategories
	// Category 41 / DefenseType 2 / PreventionType 0x2 (no struct
	// fields); Attributes not encoded (seed has no attributes field).
	// Engine twin: spellEffectHealthLeech 9 live with handler
	// (spells.go:18601).
	{id: 16495, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effHealthLeech, dieSides: 1, basePoints: 99, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16128 Infected Bite (wago name; C++ SPELL_INFECTED_BITE,
	// boss_gizrul_the_slavener.cpp:27 enum; Lua
	// eastern_kingdoms/boss_gizrul_the_slavener.lua:103, self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read d7c7d05e, DifficultyID 0 rows): eff0 6 APPLY_AURA
	// aura 3 PERIODIC_DAMAGE auraPeriod 10000 dieSides 1 basePoints 44
	// TargetA 6 TARGET_UNIT_TARGET_ENEMY chainAmplitude 1; eff1 6
	// APPLY_AURA aura 14 MOD_DAMAGE_TAKEN dieSides 1 basePoints 19
	// misc 1 TargetA 6 chainAmplitude 1; castTimeIdx 1 (instant, real
	// row), durIdx 25 (180000ms, real row), rangeIdx 2 (Combat Range,
	// real row), school 0x8 Nature, dispel 3 (SpellCategories
	// DispelType 3 DISEASE), mechanic 0. NOT carried: no
	// SpellAuraOptions row; no SpellPower row; no SpellClassOptions row
	// (no family); eff0 BonusCoefficient 0.1 / eff1 RealPointsPerLevel
	// 1 (no seed-struct fields); SpellCategories DefenseType 2 /
	// PreventionType 0x2 (no struct fields); EffectAmplitude 0
	// (zero-value); Attributes not encoded (seed has no attributes
	// field). Engine twins: auraPeriodicDmg 3 / auraModDamageTaken 14
	// live (spells.go). With 16495, removes
	// eastern_kingdoms/boss_gizrul_the_slavener.lua from the
	// 2-missing-id list (thaurissan likewise for 15636/17492).
	{id: 16128, dispel: 3, castTimeIdx: 1, durIdx: 25, rangeIdx: 2, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 10000, dieSides: 1, basePoints: 44, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 19, misc: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 3391 Thrash (wago name; C++ SPELL_THRASH,
	// boss_halycon.cpp:25 enum; Lua
	// eastern_kingdoms/boss_halycon.lua:104, self cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser read
	// 7b42192d, DifficultyID 0 rows): eff0 ADD_EXTRA_ATTACKS bp1 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur0 (no SpellDuration
	// row), range1 Self Only; dispel 0, mechanic 0; no SpellPower,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 3391, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effAddExtraAttacks, dieSides: 1, basePoints: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 13738 Rend (wago name; C++ SPELL_REND,
	// boss_halycon.cpp:24 enum; Lua
	// eastern_kingdoms/boss_halycon.lua:94, victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser read
	// 7b42192d, DifficultyID 0 rows): eff0 APPLY_AURA PERIODIC_DAMAGE
	// period3000 bp25 die1 mech15 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur8 15000ms, range2 Combat; spell mechanic 15, dispel
	// 0; SpellPower ManaCost 0.
	{id: 13738, mechanic: 15, castTimeIdx: 1, durIdx: 8, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, auraPeriod: 3000, dieSides: 1, basePoints: 25, mechanic: 15, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 6742 Bloodlust (wago name; C++ SPELL_BLOODLUST,
	// boss_houndmaster_loksey.cpp:30 enum; Lua
	// eastern_kingdoms/boss_houndmaster_loksey.lua:81, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 7b42192d, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_MELEE_HASTE bp29 die1 tgtA TARGET_UNIT_TARGET_ALLY; cast1
	// instant, dur9 30000ms, range4 Medium 30yd; dispel 1 (MAGIC),
	// SpellPower ManaCost 10; school Nature.
	{id: 6742, dispel: 1, manaCost: 10, castTimeIdx: 1, durIdx: 9, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 29, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 17164 Summon Scarlet Hound (wago name; C++
	// SPELL_SUMMON_SCARLET_HOUND, boss_houndmaster_loksey.cpp:29 enum;
	// Lua eastern_kingdoms/boss_houndmaster_loksey.lua:98, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 7b42192d, DifficultyID 0 rows): eff0 SUMMON_PET
	// misc10979 (Scarlet Hound entry) tgtA TARGET_DEST_CASTER_SUMMON;
	// cast1 instant, dur21 -1 infinite, range1 Self Only; school
	// Shadow; dispel 0, mechanic 0.
	{id: 17164, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSummonPet, dieSides: 1, targetA: targetDestCasterSummon, misc: 10979, chainAmplitude: 1},
	}},
	// 13900 Fiery Burst (wago name; C++ SPELL_FIERYBURST,
	// boss_magmus.cpp:25 enum; Lua
	// eastern_kingdoms/boss_magmus.lua:45, victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser read
	// 92ae141c, DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp509 die181
	// tgtA TARGET_UNIT_DEST_AREA_ENEMY radius13 (10yd); cast16 1500ms,
	// dur0 (no SpellDuration row), range5 Long 40yd; dispel 0,
	// mechanic 0; no SpellPower, SpellClassOptions or SpellAuraOptions
	// rows.
	{id: 13900, castTimeIdx: 16, durIdx: 0, rangeIdx: 5, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 181, basePoints: 509, targetA: targetUnitDestAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 24375 War Stomp (wago name; C++ SPELL_WARSTOMP,
	// boss_magmus.cpp:26 enum; Lua
	// eastern_kingdoms/boss_magmus.lua:46, victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 92ae141c, DifficultyID 0 rows): eff0 APPLY_AURA MOD_STUN bp-1
	// die1 tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius18 (15yd); NO EffectIndex 1 row (empty middle slot);
	// eff2 WEAPON_DAMAGE bp499 die1 same targets/radius; cast1
	// instant, dur28 5000ms, range1 Self Only; spell mechanic 12,
	// dispel 0; EffectAmplitude 1 on eff0 not carried (no struct field).
	{id: 24375, mechanic: 12, castTimeIdx: 1, durIdx: 28, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
		{},
		{effect: effWeaponDamage, dieSides: 1, basePoints: 499, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 16496 Shoot (wago name; C++ SPELL_SHOOT,
	// boss_quartermaster_zigris.cpp:24 enum; Lua
	// eastern_kingdoms/boss_quartermaster_zigris.lua:47, victim cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 92ae141c, DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp214 die75 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// (no SpellDuration row), range54 Monster Shoot 5-30yd; family
	// Hunter 9, familyFlags {1,0,0} (SpellClassOptions SpellClassSet 9,
	// mask 1); dispel 0, mechanic 0.
	{id: 16496, castTimeIdx: 1, durIdx: 0, rangeIdx: 54, school: schoolPhysical, family: familyHunter, familyFlags: [3]uint32{1, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 75, basePoints: 214, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16497 Stun Bomb (wago name; C++ SPELL_STUNBOMB,
	// boss_quartermaster_zigris.cpp:25 enum; Lua
	// eastern_kingdoms/boss_quartermaster_zigris.lua:48, victim cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 92ae141c, DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp0 die1 tgtA TARGET_UNIT_DEST_AREA_ENEMY radius8 (5yd);
	// eff1 APPLY_AURA MOD_STUN bp0 die0 same target/radius; cast5
	// 2000ms, dur28 5000ms, range4 Medium 30yd; spell mechanic 12,
	// dispel 0; eff0 RealPointsPerLevel 1.5 not carried (no field).
	{id: 16497, mechanic: 12, castTimeIdx: 5, durIdx: 28, rangeIdx: 4, school: schoolFire, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, targetA: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 6432 Smite Stomp (wago name; C++ SPELL_SMITE_STOMP,
	// boss_mr_smite.cpp:35 enum; Lua
	// eastern_kingdoms/boss_mr_smite.lua:70, self cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser read
	// 8bda548f, DifficultyID 0 rows): eff0 APPLY_AURA MOD_STUN bp0
	// die0 tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius11 (45yd, real SpellRadius row added below); cast1
	// instant, dur1 10000ms, range1 Self Only; dispel 0, mechanic 0;
	// no SpellPower, SpellClassOptions or SpellAuraOptions rows.
	{id: 6432, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 6435 Smite Slam (wago name; C++ SPELL_SMITE_SLAM,
	// boss_mr_smite.cpp:36 enum; Lua
	// eastern_kingdoms/boss_mr_smite.lua:71, victim cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 8bda548f, DifficultyID 0 rows): eff0 APPLY_AURA MOD_STUN bp0
	// die0 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 SCHOOL_DAMAGE bp49
	// die1 same target; cast1 instant, dur27 3000ms, range2 Combat
	// 5yd; spell mechanic 12, dispel 0; eff1 BonusCoefficient 1 not
	// carried (no field); no SpellPower, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 6435, mechanic: 12, castTimeIdx: 1, durIdx: 27, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 1, basePoints: 49, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 5568 Trample (wago name; C++ SPELL_TRAMPLE,
	// boss_ramstein_the_gorger.cpp:33 enum; Lua
	// eastern_kingdoms/boss_ramstein_the_gorger.lua:59, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 8bda548f, DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp7 die1 tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius13 (10yd); cast1 instant, dur0 (no SpellDuration row),
	// range1 Self Only; dispel 0, mechanic 0; RealPointsPerLevel 1
	// not carried (no field); no SpellPower, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 5568, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 7, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 17307 Knockout (wago name; C++ SPELL_KNOCKOUT,
	// boss_ramstein_the_gorger.cpp:34 enum; Lua
	// eastern_kingdoms/boss_ramstein_the_gorger.lua:60, victim cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 8bda548f, DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp299 die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK
	// bp112 die75 misc75 same target; eff2 APPLY_AURA MOD_STUN bp0
	// die0 same target; cast1 instant, dur32 6000ms, range7 Ten yards
	// 10yd; dispel 0, mechanic 0; eff2 BonusCoefficient 1 not carried
	// (no field); no SpellPower, SpellClassOptions or SpellAuraOptions
	// rows.
	{id: 17307, castTimeIdx: 1, durIdx: 32, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 299, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 75, basePoints: 112, misc: 75, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 13874 Divine Shield (wago name; C++ SPELL_DIVINE_SHIELD,
	// boss_order_of_silver_hand.cpp:51 enum; Lua
	// eastern_kingdoms/boss_silver_hand_bosses.lua:59, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9cf61680, DifficultyID 0 rows): eff0 APPLY_AURA
	// SCHOOL_IMMUNITY bp-1 die1 misc 0x1 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA MOD_MELEE_HASTE bp-101 die1 same target;
	// eff2 APPLY_AURA SCHOOL_IMMUNITY bp0 die0 misc 0x7e same
	// target; cast1 instant, dur31 8000ms, range1 Self Only, school
	// Holy; manaCost 75 (SpellPower row); dispel 0, mechanic 0; no
	// SpellAuraOptions or SpellClassOptions rows.
	{id: 13874, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolHoly, manaCost: 75, effects: [3]effect{
		{effect: effApplyAura, aura: auraSchoolImmunity, dieSides: 1, basePoints: -1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: -101, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraSchoolImmunity, misc: 126, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 25263 Holy Light (wago name; C++ SPELL_HOLY_LIGHT,
	// boss_order_of_silver_hand.cpp:50 enum; Lua
	// eastern_kingdoms/boss_silver_hand_bosses.lua:58, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 9cf61680, DifficultyID 0 rows): eff0 HEAL bp1093
	// die313 tgtA TARGET_UNIT_TARGET_ALLY; cast20 2500ms (real
	// SpellCastTimes row added below), dur0 (no SpellDuration row),
	// range5 Long 40yd, school Holy; manaCost 150 (SpellPower row);
	// dispel 0, mechanic 0; BonusCoefficient 1 not carried (no
	// field); no SpellAuraOptions or SpellClassOptions rows.
	{id: 25263, castTimeIdx: 20, durIdx: 0, rangeIdx: 5, school: schoolHoly, manaCost: 150, effects: [3]effect{
		{effect: effHeal, dieSides: 313, basePoints: 1093, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 25805 Soul Corruption (wago name; C++ SPELL_SOUL_CORRUPTION,
	// zone_duskwood.cpp:37 enum; Lua
	// eastern_kingdoms/boss_twilight_corrupter.lua:76, self cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9cf61680, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_STAT bp-101 die1 misc -1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius11 (45yd); eff1 APPLY_AURA
	// PERIODIC_DAMAGE bp739 die121 period5000 same targets/radius;
	// eff2 KNOCK_BACK bp184 die1 misc125 same targets/radius; cast1
	// instant, dur8 15000ms, range1 Self Only, school Shadow; dispel
	// 0, mechanic 0 (no SpellCategories row); eff0/eff1
	// BonusCoefficient 1 not carried (no field); no SpellPower,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 25805, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModStat, dieSides: 1, basePoints: -101, misc: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 121, basePoints: 739, auraPeriod: 5000, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 184, misc: 125, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 25806 Creature of Nightmare (wago name; C++
	// SPELL_CREATURE_OF_NIGHTMARE, zone_duskwood.cpp:38 enum; Lua
	// eastern_kingdoms/boss_twilight_corrupter.lua:77, victim cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9cf61680, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_CHARM bp64 die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// APPLY_AURA MOD_DAMAGE_PERCENT_DONE bp199 die1 misc127 same
	// target; eff2 APPLY_AURA MOD_MELEE_HASTE bp99 die1 same target;
	// cast1 instant, dur9 30000ms, range13 Anywhere 50000yd, school
	// Shadow; dispel 0, mechanic 0; eff0 RealPointsPerLevel 1 and
	// eff1/eff2 BonusCoefficient 1 not carried (no fields); no
	// SpellPower, SpellClassOptions or SpellAuraOptions rows.
	{id: 25806, castTimeIdx: 1, durIdx: 9, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCharm, dieSides: 1, basePoints: 64, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 199, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 99, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 5200 VanCleef's Allies (wago name; C++
	// SPELL_VANCLEEFS_ALLIES, boss_vancleef.cpp:26 enum; Lua
	// eastern_kingdoms/boss_vancleef.lua:81, self cast at 50%) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read c95b2873, DifficultyID 0 rows): eff0 SUMMON bp1
	// die1 misc 636 miscB 61 tgtA TARGET_DEST_CASTER_SUMMON;
	// cast1 instant, dur5 300000ms, range1 Self Only, school
	// Physical; dispel 0, mechanic 0; EffectAmplitude 1000 and
	// Attributes_0 0x10 not carried (no fields); no SpellAuraOptions,
	// SpellCategories, SpellPower or SpellClassOptions rows.
	{id: 5200, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, basePoints: 1, misc: 636, miscB: 61, targetA: targetDestCasterSummon, chainAmplitude: 1},
	}},
	// 12787 Thrash (wago name; C++ SPELL_THRASH,
	// boss_vancleef.cpp:25 enum; Lua
	// eastern_kingdoms/boss_vancleef.lua:80, self cast on engage;
	// also Azgalor Reset and Malchezaar phase-2) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read c95b2873, DifficultyID 0 rows): eff0 APPLY_AURA
	// PROC_TRIGGER_SPELL bp0 die1 trigger 3391 (seeded) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1/infinite, range1
	// Self Only, school Physical; dispel 0, mechanic 0;
	// SpellAuraOptions ProcChance 35 / ProcCategoryRecovery 4000 /
	// ProcTypeMask 0x14 and SpellCategories Category 65 DefenseType
	// Melee PreventionType 0x2 not carried (no fields); no
	// SpellPower or SpellClassOptions rows.
	{id: 12787, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: 0, trigger: 3391, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12885 Teleport to Razelikh (wago name; C++
	// SPELL_TELEPORT_SINGLE, zone_blasted_lands.cpp:32 enum; Lua
	// eastern_kingdoms/zone_blasted_lands.lua:28, player cast on
	// self when alone) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read c95b2873, DifficultyID 0
	// rows): eff0 TELEPORT_UNITS bp0 die1 tgtA
	// TARGET_UNIT_TARGET_ALLY tgtB TARGET_DEST_DB; cast1 instant,
	// dur0 (no SpellDuration row), range3 Twenty yards 20yd, school
	// Physical; dispel 0, mechanic 0; EffectAmplitude 1 and
	// EffectPos_facing 4.71 not carried (no fields); SpellCategories
	// Category 0 DefenseType Magic PreventionType 0x1 not carried;
	// no SpellAuraOptions, SpellPower or SpellClassOptions rows.
	{id: 12885, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, basePoints: 0, targetA: targetUnitTargetAlly, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 13142 Teleport to Razelikh (wago name; C++
	// SPELL_TELEPORT_SINGLE_IN_GROUP, zone_blasted_lands.cpp:33
	// enum; Lua eastern_kingdoms/zone_blasted_lands.lua:29, group
	// members cast on themselves, triggered) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read c95b2873, DifficultyID 0 rows): DB2-identical to 12885
	// in every reported field value except the DB2 row IDs; eff0
	// TELEPORT_UNITS bp0 die1 tgtA TARGET_UNIT_TARGET_ALLY tgtB
	// TARGET_DEST_DB; cast1 instant, dur0 (no SpellDuration row),
	// range3 Twenty yards 20yd, school Physical; dispel 0,
	// mechanic 0; EffectAmplitude 1 and EffectPos_facing 4.71 not
	// carried (no fields); SpellCategories Category 0 DefenseType
	// Magic PreventionType 0x1 not carried; no SpellAuraOptions,
	// SpellPower or SpellClassOptions rows.
	{id: 13142, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 1, basePoints: 0, targetA: targetUnitTargetAlly, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 22427 Concussion Blow (wago name; C++ SPELL_CONCUSSION_BLOW,
	// boss_broggok.cpp:43 enum; Lua
	// hellfirecitadel/npc_nascent_fel_orc.lua:53, pump 15s init then
	// {8s,11s}) — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read f1465377, DifficultyID 0 rows): eff0
	// WEAPON_DAMAGE bp-1 die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// APPLY_AURA MOD_STUN bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur28 5000ms, range7 Ten yards 10yd, school
	// Physical; SpellCategories Mechanic 12 (stunned), DefenseType
	// Melee, PreventionType 0x2; family Warrior (SpellClassSet 4)
	// mask 69206016; Attributes_0 0x14 and EffectBonusCoefficient 1
	// not carried (no fields); no SpellPower or SpellAuraOptions rows.
	{id: 22427, castTimeIdx: 1, durIdx: 28, rangeIdx: 7, school: schoolPhysical, mechanic: 12, family: familyWarrior, familyFlags: [3]uint32{69206016, 0, 0}, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: -1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, dieSides: 0, basePoints: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 31900 Stomp (wago name; C++ SPELL_STOMP, boss_broggok.cpp:42
	// enum; Lua hellfirecitadel/npc_nascent_fel_orc.lua:54, pump 7s
	// init then {16s,21s}) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-16 browser read f1465377, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE bp329 die41 radius8 5yd tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY; eff1
	// KNOCK_BACK bp44 die31 misc60 radius8 tgtA 22 tgtB 15; cast1
	// instant, dur0 (no SpellDuration row), range7 Ten yards 10yd,
	// school Physical; SpellCategories Mechanic 0, DefenseType Magic,
	// PreventionType 0x1; SpellAuraOptions ProcChance 101 not carried
	// (no fields); Attributes_0 0x40010 not carried; no SpellPower or
	// SpellClassOptions rows.
	{id: 31900, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 41, basePoints: 329, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 31, basePoints: 44, misc: 60, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 12739 Shadow Bolt (wago name; C++ SPELL_SHADOW_BOLT,
	// boss_kelidan_the_breaker.cpp:265 enum; Lua
	// hellfirecitadel/npc_shadowmoon_channeler.lua:64) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read f1465377, DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp127
	// die45 tgtA TARGET_UNIT_TARGET_ENEMY; cast14 3000ms, dur0 (no
	// SpellDuration row), range5 Long Range 40yd, school Shadow,
	// SpellPower ManaCost 90 carried, Speed 24 not carried (no
	// field); SpellCategories Mechanic 0, DefenseType Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 12739, castTimeIdx: 14, durIdx: 0, rangeIdx: 5, school: schoolShadow, manaCost: 90, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 45, basePoints: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 30937 Mark of Shadow (wago name; C++ SPELL_MARK_OF_SHADOW,
	// boss_kelidan_the_breaker.cpp:268 enum; Lua
	// hellfirecitadel/npc_shadowmoon_channeler.lua:63, pump
	// 5000+rand32()%2000 init then {15s,20s}) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read f1465377, DifficultyID 0 row; heroic DifficultyID 173/174
	// rows bp1000 die0 RealPointsPerLevel 100 NOT seeded): eff0
	// APPLY_AURA MOD_DAMAGE_TAKEN bp999 die1 misc32 (Shadow school)
	// tgtA TARGET_UNIT_TARGET_ENEMY; RealPointsPerLevel 50 not
	// carried (no field); cast4 1000ms, dur4 120000ms, range4 Medium
	// Range 30yd, school Shadow, dispel 1 (MAGIC); SpellCategories
	// Mechanic 0, DefenseType Magic, PreventionType 0x1;
	// SpellAuraOptions ProcChance 100 / ProcCharges 1 / ProcTypeMask
	// 0x222a0 not carried (no fields); no SpellPower or
	// SpellClassOptions rows.
	{id: 30937, dispel: 1, castTimeIdx: 4, durIdx: 4, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageTaken, dieSides: 1, basePoints: 999, misc: 32, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 3490 Frenzied Rage (wago name; C++ SPELL_FRENZIED_RAGE,
	// boss_aku_mai.cpp:25 enum; Lua kalimdor/boss_aku_mai.lua:43,
	// DamageTaken HealthBelowPctDamaged(30) one-shot self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 0b3d22c7, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_MELEE_HASTE bp74 die1 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_DAMAGE_DONE bp-16 die1 misc1 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur28 5000ms, range2 Combat
	// Range 5yd, school Physical; SpellCategories Category 21,
	// DefenseType 0, DispelType 0, Mechanic 0, PreventionType 0x0;
	// Attributes_0 0x40010 not carried (no field); no SpellPower,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 3490, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 74, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageDone, dieSides: 1, basePoints: -16, misc: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 3815 Poison Cloud (wago name; C++ SPELL_POISON_CLOUD,
	// boss_aku_mai.cpp:24 enum; Lua kalimdor/boss_aku_mai.lua:42,
	// JustEngagedWith DoCastVictim, init {5s,9s} -> {25s,50s}) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 0b3d22c7, DifficultyID 0 row): eff0 APPLY_AURA
	// PERIODIC_DAMAGE bp49 die1 auraPeriod 5000 radius14 8yd tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast4
	// 1000ms, dur22 45000ms, range1 Self Only, school Nature,
	// dispel 4 (POISON); SpellCategories Mechanic 0, DefenseType
	// Magic, PreventionType 0x1; RealPointsPerLevel 1.5 and
	// EffectBonusCoefficient 1 not carried (no fields); Attributes_1
	// 0x88 not carried; no SpellPower, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 3815, dispel: 4, castTimeIdx: 4, durIdx: 22, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 49, auraPeriod: 5000, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
	}},
	// 16245 Freeze Anim (wago name; C++ SPELL_FREEZE_ANIM,
	// boss_cthun.cpp:84 enum; Lua kalimdor/boss_eye_of_cthun.lua:122)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 0b3d22c7, DifficultyID 0 row): eff0
	// APPLY_AURA DUMMY bp0 die1 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1/infinite, range1 Self Only, school Physical;
	// visual/marker aura only — no damage or healing effect in DB2;
	// no SpellCategories, SpellClassOptions, SpellAuraOptions or
	// SpellPower rows; Attributes_0 0x100 / Attributes_1 0x10000000
	// not carried (no fields).
	{id: 16245, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 22518 Glowy (Red) (wago SpellName; C++ SPELL_RED_COLORATION,
	// boss_cthun.cpp:87 enum; Lua kalimdor/boss_eye_of_cthun.lua:121)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 0b3d22c7, DifficultyID 0 row): eff0
	// APPLY_AURA DUMMY bp0 die0 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1/infinite, range1 Self Only, school Arcane;
	// visual/marker aura only — no damage or healing effect in DB2;
	// no SpellCategories, SpellClassOptions, SpellAuraOptions or
	// SpellPower rows; Attributes_0 0x180 not carried (no field).
	{id: 22518, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28467 Mortal Wound (wago name; C++ SPELL_MORTAL_WOUND,
	// boss_fankriss.cpp:38 enum; Lua kalimdor/boss_fankriss.lua:60,
	// DoCastVictim, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read 906b92ac,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_HEALING_PCT bp-11
	// die1 misc126 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// WEAPON_PERCENT_DAMAGE bp149 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur8 15000ms, range7
	// Ten yards 10yd, school Physical; SpellCategories Mechanic 0,
	// DefenseType Melee, PreventionType 0x2; SpellAuraOptions
	// ProcChance 101 / CumulativeAura 8 not carried (no fields);
	// Attributes_0 538968064 / Attributes_3 262144 not carried; no
	// SpellPower or SpellClassOptions rows.
	{id: 28467, castTimeIdx: 1, durIdx: 8, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModHealingPct, dieSides: 1, basePoints: -11, misc: 126, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 149, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 28858 Entangling Roots (wago SpellName; C++ SPELL_ROOT,
	// boss_fankriss.cpp:39 enum; Lua kalimdor/boss_fankriss.lua:61,
	// DoCast(target) non-triggered -> ModifyThreatByPercent -100) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 906b92ac, DifficultyID 0 rows): eff0 APPLY_AURA
	// ROOT bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// PERIODIC_DAMAGE bp24 die1 auraPeriod 3000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur8 15000ms, range4
	// Medium Range 30yd, school Nature, mechanic 7 (ROOT);
	// SpellCategories Category 99, DefenseType 0, PreventionType
	// 0x2, StartRecoveryCategory 133 not carried; Attributes_0
	// 1074331648 not carried; no SpellPower, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 28858, castTimeIdx: 1, durIdx: 8, rangeIdx: 4, school: schoolNature, mechanic: 7, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, dieSides: 0, basePoints: 0, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 24, auraPeriod: 3000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 18670 Knock Away (wago name; C++ SPELL_KNOCKAWAY,
	// boss_landslide.cpp:31 enum; Lua kalimdor/boss_landslide.lua:44,
	// DoCastVictim, non-triggered, init 8s -> 15s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16 browser
	// read 906b92ac, DifficultyID 0 rows): eff0 WEAPON_DAMAGE bp29
	// die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK bp131
	// die87 misc125 tgtA TARGET_UNIT_TARGET_ENEMY; eff2 SCRIPT_EFFECT
	// bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// (no SpellDuration row), range7 Ten yards 10yd, school Physical;
	// SpellCategories Mechanic 0, DefenseType Melee, PreventionType
	// 0x2; Attributes_0 16 not carried; no SpellPower,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 18670, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 29, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 87, basePoints: 131, misc: 125, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effScriptEffect, dieSides: 0, basePoints: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 21808 Landslide (wago name; C++ SPELL_LANDSLIDE,
	// boss_landslide.cpp:33 enum; Lua kalimdor/boss_landslide.lua:46,
	// non-triggered self-cast pump) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read 906b92ac,
	// DifficultyID 0 rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die1 auraPeriod 2000 trigger 21809 (NOT seeded — trigger
	// target, see checkpoint note) tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA STUN bp0 die1 radius12 100yd tgtA TARGET_DEST_CASTER
	// tgtB TARGET_UNIT_DEST_AREA_ENEMY; cast1 instant, dur31 8000ms,
	// range1 Self Only, school Physical, mechanic 12 (stunned);
	// SpellCategories DefenseType 0, PreventionType 0x0; Attributes_1
	// 64 not carried; no SpellPower, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 21808, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: 0, auraPeriod: 2000, trigger: 21809, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, dieSides: 1, basePoints: 0, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 21687 Toxic Volley (wago name; C++ SPELL_TOXICVOLLEY,
	// boss_noxxion.cpp:31 enum; Lua kalimdor/boss_noxxion.lua:43,
	// DoCastVictim, non-triggered, init 7s -> 9s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16 browser
	// read 5c905176, DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp74
	// die51 tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius11 45yd; eff1 APPLY_AURA PERIODIC_DAMAGE bp4 die1
	// auraPeriod 3000 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius11 45yd; cast1 instant, dur8
	// 15000ms, range4 Medium Range 30yd, school Nature, dispel 4
	// (POISON); SpellCategories Mechanic 0, DefenseType 0,
	// PreventionType 0x0; SpellAuraOptions CumulativeAura 20 /
	// ProcChance 101 not carried (no fields); Attributes_0 0x80010
	// not carried; Speed 40 not carried; no SpellPower or
	// SpellClassOptions rows.
	{id: 21687, castTimeIdx: 1, durIdx: 8, rangeIdx: 4, school: schoolNature, dispel: 4, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 51, basePoints: 74, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, dieSides: 1, basePoints: 4, auraPeriod: 3000, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 22916 Uppercut (wago name; C++ SPELL_UPPERCUT,
	// boss_noxxion.cpp:32 enum; Lua kalimdor/boss_noxxion.lua:44,
	// DoCastVictim, non-triggered, init 16s -> 12s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16 browser
	// read 5c905176, DifficultyID 0 rows): eff0 WEAPON_DAMAGE bp49
	// die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK bp93
	// die63 misc10 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range7 Ten yards 10yd, school
	// Physical; SpellCategories Mechanic 0, DefenseType Melee,
	// PreventionType 0x2; Attributes_0 16 not carried; no
	// SpellPower, SpellClassOptions or SpellAuraOptions rows.
	{id: 22916, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 49, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 63, basePoints: 93, misc: 10, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 18392 Fireball (wago name; C++ SPELL_FIREBALL,
	// boss_onyxia.cpp:59 enum; Lua kalimdor/boss_onyxia.lua:183,
	// DoCast(random target), phase BREATH) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read 5c905176,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp4712 die575 tgtA
	// TARGET_DEST_TARGET_ENEMY tgtB TARGET_UNIT_DEST_AREA_ENEMY
	// radius14 8yd; cast14 3000ms, dur0 (no SpellDuration row),
	// range13 Anywhere - Unlimited 50000yd, school Fire, Speed 65
	// not carried; DifficultyID 4/2 rows bp7068 die863 and
	// DifficultyID 9 row bp1699 die601 NOT seeded (base row only);
	// SpellCategories DefenseType Magic, PreventionType 0x1,
	// StartRecoveryCategory 133 not carried; Attributes_0 0x10000 /
	// Attributes_1 0x88 / Attributes_2 0x4 not carried; no
	// SpellPower, SpellClassOptions or SpellAuraOptions rows.
	{id: 18392, castTimeIdx: 14, durIdx: 0, rangeIdx: 13, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 575, basePoints: 4712, targetA: targetDestTargetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
	}},
	// 68867 Tail Sweep (wago name; C++ SPELL_TAIL_SWEEP,
	// boss_onyxia.cpp:55 enum; Lua kalimdor/boss_onyxia.lua:182,
	// DoCastAOE, non-triggered, self-cast) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read 5c905176,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp8324 die1351 tgtA
	// TARGET_UNIT_CONE_ENEMY_54 radius10 30yd; eff1 KNOCK_BACK
	// bp149 die1 misc300 tgtA TARGET_UNIT_CONE_ENEMY_54 radius10
	// 30yd; cast1 instant, dur0 (no SpellDuration row), range4
	// Medium Range 30yd, school Physical, mechanic 12 (stunned);
	// DifficultyID 4/2 rows bp12024 die1951 NOT seeded (base row
	// only); SpellCategories DefenseType 0, PreventionType 0x0;
	// SpellAuraOptions ProcChance 101 not carried; Attributes_1
	// 0x88 / Attributes_3 0x100 not carried; no SpellPower or
	// SpellClassOptions rows.
	{id: 68867, castTimeIdx: 1, durIdx: 0, rangeIdx: 4, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1351, basePoints: 8324, targetA: targetUnitConeEnemy, radiusIdx: 10, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 149, misc: 300, targetA: targetUnitConeEnemy, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 25599 Thundercrash (wago SpellName, one word; C++
	// SPELL_THUNDERCRASH, boss_rajaxx.cpp:50 enum; Lua
	// kalimdor/boss_rajaxx.lua:45, DoCast(me) self-cast,
	// non-triggered) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read 21a13222, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp0 die0 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius12 100yd; eff1
	// KNOCK_BACK bp99 die1 misc250 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius12 100yd; cast1 instant,
	// dur0 (no SpellDuration row), range13 Anywhere - Unlimited
	// 50000yd, school Nature; SpellCategories Category 22,
	// DefenseType Magic, PreventionType 0x1, StartRecoveryCategory
	// 133 not carried; Attributes_0 0x20010000 / Attributes_1 0x88 /
	// Attributes_2 0x4 not carried; no SpellPower,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 25599, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 12, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 99, misc: 250, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 6713 Disarm (wago name; C++ SPELL_DISARM,
	// boss_rajaxx.cpp:48 enum; Lua kalimdor/boss_rajaxx.lua:44,
	// DoCastVictim, non-triggered, init 10s -> 22s) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read 21a13222, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DISARM bp-1 die1 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur28 5000ms, range2 Combat Range 5yd, school
	// Physical, mechanic 3; DifficultyID 173/174 rows bp0 die0 +
	// dur32 NOT seeded (base row only); SpellPower row all-zero
	// costs not carried; SpellCategories DefenseType Melee,
	// PreventionType 0x2; Attributes_0 0x40410 not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 6713, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, mechanic: 3, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDisarm, dieSides: 1, basePoints: -1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 26006 Shadow Bolt (wago name; C++ SPELL_SHADOWBOLT,
	// boss_twinemperors.cpp:46 enum; Lua
	// kalimdor/boss_veklor.lua:93, DoCastVictim) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16 browser
	// read 21a13222, DifficultyID 0 row): eff0 SCHOOL_DAMAGE
	// bp3599 die801 tgtA TARGET_UNIT_TARGET_ENEMY; cast4 1000ms,
	// dur0 (no SpellDuration row), range36 Longer Range 45yd,
	// school Shadow, manaCost 90, Speed 35 not carried;
	// SpellCategories DefenseType Magic, PreventionType 0x1,
	// StartRecoveryCategory 133 not carried; Attributes_0 0x10000 /
	// Attributes_1 0x88 / Attributes_2 0x4 not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 26006, castTimeIdx: 4, durIdx: 0, rangeIdx: 36, school: schoolShadow, manaCost: 90, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 801, basePoints: 3599, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 26607 Blizzard (wago name; C++ SPELL_BLIZZARD,
	// boss_twinemperors.cpp:47 enum; Lua
	// kalimdor/boss_veklor.lua:94, SelectTarget Random) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 21a13222, DifficultyID 0 rows):
	// eff0 PERSISTENT_AREA_AURA PERIODIC_DAMAGE bp1387 die225
	// auraPeriod 2000 tgtA TARGET_DEST_DYNOBJ_ENEMY radius13 10yd;
	// eff1 PERSISTENT_AREA_AURA MOD_DECREASE_SPEED bp-31 die1
	// tgtA TARGET_DEST_DYNOBJ_ENEMY radius13 10yd; cast1 instant,
	// dur1 10000ms, range36 Longer Range 45yd, school Frost;
	// SpellCategories DefenseType Magic, PreventionType 0x1;
	// Attributes_1 0x88 / Attributes_2 0x4 / Attributes_3 0x100 not
	// carried; no SpellPower, SpellClassOptions or SpellAuraOptions
	// rows.
	{id: 26607, castTimeIdx: 1, durIdx: 1, rangeIdx: 36, school: schoolFrost, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, dieSides: 225, basePoints: 1387, auraPeriod: 2000, targetA: targetDestDynobjEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraDecSpeed, dieSides: 1, basePoints: -31, targetA: targetDestDynobjEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 26027 Knockback (wago name; C++ SPELL_KNOCKBACK,
	// boss_bug_trio.cpp:40 enum; Lua kalimdor/boss_vem.lua:29,
	// DoCastVictim, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// bf85b35e, DifficultyID 0 row): eff0 KNOCK_BACK bp249 die1
	// misc250 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius13 10yd; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; SpellCategories DefenseType Melee, PreventionType
	// 0x2 not carried; Attributes_0 0x10 not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 26027, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effKnockBack, dieSides: 1, basePoints: 249, misc: 250, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 26561 Berserker Charge (wago SpellName, NOT "Charge"; C++
	// SPELL_CHARGE, boss_bug_trio.cpp:39 enum; Lua
	// kalimdor/boss_vem.lua:28, random threat-list any-unit alive)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read bf85b35e, DifficultyID 0 rows):
	// eff0 CHARGE bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// KNOCK_BACK bp349 die1 misc200 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff2 WEAPON_DAMAGE bp299 die1
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0 (no
	// SpellDuration row), range5 Long Range 40yd, school
	// Physical, Speed 27 not carried; no SpellCategories,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 26561, castTimeIdx: 1, durIdx: 0, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, dieSides: 0, basePoints: 0, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 349, misc: 200, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effWeaponDamage, dieSides: 1, basePoints: 299, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 11086 Ward of Zum'rah (wago name; C++
	// SPELL_WARD_OF_ZUM_RAH, boss_zum_rah.cpp:39 enum; Lua
	// kalimdor/boss_zum_rah.lua:60, self-cast at 1s +
	// HealthBelowPct(40) re-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// bf85b35e, DifficultyID 0 row): eff0 SUMMON bp4 die1 misc
	// 7785 miscB 121 tgtA TARGET_DEST_CASTER_SUMMON; cast1
	// instant, dur18 20000ms, range1 Self Only 0yd, school
	// Physical; no SpellCategories, SpellClassOptions or
	// SpellAuraOptions rows. SummonProperties 121 row seeded
	// below (wago.tools 3.4.0.47659, browser 3e2e3ba6): Control 1
	// Guardian, Faction 0, Title 0, Slot 0, Flags_0 0x2 — the Go
	// summon leg spawns entry 7785 as an ALLY temp summon with the
	// caster's faction (guardian flag off: Title 0, Flags 0x2
	// lacks the 512 bit).
	{id: 11086, castTimeIdx: 1, durIdx: 18, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, basePoints: 4, misc: 7785, miscB: 121, targetA: targetDestCasterSummon, chainAmplitude: 1},
	}},
	// 15245 Shadow Bolt Volley (wago SpellName, two words, NOT
	// "Shadowbolt Volley"; C++ SPELL_SHADOWBOLT_VOLLEY,
	// boss_zum_rah.cpp:38 enum; Lua kalimdor/boss_zum_rah.lua:59,
	// DoCast(target) at 10s repeating 9s, non-triggered) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read bf85b35e, DifficultyID 0 row):
	// eff0 SCHOOL_DAMAGE bp127 die45 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius10 30yd; cast14
	// 3000ms, dur0 (no SpellDuration row), range4 Medium Range
	// 30yd, school Shadow; SpellCategories DefenseType Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 not carried;
	// Attributes_0 0x90000 / Attributes_1 0x88 not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 15245, castTimeIdx: 14, durIdx: 0, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 45, basePoints: 127, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 16636 Berserker Charge (wago name; C++
	// SPELL_BERSERKER_CHARGE, zone_thunder_bluff.cpp:36 enum;
	// Lua kalimdor/npc_cairne_bloodhoof.lua:53, DoCast(target),
	// non-triggered) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read 5d61aa94,
	// DifficultyID 0 rows): eff0 CHARGE bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK bp349 die1
	// misc200 tgtA TARGET_UNIT_TARGET_ENEMY; eff2 WEAPON_DAMAGE
	// bp299 die1 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range5 Long Range 40yd, school
	// Physical, Speed 27 not carried; no SpellCategories,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 16636, castTimeIdx: 1, durIdx: 0, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, dieSides: 0, basePoints: 0, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 349, misc: 200, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effWeaponDamage, dieSides: 1, basePoints: 299, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 23123 Cairne's Hoofprint (wago SpellName, NOT "Hoofprint";
	// C++ zone_thunder_bluff.cpp:132, player->CastSpell(player,
	// 23123, false), no enum name; Lua
	// kalimdor/npc_cairne_bloodhoof.lua:160, gossip action INFO 3
	// player self-cast, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 5d61aa94, DifficultyID 0 row): eff0 CREATE_ITEM bp0 die1
	// itemType 18643 tgtA TARGET_UNIT_TARGET_ANY (25, NOT
	// self-only — seeded as DB2 reads); cast4 1000ms, dur0 (no
	// SpellDuration row), range13 Anywhere - Unlimited 50000yd,
	// school Physical; no SpellCategories, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 23123, castTimeIdx: 4, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effCreateItem, dieSides: 1, basePoints: 0, itemType: 18643, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 26139 Ground Rupture (wago name; C++
	// SPELL_GROUND_RUPTURE, boss_cthun.cpp:93 enum; Lua
	// kalimdor/npc_claw_tentacle.lua:17, claw tentacle AoE) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 5d61aa94, DifficultyID 0 rows):
	// eff0 SCHOOL_DAMAGE bp1349 die301 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius13 10yd; eff1
	// KNOCK_BACK bp99 die1 misc150 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius13 10yd; eff2
	// SCRIPT_EFFECT bp0 die0 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius13 10yd; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; no SpellCategories, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 26139, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 301, basePoints: 1349, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 99, misc: 150, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effScriptEffect, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 26141 Hamstring (wago name; C++ SPELL_HAMSTRING,
	// boss_cthun.cpp:94 enum; Lua kalimdor/npc_claw_tentacle.lua:18,
	// Hamstring on victim, 2s init -> 5s repeat per C++) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 5d61aa94, DifficultyID 0 rows):
	// eff0 SCHOOL_DAMAGE bp262 die75 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA MOD_DECREASE_SPEED
	// bp-71 die1 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur1 10000ms, range2 Combat Range 5yd, school Physical,
	// mechanic 11 SNARE; SpellCategories DefenseType Melee,
	// PreventionType 0x2 not carried; Attributes_0 0x50010 /
	// Attributes_1 0x8000000 not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 26141, castTimeIdx: 1, durIdx: 1, rangeIdx: 2, school: schoolPhysical, mechanic: 11, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 75, basePoints: 262, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -71, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 11972 Shield Bash (wago name; C++ SPELL_SHIELD_BASH,
	// zulfarrak.cpp:53 enum; Lua kalimdor/npc_sergeant_bly.lua:66,
	// DoCastVictim, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 76ce4f8c, DifficultyID 0 rows): eff0 INTERRUPT_CAST bp4
	// die1 mechanic 26 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// SCHOOL_DAMAGE bp9 die1 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur32 6000ms, range2 Combat Range 5yd,
	// school Physical; SpellCategories DefenseType Melee,
	// PreventionType 0x2 not carried; Attributes_0 0xd0010 not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 11972, castTimeIdx: 1, durIdx: 32, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effInterruptCast, dieSides: 1, basePoints: 4, mechanic: 26, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 1, basePoints: 9, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 12170 Revenge (wago name; C++ SPELL_REVENGE,
	// zulfarrak.cpp:54 enum; Lua kalimdor/npc_sergeant_bly.lua:67,
	// DoCastVictim, non-triggered, init 8s, reschedule 10s) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 76ce4f8c, DifficultyID 0 row,
	// filter[SpellID] — filter[ID] is a substring filter and
	// returns the wrong SpellMisc row 612170/SpellID 63431):
	// eff0 SCHOOL_DAMAGE bp15 die5 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0 (no
	// SpellDuration row), range2 Combat Range 5yd, school
	// Physical; SpellCategories Category 65, DefenseType Melee,
	// PreventionType 0x2 not carried; Attributes_0 0x80010 /
	// Attributes_3 0x400 not carried; RealPointsPerLevel 0.9 NOT
	// carried (seed has no field — established precedent); no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 12170, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 5, basePoints: 15, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16033 Chain Lightning (wago name; C++
	// SPELL_CHAIN_LIGHTNING, zone_orgrimmar.cpp:168 enum; Lua
	// kalimdor/npc_thrall_warchief.lua:52, DoCastVictim,
	// non-triggered) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 76ce4f8c,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp462 die75
	// chainTargets 10 chainAmplitude 0.9 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast14 (verdict resolves
	// SpellCastTimes 14 as 1600ms, but that CONFLICTS with the
	// seed's established row 14 = 3000ms, verified by the 15245
	// verdict + the 3.4.0.43659 read for 67528 — likely a
	// substring-filter misread of the aux row, so NO new cast row
	// is added and cast14 resolves 3000ms here), dur0 (no
	// SpellDuration row), range4 30yd, school Nature;
	// SpellCategories Category
	// 85, DefenseType Magic, PreventionType 0x1,
	// StartRecoveryCategory 133 not carried; Attributes_0 0x90000
	// not carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 16033, castTimeIdx: 14, durIdx: 0, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 75, basePoints: 462, targetA: targetEnemy, chainTargets: 10, chainAmplitude: 0.9},
	}},
	// 16034 Shock (wago name; C++ SPELL_SHOCK,
	// zone_orgrimmar.cpp:169 enum; Lua
	// kalimdor/npc_thrall_warchief.lua:53, DoCastVictim,
	// non-triggered) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 76ce4f8c,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp531 die87 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0 (no
	// SpellDuration row), range3 20yd, school Nature;
	// SpellCategories Category 19, DefenseType Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 not carried;
	// Attributes_0 0xd0000 not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 16034, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 87, basePoints: 531, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 6660 Shoot (wago name; C++ SPELL_SHOOT, zulfarrak.cpp:253
	// enum; Lua kalimdor/npc_weegli_blastfuse.lua:85, Shoot when
	// victim beyond melee range) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// bc333097, DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp35
	// die13 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// (no SpellDuration row), range54 Monster Shoot 5–30yd,
	// school Physical, Speed 40 not carried; SpellCategories
	// DefenseType Ranged, PreventionType 0x2 not carried;
	// Attributes_0 0x480012 not carried; SpellClassOptions
	// SpellClassSet 9 Hunter, SpellClassMask_0 1 carried as
	// family + familyFlags.
	{id: 6660, castTimeIdx: 1, durIdx: 0, rangeIdx: 54, school: schoolPhysical, family: familyHunter, familyFlags: [3]uint32{1, 0, 0}, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 13, basePoints: 35, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 8858 Bomb (wago name; C++ SPELL_BOMB, zulfarrak.cpp:251
	// enum; Lua kalimdor/npc_weegli_blastfuse.lua:84,
	// DoCastVictim, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// bc333097, DifficultyID 0 row): eff0 WEAPON_DAMAGE bp-1
	// die1 tgtA TARGET_UNIT_DEST_AREA_ENEMY radius8 5yd; cast5
	// 2000ms, dur0 (no SpellDuration row), range54 Monster
	// Shoot 5–30yd, school Fire, Speed 14 not carried; no
	// SpellCategories row (filtering by SpellID 8858 returns
	// only 28858/38858/68858), SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 8858, castTimeIdx: 5, durIdx: 0, rangeIdx: 54, school: schoolFire, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: -1, targetA: targetUnitDestAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 25824 Spotlight (wago name; C++ SPELL_SPOTLIGHT,
	// karazhan.cpp:45 enum; Lua karazhan/npc_barnes.lua:62,
	// spotlight cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read bc333097,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1 infinite,
	// range4 Medium Range 30yd, school Physical; no
	// SpellCategories, SpellClassOptions or SpellAuraOptions
	// rows.
	{id: 25824, castTimeIdx: 1, durIdx: 21, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 32616 Barnes Tuxedo Transform (wago SpellName, NOT
	// "Tuxedo"; C++ SPELL_TUXEDO, karazhan.cpp:46 enum; Lua
	// karazhan/npc_barnes.lua:61) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// bc333097, DifficultyID 0 row): eff0 APPLY_AURA TRANSFORM
	// bp0 die0 misc 18643 tgtA TARGET_UNIT_CASTER; cast3
	// 500ms, dur21 -1 infinite, range1 Self Only 0yd, school
	// Nature; no SpellCategories, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 32616, castTimeIdx: 3, durIdx: 21, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, dieSides: 0, basePoints: 0, misc: 18643, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 70013 Quel'Delar's Compulsion (wago SpellName, NOT
	// "Quel'Delar Compulsion"; C++
	// SPELL_QUEL_DELAR_COMPULSION, halls_of_reflection.h:176;
	// Lua northrend/at_hor_triggers.lua:93, Quel'Delar leg
	// HasAura check) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 6827d471,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp-1 die1 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1 infinite,
	// range1 Self Only 0yd, school Physical; no SpellCategories,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 70013, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72712 Falric/Marwyn Spawn Aura (wago SpellName, NOT "Boss
	// Spawn Aura" — the C++ name is a generic label; C++
	// SPELL_BOSS_SPAWN_AURA, halls_of_reflection.cpp:225; Lua
	// northrend/at_hor_triggers.lua:94, Falric/Marwyn triggered
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 6827d471,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur29 12000ms, range1
	// Self Only 0yd, school Physical; no SpellCategories,
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 72712, castTimeIdx: 1, durIdx: 29, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50774 Thundering Stomp (wago name; C++
	// SPELL_THUNDERING_STOMP, boss_drakos.cpp:27 enum; Lua
	// northrend/boss_drakos.lua:104, DoCast self-cast, 15s init
	// -> 15s repeat) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-16 browser read 6827d471,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp1899 die201
	// tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius48 60yd; eff1 KNOCK_BACK bp149 die1 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius48 60yd; cast4 1000ms, dur0 (no SpellDuration row),
	// range1 Self Only 0yd, school Physical; SpellCategories
	// Category 1152, DefenseType Magic, PreventionType 0x1 not
	// carried; Attributes_1 0x88 / Attributes_2 0x4 not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 50774, castTimeIdx: 4, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 201, basePoints: 1899, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 48, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 149, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 48, chainAmplitude: 1},
	}},
	// 51336 Magic Pull (wago name; C++ SPELL_MAGIC_PULL,
	// boss_drakos.cpp:26 enum; Lua northrend/boss_drakos.lua:103,
	// DoCast self-cast, EVENT_MAGIC_PULL) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read 6827d471, DifficultyID 0 row): eff0 DUMMY
	// bp0 die0 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius33 99yd; cast5 2000ms,
	// dur0 (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; no SpellCategories, SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 51336, castTimeIdx: 5, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 33, chainAmplitude: 1},
	}},
	// 47008 Berserk (wago name; C++ SPELL_BERSERK,
	// boss_razorscale.cpp:66 enum; Lua northrend/boss_razorscale.lua:301,
	// DoCastSelf, EVENT_BERSERK) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 2a7ab516, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DAMAGE_PERCENT_DONE bp899 die1 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA MOD_MELEE_HASTE bp149 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE bp24 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur30 1800000ms,
	// range1 Self Only 0yd, school Physical; SpellCategories
	// Category 21, DefenseType 0 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 47008, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 899, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 149, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 24, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 65146 Threat (wago name; C++ SPELL_THREAT,
	// boss_razorscale.cpp:96 enum; Lua northrend/boss_razorscale.lua:1757,
	// DoCastSelf, latent, non-triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 2a7ab516, DifficultyID 0 row): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 2000 bp-1 die1 trigger
	// 65143 tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Physical; NO
	// SpellCategories row (no Category/DispelType/Mechanic);
	// SpellAuraOptions ProcChance 101 not carried; Attributes
	// not carried; no SpellClassOptions rows.
	{id: 65146, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, trigger: 65143, auraPeriod: 2000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 74403 Flame Breath (wago name; C++ SPELL_FLAME_BREATH,
	// boss_saviana_ragefire.cpp:39 enum; Lua
	// northrend/boss_saviana_ragefire.lua:85, DoCastVictim,
	// EVENT_FLAME_BREATH) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-16 browser read 2a7ab516,
	// DifficultyID 0 base rows): eff0 DUMMY bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY radius10 30yd; eff1
	// SCHOOL_DAMAGE bp24499 die7001 tgtA
	// TARGET_UNIT_CONE_ENEMY_24 radius10 30yd; cast5 2000ms,
	// dur0 (no SpellDuration row), range3 0-20yd, school Fire;
	// DifficultyID 2/4 variant rows exist and DIFFER (eff1
	// bp33686 die9626) — NOT seeded, base rows only per
	// precedent; SpellCategories Category 50, DefenseType
	// Magic, PreventionType 0x1, StartRecoveryCategory 133 not
	// carried; Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 74403, castTimeIdx: 5, durIdx: 0, rangeIdx: 3, school: schoolFire, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetEnemy, radiusIdx: 10, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 7001, basePoints: 24499, targetA: targetUnitConeEnemy, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 78722 Enrage (wago name; C++ SPELL_ENRAGE,
	// boss_saviana_ragefire.cpp:38 enum; Lua
	// northrend/boss_saviana_ragefire.lua:84, DoCastSelf,
	// EVENT_ENRAGE) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read 2a7ab516,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_MELEE_HASTE
	// bp149 die1 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_SCALE bp19 die1 tgtA TARGET_UNIT_CASTER; eff2
	// APPLY_AURA PERIODIC_TRIGGER_SPELL auraPeriod 2000 bp0
	// die0 trigger 78723 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur1 10000ms, range1 Self Only 0yd, school
	// Physical; SpellCategories DispelType 9 ENRAGE carried,
	// Category 0 / DefenseType 1 / PreventionType 0x1 not
	// carried; Attributes all 0x0; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 78722, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolPhysical, dispel: 9, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, dieSides: 1, basePoints: 149, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 19, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 0, basePoints: 0, trigger: 78723, auraPeriod: 2000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 56431 Arcane Bomb (wago SpellName, NOT "Arcane Bomb
	// Knockback Damage"; C++ SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE,
	// boss_malygos.cpp:125 enum; Lua northrend/npc_arcane_overload.lua:38,
	// DoCastAOE triggered) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-16 browser read df231e6d,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp4712 die575
	// tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius13 10yd; eff1 KNOCK_BACK bp149 die1 misc 100 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY radius13
	// 10yd; cast1 instant, dur0 (no SpellDuration row), range13
	// unlimited 50000yd, school Arcane; SpellCategories
	// Category 0, DefenseType 1, PreventionType 0x1 not carried;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 56431, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 575, basePoints: 4712, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 149, misc: 100, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 56432 Arcane Overload (wago SpellName, NOT "Arcane Overload
	// 1"; C++ SPELL_ARCANE_OVERLOAD_1, boss_malygos.cpp:126 enum;
	// Lua northrend/npc_arcane_overload.lua:39, DoCastSelf
	// triggered) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read df231e6d,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 1000 bp-1 die1 trigger
	// 56435 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 1000 bp-1 die1 trigger
	// 56438 tgtA TARGET_UNIT_CASTER; cast1 instant, dur22
	// 45000ms, range1 Self Only 0yd, school Physical as read
	// (NOT Arcane); NO SpellCategories row; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	// Trigger spells 56435/56438 are not themselves seeded.
	{id: 56432, castTimeIdx: 1, durIdx: 22, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, trigger: 56435, auraPeriod: 1000, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, trigger: 56438, auraPeriod: 1000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 58113 Threat Pulse (wago name; C++ SPELL_THREAT_PULSE,
	// zone_icecrown.cpp:328 enum; Lua northrend/npc_blessed_banner.lua:115,
	// DoCast in Reset) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read df231e6d,
	// DifficultyID 0 row): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 3000 bp-1 die1 trigger
	// 58114 tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Physical; NO
	// SpellCategories row; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows. Trigger spell
	// 58114 is not itself seeded.
	{id: 58113, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, trigger: 58114, auraPeriod: 3000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 58084 Crusader's Spire Victory (wago SpellName, NOT
	// "Crusaders' Spire Victory" — singular possessive; C++
	// SPELL_CRUSADERS_SPIRE_VICTORY, zone_icecrown.cpp:329 enum;
	// Lua northrend/npc_blessed_banner.lua:130, triggered self-cast
	// on Halof-death victory) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// df231e6d, DifficultyID 0 row): eff0 QUEST_COMPLETE bp0
	// die0 misc 13141 (quest id) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY radius12 100yd; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; NO SpellCategories row; Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 58084, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effQuestComplete, dieSides: 0, basePoints: 0, misc: 13141, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 28732 Widow's Embrace (wago SpellName; C++
	// SPELL_WIDOWS_EMBRACE, boss_faerlina.cpp:43 enum; Lua
	// northrend/npc_faerlina_add.lua:46, DoCast on Faerlina) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 7e54d10b, DifficultyID 0 rows):
	// eff0 APPLY_AURA MOD_CASTING_SPEED_NOT_STACK bp-26 die1
	// tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius18 15yd; eff1 SCRIPT_EFFECT bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur9 30000ms, range1
	// Self Only 0yd, school Arcane; NO SpellCategories row;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 28732, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCastingSpeed, dieSides: 1, basePoints: -26, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 18, chainAmplitude: 1},
		{effect: effScriptEffect, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 54095 Fireball (wago SpellName, NOT "Add Fireball"; C++
	// SPELL_ADD_FIREBALL, boss_faerlina.cpp:45 enum, 25-man:
	// 54096 per C++ comment; Lua northrend/npc_faerlina_add.lua:47,
	// DoCastVictim) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 7e54d10b,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp2024 die451
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast20 2500ms, dur0 (no
	// SpellDuration row), range36 Longer 0-45yd, school Fire;
	// EffectBonusCoefficient 1 not carried; SpellCategories
	// Category 0 DefenseType 1 PreventionType 0x1
	// StartRecoveryCategory 133 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 54095, castTimeIdx: 20, durIdx: 0, rangeIdx: 36, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 451, basePoints: 2024, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 46077 Shroud of the Death Cultist (wago SpellName; C++
	// SPELL_SHROUD_OF_THE_DEATH_CULTIST, zone_borean_tundra.cpp:1459
	// enum; Lua northrend/npc_hidden_cultist.lua:41) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 7e54d10b, DifficultyID 0 row):
	// eff0 APPLY_AURA DUMMY bp0 die0 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur21 -1 infinite, range1 Self Only 0yd,
	// school Physical; NO SpellCategories row; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 46077, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 46078 Righteous Vision (wago SpellName; C++
	// SPELL_RIGHTEOUS_VISION, zone_borean_tundra.cpp:1460 enum;
	// Lua northrend/npc_hidden_cultist.lua:42, player aura) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 7e54d10b, DifficultyID 0 row):
	// eff0 APPLY_AURA DETECT_AMORE bp0 die0 misc 3 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur9 30000ms, range1
	// Self Only 0yd, school Physical; NO SpellCategories row;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows. NEW aura const auraDetectAmore=170
	// (SpellAuraDefines.h:250).
	{id: 46078, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDetectAmore, dieSides: 0, basePoints: 0, misc: 3, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 3616 Poison Proc (wago SpellName; C++ SPELL_POISON_PROC,
	// razorfen_downs.cpp:302 enum; Lua
	// kalimdor/npc_tomb_creature.lua:46, Tomb Fiend self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 7e623dca, DifficultyID 0 row):
	// eff0 APPLY_AURA PROC_TRIGGER_SPELL bp0 die1 trigger 744
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Physical; NO
	// SpellCategories row; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows. Trigger spell
	// 744 is not itself seeded.
	{id: 3616, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: 0, trigger: 744, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12254 Virulent Poison Proc (wago SpellName, NOT "Virulent
	// Poison" — the boss_tuten_kash.cpp:26 alias
	// SPELL_VIRULENT_POISON does not change the DB2 name; C++
	// SPELL_VIRULENT_POISON_PROC, razorfen_downs.cpp:303 enum;
	// Lua kalimdor/npc_tomb_creature.lua:47, Tomb Reaver
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 7e623dca,
	// DifficultyID 0 row): eff0 APPLY_AURA PROC_TRIGGER_SPELL
	// bp0 die1 trigger 12251 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1 infinite, range1 Self Only 0yd, school
	// Physical; same template as 3616, trigger differs; NO
	// SpellCategories row; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows. Trigger spell
	// 12251 is not itself seeded.
	{id: 12254, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: 0, trigger: 12251, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 24646 Avatar (wago SpellName; C++ SPELL_AVATAR,
	// boss_grilek.cpp:24 enum; Lua zulgurub/boss_grilek.lua:31,
	// non-triggered DoCast self) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 7e623dca, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DAMAGE_PERCENT_DONE bp499 die1 misc 1 tgtA
	// TARGET_UNIT_CASTER (+500%); eff1 APPLY_AURA
	// MOD_DECREASE_SPEED bp-76 die1 tgtA TARGET_UNIT_CASTER
	// (-75%); eff2 APPLY_AURA MOD_SCALE bp199 die1 tgtA
	// TARGET_UNIT_CASTER (200%); cast1 instant, dur8 15000ms,
	// range1 Self Only 0yd, school Physical; EffectBonusCoefficient
	// 1 on all three effects NOT carried (seed has no field —
	// established precedent); NO SpellCategories row;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 24646, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, dieSides: 1, basePoints: 499, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -76, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 199, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 6524 Ground Tremor (wago SpellName; C++
	// SPELL_GROUND_TREMOR, boss_grilek.cpp:25 enum; Lua
	// zulgurub/boss_grilek.lua:32, triggered DoCastVictim; also
	// cast by boss_archaedas, same row) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 7e623dca, DifficultyID 0 row): eff0 APPLY_AURA MOD_STUN
	// bp0 die1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius9 20yd; cast1 instant,
	// dur39 2000ms, range1 Self Only 0yd, school Physical;
	// SpellCategories Mechanic 12 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 6524, castTimeIdx: 1, durIdx: 39, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 1, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 11831 Frost Nova (wago SpellName; C++ SPELL_FROSTNOVA,
	// zone_hellfire_peninsula.cpp:830 enum; Lua
	// outland/npc_magister_aledis.lua:58, self-cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 8da29dbe, DifficultyID 0 rows):
	// eff0 SCHOOL_DAMAGE bp27 die5 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY, RadiusIndex_0 0 /
	// RadiusIndex_1 13 — the _1 radius is NOT seeded: the
	// engine reads RadiusIndex_0 only (store.go:1172, field
	// 92+i), 29961 precedent; eff1 APPLY_AURA MOD_ROOT bp-1
	// die1 mechanic 7 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius13 10yd; cast1 instant,
	// dur31 8000ms, range1 Self Only 0yd, school Frost;
	// EffectBonusCoefficient 1 on eff0 NOT carried (seed has
	// no field — established precedent); SpellCategories
	// DefenseType 1 DispelType 1 Mechanic 7 PreventionType 0x1
	// not carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 11831, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 5, basePoints: 27, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 1, basePoints: -1, mechanic: 7, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 20823 Fireball (wago SpellName; C++ SPELL_FIREBALL,
	// zone_hellfire_peninsula.cpp:829 enum; Lua
	// outland/npc_magister_aledis.lua:57, victim-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 8da29dbe, DifficultyID 0 row):
	// eff0 SCHOOL_DAMAGE bp89 die31 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast14 3000ms (established
	// row 14), dur0 (no SpellDuration row), range5 Long 0-40yd,
	// school Fire; EffectBonusCoefficient 1 NOT carried;
	// SpellCategories DefenseType 1 PreventionType 0x1
	// StartRecoveryCategory 133 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 20823, castTimeIdx: 14, durIdx: 0, rangeIdx: 5, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 31, basePoints: 89, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 12548 Frost Shock (wago SpellName; C++
	// SPELL_FROST_SHOCK, zone_nagrand.cpp:55 enum (also
	// SPELL_KUR_FROST_SHOCK, zone_nagrand.cpp:243, same id);
	// Lua outland/npc_nagrand_captive.lua:118, DoCastVictim)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 8da29dbe, DifficultyID 0 rows):
	// eff0 APPLY_AURA MOD_DECREASE_SPEED bp-51 die1 mechanic
	// 11 tgtA TARGET_UNIT_TARGET_ENEMY; eff1 SCHOOL_DAMAGE
	// bp88 die7 tgtA TARGET_UNIT_TARGET_ENEMY (verdict order:
	// aura first, damage second); cast1 instant, dur31 8000ms,
	// range3 Twenty 0-20yd, school Frost; EffectBonusCoefficient
	// 1 on eff1 NOT carried; SpellCategories DefenseType 1
	// DispelType 1 PreventionType 0x1 not carried; Attributes
	// not carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 12548, castTimeIdx: 1, durIdx: 31, rangeIdx: 3, school: schoolFrost, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -51, mechanic: 11, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 7, basePoints: 88, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16006 Chain Lightning (wago SpellName; C++
	// SPELL_CHAIN_LIGHTNING, zone_nagrand.cpp:53 enum (also
	// SPELL_KUR_CHAIN_LIGHTNING, zone_nagrand.cpp:241, same
	// id); Lua outland/npc_nagrand_captive.lua:116,
	// DoCastVictim) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 8da29dbe,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp184 die31
	// chainTargets 3 tgtA TARGET_UNIT_TARGET_ENEMY; cast14
	// 3000ms, dur0 (no SpellDuration row), range4 Medium
	// 0-30yd, school Nature; EffectBonusCoefficient 1 NOT
	// carried; SpellCategories Category 85 DefenseType 1
	// PreventionType 0x1 StartRecoveryCategory 133 not
	// carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 16006, castTimeIdx: 14, durIdx: 0, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 31, basePoints: 184, targetA: targetEnemy, chainTargets: 3, chainAmplitude: 1},
	}},
	// 12553 Shock (wago SpellName; C++ SPELL_SHOCK,
	// zone_hellfire_peninsula.cpp:52 enum; Lua
	// outland/npc_aeranas.lua:47, victim-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// browser read a1bb46e3, DifficultyID 0 row): eff0
	// SCHOOL_DAMAGE bp108 die9 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur0 (no SpellDuration row), range3
	// Twenty 0-20yd, school Nature; EffectBonusCoefficient 1,
	// RealPointsPerLevel 1, Coefficient 1 NOT carried (seed
	// has no fields — established precedent); SpellCategories
	// Category 19 DefenseType 1 PreventionType 0x1
	// StartRecoveryCategory 133 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 12553, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 9, basePoints: 108, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 15535 Enveloping Winds (wago SpellName, Rank 1; C++
	// SPELL_ENVELOPING_WINDS, zone_hellfire_peninsula.cpp:51
	// enum; Lua outland/npc_aeranas.lua:48, victim-cast) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read a1bb46e3, DifficultyID 0 row):
	// eff0 APPLY_AURA MOD_STUN bp-1 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast5 2000ms, dur1 10000ms,
	// range4 Medium 0-30yd, school Nature; SpellCategories
	// Category 99 DefenseType 1 DispelType 1 Mechanic 12
	// PreventionType 0x1 not carried; Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 15535, castTimeIdx: 5, durIdx: 1, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 1, basePoints: -1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 13376 Fire Shield (wago SpellName; C++
	// SPELL_SHAMAN_FIRESHIELD, pet_shaman.cpp:31 enum (also
	// SPELL_FIRE_SHIELD, boss_pyroguard_emberseer.cpp:40,
	// same id); Lua
	// world/npc_pet_shaman_fire_elemental.lua:43) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read a1bb46e3, DifficultyID 0 row):
	// eff0 SCHOOL_DAMAGE bp0 die1 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius8 5yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Fire; EffectBonusCoefficient 0.032,
	// RealPointsPerLevel 1.2 NOT carried (seed has no fields
	// — established precedent); SpellCategories DefenseType 1
	// PreventionType 0x1 not carried; Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 13376, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 57984 Fire Blast (wago SpellName; C++
	// SPELL_SHAMAN_FIREBLAST, pet_shaman.cpp:29 enum; Lua
	// world/npc_pet_shaman_fire_elemental.lua:42) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read a1bb46e3, DifficultyID 0 row):
	// eff0 SCHOOL_DAMAGE bp109 die21 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0 (no
	// SpellDuration row), range3 Twenty 0-20yd, school Fire;
	// EffectBonusCoefficient 0.429 NOT carried; SpellCategories
	// Category 19 DefenseType 1 PreventionType 0x1
	// StartRecoveryCategory 0 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 57984, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 21, basePoints: 109, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 18692 Improved Healthstone (wago SpellName, Rank 1; C++
	// SPELL_IMPROVED_HEALTH_STONE_R1, go_scripts.cpp:483 enum
	// (also SPELL_WARLOCK_IMPROVED_HEALTHSTONE_R1,
	// spell_warlock.cpp:57); Lua world/go_scripts_part1.lua:322,
	// HasAura gate) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read c062380b,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp9 die1 tgtA
	// TARGET_UNIT_CASTER (9+1 = 10% per description); cast1
	// instant, dur21 -1 infinite, range1 Self Only 0yd, school
	// Physical; EffectBonusCoefficient 1 NOT carried; NO
	// SpellCategories row; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	// SkillLineAbility 10577 (SkillLine 354) not carried.
	{id: 18692, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: 9, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 34150 Create Master Healthstone (wago SpellName; C++
	// SPELL_CREATE_MASTER_HEALTH_STONE_R2, go_scripts.cpp:488
	// enum; Lua world/go_scripts_part1.lua:300-301,308,
	// HasAura gate only — the row itself is CREATE_ITEM) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read c062380b, DifficultyID 0 row):
	// eff0 CREATE_ITEM bp0 die1 itemType 22105 tgtA
	// TARGET_UNIT_TARGET_ALLY; cast1 instant, dur0 (no
	// SpellDuration row), range12 Interact 0-5yd, school
	// Shadow; SpellCategories DefenseType 1 PreventionType
	// 0x0 not carried; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 34150, castTimeIdx: 1, durIdx: 0, rangeIdx: 12, school: schoolShadow, effects: [3]effect{
		{effect: effCreateItem, dieSides: 1, basePoints: 0, targetA: targetUnitTargetAlly, itemType: 22105, chainAmplitude: 1},
	}},
	// 22893 Arcane Blast (wago SpellName, NOT "Knock Away" —
	// the C++ SPELL_KNOCKAWAY const, the_eye.cpp:36, is named
	// for the knockback effect, not the DB2 name; C++
	// SPELL_KNOCKAWAY, the_eye.cpp:36 enum; Lua
	// the_eye/npc_crystalcore_devastator.lua:40, triggered
	// DoCastVictim) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-16 browser read c062380b,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp99 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK bp99 die1
	// misc 350 tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range6 Vision 0-100yd,
	// school Arcane; EffectBonusCoefficient 1 on both effects
	// NOT carried; SpellCategories DefenseType 0
	// PreventionType 0x1 not carried; Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 22893, castTimeIdx: 1, durIdx: 0, rangeIdx: 6, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 99, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 99, misc: 350, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 35035 Countercharge (wago SpellName; C++
	// SPELL_COUNTERCHARGE, the_eye.cpp:35 enum; Lua
	// the_eye/npc_crystalcore_devastator.lua:39, non-triggered
	// DoCast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read c062380b,
	// DifficultyID 0 row): eff0 APPLY_AURA
	// PROC_TRIGGER_SPELL bp-1 die1 trigger 35781 tgtA
	// TARGET_UNIT_CASTER; cast16 1500ms, dur8 15000ms, range1
	// Self Only 0yd, school Arcane; SpellCategories Category
	// 19 DefenseType 1 PreventionType 0x1
	// StartRecoveryCategory 133 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	// Trigger spell 35781 is not itself seeded.
	{id: 35035, castTimeIdx: 16, durIdx: 8, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: -1, trigger: 35781, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 24664 Sleep (wago SpellName; C++ SPELL_SLEEP,
	// boss_hazzarah.cpp:26 enum; Lua
	// zulgurub/boss_hazzarah.lua:34, triggered DoCastVictim)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read f28d47f3, DifficultyID 0 row):
	// eff0 APPLY_AURA MOD_STUN bp0 die0 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius12 100yd; cast1 instant, dur32 6000ms, range13
	// unlimited 50000yd, school Shadow; SpellCategories
	// DefenseType 0 PreventionType 0x0 StartRecoveryCategory
	// 133 not carried; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 24664, castTimeIdx: 1, durIdx: 32, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 26046 Mana Burn (wago SpellName; C++ SPELL_MANABURN,
	// boss_hazzarah.cpp:25 enum; Lua
	// zulgurub/boss_hazzarah.lua:33, triggered DoCastVictim)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read f28d47f3, DifficultyID 0 row):
	// eff0 POWER_BURN bp924 die151 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius10 30yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Shadow; EffectAmplitude 1 and
	// EffectBonusCoefficient 0.1 NOT carried (seed has no
	// fields — established precedent); SpellCategories
	// DefenseType 1 PreventionType 0x1 not carried;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 26046, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effPowerBurn, dieSides: 151, basePoints: 924, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 24819 Lightning Wave (wago SpellName; C++
	// SPELL_LIGHTNINGWAVE, boss_wushoolay.cpp:25 enum; Lua
	// zulgurub/boss_wushoolay.lua:33) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// f28d47f3, DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp462
	// die75 chainTargets 10 chainAmplitude 1.2 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0 (no
	// SpellDuration row), range6 Vision 0-100yd, school
	// Nature; EffectBonusCoefficient 1 NOT carried;
	// SpellCategories Category 85 DefenseType 1
	// PreventionType 0x1 StartRecoveryCategory 133 not
	// carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 24819, castTimeIdx: 1, durIdx: 0, rangeIdx: 6, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 75, basePoints: 462, targetA: targetEnemy, chainTargets: 10, chainAmplitude: 1.2},
	}},
	// 25033 Lightning Cloud (wago SpellName; C++
	// SPELL_LIGHTNINGCLOUD, boss_wushoolay.cpp:24 enum; Lua
	// zulgurub/boss_wushoolay.lua:32, triggered DoCastVictim)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read f28d47f3, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1574 die451 tgtA
	// TARGET_UNIT_DEST_AREA_ENEMY radius13 10yd; eff1
	// PERSISTENT_AREA_AURA PERIODIC_DAMAGE auraPeriod 2000
	// bp1837 die525 tgtA TARGET_DEST_DYNOBJ_ENEMY radius13
	// 10yd; cast1 instant, dur305 14000ms, range4 Medium
	// 0-30yd, school Nature; EffectBonusCoefficient 1 NOT
	// carried; SpellCategories Category 35 DefenseType 1
	// PreventionType 0x1 not carried; Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows. NEW aux
	// row SpellDuration 305 = 14000ms added, verified in
	// SpellDuration.dbc.
	{id: 25033, castTimeIdx: 1, durIdx: 305, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 451, basePoints: 1574, targetA: targetUnitDestAreaEnemy, radiusIdx: 13, chainAmplitude: 1},
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, dieSides: 525, basePoints: 1837, auraPeriod: 2000, targetA: targetDestDynobjEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 28630 "Draenei Survivor Stunned" (wago SpellName —
	// MISMATCH vs expected "Stunned"; C++ SPELL_STUNNED,
	// zone_azuremyst_isle.cpp:54 enum; Lua
	// kalimdor/npc_draenei_survivor.lua:56, DoCast(me, 28630,
	// true) + pCaster latch) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 45c90da6, DifficultyID 0 row): eff0 APPLY_AURA DUMMY
	// bp0 die0 tgtA TARGET_UNIT_CASTER; cast1 instant, dur21
	// -1 infinite, range2 Combat 0-5yd, school Physical;
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 28630, castTimeIdx: 1, durIdx: 21, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 35046 "Quest - Draenei Survivor Irradiation" (wago
	// SpellName — MISMATCH vs expected "Irridation"
	// (spelling + "Quest - Draenei Survivor " prefix); C++
	// SPELL_IRRIDATION, zone_azuremyst_isle.cpp:53 enum; Lua
	// kalimdor/npc_draenei_survivor.lua:55, DoCast(me, 35046,
	// true)) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 45c90da6,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp-1 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Physical;
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 35046, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 31673 Foul Spores (wago SpellName; C++
	// SPELL_FOUL_SPORES, boss_hungarfen.cpp:31 enum; Lua
	// theunderbog/boss_hungarfen.lua:62, once-guarded when
	// GetHealthPct() <= 20) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 45c90da6, DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 1000 trigger 31697
	// tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA MOD_ROOT tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AREA_AURA_PARTY DUMMY
	// tgtA TARGET_DEST_CASTER radius9 20yd; cast1 instant,
	// dur38 11000ms, range137 Eight yards 0-8yd, school
	// Shadow (SchoolMask 0x20 as read — note the DB2
	// Description text says Nature damage); eff0
	// SpellClassMask_0 0x4 and EffectBonusCoefficient 1 NOT
	// carried (seed has no fields — established precedent);
	// SpellCategories Category 0 DefenseType 1 DispelType 1
	// PreventionType 0x1 StartRecoveryCategory 133 not
	// carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows. NEW aux row SpellDuration 38
	// = 11000ms added, verified in SpellDuration.dbc.
	{id: 31673, castTimeIdx: 1, durIdx: 38, rangeIdx: 137, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 0, basePoints: 0, auraPeriod: 1000, trigger: 31697, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAreaAuraParty, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetDestCaster, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 38739 Acid Geyser (wago SpellName; C++
	// SPELL_ACID_GEYSER, boss_hungarfen.cpp:32 enum; Lua
	// theunderbog/boss_hungarfen.lua:63, DoCast(target,
	// 38739)) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 45c90da6,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp2187 die625
	// tgtA TARGET_UNIT_TARGET_ENEMY tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY radius29 6yd; cast1
	// instant, dur0 (no SpellDuration row), range35
	// Medium-Long 0-35yd, school Nature; SpellCategories
	// absent ("No entries found."); Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows. NEW aux
	// row SpellRange 35 = 0-35yd added, verified in
	// SpellRange.dbc.
	{id: 38739, castTimeIdx: 1, durIdx: 0, rangeIdx: 35, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 625, basePoints: 2187, targetA: targetEnemy, targetB: targetUnitDestAreaEnemy, radiusIdx: 29, chainAmplitude: 1},
	}},
	// 32325 Void Blast (wago SpellName; C++ SPELL_VOID_BLAST,
	// ManaTombs/boss_pandemonius.cpp:32 enum; Lua
	// outland/boss_pandemonius.lua:74, DoCast(target, 32325))
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 74701ec4, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1662 die475 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK bp74 die1
	// misc 200 tgtA TARGET_UNIT_TARGET_ENEMY; cast3 500ms,
	// dur0 (no SpellDuration row), range6 Vision 0-100yd,
	// school Shadow; Speed 30 NOT carried (seed has no
	// field — established precedent); SpellCategories
	// Category 0 DefenseType 1 PreventionType 0x1 not
	// carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 32325, castTimeIdx: 3, durIdx: 0, rangeIdx: 6, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 475, basePoints: 1662, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 74, misc: 200, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 32358 Dark Shell (wago SpellName; C++
	// SPELL_DARK_SHELL, ManaTombs/boss_pandemonius.cpp:33
	// enum; Lua outland/boss_pandemonius.lua:75, self-cast)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read 74701ec4, DifficultyID 0
	// rows ONLY — a DifficultyID 173 variant exists
	// (cast1 instant, dur31 8000ms, dmg-shield bp750 die0,
	// reflect bp100 die0) and is NOT seeded per the
	// base-rows-only precedent): eff0 APPLY_AURA
	// DAMAGE_SHIELD bp749 die1 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA REFLECT_SPELLS bp99 die1 tgtA
	// TARGET_UNIT_CASTER; cast4 1000ms, dur32 6000ms, range1
	// Self Only 0yd, school Shadow; EffectAmplitude 1 NOT
	// carried (seed has no field); SpellCategories absent
	// ("No entries found."); Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 32358, castTimeIdx: 4, durIdx: 32, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDamageShield, dieSides: 1, basePoints: 749, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraReflectSpells, dieSides: 1, basePoints: 99, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 36643 "Shattrath Banish Teleport" (wago SpellName —
	// MISMATCH vs expected "Banish Teleport"; C++
	// SPELL_BANISH_TELEPORT, World/npc_guard.cpp:37 enum;
	// Lua world/npc_guard.lua:144, self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 74701ec4, DifficultyID 0 rows): eff0
	// TELEPORT_UNITS bp0 die0 tgtA TARGET_UNIT_CASTER tgtB
	// TARGET_DEST_DB; eff1 SANCTUARY bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur0 (no
	// SpellDuration row), range12 0-5yd, school Physical;
	// EffectPos_facing 2.189... and EffectAmplitude 1 NOT
	// carried (seed has no fields); EffectBonusCoefficient 1
	// NOT carried; SpellCategories Category 0 DefenseType 1
	// PreventionType 0x1 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 36643, castTimeIdx: 1, durIdx: 0, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, dieSides: 0, basePoints: 0, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
		{effect: effSanctuary, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 39533 Exile (wago SpellName; C++ SPELL_EXILE,
	// World/npc_guard.cpp:38 enum; Lua
	// world/npc_guard.lua:145, self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 74701ec4, DifficultyID 0 rows): eff0
	// APPLY_AURA MOD_DECREASE_SPEED bp-21 die1 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA MOD_PERCENT_STAT
	// bp-21 die1 misc -1 (AllStats) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur25 30000ms
	// (SpellDuration row 25 re-read this batch — see the
	// corrected aux row), range12 0-5yd, school Physical;
	// raw EffectBasePoints -21 seeded as-is (the -20% aura
	// text follows the usual +1 adjustment); SpellCategories
	// Category 0 DefenseType 1 PreventionType 0x1 not
	// carried; Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 39533, castTimeIdx: 1, durIdx: 25, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -21, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModPercentStat, dieSides: 1, basePoints: -21, misc: -1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 38467 "Netherbreath" (wago SpellName — MISMATCH vs
	// expected "Nether Breath" (no space); C++
	// SPELL_NETHER_BREATH, zone_shadowmoon_valley.cpp:203
	// enum; Lua outland/npc_mature_netherwing_drake.lua:41,
	// DoCastVictim every 5s) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// abab7d59, DifficultyID 0 rows): eff0 TRIGGER_SPELL
	// trigger 38468 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 1000 trigger 38468
	// tgtA TARGET_UNIT_CASTER; damage itself is in
	// triggered spell 38468, NOT seeded (trigger spells not
	// seeded — established precedent; the trigger FIELD is
	// carried); cast20 2500ms, dur27 3000ms, range2 Combat
	// 0-5yd, school Arcane; SpellCategories Category 50
	// DefenseType 1 PreventionType 0x1 StartRecoveryCategory
	// 133 not carried; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 38467, castTimeIdx: 20, durIdx: 27, rangeIdx: 2, school: schoolArcane, effects: [3]effect{
		{effect: effTriggerSpell, dieSides: 0, basePoints: 0, trigger: 38468, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 0, basePoints: 0, auraPeriod: 1000, trigger: 38468, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 38502 "Has Eaten Recently" (wago SpellName — MISMATCH
	// vs expected "Just Eaten"; C++ SPELL_JUST_EATEN,
	// zone_shadowmoon_valley.cpp:202 enum; Lua
	// outland/npc_mature_netherwing_drake.lua:40,
	// DoCastSelf 38502 + Talk 0) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// abab7d59, DifficultyID 0 row): eff0 APPLY_AURA DUMMY
	// bp0 die0 tgtA TARGET_UNIT_CASTER; cast1 instant, dur3
	// 60000ms, range1 Self Only 0yd, school Physical;
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 38502, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 40415 "Fixated" (wago SpellName — MISMATCH vs expected
	// "Fixate Trigger"; C++ SPELL_FIXATE_TRIGGER,
	// utgarde_keep.cpp:159 enum; Lua
	// northrend/utgarde_keep.lua:36, triggered cast on the
	// caster) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read abab7d59,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_TAUNT bp0
	// die0 tgtA TARGET_UNIT_TARGET_ANY; eff1 APPLY_AURA
	// MECHANIC_IMMUNITY_MASK bp0 die0 misc 1694 tgtA
	// TARGET_UNIT_TARGET_ANY; cast1 instant, dur8 15000ms,
	// range13 unlimited 50000yd, school Physical;
	// SpellCategories Category 82 DefenseType 1
	// PreventionType 0x1 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 40415, castTimeIdx: 1, durIdx: 8, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModTaunt, dieSides: 0, basePoints: 0, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunityMask, dieSides: 0, basePoints: 0, misc: 1694, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 59687 "Explosion" (wago SpellName — MISMATCH vs
	// expected "Ticking Time Bomb Explode"; C++
	// SPELL_TICKING_TIME_BOMB_EXPLODE, utgarde_keep.cpp:122
	// enum; Lua northrend/utgarde_keep.lua:34, target casts
	// 59687 on the caster) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// abab7d59, DifficultyID 0 row): eff0 SCHOOL_DAMAGE
	// bp7399 die1201 tgtA TARGET_DEST_CASTER tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY radius26 4yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Fire; raw bp7399/die1201 seeded as-is
	// (the 7400-8600 $s1 range follows the usual +1
	// convention); SpellCategories Category 0 DefenseType 1
	// PreventionType 0x1 not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	// NEW aux row SpellRadius 26 = 4yd added, verified in
	// SpellRadius.dbc.
	{id: 59687, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1201, basePoints: 7399, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 26, chainAmplitude: 1},
	}},
	// 45702 Neural Needle Impact (wago SpellName; C++
	// SPELL_NEURAL_NEEDLE_IMPACT,
	// zone_borean_tundra.cpp:1177 enum; Lua
	// northrend/npc_imprisoned_beryl_sorcerer.lua:60,
	// DoCastSelf 45702) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read da0ce2f3,
	// DifficultyID 0 row): eff0 DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER radius18 15yd; cast1 instant, dur0
	// (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 45702, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 48254 Prototype Neural Needle Impact (wago SpellName;
	// C++ SPELL_PROTOTYPE_NEURAL_NEEDLE_IMPACT,
	// zone_borean_tundra.cpp:1178 enum; Lua
	// northrend/npc_imprisoned_beryl_sorcerer.lua:61,
	// DoCastSelf 48254) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read da0ce2f3,
	// DifficultyID 0 row): eff0 DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER radius18 15yd; cast1 instant, dur0
	// (no SpellDuration row), range1 Self Only 0yd, school
	// Physical; identical to 45702's effect except the
	// Effect row ID (Attributes_2 0x0 vs 0x4, not carried);
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 48254, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 47044 "Cosmetic - Confused State Visual (Big)" (wago
	// SpellName — MISMATCH vs expected "Confused"; no spell
	// named exactly "Confused" exists in this build/locale;
	// C++ SPELL_CONFUSED, zone_dragonblight.cpp:381 enum;
	// Lua northrend/npc_torturer_lecraft.lua:108,
	// RemoveAura(47044)) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read da0ce2f3,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp0 die0
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Physical;
	// EffectBonusCoefficient 1 NOT carried (seed has no
	// field — established precedent); SpellCategories absent
	// ("No entries found."); Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 47044, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 47550 "Create Bark of the Walkers" (wago SpellName —
	// MISMATCH vs expected "Create Item Bark"; no such spell
	// exists in this build/locale; C++
	// SPELL_CREATE_ITEM_BARK, zone_dragonblight.cpp:380
	// enum; Lua northrend/npc_torturer_lecraft.lua:107,
	// tree casts 47550 on the player) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// da0ce2f3, DifficultyID 0 row): eff0 CREATE_ITEM bp0
	// die0 itemType 36786 tgtA TARGET_UNIT_TARGET_ANY; cast1
	// instant, dur0 (no SpellDuration row), range12
	// Interact 0-5yd, school Physical; SpellCategories
	// absent ("No entries found."); Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 47550, castTimeIdx: 1, durIdx: 0, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effCreateItem, dieSides: 0, basePoints: 0, itemType: 36786, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 47533 "Camera Shake - Small" (wago SpellName — MISMATCH
	// vs expected "Camera Shake"; C++ SPELL_CAMERA_SHAKE,
	// zone_grizzly_hills.cpp:269 enum; Lua
	// northrend/npc_outhouse_bunny.lua:31, DoCast(caster,
	// 47533, TRIGGERED)) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read 9871db28,
	// DifficultyID 0 row): eff0 DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur0 (no
	// SpellDuration row), range1 Self Only 0yd, school
	// Arcane; SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 47533, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 48329 Dust Field (wago SpellName; C++
	// SPELL_DUST_FIELD, zone_grizzly_hills.cpp:270 enum; Lua
	// northrend/npc_outhouse_bunny.lua:32, DoCast(me, 48329,
	// TRIGGERED)) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 9871db28,
	// DifficultyID 0 row): eff0 KNOCK_BACK bp0 die0 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius18 15yd; cast1 instant, dur0 (no SpellDuration
	// row), range1 Self Only 0yd, school NONE (SchoolMask
	// 0x0 as read); SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 48329, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolNone, effects: [3]effect{
		{effect: effKnockBack, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 51012 Dark Matter (wago SpellName; C++
	// SPELL_DARK_MATTER, halls_of_stone.cpp:109 enum; Lua
	// northrend/npc_tribuna_controller.lua:48, summon casts
	// 51012 triggered on a random player) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9871db28, DifficultyID 0 rows): eff0
	// APPLY_AURA MOD_DAMAGE_PERCENT_TAKEN bp49 die1 misc
	// 127 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius8 5yd; eff1 APPLY_AURA
	// MOD_DECREASE_SPEED bp-31 die1 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius8 5yd; eff2
	// SCHOOL_DAMAGE bp1884 die231 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius8 5yd; cast1
	// instant, dur18 20000ms, range1 Self Only 0yd, school
	// Shadow; SpellCategories Category 0 DefenseType 0
	// DispelType 1 PreventionType 0x0 not carried;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 51012, castTimeIdx: 1, durIdx: 18, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgTaken, dieSides: 1, basePoints: 49, misc: 127, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -31, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 231, basePoints: 1884, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 8, chainAmplitude: 1},
	}},
	// 51136 Searing Gaze (wago SpellName; C++
	// SPELL_SEARING_GAZE, halls_of_stone.cpp:112 enum; Lua
	// northrend/npc_tribuna_controller.lua:49, triggered
	// 51136) — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 browser read 9871db28,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY bp-1 die1
	// tgtA TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius28 50000yd; eff1 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 500 trigger 51125
	// tgtA TARGET_UNIT_CASTER; trigger spell 51125 NOT
	// seeded (trigger spells not seeded — established
	// precedent; the trigger FIELD is carried); cast1
	// instant, dur21 -1 infinite, range13 unlimited 50000yd,
	// school Fire; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 51136, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 28, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 0, basePoints: 0, auraPeriod: 500, trigger: 51125, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 54643 "Teleport" (wago SpellName — MISMATCH vs
	// expected "Wintergrasp Teleport Trigger"; C++
	// SPELL_WINTERGRASP_TELEPORT_TRIGGER,
	// zone_wintergrasp.cpp:493 enum; Lua
	// spells/spell_wintergrasp.lua:5, teleport-trigger aura)
	// — wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 browser read c5349039, DifficultyID 0
	// rows): eff0 DUMMY bp0 die0 tgtA
	// TARGET_DEST_NEARBY_ENTRY; eff1 TELEPORT_UNITS bp0 die1
	// tgtA TARGET_UNIT_CASTER tgtB TARGET_DEST_DEST; eff2
	// APPLY_AURA DUMMY bp0 die0 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur18 20000ms, range13 unlimited
	// 50000yd, school Physical; SpellCategories absent ("No
	// entries found."); Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 54643, castTimeIdx: 1, durIdx: 18, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetDestNearbyEntry, chainAmplitude: 1},
		{effect: effTeleportUnits, dieSides: 1, basePoints: 0, targetA: targetCaster, targetB: targetDestDest, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDummy, dieSides: 0, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 60968 "Ride Wintergrasp Vehicle" (wago SpellName —
	// MISMATCH vs expected "Ride WG Vehicle"; C++
	// SPELL_RIDE_WG_VEHICLE, zone_wintergrasp.cpp:61 enum;
	// Lua spells/spell_wintergrasp.lua:40, Vehicle (60968)
	// on the caster) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-16 browser read c5349039,
	// DifficultyID 0 rows): eff0 APPLY_AURA CONTROL_VEHICLE
	// bp0 die0 tgtA TARGET_UNIT_TARGET_ANY; eff1 APPLY_AURA
	// LINKED bp0 die0 trigger 65635 tgtA TARGET_UNIT_CASTER
	// tgtB TARGET_UNIT_TARGET_ANY; trigger spell 65635 NOT
	// seeded (trigger spells not seeded — established
	// precedent; the trigger FIELD is carried); cast1
	// instant, dur21 -1 infinite, range152 Super Long
	// 0-150yd, school Arcane; SpellCategories Category 0
	// DefenseType 0 PreventionType 0x0 StartRecoveryCategory
	// 133 not carried; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 60968, castTimeIdx: 1, durIdx: 21, rangeIdx: 152, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraControlVehicle, dieSides: 0, basePoints: 0, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraLinked, dieSides: 0, basePoints: 0, trigger: 65635, targetA: targetCaster, targetB: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 57989 Shadowfiend Death (wago SpellName; C++
	// SPELL_PRIEST_SHADOWFIEND_DEATH, pet_priest.cpp:31
	// enum; Lua world/npc_pet_pri_shadowfiend.lua:41,
	// DoCastAOE(57989)) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read c5349039,
	// DifficultyID 0 row): eff0 APPLY_AURA DUMMY bp-1 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur0 (no
	// SpellDuration row), range1 Self Only 0yd, school
	// Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 57989, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: -1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 58228 Glyph of Shadowfiend (wago SpellName; C++
	// SPELL_PRIEST_GLYPH_OF_SHADOWFIEND, pet_priest.cpp:30
	// enum; Lua world/npc_pet_pri_shadowfiend.lua:40,
	// glyph-aura summoner) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// c5349039, DifficultyID 0 row): eff0 APPLY_AURA DUMMY
	// bp4 die1 misc 5 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Physical; SpellClassMask_1 67108864 NOT
	// carried (seed has no field — established precedent);
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 58228, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, basePoints: 4, misc: 5, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 63041 "Rocket Strike" (wago SpellName — MISMATCH vs
	// expected "Rocket Strike Damage"; C++
	// SPELL_ROCKET_STRIKE_DAMAGE, boss_mimiron.cpp:2278
	// comment; Lua northrend/npc_mimiron_rocket_visual.lua:23,
	// spawn handler casts 63041) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser read
	// 8bd58485, DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp4999999 die1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius15 3yd; eff1/2
	// SCHOOL_DAMAGE bp4712499 die575001 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius15 3yd (raw bp/die seeded as-is); cast1
	// instant, dur0 (no SpellDuration row), range13
	// unlimited 50000yd, school Fire; SpellCategories
	// absent ("No entries found."); Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 63041, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 4999999, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 15, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 575001, basePoints: 4712499, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 15, chainAmplitude: 1},
		{effect: effSchoolDamage, dieSides: 575001, basePoints: 4712499, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 15, chainAmplitude: 1},
	}},
	// 65040 "Not-So-Friendly Fire" (wago SpellName —
	// MISMATCH vs expected "Not So Friendly Fire"
	// (hyphenation only); C++ SPELL_NOT_SO_FRIENDLY_FIRE,
	// boss_mimiron.cpp:170 enum; Lua
	// northrend/npc_mimiron_rocket_visual.lua:24, triggered
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 8bd58485,
	// DifficultyID 0 row): eff0 DUMMY bp0 die0 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius28 50000yd; cast1 instant, dur0 (no
	// SpellDuration row), range13 unlimited 50000yd, school
	// Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 65040, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 28, chainAmplitude: 1},
	}},
	// 63387 "Rapid Burst" (wago SpellName — MISMATCH vs
	// expected "Rapid Burst Left"; C++
	// SPELL_RAPID_BURST_LEFT, boss_mimiron.cpp:117 enum; Lua
	// northrend/spell_mimiron_rapid_burst.lua:22-23, periodic
	// aura casts 63387/64019 alternating by tick parity) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 8bd58485, DifficultyID 0
	// row): eff0 SCHOOL_DAMAGE bp1413 die173 tgtA
	// TARGET_UNIT_CONE_CASTER_TO_DEST_ENEMY radius12 100yd;
	// EffectAttributes 0x20000 NOT carried (seed effect
	// struct has no attributes field); cast1 instant, dur0
	// (no SpellDuration row), range13 unlimited 50000yd,
	// school Spellfire (0x44 Arcane+Fire), Speed 75 NOT
	// carried (seed has no speed field); SpellCategories
	// absent ("No entries found."); Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 63387, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolSpellfire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 173, basePoints: 1413, targetA: targetUnitConeEnemy104, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 64019 "Rapid Burst" (wago SpellName — MISMATCH vs
	// expected "Rapid Burst Right"; C++
	// SPELL_RAPID_BURST_RIGHT, boss_mimiron.cpp:118 enum;
	// Lua northrend/spell_mimiron_rapid_burst.lua:22-23,
	// periodic aura casts 63387/64019 alternating by tick
	// parity) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 8bd58485,
	// DifficultyID 0 row): eff0 SCHOOL_DAMAGE bp1413
	// die173 tgtA TARGET_UNIT_CONE_CASTER_TO_DEST_ENEMY
	// radius12 100yd; DB2-identical to 63387 except row IDs;
	// EffectAttributes 0x20000 NOT carried (seed effect
	// struct has no attributes field); cast1 instant, dur0
	// (no SpellDuration row), range13 unlimited 50000yd,
	// school Spellfire (0x44 Arcane+Fire), Speed 75 NOT
	// carried (seed has no speed field); SpellCategories
	// absent ("No entries found."); Attributes not carried;
	// no SpellClassOptions or SpellAuraOptions rows.
	{id: 64019, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolSpellfire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 173, basePoints: 1413, targetA: targetUnitConeEnemy104, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 9053 Fireball (wago SpellName; C++ SPELL_FIREBALL,
	// razorfen_downs.cpp:76 enum; Lua
	// kalimdor/npc_belnistrasz.lua:106, DoCastVictim) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 7b31e309, DifficultyID 0
	// row): eff0 SCHOOL_DAMAGE bp63 die23 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast14 3000ms, dur0 (no
	// SpellDuration row), range5 Long 0-40yd, school Fire;
	// EffectBonusCoefficient 1 NOT carried (seed effect
	// struct has no bonusCoefficient field); Speed 24 NOT
	// carried (seed has no speed field); SpellCategories row
	// (Category 0, DefenseType 1, PreventionType 0x1,
	// StartRecoveryCategory 133) not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 9053, castTimeIdx: 14, durIdx: 0, rangeIdx: 5, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 23, basePoints: 63, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 13326 Arcane Intellect (wago SpellName; C++
	// SPELL_ARCANE_INTELLECT, razorfen_downs.cpp:75 enum;
	// Lua kalimdor/npc_belnistrasz.lua:101, DoCastSelf under
	// HasAura guard) — wago.tools 3.4.0.47659 SpellEffect row
	// + SpellMisc (2026-11-16 browser read 7b31e309,
	// DifficultyID 0 row): eff0 APPLY_AURA MOD_STAT bp9
	// die1 misc 3 (Intellect) tgtA
	// TARGET_UNIT_TARGET_ALLY; cast1 instant, dur30
	// 1800000ms, range4 Medium 0-30yd, school Nature
	// (SchoolMask 0x8 as read); EffectRealPointsPerLevel
	// 0.2 NOT carried (seed effect struct has no
	// realPointsPerLevel field); SpellCategories row
	// (Category 0, DefenseType 1, DispelType 1,
	// PreventionType 0x1) not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 13326, castTimeIdx: 1, durIdx: 30, rangeIdx: 4, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraModStat, dieSides: 1, basePoints: 9, misc: 3, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 69900 Spirit Burst (wago SpellName; C++
	// SPELL_SPIRIT_BURST, halls_of_reflection.cpp:1877
	// enum; Lua northrend/npc_hor_spiritual_reflection.lua:48,
	// JustDied DoCastAOE) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 7b31e309, DifficultyID 0 row): eff0 SCHOOL_DAMAGE
	// bp3237 die525 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius18 15yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Shadow; DifficultyID 2/4 variants (bp4624
	// die751) NOT seeded per base-rows-only precedent;
	// SpellCategories rows not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 69900, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 525, basePoints: 3237, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 69933 Baleful Strike (wago SpellName; C++
	// SPELL_BALEFUL_STRIKE, halls_of_reflection.cpp:1876
	// enum; Lua northrend/npc_hor_spiritual_reflection.lua:47,
	// cast on aura target) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 7b31e309, DifficultyID 0 row): eff0 SCHOOL_DAMAGE
	// bp2024 die451 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur0 (no SpellDuration row), range137 Eight
	// yards 0-8yd, school Shadow; DifficultyID 2/4 variants
	// (bp4499 die1001) NOT seeded per base-rows-only
	// precedent; SpellCategories rows not carried;
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 69933, castTimeIdx: 1, durIdx: 0, rangeIdx: 137, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 451, basePoints: 2024, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 65860 Barkskin (wago SpellName; C++ SPELL_BARKSKIN,
	// boss_faction_champions.cpp:51 enum; Lua
	// northrend/npc_toc_druid.lua:30) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read d74da9a8, DifficultyID 0 rows): eff0
	// APPLY_AURA REDUCE_PUSHBACK bp99 die1 misc 127 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_DAMAGE_PERCENT_TAKEN bp-41 die1 misc 127 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// PROC_TRIGGER_SPELL bp0 die1 trigger 50411 mask
	// 16777829 tgtA TARGET_UNIT_CASTER (trigger spell 50411
	// NOT seeded per precedent; the trigger FIELD and mask
	// are carried); cast1 instant, dur29 12000ms, range1
	// Self Only 0yd, school Nature; SpellCategories row
	// (Category 0, DefenseType 1, DispelType 1,
	// PreventionType 0x1) not carried; Attributes not
	// carried; no SpellClassOptions or SpellAuraOptions rows.
	{id: 65860, castTimeIdx: 1, durIdx: 29, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraReducePushback, dieSides: 1, basePoints: 99, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgTaken, dieSides: 1, basePoints: -41, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, basePoints: 0, trigger: 50411, targetA: targetCaster, chainAmplitude: 1, mask: [3]uint32{16777829, 0, 0}},
	}},
	// 68595 "Champion's Aegis" (wago SpellName — MISMATCH
	// vs expected "Anti-AoE"; C++ SPELL_ANTI_AOE,
	// boss_faction_champions.cpp:42 enum; Lua
	// northrend/npc_toc_druid.lua:31, triggered self-cast)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read d74da9a8, DifficultyID 0
	// row): eff0 APPLY_AURA MOD_AOE_DAMAGE_AVOIDANCE bp-76
	// die1 misc 127 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur21 -1 infinite, range1 Self Only 0yd, school
	// Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 68595, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModAoeDamageAvoidance, dieSides: 1, basePoints: -76, misc: 127, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64626 "Explosion" (wago SpellName — MISMATCH vs
	// expected "Frost Bomb Explosion"; C++
	// SPELL_FROST_BOMB_EXPLOSION, boss_mimiron.cpp:143
	// enum; Lua northrend/npc_mimiron_frost_bomb.lua:9,
	// DoCastAOE) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-16 browser read d74da9a8,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp9424
	// die1151 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius10 30yd; eff1
	// KNOCK_BACK bp99 die1 misc 300 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY radius10 30yd; eff2
	// DUMMY bp0 die0 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY radius10 30yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Frost; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 64626, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFrost, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1151, basePoints: 9424, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 10, chainAmplitude: 1},
		{effect: effKnockBack, dieSides: 1, basePoints: 99, misc: 300, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 10, chainAmplitude: 1},
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 65354 Clear Fires (wago SpellName; C++
	// SPELL_CLEAR_FIRES, boss_mimiron.cpp:144 enum; Lua
	// northrend/npc_mimiron_frost_bomb.lua:10, DoCastAOE) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read d74da9a8, DifficultyID 0
	// row): eff0 DUMMY bp0 die0 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY radius10 30yd; cast1
	// instant, dur0 (no SpellDuration row), range1 Self Only
	// 0yd, school Physical; SpellCategories absent ("No
	// entries found."); Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 65354, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, dieSides: 0, basePoints: 0, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 64610 "Self-Destruction" (wago SpellName — MISMATCH
	// vs expected "Self Destruction Aura"; C++
	// SPELL_SELF_DESTRUCTION_AURA, boss_mimiron.cpp:168
	// enum; Lua northrend/npc_mimiron_computer.lua:41,
	// self-cast :107) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 browser read 6085bdc6,
	// DifficultyID 0 row): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 1000 bp0 die0
	// trigger 64611 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1 infinite, range1 Self Only 0yd,
	// school Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 64610, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 0, basePoints: 0, auraPeriod: 1000, trigger: 64611, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 64613 "Self-Destruction" (wago SpellName — MISMATCH
	// vs expected "Self Destruction Visual"; C++
	// SPELL_SELF_DESTRUCTION_VISUAL, boss_mimiron.cpp:169
	// enum; Lua northrend/npc_mimiron_computer.lua:42,
	// self-cast :108) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-16 browser read 6085bdc6,
	// DifficultyID 0 rows): eff0/1/2 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 250 bp-1 die1
	// trigger 64612 tgtA TARGET_UNIT_CASTER (all three
	// identical); cast1 instant, dur21 -1 infinite,
	// range1 Self Only 0yd, school Physical;
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 64613, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 250, trigger: 64612, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 250, trigger: 64612, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 250, trigger: 64612, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 65345 "Proximity Mine" (wago SpellName — MISMATCH vs
	// expected "Proximity Mine Periodic Trigger"; C++
	// SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER,
	// boss_mimiron.cpp:112 enum; Lua
	// northrend/npc_mimiron_proximity_mine.lua:9, self-cast
	// :59) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 6085bdc6,
	// DifficultyID 0 row): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL auraPeriod 500 bp-1 die1
	// trigger 65346 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1 infinite, range1 Self Only 0yd,
	// school Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no SpellClassOptions
	// or SpellAuraOptions rows.
	{id: 65345, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 500, trigger: 65346, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 66351 "Explosion" (wago SpellName — MISMATCH vs
	// expected "Proximity Mine Explosion"; C++
	// SPELL_PROXIMITY_MINE_EXPLOSION, boss_mimiron.cpp:110
	// enum; Lua northrend/npc_mimiron_proximity_mine.lua:10,
	// cast :49) — wago.tools 3.4.0.47659 SpellEffect rows
	// + SpellMisc (2026-11-16 browser read 6085bdc6,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp8999
	// die1 tgtA TARGET_DEST_CASTER tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY radius15 3yd; eff1
	// APPLY_AURA TRANSFORM bp-1 die1 misc 17286 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only 0yd, school Fire;
	// SpellCategories row EXISTS — not carried per
	// precedent; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 66351, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 1, basePoints: 8999, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 15, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, dieSides: 1, basePoints: -1, misc: 17286, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 65543 Psychic Scream (wago SpellName; C++
	// SPELL_PSYCHIC_SCREAM, boss_faction_champions.cpp:83
	// enum; Lua northrend/npc_toc_priest.lua:179, DoCastAOE
	// :234) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 3f410a2d,
	// DifficultyID 0 row): eff0 APPLY_AURA MOD_FEAR bp-1
	// die1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius14 8yd; cast1
	// instant, dur31 8000ms, range1 Self Only 0yd, school
	// Shadow; SpellCategories row EXISTS — not carried per
	// precedent; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 65543, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, dieSides: 1, basePoints: -1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
	}},
	// 66100 Mana Burn (wago SpellName; C++
	// SPELL_MANA_BURN, boss_faction_champions.cpp:84 enum;
	// Lua northrend/npc_toc_priest.lua:180, DoCast :248)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 browser read 3f410a2d, DifficultyID 0
	// base row ONLY — variants 2/4/5/6 NOT seeded per
	// precedent): eff0 POWER_BURN bp2245 die129 tgtA
	// TARGET_UNIT_TARGET_ENEMY radiusIdx 1 (no SpellRadius
	// row with ID 1 exists in this build); cast5 2000ms,
	// dur0 (no SpellDuration row), range4 Medium 0-30yd,
	// school Shadow; EffectAmplitude 3 and
	// EffectRealPointsPerLevel 2.9 NOT carried (no seed
	// fields); SpellCategories rows exist — not carried
	// per precedent; Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 66100, castTimeIdx: 5, durIdx: 0, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effPowerBurn, dieSides: 129, basePoints: 2245, targetA: targetEnemy, radiusIdx: 1, chainAmplitude: 1},
	}},
	// 70232 "Empowered Blood" (wago SpellName — MISMATCH
	// vs expected "Empowered Blood 2"; C++
	// SPELL_EMPOWERED_BLOOD_2, icecrown_citadel.cpp:94
	// enum; Lua northrend/spell_icecrown_citadel.lua:21,
	// triggered cast :27) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser
	// read 8e09b2df, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_POWER_COST_SCHOOL_PCT bp-101 die1 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA MOD_SCALE bp99
	// die1 tgtA TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MOD_MELEE_RANGED_HASTE bp99 die1 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur9 30000ms,
	// range1 Self Only 0yd, school Physical;
	// SpellCategories absent ("No entries found.");
	// Attributes not carried; no SpellClassOptions or
	// SpellAuraOptions rows.
	{id: 70232, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModPowerCostSchoolPct, dieSides: 1, basePoints: -101, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, dieSides: 1, basePoints: 99, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeRangedHaste, dieSides: 1, basePoints: 99, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 70320 "Empowered Blood" (wago SpellName — MISMATCH
	// vs expected "Empowered Blood 4"; C++
	// SPELL_EMPOWERED_BLOOD_4, icecrown_citadel.cpp:96
	// enum; Lua northrend/spell_icecrown_citadel.lua:23,
	// triggered cast :41) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 browser
	// read 8e09b2df, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_POWER_COST_SCHOOL_PCT bp-101 die1 tgtA
	// TARGET_UNIT_TARGET_ANY; eff1 APPLY_AURA
	// MOD_CASTING_SPEED_NOT_STACK bp99999 die1 tgtA
	// TARGET_UNIT_TARGET_ANY; cast1 instant, dur9 30000ms,
	// range13 Anywhere - Unlimited 0-50000yd, school
	// Physical; SpellCategories absent ("No entries
	// found."); Attributes not carried; no
	// SpellClassOptions or SpellAuraOptions rows.
	{id: 70320, castTimeIdx: 1, durIdx: 9, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModPowerCostSchoolPct, dieSides: 1, basePoints: -101, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModCastingSpeed, dieSides: 1, basePoints: 99999, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 63784 "Bladestorm" (wago SpellName — MATCH vs
	// expected "Bladestorm"; C++ SPELL_BLADESTORM,
	// boss_grand_champions.cpp:44 enum; Lua
	// northrend/boss_grand_champions.lua:89) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9669d5ec, DifficultyID 0 rows): eff0
	// APPLY_AURA PERIODIC_TRIGGER_SPELL bp-1 die1 period1000
	// trig63785 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MECHANIC_IMMUNITY_MASK bp-1 die1 misc477 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_DECREASE_SPEED
	// bp-31 die1 tgtA TARGET_UNIT_CASTER (SpellClassMask_1=4
	// not carried); cast1 instant, dur31 8000ms, range1 Self
	// Only 0yd, school Physical; SpellCategories row present
	// (not carried); Attributes not carried.
	{id: 63784, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, basePoints: -1, auraPeriod: 1000, trigger: 63785, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunityMask, dieSides: 1, basePoints: -1, misc: 477, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, dieSides: 1, basePoints: -31, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 67540 "Intercept" (wago SpellName — MATCH vs expected
	// "Intercept"; C++ SPELL_INTERCEPT,
	// boss_grand_champions.cpp:45 enum; Lua
	// northrend/boss_grand_champions.lua:88) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-16
	// browser read 9669d5ec, DifficultyID 0 rows): eff0 CHARGE
	// bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// TRIGGER_SPELL trig67573 tgtA TARGET_UNIT_TARGET_ENEMY
	// (EffectBonusCoefficient 1 not carried); cast1 instant,
	// dur0 none, range95 Charge 8-25yd, school Physical;
	// SpellCategories row present (not carried); Attributes
	// not carried.
	{id: 67540, castTimeIdx: 1, durIdx: 0, rangeIdx: 95, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effTriggerSpell, trigger: 67573, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 41986 "Anger" (wago SpellName — MATCH vs expected
	// "Anger"; C++ SPELL_ANGER, black_temple.cpp:34 enum,
	// :236 "// 41986 - Anger"; Lua
	// blacktemple/npc_angered_soul_fragment.lua:34, cast on
	// self once) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 browser read 8d4a9a8d,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp899
	// die201 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius18 15yd; cast14
	// 3000ms, dur0 none, range1 Self Only, school Nature;
	// SpellCategories row present (not carried); Attributes
	// not carried.
	{id: 41986, castTimeIdx: 14, durIdx: 0, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, dieSides: 201, basePoints: 899, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 15580 "Strike" (wago SpellName — MATCH vs expected
	// "Strike"; C++ SPELL_STRIKE,
	// boss_pyroguard_emberseer.cpp:50 enum,
	// boss_urok_doomhowl.cpp:25 enum; Lua
	// eastern_kingdoms/npc_blackhand_incarcerator.lua:50,
	// boss_urok_doomhowl.lua:59) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// 8d4a9a8d, DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp14 die1 tgtA TARGET_UNIT_TARGET_ENEMY
	// (EffectRealPointsPerLevel 4.5 + EffectBonusCoefficient
	// 1 not carried); cast1 instant, dur0 none, range2
	// Combat 5yd, school Physical; SpellCategories row
	// present (not carried); Attributes not carried.
	{id: 15580, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, dieSides: 1, basePoints: 14, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 65347 "Proximity Mines" (wago SpellName — MISMATCH vs
	// expected "Summon Proximity Mine"; C++
	// SPELL_SUMMON_PROXIMITY_MINE, boss_mimiron.cpp:114
	// enum; Lua northrend/spell_mimiron_proximity.lua:33,
	// caster-cast triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// dac0ae8f, DifficultyID 0 rows): eff0 TRIGGER_MISSILE
	// bp0 die0 trig63016 tgtA TARGET_DEST_CASTER_RADIUS
	// radius18 15yd (Speed 10 + Attributes not carried);
	// cast1 instant, dur0 none, range1 Self Only, school
	// Physical; NO SpellCategories row.
	{id: 65347, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effTriggerMiss, trigger: 63016, targetA: targetDestCasterRadius, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 15281 "Encage Emberseer" (wago SpellName — MATCH vs
	// expected "Encage Emberseer"; C++
	// SPELL_ENCAGE_EMBERSEER,
	// boss_pyroguard_emberseer.cpp:49 enum; Lua
	// eastern_kingdoms/npc_blackhand_incarcerator.lua:52,
	// JustReachedHome self-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 browser read
	// dac0ae8f, DifficultyID 0 rows): eff0 APPLY_AURA
	// DUMMY bp0 die1 tgtA TARGET_UNIT_NEARBY_ENTRY
	// (EffectBonusCoefficient 1 not carried); cast1
	// instant, dur21 -1 infinite, range6 Vision 100yd,
	// school Physical; NO SpellCategories row.
	{id: 15281, castTimeIdx: 1, durIdx: 21, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, dieSides: 1, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 47310 "Direbrew's Disarm" (wago SpellName — MISMATCH vs
	// expected "Direbrew Disarm"; C++
	// SPELL_DIREBREW_DISARM,
	// boss_coren_direbrew.cpp:67 enum; Lua
	// eastern_kingdoms/boss_coren_direbrew.lua:134) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_47310_47370_47369_47331.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_DISARM
	// bp0 die0 mechanic3 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY radius37 7yd; eff1
	// TRIGGER_SPELL trig47411; eff2 WEAPON_PERCENT_DAMAGE
	// bp149 die1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY radius37 7yd; cast15
	// 4000ms, dur35 4000ms, range1 Self Only, school
	// Physical; SpellCategories row present (Category 0,
	// DefenseType 2 Melee — not carried); Attributes not
	// carried.
	{id: 47310, castTimeIdx: 15, durIdx: 35, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDisarm, mechanic: 3, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 37, chainAmplitude: 1},
		{effect: effTriggerSpell, trigger: 47411, chainAmplitude: 1},
		{effect: effWeaponPercentDamage, dieSides: 1, basePoints: 149, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 37, chainAmplitude: 1},
	}},
	// 47370 "Send Mug Target Picker" (wago SpellName — MATCH
	// vs expected "Send Mug Target Picker"; C++
	// SPELL_SEND_MUG_TARGET_PICKER,
	// boss_coren_direbrew.cpp:71 enum; Lua
	// eastern_kingdoms/boss_coren_direbrew.lua:130) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_47310_47370_47369_47331.txt,
	// DifficultyID 0 rows): eff0 DUMMY bp0 die0 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENTRY
	// radius28 50000yd; cast1 instant, dur0 none, range1
	// Self Only, school Physical; NO SpellCategories row;
	// Attributes not carried.
	{id: 47370, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, radiusIdx: 28, chainAmplitude: 1},
	}},
	// 47369 "Send Mug Control Aura" (wago SpellName — MATCH
	// vs expected "Send Mug Control Aura"; C++
	// SPELL_SEND_MUG_CONTROL_AURA,
	// boss_coren_direbrew.cpp:70 enum; Lua
	// eastern_kingdoms/boss_coren_direbrew.lua:129) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_47310_47370_47369_47331.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_DUMMY bp0 die0 period12000 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range1 Self Only, school Physical; NO
	// SpellCategories row; Attributes not carried.
	{id: 47369, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDummy, auraPeriod: 12000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 47331 "Has Dark Brewmaiden's Brew" (wago SpellName —
	// MISMATCH vs expected "Has Dark Brewmaidens Brew";
	// C++ SPELL_HAS_DARK_BREWMAIDENS_BREW,
	// boss_coren_direbrew.cpp:75 enum; Lua
	// eastern_kingdoms/boss_coren_direbrew.lua:71-72
	// HasAura(47331) exclusion) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict file
	// wago_spells_47310_47370_47369_47331.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL bp0 die0 period10000
	// trig47344 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur21 -1 infinite, range6 Vision 100yd, school
	// Physical; NO SpellCategories row; Attributes not
	// carried.
	{id: 47331, castTimeIdx: 1, durIdx: 21, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 10000, trigger: 47344, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 16045 "Encage" (wago SpellName — MATCH vs expected
	// "Encage"; C++ SPELL_ENCAGE,
	// boss_pyroguard_emberseer.cpp:51 enum; Lua
	// eastern_kingdoms/npc_blackhand_incarcerator.lua:51,
	// DoCast(target, 16045, true) triggered) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_16045_36119.txt, browser
	// 0df1c53e, DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_STUN bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY
	// (EffectBonusCoefficient 1 not carried); cast1
	// instant, dur9 30000ms, range4 Medium 0-30yd, school
	// Physical; SpellCategories row present (not carried);
	// Attributes not carried.
	{id: 16045, castTimeIdx: 1, durIdx: 9, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 36119 "Void Zone" (wago SpellName — MATCH vs expected
	// "Void Zone"; C++ SPELL_VOID_ZONE,
	// arcatraz/boss_zereketh_the_unbound.cpp:32 enum; Lua
	// arcatraz/boss_zereketh_the_unbound.lua:74, DoCast
	// (target, 36119)) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 verdict file
	// wago_spells_16045_36119.txt, browser 0df1c53e,
	// DifficultyID 0 rows): eff0 SUMMON bp0 die1
	// misc21101 miscB64 tgtA TARGET_DEST_TARGET_ENEMY
	// (EffectBonusCoefficient 0, PvpMultiplier 1,
	// GroupSizeBasePointsCoefficient 1 not carried);
	// cast1 instant, dur63 25000ms, range6 Vision
	// 0-100yd, school Shadow; SpellCategories row present
	// (not carried); Attributes not carried.
	{id: 36119, castTimeIdx: 1, durIdx: 63, rangeIdx: 6, school: schoolShadow, effects: [3]effect{
		{effect: effSummon, dieSides: 1, misc: 21101, miscB: 64, targetA: targetDestTargetEnemy, chainAmplitude: 1},
	}},
	// 36127 "Shadow Nova" (wago SpellName — MATCH vs
	// expected "Shadow Nova"; C++
	// SPELL_SHADOW_NOVA,
	// boss_zereketh_the_unbound.cpp:33 enum; Lua
	// arcatraz/boss_zereketh_the_unbound.lua:75) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_36127_36123.txt, DifficultyID 0 rows):
	// eff0 SCHOOL_DAMAGE bp1849 die301 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// radius23 40yd; eff1 KNOCK_BACK bp99 die1 misc250
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius10 30yd; cast16
	// 1500ms, dur0 none, range1 Self Only, school
	// Shadow; SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 36127, castTimeIdx: 16, durIdx: 0, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1849, dieSides: 301, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 23, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 99, dieSides: 1, misc: 250, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 10, chainAmplitude: 1},
	}},
	// 36123 "Seed of Corruption" (wago SpellName — MATCH
	// vs expected "Seed of Corruption"; C++
	// SPELL_SEED_OF_CORRUPTION,
	// boss_zereketh_the_unbound.cpp:34 enum; Lua
	// arcatraz/boss_zereketh_the_unbound.lua:76) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_36127_36123.txt, DifficultyID 0 rows):
	// eff0 APPLY_AURA PERIODIC_DAMAGE bp399 die1
	// period3000 tgtA TARGET_UNIT_TARGET_ENEMY
	// (EffectBonusCoefficient 0.167 not carried — no
	// seed field); eff1 APPLY_AURA DUMMY bp2499 die1
	// tgtA TARGET_UNIT_TARGET_ENEMY; eff2 KNOCK_BACK
	// bp74 die1 misc35 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast5 2000ms, dur85 18000ms, range4 Medium 30yd,
	// school Shadow; Speed 28 not carried;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 —
	// not carried); Attributes not carried.
	{id: 36123, castTimeIdx: 5, durIdx: 85, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 399, dieSides: 1, auraPeriod: 3000, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDummy, basePoints: 2499, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 74, dieSides: 1, misc: 35, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34659 "Hellfire" (wago SpellName — MATCH vs
	// expected "Hellfire"; C++ SPELL_HELLFIRE,
	// boss_thorngrin_the_tender.cpp:37 enum; Lua
	// botanica/boss_thorngrin_the_tender.lua:100) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_34659_34661_34670.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp-1 die1 period1000 trig34660 tgtA
	// TARGET_UNIT_CASTER; trigger spell 34660 NOT
	// seeded per precedent; cast1 instant, dur32
	// 6000ms, range1 Self Only, school Fire;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes not
	// carried.
	{id: 34659, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 1000, trigger: 34660, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 34661 "Sacrifice" (wago SpellName — MATCH vs
	// expected "Sacrifice"; C++ SPELL_SACRIFICE,
	// boss_thorngrin_the_tender.cpp:36 enum; Lua
	// botanica/boss_thorngrin_the_tender.lua:99) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_34659_34661_34670.txt, DifficultyID 0
	// rows): eff0 TELEPORT_UNITS bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY tgtB TARGET_DEST_DB
	// (EffectPos_facing 4.66 not carried — no seed
	// field); eff1 APPLY_AURA MOD_STUN bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff2 APPLY_AURA
	// PERIODIC_LEECH bp641 die67 period1000 tgtA
	// TARGET_UNIT_TARGET_ENEMY (EffectAmplitude 1 not
	// carried — no seed field); cast4 1000ms, dur31
	// 8000ms, range5 Long 40yd, school Shadow; NO
	// SpellCategories row; Attributes not carried.
	{id: 34661, castTimeIdx: 4, durIdx: 31, rangeIdx: 5, school: schoolShadow, effects: [3]effect{
		{effect: effTeleportUnits, targetA: targetEnemy, targetB: targetDestDB, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicLeech, basePoints: 641, dieSides: 67, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34670 "Enrage" (wago SpellName — MATCH vs expected
	// "Enrage"; C++ SPELL_ENRAGE,
	// boss_thorngrin_the_tender.cpp:38 enum; Lua
	// botanica/boss_thorngrin_the_tender.lua:101) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_34659_34661_34670.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// bp74 die1 misc1 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_MELEE_HASTE bp74 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE
	// bp24 die1 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur1 10000ms, range1 Self Only, school Physical;
	// SpellCategories row present (Category 21,
	// DispelType 9 ENRAGE — not carried); Attributes
	// not carried.
	{id: 34670, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 74, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 74, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 24, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 25054 "Holy Smite" (wago SpellName — MISMATCH vs
	// expected "Solarium Holy Smite"; C++
	// SPELL_SOLARIUM_HOLY_SMITE,
	// boss_astromancer.cpp:58 enum; Lua
	// the_eye/npc_solarium_priest.lua) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_25054_33387_33390.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp84 die31 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast19 2500ms, dur0
	// none, range5 Long 40yd, school Holy;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes not
	// carried.
	{id: 25054, castTimeIdx: 19, durIdx: 0, rangeIdx: 5, school: schoolHoly, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 84, dieSides: 31, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 33387 "Great Heal" (wago SpellName — MISMATCH vs
	// expected "Solarium Great Heal"; C++
	// SPELL_SOLARIUM_GREAT_HEAL,
	// boss_astromancer.cpp:57 enum; Lua
	// the_eye/npc_solarium_priest.lua:49) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_25054_33387_33390.txt, DifficultyID 0
	// rows): eff0 HEAL bp23124 die3751 tgtA
	// TARGET_UNIT_TARGET_ALLY (EffectAmplitude 1 not
	// carried — no seed field); cast5 2000ms, dur0
	// none, range6 Vision 100yd, school Holy;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 —
	// not carried); Attributes not carried.
	{id: 33387, castTimeIdx: 5, durIdx: 0, rangeIdx: 6, school: schoolHoly, effects: [3]effect{
		{effect: effHeal, basePoints: 23124, dieSides: 3751, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 33390 "Arcane Torrent" (wago SpellName — MISMATCH
	// vs expected "Solarium Arcane Torrent"; C++
	// SPELL_SOLARIUM_ARCANE_TORRENT,
	// boss_astromancer.cpp:59 enum; Lua
	// the_eye/npc_solarium_priest.lua) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_25054_33387_33390.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_SILENCE bp-1 die1
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius14 8yd; eff1
	// DUMMY bp33482 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur39 2000ms, range1 Self Only,
	// school Arcane; SpellCategories row present
	// (DefenseType 1 Magic, DispelType 1 MAGIC,
	// Mechanic 9, PreventionType 0x1,
	// StartRecoveryCategory 133 — not carried);
	// Attributes not carried.
	{id: 33390, castTimeIdx: 1, durIdx: 39, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraSilence, basePoints: -1, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 14, chainAmplitude: 1},
		{effect: effDummy, basePoints: 33482, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 36513 "Intangible Presence" (wago SpellName —
	// MATCH vs expected "Intangible Presence"; C++
	// SPELL_INTANGIBLE_PRESENCE,
	// zone_blades_edge_mountains.cpp:68 enum; Lua
	// outland/npc_nether_drake.lua:108) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_36513_38881_38884.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_TAKEN
	// bp-41 die1 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_DAMAGE_PERCENT_TAKEN bp19 die1
	// misc126 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur31 8000ms, range4 Medium 30yd, school Shadow;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes not
	// carried.
	{id: 36513, castTimeIdx: 1, durIdx: 31, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: -41, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: 19, dieSides: 1, misc: 126, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 38881 "Arcane Blast" (wago SpellName — MATCH vs
	// expected "Arcane Blast"; C++ SPELL_ARCANE_BLAST,
	// zone_blades_edge_mountains.cpp:66 enum; Lua
	// outland/npc_nether_drake.lua:106) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_36513_38881_38884.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp647 die105 tgtA
	// TARGET_UNIT_TARGET_ENEMY (EffectBonusCoefficient
	// 0.714 + RealPointsPerLevel 5 not carried — no
	// seed fields); eff1 APPLY_AURA
	// MOD_CASTING_SPEED_NOT_STACK bp9 die1 tgtA
	// TARGET_UNIT_CASTER (EffectSpellClassMask_0
	// 536870912 not carried per 63784 precedent);
	// cast19 2500ms, dur31 8000ms, range4 Medium 30yd,
	// school Arcane; SpellCategories row present
	// (DefenseType 1 Magic, PreventionType 0x1,
	// StartRecoveryCategory 133 — not carried);
	// Attributes not carried.
	{id: 38881, castTimeIdx: 19, durIdx: 31, rangeIdx: 4, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 647, dieSides: 105, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModCastingSpeed, basePoints: 9, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 38884 "Mana Burn" (wago SpellName — MATCH vs
	// expected "Mana Burn"; C++ SPELL_MANA_BURN,
	// zone_blades_edge_mountains.cpp:67 enum; Lua
	// outland/npc_nether_drake.lua:107) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_36513_38881_38884.txt, DifficultyID 0
	// rows): eff0 POWER_BURN bp184 die13 tgtA
	// TARGET_UNIT_TARGET_ENEMY (EffectAmplitude 0.5
	// not carried — no seed field); cast14 3000ms,
	// dur0 none, range4 Medium 30yd, school Arcane;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes not
	// carried.
	{id: 38884, castTimeIdx: 14, durIdx: 0, rangeIdx: 4, school: schoolArcane, effects: [3]effect{
		{effect: effPowerBurn, basePoints: 184, dieSides: 13, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 13321 "Mana Burn" (wago SpellName — MATCH vs
	// expected "Mana Burn"; C++ SPELL_MANA_BURN,
	// zone_netherstorm.cpp:352 enum; Lua
	// outland/npc_phase_hunter.lua:115) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_13321_34804_36574.txt, DifficultyID 0
	// rows): eff0 POWER_BURN bp184 die13 tgtA
	// TARGET_UNIT_TARGET_ENEMY (EffectAmplitude 0.5
	// not carried — no seed field; same values as
	// 38884, different school); cast14 3000ms, dur0
	// none, range4 Medium 30yd, school Shadow;
	// SpellCategories row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes not
	// carried.
	{id: 13321, castTimeIdx: 14, durIdx: 0, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effPowerBurn, basePoints: 184, dieSides: 13, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34804 "Materialize" (wago SpellName — MATCH vs
	// expected "Materialize"; C++ SPELL_MATERIALIZE,
	// zone_netherstorm.cpp:353 enum; Lua
	// outland/npc_phase_hunter.lua:114) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_13321_34804_36574.txt, DifficultyID 0
	// rows): eff0 SPAWN bp0 die0 tgtA NONE tgtB NONE
	// (clientside-only, carried verbatim per 37090
	// precedent); cast1 instant, dur0 none, range1
	// Self Only, school Physical; NO SpellCategories
	// row; Attributes not carried.
	{id: 34804, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSpawn, chainAmplitude: 1},
	}},
	// 36574 "Phase Slip" (wago SpellName — MATCH vs
	// expected "Phase Slip"; C++ SPELL_PHASE_SLIP,
	// zone_netherstorm.cpp:351 enum; Lua
	// outland/npc_phase_hunter.lua) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_13321_34804_36574.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MECHANIC_IMMUNITY
	// bp-1201 die1 misc11 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA MECHANIC_IMMUNITY bp0 die0
	// misc13 tgtA TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MECHANIC_IMMUNITY bp0 die0 misc7 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur31 8000ms,
	// range1 Self Only, school Arcane; SpellCategories
	// row present (Category 44, DefenseType 1 Magic,
	// PreventionType 0x1, StartRecoveryCategory 133 —
	// not carried); Attributes not carried.
	{id: 36574, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraMechanicImmunity, basePoints: -1201, dieSides: 1, misc: 11, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunity, misc: 13, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunity, misc: 7, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 33684 "Incite Chaos" (wago SpellName — MATCH vs
	// expected "Incite Chaos"; C++
	// SPELL_INCITE_CHAOS_B,
	// boss_blackheart_the_inciter.cpp:32 enum; Lua
	// outland/boss_blackheart_the_inciter.lua) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_33684_33707_33709.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_CHARM bp998 die1
	// tgtA TARGET_UNIT_TARGET_ANY; cast1 instant, dur8
	// 15000ms, range13 Anywhere 50000yd, school
	// Physical; NO SpellCategories row; Attributes not
	// carried.
	{id: 33684, castTimeIdx: 1, durIdx: 8, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCharm, basePoints: 998, dieSides: 1, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 33707 "War Stomp" (wago SpellName — MATCH vs
	// expected "War Stomp"; C++ SPELL_WAR_STOMP,
	// boss_blackheart_the_inciter.cpp:34 enum; Lua
	// outland/boss_blackheart_the_inciter.lua:106) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_33684_33707_33709.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1749 die501 tgtA
	// TARGET_DEST_CASTER tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY radius9 20yd; eff1
	// KNOCK_BACK bp149 die101 misc150 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius9 20yd; cast1
	// instant, dur0 none, range1 Self Only, school
	// Physical; SpellCategories row present
	// (DefenseType 1 Magic, PreventionType 0x1 — not
	// carried); Attributes not carried.
	{id: 33707, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1749, dieSides: 501, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 149, dieSides: 101, misc: 150, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 33709 "Charge" (wago SpellName — MATCH vs
	// expected "Charge"; C++ SPELL_CHARGE,
	// boss_blackheart_the_inciter.cpp:33 enum; Lua
	// outland/boss_blackheart_the_inciter.lua:107) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_33684_33707_33709.txt, DifficultyID 0
	// rows): eff0 CHARGE bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 WEAPON_DAMAGE
	// bp74 die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff2
	// APPLY_AURA MOD_STUN bp-1 die1 mechanic12 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur39
	// 2000ms, range5 Long 40yd, school Physical;
	// Speed 27 not carried; SpellCategories row
	// present (Category 44, Mechanic 12 — not
	// carried); Attributes not carried.
	{id: 33709, castTimeIdx: 1, durIdx: 39, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effWeaponDamage, basePoints: 74, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, basePoints: -1, dieSides: 1, mechanic: 12, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 33547 "Fear" (wago SpellName — MATCH vs
	// expected "Fear"; C++ SPELL_FEAR,
	// boss_ambassador_hellmaw.cpp:43 enum; Lua
	// outland/boss_ambassador_hellmaw.lua:94) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_33547_33551_34970.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_FEAR bp-1 die1 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius11 45yd; cast1
	// instant, dur32 6000ms, range1 Self Only, school
	// Shadow; SpellCategories row present (DispelType 1
	// MAGIC, Mechanic 5 — not carried); Attributes not
	// carried.
	{id: 33547, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: -1, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 33551 "Corrosive Acid" (wago SpellName — MATCH
	// vs expected "Corrosive Acid"; C++
	// SPELL_CORROSIVE_ACID,
	// boss_ambassador_hellmaw.cpp:42 enum; Lua
	// outland/boss_ambassador_hellmaw.lua:93) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_33547_33551_34970.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE bp1093
	// die313 period3000 tgtA TARGET_UNIT_CONE_ENEMY_24
	// radius9 20yd; eff1 APPLY_AURA MOD_RESISTANCE
	// bp-3939 die-1123 misc1 tgtA
	// TARGET_UNIT_CONE_ENEMY_24 radius9 20yd; cast5
	// 2000ms, dur8 15000ms, range1 Self Only, school
	// Nature; BonusCoefficient 1 not carried;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 33551, castTimeIdx: 5, durIdx: 8, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 1093, dieSides: 313, auraPeriod: 3000, targetA: targetUnitConeEnemy, radiusIdx: 9, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModResistance, basePoints: -3939, dieSides: -1123, misc: 1, targetA: targetUnitConeEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 34970 "Frenzy" (wago SpellName — MISMATCH vs
	// expected "Enrage"; wago Name_lang is "Frenzy",
	// same ID is SPELL_FRENZY in
	// boss_rokmar_the_crackler.cpp:34; C++
	// SPELL_ENRAGE, boss_ambassador_hellmaw.cpp:44
	// enum; Lua outland/boss_ambassador_hellmaw.lua:95)
	// — IDs + values govern, seeding per precedent.
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_33547_33551_34970.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_DONE bp74
	// die1 misc1 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_MELEE_HASTE bp59 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE
	// bp19 die1 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur4 120000ms, range1 Self Only, school
	// Physical; RealPointsPerLevel 3 not carried;
	// SpellCategories row present (Category 21 — not
	// carried); Attributes not carried.
	{id: 34970, castTimeIdx: 1, durIdx: 4, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageDone, basePoints: 74, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 59, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 19, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 32302 "Fiery Blast" (wago SpellName — MATCH vs
	// expected "Fiery Blast"; C++
	// SPELL_FIERY_BLAST,
	// boss_shirrak_the_dead_watcher.cpp:38 enum; Lua
	// outland/boss_shirrak_the_dead_watcher.lua:87) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_32302_36383_42075.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1584 die81 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius32 12yd; cast1
	// instant, dur0 none, range1 Self Only, school
	// Fire; BonusCoefficient 1 not carried;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 32302, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1584, dieSides: 81, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 32, chainAmplitude: 1},
	}},
	// 36383 "Carnivorous Bite" (wago SpellName — MATCH
	// vs expected "Carnivorous Bite"; C++
	// SPELL_CARNIVOROUSBITE,
	// boss_shirrak_the_dead_watcher.cpp:36 enum; Lua
	// outland/boss_shirrak_the_dead_watcher.lua:85) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_32302_36383_42075.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE bp452
	// die1 period3000 mechanic15 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY radius32 12yd; cast1
	// instant, dur18 20000ms, range2 Combat Range
	// 0-5yd, school Physical; RealPointsPerLevel 35
	// not carried; SpellCategories row present
	// (DefenseType 2 Melee, Mechanic 15,
	// PreventionType 0x2 — not carried); Attributes
	// not carried.
	{id: 36383, castTimeIdx: 1, durIdx: 18, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 452, dieSides: 1, auraPeriod: 3000, mechanic: 15, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 32, chainAmplitude: 1},
	}},
	// 42075 "Headless Horseman - Visual - Large Fire"
	// (wago SpellName — MISMATCH vs expected "Focus
	// Fire Visual"; C++ SPELL_FOCUS_FIRE_VISUAL,
	// boss_shirrak_the_dead_watcher.cpp:40 enum; Lua
	// outland/boss_shirrak_the_dead_watcher.lua:86) —
	// IDs + values govern, seeding per precedent.
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_32302_36383_42075.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DUMMY bp0 die0
	// period1000 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1 infinite, range1 Self Only,
	// school Physical; NO SpellCategories row;
	// Attributes not carried.
	{id: 42075, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDummy, auraPeriod: 1000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 15284 "Cleave" (wago SpellName — MISMATCH vs
	// expected "Omen Cleave"; C++ SPELL_OMEN_CLEAVE,
	// zone_moonglade.cpp:400 enum; Lua
	// kalimdor/npc_omen.lua:84) — IDs + values govern,
	// seeding per precedent. wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict
	// file wago_spells_15284_26392_26540.txt,
	// DifficultyID 0 rows): eff0 WEAPON_PERCENT_DAMAGE
	// bp109 die1 chainTargets 3 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// none, range2 Combat Range 0-5yd, school
	// Physical; SpellCategories row present
	// (DefenseType 2 Melee, PreventionType 0x2 — not
	// carried); Attributes not carried.
	{id: 15284, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 109, dieSides: 1, targetA: targetEnemy, chainTargets: 3, chainAmplitude: 1},
	}},
	// 26392 "Omen's Moonlight" (wago SpellName —
	// MISMATCH vs expected "Omen Summon Spotlight";
	// C++ SPELL_OMEN_SUMMON_SPOTLIGHT,
	// zone_moonglade.cpp:402 enum; Lua
	// kalimdor/npc_omen.lua:86) — IDs + values govern,
	// seeding per precedent. wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16 verdict
	// file wago_spells_15284_26392_26540.txt,
	// DifficultyID 0 rows): eff0 SUMMON bp0 die1
	// misc 15902 miscB 64 radius16 1yd tgtA
	// TARGET_DEST_CASTER; eff1 SUMMON_OBJECT_WILD bp0
	// die1 misc 180876 tgtA TARGET_DEST_CASTER; eff2
	// SUMMON_OBJECT_WILD misc 180877 tgtA
	// TARGET_DEST_CASTER; cast1 instant, dur5
	// 300000ms, range4 Medium 0-30yd, school Physical;
	// BonusCoefficient 1 (eff0) not carried; NO
	// SpellCategories row; Attributes not carried.
	{id: 26392, castTimeIdx: 1, durIdx: 5, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, dieSides: 1, misc: 15902, miscB: 64, targetA: targetDestCaster, radiusIdx: 16, chainAmplitude: 1},
		{effect: effSummonObjectWild, dieSides: 1, misc: 180876, targetA: targetDestCaster, chainAmplitude: 1},
		{effect: effSummonObjectWild, dieSides: 1, misc: 180877, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 26540 "Starfall" (wago SpellName — MISMATCH vs
	// expected "Omen Starfall"; C++ SPELL_OMEN_STARFALL,
	// zone_moonglade.cpp:401 enum; Lua
	// kalimdor/npc_omen.lua:85) — IDs + values govern,
	// seeding per precedent. wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict
	// file wago_spells_15284_26392_26540.txt,
	// DifficultyID 0 rows): eff0 PERSISTENT_AREA_AURA
	// PERIODIC_DAMAGE bp3039 die321 period2000
	// radius18 15yd tgtA TARGET_DEST_DYNOBJ_ENEMY;
	// cast1 instant, dur32 6000ms, range36 Longer
	// 0-45yd, school Arcane; SpellCategories row
	// present (DefenseType 1 Magic, PreventionType
	// 0x1 — not carried); Attributes not carried.
	{id: 26540, castTimeIdx: 1, durIdx: 32, rangeIdx: 36, school: schoolArcane, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, basePoints: 3039, dieSides: 321, auraPeriod: 2000, targetA: targetDestDynobjEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 31326 "Corrupt Medivh" (wago SpellName —
	// MISMATCH vs expected "Corrupt"; C++
	// SPELL_CORRUPT,
	// the_black_morass.cpp:50 enum; Lua
	// kalimdor/the_black_morass.lua:118) — IDs +
	// values govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_31326_31556_37853.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die0 period3000 tgtA
	// TARGET_UNIT_NEARBY_ENTRY; cast1 instant, dur21
	// -1 infinite, range13 Anywhere-Unlimited
	// 0-50000yd, school Shadow; NO SpellCategories
	// row; Attributes not carried.
	{id: 31326, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 3000, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 31556 "Medivh Channel" (wago SpellName —
	// MISMATCH vs expected "Channel"; C++
	// SPELL_CHANNEL,
	// the_black_morass.cpp:41 enum; Lua
	// kalimdor/the_black_morass.lua:117) — IDs +
	// values govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_31326_31556_37853.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA DUMMY bp0 die0 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite, range13 Anywhere-Unlimited 0-50000yd,
	// school Physical; NO SpellCategories row;
	// SpellAuraOptions row (ProcChance 101) not
	// carried (not a requested table); Attributes not
	// carried.
	{id: 31556, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 37853 "Corrupt Medivh" (wago SpellName —
	// MISMATCH vs expected "Corrupt Aeonus"; C++
	// SPELL_CORRUPT_AEONUS,
	// the_black_morass.cpp:51 enum; Lua
	// kalimdor/the_black_morass.lua:119) — IDs +
	// values govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_31326_31556_37853.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die0 period1000 tgtA
	// TARGET_UNIT_NEARBY_ENTRY; eff1 APPLY_AURA DUMMY
	// bp0 die0 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur21 -1 infinite, range13
	// Anywhere-Unlimited 0-50000yd, school Shadow;
	// NO SpellCategories row; Attributes not carried.
	{id: 37853, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 1000, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 30254 "Evocation" (wago SpellName — MATCH vs
	// expected "Evocation"; C++ SPELL_EVOCATION,
	// boss_curator.cpp:35 enum; Lua
	// karazhan/boss_curator.lua:46) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_30254_30383_30403.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA OBS_MOD_POWER bp4 die1
	// period1000 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_STUN bp0 die0 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MOD_DAMAGE_PERCENT_TAKEN bp199 die1 misc127 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur18
	// 20000ms, range1 Self Only, school Arcane; NO
	// SpellCategories row; Attributes not carried.
	{id: 30254, castTimeIdx: 1, durIdx: 18, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraObsModPower, basePoints: 4, dieSides: 1, auraPeriod: 1000, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgTaken, basePoints: 199, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 30383 "Hateful Bolt" (wago SpellName — MATCH vs
	// expected "Hateful Bolt"; C++ SPELL_HATEFUL_BOLT,
	// boss_curator.cpp:34 enum; Lua
	// karazhan/boss_curator.lua:45) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_30254_30383_30403.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp4434 die1565 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast4 1000ms, dur0
	// none, range36 Longer 0-45yd, school Arcane;
	// BonusCoefficient 1 not carried; SpellCategories
	// row present (DefenseType 1 Magic,
	// PreventionType 0x0 — not carried); Attributes
	// not carried.
	{id: 30383, castTimeIdx: 4, durIdx: 0, rangeIdx: 36, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 4434, dieSides: 1565, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 30403 "Arcane Infusion" (wago SpellName — MATCH
	// vs expected "Arcane Infusion"; C++
	// SPELL_ARCANE_INFUSION, boss_curator.cpp:36 enum;
	// Lua karazhan/boss_curator.lua:47) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_30254_30383_30403.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PROC_TRIGGER_DAMAGE
	// bp1499 die1001 tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA TRANSFORM bp0 die0 misc18780 tgtA
	// TARGET_UNIT_CASTER; cast4 1000ms, dur21 -1
	// infinite, range1 Self Only, school Arcane;
	// BonusCoefficient 1 (eff0) not carried; NO
	// SpellCategories row; Attributes not carried.
	{id: 30403, castTimeIdx: 4, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerDmg, basePoints: 1499, dieSides: 1001, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, misc: 18780, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 11027 "Knockback 500" (wago SpellName —
	// MISMATCH vs expected "Flame Wreath"; Lua
	// SPELL_FLAMEWREATH_DOT,
	// karazhan/boss_shade_of_aran.lua:110; C++ raw-ID
	// CastSpell(unit, 11027, true),
	// boss_shade_of_aran.cpp:463) — IDs + values
	// govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_11027_20476_31012.txt, DifficultyID 0
	// rows): eff0 KNOCK_BACK bp499 die1 misc500 tgtA
	// TARGET_UNIT_TARGET_ANY; cast1 instant, dur0
	// none, range13 Anywhere-Unlimited 0-50000yd,
	// school Physical; NO SpellCategories row;
	// Attributes not carried.
	{id: 11027, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effKnockBack, basePoints: 499, dieSides: 1, misc: 500, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 20476 "Explosion" (wago SpellName — MISMATCH vs
	// expected "Flame Wreath"; Lua
	// SPELL_FLAMEWREATH_PUNISH,
	// karazhan/boss_shade_of_aran.lua:108; C++ raw-ID
	// CastSpell(unit, 20476, me->GetGUID()),
	// boss_shade_of_aran.cpp:462) — IDs + values
	// govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_11027_20476_31012.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp3199 die1 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ALLY radius19 18yd; eff1
	// KNOCK_BACK bp332 die35 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur0 none, range1 Self Only,
	// school Fire; BonusCoefficient 1 (both effects)
	// not carried; SpellCategories row present
	// (DefenseType 1 Magic, PreventionType 0x1 — not
	// carried); Attributes not carried.
	{id: 20476, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 3199, dieSides: 1, targetA: targetSrcCaster, targetB: targetUnitSrcAreaAlly, radiusIdx: 19, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 332, dieSides: 35, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 31012 "Water Bolt" (wago SpellName — MISMATCH
	// vs expected "Waterbolt" (two words vs one); C++
	// SPELL_WATERBOLT, boss_shade_of_aran.cpp:68 enum;
	// Lua karazhan/boss_shade_of_aran.lua:107) — IDs +
	// values govern, seeding per precedent. wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_11027_20476_31012.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp2024 die451 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast16 1500ms, dur0
	// none, range36 Longer 0-45yd, school Frost;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 31012, castTimeIdx: 16, durIdx: 0, rangeIdx: 36, school: schoolFrost, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 2024, dieSides: 451, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 30967 "Fireball" (wago SpellName — MISMATCH vs
	// expected "Fire Ball" (one word vs two); C++
	// SPELL_FIRE_BALL, karazhan.cpp:52 enum; Lua
	// karazhan/karazhan.lua:56) — IDs + values govern,
	// seeding per precedent. wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict
	// file wago_spells_30967_30977_31635.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp1954
	// die691 tgtA TARGET_UNIT_TARGET_ANY; cast6
	// 5000ms, dur0 none, range134 Tower 80 0-80yd,
	// school Fire; BonusCoefficient 1 not carried;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 30967, castTimeIdx: 6, durIdx: 0, rangeIdx: 134, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1954, dieSides: 691, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 30977 "Conflagration Blast" (wago SpellName —
	// MATCH vs expected "Conflagration Blast"; C++
	// SPELL_CONFLAGRATION_BLAST, karazhan.cpp:54 enum;
	// Lua karazhan/karazhan.lua:57) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_30967_30977_31635.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE bp2999
	// die1 period1000 tgtA TARGET_UNIT_TARGET_ANY;
	// eff1 SCHOOL_DAMAGE bp155999 die1 tgtA
	// TARGET_UNIT_TARGET_ANY; eff2 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL bp0 die1 period1000
	// trigger16806 tgtA TARGET_UNIT_TARGET_ANY; cast1
	// instant, dur41 360000ms, range6 Vision 0-100yd,
	// school Fire; BonusCoefficient 1 (eff2) not
	// carried; NO SpellCategories row; Attributes not
	// carried.
	{id: 30977, castTimeIdx: 1, durIdx: 41, rangeIdx: 6, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 2999, dieSides: 1, auraPeriod: 1000, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 155999, dieSides: 1, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, auraPeriod: 1000, trigger: 16806, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 31635 "Mana Shield" (wago SpellName — MATCH vs
	// expected "Mana Shield"; C++ SPELL_MANA_SHIELD,
	// karazhan.cpp:55 enum; Lua
	// karazhan/karazhan.lua:58) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict
	// file wago_spells_30967_30977_31635.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY bp-1
	// die1 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur21 -1 infinite, range1 Self Only, school
	// Arcane; EffectAmplitude 2 not carried (no
	// amplitude field in seeder); NO SpellCategories
	// row; Attributes not carried.
	{id: 31635, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 35311 "Stream of Machine Fluid" (wago SpellName
	// — MATCH vs expected "Stream of Machine Fluid";
	// C++ SPELL_STREAM_OF_MACHINE_FLUID,
	// boss_gatewatcher_gyrokill.cpp:39 enum; Lua
	// mechanar/boss_gatewatcher_gyrokill.lua:59) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_35311_35318_35322.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MELEE_SLOW bp-36 die1
	// radius18 15yd tgtA TARGET_UNIT_CONE_ENEMY_24;
	// eff1 APPLY_AURA MOD_RESISTANCE_PCT bp-36 die1
	// misc1 radius18 15yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; cast1 instant, dur1
	// 10000ms, range3 Twenty yards 0-20yd, school
	// Nature; EffectAttributes 0x20000 (both effects)
	// not carried; SpellCategories row present
	// (Category 1152, DispelType 4 — not carried);
	// Attributes not carried.
	{id: 35311, castTimeIdx: 1, durIdx: 1, rangeIdx: 3, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraMeleeSlow, basePoints: -36, dieSides: 1, targetA: targetUnitConeEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModResistPct, basePoints: -36, dieSides: 1, misc: 1, targetA: targetUnitConeEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 35318 "Saw Blade" (wago SpellName — MATCH vs
	// expected "Saw Blade"; C++ SPELL_SAW_BLADE,
	// boss_gatewatcher_gyrokill.cpp:40 enum; Lua
	// mechanar/boss_gatewatcher_gyrokill.lua:60) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_35311_35318_35322.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1757 die185
	// chainTargets 5 chainAmp 1.2 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// PERIODIC_DAMAGE bp522 die55 period2000
	// mechanic15 chainTargets 5 chainAmp 1.1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur31
	// 8000ms, range6 Vision 0-100yd, school Physical;
	// BonusCoefficient 1 (eff0) and
	// BonusCoefficientFromAP 1 (eff0) not carried; NO
	// SpellCategories row; Attributes not carried.
	{id: 35318, castTimeIdx: 1, durIdx: 31, rangeIdx: 6, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1757, dieSides: 185, targetA: targetEnemy, chainTargets: 5, chainAmplitude: 1.2},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 522, dieSides: 55, auraPeriod: 2000, mechanic: 15, targetA: targetEnemy, chainTargets: 5, chainAmplitude: 1.1},
	}},
	// 35322 "Shadow Power" (wago SpellName — MATCH vs
	// expected "Shadow Power"; C++ SPELL_SHADOW_POWER,
	// boss_gatewatcher_gyrokill.cpp:42 enum; Lua
	// mechanar/boss_gatewatcher_gyrokill.lua:61) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_35311_35318_35322.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_MELEE_HASTE bp99 die1
	// tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_SCALE bp19 die1 tgtA TARGET_UNIT_CASTER;
	// cast5 2000ms, dur8 15000ms, range1 Self Only,
	// school Shadow; BonusCoefficient 1 (both
	// effects) not carried; SpellCategories row
	// present (Category 1152, DispelType 1 — not
	// carried); Attributes not carried.
	{id: 35322, castTimeIdx: 5, durIdx: 8, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 99, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 19, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 19492 "Antimagic Pulse" (wago SpellName — MATCH
	// vs expected "Antimagic Pulse"; C++
	// SPELL_ANTIMAGIC_PULSE, boss_garr.cpp:33 enum;
	// Lua moltencore/boss_garr.lua:41) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19492_19496_19497.txt, DifficultyID 0
	// rows): eff0 DISPEL bp0 die1 misc1 radius11 45yd
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur0
	// none, range1 Self Only, school Physical;
	// BonusCoefficient 1 not carried; SpellCategories
	// row present (DefenseType 1 Magic,
	// PreventionType 0x1 — not carried); Attributes
	// not carried.
	{id: 19492, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDispel, dieSides: 1, misc: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 19496 "Magma Shackles" (wago SpellName — MATCH
	// vs expected "Magma Shackles"; C++
	// SPELL_MAGMA_SHACKLES, boss_garr.cpp:34 enum; Lua
	// moltencore/boss_garr.lua:42) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19492_19496_19497.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DECREASE_SPEED bp-61
	// die1 radius11 45yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur8
	// 15000ms, range1 Self Only, school Fire;
	// BonusCoefficient 1 not carried; SpellCategories
	// row present (DefenseType 1 Magic, DispelType 1
	// carried as dispel: 1 per r[2] convention, Mechanic
	// 11, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 19496, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolFire, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -61, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 19497 "Eruption" (wago SpellName — MATCH vs
	// expected "Eruption"; C++ SPELL_ERUPTION,
	// boss_garr.cpp:39 enum; Lua
	// moltencore/boss_garr.lua:43) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19492_19496_19497.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp1849 die301
	// radius18 15yd tgtA TARGET_DEST_CASTER tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY; eff1 KNOCK_BACK
	// bp199 die26 misc200 radius18 15yd tgtA
	// TARGET_DEST_CASTER tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY; cast1 instant, dur0
	// none, range1 Self Only, school Fire;
	// BonusCoefficient 1 (both effects) not carried;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 19497, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1849, dieSides: 301, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 199, dieSides: 26, misc: 200, targetA: targetDestCaster, targetB: targetUnitDestAreaEnemy, radiusIdx: 18, chainAmplitude: 1},
	}},
	// 19716 "Gehennas' Curse" (wago SpellName —
	// MISMATCH vs expected "Gehennas Curse" — apostrophe
	// only; C++ SPELL_GEHENNAS_CURSE,
	// boss_gehennas.cpp:32 enum; Lua
	// moltencore/boss_gehennas.lua:38) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19716_19717_19728.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_HEALING_PCT bp-76
	// die1 misc127 radius11 45yd tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur5 300000ms, range1 Self Only, school Shadow,
	// dispel 2 (Curse); BonusCoefficient 1 not carried;
	// SpellCategories row present (DefenseType 1 Magic,
	// DispelType 2, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 19716, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModHealingPct, basePoints: -76, dieSides: 1, misc: 127, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 19717 "Rain of Fire" (wago SpellName — MATCH
	// vs expected "Rain of Fire"; C++
	// SPELL_RAIN_OF_FIRE, boss_gehennas.cpp:33 enum;
	// Lua moltencore/boss_gehennas.lua:39) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19716_19717_19728.txt, DifficultyID 0
	// rows): eff0 PERSISTENT_AREA_AURA PERIODIC_DAMAGE
	// bp924 die151 period2000 radius13 10yd tgtA
	// TARGET_DEST_DYNOBJ_ENEMY; cast1 instant, dur32
	// 6000ms, range5 Long 40yd, school Fire;
	// BonusCoefficient 0; SpellCategories row present
	// (DefenseType 1 Magic, PreventionType 0x1 — not
	// carried); Attributes not carried.
	{id: 19717, castTimeIdx: 1, durIdx: 32, rangeIdx: 5, school: schoolFire, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, basePoints: 924, dieSides: 151, auraPeriod: 2000, targetA: targetDestDynobjEnemy, radiusIdx: 13, chainAmplitude: 1},
	}},
	// 19728 "Shadow Bolt" (wago SpellName — MATCH
	// vs expected "Shadow Bolt"; C++
	// SPELL_SHADOW_BOLT, boss_gehennas.cpp:34 enum; Lua
	// moltencore/boss_gehennas.lua:40) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19716_19717_19728.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp2249 die501 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast3 500ms, dur0 none,
	// range5 Long 40yd, school Shadow; Speed 36 not
	// carried (no speed field in row());
	// BonusCoefficient 1 not carried; SpellCategories
	// row present (DefenseType 1 Magic, PreventionType
	// 0x1 — not carried); Attributes not carried.
	{id: 19728, castTimeIdx: 3, durIdx: 0, rangeIdx: 5, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 2249, dieSides: 501, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 19702 "Impending Doom" (wago SpellName — MATCH
	// vs expected "Impending Doom"; C++
	// SPELL_IMPENDING_DOOM, boss_lucifron.cpp:32 enum;
	// Lua moltencore/boss_lucifron.lua:31) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19702_19703_20603.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE bp1999
	// die1 period10000 radius11 45yd tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur1
	// 10000ms, range1 Self Only, school Shadow, dispel
	// 1 (Magic); SpellCategories row present
	// (DefenseType 1 Magic, DispelType 1,
	// PreventionType 0x1 — not carried); Attributes
	// not carried.
	{id: 19702, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 1999, dieSides: 1, auraPeriod: 10000, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 11, chainAmplitude: 1},
	}},
	// 19703 "Lucifron's Curse" (wago SpellName —
	// MATCH vs expected "Lucifron's Curse"; C++
	// SPELL_LUCIFRON_CURSE, boss_lucifron.cpp:33 enum;
	// Lua moltencore/boss_lucifron.lua:32) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19702_19703_20603.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_POWER_COST_SCHOOL_PCT
	// bp99 die1 misc127 miscB127 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; RadiusIndex_0 0
	// (RadiusIndex_1 11 = 45yd — recorded, NOT seeded,
	// repo convention: radiusIdx = RadiusIndex_0,
	// 29961/29963 precedent; the _0-vs-_1 question is
	// still open); cast1 instant, dur5 300000ms,
	// range1 Self Only, school Shadow, dispel 2
	// (Curse); SpellCategories row present (DefenseType
	// 1 Magic, DispelType 2, PreventionType 0x1 — not
	// carried); Attributes not carried.
	{id: 19703, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModPowerCostSchoolPct, basePoints: 99, dieSides: 1, misc: 127, miscB: 127, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 20603 "Shadow Shock" (wago SpellName — MATCH
	// vs expected "Shadow Shock"; C++
	// SPELL_SHADOW_SHOCK, boss_lucifron.cpp:34 enum;
	// Lua moltencore/boss_lucifron.lua:33) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_19702_19703_20603.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp739 die121 radius9
	// 20yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur0
	// none, range1 Self Only, school Shadow;
	// SpellCategories row present (DefenseType 1
	// Magic, PreventionType 0x1 — not carried);
	// Attributes not carried.
	{id: 20603, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 739, dieSides: 121, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, radiusIdx: 9, chainAmplitude: 1},
	}},
	// 27680 "Berserk" (wago SpellName — MATCH
	// vs expected "Berserk"; C++
	// SPELL_BERSERK, boss_anubrekhan.cpp:57 enum;
	// Lua northrend/boss_anubrekhan.lua:144) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_27680_28864_29105.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// bp499 die1 misc127 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA MOD_INCREASE_SPEED bp149 die1
	// tgtA TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MOD_SCALE bp29 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur6 600000ms, range1 Self
	// Only, school Physical; SpellCategories row
	// present (Category 21, DispelType 0 — not
	// carried); Attributes not carried.
	{id: 27680, castTimeIdx: 1, durIdx: 6, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 499, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModIncreaseSpeed, basePoints: 149, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 29, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28864 "Summon Corpse Scarabs (10)" (wago
	// SpellName — exact-string MISMATCH vs expected
	// "Summon Corpse Scarabs", verbatim " (10)"
	// suffix only; C++
	// SPELL_SUMMON_CORPSE_SCARABS_MOB,
	// boss_anubrekhan.cpp:56 enum; Lua
	// northrend/boss_anubrekhan.lua:146) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_27680_28864_29105.txt, DifficultyID 0
	// rows): eff0 SUMMON bp9 (=10 summons) die1
	// misc16698 miscB64 radius12 100yd tgtA
	// TARGET_DEST_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Physical; NO
	// SpellCategories row (no dispel); Attributes
	// not carried. IDs+values govern per precedent.
	{id: 28864, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 9, dieSides: 1, misc: 16698, miscB: 64, targetA: targetDestCaster, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 29105 "Summon Corpse Scarabs (5)" (wago
	// SpellName — exact-string MISMATCH vs expected
	// "Summon Corpse Scarabs", verbatim " (5)"
	// suffix only; C++
	// SPELL_SUMMON_CORPSE_SCARABS_PLR,
	// boss_anubrekhan.cpp:55 enum; Lua
	// northrend/boss_anubrekhan.lua:145) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_27680_28864_29105.txt, DifficultyID 0
	// rows): eff0 SUMMON bp4 (=5 summons) die1
	// misc16698 miscB64 radius12 100yd tgtA
	// TARGET_DEST_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Physical; NO
	// SpellCategories row (no dispel); Attributes
	// not carried. IDs+values govern per precedent.
	{id: 29105, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 4, dieSides: 1, misc: 16698, miscB: 64, targetA: targetDestCaster, radiusIdx: 12, chainAmplitude: 1},
	}},
	// 52092 "Temporary Electrical Charge" (wago
	// SpellName — MATCH vs expected; C++
	// SPELL_TEMPORARY_ELECTRICAL_CHARGE,
	// boss_bjarngrim.cpp:66 enum; Lua
	// northrend/boss_bjarngrim.lua:66/80) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_52092_52774_59085.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die0 period1000 trigger52097 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Arcane (0x40); NO
	// SpellCategories row (no dispel); Attributes
	// not carried.
	{id: 52092, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 1000, trigger: 52097, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 52774 "Renew Steel" (wago SpellName — MATCH
	// vs expected; C++ SPELL_RENEW_STEEL_N,
	// boss_bjarngrim.cpp:69 enum; Lua
	// northrend/boss_bjarngrim.lua:342) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_52092_52774_59085.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA OBS_MOD_HEALTH bp9
	// die1 period2000 tgtA TARGET_UNIT_NEARBY_ENTRY;
	// cast5 2000ms, dur32 6000ms, range5 Long Range
	// 0-40yd, school Holy; SpellCategories row
	// present (DispelType 0 — not carried);
	// Attributes not carried.
	{id: 52774, castTimeIdx: 5, durIdx: 32, rangeIdx: 5, school: schoolHoly, effects: [3]effect{
		{effect: effApplyAura, aura: auraObsModHealth, basePoints: 9, dieSides: 1, auraPeriod: 2000, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 59085 "Arc Weld" (wago SpellName — MATCH vs
	// expected; C++ SPELL_ARC_WELD,
	// boss_bjarngrim.cpp:68 enum; Lua
	// northrend/boss_bjarngrim.lua:113) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_52092_52774_59085.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp-1 die1 period250 trigger59086 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur1 10000ms, range5 Long Range 0-40yd,
	// school Nature; NO SpellCategories row (no
	// dispel); Attributes not carried.
	{id: 59085, castTimeIdx: 1, durIdx: 1, rangeIdx: 5, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 250, trigger: 59086, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 55813 "Eck Bite" (wago SpellName — MATCH
	// vs expected "Eck Bite"; C++
	// SPELL_ECK_BITE, boss_eck.cpp:30 enum; Lua
	// northrend/boss_eck.lua:60) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_55813_55814_55816.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE bp149 die1
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 none, range2 Combat Range 0-5yd, school
	// Physical; SpellCategories row present
	// (Category 1152, DefenseType 2 Melee, DispelType
	// 0 — not carried); Attributes not carried.
	{id: 55813, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 149, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 55814 "Eck Spit" (wago SpellName — MATCH
	// vs expected "Eck Spit"; C++
	// SPELL_ECK_SPIT, boss_eck.cpp:31 enum; Lua
	// northrend/boss_eck.lua:61) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_55813_55814_55816.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE bp2969
	// die1 period1000 radius20 25yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; eff1 APPLY_AURA
	// PERIODIC_MANA_LEECH bp219 die1 period1000
	// radius20 25yd tgtA TARGET_UNIT_CONE_ENEMY_24;
	// cast16 1500ms, dur27 3000ms, range34 0-25yd,
	// school Nature; SpellCategories row present
	// (Category 0, DefenseType 1 Magic, DispelType
	// 0 — not carried); EffectBonusCoefficient 0.1
	// (eff0) + Attributes not carried (no field).
	{id: 55814, castTimeIdx: 16, durIdx: 27, rangeIdx: 34, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 2969, dieSides: 1, auraPeriod: 1000, radiusIdx: 20, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicManaLeech, basePoints: 219, dieSides: 1, auraPeriod: 1000, radiusIdx: 20, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 55816 "Eck Berserk" (wago SpellName — MATCH
	// vs expected "Eck Berserk"; C++
	// SPELL_ECK_BERSERK, boss_eck.cpp:29 enum; Lua
	// northrend/boss_eck.lua:62) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_55813_55814_55816.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// bp499 die1 misc127 tgtA TARGET_UNIT_CASTER;
	// eff1 APPLY_AURA MOD_MELEE_HASTE bp149 die1
	// tgtA TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MOD_SCALE bp24 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur5 300000ms, range1 Self
	// Only, school Physical; SpellCategories row
	// present (Category 21, DispelType 0 — not
	// carried); Attributes not carried.
	{id: 55816, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 499, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 149, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 24, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72422 "Quivering Strike" (wago SpellName — MATCH
	// vs expected "Quivering Strike"; C++
	// SPELL_QUIVERING_STRIKE, boss_falric.cpp:35 enum;
	// Lua northrend/boss_falric.lua:183) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_72422_72426_72435.txt, DifficultyID 0
	// base rows): eff0 WEAPON_PERCENT_DAMAGE bp59
	// die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// APPLY_AURA MOD_DODGE_PERCENT bp-21 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur28
	// 5000ms, range2 Combat Range 0-5yd, school
	// Physical, dispel 1 (Magic); DifficultyID 2/4
	// eff0 bp99 NOT seeded (base row only, per
	// precedent); ProcChance 101 / Attributes not
	// carried.
	{id: 72422, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, dispel: 1, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 59, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDodgePercent, basePoints: -21, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 72426 "Impending Despair" (wago SpellName — MATCH
	// vs expected "Impending Despair"; C++
	// SPELL_IMPENDING_DESPAIR, boss_falric.cpp:36
	// enum; Lua northrend/boss_falric.lua:184) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_72422_72426_72435.txt, DifficultyID 0
	// only — no 2/4 variants exist): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL bp0 die0 period6000
	// trigger72428 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur32 6000ms, range36 Longer
	// Range 0-45yd, school Shadow, dispel 1 (Magic);
	// trigger 72428 (stun) unseeded — no verdict row,
	// triage separately; ProcChance 101 / Attributes
	// not carried.
	{id: 72426, castTimeIdx: 1, durIdx: 32, rangeIdx: 36, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 6000, trigger: 72428, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 72435 "Defiling Horror" (wago SpellName — MATCH
	// vs expected "Defiling Horror"; C++
	// SPELL_DEFILING_HORROR, boss_falric.cpp:37 enum;
	// Lua northrend/boss_falric.lua:185) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_72422_72426_72435.txt, DifficultyID 0
	// base rows): eff0 APPLY_AURA MOD_FEAR bp0 die0
	// radius48 60yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 APPLY_AURA
	// PERIODIC_DAMAGE bp2499 die1 period1000 radius48
	// 60yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur35
	// 4000ms, range1 Self Only, school Shadow,
	// mechanic 24 (Horror); DifficultyID 2/4 eff1
	// bp3999 NOT seeded (base row only, per
	// precedent); ProcChance 101 / Attributes not
	// carried.
	{id: 72435, castTimeIdx: 1, durIdx: 35, rangeIdx: 1, school: schoolShadow, mechanic: 24, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: 0, dieSides: 0, radiusIdx: 48, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 2499, dieSides: 1, auraPeriod: 1000, radiusIdx: 48, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 72395 "Hopelessness" (wago SpellName — MATCH
	// vs expected "Hopelessness"; C++
	// SPELL_HOPELESSNESS_1, boss_falric.cpp:38
	// enum; Lua northrend/boss_falric.lua:187
	// SPELL_HOPELESSNESS[1]) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_72395_72396_72397.txt, DifficultyID 0
	// rows): eff0 APPLY_AREA_AURA_ENEMY
	// MOD_DAMAGE_PERCENT_DONE bp-21 die1 misc127
	// radius12 100yd tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AREA_AURA_ENEMY MOD_HEALING_PCT bp-21
	// die1 misc127 radius12 100yd tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Shadow; NO
	// SpellCategories row (no dispel); Attributes /
	// ProcChance 101 not carried.
	{id: 72395, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAreaAuraEnemy, aura: auraModDmgDone, basePoints: -21, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAreaAuraEnemy, aura: auraModHealingPct, basePoints: -21, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72396 "Hopelessness" (wago SpellName — MATCH
	// vs expected "Hopelessness"; C++
	// SPELL_HOPELESSNESS_2, boss_falric.cpp:39
	// enum; Lua northrend/boss_falric.lua:187
	// SPELL_HOPELESSNESS[2]) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_72395_72396_72397.txt, DifficultyID 0
	// rows): eff0 APPLY_AREA_AURA_ENEMY
	// MOD_DAMAGE_PERCENT_DONE bp-41 die1 misc127
	// radius12 100yd tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AREA_AURA_ENEMY MOD_HEALING_PCT bp-41
	// die1 misc127 radius12 100yd tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Shadow; NO
	// SpellCategories row (no dispel); Attributes /
	// ProcChance 101 not carried.
	{id: 72396, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAreaAuraEnemy, aura: auraModDmgDone, basePoints: -41, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAreaAuraEnemy, aura: auraModHealingPct, basePoints: -41, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72397 "Hopelessness" (wago SpellName — MATCH
	// vs expected "Hopelessness"; C++
	// SPELL_HOPELESSNESS_3, boss_falric.cpp:40
	// enum; Lua northrend/boss_falric.lua:187
	// SPELL_HOPELESSNESS[3]) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_72395_72396_72397.txt, DifficultyID 0
	// rows): eff0 APPLY_AREA_AURA_ENEMY
	// MOD_DAMAGE_PERCENT_DONE bp-61 die1 misc127
	// radius12 100yd tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AREA_AURA_ENEMY MOD_HEALING_PCT bp-61
	// die1 misc127 radius12 100yd tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1,
	// range1 Self Only, school Shadow; NO
	// SpellCategories row (no dispel); Attributes /
	// ProcChance 101 not carried.
	{id: 72397, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAreaAuraEnemy, aura: auraModDmgDone, basePoints: -61, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAreaAuraEnemy, aura: auraModHealingPct, basePoints: -61, dieSides: 1, misc: 127, radiusIdx: 12, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28157 "Slime Spray" (wago SpellName — MATCH
	// vs expected "Slime Spray"; C++
	// SPELL_SLIME_SPRAY, boss_grobbulus.cpp:27 enum;
	// Lua northrend/boss_grobbulus.lua:70) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_28157_28169_28240.txt, DifficultyID 0
	// row): eff0 SCHOOL_DAMAGE bp6299 die1401
	// radius11 45yd tgtA TARGET_UNIT_CONE_ENEMY_24;
	// cast1 instant, dur0 none, range37 Extended
	// Range 0-50yd, school Nature; NO SpellCategories
	// row (no dispel); EffectAttributes 0x20000 not
	// carried.
	{id: 28157, castTimeIdx: 1, durIdx: 0, rangeIdx: 37, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 6299, dieSides: 1401, radiusIdx: 11, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 28169 "Mutating Injection" (wago SpellName — MATCH
	// vs expected "Mutating Injection"; C++
	// SPELL_MUTATING_INJECTION, boss_grobbulus.cpp:28
	// enum; Lua northrend/boss_grobbulus.lua:71) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28157_28169_28240.txt, DifficultyID 0
	// row): eff0 APPLY_AURA DUMMY bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur1
	// 10000ms, range37 Extended Range 0-50yd, school
	// Physical, dispel 3 (Disease).
	{id: 28169, castTimeIdx: 1, durIdx: 1, rangeIdx: 37, school: schoolPhysical, dispel: 3, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 28240 "Poison Cloud" (wago SpellName — MATCH
	// vs expected "Poison Cloud"; C++
	// SPELL_POISON_CLOUD, boss_grobbulus.cpp:30 enum;
	// Lua northrend/boss_grobbulus.lua:72) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_28157_28169_28240.txt, DifficultyID 0
	// row): eff0 SUMMON bp0 die1 misc16363 miscB64
	// tgtA TARGET_DEST_CASTER; cast1 instant, dur62
	// 75000ms (new aux row), range1 Self Only,
	// school Physical; NO SpellCategories row (no
	// dispel); Attributes not carried.
	{id: 28240, castTimeIdx: 1, durIdx: 62, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 0, dieSides: 1, misc: 16363, miscB: 64, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 28158 "Poison" (wago SpellName — name MISMATCH
	// vs expected "Poison Cloud", name-only; C++
	// SPELL_POISON_CLOUD_PASSIVE, boss_grobbulus.cpp:30
	// enum; Lua northrend/boss_grobbulus.lua:73) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28158_54362.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die0 period1000 trigger28241 misc1 tgtA
	// TARGET_UNIT_CASTER; eff1 DUMMY bp0 die0 misc1
	// radius10 30yd tgtA TARGET_DEST_CASTER; cast1
	// instant, dur3 60000ms, range1 Self Only, school
	// Nature; NO SpellCategories row (no dispel);
	// trigger 28241 seeded below
	// (wago_spells_28241_54363.txt); Attributes /
	// ProcChance not carried.
	{id: 28158, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 1000, misc: 1, trigger: 28241, targetA: targetCaster, chainAmplitude: 1},
		{effect: effDummy, basePoints: 0, dieSides: 0, misc: 1, radiusIdx: 10, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 54362 "Poison" (wago SpellName — name MISMATCH
	// vs expected "Poison Cloud", name-only; C++
	// SPELL_POISON_CLOUD_PASSIVE heroic,
	// boss_grobbulus.cpp:30 enum; Lua
	// northrend/boss_grobbulus.lua:218) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_28158_54362.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// bp0 die0 period1000 trigger54363 misc1 tgtA
	// TARGET_UNIT_CASTER; eff1 DUMMY bp0 die0 misc1
	// radius10 30yd tgtA TARGET_DEST_CASTER; cast1
	// instant, dur3 60000ms, range1 Self Only, school
	// Nature; NO SpellCategories row (no dispel);
	// trigger 54363 seeded below
	// (wago_spells_28241_54363.txt); SpellAuraOptions
	// ProcChance 101 row exists (not carried).
	{id: 54362, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 1000, misc: 1, trigger: 54363, targetA: targetCaster, chainAmplitude: 1},
		{effect: effDummy, basePoints: 0, dieSides: 0, misc: 1, radiusIdx: 10, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 28241 "Poison" (wago SpellName; C++
	// SPELL_POISON_CLOUD trigger, boss_grobbulus.cpp
	// enum; Lua northrend/boss_grobbulus.lua:73
	// poisonCloudTick) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16 verdict
	// file wago_spells_28241_54363.txt, DifficultyID 0
	// rows; re-read CORRECTS prior verdict misc 1→0,
	// verified by SpellEffect+misc filter test):
	// eff0 SCHOOL_DAMAGE bp2774 die451 misc0
	// radius10 30yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0, range1 Self Only, school Nature; NO
	// SpellCategories dispel (0); SpellAuraOptions
	// NO row; Attributes / ProcChance not carried.
	{id: 28241, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 2774, dieSides: 451, radiusIdx: 10, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 54363 "Poison" (wago SpellName; C++
	// SPELL_POISON_CLOUD trigger heroic,
	// boss_grobbulus.cpp enum; Lua
	// northrend/boss_grobbulus.lua:218) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_28241_54363.txt, DifficultyID 0
	// rows; re-read CORRECTS prior verdict misc 1→0,
	// verified by SpellEffect+misc filter test):
	// eff0 SCHOOL_DAMAGE bp3699 die601 misc0
	// radius10 30yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0, range1 Self Only, school Nature; NO
	// SpellCategories dispel (0); SpellAuraOptions
	// ProcChance 101 row exists (not carried).
	{id: 54363, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 3699, dieSides: 601, radiusIdx: 10, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 57941 "Mind Flay" (wago SpellName MATCHES; C++
	// SPELL_MIND_FLAY, boss_herald_volazj.cpp:37;
	// Lua northrend/boss_herald_volazj.lua:120) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_57941_57942_57949.txt, DifficultyID
	// 0 rows): eff0 APPLY_AURA PERIODIC_DAMAGE
	// bp2827 die345 period1000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// MOD_DECREASE_SPEED bp-51 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur27
	// 3000ms, range3 20yd, school Shadow, mechanic
	// 11 (SpellCategories, 72435 precedent);
	// SpellCategories dispel 0; EffectBonusCoefficient
	// 0.3 / ProcChance 101 not carried.
	{id: 57941, castTimeIdx: 1, durIdx: 27, rangeIdx: 3, school: schoolShadow, mechanic: 11, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 2827, dieSides: 345, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -51, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 57942 "Shadow Bolt Volley" (wago SpellName
	// MATCHES; C++ SPELL_SHADOW_BOLT_VOLLEY,
	// boss_herald_volazj.cpp:38; Lua
	// northrend/boss_herald_volazj.lua:121) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_57941_57942_57949.txt, DifficultyID
	// 0 rows): eff0 SCHOOL_DAMAGE bp849 die301
	// radius48 60yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0, range14 60yd (new aux row), school
	// Shadow; SpellCategories dispel 0;
	// EffectAmplitude 1 / EffectBonusCoefficient 1 /
	// Speed 20 / ProcChance 101 not carried.
	{id: 57942, castTimeIdx: 1, durIdx: 0, rangeIdx: 14, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 849, dieSides: 301, radiusIdx: 48, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 57949 "Shiver" (wago SpellName MATCHES; C++
	// SPELL_SHIVER, boss_herald_volazj.cpp:39; Lua
	// northrend/boss_herald_volazj.lua:122) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_57941_57942_57949.txt, DifficultyID
	// 0 rows): eff0 APPLY_AURA RAID_PROC_FROM_CHARGE
	// (aura 223; Wago label "REMOVE_BARBER_SHOP_COST"
	// is misaligned, value governs) bp0 die0
	// radius9 20yd tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur8 15000ms, range13 50000yd,
	// school Shadow, dispel 2 Curse; description
	// references 57952 but no second effect row and
	// trigger 0 — NOT seeded; ProcChance 100 /
	// ProcCharges 5 / Speed 40 not carried.
	{id: 57949, castTimeIdx: 1, durIdx: 8, rangeIdx: 13, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraRaidProcFromCharge, basePoints: 0, dieSides: 0, radiusIdx: 9, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 28131 "Frenzy" (wago SpellName MATCHES; C++
	// SPELL_FRENZY, boss_patchwerk.cpp:26; Lua
	// northrend/boss_patchwerk.lua:68) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28131_28308_32309.txt, DifficultyID
	// 0 rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// (aura 79) bp24 die1 misc127 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp39 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE
	// (aura 61) bp24 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur5 300000ms, range1 Self Only,
	// school Physical; SpellCategories Category 21 /
	// dispel 0; Attributes_0 0x40010 not carried
	// (no attributes field in the seed struct).
	{id: 28131, castTimeIdx: 1, durIdx: 5, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 24, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 39, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 24, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28308 "Hateful Strike" (wago SpellName MATCHES;
	// C++ SPELL_HATEFUL_STRIKE, boss_patchwerk.cpp:25;
	// Lua northrend/boss_patchwerk.lua:67) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28131_28308_32309.txt, DifficultyID
	// 0 rows): eff0 SCHOOL_DAMAGE bp19974 die7051
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0, range2 Combat 5yd, school Physical;
	// SpellCategories DefenseType 2 Melee / dispel 0;
	// Attributes_3 0x200 / Attributes_5 0x80000 not
	// carried (no attributes field in the seed struct).
	{id: 28308, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 19974, dieSides: 7051, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 32309 "Slime Bolt" (wago SpellName MATCHES; C++
	// SPELL_SLIME_BOLT, boss_patchwerk.cpp:28; Lua
	// northrend/boss_patchwerk.lua:70) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28131_28308_32309.txt, DifficultyID
	// 0 rows): eff0 SCHOOL_DAMAGE bp8324 die1351
	// radius24 65yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 APPLY_AURA
	// PERIODIC_DAMAGE bp5699 die601 period3000
	// radius24 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur32 6000ms, range37 Extended 50yd, school
	// Nature, dispel 4 Poison; SpellAuraOptions
	// ProcChance 101 / Speed 40 / BonusCoefficient
	// 1 / 0.1 not carried.
	{id: 32309, castTimeIdx: 1, durIdx: 32, rangeIdx: 37, school: schoolNature, dispel: 4, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 8324, dieSides: 1351, radiusIdx: 24, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 5699, dieSides: 601, auraPeriod: 3000, radiusIdx: 24, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 50770 "Magic Pull" (wago SpellName MATCHES; C++
	// SPELL_MAGIC_PULL, boss_xevozz.cpp:54; Lua
	// northrend/boss_xevozz.lua:119,369 — dummy aura
	// casts it triggered on the hit target) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_50770_54202_54226.txt, DifficultyID
	// 0 rows): eff0 TELEPORT_UNITS_FACE_CASTER
	// (effect 43) bp0 die0 radius37 7yd tgtA
	// TARGET_UNIT_TARGET_ANY tgtB
	// TARGET_DEST_CASTER_RADIUS; cast1 instant,
	// dur0, range13 Anywhere-Unlimited 50000yd,
	// school Physical; SpellCategories NO ROW
	// (dispel 0 / mechanic 0 defaults); radiusIdx_1
	// 36 (0yd) not carried (seed struct carries
	// only _0, cf. radius-40 precedent);
	// SpellAuraOptions ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 50770, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnitsFaceCaster, basePoints: 0, dieSides: 0, radiusIdx: 37, targetA: targetUnitTargetAny, targetB: targetDestCasterRadius, chainAmplitude: 1},
	}},
	// 54202 "Arcane Barrage Volley" (wago SpellName
	// MATCHES; C++ SPELL_ARCANE_BARRAGE_VOLLEY,
	// boss_xevozz.cpp:34; Lua
	// northrend/boss_xevozz.lua:116,188 — self-cast
	// DoCastAOE) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_50770_54202_54226.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp1757 die285 radius11 45yd tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0, range3 Twenty yards, school Arcane;
	// SpellCategories Category 1189 / DefenseType 1
	// Magic / PreventionType 0x1 / StartRecovery
	// 133 / Speed 18 / ProcChance 101 not carried
	// (no fields in the seed struct).
	{id: 54202, castTimeIdx: 1, durIdx: 0, rangeIdx: 3, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1757, dieSides: 285, radiusIdx: 11, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 54226 "Arcane Buffet" (wago SpellName MATCHES;
	// C++ SPELL_ARCANE_BUFFET, boss_xevozz.cpp:35;
	// Lua northrend/boss_xevozz.lua:117,206 — cast
	// on target) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16
	// verdict file wago_spells_50770_54202_54226.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp878 die143 tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff1 APPLY_AURA MOD_DAMAGE_TAKEN (aura 14)
	// bp149 die1 misc64 Arcane school tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur18 20000ms, range36 Longer Range 45yd,
	// school Arcane, dispel 1 Magic; eff0
	// BonusCoefficient 1 / SpellAuraOptions
	// CumulativeAura 10 / ProcChance 101 not
	// carried (no fields in the seed struct).
	{id: 54226, castTimeIdx: 1, durIdx: 18, rangeIdx: 36, school: schoolArcane, dispel: 1, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 878, dieSides: 143, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageTaken, basePoints: 149, dieSides: 1, misc: 64, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 72144 "Orange Blight Residue" (wago SpellName
	// MATCHES; C++ SPELL_ORANGE_BLIGHT_RESIDUE,
	// icecrown_citadel.h:41; Lua
	// northrend/npc_alchemist_adrianna.lua:23,29 —
	// player:HasAura gate) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_72144_72145_72155.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp-1 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur30 1800000ms, range1 Self
	// Only, school Physical; SpellCategories NO ROW
	// (dispel 0 / mechanic 0 defaults);
	// SpellAuraOptions ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 72144, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72145 "Green Blight Residue" (wago SpellName
	// MATCHES; C++ SPELL_GREEN_BLIGHT_RESIDUE,
	// icecrown_citadel.h:42; Lua
	// northrend/npc_alchemist_adrianna.lua:24,29 —
	// player:HasAura gate) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_72144_72145_72155.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp-1 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur30 1800000ms, range1 Self
	// Only, school Physical; SpellCategories NO ROW
	// (dispel 0 / mechanic 0 defaults);
	// SpellAuraOptions ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 72145, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 72155 "Harvest Blight Specimen" (wago SpellName
	// MATCHES; C++ SPELL_HARVEST_BLIGHT_SPECIMEN,
	// icecrown_citadel.cpp:88,553-555; Lua
	// northrend/npc_alchemist_adrianna.lua:25,30 —
	// creature:CastSpell self-cast, 3s cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_72144_72145_72155.txt,
	// DifficultyID 0 rows): eff0 SCRIPT_EFFECT
	// bp72143 die1 radius28 50000yd tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY; eff1
	// QUEST_COMPLETE bp72144 die1 misc24873
	// radius28 tgtA/tgtB same; cast14 3000ms,
	// dur0, range1 Self Only, school Physical;
	// SpellCategories NO ROW (dispel 0 defaults);
	// heroic DifficultyID 2/4 rows NOT carried
	// (seed takes the DifficultyID 0 row; heroic
	// differs only in QUEST_COMPLETE misc 24878
	// vs 24873); ProcChance 101 / Attributes not
	// carried (no fields in the seed struct).
	{id: 72155, castTimeIdx: 14, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effScriptEffect, basePoints: 72143, dieSides: 1, radiusIdx: 28, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
		{effect: effQuestComplete, basePoints: 72144, dieSides: 1, misc: 24873, radiusIdx: 28, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
	}},
	// 43931 "Rend" (wago SpellName MATCHES; C++
	// SPELL_REND, utgarde_keep.cpp:253; Lua
	// northrend/npc_enslaved_proto_drake.lua:12,49
	// — victim-cast on the rend timer) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_43931_49722_50653.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_DAMAGE (aura 3) bp42 die1 period
	// 3000ms tgtA TARGET_UNIT_TARGET_ENEMY,
	// mechanic 15 (spell + effect); cast1 instant,
	// dur8 15000ms, range2 Combat 0-5yd, school
	// Physical; SpellCategories dispel 0;
	// ProcChance 101 / Attributes / DefenseType /
	// PreventionType not carried (no fields in the
	// seed struct).
	{id: 43931, castTimeIdx: 1, durIdx: 8, rangeIdx: 2, school: schoolPhysical, mechanic: 15, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 42, dieSides: 1, auraPeriod: 3000, mechanic: 15, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 49722 "Knock Away" (wago SpellName MATCHES;
	// C++ SPELL_KNOCK_AWAY, utgarde_keep.cpp:255;
	// Lua northrend/npc_enslaved_proto_drake.lua:14,
	// 79 — victim-cast on the knockaway timer) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_43931_49722_50653.txt,
	// DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp29 die1 radius13 10yd tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; eff1
	// KNOCK_BACK bp112 die75 misc75 radius13
	// tgtA/tgtB same; cast1 instant, dur0, range7
	// Ten yards 0-10yd, school Physical;
	// SpellCategories dispel 0 / mechanic 0;
	// BonusCoefficient 1 / ProcChance 101 /
	// Attributes not carried (no fields in the
	// seed struct).
	{id: 49722, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 29, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 112, dieSides: 75, misc: 75, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 50653 "Flame Breath" (wago SpellName MATCHES;
	// C++ SPELL_FLAME_BREATH, utgarde_keep.cpp:254;
	// Lua northrend/npc_enslaved_proto_drake.lua:13,
	// 64 — victim-cast on the flame timer) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_43931_49722_50653.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp1699 die601 radius18 15yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; cast1 instant,
	// dur0, range2 Combat 0-5yd, school Fire;
	// SpellCategories dispel 0 / mechanic 0,
	// Category 50 / StartRecoveryCategory 133 not
	// carried (no fields in the seed struct);
	// BonusCoefficient 1 / EffectAttributes 0x20000
	// / ProcChance 101 / Attributes not carried.
	{id: 50653, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1699, dieSides: 601, radiusIdx: 18, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 28819 "Submerge Visual" (wago SpellName MATCHES;
	// C++ SPELL_SUBMERGE_VISUAL,
	// boss_reliquary_of_souls.cpp:59; Lua
	// blacktemple/boss_reliquary_of_souls.lua:167,
	// 218 — triggered self-cast on EVENT_SUBMERGE,
	// C++ cpp:273 DoCastSelf triggered) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28819_41535_41542.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die0 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur21 -1 infinite (aux row
	// pre-existing), range13 Anywhere 0-50000yd
	// (aux row pre-existing), school Physical;
	// NO SpellCategories row (mechanic 0, dispel
	// 0), NO SpellAuraOptions row; BonusCoefficient
	// 1 / Attributes not carried (no fields in the
	// seed struct).
	{id: 28819, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 41535 "Enslaved Soul Passive" (wago SpellName
	// MATCHES; C++ SPELL_ENSLAVED_SOUL_PASSIVE,
	// boss_reliquary_of_souls.cpp:83; Lua
	// blacktemple/boss_reliquary_of_souls.lua:172,
	// 322 — triggered self-cast in soulReset,
	// C++ cpp:599 DoCastSelf triggered) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_28819_41535_41542.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die0 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur21 -1 infinite (aux row
	// pre-existing), range1 Self Only 0yd (aux row
	// pre-existing), school Physical; NO
	// SpellCategories row (mechanic 0, dispel 0);
	// ProcChance 101 / Attributes_5 0x8 /
	// BonusCoefficient 0 not carried (no fields in
	// the seed struct).
	{id: 41535, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 41542 "Soul Release" (wago SpellName MATCHES;
	// C++ SPELL_SOUL_RELEASE,
	// boss_reliquary_of_souls.cpp:84; Lua
	// blacktemple/boss_reliquary_of_souls.lua:173,
	// 376 — non-triggered self-cast on soul death,
	// C++ cpp:620 DoCastSelf) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_28819_41535_41542.txt,
	// DifficultyID 0 rows): eff0 HEAL_PCT bp29
	// die1 radius13 10yd (aux row pre-existing)
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1
	// ENERGIZE_PCT bp29 die1 radius13 tgtA/tgtB
	// same (stored bp29 = in-game 30); cast1
	// instant, dur0 (no SpellDuration row), range1
	// Self Only 0yd, school Holy; NO SpellCategories
	// row (mechanic 0, dispel 0); ProcChance 101 /
	// Attributes not carried (no fields in the seed
	// struct).
	{id: 41542, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolHoly, effects: [3]effect{
		{effect: effHealPct, basePoints: 29, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effEnergizePct, basePoints: 29, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 29426 "Heroic Strike" (wago SpellName MATCHES;
	// C++ SPELL_HEROIC_STRIKE,
	// halls_of_reflection.cpp:2418; Lua
	// northrend/npc_quel_delar_sword.lua:94,
	// 171 — victim-cast, re-arm 6000ms, C++
	// cpp:2705 DoCastVictim) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_29426_67541_67716.txt,
	// DifficultyID 0 rows): eff0 WEAPON_DAMAGE
	// bp19 die5 tgtA TARGET_UNIT_TARGET_ENEMY
	// (in-game +20); cast1 instant, dur0 (no
	// SpellDuration row), range2 Combat 0-5yd
	// (aux row pre-existing), school Physical;
	// NO SpellAuraOptions row (mechanic 0,
	// dispel 0); BonusCoefficient 1 not carried
	// (no field in the seed struct).
	{id: 29426, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 19, dieSides: 5, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 67541 "Bladestorm" (wago SpellName MATCHES;
	// C++ SPELL_BLADESTORM,
	// halls_of_reflection.cpp:2420; Lua
	// northrend/npc_quel_delar_sword.lua:93,
	// 162 — self-cast, re-arm 10000ms, C++
	// cpp:2701 DoCast) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16
	// verdict file wago_spells_29426_67541_67716.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp-1 die1
	// period1000 trigger 67716 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MECHANIC_IMMUNITY_MASK (aura 147) bp-1
	// die1 misc 1733 tgtA caster; eff2 APPLY_AURA
	// ALLOW_ONLY_ABILITY (aura 263, wago name
	// DISABLE_CASTING_EXCEPT_ABILITIES) bp-1 die1
	// mask_1 4 tgtA caster; cast1 instant, dur32
	// 6000ms (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Physical; ProcChance 101 / Attributes not
	// carried (no fields in the seed struct).
	{id: 67541, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 1000, trigger: 67716, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunityMask, basePoints: -1, dieSides: 1, misc: 1733, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraAllowOnlyAbility, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1, mask: [3]uint32{0, 4, 0}},
	}},
	// 67716 "Whirlwind" (wago SpellName MATCHES;
	// C++ SPELL_WHIRLWIND,
	// halls_of_reflection.cpp:2419; Lua
	// northrend/npc_quel_delar_sword.lua:95,
	// 180 — DoCastAOE self-cast, re-arm 1000ms,
	// C++ cpp:2709 DoCastAOE; also the
	// Bladestorm periodic trigger) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_29426_67541_67716.txt,
	// DifficultyID 0 rows): eff0
	// NORMALIZED_WEAPON_DMG bp-1 die1 radius52
	// 5yd (aux row added) tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast1
	// instant, dur0 (no SpellDuration row),
	// range1 Self Only 0yd (aux row
	// pre-existing), school Physical; NO
	// SpellAuraOptions row (mechanic 0, dispel
	// 0); BonusCoefficient 1 not carried (no
	// field in the seed struct).
	{id: 67716, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effNormalizedWeaponDmg, basePoints: -1, dieSides: 1, radiusIdx: 52, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 50756 "Unstable Sphere Passive" (wago
	// SpellName MATCHES; C++ SPELL_UNSTABLE_SPHERE_PASSIVE,
	// boss_drakos.cpp:28; Lua
	// northrend/npc_unstable_sphere.lua:22,
	// 68 — AddAura self) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_50756_50757_50758.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die0 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur387 16000ms (aux row
	// added), range1 Self Only 0yd (aux row
	// pre-existing), school Arcane; NO
	// SpellCategories row (mechanic 0, dispel 0);
	// ProcChance 101 / Attributes not carried
	// (no fields in the seed struct).
	{id: 50756, castTimeIdx: 1, durIdx: 387, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50757 "Unstable Sphere Pulse" (wago
	// SpellName MATCHES; C++ SPELL_UNSTABLE_SPHERE_PULSE,
	// boss_drakos.cpp:29; Lua
	// northrend/npc_unstable_sphere.lua:24,
	// 55 — self-cast, repeat) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_50756_50757_50758.txt,
	// DifficultyID 0 rows): eff0 DUMMY bp0 die0
	// tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Arcane; NO SpellCategories row (mechanic
	// 0, dispel 0); ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 50757, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effDummy, basePoints: 0, dieSides: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50758 "Unstable Sphere Timer" (wago
	// SpellName MATCHES; C++ SPELL_UNSTABLE_SPHERE_TIMER,
	// boss_drakos.cpp:30; Lua
	// northrend/npc_unstable_sphere.lua:23,
	// 69 — AddAura self) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_50756_50757_50758.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp0 die0
	// period17000 trigger 50759 tgtA
	// TARGET_UNIT_CASTER (fires once at 17s of
	// the 18s aura; 50759 trigger-only, NOT a
	// live-cast ID, not seeded); cast1 instant,
	// dur85 18000ms (aux row pre-existing,
	// value now wago-confirmed), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Arcane; NO SpellCategories row (mechanic
	// 0, dispel 0); ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 50758, castTimeIdx: 1, durIdx: 85, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 17000, trigger: 50759, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 57912 "Arcane Lightning" (wago SpellName is
	// "Arcane Lightning" — the "Damage" suffix is
	// a C++/Lua label, not the stored name; C++
	// SPELL_ARCANE_LIGHTNING_DAMAGE,
	// violet_hold.cpp:129; Lua
	// northrend/npc_violet_hold_defense_system.lua:19,
	// 27 — self-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_57912_57930_58152.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp12999 die1 (in-game 13000) radius12
	// 100yd (aux row pre-existing) tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Arcane; NO SpellCategories row (mechanic
	// 0, dispel 0); ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 57912, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 12999, dieSides: 1, radiusIdx: 12, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
	}},
	// 57930 "Arcane Lightning" (wago SpellName is
	// "Arcane Lightning" — the "Dummy" suffix is
	// a C++/Lua label, not the stored name; C++
	// SPELL_ARCANE_LIGHTNING_DUMMY,
	// violet_hold.cpp:131; Lua
	// northrend/npc_violet_hold_defense_system.lua:20,
	// 28 — self-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_57912_57930_58152.txt,
	// DifficultyID 0 rows): eff0 DUMMY bp0 die1
	// radius12 100yd (aux row pre-existing) tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Arcane; NO SpellCategories row (mechanic
	// 0, dispel 0); ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 57930, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effDummy, basePoints: 0, dieSides: 1, radiusIdx: 12, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
	}},
	// 58152 "Arcane Lightning" (wago SpellName is
	// "Arcane Lightning" — the "Instakill" suffix
	// is a C++/Lua label, not the stored name;
	// C++ SPELL_ARCANE_LIGHTNING_INSTAKILL,
	// violet_hold.cpp:130; Lua
	// northrend/npc_violet_hold_defense_system.lua:21,
	// 30 — self-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_57912_57930_58152.txt,
	// DifficultyID 0 rows): eff0 INSTAKILL bp0
	// die1 radius12 100yd (aux row pre-existing)
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Arcane; NO SpellCategories row (mechanic
	// 0, dispel 0); ProcChance 101 / Attributes
	// not carried (no fields in the seed struct).
	{id: 58152, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolArcane, effects: [3]effect{
		{effect: effInstakill, basePoints: 0, dieSides: 1, radiusIdx: 12, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
	}},
	// 58008 "Portal Periodic" (wago SpellName
	// MATCHES; C++ SPELL_PORTAL_PERIODIC,
	// violet_hold.cpp:152; Lua
	// northrend/violet_hold.lua:79, 376 —
	// triggered self-cast) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_58008_58012_62139.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp0 die0
	// period15000 trigger 0 (stored 0 despite
	// the aura — the periodic leg fires via the
	// Lua-side 58008 handler, not a DBC
	// trigger) tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur0 (no SpellDuration row),
	// range1 Self Only 0yd (aux row
	// pre-existing), school Physical; NO
	// SpellCategories row (mechanic 0, dispel
	// 0); ProcChance 101 / Attributes not
	// carried (no fields in the seed struct).
	{id: 58008, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 15000, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 58012 "Portal Channel" (wago SpellName
	// MATCHES; C++ SPELL_PORTAL_CHANNEL,
	// violet_hold.cpp:154; Lua
	// northrend/violet_hold.lua:80, 189 —
	// cast on keeper) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_58008_58012_62139.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ANY; cast1 instant,
	// dur21 -1 infinite (aux row pre-existing),
	// range13 Anywhere 0-50000yd (aux row
	// pre-existing), school Arcane; NO
	// SpellCategories row (mechanic 0, dispel
	// 0); ProcChance 101 / Attributes not
	// carried (no fields in the seed struct).
	{id: 58012, castTimeIdx: 1, durIdx: 21, rangeIdx: 13, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 0, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 62139 "Teleport to Inside Violet Hold"
	// (wago SpellName is "Teleport to Inside
	// Violet Hold" — the Lua/C++ label
	// "Teleport Player Effect" is NOT the stored
	// name; C++ SPELL_TELEPORT_PLAYER_EFFECT,
	// violet_hold.cpp:181; Lua
	// northrend/violet_hold.lua:83, 452 —
	// triggered self-cast on hit unit) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_58008_58012_62139.txt,
	// DifficultyID 0 rows): eff0 TELEPORT_UNITS
	// bp0 die0 tgtA TARGET_UNIT_CASTER tgtB
	// TARGET_DEST_DB; cast1 instant, dur0 (no
	// SpellDuration row), range4 Medium 0-30yd
	// (aux row pre-existing), school Physical;
	// NO SpellCategories row (mechanic 0,
	// dispel 0); BonusCoefficient 1 / ProcChance
	// 101 / Attributes not carried (no fields
	// in the seed struct).
	{id: 62139, castTimeIdx: 1, durIdx: 0, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, basePoints: 0, dieSides: 0, targetA: targetCaster, targetB: targetDestDB, chainAmplitude: 1},
	}},
	// 15496 "Cleave" (wago SpellName MATCHES;
	// C++ SPELL_CLEAVE, black_temple.cpp:28
	// (also herod/hexlord/violet_hold); Lua
	// blacktemple/npc_wrathbone_flayer.lua:41,
	// 134 — victim-cast, re-arm {1s,2s}) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15496_39544_40094.txt,
	// DifficultyID 0 rows): eff0
	// WEAPON_PERCENT_DAMAGE bp109 die1
	// (in-game 110%) chainTargets 2 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range2 Combat
	// 0-5yd (aux row pre-existing), school
	// Physical; SpellCategories row (DefenseType
	// 2 Melee, mechanic 0, dispel 0); NO
	// SpellAuraOptions row; Attributes not
	// carried (no fields in the seed struct).
	{id: 15496, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 109, dieSides: 1, targetA: targetEnemy, chainTargets: 2, chainAmplitude: 1},
	}},
	// 39544 "Ignored" (wago SpellName MATCHES;
	// C++ SPELL_IGNORED, black_temple.cpp:29;
	// Lua blacktemple/npc_wrathbone_flayer.lua:42,
	// 142 — random-target cast, re-arm 10s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15496_39544_40094.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DETAUNT (aura 221) bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur28 5000ms (aux row pre-existing),
	// range2 Combat 0-5yd (aux row
	// pre-existing), school Physical;
	// SpellCategories row (DefenseType 0,
	// mechanic 0, dispel 0); ProcChance 101 /
	// Attributes not carried (no fields in the
	// seed struct).
	{id: 39544, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDetaunt, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 40094 "Summon Channel" (wago SpellName
	// MATCHES; C++ SPELL_SUMMON_CHANNEL,
	// black_temple.cpp:30; Lua
	// blacktemple/npc_wrathbone_flayer.lua:43,
	// 94 — cast on channel target, re-arms 12s)
	// — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15496_39544_40094.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die1 tgtA
	// TARGET_UNIT_NEARBY_ENTRY; cast1 instant,
	// dur1 10000ms (aux row pre-existing),
	// range11 Fifteen yards 0-15yd (aux row
	// pre-existing), school Physical; NO
	// SpellCategories row (mechanic 0, dispel
	// 0); ProcChance 101 / Attributes not
	// carried (no fields in the seed struct).
	{id: 40094, castTimeIdx: 1, durIdx: 1, rangeIdx: 11, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 1, targetA: targetUnitNearbyEntry, chainAmplitude: 1},
	}},
	// 22539 "Shadow Flame" (wago SpellName is
	// "Shadow Flame", two words — NOT
	// "Shadowflame"; C++ SPELL_SHADOWFLAME,
	// boss_ebonroc.cpp:24; Lua
	// blackwinglair/boss_ebonroc.lua:44, 83 —
	// victim-cast, {10s,20s}) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_22539_23339_23340.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp3937 die1125 (in-game 3938-5063)
	// radius11 45yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; cast5 2000ms
	// (aux row pre-existing), dur0 (no
	// SpellDuration row), range36 Longer 0-45yd
	// (aux row pre-existing), school Shadow; NO
	// SpellAuraOptions row; BonusCoefficient 1 /
	// Attributes not carried (no fields in the
	// seed struct).
	{id: 22539, castTimeIdx: 5, durIdx: 0, rangeIdx: 36, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 3937, dieSides: 1125, radiusIdx: 11, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 23339 "Wing Buffet" (wago SpellName
	// MATCHES; C++ SPELL_WINGBUFFET,
	// boss_ebonroc.cpp:25; Lua
	// blackwinglair/boss_ebonroc.lua:45, 101 —
	// victim-cast, 30s) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16
	// verdict file wago_spells_22539_23339_23340.txt,
	// DifficultyID 0 rows): eff0 KNOCK_BACK
	// bp149 die1 misc 250 radius9 20yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; eff1
	// SCHOOL_DAMAGE bp562 die375 (in-game
	// 563-938) radius9 tgtA cone; cast4 1000ms
	// (aux row pre-existing), dur0 (no
	// SpellDuration row), range1 Self Only 0yd
	// (aux row pre-existing), school Physical;
	// NO SpellAuraOptions row; BonusCoefficient
	// 1 / Attributes not carried (no fields in
	// the seed struct).
	{id: 23339, castTimeIdx: 4, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effKnockBack, basePoints: 149, dieSides: 1, misc: 250, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 562, dieSides: 375, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 23340 "Shadow of Ebonroc" (wago SpellName
	// MATCHES; C++ SPELL_SHADOWOFEBONROC,
	// boss_ebonroc.cpp:26; Lua
	// blackwinglair/boss_ebonroc.lua:46, 119 —
	// victim-cast, {8s,10s}) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_22539_23339_23340.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PROC_TRIGGER_SPELL (aura 42) bp0 die1
	// trigger 23394 tgtA TARGET_UNIT_TARGET_ENEMY
	// (23394 trigger-only, NOT a live-cast ID,
	// not seeded); cast1 instant, dur31 8000ms
	// (aux row pre-existing), range3 Twenty yards
	// 0-20yd (aux row pre-existing), school
	// Shadow; ProcChance 100 / ProcTypeMask 0x28
	// / Attributes not carried (no fields in the
	// seed struct).
	{id: 23340, castTimeIdx: 1, durIdx: 31, rangeIdx: 3, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, basePoints: 0, dieSides: 1, trigger: 23394, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 22425 "Fireball Volley" (wago SpellName
	// MATCHES; C++ SPELL_FIREBALLVOLLEY,
	// boss_razorgore.cpp:44; Lua
	// blackwinglair/boss_razorgore.lua:102, 164 —
	// victim-cast, re-arm {12s,15s}) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_22425_22540_23023.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp254 die91 (in-game 255-345) radius11
	// 45yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast5 2000ms
	// (aux row pre-existing), dur0 (no
	// SpellDuration row), range5 Long Range 0-40yd
	// (aux row pre-existing), school Fire; NO
	// SpellAuraOptions row; DefenseType 1 /
	// PreventionType 0x1 / Attributes_0 0x90000 /
	// Attributes_1 0x88 / BonusCoefficient 1 not
	// carried (no fields in the seed struct).
	{id: 22425, castTimeIdx: 5, durIdx: 0, rangeIdx: 5, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 254, dieSides: 91, radiusIdx: 11, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 23023 "Conflagration" (wago SpellName
	// MATCHES; C++ SPELL_CONFLAGRATION,
	// boss_razorgore.cpp:45; Lua
	// blackwinglair/boss_razorgore.lua:103, 191 —
	// victim-cast, re-arm 30s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_22425_22540_23023.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_DAMAGE (aura 3) bp299 die1 (in-game
	// 300/tick) auraPeriod 1000 radius10 30yd tgtA
	// TARGET_UNIT_CONE_ENEMY_24; eff1 APPLY_AURA
	// MOD_CONFUSE (aura 5) bp0 die1 radius10 tgtA
	// cone; eff2 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL_WITH_VALUE (aura 227)
	// bp299 die1 auraPeriod 1000 trigger 42170
	// (42170 trigger-only, NOT a live-cast ID,
	// not seeded) radius10 tgtA cone; cast1
	// instant, dur1 10000ms (aux row pre-existing),
	// range4 Medium Range 0-30yd (aux row
	// pre-existing), school Fire; NO SpellCategories
	// / SpellAuraOptions rows; EffectAttributes
	// 0x20000 / Attributes_0 0x800 /
	// Attributes_1 0x88 / BonusCoefficient not
	// carried (no fields in the seed struct).
	{id: 23023, castTimeIdx: 1, durIdx: 1, rangeIdx: 4, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 299, dieSides: 1, auraPeriod: 1000, radiusIdx: 10, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModConfuse, basePoints: 0, dieSides: 1, radiusIdx: 10, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrigWithValue, basePoints: 299, dieSides: 1, auraPeriod: 1000, trigger: 42170, radiusIdx: 10, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 6262 "Minor Healthstone" (wago SpellName
	// MISMATCH vs C++/Lua "Heal Father"; C++
	// SPELL_HEAL_FATHER, boss_warp_splinter.cpp:45;
	// Lua botanica/boss_warp_splinter.lua:128, 86 —
	// Warp self-casts with BP0 = treant current HP,
	// C++ boss_warp_splinter.cpp:110
	// AddSpellBP0(CurrentHP_Treant)) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_6262_34716_36705.txt,
	// DifficultyID 0 rows): eff0 HEAL bp99 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur0
	// (no SpellDuration row), range1 Self Only 0yd
	// (aux row pre-existing), school Physical
	// (SchoolMask 0x1); NO SpellAuraOptions row;
	// name mismatch recorded — the wago numeric data
	// FITS the Lua self-heal + BP0-override use, so
	// seeded on the data; Category 30 / DefenseType
	// 1 / PreventionType 0x2 / Attributes all 0x0 /
	// BonusCoefficient 0 not carried (no fields in
	// the seed struct).
	{id: 6262, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effHeal, basePoints: 99, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 34716 "Stomp" (wago SpellName MISMATCH, close,
	// vs C++/Lua "War Stomp"; C++ WAR_STOMP,
	// boss_warp_splinter.cpp:42; Lua
	// botanica/boss_warp_splinter.lua:98, 39 —
	// {25s,40s}) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16
	// verdict file wago_spells_6262_34716_36705.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp2374 die251 (in-game 2375-2625) tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY, RadiusIndex_0 0
	// (RadiusIndex_1 20 = 25yd — recorded, NOT
	// seeded, repo convention radiusIdx =
	// RadiusIndex_0, 19703/50770 precedent); eff1
	// APPLY_AURA MOD_STUN (aura 12) bp0 die0
	// radius20 25yd tgtA src-caster tgtB
	// src-area-enemy; eff2 APPLY_AURA
	// MOD_DAMAGE_TAKEN (aura 14) bp549 die1 misc1
	// radius20 tgtA src-caster tgtB
	// src-area-enemy; cast1 instant, dur28 5000ms
	// (aux row pre-existing), range1 Self Only
	// (aux row pre-existing), school Physical,
	// mechanic 12 (SpellCategories Mechanic 12);
	// NO SpellAuraOptions row; Attributes_0 0x10 /
	// Category 1152 / BonusCoefficient not carried
	// (no fields in the seed struct).
	{id: 34716, castTimeIdx: 1, durIdx: 28, rangeIdx: 1, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 2374, dieSides: 251, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, radiusIdx: 20, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDamageTaken, basePoints: 549, dieSides: 1, misc: 1, radiusIdx: 20, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 36705 "Arcane Volley" (wago SpellName MATCHES;
	// C++ ARCANE_VOLLEY, boss_warp_splinter.cpp:44;
	// Lua botanica/boss_warp_splinter.lua:99, 42 —
	// {8s,20s} init then {20s,35s}) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_6262_34716_36705.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// bp1849 die301 (in-game 1850-2150) radius27
	// 50yd tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast4 1000ms
	// (aux row pre-existing), dur0 (no
	// SpellDuration row), range37 Extended Range
	// 0-50yd (aux row pre-existing), school Arcane;
	// SpellAuraOptions ProcChance 101 /
	// DefenseType 1 / PreventionType 0x1 /
	// Attributes_0 0x10000 / Attributes_2 0x4 /
	// BonusCoefficient 1 not carried (no fields in
	// the seed struct).
	{id: 36705, castTimeIdx: 4, durIdx: 0, rangeIdx: 37, school: schoolArcane, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 1849, dieSides: 301, radiusIdx: 27, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 7588 "Void Bolt" (wago SpellName MATCHES;
	// C++ SPELL_VOIDBOLT, shadowfang_keep.cpp:207;
	// Lua eastern_kingdoms/boss_archmage_arugal.lua:79,
	// 50 — victim-cast, 1s init, 5s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_7588_7621_7803.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp221 die37
	// (in-game 222-258) tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast14 3000ms
	// (aux row pre-existing), dur0 (no
	// SpellDuration row), range6 Vision Range
	// 0-100yd (aux row pre-existing), school
	// Shadow; NO SpellAuraOptions row;
	// EffectAmplitude 1 / EffectRealPointsPerLevel
	// 1 / BonusCoefficient 1 / DefenseType 1 /
	// PreventionType 0x1 / Attributes all 0x0 not
	// carried (no fields in the seed struct).
	{id: 7588, castTimeIdx: 14, durIdx: 0, rangeIdx: 6, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 221, dieSides: 37, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 7621 "Arugal's Curse" (wago SpellName MATCHES;
	// C++ SPELL_ARUGAL_CURSE, shadowfang_keep.cpp:205;
	// Lua eastern_kingdoms/boss_archmage_arugal.lua:78,
	// 32 — random target, 7s init, 15s re-arm;
	// SpellHitTarget fires on landing) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_7588_7621_7803.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_CHARM (aura 6)
	// bp39 die1 tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff1 APPLY_AURA TRANSFORM (aura 56) bp0 die1
	// misc 3857 (Shadowfang Glutton) tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur1
	// 10000ms (aux row pre-existing), range4 Medium
	// Range 0-30yd (aux row pre-existing), school
	// Shadow, dispel 2 (Curse); NO SpellAuraOptions
	// row; Attributes_0 0x40000 / DefenseType 1 /
	// PreventionType 0x1 not carried (no fields in
	// the seed struct).
	{id: 7621, castTimeIdx: 1, durIdx: 1, rangeIdx: 4, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCharm, basePoints: 39, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraTransform, basePoints: 0, dieSides: 1, misc: 3857, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 7803 "Thundershock" (wago SpellName MATCHES;
	// C++ SPELL_THUNDERSHOCK, shadowfang_keep.cpp:206;
	// Lua eastern_kingdoms/boss_archmage_arugal.lua:80,
	// 53 — DoCastAOE) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-16
	// verdict file wago_spells_7588_7621_7803.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_STUN (aura 12) bp0 die0 radius8 5yd tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 SCHOOL_DAMAGE
	// bp63 die23 (in-game 64-86) radius8 tgtA
	// src-caster tgtB src-area-enemy; cast1 instant,
	// dur28 5000ms (aux row pre-existing), range12
	// Interact Range 0-5yd (aux row pre-existing),
	// school Nature, mechanic 12 (SpellCategories
	// Mechanic 12); NO SpellAuraOptions row;
	// BonusCoefficient 2 / Attributes_0 0x40000 /
	// Attributes_1 0x88 / DefenseType 1 /
	// PreventionType 0x2 not carried (no fields in
	// the seed struct).
	{id: 7803, castTimeIdx: 1, durIdx: 28, rangeIdx: 12, school: schoolNature, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 63, dieSides: 23, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 7290 "Soul Siphon" (wago SpellName MATCHES;
	// C++ SPELL_SOUL_SIPHON,
	// boss_azshir_the_sleepless.cpp:26; Lua
	// eastern_kingdoms/boss_azshir_the_sleepless.lua:41,
	// 107 — victim-cast, 20s re-arm) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_7290_7399_17831.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_LEECH (aura
	// 53) bp0 die1 auraPeriod 1000 chainTargets 2
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur1 10000ms (aux row pre-existing), range7
	// Very Short 0-10yd (aux row pre-existing),
	// school Shadow, dispel 1 (Magic); NO
	// SpellAuraOptions row; EffectAmplitude 2 /
	// EffectBonusCoefficient 0.1 /
	// EffectRealPointsPerLevel 0.7 / Attributes_0
	// 0x110000 / Attributes_1 0x4004 / Category 99 /
	// DefenseType 1 / PreventionType 0x1 not carried
	// (no fields in the seed struct).
	{id: 7290, castTimeIdx: 1, durIdx: 1, rangeIdx: 7, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicLeech, basePoints: 0, dieSides: 1, auraPeriod: 1000, targetA: targetEnemy, chainTargets: 2, chainAmplitude: 1},
	}},
	// 7399 "Terrify" (wago SpellName MATCHES; C++
	// SPELL_TERRIFY, boss_azshir_the_sleepless.cpp:25;
	// Lua eastern_kingdoms/boss_azshir_the_sleepless.lua:40,
	// 89 — victim-cast, 20s re-arm) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_7290_7399_17831.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_FEAR (aura 7) bp0
	// die0 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur35 4000ms (aux row pre-existing),
	// range2 Combat Range 0-5yd Flags 0x1 (aux row
	// pre-existing), school Shadow, dispel 1
	// (Magic), mechanic 5 (SpellCategories Mechanic
	// 5); NO SpellAuraOptions row; Attributes_0
	// 0x10 / Category 99 / DefenseType 1 /
	// PreventionType 0x1 not carried (no fields in
	// the seed struct).
	{id: 7399, castTimeIdx: 1, durIdx: 35, rangeIdx: 2, school: schoolShadow, dispel: 1, mechanic: 5, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 17831 "Call of the Grave" (wago SpellName
	// MATCHES; C++ SPELL_CALL_OF_THE_GRAVE,
	// boss_azshir_the_sleepless.cpp:24; Lua
	// eastern_kingdoms/boss_azshir_the_sleepless.lua:39,
	// 68 — victim-cast, 30s re-arm) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-16 verdict file
	// wago_spells_7290_7399_17831.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE (aura
	// 3) bp34 die1 auraPeriod 60000 radius8 5yd
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur3 60000ms (aux row pre-existing), range2
	// Combat Range 0-5yd (aux row pre-existing),
	// school Shadow, dispel 2 (Curse); NO
	// SpellAuraOptions row; EffectRealPointsPerLevel
	// 35 / Attributes_1 0x888 / DefenseType 1 /
	// PreventionType 0x1 not carried (no fields in
	// the seed struct).
	{id: 17831, castTimeIdx: 1, durIdx: 3, rangeIdx: 2, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 34, dieSides: 1, auraPeriod: 60000, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 15708 "Mortal Strike" (wago SpellName MATCHES;
	// C++ SPELL_MORTALSTRIKE,
	// boss_baron_rivendare.cpp:34; Lua
	// eastern_kingdoms/boss_baron_rivendare.lua:96,
	// 70 — victim-cast, 12s init, 10-25s re-arm)
	// — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15708_17393_17473.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE (effect 31)
	// bp299 die1 (in-game 300%) tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// MOD_HEALING_PCT (aura 118) bp-51 die1 misc
	// 127 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur28 5000ms (aux row pre-existing),
	// range2 Combat Range 0-5yd (aux row
	// pre-existing), school Physical; NO
	// SpellAuraOptions row; DefenseType 2 /
	// PreventionType 0x2 / Attributes_0 0x50010 /
	// StartRecoveryCategory 133 not carried (no
	// fields in the seed struct).
	{id: 15708, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 299, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, basePoints: -51, dieSides: 1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 17393 "Shadow Bolt" (wago SpellName MATCHES;
	// C++ SPELL_SHADOWBOLT,
	// boss_baron_rivendare.cpp:32; Lua
	// eastern_kingdoms/boss_baron_rivendare.lua:94,
	// 62 — random-target cast, 5s init, 10s re-arm)
	// — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15708_17393_17473.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp287 die101
	// (in-game 288-388) tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast5 2000ms (aux
	// row pre-existing), dur0 (no SpellDuration
	// row), range5 Long Range 0-40yd (aux row
	// pre-existing), school Shadow; NO
	// SpellAuraOptions row; Speed 24 /
	// BonusCoefficient 1 / Attributes_0 0x90000 /
	// StartRecoveryCategory 133 not carried (no
	// fields in the seed struct).
	{id: 17393, castTimeIdx: 5, durIdx: 0, rangeIdx: 5, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 287, dieSides: 101, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 17473 "Raise Dead" (wago SpellName MATCHES;
	// C++ SPELL_RAISE_DEAD,
	// boss_baron_rivendare.cpp:38; Lua
	// eastern_kingdoms/boss_baron_rivendare.lua:97,
	// 72 — self-cast, 15s init, 12s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15708_17393_17473.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// (aura 23) bp0 die0 auraPeriod 12000 trigger
	// 17471 (SPELL_DEATH_PACT_2 visual, trigger-only,
	// NOT seeded per repo convention) tgtA
	// TARGET_UNIT_CASTER; cast4 1000ms (aux row
	// pre-existing), dur29 12000ms (aux row
	// pre-existing), range1 Self Only 0yd (aux row
	// pre-existing), school Shadow; NO
	// SpellAuraOptions row; Attributes_0 0x100 /
	// Attributes_1 0x10000000 / BonusCoefficient 1
	// not carried (no fields in the seed struct).
	{id: 17473, castTimeIdx: 4, durIdx: 29, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 12000, trigger: 17471, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12098 "Sleep" (wago SpellName MATCHES; C++
	// SPELL_SLEEP, boss_dathrohan_balnazzar.cpp:43;
	// Lua eastern_kingdoms/boss_dathrohan_balnazzar.lua:87,
	// 212 — random-target cast, 15s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_12098_13704_17288.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STUN (aura 12)
	// bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY; cast4
	// 1000ms (aux row pre-existing), dur18 20000ms
	// (aux row pre-existing), range4 Medium Range
	// 0-30yd (aux row pre-existing), school Shadow,
	// dispel 1 (Magic), mechanic 10
	// (SpellCategories Mechanic 10); NO
	// SpellAuraOptions row; Attributes_0 0x40110000
	// / Attributes_1 0x40000 / Attributes_5 0x20 /
	// DefenseType 1 / PreventionType 0x1 /
	// StartRecoveryCategory 133 not carried (no
	// fields in the seed struct).
	{id: 12098, castTimeIdx: 4, durIdx: 18, rangeIdx: 4, school: schoolShadow, dispel: 1, mechanic: 10, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 13704 "Psychic Scream" (wago SpellName MATCHES;
	// C++ SPELL_PSYCHICSCREAM,
	// boss_dathrohan_balnazzar.cpp:42; Lua
	// eastern_kingdoms/boss_dathrohan_balnazzar.lua:86,
	// 204 — self-cast AoE fear, 11s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_12098_13704_17288.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_FEAR (aura 7) bp-1
	// die1 radius _1=14 8yd NOT seeded per repo
	// radius convention (seed carries _0 only;
	// _0=0 here) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur32 6000ms (aux row pre-existing), range1
	// Self Only 0yd (aux row pre-existing), school
	// Shadow, dispel 1 (Magic), mechanic 5
	// (SpellCategories Mechanic 5, effect mechanic
	// 5); NO SpellAuraOptions row; Attributes_0
	// 0x50000 / Attributes_1 0x88 / DefenseType 1 /
	// PreventionType 0x1 / StartRecoveryCategory
	// 133 not carried (no fields in the seed
	// struct). DifficultyID 173/174 rows excluded.
	{id: 13704, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolShadow, dispel: 1, mechanic: 5, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: -1, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 17288 "Balnazzar Transform" (wago SpellName
	// MATCHES; C++ SPELL_BALNAZZARTRANSFORM,
	// boss_dathrohan_balnazzar.cpp:37; Lua
	// eastern_kingdoms/boss_dathrohan_balnazzar.lua:83,
	// 36 — self-cast on transform, restores full
	// HP/mana) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-16 verdict file
	// wago_spells_12098_13704_17288.txt, DifficultyID 0
	// rows): eff0 HEAL_MAX_HEALTH (effect 67) bp-1
	// die1 tgtA TARGET_UNIT_CASTER; eff1 ENERGIZE
	// (effect 30) bp9999 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 TRIGGER_SPELL
	// (effect 64) bp0 die1 trigger 17398
	// (Balnazzar Transform Stun, trigger-only, NOT
	// seeded per repo convention) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur0 (no
	// SpellDuration row), range13 Anywhere 0-50000yd
	// (aux row pre-existing), school Shadow; NO
	// SpellAuraOptions row; Attributes_0 0x50100 /
	// StartRecoveryCategory 133 not carried (no
	// fields in the seed struct).
	{id: 17288, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolShadow, effects: [3]effect{
		{effect: effHealMaxHealth, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effEnergize, basePoints: 9999, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effTriggerSpell, basePoints: 0, dieSides: 1, trigger: 17398, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 15589 "Whirlwind" (wago SpellName MATCHES; C++
	// SPELL_WHIRLWIND, boss_drekthar.cpp:23; Lua
	// eastern_kingdoms/boss_drekthar.lua:56, 21 —
	// victim-cast, {1,20}s init, 25s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15589_13736_19128.txt, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE (effect 58) bp299
	// die1 (in-game 300%) radius _0=14 8yd tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self Only
	// 0yd (aux row pre-existing), school Physical;
	// NO SpellAuraOptions row; BonusCoefficient 1 /
	// EffectRealPointsPerLevel 6 / Attributes_0
	// 0x40010 / Attributes_3 0x200 / Attributes_5
	// 0x8000 / DefenseType 2 / PreventionType 0x2
	// not carried (no fields in the seed struct).
	{id: 15589, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 299, dieSides: 1, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 13736 "Whirlwind" (wago SpellName MATCHES; C++
	// SPELL_WHIRLWIND2, boss_drekthar.cpp:24; Lua
	// eastern_kingdoms/boss_drekthar.lua:57, 23 —
	// victim-cast aura, {1,20}s init, 25s re-arm)
	// — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15589_13736_19128.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// (aura 23) bp-1 die1 auraPeriod 2000 trigger
	// 15589 (seeded in this same trio — a live-cast
	// ID, not trigger-only) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur39
	// 2000ms (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Physical; NO SpellAuraOptions row; Category
	// 49 / Attributes_0 0x50010 / Attributes_1
	// 0x40 / DefenseType 2 / PreventionType 0x2 not
	// carried (no fields in the seed struct).
	{id: 13736, castTimeIdx: 1, durIdx: 39, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 2000, trigger: 15589, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 19128 "Knockdown" (wago SpellName MATCHES;
	// C++ SPELL_KNOCKDOWN, boss_drekthar.cpp:25;
	// Lua eastern_kingdoms/boss_drekthar.lua:58, 25
	// — victim-cast, 12s init, 25s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_15589_13736_19128.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STUN (aura 12) bp0
	// die0 chainTargets 5 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 SCHOOL_DAMAGE
	// bp59 die21 (in-game 60-80) chainTargets 5
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur39 2000ms (aux row pre-existing), range2
	// Combat Range 0-5yd (aux row pre-existing),
	// school Physical, mechanic 12 (SpellCategories
	// Mechanic 12); NO SpellAuraOptions row;
	// EffectRealPointsPerLevel 1.5 / Attributes_0
	// 0x40010 / Attributes_5 0x1000 not carried (no
	// fields in the seed struct).
	{id: 19128, castTimeIdx: 1, durIdx: 39, rangeIdx: 2, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainTargets: 5, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 59, dieSides: 21, targetA: targetEnemy, chainTargets: 5, chainAmplitude: 1},
	}},
	// 11428 "Knockdown" (wago SpellName MATCHES;
	// Lua SPELL_KNOCKDOWN,
	// eastern_kingdoms/boss_eramas_brightblaze.lua:69,
	// 28 — victim-cast, 6s init, 12s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11428_15503_46182.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STUN (aura 12) bp0
	// die0 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// SCHOOL_DAMAGE bp31 die17 (in-game 32-48)
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur39 2000ms (aux row pre-existing), range2
	// Combat Range 0-5yd (aux row pre-existing),
	// school Physical, mechanic 12 (SpellCategories
	// Mechanic 12); NO SpellAuraOptions row;
	// EffectRealPointsPerLevel 1.5 / Attributes_0
	// 0x40010 not carried (no fields in the seed
	// struct).
	{id: 11428, castTimeIdx: 1, durIdx: 39, rangeIdx: 2, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 31, dieSides: 17, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 15503 "Drink Healing Potion" (wago SpellName
	// MISMATCH vs Lua/C++ label "Healing Potion";
	// data fits the Lua use, seeded on data per the
	// warp_splinter 6262 precedent — Lua
	// SPELL_HEALING_POTION,
	// eastern_kingdoms/boss_eramas_brightblaze.lua:71,
	// 33 — HealthBelowPct(25) self-cast heal) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11428_15503_46182.txt, DifficultyID 0
	// rows): eff0 HEAL (effect 10) bp393 die63
	// (in-game 394-456) tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur0 (no SpellDuration row),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Holy; NO SpellAuraOptions row;
	// EffectAmplitude 1 / Category 25 /
	// Attributes_0 0xc0010 / DefenseType 1 /
	// PreventionType 0x1 not carried (no fields in
	// the seed struct).
	{id: 15503, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolHoly, effects: [3]effect{
		{effect: effHeal, basePoints: 393, dieSides: 63, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 46182 "Snap Kick" (wago SpellName MATCHES;
	// C++ SPELL_SNAP_KICK,
	// boss_priestess_delrissa.cpp:684; Lua
	// SPELL_SNAP_KICK,
	// eastern_kingdoms/boss_eramas_brightblaze.lua:70,
	// 30 — victim-cast, 4.5s init, 12s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11428_15503_46182.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE (effect 31)
	// bp9 die1 (in-game 10%) tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 INTERRUPT_CAST
	// (effect 68) bp0 die0 mechanic 26 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff2 APPLY_AURA
	// MOD_SILENCE (aura 27) bp0 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur39
	// 2000ms (aux row pre-existing), range2 Combat
	// Range 0-5yd (aux row pre-existing), school
	// Physical; SpellAuraOptions row EXISTS (ID
	// 184074, ProcChance 101, ProcTypeMask 0x0 —
	// not carried, no fields in the seed struct);
	// BonusCoefficient 1 (eff2) / Attributes_0
	// 0x50010 / DefenseType 2 / PreventionType 0x2
	// not carried (no fields in the seed struct).
	{id: 46182, castTimeIdx: 1, durIdx: 39, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 9, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effInterruptCast, basePoints: 0, dieSides: 0, mechanic: 26, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraSilence, basePoints: 0, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 14099 "Mighty Blow" (wago SpellName MATCHES;
	// C++ SPELL_MIGHTYBLOW,
	// boss_general_angerforge.cpp:24; Lua
	// eastern_kingdoms/boss_general_angerforge.lua:45,
	// 23 — victim-cast, 8s init, 15s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_14099_9080_20691.txt, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE (effect 58) bp24
	// die1 (in-game 25) tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK
	// (effect 98) bp74 die51 misc 100 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// (no SpellDuration row), range7 Very Short
	// 0-10yd (aux row pre-existing), school
	// Physical; NO SpellAuraOptions row;
	// BonusCoefficient 1 / EffectRealPointsPerLevel
	// 2 / Attributes_0 0x10 / DefenseType 2 /
	// PreventionType 0x2 not carried (no fields in
	// the seed struct).
	{id: 14099, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 24, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 74, dieSides: 51, misc: 100, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 9080 "Hamstring" (wago SpellName MATCHES; C++
	// SPELL_HAMSTRING,
	// boss_general_angerforge.cpp:25; Lua
	// eastern_kingdoms/boss_general_angerforge.lua:46,
	// 23 — victim-cast, 12s init, 15s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_14099_9080_20691.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE (effect 31)
	// bp19 die1 (in-game 20%) tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// MOD_DECREASE_SPEED (aura 33) bp-61 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur1
	// 10000ms (aux row pre-existing), range2 Combat
	// Range 0-5yd (aux row pre-existing), school
	// Physical, mechanic 11 (SpellCategories
	// Mechanic 11); NO SpellAuraOptions row;
	// DifficultyID 173-176 rows excluded;
	// Attributes_0 0x50010 / Attributes_1 0x8000000
	// / DefenseType 2 / PreventionType 0x2 not
	// carried (no fields in the seed struct).
	{id: 9080, castTimeIdx: 1, durIdx: 1, rangeIdx: 2, school: schoolPhysical, mechanic: 11, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 19, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -61, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 20691 "Cleave" (wago SpellName MATCHES; C++
	// SPELL_CLEAVE, boss_general_angerforge.cpp:26;
	// Lua eastern_kingdoms/boss_general_angerforge.lua:47,
	// 24 — victim-cast, 16s init, 9s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_14099_9080_20691.txt, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE (effect 58) bp-1
	// die1 chainTargets 5 chainAmplitude 0.66 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur0
	// (no SpellDuration row), range11 Fifteen yards
	// 0-15yd (aux row pre-existing), school
	// Physical; NO SpellAuraOptions row;
	// BonusCoefficient 1 / Attributes_0 0x40014 /
	// Attributes_1 0x88 / Attributes_5 0x9000 /
	// DefenseType 2 / PreventionType 0x2 not carried
	// (no fields in the seed struct).
	{id: 20691, castTimeIdx: 1, durIdx: 0, rangeIdx: 11, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: -1, dieSides: 1, targetA: targetEnemy, chainTargets: 5, chainAmplitude: 0.66},
	}},
	// 23462 "Fire Nova" (wago SpellName MATCHES;
	// C++ SPELL_FIRENOVA,
	// boss_drakkisath.cpp:24; Lua
	// eastern_kingdoms/boss_drakkisath.lua:44, 27 —
	// victim-cast, 6s init, 10s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_23462_16805_15548.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp554 die91
	// (in-game 555-645) misc 12 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range1 Self Only
	// 0yd (aux row pre-existing), school Fire; NO
	// SpellAuraOptions row; BonusCoefficient 1 /
	// Attributes_0 0x10000 / Attributes_1 0x88 /
	// Attributes_2 0x4 / DefenseType 1 /
	// PreventionType 0x1 not carried (no fields in
	// the seed struct).
	{id: 23462, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 554, dieSides: 91, misc: 12, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 16805 "Conflagration" (wago SpellName MATCHES;
	// C++ SPELL_CONFLIGURATION,
	// boss_drakkisath.cpp:26; Lua
	// eastern_kingdoms/boss_drakkisath.lua:46, 29 —
	// victim-cast, 15s init, 18s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_23462_16805_15548.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE (aura
	// 3) bp299 die1 auraPeriod 1000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// MOD_CONFUSE (aura 5) bp0 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff2 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp0 die1
	// auraPeriod 1000 trigger 16806 (periodic
	// scorch, trigger-only, NOT seeded per repo
	// convention) tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur1 10000ms (aux row
	// pre-existing), range4 Medium Range 0-30yd
	// (aux row pre-existing), school Fire; NO
	// SpellAuraOptions row; NO SpellCategories row
	// (mechanic 0, dispel 0); BonusCoefficient 1
	// (eff1/eff2) / Attributes_0 0x800 not carried
	// (no fields in the seed struct).
	{id: 16805, castTimeIdx: 1, durIdx: 1, rangeIdx: 4, school: schoolFire, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 299, dieSides: 1, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModConfuse, basePoints: 0, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 1, auraPeriod: 1000, trigger: 16806, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 15548 "Thunderclap" (wago SpellName MATCHES —
	// resolves the C++ "Not sure if right ID. 23931
	// would be a harder possibility."
	// boss_drakkisath.cpp:27 caveat in favor of
	// 15548; Lua SPELL_THUNDERCLAP,
	// eastern_kingdoms/boss_drakkisath.lua:47, 30 —
	// victim-cast, 17s init, 20s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_23462_16805_15548.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp34 die5 (in-game
	// 35-39) radius13 10yd tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; eff1
	// APPLY_AURA MOD_DECREASE_SPEED (aura 33) bp-41
	// die1 mechanic 11 radius13 tgtA src-caster
	// tgtB src-area-enemy; eff2 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp-34 die1
	// radius13 tgtA src-caster tgtB src-area-enemy;
	// cast1 instant, dur1 10000ms (aux row
	// pre-existing), range1 Self Only 0yd (aux row
	// pre-existing), school Nature, dispel 1
	// (Magic), mechanic 11 (SpellCategories
	// Mechanic 11); NO SpellAuraOptions row;
	// BonusCoefficient 1 / RealPointsPerLevel 1 /
	// -0.5 / Category 22 / Attributes_0 0x10000 /
	// Attributes_1 0x88 / Attributes_5 0x8000 /
	// DefenseType 1 / PreventionType 0x1 /
	// StartRecoveryCategory 133 not carried (no
	// fields in the seed struct).
	{id: 15548, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolNature, dispel: 1, mechanic: 11, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 34, dieSides: 5, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -41, dieSides: 1, mechanic: 11, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -34, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 17244 "Possess" (wago SpellName MATCHES;
	// C++ SPELL_POSSESS,
	// boss_baroness_anastari.cpp:30; Lua
	// eastern_kingdoms/boss_baroness_anastari.lua:65-66, 28-30 —
	// triggered victim-cast, 20-30s init) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_17244_17246_17250.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_CHARM (aura 6)
	// bp99 die1 tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff1 APPLY_AURA MECHANIC_IMMUNITY (aura 77)
	// bp0 die1 misc 1 tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff2 APPLY_AURA MOD_INCREASE_HEALTH (aura 34)
	// bp1999 die1 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur4 120000ms (aux row
	// pre-existing), range4 Medium Range 0-30yd
	// (aux row pre-existing), school Shadow
	// (0x20); SpellCategories row EXISTS (Category
	// 93, DefenseType 0, DispelType 0, Mechanic 0)
	// — category/defense not carried (no fields in
	// the seed struct); NO SpellAuraOptions row;
	// Speed 27 / Attributes_0 0x0 / Attributes_1
	// 0x10010000 not carried (no fields in the
	// seed struct); SpellTargetRestrictions row
	// (MaxTargets 0) not carried.
	{id: 17244, castTimeIdx: 1, durIdx: 4, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCharm, basePoints: 99, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunity, basePoints: 0, dieSides: 1, misc: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModIncreaseHealth, basePoints: 1999, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 17246 "Possessed" (wago SpellName MATCHES;
	// C++ SPELL_POSSESSED,
	// boss_baroness_anastari.cpp:31; Lua
	// eastern_kingdoms/boss_baroness_anastari.lua:65-66, 29 —
	// triggered victim-cast with 17244, HasAura
	// gate + RemoveAura on check) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_17244_17246_17250.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// (aura 79) bp99 die1 misc 127 tgtA
	// TARGET_UNIT_TARGET_ANY; NO eff1 row for
	// DifficultyID 0 ({} gap, 34799 precedent);
	// eff2 HEAL_MAX_HEALTH (effect 67) bp0 die1
	// tgtA TARGET_UNIT_TARGET_ANY; cast1 instant,
	// dur4 120000ms (aux row pre-existing),
	// range13 Anywhere 0-50000yd -> synthetic
	// 93008 (68415/52267 precedent), school
	// Physical; NO SpellAuraOptions row; NO
	// SpellCategories row; Attributes_0 0x4000110 /
	// Attributes_2 0x4 not carried (no fields in
	// the seed struct).
	{id: 17246, castTimeIdx: 1, durIdx: 4, rangeIdx: 93008, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 99, dieSides: 1, misc: 127, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{},
		{effect: effHealMaxHealth, basePoints: 0, dieSides: 1, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 17250 "Possess" (wago SpellName MISMATCH —
	// expected "Possess Invisibility", DB2 name is
	// plain "Possess", same as 17244; seeded on
	// data per the eramas_brightblaze 15503
	// precedent; C++ SPELL_POSSESS_INV,
	// boss_baroness_anastari.cpp:32; Lua
	// eastern_kingdoms/boss_baroness_anastari.lua:65-66, 30 —
	// triggered self-cast, baroness invisible) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_17244_17246_17250.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA SCHOOL_IMMUNITY
	// (aura 39) bp0 die1 misc 127 (0x7f as
	// displayed) tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA MOD_INVISIBILITY (aura 18)
	// bp4999 die1 tgtA TARGET_UNIT_CASTER (confirms
	// the invisibility function despite the plain
	// name); eff2 APPLY_AURA MOD_STUN (aura 12)
	// bp0 die0 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur4 120000ms (aux row
	// pre-existing), range1 Self Only 0yd (aux
	// row pre-existing), school Physical; NO
	// SpellAuraOptions row; NO SpellCategories
	// row; Attributes_0 0x190 / Attributes_1
	// 0x10000000 not carried (no fields in the
	// seed struct).
	{id: 17250, castTimeIdx: 1, durIdx: 4, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraSchoolImmunity, basePoints: 0, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModInvisibility, basePoints: 4999, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, basePoints: 0, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 8374 "Arcing Smash" (wago SpellName MATCHES;
	// C++ SPELL_ARCINGSMASH,
	// boss_ironaya.cpp:31; Lua
	// eastern_kingdoms/boss_ironaya.lua:70, 121 —
	// self-cast, 3s init) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_8374_10255_11876.txt, DifficultyID 0
	// rows; loop-c browser task 2b6ef297): eff0
	// WEAPON_DAMAGE (effect 58) bp-1 die1
	// radiusIdx 8 (5yd, aux row pre-existing)
	// tgtA TARGET_UNIT_CONE_ENEMY_24; cast1
	// instant, dur0 (no SpellDuration row),
	// range2 Combat 0-5yd (aux row pre-existing),
	// school Physical; SpellCategories row EXISTS
	// (Category 0, DefenseType 2 Melee,
	// PreventionType 0x2) — category/defense/
	// prevention not carried (no fields in the
	// seed struct); NO SpellAuraOptions row;
	// EffectAttributes 0x20000 / Attributes_0
	// 0x40014 / Attributes_1 0x88 not carried.
	{id: 8374, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: -1, dieSides: 1, radiusIdx: 8, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 10255 "Stoned" (wago SpellName MISMATCH —
	// expected "Minion Freeze Anim", DB2 name is
	// "Stoned"; seeded on data per the
	// eramas_brightblaze 15503 precedent — the Lua
	// use is an AddAura visual, not a cast under
	// that name; C++ SPELL_MINION_FREEZE_ANIM,
	// boss_archaedas.cpp:57; Lua
	// eastern_kingdoms/boss_ironaya.lua:73, 163 and
	// eastern_kingdoms/boss_archaedas.lua:166, 270/359 —
	// AddAura/RemoveAura) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_8374_10255_11876.txt, DifficultyID 0
	// rows; loop-c browser task 2b6ef297): eff0
	// APPLY_AURA DUMMY (aura 4) bp0 die1 mechanic
	// 13 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur21 -1 infinite (aux row pre-existing),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Physical; NO SpellAuraOptions row;
	// NO SpellCategories row; Attributes_0 0x110 /
	// Attributes_1 0x10000000 not carried (no
	// fields in the seed struct).
	{id: 10255, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 1, mechanic: 13, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 11876 "War Stomp" (wago SpellName MATCHES;
	// C++ SPELL_WSTOMP,
	// boss_ironaya.cpp:33; Lua
	// eastern_kingdoms/boss_ironaya.lua:72, 137 —
	// self-cast, <25% one-shot) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_8374_10255_11876.txt, DifficultyID 0
	// rows; loop-c browser task 2b6ef297): eff0
	// APPLY_AURA MOD_STUN (aura 12) bp-1 die1
	// radiusIdx 8 (5yd, aux row pre-existing)
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; NO eff1 row for
	// DifficultyID 0 ({} gap, 34799 precedent);
	// eff2 WEAPON_DAMAGE (effect 58) bp22 die1
	// radiusIdx 8 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur28 5000ms (aux row pre-existing), range1
	// Self Only 0yd (aux row pre-existing), school
	// Physical; SpellCategories row EXISTS
	// (Category 32, DefenseType 1, Mechanic 12,
	// PreventionType 0x1) — mechanic 12 carried,
	// category/defense/prevention not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; eff0 EffectAmplitude 1 / eff2
	// RealPointsPerLevel 0.5 / Attributes_0 0x40010
	// not carried (no fields in the seed struct).
	{id: 11876, castTimeIdx: 1, durIdx: 28, rangeIdx: 1, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: -1, dieSides: 1, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{},
		{effect: effWeaponDamage, basePoints: 22, dieSides: 1, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 24673 "Curse of Blood" (wago SpellName MATCHES;
	// C++ SPELL_CURSE_OF_BLOOD,
	// boss_jandice_barov.cpp:24; Lua
	// eastern_kingdoms/boss_jandice_barov.lua:45, 91 —
	// victim-cast, 15s init -> 30s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_24673_17773_26096.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_TAKEN
	// (aura 14) bp499 die1 misc 1 radiusIdx 18
	// (15yd, aux row pre-existing) tgtA
	// TARGET_UNIT_DEST_AREA_ENEMY; cast1 instant,
	// dur6 600000ms (aux row pre-existing),
	// range3 Twenty yards 0-20yd (aux row
	// pre-existing), school Shadow (0x20),
	// dispel 2 (CURSE, from SpellCategories);
	// SpellCategories row EXISTS (Category 36,
	// DefenseType 1, Mechanic 0, PreventionType
	// 0x1) — category/defense/prevention not
	// carried (no fields in the seed struct); NO
	// SpellAuraOptions row; Speed 60 /
	// Attributes_0–13 all 0x0 not carried.
	{id: 24673, castTimeIdx: 1, durIdx: 6, rangeIdx: 3, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDamageTaken, basePoints: 499, dieSides: 1, radiusIdx: 18, misc: 1, targetA: targetUnitDestAreaEnemy, chainAmplitude: 1},
	}},
	// 17773 "Summon Illusions" (wago SpellName
	// MISMATCH — expected "Illusion", DB2 name is
	// "Summon Illusions"; seeded on data per the
	// eramas_brightblaze 15503 precedent; C++
	// SPELL_ILLUSION, boss_jandice_barov.cpp:25;
	// Lua eastern_kingdoms/boss_jandice_barov.lua:46,
	// 123 — self-cast, 30s init -> 25s re-arm) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_24673_17773_26096.txt, DifficultyID 0
	// rows): eff0 SUMMON (effect 28) bp14 die1
	// misc 11439 (summon entry) miscB 61
	// (SummonProperties row pre-existing) tgtA
	// TARGET_DEST_CASTER; cast1 instant, dur9
	// 30000ms (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Shadow (0x20); NO SpellAuraOptions row; NO
	// SpellCategories row; BonusCoefficient 1 /
	// Attributes_0–13 all 0x0 not carried (no
	// fields in the seed struct). 26096 (C++
	// SPELL_DROP_JOURNAL) NOT seeded — no rows
	// for ID 26096 in build 3.4.0.47659 (verdict
	// file); unseedable, joins 22540/59046/65296/
	// 68783.
	{id: 17773, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effSummon, basePoints: 14, dieSides: 1, misc: 11439, miscB: 61, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 20294 "Immolate" (wago SpellName MATCHES;
	// C++ SPELL_IMMOLATE,
	// boss_lord_alexei_barov.cpp:31; Lua
	// eastern_kingdoms/boss_lord_alexei_barov.lua:27, 72 —
	// triggered random-target cast) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_20294_17820_17467.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE
	// (aura 3) bp379 die41 auraPeriod 3000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 SCHOOL_DAMAGE
	// bp759 die81 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur86 21000ms (NEW aux row,
	// patchwerk precedent), range5 Long Range
	// 0-40yd (aux row pre-existing), school Fire,
	// dispel 1 (MAGIC, from SpellCategories);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1, Mechanic 0, PreventionType
	// 0x1) — category/defense/prevention not
	// carried (no fields in the seed struct); NO
	// SpellAuraOptions row; BonusCoefficient
	// 0.1/0.3 / Attributes_0 0x0 / Attributes_1
	// 0x88 not carried (no fields in the seed
	// struct).
	{id: 20294, castTimeIdx: 1, durIdx: 86, rangeIdx: 5, school: schoolFire, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 379, dieSides: 41, auraPeriod: 3000, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 759, dieSides: 81, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 17820 "Veil of Shadow" (wago SpellName MATCHES;
	// C++ SPELL_VEILOFSHADOW,
	// boss_lord_alexei_barov.cpp:32; Lua
	// eastern_kingdoms/boss_lord_alexei_barov.lua:28, 90 —
	// triggered victim-cast, 15s init -> 20s
	// re-arm) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 verdict file
	// wago_spells_20294_17820_17467.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_HEALING_PCT
	// (aura 118) bp-76 die1 misc 126 radiusIdx 14
	// (8yd, aux row pre-existing) tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast16 1500ms
	// (aux row pre-existing), dur8 15000ms (aux
	// row pre-existing), range3 Twenty yards
	// 0-20yd (aux row pre-existing), school
	// Shadow (0x20), dispel 2 (CURSE, from
	// SpellCategories); SpellCategories row EXISTS
	// (Category 36, DefenseType 1, Mechanic 0,
	// PreventionType 0x1) — category/defense/
	// prevention not carried (no fields in the
	// seed struct); NO SpellAuraOptions row;
	// Attributes_0 0x10 / Attributes_1 0x8000 not
	// carried (no fields in the seed struct).
	{id: 17820, castTimeIdx: 16, durIdx: 8, rangeIdx: 3, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModHealingPct, basePoints: -76, dieSides: 1, misc: 126, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 17467 "Unholy Aura" (wago SpellName MATCHES;
	// C++ SPELL_UNHOLY_AURA,
	// boss_lord_alexei_barov.cpp:33; Lua
	// eastern_kingdoms/boss_lord_alexei_barov.lua:29, 13 —
	// HasAura-gated self-cast, re-applies after
	// dispel) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-16 verdict file
	// wago_spells_20294_17820_17467.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// (aura 23) bp0 die1 auraPeriod 2000 trigger
	// 17466 (periodic shadow damage, trigger-only,
	// NOT seeded per repo convention) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Shadow (0x20); SpellCategories row EXISTS
	// (Category 0, DefenseType 1, DispelType 0,
	// Mechanic 0, PreventionType 0x1) —
	// category/defense/prevention not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Attributes_0–13 all 0x0.
	{id: 17467, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 1, auraPeriod: 2000, trigger: 17466, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 27249 "Summon Spectral Assassin" (wago SpellName MATCHES;
	// C++ SPELL_SUMMON_SPECTRAL_ASSASSIN,
	// boss_lord_valthalak.cpp:26; Lua
	// eastern_kingdoms/boss_lord_valthalak.lua:76, 116 —
	// self-cast, 6-8s init -> 30-35s loop) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_27249_27382_27286.txt, DifficultyID 0
	// rows): eff0 SUMMON (effect 28) bp1 die1
	// misc 16066 (summon entry) miscB 64
	// (SummonProperties row pre-existing)
	// radiusIdx 8 (5yd, aux row pre-existing)
	// tgtA TARGET_DEST_CASTER_SUMMON; cast1
	// instant, dur3 60000ms (aux row pre-existing),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Physical; NO SpellAuraOptions row; NO
	// SpellCategories row; BonusCoefficient 1 /
	// Attributes_0–13 all 0x0 not carried (no
	// fields in the seed struct).
	{id: 27249, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 1, dieSides: 1, radiusIdx: 8, misc: 16066, miscB: 64, targetA: targetDestCasterSummon, chainAmplitude: 1},
	}},
	// 27382 "Shadow Bolt Volley" (wago SpellName MATCHES;
	// C++ SPELL_SHADOW_BOLT_VOLLEY,
	// boss_lord_valthalak.cpp:27; Lua
	// eastern_kingdoms/boss_lord_valthalak.lua:77, 139 —
	// victim-cast) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_27249_27382_27286.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp0 die0
	// auraPeriod 2000 trigger 27383 (shadow damage,
	// trigger-only, NOT seeded per repo convention)
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21
	// -1 infinite (aux row pre-existing), range4
	// Medium Range 0-30yd (aux row pre-existing),
	// school Shadow (0x20); SpellCategories row
	// EXISTS (Category 0, DefenseType 1,
	// PreventionType 0x1, StartRecoveryCategory
	// 133) — category/defense/prevention/recovery
	// not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; BonusCoefficient 1 /
	// Attributes_0 0x0 / Attributes_1 0x88 not
	// carried (no fields in the seed struct).
	{id: 27382, castTimeIdx: 1, durIdx: 21, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: 0, dieSides: 0, auraPeriod: 2000, trigger: 27383, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 27286 "Shadow Wrath" (wago SpellName MATCHES;
	// C++ SPELL_SHADOW_WRATH,
	// boss_lord_valthalak.cpp:28; Lua
	// eastern_kingdoms/boss_lord_valthalak.lua:78, 127 —
	// victim-cast, 9-18s init) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_27249_27382_27286.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp499 die1
	// chainTargets 10 chainAmplitude 1.5 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row), range135 Tower
	// 100 0-100yd (aux row pre-existing), school
	// Shadow (0x20); SpellCategories row EXISTS
	// (Category 0, DefenseType 1, DispelType 0,
	// Mechanic 0, PreventionType 0x1) —
	// category/defense/prevention not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; BonusCoefficient 1 / Attributes_0–13 all
	// 0x0 not carried (no fields in the seed
	// struct).
	{id: 27286, castTimeIdx: 1, durIdx: 0, rangeIdx: 135, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 499, dieSides: 1, chainTargets: 10, chainAmplitude: 1.5, targetA: targetEnemy},
	}},
	// 10947 "Mind Blast" (wago SpellName MATCHES;
	// C++ SPELL_MINDBLAST,
	// boss_moira_bronzebeard.cpp:27; Lua
	// eastern_kingdoms/boss_moira_bronzebeard.lua:33, 66 —
	// victim-cast, 16s init -> 14s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_10947_10894_10934.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp502 die29
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast16
	// 1500ms (aux row pre-existing), dur0 (no
	// SpellDuration row), range4 Medium Range
	// 0-30yd (aux row pre-existing), school
	// Shadow (0x20); SpellCategories row EXISTS
	// (Category 19, DefenseType 1, DispelType 0,
	// PreventionType 0x1, StartRecoveryCategory
	// 133) — category/defense/prevention/recovery
	// not carried (no fields in the seed struct);
	// NO SpellAuraOptions row;
	// EffectBonusCoefficient 0.429 /
	// EffectRealPointsPerLevel 2.6 not carried
	// (no fields in the seed struct).
	{id: 10947, castTimeIdx: 16, durIdx: 0, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 502, dieSides: 29, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 10894 "Shadow Word: Pain" (wago SpellName MATCHES;
	// C++ SPELL_SHADOWWORDPAIN,
	// boss_moira_bronzebeard.cpp:28; Lua
	// eastern_kingdoms/boss_moira_bronzebeard.lua:34, 74 —
	// victim-cast, 2s init -> 18s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_10947_10894_10934.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE
	// (aura 3) bp127 die1 auraPeriod 3000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur85 18000ms (aux row pre-existing),
	// range4 Medium Range 0-30yd (aux row
	// pre-existing), school Shadow (0x20);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1, DispelType 1 (MAGIC),
	// Mechanic 0, PreventionType 0x1,
	// StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); SpellAuraOptions
	// row EXISTS (ProcChance 100) — proc fields
	// not carried (no fields in the seed struct);
	// EffectBonusCoefficient 0 /
	// EffectRealPointsPerLevel 0.
	{id: 10894, castTimeIdx: 1, durIdx: 85, rangeIdx: 4, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 127, dieSides: 1, auraPeriod: 3000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 10934 "Smite" (wago SpellName MATCHES;
	// C++ SPELL_SMITE,
	// boss_moira_bronzebeard.cpp:29; Lua
	// eastern_kingdoms/boss_moira_bronzebeard.lua:35, 82 —
	// victim-cast, 8s init -> 10s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_10947_10894_10934.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE bp370 die45
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast19
	// 2500ms (aux row pre-existing), dur0 (no
	// SpellDuration row), range4 Medium Range
	// 0-30yd (aux row pre-existing), school Holy
	// (0x2); SpellCategories row EXISTS (Category
	// 0, DefenseType 1, DispelType 0, Mechanic 0,
	// PreventionType 0x1, StartRecoveryCategory
	// 133) — category/defense/prevention/recovery
	// not carried (no fields in the seed struct);
	// NO SpellAuraOptions row;
	// EffectBonusCoefficient 0.714 /
	// EffectRealPointsPerLevel 2.7 not carried
	// (no fields in the seed struct).
	{id: 10934, castTimeIdx: 19, durIdx: 0, rangeIdx: 4, school: schoolHoly, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 370, dieSides: 45, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 8122 "Psychic Scream" (wago SpellName MATCHES;
	// C++ SPELL_PSYCHICSCREAM,
	// boss_high_interrogator_gerstahn.cpp:26; Lua
	// eastern_kingdoms/boss_high_interrogator_gerstahn.lua:42, 113 —
	// victim-cast, 32s init -> 30s) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_10876_8122_22417.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_FEAR (aura 7)
	// bp-1 die1 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY, radius ONLY in
	// EffectRadiusIndex_1 (14, 8yd) — NOT carried
	// (no radiusIdxB field in the seed struct;
	// 19703 radius-_1 precedent); eff1 APPLY_AURA
	// MOD_INCREASE_SPEED (aura 31) bp24 die1
	// radiusIdx 14 (8yd, aux row pre-existing)
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur31 8000ms (aux row pre-existing), range1
	// Self Only 0yd (aux row pre-existing), school
	// Shadow (0x20); SpellCategories row EXISTS
	// (Category 43, DefenseType 1, DispelType 1
	// (MAGIC) carried as dispel 1, Mechanic 5
	// (FEAR) carried as spell mechanic per the
	// 24110 mechanic: 7 precedent, PreventionType
	// 0x1, StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); SpellAuraOptions
	// row EXISTS (ProcChance 100, ProcTypeMask
	// 0x800a22a8) — proc fields not carried (no
	// fields in the seed struct); EffectBonusCoefficient
	// 0/1 / EffectRealPointsPerLevel 0 not carried
	// (no fields in the seed struct).
	{id: 8122, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolShadow, dispel: 1, mechanic: 5, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: -1, dieSides: 1, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModIncreaseSpeed, basePoints: 24, dieSides: 1, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 22417 "Shadow Shield" (wago SpellName MATCHES;
	// C++ SPELL_SHADOWSHIELD,
	// boss_high_interrogator_gerstahn.cpp:27; Lua
	// eastern_kingdoms/boss_high_interrogator_gerstahn.lua:43, 121 —
	// self-cast, 8s init -> 25s) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_10876_8122_22417.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA SCHOOL_ABSORB
	// (aura 69) bp399 die1 misc 0x7f (all-schools
	// mask) tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA DAMAGE_SHIELD (aura 15) bp19
	// die1 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur9 30000ms (aux row pre-existing), range5
	// Long Range 0-40yd (aux row pre-existing),
	// school Shadow (0x20); SpellCategories row
	// EXISTS (Category 0, DefenseType 1,
	// DispelType 1 (MAGIC) carried as dispel 1,
	// Mechanic 0, PreventionType 0x1,
	// StartRecoveryCategory 0) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; EffectBonusCoefficient 0 /
	// EffectRealPointsPerLevel 20/1 not carried
	// (no fields in the seed struct).
	{id: 22417, castTimeIdx: 1, durIdx: 9, rangeIdx: 5, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraSchoolAbsorb, basePoints: 399, dieSides: 1, misc: 0x7f, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDamageShield, basePoints: 19, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 16104 "Crystallize" (wago SpellName MISMATCH vs
	// C++/Lua "Crystalize" (one L); seeded on data
	// per the eramas_brightblaze 15503 precedent;
	// C++ SPELL_CRYSTALIZE,
	// boss_mother_smolderweb.cpp:24; Lua
	// eastern_kingdoms/boss_mother_smolderweb.lua:52, 88 —
	// self-cast, 20s init -> 15s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_16104_16468_16103.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STUN (aura 12)
	// bp0 die0 radiusIdx 13 (10yd, aux row
	// pre-existing) tgtA TARGET_UNIT_CONE_ENEMY;
	// cast1 instant, dur32 6000ms (aux row
	// pre-existing), range2 Combat Range 0-5yd
	// (aux row pre-existing), school Arcane
	// (0x40); SpellCategories row EXISTS
	// (Category 33, DefenseType 0, DispelType 1
	// (MAGIC) carried as dispel 1, Mechanic 12
	// (STUN) carried as spell mechanic per the
	// 24110/8122 precedent, PreventionType 0x0,
	// StartRecoveryCategory 0) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; EffectBonusCoefficient 0 /
	// EffectRealPointsPerLevel 0 not carried (no
	// fields in the seed struct).
	{id: 16104, castTimeIdx: 1, durIdx: 32, rangeIdx: 2, school: schoolArcane, dispel: 1, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, radiusIdx: 13, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 16468 "Mother's Milk" (wago SpellName MATCHES;
	// C++ SPELL_MOTHERSMILK,
	// boss_mother_smolderweb.cpp:25; Lua
	// eastern_kingdoms/boss_mother_smolderweb.lua:53, 97 —
	// self-cast, 10s init) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_16104_16468_16103.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA DUMMY
	// (aura 4) bp0 die1 radiusIdx 13 (10yd, aux
	// row pre-existing) tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast1
	// instant, dur21 -1 infinite (aux row
	// pre-existing), range7 Ten yards 0-10yd
	// (aux row pre-existing), school Nature
	// (0x8); SpellCategories row EXISTS (Category
	// 0, DefenseType 0, DispelType 4 (POISON)
	// carried as dispel 4, Mechanic 0,
	// PreventionType 0x0, StartRecoveryCategory
	// 0) — category/defense/prevention/recovery
	// not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; EffectBonusCoefficient
	// 1 / EffectRealPointsPerLevel 0 not carried
	// (no fields in the seed struct).
	{id: 16468, castTimeIdx: 1, durIdx: 21, rangeIdx: 7, school: schoolNature, dispel: 4, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 0, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 16103 "Summon Spire Spiderling" (wago SpellName MATCHES;
	// C++ SPELL_SUMMON_SPIRE_SPIDERLING,
	// boss_mother_smolderweb.cpp:26; Lua
	// eastern_kingdoms/boss_mother_smolderweb.lua:54, 143 —
	// self-cast, triggered) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-16
	// verdict file wago_spells_16104_16468_16103.txt,
	// DifficultyID 0 rows): eff0 SUMMON (effect
	// 28) bp3 die1 misc 10375 (summon entry)
	// miscB 64 (SummonProperties row pre-existing)
	// tgtA TARGET_DEST_CASTER; cast1 instant,
	// dur21 -1 infinite (aux row pre-existing),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Physical; NO SpellCategories row;
	// NO SpellAuraOptions row; BonusCoefficient 1
	// / Attributes_2 0x4 not carried (no fields
	// in the seed struct).
	{id: 16103, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effSummon, basePoints: 3, dieSides: 1, misc: 10375, miscB: 64, targetA: targetDestCaster, chainAmplitude: 1},
	}},
	// 11130 "Knock Away" (wago SpellName MISMATCH vs
	// C++/Lua "Blast Wave"; seeded on data per the
	// eramas_brightblaze 15503 / smolderweb 16104
	// precedent; C++ SPELL_BLASTWAVE,
	// boss_overlord_wyrmthalak.cpp:25; Lua
	// eastern_kingdoms/boss_overlord_wyrmthalak.lua:61, 108 —
	// victim-cast, 20s init -> 20s) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11130_23511_20686.txt, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE (effect 58) bp29
	// die1 radiusIdx 13 (10yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 KNOCK_BACK
	// (effect 98) bp112 die75 misc 75 radiusIdx 13
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row 0; 10934
	// precedent), range7 Ten yards 0-10yd (aux row
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 2 (Melee), DispelType 0 carried
	// as dispel 0, Mechanic 0, PreventionType 0x2,
	// StartRecoveryCategory 0) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Attributes_0 0x10 / Attributes_1 0x0
	// not carried (no fields in the seed struct);
	// EffectBonusCoefficient 1/0 not carried (no
	// fields in the seed struct).
	{id: 11130, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 29, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 112, dieSides: 75, misc: 75, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 23511 "Demoralizing Shout" (wago SpellName
	// MISMATCH vs C++/Lua "Shout"; seeded on data
	// per the eramas_brightblaze 15503 / smolderweb
	// 16104 precedent; C++ SPELL_SHOUT,
	// boss_overlord_wyrmthalak.cpp:26; Lua
	// eastern_kingdoms/boss_overlord_wyrmthalak.lua:62, 120 —
	// victim-cast, 2s init -> 10s) —
	// wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11130_23511_20686.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_ATTACK_POWER
	// (aura 99) bp-41 die1 radiusIdx 13 (10yd,
	// aux row pre-existing) tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast1
	// instant, dur9 30000ms (aux row
	// pre-existing), range1 Self Only 0yd (aux row
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0 carried
	// as dispel 0, Mechanic 0, PreventionType 0x1,
	// StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Attributes_0 0x50010 / Attributes_1 0x0
	// not carried (no fields in the seed struct);
	// EffectRealPointsPerLevel -8 not carried (no
	// fields in the seed struct).
	{id: 23511, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModAttackPower, basePoints: -41, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 20686 "Knock Away" (wago SpellName MATCHES;
	// C++ SPELL_KNOCKAWAY,
	// boss_overlord_wyrmthalak.cpp:28; Lua
	// eastern_kingdoms/boss_overlord_wyrmthalak.lua:64, 144 —
	// victim-cast, 12s init -> 14s) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-16 verdict file
	// wago_spells_11130_23511_20686.txt, DifficultyID 0
	// rows): eff0 WEAPON_DAMAGE (effect 58) bp29
	// die1 radiusIdx 13 (10yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 KNOCK_BACK
	// (effect 98) bp112 die75 misc 75 radiusIdx 13
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff2 SCRIPT_EFFECT
	// (effect 77) bp0 die0 radiusIdx 13 tgtA
	// TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row 0; 10934
	// precedent), range7 Ten yards 0-10yd (aux row
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 2 (Melee), DispelType 0 carried
	// as dispel 0, Mechanic 0, PreventionType 0x2,
	// StartRecoveryCategory 0) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Attributes_0 0x10 / Attributes_1 0x88
	// not carried (no fields in the seed struct);
	// EffectBonusCoefficient 1/0/1 not carried (no
	// fields in the seed struct). Same first two
	// effects as 11130 plus the SCRIPT_EFFECT leg;
	// 11130 and 20686 share the wago name
	// "Knock Away" in this build.
	{id: 20686, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponDamage, basePoints: 29, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 112, dieSides: 75, misc: 75, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effScriptEffect, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 44294 "Drain Life" (wago SpellName MATCHES;
	// C++ SPELL_DRAIN_LIFE,
	// boss_selin_fireheart.cpp:42; Lua
	// eastern_kingdoms/boss_selin_fireheart.lua:122, 176 —
	// target-cast, 10s loop) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_44294_44314_46153.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_LEECH (aura 53) bp1499 die1
	// auraPeriod 1000 tgtA TARGET_UNIT_TARGET_ENEMY
	// tgtB NONE; cast1 instant, dur27 3000ms (aux
	// row pre-existing), range3 Twenty yards 0-20yd
	// (aux row pre-existing), school Shadow (0x20);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 1 (MAGIC)
	// carried as dispel 1, Mechanic 0,
	// PreventionType 0x1, StartRecoveryCategory 133)
	// — category/defense/prevention/recovery not
	// carried (no fields in the seed struct);
	// SpellAuraOptions row EXISTS (ProcChance 101)
	// — proc fields not carried (no fields in the
	// seed struct); Attributes_0 0x10000 /
	// Attributes_1 0x4004 not carried (no fields in
	// the seed struct); EffectBonusCoefficient 0.3
	// not carried (no fields in the seed struct).
	{id: 44294, castTimeIdx: 1, durIdx: 27, rangeIdx: 3, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicLeech, basePoints: 1499, dieSides: 1, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 44314 "Fel Explosion" (wago SpellName MATCHES;
	// C++ SPELL_FEL_EXPLOSION,
	// boss_selin_fireheart.cpp:43; Lua
	// eastern_kingdoms/boss_selin_fireheart.lua:121, 224 —
	// self-cast DoCastAOE, 2s loop) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_44294_44314_46153.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE (effect 2) bp974
	// die51 radiusIdx 31 (80yd, aux row added this
	// tick) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur0 (no SpellDuration row 0; 10934
	// precedent), range1 Self Only 0yd (aux row
	// pre-existing), school Fire (0x4) — wago
	// description says Arcane damage, SpellMisc
	// SchoolMask 0x4 rules (seeded on data);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0 carried
	// as dispel 0, Mechanic 0, PreventionType 0x1,
	// StartRecoveryCategory 0) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); SpellAuraOptions
	// row EXISTS (ProcChance 101) — proc fields
	// not carried (no fields in the seed struct);
	// Attributes_0 0x10000 / Attributes_1 0x88 /
	// Attributes_2 0x4 not carried (no fields in
	// the seed struct); EffectBonusCoefficient 1
	// not carried (no fields in the seed struct).
	{id: 44314, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 974, dieSides: 51, radiusIdx: 31, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 46153 "Drain Mana" (wago SpellName MATCHES;
	// C++ SPELL_DRAIN_MANA (heroic only),
	// boss_selin_fireheart.cpp:45; Lua
	// eastern_kingdoms/boss_selin_fireheart.lua:123, 190 —
	// target-cast, 10s loop) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_44294_44314_46153.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_MANA_LEECH (aura 64) bp499 die1
	// auraPeriod 1000 tgtA TARGET_UNIT_TARGET_ENEMY
	// tgtB NONE; cast1 instant, dur28 5000ms (aux
	// row pre-existing), range36 Longer Range 0-45yd
	// (aux row pre-existing), school Arcane (0x40);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 1 (MAGIC)
	// carried as dispel 1, Mechanic 0,
	// PreventionType 0x1, StartRecoveryCategory 133)
	// — category/defense/prevention/recovery not
	// carried (no fields in the seed struct);
	// SpellAuraOptions row EXISTS (ProcChance 101)
	// — proc fields not carried (no fields in the
	// seed struct); Attributes_0 0x10000 /
	// Attributes_1 0x4004 not carried (no fields in
	// the seed struct); EffectBonusCoefficient 0.2
	// not carried (no fields in the seed struct).
	{id: 46153, castTimeIdx: 1, durIdx: 28, rangeIdx: 36, school: schoolArcane, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicManaLeech, basePoints: 499, dieSides: 1, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 15588 "Thunderclap" (wago SpellName MATCHES;
	// C++ SPELL_THUNDERCLAP,
	// boss_vanndar.cpp:34; Lua
	// eastern_kingdoms/boss_vanndar.lua:44, 87 —
	// victim-cast, 4s init -> {5,15}s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_15588_19135_20685.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE (effect 2) bp70
	// die9 radiusIdx 13 (10yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 APPLY_AURA
	// MOD_DECREASE_SPEED (aura 33) bp-41 die1
	// mechanic 11 radiusIdx 13 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY (per-effect
	// mechanic carried per the auraDecSpeed
	// bp-41 mechanic 11 precedent); eff2 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp-34 die1
	// radiusIdx 13 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur1 10000ms (aux row pre-existing), range1
	// Self Only 0yd (aux row pre-existing), school
	// Nature (0x8); SpellCategories row EXISTS
	// (Category 22, DefenseType 1 (Magic), DispelType
	// 1 (MAGIC) carried as dispel 1, Mechanic 11
	// carried as spell mechanic per the 24110/8122
	// precedent, PreventionType 0x1,
	// StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Attributes_0 0x10000 / Attributes_1 0x88 /
	// Attributes_5 0x8000 not carried (no fields in
	// the seed struct); EffectRealPointsPerLevel -0.5
	// (eff1) not carried (no fields in the seed
	// struct).
	{id: 15588, castTimeIdx: 1, durIdx: 1, rangeIdx: 1, school: schoolNature, dispel: 1, mechanic: 11, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 70, dieSides: 9, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -41, dieSides: 1, mechanic: 11, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -34, dieSides: 1, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 19135 "Avatar" (wago SpellName MATCHES;
	// C++ SPELL_AVATAR,
	// boss_vanndar.cpp:33; Lua
	// eastern_kingdoms/boss_vanndar.lua:43, 79 —
	// victim-cast, 3s init -> {15,20}s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_15588_19135_20685.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// (aura 79) bp49 die1 misc 1 (Physical school
	// mask) tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_RESISTANCE_PCT (aura 101) bp49 die1 misc 1
	// (Physical school mask) tgtA TARGET_UNIT_CASTER;
	// eff2 APPLY_AURA MOD_SCALE (aura 61) bp49 die1
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur8
	// 15000ms (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school
	// Physical (0x1); NO SpellCategories row; NO
	// SpellAuraOptions row; Attributes all 0x0.
	{id: 19135, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 49, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModResistPct, basePoints: 49, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 20685 "Storm Bolt" (wago SpellName MATCHES;
	// C++ SPELL_STORMBOLT ("not sure" in C++),
	// boss_vanndar.cpp:35; Lua
	// eastern_kingdoms/boss_vanndar.lua:45, 95 —
	// victim-cast, 6s init -> {10,25}s) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_15588_19135_20685.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE (effect 2) bp4274
	// die451 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// APPLY_AURA MOD_STUN (aura 12) bp0 die0 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast4 1000ms (aux
	// row pre-existing), dur31 8000ms (aux row
	// pre-existing), range5 Long Range 0-40yd (aux
	// row pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0 carried
	// as dispel 0, Mechanic 12 (STUN) carried as
	// spell mechanic per the 24110/8122 precedent,
	// PreventionType 0x1, StartRecoveryCategory 0)
	// — category/defense/prevention/recovery not
	// carried (no fields in the seed struct); NO
	// SpellAuraOptions row; Speed 20 /
	// Attributes_1 0x88 / Attributes_3 0x100 not
	// carried (no fields in the seed struct).
	{id: 20685, castTimeIdx: 4, durIdx: 31, rangeIdx: 5, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 4274, dieSides: 451, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, basePoints: 0, dieSides: 0, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 68821 "Chain Reaction" (wago SpellName MATCHES;
	// C++ SPELL_CHAIN_REACTION,
	// boss_apothecary_hummel.cpp:35; Lua
	// eastern_kingdoms/npc_apothecary_baxter.lua:55, 103 —
	// victim-cast, 12s init -> 25s re-arm, paired with
	// the 69218 table summon below) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_68821_68948_69218.txt, DifficultyID 0
	// rows): eff0 SCHOOL_DAMAGE (effect 2) bp8549
	// die901 radiusIdx 13 (10yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY (radiusIdx_1 8 NOT
	// carried — no radiusIdxB field in the seed
	// struct; 19703 radius-_1 precedent); eff1
	// APPLY_AURA MOD_ROOT (aura 26) bp-1 die1 tgtA
	// TARGET_UNIT_CASTER (self-root); cast14 3000ms
	// (aux row pre-existing), dur39 2000ms (aux row
	// pre-existing), range2 Combat Range 0-5yd (aux
	// row pre-existing), school Fire (0x4); NO
	// SpellCategories row; SpellAuraOptions row
	// EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes not carried (no fields in the seed
	// struct).
	{id: 68821, castTimeIdx: 14, durIdx: 39, rangeIdx: 2, school: schoolFire, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 8549, dieSides: 901, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 68948 "Irresistible Cologne Spray" (wago
	// SpellName MISMATCH vs C++/Lua "Cologne Spray";
	// seeded on data per the eramas_brightblaze 15503
	// / smolderweb 16104 precedent; C++
	// SPELL_COLOGNE_SPRAY,
	// boss_apothecary_hummel.cpp:39; Lua
	// eastern_kingdoms/npc_apothecary_baxter.lua:57, 90 —
	// victim-cast, 7s init -> 4s re-arm) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_68821_68948_69218.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA DUMMY (aura 4) bp10799
	// die2401 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// PERIODIC_DAMAGE (aura 3) bp3149 die701
	// auraPeriod 1000 radiusIdx 61 (14yd, aux row
	// added this tick) tgtA TARGET_UNIT_TARGET_ENEMY
	// tgtB TARGET_UNIT_CONE_ENEMY_24 (cone damage,
	// matches the C++ cologne-spray arm); cast1
	// instant, dur27 3000ms (aux row pre-existing),
	// range2 Combat Range 0-5yd (aux row
	// pre-existing), school Nature (0x8); NO
	// SpellCategories row; SpellAuraOptions row
	// EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes / EffectAttributes 0x20000 (eff1)
	// not carried (no fields in the seed struct).
	{id: 68948, castTimeIdx: 1, durIdx: 27, rangeIdx: 2, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, basePoints: 10799, dieSides: 2401, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 3149, dieSides: 701, auraPeriod: 1000, radiusIdx: 61, targetA: targetEnemy, targetB: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 69218 "[DND] Holiday - Valentine - Summon Table"
	// (wago SpellName MISMATCH vs C++/Lua "Summon
	// Table"; seeded on data per the eramas_brightblaze
	// 15503 / smolderweb 16104 precedent — C++-checked:
	// boss_apothecary_hummel.cpp:262/385 DoCastVictim(
	// SPELL_SUMMON_TABLE) immediately before the 68821
	// chain-reaction cast, and the wago SUMMON effect
	// summons creature 36710 with SummonProperties 64,
	// i.e. the encounter table; the [DND] Valentine
	// name is the DB2 name stored in this build; C++
	// SPELL_SUMMON_TABLE, boss_apothecary_hummel.cpp:36;
	// Lua eastern_kingdoms/npc_apothecary_baxter.lua:56,
	// 102 — victim-cast, paired with 68821) —
	// wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-17 verdict file
	// wago_spells_68821_68948_69218.txt, DifficultyID 0
	// rows): eff0 DUMMY (effect 3) bp-1 die1 tgtA
	// TARGET_UNIT_TARGET_ANY; eff1 SUMMON (effect 28)
	// bp-1 die1 misc 36710 (summon entry) miscB 64
	// (SummonProperties row pre-existing) tgtA
	// TARGET_DEST_CASTER; eff2 APPLY_AURA MOD_ROOT
	// (aura 26) bp-1 die1 tgtA TARGET_UNIT_CASTER
	// (self-root); cast1 instant, dur28 5000ms (aux
	// row pre-existing), range2 Combat Range 0-5yd
	// (aux row pre-existing), school Physical (0x1);
	// NO SpellCategories row; SpellAuraOptions row
	// EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes not carried (no fields in the seed
	// struct).
	{id: 69218, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, basePoints: -1, dieSides: 1, targetA: targetUnitTargetAny, chainAmplitude: 1},
		{effect: effSummon, basePoints: -1, dieSides: 1, misc: 36710, miscB: 64, targetA: targetDestCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50276 "Chuck Mug" (wago SpellName MATCHES;
	// C++ SPELL_CHUCK_MUG,
	// boss_coren_direbrew.cpp:68; Lua
	// eastern_kingdoms/npc_coren_direbrew_sisters.lua:50, 107 —
	// target-cast, 4s re-arm) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_50276_50278_52850.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE
	// (effect 2) bp2924 die151 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast14 3000ms (aux
	// row pre-existing), dur0 (no SpellDuration row
	// 0; 10934 precedent), range5 Long Range 0-40yd
	// (aux row pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0 carried
	// as dispel 0, Mechanic 0, PreventionType 0x1,
	// StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); NO SpellAuraOptions
	// row; Speed 24 / Attributes_0 0x90000 not
	// carried (no fields in the seed struct).
	{id: 50276, castTimeIdx: 14, durIdx: 0, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 2924, dieSides: 151, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 50278 "Barreled Control Aura" (wago SpellName
	// MATCHES; C++ SPELL_BARRELED_CONTROL_AURA,
	// boss_coren_direbrew.cpp:76; Lua
	// eastern_kingdoms/npc_coren_direbrew_sisters.lua:48, 119 —
	// self-cast, triggers SPELL_BARRELED 47442 every
	// 18s) — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-17 verdict file
	// wago_spells_50276_50278_52850.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_TRIGGER_SPELL
	// (aura 23) bp0 die0 auraPeriod 18000 trigger
	// 47442 tgtA TARGET_UNIT_CASTER (auraPeriodicTrig
	// + trigger + auraPeriod per the 63322/65100
	// precedent); cast1 instant, dur21 -1 infinite
	// (aux row pre-existing), range1 Self Only 0yd
	// (aux row pre-existing), school Physical (0x1);
	// NO SpellCategories row; SpellAuraOptions row
	// EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes_0 0x100 / Attributes_1 0x10000000
	// not carried (no fields in the seed struct).
	// NOTE: the triggered spell 47442 (SPELL_BARRELED)
	// is itself unseeded — the 18s trigger tick will
	// resolve to a zero Spell in a seed-only
	// deployment until 47442 is seeded.
	{id: 50278, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 18000, trigger: 47442, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 52850 "Port to Coren" (wago SpellName MATCHES;
	// C++ SPELL_PORT_TO_COREN,
	// boss_coren_direbrew.cpp:69; Lua
	// eastern_kingdoms/npc_coren_direbrew_sisters.lua:47, 117 —
	// self-cast) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-17 verdict file
	// wago_spells_50276_50278_52850.txt, DifficultyID 0
	// rows): eff0 TELEPORT_UNITS (effect 5) tgtA
	// TARGET_UNIT_CASTER tgtB TARGET_DEST_NEARBY_ENTRY
	// (29967 effTeleportUnits + targetDestNearbyEntry
	// precedent); cast1 instant, dur0 (no
	// SpellDuration row 0; 10934 precedent), range13
	// Anywhere - Unlimited 0-50000yd (aux row
	// pre-existing), school Physical (0x1); NO
	// SpellCategories row; NO SpellAuraOptions row;
	// Attributes_0 0x100 / Attributes_2 0x4 not
	// carried (no fields in the seed struct).
	{id: 52850, castTimeIdx: 1, durIdx: 0, rangeIdx: 13, school: schoolPhysical, effects: [3]effect{
		{effect: effTeleportUnits, targetA: targetCaster, targetB: targetDestNearbyEntry, chainAmplitude: 1},
	}},
	// 52894 "Anti-Magic Zone" (wago SpellName MATCHES;
	// C++ SPELL_ANTI_MAGIC_ZONE,
	// chapter2.cpp:94; Lua
	// eastern_kingdoms/npc_koltira_deathweaver.lua:163, 252 —
	// self-cast, INTRO_5) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_52894_52899_53627.txt,
	// DifficultyID 0 rows): eff0 APPLY_AREA_AURA_FRIEND
	// (effect 128) aura 87 MOD_DAMAGE_PERCENT_TAKEN
	// bp-86 die1 tgtA TARGET_UNIT_CASTER radiusIdx_0 8
	// (5yd, aux row pre-existing) misc 126 (school mask
	// 0x7E: all magic schools); cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range7 Ten yards
	// 0-10yd (aux row pre-existing), school Shadow
	// (0x20); NO SpellCategories row; NO
	// SpellAuraOptions row; Attributes_0 0x20040100 /
	// Attributes_1 0x4cc not carried (no fields in the
	// seed struct); RadiusIdx_1 8 not carried (19703
	// precedent — only _0 is carried).
	{id: 52894, castTimeIdx: 1, durIdx: 21, rangeIdx: 7, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAreaAuraFriend, aura: auraModDmgTaken, basePoints: -86, dieSides: 1, radiusIdx: 8, misc: 126, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 52899 "Koltira Transform" (wago SpellName MATCHES;
	// C++ SPELL_KOLTIRA_TRANSFORM,
	// chapter2.cpp:93; Lua
	// eastern_kingdoms/npc_koltira_deathweaver.lua:162, 259 —
	// self-cast, INTRO_4) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_52894_52899_53627.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA TRANSFORM
	// (aura 56) bp0 die0 misc 28447 (display ID) tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range1 Self Only
	// 0yd (aux row pre-existing), school Physical
	// (0x1); NO SpellCategories row; SpellAuraOptions
	// row EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes_0 0x180 not carried (no fields in
	// the seed struct).
	{id: 52899, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraTransform, misc: 28447, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 53627 "Hero Aggro Aura" (wago SpellName MISMATCH:
	// "Hero Aggro Aura" vs expected "Hero Aggro" —
	// seeded on data per the 15503/16104 precedent;
	// C++ SPELL_HERO_AGGRO, chapter2.cpp:95; Lua
	// eastern_kingdoms/npc_koltira_deathweaver.lua:164, 347 —
	// self-cast, OUTRO) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_52894_52899_53627.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp-1 die1
	// auraPeriod 6000 trigger 53628 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range1 Self Only
	// 0yd (aux row pre-existing), school Physical
	// (0x1); NO SpellCategories row; SpellAuraOptions
	// row EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// Attributes_0 0x180 not carried (no fields in
	// the seed struct). NOTE: the triggered spell
	// 53628 is itself unseeded — the 6s trigger tick
	// will resolve to a zero Spell in a seed-only
	// deployment until 53628 is seeded.
	{id: 53627, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 6000, trigger: 53628, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 6754 "Slap!" (wago SpellName MISMATCH:
	// "Slap!" vs expected "Slap" — seeded on data per
	// the 15503/16104 precedent; C++ SPELL_SLAP,
	// stratholme.cpp:205; Lua
	// eastern_kingdoms/npc_spectral_ghostly_citizen.lua:66, 96 —
	// target-cast, rude -> DoCast(player, SPELL_SLAP))
	// — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-17 verdict file
	// wago_spells_6754_16333_16336.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STUN (aura 12) bp0
	// die1 tgtA TARGET_UNIT_TARGET_ANY; cast1 instant,
	// dur27 3000ms (aux row pre-existing), range12
	// Interact Range 0-5yd Flags 0 (aux row
	// pre-existing), school Physical (0x1); NO
	// SpellCategories row; NO SpellAuraOptions row;
	// Attributes_0 0x10 not carried (no fields in the
	// seed struct).
	{id: 6754, castTimeIdx: 1, durIdx: 27, rangeIdx: 12, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, dieSides: 1, targetA: targetUnitTargetAny, chainAmplitude: 1},
	}},
	// 16333 "Debilitating Touch" (wago SpellName MATCHES;
	// C++ SPELL_DEBILITATING_TOUCH,
	// stratholme.cpp:204; Lua
	// eastern_kingdoms/npc_spectral_ghostly_citizen.lua:54, 160 —
	// target-cast, TouchTimer 2000 init / 7000 re-arm)
	// — wago.tools 3.4.0.47659 SpellEffect rows +
	// SpellMisc (2026-11-17 verdict file
	// wago_spells_6754_16333_16336.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_STAT (aura 29) bp-21
	// die1 misc 0 (Strength) tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff1 same bp-21 die1 misc 2 (Stamina) tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur4
	// 120000ms (aux row pre-existing), range4 Medium
	// Range 0-30yd (aux row pre-existing), school
	// Shadow (0x20); SpellCategories row EXISTS
	// (Category 0, DefenseType 1 (Magic), DispelType 1
	// (MAGIC) carried as dispel 1, Mechanic 0,
	// PreventionType 0x1) — category/defense/
	// prevention/recovery not carried (no fields in
	// the seed struct); SpellAuraOptions row EXISTS
	// (CumulativeAura 5, ProcChance 101) — proc fields
	// not carried (no fields in the seed struct);
	// Attributes_0 0x10010 not carried (no fields in
	// the seed struct).
	{id: 16333, castTimeIdx: 1, durIdx: 4, rangeIdx: 4, school: schoolShadow, dispel: 1, effects: [3]effect{
		{effect: effApplyAura, aura: auraModStat, basePoints: -21, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModStat, basePoints: -21, dieSides: 1, misc: 2, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 16336 "Haunting Phantoms" (wago SpellName MISMATCH:
	// "Haunting Phantoms" vs expected "Haunting Phantom"
	// — seeded on data per the 15503/16104 precedent;
	// C++ SPELL_HAUNTING_PHANTOM, stratholme.cpp:203; Lua
	// eastern_kingdoms/npc_spectral_ghostly_citizen.lua:53, 149 —
	// target-cast, non-triggered DoCast(target, 16336))
	// — wago.tools 3.4.0.47659 SpellEffect row +
	// SpellMisc (2026-11-17 verdict file
	// wago_spells_6754_16333_16336.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA DUMMY (aura 4) bp0 die0
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast5 2000ms (aux
	// row pre-existing), dur5 300000ms (aux row
	// pre-existing), range2 Combat Range 0-5yd Flags
	// 0x1 (aux row pre-existing), school Shadow (0x20);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 2 (CURSE)
	// carried as dispel 2, Mechanic 0, PreventionType
	// 0x1) — category/defense/prevention/recovery not
	// carried (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_1 0x800 not
	// carried (no fields in the seed struct).
	{id: 16336, castTimeIdx: 5, durIdx: 5, rangeIdx: 2, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 27123 "Cosmetic - Polymorph Impact (visual only)"
	// (wago SpellName MISMATCH vs expected "Polymorph
	// Visual" — seeded on data per the 15503/16104
	// precedent; C++ SPELL_POLYMORPH_VISUAL,
	// zone_isle_of_queldanas.cpp:99; Lua
	// eastern_kingdoms/zone_isle_of_queldanas.lua:92, 365 —
	// self-cast, OUTRO_3; the effect is a DUMMY visual,
	// matching the retail cosmetic name) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_27123_50689_70265.txt,
	// DifficultyID 0 rows): eff0 DUMMY (effect 3) bp-1
	// die1 tgtA TARGET_UNIT_CASTER; cast1 instant, dur0
	// (no SpellDuration row 0; 10934 precedent), range1
	// Self Only 0yd (aux row pre-existing), school
	// Physical (0x1); NO SpellCategories row; NO
	// SpellAuraOptions row; Attributes_0 0x100 not
	// carried (no fields in the seed struct).
	{id: 27123, castTimeIdx: 1, durIdx: 0, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effDummy, basePoints: -1, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 50689 "Blood Presence" (wago SpellName MATCHES;
	// C++ SPELL_BLOOD_PRESENCE,
	// zone_isle_of_queldanas.cpp:97; Lua
	// eastern_kingdoms/zone_isle_of_queldanas.lua:90, 308 —
	// self-cast, Morlen) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_27123_50689_70265.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DAMAGE_PERCENT_DONE (aura 79) bp14 die1 misc
	// 127 (all schools) tgtA TARGET_UNIT_CASTER; eff1
	// APPLY_AURA PROC_TRIGGER_SPELL (aura 42) bp3 die1
	// trigger 1206 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur30 1800000ms (aux row pre-existing),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Shadow (0x20); SpellCategories row EXISTS
	// (Category 0, DefenseType 1 (Magic), DispelType 0
	// carried as dispel 0, Mechanic 0, PreventionType
	// 0x1, StartRecoveryCategory 133) — category/
	// defense/prevention/recovery not carried (no
	// fields in the seed struct); SpellAuraOptions row
	// EXISTS (ProcChance 100, ProcTypeMask 0x255554) —
	// proc fields not carried (no fields in the seed
	// struct); Attributes_0 0x1000000 / Attributes_3
	// 0x10000 not carried (no fields in the seed
	// struct). NOTE: the triggered spell 1206 is
	// itself unseeded — the proc tick will resolve to
	// a zero Spell in a seed-only deployment until
	// 1206 is seeded.
	{id: 50689, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 14, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraProcTriggerSpell, basePoints: 3, dieSides: 1, trigger: 1206, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 70265 "Thalorien Dawnseeker Credit" (wago SpellName
	// MISMATCH vs expected "Kill Credit" — seeded on
	// data per the 15503/16104 precedent; C++ SPELL_KILL_CREDIT,
	// zone_isle_of_queldanas.cpp:98; Lua
	// eastern_kingdoms/zone_isle_of_queldanas.lua:91, 354 —
	// target-cast, OUTRO_1; effect 134 misc 37601 is the
	// Thalorien credit) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_27123_50689_70265.txt,
	// DifficultyID 0 rows): eff0 KILL_CREDIT2 (effect
	// 134; wago labels it KILL_CREDIT, C++
	// SharedDefines.h:945 names 134 KILL_CREDIT2)
	// radiusIdx_0 10 (30yd, aux row pre-existing) misc
	// 37601 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENTRY; cast1 instant, dur0
	// (no SpellDuration row 0; 10934 precedent),
	// range7 Ten yards 0-10yd (aux row pre-existing),
	// school Physical (0x1); NO SpellCategories row;
	// SpellAuraOptions row EXISTS (ProcChance 101) —
	// proc fields not carried (no fields in the seed
	// struct); Attributes_0 0x100 not carried (no
	// fields in the seed struct).
	{id: 70265, castTimeIdx: 1, durIdx: 0, rangeIdx: 7, school: schoolPhysical, effects: [3]effect{
		{effect: effKillCredit2, radiusIdx: 10, misc: 37601, targetA: targetSrcCaster, targetB: targetUnitSrcAreaEntry, chainAmplitude: 1},
	}},
	// 22857 "Retaliation" (wago SpellName MATCHES;
	// C++ SPELL_RETALIATION,
	// boss_watchkeeper_gargolmar.cpp:43; Lua
	// hellfireramparts/boss_watchkeeper_gargolmar.lua:95, 204 —
	// self-cast, scheduled 1s after health first drops
	// below 50%) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-17 verdict file
	// wago_spells_22857_30641_34645.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PROC_TRIGGER_SPELL (aura
	// 42) bp-1 die1 trigger 22858 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur8 15000ms (aux row
	// pre-existing), range1 Self Only 0yd (aux row
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 132,
	// DefenseType 2 (Melee), DispelType 0 carried as
	// dispel 0, Mechanic 0, PreventionType 0x2) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (ProcChance 100, ProcTypeMask 0x28) —
	// proc fields not carried (no fields in the seed
	// struct); EffectBonusCoefficient 1 not carried (no
	// field in the seed struct); Attributes_0 0x50010
	// not carried (no fields in the seed struct). NOTE:
	// the triggered spell 22858 is itself unseeded —
	// the proc tick will resolve to a zero Spell in a
	// seed-only deployment until 22858 is seeded.
	{id: 22857, castTimeIdx: 1, durIdx: 8, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, basePoints: -1, dieSides: 1, trigger: 22858, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 30641 "Mortal Wound" (wago SpellName MATCHES;
	// C++ SPELL_MORTAL_WOUND,
	// boss_watchkeeper_gargolmar.cpp:41; Lua
	// hellfireramparts/boss_watchkeeper_gargolmar.lua:93, 175 —
	// victim-cast, 5s init then {5s,13s}) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_22857_30641_34645.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE (effect 31)
	// bp149 die1 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// APPLY_AURA MOD_HEALING_PCT (aura 118) bp-6 die1
	// misc 127 (all schools) tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur18
	// 20000ms (aux row pre-existing), range2 Combat
	// Range 0-5yd Flags 0x1 (aux row pre-existing),
	// school Physical (0x1); SpellCategories row
	// EXISTS (Category 0, DefenseType 2 (Melee),
	// DispelType 0 carried as dispel 0, Mechanic 0,
	// PreventionType 0x2, StartRecoveryCategory 133)
	// — category/defense/prevention/recovery not
	// carried (no fields in the seed struct);
	// SpellAuraOptions row EXISTS (CumulativeAura 8,
	// ProcChance 101) — proc fields not carried (no
	// fields in the seed struct); Attributes_0 0x50010
	// not carried (no fields in the seed struct).
	{id: 30641, castTimeIdx: 1, durIdx: 18, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 149, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, basePoints: -6, dieSides: 1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 34645 "Surge" (wago SpellName MATCHES; C++
	// SPELL_SURGE, boss_watchkeeper_gargolmar.cpp:42;
	// Lua hellfireramparts/boss_watchkeeper_gargolmar.lua:94, 191 —
	// target-cast, 4s init then {5s,13s}) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_22857_30641_34645.txt, DifficultyID 0
	// rows): eff0 CHARGE (effect 96) tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 KNOCK_BACK
	// (effect 98) bp89 die21 misc 50 tgtA
	// TARGET_DEST_TARGET_ANY tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY (RadiusIndex_1 13
	// NOT carried — 19703 precedent, only _0 is
	// carried); eff2 SCHOOL_DAMAGE (effect 2) bp89
	// die21 radiusIdx_0 13 (10yd, aux row pre-existing)
	// tgtA TARGET_DEST_TARGET_ANY tgtB
	// TARGET_UNIT_DEST_AREA_ENEMY; cast1 instant, dur0
	// (no SpellDuration row 0; 10934 precedent),
	// range5 Long Range 0-40yd (aux row pre-existing),
	// school Physical (0x1); SpellCategories row
	// EXISTS (Category 0, DefenseType 1 (Magic),
	// DispelType 0 carried as dispel 0, Mechanic 0,
	// PreventionType 0x1) — category/defense/
	// prevention/recovery not carried (no fields in
	// the seed struct); NO SpellAuraOptions row;
	// EffectBonusCoefficient 1 on eff1/eff2 not
	// carried (no field in the seed struct);
	// Attributes_0 0x80010 / Attributes_1 0x88 not
	// carried (no fields in the seed struct).
	{id: 34645, castTimeIdx: 1, durIdx: 0, rangeIdx: 5, school: schoolPhysical, effects: [3]effect{
		{effect: effCharge, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effKnockBack, basePoints: 89, dieSides: 21, misc: 50, targetA: targetDestTargetAny, targetB: targetUnitDestAreaEnemy, chainAmplitude: 1},
		{effect: effSchoolDamage, basePoints: 89, dieSides: 21, radiusIdx: 13, targetA: targetDestTargetAny, targetB: targetUnitDestAreaEnemy, chainAmplitude: 1},
	}},
	// 31422 "Time Stop" (wago SpellName MATCHES; C++
	// SPELL_TIME_STOP, boss_aeonus.cpp:40; Lua
	// kalimdor/boss_aeonus.lua:77, 139 — victim-cast,
	// EVENT_TIMESTOP) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_31422_31473_37605.txt, saved by
	// a sibling tick from the same handoff b2baaa81,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_STUN
	// (aura 12) bp0 die0 radiusIdx_0 27 (50yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur35
	// 4000ms (aux row pre-existing), range1 Self Only
	// 0yd (aux row pre-existing), school Arcane (0x40);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 12
	// carried as mechanic 12, PreventionType 0x1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_2 0x4 not
	// carried (no fields in the seed struct).
	{id: 31422, castTimeIdx: 1, durIdx: 35, rangeIdx: 1, school: schoolArcane, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, radiusIdx: 27, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 31473 "Sand Breath" (wago SpellName MATCHES; C++
	// SPELL_SAND_BREATH, boss_aeonus.cpp:42; Lua
	// kalimdor/boss_aeonus.lua:79, 121 — victim-cast,
	// EVENT_SANDBREATH) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_31422_31473_37605.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp3062
	// die875 radiusIdx_0 9 (20yd, aux row pre-existing)
	// tgtA TARGET_UNIT_CONE_ENEMY_24; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp-101 die1 radius9
	// tgtA TARGET_UNIT_CONE_ENEMY_24; eff2 APPLY_AURA
	// MOD_DECREASE_SPEED (aura 33) bp-51 die1 radius9
	// tgtA TARGET_UNIT_CONE_ENEMY_24; cast5 2000ms (aux
	// row pre-existing), dur1 10000ms (aux row
	// pre-existing), range3 Twenty yards 0-20yd (aux
	// row pre-existing), school Arcane (0x40);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 11
	// carried as mechanic 11, PreventionType 0x1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (ProcChance 101) — proc fields not
	// carried (no fields in the seed struct);
	// EffectBonusCoefficient 1 / EffectAttributes
	// 0x20000 not carried (no fields in the seed
	// struct); Attributes_0 0x40010 / Attributes_1
	// 0x88 not carried (no fields in the seed struct).
	{id: 31473, castTimeIdx: 5, durIdx: 1, rangeIdx: 3, school: schoolArcane, mechanic: 11, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 3062, dieSides: 875, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -101, dieSides: 1, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraDecSpeed, basePoints: -51, dieSides: 1, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 37605 "Enrage" (wago SpellName MATCHES; C++
	// SPELL_ENRAGE, boss_aeonus.cpp:41; Lua
	// kalimdor/boss_aeonus.lua:78, 156 — self-cast,
	// EVENT_FRENZY) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-17 verdict file
	// wago_spells_31422_31473_37605.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_MELEE_HASTE (aura 138)
	// bp49 die1 tgtA TARGET_UNIT_CASTER; cast1 instant,
	// dur31 8000ms (aux row pre-existing), range1 Self
	// Only 0yd (aux row pre-existing), school Shadow
	// (SchoolMask 0x20 = 32; the sibling-saved verdict
	// file mislabels 0x20 as "(Nature)" — 0x8 is
	// Nature, 0x20 is Shadow per the school table);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 0, DispelType 9 (ENRAGE) carried as
	// dispel 9, Mechanic 0, PreventionType 0x0) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; EffectBonusCoefficient 1
	// not carried (no field in the seed struct).
	{id: 37605, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolShadow, dispel: 9, effects: [3]effect{
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12747 "Entangling Roots" (wago SpellName MATCHES; C++
	// SPELL_ENTANGLINGROOTS, boss_celebras_the_cursed.cpp:32;
	// Lua kalimdor/boss_celebras_the_cursed.lua:49, 111 —
	// victim-cast, EVENT_ENTANGLINGROOTS) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-17
	// verdict file wago_spells_12747_21807_21968.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_ROOT
	// (aura 26) bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY;
	// eff1 APPLY_AURA PERIODIC_DAMAGE (aura 3) bp9 die1
	// auraPeriod 3000 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast16 1500ms, dur1 10000ms, range4 Medium 0-30yd
	// (aux rows pre-existing), school Nature (0x8);
	// SpellCategories row EXISTS (Category 99,
	// DefenseType 1 (Magic), DispelType 1 (Magic) carried
	// as dispel 1, Mechanic 7 carried as mechanic 7,
	// PreventionType 1) — category/defense/prevention/
	// recovery not carried (no fields in the seed
	// struct); NO SpellAuraOptions row; Attributes_0
	// 1074331648 not carried (no fields in the seed
	// struct); eff1 EffectBonusCoefficient 0.1 not
	// carried (no field in the seed struct).
	{id: 12747, castTimeIdx: 16, durIdx: 1, rangeIdx: 4, school: schoolNature, dispel: 1, mechanic: 7, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 9, dieSides: 1, auraPeriod: 3000, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 21807 "Wrath" (wago SpellName MATCHES; C++
	// SPELL_WRATH, boss_celebras_the_cursed.cpp:31; Lua
	// kalimdor/boss_celebras_the_cursed.lua:48, 99 —
	// random-target cast, EVENT_WRATH) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_12747_21807_21968.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp84
	// die31 tgtA TARGET_UNIT_TARGET_ENEMY; cast5 2000ms,
	// dur0 no row, range5 Long 0-40yd (aux rows
	// pre-existing), school Nature (0x8); SpellCategories
	// row EXISTS (Category 0, DefenseType 1 (Magic),
	// DispelType 0, Mechanic 0, PreventionType 1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 589824 not
	// carried (no fields in the seed struct);
	// EffectBonusCoefficient 1 not carried (no field in
	// the seed struct).
	{id: 21807, castTimeIdx: 5, durIdx: 0, rangeIdx: 5, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 84, dieSides: 31, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 21968 "Corrupt Forces of Nature" (wago SpellName
	// MISMATCH vs expected "Corrupt Forces" — seeded on
	// data per quel'danas/ghostly-citizen precedent; C++
	// SPELL_CORRUPT_FORCES, boss_celebras_the_cursed.cpp:33;
	// Lua kalimdor/boss_celebras_the_cursed.lua:50, 122 —
	// self-cast, EVENT_CORRUPTFORCES) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_12747_21807_21968.txt,
	// DifficultyID 0 rows): eff0 SUMMON bp2 die1
	// radiusIdx_0 8 (5yd, aux row pre-existing) misc
	// 13743 miscB 61 tgtA TARGET_DEST_CASTER_SUMMON
	// (target 32; handoff guessed TARGET_UNIT_SUMMONER,
	// seeder const is targetDestCasterSummon);
	// cast1 instant, dur3 60000ms (aux row pre-existing),
	// range1 Self Only 0yd (aux row pre-existing),
	// school Nature (0x8); NO SpellCategories row;
	// NO SpellAuraOptions row; EffectAmplitude -5 not
	// carried (no field in the seed struct).
	{id: 21968, castTimeIdx: 1, durIdx: 3, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effSummon, basePoints: 2, dieSides: 1, radiusIdx: 8, misc: 13743, miscB: 61, targetA: targetDestCasterSummon, chainAmplitude: 1},
	}},
	// 60451 "Corruption of Time" (wago SpellName MATCHES;
	// C++ SPELL_CORRUPTION_OF_TIME_TARGET,
	// boss_infinite_corruptor.cpp:30; Lua
	// kalimdor/boss_infinite_corruptor.lua:116, 208 —
	// target self-cast triggered, EVENT_CORRUPTION_OF_TIME)
	// — wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_60451_60588_60590.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA DUMMY (aura 4) bp0 die0
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range1 Self Only
	// 0yd (aux row pre-existing), school Shadow (0x20);
	// NO SpellCategories row; SpellAuraOptions row
	// EXISTS (CumulativeAura 0, ProcChance 101) — not
	// carried (no fields in the seed struct);
	// Attributes_4 0x1 not carried (no fields in the
	// seed struct).
	{id: 60451, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 60588 "Corrupting Blight" (wago SpellName MATCHES;
	// C++ SPELL_CORRUPTING_BLIGHT,
	// boss_infinite_corruptor.cpp:27; Lua
	// kalimdor/boss_infinite_corruptor.lua:112, 174 —
	// random-target cast, EVENT_CORRUPTING_BLIGHT) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_60451_60588_60590.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA PERIODIC_DAMAGE_PERCENT
	// (aura 89) bp7 die1 auraPeriod 3000 misc 127 tgtA
	// TARGET_UNIT_TARGET_ENEMY; cast1 instant, dur4
	// 120000ms, range14 Extra Long 0-60yd (aux rows
	// pre-existing), school Shadow (0x20); SpellCategories
	// row EXISTS (Category 0, DefenseType 1 (Magic),
	// DispelType 0, Mechanic 0, PreventionType 0x1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (CumulativeAura 5, ProcChance 101) —
	// not carried (no fields in the seed struct);
	// Attributes_1 0x88 / Attributes_3 0x100 not carried
	// (no fields in the seed struct).
	{id: 60588, castTimeIdx: 1, durIdx: 4, rangeIdx: 14, school: schoolShadow, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmgPct, basePoints: 7, dieSides: 1, auraPeriod: 3000, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 60590 "Void Strike" (wago SpellName MATCHES; C++
	// SPELL_VOID_STRIKE, boss_infinite_corruptor.cpp:28;
	// Lua kalimdor/boss_infinite_corruptor.lua:113, 192 —
	// victim-cast, EVENT_VOID_STRIKE) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_60451_60588_60590.txt,
	// DifficultyID 0 rows): eff0 WEAPON_PERCENT_DAMAGE
	// bp99 die1 tgtA TARGET_UNIT_TARGET_ENEMY; cast1
	// instant, dur0 no row, range2 Combat 0-5yd (aux row
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 2 (Melee), DispelType 0, Mechanic 0,
	// PreventionType 0x2) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// SpellAuraOptions row EXISTS (CumulativeAura 0,
	// ProcChance 101) — not carried (no fields in the
	// seed struct); Attributes_0 0x40010 not carried
	// (no fields in the seed struct).
	{id: 60590, castTimeIdx: 1, durIdx: 0, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 99, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 8399 "Sleep" (wago SpellName MATCHES; C++
	// SPELL_SLEEP, boss_kelris.cpp:25; Lua
	// kalimdor/boss_kelris.lua:53, 131 — random-target
	// cast, EVENT_SLEEP) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_8399_8734_15587.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_STUN
	// (aura 12) bp0 die0 radiusIdx_0 8 (5yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast50 1300ms (aux row
	// ADDED this tick — was absent from the seed),
	// dur1 10000ms, range4 Medium 0-30yd (aux rows
	// pre-existing), school Shadow (0x20);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 1 (Magic) carried
	// as dispel 1, Mechanic 10 carried as mechanic 10,
	// PreventionType 0x1) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x40110000 /
	// Attributes_1 0x40088 not carried (no fields in the
	// seed struct).
	{id: 8399, castTimeIdx: 50, durIdx: 1, rangeIdx: 4, school: schoolShadow, dispel: 1, mechanic: 10, effects: [3]effect{
		{effect: effApplyAura, aura: auraStun, radiusIdx: 8, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 8734 "Blackfathom Channeling" (wago SpellName
	// MATCHES; C++ SPELL_BLACKFATHOM_CHANNELING,
	// boss_kelris.cpp:26; Lua kalimdor/boss_kelris.lua:54,
	// 156/163 — self-cast, Reset + EVENT_CHANNELING) —
	// wago.tools 3.4.0.47659 SpellEffect row + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_8399_8734_15587.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA DUMMY (aura 4) bp0 die0
	// tgtA TARGET_UNIT_CASTER; cast1 instant, dur21 -1
	// infinite (aux row pre-existing), range1 Self Only
	// 0yd (aux row pre-existing), school Physical (0x1);
	// NO SpellCategories row; NO SpellAuraOptions row;
	// Attributes_0 0x18000100 / Attributes_1 0x10000044
	// not carried (no fields in the seed struct).
	{id: 8734, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraDummy, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 15587 "Mind Blast" (wago SpellName MATCHES; C++
	// SPELL_MIND_BLAST, boss_kelris.cpp:24; Lua
	// kalimdor/boss_kelris.lua:52, 111 — victim-cast,
	// EVENT_MIND_BLAST) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_8399_8734_15587.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp138
	// die23 tgtA TARGET_UNIT_TARGET_ENEMY; cast16
	// 1500ms, dur0 no row, range4 Medium 0-30yd (aux
	// rows pre-existing), school Shadow (0x20);
	// SpellCategories row EXISTS (Category 19,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 0,
	// PreventionType 0x1) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0xd0000 not
	// carried (no fields in the seed struct);
	// EffectBonusCoefficient 1 not carried (no field in
	// the seed struct).
	{id: 15587, castTimeIdx: 16, durIdx: 0, rangeIdx: 4, school: schoolShadow, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 138, dieSides: 23, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 31909 "Whirlwind" (wago SpellName MATCHES; C++
	// SPELL_WHIRLWIND, boss_leutenant_drake.cpp:77; Lua
	// kalimdor/boss_lieutenant_drake.lua:94, 140 —
	// victim-cast, EVENT_WHIRLWIND) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_31909_31911_33789.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp-1 die1
	// auraPeriod 2000 trigger 31910 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur35 4000ms,
	// range2 Combat 0-5yd (aux rows pre-existing),
	// school Physical (0x1); SpellCategories row EXISTS
	// (Category 49, DefenseType 2 (Melee), DispelType 0,
	// Mechanic 0, PreventionType 0x0) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (CumulativeAura 0, ProcChance 101) —
	// not carried (no fields in the seed struct);
	// Attributes_0 0x50010 / Attributes_1 0x40 not
	// carried (no fields in the seed struct); NOTE
	// triggered 31910 itself unseeded.
	{id: 31909, castTimeIdx: 1, durIdx: 35, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 2000, trigger: 31910, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 31911 "Mortal Strike" (wago SpellName MATCHES; C++
	// SPELL_MORTAL_STRIKE, boss_leutenant_drake.cpp:79;
	// Lua kalimdor/boss_lieutenant_drake.lua:96, 166 —
	// victim-cast, EVENT_MORTALSTRIKE) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_31909_31911_33789.txt, DifficultyID 0
	// rows): eff0 WEAPON_PERCENT_DAMAGE bp149 die1
	// tgtA TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA
	// MOD_HEALING_PCT (aura 118) bp-51 die1 misc 127
	// tgtA TARGET_UNIT_TARGET_ENEMY; cast1 instant,
	// dur28 5000ms, range2 Combat 0-5yd (aux rows
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 22,
	// DefenseType 2 (Melee), DispelType 0, Mechanic 0,
	// PreventionType 0x2) — category/defense/prevention/
	// recovery not carried (no fields in the seed
	// struct); SpellAuraOptions row EXISTS
	// (CumulativeAura 0, ProcChance 101) — not carried
	// (no fields in the seed struct); Attributes_0
	// 0x50010 not carried (no fields in the seed
	// struct).
	{id: 31911, castTimeIdx: 1, durIdx: 28, rangeIdx: 2, school: schoolPhysical, effects: [3]effect{
		{effect: effWeaponPercentDamage, basePoints: 149, dieSides: 1, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModHealingPct, basePoints: -51, dieSides: 1, misc: 127, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 33789 "Frightening Shout" (wago SpellName MATCHES;
	// C++ SPELL_FRIGHTENING_SHOUT,
	// boss_leutenant_drake.cpp:80; Lua
	// kalimdor/boss_lieutenant_drake.lua:97, 153 —
	// victim-cast, EVENT_FRIGHTENINGSHOUT) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_31909_31911_33789.txt, DifficultyID 0
	// rows): eff0 TRIGGER_SPELL trigger 29544 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA MOD_FEAR
	// (aura 7) bp0 die0 radiusIdx_0 14 (8yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff2 APPLY_AURA
	// MOD_INCREASE_SPEED (aura 31) bp24 die1
	// radiusIdx_0 14 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur32
	// 6000ms, range7 Ten yards 0-10yd (aux rows
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 22,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 5
	// carried as mechanic 5, PreventionType 0x1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (CumulativeAura 0, ProcChance 100,
	// ProcTypeMask_0 0x800a22a8) — not carried (no
	// fields in the seed struct); Attributes_0 0x50010
	// not carried (no fields in the seed struct); NOTE
	// triggered 29544 itself unseeded; eff2
	// EffectBonusCoefficient 1 not carried (no field in
	// the seed struct).
	{id: 33789, castTimeIdx: 1, durIdx: 32, rangeIdx: 7, school: schoolPhysical, mechanic: 5, effects: [3]effect{
		{effect: effTriggerSpell, trigger: 29544, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModFear, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModIncreaseSpeed, basePoints: 24, dieSides: 1, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 52666 "Disease Expulsion" (wago SpellName MATCHES;
	// C++ SPELL_DISEASE_EXPULSION, boss_meathook.cpp:26;
	// Lua kalimdor/boss_meathook.lua:82, 191 — self-cast
	// (DoCastAOE), EVENT_DISEASE) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_52666_52696_58841.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp712
	// die75 radiusIdx_0 13 (10yd, aux row pre-existing)
	// tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 INTERRUPT_CAST
	// (effect 68) bp0 die0 radiusIdx_0 13 tgtA
	// TARGET_SRC_CASTER tgtB TARGET_UNIT_SRC_AREA_ENEMY
	// EffectMechanic 26 carried as effect mechanic 26;
	// cast1 instant, dur35 4000ms, range1 Self Only 0yd
	// (aux rows pre-existing), school Nature (0x8);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 0,
	// PreventionType 1) — category/defense/prevention/
	// recovery not carried (no fields in the seed
	// struct); SpellAuraOptions row EXISTS
	// (CumulativeAura 0, ProcChance 101) — not carried
	// (no fields in the seed struct); Attributes_0 16 /
	// Attributes_1 136 / Attributes_2 4 not carried (no
	// fields in the seed struct).
	{id: 52666, castTimeIdx: 1, durIdx: 35, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 712, dieSides: 75, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effInterruptCast, mechanic: 26, radiusIdx: 13, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 52696 "Constricting Chains" (wago SpellName MATCHES;
	// C++ SPELL_CONSTRICTING_CHAINS, boss_meathook.cpp:25;
	// Lua kalimdor/boss_meathook.lua:81, 170/174 —
	// random-target or victim cast, EVENT_CONSTRICTING) —
	// wago.tools 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_52666_52696_58841.txt, DifficultyID 0
	// rows, Misc/Radius/Implicit re-verified on a second
	// fresh read): eff0 APPLY_AURA PERIODIC_DAMAGE
	// (aura 3) bp999 die1 auraPeriod 1000 tgtA
	// TARGET_UNIT_TARGET_ENEMY; eff1 APPLY_AURA MOD_STUN
	// (aura 12) bp0 die0 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast1 instant, dur28 5000ms, range37 Extended
	// 0-50yd (aux rows pre-existing), school Physical
	// (0x1); SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 12
	// carried as mechanic 12, PreventionType 1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); SpellAuraOptions
	// row EXISTS (CumulativeAura 0, ProcChance 101) —
	// not carried (no fields in the seed struct);
	// Attributes_0 16 / Attributes_3 256 not carried
	// (no fields in the seed struct).
	{id: 52696, castTimeIdx: 1, durIdx: 28, rangeIdx: 37, school: schoolPhysical, mechanic: 12, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicDmg, basePoints: 999, dieSides: 1, auraPeriod: 1000, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraStun, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 58841 "Frenzy" (wago SpellName MATCHES; C++
	// SPELL_FRENZY, boss_meathook.cpp:27; Lua
	// kalimdor/boss_meathook.lua:83, 206 — self-cast,
	// EVENT_FRENZY) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-17 verdict file
	// wago_spells_52666_52696_58841.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_SCALE (aura 61, C++
	// SpellAuraDefines.h:141) bp9 die1 misc 1 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_DAMAGE_PERCENT_DONE (aura 79,
	// SpellAuraDefines.h:159) bp9 die1 misc 1 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur9 30000ms
	// (aux row pre-existing; CONFLICT RESOLVED: the
	// meathook verdict's 3000ms was a substring-filter
	// misread — exact-ID 9 re-read 2026-11-17, theradras
	// verdict wago_spells_21832_21869_21909.txt, confirms
	// 30000ms), range1 Self Only 0yd (aux row
	// pre-existing), school Physical (0x1); NO
	// SpellCategories row; SpellAuraOptions row EXISTS
	// (CumulativeAura 10, ProcChance 101) — not carried
	// (no fields in the seed struct); Attributes_0
	// 262160 / Attributes_3 268435456 not carried (no
	// fields in the seed struct).
	{id: 58841, castTimeIdx: 1, durIdx: 9, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModScale, basePoints: 9, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 9, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 21832 "Boulder" (wago SpellName MATCHES; C++
	// SPELL_BOULDER, boss_princess_theradras.cpp:32; Lua
	// kalimdor/boss_princess_theradras.lua:41, 100 —
	// random-target cast, EVENT_BOULDER) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc (2026-11-17
	// verdict file wago_spells_21832_21869_21909.txt,
	// DifficultyID 0 rows): eff0 SCHOOL_DAMAGE bp393
	// die113 tgtA TARGET_UNIT_TARGET_ENEMY; eff1
	// INTERRUPT_CAST (effect 68) bp0 die1 tgtA
	// TARGET_UNIT_TARGET_ENEMY EffectMechanic 26
	// carried as effect mechanic 26; eff2 TRIGGER_SPELL
	// trigger 22592 tgtA TARGET_UNIT_TARGET_ENEMY;
	// cast5 2000ms, dur1 10000ms, range54 Monster Shoot
	// 5-30yd (aux rows pre-existing), school Physical
	// (0x1); SpellCategories row EXISTS (Category 0,
	// DefenseType 3 (Ranged), DispelType 0, Mechanic 0,
	// PreventionType 0x2) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x410010
	// not carried (no fields in the seed struct); NOTE
	// triggered 22592 itself unseeded; EffectBonusCoefficient
	// 1 on all 3 effects not carried (no field in the
	// seed struct).
	{id: 21832, castTimeIdx: 5, durIdx: 1, rangeIdx: 54, school: schoolPhysical, effects: [3]effect{
		{effect: effSchoolDamage, basePoints: 393, dieSides: 113, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effInterruptCast, dieSides: 1, mechanic: 26, targetA: targetEnemy, chainAmplitude: 1},
		{effect: effTriggerSpell, trigger: 22592, targetA: targetEnemy, chainAmplitude: 1},
	}},
	// 21869 "Repulsive Gaze" (wago SpellName MATCHES; C++
	// SPELL_REPULSIVEGAZE, boss_princess_theradras.cpp:34;
	// Lua kalimdor/boss_princess_theradras.lua:43, 121 —
	// victim-cast, EVENT_REPULSIVEGAZE) — wago.tools
	// 3.4.0.47659 SpellEffect row + SpellMisc (2026-11-17
	// verdict file wago_spells_21832_21869_21909.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_FEAR
	// (aura 7) bp-1 die1 radiusIdx_0 14 (8yd, aux row
	// pre-existing) tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur31
	// 8000ms, range7 Ten yards 0-10yd (aux rows
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 5
	// carried as mechanic 5, PreventionType 0x1) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 0x50010 not
	// carried (no fields in the seed struct).
	{id: 21869, castTimeIdx: 1, durIdx: 31, rangeIdx: 7, school: schoolPhysical, mechanic: 5, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, basePoints: -1, dieSides: 1, radiusIdx: 14, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 21909 "Dust Field" (wago SpellName MATCHES; C++
	// SPELL_DUSTFIELD, boss_princess_theradras.cpp:31; Lua
	// kalimdor/boss_princess_theradras.lua:40, 88 —
	// self-cast, EVENT_DUSTFIELD) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_21832_21869_21909.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp0 die1
	// auraPeriod 1000 trigger 21868 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA MOD_ROOT
	// (aura 26) bp0 die0 tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur31 8000ms, range1 Self Only 0yd (aux
	// rows pre-existing), school Nature (0x8); NO
	// SpellCategories row; NO SpellAuraOptions row;
	// EffectBonusCoefficient 1 on both effects not
	// carried (no field in the seed struct); NOTE
	// triggered 21868 itself unseeded.
	{id: 21909, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolNature, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, dieSides: 1, auraPeriod: 1000, trigger: 21868, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraRoot, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 26083 "Whirlwind" (wago SpellName MATCHES; C++
	// SPELL_WHIRLWIND, boss_sartura.cpp:37; Lua
	// kalimdor/boss_sartura.lua:64, 124 — self-cast,
	// EVENT_WHIRLWIND) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_26083_28747_28798.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// PERIODIC_TRIGGER_SPELL (aura 23) bp-1 die1
	// auraPeriod 1000 trigger 26084 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp-9901 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA
	// MECHANIC_IMMUNITY (aura 77) bp0 die1 misc 12
	// (stun immunity carried here, not via
	// EffectMechanic) tgtA TARGET_UNIT_CASTER; cast1
	// instant, dur8 15000ms, range36 Longer Range
	// 0-45yd (aux rows pre-existing), school Physical
	// (0x1); SpellCategories row EXISTS (Category 49,
	// DefenseType 2 (Melee), DispelType 0, Mechanic 0,
	// PreventionType 0x0) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x50010 not
	// carried (no fields in the seed struct); NOTE
	// triggered 26084 itself unseeded; eff1/eff2
	// EffectBonusCoefficient 1 not carried (no field in
	// the seed struct).
	{id: 26083, castTimeIdx: 1, durIdx: 8, rangeIdx: 36, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraPeriodicTrig, basePoints: -1, dieSides: 1, auraPeriod: 1000, trigger: 26084, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -9901, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraMechanicImmunity, dieSides: 1, misc: 12, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28747 "Frenzy" (wago SpellName MISMATCH vs expected
	// "Enrage" — seeded on data per quel'danas/
	// ghostly-citizen precedent; settles the C++
	// "Not sure if right ID" comment on
	// boss_sartura.cpp:38 — the ID is right, the name
	// is Frenzy; C++ SPELL_ENRAGE, boss_sartura.cpp:38;
	// Lua kalimdor/boss_sartura.lua:65, 260 — self-cast,
	// EVENT_ENRAGE) — wago.tools 3.4.0.47659 SpellEffect
	// rows + SpellMisc (2026-11-17 verdict file
	// wago_spells_26083_28747_28798.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// (aura 79) bp99 die1 misc 1 tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp49 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE
	// (aura 61) bp14 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur6 600000ms (10 min), range1
	// Self Only 0yd (aux rows pre-existing), school
	// Physical (0x1); SpellCategories row EXISTS
	// (Category 21, DefenseType 0, DispelType 0,
	// Mechanic 0, PreventionType 0x0) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 0x40010 /
	// Attributes_3 0x10000000 not carried (no fields
	// in the seed struct).
	{id: 28747, castTimeIdx: 1, durIdx: 6, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 99, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 14, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 28798 "Frenzy" (wago SpellName MISMATCH vs expected
	// "Enrage" — seeded on data per quel'danas/
	// ghostly-citizen precedent; C++ SPELL_ENRAGEHARD,
	// boss_sartura.cpp:39; Lua
	// kalimdor/boss_sartura.lua:66, 227 — self-cast,
	// EVENT_ENRAGEHARD) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_26083_28747_28798.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_DAMAGE_PERCENT_DONE (aura 79) bp149 die1
	// misc 1 tgtA TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp49 die1 tgtA
	// TARGET_UNIT_CASTER; eff2 APPLY_AURA MOD_SCALE
	// (aura 61) bp49 die1 tgtA TARGET_UNIT_CASTER;
	// cast1 instant, dur30 1800000ms (30 min), range1
	// Self Only 0yd (aux rows pre-existing), school
	// Physical (0x1); SpellCategories row EXISTS
	// (Category 1152, DefenseType 0, DispelType 0,
	// Mechanic 0, PreventionType 0x0) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 0x40010 /
	// Attributes_3 0x10000000 not carried (no fields
	// in the seed struct). Differs from 28747 only in
	// durIdx 6->30, Category 21->1152, bp 99/49/14 ->
	// 149/49/49.
	{id: 28798, castTimeIdx: 1, durIdx: 30, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 149, dieSides: 1, misc: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModScale, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 8876 "Thrash" (wago SpellName MATCHES; C++ SPELL_THRASH,
	// boss_tuten_kash.cpp:24; Lua
	// kalimdor/boss_tuten_kash.lua:68, 190 — self-cast,
	// EVENT_THRASH) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-17 verdict file
	// wago_spells_8876_12252_12255.txt, DifficultyID 0
	// rows): 1 effect only, APPLY_AURA
	// PROC_TRIGGER_SPELL (aura 42) bp0 die1 trigger
	// 3391 tgtA TARGET_UNIT_CASTER; cast1 instant, dur21
	// -1 (infinite), range1 Self Only 0yd (aux rows
	// pre-existing), school Physical (0x1);
	// SpellCategories row EXISTS (Category 65,
	// DefenseType 2 (Melee), DispelType 0, Mechanic 0,
	// PreventionType 0x2) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// SpellAuraOptions row EXISTS (ProcChance 15,
	// ProcTypeMask_0 0x14, ProcCategoryRecovery 4000) —
	// proc chance/type NOT carried (no fields in the
	// seed struct); Attributes_0 0x40050 not carried
	// (no fields in the seed struct); NOTE triggered
	// 3391 itself unseeded.
	{id: 8876, castTimeIdx: 1, durIdx: 21, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraProcTriggerSpell, dieSides: 1, trigger: 3391, targetA: targetCaster, chainAmplitude: 1},
	}},
	// 12252 "Web Spray" (wago SpellName MATCHES; C++
	// SPELL_WEB_SPRAY, boss_tuten_kash.cpp:25; Lua
	// kalimdor/boss_tuten_kash.lua:69, 130 — target-cast,
	// EVENT_WEB_SPRAY) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_8876_12252_12255.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA MOD_ROOT
	// (aura 26) bp0 die0 radiusIdx 9 tgtA
	// TARGET_UNIT_CONE_ENEMY_24; eff1 TRIGGER_SPELL
	// (effect 64) bp0 die1 trigger 11920 radiusIdx 9
	// tgtA TARGET_UNIT_CONE_ENEMY_24; cast1 instant,
	// dur1 10000ms, range4 Medium Range 0-30yd, radius9
	// 20yd (aux rows pre-existing), school Physical
	// (0x1), mechanic 7 (SpellCategories Mechanic 7);
	// SpellCategories row EXISTS (Category 34,
	// DefenseType 1 (Magic), DispelType 0,
	// PreventionType 0x1) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x10 /
	// Attributes_1 0x88, Speed 12, EffectAttributes
	// 0x20000 not carried (no fields in the seed struct);
	// NOTE triggered 11920 itself unseeded.
	{id: 12252, mechanic: 7, castTimeIdx: 1, durIdx: 1, rangeIdx: 4, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraRoot, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
		{effect: effTriggerSpell, dieSides: 1, trigger: 11920, radiusIdx: 9, targetA: targetUnitConeEnemy, chainAmplitude: 1},
	}},
	// 12255 "Curse of Tuten'kash" (wago SpellName
	// MATCHES; C++ SPELL_CURSE_OF_TUTENKASH,
	// boss_tuten_kash.cpp:27; Lua
	// kalimdor/boss_tuten_kash.lua:71, 146 — self-cast,
	// EVENT_CURSE_OF_TUTENKASH) — wago.tools 3.4.0.47659
	// SpellEffect rows + SpellMisc (2026-11-17 verdict
	// file wago_spells_8876_12252_12255.txt,
	// DifficultyID 0 rows): eff0 APPLY_AURA
	// MOD_CASTING_SPEED_NOT_STACK (aura 65) bp-16 die1
	// radiusIdx 10 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp-19 die1 radiusIdx
	// 10 tgtA TARGET_SRC_CASTER tgtB
	// TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant, dur347
	// 900000ms (15 min), range1 Self Only 0yd, radius10
	// 30yd, school Shadow (0x20), dispel 2 (CURSE,
	// SpellCategories DispelType 2); SpellCategories row
	// EXISTS (Category 0, DefenseType 0, Mechanic 0,
	// PreventionType 0x0) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x10 not
	// carried (no fields in the seed struct).
	{id: 12255, castTimeIdx: 1, durIdx: 347, rangeIdx: 1, school: schoolShadow, dispel: 2, effects: [3]effect{
		{effect: effApplyAura, aura: auraModCastSpeedNotStack, basePoints: -16, dieSides: 1, radiusIdx: 10, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: -19, dieSides: 1, radiusIdx: 10, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 19408 "Panic" (wago SpellName MISMATCH vs expected
	// "Fear" — seeded on data per quel'danas/
	// ghostly-citizen precedent; C++ SPELL_FEAR,
	// boss_bug_trio.cpp:43; Lua
	// kalimdor/boss_yauj.lua:27, 73 — victim-cast,
	// EVENT_FEAR) — wago.tools 3.4.0.47659 SpellEffect
	// row + SpellMisc (2026-11-17 verdict file
	// wago_spells_19408_25807_34624.txt, DifficultyID 0
	// rows): 1 effect only, APPLY_AURA MOD_FEAR (aura
	// 7) bp0 die1 radiusIdx 11 tgtA TARGET_SRC_CASTER
	// tgtB TARGET_UNIT_SRC_AREA_ENEMY; cast1 instant,
	// dur31 8000ms, range1 Self Only 0yd, radius11
	// 45yd (aux rows pre-existing), school Shadow
	// (0x20), dispel 1, mechanic 5 (SpellCategories
	// DispelType 1, Mechanic 5); SpellCategories row
	// EXISTS (Category 0, DefenseType 1 (Magic),
	// PreventionType 1) — category/defense/prevention/
	// recovery not carried (no fields in the seed struct);
	// NO SpellAuraOptions row; Attributes_0 0x10 /
	// Attributes_1 0x88 / Attributes_2 0x4 and
	// EffectBonusCoefficient 1 not carried (no fields
	// in the seed struct).
	{id: 19408, castTimeIdx: 1, durIdx: 31, rangeIdx: 1, school: schoolShadow, dispel: 1, mechanic: 5, effects: [3]effect{
		{effect: effApplyAura, aura: auraModFear, dieSides: 1, radiusIdx: 11, targetA: targetSrcCaster, targetB: targetSrcAreaEnemy, chainAmplitude: 1},
	}},
	// 25807 "Great Heal" (wago SpellName MISMATCH vs
	// expected "Heal" — seeded on data per
	// quel'danas/ghostly-citizen precedent; C++
	// SPELL_HEAL, boss_bug_trio.cpp:42; Lua
	// kalimdor/boss_yauj.lua:29, 96/101/104 — cast on
	// Kri/Vem/self, EVENT_HEAL) — wago.tools 3.4.0.47659
	// SpellEffect row + SpellMisc (2026-11-17 verdict
	// file wago_spells_19408_25807_34624.txt,
	// DifficultyID 0 rows): 1 effect only, HEAL
	// (effect 10) bp69374 die11251 tgtA
	// TARGET_UNIT_TARGET_ALLY; cast5 2000ms, dur0 (no
	// duration), range13 Anywhere - Unlimited 0-50000yd
	// (aux rows pre-existing), school Holy (0x2);
	// SpellCategories row EXISTS (Category 0,
	// DefenseType 1 (Magic), DispelType 0, Mechanic 0,
	// PreventionType 1, StartRecoveryCategory 133) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 0x10000 and
	// EffectBonusCoefficient 1 not carried (no fields
	// in the seed struct).
	{id: 25807, castTimeIdx: 5, rangeIdx: 13, school: schoolHoly, effects: [3]effect{
		{effect: effHeal, basePoints: 69374, dieSides: 11251, targetA: targetUnitTargetAlly, chainAmplitude: 1},
	}},
	// 34624 "Frenzy" (wago SpellName MISMATCH vs expected
	// "Enrage" — seeded on data per quel'danas/
	// ghostly-citizen precedent; C++ SPELL_ENRAGE,
	// boss_bug_trio.cpp:37 (comment notes 25790 is cast
	// on gamers too); Lua kalimdor/boss_yauj.lua:28,
	// 125 — self-cast, EVENT_ENRAGE) — wago.tools
	// 3.4.0.47659 SpellEffect rows + SpellMisc
	// (2026-11-17 verdict file
	// wago_spells_19408_25807_34624.txt, DifficultyID 0
	// rows): eff0 APPLY_AURA MOD_DAMAGE_PERCENT_DONE
	// (aura 79) bp99 die1 misc 127 (all-schools mask —
	// cf. 28747/28798 misc 1 Physical only) tgtA
	// TARGET_UNIT_CASTER; eff1 APPLY_AURA
	// MOD_MELEE_HASTE (aura 138) bp49 die1 tgtA
	// TARGET_UNIT_CASTER; cast1 instant, dur32 6000ms,
	// range1 Self Only 0yd (aux rows pre-existing),
	// school Physical (0x1); SpellCategories row EXISTS
	// (Category 21, DefenseType 0, DispelType 0,
	// Mechanic 0, PreventionType 0) —
	// category/defense/prevention/recovery not carried
	// (no fields in the seed struct); NO
	// SpellAuraOptions row; Attributes_0 0x40010 /
	// Attributes_3 0x10000000 and eff0
	// RealPointsPerLevel 1 not carried (no fields in
	// the seed struct).
	{id: 34624, castTimeIdx: 1, durIdx: 32, rangeIdx: 1, school: schoolPhysical, effects: [3]effect{
		{effect: effApplyAura, aura: auraModDmgDone, basePoints: 99, dieSides: 1, misc: 127, targetA: targetCaster, chainAmplitude: 1},
		{effect: effApplyAura, aura: auraModMeleeHaste, basePoints: 49, dieSides: 1, targetA: targetCaster, chainAmplitude: 1},
	}},
}

func main() {
	out := "engine/data/wotlk/seeddbc"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		panic(err)
	}
	spellRows := make([][]uint32, 0, len(spells))
	for _, s := range spells {
		spellRows = append(spellRows, s.row())
	}
	if err := writeWDBC(filepath.Join(out, "Spell.dbc"), 234, spellRows); err != nil {
		panic(err)
	}
	// SpellDuration.dbc: ID, Duration, DurationPerLevel, MaxDuration.
	durations := [][]uint32{
		{1, 10000, 0, 10000},            // real SpellDuration 1 for 31266 (wago.tools 3.4.0.43659)
		{6, 600000, 0, 600000},          // real SpellDuration 6 (10 min) for MEMORY_* summons
		{3, 60000, 0, 60000},            // real SpellDuration 3 for 31331 (wago.tools 3.4.0.43659)
		{105, 9000, 0, 9000},            // real SpellDuration 105 for 31263 (wago.tools 3.4.0.43659)
		{23, 90000, 0, 90000},           // real SpellDuration 23 for 31264 (wago.tools 3.4.0.43659)
		{28, 5000, 0, 5000},             // real SpellDuration 28 for 31308 (wago.tools 3.4.0.43659)
		{21, 0xFFFFFFFF, 0, 0xFFFFFFFF}, // real SpellDuration 21 (-1 infinite) for 68415
		{32, 6000, 0, 6000},             // real SpellDuration 32 for 54690 (wago.tools 3.4.0.43659)
		{22, 45000, 0, 45000},           // real SpellDuration 22 for 15786 (wago.tools 3.4.0.43659)
		{31, 8000, 0, 8000},             // real SpellDuration 31 for 2139 Counterspell (wago.tools 3.4.0.43659)
		{35, 4000, 0, 4000},             // real SpellDuration 35 for 6552 Pummel / 47528 Mind Freeze (wago.tools 3.4.0.43659)
		{39, 2000, 0, 2000},             // real SpellDuration 39 for 57994 Wind Shear (wago.tools 3.4.0.43659)
		{18, 20000, 0, 20000},           // real SpellDuration 18 for 29946 Flame Wreath (wago.tools 3.4.0.47659, aux read 54197700)
		{90001, 2000, 0, 2000},
		{90002, 6000, 0, 6000},
		{90003, 8000, 0, 8000},
		{90004, 10000, 0, 10000},
		{90005, 30000, 0, 30000},
		{90006, 300000, 0, 300000},
		{90007, 180000, 0, 180000},
		{90008, 604800000, 0, 604800000},
		{90009, 4000, 0, 4000},
		{90010, 500, 0, 500},
		{4, 120000, 0, 120000},     // real SpellDuration 4 for 67530 (wago.tools 3.4.0.43659)
		{8, 15000, 0, 15000},       // real SpellDuration 8 for 49263 (wago.tools 3.4.0.43659)
		{27, 3000, 0, 3000},        // real SpellDuration 27 for 66083 (wago.tools 3.4.0.43659)
		{9, 30000, 0, 30000},       // real SpellDuration 9 for 49519 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
		{85, 18000, 0, 18000},      // real SpellDuration 85 for 2767 (wago.tools 3.4.0.43659 SpellMisc DurationIndex 85) — value CONFIRMED 2026-11-16 by wago.tools 3.4.0.47659 SpellDuration row 85 (verdict wago_spells_50756_50757_50758.txt)
		{387, 16000, 0, 16000},     // real SpellDuration 387 for 50756 (wago.tools 3.4.0.47659 SpellDuration row 387, 2026-11-16 verdict wago_spells_50756_50757_50758.txt)
		{25, 30000, 0, 30000},      // real SpellDuration 25 (wago.tools 3.4.0.47659 SpellDuration row 25, browser 74701ec4) — CORRECTED 2026-11-16 from 180000ms (old value was a Wowhead display cross-check for 1834, never a DBC-row read); this also re-times the other durIdx-25 spells 1834/45532/16128 to 30000ms
		{367, 7200000, 0, 7200000}, // real SpellDuration 367 for 23261 (wago.tools 3.4.0.47659 SpellDuration row 367)
		{5, 300000, 0, 300000},     // real SpellDuration 5 for 18377 (wago.tools 3.4.0.47659 SpellDuration row 5, browser 32eeed6d)
		{36, 1000, 0, 1000},        // real SpellDuration 36 for 64562 (wago.tools 3.4.0.47659 SpellDuration row 36, 2026-11-15 verdict)
		{30, 1800000, 0, 1800000},  // real SpellDuration 30 for 12544 Frost Armor (wago.tools 3.4.0.47659 SpellDuration row 30, browser c857c690)
		{29, 12000, 0, 12000},      // real SpellDuration 29 for 35759 Felfire Shock (wago.tools 3.4.0.47659 SpellDuration row 29, browser 2f3bd931)
		{555, 4500, 0, 4500},       // real SpellDuration 555 for 36512 Knock Away (wago.tools 3.4.0.47659 SpellDuration row 555, browser 2f3bd931)
		{327, 500, 0, 500},         // real SpellDuration 327 for 34799 Arcane Devastation (wago.tools 3.4.0.47659 SpellDuration row 327, browser bb66d46a)
		{305, 14000, 0, 14000},     // real SpellDuration 305 for 25033 Lightning Cloud (wago.tools 3.4.0.47659 SpellDuration row 305, browser f28d47f3)
		{38, 11000, 0, 11000},      // real SpellDuration 38 for 31673 Foul Spores (wago.tools 3.4.0.47659 SpellDuration row 38, browser 45c90da6)
		{63, 25000, 0, 25000},      // real SpellDuration 63 for 36119 Void Zone (wago.tools 3.4.0.47659 SpellDuration row 63, 2026-11-16 verdict wago_spells_16045_36119.txt)
		{41, 360000, 0, 360000},    // real SpellDuration 41 for 30977 Conflagration Blast (wago.tools 3.4.0.47659 SpellDuration row 41, 2026-11-16 verdict wago_spells_30967_30977_31635.txt)
		{62, 75000, 0, 75000},      // real SpellDuration 62 for 28240 Poison Cloud (wago.tools 3.4.0.47659 SpellDuration row 62, 2026-11-16 verdict wago_spells_28157_28169_28240.txt)
		{86, 21000, 0, 21000},      // real SpellDuration 86 for 20294 Immolate (wago.tools 3.4.0.47659 SpellDuration row 86, 2026-11-16 verdict wago_spells_20294_17820_17467.txt)
		{347, 900000, 0, 900000},   // real SpellDuration 347 for 12255 Curse of Tuten'kash (wago.tools 3.4.0.47659 SpellDuration row 347, 2026-11-17 verdict wago_spells_8876_12252_12255.txt)
	}
	if err := writeWDBC(filepath.Join(out, "SpellDuration.dbc"), 4, durations); err != nil {
		panic(err)
	}
	// SpellRadius.dbc: ID, Radius, RadiusPerLevel, RadiusMax.
	// maxRadius >= radius on every row (store.go clamps to maxRadius
	// when level > 0 — Unit A gotcha).
	radii := [][]uint32{
		{1, f32(100), f32(0), f32(100)},
		{28, f32(50000), f32(0), f32(50000)}, // real SpellRadius 28 for 66545 (wago.tools 3.4.0.43659)
		{37, f32(7), f32(0), f32(7)},         // real SpellRadius 37 for 47310 (wago.tools 3.4.0.47659, 2026-11-16 verdict wago_spells_47310_47370_47369_47331.txt)
		{36, f32(0), f32(0), f32(0)},         // real SpellRadius 36 for 50415 (wago.tools 3.4.0.43659, 2026-11-13 22:17 verdict: all zeros)
		{13, f32(10), f32(0), f32(10)},       // real SpellRadius 13 for 65100 (wago.tools 3.4.0.43659)
		{11, f32(45), f32(0), f32(45)},       // real SpellRadius 11 for 6432 Smite Stomp (wago.tools 3.4.0.47659, 2026-11-16 verdict 8bda548f)
		{14, f32(8), f32(0), f32(8)},         // real SpellRadius 14 for 59335 (wago.tools 3.4.0.43659)
		{9, f32(20), f32(0), f32(20)},        // real SpellRadius 9 for 44194 (wago.tools 3.4.0.43659)
		{18, f32(15), f32(0), f32(15)},       // real SpellRadius 18 for 44196 (wago.tools 3.4.0.43659)
		{8, f32(5), f32(0), f32(5)},          // real SpellRadius 8 for 31263 (wago.tools 3.4.0.43659)
		{15, f32(3), f32(0), f32(3)},         // real SpellRadius 15 for 31264 (wago.tools 3.4.0.43659)
		{10, f32(30), f32(0), f32(30)},       // real SpellRadius 10 for 31308 (wago.tools 3.4.0.43659)
		{20, f32(25), f32(0), f32(25)},       // real SpellRadius 20 for 38196 (wago.tools 3.4.0.47659)
		{91001, f32(8), f32(0), f32(8)},
		{91002, f32(10), f32(0), f32(10)},
		{91003, f32(15), f32(0), f32(15)},
		{91004, f32(30), f32(0), f32(30)},
		{91005, f32(45), f32(0), f32(45)},
		{91006, f32(200), f32(0), f32(200)},
		{91007, f32(50000), f32(0), f32(50000)},
		{91008, f32(100), f32(0), f32(100)},
		{91009, f32(1), f32(0), f32(1)},
		{17, f32(13), f32(0), f32(13)},   // real SpellRadius 17 for 30035/29951 (wago.tools 3.4.0.47659, aux read 54197700)
		{19, f32(18), f32(0), f32(18)},   // real SpellRadius 19 for 30035 eff1 (wago.tools 3.4.0.47659, aux read 54197700)
		{27, f32(50), f32(0), f32(50)},   // real SpellRadius 27 for 32265 (wago.tools 3.4.0.47659, aux read 54197700)
		{38, f32(21), f32(0), f32(21)},   // real SpellRadius 38 for 29973 (wago.tools 3.4.0.47659, aux read 54197700)
		{12, f32(100), f32(0), f32(100)}, // real SpellRadius 12 for 29963 eff1 (wago.tools 3.4.0.47659, aux read 54197700)
		{30, f32(500), f32(0), f32(500)}, // real SpellRadius 30 for 42442 (wago.tools 3.4.0.47659, aux read 54197700)
		{40, f32(9), f32(0), f32(9)},     // real SpellRadius 40 = 29961 RadiusIndex_1 (wago.tools 3.4.0.47659, aux read 54197700); UNREFERENCED — no seeded spell uses radiusIdx 40 until the _0-vs-_1 radius question settles
		{33, f32(99), f32(0), f32(99)},   // real SpellRadius 33 for 51110 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
		{48, f32(60), f32(0), f32(60)},   // real SpellRadius 48 for 50774 (wago.tools 3.4.0.47659, 2026-11-16 verdict 6827d471)
		{29, f32(6), f32(0), f32(6)},     // real SpellRadius 29 for 33814 (wago.tools 3.4.0.47659)
		{26, f32(4), f32(0), f32(4)},     // real SpellRadius 26 for 59687 Explosion (wago.tools 3.4.0.47659 SpellRadius row 26, browser abab7d59)
		{23, f32(40), f32(0), f32(40)},   // real SpellRadius 23 for 36127 Shadow Nova (wago.tools 3.4.0.47659 SpellRadius row 23, 2026-11-16 verdict wago_spells_36127_36123.txt)
		{32, f32(12), f32(0), f32(12)},   // real SpellRadius 32 for 32302/36383 (wago.tools 3.4.0.47659 SpellRadius row 32, 2026-11-16 verdict wago_spells_32302_36383_42075.txt)
		{16, f32(1), f32(0), f32(1)},     // real SpellRadius 16 for 26392 eff0 (wago.tools 3.4.0.47659 SpellRadius row 16, 2026-11-16 verdict wago_spells_15284_26392_26540.txt)
		{24, f32(65), f32(0), f32(65)},   // real SpellRadius 24 for 32309 (wago.tools 3.4.0.47659 SpellRadius row 24, 2026-11-16 verdict wago_spells_28131_28308_32309.txt)
		{52, f32(5), f32(0), f32(5)},     // real SpellRadius 52 for 67716 (wago.tools 3.4.0.47659 SpellRadius row 52, 2026-11-16 verdict wago_spells_29426_67541_67716.txt)
		{31, f32(80), f32(0), f32(80)},   // real SpellRadius 31 for 44314 (wago.tools 3.4.0.47659 SpellRadius row 31, 2026-11-17 verdict wago_spells_44294_44314_46153.txt)
		{61, f32(14), f32(0), f32(14)},   // real SpellRadius 61 for 68948 (wago.tools 3.4.0.47659 SpellRadius row 61, 2026-11-17 verdict wago_spells_68821_68948_69218.txt)
	}
	if err := writeWDBC(filepath.Join(out, "SpellRadius.dbc"), 4, radii); err != nil {
		panic(err)
	}
	// SpellCastTimes.dbc: ID, Base, PerLevel (real "nif" layout).
	castTimes := [][]uint32{
		{5, 2000, 0}, // real SpellCastTimes 5 for 44196 (wago.tools 3.4.0.43659)
		{6, 5000, 0}, // real SpellCastTimes 6 for 31263 (wago.tools 3.4.0.43659)
		{3, 500, 0},  // real SpellCastTimes 3 for 31264 (wago.tools 3.4.0.43659)
		{4, 1000, 0}, // real SpellCastTimes 4 for 31266 (wago.tools 3.4.0.43659)
		{92001, 2000, 0},
		{92002, 3000, 0},
		{2, 250, 0},    // real SpellCastTimes 2 for 65147 (wago.tools 3.4.0.43659)
		{14, 3000, 0},  // real SpellCastTimes 14 for 67528 (wago.tools 3.4.0.43659)
		{16, 1500, 0},  // real SpellCastTimes 16 for 66043/67529/67534 (wago.tools 3.4.0.43659)
		{19, 2500, 0},  // real SpellCastTimes 19 for 66042 (wago.tools 3.4.0.43659)
		{90, 1700, 0},  // real SpellCastTimes 90 for 65868 (wago.tools 3.4.0.43659)
		{7, 10000, 0},  // real SpellCastTimes 7 for 29973 Arcane Explosion (wago.tools 3.4.0.47659, aux read 54197700)
		{1, 0, 0},      // real SpellCastTimes 1 (instant) for 41342 Shared Rule (wago.tools 3.4.0.43659)
		{15, 4000, 0},  // real SpellCastTimes 15 for 50476/50495/50496 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
		{20, 2500, 0},  // real SpellCastTimes 20 for 25263 Holy Light (wago.tools 3.4.0.47659, 2026-11-16 verdict 9cf61680)
		{170, 8000, 0}, // real SpellCastTimes 170 for 51110 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
		{50, 1300, 0},  // real SpellCastTimes 50 for 8399 (wago.tools 3.4.0.47659, 2026-11-17 verdict wago_spells_8399_8734_15587.txt)
	}
	if err := writeWDBC(filepath.Join(out, "SpellCastTimes.dbc"), 3, castTimes); err != nil {
		panic(err)
	}
	// SpellRange.dbc: ID, MinHostile, MinFriendly, MaxHostile,
	// MaxFriendly, Flags (floats for the ranges).
	ranges := [][]uint32{
		{152, f32(0), f32(0), f32(150), f32(150), 0}, // real SpellRange 152 "Super Long" for seat spells (wago.tools 3.4.0.47659, 2026-11-13 23:00 verdict)
		{4, f32(0), f32(0), f32(30), f32(30), 0},     // real SpellRange 4 "Medium Range" for 54690 (wago.tools 3.4.0.43659)
		{74, f32(0), f32(0), f32(30), f32(30), 2},    // real SpellRange 74 "Ranged Weapon" for 59335 (wago.tools 3.4.0.43659, Flags 0x2)
		{5, f32(0), f32(0), f32(40), f32(40), 0},     // real SpellRange 5 "Long Range" for 31263 (wago.tools 3.4.0.43659)
		{6, f32(0), f32(0), f32(100), f32(100), 0},   // real SpellRange 6 "Vision Range" for Aran bolts 29954/29953/29955 (wago.tools 3.4.0.43659)
		{135, f32(0), f32(0), f32(100), f32(100), 0}, // real SpellRange 135 "Tower 100" for 53456 (wago.tools 3.4.0.43659); also repairs the dangling 42198/42937 rangeIdx 135 references
		{1, f32(0), f32(0), f32(0), f32(0), 0},       // real SpellRange 1 "Self Only" for 31308 (wago.tools 3.4.0.43659)
		{2, f32(0), f32(0), f32(5), f32(5), 1},       // real SpellRange 2 "Combat Range" for interrupt spells (wago.tools 3.4.0.43659, Flags 0x1)
		{34, f32(0), f32(0), f32(25), f32(25), 0},    // real SpellRange 34 "Twenty-Five yards" for 57994 (wago.tools 3.4.0.43659)
		{3, f32(0), f32(0), f32(20), f32(20), 0},     // real SpellRange 3 "Twenty yards" for 15574 (wago.tools 3.4.0.47659)
		{93001, f32(0), f32(0), f32(0), f32(0), 0},
		{93002, f32(0), f32(0), f32(5), f32(5), 0},
		{93003, f32(8), f32(8), f32(25), f32(25), 0},
		{93004, f32(0), f32(0), f32(15), f32(15), 0},
		{93005, f32(0), f32(0), f32(20), f32(20), 0},
		{93006, f32(0), f32(0), f32(60), f32(60), 0},
		{93007, f32(0), f32(0), f32(100), f32(100), 0},
		{93008, f32(0), f32(0), f32(50000), f32(50000), 0},
		{36, f32(0), f32(0), f32(45), f32(45), 0},       // real SpellRange 36 "Longer Range" for 29946/32265 (wago.tools 3.4.0.47659, aux read 54197700)
		{37, f32(0), f32(0), f32(50), f32(50), 0},       // real SpellRange 37 "Extended Range" for 45532 (wago.tools 3.4.0.47659 SpellRange row 37, browser 4d453dd5)
		{11, f32(0), f32(0), f32(15), f32(15), 0},       // real SpellRange 11 "Fifteen yards" for 29961 (wago.tools 3.4.0.47659, aux read 54197700)
		{13, f32(0), f32(0), f32(50000), f32(50000), 0}, // real SpellRange 13 "Anywhere - Unlimited" for 29967 (wago.tools 3.4.0.47659, aux read 54197700)
		{95, f32(8), f32(8), f32(25), f32(25), 0},       // real SpellRange 95 for 63010 (wago.tools 3.4.0.43659)
		{9, f32(10), f32(10), f32(30), f32(30), 0},      // real SpellRange 9 for 65147 (wago.tools 3.4.0.43659)
		{7, f32(0), f32(0), f32(10), f32(10), 0},        // real SpellRange 7 for 49256 (wago.tools 3.4.0.43659)
		{54, f32(5), f32(5), f32(30), f32(30), 0},       // real SpellRange 54 for 66081 (wago.tools 3.4.0.43659)
		{12, f32(0), f32(0), f32(5), f32(5), 0},         // real SpellRange 12 "Interact Range" for 23261 (wago.tools 3.4.0.47659 SpellRange row 12, Flags 0)
		{137, f32(0), f32(0), f32(8), f32(8), 0},        // real SpellRange 137 \"Eight yards\" for 34697 Allergic Reaction (wago.tools 3.4.0.47659 SpellRange row 137, browser bb66d46a)
		{35, f32(0), f32(0), f32(35), f32(35), 0},       // real SpellRange 35 "Medium-Long Range" for 38739 Acid Geyser (wago.tools 3.4.0.47659 SpellRange row 35, browser 45c90da6)
		{134, f32(0), f32(0), f32(80), f32(80), 0},      // real SpellRange 134 "Tower 80" for 30967 Fireball (wago.tools 3.4.0.47659 SpellRange row 134, 2026-11-16 verdict wago_spells_30967_30977_31635.txt)
		{14, f32(0), f32(0), f32(60), f32(60), 0},       // real SpellRange 14 "Extra Long Range" for 57942 (wago.tools 3.4.0.47659, 2026-11-16 verdict wago_spells_57941_57942_57949.txt)
	}
	if err := writeWDBC(filepath.Join(out, "SpellRange.dbc"), 6, ranges); err != nil {
		panic(err)
	}
	// SummonProperties.dbc: ID, Control, Faction, Title, Slot, Flags.
	// Row 64 is REAL (wago.tools 3.4.0.43659 SummonProperties,
	// 2026-11-15): (Control 0/WILD, Faction 0, Title 0, Slot 0,
	// Flags_0 0x100; Flags_1 0x0 is not encoded — the seed has one
	// Flags field). It REPLACES the 2026-11-13 INFERRED row (Faction
	// 14, Title 2/GUARDIAN), which was justified by C++ EffectSummon
	// multi-summon behavior + the Go GO-summon leg gates
	// (go_origin_summon.go:89-98) + hostile-guardian fight behavior
	// (boss_marli.cpp:119). C++ consequence (SpellEffects.cpp
	// EffectSummon): WILD + Flags 0x100 (no 512 bit) + Title 0 falls
	// through to the default TempSummon branch, NOT SummonGuardian;
	// the Go mirror (summonPropertiesIsGuardian, spells.go) does the
	// same. The multi-summon list still names 64, so counts stay
	// damage-or-1. A 3.3.5 client-DBC re-read is the remaining
	// confirmation (wago build is 3.4.0). Row 61 is REAL (wago.tools 3.4.0.43659
	// SummonProperties: Control 1/Guardian, Faction 0, Title 2,
	// Slot 0, Flags_0 0x2; Flags_1 0x0 is not encoded — the seed has
	// one Flags field). Row 81 is REAL (wago.tools 3.4.0.43659
	// SummonProperties: Control 1/Guardian, Faction 0, Title 4/
	// TOTEM, Slot 2 (earth totem), Flags_0 0x100002).
	summonProps := [][]uint32{{64, 0, 0, 0, 0, 0x100}, {61, 1, 0, 2, 0, 2}, {81, 1, 0, 4, 2, 0x100002}, {1362, 0, 0, 0, 0, 0x800}, {66, 0, 0, 0, 0, 0x1}, {121, 1, 0, 0, 0, 0x2}} // real SummonProperties 1362 for 69424 (wago.tools 3.4.0.47659, browser 7f680372); real SummonProperties 66 for 43289 (wago.tools 3.4.0.47659, browser 3ce851bf); real SummonProperties 121 for 11086 (wago.tools 3.4.0.47659, browser 3e2e3ba6)
	if err := writeWDBC(filepath.Join(out, "SummonProperties.dbc"), 6, summonProps); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d spells + aux tables to %s\n", len(spells), out)
}
