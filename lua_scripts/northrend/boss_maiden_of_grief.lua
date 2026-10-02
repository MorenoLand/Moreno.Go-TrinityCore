-- Maiden of Grief (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_maiden_of_grief.cpp
-- (boss_maiden_of_grief (CreatureScript) via
-- GetHallsOfStoneAI<boss_maiden_of_griefAI> (BossAI), DATA_MAIDEN_OF_GRIEF
-- = 1 — registered from inside AddSC_boss_maiden_of_grief(); loader
-- decl 104 / call 299 per northrend_script_loader.cpp — the FIRST
-- group of the "// Halls of Stone" block in AddNorthrendScripts(),
-- under the "// Ulduar: Halls of Stone" decl marker at loader line
-- 103 and the "// Halls of Stone" call marker at loader line 298,
-- immediately after AddSC_instance_halls_of_lightning() (decl 102 /
-- call 297), which closed the Halls of Lightning block; the call
-- after it is AddSC_boss_krystallus() (decl 105 / call 300) —
-- loader order confirmed this run; the checkpoint sequence
-- (instance_halls_of_lightning -> boss_maiden_of_grief) is followed).
-- Entry: 27975 Maiden of Grief (halls_of_stone.h NPC_MAIDEN, line 49;
-- DATA_MAIDEN_OF_GRIEF = 1, line 32; GO_MAIDEN_DOOR = 191292, line
-- 63) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_maiden_of_grief" hits boss_maiden_of_grief.cpp + the loader
-- decl/call lines for AddSC_boss_maiden_of_grief() only; zero sql/
-- hits. No maiden_of_grief lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough and the
-- DoStartTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT 20383) leg
-- have no bridges — the boss_bookkeeping precedent for the former,
-- no timed-achievement bridge for the latter).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — nineteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27975):
-- Reset / Initialize (_Reset() + four ScheduleEvent legs
-- (EVENT_PARTING_SORROW 25s/30s heroic-only, EVENT_STORM_OF_GRIEF
-- 10s, EVENT_SHOCK_OF_SORROW 20s/25s, EVENT_PILLAR_OF_WOE 5s/15s)
-- + DoStopTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT) — no
-- timer / achievement bridges).
-- UpdateAI event machine — EVENT_PARTING_SORROW (random-target
-- DoCast(SPELL_PARTING_SORROW 59723), 30s/40s); EVENT_STORM_OF_GRIEF
-- (DoCastVictim(SPELL_STORM_OF_GRIEF 50752, true), 15s/20s);
-- EVENT_SHOCK_OF_SORROW (ResetThreatList + Talk(SAY_STUN 3) +
-- DoCastAOE(SPELL_SHOCK_OF_SORROW 50760), 20s/30s); EVENT_PILLAR_OF_WOE
-- (random-target DoCast(SPELL_PILLAR_OF_WOE 50761) else
-- DoCastVictim, 5s/25s) — the UNIT_STATE_CASTING gate and the
-- DoMeleeAttackIfReady tail have no bridges; Talk(3) SAY_STUN rides
-- the unmodeled machine.

local ENTRY_MAIDEN_OF_GRIEF = 27975

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ JustEngagedWith: Talk(AGGRO) + DoStartTimedAchievement +
-- BossAI passthrough — only the Talk arm is bridgeable.
local function maidenEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function maidenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function maidenDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 1, maidenEnterCombat)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 3, maidenTargetDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 4, maidenDied)
