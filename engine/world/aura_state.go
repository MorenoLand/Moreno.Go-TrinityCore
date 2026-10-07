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

// Spell family names used by _LoadAuraState (SharedDefines.h:3585-3589)
// plus SPELLFAMILY_POTION (SharedDefines.h:3594) for the EffectEnergize
// power-type gate.
const (
	spellFamilyWarrior = 4
	spellFamilyWarlock = 5
	spellFamilyDruid   = 7
	spellFamilyRogue   = 8
	spellFamilyPotion  = 13
)

// cloneCasterGUIDForUnit is the per-unit aura-index lookup for the mirror
// image handler: it returns the caster GUID of the target unit's live
// SPELL_AURA_CLONE_CASTER aura (the aura does not stack in C++, so the first
// match is the GetAuraEffectsByType(...).front() analog).
// Reference: WorldSession::HandleMirrorImageDataRequest (SpellHandler.cpp:740-745).
// Player targets resolve through the session's activeAuras under its castMu
// (the dispel.go locking pattern); creature and pet targets consult the
// server's activeCreatureAuras index under auraMu. A zero caster GUID is
// the C++ !GetCaster() return. Unresolvable GUIDs yield no match, like the
// C++ !unit return.
func (s *session) cloneCasterGUIDForUnit(targetGUID uint64) (uint64, bool) {
	if s == nil || s.server == nil || targetGUID == 0 {
		return 0, false
	}
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		for _, aura := range ts.activeAuras {
			if aura == nil || aura.Stopped || aura.AuraType != spellAuraCloneCaster {
				continue
			}
			casterGUID := aura.CasterGUID
			ts.castMu.Unlock()
			if casterGUID == 0 {
				return 0, false
			}
			return casterGUID, true
		}
		ts.castMu.Unlock()
		return 0, false
	}
	if s.player == nil {
		return 0, false
	}
	s.server.auraMu.Lock()
	defer s.server.auraMu.Unlock()
	for _, aura := range s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, targetGUID)] {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraCloneCaster {
			continue
		}
		if aura.CasterGUID == 0 {
			return 0, false
		}
		return aura.CasterGUID, true
	}
	return 0, false
}

// spellAuraAbilityIgnoreAuraState is SPELL_AURA_ABILITY_IGNORE_AURASTATE
// (SpellAuraDefines.h:342): a caster-side aura effect that makes
// Unit::HasAuraState succeed for spells its affect mask covers
// (Unit.cpp:5946-5954).
const spellAuraAbilityIgnoreAuraState = 262

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
	// Sting (hunter's pet ability, SpellInfo.cpp:1986-1988)
	if spell.Category == 1133 {
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
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass runs first, like C++
// (Unit.cpp:5946-5954): a matching caster aura makes every state query
// succeed, including the Exclude (Not) terms.
func (s *session) hasAuraState(state uint32, spell wotlk.Spell) bool {
	if state == auraStateNone || s.player == nil {
		return false
	}
	if s.casterIgnoresAuraState(spell) {
		return true
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
// GUID, so those only match through activeCreatureAuras (noted gap). The
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass reads the caster's own auras
// (Unit.cpp:5946-5954), so it runs before the target-kind dispatch below.
func (s *session) targetHasAuraState(ctx context.Context, targetGUID uint64, state uint32, spell wotlk.Spell) bool {
	if state == auraStateNone || targetGUID == 0 {
		return false
	}
	// The bypass reads the caster's auras, not the target's (Unit.cpp:5952),
	// so it applies on every target kind.
	if s.casterIgnoresAuraState(spell) {
		return true
	}
	if s.player != nil && targetGUID == s.playerGUID {
		return s.hasAuraState(state, spell)
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

// casterIgnoresAuraState reports whether the casting session carries an aura
// effect of type SPELL_AURA_ABILITY_IGNORE_AURASTATE whose spell affects the
// spell being cast, mirroring the bypass term at the top of
// Unit::HasAuraState (Unit.cpp:5946-5954) via HasAuraTypeWithAffectMask
// (Unit.cpp:4689-4696).
func (s *session) casterIgnoresAuraState(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	data := s.server.Data
	return auraStateBypassApplies(s.loadedAuras(), func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, -1)
}

// auraStateBypassApplies is the data-free core of casterIgnoresAuraState: true
// when any live aura grants a 262 effect (masked in by EffectMask, the
// per-effect merge accumulator) whose spell-family affect mask covers the
// spell being cast (AuraEffect::IsAffectedOnSpell, SpellAuraEffects.cpp:848).
// miscValue < 0 disables the MiscValue filter; otherwise only effects whose
// MiscValue equals it count (AuraEffect::GetMiscValue reads the granting
// spell's effect row: m_spellInfo->Effects[m_effIndex].MiscValue).
func auraStateBypassApplies(auras []*activeAura, grantingSpell func(uint32) (wotlk.Spell, bool), spell wotlk.Spell, miscValue int32) bool {
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, ok := grantingSpell(aura.SpellID)
		if !ok {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraAbilityIgnoreAuraState || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if miscValue >= 0 && effect.MiscValue != miscValue {
				continue
			}
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				return true
			}
		}
	}
	return false
}

// auraStateReqCombatExempt mirrors the reqCombat leg of Spell::CheckCast
// (Spell.cpp:5280-5292): a SPELL_AURA_ABILITY_IGNORE_AURASTATE (262)
// effect affecting the spell whose MiscValue is 1 lifts the in-combat
// CanBeUsedInCombat gate (the SPELL_FAILED_AFFECTING_COMBAT arm of the
// caster-state block, Spell.cpp:5311-5312).
func (s *session) auraStateReqCombatExempt(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	data := s.server.Data
	return auraStateBypassApplies(s.loadedAuras(), func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, 1)
}
