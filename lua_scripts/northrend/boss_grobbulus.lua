-- Grobbulus (Naxxramas, Construct Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_grobbulus.cpp
-- (boss_grobbulus (BossAI) + npc_grobbulus_poison_cloud
-- (ScriptedAI); AddSC_boss_grobbulus also registers
-- spell_grobbulus_mutating_injection and
-- spell_grobbulus_poison_cloud, both unmodeled below).
-- Entry: 15931 Grobbulus (creature_template table, cross-checked:
-- Anub'Rekhan 15956 / Faerlina 15953 / Thaddius 15928 / Maexxna 15952 /
-- Gluth 15932 / Heigan 15936 / Sapphiron 15989 / Kel'Thuzad 15990 all
-- match naxxramas.h NAXCreaturesIds; 25-man difficulty entry 29373 is
-- NOT registered, base-entry-only convention like anubrekhan/maexxna).
-- Fallout slime entry 16290 is NPC_FALLOUT_SLIME in the C++ file.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 15 OnSpellHitTarget, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- C++ event arms use `return` after each Repeat (one event per
-- UpdateAI tick); Lua pumps are independent timers, so each arm
-- re-arms unconditionally like C++ Repeat does.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith (event 1; BossAI::JustEngagedWith
-- instance-bookkeeping leg has no bridge — tharon_ja precedent) +
-- EVENT_CLOUD 15s + EVENT_INJECT 20s + EVENT_SPRAY randtime
-- (15s, 30s) + EVENT_BERSERK 12min; EVENT_CLOUD DoCastAOE
-- (POISON_CLOUD 28240, non-triggered) lowered to self-cast
-- (DoCastAOE self-cast convention, selin/mr_smite precedent),
-- 15s repeat; EVENT_BERSERK DoCastAOE (BERSERK 26662, triggered)
-- self-cast, one-shot; EVENT_SPRAY DoCastAOE (SLIME_SPRAY 28157,
-- non-triggered) self-cast, randtime (15s, 30s) repeat via the
-- {min, max} CreateLuaEvent delay (maiden/moroes precedent);
-- EVENT_INJECT SelectTarget(Random, 1, 0.0f, playerOnly, withTank,
-- -MUTATING_INJECTION) rendered as: threat-ordered GetAITargets
-- (curator precedent) filtered to players without MUTATING_
-- INJECTION 28169 (player HasAura bridge, lua_player.go:283),
-- skipping the first entry (threat head — gurtogg offset-1
-- precedent), alive-filtered (C++ SelectTargetList skips offline
-- refs at build, UnitAI.h; dead = offline via ShouldBeOffline,
-- ThreatManager.cpp:101), random pick among the rest; no target means no
-- cast but the arm still re-arms (C++ Repeat sits outside the
-- target-null check); DoCast(target, MUTATING_INJECTION 28169,
-- non-triggered); re-arm 8s + round(120 * healthPct) ms, health
-- re-read each repeat (C++ Repeat(8s + round(120 * GetHealthPct())
-- ms)). SpellHitTarget (event 15, arugal/lethon precedent:
-- handler args are event, creature, target, spellId): SLIME_SPRAY
-- 28157 hits summon FALLOUT_SLIME 16290 at the target's position
-- (target:GetX/Y/Z/O) with summon type 3 =
-- TIMED_DESPAWN_OUT_OF_COMBAT (lethon precedent: Go's
-- TempSummonType mapping at lua_creature_events.go:1163) and
-- despawn timer 0 (C++ passes no time for this summon type).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired): Reset/
-- JustEngagedWith/JustDied instance bookkeeping (_Reset/_JustDied
-- via BossAI); npc_grobbulus_poison_cloud AI (IsSummonedBy
-- self-cast POISON_CLOUD_PASSIVE 28158 + SetReactState PASSIVE +
-- SetCombatMovement(false)) — its creature entry is NOT in the
-- C++ file (the cloud arrives via the POISON_CLOUD spell effect,
-- DB-side), so no faithful registration exists (vengeful-shade
-- precedent: registering without a verified entry would be
-- fabrication); spell_grobbulus_mutating_injection AuraScript
-- (MUTATING_EXPLOSION 28206 + POISON_CLOUD on remove) and
-- spell_grobbulus_poison_cloud AuraScript (periodic trigger
-- with radius mod) — no SpellScript bridge; SPELL_BOMBARD_SLIME
-- 28280 is enum-only in this file (no arm uses it); melee
-- engine-driven.

local ENTRY_GROBBULUS = 15931
local NPC_FALLOUT_SLIME = 16290

local SPELL_SLIME_SPRAY = 28157
local SPELL_MUTATING_INJECTION = 28169
local SPELL_POISON_CLOUD = 28240
local SPELL_BERSERK = 26662

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

local function cloudTick(creature, guid)
    creature:CastSpell(creature, SPELL_POISON_CLOUD)
    schedule(guid, "cloud", 15000, function()
        cloudTick(creature, guid)
    end)
end

local function injectTick(creature, guid)
    local candidates = {}
    local targets = creature:GetAITargets()
    for i = 2, #targets do
        local unit = targets[i]
        if unit:GetObjectType() == "Player"
                and not unit:IsDead()
                and not unit:HasAura(SPELL_MUTATING_INJECTION) then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        creature:CastSpell(target, SPELL_MUTATING_INJECTION)
    end
    local delay = 8000 + math.floor(120 * creature:GetHealthPct() + 0.5)
    schedule(guid, "inject", delay, function()
        injectTick(creature, guid)
    end)
end

local function sprayTick(creature, guid)
    creature:CastSpell(creature, SPELL_SLIME_SPRAY)
    schedule(guid, "spray", {15000, 30000}, function()
        sprayTick(creature, guid)
    end)
end

local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

local function grobbulusEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "cloud", 15000, function()
        cloudTick(creature, guid)
    end)
    schedule(guid, "inject", 20000, function()
        injectTick(creature, guid)
    end)
    schedule(guid, "spray", {15000, 30000}, function()
        sprayTick(creature, guid)
    end)
    schedule(guid, "berserk", 720000, function()
        berserkTick(creature, guid)
    end)
end

local function grobbulusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function grobbulusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function grobbulusSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_SLIME_SPRAY and target ~= nil then
        creature:SummonCreature(NPC_FALLOUT_SLIME, target:GetX(),
            target:GetY(), target:GetZ(), target:GetO(), 3, 0)
    end
end

local function grobbulusReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GROBBULUS, 1, grobbulusEnterCombat)
RegisterCreatureEvent(ENTRY_GROBBULUS, 2, grobbulusLeaveCombat)
RegisterCreatureEvent(ENTRY_GROBBULUS, 4, grobbulusDied)
RegisterCreatureEvent(ENTRY_GROBBULUS, 15, grobbulusSpellHitTarget)
RegisterCreatureEvent(ENTRY_GROBBULUS, 23, grobbulusReset)
