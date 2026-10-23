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
-- 17531) ported via creature:PlayDirectSound (savianaDied);
-- zero Talk lines in the arm.
-- boss_saviana_ragefire MovementInform — Talk(SAY_CONFLAGRATION 1)
-- rides POINT_FLIGHT (no-movement-inform bridge); the POINT_LAND /
-- POINT_LAND_GROUND / POINT_TAKEOFF MotionMaster legs have no
-- motion-master bridge.
-- boss_saviana_ragefire UpdateAI — Talk(EMOTE_ENRAGED 2) +
-- EVENT_ENRAGE ported 2026-10-13 on the maiden CreateLuaEvent
-- convention (see below); the EVENT_FLIGHT / EVENT_AIR_MOVEMENT /
-- EVENT_LAND_GROUND legs ride the MotionMaster (no motion-master
-- bridge) and EVENT_CONFLAGRATION (:173 — DoCastSelf(SPELL_CONFLA-
-- GRATION 74452, true)) stays out because it is scheduled only from
-- MovementInform(POINT_FLIGHT) (:113): an air-phase-gated cast behind
-- the unbridged flight machine (the gothik precedent) — the cast and
-- timer bridges exist, the phase machine is the blocker; the
-- EnterEvadeMode _DespawnAtEvade() leg has no evade-model bridge.
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
-- SAY_KILL 3 + EMOTE_ENRAGED 2 ported; SAY_CONFLAGRATION 1 documented
-- above).

local ENTRY_SAVIANA_RAGEFIRE = 39747

local SAY_AGGRO = 0
local SAY_KILL  = 3

local EMOTE_ENRAGED      = 2
local SPELL_ENRAGE       = 78722
local SPELL_FLAME_BREATH = 74403

local SOUND_ID_DEATH = 17531

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_ENRAGE: DoCastSelf(SPELL_ENRAGE 78722) (non-triggered) +
-- Talk(EMOTE_ENRAGED 2), 20s init, Repeat(24s).
local function enrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_ENRAGE)
    creature:Talk(EMOTE_ENRAGED)
    schedule(guid, "enrage", 24000, function()
        enrageTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_BREATH: DoCastVictim(SPELL_FLAME_BREATH 74403)
-- (non-triggered), 14s init, Repeat(20s, 30s).
local function flameBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_FLAME_BREATH)
    schedule(guid, "flamebreath", {20000, 30000}, function()
        flameBreathTick(creature, guid)
    end)
end

-- C++ boss_saviana_ragefire::JustEngagedWith:
-- BossAI::JustEngagedWith(who); Talk(SAY_AGGRO);
-- events.Reset(); events.ScheduleEvent(EVENT_ENRAGE, Seconds(20),
-- EVENT_GROUP_LAND_PHASE); events.ScheduleEvent(EVENT_FLAME_BREATH,
-- Seconds(14), EVENT_GROUP_LAND_PHASE); events.ScheduleEvent(
-- EVENT_FLIGHT, Seconds(60), EVENT_GROUP_LAND_PHASE) — the auriaya
-- precedent; the land-phase timer legs (EVENT_ENRAGE /
-- EVENT_FLAME_BREATH) ride the maiden CreateLuaEvent convention now
-- (see below).
-- Caveat: C++ cancels the land-phase group at takeoff (EVENT_FLIGHT)
-- and reschedules on landing; the flight legs ride the MotionMaster
-- (no bridge), so the group-cancel can't be modeled — since flight
-- itself is unported the Lua boss never leaves the ground and
-- continuous timers match the ground-phase behavior.
local function savianaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "enrage", 20000, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "flamebreath", 14000, function()
        flameBreathTick(creature, guid)
    end)
end

local function savianaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- JustDied's DoPlaySoundToSet arm ported via creature:PlayDirectSound
-- (savianaDied); the _JustDied() instance leg stays out (no bridge).
-- C++ boss_saviana_ragefireAI::JustDied: _JustDied() (instance leg —
-- no bridge) + DoPlaySoundToSet(me, SOUND_ID_DEATH 17531), ungated.
local function savianaDied(event, creature, killer)
    creature:PlayDirectSound(SOUND_ID_DEATH)
    cancelTimers(creature:GetGUID())
end

local function savianaReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_saviana_ragefire::Reset (boss_saviana_ragefire.cpp:80-86):
    -- _Reset() -> SetReactState(REACT_AGGRESSIVE) -> me->SetCanFly(false)
    -- (:85) -> me->SetDisableGravity(false) (:86).
    creature:SetCanFly(false)
    creature:SetDisableGravity(false)
end

-- C++ boss_saviana_ragefire::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function savianaKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 1, savianaEnterCombat)
RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 2, savianaLeaveCombat)
RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 3, savianaKilledUnit)
RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 4, savianaDied)
RegisterCreatureEvent(ENTRY_SAVIANA_RAGEFIRE, 23, savianaReset)
