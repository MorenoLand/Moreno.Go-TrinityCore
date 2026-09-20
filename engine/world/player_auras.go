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
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT casterGuid, itemGuid, spell, effectMask, stackCount,
		amount0, maxDuration, remainTime, remainCharges FROM character_aura WHERE guid = ? ORDER BY spell`, state.GUID)
	if err != nil {
		if errorsMissingAuraTable(err) {
			return nil
		}
		return err
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
		var spellID, effectMask, stackCount, amount, maxDuration, remainTime, remainCharges int64
		if err := rows.Scan(&casterGUID, &itemGUID, &spellID, &effectMask, &stackCount, &amount, &maxDuration, &remainTime, &remainCharges); err != nil {
			continue
		}
		if spellID <= 0 || (remainTime == 0 || remainTime < -1) || spellID > int64(^uint32(0)) || len(s.activeAuras) >= 64 {
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
		aura := &activeAura{SpellID: id, CasterGUID: casterGUID, TargetGUID: state.GUID, EffectMask: uint8(effectMask) & 0x07, Slot: uint8(len(s.activeAuras)), Positive: true, CasterLevel: state.Level}
		if maxDuration > 0 {
			aura.DurationMs = clampAuraDuration(maxDuration)
		}
		if remainTime > 0 {
			aura.RemainingMs = clampAuraDuration(remainTime)
		}
		if amount > 0 {
			aura.Amount = uint32(amount)
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
					if effect.AuraPeriod > 0 {
						aura.PeriodMs = effect.AuraPeriod
					}
					if aura.Amount == 0 && effect.BasePoints >= 0 {
						aura.Amount = uint32(effect.BasePoints + 1)
					}
					break
				}
				aura.Positive = !isHarmfulAura(aura.AuraType)
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
		if s.server.Data != nil {
			if spell, found, _ := s.server.Data.Spell(id); found {
				fadesWhileOffline = spell.AttributesEx4&0x00000004 != 0 && id != 15007
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
	for _, glyphID := range state.Glyphs[state.ActiveTalentGroup] {
		if glyphID == 0 {
			continue
		}
		glyph, found, err := s.server.Data.GlyphProperties(uint32(glyphID))
		if err != nil || !found || glyph.SpellID == 0 {
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
