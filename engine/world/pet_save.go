package world

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

type petAuraStoredRow struct {
	values  [17]any
	spellID uint32
}

func (s *session) savePetState(ctx context.Context, tx *sql.Tx, saveMode ...uint8) error {
	if s == nil || s.player == nil || s.player.PetGUID == 0 {
		return nil
	}
	state := s.player
	if tx == nil || s.server == nil || s.server.CharactersStore == nil {
		return errors.New("active pet save requires a transaction and character store")
	}
	mode := petSaveAsCurrent
	if len(saveMode) > 1 {
		return errors.New("pet save accepts at most one save mode")
	}
	if len(saveMode) == 1 {
		mode = saveMode[0]
	}
	petGUID := state.PetGUID
	petID := state.PetNumber
	if petID == 0 {
		petID = uint32(petGUID)
	}
	if petID == 0 || state.GUID == 0 {
		return errors.New("active pet save has an invalid owner or pet GUID")
	}
	if mode == petSaveAsDeleted {
		if _, err := tx.ExecContext(ctx, "DELETE FROM character_pet WHERE owner = ? AND id = ?", state.GUID, petID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM character_pet_declinedname WHERE owner = ? AND id = ?", state.GUID, petID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ?", petID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ?", petID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM pet_spell_cooldown WHERE guid = ?", petID); err != nil {
			return err
		}
		return nil
	}
	if mode != petSaveAsCurrent && mode != petSaveNotInSlot {
		return errors.New("unknown pet save mode")
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	if motion == nil || motion.GUID != petGUID || motion.Entry == 0 || motion.OwnerGUID != state.GUID {
		s.server.motionMu.Unlock()
		return nil
	}
	petLevel, health, mana, happiness, experience, reactState := motion.Level, motion.Health, motion.Mana, motion.Happiness, motion.Experience, motion.PetReact
	spellCooldowns := make(map[uint32]time.Time, len(motion.SpellCooldowns))
	for spellID, castAt := range motion.SpellCooldowns {
		spellCooldowns[spellID] = castAt
	}
	s.server.motionMu.Unlock()
	if s.server.Data == nil {
		return errors.New("active pet save requires game data")
	}
	now := time.Now()
	petSlot := uint8(0)
	if mode == petSaveNotInSlot {
		petSlot = petStorageSlotNotInSlot
	}
	result, err := tx.ExecContext(ctx, "UPDATE character_pet SET slot = ?, level = ?, curhealth = ?, curmana = ?, curhappiness = ?, exp = ?, Reactstate = ?, savetime = ? WHERE owner = ? AND id = ?", petSlot, petLevel, health, mana, happiness, experience, reactState, now.Unix(), state.GUID, petID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		var exists uint8
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM character_pet WHERE owner = ? AND id = ?", state.GUID, petID).Scan(&exists); err != nil {
			return err
		}
	}
	s.server.auraMu.Lock()
	auras := make([]activeAura, 0, len(s.server.activeCreatureAuras[petGUID]))
	for _, aura := range s.server.activeCreatureAuras[petGUID] {
		if aura == nil || aura.Stopped || aura.TargetGUID != petGUID {
			continue
		}
		snapshot := *aura
		advanceAuraDuration(&snapshot, now)
		auras = append(auras, snapshot)
	}
	s.server.auraMu.Unlock()
	sort.Slice(auras, func(i, j int) bool {
		if auras[i].Slot != auras[j].Slot {
			return auras[i].Slot < auras[j].Slot
		}
		return auras[i].SpellID < auras[j].SpellID
	})
	unreadable, maxDurations, err := unreadablePetAuraRows(ctx, tx, petID)
	if err != nil {
		if isMissingAuraTableError(err) {
			return nil
		}
		return err
	}
	type petAuraInsert struct {
		spellID uint32
		args    []any
	}
	toInsert := make([]petAuraInsert, 0, len(auras))
	for _, aura := range auras {
		if aura.OwnerPetAura {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if aura.Amounts == [3]int32{} && aura.BaseAmounts == [3]int32{} && aura.EffectMask != 0 && aura.EffectMask&(aura.EffectMask-1) == 0 {
			for index, effect := range spell.Effects {
				if aura.EffectMask&(1<<uint(index)) != 0 {
					setAuraEffectPersistence(&aura, spell, effect, aura.Amount)
					break
				}
			}
		}
		saveable, err := s.canSavePlayerAura(petGUID, &aura, spell)
		if err != nil {
			return err
		}
		if !saveable || aura.DurationMs > 0 && aura.RemainingMs == 0 {
			continue
		}
		remaining := int64(-1)
		if aura.DurationMs > 0 {
			remaining = int64(aura.RemainingMs)
		}
		maxDuration := int64(aura.DurationMs)
		if aura.DurationMs == 0 {
			key := auraSaveKey{spell: aura.SpellID, caster: aura.CasterGUID}
			saved, ok := maxDurations[key]
			if !ok && aura.CasterGUID == petGUID {
				saved, ok = maxDurations[auraSaveKey{spell: aura.SpellID}]
			}
			if ok {
				maxDuration = saved
			} else {
				level := uint32(aura.CasterLevel)
				if level == 0 {
					level = petLevel
				}
				duration, found, err := s.server.Data.SpellDuration(spell.DurationIndex, level)
				if err != nil {
					return err
				}
				if found {
					maxDuration = int64(duration)
				}
			}
		}
		stackCount := aura.StackCount
		if stackCount == 0 {
			stackCount = 1
		}
		casterGUID := aura.CasterGUID
		if casterGUID == petGUID {
			casterGUID = 0
		}
		toInsert = append(toInsert, petAuraInsert{spellID: aura.SpellID, args: []any{petID, auraGUIDDatabaseValue(s.server.CharactersStore.Backend, casterGUID), aura.SpellID, aura.EffectMask, aura.RecalculateMask, stackCount, aura.Amounts[0], aura.Amounts[1], aura.Amounts[2], aura.BaseAmounts[0], aura.BaseAmounts[1], aura.BaseAmounts[2], maxDuration, remaining, aura.RemainingCharges, aura.CritChance, aura.ApplyResilience}})
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ?", petID); err != nil {
		if isMissingAuraTableError(err) {
			return nil
		}
		return err
	}
	saveableSpellIDs := make(map[uint32]struct{}, len(toInsert))
	for _, aura := range toInsert {
		saveableSpellIDs[aura.spellID] = struct{}{}
	}
	const insertPetAura = "INSERT INTO pet_aura (guid, casterGuid, spell, effectMask, recalculateMask, stackCount, amount0, amount1, amount2, base_amount0, base_amount1, base_amount2, maxDuration, remainTime, remainCharges, critChance, applyResilience) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	for _, row := range unreadable {
		if _, ok := saveableSpellIDs[row.spellID]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, insertPetAura, row.values[:]...); err != nil {
			return err
		}
	}
	for _, aura := range toInsert {
		if _, err := tx.ExecContext(ctx, insertPetAura, aura.args...); err != nil {
			return err
		}
	}
	type savedCooldown struct {
		spell, category  uint32
		end, categoryEnd int64
	}
	nowUnix := now.Unix()
	cooldownRows, err := tx.QueryContext(ctx, "SELECT spell, categoryId, time, categoryEnd FROM pet_spell_cooldown WHERE guid = ?", petID)
	if err != nil && !missingTable(err) {
		return err
	}
	cooldowns := make(map[uint32]savedCooldown)
	if err == nil {
		for cooldownRows.Next() {
			var row savedCooldown
			if scanErr := cooldownRows.Scan(&row.spell, &row.category, &row.end, &row.categoryEnd); scanErr != nil {
				_ = cooldownRows.Close()
				return scanErr
			}
			if row.end >= nowUnix {
				cooldowns[row.spell] = row
			}
		}
		if err := cooldownRows.Err(); err != nil {
			_ = cooldownRows.Close()
			return err
		}
		if err := cooldownRows.Close(); err != nil {
			return err
		}
	}
	for spellID, castAt := range spellCooldowns {
		spell, found, err := s.server.Data.Spell(spellID)
		if err != nil {
			return err
		}
		if !found || castAt.IsZero() {
			continue
		}
		categoryID, categoryRecoveryTime, err := s.spellCooldownCategory(spellID)
		if err != nil {
			return err
		}
		if spell.RecoveryTime == 0 && categoryRecoveryTime == 0 {
			continue
		}
		end := nowUnix
		if spell.RecoveryTime > 0 {
			end = castAt.Add(time.Duration(spell.RecoveryTime) * time.Millisecond).Unix()
		}
		categoryEnd := int64(0)
		if categoryID != 0 && categoryRecoveryTime > 0 {
			categoryEnd = castAt.Add(time.Duration(categoryRecoveryTime) * time.Millisecond).Unix()
		}
		if end < nowUnix {
			delete(cooldowns, spellID)
			continue
		}
		cooldowns[spellID] = savedCooldown{spell: spellID, category: categoryID, end: end, categoryEnd: categoryEnd}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pet_spell_cooldown WHERE guid = ?", petID); err != nil && !missingTable(err) {
		return err
	}
	spellIDs := make([]uint32, 0, len(cooldowns))
	for spellID := range cooldowns {
		spellIDs = append(spellIDs, spellID)
	}
	sort.Slice(spellIDs, func(i, j int) bool { return spellIDs[i] < spellIDs[j] })
	for _, spellID := range spellIDs {
		cooldown := cooldowns[spellID]
		if _, err := tx.ExecContext(ctx, "INSERT INTO pet_spell_cooldown (guid, spell, time, categoryId, categoryEnd) VALUES (?, ?, ?, ?, ?)", petID, cooldown.spell, cooldown.end, cooldown.category, cooldown.categoryEnd); err != nil {
			return err
		}
	}
	return nil
}

func unreadablePetAuraRows(ctx context.Context, tx *sql.Tx, petID uint32) ([]petAuraStoredRow, map[auraSaveKey]int64, error) {
	rows, err := tx.QueryContext(ctx, "SELECT guid, casterGuid, spell, effectMask, recalculateMask, stackCount, amount0, amount1, amount2, base_amount0, base_amount1, base_amount2, maxDuration, remainTime, remainCharges, critChance, applyResilience FROM pet_aura WHERE guid = ?", petID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var unreadable []petAuraStoredRow
	maxDurations := make(map[auraSaveKey]int64)
	for rows.Next() {
		var row petAuraStoredRow
		destinations := make([]any, len(row.values))
		for index := range row.values {
			destinations[index] = &row.values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, nil, err
		}
		spellID, spellOK := auraUint32(row.values[2])
		casterGUID, casterOK := uint64(0), row.values[1] == nil
		if row.values[1] != nil {
			casterGUID, casterOK = auraGUIDUint64(row.values[1])
		}
		maxDuration, durationOK := auraInt64(row.values[12])
		if spellOK && casterOK && durationOK {
			maxDurations[auraSaveKey{spell: spellID, caster: casterGUID}] = maxDuration
		}
		if row.values[1] == nil || !casterOK {
			row.spellID = spellID
			unreadable = append(unreadable, row)
		}
	}
	return unreadable, maxDurations, rows.Err()
}
