package scripting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Shopify/go-lua"
)

const (
	PlayerEventFirstLogin        = 30
	PlayerEventCharacterCreate   = 1 // OnCreate fires with the live Player*; Go has no player object at create time, so the engine never fires this event
	PlayerEventCharacterDelete   = 2
	PlayerEventLogin             = 3
	PlayerEventLogout            = 4
	PlayerEventSpellCast         = 5 // OnSpellCast fires from finishSpellCast (client/item casts) and castSpellDirectWithOverrides (triggered casts); the spell object is luaSpellObject (lua_spell.go)
	PlayerEventDuelRequest       = 9
	PlayerEventDuelStart         = 10
	PlayerEventDuelEnd           = 11
	PlayerEventKillPlayer        = 6
	PlayerEventKillCreature      = 7
	PlayerEventKilledByCreature  = 8
	PlayerEventGiveXP            = 12
	PlayerEventLevelChange       = 13
	PlayerEventMoneyChange       = 14 // OnMoneyChanged fires from Player::ModifyMoney (Player.cpp:22826); Go fires it from session modifyPlayerMoney (quest_reward_grant.go), which every ModifyMoney-equivalent money write routes through; direct SetMoney-style writes (GM set, SetCoinage, values-table write) fire nothing, like C++
	PlayerEventReputationChange  = 15
	PlayerEventTalentChange      = 16 // OnTalentsChange fires from Player::SetFreeTalentPoints; the binding fires it, internal learn/reset/level-up recomputes do not
	PlayerEventTalentReset       = 17 // OnTalentsReset fires from Player::ResetTalents; Go has no talent system, so the engine never fires this event
	PlayerEventChat              = 18
	PlayerEventWhisper           = 19
	PlayerEventGroupChat         = 20
	PlayerEventGuildChat         = 21
	PlayerEventChannelChat       = 22
	PlayerEventEmote             = 23
	PlayerEventTextEmote         = 24
	PlayerEventSave              = 25
	PlayerEventBindToInstance    = 26 // OnBindToInstance fires from Player::BindToInstance (Player.cpp:19121, ScriptMgr.cpp:2162); Go fires it from the instance-lock-response bind write (engine/world/group.go), the one bind site modeled as BindToInstance; worldport instance-save creation does not bind the player and fires nothing
	PlayerEventUpdateZone        = 27
	PlayerEventMapChange         = 28
	PlayerEventEquip             = 29
	PlayerEventCanUseItem        = 31
	PlayerEventLootItem          = 32
	PlayerEventEnterCombat       = 33 // OnPlayerEnterCombat fires from CombatManager::NotifyAICombat per new combat reference; Go fires it from markPlayerInCombat (combat.go) on the false->true UNIT_FLAG_IN_COMBAT transition at every combat-enter site, the first-ref twin (no ref model)
	PlayerEventLeaveCombat       = 34 // OnPlayerLeaveCombat fires from CombatManager reference teardown (CombatManager.cpp:88-337); Go fires it from markPlayerOutOfCombat (combat.go) on the true->false UNIT_FLAG_IN_COMBAT transition at every combat-leave site, the last-ref twin (no ref model)
	PlayerEventRepop             = 35
	PlayerEventResurrect         = 36
	PlayerEventLootMoney         = 37
	PlayerEventQuestAbandon      = 38
	PlayerEventLearnTalents      = 39 // OnLearnTalents fires from Player::LearnTalent; Go has no talent system, so the engine never fires this event
	PlayerEventQuestStatusChange = 40 // OnQuestStatusChange fires from Player::AddQuest/RewardQuest/SetQuestStatus/quest-abandon/EffectQuestClear (ScriptMgr.cpp:2194); Hooks.h marks 40 UNUSED and ScriptMgr::OnQuestStatusChange is PlayerScript-only (no Eluna call), so Go event 40 is a custom extension surfacing that hook to Lua as (event, player, questId), void
	PlayerEventCommand           = 42
	MapEventOnPlayerEnter        = 21
	MapEventOnPlayerLeave        = 22
)

// Eluna GuildEvents contract for RegisterGuildEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h GuildEvents): 1-11 are
// all fired by Eluna::GuildHooks; only the events the engine fires carry Go
// constants.
const (
	GuildEventOnAddMember     = 1
	GuildEventOnRemoveMember  = 2
	GuildEventOnMotdChange    = 3
	GuildEventOnInfoChange    = 4
	GuildEventOnCreate        = 5
	GuildEventOnDisband       = 6
	GuildEventOnMoneyWithdraw = 7
	GuildEventOnMoneyDeposit  = 8
	GuildEventOnItemMove      = 9
	GuildEventOnEvent         = 10
	GuildEventOnBankEvent     = 11
)

// Eluna GroupEvents contract for RegisterGroupEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h GroupEvents):
// GROUP_EVENT_ON_CREATE (6) has no fire site in this fork —
// Eluna::GroupHooks::OnCreate is never called from ScriptMgr or Group — so
// it is documented but never fired by the engine.
const (
	GroupEventOnMemberAdd    = 1
	GroupEventOnMemberInvite = 2
	GroupEventOnMemberRemove = 3
	GroupEventOnLeaderChange = 4
	GroupEventOnDisband      = 5
	GroupEventOnCreate       = 6
)

// Eluna VehicleEvents contract for RegisterVehicleEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h VehicleEvents): 3 is
// UNUSED in C++ and carries no Go constant. The engine fires all five live
// events; the vehicle argument is the base creature object (Go has no
// Vehicle userdata type — see triggerVehicleEvent in engine/world).
const (
	VehicleEventOnInstall          = 1
	VehicleEventOnUninstall        = 2
	VehicleEventOnInstallAccessory = 4
	VehicleEventOnAddPassenger     = 5
	VehicleEventOnRemovePassenger  = 6
)

// Eluna GameObjectEvents contract for RegisterGameObjectEvent. Numbering
// is the TrinityCore LuaEngine numbering (LuaEngine/Hooks.h
// GameObjectEvents): 11 is UNUSED in C++ and carries no Go constant; only
// the events the engine fires carry Go constants.
const (
	GameObjectEventOnAIUpdate      = 1
	GameObjectEventOnSpawn         = 2
	GameObjectEventOnDummyEffect   = 3
	GameObjectEventOnQuestAccept   = 4
	GameObjectEventOnQuestReward   = 5
	GameObjectEventOnDialogStatus  = 6
	GameObjectEventOnDestroyed     = 7
	GameObjectEventOnDamaged       = 8
	GameObjectEventOnLootState     = 9
	GameObjectEventOnGOStateChange = 10
	GameObjectEventOnAdd           = 12
	GameObjectEventOnRemove        = 13
	GameObjectEventOnUse           = 14
)

// Eluna ItemEvents contract for RegisterItemEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h ItemEvents). The
// engine fires ON_USE (2), ON_QUEST_ACCEPT (3) and ON_EXPIRE (4);
// ON_DUMMY_EFFECT (1) and ON_REMOVE (5) carry constants but have no Go fire
// sites (documented in engine/world/item_hooks.go).
const (
	ItemEventOnDummyEffect = 1
	ItemEventOnUse         = 2
	ItemEventOnQuestAccept = 3
	ItemEventOnExpire      = 4
	ItemEventOnRemove      = 5
)

// Eluna GossipEvents contract shared by RegisterCreatureGossipEvent,
// RegisterGameObjectGossipEvent, RegisterItemGossipEvent and
// RegisterPlayerGossipEvent. Numbering is the TrinityCore LuaEngine
// numbering (LuaEngine/Hooks.h GossipEvents). The engine fires the item
// gossip arms (ON_HELLO, ON_SELECT) via TriggerItemGossipEvent; the
// creature arms fire inline in engine/world/gossip.go; the gameobject hello
// arm fires via TriggerGameObjectGossipEvent from handleGameObjectUse; the
// gameobject select arm fires via TriggerGameObjectGossipEvent from the
// gameobject branch of handleGossipSelectOption (gameobject_hooks.go), and
// the player select arm fires via TriggerPlayerGossipEvent from the player
// branch — both keyed like their Eluna bindings
// (HandleGossipSelectOptionOpcode, MiscHandler.cpp:138-227).
const (
	GossipEventOnHello  = 1
	GossipEventOnSelect = 2
)

// Eluna InstanceEvents contract for RegisterMapEvent/RegisterInstanceEvent.
// Numbering is the TrinityCore LuaEngine numbering (LuaEngine/Hooks.h
// InstanceEvents). RegisterMapEvent binds a map ID (Go kind "map:<mapID>");
// RegisterInstanceEvent binds an instance ID (Go kind "instance:<id>") —
// C++ fires both binding maps at every fire site
// (LuaEngine/InstanceHooks.cpp START_HOOK). The engine fires ON_UPDATE (3),
// ON_PLAYER_ENTER (4), ON_CREATURE_CREATE (5), ON_GAMEOBJECT_CREATE (6) and
// ON_CHECK_ENCOUNTER_IN_PROGRESS (7); ON_INITIALIZE (1) is #ifndef
// TRINITY-dead in C++ on this fork and ON_LOAD (2) fires from the
// worldport entry path (engine/world/instance_hooks.go fireInstanceLoad),
// once per (map, instance) per server run, before ON_PLAYER_ENTER.
// ON_SET_DATA (8) is the Go-only extension above, fired from the
// InstanceData:SetData write path.
const (
	InstanceEventOnInitialize             = 1
	InstanceEventOnLoad                   = 2
	InstanceEventOnUpdate                 = 3
	InstanceEventOnPlayerEnter            = 4
	InstanceEventOnCreatureCreate         = 5
	InstanceEventOnGameObjectCreate       = 6
	InstanceEventOnCheckEncounterProgress = 7
	// InstanceEventOnSetData is a Go-only extension: the fork's Eluna
	// InstanceEvents enum ends at 7 (LuaEngine/Hooks.h
	// INSTANCE_EVENT_COUNT), and C++ InstanceScript::SetData has no
	// Eluna hook — the C++ instance script IS the SetData consumer.
	// Go has no C++ instance script (the instance script is Lua), so
	// the InstanceData:SetData write path (engine/world/
	// instance_hooks.go) fires this event with (event,
	// instance_data, map, type, data), letting the Lua instance
	// script run the SetData switch (e.g. Eye of Eternity
	// DATA_VORTEX_HANDLING -> VortexHandling).
	InstanceEventOnSetData = 8
	// InstanceEventOnUnitDeath is a Go-only extension on the same
	// free-slot precedent: C++ InstanceScript::OnUnitDeath
	// (ZoneScript.h) has no Eluna hook, but the C++ instance script
	// IS the OnUnitDeath consumer (e.g. Drak'Tharon Keep forwards
	// crystal-handler deaths to Novos). Go has no C++ instance
	// script, so the creature death path (engine/world/kill.go
	// clearCreatureDeathState, the Unit::setDeathState twin, plus the
	// session onCreatureKilled funnel arm) fires this event with
	// (event, instance_data, map, unit), letting the Lua instance
	// script run the OnUnitDeath switch. Fires for creature deaths
	// only; player deaths in instances take a separate session path
	// with no fire yet.
	InstanceEventOnUnitDeath = 9
)

// Eluna ServerEvents contract for RegisterServerEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h ServerEvents); only
// the events the engine fires carry Go constants.
const (
	ServerEventAddonMessage = 30
	// ServerEventLuaStateClose is ELUNA_EVENT_ON_LUA_STATE_CLOSE
	// (LuaEngine/Hooks.h:123 = 16, args (event)): fires just before the
	// Lua state shuts down (on shutdown and restart). The Go engine fires
	// it at the end of Server.Stop, after WORLD_EVENT_ON_SHUTDOWN (15),
	// matching the C++ teardown order (World::ShutdownServ, then Eluna
	// destruction -> CloseLua -> OnLuaStateClose, LuaEngine.cpp:202).
	ServerEventLuaStateClose = 16
	// ServerEventLuaStateOpen is ELUNA_EVENT_ON_LUA_STATE_OPEN
	// (LuaEngine/Hooks.h:153 = 33, args (event)): fires after all scripts
	// are loaded. The Go engine fires it at the end of Runtime.Load,
	// matching Eluna::LoadScripts (LuaEngine.cpp:520).
	ServerEventLuaStateOpen = 33
	// ServerEventAreaTrigger is the Eluna TRIGGER_EVENT_ON_TRIGGER
	// (LuaEngine/Hooks.h:135 = 24, args (event, player, triggerId)):
	// ServerHooks.cpp:115-125 fires it from
	// WorldSession::HandleAreaTriggerOpcode (MiscHandler.cpp:678) after
	// the radius validation, and a boolean true return vetoes the rest
	// of the handler (CallAllFunctionsBool).
	ServerEventAreaTrigger = 24
	// ServerEventPacketReceive is SERVER_EVENT_ON_PACKET_RECEIVE
	// (LuaEngine/Hooks.h:108 = 5, args (event, packet, player) - player only
	// if accessible): fires on every received packet before the opcode
	// handler. Each hook's first return is a boolean (false blocks the
	// handler) and the second return may be a Packet userdata that replaces
	// the packet the handler sees (LuaEngine/PacketHooks.cpp:93-115,
	// CallOneFunction(n, 2, 2)).
	ServerEventPacketReceive = 5
	// ServerEventPacketSend is SERVER_EVENT_ON_PACKET_SEND
	// (LuaEngine/Hooks.h:110 = 7, args (event, packet, player) - player only
	// if accessible): fires on every outgoing packet. Each hook's first
	// return is a boolean (false drops the packet); Lua receives a copy and
	// cannot alter the outgoing packet (LuaEngine/PacketHooks.cpp:42-80,
	// CallOneFunction(n, 2, 1)).
	ServerEventPacketSend = 7
	// ServerEventOpenStateChange is WORLD_EVENT_ON_OPEN_STATE_CHANGE
	// (LuaEngine/Hooks.h:129 = 8, args (event, open)): World::SetClosed
	// inverts the value for scripters (World.cpp:208), so "closed on"
	// pushes open=false and "closed off" pushes open=true.
	ServerEventOpenStateChange = 8
	// ServerEventConfigLoad is WORLD_EVENT_ON_CONFIG_LOAD
	// (LuaEngine/Hooks.h:114 = 9, args (event, reload)): C++ fires
	// OnConfigLoad(false) from World.cpp:2208 after Eluna scripts run,
	// before OnStartup (Main.cpp:336), and OnConfigLoad(true) on config
	// reload (World.cpp:1533). The Go engine fires only the initial
	// arm, immediately before the startup fire below; Go has no config
	// reload, so the reload arm stays out.
	ServerEventConfigLoad = 9
	// ServerEventShutdownInit is WORLD_EVENT_ON_SHUTDOWN_INIT
	// (LuaEngine/Hooks.h:116 = 11, args (event, code, mask)):
	// ServerHooks.cpp:282-288 fires it from World::ShutdownServ
	// (World.cpp:3022) after the initial countdown message, with the
	// ShutdownExitCode (0 shutdown, 2 restart) and ShutdownMask
	// (World.h:54-58: 1 restart, 2 idle, 4 force).
	ServerEventShutdownInit = 11
	// ServerEventShutdownCancel is WORLD_EVENT_ON_SHUTDOWN_CANCEL
	// (LuaEngine/Hooks.h:117 = 12, args (event)): ServerHooks.cpp:290-295
	// fires it from World::ShutdownCancel (World.cpp:3069) after the
	// cancel broadcast, only when a timer actually existed.
	ServerEventShutdownCancel = 12
	// ServerEventMapUpdate is MAP_EVENT_ON_UPDATE
	// (LuaEngine/Hooks.h:132 = 23, args (event, map, diff)):
	// ServerHooks.cpp:354-360 fires it from ScriptMgr::OnMapUpdate
	// (ScriptMgr.cpp:1560), called from Map::Update (Map.cpp:907) on
	// every map update. The Go engine fires it from the world tick
	// for each map carrying at least one session; a player-less Go
	// map has no materialized update at all, so it never fires (the
	// same scope rule as updateInstanceScriptTicks).
	ServerEventMapUpdate = 23
	// ServerEventGameEventStart is GAME_EVENT_START
	// (LuaEngine/Hooks.h:155 = 34, args (event, gameeventid)):
	// GameEventMgr::StartEvent fires it via Eluna::OnGameEventStart
	// (GameEventMgr.cpp:152/179, ServerHooks.cpp:90-95) once the event
	// is active. The Go engine fires it from the world tick when a
	// GAMEEVENT_NORMAL event enters the cached active set (see
	// game_event_hooks.go); world-event states stay inactive, matching
	// the idle-GameEventMgr stance in conditions.go.
	ServerEventGameEventStart = 34
	// ServerEventGameEventStop is GAME_EVENT_STOP
	// (LuaEngine/Hooks.h:156 = 35, args (event, gameeventid)):
	// GameEventMgr::StopEvent fires it via Eluna::OnGameEventStop
	// (GameEventMgr.cpp:229, ServerHooks.cpp:97-101) once the event is
	// no longer active. Go fires it from the same tick diff, after the
	// starts, mirroring GameEventMgr::Update's activate-then-deactivate
	// order (GameEventMgr.cpp:1143-1156).
	ServerEventGameEventStop = 35
	// ServerEventWeatherChange is WEATHER_EVENT_ON_CHANGE
	// (LuaEngine/Hooks.h:137 = 25, args (event, zoneId, state, grade)):
	// Weather::UpdateWeather fires it via ScriptMgr::OnWeatherChange
	// only when players in the zone received the packet (Weather.cpp),
	// with state from GetWeatherState and grade the clamped intensity.
	ServerEventWeatherChange = 25
	// ServerEventAuctionAdd/Remove/Successful/Expire are the Eluna
	// AUCTION_EVENT_ON_* hooks (LuaEngine/Hooks.h:141-144 = 26-29,
	// args (event, auctionId, owner, item, expireTime, buyout,
	// startBid, currentBid, bidderGUIDLow)): ServerHooks.cpp
	// OnAdd/OnRemove/OnSuccessful/OnExpire push them from
	// ScriptMgr::OnAuctionAdd/Remove/Successful/Expire
	// (AuctionHouseMgr.cpp). All four share the C++ prologue gate:
	// the owner must be online (FindPlayer) and the item must resolve
	// in the auction item map (GetAItem), else nothing fires.
	ServerEventAuctionAdd        = 26
	ServerEventAuctionRemove     = 27
	ServerEventAuctionSuccessful = 28
	ServerEventAuctionExpire     = 29
	// ServerEventDeleteCreature / ServerEventDeleteGameObject are the
	// Eluna WORLD_EVENT_ON_DELETE_CREATURE/GAMEOBJECT hooks
	// (LuaEngine/Hooks.h:149-150 = 31/32, args (event, creature) /
	// (event, gameobject)): Map::AddObjectToRemoveList (Map.cpp:3545-3553)
	// fires them when an object is queued for deletion, before
	// CleanupsBeforeDelete — not on RemoveFromWorld (that is creature
	// event 37 / GO event 13 territory). Go has no deferred remove list,
	// so the fires land at the permanent-deletion point of each leg.
	ServerEventDeleteCreature   = 31
	ServerEventDeleteGameObject = 32
)

// Eluna BGEvents contract for RegisterBGEvent. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/Hooks.h:334-337). All four
// carry the "Needs to be added to TC" comment there: no ScriptMgr call
// site invokes OnBGStart/OnBGEnd/OnBGCreate/OnBGDestroy in C++, so the
// events never fire upstream. The Go engine fires them at its own
// battleground lifecycle legs (world/triggerBGEvent) so registered
// handlers are no longer dead; only the events the engine fires carry
// Go constants.
const (
	// BGEventOnStart is BG_EVENT_ON_START (args (event, bg, bgId,
	// instanceId)): fires when a match starts.
	BGEventOnStart = 1
	// BGEventOnEnd is BG_EVENT_ON_END (args (event, bg, bgId,
	// instanceId, winner)): fires when a match ends; winner is the
	// TeamId index (0 Alliance, 1 Horde, 2 neutral/draw).
	BGEventOnEnd = 2
	// BGEventOnCreate is BG_EVENT_ON_CREATE (args (event, bg, bgId,
	// instanceId)): fires when a battleground state materializes.
	BGEventOnCreate = 3
	// BGEventOnPreDestroy is BG_EVENT_ON_PRE_DESTROY (args (event, bg,
	// bgId, instanceId)): fires when a battleground state is removed.
	BGEventOnPreDestroy = 4
)

// Eluna CreatureEvents contract for RegisterCreatureEvent /
// RegisterUniqueCreatureEvent. The world engine fires these through
// TriggerCreatureEvent; scripts use the raw event numbers. Numbering is the
// TrinityCore LuaEngine numbering (LuaEngine/GlobalMethods.h CreatureEvents,
// hooks fired in LuaEngine/CreatureHooks.cpp): 16-18, 25, 29, 32-33
// are UNUSED in C++ and carry no Go constant. 28 is likewise unused
// in C++ Eluna; Go assigns it to CreatureEventOnMovementInform below,
// a Go-side extension on the free slot (the DoAction-38 precedent),
// matching the event number every in-tree "no ev28" note cites. 11 is likewise unused by
// C++ Eluna (CreatureHooks has no heal hook); Go uses it for
// CreatureEventOnHealReceived, a Go-side extension fired from the
// Unit::DealHeal AI-hook arm, not a C++ Eluna event.
const (
	CreatureEventOnEnterCombat             = 1
	CreatureEventOnLeaveCombat             = 2
	CreatureEventOnTargetDied              = 3
	CreatureEventOnDied                    = 4
	CreatureEventOnSpawn                   = 5
	CreatureEventOnReachWP                 = 6
	CreatureEventOnAIUpdate                = 7
	CreatureEventOnReceiveEmote            = 8
	CreatureEventOnDamageTaken             = 9
	CreatureEventOnPreCombat               = 10
	CreatureEventOnHealReceived            = 11
	CreatureEventOnOwnerAttacked           = 12
	CreatureEventOnOwnerAttackedAt         = 13
	CreatureEventOnHitBySpell              = 14
	CreatureEventOnSpellHitTarget          = 15
	CreatureEventOnJustSummonedCreature    = 19
	CreatureEventOnSummonedCreatureDespawn = 20
	CreatureEventOnSummonedCreatureDied    = 21
	CreatureEventOnSummoned                = 22
	CreatureEventOnReset                   = 23
	CreatureEventOnReachHome               = 24
	CreatureEventOnCorpseRemoved           = 26
	CreatureEventOnMoveInLOS               = 27
	// CreatureEventOnMovementInform is the Go-side MovementInform hook
	// (event 28, args (event, creature, type, id)): C++ motion-master
	// generators call AI->MovementInform(type, id) on finalize, and
	// Eluna turns WAYPOINT_MOTION_TYPE into ON_REACH_WP (6) while the
	// other types reach this hook — POINT (8, MovePoint id),
	// EFFECT (16, MoveJump id), FOLLOW (14, target GUID counter).
	// CHASE (5) has no Go finalize arm (Go's chase re-paths instead
	// of finalizing) and no in-tree consumer needs it.
	CreatureEventOnMovementInform = 28
	CreatureEventOnDummyEffect    = 30
	CreatureEventOnQuestAccept    = 31
	CreatureEventOnQuestReward    = 34
	CreatureEventOnDialogStatus   = 35
	CreatureEventOnAdd            = 36
	CreatureEventOnRemove         = 37
	// CreatureEventOnDoAction is a Go-side extension (no C++ Eluna hook):
	// CreatureAI::DoAction dispatches through the AI virtual table, which
	// Lua-ported AIs have no bearer for, so creature:DoAction(action) is
	// the transport — it fires (event, creature, action) on the entry
	// then unique bindings, the void-hook twin of the other creature
	// events (C++ DoAction returns void, so returns are discarded). 38
	// is unused in the TrinityCore LuaEngine CreatureEvents numbering.
	CreatureEventOnDoAction = 38
	// CreatureEventOnSpellClick is a Go-side extension (no C++ Eluna hook):
	// CreatureAI::OnSpellClick dispatches through the AI virtual table, which
	// Lua-ported AIs have no bearer for, so the spell-click funnel fires
	// (event, creature, clicker, handled) on the entry then unique bindings,
	// the void-hook twin of the other creature events (C++ OnSpellClick
	// returns void, so returns are discarded). 39 is unused in the
	// TrinityCore LuaEngine CreatureEvents numbering.
	CreatureEventOnSpellClick = 39
	// CreatureEventOnSetGUID is a Go-side extension (no C++ Eluna hook):
	// CreatureAI::SetGUID dispatches through the AI virtual table, which
	// Lua-ported AIs have no bearer <redacted>, so creature:SetGUID(guid, id) is
	// the transport — it fires (event, creature, guid, id) on the entry
	// then unique bindings, the void-hook twin of the other creature
	// events (C++ SetGUID returns void, so returns are discarded). The
	// guid is the low half (GetGUIDLow convention): packed GUIDs lose
	// bits through Lua numbers, so senders pass the counter half and
	// receivers resolve it with GetCreatureByGUID. 40 is the next free
	// slot after the DoAction-38 / SpellClick-39 precedents.
	CreatureEventOnSetGUID = 40
)

// AuraEvents contract for RegisterAuraEvent(spellID, event, fn). There is
// no C++ Eluna RegisterAuraEvent: this is a Go-side extension mirroring
// Aura::CallScriptEffectPeriodicHandlers (SpellAuras.cpp:2341-2361), the
// hook AuraEffect::PeriodicTick runs before its switch
// (SpellAuraEffects.cpp:868-872). A handler returning boolean true is the
// AuraScript PreventDefaultAction() arm: the default tick is skipped. The
// hook fires for every periodic aura type, not just PERIODIC_DUMMY (226),
// because C++ runs it ahead of the switch for all of them; handlers
// registered for a spellID see its ticks regardless of which effect index
// the C++ registration bound, so multi-effect auras must filter on the
// aura object when the C++ script bound a single effect.
// OnApply/OnRemove mirror AuraEffect::HandleEffect's
// CallScriptEffectApplyHandlers arms (SpellAuras.cpp:2341-2361 twin):
// they fire once per fresh application / actual removal, on both the
// player and creature apply/remove funnels, with args (event, aura,
// target) like the periodic hook. They are notification-only: a handler
// returning true does NOT prevent the apply/remove — the aura record is
// already registered (apply) or erased (remove) when the hook fires,
// and Go performs the default effect handling inline at several points
// rather than in one skippable step, so there is no veto to model.
// The remove hook's aura object reads the removed record's last values
// (a snapshot: GetCaster/GetAmount/GetTickNumber still resolve); writes
// to it land on the detached record and are inert. Re-casts that merge
// or refresh an existing aura do not re-fire OnApply.
// OnChangeAmount mirrors the AURA_EFFECT_HANDLE_CHANGE_AMOUNT arm of
// AuraEffect::ChangeAmount (SpellAuraEffects.cpp:662-683), fired by
// Aura::ModStackAmount/SetStackAmount (SpellAuras.cpp:1008-1030) when a
// merge re-cast raises the aura's stack count. A re-cast that only
// refreshes duration (already at max stacks, or a non-stackable aura)
// does not fire it — C++ ChangeAmount returns early when the amount is
// unchanged. Notification-only like OnApply; handlers read the new stack
// count via aura:GetStackAmount().
// OnDispel mirrors Aura::CallScriptAfterDispel (Unit.cpp:3955): it fires
// once per successful dispel application, after the charge/stack
// decrement (ModCharges/ModStackAmount, Unit.cpp:3949-3952), with args
// (event, aura, target, dispeller) — the dispeller is
// DispelInfo::GetDispeller(). It fires whether or not the aura is fully
// removed, like C++; a charge/stack aura that survives the decrement
// still fires it. Notification-only like OnApply. The hook fires from
// the SPELL_EFFECT_DISPEL funnel (player, pet, and owned-creature
// casters), the SPELL_EFFECT_DISPEL_MECHANIC funnel, and the
// wild-creature dispel funnels; the aura object reads a snapshot
// captured before the decrement, so GetCaster still resolves after a
// full removal. Steal never fires it (RemoveAurasDueToSpellBySteal,
// Unit.cpp:3969, has no dispel-hook call).
// OnBeforeDispel mirrors Aura::CallScriptDispel (Unit.cpp:3947): it fires
// once per successful dispel application, before the charge/stack
// decrement, with the same args (event, aura, target, dispeller) and the
// same snapshot aura object as OnDispel. A handler returning boolean
// true is DispelInfo::SetRemovedCharges(0): the decrement is skipped
// and the aura keeps its charges, like C++; the AfterDispel hook still
// fires afterwards. It fires only from the SPELL_EFFECT_DISPEL funnels
// (session and wild-creature casters) — EffectDispelMechanic removes
// via Unit::RemoveAura (SpellEffects.cpp:4761), which calls no dispel
// hook, so the mechanic funnels never fire it.
// OnProc mirrors AuraScript::OnEffectProc (AuraEffectProcFn,
// SpellAuras.cpp:2192-2212 via Aura::TriggerProcOnEvent): it fires once
// per eligible proc effect, after the proc gates (flags, chance roll,
// charge/cooldown consumption) and before that effect's default leg
// (the DBC trigger-spell cast, the scriptAuraProcCast dummy arm, or the
// putricide/vampiric special cases). Args are (event, aura, target,
// actor, procTarget, damage): target is the aura holder (the
// periodic-hook convention), actor is the proc actor as a unit object
// (nil when the event carries no actor GUID), procTarget is the proc
// target as a unit object (the triggerTargetGUID — the damage victim
// on the done side, the attacker on the taken side), damage is the
// proc event's damage (0 for non-damage events — the handler gates on
// it, like the C++ damageInfo->GetDamage() early return). A handler
// returning boolean true is PreventDefaultAction(): the default leg
// for that effect is skipped, matching C++ where the script handler
// replaces the trigger cast. Only the session-side proc loop fires it
// today (player and pet mod-owner auras); creature auras have no proc
// evaluation site in Go, so creature-held proc auras stay out until
// one is built.
const (
	AuraEventOnPeriodic     = 1
	AuraEventOnApply        = 2
	AuraEventOnRemove       = 3
	AuraEventOnChangeAmount = 4
	AuraEventOnDispel       = 5
	AuraEventOnProc         = 6
	AuraEventOnBeforeDispel = 7
)

// SpellEvents contract for RegisterSpellEvent(spellID, event, fn). There
// is no C++ Eluna RegisterSpellEvent: this is a Go-side extension
// mirroring the AuraEvents one, over the SpellScript hooks in
// SpellScript.h. Only the hooks Go can fire exist here: OnCast mirrors
// Spell::CallScriptOnCastHandlers (Spell.cpp:3432, :7951), fired once
// per cast after target selection and before SendSpellGo, with args
// (event, spell); OnHit mirrors the CallScriptOnHitHandlers arm
// (Spell.cpp:2377, in Spell::TargetInfo::PreprocessTarget, before the
// effect loop deals damage in C++), fired once per unit hit target,
// with args (event, spell, hitTarget). All three twins fire
// pre-damage (boss_ai.go pass-1 position; spells.go client path and
// triggered funnel, moved 2026-11-09), so a handler sees the target
// alive on a lethal hit, like C++; the damage-rewrite legs ride the
// pre-damage spellScriptOnHitDamage table and the lethal-latch legs
// ride spellScriptOnHitLatch (spell_onhit.go) instead — GetHitDamage()
// still reads the pre-effect seed at OnHit, matching C++. AfterHit
// mirrors CallScriptAfterHitHandlers (Spell.cpp:
// 2659), fired once per unit hit target after the per-hit trigger
// consumer, with the same args. OnEffectHit mirrors
// CallScriptEffectHandlers in the SPELL_EFFECT_HANDLE_HIT mode
// (Spell.cpp:5168, called from Spell::HandleEffects; the HIT phase
// runs in _handle_immediate_phase, Spell.cpp:3722-3730), fired once
// per non-zero effect with no unit target, with args (event, spell,
// effectIndex). OnEffectHitTarget mirrors the same call in the
// SPELL_EFFECT_HANDLE_HIT_TARGET mode (Spell.cpp:2909, called from
// Spell::DoSpellEffectHit per unit target), fired once per (effect,
// live hit target) in effect-major order, with args (event, spell,
// hitTarget, effectIndex). BeforeHit mirrors
// CallScriptBeforeHitHandlers (Spell.cpp:2357, in
// Spell::TargetInfo::PreprocessTarget), fired once per unit hit
// target, miss-inclusive, with args (event, spell, hitTarget,
// missInfo) where missInfo is the C++ SpellMissInfo number (0 =
// SPELL_MISS_NONE; the Go protocol values are numerically
// identical). All six are notification-only: a handler returning
// true does NOT veto anything — C++ OnCast has no veto (CheckCast
// is the gate). The hit-damage legs are modeled through the
// per-cast spellHitDamageState (lua_spell.go): spell:GetHitDamage()
// reads the pre-effect seed (0 on a fresh target, Spell.cpp:2344)
// in BeforeHit/OnHit/OnEffectHitTarget and the final
// post-mitigation damage (Spell.cpp:2547) at AfterHit, and
// spell:SetHitDamage() writes the seed in the modifiable hooks
// (SpellScript.cpp:421-434) — a recorded write only, since Go's
// damage arms never consult the seed; it never rescales dealt damage.
// The record is threaded from the damage arms on all three paths
// (creatureEffectSpellDamage / creatureEffectSpellDamageCreature;
// the player paths write damageByTarget in their damage arms), so
// AfterHit reads the recorded final damage everywhere; the inline
// victim-spell path fires no SpellScript hooks at all, so no
// hit-damage model is needed there. GetHitHeal/SetHitHeal and
// PreventHitDamage/PreventHitDefaultEffect have no Go model: the
// default effect still runs — for SCRIPT_EFFECT legs the Go default
// is a per-spell no-op unless a case exists in
// creatureEffectScriptEffect / handleEffectScriptEffect, so a
// PreventHitDefaultEffect leg ports as hook-only. The hitTarget is
// the Lua unit object (player or creature) or nil when the target
// GUID resolves to nothing; GO, item and corpse TargetInfo legs
// (Spell.cpp:2661-2707) have no Go model and never fire.
// OnEffectHitTarget fires on the creature path and on the triggered
// (funnel) path: the player path resolves each effect's target list per
// case inside its effects switch, so there is no clean per-effect list
// site there yet — a player-cast OnEffectHitTarget leg stays blocked
// until one is built.
// OnObjectAreaTargetSelect mirrors the SpellScript hook of the same
// name (SpellScript.h, fired from Spell::SelectImplicitAreaTargets,
// Spell.cpp:1227, inside SelectTargets ahead of OnCast at :3432),
// fired once per effect carrying a selecting leg (area, cone, entry
// or ref-centered implicit target), with args (event, spell,
// effectIndex, targets) where targets is a SpellTargetList object.
// The handler MUTATES the list through its methods — GetTargets()
// reads the current list as a table of unit objects, GetCount()
// its size, Remove(unit) drops one unit, Clear() empties it,
// Add(unit) appends one unit — and the mutated list is that
// effect's hit list for every effect arm below (the C++
// std::list<WorldObject*>& mutation model); the return value is
// ignored. The list a handler sees is the Go per-effect partition
// (bossConeEffectTargets), i.e. the flat hit list for non-cone
// spells, minus the caster unless the effect carries a friendly
// area leg (C++ enemy/entry/ref/cone lists never contain the
// caster; friendly-area lists do). Creature path only, like
// OnEffectHitTarget: the player path has no per-effect list site.
// GO-area legs (51/52/108) never fire it — the list holds units
// only. The deferred-fire call re-runs the cast function, so a
// cast-time cast fires it once, at fire time, like OnCast.
const (
	SpellEventOnCast                   = 1
	SpellEventOnHit                    = 2
	SpellEventAfterHit                 = 3
	SpellEventOnEffectHit              = 4
	SpellEventOnEffectHitTarget        = 5
	SpellEventBeforeHit                = 6
	SpellEventOnObjectAreaTargetSelect = 7
)

type Hook struct {
	Kind  string
	Event int
}

type ObjectMethod func(context.Context, []any) ([]any, error)

type Object struct {
	Type    string
	Methods map[string]ObjectMethod
	Fields  map[string]any
}

// SpawnArgs is the GlobalMethods::PerformIngameSpawn argument shape:
// spawnType 1 = creature, 2 = gameobject; DurOrRespawnTime is the raw
// C++ durorresptime (milliseconds on the creature arm, seconds on the
// gameobject arm — the asymmetry is in the C++ itself, not a Go choice).
type SpawnArgs struct {
	SpawnType        uint32
	Entry            uint32
	MapID            uint32
	InstanceID       uint32
	X, Y, Z, O       float32
	Save             bool
	DurOrRespawnTime uint32
	Phase            uint32
}

// MailItemPair is one (entry, amount) attachment pair of SendMail.
type MailItemPair struct {
	Entry  uint32
	Amount uint32
}

// BanArgs is the GlobalMethods::Ban argument shape: mode 0 = account,
// 1 = character, 2 = IP; Duration is seconds, like the C++ uint32
// BanAccount overload Eluna calls. The provider returns the Eluna result
// int (0 success, 1 syntax, 2 not found, 3 longer/existing ban) and a
// non-empty luaL_argerror message for the name/IP argument when the
// C++ validation arm rejects it.
type BanArgs struct {
	Mode      int
	NameOrIP  string
	Duration  uint32
	Reason    string
	WhoBanned string
}

// SendMailArgs is the GlobalMethods::SendMail argument shape. Stationery
// defaults to MAIL_STATIONERY_DEFAULT (41); Delay is added to the game
// time in seconds by MailDraft::SendMailTo, despite the Eluna doc calling
// it milliseconds.
type SendMailArgs struct {
	Subject     string
	Text        string
	ReceiverLow uint32
	SenderLow   uint32
	Stationery  uint32
	Delay       uint32
	Money       uint32
	COD         uint32
	Items       []MailItemPair
}

// TaxiWaypoint is one GlobalMethods::AddTaxiPath waypoint: MapID, X, Y, Z
// mandatory, Flags and Delay optional (both default 0).
type TaxiWaypoint struct {
	MapID   uint32
	X, Y, Z float32
	Flags   uint32
	Delay   uint32
}

// TaxiPathArgs is the GlobalMethods::AddTaxiPath argument shape:
// (waypoints, mountA, mountH, price = 0, pathId = 0). The provider
// returns the used path id.
type TaxiPathArgs struct {
	Waypoints []TaxiWaypoint
	MountA    uint32
	MountH    uint32
	Price     uint32
	PathID    uint32
}

type Config struct {
	Enabled                  bool
	ScriptPath               string
	CoreName                 string
	CoreVersion              string
	RealmID                  uint32
	CoreExpansion            uint32
	PlayerProvider           func() []*Object
	GameEventProvider        func() []uint32
	GameEventControlProvider func(eventID uint32, start, force bool)
	MapProvider              func(mapID, instanceID uint32) *Object
	WorldMessageProvider     func(string)
	AreaNameProvider         func(id uint32) (string, bool)
	SpawnProvider            func(SpawnArgs) *Object
	TaxiPathProvider         func(TaxiPathArgs) uint32
	SendMailProvider         func(SendMailArgs) error
	VendorAddProvider        func(entry, item uint32, maxCount int32, incrTime, extCost uint32)
	VendorRemoveProvider     func(entry, item uint32) error
	VendorRemoveAllProvider  func(entry uint32)
	KickProvider             func(playerGUID uint64)
	BanProvider              func(args BanArgs) (int, string)
	SaveAllPlayersProvider   func()
	AuthDatabase             *sql.DB
	CharacterDB              *sql.DB
	WorldDatabase            *sql.DB
	Logger                   *slog.Logger
}

type Runtime struct {
	config       Config
	mu           reentrantMutex
	state        *lua.State
	nextRef      int
	nextHook     int
	hooks        []registeredHook
	cancelled    map[int]struct{}
	timers       []timer
	loadedFiles  []string
	loadFailures []error
}

type registeredHook struct {
	Hook
	id    int
	ref   int
	shots int
}

type timer struct {
	id       int
	ref      int
	minDelay int64
	maxDelay int64
	delay    int64
	repeats  int
	elapsed  int64
}

func NewRuntime(c Config) *Runtime {
	return &Runtime{config: c, nextRef: 3, nextHook: 1, cancelled: make(map[int]struct{})}
}

func (r *Runtime) Load(ctx context.Context) error {
	if !r.config.Enabled || strings.TrimSpace(r.config.ScriptPath) == "" {
		return nil
	}
	if _, err := os.Stat(r.config.ScriptPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	r.mu.Lock()
	r.initializeLocked()
	files := make([]string, 0)
	err := filepath.WalkDir(r.config.ScriptPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".lua" || extension == ".ext" {
			if strings.HasSuffix(strings.ToLower(path), string(filepath.Separator)+"stacktraceplus"+string(filepath.Separator)+"stacktraceplus.ext") {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		r.mu.Unlock()
		return err
	}
	sort.Slice(files, func(i, j int) bool {
		iExt := strings.ToLower(filepath.Ext(files[i]))
		jExt := strings.ToLower(filepath.Ext(files[j]))
		if iExt != jExt {
			return iExt == ".ext"
		}
		return files[i] < files[j]
	})
	for _, path := range files {
		r.state.SetTop(0)
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			r.state.SetTop(0)
			r.loadFailures = append(r.loadFailures, fmt.Errorf("%s: %w", path, readErr))
			if r.config.Logger != nil {
				r.config.Logger.Error("lua script failed", "path", path, "error", readErr)
			}
			continue
		}
		loadErr := r.state.Load(strings.NewReader(string(source)), "@"+path, "")
		if loadErr == nil {
			loadErr = r.state.ProtectedCall(0, lua.MultipleReturns, 0)
		}
		if loadErr != nil {
			r.state.SetTop(0)
			r.loadFailures = append(r.loadFailures, fmt.Errorf("%s: %w", path, loadErr))
			if r.config.Logger != nil {
				r.config.Logger.Error("lua script failed", "path", path, "error", loadErr)
			}
			continue
		}
		r.state.SetTop(0)
		r.loadedFiles = append(r.loadedFiles, path)
	}
	r.mu.Unlock()
	// C++ Eluna::LoadScripts fires ELUNA_EVENT_ON_LUA_STATE_OPEN (33) right
	// after every script has loaded (LuaEngine.cpp:520). The trigger takes
	// the runtime lock, so it runs after the load lock is released.
	_, _ = r.TriggerServerEvent(ctx, ServerEventLuaStateOpen)
	return nil
}

func (r *Runtime) LoadString(source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initializeLocked()
	return lua.DoString(r.state, source)
}

func (r *Runtime) LoadedFiles() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.loadedFiles...)
}

func (r *Runtime) LoadFailures() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.loadFailures...)
}

func (r *Runtime) Hooks() []Hook {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Hook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		result = append(result, hook.Hook)
	}
	return result
}

func (r *Runtime) Trigger(ctx context.Context, kind string, event int, args ...any) ([]any, error) {
	pairs, err := r.triggerN(ctx, kind, event, 1, args...)
	result := make([]any, 0, len(pairs))
	for _, pair := range pairs {
		if len(pair) > 0 {
			result = append(result, pair[0])
		}
	}
	return result, err
}

// HasHook reports whether at least one non-cancelled hook is registered
// for the given kind and event — the Go model of Eluna's
// Bindings::HasBindingsFor, which the START_HOOK macros consult before
// running hook prologues such as PlayerTalkClass::ClearMenus().
func (r *Runtime) HasHook(kind string, event int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, hook := range r.hooks {
		if _, cancelled := r.cancelled[hook.id]; cancelled {
			continue
		}
		if hook.Kind == kind && hook.Event == event {
			return true
		}
	}
	return false
}

// triggerN is the shared hook dispatcher behind Trigger. It captures
// nresults return values per handler so Eluna hooks whose C++ call sites
// consume a second return (CallOneFunction(n, args, 2)) can read it. Each
// inner slice holds one handler's returns in order; handlers returning
// fewer values leave the trailing slots nil, mirroring Lua's own padding.
// When update is non-nil it runs after each handler's returns are captured
// and may mutate args, so subsequent handlers see the updated arguments —
// the Go model of Eluna's ReplaceArgument chaining (e.g. DamageTaken's
// damageIndex rewrite).
func (r *Runtime) triggerN(ctx context.Context, kind string, event int, nresults int, args ...any) ([][]any, error) {
	return r.triggerNUpdated(ctx, kind, event, nresults, args, nil)
}

func (r *Runtime) triggerNUpdated(ctx context.Context, kind string, event int, nresults int, args []any, update func(returns []any)) ([][]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		return nil, nil
	}
	result := make([][]any, 0)
	initial := append([]registeredHook(nil), r.hooks...)
	initialIDs := make(map[int]struct{}, len(initial))
	for _, hook := range initial {
		initialIDs[hook.id] = struct{}{}
	}
	// Nested dispatch (possible now that r.mu is re-entrant) must not
	// clobber this dispatch's bookkeeping: a nested triggerNUpdated
	// reassigns r.hooks from its own snapshot, so rebuilding r.hooks
	// from this dispatch's snapshot would discard the nested dispatch's
	// shots consumption and resurrect shots==1 hooks it removed. Record
	// only what this dispatch itself did - hooks it fired (with their
	// pre-fire shots) and hooks it skipped as cancelled - and merge
	// those outcomes onto the current list at the end, so nested
	// consumption accumulates instead of being overwritten.
	fired := make(map[int]int)
	skipped := make(map[int]struct{})
	for _, hook := range initial {
		if _, cancelled := r.cancelled[hook.id]; cancelled {
			skipped[hook.id] = struct{}{}
			continue
		}
		if hook.Kind != kind || hook.Event != event {
			continue
		}
		fired[hook.id] = hook.shots
		r.state.SetTop(0)
		r.state.RawGetInt(lua.RegistryIndex, hook.ref)
		for _, arg := range args {
			if err := pushValue(r.state, arg); err != nil {
				return result, err
			}
		}
		if err := r.state.ProtectedCall(len(args), nresults, 0); err != nil {
			if r.config.Logger != nil {
				r.config.Logger.Error("lua hook failed", "kind", kind, "event", event, "error", err)
			}
			continue
		}
		vals := make([]any, nresults)
		for i := nresults - 1; i >= 0 && r.state.Top() > 0; i-- {
			vals[i] = luaValue(r.state, -1)
			r.state.Pop(1)
		}
		result = append(result, vals)
		if update != nil {
			update(vals)
		}
	}
	merged := make([]registeredHook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		if _, skip := skipped[hook.id]; skip {
			continue
		}
		if _, existed := initialIDs[hook.id]; !existed {
			if _, cancelled := r.cancelled[hook.id]; cancelled {
				continue
			}
			merged = append(merged, hook)
			continue
		}
		if shots, wasFired := fired[hook.id]; wasFired {
			switch {
			case shots == 1:
				continue
			case shots > 1:
				if hook.shots == 1 {
					continue
				}
				if hook.shots > 1 {
					hook.shots--
				}
			}
		}
		merged = append(merged, hook)
	}
	r.hooks = merged
	_ = ctx
	return result, nil
}

func (r *Runtime) TriggerPlayerEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "player", event, args...)
}

// TriggerPlayerEvent2 is TriggerPlayerEvent capturing two returns per
// handler, mirroring TriggerCreatureEvent2: Eluna chat hooks consume a
// second return (CallOneFunction(n, args, 2)) carrying the rewritten
// message, so the chat path fires through this instead of Trigger.
func (r *Runtime) TriggerPlayerEvent2(ctx context.Context, event int, args ...any) ([][]any, error) {
	return r.triggerN(ctx, "player", event, 2, args...)
}

// TriggerPlayerEventUpdated is the chained-argument player-event trigger,
// mirroring Eluna's ReplaceArgument semantics (LuaEngine/PlayerHooks.cpp):
// hooks whose C++ call sites consume a numeric return (PLAYER_EVENT_ON_GIVE_XP
// rewrites amount, PLAYER_EVENT_ON_MONEY_CHANGE rewrites amount,
// PLAYER_EVENT_ON_REPUTATION_CHANGE rewrites standing) pass each handler the
// rewritten argument, so update mutates args between handler invocations.
func (r *Runtime) TriggerPlayerEventUpdated(ctx context.Context, event int, args []any, update func(returns []any)) ([][]any, error) {
	return r.triggerNUpdated(ctx, "player", event, 1, args, update)
}

// TriggerSpellEvent fires hooks registered with
// RegisterSpellEvent(spellID, event, fn) for the given spell. Callers
// pass the Eluna-style leading event number in args, like
// TriggerAuraEvent.
func (r *Runtime) TriggerSpellEvent(ctx context.Context, spellID uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "spell:"+strconv.FormatUint(uint64(spellID), 10), event, args...)
}

// TriggerAuraEvent fires hooks registered with
// RegisterAuraEvent(spellID, event, fn) for the given spell. Callers pass
// the Eluna-style leading event number in args, like TriggerCreatureEvent.
func (r *Runtime) TriggerAuraEvent(ctx context.Context, spellID uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "aura:"+strconv.FormatUint(uint64(spellID), 10), event, args...)
}

// TriggerCreatureEvent fires hooks registered with
// RegisterCreatureEvent(entry, event, fn) for the given creature entry.
func (r *Runtime) TriggerCreatureEvent(ctx context.Context, entry uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "creature:"+strconv.FormatUint(uint64(entry), 10), event, args...)
}

// TriggerCreatureEvent2 fires hooks registered with
// RegisterCreatureEvent(entry, event, fn) for the given creature entry,
// capturing two return values per handler. It exists for Eluna hooks whose
// C++ call sites consume a second return via CallOneFunction(n, args, 2):
// CREATURE_EVENT_ON_DAMAGE_TAKEN (9) rewrites damage, and
// CREATURE_EVENT_ON_CORPSE_REMOVED (26) rewrites respawnDelay. Each inner
// slice is [firstReturn, secondReturn] in handler call order.
func (r *Runtime) TriggerCreatureEvent2(ctx context.Context, entry uint32, event int, args ...any) ([][]any, error) {
	return r.triggerN(ctx, "creature:"+strconv.FormatUint(uint64(entry), 10), event, 2, args...)
}

// TriggerCreatureEvent2Updated is TriggerCreatureEvent2 with per-handler
// argument chaining: after each handler's returns are captured, update may
// mutate args for the subsequent handlers, mirroring Eluna's
// ReplaceArgument. Used by CREATURE_EVENT_ON_DAMAGE_TAKEN (9), whose C++
// loop rewrites the damage argument each handler sees.
func (r *Runtime) TriggerCreatureEvent2Updated(ctx context.Context, entry uint32, event int, args []any, update func(returns []any)) ([][]any, error) {
	return r.triggerNUpdated(ctx, "creature:"+strconv.FormatUint(uint64(entry), 10), event, 2, args, update)
}

// TriggerUniqueCreatureEvent2Updated is the two-return, chained-argument
// variant of TriggerUniqueCreatureEvent. Eluna creature hooks consume both
// the entry bindings and the unique bindings (SetupStack merges
// CreatureEventBindings then CreatureUniqueBindings), so callers run the
// entry pass first and then this pass, threading the same args/update
// through both to mirror ReplaceArgument chaining across families.
func (r *Runtime) TriggerUniqueCreatureEvent2Updated(ctx context.Context, guid uint64, instanceID uint32, event int, args []any, update func(returns []any)) ([][]any, error) {
	return r.triggerNUpdated(ctx, "creature_unique:"+strconv.FormatUint(guid, 10)+":"+strconv.FormatUint(uint64(instanceID), 10), event, 2, args, update)
}

// TriggerUniqueCreatureEvent fires hooks registered with
// RegisterUniqueCreatureEvent(guid, instanceID, event, fn).
func (r *Runtime) TriggerUniqueCreatureEvent(ctx context.Context, guid uint64, instanceID uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "creature_unique:"+strconv.FormatUint(guid, 10)+":"+strconv.FormatUint(uint64(instanceID), 10), event, args...)
}

// TriggerGameObjectEvent fires hooks registered with
// RegisterGameObjectEvent(entry, event, fn) for the given gameobject entry.
// C++ Eluna gameobject hooks pass (event, go, ...) (LuaEngine/
// GameObjectHooks.cpp), so the event number is prepended like
// TriggerGuildEvent.
func (r *Runtime) TriggerGameObjectEvent(ctx context.Context, entry uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "gameobject:"+strconv.FormatUint(uint64(entry), 10), event, append([]any{event}, args...)...)
}

// TriggerItemEvent fires hooks registered with
// RegisterItemEvent(entry, event, fn) for the given item entry.
// C++ Eluna item hooks pass (event, player, item, ...) (LuaEngine/
// ItemHooks.cpp), so the event number is prepended like
// TriggerGameObjectEvent. Callers pass the C++ argument order without the
// leading event.
func (r *Runtime) TriggerItemEvent(ctx context.Context, entry uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "item:"+strconv.FormatUint(uint64(entry), 10), event, append([]any{event}, args...)...)
}

// ItemGossipKind is the hook kind for RegisterItemGossipEvent(entry, event,
// fn): "item_gossip:<entry>", mirroring Eluna's ItemGossipBindings entry
// key (LuaEngine/GossipHooks.cpp).
func ItemGossipKind(entry uint32) string {
	return "item_gossip:" + strconv.FormatUint(uint64(entry), 10)
}

// TriggerItemGossipEvent fires hooks registered with
// RegisterItemGossipEvent(entry, event, fn) for the given item entry.
// C++ Eluna gossip hooks pass (event, player, item, ...) (LuaEngine/
// GossipHooks.cpp), so the event number is prepended like
// TriggerItemEvent. Callers pass the C++ argument order without the
// leading event.
func (r *Runtime) TriggerItemGossipEvent(ctx context.Context, entry uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, ItemGossipKind(entry), event, append([]any{event}, args...)...)
}

// GameObjectGossipKind is the hook kind for RegisterGameObjectGossipEvent(entry,
// event, fn): "gameobject_gossip:<entry>", mirroring Eluna's
// GameObjectGossipBindings entry key (LuaEngine/GossipHooks.cpp).
func GameObjectGossipKind(entry uint32) string {
	return "gameobject_gossip:" + strconv.FormatUint(uint64(entry), 10)
}

// TriggerGameObjectGossipEvent fires hooks registered with
// RegisterGameObjectGossipEvent(entry, event, fn) for the given gameobject
// entry. C++ Eluna gossip hooks pass (event, player, go, ...)
// (LuaEngine/GossipHooks.cpp), so the event number is prepended like
// TriggerItemGossipEvent. Callers pass the C++ argument order without the
// leading event.
func (r *Runtime) TriggerGameObjectGossipEvent(ctx context.Context, entry uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, GameObjectGossipKind(entry), event, append([]any{event}, args...)...)
}

// PlayerGossipKind is the hook kind for RegisterPlayerGossipEvent(menuID,
// event, fn): "player_gossip:<menuID>", mirroring Eluna's PlayerGossipBindings
// menuId key (LuaEngine/GossipHooks.cpp:66).
func PlayerGossipKind(menuID uint32) string {
	return "player_gossip:" + strconv.FormatUint(uint64(menuID), 10)
}

// TriggerPlayerGossipEvent fires hooks registered with
// RegisterPlayerGossipEvent(menuID, event, fn) for the given gossip menu.
// C++ Eluna gossip hooks pass (event, player, ...) (LuaEngine/GossipHooks.cpp),
// so the event number is prepended like TriggerGameObjectGossipEvent. The
// player select arm passes (event, player, player, sender, action, code-or-nil)
// with no cancel semantics (CallAllFunctions, GossipHooks.cpp:78).
func (r *Runtime) TriggerPlayerGossipEvent(ctx context.Context, menuID uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, PlayerGossipKind(menuID), event, append([]any{event}, args...)...)
}

// TriggerInstanceEvent fires hooks registered with RegisterMapEvent(mapID,
// event, fn) (kind "map:<mapID>") and RegisterInstanceEvent(instanceID,
// event, fn) (kind "instance:<instanceID>"), mirroring Eluna::InstanceHooks
// (LuaEngine/InstanceHooks.cpp), whose START_HOOK macro calls
// CallAllFunctions over both the map-level and instance-level binding maps
// at every fire site. C++ instance hooks pass (event, instance_data, map,
// ...) (LuaEngine/InstanceHooks.cpp), so the event number is prepended like
// TriggerMapEvent. Callers pass the C++ argument order without the leading
// event.
func (r *Runtime) TriggerInstanceEvent(ctx context.Context, mapID, instanceID uint32, event int, args ...any) ([]any, error) {
	mapValues, err := r.Trigger(ctx, "map:"+strconv.FormatUint(uint64(mapID), 10), event, append([]any{event}, args...)...)
	if err != nil {
		return mapValues, err
	}
	instanceValues, err := r.Trigger(ctx, "instance:"+strconv.FormatUint(uint64(instanceID), 10), event, append([]any{event}, args...)...)
	return append(mapValues, instanceValues...), err
}

func (r *Runtime) TriggerMapEvent(ctx context.Context, mapID uint32, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "map:"+strconv.FormatUint(uint64(mapID), 10), event, append([]any{event}, args...)...)
}

func (r *Runtime) TriggerServerEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "server", event, append([]any{event}, args...)...)
}

// TriggerGuildEvent fires hooks registered with RegisterGuildEvent(event,
// fn). C++ Eluna guild hooks pass (event, guild, ...) (LuaEngine/
// GuildHooks.cpp), so the event number is prepended like TriggerServerEvent.
func (r *Runtime) TriggerGuildEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "guild", event, append([]any{event}, args...)...)
}

// TriggerGuildEventUpdated is TriggerGuildEvent with per-handler argument
// chaining: after each handler's returns are captured, update may mutate
// args for the subsequent handlers, mirroring Eluna's ReplaceArgument. Used
// by GUILD_EVENT_ON_MONEY_WITHDRAW (7) and GUILD_EVENT_ON_MONEY_DEPOSIT (8),
// whose C++ call sites (GuildHooks.cpp OnMemberWitdrawMoney /
// OnMemberDepositMoney) consume one numeric return per handler as the new
// amount and rewrite the argument each handler sees (CallOneFunction(n,
// args, 1), so nresults is 1).
func (r *Runtime) TriggerGuildEventUpdated(ctx context.Context, event int, args []any, update func(returns []any)) ([][]any, error) {
	return r.triggerNUpdated(ctx, "guild", event, 1, args, update)
}

// TriggerGroupEvent fires hooks registered with RegisterGroupEvent(event,
// fn). C++ Eluna group hooks pass (event, group, ...) (LuaEngine/
// GroupHooks.cpp), so the event number is prepended like TriggerGuildEvent.
func (r *Runtime) TriggerGroupEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "group", event, append([]any{event}, args...)...)
}

// TriggerVehicleEvent fires hooks registered with RegisterVehicleEvent(event,
// fn). C++ Eluna vehicle hooks pass (event, vehicle, ...) (LuaEngine/
// VehicleHooks.cpp), so the event number is prepended like TriggerGroupEvent.
func (r *Runtime) TriggerVehicleEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "vehicle", event, append([]any{event}, args...)...)
}

// TriggerBGEvent fires hooks registered with RegisterBGEvent(event, fn).
// C++ Eluna battleground hooks pass (event, bg, bgId, instanceId[, winner])
// (LuaEngine/BattleGroundHooks.cpp), so the event number is prepended like
// TriggerVehicleEvent. Callers pass the C++ argument order without the
// leading event.
func (r *Runtime) TriggerBGEvent(ctx context.Context, event int, args ...any) ([]any, error) {
	return r.Trigger(ctx, "bg", event, append([]any{event}, args...)...)
}

func (r *Runtime) TriggerPacketEvent(ctx context.Context, opcode, event int, args ...any) ([]any, error) {
	if !r.mu.TryLockFree() {
		return nil, nil
	}
	r.mu.Unlock()
	return r.Trigger(ctx, "packet:"+strconv.Itoa(opcode), event, append([]any{event}, args...)...)
}

// TriggerServerEvent2 fires hooks registered with RegisterServerEvent(event,
// fn), capturing two return values per handler. It exists for Eluna hooks
// whose C++ call sites consume a second return via CallOneFunction(n, args,
// 2): SERVER_EVENT_ON_PACKET_RECEIVE (5), where a boolean false first return
// blocks the opcode handler and a Packet userdata second return replaces the
// packet the handler sees (LuaEngine/PacketHooks.cpp:93-115). The TryLock
// guard mirrors TriggerPacketEvent: packet hooks can fire while a hook
// already holds the runtime lock (e.g. Lua -> SendPacket -> send hooks), in
// which case the fire is skipped instead of deadlocking.
func (r *Runtime) TriggerServerEvent2(ctx context.Context, event int, args ...any) ([][]any, error) {
	if !r.mu.TryLockFree() {
		return nil, nil
	}
	r.mu.Unlock()
	return r.triggerN(ctx, "server", event, 2, append([]any{event}, args...)...)
}

// TriggerServerEventGuarded is TriggerServerEvent with TriggerServerEvent2's
// TryLock guard: the packet-send hook fire (world firePacketSendHooks) runs
// on the write path, so a send-hook handler that broadcasts via Group or
// Guild SendPacket re-enters the send-hook fire on the same goroutine while
// triggerNUpdated still holds r.mu. The blocking TriggerServerEvent would
// deadlock there; the guarded fire is skipped instead, like the receive
// path, and the packet is delivered.
func (r *Runtime) TriggerServerEventGuarded(ctx context.Context, event int, args ...any) ([]any, error) {
	if !r.mu.TryLockFree() {
		return nil, nil
	}
	r.mu.Unlock()
	return r.Trigger(ctx, "server", event, append([]any{event}, args...)...)
}

// TriggerPacketEvent2 fires hooks registered with RegisterPacketEvent(opcode,
// event, fn), capturing two return values per handler, like
// TriggerCreatureEvent2. It exists for the fork's per-opcode RECEIVE bindings,
// whose C++ lookup site (LuaEngine/PacketHooks.cpp:117-119) consumes two
// returns per handler: a boolean false first return blocks the opcode handler
// and a Packet userdata second return replaces the packet the handler sees.
// NOTE the fork quirk: that lookup uses the SEND event number
// (PACKET_EVENT_ON_PACKET_SEND = 7, LuaEngine/Hooks.h:96), so on receive the
// engine fires event-7 per-opcode bindings, never event-5 ones.
func (r *Runtime) TriggerPacketEvent2(ctx context.Context, opcode, event int, args ...any) ([][]any, error) {
	if !r.mu.TryLockFree() {
		return nil, nil
	}
	r.mu.Unlock()
	return r.triggerN(ctx, "packet:"+strconv.Itoa(opcode), event, 2, append([]any{event}, args...)...)
}

func (r *Runtime) Tick(ctx context.Context, elapsed int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil || elapsed < 0 {
		return nil
	}
	remaining := r.timers[:0]
	for _, current := range r.timers {
		current.elapsed += elapsed
		removeCurrent := false
		for current.elapsed >= current.delay {
			current.elapsed -= current.delay
			callbackDelay := current.delay
			remove := current.repeats == 1
			remainingRepeats := current.repeats
			if current.repeats > 1 {
				current.repeats--
			}
			r.state.SetTop(0)
			r.state.RawGetInt(lua.RegistryIndex, current.ref)
			r.state.PushInteger(current.id)
			r.state.PushInteger(int(callbackDelay))
			r.state.PushInteger(remainingRepeats)
			if err := r.state.ProtectedCall(3, 0, 0); err != nil && r.config.Logger != nil {
				r.config.Logger.Error("lua timer failed", "id", current.id, "error", err)
			}
			if remove {
				removeCurrent = true
				break
			}
			current.delay = timerDelay(current.minDelay, current.maxDelay)
		}
		if !removeCurrent {
			remaining = append(remaining, current)
		}
	}
	r.timers = remaining
	_ = ctx
	return nil
}

func (r *Runtime) initializeLocked() {
	if r.state != nil {
		return
	}
	r.state = lua.NewState()
	lua.OpenLibraries(r.state)
	installLuaStringCompatibility(r.state)
	for _, name := range []string{"Map", "Player", "Creature", "GameObject"} {
		r.state.NewTable()
		r.state.SetGlobal(name)
	}
	r.state.Register("RegisterPlayerEvent", r.registerPlayerEvent)
	r.state.Register("RegisterGuildEvent", r.registerGuildEvent)
	r.state.Register("RegisterGroupEvent", r.registerGroupEvent)
	r.state.Register("RegisterVehicleEvent", r.registerVehicleEvent)
	r.state.Register("RegisterCreatureEvent", r.registerCreatureEvent)
	r.state.Register("RegisterAuraEvent", r.registerAuraEvent)
	r.state.Register("RegisterSpellEvent", r.registerSpellEvent)
	r.state.Register("RegisterUniqueCreatureEvent", r.registerUniqueCreatureEvent)
	r.state.Register("RegisterCreatureGossipEvent", r.registerCreatureGossipEvent)
	r.state.Register("RegisterGameObjectEvent", r.registerGameObjectEvent)
	r.state.Register("RegisterGameObjectGossipEvent", r.registerGameObjectGossipEvent)
	r.state.Register("RegisterItemEvent", r.registerItemEvent)
	r.state.Register("RegisterItemGossipEvent", r.registerItemGossipEvent)
	r.state.Register("RegisterPlayerGossipEvent", r.registerPlayerGossipEvent)
	r.state.Register("RegisterBGEvent", r.registerBGEvent)
	r.state.Register("RegisterPacketEvent", r.registerPacketEvent)
	r.state.Register("RegisterMapEvent", r.registerMapEvent)
	r.state.Register("RegisterInstanceEvent", r.registerInstanceEvent)
	r.state.Register("RegisterServerEvent", r.registerServerEvent)
	r.state.Register("RegisterGlobalEvent", r.registerServerEvent)
	r.state.Register("ClearBattleGroundEvents", r.clearBattleGroundEvents)
	r.state.Register("ClearCreatureEvents", r.clearCreatureEvents)
	r.state.Register("ClearAuraEvents", r.clearAuraEvents)
	r.state.Register("ClearSpellEvents", r.clearSpellEvents)
	r.state.Register("ClearUniqueCreatureEvents", r.clearUniqueCreatureEvents)
	r.state.Register("ClearCreatureGossipEvents", r.clearCreatureGossipEvents)
	r.state.Register("ClearGameObjectEvents", r.clearGameObjectEvents)
	r.state.Register("ClearGameObjectGossipEvents", r.clearGameObjectGossipEvents)
	r.state.Register("ClearGroupEvents", r.clearGroupEvents)
	r.state.Register("ClearVehicleEvents", r.clearVehicleEvents)
	r.state.Register("ClearGuildEvents", r.clearGuildEvents)
	r.state.Register("ClearItemEvents", r.clearItemEvents)
	r.state.Register("ClearItemGossipEvents", r.clearItemGossipEvents)
	r.state.Register("ClearPacketEvents", r.clearPacketEvents)
	r.state.Register("ClearPlayerEvents", r.clearPlayerEvents)
	r.state.Register("ClearPlayerGossipEvents", r.clearPlayerGossipEvents)
	r.state.Register("ClearServerEvents", r.clearServerEvents)
	r.state.Register("ClearMapEvents", r.clearMapEvents)
	r.state.Register("ClearInstanceEvents", r.clearInstanceEvents)
	r.state.Register("GetCoreExpansion", r.getCoreExpansion)
	r.state.Register("GetLuaEngine", r.getLuaEngine)
	r.state.Register("GetCoreName", r.getCoreName)
	r.state.Register("GetRealmID", r.getRealmID)
	r.state.Register("GetCoreVersion", r.getCoreVersion)
	r.state.Register("GetGameTime", r.getGameTime)
	r.state.Register("GetGameTimeMS", r.getGameTimeMS)
	r.state.Register("GetQuest", r.getQuest)
	r.state.Register("GetGuildByName", r.getGuildByName)
	r.state.Register("GetItemLink", r.getItemLink)
	r.state.Register("GetGuildByLeaderGUID", r.getGuildByLeaderGUID)
	r.state.Register("GetPlayersInWorld", r.getPlayersInWorld)
	r.state.Register("GetPlayerByGUID", r.getPlayerByGUID)
	r.state.Register("GetPlayerByName", r.getPlayerByName)
	r.state.Register("GetPlayerCount", r.getPlayerCount)
	r.state.Register("GetPlayerGUID", r.getPlayerGUID)
	r.state.Register("GetItemGUID", r.getItemGUID)
	r.state.Register("GetObjectGUID", r.getObjectGUID)
	r.state.Register("GetUnitGUID", r.getUnitGUID)
	r.state.Register("GetGUIDLow", r.getGUIDLow)
	r.state.Register("GetGUIDType", r.getGUIDType)
	r.state.Register("GetGUIDEntry", r.getGUIDEntry)
	r.state.Register("GetCurrTime", r.getCurrTime)
	r.state.Register("GetTimeDiff", r.getTimeDiff)
	r.state.Register("IsInventoryPos", r.isInventoryPos)
	r.state.Register("IsEquipmentPos", r.isEquipmentPos)
	r.state.Register("IsBankPos", r.isBankPos)
	r.state.Register("IsBagPos", r.isBagPos)
	r.state.Register("IsGameEventActive", r.isGameEventActive)
	r.state.Register("GetActiveGameEvents", r.getActiveGameEvents)
	r.state.Register("StartGameEvent", r.startGameEvent)
	r.state.Register("StopGameEvent", r.stopGameEvent)
	r.state.Register("GetMapById", r.getMapById)
	r.state.Register("PerformIngameSpawn", r.performIngameSpawn)
	r.state.Register("AddTaxiPath", r.addTaxiPath)
	r.state.Register("SendMail", r.sendMail)
	r.state.Register("AddVendorItem", r.addVendorItem)
	r.state.Register("VendorRemoveItem", r.vendorRemoveItem)
	r.state.Register("VendorRemoveAllItems", r.vendorRemoveAllItems)
	r.state.Register("Kick", r.kickPlayer)
	r.state.Register("Ban", r.banPlayer)
	r.state.Register("SaveAllPlayers", r.saveAllPlayers)
	r.state.Register("SendWorldMessage", r.sendWorldMessage)
	r.state.Register("GetAreaName", r.getAreaName)
	r.state.Register("CreateInt64", r.createInt64)
	r.state.Register("CreateUint64", r.createUint64)
	r.state.Register("CreateLongLong", r.createInt64)
	r.state.Register("CreateULongLong", r.createUint64)
	r.state.Register("CreatePacket", r.createPacket)
	r.state.Register("bit_and", luaBitAnd)
	r.state.Register("bit_or", luaBitOr)
	r.state.Register("bit_lshift", luaBitLShift)
	r.state.Register("bit_rshift", luaBitRShift)
	r.state.Register("bit_xor", luaBitXor)
	r.state.Register("bit_not", luaBitNot)
	r.state.Register("PrintInfo", r.printInfo)
	r.state.Register("PrintError", r.printError)
	r.state.Register("PrintDebug", r.printDebug)
	r.state.Register("CreateLuaEvent", r.createLuaEvent)
	r.state.Register("RemoveEventById", r.removeEvent)
	r.state.Register("RemoveEvents", r.removeEvents)
	r.state.Register("CharDBQuery", r.charDBQuery)
	r.state.Register("WorldDBQuery", r.worldDBQuery)
	r.state.Register("AuthDBQuery", r.authDBQuery)
	r.state.Register("CharDBExecute", r.charDBExecute)
	r.state.Register("WorldDBExecute", r.worldDBExecute)
	r.state.Register("AuthDBExecute", r.authDBExecute)
	lua.NewMetaTable(r.state, queryMetaTable)
	lua.SetFunctions(r.state, []lua.RegistryFunction{{Name: "__index", Function: queryIndex}}, 0)
	r.state.Pop(1)
	lua.NewMetaTable(r.state, objectMetaTable)
	lua.SetFunctions(r.state, []lua.RegistryFunction{{Name: "__index", Function: objectIndex}, {Name: "__eq", Function: objectEqual}, {Name: "__newindex", Function: objectNewIndex}}, 0)
	r.state.Pop(1)
	r.state.NewTable()
	r.state.SetField(lua.RegistryIndex, instanceDataStoreKey)
	installUInt64MetaTable(r.state)
	installPacketMetaTable(r.state)
	setPackagePath(r.state, r.config.ScriptPath)
}

func (r *Runtime) getCoreExpansion(state *lua.State) int {
	state.PushUnsigned(uint(r.config.CoreExpansion))
	return 1
}

func (r *Runtime) getPlayersInWorld(state *lua.State) int {
	team := uint32(2)
	if state.Top() >= 1 && !state.IsNil(1) {
		team = uint32(lua.CheckUnsigned(state, 1))
	}
	onlyGM := state.Top() >= 2 && !state.IsNil(2) && state.ToBoolean(2)
	state.NewTable()
	if r.config.PlayerProvider == nil {
		return 1
	}
	index := 0
	for _, player := range r.config.PlayerProvider() {
		if player == nil {
			continue
		}
		if team != 2 {
			playerTeam, ok := objectUint32(player, "Team")
			if !ok || playerTeam != team {
				continue
			}
		}
		if onlyGM {
			isGM, ok := player.Fields["IsGM"].(bool)
			if !ok || !isGM {
				continue
			}
		}
		if err := pushValue(state, player); err != nil {
			continue
		}
		index++
		state.RawSetInt(-2, index)
	}
	return 1
}

func (r *Runtime) getPlayerByName(state *lua.State) int {
	name := lua.CheckString(state, 1)
	if r.config.PlayerProvider != nil {
		for _, player := range r.config.PlayerProvider() {
			if player == nil {
				continue
			}
			if value, ok := player.Fields["Name"].(string); ok && strings.EqualFold(value, name) {
				PushObject(state, player)
				return 1
			}
		}
	}
	state.PushNil()
	return 1
}

// isInventoryPos is Player::IsInventoryPos (Player.cpp:10141): bag 255
// with slot 255, bag 255 with an inventory-pack slot, a bag-container bag,
// or bag 255 with a keyring/currency slot.
func (r *Runtime) isInventoryPos(state *lua.State) int {
	bag := uint8(lua.CheckUnsigned(state, 1))
	slot := uint8(lua.CheckUnsigned(state, 2))
	state.PushBoolean(bag == 255 && slot == 255 || bag == 255 && slot >= 23 && slot < 39 || bag >= 19 && bag < 23 || bag == 255 && slot >= 86 && slot < 150)
	return 1
}

// isEquipmentPos is Player::IsEquipmentPos (Player.cpp:10157): bag 255
// with an equipment or bag-container slot.
func (r *Runtime) isEquipmentPos(state *lua.State) int {
	bag := uint8(lua.CheckUnsigned(state, 1))
	slot := uint8(lua.CheckUnsigned(state, 2))
	state.PushBoolean(bag == 255 && slot < 19 || bag == 255 && slot >= 19 && slot < 23)
	return 1
}

// isBankPos is Player::IsBankPos (Player.cpp:10166): bag 255 with a bank
// item or bank bag slot, or a bank-bag container bag.
func (r *Runtime) isBankPos(state *lua.State) int {
	bag := uint8(lua.CheckUnsigned(state, 1))
	slot := uint8(lua.CheckUnsigned(state, 2))
	state.PushBoolean(bag == 255 && slot >= 39 && slot < 67 || bag == 255 && slot >= 67 && slot < 74 || bag >= 67 && bag < 74)
	return 1
}

// isBagPos is Player::IsBagPos (Player.cpp:10178) via the Eluna
// (bag<<8)+slot fold: bag 255 with a bag-container or bank-bag slot.
func (r *Runtime) isBagPos(state *lua.State) int {
	bag := uint8(lua.CheckUnsigned(state, 1))
	slot := uint8(lua.CheckUnsigned(state, 2))
	state.PushBoolean(bag == 255 && slot >= 19 && slot < 23 || bag == 255 && slot >= 67 && slot < 74)
	return 1
}

// isGameEventActive is GameEventMgr::IsActiveEvent behind the Eluna
// IsGameEventActive global; without a provider no event reads active.
func (r *Runtime) isGameEventActive(state *lua.State) int {
	id := uint32(lua.CheckUnsigned(state, 1))
	active := false
	if r.config.GameEventProvider != nil {
		for _, eventID := range r.config.GameEventProvider() {
			if eventID == id {
				active = true
				break
			}
		}
	}
	state.PushBoolean(active)
	return 1
}

// getActiveGameEvents is GameEventMgr::GetActiveEventList behind the Eluna
// GetActiveGameEvents global: the active ids, 1-indexed, ascending like
// the C++ std::set iteration order.
func (r *Runtime) getActiveGameEvents(state *lua.State) int {
	state.NewTable()
	if r.config.GameEventProvider == nil {
		return 1
	}
	for index, eventID := range r.config.GameEventProvider() {
		state.PushUnsigned(uint(eventID))
		state.RawSetInt(-2, index+1)
	}
	return 1
}

// startGameEvent is GlobalMethods::StartGameEvent: (eventId, force = false),
// no return. Without a provider it is a no-op.
func (r *Runtime) startGameEvent(state *lua.State) int {
	id := uint32(lua.CheckUnsigned(state, 1))
	force := state.Top() >= 2 && !state.IsNil(2) && state.ToBoolean(2)
	if r.config.GameEventControlProvider != nil {
		r.config.GameEventControlProvider(id, true, force)
	}
	return 0
}

// stopGameEvent is GlobalMethods::StopGameEvent: (eventId, force = false),
// no return. Without a provider it is a no-op.
func (r *Runtime) stopGameEvent(state *lua.State) int {
	id := uint32(lua.CheckUnsigned(state, 1))
	force := state.Top() >= 2 && !state.IsNil(2) && state.ToBoolean(2)
	if r.config.GameEventControlProvider != nil {
		r.config.GameEventControlProvider(id, false, force)
	}
	return 0
}

// getMapById is MapMgr::FindMap behind the Eluna GetMapById global
// (mapId, instanceId = 0); without a provider, or for an unknown map,
// it reads nil like the C++ nullptr arm.
func (r *Runtime) getMapById(state *lua.State) int {
	mapID := uint32(lua.CheckUnsigned(state, 1))
	instanceID := uint32(0)
	if state.Top() >= 2 && !state.IsNil(2) {
		instanceID = uint32(lua.CheckUnsigned(state, 2))
	}
	if r.config.MapProvider == nil {
		state.PushNil()
		return 1
	}
	if object := r.config.MapProvider(mapID, instanceID); object != nil {
		PushObject(state, object)
		return 1
	}
	state.PushNil()
	return 1
}

// performIngameSpawn is GlobalMethods::PerformIngameSpawn
// (GlobalMethods.h, Trinity branch): (spawnType, entry, mapId,
// instanceId, x, y, z, o, save = false, durorresptime = 0,
// phase = PHASEMASK_NORMAL). A phase of 0 reads nil like the C++
// prologue arm; an unknown spawnType falls through to nil, and without
// a provider the call reads nil like the C++ FindMap nullptr arm.
func (r *Runtime) performIngameSpawn(state *lua.State) int {
	args := SpawnArgs{
		SpawnType:  uint32(lua.CheckUnsigned(state, 1)),
		Entry:      uint32(lua.CheckUnsigned(state, 2)),
		MapID:      uint32(lua.CheckUnsigned(state, 3)),
		InstanceID: uint32(lua.CheckUnsigned(state, 4)),
		X:          float32(lua.CheckNumber(state, 5)),
		Y:          float32(lua.CheckNumber(state, 6)),
		Z:          float32(lua.CheckNumber(state, 7)),
		O:          float32(lua.CheckNumber(state, 8)),
		Phase:      1,
	}
	if state.Top() >= 9 && !state.IsNil(9) {
		args.Save = state.ToBoolean(9)
	}
	if state.Top() >= 10 && !state.IsNil(10) {
		args.DurOrRespawnTime = uint32(lua.CheckUnsigned(state, 10))
	}
	if state.Top() >= 11 && !state.IsNil(11) {
		args.Phase = uint32(lua.CheckUnsigned(state, 11))
	}
	if args.Phase == 0 {
		state.PushNil()
		return 1
	}
	if r.config.SpawnProvider == nil {
		state.PushNil()
		return 1
	}
	if object := r.config.SpawnProvider(args); object != nil {
		PushObject(state, object)
		return 1
	}
	state.PushNil()
	return 1
}

// addTaxiPath is GlobalMethods::AddTaxiPath (GlobalMethods.h, Trinity
// branch): (waypoints, mountA, mountH, price = 0, pathId = 0). Each
// waypoint is {map, x, y, z[, actionFlag, delay]} in array form, which is
// the order the C++ lua_next collection reads them in. A waypoint with
// no values is skipped like the C++ start == end arm; one with one to
// three values raises like the C++ luaL_argerror arm; without a provider
// the call reads nil like the PerformIngameSpawn no-provider arm.
func (r *Runtime) addTaxiPath(state *lua.State) int {
	if !state.IsTable(1) {
		lua.ArgumentError(state, 1, "table expected")
	}
	args := TaxiPathArgs{
		MountA: uint32(lua.CheckUnsigned(state, 2)),
		MountH: uint32(lua.CheckUnsigned(state, 3)),
	}
	if state.Top() >= 4 && !state.IsNil(4) {
		args.Price = uint32(lua.CheckUnsigned(state, 4))
	}
	if state.Top() >= 5 && !state.IsNil(5) {
		args.PathID = uint32(lua.CheckUnsigned(state, 5))
	}
	count := state.RawLength(1)
	for i := 1; i <= count; i++ {
		state.RawGetInt(1, i)
		if state.IsNil(-1) {
			state.Pop(1)
			continue
		}
		if !state.IsTable(-1) {
			lua.ArgumentError(state, 1, "table expected")
		}
		waypointLen := state.RawLength(-1)
		if waypointLen < 6 {
			waypointLen = 6
		}
		var waypoint TaxiWaypoint
		values := 0
		for field := 1; field <= waypointLen; field++ {
			state.RawGetInt(-1, field)
			if state.IsNil(-1) {
				state.Pop(1)
				continue
			}
			values++
			switch field {
			case 1:
				waypoint.MapID = uint32(lua.CheckUnsigned(state, -1))
			case 2:
				waypoint.X = float32(lua.CheckNumber(state, -1))
			case 3:
				waypoint.Y = float32(lua.CheckNumber(state, -1))
			case 4:
				waypoint.Z = float32(lua.CheckNumber(state, -1))
			case 5:
				waypoint.Flags = uint32(lua.CheckUnsigned(state, -1))
			case 6:
				waypoint.Delay = uint32(lua.CheckUnsigned(state, -1))
			}
			state.Pop(1)
		}
		state.Pop(1)
		if values == 0 {
			continue
		}
		if values < 4 {
			lua.ArgumentError(state, 1, "all waypoints do not have mandatory arguments")
		}
		args.Waypoints = append(args.Waypoints, waypoint)
	}
	if r.config.TaxiPathProvider == nil {
		state.PushNil()
		return 1
	}
	state.PushUnsigned(uint(r.config.TaxiPathProvider(args)))
	return 1
}

// sendMail is GlobalMethods::SendMail (GlobalMethods.h): (subject, text,
// receiverGUIDLow, senderGUIDLow = 0, stationary = MAIL_STATIONERY_DEFAULT,
// delay = 0, money = 0, cod = 0, entry, amount, ...). The attachment
// pairs follow the C++ loop shape exactly: while added <= MAX_MAIL_ITEMS
// (12) and a full pair remains, so up to 13 pairs are accepted. A bad
// pair raises like the C++ luaL_error arms; without a provider it is a
// no-op.
func (r *Runtime) sendMail(state *lua.State) int {
	args := SendMailArgs{
		Subject:     lua.CheckString(state, 1),
		Text:        lua.CheckString(state, 2),
		ReceiverLow: uint32(lua.CheckUnsigned(state, 3)),
		Stationery:  41,
	}
	if state.Top() >= 4 && !state.IsNil(4) {
		args.SenderLow = uint32(lua.CheckUnsigned(state, 4))
	}
	if state.Top() >= 5 && !state.IsNil(5) {
		args.Stationery = uint32(lua.CheckUnsigned(state, 5))
	}
	if state.Top() >= 6 && !state.IsNil(6) {
		args.Delay = uint32(lua.CheckUnsigned(state, 6))
	}
	if state.Top() >= 7 && !state.IsNil(7) {
		args.Money = uint32(lua.CheckUnsigned(state, 7))
	}
	if state.Top() >= 8 && !state.IsNil(8) {
		args.COD = uint32(lua.CheckUnsigned(state, 8))
	}
	for index, added := 9, 0; added <= 12 && index+1 <= state.Top(); index, added = index+2, added+1 {
		args.Items = append(args.Items, MailItemPair{
			Entry:  uint32(lua.CheckUnsigned(state, index)),
			Amount: uint32(lua.CheckUnsigned(state, index+1)),
		})
	}
	if r.config.SendMailProvider != nil {
		if err := r.config.SendMailProvider(args); err != nil {
			lua.Errorf(state, "%s", err.Error())
			return 0
		}
	}
	return 0
}

// addVendorItem is GlobalMethods::AddVendorItem (GlobalMethods.h, Trinity
// branch): (entry, item, maxcount, incrtime, extendedcost). Validation
// failure reads a silent no-op like the C++ IsVendorItemValid arm; without
// a provider it is a no-op.
func (r *Runtime) addVendorItem(state *lua.State) int {
	entry := uint32(lua.CheckUnsigned(state, 1))
	item := uint32(lua.CheckUnsigned(state, 2))
	maxCount := int32(lua.CheckInteger(state, 3))
	incrTime := uint32(lua.CheckUnsigned(state, 4))
	extCost := uint32(lua.CheckUnsigned(state, 5))
	if r.config.VendorAddProvider != nil {
		r.config.VendorAddProvider(entry, item, maxCount, incrTime, extCost)
	}
	return 0
}

// vendorRemoveItem is GlobalMethods::VendorRemoveItem: (entry, item). The
// C++ template-missing arm is a luaL_argerror; without a provider the
// call reads a no-op.
func (r *Runtime) vendorRemoveItem(state *lua.State) int {
	entry := uint32(lua.CheckUnsigned(state, 1))
	item := uint32(lua.CheckUnsigned(state, 2))
	if r.config.VendorRemoveProvider != nil {
		if err := r.config.VendorRemoveProvider(entry, item); err != nil {
			lua.ArgumentError(state, 1, "valid CreatureEntry expected")
			return 0
		}
	}
	return 0
}

// vendorRemoveAllItems is GlobalMethods::VendorRemoveAllItems: (entry).
// Without a provider it is a no-op.
func (r *Runtime) vendorRemoveAllItems(state *lua.State) int {
	entry := uint32(lua.CheckUnsigned(state, 1))
	if r.config.VendorRemoveAllProvider != nil {
		r.config.VendorRemoveAllProvider(entry)
	}
	return 0
}

// kickPlayer is GlobalMethods::Kick: (player). The argument is the player
// object; its GUID field resolves the session, like the C++ session kick.
// A non-player object resolves to no session and no-ops. Without a
// provider it is a no-op.
func (r *Runtime) kickPlayer(state *lua.State) int {
	object := lua.CheckUserData(state, 1, objectMetaTable).(*Object)
	guid, _ := objectUint64(object, "GUID")
	if r.config.KickProvider != nil {
		r.config.KickProvider(guid)
	}
	return 0
}

// banPlayer is GlobalMethods::Ban: (banMode, nameOrIP, duration, reason,
// whoBanned). The mode switch and its luaL_argerror arms mirror the C++
// function; name/IP validation runs in the provider, which reports the
// argument-2 message. Without a provider the result reads nil (no ban
// performed, so no success value is claimed); an unknown mode still
// argerrors.
func (r *Runtime) banPlayer(state *lua.State) int {
	mode := int(lua.CheckInteger(state, 1))
	if mode < 0 || mode > 2 {
		lua.ArgumentError(state, 1, "unknown banmode")
		return 0
	}
	args := BanArgs{
		Mode:     mode,
		NameOrIP: lua.CheckString(state, 2),
		Duration: uint32(lua.CheckUnsigned(state, 3)),
	}
	if state.Top() >= 4 && !state.IsNil(4) {
		args.Reason = lua.CheckString(state, 4)
	}
	if state.Top() >= 5 && !state.IsNil(5) {
		args.WhoBanned = lua.CheckString(state, 5)
	}
	if r.config.BanProvider != nil {
		result, argErr := r.config.BanProvider(args)
		if argErr != "" {
			lua.ArgumentError(state, 2, argErr)
			return 0
		}
		state.PushInteger(result)
		return 1
	}
	state.PushNil()
	return 1
}

// saveAllPlayers is GlobalMethods::SaveAllPlayers: no arguments, no
// return. Without a provider it is a no-op.
func (r *Runtime) saveAllPlayers(state *lua.State) int {
	if r.config.SaveAllPlayersProvider != nil {
		r.config.SaveAllPlayersProvider()
	}
	return 0
}

// sendWorldMessage is World::SendServerMessage(SERVER_MSG_STRING) behind
// the Eluna SendWorldMessage global; without a provider it is a no-op.
func (r *Runtime) sendWorldMessage(state *lua.State) int {
	if r.config.WorldMessageProvider != nil {
		r.config.WorldMessageProvider(lua.CheckString(state, 1))
	}
	return 0
}

// getAreaName is GlobalMethods::GetAreaName (GlobalMethods.h:483): the
// AreaTable name for the id, with the C++ luaL_argerror arms for an
// unknown id and a locale >= TOTAL_LOCALES (7, the getItemLink twin).
// The locale argument is accepted and range-checked but Go DBC names are
// enUS-only, so it has no effect.
func (r *Runtime) getAreaName(state *lua.State) int {
	id := uint32(lua.CheckUnsigned(state, 1))
	var locale uint64
	if state.Top() >= 2 && !state.IsNil(2) {
		locale = checkLuaUint64(state, 2)
	}
	if locale >= 7 {
		lua.ArgumentError(state, 2, "valid LocaleConstant expected")
		return 0
	}
	if r.config.AreaNameProvider == nil {
		lua.ArgumentError(state, 1, "valid Area or Zone ID expected")
		return 0
	}
	name, found := r.config.AreaNameProvider(id)
	if !found {
		lua.ArgumentError(state, 1, "valid Area or Zone ID expected")
		return 0
	}
	state.PushString(name)
	return 1
}

func (r *Runtime) SetPlayerProvider(provider func() []*Object) {
	r.mu.Lock()
	r.config.PlayerProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetGameEventProvider(provider func() []uint32) {
	r.mu.Lock()
	r.config.GameEventProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetGameEventControlProvider(provider func(eventID uint32, start, force bool)) {
	r.mu.Lock()
	r.config.GameEventControlProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetMapProvider(provider func(mapID, instanceID uint32) *Object) {
	r.mu.Lock()
	r.config.MapProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetWorldMessageProvider(provider func(string)) {
	r.mu.Lock()
	r.config.WorldMessageProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetAreaNameProvider(provider func(id uint32) (string, bool)) {
	r.mu.Lock()
	r.config.AreaNameProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetTaxiPathProvider(provider func(TaxiPathArgs) uint32) {
	r.mu.Lock()
	r.config.TaxiPathProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetSpawnProvider(provider func(SpawnArgs) *Object) {
	r.mu.Lock()
	r.config.SpawnProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetVendorAddProvider(provider func(entry, item uint32, maxCount int32, incrTime, extCost uint32)) {
	r.mu.Lock()
	r.config.VendorAddProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetVendorRemoveProvider(provider func(entry, item uint32) error) {
	r.mu.Lock()
	r.config.VendorRemoveProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetVendorRemoveAllProvider(provider func(entry uint32)) {
	r.mu.Lock()
	r.config.VendorRemoveAllProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetKickProvider(provider func(playerGUID uint64)) {
	r.mu.Lock()
	r.config.KickProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetBanProvider(provider func(args BanArgs) (int, string)) {
	r.mu.Lock()
	r.config.BanProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetSaveAllPlayersProvider(provider func()) {
	r.mu.Lock()
	r.config.SaveAllPlayersProvider = provider
	r.mu.Unlock()
}

func (r *Runtime) SetSendMailProvider(provider func(SendMailArgs) error) {
	r.mu.Lock()
	r.config.SendMailProvider = provider
	r.mu.Unlock()
}

func setPackagePath(state *lua.State, scriptPath string) {
	state.Global("package")
	if state.IsNil(-1) {
		state.Pop(1)
		return
	}
	state.PushString(filepath.Join(scriptPath, "?.lua") + ";" + filepath.Join(scriptPath, "?.ext") + ";" + filepath.Join(scriptPath, "extensions", "?.ext") + ";" + filepath.Join(scriptPath, "extensions", "?", "?.ext"))
	state.SetField(-2, "path")
	state.Pop(1)
}

func (r *Runtime) registerPlayerEvent(state *lua.State) int {
	return r.registerHook(state, "player", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerServerEvent(state *lua.State) int {
	return r.registerHook(state, "server", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerCreatureGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "creature_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerPlayerGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "player_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerGuildEvent(state *lua.State) int {
	return r.registerHook(state, "guild", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerGroupEvent(state *lua.State) int {
	return r.registerHook(state, "group", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerVehicleEvent(state *lua.State) int {
	return r.registerHook(state, "vehicle", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerBGEvent(state *lua.State) int {
	return r.registerHook(state, "bg", lua.CheckInteger(state, 1), 2, 3)
}

func (r *Runtime) registerAuraEvent(state *lua.State) int {
	return r.registerEntryHook(state, "aura", 1, 2, 3, 4)
}

func (r *Runtime) registerSpellEvent(state *lua.State) int {
	return r.registerEntryHook(state, "spell", 1, 2, 3, 4)
}

func (r *Runtime) clearAuraEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "aura")
}

func (r *Runtime) clearSpellEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "spell")
}

func (r *Runtime) registerCreatureEvent(state *lua.State) int {
	return r.registerEntryHook(state, "creature", 1, 2, 3, 4)
}

func (r *Runtime) registerUniqueCreatureEvent(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	instanceID := lua.CheckUnsigned(state, 2)
	event := lua.CheckInteger(state, 3)
	return r.registerHook(state, "creature_unique:"+strconv.FormatUint(guid, 10)+":"+strconv.FormatUint(uint64(instanceID), 10), event, 4, 5)
}

func (r *Runtime) registerGameObjectEvent(state *lua.State) int {
	return r.registerEntryHook(state, "gameobject", 1, 2, 3, 4)
}

func (r *Runtime) registerGameObjectGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "gameobject_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerItemEvent(state *lua.State) int {
	return r.registerEntryHook(state, "item", 1, 2, 3, 4)
}

func (r *Runtime) registerItemGossipEvent(state *lua.State) int {
	return r.registerEntryHook(state, "item_gossip", 1, 2, 3, 4)
}

func (r *Runtime) registerPacketEvent(state *lua.State) int {
	return r.registerEntryHook(state, "packet", 1, 2, 3, 4)
}

func (r *Runtime) registerMapEvent(state *lua.State) int {
	return r.registerEntryHook(state, "map", 1, 2, 3, 4)
}

func (r *Runtime) registerInstanceEvent(state *lua.State) int {
	return r.registerEntryHook(state, "instance", 1, 2, 3, 4)
}

func (r *Runtime) registerEntryHook(state *lua.State, kind string, entryIndex, eventIndex, functionIndex, shotsIndex int) int {
	entry := lua.CheckUnsigned(state, entryIndex)
	event := lua.CheckInteger(state, eventIndex)
	return r.registerHook(state, kind+":"+strconv.FormatUint(uint64(entry), 10), event, functionIndex, shotsIndex)
}

func (r *Runtime) registerHook(state *lua.State, kind string, event, functionIndex, shotsIndex int) int {
	if !state.IsFunction(functionIndex) {
		lua.ArgumentError(state, functionIndex, "function expected")
	}
	shots := 0
	if state.Top() >= shotsIndex && !state.IsNil(shotsIndex) {
		shots = lua.CheckInteger(state, shotsIndex)
		if shots < 0 {
			lua.ArgumentError(state, shotsIndex, "non-negative shots expected")
		}
	}
	ref := r.storeFunction(state, functionIndex)
	id := r.nextHook
	r.nextHook++
	r.hooks = append(r.hooks, registeredHook{Hook: Hook{Kind: kind, Event: event}, id: id, ref: ref, shots: shots})
	state.PushGoFunction(func(_ *lua.State) int {
		r.cancelHook(id)
		return 0
	})
	return 1
}

func (r *Runtime) clearBattleGroundEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "bg")
}
func (r *Runtime) clearGroupEvents(state *lua.State) int { return r.clearGlobalEvents(state, "group") }
func (r *Runtime) clearVehicleEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "vehicle")
}
func (r *Runtime) clearGuildEvents(state *lua.State) int { return r.clearGlobalEvents(state, "guild") }
func (r *Runtime) clearPlayerEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "player")
}
func (r *Runtime) clearServerEvents(state *lua.State) int {
	return r.clearGlobalEvents(state, "server")
}

func (r *Runtime) clearCreatureEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "creature")
}

func (r *Runtime) clearCreatureGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "creature_gossip")
}

func (r *Runtime) clearGameObjectEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "gameobject")
}

func (r *Runtime) clearGameObjectGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "gameobject_gossip")
}

func (r *Runtime) clearItemEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "item")
}

func (r *Runtime) clearItemGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "item_gossip")
}

func (r *Runtime) clearPacketEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "packet")
}

func (r *Runtime) clearPlayerGossipEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "player_gossip")
}

func (r *Runtime) clearMapEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "map")
}

func (r *Runtime) clearInstanceEvents(state *lua.State) int {
	return r.clearEntryEvents(state, "instance")
}

func (r *Runtime) clearUniqueCreatureEvents(state *lua.State) int {
	guid := checkLuaUint64(state, 1)
	instanceID := lua.CheckUnsigned(state, 2)
	prefix := "creature_unique:" + strconv.FormatUint(guid, 10) + ":" + strconv.FormatUint(uint64(instanceID), 10)
	event := optionalEvent(state, 3)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == prefix && (event < 0 || hook.Event == event) })
	return 0
}

func (r *Runtime) clearGlobalEvents(state *lua.State, kind string) int {
	event := optionalEvent(state, 1)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == kind && (event < 0 || hook.Event == event) })
	return 0
}

func (r *Runtime) clearEntryEvents(state *lua.State, kind string) int {
	entry := lua.CheckUnsigned(state, 1)
	prefix := kind + ":" + strconv.FormatUint(uint64(entry), 10)
	event := optionalEvent(state, 2)
	r.clearHooks(func(hook registeredHook) bool { return hook.Kind == prefix && (event < 0 || hook.Event == event) })
	return 0
}

func optionalEvent(state *lua.State, index int) int {
	if state.Top() < index || state.IsNil(index) {
		return -1
	}
	return lua.CheckInteger(state, index)
}

func (r *Runtime) clearHooks(predicate func(registeredHook) bool) {
	remaining := r.hooks[:0]
	for _, hook := range r.hooks {
		if predicate(hook) {
			r.cancelled[hook.id] = struct{}{}
			continue
		}
		remaining = append(remaining, hook)
	}
	r.hooks = remaining
}

func (r *Runtime) cancelHook(id int) {
	r.cancelled[id] = struct{}{}
	remaining := r.hooks[:0]
	for _, hook := range r.hooks {
		if hook.id != id {
			remaining = append(remaining, hook)
		}
	}
	r.hooks = remaining
}

func (r *Runtime) storeFunction(state *lua.State, index int) int {
	ref := r.nextRef
	r.nextRef++
	state.PushValue(index)
	state.RawSetInt(lua.RegistryIndex, ref)
	return ref
}

func (r *Runtime) createLuaEvent(state *lua.State) int {
	if !state.IsFunction(1) {
		lua.ArgumentError(state, 1, "function expected")
	}
	var minDelay, maxDelay int64
	if state.IsTable(2) {
		state.RawGetInt(2, 1)
		minDelay = int64(lua.CheckInteger(state, -1))
		state.Pop(1)
		state.RawGetInt(2, 2)
		maxDelay = int64(lua.CheckInteger(state, -1))
		state.Pop(1)
	} else {
		minDelay = int64(lua.CheckInteger(state, 2))
		maxDelay = minDelay
	}
	if minDelay < 1 || maxDelay < minDelay {
		lua.ArgumentError(state, 2, "invalid delay range")
	}
	repeats := 1
	if state.Top() >= 3 && state.IsNumber(3) {
		repeats = lua.CheckInteger(state, 3)
	}
	id := r.nextRef
	r.timers = append(r.timers, timer{id: id, ref: r.storeFunction(state, 1), minDelay: minDelay, maxDelay: maxDelay, delay: timerDelay(minDelay, maxDelay), repeats: repeats})
	state.PushInteger(id)
	return 1
}

func timerDelay(minDelay, maxDelay int64) int64 {
	if maxDelay <= minDelay {
		return minDelay
	}
	return minDelay + rand.Int63n(maxDelay-minDelay+1)
}

func (r *Runtime) removeEvent(state *lua.State) int {
	id := lua.CheckInteger(state, 1)
	for index, current := range r.timers {
		if current.id == id {
			r.timers = append(r.timers[:index], r.timers[index+1:]...)
			break
		}
	}
	return 0
}

func (r *Runtime) removeEvents(state *lua.State) int {
	r.timers = nil
	return 0
}

func (r *Runtime) charDBQuery(state *lua.State) int  { return r.dbQuery(state, r.config.CharacterDB) }
func (r *Runtime) worldDBQuery(state *lua.State) int { return r.dbQuery(state, r.config.WorldDatabase) }
func (r *Runtime) authDBQuery(state *lua.State) int  { return r.dbQuery(state, r.config.AuthDatabase) }
func (r *Runtime) charDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.CharacterDB)
}
func (r *Runtime) worldDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.WorldDatabase)
}
func (r *Runtime) authDBExecute(state *lua.State) int {
	return r.dbExecute(state, r.config.AuthDatabase)
}

func (r *Runtime) dbQuery(state *lua.State, db *sql.DB) int {
	sqlText := lua.CheckString(state, 1)
	query, err := queryDatabase(db, sqlText)
	if err != nil {
		if r.config.Logger != nil {
			r.config.Logger.Error("lua database query failed", "error", err)
		}
		state.PushNil()
		return 1
	}
	if query == nil {
		state.PushNil()
		return 1
	}
	pushQuery(state, query)
	return 1
}

func (r *Runtime) dbExecute(state *lua.State, db *sql.DB) int {
	sqlText := lua.CheckString(state, 1)
	if err := executeDatabase(db, sqlText); err != nil && r.config.Logger != nil {
		r.config.Logger.Error("lua database execute failed", "error", err)
	}
	return 0
}
