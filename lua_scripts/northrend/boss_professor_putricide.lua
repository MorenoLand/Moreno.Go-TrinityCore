-- Professor Putricide (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_professor_putricide.cpp
-- (1517 lines incl. license; 3 CreatureScripts
-- (boss_professor_putricide (BossAI, DATA_PROFESSOR_PUTRICIDE = 6),
-- npc_volatile_ooze (npc_putricide_oozeAI, ScriptedAI),
-- npc_gas_cloud (npc_putricide_oozeAI, ScriptedAI)) + 20 SpellScripts
-- (spell_putricide_ooze_channel, spell_putricide_slime_puddle,
-- spell_putricide_slime_puddle_aura, spell_putricide_unstable_experiment,
-- spell_putricide_ooze_eruption_searcher, spell_putricide_choking_gas_bomb,
-- spell_putricide_unbound_plague, spell_putricide_eat_ooze,
-- spell_putricide_mutation_init, spell_putricide_mutated_transformation,
-- spell_putricide_mutated_transformation_dmg, spell_putricide_regurgitated_ooze,
-- spell_putricide_clear_aura_effect_value, spell_stinky_precious_decimate,
-- spell_abomination_mutated_transformation, spell_putricide_choking_gas_filter)
-- + 5 AuraScripts (spell_putricide_gaseous_bloat,
-- spell_putricide_ooze_tank_protection, spell_putricide_mutated_plague,
-- spell_putricide_mutation_init_aura, spell_putricide_mutated_transformation_dismiss);
-- all registered from inside AddSC_boss_professor_putricide();
-- loader decl 177 / call 372 per northrend_script_loader.cpp — the
-- SEVENTH group of the "// Icecrown Citadel" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_rotface()
-- (call 371); the call after it is AddSC_boss_valithria_dreamwalker()
-- (call 376) — verified from the loader this run; the checkpoint
-- sequence (boss_rotface -> boss_professor_putricide) is followed).
-- Entry: 36678 Professor Putricide (icecrown_citadel.h
-- NPC_PROFESSOR_PUTRICIDE, line 234;
-- icecrown_citadel.h DATA_PROFESSOR_PUTRICIDE = 6, line 80;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_PROFESSOR_PUTRICIDE, line 244) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_professor_putricide" hits
-- boss_professor_putricide.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings
-- are DB-side by construction. No putricide lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36678):
-- boss_professor_putricide UpdateAI — Talk(SAY_BERSERK 12) rides
-- the EVENT_BERSERK scheduler leg (no-timer-bridge; the
-- boss_toravon precedent); Talk(EMOTE_UNSTABLE_EXPERIMENT 5)
-- rides EVENT_UNSTABLE_EXPERIMENT, Talk(EMOTE_MALLEABLE_GOO 9)
-- ×2 rides EVENT_MALLEABLE_GOO (25-man/10-man target-gate legs),
-- Talk(EMOTE_CHOKING_GAS_BOMB 10) rides EVENT_CHOKING_GAS_BOMB
-- (no-timer-bridge); Talk(SAY_FESTERGUT_DEATH 1) rides
-- EVENT_FESTERGUT_DIES and Talk(SAY_ROTFACE_DEATH 3) rides
-- EVENT_ROTFACE_DIES (no-timer-bridge); Talk(SAY_TRANSFORM_1 7)
-- / Talk(SAY_TRANSFORM_2 8) ride the EVENT_PHASE_TRANSITION
-- legs (no-timer-bridge); the Slime Puddle / Unbound Plague /
-- Mutated Plague DoCast legs have no-cast bridge.
-- boss_professor_putricide DoAction — Talk(
-- SAY_FESTERGUT_GASEOUS_BLIGHT 0) rides ACTION_FESTERGUT_GAS,
-- Talk(SAY_ROTFACE_OOZE_FLOOD 2) rides ACTION_ROTFACE_OOZE, and
-- Talk(SAY_PHASE_TRANSITION_HEROIC 6) rides ACTION_CHANGE_PHASE —
-- no-DoAction bridge (the krick ACTION_OUTRO precedent).
-- boss_professor_putricide JustEngagedWith instance legs —
-- instance->DoCastSpellOnPlayers(LIGHT_S_HAMMER_TELEPORT) /
-- DoZoneInCombat / instance->SetBossState(DATA_PROFESSOR_PUTRICIDE,
-- IN_PROGRESS) — no instance bridge; JustDied DoCastAOE(
-- SPELL_UNHOLY_INFUSION_CREDIT) leg has no-cast bridge;
-- JustReachedHome instance->SetBossState(FAIL) — no instance
-- bridge.
-- npc_volatile_ooze / npc_gas_cloud (npc_putricide_oozeAI, lines
-- 718–810) — zero Talk lines — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent); the aura-application /
-- explode legs have no bridge.
-- The 20 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent) and the 5 aura scripts the
-- no-AuraScript-bridge queue.
-- DOCUMENTED-ENTRY EVIDENCE (not registered): icecrown_citadel.h —
-- NPC_VOLATILE_OOZE = 37697 (line 238); NPC_GAS_CLOUD = 37562
-- (line 237); instance_icecrown_citadel.cpp OnCreatureCreate binds
-- both (lines 247–248).
-- All 15 Talk() calls in the file accounted for (15 =
-- SAY_FESTERGUT_GASEOUS_BLIGHT 0 (1, DoAction ACTION_FESTERGUT_GAS)
-- + SAY_FESTERGUT_DEATH 1 (1, EVENT_FESTERGUT_DIES) +
-- SAY_ROTFACE_OOZE_FLOOD 2 (1, DoAction ACTION_ROTFACE_OOZE) +
-- SAY_ROTFACE_DEATH 3 (1, EVENT_ROTFACE_DIES) + SAY_AGGRO 4 (1)
-- + EMOTE_UNSTABLE_EXPERIMENT 5 (1, scheduler) +
-- SAY_PHASE_TRANSITION_HEROIC 6 (1, DoAction ACTION_CHANGE_PHASE)
-- + SAY_TRANSFORM_1 7 (1, EVENT_PHASE_TRANSITION) +
-- SAY_TRANSFORM_2 8 (1, EVENT_PHASE_TRANSITION) +
-- EMOTE_MALLEABLE_GOO 9 (2, scheduler) + EMOTE_CHOKING_GAS_BOMB
-- 10 (1, scheduler) + SAY_KILL 11 (1) + SAY_BERSERK 12 (1,
-- scheduler) + SAY_DEATH 13 (1)).

local ENTRY_PROFESSOR_PUTRICIDE = 36678

local SAY_AGGRO = 4
local SAY_KILL = 11
local SAY_DEATH = 13

-- C++ boss_professor_putricide::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function putricideJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_professor_putricide::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function putricideKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_professor_putricide::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function putricideJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 1, putricideJustEngagedWith)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 3, putricideKilledUnit)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 4, putricideJustDied)
