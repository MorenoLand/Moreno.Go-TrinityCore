package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *Server) gameObjectStateLocked(mapID, instanceID uint32, guid uint64) *dynamicGameObjectState {
	if instanceID != 0 {
		if state := s.instanceGameObjects[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}][guid]; state != nil {
			return state
		}
	}
	if state := s.dynamicGameObjects[guid]; state != nil && state.Map == mapID && state.InstanceID == instanceID {
		return state
	}
	return nil
}

func (s *Server) gameObjectState(mapID, instanceID uint32, guid uint64) *dynamicGameObjectState {
	if s == nil {
		return nil
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	return s.gameObjectStateLocked(mapID, instanceID, guid)
}

func (s *Server) storeGameObjectState(state *dynamicGameObjectState) {
	if s == nil || state == nil {
		return
	}
	s.objectsMu.Lock()
	if state.InstanceID == 0 {
		if s.dynamicGameObjects == nil {
			s.dynamicGameObjects = make(map[uint64]*dynamicGameObjectState)
		}
		s.dynamicGameObjects[state.GUID] = state
	} else {
		key := instanceAdmissionKey{MapID: state.Map, InstanceID: state.InstanceID}
		if s.instanceGameObjects == nil {
			s.instanceGameObjects = make(map[instanceAdmissionKey]map[uint64]*dynamicGameObjectState)
		}
		if s.instanceGameObjects[key] == nil {
			s.instanceGameObjects[key] = make(map[uint64]*dynamicGameObjectState)
		}
		s.instanceGameObjects[key][state.GUID] = state
	}
	s.objectsMu.Unlock()
}

func (s *Server) gameObjectStatesInInstance(mapID, instanceID uint32) []dynamicGameObjectState {
	if s == nil {
		return nil
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	states := make([]dynamicGameObjectState, 0)
	for _, state := range s.dynamicGameObjects {
		if state != nil && state.Map == mapID && state.InstanceID == instanceID {
			states = append(states, *state)
		}
	}
	if instanceID != 0 {
		for _, state := range s.instanceGameObjects[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}] {
			if state != nil {
				states = append(states, *state)
			}
		}
	}
	return states
}

func (s *Server) setGameObjectStateInInstance(mapID, instanceID uint32, guid uint64, state uint8) {
	if s == nil {
		return
	}
	found := false
	var bytes1 uint32
	s.objectsMu.Lock()
	if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
		object.State = state
		bytes1 = uint32(object.State) | uint32(object.Type)<<8 | uint32(object.ArtKit)<<16 | uint32(object.AnimProgress)<<24
		found = true
	}
	s.objectsMu.Unlock()
	if !found {
		// A static DB GO has no live state until something materializes
		// one (getOrLoadGameObjectState does it for use/gossip/quest/
		// spell paths); a bare SetGoState used to be a silent no-op
		// for exactly those GOs, so scripted doors never moved and
		// GetGoState kept reading the spawn snapshot. Materialize
		// from the spawn row, then apply the state to the stored
		// record (re-resolved under the lock, so a racing
		// materialization wins over this call's snapshot).
		if s.materializeStaticGameObjectState(mapID, instanceID, guid) != nil {
			s.objectsMu.Lock()
			if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
				object.State = state
				bytes1 = uint32(object.State) | uint32(object.Type)<<8 | uint32(object.ArtKit)<<16 | uint32(object.AnimProgress)<<24
				found = true
			}
			s.objectsMu.Unlock()
		}
	}
	if !found {
		return
	}
	// GameObject::SetGoState (GameObject.cpp:2407) marks
	// GAMEOBJECT_BYTES_1 dirty; Go has no update tick, so the values
	// update is the client-visible half of the state change.
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectBytes1: bytes1})
	// Eluna GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED (event 10), fired from
	// GameObject::SetGoState (GameObject.cpp:2407): (event, go, state).
	// The hook runs after objectsMu is released — Lua handlers must never
	// run under the object lock.
	s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(state))
}

// setGameObjectLootState is the SetLootState write half (lua_objects.go):
// the live record wins, materializing a static spawn exactly like
// setGameObjectStateInInstance; without a live record the caller's
// closure copy is the only store and this is a no-op.
func (s *Server) setGameObjectLootState(mapID, instanceID uint32, guid uint64, value uint8) {
	if s == nil {
		return
	}
	if s.gameObjectState(mapID, instanceID, guid) == nil && s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
		return
	}
	s.objectsMu.Lock()
	if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
		object.LootState = value
		object.LootStateSet = true
	}
	s.objectsMu.Unlock()
}

// setGameObjectRespawnDelay is the SetRespawnTime write half
// (lua_objects.go): same materialization rule as setGameObjectLootState.
func (s *Server) setGameObjectRespawnDelay(mapID, instanceID uint32, guid uint64, delay int32) {
	if s == nil {
		return
	}
	if s.gameObjectState(mapID, instanceID, guid) == nil && s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
		return
	}
	s.objectsMu.Lock()
	if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
		object.RespawnDelaySec = delay
		object.RespawnDelaySet = true
	}
	s.objectsMu.Unlock()
}

// rekeyDynamicGameObject moves a live record to a new GUID (SaveToDB's
// spawn-id adoption for a runtime GO, GameObject.cpp:1004-1007): the
// caller holds no lock; the record's own GUID fields move with it.
func (s *Server) rekeyDynamicGameObject(mapID, instanceID uint32, oldGUID, newGUID uint64) {
	if s == nil || oldGUID == 0 || newGUID == 0 || oldGUID == newGUID {
		return
	}
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	var state *dynamicGameObjectState
	if instanceID == 0 {
		state = s.dynamicGameObjects[oldGUID]
		delete(s.dynamicGameObjects, oldGUID)
	} else {
		key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
		state = s.instanceGameObjects[key][oldGUID]
		delete(s.instanceGameObjects[key], oldGUID)
		if len(s.instanceGameObjects[key]) == 0 {
			delete(s.instanceGameObjects, key)
		}
	}
	if state == nil {
		return
	}
	state.GUID = newGUID
	state.LowGUID = uint32(newGUID & 0x00FFFFFF)
	if instanceID == 0 {
		if s.dynamicGameObjects == nil {
			s.dynamicGameObjects = make(map[uint64]*dynamicGameObjectState)
		}
		s.dynamicGameObjects[newGUID] = state
	} else {
		key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
		if s.instanceGameObjects == nil {
			s.instanceGameObjects = make(map[instanceAdmissionKey]map[uint64]*dynamicGameObjectState)
		}
		if s.instanceGameObjects[key] == nil {
			s.instanceGameObjects[key] = make(map[uint64]*dynamicGameObjectState)
		}
		s.instanceGameObjects[key][newGUID] = state
	}
}

// materializeStaticGameObjectState builds a live dynamic record for a
// static DB spawn, mirroring the grid-stream spawn-row query
// (gameobjects.go: the gameobject + template + template_addon +
// gameobject_addon join) so the record is a faithful copy of the spawn:
// position, spawn state (m_prevGoState), anim progress, type, display,
// size, addon flags/faction/artkit and parent rotation, phase. The
// record carries no owner/spell link and IsRuntimeSpawn stays false, so
// the summoned-GO legs (despawn-on-loot-release, goober auto-close
// delete, RemoveFromWorld owner gate) all treat it as a static GO, and
// the grid stream keeps sourcing flags/display from the DB row with
// only State overridden. Unknown GUIDs and map mismatches return nil
// and preserve the old silent no-op. No lock is held across the DB
// query; a racing materialization is benign (identical snapshot) and
// the caller re-resolves under the lock.
func (s *Server) materializeStaticGameObjectState(mapID, instanceID uint32, guid uint64) *dynamicGameObjectState {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || guid == 0 {
		return nil
	}
	low := uint32(guid & 0xFFFFFF)
	entry := uint32(guid>>24) & 0xFFFFFF
	var goX, goY, goZ, goO, size, pr0, pr1, pr2, pr3 float64
	var goState, animProgress, goType, displayID, flags, faction, artKit, data1, phaseMask int64
	var iconName string
	err := s.WorldStore.DB.QueryRowContext(context.Background(), `SELECT g.position_x, g.position_y, g.position_z, g.orientation, g.state, g.animprogress,
		t.type, t.displayId, t.size, COALESCE(t.data1, 0), COALESCE(t.IconName, ''), COALESCE(g.phaseMask, 0),
		COALESCE(ta.flags, 0), COALESCE(ta.faction, 0), COALESCE(ta.artkit0, 0),
		COALESCE(ga.parent_rotation0, 0), COALESCE(ga.parent_rotation1, 0), COALESCE(ga.parent_rotation2, 0), COALESCE(ga.parent_rotation3, 1)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		LEFT JOIN gameobject_template_addon AS ta ON ta.entry = g.id
		LEFT JOIN gameobject_addon AS ga ON ga.guid = g.guid
		WHERE g.guid = ? AND g.id = ? AND g.map = ? LIMIT 1`, low, entry, mapID).Scan(
		&goX, &goY, &goZ, &goO, &goState, &animProgress, &goType, &displayID, &size, &data1, &iconName, &phaseMask,
		&flags, &faction, &artKit, &pr0, &pr1, &pr2, &pr3)
	if err != nil {
		return nil
	}
	dyn := &dynamicGameObjectState{
		GUID:           guid,
		LowGUID:        low,
		Entry:          entry,
		Map:            mapID,
		InstanceID:     instanceID,
		X:              float32(goX),
		Y:              float32(goY),
		Z:              float32(goZ),
		Orientation:    float32(goO),
		State:          uint8(goState),
		SpawnState:     uint8(goState),
		AnimProgress:   uint8(animProgress),
		ArtKit:         uint8(artKit),
		Type:           uint8(goType),
		DisplayID:      uint32(displayID),
		Size:           float32(size),
		IconName:       iconName,
		Flags:          uint32(flags),
		Faction:        uint32(faction),
		PhaseMask:      uint32(phaseMask),
		Data1:          uint32(data1),
		ParentRotation: [4]float32{float32(pr0), float32(pr1), float32(pr2), float32(pr3)},
	}
	s.storeGameObjectState(dyn)
	return dyn
}

// goFlagInUse is GO_FLAG_IN_USE (SharedDefines.h:1637): disables interaction
// while animated. Doors/buttons set it in UseDoorOrButton (via
// SwitchDoorOrButton) and ResetDoorOrButton clears it; it doubles as the Go
// twin of the lootState != GO_READY re-use gate.
const goFlagInUse uint32 = 0x00000001

// useDoorOrButton mirrors GameObject::UseDoorOrButton + SwitchDoorOrButton
// (GameObject.cpp:1434-1484): a use while activated is dropped (the
// lootState != GO_READY early-out, keyed on GO_FLAG_IN_USE here). With
// alternative set (GameObjectActions::Destroy, SpellEffects.cpp:3986) a
// closed door flips to GO_STATE_DESTROYED (SharedDefines.h:1663) instead of
// ACTIVE; an open or destroyed door flips back to READY. GO_FLAG_IN_USE goes
// out on the flags field, the new state on bytes1, and the auto-close is
// scheduled from time_to_restore — 0 falls back to the template autoCloseTime
// (door/button data2, consumed as milliseconds against the ms game clock —
// m_cooldownTime = GetGameTimeMS() + GetAutoCloseTime()) whenever the door is
// left non-ready. A zero template value means no auto-close (m_cooldownTime =
// 0 fails the Update arm's m_cooldownTime check); scripts close it via
// ResetDoorOrButton. Shared by handleGameObjectUse (door and button share the
// identical C++ Use arm: UseDoorOrButton(0, false, user)) and the DoLootRelease
// door arm (LootHandler.cpp:281-285).
func (s *Server) useDoorOrButton(mapID, instanceID uint32, guid uint64, alternative bool, delayMs uint32) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st == nil {
		// Same materialization as setGameObjectStateInInstance: a
		// static door/button has no live state until something
		// creates one, and C++ UseDoorOrButton works on every door,
		// not just summoned ones. The auto-close below then runs
		// off the materialized record like any dynamic GO.
		s.objectsMu.Unlock()
		if s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
			return
		}
		s.objectsMu.Lock()
		st = s.gameObjectStateLocked(mapID, instanceID, guid)
		if st == nil {
			s.objectsMu.Unlock()
			return
		}
	}
	if st.Flags&goFlagInUse != 0 {
		s.objectsMu.Unlock()
		return
	}
	entry := st.Entry
	st.Flags |= goFlagInUse
	newState := GameObjectStateReady
	if st.State == GameObjectStateReady {
		newState = GameObjectStateActive
		if alternative {
			// GO_STATE_DESTROYED twin (SharedDefines.h:1663); Go reuses the
			// ActiveAlternative constant for value 2.
			newState = GameObjectStateActiveAlternative
		}
	}
	st.State = newState
	flags := st.Flags
	bytes1 := uint32(newState) | uint32(st.Type)<<8 | uint32(st.ArtKit)<<16 | uint32(st.AnimProgress)<<24
	s.objectsMu.Unlock()
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags, gameObjectBytes1: bytes1})
	// Eluna GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED (event 10), fired from
	// GameObject::SetGoState (GameObject.cpp:2407), like
	// setGameObjectStateInInstance.
	s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(newState))
	// GameObject.cpp:1447 arms m_cooldownTime on every UseDoorOrButton,
	// including the close-toggle branch, so the GO_ACTIVATED Update arm
	// always fires ResetDoorOrButton (clears GO_FLAG_IN_USE, restores
	// m_prevGoState). A newState gate here left close-toggled doors flagged
	// IN_USE forever and permanently unusable.
	restoreMs := delayMs
	if restoreMs == 0 {
		if d := s.gameObjectAutoCloseDelay(entry); d > 0 {
			restoreMs = uint32(d / time.Millisecond)
		}
	}
	if restoreMs > 0 {
		s.scheduleGameObjectResetInInstance(mapID, instanceID, guid, time.Duration(restoreMs)*time.Millisecond, true)
	}
}

// serverActivateGameObjectUse is the Server-level twin of the session
// activateGameObjectUse (gameobjects.go:1068) for a creature caster with no
// player session: Spell::EffectActivateObject Disturb/Open
// (SpellEffects.cpp:3956-3960) calls GameObject::Use on the target for any
// unit caster, and the player-only head arms (immunity, mounted, ClearMenus,
// gossip hello — GameObject.cpp:1491-1505) sit inside `if (Player*)`, so a
// creature user skips them; only the template-cooldown gate
// (GameObject.cpp:1513-1518) and the type dispatch run. Door/button,
// questgiver, trap and chair arms stay out: door/button already run through
// useDoorOrButton on the other effect arms, and the rest are player-only in
// C++ (GameObject.cpp:1520-1560).
func (s *Server) serverActivateGameObjectUse(ctx context.Context, m *creatureMotion, guid uint64) {
	if s == nil || m == nil || guid == 0 {
		return
	}
	entry := uint32((guid >> 24) & 0xFFFFFF)
	lowGUID := uint32(guid & 0xFFFFFF)
	goState, err := s.getOrLoadGameObjectState(ctx, guid, lowGUID, entry, m.Map, m.InstanceID)
	if err != nil || goState == nil {
		return
	}
	// GameObject::IsInvisibleDueToDespawn (GameObject.cpp:1293): the
	// spell-effect target resolves from the map, so a despawned GO never
	// reaches Use — same guard as the session twin.
	if s.isGameObjectHiddenInInstance(m.Map, m.InstanceID, guid) {
		return
	}
	if !s.admitServerGameObjectUse(entry, goState) {
		return
	}
	switch goState.Type {
	case GameObjectTypeGoober:
		s.serverUseGooberGameObject(ctx, guid, entry, goState)
	}
}

// admitServerGameObjectUse is the Server-level twin of the session
// admitGameObjectUse (gameobjects.go:562): the GameObject::Use
// template-cooldown gate (GameObject.cpp:1513-1518) for a use with no player
// session.
func (s *Server) admitServerGameObjectUse(entry uint32, goState *dynamicGameObjectState) bool {
	if s == nil || goState == nil {
		return true
	}
	if goState.Type != GameObjectTypeTrap && goState.Type != GameObjectTypeGoober {
		return true
	}
	cd := s.gameObjectTemplateCooldown(entry, goState.Type)
	if cd <= 0 {
		return true
	}
	now := time.Now().UnixMilli()
	s.objectsMu.Lock()
	blocked := now < goState.CooldownUntilMS
	if !blocked {
		goState.CooldownUntilMS = now + int64(cd)*1000
	}
	s.objectsMu.Unlock()
	return !blocked
}

// serverGooberTemplateData carries the gameobject_template fields the
// server-side goober arm reads (GameObjectData.h goober struct): data0
// lockId, data3 autoCloseTime, data4 customAnim, data5 consumable,
// data10 spellId.
type serverGooberTemplateData struct {
	lockID, autoCloseTime, customAnim, consumable, spellID uint32
}

// loadServerGooberTemplate is the Server-level twin of the session
// loadGooberTemplate (gameobjects.go:1448), extended with the consumable
// flag the GO_JUST_DEACTIVATED despawn leg reads.
func (s *Server) loadServerGooberTemplate(ctx context.Context, entry uint32) serverGooberTemplateData {
	var tpl serverGooberTemplateData
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return tpl
	}
	var lockID, autoCloseTime, customAnim, consumable, spellID int64
	err := s.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(Data0, 0), COALESCE(Data3, 0), COALESCE(Data4, 0), COALESCE(Data5, 0), COALESCE(Data10, 0)
		FROM gameobject_template WHERE entry = ? LIMIT 1`, entry).
		Scan(&lockID, &autoCloseTime, &customAnim, &consumable, &spellID)
	if err != nil {
		return tpl
	}
	tpl.lockID = uint32(lockID)
	tpl.autoCloseTime = uint32(autoCloseTime)
	tpl.customAnim = uint32(customAnim)
	tpl.consumable = uint32(consumable)
	tpl.spellID = uint32(spellID)
	return tpl
}

// serverUseGooberGameObject is the Server-level twin of the session
// useGooberGameObject (gameobjects.go:583) for a creature user:
// GameObject::Use GAMEOBJECT_TYPE_GOOBER arm (GameObject.cpp:1630-1696).
// The page, gossip, eventId, quest-gate and KillCreditGO legs sit inside
// `if (Player*)` in C++ and never run for a creature user; the linked-trap
// leg has no Go trap model (same as the session twin). What runs is the
// state flip, the autoCloseTime re-arm, the reset schedule, and the
// template-spell cast with the GO as caster (GameObject.cpp:2075-2097).
func (s *Server) serverUseGooberGameObject(ctx context.Context, guid uint64, entry uint32, goState *dynamicGameObjectState) {
	if s == nil || goState == nil {
		return
	}
	tpl := s.loadServerGooberTemplate(ctx, entry)
	if tpl.customAnim != 0 {
		s.broadcastGameObjectCustomAnimInInstance(goState.Map, goState.InstanceID, guid, uint32(goState.AnimProgress))
	} else {
		s.setGameObjectStateInInstance(goState.Map, goState.InstanceID, guid, GameObjectStateActive)
	}
	// GameObject.cpp:1689: m_cooldownTime = GetGameTimeMS() +
	// GetAutoCloseTime() overwrites the head gate's data6 re-arm, same as
	// the session twin.
	s.objectsMu.Lock()
	goState.CooldownUntilMS = time.Now().UnixMilli() + int64(tpl.autoCloseTime)
	s.objectsMu.Unlock()
	// The GO_ACTIVATED update arm fires at the next tick and the
	// GO_JUST_DEACTIVATED arm re-readies the state when the template
	// carries a lockId or an autoCloseTime (GameObject.cpp:797-798); a
	// consumable goober with neither still reaches deactivation at the
	// next tick, so it schedules the zero-delay reset that despawns it.
	if tpl.autoCloseTime > 0 {
		s.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, time.Duration(tpl.autoCloseTime)*time.Millisecond, false)
	} else if tpl.lockID != 0 {
		s.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, 0, false)
	} else if tpl.consumable != 0 {
		s.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, 0, false)
	}
	// GameObject::Use tail (GameObject.cpp:2093-2094): the GO casts the
	// template spell on the user. The zero duration lets the GO-origin
	// summon helper fall back to the spell's DurationIndex (its doc
	// asserts 5 minutes for 24081); the helper stays silent with no
	// Spell.dbc.
	if tpl.spellID != 0 {
		s.serverSummonFromGameObject(ctx, goState, tpl.spellID, 0)
	}
}

// setGameObjectFlag mirrors GameObject::SetFlag(GAMEOBJECT_FLAGS, flag)
// (GameObject.cpp ApplyModFlag path): ORs the bits into the live dynamic
// state and broadcasts the flags field, the same values-update twin
// useDoorOrButton uses for GO_FLAG_IN_USE. Only the flags field exists
// on the Go GO model, so the Lua SetFlag bridge gates on it. A static DB
// spawn gains its live state on its first flag write, the
// setGameObjectStateInInstance materialization: a bare SetFlag used to be
// a silent no-op for exactly those GOs, so scripted flag arms (the
// Zul'Gurub gong, the suppression devices) never stuck.
func (s *Server) setGameObjectFlag(mapID, instanceID uint32, guid uint64, flag uint32) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st == nil {
		s.objectsMu.Unlock()
		if s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
			return
		}
		s.objectsMu.Lock()
		st = s.gameObjectStateLocked(mapID, instanceID, guid)
		if st == nil {
			s.objectsMu.Unlock()
			return
		}
	}
	st.Flags |= flag
	flags := st.Flags
	s.objectsMu.Unlock()
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags})
}

// clearGameObjectFlag mirrors GameObject::RemoveFlag(GAMEOBJECT_FLAGS, flag)
// (GameObject.cpp ApplyModFlag path): AND-NOTs the bits out of the live
// dynamic state and broadcasts the flags field, the setGameObjectFlag twin,
// including its static materialization: a RemoveFlag on a never-touched
// static spawn (the Karazhan urn's IN_USE clear) materializes the spawn
// flags first, so the clear applies to the spawn value instead of
// dropping on the floor.
func (s *Server) clearGameObjectFlag(mapID, instanceID uint32, guid uint64, flag uint32) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st == nil {
		s.objectsMu.Unlock()
		if s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
			return
		}
		s.objectsMu.Lock()
		st = s.gameObjectStateLocked(mapID, instanceID, guid)
		if st == nil {
			s.objectsMu.Unlock()
			return
		}
	}
	st.Flags &^= flag
	flags := st.Flags
	s.objectsMu.Unlock()
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags})
}

// gameObjectHasFlag mirrors GameObject::HasFlag(GAMEOBJECT_FLAGS, flag):
// the live dynamic flags field when one exists, else the spawn flags in
// grid-stream precedence: the destructible-health override for damaged
// buildings, then the template_addon flags (GameObject.cpp:336, the
// GameObjectOverride value a static spawn is created with). A static GO
// that was never touched used to read false for every flag, including
// bits its spawn row carries; the read itself never materializes a
// record, so a pure probe leaves no live state behind.
func (s *Server) gameObjectHasFlag(mapID, instanceID uint32, guid uint64, flag uint32) bool {
	if s == nil {
		return false
	}
	s.objectsMu.RLock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st != nil {
		flags := st.Flags
		s.objectsMu.RUnlock()
		return flags&flag != 0
	}
	s.objectsMu.RUnlock()
	if over := s.destructibleSpawnOverride(mapID, instanceID, uint32(guid&0xFFFFFF)); over != nil {
		return over.Flags&flag != 0
	}
	return s.gameObjectTemplateAddonFlags(uint32(guid>>24)&0xFFFFFF)&flag != 0
}

// gameObjectTemplateAddonFlags mirrors the spawn-flags read behind
// GameObject::Create (GameObject.cpp:336): the gameobject_template_addon
// flags for the entry, 0 when the row is absent. The gameObjectTemplateType
// twin, one column over.
func (s *Server) gameObjectTemplateAddonFlags(entry uint32) uint32 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	var flags int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(flags, 0) FROM gameobject_template_addon WHERE entry = ? LIMIT 1", entry).Scan(&flags); err != nil || flags < 0 {
		return 0
	}
	return uint32(flags)
}

// gameObjectTemplateAddonFaction mirrors the spawn-faction read behind
// GameObject::Create (GameObject.cpp:336): the gameobject_template_addon
// faction for the entry, 0 when the row is absent. The
// gameObjectTemplateAddonFlags twin, one column over.
func (s *Server) gameObjectTemplateAddonFaction(entry uint32) uint32 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	var faction int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(faction, 0) FROM gameobject_template_addon WHERE entry = ? LIMIT 1", entry).Scan(&faction); err != nil || faction < 0 {
		return 0
	}
	return uint32(faction)
}

// gameObjectAutoCloseDelay mirrors GameObjectTemplate::GetAutoCloseTime
// (GameObjectData.h:535) for the door/button arms: data2, consumed as
// milliseconds. Zero (or an unavailable DB/template) means no auto-close.
func (s *Server) gameObjectAutoCloseDelay(entry uint32) time.Duration {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	var d int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		`SELECT COALESCE(data2, 0) FROM gameobject_template WHERE entry = ? LIMIT 1`, entry).Scan(&d); err != nil || d <= 0 {
		return 0
	}
	return time.Duration(d) * time.Millisecond
}

// gameObjectTemplateType mirrors the template-type read behind
// GameObject::IsTransport (GameObject.cpp): 0 on an unavailable DB/template.
func (s *Server) gameObjectTemplateType(entry uint32) uint8 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	var t int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(type, 0) FROM gameobject_template WHERE entry = ? LIMIT 1", entry).Scan(&t); err != nil || t < 0 || t > 255 {
		return 0
	}
	return uint8(t)
}

// gameObjectTemplateCooldown mirrors GameObjectTemplate::GetCooldown
// (GameObjectData.h:582): trap.cooldown (data5) / goober.cooldown (data6),
// in seconds; 0 when the template has none.
func (s *Server) gameObjectTemplateCooldown(entry uint32, goType uint8) uint32 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	var col string
	switch goType {
	case GameObjectTypeTrap:
		col = "data5"
	case GameObjectTypeGoober:
		col = "data6"
	default:
		return 0
	}
	var d int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE("+col+", 0) FROM gameobject_template WHERE entry = ? LIMIT 1", entry).Scan(&d); err != nil || d <= 0 {
		return 0
	}
	return uint32(d)
}

// gameObjectTemplateGooberConsumable mirrors GameObjectTemplate::
// IsDespawnAtAction (GameObjectData.h:431) for goobers: goober.consumable
// (data5) != 0.
func (s *Server) gameObjectTemplateGooberConsumable(entry uint32) bool {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return false
	}
	var d int64
	if err := s.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(data5, 0) FROM gameobject_template WHERE entry = ? LIMIT 1", entry).Scan(&d); err != nil || d <= 0 {
		return false
	}
	return true
}

// resetDoorOrButtonLocked mirrors GameObject::ResetDoorOrButton
// (GameObject.cpp:1422-1431); the caller must hold objectsMu. It is a no-op
// when the door/button was never activated (or already reset) — the IN_USE
// flag is the Go twin of the GO_READY/GO_JUST_DEACTIVATED early-out.
// Otherwise the flag is cleared and the spawn state (m_prevGoState, set once
// in GameObject::Create, GameObject.cpp:349) is restored. It returns the
// values to broadcast.
func (s *Server) resetDoorOrButtonLocked(st *dynamicGameObjectState) (flags, bytes1 uint32, newState uint8, ok bool) {
	if st == nil || st.Flags&goFlagInUse == 0 {
		return 0, 0, 0, false
	}
	st.Flags &^= goFlagInUse
	st.State = st.SpawnState
	flags = st.Flags
	newState = st.State
	bytes1 = uint32(newState) | uint32(st.Type)<<8 | uint32(st.ArtKit)<<16 | uint32(st.AnimProgress)<<24
	return flags, bytes1, newState, true
}

// resetDoorOrButton is the unlocked ResetDoorOrButton twin for the spell
// Close/Rebuild/CloseAndLock arms.
func (s *Server) resetDoorOrButton(mapID, instanceID uint32, guid uint64) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	flags, bytes1, newState, ok := s.resetDoorOrButtonLocked(st)
	s.objectsMu.Unlock()
	if !ok {
		return
	}
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags, gameObjectBytes1: bytes1})
	s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(newState))
}

func (s *Server) scheduleGameObjectResetInInstance(mapID, instanceID uint32, guid uint64, delay time.Duration, restorePrev bool) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	state := s.gameObjectStateLocked(mapID, instanceID, guid)
	if state == nil {
		s.objectsMu.Unlock()
		return
	}
	if state.AutoCloseTimer != nil {
		state.AutoCloseTimer.Stop()
	}
	state.AutoCloseTimer = time.AfterFunc(delay, func() {
		// The pointer-identity check preserves the original semantics: a
		// replaced state object is not reset.
		s.objectsMu.Lock()
		current := s.gameObjectStateLocked(mapID, instanceID, guid)
		if current != state {
			s.objectsMu.Unlock()
			return
		}
		var flags uint32
		var newState uint8
		var bytes1 uint32
		hasBytes1 := false
		if restorePrev {
			var ok bool
			flags, bytes1, newState, ok = s.resetDoorOrButtonLocked(current)
			if !ok {
				s.objectsMu.Unlock()
				return
			}
			hasBytes1 = true
		} else {
			// GO_JUST_DEACTIVATED goober arm (GameObject.cpp:778-814): after
			// the (vacuous for pure goobers — m_unique_users is only ever
			// populated by the ritual arm) spell tail and the
			// lockId/autoCloseTime re-ready, a summoned GO (owner or spell
			// link) whose respawn timer is still live is DELETED, not
			// re-readied — the isSummonedAndExpired keep-alive only fires
			// when (owner || spellId) && m_respawnTime == 0, i.e. a
			// zero-duration summon. Go: delete when the GO carries an
			// owner/spell link AND an active DespawnTimer (the live
			// m_respawnTime twin); the zero-duration keep-alive keeps the
			// old re-ready behavior. Previously a spell-summoned goober
			// lingered after its auto-close instead of vanishing.
			if current != nil && (current.OwnerGUID != 0 || current.SpellID != 0) && current.DespawnTimer != nil {
				s.objectsMu.Unlock()
				s.despawnDynamicGameObjectInInstance(mapID, instanceID, guid)
				return
			}
			// GameObjectTemplate::IsDespawnAtAction (GameObjectData.h:431):
			// a consumable goober fails the GO_JUST_DEACTIVATED keep-alive
			// (GameObject.cpp:810) and despawns instead of re-readying —
			// the spider egg (consumable=1) vanishes at deactivation and
			// comes back on the scripted respawn scan, not on re-ready.
			// Gated on goober: chest.consumable shares the data5 column
			// but chests never reach this branch.
			if current.Type == GameObjectTypeGoober && s.gameObjectTemplateGooberConsumable(current.Entry) {
				s.objectsMu.Unlock()
				s.despawnDynamicGameObjectInInstance(mapID, instanceID, guid)
				return
			}
			// Goober reset twin: the C++ GO_JUST_DEACTIVATED update arm
			// (GameObject.cpp:797-798) re-readies the state when the
			// template carries a lockId or autoCloseTime; SetGoState marks
			// GAMEOBJECT_BYTES_1 dirty (GameObject.cpp:2407-2409), so the
			// READY flip is broadcast like the door/button reset above.
			current.State = GameObjectStateReady
			bytes1 = uint32(current.State) | uint32(current.Type)<<8 | uint32(current.ArtKit)<<16 | uint32(current.AnimProgress)<<24
			hasBytes1 = true
		}
		flags = current.Flags
		newState = current.State
		s.objectsMu.Unlock()
		fields := map[int]uint32{gameObjectFlags: flags}
		if hasBytes1 {
			fields[gameObjectBytes1] = bytes1
		}
		s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, fields)
		// Eluna GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED (event 10), fired from
		// GameObject::SetGoState (GameObject.cpp:2407).
		s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(newState))
	})
	s.objectsMu.Unlock()
}

func (s *Server) setGameObjectHiddenInInstance(mapID, instanceID uint32, guid uint64, hidden bool) {
	if s == nil || instanceID == 0 {
		s.setGameObjectHidden(guid, hidden)
		return
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	s.objectsMu.Lock()
	if s.instanceHiddenGameObjects == nil {
		s.instanceHiddenGameObjects = make(map[instanceAdmissionKey]map[uint64]struct{})
	}
	if s.instanceHiddenGameObjects[key] == nil {
		s.instanceHiddenGameObjects[key] = make(map[uint64]struct{})
	}
	if hidden {
		s.instanceHiddenGameObjects[key][guid] = struct{}{}
	} else {
		delete(s.instanceHiddenGameObjects[key], guid)
		if len(s.instanceHiddenGameObjects[key]) == 0 {
			delete(s.instanceHiddenGameObjects, key)
		}
	}
	s.objectsMu.Unlock()
}

func (s *Server) isGameObjectHiddenInInstance(mapID, instanceID uint32, guid uint64) bool {
	if s == nil {
		return false
	}
	s.objectsMu.RLock()
	hidden := false
	if instanceID == 0 {
		_, hidden = s.hiddenGameObjects[guid]
	}
	if scoped := s.instanceHiddenGameObjects[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; scoped != nil {
		_, hiddenInInstance := scoped[guid]
		hidden = hidden || hiddenInInstance
	}
	s.objectsMu.RUnlock()
	return hidden
}

// gameObjectPositionInInstance returns the live dynamic GO position for the
// visibility-set fan-out below. GameObject::SendCustomAnim
// (GameObject.cpp:2097) and WorldObject::SendObjectDeSpawnAnim
// (Object.cpp:1826) both use SendMessageToSet(true) = the GO's visibility
// set, and WorldObject::BuildUpdate (Object.cpp:3620) builds values updates
// "for all visible players" — so all four instance helpers center on the GO
// instead of the whole instance. Unknown GUIDs keep the old instance-wide
// fan-out so no recipient is dropped for an unlocated GO.
func (s *Server) gameObjectPositionInInstance(mapID, instanceID uint32, guid uint64) (float32, float32, float32, uint32, bool) {
	if s == nil {
		return 0, 0, 0, 0, false
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st == nil {
		return 0, 0, 0, 0, false
	}
	return st.X, st.Y, st.Z, st.PhaseMask, true
}

func (s *Server) broadcastToVisibleSetForGameObject(opcode uint16, payload []byte, mapID, instanceID uint32, guid uint64) {
	if x, y, z, phase, ok := s.gameObjectPositionInInstance(mapID, instanceID, guid); ok {
		s.broadcastToVisibleSetFromPhase(opcode, payload, mapID, instanceID, x, y, z, phase)
		return
	}
	s.broadcastToInstance(mapID, instanceID, opcode, payload, nil)
}

func (s *Server) gameObjectPositionOnMap(mapID uint32, guid uint64) (instanceID uint32, x, y, z float32, phase uint32, ok bool) {
	if s == nil {
		return 0, 0, 0, 0, 0, false
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	if st := s.dynamicGameObjects[guid]; st != nil && st.Map == mapID {
		return st.InstanceID, st.X, st.Y, st.Z, st.PhaseMask, true
	}
	for key, states := range s.instanceGameObjects {
		if key.MapID != mapID {
			continue
		}
		if st := states[guid]; st != nil {
			return key.InstanceID, st.X, st.Y, st.Z, st.PhaseMask, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

func (s *Server) broadcastToVisibleSetForGameObjectOnMap(opcode uint16, payload []byte, mapID uint32, guid uint64) {
	if instanceID, x, y, z, phase, ok := s.gameObjectPositionOnMap(mapID, guid); ok {
		s.broadcastToVisibleSetFromPhase(opcode, payload, mapID, instanceID, x, y, z, phase)
		return
	}
	s.broadcastToMap(mapID, opcode, payload)
}

func (s *Server) broadcastGameObjectCustomAnimInInstance(mapID, instanceID uint32, guid uint64, anim uint32) {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(guid)
	buf.WriteU32(anim)
	s.broadcastToVisibleSetForGameObject(uint16(protocol.OpcodeSMSG_GAMEOBJECT_CUSTOM_ANIM), buf.Bytes(), mapID, instanceID, guid)
}

func (s *Server) broadcastGameObjectResetStateInInstance(mapID, instanceID uint32, guid uint64) {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToVisibleSetForGameObject(uint16(protocol.OpcodeSMSG_GAMEOBJECT_RESET_STATE), buf.Bytes(), mapID, instanceID, guid)
}

func (s *Server) broadcastGameObjectDespawnInInstance(mapID, instanceID uint32, guid uint64) {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToVisibleSetForGameObject(uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), buf.Bytes(), mapID, instanceID, guid)
}

// broadcastGameObjectValuesUpdateInInstance mirrors the values-update half of
// the runtime GameObject field writes (e.g. GameObject::ApplyModFlag,
// GameObject::SetGoArtKit): an SMSG_UPDATE_OBJECT values block carrying only
// the changed fields. Go previously never updated GO fields at runtime
// (flags/artkit only rode the create block), so flag and artkit arms had no
// client-visible path.
func (s *Server) broadcastGameObjectValuesUpdateInInstance(mapID, instanceID uint32, guid uint64, fields map[int]uint32) {
	if s == nil || len(fields) == 0 {
		return
	}
	values := make([]uint32, gameObjectValuesCount)
	mask := protocol.NewUpdateMask(gameObjectValuesCount)
	for index, value := range fields {
		if index < 0 || index >= gameObjectValuesCount {
			continue
		}
		values[index] = value
		_ = mask.Set(index)
	}
	block := protocol.NewBuffer(64 + len(fields)*4)
	block.WriteU8(protocol.UpdateValues)
	block.WritePackedGUID(guid)
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index := 0; index < gameObjectValuesCount; index++ {
		if mask.Has(index) {
			block.WriteU32(values[index])
		}
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block.Bytes())
	packet, err := updates.BuildPacket(0)
	if err != nil || packet == nil {
		return
	}
	s.broadcastToVisibleSetForGameObject(packet.Opcode, packet.Payload.Bytes(), mapID, instanceID, guid)
}

// broadcastGameObjectBytes1InInstance pushes the recomputed GAMEOBJECT_BYTES_1
// dword (state | type<<8 | artkit<<16 | animprogress<<24, gameobjects.go) after
// a runtime state/artkit change such as the Destroy or UseArtKit activate
// arms, which have no dedicated anim packet.
func (s *Server) broadcastGameObjectBytes1InInstance(mapID, instanceID uint32, guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	var bytes1 uint32
	found := false
	s.objectsMu.Lock()
	if st := s.gameObjectStateLocked(mapID, instanceID, guid); st != nil {
		bytes1 = uint32(st.State) | uint32(st.Type)<<8 | uint32(st.ArtKit)<<16 | uint32(st.AnimProgress)<<24
		found = true
	}
	s.objectsMu.Unlock()
	if found {
		s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectBytes1: bytes1})
	}
}

// applyGameObjectFlag mirrors GameObject::ApplyModFlag(GAMEOBJECT_FLAGS, flag,
// apply): it flips the bit on the live GO state and pushes the flags field to
// clients. A static DB spawn gains its live state on its first flag write,
// the setGameObjectStateInInstance materialization: the spell
// EffectActivateObject lock/unlock arms (spells.go, boss_ai.go) target
// static instance doors, and used to drop on the floor for exactly them.
func (s *Server) applyGameObjectFlag(mapID, instanceID uint32, guid uint64, flag uint32, set bool) {
	if s == nil || guid == 0 {
		return
	}
	var flags uint32
	found := false
	s.objectsMu.Lock()
	st := s.gameObjectStateLocked(mapID, instanceID, guid)
	if st == nil {
		s.objectsMu.Unlock()
		if s.materializeStaticGameObjectState(mapID, instanceID, guid) == nil {
			return
		}
		s.objectsMu.Lock()
		st = s.gameObjectStateLocked(mapID, instanceID, guid)
		if st == nil {
			s.objectsMu.Unlock()
			return
		}
	}
	if set {
		st.Flags |= flag
	} else {
		st.Flags &^= flag
	}
	flags = st.Flags
	found = true
	s.objectsMu.Unlock()
	if found {
		s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags})
	}
}

func (s *Server) despawnDynamicGameObjectInInstance(mapID, instanceID uint32, guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	// Resolved before deletion: the ev13 fire below runs after the map
	// delete, when a fresh resolve would always miss.
	goObj := s.serverLuaGameObject(context.Background(), guid)
	s.objectsMu.Lock()
	var x, y, z float32
	var phase uint32
	state := s.gameObjectStateLocked(mapID, instanceID, guid)
	if state != nil {
		if state.AutoCloseTimer != nil {
			state.AutoCloseTimer.Stop()
		}
		if state.DespawnTimer != nil {
			state.DespawnTimer.Stop()
		}
		if instanceID == 0 {
			delete(s.dynamicGameObjects, guid)
		} else {
			key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
			delete(s.instanceGameObjects[key], guid)
			if len(s.instanceGameObjects[key]) == 0 {
				delete(s.instanceGameObjects, key)
			}
		}
	}
	if state != nil {
		x, y, z = state.X, state.Y, state.Z
		phase = state.PhaseMask
	}
	s.objectsMu.Unlock()
	if state == nil {
		return
	}
	// Unit::RemoveGameObject (Unit.cpp:5246-5251): removing a game object
	// created by a cooldown-started-on-event spell releases the owner's
	// infinity cooldown via SendCooldownEvent. The recast arm in
	// handleEffectSummonObject nulls the spell id before despawning, so a
	// same-spell recast never releases here.
	if state.SpellID != 0 && state.OwnerGUID != 0 && s.Data != nil {
		if owner := s.findSessionByGUID(state.OwnerGUID); owner != nil && owner.playerGUID == state.OwnerGUID {
			if spell, found, err := s.Data.Spell(state.SpellID); err == nil && found && s.spellIsCooldownStartedOnEvent(spell) {
				owner.sendSpellCooldownEvent(spell)
			}
		}
	}
	// SMSG_FISH_ESCAPED fires only for fishing bobbers, on the uncaught
	// bite-timeout arm (GameObject.cpp:577-592: FISHINGNODE with the owner
	// link still live and the catch never handled) — never for other owned
	// runtime GOs (rituals, slot summons).
	if state.Type == GameObjectTypeFishingNode && state.OwnerGUID != 0 && !state.FishingHandled {
		if owner := s.findSessionByGUID(state.OwnerGUID); owner != nil {
			_ = owner.write(uint16(protocol.OpcodeSMSG_FISH_ESCAPED), nil, true)
		}
	}
	// The state was deleted above, so the lookup-based despawn twin would
	// always miss and fall back instance-wide — fan out from the captured
	// coordinates instead (SendObjectDeSpawnAnim, Object.cpp:1826).
	s.broadcastGameObjectDespawnAt(mapID, instanceID, guid, x, y, z, phase)
	// Eluna WORLD_EVENT_ON_DELETE_GAMEOBJECT (32): same Delete /
	// DespawnOrUnsummon twin as the world despawn path above, fired
	// with the same pre-delete object. C++ order is 32 at queue time,
	// then 13 at RemoveFromWorld — so 32 fires first here, then 13
	// (previously this path fired 13 before 32).
	if goObj != nil && s.Features != nil && s.Features.Scripts != nil {
		_, _ = s.Features.Scripts.TriggerServerEvent(context.Background(), scripting.ServerEventDeleteGameObject, goObj)
	}
	// Eluna GAMEOBJECT_EVENT_ON_REMOVE (event 13), fired from
	// GameObject::RemoveFromWorld (GameObject.cpp:244): (event, gameobject).
	// Only dynamic despawns have a live server-side object — static template
	// GOs are per-client update packets, so their map removal has no Go
	// counterpart. The object is the pre-delete resolve above: the lookup
	// maps no longer contain it here, so a fresh resolve would always miss
	// and the event would never fire.
	if goObj != nil {
		s.triggerGameObjectEventWithObject(context.Background(), goObj, uint32((guid>>24)&0x00FFFFFF), scripting.GameObjectEventOnRemove)
	}
}
