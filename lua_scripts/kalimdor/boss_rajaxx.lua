-- Ruins of Ahn'Qiraj: General Rajaxx — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_rajaxx.cpp
-- (boss_rajaxxAI : public BossAI(creature, DATA_RAJAXX);
-- GetAI via GetAQ20AI<boss_rajaxxAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_rajaxx at end
-- registers the boss script + spell_rajaxx_thundercrash;
-- kalimdor loader decl 80 / call 193 per
-- kalimdor_script_loader.cpp — second "// Ruins of ahn'qiraj" loader
-- group, after kurinnaxx, before moam).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_rajaxx" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:42 names NPC_RAJAXX = 15341; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- The C++ while(ExecuteEvent()) + HasUnitState(UNIT_STATE_CASTING) gate
-- has no casting-state bridge in the Lua API (aeonus precedent —
-- unmodeled).
-- Ported arms (the self-contained in-combat legs):
-- - Engage: arm each timer at its C++ ScheduleEvent cooldown (no Talk
--   in C++ — boss_rajaxxAI has zero Talk() calls; the Yells enum
--   belongs to the Andorov event NPCs, unused here).
-- - Disarm 6713 on the victim (C++ DoCastVictim, non-triggered),
--   init 10s -> 22s.
-- - Thunder Crash 25599 self-cast (C++ DoCast(me), non-triggered),
--   init 12s -> 21s.
-- Unmodeled (documented-only, no bridges):
-- - spell_rajaxx_thundercrash (SpellScript, HandleDamageCalc:
--   damage = GetHitUnit()->GetHealth()/2, min 200) — no SpellScript
--   bridge (standing).
-- - EVENT_CHANGE_AGGRO (enum value 3): declared but never scheduled
--   or handled in C++ UpdateAI — dead event, nothing to port.
-- - The `enraged` bool: initialized in Initialize() but never read
--   or set in C++ — dead variable, nothing to port.
-- - BossAI ctor leg DATA_RAJAXX (ruins_of_ahnqiraj.h:29 = 1) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15341

local SPELL_DISARM = 6713
local SPELL_THUNDERCRASH = 25599

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

-- C++ EVENT_DISARM: DoCastVictim(6713), non-triggered, init 10s -> 22s.
local function onDisarm(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM)
    end
    schedule(guid, "disarm", 22000, function()
        onDisarm(creature, guid)
    end)
end

-- C++ EVENT_THUNDERCRASH: DoCast(me, 25599), non-triggered,
-- init 12s -> 21s.
local function onThunderCrash(creature, guid)
    creature:CastSpell(creature, SPELL_THUNDERCRASH)
    schedule(guid, "thundercrash", 21000, function()
        onThunderCrash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "disarm", 10000, function()
        onDisarm(creature, guid)
    end)
    schedule(guid, "thundercrash", 12000, function()
        onThunderCrash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ has no JustDied override for boss_rajaxxAI — cancel only.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
