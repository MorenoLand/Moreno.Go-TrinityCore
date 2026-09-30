-- Nalorakk (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_nalorakk.cpp
-- (boss_nalorakkAI); zulaman.h:28 (BOSS_NALORAKK = 0),
-- zulaman.h:45 (NPC_NALORAKK = 23576).
-- Creature entry: 23576 Nalorakk (C++ ScriptName "boss_nalorakk" per
-- AddSC_boss_nalorakk).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: troll form opens with brutal swipe 42384 at {7s,12s},
-- mangle 42389 at {10s,15s} (re-cast 1s later when it lands, otherwise
-- {10s,15s} — the cast is skipped when the victim still carries the
-- mangle effect 44955), and surge 42402 at {15s,20s} (Talk YELL_SURGE,
-- random player within 45 yd). The shapeshift timer fires at {45s,50s}:
-- bear form — Talk YELL_SHIFTEDTOBEAR, triggered self-cast 42377, then
-- lacerating slash 42395 at 2s (repeat {18s,23s}), rend flesh 42397 at 3s
-- (repeat {5s,10s}), deafening roar 42398 at {5s,10s} (repeat {15s,20s}) —
-- and shifts back to troll after {20s,25s} with Talk YELL_SHIFTEDTOTROLL,
-- RemoveAura(42377), and the troll timers re-armed to their initial
-- ranges. Berserk 45078 is a one-shot at 600s with Talk YELL_BERSERK,
-- re-armed for 600s (C++-exact). Kills: random YELL_KILL_ONE/YELL_KILL_TWO.
-- Deviations from C++: the entire pre-fight trash-wave gauntlet has no
-- bridge — no movement model (MovePoint/MovementInform/SetOrientation/
-- SetSpeedRate), no NOT_SELECTABLE/NON_ATTACKABLE flags on the Lua
-- object, no MoveInLineOfSight proximity hook, and no friendly-creature
-- AttackStart wiring — so the YELL_NALORAKK_WAVE1-4 arms, the waypoint
-- walks, and the SendAttacker arms never run and combat starts directly
-- at the engaged state; no instance-script model — admission only via the
-- luaBossAI shim (GetZulAmanAI/BossAI SetBossState arms); no threat model
-- — the surge target (C++ SelectTarget(Random,1,45,true), a random
-- non-tank) is approximated by a random alive player within 45 yd; the
-- commented-out SetUInt32Value equipment arms in C++ are dead code, not
-- modeled; mangle's victim aura check uses victim:HasAura(44955)
-- (C++ EnsureVictim()->HasAura(SPELL_MANGLEEFFECT)).

local ENTRY_NALORAKK = 23576

local YELL_SURGE = 5
local YELL_SHIFTEDTOBEAR = 6
local YELL_SHIFTEDTOTROLL = 7
local YELL_AGGRO = 4
local YELL_BERSERK = 8
local YELL_KILL_ONE = 9
local YELL_KILL_TWO = 10
local YELL_DEATH = 11

local SPELL_BRUTALSWIPE = 42384
local SPELL_MANGLE = 42389
local SPELL_MANGLEEFFECT = 44955
local SPELL_SURGE = 42402
local SPELL_BEARFORM = 42377
local SPELL_LACERATINGSLASH = 42395
local SPELL_RENDFLESH = 42397
local SPELL_DEAFENINGROAR = 42398
local SPELL_BERSERK = 45078

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function nalorakkState(guid)
    local st = state[guid]
    if not st then
        st = { inBearForm = false }
        state[guid] = st
    end
    return st
end

local onSurge
local onBrutalSwipe
local onMangle

local function armTrollTimers(creature, guid)    schedule(timers, guid, "surge", math.random(15000, 20000), function()
        onSurge(creature, guid)
    end)
    schedule(timers, guid, "brutalswipe", math.random(7000, 12000), function()
        onBrutalSwipe(creature, guid)
    end)
    schedule(timers, guid, "mangle", math.random(10000, 15000), function()
        onMangle(creature, guid)
    end)
end

-- C++ surge (troll form): Talk, cast 42402 on a random target within 45 yd
-- (C++-exact repeat {15s,20s}).
onSurge = function(creature, guid)
    creature:Talk(YELL_SURGE)
    local target = randomPlayerInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_SURGE)
    end
    schedule(timers, guid, "surge", math.random(15000, 20000), function()
        onSurge(creature, guid)
    end)
end

-- C++ brutal swipe (troll form): DoCastVictim(42384). Repeat {7s,12s}.
onBrutalSwipe = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BRUTALSWIPE)
    end
    schedule(timers, guid, "brutalswipe", math.random(7000, 12000), function()
        onBrutalSwipe(creature, guid)
    end)
end

-- C++ mangle (troll form): cast 42389 on the victim only when it does not
-- already carry the mangle effect (44955). When it lands, the timer
-- re-arms at 1s; otherwise {10s,15s}.
onMangle = function(creature, guid)
    local victim = creature:GetVictim()
    local repeatDelay = math.random(10000, 15000)
    if victim and not victim:HasAura(SPELL_MANGLEEFFECT) then
        creature:CastSpell(victim, SPELL_MANGLE)
        repeatDelay = 1000
    end
    schedule(timers, guid, "mangle", repeatDelay, function()
        onMangle(creature, guid)
    end)
end

local onLaceratingSlash
local onRendFlesh
local onDeafeningRoar

-- C++ bear-form timers: lacerating slash 2s then {18s,23s}, rend flesh 3s
-- then {5s,10s}, deafening roar {5s,10s} then {15s,20s} (DoCastVictim).
onLaceratingSlash = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_LACERATINGSLASH)
    end
    schedule(timers, guid, "lacerating", math.random(18000, 23000), function()
        onLaceratingSlash(creature, guid)
    end)
end

onRendFlesh = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_RENDFLESH)
    end
    schedule(timers, guid, "rend", math.random(5000, 10000), function()
        onRendFlesh(creature, guid)
    end)
end

onDeafeningRoar = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DEAFENINGROAR)
    end
    schedule(timers, guid, "roar", math.random(15000, 20000), function()
        onDeafeningRoar(creature, guid)
    end)
end

-- C++ shapeshift timer: bear -> troll (Talk, RemoveAura, troll timers
-- re-armed to their initial ranges, next shift {45s,50s}) or troll ->
-- bear (Talk, triggered self-cast 42377, bear timers, next shift
-- {20s,25s}).
local function onShapeshift(creature, guid)
    local st = nalorakkState(guid)
    if st.inBearForm then
        creature:Talk(YELL_SHIFTEDTOTROLL)
        creature:RemoveAura(SPELL_BEARFORM)
        armTrollTimers(creature, guid)
        schedule(timers, guid, "shapeshift", math.random(45000, 50000), function()
            onShapeshift(creature, guid)
        end)
        st.inBearForm = false
    else
        creature:Talk(YELL_SHIFTEDTOBEAR)
        creature:CastSpell(creature, SPELL_BEARFORM, true)
        schedule(timers, guid, "lacerating", 2000, function()
            onLaceratingSlash(creature, guid)
        end)
        schedule(timers, guid, "rend", 3000, function()
            onRendFlesh(creature, guid)
        end)
        schedule(timers, guid, "roar", math.random(5000, 10000), function()
            onDeafeningRoar(creature, guid)
        end)
        schedule(timers, guid, "shapeshift", math.random(20000, 25000), function()
            onShapeshift(creature, guid)
        end)
        st.inBearForm = true
    end
end

-- C++ berserk: one-shot 600s, Talk, triggered self-cast 45078; the C++
-- timer re-arms for 600s.
local function onBerserk(creature, guid)
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(timers, guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
end

local function nalorakkResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { inBearForm = false }
end

local function nalorakkEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    nalorakkResetState(guid)
    -- C++ JustEngagedWith: Talk + the Initialize timer set.
    creature:Talk(YELL_AGGRO)
    armTrollTimers(creature, guid)
    schedule(timers, guid, "shapeshift", math.random(45000, 50000), function()
        onShapeshift(creature, guid)
    end)
    schedule(timers, guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
end

local function nalorakkLeaveCombat(event, creature)
    nalorakkResetState(creature:GetGUID())
end

-- C++ KilledUnit: random YELL_KILL_ONE / YELL_KILL_TWO (no player gate).
local function nalorakkTargetDied(event, creature, victim)
    if math.random(0, 1) == 0 then
        creature:Talk(YELL_KILL_ONE)
    else
        creature:Talk(YELL_KILL_TWO)
    end
end

local function nalorakkDied(event, creature, killer)
    nalorakkResetState(creature:GetGUID())
    creature:Talk(YELL_DEATH)
end

local function nalorakkReset(event, creature)
    nalorakkResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_NALORAKK, 1, nalorakkEnterCombat)
RegisterCreatureEvent(ENTRY_NALORAKK, 2, nalorakkLeaveCombat)
RegisterCreatureEvent(ENTRY_NALORAKK, 3, nalorakkTargetDied)
RegisterCreatureEvent(ENTRY_NALORAKK, 4, nalorakkDied)
RegisterCreatureEvent(ENTRY_NALORAKK, 23, nalorakkReset)
