-- Deathbringer Saurfang (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_deathbringer_saurfang.cpp
-- (1275 lines incl. license; 4 CreatureScripts
-- (boss_deathbringer_saurfang (BossAI, DATA_DEATHBRINGER_SAURFANG =
-- 3), npc_high_overlord_saurfang_icc (ScriptedAI intro/outro event
-- NPC), npc_muradin_bronzebeard_icc (ScriptedAI intro/outro event
-- NPC), npc_saurfang_event (ScriptedAI guard)) + 7 SpellScripts
-- (spell_deathbringer_blood_link, spell_deathbringer_blood_power,
-- spell_deathbringer_rune_of_blood, spell_deathbringer_blood_nova,
-- spell_deathbringer_blood_nova_targeting,
-- spell_deathbringer_boiling_blood, spell_deathbringer_remove_marks)
-- + 3 AuraScripts (spell_deathbringer_blood_link_aura,
-- spell_deathbringer_blood_power_aura,
-- spell_deathbringer_blood_beast_blood_link) + 1
-- AchievementCriteriaScript
-- (achievement_ive_gone_and_made_a_mess); all registered from
-- inside AddSC_boss_deathbringer_saurfang(); loader decl 174 /
-- call 369 per northrend_script_loader.cpp — the FOURTH group of
-- the "// Icecrown Citadel" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_icecrown_gunship_battle() (call
-- 368); the call after it is AddSC_boss_festergut() — verified
-- from the loader this run; the checkpoint sequence
-- (boss_icecrown_gunship_battle -> boss_deathbringer_saurfang) is
-- followed).
-- Entry: 37813 Deathbringer Saurfang (icecrown_citadel.h
-- NPC_DEATHBRINGER_SAURFANG, line 211; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_DEATHBRINGER_SAURFANG, line 227)
-- — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- DOCUMENTED-ENTRY EVIDENCE (for later reference, not registered):
-- icecrown_citadel.h — NPC_SE_HIGH_OVERLORD_SAURFANG = 37187
-- (line 216, spawned when TeamInInstance == HORDE);
-- NPC_SE_MURADIN_BRONZEBEARD = 37200 (line 214, spawned when
-- TeamInInstance == ALLIANCE).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_deathbringer_saurfang" hits
-- boss_deathbringer_saurfang.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction. No saurfang lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 37813):
-- boss_deathbringer_saurfang DamageTaken — Talk(SAY_FRENZY 11)
-- rides the HealthBelowPct(31) gate (no health-pct bridge; the
-- garfrost precedent); Talk(SAY_DEATH 13) rides the
-- FightWonValue health gate of the fight-won leg (no bridge —
-- JustDied is EMPTY in C++, so the sjonnir JustDied-Talk
-- precedent does NOT apply here); the SPELL_REMOVE_MARKS /
-- SPELL_ACHIEVEMENT / SPELL_REPUTATION_BOSS_KILL /
-- SPELL_PERMANENT_FEIGN_DEATH casts and the
-- creature->AI()->DoAction(ACTION_START_OUTRO) cross-AI leg have no
-- bridge.
-- boss_deathbringer_saurfang SpellHitTarget —
-- Talk(SAY_MARK_OF_THE_FALLEN_CHAMPION 8) rides the
-- SPELL_MARK_OF_THE_FALLEN_CHAMPION case of the SpellHitTarget
-- switch — no-SpellHit-bridge queue (the SpellHit-15-never-fires
-- queue family).
-- boss_deathbringer_saurfang intro machine —
-- Talk(SAY_INTRO_ALLIANCE_2 0 / SAY_INTRO_ALLIANCE_3 1 /
-- SAY_INTRO_ALLIANCE_6 2 / SAY_INTRO_ALLIANCE_7 3 /
-- SAY_INTRO_HORDE_2 4 / SAY_INTRO_HORDE_4 5 / SAY_INTRO_HORDE_9 6)
-- ride the EVENT_INTRO_* scheduler machine (no-timer-bridge; the
-- boss_toravon precedent); the DoAction(PHASE_INTRO_A/H /
-- ACTION_CONTINUE_INTRO) legs have no-DoAction bridge (the krick
-- ACTION_OUTRO precedent); the ACTION_MARK_OF_THE_FALLEN_CHAMPION
-- DoAction leg (random-target SelectTarget + DoCast) has no
-- bridge.
-- boss_deathbringer_saurfang combat scheduler legs —
-- Talk(SAY_BLOOD_BEASTS 9) rides EVENT_SUMMON_BLOOD_BEAST,
-- Talk(SAY_BERSERK 12) rides EVENT_BERSERK, Talk(EMOTE_SCENT_OF_BLOOD
-- 14) rides EVENT_SCENT_OF_BLOOD — timer-driven (no-timer-bridge);
-- the DoCastSelf/DoCastAOE/DoCastVictim legs have no-cast bridge;
-- the EVENT_BLOOD_NOVA / EVENT_RUNE_OF_BLOOD / EVENT_BOILING_BLOOD
-- casts ride the no-SpellScript-bridge queue.
-- boss_deathbringer_saurfang JustEngagedWith scheduler legs — the
-- 5 EVENT_* ScheduleEvent legs (no-timer-bridge); the
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- DoZoneInCombat / instance->SetBossState(IN_PROGRESS) legs have no
-- instance bridge; JustReachedHome instance->SetBossState(FAIL)
-- has no instance bridge.
-- npc_high_overlord_saurfang_icc / npc_muradin_bronzebeard_icc —
-- zero bridgeable arms, NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent): Talk(SAY_INTRO_HORDE_1 0
-- / SAY_INTRO_ALLIANCE_1 0) rides DoAction(ACTION_START_EVENT)
-- (no-DoAction bridge; fired from OnGossipSelect — the
-- instance-model-blocked gossip queue); Talk(SAY_INTRO_HORDE_3 1 /
-- SAY_INTRO_ALLIANCE_4 1) rides MovementInform POINT_FIRST_STEP
-- (no-motion bridge); Talk(SAY_INTRO_HORDE_5..8 2..5 /
-- SAY_INTRO_ALLIANCE_5 2) ride scheduler events (no-timer-bridge);
-- Talk(SAY_OUTRO_HORDE_1..4 11..14 / SAY_OUTRO_ALLIANCE_1 3) ride
-- DoAction(ACTION_START_OUTRO) / scheduler (no-DoAction /
-- no-timer-bridge); the cross-AI
-- deathbringer->AI()->DoAction(PHASE_INTRO_*) legs have no bridge.
-- npc_saurfang_event (guards) — zero own Talk lines — NOT
-- registered (the bronjahm npc_corrupted_soul_fragment precedent).
-- spell_deathbringer_blood_link / spell_deathbringer_blood_power /
-- spell_deathbringer_rune_of_blood / spell_deathbringer_blood_nova /
-- spell_deathbringer_blood_nova_targeting /
-- spell_deathbringer_boiling_blood / spell_deathbringer_remove_marks
-- join the no-SpellScript-bridge queue (the boss_moragg
-- optic-link precedent); spell_deathbringer_blood_link_aura /
-- spell_deathbringer_blood_power_aura /
-- spell_deathbringer_blood_beast_blood_link join the
-- no-AuraScript-bridge queue.
-- achievement_ive_gone_and_made_a_mess joins the no-achievement
-- bridge queue (the gunship achievement_im_on_a_boat precedent).
-- All 29 Talk() calls in the file accounted for (29 = saurfang
-- boss aggro 7 (1) + kill 10 (1) + frenzy 11 (1) + death 13 (1) +
-- mark 8 (1) + boss intro alliance 0-3 (4) + boss intro horde 4-6
-- (3) + blood beasts 9 (1) + berserk 12 (1) + emote 14 (1) [15] +
-- horde saurfang intro/outro 0/1/2..5/11..14 (10) + alliance
-- muradin intro/outro 0/1/2/3 (4)).

local ENTRY_DEATHBRINGER_SAURFANG = 37813

local SAY_AGGRO = 7
local SAY_KILL = 10

-- C++ boss_deathbringer_saurfang::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function saurfangJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_deathbringer_saurfang::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function saurfangKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 1, saurfangJustEngagedWith)
RegisterCreatureEvent(ENTRY_DEATHBRINGER_SAURFANG, 3, saurfangKilledUnit)
