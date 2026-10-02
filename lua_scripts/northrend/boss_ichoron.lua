-- Ichoron (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_ichoron.cpp
-- (536 lines incl. license; 7 scripts — boss_ichoron (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_ICHORON = 5); npc_ichor_globule
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), zero Talk arms);
-- spell_ichoron_drained / spell_ichoron_protective_bubble /
-- spell_ichoron_splatter (SpellScriptLoader AuraScripts) /
-- spell_ichoron_merge (SpellScriptLoader SpellScript);
-- achievement_dehydration (AchievementCriteriaScript, OnCheck
-- GetData(DATA_DEHYDRATION)); all registered from inside
-- AddSC_boss_ichoron(); loader decl 147 / call 342 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_erekem() (call 341); the call after it is
-- AddSC_boss_lavanthor() (call 343) — the checkpoint sequence
-- (boss_erekem -> boss_ichoron) is followed).
-- Entries: 29313 Ichoron (violet_hold.h NPC_ICHORON, line 98) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_ICHORON = 5
-- (violet_hold.h line 55).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ichoron"
-- hits boss_ichoron.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No ichoron lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough leg
-- has no bridge).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 29313):
-- SAY_SPAWN (3) is never Talk()ed anywhere in the file (dead enum text,
-- the cyanigosa precedent; grep -c "Talk(" = 7 total, the 3 ported
-- above plus the 4 below).
-- Talk(SAY_SHATTER 5) + Talk(EMOTE_SHATTER 7) ride the DoAction
-- ACTION_PROTECTIVE_BUBBLE_SHATTERED leg (aura shattered -> AOE
-- splatter/burst, DRAINED cast, 30% max-health damage leg,
-- scheduler.DelayAll) — no DoAction bridge; joins the no-DoAction-
-- bridge queue. Talk(SAY_BUBBLE 6) rides the DoAction ACTION_DRAINED
-- leg (HealthAbovePct(30) -> protective bubble recast) — same queue.
-- Talk(SAY_ENRAGE 4) rides the UpdateAI health-threshold leg
-- (HealthBelowPct(25) + no DRAINED aura -> FRENZY) — no
-- health-threshold bridge; joins the bridge queue.
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS, DATA_ICHORON)
-- leg, the Reset SPELL_THREAT_PROC self-cast, the ScheduleTasks machine
-- (SHRINK + PROTECTIVE_BUBBLE async leg, WATER_BOLT_VOLLEY 10-15s AOE,
-- WATER_BLAST 6-9s random target), the ACTION_WATER_GLOBULE_HIT
-- DoAction leg (3% max-health heal, _dehydration = false), the
-- GetData(DATA_DEHYDRATION) hook and the npc_ichor_globule machine
-- (SpellHit transform -> MoveFollow, MovementInform FOLLOW_MOTION_TYPE
-- MERGE + DespawnOrUnsummon, DamageTaken splash leg) have no bridges;
-- the spell_ichoron_drained / spell_ichoron_merge /
-- spell_ichoron_protective_bubble / spell_ichoron_splatter
-- SpellScript/AuraScript hooks join the no-SpellScript / no-AuraScript-
-- bridge queues; achievement_dehydration (OnCheck
-- AI()->GetData(DATA_DEHYDRATION)) joins the unmodeled-achievement /
-- no-GetData-bridge queue.

local ENTRY_ICHORON = 29313

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_ichoronAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function ichoronEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function ichoronKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function ichoronJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ICHORON, 1, ichoronEnterCombat)
RegisterCreatureEvent(ENTRY_ICHORON, 3, ichoronKilledUnit)
RegisterCreatureEvent(ENTRY_ICHORON, 4, ichoronJustDied)
