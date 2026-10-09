package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Combat rating indices for spells matching TrinityCore SharedDefines.h:1760-1790
const (
	CombatRatingHitSpell   uint8 = 7  // CR_HIT_SPELL
	CombatRatingCritSpell  uint8 = 10 // CR_CRIT_SPELL
	CombatRatingHasteSpell uint8 = 19 // CR_HASTE_SPELL
)

// getSpellHastePct returns the spell haste percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HASTE_SPELL) (Player.cpp:8800).
func (s *session) getSpellHastePct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHasteSpell])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 32.789989 rating = 1.0% spell haste (from gtCombatRatings.dbc)
	ratingPerPct := 32.789989 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// getSpellHitPct returns the bonus spell hit percentage from gear rating.
// Mirrors TrinityCore Player::GetRatingBonusValue(CR_HIT_SPELL) (Player.cpp:8800).
func (s *session) getSpellHitPct() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	rating := float64(s.player.CombatRatings[CombatRatingHitSpell])
	if rating <= 0 {
		return 0
	}
	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}
	// At level 80: 26.231995 rating = 1.0% spell hit (from gtCombatRatings.dbc)
	ratingPerPct := 26.231995 * (lvl / 80.0)
	if ratingPerPct < 5.0 {
		ratingPerPct = 5.0
	}
	return rating / ratingPerPct
}

// calculateSpellHitChance calculates the chance [0.0, 1.0] for a spell to hit the target,
// based on caster level, target level, whether target is a player or creature, and caster's Spell Hit Rating.
// Formula mirrors TrinityCore Unit::MagicSpellHitResult (Unit.cpp:2470-2520).
func (s *session) calculateSpellHitChance(targetLevel uint8, isTargetPlayer bool) float64 {
	if s == nil || s.player == nil {
		return 1.0
	}
	casterLevel := s.player.Level
	if casterLevel == 0 {
		casterLevel = 80
	}
	if targetLevel == 0 {
		targetLevel = casterLevel
	}

	levelDiff := int(targetLevel) - int(casterLevel)
	hitChance := 100.0

	if isTargetPlayer {
		if levelDiff <= 0 {
			hitChance = 96.0 // 4% base miss in PvP
		} else if levelDiff == 1 {
			hitChance = 95.0 // 5% miss
		} else if levelDiff == 2 {
			hitChance = 94.0 // 6% miss
		} else {
			hitChance = 94.0 - float64(levelDiff-2)*7.0
		}
	} else {
		// PvE against creatures
		if levelDiff <= 0 {
			hitChance = 96.0 // 4% base miss
		} else if levelDiff == 1 {
			hitChance = 95.0 // 5% miss
		} else if levelDiff == 2 {
			hitChance = 94.0 // 6% miss
		} else {
			// Level 83 boss against Level 80 player: 83% hit chance (17% miss)
			hitChance = 83.0 - float64(levelDiff-3)*11.0
		}
	}

	// Add spell hit percentage from gear rating (CombatRatingHitSpell = 7)
	hitChance += s.getSpellHitPct()

	if hitChance > 100.0 {
		hitChance = 100.0
	}
	if hitChance < 1.0 {
		hitChance = 1.0
	}

	return hitChance / 100.0
}

// rollSpellHit rolls whether a spell hits or misses the target.
func (s *session) rollSpellHit(targetLevel uint8, isTargetPlayer bool) bool {
	chance := s.calculateSpellHitChance(targetLevel, isTargetPlayer)
	return rand.Float64() < chance
}

// calculateSpellCastTime mirrors TrinityCore SpellInfo::CalcCastTime
// (SpellInfo.cpp:3091) plus WorldObject::ModSpellCastTime (Object.cpp:2448):
// the SpellCastTimes.dbc base is multiplied by the cast-speed modifier
// (UNIT_MOD_CAST_SPEED = 1/(1+hastePct/100), fed by CR_HASTE_SPELL rating
// via Player::ApplyCastTimePercentMod — Player.cpp:5621), then non-auto-repeat
// ranged spells with ATTR0_REQ_AMMO gain +500ms (SpellInfo.cpp:3102).
// Haste skips ATTR0_ABILITY / ATTR0_TRADESPELL / ATTR3_NO_DONE_BONUS spells
// and, for players, spells with no spell family name (Object.cpp:2462-2468).
// Player spell mods (SPELLMOD_CASTING_TIME) apply to the base before the
// haste multiplier (Object.cpp:2455, ahead of Object.cpp:2462).
// Noted gaps (not stubs): the ranged-attack-speed
// branch (m_modAttackSpeedPct[RANGED_ATTACK], Object.cpp:2470) has no Go
// ranged-haste infra, so that branch is a no-op here.
func (s *session) calculateSpellCastTime(spell wotlk.Spell) uint32 {
	baseCastTime := int32(0)
	if s.server != nil && s.server.Data != nil && spell.CastingTimeIndex > 0 {
		if value, ok, err := s.server.Data.SpellCastTime(spell.CastingTimeIndex); err == nil && ok && value > 0 {
			baseCastTime = int32(value)
		}
	}
	if baseCastTime == 0 {
		return 0
	}
	castTime := s.applySpellMod(spell, spellModCastingTime, baseCastTime)
	reqAmmo := spell.Attributes&spellAttr0ReqAmmo != 0 && spell.AttributesEx1&spellAttr2AutorepeatFlag == 0
	switch {
	case s.player != nil && spell.Attributes&(spellAttr0Ability|spellAttr0Tradespell) == 0 && spell.AttributesEx3&spellAttr3NoDoneBonus == 0 && spell.SpellFamilyName != 0:
		// Unit::CanInstantCast (Object.cpp:2468), set by
		// AuraEffect::HandleModCastingSpeed (SpellAuraEffects.cpp:3904-3919):
		// an SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (65) effect with amount
		// >= 1000 flags the unit for instant casts, ahead of the haste
		// multiplier. C++ re-evaluates on aura remove (the flag clears
		// unless another >=1000 effect remains); Go reads the live aura set
		// on every cast, so no cached flag can go stale.
		if s.hasInstantCastAura() {
			return 0
		}
		if hastePct := s.getSpellHastePct(); hastePct > 0 {
			castTime = int32(float64(castTime) / (1.0 + hastePct/100.0))
		}
	case reqAmmo:
		// Ranged-attack-speed branch (Object.cpp:2470) — no Go ranged-haste infra, no-op.
	case len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3881 && s.hasAura(67556):
		castTime = 500 // cooking with Chef Hat (Object.cpp:2471)
	}
	if reqAmmo {
		castTime += 500
	}
	if castTime < 0 {
		return 0
	}
	return uint32(castTime)
}

// hasInstantCastAura mirrors Unit::CanInstantCast (Unit.h:1674): true when
// the player carries an SPELL_AURA_MOD_CASTING_SPEED_NOT_STACK (65) effect
// with amount >= 1000 (AuraEffect::HandleModCastingSpeed,
// SpellAuraEffects.cpp:3904-3919). Amounts below 1000 take the normal
// cast-speed percent path and do not trigger the flag.
func (s *session) hasInstantCastAura() bool {
	for _, amount := range s.auraTypeModifiers(spellAuraCastingSpeedNotStack) {
		if amount >= 1000 {
			return true
		}
	}
	return false
}

// calculateSpellCritChance resolves the probability [0.0, 1.0] of a spell critical strike.
// Mirrors TrinityCore Player::UpdateSpellCritChance (StatSystem.cpp:819):
// school 0 (SPELL_SCHOOL_NORMAL) is zeroed; other schools sum the Gt-table
// Intellect contribution (Player::GetSpellCritFromIntellect, Player.cpp:5502),
// SPELL_AURA_MOD_SPELL_CRIT_CHANCE (57) and SPELL_AURA_MOD_CRIT_PCT (290),
// the school-masked SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL (71) term, and the
// CR_CRIT_SPELL rating bonus.
func (s *session) calculateSpellCritChance(targetGUID uint64, schoolMask uint8) float64 {
	if s == nil || s.player == nil {
		return 0.05
	}

	// Physical schools never crit via spell crit.
	if schoolMask == 1 {
		return 0
	}

	// 1. Intellect contribution from the Gt tables (includes the class base
	// crit from gtChanceToSpellCritBase.dbc): crit = critBase +
	// GetStat(STAT_INTELLECT)*critRatio, *100.
	critPct := s.getSpellCritFromIntellect(s.player)

	// 2. Flat aura bonuses.
	critPct += float64(s.playerAuraModifier(spellAuraModSpellCritChance))
	critPct += float64(s.playerAuraModifier(spellAuraModCritPct))

	// 3. School-specific aura bonus (Unit.cpp:4937: effect MiscValue & school mask).
	critPct += float64(s.playerAuraModifierByMiscMask(spellAuraModSpellCritChanceSchool, int32(schoolMask)))

	lvl := float64(s.player.Level)
	if lvl <= 0 {
		lvl = 80
	}

	// 4. Spell Crit Rating (CR_CRIT_SPELL = 10): 45.905987 rating per 1.0% crit at level 80
	rating := float64(s.player.CombatRatings[CombatRatingCritSpell])
	if rating > 0 {
		ratingPerPct := 45.905987 * (lvl / 80.0)
		if ratingPerPct < 5.0 {
			ratingPerPct = 5.0
		}
		critPct += rating / ratingPerPct
	}

	// 5. Defender resilience reduction (in PvP)
	if targetGUID != 0 && targetGUID != s.playerGUID && s.server != nil {
		if vicSess := s.server.findSessionByGUID(targetGUID); vicSess != nil {
			critBP := int32(math.Round(critPct * 100))
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenSpell, &critBP)
			critPct = float64(critBP) / 100.0
		}
	}

	if critPct < 0 {
		critPct = 0
	}
	if critPct > 100 {
		critPct = 100
	}

	return critPct / 100.0
}

// playerAuraModifierByMiscMask sums the active amounts of auras of the given
// type whose MiscValue overlaps miscMask.
// Mirrors TrinityCore Unit::GetTotalAuraModifierByMiscMask (Unit.cpp:4937):
// (effect MiscValue & miscMask) != 0.
func (s *session) playerAuraModifierByMiscMask(auraType uint32, miscMask int32) float32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total float32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraType {
				continue
			}
			if effect.MiscValue&miscMask == 0 {
				continue
			}
			total += float32(aura.Amounts[index])
		}
	}
	return total
}

// auraTypeModifiersFiltered returns per-effect amounts of the given aura type
// for which match(effect MiscValue) is true. It mirrors
// Unit::GetTotalAuraModifierByMiscMask (Unit.cpp:4937) when match tests the
// mask overlap, and Unit::GetTotalAuraModifierByMiscValue (Unit.cpp:4977-4985)
// when match tests equality. The structure follows auraTypeModifiers in
// movement_speed.go: per-effect amounts from loadedAuras via the DBC spell
// effects, with the stored single-amount fallback.
func (s *session) auraTypeModifiersFiltered(auraType uint32, match func(miscValue int32) bool) []int32 {
	if s == nil {
		return nil
	}
	result := make([]int32, 0)
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		matched := false
		usedStoredAmount := false
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != auraType || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if !match(effect.MiscValue) {
						continue
					}
					amount := aura.Amounts[index]
					if amount == 0 && aura.AuraType == auraType && !usedStoredAmount {
						amount = int32(aura.Amount)
						usedStoredAmount = true
					}
					if amount == 0 {
						amount = effect.BasePoints + 1
					}
					result = append(result, amount)
					matched = true
				}
			}
		}
		if !matched && aura.AuraType == auraType && match(aura.MiscValue) {
			amount := int32(aura.Amount)
			if amount == 0 {
				for _, storedAmount := range aura.Amounts {
					if storedAmount != 0 {
						amount = storedAmount
						break
					}
				}
			}
			result = append(result, amount)
		}
	}
	return result
}

// maxPositiveAuraModifierByMiscMask mirrors Unit::GetMaxPositiveAuraModifierByMiscMask
// (Unit.cpp:4945): the largest positive amount of the given aura type whose
// effect MiscValue overlaps miscMask, 0 when none.
func (s *session) maxPositiveAuraModifierByMiscMask(auraType uint32, miscMask int32) int32 {
	maxValue := int32(0)
	for _, amount := range s.auraTypeModifiersFiltered(auraType, func(miscValue int32) bool {
		return miscValue&miscMask != 0
	}) {
		if amount > maxValue {
			maxValue = amount
		}
	}
	return maxValue
}

// totalAuraModifierByMiscValue mirrors Unit::GetTotalAuraModifierByMiscValue
// (Unit.cpp:4977-4985): the sum of amounts of the given aura type whose effect
// MiscValue equals miscValue.
func (s *session) totalAuraModifierByMiscValue(auraType uint32, miscValue int32) int32 {
	var total int32
	for _, amount := range s.auraTypeModifiersFiltered(auraType, func(mv int32) bool {
		return mv == miscValue
	}) {
		total += amount
	}
	return total
}

// rollSpellCrit rolls whether the spell achieves a critical strike.
func (s *session) rollSpellCrit(targetGUID uint64, schoolMask uint8) bool {
	chance := s.calculateSpellCritChance(targetGUID, schoolMask)
	return rand.Float64() < chance
}

// tickCritChance mirrors the chance arm of AuraEffect::GetCritChanceFor
// (SpellAuraEffects.cpp:843-846) as used by the periodic tick handlers
// (HandlePeriodicDamageAurasTick, HandlePeriodicHealAurasTick): the caster's
// spell-crit chance done (baked into the aura's crit chance in C++) plus the
// victim's SPELL_AURA_MOD_ATTACKER_SPELL_CRIT_CHANCE taken modifier, minus
// resilience crit-chance reduction (folded into calculateSpellCritChance).
// takenCritBonusPct carries the victim-side modifier in percent points (0 for
// positive spells, where C++ skips the taken arm). The scripted taken arms
// (Shatter, Glyph of Shadowburn, Renewed Hope, Glyph of Fire Blast,
// Improved Faerie Fire, Starfire/Insect Swarm, Exorcism, Lava Burst) ride in
// takenCritBonusPct via tickScriptedTakenCritBonus at the tick call sites;
// the remaining arms (Shiv poisons, Flash of Light/Sacred Shield, Rend and
// Tear, Victory Rush) and SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) have
// no Go model and stay unbridged.
func (s *session) tickCritChance(targetGUID uint64, schoolMask uint8, takenCritBonusPct float64) float64 {
	chance := 0.0
	if s != nil {
		chance = s.calculateSpellCritChance(targetGUID, schoolMask)
	}
	chance += takenCritBonusPct / 100.0
	if chance < 0 {
		chance = 0
	}
	return chance
}

// tickScriptedTakenCritBonus mirrors the scripted taken-side arms of
// Unit::SpellCritChanceTaken (Unit.cpp:7236-7341) that fire for periodic
// ticks via AuraEffect::GetCritChanceFor (SpellAuraEffects.cpp:843-846):
//   - Shatter (caster OVERRIDE_CLASS_SCRIPTS miscValue 849/910/911
//     affecting the tick spell + victim AURA_STATE_FROZEN): +17/34/50
//     (Unit.cpp:7253-7264)
//   - Glyph of Shadowburn (caster OVERRIDE_CLASS_SCRIPTS miscValue 7917
//     affecting the tick spell + victim AURA_STATE_HEALTHLESS_35_PERCENT):
//     +aurEff->GetAmount() (Unit.cpp:7266-7269)
//   - Renewed Hope (caster OVERRIDE_CLASS_SCRIPTS 7997/7998 + caster
//     carries Weakened Soul 6788): +aurEff->GetAmount() (Unit.cpp:7271-7275)
//   - Glyph of Fire Blast (MAGE family, SpellFamilyFlags[0] == 0x2,
//     SpellIconID == 12; victim stunned or knocked out; caster aura 56369
//     effect 0): +aurEff->GetAmount() (Unit.cpp:7283-7288)
//   - Improved Faerie Fire (DRUID family; victim AURA_STATE_FAERIE_FIRE;
//     caster DUMMY aura family-DRUID icon 109): +aurEff->GetAmount()
//     (Unit.cpp:7293-7296)
//   - Starfire / Improved Insect Swarm (DRUID family,
//     SpellFamilyFlags[0] & 0x4, SpellIconID == 1485; caster DUMMY aura
//     family-DRUID icon 1771; victim carries a DRUID PERIODIC_DAMAGE aura
//     with SpellFamilyFlags[0] & 0x2): +aurEff->GetAmount()
//     (Unit.cpp:7300-7306)
//   - Exorcism (PALADIN family, Category == 19; victim demon or undead):
//     the tick crits outright (Unit.cpp:7308-7313)
//   - Lava Burst (SHAMAN family, SpellFamilyFlags[1] & 0x1000): victim
//     carries the caster's Flame Shock and victim aura-197 > -100 -> the
//     tick crits outright (Unit.cpp:7316-7324)
//
// Returns the bonus in percent points and whether the tick is a forced
// crit. The remaining arms (Shiv poisons — no Go current-spell model;
// Flash of Light/Sacred Shield — the tick-spell gate is a direct heal;
// Rend and Tear / Victory Rush — SPELL_DAMAGE_CLASS_MELEE, never
// periodic; SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308)) have no Go model
// and stay unbridged.
func (s *session) tickScriptedTakenCritBonus(ctx context.Context, targetGUID uint64, spell wotlk.Spell, tickKnown bool, victimFrozen, victimHasFlameShock bool, victimAura197Total int32, victimHealthless35, victimFaerieFire bool) (float64, bool) {
	bonus := 0.0
	if s == nil || s.server == nil || s.server.Data == nil || !tickKnown {
		return bonus, false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraOverrideClassScripts {
				continue
			}
			switch effect.MiscValue {
			case 849, 910, 911: // Shatter
				// AuraEffect::IsAffectedOnSpell gate (Unit.cpp:7242-7244)
				// plus the frozen-state check (Unit.cpp:7262-7264).
				if !victimFrozen || !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
					continue
				}
				switch effect.MiscValue {
				case 911:
					bonus += 50
				case 910:
					bonus += 34
				default:
					bonus += 17
				}
			case 7917: // Glyph of Shadowburn
				// IsAffectedOnSpell gate (Unit.cpp:7242-7244) plus the
				// healthless-35% state check (Unit.cpp:7267-7268).
				if victimHealthless35 && spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
					bonus += float64(aura.Amounts[index])
				}
			case 7997, 7998: // Renewed Hope
				if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) && s.hasAura(6788) {
					bonus += float64(aura.Amounts[index])
				}
			}
		}
	}
	// Custom crit by class (Unit.cpp:7279-7313).
	switch spell.SpellFamilyName {
	case spellFamilyMage:
		// Glyph of Fire Blast (Unit.cpp:7283-7288): the tick spell is Fire
		// Blast (SpellFamilyFlags[0] == 0x2 exactly, SpellIconID 12) and the
		// victim is stunned or knocked out.
		if spell.SpellFamilyFlags[0] == 0x2 && spell.SpellIconID == 12 &&
			s.targetHasAuraWithMechanic(ctx, targetGUID, (1<<mechanicStun)|(1<<mechanicKnockout)) {
			if amt, ok := s.auraEffectAmount(56369, 0); ok {
				bonus += float64(amt)
			}
		}
	case spellFamilyDruid:
		// Improved Faerie Fire (Unit.cpp:7293-7296): cumulative with the
		// Starfire arm below — C++ does not break between them.
		if victimFaerieFire {
			if amt, ok := s.dummyAuraAmountByIcon(spellFamilyDruid, 109); ok {
				bonus += float64(amt)
			}
		}
		// Starfire / Improved Insect Swarm (Unit.cpp:7300-7306).
		if spell.SpellFamilyFlags[0]&0x4 != 0 && spell.SpellIconID == 1485 {
			if amt, ok := s.dummyAuraAmountByIcon(spellFamilyDruid, 1771); ok &&
				s.targetHasFamilyAuraEffect(ctx, targetGUID, spellAuraPeriodicDamage, spellFamilyDruid, 0x2) {
				bonus += float64(amt)
			}
		}
	case spellFamilyPaladin:
		// Exorcism (Unit.cpp:7308-7313): guaranteed crit against demons
		// and undead. The creature-type mask resolves lazily — only a
		// Category-19 tick spell can reach it.
		if spell.Category == 19 {
			if mask, _ := s.targetCreatureTypeMask(ctx, targetGUID); mask&creatureTypeMaskDemonOrUndead != 0 {
				return bonus, true
			}
		}
	}
	// Lava Burst (Unit.cpp:7316-7324): guaranteed crit when the victim
	// carries the caster's Flame Shock and the victim's
	// MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE total exceeds -100.
	if spell.SpellFamilyName == spellFamilyShaman && spell.SpellFamilyFlags[1]&0x1000 != 0 &&
		victimHasFlameShock && victimAura197Total > -100 {
		return bonus, true
	}
	return bonus, false
}

// getSpellCritMultiplier calculates the critical strike damage/healing multiplier for a spell,
// factoring in the base 150% multiplier, talents/auras modifying critical bonus (AuraType 182),
// and metagem modifiers (+3% crit damage).
// Mirrors TrinityCore Unit::SpellCriticalDamageBonus (Unit.cpp:1650-1700).
func (s *session) getSpellCritMultiplier(spell wotlk.Spell) float64 {
	baseBonusPct := 50.0 // 1.5x base multiplier (1.0 + 50/100)
	extraBonusPct := 0.0
	metaBonusPct := 0.0

	if s == nil || s.player == nil {
		return 1.5
	}

	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped {
			continue
		}
		// SPELL_AURA_MOD_CRIT_DAMAGE_BONUS = 182
		if aura.AuraType == 182 {
			if aura.SchoolMask == 0 || (spell.SchoolMask != 0 && aura.SchoolMask&spell.SchoolMask != 0) {
				extraBonusPct += float64(aura.Amount)
			}
		}
		// Chaotic Skyflare Diamond / Relentless Earthsiege Diamond (3% increased critical damage)
		if aura.SpellID == 26297 || aura.SpellID == 44795 || aura.SpellID == 55341 || aura.SpellID == 28557 {
			metaBonusPct += 3.0
		}
	}
	s.castMu.Unlock()

	// Check learned talent spells:
	// Ruin (Warlock 17959): 100% extra bonus for Destruction (Fire 4 / Shadow 32) spells
	// Elemental Fury (Shaman 16089): 100% extra bonus for Fire 4 / Nature 8 / Frost 16 spells
	// Ice Shards (Mage 15058): up to 100% extra bonus for Frost 16 spells
	// Spell Power (Mage 35581): 25%/50% extra bonus
	if s.hasActiveSpell(17959) && (spell.SchoolMask&4 != 0 || spell.SchoolMask&32 != 0) { // Ruin
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(16089) && (spell.SchoolMask&4 != 0 || spell.SchoolMask&8 != 0 || spell.SchoolMask&16 != 0) { // Elemental Fury
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(15058) && (spell.SchoolMask&16 != 0) { // Ice Shards Rank 3
		extraBonusPct += 50.0
	} else if s.hasActiveSpell(35581) { // Spell Power Rank 2
		extraBonusPct += 25.0
	}

	multiplier := 1.0 + (baseBonusPct+extraBonusPct)/100.0
	if metaBonusPct > 0 {
		multiplier *= (1.0 + metaBonusPct/100.0)
	}
	return multiplier
}
