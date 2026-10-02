-- Blood Prince Council (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_prince_council.cpp
-- (1369 lines incl. license; 8 CreatureScripts
-- (boss_blood_council_controller (BossAI);
-- boss_prince_keleseth_icc / boss_prince_taldaram_icc /
-- boss_prince_valanar_icc (BloodPrincesBossAI : BossAI);
-- npc_blood_queen_lana_thel (PassiveAI); npc_ball_of_flame
-- (ScriptedAI); npc_kinetic_bomb (ScriptedAI); npc_dark_nucleus
-- (ScriptedAI)) + 6 SpellScripts (spell_taldaram_glittering_sparks,
-- spell_taldaram_summon_flame_ball,
-- spell_taldaram_ball_of_inferno_flame, spell_valanar_kinetic_bomb,
-- spell_valanar_kinetic_bomb_knockback,
-- spell_blood_council_shadow_prison_damage) + 5 AuraScripts
-- (spell_taldaram_flame_ball_visual,
-- spell_taldaram_ball_of_inferno_flame_aura,
-- spell_valanar_kinetic_bomb_aura, spell_valanar_kinetic_bomb_absorb,
-- spell_blood_council_shadow_prison) + 1 AreaTriggerScript
-- (at_blood_prince_council_start_intro); all registered from inside
-- AddSC_boss_blood_prince_council(); loader decl 178 / call 373 per
-- northrend_script_loader.cpp — the EIGHTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_professor_putricide() (call 372); the call after
-- it is AddSC_boss_blood_queen_lana_thel() (call 374) — verified
-- from the loader this run. NOTE: the 09:34 run's checkpoint entry
-- recorded call 376 (AddSC_boss_valithria_dreamwalker()) as the call
-- after putricide, skipping loader calls 373–375
-- (blood_prince_council, blood_queen_lana_thel, sister_svalna); the
-- loader's actual call order governs, so this run ports the
-- blood-prince-council group first.
-- Entries: 37972 Prince Keleseth (icecrown_citadel.h
-- NPC_PRINCE_KELESETH, line 245), 37973 Prince Taldaram
-- (NPC_PRINCE_TALDARAM, line 246), 37970 Prince Valanar
-- (NPC_PRINCE_VALANAR, line 247);
-- instance_icecrown_citadel.cpp OnCreatureCreate binds all three
-- (lines 255–261) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_prince_council" hits
-- boss_blood_prince_council.cpp (+ the loader decl/call lines) only;
-- this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction. No blood-prince-council lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entries
-- 37972/37973/37970):
-- BloodPrincesBossAI DoAction — Talk(SelectInvocationSay()) rides
-- ACTION_CAST_INVOCATION — no-DoAction bridge (the krick
-- ACTION_OUTRO precedent); the invocation invocation-order
-- Talk(textId) fired by boss_blood_council_controller UpdateAI
-- EVENT_INVOCATION_OF_BLOOD on the empowered prince is cross-AI
-- Talk on a scheduler leg — no bridge.
-- boss_prince_keleseth_icc / boss_prince_taldaram_icc /
-- boss_prince_valanar_icc UpdateAI — Talk(EMOTE_KELESETH_BERSERK /
-- SAY_KELESETH_SPECIAL), Talk(EMOTE_TALDARAM_FLAME /
-- EMOTE_TALDARAM_BERSERK / SAY_TALDARAM_SPECIAL),
-- Talk(SAY_VALANAR_BERSERK / SAY_VALANAR_SPECIAL /
-- EMOTE_VALANAR_SHOCK_VORTEX) all ride scheduler legs
-- (no-timer-bridge; the boss_toravon precedent); the shadow-lance /
-- conjure-flame / shock-vortex / kinetic-bomb DoCast legs have
-- no-cast bridge.
-- boss_blood_council_controller — zero own Talk lines — NOT
-- registered (the bronjahm npc_corrupted_soul_fragment precedent);
-- its JustEngagedWith instance legs (CheckRequiredBosses /
-- DoCastSpellOnPlayers / DoZoneInCombat / SetBossState) have no
-- instance bridge; the invocation rotation machine has no bridge.
-- npc_blood_queen_lana_thel — Talk(SAY_INTRO_1 0) rides the
-- ACTION_START_INTRO DoAction leg (no-DoAction bridge);
-- Talk(SAY_INTRO_2 1) rides the EVENT_INTRO_1 scheduler leg
-- (no-timer-bridge) — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- npc_ball_of_flame / npc_kinetic_bomb / npc_dark_nucleus — zero
-- Talk lines — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent); their explode /
-- despawn legs have no bridge.
-- The 6 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent), the 5 aura scripts the
-- no-AuraScript-bridge queue, and at_blood_prince_council_start_intro
-- (OnTrigger drives the controller's ACTION_START_INTRO) the
-- no-AreaTrigger bridge queue (the boss_anubrekhan
-- at_anubrekhan_entrance precedent).
-- Talk accounting for the base BloodPrincesBossAI arms (shared by
-- all three princes): JustDied Talk(SAY_KELESETH_DEATH 5 /
-- EMOTE_TALDARAM_DEATH 6 / SAY_VALANAR_DEATH 6) entry-switched
-- (3 calls); KilledUnit Talk(SAY_KELESETH_KILL 3 / SAY_TALDARAM_KILL
-- 4 / SAY_VALANAR_KILL 4) victim->GetTypeId() == TYPEID_PLAYER
-- gated, entry-switched (3 calls).

local ENTRY_KELESETH = 37972
local ENTRY_TALDARAM = 37973
local ENTRY_VALANAR = 37970

-- entry-switched kill texts (C++ BloodPrincesBossAI::KilledUnit):
-- keleseth SAY_KELESETH_KILL = 3, taldaram SAY_TALDARAM_KILL = 4,
-- valanar SAY_VALANAR_KILL = 4
local KILL_TEXT = {
    [ENTRY_KELESETH] = 3,
    [ENTRY_TALDARAM] = 4,
    [ENTRY_VALANAR] = 4,
}

-- entry-switched death texts (C++ BloodPrincesBossAI::JustDied):
-- keleseth SAY_KELESETH_DEATH = 5, taldaram EMOTE_TALDARAM_DEATH = 6,
-- valanar SAY_VALANAR_DEATH = 6
local DEATH_TEXT = {
    [ENTRY_KELESETH] = 5,
    [ENTRY_TALDARAM] = 6,
    [ENTRY_VALANAR] = 6,
}

local PRINCES = { ENTRY_KELESETH, ENTRY_TALDARAM, ENTRY_VALANAR }

-- C++ BloodPrincesBossAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) switch (me->GetEntry())
-- Talk(kill text) — the razuvious player-gated variant precedent.
local function princesKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(KILL_TEXT[creature:GetEntry()])
    end
end

-- C++ BloodPrincesBossAI::JustDied:
-- switch (me->GetEntry()) Talk(death text) — the sjonnir
-- JustDied-Talk precedent.
local function princesJustDied(event, creature, killer)
    creature:Talk(DEATH_TEXT[creature:GetEntry()])
end

for _, entry in ipairs(PRINCES) do
    RegisterCreatureEvent(entry, 3, princesKilledUnit)
    RegisterCreatureEvent(entry, 4, princesJustDied)
end
