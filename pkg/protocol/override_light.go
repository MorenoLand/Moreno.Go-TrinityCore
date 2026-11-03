package protocol

// BuildOverrideLight mirrors WorldPackets::Misc::OverrideLight::Write
// (MiscPackets.cpp:159-166): AreaLightID, OverrideLightID,
// TransitionMilliseconds, all int32, in that wire order.
func BuildOverrideLight(areaLightID, overrideLightID, transitionMilliseconds uint32) []byte {
	packet := NewBuffer(12)
	packet.WriteI32(int32(areaLightID))
	packet.WriteI32(int32(overrideLightID))
	packet.WriteI32(int32(transitionMilliseconds))
	return packet.Bytes()
}
