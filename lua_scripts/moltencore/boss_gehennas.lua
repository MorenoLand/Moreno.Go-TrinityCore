-- Gehennas (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_gehennas.cpp (boss_gehennasAI only); molten_core.h:32
-- (BOSS_GEHENNAS = 2, third boss), :56 (NPC_GEHENNAS = 12259).
-- Creature entry: 12259 Gehennas (C++ ScriptName "boss_gehennas" per
-- AddSC_boss_gehennas). No Talk lines in the C++ AI. SDComment
-- upstream: "Adds MC NYI".
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- Gehennas curse 19716 12s then {22s,30s}, non-triggered
-- DoCastVictim / rain of fire 19717 10s then {4s,12s}, non-triggered
-- on a random alive player in the instance (wushoolay
-- randomAlivePlayer helper) / shadow bolt 19728 6s then 7s,
-- non-triggered on a random alive player in the instance excluding the
-- current victim (C++ SelectTarget(Random, 1) skips position 0).
-- Nil-target ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_GEHENNAS bookkeeping arms skipped); the SDComment "Adds
-- MC NYI" arms are absent from the C++ AI itself (the Flamewaker adds
-- are not scripted in this file) and are not modeled.

local ENTRY_GEHENNAS = 12259

local SPELL_GEHENNAS_CURSE = 19716
local SPELL_RAIN_OF_FIRE = 19717
local SPELL_SHADOW_BOLT = 19728

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

-- C++ EVENT_GEHENNAS_CURSE: non-triggered DoCastVictim(19716);
-- re-arm {22s,30s}.
local function onGehennasCurse(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GEHENNAS_CURSE)
    end
    schedule(guid, "curse", math.random(22000, 30000), function()
        onGehennasCurse(creature, guid)
    end)
end

-- C++ EVENT_RAIN_OF_FIRE: non-triggered cast 19717 on a random alive
-- player in the instance; re-arm {4s,12s}.
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function onRainOfFire(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_RAIN_OF_FIRE)
    end
    schedule(guid, "rainoffire", math.random(4000, 12000), function()
        onRainOfFire(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BOLT: non-triggered cast 19728 on a random alive
-- player in the instance excluding the current victim (C++
-- SelectTarget(Random, 1) skips position 0); a sole-victim tick casts
-- nothing but keeps the schedule. Re-arm 7s regardless.
local function randomAlivePlayerExcluding(creature, victim)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victimGUID = victim and victim:GetGUID() or 0
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and p:GetGUID() ~= victimGUID then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function onShadowBolt(creature, guid)
    local pick = randomAlivePlayerExcluding(creature, creature:GetVictim())
    if pick then
        creature:CastSpell(pick, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", 7000, function()
        onShadowBolt(creature, guid)
    end)
end

local function gehennasEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "curse", 12000, function()
        onGehennasCurse(creature, guid)
    end)
    schedule(guid, "rainoffire", 10000, function()
        onRainOfFire(creature, guid)
    end)
    schedule(guid, "shadowbolt", 6000, function()
        onShadowBolt(creature, guid)
    end)
end

local function gehennasResetState(guid)
    cancelTimers(guid)
end

local function gehennasLeaveCombat(event, creature)
    gehennasResetState(creature:GetGUID())
end

local function gehennasDied(event, creature, killer)
    gehennasResetState(creature:GetGUID())
end

local function gehennasReset(event, creature)
    gehennasResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GEHENNAS, 1, gehennasEnterCombat)
RegisterCreatureEvent(ENTRY_GEHENNAS, 2, gehennasLeaveCombat)
RegisterCreatureEvent(ENTRY_GEHENNAS, 4, gehennasDied)
RegisterCreatureEvent(ENTRY_GEHENNAS, 23, gehennasReset)
