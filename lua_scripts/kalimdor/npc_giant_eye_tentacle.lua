-- Temple of Ahn'Qiraj: Giant Eye Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (npc_giant_eye_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<giant_eye_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15334 (worker-verified from C++ NPC_GIANT_EYE_TENTACLE).
-- Ported: Green Beam 26134 on random player (500ms init -> 2.1s
-- repeat; C++ SelectTarget Random has no bridge — GetPlayersInWorld
-- filtered by map/instance/alive, maiden_of_virtue convention).
-- Summon/positioning legs have no bridge — documented, not wired.
local ENTRY = 15334
local SPELL_GREEN_BEAM = 26134

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

local function onGreenBeam(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_GREEN_BEAM)
    end
    local per = timers[guid]
    if per then
        per["beam"] = CreateLuaEvent(function()
            onGreenBeam(creature, guid)
        end, 2100)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    timers[guid] = {}
    timers[guid]["beam"] = CreateLuaEvent(function()
        onGreenBeam(creature, guid)
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
