-- Blackrock Stadium area trigger (Blackrock Spire) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockSpire/
-- instance_blackrock_spire.cpp:574-596
-- (class at_blackrock_stadium : public AreaTriggerScript).
-- Trigger 2026 (TDB areatrigger_scripts row for at_blackrock_stadium;
-- at_census_2026-11-15.md). Fire site: engine/world/areatrigger.go:64
-- (TriggerServerEvent, ServerEventAreaTrigger = 24); handlers
-- registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) (the at_ring_of_law.lua precedent).
-- PORTED (2026-11-21): C++ OnTrigger — player alive gate, Rend found
-- via FindNearestCreature(NPC_WARCHIEF_REND_BLACKHAND 10429, 50.0f),
-- rend->AI()->SetData(AREATRIGGER 1, AREATRIGGER_BLACKROCK_STADIUM
-- 2026) (blackrock_spire.h:88/:90), which fires ev41 on Rend and
-- starts the pre-fight gyth event chain (boss_rend_blackhand.lua).
-- The C++ return value is moot here (handler return discarded).
-- No GM gate in C++ for this trigger (unlike the Beast triggers).

local TRIGGER_BLACKROCK_STADIUM = 2026 -- TDB areatrigger_scripts row
local NPC_WARCHIEF_REND_BLACKHAND = 10429 -- blackrock_spire.h:70
local AREATRIGGER = 1 -- blackrock_spire.h:88
local AREATRIGGER_BLACKROCK_STADIUM = 2026 -- blackrock_spire.h:90

-- C++ at_blackrock_stadium::OnTrigger (:578-594).
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_BLACKROCK_STADIUM then
        return
    end
    if not player:IsAlive() then
        return
    end
    local rend = player:FindNearestCreature(NPC_WARCHIEF_REND_BLACKHAND, 50.0)
    if rend then
        rend:SetData(AREATRIGGER, AREATRIGGER_BLACKROCK_STADIUM)
    end
end)
