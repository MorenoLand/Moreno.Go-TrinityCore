-- Doctor Theolen Krastinov (11261), Scholomance — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_doctor_theolen_krastinov.cpp (creature AI only; script name
-- "boss_doctor_theolen_krastinov", loader decl AddSC_boss_theolenkrastinov
-- :111 / call :289 follows darkreaver). Entry has no NPC_ constant in
-- scholomance.h (DB-side ScriptName binding; classic wowhead-verified
-- 11261 <The Butcher>). Eluna creature events: 1 OnEnterCombat,
-- 2 OnLeaveCombat, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- OnEnterCombat: rend 16509 at 8s / backhand 18103 at 9s / frenzy 8269
-- at 1s. EVENT_REND and EVENT_BACKHAND are triggered DoCastVictim ->
-- GetVictim + CastSpell(victim, spell, true), nil-victim keeps schedule
-- (moroes/maiden/gandling triggered convention), re-arm 10s both.
-- EVENT_FRENZY: triggered self-cast DoCast(me, 8269, true) ->
-- creature:CastSpell(creature, 8269, true) + Talk(EMOTE_FRENZY_KILL 0),
-- re-arm 120s.
--
-- Documented-unmodeled (no bridges): UpdateAI UNIT_STATE_CASTING queue
-- + post-event gates + BossAI::JustEngagedWith +
-- DATA_DOCTORTHEOLENKRASTINOV=0 bookkeeping (scholomance.h:30)
-- (luaBossAI shim).

local ENTRY_KRASTINOV = 11261

local SPELL_REND = 16509
local SPELL_BACKHAND = 18103
local SPELL_FRENZY = 8269

local EMOTE_FRENZY_KILL = 0

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

-- C++ EVENT_REND: triggered DoCastVictim(16509); Repeat(10s).
local function onRend(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_REND, true)
    end
    schedule(guid, "rend", 10000, function()
        onRend(creature, guid)
    end)
end

-- C++ EVENT_BACKHAND: triggered DoCastVictim(18103); Repeat(10s).
local function onBackhand(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BACKHAND, true)
    end
    schedule(guid, "backhand", 10000, function()
        onBackhand(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: triggered self-cast DoCast(me, 8269, true) +
-- Talk(EMOTE_FRENZY_KILL); Repeat(120s).
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY, true)
    creature:Talk(EMOTE_FRENZY_KILL)
    schedule(guid, "frenzy", 120000, function()
        onFrenzy(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (shim) + the three events.
local function krastinovEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rend", 8000, function()
        onRend(creature, guid)
    end)
    schedule(guid, "backhand", 9000, function()
        onBackhand(creature, guid)
    end)
    schedule(guid, "frenzy", 1000, function()
        onFrenzy(creature, guid)
    end)
end

local function krastinovLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function krastinovDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset (no extra legs in this script).
local function krastinovReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KRASTINOV, 1, krastinovEnterCombat)
RegisterCreatureEvent(ENTRY_KRASTINOV, 2, krastinovLeaveCombat)
RegisterCreatureEvent(ENTRY_KRASTINOV, 4, krastinovDied)
RegisterCreatureEvent(ENTRY_KRASTINOV, 23, krastinovReset)
