package world

import (
	"context"
	"database/sql"
)

// Live instance boss-state store: InstanceScript::bosses plus
// InstanceScript::UpdateSpawnGroups (Instances/InstanceScript.cpp:238-273,
// 340-390) and the instance_spawn_groups load
// (Globals/ObjectMgr.cpp:2676-2740). Go previously kept only the
// completedEncounters mask on the instance row (instance_encounters.go);
// there was no per-boss state, so the .instance setbossstate/getbossstate
// arms blocked and instance spawn groups never moved. This file is the
// missing live model: a sparse per-(map, instance) boss map plus the
// spawn-group evaluation that SetBossState drives.
//
// EncounterState (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1,
// FAIL 2, DONE 3, SPECIAL 4, TO_BE_DECIDED 5. GetBossState returns
// TO_BE_DECIDED for an id outside the bosses vector, so an unset Go boss
// reads the same way; a live instance's bosses are never TO_BE_DECIDED
// after Create/Load, which Go has no hook for, so the first set on an
// unseen boss both initializes it and runs the spawn-group pass (the Go
// equivalent of Create's NOT_STARTED loop followed by the caller's set).
const (
	encounterStateNotStarted  uint8 = 0
	encounterStateInProgress  uint8 = 1
	encounterStateFail        uint8 = 2
	encounterStateDone        uint8 = 3
	encounterStateSpecial     uint8 = 4
	encounterStateToBeDecided uint8 = 5
)

// encounterStateName mirrors InstanceScript::GetBossStateName
// (InstanceScript.cpp:788-810), including INVALID past TO_BE_DECIDED.
func encounterStateName(state uint8) string {
	switch state {
	case encounterStateNotStarted:
		return "NOT_STARTED"
	case encounterStateInProgress:
		return "IN_PROGRESS"
	case encounterStateFail:
		return "FAIL"
	case encounterStateDone:
		return "DONE"
	case encounterStateSpecial:
		return "SPECIAL"
	case encounterStateToBeDecided:
		return "TO_BE_DECIDED"
	default:
		return "INVALID"
	}
}

// instanceBossStatesSnapshot copies the live boss states for one
// instance, in the shape the save path marshals into the blob's
// bossStates section (InstanceScript::WriteSaveDataBossStates writes
// the raw states, normalization happens on load, never on save).
func (s *Server) instanceBossStatesSnapshot(mapID, instanceID uint32) map[uint32]uint8 {
	if s == nil {
		return nil
	}
	s.instanceBossStateMu.RLock()
	defer s.instanceBossStateMu.RUnlock()
	bosses, ok := s.instanceBossStates[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]
	if !ok || len(bosses) == 0 {
		return nil
	}
	snapshot := make(map[uint32]uint8, len(bosses))
	for id, state := range bosses {
		snapshot[id] = state
	}
	return snapshot
}

// normalizeLoadedInstanceBossState is the load-side normalization in
// InstanceScript::ReadSaveDataBossStates (InstanceScript.cpp:437-450):
// a boss saved mid-fight, failed, or in a special state restarts
// NOT_STARTED; DONE and NOT_STARTED survive the load.
func normalizeLoadedInstanceBossState(state uint8) uint8 {
	switch state {
	case encounterStateInProgress, encounterStateFail, encounterStateSpecial:
		return encounterStateNotStarted
	default:
		return state
	}
}

// loadInstanceBossStates applies the boss states decoded from the
// instance-data blob, mirroring InstanceScript::ReadSaveDataBossStates
// (InstanceScript.cpp:437-450) plus its trailing UpdateSpawnGroups:
// normalize first, skip states at or past TO_BE_DECIDED (C++ only sets
// buff < TO_BE_DECIDED), take the loading branch for an unseen or
// TO_BE_DECIDED boss (set, no per-boss spawn-group pass), keep the
// live guards for a boss already set (same-state no-op, DONE never
// moves backwards), then run the spawn-group pass once for the whole
// load. The team is unknown on this path, so team-flagged rows stay
// blocked, matching a team-less setInstanceBossState caller.
func (s *Server) loadInstanceBossStates(ctx context.Context, mapID, instanceID uint32, states map[uint32]uint8) {
	if s == nil || len(states) == 0 {
		return
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	s.instanceBossStateMu.Lock()
	bosses, ok := s.instanceBossStates[key]
	if !ok {
		bosses = make(map[uint32]uint8)
		if s.instanceBossStates == nil {
			s.instanceBossStates = make(map[instanceAdmissionKey]map[uint32]uint8)
		}
		s.instanceBossStates[key] = bosses
	}
	for bossID, raw := range states {
		state := normalizeLoadedInstanceBossState(raw)
		if state >= encounterStateToBeDecided {
			continue
		}
		if current, seen := bosses[bossID]; seen {
			if current == encounterStateDone {
				continue
			}
			if current != encounterStateToBeDecided && current == state {
				continue
			}
		}
		bosses[bossID] = state
	}
	s.instanceBossStateMu.Unlock()
	s.updateInstanceSpawnGroups(ctx, mapID, instanceID, 0, false)
}

// instanceBossState mirrors InstanceScript::GetBossState: the stored state,
// or TO_BE_DECIDED for a boss never set in this instance.
func (s *Server) instanceBossState(mapID, instanceID, bossID uint32) uint8 {
	if s == nil {
		return encounterStateToBeDecided
	}
	s.instanceBossStateMu.RLock()
	defer s.instanceBossStateMu.RUnlock()
	if bosses, ok := s.instanceBossStates[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; ok {
		if state, ok := bosses[bossID]; ok {
			return state
		}
	}
	return encounterStateToBeDecided
}

// setInstanceBossState mirrors InstanceScript::SetBossState, minus the
// legs Go has no model for: the door/minion updates (no door/minion
// registries) and the DONE-transition world-boss-minion guard (no minion
// registry), and SaveToDB (no script save-data model; the savedata arm
// stays blocked). The guards that survive are C++-exact: a stored
// TO_BE_DECIDED boss takes the loading branch (set, no spawn-group pass,
// false), a same-state set is a no-op, and DONE never moves backwards.
// A first set on an unseen boss initializes it and runs the spawn-group
// pass, since Go has no Create/Load hook to pre-fill NOT_STARTED. The
// team/teamKnown pair feeds the spawn-group team flags: the command path
// passes the target player's effective team (C++ InstanceTeam comes from
// the instance save's creating team, and instances are single-team), while
// a team-less caller passes teamKnown false and team-flagged rows stay
// blocked rather than spawning for the wrong faction.
func (s *Server) setInstanceBossState(ctx context.Context, mapID, instanceID, bossID uint32, state uint8, team uint32, teamKnown bool) bool {
	if s == nil {
		return false
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	s.instanceBossStateMu.Lock()
	bosses, ok := s.instanceBossStates[key]
	if !ok {
		bosses = make(map[uint32]uint8)
		if s.instanceBossStates == nil {
			s.instanceBossStates = make(map[instanceAdmissionKey]map[uint32]uint8)
		}
		s.instanceBossStates[key] = bosses
	}
	current, seen := bosses[bossID]
	if seen {
		if current == encounterStateToBeDecided {
			bosses[bossID] = state
			s.instanceBossStateMu.Unlock()
			return false
		}
		if current == state {
			s.instanceBossStateMu.Unlock()
			return false
		}
		if current == encounterStateDone {
			s.instanceBossStateMu.Unlock()
			return false
		}
	}
	bosses[bossID] = state
	s.instanceBossStateMu.Unlock()
	s.updateInstanceSpawnGroups(ctx, mapID, instanceID, team, teamKnown)
	return true
}

// instanceSpawnGroupInfo mirrors InstanceSpawnGroupInfo
// (Globals/ObjectMgr.h:441-456): one instance_spawn_groups row.
type instanceSpawnGroupInfo struct {
	BossStateID  uint8
	BossStates   uint8
	SpawnGroupID uint32
	Flags        uint8
}

const (
	instanceSpawnGroupFlagActivateSpawn uint8 = 0x01
	instanceSpawnGroupFlagBlockSpawn    uint8 = 0x02
	instanceSpawnGroupFlagAllianceOnly  uint8 = 0x04
	instanceSpawnGroupFlagHordeOnly     uint8 = 0x08
	instanceSpawnGroupFlagsAll          uint8 = 0x0F
)

// ensureInstanceSpawnGroups mirrors ObjectMgr::LoadInstanceSpawnGroups
// (ObjectMgr.cpp:2676-2740) as a lazy load-once cache keyed by instance
// map id: rows naming an unknown or SYSTEM spawn group are skipped (the
// C++ load errors and continues), boss-state masks truncate to bits 0..4
// (ALL_STATES = (1 << TO_BE_DECIDED) - 1), flags truncate to FLAG_ALL,
// and a row carrying both team flags loses both. A missing table degrades
// to an empty cache, matching the spawn-group manager's convention.
func (s *Server) ensureInstanceSpawnGroups(ctx context.Context, db *sql.DB) error {
	if s == nil {
		return sql.ErrConnDone
	}
	s.instanceSpawnGroupMu.RLock()
	if s.instanceSpawnGroupLoaded {
		s.instanceSpawnGroupMu.RUnlock()
		return nil
	}
	s.instanceSpawnGroupMu.RUnlock()
	if db == nil {
		return sql.ErrConnDone
	}
	if err := s.ensureSpawnGroups(ctx, db); err != nil {
		return err
	}
	s.spawnGroupMu.RLock()
	valid := make(map[uint32]bool, len(s.spawnGroupTemplates))
	for id, tmpl := range s.spawnGroupTemplates {
		valid[id] = tmpl != nil && tmpl.Flags&spawnGroupFlagSystem == 0
	}
	s.spawnGroupMu.RUnlock()
	rows, err := db.QueryContext(ctx, "SELECT instanceMapId, bossStateId, bossStates, spawnGroupId, flags FROM instance_spawn_groups")
	if err != nil {
		if !missingTable(err) {
			return err
		}
		s.instanceSpawnGroupMu.Lock()
		s.instanceSpawnGroups = make(map[uint32][]instanceSpawnGroupInfo)
		s.instanceSpawnGroupLoaded = true
		s.instanceSpawnGroupMu.Unlock()
		return nil
	}
	defer rows.Close()
	byMap := make(map[uint32][]instanceSpawnGroupInfo)
	for rows.Next() {
		var mapID uint16
		var info instanceSpawnGroupInfo
		var bossStates, flags uint8
		if err := rows.Scan(&mapID, &info.BossStateID, &bossStates, &info.SpawnGroupID, &flags); err != nil {
			continue
		}
		if !valid[info.SpawnGroupID] {
			continue
		}
		info.BossStates = bossStates & ((1 << encounterStateToBeDecided) - 1)
		info.Flags = flags & instanceSpawnGroupFlagsAll
		if info.Flags&(instanceSpawnGroupFlagAllianceOnly|instanceSpawnGroupFlagHordeOnly) == instanceSpawnGroupFlagAllianceOnly|instanceSpawnGroupFlagHordeOnly {
			info.Flags &^= instanceSpawnGroupFlagAllianceOnly | instanceSpawnGroupFlagHordeOnly
		}
		byMap[uint32(mapID)] = append(byMap[uint32(mapID)], info)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.instanceSpawnGroupMu.Lock()
	s.instanceSpawnGroups = byMap
	s.instanceSpawnGroupLoaded = true
	s.instanceSpawnGroupMu.Unlock()
	return nil
}

// instance spawn-group evaluation states (InstanceScript.cpp:241): BLOCK
// is the map default, SPAWN activates, FORCEBLOCK wins over everything.
const (
	instanceSpawnGroupStateBlock = iota
	instanceSpawnGroupStateSpawn
	instanceSpawnGroupStateForceBlock
)

// evalInstanceSpawnGroupStates is the pure evaluation half of
// UpdateSpawnGroups (InstanceScript.cpp:242-262): rows apply in load
// order, a FORCEBLOCK group never moves again, a row matches only when
// the boss's current state bit is in its mask, team-flagged rows match
// only their team (and stay blocked when the team is unknown), BLOCK_SPAWN
// forces FORCEBLOCK and ACTIVATE_SPAWN sets SPAWN.
func evalInstanceSpawnGroupStates(infos []instanceSpawnGroupInfo, bossState func(uint32) uint8, team uint32, teamKnown bool) map[uint32]int {
	newStates := make(map[uint32]int)
	for _, info := range infos {
		cur := newStates[info.SpawnGroupID]
		if cur == instanceSpawnGroupStateForceBlock {
			continue
		}
		if (uint32(1)<<bossState(uint32(info.BossStateID)))&uint32(info.BossStates) == 0 {
			continue
		}
		if teamKnown {
			if team == 0 && info.Flags&instanceSpawnGroupFlagHordeOnly != 0 {
				continue
			}
			if team == 1 && info.Flags&instanceSpawnGroupFlagAllianceOnly != 0 {
				continue
			}
		} else if info.Flags&(instanceSpawnGroupFlagAllianceOnly|instanceSpawnGroupFlagHordeOnly) != 0 {
			continue
		}
		if info.Flags&instanceSpawnGroupFlagBlockSpawn != 0 {
			cur = instanceSpawnGroupStateForceBlock
		} else if info.Flags&instanceSpawnGroupFlagActivateSpawn != 0 {
			cur = instanceSpawnGroupStateSpawn
		}
		newStates[info.SpawnGroupID] = cur
	}
	return newStates
}

// updateInstanceSpawnGroups is the application half of UpdateSpawnGroups
// (InstanceScript.cpp:263-272): groups whose evaluated state differs from
// the manager's active state move — SPAWN via spawnGroupSpawn (active
// first, then members, mirroring Map::SpawnGroupSpawn), anything else via
// the inactive flip only (C++ SetSpawnGroupInactive deliberately does NOT
// despawn live members, it only stops respawns; the sweep filter hides
// them). Groups the manager cannot resolve are skipped, matching C++'s
// refusal of unknown/SYSTEM groups.
func (s *Server) updateInstanceSpawnGroups(ctx context.Context, mapID, instanceID uint32, team uint32, teamKnown bool) {
	if s == nil {
		return
	}
	var db *sql.DB
	if s.WorldStore != nil {
		db = s.WorldStore.DB
	}
	if err := s.ensureInstanceSpawnGroups(ctx, db); err != nil {
		return
	}
	s.instanceSpawnGroupMu.RLock()
	infos := s.instanceSpawnGroups[mapID]
	s.instanceSpawnGroupMu.RUnlock()
	if len(infos) == 0 {
		return
	}
	bossState := func(bossID uint32) uint8 {
		return s.instanceBossState(mapID, instanceID, bossID)
	}
	for groupID, state := range evalInstanceSpawnGroupStates(infos, bossState, team, teamKnown) {
		doSpawn := state == instanceSpawnGroupStateSpawn
		s.spawnGroupMu.RLock()
		tmpl := s.spawnGroupTemplates[groupID]
		s.spawnGroupMu.RUnlock()
		if tmpl == nil || tmpl.Flags&spawnGroupFlagSystem != 0 {
			continue
		}
		if s.spawnGroupIsActive(mapID, instanceID, tmpl) == doSpawn {
			continue
		}
		if doSpawn {
			s.spawnGroupSpawn(ctx, db, mapID, instanceID, groupID, false, false)
		} else {
			s.spawnGroupSetActive(mapID, instanceID, tmpl, false)
		}
	}
}
