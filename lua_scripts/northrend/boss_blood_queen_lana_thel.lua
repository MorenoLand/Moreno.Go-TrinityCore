-- Blood-Queen Lana'thel (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_queen_lana_thel.cpp
-- (843 lines incl. license; 1 CreatureScript
-- (boss_blood_queen_lana_thel (BossAI, DATA_BLOOD_QUEEN_LANA_THEL = 8))
-- + 5 SpellScripts (spell_blood_queen_vampiric_bite,
-- spell_blood_queen_bloodbolt, spell_blood_queen_pact_of_the_darkfallen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg_target,
-- spell_blood_queen_twilight_bloodbolt) + 3 AuraScripts
-- (spell_blood_queen_frenzied_bloodthirst,
-- spell_blood_queen_essence_of_the_blood_queen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg) + 2
-- AchievementCriteriaScripts (achievement_once_bitten_twice_shy_n,
-- achievement_once_bitten_twice_shy_v); all registered from inside
-- AddSC_boss_blood_queen_lana_thel(); loader decl 179 / call 374 per
-- northrend_script_loader.cpp — the NINTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_blood_prince_council() (call 373); the call after
-- it is AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (blood_prince_council ->
-- blood_queen_lana_thel) is followed).
-- Entry: 37955 Blood-Queen Lana'thel (icecrown_citadel.h
-- NPC_BLOOD_QUEEN_LANA_THEL, line 266; icecrown_citadel.h
-- DATA_BLOOD_QUEEN_LANA_THEL = 8, line 82;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_BLOOD_QUEEN_LANA_THEL, line 270) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_queen_lana_thel" hits
-- boss_blood_queen_lana_thel.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings
-- are DB-side by construction. No lana'thel lua existed.
-- (The npc_blood_queen_lana_thel PassiveAI referenced by the
-- council file's intro legs lives in boss_blood_prince_council.cpp
-- and was documented there, not here.)
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 37955):
-- boss_blood_queen_lana_thel JustEngagedWith — instance legs
-- (instance->CheckRequiredBosses / DoCastSpellOnPlayers(
-- LIGHT_S_HAMMER_TELEPORT) / DoZoneInCombat / instance->
-- SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, IN_PROGRESS)) have no
-- instance bridge; the DoCast aura legs have no-cast bridge.
-- boss_blood_queen_lana_thel JustDied — DoCastAOE(
-- SPELL_BLOOD_INFUSION_CREDIT) leg has no-cast bridge;
-- instance->SetData(DATA_BLOOD_QUICKENING_STATE, DONE) /
-- RewardPlayerAndGroupAtEvent / FindNearestCreature minchar legs
-- have no instance bridge.
-- boss_blood_queen_lana_thel JustReachedHome — Talk(SAY_WIPE 9)
-- rides with the unbridged instance->SetBossState(FAIL) leg; no
-- event-24 port precedent exists in the lua set, so it is
-- documented here rather than wired.
-- boss_blood_queen_lana_thel MovementInform — Talk(SAY_AIR_PHASE
-- 7) rides the POINT_AIR movement-inform leg (no MovementInform
-- bridge); the air-phase machine (MovePoint / SetDisableGravity /
-- SetReactState / scheduler reschedule legs) has no bridge.
-- boss_blood_queen_lana_thel UpdateAI — Talk(EMOTE_BERSERK_RAID
-- 12 / SAY_BERSERK 10) rides the EVENT_BERSERK scheduler leg
-- (no-timer-bridge; the boss_toravon precedent);
-- Talk(SAY_VAMPIRIC_BITE 1) rides EVENT_VAMPIRIC_BITE;
-- Talk(SAY_PACT_OF_THE_DARKFALLEN 6) rides
-- EVENT_PACT_OF_THE_DARKFALLEN; Talk(EMOTE_SWARMING_SHADOWS 5,
-- target) / Talk(SAY_SWARMING_SHADOWS 4) ride
-- EVENT_SWARMING_SHADOWS (no-timer-bridge); the Blood Mirror /
-- Delirious Slash / Twilight Bloodbolt DoCast legs have no-cast
-- bridge.
-- boss_blood_queen_lana_thel DoAction — ACTION_KILL_MINCHAR leg
-- (no-DoAction bridge; the krick ACTION_OUTRO precedent); the
-- EnterEvadeMode / SetGUID vampire-bloodbolt GUID-set bookkeeping
-- rides GUID / cross-AI bridges that do not exist; the quest-credit
-- legs ride the absent quest bridge.
-- spell_blood_queen_frenzied_bloodthirst (AuraScript) —
-- bloodQueen->AI()->Talk(EMOTE_BLOODTHIRST 3, GetTarget()) OnApply
-- and bloodQueen->AI()->Talk(SAY_MIND_CONTROL 2) OnRemove are
-- cross-AI Talk (no cross-AI-Talk bridge); the remaining AuraScript
-- + 5 SpellScript + 2 AchievementCriteriaScript legs join the
-- no-AuraScript-bridge / no-SpellScript-bridge /
-- no-achievement-criteria-bridge queues (the boss_moragg
-- optic-link precedent).
-- All 13 Talk() calls in the file accounted for (13 =
-- SAY_AGGRO 0 (1) + SAY_VAMPIRIC_BITE 1 (1, scheduler) +
-- SAY_MIND_CONTROL 2 (1, cross-AI aura leg) +
-- EMOTE_BLOODTHIRST 3 (1, cross-AI aura leg) +
-- SAY_SWARMING_SHADOWS 4 (1, scheduler) + EMOTE_SWARMING_SHADOWS
-- 5 (1, scheduler) + SAY_PACT_OF_THE_DARKFALLEN 6 (1, scheduler)
-- + SAY_AIR_PHASE 7 (1, MovementInform) + SAY_KILL 8 (1) +
-- SAY_WIPE 9 (1, JustReachedHome) + SAY_BERSERK 10 (1, scheduler)
-- + SAY_DEATH 11 (1) + EMOTE_BERSERK_RAID 12 (1, scheduler)).

local ENTRY_BLOOD_QUEEN_LANA_THEL = 37955

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 11

-- C++ boss_blood_queen_lana_thel::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function lanaThelJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_blood_queen_lana_thel::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function lanaThelKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_blood_queen_lana_thel::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function lanaThelJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 1, lanaThelJustEngagedWith)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 3, lanaThelKilledUnit)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 4, lanaThelJustDied)
