-- Lady Deathwhisper entrance area trigger (Icecrown Citadel) —
-- Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lady_deathwhisper.cpp:1038-1052
-- (class at_lady_deathwhisper_entrance : public
-- OnlyOnceAreaTriggerScript). Trigger 5709 (TDB 335.26091
-- areatrigger_scripts row for at_lady_deathwhisper_entrance).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return
-- is discarded, so the C++ `return true` is moot here.
-- PORTED 2026-11-15 (at_* census) — sender only. The OnlyOnce
-- latch rides the instanceData table (the nerubar-broodkeeper
-- precedent: per-(instance, trigger) latch; it does not clear on
-- instance reset, where C++ ResetAreaTriggerDone would re-arm
-- it — negligible, noted). The receiver is NOT ported yet: the
-- intro machine (boss_lady_deathwhisper.cpp:234-..., Talk
-- SAY_INTRO_1..6 over ~72s) has no ev38 handler, so this
-- DoAction fires into void until it lands.

local TRIGGER_LADY_DEATHWHISPER_ENTRANCE = 5709 -- TDB areatrigger_scripts row

local DATA_LADY_DEATHWHISPER = 1 -- icecrown_citadel.h:75
local ACTION_START_INTRO = 0 -- boss_lady_deathwhisper.cpp:165

local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_LADY_DEATHWHISPER_ENTRANCE then
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
    if data.ladyDeathwhisperEntranceDone then
        return
    end
    data.ladyDeathwhisperEntranceDone = true
    local lady = data.ladyDeathwhisper
    if lady and lady:GetBossState(DATA_LADY_DEATHWHISPER) ~= BOSS_STATE_DONE then
        lady:DoAction(ACTION_START_INTRO)
    end
end)
