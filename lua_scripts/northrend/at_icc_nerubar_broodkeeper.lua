-- Icecrown Citadel: at_icc_nerubar_broodkeeper — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (at_icc_nerubar_broodkeeper : public OnlyOnceAreaTriggerScript,
-- :1576-1591). Trigger ID 5611 is the in-file AT_NERUBAR_BROODKEEPER
-- (icecrown_citadel.h:541); the script name binds it in the DB.
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers receive (event, player,
-- triggerId) (the at_hor_triggers.lua precedent).
-- C++ :1582-1589: a non-GM player in the instance fires
-- instance->SetData(DATA_NERUBAR_BROODKEEPER_EVENT 46, triggerId);
-- the instance OnSetData receiver in instance_icecrown_citadel.lua
-- fans out DoAction(ACTION_NERUBAR_FALL) to the trigger's broodkeeper
-- group. OnlyOnce is per (instance, triggerId)
-- (ScriptMgr.cpp:2531-2542: IsAreaTriggerDone / MarkAreaTriggerDone),
-- modeled as a latch on the shared instanceData table.
-- Deviations: the GM gate rides player:IsGM() (lua_player.go:1411,
-- security > 0 = Player::IsGameMaster twin); the early return runs
-- before the latch write, matching C++ return false (the trigger is
-- not marked done for a GM). The DB may bind a second trigger id to
-- but no second id appears in the C++ file, so only 5611 dispatches
-- (the gelihast precedent — never guess an id).

local AT_NERUBAR_BROODKEEPER = 5611 -- icecrown_citadel.h:541
local DATA_NERUBAR_BROODKEEPER_EVENT = 46 -- icecrown_citadel.h:122

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= AT_NERUBAR_BROODKEEPER then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if data.broodkeeperTriggerDone == nil then
        data.broodkeeperTriggerDone = {}
    end
    if player:IsGM() then
        return
    end
    if data.broodkeeperTriggerDone[triggerId] then
        return
    end
    data.broodkeeperTriggerDone[triggerId] = true
    data:SetData(DATA_NERUBAR_BROODKEEPER_EVENT, triggerId)
end)
