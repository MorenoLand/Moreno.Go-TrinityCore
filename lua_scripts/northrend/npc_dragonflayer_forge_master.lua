-- Dragonflayer Forge Master — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/utgarde_keep.cpp
-- (class npc_dragonflayer_forge_master : public CreatureScript,
-- entry 24079 'Dragonflayer Forge Master', TDB 26091 creature_template
-- ScriptName 'npc_dragonflayer_forge_master', 3 spawns on map 574).
-- The forge-master drives the Utgarde forge machine: Reset writes
-- NOT_STARTED, engage writes IN_PROGRESS, death writes DONE to the
-- instance SetData DATA_FORGE_1/2/3 leg (instance_utgarde_keep.lua),
-- which opens/closes that forge's anvil, bellow and fire GOs.
-- Ported arms (C++-exact unless noted):
-- - Reset (utgarde_keep.cpp:47-58): derive _forgeId once (nearest
--   glowing anvil within 30yd, closest wins); if alive and bound,
--   SetData(DATA_FORGE_1 + _forgeId - 1, NOT_STARTED).
-- - JustDied (:60-68): same derivation, SetData(..., DONE).
-- - JustEngagedWith (:70-80): same derivation, SetData(...,
--   IN_PROGRESS); clears the emote state (EMOTE_ONESHOT_NONE).
-- - UpdateAI (:82-90): the lazy _forgeId derivation only — melee is
--   engine-side for Lua creatures and UpdateVictim has no Lua leg;
--   Reset already derives at spawn, so no ev24 leg is registered.
-- - GetForgeMasterType (:91-108): nearest GO_GLOWING_ANVIL_* within
--   30yd (FindNearestGameObject, 3D), closest by 2D distance wins
--   (IsWithinDist(..., false)); TDB geometry: each master sits
--   ~22yd from exactly one anvil, the other two 45yd+ away.

local ENTRY_FORGE_MASTER = 24079 -- TDB 26091 creature_template ScriptName

local DATA_FORGE_1 = 5 -- utgarde_keep.h:39
local ENCOUNTER_NOT_STARTED = 0 -- InstanceScript.h:70
local ENCOUNTER_IN_PROGRESS = 1 -- InstanceScript.h:71
local ENCOUNTER_DONE = 3 -- InstanceScript.h:73

local EMOTE_ONESHOT_NONE = 0 -- SharedDefines.h

local GO_GLOWING_ANVIL_1 = 186609 -- utgarde_keep.h:70
local GO_GLOWING_ANVIL_2 = 186610 -- utgarde_keep.h:71
local GO_GLOWING_ANVIL_3 = 186611 -- utgarde_keep.h:72

-- C++ ForgeSearch[3] (utgarde_keep.cpp:28-33).
local FORGE_SEARCH = { GO_GLOWING_ANVIL_1, GO_GLOWING_ANVIL_2, GO_GLOWING_ANVIL_3 }

-- C++ _forgeId, per creature (GetGUIDLow convention).
local forgeIdByGuid = {}

-- C++ GetForgeMasterType (utgarde_keep.cpp:91-108): the nearest
-- glowing anvil within 30yd; the running diff keeps the closest of
-- the three (IsWithinDist(go, diff, false) is the 2D check).
local function getForgeMasterType(creature)
    local diff = 30.0
    local id = 0
    for i = 1, 3 do
        local go = creature:FindNearestGameObject(FORGE_SEARCH[i], 30)
        if go ~= nil and creature:GetDistance2d(go) <= diff then
            id = i
            diff = creature:GetDistance2d(go)
        end
    end
    return id
end

local function forgeIdOf(creature)
    local guid = creature:GetGUIDLow()
    local id = forgeIdByGuid[guid]
    if not id then
        id = getForgeMasterType(creature)
        forgeIdByGuid[guid] = id
    end
    return id
end

-- The boss files reach the instance script through the shared
-- instanceData table (gruul precedent:
-- creature:GetMap():GetInstanceData()).
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

local function setForgeData(creature, data)
    local id = forgeIdOf(creature)
    if id ~= 0 then
        local instanceData = instanceDataOf(creature)
        if instanceData ~= nil then
            instanceData:SetData(DATA_FORGE_1 + id - 1, data)
        end
    end
end

-- C++ Reset (utgarde_keep.cpp:47-58).
local function onReset(event, creature)
    forgeIdOf(creature)
    if not creature:IsAlive() then
        return
    end
    setForgeData(creature, ENCOUNTER_NOT_STARTED)
end

-- C++ JustDied (utgarde_keep.cpp:60-68).
local function onDied(event, creature, killer)
    forgeIdOf(creature)
    setForgeData(creature, ENCOUNTER_DONE)
    forgeIdByGuid[creature:GetGUIDLow()] = nil
end

-- C++ JustEngagedWith (utgarde_keep.cpp:70-80).
local function onEngaged(event, creature, target)
    forgeIdOf(creature)
    setForgeData(creature, ENCOUNTER_IN_PROGRESS)
    creature:EmoteState(EMOTE_ONESHOT_NONE)
end

RegisterCreatureEvent(ENTRY_FORGE_MASTER, 1, onEngaged)
RegisterCreatureEvent(ENTRY_FORGE_MASTER, 4, onDied)
RegisterCreatureEvent(ENTRY_FORGE_MASTER, 23, onReset)
