package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// groupLuaObject builds the Lua Group object for the Eluna GROUP_EVENT_*
// hooks, the Go model of Eluna::GroupHooks' Push(group): the protocol group
// GUID, the leader GUID and the member GUID list. Groups are in-memory
// (groupState), so unlike the guild surface there is no DB row to read —
// callers snapshot the object at the C++ hook's fire point (e.g. before the
// member list is mutated) and trigger after releasing groupsMu, since Lua
// handlers must never run under the group lock.
func groupLuaObject(g *groupState) *scripting.Object {
	if g == nil {
		return nil
	}
	members := make([]uint64, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, m.GUID)
	}
	return scripting.NewGroupObject(groupGUID(g.ID), g.LeaderGUID, members)
}

// RemoveMethod values for GROUP_EVENT_ON_MEMBER_REMOVE, the Eluna-visible
// RemoveMethod enum (SharedDefines.h:3640-3643). C++ Eluna::OnRemoveMember
// pushes only (group, guid, method); kicker/reason go to GroupScript only.
const (
	groupRemoveMethodDefault = 0
	groupRemoveMethodKick    = 1
	groupRemoveMethodLeave   = 2
)

// triggerGroupEvent dispatches an Eluna GROUP_EVENT_* hook
// (RegisterGroupEvent) as (event, group, ...args), mirroring
// Eluna::GroupHooks (GroupHooks.cpp) via ScriptMgr::OnGroup*
// (ScriptMgr.cpp:2336-2385). A nil group object means no handler can see the
// group, so the event does not fire — C++ always has a live Group*. These
// hooks never cancel: the C++ call sites use CallAllFunctions, not the Bool
// form.
func (srv *Server) triggerGroupEvent(event int, group *scripting.Object, args ...any) {
	if srv == nil || group == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	if _, err := srv.Features.Scripts.TriggerGroupEvent(context.Background(), event, append([]any{group}, args...)...); err != nil {
		srv.debug("lua group event failed", "event", event, "error", err)
	}
}
