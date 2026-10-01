-- Gizrul the Slavener (Blackrock Spire; Gizrul's lair) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_gizrul_the_slavener.cpp
-- (boss_gizrul_the_slavener — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_rend_blackhand done,
-- registered for 10429; AddSC_boss_gizrul_the_slavener next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h:66 names
-- NPC_GIZRUL_THE_SLAVENER = 10268 in the BRS creatures enum; the AI runs
-- under BossAI(DATA_GIZRUL_THE_SLAVENER = 6). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace), but the entry
-- itself is C++-verifiable so the port is registered (gyth / rend_blackhand
-- precedent). Note: halycon's JustDied summon of this creature was
-- unbridgeable (no SummonCreature bridge), but Gizrul's own entry is
-- verifiable here, so registration is valid.
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h); the combat arms have no instance state.
-- Reset() _Reset() / JustDied _JustDied() are internal machinery
-- covered here by the cancel on combat events (halycon precedent).
-- SPELL_FRENZY (8269) is defined in the file's spell enum but never
-- cast by the AI — omitted by design (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FATAL_BITE: victim-cast FATAL_BITE 16495, 17s init -> 8s loop
-- (C++ urand(17s,20s) init / urand(8s,10s) loop — ranges use the
-- lower bound, halycon recurring-timer convention).
-- EVENT_INFECTED_BITE: one-shot self-cast INFECTED_BITE 16128 at 10s
-- (C++ urand(10s,12s) — lower bound). The C++ handler does NOT re-arm
-- EVENT_INFECTED_BITE; it schedules EVENT_FATAL_BITE at 8s-10s, so the
-- arm fires exactly once and overwrites the live fatal-bite timer
-- (EventMap semantics: schedule() cancels the same-key timer first —
-- the same modeling as voone's pummel overwriting mortal-strike).
-- Victim casts are GetVictim nil-guarded (incarcerator convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).
-- Documented-only (no bridges on the Lua surface): IsSummonedBy's
-- MovePath(GIZRUL_PATH = 402450) gates on MotionMaster (the_beast
-- MovePath precedent); GIZRUL_PATH is enum-only here.

local SPELL_FATAL_BITE = 16495
local SPELL_INFECTED_BITE = 16128

local ENTRY_GIZRUL_THE_SLAVENER = 10268

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

local function onFatalBite(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FATAL_BITE)
    end
    schedule(guid, "fatalbite", 8000, function() onFatalBite(creature, guid) end)
end

local function onInfectedBite(creature, guid)
    creature:CastSpell(creature, SPELL_INFECTED_BITE)
    schedule(guid, "fatalbite", 8000, function() onFatalBite(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fatalbite", 17000, function() onFatalBite(creature, guid) end)
    schedule(guid, "infectedbite", 10000, function() onInfectedBite(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 23, onCombatEnd)
