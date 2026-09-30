-- Razorgore the Untamed (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_razorgore.cpp (boss_razorgoreAI); blackwing_lair.h:31
-- (DATA_RAZORGORE_THE_UNTAMED = 0, first boss), :88 (ACTION_PHASE_TWO =
-- 1), :89 (DATA_EGG_EVENT), :49 (NPC_RAZORGORE = 12435). Creature entry:
-- 12435 Razorgore the Untamed (C++ ScriptName "boss_razorgore" per
-- AddSC_boss_razorgore). The go_orb_of_domination GameObjectScript and
-- the spell_egg_event SpellScript from the same C++ file have no Lua
-- bridges (no gameobject/spellscript model) and are not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. OnDied: Talk(SAY_DEATH=3). The SAY_EGGS_BROKEN
-- talk lines (0-2) are never used by the boss AI and are not modeled.
-- Fight shape (C++-exact for the modeled arms): phase one has no
-- events scheduled — the boss takes no part in combat until
-- DoAction(ACTION_PHASE_TWO). DamageTaken zeroes all damage while
-- secondPhase is false (the C++ @todo admits this is wrong — real
-- behavior nukes the room and respawns him — but the damage-zeroing is
-- the shipped logic). The phase-two DoAction trigger comes from the
-- instance script once the egg event completes; with no
-- instance-script/DoAction bridge it can never fire, so secondPhase
-- stays false: phase-two timers (cleave 22540 15s then {7s,10s} /
-- warstomp 24375 35s then {15s,25s} / fireball volley 22425 7s then
-- {12s,15s} / conflagration 23023 12s then 30s, all triggered
-- DoCastVictim with unconditional re-arms) are modeled as code but
-- unreachable, and Razorgore stays unkillable doing melee only — which
-- is exactly what C++ phase one does in combat. Reset cancels timers
-- and clears the phase flag.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- modeled arms would fire unconditionally (jeklik convention); no
-- instance-script model — boss admission via the luaBossAI shim
-- (_Reset/_JustDied/BossAI::JustEngagedWith and the
-- DATA_RAZORGORE_THE_UNTAMED bookkeeping arms skipped); the
-- Reset/JustDied instance->SetData(DATA_EGG_EVENT, NOT_STARTED) arms
-- have no bearer; RemoveAllAuras/SetFullHealth in DoChangePhase have
-- no bridges; the go_orb_of_domination OnGossipHello (instance
-- GetCreature(DATA_RAZORGORE_THE_UNTAMED) -> Attack(player, true) +
-- CastSpell SPELL_MINDCONTROL 42013) and the spell_egg_event
-- SpellScript (OnHit -> instance->SetData(DATA_EGG_EVENT, SPECIAL))
-- have no Lua bridges — the mind-control/orb/egg-destruction gameplay
-- has no bearer, so the fight can never reach phase two (DoAction has
-- no Lua bridge either).

local ENTRY_RAZORGORE = 12435

local SPELL_CLEAVE = 22540
local SPELL_WARSTOMP = 24375
local SPELL_FIREBALLVOLLEY = 22425
local SPELL_CONFLAGRATION = 23023

local timers = {}
local secondPhase = {}

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

-- C++ EVENT_CLEAVE: triggered DoCastVictim(22540); re-arm {7s,10s}.
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE, true)
    end
    schedule(guid, "cleave", math.random(7000, 10000), function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_STOMP: triggered DoCastVictim(24375); re-arm {15s,25s}.
local function onWarstomp(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WARSTOMP, true)
    end
    schedule(guid, "stomp", math.random(15000, 25000), function()
        onWarstomp(creature, guid)
    end)
end

-- C++ EVENT_FIREBALL: triggered DoCastVictim(22425); re-arm {12s,15s}.
local function onFireballVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALLVOLLEY, true)
    end
    schedule(guid, "fireball", math.random(12000, 15000), function()
        onFireballVolley(creature, guid)
    end)
end

-- C++ EVENT_CONFLAGRATION: triggered DoCastVictim(23023); re-arm 30s.
local function onConflagration(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONFLAGRATION, true)
    end
    schedule(guid, "conflag", 30000, function()
        onConflagration(creature, guid)
    end)
end

-- C++ DoChangePhase: arms the four phase-two timers, sets secondPhase,
-- RemoveAllAuras + SetFullHealth (no bridge). Unreachable today: no
-- DoAction/instance-script bridge can trigger ACTION_PHASE_TWO (1).
local function doChangePhase(creature, guid)
    secondPhase[guid] = true
    schedule(guid, "cleave", 15000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "stomp", 35000, function()
        onWarstomp(creature, guid)
    end)
    schedule(guid, "fireball", 7000, function()
        onFireballVolley(creature, guid)
    end)
    schedule(guid, "conflag", 12000, function()
        onConflagration(creature, guid)
    end)
end

local function razorgoreResetState(guid)
    cancelTimers(guid)
    secondPhase[guid] = nil
end

local function razorgoreEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    secondPhase[guid] = nil
    -- C++ phase one schedules nothing: the mind-controlled orb
    -- handler drives the fight, not the AI. Combat AI starts only on
    -- DoAction(ACTION_PHASE_TWO), which has no Lua bridge.
end

-- C++ DamageTaken: pre-damage hook; zeroes all damage until phase two
-- (thekal convention — return false, 0). No adjustment needed: the
-- zeroing is C++-exact as shipped (the @todo is noted in the header).
local function razorgoreDamageTaken(event, creature, attacker, damage)
    if not secondPhase[creature:GetGUID()] then
        return false, 0
    end
end

local function razorgoreLeaveCombat(event, creature)
    razorgoreResetState(creature:GetGUID())
end

local function razorgoreDied(event, creature, killer)
    -- C++ JustDied: _JustDied bookkeeping has no bridge; the
    -- SetData(DATA_EGG_EVENT, NOT_STARTED) arm has no bearer.
    creature:Talk(3) -- SAY_DEATH
    razorgoreResetState(creature:GetGUID())
end

local function razorgoreReset(event, creature)
    -- C++ Reset's _Reset bookkeeping and SetData(DATA_EGG_EVENT,
    -- NOT_STARTED) have no bridges — only the timer cancel and the
    -- phase flag clear are modeled.
    razorgoreResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAZORGORE, 1, razorgoreEnterCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 2, razorgoreLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 4, razorgoreDied)
RegisterCreatureEvent(ENTRY_RAZORGORE, 9, razorgoreDamageTaken)
RegisterCreatureEvent(ENTRY_RAZORGORE, 23, razorgoreReset)
