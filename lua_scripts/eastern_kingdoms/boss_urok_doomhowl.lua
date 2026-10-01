-- Urok Doomhowl (Blackrock Spire; Doomhowl's hall) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_urok_doomhowl.cpp
-- (boss_urok_doomhowl — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_gizrul_the_slavener
-- done, registered for 10268; AddSC_boss_urok_doomhowl next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h:64 names
-- NPC_UROK_DOOMHOWL = 10584 in the BRS creatures enum; the AI runs
-- under BossAI(DATA_UROK_DOOMHOWL = 4). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace), but the
-- entry itself is C++-verifiable so the port is registered
-- (gizrul / rend_blackhand precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h); the combat arms have no instance state.
-- Reset() _Reset() / JustDied _JustDied() are internal machinery
-- covered here by the cancel on combat events (halycon precedent).
-- SPELL_INTIMIDATING_ROAR (16508) is defined in the file's spell enum
-- but never cast by the AI — omitted by design (shadowvosh ice-armor
-- precedent). SAY_SUMMON (0) is never spoken by the AI — enum-only.
-- The EVENT_REND / EVENT_STRIKE / EVENT_INTIMIDATING_ROAR constants
-- are never scheduled; the C++ file schedules the spell ids
-- (SPELL_REND / SPELL_STRIKE) as the event ids directly, and the
-- switch matches on them. The port keys timers "rend"/"strike" —
-- scheduling a live key cancels the existing timer first (EventMap
-- semantics; voone pummel precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- SPELL_REND: victim-cast 16509, 17s init -> 8s loop
-- (C++ urand(17s,20s) init / urand(8s,10s) loop — ranges use the
-- lower bound, halycon recurring-timer convention).
-- SPELL_STRIKE: victim-cast 15580, 10s init -> 8s loop
-- (C++ urand(10s,12s) init / urand(8s,10s) loop — lower bound).
-- Talk(SAY_AGGRO) fires once on combat entry (maiden-of-virtue
-- convention; creature:Talk is bridged in lua_creature_events.go).
-- Victim casts are GetVictim nil-guarded (incarcerator convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention). BossAI::JustEngagedWith(who) is the standard
-- threat/aggro entry already modeled engine-side in Go.

local SPELL_REND = 16509
local SPELL_STRIKE = 15580
local SAY_AGGRO = 1

local ENTRY_UROK_DOOMHOWL = 10584

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

local function onRend(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_REND)
    end
    schedule(guid, "rend", 8000, function() onRend(creature, guid) end)
end

local function onStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_STRIKE)
    end
    schedule(guid, "strike", 8000, function() onStrike(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "rend", 17000, function() onRend(creature, guid) end)
    schedule(guid, "strike", 10000, function() onStrike(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_UROK_DOOMHOWL, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_UROK_DOOMHOWL, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_UROK_DOOMHOWL, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_UROK_DOOMHOWL, 23, onCombatEnd)
