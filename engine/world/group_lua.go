package world

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// groupLuaMaxRaidSubGroups mirrors MAX_RAID_SUBGROUPS (Group.h:44); the
// world package otherwise uses the literal 8 at the subgroup call sites.
const groupLuaMaxRaidSubGroups = 8

// attachGroupMutations registers the Eluna mutating Group methods on a Lua
// Group object: AddMember/SetLeader/SendPacket/RemoveMember/Disband/
// ConvertToRaid/SetMembersGroup/SetTargetIcon (GroupMethods.h). Each method
// re-resolves the live groupState by ID, so a group that disbanded since the
// object's snapshot was taken degrades to the C++ no-op/false instead of
// mutating a stale copy. The methods run on the scripting goroutine, never
// under the group lock; the helpers below take the lock themselves.
func attachGroupMutations(srv *Server, obj *scripting.Object, groupID uint64) {
	if srv == nil || obj == nil || obj.Methods == nil || groupID == 0 {
		return
	}
	live := func() *groupState { return srv.findGroupByID(groupID) }
	groupLimit := func(g *groupState) int {
		if g.IsRaid {
			return maxRaidSize
		}
		return maxGroupSize
	}

	// AddMember: player->GetGroup() / !group->IsCreated() / group->IsFull()
	// reject with false; a pending invite is withdrawn
	// (Player::UninviteFromGroup); then the Group::AddMember body — the
	// first non-full subgroup for raids (Group.cpp:404), instance
	// revalidation, LFG leave, broadcast plus the Eluna OnGroupAddMember
	// event (Group.cpp:477). The BG-raid SetBattlegroundOrBattlefieldRaid
	// arm has no Go twin (Go keeps no per-player BG-group reference).
	obj.Methods["AddMember"] = func(ctx context.Context, args []any) ([]any, error) {
		playerObj, err := groupLuaObjectArg(args, 0, "player")
		if err != nil {
			return nil, err
		}
		playerGUID, ok := objectUint64Field(playerObj, "GUID")
		if !ok || playerGUID == 0 {
			return []any{false}, nil
		}
		sess := srv.findSessionByGUID(playerGUID)
		if sess == nil || sess.player == nil || sess.groupID != 0 {
			return []any{false}, nil
		}
		srv.groupsMu.Lock()
		g := srv.groups[groupID]
		if g == nil || len(g.Members) >= groupLimit(g) {
			srv.groupsMu.Unlock()
			return []any{false}, nil
		}
		if sess.pendingGroupLeader != 0 {
			sess.pendingGroupLeader = 0
		}
		subGroup := uint8(0)
		if g.IsRaid {
			found := false
			for sub := uint8(0); sub < groupLuaMaxRaidSubGroups; sub++ {
				if g.countInSubGroup(sub) < maxGroupSize {
					subGroup = sub
					found = true
					break
				}
			}
			if !found {
				srv.groupsMu.Unlock()
				return []any{false}, nil
			}
		}
		g.Members = append(g.Members, groupMember{GUID: playerGUID, Name: sess.player.Name, SubGroup: subGroup})
		refreshGroupMaxEnchantingLevel(srv, g)
		sess.groupID = g.ID
		srv.groupsMu.Unlock()

		// Group::AddMember (Group.cpp:451): re-adding recomputes instance
		// validity, like the CMSG_GROUP_ACCEPT path.
		sess.instanceValid = sess.checkInstanceValidity(ctx, g)
		if srv.Features != nil && srv.Features.LFG != nil &&
			srv.Features.LFG.Leave(playerGUID) {
			_ = sess.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: playerGUID, State: LFGStateNone})
		}
		srv.broadcastGroupList(g)
		srv.triggerGroupEvent(scripting.GroupEventOnMemberAdd, groupLuaObject(srv, g), playerGUID)
		return []any{true}, nil
	}

	// SetLeader: Group::ChangeLeader + SendUpdate; setGroupLeader carries
	// the member/online checks, the assistant-strip and the Eluna
	// OnGroupChangeLeader event.
	obj.Methods["SetLeader"] = func(_ context.Context, args []any) ([]any, error) {
		guid, err := groupLuaUint64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if g := live(); g != nil {
			srv.setGroupLeader(g, guid)
		}
		return nil, nil
	}

	// SendPacket: Group::BroadcastPacket(data, ignorePlayersInBg, -1,
	// ignoreGuid). The ignorePlayersInBg arm is vacuous in Go: the fan-out
	// only visits sessions whose groupID matches, which is exactly the C++
	// "player->GetGroup() != this" skip (a player in a BG sits in the BG
	// raid group, so their group differs). The -1 "except subgroup" arm has
	// no Go twin.
	obj.Methods["SendPacket"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("SendPacket requires a packet, ignorePlayersInBg and ignore")
		}
		packet, ok := args[0].(*scripting.Packet)
		if !ok || packet == nil {
			return nil, fmt.Errorf("SendPacket requires a WorldPacket")
		}
		if _, err := groupLuaBoolArg(args, 1); err != nil {
			return nil, err
		}
		ignore, err := groupLuaUint64Arg(args, 2)
		if err != nil {
			return nil, err
		}
		if g := live(); g != nil {
			srv.broadcastGroupPacket(g.ID, uint16(packet.Opcode), packet.Data, ignore)
		}
		return nil, nil
	}

	// RemoveMember: Group::RemoveMember (Group.cpp:564) — online members
	// ride the session detach path, offline members drop from the slot list;
	// the RemoveMethod reaches the Eluna event like the C++
	// sScriptMgr->OnGroupRemoveMember arm.
	obj.Methods["RemoveMember"] = func(ctx context.Context, args []any) ([]any, error) {
		guid, err := groupLuaUint64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		var method uint8
		if len(args) > 1 {
			m, err := groupLuaUint64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			method = uint8(m)
		}
		g := live()
		if g == nil {
			return []any{false}, nil
		}
		return []any{srv.luaRemoveGroupMember(ctx, g, guid, method)}, nil
	}

	// Disband: Group::Disband (Group.cpp:857).
	obj.Methods["Disband"] = func(ctx context.Context, _ []any) ([]any, error) {
		if g := live(); g != nil {
			srv.disbandGroup(ctx, g, false)
		}
		return nil, nil
	}

	// ConvertToRaid: Group::ConvertToRaid (Group.cpp:311).
	obj.Methods["ConvertToRaid"] = func(ctx context.Context, _ []any) ([]any, error) {
		if g := live(); g != nil {
			srv.convertGroupToRaid(ctx, g)
		}
		return nil, nil
	}

	// SetMembersGroup: the Eluna argerror on subGroup >= MAX_RAID_SUBGROUPS
	// plus the HasFreeSlotSubGroup gate, then Group::ChangeMembersGroup
	// (Group.cpp:1898) — raid groups only, member must exist, no-op when
	// already in the target subgroup.
	obj.Methods["SetMembersGroup"] = func(_ context.Context, args []any) ([]any, error) {
		guid, err := groupLuaUint64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		sub, err := groupLuaUint64Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if sub >= groupLuaMaxRaidSubGroups {
			return nil, fmt.Errorf("valid subGroup ID expected")
		}
		g := live()
		if g == nil {
			return nil, nil
		}
		srv.groupsMu.Lock()
		if !g.IsRaid || g.countInSubGroup(uint8(sub)) >= maxGroupSize {
			srv.groupsMu.Unlock()
			return nil, nil
		}
		moved := false
		for i := range g.Members {
			if g.Members[i].GUID == guid && g.Members[i].SubGroup != uint8(sub) {
				g.Members[i].SubGroup = uint8(sub)
				moved = true
				break
			}
		}
		srv.groupsMu.Unlock()
		if moved {
			srv.broadcastGroupList(g)
		}
		return nil, nil
	}

	// SetTargetIcon: the Eluna argerror on icon >= TARGETICONCOUNT, then
	// Group::SetTargetIcon (Group.cpp:1690).
	obj.Methods["SetTargetIcon"] = func(_ context.Context, args []any) ([]any, error) {
		icon, err := groupLuaUint64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if icon >= targetIconCount {
			return nil, fmt.Errorf("valid target icon expected")
		}
		target, err := groupLuaUint64Arg(args, 1)
		if err != nil {
			return nil, err
		}
		var setter uint64
		if len(args) > 2 {
			setter, err = groupLuaUint64Arg(args, 2)
			if err != nil {
				return nil, err
			}
		}
		if g := live(); g != nil {
			srv.setGroupTargetIcon(g.ID, uint8(icon), setter, target)
		}
		return nil, nil
	}
}

// groupLuaObjectArg extracts a Lua object argument (the CHECKOBJ<T> arm).
func groupLuaObjectArg(args []any, index int, what string) (*scripting.Object, error) {
	if index >= len(args) {
		return nil, fmt.Errorf("argument %d is required", index+1)
	}
	obj, ok := args[index].(*scripting.Object)
	if !ok || obj == nil {
		return nil, fmt.Errorf("argument %d must be a %s object", index+1, what)
	}
	return obj, nil
}

// groupLuaUint64Arg coerces a Lua unsigned argument: UInt64 userdata (what
// GetGUID-style pushes produce), plain Go integers, integral floats, or
// decimal strings.
func groupLuaUint64Arg(args []any, index int) (uint64, error) {
	if index >= len(args) {
		return 0, fmt.Errorf("argument %d is required", index+1)
	}
	switch value := args[index].(type) {
	case scripting.UInt64:
		return uint64(value), nil
	case uint64:
		return value, nil
	case uint32:
		return uint64(value), nil
	case uint8:
		return uint64(value), nil
	case int:
		if value >= 0 {
			return uint64(value), nil
		}
	case int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case float64:
		if value >= 0 && value <= math.MaxUint64 && value == math.Trunc(value) {
			return uint64(value), nil
		}
	case string:
		return strconv.ParseUint(value, 10, 64)
	}
	return 0, fmt.Errorf("argument %d must be uint64", index+1)
}

// groupLuaBoolArg coerces a Lua boolean argument (CHECKVAL<bool>).
func groupLuaBoolArg(args []any, index int) (bool, error) {
	if index >= len(args) {
		return false, fmt.Errorf("argument %d is required", index+1)
	}
	switch value := args[index].(type) {
	case bool:
		return value, nil
	case float64:
		return value != 0, nil
	case int:
		return value != 0, nil
	case int64:
		return value != 0, nil
	case uint64:
		return value != 0, nil
	}
	return false, fmt.Errorf("argument %d must be bool", index+1)
}

// broadcastGroupPacket mirrors Group::BroadcastPacket's ignoredPlayer arm
// (Group.cpp:1824): raw-packet fan-out to the group's sessions, skipping the
// ignored GUID. There is no subgroup-"except" twin in Go.
func (s *Server) broadcastGroupPacket(groupID uint64, opcode uint16, payload []byte, ignoreGUID uint64) {
	if groupID == 0 {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess.groupID != groupID || !sess.worldReady.Load() {
			continue
		}
		if ignoreGUID != 0 && sess.playerGUID == ignoreGUID {
			continue
		}
		_ = sess.write(opcode, payload, true)
	}
}

// setGroupTargetIcon mirrors Group::SetTargetIcon (Group.cpp:1690): the
// target GUID is cleared from any other slot first (each clear broadcasts
// its own MSG_RAID_TARGET_UPDATE), then the slot is set and broadcast, all
// with the C++ ignorePlayersInBg=true arm.
func (s *Server) setGroupTargetIcon(groupID uint64, icon uint8, setter, target uint64) {
	g := s.findGroupByID(groupID)
	if g == nil {
		return
	}
	s.groupsMu.Lock()
	var cleared []uint8
	if target != 0 {
		for i := range g.TargetIcons {
			if uint8(i) != icon && g.TargetIcons[i] == target {
				g.TargetIcons[i] = 0
				cleared = append(cleared, uint8(i))
			}
		}
	}
	g.TargetIcons[icon] = target
	s.groupsMu.Unlock()
	build := func(idx uint8, guid uint64) []byte {
		b := protocol.NewBuffer(18)
		b.WriteU8(0)
		b.WriteU64(setter)
		b.WriteU8(idx)
		b.WriteU64(guid)
		return b.Bytes()
	}
	for _, idx := range cleared {
		s.broadcastGroupPacket(groupID, uint16(protocol.OpcodeMSG_RAID_TARGET_UPDATE), build(idx, 0), 0)
	}
	s.broadcastGroupPacket(groupID, uint16(protocol.OpcodeMSG_RAID_TARGET_UPDATE), build(icon, target), 0)
}

// convertGroupToRaid flips a 5-man group to raid, mirroring
// Group::ConvertToRaid (Group.cpp:311): unconditional void — the raid flag
// and group type are set, the type is persisted (CHAR_UPD_GROUP_TYPE,
// Group.cpp:318-324; Go has no BG/BF groups so the persist is unconditional),
// and the roster is rebroadcast. The per-member
// UpdateVisibleGameobjectsOrSpellClicks sweep stays unbridged (parked with
// the quest-GO refresh item). The <2-member gate lives in the packet handler
// (GroupHandler.cpp:583-584), not here.
func (s *Server) convertGroupToRaid(ctx context.Context, g *groupState) {
	if g == nil {
		return
	}
	s.groupsMu.Lock()
	g.IsRaid = true
	g.GroupType |= groupTypeRaid
	dbID := g.DBID
	groupType := g.GroupType
	s.groupsMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil && dbID != 0 {
		_, _ = s.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_UPD_GROUP_TYPE"), groupType, dbID)
	}
	s.broadcastGroupList(g)
}

// luaRemoveGroupMember mirrors Group::RemoveMember (Group.cpp:564) for the
// Lua bridge: online members ride the session detach path, offline members
// are dropped from the slot list, and the RemoveMethod reaches the Eluna
// event like the C++ sScriptMgr->OnGroupRemoveMember arm.
func (s *Server) luaRemoveGroupMember(ctx context.Context, g *groupState, guid uint64, method uint8) bool {
	s.groupsMu.RLock()
	member := false
	for _, m := range g.Members {
		if m.GUID == guid {
			member = true
			break
		}
	}
	s.groupsMu.RUnlock()
	if !member {
		return false
	}
	// Group::RemoveMember (Group.cpp:744-749): the offline arm (player ==
	// nullptr) disbands and returns false; the online arm returns true even
	// when the removal disbands the group (:736-742).
	online := s.findSessionByGUID(guid) != nil
	s.removeGroupMemberByGUID(ctx, g, guid, method)
	return online
}
