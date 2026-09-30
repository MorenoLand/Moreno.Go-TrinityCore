-- Wushoolay the Storm Witch (Zul'Gurub, Edge of Madness) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_wushoolay.cpp
-- (boss_wushoolayAI); zulgurub.h:39 (DATA_EDGE_OF_MADNESS = 9, optional
-- event — Gri'lek, Renataki, Hazza'rah, or Wushoolay). Creature entry:
-- 15085 Wushoolay (C++ ScriptName "boss_wushoolay" per
-- AddSC_boss_wushoolay; classic.wowhead.com/npc=15085/wushoolay;
-- zulgurub.h defines no NPC_WUSHOOLAY constant). Wushoolay is summoned
-- by the brazier event (no summon model — she simply starts combat
-- already spawned). Eluna creature events: 1 OnEnterCombat,
-- 2 OnLeaveCombat, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. No Talk lines in the C++ AI.
-- Fight shape: enter combat arms the two timers — lightning cloud 25033
-- at {5s,10s} then {15s,20s} on the victim (triggered DoCastVictim,
-- C++-exact) / lightning wave 24819 at {8s,16s} then {12s,16s} on a
-- random alive player in the instance (non-triggered DoCast,
-- C++-exact). The re-arms are unconditional (nil-victim ticks cast
-- nothing but keep the schedule, jeklik convention). Reset cancels
-- timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); the lightning-wave SelectTarget 100-yd range check is
-- skipped (shared arena) — the pick is a random alive player in the
-- instance.

local ENTRY_WUSHOOLAY = 15085

local SPELL_LIGHTNINGCLOUD = 25033
local SPELL_LIGHTNINGWAVE = 24819

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

-- C++ EVENT_LIGHTNINGCLOUD: triggered DoCastVictim(25033); re-arm
-- {15s,20s}.
local function onLightningCloud(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_LIGHTNINGCLOUD, true)
    end
    schedule(guid, "cloud", math.random(15000, 20000), function()
        onLightningCloud(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNINGWAVE: non-triggered DoCast on
-- SelectTarget(Random, 0, 100, true); re-arm {12s,16s}.
local function onLightningWave(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_LIGHTNINGWAVE)
    end
    schedule(guid, "wave", math.random(12000, 16000), function()
        onLightningWave(creature, guid)
    end)
end

local function wushoolayResetState(guid)
    cancelTimers(guid)
end

local function wushoolayEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "cloud", math.random(5000, 10000), function()
        onLightningCloud(creature, guid)
    end)
    schedule(guid, "wave", math.random(8000, 16000), function()
        onLightningWave(creature, guid)
    end)
end

local function wushoolayLeaveCombat(event, creature)
    wushoolayResetState(creature:GetGUID())
end

local function wushoolayDied(event, creature, killer)
    wushoolayResetState(creature:GetGUID())
end

local function wushoolayReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    wushoolayResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WUSHOOLAY, 1, wushoolayEnterCombat)
RegisterCreatureEvent(ENTRY_WUSHOOLAY, 2, wushoolayLeaveCombat)
RegisterCreatureEvent(ENTRY_WUSHOOLAY, 4, wushoolayDied)
RegisterCreatureEvent(ENTRY_WUSHOOLAY, 23, wushoolayReset)
