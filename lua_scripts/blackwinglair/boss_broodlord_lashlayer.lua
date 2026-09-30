-- Broodlord Lashlayer (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_broodlord_lashlayer.cpp (boss_broodlordAI); blackwing_lair.h:33
-- (DATA_BROODLORD_LASHLAYER = 2, third boss), :55 (NPC_BROODLORD =
-- 12017). Creature entry: 12017 Broodlord Lashlayer (C++ ScriptName
-- "boss_broodlord" per AddSC_boss_broodlord). The go_suppression_device
-- GameObjectScript from the same C++ file has no Lua bridge (no
-- gameobject model) and is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO=0), arm cleave 26350 8s then every 7s / blast wave
-- 23331 12s then {8s,16s} / mortal strike 24573 20s then {25s,35s} /
-- knockback 25778 30s then {15s,30s}, all triggered DoCastVictim
-- (jeklik convention for C++ DoCastVictim), plus the 1s boundary
-- check. Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied: Talk is not used by C++ — JustDied only runs
-- the suppression-device deactivation (no gameobject bridge). OnLeave
-- Combat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate has no
-- UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (_Reset/BossAI::JustEngagedWith and the
-- DATA_BROODLORD_LASHLAYER bookkeeping arms skipped); the
-- knockback-arm ModifyThreatByPercent(victim, -50) has no bridge (no
-- threat model) — the cast still lands and the 15-30s re-arm is kept;
-- the EVENT_CHECK home-distance > 150 yd leash check and the
-- Talk(SAY_LEASH=1) + EnterEvadeMode(EVADE_REASON_BOUNDARY) arms have
-- no bridges (no home-position/distance bridge, no evade bridge) —
-- only the 1s re-arm cycle is kept, inert (renataki visible-timer
-- convention); the JustDied GetGameObjectListWithEntryInGrid
-- suppression-device scan and the DoAction(ACTION_DEACTIVATE) arms
-- have no gameobject bridge — the devices stay active after the kill;
-- the whole go_suppression_device machine (0-5s first suppression
-- aura 22247 cast, 5s cycle, disarm-deactivate, 30-120s re-activate,
-- DoAction deactivation) is not modeled (standing gameobject gap).

local ENTRY_BROODLORD = 12017

local SAY_AGGRO = 0
-- SAY_LEASH = 1: unmodeled — no evade bridge (see header).

local SPELL_CLEAVE = 26350
local SPELL_BLASTWAVE = 23331
local SPELL_MORTALSTRIKE = 24573
local SPELL_KNOCKBACK = 25778
-- SPELL_SUPPRESSION_AURA = 22247: unmodeled — go_suppression_device
-- has no gameobject bridge.

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

-- C++ EVENT_CLEAVE: triggered DoCastVictim(26350); re-arm 7s.
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE, true)
    end
    schedule(guid, "cleave", 7000, function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_BLASTWAVE: triggered DoCastVictim(23331); re-arm {8s,16s}.
local function onBlastwave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLASTWAVE, true)
    end
    schedule(guid, "blastwave", math.random(8000, 16000), function()
        onBlastwave(creature, guid)
    end)
end

-- C++ EVENT_MORTALSTRIKE: triggered DoCastVictim(24573);
-- re-arm {25s,35s}.
local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALSTRIKE, true)
    end
    schedule(guid, "mortalstrike", math.random(25000, 35000), function()
        onMortalStrike(creature, guid)
    end)
end

-- C++ EVENT_KNOCKBACK: triggered DoCastVictim(25778) + -50% threat on
-- the victim (threat arm has no bridge); re-arm {15s,30s}.
local function onKnockback(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKBACK, true)
    end
    schedule(guid, "knockback", math.random(15000, 30000), function()
        onKnockback(creature, guid)
    end)
end

-- C++ EVENT_CHECK: leash check — distance(home) > 150 yd ->
-- Talk(SAY_LEASH) + EnterEvadeMode. Both the distance query and the
-- evade have no bridge: the 1s cycle is kept, inert.
local function onCheck(creature, guid)
    schedule(guid, "check", 1000, function()
        onCheck(creature, guid)
    end)
end

local function broodlordEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "cleave", 8000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "blastwave", 12000, function()
        onBlastwave(creature, guid)
    end)
    schedule(guid, "mortalstrike", 20000, function()
        onMortalStrike(creature, guid)
    end)
    schedule(guid, "knockback", 30000, function()
        onKnockback(creature, guid)
    end)
    schedule(guid, "check", 1000, function()
        onCheck(creature, guid)
    end)
end

local function broodlordResetState(guid)
    cancelTimers(guid)
end

local function broodlordLeaveCombat(event, creature)
    broodlordResetState(creature:GetGUID())
end

local function broodlordDied(event, creature, killer)
    -- C++ JustDied: the suppression-device scan + DoAction
    -- deactivation has no gameobject bridge (see header).
    broodlordResetState(creature:GetGUID())
end

local function broodlordReset(event, creature)
    broodlordResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BROODLORD, 1, broodlordEnterCombat)
RegisterCreatureEvent(ENTRY_BROODLORD, 2, broodlordLeaveCombat)
RegisterCreatureEvent(ENTRY_BROODLORD, 4, broodlordDied)
RegisterCreatureEvent(ENTRY_BROODLORD, 23, broodlordReset)
