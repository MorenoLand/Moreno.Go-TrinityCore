-- Darkshore: Prospector Remtravel — Lua port of
-- src/server/scripts/Kalimdor/zone_darkshore.cpp
-- (class npc_prospector_remtravel : CreatureScript("npc_prospector_remtravel")).
-- Entry: 2917 (wowhead npc=2917/prospector-remtravel, verified
-- 2026-11-07; quest 731 "The Absent Minded Prospector" lists him as Start;
-- no NPC_ constant exists in the C++ file).
-- Eluna creature events: 31 OnQuestAccept, 4 JustDied.
-- Ported arms:
-- - OnQuestAccept (zone_darkshore.cpp:283-290, event 31): quest id
--   731 (QUEST_ABSENT_MINDED_PT2) -> SetFaction(10
--   FACTION_ESCORTEE_A_NEUTRAL_PASSIVE) + StartEscort(player).
-- - JustDied (zone_darkshore.cpp:292-296, event 4): if escorting,
--   the escort player fails quest 731 (muglash / injured-goblin
--   precedent; IsEscorted + GetEscortPlayer bridges live,
--   lua_creature_events.go:2652/:2691).
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached 0-42 (Talk progress lines + GroupEventHappens
--   credit at waypoint 42): the escort walk rides MoveWaypoint +
--   the ev6 arrival event over DB waypoint_data rows — unported,
--   so the arms never fire in the Go model; the quest-accept start
--   latches escort state only.

local ENTRY_PROSPECTOR_REMTRAVEL = 2917

local QUEST_ABSENT_MINDED_PT2 = 731

local FACTION_ESCORTEE_A_NEUTRAL_PASSIVE = 10

-- C++ OnQuestAccept for quest 731: SetFaction(10) + Start escort.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ABSENT_MINDED_PT2 then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_A_NEUTRAL_PASSIVE)
    creature:StartEscort(player)
end

-- C++ JustDied (zone_darkshore.cpp:292-296): if escorting, the
-- escort player fails QUEST_ABSENT_MINDED_PT2.
local function onDied(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player then
        player:FailQuest(QUEST_ABSENT_MINDED_PT2)
    end
end

RegisterCreatureEvent(ENTRY_PROSPECTOR_REMTRAVEL, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_PROSPECTOR_REMTRAVEL, 4, onDied)
