package world

import "strings"

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

// petBaseArmorBonus sums the live SPELL_AURA_MOD_BASE_RESISTANCE (83)
// amounts carrying the NORMAL school misc bit for a pet creature, mirroring
// the pet arm of AuraEffect::HandleModBaseResistance
// (SpellAuraEffects.cpp:3348-3351): pets (only) gain flat UNIT_MOD_ARMOR
// TOTAL_VALUE from those effects. The 22/83 arms for players are bridged
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
			if effect.Aura != spellAuraModBaseResistance || effect.MiscValue&1 == 0 {
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

// petArmorPctBonus mirrors the pet arms of
// AuraEffect::HandleAuraModBaseResistancePCT (142,
// SpellAuraEffects.cpp:3343-3351) and AuraEffect::HandleModResistancePercent
// (101, SpellAuraEffects.cpp:3313-3331): pets (only) multiply UNIT_MOD_ARMOR
// BASE_PCT by the product of (1 + amount/100) over live 142 effects carrying
// the NORMAL school misc bit (GetTotalAuraMultiplierByMiscMask,
// Unit.cpp:4947), and UNIT_MOD_ARMOR TOTAL_PCT by the product over live 101
// effects carrying the NORMAL bit (UNIT_MOD_RESISTANCE_START+0 is
// UNIT_MOD_ARMOR). Returns (basePct, totalPct) as multipliers; callers apply
// (baseArmor*basePct + flat83) * totalPct, matching the C++ armor composition
// (BASE_VALUE*BASE_PCT + TOTAL_VALUE)*TOTAL_PCT — the 142 arm lands on
// BASE_PCT so it multiplies the base armor only, not the 83 flat arm. Like
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
