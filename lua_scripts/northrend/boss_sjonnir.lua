-- Sjonnir the Ironshaper (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_sjonnir.cpp
-- (boss_sjonnir (CreatureScript) via
-- GetHallsOfStoneAI<boss_sjonnirAI> (BossAI), DATA_SJONNIR = 3
-- — registered from inside AddSC_boss_sjonnir(); loader decl 106
-- / call 301 per northrend_script_loader.cpp — the THIRD group of
-- the "// Halls of Stone" block in AddNorthrendScripts(), under
-- the "// Ulduar: Halls of Stone" decl marker (line 103) and the
-- "// Halls of Stone" call marker (line 298), immediately after
-- AddSC_boss_krystallus() (decl 105 / call 300); the call after
-- it is AddSC_instance_halls_of_stone() — loader order confirmed
-- this run; the checkpoint sequence (boss_krystallus ->
-- boss_sjonnir) is followed).
-- Entry: 27978 Sjonnir the Ironshaper (halls_of_stone.h
-- NPC_SJONNIR, line 51; DATA_SJONNIR = 3, line 34;
-- GO_SJONNIR_DOOR 191296, line 65 — bound in the instance
-- script, not here) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_sjonnir" hits boss_sjonnir.cpp (+ the loader decl/call
-- lines for AddSC_boss_sjonnir()) only; whole-tree grep for
-- "npc_malformed_ooze", "npc_iron_sludge" and
-- "achievement_abuse_the_ooze" hits boss_sjonnir.cpp only; zero
-- sql/ hits for all four. No sjonnir lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied. No timers in the ported arms; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- CheckRequiredBosses / EnterEvadeMode gate has no
-- required-bosses bridge; the BossAI::JustEngagedWith passthrough
-- and the six ScheduleEvent legs have no bridges — the
-- boss_bookkeeping precedent for the passthrough, no timer
-- bridge for the legs).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious
-- player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player") — twenty-first
-- player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27978):
-- Reset / Initialize — _Reset() + abuseTheOoze = 0 — no bridge.
-- DoAction(ACTION_OOZE_DEAD 1) / GetData(DATA_ABUSE_THE_OOZE 2)
-- — abuseTheOoze counter — no DoAction / GetData bridge
-- (consumer: achievement_abuse_the_ooze below).
-- UpdateAI event machine — EVENT_CHAIN_LIGHTNING (random-target
-- DoCast 50830, 10s/15s); EVENT_LIGHTNING_SHIELD (self 50831);
-- EVENT_STATIC_CHARGE (DoCastVictim 50834, 20s/25s);
-- EVENT_LIGHTNING_RING (self 51849, 30s/35s); EVENT_SUMMON
-- (health-tier PipeLocations summon: FORGED_IRON_DWARF 27982 /
-- FORGED_IRON_TROGG 27979 / MALFORMED_OOZE 27981 / EARTHEN_DWARF
-- 27980, TEMPSUMMON_CORPSE_TIMED_DESPAWN 30s, 20s reschedule);
-- EVENT_FRENZY (triggered FRENZY 28747); the UNIT_STATE_CASTING
-- gates and the DoMeleeAttackIfReady tail have no bridges —
-- no timer-event / cast / target-selection / health-pct /
-- summon bridges.
-- npc_malformed_ooze (CreatureScript via
-- GetHallsOfStoneAI<npc_malformed_oozeAI> (ScriptedAI);
-- NPC_MALFORMED_OOZE = 27981, local enum in boss_sjonnir.cpp —
-- entry-verifiable but bridge-blocked, NO registration — the
-- npc_spark_of_ionar / stormforged_lieutenant no-bridgeable-arms
-- precedent; no Talk arms anywhere in the script — the merge
-- machine (FindNearestCreature 27981 3.0f -> DoSpawnCreature
-- IRON_SLUDGE 28165 + double DisappearAndDie; _mergeTimer 10s /
-- 3s) has no proximity / summon / despawn / timer bridges).
-- npc_iron_sludge (CreatureScript via
-- GetHallsOfStoneAI<npc_iron_sludgeAI> (ScriptedAI);
-- NPC_IRON_SLUDGE = 28165, local enum — entry-verifiable but
-- bridge-blocked, NO registration; no Talk arms anywhere —
-- JustDied -> ObjectAccessor::GetCreature(instance GetGuidData
-- DATA_SJONNIR) -> DoAction(ACTION_OOZE_DEAD) — no
-- ObjectAccessor / GetGuidData / DoAction bridges).
-- achievement_abuse_the_ooze (AchievementCriteriaScript OnCheck:
-- Sjonnir->AI()->GetData(DATA_ABUSE_THE_OOZE) >= 5 — no GetData
-- / achievement bridges; joins the unmodeled-achievement queue).

local ENTRY_SJONNIR = 27978

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ JustEngagedWith: Talk(AGGRO) + required-bosses gate +
-- BossAI passthrough + ScheduleEvent legs — only the Talk arm is
-- bridgeable.
local function sjonnirEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function sjonnirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function sjonnirDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SJONNIR, 1, sjonnirEnterCombat)
RegisterCreatureEvent(ENTRY_SJONNIR, 3, sjonnirTargetDied)
RegisterCreatureEvent(ENTRY_SJONNIR, 4, sjonnirDied)
