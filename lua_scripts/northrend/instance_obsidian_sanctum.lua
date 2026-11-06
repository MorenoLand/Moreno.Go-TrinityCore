-- The Obsidian Sanctum instance script — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/instance_obsidian_sanctum.cpp
-- (class instance_obsidian_sanctum : public InstanceMapScript, map 615).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_oculus.lua / instance_eye_of_eternity.lua precedent).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: Sartharion / Tenebron / Shadron / Vesperon bound
--   into instanceData (the C++ GUID latches, instance_obsidian_sanctum.cpp:60-78)
--   + SetFarVisible(true) on the three drakes (bridged,
--   lua_creature_events.go:5133). The GUID-store side of the latch is
--   served by the ev5 SetGuidData handlers in boss_sartharion.lua /
--   obsidian_sanctum.lua (the Go guid-store route); this file binds the
--   live objects for instance-level legs.
-- Unmodeled (documented-only, no bridges):
-- - setActive(true) on the three drakes: no SetActive bridge.
-- - SetBossState override (instance_obsidian_sanctum.cpp:81-99): vacuous —
--   the switch has no payload legs, it only calls the base.
-- - GetGuidData: the read side is served by the Go guid store
--   (creature:GetGuidData / GetInstanceCreature).
-- - Boss boundaries (RectangleBoundary 3218.86-3275.69 / 484.68-572.4):
--   no boundary bridge (naxxramas precedent).
-- - SaveToDB / instance-data persistence: the table lives for the
--   runtime lifetime only (black-morass delta).

local MAP_OBSIDIAN_SANCTUM = 615
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_SARTHARION = 28860 -- obsidian_sanctum.h:40
local NPC_TENEBRON = 30452 -- obsidian_sanctum.h:41
local NPC_SHADRON = 30451 -- obsidian_sanctum.h:42
local NPC_VESPERON = 30449 -- obsidian_sanctum.h:43

local creatureBinds = {
    [NPC_SARTHARION] = "sartharion",
    [NPC_TENEBRON] = "tenebron",
    [NPC_SHADRON] = "shadron",
    [NPC_VESPERON] = "vesperon",
}

-- C++ OnCreatureCreate (instance_obsidian_sanctum.cpp:58-79): latch the
-- GUID + setActive(true) + SetFarVisible(true) on the three drakes.
-- setActive has no bridge (documented above); the far-visible leg ports.
RegisterMapEvent(MAP_OBSIDIAN_SANCTUM, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = creatureBinds[creature:GetEntry()]
    if not key then
        return
    end
    instanceData[key] = creature
    if key ~= "sartharion" then
        creature:SetFarVisible(true)
    end
end)
