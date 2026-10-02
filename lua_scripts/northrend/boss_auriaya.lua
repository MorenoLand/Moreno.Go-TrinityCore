-- Auriaya (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_auriaya.cpp
-- (boss_auriaya (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_auriaya> (BossAI), BOSS_AURIAYA = 6
-- — registered from inside AddSC_boss_auriaya(); loader decl 110
-- / call 305 per northrend_script_loader.cpp — the FIRST group of
-- the "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_halls_of_stone() (decl 108 / call 303) which closed
-- the Halls of Stone block; the call after it is
-- AddSC_boss_flame_leviathan() (decl 111 / call 306) — loader
-- order confirmed this run; the checkpoint sequence
-- (halls_of_stone -> boss_auriaya) is followed).
-- Entry: 33515 Auriaya (ulduar.h NPC_AURIAYA, line 76 —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_auriaya" hits boss_auriaya.cpp (+ ulduar.h NPC_AURIAYA
-- and the loader decl/call lines for AddSC_boss_auriaya) only;
-- whole-tree grep for "npc_feral_defender",
-- "npc_sanctum_sentry", "npc_swarming_guardian",
-- "npc_seeping_essence_stalker",
-- "spell_auriaya_strenght_of_the_pack",
-- "spell_auriaya_sentinel_blast", "spell_auriaya_agro_creator",
-- "spell_auriaya_random_agro_periodic",
-- "spell_auriaya_feral_essence_removal", "spell_auriaya_feral_rush",
-- "achievement_nine_lives" and "achievement_crazy_cat_lady" hits
-- boss_auriaya.cpp only; zero sql/ hits for all thirteen. No
-- auriaya lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied. No
-- timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the SendEncounterUnit
-- engage leg and the five ScheduleEvent legs have no bridges —
-- the boss_bookkeeping precedent for the passthrough, no
-- timer bridge for the legs).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER
-- AND roll_chance_i(50), then Talk(SAY_SLAY 1) — PORTED
-- (event 3; the razuvious player-gated variant precedent:
-- victim nil-guard, victim:GetObjectType() == "Player", with
-- math.random(1, 100) <= 50 = roll_chance_i(50) — the first
-- chance-gated slay variant ported; twenty-second player-gated
-- variant ported).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33515):
-- Reset / Initialize — _Reset() + _crazyCatLady = true /
-- _nineLives = false + HandleCats(true) grid-respawn — no
-- bridge.
-- DoAction(ACTION_CRAZY_CAT_LADY 0) / DoAction(ACTION_DEFENDER_DIED
-- 1) / GetData(DATA_NINE_LIVES 30763077) / GetData(DATA_CRAZY_CAT_LADY
-- 30063007) — no DoAction / GetData bridges (consumers:
-- npc_sanctum_sentry, npc_feral_defender, achievement_nine_lives,
-- achievement_crazy_cat_lady below).
-- JustDied — DoPlaySoundToSet(15476) + SendEncounterUnit
-- disengage + HandleCats(false) grid-despawn — no Talk arms, no
-- bridges; not registered (event 4 has nothing to model).
-- UpdateAI event machine — EVENT_SONIC_SCREECH (DoCastVictim
-- 64422, 22-30s); EVENT_TERRIFYING_SCREECH (Talk(EMOTE_FEAR 3) +
-- DoCastSelf 64386 + EVENT_BLAST, 36-45s); EVENT_BLAST (DoCastAOE
-- 64389); EVENT_SUMMON_DEFENDER (Talk(EMOTE_DEFENDER 4) +
-- DoCastSelf 64448 + EVENT_ACTIVATE_DEFENDER 2s); EVENT_ACTIVATE_DEFENDER
-- (DoCastSelf 64449); EVENT_SWARNING_GUARDIAN (random-target
-- DoCast 64396, 25-45s); EVENT_BERSERK (DoCastSelf 47008 +
-- Talk(SAY_BERSERK 2), 10min) — the UNIT_STATE_CASTING gates,
-- CatsTargetSelector legs and the DoMeleeAttackIfReady tail have
-- no bridges — no timer-event / cast / target-selection
-- bridges. EMOTE_FEAR / EMOTE_DEFENDER / SAY_BERSERK are
-- timer-leg yells with no bridge; they ride the unbridgeable
-- event machine.
-- npc_sanctum_sentry (CreatureScript via
-- RegisterUlduarCreatureAI<npc_sanctum_sentry> (ScriptedAI);
-- NPC_SANCTUM_SENTRY = 34014, local enum in boss_auriaya.cpp —
-- entry-verifiable from C++ but bridge-blocked, NO registration
-- — the npc_spark_of_ionar / stormforged_lieutenant
-- no-bridgeable-arms precedent; no Talk arms anywhere in the
-- script — Reset self-cast 64369 has no cast bridge; the
-- EVENT_RIP / EVENT_SAVAGE_POUNCE machine has no timer-event /
-- cast / target-selection bridges; JustDied -> GetCreature
-- (BOSS_AURIAYA) -> DoAction(ACTION_CRAZY_CAT_LADY) has no
-- ObjectAccessor / GetData / DoAction bridges).
-- npc_feral_defender (CreatureScript via
-- RegisterUlduarCreatureAI<npc_feral_defender> (ScriptedAI); no
-- creature entry anywhere in C++ evidence — entry unverifiable
-- this run, NO registration; no Talk arms anywhere — the
-- Reset/EventStartCombat/UpdateAI machines and the DamageTaken
-- feign-death/respond machinery ride legs with no bridges (no
-- react-state / aura / target-selection / timer / DoAction
-- bridges); JustDied DoAction(ACTION_DEFENDER_DIED) same
-- DoAction-bridge block).
-- npc_swarming_guardian (CreatureScript via
-- RegisterUlduarCreatureAI<npc_swarming_guardian> (ScriptedAI);
-- no creature entry in C++ evidence, NO registration; no Talk
-- arms — the 1s scheduler aggressivity + DoCastSelf(63709) leg
-- has no timer / cast bridges).
-- npc_seeping_essence_stalker (CreatureScript via
-- RegisterUlduarCreatureAI<npc_seeping_essence_stalker>
-- (ScriptedAI); no creature entry in C++ evidence, NO
-- registration; no Talk arms — Reset self-cast 64458 has no
-- cast bridge).
-- spell_auriaya_strenght_of_the_pack (64381 area-target filter
-- by NPC_SANCTUM_SENTRY entry), spell_auriaya_sentinel_blast
-- (64392 / 64679 area-target filter keeping players and pets),
-- spell_auriaya_agro_creator (63709 dummy: random CatsTarget
-- 5-10.0f -> 64399 + 50000000 threat + AttackStart),
-- spell_auriaya_feral_essence_removal (64456 script-effect:
-- FERAL_ESSENCE 64455 ModStackAmount(-1)),
-- spell_auriaya_feral_rush (64496 / 64674 on-hit -> self
-- 64489) — no SpellScript bridge; all five join the
-- no-SpellScript-bridge queue.
-- spell_auriaya_random_agro_periodic (61906 periodic-dummy
-- aura: random CatsTarget 15-25.0f -> 3000000 threat + 64478 +
-- AttackStart) — no AuraScript bridge; joins the
-- no-AuraScript-bridge queue.
-- achievement_nine_lives (OnCheck: Auriaya AI GetData
-- (DATA_NINE_LIVES) != 0) / achievement_crazy_cat_lady (OnCheck:
-- GetData(DATA_CRAZY_CAT_LADY) != 0) — no GetData /
-- achievement bridges; both join the unmodeled-achievement
-- queue.

local ENTRY_AURIAYA = 33515

local SAY_AGGRO = 0
local SAY_SLAY = 1

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI passthrough +
-- encounter-frame engage + ScheduleEvent legs — only the Talk
-- arm is bridgeable.
local function auriayaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate AND
-- roll_chance_i(50), then Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent plus the roll gate:
-- math.random(1, 100) <= 50 is roll_chance_i(50).
local function auriayaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" and math.random(1, 100) <= 50 then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_AURIAYA, 1, auriayaEnterCombat)
RegisterCreatureEvent(ENTRY_AURIAYA, 3, auriayaTargetDied)
