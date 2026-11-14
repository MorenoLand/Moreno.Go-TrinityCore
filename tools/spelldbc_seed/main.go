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
	effSchoolDamage           = 2
	effDummy                  = 3
	effApplyAura              = 6
	effHealthLeech            = 9
	effSummon                 = 28
	effPersistentAreaAura     = 27 // SPELL_EFFECT_PERSISTENT_AREA_AURA (SharedDefines.h)
	effEnergize               = 30
	effTriggerMiss            = 32
	effSendEvent              = 61
	effTriggerSpell           = 64
	effScriptEffect           = 77
	effActivateObj            = 86
	effCharge                 = 96
	effKnockBack              = 98
	effKillCredit2            = 134 // SPELL_EFFECT_KILL_CREDIT2 (SharedDefines.h:945)
	effWeaponDamage           = 58
	effHealMaxHealth          = 67 // SPELL_EFFECT_HEAL_MAX_HEALTH
	auraPeriodicDmg           = 3
	auraDummy                 = 4
	auraStun                  = 12
	auraModInvisibility       = 18 // SPELL_AURA_MOD_INVISIBILITY
	auraPeriodicTrig          = 23
	auraRoot                  = 26
	auraSilence               = 27
	auraModIncreaseSpeed      = 31 // SPELL_AURA_MOD_INCREASE_SPEED (SpellAuraDefines.h)
	auraDecSpeed              = 33
	auraEffImmunity           = 37
	auraTransform             = 56
	auraModScale              = 61
	auraModDmgDone            = 79
	auraModDmgTaken           = 87
	auraModResistPct          = 101
	auraModMeleeHaste         = 138
	auraPeriodicDummy         = 226
	auraControlVehicle        = 236 // SPELL_AURA_CONTROL_VEHICLE (SpellAuraDefines.h:301)
	auraLinked                = 284
	auraPreventRegen          = 294
	schoolPhysical            = 1
	schoolFire                = 4
	schoolNature              = 8
	schoolFrost               = 16
	schoolShadow              = 32
	schoolArcane              = 64
	targetCaster              = 1
	targetEnemy               = 6
	targetSrcAreaEnemy        = 15
	targetDestCaster          = 18 // TARGET_DEST_CASTER (SharedDefines.h:1455)
	targetUnitDestAreaEnemy   = 16 // TARGET_UNIT_DEST_AREA_ENEMY
	targetDestCasterBackRight = 42 // TARGET_DEST_CASTER_BACK_RIGHT (SharedDefines.h:1478)
	targetDestCasterSummon    = 32 // TARGET_DEST_CASTER_SUMMON
	targetDestCasterRadius    = 73 // TARGET_DEST_CASTER_RADIUS
	targetUnitTargetAny       = 25 // TARGET_UNIT_TARGET_ANY (SharedDefines.h:1461)
	targetDestTargetAny       = 63 // TARGET_DEST_TARGET_ANY (SharedDefines.h:1499)
	targetDestTargetEnemy     = 53 // TARGET_DEST_TARGET_ENEMY
	targetSrcCaster           = 22 // TARGET_SRC_CASTER (SharedDefines.h:1459)
	targetUnitSrcAreaEntry    = 7  // TARGET_UNIT_SRC_AREA_ENTRY (SharedDefines.h:1448)
	targetUnitNearbyEntry     = 38 // TARGET_UNIT_NEARBY_ENTRY (SharedDefines.h:1474)
	targetGONearbyEntry       = 40
	auraAddPctModifier        = 108 // SPELL_AURA_ADD_PCT_MODIFIER (SpellAuraDefines.h:188)
	spellModDamage            = 0   // SPELLMOD_DAMAGE (SpellMgr.h)
	spellModEffect1           = 3   // SPELLMOD_EFFECT1 (SpellMgr.h)
	familyShaman              = 11  // SPELLFAMILY_SHAMAN (SharedDefines.h)
	familyMage                = 3   // SPELLFAMILY_MAGE (SpellClassOptions SpellClassSet 3, wago 3.4.0.43659)
)

type effect struct {
	effect     uint32
	dieSides   int32
	basePoints int32
	mechanic   uint32
	targetA    uint32
	targetB    uint32
	radiusIdx  uint32
	aura       uint32
	auraPeriod uint32
	misc       int32
	miscB      int32
	trigger    uint32
	mask       [3]uint32 // EffectSpellClassMask (store.go fields 122-130)
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
	family      uint32 // SpellFamilyName (store.go field 208)
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
	r[212] = s.maxTargets
	r[225] = s.school
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
	// 63081/63145 MiscValueB 64 INFERRED (same row-64 inference as
	// SummonProperties below); MiscValue NPC entries Wowhead-verified.
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
	// (SummonProperties 64, seeded INFERRED row), TargetA 42
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
	// (SummonProperties 64, seeded INFERRED row — on the Go
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
	// 31308 (NOT seeded — no wago row yet; the periodic-trigger leg
	// will not resolve it until then), TargetA 1; DurationIndex 21
	// (infinite, seeded row), RangeIndex 1 (Self Only, 0-0 yd =
	// synthetic 93001's value), SchoolMask 0x40 (arcane),
	// CastingTimeIndex 1 (instant, default 0). No SpellClassOptions
	// row (family 0). Attributes not encoded (seed has no attributes
	// field).
	{id: 31260, durIdx: 21, rangeIdx: 93001, school: schoolArcane, effects: [3]effect{
		{effect: effApplyAura, dieSides: 1, aura: auraDummy, targetA: targetCaster},
		{effect: effApplyAura, aura: auraPeriodicTrig, auraPeriod: 4000, trigger: 31308, targetA: targetCaster},
	}},
	// 31264 Summon Water Elementals (hyjalAI.h:35
	// SPELL_SUMMON_ELEMENTALS; Jaina's summon) — wago.tools 3.4.0.43659
	// Spell/SpellEffect/SpellMisc rows, 2026-11-14 verdict: single
	// effect 28 SUMMON, EffectAmplitude -3 (not encoded — the seed has
	// no amplitude field; C++ EffectSummonType does not use amplitude
	// for the count), BasePoints 2, DieSides 1 (CalcValue +1 = 3
	// elementals, matching "$s1 powerful water elementals"),
	// MiscValue 18001 (water elemental entry), MiscValueB 61
	// (SummonProperties 61 — row NOT seeded, no wago values yet; the
	// GO-summon leg skips when the row is missing, so the summon stays
	// inert until row 61 lands), TargetA 32
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
	// DurationIndex; 24081 now rides synthetic 90006 until its real
	// DurationIndex lands from wago), RangeIndex 4 (Medium Range,
	// 0-30 yd, seeded real row), SchoolMask 0x10 (frost),
	// CastingTimeIndex 4 (1000 ms, seeded real row below). No
	// SpellClassOptions row (family 0).
	{id: 31266, durIdx: 1, rangeIdx: 4, castTimeIdx: 4, school: schoolFrost, effects: [3]effect{
		{effect: effPersistentAreaAura, aura: auraPeriodicDmg, dieSides: 101, basePoints: 949, auraPeriod: 2000, targetA: targetUnitDestAreaEnemy, radiusIdx: 13},
		{effect: effPersistentAreaAura, aura: auraModIncreaseSpeed, dieSides: 1, basePoints: -71, radiusIdx: 13},
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
		{21, 0xFFFFFFFF, 0, 0xFFFFFFFF}, // real SpellDuration 21 (-1 infinite) for 68415
		{32, 6000, 0, 6000},             // real SpellDuration 32 for 54690 (wago.tools 3.4.0.43659)
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
		{91001, f32(8), f32(0), f32(8)},
		{91002, f32(10), f32(0), f32(10)},
		{91003, f32(15), f32(0), f32(15)},
		{91004, f32(30), f32(0), f32(30)},
		{91005, f32(45), f32(0), f32(45)},
		{91006, f32(200), f32(0), f32(200)},
		{91007, f32(50000), f32(0), f32(50000)},
		{91008, f32(100), f32(0), f32(100)},
		{91009, f32(1), f32(0), f32(1)},
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
		{93001, f32(0), f32(0), f32(0), f32(0), 0},
		{93002, f32(0), f32(0), f32(5), f32(5), 0},
		{93003, f32(8), f32(8), f32(25), f32(25), 0},
		{93004, f32(0), f32(0), f32(15), f32(15), 0},
		{93005, f32(0), f32(0), f32(20), f32(20), 0},
		{93006, f32(0), f32(0), f32(60), f32(60), 0},
		{93007, f32(0), f32(0), f32(100), f32(100), 0},
		{93008, f32(0), f32(0), f32(50000), f32(50000), 0},
	}
	if err := writeWDBC(filepath.Join(out, "SpellRange.dbc"), 6, ranges); err != nil {
		panic(err)
	}
	// SummonProperties.dbc: ID, Control, Faction, Title, Slot, Flags.
	// Row 64 is INFERRED, not client-DBC-verified: (Control 0/WILD,
	// Faction 14, Title 2/GUARDIAN, Slot 0, Flags 0), forced by the
	// C++ EffectSummonType multi-summon list + the Go GO-summon leg
	// gates (go_origin_summon.go:89-98) + hostile-guardian fight
	// behavior (boss_marli.cpp:119). Replace with the real row when
	// client data ships.
	summonProps := [][]uint32{{64, 0, 14, 2, 0, 0}}
	if err := writeWDBC(filepath.Join(out, "SummonProperties.dbc"), 6, summonProps); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d spells + aux tables to %s\n", len(spells), out)
}
