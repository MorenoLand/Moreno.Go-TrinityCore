-- Gatewatcher Gyro-Kill (The Mechanar) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- Mechanar/boss_gatewatcher_gyrokill.cpp
-- (boss_gatewatcher_gyrokill — the only CreatureScript AI
-- class AddSC_boss_gatewatcher_gyrokill registers).
-- Second unit of the Mechanar block per
-- outland_script_loader.cpp order (iron_hand, gyrokill,
-- nethermancer_sepethrea, pathaleon, capacitus,
-- instance_mechanar).
-- Entry: 19218 (no NPC_ constant in the C++ tree — DB-side
-- ScriptName binding, like 19710 iron_hand; entry per
-- standard TBC creature data, not verifiable from this
-- workspace). DATA_GATEWATCHER_GYROKILL = 0 is
-- instance-side, unbridgeable.
-- Talk: SAY_AGGRO = 0 (engage — OnEnterCombat, C++-exact),
-- SAY_SLAY = 1 (kill — OnTargetDied, C++-exact, no roll),
-- SAY_SAW_BLADEs = 2 (saw-blade arm — C++-exact),
-- SAY_DEATH = 3 (death — OnDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell,
-- triggered) = DoCastVictim, creature:CastSpell(creature,
-- spell) = DoCast(me).
-- Fight shape (C++-exact for all modeled arms):
-- 1. Stream of Machine Fluid 35311: 10s init then randtime
--    13-17s — TRIGGERED DoCastVictim (C++-exact).
-- 2. Saw Blade 35318: 20s init then randtime 20-30s —
--    non-triggered DoCast(me) self-cast (C++-exact) +
--    Talk(SAY_SAW_BLADEs). H_SPELL_SAW_BLADE 39192 exists in
--    the C++ enum but is NEVER fired by UpdateAI — the port
--    fires the normal spell always (C++-exact, not
--    IsHeroic-gated; iron-hand precedent).
-- 3. Shadow Power 35322: 25s init — non-triggered DoCast(me)
--    self-cast, H_SPELL_SHADOW_POWER 39193 likewise never
--    fired (C++-exact, not IsHeroic-gated). C++-EXACT QUIRK:
--    the EVENT_SHADOW_POWER arm schedules EVENT_SAW_BLADE
--    at randtime 25-35s instead of re-arming itself — shadow
--    power fires exactly once per combat and accelerates the
--    saw-blade schedule (a second pending saw-blade event).
--    Modeled via the sawBladeExtra countdown, which fires
--    the same cast+Talk handler.
-- OnDied(4): Talk(SAY_DEATH) (C++-exact); the BossAI
-- _JustDied arm is instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() / BossAI _Reset() arms are instance-blocked).
-- The C++ UNIT_STATE_CASTING early-return (top of UpdateAI)
-- and the post-switch cast-state re-check have no cast-state
-- bridge — the arms fire unconditionally (zereketh
-- precedent).

local SPELL_STREAM_OF_MACHINE_FLUID = 35311
local SPELL_SAW_BLADE = 35318
local SPELL_SHADOW_POWER = 35322

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SAW_BLADES = 2
local SAY_DEATH = 3

local ENTRY_GATEWATCHER_GYROKILL = 19218

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ JustEngagedWith schedules: STREAM 10s, SAW_BLADE 20s,
-- SHADOW_POWER 25s (mechanar engagement-schedule
-- precedent, matching iron_hand / alar / void_reaver).
local function freshState()
    return {
        stream = 10000,
        sawBlade = 20000,
        shadowPower = 25000,
        sawBladeExtra = nil,
    }
end

local function fireSawBlade(creature, st)
    creature:CastSpell(creature, SPELL_SAW_BLADE)
    creature:Talk(SAY_SAW_BLADES)
    st.sawBlade = math.random(20000, 30000)
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Stream of Machine Fluid 35311: 10s init then randtime
    -- 13-17s — TRIGGERED DoCastVictim (C++-exact).
    if st.stream <= 1000 then
        creature:CastSpell(nil, SPELL_STREAM_OF_MACHINE_FLUID, true)
        st.stream = math.random(13000, 17000)
    else
        st.stream = st.stream - 1000
    end

    -- Saw Blade 35318: 20s init then randtime 20-30s —
    -- non-triggered DoCast(me) self-cast + Talk (C++-exact);
    -- the heroic 39192 variant is never fired by the C++.
    if st.sawBlade <= 1000 then
        fireSawBlade(creature, st)
    else
        st.sawBlade = st.sawBlade - 1000
    end

    -- The shadow-power quirk: C++ re-schedules EVENT_SAW_BLADE
    -- (a second pending event), not EVENT_SHADOW_POWER — the
    -- extra countdown fires the same saw-blade handler.
    if st.sawBladeExtra then
        if st.sawBladeExtra <= 1000 then
            fireSawBlade(creature, st)
            st.sawBladeExtra = nil
        else
            st.sawBladeExtra = st.sawBladeExtra - 1000
        end
    end

    -- Shadow Power 35322: 25s init, fires ONCE —
    -- non-triggered DoCast(me) self-cast (C++-exact); the arm
    -- schedules EVENT_SAW_BLADE at randtime 25-35s instead of
    -- re-arming itself (C++-exact quirk, local-tree text).
    if st.shadowPower then
        if st.shadowPower <= 1000 then
            creature:CastSpell(creature, SPELL_SHADOW_POWER)
            st.shadowPower = nil
            st.sawBladeExtra = math.random(25000, 35000)
        else
            st.shadowPower = st.shadowPower - 1000
        end
    end
end

RegisterCreatureEvent(ENTRY_GATEWATCHER_GYROKILL, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_GYROKILL, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_GYROKILL, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally (C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_GYROKILL, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GATEWATCHER_GYROKILL, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
