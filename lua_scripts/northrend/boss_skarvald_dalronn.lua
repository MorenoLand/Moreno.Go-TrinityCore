-- Skarvald the Constructor and Dalronn the Controller (Utgarde Keep) —
-- Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_skarvald_dalronn.cpp
-- (302 lines; 2 scripts — boss_skarvald_the_constructor /
-- boss_dalronn_the_controller (CreatureScript via GetUtgardeKeepAI
-- (BossAI), DATA_SKARVALD_DALRONN = 1; generic_boss_controllerAI shared
-- base, OtherBossData DATA_DALRONN = 4 / DATA_SKARVALD = 3); all
-- registered from inside AddSC_boss_skarvald_dalronn(); loader decl 127
-- / call 322 per northrend_script_loader.cpp — the SECOND group of the
-- "// Utgarde Keep - Utgarde Keep" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_keleseth() (decl 126 / call 321); the
-- checkpoint sequence (boss_keleseth -> boss_skarvald_dalronn) is
-- followed).
-- Entries: 24200 Skarvald the Constructor / 24201 Dalronn the
-- Controller (utgarde_keep.h NPC_SKARVALD / NPC_DALRONN, lines 47-48 —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual). DATA_SKARVALD = 3 /
-- DATA_DALRONN = 4 (utgarde_keep.h lines 36-37). Ghosts 27390
-- (NPC_SKARVALD_GHOST) / 27389 (NPC_DALRONN_GHOST, lines 52-53) share
-- the script names DB-side but carry no bridgeable Talk arms — no
-- registrations; the !IsInGhostForm gates below are satisfied
-- structurally (only non-ghost entries run this lua).
-- Sole-source verified: whole-server-tree grep for both script names
-- hits boss_skarvald_dalronn.cpp only (+ the loader decl/call lines for
-- AddSC_boss_skarvald_dalronn); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No skarvald lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill.
-- Ported arms (C++-exact for all modeled arms):
-- Skarvald JustEngagedWith — Talk(SAY_AGGRO 0) (event 1); the Talk fires
-- immediately in C++ (the !IsInGhostForm gate is structural; the
-- BossAI::JustEngagedWith passthrough and the
-- EVENT_SKARVALD_CHARGE/EVENT_STONE_STRIKE schedule legs have no bridges
-- — the auriaya engage-port precedent).
-- KilledUnit — Talk(SAY_KILL 3) player-gated (event 3, both bosses; the
-- razuvious player-gated variant precedent; the !IsInGhostForm gate is
-- structural).
-- DOCUMENTED-ONLY (in this header; no registrations beyond entries
-- 24200 / 24201):
-- Dalronn JustEngagedWith — Talk(SAY_AGGRO) rides EVENT_DELAYED_AGGRO_SAY
-- (5s delay, deliberate overlap avoidance); the timer event machine has
-- no bridge — event 1 deliberately NOT registered for Dalronn (an
-- immediate yell would deviate from C++); the
-- EVENT_SHADOW_BOLT/EVENT_DEBILITATE/EVENT_SUMMON_SKELETONS schedule
-- legs have no bridges; joins the no-timer-bridge queue.
-- JustDied — Talk(SAY_DIED_FIRST 2) when the other boss is still alive
-- (via instance GetGuidData(OtherBossData) + cross-creature
-- DoAction(ACTION_OTHER_JUST_DIED) and ghost summon DoCast) vs
-- Talk(SAY_DEATH 1) when both are dead (cross-creature
-- DoAction(ACTION_DESPAWN_SUMMONS)) — no instance-GuidData /
-- cross-creature-DoAction bridges; event 4 deliberately NOT registered
-- (neither Talk is unconditional); the RemoveFlag(UNIT_DYNFLAG_LOOTABLE)
-- leg has no bridge.
-- DoAction(ACTION_OTHER_JUST_DIED) — schedules EVENT_DEATH_RESPONSE
-- (2s) → Talk(SAY_DEATH_RESPONSE 4) — no DoAction / timer bridges.
-- Skarvald DamageTaken — Enrage latch + DoCast SPELL_ENRAGE at
-- HealthBelowPctDamaged(15) — no Talk arm, no registration.
-- The charge/stone-strike/shadow-bolt/debilitate/summon-skeletons spell
-- casts and SkarvaldChargePredicate target selection have no bridges.

local ENTRY_SKARVALD = 24200
local ENTRY_DALRONN = 24201

local SAY_AGGRO = 0
local SAY_KILL = 3

-- C++ boss_skarvald_the_constructorAI::JustEngagedWith:
-- generic_boss_controllerAI::JustEngagedWith(who) (BossAI passthrough,
-- no bridge); if (!IsInGhostForm) Talk(SAY_AGGRO) — gate satisfied
-- structurally (ghost entry 27390 not registered) — the auriaya
-- engage-port precedent.
local function skarvaldEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ generic_boss_controllerAI::KilledUnit: if (!IsInGhostForm &&
-- who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — ghost gate
-- structural — the razuvious player-gated variant precedent.
local function skarvaldKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function dalronnKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_SKARVALD, 1, skarvaldEnterCombat)
RegisterCreatureEvent(ENTRY_SKARVALD, 3, skarvaldKilledUnit)
RegisterCreatureEvent(ENTRY_DALRONN, 3, dalronnKilledUnit)
