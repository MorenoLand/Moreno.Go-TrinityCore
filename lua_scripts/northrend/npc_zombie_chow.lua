-- Zombie Chow (Naxxramas) — Lua port of npc_zombie_chowAI in
-- src/server/scripts/Northrend/Naxxramas/boss_gluth.cpp
-- (npc_zombie_chow (ScriptedAI); AddSC_boss_gluth registers it
-- alongside boss_gluth; loader decl 67 / call 262 per
-- northrend_script_loader.cpp).
-- Entries 16360 (10-man) / 30303 (25-man) appear only in a C++
-- comment (boss_gluth.cpp:391); no NPC_ constants in naxxramas.h,
-- no instance bindings — the entries are taken from that comment,
-- the only C++ evidence.
-- ctor (ev22 OnSummoned): DoCastSelf(Infected Wound 29307); the
-- Gluth GUID leg (C++ ctor reads GetGuidData(DATA_GLUTH) from the
-- instance script) rides the FindNearestCreature entry-scan twin —
-- Go has no instance-creature registry (marli-forward precedent).
-- SetData (ev41, DATA_ZOMBIE_STATE 1): DECIMATED (2) -> REACT_PASSIVE
-- + AttackStop + SetTarget(nil) + SetWalk(true), walk tick via ev7;
-- TOBE_EATEN (3) -> StopMoving (the motion-Clear twin) + REACT_PASSIVE +
-- AttackStop + SetTarget(nil) + ApplySpellImmune(0, MECHANIC 5,
-- GRIP 6, true) (SharedDefines.h:1363).
-- UpdateAI decimated leg (boss_gluth.cpp:409-433): every tick, if a
-- Gluth is found — timer accumulates 100ms per ev7 tick (C++ diff);
-- over 1600ms with dist2d to Gluth > 10.0 and CanFreeMove ->
-- MoveTo(0, gluth position), timer reset; dist2d <= 10.0 ->
-- StopMoving every tick. CanFreeMove twin is not-rooted/stunned
-- (Unit.cpp:9533 also excludes CONFUSED/FLEEING/IN_FLIGHT/
-- DISTRACTED, which have no Lua bridge — zombies never take those
-- states in this fight). The normal leg is melee, engine-driven.
-- DELTAS: GetData(DATA_ZOMBIE_STATE) has no C++ caller in the file
-- (defined but never read); the 1600ms animation-cycle comment
-- (:421) is modeled by the timer start (1000 on SetData, first
-- MovePoint ~600ms of ticking later, then every 1600ms).

local ENTRY_ZOMBIE_CHOW = 16360
local ENTRY_ZOMBIE_CHOW_HEROIC = 30303
local ENTRY_GLUTH = 15932

local SPELL_INFECTED_WOUND = 29307

local DATA_ZOMBIE_STATE = 1
local STATE_ZOMBIE_NORMAL = 1
local STATE_ZOMBIE_DECIMATED = 2
local STATE_ZOMBIE_TOBE_EATEN = 3

local chowState = {}

local function chowStateFor(guid)
    local st = chowState[guid]
    if not st then
        st = { state = STATE_ZOMBIE_NORMAL, timer = 0, gluthGUID = 0 }
        chowState[guid] = st
    end
    return st
end

-- C++ npc_zombie_chowAI ctor (boss_gluth.cpp:400-407): Gluth GUID
-- from the instance script + DoCastSelf(Infected Wound 29307) +
-- timer 0 + state NORMAL.
local function chowSummoned(event, creature)
    local guid = creature:GetGUIDLow()
    local st = chowStateFor(guid)
    st.state = STATE_ZOMBIE_NORMAL
    st.timer = 0
    local gluth = creature:FindNearestCreature(ENTRY_GLUTH, 100.0)
    if gluth ~= nil then
        st.gluthGUID = gluth:GetGUIDLow()
    end
    creature:CastSpell(creature, SPELL_INFECTED_WOUND)
end

-- C++ npc_zombie_chowAI::SetData (boss_gluth.cpp:435-465).
local function chowSetData(event, creature, setType, setValue)
    if setType ~= DATA_ZOMBIE_STATE then
        return
    end
    local st = chowStateFor(creature:GetGUIDLow())
    st.state = setValue
    if setValue == STATE_ZOMBIE_DECIMATED then
        creature:SetReactState(0)
        creature:AttackStop()
        creature:SetTarget(nil)
        creature:SetWalk(true)
        st.timer = 1000
    elseif setValue == STATE_ZOMBIE_TOBE_EATEN then
        creature:StopMoving()
        creature:SetReactState(0)
        creature:AttackStop()
        creature:SetTarget(nil)
        creature:ApplySpellImmune(0, 5, 6, true)
    end
end

-- C++ npc_zombie_chowAI::UpdateAI decimated leg
-- (boss_gluth.cpp:409-433). ev7 fires every 100ms tick for hooked
-- creatures (creaturemotion.go:9926-9931); the diff accumulates in
-- the per-zombie timer exactly like C++.
local function chowAIUpdate(event, creature)
    local st = chowState[creature:GetGUIDLow()]
    if st == nil or st.state ~= STATE_ZOMBIE_DECIMATED then
        return
    end
    if not creature:IsInCombat() then
        return
    end
    local gluth = nil
    if st.gluthGUID ~= 0 then
        gluth = creature:GetCreatureByGUID(st.gluthGUID)
    end
    if gluth == nil then
        return
    end
    st.timer = st.timer + 100
    if st.timer > 1600 and creature:GetDistance2d(gluth) > 10.0
            and not creature:IsRooted() and not creature:IsStunned() then
        creature:MoveTo(0, gluth:GetX(), gluth:GetY(), gluth:GetZ())
        st.timer = 0
    end
    if creature:GetDistance2d(gluth) <= 10.0 then
        creature:StopMoving()
    end
end

local function chowCleanup(event, creature)
    chowState[creature:GetGUIDLow()] = nil
end

RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 22, chowSummoned)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 22, chowSummoned)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 41, chowSetData)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 41, chowSetData)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 7, chowAIUpdate)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 7, chowAIUpdate)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 2, chowCleanup)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 2, chowCleanup)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 4, chowCleanup)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 4, chowCleanup)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW, 23, chowCleanup)
RegisterCreatureEvent(ENTRY_ZOMBIE_CHOW_HEROIC, 23, chowCleanup)
