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
-- OnDied, 23 OnReset. No scheduler pump: the C++ herald's
-- whole phase machine is movement/summon-gated (see
-- deviations). Melee is engine-driven.
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
-- (-1377.0, 1760.0, 112.0)) unmodeled; no summon bridge —
-- the phase-2 SummonAdds machine (SummonCreature NPC_
-- VAZRUDEN/NPC_NAZAN at VazrudenMiddle, TEMPSUMMON_CORPSE_
-- TIMED_DESPAWN 100min; the JustSummoned SetDisableGravity/
-- speed/AttackStart arms; the UnsummonAdds DisappearAndDie
-- sweeps via GUID or FindNearestCreature 5000; the adds-
-- alive/adds-victim watch arm and the both-dead
-- DisappearAndDie arm) unmodeled (steamrigger precedent);
-- the SetVisible(false)/UNIT_STATE_ROOT vanish arms and
-- the ClearUnitState/SetVisible(true) restore arms have no
-- visibility/state bridges; no cross-creature bridge — the
-- sentry JustDied -> herald SentryDownBy(killer) relay and
-- the sentryDown AttackStartNoMove arm unmodeled (see
-- npc_hellfire_sentry.lua); no SpellScript/AuraScript
-- scripts in this file.

local SAY_INTRO = 0

local ENTRY_VAZRUDEN_THE_HERALD = 17307

local states = {}

local function resetHerald(guid)
    states[guid] = nil
end

-- C++ Initialize(): phase 0; the phase-1/2 schedule needs
-- movement/summon bridges, so only the phase 0 -> 1 latch
-- and the SAY_INTRO arm land here (the phase = 0 re-latch
-- from C++ Reset() lands on the cleanup hooks).
local function freshState()
    return {
        phase = 0,
    }
end

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 1,
    function(_, creature)
        local guid = creature:GetGUID()
        resetHerald(guid)
        local st = freshState()
        states[guid] = st
        -- C++ JustEngagedWith: phase 0 -> 1 + Talk(SAY_INTRO)
        -- (C++-exact).
        if st.phase == 0 then
            st.phase = 1
            creature:Talk(SAY_INTRO)
        end
    end)

-- No pump: the phase-1 MovePoint-to-middle and phase-2
-- summon arms have no bridges in this model.

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 2,
    function(_, creature)
        resetHerald(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 4,
    function(_, creature)
        resetHerald(creature:GetGUID())
    end)

RegisterCreatureEvent(ENTRY_VAZRUDEN_THE_HERALD, 23,
    function(_, creature)
        resetHerald(creature:GetGUID())
    end)
