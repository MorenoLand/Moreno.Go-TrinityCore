-- Saviana Ragefire (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_saviana_ragefire.cpp
-- (all 1 CreatureScript registered from inside
-- AddSC_boss_saviana_ragefire() (line 292); loader decl 191 / call 386
-- per northrend_script_loader.cpp — the FOURTH group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_baltharus_the_warborn() (call 385) — verified from the
-- loader this run; the checkpoint sequence (baltharus -> saviana) is
-- followed).
-- Entry: 39747 Saviana Ragefire (ruby_sanctum.h
-- NPC_SAVIANA_RAGEFIRE, line 77; instance_ruby_sanctum.cpp
-- creatureData binds NPC_SAVIANA_RAGEFIRE -> DATA_SAVIANA_RAGEFIRE
-- (line 53); DATA_SAVIANA_RAGEFIRE = 2 (ruby_sanctum.h line 33)) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_saviana_ragefire" hits boss_saviana_ragefire.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill.
-- Ported arms (C++-exact for all modeled arms):
-- boss_saviana_ragefire JustEngagedWith — Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent; the BossAI::JustEngagedWith /
-- events.Reset / ScheduleEvent(EVENT_ENRAGE / EVENT_FLAME_BREATH /
-- EVENT_FLIGHT) legs have no bridges).
-- boss_saviana_ragefire KilledUnit — Talk(SAY_KILL 3) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; only entry 39747 registered):
-- boss_saviana_ragefire JustDied — _JustDied() instance leg (no
-- instance-script bridge) + DoPlaySoundToSet(me, SOUND_ID_DEATH
-- 17531) (no play-sound bridge); zero Talk lines in the arm — NOT
-- a bridgeable Talk event.
-- boss_saviana_ragefire MovementInform — Talk(SAY_CONFLAGRATION 1)
-- rides POINT_FLIGHT (no-movement-inform bridge); the POINT_LAND /
-- POINT_LAND_GROUND / POINT_TAKEOFF MotionMaster legs have no
-- motion-master bridge.
-- boss_saviana_ragefire UpdateAI — Talk(EMOTE_ENRAGED 2) rides
-- EVENT_ENRAGE (no-timer-bridge); the EVENT_FLIGHT / EVENT_FLAME_BREATH
-- / EVENT_CONFLAGRATION / EVENT_AIR_MOVEMENT / EVENT_LAND_GROUND
-- DoCastSelf / DoCastVictim / MoveTakeoff / MovePoint / MoveLand /
-- events.Repeat legs have no-cast / no-motion-master / no-timer
-- bridges; the EnterEvadeMode _DespawnAtEvade() leg has no
-- evade-model bridge.
-- spell_saviana_conflagration_init (74452) — SpellScript:
-- FilterTargets (OnObjectAreaTargetSelect, EFFECT_0,
-- TARGET_UNIT_SRC_AREA_ENEMY; remove non-players; RandomResize
-- 3/6 by spawn mode) + HandleDummy (PreventHitDefaultEffect;
-- triggered FLAME_BEACON 74453 + CONFLAGRATION_2 74454) — no
-- SpellScript bridge (the sindragosa ice_tomb_target precedent).
-- spell_saviana_conflagration_throwback (74455) — SpellScript:
-- HandleScript (PreventHitDefaultEffect; hit-unit CastSpell
-- GetEffectValue on caster + MovePoint(POINT_LAND,
-- SavianaRagefireFlyInPos)) — no SpellScript bridge.
-- All 4 Talk() calls in the file accounted for (SAY_AGGRO 0 +
-- SAY_KILL 3 ported; SAY_CONFLAGRATION 1 + EMOTE_ENRAGED 2
-- documented above).

local ENTRY_SAVIANA_RAGEFIRE = 39747

local SAY_AGGRO = 0
local SAY_KILL  = 3

-- C++ boss_saviana_ragefire::JustEngagedWith:
-- BossAI::JustEngagedWith(who); Talk(SAY_AGGRO);
-- events.Reset(); events.ScheduleEvent(EVENT_ENRAGE, Seconds(20),
-- EVENT_GROUP_LAND_PHASE); events.ScheduleEvent(EVENT_FLAME_BREATH,
-- Seconds(14), EVENT_GROUP_LAND_PHASE); events.ScheduleEvent(
-- EVENT_FLIGHT, Seconds(60), EVENT_GROUP_LAND_PHASE) — the auriaya
-- precedent (the non-Talk legs have no bridges).
local function savianaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_saviana_ragefire::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function savianaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 1, savianaEnterCombat)
RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 3, savianaKilledUnit)
