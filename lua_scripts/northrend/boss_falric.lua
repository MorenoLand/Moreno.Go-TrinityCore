-- Falric (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/boss_falric.cpp
-- (1 script — boss_falric (CreatureScript via boss_horAI (custom
-- HallsOfReflection base AI wrapping GetHallsOfReflectionAI),
-- DATA_FALRIC = 0); registered from inside AddSC_boss_falric();
-- loader decl 168 / call 363 per northrend_script_loader.cpp — the THIRD
-- group of the "// Halls of Reflection" block in AddNorthrendScripts(),
-- immediately after AddSC_halls_of_reflection() (call 362); the call
-- after it is AddSC_boss_marwyn() (call 364) — verified from the loader
-- this run; the checkpoint sequence (halls_of_reflection ->
-- boss_falric) is followed).
-- Entry: 38112 Falric (halls_of_reflection.h NPC_FALRIC, line 71;
-- instance_halls_of_reflection.cpp OnCreatureCreate binds case
-- NPC_FALRIC, line 132) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_falric"
-- hits boss_falric.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No falric lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill (player-gated), 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_falric JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 — the auriaya
-- engage-port precedent; the DoZoneInCombat, instance->SetBossState
-- IN_PROGRESS and three ScheduleEvent legs have no bridge).
-- boss_falric KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the
-- razuvious player-gated variant precedent; the tenth ported variant,
-- eighth identical to
-- nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus).
-- boss_falric JustDied — Talk(SAY_DEATH 2) (event 4 — the sjonnir
-- JustDied-Talk precedent; the events.Reset and instance->SetBossState
-- DONE legs have no bridge). Melee engine-driven.
-- DOCUMENTED-ONLY (in this header; only entry 38112 registered):
-- boss_falric Reset — boss_horAI::Reset passthrough + Initialize
-- hopelessness counter — no bridge by construction.
-- boss_falric JustEngagedWith scheduler legs — EVENT_QUIVERING_STRIKE
-- 23s / EVENT_IMPENDING_DESPAIR 9s / EVENT_DEFILING_HORROR 21s-39s —
-- no-timer-bridge queue (the boss_toravon precedent).
-- boss_falric UpdateAI — the whole EventMap machine (QUIVERING_STRIKE
-- victim-cast; IMPENDING_DESPAIR random-target cast with
-- Talk(SAY_IMPENDING_DESPAIR 3) riding the timer leg; DEFILING_HORROR
-- AoE) — no-timer-bridge / no-random-target-SelectTarget queues; the
-- UNIT_STATE_CASTING gate, DoCast / DoCastVictim / DoCastAOE and
-- DoMeleeAttackIfReady legs have no bridge.
-- boss_falric DamageTaken — the hopelessness machine
-- (_hopelessnessCount < 1/2/3 + HealthBelowPctDamaged(66/33/10) +
-- RemoveOwnedAura / DoCast HOPELESSNESS_1/2/3) — no health-pct bridge
-- (the garfrost phase-change precedent); the aura-cleansing and cast
-- legs have no bridge by construction.

local ENTRY_FALRIC = 38112

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_falricAI::JustEngagedWith: Talk(SAY_AGGRO) — the auriaya
-- engage-port precedent.
local function falricJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_falricAI::KilledUnit: if (who->GetTypeId() != TYPEID_PLAYER)
-- return; Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function falricKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_falricAI::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function falricJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_FALRIC, 1, falricJustEngagedWith)
RegisterCreatureEvent(ENTRY_FALRIC, 3, falricKilledUnit)
RegisterCreatureEvent(ENTRY_FALRIC, 4, falricJustDied)
