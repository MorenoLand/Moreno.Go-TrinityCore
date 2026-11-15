-- Lord Marrowgar entrance area trigger (Icecrown Citadel) —
-- Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lord_marrowgar.cpp:670-684
-- (class at_lord_marrowgar_entrance : public
-- OnlyOnceAreaTriggerScript). Trigger 5732 (TDB 335.26091
-- areatrigger_scripts row for at_lord_marrowgar_entrance).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return
-- is discarded, so the C++ `return true` is moot here.
-- PORTED 2026-11-15 (at_* census) — sender only. The OnlyOnce
-- latch rides the instanceData table (the nerubar-broodkeeper
-- precedent: per-(instance, trigger) latch; it does not clear on
-- instance reset, where C++ ResetAreaTriggerDone would re-arm
-- it — negligible, noted). The receiver is NOT ported yet:
-- Marrowgar's DoAction(ACTION_TALK_ENTER_ZONE) leg
-- (boss_lord_marrowgar.cpp:337, Talk(SAY_ENTER_ZONE 0) if alive)
-- has no ev38 handler, so this DoAction fires into void until
-- it lands.

local TRIGGER_LORD_MARROWGAR_ENTRANCE = 5732 -- TDB areatrigger_scripts row

local ACTION_TALK_ENTER_ZONE = 2 -- boss_lord_marrowgar.cpp:104

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_LORD_MARROWGAR_ENTRANCE then
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
    if data.lordMarrowgarEntranceDone then
        return
    end
    data.lordMarrowgarEntranceDone = true
    local marrowgar = data.lordMarrowgar
    if marrowgar then
        marrowgar:DoAction(ACTION_TALK_ENTER_ZONE)
    end
end)
