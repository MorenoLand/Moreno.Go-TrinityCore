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

// petBaseArmor mirrors the armor arm of Pet::InitStatsForLevel (Pet.cpp:885,
// 903-905) folded through Guardian::UpdateArmor (StatSystem.cpp:1316-1330):
// flat(BASE_VALUE) + agility*2 + flat(TOTAL_VALUE) + owner bonus, WITHOUT the
// aura pct/flat folds. The aura-driven slots (142 BASE_PCT, 22/83
// TOTAL_VALUE, 101 TOTAL_PCT) fold live at damage time in the combat-time
// armor folds and at display time in petDisplayArmor — C++ UpdateArmor
// recomputes the full value from the stored base whenever auras change, so
// folding them into the stored value here double-counted them against the
// combat-time fold whenever auras were live at a stat-compute point
// (level-up, owner-stat change). Hunter and warlock pets gain 35% of the
// owner's armor as a flat bonus; C++ gates on IsPet(), and Go's character
// pets (summon/hunter) are the Pet twin.
func (s *session) petBaseArmor(ctx context.Context, entry, level uint32, petType uint8, agility uint32) uint32 {
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
	armorF := float64(armor)
	armorF += float64(agility) * 2
	if s != nil && s.player != nil && (petType == petTypeHunter || petType == petTypeSummon) {
		armorF += float64(s.player.Armor) * 0.35
	}
	if armorF < 0 {
		armorF = 0
	}
	return uint32(armorF)
}

// petDisplayArmor folds the pet's live armor aura modifiers onto the
// aura-free petBaseArmor base for display (unitFieldResistances): the same
// (base*basePct + flat) * totalPct composition the combat-time damage folds
// apply, matching Guardian::UpdateArmor's full recompute
// (StatSystem.cpp:1316-1330). BASE_PCT from SPELL_AURA_MOD_BASE_RESISTANCE_PCT
// (142) with misc&NORMAL (SpellAuraEffects.cpp:3281-3296;
// ApplyStatPctModifier compounds via AddPct, Unit.cpp:9237-9256),
// TOTAL_VALUE flat from SPELL_AURA_MOD_RESISTANCE (22) through the school
// loop (SpellAuraEffects.cpp:3255-3269, UNIT_MOD_RESISTANCE_START+0 ==
// UNIT_MOD_ARMOR) and from SPELL_AURA_MOD_BASE_RESISTANCE (83) with
// misc&NORMAL (SpellAuraEffects.cpp:3340-3350), TOTAL_PCT from
// SPELL_AURA_MOD_RESISTANCE_PCT (101) with misc&NORMAL (product of
// (1+amount/100), SpellAuraEffects.cpp:3303-3321).
func (s *Server) petDisplayArmor(key creatureAuraKey, base uint32) uint32 {
	if s == nil {
		return base
	}
	basePct, totalPct := s.petArmorPctBonus(key, true)
	armor := float64(base)*basePct + s.petBaseArmorBonus(key, true)
	armor *= totalPct
	if armor < 0 {
		armor = 0
	}
	return uint32(armor)
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
	_, maxHealth, _, maxMana := s.getPetStats(ctx, entry, level, petType, petID)
	attributes := s.getPetAttributes(ctx, entry, level, petType)
	armor := s.petBaseArmor(ctx, entry, level, petType, attributes[1])
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
	fields := map[int]uint32{unitFieldHealth: curHealth, unitFieldMaxHealth: maxHealth, unitFieldPower1: curMana, unitFieldMaxPower1: maxMana, unitFieldResistances: s.server.petDisplayArmor(petKey, armor)}
	for index, value := range attributes {
		fields[unitFieldStat0+index] = value
	}
	mapID, instanceID, petGUID := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, petGUID, fields)
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
	health, maxHealth, mana, maxMana := s.getPetStats(ctx, entry, newLevel, petType, petID)
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
	motion.Armor = s.petBaseArmor(ctx, entry, newLevel, petType, attributes[1])
	petScale := s.petNativeObjectScale(ctx, entry, newLevel, petType)
	if petType == 1 {
		// Guardian::UpdateDamagePhysical's happiness leg (StatSystem.cpp:1521-1534),
		// read from the pet's live happiness at stat-update time.
		mod := petHappinessDamageMod(motion.Powers[4])
		motion.MinDamage, motion.MaxDamage = float32(newLevel-newLevel/4)*mod, float32(newLevel+newLevel/4)*mod
	}
	fields := map[int]uint32{unitFieldLevel: newLevel, unitFieldHealth: health, unitFieldMaxHealth: maxHealth, unitFieldPower1: mana, unitFieldMaxPower1: maxMana, unitFieldPetExperience: experience, unitFieldResistances: s.server.petDisplayArmor(creatureAuraKeyForMotion(motion), motion.Armor), objectFieldScale: math.Float32bits(petScale)}
	for index, value := range attributes {
		fields[unitFieldStat0+index] = value
	}
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
