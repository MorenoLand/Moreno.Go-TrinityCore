-- Sartharion (Obsidian Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/boss_sartharion.cpp
-- (boss_sartharion (CreatureScript) via
-- GetObsidianSanctumAI<boss_sartharionAI> (BossAI, DATA_SARTHARION);
-- registered from inside AddSC_boss_sartharion(); loader decl 94 /
-- call 289 per northrend_script_loader.cpp — the FIRST group of the
-- "// Obsidian Sanctum" block in AddNorthrendScripts(), immediately
-- after AddSC_instance_eye_of_eternity() (decl 92 / call 287),
-- under the "// Obsidian Sanctum" marker; the call after it is
-- AddSC_obsidian_sanctum() (decl 95 / call 290) — loader order
-- confirmed this run; the checkpoint sequence
-- (instance_eye_of_eternity -> boss_sartharion) is followed).
-- Entry: 28860 Sartharion (obsidian_sanctum.h NPC_SARTHARION, line 36;
-- DATA_SARTHARION = 0, line 30; EncounterCount = 5, line 27);
-- instance_obsidian_sanctum.cpp OnCreatureCreate binds case
-- NPC_SARTHARION (line 51) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_sartharion"
-- hits boss_sartharion.cpp (+ the loader decl/call lines for
-- AddSC_boss_sartharion) only; zero sql/ hits. No sartharion lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_SARTHARION_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith bookkeeping (setActive /
-- CheckRequiredBosses / SetBossState IN_PROGRESS), DoZoneInCombat,
-- and the FetchDragons machine (PowerOf Tenebron / Shadron /
-- Vesperon casts 61248 / 58105 / 61251, WILL_OF_SARTHARION 61254,
-- drake loot-mode ladder, MotionMaster MovePoints to the drake
-- init positions, UNIT_FLAG_NON_ATTACKABLE legs — all gated on
-- instance GUIDs and the MotionMaster) plus the nine-event
-- ScheduleEvent calls (lava strike / cleave / flame breath / tail
-- sweep / flame tsunami / call tenebron / shadron / vesperon)
-- have no bridges — the tharon_ja / malygos BossAI-bookkeeping and
-- no-cast / no-timer-event precedents).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SARTHARION_SLAY 8) — PORTED (event 3, the
-- razuvious player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player").
-- JustDied — Talk(SAY_SARTHARION_DEATH 6) (event 4; the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent; the three drake
-- kills via instance->GetGuidData(DATA_TENEBRON / DATA_SHADRON /
-- DATA_VESPERON) (NPC_TENEBRON 30452 / NPC_SHADRON 30451 /
-- NPC_VESPERON 30449) have no instance bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28860 — no bridgeable arms):
-- Reset / Initialize — Initialize flag clears; RemoveAurasDueToSpell
-- (SPELL_TWILIGHT_REVENGE 60639); SetHomePosition; DrakeRespawn
-- (instance-boss-state-gated drake respawn + MoveTargetedHome +
-- UNIT_FLAG_NON_ATTACKABLE legs — no instance / motion bridges);
-- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) — no instance bridge.
-- JustReachedHome — _Reset() bookkeeping — no bridge.
-- AddDrakeLootMode — LOOT_MODE_HARD_MODE_1/2/3 ladder — no
-- loot-mode bridge.
-- FetchDragons — instance GUID fetch + IsInCombat / GetVictim
-- gates + casts + MovePoints + loot modes — no instance /
-- motion / cast bridges (drake entries verifiable — 30452 /
-- 30451 / 30449 — but the arms are bridge-blocked, so no
-- separate registration).
-- CallDragon (EVENT_CALL_TENEBRON / SHADRON / VESPERON legs) —
-- entry-switched Talk(SAY_SARTHARION_CALL_TENEBRON 3 /
-- CALL_SHADRON 4 / CALL_VESPERON 5) + AddAura power-of legs +
-- MovePoint(POINT_ID_LAND) legs — no timer-event / cast /
-- motion bridges.
-- GetData TWILIGHT_ACHIEVEMENTS (drakeCount) — no achievement
-- bridge; joins the unmodeled-achievement queue.
-- CastLavaStrikeOnTarget — Cell::VisitAllObjects fire-cyclone
-- search (NPC_FIRE_CYCLONE 30648) + random-cyclone CastSpell
-- (SPELL_LAVA_STRIKE 57571) on target — no creature-search /
-- cast bridges.
-- UpdateAI — the nine-event machine (EVENT_HARD_ENRAGE
-- SPELL_PYROBUFFET 56916 triggered; EVENT_FLAME_TSUNAMI
-- WHISPER_LAVA_CHURN 9 + SummonCreature NPC_FLAME_TSUNAMI 30616
-- waves with MovePoints; EVENT_FLAME_BREATH Talk(SAY_BREATH 2)
-- + DoCastVictim SPELL_FLAME_BREATH 56908; EVENT_TAIL_SWEEP
-- DoCastVictim SPELL_TAIL_LASH 56910; EVENT_CLEAVE_ATTACK
-- DoCastVictim SPELL_CLEAVE 56909; EVENT_LAVA_STRIKE random-target
-- lava strike + urand SAY_SARTHARION_SPECIAL 7; EVENT_CALL_*
-- dragon calls) + the 35%-health SPELL_BERSERK 61632 leg (Talk
-- SAY_SARTHARION_BERSERK 1) gated on the three drake boss states
-- + the 10%-health soft-enrage lava-strike cadence shift — no
-- timer-event / cast / health / instance bridges.
-- Whisper arms (WHISPER_LAVA_CHURN 9) ride the tsunami event —
-- no bridge.

local ENTRY_SARTHARION = 28860

local SAY_SARTHARION_AGGRO = 0
local SAY_SARTHARION_DEATH = 6
local SAY_SARTHARION_SLAY = 8

-- C++ JustEngagedWith: BossAI::JustEngagedWith (setActive /
-- CheckRequiredBosses / SetBossState IN_PROGRESS) + Talk(AGGRO)
-- + DoZoneInCombat + FetchDragons (drake power-aura casts, loot
-- modes, drake init MovePoints, non-attackable flags) +
-- ScheduleEvent x9 — only the Talk arm is bridgeable.
local function sartharionEnterCombat(event, creature, target)
    creature:Talk(SAY_SARTHARION_AGGRO)
end

-- C++ KilledUnit: victim->GetTypeId() == TYPEID_PLAYER gate,
-- then Talk(SAY_SARTHARION_SLAY) — the razuvious player-gated
-- variant precedent.
local function sartharionTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SARTHARION_SLAY)
    end
end

-- C++ JustDied: _JustDied() + Talk(DEATH) + the three drake
-- DisappearAndDie legs via instance GUIDs — only the Talk arm
-- is bridgeable.
local function sartharionDied(event, creature, killer)
    creature:Talk(SAY_SARTHARION_DEATH)
end

RegisterCreatureEvent(ENTRY_SARTHARION, 1, sartharionEnterCombat)
RegisterCreatureEvent(ENTRY_SARTHARION, 3, sartharionTargetDied)
RegisterCreatureEvent(ENTRY_SARTHARION, 4, sartharionDied)
