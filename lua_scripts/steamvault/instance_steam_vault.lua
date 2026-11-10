-- Steam Vault instance script — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SteamVault/
-- instance_steam_vault.cpp (instance_steam_vault,
-- InstanceMapScript "instance_steam_vault", map 545) and
-- steam_vault.h (SVDataTypes, SVMisc).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_serpent_shrine.lua precedent).
--
-- PORTED — CheckMainDoor (instance_steam_vault.cpp:100-114):
-- both DATA_HYDROMANCER_THESPIA (0) and DATA_MEKGINEER_
-- STEAMRIGGER (1) DONE -> the door controller (20926) Talks
-- CONTROLLER_TEXT_MAIN_DOOR_OPEN (1), then the main-chambers
-- door (183049) goes GO_STATE_ACTIVE + GO_FLAG_NOT_SELECTABLE.
-- Go fires no event on boss-state writes, so the two boss files
-- write instanceData.thespiaDone / steamriggerDone = true in
-- their ev4 handlers in the same breath as creature:SetBossState
-- (DATA_X, DONE) (the Ramparts flag pattern); CheckMainDoor
-- reads those flags. It runs ONLY on the C++ trigger sites:
-- the access-panel gossip (SetData ACTION_OPEN_DOOR, event 8
-- below) and main-door creation (event 6) — there is NO
-- OnUpdate poll: C++ never opens the door on boss death alone,
-- and a poll would fabricate the auto-open. The controller
-- Talk and the door GO resolve entry-based (FindNearestCreature
-- / FindNearestGameObject from a map player, 1000y whole-
-- instance radius); the C++ GUID latch (LoadObjectData) has no
-- Lua counterpart. The main-door legs are idempotent across
-- re-fires (C++ has no latch either — a door respawn re-fires
-- CheckMainDoor and re-Talks, same as C++).
-- PORTED — SetData machine (instance_steam_vault.cpp:116-123):
-- ACTION_OPEN_DOOR (1) -> CheckMainDoor; DATA_DISTILLER (3) ->
-- instanceData.distillerState (the distiller writer/reader
-- lives in the unregistered npc_naga_distiller arms of
-- boss_warlord_kalithresh.cpp:72-94/:156, so the store has no
-- writer yet — the SSC DATA_TRASH precedent: machine
-- implemented, nothing increments it until the distiller
-- script lands).
-- PORTED — OnGameObjectCreate (:92-98): the main door (183049)
-- -> CheckMainDoor. The access-panel / door-controller GUID
-- latches (LoadObjectData) have no Lua counterpart — entry-
-- based resolution above is the twin.
-- PORTED — access-panel gossip legs (go_main_chambers_access_
-- panel.lua split, the go_bridge_console.lua precedent):
-- controller Talk(CONTROLLER_TEXT_ACESS_USED 0) + SetData
-- (ACTION_OPEN_DOOR, 0) + panel SetFlag NOT_SELECTABLE +
-- SetGoState ACTIVE, unconditional as in C++ (:36-44).
-- PORTED — SetBossState panel arms (:134-153): on DONE the
-- boss's own access panel (hydro 184125 / mek 184126) loses
-- GO_FLAG_NOT_SELECTABLE. Go fires no event on boss-state
-- writes, so each boss file clears its own panel in ev4 via
-- creature:GetGameObjectsInGrid (the arlokk gong precedent) —
-- C++-exact timing (the override fires at death), no poll.
--
-- STAY-OUTS: SetHeaders ("SV") / SetBossNumber (3) — no
-- bridges (ICC precedent). GetData(DATA_DISTILLER) reads in
-- boss_warlord_kalithresh.cpp stay in that file (its distiller
-- arms are documented-only; the plain instanceData.distillerState
-- field is the read shape when they land). The C++ file has no
-- GetSaveData/Load overrides, so nothing persists; the done
-- flags are runtime-only (the Ramparts done flags are the same).

local MAP_STEAM_VAULT = 545

local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_SET_DATA = 8

-- steam_vault.h SVDataTypes.
local ACTION_OPEN_DOOR = 1
local DATA_DISTILLER = 3

local CONTROLLER_TEXT_MAIN_DOOR_OPEN = 1

local NPC_COILFANG_DOOR_CONTROLLER = 20926

local GO_MAIN_CHAMBERS_DOOR = 183049

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 0

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

-- Entry-based GO/creature resolution from any player in the
-- instance (player:FindNearestGameObject / FindNearestCreature
-- cover DB spawns; the instance_serpent_shrine.lua precedent).
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

local function findCreature(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local creature = player:FindNearestCreature(entry, GO_SEARCH_DIST)
        if creature then
            return creature
        end
    end
    return nil
end

-- C++ CheckMainDoor (instance_steam_vault.cpp:100-114): both
-- bosses DONE -> controller Talk + main door ACTIVE +
-- NOT_SELECTABLE. Reads the boss-written instanceData flags
-- (no boss-state-change event exists).
local function checkMainDoor(instanceData, mapObj)
    if not (instanceData.thespiaDone and instanceData.steamriggerDone) then
        return
    end
    local controller = findCreature(mapObj, NPC_COILFANG_DOOR_CONTROLLER)
    if controller then
        controller:Talk(CONTROLLER_TEXT_MAIN_DOOR_OPEN)
    end
    local mainDoor = findGO(mapObj, GO_MAIN_CHAMBERS_DOOR)
    if mainDoor then
        mainDoor:SetGoState(GO_STATE_ACTIVE)
        mainDoor:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
end

-- C++ OnGameObjectCreate (instance_steam_vault.cpp:92-98):
-- the main door -> CheckMainDoor.
RegisterMapEvent(MAP_STEAM_VAULT, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    if go:GetEntry() == GO_MAIN_CHAMBERS_DOOR then
        checkMainDoor(instanceData, mapObj)
    end
end)

-- Static-spawn path for the door leg: re-run CheckMainDoor on
-- player enter (idempotent — it only acts once both bosses
-- are done; the Gundrak precedent).
RegisterMapEvent(MAP_STEAM_VAULT, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    checkMainDoor(instanceData, mapObj)
end)

-- C++ SetData (instance_steam_vault.cpp:116-123).
RegisterMapEvent(MAP_STEAM_VAULT, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    if type == ACTION_OPEN_DOOR then
        checkMainDoor(instanceData, mapObj)
    elseif type == DATA_DISTILLER then
        instanceData.distillerState = value
    end
end)
