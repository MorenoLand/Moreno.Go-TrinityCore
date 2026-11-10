-- Magtheridon's Lair instance script — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/MagtheridonsLair/
-- instance_magtheridons_lair.cpp (instance_magtheridons_lair,
-- InstanceMapScript "instance_magtheridons_lair", map 544) and
-- magtheridons_lair.h (MLDataTypes, MLActions, MLCreatureIds,
-- MLGameObjectIds).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_gruuls_lair.lua / instance_steam_vault.lua precedent).
--
-- PORTED — door data (instance_magtheridons_lair.cpp:31-35 +
-- InstanceScript::UpdateDoorState, InstanceScript.cpp:188-215):
-- GO_MAGTHERIDON_DOOR (183847, DOOR_TYPE_ROOM) opens iff
-- DATA_MAGTHERIDON is NOT IN_PROGRESS. Go fires no event on
-- boss-state writes, so boss_magtheridon.lua writes
-- instanceData.magtheridonInProgress in the same breath as
-- creature:SetBossState (the Ramparts/Gruul flag pattern); the
-- OnUpdate poll (event 3) applies the door state, retrying on
-- later ticks if the door GO is unresolvable. The
-- OnGameObjectCreate handler (event 6) covers the dynamic-spawn
-- path and the OnPlayerEnter apply covers the static-spawn path
-- (static GOs never fire event 6 in Go — dynamic-spawn-only,
-- instance_hooks.go:488-503). The door is entry-resolved via
-- player:FindNearestGameObject (lua_player.go:6710, 1000y — the
-- whole instance); the applied state is tracked in
-- instanceData (magtheridonDoorOpen) so re-fires are no-ops.
-- PORTED — SetData machine (instance_magtheridons_lair.cpp:99-133,
-- event 8; the instanceData:SetData bridge fires event 8 with
-- (event, instanceData, mapObj, type, value),
-- instance_hooks.go:51-63): DATA_MANTICRON_CUBE (9):
-- ACTION_ENABLE (1) clears GO_FLAG_NOT_SELECTABLE on every cube
-- (181713), otherwise sets it; DATA_COLLAPSE (10): the hall
-- (184653) goes GO_STATE_ACTIVE iff ENABLE (HandleGameObject
-- twin: open -> ACTIVE 0, closed -> READY 1,
-- InstanceScript.cpp:479-487); DATA_COLLAPSE_2 (11): all six
-- columns (184638/184639/184635/184634/184636/184637) the same;
-- DATA_CALL_WARDERS (12): every alive Hellfire Warder (18829)
-- DoZoneInCombat. Cubes and warders resolve entry-based from
-- Magtheridon (GetGameObjectsInGrid / GetCreaturesInGrid, 500y
-- razorscale precedent); the hall and columns resolve entry-based
-- from a map player (Steam Vault precedent).
-- PORTED — the OnGameObjectCreate / OnCreatureCreate GUID latches
-- (cubesGUIDS / warderGUIDS, :83-97) have no Lua counterpart:
-- the instance event-5/6 hooks are dynamic-spawn-only in Go, so
-- static spawns never latch — the entry-based grid resolution
-- above is the twin (the Slave Pens re-triage).
--
-- STAY-OUTS: SetHeaders ("ML") / SetBossNumber (1) — no bridges
-- (ICC precedent). BossBoundaryData (circle (-18.70, 2.24) r52.30
-- for DATA_MAGTHERIDON, :26-29) — no bridge; the boundary stays
-- engine-side. LoadObjectData (creatureData: 17257 ->
-- DATA_MAGTHERIDON, 21252 -> DATA_WORLD_TRIGGER; gameObjectData:
-- hall + 6 columns, :37-55) — no load bridge; the
-- DATA_MAGTHERIDON latch rides creature:SetGuidData(0) in
-- boss_magtheridon.lua's ev5 spawn handler (illidari council
-- precedent), the hall / columns resolve entry-based above, and
-- the world trigger (21252) resolves via the boss file's
-- FindNearestCreature grid twin in the debris-knockdown leg.
-- The C++ file has no GetSaveData/Load overrides, so nothing
-- persists; the done flags are runtime-only (the Ramparts done
-- flags are the same).

local MAP_MAGTHERIDONS_LAIR = 544

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_SET_DATA = 8

-- magtheridons_lair.h MLDataTypes.
local DATA_MANTICRON_CUBE = 9
local DATA_COLLAPSE = 10
local DATA_COLLAPSE_2 = 11
local DATA_CALL_WARDERS = 12

-- magtheridons_lair.h MLActions.
local ACTION_ENABLE = 1

-- magtheridons_lair.h MLCreatureIds / MLGameObjectIds.
local NPC_MAGTHERIDON = 17257
local NPC_HELLFIRE_WARDER = 18829
local GO_MAGTHERIDON_DOOR = 183847
local GO_MANTICRON_CUBE = 181713
local GO_MAGTHERIDON_HALL = 184653
local GO_MAGTHERIDON_COLUMNS = { 184638, 184639, 184635, 184634, 184636, 184637 }

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO
local GRID_SEARCH_DIST = 500 -- razorscale arena precedent

-- Entry-based GO/creature resolution from any player in the
-- instance (player:FindNearestGameObject / FindNearestCreature
-- cover DB spawns; the instance_steam_vault.lua precedent).
local function findGO(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local go = player:FindNearestGameObject(entry, GO_SEARCH_DIST)
        if go then
            return go
        end
    end
    return nil
end

local function findMagtheridon(mapObj)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local creature = player:FindNearestCreature(NPC_MAGTHERIDON, GO_SEARCH_DIST)
        if creature then
            return creature
        end
    end
    return nil
end

-- C++ InstanceScript::UpdateDoorState (InstanceScript.cpp:188-215):
-- ROOM opens iff its boss is NOT IN_PROGRESS. Reads the
-- boss-written instanceData flag (no boss-state-change event
-- exists); idempotent across re-fires via the tracked applied
-- state.
local function updateDoor(instanceData, mapObj)
    local doorOpen = not instanceData.magtheridonInProgress
    if instanceData.magtheridonDoorOpen ~= doorOpen then
        local door = findGO(mapObj, GO_MAGTHERIDON_DOOR)
        if door then
            if doorOpen then
                door:SetGoState(GO_STATE_ACTIVE)
            else
                door:SetGoState(GO_STATE_READY)
            end
            instanceData.magtheridonDoorOpen = doorOpen
        end
    end
end

-- C++ OnGameObjectCreate (instance_magtheridons_lair.cpp:78-80):
-- a door creation re-evaluates its state. Fires only for dynamic
-- spawns in Go; the static-spawn path is the OnPlayerEnter apply
-- below.
RegisterMapEvent(MAP_MAGTHERIDONS_LAIR, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    if go:GetEntry() == GO_MAGTHERIDON_DOOR then
        updateDoor(instanceData, mapObj)
    end
end)

-- Static-spawn path for the door leg: re-apply on player enter
-- (idempotent — guarded by the tracked applied state).
RegisterMapEvent(MAP_MAGTHERIDONS_LAIR, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    updateDoor(instanceData, mapObj)
end)

-- C++ SetBossState override via the OnUpdate poll: no
-- boss-state-change event exists, so the boss file's flag (see
-- header) drives the door state. Retries on later ticks if the
-- door GO is unresolvable.
RegisterMapEvent(MAP_MAGTHERIDONS_LAIR, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    updateDoor(instanceData, mapObj)
end)

-- C++ SetData (instance_magtheridons_lair.cpp:99-133).
RegisterMapEvent(MAP_MAGTHERIDONS_LAIR, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    if type == DATA_MANTICRON_CUBE then
        -- C++ :103-112: every cube loses NOT_SELECTABLE iff ENABLE.
        local magtheridon = findMagtheridon(mapObj)
        if magtheridon then
            for _, cube in ipairs(magtheridon:GetGameObjectsInGrid(GO_MANTICRON_CUBE, GRID_SEARCH_DIST)) do
                if value == ACTION_ENABLE then
                    cube:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
                else
                    cube:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
                end
            end
        end
    elseif type == DATA_COLLAPSE then
        -- C++ :113-116: the hall opens iff ENABLE.
        local hall = findGO(mapObj, GO_MAGTHERIDON_HALL)
        if hall then
            if value == ACTION_ENABLE then
                hall:SetGoState(GO_STATE_ACTIVE)
            else
                hall:SetGoState(GO_STATE_READY)
            end
        end
    elseif type == DATA_COLLAPSE_2 then
        -- C++ :117-121: all six columns open iff ENABLE.
        for _, entry in ipairs(GO_MAGTHERIDON_COLUMNS) do
            local column = findGO(mapObj, entry)
            if column then
                if value == ACTION_ENABLE then
                    column:SetGoState(GO_STATE_ACTIVE)
                else
                    column:SetGoState(GO_STATE_READY)
                end
            end
        end
    elseif type == DATA_CALL_WARDERS then
        -- C++ :122-129: every alive warder DoZoneInCombat.
        local magtheridon = findMagtheridon(mapObj)
        if magtheridon then
            for _, warder in ipairs(magtheridon:GetCreaturesInGrid(NPC_HELLFIRE_WARDER, GRID_SEARCH_DIST)) do
                if warder:IsAlive() then
                    warder:DoZoneInCombat()
                end
            end
        end
    end
end)
