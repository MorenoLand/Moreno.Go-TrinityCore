-- The Maker (Blood Furnace, Hellfire Citadel) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_the_maker.cpp (boss_the_maker — the only CreatureScript AI
-- class AddSC_boss_the_maker registers; no other creature/GO
-- scripts in this file).
-- Final boss in the Blood Furnace set per outland_script_loader.
-- cpp order (instance_blood_furnace.cpp stays blocked on the
-- instance-script model).
-- Entry: NPC_THE_MAKER = 17381 in blood_furnace.h (mapped in
-- instance_blood_furnace.cpp OnCreatureCreate — TheMakerGUID);
-- the creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++ JustEngagedWith,
-- C++-exact), SAY_KILL = 1 (kill — the C++ gates with
-- who->GetTypeId() == TYPEID_PLAYER, terestian/gurtogg
-- 3-arg-handler convention, C++-exact), SAY_DIE = 2 (death).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): The Maker
-- (17381): OnEnterCombat(1): per-GUID reset to the C++
-- JustEngagedWith schedule values (acidSpray 15000 /
-- explodingBreaker 6000 / domination 120000 / knockdown
-- 10000) + Talk(SAY_AGGRO) + 1s scheduler pump (a port of
-- ExecuteEvent — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump).
-- Pump: acid spray 38153: 15s init then 15000+rand32()%8000
-- (C++ {15s,23s}) — non-triggered DoCastVictim (felmyst
-- convention), re-arm {15s,23s} regardless (C++-exact);
-- exploding breaker 30925: 6s init then {4s,12s} — target =
-- random alive player within 30 yd (C++ SelectTarget(Random,
-- 0, 30.0f, true), position arg 0 = victim not excluded,
-- thespia distance-gate convention), nil pick casts nothing,
-- non-triggered DoCast on the target (C++ DoCast(target),
-- C++-exact), re-arm {4s,12s} regardless (C++-exact);
-- domination 25772: 120s init then 120s — target = random
-- alive player, no distance limit (C++ SelectTarget(Random, 0,
-- 0.0f, true), thespia precedent), nil pick casts nothing,
-- non-triggered DoCast on the target (C++-exact), re-arm
-- 120000 regardless (C++-exact); knockdown 20276: 10s init
-- then {4s,12s} — non-triggered DoCastVictim (C++-exact),
-- re-arm {4s,12s} regardless (C++-exact). OnTargetDied(3):
-- Talk(SAY_KILL) gated on the killed unit being a player
-- (C++-exact). OnDied(4): Talk(SAY_DIE) + cleanup (the
-- _JustDied arm is instance-blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the
-- _Reset arm is instance-blocked). Melee is engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_blood_furnace.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_THE_MAKER = 0
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_THE_MAKER is an instance-side constant, unbridgeable);
-- no SpellScript/AuraScript scripts in this file.

local SPELL_ACID_SPRAY = 38153
local SPELL_EXPLODING_BREAKER = 30925
local SPELL_KNOCKDOWN = 20276
local SPELL_DOMINATION = 25772

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DIE = 2

local ENTRY_THE_MAKER = 17381

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

-- C++ SelectTarget(Random, 0, 30.0f, true): random alive player
-- within 30 yd, victim NOT excluded (thespia distance-gate
-- convention). Nil when no player is in range — the caller
-- skips the cast (C++-exact).
local function pickRandomPlayerIn30Yd(creature)
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 30.0 then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ ScheduleEvent(event, 15s, 23s) picks uniformly in the
-- range (gruul precedent: math.random(15000, 23000)).
-- JustEngagedWith initial values: acidSpray 15000,
-- explodingBreaker 6000, domination 120000, knockdown 10000.
local function freshState()
    return {
        acidSpray = 15000,
        explodingBreaker = 6000,
        domination = 120000,
        knockdown = 10000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Acid spray 38153: 15s init then {15s,23s} — non-triggered
    -- DoCastVictim (felmyst convention), re-arm {15s,23s}
    -- regardless (C++-exact).
    if st.acidSpray <= 1000 then
        creature:CastSpell(nil, SPELL_ACID_SPRAY)
        st.acidSpray = math.random(15000, 23000)
    else
        st.acidSpray = st.acidSpray - 1000
    end

    -- Exploding breaker 30925: 6s init then {4s,12s} — random
    -- alive player within 30 yd (thespia distance-gate
    -- convention), nil pick casts nothing, non-triggered
    -- DoCast on the target (C++-exact), re-arm {4s,12s}
    -- regardless (C++-exact).
    if st.explodingBreaker <= 1000 then
        local target = pickRandomPlayerIn30Yd(creature)
        if target then
            creature:CastSpell(target, SPELL_EXPLODING_BREAKER)
        end
        st.explodingBreaker = math.random(4000, 12000)
    else
        st.explodingBreaker = st.explodingBreaker - 1000
    end

    -- Domination 25772: 120s init then 120s — random alive
    -- player in the instance, no distance limit (thespia
    -- precedent), nil pick casts nothing, non-triggered
    -- DoCast on the target (C++-exact), re-arm 120000
    -- regardless (C++-exact).
    if st.domination <= 1000 then
        local players = playersInInstance(creature)
        if #players > 0 then
            creature:CastSpell(players[math.random(#players)], SPELL_DOMINATION)
        end
        st.domination = 120000
    else
        st.domination = st.domination - 1000
    end

    -- Knockdown 20276: 10s init then {4s,12s} — non-triggered
    -- DoCastVictim (C++-exact), re-arm {4s,12s} regardless
    -- (C++-exact).
    if st.knockdown <= 1000 then
        creature:CastSpell(nil, SPELL_KNOCKDOWN)
        st.knockdown = math.random(4000, 12000)
    else
        st.knockdown = st.knockdown - 1000
    end
end

RegisterCreatureEvent(ENTRY_THE_MAKER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_THE_MAKER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_THE_MAKER, 3, function(_, creature, victim)
    -- C++ `if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL)`
    -- — terestian/gurtogg 3-arg-handler convention (C++-exact).
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end)

RegisterCreatureEvent(ENTRY_THE_MAKER, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_THE_MAKER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
