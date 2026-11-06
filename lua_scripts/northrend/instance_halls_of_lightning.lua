-- The Halls of Lightning instance script — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/instance_halls_of_lightning.cpp
-- (class instance_halls_of_lightning : public InstanceMapScript, map 602).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua / instance_ulduar.lua precedent); its presence
-- opens the Map:GetInstanceData gate for map 602
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- bjarngrim / volkhan / ionar / loken — the creature objects bound by
-- ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches,
-- instance_halls_of_lightning.cpp:44-68); lokenDone — written by
-- Loken's onDied handler (boss_loken.lua, the drakos/maexxna
-- precedent); lokenProcessed — owned by the ON_UPDATE poll below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the four boss binds above. Fires on
--   materialization (motionFor funnel, instance_hooks.go:311-320);
--   a static template spawn never materializes server-side and is
--   never bound (naxxramas LoadGrid delta).
-- - SetBossState DATA_LOKEN DONE (instance_halls_of_lightning.cpp:
--   87-93) -> LokenGlobe SendCustomAnim(0), re-expressed at DONE
--   time via the ON_UPDATE poll (naxxramas/oculus precedent): the
--   globe is found via FindNearestGameObject(GO_LOKEN_THRONE,
--   1000) from the bound Loken (maexxna precedent) and
--   SendCustomAnim(0) is bridged (lua_objects.go:4481).
-- - GetGuidData (instance_halls_of_lightning.cpp:96-113): the Lua
--   mirror is the instanceData binds above, read as
--   instanceData.<boss> by the boss files (oculus precedent); the
--   C++ consumers (boss_bjarngrim.cpp:425/:448, boss_ionar.cpp:356,
--   ObjectAccessor::GetCreature over the GUID) ride unported
--   cross-creature machines in those boss files, so the GUID legs
--   as consumed stay out there.
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_VOLKHAN_DOOR 191325 / GO_IONAR_DOOR 191326 /
--   GO_LOKEN_DOOR 191324, DOOR_TYPE_PASSAGE tied to DATA_VOLKHAN /
--   DATA_IONAR / DATA_LOKEN, instance_halls_of_lightning.cpp:24-30):
--   no boss-state door machine in Go (oculus/naxxramas precedent).
--   GO_BJARNGRIM_DOOR 191416 is not in doorData at all.
-- - OnGameObjectCreate LokenGlobe latch (instance_halls_of_lightning
--   .cpp:70-82): static template GOs never pass the Go GO-create
--   funnel (instance_hooks.go:291-306, oculus precedent), so the
--   latch is vacuous here and the DONE-time FindNearestGameObject
--   form above is the live one.
-- - SetHeaders(DataHeader "HOL") / SetBossNumber(EncounterCount 4) /
--   LoadDoorData: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5908) and read via
--   creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).

local MAP_HALLS_OF_LIGHTNING = 602
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local DATA_BJARNGRIM = 0
local DATA_VOLKHAN = 1
local DATA_IONAR = 2
local DATA_LOKEN = 3

local NPC_BJARNGRIM = 28586 -- halls_of_lightning.h:39
local NPC_VOLKHAN = 28587 -- halls_of_lightning.h:40
local NPC_IONAR = 28546 -- halls_of_lightning.h:41
local NPC_LOKEN = 28923 -- halls_of_lightning.h:42

local GO_LOKEN_THRONE = 192654 -- halls_of_lightning.h:53

local bossBinds = {
    [NPC_BJARNGRIM] = "bjarngrim",
    [NPC_VOLKHAN] = "volkhan",
    [NPC_IONAR] = "ionar",
    [NPC_LOKEN] = "loken",
}

RegisterMapEvent(MAP_HALLS_OF_LIGHTNING, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)

RegisterMapEvent(MAP_HALLS_OF_LIGHTNING, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.lokenDone and not instanceData.lokenProcessed then
        instanceData.lokenProcessed = true
        -- C++ SetBossState DATA_LOKEN DONE (instance_halls_of_lightning.cpp:87-93):
        -- LokenGlobe SendCustomAnim(0).
        if instanceData.loken then
            local globe = instanceData.loken:FindNearestGameObject(GO_LOKEN_THRONE, 1000)
            if globe then
                globe:SendCustomAnim(0)
            end
        end
    end
end)
