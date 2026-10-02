-- Thorim (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_thorim.cpp
-- (boss_thorim (CreatureScript) via GetUlduarAI<boss_thorimAI>
-- (BossAI), BOSS_THORIM = 8 — registered from inside
-- AddSC_boss_thorim(); loader decl 121 / call 316 per
-- northrend_script_loader.cpp — the TWELFTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_freya() (decl 120 / call 315); the call
-- after it is AddSC_boss_yogg_saron() — loader order
-- confirmed this run; the checkpoint sequence (boss_freya
-- -> boss_thorim) is followed).
-- Entry: 32865 Thorim (ulduar.h NPC_THORIM, line 79 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the twenty script names (\"boss_thorim\" (+ the loader
-- decl/call lines for AddSC_boss_thorim),
-- \"npc_thorim_pre_phase\", \"npc_thorim_arena_phase\",
-- \"npc_runic_colossus\", \"npc_ancient_rune_giant\",
-- \"npc_sif\", \"spell_thorim_blizzard_effect\",
-- \"spell_thorim_frostbolt_volley\", \"spell_thorim_charge_orb\",
-- \"spell_thorim_lightning_charge\", \"spell_thorim_stormhammer\",
-- \"spell_thorim_stormhammer_sif\",
-- \"spell_thorim_stormhammer_boomerang\",
-- \"spell_thorim_arena_leap\", \"spell_thorim_runic_smash\",
-- \"spell_thorim_activate_lightning_orb_periodic\",
-- \"achievement_dont_stand_in_the_lightning\",
-- \"achievement_lose_your_illusion\",
-- \"achievement_i_ll_take_you_all_on\",
-- \"condition_thorim_arena_leap\") hits boss_thorim.cpp
-- only; zero sql/ hits for all twenty.
-- No thorim lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO_1 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the phase-1
-- ScheduleEvent legs (EVENT_SAY_AGGRO_2 / EVENT_SAY_SIF_START /
-- EVENT_START_SIF_CHANNEL / EVENT_STORMHAMMER / EVENT_CHARGE_ORB /
-- EVENT_SUMMON_ADDS / EVENT_BERSERK), the DoCast
-- SPELL_SHEATH_OF_LIGHTNING leg, the runic-colossus
-- immunity/DoAction leg and the lever-flag leg have no
-- bridges — the auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 4) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the thirty-fourth
-- player-gated variant ported).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entry 32865):
-- Talk(SAY_DEATH 7) lives in FinishEncounter() (DoCastAOE
-- SPELL_CREDIT_KILL + REACT_PASSIVE + InterruptNonMeleeSpells +
-- RemoveAllAttackers + AttackStop + SetFaction FRIENDLY +
-- SetFlag UNIT_FLAG_RENAME + controller/pillar RemoveAllAuras +
-- hard-mode Sif despawn + _JustDied()) — FinishEncounter is
-- called from the phase-1/phase-2 event machine (trash-clear
-- and pillar legs), not from a bridgeable hook; event 4 is
-- deliberately NOT registered since C++ never calls
-- Talk(SAY_DEATH) from JustDied.
-- Timer Talks — Talk(SAY_AGGRO_2 1) (EVENT_SAY_AGGRO_2, 9s),
-- Talk(SAY_WIPE 6) (EVENT_WIPE), Talk(SAY_BERSERK 5)
-- (EVENT_BERSERK), the outro Talks Talk(SAY_END_NORMAL_1 8) /
-- Talk(SAY_END_NORMAL_2 9) / Talk(SAY_END_NORMAL_3 10) and
-- their hard-mode variants 11/12/13, sif Talk(SAY_SIF_START 0)
-- (EVENT_SAY_SIF_START — cross-creature, the mimiron
-- precedent) — the timer event machine has no bridge; all
-- join the no-timer-bridge queue.
-- npc_thorim_pre_phaseAI DamageTaken — Talk(SAY_JUMPDOWN 3)
-- gated on PHASE_1 + CanStartPhase2(attacker)
-- (TYPEID_PLAYER within 10y + DATA_RUNIC_COLOSSUS /
-- DATA_RUNE_GIANT dead) — no phase / instance GuidData /
-- distance bridges; the event-9 DamageTaken bridge cannot
-- model the condition — bridge-blocked, no registration.
-- npc_thorim_pre_phaseAI / npc_thorim_arena_phaseAI share
-- creature entry 32865 (phase variants selected by
-- GetUlduarAI on instance state) — both have no bridgeable
-- Talk arms of their own; one registration covers the
-- creature.
-- npc_runic_colossusAI JustDied — opens the Runic Door via
-- HandleGameObject(DATA_RUNIC_DOOR) + cross-creature
-- thorim->AI()->Talk(SAY_SPECIAL 2) — no GO / cross-creature
-- Talk bridges (the mimiron cross-creature precedent);
-- JustEngagedWith / Reset / DoAction legs — no Talk arms,
-- no bridges; Talk(EMOTE_RUNIC_BARRIER 0) rides the timer
-- event machine — no registration (NPC_RUNIC_COLOSSUS 32872
-- entry-verifiable, ulduar.h line 179).
-- npc_ancient_rune_giantAI JustDied — opens the Stone Door
-- via HandleGameObject(DATA_STONE_DOOR) — no GO bridge;
-- JustEngagedWith / Reset legs — no Talk arms;
-- Talk(EMOTE_RUNIC_MIGHT 0) rides the timer event machine —
-- no registration (NPC_RUNE_GIANT 32873 entry-verifiable,
-- ulduar.h line 180).
-- npc_sifAI — Talk(SAY_SIF_EVENT 2) via
-- DoAction(ACTION_START_HARD_MODE) — no DoAction bridge;
-- SpellHit(STORMHAMMER_SIF) / Reset legs — no bridges; no
-- registration (NPC_SIF 33196 entry-verifiable, ulduar.h
-- line 192).
-- The ten spell scripts
-- (spell_thorim_blizzard_effect / spell_thorim_frostbolt_volley /
-- spell_thorim_charge_orb / spell_thorim_lightning_charge /
-- spell_thorim_stormhammer / spell_thorim_stormhammer_sif /
-- spell_thorim_stormhammer_boomerang / spell_thorim_arena_leap /
-- spell_thorim_runic_smash /
-- spell_thorim_activate_lightning_orb_periodic) — SpellScript /
-- AuraScript — no SpellScript / AuraScript bridges; all join
-- the no-SpellScript / no-AuraScript-bridge queues.
-- condition_thorim_arena_leap (ConditionScript: arena-leap
-- phase gating) — no ConditionScript bridge; joins the
-- unmodeled-condition queue.
-- achievement_dont_stand_in_the_lightning /
-- achievement_lose_your_illusion /
-- achievement_i_ll_take_you_all_on (OnCheck: GetData
-- legs — DATA_DONT_STAND_IN_THE_LIGHTNING /
-- DATA_THORIM_HARDMODE / DATA_ILL_TAKE_YOU_ALL_ON) — no
-- GetData / achievement bridges; all join the
-- unmodeled-achievement queue.

local ENTRY_THORIM = 32865

local SAY_AGGRO_1 = 0
local SAY_SLAY = 4

-- C++ JustEngagedWith: BossAI passthrough + phase-1
-- ScheduleEvent legs + SPELL_SHEATH_OF_LIGHTNING +
-- colossus/lever legs — only the Talk arm is bridgeable.
local function thorimEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO_1)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function thorimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_THORIM, 1, thorimEnterCombat)
RegisterCreatureEvent(ENTRY_THORIM, 3, thorimTargetDied)
