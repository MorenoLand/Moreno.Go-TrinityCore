package world

// Live instance guid store: InstanceScript::_objectGuids plus
// InstanceScript::GetGuidData / GetCreature
// (Instances/InstanceScript.cpp:108-120, 294-303). C++ populates the map
// from the instance script's OnCreatureCreate / OnGameObjectCreate —
// either the base AddObject leg (entry -> DATA type via the script's
// LoadObjectData table) or a derived script's own member latching
// (instance_icecrown_citadel.cpp:255-261 latches the blood princes into
// BloodCouncilGUIDs, read back by its GetGuidData override :789-795) —
// and erases a GUID on remove only when the stored GUID matches the
// leaving object. GetGuidData returns ObjectGuid::Empty for an unlatched
// type; GetCreature resolves that GUID on the map and returns nullptr
// when the GUID is empty or the creature is not on the map.
//
// Go has no ported instance scripts, so there is no OnCreatureCreate
// to populate the store from. Population is Lua-driven instead, through
// the creature:SetGuidData bridge below: a boss Lua file registers
// CREATURE_EVENT_ON_SPAWN (5) — fired by motionFor on first spawn and
// on respawn (creaturemotion.go:1204-1226), the Go equivalent of the
// C++ first-grid-activation JustAppeared — and latches its own GUID
// under its instance DATA_X constant, the exact Lua mirror of the
// C++ OnCreatureCreate entry-switch latching. The removal leg needs
// no Go equivalent for correctness: GetInstanceCreature resolves
// through the live motion map, so a stale GUID left by a despawned
// creature reads nil, exactly like C++ GetCreature on an absent GUID.
// A respawn re-latches via the same ON_SPAWN handler.
//
// The second population route — an instance Lua file registering
// RegisterMapEvent(mapID, 5, ...) for INSTANCE_EVENT_ON_CREATURE_CREATE,
// fired by triggerInstanceCreatureCreate (instance_hooks.go:320) —
// stays available for a future instance-script port; no instance Lua
// files exist yet, and the creature-side route covers the queued
// SendEncounterUnit non-me-unit ports.
func (s *Server) instanceGuidData(mapID, instanceID, dataID uint32) uint64 {
	if s == nil {
		return 0
	}
	s.instanceGuidMu.RLock()
	defer s.instanceGuidMu.RUnlock()
	if guids, ok := s.instanceGuids[instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}]; ok {
		return guids[dataID]
	}
	return 0
}

// setInstanceGuidData mirrors the AddObject create leg
// (InstanceScript.cpp:294-297): store guid under dataID for this
// instance. A zero guid clears the slot, matching the remove leg's
// erase for the caller that owns it.
func (s *Server) setInstanceGuidData(mapID, instanceID, dataID uint32, guid uint64) {
	if s == nil {
		return
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	s.instanceGuidMu.Lock()
	defer s.instanceGuidMu.Unlock()
	if guid == 0 {
		if guids, ok := s.instanceGuids[key]; ok {
			delete(guids, dataID)
		}
		return
	}
	guids, ok := s.instanceGuids[key]
	if !ok {
		guids = make(map[uint32]uint64)
		if s.instanceGuids == nil {
			s.instanceGuids = make(map[instanceAdmissionKey]map[uint32]uint64)
		}
		s.instanceGuids[key] = guids
	}
	guids[dataID] = guid
}

// instanceCreatureByGuidData mirrors InstanceScript::GetCreature
// (InstanceScript.cpp:117-120): resolve the stored GUID on the
// instance's live motion map; nil when unlatched or not on the map.
func (s *Server) instanceCreatureByGuidData(mapID, instanceID, dataID uint32) *creatureMotion {
	guid := s.instanceGuidData(mapID, instanceID, dataID)
	if guid == 0 {
		return nil
	}
	return s.findCreatureMotion(mapID, instanceID, guid)
}
