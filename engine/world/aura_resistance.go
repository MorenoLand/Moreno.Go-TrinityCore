package world

// Aura-driven resistance arms: AuraEffect::HandleAuraModResistanceExclusive
// (SpellAuraEffects.cpp:3232 — aura 143), HandleAuraModResistance
// (3255 — aura 22), HandleAuraModBaseResistancePCT (3278 — aura 142),
// HandleModResistancePercent (3290 — aura 101), HandleModBaseResistance
// (3335 — aura 83) and HandleModTargetResistance (3357 — aura 123).
// C++ keeps these in Unit::m_auraModifiersGroup[UNIT_MOD_RESISTANCE_START +
// school] (Unit::GetTotalAuraModValue, Unit.cpp:9437-9446): BASE_VALUE flat
// (the 143 winner — max positive amount per school bit, excluding the effect
// itself, SpellAuraEffects.cpp:3239-3246), BASE_PCT product (142), TOTAL_VALUE
// flat sum (22 + 83) and TOTAL_PCT product (101); effective =
// (BASE_VALUE*BASE_PCT + TOTAL_VALUE)*TOTAL_PCT. WotLK players carry no
// level-based base resistance, so BASE_VALUE starts at the 143 winner alone.
// The school-NORMAL (bit 0, armor) arms write UNIT_MOD_RESISTANCE_START+0,
// which Player::UpdateArmor never reads (StatSystem.cpp:245-280 — armor
// comes from UNIT_MOD_ARMOR), so those bits are write-only/display-only in
// C++ and stay out of Go's effective armor too.

const (
	spellAuraModResistance          = 22  // SPELL_AURA_MOD_RESISTANCE (SpellAuraDefines.h:102)
	spellAuraModBaseResistance      = 83  // SPELL_AURA_MOD_BASE_RESISTANCE (SpellAuraDefines.h:163)
	spellAuraModResistancePct       = 101 // SPELL_AURA_MOD_RESISTANCE_PCT (SpellAuraDefines.h:181)
	spellAuraModTargetResistance    = 123 // SPELL_AURA_MOD_TARGET_RESISTANCE (SpellAuraDefines.h:203)
	spellAuraModBaseResistancePct   = 142 // SPELL_AURA_MOD_BASE_RESISTANCE_PCT (SpellAuraDefines.h:222)
	spellAuraModResistanceExclusive = 143 // SPELL_AURA_MOD_RESISTANCE_EXCLUSIVE (SpellAuraDefines.h:223)
)

// schoolCount is MAX_SPELL_SCHOOL (SharedDefines.h): 0 normal/armor, 1 holy,
// 2 fire, 3 nature, 4 frost, 5 shadow, 6 arcane.
const schoolCount = 7

// resistanceFold carries the four UNIT_MOD buckets per school for the
// resistance fold, plus the sign-split 22 sums the buff-mod display fields
// need (Unit::UpdateResistanceBuffModsMod reads only 22, never 83).
type resistanceFold struct {
	exclusive [schoolCount]int32   // 143: max positive amount per school (BASE_VALUE)
	flat      [schoolCount]int32   // 22 + 83 flat sum (TOTAL_VALUE)
	basePct   [schoolCount]float32 // 142 product (BASE_PCT)
	totalPct  [schoolCount]float32 // 101 product (TOTAL_PCT)
	pos22     [schoolCount]int32   // 22: positive-amount sum per school (display)
	neg22     [schoolCount]int32   // 22: negative-amount sum per school (display)
}

// auraResistanceMods folds the live resistance-modifier aura effects into the
// four UNIT_MOD buckets. extra carries aura records not in the session's
// active map, with the same Stopped convention as auraHealthMods.
func (s *session) auraResistanceMods(excludeSpellID uint32, extra ...*activeAura) (f resistanceFold) {
	for i := range f.basePct {
		f.basePct[i], f.totalPct[i] = 1, 1
	}
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return f
	}
	auras := s.loadedAuras()
	auras = append(auras, extra...)
	for i, aura := range auras {
		if aura == nil || aura.SpellID == excludeSpellID {
			continue
		}
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
			case spellAuraModResistanceExclusive:
				// Winner-takes-all per school (SpellAuraEffects.cpp:3239-3246):
				// the registered BASE_VALUE is the max positive amount; ties
				// keep the already-applied (earlier) effect because the apply
				// leg writes GetAmount()-amount = 0. Order i is the
				// Slot-order proxy like statWinners.
				if amount > 0 {
					for x := 0; x < schoolCount; x++ {
						if effect.MiscValue&(1<<uint(x)) != 0 && amount > f.exclusive[x] {
							f.exclusive[x] = amount
						}
					}
				}
			case spellAuraModResistance:
				for x := 0; x < schoolCount; x++ {
					if effect.MiscValue&(1<<uint(x)) != 0 {
						f.flat[x] += amount
						// UpdateResistanceBuffModsMod sign split
						// (Unit.cpp:5059-5084): the display fields sum only
						// the 22 arm, positive and negative amounts apart.
						if amount > 0 {
							f.pos22[x] += amount
						} else if amount < 0 {
							f.neg22[x] += amount
						}
					}
				}
			case spellAuraModBaseResistance:
				for x := 0; x < schoolCount; x++ {
					if effect.MiscValue&(1<<uint(x)) != 0 {
						f.flat[x] += amount
					}
				}
			case spellAuraModBaseResistancePct:
				for x := 0; x < schoolCount; x++ {
					if effect.MiscValue&(1<<uint(x)) != 0 {
						f.basePct[x] *= 1 + float32(amount)/100
					}
				}
			case spellAuraModResistancePct:
				for x := 0; x < schoolCount; x++ {
					if effect.MiscValue&(1<<uint(x)) != 0 {
						f.totalPct[x] *= 1 + float32(amount)/100
					}
				}
			}
		}
	}
	return f
}

// auraTouchesResistance reports whether any of the aura's effects is a
// resistance modifier (22/83/101/142/143), mirroring the five handlers'
// table rows (SpellAuraEffects.cpp:87/168/186/227/228). Aura 123
// (MOD_TARGET_RESISTANCE) is display-only (HandleNoImmediateEffect) and does
// not participate in the fold.
func (s *session) auraTouchesResistance(aura *activeAura) bool {
	for _, auraType := range []uint32{
		spellAuraModResistance,
		spellAuraModBaseResistance,
		spellAuraModResistancePct,
		spellAuraModBaseResistancePct,
		spellAuraModResistanceExclusive,
	} {
		if s.activeAuraHasEffect(aura, auraType) {
			return true
		}
	}
	return false
}

// effectiveResistances mirrors Player::UpdateResistances (StatSystem.cpp:245)
// for schools 1..6: (BASE_VALUE*BASE_PCT + TOTAL_VALUE)*TOTAL_PCT, clamped at
// 0 (Unit::GetResistance, Unit.cpp:863 — the field is uint32). base carries
// the non-aura total (item resists). School 0 is armor, whose C++ value never
// reads the resistance buckets, so it is left to the armor path.
func (s *session) effectiveResistances(base [schoolCount]uint32) [schoolCount]uint32 {
	f := s.auraResistanceMods(0)
	out := base
	for x := 1; x < schoolCount; x++ {
		v := (float64(f.exclusive[x])*float64(f.basePct[x]) + float64(f.flat[x])) * float64(f.totalPct[x])
		if v < 0 {
			v = 0
		}
		out[x] = uint32(v)
	}
	return out
}

// resistanceBuffModDisplay mirrors Unit::UpdateResistanceBuffModsMod
// (Unit.cpp:5059-5084): per school, the positive buff-mod display value is
// the max-positive 143 exclusive plus the positive-amount 22 sum, times the
// 101 pct factor; the negative value is the negative-amount 22 sum times the
// 101 factor. 83 (base resistance) and 142 (base pct) never participate.
// The caller writes the pair as float32 bits into the PRIVATE|OWNER
// UNIT_FIELD_RESISTANCEBUFFMODSPOSITIVE/NEGATIVE fields.
func (s *session) resistanceBuffModDisplay() ([schoolCount]float32, [schoolCount]float32) {
	f := s.auraResistanceMods(0)
	var pos, neg [schoolCount]float32
	for x := 0; x < schoolCount; x++ {
		pos[x] = float32(f.exclusive[x]+f.pos22[x]) * f.totalPct[x]
		neg[x] = float32(f.neg22[x]) * f.totalPct[x]
	}
	return pos, neg
}

// 83/142 pet arms and 123 target-resistance display: HandleModBaseResistance
// and HandleAuraModBaseResistancePCT write UNIT_MOD_ARMOR for pets, and
// HandleModTargetResistance only flips PLAYER_FIELD_MOD_TARGET_RESISTANCE /
// PLAYER_FIELD_MOD_TARGET_PHYSICAL_RESISTANCE (SpellAuraEffects.cpp:3357-
// 3376) as a HandleNoImmediateEffect display arm. The 123 display leg is
// bridged in targetResistanceDisplay below; the 22/83 pet armor arms stay a
// documented gap (Go has no pet armor model), same queue as the creature
// resistance leg.

// spellSchoolMaskSpell is SPELL_SCHOOL_MASK_SPELL (SharedDefines.h): the
// fire/nature/frost/shadow/arcane school bits, excluding holy and armor.
const spellSchoolMaskSpell = 0x7C

// targetResistanceDisplay mirrors AuraEffect::HandleModTargetResistance's
// display arms (SpellAuraEffects.cpp:3357-3376). The live 123 amount lands on
// PLAYER_FIELD_MOD_TARGET_PHYSICAL_RESISTANCE (1196) when the effect's misc
// covers SPELL_SCHOOL_NORMAL, and on PLAYER_FIELD_MOD_TARGET_RESISTANCE
// (1195) only when it covers every spell school
// ((misc & SPELL_SCHOOL_MASK_SPELL) == SPELL_SCHOOL_MASK_SPELL). The 1195
// field also carries Player::ApplySpellPenetrationBonus's -amount leg
// (StatSystem.cpp:241); buildPlayerUpdate adds both legs.
func (s *session) targetResistanceDisplay() (spell, physical int32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spellData, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spellData.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModTargetResistance {
				continue
			}
			amount := aura.Amounts[index]
			if effect.MiscValue&(1<<0) != 0 {
				physical += amount
			}
			if effect.MiscValue&spellSchoolMaskSpell == spellSchoolMaskSpell {
				spell += amount
			}
		}
	}
	return spell, physical
}
