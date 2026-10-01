-- Brutallus (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_brutallus.cpp
-- (boss_brutallus only; no spell/aura script classes in this file);
-- sunwell_plateau.h:32 (DATA_BRUTALLUS = 1, second boss), :41
-- (DATA_MADRIGOSA), :65 (NPC_BRUTALLUS = 24882), :67 (NPC_FELMYST =
-- 25038). Entry: 24882 Brutallus (C++ ScriptName "boss_brutallus"
-- per RegisterSunwellPlateauCreatureAI in AddSC_boss_brutallus;
-- the creature_template ScriptName binding is DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified. Talk
-- lines used: YELL_AGGRO=5 (pull), YELL_KILL=6 (kill), YELL_LOVE=7
-- (stomp), YELL_BERSERK=8 (berserk), YELL_DEATH=9 (death); the
-- YELL_INTRO*=0/1/2/3/4 and YELL_MADR_* lines belong to the
-- unmodeled Madrigosa intro chain.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnReset(23): per-
-- GUID state reset (Enraged=false), triggered self-cast dual wield
-- 42459 (C++ DoCast(me, SPELL_DUAL_WIELD, true), C++-exact); the
-- SetBossState(DATA_BRUTALLUS, NOT_STARTED) arm is blocked on the
-- instance-script model. OnEnterCombat(1): per-GUID reset,
-- Talk(YELL_AGGRO), arm meteor slash 45150 11s then 11s, non-
-- triggered DoCastVictim (nil ticks cast nothing but keep the
-- schedule, jeklik convention) / stomp 45185 30s then 30s, Talk(
-- YELL_LOVE) + non-triggered DoCastVictim / burn 46394 60s then
-- {60s,180s}: random alive player in the instance within 100 yd
-- not carrying 46394 (C++ SelectTarget(Random, 0, 100.0f, true,
-- true, -SPELL_BURN), C++-exact), targeted self-cast burn,
-- triggered (C++ target->CastSpell(target, SPELL_BURN, true));
-- nil pick casts nothing but keeps the schedule / berserk 26662
-- 6min one-shot: Talk(YELL_BERSERK), non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz
-- convention), Enraged=true. OnTargetDied(3): Talk(YELL_KILL),
-- no TYPEID gate (C++-exact — the C++ KilledUnit Talk has no
-- victim gate). OnDied(4): Talk(YELL_DEATH); the SetBossState(
-- DONE) arm is blocked on the instance-script model and the
-- SummonCreature(NPC_FELMYST) arm has no summon bridge —
-- unmodeled (gurtogg convention). OnLeaveCombat(2)/OnReset(23):
-- cancel timers, drop per-GUID state (the EnterEvadeMode
-- !Intro gate has no bridge — evade-side cleanup is engine-side).
-- The intro chain is unmodeled: StartIntro/DoIntro's Madrigosa
-- (DATA_MADRIGOSA) lookup, health copy, NON_ATTACKABLE flag arm,
-- Talk(YELL_INTRO*) / Madrigosa Talk(YELL_MADR_*) relays, frost
-- blast 45203 / frostbolt 44843 / encapsulate 45665 / channelling
-- 45661 cross-creature casts, Unit::Kill(Madrigosa), SetFullHealth
-- and the SetBossState(SPECIAL)/MoveInLineOfSight intro start all
-- have no instance-creature, cross-creature, flag or movement
-- bridges (blocked); the AttackStart Intro/IsIntro gate has no
-- bridge — the fight starts on pull (ragnaros-intro convention).
-- The burn arm deviates once: the target-self-cast has no
-- player-side CastSpell bridge — modeled as target:AddAura(46394),
-- the spell's entire effect is the aura (gurtogg fel-rage
-- convention).

local NPC_BRUTALLUS = 24882

local SPELL_METEOR_SLASH = 45150
local SPELL_BURN = 46394
local SPELL_STOMP = 45185
local SPELL_BERSERK = 26662
local SPELL_DUAL_WIELD = 42459

local YELL_AGGRO = 5
local YELL_KILL = 6
local YELL_LOVE = 7
local YELL_BERSERK = 8
local YELL_DEATH = 9

local timers = {}
local brutallusState = {}

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

-- C++ EVENT metor slash: DoCastVictim(45150); repeat 11s.
local function brutallusMeteorSlash(creature, guid)
    creature:CastSpell(nil, SPELL_METEOR_SLASH)
    schedule(guid, "meteorslash", 11000, function()
        brutallusMeteorSlash(creature, guid)
    end)
end

-- C++ stomp arm: Talk(YELL_LOVE) + DoCastVictim(45185); repeat 30s.
local function brutallusStomp(creature, guid)
    creature:Talk(YELL_LOVE)
    creature:CastSpell(nil, SPELL_STOMP)
    schedule(guid, "stomp", 30000, function()
        brutallusStomp(creature, guid)
    end)
end

-- C++ burn arm: random alive player within 100 yd not carrying
-- 46394 gets the targeted self-cast; repeat {60s,180s}. The
-- player-side self-cast has no bridge — AddAura reproduces the
-- whole effect (gurtogg fel-rage convention).
local function brutallusBurn(creature, guid)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 100 and not p:HasAura(SPELL_BURN) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        candidates[math.random(1, #candidates)]:AddAura(SPELL_BURN)
    end
    schedule(guid, "burn", math.random(60000, 180000), function()
        brutallusBurn(creature, guid)
    end)
end

-- C++ berserk arm: one-shot at 6min, Talk(YELL_BERSERK) + self-
-- cast 26662, Enraged=true (C++-exact).
local function brutallusBerserk(creature, guid)
    local state = brutallusState[guid]
    if state == nil or state.enraged then
        return
    end
    state.enraged = true
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ JustEngagedWith: Talk(YELL_AGGRO) + the four initial arms.
-- The SetBossState(IN_PROGRESS) arm is blocked on the
-- instance-script model.
local function brutallusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    brutallusState[guid] = { enraged = false }
    creature:Talk(YELL_AGGRO)
    schedule(guid, "meteorslash", 11000, function()
        brutallusMeteorSlash(creature, guid)
    end)
    schedule(guid, "stomp", 30000, function()
        brutallusStomp(creature, guid)
    end)
    schedule(guid, "burn", 60000, function()
        brutallusBurn(creature, guid)
    end)
    schedule(guid, "berserk", 360000, function()
        brutallusBerserk(creature, guid)
    end)
end

-- C++ KilledUnit: Talk(YELL_KILL) with no victim gate (C++-exact).
local function brutallusTargetDied(event, creature, victim)
    creature:Talk(YELL_KILL)
end

-- C++ JustDied: Talk(YELL_DEATH); the SetBossState(DONE) arm is
-- blocked on the instance-script model and the Felmyst summon arm
-- has no summon bridge (gurtogg convention).
local function brutallusDied(event, creature, killer)
    creature:Talk(YELL_DEATH)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    brutallusState[guid] = nil
end

local function brutallusResetState(guid)
    cancelTimers(guid)
    brutallusState[guid] = nil
end

local function brutallusLeaveCombat(event, creature)
    brutallusResetState(creature:GetGUID())
end

-- C++ Reset: Initialize (Enraged=false) + triggered self-cast
-- dual wield 42459 (C++-exact); the SetBossState(NOT_STARTED) arm
-- is blocked on the instance-script model.
local function brutallusReset(event, creature)
    local guid = creature:GetGUID()
    brutallusResetState(guid)
    brutallusState[guid] = { enraged = false }
    creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
end

RegisterCreatureEvent(NPC_BRUTALLUS, 1, brutallusEnterCombat)
RegisterCreatureEvent(NPC_BRUTALLUS, 2, brutallusLeaveCombat)
RegisterCreatureEvent(NPC_BRUTALLUS, 3, brutallusTargetDied)
RegisterCreatureEvent(NPC_BRUTALLUS, 4, brutallusDied)
RegisterCreatureEvent(NPC_BRUTALLUS, 23, brutallusReset)
