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
	effSchoolDamage            = 2
	effDummy                   = 3
	effTeleportUnits           = 5 // SPELL_EFFECT_TELEPORT_UNITS (SharedDefines.h:816)
	effApplyAura               = 6
	effHealthLeech             = 9
	effHeal                    = 10 // SPELL_EFFECT_HEAL (SharedDefines.h:821)
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
	effSkinning                = 95  // SPELL_EFFECT_SKINNING (SharedDefines.h:906)
	effSkill                   = 118 // SPELL_EFFECT_SKILL (SharedDefines.h:929)
	effKnockBack               = 98
	effPullTowards             = 124 // SPELL_EFFECT_PULL_TOWARDS (SharedDefines.h:935)
	effKillCredit2             = 134 // SPELL_EFFECT_KILL_CREDIT2 (SharedDefines.h:945)
	effWeaponDamage            = 58
	effHealMaxHealth           = 67  // SPELL_EFFECT_HEAL_MAX_HEALTH
	effInterruptCast           = 68  // SPELL_EFFECT_INTERRUPT_CAST (SharedDefines.h:879)
	effDispelMechanic          = 108 // SPELL_EFFECT_DISPEL_MECHANIC (SharedDefines.h:919)
	effForceCast               = 140 // SPELL_EFFECT_FORCE_CAST (SharedDefines.h:951)
	auraPeriodicDmg            = 3
	auraDummy                  = 4
	auraModConfuse             = 5 // SPELL_AURA_MOD_CONFUSE (SpellAuraDefines.h:85)
	auraStun                   = 12
	auraModDamageTaken         = 14 // SPELL_AURA_MOD_DAMAGE_TAKEN (SpellAuraDefines.h:94)
	auraModInvisibility        = 18 // SPELL_AURA_MOD_INVISIBILITY
	auraObsModHealth           = 20 // SPELL_AURA_OBS_MOD_HEALTH (SpellAuraDefines.h:100)
	auraObsModPower            = 21 // SPELL_AURA_OBS_MOD_POWER (SpellAuraDefines.h:101)
	auraPeriodicTrig           = 23
	auraRoot                   = 26
	auraSilence                = 27
	auraModIncreaseSpeed       = 31 // SPELL_AURA_MOD_INCREASE_SPEED (SpellAuraDefines.h)
	auraDecSpeed               = 33
	auraEffImmunity            = 37
	auraTransform              = 56
	auraModScale               = 61
	auraModDmgDone             = 79
	auraModDmgTaken            = 87
	auraModResistPct           = 101
	auraModMeleeHaste          = 138
	auraModHealingPct          = 118 // SPELL_AURA_MOD_HEALING_PCT (SpellAuraDefines.h:198)
	auraMeleeSlow              = 193 // SPELL_AURA_MELEE_SLOW (SpellAuraDefines.h:273)
	auraModTotalStatPct        = 137 // SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE (SpellAuraDefines.h:217)
	auraPeriodicDummy          = 226
	auraControlVehicle         = 236 // SPELL_AURA_CONTROL_VEHICLE (SpellAuraDefines.h:301)
	auraCloneCaster            = 247 // SPELL_AURA_CLONE_CASTER (SpellAuraDefines.h)
	auraPhase                  = 261 // SPELL_AURA_PHASE (SpellAuraDefines.h)
	auraInitializeImages       = 279 // SPELL_AURA_INITIALIZE_IMAGES (SpellAuraDefines.h)
	auraLinked                 = 284
	auraPreventRegen           = 294
	schoolPhysical             = 1
	schoolHoly                 = 2
	schoolFire                 = 4
	schoolNature               = 8
	schoolFrost                = 16
	schoolFrostfire            = 20 // Frost (16) + Fire (4), wago SpellMisc 47610
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
	targetUnitSrcAreaAlly      = 30  // TARGET_UNIT_SRC_AREA_ALLY (SharedDefines.h:1467)
	targetUnitSrcAreaEntry     = 7   // TARGET_UNIT_SRC_AREA_ENTRY (SharedDefines.h:1448)
	targetUnitNearbyEntry      = 38  // TARGET_UNIT_NEARBY_ENTRY (SharedDefines.h:1474)
	targetDestNearbyEntry      = 46  // TARGET_DEST_NEARBY_ENTRY (SharedDefines.h:1482)
	targetGONearbyEntry        = 40
	auraAddPctModifier         = 108 // SPELL_AURA_ADD_PCT_MODIFIER (SpellAuraDefines.h:188)
	spellModDamage             = 0   // SPELLMOD_DAMAGE (SpellMgr.h)
	spellModEffect1            = 3   // SPELLMOD_EFFECT1 (SpellMgr.h)
	familyShaman               = 11  // SPELLFAMILY_SHAMAN (SharedDefines.h)
	familyMage                 = 3   // SPELLFAMILY_MAGE (SpellClassOptions SpellClassSet 3, wago 3.4.0.43659)
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
		{4, 120000, 0, 120000}, // real SpellDuration 4 for 67530 (wago.tools 3.4.0.43659)
		{8, 15000, 0, 15000},   // real SpellDuration 8 for 49263 (wago.tools 3.4.0.43659)
		{27, 3000, 0, 3000},    // real SpellDuration 27 for 66083 (wago.tools 3.4.0.43659)
		{9, 30000, 0, 30000},   // real SpellDuration 9 for 49519 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
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
		{36, f32(0), f32(0), f32(0)},         // real SpellRadius 36 for 50415 (wago.tools 3.4.0.43659, 2026-11-13 22:17 verdict: all zeros)
		{13, f32(10), f32(0), f32(10)},       // real SpellRadius 13 for 65100 (wago.tools 3.4.0.43659)
		{14, f32(8), f32(0), f32(8)},         // real SpellRadius 14 for 59335 (wago.tools 3.4.0.43659)
		{9, f32(20), f32(0), f32(20)},        // real SpellRadius 9 for 44194 (wago.tools 3.4.0.43659)
		{18, f32(15), f32(0), f32(15)},       // real SpellRadius 18 for 44196 (wago.tools 3.4.0.43659)
		{8, f32(5), f32(0), f32(5)},          // real SpellRadius 8 for 31263 (wago.tools 3.4.0.43659)
		{15, f32(3), f32(0), f32(3)},         // real SpellRadius 15 for 31264 (wago.tools 3.4.0.43659)
		{10, f32(30), f32(0), f32(30)},       // real SpellRadius 10 for 31308 (wago.tools 3.4.0.43659)
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
		{170, 8000, 0}, // real SpellCastTimes 170 for 51110 (wago.tools 3.4.0.43659, 2026-11-15 verdict)
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
		{93001, f32(0), f32(0), f32(0), f32(0), 0},
		{93002, f32(0), f32(0), f32(5), f32(5), 0},
		{93003, f32(8), f32(8), f32(25), f32(25), 0},
		{93004, f32(0), f32(0), f32(15), f32(15), 0},
		{93005, f32(0), f32(0), f32(20), f32(20), 0},
		{93006, f32(0), f32(0), f32(60), f32(60), 0},
		{93007, f32(0), f32(0), f32(100), f32(100), 0},
		{93008, f32(0), f32(0), f32(50000), f32(50000), 0},
		{36, f32(0), f32(0), f32(45), f32(45), 0},       // real SpellRange 36 "Longer Range" for 29946/32265 (wago.tools 3.4.0.47659, aux read 54197700)
		{11, f32(0), f32(0), f32(15), f32(15), 0},       // real SpellRange 11 "Fifteen yards" for 29961 (wago.tools 3.4.0.47659, aux read 54197700)
		{13, f32(0), f32(0), f32(50000), f32(50000), 0}, // real SpellRange 13 "Anywhere - Unlimited" for 29967 (wago.tools 3.4.0.47659, aux read 54197700)
		{95, f32(8), f32(8), f32(25), f32(25), 0},       // real SpellRange 95 for 63010 (wago.tools 3.4.0.43659)
		{9, f32(10), f32(10), f32(30), f32(30), 0},      // real SpellRange 9 for 65147 (wago.tools 3.4.0.43659)
		{7, f32(0), f32(0), f32(10), f32(10), 0},        // real SpellRange 7 for 49256 (wago.tools 3.4.0.43659)
		{54, f32(5), f32(5), f32(30), f32(30), 0},       // real SpellRange 54 for 66081 (wago.tools 3.4.0.43659)
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
	summonProps := [][]uint32{{64, 0, 0, 0, 0, 0x100}, {61, 1, 0, 2, 0, 2}, {81, 1, 0, 4, 2, 0x100002}}
	if err := writeWDBC(filepath.Join(out, "SummonProperties.dbc"), 6, summonProps); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d spells + aux tables to %s\n", len(spells), out)
}
