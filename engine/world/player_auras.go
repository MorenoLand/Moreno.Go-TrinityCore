package world

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *session) loadPlayerAuras(ctx context.Context, state *playerState) error {
	if s == nil || state == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	fullState := true
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT casterGuid, itemGuid, spell, effectMask, recalculateMask, stackCount,
		amount0, amount1, amount2, base_amount0, base_amount1, base_amount2, maxDuration, remainTime, remainCharges, critChance, applyResilience
		FROM character_aura WHERE guid = ? ORDER BY spell`, state.GUID)
	if err != nil {
		if errorsMissingAuraTable(err) {
			fullState = false
			rows, err = s.server.CharactersStore.DB.QueryContext(ctx, `SELECT casterGuid, itemGuid, spell, effectMask, stackCount,
				amount0, maxDuration, remainTime, remainCharges FROM character_aura WHERE guid = ? ORDER BY spell`, state.GUID)
			if err != nil {
				if errorsMissingAuraTable(err) {
					return nil
				}
				return err
			}
		} else {
			return err
		}
	}
	defer rows.Close()
	s.castMu.Lock()
	s.auras = make(map[uint32]struct{})
	s.auraSlots = make(map[uint32]uint8)
	s.activeAuras = make(map[uint32]*activeAura)
	offlineMs := int64(0)
	if state.LogoutTime > 0 && time.Now().Unix() > state.LogoutTime {
		offlineMs = (time.Now().Unix() - state.LogoutTime) * 1000
	}
	var periodic []*activeAura
	for rows.Next() {
		var casterGUID, itemGUID uint64
		var spellID, effectMask, recalculateMask, stackCount, maxDuration, remainTime, remainCharges int64
		var amounts, baseAmounts [3]int64
		var critChance float64
		var applyResilience bool
		var scanErr error
		if fullState {
			scanErr = rows.Scan(&casterGUID, &itemGUID, &spellID, &effectMask, &recalculateMask, &stackCount, &amounts[0], &amounts[1], &amounts[2], &baseAmounts[0], &baseAmounts[1], &baseAmounts[2], &maxDuration, &remainTime, &remainCharges, &critChance, &applyResilience)
		} else {
			scanErr = rows.Scan(&casterGUID, &itemGUID, &spellID, &effectMask, &stackCount, &amounts[0], &maxDuration, &remainTime, &remainCharges)
		}
		if scanErr != nil {
			continue
		}
		if spellID <= 0 || effectMask <= 0 || effectMask&^int64(0x07) != 0 || (remainTime == 0 || remainTime < -1) || spellID > int64(^uint32(0)) || len(s.activeAuras) >= 255 {
			continue
		}
		id := uint32(spellID)
		if casterGUID == 0 {
			casterGUID = state.GUID
		}
		if id == 8326 || id == 20584 {
			state.PlayerFlags |= playerFlagGhost
		}
		if _, exists := s.activeAuras[id]; exists {
			continue
		}
		if s.server.Data != nil {
			spell, found, spellErr := s.server.Data.Spell(id)
			if spellErr != nil || !found {
				continue
			}
			if spell.Attributes&spellAttributePassive != 0 || spell.AttributesEx1&(spellAttr1Channeled1|spellAttr1Channeled2) != 0 {
				continue
			}
			switch id {
			case 44413, 40075, 55849, 73822, 73828:
				continue
			}
			if itemGUID != 0 && maxDuration <= 0 {
				continue
			}
			unsavable := false
			for index, effect := range spell.Effects {
				if effectMask&(1<<uint(index)) == 0 {
					continue
				}
				switch effect.Aura {
				case 1, 2, 6, 128, 177, 236, 249, 292:
					unsavable = true
				}
			}
			if unsavable {
				continue
			}
		}
		aura := &activeAura{SpellID: id, CasterGUID: casterGUID, TargetGUID: state.GUID, ItemGUID: itemGUID, EffectMask: uint8(effectMask) & 0x07, RecalculateMask: uint8(recalculateMask), CritChance: float32(critChance), ApplyResilience: applyResilience, Slot: uint8(len(s.activeAuras)), Positive: true, CasterLevel: state.Level}
		for index := range amounts {
			aura.Amounts[index] = int32(amounts[index])
			aura.BaseAmounts[index] = int32(baseAmounts[index])
		}
		if maxDuration > 0 {
			aura.DurationMs = clampAuraDuration(maxDuration)
		}
		if remainTime > 0 {
			aura.RemainingMs = clampAuraDuration(remainTime)
		}
		if stackCount > 0 {
			aura.StackCount = uint8(stackCount)
		}
		if remainCharges > 0 {
			aura.RemainingCharges = uint8(remainCharges)
		}
		if s.server.Data != nil {
			if spell, found, _ := s.server.Data.Spell(id); found {
				aura.DispelType = spell.DispelType
				aura.Mechanic = spell.Mechanic
				aura.SchoolMask = spell.SchoolMask
				aura.AuraInterruptFlags = spell.AuraInterruptFlags
				for index, effect := range spell.Effects {
					if effect.Effect == 0 || effect.Aura == 0 || effectMask&(1<<uint(index)) == 0 {
						continue
					}
					aura.AuraType = effect.Aura
					aura.MiscValue = effect.MiscValue
					if aura.Amounts[index] > 0 {
						aura.Amount = uint32(aura.Amounts[index])
					}
					if effect.AuraPeriod > 0 {
						aura.PeriodMs = effect.AuraPeriod
					}
					if aura.Amount == 0 && effect.BasePoints >= 0 {
						aura.Amount = uint32(effect.BasePoints + 1)
					}
					break
				}
				aura.Positive = !isHarmfulAura(aura.AuraType)
				if aura.AuraType == spellAuraStun || aura.AuraType == spellAuraRoot {
					s.rooted = true
					if aura.AuraType == spellAuraStun {
						state.UnitFlags |= unitFlagStunned
					}
				}
				aura.StackAmount = spell.StackAmount
				aura.HideDuration = spell.AttributesEx5&0x00000400 != 0
				if spell.ProcCharges > 0 {
					if aura.RemainingCharges == 0 || aura.RemainingCharges > uint8(spell.ProcCharges) {
						aura.RemainingCharges = uint8(spell.ProcCharges)
					}
				} else {
					aura.RemainingCharges = 0
				}
			}
		}
		fadesWhileOffline := false
		resSicknessSpellID := uint32(15007)
		if s.server.Data != nil {
			if race, found, _ := s.server.Data.Race(uint32(state.Race)); found && race.ResSicknessSpellID != 0 {
				resSicknessSpellID = race.ResSicknessSpellID
			}
			if spell, found, _ := s.server.Data.Spell(id); found {
				fadesWhileOffline = spell.AttributesEx4&0x00000004 != 0 && id != resSicknessSpellID
			}
		}
		if (!aura.Positive || fadesWhileOffline) && aura.RemainingMs > 0 && offlineMs > 0 {
			if offlineMs >= int64(aura.RemainingMs) {
				continue
			}
			aura.RemainingMs -= uint32(offlineMs)
		}
		if aura.DurationMs > 0 && aura.RemainingMs > aura.DurationMs {
			aura.RemainingMs = aura.DurationMs
		}
		s.auras[id] = struct{}{}
		s.auraSlots[id] = aura.Slot
		s.activeAuras[id] = aura
		if aura.PeriodMs > 0 && (aura.RemainingMs > 0 || aura.DurationMs == 0) {
			periodic = append(periodic, aura)
		}
	}
	rowErr := rows.Err()
	s.castMu.Unlock()
	if rowErr != nil {
		return rowErr
	}
	for _, aura := range periodic {
		s.schedulePlayerPeriodicTick(aura, aura.PeriodMs)
	}
	for _, aura := range s.loadedAuras() {
		if aura.DurationMs > 0 && aura.RemainingMs > 0 {
			aura.Timer = time.AfterFunc(time.Duration(aura.RemainingMs)*time.Millisecond, func(spellID uint32) func() {
				return func() { s.expirePlayerAura(spellID) }
			}(aura.SpellID))
		}
	}
	return nil
}

func (s *session) loadGlyphAuras(state *playerState) {
	if s == nil || state == nil || s.server == nil || s.server.Data == nil {
		return
	}
	s.castMu.Lock()
	if s.auras == nil {
		s.auras = make(map[uint32]struct{})
	}
	if s.auraSlots == nil {
		s.auraSlots = make(map[uint32]uint8)
	}
	if s.activeAuras == nil {
		s.activeAuras = make(map[uint32]*activeAura)
	}
	for index, glyphID := range state.Glyphs[state.ActiveTalentGroup] {
		if glyphID == 0 {
			continue
		}
		glyph, found, err := s.server.Data.GlyphProperties(uint32(glyphID))
		if err != nil || !found || glyph.SpellID == 0 {
			state.Glyphs[state.ActiveTalentGroup][index] = 0
			continue
		}
		slotType, slotFound, slotErr := s.server.Data.GlyphSlotType(state.GlyphSlots[index])
		if slotErr != nil || !slotFound || slotType != glyph.GlyphSlotFlags {
			state.Glyphs[state.ActiveTalentGroup][index] = 0
			continue
		}
		if _, exists := s.activeAuras[glyph.SpellID]; exists {
			continue
		}
		spell, found, err := s.server.Data.Spell(glyph.SpellID)
		if err != nil || !found {
			continue
		}
		aura := &activeAura{SpellID: glyph.SpellID, CasterGUID: state.GUID, TargetGUID: state.GUID, EffectMask: 0x01, Slot: uint8(len(s.activeAuras)), Positive: true, CasterLevel: state.Level}
		for _, effect := range spell.Effects {
			if effect.Effect == 0 || effect.Aura == 0 {
				continue
			}
			aura.AuraType = effect.Aura
			aura.MiscValue = effect.MiscValue
			if effect.BasePoints >= 0 {
				aura.Amount = uint32(effect.BasePoints + 1)
			}
			aura.PeriodMs = effect.AuraPeriod
			aura.Positive = !isHarmfulAura(effect.Aura)
			aura.StackAmount = spell.StackAmount
			aura.HideDuration = spell.AttributesEx5&0x00000400 != 0
			break
		}
		if aura.AuraType == 0 {
			continue
		}
		if aura.EffectMask == 0 {
			aura.EffectMask = 0x01
		}
		s.auras[glyph.SpellID] = struct{}{}
		s.auraSlots[glyph.SpellID] = aura.Slot
		s.activeAuras[glyph.SpellID] = aura
	}
	s.castMu.Unlock()
}

func errorsMissingAuraTable(err error) bool {
	value := strings.ToLower(err.Error())
	return sql.ErrNoRows == err || strings.Contains(value, "no such table") || strings.Contains(value, "no such column") || strings.Contains(value, "unknown column")
}

func clampAuraDuration(value int64) uint32 {
	if value <= 0 {
		return 0
	}
	if value > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(value)
}

func (s *session) loadedAuras() []*activeAura {
	s.castMu.Lock()
	result := make([]*activeAura, 0, len(s.activeAuras))
	for _, aura := range s.activeAuras {
		result = append(result, aura)
	}
	s.castMu.Unlock()
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result
}

func (s *session) sendLoadedAuras() {
	auras := s.loadedAuras()
	if len(auras) == 0 {
		return
	}
	records := make([]protocol.AuraUpdateRecord, 0, len(auras))
	for _, aura := range auras {
		stackCount := aura.StackCount
		if aura.StackAmount == 0 {
			stackCount = aura.RemainingCharges
		}
		maxDuration, duration := aura.DurationMs, aura.RemainingMs
		if aura.HideDuration {
			maxDuration, duration = 0, 0
		}
		records = append(records, protocol.AuraUpdateRecord{CasterGUID: aura.CasterGUID, Slot: aura.Slot, SpellID: aura.SpellID, EffectMask: aura.EffectMask, Positive: aura.Positive, MaxDurationMs: maxDuration, DurationMs: duration, CasterLevel: aura.CasterLevel, StackCount: stackCount})
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE_ALL), protocol.BuildAuraUpdateAll(s.playerGUID, records), true)
}
