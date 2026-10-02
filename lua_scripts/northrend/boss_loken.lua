-- Loken (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_loken.cpp
-- (boss_loken (CreatureScript) via GetHallsOfLightningAI<boss_lokenAI>
-- (BossAI); spell_loken_pulsing_shockwave (SpellScriptLoader) —
-- registered from inside AddSC_boss_loken(); loader decl 99 /
-- call 294 per northrend_script_loader.cpp — the SECOND group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_bjarngrim() (decl 98 / call 293), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_ionar() (decl 100 / call 295) — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_bjarngrim -> boss_loken) is followed).
-- Entry: 28923 Loken (halls_of_lightning.h NPC_LOKEN, line 42;
-- DATA_LOKEN = 3, line 34); instance_halls_of_lightning.cpp
-- OnCreatureCreate binds case NPC_LOKEN (line 60) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_loken"
-- hits boss_loken.cpp + the loader decl/call lines for
-- AddSC_boss_loken() only; whole-server-tree grep for
-- "spell_loken_pulsing_shockwave" hits boss_loken.cpp only; zero
-- sql/ hits for either. No loken lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 2) (event 1; the BossAI
-- bookkeeping passthrough, events.SetPhase(PHASE_NORMAL), the three
-- ScheduleEvent legs, and
-- instance->DoStartTimedAchievement(ACHIEV_TIMELY_DEATH_START_EVENT
-- 20384) have no bridges — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 4) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — sixteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 8) (event 4; the _JustDied boss
-- bookkeeping passthrough and
-- instance->DoRemoveAurasDueToSpellOnPlayers(SPELL_PULSING_SHOCKWAVE_AURA
-- 59414) have no bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28923):
-- boss_loken Reset / Initialize (_healthAmountModifier = 1 flag +
-- instance->DoStopTimedAchievement(ACHIEV_TIMELY_DEATH_START_EVENT
-- 20384) — no timed-achievement bridge); JustEngagedWith's
-- DoStartTimedAchievement leg — no achievement bridge.
-- MoveInLineOfSight intro (Talk(SAY_INTRO_1 0) gated on
-- _isIntroDone + IsValidAttackTarget + IsWithinDistInMap 40.0f,
-- schedules EVENT_INTRO_DIALOGUE at 20s) — no MoveInLineOfSight
-- bridge.
-- EVENT_INTRO_DIALOGUE (Talk(SAY_INTRO_2 1) +
-- events.SetPhase(PHASE_NORMAL)) — no timer-event bridge.
-- UpdateAI timer machine: EVENT_ARC_LIGHTNING (random-target
-- DoCast(SPELL_ARC_LIGHTNING 52921), 15-16s); EVENT_LIGHTNING_NOVA
-- (Talk(SAY_NOVA 3) + Talk(EMOTE_NOVA 9) +
-- DoCastAOE(SPELL_LIGHTNING_NOVA 52960) +
-- RemoveAurasDueToSpell(SPELL_PULSING_SHOCKWAVE 52961) + reschedule
-- EVENT_RESUME_PULSING_SHOCKWAVE at DUNGEON_MODE(5s, 4s) + reschedule
-- self at 20-21s); EVENT_RESUME_PULSING_SHOCKWAVE
-- (DoCast(me, SPELL_PULSING_SHOCKWAVE_AURA 59414, triggered) +
-- ClearUnitState(UNIT_STATE_CASTING) + DoCast(me,
-- SPELL_PULSING_SHOCKWAVE 52961, triggered)) — no timer-event /
-- cast / target-selection bridges; Talk arms 3 and 9 ride the
-- unmodeled machine.
-- DamageTaken — the health-pct one-shot machine (SAY_75HEALTH 5 /
-- SAY_50HEALTH 6 / SAY_25HEALTH 7 via _healthAmountModifier stepping
-- 100-25*n) — no DamageTaken bridge.
-- spell_loken_pulsing_shockwave (SpellScript OnEffectHitTarget
-- CalculateDamage: damage *= distance2d when > 1.0f,
-- SPELL_PULSING_SHOCKWAVE 52961 EFFECT_0) — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue.

local ENTRY_LOKEN = 28923

local SAY_AGGRO = 2
local SAY_SLAY = 4
local SAY_DEATH = 8

-- C++ JustEngagedWith: BossAI bookkeeping + Talk(AGGRO) +
-- SetPhase(PHASE_NORMAL) + three ScheduleEvent legs +
-- DoStartTimedAchievement — only the Talk arm is bridgeable.
local function lokenEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function lokenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + _JustDied() +
-- DoRemoveAurasDueToSpellOnPlayers(PULSING_SHOCKWAVE_AURA 59414) —
-- only the Talk arm is bridgeable.
local function lokenDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_LOKEN, 1, lokenEnterCombat)
RegisterCreatureEvent(ENTRY_LOKEN, 3, lokenTargetDied)
RegisterCreatureEvent(ENTRY_LOKEN, 4, lokenDied)
