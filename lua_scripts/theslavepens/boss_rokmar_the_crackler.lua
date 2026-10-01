-- Rokmar the Crackler (The Slave Pens) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- boss_rokmar_the_crackler.cpp (boss_rokmar_the_crackler — the
-- only AI class AddSC_boss_rokmar_the_crackler registers; no
-- other creature/GO scripts in this file). Second boss in The
-- Slave Pens set per outland_script_loader.cpp order (mennu
-- closed; quagmirran, ahune remain).
-- Entry 17991 verified externally (wowhead npc=17991/
-- rokmar-the-crackler, mennu precedent); no OnCreatureCreate
-- mapping exists for Rokmar in instance_the_slave_pens.cpp
-- (mennu/morogrim precedent — the instance is a placeholder
-- script that only tracks Ahune/flamecaller/bonfire bunnies);
-- the_slave_pens.h DATA_ROKMAR_THE_CRACKLER = 2 is an
-- instance-side constant (unbridgeable). The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- The file has NO Talk calls (no Say enum at all) and its
-- KilledUnit override is empty — no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention);
-- DoCastVictim with the triggered=true arg is a triggered
-- DoCastVictim (gruul reverberation precedent); DoCastAOE
-- lands on the caster's own location — modeled as a
-- non-triggered self-cast (supremus volcano precedent);
-- DoCastVictim takes nil as the victim arm (felmyst
-- convention). Fight shape (C++-exact for all modeled arms):
-- boss_rokmar_the_crackler (17991): OnEnterCombat(1): per-GUID
-- reset to the C++ JustEngagedWith schedule values (grievous
-- wound 10000 / ensnaring moss 20000 / water spit 14000 /
-- rokmarFrenzy false) + 1s scheduler pump (a port of UpdateAI
-- — 1s granularity is exact for all C++ timers here; the
-- !UpdateVictim early return collapses into the pump, which
-- only runs in combat). Pump: grievous wound 31956 10s then
-- {20s,30s} — triggered DoCastVictim (the C++ true arg),
-- re-arm {20s,30s} (C++-exact); ensnaring moss 31948 20s then
-- {20s,30s} — DoCastAOE (self-cast, C++-exact), re-arm
-- {20s,30s}; water spit 35008 14s then {14s,18s} — DoCastAOE
-- (self-cast, C++-exact), re-arm {14s,18s}; once-guarded
-- GetHealthPct() < 10 (C++ HealthBelowPct(10)) -> DoCast(me)
-- SPELL_FRENZY 34970 non-triggered self-cast (C++-exact), flag
-- set. The UNIT_STATE_CASTING early returns have no
-- unit-state bridge (mennu/thespia precedent). OnDied(4):
-- cleanup (the _JustDied arm is instance-blocked). OnLeave-
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID state
-- (the _Reset/Initialize arm is instance-blocked). Melee is
-- engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (the DATA_MENNU_THE_BETRAYER/NOT_STARTED/IN_PROGRESS/
-- DONE bookkeeping in the ctor/Reset/JustEngagedWith/JustDied
-- skipped; instance_the_slave_pens.cpp is a placeholder and
-- stays blocked on the instance-script model); no unit-state
-- bridge — the UNIT_STATE_CASTING early returns in UpdateAI
-- unmodeled (timer bookkeeping exact); no SpellScript/
-- AuraScript scripts in this file. Note: the C++ ctor passes
-- DATA_MENNU_THE_BETRAYER (1) as the BossAI boss-id — an
-- upstream copy/paste quirk (DATA_ROKMAR_THE_CRACKLER = 2
-- exists in the_slave_pens.h) — kept as-is in spirit since the
-- whole instance arm is skipped here anyway.

local SPELL_GRIEVOUS_WOUND = 31956
local SPELL_ENSNARING_MOSS = 31948
local SPELL_WATER_SPIT = 35008
local SPELL_FRENZY = 34970

local ENTRY_ROKMAR_THE_CRACKLER = 17991

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

-- C++ ScheduleEvent(event, 20s, 30s) picks uniformly in the
-- range (gruul precedent: math.random(20000, 30000)).
-- JustEngagedWith initial values: grievous wound 10000,
-- ensnaring moss 20000, water spit 14000, rokmarFrenzy false.
local function freshState()
    return {
        grievousWound = 10000,
        ensnaringMoss = 20000,
        waterSpit = 14000,
        frenzyUsed = false,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Grievous wound 31956: 10s init then {20s,30s} —
    -- triggered DoCastVictim (the C++ true arg), re-arm
    -- {20s,30s} (C++-exact).
    if st.grievousWound <= 1000 then
        creature:CastSpell(nil, SPELL_GRIEVOUS_WOUND, true)
        st.grievousWound = math.random(20000, 30000)
    else
        st.grievousWound = st.grievousWound - 1000
    end

    -- Ensnaring moss 31948: 20s init then {20s,30s} —
    -- DoCastAOE (non-triggered self-cast, supremus precedent),
    -- re-arm {20s,30s} (C++-exact).
    if st.ensnaringMoss <= 1000 then
        creature:CastSpell(nil, SPELL_ENSNARING_MOSS)
        st.ensnaringMoss = math.random(20000, 30000)
    else
        st.ensnaringMoss = st.ensnaringMoss - 1000
    end

    -- Water spit 35008: 14s init then {14s,18s} — DoCastAOE
    -- (non-triggered self-cast, supremus precedent), re-arm
    -- {14s,18s} (C++-exact).
    if st.waterSpit <= 1000 then
        creature:CastSpell(nil, SPELL_WATER_SPIT)
        st.waterSpit = math.random(14000, 18000)
    else
        st.waterSpit = st.waterSpit - 1000
    end

    -- Frenzy 34970: once-guarded GetHealthPct() < 10 (C++
    -- HealthBelowPct(10)) — DoCast(me) non-triggered self-cast
    -- (C++-exact).
    if not st.frenzyUsed and creature:GetHealthPct() < 10 then
        creature:CastSpell(nil, SPELL_FRENZY)
        st.frenzyUsed = true
    end
end

RegisterCreatureEvent(ENTRY_ROKMAR_THE_CRACKLER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_ROKMAR_THE_CRACKLER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_ROKMAR_THE_CRACKLER, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_ROKMAR_THE_CRACKLER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
