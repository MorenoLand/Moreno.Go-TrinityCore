-- Baltharus the Warborn (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_baltharus_the_warborn.cpp
-- (368 lines incl. license; 2 CreatureScripts (boss_baltharus_the_warborn
-- (BossAI, DATA_BALTHARUS_THE_WARBORN = 0) +
-- npc_baltharus_the_warborn_clone (BossAI, DATA_BALTHARUS_CLONE = 19));
-- all registered from inside AddSC_boss_baltharus_the_warborn()
-- (line 363); loader decl 190 / call 385 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_ruby_sanctum() (call 384) — verified from the loader this
-- run; the checkpoint sequence (ruby_sanctum -> baltharus) is
-- followed).
-- Entry: 39751 Baltharus the Warborn (ruby_sanctum.h
-- NPC_BALTHARUS_THE_WARBORN, line 67; instance_ruby_sanctum.cpp
-- creatureData binds NPC_BALTHARUS_THE_WARBORN (line 49)) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_baltharus_the_warborn" hits boss_baltharus_the_warborn.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_baltharus_the_warborn JustEngagedWith — Talk(SAY_AGGRO 1)
-- (event 1 — the auriaya precedent; the InterruptNonMeleeSpells /
-- BossAI::JustEngagedWith / event-scheduling legs have no bridges).
-- boss_baltharus_the_warborn JustDied — Talk(SAY_DEATH 4) (event 4 —
-- the sjonnir precedent; the _JustDied() instance leg has no instance
-- bridge and the xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH)
-- cross-AI leg has no cross-AI-DoAction bridge).
-- boss_baltharus_the_warborn KilledUnit — Talk(SAY_KILL 2) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; only entry 39751 registered):
-- boss_baltharus_the_warborn DoAction — Talk(SAY_CLONE 3) rides
-- ACTION_CLONE (no-DoAction bridge).
-- boss_baltharus_the_warborn UpdateAI scheduler — Talk(SAY_BALTHARUS_INTRO
-- 0) rides EVENT_INTRO_TALK (no-timer-bridge; the EVENT_INTRO_TALK leg
-- itself rides DoAction(ACTION_INTRO_BALTHARUS) issued by
-- at_baltharus_plateau — no cross-AI-DoAction bridge; the
-- EVENT_CLEAVE / EVENT_BLADE_TEMPEST / EVENT_ENERVATING_BRAND /
-- EVENT_SUMMONS_ATTACK / EVENT_CLONE DoCastSelf legs have no-cast
-- bridge).
-- npc_baltharus_the_warborn_clone (39899) — zero Talk lines (NOT
-- registered, the bronjahm npc_corrupted_soul_fragment precedent); the
-- DamageTaken DATA_BALTHARUS_SHARED_HEALTH leg and the JustDied
-- Unit::Kill(baltharus) leg ride instance/actor bridges that don't
-- exist.
-- spell_baltharus_enervating_brand_trigger (SpellScript, OnHit
-- HandleSiphonedMight: aura-caster CastSpell SPELL_SIPHONED_MIGHT) —
-- joins the standing no-SpellScript-bridge queue.
-- All 5 Talk() calls in the file accounted for (SAY_AGGRO 1 +
-- SAY_DEATH 4 + SAY_KILL 2 ported; SAY_CLONE 3 + SAY_BALTHARUS_INTRO 0
-- documented above).

local ENTRY_BALTHARUS_THE_WARBORN = 39751

local SAY_AGGRO = 1
local SAY_KILL  = 2
local SAY_DEATH = 4

-- C++ boss_baltharus_the_warborn::JustEngagedWith:
-- me->InterruptNonMeleeSpells(false); BossAI::JustEngagedWith(who);
-- events.Reset(); events.SetPhase(PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_CLEAVE, Seconds(13), 0, PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_ENERVATING_BRAND, Seconds(13), 0, PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_BLADE_TEMPEST, Seconds(18), 0, PHASE_COMBAT);
-- Talk(SAY_AGGRO) — the auriaya precedent (the non-Talk legs have no
-- bridges).
local function baltharusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_baltharus_the_warborn::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function baltharusKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_baltharus_the_warborn::JustDied:
-- _JustDied();
-- Talk(SAY_DEATH);
-- if (Creature* xerestrasza = instance->GetCreature(DATA_XERESTRASZA))
--     xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH) — the sjonnir
-- precedent (the _JustDied() instance leg and the cross-AI DoAction leg
-- have no bridges).
local function baltharusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 1, baltharusEnterCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 3, baltharusKilledUnit)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 4, baltharusDied)
