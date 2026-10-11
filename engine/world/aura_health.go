package world

// Aura-driven max-health arms: AuraEffect::HandleAuraModIncreaseHealth
// (SpellAuraEffects.cpp:3650 — aura types 34/230/250),
// HandleAuraModIncreaseMaxHealth (3667 — same handler via the 230/250 table
// rows, SpellAuraEffects.cpp:295/315), HandleAuraModIncreaseHealthPercent
// (3729 — aura 133) and HandleAuraIncreaseBaseHealthPercent (3757 — aura
// 282). C++ keeps these in Unit::m_auraModifiersGroup[UNIT_MOD_HEALTH]
// (Unit.cpp:9237-9251): flat TOTAL_VALUE adds from 34/230/250,
// BASE_PCT multiplies the base (282) and TOTAL_PCT multiplies the total
// (133), the pct legs folding multiplicatively
// (AddPct: m *= 1+pct/100; the remove leg rebuilds the product from the
// remaining auras via GetTotalAuraMultiplier, SpellAuraEffects.cpp:3742).
// Go has no UNIT_MOD group, so the live max health is recomputed from the
// active aura records on demand.

const (
	spellAuraModIncreaseHealth        = 34  // SPELL_AURA_MOD_INCREASE_HEALTH (SpellAuraDefines.h:114)
	spellAuraModIncreaseHealthOld     = 230 // second SPELL_AURA_MOD_INCREASE_HEALTH_2 row (SpellAuraEffects.cpp:295)
	spellAuraModIncreaseHealth2       = 250 // SPELL_AURA_MOD_INCREASE_HEALTH_2 (SpellAuraEffects.cpp:315)
	spellAuraModIncreaseHealthPercent = 133 // SPELL_AURA_MOD_INCREASE_HEALTH_PERCENT (SpellAuraEffects.cpp:198)
	spellAuraModBaseHealthPct         = 282 // SPELL_AURA_MOD_BASE_HEALTH_PCT (SpellAuraEffects.cpp:347)
)

// auraHealthMods folds the live health-modifier aura effects into the
// UNIT_MOD_HEALTH buckets: flat total-value sum, base-pct product and
// total-pct product. extra carries aura records that are not (or no longer)
// in the session's active map — the removed aura on the remove leg, whose
// C++ handler still sees the effect when it runs
// (Unit::_UnapplyAura erases before the handlers, Unit.cpp:3548).
func (s *session) auraHealthMods(excludeSpellID uint32, extra ...*activeAura) (flat int32, baseMult, totalMult float32) {
	baseMult, totalMult = 1, 1
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, 1, 1
	}
	auras := s.loadedAuras()
	auras = append(auras, extra...)
	for i, aura := range auras {
		if aura == nil || aura.SpellID == excludeSpellID {
			continue
		}
		// The extra records (the just-removed aura on the remove leg) are
		// already flagged Stopped by removeAura — C++ still runs their
		// handlers during _UnapplyAura (Unit.cpp:3548), so only the live
		// map records honor Stopped here.
		if i < len(auras)-len(extra) && aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			switch effect.Aura {
			case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2:
				flat += amount
			case spellAuraModBaseHealthPct:
				baseMult *= 1 + float32(amount)/100
			case spellAuraModIncreaseHealthPercent:
				totalMult *= 1 + float32(amount)/100
			}
		}
	}
	return flat, baseMult, totalMult
}

// auraTouchesHealth reports whether any of the aura's effects is a
// max-health modifier (34/230/250/133/282), mirroring the four handlers'
// table rows (SpellAuraEffects.cpp:99/198/295/315/347).
func (s *session) auraTouchesHealth(aura *activeAura) bool {
	for _, auraType := range []uint32{
		spellAuraModIncreaseHealth,
		spellAuraModIncreaseHealthOld,
		spellAuraModIncreaseHealth2,
		spellAuraModIncreaseHealthPercent,
		spellAuraModBaseHealthPct,
	} {
		if s.activeAuraHasEffect(aura, auraType) {
			return true
		}
	}
	return false
}

// effectiveMaxHealth mirrors Unit::UpdateMaxHealth for UNIT_MOD_HEALTH
// (Unit.cpp:9315): (base*basePct + totalValue) * totalPct, floored at 1 like
// Unit::SetMaxHealth (Unit.cpp:9588-9591).
func (s *session) effectiveMaxHealth() uint32 {
	if s == nil || s.player == nil {
		return 1
	}
	flat, baseMult, totalMult := s.auraHealthMods(0)
	v := (float64(s.player.MaxHealth)*float64(baseMult) + float64(flat)) * float64(totalMult)
	if v < 1 {
		v = 1
	}
	return uint32(v)
}

// effectiveMaxHealthExcluding recomputes the max health as if the named
// spell's aura were absent — the pre-apply max for the pct-preserve arms.
func (s *session) effectiveMaxHealthExcluding(spellID uint32) uint32 {
	if s == nil || s.player == nil {
		return 1
	}
	flat, baseMult, totalMult := s.auraHealthMods(spellID)
	v := (float64(s.player.MaxHealth)*float64(baseMult) + float64(flat)) * float64(totalMult)
	if v < 1 {
		v = 1
	}
	return uint32(v)
}

// applyHealthAuraArms runs the current-health half of the health-aura apply
// leg, called from the fresh-apply tail of applyAuraToTarget once the aura
// record is registered. 34/230/250 heal by the flat amount, capped at the new
// max (Unit::ModifyHealth, Unit.cpp:8519); 133 re-anchors current health to
// the pre-apply health pct (SpellAuraEffects.cpp:3729-3754); 282 adjusts no
// current health (SpellAuraEffects.cpp:3757-3768).
func (s *session) applyHealthAuraArms(spellID, auraType uint32, amount int32) {
	if s == nil || s.player == nil {
		return
	}
	newMax := s.effectiveMaxHealth()
	switch auraType {
	case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2:
		if amount > 0 {
			if nh := s.player.Health + uint32(amount); nh <= newMax {
				s.player.Health = nh
			} else {
				s.player.Health = newMax
			}
		} else if amount < 0 && s.player.Health > 1 {
			// C++ apply with a negative amount floors at 1
			// (ModifyHealth(max(1-health, amt)), SpellAuraEffects.cpp:3658).
			if drop := uint32(-amount); drop >= s.player.Health {
				s.player.Health = 1
			} else {
				s.player.Health -= drop
			}
		}
	case spellAuraModIncreaseHealthPercent:
		if s.player.Health == 0 {
			return
		}
		oldMax := s.effectiveMaxHealthExcluding(spellID)
		pct := float64(s.player.Health) / float64(oldMax)
		nh := uint32(pct * float64(newMax))
		if nh < 1 {
			nh = 1
		}
		s.player.Health = nh
	}
}

// removeHealthAuraArms runs the current-health half of the health-aura remove
// leg, called from removeAura after the aura record is erased. removed is the
// erased record so its amounts still count for the pre-remove max. 34/230/250
// drop current health by the amount, floored at 1
// (ModifyHealth(max(1-health, -amount)), SpellAuraEffects.cpp:3657-3658); a
// removed negative-amount aura heals back, capped at the new max. 133
// re-anchors to the pre-remove health pct; 282 adjusts nothing.
func (s *session) removeHealthAuraArms(removed *activeAura) {
	if s == nil || s.player == nil || removed == nil || s.server == nil || s.server.Data == nil {
		return
	}
	newMax := s.effectiveMaxHealth()
	spell, found, err := s.server.Data.Spell(removed.SpellID)
	if err != nil || !found {
		return
	}
	flat, baseMult, totalMult := s.auraHealthMods(0, removed)
	v := (float64(s.player.MaxHealth)*float64(baseMult) + float64(flat)) * float64(totalMult)
	if v < 1 {
		v = 1
	}
	oldMax := uint32(v)
	for index, effect := range spell.Effects {
		if index >= len(removed.Amounts) || removed.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		amount := removed.Amounts[index]
		switch effect.Aura {
		case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2:
			if amount > 0 {
				// Removal of a positive buff: health drops with the max,
				// never below 1.
				if drop := uint32(amount); drop >= s.player.Health {
					s.player.Health = 1
				} else {
					s.player.Health -= drop
				}
			} else if amount < 0 {
				if nh := s.player.Health + uint32(-amount); nh <= newMax {
					s.player.Health = nh
				} else {
					s.player.Health = newMax
				}
			}
		case spellAuraModIncreaseHealthPercent:
			if s.player.Health == 0 {
				continue
			}
			pct := float64(s.player.Health) / float64(oldMax)
			nh := uint32(pct * float64(newMax))
			if nh < 1 {
				nh = 1
			}
			s.player.Health = nh
		}
	}
}

// Creature targets are out of scope here: the C++ handlers are unit-generic
// (SpellAuraEffects.cpp:3650-3768) but Go tracks creature aura health deltas
// nowhere; bridging the creature leg needs the creature stat-mod model
// (standing gap, same queue as creature-caster EffectTransmitted).
