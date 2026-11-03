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
-- Initialize() flag/timer clears, RemoveFlag legs (PORTED in
-- ionarCombatEnd — the field-59 flag surface is bridged),
-- instance->SetBossState(DATA_IONAR, NOT_STARTED) — no summon-list /
-- instance bridges; the flag SET side is PORTED in ionarSplit. The SetControlled(false, UNIT_STATE_ROOT)
-- leg ports via creature:SetRooted(false) (lua_creature_events.go)
-- and the if (!IsVisible) SetVisible(true) leg via creature:SetVisible,
-- both in ionarCombatEnd (C++-exact arms).
-- SpellHit — PORTED 2026-11-03 as the disperse health-poll
-- trigger: the poll casts SPELL_DISPERSE 52770 and runs the split
-- arms inline (five summons + AttackStop + invisible + flags +
-- root + MoveIdle); no ev14 relay needed (self-cast trigger
-- collapsed into the poll — same arm order as C++).
-- CallBackSparks — PORTED 2026-11-03 (ionarCallbackSparks):
-- 25s split timer, alive sparks SetSpeedRate(1, 2.0) + MoveTo(0,
-- boss pos) with returning flag, dead sparks Despawned; tracked
-- via SummonCreature return values, not ObjectAccessor.
-- DamageTaken — PORTED 2026-11-03 (event 9): uiDamage = 0
-- while the own-state split flag is set (no IsVisible bridge —
-- Lua knows it set invisible); un-split hits pass through.
-- JustSummoned — PORTED 2026-11-03 inline at summon:
-- SPELL_SPARK_VISUAL_TRIGGER 52667 self-cast + random-threat
-- MoveFollow; SetInCombatWith has no bridge (MoveFollow carries
-- the follow leg); no ev19 registration (would double-apply).
-- SummonedCreatureDespawn — set maintained by the spark watch
-- (arrival despawn) + Ionar death/evade handlers; no ev20
-- registration needed.
-- UpdateAI split machine — PORTED 2026-11-03 (25s callback /
-- 2.5s restore poll; restore = visible + flag clear + root clear
-- + SPELL_SPARK_DESPAWN 52776 + MoveChase victim; ball/static
-- timers cancel at split, re-seed fresh on restore — C++ freezes
-- remaining time, documented delta).
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
-- UpdateAI disperse health check — PORTED 2026-11-03 (500ms
-- poll; HealthBelowPct = health < floor(max*pct/100) per Unit.h:872
-- + CalculatePct truncation; roll 45+rand(0,10) at engage):
-- Talk(SAY_SPLIT 1) + InterruptNonMeleeSpells + DoCast(me,
-- SPELL_DISPERSE 52770) non-triggered, then the split arms.
-- npc_spark_of_ionar (NPC_SPARK_OF_IONAR 28926, local enum in
-- boss_ionar.cpp — the acolyte_of_shadron / stormforged_lieutenant
-- local-enum precedent) — entry-verifiable but bridge-blocked, no
-- registration (the stormforged_lieutenant no-bridgeable-arms
-- precedent): Reset (REACT_PASSIVE) — PORTED on event 23 (sparkReset;
-- the summon / movement machines stay out, same inert-until-summon
-- standing as the registered DamageTaken arm below);
-- MovementInform POINT_MOTION_TYPE / DATA_POINT_CALLBACK
-- DespawnOrUnsummon — no motion hook (no event 28); arrival
-- despawn is the spark-watch returning-flag poll (2s, 5yd),
-- PORTED 2026-11-03; DamageTaken uiDamage = 0 stays REGISTERED
-- (event 9, return false, 0 — halazzi precedent); UpdateAI >90
-- redirect PORTED in the watch, boss-state gate has no instance
-- bridge (Ionar death/evade despawn covers encounter end).
-- No Talk arms anywhere in the script.

local ENTRY_IONAR = 28546
local ENTRY_SPARK_OF_IONAR = 28926

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local SAY_AGGRO = 0
local SAY_SLAY = 2
local SAY_DEATH = 3
local SAY_SPLIT = 1

local SPELL_DISPERSE = 52770
local SPELL_SPARK_VISUAL_TRIGGER = 52667
local SPELL_SPARK_DESPAWN = 52776

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
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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

-- Split machine (boss_ionar.cpp SpellHit / CallBackSparks / UpdateAI
-- invisible branch / spark UpdateAI, PORTED 2026-11-03; the old
-- "no summon-list / motion / visibility bridge" stay-out notes are
-- retracted — SummonCreature returns the summoned object (so Lua
-- tracks spark GUIDs directly), MoveFollow/MoveTo/MoveChase/
-- SetSpeedRate/AttackStop/SetFlag/SetRooted/SetVisible/Despawn are
-- bridged, and the invisible DamageTaken gate rides this own-state
-- flag (no IsVisible bridge exists, but Lua knows it set
-- invisible). Deltas: (a) SetInCombatWith has no bridge — MoveFollow
-- carries the follow leg; (b) MovementInform has no hook (no event
-- 28) — arrival despawn is a 2s poll on the returning flag
-- (callback or >90 redirect) with a 5yd threshold; (c) ball/static
-- timers freeze in C++ while invisible — Lua cancels them at split
-- and re-seeds fresh init ranges on restore; (d) the disperse
-- health check is a 500ms poll, not a per-tick check; (e) sparks
-- summon as type 7 MANUAL (no timed despawn exists in C++ — every
-- C++ despawn leg is explicit here); (f) the spark boss-state gate
-- has no instance bridge — Ionar death/evade despawn covers the
-- encounter-end leg.
local ionarState = {}

local function ionarDespawnSparks(state)
    if not state then
        return
    end
    for sguid, entry in pairs(state.sparks) do
        entry.obj:Despawn()
        state.sparks[sguid] = nil
    end
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local ionarSplit, ionarCallbackSparks, ionarRestoreCheck, ionarSparkWatch, ionarHealthPoll

ionarHealthPoll = function(creature, guid)
    local state = ionarState[guid]
    if not state or timers[guid] == nil then
        return
    end
    if state.split then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * state.disperseHealth / 100) then
        creature:Talk(SAY_SPLIT)
        creature:InterruptNonMeleeSpells()
        creature:CastSpell(creature, SPELL_DISPERSE)
        ionarSplit(creature, guid, state)
        return
    end
    schedule(guid, "healthPoll", 500, function()
        ionarHealthPoll(creature, guid)
    end)
end

ionarSplit = function(creature, guid, state)
    state.split = true
    state.splitPhase = true
    cancelKey(guid, "ballLightning")
    cancelKey(guid, "staticOverload")
    cancelKey(guid, "healthPoll")
    for _ = 1, 5 do
        local spark = creature:SummonCreature(ENTRY_SPARK_OF_IONAR, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
        if spark then
            spark:CastSpell(spark, SPELL_SPARK_VISUAL_TRIGGER, true)
            local target = randomThreatTarget(creature)
            if target then
                spark:MoveFollow(target, 0, 0)
            end
            state.sparks[spark:GetGUID()] = { obj = spark, returning = false }
        end
    end
    creature:AttackStop()
    creature:SetVisible(false)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:MoveIdle()
    schedule(guid, "splitCallback", 25000, function()
        ionarCallbackSparks(creature, guid)
    end)
    schedule(guid, "sparkWatch", 2000, function()
        ionarSparkWatch(creature, guid)
    end)
end

ionarCallbackSparks = function(creature, guid)
    local state = ionarState[guid]
    if not state or timers[guid] == nil then
        return
    end
    for sguid, entry in pairs(state.sparks) do
        if entry.obj:IsDead() then
            entry.obj:Despawn()
            state.sparks[sguid] = nil
        else
            entry.obj:SetSpeedRate(1, 2.0)
            entry.obj:MoveTo(0, creature:GetX(), creature:GetY(), creature:GetZ())
            entry.returning = true
        end
    end
    state.splitPhase = false
    schedule(guid, "splitRestore", 2500, function()
        ionarRestoreCheck(creature, guid)
    end)
end

ionarRestoreCheck = function(creature, guid)
    local state = ionarState[guid]
    if not state or timers[guid] == nil then
        return
    end
    if next(state.sparks) == nil then
        creature:SetVisible(true)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        creature:SetRooted(false)
        creature:CastSpell(creature, SPELL_SPARK_DESPAWN)
        local victim = creature:GetVictim()
        if victim then
            creature:MoveChase(victim)
        end
        state.split = false
        state.splitPhase = true
        cancelKey(guid, "sparkWatch")
        schedule(guid, "ballLightning", {10000, 11000}, function()
            ballLightningTick(creature, guid)
        end)
        schedule(guid, "staticOverload", {5000, 6000}, function()
            staticOverloadTick(creature, guid)
        end)
        return
    end
    schedule(guid, "splitRestore", 2500, function()
        ionarRestoreCheck(creature, guid)
    end)
end

ionarSparkWatch = function(creature, guid)
    local state = ionarState[guid]
    if not state or timers[guid] == nil then
        return
    end
    for sguid, entry in pairs(state.sparks) do
        if entry.obj:IsDead() then
            entry.obj:Despawn()
            state.sparks[sguid] = nil
        else
            local dist = entry.obj:GetDistance(creature)
            if dist > 90 then
                entry.obj:SetSpeedRate(1, 2.0)
                entry.obj:MoveTo(0, creature:GetX(), creature:GetY(), creature:GetZ())
                entry.returning = true
            elseif entry.returning and dist < 5 then
                entry.obj:Despawn()
                state.sparks[sguid] = nil
            end
        end
    end
    schedule(guid, "sparkWatch", 2000, function()
        ionarSparkWatch(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) +
-- instance->SetBossState(DATA_IONAR, IN_PROGRESS) — only the Talk
-- arm is bridgeable.
local function ionarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ionarState[guid] = { dispersed = false, disperseHealth = 45 + math.random(0, 10), split = false, splitPhase = true, sparks = {} }
    schedule(guid, "ballLightning", {10000, 11000}, function()
        ballLightningTick(creature, guid)
    end)
    schedule(guid, "staticOverload", {5000, 6000}, function()
        staticOverloadTick(creature, guid)
    end)
    schedule(guid, "healthPoll", 500, function()
        ionarHealthPoll(creature, guid)
    end)
end

local function ionarCombatEnd(event, creature)
    -- C++ Reset (boss_ionar.cpp:114-122): lSparkList.DespawnAll() (the
    -- spark summon list is unmodeled) + Initialize() (flag/timer clears,
    -- covered by cancelTimers) + RemoveFlag(NON_ATTACKABLE |
    -- NOT_SELECTABLE) (PORTED below; the set side rides the unported
    -- SpellHit DISPERSE split machine, so the clear is a restore)
    -- + SetControlled(false, UNIT_STATE_ROOT) (SetRooted
    -- bridge, below) + if (!me->IsVisible()) me->SetVisible(true)
    -- (creature:SetVisible bridge, Unit::SetVisible Unit.cpp:8656 —
    -- unconditional here is end-state-identical to the C++ gate).
    -- C++ Reset (:118): RemoveFlag(NON_ATTACKABLE | NOT_SELECTABLE).
    ionarDespawnSparks(ionarState[creature:GetGUID()])
    ionarState[creature:GetGUID()] = nil
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetVisible(true)
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
    ionarDespawnSparks(ionarState[creature:GetGUID()])
    ionarState[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
end

-- C++ boss_ionar::DamageTaken (boss_ionar.cpp:190-194):
-- uiDamage = 0 when !me->IsVisible() — PORTED via the own-state
-- split flag (no IsVisible bridge; Lua knows it set invisible).
local function ionarDamageTaken(event, creature, attacker, damage)
    local state = ionarState[creature:GetGUID()]
    if state and state.split then
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_IONAR, 1, ionarEnterCombat)
RegisterCreatureEvent(ENTRY_IONAR, 2, ionarCombatEnd)
RegisterCreatureEvent(ENTRY_IONAR, 3, ionarTargetDied)
RegisterCreatureEvent(ENTRY_IONAR, 4, ionarDied)
RegisterCreatureEvent(ENTRY_IONAR, 23, ionarCombatEnd)
RegisterCreatureEvent(ENTRY_IONAR, 9, ionarDamageTaken)

-- C++ npc_spark_of_ionar::DamageTaken (boss_ionar.cpp:339-342):
-- uiDamage = 0 unconditionally. Event 9's second return rewrites
-- damage (fireCreatureDamageTaken; halazzi precedent). Inert today:
-- sparks spawn from the unported summon machine.
local function sparkDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_SPARK_OF_IONAR, 9, sparkDamageTaken)
RegisterCreatureEvent(ENTRY_SPARK_OF_IONAR, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_ionar.cpp:344: custom UpdateAI never calls DoMeleeAttackIfReady (C++ comments "No melee attack at all!")
end)

-- C++ npc_spark_of_ionar::Reset (boss_ionar.cpp:322-326): Initialize() +
-- me->SetReactState(REACT_PASSIVE) — the spark never engages on its own
-- (its UpdateAI never attacks); the MovementInform callback leg stays
-- out (no motion bridge).
local function sparkReset(event, creature)
    creature:SetReactState(0) -- REACT_PASSIVE
end

RegisterCreatureEvent(ENTRY_SPARK_OF_IONAR, 23, sparkReset)
