package world

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// ReactiveType indexes (Unit.h:709-717); REACTIVE_TIMER_START is 4000ms
// (Unit.h:707).
const (
	reactiveDefense       = 0 // REACTIVE_DEFENSE
	reactiveHunterParry   = 1 // REACTIVE_HUNTER_PARRY
	reactiveOverpower     = 2 // REACTIVE_OVERPOWER
	reactiveWolverineBite = 3 // REACTIVE_WOLVERINE_BITE
)

const reactiveTimerStart = 4000 * time.Millisecond // REACTIVE_TIMER_START (Unit.h:707)

// startReactiveTimer mirrors Unit::StartReactiveTimer (Unit.h:1602): the
// reactive's 4000ms window opens now.
func (s *session) startReactiveTimer(reactive int) {
	if s == nil || s.player == nil || reactive < 0 || reactive >= len(s.reactiveUntil) {
		return
	}
	s.reactiveUntil[reactive] = time.Now().Add(reactiveTimerStart)
}

// clearReactive mirrors Unit::ClearReactive (Unit.cpp:10729-10752): the
// reactive's timer drops; DEFENSE drops AURA_STATE_DEFENSE and
// HUNTER_PARRY drops AURA_STATE_HUNTER_PARRY for hunters (both bits are
// timer-derived in unitAuraStateMask, so zeroing the timer is the
// clear); OVERPOWER clears the warrior's combo points with no
// TYPEID_PLAYER gate — C++ only gates the UpdateReactives expiry arm
// that way, not this one. REACTIVE_WOLVERINE_BITE is an explicit
// no-op in C++ (the //TODO at :10750), so it has no arm here either.
// No core caller exists yet (the only in-tree caller is npcbot, out of
// scope); the sweep below is the live consumer of the same arms.
func (s *session) clearReactive(reactive int) {
	if s == nil || s.player == nil || reactive < 0 || reactive >= len(s.reactiveUntil) {
		return
	}
	s.reactiveUntil[reactive] = time.Time{}
	if reactive == reactiveOverpower && s.player.Class == 1 { // CLASS_WARRIOR
		s.clearSessionComboPoints()
	}
}

// sweepReactiveTimers mirrors Unit::UpdateReactives (Unit.cpp:10769-10800):
// expired reactives drop. REACTIVE_OVERPOWER expiry clears the warrior's
// combo points (ClearReactive(REACTIVE_OVERPOWER) -> ClearComboPoints,
// Unit.cpp:10743-10745, including the RETAIN_COMBO_POINTS aura strip that
// ClearComboPoints performs, Unit.cpp:10674-10680); the defense and
// hunter-parry arms only drop their aura-state bits. REACTIVE_WOLVERINE_BITE
// expiry clears a hunter pet's combo points (Unit.cpp:10794-10797) —
// Go has no per-motion reactive timer to drive it, so that leg stays
// unbridged (see clearHunterPetReactives); a player is never
// IsHunterPet(), so the player sweep is exact without it. The npcbot
// attacker arms ride the same timers; Go has no npcbot infrastructure
// (standing no-infra).
func (s *session) sweepReactiveTimers() {
	if s == nil || s.player == nil {
		return
	}
	now := time.Now()
	for i := range s.reactiveUntil {
		if s.reactiveUntil[i].IsZero() || now.Before(s.reactiveUntil[i]) {
			continue
		}
		s.reactiveUntil[i] = time.Time{}
		if i == reactiveOverpower && s.player.Class == 1 { // CLASS_WARRIOR
			s.removeAurasByType(spellAuraRetainComboPoints)
			s.clearSessionComboPoints()
		}
	}
}

// clearAllReactives mirrors Unit::ClearAllReactives (Unit.cpp:10754-10767):
// every reactive timer drops; the overpower arm keeps its combo-point
// clearing for warriors. Called on JUST_DIED (Unit.cpp:8927) and from
// Player::CleanupsBeforeDelete (Player.cpp:26311). The hunter-pet
// combo-point arm (IsHunterPet -> ClearComboPoints, :10766-10767) rides
// clearHunterPetReactives at the pet-death hook (kill.go), not here —
// this session method only ever runs for the player.
func (s *session) clearAllReactives() {
	if s == nil || s.player == nil {
		return
	}
	for i := range s.reactiveUntil {
		s.reactiveUntil[i] = time.Time{}
	}
	if s.player.Class == 1 { // CLASS_WARRIOR
		s.removeAurasByType(spellAuraRetainComboPoints)
		s.clearSessionComboPoints()
	}
}

// clearHunterPetReactives is the hunter-pet arm shared by
// Unit::ClearAllReactives (Unit.cpp:10766-10767, IsHunterPet ->
// ClearComboPoints) and the REACTIVE_WOLVERINE_BITE expiry arm of
// Unit::UpdateReactives (Unit.cpp:10794-10797). Creature DEFENSE /
// HUNTER_PARRY arms are vacuous — creatures carry no reactive timers
// in Go — so only the combo-point clear is bridged, via the
// clearCreatureComboPoints packet leg (boss_ai.go).
func (s *Server) clearHunterPetReactives(m *creatureMotion) {
	if s == nil || m == nil || m.PetType != petTypeHunter {
		return
	}
	s.clearCreatureComboPoints(m)
}

// procReactiveVictimArms mirrors the victim half of
// Unit::ProcSkillsAndReactives (Unit.cpp:10283-10324): a dodged swing arms
// AURA_STATE_DEFENSE (rogues skip it — Riposte), a parried swing arms
// AURA_STATE_HUNTER_PARRY for hunters and AURA_STATE_DEFENSE for everyone
// else, and a blocked swing arms AURA_STATE_DEFENSE. Each state lives for
// the 4000ms reactive window and gates Revenge/Counterattack-style
// CasterAuraState checks.
func (s *session) procReactiveVictimArms(outcome protocol.MeleeHitOutcome) {
	if s == nil || s.player == nil {
		return
	}
	switch outcome {
	case protocol.MeleeHitDodge:
		if s.player.Class == 4 { // CLASS_ROGUE: Riposte skip (Unit.cpp:10290)
			return
		}
		s.startReactiveTimer(reactiveDefense)
	case protocol.MeleeHitParry:
		if s.player.Class == 3 { // CLASS_HUNTER: Counterattack (Unit.cpp:10301-10304)
			s.startReactiveTimer(reactiveHunterParry)
		} else {
			s.startReactiveTimer(reactiveDefense)
		}
	case protocol.MeleeHitBlock:
		s.startReactiveTimer(reactiveDefense)
	}
}

// procReactiveAttackerArms mirrors the attacker half of
// Unit::ProcSkillsAndReactives (Unit.cpp:10326-10351): a warrior whose
// swing is dodged banks a combo point on the dodger and opens the 4000ms
// Overpower window. The hunter-pet crit arm (REACTIVE_WOLVERINE_BITE +
// pet combo points, Unit.cpp:10345-10351) has no Go pet-reactive timer
// model to arm, so it stays unbridged; its expiry leg is
// clearHunterPetReactives, driven by the pet-death hook until a
// per-motion timer exists.
func (s *session) procReactiveAttackerArms(targetGUID uint64, outcome protocol.MeleeHitOutcome) {
	if s == nil || s.player == nil || s.player.Class != 1 { // CLASS_WARRIOR
		return
	}
	if outcome != protocol.MeleeHitDodge || targetGUID == 0 {
		return
	}
	s.addSessionComboPoints(targetGUID, 1)
	s.startReactiveTimer(reactiveOverpower)
}

// Weapon skill ids (SharedDefines.h:2895-2990) for the UpdateCombatSkills
// weapon-skill arm.
const (
	skillSwords        uint32 = 43  // SKILL_SWORDS
	skillAxes          uint32 = 44  // SKILL_AXES
	skillBows          uint32 = 45  // SKILL_BOWS
	skillGuns          uint32 = 46  // SKILL_GUNS
	skillMaces         uint32 = 54  // SKILL_MACES
	skillTwoHandSwords uint32 = 55  // SKILL_2H_SWORDS
	skillDefense       uint32 = 95  // SKILL_DEFENSE
	skillStaves        uint32 = 136 // SKILL_STAVES
	skillTwoHandMaces  uint32 = 160 // SKILL_2H_MACES
	skillUnarmed       uint32 = 162 // SKILL_UNARMED
	skillTwoHandAxes   uint32 = 172 // SKILL_2H_AXES
	skillDaggers       uint32 = 173 // SKILL_DAGGERS
	skillThrown        uint32 = 176 // SKILL_THROWN
	skillCrossbows     uint32 = 226 // SKILL_CROSSBOWS
	skillWands         uint32 = 228 // SKILL_WANDS
	skillPolearms      uint32 = 229 // SKILL_POLEARMS
	skillAssassination uint32 = 253 // SKILL_ASSASSINATION
	skillFistWeapons   uint32 = 473 // SKILL_FIST_WEAPONS
	// skillFishing (356) already lives in spells.go's skill-id block.
)

// creatureFlagExtraNoSkillgain mirrors CREATURE_FLAG_EXTRA_NO_SKILLGAIN
// (CreatureData.h:55): the creature never raises weapon skills.
const creatureFlagExtraNoSkillgain uint32 = 0x00040000

// weaponSkillForSubClass mirrors ItemTemplate::GetSkill
// (ItemTemplate.cpp:106-139): the weapon-skill id for an item subclass,
// 0 when the subclass trains no skill.
func weaponSkillForSubClass(subClass uint32) uint32 {
	skills := [...]uint32{
		skillAxes, skillTwoHandAxes, skillBows, skillGuns, skillMaces,
		skillTwoHandMaces, skillPolearms, skillSwords, skillTwoHandSwords, 0,
		skillStaves, 0, 0, skillFistWeapons, 0,
		skillDaggers, skillThrown, skillAssassination, skillCrossbows, skillWands,
		skillFishing,
	}
	if subClass >= uint32(len(skills)) {
		return 0
	}
	return skills[subClass]
}

// weaponSkillIDsForAttack mirrors Player::GetBaseWeaponSkillValue +
// UpdateWeaponSkill (Player.cpp:24187-24198, 5951-5988): the skill ids a
// swing can raise for attType, first entry first for the chance roll.
// An unarmed main-hand swing raises unarmed and fist weapons together; a
// fist weapon raises unarmed, then its own skill (the fallthrough in
// UpdateWeaponSkill) — the slice still leads with the weapon's own skill
// because the chance roll reads GetBaseWeaponSkillValue, and the two +1
// gains are independent, so their order is inert. Other weapons raise
// only their own skill. A missing off-hand or ranged weapon raises
// nothing (GetBaseWeaponSkillValue returns 0 and UpdateWeaponSkill finds
// no item).
func (s *session) weaponSkillIDsForAttack(ctx context.Context, attType protocol.WeaponAttackType) []uint32 {
	if s == nil || s.server == nil {
		return nil
	}
	slot := uint8(15) // EQUIPMENT_SLOT_MAINHAND
	if attType == protocol.OffAttack {
		slot = 16 // EQUIPMENT_SLOT_OFFHAND
	} else if attType == protocol.RangedAttack {
		slot = 17 // EQUIPMENT_SLOT_RANGED
	}
	_, entry := s.equippedWeaponInstance(ctx, slot)
	if entry == 0 {
		if attType == protocol.BaseAttack {
			return []uint32{skillUnarmed, skillFistWeapons}
		}
		return nil
	}
	tmpl, ok := s.server.getItemStoreTemplateInfo(ctx, entry)
	if !ok {
		return nil
	}
	skillID := weaponSkillForSubClass(tmpl.SubClass)
	if skillID == 0 {
		return nil
	}
	if tmpl.SubClass == itemSubclassWeaponFist {
		return []uint32{skillID, skillUnarmed}
	}
	return []uint32{skillID}
}

// updateCombatSkills mirrors Player::UpdateCombatSkills
// (Player.cpp:5989-6022): the weapon-skill roll (defense=false) or the
// defense roll (defense=true) after a melee/ranged swing against a
// creature. victimLevel is the other unit's level for target
// (GetLevelForTarget); attType selects the weapon skill. The caller
// enforces the ProcSkillsAndReactives gates (player attacker/victim,
// non-player target, not a pet or player-owned totem) and the hit-mask
// arms. One roll gates all gains (UpdateWeaponSkill's gains are
// unconditional once the roll passes); a skill already at max gains
// nothing (Player::UpdateSkill, Player.cpp:5743-5758). GetBaseSkillValue
// is the stored value — Go has no skill-bonus model.
func (s *session) updateCombatSkills(ctx context.Context, victimLevel uint32, attType protocol.WeaponAttackType, defense bool) {
	if s == nil || s.player == nil || s.player.Level == 0 {
		return
	}
	plevel := uint32(s.player.Level)
	greyLevel := grayLevel(plevel)
	mobLevel := victimLevel
	if mobLevel > plevel+5 {
		mobLevel = plevel + 5
	}
	// C++ stores the difference in a uint8 (Player.cpp:6002-6004), so a
	// victim below the grey level wraps (moblevel - greylevel mod 256)
	// instead of clamping to a small difference.
	lvlDif := int32(uint8(mobLevel - greyLevel))
	if lvlDif < 3 {
		lvlDif = 3
	}
	var skillIDs []uint32
	if defense {
		skillIDs = []uint32{skillDefense}
	} else {
		// UpdateWeaponSkill (Player.cpp:5951-5955): feral and tree
		// forms never raise weapon skills.
		if s.player.isInFeralForm() || s.player.ShapeshiftForm == 2 { // FORM_TREE
			return
		}
		skillIDs = s.weaponSkillIDsForAttack(ctx, attType)
	}
	if len(skillIDs) == 0 {
		return
	}
	baseSkill := s.getSkillValue(skillIDs[0])
	skillDif := int32(5*plevel) - int32(baseSkill)
	if skillDif <= 0 {
		return
	}
	chance := float64(3*lvlDif*skillDif) / float64(plevel)
	if !defense && (s.player.Class == 1 || s.player.Class == 4) { // CLASS_WARRIOR / CLASS_ROGUE
		chance += chance * 0.02 * float64(s.player.Stats[3]) // STAT_INTELLECT
	}
	if chance < 1.0 {
		chance = 1.0
	}
	if rand.IntN(1000)+1 > int(chance*10) {
		return
	}
	for _, skillID := range skillIDs {
		s.updateSkillPro(ctx, skillID, 1000, 1)
	}
}

// procMeleeSkillGain applies the attacker-side skill arm of
// Unit::ProcSkillsAndReactives (Unit.cpp:10260-10279) for a player melee
// or ranged swing: hitMask NORMAL|MISS|FULL_RESIST, victim a creature
// (not a pet, not a player-owned totem, not a critter, without
// CREATURE_FLAG_EXTRA_NO_SKILLGAIN).
func (s *session) procMeleeSkillGain(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType, hitMask uint32) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	if hitMask&(procHitNormal|procHitMiss|procHitFullResist) == 0 {
		return
	}
	motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID)
	if motion == nil || motionIsPet(motion) || motion.CreatureType == creatureTypeCritter || motion.FlagsExtra&creatureFlagExtraNoSkillgain != 0 {
		return
	}
	if motion.IsTotem && motion.OwnerGUID != 0 {
		if owner := s.server.findSessionByGUID(motion.OwnerGUID); owner != nil {
			return
		}
	}
	s.updateCombatSkills(ctx, uint32(target.Level), attType, false)
}

// procVictimSkillGain applies the victim-side defense arm of
// Unit::ProcSkillsAndReactives (Unit.cpp:10260-10279): a player victim of
// a creature swing raises defense on a normal, missed or fully-resisted
// swing (not from a critter — the inner gate covers the whole normal arm)
// or on a dodged, parried or blocked swing (no critter gate on that arm).
// attackerLevel is the attacker's level for target; attType is the swing's
// attack type (ignored by the defense roll, as in C++).
func (s *session) procVictimSkillGain(ctx context.Context, attackerLevel uint32, attType protocol.WeaponAttackType, hitMask uint32, attackerIsCritter bool) {
	if s == nil || s.player == nil {
		return
	}
	if hitMask&(procHitNormal|procHitMiss|procHitFullResist) != 0 {
		if attackerIsCritter {
			return
		}
	} else if hitMask&(procHitDodge|procHitParry|procHitBlock) == 0 {
		return
	}
	s.updateCombatSkills(ctx, attackerLevel, attType, true)
}

// AuraStateType values (SharedDefines.h:1320-1354).
const (
	auraStateNone               = 0
	auraStateDefense            = 1
	auraStateHealthless20Pct    = 2
	auraStateBerserking         = 3
	auraStateFrozen             = 4
	auraStateJudgement          = 5
	auraStateHunterParry        = 7
	auraStateWarriorVictoryRush = 10
	auraStateFaerieFire         = 12
	auraStateHealthless35Pct    = 13
	auraStateConflagrate        = 14
	auraStateSwiftmend          = 15
	auraStateDeadlyPoison       = 16
	auraStateEnrage             = 17
	auraStateBleeding           = 18
	auraStateUnknown19          = 19
	auraStateUnknown22          = 22
	auraStateHealthAbove75Pct   = 23
)

// perCasterAuraStateMask mirrors PER_CASTER_AURA_STATE_MASK (SharedDefines.h:
// 1356-1357). These states are tracked per aura caster, not as a unit-wide
// flag (Unit::HasAuraState, Unit.cpp:5946).
const perCasterAuraStateMask = uint32(1<<(auraStateConflagrate-1)) | uint32(1<<(auraStateDeadlyPoison-1))

// auraStateHealthBits covers the states Unit::Update re-derives from
// health every tick (Unit.cpp:467-469), so a debug-set bit on one of
// them lives at most a tick in C++ and is never stored by the override.
const auraStateHealthBits = uint32(1<<(auraStateHealthless20Pct-1)) | uint32(1<<(auraStateHealthless35Pct-1)) | uint32(1<<(auraStateHealthAbove75Pct-1))

// Spell family names used by _LoadAuraState (SharedDefines.h:3585-3589)
// plus SPELLFAMILY_POTION (SharedDefines.h:3594) for the EffectEnergize
// power-type gate.
const (
	spellFamilyWarrior = 4
	spellFamilyWarlock = 5
	spellFamilyDruid   = 7
	spellFamilyRogue   = 8
	spellFamilyPotion  = 13
)

// cloneCasterGUIDForUnit is the per-unit aura-index lookup for the mirror
// image handler: it returns the caster GUID of the target unit's live
// SPELL_AURA_CLONE_CASTER aura (the aura does not stack in C++, so the first
// match is the GetAuraEffectsByType(...).front() analog).
// Reference: WorldSession::HandleMirrorImageDataRequest (SpellHandler.cpp:740-745).
// Player targets resolve through the session's activeAuras under its castMu
// (the dispel.go locking pattern); creature and pet targets consult the
// server's activeCreatureAuras index under auraMu. A zero caster GUID is
// the C++ !GetCaster() return. Unresolvable GUIDs yield no match, like the
// C++ !unit return.
func (s *session) cloneCasterGUIDForUnit(targetGUID uint64) (uint64, bool) {
	if s == nil || s.server == nil || targetGUID == 0 {
		return 0, false
	}
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		for _, aura := range ts.activeAuras {
			if aura == nil || aura.Stopped || aura.AuraType != spellAuraCloneCaster {
				continue
			}
			casterGUID := aura.CasterGUID
			ts.castMu.Unlock()
			if casterGUID == 0 {
				return 0, false
			}
			return casterGUID, true
		}
		ts.castMu.Unlock()
		return 0, false
	}
	if s.player == nil {
		return 0, false
	}
	s.server.auraMu.Lock()
	defer s.server.auraMu.Unlock()
	for _, aura := range s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, targetGUID)] {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraCloneCaster {
			continue
		}
		if aura.CasterGUID == 0 {
			return 0, false
		}
		return aura.CasterGUID, true
	}
	return 0, false
}

// spellAuraAbilityIgnoreAuraState is SPELL_AURA_ABILITY_IGNORE_AURASTATE
// (SpellAuraDefines.h:342): a caster-side aura effect that makes
// Unit::HasAuraState succeed for spells its affect mask covers
// (Unit.cpp:5946-5954).
const spellAuraAbilityIgnoreAuraState = 262

func spellAuraState(spell wotlk.Spell) uint32 {
	// Seals (SpellInfo.cpp:1971; classifier takes nil — the seal branch
	// never resolves the first-rank chain).
	if spellSpecific(spell, nil) == spellSpecificSeal {
		return auraStateJudgement
	}
	// Conflagrate aura state on Immolate and Shadowflame
	if spell.SpellFamilyName == spellFamilyWarlock &&
		(spell.SpellFamilyFlags[0]&4 != 0 || spell.SpellFamilyFlags[2]&2 != 0) {
		return auraStateConflagrate
	}
	// Faerie Fire (druid versions)
	if spell.SpellFamilyName == spellFamilyDruid && spell.SpellFamilyFlags[0]&0x400 != 0 {
		return auraStateFaerieFire
	}
	// Sting (hunter's pet ability, SpellInfo.cpp:1986-1988)
	if spell.Category == 1133 {
		return auraStateFaerieFire
	}
	// Victorious
	if spell.SpellFamilyName == spellFamilyWarrior && spell.SpellFamilyFlags[1]&0x00040000 != 0 {
		return auraStateWarriorVictoryRush
	}
	// Swiftmend state on Regrowth & Rejuvenation
	if spell.SpellFamilyName == spellFamilyDruid && spell.SpellFamilyFlags[0]&0x50 != 0 {
		return auraStateSwiftmend
	}
	// Deadly poison aura state
	if spell.SpellFamilyName == spellFamilyRogue && spell.SpellFamilyFlags[0]&0x10000 != 0 {
		return auraStateDeadlyPoison
	}
	// Enrage aura state
	if spell.DispelType == DispelEnrage {
		return auraStateEnrage
	}
	// Bleeding aura state
	for _, effect := range spell.Effects {
		if effect.Mechanic == 15 { // MECHANIC_BLEED
			return auraStateBleeding
		}
	}
	// Frozen: frost school aura that stuns or roots
	if spell.SchoolMask&16 != 0 { // SPELL_SCHOOL_MASK_FROST
		for _, effect := range spell.Effects {
			if effect.Effect == 6 && (effect.Aura == spellAuraStun || effect.Aura == spellAuraRoot) { // SPELL_EFFECT_APPLY_AURA
				return auraStateFrozen
			}
		}
	}
	switch spell.ID {
	case 71465, 50241: // Divine Surge, Evasive Charges
		return auraStateUnknown22
	case 9991, 35325, 35328, 35329, 35331, 49163: // Touch of Zanzil, blood auras, Perpetual Instability
		return auraStateFaerieFire
	}
	return auraStateNone
}

// auraStateHealthMask mirrors the health-threshold ModifyAuraState updates
// (Unit.cpp:467-469): AURA_STATE_HEALTHLESS_20_PERCENT, _35_PERCENT and
// HEALTH_ABOVE_75_PERCENT. Comparisons use the floored pct of max health,
// like Unit::HealthBelowPct/HealthAbovePct (Unit.h:872-874).
func auraStateHealthMask(health, maxHealth uint32) uint32 {
	if maxHealth == 0 {
		return 0
	}
	h, m := uint64(health), uint64(maxHealth)
	var mask uint32
	if h < m*20/100 {
		mask |= uint32(1) << (auraStateHealthless20Pct - 1)
	}
	if h < m*35/100 {
		mask |= uint32(1) << (auraStateHealthless35Pct - 1)
	}
	if h > m*75/100 {
		mask |= uint32(1) << (auraStateHealthAbove75Pct - 1)
	}
	return mask
}

// lookupAuraState resolves a spell's aura state through the DBC store,
// returning auraStateNone when the spell is unknown.
func (s *session) lookupAuraState(spellID uint32) uint32 {
	if s.server == nil || s.server.Data == nil {
		return auraStateNone
	}
	if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
		return spellAuraState(spell)
	}
	return auraStateNone
}

// unitAuraStateMask builds the unit-wide aura state mask (health thresholds
// plus one bit per active aura's state), mirroring UNIT_FIELD_AURASTATE as
// maintained by Unit::ModifyAuraState (Unit.cpp:5880). The reactive
// defense/hunter-parry bits ride the ProcSkillsAndReactives timers
// (Unit.cpp:10283-10324); expired timers are swept first, mirroring
// Unit::UpdateReactives (Unit.cpp:10769).
func (s *session) unitAuraStateMask() uint32 {
	s.sweepReactiveTimers()
	mask := auraStateHealthMask(s.player.Health, s.player.MaxHealth)
	now := time.Now()
	if !s.reactiveUntil[reactiveDefense].IsZero() && now.Before(s.reactiveUntil[reactiveDefense]) {
		mask |= uint32(1) << (auraStateDefense - 1)
	}
	if !s.reactiveUntil[reactiveHunterParry].IsZero() && now.Before(s.reactiveUntil[reactiveHunterParry]) {
		mask |= uint32(1) << (auraStateHunterParry - 1)
	}
	for spellID := range s.auras {
		if state := s.lookupAuraState(spellID); state != auraStateNone {
			mask |= uint32(1) << (state - 1)
		}
	}
	mask |= s.debugAuraStateMask &^ auraStateHealthBits
	return mask
}

// hasAuraState mirrors Unit::HasAuraState(flag, spellProto, caster) for the
// casting unit itself, used by the caster-state block of Spell::CheckCast
// (Spell.cpp:5298-5308). Per-caster states are only visible from auras
// applied by this caster (Unit.cpp:5957-5965). The
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass runs first, like C++
// (Unit.cpp:5946-5954): a matching caster aura makes every state query
// succeed, including the Exclude (Not) terms.
func (s *session) hasAuraState(state uint32, spell wotlk.Spell) bool {
	if state == auraStateNone || s.player == nil {
		return false
	}
	if s.casterIgnoresAuraState(spell) {
		return true
	}
	if uint32(1)<<(state-1)&perCasterAuraStateMask != 0 {
		for _, aura := range s.loadedAuras() {
			if aura == nil || aura.CasterGUID != s.playerGUID {
				continue
			}
			if s.lookupAuraState(aura.SpellID) == state {
				return true
			}
		}
		return false
	}
	return s.unitAuraStateMask()&(uint32(1)<<(state-1)) != 0
}

// targetHasAuraState mirrors Unit::HasAuraState for the spell target, used by
// SpellInfo::CheckTarget (SpellInfo.cpp:1760-1766). Player targets resolve to
// their session's aura set; creature targets consult the server creature aura
// maps (same read pattern as targetHasAura). Per-caster states need an aura
// from the casting session; the creatureAuras presence map carries no caster
// GUID, so those only match through activeCreatureAuras (noted gap). The
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass reads the caster's own auras
// (Unit.cpp:5946-5954), so it runs before the target-kind dispatch below.
// targetMaxHealth resolves a unit GUID to its live effective max health:
// the session player (target 0 is self, like the apply funnel's own
// self-target fold), another online player, or a creature motion. 0 when
// the target cannot be resolved — the caller keeps the DBC amount.
func (s *session) targetMaxHealth(targetGUID uint64) uint32 {
	if targetGUID == 0 || targetGUID == s.playerGUID {
		if s.player != nil {
			return s.player.MaxHealth
		}
		return 0
	}
	if s.server == nil {
		return 0
	}
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		return targetSess.player.MaxHealth
	}
	if motion := s.findCreatureMotion(targetGUID); motion != nil {
		return motion.MaxHealth
	}
	return 0
}

// targetMaxMana resolves a unit GUID to its live effective max mana
// (POWER_MANA, power index 0): the session player, another online player,
// or a creature motion. 0 when the target cannot be resolved or has no
// mana pool — the replenishment leg's Load() gate reads this 0 as
// "owner power type is not MANA, keep the DBC base".
func (s *session) targetMaxMana(targetGUID uint64) uint32 {
	if targetGUID == 0 || targetGUID == s.playerGUID {
		if s.player != nil {
			return s.player.MaxPowers[0]
		}
		return 0
	}
	if s.server == nil {
		return 0
	}
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		return targetSess.player.MaxPowers[0]
	}
	if motion := s.findCreatureMotion(targetGUID); motion != nil {
		return motion.MaxPowers[0]
	}
	return 0
}

func (s *session) targetHasAuraState(ctx context.Context, targetGUID uint64, state uint32, spell wotlk.Spell) bool {
	if state == auraStateNone || targetGUID == 0 {
		return false
	}
	// The bypass reads the caster's auras, not the target's (Unit.cpp:5952),
	// so it applies on every target kind.
	if s.casterIgnoresAuraState(spell) {
		return true
	}
	if s.player != nil && targetGUID == s.playerGUID {
		return s.hasAuraState(state, spell)
	}
	bit := uint32(1) << (state - 1)
	perCaster := bit&perCasterAuraStateMask != 0
	if s.server == nil {
		return false
	}
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		if perCaster {
			for _, aura := range targetSess.loadedAuras() {
				if aura == nil || aura.CasterGUID != s.playerGUID {
					continue
				}
				if s.lookupAuraState(aura.SpellID) == state {
					return true
				}
			}
			return false
		}
		return targetSess.unitAuraStateMask()&bit != 0
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		return false
	}
	type auraCaster struct {
		spellID    uint32
		casterGUID uint64
		attributed bool
	}
	var found []auraCaster
	key := creatureAuraKeyForTarget(target)
	s.server.auraMu.Lock()
	for spellID := range s.server.creatureAuras[key] {
		found = append(found, auraCaster{spellID: spellID})
	}
	for spellID, aura := range s.server.activeCreatureAuras[key] {
		entry := auraCaster{spellID: spellID, attributed: true}
		if aura != nil {
			entry.casterGUID = aura.CasterGUID
		}
		found = append(found, entry)
	}
	s.server.auraMu.Unlock()
	mask := auraStateHealthMask(target.Health, target.MaxHealth)
	for _, aura := range found {
		if s.lookupAuraState(aura.spellID) != state {
			continue
		}
		if perCaster && (!aura.attributed || aura.casterGUID != s.playerGUID) {
			continue
		}
		mask |= bit
	}
	return mask&bit != 0
}

// creatureAuraStateMask mirrors the creature-target branch of
// targetHasAuraState for a tick whose target is already resolved: the
// health mask plus spell-granted states from the server creature aura
// maps. C++ Unit::HasAuraState(flag) with no spellProto/caster skips the
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass (Unit.cpp:5946-5954) and the
// per-caster filter, so those legs are omitted here.
func (s *session) creatureAuraStateMask(key creatureAuraKey, target combatTarget) uint32 {
	mask := auraStateHealthMask(target.Health, target.MaxHealth)
	if s.server == nil {
		return mask
	}
	s.server.auraMu.Lock()
	spellIDs := make([]uint32, 0, len(s.server.creatureAuras[key])+len(s.server.activeCreatureAuras[key]))
	for spellID := range s.server.creatureAuras[key] {
		spellIDs = append(spellIDs, spellID)
	}
	for spellID := range s.server.activeCreatureAuras[key] {
		spellIDs = append(spellIDs, spellID)
	}
	s.server.auraMu.Unlock()
	for _, spellID := range spellIDs {
		if state := s.lookupAuraState(spellID); state != auraStateNone {
			mask |= uint32(1) << (state - 1)
		}
	}
	if s.server != nil {
		s.server.motionMu.Lock()
		for scope := range s.server.instanceCreatureMotion {
			if motion := s.server.findCreatureMotionLocked(scope.MapID, scope.InstanceID, key.GUID); motion != nil {
				mask |= motion.DebugAuraStateMask &^ auraStateHealthBits
				break
			}
		}
		s.server.motionMu.Unlock()
	}
	return mask
}

// spellFamilyBlockedByAura mirrors the SPELL_AURA_BLOCK_SPELL_FAMILY leg of
// Spell::CheckCast (Spell.cpp:5277-5278): a caster carrying a
// BLOCK_SPELL_FAMILY aura effect whose MiscValue matches the spell's
// SpellFamilyName cannot cast it. MiscValue resolves from the DBC effect
// row, not the record's single MiscValue, and the gate is EffectMask-gated
// — the session twin of checkCreatureSpellFamilyBlocked (boss_ai.go).
func (s *session) spellFamilyBlockedByAura(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraBlockSpellFamily || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if uint32(effect.MiscValue) == spell.SpellFamilyName {
				return true
			}
		}
	}
	return false
}

// casterIgnoresAuraState reports whether the casting session carries an aura
// effect of type SPELL_AURA_ABILITY_IGNORE_AURASTATE whose spell affects the
// spell being cast, mirroring the bypass term at the top of
// Unit::HasAuraState (Unit.cpp:5946-5954) via HasAuraTypeWithAffectMask
// (Unit.cpp:4689-4696).
func (s *session) casterIgnoresAuraState(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	data := s.server.Data
	return auraStateBypassApplies(s.loadedAuras(), func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, -1)
}

// auraStateBypassApplies is the data-free core of casterIgnoresAuraState: true
// when any live aura grants a 262 effect (masked in by EffectMask, the
// per-effect merge accumulator) whose spell-family affect mask covers the
// spell being cast (AuraEffect::IsAffectedOnSpell, SpellAuraEffects.cpp:848).
// miscValue < 0 disables the MiscValue filter; otherwise only effects whose
// MiscValue equals it count (AuraEffect::GetMiscValue reads the granting
// spell's effect row: m_spellInfo->Effects[m_effIndex].MiscValue).
func auraStateBypassApplies(auras []*activeAura, grantingSpell func(uint32) (wotlk.Spell, bool), spell wotlk.Spell, miscValue int32) bool {
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, ok := grantingSpell(aura.SpellID)
		if !ok {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraAbilityIgnoreAuraState || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if miscValue >= 0 && effect.MiscValue != miscValue {
				continue
			}
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				return true
			}
		}
	}
	return false
}

// auraStateReqCombatExempt mirrors the reqCombat leg of Spell::CheckCast
// (Spell.cpp:5280-5292): a SPELL_AURA_ABILITY_IGNORE_AURASTATE (262)
// effect affecting the spell whose MiscValue is 1 lifts the in-combat
// CanBeUsedInCombat gate (the SPELL_FAILED_AFFECTING_COMBAT arm of the
// caster-state block, Spell.cpp:5311-5312).
func (s *session) auraStateReqCombatExempt(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	data := s.server.Data
	return auraStateBypassApplies(s.loadedAuras(), func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, 1)
}
