-- Maraudon: Celebras the Cursed — Lua port of
-- src/server/scripts/Kalimdor/Maraudon/boss_celebras_the_cursed.cpp
-- (108 lines; SD%Complete: 100; class celebras_the_cursed : public
-- CreatureScript { celebras_the_cursedAI : public ScriptedAI }; GetAI
-- via GetMaraudonAI<celebras_the_cursedAI> (MaraudonScriptName
-- "instance_maraudon" gate); AddSC_boss_celebras_the_cursed at end
-- registers the one script; kalimdor loader decl 60 / call 173 per
-- kalimdor_script_loader.cpp — the first "//Maraudon" loader group,
-- right after AddSC_instance_ragefire_chasm()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "celebras_the_cursed" (loader decl/call lines only
-- otherwise).
-- Entry: maraudon.h carries no NPC_ constants and the instance file
-- never names the boss — 12225 is classicdb/tauri-cited for
-- "Celebras the Cursed" (no NPC_ constant in the C++ tree; vishas /
-- gelihast precedent); the creature_template ScriptName binding stays
-- DB-side. JustDied summons 13716 (Celebras the Redeemed, the quest
-- NPC — wowhead-cited ID), TEMPSUMMON_TIMED_DESPAWN 10min at the
-- death position.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (C++ JustEngagedWith is empty — no Talk): arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm).
-- - Wrath 21807 on SelectTarget(Random, 0) with no range cap ->
--   random threat-listed unit (playerOnly=false default, UnitAI.h:170 —
--   C++ passes no playerOnly flag, so pets/totems qualify), plain
--   GetAITargets pick (the midnight precedent), non-triggered
--   (C++ DoCast), init 8s -> 8s.
-- - Entangling Roots 12747 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init 2s ->
--   20s.
-- - Corrupt Forces 21968 self-cast (C++ DoCast(me)),
--   non-triggered, init 30s -> 20s; the C++ pre-cast
--   me->InterruptNonMeleeSpells(false) arm is bridged (no-op twin)
--   and ported in onCorruptForces below.
-- Unmodeled (documented-only, no bridges):
-- - JustDied's SummonCreature(13716, death position, 0,
--   TEMPSUMMON_TIMED_DESPAWN Eluna type 2, 10min) — PORTED in onDied;
--   no Talk arm exists in C++ to wire.
-- - The C++ file declares zero Talk lines and no KilledUnit override
--   — no event-3 registration.

local ENTRY = 12225

local SPELL_WRATH            = 21807
local SPELL_ENTANGLING_ROOTS = 12747
local SPELL_CORRUPT_FORCES   = 21968

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

-- C++ SelectTarget(Random, 0) — playerOnly=false default
-- (UnitAI.h:170): the pool is the whole threat list (pets/totems
-- included) — plain pick over GetAITargets (the bridged threat list;
-- midnight precedent), no range cap, no Player filter.
local function randomThreatTarget(creature)
    local alive = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

-- C++ Wrath: DoCast(SelectTarget(Random, 0), 21807), non-triggered,
-- init 8s -> 8s; jeklik nil-target-keeps-schedule convention (C++
-- skips the cast when the pick is nil but the re-arm is outside).
local function onWrath(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_WRATH)
    end
    schedule(guid, "wrath", 8000, function()
        onWrath(creature, guid)
    end)
end

-- C++ EntanglingRoots: DoCastVictim(12747), non-triggered, init 2s
-- -> 20s; jeklik GetVictim + CastSpell convention.
local function onEntanglingRoots(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_ENTANGLING_ROOTS)
    end
    schedule(guid, "roots", 20000, function()
        onEntanglingRoots(creature, guid)
    end)
end

-- C++ CorruptForces: InterruptNonMeleeSpells(false) + DoCast(me, 21968), non-triggered, init 30s ->
-- 20s (pre-cast interrupt bridged no-op twin, ported below).
local function onCorruptForces(creature, guid)
    creature:InterruptNonMeleeSpells(false)
    creature:CastSpell(creature, SPELL_CORRUPT_FORCES)
    schedule(guid, "corruptforces", 20000, function()
        onCorruptForces(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "wrath", 8000, function()
        onWrath(creature, guid)
    end)
    schedule(guid, "roots", 2000, function()
        onEntanglingRoots(creature, guid)
    end)
    schedule(guid, "corruptforces", 30000, function()
        onCorruptForces(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (boss_celebras_the_cursed.cpp:71-74): the whole body is
-- SummonCreature(13716, death position, o=0, TEMPSUMMON_TIMED_DESPAWN
-- Eluna type 2, 10min) — the stale "no summon bridge" claim is
-- retracted; o=0 falls back to the summoner's orientation in both
-- C++ (Object.cpp:2061) and the Go bridge.
local function onDied(event, creature, killer)
    creature:SummonCreature(13716, creature:GetX(), creature:GetY(),
        creature:GetZ(), 0, 2, 600000)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
