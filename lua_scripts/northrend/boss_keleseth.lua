-- Prince Keleseth (Utgarde Keep) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_keleseth.cpp
-- (390 lines; 5 scripts — boss_keleseth (CreatureScript via
-- GetUtgardeKeepAI (BossAI), DATA_PRINCE_KELESETH = 0); npc_frost_tomb /
-- npc_vrykul_skeleton (CreatureScript via GetUtgardeKeepAI (ScriptedAI));
-- spell_frost_tomb (SpellScriptLoader, AuraScript OnRemove);
-- achievement_on_the_rocks (AchievementCriteriaScript, OnCheck GetData
-- DATA_ON_THE_ROCKS); all registered from inside
-- AddSC_boss_keleseth(); loader decl 126 / call 321 per
-- northrend_script_loader.cpp — the FIRST group of the "// Utgarde Keep
-- - Utgarde Keep" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_ulduar() (call 319); the call after it is
-- AddSC_boss_skarvald_dalronn() — verified from the loader this run;
-- the checkpoint sequence (instance_ulduar -> boss_keleseth) is
-- followed).
-- Entry: 23953 Prince Keleseth (utgarde_keep.h NPC_PRINCE_KELESETH, line
-- 46 — entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual). DATA_PRINCE_KELESETH = 0 (utgarde_keep.h
-- line 31).
-- Sole-source verified this run: whole-server-tree grep for each of the
-- five script names hits boss_keleseth.cpp (+ the loader decl/call lines
-- for AddSC_boss_keleseth) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. The lua was ported
-- Oct 2 09:55 in e526894 and re-audited this run (both arms C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_START_COMBAT 1) (event 1); the Talk fires
-- unconditionally in C++ (before the !who early return) and the guard
-- AttackStart leg (NPC_RUNEMAGE 23960 / NPC_STRATEGIST 23956 grid search,
-- LOS, AttackStart) has no bridges — the auriaya engage-port precedent.
-- JustDied — Talk(SAY_DEATH 5) (event 4 — the sjonnir JustDied-Talk
-- precedent); the BossAI::_JustDied() passthrough has no bridge.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 23953):
-- Timer Talks — Talk(SAY_SUMMON_SKELETONS 2) (EVENT_SUMMON_SKELETONS) and
-- Talk(SAY_FROST_TOMB 3) + Talk(SAY_FROST_TOMB_EMOTE 4, target)
-- (EVENT_FROST_TOMB) — the timer event machine has no bridge; both join
-- the no-timer-bridge queue (the emote's target leg has no targeted-Talk
-- bridge either).
-- npc_frost_tomb (NPC_FROSTTOMB 23965) — no Talk arms (IsSummonedBy
-- DoCast SPELL_FROST_TOMB; JustDied cross-creature
-- SetData(DATA_ON_THE_ROCKS, false) via instance DATA_PRINCE_KELESETH) —
-- no bridges; no registration.
-- npc_vrykul_skeleton (NPC_SKELETON 23970) — no Talk arms (DamageTaken
-- lethal rewrite + UNIT_FLAG_NOT_SELECTABLE / stand-state dead /
-- MoveIdle fake-death machine, EVENT_RESURRECT/FULL_HEAL/SHADOW_FISSURE
-- resurrection cycle) — no bridges; no registration.
-- spell_frost_tomb (AuraScript OnRemove: despawn-or-unsummon the tomb
-- caster when the stun aura is removed by non-death) — no AuraScript /
-- SpellScript bridge; joins the no-SpellScript / no-AuraScript-bridge
-- queues.
-- achievement_on_the_rocks (OnCheck: GetAI()->GetData(DATA_ON_THE_ROCKS))
-- — no GetData / achievement bridges; joins the unmodeled-achievement
-- queue.

local ENTRY_KELESETH = 23953

local SAY_START_COMBAT = 1
local SAY_DEATH = 5

-- C++ JustEngagedWith: BossAI::JustEngagedWith(who); Talk(SAY_START_COMBAT)
-- — unconditional (the guard AttackStart legs have no bridges) — the
-- auriaya engage-port precedent.
local function kelesethEnterCombat(event, creature, target)
    creature:Talk(SAY_START_COMBAT)
end

-- C++ JustDied: _JustDied(); Talk(SAY_DEATH) — the sjonnir JustDied-Talk
-- precedent.
local function kelesethDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_KELESETH, 1, kelesethEnterCombat)
RegisterCreatureEvent(ENTRY_KELESETH, 4, kelesethDied)
