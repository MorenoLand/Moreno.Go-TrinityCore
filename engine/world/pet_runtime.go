package world

import (
	"context"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	petFocusRegenInterval      = 4 * time.Second
	petHappinessInterval       = 7500 * time.Millisecond
	petPowerTypeFocus          = 2
	spellAuraPowerRegen        = 85
	spellAuraPowerRegenPercent = 110
	spellAuraIncreaseXPPercent = 200
)

type PetRuntimeState struct {
	PetType         uint8
	PowerType       uint32
	UnitFlags2      uint32
	Powers          [7]uint32
	MaxPowers       [7]uint32
	FocusRegenTimer time.Duration
	HappinessTimer  time.Duration
	InCombat        bool
}

type PetFocusModifier struct {
	SpellID    uint32
	Effect     uint8
	AuraType   uint32
	Amount     int32
	StackGroup uint32
}

type petAuraStackKey struct {
	SpellID  uint32
	AuraType uint32
}

type petAuraStackCache struct {
	groupBySpell map[petAuraStackKey]uint32
	firstRank    map[uint32]uint32
}

func ResolvePetFocusRegen(rate float64, modifiers []PetFocusModifier) int32 {
	flatTotal := int32(0)
	flatGroups := make(map[uint32]int32)
	for _, modifier := range modifiers {
		switch modifier.AuraType {
		case spellAuraPowerRegen:
			if modifier.StackGroup == 0 {
				flatTotal += modifier.Amount
			} else if current, found := flatGroups[modifier.StackGroup]; !found || absAuraAmount(modifier.Amount) > absAuraAmount(current) {
				flatGroups[modifier.StackGroup] = modifier.Amount
			}
		}
	}
	groupIDs := make([]uint32, 0, len(flatGroups))
	for groupID := range flatGroups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	for _, groupID := range groupIDs {
		flatTotal += flatGroups[groupID]
	}
	flatGain := uint32(flatTotal) * uint32(petFocusRegenInterval/time.Millisecond) / 5000
	return int32(float32(24*float32(rate))*ResolveAuraPercentMultiplierByType(modifiers, spellAuraPowerRegenPercent) + float32(flatGain))
}

func absAuraAmount(amount int32) int64 {
	value := int64(amount)
	if value < 0 {
		return -value
	}
	return value
}

func ResolveAuraPercentMultiplierByType(modifiers []PetFocusModifier, auraType uint32) float32 {
	multiplier := float32(1)
	groups := make(map[uint32]int32)
	for _, modifier := range modifiers {
		if modifier.AuraType != auraType {
			continue
		}
		if modifier.StackGroup == 0 {
			multiplier *= (100 + float32(modifier.Amount)) / 100
		} else if current, found := groups[modifier.StackGroup]; !found || absAuraAmount(modifier.Amount) > absAuraAmount(current) {
			groups[modifier.StackGroup] = modifier.Amount
		}
	}
	groupIDs := make([]uint32, 0, len(groups))
	for groupID := range groups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	for _, groupID := range groupIDs {
		multiplier *= (100 + float32(groups[groupID])) / 100
	}
	return multiplier
}

func (s *Server) petAuraStackGroups() *petAuraStackCache {
	if s == nil {
		return &petAuraStackCache{}
	}
	s.petAuraStackMu.Lock()
	defer s.petAuraStackMu.Unlock()
	if s.petAuraStackCache != nil {
		return s.petAuraStackCache
	}
	cache := &petAuraStackCache{groupBySpell: make(map[petAuraStackKey]uint32), firstRank: make(map[uint32]uint32)}
	if s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		s.petAuraStackCache = cache
		return cache
	}
	ranks, err := s.WorldStore.DB.QueryContext(context.Background(), "SELECT spell_id, first_spell_id FROM spell_ranks")
	if err == nil {
		for ranks.Next() {
			var spellID, firstRank uint32
			if ranks.Scan(&spellID, &firstRank) == nil {
				cache.firstRank[spellID] = firstRank
			}
		}
		_ = ranks.Close()
	}
	groupRows, err := s.WorldStore.DB.QueryContext(context.Background(), "SELECT id, spell_id FROM spell_group")
	if err != nil {
		s.petAuraStackCache = cache
		return cache
	}
	groupMembers := make(map[uint32][]int64)
	for groupRows.Next() {
		var groupID uint32
		var spellID int64
		if groupRows.Scan(&groupID, &spellID) == nil {
			groupMembers[groupID] = append(groupMembers[groupID], spellID)
		}
	}
	_ = groupRows.Close()
	ruleRows, err := s.WorldStore.DB.QueryContext(context.Background(), "SELECT group_id FROM spell_group_stack_rules WHERE stack_rule = 3 ORDER BY group_id")
	if err != nil {
		s.petAuraStackCache = cache
		return cache
	}
	var groups []uint32
	for ruleRows.Next() {
		var groupID uint32
		if ruleRows.Scan(&groupID) == nil {
			groups = append(groups, groupID)
		}
	}
	_ = ruleRows.Close()
	var expand func(uint32, map[uint32]struct{}, map[uint32]struct{})
	expand = func(groupID uint32, visited, spellIDs map[uint32]struct{}) {
		if _, found := visited[groupID]; found {
			return
		}
		visited[groupID] = struct{}{}
		for _, member := range groupMembers[groupID] {
			if member < 0 {
				expand(uint32(-member), visited, spellIDs)
			} else if member <= int64(^uint32(0)) {
				spellIDs[uint32(member)] = struct{}{}
			}
		}
	}
	for _, groupID := range groups {
		spellIDs := make(map[uint32]struct{})
		expand(groupID, make(map[uint32]struct{}), spellIDs)
		spellList := make([]uint32, 0, len(spellIDs))
		frequency := make(map[uint32]int)
		for spellID := range spellIDs {
			spellList = append(spellList, spellID)
		}
		sort.Slice(spellList, func(i, j int) bool { return spellList[i] < spellList[j] })
		for _, spellID := range spellList {
			spell, found, err := s.Data.Spell(spellID)
			if err != nil || !found {
				continue
			}
			for _, effect := range spell.Effects {
				if effect.Effect != 0 && effect.Aura != 0 {
					frequency[effect.Aura]++
				}
			}
		}
		var auraTypes []uint32
		for auraType := range frequency {
			auraTypes = append(auraTypes, auraType)
		}
		sort.Slice(auraTypes, func(i, j int) bool { return auraTypes[i] < auraTypes[j] })
		var selectedAura uint32
		var selectedCount int
		for _, auraType := range auraTypes {
			if frequency[auraType] > selectedCount {
				selectedAura, selectedCount = auraType, frequency[auraType]
			}
		}
		if selectedAura == 0 {
			continue
		}
		for _, spellID := range spellList {
			spell, found, err := s.Data.Spell(spellID)
			if err != nil || !found {
				continue
			}
			for _, effect := range spell.Effects {
				if effect.Aura == selectedAura {
					key := petAuraStackKey{SpellID: spellID, AuraType: selectedAura}
					if cache.groupBySpell[key] == 0 {
						cache.groupBySpell[key] = groupID
					}
				}
			}
		}
	}
	s.petAuraStackCache = cache
	return cache
}

func (s *Server) petAuraStackGroup(spellID, auraType uint32) uint32 {
	if s == nil {
		return 0
	}
	cache := s.petAuraStackGroups()
	if firstRank, found := cache.firstRank[spellID]; found {
		spellID = firstRank
	}
	return cache.groupBySpell[petAuraStackKey{SpellID: spellID, AuraType: auraType}]
}

func (s *Server) petFocusAuraModifiers(petGUID uint64) []PetFocusModifier {
	if s == nil || s.Data == nil || petGUID == 0 {
		return nil
	}
	s.auraMu.Lock()
	active := make([]activeAura, 0, len(s.activeCreatureAuras[petGUID]))
	for _, aura := range s.activeCreatureAuras[petGUID] {
		if aura != nil && !aura.Stopped {
			active = append(active, *aura)
		}
	}
	s.auraMu.Unlock()
	sort.Slice(active, func(i, j int) bool { return active[i].SpellID < active[j].SpellID })
	modifiers := make([]PetFocusModifier, 0)
	for _, aura := range active {
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Effect == 0 || effect.MiscValue != petPowerTypeFocus || effect.Aura != spellAuraPowerRegen && effect.Aura != spellAuraPowerRegenPercent || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 && aura.AuraType == effect.Aura && aura.Amount != 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 && aura.BaseAmounts[index] == 0 && effect.BasePoints != -1 {
				amount = effect.BasePoints + 1
			}
			groupID := s.petAuraStackGroup(aura.SpellID, effect.Aura)
			modifiers = append(modifiers, PetFocusModifier{SpellID: aura.SpellID, Effect: uint8(index), AuraType: effect.Aura, Amount: amount, StackGroup: groupID})
		}
	}
	sort.SliceStable(modifiers, func(i, j int) bool {
		if modifiers[i].SpellID != modifiers[j].SpellID {
			return modifiers[i].SpellID < modifiers[j].SpellID
		}
		return modifiers[i].Effect < modifiers[j].Effect
	})
	return modifiers
}

func AdvancePetRuntime(state PetRuntimeState, diff time.Duration, focusRate float64) (PetRuntimeState, map[int]uint32) {
	return advancePetRuntime(state, diff, ResolvePetFocusRegen(focusRate, nil))
}

func AdvancePetRuntimeWithAuras(state PetRuntimeState, diff time.Duration, focusRate float64, modifiers []PetFocusModifier) (PetRuntimeState, map[int]uint32) {
	return advancePetRuntime(state, diff, ResolvePetFocusRegen(focusRate, modifiers))
}

func advancePetRuntime(state PetRuntimeState, diff time.Duration, focusGain int32) (PetRuntimeState, map[int]uint32) {
	if diff <= 0 {
		return state, nil
	}
	var fields map[int]uint32
	if state.FocusRegenTimer > 0 {
		if state.FocusRegenTimer > diff {
			state.FocusRegenTimer -= diff
		} else if state.PowerType == petPowerTypeFocus {
			if state.UnitFlags2&unitFlag2RegeneratePower != 0 && state.Powers[2] < state.MaxPowers[2] {
				value := int64(state.Powers[2]) + int64(focusGain)
				if value < 0 {
					value = 0
				} else if value > int64(state.MaxPowers[2]) {
					value = int64(state.MaxPowers[2])
				}
				if uint32(value) != state.Powers[2] {
					state.Powers[2] = uint32(value)
					fields = map[int]uint32{unitFieldPower1 + 2: state.Powers[2]}
				}
			}
			state.FocusRegenTimer += petFocusRegenInterval - diff
			if state.FocusRegenTimer == 0 {
				state.FocusRegenTimer = time.Millisecond
			}
			if state.FocusRegenTimer < 0 || state.FocusRegenTimer > petFocusRegenInterval {
				state.FocusRegenTimer = petFocusRegenInterval
			}
		} else {
			state.FocusRegenTimer = 0
		}
	}
	if state.PetType == 1 {
		if state.HappinessTimer <= diff {
			loss := uint32(670)
			if state.InCombat {
				loss = uint32(float32(loss) * 1.5)
			}
			value := uint32(0)
			if state.Powers[4] > loss {
				value = state.Powers[4] - loss
			}
			if value != state.Powers[4] {
				state.Powers[4] = value
				if fields == nil {
					fields = make(map[int]uint32, 2)
				}
				fields[unitFieldPower1+4] = value
			}
			state.HappinessTimer = petHappinessInterval
		} else {
			state.HappinessTimer -= diff
		}
	}
	return state, fields
}

func (s *Server) updatePetRuntime(now time.Time, diff time.Duration) {
	if s == nil || diff <= 0 {
		return
	}
	type petUpdate struct {
		mapID        uint32
		guid         uint64
		ownerGUID    uint64
		currentPower uint32
		fields       map[int]uint32
	}
	updates := make([]petUpdate, 0)
	motions := make([]*creatureMotion, 0)
	seen := make(map[*creatureMotion]struct{})
	s.motionMu.Lock()
	for _, motion := range s.creatureMotion {
		if motion == nil || motion.PetID == 0 || motion.OwnerGUID == 0 || motion.Health == 0 {
			continue
		}
		if _, ok := seen[motion]; ok {
			continue
		}
		seen[motion] = struct{}{}
		motions = append(motions, motion)
	}
	s.motionMu.Unlock()
	for _, motion := range motions {
		s.motionMu.Lock()
		if s.creatureMotion[motion.GUID] != motion || motion.Health == 0 {
			s.motionMu.Unlock()
			continue
		}
		focusDue := motion.PowerType == petPowerTypeFocus && motion.FocusRegenTimer > 0 && motion.FocusRegenTimer <= diff
		guid := motion.GUID
		s.motionMu.Unlock()
		var modifiers []PetFocusModifier
		if focusDue {
			modifiers = s.petFocusAuraModifiers(guid)
		}
		s.motionMu.Lock()
		if s.creatureMotion[guid] != motion || motion.Health == 0 {
			s.motionMu.Unlock()
			continue
		}
		prunePetSpellCooldowns(motion, now)
		motion.Refreshed = now
		state := PetRuntimeState{PetType: motion.PetType, PowerType: motion.PowerType, UnitFlags2: motion.UnitFlags2, Powers: motion.Powers, MaxPowers: motion.MaxPowers, FocusRegenTimer: motion.FocusRegenTimer, HappinessTimer: motion.HappinessTimer, InCombat: motion.InCombat || motion.UnitFlags&unitFlagInCombat != 0}
		var fields map[int]uint32
		if focusDue {
			state, fields = AdvancePetRuntimeWithAuras(state, diff, s.Config.FocusRate, modifiers)
		} else {
			state, fields = AdvancePetRuntime(state, diff, s.Config.FocusRate)
		}
		motion.Powers = state.Powers
		motion.FocusRegenTimer = state.FocusRegenTimer
		motion.HappinessTimer = state.HappinessTimer
		motion.Happiness = state.Powers[4]
		if len(fields) != 0 {
			currentPower := uint32(0)
			if motion.PowerType < uint32(len(motion.Powers)) {
				currentPower = motion.Powers[motion.PowerType]
			}
			updates = append(updates, petUpdate{mapID: motion.Map, guid: motion.GUID, ownerGUID: motion.OwnerGUID, currentPower: currentPower, fields: fields})
		}
		s.motionMu.Unlock()
	}
	for _, update := range updates {
		s.broadcastCreatureValuesUpdate(update.mapID, update.guid, update.fields)
		changedPower := false
		for powerType := uint32(0); powerType < 7; powerType++ {
			power, ok := update.fields[unitFieldPower1+int(powerType)]
			if !ok {
				continue
			}
			changedPower = true
			packet := protocol.NewBuffer(packedGUIDSize(update.guid) + 5)
			packet.WritePackedGUID(update.guid)
			packet.WriteU8(uint8(powerType))
			packet.WriteU32(power)
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), nil)
		}
		if changedPower {
			s.sendGroupPetCurrentPower(update.ownerGUID, update.currentPower)
		}
	}
}
