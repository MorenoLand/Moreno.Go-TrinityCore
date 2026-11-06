-- Violet Hold — Lua port of
-- src/server/scripts/Northrend/VioletHold/violet_hold.cpp
-- (spell_violet_hold_portal_periodic only; the rest of the file's
-- creature/instance scripts are not ported here).

-- C++ spell_violet_hold_portal_periodic_AuraScript::PeriodicTick
-- (violet_hold.cpp:1410-1420, aura 58008): PreventDefaultAction;
-- the portal's AI gets SetData(DATA_PORTAL_PERIODIC_TICK 1,
-- tickNumber). creature:SetData is the CreatureAI::SetData
-- virtual-dispatch twin (lua_creature_events.go:767 — the
-- per-creature ScriptData store, each handler interpreting its own
-- types). Returns true, the PreventDefaultAction twin, so the Go
-- default trigger cast does not also fire.
-- LATENT: the consumer — npc_violet_hold_teleportation_portalAI::
-- SetData (:737-772, tick 1 summons a portal guardian/keeper +
-- Sinclari Talk, later ticks summon 3-4 azure invaders) — is
-- unported (instance GetData(DATA_WAVE_COUNT) / GetCreature
-- (DATA_SINCLARI_TRIGGER) / DoSummon legs have no bridges), so
-- this write sits in ScriptData until the portal AI is ported.
-- AddSC_violet_hold audit (Violet Hold strip): nothing else in
-- violet_hold.cpp is real and bridgeable — the zone closes
-- documented-only beyond the portal-periodic port above plus the
-- already-ported trash (violet_hold_trash.lua), defense system
-- (npc_violet_hold_defense_system.lua) and activation crystal
-- (npc_violet_hold_activation_crystal.lua):
-- - npc_sinclari_vh OnGossipHello (GetData(DATA_MAIN_EVENT_STATE)
--   menu branch): no instance GetData bridge — stays out.
-- - npc_sinclari_vh OnGossipSelect (START_ENCOUNTER 9998 list 0 ->
--   SetData MAIN_EVENT_STATE SPECIAL + ScheduleIntro; SEND_ME_IN
--   10275 list 0 -> SPELL_TELEPORT_PLAYER 62138 on the player):
--   the select hook IS bridged (RegisterCreatureGossipEvent, listID
--   == 0 gates port directly) but menuID is intentionally NOT
--   forwarded (engine/world/gossip.go:360-381 — the post-hook
--   stale-menu gate enforces it server-side), and both options are
--   list 0 in different menus, so a Lua handler cannot tell them
--   apart — porting would conflate start-encounter with teleport.
--   The ScheduleIntro legs (MovePoint, guard grid sweep, main-door
--   GO state) have no bridges either.
-- - npc_azure_saboteur (Reset 2s StartMovement, MovementInform
--   SHIELD_DISRUPTION x3 + SetData DATA_START_BOSS_ENCOUNTER +
--   TELEPORT_VISUAL + despawn): no MoveSmoothPath / MovementInform
--   bridges — stays out.
-- - npc_violet_hold_teleportation_portal(_elite/_intro) AIs
--   (InitializeAI SPELL_PORTAL_PERIODIC 58008 self-cast, tick-1
--   guardian/keeper summon + Sinclari Talks, elite 15s azure
--   summons, intro 15s trickle): gated on instance GetData
--   (DATA_WAVE_COUNT / DATA_MAIN_EVENT_STATE) and GetCreature
--   (DATA_SINCLARI_TRIGGER) — no bridges; the periodic self-cast
--   alone would only feed the unported SetData tick machine (the
--   gothik bubble-phase precedent) — stays out.
-- - spell_violet_hold_destroy_door_seal (58040 AuraScript periodic:
--   GetData(DATA_DOOR_INTEGRITY) - 1 -> SetData): no instance
--   GetData/SetData bridge — stays out.
-- - spell_violet_hold_teleport_player (62138 SpellScript: hit unit
--   self-casts SPELL_TELEPORT_PLAYER_EFFECT 62139, triggered): no
--   SpellScript bridge — stays out.
local SPELL_PORTAL_PERIODIC = 58008
local DATA_PORTAL_PERIODIC_TICK = 1

local function portalPeriodicTick(event, aura, target)
    target:SetData(DATA_PORTAL_PERIODIC_TICK, aura:GetTickNumber())
    return true
end

RegisterAuraEvent(SPELL_PORTAL_PERIODIC, 1, portalPeriodicTick)
