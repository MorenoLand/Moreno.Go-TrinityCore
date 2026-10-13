package world

import (
	"context"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func PetLevelForOwner(petType uint8, petLevel, ownerLevel uint32) uint32 {
	switch petType {
	case 0:
		return ownerLevel
	case 1:
		if petLevel > ownerLevel {
			return ownerLevel
		}
		if ownerLevel > 5 && petLevel+5 < ownerLevel {
			return ownerLevel - 5
		}
	}
	return petLevel
}

func AdvanceHunterPetExperience(level, currentXP, earnedXP, maxLevel, currentNextLevelXP uint32, xpForLevel []uint32) (uint32, uint32, uint32) {
	if level == 0 || earnedXP == 0 || level >= maxLevel {
		return level, currentXP, currentNextLevelXP
	}
	xp := currentXP + earnedXP
	nextLevelXP := currentNextLevelXP
	for level < maxLevel && xp >= nextLevelXP {
		xp -= nextLevelXP
		level++
		if int(level) < len(xpForLevel) {
			nextLevelXP = xpForLevel[level] / 20
		} else {
			nextLevelXP = 0
		}
	}
	if level >= maxLevel {
		xp = 0
	}
	return level, xp, nextLevelXP
}

func PetFoodInDiet(foodType, foodMask uint32) bool {
	return foodType > 0 && foodType <= 32 && foodMask&(uint32(1)<<(foodType-1)) != 0
}

// petNativeObjectScale mirrors Pet::GetNativeObjectScale (Pet.cpp:1985-2001):
// hunter pets interpolate the CreatureFamily Min/MaxScale band by level;
// every other pet falls back to the creature_template scale
// (Guardian::GetNativeObjectScale, Creature.cpp:3194-3197).
func (s *session) petNativeObjectScale(ctx context.Context, entry, level uint32, petType uint8) float32 {
	scale := float32(1.0)
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || entry == 0 {
		return scale
	}
	wdb := s.server.WorldStore.DB
	var dbScale float64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(scale, 1) FROM creature_template WHERE entry = ?", entry).Scan(&dbScale); err == nil && dbScale > 0 {
		scale = float32(dbScale)
	}
	if petType != 1 || s.server.Data == nil {
		return scale
	}
	var family int64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(family, 0) FROM creature_template WHERE entry = ?", entry).Scan(&family); err != nil || family <= 0 {
		return scale
	}
	minScale, minLevel, maxScale, maxLevel, found, err := s.server.Data.CreatureFamilyScale(uint32(family))
	if err != nil || !found || minScale <= 0 {
		return scale
	}
	if level >= maxLevel {
		return maxScale
	}
	if level <= minLevel {
		return minScale
	}
	return minScale + float32(level-minLevel)/float32(maxLevel)*(maxScale-minScale)
}

// petBaseArmor returns the aura-free flat BASE_VALUE for a pet's armor
// (Pet::InitStatsForLevel, Pet.cpp:885, 903-905): the pet_levelstats armor
// (level*50 fallback). Guardian::UpdateArmor (StatSystem.cpp:1316-1330)
// multiplies only this flat base by BASE_PCT (aura-142) and adds agility*2,
// the TOTAL_VALUE flats (aura-22/83) and the 35% owner bonus after — so those
// legs must NOT be baked into the stored value. They fold live at
// damage/display time in petFullArmor, which resolves the pet's agility via
// petGuardianStat (create-stat base plus the live aura-29 flat / aura-137
// pct feeds) and the owner bonus from the owner's live armor
// (C++ recomputes it on every owner-armor change, StatSystem.cpp:283).
func (s *session) petBaseArmor(ctx context.Context, entry, level uint32, petType uint8) uint32 {
	armor := level * 50
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return armor
	}
	statEntry := entry
	if petType == 1 {
		statEntry = 1
	}
	if statEntry == 0 {
		return armor
	}
	var dbArmor int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT armor FROM pet_levelstats WHERE creature_entry = ? AND level = ?", statEntry, level).Scan(&dbArmor); err == nil && dbArmor > 0 {
		armor = uint32(dbArmor)
	}
	return armor
}

// petDisplayArmor folds the pet's live armor legs onto the stored flat base
// for display (unitFieldResistances): the full Guardian::UpdateArmor
// composition (StatSystem.cpp:1316-1330) via petFullArmor.
func (s *Server) petDisplayArmor(key creatureAuraKey, motion *creatureMotion) uint32 {
	return uint32(s.petFullArmor(key, motion))
}

// refreshPetStatsOnOwnerStatChange mirrors the pet arm of
// Player::UpdateStats (StatSystem.cpp:108-114): when the owner's stamina or
// intellect changes, Guardian::UpdateStats reruns for the pet — stamina
// feeds UpdateMaxHealth (StatSystem.cpp:1337-1357), intellect feeds
// UpdateMaxPower (StatSystem.cpp:1359-1379). The strength arm feeds
// UpdateAttackPowerAndDamage, which has no Go consumer, so it never
// triggers. The pet's current health and mana are preserved (C++
// UpdateMaxHealth/UpdateMaxPower never move the current value), clamped to
// the new maxima, and the recomputed armor/attributes/maxima broadcast to
// the instance like the level-up path.
func (s *session) refreshPetStatsOnOwnerStatChange(ctx context.Context) {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil {
		s.server.motionMu.Unlock()
		return
	}
	entry, level, petType, petID := motion.Entry, motion.Level, motion.PetType, motion.PetID
	petKey := creatureAuraKeyForMotion(motion)
	curHealth, curMana := motion.Health, motion.Mana
	s.server.motionMu.Unlock()
	_, maxHealth, _, maxMana := s.getPetStatsWithAuraFeed(ctx, motion, entry, level, petType, petID)
	attributes := s.getPetAttributes(ctx, entry, level, petType)
	armor := s.petBaseArmor(ctx, entry, level, petType)
	if maxHealth == 0 || maxMana == 0 {
		return
	}
	if curHealth > maxHealth {
		curHealth = maxHealth
	}
	if curMana > maxMana {
		curMana = maxMana
	}
	s.server.motionMu.Lock()
	if motion = s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID); motion == nil || motion.PetID != petID {
		s.server.motionMu.Unlock()
		return
	}
	motion.Health, motion.MaxHealth, motion.Mana, motion.MaxMana = curHealth, maxHealth, curMana, maxMana
	motion.Powers[0], motion.MaxPowers[0] = curMana, maxMana
	motion.Stats = attributes
	motion.Armor = armor
	motion.PetOwnerArmorBonus = petOwnerArmorValue(petType, s.player.Armor)
	fields := map[int]uint32{unitFieldHealth: curHealth, unitFieldMaxHealth: maxHealth, unitFieldPower1: curMana, unitFieldMaxPower1: maxMana, unitFieldResistances: s.server.petDisplayArmor(petKey, motion)}
	// Player::UpdateStats reruns Guardian::UpdateStats for the pet
	// (StatSystem.cpp:108-114), rewriting UNIT_FIELD_STAT0+index from the
	// aura-fed stats — at this point pet auras are live, so the aura-29/137
	// feed folds in via petGuardianStat.
	for index, value := range s.petGuardianFieldStats(motion, entry, petType, petID) {
		fields[unitFieldStat0+index] = value
	}
	mapID, instanceID, petGUID := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID, fields)
}

// rebroadcastPetArmorForOwner mirrors the pet arm of Player::UpdateArmor
// (StatSystem.cpp:281-283): whenever the owner's armor rebuilds, the pet's
// Guardian::UpdateArmor recomputes — its 35% owner-armor leg
// (StatSystem.cpp:1323-1324) reads the owner's live armor. Go's damage fold
// already reads it live via petOwnerArmorBonus, but the pet's
// UNIT_FIELD_RESISTANCES (armor) field must rebroadcast so the client sees
// the same SetArmor value. The stamina/intellect owner-stat path rides
// refreshPetStatsOnOwnerStatChange's full rebroadcast; this covers the
// agility-only and gear/aura-armor changes. With no live pet motion or no
// pet flag there is nothing to rewrite (C++ calls it on GetPet(), which is
// always a Pet).
func (s *session) rebroadcastPetArmorForOwner(ctx context.Context) {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 {
		return
	}
	motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil || !motionIsPet(motion) {
		return
	}
	petKey := creatureAuraKeyForMotion(motion)
	armor := s.server.petDisplayArmor(petKey, motion)
	mapID, instanceID, petGUID := motion.Map, motion.InstanceID, motion.GUID
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID,
		map[int]uint32{unitFieldResistances: armor})
}

// recomputePetDerivedOnStatAuraChange mirrors the Guardian::UpdateStats arms
// that C++ runs on every pet aura apply/remove (StatSystem.cpp:1266-1282 via
// AuraEffect::HandleStatFlatModifier / HandleModTotalPercentStat ->
// Unit::UpdateUnitMod -> UpdateStats, Unit.cpp:9304-9310): SetStat rewrites
// UNIT_FIELD_STAT0+stat from the aura-fed stat, and the STAT_STAMINA /
// STAT_INTELLECT arms rerun Guardian::UpdateMaxHealth / UpdateMaxPower
// (StatSystem.cpp:1336-1379), whose (GetStat - createStat) * multiplicator
// legs carry the live aura-29/137 feed. The UNIT_MOD_HEALTH / UNIT_MOD_POWER
// modifier auras (34/230/250/133/282, 35/132 — SpellAuraEffects.cpp:3650-3768)
// feed the same UpdateMaxHealth / UpdateMaxPower composition on every unit,
// pet or not (Creature::UpdateMaxHealth/UpdateMaxPower, StatSystem.cpp:1024-1041);
// a Commanding-Shout-type aura on a pet or creature previously moved nothing
// in Go. apply selects the apply/remove half of the per-spell current-health
// adjustments (34 heals on apply, drops on remove; 133 re-anchors to the
// health pct both ways). The 137 stamina tail (SpellAuraEffects.cpp:3570-3572,
// SPELL_ATTR0_ABILITY) re-anchors current health to the pre-change health pct
// instead of clamping. With no live owner session the pet values are left
// alone — C++ has no offline-owner concept (the pet is dismissed with the
// owner). Callers must not hold auraMu or motionMu.
func (s *Server) recomputePetDerivedOnStatAuraChange(key creatureAuraKey, spellID uint32, apply bool, removed *activeAura) {
	if s == nil || s.Data == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	spell, found, err := s.Data.Spell(spellID)
	if err != nil || !found || (!spellHasStatModAura(spell) && !spellHasHealthPowerAura(spell)) {
		return
	}
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion == nil {
		return
	}
	halves := s.creatureAuraHealthPowerHalves(key, spell, spellID, apply, removed)
	if !motionIsPet(motion) {
		s.recomputeCreatureHealthPowerOnAuraChange(key, motion, halves, apply)
		return
	}
	owner := s.playerSessionForGUID(motion.OwnerGUID)
	if owner == nil || owner.player == nil {
		return
	}
	entry, level, petType, petID := motion.Entry, motion.Level, motion.PetType, motion.PetID
	s.motionMu.Lock()
	curHealth, curMana := motion.Health, motion.Mana
	oldMaxHealth, oldMaxMana := motion.MaxHealth, motion.MaxMana
	s.motionMu.Unlock()
	var healthPct float64
	if oldMaxHealth > 0 && curHealth > 0 {
		healthPct = float64(curHealth) / float64(oldMaxHealth)
	}
	zeroHealth := curHealth == 0
	ctx := context.Background()
	_, baseMaxHealth, _, baseMaxMana := owner.getPetStats(ctx, entry, level, petType, petID)
	if baseMaxHealth == 0 || baseMaxMana == 0 {
		return
	}
	// Guardian::UpdateMaxHealth composition (StatSystem.cpp:1336-1357):
	// (createHealth * basePct282) + flatTOTAL(34/230/250) + (ownerBonus +
	// aura-fed stamina delta) * multiplicator, all * totalPct133. The owner
	// bonus lives in the getPetStats base already multiplied, so it is
	// split back out — the 282 BASE_PCT leg covers the create health only,
	// like C++.
	healthBonus := float64(owner.petOwnerStaminaHealthBonus(entry, petType, petID))
	createHealth := float64(baseMaxHealth) - healthBonus
	healthFlat, healthBasePct, healthTotalPct := s.creatureHealthAuraMods(key)
	staminaDelta := s.petStaminaAuraHealthDelta(key, motion, entry)
	newMaxHealth := (createHealth*healthBasePct + float64(healthFlat) + healthBonus + staminaDelta) * healthTotalPct
	if newMaxHealth < 1 {
		newMaxHealth = 1
	}
	// Guardian::UpdateMaxPower composition (StatSystem.cpp:1359-1379):
	// (createMana + flatTOTAL35 + ownerBonus*mult + intellectDelta) *
	// totalPct132 — no BASE feeders exist for the power modifiers in C++.
	manaBonus := float64(owner.petOwnerIntellectManaBonus(entry))
	createMana := float64(baseMaxMana) - manaBonus
	powerFlat, powerTotalPct := s.creaturePowerAuraMods(key, 0)
	intellectDelta := s.petIntellectAuraManaDelta(key, motion, entry)
	newMaxMana := (createMana + float64(powerFlat) + manaBonus + intellectDelta) * powerTotalPct
	if newMaxMana < 1 {
		newMaxMana = 1
	}
	// The 137 stamina tail (SpellAuraEffects.cpp:3570-3572): a
	// SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE aura touching stamina on a
	// SPELL_ATTR0_ABILITY spell re-anchors current health to the pre-apply
	// pct of the new max, on apply and on remove alike (the handler ignores
	// the apply flag for this tail). The 133 health-pct aura re-anchors the
	// same way (SpellAuraEffects.cpp:3729-3754).
	preserveHealthPct := halves.hasHealthPct
	if !preserveHealthPct && spell.Attributes&spellAttr0Ability != 0 {
		for _, eff := range spell.Effects {
			if eff.Aura == spellAuraModTotalStatPct && (eff.MiscValue == statStaminaIndex || eff.MiscValue == -1) {
				preserveHealthPct = true
				break
			}
		}
	}
	s.motionMu.Lock()
	m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if m == nil || m.PetID != petID {
		s.motionMu.Unlock()
		return
	}
	newHealth := curHealth
	switch {
	case preserveHealthPct:
		newHealth = uint32(healthPct * newMaxHealth)
		if !zeroHealth && newHealth < 1 {
			newHealth = 1
		}
	case halves.flatHealth != 0 && curHealth > 0:
		// HandleAuraModIncreaseHealth (SpellAuraEffects.cpp:3650-3665):
		// the flat mod lands first, then current health heals by the
		// amount on apply (capped at the new max) or drops by the amount
		// floored at 1 on remove; a negative amount inverts the halves.
		// ModifyHealth no-ops on a dead unit, so a dead pet keeps 0.
		if apply {
			if halves.flatHealth < 0 {
				if drop := uint32(-halves.flatHealth); drop >= newHealth {
					newHealth = 1
				} else {
					newHealth -= drop
				}
			} else if nh := newHealth + uint32(halves.flatHealth); nh > uint32(newMaxHealth) {
				newHealth = uint32(newMaxHealth)
			} else {
				newHealth = nh
			}
		} else if halves.flatHealth > 0 {
			if drop := uint32(halves.flatHealth); drop >= newHealth {
				newHealth = 1
			} else {
				newHealth -= drop
			}
		} else if nh := newHealth + uint32(-halves.flatHealth); nh > uint32(newMaxHealth) {
			newHealth = uint32(newMaxHealth)
		} else {
			newHealth = nh
		}
	}
	if newHealth > uint32(newMaxHealth) {
		newHealth = uint32(newMaxHealth)
	}
	newMana := curMana
	if halves.powerPct[0] {
		// HandleAuraModIncreaseEnergyPercent (SpellAuraEffects.cpp:3698-3727):
		// current power shifts by the max-power change, on apply and
		// remove alike, clamped into [0, new max] like ModifyPower.
		if np := int64(newMana) + int64(newMaxMana) - int64(oldMaxMana); np < 0 {
			newMana = 0
		} else if np > int64(newMaxMana) {
			newMana = uint32(newMaxMana)
		} else {
			newMana = uint32(np)
		}
	} else if newMana > uint32(newMaxMana) {
		newMana = uint32(newMaxMana)
	}
	m.Health, m.MaxHealth, m.Mana, m.MaxMana = newHealth, uint32(newMaxHealth), newMana, uint32(newMaxMana)
	m.Powers[0], m.MaxPowers[0] = newMana, uint32(newMaxMana)
	mapID, instanceID, petGUID := m.Map, m.InstanceID, m.GUID
	s.motionMu.Unlock()
	fields := map[int]uint32{unitFieldHealth: newHealth, unitFieldMaxHealth: uint32(newMaxHealth), unitFieldPower1: newMana, unitFieldMaxPower1: uint32(newMaxMana)}
	for index, value := range owner.petGuardianFieldStats(m, entry, petType, petID) {
		fields[unitFieldStat0+index] = value
	}
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID, fields)
}

// recomputePetAttackPowerOnAuraChange mirrors the
// Guardian::UpdateAttackPowerAndDamage field-rewrite arms that C++ runs when
// a pet's own attack-power-feeding aura applies or is removed
// (StatSystem.cpp:1468-1476): UNIT_FIELD_ATTACK_POWER / _MODS /
// _MULTIPLIER update on every strength aura (29/137 via
// Guardian::UpdateStats(STAT_STRENGTH), StatSystem.cpp:1274) and on every
// 99/166 aura (via Unit::UpdateUnitMod(UNIT_MOD_ATTACK_POWER),
// Unit.cpp:9336-9337). Previously Go rewrote these fields only at summon and
// level-up, so a Blessing of Might-type aura landing on a pet never moved
// its advertised attack power. The composition reads live state
// (petGuardianAttackPower folds the pet's current auras and the owner's live
// AP), matching the C++ recompute-at-apply/remove semantics — on remove the
// aura is already erased, so the folds read the post-remove state like the
// C++ remove leg. With no live owner session the fields are left alone —
// C++ has no offline-owner concept (the pet is dismissed with the owner).
// Callers must not hold auraMu or motionMu.
func (s *Server) recomputePetAttackPowerOnAuraChange(key creatureAuraKey, spellID uint32) {
	if s == nil || s.Data == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	spell, found, err := s.Data.Spell(spellID)
	if err != nil || !found || !spellHasPetAttackPowerAura(spell) {
		return
	}
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion == nil || !motionIsPet(motion) {
		return
	}
	owner := s.playerSessionForGUID(motion.OwnerGUID)
	if owner == nil || owner.player == nil {
		return
	}
	_, apBase, apFlat, apMult := s.petGuardianAttackPower(owner, motion)
	mindmg, maxdmg := s.petPhysicalDamageRange(owner, motion)
	s.broadcastCreatureValuesUpdateInInstance(key.Map, key.InstanceID, key.GUID, map[int]uint32{
		unitFieldAttackPower:           uint32(int32(apBase)),
		unitFieldAttackPowerMods:       uint32(apFlat),
		unitFieldAttackPowerMultiplier: math.Float32bits(float32(apMult)),
		// Guardian::UpdateAttackPowerAndDamage tails into
		// Guardian::UpdateDamagePhysical (StatSystem.cpp:1476), so the
		// strength/AP aura apply/remove rewrites the weapon-damage fields
		// too — not just the AP trio.
		unitFieldMinDamage: math.Float32bits(float32(mindmg)),
		unitFieldMaxDamage: math.Float32bits(float32(maxdmg)),
	})
}

// recomputePetDamageFieldsOnAuraChange mirrors the Guardian::UpdateDamagePhysical
// half that C++ runs when a pet's own weapon-damage-feeding aura applies or is
// removed: SPELL_AURA_MOD_DAMAGE_DONE (13) and
// SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79) with the NORMAL school misc bit run
// through Unit::UpdateUnitMod -> UpdateDamagePhysical (Unit.cpp:9335,
// StatSystem.cpp:1479-1554) via UpdateAllDamageDoneMods /
// UpdateAllDamagePctDoneMods (SpellAuraEffects.cpp:4166-4199), rewriting
// UNIT_FIELD_MINDAMAGE/MAXDAMAGE. The strength/AP arms (29/137/99/166) reach
// the same fields through the UpdateAttackPowerAndDamage tail and ride
// recomputePetAttackPowerOnAuraChange instead. With no live owner session the
// fields are left alone — C++ has no offline-owner concept (the pet is
// dismissed with the owner). Callers must not hold auraMu or motionMu.
func (s *Server) recomputePetDamageFieldsOnAuraChange(key creatureAuraKey, spellID uint32) {
	if s == nil || s.Data == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	spell, found, err := s.Data.Spell(spellID)
	if err != nil || !found || !spellHasPetWeaponDamageAura(spell) {
		return
	}
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion == nil || !motionIsPet(motion) {
		return
	}
	owner := s.playerSessionForGUID(motion.OwnerGUID)
	if owner == nil || owner.player == nil {
		return
	}
	mindmg, maxdmg := s.petPhysicalDamageRange(owner, motion)
	s.broadcastCreatureValuesUpdateInInstance(key.Map, key.InstanceID, key.GUID, map[int]uint32{
		unitFieldMinDamage: math.Float32bits(float32(mindmg)),
		unitFieldMaxDamage: math.Float32bits(float32(maxdmg)),
	})
}

// getPetStatsWithAuraFeed wraps getPetStats with the live stamina/intellect
// aura-feed arms of Guardian::UpdateMaxHealth / UpdateMaxPower
// (StatSystem.cpp:1336-1379): the (GetStat - createStat) * multiplicator legs
// carry the aura-29/137 feed, which C++ recomputes on every UpdateStats —
// including aura apply/remove — so the stat-compute points that overwrite the
// stored maxima (level-up, owner-stat change) must re-add the feed, or the
// live recompute from recomputePetDerivedOnStatAuraChange would be clobbered
// by the next compute point. The UNIT_MOD_HEALTH / UNIT_MOD_POWER modifier
// folds (34/230/250/133/282, 35/132) ride along in the C++ composition, so
// they fold here too.
func (s *session) getPetStatsWithAuraFeed(ctx context.Context, motion *creatureMotion, entry, level uint32, petType uint8, petNumber uint32) (curHealth, maxHealth, curMana, maxMana uint32) {
	curHealth, maxHealth, curMana, maxMana = s.getPetStats(ctx, entry, level, petType, petNumber)
	if s == nil || s.server == nil || motion == nil {
		return curHealth, maxHealth, curMana, maxMana
	}
	key := creatureAuraKeyForMotion(motion)
	healthBonus := float64(s.petOwnerStaminaHealthBonus(entry, petType, petNumber))
	createHealth := float64(maxHealth) - healthBonus
	healthFlat, healthBasePct, healthTotalPct := s.server.creatureHealthAuraMods(key)
	if v := (createHealth*healthBasePct + float64(healthFlat) + healthBonus + s.server.petStaminaAuraHealthDelta(key, motion, entry)) * healthTotalPct; v >= 1 {
		maxHealth = uint32(v)
	} else {
		maxHealth = 1
	}
	manaBonus := float64(s.petOwnerIntellectManaBonus(entry))
	createMana := float64(maxMana) - manaBonus
	powerFlat, powerTotalPct := s.server.creaturePowerAuraMods(key, 0)
	if v := (createMana + float64(powerFlat) + manaBonus + s.server.petIntellectAuraManaDelta(key, motion, entry)) * powerTotalPct; v >= 1 {
		maxMana = uint32(v)
	} else {
		maxMana = 1
	}
	return curHealth, maxHealth, curMana, maxMana
}

func PetFoodBenefitLevel(petLevel, itemLevel uint32) uint32 {
	if petLevel <= itemLevel+5 {
		return 35000
	}
	if petLevel <= itemLevel+10 {
		return 17000
	}
	if petLevel <= itemLevel+14 {
		return 8000
	}
	return 0
}

// petHappinessLevelSize mirrors HAPPINESS_LEVEL_SIZE (Pet.h:24): one
// happiness level is 333000 (petHappinessMax = 1050000, ~3.15 levels).
const petHappinessLevelSize = 333000

// petHappinessDamageMod mirrors Pet::GetHappinessState (Pet.cpp:707-714)
// baked into Guardian::UpdateDamagePhysical (StatSystem.cpp:1521-1534):
// HAPPY pets (2+ levels of happiness) deal 125%, UNHAPPY pets (under one
// level) deal 75%, CONTENT pets deal 100%. C++ bakes the multiplier at
// stat-update time only — happiness ticks never recompute damage — so Go
// applies it at the pet stat-compute points (summon, level-up), not per
// swing.
func petHappinessDamageMod(happiness uint32) float32 {
	if happiness < petHappinessLevelSize {
		return 0.75
	}
	if happiness >= 2*petHappinessLevelSize {
		return 1.25
	}
	return 1.0
}

func (s *session) applyPetLevel(ctx context.Context, petID, entry uint32, petType uint8, oldLevel, newLevel, experience, nextLevelXP uint32) bool {
	if s == nil || s.server == nil || s.player == nil || petID == 0 || entry == 0 || newLevel == 0 || oldLevel == newLevel || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil || motion.PetID != petID || motion.Entry != entry || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return false
	}
	s.server.motionMu.Unlock()
	health, maxHealth, mana, maxMana := s.getPetStatsWithAuraFeed(ctx, motion, entry, newLevel, petType, petID)
	attributes := s.getPetAttributes(ctx, entry, newLevel, petType)
	if health == 0 || maxHealth == 0 {
		return false
	}
	result, err := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_pet SET level = ?, exp = ?, curhealth = ?, curmana = ? WHERE id = ? AND owner = ?", newLevel, experience, health, mana, petID, s.playerGUID)
	if err != nil {
		s.debug("pet level persistence failed", "account", s.accountName, "petID", petID, "error", err)
		return false
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return false
	}
	s.server.motionMu.Lock()
	if s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID) != motion || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return false
	}
	motion.Level, motion.Experience, motion.Stats = newLevel, experience, attributes
	motion.Health, motion.MaxHealth, motion.Mana, motion.MaxMana = health, maxHealth, mana, maxMana
	motion.Powers[0], motion.MaxPowers[0] = mana, maxMana
	motion.PetNextLevelXP = nextLevelXP
	// Pet::InitStatsForLevel (Pet.cpp:885-890) recomputes base armor and
	// the native object scale at every level-up.
	motion.Armor = s.petBaseArmor(ctx, entry, newLevel, petType)
	motion.PetOwnerArmorBonus = petOwnerArmorValue(petType, s.player.Armor)
	petScale := s.petNativeObjectScale(ctx, entry, newLevel, petType)
	if petType == 1 {
		// Guardian::UpdateDamagePhysical's happiness leg (StatSystem.cpp:1521-1534),
		// read from the pet's live happiness at stat-update time.
		mod := petHappinessDamageMod(motion.Powers[4])
		motion.MinDamage, motion.MaxDamage = float32(newLevel-newLevel/4)*mod, float32(newLevel+newLevel/4)*mod
	}
	fields := map[int]uint32{unitFieldLevel: newLevel, unitFieldHealth: health, unitFieldMaxHealth: maxHealth, unitFieldPower1: mana, unitFieldMaxPower1: maxMana, unitFieldPetExperience: experience, unitFieldResistances: s.server.petDisplayArmor(creatureAuraKeyForMotion(motion), motion), objectFieldScale: math.Float32bits(petScale)}
	// Guardian::UpdateStats (StatSystem.cpp:1266-1282) rewrites
	// UNIT_FIELD_STAT0+index from the aura-fed stats at every level-up via
	// Pet::InitStatsForLevel -> UpdateAllStats.
	for index, value := range s.petGuardianFieldStats(motion, entry, petType, petID) {
		fields[unitFieldStat0+index] = value
	}
	// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1468-1476)
	// rewrites the AP unit fields at every stat update too.
	_, apBase, apFlat, apMult := s.server.petGuardianAttackPower(s, motion)
	fields[unitFieldAttackPower] = uint32(int32(apBase))
	fields[unitFieldAttackPowerMods] = uint32(apFlat)
	fields[unitFieldAttackPowerMultiplier] = math.Float32bits(float32(apMult))
	// The UpdateAttackPowerAndDamage tail runs Guardian::UpdateDamagePhysical
	// (StatSystem.cpp:1476, 1479-1554), rewriting UNIT_FIELD_MINDAMAGE/
	// MAXDAMAGE at the same level-up.
	rangeMin, rangeMax := s.server.petPhysicalDamageRange(s, motion)
	fields[unitFieldMinDamage] = math.Float32bits(float32(rangeMin))
	fields[unitFieldMaxDamage] = math.Float32bits(float32(rangeMax))
	if petType == 1 {
		fields[unitFieldPetNextLevelExp] = nextLevelXP
	}
	mapID, instanceID, petGUID := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID, fields)
	return true
}

func (s *session) updatePetLevelSpells(ctx context.Context, petID, entry, oldLevel, newLevel uint32, petType uint8, notify bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || petID == 0 || oldLevel == newLevel {
		return
	}
	cdb := s.server.CharactersStore.DB
	type rankSwap struct {
		oldSpell, newSpell uint32
		inheritedActive    uint8
	}
	var swaps []rankSwap
	var unlearned []uint32
	oldActive := make(map[uint32]uint8)
	for _, chain := range getPetSpellChains(entry) {
		var oldRank, newRank *petSpellRank
		for i := range chain {
			if chain[i].minLevel <= oldLevel {
				oldRank = &chain[i]
			}
			if chain[i].minLevel <= newLevel {
				newRank = &chain[i]
			}
		}
		if oldRank != nil && (newRank == nil || oldRank.spellID != newRank.spellID) {
			// Capture the old rank's live active state before the row is
			// deleted: the replacing rank inherits it (Pet.cpp:1432).
			var rowActive int64 = -1
			if err := cdb.QueryRowContext(ctx, "SELECT active FROM pet_spell WHERE guid = ? AND spell = ?", petID, oldRank.spellID).Scan(&rowActive); err == nil {
				oldActive[oldRank.spellID] = 0
				if rowActive != 0 {
					oldActive[oldRank.spellID] = 1
				}
			}
			if _, err := cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petID, oldRank.spellID); err != nil {
				s.debug("pet rank removal failed", "account", s.accountName, "petID", petID, "spell", oldRank.spellID, "error", err)
			}
			if notify {
				unlearn := protocol.NewBuffer(4)
				unlearn.WriteU32(oldRank.spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearn.Bytes(), true)
			}
			unlearned = append(unlearned, oldRank.spellID)
		}
		if newRank != nil && (oldRank == nil || oldRank.spellID != newRank.spellID) {
			active := uint8(0)
			if newRank.autocast {
				active = 1
			}
			swapped := oldRank != nil && oldRank.spellID != newRank.spellID
			if swapped {
				// Pet::addSpell ranked-replacement arm (Pet.cpp:1429-1447):
				// the new rank inherits the old rank's live active state
				// instead of resetting to the static autocastable default.
				if inherited, ok := oldActive[oldRank.spellID]; ok {
					active = inherited
				}
			}
			if _, err := cdb.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, newRank.spellID, active); err != nil {
				s.debug("pet rank learn persistence failed", "account", s.accountName, "petID", petID, "spell", newRank.spellID, "error", err)
			}
			if notify {
				learn := protocol.NewBuffer(4)
				learn.WriteU32(newRank.spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learn.Bytes(), true)
			}
			oldSpell := uint32(0)
			if swapped {
				oldSpell = oldRank.spellID
			}
			swaps = append(swaps, rankSwap{oldSpell: oldSpell, newSpell: newRank.spellID, inheritedActive: active})
		}
	}
	// Sync the live motion with the rank changes: C++ unlearnSpell removes
	// the old rank from the pet's spell book (Pet.cpp:1541-1555) and
	// removeSpell strips its auras (Pet.cpp:1571), so the engine's known
	// spell and autocast lists must follow the DB, not keep casting the
	// replaced rank until the next resummon.
	if s.player != nil && s.player.PetGUID != 0 && (len(swaps) > 0 || len(unlearned) > 0) {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
		var petKey creatureAuraKey
		if motion != nil && motion.PetID == petID {
			for _, oldSpell := range unlearned {
				for j, sp := range motion.Spells {
					if sp == oldSpell {
						motion.Spells = append(motion.Spells[:j], motion.Spells[j+1:]...)
						break
					}
				}
				for j, sp := range motion.AutocastSpells {
					if sp == oldSpell {
						motion.AutocastSpells = append(motion.AutocastSpells[:j], motion.AutocastSpells[j+1:]...)
						break
					}
				}
			}
			for _, swap := range swaps {
				known := false
				for _, sp := range motion.Spells {
					if sp == swap.newSpell {
						known = true
						break
					}
				}
				if !known {
					motion.Spells = append(motion.Spells, swap.newSpell)
				}
				if swap.inheritedActive != 0 {
					auto := false
					for _, sp := range motion.AutocastSpells {
						if sp == swap.newSpell {
							auto = true
							break
						}
					}
					if !auto {
						motion.AutocastSpells = append(motion.AutocastSpells, swap.newSpell)
					}
				}
			}
			petKey = creatureAuraKeyForMotion(motion)
		}
		s.server.motionMu.Unlock()
		if petKey.GUID != 0 {
			for _, oldSpell := range unlearned {
				s.server.removeCreatureAura(petKey, oldSpell)
			}
		}
	}
	// CharmInfo::AddSpellToActionBar (Unit.cpp:10053): a ranked replacement
	// does not vacate the bar slot — the slot's action is swapped to the new
	// rank in place, keeping the autocast type. Go's rank swap left the old
	// rank's action in abdata, so the client bar kept a spell the pet no
	// longer knew.
	if len(swaps) > 0 && s.server != nil {
		if slots, ok := loadPetActionBar(ctx, cdb, petID, s.playerGUID); ok {
			changed := false
			for _, swap := range swaps {
				if swap.oldSpell == 0 || swap.newSpell == 0 {
					continue
				}
				first := s.server.getFirstSpellInChain(swap.newSpell)
				for i, slot := range slots {
					actionType := uint8(slot >> 24)
					if !petSpellActionType(actionType) {
						continue
					}
					if action := slot & 0x00FFFFFF; action != 0 && s.server.getFirstSpellInChain(action) == first {
						slots[i] = makePetActionSlot(actionType, swap.newSpell)
						changed = true
						break
					}
				}
			}
			if changed {
				savePetActionBar(ctx, cdb, petID, s.playerGUID, slots)
			}
		}
	}
	if notify {
		if petType == 1 {
			s.sendTalentsInfo(true)
		}
		s.sendPetSpells(ctx, petID, entry, 1)
	}
}

func (s *session) giveHunterPetXP(ctx context.Context, earnedXP uint32) {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 || earnedXP == 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil || motion.OwnerGUID != s.playerGUID || motion.PetType != 1 || motion.Health == 0 || motion.PetID == 0 {
		s.server.motionMu.Unlock()
		return
	}
	petID, entry, oldLevel, currentXP, currentNextXP := motion.PetID, motion.Entry, motion.Level, motion.Experience, motion.PetNextLevelXP
	s.server.motionMu.Unlock()
	maxLevel := s.server.Config.MaxPlayerLevel
	if ownerLevel := uint32(s.player.Level); ownerLevel < maxLevel {
		maxLevel = ownerLevel
	}
	xpForLevel := make([]uint32, len(xpCurve))
	copy(xpForLevel, xpCurve[:])
	for level := uint32(1); level < uint32(len(xpForLevel)); level++ {
		xpForLevel[level] = s.server.xpForLevel(ctx, level)
	}
	newLevel, newXP, nextXP := AdvanceHunterPetExperience(oldLevel, currentXP, earnedXP, maxLevel, currentNextXP, xpForLevel)
	if newLevel == oldLevel && newXP == currentXP && nextXP == currentNextXP {
		return
	}
	if newLevel != oldLevel {
		if !s.applyPetLevel(ctx, petID, entry, 1, oldLevel, newLevel, newXP, nextXP) {
			return
		}
		s.updatePetLevelSpells(ctx, petID, entry, oldLevel, newLevel, 1, true)
		return
	}
	s.server.motionMu.Lock()
	if s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID) != motion || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return
	}
	motion.Experience, motion.PetNextLevelXP = newXP, nextXP
	mapID, instanceID, petGUID := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID, map[int]uint32{unitFieldPetExperience: newXP, unitFieldPetNextLevelExp: nextXP})
}
