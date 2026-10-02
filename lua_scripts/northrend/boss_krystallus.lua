-- Krystallus (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_krystallus.cpp
-- (boss_krystallus (CreatureScript) via
-- GetHallsOfStoneAI<boss_krystallusAI> (BossAI), DATA_KRYSTALLUS
-- = 0 — registered from inside AddSC_boss_krystallus(); loader
-- decl 105 / call 300 per northrend_script_loader.cpp — the SECOND
-- group of the "// Halls of Stone" block in AddNorthrendScripts(),
-- under the "// Ulduar: Halls of Stone" decl marker at loader line
-- 103 and the "// Halls of Stone" call marker at loader line 298,
-- immediately after AddSC_boss_maiden_of_grief() (decl 104 /
-- call 299), which opened the Halls of Stone block; the call
-- after it is AddSC_boss_sjonnir() (decl 106 / call 301) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_maiden_of_grief -> boss_krystallus) is followed).
-- Entry: 27977 Krystallus (halls_of_stone.h NPC_KRYSTALLUS, line 42;
-- DATA_KRYSTALLUS = 0, line 30; GO_KRYSTALLUS doors are not bound
-- in this script) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_krystallus" hits boss_krystallus.cpp + the loader
-- decl/call lines for AddSC_boss_krystallus() only; whole-tree
-- grep for "spell_krystallus_shatter" and
-- "spell_krystallus_shatter_effect" hits boss_krystallus.cpp
-- only; zero sql/ hits for all three. No krystallus lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough and the five ScheduleEvent
-- legs have no bridges — the boss_bookkeeping precedent for the
-- former, no timer bridge for the latter).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_KILL 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — twentieth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27977):
-- Reset (_Reset() — no bridge).
-- JustEngagedWith ScheduleEvent legs (EVENT_BOULDER_TOSS 3s/9s,
-- EVENT_GROUND_SLAM 15s/18s, EVENT_STOMP 20s/29s,
-- EVENT_GROUND_SPIKE 9s/14s heroic-only — no timer bridge).
-- UpdateAI event machine — EVENT_BOULDER_TOSS (random-target
-- 50.0f DoCast(SPELL_BOULDER_TOSS 50843), 9s/15s);
-- EVENT_GROUND_SPIKE (random-target 100.0f
-- DoCast(SPELL_GROUND_SPIKE 59750), 12s/17s); EVENT_GROUND_SLAM
-- (DoCast(SPELL_GROUND_SLAM 50827) + EVENT_SHATTER 10s, 15s/18s);
-- EVENT_STOMP (DoCast(SPELL_STOMP 48131), 20s/29s);
-- EVENT_SHATTER (DoCast(SPELL_SHATTER 50810)) — the
-- UNIT_STATE_CASTING gates and the DoMeleeAttackIfReady tail have
-- no bridges. (enum Yells SAY_SHATTER = 3 is declared but never
-- Talk()ed in C++; nothing rides it.)
-- spell_krystallus_shatter (SpellScriptLoader:
-- EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT handler —
-- RemoveAurasDueToSpell(SPELL_STONED 50812) + triggered
-- CastSpell(SPELL_SHATTER_EFFECT 50811) — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue).
-- spell_krystallus_shatter_effect (SpellScriptLoader: OnHit
-- CalculateDamage — radius-scaled SetHitDamage via CalcRadius —
-- no SpellScript bridge; joins the no-SpellScript-bridge queue).

local ENTRY_KRYSTALLUS = 27977

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI passthrough +
-- ScheduleEvent legs — only the Talk arm is bridgeable.
local function krystallusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function krystallusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function krystallusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_KRYSTALLUS, 1, krystallusEnterCombat)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 3, krystallusTargetDied)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 4, krystallusDied)
