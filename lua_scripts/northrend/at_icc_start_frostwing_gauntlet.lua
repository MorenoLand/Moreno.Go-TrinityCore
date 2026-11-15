-- Frostwing gauntlet area trigger (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sister_svalna.cpp:1481-1493
-- (class at_icc_start_frostwing_gauntlet : public AreaTriggerScript).
-- Triggers 5616 / 5617 / 5618 (TDB 335.26091 areatrigger_scripts
-- rows for at_icc_start_frostwing_gauntlet — all three dispatch).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return
-- is discarded, so the C++ `return true` is moot here.
-- PORTED 2026-11-15 (at_* census) — sender only. The receiver is
-- NOT ported yet: Crok Scourgebane's DoAction(ACTION_START_GAUNTLET)
-- leg (boss_sister_svalna.cpp:404, escort Start + waypoints) has
-- no ev38 handler, so this DoAction fires into void until it
-- lands.

local TRIGGER_FROSTWING_GAUNTLET_1 = 5616 -- TDB areatrigger_scripts row
local TRIGGER_FROSTWING_GAUNTLET_2 = 5617 -- TDB areatrigger_scripts row
local TRIGGER_FROSTWING_GAUNTLET_3 = 5618 -- TDB areatrigger_scripts row

local ACTION_START_GAUNTLET = 2 -- boss_sister_svalna.cpp:198

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_FROSTWING_GAUNTLET_1
        and triggerId ~= TRIGGER_FROSTWING_GAUNTLET_2
        and triggerId ~= TRIGGER_FROSTWING_GAUNTLET_3 then
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
    local crok = data.crokScourgebane
    if crok then
        crok:DoAction(ACTION_START_GAUNTLET)
    end
end)
