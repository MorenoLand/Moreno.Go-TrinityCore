package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// serverLuaGameObject builds the Lua GameObject object for the Eluna
// GAMEOBJECT_EVENT_* hooks, the Go model of Eluna::GameObjectHooks'
// Push(gameobject). Dynamic/instance spawns resolve from the object
// registry; static template GOs resolve from the gameobject row like
// loadLuaGameObject. A nil return means no handler can see the gameobject,
// so the event does not fire — C++ always has a live GameObject*.
func (srv *Server) serverLuaGameObject(ctx context.Context, guid uint64) *scripting.Object {
	if srv == nil || uint16(guid>>48) != 0xF110 {
		return nil
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var state luaGameObjectState
	var ok bool
	srv.objectsMu.RLock()
	if dyn, found := srv.dynamicGameObjects[guid]; found && dyn != nil {
		state = luaGameObjectState{GUID: dyn.GUID, LowGUID: dyn.LowGUID, Entry: dyn.Entry, DisplayID: dyn.DisplayID, Map: dyn.Map, InstanceID: dyn.InstanceID, X: dyn.X, Y: dyn.Y, Z: dyn.Z, Orientation: dyn.Orientation, GoState: uint32(dyn.State), LootState: 1, Type: uint32(dyn.Type), OwnerGUID: dyn.OwnerGUID}
		ok = true
	}
	if !ok {
		for _, states := range srv.instanceGameObjects {
			if dyn := states[guid]; dyn != nil {
				state = luaGameObjectState{GUID: dyn.GUID, LowGUID: dyn.LowGUID, Entry: dyn.Entry, DisplayID: dyn.DisplayID, Map: dyn.Map, InstanceID: dyn.InstanceID, X: dyn.X, Y: dyn.Y, Z: dyn.Z, Orientation: dyn.Orientation, GoState: uint32(dyn.State), LootState: 1, Type: uint32(dyn.Type), OwnerGUID: dyn.OwnerGUID}
				ok = true
				break
			}
		}
	}
	srv.objectsMu.RUnlock()
	if !ok {
		state, ok = srv.loadLuaGameObject(ctx, low, entry)
	}
	if !ok {
		return nil
	}
	if state.Name == "" && srv.WorldStore != nil && srv.WorldStore.DB != nil {
		var name string
		if err := srv.WorldStore.DB.QueryRowContext(ctx, "SELECT name FROM gameobject_template WHERE entry = ? LIMIT 1", state.Entry).Scan(&name); err == nil {
			state.Name = name
		}
	}
	return srv.luaGameObjectObject(state)
}

// triggerGameObjectEvent dispatches an Eluna GAMEOBJECT_EVENT_* hook
// (RegisterGameObjectEvent) as (event, go, ...args), mirroring
// Eluna::GameObjectHooks (GameObjectHooks.cpp). It returns true when a
// handler cancelled the action: the Go bridge follows the engine's
// luaCancelled convention (a Lua false return cancels), the same convention
// the chat and addon-message hooks use. Only the cancel-returning hooks
// (ON_USE) read the return; every other C++ call site discards it.
func (srv *Server) triggerGameObjectEvent(ctx context.Context, guid uint64, event int, args ...any) bool {
	if srv == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return false
	}
	goObj := srv.serverLuaGameObject(ctx, guid)
	if goObj == nil {
		return false
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	values, err := srv.Features.Scripts.TriggerGameObjectEvent(ctx, entry, event, append([]any{goObj}, args...)...)
	if err != nil {
		srv.debug("lua gameobject event failed", "event", event, "error", err)
	}
	return luaCancelled(values)
}

// fireGameObjectEvent is the session-level variant of triggerGameObjectEvent.
// The gameobject resolves through the session's luaGameObject, so the
// player-map gate applies: the GO must be on the player's map, matching the
// interact-range context these hooks fire in.
func (s *session) fireGameObjectEvent(ctx context.Context, guid uint64, event int, args ...any) bool {
	if s == nil || s.server == nil || uint16(guid>>48) != 0xF110 {
		return false
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	goObj := s.luaGameObject(ctx, guid)
	if goObj == nil {
		// Dynamic/instance spawns have no gameobject-table row; resolve
		// from the object registry instead.
		goObj = s.server.serverLuaGameObject(ctx, guid)
	}
	if goObj == nil {
		return false
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	values, err := s.server.Features.Scripts.TriggerGameObjectEvent(ctx, entry, event, append([]any{goObj}, args...)...)
	if err != nil {
		s.debug("lua gameobject event failed", "event", event, "error", err)
	}
	return luaCancelled(values)
}

// fireGameObjectQuestHook dispatches an Eluna RegisterGameObjectEvent quest
// hook for the gameobject questgiver: ON_QUEST_ACCEPT (4), ON_QUEST_REWARD
// (5) and ON_DIALOG_STATUS (6). C++ fires these only from the
// TYPEID_GAMEOBJECT arms (Player.cpp:15156, QuestHandler.cpp:332,
// Player.cpp:16281); creature and item givers route to the CreatureEvents
// and ItemQuestEvents families, so non-gameobject GUIDs are skipped here.
// Unlike the combat hooks, the quest hooks push the player first — Eluna
// argument order is (event, player, go, quest[, opt]) per
// GameObjectHooks.cpp. TriggerGameObjectEvent prepends the event, so the
// caller passes (player, go, quest) without it. The return values of these
// hooks are discarded by every C++ call site, so Go discards them too.
func (s *session) fireGameObjectQuestHook(ctx context.Context, giverGUID uint64, event int, extra ...any) {
	if s == nil || s.server == nil || uint16(giverGUID>>48) != 0xF110 {
		return
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	goObj := s.luaGameObject(ctx, giverGUID)
	if goObj == nil {
		return
	}
	entry := uint32((giverGUID >> 24) & 0x00FFFFFF)
	args := make([]any, 0, len(extra)+2)
	args = append(args, s.luaPlayer(), goObj)
	args = append(args, extra...)
	_, _ = s.server.Features.Scripts.TriggerGameObjectEvent(ctx, entry, event, args...)
}

// fireGameObjectGossipHelloHook dispatches GOSSIP_EVENT_ON_HELLO (1) for the
// gameobject_gossip bindings as (event, player, go), mirroring
// Eluna::OnGossipHello(Player*, GameObject*) (GossipHooks.cpp:27-34), which
// fires at the head of GameObject::Use (GameObject.cpp:1500-1503):
// `if (sEluna->OnGossipHello(playerUser, this)) return;`. Eluna returns
// CallAllFunctionsBool(..., true) (HookHelpers.h:92-117): false iff at
// least one handler returned boolean false, true otherwise. So a Lua
// handler returning false lets the native use arms PROCEED; returning true
// or nothing suppresses them (handled). The caller passes the C++ argument
// order without the leading event — TriggerGameObjectGossipEvent prepends
// it, like TriggerItemGossipEvent. The pending gossip menu is cleared first,
// but only when a handler is actually registered: C++'s ClearMenus() sits
// behind START_HOOK_WITH_RETVAL's early return for unbound entries.
// Returns true (skip native use) when a hook is registered and no handler
// returned false — the uniform Eluna gossip polarity, same as the creature
// hello hook in gossip.go. (An older revision had this inverted — Lua false
// suppressing native — and mis-cited GossipHooks.cpp:32; corrected
// 2026-10-19 against the in-tree Eluna sources.)
//
// gameobject_gossip ON_SELECT (2) is WIRED, not a no-bridge:
// fireGameObjectGossipSelectHook below fires from the guid.IsGameObject()
// arm of the select handler (gossip.go:718, MiscHandler.cpp:138-198), and GO
// gossip menus do open in Go (gameobjects.go:530-550 goober arm via
// prepareGameObjectGossip + sendGossipMenu). Polarity is Eluna's: a Lua
// false return lets the native select arms proceed; true/nothing vetoes
// them (see fireGameObjectGossipSelectHook). (An older revision of this
// comment claimed the arm unreachable; corrected 2026-10-19.)
//
// Documented no-bridge (player_gossip family): ON_HELLO does not exist in
// Eluna — GossipHooks.cpp has no player hello arm and Hooks.h:327 documents
// the hello object as Creature/GameObject/Item only. ON_SELECT (2) fires
// from the guid.IsPlayer() arm of HandleGossipSelectOptionOpcode via
// ScriptMgr::OnGossipSelect (ScriptMgr.cpp:2178), keyed by menuId; Go never
// opens player gossip menus, so that arm can never be reached either.
func (s *session) fireGameObjectGossipHelloHook(ctx context.Context, guid uint64) bool {
	if s == nil || s.server == nil || uint16(guid>>48) != 0xF110 {
		return false
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	goObj := s.luaGameObject(ctx, guid)
	if goObj == nil {
		// Dynamic/instance spawns have no gameobject-table row; resolve
		// from the object registry instead, like fireGameObjectEvent.
		goObj = s.server.serverLuaGameObject(ctx, guid)
	}
	if goObj == nil {
		return false
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	if !s.server.Features.Scripts.HasHook(scripting.GameObjectGossipKind(entry), scripting.GossipEventOnHello) {
		return false
	}
	s.gossip = nil
	values, err := s.server.Features.Scripts.TriggerGameObjectGossipEvent(ctx, entry, scripting.GossipEventOnHello, s.luaPlayer(), goObj)
	if err != nil {
		s.debug("lua gameobject gossip hello failed", "entry", entry, "error", err)
	}
	return !luaCancelled(values)
}

// fireGameObjectGossipSelectHook mirrors Eluna::OnGossipSelect[Code]
// (LuaEngine/GossipHooks.cpp:36-61): ClearMenus, then (player, go, sender,
// action, code, listID) on the GOSSIP_EVENT_ON_SELECT bindings keyed by GO
// entry. Eluna passes no list index; listID is a Go extension appended after
// code (code is "" when the option has no code box, so listID is always the
// last argument). listID is the CMSG_GOSSIP_SELECT_OPTION menu index — the
// C++ gossipListId PlayerTalkClass resolves via GetGossipOptionSender/Action
// — so select arms that index tables by gossipListId (icecrown teleport) port
// directly, while action-branching arms read sender/action as before. menuID
// is intentionally not forwarded: the post-hook stale-menu gate already
// enforces it, and no ported arm branches on it.
// HandleGossipSelectOptionOpcode runs the native arms only when the Eluna
// hook did NOT veto (MiscHandler.cpp:198-204, 225:
// `if (!sEluna->OnGossipSelect[Code](...))` then AI + native arms), and the
// Eluna hook returns false iff a handler returned boolean false
// (CallAllFunctionsBool(..., true), HookHelpers.h:92-117). So a Lua false
// return lets the native select arms PROCEED; true/nothing vetoes them.
// Returns true (skip native) when a hook is registered and no handler
// returned false. (An older revision had this inverted; corrected
// 2026-10-19 against the in-tree Eluna sources.)
func (s *session) fireGameObjectGossipSelectHook(ctx context.Context, guid uint64, sender, action uint32, code string, listID uint32) bool {
	if s == nil || s.server == nil || uint16(guid>>48) != 0xF110 {
		return false
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	goObj := s.luaGameObject(ctx, guid)
	if goObj == nil {
		// Dynamic/instance spawns have no gameobject-table row; resolve
		// from the object registry instead, like the hello hook.
		goObj = s.server.serverLuaGameObject(ctx, guid)
	}
	if goObj == nil {
		return false
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	if !s.server.Features.Scripts.HasHook(scripting.GameObjectGossipKind(entry), scripting.GossipEventOnSelect) {
		return false
	}
	s.gossip = nil
	args := []any{s.luaPlayer(), goObj, sender, action, code, listID}
	values, err := s.server.Features.Scripts.TriggerGameObjectGossipEvent(ctx, entry, scripting.GossipEventOnSelect, args...)
	if err != nil {
		s.debug("lua gameobject gossip select failed", "entry", entry, "error", err)
	}
	return !luaCancelled(values)
}
