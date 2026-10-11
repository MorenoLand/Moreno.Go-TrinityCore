package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *session) registerPetMotion(ctx context.Context, petGUID uint64, petID uint32, petType uint8, entry, level, faction, health, maxHealth, mana, maxMana uint32, attributes [5]uint32, happiness, experience, nextLevelXP uint32, reactState uint8, combatReach float32, x, y, z, orientation float32) {
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
		// Guardian::UpdateDamagePhysical's happiness leg (StatSystem.cpp:1521-1534).
		mod := petHappinessDamageMod(happiness)
		minDamage, maxDamage = minDamage*mod, maxDamage*mod
	}
	motion := &creatureMotion{GUID: petGUID, Entry: entry, Map: s.player.Map, InstanceID: s.player.InstanceID, HomeX: x, HomeY: y, HomeZ: z, X: x, Y: y, Z: z, Orientation: orientation, Speed: 2.5, RunSpeed: 7, Faction: faction, Level: level, UnitFlags: unitFlagPlayerControlled, UnitFlags2: unitFlag2RegeneratePower, AttackTime: 2000, CombatReach: combatReach, Health: health, MaxHealth: maxHealth, Mana: mana, MaxMana: maxMana, PowerType: powerType, Powers: powers, MaxPowers: maxPowers, PetID: petID, PetType: petType, PetNextLevelXP: nextLevelXP, FocusRegenTimer: 4 * time.Second, HappinessTimer: 7500 * time.Millisecond, Happiness: happiness, Experience: experience, OwnerGUID: s.playerGUID, Spells: spells, SpellCooldowns: make(map[uint32]time.Time), SpellCategoryCooldowns: make(map[uint32]time.Time), SpellCooldownCategories: make(map[uint32]uint32), SpellCooldownCategoryEnds: make(map[uint32]time.Time), PetCommand: PetCommandFollow, PetReact: reactState, AutocastSpells: autocast, MinDamage: minDamage, MaxDamage: maxDamage, Refreshed: time.Now()}
	motion.Stats = attributes
	s.loadPetCooldowns(ctx, petID, motion)
	s.server.motionMu.Lock()
	s.server.motionMapLocked(s.player.Map, s.player.InstanceID)[petGUID] = motion
	s.server.motionMu.Unlock()
}

func (s *session) loadPetCooldowns(ctx context.Context, petID uint32, motion *creatureMotion) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || motion == nil {
		return
	}
	now := time.Now()
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell, categoryId, time, categoryEnd FROM pet_spell_cooldown WHERE guid = ? AND time > ? ORDER BY spell", petID, now.Unix())
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
		if spellID != 0 && spellEnd > now.Unix() {
			motion.SpellCooldowns[spellID] = time.Unix(spellEnd, 0)
			if categoryID != 0 {
				motion.SpellCooldownCategories[spellID] = categoryID
				motion.SpellCooldownCategoryEnds[spellID] = time.Unix(categoryEnd, 0)
			}
		}
		if categoryID != 0 && categoryEnd > now.Unix() {
			motion.SpellCategoryCooldowns[categoryID] = time.Unix(categoryEnd, 0)
		}
	}
}

func prunePetSpellCooldowns(motion *creatureMotion, now time.Time) {
	if motion == nil {
		return
	}
	for categoryID, categoryEnd := range motion.SpellCategoryCooldowns {
		if categoryEnd.Before(now) {
			delete(motion.SpellCategoryCooldowns, categoryID)
		}
	}
	for spellID, cooldownEnd := range motion.SpellCooldowns {
		if !cooldownEnd.Before(now) {
			continue
		}
		if categoryID := motion.SpellCooldownCategories[spellID]; categoryID != 0 {
			delete(motion.SpellCategoryCooldowns, categoryID)
		}
		delete(motion.SpellCooldowns, spellID)
		delete(motion.SpellCooldownCategories, spellID)
		delete(motion.SpellCooldownCategoryEnds, spellID)
	}
}

func petCooldownMilliseconds(end, now time.Time) uint32 {
	remaining := end.Sub(now).Milliseconds()
	if remaining <= 0 {
		return 0
	}
	if remaining > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(remaining)
}

func ResolvePetCooldownEnds(now time.Time, spellRecovery, categoryRecovery uint32) (time.Time, time.Time, bool) {
	if spellRecovery == 0 && categoryRecovery == 0 {
		return time.Time{}, time.Time{}, false
	}
	spellEnd := now
	if spellRecovery > 0 {
		spellEnd = now.Add(time.Duration(spellRecovery) * time.Millisecond)
	}
	categoryEnd := time.Time{}
	if categoryRecovery > 0 {
		categoryEnd = now.Add(time.Duration(categoryRecovery) * time.Millisecond)
		if spellRecovery == 0 {
			spellEnd = categoryEnd
		}
	}
	return spellEnd, categoryEnd, true
}

func (s *session) recordPetSpellCooldown(motion *creatureMotion, spell wotlk.Spell, now time.Time) {
	if s == nil || s.server == nil || motion == nil {
		return
	}
	categoryID, categoryRecovery := spell.Category, spell.CategoryRecoveryTime
	cooldownEnd, categoryEnd, hasCooldown := ResolvePetCooldownEnds(now, spell.RecoveryTime, categoryRecovery)
	if !hasCooldown {
		s.server.motionMu.Lock()
		motion.LastSpell = now
		s.server.motionMu.Unlock()
		return
	}
	s.server.motionMu.Lock()
	prunePetSpellCooldowns(motion, now)
	if motion.SpellCooldowns == nil {
		motion.SpellCooldowns = make(map[uint32]time.Time)
	}
	if motion.SpellCooldownCategories == nil {
		motion.SpellCooldownCategories = make(map[uint32]uint32)
	}
	if motion.SpellCooldownCategoryEnds == nil {
		motion.SpellCooldownCategoryEnds = make(map[uint32]time.Time)
	}
	motion.SpellCooldowns[spell.ID] = cooldownEnd
	if categoryID != 0 {
		motion.SpellCooldownCategories[spell.ID] = categoryID
		motion.SpellCooldownCategoryEnds[spell.ID] = categoryEnd
	} else {
		delete(motion.SpellCooldownCategories, spell.ID)
		delete(motion.SpellCooldownCategoryEnds, spell.ID)
	}
	if categoryID != 0 && !categoryEnd.IsZero() {
		if motion.SpellCategoryCooldowns == nil {
			motion.SpellCategoryCooldowns = make(map[uint32]time.Time)
		}
		motion.SpellCategoryCooldowns[categoryID] = categoryEnd
	}
	motion.LastSpell = now
	s.server.motionMu.Unlock()
}

// triggerPetGlobalCooldown mirrors Spell::TriggerGlobalCooldown
// (Spell.cpp:8232-8261) for a pet/charmed caster: the cooldown is keyed by
// the spell's StartRecoveryCategory (SpellHistory::AddGlobalCooldown,
// SpellHistory.cpp:591-594). The haste-scaling arm (8249-8258) has no Go
// bridge — the pet's UNIT_MOD_CAST_SPEED is unmodeled (pet_combat.go:1547);
// the 1000..1500 clamp only applies inside that arm, so the raw
// StartRecoveryTime is recorded. SPELLMOD_GLOBAL_COOLDOWN folds through the
// owner player in C++ (Object.cpp:2246); Go leaves it unapplied here, the
// same gap the player path documents (gcd.go:31).
func (s *Server) triggerPetGlobalCooldown(motion *creatureMotion, spell wotlk.Spell, now time.Time) {
	if s == nil || motion == nil || spell.StartRecoveryCategory == 0 {
		return
	}
	gcdMs := int64(spell.StartRecoveryTime)
	if gcdMs == 0 {
		return
	}
	s.motionMu.Lock()
	if motion.GCDCooldowns == nil {
		motion.GCDCooldowns = make(map[uint32]time.Time)
	}
	for category, end := range motion.GCDCooldowns {
		if !end.After(now) {
			delete(motion.GCDCooldowns, category)
		}
	}
	motion.GCDCooldowns[spell.StartRecoveryCategory] = now.Add(time.Duration(gcdMs) * time.Millisecond)
	s.motionMu.Unlock()
}

// petGlobalCooldownActive mirrors SpellHistory::HasGlobalCooldown
// (SpellHistory.cpp:585-589): the lookup is keyed by the spell's own
// StartRecoveryCategory.
func (s *Server) petGlobalCooldownActive(motion *creatureMotion, spell wotlk.Spell, now time.Time) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	end, ok := motion.GCDCooldowns[spell.StartRecoveryCategory]
	return ok && end.After(now)
}

// resetPetSpellCooldownCheat mirrors the cheat arm of Spell::_cast's modOwner
// tail (Spell.cpp:3517-3522) for a pet caster: GetSpellModOwner returns the
// owner player for pets (Object.cpp:2246 — only Pet objects carry
// UNIT_MASK_PET, Pet.cpp:49), and with .cheat cooldown on that player the
// just-cast spell's own RecoveryTime cooldown entry on the original caster
// (the pet) is erased. Like C++ SpellHistory::ResetCooldown(spellId, true)
// only the spell's own entry is cleared — category cooldowns are untouched
// (SpellHistory.cpp:441-455). SMSG_CLEAR_COOLDOWN (spell id then caster
// guid) goes to the owner player, which is this session. The
// SetSpellModTakingSpell(this, false) arm of the same block has no reader
// on the pet path — the Go pet path has no spell-mod-taking model
// (documented standing gap) — so it stays unbridged. Runs for triggered
// casts too: the C++ block carries no triggered exemption.
func (s *session) resetPetSpellCooldownCheat(motion *creatureMotion, spellID uint32) {
	if s == nil || s.player == nil || s.player.ActiveCheats&cheatCooldown == 0 {
		return
	}
	if motion == nil || s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	_, removed := motion.SpellCooldowns[spellID]
	if removed {
		delete(motion.SpellCooldowns, spellID)
		delete(motion.SpellCooldownCategories, spellID)
		delete(motion.SpellCooldownCategoryEnds, spellID)
	}
	s.server.motionMu.Unlock()
	if !removed {
		return
	}
	buf := protocol.NewBuffer(4 + 8)
	buf.WriteU32(spellID)
	buf.WriteU64(motion.GUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buf.Bytes(), true)
}
