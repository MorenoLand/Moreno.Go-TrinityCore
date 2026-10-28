package world

import "context"

// nextLevelXPFor is the PLAYER_NEXT_LEVEL_XP field: ObjectMgr::GetXPForLevel
// reads the player_xp_for_level table (Player.cpp:2727/2857), and Go's
// xpForLevel mirrors it with the static curve as fallback. Rest arms must
// use this, not the raw curve, or a customized table splits the rest
// accumulation/clamp from the level-up thresholds.
func nextLevelXPFor(server *Server, ctx context.Context, level uint8) uint32 {
	if server != nil {
		return server.xpForLevel(ctx, uint32(level))
	}
	return playerNextLevelXP(level)
}

func playerNextLevelXP(level uint8) uint32 {
	if int(level) >= len(xpCurve) {
		return 0
	}
	return xpCurve[level]
}
