package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// itemTemplateLiveFromQuery maps the item_template row into the exported
// scripting.ItemTemplateLive behind Lua Item objects (Eluna reads these
// through Item::GetTemplate()). Spell IDs are uint32 in C++; a negative
// DB sentinel maps to 0.
func itemTemplateLiveFromQuery(data itemQueryData) scripting.ItemTemplateLive {
	live := scripting.ItemTemplateLive{
		Name:           data.Name,
		Class:          data.Class,
		SubClass:       data.SubClass,
		DisplayInfoID:  data.DisplayInfoID,
		Quality:        data.Quality,
		ItemLevel:      data.ItemLevel,
		RequiredLevel:  data.RequiredLevel,
		BuyPrice:       data.BuyPrice,
		SellPrice:      data.SellPrice,
		InventoryType:  data.InventoryType,
		AllowableClass: data.AllowableClass,
		AllowableRace:  data.AllowableRace,
		Stackable:      data.Stackable,
		ContainerSlots: data.ContainerSlots,
		StatsCount:     data.StatsCount,
	}
	for index := range data.Spells {
		if id := data.Spells[index].ID; id > 0 {
			live.SpellIDs[index] = uint32(id)
		}
		live.SpellTriggers[index] = data.Spells[index].Trigger
	}
	return live
}

// itemTemplateLive loads the item_template row for the entry behind a Lua
// Item object. sessionLuaItem and the guild item-move leg both supply it.
func (s *session) itemTemplateLive(ctx context.Context, entry uint32) scripting.ItemLive {
	return scripting.ItemLive{LoadTemplate: func() (scripting.ItemTemplateLive, bool) {
		data, err := s.loadItemQueryData(ctx, entry)
		if err != nil {
			return scripting.ItemTemplateLive{}, false
		}
		return itemTemplateLiveFromQuery(data), true
	}}
}

// sessionLuaItem builds the Lua Item object for the Eluna ITEM_EVENT_*
// hooks from the character's item_instance row — the Go model of Eluna's
// Push(Item*) via ElunaTemplate. It reuses scripting.NewItemObject
// (GUID/Entry/Count + the template-read surface in globals.go); the
// instance-bound and mutating Item surface has no live-item model behind
// it and stays unbridged. rawGUID is the item_instance low part: client
// GUIDs carry the 0x4000 high bits (see handleUseItem and
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
	return scripting.NewItemObject(rawGUID|(uint64(0x4000)<<48), entry, count, s.itemTemplateLive(ctx, entry))
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
// item_gossip bindings as (event, player, item, sender, action, code),
// mirroring Eluna::HandleGossipSelectOption's item arm (GossipHooks.cpp:90)
// via ScriptMgr::OnGossipSelect[Code] (ScriptMgr.cpp:1655-1677), fired from
// the item-GUID arm of HandleGossipSelectOptionOpcode (MiscHandler.cpp).
// C++ always pushes the 5th argument — nil when the code is empty — so Go
// always appends it too (pushValue maps Go nil to Lua nil), including for
// non-coded options which C++ routes through the same single fire site.
// C++ fires it with CallAllFunctions (no cancel semantics), so the return
// is discarded. The pending menu is cleared before firing, gated on
// registered bindings exactly like the hello arm.
func (s *session) fireItemGossipSelectHook(ctx context.Context, entry uint32, menuItem gossipMenuItem, code string, item *scripting.Object) {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil || item == nil {
		return
	}
	if !s.server.Features.Scripts.HasHook(scripting.ItemGossipKind(entry), scripting.GossipEventOnSelect) {
		return
	}
	s.gossip = nil
	var codeArg any = code
	if code == "" {
		codeArg = nil
	}
	args := []any{s.luaPlayer(), item, menuItem.Sender, menuItem.Action, codeArg}
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
// (4) EXPIRE — BRIDGED 2026-10-12: ScriptMgr::OnItemExpire fires from
// Item::UpdateDuration (Item.cpp:314) when the duration countdown crosses
// zero. updateItemDurations (below, on the world tick) decrements
// item_instance.duration in whole seconds (the C++ unit — Player.cpp:1078
// passes time_t diffs) and fires ITEM_EVENT_ON_EXPIRE (4) as
// (event, player, itemEntry) before destroying the item, matching the C++
// hook-then-DestroyItem order with the return discarded. The realtime-only
// offline arm is updateOfflineRealtimeItemDurations (Player.cpp:17989).
// Remaining delta: item creation paths still insert duration=0 (C++
// Item::CreateItem seeds proto->Duration at once); the login normalization
// (C++ Item::LoadFromDB parity) repairs it at next login.
//
// (5) REMOVE — ScriptMgr::OnItemRemove fires from Player::DestroyItem
// (Player.cpp:12662), the single funnel for every item removal in C++.
// Go has no central DestroyItem: removals are scattered SQL across
// handleDestroyItem, destroyPlayerItemCount, stack merges, ammo
// consumption, quest turn-in consumption, item refunds, trade, auction,
// mail, guild bank and character cleanup — some server-side with no
// session. Bridging them one by one is a dedicated removal-funnel leg,
// not this unit.

// updateItemDurations mirrors Player::UpdateItemDuration as driven by
// Player::Update (Player.cpp:1078, 13752-13766): while online, every carried
// item's remaining duration ticks down and items crossing zero expire —
// ScriptMgr::OnItemExpire fires first, then the item is destroyed whole
// (Item::UpdateDuration, Item.cpp:305-320). C++ ticks in whole seconds
// (GameTime::GetGameTime() is time_t), so this sweep runs at most once per
// second on the world tick (runWorldTick); the realtime-only offline arm is
// updateOfflineRealtimeItemDurations (Player.cpp:17989). Durations are
// seconds in both item_template.Duration and item_instance.duration.
func (s *Server) updateItemDurations(ctx context.Context) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	now := time.Now()
	if !s.lastItemDurationTick.IsZero() && now.Sub(s.lastItemDurationTick) < time.Second {
		return
	}
	elapsed := int64(1)
	if !s.lastItemDurationTick.IsZero() {
		elapsed = int64(now.Sub(s.lastItemDurationTick).Seconds())
	}
	s.lastItemDurationTick = now
	if elapsed <= 0 {
		return
	}
	s.sessionsMu.RLock()
	var loaded []*session
	for sess := range s.sessions {
		if sess != nil && sess.playerLoaded && sess.player != nil {
			loaded = append(loaded, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range loaded {
		sess.tickItemDurations(ctx, elapsed)
	}
}

// tickItemDurations decrements one online player's item durations by elapsed
// seconds (the C++ realtimeonly=false arm — every duration item ticks while
// online) and expires the ones that crossed zero. The Eluna
// ITEM_EVENT_ON_EXPIRE (4) hook fires as (event, player, itemEntry):
// Eluna::OnExpire pushes the template entry, not the item (ItemHooks.cpp:119-124),
// and Item::UpdateDuration discards the hook's return before destroying the
// item, so Go discards it too. Destruction mirrors handleDestroyItem's
// full-removal arm (Player::DestroyItem via Player::RemoveItem, Player.cpp:12662).
func (s *session) tickItemDurations(ctx context.Context, elapsed int64) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	db := s.server.CharactersStore.DB
	// Candidates expiring this tick must be identified BEFORE the decrement:
	// after it, a pre-existing 0-duration item is indistinguishable from one
	// that just crossed zero (both read <= 0).
	var expiring []int64
	rows, err := db.QueryContext(ctx, `SELECT ii.guid FROM item_instance AS ii
		JOIN character_inventory AS ci ON ci.item = ii.guid
		WHERE ci.guid = ? AND ii.duration > 0 AND ii.duration <= ?`, s.playerGUID, elapsed)
	if err != nil {
		return
	}
	for rows.Next() {
		var guid int64
		if rows.Scan(&guid) == nil && guid != 0 {
			expiring = append(expiring, guid)
		}
	}
	rows.Close()
	if _, err := db.ExecContext(ctx, `UPDATE item_instance SET duration = duration - ?
		WHERE guid IN (SELECT item FROM character_inventory WHERE guid = ?) AND duration > 0`, elapsed, s.playerGUID); err != nil {
		return
	}
	expired := false
	for _, itemGUID := range expiring {
		var itemEntry, count, bag, slot int64
		if err := db.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.count, ci.bag, ci.slot
			FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, itemGUID).Scan(&itemEntry, &count, &bag, &slot); err != nil || itemEntry == 0 {
			continue
		}
		entry := uint32(itemEntry)
		s.fireItemEvent(ctx, entry, scripting.ItemEventOnExpire, s.luaPlayer(), entry)
		s.adjustQuestItemCount(ctx, entry, uint32(count), false)
		if bag == 0 && uint8(slot) < equipSlotEnd {
			s.removeItemSetItem(ctx, entry)
			s.applyItemEquipSpells(ctx, itemGUID, entry, false)
			s.removeItemDependentAuras(ctx, uint64(itemGUID))
			s.applyEnchantEquipSpells(ctx, itemGUID, entry, false)
		}
		_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND item = ?", s.playerGUID, itemGUID)
		_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
		s.applyItemObtainSpells(ctx, entry, 0, false)
		s.despawnItem(uint64(itemGUID))
		expired = true
	}
	if expired {
		s.syncEquipmentCache(ctx)
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
}
