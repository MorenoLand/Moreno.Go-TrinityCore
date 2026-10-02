-- Ambassador Hellmaw (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_ambassador_hellmaw.cpp
-- (1 CreatureScript — boss_ambassador_hellmaw (EscortAI via
-- GetShadowLabyrinthAI, DATA_AMBASSADOR_HELLMAW = 0); registered from
-- inside AddSC_boss_ambassador_hellmaw() (line 185); loader decl 36 /
-- call 159 per outland_script_loader.cpp — the FIRST group of the
-- "// Auchindoun - Shadow Labyrinth" sub-block in AddOutlandScripts(),
-- immediately after AddSC_instance_sethekk_halls() (call 156) —
-- verified from the loader this run; the checkpoint sequence
-- (instance_sethekk_halls -> ambassador_hellmaw) is followed).
-- Entry: 18731 NPC_AMBASSADOR_HELLMAW — from shadow_labyrinth.h's
-- Creatures enum (line 43); the .cpp file itself carries zero NPC_
-- constants (the boss entry is verifiable from the C++ tree —
-- NPC_ANZU-in-sethekk_halls.h precedent). ScriptName bindings are
-- DB-side as usual. Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_ambassador_hellmaw" hits boss_ambassador_hellmaw.cpp
-- (+ the loader decl/call lines) only. No ambassador_hellmaw lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_ambassador_hellmawAI::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 119; event 1 — the auriaya precedent; the
-- _instance->SetBossState(DATA_AMBASSADOR_HELLMAW, IN_PROGRESS) leg
-- rides the no-instance bridge and is documented-only below).
-- boss_ambassador_hellmawAI::KilledUnit — Talk(SAY_SLAY 3) (line
-- 125) gated on victim->GetTypeId() == TYPEID_PLAYER — the
-- razuvious player-gated precedent (event 3).
-- boss_ambassador_hellmawAI::JustDied — Talk(SAY_DEATH 4) (line
-- 131) — the sjonnir self-Talk precedent (event 4); the
-- _instance->SetBossState(DATA_AMBASSADOR_HELLMAW, DONE) leg rides
-- the no-instance bridge (documented-only).
-- DOCUMENTED-ONLY (in this header; only entry 18731 registered):
-- boss_ambassador_hellmawAI::Reset — _events.Reset() +
-- _instance->SetBossState(NOT_STARTED) + ScheduleEvent
-- (EVENT_CORROSIVE_ACID 5-10s, EVENT_FEAR 25-30s, EVENT_BERSERK 3min
-- heroic-only) + DoAction(ACTION_AMBASSADOR_HELLMAW_BANISH) —
-- no-timer / no-instance bridges — documented-only.
-- boss_ambassador_hellmawAI::DoAction INTRO leg —
-- hellmaw->AI()->DoAction(ACTION_AMBASSADOR_HELLMAW_INTRO) is called
-- from instance_shadow_labyrinth.cpp:137 (the InstanceMapScript —
-- instance_auchenai_crypts precedent: no InstanceScript dispatch
-- bridge) → DoIntro(): _intro one-shot + RemoveAurasDueToSpell
-- (SPELL_BANISH 30231) + Talk(SAY_INTRO 0) (line 112) + Start(true,
-- false, ObjectGuid::Empty, nullptr, false, true) escort leg —
-- no-escort bridge — documented-only. (SAY_HELP = 2 is never used
-- anywhere in the file.)
-- boss_ambassador_hellmawAI::DoAction BANISH leg (lines 97-98):
-- _instance->GetData(DATA_FEL_OVERSEER) + me->HasAura(SPELL_BANISH)
-- gated DoCast(me, SPELL_BANISH 30231, triggered) — no-instance /
-- no-cast bridges — documented-only.
-- boss_ambassador_hellmawAI::MoveInLineOfSight — HasAura
-- (SPELL_BANISH) gate then EscortAI::MoveInLineOfSight — no-escort
-- bridge — documented-only.
-- boss_ambassador_hellmawAI::UpdateEscortAI — the whole machine
-- rides no-timer / no-cast / no-random-target / no-heroic-mode
-- bridges: HasAura(SPELL_BANISH) → EnterEvadeMode(EVADE_REASON_OTHER);
-- EVENT_CORROSIVE_ACID DoCastVictim(SPELL_CORROSIVE_ACID 33551)
-- 15-25s cycle; EVENT_FEAR DoCastAOE(SPELL_FEAR 33547) 20-35s cycle;
-- EVENT_BERSERK heroic-only DoCast(me, SPELL_ENRAGE 34970,
-- triggered); DoMeleeAttackIfReady — documented-only.
-- All 4 Talk() calls in the file accounted for (3 ported above +
-- SAY_INTRO 0 DoAction-driven documented above; SAY_HELP 2 unused).

local ENTRY_AMBASSADOR_HELLMAW = 18731 -- NPC_AMBASSADOR_HELLMAW (shadow_labyrinth.h)

local SAY_AGGRO = 1
local SAY_SLAY  = 3
local SAY_DEATH = 4

-- C++ boss_ambassador_hellmawAI::JustEngagedWith:
-- _instance->SetBossState(DATA_AMBASSADOR_HELLMAW, IN_PROGRESS);
-- Talk(SAY_AGGRO); — the auriaya precedent (the SetBossState leg has
-- no-instance bridge and is documented-only).
local function hellmawEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_ambassador_hellmawAI::KilledUnit:
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:IsPlayer() convention).
local function hellmawTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_ambassador_hellmawAI::JustDied:
-- _instance->SetBossState(DATA_AMBASSADOR_HELLMAW, DONE);
-- Talk(SAY_DEATH); — the sjonnir self-Talk precedent (event 4; the
-- SetBossState leg has no-instance bridge).
local function hellmawDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_AMBASSADOR_HELLMAW, 1, hellmawEnterCombat)
RegisterCreatureEvent(ENTRY_AMBASSADOR_HELLMAW, 3, hellmawTargetDied)
RegisterCreatureEvent(ENTRY_AMBASSADOR_HELLMAW, 4, hellmawDied)
