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
	s.objectsMu.Lock()
	if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
		object.State = state
		found = true
	}
	s.objectsMu.Unlock()
	// Eluna GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED (event 10), fired from
	// GameObject::SetGoState (GameObject.cpp:2407): (event, go, state).
	// The hook runs after objectsMu is released — Lua handlers must never
	// run under the object lock.
	if found {
		s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(state))
	}
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
		s.objectsMu.Unlock()
		return
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

func (s *Server) broadcastGameObjectCustomAnimInInstance(mapID, instanceID uint32, guid uint64, anim uint32) {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(guid)
	buf.WriteU32(anim)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_CUSTOM_ANIM), buf.Bytes(), nil)
}

func (s *Server) broadcastGameObjectResetStateInInstance(mapID, instanceID uint32, guid uint64) {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_RESET_STATE), buf.Bytes(), nil)
}

func (s *Server) broadcastGameObjectDespawnInInstance(mapID, instanceID uint32, guid uint64) {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), buf.Bytes(), nil)
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
	s.broadcastToInstance(mapID, instanceID, packet.Opcode, packet.Payload.Bytes(), nil)
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
// clients. Static GOs with no live state object are skipped — Go has no
// runtime record for them.
func (s *Server) applyGameObjectFlag(mapID, instanceID uint32, guid uint64, flag uint32, set bool) {
	if s == nil || guid == 0 {
		return
	}
	var flags uint32
	found := false
	s.objectsMu.Lock()
	if st := s.gameObjectStateLocked(mapID, instanceID, guid); st != nil {
		if set {
			st.Flags |= flag
		} else {
			st.Flags &^= flag
		}
		flags = st.Flags
		found = true
	}
	s.objectsMu.Unlock()
	if found {
		s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{gameObjectFlags: flags})
	}
}

func (s *Server) despawnDynamicGameObjectInInstance(mapID, instanceID uint32, guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	s.objectsMu.Lock()
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
	s.broadcastGameObjectDespawnInInstance(mapID, instanceID, guid)
	// Eluna GAMEOBJECT_EVENT_ON_REMOVE (event 13), fired from
	// GameObject::RemoveFromWorld (GameObject.cpp:244): (event, gameobject).
	// Only dynamic despawns have a live server-side object — static template
	// GOs are per-client update packets, so their map removal has no Go
	// counterpart.
	s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnRemove)
}
