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
