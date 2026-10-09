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

// useDoorOrButton mirrors GameObject::UseDoorOrButton for doors
// (GameObject.cpp): the door flips between active and ready with the
// custom-anim broadcast, and an opened door schedules its auto-close from
// the template timing. Shared by handleGameObjectUse and the DoLootRelease
// door arm (LootHandler.cpp:281-285) — locked doors opened with openlock are
// re-used on release instead of being marked looted.
func (s *Server) useDoorOrButton(mapID, instanceID uint32, guid uint64) {
	if s == nil {
		return
	}
	current := GameObjectStateReady
	if st := s.gameObjectState(mapID, instanceID, guid); st != nil {
		current = st.State
	}
	newState := GameObjectStateActive
	if current == GameObjectStateActive {
		newState = GameObjectStateReady
	}
	s.setGameObjectStateInInstance(mapID, instanceID, guid, newState)
	s.broadcastGameObjectCustomAnimInInstance(mapID, instanceID, guid, 0)
	if newState == GameObjectStateActive {
		s.scheduleGameObjectResetInInstance(mapID, instanceID, guid, 10*time.Second)
	}
}

func (s *Server) scheduleGameObjectResetInInstance(mapID, instanceID uint32, guid uint64, delay time.Duration) {
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
		// Routed through setGameObjectStateInInstance so the Eluna
		// GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED hook fires, matching C++
		// where the door auto-close goes through GameObject::SetGoState.
		// The pointer-identity check preserves the original semantics: a
		// replaced state object is not reset.
		s.objectsMu.RLock()
		current := s.gameObjectStateLocked(mapID, instanceID, guid)
		s.objectsMu.RUnlock()
		if current == state {
			s.setGameObjectStateInInstance(mapID, instanceID, guid, GameObjectStateReady)
		}
		s.broadcastGameObjectResetStateInInstance(mapID, instanceID, guid)
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
	if state.OwnerGUID != 0 && !state.FishingHandled {
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
