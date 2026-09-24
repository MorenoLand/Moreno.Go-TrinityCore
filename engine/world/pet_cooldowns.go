package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

func (s *session) registerPetMotion(ctx context.Context, petGUID uint64, petID uint32, petType uint8, entry, level, faction, health, maxHealth, mana, maxMana, happiness, experience, nextLevelXP uint32, reactState uint8, combatReach float32, x, y, z, orientation float32) {
	if s == nil || s.server == nil || s.player == nil || petGUID == 0 || petID == 0 {
		return
	}
	var spells, autocast []uint32
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		if rows, err := cdb.QueryContext(ctx, "SELECT spell, active FROM pet_spell WHERE guid = ? ORDER BY spell", petID); err == nil {
			for rows.Next() {
				var spellID uint32
				var active uint8
				if rows.Scan(&spellID, &active) == nil && spellID > 0 {
					spells = append(spells, spellID)
					if active != 0 {
						autocast = append(autocast, spellID)
					}
				}
			}
			_ = rows.Close()
		}
	}
	powerType := uint32(0)
	powers, maxPowers := [7]uint32{}, [7]uint32{}
	powers[0], maxPowers[0] = mana, maxMana
	if petType == 1 {
		powerType = 2
		powers[2], maxPowers[2] = petFocusMax, petFocusMax
		powers[4], maxPowers[4] = happiness, petHappinessMax
	}
	minDamage, maxDamage := float32(maxUint32(level*2, 5)), float32(maxUint32(level*3, 10))
	if petType == 1 {
		minDamage, maxDamage = float32(level-level/4), float32(level+level/4)
	}
	motion := &creatureMotion{GUID: petGUID, Entry: entry, Map: s.player.Map, HomeX: x, HomeY: y, HomeZ: z, X: x, Y: y, Z: z, Orientation: orientation, Speed: 2.5, RunSpeed: 7, Faction: faction, Level: level, UnitFlags: unitFlagPlayerControlled, UnitFlags2: unitFlag2RegeneratePower, AttackTime: 2000, CombatReach: combatReach, Health: health, MaxHealth: maxHealth, Mana: mana, MaxMana: maxMana, PowerType: powerType, Powers: powers, MaxPowers: maxPowers, PetID: petID, PetType: petType, PetNextLevelXP: nextLevelXP, FocusRegenTimer: 4 * time.Second, HappinessTimer: 7500 * time.Millisecond, Happiness: happiness, Experience: experience, OwnerGUID: s.playerGUID, Spells: spells, SpellCooldowns: make(map[uint32]time.Time), SpellCategoryCooldowns: make(map[uint32]time.Time), PetCommand: PetCommandFollow, PetReact: reactState, AutocastSpells: autocast, MinDamage: minDamage, MaxDamage: maxDamage, Refreshed: time.Now()}
	s.loadPetCooldowns(ctx, petID, motion)
	s.server.motionMu.Lock()
	if s.server.creatureMotion == nil {
		s.server.creatureMotion = make(map[uint64]*creatureMotion)
	}
	s.server.creatureMotion[petGUID] = motion
	s.server.motionMu.Unlock()
}

func (s *session) loadPetCooldowns(ctx context.Context, petID uint32, motion *creatureMotion) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || motion == nil {
		return
	}
	now := time.Now()
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell, categoryId, time, categoryEnd FROM pet_spell_cooldown WHERE guid = ? AND (time > ? OR categoryEnd > ?) ORDER BY spell", petID, now.Unix(), now.Unix())
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var spellID, categoryID uint32
		var spellEnd, categoryEnd int64
		if rows.Scan(&spellID, &categoryID, &spellEnd, &categoryEnd) != nil {
			continue
		}
		if spellID != 0 && spellEnd > now.Unix() && s.server.Data != nil {
			if spell, found, spellErr := s.server.Data.Spell(spellID); spellErr == nil && found && spell.RecoveryTime > 0 {
				motion.SpellCooldowns[spellID] = time.Unix(spellEnd, 0).Add(-time.Duration(spell.RecoveryTime) * time.Millisecond)
			}
		}
		if categoryID != 0 && categoryEnd > now.Unix() {
			motion.SpellCategoryCooldowns[categoryID] = time.Unix(categoryEnd, 0)
		}
	}
}

func (s *session) recordPetSpellCooldown(motion *creatureMotion, spell wotlk.Spell, now time.Time) {
	if s == nil || s.server == nil || motion == nil {
		return
	}
	categoryID, categoryRecovery, _ := s.spellCooldownCategory(spell.ID)
	s.server.motionMu.Lock()
	if motion.SpellCooldowns == nil {
		motion.SpellCooldowns = make(map[uint32]time.Time)
	}
	motion.SpellCooldowns[spell.ID] = now
	if categoryID != 0 && categoryRecovery > 0 {
		if motion.SpellCategoryCooldowns == nil {
			motion.SpellCategoryCooldowns = make(map[uint32]time.Time)
		}
		motion.SpellCategoryCooldowns[categoryID] = now.Add(time.Duration(categoryRecovery) * time.Millisecond)
	}
	motion.LastSpell = now
	s.server.motionMu.Unlock()
}
