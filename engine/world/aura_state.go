package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// AuraStateType values (SharedDefines.h:1320-1354).
const (
	auraStateNone               = 0
	auraStateDefense            = 1
	auraStateHealthless20Pct    = 2
	auraStateBerserking         = 3
	auraStateFrozen             = 4
	auraStateJudgement          = 5
	auraStateHunterParry        = 7
	auraStateWarriorVictoryRush = 10
	auraStateFaerieFire         = 12
	auraStateHealthless35Pct    = 13
	auraStateConflagrate        = 14
	auraStateSwiftmend          = 15
	auraStateDeadlyPoison       = 16
	auraStateEnrage             = 17
	auraStateBleeding           = 18
	auraStateUnknown19          = 19
	auraStateUnknown22          = 22
	auraStateHealthAbove75Pct   = 23
)

// perCasterAuraStateMask mirrors PER_CASTER_AURA_STATE_MASK (SharedDefines.h:
// 1356-1357). These states are tracked per aura caster, not as a unit-wide
// flag (Unit::HasAuraState, Unit.cpp:5946).
const perCasterAuraStateMask = uint32(1<<(auraStateConflagrate-1)) | uint32(1<<(auraStateDeadlyPoison-1))

// Spell family names used by _LoadAuraState (SharedDefines.h:3585-3589).
const (
	spellFamilyWarrior = 4
	spellFamilyWarlock = 5
	spellFamilyDruid   = 7
	spellFamilyRogue   = 8
)

// spellAuraState mirrors SpellInfo::_LoadAuraState (SpellInfo.cpp:1966-2032):
// the aura state a unit gains while an aura of this spell is applied on it.
// Gap (not a stub): Sting loses AURA_STATE_FAERIE_FIRE (Go has no spell
// category field for the category-1133 check).
func spellAuraState(spell wotlk.Spell) uint32 {
	// Seals (SpellInfo.cpp:1971; classifier takes nil — the seal branch
	// never resolves the first-rank chain).
	if spellSpecific(spell, nil) == spellSpecificSeal {
		return auraStateJudgement
	}
	// Conflagrate aura state on Immolate and Shadowflame
	if spell.SpellFamilyName == spellFamilyWarlock &&
		(spell.SpellFamilyFlags[0]&4 != 0 || spell.SpellFamilyFlags[2]&2 != 0) {
		return auraStateConflagrate
	}
	// Faerie Fire (druid versions)
	if spell.SpellFamilyName == spellFamilyDruid && spell.SpellFamilyFlags[0]&0x400 != 0 {
		return auraStateFaerieFire
	}
	// Victorious
	if spell.SpellFamilyName == spellFamilyWarrior && spell.SpellFamilyFlags[1]&0x00040000 != 0 {
		return auraStateWarriorVictoryRush
	}
	// Swiftmend state on Regrowth & Rejuvenation
	if spell.SpellFamilyName == spellFamilyDruid && spell.SpellFamilyFlags[0]&0x50 != 0 {
		return auraStateSwiftmend
	}
	// Deadly poison aura state
	if spell.SpellFamilyName == spellFamilyRogue && spell.SpellFamilyFlags[0]&0x10000 != 0 {
		return auraStateDeadlyPoison
	}
	// Enrage aura state
	if spell.DispelType == DispelEnrage {
		return auraStateEnrage
	}
	// Bleeding aura state
	for _, effect := range spell.Effects {
		if effect.Mechanic == 15 { // MECHANIC_BLEED
			return auraStateBleeding
		}
	}
	// Frozen: frost school aura that stuns or roots
	if spell.SchoolMask&16 != 0 { // SPELL_SCHOOL_MASK_FROST
		for _, effect := range spell.Effects {
			if effect.Effect == 6 && (effect.Aura == spellAuraStun || effect.Aura == spellAuraRoot) { // SPELL_EFFECT_APPLY_AURA
				return auraStateFrozen
			}
		}
	}
	switch spell.ID {
	case 71465, 50241: // Divine Surge, Evasive Charges
		return auraStateUnknown22
	case 9991, 35325, 35328, 35329, 35331, 49163: // Touch of Zanzil, blood auras, Perpetual Instability
		return auraStateFaerieFire
	}
	return auraStateNone
}

// auraStateHealthMask mirrors the health-threshold ModifyAuraState updates
// (Unit.cpp:467-469): AURA_STATE_HEALTHLESS_20_PERCENT, _35_PERCENT and
// HEALTH_ABOVE_75_PERCENT. Comparisons use the floored pct of max health,
// like Unit::HealthBelowPct/HealthAbovePct (Unit.h:872-874).
func auraStateHealthMask(health, maxHealth uint32) uint32 {
	if maxHealth == 0 {
		return 0
	}
	h, m := uint64(health), uint64(maxHealth)
	var mask uint32
	if h < m*20/100 {
		mask |= uint32(1) << (auraStateHealthless20Pct - 1)
	}
	if h < m*35/100 {
		mask |= uint32(1) << (auraStateHealthless35Pct - 1)
	}
	if h > m*75/100 {
		mask |= uint32(1) << (auraStateHealthAbove75Pct - 1)
	}
	return mask
}

// lookupAuraState resolves a spell's aura state through the DBC store,
// returning auraStateNone when the spell is unknown.
func (s *session) lookupAuraState(spellID uint32) uint32 {
	if s.server == nil || s.server.Data == nil {
		return auraStateNone
	}
	if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
		return spellAuraState(spell)
	}
	return auraStateNone
}

// unitAuraStateMask builds the unit-wide aura state mask (health thresholds
// plus one bit per active aura's state), mirroring UNIT_FIELD_AURASTATE as
// maintained by Unit::ModifyAuraState (Unit.cpp:5880).
func (s *session) unitAuraStateMask() uint32 {
	mask := auraStateHealthMask(s.player.Health, s.player.MaxHealth)
	for spellID := range s.auras {
		if state := s.lookupAuraState(spellID); state != auraStateNone {
			mask |= uint32(1) << (state - 1)
		}
	}
	return mask
}

// hasAuraState mirrors Unit::HasAuraState(flag, spellProto, caster) for the
// casting unit itself, used by the caster-state block of Spell::CheckCast
// (Spell.cpp:5298-5308). Per-caster states are only visible from auras
// applied by this caster (Unit.cpp:5957-5965). The
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass has no Go aura plumbing yet
// (noted gap, not a stub).
func (s *session) hasAuraState(state uint32) bool {
	if state == auraStateNone || s.player == nil {
		return false
	}
	if uint32(1)<<(state-1)&perCasterAuraStateMask != 0 {
		for _, aura := range s.loadedAuras() {
			if aura == nil || aura.CasterGUID != s.playerGUID {
				continue
			}
			if s.lookupAuraState(aura.SpellID) == state {
				return true
			}
		}
		return false
	}
	return s.unitAuraStateMask()&(uint32(1)<<(state-1)) != 0
}

// targetHasAuraState mirrors Unit::HasAuraState for the spell target, used by
// SpellInfo::CheckTarget (SpellInfo.cpp:1760-1766). Player targets resolve to
// their session's aura set; creature targets consult the server creature aura
// maps (same read pattern as targetHasAura). Per-caster states need an aura
// from the casting session; the creatureAuras presence map carries no caster
// GUID, so those only match through activeCreatureAuras (noted gap).
func (s *session) targetHasAuraState(ctx context.Context, targetGUID uint64, state uint32) bool {
	if state == auraStateNone || targetGUID == 0 {
		return false
	}
	if s.player != nil && targetGUID == s.playerGUID {
		return s.hasAuraState(state)
	}
	bit := uint32(1) << (state - 1)
	perCaster := bit&perCasterAuraStateMask != 0
	if s.server == nil {
		return false
	}
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		if perCaster {
			for _, aura := range targetSess.loadedAuras() {
				if aura == nil || aura.CasterGUID != s.playerGUID {
					continue
				}
				if s.lookupAuraState(aura.SpellID) == state {
					return true
				}
			}
			return false
		}
		return targetSess.unitAuraStateMask()&bit != 0
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		return false
	}
	type auraCaster struct {
		spellID    uint32
		casterGUID uint64
		attributed bool
	}
	var found []auraCaster
	key := creatureAuraKeyForTarget(target)
	s.server.auraMu.Lock()
	for spellID := range s.server.creatureAuras[key] {
		found = append(found, auraCaster{spellID: spellID})
	}
	for spellID, aura := range s.server.activeCreatureAuras[key] {
		entry := auraCaster{spellID: spellID, attributed: true}
		if aura != nil {
			entry.casterGUID = aura.CasterGUID
		}
		found = append(found, entry)
	}
	s.server.auraMu.Unlock()
	mask := auraStateHealthMask(target.Health, target.MaxHealth)
	for _, aura := range found {
		if s.lookupAuraState(aura.spellID) != state {
			continue
		}
		if perCaster && (!aura.attributed || aura.casterGUID != s.playerGUID) {
			continue
		}
		mask |= bit
	}
	return mask&bit != 0
}
