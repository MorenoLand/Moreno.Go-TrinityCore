package world

import (
	"time"

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
	s.objectsMu.Lock()
	if object := s.gameObjectStateLocked(mapID, instanceID, guid); object != nil {
		object.State = state
	}
	s.objectsMu.Unlock()
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
		s.objectsMu.Lock()
		if current := s.gameObjectStateLocked(mapID, instanceID, guid); current == state {
			current.State = GameObjectStateReady
		}
		s.objectsMu.Unlock()
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
}
