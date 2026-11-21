-- The Beast area triggers (Blackrock Spire) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockSpire/
-- boss_the_beast.cpp:219-262
-- (class at_trigger_the_beast_movement : public AreaTriggerScript;
-- class at_the_beast_room : public AreaTriggerScript).
-- Triggers 2066 (movement) / 2067 (room) (TDB areatrigger_scripts
-- rows; at_census_2026-11-15.md). Fire site:
-- engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the at_ring_of_law.lua precedent).
-- PORTED (2026-11-21): C++ OnTrigger — GM gate, instance GetGuidData
-- (DATA_THE_BEAST) -> beast->AI()->SetData(DATA_BEAST_REACHED 1, 1)
-- / SetData(DATA_BEAST_ROOM 2, 2), consumed by the ev41 handler in
-- boss_the_beast.lua.
-- DELTAS: (1) the GM gate is unmodeled (no player:IsGameMaster
-- bridge); (2) the Beast binds via player:FindNearestCreature(10430,
-- 200) instead of instance GetGuidData (no instance_blackrock_spire
-- .lua); the Beast is the only 10430 in the instance, so the
-- proximity bind resolves to the same creature.

local TRIGGER_BEAST_MOVEMENT = 2066 -- TDB areatrigger_scripts row
local TRIGGER_BEAST_ROOM = 2067 -- TDB areatrigger_scripts row
local NPC_THE_BEAST = 10430 -- blackrock_spire.h:72
local DATA_BEAST_REACHED = 1 -- boss_the_beast.cpp:39
local DATA_BEAST_ROOM = 2 -- boss_the_beast.cpp:40

-- C++ at_trigger_the_beast_movement::OnTrigger (:223-242) and
-- at_the_beast_room::OnTrigger (:244-262).
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_BEAST_MOVEMENT and triggerId ~= TRIGGER_BEAST_ROOM then
        return
    end
    if not player:IsAlive() then
        return
    end
    local beast = player:FindNearestCreature(NPC_THE_BEAST, 200.0)
    if not beast then
        return
    end
    if triggerId == TRIGGER_BEAST_MOVEMENT then
        beast:SetData(DATA_BEAST_REACHED, DATA_BEAST_REACHED)
    else
        beast:SetData(DATA_BEAST_ROOM, DATA_BEAST_ROOM)
    end
end)
