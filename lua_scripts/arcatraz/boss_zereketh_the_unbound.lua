-- Zereketh the Unbound (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_zereketh_the_unbound.cpp (boss_zereketh_the_
-- unbound — the only CreatureScript AI class AddSC_boss_
-- zereketh_the_unbound registers; no SpellScript/AuraScript
-- scripts in this file).
-- First boss in the Arcatraz set per outland_script_loader.cpp
-- order (instance_arcatraz.cpp stays blocked on the instance-
-- script model).
-- Entry: arcatraz.h carries no NPC_ constant for Zereketh (it
-- lists only Dalliah 20885 / Soccothrates 20886 / Mellichar
-- 20904 / Millhouse 20977 / Alpha Pod Target 21436) and
-- instance_arcatraz.cpp has no OnCreatureCreate mapping for
-- the boss, so the entry is verified externally: wowhead
-- npc=20870/zereketh-the-unbound (also cited as npc=20870 on
-- tbc.wowhead and wow-mania). The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 1 (kill — the C++
-- KilledUnit talks unconditionally, no player gate, gargolmar
-- precedent, C++-exact), SAY_SHADOW_NOVA = 2 (shadow nova —
-- fired from the EVENT_SHADOW_NOVA arm, C++-exact), SAY_DEATH
-- = 3 (death — fired from the C++ JustDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Zereketh
-- (20870): OnEnterCombat(1): schedule void zone {6s,10s} /
-- shadow nova {6s,10s} / seed of corruption {12s,20s} + Talk
-- (SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump; the
-- BossAI::JustEngagedWith/JustDied/_Reset arms are instance-
-- blocked, DATA_ZEREKETH = 0 is an instance-side constant,
-- unbridgeable). Pump: void zone 36119 — target = a random
-- alive player in the instance within 100 yd, excluding the
-- current victim (C++ SelectTarget(Random, 1, 100, true),
-- supremus precedent), nil pick casts nothing, non-triggered
-- DoCast on the target (C++-exact), re-arm {6s,10s} regardless
-- (C++-exact); shadow nova 36127 — TRIGGERED DoCastVictim
-- (C++ `DoCastVictim(SPELL_SHADOW_NOVA, true)`, C++-exact),
-- Talk(SAY_SHADOW_NOVA), re-arm {6s,10s} (C++-exact); seed of
-- corruption 36123 — target = random alive player within 100
-- yd excluding the current victim (same SelectTarget arm),
-- nil pick casts nothing, non-triggered DoCast on the target
-- (C++-exact), re-arm {12s,20s} regardless (C++-exact). The
-- C++ UNIT_STATE_CASTING early-return gates (before and after
-- the event switch) have no cast-state bridge — the arms fire
-- unconditionally (netherspite precedent). Melee is
-- engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, gargolmar precedent, C++-exact). OnDied(4):
-- Talk(SAY_DEATH) + cleanup (the _JustDied arm is instance-
-- blocked). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the C++ Reset() has no observable
-- remainder beyond the _Reset arm; the engage schedule
-- re-latches on OnEnterCombat).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_arcatraz.cpp stays blocked on the instance-script
-- model; the BossAI ctor DATA_ZEREKETH = 0 bookkeeping in
-- Reset/JustEngagedWith/JustDied skipped); no cast-state
-- bridge — the UNIT_STATE_CASTING early-return gates unmodeled
-- (netherspite precedent); no SpellScript/AuraScript scripts
-- in this file.

local SPELL_VOID_ZONE = 36119
local SPELL_SHADOW_NOVA = 36127
local SPELL_SEED_OF_CORRUPTION = 36123

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SHADOW_NOVA = 2
local SAY_DEATH = 3

local ENTRY_ZEREKETH = 20870

local TARGET_RANGE = 100

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

-- C++ JustEngagedWith schedule (the BossAI arm is instance
-- blocked, so the whole engagement schedule lands on
-- OnEnterCombat — omor precedent): void zone {6s,10s} /
-- shadow nova {6s,10s} / seed of corruption {12s,20s}.
local function freshState()
    return {
        voidZone = 6000 + math.random(0, 3999),
        shadowNova = 6000 + math.random(0, 3999),
        seedOfCorruption = 12000 + math.random(0, 7999),
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

-- C++ SelectTarget(Random, 1, 100, true): a random alive player
-- in the instance within 100 yd, excluding the current victim
-- (supremus precedent), player-only.
local function randomNonVictimPlayer(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if (not victim or p:GetGUID() ~= victim:GetGUID())
                and creature:GetDistance(p) <= TARGET_RANGE then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Void zone 36119: {6s,10s} — target = random alive player
    -- within 100 yd excluding the current victim, nil pick
    -- casts nothing, non-triggered DoCast on the target (C++-
    -- exact), re-arm {6s,10s} regardless (C++-exact).
    if st.voidZone <= 1000 then
        local target = randomNonVictimPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_VOID_ZONE)
        end
        st.voidZone = 6000 + math.random(0, 3999)
    else
        st.voidZone = st.voidZone - 1000
    end

    -- Shadow nova 36127: {6s,10s} — TRIGGERED DoCastVictim
    -- (C++ `DoCastVictim(SPELL_SHADOW_NOVA, true)`, C++-exact),
    -- Talk(SAY_SHADOW_NOVA), re-arm {6s,10s} (C++-exact). The
    -- C++ UNIT_STATE_CASTING early-return gate has no
    -- cast-state bridge — the arm fires unconditionally
    -- (netherspite precedent).
    if st.shadowNova <= 1000 then
        creature:CastSpell(nil, SPELL_SHADOW_NOVA, true)
        creature:Talk(SAY_SHADOW_NOVA)
        st.shadowNova = 6000 + math.random(0, 3999)
    else
        st.shadowNova = st.shadowNova - 1000
    end

    -- Seed of corruption 36123: {12s,20s} — target = random
    -- alive player within 100 yd excluding the current victim,
    -- nil pick casts nothing, non-triggered DoCast on the
    -- target (C++-exact), re-arm {12s,20s} regardless (C++-
    -- exact).
    if st.seedOfCorruption <= 1000 then
        local target = randomNonVictimPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_SEED_OF_CORRUPTION)
        end
        st.seedOfCorruption = 12000 + math.random(0, 7999)
    else
        st.seedOfCorruption = st.seedOfCorruption - 1000
    end
end

RegisterCreatureEvent(ENTRY_ZEREKETH, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
