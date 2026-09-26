package world

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

func (s *session) savePlayerCooldowns(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || tx == nil || state == nil || s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	latest := make(map[uint32]spellCooldown, len(state.Cooldowns))
	for _, cooldown := range state.Cooldowns {
		latest[cooldown.Spell] = cooldown
	}
	now := time.Now().Unix()
	cooldowns := make([]spellCooldown, 0, len(latest))
	for _, cooldown := range latest {
		if cooldown.End >= now {
			cooldowns = append(cooldowns, cooldown)
		}
	}
	sort.Slice(cooldowns, func(i, j int) bool { return cooldowns[i].Spell < cooldowns[j].Spell })
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_CHAR_SPELL_COOLDOWNS", state.GUID); err != nil {
		return err
	}
	for _, cooldown := range cooldowns {
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_CHAR_SPELL_COOLDOWN", state.GUID, cooldown.Spell, cooldown.Item, cooldown.End, cooldown.Category, cooldown.CategoryEnd); err != nil {
			return err
		}
	}
	return nil
}
