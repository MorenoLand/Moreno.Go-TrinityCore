-- Bronjahm (Forge of Souls) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/boss_bronjahm.cpp
-- (411 lines incl. license; 8 scripts — boss_bronjahm (CreatureScript via
-- GetForgeOfSoulsAI (BossAI), DATA_BRONJAHM = 0); npc_corrupted_soul_fragment
-- (CreatureScript via GetForgeOfSoulsAI (ScriptedAI), zero Talk arms,
-- unbridged); spell_bronjahm_magic_bane / spell_bronjahm_consume_soul /
-- spell_bronjahm_soulstorm_targeting (SpellScriptLoaders — SpellScript);
-- spell_bronjahm_soulstorm_visual (AuraScript, registered twice: channel +
-- visual); achievement_bronjahm_soul_power (AchievementCriteriaScript
-- OnCheck AI()->GetData(DATA_SOUL_POWER) >= 4); all registered from inside
-- AddSC_boss_bronjahm(); loader decl 157 / call 352 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Forge of Souls" block in AddNorthrendScripts(), immediately after
-- AddSC_forge_of_souls() (call 351); the call after it is
-- AddSC_boss_devourer_of_souls() (call 353) — the checkpoint sequence
-- (forge_of_souls -> boss_bronjahm) is followed).
-- Entries: 36497 Bronjahm (forge_of_souls.h NPC_BRONJAHM, line 40) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. DATA_BRONJAHM = 0 (forge_of_souls.h
-- line 31). NPC_CORRUPTED_SOUL_FRAGMENT = 36535 (line 42, zero Talk —
-- not registered). DATA_SOUL_POWER = 1.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_bronjahm"
-- hits boss_bronjahm.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No bronjahm lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough leg and
-- the RemoveAurasDueToSpell(SPELL_SOULSTORM_CHANNEL) pre-fight-channel
-- removal have no bridge).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 36497):
-- Talk(SAY_CORRUPT_SOUL 4) and Talk(SAY_SOUL_STORM 3) ride the
-- scheduler-driven UpdateAI event legs (EVENT_CORRUPT_SOUL /
-- EVENT_SOULSTORM cases) — no-timer-bridge queue (the boss_toravon
-- precedent), so they do not port by construction.
-- The Reset schedule machine (SHADOW_BOLT 2s / MAGIC_BANE 8s-20s /
-- CORRUPT_SOUL 25s-35s phase-1), the EVENT_SOULSTORM one-shot chain
-- (SOULSTORM_VISUAL then SOULSTORM cast), the EVENT_FEAR 8s-12s
-- phase-2 leg, the DamageTaken 30%-hp PHASE_1->PHASE_2 flip (TELEPORT
-- cast + phase-2 event schedule), the JustSummoned soul-fragment
-- machine (REACT_PASSIVE + MoveFollow + PURPLE_BANISH_VISUAL) and the
-- DoMeleeAttackIfReady phase-1 leg all join the no-timer-bridge /
-- no-DoAction / no-SummonList bridge queues (the violet_hold trash
-- precedent); the ctor / JustReachedHome SPELL_SOULSTORM_CHANNEL
-- pre-fight casts have no bridge. GetData(DATA_SOUL_POWER) joins the
-- no-GetData-bridge queue (the boss_zuramat precedent).
-- npc_corrupted_soul_fragment (IsSummonedBy instance->GetGuidData
-- -> JustSummoned hand-off; MovementInform FOLLOW -> CONSUME_SOUL
-- cast + DespawnOrUnsummon) joins the no-MovementInform /
-- no-instance-GetGuidData bridge queues (the violet_hold saboteur
-- precedent). The four SpellScript/AuraScript hooks (magic_bane
-- mana-power damage recalc, consume_soul script-effect redirect,
-- soulstorm_visual 8-spell periodic ring, soulstorm_targeting 10yd
-- exclusion) join the no-SpellScript / no-AuraScript bridge queues
-- (the boss_moragg optic-link / spell_forge_of_souls_soul_sickness
-- precedent). achievement_bronjahm_soul_power (OnCheck
-- AI()->GetData(DATA_SOUL_POWER) >= 4) joins the unmodeled-achievement
-- / no-GetData-bridge queue (the cyanigosa / ichoron precedent).

local ENTRY_BRONJAHM = 36497

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_bronjahmAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional; the
-- RemoveAurasDueToSpell(SPELL_SOULSTORM_CHANNEL) pre-fight-channel
-- removal has no bridge — the auriaya engage-port precedent.
local function bronjahmEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function bronjahmKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function bronjahmJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BRONJAHM, 1, bronjahmEnterCombat)
RegisterCreatureEvent(ENTRY_BRONJAHM, 3, bronjahmKilledUnit)
RegisterCreatureEvent(ENTRY_BRONJAHM, 4, bronjahmJustDied)
