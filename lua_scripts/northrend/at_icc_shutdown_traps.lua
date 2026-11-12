-- Icecrown Citadel: at_icc_shutdown_traps — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (at_icc_shutdown_traps, :1548-1560). Trigger ID 5649 is the TDB
-- areatrigger_scripts row (25101 + 26091 agree); no in-file ID exists
-- in C++ — the ID is read from the DB dumps, not guessed (the
-- at_zumrah 962 precedent, d8b1a516).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers receive (event, player,
-- triggerId).
-- C++ :1553-1559: the whole payload is instance->SetData(
-- DATA_UPPERSPIRE_TELE_ACT (41), DONE). The receiver leg lives in
-- instance_icecrown_citadel.lua (event-8): the value is stored and the
-- Upper Spire teleporter GO (202235) is activated (RemoveFlag
-- GO_FLAG_NOT_SELECTABLE + SetGoState GO_STATE_ACTIVE), the twin of
-- instance_icecrown_citadel.cpp:1110-1116 + SetTeleporterState
-- (:164-176). The trigger fires on every entry (plain AreaTriggerScript,
-- not OnlyOnce); the re-activation is idempotent, matching C++.

local TRIGGER_SHUTDOWN_TRAPS = 5649 -- TDB areatrigger_scripts

local DATA_UPPERSPIRE_TELE_ACT = 41 -- icecrown_citadel.h:117
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_SHUTDOWN_TRAPS then
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
    data:SetData(DATA_UPPERSPIRE_TELE_ACT, BOSS_STATE_DONE)
end)
