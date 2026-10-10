package world

import (
	"context"
	"math/rand/v2"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// formShadow mirrors FORM_SHADOW (SpellAuraDefines.h:435): the Shadowform
// shapeshift form id carried by SPELL_AURA_MOD_SHAPESHIFT's MiscValue and,
// through refreshTransformDisplay, by state.ShapeshiftForm.
const formShadow uint8 = 0x1C

func playerPowerType(state *playerState) uint8 {
	if state == nil {
		return 0
	}
	switch state.ShapeshiftForm {
	case 1, 7:
		return 3
	case 5, 8, 17, 18, 19:
		return 1
	default:
		return classPowerType(state.Class)
	}
}

func updatePlayerShapeshiftForm(state *playerState, data *wotlk.Store, form uint8, spellID uint32) uint32 {
	if state == nil {
		return 0
	}
	if state.ShapeshiftForm == 0 && form != 0 {
		state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime = state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime
	}
	if form == 0 {
		if state.ShapeshiftForm != 0 {
			state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime = state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime
			state.ShapeshiftBaseAttackTime, state.ShapeshiftBaseOffhandAttackTime, state.ShapeshiftBaseRangedAttackTime = 0, 0, 0
		}
		state.ShapeshiftForm = 0
		return 0
	}
	state.ShapeshiftForm = form
	if data != nil {
		if shape, found, err := data.ShapeshiftForm(uint32(form)); err == nil && found && shape.CombatRoundTime != 0 {
			state.AttackTime, state.OffhandAttackTime, state.RangedAttackTime = shape.CombatRoundTime, shape.CombatRoundTime, 2000
		}
	}
	return shapeshiftFormDisplayID(data, state, form, spellID)
}

func shapeshiftFormDisplayID(data *wotlk.Store, state *playerState, form uint8, spellID uint32) uint32 {
	if state == nil {
		return 0
	}
	switch spellID {
	case 7090:
		return 29414
	case 35200:
		return 4877
	}
	if form == 1 {
		if state.Race == 4 {
			switch state.HairColor {
			case 7, 8:
				return 29405
			case 3:
				return 29406
			case 0, 1, 2:
				return 29407
			case 4:
				return 29408
			default:
				return 892
			}
		}
		if state.Race == 6 {
			if state.Gender == 0 {
				switch state.Skin {
				case 12, 13, 14, 18:
					return 29409
				case 9, 10, 11:
					return 29410
				case 6, 7, 8:
					return 29411
				case 0, 1, 2, 3, 4, 5:
					return 29412
				default:
					return 8571
				}
			}
			switch state.Skin {
			case 10:
				return 29409
			case 6, 7:
				return 29410
			case 4, 5:
				return 29411
			case 0, 1, 2, 3:
				return 29412
			default:
				return 8571
			}
		}
		if isAllianceRace(state.Race) {
			return 892
		}
		return 8571
	}
	if form == 5 || form == 8 {
		if state.Race == 4 {
			switch state.HairColor {
			case 0, 1, 2:
				return 29413
			case 6:
				return 29414
			case 4:
				return 29416
			case 3:
				return 29417
			default:
				return 2281
			}
		}
		if state.Race == 6 {
			if state.Gender == 0 {
				switch state.Skin {
				case 0, 1, 2:
					return 29418
				case 3, 4, 5, 12, 13, 14:
					return 29419
				case 9, 10, 11, 15, 16, 17:
					return 29420
				case 18:
					return 29421
				default:
					return 2289
				}
			}
			switch state.Skin {
			case 0, 1:
				return 29418
			case 2, 3:
				return 29419
			case 6, 7, 8, 9:
				return 29420
			case 10:
				return 29421
			default:
				return 2289
			}
		}
		if isAllianceRace(state.Race) {
			return 2281
		}
		return 2289
	}
	if form == 29 {
		if isAllianceRace(state.Race) {
			return 20857
		}
		return 20872
	}
	if form == 27 {
		if isAllianceRace(state.Race) {
			return 21243
		}
		return 21244
	}
	if data == nil {
		return 0
	}
	shape, found, err := data.ShapeshiftForm(uint32(form))
	if err != nil || !found {
		return 0
	}
	if isAllianceRace(state.Race) || shape.CreatureDisplayIDs[1] == 0 {
		return shape.CreatureDisplayIDs[0]
	}
	return shape.CreatureDisplayIDs[1]
}

// checkShapeshiftCast mirrors SpellInfo::CheckShapeshift (SpellInfo.cpp:1455):
// validates the caster's current shapeshift form against the spell's
// ShapeshiftMask (C++ Stances) / ShapeshiftExclude (C++ StancesNot) DBC fields.
// Returns 0 on success, or a SPELL_FAILED_* cast result otherwise.
func (s *session) checkShapeshiftCast(spell wotlk.Spell) uint8 {
	var form uint64
	if s.player != nil {
		form = uint64(s.player.ShapeshiftForm)
	}
	return checkShapeshiftCastForm(s.server.Data, spell, form)
}

// checkShapeshiftCastForm is the form-parameterized core of
// checkShapeshiftCast (SpellInfo::CheckShapeshift, SpellInfo.cpp:1450):
// the creature bridge passes form 0 — creatures carry no shapeshift
// form, so stanceMask is 0 and only the Stances != 0 arm can fire.
func checkShapeshiftCastForm(data *wotlk.Store, spell wotlk.Spell, form uint64) uint8 {
	// Talents that learn spells can have stance requirements that need ignore
	// (this requirement is only for client-side stance show in the talent
	// description) (SpellInfo.cpp:1451-1455): a talent-ranked spell carrying
	// SPELL_EFFECT_LEARN_SPELL on any of the first three effects bypasses the
	// stance gates entirely.
	if data != nil {
		if cost, costErr := data.TalentSpellCost(spell.ID); costErr == nil && cost > 0 {
			for i := 0; i < len(spell.Effects) && i < 3; i++ {
				if spell.Effects[i].Effect == spellEffectLearnSpell {
					return 0
				}
			}
		}
	}
	stances := uint64(spell.ShapeshiftMask[0]) | uint64(spell.ShapeshiftMask[1])<<32
	stancesNot := uint64(spell.ShapeshiftExclude[0]) | uint64(spell.ShapeshiftExclude[1])<<32
	var stanceMask uint64
	if form > 0 && form <= 64 {
		stanceMask = uint64(1) << (form - 1)
	}
	if stanceMask&stancesNot != 0 {
		return spellFailedNotShapeshift
	}
	if stanceMask&stances != 0 {
		return 0
	}
	actAsShifted := false
	shapeKnown := false
	var shapeFlags uint32
	if form > 0 && data != nil {
		shape, found, err := data.ShapeshiftForm(uint32(form))
		if err != nil || !found {
			return 0
		}
		shapeKnown = true
		shapeFlags = shape.Flags
		actAsShifted = shapeFlags&1 == 0
	}
	if actAsShifted {
		if spell.Attributes&spellAttr0NotShapeshift != 0 {
			return spellFailedNotShapeshift
		}
		if stances != 0 {
			return spellFailedOnlyShapeshift
		}
	} else {
		if spell.AttributesEx1&spellAttr2NotNeedShapeshift == 0 && stances != 0 {
			return spellFailedOnlyShapeshift
		}
	}
	if shapeKnown && shapeFlags&0x400 != 0 {
		if stanceMask&stances == 0 {
			return spellFailedOnlyShapeshift
		}
	}
	return 0
}

// hasIgnoreShapeshiftAura mirrors the Spell::CheckCast gate at Spell.cpp:5250-5260:
// the shapeshift check is skipped when any SPELL_AURA_MOD_IGNORE_SHAPESHIFT aura
// effect is affected on the spell (AuraEffect::IsAffectedOnSpell).
func (s *session) hasIgnoreShapeshiftAura(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModIgnoreShapeshift {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraModIgnoreShapeshift || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				return true
			}
		}
	}
	return false
}

// predatoryStrikesPct mirrors the Predatory Strikes lookup in
// Player::UpdateAttackPowerAndDamage (StatSystem.cpp:407-419): the
// SPELL_AURA_DUMMY aura of the druid family with SpellIconID 1563 (the
// icon-ID overload of Unit::GetAuraEffect, Unit.cpp:4510-4524 — the
// spell must carry no family flags). Returns the EFFECT_0 (level) and
// EFFECT_1 (weapon) percent amounts.
func (s *session) predatoryStrikesPct() (levelPct, weaponPct float64, ok bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0, 0, false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellFamilyName != spellFamilyDruid || spell.SpellIconID != 1563 {
			continue
		}
		if spell.SpellFamilyFlags[0] != 0 || spell.SpellFamilyFlags[1] != 0 || spell.SpellFamilyFlags[2] != 0 {
			continue
		}
		matched := false
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || effect.Aura != spellAuraDummy || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			matched = true
			if index == 0 {
				levelPct = float64(aura.Amounts[index])
			} else if index == 1 {
				weaponPct = float64(aura.Amounts[index])
			}
		}
		if matched {
			return levelPct, weaponPct, true
		}
	}
	return 0, 0, false
}

const powerEnergy = 3 // POWER_ENERGY (SharedDefines.h power enum)

// spellHasCuAuraCC mirrors the SPELL_ATTR0_CU_AURA_CC arms of the C++
// spell-load pass (SpellMgr.cpp:2662-2672, 2842-2858): any possess/confuse/
// charm/fear/stun aura effect, warrior shouts (family flags[0] & 0x20000),
// druid roars (family flags[0] & 0x8), and the Stoneclaw Totem effect (5729).
// Unit::RemoveAurasByShapeShift consults it through HasAttribute.
func spellHasCuAuraCC(spell wotlk.Spell) bool {
	for _, effect := range spell.Effects {
		switch effect.Aura {
		case spellAuraModPossess, spellAuraModConfuse, spellAuraCharm, spellAuraAoeCharm, spellAuraModFear, spellAuraModStun:
			return true
		}
	}
	switch spell.SpellFamilyName {
	case spellFamilyWarrior:
		return spell.SpellFamilyFlags[0]&0x20000 != 0 // Shout / Piercing Howl
	case spellFamilyDruid:
		return spell.SpellFamilyFlags[0]&0x8 != 0 // Roar
	case spellFamilyGeneric:
		return spell.ID == 5729 // Stoneclaw Totem effect
	}
	return false
}

// removeAurasByShapeShift mirrors Unit::RemoveAurasByShapeShift
// (Unit.cpp:4234-4248): every aura whose all-effects mechanic mask touches
// snare|root leaves, except CC auras carrying the custom AURA_CC attribute.
func (s *session) removeAurasByShapeShift() {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	var mask uint32 = (1 << mechanicSnare) | (1 << mechanicRoot)
	var ids []uint32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if spellMechanicMask(spell)&mask != 0 && !spellHasCuAuraCC(spell) {
			ids = append(ids, aura.SpellID)
		}
	}
	for _, id := range ids {
		s.expirePlayerAura(id)
	}
}

// removeOtherShapeshiftAuras mirrors the
// RemoveAurasByType(SPELL_AURA_MOD_SHAPESHIFT, ObjectGuid::Empty, GetBase())
// arm of AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1712): a
// newly applied shapeshift strips every other shapeshift aura; the applying
// spell itself is the excluded base.
func (s *session) removeOtherShapeshiftAuras(exceptSpellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	var ids []uint32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModShapeshift || aura.SpellID == exceptSpellID {
			continue
		}
		ids = append(ids, aura.SpellID)
	}
	for _, id := range ids {
		s.expirePlayerAura(id)
	}
}

// removePolymorphAura mirrors the polymorph-drop arm of
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1707-1708):
// shifting into a listed form while polymorphed removes the transform aura
// (Unit::IsPolymorphed + RemoveAurasDueToSpell(GetTransformSpell()),
// Unit.cpp:10574-10584 — the transform spell whose specific is
// SPELL_SPECIFIC_MAGE_POLYMORPH).
func (s *session) removePolymorphAura() {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraTransform {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if spellSpecific(spell, s.server.spellFirstRank) == spellSpecificMagePolymorph {
			s.expirePlayerAura(aura.SpellID)
			return
		}
	}
}

// furorProcChance mirrors the Furor lookup in
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1731-1733): the
// SPELL_AURA_DUMMY effect-0 amount of the druid-family aura with SpellIconID
// 238, floored at 0.
func (s *session) furorProcChance() int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraDummy || aura.EffectMask&1 == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellFamilyName != spellFamilyDruid || spell.SpellIconID != 238 {
			continue
		}
		if aura.Amounts[0] > 0 {
			return aura.Amounts[0]
		}
		return 0
	}
	return 0
}

// applyShapeshiftFormEffects runs the apply-leg arms of
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1696-1790) that
// the refreshTransformDisplay + calculatePlayerStats pair below doesn't
// cover: the other-form strip, the snare/root + polymorph drop for the
// listed forms, and the Furor power arms for cat/bear/direbear. The
// HandleShapeshiftBoosts companion casts, the Dash amount recalc, and the
// form spell-learning arms are separate units.
func (s *session) applyShapeshiftFormEffects(ctx context.Context, spellID uint32, form uint8) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	s.removeOtherShapeshiftAuras(spellID)
	switch form {
	case 1, 2, 3, 4, 5, 8, 27, 29, 31: // cat/tree/travel/aqua/bear/direbear/flight-epic/flight/moonkin
		s.removeAurasByShapeShift()
		s.removePolymorphAura()
	}
	switch form {
	case 1, 5, 8: // cat/bear/direbear carry non-mana power: the Furor arms
		chance := s.furorProcChance()
		if form == 1 {
			oldPower := s.player.Powers[powerEnergy]
			s.adjustSpellPower(ctx, s.playerGUID, powerEnergy, -int64(oldPower))
			grant := int32(oldPower)
			if grant > chance {
				grant = chance
			}
			if grant < 0 {
				grant = 0
			}
			s.castSpellDirectWithBasePoint(ctx, 17099, s.playerGUID, uint32(grant))
		} else if chance > 0 && rand.IntN(100) < int(chance) {
			s.castSpellDirect(ctx, 17057, s.playerGUID)
		}
	}
}

// hasLiveAuraEffect reports whether the spell's effect index is live on the
// player (Unit::GetAuraEffect(spellId, effIndex) non-null, Unit.cpp:4494).
func (s *session) hasLiveAuraEffect(spellID uint32, effIndex uint8) bool {
	if s == nil {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	aura, ok := s.activeAuras[spellID]
	return ok && aura != nil && !aura.Stopped && aura.EffectMask&(1<<uint(effIndex)) != 0
}

// defensiveTacticsRage mirrors Unit::IsScriptOverriden(m_spellInfo, 831)
// (Unit.cpp:4764-4776) for the defensive-stance remove arm: the first
// SPELL_AURA_OVERRIDE_CLASS_SCRIPTS aura with MiscValue 831 whose family
// mask affects the stance spell, amount x10 into POWER_RAGE units.
func (s *session) defensiveTacticsRage(stanceSpell wotlk.Spell) uint32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != auraOverrideClassScripts || aura.MiscValue != 831 {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, stanceSpell) {
				if aura.Amounts[index] > 0 {
					return uint32(aura.Amounts[index]) * 10
				}
				return 0
			}
		}
		return 0
	}
	return 0
}

// clampStanceRage mirrors the warrior-stance remove arm of
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1826-1854):
// leaving a stance clamps rage to the retained amount — Defensive Tactics
// (form 18 only) plus Stance Mastery / Tactical Mastery (warrior family,
// SpellIconID 139) effect-0 base points, all x10 into POWER_RAGE units.
func (s *session) clampStanceRage(stanceSpellID uint32, form uint8) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	var rageVal uint32
	if form == 18 { // FORM_DEFENSIVESTANCE
		if stanceSpell, found, err := s.server.Data.Spell(stanceSpellID); err == nil && found {
			rageVal += s.defensiveTacticsRage(stanceSpell)
		}
	}
	for _, learned := range s.player.Spells {
		if learned.Disabled {
			continue
		}
		sp, found, err := s.server.Data.Spell(learned.ID)
		if err != nil || !found || sp.SpellFamilyName != spellFamilyWarrior || sp.SpellIconID != 139 {
			continue
		}
		if len(sp.Effects) == 0 || sp.Effects[0].BasePoints < 0 {
			continue
		}
		rageVal += uint32(sp.Effects[0].BasePoints+1) * 10
	}
	if cur := s.player.Powers[powerRage]; cur > rageVal {
		s.adjustSpellPower(context.Background(), s.playerGUID, powerRage, -int64(cur-rageVal))
	}
}

// removeShapeshiftFormEffects runs the remove-leg arms of
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1808-1860): the
// druid shift-out movement-impair strip (only once the last shapeshift aura
// left), the Nordrassil set-bonus procs, and the warrior stance rage clamp.
// The form reset and display restore ride the refreshTransformDisplay call
// above; HandleShapeshiftBoosts(target, false) is a separate unit.
func (s *session) removeShapeshiftFormEffects(stanceSpellID uint32, form uint8) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if !s.hasAuraType(spellAuraModShapeshift) && s.player.Class == 11 { // CLASS_DRUID
		s.removeAurasByShapeShift()
	}
	switch form {
	case 1, 5, 8: // cat/bear/direbear: Nordrassil Harness bonus (37315 -> 37316)
		if s.hasLiveAuraEffect(37315, 0) {
			s.castSpellDirect(context.Background(), 37316, s.playerGUID)
		}
	case 31: // moonkin: Nordrassil Regalia bonus (37324 -> 37325)
		if s.hasLiveAuraEffect(37324, 0) {
			s.castSpellDirect(context.Background(), 37325, s.playerGUID)
		}
	case 17, 18, 19: // warrior stances: rage clamp
		s.clampStanceRage(stanceSpellID, form)
	}
}
