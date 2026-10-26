-- Tharon'ja (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_tharon_ja.cpp
-- (boss_tharon_ja (BossAI), spell_tharon_ja_clear_gift_of_tharon_ja
-- (SpellScript); AddSC_boss_tharon_ja at end registers all via
-- GetDrakTharonKeepAI / RegisterSpellScript). The fourth
-- Drak'Tharon Keep group in northrend_script_loader.cpp order
-- (decl 42 / call 237, immediately after AddSC_boss_king_dred();
-- instance_drak_tharon_keep follows at decl 43 / call 238).
-- Entry: 26632 Tharon'ja (drak_tharon_keep.h NPC_THARON_JA —
-- kalecgos pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_tharon_ja"
-- and "spell_tharon_ja_clear_gift_of_tharon_ja" hits
-- boss_tharon_ja.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- JustEngagedWith/KilledUnit/JustDied hooks fire unconditionally
-- (the phase machine never gates them), so the three Talk arms are
-- C++-exact.
-- Timer doctrine (Oct-17 re-sweep): the phase machine is a pure
-- self-driven timer chain (DECAY_FLESH -> GOING_FLESH ->
-- RETURN_FLESH -> GOING_SKELETAL); every driver leg is a
-- bridgeable cast/Talk, so gated emulation is C++-exact — this is
-- NOT the gothik case (gothik's driver is an unbridged wave
-- machine). events.Reset() is emulated by canceling all GUID-keyed
-- timers then re-scheduling the new phase's arms (nightbane idiom).
-- Accepted deviations (moroes/nightbane idiom): the
-- HasUnitState(UNIT_STATE_CASTING) skips (~:127, :193) have no
-- bridge; the C++ one-event-per-tick UpdateAI loop is not
-- replicated — Lua timers fire independently, like every
-- CreateLuaEvent port.
-- Ported arms (all C++-exact):
-- JustEngagedWith Talk(SAY_AGGRO 0) + phase-1 schedule
-- (decay_flesh 20s, curse_of_life 1s, rain_of_fire {14s,18s},
-- shadow_volley {8s,10s}); KilledUnit player-gated Talk(SAY_KILL 1)
-- (event 3, nalorakk precedent); JustDied Talk(SAY_DEATH 4) +
-- DoCastAOE(CLEAR_GIFT_OF_THARON_JA 53242, true) + DoCastAOE(
-- SPELL_ACHIEVEMENT_CHECK 61863, true) (event 4 — cast arms bridge
-- independently of the unbridged SpellScript bindings, razelikh
-- precedent; both inert in Go until SpellScripts bridge).
-- Phase-1 rotation: EVENT_CURSE_OF_LIFE — random player target
-- (SelectTarget(Random,0,100.0f,true) — threat-list player pick via GetAITargets)
-- -> DoCast(49527), 1s init, {10s,15s} re-arm sitting outside the
-- target check (C++-exact); EVENT_RAIN_OF_FIRE — random player
-- target -> DoCast(49518), {14s,18s} init, {14s,18s} re-arm outside
-- the target check; EVENT_SHADOW_VOLLEY — DoCastVictim(49528)
-- (CastSpell(nil, 49528) — moroes precedent), {8s,10s} init,
-- {8s,10s} re-arm; EVENT_DECAY_FLESH — DoCastAOE(49356) non-
-- triggered as self-cast (selin_fireheart/mr_smite DoCastAOE
-- convention), 20s init, schedules GOING_FLESH +6s (no self
-- re-arm, C++-exact).
-- EVENT_GOING_FLESH — Talk(SAY_FLESH 2) + DoCastAOE(GIFT_OF_
-- THARON_JA 52509, true) + DoCast(me, FLESH_VISUAL 52582, true) +
-- DoCast(me, DUMMY 49551, true) as self-casts, then events.Reset()
-- emulation + phase-2 schedule (return_flesh 20s, lightning_breath
-- {3s,4s}, eye_beam {4s,8s}, poison_cloud {6s,7s}).
-- Phase-2 rotation: EVENT_LIGHTNING_BREATH — random player target
-- -> DoCast(49537), {3s,4s} init, {6s,7s} re-arm outside the target
-- check; EVENT_EYE_BEAM — random player target -> DoCast(49544),
-- {4s,8s} init, {4s,6s} re-arm outside the target check;
-- EVENT_POISON_CLOUD — DoCastAOE(49548) non-triggered self-cast,
-- {6s,7s} init, {10s,12s} re-arm.
-- EVENT_RETURN_FLESH — DoCastAOE(53463) non-triggered self-cast,
-- schedules GOING_SKELETAL +6s (no self re-arm, C++-exact);
-- EVENT_GOING_SKELETAL — Talk(SAY_SKELETON 3) + DoCastAOE(CLEAR_
-- GIFT_OF_THARON_JA 53242, true) self-cast, then events.Reset()
-- emulation + phase-1 re-arm (decay_flesh 20s, curse_of_life 1s,
-- rain_of_fire {14s,18s}, shadow_volley {8s,10s}).
-- Unmodeled (no bridges — documented, not wired):
-- Reset's _Reset() — instance bookkeeping, no bridge; UpdateAI
-- HasUnitState skips — no live unit-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) (moroes precedent); spell_tharon_ja_clear_gift_of_
-- tharon_ja SpellScript (53242 EFFECT_0 SCRIPT_EFFECT ->
-- RemoveAura(52509)) — no SpellScript binding bridge (razelikh
-- precedent); achievement criteria script — no achievement bridge
-- (kelthuzad precedent).
-- C++ "Known Issues" note carried: spells 49356 (Decay Flesh) and
-- 53463 (Return Flesh) will be interrupted for an unknown reason.

local ENTRY_THARON_JA = 26632

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_FLESH = 2
local SAY_SKELETON = 3
local SAY_DEATH = 4

local SPELL_CURSE_OF_LIFE = 49527
local SPELL_RAIN_OF_FIRE = 49518
local SPELL_SHADOW_VOLLEY = 49528
local SPELL_DECAY_FLESH = 49356
local SPELL_GIFT_OF_THARON_JA = 52509
local SPELL_CLEAR_GIFT_OF_THARON_JA = 53242
local SPELL_EYE_BEAM = 49544
local SPELL_LIGHTNING_BREATH = 49537
local SPELL_POISON_CLOUD = 49548
local SPELL_RETURN_FLESH = 53463
local SPELL_ACHIEVEMENT_CHECK = 61863
local SPELL_FLESH_VISUAL = 52582
local SPELL_DUMMY = 49551

-- MODEL_FLESH: boss_tharon_ja.cpp enum (line ~75), the flesh-form
-- display for EVENT_GOING_FLESH's SetDisplayId arm.
local MODEL_FLESH = 27073

-- GUID-keyed native-display table: captured via creature:GetDisplayId()
-- at engage; implements me->RestoreDisplayId() as
-- creature:SetDisplayId(nativeDisplay[guid]).
local nativeDisplay = {}

-- GUID-keyed timer table (nightbane idiom); events.Reset() in the
-- C++ phase machine is emulated by canceling every key then
-- re-scheduling the new phase's arms.
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

-- C++ SelectTarget(Random, 0, 100.0f, true): random alive threat-list
-- player within 100 yards (UnitAI.h:162-184; playerOnly=true draws
-- the threat list, not instance players). Pool corrected 2026-10-25:
-- was GetPlayersInWorld (instance-wide, distance-checked) — the same
-- stale-pool class fixed in the Violet Hold strip. Alive filter: C++
-- SelectTargetList skips offline refs and dead targets are offline
-- (Creature.cpp:2508-2524); Go SortedEntries returns dead entries too.
local function randomThreatPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Forward declarations: the phase chain is cyclic
-- (decayFlesh -> goingFlesh -> returnFlesh -> goingSkeletal ->
-- decayFlesh), so the four transition ticks are declared here and
-- assigned below (gal_darah transformTick idiom).
local decayFleshTick, goingFleshTick, returnFleshTick, goingSkeletalTick

-- C++ EVENT_CURSE_OF_LIFE: random player target -> DoCast(49527),
-- re-arm {10s,15s} sits outside the target check (C++-exact).
local function curseOfLifeTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_LIFE)
    end
    schedule(guid, "curseoflife", {10000, 15000}, function()
        curseOfLifeTick(creature, guid)
    end)
end

-- C++ EVENT_RAIN_OF_FIRE: random player target -> DoCast(49518),
-- re-arm {14s,18s} sits outside the target check (C++-exact).
local function rainOfFireTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_RAIN_OF_FIRE)
    end
    schedule(guid, "rainoffire", {14000, 18000}, function()
        rainOfFireTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_VOLLEY: DoCastVictim(49528) (CastSpell(nil, ...)
-- = victim-cast, moroes precedent), {8s,10s} re-arm.
local function shadowVolleyTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_VOLLEY)
    schedule(guid, "shadowvolley", {8000, 10000}, function()
        shadowVolleyTick(creature, guid)
    end)
end

-- C++ EVENT_POISON_CLOUD: DoCastAOE(49548) non-triggered as
-- self-cast (selin_fireheart/mr_smite convention), {10s,12s} re-arm.
local function poisonCloudTick(creature, guid)
    creature:CastSpell(creature, SPELL_POISON_CLOUD)
    schedule(guid, "poisoncloud", {10000, 12000}, function()
        poisonCloudTick(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNING_BREATH: random player target -> DoCast(49537),
-- re-arm {6s,7s} sits outside the target check (C++-exact).
local function lightningBreathTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_LIGHTNING_BREATH)
    end
    schedule(guid, "lightningbreath", {6000, 7000}, function()
        lightningBreathTick(creature, guid)
    end)
end

-- C++ EVENT_EYE_BEAM: random player target -> DoCast(49544),
-- re-arm {4s,6s} sits outside the target check (C++-exact).
local function eyeBeamTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_EYE_BEAM)
    end
    schedule(guid, "eyebeam", {4000, 6000}, function()
        eyeBeamTick(creature, guid)
    end)
end

-- Phase-1 schedule helper (C++ JustEngagedWith / GOING_SKELETAL
-- arms: decay_flesh 20s, curse_of_life 1s, rain_of_fire {14s,18s},
-- shadow_volley {8s,10s}).
local function schedulePhaseOne(creature, guid)
    schedule(guid, "decayflesh", 20000, function()
        decayFleshTick(creature, guid)
    end)
    schedule(guid, "curseoflife", 1000, function()
        curseOfLifeTick(creature, guid)
    end)
    schedule(guid, "rainoffire", {14000, 18000}, function()
        rainOfFireTick(creature, guid)
    end)
    schedule(guid, "shadowvolley", {8000, 10000}, function()
        shadowVolleyTick(creature, guid)
    end)
end

-- Phase-2 schedule helper (C++ GOING_FLESH arms: return_flesh 20s,
-- lightning_breath {3s,4s}, eye_beam {4s,8s}, poison_cloud {6s,7s}).
local function schedulePhaseTwo(creature, guid)
    schedule(guid, "returnflesh", 20000, function()
        returnFleshTick(creature, guid)
    end)
    schedule(guid, "lightningbreath", {3000, 4000}, function()
        lightningBreathTick(creature, guid)
    end)
    schedule(guid, "eyebeam", {4000, 8000}, function()
        eyeBeamTick(creature, guid)
    end)
    schedule(guid, "poisoncloud", {6000, 7000}, function()
        poisonCloudTick(creature, guid)
    end)
end

-- C++ EVENT_DECAY_FLESH: DoCastAOE(49356) non-triggered self-cast,
-- schedules EVENT_GOING_FLESH +6s (no self re-arm, C++-exact).
decayFleshTick = function(creature, guid)
    creature:CastSpell(creature, SPELL_DECAY_FLESH)
    schedule(guid, "goingflesh", 6000, function()
        goingFleshTick(creature, guid)
    end)
end

-- C++ EVENT_GOING_FLESH: Talk(2) + SetDisplayId(MODEL_FLESH) +
-- triggered self-casts + events.Reset() emulation + phase-2 schedule.
goingFleshTick = function(creature, guid)
    creature:Talk(SAY_FLESH)
    creature:SetDisplayId(MODEL_FLESH)
    creature:CastSpell(creature, SPELL_GIFT_OF_THARON_JA, true)
    creature:CastSpell(creature, SPELL_FLESH_VISUAL, true)
    creature:CastSpell(creature, SPELL_DUMMY, true)
    cancelTimers(guid)
    schedulePhaseTwo(creature, guid)
end

-- C++ EVENT_RETURN_FLESH: DoCastAOE(53463) non-triggered self-cast,
-- schedules EVENT_GOING_SKELETAL +6s (no self re-arm, C++-exact).
returnFleshTick = function(creature, guid)
    creature:CastSpell(creature, SPELL_RETURN_FLESH)
    schedule(guid, "goingskeletal", 6000, function()
        goingSkeletalTick(creature, guid)
    end)
end

-- C++ EVENT_GOING_SKELETAL: Talk(3) + me->RestoreDisplayId() (as
-- SetDisplayId(nativeDisplay[guid])) + DoCastAOE(53242, true)
-- self-cast + events.Reset() emulation + phase-1 re-arm.
goingSkeletalTick = function(creature, guid)
    creature:Talk(SAY_SKELETON)
    if nativeDisplay[guid] then
        creature:SetDisplayId(nativeDisplay[guid])
    end
    creature:CastSpell(creature, SPELL_CLEAR_GIFT_OF_THARON_JA, true)
    cancelTimers(guid)
    schedulePhaseOne(creature, guid)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO 0) (the BossAI::JustEngagedWith
-- leg is instance bookkeeping, no bridge) + phase-1 schedule.
local function tharonJaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    nativeDisplay[guid] = creature:GetDisplayId()
    cancelTimers(guid)
    schedulePhaseOne(creature, guid)
end

local function tharonJaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL 1) only when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk precedent).
local function tharonJaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 4) (the _JustDied leg is instance
-- bookkeeping, no bridge) + the two DoCastAOE arms as triggered
-- self-casts.
local function tharonJaDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    creature:CastSpell(creature, SPELL_CLEAR_GIFT_OF_THARON_JA, true)
    creature:CastSpell(creature, SPELL_ACHIEVEMENT_CHECK, true)
end

-- C++ Reset: _Reset() (instance bookkeeping, no bridge) +
-- me->RestoreDisplayId(); the event-table reset is the timer cancel.
local function tharonJaReset(event, creature)
    local guid = creature:GetGUID()
    if nativeDisplay[guid] then
        creature:SetDisplayId(nativeDisplay[guid])
        nativeDisplay[guid] = nil
    end
    cancelTimers(guid)
end

RegisterCreatureEvent(ENTRY_THARON_JA, 1, tharonJaEnterCombat)
RegisterCreatureEvent(ENTRY_THARON_JA, 2, tharonJaLeaveCombat)
RegisterCreatureEvent(ENTRY_THARON_JA, 3, tharonJaTargetDied)
RegisterCreatureEvent(ENTRY_THARON_JA, 4, tharonJaDied)
RegisterCreatureEvent(ENTRY_THARON_JA, 23, tharonJaReset)
