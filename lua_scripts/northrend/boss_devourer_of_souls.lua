-- Devourer of Souls (Forge of Souls) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/boss_devourer_of_souls.cpp
-- (500 lines incl. license; 5 scripts — boss_devourer_of_souls
-- (CreatureScript via GetForgeOfSoulsAI (BossAI), DATA_DEVOURER_OF_SOULS =
-- 1); spell_devourer_of_souls_mirrored_soul (SpellScript OnEffectHitTarget
-- -> CastSpell MIRRORED_SOUL_PROC_AURA on hit unit);
-- spell_devourer_of_souls_mirrored_soul_proc (AuraScript DoCheckProc caster
-- alive + OnEffectProc 45%-of-damage MIRRORED_SOUL_DAMAGE redirect);
-- spell_devourer_of_souls_mirrored_soul_target_selector (SpellScript
-- TARGET_UNIT_SRC_AREA_ENTRY random-single FilterTargets + OnEffectHitTarget
-- MIRRORED_SOUL_BUFF cast); achievement_three_faced (AchievementCriteriaScript
-- OnCheck target->ToCreature AI()->GetData(DATA_THREE_FACED)); all
-- registered from inside AddSC_boss_devourer_of_souls(); loader decl 158 /
-- call 353 per northrend_script_loader.cpp — the FOURTH and LAST group of
-- the "// Forge of Souls" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_bronjahm() (call 352); no call follows it in the Forge of
-- Souls block — the checkpoint sequence (boss_bronjahm ->
-- boss_devourer_of_souls) is followed).
-- Entries: 36502 Devourer of Souls (forge_of_souls.h NPC_DEVOURER, line 41)
-- — entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. DATA_DEVOURER_OF_SOULS = 1
-- (forge_of_souls.h line 29). DATA_THREE_FACED = 1 (boss_devourer_of_souls.cpp
-- line 107).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_devourer_of_souls" hits boss_devourer_of_souls.cpp only (+
-- the loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No devourer lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDeath. (No OnKill
-- registration: the KilledUnit Talk is display-ID gated with no bridge —
-- see below.)
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_FACE_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough leg, the
-- NPC_CRUCIBLE_OF_SOULS 60yd double-spawn guard SummonCreature leg and the
-- five events.ScheduleEvent legs have no bridge).
-- JustDied — Talk(SAY_FACE_DEATH 4) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg and the
-- instance->GetData(DATA_TEAM_IN_INSTANCE) outro-summon choreography have
-- no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 36502):
-- KilledUnit Talk(textId) is player-gated BUT text-selected by
-- me->GetDisplayId() (ANGER 30148 / SORROW 30149 / DESIRE 30150 ->
-- SAY_FACE_ANGER_SLAY 1 / SAY_FACE_SORROW_SLAY 2 / SAY_FACE_DESIRE_SLAY 3)
-- — no display-ID bridge exists, so it does not port (the razuvious
-- player-gated variant precedent covers only unconditional Talk).
-- The JustDied outro loop (22 outroPositions team-swapped
-- instance->instance->SummonCreature spawns with MovePoint legs) and the
-- summon->AI()->Talk(SAY_JAINA_OUTRO / SAY_SYLVANAS_OUTRO 0) lines are
-- instance-driven Talk on other creatures' AI — no bridge
-- (the instance_violet_hold ScheduleCyanigosaIntro precedent).
-- Talk(EMOTE_MIRRORED_SOUL 5), Talk(SAY_FACE_UNLEASH_SOUL 7) +
-- Talk(EMOTE_UNLEASH_SOUL 6) and Talk(SAY_FACE_WAILING_SOUL 9) +
-- Talk(EMOTE_WAILING_SOUL 8) all ride scheduler-driven UpdateAI event
-- legs (EVENT_MIRRORED_SOUL / EVENT_UNLEASHED_SOULS / EVENT_WAILING_SOULS)
-- — no-timer-bridge queue (the boss_toravon precedent). The whole
-- scheduler machine (PHANTOM_BLAST 5s victim / MIRRORED_SOUL 15s-30s
-- AOE selector / WELL_OF_SOULS 20s random / UNLEASHED_SOULS 30s random +
-- SORROW-face flip + 5s FACE_ANGER restore / WAILING_SOULS 60s-70s DESIRE
-- phase: beamAngle PI/30-per-tick sweep, UNIT_STATE_ROOT + REACT_PASSIVE,
-- 15 TICK casts, MoveChase restore; the HasUnitState(UNIT_STATE_CASTING)
-- gates; DoMeleeAttackIfReady) and the Reset display/control/react-state
-- choreography (DISPLAY_ANGER + SetControlled(false, UNIT_STATE_ROOT) +
-- REACT_AGGRESSIVE) join the no-timer-bridge queue (the boss_toravon
-- precedent). SpellHitTarget H_SPELL_PHANTOM_BLAST -> threeFaced = false
-- and GetData(DATA_THREE_FACED) join the no-SpellHit-bridge /
-- no-GetData-bridge queues (the boss_zuramat precedent). The three
-- spell_devourer_of_souls_mirrored_soul* SpellScript/AuraScript hooks join
-- the no-SpellScript / no-AuraScript bridge queues (the boss_moragg
-- optic-link precedent). achievement_three_faced (OnCheck
-- target->ToCreature AI()->GetData(DATA_THREE_FACED)) joins the
-- unmodeled-achievement / no-GetData-bridge queue (the cyanigosa / ichoron
-- precedent). The file carries a @todo (model id 36503/36504 during
-- unleash soul; outro npc movement).

local ENTRY_DEVOURER = 36502

local SAY_FACE_AGGRO = 0
local SAY_FACE_DEATH = 4

-- C++ boss_devourer_of_soulsAI::JustEngagedWith:
-- BossAI::JustEngagedWith(who) (passthrough, no bridge);
-- Talk(SAY_FACE_AGGRO) — unconditional; the NPC_CRUCIBLE_OF_SOULS
-- double-spawn-guard SummonCreature leg and the five ScheduleEvent legs
-- (PHANTOM_BLAST 5s / MIRRORED_SOUL 8s / WELL_OF_SOULS 30s /
-- UNLEASHED_SOULS 20s / WAILING_SOULS 60s-70s) have no bridge — the
-- auriaya engage-port precedent.
local function devourerEnterCombat(event, creature, target)
    creature:Talk(SAY_FACE_AGGRO)
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_FACE_DEATH) — unconditional — the sjonnir JustDied-Talk
-- precedent; the DATA_TEAM_IN_INSTANCE outro-summon loop and the
-- summon->AI()->Talk(SAY_JAINA_OUTRO / SAY_SYLVANAS_OUTRO) legs are
-- instance-driven Talk on other creatures with no bridge.
local function devourerJustDied(event, creature)
    creature:Talk(SAY_FACE_DEATH)
end

RegisterCreatureEvent(ENTRY_DEVOURER, 1, devourerEnterCombat)
RegisterCreatureEvent(ENTRY_DEVOURER, 4, devourerJustDied)
