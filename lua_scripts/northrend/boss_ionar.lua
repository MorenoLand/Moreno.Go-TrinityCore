-- Ionar (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_ionar.cpp
-- (boss_ionar (CreatureScript) via GetHallsOfLightningAI<boss_ionarAI>
-- (ScriptedAI); npc_spark_of_ionar (CreatureScript) via
-- GetHallsOfLightningAI<npc_spark_of_ionarAI> (ScriptedAI) — both
-- registered from inside AddSC_boss_ionar(); loader decl 100 /
-- call 295 per northrend_script_loader.cpp — the THIRD group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_loken() (decl 99 / call 294), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_volkhan() (decl 101 / call 296) — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_loken -> boss_ionar) is followed).
-- Entry: 28546 Ionar (halls_of_lightning.h NPC_IONAR, line 41;
-- DATA_IONAR = 2, line 33); instance_halls_of_lightning.cpp
-- OnCreatureCreate binds case NPC_IONAR (line 57) and GetGuidData
-- handles DATA_IONAR (line 109) — entry-verifiable, registration
-- proceeds (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for "boss_ionar"
-- hits boss_ionar.cpp + the loader decl/call lines for
-- AddSC_boss_ionar() only; whole-server-tree grep for
-- "npc_spark_of_ionar" hits boss_ionar.cpp only; zero sql/ hits
-- for either. No ionar lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- instance->SetBossState(DATA_IONAR, IN_PROGRESS) leg has no
-- instance bridge — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 2) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — seventeenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 3) (event 4; the lSparkList.DespawnAll()
-- and instance->SetBossState(DATA_IONAR, DONE) legs have no
-- bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28546):
-- boss_ionar Reset / Initialize — lSparkList.DespawnAll() +
-- Initialize() flag/timer clears, RemoveFlag / SetVisible legs,
-- instance->SetBossState(DATA_IONAR, NOT_STARTED) — no summon-list /
-- flag / visibility / instance bridges. The SetControlled(false,
-- UNIT_STATE_ROOT) leg ports via creature:SetRooted(false)
-- (lua_creature_events.go) in ionarCombatEnd.
-- SpellHit — the SPELL_DISPERSE 52770 split machine (five
-- SPELL_SUMMON_SPARK 52746 casts, AttackStop, SetVisible(false),
-- NON_ATTACKABLE+NOT_SELECTABLE flags, UNIT_STATE_ROOT, motion
-- clear/idle) — no cast / visibility / flag / motion bridges.
-- CallBackSparks — ObjectAccessor GUID loop, SetSpeedRate(MOVE_RUN,
-- 2.0f), MovePoint(DATA_POINT_CALLBACK 0) per alive spark,
-- DespawnOrUnsummon on dead — no ObjectAccessor / motion bridges.
-- DamageTaken — uiDamage = 0 when !me->IsVisible() — the event-9
-- bridge is live; the arm stays out on the missing visibility bridge
-- (no IsVisible in the Lua surface).
-- JustSummoned — NPC_SPARK_OF_IONAR 28926 SummonList latch +
-- SPELL_SPARK_VISUAL_TRIGGER 52667 self-cast + random-target
-- SetInCombatWith + MoveFollow — no summon-list / cast /
-- target-selection / motion bridges.
-- SummonedCreatureDespawn — lSparkList.Despawn latch — no bridge.
-- UpdateAI split machine (uiSplitTimer 25s / 2.5s; bIsSplitPhase
-- CallBackSparks leg; lSparkList.empty() restore leg with
-- SetVisible(true), flag removal, SPELL_SPARK_DESPAWN 52776
-- self-cast, MoveChase) — no timer / visibility / flag / cast /
-- motion bridges.
-- UpdateAI timer events (boss_ionar.cpp:260-275) — PORTED:
-- uiBallLightningTimer DoCastVictim(SPELL_BALL_LIGHTNING 52780)
-- non-triggered, urand(10s, 11s) init -> urand(10s, 11s) loop
-- — PORTED as CreateLuaEvent {10000, 11000} + CastSpell(nil,
-- 52780), cancel on 2/4/23 (the stale "no timer-event / cast
-- bridges" claim is retracted for this arm).
-- uiStaticOverloadTimer — PORTED: DoCast(target,
-- SPELL_STATIC_OVERLOAD 52658), 2-arg non-triggered, urand(5s,6s)
-- init -> urand(5s,6s) re-arm (boss_ionar.cpp:260-266), cast only
-- when SelectTarget(Random, 0) (boss_ionar.cpp:261-263) finds a
-- target, re-arm unconditional (the rage_winterchill precedent).
-- Random target is a plain pick over the bridged threat list
-- GetAITargets (midnight precedent) — C++ passes no playerOnly flag,
-- so pets/totems qualify (playerOnly=false default, UnitAI.h:170);
-- the stale player-only deviation note is retracted.
-- UpdateAI disperse health check — !bHasDispersed &&
-- HealthBelowPct(uiDisperseHealth) (45 + urand(0,10)):
-- Talk(SAY_SPLIT 1) + InterruptNonMeleeSpells +
-- DoCast(me, SPELL_DISPERSE 52770) — Talk(1) rides the unmodeled
-- machine; firing it without the split would fabricate context. NOTE:
-- creature:GetHealthPct IS bridged (object.go:107 fallback, sindragosa
-- precedent) — the arm is blocked by the split/disperse MACHINE
-- (gothik rule), not the health check.
-- npc_spark_of_ionar (NPC_SPARK_OF_IONAR 28926, local enum in
-- boss_ionar.cpp — the acolyte_of_shadron / stormforged_lieutenant
-- local-enum precedent) — entry-verifiable but bridge-blocked, no
-- registration (the stormforged_lieutenant no-bridgeable-arms
-- precedent): Reset (REACT_PASSIVE — no react bridge);
-- MovementInform POINT_MOTION_TYPE / DATA_POINT_CALLBACK
-- DespawnOrUnsummon — no motion bridge; DamageTaken uiDamage = 0 —
-- REGISTERED 2026-10-19 (event 9, return false, 0 — halazzi
-- precedent; the handler is inert until the spark summon machine is
-- ported — trigger-path grounds); UpdateAI boss-state-gated
-- DespawnOrUnsummon + uiCheckTimer distance-to-Ionar
-- (> DATA_MAX_SPARK_DISTANCE 90) MovePoint callback machine via
-- instance->GetGuidData(DATA_IONAR) — no timer / instance / motion
-- / distance bridges. No Talk arms anywhere in the script.

local ENTRY_IONAR = 28546
local ENTRY_SPARK_OF_IONAR = 28926

local SAY_AGGRO = 0
local SAY_SLAY = 2
local SAY_DEATH = 3

local SPELL_BALL_LIGHTNING = 52780
local SPELL_STATIC_OVERLOAD = 52658

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — playerOnly=false
-- default (UnitAI.h:170), so the pool is the whole threat list
-- (pets/totems included): plain pick over GetAITargets (the bridged
-- threat list; midnight precedent), no Player filter.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- C++ uiBallLightningTimer (:270-274): DoCastVictim(52780)
-- non-triggered, urand(10s, 11s) init -> urand(10s, 11s) loop.
local function ballLightningTick(creature, guid)
    creature:CastSpell(nil, SPELL_BALL_LIGHTNING)
    schedule(guid, "ballLightning", {10000, 11000}, function()
        ballLightningTick(creature, guid)
    end)
end

-- C++ uiStaticOverloadTimer (:260-266): if (Unit* target =
-- SelectTarget(Random, 0)) DoCast(target, SPELL_STATIC_OVERLOAD),
-- 2-arg non-triggered; urand(5s,6s) init / unconditional
-- urand(5s,6s) re-arm.
local function staticOverloadTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_STATIC_OVERLOAD)
    end
    schedule(guid, "staticOverload", {5000, 6000}, function()
        staticOverloadTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) +
-- instance->SetBossState(DATA_IONAR, IN_PROGRESS) — only the Talk
-- arm is bridgeable.
local function ionarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "ballLightning", {10000, 11000}, function()
        ballLightningTick(creature, guid)
    end)
    schedule(guid, "staticOverload", {5000, 6000}, function()
        staticOverloadTick(creature, guid)
    end)
end

local function ionarCombatEnd(event, creature)
    -- C++ Reset: me->SetControlled(false, UNIT_STATE_ROOT)
    -- (boss_ionar.cpp:119); both ev2 (EnterEvadeMode -> Reset) and ev23
    -- (Reset) clear the root state.
    creature:SetRooted(false)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function ionarTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + lSparkList.DespawnAll() +
-- instance->SetBossState(DATA_IONAR, DONE) — only the Talk arm is
-- bridgeable.
local function ionarDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_IONAR, 1, ionarEnterCombat)
RegisterCreatureEvent(ENTRY_IONAR, 2, ionarCombatEnd)
RegisterCreatureEvent(ENTRY_IONAR, 3, ionarTargetDied)
RegisterCreatureEvent(ENTRY_IONAR, 4, ionarDied)
RegisterCreatureEvent(ENTRY_IONAR, 23, ionarCombatEnd)

-- C++ npc_spark_of_ionar::DamageTaken (boss_ionar.cpp:339-342):
-- uiDamage = 0 unconditionally. Event 9's second return rewrites
-- damage (fireCreatureDamageTaken; halazzi precedent). Inert today:
-- sparks spawn from the unported summon machine.
local function sparkDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_SPARK_OF_IONAR, 9, sparkDamageTaken)
