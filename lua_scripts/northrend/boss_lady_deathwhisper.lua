-- Lady Deathwhisper (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lady_deathwhisper.cpp
-- (1071 lines incl. license; 9 scripts — 5 CreatureScripts
-- (boss_lady_deathwhisper (BossAI, DATA_LADY_DEATHWHISPER = 1),
-- npc_cult_fanatic (ScriptedAI trash), npc_cult_adherent (ScriptedAI
-- trash), npc_vengeful_shade (ScriptedAI summon),
-- npc_darnavan (ScriptedAI ally)) + 4 SpellScripts
-- (spell_deathwhisper_mana_barrier (AuraScript),
-- spell_deathwhisper_dominated_mind (AuraScript),
-- spell_deathwhisper_summon_spirits (SpellScript),
-- spell_deathwhisper_vampiric_might (AuraScript)) + 1
-- AreaTriggerScript (at_lady_deathwhisper_entrance); all registered
-- from inside AddSC_boss_lady_deathwhisper(); loader decl 172 / call
-- 367 per northrend_script_loader.cpp — the SECOND group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_lord_marrowgar() (call 366); the call after it is
-- verified from the loader this run; the checkpoint sequence
-- (boss_lord_marrowgar -> boss_lady_deathwhisper) is followed).
-- Entry: 36855 Lady Deathwhisper (icecrown_citadel.h
-- NPC_LADY_DEATHWHISPER, line 174; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_LADY_DEATHWHISPER, line 224) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Entries: 38472 / 38485 Darnavan (boss_lady_deathwhisper.cpp
-- enum, lines 154-155: NPC_DARNAVAN_10 = 38472, NPC_DARNAVAN_25 =
-- 38485; NPC_DARNAVAN = RAID_MODE(38472, 38485, 38472, 38485)) —
-- entry-verifiable from the C++ enum, registration proceeds (the
-- raid-mode dual-entry pattern); ScriptName binding is DB-side.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_lady_deathwhisper" hits boss_lady_deathwhisper.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/
-- tree, so ScriptName bindings are DB-side by construction. No
-- deathwhisper lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entries 36855 /
-- 38472 / 38485):
-- boss_lady_deathwhisper DoAction(ACTION_START_INTRO) —
-- Talk(SAY_INTRO_1 0) rides a DoAction leg with no bridge (the
-- krick ACTION_OUTRO precedent); Talk(SAY_INTRO_2 1 .. SAY_INTRO_7
-- 6) ride the intro scheduler machine — timer-driven
-- (no-timer-bridge; the boss_toravon precedent).
-- boss_lady_deathwhisper JustEngagedWith scheduler legs —
-- Talk(SAY_BERSERK 15) rides a 10-minute scheduler task
-- (no-timer-bridge); Talk(SAY_DOMINATE_MIND 10) rides a scheduler
-- task with random-target SelectTargetList
-- (no-timer-bridge / no-random-target-SelectTarget queues);
-- SummonWaveP1 / Shadow Bolt / DoImproveCultist scheduler tasks
-- with Talk(SAY_DARK_EMPOWERMENT 11 / SAY_DARK_TRANSFORMATION 12)
-- (no-timer-bridge; DoCast legs have no-cast bridge); the
-- CheckRequiredBosses / EnterEvadeMode / instance->DoCastSpellOnPlayers
-- / DoZoneInCombat / instance->SetBossState legs have no instance
-- bridge.
-- boss_lady_deathwhisper DamageTaken — Talk(SAY_PHASE_2 8) +
-- Talk(EMOTE_PHASE_2 9) ride the mana-power pct gate of the phase
-- transition (no health-pct bridge; the garfrost precedent);
-- the mana-barrier aura legs and phase-2 scheduler machine
-- (Frostbolt / Frostbolt Volley / Touch of Insignificance /
-- Summon Spirits casts) have no bridge.
-- boss_lady_deathwhisper JustDied — the Full House achievement
-- legs, the DaranavanMoveEvent, faction/ReactState legs, and
-- KilledMonsterCredit(NPC_DARNAVAN_CREDIT) group loop have no
-- bridge; darnavan->AI()->Talk(SAY_DARNAVAN_RESCUED 1) is Talk on
-- another creature riding instance legs (no instance bridge).
-- Talk(SAY_ANIMATE_DEAD 13) is called by
-- ladyDeathwhisper->AI()->Talk from inside the npc_cult_fanatic /
-- npc_cult_adherent summon-death scheduler legs — timer-driven
-- (no-timer-bridge).
-- npc_cult_fanatic / npc_cult_adherent / npc_vengeful_shade — zero
-- own Talk lines; cast / shadow-bolt / transformation / feign-death
-- legs have no bridge — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- spell_deathwhisper_mana_barrier /
-- spell_deathwhisper_dominated_mind /
-- spell_deathwhisper_vampiric_might join the no-AuraScript-bridge
-- queue (the boss_moragg optic-link precedent);
-- spell_deathwhisper_summon_spirits joins the
-- no-SpellScript-bridge queue.
-- at_lady_deathwhisper_entrance joins the unbridged-area-trigger
-- queue (the pit_of_saron cavern-triggers precedent).
-- All 19 Talk() calls in the file accounted for (19 = lady intro
-- 0-6 (7) + berserk 15 + dominate 10 + aggro 7 + death 16 + kill
-- 14 + phase-2 8 + emote 9 + dark 11/12 (1) + animate-dead 13 (2)
-- + darnavan rescued 1 (1) + darnavan aggro 0 (1)).

local ENTRY_LADY_DEATHWHISPER = 36855
local ENTRY_DARNAVAN_10 = 38472
local ENTRY_DARNAVAN_25 = 38485

local SAY_AGGRO = 7
local SAY_KILL = 14
local SAY_DEATH = 16
local SAY_DARNAVAN_AGGRO = 0

-- C++ boss_lady_deathwhisper::JustEngagedWith: Talk(SAY_AGGRO) — the
-- auriaya engage-port precedent.
local function deathwhisperJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_lady_deathwhisper::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function deathwhisperKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_lady_deathwhisper::JustDied: Talk(SAY_DEATH) — the
-- sjonnir JustDied-Talk precedent.
local function deathwhisperJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

-- C++ npc_darnavan::JustEngagedWith: Talk(SAY_DARNAVAN_AGGRO) —
-- the auriaya engage-port precedent.
local function darnavanJustEngagedWith(event, creature, target)
    creature:Talk(SAY_DARNAVAN_AGGRO)
end

RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 1, deathwhisperJustEngagedWith)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 3, deathwhisperKilledUnit)
RegisterCreatureEvent(ENTRY_LADY_DEATHWHISPER, 4, deathwhisperJustDied)
RegisterCreatureEvent(ENTRY_DARNAVAN_10, 1, darnavanJustEngagedWith)
RegisterCreatureEvent(ENTRY_DARNAVAN_25, 1, darnavanJustEngagedWith)
