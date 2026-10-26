-- Erekem (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_erekem.cpp
-- (323 lines; 2 scripts — boss_erekem (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_EREKEM = 4); npc_erekem_guard
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), entry 29395 —
-- melee-only, the spell machine is dead code, see below); both
-- registered from inside AddSC_boss_erekem(); loader decl 146 /
-- call 341 per northrend_script_loader.cpp — the SECOND group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_cyanigosa() (call 340); the call after it is
-- AddSC_boss_ichoron() (call 342).
-- Entries: 29315 Erekem (violet_hold.h NPC_EREKEM, line 104);
-- 29395 Erekem Guard (violet_hold.h NPC_EREKEM_GUARD, line 105) —
-- entry-verifiable (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName bindings are
-- DB-side as usual. DATA_EREKEM = 4 (violet_hold.h line 54).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). BossAI::_JustEngagedWith calls ScheduleTasks()
-- (ScriptedCreature.cpp:529-544), so the timers below are seeded at
-- event 1. A {minMs, maxMs} table delay is a randtime range, matching
-- C++ ScheduleEvent(min, max) / task.Repeat(min, max) (the freya
-- precedent).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCastAOE (2-arg = non-triggered);
-- creature:CastSpell(target, spell) = DoCast(target, spell) (2-arg =
-- non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_erekem.cpp):
-- boss_erekemAI::JustEngagedWith (:71-76) — Talk(SAY_AGGRO 0) (:74)
-- + DoCast(me, SPELL_EARTH_SHIELD 54479) (:75, 2-arg = non-triggered
-- self-cast) -> CastSpell(creature, 54479) once at engage, cast order
-- kept (the toravon FROZEN_MALLET engage self-cast precedent); the
-- BossAI::JustEngagedWith(who) passthrough is unmodeled; the
-- ScheduleTasks seeding is carried by the event-1 scheduling below
-- (the EARTH_SHIELD 20s re-arm is NOT ported — documented below).
-- BLOODLUST (:195-198) — DoCast(SPELL_BLOODLUST 54516) (:197, 1-arg
-- = non-triggered self-cast) -> CastSpell(creature, 54516); 2s init,
-- task.Repeat(35s, 45s) -> {35000, 45000} randtime re-arm.
-- LIGHTNING_BOLT (:201-206) — SelectTarget(Random, 0, 40.0f) (:204,
-- random threat-listed unit within 40y; playerOnly=false default,
-- UnitAI.h:170 — pets/totems qualify) -> DoCast(target,
-- SPELL_LIGHTNING_BOLT 53044) (:205, 2-arg = non-triggered) ->
-- CastSpell(target, 53044); 2s init, task.Repeat(2500ms) -> 2500ms
-- re-arm. Random-threat pool: GetAITargets within 40y (the
-- murmur randomThreatTarget precedent).
-- EARTH_SHOCK (:220-224) — DoCastVictim(SPELL_EARTH_SHOCK 54511)
-- (:222, 1-arg = non-triggered) -> CastSpell(nil, 54511); init
-- Schedule(2s, 8s) -> {2000, 8000} randtime, task.Repeat(8s, 13s) ->
-- {8000, 13000} randtime re-arm.
-- KilledUnit (:90-94) — player-gated Talk(SAY_SLAY 1) (event 3 — the
-- razuvious precedent).
-- JustDied (:96-100) — Talk(SAY_DEATH 2) (:98); the _JustDied()
-- passthrough is unmodeled (the sjonnir precedent).
-- DOCUMENTED DEVIATIONS:
-- LIGHTNING_BOLT's SelectTarget (:204) passes no playerOnly flag
-- (playerOnly=false default, UnitAI.h:170) — C++ would also consider
-- non-player hostile units within 40y; the port now picks from the
-- threat list via GetAITargets (the midnight precedent), which is the
-- same pool C++ draws from.
-- DOCUMENTED-ONLY (in this header; no guard registration):
-- npc_erekem_guardAI::ScheduledTasks (:288-309) — DEAD CODE: the
-- method name is typo'd (ScheduledTasks, not the ScheduleTasks
-- override called by BossAI::_JustEngagedWith), ScriptedAI has no
-- such hook, and the guard's JustEngagedWith (:274-277) calls only
-- DoZoneInCombat() — so the STRIKE 14516 (init/repeat {4s,8s},
-- DoCastVictim), HOWLING_SCREECH 54462 (init/repeat {8s,13s},
-- DoCastAOE), and GUSHING_WOUND 39215 (init {1s,3s}, repeat {7s,12s},
-- DoCastVictim) legs NEVER FIRE in C++; the guard is melee-only.
-- Porting them would fabricate casts that never occur. DoZoneInCombat
-- (:276) has no bridge; DoMeleeAttackIfReady (:284-285) is
-- engine-driven in Go.
-- EARTH_SHIELD re-cast (:187-192) — if (Unit* ally =
-- DoSelectLowestHpFriendly(30.0f)) DoCast(ally, SPELL_EARTH_SHIELD):
-- lowest-HP-friendly ally selection (includes the guard creatures,
-- 30y) has no bridge; 20s init / task.Repeat(20s).
-- CHAIN_HEAL (:209-217) — if (Unit* ally =
-- DoSelectLowestHpFriendly(40.0f)) DoCast(ally, SPELL_CHAIN_HEAL
-- 54481): same unbridged friendly-selection leg; the re-arm is
-- additionally gated on the unbridged guard leg (task.Repeat(3s) if
-- !CheckGuardAlive() else (8s, 11s)); 10s init.
-- BREAK_BONDS scan (:226-241) — instance->GetGuidData
-- (DATA_EREKEM_GUARD_1/2) + CheckGuardAuras (:102-135) ->
-- DoCastAOE(SPELL_BREAK_BONDS 59463), 10s re-arm on a hit / 500ms
-- poll: instance-GUID / cross-creature machine unbridged.
-- Reset() SetCanDualWield(false) (:68) — phase-machine arm (see
-- below), unmodeled.
-- Phase machine (:169-181) — _phase 0->1 on !CheckGuardAlive()
-- (:137-150, instance GetGuidData legs): SetCanDualWield(true),
-- DoCast(me, SPELL_WINDFURY 54493, true) (triggered — phase-gated,
-- stays out with the machine), DoSpellAttackIfReady
-- (SPELL_STORMSTRIKE 51876) vs DoMeleeAttackIfReady: no bridges.
-- GetChainHealTarget (:149-166) — dead helper: zero call sites in the
-- file (the chain-heal arm uses DoSelectLowestHpFriendly).
-- MovementInform (:78-82) — EFFECT_MOTION_TYPE/POINT_INTRO ->
-- me->SetFacingTo(4.921828f): no MovementInform bridge.
-- JustReachedHome (:84-88) — instance->SetData(DATA_HANDLE_CELLS,
-- DATA_EREKEM): no instance SetData bridge.
-- SAY_SPAWN (3) / SAY_ADD_KILLED (4) / SAY_BOTH_ADDS_KILLED (5) —
-- never Talk()ed anywhere in the file (grep -c "Talk(" = 3, all
-- ported above): dead enum text.

local ENTRY_EREKEM = 29315

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_EARTH_SHIELD = 54479
local SPELL_BLOODLUST = 54516
local SPELL_LIGHTNING_BOLT = 53044
local SPELL_EARTH_SHOCK = 54511

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 40.0f) — playerOnly=false
-- default (UnitAI.h:170): any threat-listed unit within 40y
-- (IsWithinCombatRange 3D check) — plain pick over GetAITargets (the
-- bridged threat list; midnight precedent), no Player filter.
-- Alive filter: C++ SelectTargetList skips offline refs
-- (UnitAI.h:162-184) and dead targets are offline (UNIT_STATE_DIED
-- fails _IsTargetAcceptable, Creature.cpp:2508-2524); Go
-- SortedEntries returns dead entries too, so the pool is
-- alive-filtered (pandemonius/maladaar precedent).
local function randomThreatTargetInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() and creature:GetDistance(t) - creature:GetCombatReach() < maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ BLOODLUST (:195-198): DoCast 1-arg = non-triggered self-cast;
-- 2s init, task.Repeat(35s, 45s).
local function bloodlustTick(creature, guid)
    creature:CastSpell(creature, SPELL_BLOODLUST)
    schedule(guid, "bloodlust", {35000, 45000}, function()
        bloodlustTick(creature, guid)
    end)
end

-- C++ LIGHTNING_BOLT (:201-206): random threat-listed unit within 40y
-- -> DoCast(target), non-triggered; 2s init, task.Repeat(2500ms).
local function lightningBoltTick(creature, guid)
    local target = randomThreatTargetInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_LIGHTNING_BOLT)
    end
    schedule(guid, "lightningbolt", 2500, function()
        lightningBoltTick(creature, guid)
    end)
end

-- C++ EARTH_SHOCK (:220-224): DoCastVictim, non-triggered; init
-- Schedule(2s, 8s), task.Repeat(8s, 13s).
local function earthShockTick(creature, guid)
    creature:CastSpell(nil, SPELL_EARTH_SHOCK)
    schedule(guid, "earthshock", {8000, 13000}, function()
        earthShockTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:71-76): Talk(SAY_AGGRO) + DoCast(me,
-- SPELL_EARTH_SHIELD) non-triggered self-cast at engage (the toravon
-- engage self-cast precedent); the ScheduleTasks seeding below.
local function erekemEnterCombat(event, creature)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_EARTH_SHIELD)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bloodlust", 2000, function()
        bloodlustTick(creature, guid)
    end)
    schedule(guid, "lightningbolt", 2000, function()
        lightningBoltTick(creature, guid)
    end)
    schedule(guid, "earthshock", {2000, 8000}, function()
        earthShockTick(creature, guid)
    end)
end

-- C++ KilledUnit (:90-94): player-gated Talk(SAY_SLAY) (the
-- razuvious precedent).
local function erekemKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:96-100): Talk(SAY_DEATH) (the sjonnir precedent);
-- the _JustDied() passthrough is unmodeled.
local function erekemJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function erekemCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EREKEM, 1, erekemEnterCombat)
RegisterCreatureEvent(ENTRY_EREKEM, 2, erekemCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EREKEM, 3, erekemKilledUnit)
RegisterCreatureEvent(ENTRY_EREKEM, 4, erekemCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EREKEM, 4, erekemJustDied)
RegisterCreatureEvent(ENTRY_EREKEM, 23, erekemCancelCombatTimers)
