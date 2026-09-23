package world

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

func (s *session) savePlayerAuras(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || tx == nil || state == nil || s.server == nil || s.server.CharactersStore == nil || s.server.Data == nil {
		return nil
	}
	now := time.Now()
	s.castMu.Lock()
	auras := make([]activeAura, 0, len(s.activeAuras))
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped || aura.TargetGUID != state.GUID {
			continue
		}
		snapshot := *aura
		advanceAuraDuration(&snapshot, now)
		auras = append(auras, snapshot)
	}
	s.castMu.Unlock()
	sort.Slice(auras, func(i, j int) bool {
		if auras[i].Slot != auras[j].Slot {
			return auras[i].Slot < auras[j].Slot
		}
		return auras[i].SpellID < auras[j].SpellID
	})
	unreadable, maxDurations, err := unreadableAuraRows(ctx, tx, state.GUID)
	if err != nil && !isMissingAuraTableError(err) {
		return err
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_CHAR_AURA", state.GUID); err != nil {
		if errorsMissingAuraTable(err) {
			return nil
		}
		return err
	}
	for _, row := range unreadable {
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_AURA", row[:]...); err != nil {
			return err
		}
	}
	for _, aura := range auras {
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		saveable, err := s.canSavePlayerAura(state.GUID, &aura, spell)
		if err != nil {
			return err
		}
		if !saveable || (aura.DurationMs > 0 && aura.RemainingMs == 0) {
			continue
		}
		remaining := int64(-1)
		if aura.DurationMs > 0 {
			remaining = int64(aura.RemainingMs)
		}
		maxDuration := int64(aura.DurationMs)
		if aura.DurationMs == 0 {
			key := auraSaveKey{spell: aura.SpellID, caster: aura.CasterGUID, item: aura.ItemGUID}
			if saved, ok := maxDurations[key]; ok {
				maxDuration = saved
			} else {
				level := uint32(aura.CasterLevel)
				if level == 0 {
					level = uint32(state.Level)
				}
				duration, found, durationErr := s.server.Data.SpellDuration(spell.DurationIndex, level)
				if durationErr != nil {
					return durationErr
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
		_, err = s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_AURA", state.GUID, aura.CasterGUID, aura.ItemGUID, aura.SpellID, aura.EffectMask, aura.RecalculateMask, stackCount, aura.Amounts[0], aura.Amounts[1], aura.Amounts[2], aura.BaseAmounts[0], aura.BaseAmounts[1], aura.BaseAmounts[2], maxDuration, remaining, aura.RemainingCharges, aura.CritChance, aura.ApplyResilience)
		if err != nil {
			return err
		}
	}
	return nil
}

type auraSaveKey struct {
	spell  uint32
	caster uint64
	item   uint64
}

func unreadableAuraRows(ctx context.Context, tx *sql.Tx, guid uint64) ([][18]any, map[auraSaveKey]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT guid, casterGuid, itemGuid, spell, effectMask, recalculateMask, stackCount, amount0, amount1, amount2, base_amount0, base_amount1, base_amount2, maxDuration, remainTime, remainCharges, critChance, applyResilience FROM character_aura WHERE guid = ?`, guid)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var unreadable [][18]any
	maxDurations := make(map[auraSaveKey]int64)
	for rows.Next() {
		var row [18]any
		destinations := make([]any, len(row))
		for index := range row {
			destinations[index] = &row[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, nil, err
		}
		spell, spellOK := auraUint32(row[3])
		caster, casterOK := auraGUIDUint64(row[1])
		item, itemOK := auraGUIDUint64(row[2])
		maxDuration, durationOK := auraInt64(row[13])
		if spellOK && casterOK && itemOK && durationOK {
			maxDurations[auraSaveKey{spell: spell, caster: caster, item: item}] = maxDuration
		}
		if !auraGUIDValueReadable(row[1]) || !auraGUIDValueReadable(row[2]) {
			unreadable = append(unreadable, row)
		}
	}
	return unreadable, maxDurations, rows.Err()
}

func auraGUIDValueReadable(value any) bool {
	_, ok := auraGUIDUint64(value)
	return ok
}

func auraGUIDUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case int64:
		return uint64(value), value >= 0
	case int32:
		return uint64(value), value >= 0
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case []byte:
		parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func auraUint32(value any) (uint32, bool) {
	parsed, ok := auraInt64(value)
	return uint32(parsed), ok && parsed >= 0 && parsed <= math.MaxUint32
}

func auraInt64(value any) (int64, bool) {
	switch value := value.(type) {
	case int64:
		return value, true
	case int32:
		return int64(value), true
	case uint64:
		return int64(value), value <= math.MaxInt64
	case uint32:
		return int64(value), true
	case float64:
		return int64(value), math.Trunc(value) == value && value >= float64(math.MinInt64) && value <= float64(math.MaxInt64)
	case float32:
		number := float64(value)
		return int64(number), math.Trunc(number) == number && number >= float64(math.MinInt64) && number <= float64(math.MaxInt64)
	case []byte:
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func isMissingAuraTableError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "no such table") || strings.Contains(value, "doesn't exist") || strings.Contains(value, "unknown table")
}

func (s *session) canSavePlayerAura(ownerGUID uint64, aura *activeAura, spell wotlk.Spell) (bool, error) {
	if aura == nil || aura.EffectMask == 0 || spell.Attributes&spellAttributePassive != 0 || spell.AttributesEx1&(spellAttr1Channeled1|spellAttr1Channeled2) != 0 {
		return false, nil
	}
	if s.server.getSpellCustomAttr(spell.ID)&0x00400000 != 0 {
		return false, nil
	}
	liquid, err := s.server.Data.IsLiquidAuraSpell(spell.ID)
	if err != nil {
		return false, err
	}
	if liquid {
		return false, nil
	}
	switch spell.ID {
	case 44413, 40075, 55849, 73822, 73828:
		return false, nil
	}
	if aura.CasterGUID != ownerGUID {
		if aura.SingleTarget || isSingleTargetAuraSpell(spell) {
			return false, nil
		}
		for _, effect := range spell.Effects {
			if effect.Effect != 0 && (isAreaAuraEffect(effect.Effect) || isAreaAuraTarget(effect.ImplicitTargetA) || isAreaAuraTarget(effect.ImplicitTargetB)) {
				return false, nil
			}
		}
	}
	for index, effect := range spell.Effects {
		if aura.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		switch effect.Aura {
		case 1, 2, 6, 128, 177, 236, 249, 292:
			return false, nil
		}
	}
	if spell.ProcCharges > 0 && aura.RemainingCharges == 0 || aura.ItemGUID != 0 && aura.DurationMs == 0 {
		return false, nil
	}
	return true, nil
}

func isSingleTargetAuraSpell(spell wotlk.Spell) bool {
	if spell.AttributesEx5&0x00000020 != 0 {
		return true
	}
	if spell.SpellFamilyName != 10 {
		return false
	}
	if spell.SpellFamilyFlags[1]&0x26000C00 != 0 || spell.SpellFamilyFlags[0]&0x0A000000 != 0 || spell.SpellFamilyFlags[0]&0x00002190 != 0 {
		return false
	}
	return spell.ID == 20184 || spell.ID == 20185 || spell.ID == 20186
}

func isSingleTargetAuraSpellID(data *wotlk.Store, spellID uint32) bool {
	if data == nil {
		return false
	}
	spell, found, err := data.Spell(spellID)
	return err == nil && found && isSingleTargetAuraSpell(spell)
}

func (s *session) startPlayerAuraDurations() {
	if s == nil {
		return
	}
	now := time.Now()
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura != nil && aura.DurationMs > 0 && aura.DurationUpdatedAt.IsZero() {
			aura.DurationUpdatedAt = now
		}
	}
	s.castMu.Unlock()
}

func advanceAuraDuration(aura *activeAura, now time.Time) {
	if aura == nil || aura.DurationMs == 0 || aura.RemainingMs == 0 {
		return
	}
	if aura.DurationUpdatedAt.IsZero() {
		aura.DurationUpdatedAt = now
		return
	}
	elapsed := now.Sub(aura.DurationUpdatedAt).Milliseconds()
	if elapsed <= 0 {
		return
	}
	if elapsed >= int64(aura.RemainingMs) {
		aura.RemainingMs = 0
	} else {
		aura.RemainingMs -= uint32(elapsed)
	}
	aura.DurationUpdatedAt = now
}

func isAreaAuraEffect(effect uint32) bool {
	switch effect {
	case 35, 65, 119, 128, 129, 143:
		return true
	default:
		return false
	}
}

func isAreaAuraTarget(target uint32) bool {
	switch target {
	case 7, 8, 15, 16, 20, 24, 30, 31, 33, 34, 37, 51, 52, 54, 56, 59, 60, 61, 93, 104, 108:
		return true
	default:
		return false
	}
}

func setAuraEffectPersistence(aura *activeAura, spell wotlk.Spell, effect wotlk.SpellEffect, amount uint32) {
	if aura == nil {
		return
	}
	for index, candidate := range spell.Effects {
		if candidate != effect {
			continue
		}
		mask := uint8(1 << uint(index))
		aura.EffectMask = mask
		aura.Amounts[index] = int32(amount)
		aura.BaseAmounts[index] = candidate.BasePoints
		if auraEffectCanBeRecalculated(candidate.Aura) {
			aura.RecalculateMask |= mask
		} else {
			aura.RecalculateMask &^= mask
		}
		return
	}
}

func auraEffectCanBeRecalculated(auraType uint32) bool {
	switch auraType {
	case spellAuraConfuse, spellAuraFear, spellAuraStun, spellAuraRoot, 56, 69, 97:
		return false
	default:
		return true
	}
}
