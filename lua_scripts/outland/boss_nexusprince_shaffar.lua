-- Nexus-Prince Shaffar (Mana Tombs, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ManaTombs/boss_nexusprince_shaffar.cpp
-- (4 CreatureScripts: boss_nexusprince_shaffar (BossAI via
-- GetManaTombsAI, DATA_NEXUSPRINCE_SHAFFAR) + npc_ethereal_beacon
-- (ScriptedAI) + npc_ethereal_apprentice (ScriptedAI) + npc_yor
-- (ScriptedAI); all registered from inside
-- AddSC_boss_nexusprince_shaffar() (line 387); loader decl 26 / call
-- 149 per outland_script_loader.cpp — the SECOND group of the
-- "// Auchindoun - Mana Tombs" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_pandemonius() (call 148) — verified
-- from the loader this run; the checkpoint sequence (pandemonius ->
-- nexusprince_shaffar) is followed).
-- Entries: 18344 NPC_SHAFFAR / 18431 NPC_BEACON — from the file's own
-- Creatures enum; the ethereal apprentice entry (18430) appears only
-- in the SPELL_ETHEREAL_APPRENTICE comment; the ScriptName bindings
-- are DB-side as usual. Sole-source verified: whole-server-tree grep
-- for "AddSC_boss_nexusprince_shaffar" hits
-- boss_nexusprince_shaffar.cpp (+ the loader decl/call lines) only.
-- No shaffar lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_nexusprince_shaffarAI::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 137; event 1 — the auriaya precedent; the
-- BossAI::JustEngagedWith legs — EVENT_BEACON 10s / EVENT_FIREBALL
-- 8s / EVENT_FROSTBOLT 4s / EVENT_FROST_NOVA 15s ScheduleEvent —
-- have no-timer bridges and are documented-only below).
-- boss_nexusprince_shaffarAI::KilledUnit — Talk(SAY_SLAY 2) (line
-- 161) gated on victim->GetTypeId() == TYPEID_PLAYER — the
-- razuvious player-gated precedent (event 3).
-- boss_nexusprince_shaffarAI::JustDied — Talk(SAY_DEAD 4) (line
-- 166) — the sjonnir self-Talk precedent (event 4).
-- DOCUMENTED-ONLY (in this header; only entry 18344 registered):
-- boss_nexusprince_shaffarAI::Reset — the three initial beacon
-- SummonCreature(NPC_BEACON 18431, TEMPSUMMON_CORPSE_TIMED_DESPAWN
-- 2h) legs — no-summon bridge (shirrak precedent).
-- boss_nexusprince_shaffarAI::MoveInLineOfSight — Talk(SAY_INTRO 0)
-- (line 131) one-shot _hasTaunted-gated on a player within 100.0f —
-- the maladaar precedent: event 27 exists in the engine but its fire
-- site is the aggro-engage branch ("when the creature is about to
-- aggro a player it has sighted"), not a 100-unit sighting — fire-site
-- mismatch, zero RegisterCreatureEvent(27) precedent in lua_scripts/
-- — documented-only.
-- boss_nexusprince_shaffarAI::JustSummoned — beacon
-- CastSpell(SPELL_ETHEREAL_BEACON_VISUAL 32368) + Random-SelectTarget
-- AttackStart legs — no-cast / no-random-target / no-cross-creature
-- bridges (the JustSummoned summon-coordination leg is the maladaar
-- precedent) — documented-only.
-- boss_nexusprince_shaffarAI::ExecuteEvent — the whole machine rides
-- no-timer / no-cast bridges: EVENT_BLINK interrupt + Clear
-- MOTION_PRIORITY_NORMAL + DoCast(SPELL_BLINK 34605); EVENT_BEACON
-- urand(0,3)==0-gated Talk(SAY_SUMMON 3) + DoCast(SPELL_ETHEREAL_
-- BEACON 32371, triggered) 10s; EVENT_FIREBALL / EVENT_FROSTBOLT
-- DoCastVictim(SPELL_FROSTBOLT 32364) 4.5-6s (the C++ fires
-- SPELL_FROSTBOLT 32364 under the EVENT_FIREBALL case — preserved
-- C++-exact); EVENT_FROST_NOVA DoCastSelf(SPELL_FROSTNOVA 32365)
-- 17.5-25s + chained EVENT_BLINK 1.5s.
-- npc_ethereal_beacon (18431) — zero Talk; the JustEngagedWith
-- cross-creature leg (FindNearestCreature(NPC_SHAFFAR 18344, 100f) +
-- AttackStart(who) when shaffar not in combat) rides no-cross-creature
-- / no-summon bridges; the EVENT_APPRENTICE 20s/10s
-- DoCast(SPELL_ETHEREAL_APPRENTICE 32372, triggered) +
-- DespawnOrUnsummon + EVENT_ARCANE_BOLT 1s DoCastVictim(SPELL_ARCANE_
-- BOLT 15254) 2-4.5s machine rides no-timer / no-cast /
-- no-despawn bridges — NOT registered.
-- npc_ethereal_apprentice — zero Talk; the alternating
-- EVENT_ETHEREAL_APPRENTICE_FIREBOLT (SPELL_FIREBOLT 32369) /
-- EVENT_ETHEREAL_APPRENTICE_FROSTBOLT (SPELL_FROSTBOLT 32370)
-- 3s-ping-pong triggered-victim-cast machine rides no-timer /
-- no-cast bridges — NOT registered.
-- npc_yor — zero Talk; the EVENT_DOUBLE_BREATH
-- DoCastVictim(SPELL_DOUBLE_BREATH 38361) 6-9s distance-gated
-- (ATTACK_DISTANCE) + DoMeleeAttackIfReady machine rides no-timer /
-- no-cast bridges — NOT registered.
-- All Talk() calls in the file accounted for (3 ported above +
-- SAY_INTRO 0 MoveInLineOfSight + SAY_SUMMON 3 EVENT_BEACON machine
-- documented above).

local ENTRY_SHAFFAR = 18344 -- NPC_SHAFFAR (Creatures enum)

local SAY_AGGRO = 1
local SAY_SLAY  = 2
local SAY_DEAD  = 4

-- C++ boss_nexusprince_shaffarAI::JustEngagedWith:
-- Talk(SAY_AGGRO); BossAI::JustEngagedWith(who) — the auriaya
-- precedent (the BossAI legs — the four ScheduleEvent legs — have
-- no-timer bridges).
local function shaffarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_nexusprince_shaffarAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:IsPlayer() convention).
local function shaffarTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_nexusprince_shaffarAI::JustDied: Talk(SAY_DEAD) — the
-- sjonnir self-Talk precedent (event 4).
local function shaffarDied(event, creature, killer)
    creature:Talk(SAY_DEAD)
end

RegisterCreatureEvent(ENTRY_SHAFFAR, 1, shaffarEnterCombat)
RegisterCreatureEvent(ENTRY_SHAFFAR, 3, shaffarTargetDied)
RegisterCreatureEvent(ENTRY_SHAFFAR, 4, shaffarDied)
