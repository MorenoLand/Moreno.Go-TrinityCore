package world

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
// effects whose misc value carries the NORMAL school bit. The
// CheckAttackFitToAuraRequirement weapon-fit gate has no Go model (same
// standing delta as the player fold meleeDamageDoneFlat in aura_damage.go),
// so amounts sum unconditionally. Only the flat arm belongs here: the
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
