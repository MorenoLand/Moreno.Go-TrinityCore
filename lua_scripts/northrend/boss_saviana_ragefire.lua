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
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnKill, 4 OnDied, 23 OnReset. No 24 OnReachHome: C++ has no
-- JustReachedHome override (base arm is setActive(false) only,
-- ScriptedCreature.cpp:521-524) — see savianaReset below.
-- Ported arms (C++-exact for all modeled arms):
-- boss_saviana_ragefire JustEngagedWith — BossAI::JustEngagedWith
-- SetBossState(IN_PROGRESS) (ScriptedCreature.cpp:537, base-first)
-- + SetCombatPulseDelay(5) + DoZoneInCombat + Talk(SAY_AGGRO 0)
-- (event 1 — the anubrekhan precedent; CheckRequiredBosses is
-- vacuous here, default true per InstanceScript.h:246).
-- boss_saviana_ragefire JustDied — BossAI::_JustDied SetBossState
-- (DONE) (ScriptedCreature.cpp:519) + DoPlaySoundToSet(me,
-- SOUND_ID_DEATH 17531) via creature:PlayDirectSound (event 4,
-- _JustDied-first order per boss_saviana_ragefire.cpp:124-125).
-- boss_saviana_ragefire Reset — BossAI::_Reset SetBossState
-- (NOT_STARTED), != DONE-gated (ScriptedCreature.cpp:510), +
-- SetReactState(REACT_AGGRESSIVE) + SetCanFly(false) +
-- SetDisableGravity(false) (event 23); the EnterEvadeMode FAIL leg
-- rides the ev23 evade fire (palehoof/tyrannus precedent — state ==
-- IN_PROGRESS at evade -> FAIL, ScriptedCreature.cpp:617), since
-- EnterEvadeMode itself has no Lua bridge (boss_ai.go:5753-5754)
-- and the DespawnOrUnsummon leg is Go-owned (despawnAtEvade).
-- boss_saviana_ragefire KilledUnit — Talk(SAY_KILL 3) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; only entry 39747 registered):
-- boss_saviana_ragefire MovementInform — Talk(SAY_CONFLAGRATION 1)
-- rides POINT_FLIGHT (no-movement-inform bridge); the POINT_LAND /
-- POINT_LAND_GROUND / POINT_TAKEOFF MotionMaster legs have no
-- motion-master bridge.
-- boss_saviana_ragefire UpdateAI — Talk(EMOTE_ENRAGED 2) +
-- EVENT_ENRAGE ported 2026-10-13 on the maiden CreateLuaEvent
-- convention (see below); the UpdateAI CASTING gates (:153 pre-loop +
-- :195 in-loop) are ported as 500ms IsCasting defer on enrage and
-- flame-breath ticks; the EVENT_FLIGHT / EVENT_AIR_MOVEMENT /
-- EVENT_LAND_GROUND legs ride the MotionMaster (no motion-master
-- bridge) and EVENT_CONFLAGRATION (:173 — DoCastSelf(SPELL_CONFLA-
-- GRATION 74452, true)) stays out because it is scheduled only from
-- MovementInform(POINT_FLIGHT) (:113): an air-phase-gated cast behind
-- the unbridged flight machine (the gothik precedent) — the cast and
-- timer bridges exist, the phase machine is the blocker; the
-- EnterEvadeMode _DespawnAtEvade() FAIL leg rides the ev23 evade
-- fire (see savianaReset below), only its DespawnOrUnsummon leg is
-- Go-owned (despawnAtEvade).
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

local DATA_SAVIANA_RAGEFIRE = 2 -- ruby_sanctum.h:33 (RSDataTypes)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

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
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_ENRAGED)
    schedule(guid, "enrage", 24000, function()
        enrageTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_BREATH: DoCastVictim(SPELL_FLAME_BREATH 74403)
-- (non-triggered), 14s init, Repeat(20s, 30s).
local function flameBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_FLAME_BREATH)
    if creature:IsCasting() then
        schedule(guid, "flamebreath", 500, function()
            flameBreathTick(creature, guid)
        end)
        return
    end
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
-- (see below); the EVENT_FLIGHT schedule stays out (MotionMaster leg,
-- no bridge).
-- Caveat: C++ cancels the land-phase group at takeoff (EVENT_FLIGHT)
-- and reschedules on landing; the flight legs ride the MotionMaster
-- (no bridge), so the group-cancel can't be modeled — since flight
-- itself is unported the Lua boss never leaves the ground and
-- continuous timers match the ground-phase behavior.
local function savianaEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base call first (boss_saviana_ragefire.cpp:91):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:526-543) --
    -- SetBossState(IN_PROGRESS) (:537) + SetCombatPulseDelay(5) (:540)
    -- + me-form DoZoneInCombat (:542), base-first (C++ arm order;
    -- anubrekhan precedent); CheckRequiredBosses (:530-535) is vacuous
    -- here (default returns true, InstanceScript.h:246; the RS instance
    -- script does not override it) and setActive(true)/ScheduleTasks()
    -- have no bridge.
    creature:SetBossState(DATA_SAVIANA_RAGEFIRE, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537), base-first
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "enrage", 20000, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "flamebreath", 14000, function()
        flameBreathTick(creature, guid)
    end)
end

local function savianaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- JustDied's _JustDied() instance leg is PORTED via
-- creature:SetBossState (DONE) + cancelTimers (events.Reset), C++
-- order (ScriptedCreature.cpp:514-519: events.Reset, then
-- SetBossState(DONE)); the DoPlaySoundToSet arm follows via
-- creature:PlayDirectSound. C++ boss_saviana_ragefireAI::JustDied:
-- _JustDied() first, then DoPlaySoundToSet(me, SOUND_ID_DEATH 17531);
-- zero Talk lines in the arm.
local function savianaDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(DATA_SAVIANA_RAGEFIRE, BOSS_STATE_DONE) -- BossAI::_JustDied (ScriptedCreature.cpp:519)
    creature:PlayDirectSound(SOUND_ID_DEATH)
end

local function savianaReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ boss_saviana_ragefire::Reset (boss_saviana_ragefire.cpp:80-86):
    -- _Reset() -> SetReactState(REACT_AGGRESSIVE) -> me->SetCanFly(false)
    -- (:85) -> me->SetDisableGravity(false) (:86). BossAI::_Reset
    -- (ScriptedCreature.cpp:499-511) early-returns when dead and sets
    -- NOT_STARTED only when state != DONE (:509-510); the IsAlive guard
    -- below also skips the death pre-fire (Go fires ev23 before ev4 on
    -- death; DONE lands in ev4). The FAIL leg rides the ev23 evade fire
    -- (palehoof/tyrannus precedent): Saviana's EnterEvadeMode
    -- (:149-152) calls _DespawnAtEvade(), whose instance arm is
    -- SetBossState(FAIL) (ScriptedCreature.cpp:617); Go's evade funnel
    -- fires ev23 while state is still IN_PROGRESS
    -- (creaturemotion.go:4505), and the DespawnOrUnsummon leg itself
    -- stays out — the Go evade funnel owns evade-despawn timing
    -- (boss_ai.go despawnAtEvade). No ev24 handler: C++ has no
    -- JustReachedHome override, so the base arm is only
    -- setActive(false) (ScriptedCreature.cpp:521-524) with no state
    -- leg — an ev24 FAIL would fabricate one.
    if creature:IsAlive() then
        if creature:GetBossState(DATA_SAVIANA_RAGEFIRE) == BOSS_STATE_IN_PROGRESS then
            creature:SetBossState(DATA_SAVIANA_RAGEFIRE, BOSS_STATE_FAIL) -- _DespawnAtEvade FAIL, evade path
        elseif creature:GetBossState(DATA_SAVIANA_RAGEFIRE) ~= BOSS_STATE_DONE then
            creature:SetBossState(DATA_SAVIANA_RAGEFIRE, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
        end
    end
    creature:SetReactState(2) -- REACT_AGGRESSIVE
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
