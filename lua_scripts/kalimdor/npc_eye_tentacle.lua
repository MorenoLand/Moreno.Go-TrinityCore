-- Temple of Ahn'Qiraj: Eye Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_eye_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<eye_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15726 (worker-verified from C++ NPC_EYE_TENTACLE).
-- Ported: Mind Flay 26143 on random player (500ms init -> 10s
-- repeat; C++ SelectTarget Random has no bridge — GetPlayersInWorld
-- filtered by map/instance/alive, maiden_of_virtue convention).
-- The 35s KillSelf has no Lua bridge (no KillSelf API); the portal
-- summon/JustDied cross-kill and DoZoneInCombat have no bridge —
-- documented, not wired.
local ENTRY = 15726
local SPELL_MIND_FLAY = 26143

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

local function onMindFlay(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and not p:HasAura(26476) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_MIND_FLAY)
    end
    local per = timers[guid]
    if per then
        per["flay"] = CreateLuaEvent(function()
            onMindFlay(creature, guid)
        end, 10000)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    timers[guid] = {}
    timers[guid]["flay"] = CreateLuaEvent(function()
        onMindFlay(creature, guid)
    end, 500)
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
