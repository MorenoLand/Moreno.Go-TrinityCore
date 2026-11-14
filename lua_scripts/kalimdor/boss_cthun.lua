-- Temple of Ahn'Qiraj: C'Thun body — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (cthunAI : public BossAI; GetAI via GetAQ40AI<cthunAI>(creature);
-- registered in AddSC_boss_cthun()).
-- Entry: 15727 — NOT a C++ constant (temple_of_ahnqiraj.h names only
-- the eye 15589, portal 15896, tentacles and body-of-cthun 15809);
-- verified in TDB ('C'Thun', ScriptName boss_cthun, VerifiedBuild
-- 12340) — the wowhead-only justification is superseded.
-- Ported: the out-of-combat whisper arm (boss_cthun.cpp:585-596).
-- C++ UpdateAI runs it only on the no-victim branch: while the body
-- has no victim (before pull, and throughout the eye phase, when
-- players fight the eye instead) it plays RANDOM_SOUND_WHISPER 8663
-- to every player in the map on a 90s init -> {90s, 300s} re-arm
-- (the Yells comment notes the client picks the whisper variant).
-- The phase machine it sits inside stays out (below).
-- Lua shape: spawn/reset-started pump (event 5 spawn fires after
-- event 23 reset, so the start cancels first and is idempotent;
-- felmyst/muru spawn-pump precedent). Each tick checks
-- IsInCombat (lua_creature_events.go:652): while the body has a
-- victim the C++ timer freezes, so Lua polls every 5s without
-- playing and resumes the long re-arm once combat drops — the
-- only deviation, documented here, since a Lua timer cannot freeze
-- mid-flight. PlayDirectSound broadcasts with a nil source
-- (lua_objects.go:168, generic object method); C++ plays per-player to the map,
-- so the audible set is broadcast-nearby rather than map-wide.
-- Cancelled on death (event 4); restarted on reset/evade (event
-- 23, matching C++ Reset re-initialising the 90s timer).
-- Stay-outs (all instance DATA_CTHUN_PHASE-gated, no instance
-- bridge): Reset flag/visible/instance legs (:533-544), the
-- TRANSITION/STOMACH/WEAK switch with transform cast, full-health
-- set, threat-list stomach map build and flesh-tentacle spawns
-- (:622-665), stomach acid/enter/knockback/teleport legs, giant
-- tentacle spawn timers, weaken Talk + purple coloration + kick-out
-- (:675-720), DamageTaken 99% reduction / death prevention
-- (:843-867, re-triaged round 14: STAY OUT — both DamageTaken arms
-- read instance->GetData(DATA_CTHUN_PHASE); the bridged
-- creature:GetData is the per-AI ScriptData store
-- (lua_creature_events.go:800-835), NOT the instance store, and no
-- instance GetData read bridge exists (a Lua-side InstanceData:SetData
-- write bridge exists at instance_hooks.go:51, but there is no AQ40
-- Lua instance script to consume it); re-checked round 23:
-- the global GetInstanceData (scripting/globals.go:1442) returns a
-- Lua-side per-(map,instance) table populated only by Lua instance
-- scripts — the C++ InstanceScript store is not mirrored into it,
-- and the AQ40 instance script is unported (boss_ai.go:18360-18411 audit,
-- no-instance-script-bridge queue), so the C++ phase writes at
-- boss_cthun.cpp:216/227/307/367/429/529/627/671/815/840 never reach
-- Lua; the phase writers are all in the unported phase machine), JustDied PHASE_DONE SetData (:838-842), DoAction
-- FLESH_TENTACLE_KILLED counter (:869-877, consumer of the flesh
-- tentacle JustDied relay — DoAction IS bridged, but the counter
-- lives in the unported phase machine, so the relay stays out).
-- SetBossState legs (tick 278): ev1 IN_PROGRESS + ev23 NOT_STARTED
-- PORTED below. NO ev4 DONE leg: cthunAI::JustDied (boss_cthun.cpp:
-- 838-842) overrides WITHOUT calling _JustDied()/BossAI::JustDied()
-- — it only sets DATA_CTHUN_PHASE (no instance bridge) — so base
-- DONE never runs. The inverse of the rajaxx correction: no override
-- = base runs; an override that omits _JustDied = base does NOT run.
local DATA_CTHUN = 5 -- temple_of_ahnqiraj.h:35
local ENTRY = 15727
local SPELL_TRANSFORM = 26232
local SOUND_WHISPER = 8663

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

local function onWhisper(creature, guid)
    local per = timers[guid]
    if not per then
        return
    end
    if creature:IsInCombat() then
        -- C++ cthunAI::UpdateAI (boss_cthun.cpp:596): unconditional
        -- SetTarget(Empty) on every combat tick, before the phase
        -- switch (so the STOMACH-case clear at :666 is subsumed);
        -- 5s poll granularity here, kaelthas.lua:308 empty-clear form.
        creature:SetTarget()
        per["whisper"] = CreateLuaEvent(function()
            onWhisper(creature, guid)
        end, 5000)
        return
    end
    creature:PlayDirectSound(SOUND_WHISPER)
    per["whisper"] = CreateLuaEvent(function()
        onWhisper(creature, guid)
    end, {90000, 300000})
end

local function startWhispers(event, creature)
    local guid = creature:GetGUIDLow()
    -- C++ Reset _Reset() leg (boss_cthun.cpp:516-519): alive-gated and
    -- != DONE-gated (ScriptedCreature.cpp:499-511); ev23 only — the
    -- ev5 spawn pass shares this handler, and a spawn write would
    -- clobber a saved DONE state.
    if event == 23 and creature:IsAlive() and creature:GetBossState(DATA_CTHUN) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_CTHUN, BOSS_STATE_NOT_STARTED)
    end
    -- C++ boss_cthunAI::Reset (boss_cthun.cpp:525): unconditional
    -- RemoveAurasDueToSpell(SPELL_TRANSFORM); the flag/visible/instance
    -- legs of the same Reset stay out (no instance bridge).
    creature:RemoveAura(SPELL_TRANSFORM)
    cancelTimers(guid)
    timers[guid] = {}
    timers[guid]["whisper"] = CreateLuaEvent(function()
        onWhisper(creature, guid)
    end, 90000)
end

local function onEnterCombat(event, creature, target)
    -- C++ cthunAI has no JustEngagedWith override (boss_cthun.cpp:462-890) —
    -- base BossAI::JustEngagedWith writes SetBossState(DATA_CTHUN,
    -- IN_PROGRESS) (ScriptedCreature.cpp:499-542; creature:SetBossState,
    -- lua_creature_events.go:7099).
    creature:SetBossState(DATA_CTHUN, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

-- C++ cthunAI::JustDied (boss_cthun.cpp:838-842) sets only
-- DATA_CTHUN_PHASE and never calls _JustDied(): NO ev4 DONE leg.
local function onDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY, 5, startWhispers)
RegisterCreatureEvent(ENTRY, 23, startWhispers)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
