-- Baltharus plateau area trigger (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/ruby_sanctum.cpp:157-176
-- (class at_baltharus_plateau : public OnlyOnceAreaTriggerScript).
-- Trigger 5867 (TDB 335.26091 areatrigger_scripts row for
-- at_baltharus_plateau). Fire site: engine/world/areatrigger.go:64
-- (TriggerServerEvent, ServerEventAreaTrigger = 24); handlers
-- registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) (the sartura / at_scripts.lua
-- precedent). The handler return is discarded, so the C++
-- `return true` is moot here.
-- PORTED 2026-11-15 (at_* census) — sender only. The OnlyOnce
-- latch rides the instanceData table (the nerubar-broodkeeper
-- precedent: per-(instance, trigger) latch; it does not clear on
-- instance reset, where C++ ResetAreaTriggerDone would re-arm
-- it — negligible, noted). The receivers are NOT ported yet:
-- Xerestrasza's DoAction(ACTION_INTRO_BALTHARUS) leg
-- (ruby_sanctum.cpp:97-105, Talk(SAY_XERESTRASZA_INTRO 8) once-
-- latched) and Baltharus's (boss_baltharus_the_warborn.cpp:92-95,
-- setActive + 7s EVENT_INTRO_TALK Talk(0)) have no ev38
-- handler, so these DoActions fire into void until they land.

local TRIGGER_BALTHARUS_PLATEAU = 5867 -- TDB areatrigger_scripts row

local ACTION_INTRO_BALTHARUS = -3975101 -- ruby_sanctum.h:58

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_BALTHARUS_PLATEAU then
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
    if data.baltharusPlateauDone then
        return
    end
    data.baltharusPlateauDone = true
    local xerestrasza = data.xerestrasza
    if xerestrasza then
        xerestrasza:DoAction(ACTION_INTRO_BALTHARUS)
    end
    local baltharus = data.baltharus
    if baltharus then
        baltharus:DoAction(ACTION_INTRO_BALTHARUS)
    end
end)
