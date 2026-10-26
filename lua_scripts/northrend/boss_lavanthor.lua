-- Lavanthor (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_lavanthor.cpp
-- (94 lines; 1 script — boss_lavanthor (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_LAVANTHOR = 6); registered from
-- inside AddSC_boss_lavanthor() — loader decl 148 / call 343, the
-- "// Violet Hold" block in AddNorthrendScripts(),
-- northrend_script_loader.cpp, after AddSC_boss_ichoron() (342);
-- AddSC_boss_moragg() (344) follows).
-- Entry: 29312 Lavanthor (violet_hold.h NPC_LAVANTHOR, line 97) —
-- entry-verifiable, registration proceeds; the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_LAVANTHOR = 6
-- (violet_hold.h line 56).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). C++ Schedule(init) literals become plain ms delays;
-- Repeat(min, max) ranges become {minMs, maxMs} table delays (the
-- toravon randtime convention).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (target, spell) = DoCast(target, spell) (2-arg = non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Zero Talk lines in the C++ — none ported.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_lavanthor.cpp):
-- boss_lavanthorAI::JustEngagedWith (:61-64) — seeds the three combat
-- timers below; the BossAI::JustEngagedWith passthrough leg has no
-- bridge.
-- ScheduleTasks firebolt leg (:67-72) — SelectTarget(Random, 0,
-- 40.0f, true) (player-only) -> DoCast(target, SPELL_FIREBOLT 54235)
-- (:70, 2-arg = non-triggered) -> CastSpell(target, 54235); init 1s,
-- re-arm 5-13s.
-- ScheduleTasks flame-breath leg (:74-78) — DoCastVictim
-- (SPELL_FLAME_BREATH 54282) (:76, 1-arg = non-triggered) ->
-- CastSpell(nil, 54282); init 5s, re-arm 10-15s.
-- ScheduleTasks lava-burn leg (:80-85) — SelectTarget(Random, 0,
-- 50.0f) -> DoCast(target, SPELL_LAVA_BURN 54249) (:83, 2-arg =
-- non-triggered) -> CastSpell(target, 54249); init 10s, re-arm
-- 15-23s.
-- ScheduleTasks cauterizing-flames leg (:87-93, heroic-only) —
-- DoCastAOE(SPELL_CAUTERIZING_FLAMES 59466) -> CastSpell(creature,
-- 59466) self-cast (the boss_ai.go DoCastAOE lowering), 1-arg =
-- non-triggered; init 3s, re-arm 10-16s; seeded at engage only when
-- creature:IsHeroic() (the earlier "no IsHeroic bridge" claim was
-- stale — IsHeroic is bridged, engine/world/lua_creature_events.go:260;
-- the cyanigosa mana-destruction port).
-- DOCUMENTED DEVIATIONS:
-- The SelectTarget pools are threat-list picks (firebolt 40.0f :68
-- player-only, lava-burn 50.0f :81 any-unit) via GetAITargets
-- (corrected 2026-10-25; the old "no Lua unit-enumeration bridge"
-- claim was stale — GetAITargets IS the threat-list bridge, and
-- the old player-only lava-burn approximation is closed).
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) pre-check has no
-- bridge — casts fire unconditionally on their timers (the hodir
-- precedent).
-- DOCUMENTED-ONLY (in this header; entry 29312):
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_LAVANTHOR) leg — no instance-script bridge.

local ENTRY_LAVANTHOR = 29312

local SPELL_FIREBOLT = 54235
local SPELL_FLAME_BREATH = 54282
local SPELL_LAVA_BURN = 54249
local SPELL_CAUTERIZING_FLAMES = 59466 -- :25 (heroic-only)

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

-- C++ SelectTarget draws the threat list (UnitAI.h:162-184), not
-- instance players (corrected 2026-10-25 — the old instance-wide
-- pool was the same bug class as moragg/erekem/xevozz/zuramat).
-- Firebolt is playerOnly=true; lava-burn omits the flag
-- (playerOnly=false default), so any threat-listed unit within
-- range qualifies. Alive filter: dead targets are offline
-- (UNIT_STATE_DIED fails _IsTargetAcceptable, Creature.cpp:2508)
-- and skipped by SelectTargetList, while Go SortedEntries returns
-- dead entries too. C++ IsWithinCombatRange adds both combat
-- reaches (Unit.cpp:583); Lua subtracts own reach + target reach
-- (1.5 if Player else target:GetCombatReach(), hadronox idiom).
local function randomThreatTargetInRange(creature, maxDist, playerOnly)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if t:IsAlive() and creature:GetDistance(t) - creature:GetCombatReach() - targetReach < maxDist
                and (not playerOnly or t:GetObjectType() == "Player") then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks firebolt leg (:67-72): DoCast(target,
-- SPELL_FIREBOLT), 2-arg = non-triggered; init 1s, re-arm 5-13s.
local function fireboltTick(creature, guid)
    local target = randomThreatTargetInRange(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_FIREBOLT)
    end
    schedule(guid, "firebolt", {5000, 13000}, function()
        fireboltTick(creature, guid)
    end)
end

-- C++ ScheduleTasks flame-breath leg (:74-78): DoCastVictim
-- (SPELL_FLAME_BREATH), 1-arg = non-triggered; init 5s, re-arm
-- 10-15s.
local function flameBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_FLAME_BREATH)
    schedule(guid, "flamebreath", {10000, 15000}, function()
        flameBreathTick(creature, guid)
    end)
end

-- C++ ScheduleTasks lava-burn leg (:80-85): DoCast(target,
-- SPELL_LAVA_BURN), 2-arg = non-triggered; init 10s, re-arm 15-23s.
local function lavaBurnTick(creature, guid)
    local target = randomThreatTargetInRange(creature, 50, false)
    if target then
        creature:CastSpell(target, SPELL_LAVA_BURN)
    end
    schedule(guid, "lavaburn", {15000, 23000}, function()
        lavaBurnTick(creature, guid)
    end)
end

-- C++ ScheduleTasks cauterizing-flames leg (:87-93, heroic-only):
-- DoCastAOE(SPELL_CAUTERIZING_FLAMES) (self-cast per the boss_ai.go
-- DoCastAOE lowering), 1-arg = non-triggered; init 3s, re-arm 10-16s.
local function cauterizingFlamesTick(creature, guid)
    creature:CastSpell(creature, SPELL_CAUTERIZING_FLAMES)
    schedule(guid, "cauterizing", {10000, 16000}, function()
        cauterizingFlamesTick(creature, guid)
    end)
end

-- C++ boss_lavanthorAI::JustEngagedWith (:61-64): seeds the three
-- combat timers; the BossAI::JustEngagedWith passthrough leg has no
-- bridge.
local function lavanthorEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "firebolt", 1000, function()
        fireboltTick(creature, guid)
    end)
    schedule(guid, "flamebreath", 5000, function()
        flameBreathTick(creature, guid)
    end)
    schedule(guid, "lavaburn", 10000, function()
        lavaBurnTick(creature, guid)
    end)
    -- C++ :87: the cauterizing-flames scheduler seeds only when
    -- IsHeroic() (the earlier "no IsHeroic bridge" header claim was
    -- stale — creature:IsHeroic() is bridged,
    -- engine/world/lua_creature_events.go:260).
    if creature:IsHeroic() then
        schedule(guid, "cauterizing", 3000, function()
            cauterizingFlamesTick(creature, guid)
        end)
    end
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function lavanthorCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LAVANTHOR, 1, lavanthorEnterCombat)
RegisterCreatureEvent(ENTRY_LAVANTHOR, 2, lavanthorCancelCombatTimers)
RegisterCreatureEvent(ENTRY_LAVANTHOR, 4, lavanthorCancelCombatTimers)
RegisterCreatureEvent(ENTRY_LAVANTHOR, 23, lavanthorCancelCombatTimers)
