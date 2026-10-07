-- Maraudon: Princess Theradras — Lua port of
-- src/server/scripts/Kalimdor/Maraudon/boss_princess_theradras.cpp
-- (SD%Complete: 100; class boss_princess_theradras : public
-- CreatureScript { boss_ptheradrasAI : public ScriptedAI }; GetAI via
-- GetMaraudonAI<boss_ptheradrasAI>; AddSC_boss_ptheradras at end
-- registers the one script; kalimdor loader decl 63 / call 176 per
-- kalimdor_script_loader.cpp — the fourth "//Maraudon" loader group,
-- right after AddSC_boss_noxxion()).
-- Entry: maraudon.h carries no NPC_ constants and the instance file
-- never names the boss — 12201 is wowhead-cited for "Princess
-- Theradras" (https://www.wowhead.com/classic/npc=12201/princess-theradras;
-- no NPC_ constant in the C++ tree; celebras / vishas / gelihast
-- precedent); the creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (C++ JustEngagedWith is empty — no Talk): arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm).
-- - Dust Field 21909 self-cast (C++ DoCast(me)), non-triggered (C++
--   2-arg cast), init 8s -> 14s.
-- - Boulder 21832 on SelectTarget(Random, 0) with no range cap ->
--   unbounded random alive player in the instance (nefarian
--   randomAlivePlayer convention), non-triggered, init 2s -> 10s.
-- - Repulsive Gaze 21869 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init 23s -> 20s.
-- - Thrash 3391 self-cast (C++ DoCast(me)), non-triggered, init 5s
--   -> 18s.
-- Unmodeled (documented-only, no bridges):
-- - JustDied's SummonCreature(12238, 28.067, 61.875, -123.405, 4.67,
--   TEMPSUMMON_TIMED_DESPAWN, 10min) — the quest-NPC spawn is summon
--   work with no summon bridge (standing; celebras precedent).
-- - The C++ file declares zero Talk lines and no KilledUnit override
--   — no event-3 registration.

local ENTRY = 12201

local SPELL_DUST_FIELD    = 21909
local SPELL_BOULDER       = 21832
local SPELL_THRASH        = 3391
local SPELL_REPULSIVE_GAZE = 21869

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- nefarian convention: unbounded random alive player in the instance.
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Dustfield: DoCast(me, 21909), non-triggered, init 8s -> 14s.
local function onDustField(creature, guid)
    creature:CastSpell(creature, SPELL_DUST_FIELD)
    schedule(guid, "dustfield", 14000, function()
        onDustField(creature, guid)
    end)
end

-- C++ Boulder: DoCast(SelectTarget(Random, 0), 21832), non-triggered,
-- init 2s -> 10s; jeklik nil-target-keeps-schedule convention (C++
-- skips the cast when the pick is nil but the re-arm is outside).
local function onBoulder(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BOULDER)
    end
    schedule(guid, "boulder", 10000, function()
        onBoulder(creature, guid)
    end)
end

-- C++ Thrash: DoCast(me, 3391), non-triggered, init 5s -> 18s.
local function onThrash(creature, guid)
    creature:CastSpell(creature, SPELL_THRASH)
    schedule(guid, "thrash", 18000, function()
        onThrash(creature, guid)
    end)
end

-- C++ RepulsiveGaze: DoCastVictim(21869), non-triggered, init 23s
-- -> 20s.
local function onRepulsiveGaze(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_REPULSIVE_GAZE)
    end
    schedule(guid, "repulsivegaze", 20000, function()
        onRepulsiveGaze(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "dustfield", 8000, function()
        onDustField(creature, guid)
    end)
    schedule(guid, "boulder", 2000, function()
        onBoulder(creature, guid)
    end)
    schedule(guid, "thrash", 5000, function()
        onThrash(creature, guid)
    end)
    schedule(guid, "repulsivegaze", 23000, function()
        onRepulsiveGaze(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
