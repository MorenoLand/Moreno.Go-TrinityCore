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
-- precedent). Note: halycon's JustDied summon of this creature (JustDied ->
-- me->SummonCreature(10268, SummonLocation, TEMPSUMMON_TIMED_
-- DESPAWN, 5min) then Talk) is ported in eastern_kingdoms/
-- boss_halycon.lua:97 (onDied) — 'unbridgeable' is stale, so
-- Gizrul's own entry is verifiable here and registration is valid.
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h).
-- SetBossState legs (DATA_GIZRUL_THE_SLAVENER = 6, blackrock_spire.h:36;
-- EncounterCount = 23, blackrock_spire.h:23; instance calls
-- SetBossNumber(23), instance_blackrock_spire.cpp:62): ev1 IN_PROGRESS
-- via BossAI::JustEngagedWith (boss_gizrul_the_slavener.cpp:64 base call
-- first; ScriptedCreature.cpp:537); ev23 NOT_STARTED via Reset's _Reset()
-- (:54; ScriptedCreature.cpp:499-512, alive-gated, skipped when DONE);
-- ev4 DONE via JustDied's _JustDied() (:71; ScriptedCreature.cpp:513-520).
-- The _Reset()/_JustDied() bookkeeping (timers/summons) is covered by the
-- cancel on combat events (halycon precedent).
-- SPELL_FRENZY (8269) is defined in the file's spell enum but never
-- cast by the AI — omitted by design (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FATAL_BITE: victim-cast FATAL_BITE 16495, 17s init -> 8s loop
-- (C++ urand(17s,20s) init / urand(8s,10s) loop — CreateLuaEvent
-- {min,max} tables, maiden convention).
-- EVENT_INFECTED_BITE: one-shot self-cast INFECTED_BITE 16128 at 10s
-- (C++ urand(10s,12s)). The C++ handler does NOT re-arm
-- EVENT_INFECTED_BITE; it schedules EVENT_FATAL_BITE at 8s-10s, so the
-- arm fires exactly once and overwrites the live fatal-bite timer
-- (EventMap semantics: schedule() cancels the same-key timer first —
-- the same modeling as voone's pummel overwriting mortal-strike).
-- Victim casts are GetVictim nil-guarded (incarcerator convention).
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:81) + in-loop
-- gate (:100) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis/rend idiom) -- a
-- gated arm fires nothing and consumes no re-arm. The C++
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
    if creature:IsCasting() then
        schedule(guid, "fatalbite", 500, function() onFatalBite(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FATAL_BITE)
    end
    schedule(guid, "fatalbite", {8000, 10000}, function() onFatalBite(creature, guid) end)
end

local function onInfectedBite(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "infectedbite", 500, function() onInfectedBite(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_INFECTED_BITE)
    schedule(guid, "fatalbite", {8000, 10000}, function() onFatalBite(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_gizrul_the_slavener.cpp:64 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_gizrul_the_slavener.cpp:64 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_GIZRUL_THE_SLAVENER=6 (blackrock_spire.h:36).
    creature:SetBossState(6, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "fatalbite", {17000, 20000}, function() onFatalBite(creature, guid) end)
    schedule(guid, "infectedbite", {10000, 12000}, function() onInfectedBite(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_gizrul_the_slavener.cpp:52-55): _Reset() writes
-- NOT_STARTED when alive and the state is not DONE (ScriptedCreature.cpp:499-512).
-- Alive-gated: Go fires ev23 on the death path too (AQ40/RFD convention).
local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(6) ~= 3 then
        creature:SetBossState(6, 0)
    end
end

-- C++ JustDied (boss_gizrul_the_slavener.cpp:69-72): _JustDied() writes DONE
-- (ScriptedCreature.cpp:513-520).
local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(6, 3)
end

RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 4, onDied)
RegisterCreatureEvent(ENTRY_GIZRUL_THE_SLAVENER, 23, onReset)
