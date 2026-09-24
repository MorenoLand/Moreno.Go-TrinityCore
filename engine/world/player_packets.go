package world

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type learnedSpell struct {
	ID        uint32
	Active    bool
	Disabled  bool
	Dependent bool
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

type spellCooldown struct {
	Spell       uint32
	Item        uint32
	Category    uint32
	End         int64
	CategoryEnd int64
}

func (s *session) loadPlayerPacketsState(ctx context.Context, state *playerState) error {
	spells, err := s.loadLearnedSpells(ctx, state.GUID, state.Race, state.Class, state.Level)
	if err != nil {
		return err
	}
	actions, err := s.loadActionButtons(ctx, state.GUID, spells)
	if err != nil {
		return err
	}
	cooldowns, err := s.loadSpellCooldowns(ctx, state.GUID)
	if err != nil {
		return err
	}
	state.Spells, state.Actions, state.Cooldowns = spells, actions, cooldowns
	return nil
}

func (s *session) resetSpellsAtLogin(ctx context.Context) error {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	previous := append([]learnedSpell(nil), s.player.Spells...)
	cdb := s.server.CharactersStore.DB
	if _, err := cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ?", s.playerGUID); err != nil && !missingTable(err) {
		return err
	}
	for _, spell := range previous {
		if spell.ID == 0 {
			continue
		}
		packet := protocol.NewBuffer(4)
		packet.WriteU32(spell.ID)
		if err := s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), packet.Bytes(), true); err != nil {
			return err
		}
	}
	spells, err := s.loadLearnedSpells(ctx, s.playerGUID, s.player.Race, s.player.Class, s.player.Level)
	if err != nil {
		return err
	}
	s.player.Spells = spells
	for _, spell := range spells {
		if !spell.Active || spell.Disabled {
			continue
		}
		packet := protocol.NewBuffer(6)
		packet.WriteU32(spell.ID)
		packet.WriteU16(0)
		if err := s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), packet.Bytes(), true); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) loadLearnedSpells(ctx context.Context, guid uint64, race, class, level uint8) ([]learnedSpell, error) {
	defaults := defaultRacialSpells(race)
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return defaults, nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell, active, disabled FROM character_spell WHERE guid = ? ORDER BY spell", guid)
	if err != nil {
		return defaults, nil
	}
	result := make([]learnedSpell, 0)
	for rows.Next() {
		var spell, active, disabled int64
		if err := rows.Scan(&spell, &active, &disabled); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if spell <= 0 || spell > int64(^uint32(0)) {
			continue
		}
		spellID := uint32(spell)
		if s.server.Data != nil {
			if _, found, spellErr := s.server.Data.Spell(spellID); spellErr != nil || !found {
				s.debug("unknown persisted spell skipped", "guid", guid, "spell", spellID)
				continue
			}
		}
		result = append(result, learnedSpell{ID: spellID, Active: active != 0, Disabled: disabled != 0})
	}
	_ = rows.Close()

	for _, def := range defaults {
		found := false
		for _, sp := range result {
			if sp.ID == def.ID {
				found = true
				break
			}
		}
		if !found {
			result = append(result, def)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, def.ID)
		}
	}
	// If player has few spells, ensure custom starter spells from playercreateinfo_spell_custom are also learned (if PlayerStart.AllSpells enabled)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && s.server.Config.PlayerStartAllSpells {
		raceMask, classMask := playerCreateMask(race), playerCreateMask(class)
		crows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT Spell FROM playercreateinfo_spell_custom WHERE (racemask = 0 OR (racemask & ?) <> 0) AND (classmask = 0 OR (classmask & ?) <> 0)", raceMask, classMask)
		if err == nil {
			defer crows.Close()
			for crows.Next() {
				var customSpell int64
				if err := crows.Scan(&customSpell); err == nil && customSpell > 0 {
					id := uint32(customSpell)
					if s.server.Data != nil {
						if _, found, spellErr := s.server.Data.Spell(id); spellErr != nil || !found {
							continue
						}
					}
					found := false
					for _, sp := range result {
						if sp.ID == id {
							found = true
							break
						}
					}
					if !found {
						result = append(result, learnedSpell{ID: id, Active: true})
						_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, id)
					}
				}
			}
		}
	}
	if s.server != nil && s.server.Data != nil {
		for index := 0; index < len(result); index++ {
			spell := result[index]
			if _, _, talent := s.server.Data.TalentBySpell(spell.ID); talent {
				continue
			}
			previous := s.server.getPrevSpellInChain(spell.ID)
			for previous != 0 {
				if _, found, spellErr := s.server.Data.Spell(previous); spellErr != nil || !found {
					break
				}
				foundIndex := -1
				for existing := range result {
					if result[existing].ID == previous {
						foundIndex = existing
						break
					}
				}
				if foundIndex < 0 {
					result = append(result, learnedSpell{ID: previous, Active: spell.Active, Disabled: spell.Disabled, Dependent: true})
					_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, ?, ?)", guid, previous, boolToInt(spell.Active), boolToInt(spell.Disabled))
				} else {
					result[foundIndex].Active, result[foundIndex].Disabled, result[foundIndex].Dependent = spell.Active, spell.Disabled, true
					_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_spell SET active = ?, disabled = ? WHERE guid = ? AND spell = ?", boolToInt(spell.Active), boolToInt(spell.Disabled), guid, previous)
				}
				if _, _, talent := s.server.Data.TalentBySpell(previous); talent {
					break
				}
				previous = s.server.getPrevSpellInChain(previous)
			}
		}
	}
	if s.server != nil && s.server.Data != nil {
		selectedTalentSpell := make(map[uint32]uint32)
		selectedTalentRank := make(map[uint32]uint8)
		for _, spell := range result {
			if talentID, rank, found := s.server.Data.TalentBySpell(spell.ID); found && (selectedTalentSpell[talentID] == 0 || rank > selectedTalentRank[talentID]) {
				selectedTalentSpell[talentID], selectedTalentRank[talentID] = spell.ID, rank
			}
		}
		filtered := make([]learnedSpell, 0, len(result))
		for _, spell := range result {
			if talentID, _, found := s.server.Data.TalentBySpell(spell.ID); found && selectedTalentSpell[talentID] != spell.ID {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", guid, spell.ID)
				continue
			}
			filtered = append(filtered, spell)
		}
		result = filtered
		for index := range result {
			if !result[index].Active || result[index].Disabled {
				continue
			}
			stackable, found, err := s.server.Data.SpellStackableWithRanks(result[index].ID)
			if err != nil {
				return nil, err
			}
			if !found || stackable {
				continue
			}
			seenRanks := map[uint32]struct{}{result[index].ID: {}}
			for previous := s.server.getPrevSpellInChain(result[index].ID); previous != 0; previous = s.server.getPrevSpellInChain(previous) {
				if _, seen := seenRanks[previous]; seen {
					break
				}
				seenRanks[previous] = struct{}{}
				for previousIndex := range result {
					if result[previousIndex].ID == previous && result[previousIndex].Active && !result[previousIndex].Disabled {
						result[previousIndex].Active = false
						_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_spell SET active = 0 WHERE guid = ? AND spell = ?", guid, previous)
						break
					}
				}
			}
		}
	}
	return result, nil
}

func (s *session) loadFirstLoginCastSpellIDs(ctx context.Context, race, class uint8) []uint32 {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return nil
	}
	raceMask, classMask := playerCreateMask(race), playerCreateMask(class)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell FROM playercreateinfo_cast_spell WHERE (raceMask = 0 OR (raceMask & ?) <> 0) AND (classMask = 0 OR (classMask & ?) <> 0) ORDER BY spell", raceMask, classMask)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]uint32, 0)
	for rows.Next() {
		var spellID int64
		if err := rows.Scan(&spellID); err == nil && spellID > 0 && spellID <= int64(^uint32(0)) {
			result = append(result, uint32(spellID))
		}
	}
	return result
}

func playerCreateMask(id uint8) uint32 {
	if id == 0 || id > 32 {
		return 0
	}
	return uint32(1) << (id - 1)
}

func defaultRacialSpells(race uint8) []learnedSpell {
	spells := make([]learnedSpell, 0, 4)
	spells = append(spells, learnedSpell{ID: 6603, Active: true})
	switch race {
	case 1:
		spells = append(spells, learnedSpell{ID: 668, Active: true})
	case 2:
		spells = append(spells, learnedSpell{ID: 669, Active: true})
	case 3:
		spells = append(spells, learnedSpell{ID: 668, Active: true}, learnedSpell{ID: 672, Active: true})
	case 4:
		spells = append(spells, learnedSpell{ID: 668, Active: true}, learnedSpell{ID: 671, Active: true})
	case 5:
		spells = append(spells, learnedSpell{ID: 669, Active: true}, learnedSpell{ID: 17737, Active: true})
	case 6:
		spells = append(spells, learnedSpell{ID: 669, Active: true}, learnedSpell{ID: 670, Active: true})
	case 7:
		spells = append(spells, learnedSpell{ID: 668, Active: true}, learnedSpell{ID: 7340, Active: true})
	case 8:
		spells = append(spells, learnedSpell{ID: 669, Active: true}, learnedSpell{ID: 7341, Active: true})
	case 10:
		spells = append(spells, learnedSpell{ID: 669, Active: true}, learnedSpell{ID: 813, Active: true})
	case 11:
		spells = append(spells, learnedSpell{ID: 668, Active: true}, learnedSpell{ID: 29932, Active: true})
	default:
		spells = append(spells, learnedSpell{ID: 668, Active: true})
	}
	return spells
}

func isLanguageSpell(spellID uint32) bool {
	switch spellID {
	case 668, 669, 670, 671, 672, 813, 7340, 7341, 17737, 29932:
		return true
	}
	return false
}

func (s *session) loadActionButtons(ctx context.Context, guid uint64, spells []learnedSpell) ([144]uint32, error) {
	var result [144]uint32
	knownSpells := make(map[uint32]struct{}, len(spells))
	for _, spell := range spells {
		if spell.Active && !spell.Disabled {
			knownSpells[spell.ID] = struct{}{}
		}
	}
	validAction := func(action, kind int64) bool {
		if action < 0 || action >= 0x01000000 || kind < 0 || kind > 255 {
			return false
		}
		switch uint8(kind) {
		case 0:
			if _, ok := knownSpells[uint32(action)]; !ok {
				return false
			}
			if s.server.Data == nil {
				return true
			}
			_, found, err := s.server.Data.Spell(uint32(action))
			return err == nil && found
		case 1, 0x20, 0x40, 0x41:
			return true
		case 0x80:
			if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
				return true
			}
			var found int64
			err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM item_template WHERE entry = ? LIMIT 1", action).Scan(&found)
			return err == nil && found != 0
		default:
			return false
		}
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT button, action, type FROM character_action WHERE guid = ? AND spec = (SELECT activeTalentGroup FROM characters WHERE guid = ?) ORDER BY button", guid, guid)
	if err != nil {
		if missingTable(err) || isMissingColumn(err) {
			return result, nil
		}
		return result, err
	}
	for rows.Next() {
		var button, action, kind int64
		if err := rows.Scan(&button, &action, &kind); err != nil {
			return result, err
		}
		if button >= 0 && button < int64(len(result)) && validAction(action, kind) {
			result[button] = uint32(action) | uint32(kind)<<24
			continue
		}
		s.debug("invalid action button skipped", "guid", guid, "button", button, "action", action, "type", kind)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	return result, nil
}

func (s *session) loadSpellCooldowns(ctx context.Context, guid uint64) ([]spellCooldown, error) {
	now := time.Now().Unix()
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell, item, categoryId, time, categoryEnd FROM character_spell_cooldown WHERE guid = ? AND time > ? ORDER BY spell", guid, now)
	if err != nil {
		if missingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	result := make([]spellCooldown, 0)
	for rows.Next() {
		var spell, item, category, end, categoryEnd int64
		if err := rows.Scan(&spell, &item, &category, &end, &categoryEnd); err != nil {
			return nil, err
		}
		result = append(result, spellCooldown{Spell: uint32(spell), Item: uint32(item), Category: uint32(category), End: end, CategoryEnd: categoryEnd})
	}
	return result, rows.Err()
}

func buildInitialSpells(state playerState) []byte {
	spells := append([]learnedSpell(nil), state.Spells...)
	sort.Slice(spells, func(i, j int) bool { return spells[i].ID < spells[j].ID })
	packet := protocol.NewBuffer(8 + len(spells)*6 + len(state.Cooldowns)*16)
	packet.WriteU8(0)
	count := 0
	for _, spell := range spells {
		if spell.Active && !spell.Disabled {
			count++
		}
	}
	packet.WriteU16(uint16(count))
	for _, spell := range spells {
		if spell.Active && !spell.Disabled {
			packet.WriteU32(spell.ID)
			packet.WriteU16(0)
		}
	}
	now := time.Now().Unix()
	cooldowns := append([]spellCooldown(nil), state.Cooldowns...)
	sort.Slice(cooldowns, func(i, j int) bool { return cooldowns[i].Spell < cooldowns[j].Spell })
	packet.WriteU16(uint16(len(cooldowns)))
	for _, cooldown := range cooldowns {
		packet.WriteU32(cooldown.Spell)
		packet.WriteU16(uint16(cooldown.Item))
		packet.WriteU16(uint16(cooldown.Category))
		if cooldown.End >= now+15*24*60*60 {
			packet.WriteU32(1)
			packet.WriteU32(0x80000000)
			continue
		}
		cooldownTime := remainingMilliseconds(cooldown.End, now)
		categoryTime := remainingMilliseconds(cooldown.CategoryEnd, now)
		if cooldownTime == 0 {
			packet.WriteU32(0)
			packet.WriteU32(0)
		} else if cooldown.CategoryEnd >= now {
			packet.WriteU32(0)
			packet.WriteU32(categoryTime)
		} else {
			packet.WriteU32(cooldownTime)
			packet.WriteU32(0)
		}
	}
	return packet.Bytes()
}

func (s *session) buildUnlearnSpells(ctx context.Context, state playerState) []byte {
	active := make(map[uint32]struct{}, len(state.Spells))
	inactive := make([]uint32, 0)
	for _, spell := range state.Spells {
		if spell.Active && !spell.Disabled {
			active[spell.ID] = struct{}{}
		} else if !spell.Active && !spell.Disabled {
			inactive = append(inactive, spell.ID)
		}
	}
	nextRanks := make(map[uint32]uint32)
	if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		if rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT r1.spell_id, r2.spell_id
			FROM spell_ranks AS r1 JOIN spell_ranks AS r2
			ON r1.first_spell_id = r2.first_spell_id AND r2.rank = r1.rank + 1`); err == nil {
			for rows.Next() {
				var current, next uint32
				if rows.Scan(&current, &next) == nil {
					nextRanks[current] = next
				}
			}
			rows.Close()
		}
	}
	result := make([]uint32, 0, len(inactive))
	for _, spellID := range inactive {
		if s != nil && s.server != nil && s.server.Data != nil {
			abilities, found, err := s.server.Data.SkillLineAbilities(spellID)
			if err != nil || !found || len(abilities) == 0 {
				continue
			}
			superseded := false
			for _, ability := range abilities {
				if ability.SupercededBySpell != 0 {
					superseded = true
					break
				}
			}
			if superseded {
				continue
			}
		}
		next, ok := nextRanks[spellID]
		if !ok {
			continue
		}
		if _, ok := active[next]; !ok {
			continue
		}
		result = append(result, spellID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	packet := protocol.NewBuffer(4 + len(result)*4)
	packet.WriteU32(uint32(len(result)))
	for _, spellID := range result {
		packet.WriteU32(spellID)
	}
	return packet.Bytes()
}

func buildActionButtons(actions [144]uint32) []byte {
	packet := protocol.NewBuffer(1 + len(actions)*4)
	packet.WriteU8(1)
	for _, action := range actions {
		packet.WriteU32(action)
	}
	return packet.Bytes()
}

func remainingMilliseconds(end, now int64) uint32 {
	if end <= now {
		return 0
	}
	remaining := (end - now) * 1000
	if remaining > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(remaining)
}

func missingTable(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "no such table")
}
