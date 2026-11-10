-- Tyrion Spybot (Stormwind City) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_stormwind_city.cpp
-- (npc_tyrion_spybotAI — EscortAI + phase machine).
-- Zone-script unit per eastern_kingdoms_script_loader.cpp order
-- (stormwind_city: npc_tyrion, npc_tyrion_spybot, npc_lord_gregor_
-- lescovar, npc_marzon_silent_blade ported; zone_stormwind_city
-- closes the EK zone set).
-- Entries (TyrionSpybot / LordGregorLescovar enums, verifiable
-- from the C++ sources): 8856 (NPC_TYRION_SPYBOT), 7766
-- (NPC_TYRION), 7779 (NPC_PRIESTESS_TYRIONA), 1756
-- (NPC_STORMWIND_ROYAL), 1754 (NPC_LORD_GREGOR_LESCOVAR).
-- The creature_template ScriptName binding is DB-side (no TDB
-- in this workspace).
-- Eluna creature events: 6 OnReachWP, 23 OnReset (the C++
-- overrides no JustEngagedWith / EnterEvadeMode / DamageTaken
-- arms — melee is engine-driven; the escort walk rides the DB
-- waypoint path via StartEscort + MoveWaypoint, legoso
-- precedent).
-- Phase machine: 100ms CreateLuaEvent pump (legoso precedent),
-- started lazily from OnReachWP when a phase arms — the escort
-- itself is started cross-file from npc_tyrion.lua, which has no
-- hook into this file's pump. The C++ UpdateAI phase section
-- decrements uiTimer unconditionally (before the UpdateVictim
-- return), so the pump has no combat gate — unlike legoso,
-- whose C++ froze its timer while engaged.
-- Fight shape (C++-exact for all modeled arms):
-- WaypointReached(6) — waypoint id is the DB point via
-- GetCurrentWaypointId (legoso precedent), not the event's node
-- index: wp 1 -> pause, 2s, phase 1; wp 5 -> pause, Talk(
-- SAY_SPYBOT_1 1), 2s, phase 5; wp 17 -> pause, Talk(
-- SAY_SPYBOT_3 3), 3s, phase 8.
-- Phase 1 -> Talk(SAY_QUEST_ACCEPT_ATTACK 0), 3s, phase 2.
-- Phase 2 -> nearest Tyrion (7766, 10yd) Talk(SAY_TYRION_1 0)
-- whispered to the escort player, 3s, phase 3.
-- Phase 3 -> UpdateEntry(NPC_PRIESTESS_TYRIONA 7779), 2s,
-- phase 4. Phase 4 -> resume escort, phase 0.
-- Phase 5 -> nearest royal guard (1756, 10yd) Talk(SAY_GUARD_1
-- 1), 3s, phase 6. Phase 6 -> Talk(SAY_SPYBOT_2 2), 3s, phase
-- 7. Phase 7 -> resume escort, phase 0.
-- Phase 8 -> nearest Lescovar (1754, 10yd) Talk(SAY_LESCOVAR_1
-- 3), 3s, phase 9. Phase 9 -> Talk(SAY_SPYBOT_4 4), 3s, phase
-- 10. Phase 10 -> nearest Lescovar StartEscort(player) +
-- MoveWaypoint() for the escort player, then Despawn() self
-- (C++ DisappearAndDie, vazruden/muru convention).
-- Deliberate delta: C++ phase 10 also calls Lescovar AI
-- SetMaxPlayerDistance(200.0f) — no bridge; the 200yd leash
-- is unmodeled, the escort handoff itself is exact.

local NPC_TYRION_SPYBOT = 8856
local NPC_TYRION = 7766
local NPC_PRIESTESS_TYRIONA = 7779
local NPC_STORMWIND_ROYAL = 1756
local NPC_LORD_GREGOR_LESCOVAR = 1754

local SAY_QUEST_ACCEPT_ATTACK = 0
local SAY_SPYBOT_1 = 1
local SAY_SPYBOT_2 = 2
local SAY_SPYBOT_3 = 3
local SAY_SPYBOT_4 = 4
local SAY_TYRION_1 = 0
local SAY_GUARD_1 = 1
local SAY_LESCOVAR_1 = 3

local PHASE_NONE = 0

local spybotEscort = {}
local spybotEscortPump = {}

local function escortState(guid)
    local es = spybotEscort[guid]
    if not es then
        es = { phase = PHASE_NONE, timer = 0 }
        spybotEscort[guid] = es
    end
    return es
end

local function cancelEscortPump(guid)
    local id = spybotEscortPump[guid]
    if id then
        RemoveEventById(id)
        spybotEscortPump[guid] = nil
    end
end

local function runPhase(creature, guid, es)
    local phase = es.phase
    if phase == 1 then
        creature:Talk(SAY_QUEST_ACCEPT_ATTACK)
        es.timer = 3000
        es.phase = 2
    elseif phase == 2 then
        local tyrion = creature:FindNearestCreature(NPC_TYRION, 10.0)
        if tyrion then
            local player = creature:GetEscortPlayer()
            if player then
                tyrion:Talk(SAY_TYRION_1, player)
            end
        end
        es.timer = 3000
        es.phase = 3
    elseif phase == 3 then
        creature:UpdateEntry(NPC_PRIESTESS_TYRIONA)
        es.timer = 2000
        es.phase = 4
    elseif phase == 4 then
        creature:SetEscortPaused(false)
        es.timer = 0
        es.phase = PHASE_NONE
    elseif phase == 5 then
        local guard = creature:FindNearestCreature(NPC_STORMWIND_ROYAL, 10.0)
        if guard then
            guard:Talk(SAY_GUARD_1)
        end
        es.timer = 3000
        es.phase = 6
    elseif phase == 6 then
        creature:Talk(SAY_SPYBOT_2)
        es.timer = 3000
        es.phase = 7
    elseif phase == 7 then
        creature:SetEscortPaused(false)
        es.timer = 0
        es.phase = PHASE_NONE
    elseif phase == 8 then
        local lescovar = creature:FindNearestCreature(NPC_LORD_GREGOR_LESCOVAR, 10.0)
        if lescovar then
            lescovar:Talk(SAY_LESCOVAR_1)
        end
        es.timer = 3000
        es.phase = 9
    elseif phase == 9 then
        creature:Talk(SAY_SPYBOT_4)
        es.timer = 3000
        es.phase = 10
    elseif phase == 10 then
        local lescovar = creature:FindNearestCreature(NPC_LORD_GREGOR_LESCOVAR, 10.0)
        if lescovar then
            local player = creature:GetEscortPlayer()
            if player then
                lescovar:StartEscort(player)
                lescovar:MoveWaypoint()
            end
        end
        es.timer = 0
        es.phase = PHASE_NONE
        cancelEscortPump(guid)
        creature:Despawn()
    end
end

-- C++ UpdateAI phase section (zone_stormwind_city.cpp:420-464):
-- fires the armed phase when uiTimer expires, decrements every
-- tick with no combat gate.
local function phaseTick(creature, guid)
    local es = spybotEscort[guid]
    if not es or es.phase == PHASE_NONE then
        return
    end
    if es.timer <= 100 then
        runPhase(creature, guid, es)
    else
        es.timer = es.timer - 100
    end
end

local function startEscortPump(guid, creature)
    cancelEscortPump(guid)
    escortState(guid)
    spybotEscortPump[guid] = CreateLuaEvent(function()
        phaseTick(creature, guid)
    end, 100, 0)
end

-- C++ WaypointReached (zone_stormwind_city.cpp:359-383): wp 1/5/17
-- pause + arm the Talk-chain phases. The pump starts here (the
-- escort is started cross-file from npc_tyrion.lua).
local function onReachWP(_, creature)
    local guid = creature:GetGUIDLow()
    local es = escortState(guid)
    local wp = creature:GetCurrentWaypointId()
    if wp == 1 then
        creature:SetEscortPaused(true)
        es.timer = 2000
        es.phase = 1
    elseif wp == 5 then
        creature:SetEscortPaused(true)
        creature:Talk(SAY_SPYBOT_1)
        es.timer = 2000
        es.phase = 5
    elseif wp == 17 then
        creature:SetEscortPaused(true)
        creature:Talk(SAY_SPYBOT_3)
        es.timer = 3000
        es.phase = 8
    else
        return
    end
    if not spybotEscortPump[guid] then
        startEscortPump(guid, creature)
    end
end

-- C++ Reset (zone_stormwind_city.cpp:351-357): Initialize() clears
-- uiTimer/uiPhase — the pump and per-GUID state die with it.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelEscortPump(guid)
    spybotEscort[guid] = nil
end

RegisterCreatureEvent(NPC_TYRION_SPYBOT, 6, onReachWP)
RegisterCreatureEvent(NPC_TYRION_SPYBOT, 23, onReset)
