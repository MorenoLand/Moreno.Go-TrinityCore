-- Felmyst (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_felmyst.cpp
-- (boss_felmyst, npc_felmyst_vapor, npc_felmyst_trail); sunwell_
-- plateau.h:33 (DATA_FELMYST = 2, third boss), :67 (NPC_FELMYST =
-- 25038), :69 (NPC_DEAD = 25268), :73 (NPC_VAPOR = 25265), :74
-- (NPC_VAPOR_TRAIL = 25267). Entries: 25038 Felmyst (C++
-- ScriptName "boss_felmyst"), 25267 vapor trail ("npc_felmyst_
-- trail") — entry+ScriptName verifiable from the C++ sources;
-- the creature_template ScriptName bindings are DB-side — no TDB
-- in this workspace, so only the C++-side naming is verified.
-- Talk lines used: YELL_KILL=1 (kill), YELL_BERSERK=4 (berserk),
-- YELL_DEATH=5 (death); YELL_BIRTH=0 (JustAppeared), YELL_
-- BREATH=2 (the DoTextEmote is commented out in C++) and YELL_
-- TAKEOFF=3 belong to the unmodeled spawn/flight arms.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat(1):
-- triggered self-cast sunwell radiance 45769 + noxious fumes
-- 47002 (C++ DoCast(me, AURA_*, true)), arm cleave 19983 {5s,10s}
-- then {5s,10s}, non-triggered DoCastVictim (nil ticks cast
-- nothing but keep the schedule, jeklik convention) / corrosion
-- 45866 {10s,20s} then {20s,30s}, non-triggered DoCastVictim /
-- gas nova 45855 {15s,20s} then {20s,25s}, non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz
-- convention) / encapsulate 45661 {20s,25s} then {25s,30s}, non-
-- triggered on a random alive player in the instance within 150 yd
-- (C++ SelectTarget(Random, 0, 150, true), player-only — teron
-- convention; nil pick casts nothing but keeps the schedule) /
-- berserk 45078 10min one-shot: Talk(YELL_BERSERK), triggered
-- self-cast (C++ DoCast(me, SPELL_BERSERK, true)), then re-armed
-- at 10s (C++-exact — the C++ handler re-schedules EVENT_BERSERK
-- at 10s after every fire). OnTargetDied(3): Talk(YELL_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): Talk(YELL_DEATH); the SetBossState(DONE) arm
-- is blocked on the instance-script model. OnReset(23): the
-- Initialize/events.Reset arms (phase tracking lives only in the
-- unmodeled flight sequence — see below); the DisableGravity,
-- bounding-radius/combat-reach, setActive(false) and
-- DespawnSummons(NPC_VAPOR_TRAIL) arms have no
-- flag/combat-reach/active/creature-enumeration bridges and the
-- SetBossState(NOT_STARTED) arm is blocked on the instance-script
-- model. OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Trail (25267): OnReset(23): triggered self-cast trail trigger
-- 45399 (C++ constructor arm, C++-exact); the NOT_SELECTABLE flag
-- and 0.01 bounding-radius arms have no flag/combat-reach bridges.
-- Vapor (25265, "npc_felmyst_vapor") is not registered: its only
-- C++ arms are the NOT_SELECTABLE flag, the 0.8 run-speed arm and
-- the no-victim random-target AttackStart selection, none of which
-- have flag/speed/threat bridges — entry verified for future
-- registration. NPC_DEAD (25268, Blazing Dead) has no AI class in
-- this file — no C++ ScriptName to verify a binding against, so it
-- is not registered (firesworn convention); its boss-side
-- JustSummoned (AttackStart random + DoZoneInCombat + triggered
-- self-cast dead passive 45415) and SpellHit arms are summon-
-- blocked (below).
-- Deliberate deviations (all await engine bridges): the flight
-- phase is unmodeled — the EVENT_FLIGHT 1min transition, the
-- entire 10-step HandleFlightSequence (liftoff emote + Talk(YELL_
-- TAKEOFF), MovePoint/contact-point/facing movement, the two
-- vapor summons + VAPOR_CHANNEL 45389 / VAPOR_TRIGGER 45411
-- cross-creature casts, the fog-breath 45495 self-cast + FOG_
-- TRIGGER 45582 / FOG_FORCE 45782 fog summon + breath-move loop x3,
-- the max-threat chase and the landing + EnterPhase(GROUND) re-
-- entry) has no movement/contact-point/facing/cross-creature/
-- summon bridges — the fight stays in ground-phase timers
-- (ragnaros-intro convention); the DamageTaken lethal-damage
-- rewrite arm has no bearer without the flight phase (phase is
-- always GROUND here). The SpellHit FOG_INFORM 45714 arm (summon
-- NPC_DEAD at the caster, copy health, triggered FOG_CHARM 45717
-- + FOG_CHARM2 45726 self-casts, lethal DealDamage on the caster)
-- has no summon or kill bridges — unmodeled. The
-- DespawnSummons(NPC_VAPOR_TRAIL)->NPC_DEAD summon arm has no
-- creature-enumeration/summon bridges — unmodeled. The
-- JustAppeared Talk(YELL_BIRTH) arm has no spawn-fire bridge for
-- the summon path (Go fires ON_SPAWN (5) only on the respawn
-- path) — unmodeled. The AttackStart/MoveInLineOfSight PHASE_
-- FLIGHT gates have no bridge (melee/aggro gating is engine-
-- side). The UpdateAI UNIT_STATE_CASTING queue gate and the
-- UpdateVictim evade-side arm have no UNIT_STATE bridge — timers
-- fire unconditionally (jeklik convention) and evade-side cleanup
-- is engine-side.

local NPC_FELMYST = 25038
local NPC_VAPOR_TRAIL = 25267

local SPELL_CLEAVE = 19983
local SPELL_CORROSION = 45866
local SPELL_GAS_NOVA = 45855
local SPELL_ENCAPSULATE_CHANNEL = 45661
local SPELL_BERSERK = 45078
local AURA_SUNWELL_RADIANCE = 45769
local AURA_NOXIOUS_FUMES = 47002
local SPELL_TRAIL_TRIGGER = 45399

local YELL_KILL = 1
local YELL_BERSERK = 4
local YELL_DEATH = 5

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

-- Alive players sharing the creature's map+instance (teron
-- convention).
local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ EVENT_CLEAVE: DoCastVictim(19983); repeat {5s,10s}.
local function felmystCleave(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
end

-- C++ EVENT_CORROSION: DoCastVictim(45866); repeat {20s,30s}.
local function felmystCorrosion(creature, guid)
    creature:CastSpell(nil, SPELL_CORROSION)
    schedule(guid, "corrosion", math.random(20000, 30000), function()
        felmystCorrosion(creature, guid)
    end)
end

-- C++ EVENT_GAS_NOVA: DoCast(me, 45855); repeat {20s,25s}.
local function felmystGasNova(creature, guid)
    creature:CastSpell(creature, SPELL_GAS_NOVA)
    schedule(guid, "gasnova", math.random(20000, 25000), function()
        felmystGasNova(creature, guid)
    end)
end

-- C++ EVENT_ENCAPSULATE: DoCast(random alive player within 150
-- yd, 45661); repeat {25s,30s}.
local function felmystEncapsulate(creature, guid)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 150 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(1, #candidates)],
            SPELL_ENCAPSULATE_CHANNEL)
    end
    schedule(guid, "encapsulate", math.random(25000, 30000), function()
        felmystEncapsulate(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(YELL_BERSERK) + triggered self-cast
-- 45078; C++ re-arms the event at 10s after every fire.
local function felmystBerserkTick(creature, guid)
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(guid, "berserk", 10000, function()
        felmystBerserkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: triggered self-cast of the two pull auras
-- + the ground-phase schedule. The setActive/DoZoneInCombat arms
-- have no bridges and SetBossState(IN_PROGRESS) is blocked on the
-- instance-script model.
local function felmystEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, AURA_SUNWELL_RADIANCE, true)
    creature:CastSpell(creature, AURA_NOXIOUS_FUMES, true)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
    schedule(guid, "corrosion", math.random(10000, 20000), function()
        felmystCorrosion(creature, guid)
    end)
    schedule(guid, "gasnova", math.random(15000, 20000), function()
        felmystGasNova(creature, guid)
    end)
    schedule(guid, "encapsulate", math.random(20000, 25000), function()
        felmystEncapsulate(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        felmystBerserkTick(creature, guid)
    end)
end

-- C++ KilledUnit: Talk(YELL_KILL) with no victim gate (C++-exact).
local function felmystTargetDied(event, creature, victim)
    creature:Talk(YELL_KILL)
end

-- C++ JustDied: Talk(YELL_DEATH); SetBossState(DONE) is blocked
-- on the instance-script model.
local function felmystDied(event, creature, killer)
    creature:Talk(YELL_DEATH)
    cancelTimers(creature:GetGUID())
end

local function felmystResetState(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_felmyst_trail constructor arm: triggered self-cast
-- trail trigger 45399 (C++-exact); the NOT_SELECTABLE flag and
-- bounding-radius arms have no bridges.
local function trailReset(event, creature)
    creature:CastSpell(creature, SPELL_TRAIL_TRIGGER, true)
end

RegisterCreatureEvent(NPC_FELMYST, 1, felmystEnterCombat)
RegisterCreatureEvent(NPC_FELMYST, 2, felmystResetState)
RegisterCreatureEvent(NPC_FELMYST, 3, felmystTargetDied)
RegisterCreatureEvent(NPC_FELMYST, 4, felmystDied)
RegisterCreatureEvent(NPC_FELMYST, 23, felmystResetState)
RegisterCreatureEvent(NPC_VAPOR_TRAIL, 23, trailReset)
