-- Maiden of Grief (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_maiden_of_grief.cpp
-- (boss_maiden_of_grief (CreatureScript) via
-- GetHallsOfStoneAI<boss_maiden_of_griefAI> (BossAI), DATA_MAIDEN_OF_GRIEF
-- = 1 — registered from inside AddSC_boss_maiden_of_grief(); loader
-- decl 104 / call 299 per northrend_script_loader.cpp — the FIRST
-- group of the "// Halls of Stone" block in AddNorthrendScripts(),
-- under the "// Ulduar: Halls of Stone" decl marker at loader line
-- 103 and the "// Halls of Stone" call marker at loader line 298,
-- immediately after AddSC_instance_halls_of_lightning() (decl 102 /
-- call 297), which closed the Halls of Lightning block; the call
-- after it is AddSC_boss_krystallus() (decl 105 / call 300) —
-- loader order confirmed this run; the checkpoint sequence
-- (instance_halls_of_lightning -> boss_maiden_of_grief) is followed).
-- Entry: 27975 Maiden of Grief (halls_of_stone.h NPC_MAIDEN, line 49;
-- DATA_MAIDEN_OF_GRIEF = 1, line 32; GO_MAIDEN_DOOR = 191292, line
-- 63) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_maiden_of_grief" hits boss_maiden_of_grief.cpp + the loader
-- decl/call lines for AddSC_boss_maiden_of_grief() only; zero sql/
-- hits. No maiden_of_grief lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer arms ride CreateLuaEvent
-- (the maiden CreateLuaEvent convention); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough and the
-- DoStartTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT 20383) leg
-- have no bridges — the boss_bookkeeping precedent for the former,
-- no timed-achievement bridge for the latter).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — nineteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- UpdateAI EVENT_STORM_OF_GRIEF — PORTED (retraction re-audit
-- 2026-10-14: the Oct-2 "no-timer-bridge" claim is stale):
-- DoCastVictim(SPELL_STORM_OF_GRIEF 50752, triggered), 10s init
-- (:67) / {15s,20s} re-arm (:113-115) — zero gating legs —
-- creature:CastSpell(nil, 50752, true).
-- UpdateAI EVENT_SHOCK_OF_SORROW — PORTED (same retraction):
-- ResetThreatList (no threat bridge — documented-only) +
-- Talk(SAY_STUN 3) + DoCastAOE(SPELL_SHOCK_OF_SORROW 50760)
-- non-triggered, {20s,25s} init (:68) / {20s,30s} re-arm (:117-121)
-- — creature:Talk(3) + creature:CastSpell(creature, 50760) (the
-- DoCastAOE self-cast convention — selin_fireheart / mr_smite /
-- razuvious precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27975):
-- Reset ScheduleEvent EVENT_PARTING_SORROW (25s/30s, :66 —
-- heroic-only: no heroic bridge) + DoStopTimedAchievement(
-- ACHIEV_GOOD_GRIEF_START_EVENT) — no achievement bridge.
-- UpdateAI EVENT_PARTING_SORROW (random-target
-- DoCast(SPELL_PARTING_SORROW 59723), 30s/40s re-arm, :108-111) —
-- no random-target bridge; EVENT_PILLAR_OF_WOE (random-target
-- DoCast(SPELL_PILLAR_OF_WOE 50761) else DoCastVictim, 5s/25s
-- re-arm, :123-128) — no random-target bridge; the
-- UNIT_STATE_CASTING gate and the DoMeleeAttackIfReady tail have no
-- bridges.

local ENTRY_MAIDEN_OF_GRIEF = 27975

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_STUN = 3

local SPELL_STORM_OF_GRIEF = 50752
local SPELL_SHOCK_OF_SORROW = 50760

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the maiden / bjarngrim-lieutenant convention.
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

-- C++ UpdateAI EVENT_STORM_OF_GRIEF
-- (boss_maiden_of_grief.cpp:113-115): DoCastVictim(
-- SPELL_STORM_OF_GRIEF 50752, triggered), 10s init (:67) /
-- {15s,20s} re-arm — zero gating legs.
local function stormTick(creature, guid)
    creature:CastSpell(nil, SPELL_STORM_OF_GRIEF, true)
    schedule(guid, "storm", {15000, 20000}, function()
        stormTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_SHOCK_OF_SORROW (:117-121): ResetThreatList
-- (no threat bridge — documented-only) + Talk(SAY_STUN 3) +
-- DoCastAOE(SPELL_SHOCK_OF_SORROW 50760) non-triggered,
-- {20s,25s} init (:68) / {20s,30s} re-arm.
local function shockTick(creature, guid)
    creature:Talk(SAY_STUN)
    creature:CastSpell(creature, SPELL_SHOCK_OF_SORROW)
    schedule(guid, "shock", {20000, 30000}, function()
        shockTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI / achievement legs —
-- only the Talk arm is bridgeable. The Reset() ScheduleEvent legs
-- seed the timer machine here: the C++ EventMap only ticks once
-- UpdateAI runs with a victim, so engage-seeding is
-- behavior-identical (the krystallus precedent).
local function maidenEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "storm", 10000, function()
        stormTick(creature, guid)
    end)
    schedule(guid, "shock", {20000, 25000}, function()
        shockTick(creature, guid)
    end)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function maidenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function maidenLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function maidenDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 1, maidenEnterCombat)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 2, maidenLeaveCombat)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 3, maidenTargetDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 4, maidenDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 23, maidenLeaveCombat)
