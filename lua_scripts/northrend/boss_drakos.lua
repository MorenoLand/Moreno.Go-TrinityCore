-- Drakos the Interrogator (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_drakos.cpp
-- (boss_drakos (CreatureScript) via GetOculusAI<boss_drakosAI>
-- (BossAI, DATA_DRAKOS); npc_unstable_sphere (CreatureScript) via
-- GetOculusAI<npc_unstable_sphereAI> (ScriptedAI); AddSC_boss_drakos()
-- at end registers both; loader decl 84 / call 279 per
-- northrend_script_loader.cpp — the FIRST group of the "// The Nexus:
-- The Oculus" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_nexus() (decl 82 / call 277); the call after it is
-- AddSC_boss_urom() — loader order confirmed this run; the
-- checkpoint sequence (instance_nexus -> boss_drakos) is followed).
-- Entry: 27654 Drakos the Interrogator (oculus.h NPC_DRAKOS, line
-- 41; instance_oculus.cpp OnCreatureCreate binds case NPC_DRAKOS
-- (line 59) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- NPC_UNSTABLE_SPHERE (28166) exists only in the .cpp's local Spells
-- enum — local-enum-only like gothik's minions — entry-unverifiable,
-- no registration, no lua for npc_unstable_sphere.
-- Sole-source verified: whole-server-tree grep for "boss_drakos"
-- hits boss_drakos.cpp + the loader decl/call lines only; grep for
-- "npc_unstable_sphere" hits boss_drakos.cpp only (registered from
-- inside AddSC_boss_drakos, no separate loader lines); zero sql/
-- hits. No drakos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. No timers in the ported arms;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the BossAI::
-- JustEngagedWith instance-bookkeeping leg has no instance bridge —
-- tharon_ja precedent; the three events.ScheduleEvent legs have no
-- timer-event bridge).
-- KilledUnit — Talk(SAY_KILL 1) (event 3 — C++-UNCONDITIONAL: the
-- Talk fires for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant).
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied bookkeeping
-- has no bridge — tharon_ja precedent; the instance->
-- DoStartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT,
-- ACHIEV_TIMED_START_EVENT 18153) leg has no achievement bridge —
-- kelthuzad/thaddius precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (postPull=false) + _Reset() + the three
-- events.ScheduleEvent legs (EVENT_MAGIC_PULL 15s / EVENT_STOMP 15s
-- / EVENT_BOMB_SUMMON 2s) — no timer-event / instance bridges.
-- UpdateAI — EVENT_BOMB_SUMMON: for i <= (postPull ? 3 : 0)
-- SummonCreature(NPC_UNSTABLE_SPHERE 28166, GetRandomNearPosition)
-- + 2s repeat — no summon bridge (summon STRAND absent);
-- EVENT_MAGIC_PULL: DoCast(SPELL_MAGIC_PULL 51336) + postPull=true +
-- 15s repeat — no cast bridge; EVENT_STOMP: Talk(SAY_STOMP 4) +
-- DoCast(SPELL_THUNDERING_STOMP 50774) + 15s repeat — no
-- timer-event/cast bridges.
-- npc_unstable_sphere — entry-unverifiable (28166, local-enum-only)
-- — no registration, no lua; its arms are documented here only:
-- Reset — SetReactState(REACT_PASSIVE) + MoveRandom(40.0f) + AddAura(
-- SPELL_UNSTABLE_SPHERE_PASSIVE 50756, SPELL_UNSTABLE_SPHERE_TIMER
-- 50758, self) + DespawnOrUnsummon(19s) — no react-state / motion /
-- aura / despawn bridges; UpdateAI pulseTimer 3000ms ->
-- DoCast(SPELL_UNSTABLE_SPHERE_PULSE 50757) + 3s repeat — no
-- timer-event / cast bridges.

local ENTRY_DRAKOS = 27654

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

-- C++ JustEngagedWith: Talk(SAY_AGGRO) (the BossAI::JustEngagedWith
-- instance-bookkeeping leg has no instance bridge — tharon_ja
-- precedent).
local function drakosEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: Talk(SAY_KILL) — C++-UNCONDITIONAL: the Talk fires
-- for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant.
local function drakosTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent; the instance->DoStartTimedAchievement
-- (ACHIEV_TIMED_START_EVENT 18153) leg has no achievement bridge —
-- kelthuzad/thaddius precedent).
local function drakosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_DRAKOS, 1, drakosEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKOS, 3, drakosTargetDied)
RegisterCreatureEvent(ENTRY_DRAKOS, 4, drakosDied)
