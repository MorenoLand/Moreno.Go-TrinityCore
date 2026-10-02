-- Grandmaster Vorpil (Shadow Labyrinth, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/boss_grandmaster_vorpil.cpp
-- (2 CreatureScripts — boss_grandmaster_vorpil (BossAI via
-- GetShadowLabyrinthAI, DATA_GRANDMASTER_VORPIL = 2) + npc_voidtraveler
-- (ScriptedAI); both registered from inside
-- AddSC_boss_grandmaster_vorpil() (lines 296-300); loader decl 38 /
-- call 161 per outland_script_loader.cpp — the THIRD group of the
-- "// Auchindoun - Shadow Labyrinth" sub-block in AddOutlandScripts(),
-- immediately after AddSC_boss_blackheart_the_inciter() (call 160) —
-- verified from the loader this run; the checkpoint sequence
-- (blackheart_the_inciter -> grandmaster_vorpil) is followed).
-- Entry: 18732 NPC_GRANDMASTER_VORPIL — from shadow_labyrinth.h's
-- Creatures enum (line 50); the .cpp file's own enum carries only the
-- companion entries NPC_VOID_TRAVELER = 19226 / NPC_VOID_PORTAL =
-- 19224 — the boss entry is verifiable from the C++ tree
-- (NPC_ANZU-in-sethekk_halls.h / ambassador_hellmaw precedents).
-- ScriptName bindings are DB-side as usual. Sole-source verified:
-- whole-server-tree grep for "AddSC_boss_grandmaster_vorpil" hits
-- boss_grandmaster_vorpil.cpp (+ the loader decl/call lines) only.
-- No grandmaster_vorpil lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_grandmaster_vorpilAI::JustEngagedWith — Talk(SAY_AGGRO 1)
-- (line 149; event 1 — the auriaya precedent; the ScheduleEvent legs
-- (EVENT_SHADOWBOLT_VOLLEY 7-14s / heroic-only EVENT_BANISH 15s /
-- EVENT_DRAW_SHADOWS 45s / EVENT_SUMMON_TRAVELER 90s) + SummonPortals()
-- ride the no-timer / no-summon bridges and are documented-only
-- below).
-- boss_grandmaster_vorpilAI::KilledUnit — Talk(SAY_SLAY 3) (line
-- 131) gated on who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated precedent (event 3).
-- boss_grandmaster_vorpilAI::JustDied — Talk(SAY_DEATH 4) (line
-- 137) — the sjonnir self-Talk precedent (event 4; the _JustDied()
-- instance leg rides the no-instance bridge — documented-only).
-- DOCUMENTED-ONLY (in this header; only entry 18732 registered):
-- boss_grandmaster_vorpilAI::Reset — _Reset() + _intro/_helpYell
-- reset — no-instance bridge — documented-only.
-- boss_grandmaster_vorpilAI::spawnVoidTraveler — SummonCreature
-- (NPC_VOID_TRAVELER 19226, random VoidPortalCoords) + the one-shot
-- _helpYell Talk(SAY_HELP 2) (line 123) — rides the
-- EVENT_SUMMON_TRAVELER timer machine — no-summon / no-timer
-- bridges — documented-only.
-- boss_grandmaster_vorpilAI::SummonPortals — 5x SummonCreature
-- (NPC_VOID_PORTAL 19224 at VoidPortalCoords) + SPELL_VOID_PORTAL_VISUAL
-- 33569 visual cast — no-summon / no-cast bridges —
-- documented-only.
-- boss_grandmaster_vorpilAI::MoveInLineOfSight — the _intro
-- one-shot Talk(SAY_INTRO 0) (line 159) gated on player within
-- 100.0f + LOS + valid-attack-target — event 27 exists but its
-- fire site is the aggro-engage branch, not a 100-unit sighting —
-- fire-site mismatch (shaffar/maladaar precedent), zero
-- RegisterCreatureEvent(27) usage in lua_scripts/ — documented-only.
-- boss_grandmaster_vorpilAI::UpdateAI — the whole timer machine
-- rides no-timer / no-cast / no-random-target bridges:
-- EVENT_SHADOWBOLT_VOLLEY (DoCast(me, SPELL_SHADOWBOLT_VOLLEY
-- 33841), 15-30s cycle); heroic-only EVENT_BANISH (SelectTarget
-- random 30f → DoCast(target, SPELL_BANISH 38791), 15s cycle);
-- EVENT_DRAW_SHADOWS (teleports every alive non-banished player to
-- VorpilPosition, UpdatePosition, DoCast SPELL_DRAW_SHADOWS 33563
-- triggered + SPELL_RAIN_OF_FIRE 33617 / heroic 39363, 30s cycle);
-- EVENT_SUMMON_TRAVELER (spawnVoidTraveler, 10s cycle, 5s when
-- HealthBelowPct(20)); DoMeleeAttackIfReady — documented-only.
-- npc_voidtraveler (NPC_VOID_TRAVELER = 19226) — zero Talk calls;
-- UpdateAI follow/sacrifice machine: GetGuidData
-- (DATA_GRANDMASTER_VORPIL) Vorpil lookup, MoveFollow, at 3 yards
-- DoCast(me, SPELL_SACRIFICE 33587, false) then _sacrificed —
-- DoCastAOE(SPELL_EMPOWERING_SHADOWS 33783, true) +
-- DoCast(me, SPELL_SHADOW_NOVA 33846, true) + KillSelf — no-instance
-- / no-cast / no-motion bridges — documented-only (NOT registered;
-- shaffar-beacon precedent).
-- All Talk() calls in the file accounted for (3 ported above;
-- SAY_HELP 2 rides the traveler timer machine — documented-only;
-- SAY_INTRO 0 — documented-only above).

local ENTRY_GRANDMASTER_VORPIL = 18732 -- NPC_GRANDMASTER_VORPIL (shadow_labyrinth.h)

local SAY_AGGRO = 1
local SAY_SLAY  = 3
local SAY_DEATH = 4

-- C++ boss_grandmaster_vorpilAI::JustEngagedWith:
-- events.ScheduleEvent(EVENT_SHADOWBOLT_VOLLEY, 7s, 14s);
-- if (IsHeroic()) events.ScheduleEvent(EVENT_BANISH, 15s);
-- events.ScheduleEvent(EVENT_DRAW_SHADOWS, 45s);
-- events.ScheduleEvent(EVENT_SUMMON_TRAVELER, 90s);
-- Talk(SAY_AGGRO); SummonPortals(); — the auriaya precedent
-- (event 1; the ScheduleEvent legs + SummonPortals() ride the
-- no-timer / no-summon bridges — documented-only).
local function vorpilEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_grandmaster_vorpilAI::KilledUnit:
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3 — the illidan
-- victim:IsPlayer() convention).
local function vorpilTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_grandmaster_vorpilAI::JustDied:
-- _JustDied(); Talk(SAY_DEATH); — the sjonnir self-Talk precedent
-- (event 4; the _JustDied() instance leg rides the no-instance
-- bridge — documented-only).
local function vorpilDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 1, vorpilEnterCombat)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 3, vorpilTargetDied)
RegisterCreatureEvent(ENTRY_GRANDMASTER_VORPIL, 4, vorpilDied)
