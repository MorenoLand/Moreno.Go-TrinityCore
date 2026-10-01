-- Warmaster Voone (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_warmaster_voone.cpp
-- (boss_warmaster_voone — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_the_beast done,
-- registered for 10430; AddSC_boss_warmastervoone next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_WARMASTER_VOONE = 9237 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_the_beast / boss_shadow_hunter_voshgajin
-- precedent). GetBlackrockSpireAI is a GetInstanceAI retrieval
-- wrapper only (blackrock_spire.h:129-132); the AI has no instance
-- arms — Reset() _Reset() / JustDied _JustDied() are internal
-- machinery covered here by the cancel/re-arm on combat events
-- (halycon precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_SNAP_KICK: victim-cast SNAPKICK 15618, 8s init -> 6s loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_CLEAVE: victim-cast CLEAVE 15284, 14s init -> 12s loop
-- (maiden convention).
-- EVENT_UPPERCUT: victim-cast UPPERCUT 10966, 20s init -> 14s loop
-- (maiden convention).
-- EVENT_MORTAL_STRIKE: victim-cast MORTALSTRIKE 16856, 12s init ->
-- 10s loop (maiden convention).
-- EVENT_PUMMEL: victim-cast PUMMEL 15615, one-shot at 32s —
-- C++-exact: the EVENT_PUMMEL handler does NOT re-arm EVENT_PUMMEL;
-- it re-arms EVENT_MORTAL_STRIKE at 16s (the file's own switch arm,
-- likely an upstream copy-paste but faithfully replicated). The Lua
-- surface models EventMap semantics: scheduling mortalstrike replaces
-- the live mortalstrike timer (the stale event id is cancelled first),
-- so after pummel fires once the mortal-strike cadence becomes 16s
-- instead of 10s, exactly as the C++ EventMap overwrite does.
-- EVENT_THROW_AXE: victim-cast THROWAXE 16075, 1s init -> 8s loop
-- (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_SNAPKICK = 15618
local SPELL_CLEAVE = 15284
local SPELL_UPPERCUT = 10966
local SPELL_MORTALSTRIKE = 16856
local SPELL_PUMMEL = 15615
local SPELL_THROWAXE = 16075

local ENTRY_WARMASTER_VOONE = 9237

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

local function onSnapKick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SNAPKICK)
    end
    schedule(guid, "snapkick", 6000, function() onSnapKick(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 12000, function() onCleave(creature, guid) end)
end

local function onUppercut(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_UPPERCUT)
    end
    schedule(guid, "uppercut", 14000, function() onUppercut(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALSTRIKE)
    end
    schedule(guid, "mortalstrike", 10000, function() onMortalStrike(creature, guid) end)
end

local function onPummel(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PUMMEL)
    end
    -- C++-exact: EVENT_PUMMEL's handler does not re-arm EVENT_PUMMEL;
    -- it re-arms EVENT_MORTAL_STRIKE at 16s, replacing the 10s-loop
    -- timer (EventMap overwrite). So mortal-strike fires again 16s
    -- after pummel, then resumes its own 10s loop.
    schedule(guid, "mortalstrike", 16000, function() onMortalStrike(creature, guid) end)
end

local function onThrowAxe(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THROWAXE)
    end
    schedule(guid, "throwaxe", 8000, function() onThrowAxe(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "snapkick", 8000, function() onSnapKick(creature, guid) end)
    schedule(guid, "cleave", 14000, function() onCleave(creature, guid) end)
    schedule(guid, "uppercut", 20000, function() onUppercut(creature, guid) end)
    schedule(guid, "mortalstrike", 12000, function() onMortalStrike(creature, guid) end)
    schedule(guid, "pummel", 32000, function() onPummel(creature, guid) end)
    schedule(guid, "throwaxe", 1000, function() onThrowAxe(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WARMASTER_VOONE, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WARMASTER_VOONE, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARMASTER_VOONE, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARMASTER_VOONE, 23, onCombatEnd)
