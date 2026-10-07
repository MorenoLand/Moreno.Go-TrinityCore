-- The Ravenian (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_the_ravenian.cpp
-- (boss_theravenianAI only; CreatureScript name "boss_the_ravenian";
-- EK loader AddSC_boss_theravenian decl :119 / call :297 follows
-- rasfrost :118/:296). No NPC_ constant in scholomance.h —
-- RegisterLuaBoss binds the creature_template ScriptName
-- "boss_the_ravenian" DB-side; entry wowhead-verified: 10507.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- trample 15550 24s->10s / cleave 20691 15s->7s / sundering cleave
-- 25174 40s->20s / knock away 10101 32s->12s (all triggered
-- DoCastVictim -> GetVictim + CastSpell(victim, spell, true),
-- moroes/maiden convention; nil-victim ticks cast nothing but
-- keep the schedule). Zero Talk lines in C++ (no _SAY enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention).
-- Documented-only (no bridges): GetScholomanceAI template instance
-- validation (luaBossAI shim); BossAI::JustEngagedWith + DATA_THERAVENIAN=5
-- bookkeeping (scholomance.h:35) via luaBossAI shim.

local ENTRY_THE_RAVENIAN = 10507

local SPELL_TRAMPLE = 15550
local SPELL_CLEAVE = 20691
local SPELL_SUNDERING_CLEAVE = 25174
local SPELL_KNOCK_AWAY = 10101

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ DoCastVictim(spell, true) -> GetVictim + CastSpell(victim, spell, true);
-- nil-victim tick casts nothing but keeps the schedule (moroes/maiden
-- convention).
local function onVictimTimer(creature, guid, spell, key, rearm, fn)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell, true)
    end
    schedule(guid, key, rearm, function()
        fn(creature, guid)
    end)
end

local function onTrample(creature, guid)
    onVictimTimer(creature, guid, SPELL_TRAMPLE, "trample", 10000, onTrample)
end

local function onCleave(creature, guid)
    onVictimTimer(creature, guid, SPELL_CLEAVE, "cleave", 7000, onCleave)
end

local function onSunderingCleave(creature, guid)
    onVictimTimer(creature, guid, SPELL_SUNDERING_CLEAVE, "sundering_cleave", 20000, onSunderingCleave)
end

local function onKnockAway(creature, guid)
    onVictimTimer(creature, guid, SPELL_KNOCK_AWAY, "knock_away", 12000, onKnockAway)
end

local function theRavenianResetState(guid)
    cancelTimers(guid)
end

local function theRavenianReset(event, creature)
    theRavenianResetState(creature:GetGUID())
end

local function theRavenianEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    theRavenianResetState(guid)
    schedule(guid, "trample", 24000, function()
        onTrample(creature, guid)
    end)
    schedule(guid, "cleave", 15000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "sundering_cleave", 40000, function()
        onSunderingCleave(creature, guid)
    end)
    schedule(guid, "knock_away", 32000, function()
        onKnockAway(creature, guid)
    end)
end

local function theRavenianLeaveCombat(event, creature)
    theRavenianResetState(creature:GetGUID())
end

local function theRavenianDied(event, creature, killer)
    theRavenianResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_THE_RAVENIAN, 1, theRavenianEnterCombat)
RegisterCreatureEvent(ENTRY_THE_RAVENIAN, 2, theRavenianLeaveCombat)
RegisterCreatureEvent(ENTRY_THE_RAVENIAN, 4, theRavenianDied)
RegisterCreatureEvent(ENTRY_THE_RAVENIAN, 23, theRavenianReset)
