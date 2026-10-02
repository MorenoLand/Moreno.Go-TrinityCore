-- Festergut (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_festergut.cpp
-- (476 lines incl. license; 2 CreatureScripts
-- (boss_festergut (BossAI, DATA_FESTERGUT = 4),
-- npc_stinky_icc (ScriptedAI, Stinky)) + 2 SpellScripts
-- (spell_festergut_pungent_blight, spell_festergut_gastric_bloat)
-- + 1 AuraScript (spell_festergut_blighted_spores) + 1
-- AchievementCriteriaScript (achievement_flu_shot_shortage); all
-- registered from inside AddSC_boss_festergut(); loader decl 175 /
-- call 370 per northrend_script_loader.cpp — the FIFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_deathbringer_saurfang() (call 369); the call
-- after it is AddSC_boss_rotface() (call 371) — verified from the
-- loader this run; the checkpoint sequence
-- (boss_deathbringer_saurfang -> boss_festergut) is followed).
-- Entry: 36626 Festergut (icecrown_citadel.h NPC_FESTERGUT, line
-- 222; icecrown_citadel.h DATA_FESTERGUT = 4, line 78;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_FESTERGUT, line 238) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_festergut" hits boss_festergut.cpp (+ the loader
-- decl/call lines) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No festergut lua
-- existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36626):
-- boss_festergut UpdateAI — Talk(SAY_PUNGENT_BLIGHT 4) /
-- Talk(EMOTE_WARN_PUNGENT_BLIGHT 5) ride the EVENT_INHALE_BLIGHT
-- scheduler leg, Talk(EMOTE_GAS_SPORE 2) /
-- Talk(EMOTE_WARN_GAS_SPORE 3) ride the EVENT_GAS_SPORE leg,
-- Talk(SAY_BERSERK 8) rides the EVENT_BERSERK leg — timer-driven
-- (no-timer-bridge; the boss_toravon precedent); the inhale
-- counter / gaseous-blight aura-manipulation legs have no bridge.
-- npc_stinky_icc::JustDied — festergut->AI()->Talk(SAY_STINKY_DEAD
-- 0) is cross-AI Talk with no bridge, and npc_stinky_icc itself has
-- zero own Talk arms — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- spell_festergut_pungent_blight::HandleScriptEffect —
-- festergut->AI()->Talk(EMOTE_PUNGENT_BLIGHT 6) is cross-AI Talk
-- riding a SpellScript — no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent);
-- spell_festergut_gastric_bloat joins the same queue.
-- spell_festergut_blighted_spores (AuraScript, cross-AI
-- festergut->AI()->SetData(DATA_INOCULATED_STACK) leg) joins the
-- no-AuraScript-bridge queue. achievement_flu_shot_shortage joins
-- the no-achievement bridge queue (the gunship
-- achievement_im_on_a_boat precedent).
-- boss_festergut JustEngagedWith legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- FindNearestCreature(NPC_GAS_DUMMY) / DoZoneInCombat /
-- professor->AI()->DoAction(ACTION_FESTERGUT_COMBAT) — have no
-- instance / DoAction bridge; JustDied _JustDied + DoAction(
-- ACTION_FESTERGUT_DEATH) + RemoveBlight legs, JustReachedHome
-- instance->SetBossState(FAIL), EnterEvadeMode cross-AI professor
-- EnterEvadeMode — no instance / DoAction bridge. SpellHitTarget
-- is Talk-free (PUNGENT_BLIGHT_HELPER aura-removal leg only).
-- All 10 Talk() calls in the file accounted for (10 =
-- SAY_STINKY_DEAD 0 (1, cross-AI from npc_stinky_icc) + SAY_AGGRO 1
-- (1) + EMOTE_GAS_SPORE 2 (1) + EMOTE_WARN_GAS_SPORE 3 (1) +
-- SAY_PUNGENT_BLIGHT 4 (1) + EMOTE_WARN_PUNGENT_BLIGHT 5 (1) +
-- EMOTE_PUNGENT_BLIGHT 6 (1, from the SpellScript) + SAY_KILL 7
-- (1) + SAY_BERSERK 8 (1) + SAY_DEATH 9 (1)).

local ENTRY_FESTERGUT = 36626

local SAY_AGGRO = 1
local SAY_KILL = 7
local SAY_DEATH = 9

-- C++ boss_festergut::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function festergutJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_festergut::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function festergutKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_festergut::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function festergutJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_FESTERGUT, 1, festergutJustEngagedWith)
RegisterCreatureEvent(ENTRY_FESTERGUT, 3, festergutKilledUnit)
RegisterCreatureEvent(ENTRY_FESTERGUT, 4, festergutJustDied)
