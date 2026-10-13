package world

import (
	"math"
	"sort"
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

// motionIsPet mirrors Unit::IsPet (Unit.h:839): (m_unitTypeMask & UNIT_MASK_PET)
// != 0, set only for Pet objects (Pet.cpp:49). In Go only registerPetMotion
// sets motion.PetID, so PetID != 0 is the twin — charmed creatures and any
// guardian/temp-summon motions carry an owner but are not pets, and must not
// receive the IsPet-gated arms (aura-142 BASE_PCT, aura-83 TOTAL_VALUE, the
// 35% owner-armor bonus).
func motionIsPet(motion *creatureMotion) bool {
	return motion != nil && motion.PetID != 0
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
// Guardian::UpdateArmor (StatSystem.cpp:1323-1324), gated on Unit::IsPet
// (Unit.h:839) via motionIsPet: charmed creatures carry an owner but are
// Creature objects, not Pets, so they get nothing here — a charmed motion's
// zero-value PetType previously read as petTypeSummon (0) and leaked the
// bonus whenever the charmer was connected. C++ recomputes it from
// the owner's live armor whenever the owner's armor changes
// (Player::UpdateArmor calls pet->UpdateArmor(), StatSystem.cpp:283), so a
// connected owner's live armor is the C++-exact source; the
// stat-compute-point snapshot on the motion covers offline owners (the
// established pass-through convention).
func (s *Server) petOwnerArmorBonus(motion *creatureMotion) float64 {
	if s == nil || motion == nil || motion.OwnerGUID == 0 || !motionIsPet(motion) {
		return 0
	}
	if sess := s.playerSessionForGUID(motion.OwnerGUID); sess != nil && sess.player != nil {
		return petOwnerArmorValue(motion.PetType, sess.player.Armor)
	}
	return motion.PetOwnerArmorBonus
}

// statAgilityIndex is STAT_AGILITY (SharedDefines.h: the Stats enum runs
// 0 strength, 1 agility, 2 stamina, 3 intellect, 4 spirit).
const statAgilityIndex = 1

// petGuardianStat mirrors the stat-aura feed of Unit::GetStat for a guardian
// (StatSystem.cpp:1316-1330, 1337-1379): GetTotalStatValue(stat) —
// ((BASE_VALUE + createStat) * BASE_PCT + aura-29 flat) * aura-137 pct —
// before the owner-bonus arms, which Go folds separately. The stat BASE_VALUE
// has no feeder (Unit.cpp) and stays 0; aura-80 (SPELL_AURA_MOD_PERCENT_STAT)
// returns early for non-players (SpellAuraEffects.cpp:3427), so the stat
// BASE_PCT stays 1.0 for guardians. The aura-29 (SPELL_AURA_MOD_STAT,
// SpellAuraEffects.cpp:3379-3413) flat leg sums the live amounts covering the
// stat (misc < 0 or == stat; other misc values are rejected like the C++
// log-and-return) under the exclusive-same-effect winner gate, resolved per
// misc value exactly like Unit::GetHighestExclusiveSameEffectSpellGroupValue
// (Unit.cpp:13967) with checkMiscValue=true: a candidate is suppressed only
// by a peer with the same misc value sharing an exclusive group and a
// strictly larger |amount| (ties to the earliest-applied record, matching
// the C++ apply leg's first-wins). The aura-137
// (SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE, SpellAuraEffects.cpp:3525-3566) pct
// leg multiplies (1 + amount/100) over the live effects with misc == stat or
// == -1 — misc -2 is NOT all-stats for 137, unlike 29 — ungated, matching
// the handler's recompute-from-live semantics (the apply-time no-stack purge
// is shared with the other creature folds).
func (s *Server) petGuardianStat(key creatureAuraKey, motion *creatureMotion, stat int) float64 {
	var base float64
	if motion != nil && stat >= 0 && stat < len(motion.Stats) {
		base = float64(motion.Stats[stat])
	}
	if s == nil || s.Data == nil || key.GUID == 0 || stat < 0 || stat > 4 {
		return base
	}
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	s.auraMu.Unlock()
	// Slot-sorted apply order, matching the other creature folds, so
	// same-effect tie-breaking is deterministic.
	sort.Slice(copies, func(i, j int) bool {
		if copies[i] == nil || copies[j] == nil {
			return copies[j] == nil
		}
		return copies[i].Slot < copies[j].Slot
	})
	type stat29Candidate struct {
		order       int
		spellID     uint32
		effectIndex int
		misc        int32
		amount      int32
		firstRank   uint32
	}
	var cands []stat29Candidate
	totalPct := 1.0
	for order, aura := range copies {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		usedStoredAmount := false
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == effect.Aura && !usedStoredAmount {
				amount = int32(aura.Amount)
				usedStoredAmount = true
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			switch effect.Aura {
			case spellAuraModStat:
				if effect.MiscValue < -2 || effect.MiscValue > 4 {
					continue
				}
				if effect.MiscValue < 0 || int(effect.MiscValue) == stat {
					cands = append(cands, stat29Candidate{order: order, spellID: aura.SpellID, effectIndex: index, misc: effect.MiscValue, amount: amount, firstRank: s.spellFirstRank(aura.SpellID)})
				}
			case spellAuraModTotalStatPct:
				if effect.MiscValue < -1 || effect.MiscValue > 4 {
					continue
				}
				if effect.MiscValue == -1 || int(effect.MiscValue) == stat {
					totalPct *= 1 + float64(amount)/100
				}
			}
		}
	}
	cache := s.petAuraStackGroups()
	var flat int64
	for _, c := range cands {
		suppressed := false
		for _, p := range cands {
			if p.order == c.order && p.spellID == c.spellID && p.effectIndex == c.effectIndex {
				continue
			}
			if p.misc != c.misc {
				continue
			}
			if !sharesExclusiveSameEffectGroup(c.firstRank, p.firstRank, cache) {
				continue
			}
			pa, ca := absAuraAmount(p.amount), absAuraAmount(c.amount)
			if pa > ca || (pa == ca && p.order < c.order) {
				suppressed = true
				break
			}
		}
		if !suppressed {
			flat += int64(c.amount)
		}
	}
	return (base + float64(flat)) * totalPct
}

// statIntellectIndex is STAT_INTELLECT (SharedDefines.h: the Stats enum runs
// 0 strength, 1 agility, 2 stamina, 3 intellect, 4 spirit).
const statIntellectIndex = 3

// petManaMultiplicator mirrors the entry switch in Guardian::UpdateMaxPower
// (StatSystem.cpp:1359-1379).
func petManaMultiplicator(entry uint32) float64 {
	switch entry {
	case 416: // ENTRY_IMP
		return 4.95
	case 1860, 1863, 417, 17252: // ENTRY_VOIDWALKER, ENTRY_SUCCUBUS, ENTRY_FELHUNTER, ENTRY_FELGUARD
		return 11.5
	default:
		return 15.0
	}
}

// petStaminaAuraHealthDelta mirrors the stamina arm of
// Guardian::UpdateMaxHealth (StatSystem.cpp:1336-1357):
// (GetStat(STAT_STAMINA) - GetCreateStat(STAT_STAMINA)) * multiplicator.
// GetStat carries the live aura-29/137 feed (petGuardianStat) plus the owner
// bonus; the owner bonus already lives in the stored max through
// petOwnerStaminaHealthBonus, so this returns only the aura-feed part:
// (auraFed - createStamina) * petHealthMultiplicator(entry). C++ recomputes
// it on every UpdateStats — including aura apply/remove — not just at the
// pet stat-compute points.
func (s *Server) petStaminaAuraHealthDelta(key creatureAuraKey, motion *creatureMotion, entry uint32) float64 {
	if s == nil || motion == nil || len(motion.Stats) <= statStaminaIndex {
		return 0
	}
	fed := s.petGuardianStat(key, motion, statStaminaIndex)
	return (fed - float64(motion.Stats[statStaminaIndex])) * petHealthMultiplicator(entry)
}

// petIntellectAuraManaDelta mirrors the intellect arm of
// Guardian::UpdateMaxPower (StatSystem.cpp:1359-1379):
// (GetStat(STAT_INTELLECT) - GetCreateStat(STAT_INTELLECT)) * multiplicator.
// Unlike the owner-intellect bonus (warlock/mage pets only,
// StatSystem.cpp:1251-1257), the (GetStat - createStat) arm applies to every
// guardian — a hunter pet carrying an intellect aura gains max mana in C++.
func (s *Server) petIntellectAuraManaDelta(key creatureAuraKey, motion *creatureMotion, entry uint32) float64 {
	if s == nil || motion == nil || len(motion.Stats) <= statIntellectIndex {
		return 0
	}
	fed := s.petGuardianStat(key, motion, statIntellectIndex)
	return (fed - float64(motion.Stats[statIntellectIndex])) * petManaMultiplicator(entry)
}

// creatureHealthAuraMods folds the live UNIT_MOD_HEALTH modifier auras on a
// creature motion (SpellAuraEffects.cpp:3650-3768): 34/230/250 flat amounts
// sum into TOTAL_VALUE (HandleAuraModIncreaseHealth feeds
// HandleStatFlatModifier, whose flat group sums like the 22/83 legs),
// 282 multiplies BASE_PCT and 133 multiplies TOTAL_PCT (both
// ApplyStatPctModifier product folds; the remove leg rebuilds the product
// from the remaining auras, SpellAuraEffects.cpp:3742). Amounts resolve per
// effect like the other creature folds (stored amount, then aura record,
// then DBC base points).
func (s *Server) creatureHealthAuraMods(key creatureAuraKey) (flat int32, basePct, totalPct float64) {
	basePct, totalPct = 1, 1
	if s == nil || s.Data == nil || key.GUID == 0 {
		return 0, 1, 1
	}
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	s.auraMu.Unlock()
	for _, aura := range copies {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		usedStoredAmount := false
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == effect.Aura && !usedStoredAmount {
				amount = int32(aura.Amount)
				usedStoredAmount = true
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			switch effect.Aura {
			case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2:
				flat += amount
			case spellAuraModBaseHealthPct:
				basePct *= 1 + float64(amount)/100
			case spellAuraModIncreaseHealthPercent:
				totalPct *= 1 + float64(amount)/100
			}
		}
	}
	return flat, basePct, totalPct
}

// creaturePowerAuraMods is the UNIT_MOD_POWER_START+power twin of
// creatureHealthAuraMods (SpellAuraEffects.cpp:3686-3727): 35 flat amounts
// sum into TOTAL_VALUE (misc-gated to the power) and 132 multiplies
// TOTAL_PCT (ApplyStatPctModifier product fold). No BASE feeders exist for
// the power modifiers in C++.
func (s *Server) creaturePowerAuraMods(key creatureAuraKey, power uint32) (flat int32, totalPct float64) {
	totalPct = 1
	if s == nil || s.Data == nil || key.GUID == 0 {
		return 0, 1
	}
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	s.auraMu.Unlock()
	for _, aura := range copies {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		usedStoredAmount := false
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.MiscValue != int32(power) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == effect.Aura && !usedStoredAmount {
				amount = int32(aura.Amount)
				usedStoredAmount = true
			}
			if amount == 0 {
				amount = effect.BasePoints
			}
			switch effect.Aura {
			case spellAuraModIncreaseEnergy:
				flat += amount
			case spellAuraModIncreaseEnergyPercent:
				totalPct *= 1 + float64(amount)/100
			}
		}
	}
	return flat, totalPct
}

// recomputeCreatureMaxHealthPower folds the live UNIT_MOD_HEALTH /
// UNIT_MOD_POWER modifier auras over the banked aura-free base — the
// Creature::UpdateMaxHealth / Creature::UpdateMaxPower twins
// (StatSystem.cpp:1024-1041): ((base*basePct282)+flat34/230/250)*totalPct133
// for health (GetTotalAuraModValue composition, Unit.cpp:9437-9448),
// (base+flat35)*totalPct132 for power 0. Creatures have no stamina/intellect
// arm. A zero bank falls back to the stored max (callers bank the fallback
// before writing when the motion predates the bank fields). Callers must not
// hold auraMu or motionMu.
func (s *Server) recomputeCreatureMaxHealthPower(key creatureAuraKey, motion *creatureMotion) (newMaxHealth, newMaxPower0 uint32) {
	if motion == nil {
		return 0, 0
	}
	base := motion.BaseMaxHealth
	if base == 0 {
		base = motion.MaxHealth
	}
	flat, basePct, totalPct := s.creatureHealthAuraMods(key)
	v := (float64(base)*basePct + float64(flat)) * totalPct
	if v < 1 {
		v = 1
	}
	newMaxHealth = uint32(v)
	powerBase := motion.BaseMaxPowers[0]
	if powerBase == 0 {
		powerBase = motion.MaxPowers[0]
	}
	pflat, ptotal := s.creaturePowerAuraMods(key, 0)
	pv := (float64(powerBase) + float64(pflat)) * ptotal
	if pv < 0 {
		pv = 0
	}
	newMaxPower0 = uint32(pv)
	return newMaxHealth, newMaxPower0
}

// recomputeCreatureHealthPowerOnAuraChange is the non-pet twin of
// recomputePetDerivedOnStatAuraChange: Creature::UpdateMaxHealth /
// UpdateMaxPower (StatSystem.cpp:1024-1041) refold the live 34/230/250/133/
// 282 and 35/132 auras over the banked aura-free base on every aura
// apply/remove (SpellAuraEffects.cpp:3650-3768) — previously Go tracked no
// creature aura health deltas at all. Creatures have no stamina/intellect
// arm and no owner session. Current health follows the per-spell halves
// (34 heal/drop, 133 pct-keep); a pure max change leaves current health
// alone like C++ (no clamp). Callers must not hold auraMu or motionMu.
func (s *Server) recomputeCreatureHealthPowerOnAuraChange(key creatureAuraKey, motion *creatureMotion, halves auraHealthPowerHalves, apply bool) {
	if s == nil || motion == nil || key.GUID == 0 {
		return
	}
	// Folds read under auraMu only — never nested inside motionMu.
	healthFlat, healthBasePct, healthTotalPct := s.creatureHealthAuraMods(key)
	powerFlat, powerTotalPct := s.creaturePowerAuraMods(key, 0)
	s.motionMu.Lock()
	m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if m == nil || m != motion {
		s.motionMu.Unlock()
		return
	}
	if m.BaseMaxHealth == 0 {
		// Motions that predate the bank fields never had a fold applied,
		// so the stored max is the aura-free base.
		m.BaseMaxHealth = m.MaxHealth
	}
	if m.BaseMaxPowers[0] == 0 {
		m.BaseMaxPowers[0] = m.MaxPowers[0]
	}
	curHealth, curPower := m.Health, m.Powers[0]
	oldMaxHealth, oldMaxPower := m.MaxHealth, m.MaxPowers[0]
	var healthPct float64
	if oldMaxHealth > 0 && curHealth > 0 {
		healthPct = float64(curHealth) / float64(oldMaxHealth)
	}
	zeroHealth := curHealth == 0
	v := (float64(m.BaseMaxHealth)*healthBasePct + float64(healthFlat)) * healthTotalPct
	if v < 1 {
		v = 1
	}
	newMaxHealth := uint32(v)
	pv := (float64(m.BaseMaxPowers[0]) + float64(powerFlat)) * powerTotalPct
	if pv < 0 {
		pv = 0
	}
	newMaxPower0 := uint32(pv)
	newHealth := curHealth
	switch {
	case halves.hasHealthPct:
		newHealth = uint32(healthPct * float64(newMaxHealth))
		if !zeroHealth && newHealth < 1 {
			newHealth = 1
		}
	case halves.flatHealth != 0 && curHealth > 0:
		// HandleAuraModIncreaseHealth (SpellAuraEffects.cpp:3650-3665):
		// the flat mod lands first, then current health heals by the
		// amount on apply (capped at the new max) or drops by the amount
		// floored at 1 on remove; a negative amount inverts the halves.
		// ModifyHealth no-ops on a dead unit, so a dead creature keeps 0.
		if apply {
			if halves.flatHealth < 0 {
				if drop := uint32(-halves.flatHealth); drop >= newHealth {
					newHealth = 1
				} else {
					newHealth -= drop
				}
			} else if nh := newHealth + uint32(halves.flatHealth); nh > newMaxHealth {
				newHealth = newMaxHealth
			} else {
				newHealth = nh
			}
		} else if halves.flatHealth > 0 {
			if drop := uint32(halves.flatHealth); drop >= newHealth {
				newHealth = 1
			} else {
				newHealth -= drop
			}
		} else if nh := newHealth + uint32(-halves.flatHealth); nh > newMaxHealth {
			newHealth = newMaxHealth
		} else {
			newHealth = nh
		}
	}
	newPower := curPower
	if halves.hasPowerPct {
		// HandleAuraModIncreaseEnergyPercent (SpellAuraEffects.cpp:3698-3727):
		// current power shifts by the max-power change, on apply and
		// remove alike, clamped into [0, new max] like ModifyPower.
		if np := int64(curPower) + int64(newMaxPower0) - int64(oldMaxPower); np < 0 {
			newPower = 0
		} else if np > int64(newMaxPower0) {
			newPower = newMaxPower0
		} else {
			newPower = uint32(np)
		}
	} else if newPower > newMaxPower0 {
		newPower = newMaxPower0
	}
	m.Health, m.MaxHealth = newHealth, newMaxHealth
	m.Mana, m.MaxMana = newPower, newMaxPower0
	m.Powers[0], m.MaxPowers[0] = newPower, newMaxPower0
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{
		unitFieldHealth:    newHealth,
		unitFieldMaxHealth: newMaxHealth,
		unitFieldPower1:    newPower,
		unitFieldMaxPower1: newMaxPower0,
	})
}

// petFullArmor computes the live Guardian::UpdateArmor value
// (StatSystem.cpp:1316-1330) for a pet motion (callers must gate on
// motionIsPet; the IsPet-gated arms — aura-142 BASE_PCT, aura-83 TOTAL_VALUE,
// the 35% owner bonus — apply only to Pets, Unit.h:839) from the stored
// aura-free flat BASE_VALUE (motion.Armor): the BASE_PCT fold (aura-142)
// multiplies ONLY the flat base — C++ adds agility*2, the TOTAL_VALUE flats
// (aura-22/83) and the 35% owner bonus AFTER, then multiplies TOTAL_PCT
// (aura-101). The previous combat-time fold applied basePct over the whole
// stored value (which carried agility*2 and the owner bonus), wrongly scaling
// those legs. The agility leg is the C++-exact GetStat(STAT_AGILITY) via
// petGuardianStat (create-stat base plus the live aura-29 flat / aura-137 pct
// feeds); non-pet motions must not reach this function — they take
// creatureArmorFold (Creature::UpdateArmor) via liveCreatureArmor.
func (s *Server) petFullArmor(key creatureAuraKey, motion *creatureMotion) float64 {
	if s == nil || motion == nil {
		return 0
	}
	if motion.OwnerGUID == 0 {
		return float64(motion.Armor)
	}
	basePct, totalPct := s.petArmorPctBonus(key, true)
	// C++: flat(BASE_VALUE)*pct(BASE_PCT) + agility*2 + flat(TOTAL_VALUE) +
	// owner bonus, x pct(TOTAL_PCT) (StatSystem.cpp:1316-1330). The flat base
	// is the DB base (SetStatFlatModifier(BASE_VALUE), Pet.cpp:906) plus the
	// ungated aura-143 exclusive winner (SpellAuraEffects.cpp:3232-3252),
	// beside the IsPet-gated 142/83 arms.
	armor := (float64(motion.Armor) + s.exclusiveResistanceArmorBase(key)) * basePct
	armor += s.petGuardianStat(key, motion, statAgilityIndex) * 2
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

// petFoldedResistances returns base with the live resistance fold applied
// for magic-resistance mitigation at damage time. The pet path mirrors
// Guardian::UpdateResistances (StatSystem.cpp:1301-1314): GetTotalAuraModValue
// per school — the ungated aura-143 exclusive winner and aura-22 flat sum
// into the flat leg, the aura-101 product multiplies, all for schools 1-6 —
// plus the IsPet-gated 40% of the owner's live resistance
// (hunter/warlock pets only; offline owners resolve to no bonus).
// School 0 (armor) keeps the old 22-only fold here — the armor composition
// lives in liveCreatureArmor/petFullArmor, not here.
// The non-pet path was gated on OwnerGUID != 0 — but the C++ handlers are
// ungated: HandleAuraModResistance (SpellAuraEffects.cpp:3255-3269), the
// aura-143 exclusive arm (SpellAuraEffects.cpp:3232-3252) and
// HandleModResistancePercent (SpellAuraEffects.cpp:3313-3331) all fold into
// UNIT_MOD_RESISTANCE for every unit, so regular creature victims get
// Creature::UpdateResistances (StatSystem.cpp:1007-1014): (DB base + 143
// winner + 22 flat) x 101 product per school. School 0 (armor) routes to
// UpdateArmor in C++ and is folded by liveCreatureArmor, so it stays out
// here; aura-83 is IsPet-gated (:3344) and stays out for non-pet creatures.
func (s *Server) petFoldedResistances(key creatureAuraKey, base [schoolCount]uint32) [schoolCount]uint32 {
	if s == nil || key.GUID == 0 {
		return base
	}
	tm := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if tm == nil || motionIsPet(tm) {
		if tm == nil {
			return base
		}
		bonus := s.petResistanceBonus(key, true)
		out := base
		// The aura-143 exclusive winner and the aura-101 pct product are
		// ungated (SpellAuraEffects.cpp:3232-3252, :3313-3331), so the pet
		// schools fold them exactly like Creature::UpdateResistances:
		// (base + winner + flat) x totalPct (GetTotalAuraModValue,
		// Unit.cpp:9437-9448).
		var exclusive [schoolCount]int64
		totalPct := [schoolCount]float64{1, 1, 1, 1, 1, 1, 1}
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
				if aura.EffectMask&(1<<uint(index)) == 0 || index >= len(aura.Amounts) {
					continue
				}
				for x := 1; x < schoolCount; x++ {
					if effect.MiscValue&(1<<uint(x)) == 0 {
						continue
					}
					switch effect.Aura {
					case spellAuraModResistanceExclusive:
						if a := int64(aura.Amounts[index]); a > exclusive[x] {
							exclusive[x] = a
						}
					case spellAuraModResistancePct:
						totalPct[x] *= 1 + float64(aura.Amounts[index])/100
					}
				}
			}
		}
		s.auraMu.Unlock()
		// School 0 (armor) keeps the old 22-only fold — the armor composition
		// lives in liveCreatureArmor/petFullArmor, not here.
		out[0] += uint32(bonus[0])
		// Guardian::UpdateResistances (StatSystem.cpp:1301-1314): hunter and
		// warlock pets add 40% of the owner's live resistance on top of the
		// folded value; school <= NORMAL routes to UpdateArmor in C++.
		var ownerRes40 [schoolCount]float64
		if tm.PetType == petTypeHunter || tm.PetType == petTypeSummon {
			if sess := s.playerSessionForGUID(tm.OwnerGUID); sess != nil && sess.player != nil {
				for x := 1; x < schoolCount; x++ {
					ownerRes40[x] = float64(sess.player.Resistances[x]) * 0.4
				}
			}
		}
		for x := 1; x < schoolCount; x++ {
			v := (float64(base[x]) + float64(exclusive[x]) + float64(bonus[x])) * totalPct[x]
			v += ownerRes40[x]
			if v < 0 {
				v = 0
			}
			out[x] = uint32(v)
		}
		return out
	}
	var flat [schoolCount]int64
	var exclusive [schoolCount]int64
	totalPct := [schoolCount]float64{1, 1, 1, 1, 1, 1, 1}
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
			if aura.EffectMask&(1<<uint(index)) == 0 || index >= len(aura.Amounts) {
				continue
			}
			amount := aura.Amounts[index]
			for x := 1; x < schoolCount; x++ {
				if effect.MiscValue&(1<<uint(x)) == 0 {
					continue
				}
				switch effect.Aura {
				case spellAuraModResistance:
					flat[x] += int64(amount)
				case spellAuraModResistanceExclusive:
					if a := int64(amount); a > exclusive[x] {
						exclusive[x] = a
					}
				case spellAuraModResistancePct:
					totalPct[x] *= 1 + float64(amount)/100
				}
			}
		}
	}
	s.auraMu.Unlock()
	out := base
	for x := 1; x < schoolCount; x++ {
		v := (float64(base[x]) + float64(exclusive[x]) + float64(flat[x])) * totalPct[x]
		if v < 0 {
			v = 0
		}
		out[x] = uint32(v)
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

// exclusiveResistanceArmorBase mirrors the armor (misc NORMAL) leg of
// AuraEffect::HandleAuraModResistanceExclusive (SpellAuraEffects.cpp:3232-3252,
// aura 143): ungated — every creature target, pets included, adds the max
// positive exclusive amount into UNIT_MOD_ARMOR BASE_VALUE
// (UNIT_MOD_RESISTANCE_START+0 == UNIT_MOD_ARMOR, Unit.h:178). The C++
// apply/remove legs net to the max positive winner
// (GetMaxPositiveAuraModifierByMiscMask + the amount< GetAmount() guard), so
// the live fold takes max() directly.
func (s *Server) exclusiveResistanceArmorBase(key creatureAuraKey) float64 {
	if s == nil || s.Data == nil || key.GUID == 0 {
		return 0
	}
	var winner int64
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
			if effect.Aura != spellAuraModResistanceExclusive ||
				aura.EffectMask&(1<<uint(index)) == 0 || effect.MiscValue&1 == 0 ||
				index >= len(aura.Amounts) {
				continue
			}
			if amt := int64(aura.Amounts[index]); amt > winner {
				winner = amt
			}
		}
	}
	s.auraMu.Unlock()
	if winner < 0 {
		return 0
	}
	return float64(winner)
}

// creatureArmorFold mirrors Creature::UpdateArmor (StatSystem.cpp:1018-1022)
// for non-pet creatures: GetTotalAuraModValue(UNIT_MOD_ARMOR) — the stored
// template armor (BASE_VALUE, Creature.cpp:1553) times BASE_PCT, plus the
// flat TOTAL_VALUE, times TOTAL_PCT. The only BASE_PCT feeder is the
// IsPet-gated 142 arm (SpellAuraEffects.cpp:3287), so it stays 1.0 here; the
// flat leg is the ungated aura-22 (SPELL_AURA_MOD_RESISTANCE, misc NORMAL)
// sum — the 22 handler has no IsPet gate (SpellAuraEffects.cpp:3255-3269),
// so Sunder Armor / Faerie Fire land on regular creatures too — plus the
// ungated aura-143 (SPELL_AURA_MOD_RESISTANCE_EXCLUSIVE, misc NORMAL) max
// positive winner into BASE_VALUE (SpellAuraEffects.cpp:3232-3252) — and the
// pct leg is the ungated aura-101 (SPELL_AURA_MOD_RESISTANCE_PCT, misc NORMAL)
// product (:3313-3331). Aura-83 (SPELL_AURA_MOD_BASE_RESISTANCE) is
// IsPet-gated (:3344) and stays out, like the 35% owner bonus. Negative
// totals clamp at 0, matching the pet fold's convention (C++ SetArmor takes
// the raw int32).
func (s *Server) creatureArmorFold(key creatureAuraKey, storedArmor float64) float64 {
	if s == nil || s.Data == nil || key.GUID == 0 {
		if storedArmor < 0 {
			storedArmor = 0
		}
		return storedArmor
	}
	var flat int64
	totalPct := 1.0
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
			if aura.EffectMask&(1<<uint(index)) == 0 || effect.MiscValue&1 == 0 || index >= len(aura.Amounts) {
				continue
			}
			switch effect.Aura {
			case spellAuraModResistance:
				flat += int64(aura.Amounts[index])
			case spellAuraModResistancePct:
				totalPct *= 1 + float64(aura.Amounts[index])/100
			}
		}
	}
	s.auraMu.Unlock()
	// C++: (flat(BASE)*pct(BASE_PCT) + flat(TOTAL)) * pct(TOTAL_PCT); the
	// only BASE_PCT feeder (142) is IsPet-gated so it stays 1.0 here, and the
	// 143 winner joins the BASE_VALUE leg beside the stored template base.
	armor := (storedArmor + s.exclusiveResistanceArmorBase(key) + float64(flat)) * totalPct
	if armor < 0 {
		armor = 0
	}
	return armor
}

// liveCreatureArmor dispatches the C++-exact live armor fold by victim kind:
// Guardian::UpdateArmor (StatSystem.cpp:1316-1330) for pet motions
// (motionIsPet, the Unit::IsPet twin, Unit.h:839) via petFullArmor,
// Creature::UpdateArmor (StatSystem.cpp:1018-1022) for everything else via
// creatureArmorFold — charmed creatures carry an owner but are Creature
// objects, not Pets, so they must not receive the IsPet-gated 142/83/owner
// arms that the old OwnerGUID!=0 gate applied. (A non-pet Guardian such as a
// treant would add agility*2 over the Creature fold, but Go has no
// temp-summon/guardian motions — item-9 parked — so the Creature fold is the
// live-correct branch.) C++ recomputes both on every stat-modifier change
// (Unit::UpdateUnitMod, Unit.cpp:9307-9340); Go folds live at damage/display
// time per the established convention. The stored armor is the no-aura
// fallback base, so callers no longer need to gate on OwnerGUID themselves.
func (s *Server) liveCreatureArmor(key creatureAuraKey, motion *creatureMotion, storedArmor float64) float64 {
	if motionIsPet(motion) {
		return s.petFullArmor(key, motion)
	}
	return s.creatureArmorFold(key, storedArmor)
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
