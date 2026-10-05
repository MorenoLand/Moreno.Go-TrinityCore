-- Rotface (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_rotface.cpp
-- (813 lines incl. license; 4 CreatureScripts
-- (boss_rotface (BossAI, DATA_ROTFACE = 5),
-- npc_little_ooze (ScriptedAI), npc_big_ooze (ScriptedAI),
-- npc_precious_icc (ScriptedAI)) + 10 SpellScripts
-- (spell_rotface_ooze_flood, spell_rotface_mutated_infection,
-- spell_rotface_little_ooze_combine, spell_rotface_large_ooze_combine,
-- spell_rotface_large_ooze_buff_combine,
-- spell_rotface_unstable_ooze_explosion_init,
-- spell_rotface_unstable_ooze_explosion,
-- spell_rotface_unstable_ooze_explosion_suicide,
-- spell_rotface_vile_gas_trigger, spell_rotface_slime_spray)
-- + 1 AuraScript (spell_rotface_mutated_infection_aura); all
-- registered from inside AddSC_boss_rotface(); loader decl 176 /
-- call 371 per northrend_script_loader.cpp — the SIXTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_festergut() (call 370); the call
-- after it is AddSC_boss_professor_putricide() (call 372) —
-- verified from the loader this run; the checkpoint sequence
-- (boss_festergut -> boss_rotface) is followed).
-- Entry: 36627 Rotface (icecrown_citadel.h NPC_ROTFACE, line
-- 227; icecrown_citadel.h DATA_ROTFACE = 5, line 79;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_ROTFACE, line 241) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_rotface" hits boss_rotface.cpp (+ the loader
-- decl/call lines) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No rotface lua
-- existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36627):
-- boss_rotface UpdateAI — Talk(EMOTE_SLIME_SPRAY 2) rides the
-- EVENT_SLIME_SPRAY scheduler leg (no-timer-bridge; the
-- boss_toravon precedent); the Slime Spray / Mutated Infection /
-- Vile Gas DoCast legs have no-cast bridge.
-- boss_rotface SpellHitTarget — Talk(SAY_SLIME_SPRAY 3) rides the
-- SPELL_SLIME_SPRAY case of the SpellHitTarget switch —
-- no-SpellHit-bridge queue (the SpellHit-15-never-fires queue
-- family).
-- boss_rotface JustEngagedWith instance legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- DoZoneInCombat (no instance bridge) and
-- professor->AI()->DoAction(ACTION_ROTFACE_COMBAT) (no-DoAction
-- bridge; the krick ACTION_OUTRO precedent); JustDied legs
-- DoRemoveAurasDueToSpellOnPlayers(MUTATED_INFECTION) and
-- professor->AI()->DoAction(ACTION_ROTFACE_DEATH) — no instance /
-- DoAction bridge; JustReachedHome
-- instance->SetBossState(DATA_ROTFACE, FAIL) + SetData(
-- DATA_OOZE_DANCE_ACHIEVEMENT) — no instance bridge; EnterEvadeMode
-- cross-AI professor EnterEvadeMode — no bridge; JustSummoned
-- cross-AI professor CastSpell leg (VILE_GAS_H) — no bridge.
-- npc_precious_icc::JustDied —
-- rotface->AI()->Talk(SAY_PRECIOUS_DIES 0) is cross-AI Talk with no
-- bridge (the festergut stinky precedent); Talk(
-- EMOTE_PRECIOUS_ZOMBIES 0) rides the EVENT_SUMMON_ZOMBIES
-- scheduler leg (no-timer-bridge). npc_precious_icc has zero
-- bridgeable arms — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent). Its entry constant is
-- DB-side/script-name bound (no NPC_PRECIOUS constant in the ICC
-- script tree).
-- npc_little_ooze / npc_big_ooze — zero Talk lines — NOT
-- registered (the bronjahm npc_corrupted_soul_fragment precedent);
-- the combine / sticky-ooze DoCast and JustSummoned / JustDied
-- instance legs have no bridge.
-- The 10 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent): spell_rotface_mutated_infection
-- Talk(EMOTE_MUTATED_INFECTION 9) is cross-AI Talk riding a
-- SpellScript (NotifyTargets); spell_rotface_large_ooze_buff_combine
-- Talk(EMOTE_UNSTABLE_2..4 0..2) + Talk(EMOTE_UNSTABLE_EXPLOSION 3)
-- + rotface->AI()->Talk(SAY_UNSTABLE_EXPLOSION 5) (:597-:618) and
-- spell_rotface_large_ooze_combine rotface->AI()->Talk(
-- SAY_UNSTABLE_EXPLOSION 5) (:557) are caster / cross-AI Talk riding
-- SpellScripts; spell_rotface_unstable_ooze_explosion_init carries
-- zero Talk calls; the remaining 7 join the
-- same queue on their caster / targeting legs.
-- spell_rotface_mutated_infection_aura joins the no-AuraScript-bridge
-- queue.
-- All 14 Talk() calls in the file accounted for (14 =
-- SAY_PRECIOUS_DIES 0 (1, cross-AI from npc_precious_icc JustDied)
-- + SAY_AGGRO 1 (1) + EMOTE_SLIME_SPRAY 2 (1, scheduler) +
-- SAY_SLIME_SPRAY 3 (1, SpellHitTarget) + SAY_UNSTABLE_EXPLOSION 5
-- (2, cross-AI from the large_ooze_combine + large_ooze_buff_combine
-- SpellScripts) + EMOTE_PRECIOUS_ZOMBIES 0 (1, scheduler) +
-- SAY_KILL 6 (1) + SAY_DEATH 8 (1) +
-- EMOTE_MUTATED_INFECTION 9 (1, SpellScript) +
-- EMOTE_UNSTABLE_2..4 / EMOTE_UNSTABLE_EXPLOSION 0..3 (4,
-- SpellScript); SAY_BERSERK 7 is declared in the C++ Texts enum
-- but never used in a Talk() call).

local ENTRY_ROTFACE = 36627

local SAY_AGGRO = 1
local SAY_KILL = 6
local SAY_DEATH = 8

-- C++ boss_rotface::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function rotfaceJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_rotface::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function rotfaceKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_rotface::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function rotfaceJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ROTFACE, 1, rotfaceJustEngagedWith)
RegisterCreatureEvent(ENTRY_ROTFACE, 3, rotfaceKilledUnit)
RegisterCreatureEvent(ENTRY_ROTFACE, 4, rotfaceJustDied)
