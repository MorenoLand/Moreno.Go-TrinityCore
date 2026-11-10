-- Hellfire Peninsula: Colonel Jules — Lua port of
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp
-- (class npc_colonel_jules : CreatureScript("npc_colonel_jules")).
-- Entry: 22432 (C++ ExorcismMisc enum NPC_COLONEL_JULES;
-- wowhead npc=22432/colonel-jules confirms).
-- Eluna creature events: 23 OnReset, 38 DoAction; gossip event 1
-- OnGossipHello.
-- Ported arms:
-- - DoAction ACTION_JULES_MOVE_HOME (4)
--   (zone_hellfire_peninsula.cpp:461-467, event 38): SetSpeedRate
--   (MOVE_RUN 1, 1.0) + MoveTo(11, exorcismPos[2] -710.332,
--   2754.394, 102.948). Bridges: DoAction receiver (event 38,
--   lua_creature_events.go:827), SetSpeedRate
--   (lua_creature_events.go:5124, MOVE_RUN=1 only), MoveTo
--   (lua_creature_events.go:4624, id + x/y/z).
-- - OnGossipHello (zone_hellfire_peninsula.cpp:525-532, gossip
--   event 1): when the exorcism `success` latch is set (Barada
--   step 21), KilledMonsterCredit(NPC_COLONEL_JULES 22432) — the
--   quest-10935 completion credit — then the gossip menu.
--   Bridges: KilledMonsterCredit (lua_player.go:4541), GossipSendMenu
--   + GetGossipTextId (kodo precedent). The latch lives in the
--   shared table below: all lua_scripts files load into ONE Lua
--   state (engine/scripting/runtime.go LoadScripts), so Barada's
--   step-21/EVENT_RESET writes land here; Reset (event 23) clears
--   it per C++ Reset -> Initialize (success = false).
-- Unmodeled (documented-only): the C++ arm also clears the
-- member bool wpreached and cancels EVENT_SUMMON_SKULL — both
-- are vacuous here (no Jules timers exist in this file; the skull
-- summon machine is unported). The ACTION_JULES_HOVER (2) and
-- ACTION_JULES_FLIGHT (3) arms stay out, but their bridges are
-- LIVE, not absent as this header used to claim: AddAura
-- (lua_creature_events.go:1408), SetCanFly (:3646), MoveJump
-- (:5317), SummonCreature (anzu precedent), SetSpeedRate, MoveTo
-- — the hover/flight + skull-summon + waypoint-chain machine is a
-- queued port, not a stay-out.

-- Shared with npc_barada.lua: per-Jules-GUID exorcism success latch.
ColonelJulesSuccess = ColonelJulesSuccess or {}

local ENTRY_COLONEL_JULES = 22432

local ACTION_JULES_MOVE_HOME = 4

local MOVE_RUN = 1

-- C++ DoAction case ACTION_JULES_MOVE_HOME: run-speed reset +
-- move back to the exorcism bed position.
local function onDoAction(event, creature, action)
    if action ~= ACTION_JULES_MOVE_HOME then
        return
    end
    creature:SetSpeedRate(MOVE_RUN, 1.0)
    creature:MoveTo(11, -710.332, 2754.394, 102.948)
end

-- C++ Reset -> Initialize (zone_hellfire_peninsula.cpp:417-429):
-- the exorcism success latch clears on reset.
local function onReset(event, creature)
    ColonelJulesSuccess[creature:GetGUIDLow()] = nil
end

-- C++ OnGossipHello (zone_hellfire_peninsula.cpp:525-532).
local function julesGossipHello(event, player, creature)
    if ColonelJulesSuccess[creature:GetGUIDLow()] then
        player:KilledMonsterCredit(ENTRY_COLONEL_JULES)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

RegisterCreatureEvent(ENTRY_COLONEL_JULES, 23, onReset)
RegisterCreatureEvent(ENTRY_COLONEL_JULES, 38, onDoAction)
RegisterCreatureGossipEvent(ENTRY_COLONEL_JULES, 1, julesGossipHello)
