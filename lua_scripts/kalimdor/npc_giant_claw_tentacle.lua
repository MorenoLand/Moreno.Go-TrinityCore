-- Temple of Ahn'Qiraj: Giant Claw Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_giant_claw_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<giant_claw_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15728 (worker-verified from C++ NPC_GIANT_CLAW_TENTACLE).
-- Ported: Ground Rupture 26139 on victim (500ms init -> 30s
-- repeat); Thrash 3391 on victim (5s init -> 10s repeat per C++
-- ThrashTimer reset, kurinnaxx convention). The EvadeTimer leg
-- has no Lua bridge — documented, not wired.
local ENTRY = 15728
local SPELL_GROUND_RUPTURE = 26139
local SPELL_THRASH = 3391

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

local function onGroundRupture(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_RUPTURE)
    end
    schedule(guid, "rupture", 30000, function()
        onGroundRupture(creature, guid)
    end)
end

local function onThrash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THRASH)
    end
    schedule(guid, "thrash", 10000, function()
        onThrash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rupture", 500, function()
        onGroundRupture(creature, guid)
    end)
    schedule(guid, "thrash", 5000, function()
        onThrash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
