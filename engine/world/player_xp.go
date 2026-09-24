package world

func playerNextLevelXP(level uint8) uint32 {
	if int(level) >= len(xpCurve) {
		return 0
	}
	return xpCurve[level]
}
