-- Zuramat the Obliterator (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_zuramat.cpp
-- (232 lines incl. license; 3 scripts — boss_zuramat (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_ZURAMAT = 8); npc_void_sentry
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), zero Talk arms,
-- unbridged); achievement_void_dance (AchievementCriteriaScript OnCheck
-- AI()->GetData(DATA_VOID_DANCE)); all registered from inside
-- AddSC_boss_zuramat(); loader decl 151 / call 346 per
-- northrend_script_loader.cpp — the SEVENTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_xevozz() (call 345); the call after it is
-- AddSC_instance_violet_hold() (call 347) — the checkpoint sequence
-- (boss_xevozz -> boss_zuramat) is followed).
-- Entries: 29314 Zuramat the Obliterator (violet_hold.h NPC_ZURAMAT,
-- line 101) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_ZURAMAT
-- = 8 (violet_hold.h line 58). NPC_VOID_SENTRY = 29364 (line 102,
-- zero Talk — not registered); NPC_VOID_SENTRY_BALL = 29365 (line
-- 103).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_zuramat"
-- hits boss_zuramat.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No zuramat lua existed.
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
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 29314):
-- SAY_SPAWN (3), SAY_SHIELD (4) and SAY_WHISPER (5) are never Talk()ed
-- anywhere in the file (dead enum text, the cyanigosa precedent;
-- grep -c "Talk(" = 3 total, all ported above).
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_ZURAMAT) leg joins the no-instance-SetData bridge queue (the
-- erekem/ichoron/lavanthor precedent). The ScheduleTasks machine
-- (SUMMON_VOID_SENTRY 4s-then-7-10s / VOID_SHIFT 9s-then-15s
-- random-target-60yd / SHROUD_OF_DARKNESS 18-20s-then-18-20s) and the
-- DoMeleeAttackIfReady UpdateAI leg join the no-timer-bridge queue
-- (the boss_toravon precedent). SummonedCreatureDies
-- (VOID_SENTRY death -> _voidDance = false) / SummonedCreatureDespawn
-- (VOID_SENTRY despawn -> DoAction ACTION_DESPAWN_VOID_SENTRY_BALL on
-- the summon) and the npc_void_sentry machine (IsSummonedBy
-- self-cast SUMMON_VOID_SENTRY_BALL, JustSummoned SummonList +
-- REACT_PASSIVE, SummonedCreatureDespawn Despawn, DoAction
-- ACTION_DESPAWN_VOID_SENTRY_BALL DespawnAll, JustDied DespawnAll)
-- have no bridges — the no-DoAction / no-SummonList bridge queues.
-- achievement_void_dance (OnCheck AI()->GetData(DATA_VOID_DANCE))
-- joins the unmodeled-achievement / no-GetData-bridge queue (the
-- cyanigosa / ichoron precedent).

local ENTRY_ZURAMAT = 29314

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_zuramatAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function zuramatEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function zuramatKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function zuramatJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ZURAMAT, 1, zuramatEnterCombat)
RegisterCreatureEvent(ENTRY_ZURAMAT, 3, zuramatKilledUnit)
RegisterCreatureEvent(ENTRY_ZURAMAT, 4, zuramatJustDied)
