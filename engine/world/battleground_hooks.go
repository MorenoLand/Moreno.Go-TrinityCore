package world

import (
	"context"
)

// triggerBGEvent dispatches an Eluna BG_EVENT_* hook (RegisterBGEvent) as
// (event, bg, bgId, instanceId, ...args), mirroring Eluna::BattleGroundHooks
// (BattleGroundHooks.cpp). C++ pushes a BattleGround userdata; Go has no
// BattleGround object type, so the bg argument is nil. Go's non-arena
// battleground states are per-map, not per-instance, so instanceId is 0:
// the per-map state carries no instance id to report. Arena legs fire
// through triggerArenaBGEvent instead: arena states are per-instance and
// carry a real instance id plus the per-map type id (NA/BE/RL/DS/RV),
// not battlegroundTypeForMap's shared queue type 4. These hooks never
// cancel: the C++ call sites use CallAllFunctions, not the Bool form.
// Callers fire only after releasing the battleground registry lock,
// except the start/end legs, which fire under the per-map state lock
// like the C++ EndBattleground fire point; no Lua battleground API
// reaches that lock, so no handler can re-enter it.
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

// triggerArenaBGEvent is triggerBGEvent for arena instances: the caller
// passes the state's own per-map type id and real instance id. The bg
// argument is nil like the non-arena helper (no BattleGround userdata
// type in Go), and the winner argument on BGEventOnEnd is the TeamId
// mapping of the arena winner (Gold = Alliance 0, Green = Horde 1,
// draw = 2), matching the non-arena END convention.
func (srv *Server) triggerArenaBGEvent(bgTypeID, instanceID uint32, event int, args ...any) {
	if srv == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	hookArgs := append([]any{nil, bgTypeID, instanceID}, args...)
	if _, err := srv.Features.Scripts.TriggerBGEvent(context.Background(), event, hookArgs...); err != nil {
		srv.debug("lua bg event failed", "event", event, "error", err)
	}
}
