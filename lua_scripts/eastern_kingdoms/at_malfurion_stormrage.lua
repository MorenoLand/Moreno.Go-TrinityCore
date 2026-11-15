-- Malfurion Stormrage area trigger (Sunken Temple) — Lua port of
-- src/server/scripts/EasternKingdoms/SunkenTemple/sunken_temple.cpp:49-65
-- (class at_malfurion_stormrage : public AreaTriggerScript).
-- Trigger 4016 (TDB 335.26091 areatrigger_scripts row for
-- at_malfurion_stormrage). Fire site: engine/world/areatrigger.go:64
-- (TriggerServerEvent, ServerEventAreaTrigger = 24); handlers
-- registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) (the sartura / at_scripts.lua
-- precedent). The handler return is discarded, so the C++
-- `return false` is moot here.
-- PORTED 2026-11-15 (at_* census).

local TRIGGER_MALFURION_STORMRAGE = 4016 -- TDB areatrigger_scripts row

local NPC_MALFURION_STORMRAGE = 15362 -- sunken_temple.cpp:44
local QUEST_THE_CHARGE_OF_DRAGONFLIGHTS = 8555 -- sunken_temple.cpp:46
local QUEST_ERANIKUS_TYRANT_OF_DREAMS = 8733 -- sunken_temple.cpp:45
local SUMMON_TYPE_TIMED_OR_DEAD_DESPAWN = 0 -- lua_creature_events.go:3699-3704

local QUEST_STATUS_REWARDED = 6 -- lua_player.go GetQuestStatus

-- C++ :54-60 at_malfurion_stormrage::OnTrigger. The instance
-- presence test is the GetInstanceScript() twin (broodlord
-- precedent): no instanceData table, no summon.
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_MALFURION_STORMRAGE then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj or not mapObj:GetInstanceData() then
        return
    end
    if player:FindNearestCreature(NPC_MALFURION_STORMRAGE, 15) then
        return
    end
    if player:GetQuestStatus(QUEST_THE_CHARGE_OF_DRAGONFLIGHTS) == QUEST_STATUS_REWARDED
        and player:GetQuestStatus(QUEST_ERANIKUS_TYRANT_OF_DREAMS) ~= QUEST_STATUS_REWARDED then
        player:SummonCreature(NPC_MALFURION_STORMRAGE, player:GetX(), player:GetY(), player:GetZ(),
            -1.52, SUMMON_TYPE_TIMED_OR_DEAD_DESPAWN, 100000)
    end
end)
