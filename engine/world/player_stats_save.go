package world

import (
	"context"
	"database/sql"
)

func (s *session) saveCharacterStats(ctx context.Context, tx *sql.Tx, state *playerState, logout bool) error {
	if s == nil || state == nil || s.server == nil || s.server.CharactersStore == nil || tx == nil {
		return nil
	}
	config := s.server.Config
	if config.PlayerSaveStatsMinLevel == 0 || uint32(state.Level) < config.PlayerSaveStatsMinLevel || config.PlayerSaveStatsSaveOnlyOnLogout && !logout {
		return nil
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_CHAR_STATS", state.GUID); err != nil {
		return err
	}
	args := make([]any, 0, 31)
	args = append(args, state.GUID, state.MaxHealth)
	for _, power := range state.MaxPowers {
		args = append(args, power)
	}
	for _, stat := range state.Stats {
		args = append(args, stat)
	}
	args = append(args, state.Armor)
	for school := 1; school < len(state.Resistances); school++ {
		args = append(args, state.Resistances[school])
	}
	args = append(args, state.BlockPercentage, state.DodgePercentage, state.ParryPercentage, state.MeleeCrit, state.RangedCrit, state.SpellCrit[0], state.AttackPower, state.RangedAttackPower, state.BaseSpellPower, state.CombatRatings[16])
	_, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_CHAR_STATS", args...)
	return err
}
