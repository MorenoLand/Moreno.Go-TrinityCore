package world

import (
	"context"
)

// triggerBGEvent dispatches an Eluna BG_EVENT_* hook (RegisterBGEvent) as
// (event, bg, bgId, instanceId, ...args), mirroring Eluna::BattleGroundHooks
// (BattleGroundHooks.cpp). C++ pushes a BattleGround userdata; Go has no
// BattleGround object type, so the bg argument is nil. Go's non-arena
// battleground states are per-map, not per-instance, so instanceId is 0:
// the per-map state carries no instance id to report. Arena maps resolve
// through battlegroundTypeForMap too, but their states are per-instance
// with a real instance id, so arena legs do not fire through this helper
// yet. These hooks never cancel: the C++ call sites use CallAllFunctions,
// not the Bool form. Callers fire only after releasing the battleground
// registry lock, except the start/end legs, which fire under the per-map
// state lock like the C++ EndBattleground fire point; no Lua battleground
// API reaches that lock, so no handler can re-enter it.
func (srv *Server) triggerBGEvent(mapID uint32, event int, args ...any) {
	if srv == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	bgTypeID, _, isArena, ok := battlegroundTypeForMap(mapID)
	if !ok || isArena {
		return
	}
	hookArgs := append([]any{nil, bgTypeID, uint32(0)}, args...)
	if _, err := srv.Features.Scripts.TriggerBGEvent(context.Background(), event, hookArgs...); err != nil {
		srv.debug("lua bg event failed", "event", event, "error", err)
	}
}
