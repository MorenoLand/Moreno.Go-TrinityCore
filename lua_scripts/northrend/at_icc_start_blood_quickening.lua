-- Icecrown Citadel: at_icc_start_blood_quickening — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (at_icc_start_blood_quickening, :1562-1574). Trigger ID 5729 is the
-- TDB areatrigger_scripts row (25101 + 26091 agree); no in-file ID
-- exists in C++ — the ID is read from the DB dumps, not guessed (the
-- at_zumrah 962 precedent, d8b1a516).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers receive (event, player,
-- triggerId).
-- C++ :1567-1573: if DATA_BLOOD_QUICKENING_STATE (28) is NOT_STARTED,
-- instance->SetData(28, IN_PROGRESS). The instance file stores the
-- value (event-8); it feeds the Blood Queen :191 credit read
-- (GetData(28) == IN_PROGRESS) when that leg ports.
-- STAYS OUT: the C++ receiver's timer legs (:1078-1103, 1-min
-- EVENT_UPDATE_EXECUTION_TIME countdown from 30, world states 4903/
-- 4904, DONE + ACTION_KILL_MINCHAR to the Blood Queen at 0) are gated
-- on sQuestPoolMgr->IsQuestActive for the weekly quest (:1083) — no
-- quest-pool bridge exists (commands_debug.go:1842). Starting the
-- timer without the gate would run it in non-Blood-Quickening weeks,
-- so the legs stay out until a quest-pool bridge lands; the stored
-- state is the honest record of the C++ write either way.

local TRIGGER_START_BLOOD_QUICKENING = 5729 -- TDB areatrigger_scripts

local DATA_BLOOD_QUICKENING_STATE = 28 -- icecrown_citadel.h:104

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_START_BLOOD_QUICKENING then
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
    if (data.bloodQuickeningState or BOSS_STATE_NOT_STARTED) ~= BOSS_STATE_NOT_STARTED then
        return
    end
    data:SetData(DATA_BLOOD_QUICKENING_STATE, BOSS_STATE_IN_PROGRESS)
end)
