package world

import (
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// DiminishingGroup mirrors TrinityCore enum DiminishingGroup (SharedDefines.h:3269-3294).
type DiminishingGroup uint16

const (
	DiminishingNone           DiminishingGroup = 0
	DiminishingBanish         DiminishingGroup = 1
	DiminishingCharge         DiminishingGroup = 2
	DiminishingOpeningStun    DiminishingGroup = 3  // Cheap Shot and Pounce
	DiminishingControlledStun DiminishingGroup = 4  // Hammer of Justice, Kidney Shot, Bash, Intercept, etc.
	DiminishingControlledRoot DiminishingGroup = 5  // Frost Nova, Entangling Roots, Freeze
	DiminishingCyclone        DiminishingGroup = 6  // Cyclone
	DiminishingDisarm         DiminishingGroup = 7  // Disarm, Dismantle
	DiminishingDisorient      DiminishingGroup = 8  // Polymorph, Sap, Gouge, Freezing Trap, Wyvern Sting, Repentance, Hungering Cold
	DiminishingEntrapment     DiminishingGroup = 9  // Entrapment
	DiminishingFear           DiminishingGroup = 10 // Fear, Blind, Seduction, Turn Evil, Psychic Scream
	DiminishingHorror         DiminishingGroup = 11 // Death Coil, Psychic Horror
	DiminishingMindControl    DiminishingGroup = 12 // Mind Control
	DiminishingRoot           DiminishingGroup = 13 // Random proc roots (Frostbite, Shattered Barrier)
	DiminishingStun           DiminishingGroup = 14 // Random proc stuns (Impact, Blackout)
	DiminishingScatterShot    DiminishingGroup = 15 // Scatter Shot
	DiminishingSilence        DiminishingGroup = 16 // Silence, Strangulate, Gag Order, Spell Lock
	DiminishingSleep          DiminishingGroup = 17 // Sleep
	DiminishingTaunt          DiminishingGroup = 18 // Taunt, Growl
	DiminishingLimitOnly      DiminishingGroup = 19 // Spells limited to 10s PvP cap only (Crippling Poison, Hamstring, etc.)
	DiminishingDragonsBreath  DiminishingGroup = 20 // Dragon's Breath
	DiminishingMax            DiminishingGroup = 21
)

// DiminishingLevel mirrors TrinityCore enum DiminishingLevels (SpellInfo.h).
type DiminishingLevel uint8

const (
	DiminishingLevel1           DiminishingLevel = 0 // 100% duration
	DiminishingLevel2           DiminishingLevel = 1 // 50% duration
	DiminishingLevel3           DiminishingLevel = 2 // 25% duration
	DiminishingLevelImmune      DiminishingLevel = 3 // 0% duration (Immune)
	DiminishingLevelTauntImmune DiminishingLevel = 4 // taunt-group-only max level (SharedDefines.h:3779)
)

const (
	diminishingDurationLimit uint32        = 10000            // 10 second PvP CC limit (Unit.cpp:9048-9062)
	diminishingResetDuration time.Duration = 15 * time.Second // 15 second DR reset window (Unit.cpp:9019)
)

// diminishingReturn mirrors TrinityCore struct DiminishingReturn (Unit.h:1810-1820).
type diminishingReturn struct {
	hitCount uint8
	stack    uint16
	hitTime  time.Time
}

// getDiminishingReturnsGroup resolves the diminishing returns group for a spell.
// Mirrors TrinityCore SpellInfo::diminishingGroupCompute (SpellInfo.cpp:2242-2433),
// in implemented order: IsPositive, any SPELL_AURA_MOD_TAUNT effect, the
// spell-ID table, the family switch, then the all-effects mechanic mask.
// (C++ orders family before mechanic and has no ID table; the table runs
// first here so listed ranks keep their current group where a family arm
// would resolve differently.) The triggered flag selects the aura-proc
// variant (SpellInfo.cpp:2424/2428): mechanic-STUN resolves to
// DIMINISHING_STUN (random proc stuns) and mechanic-ROOT to DIMINISHING_ROOT
// (random proc roots) when the cast was triggered by an aura
// (Spell::m_triggeredByAuraSpell != null, Spell.cpp:2798).
func getDiminishingReturnsGroup(spell wotlk.Spell, triggered bool) DiminishingGroup {
	// SpellInfo::IsPositive (SpellInfo.cpp:2244): positive spells never
	// diminish. The apply path gates per effect; the bounce path relies
	// on this arm (spellIsPositive mirrors the CU_NEGATIVE mask).
	if spellIsPositive(spell) {
		return DiminishingNone
	}
	// Any SPELL_AURA_MOD_TAUNT effect puts the spell in the taunt group
	// (SpellInfo.cpp:2248-2252), ahead of the family switch: Mocking Blow,
	// Challenging Shout and Challenging Roar carry the effect but no
	// taunt spell ID, so the ID table below cannot reach them.
	for _, eff := range spell.Effects {
		if eff.Effect != 0 && eff.Aura == spellAuraModTaunt {
			return DiminishingTaunt
		}
	}
	// Specific spell overrides matching TrinityCore
	switch spell.ID {
	// Cyclone
	case 33786:
		return DiminishingCyclone

	// Cheap Shot & Pounce (Opening stuns)
	case 1833, 9005, 9823, 9827, 27006, 49803:
		return DiminishingOpeningStun

	// Kidney Shot, Hammer of Justice, Bash, Deep Freeze, Intercept, Shadowfury, Concussion Blow, Shockwave, Gnaw, Intimidation, War Stomp
	case 408, 8643,
		853, 5588, 5589, 10308,
		5211, 6798, 8983,
		44572,
		20252, 20616, 20617, 25272, 25275,
		30283, 30413, 30414,
		12809,
		46968,
		47481,
		19577, 24394,
		20549,
		2812, 10318, 27139, 48816, 48817:
		return DiminishingControlledStun

	// Charge Stun
	case 100, 6178, 11578, 7922:
		return DiminishingCharge

	// Scatter Shot
	case 19503:
		return DiminishingScatterShot

	// Entrapment
	case 19184, 19387, 19388, 64803:
		return DiminishingEntrapment

	// Dragon's Breath
	case 31661, 33041, 33042, 33043, 42949, 42950:
		return DiminishingDragonsBreath

	// Death Coil & Psychic Horror
	case 6789, 17925, 17926, 27223, 47859, 47860, 64044:
		return DiminishingHorror

	// Fear, Howl of Terror, Seduction, Blind, Psychic Scream, Turn Evil, Intimidating Shout
	case 5782, 6213, 6215,
		5484, 17928,
		6358,
		2094,
		8122, 8124, 10888, 10890,
		10326,
		5246:
		return DiminishingFear

	// Disorient: Polymorph, Sap, Gouge, Freezing Trap, Wyvern Sting, Repentance, Hungering Cold
	case 118, 12824, 12825, 12826, 28271, 28272, 61305, 61721, 61780,
		6770, 2070, 11297,
		1776,
		1499, 14310, 14311, 43415,
		19386, 24132, 24133, 27068, 49011, 49012,
		20066,
		49203:
		return DiminishingDisorient

	// Controlled Root: Frost Nova, Freeze (Water Elemental), Entangling Roots, Nature's Grasp
	case 122, 865, 6131, 10230, 27088, 42917,
		33395,
		339, 1062, 5195, 5196, 9852, 9853, 26989, 53308,
		16689:
		return DiminishingControlledRoot

	// Random Roots
	case 11071, 12494, 55080:
		return DiminishingRoot

	// Silence: Silence, Strangulate, Spell Lock, Garrote - Silence, Arcane Torrent
	case 15487, 47476, 19244, 19647, 1330,
		28730, 25046, 50613:
		return DiminishingSilence

	// Disarm: Disarm, Dismantle, Chimera Shot - Scorpid, Psychic Horror - Disarm
	case 676, 51722, 53359, 64058:
		return DiminishingDisarm

	// Mind Control
	case 605, 10911, 10912:
		return DiminishingMindControl

	// Banish
	case 710, 18647:
		return DiminishingBanish

	// Taunt
	case 355, 6795, 62124, 56222:
		return DiminishingTaunt

	// Limit Only (PvP 10s cap without DR decay)
	case 1715, 3408, 2974, 12323, 1130, 20164:
		return DiminishingLimitOnly
	}

	// Family switch (SpellInfo.cpp:2255-2404), in C++ arm order within
	// each family. These arms run after the ID table so listed ranks
	// keep their current group; they matter for unlisted ranks and for
	// the LIMITONLY debuffs (Faerie Fire, Hunter's Mark, Curse of
	// Elements, Mark of Blood), which carry no CC mechanic and previously
	// resolved to NONE, losing their PvP duration cap entirely.
	switch spell.SpellFamilyName {
	case spellFamilyGeneric:
		// Pet charge effects (Infernal Awakening, Demon Charge)
		if spell.SpellVisual[0] == 2816 && spell.SpellIconID == 15 {
			return DiminishingControlledStun
		}
		// Frost Tomb, Black Plague, Screams of the Dead, Crystallize,
		// ToC Icehowl Arctic Breath: explicitly no diminishing.
		switch spell.ID {
		case 48400, 64155, 51750, 48179:
			return DiminishingNone
		}
		if spell.SpellVisual[0] == 14153 {
			return DiminishingNone
		}
	case 1: // SPELLFAMILY_UNK1: event spells never diminish.
		return DiminishingNone
	case spellFamilyMage:
		// Frostbite
		if spell.SpellFamilyFlags[1]&0x80000000 != 0 {
			return DiminishingRoot
		}
		// Shattered Barrier
		if spell.SpellVisual[0] == 12297 {
			return DiminishingRoot
		}
		// Deep Freeze
		if spell.SpellIconID == 2939 && spell.SpellVisual[0] == 9963 {
			return DiminishingControlledStun
		}
		// Frost Nova / Freeze (Water Elemental)
		if spell.SpellIconID == 193 {
			return DiminishingControlledRoot
		}
		// Dragon's Breath
		if spell.SpellFamilyFlags[0]&0x800000 != 0 {
			return DiminishingDragonsBreath
		}
	case spellFamilyWarrior:
		// Hamstring - limit duration to 10s in PvP
		if spell.SpellFamilyFlags[0]&0x2 != 0 {
			return DiminishingLimitOnly
		}
		// Charge Stun (own diminishing)
		if spell.SpellFamilyFlags[0]&0x01000000 != 0 {
			return DiminishingCharge
		}
	case spellFamilyWarlock:
		// Curses/etc
		if spell.SpellFamilyFlags[0]&0x80000000 != 0 || spell.SpellFamilyFlags[1]&0x200 != 0 {
			return DiminishingLimitOnly
		}
		// Seduction
		if spell.SpellFamilyFlags[1]&0x10000000 != 0 {
			return DiminishingFear
		}
	case spellFamilyDruid:
		// Pounce
		if spell.SpellFamilyFlags[0]&0x20000 != 0 {
			return DiminishingOpeningStun
		}
		// Cyclone
		if spell.SpellFamilyFlags[1]&0x20 != 0 {
			return DiminishingCyclone
		}
		// Entangling Roots / Nature's Grasp
		if spell.SpellFamilyFlags[0]&0x00000200 != 0 {
			return DiminishingControlledRoot
		}
		// Faerie Fire
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return DiminishingLimitOnly
		}
	case spellFamilyRogue:
		// Gouge
		if spell.SpellFamilyFlags[0]&0x8 != 0 {
			return DiminishingDisorient
		}
		// Blind
		if spell.SpellFamilyFlags[0]&0x1000000 != 0 {
			return DiminishingFear
		}
		// Cheap Shot
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return DiminishingOpeningStun
		}
		// Crippling poison - Limit to 10 seconds in PvP (No SpellFamilyFlags)
		if spell.SpellIconID == 163 {
			return DiminishingLimitOnly
		}
	case spellFamilyHunter:
		// Hunter's Mark
		if spell.SpellFamilyFlags[0]&0x400 != 0 && spell.SpellIconID == 538 {
			return DiminishingLimitOnly
		}
		// Scatter Shot (own diminishing)
		if spell.SpellFamilyFlags[0]&0x40000 != 0 && spell.SpellIconID == 132 {
			return DiminishingScatterShot
		}
		// Entrapment (own diminishing)
		if spell.SpellVisual[0] == 7484 && spell.SpellIconID == 20 {
			return DiminishingEntrapment
		}
		// Wyvern Sting mechanic is MECHANIC_SLEEP but the diminishing is DIMINISHING_DISORIENT
		if spell.SpellFamilyFlags[1]&0x1000 != 0 && spell.SpellIconID == 1721 {
			return DiminishingDisorient
		}
		// Freezing Arrow
		if spell.SpellFamilyFlags[0]&0x8 != 0 {
			return DiminishingDisorient
		}
	case spellFamilyPaladin:
		// Judgement of Justice - limit duration to 10s in PvP
		if spell.SpellFamilyFlags[0]&0x100000 != 0 {
			return DiminishingLimitOnly
		}
		// Turn Evil
		if spell.SpellFamilyFlags[1]&0x804000 != 0 && spell.SpellIconID == 309 {
			return DiminishingFear
		}
	case spellFamilyShaman:
		// Storm, Earth and Fire - Earthgrab
		if spell.SpellFamilyFlags[2]&0x4000 != 0 {
			return DiminishingNone
		}
	case spellFamilyDeathKnight:
		// Hungering Cold (no flags)
		if spell.SpellIconID == 2797 {
			return DiminishingDisorient
		}
		// Mark of Blood
		if spell.SpellFamilyFlags[0]&0x10000000 != 0 && spell.SpellIconID == 2285 {
			return DiminishingLimitOnly
		}
	}

	// Mechanic fallback on the all-effects mask (SpellInfo.cpp:2406-2433,
	// GetAllEffectsMechanicMask): the spell-level mechanic plus every
	// effect mechanic, in C++ priority order. The old single-mechanic
	// switch missed effect-only mechanics and resolved multi-mechanic
	// spells by the spell-level value alone.
	mechanicMask := uint32(0)
	if spell.Mechanic != 0 && spell.Mechanic < 32 {
		mechanicMask |= uint32(1) << spell.Mechanic
	}
	for _, eff := range spell.Effects {
		if eff.Effect != 0 && eff.Mechanic != 0 && eff.Mechanic < 32 {
			mechanicMask |= uint32(1) << eff.Mechanic
		}
	}
	switch {
	case mechanicMask&(1<<1) != 0: // MECHANIC_CHARM
		return DiminishingMindControl
	case mechanicMask&(1<<9) != 0: // MECHANIC_SILENCE
		return DiminishingSilence
	case mechanicMask&(1<<10) != 0: // MECHANIC_SLEEP
		return DiminishingSleep
	case mechanicMask&(1<<30|1<<17|1<<20) != 0: // MECHANIC_SAPPED, POLYMORPH, SHACKLE
		return DiminishingDisorient
	case mechanicMask&(1<<14) != 0 && spell.SpellIconID != 292: // MECHANIC_KNOCKOUT, except Blast Wave
		return DiminishingDisorient
	case mechanicMask&(1<<3) != 0: // MECHANIC_DISARM
		return DiminishingDisarm
	case mechanicMask&(1<<5) != 0: // MECHANIC_FEAR
		return DiminishingFear
	case mechanicMask&(1<<12) != 0: // MECHANIC_STUN
		// SpellInfo.cpp:2424 - random proc stuns (Impact, Blackout) track
		// DIMINISHING_STUN when triggered by an aura.
		if triggered {
			return DiminishingStun
		}
		return DiminishingControlledStun
	case mechanicMask&(1<<18) != 0: // MECHANIC_BANISH
		return DiminishingBanish
	case mechanicMask&(1<<7) != 0: // MECHANIC_ROOT
		// SpellInfo.cpp:2428 - random proc roots (Frostbite, Shattered
		// Barrier) track DIMINISHING_ROOT when triggered by an aura.
		if triggered {
			return DiminishingRoot
		}
		return DiminishingControlledRoot
	case mechanicMask&(1<<24) != 0: // MECHANIC_HORROR
		return DiminishingHorror
	// No C++ mask arm exists for these mechanics; the nets below preserve
	// the previous single-mechanic behavior for unlisted ranks (a named
	// DBC case would be needed before dropping them: C++ returns NONE
	// here, so a long SNARE would lose its 10s PvP cap).
	case mechanicMask&(1<<2) != 0: // MECHANIC_DISORIENTED
		return DiminishingDisorient
	case mechanicMask&(1<<13) != 0: // MECHANIC_FREEZE
		if triggered {
			return DiminishingRoot
		}
		return DiminishingControlledRoot
	case mechanicMask&(1<<11) != 0: // MECHANIC_SNARE
		return DiminishingLimitOnly
	default:
		return DiminishingNone
	}
}

// isGroupDurationLimited mirrors the isGroupDurationLimited lambda inside
// SpellInfo::diminishingLimitDurationCompute (SpellInfo.cpp:2465-2490): only
// these groups cap the aura duration in PvP. Scatter Shot, Silence and
// Dragon's Breath resolve DR groups but carry no duration cap in C++.
func isGroupDurationLimited(group DiminishingGroup) bool {
	switch group {
	case DiminishingBanish,
		DiminishingControlledStun,
		DiminishingControlledRoot,
		DiminishingCyclone,
		DiminishingDisorient,
		DiminishingEntrapment,
		DiminishingFear,
		DiminishingHorror,
		DiminishingMindControl,
		DiminishingOpeningStun,
		DiminishingRoot,
		DiminishingSleep,
		DiminishingStun,
		DiminishingLimitOnly:
		return true
	default:
		return false
	}
}

// diminishingDurationLimitMs mirrors the diminishingLimitDurationCompute lambda
// in SpellInfo::_LoadDiminishingReturns (SpellInfo.cpp:2462-2528): 0 when the
// group carries no duration cap; the family-specific explicit PvP cap when one
// matches (Wyvern Sting 6s, Repentance 6s, Faerie Fire 40s, ...); 10s otherwise.
func diminishingDurationLimitMs(spell wotlk.Spell, group DiminishingGroup) uint32 {
	if !isGroupDurationLimited(group) {
		return 0
	}
	switch spell.SpellFamilyName {
	case spellFamilyDruid:
		// Faerie Fire - limit to 40 seconds in PvP (3.1)
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return 40000
		}
	case spellFamilyHunter:
		// Wyvern Sting
		if spell.SpellFamilyFlags[1]&0x1000 != 0 {
			return 6000
		}
		// Hunter's Mark
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return 120000
		}
	case spellFamilyPaladin:
		// Repentance - limit to 6 seconds in PvP
		if spell.SpellFamilyFlags[0]&0x4 != 0 {
			return 6000
		}
	case spellFamilyWarlock:
		// Banish - limit to 6 seconds in PvP
		if spell.SpellFamilyFlags[1]&0x8000000 != 0 {
			return 6000
		}
		// Curse of Tongues - limit to 12 seconds in PvP
		if spell.SpellFamilyFlags[2]&0x800 != 0 {
			return 12000
		}
		// Curse of Elements - limit to 120 seconds in PvP
		if spell.SpellFamilyFlags[1]&0x200 != 0 {
			return 120000
		}
	}
	return diminishingDurationLimit
}

// diminishingReturnsType mirrors TrinityCore enum DiminishingReturnsType,
// the DiminishReturnType computed per group in
// SpellInfo::_LoadDiminishingReturns (SpellInfo.cpp:2435-2449).
type diminishingReturnsType uint8

const (
	diminishingTypeNone diminishingReturnsType = iota
	diminishingTypeAll
	diminishingTypePlayer
)

// diminishingGroupType mirrors the diminishingTypeCompute lambda in
// SpellInfo::_LoadDiminishingReturns (SpellInfo.cpp:2435-2449).
func diminishingGroupType(group DiminishingGroup) diminishingReturnsType {
	switch group {
	case DiminishingTaunt,
		DiminishingControlledStun,
		DiminishingStun,
		DiminishingOpeningStun,
		DiminishingCyclone,
		DiminishingCharge:
		return diminishingTypeAll
	case DiminishingLimitOnly,
		DiminishingNone:
		return diminishingTypeNone
	default:
		return diminishingTypePlayer
	}
}

// getDiminishing returns the current diminishing level for a group.
// Mirrors TrinityCore Unit::GetDiminishing (Unit.cpp:9012-9023).
func (s *session) getDiminishing(group DiminishingGroup) DiminishingLevel {
	if s == nil || group >= DiminishingMax {
		return DiminishingLevel1
	}
	dim := &s.diminishing[group]
	if dim.hitCount == 0 {
		return DiminishingLevel1
	}
	// If last aura in this group expired more than 15 seconds ago, reset to level 1
	if dim.stack == 0 && !dim.hitTime.IsZero() && time.Since(dim.hitTime) > diminishingResetDuration {
		dim.hitCount = 0
		return DiminishingLevel1
	}
	return DiminishingLevel(dim.hitCount)
}

// incrDiminishing advances the diminishing return hit counter.
// Mirrors TrinityCore Unit::IncrDiminishing (Unit.cpp:9025-9034): the cap is
// the spell's DiminishMaxLevel — DIMINISHING_LEVEL_TAUNT_IMMUNE for the taunt
// group, DIMINISHING_LEVEL_IMMUNE otherwise (SpellInfo.cpp:2454-2463).
func (s *session) incrDiminishing(group DiminishingGroup) {
	if s == nil || group >= DiminishingMax {
		return
	}
	currentLevel := s.getDiminishing(group)
	maxLevel := DiminishingLevelImmune
	if group == DiminishingTaunt {
		maxLevel = DiminishingLevelTauntImmune
	}
	dim := &s.diminishing[group]
	if currentLevel < maxLevel {
		dim.hitCount = uint8(currentLevel) + 1
	}
}

// applyDiminishingAura tracks active aura counts and timestamps the 15-second reset countdown.
// Mirrors TrinityCore Unit::ApplyDiminishingAura (Unit.cpp:9101-9116).
func (s *session) applyDiminishingAura(group DiminishingGroup, apply bool) {
	if s == nil || group >= DiminishingMax {
		return
	}
	dim := &s.diminishing[group]
	if apply {
		dim.stack++
	} else if dim.stack > 0 {
		dim.stack--
		if dim.stack == 0 {
			dim.hitTime = time.Now()
		}
	}
}

// clearDiminishings resets all diminishing return states (e.g. on death).
// Mirrors TrinityCore Unit::ClearDiminishings (Unit.cpp:9118-9122).
func (s *session) clearDiminishings() {
	if s == nil {
		return
	}
	for i := range s.diminishing {
		s.diminishing[i] = diminishingReturn{}
	}
}

// applyDiminishingToDuration calculates the final CC duration and determines immunity.
// Mirrors TrinityCore Unit::ApplyDiminishingToDuration (Unit.cpp:9036-9099).
// The triggered flag selects the aura-proc DR group variant
// (Spell::PreprocessSpellHit, Spell.cpp:2798 — m_triggeredByAuraSpell != null).
func (s *session) applyDiminishingToDuration(spell wotlk.Spell, durationMs uint32, isPvP, triggered bool) (DiminishingGroup, uint32, bool) {
	group := getDiminishingReturnsGroup(spell, triggered)
	if group == DiminishingNone || durationMs == 0 {
		return DiminishingNone, durationMs, true
	}

	// Duration of crowd control abilities on PvP targets is limited
	// (Unit.cpp:9048-9062, the 2.2.0 rule): the cap is the spell's
	// DiminishDurationLimit — the family-specific explicit cap when one
	// matches, 10s otherwise, none when the group is not duration-limited
	// (SpellInfo.cpp:2462-2528).
	if isPvP {
		if limit := diminishingDurationLimitMs(spell, group); limit > 0 && durationMs > limit {
			durationMs = limit
		}
	}

	if group == DiminishingLimitOnly {
		return group, durationMs, true
	}

	// Taunt spells take no duration mod on player targets: the
	// DIMINISHING_TAUNT branch of Unit::ApplyDiminishingToDuration
	// (Unit.cpp:9064-9082) only runs its 0.65/0.4225/0.274625 sequence for
	// creatures carrying CREATURE_FLAG_EXTRA_TAUNT_DIMINISH (0x00080000);
	// for everyone else the mod stays 1.0. Go runs the DR duration path
	// for player targets only, so taunts land at full duration every time.
	// Without this arm the generic 1/0.5/0.25/0 ladder made the 4th taunt
	// on a player come back SPELL_FAILED_IMMUNE.
	if group == DiminishingTaunt {
		return group, durationMs, true
	}

	level := s.getDiminishing(group)
	mod := float32(1.0)
	switch level {
	case DiminishingLevel1:
		mod = 1.0
	case DiminishingLevel2:
		mod = 0.5
	case DiminishingLevel3:
		mod = 0.25
	case DiminishingLevelImmune:
		mod = 0.0
	}

	newDuration := uint32(float32(durationMs) * mod)
	if newDuration == 0 {
		return group, 0, false // Immune!
	}
	return group, newDuration, true
}
