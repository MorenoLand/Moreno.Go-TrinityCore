package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

type worldportInstanceSelection struct {
	MapID                  uint32
	InstanceID             uint32
	Difficulty             uint32
	CreateSave             bool
	BindPlayer             bool
	BindGroup              bool
	GroupDBID              uint32
	GroupPermanent         bool
	UnbindPlayerInstanceID uint32
	Reserved               bool
}

type instanceAdmissionKey struct {
	MapID      uint32
	InstanceID uint32
}

func (s *session) resolveWorldportInstance(ctx context.Context, state playerState, entry wotlk.MapEntry) (worldportInstanceSelection, bool, error) {
	selection := worldportInstanceSelection{MapID: state.Map}
	if entry.IsBattleground() || entry.IsBattleArena() {
		if s.bgData.InstanceID == 0 {
			return selection, false, nil
		}
		selection.InstanceID = s.bgData.InstanceID
		return selection, true, nil
	}
	if !entry.IsDungeon() {
		return selection, true, nil
	}
	if s.server == nil || s.server.Data == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return selection, false, errors.New("instance map resolution requires loaded stores")
	}
	playerDifficulty := uint32(state.DungeonDifficulty)
	if entry.IsRaid() {
		playerDifficulty = uint32(state.RaidDifficulty)
	}
	_, playerDifficulty, found, err := s.server.Data.DownscaledMapDifficulty(state.Map, playerDifficulty)
	if err != nil {
		return selection, false, fmt.Errorf("map %d player difficulty lookup failed: %w", state.Map, err)
	}
	if !found {
		return selection, false, nil
	}
	var instanceTemplate uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT map FROM instance_template WHERE map = ?", state.Map).Scan(&instanceTemplate); errors.Is(err, sql.ErrNoRows) || err != nil && missingTable(err) {
		return selection, false, nil
	} else if err != nil {
		return selection, false, err
	}
	var characterInstanceID, characterDifficulty, characterPermanent int64
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id, i.difficulty, ci.permanent FROM character_instance AS ci
		JOIN instance AS i ON i.id = ci.instance WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? AND ci.extendState <> 0 LIMIT 1`, state.GUID, state.Map, playerDifficulty).Scan(&characterInstanceID, &characterDifficulty, &characterPermanent)
	characterBound := err == nil && characterInstanceID > 0 && characterInstanceID <= int64(^uint32(0))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return selection, false, err
	}
	if characterBound && characterPermanent != 0 {
		selection.InstanceID, selection.Difficulty = uint32(characterInstanceID), uint32(characterDifficulty)
		return selection, true, nil
	}
	if s.initialLoginPending && state.InstanceID != 0 {
		var savedMap, savedDifficulty int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT map, difficulty FROM instance WHERE id = ?", state.InstanceID).Scan(&savedMap, &savedDifficulty)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return selection, false, err
		}
		active := s.hasActiveWorldportInstance(state.Map, state.InstanceID)
		if savedMap == int64(state.Map) && ((characterBound && characterInstanceID == int64(state.InstanceID)) || active) {
			selection.InstanceID, selection.Difficulty = state.InstanceID, uint32(savedDifficulty)
			selection.GroupPermanent, err = s.groupInstancePermanent(ctx, selection)
			if err != nil {
				return selection, false, err
			}
			return selection, true, nil
		}
		if errors.Is(err, sql.ErrNoRows) && active {
			selection.InstanceID, selection.Difficulty = state.InstanceID, playerDifficulty
			return selection, true, nil
		}
		return selection, false, nil
	}
	group := s.server.findGroupByID(s.groupID)
	canBindGroup := group != nil && group.DBID != 0 && group.GroupType&groupTypeBattleground == 0
	groupDBID := uint32(0)
	groupDifficulty := playerDifficulty
	if group != nil {
		groupDBID = group.DBID
		groupDifficulty = uint32(group.DungeonDiff)
		if entry.IsRaid() {
			groupDifficulty = uint32(group.RaidDiff)
		}
		if _, downscaled, groupModeFound, modeErr := s.server.Data.DownscaledMapDifficulty(state.Map, groupDifficulty); modeErr == nil && groupModeFound {
			groupDifficulty = downscaled
		}
	}
	var groupInstanceID, groupBoundDifficulty, groupPermanent int64
	groupBound := false
	if canBindGroup {
		err = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id, i.difficulty, gi.permanent FROM group_instance AS gi
			JOIN instance AS i ON i.id = gi.instance WHERE gi.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, groupDBID, state.Map, groupDifficulty).Scan(&groupInstanceID, &groupBoundDifficulty, &groupPermanent)
		groupBound = err == nil && groupInstanceID > 0 && groupInstanceID <= int64(^uint32(0))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return selection, false, err
		}
	}
	if groupBound {
		selection.InstanceID, selection.Difficulty, selection.GroupPermanent = uint32(groupInstanceID), uint32(groupBoundDifficulty), groupPermanent != 0
		if characterBound {
			selection.UnbindPlayerInstanceID = uint32(characterInstanceID)
		}
		return selection, true, nil
	}
	if characterBound {
		selection.InstanceID, selection.Difficulty = uint32(characterInstanceID), uint32(characterDifficulty)
		if canBindGroup {
			selection.BindGroup, selection.GroupDBID = true, groupDBID
		}
		return selection, true, nil
	}
	newDifficulty := playerDifficulty
	if group != nil {
		newDifficulty = groupDifficulty
	}
	instanceID, err := s.server.reserveWorldportInstanceID(ctx, s.bgData.InstanceID)
	if err != nil {
		return selection, false, err
	}
	selection.InstanceID, selection.Difficulty, selection.CreateSave, selection.Reserved = instanceID, newDifficulty, true, true
	if canBindGroup {
		selection.BindGroup, selection.GroupDBID = true, groupDBID
	} else {
		selection.BindPlayer = true
	}
	return selection, true, nil
}

func (s *session) groupInstancePermanent(ctx context.Context, selection worldportInstanceSelection) (bool, error) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || selection.InstanceID == 0 {
		return false, nil
	}
	group := s.server.findGroupByID(s.groupID)
	if group == nil || group.DBID == 0 {
		return false, nil
	}
	var permanent int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT gi.permanent FROM group_instance AS gi JOIN instance AS i ON i.id = gi.instance
		WHERE gi.guid = ? AND i.id = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, group.DBID, selection.InstanceID, selection.MapID, selection.Difficulty).Scan(&permanent)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return permanent != 0, nil
}

func (s *Server) reserveWorldportInstanceID(ctx context.Context, avoid uint32) (uint32, error) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0, errors.New("instance ID allocation requires a character database")
	}
	used := make(map[uint32]struct{})
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT id FROM instance ORDER BY id")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && id > 0 && id <= int64(^uint32(0)) {
			used[uint32(id)] = struct{}{}
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return 0, rowsErr
	}
	if avoid != 0 {
		used[avoid] = struct{}{}
	}
	if bgRows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT instanceId FROM character_battleground_data WHERE instanceId > 0"); err == nil {
		for bgRows.Next() {
			var id int64
			if bgRows.Scan(&id) == nil && id > 0 && id <= int64(^uint32(0)) {
				used[uint32(id)] = struct{}{}
			}
		}
		bgErr := bgRows.Err()
		bgRows.Close()
		if bgErr != nil {
			return 0, bgErr
		}
	} else if !missingTable(err) {
		return 0, err
	}
	s.instanceIDMu.Lock()
	defer s.instanceIDMu.Unlock()
	if s.reservedInstanceIDs == nil {
		s.reservedInstanceIDs = make(map[uint32]struct{})
	}
	for candidate := uint64(1); candidate < uint64(^uint32(0)); candidate++ {
		id := uint32(candidate)
		if _, exists := used[id]; exists {
			continue
		}
		if _, exists := s.reservedInstanceIDs[id]; exists {
			continue
		}
		s.reservedInstanceIDs[id] = struct{}{}
		return id, nil
	}
	return 0, errors.New("instance ID space exhausted")
}

func (s *Server) releaseWorldportInstanceID(instanceID uint32) {
	if s == nil || instanceID == 0 {
		return
	}
	s.instanceIDMu.Lock()
	delete(s.reservedInstanceIDs, instanceID)
	s.instanceIDMu.Unlock()
}

func (s *session) hasActiveWorldportInstance(mapID, instanceID uint32) bool {
	if s == nil || s.server == nil || instanceID == 0 {
		return false
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	target := uint64(mapID)<<32 | uint64(instanceID)
	for peer := range s.server.sessions {
		if peer == nil || peer == s || !peer.worldReady.Load() {
			continue
		}
		if peer.worldInstance.Load() == target {
			return true
		}
	}
	return false
}

func (s *session) worldportInstanceHasRoom(selection worldportInstanceSelection, entry wotlk.MapEntry) bool {
	if !entry.IsDungeon() || s.isMapAdmissionGM() {
		return true
	}
	difficulty, found, err := s.server.Data.MapDifficulty(entry.ID, selection.Difficulty)
	if err != nil {
		return false
	}
	maxPlayers := entry.MaxPlayers
	if found && difficulty.MaxPlayers != 0 {
		maxPlayers = difficulty.MaxPlayers
	}
	key := instanceAdmissionKey{MapID: entry.ID, InstanceID: selection.InstanceID}
	target := uint64(key.MapID)<<32 | uint64(key.InstanceID)
	s.server.instanceAdmissionMu.Lock()
	defer s.server.instanceAdmissionMu.Unlock()
	players := uint32(0)
	players += s.server.pendingInstanceAdmissions[key]
	countNpcBots := s.server.Config.NPCBots.LimitDungeon && entry.IsDungeon() && !entry.IsRaid() || s.server.Config.NPCBots.LimitRaid && entry.IsRaid()
	var occupants []*session
	s.server.sessionsMu.RLock()
	for peer := range s.server.sessions {
		if peer == s || peer == nil || !peer.worldReady.Load() || peer.worldReadyGM.Load() || peer.worldInstance.Load() != target {
			continue
		}
		occupants = append(occupants, peer)
	}
	s.server.sessionsMu.RUnlock()
	for _, peer := range occupants {
		players += s.server.instanceOccupantCount(peer, countNpcBots)
	}
	if players >= maxPlayers {
		return false
	}
	if s.server.pendingInstanceAdmissions == nil {
		s.server.pendingInstanceAdmissions = make(map[instanceAdmissionKey]uint32)
	}
	s.server.pendingInstanceAdmissions[key]++
	return true
}

func (s *session) worldportInstanceAtCapacity(selection worldportInstanceSelection, entry wotlk.MapEntry) bool {
	if !entry.IsDungeon() || s.isMapAdmissionGM() {
		return false
	}
	difficulty, found, err := s.server.Data.MapDifficulty(entry.ID, selection.Difficulty)
	if err != nil {
		return true
	}
	maxPlayers := entry.MaxPlayers
	if found && difficulty.MaxPlayers != 0 {
		maxPlayers = difficulty.MaxPlayers
	}
	key := instanceAdmissionKey{MapID: entry.ID, InstanceID: selection.InstanceID}
	target := uint64(key.MapID)<<32 | uint64(key.InstanceID)
	s.server.instanceAdmissionMu.Lock()
	defer s.server.instanceAdmissionMu.Unlock()
	players := s.server.pendingInstanceAdmissions[key]
	countNpcBots := s.server.Config.NPCBots.LimitDungeon && entry.IsDungeon() && !entry.IsRaid() || s.server.Config.NPCBots.LimitRaid && entry.IsRaid()
	var occupants []*session
	s.server.sessionsMu.RLock()
	for peer := range s.server.sessions {
		if peer == s || peer == nil || !peer.worldReady.Load() || peer.worldReadyGM.Load() || peer.worldInstance.Load() != target {
			continue
		}
		occupants = append(occupants, peer)
	}
	s.server.sessionsMu.RUnlock()
	for _, peer := range occupants {
		players += s.server.instanceOccupantCount(peer, countNpcBots)
	}
	return players >= maxPlayers
}

func (s *Server) releaseWorldportAdmission(key instanceAdmissionKey) {
	if s == nil {
		return
	}
	s.instanceAdmissionMu.Lock()
	if s.pendingInstanceAdmissions[key] <= 1 {
		delete(s.pendingInstanceAdmissions, key)
	} else {
		s.pendingInstanceAdmissions[key]--
	}
	s.instanceAdmissionMu.Unlock()
}

func (s *session) commitWorldportAdmission(selection worldportInstanceSelection, reserved bool) {
	if s == nil || s.server == nil {
		return
	}
	s.server.instanceAdmissionMu.Lock()
	if reserved {
		key := instanceAdmissionKey{MapID: selection.MapID, InstanceID: selection.InstanceID}
		if s.server.pendingInstanceAdmissions[key] <= 1 {
			delete(s.server.pendingInstanceAdmissions, key)
		} else {
			s.server.pendingInstanceAdmissions[key]--
		}
	}
	s.worldInstance.Store(uint64(selection.MapID)<<32 | uint64(selection.InstanceID))
	s.worldReadyGM.Store(s.isMapAdmissionGM())
	s.worldReady.Store(true)
	s.server.instanceAdmissionMu.Unlock()
}

func (s *session) updateWorldReadyGM() {
	if s != nil && s.worldReady.Load() {
		s.worldReadyGM.Store(s.isMapAdmissionGM())
	}
}

func (s *session) sendWorldportGroupLockWarning(ctx context.Context, selection worldportInstanceSelection) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || !selection.GroupPermanent {
		return
	}
	var encounterMask int64
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT completedEncounters FROM instance WHERE id = ?", selection.InstanceID).Scan(&encounterMask)
	s.sendInstanceLockWarningQuery(60000, uint32(encounterMask), 0)
	s.setPendingBind(uint64(selection.InstanceID), selection.MapID, selection.Difficulty, 60000)
}

func (s *Server) beginInstanceEncounter(motion *creatureMotion, state *playerState) {
	if s == nil || motion == nil || motion.BossAI == nil || state == nil || state.InstanceID == 0 || s.Data == nil {
		return
	}
	entry, found, err := s.Data.Map(motion.Map)
	if err != nil || !found || !entry.IsRaid() {
		return
	}
	motion.EncounterMapID, motion.EncounterInstanceID = motion.Map, state.InstanceID
	s.setInstanceEncounter(motion.EncounterMapID, motion.EncounterInstanceID, motion.GUID, true)
}

func (s *Server) clearInstanceEncounter(motion *creatureMotion) {
	if s == nil || motion == nil || motion.EncounterInstanceID == 0 {
		return
	}
	s.setInstanceEncounter(motion.EncounterMapID, motion.EncounterInstanceID, motion.GUID, false)
	motion.EncounterMapID, motion.EncounterInstanceID = 0, 0
}

func (s *Server) setInstanceEncounter(mapID, instanceID uint32, creatureGUID uint64, active bool) {
	if s == nil || instanceID == 0 || creatureGUID == 0 {
		return
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	s.instanceAdmissionMu.Lock()
	if active {
		if s.instanceEncounters == nil {
			s.instanceEncounters = make(map[instanceAdmissionKey]map[uint64]struct{})
		}
		if s.instanceEncounters[key] == nil {
			s.instanceEncounters[key] = make(map[uint64]struct{})
		}
		s.instanceEncounters[key][creatureGUID] = struct{}{}
	} else if encounters := s.instanceEncounters[key]; encounters != nil {
		delete(encounters, creatureGUID)
		if len(encounters) == 0 {
			delete(s.instanceEncounters, key)
		}
	}
	s.instanceAdmissionMu.Unlock()
}

func (s *Server) instanceEncounterInProgress(mapID, instanceID uint32) bool {
	if s == nil || instanceID == 0 {
		return false
	}
	s.instanceAdmissionMu.Lock()
	inProgress := len(s.instanceEncounters[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]) != 0
	s.instanceAdmissionMu.Unlock()
	return inProgress
}

func (s *session) persistWorldportInstance(ctx context.Context, selection worldportInstanceSelection) error {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || selection.InstanceID == 0 {
		return errors.New("worldport instance persistence requires an instance and character database")
	}
	tx, err := s.server.CharactersStore.Begin(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if selection.CreateSave {
		resetTime := uint64(0)
		if entry, found, mapErr := s.server.Data.Map(selection.MapID); mapErr != nil || !found {
			return fmt.Errorf("worldport instance map %d disappeared before save: %v", selection.MapID, mapErr)
		} else if !entry.IsRaid() && selection.Difficulty == 0 {
			resetTime = uint64(time.Now().Unix() + 2*60*60)
		}
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_INSTANCE_SAVE", selection.InstanceID, selection.MapID, resetTime, uint8(selection.Difficulty), uint32(0), ""); err != nil {
			return err
		}
	}
	if selection.UnbindPlayerInstanceID != 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM character_instance WHERE guid = ? AND instance = ? AND permanent = 0", s.playerGUID, selection.UnbindPlayerInstanceID); err != nil {
			return err
		}
	}
	if selection.BindGroup {
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_REP_GROUP_INSTANCE", selection.GroupDBID, selection.InstanceID, false); err != nil {
			return err
		}
	}
	if selection.BindPlayer {
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_CHAR_INSTANCE", s.playerGUID, selection.InstanceID, false, uint8(1)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
