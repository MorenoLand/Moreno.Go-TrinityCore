package protocol

func BuildWeather(weatherID uint32, intensity float32, abrupt bool) []byte {
	packet := NewBuffer(9)
	packet.WriteU32(weatherID)
	packet.WriteF32(intensity)
	if abrupt {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	return packet.Bytes()
}
