package world

import (
	"context"
)

func (s *session) applyOwnerPetAuras(ctx context.Context, petEntry uint32, petGUID uint64) {
	if s == nil || s.server == nil || s.player == nil || petGUID == 0 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	s.server.motionMu.Unlock()
	if motion == nil {
		return
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM character_spell WHERE guid = ? AND active <> 0 AND disabled = 0 ORDER BY spell", s.playerGUID)
	if err != nil {
		return
	}
	spells := make([]uint32, 0)
	for rows.Next() {
		var spellID uint32
		if rows.Scan(&spellID) == nil && spellID > 0 {
			spells = append(spells, spellID)
		}
	}
	rowErr := rows.Err()
	_ = rows.Close()
	if rowErr != nil {
		return
	}
	for _, ownerSpellID := range spells {
		ownerSpell, found, err := s.server.Data.Spell(ownerSpellID)
		if err != nil || !found {
			continue
		}
		if ownerSpell.Attributes&spellAttributePassive == 0 {
			s.castMu.Lock()
			_, active := s.activeAuras[ownerSpellID]
			s.castMu.Unlock()
			if !active {
				continue
			}
		}
		for effectIndex, ownerEffect := range ownerSpell.Effects {
			if ownerEffect.Effect != 3 && (ownerEffect.Effect != 6 || ownerEffect.Aura != 4) {
				continue
			}
			var auraID int64
			err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT aura FROM spell_pet_auras WHERE spell = ? AND effectId = ? AND pet IN (0, ?)
				ORDER BY CASE WHEN pet = ? THEN 0 ELSE 1 END, pet LIMIT 1`, ownerSpellID, effectIndex, petEntry, petEntry).Scan(&auraID)
			if err != nil || auraID <= 0 || auraID > int64(^uint32(0)) || uint32(auraID) == 35696 {
				continue
			}
			auraSpell, found, err := s.server.Data.Spell(uint32(auraID))
			if err != nil || !found {
				continue
			}
			auraEffectIndex, auraEffectCount := 0, 0
			for index, effect := range auraSpell.Effects {
				if effect.Effect != 0 && effect.Aura != 0 {
					auraEffectIndex, auraEffectCount = index, auraEffectCount+1
				}
			}
			if auraEffectCount != 1 {
				continue
			}
			auraEffect := auraSpell.Effects[auraEffectIndex]
			amount := auraEffect.BasePoints + 1
			if amount < 0 {
				continue
			}
			durationMs := uint32(0)
			if auraSpell.DurationIndex > 0 {
				if duration, durationFound, durationErr := s.server.Data.SpellDuration(auraSpell.DurationIndex, motion.Level); durationErr == nil && durationFound && duration > 0 && uint64(duration) <= uint64(^uint32(0)) {
					durationMs = uint32(duration)
				}
			}
			if !s.applyPetAura(ctx, motion, auraSpell, auraEffect, petGUID, durationMs, uint32(amount)) {
				continue
			}
			s.server.auraMu.Lock()
			if aura := s.server.activeCreatureAuras[petGUID][auraSpell.ID]; aura != nil {
				aura.OwnerPetAura = true
			}
			s.server.auraMu.Unlock()
		}
	}
}
