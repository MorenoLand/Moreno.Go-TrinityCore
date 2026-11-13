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
	targetGONearbyEntry = 40
)

type effect struct {
	effect     uint32
	dieSides   int32
	basePoints int32
	mechanic   uint32
	targetA    uint32
	radiusIdx  uint32
	aura       uint32
	auraPeriod uint32
	misc       int32
	miscB      int32
	trigger    uint32
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
		r[92+i] = e.radiusIdx
		r[95+i] = e.aura
		r[98+i] = e.auraPeriod
		r[110+i] = uint32(e.misc)
		r[113+i] = uint32(e.miscB)
		r[116+i] = e.trigger
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
}

// 68415 NOT shipped: aura types + SpellClassMask not derivable from
// Wowhead (needs eff0/eff1 aura types + 3-word class mask).

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
