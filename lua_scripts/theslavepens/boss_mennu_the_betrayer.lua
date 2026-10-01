-- Mennu the Betrayer (The Slave Pens) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- boss_mennu_the_betrayer.cpp (boss_mennu_the_betrayer — the
-- only AI class AddSC_boss_mennu_the_betrayer registers; no
-- other creature/GO scripts in this file). First boss in The
-- Slave Pens set per outland_script_loader.cpp order; Steam
-- Vault roster is 3/3 COMPLETE (thespia + steamrigger +
-- kalithresh), so this is the next untouched zone.
-- Entry 17941 verified externally (wowhead npc=17941/
-- mennu-the-betrayer, morogrim precedent); no OnCreatureCreate
-- mapping exists for Mennu in instance_the_slave_pens.cpp
-- (morogrim precedent — the instance is a placeholder script
-- that only tracks Ahune/flamecaller/bonfire bunnies); the_
-- slave_pens.h DATA_MENNU_THE_BETRAYER = 1 is an instance-side
-- constant (unbridgeable). The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk lines used: SAY_AGGRO=0 (pull), SAY_SLAY=1 (kill — NO
-- TYPEID gate in C++, C++-exact, morogrim/vashj precedent),
-- SAY_DEATH=2 (death); no unused enum members in this file.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention);
-- DoCastVictim with the triggered=true arg is a triggered
-- DoCastVictim (gruul reverberation precedent); DoCastVictim
-- takes nil as the victim arm (felmyst convention). Fight
-- shape (C++-exact for all modeled arms):
-- boss_mennu_the_betrayer (17941): OnEnterCombat(1): per-GUID
-- reset to the C++ JustEngagedWith schedule values (stoneskin
-- 30000 / earthgrab 20000 / nova 60000 / healingWard
-- {14000,25000} / lightningBolt {14000,19000}) + Talk(SAY_
-- AGGRO) + 1s scheduler pump (a port of UpdateAI — 1s
-- granularity is exact for all C++ timers here; the
-- !UpdateVictim early return collapses into the pump, which
-- only runs in combat). Pump: tainted stoneskin totem 31985
-- 30s — fires only when HealthBelowPct(100) (the C++ gate),
-- re-arm 30000 regardless (C++-exact); tainted earthgrab totem
-- 31981 20s — DoCast(me), one-shot (no re-arm, C++-exact);
-- corrupted nova totem 31991 60s — DoCast(me), one-shot (no
-- re-arm, C++-exact); Mennu's healing ward 34980 {14s,25s} —
-- DoCast(me), re-arm {14s,25s} (C++-exact); lightning bolt
-- 35010 {14s,19s} then {14s,25s} — triggered DoCastVictim
-- (the C++ true arg), re-arm {14s,25s} (C++-exact). The
-- UNIT_STATE_CASTING early returns have no unit-state bridge
-- (thespia elemental precedent). OnTargetDied(3): Talk(SAY_
-- SLAY) (no TYPEID gate, C++-exact). OnDied(4): Talk(SAY_
-- DEATH) + cleanup (the _JustDied/SetBossState DONE arm is
-- instance-blocked). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the _Reset/SetBossState NOT_
-- STARTED arm is instance-blocked). Melee is engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (the DATA_MENNU_THE_BETRAYER = 1 NOT_STARTED/
-- IN_PROGRESS/DONE bookkeeping in Reset/JustEngagedWith/
-- JustDied skipped; instance_the_slave_pens.cpp is a
-- placeholder and stays blocked on the instance-script model);
-- no unit-state bridge — the UNIT_STATE_CASTING early returns
-- in UpdateAI unmodeled (timer bookkeeping exact); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_TAINTED_STONESKIN_TOTEM = 31985
local SPELL_TAINTED_EARTHGRAB_TOTEM = 31981
local SPELL_CORRUPTED_NOVA_TOTEM = 31991
local SPELL_MENNUS_HEALING_WARD = 34980
local SPELL_LIGHTNING_BOLT = 35010

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local ENTRY_MENNU_THE_BETRAYER = 17941

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

-- C++ ScheduleEvent(event, 14s, 25s) picks uniformly in the
-- range (gruul precedent: math.random(14000, 25000)).
-- JustEngagedWith initial values: stoneskin 30000, earthgrab
-- 20000, nova 60000, healingWard {14000,25000}, lightningBolt
-- {14000,19000}.
local function freshState()
    return {
        stoneskin = 30000,
        earthgrab = 20000,
        nova = 60000,
        healingWard = math.random(14000, 25000),
        lightningBolt = math.random(14000, 19000),
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Tainted stoneskin totem 31985: 30s, fires only when
    -- HealthBelowPct(100) (C++ gate); re-arm 30000 regardless
    -- (C++-exact).
    if st.stoneskin <= 1000 then
        if creature:GetHealthPct() < 100 then
            creature:CastSpell(nil, SPELL_TAINTED_STONESKIN_TOTEM)
        end
        st.stoneskin = 30000
    else
        st.stoneskin = st.stoneskin - 1000
    end

    -- Tainted earthgrab totem 31981: 20s one-shot, DoCast(me)
    -- (C++-exact, no re-arm).
    if st.earthgrab <= 1000 then
        creature:CastSpell(nil, SPELL_TAINTED_EARTHGRAB_TOTEM)
        st.earthgrab = 999999
    else
        st.earthgrab = st.earthgrab - 1000
    end

    -- Corrupted nova totem 31991: 60s one-shot, DoCast(me)
    -- (C++-exact, no re-arm).
    if st.nova <= 1000 then
        creature:CastSpell(nil, SPELL_CORRUPTED_NOVA_TOTEM)
        st.nova = 999999
    else
        st.nova = st.nova - 1000
    end

    -- Mennu's healing ward 34980: {14s,25s}, DoCast(me),
    -- re-arm {14s,25s} (C++-exact).
    if st.healingWard <= 1000 then
        creature:CastSpell(nil, SPELL_MENNUS_HEALING_WARD)
        st.healingWard = math.random(14000, 25000)
    else
        st.healingWard = st.healingWard - 1000
    end

    -- Lightning bolt 35010: {14s,19s} init then {14s,25s},
    -- triggered DoCastVictim (the C++ true arg), re-arm
    -- {14s,25s} (C++-exact).
    if st.lightningBolt <= 1000 then
        creature:CastSpell(nil, SPELL_LIGHTNING_BOLT, true)
        st.lightningBolt = math.random(14000, 25000)
    else
        st.lightningBolt = st.lightningBolt - 1000
    end
end

RegisterCreatureEvent(ENTRY_MENNU_THE_BETRAYER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_MENNU_THE_BETRAYER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_MENNU_THE_BETRAYER, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_MENNU_THE_BETRAYER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_MENNU_THE_BETRAYER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
