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
	effSchoolDamage     = 2
	effDummy            = 3
	effApplyAura        = 6
	effHealthLeech      = 9
	effSummon           = 28
	effEnergize         = 30
	effTriggerMiss      = 32
	effSendEvent        = 61
	effTriggerSpell     = 64
	effScriptEffect     = 77
	effActivateObj      = 86
	effCharge           = 96
	effKnockBack        = 98
	effWeaponDamage     = 58
	auraPeriodicDmg     = 3
	auraDummy           = 4
	auraStun            = 12
	auraPeriodicTrig    = 23
	auraRoot            = 26
	auraSilence         = 27
	auraDecSpeed        = 33
	auraEffImmunity     = 37
	auraTransform       = 56
	auraModScale        = 61
	auraModDmgDone      = 79
	auraModDmgTaken     = 87
	auraModResistPct    = 101
	auraModMeleeHaste   = 138
	auraPeriodicDummy   = 226
	auraLinked          = 284
	auraPreventRegen    = 294
	schoolPhysical      = 1
	schoolFire          = 4
	schoolNature        = 8
	schoolShadow        = 32
	schoolArcane        = 64
	targetCaster        = 1
	targetEnemy         = 6
	targetSrcAreaEnemy  = 15
	targetDestCaster    = 18 // TARGET_DEST_CASTER (SharedDefines.h:1455)
	targetSrcCaster     = 22 // TARGET_SRC_CASTER (SharedDefines.h:1459)
	targetGONearbyEntry = 40
	auraAddPctModifier  = 108 // SPELL_AURA_ADD_PCT_MODIFIER (SpellAuraDefines.h:188)
	spellModDamage      = 0   // SPELLMOD_DAMAGE (SpellMgr.h)
	spellModEffect1     = 3   // SPELLMOD_EFFECT1 (SpellMgr.h)
	familyShaman        = 11  // SPELLFAMILY_SHAMAN (SharedDefines.h)
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
	{id: 24081, durIdx: 1, rangeIdx: 93001, school: schoolPhysical, effects: [3]effect{
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
		{1, 300000, 0, 300000},
		{6, 600000, 0, 600000},          // real SpellDuration 6 (10 min) for MEMORY_* summons
		{21, 0xFFFFFFFF, 0, 0xFFFFFFFF}, // real SpellDuration 21 (-1 infinite) for 68415
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
	castTimes := [][]uint32{{92001, 2000, 0}, {92002, 3000, 0}}
	if err := writeWDBC(filepath.Join(out, "SpellCastTimes.dbc"), 3, castTimes); err != nil {
		panic(err)
	}
	// SpellRange.dbc: ID, MinHostile, MinFriendly, MaxHostile,
	// MaxFriendly, Flags (floats for the ranges).
	ranges := [][]uint32{
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
