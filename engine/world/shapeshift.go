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

// shapeshiftBoostSpells mirrors the form -> (spellId, spellId2,
// HotWSpellId) table at the top of AuraEffect::HandleShapeshiftBoosts
// (SpellAuraEffects.cpp:1079-1145). Forms with no row (ghoul, ambient,
// stealth, creature cat/bear) yield zeros, matching the C++ break arms.
func shapeshiftBoostSpells(form uint8) (uint32, uint32, uint32) {
	switch form {
	case 1: // FORM_CAT
		return 3025, 0, 24900
	case 2: // FORM_TREE
		return 34123, 0, 0
	case 3: // FORM_TRAVEL
		return 5419, 0, 0
	case 4: // FORM_AQUA
		return 5421, 0, 0
	case 5: // FORM_BEAR
		return 1178, 21178, 24899
	case 8: // FORM_DIREBEAR
		return 9635, 21178, 24899
	case 16: // FORM_GHOSTWOLF
		return 67116, 0, 0
	case 17: // FORM_BATTLESTANCE
		return 21156, 0, 0
	case 18: // FORM_DEFENSIVESTANCE
		return 7376, 0, 0
	case 19: // FORM_BERSERKERSTANCE
		return 7381, 0, 0
	case 22: // FORM_METAMORPHOSIS
		return 54817, 54879, 0
	case 27: // FORM_FLIGHT_EPIC
		return 40122, 40121, 0
	case 28: // FORM_SHADOW
		return 49868, 71167, 0
	case 29: // FORM_FLIGHT
		return 33948, 34764, 0
	case 31: // FORM_MOONKIN
		return 24905, 69366, 0
	case 32: // FORM_SPIRITOFREDEMPTION
		return 27792, 27795, 0
	default:
		return 0, 0, 0
	}
}

// spellStanceMask folds the two ShapeshiftMask DBC words the way
// SpellInfo::Stances does (DBCStructure.h:1404: fields 12-13).
func spellStanceMask(spell wotlk.Spell) uint64 {
	return uint64(spell.ShapeshiftMask[0]) | uint64(spell.ShapeshiftMask[1])<<32
}

// playerHasKnownSpell mirrors Player::HasSpell's spell-map lookup
// (Player.cpp) for the passive checks in HandleShapeshiftBoosts: the spell
// is known and not disabled.
func (s *session) playerHasKnownSpell(spellID uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	for _, learned := range s.player.Spells {
		if learned.ID == spellID && !learned.Disabled {
			return true
		}
	}
	return false
}

// dummyAuraEffectAmount mirrors Unit::GetDummyAuraEffect(family, icon, 0)
// (Unit.cpp:4510-4524): the live effect-0 amount of the first matching
// SPELL_AURA_DUMMY aura.
func (s *session) dummyAuraEffectAmount(family uint32, iconID uint32) (int32, bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0, false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraDummy || aura.EffectMask&1 == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellFamilyName != family || spell.SpellIconID != iconID {
			continue
		}
		return aura.Amounts[0], true
	}
	return 0, false
}

// heartOfTheWildStaminaPct mirrors the Heart of the Wild arm of
// AuraEffect::HandleShapeshiftBoosts (SpellAuraEffects.cpp:1208-1224): the
// amount of the first MOD_TOTAL_STAT_PERCENTAGE aura with SpellIconID 240
// and MiscValue 3 (the Cat/Bear/Direbear talent). SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE = 137.
func (s *session) heartOfTheWildStaminaPct() int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != 137 || aura.EffectMask&1 == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellIconID != 240 {
			continue
		}
		if aura.MiscValue != 3 {
			continue
		}
		return aura.Amounts[0]
	}
	return 0
}

// savageRoarDummyActive mirrors the Savage Roar arm of
// AuraEffect::HandleShapeshiftBoosts (SpellAuraEffects.cpp:1227-1229):
// a live SPELL_AURA_DUMMY druid-family aura with family flags (0,
// 0x10000000, 0) (the Savage Roar talent aura).
func (s *session) savageRoarDummyActive() bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraDummy {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellFamilyName != spellFamilyDruid {
			continue
		}
		if spell.SpellFamilyFlags[0] == 0 && spell.SpellFamilyFlags[1] == 0x10000000 && spell.SpellFamilyFlags[2] == 0 {
			return true
		}
	}
	return false
}

// survivalOfTheFittestBonus mirrors the Survival of the Fittest arm of
// AuraEffect::HandleShapeshiftBoosts (SpellAuraEffects.cpp:1242-1248):
// the EFFECT_2 template value (not the live amount) of the
// MOD_TOTAL_STAT_PERCENTAGE druid-family aura with family flags[0] 961.
func (s *session) survivalOfTheFittestBonus() (int32, bool) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0, false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != 137 || aura.EffectMask&1 == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || spell.SpellFamilyName != spellFamilyDruid || spell.SpellFamilyFlags[0] != 961 {
			continue
		}
		if len(spell.Effects) < 3 {
			return 0, false
		}
		return spell.Effects[2].BasePoints, true
	}
	return 0, false
}

// newShapeshiftStanceMask mirrors the newAura lookup at the top of the
// HandleShapeshiftBoosts remove leg (SpellAuraEffects.cpp:1286-1297):
// the stance bit of another still-active shapeshift aura, or 0 when the
// player is leaving every form. removedSpellID is the aura being removed;
// the C++ comparison is against the removed AuraEffect itself, and Go holds
// one aura per spell ID, so the removed spell ID is the exact match.
func (s *session) newShapeshiftStanceMask(removedSpellID uint32) uint64 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModShapeshift || aura.SpellID == removedSpellID {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for _, effect := range spell.Effects {
			if effect.Aura != spellAuraModShapeshift || effect.MiscValue <= 0 {
				continue
			}
			return uint64(1) << (uint64(effect.MiscValue) - 1)
		}
	}
	return 0
}

// auraRemovedOnShapeLost mirrors Aura::IsRemovedOnShapeLost
// (SpellAuras.cpp:1096-1103): a self-cast aura whose spell carries a
// non-zero Stances mask and neither the NOT_SHAPESHIFT nor the
// NOT_NEED_SHAPESHIFT attribute.
func (s *session) auraRemovedOnShapeLost(aura *activeAura, spell wotlk.Spell) bool {
	if s == nil || s.player == nil || aura == nil {
		return false
	}
	if aura.CasterGUID != s.playerGUID {
		return false
	}
	if spellStanceMask(spell) == 0 {
		return false
	}
	if spell.AttributesEx1&spellAttr2NotNeedShapeshift != 0 {
		return false
	}
	if spell.Attributes&spellAttr0NotShapeshift != 0 {
		return false
	}
	return true
}

// handleShapeshiftBoosts mirrors AuraEffect::HandleShapeshiftBoosts
// (SpellAuraEffects.cpp:1072-1316) both legs. The caller runs it with
// prevForm != form on apply (SpellAuraEffects.cpp:1784) and unconditionally
// on remove (SpellAuraEffects.cpp:1860).
func (s *session) handleShapeshiftBoosts(ctx context.Context, spellID uint32, form uint8, apply bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	spellId, spellId2, hotWSpellId := shapeshiftBoostSpells(form)
	if apply {
		if spellId != 0 {
			s.castSpellDirect(ctx, spellId, s.playerGUID)
		}
		if spellId2 != 0 {
			s.castSpellDirect(ctx, spellId2, s.playerGUID)
		}
		var stanceBit uint64
		if form > 0 && form <= 64 {
			stanceBit = uint64(1) << (form - 1)
		}
		// Known passive/hidden-clientside spells gated on the new form's
		// stance bit (SpellAuraEffects.cpp:1158-1172).
		for _, learned := range s.player.Spells {
			if learned.Disabled || learned.ID == spellId || learned.ID == spellId2 {
				continue
			}
			spell, found, err := s.server.Data.Spell(learned.ID)
			if err != nil || !found {
				continue
			}
			if spell.Attributes&spellAttr0Passive == 0 && spell.Attributes&spellAttr0HiddenClientside == 0 {
				continue
			}
			if stanceBit != 0 && spellStanceMask(spell)&stanceBit != 0 {
				s.castSpellDirect(ctx, learned.ID, s.playerGUID)
			}
		}
		// Glyphs (SpellAuraEffects.cpp:1175-1188).
		for _, glyphID := range s.player.Glyphs[s.player.ActiveTalentGroup] {
			if glyphID == 0 {
				continue
			}
			glyph, found, err := s.server.Data.GlyphProperties(uint32(glyphID))
			if err != nil || !found {
				continue
			}
			spell, found, err := s.server.Data.Spell(glyph.SpellID)
			if err != nil || !found {
				continue
			}
			if spell.Attributes&spellAttr0Passive == 0 && spell.Attributes&spellAttr0HiddenClientside == 0 {
				continue
			}
			if stanceBit != 0 && spellStanceMask(spell)&stanceBit != 0 {
				s.castSpellDirect(ctx, glyph.SpellID, s.playerGUID)
			}
		}
		// Leader of the Pack (SpellAuraEffects.cpp:1191-1196).
		if s.playerHasKnownSpell(17007) {
			if spell, found, err := s.server.Data.Spell(24932); err == nil && found && stanceBit != 0 && spellStanceMask(spell)&stanceBit != 0 {
				s.castSpellDirect(ctx, 24932, s.playerGUID)
			}
		}
		// Improved Barkskin (SpellAuraEffects.cpp:1198-1205).
		if s.playerHasKnownSpell(63410) || s.playerHasKnownSpell(63411) {
			s.removeAura(66530)
			if form == 3 { // FORM_TRAVEL; FORM_NONE is impossible on the apply leg
				s.castSpellDirect(ctx, 66530, s.playerGUID)
			}
		}
		// Heart of the Wild (SpellAuraEffects.cpp:1208-1224): 1% stamina
		// and 1% attack power per 2% intellect.
		if hotWSpellId != 0 {
			if pct := s.heartOfTheWildStaminaPct(); pct > 0 {
				s.castSpellDirectWithBasePoint(ctx, hotWSpellId, s.playerGUID, uint32(pct/2))
			}
		}
		switch form {
		case 1: // FORM_CAT
			if s.savageRoarDummyActive() {
				s.castSpellDirect(ctx, 62071, s.playerGUID)
			}
			if amount, ok := s.dummyAuraEffectAmount(spellFamilyGeneric, 2851); ok && amount > 0 {
				s.castSpellDirectWithBasePoint(ctx, 48420, s.playerGUID, uint32(amount))
			}
		case 5, 8: // FORM_BEAR / FORM_DIREBEAR
			if amount, ok := s.dummyAuraEffectAmount(spellFamilyGeneric, 2851); ok && amount > 0 {
				s.castSpellDirectWithBasePoint(ctx, 48418, s.playerGUID, uint32(amount))
			}
			if bonus, ok := s.survivalOfTheFittestBonus(); ok && bonus > 0 {
				s.castSpellDirectWithBasePoint(ctx, 62069, s.playerGUID, uint32(bonus))
			}
		case 31: // FORM_MOONKIN
			if amount, ok := s.dummyAuraEffectAmount(spellFamilyGeneric, 2851); ok && amount > 0 {
				s.castSpellDirectWithBasePoint(ctx, 48421, s.playerGUID, uint32(amount))
			}
		case 2: // FORM_TREE
			if amount, ok := s.dummyAuraEffectAmount(spellFamilyGeneric, 2851); ok && amount > 0 {
				s.castSpellDirectWithBasePoint(ctx, 48422, s.playerGUID, uint32(amount))
			}
		}
		return
	}
	// Remove leg (SpellAuraEffects.cpp:1278-1316). RemoveOwnedAura's caster
	// match is vacuous: Go holds one aura per spell ID and the companion
	// casts are self-casts.
	if spellId != 0 {
		s.removeAura(spellId)
	}
	if spellId2 != 0 {
		s.removeAura(spellId2)
	}
	// Improved Barkskin remove (SpellAuraEffects.cpp:1280-1285).
	if s.playerHasKnownSpell(63410) || s.playerHasKnownSpell(63411) {
		s.removeAura(66530)
		s.castSpellDirect(ctx, 66530, s.playerGUID)
	}
	// Strip auras that don't fit the form being shifted into
	// (SpellAuraEffects.cpp:1299-1315).
	newStance := s.newShapeshiftStanceMask(spellID)
	var strip []uint32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if !s.auraRemovedOnShapeLost(aura, spell) {
			continue
		}
		if spellStanceMask(spell)&newStance == 0 {
			strip = append(strip, aura.SpellID)
		}
	}
	for _, id := range strip {
		s.removeAura(id)
	}
}

// addTemporaryShapeshiftSpell mirrors Player::AddTemporarySpell
// (Player.cpp:3609-3620): a spell already in the list — temporary or
// permanent — is left alone; otherwise it is recorded active and temporary.
// Temporary spells are never persisted (Player::_SaveSpells skips
// PLAYERSPELL_TEMPORARY), so no DB write happens here.
func (s *session) addTemporaryShapeshiftSpell(spellID uint32) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	for _, learned := range s.player.Spells {
		if learned.ID == spellID {
			return
		}
	}
	s.player.Spells = append(s.player.Spells, learnedSpell{ID: spellID, Active: true, Temporary: true})
}

// removeTemporaryShapeshiftSpell mirrors Player::RemoveTemporarySpell
// (Player.cpp:3622-3630): only a temporary entry is erased; a permanently
// learned copy of the same spell survives.
func (s *session) removeTemporaryShapeshiftSpell(spellID uint32) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	for i, learned := range s.player.Spells {
		if learned.ID == spellID && learned.Temporary {
			s.player.Spells = append(s.player.Spells[:i], s.player.Spells[i+1:]...)
			return
		}
	}
}

// updateFormPresetSpells mirrors the spell-learning tail of
// AuraEffect::HandleAuraModShapeshift (SpellAuraEffects.cpp:1897-1907):
// the SpellShapeshiftFormEntry preset spells are granted as temporary
// spells on apply and revoked on remove. C++ notes no action-bar or
// spellbook packet is needed.
func (s *session) updateFormPresetSpells(form uint8, apply bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	shape, found, err := s.server.Data.ShapeshiftForm(uint32(form))
	if err != nil || !found {
		return
	}
	for _, presetID := range shape.PresetSpellIDs {
		if presetID == 0 {
			continue
		}
		if apply {
			s.addTemporaryShapeshiftSpell(presetID)
		} else {
			s.removeTemporaryShapeshiftSpell(presetID)
		}
	}
}
