-- Xevozz (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_xevozz.cpp
-- (291 lines incl. license; 3 scripts — boss_xevozz (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_XEVOZZ = 7); npc_ethereal_sphere
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), DoAction
-- ACTION_SUMMON Talk leg, unbridged); spell_xevozz_summon_players
-- (SpellScriptLoader SpellScript); all registered from inside
-- AddSC_boss_xevozz(); loader decl 150 / call 345 per
-- northrend_script_loader.cpp — the SIXTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_moragg() (call 344); the call after it is
-- AddSC_boss_zuramat() (call 346) — the checkpoint sequence
-- (boss_moragg -> boss_xevozz) is followed).
-- Entries: 29266 Xevozz (violet_hold.h NPC_XEVOZZ, line 96) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_XEVOZZ = 7
-- (violet_hold.h line 57).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_xevozz"
-- hits boss_xevozz.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (both timer delays here are ranges).
-- creature:CastSpell(creature, spell) = DoCastAOE / DoCast(me)
-- (1/2-arg = non-triggered); creature:CastSpell(target, spell) =
-- DoCast(target, spell) (2-arg = non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDeath, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent); seeds the two combat timers below; the
-- BossAI::JustEngagedWith base arms (:96) — SetBossState(DATA_XEVOZZ
-- 7, IN_PROGRESS) via creature:SetBossState
-- (lua_creature_events.go:5917), then pulse + me-zone — ported
-- base-first below.
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent); then _JustDied() (:121) — SetBossState
-- (DATA_XEVOZZ 7, DONE), ported in xevozzJustDied; the stale
-- "_JustDied() passthrough leg has no bridge" claim is retracted.
-- BossAI::Reset (:89-92, base only) — _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_XEVOZZ 7,
-- NOT_STARTED), != DONE-gated, ported in xevozzReset (new ev23,
-- split from ev2 — leave-combat does not call Reset in C++).
-- ScheduleTasks ARCANE_BARRAGE_VOLLEY — DoCastAOE(SPELL_ARCANE_
-- BARRAGE_VOLLEY 54202) = self-cast per the in-tree DoCastAOE
-- convention (boss_ai.go); init {8s,10s} / Repeat {8s,10s} —
-- unconditional re-arm.
-- ScheduleTasks ARCANE_BUFFET — SelectTarget(Random, 0, 45.0f, true)
-- -> DoCast(target, SPELL_ARCANE_BUFFET 54226), 2-arg =
-- non-triggered; init {10s,11s} / Repeat {15s,20s} — unconditional
-- re-arm. Random-player pool corrected 2026-10-25: threat-list
-- players within 45 yd (see randomThreatPlayerInRange); the old
-- instance-wide pool was the same bug class as moragg/erekem.
-- ScheduleTasks ethereal-sphere summon — Talk(SAY_REPEAT_SUMMON 5) +
-- random {54102, 54137, 54138} DoCast(me) + heroic 2.5s second pick;
-- 5s init / {45s,47s} repeat — PORTED (summonTick); the 33-35s
-- DoAction leg and the sphere AI stay out (see below).
-- DOCUMENTED DEVIATIONS: none on the ported arms — all ported timers
-- carry C++-exact timing and targeting. The UpdateAI
-- DoMeleeAttackIfReady() leg has no bridge — melee is unmodeled.
-- DOCUMENTED-ONLY (in this header; entry 29266 is the sole
-- registration):
-- SAY_SPAWN (3) and SAY_CHARGED (4) are never Talk()ed anywhere in the
-- file (dead enum text, the cyanigosa precedent).
-- Talk(SAY_SUMMON_ENERGY 6) rides the SpellHit SPELL_ARCANE_POWER
-- (54160) / H_SPELL_ARCANE_POWER (59474) leg — PORTED tick 486 via
-- ev14 (fires on each hit-target creature, boss_ai.go:4061); the
-- sphere proximity cast that feeds it is ported in
-- npc_ethereal_sphere.lua the same tick.
-- The ScheduleTasks ethereal-sphere summon leg (Talk(SAY_REPEAT_
-- SUMMON 5), 5s init / 45-47s Repeat, random summon-spell pick from
-- {54102, 54137, 54138} via DoCast(me, ...), heroic 2.5s delayed
-- second cast) is PORTED in summonTick below — the casts are ordinary
-- creature casts and the Go cast path implements SPELL_EFFECT_SUMMON
-- (boss_ai.go creatureEffectSummonType), so the old "whole event out"
-- (emalon EVENT_OVERCHARGE precedent) over-blocked. The 33-35s
-- delayed summons.DoAction(ACTION_SUMMON) leg is PORTED tick 486 as
-- a per-sphere self-timer seeded in npc_ethereal_sphere.lua Reset
-- (deviation: roll origin is sphere spawn vs the boss's cast-issue;
-- identical when the summon spell is instant — and the boss-side
-- shared-roll shape is unmodelable without a summon list). The
-- summoned sphere's own AI (npc_ethereal_sphere ARCANE_POWER
-- proximity machine / SAY_ETHEREAL_SPHERE_SUMMON DoAction Talk leg)
-- is PORTED tick 486 in npc_ethereal_sphere.lua. The JustSummoned
-- MoveFollow leg (boss_xevozz.cpp:105-109) is PORTED tick 486 via
-- ev19; the JustReachedHome leg (:98-102) is PORTED tick 486 via
-- ev24 + FindNearestGameObject(GO_XEVOZZ_DOOR 191556, 50) +
-- SetGoState(1) (HandleCells(7, false)).
-- spell_xevozz_summon_players (SpellScript: OnEffectHitTarget
-- SPELL_EFFECT_DUMMY -> SPELL_MAGIC_PULL 50770) joins the
-- no-SpellScript-bridge queue. The file carries a TODO ("Implement
-- Ethereal Summon Target").

local ENTRY_XEVOZZ = 29266
local DATA_XEVOZZ = 7
local GO_XEVOZZ_DOOR = 191556 -- violet_hold.h:127

local SAY_AGGRO = 0
local SAY_REPEAT_SUMMON = 5
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_SUMMON_ENERGY = 6

local SPELL_ARCANE_BARRAGE_VOLLEY = 54202
local SPELL_ARCANE_BUFFET = 54226

local SPHERE_SUMMON_SPELLS = { 54102, 54137, 54138 }

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

-- Cast-state gate (2026-11-04): BossAI ctor scheduler validator
-- (ScriptedCreature.cpp:493-496, !CASTING) — TaskScheduler::Dispatch
-- aborts while casting with tasks still queued, so each scheduler tick
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom); barrage/buffet/summon scheduler ticks carry the gate (defer precedes the summon Talk+cast); the heroic 2.5s sub-cast is a scheduler sub-task, gated the same way.

-- C++ SelectTarget(Random, 0, 45.0f, true) draws the threat list
-- (UnitAI.h:162-184, playerOnly=true), not instance players: random
-- alive threat-list player within 45y (nefarian/vaelastrasz
-- precedent; the old instance-wide pool could buffet a player with
-- zero threat). Alive filter: dead targets are offline
-- (UNIT_STATE_DIED fails _IsTargetAcceptable, Creature.cpp:2508)
-- and skipped by SelectTargetList, while Go SortedEntries returns
-- dead entries too. C++ IsWithinCombatRange adds both combat
-- reaches (Unit.cpp:583); player reach 1.5 (ObjectDefines.h:40),
-- reach-exact.
local function randomThreatPlayerInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks ARCANE_BARRAGE_VOLLEY: DoCastAOE(54202) =
-- self-cast per the in-tree DoCastAOE convention; init {8s,10s} /
-- Repeat {8s,10s} — re-armed unconditionally.
local function barrageTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "barrage", 500, function()
            barrageTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ARCANE_BARRAGE_VOLLEY)
    schedule(guid, "barrage", {8000, 10000}, function()
        barrageTick(creature, guid)
    end)
end

-- C++ ScheduleTasks ARCANE_BUFFET: SelectTarget(Random, 0, 45.0f,
-- true) -> DoCast(target, 54226), 2-arg = non-triggered; init
-- {10s,11s} / Repeat {15s,20s} — re-armed unconditionally.
local function buffetTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "buffet", 500, function()
            buffetTick(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_BUFFET)
    end
    schedule(guid, "buffet", {15000, 20000}, function()
        buffetTick(creature, guid)
    end)
end

-- C++ ScheduleTasks ethereal-sphere summon: Talk(SAY_REPEAT_SUMMON 5) +
-- random pick from {54102, 54137, 54138} -> DoCast(me, spell)
-- (non-triggered); heroic: second pick from the remaining two after
-- 2500ms. 5s init / Repeat {45s,47s}. The 33-35s delayed
-- summons.DoAction(ACTION_SUMMON) leg stays out (dispatch IS bridged
-- — ev38 — but the summoned spheres carry no registered AI),
-- as does the summoned sphere's own AI (npc_ethereal_sphere
-- ARCANE_POWER proximity machine, unregistered).
local function summonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon", 500, function()
            summonTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_REPEAT_SUMMON)
    local pool = { SPHERE_SUMMON_SPELLS[1], SPHERE_SUMMON_SPELLS[2], SPHERE_SUMMON_SPELLS[3] }
    local i = math.random(#pool)
    creature:CastSpell(creature, pool[i])
    table.remove(pool, i)
    if creature:IsHeroic() then
        local second = pool[math.random(#pool)]
        local function summonHeroicTick()
            if creature:IsCasting() then
                schedule(guid, "summonHeroic", 500, function()
                    summonHeroicTick()
                end)
                return
            end
            creature:CastSpell(creature, second)
        end
        schedule(guid, "summonHeroic", 2500, summonHeroicTick)
    end
    schedule(guid, "summon", {45000, 47000}, function()
        summonTick(creature, guid)
    end)
end

-- C++ boss_xevozzAI::JustEngagedWith: Talk(SAY_AGGRO) — unconditional
-- (the auriaya engage-port precedent); the ScheduleTasks timers
-- seed their combat timers below; the BossAI::JustEngagedWith
-- base arms (:96) ported base-first below.
local function xevozzEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- BossAI::JustEngagedWith (:96, base-first) — SetBossState
    -- (DATA_XEVOZZ 7, IN_PROGRESS) first (ScriptedCreature.cpp:527-542),
    -- then SetCombatPulseDelay(5) + me-form DoZoneInCombat.
    creature:SetBossState(DATA_XEVOZZ, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "barrage", {8000, 10000}, function()
        barrageTick(creature, guid)
    end)
    schedule(guid, "buffet", {10000, 11000}, function()
        buffetTick(creature, guid)
    end)
    schedule(guid, "summon", 5000, function()
        summonTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function xevozzCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function xevozzKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; then _JustDied()
-- (:121) — SetBossState(DATA_XEVOZZ 7, DONE)
-- (ScriptedCreature.cpp:513-520), after the death Talk per C++ order.
local function xevozzJustDied(event, creature)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_XEVOZZ, 3)
end

-- C++ BossAI::Reset (:89-92, base only): _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_XEVOZZ 7,
-- NOT_STARTED) only when the stored state != DONE. Split from ev2
-- (the keleseth/svala precedent): ev2 leave-combat does not call
-- Reset in C++, so it stays wipe-only.
local function xevozzReset(event, creature)
    -- Latch Xevozz's GUID under DATA_XEVOZZ for the sphere proximity
    -- poll (boss_xevozz.cpp:229 reads the instance DATA_XEVOZZ
    -- object GUID). ev23 fires on spawn as well as evade
    -- (creaturemotion.go:1262 + lua_creature_events.go:7264), so the latch lands at spawn;
    -- re-latching the same GUID on evade is a no-op (ichoron idiom).
    creature:SetGuidData(DATA_XEVOZZ)
    if creature:GetBossState(DATA_XEVOZZ) ~= 3 then
        creature:SetBossState(DATA_XEVOZZ, 0)
    end
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_xevozzAI::JustSummoned (:105-109): BossAI base, then
-- summon->GetMotionMaster()->MoveFollow(me, 0, 0) — unconditional;
-- the spheres are the only summons. ev19 fires for spell summons
-- (lua_creature_events.go:190).
local function xevozzJustSummoned(event, creature, summon)
    summon:MoveFollow(creature, 0, 0)
end

-- C++ boss_xevozzAI::SpellHit (:123-127): SPELL_ARCANE_POWER
-- (54160) / H_SPELL_ARCANE_POWER (59474) -> Talk(SAY_SUMMON_ENERGY
-- 6). ev14 fires on each hit-target creature (boss_ai.go:4061).
local function xevozzSpellHit(event, creature, caster, spellId)
    if spellId == 54160 or spellId == 59474 then
        creature:Talk(SAY_SUMMON_ENERGY)
    end
end

-- C++ boss_xevozzAI::JustReachedHome (:98-102): BossAI base +
-- instance->SetData(DATA_HANDLE_CELLS, DATA_XEVOZZ) ->
-- HandleCells(7, false) -> HandleGameObject(GO_XEVOZZ_DOOR, false)
-- -> GO_STATE_READY (1). Boss-side mirror via ev24.
local function xevozzReachedHome(event, creature)
    local door = creature:FindNearestGameObject(GO_XEVOZZ_DOOR, 50)
    if door then
        door:SetGoState(1)
    end
end

RegisterCreatureEvent(ENTRY_XEVOZZ, 1, xevozzEnterCombat)
RegisterCreatureEvent(ENTRY_XEVOZZ, 2, xevozzCancelCombatTimers)
RegisterCreatureEvent(ENTRY_XEVOZZ, 3, xevozzKilledUnit)
RegisterCreatureEvent(ENTRY_XEVOZZ, 4, xevozzJustDied)
RegisterCreatureEvent(ENTRY_XEVOZZ, 4, xevozzCancelCombatTimers)
RegisterCreatureEvent(ENTRY_XEVOZZ, 23, xevozzReset)
RegisterCreatureEvent(ENTRY_XEVOZZ, 19, xevozzJustSummoned)
RegisterCreatureEvent(ENTRY_XEVOZZ, 14, xevozzSpellHit)
RegisterCreatureEvent(ENTRY_XEVOZZ, 24, xevozzReachedHome)
