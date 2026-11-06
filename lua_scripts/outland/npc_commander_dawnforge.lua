-- npc_commander_dawnforge + at_commander_dawnforge (Netherstorm zone script)
-- Lua port of src/server/scripts/Outland/zone_netherstorm.cpp
-- (npc_commander_dawnforgeAI — ScriptedAI, registered by
-- AddSC_netherstorm in outland_script_loader.cpp).
-- Entries are the file's own CreatureEntry table (:44-50):
-- 19830 Arcanist Ardonis, 19831 Commander Dawnforge,
-- 21504 Pathaleon the Calculator's Image. Quest 10198
-- "Information Gathering", SPELL_SUNFURY_DISGUISE 34603.
-- Talk IDs from the file's CommanderDawnforgeData enum:
-- Dawnforge 0-4, Ardonis 0-1, Pathaleon 0-3.
-- AreaTrigger handler: RegisterServerEvent(24, fn) receives
-- (event, player, triggerID); the C++ OnTrigger ignores its
-- `at` param, so this handler is trigger-ID-agnostic too — the
-- aura + quest + 30yd Dawnforge gates confine firing.
-- Eluna creature events: 5 OnSpawn, 23 OnReset (state clear).
-- JustEngagedWith is empty in C++; melee is engine-driven.
-- Ported arms (C++-exact):
-- - AreaTrigger (zone_netherstorm.cpp:310-330): player HasAura
--   (34603), IsAlive, quest 10198 INCOMPLETE (3), nearest
--   Dawnforge within 30yd, then CanStartEvent: Ardonis within
--   10yd or abort, per-GUID event latch, Turn_to_eachother
--   immediately. First phase tick after the C++ Initialize
--   Phase_Timer of 4000ms.
-- - Phase pump (UpdateAI :193-307): 500ms CreateLuaEvent
--   chain carrying Phase / PhaseSubphase / Phase_Timer exactly
--   (P1 Dawnforge Talk 0, 16000; P2 Ardonis Talk 0, 16000;
--   P3 Dawnforge Talk 1, 16000; P4 SummonCreature Pathaleon
--   at (2325.851563, 2799.534668, 133.084229, 6.038996),
--   TIMED_DESPAWN (2), 90s, 500; P5 Pathaleon Talk 0, 6000;
--   P6 turn-to-image + kneel, Dawnforge Talk 2, 8000/8000;
--   P7 Pathaleon Talk 1/2/3, 12000/16000/10000; P8 Dawnforge
--   Talk 3 + Ardonis Talk 1, 4000; P9 turn-to-eachother +
--   Pathaleon SetVisible(false), 3000; P10 Dawnforge Talk 4
--   + player AreaExploredOrEventHappens(10198), Reset).
-- - Reset guards (UpdateAI :205-216): Ardonis or player gone
--   -> Reset; Phase > 4 and Pathaleon gone -> Reset.
-- - Turn_to_Pathaleons_Image (:146-164): Dawnforge + Ardonis
--   face Pathaleon, SetStandState KNEEL (8, UnitDefines.h:41);
--   Turn_to_eachother (:167-183): face each other, STAND (0).
-- Deviations (documented): Pathaleon is held from the
-- SummonCreature return value instead of the C++ JustSummoned
-- GUID latch (equivalent — same summon, same object); the
-- event latch is per-Dawnforge-GUID; the pump is a
-- CreateLuaEvent chain rather than the C++ UpdateAI countdown
-- (all C++ timers are multiples of 500ms, so phase order and
-- spacing match).

local ENTRY_ARCANIST_ARDONIS = 19830
local ENTRY_COMMANDER_DAWNFORGE = 19831
local ENTRY_PATHALEON_IMAGE = 21504

local QUEST_INFO_GATHERING = 10198
local SPELL_SUNFURY_DISGUISE = 34603

local QUEST_STATUS_INCOMPLETE = 3

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_KNEEL = 8

local SUMMON_TYPE_TIMED_DESPAWN = 2

local timers = {}
local states = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function resetEvent(creature, guid)
    states[guid] = nil
    cancelTimers(guid)
end

-- C++ Turn_to_eachother (:167-183).
local function turnToEachother(creature, state)
    local ardonis = state.ardonis
    if not ardonis then
        return
    end
    creature:SetFacingToObject(ardonis)
    ardonis:SetFacingToObject(creature)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    ardonis:SetStandState(UNIT_STAND_STATE_STAND)
end

-- C++ Turn_to_Pathaleons_Image (:146-164).
local function turnToPathaleonsImage(creature, state)
    local ardonis = state.ardonis
    local pathaleon = state.pathaleon
    if not ardonis or not pathaleon then
        return
    end
    creature:SetFacingToObject(pathaleon)
    ardonis:SetFacingToObject(pathaleon)
    creature:SetStandState(UNIT_STAND_STATE_KNEEL)
    ardonis:SetStandState(UNIT_STAND_STATE_KNEEL)
end

-- C++ UpdateAI (:193-307): one phase per tick, Phase_Timer
-- carried as the next schedule delay.
local function phaseTick(creature, guid)
    local state = states[guid]
    if not state or not state.isEvent then
        return
    end
    -- C++ re-resolves Ardonis via its GUID every tick (UpdateAI
    -- :199-200) and Resets if gone; re-resolve by entry+range
    -- here (same 10yd as CanStartEvent).
    local ardonis = creature:FindNearestCreature(ENTRY_ARCANIST_ARDONIS, 10)
    local pathaleon = state.pathaleon
    local player = GetPlayerByGUID(state.playerGuid)
    if not ardonis or not player then
        resetEvent(creature, guid)
        return
    end
    state.ardonis = ardonis
    if state.phase > 4 and not pathaleon then
        resetEvent(creature, guid)
        return
    end
    local function next(delay)
        schedule(guid, "phase", delay, function()
            phaseTick(creature, guid)
        end)
    end
    if state.phase == 1 then
        creature:Talk(0)
        state.phase = 2
        next(16000)
    elseif state.phase == 2 then
        ardonis:Talk(0)
        state.phase = 3
        next(16000)
    elseif state.phase == 3 then
        creature:Talk(1)
        state.phase = 4
        next(16000)
    elseif state.phase == 4 then
        local summoned = creature:SummonCreature(ENTRY_PATHALEON_IMAGE,
            2325.851563, 2799.534668, 133.084229, 6.038996,
            SUMMON_TYPE_TIMED_DESPAWN, 90000)
        if summoned then
            state.pathaleon = summoned
        end
        state.phase = 5
        next(500)
    elseif state.phase == 5 then
        pathaleon:Talk(0)
        state.phase = 6
        next(6000)
    elseif state.phase == 6 then
        if state.subphase == 0 then
            turnToPathaleonsImage(creature, state)
            state.subphase = 1
            next(8000)
        else
            creature:Talk(2)
            state.subphase = 0
            state.phase = 7
            next(8000)
        end
    elseif state.phase == 7 then
        if state.subphase == 0 then
            pathaleon:Talk(1)
            state.subphase = 1
            next(12000)
        elseif state.subphase == 1 then
            pathaleon:Talk(2)
            state.subphase = 2
            next(16000)
        else
            pathaleon:Talk(3)
            state.subphase = 0
            state.phase = 8
            next(10000)
        end
    elseif state.phase == 8 then
        creature:Talk(3)
        ardonis:Talk(1)
        state.phase = 9
        next(4000)
    elseif state.phase == 9 then
        turnToEachother(creature, state)
        pathaleon:SetVisible(false)
        state.subphase = 0
        state.phase = 10
        next(3000)
    elseif state.phase == 10 then
        creature:Talk(4)
        player:AreaExploredOrEventHappens(QUEST_INFO_GATHERING)
        resetEvent(creature, guid)
    end
end

-- C++ CanStartEvent (:186-204): per-event latch, Ardonis within
-- 10yd, Turn_to_eachother immediately, first phase after 4000ms.
local function onAreaTrigger(_, player, _triggerID)
    if not player:HasAura(SPELL_SUNFURY_DISGUISE) then
        return
    end
    if not player:IsAlive()
            or player:GetQuestStatus(QUEST_INFO_GATHERING) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    local dawnforge = player:FindNearestCreature(ENTRY_COMMANDER_DAWNFORGE, 30)
    if not dawnforge then
        return
    end
    local guid = dawnforge:GetGUID()
    local state = states[guid]
    if state and state.isEvent then
        return
    end
    local ardonis = dawnforge:FindNearestCreature(ENTRY_ARCANIST_ARDONIS, 10)
    if not ardonis then
        return
    end
    states[guid] = {
        isEvent = true,
        phase = 1,
        subphase = 0,
        playerGuid = player:GetGUID(),
        ardonis = ardonis,
        pathaleon = nil,
    }
    turnToEachother(dawnforge, states[guid])
    schedule(guid, "phase", 4000, function()
        phaseTick(dawnforge, guid)
    end)
end

-- C++ Reset (:131-134) via Initialize: latch + GUIDs cleared.
local function onReset(_, creature)
    resetEvent(creature, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_COMMANDER_DAWNFORGE, 5, onReset)
RegisterCreatureEvent(ENTRY_COMMANDER_DAWNFORGE, 23, onReset)
RegisterServerEvent(24, onAreaTrigger)
