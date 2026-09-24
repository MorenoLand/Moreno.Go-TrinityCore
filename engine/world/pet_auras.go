package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

type ownerPetAuraKey struct {
	SpellID     uint32
	EffectIndex uint8
}

type ownerPetAuraSource struct {
	AuraByPet      map[uint32]uint32
	Damage         int32
	RemoveOnChange bool
}

func PetAuraEffectMask(spell wotlk.Spell) uint8 {
	var mask uint8
	for index, effect := range spell.Effects {
		if index < 8 && effect.Effect != 0 && effect.Aura != 0 {
			mask |= 1 << uint(index)
		}
	}
	return mask
}

func ResolveOwnerPetAuraAmount(auraSpellID uint32, effectIndex uint8, basePoints, sourceDamage int32, stats [5]uint32) int32 {
	if auraSpellID == 35696 && effectIndex == 0 {
		return int32(int64(sourceDamage) * int64(stats[2]+stats[3]) / 100)
	}
	return basePoints
}

func (s *session) readOwnerPetAuraSource(ctx context.Context, spellID uint32, effectIndex uint8) (ownerPetAuraSource, bool) {
	if s == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return ownerPetAuraSource{}, false
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || int(effectIndex) >= len(spell.Effects) {
		return ownerPetAuraSource{}, false
	}
	effect := spell.Effects[effectIndex]
	if effect.Effect != 3 && (effect.Effect != 6 || effect.Aura != 4) {
		return ownerPetAuraSource{}, false
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT pet, aura FROM spell_pet_auras WHERE spell = ? AND effectId = ? ORDER BY pet", spellID, effectIndex)
	if err != nil {
		return ownerPetAuraSource{}, false
	}
	defer rows.Close()
	source := ownerPetAuraSource{AuraByPet: make(map[uint32]uint32), Damage: effect.CalcValue(), RemoveOnChange: effect.ImplicitTargetA == 5}
	for rows.Next() {
		var petID, auraID int64
		if rows.Scan(&petID, &auraID) != nil || petID < 0 || petID > int64(^uint32(0)) || auraID <= 0 || auraID > int64(^uint32(0)) {
			continue
		}
		if _, found, err := s.server.Data.Spell(uint32(auraID)); err != nil || !found {
			continue
		}
		source.AuraByPet[uint32(petID)] = uint32(auraID)
	}
	return source, rows.Err() == nil && len(source.AuraByPet) != 0
}

func (source ownerPetAuraSource) auraForPet(entry uint32) uint32 {
	if aura := source.AuraByPet[entry]; aura != 0 {
		return aura
	}
	return source.AuraByPet[0]
}

func (s *session) addOwnerPetAuraSource(ctx context.Context, spellID uint32, effectIndex uint8) bool {
	source, found := s.readOwnerPetAuraSource(ctx, spellID, effectIndex)
	if !found {
		return false
	}
	key := ownerPetAuraKey{SpellID: spellID, EffectIndex: effectIndex}
	s.ownerPetAuraMu.Lock()
	if s.ownerPetAuraSources == nil {
		s.ownerPetAuraSources = make(map[ownerPetAuraKey]ownerPetAuraSource)
	}
	s.ownerPetAuraSources[key] = source
	s.ownerPetAuraMu.Unlock()
	if s.player != nil && s.player.PetGUID != 0 {
		return s.castOwnerPetAuraSource(ctx, key, source, s.player.PetGUID)
	}
	return true
}

func (s *session) castOwnerPetAuraSource(ctx context.Context, key ownerPetAuraKey, source ownerPetAuraSource, petGUID uint64) bool {
	if s == nil || s.server == nil || s.server.Data == nil || petGUID == 0 {
		return false
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	s.server.motionMu.Unlock()
	if motion == nil || motion.OwnerGUID != s.playerGUID {
		return false
	}
	auraSpellID := source.auraForPet(motion.Entry)
	if auraSpellID == 0 {
		return false
	}
	s.server.auraMu.Lock()
	active := s.server.activeCreatureAuras[petGUID][auraSpellID]
	alreadyApplied := active != nil && active.OwnerPetAura && active.OwnerPetAuraSourceSpell == key.SpellID && active.OwnerPetAuraSourceEffect == key.EffectIndex
	s.server.auraMu.Unlock()
	if alreadyApplied {
		return true
	}
	auraSpell, found, err := s.server.Data.Spell(auraSpellID)
	if err != nil || !found {
		return false
	}
	effectIndex := -1
	for index, effect := range auraSpell.Effects {
		if effect.Effect != 0 && effect.Aura != 0 {
			effectIndex = index
			break
		}
	}
	if effectIndex < 0 {
		return false
	}
	effect := auraSpell.Effects[effectIndex]
	amount := ResolveOwnerPetAuraAmount(auraSpellID, uint8(effectIndex), effect.CalcValueForLevel(auraSpell, uint32(motion.Level)), source.Damage, motion.Stats)
	var durationMs uint32
	if auraSpell.DurationIndex > 0 {
		if duration, ok, err := s.server.Data.SpellDuration(auraSpell.DurationIndex, motion.Level); err == nil && ok && duration > 0 && uint64(duration) <= uint64(^uint32(0)) {
			durationMs = uint32(duration)
		}
	}
	return s.applyPetAuraWithSource(ctx, motion, auraSpell, effect, petGUID, durationMs, amount, key, source.Damage, source.RemoveOnChange)
}

func (s *session) removeOwnerPetAuraSource(ctx context.Context, key ownerPetAuraKey, petGUID uint64) {
	if s == nil || s.server == nil {
		return
	}
	s.ownerPetAuraMu.Lock()
	source, found := s.ownerPetAuraSources[key]
	delete(s.ownerPetAuraSources, key)
	s.ownerPetAuraMu.Unlock()
	if !found {
		return
	}
	if petGUID == 0 && s.player != nil {
		petGUID = s.player.PetGUID
	}
	if petGUID == 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	s.server.motionMu.Unlock()
	if motion != nil {
		if auraSpellID := source.auraForPet(motion.Entry); auraSpellID != 0 {
			s.server.removeCreatureAura(petGUID, auraSpellID)
		}
	}
}

func (s *session) removeOwnerPetAuraEffects(ctx context.Context, spellID uint32, effectMask uint8) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	for index, effect := range spell.Effects {
		if effect.Aura == 4 && effectMask&(1<<uint(index)) != 0 {
			s.removeOwnerPetAuraSource(ctx, ownerPetAuraKey{SpellID: spellID, EffectIndex: uint8(index)}, 0)
		}
	}
}

func (s *session) addOwnerPetAuraEffects(ctx context.Context, spellID uint32, effectMask uint8) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	for index, effect := range spell.Effects {
		if effect.Aura == 4 && effectMask&(1<<uint(index)) != 0 {
			s.addOwnerPetAuraSource(ctx, spellID, uint8(index))
		}
	}
}

func (s *session) learnOwnerPetAuraSources(ctx context.Context, spellID uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || spell.Attributes&spellAttributePassive == 0 {
		return
	}
	for index, effect := range spell.Effects {
		if effect.Effect == 6 && effect.Aura == 4 {
			s.addOwnerPetAuraSource(ctx, spellID, uint8(index))
		}
	}
}

func (s *session) removeOwnerPetAurasForSpell(ctx context.Context, spellID uint32) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	for index, effect := range spell.Effects {
		if effect.Effect == 3 || effect.Effect == 6 && effect.Aura == 4 {
			s.removeOwnerPetAuraSource(ctx, ownerPetAuraKey{SpellID: spellID, EffectIndex: uint8(index)}, 0)
		}
	}
}

func (s *session) removeOwnerPetAuraSourcesOnPetChange(ctx context.Context, petGUID uint64) {
	if s == nil || s.server == nil || petGUID == 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	s.server.motionMu.Unlock()
	if motion == nil {
		return
	}
	s.ownerPetAuraMu.Lock()
	removed := make(map[ownerPetAuraKey]ownerPetAuraSource)
	for key, source := range s.ownerPetAuraSources {
		if source.RemoveOnChange {
			removed[key] = source
			delete(s.ownerPetAuraSources, key)
		}
	}
	s.ownerPetAuraMu.Unlock()
	for _, source := range removed {
		if auraSpellID := source.auraForPet(motion.Entry); auraSpellID != 0 {
			s.server.removeCreatureAura(petGUID, auraSpellID)
		}
	}
}

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
	s.ownerPetAuraMu.Lock()
	loadSources := !s.ownerPetAuraSourcesLoaded
	s.ownerPetAuraMu.Unlock()
	if loadSources {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM character_spell WHERE guid = ? AND active <> 0 AND disabled = 0 ORDER BY spell", s.playerGUID)
		if err != nil {
			return
		}
		discovered := make(map[ownerPetAuraKey]ownerPetAuraSource)
		for rows.Next() {
			var ownerSpellID uint32
			if rows.Scan(&ownerSpellID) != nil || ownerSpellID == 0 {
				continue
			}
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
				key := ownerPetAuraKey{SpellID: ownerSpellID, EffectIndex: uint8(effectIndex)}
				if source, found := s.readOwnerPetAuraSource(ctx, key.SpellID, key.EffectIndex); found {
					discovered[key] = source
				}
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return
		}
		if err := rows.Close(); err != nil {
			return
		}
		s.ownerPetAuraMu.Lock()
		if s.ownerPetAuraSources == nil {
			s.ownerPetAuraSources = make(map[ownerPetAuraKey]ownerPetAuraSource)
		}
		for key, source := range discovered {
			if _, exists := s.ownerPetAuraSources[key]; !exists {
				s.ownerPetAuraSources[key] = source
			}
		}
		s.ownerPetAuraSourcesLoaded = true
		s.ownerPetAuraMu.Unlock()
	}
	s.ownerPetAuraMu.Lock()
	sources := make(map[ownerPetAuraKey]ownerPetAuraSource, len(s.ownerPetAuraSources))
	for key, source := range s.ownerPetAuraSources {
		sources[key] = source
	}
	s.ownerPetAuraMu.Unlock()
	for key, source := range sources {
		s.castOwnerPetAuraSource(ctx, key, source, petGUID)
	}
}
