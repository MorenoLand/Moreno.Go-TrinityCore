package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Encounter-frame types (InstanceScript.h:57-64): ENGAGE 0, DISENGAGE 1,
// UPDATE_PRIORITY 2, ADD_TIMER 3, ENABLE_OBJECTIVE 4, UPDATE_OBJECTIVE 5,
// DISABLE_OBJECTIVE 6, UNK7 7.
const (
	encounterFrameEngage           uint32 = 0
	encounterFrameDisengage        uint32 = 1
	encounterFrameUpdatePriority   uint32 = 2
	encounterFrameAddTimer         uint32 = 3
	encounterFrameEnableObjective  uint32 = 4
	encounterFrameUpdateObjective  uint32 = 5
	encounterFrameDisableObjective uint32 = 6
	encounterFrameUnk7             uint32 = 7
)

// sendEncounterUnit mirrors InstanceScript::SendEncounterUnit
// (Instances/InstanceScript.cpp:703-736): the SMSG_UPDATE_INSTANCE_ENCOUNTER_UNIT
// packet carries the frame type, then — per type — the packed unit GUID plus
// param1 (ENGAGE/DISENGAGE/UPDATE_PRIORITY, nil unit sends nothing),
// param1 alone (ADD_TIMER/ENABLE_OBJECTIVE/DISABLE_OBJECTIVE), or
// param1+param2 (UPDATE_OBJECTIVE); UNK7 and unknown types carry nothing
// past the type. Delivery is the instance's players only
// (instance->SendToPlayers), the updateInstanceWorldState session filter
// (battleground_wsg.go): authed, world-ready sessions on the same
// map/instance pair.
func (s *Server) sendEncounterUnit(mapID, instanceID uint32, unitGUID uint64, frameType uint32, param1, param2 uint8) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(15)
	buf.WriteU32(frameType)
	switch frameType {
	case encounterFrameEngage, encounterFrameDisengage, encounterFrameUpdatePriority:
		if unitGUID == 0 {
			return
		}
		buf.WritePackedGUID(unitGUID)
		buf.WriteU8(param1)
	case encounterFrameAddTimer, encounterFrameEnableObjective, encounterFrameDisableObjective:
		buf.WriteU8(param1)
	case encounterFrameUpdateObjective:
		buf.WriteU8(param1)
		buf.WriteU8(param2)
	case encounterFrameUnk7:
	default:
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil ||
			target.player.Map != mapID || target.player.InstanceID != instanceID {
			continue
		}
		_ = target.write(uint16(protocol.OpcodeSMSG_UPDATE_INSTANCE_ENCOUNTER_UNIT), buf.Bytes(), true)
	}
}
