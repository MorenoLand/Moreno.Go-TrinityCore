-- Gruul's Lair instance script — Lua port of
-- src/server/scripts/Outland/GruulsLair/instance_gruuls_lair.cpp
-- (instance_gruuls_lair, InstanceMapScript "instance_gruuls_lair",
-- map 565) and gruuls_lair.h (GLDataTypes, GLGameObjectIds).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_hellfire_ramparts.lua precedent).
--
-- PORTED — door data (instance_gruuls_lair.cpp:23-33 + InstanceScript::
-- UpdateDoorState, InstanceScript.cpp:188-215): GO_MAULGAR_DOOR (184468,
-- DOOR_TYPE_PASSAGE) opens iff DATA_MAULGAR is DONE; GO_GRUUL_DOOR
-- (184662, DOOR_TYPE_ROOM) opens iff DATA_GRUUL is NOT IN_PROGRESS
-- (open while NOT_STARTED or DONE, closed only mid-fight). Go fires no
-- event on boss-state writes, so the boss files write
-- instanceData.maulgardone / gruulInProgress in the same breath as
-- creature:SetBossState (the Ramparts flag pattern); the OnUpdate poll
-- (event 3, fires per Map::Update with players present) applies the
-- door states, retrying on later ticks if a door GO is unresolvable.
-- The OnGameObjectCreate handler (event 6) covers the dynamic-spawn
-- path and the OnPlayerEnter apply covers the static-spawn path
-- (static GOs never fire event 6 in Go — triggerInstanceGameObjectCreate
-- is dynamic-spawn-only, instance_hooks.go:488-503). Doors are
-- entry-resolved via player:FindNearestGameObject (lua_player.go:6710,
-- 1000y — the whole instance); the applied state is tracked in
-- instanceData (maulgardoorOpen / gruuldoorOpen) so re-fires are
-- no-ops. The C++ MaulgarGUID latch (OnCreatureCreate) has no
-- instance-side consumer left — its only readers are the four
-- council JustDied relays, already ported entry-based
-- (FindNearestCreature) in boss_high_king_maulgar.lua.
--
-- STAY-OUTS: SetHeaders ("GL") / SetBossNumber (2) — no bridges
-- (ICC precedent). MinionData (5 NPCs -> DATA_MAULGAR) — no bridge;
-- minion respawn/evade bookkeeping stays engine-side. The C++ file
-- has no GetSaveData/Load overrides, so nothing persists; the done
-- flags are runtime-only (the Ramparts done flags are the same).

local MAP_GRUULS_LAIR = 565

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- gruuls_lair.h GLGameObjectIds.
local GO_MAULGAR_DOOR = 184468
local GO_GRUUL_DOOR = 184662

local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1

local GO_SEARCH_DIST = 1000 -- whole-instance radius; one door per entry

-- Entry-based GO resolution from any player in the instance
-- (player:FindNearestGameObject covers DB spawns; the
-- instance_steam_vault.lua precedent).
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

-- C++ InstanceScript::UpdateDoorState (InstanceScript.cpp:188-215):
-- PASSAGE opens iff its boss is DONE; ROOM opens iff its boss is
-- NOT IN_PROGRESS. Reads the boss-written instanceData flags
-- (no boss-state-change event exists); idempotent across re-fires
-- via the tracked applied state.
local function updateDoors(instanceData, mapObj)
    local maulgarOpen = instanceData.maulgardone == true
    if instanceData.maulgardoorOpen ~= maulgarOpen then
        local door = findGO(mapObj, GO_MAULGAR_DOOR)
        if door then
            if maulgarOpen then
                door:SetGoState(GO_STATE_ACTIVE)
            else
                door:SetGoState(GO_STATE_READY)
            end
            instanceData.maulgardoorOpen = maulgarOpen
        end
    end
    local gruulOpen = not instanceData.gruulInProgress
    if instanceData.gruuldoorOpen ~= gruulOpen then
        local door = findGO(mapObj, GO_GRUUL_DOOR)
        if door then
            if gruulOpen then
                door:SetGoState(GO_STATE_ACTIVE)
            else
                door:SetGoState(GO_STATE_READY)
            end
            instanceData.gruuldoorOpen = gruulOpen
        end
    end
end

-- C++ OnGameObjectCreate (instance_gruuls_lair.cpp: LoadDoorData):
-- a door creation re-evaluates its state. Fires only for dynamic
-- spawns in Go; the static-spawn path is the OnPlayerEnter apply
-- below.
RegisterMapEvent(MAP_GRUULS_LAIR, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    if entry == GO_MAULGAR_DOOR or entry == GO_GRUUL_DOOR then
        updateDoors(instanceData, mapObj)
    end
end)

-- Static-spawn path for the door legs: re-apply on player enter
-- (idempotent — guarded by the tracked applied state).
RegisterMapEvent(MAP_GRUULS_LAIR, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    updateDoors(instanceData, mapObj)
end)

-- C++ SetBossState override via the OnUpdate poll: no
-- boss-state-change event exists, so the boss files' flags (see
-- header) drive the door states. Retries on later ticks if a
-- door GO is unresolvable.
RegisterMapEvent(MAP_GRUULS_LAIR, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    updateDoors(instanceData, mapObj)
end)
