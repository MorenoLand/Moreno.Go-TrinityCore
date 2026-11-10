-- Hellfire Ramparts instance script — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/HellfireRamparts/
-- instance_hellfire_ramparts.cpp (instance_ramparts,
-- InstanceMapScript "instance_ramparts", map 543).
-- RegisterMapEvent binds the map-level Eluna instance hook
-- (instance_gundrak.lua precedent).
--
-- PORTED — SetBossState fel-iron chest arm
-- (instance_hellfire_ramparts.cpp:64-80): when both
-- DATA_VAZRUDEN (2) and DATA_NAZAN (3) are DONE, the
-- fel-iron chest loses GO_FLAG_NOT_SELECTABLE. Go fires no
-- event on boss-state writes (setInstanceBossState,
-- instance_boss_states.go:162), so the two boss files
-- write instanceData.vazrudenDone / nazanDone = true in
-- their ev4 handlers in the same breath as
-- creature:SetBossState(DATA_X, DONE) (BossAI _JustDied,
-- ScriptedCreature.cpp:513-520 — the Gundrak pattern), and
-- the OnUpdate poll (event 3, fires per Map::Update with
-- players present) clears the chest flag once both flags
-- are set. Retries on later ticks if the chest GO is
-- unresolvable. The OnGameObjectCreate handler (event 6)
-- covers the dynamic-spawn path and the OnPlayerEnter
-- apply covers the static-spawn path (static GOs never fire
-- event 6 in Go — triggerInstanceGameObjectCreate is
-- dynamic-spawn-only, instance_hooks.go:488-503). The
-- chest is entry-resolved via player:FindNearestGameObject
-- (lua_player.go:6710, DB spawns + runtime GOs, 1000y —
-- the whole instance); only one of the two entries spawns,
-- normal 185168 or heroic 185169, so both are tried.
-- No GUID is stored in instanceData (identity-key hazard
-- + float64 low-bit loss, instance_gundrak.lua header).
--
-- STAY-OUTS: SetHeaders ("HR") / SetBossNumber (4) — no
-- bridges (ICC precedent). The C++ OnGameObjectCreate GUID
-- latch itself (felIronChestGUID member) has no Lua
-- counterpart — entry-based resolution above is the twin.
-- The C++ file has no GetSaveData/Load overrides, so
-- nothing persists; the done flags are runtime-only (the
-- Gundrak done flags are the same).

local MAP_HELLFIRE_RAMPARTS = 543

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- hellfire_ramparts.h HRGameobjectIds.
local GO_FEL_IRON_CHEST_NORMAL = 185168
local GO_FEL_IRON_CHEST_HEROIC = 185169

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

local GO_SEARCH_DIST = 1000 -- whole-instance radius; one chest spawns

-- Entry-based chest resolution from any player in the
-- instance (player:FindNearestGameObject covers DB spawns;
-- the entering player is passed directly by the
-- OnPlayerEnter handler).
local function findChestFromPlayer(player, entry)
    if player == nil then
        return nil
    end
    return player:FindNearestGameObject(entry, GO_SEARCH_DIST)
end

local function findChest(mapObj)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local go = findChestFromPlayer(player, GO_FEL_IRON_CHEST_NORMAL)
            or findChestFromPlayer(player, GO_FEL_IRON_CHEST_HEROIC)
        if go then
            return go
        end
    end
    return nil
end

local function findChestForPlayer(player)
    return findChestFromPlayer(player, GO_FEL_IRON_CHEST_NORMAL)
        or findChestFromPlayer(player, GO_FEL_IRON_CHEST_HEROIC)
end

-- C++ SetBossState override (instance_hellfire_ramparts.cpp:
-- 64-80): both bosses DONE -> chest selectable. Idempotent
-- (guarded by instanceData.chestCleared).
local function tryClearChest(instanceData, mapObj, go)
    if instanceData.chestCleared then
        return
    end
    if not (instanceData.vazrudenDone and instanceData.nazanDone) then
        return
    end
    if go == nil then
        go = findChest(mapObj)
    end
    if go then
        go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        instanceData.chestCleared = true
    end
end

-- C++ OnGameObjectCreate (instance_hellfire_ramparts.cpp:
-- 50-61): the chest latch. Fires only for dynamic spawns
-- in Go; the static-spawn path is the OnPlayerEnter apply
-- below.
RegisterMapEvent(MAP_HELLFIRE_RAMPARTS, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    if entry == GO_FEL_IRON_CHEST_NORMAL or entry == GO_FEL_IRON_CHEST_HEROIC then
        tryClearChest(instanceData, mapObj, go)
    end
end)

-- Static-spawn path for the chest arm: the entering player
-- is the search anchor (idempotent — guarded by
-- instanceData.chestCleared).
RegisterMapEvent(MAP_HELLFIRE_RAMPARTS, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    tryClearChest(instanceData, mapObj, findChestForPlayer(player))
end)

-- C++ SetBossState override via the OnUpdate poll: no
-- boss-state-change event exists, so the boss files' DONE
-- flags (see header) drive the once-only chest flag clear.
-- Retries on later ticks if the chest GO is unresolvable.
RegisterMapEvent(MAP_HELLFIRE_RAMPARTS, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    tryClearChest(instanceData, mapObj, nil)
end)
