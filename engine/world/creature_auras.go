package world

import (
	"math"
	"strings"
)

type creatureAuraKey struct {
	Map        uint32
	InstanceID uint32
	GUID       uint64
}

func creatureAuraKeyForTarget(target combatTarget) creatureAuraKey {
	return creatureAuraKey{Map: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}
}

func creatureAuraKeyForPlayer(state playerState, guid uint64) creatureAuraKey {
	return creatureAuraKey{Map: state.Map, InstanceID: state.InstanceID, GUID: guid}
}

func creatureAuraKeyForMotion(motion *creatureMotion) creatureAuraKey {
	if motion == nil {
		return creatureAuraKey{}
	}
	return creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}
}

// refreshCreaturePhaseOverrideLocked mirrors the creature arm of
// AuraEffect::HandlePhase (SpellAuraEffects.cpp:1581-1632): a creature's
// runtime phase mask is the OR of its remaining SPELL_AURA_PHASE (261) aura
// misc values. With no 261 aura the C++ falls back to PHASEMASK_NORMAL or
// the spawn's data phaseMask — exactly the c.phaseMask row the visibility
// queries already read — so the override exists only while a 261 aura is
// active. The aura's MiscValue carries the effect's misc (see the activeAura
// construction in applyAuraToTarget); multi-effect spells store one record
// per spell ID, so the OR runs over the per-spell records. Caller must hold
// auraMu.
func (s *Server) refreshCreaturePhaseOverrideLocked(key creatureAuraKey) {
	if s == nil || key.GUID == 0 {
		return
	}
	var mask uint32
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			for _, aura := range auras {
				if aura != nil && !aura.Stopped && aura.AuraType == spellAuraPhase {
					mask |= uint32(aura.MiscValue)
				}
			}
		}
	}
	if s.creaturePhaseOverride == nil {
		s.creaturePhaseOverride = make(map[creatureAuraKey]uint32)
	}
	if mask == 0 {
		delete(s.creaturePhaseOverride, key)
	} else {
		s.creaturePhaseOverride[key] = mask
	}
}

// creaturePhaseOverrideGUIDs returns the spawn-table GUIDs (c.guid) on mapID
// whose runtime 261-phase override intersects observerMask — observerMask 0
// matches any non-zero override. The visibility queries only see the spawn's
// static c.phaseMask row, so these GUIDs are OR'd into the SQL phase
// predicate. Pets (0xF140) and non-creature keys are excluded: they have no
// creature-table row to match.
func (s *Server) creaturePhaseOverrideGUIDs(mapID uint32, observerMask uint32) []uint32 {
	if s == nil {
		return nil
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	var guids []uint32
	for key, mask := range s.creaturePhaseOverride {
		if key.Map != mapID || mask == 0 || key.GUID>>48 != 0xF130 {
			continue
		}
		if observerMask == 0 || mask&observerMask != 0 {
			guids = append(guids, uint32(key.GUID&0xFFFFFF))
		}
	}
	return guids
}

// creaturePhasePredicate builds the SQL phase clause for a creature query:
// the static row gate plus the runtime 261-phase overrides for observerMask.
// The override GUID args are returned in clause order for the query args.
func creaturePhasePredicate(staticClause string, overrideGUIDs []uint32) (string, []any) {
	if len(overrideGUIDs) == 0 {
		return staticClause, nil
	}
	marks := make([]string, len(overrideGUIDs))
	args := make([]any, len(overrideGUIDs))
	for i, g := range overrideGUIDs {
		marks[i] = "?"
		args[i] = g
	}
	return "(" + staticClause + " OR c.guid IN (" + strings.Join(marks, ",") + "))", args
}

// creatureAuraScaleSum mirrors the creature half of
// Unit::RecalculateObjectScale (Unit.cpp:10593-10598): the summed amounts
// of SPELL_AURA_MOD_SCALE (61) and SPELL_AURA_MOD_SCALE_2 (239) effects
// on the creature's live auras. The player half is auraObjectScale
// (aura_unit.go); creatures floor at 0.01, not 0.1.
func (s *Server) creatureAuraScaleSum(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 0
	}
	var sum int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModScale && effect.Aura != spellAuraModScale2 {
				continue
			}
			if index < len(aura.Amounts) {
				sum += aura.Amounts[index]
			}
		}
	}
	s.auraMu.Unlock()
	return sum
}

// creatureCanInstantCast mirrors the >=1000 arm of
// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3901-3922): a
// live SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (65) effect with amount >=
// 1000 sets the unit's instant-cast flag (Unit::SetInstantCast), and
// WorldObject::ModSpellCastTime (Object.cpp:2471) then reduces the cast
// time to 0. Only aura 65 takes this path in C++; 216 never does.
func (s *Server) creatureCanInstantCast(key creatureAuraKey) bool {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return false
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraCastingSpeedNotStack {
				continue
			}
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if aura.Amounts[index] >= 1000 {
				return true
			}
		}
	}
	return false
}

// creatureCastingSpeedLiveAmounts mirrors the sub-1000 arm of
// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3924-3936) for
// creature auras — the creature twin of castingSpeedNotStackLiveAmounts
// (aura_haste.go:191). Each live 65/216 effect with amount < 1000 is a
// candidate; the exclusive-same-effect gate
// (Unit::GetHighestExclusiveSameEffectSpellGroupValue, Unit.cpp:13967)
// keeps an effect only when no same-type rival in a shared exclusive
// group carries |amount| >= its own (ties lose, C++ :3931).
func (s *Server) creatureCastingSpeedLiveAmounts(key creatureAuraKey) []int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return nil
	}
	var cands []statCandidate
	s.auraMu.Lock()
	order := 0
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			order++
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			order++
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraCastingSpeedNotStack && effect.Aura != spellAuraHasteSpells {
				continue
			}
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if aura.Amounts[index] >= 1000 {
				continue
			}
			cands = append(cands, statCandidate{
				order:       order,
				spellID:     aura.SpellID,
				effectIndex: index,
				amount:      aura.Amounts[index],
				firstRank:   s.spellFirstRank(aura.SpellID),
				auraType:    effect.Aura,
			})
		}
		order++
	}
	s.auraMu.Unlock()
	if len(cands) == 0 {
		return nil
	}
	cache := s.petAuraStackGroups()
	var live []int32
	for _, c := range cands {
		rivalMax := int32(0)
		for _, r := range cands {
			if r.auraType != c.auraType {
				continue
			}
			if r.spellID == c.spellID && r.effectIndex == c.effectIndex && r.order == c.order {
				continue
			}
			if !sharesExclusiveSameEffectGroup(c.firstRank, r.firstRank, cache) {
				continue
			}
			if absAuraAmount(r.amount) > absAuraAmount(rivalMax) {
				rivalMax = r.amount
			}
		}
		if absAuraAmount(rivalMax) >= absAuraAmount(c.amount) {
			continue
		}
		live = append(live, c.amount)
	}
	return live
}

// creatureCastingSpeedCastFactor mirrors the cast-time arm of
// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3933/3935) via
// Unit::ApplyCastTimePercentMod (Unit.cpp:10867) for creature auras — the
// creature twin of castingSpeedCastFactor (aura_haste.go:261): positive
// amounts divide UNIT_MOD_CAST_SPEED by (1+amount/100) (haste), negative
// amounts multiply by (1+|amount|/100) (slow), composing multiplicatively.
func (s *Server) creatureCastingSpeedCastFactor(key creatureAuraKey) float64 {
	factor := 1.0
	for _, a := range s.creatureCastingSpeedLiveAmounts(key) {
		if a > 0 {
			factor /= 1.0 + float64(a)/100.0
		} else {
			factor *= 1.0 + float64(-a)/100.0
		}
	}
	return factor
}

// creatureAuraTypeAmountsByMiscMask mirrors the per-effect iteration of
// Unit::GetTotalAuraModifierByMiscMask (Unit.cpp:4977) for creature auras:
// the live amount of each active effect of auraType whose DBC MiscValue
// overlaps miscMask.
func (s *Server) creatureAuraTypeAmountsByMiscMask(key creatureAuraKey, auraType uint32, miscMask uint32) []int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return nil
	}
	var amounts []int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != auraType || uint32(effect.MiscValue)&miscMask == 0 {
				continue
			}
			if index < len(aura.Amounts) {
				amounts = append(amounts, aura.Amounts[index])
			}
		}
	}
	s.auraMu.Unlock()
	return amounts
}

// creatureIgnoreTargetResistArmor mirrors the creature-attacker arms of
// Unit::CalcArmorReducedDamage (Unit.cpp:1632-1650) on the auto-swing path
// (null spellInfo): the attacker's SPELL_AURA_MOD_TARGET_RESISTANCE (123)
// NORMAL-misc flat sum lands on the victim's armor first, then each
// SPELL_AURA_MOD_IGNORE_TARGET_RESIST (269) NORMAL-misc effect shrinks it by
// floor(armor*(100-amount)/100). The 270 leg never fires (null spellInfo),
// the SPELLMOD_IGNORE_ARMOR leg is player-only, and the 280/rating armor
// penetration leg is TYPEID_PLAYER-gated in C++ — creatures get none.
func (s *Server) creatureIgnoreTargetResistArmor(key creatureAuraKey, armor float64) float64 {
	if armor <= 0 {
		return armor
	}
	var flat int32
	for _, amount := range s.creatureAuraTypeAmountsByMiscMask(key, spellAuraModTargetResistance, spellSchoolMaskNormal) {
		flat += amount
	}
	armor += float64(flat)
	for _, amount := range s.creatureAuraTypeAmountsByMiscMask(key, spellAuraModIgnoreTargetResist, spellSchoolMaskNormal) {
		armor = math.Floor(armor * (100 - float64(amount)) / 100)
	}
	if armor < 0 {
		armor = 0
	}
	return armor
}

// creatureAuraDamageDoneFlat mirrors the creature half of the flat arm
// AuraEffect::HandleModDamageDone calls on every target type
// (SpellAuraEffects.cpp: the NORMAL-school gate into
// Unit::UpdateAllDamageDoneMods → Unit::UpdateDamageDoneMods,
// Unit.cpp:9334-9360): the summed amounts of SPELL_AURA_MOD_DAMAGE_DONE (13)
// effects whose misc value carries the NORMAL school bit.
// Unit::CheckAttackFitToAuraRequirement (Unit.h:1416) returns true for
// non-players, so creatures skip the weapon-fit gate C++-exactly; the player
// twin meleeDamageDoneFlat (aura_damage.go) applies it via
// attackFitsAuraRequirement. Only the flat arm belongs here: the
// percent arm is the separate HandleModDamagePercentDone (79) handler.
func (s *Server) creatureAuraDamageDoneFlat(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 0
	}
	var sum int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModDamageDone || effect.MiscValue&1 == 0 {
				continue
			}
			if index < len(aura.Amounts) {
				sum += aura.Amounts[index]
			}
		}
	}
	s.auraMu.Unlock()
	return sum
}

// creatureAuraDamageDonePct mirrors the creature half of the percent arm
// AuraEffect::HandleModDamagePercentDone calls on every target type
// (SpellAuraEffects.cpp:4176-4199): the product of (1 + amount/100) over
// SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79) effects whose misc value carries
// the NORMAL school bit. Creatures attack with BASE_ATTACK, so only the 79
// arm applies here — the 122 off-hand arm of
// Unit::UpdateDamagePctDoneMods (Unit.cpp:9412-9413) has no creature model,
// mirroring the player twin meleeDamageDonePctMod (aura_damage.go).
func (s *Server) creatureAuraDamageDonePct(key creatureAuraKey) float64 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 1.0
	}
	mod := 1.0
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != auraModDamagePercentDone || effect.MiscValue&1 == 0 {
				continue
			}
			if index < len(aura.Amounts) {
				mod *= 1 + float64(aura.Amounts[index])/100
			}
		}
	}
	s.auraMu.Unlock()
	return mod
}

// petBaseArmorBonus sums the live flat TOTAL_VALUE armor legs for a pet
// creature, mirroring Guardian::UpdateArmor's GetFlatModifierValue
// (UNIT_MOD_ARMOR, TOTAL_VALUE) (StatSystem.cpp:1326): the
// SPELL_AURA_MOD_BASE_RESISTANCE (83) pets-only arm with the NORMAL school
// misc bit (SpellAuraEffects.cpp:3340-3350) plus the
// SPELL_AURA_MOD_RESISTANCE (22) NORMAL-school leg, which the school loop
// folds into UNIT_MOD_ARMOR (UNIT_MOD_RESISTANCE_START+0,
// SpellAuraEffects.cpp:3255-3269). The 22/83 arms for players are bridged
// elsewhere; non-pet creatures get nothing (C++ gates on IsPet()).
func (s *Server) petBaseArmorBonus(key creatureAuraKey, isPet bool) float64 {
	if s == nil || key.GUID == 0 || !isPet || s.Data == nil {
		return 0
	}
	bonus := 0.0
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.MiscValue&1 == 0 {
				continue
			}
			switch effect.Aura {
			case spellAuraModBaseResistance, spellAuraModResistance:
			default:
				continue
			}
			if index < len(aura.Amounts) {
				bonus += float64(aura.Amounts[index])
			}
		}
	}
	s.auraMu.Unlock()
	return bonus
}

// petOwnerArmorValue mirrors the IsPet() arm of Guardian::UpdateArmor
// (StatSystem.cpp:1323-1324): hunter and warlock pets gain 35% of the
// owner's armor, added after the agility leg and before the TOTAL_PCT fold.
// Go's character pets (summon/hunter) are the Pet twin; guardians keep 0.
func petOwnerArmorValue(petType uint8, ownerArmor uint32) float64 {
	if petType != petTypeHunter && petType != petTypeSummon {
		return 0
	}
	return float64(ownerArmor) * 0.35
}

// petOwnerArmorBonus resolves the 35% owner-armor leg of
// Guardian::UpdateArmor (StatSystem.cpp:1323-1324). C++ recomputes it from
// the owner's live armor whenever the owner's armor changes
// (Player::UpdateArmor calls pet->UpdateArmor(), StatSystem.cpp:283), so a
// connected owner's live armor is the C++-exact source; the
// stat-compute-point snapshot on the motion covers offline owners (the
// established pass-through convention).
func (s *Server) petOwnerArmorBonus(motion *creatureMotion) float64 {
	if s == nil || motion == nil || motion.OwnerGUID == 0 {
		return 0
	}
	if sess := s.playerSessionForGUID(motion.OwnerGUID); sess != nil && sess.player != nil {
		return petOwnerArmorValue(motion.PetType, sess.player.Armor)
	}
	return motion.PetOwnerArmorBonus
}

// petFullArmor computes the live Guardian::UpdateArmor value
// (StatSystem.cpp:1316-1330) for a pet motion from the stored aura-free flat
// BASE_VALUE (motion.Armor): the BASE_PCT fold (aura-142) multiplies ONLY the
// flat base — C++ adds agility*2, the TOTAL_VALUE flats (aura-22/83) and the
// 35% owner bonus AFTER, then multiplies TOTAL_PCT (aura-101). The previous
// combat-time fold applied basePct over the whole stored value (which
// carried agility*2 and the owner bonus), wrongly scaling those legs. The
// agility leg reads motion.Stats[1] (the aura stat-mod feed of GetStat stays
// unbridged); non-pet motions get the stored value unchanged.
func (s *Server) petFullArmor(key creatureAuraKey, motion *creatureMotion) float64 {
	if s == nil || motion == nil {
		return 0
	}
	if motion.OwnerGUID == 0 {
		return float64(motion.Armor)
	}
	basePct, totalPct := s.petArmorPctBonus(key, true)
	armor := float64(motion.Armor) * basePct
	var agility uint32
	if len(motion.Stats) > 1 {
		agility = motion.Stats[1]
	}
	armor += float64(agility) * 2
	armor += s.petBaseArmorBonus(key, true) + s.petOwnerArmorBonus(motion)
	armor *= totalPct
	if armor < 0 {
		armor = 0
	}
	return armor
}

// petResistanceBonus sums the live SPELL_AURA_MOD_RESISTANCE (22) amounts
// per school for a pet creature, mirroring the pet arm of
// AuraEffect::HandleAuraModResistance (SpellAuraEffects.cpp:3255-3271):
// pets (only) gain flat UNIT_MOD_RESISTANCE TOTAL_VALUE per school from
// effects whose misc bit covers that school (1<<x). Negative school totals
// clamp at 0, matching the armor fold's convention. Non-pet creatures get
// nothing (C++ gates on IsPet()).
func (s *Server) petResistanceBonus(key creatureAuraKey, isPet bool) [schoolCount]int32 {
	var bonus [schoolCount]int32
	if s == nil || key.GUID == 0 || !isPet || s.Data == nil {
		return bonus
	}
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModResistance {
				continue
			}
			if index < len(aura.Amounts) {
				for x := 0; x < schoolCount; x++ {
					if effect.MiscValue&(1<<x) != 0 {
						bonus[x] += aura.Amounts[index]
					}
				}
			}
		}
	}
	s.auraMu.Unlock()
	for x := 0; x < schoolCount; x++ {
		if bonus[x] < 0 {
			bonus[x] = 0
		}
	}
	return bonus
}

// petFoldedResistances returns base with the live 22 pet-resistance fold
// applied when the key's creature is a pet (its motion carries an owner);
// non-pets and unknown motions get base unchanged.
func (s *Server) petFoldedResistances(key creatureAuraKey, base [schoolCount]uint32) [schoolCount]uint32 {
	if s == nil || key.GUID == 0 {
		return base
	}
	tm := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if tm == nil || tm.OwnerGUID == 0 {
		return base
	}
	bonus := s.petResistanceBonus(key, true)
	out := base
	for x := range out {
		out[x] += uint32(bonus[x])
	}
	return out
}

// petArmorPctBonus mirrors the pet arms of
// AuraEffect::HandleAuraModBaseResistancePCT (142,
// SpellAuraEffects.cpp:3343-3351) and AuraEffect::HandleModResistancePercent
// (101, SpellAuraEffects.cpp:3313-3331): pets (only) multiply UNIT_MOD_ARMOR
// BASE_PCT by the product of (1 + amount/100) over live 142 effects carrying
// the NORMAL school misc bit (GetTotalAuraMultiplierByMiscMask,
// Unit.cpp:4947), and UNIT_MOD_ARMOR TOTAL_PCT by the product over live 101
// effects carrying the NORMAL bit (UNIT_MOD_RESISTANCE_START+0 is
// UNIT_MOD_ARMOR). Returns (basePct, totalPct) as multipliers; the pet armor
// composition lives in petFullArmor, which applies basePct to the flat base
// only per Guardian::UpdateArmor (StatSystem.cpp:1316-1330). Like
// petBaseArmorBonus, non-pet creatures get nothing (C++ gates both on
// IsPet()).
func (s *Server) petArmorPctBonus(key creatureAuraKey, isPet bool) (float64, float64) {
	if s == nil || key.GUID == 0 || !isPet || s.Data == nil {
		return 1.0, 1.0
	}
	basePct, totalPct := 1.0, 1.0
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.MiscValue&1 == 0 || index >= len(aura.Amounts) {
				continue
			}
			switch effect.Aura {
			case spellAuraModBaseResistancePct:
				basePct *= 1 + float64(aura.Amounts[index])/100
			case spellAuraModResistancePct:
				totalPct *= 1 + float64(aura.Amounts[index])/100
			}
		}
	}
	s.auraMu.Unlock()
	return basePct, totalPct
}

// creatureAuraSpellHitSum mirrors the creature half of
// AuraEffect::HandleModSpellHitChance (SpellAuraEffects.cpp:3839): the summed
// creatureAuraTypeSum folds the live amounts of one aura type on the
// creature's active auras, per effect via the DBC spell (no misc gate),
// mirroring the creatureAuraSpellHitSum pattern. It serves the
// Unit::MeleeSpellMissChance victim arm (Unit.cpp:12425):
// SPELL_AURA_MOD_ATTACKER_MELEE_HIT_CHANCE (184) and
// SPELL_AURA_MOD_ATTACKER_RANGED_HIT_CHANCE (185) are summed on the victim.
func (s *Server) creatureAuraTypeSum(key creatureAuraKey, auraType uint32) int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 0
	}
	var sum int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != auraType {
				continue
			}
			if index < len(aura.Amounts) {
				sum += aura.Amounts[index]
			}
		}
	}
	s.auraMu.Unlock()
	return sum
}

// creatureAuraTypeSumByMisc mirrors Unit::GetTotalAuraModifierByMiscValue
// (Unit.cpp:4977-4985) for creature victims: the sum of live amounts of one
// aura type whose effect MiscValue equals miscValue. It serves the
// SPELL_AURA_MOD_DEBUFF_RESISTANCE (178) arm of
// WorldObject::MagicSpellHitResult (Object.cpp:2585-2592), where the sum is
// gated on the spell's DispelType.
func (s *Server) creatureAuraTypeSumByMisc(key creatureAuraKey, auraType uint32, miscValue int32) int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 0
	}
	var sum int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != auraType || effect.MiscValue != miscValue {
				continue
			}
			if index < len(aura.Amounts) {
				sum += aura.Amounts[index]
			}
		}
	}
	s.auraMu.Unlock()
	return sum
}

// amounts of SPELL_AURA_MOD_SPELL_HIT_CHANCE (55) effects on the creature's
// live auras. C++ accumulates the amounts straight into m_modSpellHitChance
// (no misc/school gate), and WorldObject::MagicSpellHitResult
// (Object.cpp:2555) consumes that field for any unit caster, so creature
// casters fold it into the spell-hit roll in percent units.
func (s *Server) creatureAuraSpellHitSum(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 0
	}
	var sum int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModSpellHitChance {
				continue
			}
			if index < len(aura.Amounts) {
				sum += aura.Amounts[index]
			}
		}
	}
	s.auraMu.Unlock()
	return sum
}

// creatureHasAuraType reports whether the creature carries a live (unstopped)
// aura whose stored AuraType matches, mirroring Unit::HasAuraType for the
// byte-flag folds in the creature update builder (the C++ remove legs gate
// on HasAuraType; a live fold over the same set is the equivalent check).
func (s *Server) creatureHasAuraType(key creatureAuraKey, auraType uint32) bool {
	if s == nil || key.GUID == 0 {
		return false
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if aura.AuraType == auraType {
			return true
		}
	}
	return false
}
