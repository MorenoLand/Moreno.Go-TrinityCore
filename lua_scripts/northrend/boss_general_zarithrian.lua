-- General Zarithrian (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_general_zarithrian.cpp
-- (2 CreatureScripts: boss_general_zarithrian (BossAI,
-- DATA_GENERAL_ZARITHRIAN = 1) + npc_onyx_flamecaller (ScriptedAI,
-- NPC_ONYX_FLAMECALLER = 39814); both registered from inside
-- AddSC_boss_general_zarithrian() (line 276); loader decl 192 /
-- call 387 per northrend_script_loader.cpp — the FIFTH group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_saviana_ragefire() (call 386) — verified from the
-- loader this run; the checkpoint sequence (saviana -> zarithrian) is
-- followed).
-- Entry: 39746 General Zarithrian (ruby_sanctum.h
-- NPC_GENERAL_ZARITHRIAN, line 72; instance_ruby_sanctum.cpp
-- creatureData binds NPC_GENERAL_ZARITHRIAN ->
-- DATA_GENERAL_ZARITHRIAN (line 52); DATA_GENERAL_ZARITHRIAN = 1
-- (ruby_sanctum.h line 32)) — entry-verifiable, registration proceeds
-- (the nexus_commanders kalecgos precedent); the ScriptName bindings
-- are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_general_zarithrian" hits boss_general_zarithrian.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_general_zarithrian JustEngagedWith — Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent; the BossAI::JustEngagedWith /
-- events.ScheduleEvent(EVENT_CLEAVE / EVENT_INTIDMDATING_ROAR /
-- EVENT_SUMMON_ADDS [/EVENT_SUMMON_ADDS2]) legs have no bridges).
-- boss_general_zarithrian KilledUnit — Talk(SAY_KILL 1) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_general_zarithrian JustDied — Talk(SAY_DEATH 3)
-- (event 4 — the sjonnir precedent; the _JustDied() instance leg has
-- no bridge).
-- DOCUMENTED-ONLY (in this header; only entry 39746 registered):
-- boss_general_zarithrian Reset / CanAIAttack — the
-- instance->GetBossState(DATA_SAVIANA_RAGEFIRE) /
-- instance->GetBossState(DATA_BALTHARUS_THE_WARBORN) legs (gating
-- Reset's RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) / SetImmuneToPC(false)
-- and CanAIAttack) ride the instance-script model (no instance
-- bridge).
-- boss_general_zarithrian JustSummoned — summons.Summon(summon)
-- add-registration leg (no summon-registry bridge).
-- boss_general_zarithrian EnterEvadeMode — summons.DespawnAll() +
-- _DespawnAtEvade() legs (no summon-registry / evade-model bridges).
-- boss_general_zarithrian UpdateAI — Talk(SAY_ADDS 2) rides
-- EVENT_SUMMON_ADDS (no-timer-bridge); the EVENT_SUMMON_ADDS2 stalker
-- CastSpell(SPELL_SUMMON_FLAMECALLER 74398) legs ride cross-creature
-- casts with no-cast / no-AreaTrigger bridges;
-- DoCastSelf(SPELL_INTIMIDATING_ROAR 74384) / DoCastVictim(
-- SPELL_CLEAVE_ARMOR 74367) legs have no-cast bridges;
-- DoMeleeAttackIfReady has no melee-model bridge.
-- npc_onyx_flamecaller (NPC_ONYX_FLAMECALLER = 39814) — zero Talk
-- lines in the file — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent); its JustEngagedWith /
-- UpdateAI scheduler legs (EVENT_BLAST_NOVA / SPELL_BLAST_NOVA 74392
-- DoCastAOE; EVENT_LAVA_GOUT / SPELL_LAVA_GOUT 74394 DoCastVictim,
-- the 3-cast gating) ride no-timer / no-cast bridges; the Reset
-- MoveAlongSplineChain(POINT_GENERAL_ROOM, SPLINE_GENERAL_EAST /
-- SPLINE_GENERAL_WEST) leg rides no-motion-master bridge; the
-- MovementInform DoZoneInCombat leg rides no-movement-inform bridge;
-- the IsSummonedBy zarithrian->AI()->JustSummoned(me) leg rides no
-- cross-AI-summon bridge.
-- All 4 Talk() calls in the file accounted for (SAY_AGGRO 0 +
-- SAY_KILL 1 + SAY_DEATH 3 ported; SAY_ADDS 2 documented above).

local ENTRY_GENERAL_ZARITHRIAN = 39746

local SAY_AGGRO = 0
local SAY_KILL  = 1
local SAY_DEATH = 3

-- C++ boss_general_zarithrian::JustEngagedWith:
-- BossAI::JustEngagedWith(who); Talk(SAY_AGGRO);
-- events.ScheduleEvent(EVENT_CLEAVE, 8s);
-- events.ScheduleEvent(EVENT_INTIDMDATING_ROAR, 14s);
-- events.ScheduleEvent(EVENT_SUMMON_ADDS, 15s); [+ 16s 25-man
-- EVENT_SUMMON_ADDS2] — the auriaya precedent (the non-Talk legs have
-- no bridges).
local function zarithrianEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_general_zarithrian::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function zarithrianKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_general_zarithrian::JustDied:
-- _JustDied(); Talk(SAY_DEATH) — the sjonnir precedent (the
-- _JustDied() instance leg has no bridge).
local function zarithrianDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 1, zarithrianEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 3, zarithrianKilledUnit)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 4, zarithrianDied)
