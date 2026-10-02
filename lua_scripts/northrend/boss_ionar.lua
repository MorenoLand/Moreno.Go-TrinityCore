-- Ionar (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_ionar.cpp
-- (boss_ionar (CreatureScript) via GetHallsOfLightningAI<boss_ionarAI>
-- (ScriptedAI); npc_spark_of_ionar (CreatureScript) via
-- GetHallsOfLightningAI<npc_spark_of_ionarAI> (ScriptedAI) — both
-- registered from inside AddSC_boss_ionar(); loader decl 100 /
-- call 295 per northrend_script_loader.cpp — the THIRD group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_loken() (decl 99 / call 294), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_volkhan() (decl 101 / call 296) — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_loken -> boss_ionar) is followed).
-- Entry: 28546 Ionar (halls_of_lightning.h NPC_IONAR, line 41;
-- DATA_IONAR = 2, line 33); instance_halls_of_lightning.cpp
-- OnCreatureCreate binds case NPC_IONAR (line 57) and GetGuidData
-- handles DATA_IONAR (line 109) — entry-verifiable, registration
-- proceeds (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for "boss_ionar"
-- hits boss_ionar.cpp + the loader decl/call lines for
-- AddSC_boss_ionar() only; whole-server-tree grep for
-- "npc_spark_of_ionar" hits boss_ionar.cpp only; zero sql/ hits
-- for either. No ionar lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- instance->SetBossState(DATA_IONAR, IN_PROGRESS) leg has no
-- instance bridge — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 2) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — seventeenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 3) (event 4; the lSparkList.DespawnAll()
-- and instance->SetBossState(DATA_IONAR, DONE) legs have no
-- bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28546):
-- boss_ionar Reset / Initialize — lSparkList.DespawnAll() +
-- Initialize() flag/timer clears, RemoveFlag / SetControlled /
-- SetVisible legs, instance->SetBossState(DATA_IONAR, NOT_STARTED)
-- — no summon-list / flag / visibility / instance bridges.
-- SpellHit — the SPELL_DISPERSE 52770 split machine (five
-- SPELL_SUMMON_SPARK 52746 casts, AttackStop, SetVisible(false),
-- NON_ATTACKABLE+NOT_SELECTABLE flags, UNIT_STATE_ROOT, motion
-- clear/idle) — no cast / visibility / flag / motion bridges.
-- CallBackSparks — ObjectAccessor GUID loop, SetSpeedRate(MOVE_RUN,
-- 2.0f), MovePoint(DATA_POINT_CALLBACK 0) per alive spark,
-- DespawnOrUnsummon on dead — no ObjectAccessor / motion bridges.
-- DamageTaken — uiDamage = 0 when !me->IsVisible() — no
-- DamageTaken bridge.
-- JustSummoned — NPC_SPARK_OF_IONAR 28926 SummonList latch +
-- SPELL_SPARK_VISUAL_TRIGGER 52667 self-cast + random-target
-- SetInCombatWith + MoveFollow — no summon-list / cast /
-- target-selection / motion bridges.
-- SummonedCreatureDespawn — lSparkList.Despawn latch — no bridge.
-- UpdateAI split machine (uiSplitTimer 25s / 2.5s; bIsSplitPhase
-- CallBackSparks leg; lSparkList.empty() restore leg with
-- SetVisible(true), flag removal, SPELL_SPARK_DESPAWN 52776
-- self-cast, MoveChase) — no timer / visibility / flag / cast /
-- motion bridges.
-- UpdateAI timer events: EVENT-less uiStaticOverloadTimer
-- (random-target DoCast(SPELL_STATIC_OVERLOAD 52658), 5-6s) and
-- uiBallLightningTimer (DoCastVictim(SPELL_BALL_LIGHTNING 52780),
-- 10-11s) — no timer-event / cast / target-selection bridges.
-- UpdateAI disperse health check — !bHasDispersed &&
-- HealthBelowPct(uiDisperseHealth) (45 + urand(0,10)):
-- Talk(SAY_SPLIT 1) + InterruptNonMeleeSpells +
-- DoCast(me, SPELL_DISPERSE 52770) — Talk(1) rides the unmodeled
-- machine; no health-check / interrupt / cast bridges.
-- npc_spark_of_ionar (NPC_SPARK_OF_IONAR 28926, local enum in
-- boss_ionar.cpp — the acolyte_of_shadron / stormforged_lieutenant
-- local-enum precedent) — entry-verifiable but bridge-blocked, no
-- registration (the stormforged_lieutenant no-bridgeable-arms
-- precedent): Reset (REACT_PASSIVE — no react bridge);
-- MovementInform POINT_MOTION_TYPE / DATA_POINT_CALLBACK
-- DespawnOrUnsummon — no motion bridge; DamageTaken uiDamage = 0 —
-- no DamageTaken bridge; UpdateAI boss-state-gated
-- DespawnOrUnsummon + uiCheckTimer distance-to-Ionar
-- (> DATA_MAX_SPARK_DISTANCE 90) MovePoint callback machine via
-- instance->GetGuidData(DATA_IONAR) — no timer / instance / motion
-- / distance bridges. No Talk arms anywhere in the script.

local ENTRY_IONAR = 28546

local SAY_AGGRO = 0
local SAY_SLAY = 2
local SAY_DEATH = 3

-- C++ JustEngagedWith: Talk(AGGRO) +
-- instance->SetBossState(DATA_IONAR, IN_PROGRESS) — only the Talk
-- arm is bridgeable.
local function ionarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function ionarTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + lSparkList.DespawnAll() +
-- instance->SetBossState(DATA_IONAR, DONE) — only the Talk arm is
-- bridgeable.
local function ionarDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_IONAR, 1, ionarEnterCombat)
RegisterCreatureEvent(ENTRY_IONAR, 3, ionarTargetDied)
RegisterCreatureEvent(ENTRY_IONAR, 4, ionarDied)
