-- Wounded Blood Elf Pilgrim (Hellfire Peninsula, "The Road to Falcon
-- Watch" escort) — Lua port of npc_wounded_blood_elfAI in
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp (:260-331).
-- Entry 16993 TDB-verified (TDB_full_world_335.26091_2026_09_09.sql:
-- 'Wounded Blood Elf Pilgrim', faction 1604, npcflag 2 questgiver,
-- ScriptName '' — the geezle/webbed entry-binding precedent: Lua
-- binds by entry). Quest 9375 'The Road to Falcon Watch'
-- (creature_queststarter 16993; ender 17015 Taleris Dawngazer). Talk
-- groups 0-5 all present in TDB creature_text, C++ ids verbatim.
-- One static spawn: guid 59383, map 530 (-1137.73, 4272.1, 14.05).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 19
-- OnJustSummonedCreature, 1 OnEnterCombat, 2 OnLeaveCombat.
-- Ported arms (C++-exact, zone_hellfire_peninsula.cpp:260-331):
-- - OnQuestAccept (31): quest 9375 match -> SetFaction(775
--   FACTION_ESCORTEE_H_PASSIVE, SharedDefines.h:252) +
--   StartEscort(player) + MoveWaypoint() (the C++ Start(true,
--   false, playerGUID) walk) + SetNPCFlags(0) (the
--   EscortAI::Start UNIT_NPC_FLAGS clear, ScriptedEscortAI.cpp;
--   nefarian SetNPCFlags(0) precedent).
-- - JustEngagedWith (1): IsEscorted() (the HasEscortState
--   (STATE_ESCORT_ESCORTING) twin) -> Talk(AGGRO 5) + combat pause
--   SetEscortPaused(true) (C++ EscortAI advances waypoints only
--   while !engaged; nagrand-captive precedent).
-- - LeaveCombat (2): SetEscortPaused(false) while escorted.
-- - JustSummoned (19): summoned:AttackStart(me) (C++-verbatim,
--   no entry filter — only the four escort summons ever summon).
-- - WaypointReached (6, DB point via GetCurrentWaypointId): wp9
--   Talk(SUMMON1 1) + 2x Talonguard 16967 at (-15,-15) /
--   (-17,-17) relative, wp13 Talk(RESTING 2), wp14 Talk(SUMMON2 3)
--   + 2x Windwalker 16966 same offsets, wp27 Talk(COMPLETE 4) +
--   GroupEventHappens(9375). Summons: type 3
--   TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 5000ms (C++ 5s).
--   wp36 StopEscort: the C++ path-end MoveIdle + drop-ESCORTING
--   twin (Go's NextIdx wraps, nagrand-captive precedent).
-- Deliberate deviations / blockers (all data-side, no engine work):
-- - C++ case 0 Talk(START 0) NEVER fires in C++ either: the TDB
--   path (waypoint_data 135945, pts 1-36, coords identical to the
--   25101 script_waypoint rows for 16993) has no point 0, and
--   EscortAI fires WaypointReached(waypoint.id) per node — so the
--   START line is dead in both engines; no wp0 arm is ported.
-- - Reset is empty in C++ ({}); no ev23 registration.
-- - No C++ JustDied: the quest-fail-on-death leg lives only in
--   the TDB SmartAI row (On Death -> Fail Quest 9375), which Go
--   does not run — not ported (C++-exact doctrine; the SmartAI
--   timed actionlists 1699300-1699303 implement the same escort
--   with different waypoint triggers 12/18/36 and are DB content,
--   not C++ code).
-- - TDB binds the walk via sql/*/updates/world/
--   2026111403_wounded_blood_elf_pilgrim_path_binding.sql
--   (creature_template_addon.path_id 16993 -> 135945; template
--   path_id is 0 and no creature_addon row exists for spawn guid
--   59383, so loadCreaturePathID falls back to the guid and
--   MoveWaypoint() is a no-op until the update applies —
--   nagrand-captive precedent).

local ENTRY_WOUNDED_BLOOD_ELF = 16993

local QUEST_ROAD_TO_FALCON_WATCH = 9375

local NPC_HAALESHI_WINDWALKER = 16966
local NPC_HAALESHI_TALONGUARD = 16967

local SAY_SUMMON1 = 1
local SAY_RESTING = 2
local SAY_SUMMON2 = 3
local SAY_COMPLETE = 4
local SAY_AGGRO = 5

local FACTION_ESCORTEE_H_PASSIVE = 775

local SUMMON_TYPE_OUT_OF_COMBAT = 3
local SUMMON_DESPAWN_MS = 5000

-- C++ OnQuestAccept (zone_hellfire_peninsula.cpp:280-287), C++
-- order: SetFaction, Start, (Start clears UNIT_NPC_FLAGS).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ROAD_TO_FALCON_WATCH then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_H_PASSIVE)
    creature:StartEscort(player)
    creature:SetNPCFlags(0)
    creature:MoveWaypoint()
end

-- C++ JustEngagedWith (zone_hellfire_peninsula.cpp:271-275) +
-- the combat pause (nagrand-captive precedent).
local function onEnterCombat(event, creature)
    if creature:IsEscorted() then
        creature:Talk(SAY_AGGRO)
        creature:SetEscortPaused(true)
    end
end

-- C++ EscortAI resumes waypoint advance on leave-combat.
local function onLeaveCombat(event, creature)
    if creature:IsEscorted() then
        creature:SetEscortPaused(false)
    end
end

-- C++ JustSummoned (zone_hellfire_peninsula.cpp:277-280).
local function onJustSummoned(event, creature, summoned)
    if summoned then
        summoned:AttackStart(creature)
    end
end

-- C++ WaypointReached (zone_hellfire_peninsula.cpp:289-325).
local function onReachWP(event, creature)
    local wp = creature:GetCurrentWaypointId()
    if wp == 9 then
        creature:Talk(SAY_SUMMON1)
        creature:SummonCreature(NPC_HAALESHI_TALONGUARD,
            creature:GetX() - 15, creature:GetY() - 15, creature:GetZ(),
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
        creature:SummonCreature(NPC_HAALESHI_TALONGUARD,
            creature:GetX() - 17, creature:GetY() - 17, creature:GetZ(),
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
    elseif wp == 13 then
        creature:Talk(SAY_RESTING)
    elseif wp == 14 then
        creature:Talk(SAY_SUMMON2)
        creature:SummonCreature(NPC_HAALESHI_WINDWALKER,
            creature:GetX() - 15, creature:GetY() - 15, creature:GetZ(),
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
        creature:SummonCreature(NPC_HAALESHI_WINDWALKER,
            creature:GetX() - 17, creature:GetY() - 17, creature:GetZ(),
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
    elseif wp == 27 then
        creature:Talk(SAY_COMPLETE)
        local player = creature:GetEscortPlayer()
        if player then
            player:GroupEventHappens(QUEST_ROAD_TO_FALCON_WATCH, creature)
        end
    elseif wp == 36 then
        -- C++ path end: EscortAI drops ESCORTING and idles
        -- (MoveIdle); without this Go's NextIdx wraps and the
        -- escort loops the path.
        creature:StopEscort()
    end
end

RegisterCreatureEvent(ENTRY_WOUNDED_BLOOD_ELF, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_WOUNDED_BLOOD_ELF, 6, onReachWP)
RegisterCreatureEvent(ENTRY_WOUNDED_BLOOD_ELF, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_WOUNDED_BLOOD_ELF, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WOUNDED_BLOOD_ELF, 2, onLeaveCombat)
