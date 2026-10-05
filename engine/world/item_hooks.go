package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// sessionLuaItem builds the Lua Item object for the Eluna ITEM_EVENT_*
// hooks from the character's item_instance row — the Go model of Eluna's
// Push(Item*) via ElunaTemplate. It reuses scripting.NewItemObject
// (GUID/Entry/Count + GetCount), the surface built for the guild event 9
// leg; the rest of Eluna's Item method surface has no live-item model
// behind it and stays unbridged. rawGUID is the item_instance low part:
// client GUIDs carry the 0x4000 high bits (see handleUseItem and
// questgiverStartsQuest), so both forms are probed. A nil return means no
// handler can see the item, so the event does not fire — C++ always has a
// live Item*.
func (s *session) sessionLuaItem(ctx context.Context, rawGUID uint64) *scripting.Object {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || rawGUID == 0 {
		return nil
	}
	var entry, count uint32
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
		"SELECT itemEntry, `count` FROM item_instance WHERE guid = ? OR guid = ? LIMIT 1",
		rawGUID, rawGUID|(uint64(0x4000)<<48)).Scan(&entry, &count); err != nil || entry == 0 {
		return nil
	}
	return scripting.NewItemObject(rawGUID|(uint64(0x4000)<<48), entry, count)
}

// fireItemEvent dispatches an Eluna ITEM_EVENT_* hook (RegisterItemEvent)
// for the item entry. The caller passes the C++ argument order without the
// leading event — TriggerItemEvent prepends it, like TriggerGameObjectEvent.
// It returns true when a handler cancelled the action, following the
// engine's luaCancelled convention (a Lua false return cancels), the same
// convention the chat and addon-message hooks use. Only ITEM_EVENT_ON_USE
// (2) reads the return; the C++ quest-accept call site (Player.cpp:15128)
// discards ScriptMgr::OnQuestAccept's result, so Go discards it too.
func (s *session) fireItemEvent(ctx context.Context, entry uint32, event int, args ...any) bool {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	values, err := s.server.Features.Scripts.TriggerItemEvent(ctx, entry, event, args...)
	if err != nil {
		s.debug("lua item event failed", "event", event, "error", err)
	}
	return luaCancelled(values)
}

// fireItemUseHook dispatches ITEM_EVENT_ON_USE (2) as
// (event, player, item, target), mirroring Eluna::OnItemUse (ItemHooks.cpp)
// via ScriptMgr::OnItemUse (SpellHandler.cpp:176). The target is nil: Go's
// spell-target decode has no Lua object surface for it, the same documented
// delta as the gameobject OnUse leg. Eluna::OnUse also fires the item
// gossip hello hook (GOSSIP_EVENT_ON_HELLO) when the item still exists;
// that arm is fireItemGossipHelloHook, fired from handleUseItem right after
// this one in Eluna::OnUse's OnItemUse-then-OnItemGossip order. Returns true
// when a handler returned false, i.e. the cast must not start.
func (s *session) fireItemUseHook(ctx context.Context, rawItemGUID uint64) bool {
	item := s.sessionLuaItem(ctx, rawItemGUID)
	if item == nil {
		return false
	}
	return s.fireItemEvent(ctx, objectUint32OrZero(item, "Entry"), scripting.ItemEventOnUse, s.luaPlayer(), item, nil)
}

// fireItemGossipEvent dispatches an Eluna item-gossip hook
// (RegisterItemGossipEvent) for the item entry. The caller passes the C++
// argument order without the leading event — TriggerItemGossipEvent
// prepends it, like TriggerItemEvent. The gossip hello arm (1) reads the
// return: a Lua false stops the spell cast (GossipHooks.cpp). The select
// arm (2) discards it: C++ CallAllFunctions has no return.
func (s *session) fireItemGossipEvent(ctx context.Context, entry uint32, event int, args ...any) bool {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	values, err := s.server.Features.Scripts.TriggerItemGossipEvent(ctx, entry, event, args...)
	if err != nil {
		s.debug("lua item gossip event failed", "event", event, "error", err)
	}
	return luaCancelled(values)
}

// fireItemGossipHelloHook dispatches GOSSIP_EVENT_ON_HELLO (1) for the
// item_gossip bindings as (event, player, item), mirroring
// Eluna::OnItemGossip (GossipHooks.cpp:81), which Eluna::OnUse calls after
// OnItemUse whenever the item still exists in the player's inventory — the
// sessionLuaItem re-fetch is that existence gate. A Lua false return stops
// the spell cast, so it returns true when cancelled, the same convention
// as fireItemUseHook. The pending gossip menu is cleared first, but only
// when a handler is actually registered: C++'s ClearMenus() sits behind
// START_HOOK_WITH_RETVAL's early return for unbound entries.
func (s *session) fireItemGossipHelloHook(ctx context.Context, rawItemGUID uint64) bool {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	item := s.sessionLuaItem(ctx, rawItemGUID)
	if item == nil {
		return false
	}
	entry := objectUint32OrZero(item, "Entry")
	if !s.server.Features.Scripts.HasHook(scripting.ItemGossipKind(entry), scripting.GossipEventOnHello) {
		return false
	}
	s.gossip = nil
	return s.fireItemGossipEvent(ctx, entry, scripting.GossipEventOnHello, s.luaPlayer(), item)
}

// fireItemGossipSelectHook dispatches GOSSIP_EVENT_ON_SELECT (2) for the
// item_gossip bindings as (event, player, item, sender, action[, code]),
// mirroring Eluna::HandleGossipSelectOption's item arm (GossipHooks.cpp:90)
// via ScriptMgr::OnGossipSelect[Code] (ScriptMgr.cpp:1655-1677), fired from
// the item-GUID arm of HandleGossipSelectOptionOpcode (MiscHandler.cpp).
// C++ fires it with CallAllFunctions (no cancel semantics), so the return
// is discarded. The pending menu is cleared before firing, gated on
// registered bindings exactly like the hello arm. code is appended only
// when the menu option is coded — C++ pushes nil for an empty code, but Go
// passes the decoded string, the same convention as the creature gossip
// select arm in handleGossipSelectOption.
func (s *session) fireItemGossipSelectHook(ctx context.Context, entry uint32, menuItem gossipMenuItem, code string, item *scripting.Object) {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil || item == nil {
		return
	}
	if !s.server.Features.Scripts.HasHook(scripting.ItemGossipKind(entry), scripting.GossipEventOnSelect) {
		return
	}
	s.gossip = nil
	args := []any{s.luaPlayer(), item, menuItem.Sender, menuItem.Action}
	if menuItem.Coded {
		args = append(args, code)
	}
	s.fireItemGossipEvent(ctx, entry, scripting.GossipEventOnSelect, args...)
}

// fireItemQuestHook dispatches ITEM_EVENT_ON_QUEST_ACCEPT (3) as
// (event, player, item, quest), mirroring Eluna::OnQuestAccept
// (ItemHooks.cpp) via ScriptMgr::OnQuestAccept, fired only from the
// TYPEID_ITEM/TYPEID_CONTAINER arm of Player::AddQuestAndCheckCompletion
// (Player.cpp:15128) after AddQuest and the auto-complete check — so
// non-item giver GUIDs are skipped here. The C++ call site discards
// ScriptMgr::OnQuestAccept's result, so Go discards it too.
func (s *session) fireItemQuestHook(ctx context.Context, giverGUID uint64, questID uint32) {
	if s == nil || s.server == nil || uint16(giverGUID>>48) != 0x4000 {
		return
	}
	item := s.sessionLuaItem(ctx, giverGUID&0x0000FFFFFFFFFFFF)
	if item == nil {
		return
	}
	s.fireItemEvent(ctx, objectUint32OrZero(item, "Entry"), scripting.ItemEventOnQuestAccept, s.luaPlayer(), item, s.luaQuest(ctx, questID))
}

// Documented no-bridge arms of the Eluna item event family (ItemHooks.cpp):
//
// (1) DUMMY_EFFECT — ScriptMgr/Eluna::OnDummyEffect fires from
// Spell::EffectDummy's item-target arm (SpellEffects.cpp:819). Go's spell
// engine has no SPELL_EFFECT_DUMMY dispatch at all and no item-target
// concept (spells.go carries item casts only as cast-item GUIDs through
// handleUseItem), so there is no fire site — the same no-bridge family as
// the gameobject leg's event 3.
//
// (4) EXPIRE — ScriptMgr::OnItemExpire fires from Item::UpdateDuration
// (Item.cpp:314) when the duration countdown crosses zero. Go has no
// item-duration countdown tick anywhere in the engine (item_template
// Duration is loaded and sent to the client, never decremented), so there
// is no fire site.
//
// (5) REMOVE — ScriptMgr::OnItemRemove fires from Player::DestroyItem
// (Player.cpp:12662), the single funnel for every item removal in C++.
// Go has no central DestroyItem: removals are scattered SQL across
// handleDestroyItem, destroyPlayerItemCount, stack merges, ammo
// consumption, quest turn-in consumption, item refunds, trade, auction,
// mail, guild bank and character cleanup — some server-side with no
// session. Bridging them one by one is a dedicated removal-funnel leg,
// not this unit.
