-- Vazruden the Herald (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_vazruden_the_herald.cpp (boss_
-- vazruden_the_herald — the herald AI class; the boss_
-- vazruden, boss_nazan and npc_hellfire_sentry AI classes
-- the same file's AddSC_boss_vazruden_the_herald registers
-- are ported separately; no SpellScript/AuraScript scripts
-- in this file).
-- Final boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp stays blocked on the instance-script model).
-- Entries (hellfire_ramparts.h, verifiable from the C++
-- sources): NPC_VAZRUDEN_HERALD = 17307, NPC_VAZRUDEN =
-- 17537, NPC_NAZAN = 17536, NPC_LIQUID_FIRE = 22515,
-- NPC_HELLFIRE_SENTRY = 17517; DATA_VAZRUDEN = 2 and
-- DATA_NAZAN = 3 are instance-side constants, unbridgeable.
-- instance_hellfire_ramparts.cpp has no OnCreatureCreate
-- mapping for any of them — only OnGameObjectCreate doors
-- and a SetBossState DATA_VAZRUDEN/DATA_NAZAN completion
-- arm. The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- Talk: SAY_INTRO = 0 (engage — fired from the C++
-- JustEngagedWith phase 0 -> 1 arm, C++-exact). The rest of
-- the shared Says enum (SAY_WIPE = 0 / SAY_AGGRO = 1 /
-- SAY_KILL = 2 / SAY_DIE = 3) belongs to boss_vazruden
-- (see boss_vazruden.lua); the herald itself never uses
-- them.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 19 OnJustSummonedCreature, 23 OnReset. Scheduler
-- pump: 1s CreateLuaEvent tick for the phase-1 position gate
-- only (the MovePoint leg stays out). Melee is engine-driven.
-- Fight shape (C++-exact for the one modeled arm): Herald
-- (17307): OnEnterCombat(1): phase 0 -> 1 + Talk(SAY_INTRO)
-- (the C++ JustEngagedWith arm — C++-exact). The Reset()
-- phase = 0 re-latch lands on OnLeaveCombat(2)/OnReset(23)
-- via per-GUID state cleanup (kargath talk-only
-- precedent). OnDied(4): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model); no movement bridge — the whole
-- phase-0 platform circle, the phase-1 MovePoint to
-- VazrudenMiddle (-1406.5, 1746.5, 81.2) and the phase-2
-- ring waypoints VazrudenRing ((-1430.0, 1705.0, 112.0) /
-- (-1377.0, 1760.0, 112.0)) unmodeled; the phase-2 SummonAdds
-- machine is PORTED 2026-10-24 (SummonCreature NPC_VAZRUDEN
-- 17537/NPC_NAZAN 17536 at VazrudenMiddle, TEMPSUMMON_CORPSE_
-- TIMED_DESPAWN type 5, 100min; the JustSummoned Nazan
-- SetDisableGravity + Vazruden AttackStart(victim) arms; the
-- herald SetVisible(false)/UNIT_STATE_ROOT vanish arms via
-- creature:SetVisible + creature:SetRooted, shirrak precedent).
-- The phase-1 trigger is the C++ position gate itself
-- (IsWithinDist3d middle 5yd, 1s check) — no timer fabrication,
-- the MovePoint leg just never delivers the herald. The
-- JustSummoned Nazan SetSpeedRate(MOVE_FLIGHT, 2.5) leg stays
-- out (SetSpeedRate supports MOVE_RUN=1 only) and the
-- AttackStartNoMove(victim) leg stays out (no bridge);
-- the UnsummonAdds DisappearAndDie sweeps, the adds-alive/
-- adds-victim watch arm and the both-dead DisappearAndDie arm
-- stay out with EnterEvadeMode (no bridge); no cross-creature
-- bridge — the
-- sentry JustDied -> herald SentryDownBy(killer) relay and
-- the sentryDown AttackStartNoMove arm unmodeled (see
-- npc_hellfire_sentry.lua); no SpellScript/AuraScript
-- scripts in this file.

local SAY_INTRO = 0

local ENTRY_VAZRUDEN_THE_HERALD = 17307
local ENTRY_VAZRUDEN = 17537
local ENTRY_NAZAN = 17536

-- boss_vazruden_the_herald.cpp: VazrudenMiddle + SummonAdds args.
local MIDDLE_X = -1406.5
local MIDDLE_Y = 1746.5
local MIDDLE_Z = 81.2
local SUMMON_TYPE_CORPSE_TIMED = 5
local SUMMON_DESPAWN_MS = 6000000

local states = {}
local pumps = {}

local function resetHerald(guid)
    states[guid] = nil
end

-- C++ Initialize(): phase 0; the phase 0 -> 1 latch and the
-- SAY_INTRO arm land on OnEnterCombat (the phase = 0 re-latch
-- from C++ Reset() lands on the cleanup hooks); the phase-1
-- MovePoint leg stays out (no movement bridge) but the
-- position gate + SummonAdds arms are ported below.
local function freshState()
    return {
        phase = 0,
    }
end

local function cancelPump(guid)
    local id = pumps[guid]
    if id then
        RemoveEventById(id)
        pumps[guid] = nil
    end
end

-- C++ SummonAdds' Nazan leg: SelectNearestPlayer(60.0f) —
-- nearest alive player in the instance within 60yd (3D).
local function nearestPlayer60(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local cx, cy, cz = creature:GetX(), creature:GetY(), creature:GetZ()
    local best, bestDist2 = nil, 3600
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            local dx, dy, dz = p:GetX() - cx, p:GetY() - cy, p:GetZ() - cz
            local d2 = dx * dx + dy * dy + dz * dz
            if d2 <= 3600 and (best == nil or d2 < bestDist2) then
                best, bestDist2 = p, d2
            end
        end
    end
    return best
end

-- C++ boss_vazruden_the_heraldAI::SummonAdds
-- (boss_vazruden_the_herald.cpp:346-363, C++ arm order):
-- Vazruden then Nazan at VazrudenMiddle, then summoned=true,
-- SetVisible(false), AddUnitState(UNIT_STATE_ROOT). The
-- JustSummoned arms run inside each SummonCreature call
-- (event 19 on the summoner, heraldJustSummoned below);
-- the Nazan AttackStart(nearest player 60) leg lands after
-- the Nazan summon returns, matching the C++ order.
local function summonAdds(creature)
    creature:SummonCreature(ENTRY_VAZRUDEN, MIDDLE_X, MIDDLE_Y,
        MIDDLE_Z, 0, SUMMON_TYPE_CORPSE_TIMED, SUMMON_DESPAWN_MS)
    local nazan = creature:SummonCreature(ENTRY_NAZAN, MIDDLE_X,
        MIDDLE_Y, MIDDLE_Z, 0, SUMMON_TYPE_CORPSE_TIMED,
        SUMMON_DESPAWN_MS)
    if nazan then
        local player = nearestPlayer60(creature)
        if player then
            nazan:EngageWithTarget(player)
        end
    end
    creature:SetVisible(false)
    creature:SetRooted(true)
end

-- C++ phase-1 UpdateAI check (boss_vazruden_the_herald.cpp:401-417,
-- 1s cadence): not within 5yd (3D) of VazrudenMiddle -> MovePoint
-- (stays out, no movement bridge); within 5yd -> SummonAdds +
-- phase 2. Phase 2's watch arm stays out (EnterEvadeMode, no
-- bridge), so the pump idles there.
local function heraldTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    if st.phase == 1 then
        local dx = creature:GetX() - MIDDLE_X
        local dy = creature:GetY() - MIDDLE_Y
        local dz = creature:GetZ() - MIDDLE_Z
        if dx * dx + dy * dy + dz * dz <= 25 then
            summonAdds(creature)
            st.phase = 2
        end
    end
end

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 1,
    function(_, creature)
        creature:SetMeleeAttack(false) -- boss_vazruden_the_herald.cpp:402: custom UpdateAI never calls DoMeleeAttackIfReady (phase machine in C++, never melees)
        local guid = creature:GetGUID()
        cancelPump(guid)
        resetHerald(guid)
        local st = freshState()
        states[guid] = st
        -- C++ JustEngagedWith: phase 0 -> 1 + Talk(SAY_INTRO)
        -- (C++-exact).
        if st.phase == 0 then
            st.phase = 1
            creature:Talk(SAY_INTRO)
        end
        pumps[guid] = CreateLuaEvent(function()
            heraldTick(creature, guid)
        end, 1000, 0)
    end)

-- C++ boss_vazruden_the_heraldAI::JustSummoned
-- (boss_vazruden_the_herald.cpp:372-383): entry-gated on the
-- SUMMON — event 19 fires on the summoner's entry bindings
-- with (event, summoner, summon). Nazan: SetDisableGravity
-- (true); SetSpeedRate(MOVE_FLIGHT, 2.5) stays out
-- (MOVE_RUN=1 only) and AttackStartNoMove(victim) on the
-- herald stays out (no bridge). Any other summon:
-- AttackStart(herald's victim) via EngageWithTarget.
local function heraldJustSummoned(_, creature, summon)
    if not summon then
        return
    end
    if summon:GetEntry() == ENTRY_NAZAN then
        summon:SetDisableGravity(true)
    else
        local victim = creature:GetVictim()
        if victim then
            summon:EngageWithTarget(victim)
        end
    end
end

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 19, heraldJustSummoned)

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 2,
    function(_, creature)
        cancelPump(creature:GetGUID())
        resetHerald(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 4,
    function(_, creature)
        cancelPump(creature:GetGUID())
        resetHerald(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 23,
    function(_, creature)
        cancelPump(creature:GetGUID())
        resetHerald(creature:GetGUID())
    end)
