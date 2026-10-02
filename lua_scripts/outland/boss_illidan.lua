-- Illidan Stormrage (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_illidan.cpp
-- (11 CreatureScripts — boss_illidan_stormrage (BossAI, DATA_ILLIDAN_STORMRAGE)
-- + npc_akama_illidan (ScriptedAI) + npc_parasitic_shadowfiend (ScriptedAI)
-- + npc_blade_of_azzinoth (NullCreatureAI) + npc_flame_of_azzinoth
-- (ScriptedAI) + npc_illidan_db_target (NullCreatureAI) + npc_maiev
-- (ScriptedAI) + npc_shadow_demon (PassiveAI) + npc_cage_trap_trigger
-- (PassiveAI) + npc_illidari_elite (ScriptedAI) +
-- npc_illidan_generic_fire (ScriptedAI); 21 SpellScriptLoaders;
-- all registered from inside AddSC_boss_illidan() — loader decl 45 /
-- call 168 per outland_script_loader.cpp — the SECOND group of the
-- "// Black Temple" block in AddOutlandScripts(), immediately after
-- AddSC_black_temple() (call 167) — verified from the loader this run;
-- the checkpoint sequence (black_temple -> boss_illidan) is followed).
-- Entry: 22917 NPC_ILLIDAN_STORMRAGE — from black_temple.h's Creatures
-- enum (line 79). Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_illidan\b" hits boss_illidan.cpp (+ the loader decl/call
-- lines) only.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_illidan_stormrageAI::KilledUnit — Talk(SAY_ILLIDAN_KILL 1)
-- (line 520) gated on victim->GetTypeId() == TYPEID_PLAYER — the
-- razuvious player-gated precedent (event 3).
-- DOCUMENTED-ONLY (in this header; only entry 22917 registered):
-- boss_illidan_stormrageAI::Reset — _Reset() + SummonCreatureGroup(1) +
-- LoadEquipment(1) + sheath ROOT legs + instance akama intro leg —
-- no-instance / no-equipment bridges — documented-only.
-- boss_illidan_stormrageAI::JustEngagedWith — no Talk: dual-wield +
-- music + EVENT_EVADE_CHECK/EVENT_BERSERK/EVENT_TAUNT schedules + phase-1
-- machine — no-timer / no-music bridges — documented-only.
-- boss_illidan_stormrageAI::JustDied — no Talk: RemoveFlag +
-- SetBossState(DONE) — no-instance bridge — documented-only.
-- boss_illidan_stormrageAI::DamageTaken — health-lock intro (ACTION_START_OUTRO,
-- maiev ACTION_START_OUTRO) + 90/65/30% phase transitions (ACTION_START_MINIONS,
-- ACTION_START_PHASE_2, ACTION_START_PHASE_4 + demon-form cancel path) —
-- no-health / no-action / no-instance bridges — documented-only.
-- boss_illidan_stormrageAI::DoAction — ACTION_START_ENCOUNTER (intro events +
-- akama ACTION_FREE) / ACTION_INTRO_DONE / ACTION_START_MINIONS Talk
-- (SAY_ILLIDAN_MINION 0) (rides no-action — DoAction fire sites are not
-- modeled) / ACTION_START_MINIONS_WEAVE / ACTION_START_PHASE_2 /
-- ACTION_FLAME_DEAD / ACTION_FINALIZE_AIR_PHASE / ACTION_START_PHASE_4 /
-- ACTION_ILLIDAN_CAGED / ACTION_START_OUTRO — no-action / no-instance /
-- no-motion / no-cast bridges — documented-only.
-- boss_illidan_stormrageAI::UpdateAI + ExecuteSpecialEvents — the whole timer
-- machine (EVENT_START_INTRO/UNCONVINCED/PREPARED intro Talks 9/10/8,
-- EVENT_FLAME_CRASH/DRAW_SOUL/SHEAR/PARASITIC_SHADOWFIEND/MINIONS_WEAVE,
-- air-phase legs EVENT_FLY/THROW_WARGLAIVE x2/FACE_MIDDLE/
-- FLY_TO_RANDOM_PILLAR/FIREBALL/EYE_BLAST (Talk SAY_ILLIDAN_EYE_BLAST 4)/
-- DARK_BARRAGE/GLAIVE_EMOTE/RESUME_COMBAT, EVENT_AGONIZING_FLAMES,
-- EVENT_DEMON (Talk SAY_ILLIDAN_MORPH 5)/EVENT_DEMON_TEXT/
-- EVENT_SCHEDULE_DEMON_SPELLS/CANCEL_DEMON_FORM/RESUME_COMBAT_DEMON,
-- demon-form legs EVENT_SHADOW_BLAST/FLAME_BURST/SHADOW_DEMON,
-- phase-4 legs EVENT_SHADOW_PRISON_TEXT (Talk SAY_ILLIDAN_SHADOW_PRISON 11)/
-- EVENT_SUMMON_MAIEV/EVENT_CONFRONT_MAIEV_TEXT (Talk
-- SAY_ILLIDAN_CONFRONT_MAIEV 12)/EVENT_RESUME_COMBAT_PHASE_4/EVENT_FRENZY
-- (Talk SAY_ILLIDAN_FRENZY 13)/EVENT_TAUNT (Talk SAY_ILLIDAN_TAUNT 7),
-- outro legs EVENT_DEFEATED_TEXT (Talk SAY_ILLIDAN_DEFEATED 14)/
-- EVENT_QUIET_SUICIDE, specialEvents EVENT_BERSERK (Talk SAY_ILLIDAN_ENRAGE 6)/
-- EVENT_EVADE_CHECK) — no-timer / no-cast / no-summon / no-motion /
-- no-random-target bridges — documented-only.
-- boss_illidan_stormrageAI::MovementInform — POINT_THROW_GLAIVE /
-- POINT_RANDOM_PILLAR / POINT_ILLIDAN_MIDDLE (glaive-returns leg) —
-- no-motion / no-cast bridges — documented-only.
-- npc_akama_illidan — gossip handlers (GOSSIP_START_INTRO/FIGHT),
-- MovementInform Talks (SAY_AKAMA_FINISH 8 / SAY_AKAMA_BETRAYER 3), the
-- timer machine (EVENT_AKAMA_SAY_DOOR 0 / SAY_ALONE 1 / spirit ALONE Talks /
-- SAY_SALUTE 2 / SAY_FREE 4 / SAY_TIME_HAS_COME 5 / SAY_MINIONS 6 /
-- SAY_LIGHT 7), DamageTaken health-lock, ACTION_OPEN_DOOR/FREE/
-- START_ENCOUNTER/START_MINIONS/START_OUTRO legs — no-gossip /
-- no-motion / no-timer / no-instance / no-cast / no-action bridges —
-- documented-only (NOT registered — boss companion-AI precedent).
-- npc_parasitic_shadowfiend / npc_blade_of_azzinoth / npc_flame_of_azzinoth
-- (JustDied ACTION_FLAME_DEAD leg) / npc_illidan_db_target /
-- npc_shadow_demon / npc_cage_trap_trigger / npc_illidari_elite /
-- npc_illidan_generic_fire — scheduler-driven add machines, all zero-Talk
-- — no-timer / no-cast / no-instance / no-motion bridges — documented-only
-- (NOT registered — boss companion-AI precedent).
-- npc_maiev — zero bridgeable arms: DamageTaken Talk(SAY_MAIEV_SHADOWSONG_DOWN
-- 4, me) is damage-gated, not one of the four bridgeable Talk sites;
-- DoAction ACTION_START_OUTRO Talk(SAY_MAIEV_SHADOWSONG_FINISHED 5) rides
-- no-action; JustAppeared/UpdateAI timer machine (APPEAR 1 / JUSTICE 2 /
-- TRAP 3 / TAUNT 0 / OUTRO 6 / FAREWELL 7) rides no-timer — documented-only
-- (NOT registered).
-- 21 SpellScriptLoaders (spell_illidan_akama_teleport / akama_door_channel /
-- draw_soul / parasitic_shadowfiend / parasitic_shadowfiend_proc /
-- remove_parasitic_shadowfiend / throw_warglaive / tear_of_azzinoth_channel /
-- flame_blast / return_glaives / agonizing_flames / demon_transform1 /
-- demon_transform2 / flame_burst / find_target / eye_blast / cage_trap /
-- caged / maiev_down / cage_teleport / despawn_akama) — no SpellScript /
-- no AuraScript bridges (instance_ulduar precedent) — documented-only.
-- All Talk() calls in the file accounted for (32 total): 1 ported above;
-- SAY_ILLIDAN_MINION 0 (DoAction), SAY_ILLIDAN_ENRAGE 6 / DUPLICITY 8 /
-- UNCONVINCED 9 / PREPARED 10 / EYE_BLAST 4 / MORPH 5 / SHADOW_PRISON 11 /
-- CONFRONT_MAIEV 12 / FRENZY 13 / TAUNT 7 / DEFEATED 14 (timer machine),
-- SAY_AKAMA_* 0-8 (timer machine / MovementInform / gossip legs),
-- SAY_SPIRIT_ALONE 0 x2 (timer machine), SAY_MAIEV_SHADOWSONG_* 0-7
-- (timer machine / DamageTaken / DoAction) — documented-only above.

local ENTRY_ILLIDAN_STORMRAGE = 22917 -- NPC_ILLIDAN_STORMRAGE (black_temple.h)

local SAY_ILLIDAN_KILL = 1

-- C++ boss_illidan_stormrageAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_ILLIDAN_KILL) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:IsPlayer() convention).
local function illidanTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_ILLIDAN_KILL)
    end
end

RegisterCreatureEvent(ENTRY_ILLIDAN_STORMRAGE, 3, illidanTargetDied)
