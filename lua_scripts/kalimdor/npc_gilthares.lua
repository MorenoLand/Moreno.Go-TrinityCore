-- The Barrens: Gilthares Firebough — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/Kalimdor/zone_the_barrens.cpp
-- (class npc_gilthares : CreatureScript("npc_gilthares");
-- struct npc_giltharesAI : public EscortAI).
-- Entry: 3465 (TDB 335.26091 creature_template ScriptName
-- npc_gilthares; creature_text groups 0-7).
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, zone_the_barrens.cpp:73-105), all under
-- the GetPlayerForEscort() nil-guard (Lua twin: GetEscortPlayer()):
-- - wp16 Talk(SAY_GIL_AT_LAST 1, player).
-- - wp17 Talk(SAY_GIL_PROCEED 2, player).
-- - wp18 Talk(SAY_GIL_FREEBOOTERS 3, player).
-- - wp37 Talk(SAY_GIL_ALMOST 5, player).
-- - wp47 Talk(SAY_GIL_SWEET 6, player).
-- - wp53 Talk(SAY_GIL_FREED 7, player) +
--   player:GroupEventHappens(QUEST_FREE_FROM_HOLD 898, me)
--   (bridged, lua_player.go:5336).
-- Unported scope: the OnQuestAccept driver (SetFaction, stand,
-- Talk SAY_GIL_START 0, Start) + JustEngagedWith aggro Talk
-- (SAY_GIL_AGGRO 4) — the escort driver, so this handler fires
-- once that lands.

local ENTRY_GILTHARES = 3465

local SAY_GIL_AT_LAST = 1
local SAY_GIL_PROCEED = 2
local SAY_GIL_FREEBOOTERS = 3
local SAY_GIL_ALMOST = 5
local SAY_GIL_SWEET = 6
local SAY_GIL_FREED = 7

local QUEST_FREE_FROM_HOLD = 898

-- C++ WaypointReached (zone_the_barrens.cpp:73-105).
local function giltharesReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 16 then
        creature:Talk(SAY_GIL_AT_LAST, player)
    elseif wp == 17 then
        creature:Talk(SAY_GIL_PROCEED, player)
    elseif wp == 18 then
        creature:Talk(SAY_GIL_FREEBOOTERS, player)
    elseif wp == 37 then
        creature:Talk(SAY_GIL_ALMOST, player)
    elseif wp == 47 then
        creature:Talk(SAY_GIL_SWEET, player)
    elseif wp == 53 then
        creature:Talk(SAY_GIL_FREED, player)
        player:GroupEventHappens(QUEST_FREE_FROM_HOLD, creature)
    end
end

RegisterCreatureEvent(ENTRY_GILTHARES, 6, giltharesReachWP)
