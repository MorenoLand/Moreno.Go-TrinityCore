-- Quagmirran (The Slave Pens) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- boss_quagmirran.cpp (boss_quagmirran — the only AI class
-- AddSC_boss_quagmirran registers; no other creature/GO scripts
-- in this file). Third and final boss in The Slave Pens set per
-- outland_script_loader.cpp order (mennu, rokmar closed; ahune
-- remains).
-- Entry 17942 verified externally (wowhead npc=17942/quagmirran,
-- mmo4ever creature 17942, mennu/rokmar precedent); no
-- OnCreatureCreate mapping exists for Quagmirran in
-- instance_the_slave_pens.cpp (mennu/morogrim precedent — the
-- instance is a placeholder script that only tracks
-- Ahune/flamecaller/bonfire bunnies); the_slave_pens.h
-- DATA_QUAGMIRRAN = 3 is an instance-side constant
-- (unbridgeable; note the C++ ctor correctly passes DATA_
-- QUAGMIRRAN as the BossAI boss-id — no rokmar-style quirk
-- here). The creature_template ScriptName bindings are DB-side
-- (no TDB in this workspace).
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
-- boss_quagmirran (17942): OnEnterCombat(1): per-GUID reset to
-- the C++ JustEngagedWith schedule values (acidSpray 25000 /
-- cleave 9000 / uppercut 20000 / poisonBoltVolley 31000) + 1s
-- scheduler pump (a port of UpdateAI — 1s granularity is exact
-- for all C++ timers here; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat). Pump:
-- acid spray 38153 25s then {20s,25s} — DoCastAOE
-- (non-triggered self-cast, supremus precedent), re-arm
-- {20s,25s} (C++-exact); cleave 40504 9s then {18s,34s} —
-- triggered DoCastVictim (the C++ true arg), re-arm {18s,34s}
-- (C++-exact); uppercut 32055 20s then 22s — target = random
-- alive player within 10 yd excluding the victim (C++
-- SelectTarget(Random, 1, 10.0f, true), thespia distance-gate
-- convention), nil pick casts nothing, re-arm 22000 regardless
-- (C++-exact); poison bolt volley 34780 31s then 24s — DoCast
-- (me) non-triggered self-cast, re-arm 24000 (C++-exact). The
-- UNIT_STATE_CASTING early returns have no unit-state bridge
-- (mennu/thespia precedent). OnDied(4): cleanup (the _JustDied
-- arm is instance-blocked). OnLeaveCombat(2)/OnReset(23):
-- cancel the pump, drop per-GUID state (the _Reset arm is
-- instance-blocked). Melee is engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_QUAGMIRRAN NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped;
-- instance_the_slave_pens.cpp is a placeholder and stays
-- blocked on the instance-script model); no unit-state bridge
-- — the UNIT_STATE_CASTING early returns in UpdateAI unmodeled
-- (timer bookkeeping exact); no SpellScript/AuraScript scripts
-- in this file. SPELL_POISON_BOLT_VOLLEY carries the upstream
-- "// 39340" comment (alternate id); unmodeled, documented
-- (gruul unused-enum precedent).

local SPELL_ACID_SPRAY = 38153
local SPELL_CLEAVE = 40504
local SPELL_UPPERCUT = 32055
local SPELL_POISON_BOLT_VOLLEY = 34780

local ENTRY_QUAGMIRRAN = 17942

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

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 1, 10.0f, true): random alive player
-- within 10 yd excluding the victim (thespia distance-gate
-- convention). Nil when no other player is in range — the
-- caller skips the cast (C++-exact).
local function pickRandomPlayerIn10YdExcludingVictim(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or nil
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if (not victimGuid or p:GetGUID() ~= victimGuid)
                and creature:GetDistance(p) <= 10.0 then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ ScheduleEvent(event, 20s, 25s) picks uniformly in the
-- range (gruul precedent: math.random(20000, 25000)).
-- JustEngagedWith initial values: acidSpray 25000, cleave
-- 9000, uppercut 20000, poisonBoltVolley 31000.
local function freshState()
    return {
        acidSpray = 25000,
        cleave = 9000,
        uppercut = 20000,
        poisonBoltVolley = 31000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Acid spray 38153: 25s init then {20s,25s} — DoCastAOE
    -- (non-triggered self-cast, supremus precedent), re-arm
    -- {20s,25s} (C++-exact).
    if st.acidSpray <= 1000 then
        creature:CastSpell(nil, SPELL_ACID_SPRAY)
        st.acidSpray = math.random(20000, 25000)
    else
        st.acidSpray = st.acidSpray - 1000
    end

    -- Cleave 40504: 9s init then {18s,34s} — triggered
    -- DoCastVictim (the C++ true arg, gruul precedent), re-arm
    -- {18s,34s} (C++-exact).
    if st.cleave <= 1000 then
        creature:CastSpell(nil, SPELL_CLEAVE, true)
        st.cleave = math.random(18000, 34000)
    else
        st.cleave = st.cleave - 1000
    end

    -- Uppercut 32055: 20s init then 22s — random alive player
    -- within 10 yd excluding the victim (thespia precedent),
    -- nil pick casts nothing, re-arm 22000 regardless
    -- (C++-exact).
    if st.uppercut <= 1000 then
        local target = pickRandomPlayerIn10YdExcludingVictim(creature)
        if target then
            creature:CastSpell(target, SPELL_UPPERCUT)
        end
        st.uppercut = 22000
    else
        st.uppercut = st.uppercut - 1000
    end

    -- Poison bolt volley 34780: 31s init then 24s — DoCast(me)
    -- non-triggered self-cast (C++-exact), re-arm 24000.
    if st.poisonBoltVolley <= 1000 then
        creature:CastSpell(nil, SPELL_POISON_BOLT_VOLLEY)
        st.poisonBoltVolley = 24000
    else
        st.poisonBoltVolley = st.poisonBoltVolley - 1000
    end
end

RegisterCreatureEvent(ENTRY_QUAGMIRRAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_QUAGMIRRAN, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_QUAGMIRRAN, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_QUAGMIRRAN, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
