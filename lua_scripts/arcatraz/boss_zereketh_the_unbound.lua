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
-- BossAI::JustEngagedWith setActive/ScheduleTasks legs +
-- JustDied/_Reset setActive legs stay blocked; the
-- SetBossState bookkeeping (DATA_ZEREKETH = 0) is PORTED
-- as the (0,1)/(0,0)/(0,3) writes below (lua_creature_
-- events.go:6605; omor/gruul precedent). The base pulse +
-- me-zone arms are PORTED in ev1 below). Pump: void zone 36119 — target = a random
-- threat-list player within 100 yd after skipping the
-- top-threat entry (C++ SelectTarget(Random, 1, 100, true),
-- UnitAI.h:170-197), nil pick casts nothing, non-triggered
-- DoCast on the target (C++-exact), re-arm {6s,10s} regardless
-- (C++-exact); shadow nova 36127 — TRIGGERED DoCastVictim
-- (C++ `DoCastVictim(SPELL_SHADOW_NOVA, true)`, C++-exact),
-- Talk(SAY_SHADOW_NOVA), re-arm {6s,10s} (C++-exact); seed of
-- corruption 36123 — target = same threat-list pick as
-- void zone (same SelectTarget arm),
-- nil pick casts nothing, non-triggered DoCast on the target
-- (C++-exact), re-arm {12s,20s} regardless (C++-exact). The
-- C++ UNIT_STATE_CASTING early-return gates (before and after
-- the event switch, boss_zereketh_the_unbound.cpp:85/:111)
-- are PORTED: the pump decrements all timers first (C++
-- events.Update order), then gates on creature:IsCasting()
-- before the arms and after each fired arm. Melee is
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
-- model; the BossAI ctor DATA_ZEREKETH = 0 SetBossState
-- bookkeeping is PORTED as the (0,1)/(0,0)/(0,3) writes
-- below); no SpellScript/AuraScript scripts
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

-- C++ SelectTarget(Random, 1, 100, true) (boss_zereketh_the_unbound.cpp:93/:103):
-- UnitAI.h builds the list victim-first, so offset 1 removes the
-- victim, not the threat head; Go's threat-sorted list needs the
-- victim-GUID exclusion (nil victim falls back to skipping index 1).
-- The pick is uniform random over the filtered pool (nil when empty,
-- matching the C++ GetThreatListSize() <= offset shortcut).
-- The 100-yd gate uses the IsWithinCombatRange reach twin
-- (Unit.cpp:583; reach-exact twin, player reach 1.5); the alive filter is
-- kept (a dead pick would fail the C++ cast outright).
local function randomNonVictimPlayer(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end

    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetTypeId() == 4 and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < TARGET_RANGE then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
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

    -- C++ events.Update(diff) advances every timer before the
    -- casting gate (boss_zereketh_the_unbound.cpp:82-86), so all
    -- countdowns decrement first; due arms stay due while the
    -- gate holds and fire on the first tick after the cast ends.
    -- Decrement-first + <= 0 fires on the same tick as the old
    -- <= 1000-then-decrement shape.
    st.voidZone = st.voidZone - 1000
    st.shadowNova = st.shadowNova - 1000
    st.seedOfCorruption = st.seedOfCorruption - 1000

    -- C++ pre-loop gate (:85): while casting, no event executes.
    -- creature:IsCasting reads the live CastingUntil window
    -- (broodkeeper precedent).
    if creature:IsCasting() then
        return
    end

    -- Void zone 36119: {6s,10s} — target = random threat-list
    -- player within 100 yd after the top-threat skip, nil pick
    -- casts nothing, non-triggered DoCast on the target (C++-
    -- exact), re-arm {6s,10s} regardless (C++-exact).
    if st.voidZone <= 0 then
        local target = randomNonVictimPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_VOID_ZONE)
        end
        st.voidZone = 6000 + math.random(0, 3999)
    end

    -- C++ in-loop gate (:111): after an event executes, a fresh
    -- cast state stops the tick — later due arms wait for the
    -- next tick (their timers already advanced above).
    if creature:IsCasting() then
        return
    end

    -- Shadow nova 36127: {6s,10s} — TRIGGERED DoCastVictim
    -- (C++ `DoCastVictim(SPELL_SHADOW_NOVA, true)`, C++-exact),
    -- Talk(SAY_SHADOW_NOVA), re-arm {6s,10s} (C++-exact).
    if st.shadowNova <= 0 then
        creature:CastSpell(nil, SPELL_SHADOW_NOVA, true)
        creature:Talk(SAY_SHADOW_NOVA)
        st.shadowNova = 6000 + math.random(0, 3999)
    end

    if creature:IsCasting() then
        return
    end

    -- Seed of corruption 36123: {12s,20s} — target = same
    -- threat-list pick as void zone, nil pick casts nothing,
    -- non-triggered DoCast on the
    -- target (C++-exact), re-arm {12s,20s} regardless (C++-
    -- exact).
    if st.seedOfCorruption <= 0 then
        local target = randomNonVictimPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_SEED_OF_CORRUPTION)
        end
        st.seedOfCorruption = 12000 + math.random(0, 7999)
    end
end

RegisterCreatureEvent(ENTRY_ZEREKETH, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState()
    creature:SetBossState(0, 1) -- DATA_ZEREKETH, IN_PROGRESS (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537; gruul/omor precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 4, function(_, creature)
    creature:SetBossState(0, 3) -- DATA_ZEREKETH, DONE (boss_zereketh_the_unbound.cpp:58-62: _JustDied then Talk)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_ZEREKETH, 23, function(_, creature)
    creature:SetBossState(0, 0) -- DATA_ZEREKETH, NOT_STARTED (C++ Reset _Reset, ScriptedCreature.cpp:499-510)
    resetBoss(creature:GetGUIDLow())
end)
