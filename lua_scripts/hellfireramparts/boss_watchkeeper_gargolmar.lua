-- Watchkeeper Gargolmar (Hellfire Ramparts, Hellfire Citadel) --
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- HellfireRamparts/boss_watchkeeper_gargolmar.cpp (boss_
-- watchkeeper_gargolmar — the only CreatureScript AI class
-- AddSC_boss_watchkeeper_gargolmar registers; no SpellScript/
-- AuraScript scripts in this file).
-- First boss in the Hellfire Ramparts set per
-- outland_script_loader.cpp order (instance_hellfire_
-- ramparts.cpp stays blocked on the instance-script model).
-- Entry: no NPC_ constant for Gargolmar exists in hellfire_
-- ramparts.h and instance_hellfire_ramparts.cpp has no
-- OnCreatureCreate mapping for the boss, so the entry is
-- verified externally: wowhead npc=17306/watchkeeper-
-- gargolmar (also the wotlk variant and several azerothcore
-- issue threads citing entry 17306). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_AGGRO = 3 (pull — fired from the C++ JustEngaged
-- With, C++-exact), SAY_SURGE = 2 (surge — fired from the
-- EVENT_SURGE arm, C++-exact), SAY_KILL = 4 (kill — the C++
-- KilledUnit override talks unconditionally, no player gate,
-- C++-exact nethekurse precedent), SAY_DIE = 5 (death —
-- fired from the C++ JustDied, C++-exact), SAY_HEAL = 1
-- (health < 40% — fired once from the yelledForHeal arm,
-- C++-exact); SAY_TAUNT = 0 fires only from the
-- MoveInLineOfSight 60yd proximity arm — no LoS-aggro
-- bridge (fel orc convert precedent), unreachable in this
-- model.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Gargolmar
-- (17306): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (hasTaunted false / yelledForHeal
-- false / retaliation false; the JustEngagedWith schedule —
-- mortalWound 5000 / surge 4000 — drives the pump) + Talk
-- (SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump; the
-- BossAI::JustEngagedWith/JustDied/_Reset arms are instance-
-- blocked, DATA_WATCHKEEPER_GARGOLMAR = 0 is an instance-
-- side constant, unbridgeable).
-- Pump: mortal wound 30641 — 5s init then {5s,13s},
-- non-triggered DoCastVictim (felmyst convention), re-arm
-- {5s,13s} regardless (C++-exact); surge 34645 — 4s init
-- then {5s,13s}, Talk(SAY_SURGE), target = random alive
-- player in the instance (C++ SelectTarget(Random, 0),
-- thespia precedent), nil pick casts nothing, non-triggered
-- DoCast on the target (C++-exact), re-arm {5s,13s}
-- regardless (C++-exact); retaliation 22857 — scheduled 1s
-- after health drops strictly below 20% (C++ HealthBelowPct
-- (20), halazzi strict-fraction precedent), non-triggered
-- DoCastSelf (C++ DoCast(me), C++-exact), re-arm 30s,
-- retaliation latch set once (C++-exact); heal yell — Talk
-- (SAY_HEAL) once when health drops strictly below 40% (C++
-- HealthBelowPct(40), C++-exact). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) (C++ talks
-- unconditionally — no player gate, C++-exact). OnDied(4):
-- Talk(SAY_DIE) + cleanup (the _JustDied arm is instance-
-- blocked). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the Reset() observable remainder is
-- the Initialize() re-latch, which lands on OnEnterCombat;
-- the _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_hellfire_ramparts.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_WATCHKEEPER_
-- GARGOLMAR = 0 bookkeeping in Reset/JustEngagedWith/
-- JustDied skipped); no LoS-aggro bridge — the
-- MoveInLineOfSight SAY_TAUNT arm (and the whole AttackStart
-- proximity machine) unmodeled, so SAY_TAUNT is unreachable;
-- the ScriptData "SD%Complete: 80" caveats are upstream,
-- documented, not bridged: the "missing adds to heal him"
-- machine has no summon arms in the C++ at all, and the
-- "Surge should be used on target furthest away, not
-- random" note is modeled C++-exact (random target); no
-- movement bridge — no movement arms in this file; no
-- SpellScript/AuraScript scripts in this file.

local SPELL_MORTAL_WOUND = 30641
local SPELL_SURGE = 34645
local SPELL_RETALIATION = 22857

local SAY_TAUNT = 0
local SAY_HEAL = 1
local SAY_SURGE = 2
local SAY_AGGRO = 3
local SAY_KILL = 4
local SAY_DIE = 5

local ENTRY_GARGOLMAR = 17306

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

-- C++ Initialize() values (the BossAI ctor instance arms are
-- instance blocked, so the whole engagement schedule lands
-- on OnEnterCombat — broggok precedent): mortalWound 5000 /
-- surge 4000 / retaliation timer unscheduled until the
-- health gate trips / retaliation false / yelledForHeal
-- false.
local function freshState()
    return {
        mortalWound = 5000,
        surge = 4000,
        retaliationT = 0,
        retaliation = false,
        yelledForHeal = false,
    }
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

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0
        and creature:GetHealth() * 100 / maxHealth < pct
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Mortal wound 30641: 5s init then {5s,13s} — non-
    -- triggered DoCastVictim (felmyst convention), re-arm
    -- {5s,13s} regardless (C++-exact).
    if st.mortalWound <= 1000 then
        creature:CastSpell(nil, SPELL_MORTAL_WOUND)
        st.mortalWound = math.random(5000, 13000)
    else
        st.mortalWound = st.mortalWound - 1000
    end

    -- Surge 34645: 4s init then {5s,13s} — Talk(SAY_SURGE),
    -- target = random alive player in the instance (C++
    -- SelectTarget(Random, 0), thespia precedent), nil pick
    -- casts nothing, non-triggered DoCast on the target
    -- (C++-exact), re-arm {5s,13s} regardless (C++-exact).
    if st.surge <= 1000 then
        creature:Talk(SAY_SURGE)
        local players = playersInInstance(creature)
        if #players > 0 then
            creature:CastSpell(players[math.random(#players)],
                SPELL_SURGE)
        end
        st.surge = math.random(5000, 13000)
    else
        st.surge = st.surge - 1000
    end

    -- Retaliation 22857: scheduled 1s after health first
    -- drops strictly below 20% (C++-exact latch), then 30s
    -- — non-triggered DoCastSelf (C++ DoCast(me),
    -- C++-exact).
    if st.retaliation then
        if st.retaliationT <= 1000 then
            creature:CastSpell(nil, SPELL_RETALIATION)
            st.retaliationT = 30000
        else
            st.retaliationT = st.retaliationT - 1000
        end
    end

    if not st.retaliation and healthBelowPct(creature, 20) then
        st.retaliation = true
        st.retaliationT = 1000
    end

    -- Heal yell: Talk(SAY_HEAL) once when health first drops
    -- strictly below 40% (C++-exact).
    if not st.yelledForHeal and healthBelowPct(creature, 40) then
        creature:Talk(SAY_HEAL)
        st.yelledForHeal = true
    end
end

RegisterCreatureEvent(ENTRY_GARGOLMAR, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- No LoS-aggro bridge: the MoveInLineOfSight SAY_TAUNT arm
-- has no hook in this model (fel orc convert precedent).

RegisterCreatureEvent(ENTRY_GARGOLMAR, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GARGOLMAR, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (nethekurse precedent, C++-exact).
    creature:Talk(SAY_KILL)
end)

RegisterCreatureEvent(ENTRY_GARGOLMAR, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_GARGOLMAR, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
