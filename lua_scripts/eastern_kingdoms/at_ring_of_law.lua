-- Ring of Law area trigger (Blackrock Depths) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/blackrock_depths.cpp:95-115
-- (class at_ring_of_law : public AreaTriggerScript).
-- Trigger 1526 (TDB 335.26091 areatrigger_scripts row for
-- at_ring_of_law). Fire site: engine/world/areatrigger.go:64
-- (TriggerServerEvent, ServerEventAreaTrigger = 24); handlers
-- registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) (the sartura / at_scripts.lua
-- precedent). The handler return is discarded, so the C++
-- `return false` legs are moot here.
-- PORTED 2026-11-15 (at_* census): the gate reads the
-- instanceData.ringOfLaw store added to
-- instance_blackrock_depths.lua (SetData TYPE_RING_OF_LAW 1,
-- blackrock_depths.cpp:234, encounter[0] = data; DONE writes
-- save). The Ring-of-Law UpdateAI event machine itself
-- (blackrock_depths.cpp:275-360) stays out with
-- npc_grimstone.lua — this trigger only starts it, via the
-- Grimstone summon; the wp5 DONE write already rides the same
-- store.

local TRIGGER_RING_OF_LAW = 1526 -- TDB areatrigger_scripts row

local TYPE_RING_OF_LAW = 1 -- blackrock_depths.h:28
local NPC_GRIMSTONE = 10096 -- blackrock_depths.cpp:101
local SUMMON_TYPE_DEAD_DESPAWN = 6 -- lua_creature_events.go:3699-3704

local ENCOUNTER_IN_PROGRESS = 1 -- InstanceScript.h:71
local ENCOUNTER_DONE = 3 -- InstanceScript.h:73

-- C++ :103-113 at_ring_of_law::OnTrigger.
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_RING_OF_LAW then
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
    local state = data.ringOfLaw or 0
    if state == ENCOUNTER_IN_PROGRESS or state == ENCOUNTER_DONE then
        return
    end
    data:SetData(TYPE_RING_OF_LAW, ENCOUNTER_IN_PROGRESS)
    player:SummonCreature(NPC_GRIMSTONE, 625.559, -205.618, -52.735, 2.609, SUMMON_TYPE_DEAD_DESPAWN)
end)
