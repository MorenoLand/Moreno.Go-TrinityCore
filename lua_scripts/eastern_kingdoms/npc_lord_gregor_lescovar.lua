-- Lord Gregor Lescovar (Stormwind City) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_stormwind_city.cpp
-- (npc_lord_gregor_lescovarAI — EscortAI + phase machine).
-- Zone-script unit per eastern_kingdoms_script_loader.cpp order
-- (stormwind_city: npc_tyrion, npc_tyrion_spybot, npc_lord_gregor_
-- lescovar, npc_marzon_silent_blade ported; zone_stormwind_city
-- closes the EK zone set).
-- Entries (LordGregorLescovar enum, verifiable from the C++
-- sources): 1754 (NPC_LORD_GREGOR_LESCOVAR), 1756
-- (NPC_STORMWIND_ROYAL), 1755 (NPC_MARZON_BLADE), 7766
-- (NPC_TYRION), quest 434 (QUEST_THE_ATTACK). FACTION_MONSTER
-- = 14 (SharedDefines.h:226). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat
-- (evade), 6 OnReachWP, 23 OnReset, 38 OnDoAction (the
-- cross-creature Marzon-arrival latch — see below). Melee is
-- engine-driven; the escort walk rides the DB waypoint path
-- via StartEscort + MoveWaypoint (legoso precedent).
-- Phase machine: 100ms CreateLuaEvent pump (legoso precedent),
-- started lazily from OnReachWP / OnDoAction when a phase arms
-- — the escort itself is started cross-file from
-- npc_tyrion_spybot.lua, which has no hook into this file's
-- pump. The C++ UpdateAI phase section decrements uiTimer
-- unconditionally (before the UpdateVictim return), so the pump
-- has no combat gate — unlike legoso, whose C++ froze its timer
-- while engaged.
-- Cross-AI latch (C++-exact): Marzon's MovementInform writes
-- uiTimer = 2000 / uiPhase = 5 directly into this AI (CAST_AI
-- in npc_marzon_silent_blade::MovementInform). In Lua the
-- write crosses files via DoAction: npc_marzon_silent_blade.lua
-- sends ACTION_MARZON_ARRIVED (1) on its ev28 POINT arrival
-- (sironas precedent — the same constant value is defined in
-- both files); the ev38 handler below arms phase 5 + starts
-- the pump if it is not running.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1) JustEngagedWith: stored Marzon, if alive
-- and not in combat, AttackStart on the same target (event
-- target, victim fallback — marzon precedent).
-- OnLeaveCombat(2) EnterEvadeMode: Despawn() self, then stored
-- Marzon Despawn() if alive (vazruden/muru convention).
-- OnReset(23): Initialize() clears uiTimer/uiPhase/MarzonGUID
-- — pump + per-GUID state die with it.
-- WaypointReached(6) — waypoint id is the DB point via
-- GetCurrentWaypointId (legoso precedent): wp 14 -> pause, Talk(
-- SAY_LESCOVAR_2 0), 3s, phase 1; wp 16 -> pause, SummonCreature(
-- NPC_MARZON_BLADE 1755, -8411.360352, 480.069733, 123.760895,
-- 4.941504, TEMPSUMMON_CORPSE_TIMED_DESPAWN 6, 1000ms) then
-- Marzon MoveTo(0, -8408.000977, 468.61145, 123.759903)
-- (MovePoint twin — arrival is never awaited, the machine
-- advances on the Marzon DoAction latch), store the Marzon
-- GUID, 2s, phase 4.
-- Phase 1 -> nearest royal guard (1756, 8yd) Talk(SAY_GUARD_2
-- 0), 3s, phase 2. Phase 2 -> every royal guard in 8yd
-- Despawn() (GetCreaturesInGrid twin of
-- GetCreatureListWithEntryInGrid), 2s, phase 3. Phase 3 ->
-- resume escort, phase 0.
-- Phase 4 -> Talk(SAY_LESCOVAR_3 1), phase 0.
-- Phase 5 (Marzon arrival latch) -> stored Marzon Talk(
-- SAY_MARZON_1 0), 3s, phase 6.
-- Phase 6 -> Talk(SAY_LESCOVAR_4 2) + escort player
-- AreaExploredOrEventHappens(QUEST_THE_ATTACK 434), 2s, phase
-- 7. Phase 7 -> nearest Tyrion (7766, 20yd) Talk(SAY_TYRION_2
-- 1); stored Marzon + self SetFaction(FACTION_MONSTER 14)
-- (the traitor flip), phase 0.
-- Deliberate delta: C++ phase 10 of the spybot escort also
-- calls SetMaxPlayerDistance(200.0f) on this AI — no bridge;
-- the 200yd leash is unmodeled, the escort handoff itself is
-- exact.

local NPC_LORD_GREGOR_LESCOVAR = 1754
local NPC_STORMWIND_ROYAL = 1756
local NPC_MARZON_BLADE = 1755
local NPC_TYRION = 7766

local QUEST_THE_ATTACK = 434
local FACTION_MONSTER = 14

local SAY_LESCOVAR_2 = 0
local SAY_LESCOVAR_3 = 1
local SAY_LESCOVAR_4 = 2
local SAY_GUARD_2 = 0
local SAY_MARZON_1 = 0
local SAY_TYRION_2 = 1

local TEMPSUMMON_CORPSE_TIMED_DESPAWN = 6

local ACTION_MARZON_ARRIVED = 1

local PHASE_NONE = 0

local lescovarEscort = {}
local lescovarEscortPump = {}

local function escortState(guid)
    local es = lescovarEscort[guid]
    if not es then
        es = { phase = PHASE_NONE, timer = 0, marzonGUID = 0 }
        lescovarEscort[guid] = es
    end
    return es
end

local function cancelEscortPump(guid)
    local id = lescovarEscortPump[guid]
    if id then
        RemoveEventById(id)
        lescovarEscortPump[guid] = nil
    end
end

local function getMarzon(creature, es)
    if not es.marzonGUID or es.marzonGUID == 0 then
        return nil
    end
    return creature:GetCreatureByGUID(es.marzonGUID)
end

local function runPhase(creature, guid, es)
    local phase = es.phase
    if phase == 1 then
        local guard = creature:FindNearestCreature(NPC_STORMWIND_ROYAL, 8.0)
        if guard then
            guard:Talk(SAY_GUARD_2)
        end
        es.timer = 3000
        es.phase = 2
    elseif phase == 2 then
        local guards = creature:GetCreaturesInGrid(NPC_STORMWIND_ROYAL, 8.0)
        for _, guard in ipairs(guards) do
            guard:Despawn()
        end
        es.timer = 2000
        es.phase = 3
    elseif phase == 3 then
        creature:SetEscortPaused(false)
        es.timer = 0
        es.phase = PHASE_NONE
    elseif phase == 4 then
        creature:Talk(SAY_LESCOVAR_3)
        es.timer = 0
        es.phase = PHASE_NONE
    elseif phase == 5 then
        local marzon = getMarzon(creature, es)
        if marzon then
            marzon:Talk(SAY_MARZON_1)
        end
        es.timer = 3000
        es.phase = 6
    elseif phase == 6 then
        creature:Talk(SAY_LESCOVAR_4)
        local player = creature:GetEscortPlayer()
        if player then
            player:AreaExploredOrEventHappens(QUEST_THE_ATTACK)
        end
        es.timer = 2000
        es.phase = 7
    elseif phase == 7 then
        local tyrion = creature:FindNearestCreature(NPC_TYRION, 20.0)
        if tyrion then
            tyrion:Talk(SAY_TYRION_2)
        end
        local marzon = getMarzon(creature, es)
        if marzon then
            marzon:SetFaction(FACTION_MONSTER)
        end
        creature:SetFaction(FACTION_MONSTER)
        es.timer = 0
        es.phase = PHASE_NONE
    end
end

-- C++ UpdateAI phase section (zone_stormwind_city.cpp:190-214):
-- fires the armed phase when uiTimer expires, decrements every
-- tick with no combat gate.
local function phaseTick(creature, guid)
    local es = lescovarEscort[guid]
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
    lescovarEscortPump[guid] = CreateLuaEvent(function()
        phaseTick(creature, guid)
    end, 100, 0)
end

-- C++ JustEngagedWith (zone_stormwind_city.cpp:102-110): stored
-- Marzon, if alive and not in combat, attacks the same target.
local function onEnterCombat(_, creature, target)
    local es = escortState(creature:GetGUIDLow())
    local marzon = getMarzon(creature, es)
    if marzon and marzon:IsAlive() and not marzon:IsInCombat() then
        local who = target or creature:GetVictim()
        if who then
            marzon:AttackStart(who)
        end
    end
end

-- C++ EnterEvadeMode (zone_stormwind_city.cpp:93-101):
-- DisappearAndDie self, then stored Marzon if alive.
local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    local es = escortState(guid)
    local marzon = getMarzon(creature, es)
    cancelEscortPump(guid)
    lescovarEscort[guid] = nil
    creature:Despawn()
    if marzon and marzon:IsAlive() then
        marzon:Despawn()
    end
end

-- C++ WaypointReached (zone_stormwind_city.cpp:112-141): wp 14
-- pause + Talk chain; wp 16 pause + summon Marzon and send him
-- to his point. The pump starts here (the escort is started
-- cross-file from npc_tyrion_spybot.lua).
local function onReachWP(_, creature)
    local guid = creature:GetGUIDLow()
    local es = escortState(guid)
    local wp = creature:GetCurrentWaypointId()
    if wp == 14 then
        creature:SetEscortPaused(true)
        creature:Talk(SAY_LESCOVAR_2)
        es.timer = 3000
        es.phase = 1
    elseif wp == 16 then
        creature:SetEscortPaused(true)
        local marzon = creature:SummonCreature(NPC_MARZON_BLADE, -8411.360352, 480.069733, 123.760895, 4.941504, TEMPSUMMON_CORPSE_TIMED_DESPAWN, 1000)
        if marzon then
            marzon:MoveTo(0, -8408.000977, 468.61145, 123.759903)
            es.marzonGUID = marzon:GetGUID()
        end
        es.timer = 2000
        es.phase = 4
    else
        return
    end
    if not lescovarEscortPump[guid] then
        startEscortPump(guid, creature)
    end
end

-- C++ npc_marzon_silent_blade::MovementInform (zone_stormwind_
-- city.cpp:282-300) writes uiTimer = 2000 / uiPhase = 5 into
-- this AI when Marzon reaches his point — carried here as the
-- DoAction latch (see header).
local function onDoAction(_, creature, action)
    if action ~= ACTION_MARZON_ARRIVED then
        return
    end
    local guid = creature:GetGUIDLow()
    local es = escortState(guid)
    es.timer = 2000
    es.phase = 5
    if not lescovarEscortPump[guid] then
        startEscortPump(guid, creature)
    end
end

-- C++ Reset (zone_stormwind_city.cpp:88-91): Initialize() clears
-- uiTimer/uiPhase/MarzonGUID — the pump and per-GUID state die
-- with it.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelEscortPump(guid)
    lescovarEscort[guid] = nil
end

RegisterCreatureEvent(NPC_LORD_GREGOR_LESCOVAR, 1, onEnterCombat)
RegisterCreatureEvent(NPC_LORD_GREGOR_LESCOVAR, 2, onLeaveCombat)
RegisterCreatureEvent(NPC_LORD_GREGOR_LESCOVAR, 6, onReachWP)
RegisterCreatureEvent(NPC_LORD_GREGOR_LESCOVAR, 23, onReset)
RegisterCreatureEvent(NPC_LORD_GREGOR_LESCOVAR, 38, onDoAction)
