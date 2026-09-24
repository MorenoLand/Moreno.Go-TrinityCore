package world

import (
	"context"
	"database/sql"
	"sort"
)

func (s *session) saveBattlegroundData(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || tx == nil || state == nil {
		return nil
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_PLAYER_BGDATA", state.GUID); err != nil {
		return err
	}
	_, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_PLAYER_BGDATA", state.GUID, s.bgData.InstanceID, s.bgData.Team, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO, s.bgData.JoinMap, s.bgData.TaxiStart, s.bgData.TaxiEnd, s.bgData.MountSpell)
	return err
}

func (s *session) saveInstanceTimeRestrictions(ctx context.Context, tx *sql.Tx) error {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || tx == nil || len(s.instanceLockTimes) == 0 {
		return nil
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_ACCOUNT_INSTANCE_LOCK_TIMES", s.accountID); err != nil {
		return err
	}
	instances := make([]uint32, 0, len(s.instanceLockTimes))
	for instanceID := range s.instanceLockTimes {
		instances = append(instances, instanceID)
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i] < instances[j] })
	for _, instanceID := range instances {
		if instanceID == 0 || s.instanceLockTimes[instanceID] <= 0 {
			continue
		}
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_ACCOUNT_INSTANCE_LOCK_TIMES", s.accountID, instanceID, s.instanceLockTimes[instanceID]); err != nil {
			return err
		}
	}
	return nil
}
