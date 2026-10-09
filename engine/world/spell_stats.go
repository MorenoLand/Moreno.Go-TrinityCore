package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
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
// CR_CRIT_SPELL rating bonus. isPeriodic selects the periodic-tick done side
// (Aura::CalcPeriodicCritChance, SpellAuras.cpp:494-502); the direct-spell
// done side runs through Unit::SpellCritChanceDone (Unit.cpp:7167-7214).
func (s *session) calculateSpellCritChance(targetGUID uint64, schoolMask uint8, spell wotlk.Spell, isPeriodic bool) float64 {
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

	// 4b. Caster spell mods (Unit::SpellCritChanceDone, Unit.cpp:7207-7211):
	// SPELLMOD_CRITICAL_CHANCE folds into the done chance before the taken
	// side (resilience) runs, so PCT mods multiply the pre-resilience base
	// exactly as in C++. C++ bakes this once at aura apply for periodic
	// ticks (Aura::SaveCasterInfo); Go recomputes per tick, which matches
	// the baked value while the caster's mods are unchanged.
	critPct = s.applySpellModFloat(spell, spellModCriticalChance, critPct)

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
// C++ Unit::SpellCritChanceDone (Unit.cpp:7167-7170): direct (non-periodic)
// spells without SPELL_ATTR0_CU_CAN_CRIT never crit.
func (s *session) rollSpellCrit(targetGUID uint64, schoolMask uint8, spell wotlk.Spell) bool {
	if spell.Attributes&spellAttr0CuCanCrit == 0 {
		return false
	}
	return rand.Float64() < s.calculateSpellCritChance(targetGUID, schoolMask, spell, false)
}

// directSpellCritChance is the full direct-spell crit probability: the
// CU_CAN_CRIT gate, the done side (calculateSpellCritChance), and the
// victim-side taken arms below. Mirrors Spell::DoEffectOnLaunchTarget
// (Spell.cpp:7784-7788): m_spellValue->CriticalChance override (no Go
// model), then SpellCritChanceDone, then SpellCritChanceTaken. The
// m_attackType routing from SpellInfo::GetAttackType (SpellInfo.cpp:1266)
// applies: melee/ranged-DmgClass spells use the weapon-crit done leg
// (Unit::GetUnitCriticalChanceDone, Unit.cpp:2819) and the
// GetUnitCriticalChanceTaken victim leg (Unit.cpp:2846); SPELL_DAMAGE_CLASS_NONE
// spells never crit (both legs return 0).
func (s *session) directSpellCritChance(ctx context.Context, target combatTarget, isPlayerVictim bool, schoolMask uint8, spell wotlk.Spell) float64 {
	if spell.Attributes&spellAttr0CuCanCrit == 0 {
		return 0
	}
	switch spell.DefenseType {
	case spellDamageClassMelee, spellDamageClassRanged:
		return s.directWeaponCritChance(ctx, target, isPlayerVictim, schoolMask, spell)
	case spellDamageClassMagic:
		chance := s.calculateSpellCritChance(target.GUID, schoolMask, spell, false)
		return chance + s.directSpellTakenCritBonus(target, isPlayerVictim, schoolMask, spell)
	default:
		return 0
	}
}

// rollDirectSpellCrit rolls a direct-spell crit including the victim taken arms.
func (s *session) rollDirectSpellCrit(ctx context.Context, target combatTarget, isPlayerVictim bool, schoolMask uint8, spell wotlk.Spell) bool {
	return rand.Float64() < s.directSpellCritChance(ctx, target, isPlayerVictim, schoolMask, spell)
}

// spellWeaponAttackType mirrors SpellInfo::GetAttackType (SpellInfo.cpp:1266-1290),
// the m_attackType the spell constructor derives for DoEffectOnLaunchTarget's
// crit legs (Spell.cpp:7786-7787). MELEE-DmgClass spells use the offhand when
// SPELL_ATTR3_REQ_OFFHAND is set; RANGED-DmgClass spells use the ranged attack
// when they are ranged-weapon spells; other spells use the ranged attack only
// for auto-repeat (wands), else the base attack.
func spellWeaponAttackType(spell wotlk.Spell) protocol.WeaponAttackType {
	switch spell.DefenseType {
	case spellDamageClassMelee:
		if spell.AttributesEx3&spellAttr3ReqOffhand != 0 {
			return protocol.OffAttack
		}
		return protocol.BaseAttack
	case spellDamageClassRanged:
		if isRangedWeaponSpell(spell) {
			return protocol.RangedAttack
		}
		return protocol.BaseAttack
	default:
		if spell.AttributesEx1&spellAttr2AutorepeatFlag != 0 {
			return protocol.RangedAttack
		}
		return protocol.BaseAttack
	}
}

// directWeaponCritChance is the full direct-spell crit probability for
// SPELL_DAMAGE_CLASS_MELEE/RANGED spells: the weapon-crit done leg plus the
// GetUnitCriticalChanceTaken victim leg (Unit.cpp:7200-7201, 7378-7386).
func (s *session) directWeaponCritChance(ctx context.Context, target combatTarget, isPlayerVictim bool, schoolMask uint8, spell wotlk.Spell) float64 {
	if s == nil {
		return 0
	}
	attackType := spellWeaponAttackType(spell)
	chance := s.directWeaponCritChanceDone(spell, schoolMask, attackType)
	return s.directWeaponCritChanceTaken(ctx, target, isPlayerVictim, spell, attackType, chance)
}

// directWeaponCritChanceDone mirrors the MELEE/RANGED arms of
// Unit::SpellCritChanceDone (Unit.cpp:7196-7211): GetUnitCriticalChanceDone
// (Unit.cpp:2819-2844) plus the school-masked
// SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL (71) term, then
// SPELLMOD_CRITICAL_CHANCE via the spell-mod owner, clamped at zero.
// For players the weapon-crit basis is the derived PLAYER_*_CRIT_PERCENTAGE
// field selected by attack type; the flat aura terms (52/290) that
// UpdateCritPercentage (StatSystem.cpp:624) folds into those fields are not
// part of Go's stored fields, so they are added here. The weapon-skill term
// (0.04%/skill vs max for level) has no Go model and stays a residual.
func (s *session) directWeaponCritChanceDone(spell wotlk.Spell, schoolMask uint8, attackType protocol.WeaponAttackType) float64 {
	if s == nil || s.player == nil {
		return 0
	}
	var chance float64
	switch attackType {
	case protocol.OffAttack:
		chance = float64(s.player.OffhandCrit)
	case protocol.RangedAttack:
		chance = float64(s.player.RangedCrit)
	default:
		chance = float64(s.player.MeleeCrit)
	}
	chance += float64(s.playerAuraModifier(spellAuraModWeaponCritPercent))
	chance += float64(s.playerAuraModifier(spellAuraModCritPct))
	chance += float64(s.playerAuraModifierByMiscMask(spellAuraModSpellCritChanceSchool, int32(schoolMask)))
	chance = s.applySpellModFloat(spell, spellModCriticalChance, chance)
	if chance < 0 {
		chance = 0
	}
	return chance / 100.0
}

// directWeaponCritChanceTaken mirrors the MELEE/RANGED arms of
// Unit::SpellCritChanceTaken for direct spells: the Rend and Tear /
// Victory Rush caster class arms (Unit.cpp:7353-7376), then
// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2846-2880) — flat victim
// SPELL_AURA_MOD_ATTACKER_MELEE/RANGED_CRIT_CHANCE (187/188) by attack type,
// SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) caster-matched (no
// IsAffectedOnSpell gate on this path, per the C++ lambda), resilience with
// the melee/ranged taken rating, then SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE
// (197) applied after resilience with no positivity gate (unlike the MAGIC
// leg). The defense-skill vs weapon-skill bonus has no Go weapon-skill model
// and stays a residual; the chance is clamped at zero like C++.
func (s *session) directWeaponCritChanceTaken(ctx context.Context, target combatTarget, isPlayerVictim bool, spell wotlk.Spell, attackType protocol.WeaponAttackType, doneChance float64) float64 {
	if s == nil || s.server == nil {
		return doneChance
	}
	chance := doneChance * 100.0
	// Custom crit by class (Unit.cpp:7353-7376) — MELEE DmgClass only.
	if spell.DefenseType == spellDamageClassMelee {
		switch spell.SpellFamilyName {
		case spellFamilyDruid:
			// Rend and Tear (Unit.cpp:7357-7364): Ferocious Bite
			// (family flags[0] & 0x800000, icon 1680) on a bleeding target
			// gains the caster's eff-1 dummy aura amount (icon 2859).
			if spell.SpellFamilyFlags[0]&0x00800000 != 0 && spell.SpellIconID == 1680 &&
				s.targetHasAuraState(ctx, target.GUID, auraStateBleeding, spell) {
				if amt, ok := s.dummyAuraAmountByIconEffIndex(spellFamilyDruid, 2859, 1); ok {
					chance += float64(amt)
				}
			}
		case spellFamilyWarrior:
			// Victory Rush (Unit.cpp:7368-7375): glyph 58382 eff 0.
			if spell.SpellFamilyFlags[1]&0x100 != 0 {
				if amt, ok := s.auraEffectAmount(58382, 0); ok {
					chance += float64(amt)
				}
			}
		}
	}
	var flatAura uint32 = spellAuraModAttackerMeleeCritChance
	if attackType == protocol.RangedAttack {
		flatAura = spellAuraModAttackerRangedCritChance
	}
	if isPlayerVictim {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			chance += float64(vicSess.playerAuraModifier(flatAura))
			chance += s.casterMatchedCritChanceForCaster(spell)
			bp := int32(math.Round(chance * 100))
			cr := CombatRatingCritTakenMelee
			if attackType == protocol.RangedAttack {
				cr = CombatRatingCritTakenRanged
			}
			vicSess.applyResilienceToMeleeCritChance(true, cr, &bp)
			chance = float64(bp) / 100.0
			chance += float64(vicSess.playerAuraModifier(spellAuraModAttackerSpellAndWeaponCritChance))
		}
	} else {
		key := creatureAuraKeyForTarget(target)
		chance += float64(creatureAuraModifierSum(s.server, key, flatAura))
		chance += s.casterMatchedCritChanceForCaster(spell)
		chance += float64(creatureAuraModifierSum(s.server, key, spellAuraModAttackerSpellAndWeaponCritChance))
	}
	if chance < 0 {
		chance = 0
	}
	return chance / 100.0
}

// casterMatchedCritChanceForCaster mirrors the SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER
// (308) arm inside Unit::GetUnitCriticalChanceTaken (Unit.cpp:2862-2866): the
// sum of the caster's 308-aura amounts whose CasterGUID is the caster's own.
// This path has no IsAffectedOnSpell gate (the magic leg's post-switch arm at
// Unit.cpp:7390-7397 does, and stays unbridged).
func (s *session) casterMatchedCritChanceForCaster(spell wotlk.Spell) float64 {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return 0
	}
	var total float64
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 || aura.CasterGUID != s.playerGUID {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != spellAuraModCritChanceForCaster {
				continue
			}
			total += float64(aura.Amounts[index])
		}
	}
	return total
}

// dummyAuraAmountByIconEffIndex mirrors Unit::GetDummyAuraEffect(family,
// icon, effIndex) (Unit.cpp:4540-4543 -> 4510-4522): the first live DUMMY
// aura on the caster whose spell matches family+icon (with no family flags
// set) at exactly the given effect index.
func (s *session) dummyAuraAmountByIconEffIndex(family, iconID uint32, effIndex int) (int32, bool) {
	if s.server == nil || s.server.Data == nil || effIndex < 0 {
		return 0, false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || auraSpell.SpellFamilyName != family || auraSpell.SpellIconID != iconID {
			continue
		}
		if auraSpell.SpellFamilyFlags[0] != 0 || auraSpell.SpellFamilyFlags[1] != 0 || auraSpell.SpellFamilyFlags[2] != 0 {
			continue
		}
		if effIndex >= len(aura.Amounts) || effIndex >= len(auraSpell.Effects) || aura.EffectMask&(1<<uint(effIndex)) == 0 {
			continue
		}
		eff := auraSpell.Effects[effIndex]
		if !spellEffectIsAuraEffect(eff) || eff.Aura != spellAuraDummy {
			continue
		}
		amount := aura.Amounts[effIndex]
		if amount == 0 {
			amount = eff.BasePoints + 1
		}
		return amount, true
	}
	return 0, false
}

// directSpellTakenCritBonus mirrors the victim-side arms of
// Unit::SpellCritChanceTaken (Unit.cpp:7230-7240) for direct (non-periodic)
// SPELL_DAMAGE_CLASS_MAGIC spells: the victim's
// SPELL_AURA_MOD_ATTACKER_SPELL_CRIT_CHANCE (179, school-masked) and
// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197, unfiltered),
// gated on the spell being non-positive like C++. The MELEE/RANGED DmgClass
// arms live in directWeaponCritChanceTaken instead (C++ takes the
// GetUnitCriticalChanceTaken leg there, never the 179/197 magic arms).
// C++ applies 179 before resilience and 197 after; both are flat additions and
// Go's resilience leg already ran inside calculateSpellCritChance, so the
// post-resilience fold here is arithmetically identical.
func (s *session) directSpellTakenCritBonus(target combatTarget, isPlayerVictim bool, schoolMask uint8, spell wotlk.Spell) float64 {
	if spellIsPositive(spell) || s == nil || s.server == nil {
		return 0
	}
	bonus := float64(0)
	if isPlayerVictim {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			bonus += float64(vicSess.playerAuraModifierByMiscMask(spellAuraModAttackerSpellCritChance, int32(schoolMask)))
			bonus += float64(vicSess.playerAuraModifier(spellAuraModAttackerSpellAndWeaponCritChance))
		}
	} else {
		key := creatureAuraKeyForTarget(target)
		for _, amt := range creatureAuraModifiersByMiscMask(s.server, key, spellAuraModAttackerSpellCritChance, uint32(schoolMask)) {
			bonus += float64(amt)
		}
		bonus += float64(creatureAuraModifierSum(s.server, key, spellAuraModAttackerSpellAndWeaponCritChance))
	}
	return bonus / 100.0
}

// tickCritChance mirrors the chance arm of AuraEffect::GetCritChanceFor
// (SpellAuraEffects.cpp:843-846) as used by the periodic tick handlers
// (HandlePeriodicDamageAurasTick, HandlePeriodicHealAurasTick): the caster's
// spell-crit chance done (baked into the aura's crit chance in C++ via
// Aura::CalcPeriodicCritChance, SpellAuras.cpp:494-502 — gated here by
// canPeriodicTickCrit) plus the victim's SPELL_AURA_MOD_ATTACKER_SPELL_CRIT_CHANCE
// taken modifier, minus resilience crit-chance reduction (folded into
// calculateSpellCritChance).
// takenCritBonusPct carries the victim-side modifier in percent points (0 for
// positive spells, where C++ skips the taken arm). The scripted taken arms
// (Shatter, Glyph of Shadowburn, Renewed Hope, Glyph of Fire Blast,
// Improved Faerie Fire, Starfire/Insect Swarm, Exorcism, Lava Burst) ride in
// takenCritBonusPct via tickScriptedTakenCritBonus at the tick call sites;
// the remaining arms (Shiv poisons, Flash of Light/Sacred Shield, Rend and
// Tear, Victory Rush) and SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) have
// no Go model and stay unbridged.
func (s *session) tickCritChance(targetGUID uint64, schoolMask uint8, spell wotlk.Spell, takenCritBonusPct float64) float64 {
	chance := 0.0
	// Aura::CalcPeriodicCritChance (SpellAuras.cpp:494-502): the done side
	// is zero unless the aura may periodic-crit. The victim-side taken arms
	// still apply on top — C++ bakes a zero done chance and the scripted
	// taken arms (Lava Burst, Shatter, ...) can still force the crit.
	if s != nil && s.canPeriodicTickCrit(spell) {
		chance = s.calculateSpellCritChance(targetGUID, schoolMask, spell, true)
	}
	chance += takenCritBonusPct / 100.0
	if chance < 0 {
		chance = 0
	}
	return chance
}

// canPeriodicTickCrit mirrors Aura::CanPeriodicTickCrit (SpellAuras.cpp:475-491):
// a periodic aura's ticks crit only when the spell allows it.
// SPELL_ATTR2_CANT_CRIT blocks outright; SPELL_ATTR4_INHERIT_CRIT_FROM_AURA
// passes through; a SPELL_AURA_ABILITY_PERIODIC_CRIT (286) aura on the caster
// affecting the spell opens it; Rupture (rogue, icon 500) is hardcoded.
// C++ evaluates this once at aura apply (Aura::SaveCasterInfo); Go evaluates
// per tick, matching while the caster's auras are unchanged.
func (s *session) canPeriodicTickCrit(spell wotlk.Spell) bool {
	if spell.AttributesEx1&spellAttr2CantCrit != 0 {
		return false
	}
	if spell.AttributesEx4&spellAttr4InheritCritFromAura != 0 {
		return true
	}
	if s != nil && s.server != nil && s.server.Data != nil {
		for _, aura := range s.loadedAuras() {
			if aura == nil || aura.Stopped {
				continue
			}
			auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
			if err != nil || !found {
				continue
			}
			for index, effect := range auraSpell.Effects {
				if effect.Aura != spellAuraAbilityPeriodicCrit || aura.EffectMask&(1<<uint(index)) == 0 {
					continue
				}
				if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
					return true
				}
			}
		}
	}
	if spell.SpellIconID == 500 && spell.SpellFamilyName == spellFamilyRogue {
		return true
	}
	return false
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
// Flash of Light/Sacred Shield — the tick-spell gate is a direct heal)
// have no Go model and stay unbridged. Rend and Tear / Victory Rush and
// SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) are bridged on the direct
// melee-DmgClass path (directWeaponCritChanceTaken); they never fire for
// periodic ticks in C++ (DmgClass gate at Unit.cpp:7347-7386), so they stay
// unbridged here by design.
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

// Crit-bonus aura types (SpellAuraDefines.h:50,243,249,283,284).
const (
	spellAuraModCriticalHealingAmount uint32 = 50  // SPELL_AURA_MOD_CRITICAL_HEALING_AMOUNT
	spellAuraModCritDamageBonus       uint32 = 163 // SPELL_AURA_MOD_CRIT_DAMAGE_BONUS
	spellAuraModCritPercentVersus     uint32 = 169 // SPELL_AURA_MOD_CRIT_PERCENT_VERSUS
	spellAuraModAttackerMeleeCritDmg  uint32 = 203 // SPELL_AURA_MOD_ATTACKER_MELEE_CRIT_DAMAGE (victim auras)
	spellAuraModAttackerRangedCritDmg uint32 = 204 // SPELL_AURA_MOD_ATTACKER_RANGED_CRIT_DAMAGE (victim auras)
)

// critDamageTalentRanks lists every rank of the passive crit-damage talents.
// The bonus amounts come from the DBC (Go holds no passive aura instances),
// misc-matched against the crit spell's school like any other 163 aura.
var critDamageTalentRanks = map[uint32]bool{
	17873: true, 17875: true, 17876: true, 17877: true, 17959: true, // Ruin ranks 1-5
	15047: true, 15062: true, 15061: true, 15059: true, 15058: true, // Ice Shards ranks 1-5
	16089: true,              // Elemental Fury
	35578: true, 35581: true, // Spell Power ranks 1-2
}

// critDamageMetaGemSpells are the +3% critical damage meta-gem bonus spells
// (Chaotic Skyflare Diamond / Relentless Earthsiege Diamond and kin).
var critDamageMetaGemSpells = map[uint32]bool{26297: true, 44795: true, 55341: true, 28557: true}

// critDamageAuraMultiplier mirrors the GetTotalAuraMultiplierByMiscMask
// (SPELL_AURA_MOD_CRIT_DAMAGE_BONUS, schoolMask) leg of
// Unit::SpellCriticalDamageBonus (Unit.cpp:7428): the product of
// (1 + amount/100) over the caster's 163 auras whose misc mask intersects the
// spell school, with the passive crit-damage talents folded in from DBC
// amounts and each active +3% meta gem folded in multiplicatively.
// Gaps (not stubs): the SameEffectSpellGroup highest-wins rule (Unit.cpp:4872)
// has no Go fold — every matching 163 aura multiplies.
func (s *session) critDamageAuraMultiplier(schoolMask uint32) float64 {
	mult := 1.0
	if s == nil || s.player == nil {
		return mult
	}
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped {
			continue
		}
		if critDamageMetaGemSpells[aura.SpellID] {
			mult *= 1.03
			continue
		}
		if aura.AuraType != spellAuraModCritDamageBonus {
			continue
		}
		if schoolMask != 0 && uint32(aura.MiscValue)&schoolMask != 0 {
			mult *= 1.0 + float64(int32(aura.Amount))/100.0
		}
	}
	s.castMu.Unlock()
	mult *= s.critDamageTalentMultiplier(schoolMask)
	return mult
}

// critDamageTalentMultiplier folds the passive crit-damage talents into the
// 163 product: each learned rank contributes (1 + DBC amount/100) when its
// misc mask intersects the spell school, mirroring what C++ reads had the
// passive aura been applied (Unit.cpp:7428).
func (s *session) critDamageTalentMultiplier(schoolMask uint32) float64 {
	mult := 1.0
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return mult
	}
	for _, learned := range s.player.Spells {
		if !learned.Active || learned.Disabled || !critDamageTalentRanks[learned.ID] {
			continue
		}
		spell, found, err := s.server.Data.Spell(learned.ID)
		if err != nil || !found {
			continue
		}
		for i := range spell.Effects {
			eff := &spell.Effects[i]
			if eff.Aura != spellAuraModCritDamageBonus {
				continue
			}
			if schoolMask != 0 && uint32(eff.MiscValue)&schoolMask != 0 {
				mult *= 1.0 + float64(eff.CalcValue())/100.0
			}
		}
	}
	return mult
}

type versusAuraAmount struct {
	misc   int32
	amount int32
}

// casterVersusAuras collects the caster's live 169
// (MOD_CRIT_PERCENT_VERSUS) auras; the victim-mask match happens in the
// callers so the mask is only resolved when such an aura exists.
func (s *session) casterVersusAuras() []versusAuraAmount {
	if s == nil || s.player == nil {
		return nil
	}
	var auras []versusAuraAmount
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModCritPercentVersus {
			continue
		}
		auras = append(auras, versusAuraAmount{misc: aura.MiscValue, amount: int32(aura.Amount)})
	}
	s.castMu.Unlock()
	return auras
}

// spellCriticalDamageBonus mirrors Unit::SpellCriticalDamageBonus
// (Unit.cpp:7410-7449): base 50% bonus on the default DmgClass (melee/ranged
// DmgClass spells take the 100% weapon arm in spellWeaponCritDamageBonus —
// callers route DefenseType 2/3 away from this function), the 163 aura product by school misc mask, the
// additive 169 versus arm by victim creature-type mask, then
// SPELLMOD_CRIT_DAMAGE_BONUS on the bonus when it is non-negative but below
// the base damage (Unit.cpp:7438-7442; the C++ uint32 cast means a negative
// bonus skips the mod).
func (s *session) spellCriticalDamageBonus(ctx context.Context, spell wotlk.Spell, damage uint32, targetGUID uint64) uint32 {
	if s == nil || s.player == nil {
		return damage + damage/2
	}
	critBonus := int64(damage) + int64(damage)/2
	auraMult := s.critDamageAuraMultiplier(spell.SchoolMask)
	critMod := (auraMult - 1.0) * 100.0
	if versus := s.casterVersusAuras(); len(versus) > 0 {
		mask, _ := s.targetCreatureTypeMask(ctx, targetGUID)
		for _, a := range versus {
			if mask != 0 && uint32(a.misc)&mask != 0 {
				critMod += float64(a.amount)
			}
		}
	}
	if critBonus != 0 {
		// AddPct(int32 base, float pct): base += int32(float32(base) * pct / 100).
		critBonus += int64(float32(critBonus) * float32(critMod) / 100.0)
	}
	bonus := critBonus - int64(damage)
	if bonus >= 0 && bonus < int64(damage) {
		bonus = int64(s.applySpellMod(spell, spellModCritDamageBonus, int32(bonus)))
	}
	if total := int64(damage) + bonus; total > 0 {
		return uint32(total)
	}
	return 0
}

// spellWeaponCritDamageBonus mirrors the melee/ranged arm of
// Unit::CalculateSpellDamageTaken (Unit.cpp:1023-1040): for
// SPELL_DAMAGE_CLASS_MELEE/RANGED spells the crit bonus is the full damage
// again (100%), unlike the magic leg's SpellCriticalDamageBonus half. The
// SPELLMOD_CRIT_DAMAGE_BONUS lands on the bonus (no uint32-cast gate here —
// that gate belongs to SpellCriticalDamageBonus only), then the victim's 203
// (melee) / 204 (ranged) auras, the caster's 163 multiplier by school mask,
// and the caster's 169 sum by victim creature-type mask fold in as one
// percent add over the total (AddPct). Callers must have already gated on
// DefenseType 2/3 and no SPELL_ATTR4_FIXED_DAMAGE.
func (s *session) spellWeaponCritDamageBonus(ctx context.Context, spell wotlk.Spell, damage uint32, target combatTarget, isPlayerVictim bool) uint32 {
	if s == nil || s.player == nil {
		return damage + damage
	}
	critBonus := int64(s.applySpellMod(spell, spellModCritDamageBonus, int32(damage)))
	total := int64(damage) + critBonus
	var critPctDamageMod float64
	auraType := spellAuraModAttackerMeleeCritDmg
	if spellWeaponAttackType(spell) == protocol.RangedAttack {
		auraType = spellAuraModAttackerRangedCritDmg
	}
	if isPlayerVictim {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			for _, amount := range vicSess.auraTypeModifiersFiltered(auraType, func(int32) bool { return true }) {
				critPctDamageMod += float64(amount)
			}
		}
	} else {
		critPctDamageMod += float64(creatureAuraModifierSum(s.server, creatureAuraKeyForTarget(target), auraType))
	}
	critPctDamageMod += (s.critDamageAuraMultiplier(spell.SchoolMask) - 1.0) * 100.0
	if versus := s.casterVersusAuras(); len(versus) > 0 {
		mask, _ := s.targetCreatureTypeMask(ctx, target.GUID)
		for _, a := range versus {
			if mask != 0 && uint32(a.misc)&mask != 0 {
				critPctDamageMod += float64(a.amount)
			}
		}
	}
	if critPctDamageMod != 0 {
		// AddPct(int32 base, float pct): base += int32(float32(base) * pct / 100).
		total += int64(float32(total) * float32(critPctDamageMod) / 100.0)
	}
	if total < 0 {
		return 0
	}
	return uint32(total)
}

// criticalHealingAmountMultiplier mirrors the GetTotalAuraMultiplier
// (SPELL_AURA_MOD_CRITICAL_HEALING_AMOUNT) tail of
// Unit::SpellCriticalHealingBonus (Unit.cpp:7477): every 50 aura multiplies,
// with no misc-mask gate.
func (s *session) criticalHealingAmountMultiplier() float64 {
	mult := 1.0
	if s == nil || s.player == nil {
		return mult
	}
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModCriticalHealingAmount {
			continue
		}
		mult *= 1.0 + float64(int32(aura.Amount))/100.0
	}
	s.castMu.Unlock()
	return mult
}

// spellCriticalHealingBonus mirrors Unit::SpellCriticalHealingBonus
// (Unit.cpp:7453-7479): base 50% bonus (default DmgClass; no Go DmgClass
// model), the 169 versus arm multiplicative on the BONUS half by victim
// creature-type mask, then the 50 (MOD_CRITICAL_HEALING_AMOUNT) product over
// the whole. The 163/talent/meta damage arms do NOT apply to heals.
func (s *session) spellCriticalHealingBonus(ctx context.Context, heal uint32, targetGUID uint64) uint32 {
	if s == nil || s.player == nil {
		return heal + heal/2
	}
	critBonus := int64(heal) / 2
	if versus := s.casterVersusAuras(); len(versus) > 0 && targetGUID != 0 {
		mask, _ := s.targetCreatureTypeMask(ctx, targetGUID)
		versusMult := 1.0
		for _, a := range versus {
			if mask != 0 && uint32(a.misc)&mask != 0 {
				versusMult *= 1.0 + float64(a.amount)/100.0
			}
		}
		if versusMult != 1.0 {
			critBonus = int64(float64(critBonus) * versusMult)
		}
	}
	damage := int64(heal)
	if critBonus > 0 {
		damage += critBonus
	}
	// int32(float(damage) * multiplier), truncation toward zero.
	damage = int64(float32(damage) * float32(s.criticalHealingAmountMultiplier()))
	if damage < 0 {
		return 0
	}
	return uint32(damage)
}
