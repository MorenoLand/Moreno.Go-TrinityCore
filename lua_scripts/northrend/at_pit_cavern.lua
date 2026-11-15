-- Pit of Saron cavern area triggers — Lua port of the
-- AreaTriggerScript classes in
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/pit_of_saron.cpp:317-355
-- (at_pit_cavern_entrance :317-336, at_pit_cavern_end :338-355).
-- Triggers 5580 / 5581 (TDB 335.26091 areatrigger_scripts rows).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return is
-- discarded, so the C++ `return true` legs are moot here.
-- PORTED 2026-11-15 (at_* census). The SetData legs ride the
-- instance file's event-8 receiver (instance_pit_of_saron.lua:
-- DATA_CAVERN_ACTIVE 10 stores cavernActive and runs the icicle
-- chain; DATA_ICE_SHARDS_HIT 9 stores shardsHit). The cavern-end
-- achievement-credit leg (DoUpdateAchievementCriteria,
-- SPELL_DONT_LOOK_UP_ACHIEV_CREDIT 72845) stays out — no Lua
-- bridge (instance_pit_of_saron.lua:91 note).

local TRIGGER_PIT_CAVERN_ENTRANCE = 5580 -- TDB areatrigger_scripts row
local TRIGGER_PIT_CAVERN_END = 5581 -- TDB areatrigger_scripts row

local DATA_CAVERN_ACTIVE = 10 -- pit_of_saron.h:44
local SAY_TYRANNUS_CAVERN_ENTRANCE = 3 -- pit_of_saron.cpp:314

-- C++ :322-335 at_pit_cavern_entrance::OnTrigger.
local function onPitCavernEntrance(player)
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if (data.cavernActive or 0) ~= 0 then
        return
    end
    data:SetData(DATA_CAVERN_ACTIVE, 1)
    local tyrannus = data.tyrannusEvents
    if tyrannus then
        tyrannus:Talk(SAY_TYRANNUS_CAVERN_ENTRANCE)
    end
end

-- C++ :343-353 at_pit_cavern_end::OnTrigger (SetData leg only;
-- the achievement-credit leg stays out, see header).
local function onPitCavernEnd(player)
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    data:SetData(DATA_CAVERN_ACTIVE, 0)
end

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId == TRIGGER_PIT_CAVERN_ENTRANCE then
        onPitCavernEntrance(player)
    elseif triggerId == TRIGGER_PIT_CAVERN_END then
        onPitCavernEnd(player)
    end
end)
