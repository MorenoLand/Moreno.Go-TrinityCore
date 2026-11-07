-- Sholazar Basin: Engineer Helice — Lua port of
-- src/server/scripts/Northrend/zone_sholazar_basin.cpp
-- (class npc_engineer_helice : CreatureScript("npc_engineer_helice")).
-- Entry: 28787 (wowhead npc=28787/engineer-helice, verified
-- 2026-11-07; starts quest 12688 "Engineering a Disaster";
-- no NPC_ constant exists in the C++ file).
-- Eluna creature events: 31 OnQuestAccept, 4 JustDied.
-- Ported arms:
-- - OnQuestAccept (zone_sholazar_basin.cpp:141-149, event 31):
--   quest id 12688 (QUEST_DISASTER) -> SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE) + StartEscort(player) +
--   Talk(SAY_WP_1 0). The C++ MoveJumpTo hop has no bridge.
-- - JustDied (zone_sholazar_basin.cpp:116-122, event 4): if
--   escorting, the escort player fails quest 12688 (injured-goblin
--   precedent; IsEscorted + GetEscortPlayer bridges live,
--   lua_creature_events.go:2328/:2361).
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached 9/13 (Talk + GroupEventHappens credit) and the
--   12s UpdateAI chat timer: the escort walk rides MoveWaypoint +
--   the ev6 arrival event over DB waypoint_data rows — unported.

local ENTRY_ENGINEER_HELICE = 28787

local QUEST_DISASTER = 12688

local SAY_WP_1 = 0

local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

-- C++ OnQuestAccept for quest 12688: SetFaction(113) + Start
-- escort + Talk(0).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_DISASTER then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_PASSIVE)
    creature:StartEscort(player)
    creature:Talk(SAY_WP_1)
end

-- C++ JustDied (zone_sholazar_basin.cpp:116-122): if escorting,
-- the escort player fails QUEST_DISASTER.
local function onDied(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player then
        player:FailQuest(QUEST_DISASTER)
    end
end

RegisterCreatureEvent(ENTRY_ENGINEER_HELICE, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_ENGINEER_HELICE, 4, onDied)
