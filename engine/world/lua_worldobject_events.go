package world

import (
	"context"
	"fmt"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaWorldObjectEventMethods builds the Eluna WorldObject timed-event
// trio (WorldObjectMethods.h:867-919): RegisterEvent(func, delay|
// {min,max}, repeats) returns the event id, RemoveEventById(id) aborts
// one event, RemoveEvents() aborts them all. The timers live in the
// scripting runtime's owner-tagged timer list (see CreateObjectTimer):
// the callback fires as (eventId, delay, repeats, object) like C++
// OnTimedEvent (ServerHooks.cpp:67-82), with the object re-resolved at
// fire time by resolve; when the owner no longer resolves the timer
// is dropped, matching Eluna destroying the object's event processor
// with the object. resolve returns (object, true) while the owner
// lives, (nil, false) once it is gone. RegisterEvent needs the raw
// Lua state to store the callback ref, so it reads it from the
// method-dispatch context (scripting.CallState); without it the call
// is a silent no-op.
func luaWorldObjectEventMethods(srv *Server, guid uint64, resolve func(ctx context.Context) (any, bool)) map[string]scripting.ObjectMethod {
	scripts := func() *scripting.Runtime {
		if srv == nil || srv.Features == nil {
			return nil
		}
		return srv.Features.Scripts
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["RegisterEvent"] = func(ctx context.Context, args []any) ([]any, error) {
		runtime := scripts()
		state, ok := scripting.CallState(ctx)
		if runtime == nil || !ok || state == nil {
			return nil, nil
		}
		id := runtime.CreateObjectTimer(state, 2, guid, resolve)
		return []any{id}, nil
	}
	methods["RemoveEventById"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		id, err := luaInt64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if runtime := scripts(); runtime != nil {
			runtime.RemoveObjectTimer(guid, int(id))
		}
		return nil, nil
	}
	methods["RemoveEvents"] = func(_ context.Context, _ []any) ([]any, error) {
		if runtime := scripts(); runtime != nil {
			runtime.RemoveObjectTimers(guid)
		}
		return nil, nil
	}
	return methods
}
