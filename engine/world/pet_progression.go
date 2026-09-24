package world

import (
	"context"

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

func (s *session) applyPetLevel(ctx context.Context, petID, entry uint32, petType uint8, oldLevel, newLevel, experience, nextLevelXP uint32) bool {
	if s == nil || s.server == nil || s.player == nil || petID == 0 || entry == 0 || newLevel == 0 || oldLevel == newLevel || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[s.player.PetGUID]
	if motion == nil || motion.PetID != petID || motion.Entry != entry || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return false
	}
	s.server.motionMu.Unlock()
	health, maxHealth, mana, maxMana := s.getPetStats(ctx, entry, newLevel, petType)
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
	if s.server.creatureMotion[s.player.PetGUID] != motion || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return false
	}
	motion.Level, motion.Experience, motion.Stats = newLevel, experience, attributes
	motion.Health, motion.MaxHealth, motion.Mana, motion.MaxMana = health, maxHealth, mana, maxMana
	motion.Powers[0], motion.MaxPowers[0] = mana, maxMana
	motion.PetNextLevelXP = nextLevelXP
	if petType == 1 {
		motion.MinDamage, motion.MaxDamage = float32(newLevel-newLevel/4), float32(newLevel+newLevel/4)
	}
	fields := map[int]uint32{unitFieldLevel: newLevel, unitFieldHealth: health, unitFieldMaxHealth: maxHealth, unitFieldPower1: mana, unitFieldMaxPower1: maxMana, unitFieldPetExperience: experience}
	for index, value := range attributes {
		fields[unitFieldStat0+index] = value
	}
	if petType == 1 {
		fields[unitFieldPetNextLevelExp] = nextLevelXP
	}
	mapID, petGUID := motion.Map, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdate(mapID, petGUID, fields)
	return true
}

func (s *session) updatePetLevelSpells(ctx context.Context, petID, entry, oldLevel, newLevel uint32, petType uint8, notify bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || petID == 0 || oldLevel == newLevel {
		return
	}
	cdb := s.server.CharactersStore.DB
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
			if _, err := cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petID, oldRank.spellID); err != nil {
				s.debug("pet rank removal failed", "account", s.accountName, "petID", petID, "spell", oldRank.spellID, "error", err)
			}
			if notify {
				unlearn := protocol.NewBuffer(4)
				unlearn.WriteU32(oldRank.spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearn.Bytes(), true)
			}
		}
		if newRank != nil && (oldRank == nil || oldRank.spellID != newRank.spellID) {
			active := uint8(0)
			if newRank.autocast {
				active = 1
			}
			if _, err := cdb.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, newRank.spellID, active); err != nil {
				s.debug("pet rank learn persistence failed", "account", s.accountName, "petID", petID, "spell", newRank.spellID, "error", err)
			}
			if notify {
				learn := protocol.NewBuffer(4)
				learn.WriteU32(newRank.spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learn.Bytes(), true)
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
	motion := s.server.creatureMotion[s.player.PetGUID]
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
	if s.server.creatureMotion[s.player.PetGUID] != motion || motion.Level != oldLevel {
		s.server.motionMu.Unlock()
		return
	}
	motion.Experience, motion.PetNextLevelXP = newXP, nextXP
	mapID, petGUID := motion.Map, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdate(mapID, petGUID, map[int]uint32{unitFieldPetExperience: newXP, unitFieldPetNextLevelExp: nextXP})
}
