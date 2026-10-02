-- Keristrasza (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_keristrasza.cpp
-- (boss_keristrasza (CreatureScript) via GetNexusAI<boss_keristraszaAI>
-- (BossAI, DATA_KERISTRASZA); containment_sphere (GameObjectScript) via
-- GetNexusAI<containment_sphereAI> (GameObjectAI); achievement_intense_
-- cold (AchievementCriteriaScript); spell_intense_cold (SpellScriptLoader);
-- AddSC_boss_keristrasza() at end registers all four; loader decl 81 /
-- call 276 per northrend_script_loader.cpp — the FIFTH group of the
-- "// The Nexus: Nexus" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_ormorok() (decl 80 / call 275); the call after it is
-- AddSC_instance_nexus() (decl 82 / call 277) — loader order confirmed
-- this run; the checkpoint sequence (ormorok -> keristrasza) is followed).
-- Entry: 26723 Keristrasza (nexus.h NPC_KERISTRASZA, line 44;
-- instance_nexus.cpp OnCreatureCreate binds case NPC_KERISTRASZA (line
-- 53) — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_keristrasza"
-- hits boss_keristrasza.cpp + the loader decl/call lines only; grep for
-- "containment_sphere" / "achievement_intense_cold" / "spell_intense_
-- cold" hits boss_keristrasza.cpp only (registered from inside
-- AddSC_boss_keristrasza, no separate loader lines); zero sql/ hits. No
-- keristrasza lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied. No
-- timers in the ported arms; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the DoCastAOE(SPELL_
-- INTENSE_COLD 48094) leg has no DoCastAOE bridge — nexus_commanders
-- EVENT_FRIGHTENING_SHOUT precedent — the BossAI::JustEngagedWith
-- instance-bookkeeping leg has no bridge — tharon_ja precedent — and
-- the three events.ScheduleEvent legs (EVENT_CRYSTAL_FIRE_BREATH 14s /
-- EVENT_CRYSTAL_CHAINS_CRYSTALLIZE DUNGEON_MODE(30s, 11s) / EVENT_TAIL_
-- SWEEP 5s) have no timer-event bridge).
-- JustDied — Talk(SAY_DEATH 3) (event 4; the _JustDied bookkeeping has
-- no bridge — tharon_ja precedent).
-- KilledUnit — Talk(SAY_SLAY 1) (event 3 — C++-GATED: who->
-- GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad player-gate
-- variant — seventh ported variant, fifth identical to nalorakk /
-- kelthuzad / gothik / thaddius).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (_enrage=false, _intenseCold=true) + _intense_
-- ColdList.clear() + RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED) +
-- RemovePrison(CheckContainmentSpheres()) + _Reset() — no instance /
-- GO-state / flag bridges (the containment-sphere prison machine is
-- documented-only; see containment_sphere below).
-- DamageTaken — HealthBelowPctDamaged(25) -> Talk(SAY_ENRAGE 2) +
-- Talk(SAY_FRENZY 5) + DoCast(me, SPELL_ENRAGE 8599), one-shot latch —
-- no health-pct bridge.
-- UpdateAI — EVENT_CRYSTAL_FIRE_BREATH: DoCastVictim(SPELL_CRYSTALFIRE_
-- BREATH 48096), 14s repeat; EVENT_CRYSTAL_CHAINS_CRYSTALLIZE:
-- DoCast(me, SPELL_TAIL_SWEEP 50155), 5s repeat (DUNGEON_MODE init
-- 30s/11s); EVENT_TAIL_SWEEP: Talk(SAY_CRYSTAL_NOVA 4) + heroic
-- DoCast(me, SPELL_CRYSTALLIZE 48179) / normal SelectTarget(Random, 0,
-- 100.0f, true) -> DoCast(SPELL_CRYSTAL_CHAINS 50997), DUNGEON_MODE
-- (30s, 11s) repeat — no timer-event / random-target SelectTarget /
-- cast bridges.
-- SetGUID(DATA_INTENSE_COLD) — _intenseColdList push, consumed only by
-- achievement_intense_cold's OnCheck — no SetGUID / GetData bridge and
-- no achievement bridge.
-- containment_sphere — GameObjectScript: OnGossipHello — keristrasza
-- via instance->GetGuidData(DATA_KERISTRASZA) -> alive: SetFlag(GAME_
-- OBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE) + SetGoState(GO_STATE_ACTIVE)
-- + cross-AI CheckContainmentSpheres(true) (instance GetGuidData
-- ANOMALUS/ORMOROK/TELESTRAS_CONTAINMENT_SPHERE -> ObjectAccessor::
-- GetGameObject + GO_STATE_ACTIVE gate) -> RemovePrison(true)
-- (SetImmuneToPC(false) + RemoveFlag(NON_ATTACKABLE) +
-- RemoveAurasDueToSpell(FROZEN_PRISON 47854)) — joins the instance-
-- model-blocked gossip queue.
-- spell_intense_cold — AuraScript HandlePeriodicTick (EFFECT_1
-- SPELL_AURA_PERIODIC_DAMAGE): stack < 2 early-return; caster AI ->
-- SetGUID(target GUID, DATA_INTENSE_COLD) (caster should be the boss
-- but is the player in practice — the C++ @todo) — no AuraScript
-- bridge.
-- achievement_intense_cold — OnCheck: ENSURE_AI(boss_keristraszaAI)->
-- _intenseColdList not containing the player's GUID — no achievement
-- bridge (kelthuzad/thaddius precedent).

local ENTRY_KERISTRASZA = 26723

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_DEATH = 3

-- C++ JustEngagedWith: Talk(SAY_AGGRO) (the DoCastAOE(SPELL_INTENSE_
-- COLD 48094) leg has no DoCastAOE bridge — nexus_commanders
-- precedent — the BossAI::JustEngagedWith instance-bookkeeping leg has
-- no bridge — tharon_ja precedent — and the three events.ScheduleEvent
-- legs have no timer-event bridge).
local function keristraszaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
local function keristraszaDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: Talk(SAY_SLAY) only when who->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function keristraszaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_KERISTRASZA, 1, keristraszaEnterCombat)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 3, keristraszaTargetDied)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 4, keristraszaDied)
