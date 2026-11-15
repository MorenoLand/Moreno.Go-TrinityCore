-- Darkshore: Prospector Remtravel — Lua port of
-- src/server/scripts/Kalimdor/zone_darkshore.cpp
-- (class npc_prospector_remtravel : CreatureScript("npc_prospector_remtravel")).
-- Entry: 2917 (wowhead npc=2917/prospector-remtravel, verified
-- 2026-11-07; quest 731 "The Absent Minded Prospector" lists him as Start;
-- no NPC_ constant exists in the C++ file).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 4 JustDied.
-- Ported arms:
-- - OnQuestAccept (zone_darkshore.cpp:283-290, event 31): quest id
--   731 (QUEST_ABSENT_MINDED_PT2) -> SetFaction(10
--   FACTION_ESCORTEE_A_NEUTRAL_PASSIVE) + StartEscort(player) +
--   MoveWaypoint() (the walk rides MoveWaypoint — StartEscort only
--   latches the player, ruul precedent).
-- - WaypointReached 0-42 (zone_darkshore.cpp:222-282, event 6,
--   onReachWP below) — PORTED 2026-11-15: all Talk arms (player-
--   targeted, whole switch gated on GetPlayerForEscort()),
--   DoSpawnCreature gravel legs at wp6/16/29 (SummonCreature with
--   raw-added offsets, halazzi/janalai precedent; type 3
--   TIMED_DESPAWN_OUT_OF_COMBAT, 30s, orientation 0 per C++) and
--   the wp42 GroupEventHappens(731) credit + StopEscort() (path
--   end; Go paths loop by default, so without the halt the spawns
--   and credit would re-fire every loop).
-- - JustDied (zone_darkshore.cpp:292-296, event 4): if escorting,
--   the escort player fails quest 731 (muglash / injured-goblin
--   precedent; IsEscorted + GetEscortPlayer bridges live,
--   lua_creature_events.go:2652/:2691).

local ENTRY_PROSPECTOR_REMTRAVEL = 2917

local QUEST_ABSENT_MINDED_PT2 = 731

local FACTION_ESCORTEE_A_NEUTRAL_PASSIVE = 10

local SAY_REM_START = 0
local SAY_REM_RAMP1_1 = 2
local SAY_REM_RAMP1_2 = 3
local SAY_REM_BOOK = 4
local SAY_REM_TENT1_1 = 5
local SAY_REM_TENT1_2 = 6
local SAY_REM_MOSS = 7
local EMOTE_REM_MOSS = 8
local SAY_REM_MOSS_PROGRESS = 9
local SAY_REM_PROGRESS = 10
local SAY_REM_REMEMBER = 11
local EMOTE_REM_END = 12

local NPC_GRAVEL_SCOUT = 2158
local NPC_GRAVEL_BONE = 2159
local NPC_GRAVEL_GEO = 2160

local SUMMON_TYPE_TIMED_DESPAWN_OOC = 3
local SUMMON_DESPAWN_MS = 30000

-- C++ OnQuestAccept for quest 731: SetFaction(10) + Start escort +
-- MoveWaypoint() (the walk rides MoveWaypoint — StartEscort only latches
-- the player, ruul precedent).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ABSENT_MINDED_PT2 then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_A_NEUTRAL_PASSIVE)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ DoSpawnCreature(entry, dx, dy, 0, 0, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_
-- COMBAT, 30s): offsets are raw-added to the escort's position
-- (ScriptedCreature.cpp:286-289, halazzi/janalai precedent).
local function spawnGravel(creature, entry, dx, dy)
    creature:SummonCreature(entry, creature:GetX() + dx, creature:GetY() + dy,
        creature:GetZ(), 0, SUMMON_TYPE_TIMED_DESPAWN_OOC, SUMMON_DESPAWN_MS)
end

-- C++ WaypointReached (zone_darkshore.cpp:222-282), whole switch gated on
-- GetPlayerForEscort(): wp0/5/9/14/15/17/26/27/28/31/41 Talks (all
-- player-targeted); wp6/16 spawn scout+bone at (-10, 5)/(-10, 7); wp29
-- spawns scout+bone+geo at (-15, 3)/(-15, 5)/(-15, 7); wp42 Talk(12) +
-- GroupEventHappens(731) + StopEscort() (path end; Go paths loop).
local function onReachWP(_, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_REM_START, player)
    elseif wp == 5 then
        creature:Talk(SAY_REM_RAMP1_1, player)
    elseif wp == 6 then
        spawnGravel(creature, NPC_GRAVEL_SCOUT, -10.0, 5.0)
        spawnGravel(creature, NPC_GRAVEL_BONE, -10.0, 7.0)
    elseif wp == 9 then
        creature:Talk(SAY_REM_RAMP1_2, player)
    elseif wp == 14 then
        creature:Talk(SAY_REM_BOOK, player)
    elseif wp == 15 then
        creature:Talk(SAY_REM_TENT1_1, player)
    elseif wp == 16 then
        spawnGravel(creature, NPC_GRAVEL_SCOUT, -10.0, 5.0)
        spawnGravel(creature, NPC_GRAVEL_BONE, -10.0, 7.0)
    elseif wp == 17 then
        creature:Talk(SAY_REM_TENT1_2, player)
    elseif wp == 26 then
        creature:Talk(SAY_REM_MOSS, player)
    elseif wp == 27 then
        creature:Talk(EMOTE_REM_MOSS, player)
    elseif wp == 28 then
        creature:Talk(SAY_REM_MOSS_PROGRESS, player)
    elseif wp == 29 then
        spawnGravel(creature, NPC_GRAVEL_SCOUT, -15.0, 3.0)
        spawnGravel(creature, NPC_GRAVEL_BONE, -15.0, 5.0)
        spawnGravel(creature, NPC_GRAVEL_GEO, -15.0, 7.0)
    elseif wp == 31 then
        creature:Talk(SAY_REM_PROGRESS, player)
    elseif wp == 41 then
        creature:Talk(SAY_REM_REMEMBER, player)
    elseif wp == 42 then
        creature:Talk(EMOTE_REM_END, player)
        player:GroupEventHappens(QUEST_ABSENT_MINDED_PT2, creature)
        creature:StopEscort()
    end
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
RegisterCreatureEvent(ENTRY_PROSPECTOR_REMTRAVEL, 6, onReachWP)
RegisterCreatureEvent(ENTRY_PROSPECTOR_REMTRAVEL, 4, onDied)
