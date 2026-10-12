package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// groupLuaObject builds the Lua Group object for the Eluna GROUP_EVENT_*
// hooks, the Go model of Eluna::GroupHooks' Push(group): the protocol group
// GUID plus the live roster (leader GUID, members, raid/BG shape) behind the
// GetId/GetGUID/GetLeaderGUID/GetMembers/GetMembersCount/IsLeader/IsFull/
// IsRaidGroup/IsBGGroup/IsMember/IsAssistant/SameSubGroup/
// HasFreeSlotSubGroup/GetMemberGUID/GetMemberGroup methods
// (scripting.NewGroupObject). Groups are in-memory (groupState), so callers
// snapshot the object at the C++ hook's fire point (e.g. before the member
// list is mutated) and trigger after releasing groupsMu, since Lua handlers
// must never run under the group lock.
func groupLuaObject(srv *Server, g *groupState) *scripting.Object {
	if g == nil {
		return nil
	}
	members := make([]scripting.GroupMemberLive, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, scripting.GroupMemberLive{GUID: m.GUID, Name: m.Name, SubGroup: m.SubGroup, Assistant: m.Flags&memberFlagAssistant != 0})
	}
	snap := scripting.GroupSnapshot{LeaderGUID: g.LeaderGUID, Members: members, IsRaid: g.IsRaid, IsBG: g.GroupType&groupTypeBattleground != 0}
	var players func() []*scripting.Object
	if srv != nil {
		players = srv.luaPlayers
	}
	return scripting.NewGroupObject(groupGUID(g.ID), scripting.GroupLive{
		Load:    func() (scripting.GroupSnapshot, bool) { return snap, true },
		Players: players,
	})
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
