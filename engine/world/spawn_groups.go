// Spawn-group manager: ObjectMgr::LoadSpawnGroupTemplates/LoadSpawnGroups
// (Globals/ObjectMgr.cpp:2553/2612) plus Map::SpawnGroupSpawn,
// Map::SpawnGroupDespawn, Map::SetSpawnGroupActive and
// Map::IsSpawnGroupActive (Maps/Map.cpp:3378/3450/3473/3487), backing the
// .npc and .gobject spawngroup/despawngroup arms (cs_npc.cpp:1356/1397,
// shared by both command tables).
//
// C++ loads templates and members once at startup; Go loads them lazily on
// first use (a command or a sweep filter) and caches them for the process
// lifetime, so later world-DB edits to spawn_group(_template) need a restart
// to take effect. Templates carry the C++ flag sanitization (unknown bits
// dropped, SYSTEM|MANUAL_SPAWN loses MANUAL_SPAWN) and the synthetic groups
// 0 ("Default Group", SYSTEM) and 1 ("Legacy Group",
// SYSTEM|COMPATIBILITY_MODE). A member whose row is missing, whose group is
// unknown, or whose map differs from the group's first member's map is
// dropped, matching the C++ load errors; spawns with no spawn_group row
// belong to group 0 and never appear in the member map.
//
// Go creature/gameobject visibility is DB-driven (buildNearbyCreatureUpdates
// / buildNearbyGameObjectUpdates read the spawn rows live), so the manager's
// lasting effect is the active-state gate in those sweeps: members of an
// inactive group are filtered out, members of an active group flow through.
// The spawn/despawn legs below therefore flip the active state, clear or
// keep respawn records per the C++ flag rules, remove live creature motions
// with the despawnAtEvade destroy broadcast, and count exactly the members
// that change visibility, in C++ order (spawn sets active first, despawn
// sets inactive last). DELTAS: no grid-loaded gate (C++ skips unloaded
// grids; Go has no grids, so counts are map-wide); no gameobject respawn
// model (GO members have no respawn timers to clear or skip on); no
// ApplyNewEvent/instance-spawn-group legs (instance_spawn_groups is a
// separate boss-state system, untouched here); spawn re-appearance rides the
// next sweep update rather than an explicit create broadcast.
package world

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spawnGroupFlagSystem      = 0x01
	spawnGroupFlagCompat      = 0x02
	spawnGroupFlagManualSpawn = 0x04
	spawnGroupFlagsAll        = 0x1F

	spawnTypeCreature   = 0
	spawnTypeGameObject = 1
)

// spawnGroupTemplate mirrors SpawnGroupTemplateData (Maps/SpawnData.h): the
// mapId is derived from the first member's row at load time, 0 when the
// group has no members.
type spawnGroupTemplate struct {
	ID    uint32
	Name  string
	Flags uint32
	MapID uint32
}

// spawnGroupMember is one spawn_group row joined to its spawn row: Entry for
// the world GUID, position/phase for destroy broadcasts.
type spawnGroupMember struct {
	Type      uint8
	SpawnID   uint32
	Entry     uint32
	X, Y, Z   float32
	PhaseMask uint32
}

type spawnGroupMemberKey struct {
	Type    uint8
	SpawnID uint32
}

type spawnGroupInstanceKey struct {
	MapID      uint32
	InstanceID uint32
	GroupID    uint32
}

// ensureSpawnGroups loads the template/member cache once. A missing table
// degrades to an empty cache (C++ logs and continues with zero groups); any
// other error leaves the cache unloaded so the next use retries.
func (s *Server) ensureSpawnGroups(ctx context.Context, db *sql.DB) error {
	if s == nil || db == nil {
		return sql.ErrConnDone
	}
	s.spawnGroupMu.RLock()
	if s.spawnGroupLoaded {
		s.spawnGroupMu.RUnlock()
		return nil
	}
	s.spawnGroupMu.RUnlock()
	s.spawnGroupMu.Lock()
	defer s.spawnGroupMu.Unlock()
	if s.spawnGroupLoaded {
		return nil
	}
	templates := make(map[uint32]*spawnGroupTemplate)
	rows, err := db.QueryContext(ctx, "SELECT groupId, groupName, groupFlags FROM spawn_group_template")
	if err != nil {
		if !missingTable(err) {
			return err
		}
	} else {
		for rows.Next() {
			var id uint32
			var name string
			var flags uint32
			if err := rows.Scan(&id, &name, &flags); err != nil {
				continue
			}
			flags &= spawnGroupFlagsAll
			if flags&spawnGroupFlagSystem != 0 {
				flags &^= spawnGroupFlagManualSpawn
			}
			templates[id] = &spawnGroupTemplate{ID: id, Name: name, Flags: flags}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	if _, ok := templates[0]; !ok {
		templates[0] = &spawnGroupTemplate{ID: 0, Name: "Default Group", Flags: spawnGroupFlagSystem}
	}
	if _, ok := templates[1]; !ok {
		templates[1] = &spawnGroupTemplate{ID: 1, Name: "Legacy Group", Flags: spawnGroupFlagSystem | spawnGroupFlagCompat}
	}
	groupMembers := make(map[uint32][]spawnGroupMember)
	memberGroup := make(map[spawnGroupMemberKey]uint32)
	groupMapKnown := make(map[uint32]bool)
	mrows, err := db.QueryContext(ctx, "SELECT groupId, spawnType, spawnId FROM spawn_group ORDER BY groupId, spawnType, spawnId")
	if err != nil {
		if !missingTable(err) {
			return err
		}
	} else {
		for mrows.Next() {
			var groupID uint32
			var spawnType uint8
			var spawnID uint32
			if err := mrows.Scan(&groupID, &spawnType, &spawnID); err != nil {
				continue
			}
			if spawnType != spawnTypeCreature && spawnType != spawnTypeGameObject {
				continue
			}
			tmpl, ok := templates[groupID]
			if !ok {
				continue
			}
			var member spawnGroupMember
			var rowMap uint32
			var scanErr error
			if spawnType == spawnTypeCreature {
				scanErr = db.QueryRowContext(ctx, "SELECT map, id, position_x, position_y, position_z, phaseMask FROM creature WHERE guid = ?", spawnID).
					Scan(&rowMap, &member.Entry, &member.X, &member.Y, &member.Z, &member.PhaseMask)
			} else {
				scanErr = db.QueryRowContext(ctx, "SELECT map, id, position_x, position_y, position_z, phaseMask FROM gameobject WHERE guid = ?", spawnID).
					Scan(&rowMap, &member.Entry, &member.X, &member.Y, &member.Z, &member.PhaseMask)
			}
			if scanErr != nil {
				continue
			}
			if !groupMapKnown[groupID] {
				tmpl.MapID = rowMap
				groupMapKnown[groupID] = true
			} else if tmpl.MapID != rowMap {
				continue
			}
			member.Type = spawnType
			member.SpawnID = spawnID
			groupMembers[groupID] = append(groupMembers[groupID], member)
			memberGroup[spawnGroupMemberKey{Type: spawnType, SpawnID: spawnID}] = groupID
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			return err
		}
	}
	s.spawnGroupTemplates = templates
	s.spawnGroupMembers = groupMembers
	s.spawnGroupMemberGroup = memberGroup
	s.spawnGroupLoaded = true
	return nil
}

// spawnGroupIsActiveLocked mirrors Map::IsSpawnGroupActive: SYSTEM groups
// are always active; otherwise the toggled set flips the MANUAL_SPAWN
// default (manual groups start inactive, the rest start active).
func (s *Server) spawnGroupIsActiveLocked(mapID, instanceID uint32, tmpl *spawnGroupTemplate) bool {
	if tmpl == nil || tmpl.Flags&spawnGroupFlagSystem != 0 {
		return true
	}
	toggled := s.spawnGroupToggled[spawnGroupInstanceKey{MapID: mapID, InstanceID: instanceID, GroupID: tmpl.ID}]
	return toggled != (tmpl.Flags&spawnGroupFlagManualSpawn == 0)
}

func (s *Server) spawnGroupIsActive(mapID, instanceID uint32, tmpl *spawnGroupTemplate) bool {
	s.spawnGroupMu.RLock()
	defer s.spawnGroupMu.RUnlock()
	return s.spawnGroupIsActiveLocked(mapID, instanceID, tmpl)
}

// spawnGroupSetActive mirrors Map::SetSpawnGroupActive: the toggled set
// holds exactly the groups whose state differs from their flag default.
func (s *Server) spawnGroupSetActive(mapID, instanceID uint32, tmpl *spawnGroupTemplate, state bool) {
	if s == nil || tmpl == nil || tmpl.Flags&spawnGroupFlagSystem != 0 {
		return
	}
	s.spawnGroupMu.Lock()
	defer s.spawnGroupMu.Unlock()
	key := spawnGroupInstanceKey{MapID: mapID, InstanceID: instanceID, GroupID: tmpl.ID}
	if state != (tmpl.Flags&spawnGroupFlagManualSpawn == 0) {
		if s.spawnGroupToggled == nil {
			s.spawnGroupToggled = make(map[spawnGroupInstanceKey]bool)
		}
		s.spawnGroupToggled[key] = true
	} else {
		delete(s.spawnGroupToggled, key)
	}
}

// spawnGroupForCommand resolves a command's group: template exists, not a
// SYSTEM group (C++ refuses those with the bad-group message), the template
// map matches the GM's map (GetSpawnGroupData's map gate), and the group has
// at least one member on that map (a memberless template keeps mapId unset
// and fails the gate the same way).
func (s *Server) spawnGroupForCommand(ctx context.Context, db *sql.DB, groupID, mapID uint32) (*spawnGroupTemplate, []spawnGroupMember, bool) {
	if err := s.ensureSpawnGroups(ctx, db); err != nil {
		return nil, nil, false
	}
	s.spawnGroupMu.RLock()
	defer s.spawnGroupMu.RUnlock()
	tmpl := s.spawnGroupTemplates[groupID]
	if tmpl == nil || tmpl.Flags&spawnGroupFlagSystem != 0 || tmpl.MapID != mapID {
		return nil, nil, false
	}
	members := s.spawnGroupMembers[groupID]
	if len(members) == 0 {
		return nil, nil, false
	}
	return tmpl, members, true
}

// spawnGroupHiddenFromSweep is the sweep filter: a spawn row belongs to an
// inactive group on this (map, instance) pair. Unlisted spawns are group 0
// (always active); SYSTEM groups never hide; a load failure fails open.
func (s *Server) spawnGroupHiddenFromSweep(ctx context.Context, mapID, instanceID uint32, spawnType uint8, spawnID uint32) bool {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return false
	}
	if err := s.ensureSpawnGroups(ctx, s.WorldStore.DB); err != nil {
		return false
	}
	s.spawnGroupMu.RLock()
	defer s.spawnGroupMu.RUnlock()
	groupID, ok := s.spawnGroupMemberGroup[spawnGroupMemberKey{Type: spawnType, SpawnID: spawnID}]
	if !ok {
		return false
	}
	tmpl := s.spawnGroupTemplates[groupID]
	if tmpl == nil || tmpl.Flags&spawnGroupFlagSystem != 0 || tmpl.MapID != mapID {
		return false
	}
	return !s.spawnGroupIsActiveLocked(mapID, instanceID, tmpl)
}

// spawnGroupOfMember resolves the info-line group for one spawn: group 0
// (unlisted) returns groupID 0 with a nil template, which the caller
// suppresses exactly like C++'s groupId check.
func (s *Server) spawnGroupOfMember(ctx context.Context, db *sql.DB, spawnType uint8, spawnID, mapID, instanceID uint32) (uint32, *spawnGroupTemplate, bool) {
	if err := s.ensureSpawnGroups(ctx, db); err != nil {
		return 0, nil, false
	}
	s.spawnGroupMu.RLock()
	defer s.spawnGroupMu.RUnlock()
	groupID, ok := s.spawnGroupMemberGroup[spawnGroupMemberKey{Type: spawnType, SpawnID: spawnID}]
	if !ok {
		return 0, nil, false
	}
	tmpl := s.spawnGroupTemplates[groupID]
	if tmpl == nil {
		return 0, nil, false
	}
	return groupID, tmpl, s.spawnGroupIsActiveLocked(mapID, instanceID, tmpl)
}

// spawnGroupCreatureRespawnPending mirrors the Map respawn-timer check:
// either in-memory respawn record for this spawn (global or this
// (map, instance) pair; DB guids are globally unique).
func (s *Server) spawnGroupCreatureRespawnPending(mapID, instanceID, guid uint32) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if _, ok := s.creatureRespawns[guid]; ok {
		return true
	}
	if respawns, ok := s.instanceCreatureRespawns[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; ok {
		_, ok = respawns[guid]
		return ok
	}
	return false
}

// spawnGroupClearCreatureRespawn mirrors Map::RemoveRespawnTime for one
// creature spawn: in-memory records plus the saved respawn row.
func (s *Server) spawnGroupClearCreatureRespawn(ctx context.Context, mapID, instanceID, guid uint32) {
	if s == nil {
		return
	}
	s.motionMu.Lock()
	delete(s.creatureRespawns, guid)
	if respawns, ok := s.instanceCreatureRespawns[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; ok {
		delete(respawns, guid)
	}
	s.motionMu.Unlock()
	s.deleteCreatureRespawnRow(ctx, guid, mapID, instanceID)
}

// spawnGroupRemoveCreatureMotion removes a live creature motion with the
// despawnAtEvade removal shape (delete + SMSG_DESTROY_OBJECT + OnRemove)
// minus the evade re-arm: a group despawn is not a delayed respawn.
func (s *Server) spawnGroupRemoveCreatureMotion(ctx context.Context, mapID, instanceID uint32, worldGUID uint64) *creatureMotion {
	if s == nil {
		return nil
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(mapID, instanceID, worldGUID)
	if motion != nil {
		delete(s.motionMapLocked(mapID, instanceID), worldGUID)
	}
	s.motionMu.Unlock()
	if motion == nil {
		return nil
	}
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(motion.GUID)
	destroyBuf.WriteU8(0)
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask)
	s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnRemove)
	return motion
}

// spawnGroupDestroyAt broadcasts the destroy packet for a sweep-visible
// spawn with no live motion (row position, 0 phase reads as PHASEMASK_NORMAL).
func (s *Server) spawnGroupDestroyAt(mapID, instanceID uint32, worldGUID uint64, x, y, z float32, phaseMask uint32) {
	if s == nil {
		return
	}
	if phaseMask == 0 {
		phaseMask = 1
	}
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(worldGUID)
	destroyBuf.WriteU8(0)
	s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), mapID, instanceID, x, y, z, phaseMask)
}

// spawnGroupSpawn mirrors Map::SpawnGroupSpawn: active first, then per
// member (force/ignorerespawn clear the respawn timer first, a remaining
// timer skips, a live-and-alive creature skips unless forced, an
// already-visible member skips, everything else counts as spawned).
func (s *Server) spawnGroupSpawn(ctx context.Context, db *sql.DB, mapID, instanceID, groupID uint32, ignoreRespawn, force bool) (int, bool) {
	tmpl, members, ok := s.spawnGroupForCommand(ctx, db, groupID, mapID)
	if !ok {
		return 0, false
	}
	wasActive := s.spawnGroupIsActive(mapID, instanceID, tmpl)
	s.spawnGroupSetActive(mapID, instanceID, tmpl, true)
	count := 0
	for _, m := range members {
		hadTimer := false
		if m.Type == spawnTypeCreature {
			hadTimer = s.spawnGroupCreatureRespawnPending(mapID, instanceID, m.SpawnID)
			if force || ignoreRespawn {
				s.spawnGroupClearCreatureRespawn(ctx, mapID, instanceID, m.SpawnID)
			}
			if s.spawnGroupCreatureRespawnPending(mapID, instanceID, m.SpawnID) {
				continue
			}
			worldGUID := creatureWorldGUID(m.SpawnID, m.Entry)
			s.motionMu.Lock()
			motion := s.findCreatureMotionLocked(mapID, instanceID, worldGUID)
			alive := motion != nil && motion.Health > 0
			s.motionMu.Unlock()
			if motion != nil {
				if alive && !force {
					continue
				}
				s.spawnGroupRemoveCreatureMotion(ctx, mapID, instanceID, worldGUID)
				count++
				continue
			}
			if wasActive && !hadTimer {
				continue
			}
			count++
			continue
		}
		if wasActive {
			if !force {
				continue
			}
			s.broadcastGameObjectDespawn(mapID, gameObjectGUID(m.SpawnID, m.Entry))
		}
		count++
	}
	return count, true
}

// spawnGroupDespawn mirrors Map::SpawnGroupDespawn: per member
// (removerespawntime clears the timer first, live objects are removed and
// counted, sweep-visible members are destroy-broadcast and counted), then
// inactive last.
func (s *Server) spawnGroupDespawn(ctx context.Context, db *sql.DB, mapID, instanceID, groupID uint32, deleteRespawnTimes bool) (int, bool) {
	tmpl, members, ok := s.spawnGroupForCommand(ctx, db, groupID, mapID)
	if !ok {
		return 0, false
	}
	wasActive := s.spawnGroupIsActive(mapID, instanceID, tmpl)
	count := 0
	for _, m := range members {
		pendingBefore := false
		if m.Type == spawnTypeCreature {
			pendingBefore = s.spawnGroupCreatureRespawnPending(mapID, instanceID, m.SpawnID)
			if deleteRespawnTimes {
				s.spawnGroupClearCreatureRespawn(ctx, mapID, instanceID, m.SpawnID)
			}
			if s.spawnGroupRemoveCreatureMotion(ctx, mapID, instanceID, creatureWorldGUID(m.SpawnID, m.Entry)) != nil {
				count++
				continue
			}
			if wasActive && !pendingBefore {
				count++
				s.spawnGroupDestroyAt(mapID, instanceID, creatureWorldGUID(m.SpawnID, m.Entry), m.X, m.Y, m.Z, m.PhaseMask)
			}
			continue
		}
		if wasActive {
			count++
			s.broadcastGameObjectDespawn(mapID, gameObjectGUID(m.SpawnID, m.Entry))
		}
	}
	s.spawnGroupSetActive(mapID, instanceID, tmpl, false)
	return count, true
}

// parseSpawnGroupArgs mirrors the cs_npc.cpp variant decode: the first uint
// is the group id (a later uint overwrites it, as the C++ variant loop
// does); the remaining words are exact flags, anything else fails parsing.
func parseSpawnGroupArgs(args []string, spawnFlags bool) (groupID uint32, flagA, flagB, ok bool) {
	if len(args) == 0 {
		return 0, false, false, false
	}
	v, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return 0, false, false, false
	}
	groupID = uint32(v)
	for _, a := range args[1:] {
		if v, err := strconv.ParseUint(a, 10, 32); err == nil {
			groupID = uint32(v)
			continue
		}
		switch strings.ToLower(a) {
		case "force":
			if !spawnFlags {
				return 0, false, false, false
			}
			flagA = true
		case "ignorerespawn":
			if !spawnFlags {
				return 0, false, false, false
			}
			flagB = true
		case "removerespawntime":
			if spawnFlags {
				return 0, false, false, false
			}
			flagA = true
		default:
			return 0, false, false, false
		}
	}
	return groupID, flagA, flagB, true
}
