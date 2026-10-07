-- Temple of Ahn'Qiraj: Claw Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_claw_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<claw_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15725 (worker-verified from C++ NPC_CLAW_TENTACLE).
-- Hamstring 26141 on victim (2s init -> 5s repeat per C++
-- HamstringTimer reset; the giant-claw port (C++ repeat 10s) is
-- a different AI — not copied here). The EvadeTimer leg (5s
-- bridge — documented, not wired.
local ENTRY = 15725
local SPELL_GROUND_RUPTURE = 26139
local SPELL_HAMSTRING = 26141

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

local function onHamstring(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HAMSTRING)
    end
    schedule(guid, "hamstring", 5000, function()
        onHamstring(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rupture", 500, function()
        onGroundRupture(creature, guid)
    end)
    schedule(guid, "hamstring", 2000, function()
        onHamstring(creature, guid)
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
