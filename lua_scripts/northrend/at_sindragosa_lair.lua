-- Sindragosa's lair area trigger (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sindragosa.cpp:1481-1511
-- (class at_sindragosa_lair : public AreaTriggerScript).
-- Trigger 5604 (TDB 335.26091 areatrigger_scripts row for
-- at_sindragosa_lair). Fire site: engine/world/areatrigger.go:64
-- (TriggerServerEvent, ServerEventAreaTrigger = 24); handlers
-- registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerID) (the sartura / at_scripts.lua
-- precedent). The handler return is discarded, so the C++
-- `return true` legs are moot here.
-- PORTED 2026-11-15 (at_* census). The C++ trash-set / frostwyrm-
-- set GetData reads ride the live-sweep twins from
-- instance_icecrown_citadel.lua's OnUnitDeath legs (the same
-- three gates, ported 2026-11-10): the whelp's home Y
-- (< 2484.35f = RIMEFANG, else SPINESTALKER) picks the owner,
-- and GetCreaturesInGrid(37532, 500) filtered to live whelps of
-- that owner stands in for the SpinestalkerTrash / RimefangTrash
-- sets; the FrostwyrmGUIDs-empty gate is a live-frostwyrm sweep.
-- The receivers are PORTED (boss_sindragosa.lua:
-- frostwyrmDoAction ev38 on both frostwyrms, sindragosaDoAction
-- ev38 on Sindragosa). Deltas (same as the OnUnitDeath twin):
-- the heroic-attempts gate (DATA_HEROIC_ATTEMPTS 29) has no
-- bridge — the attempts machine is unmodeled, so a heroic run
-- with 0 attempts left would wrongly summon; LoadGrid has no
-- bridge. Whelps in never-loaded grids are invisible to the
-- sweep, so a frostwyrm can fire early if one hall's whelps
-- never materialized. If neither frostwyrm is bound there is no
-- sweep origin and no boss-state probe, so all three legs skip.

local TRIGGER_SINDRAGOSA_LAIR = 5604 -- TDB areatrigger_scripts row

local NPC_FROSTWING_WHELP = 37532 -- icecrown_citadel.h:308
local NPC_SPINESTALKER = 37534 -- icecrown_citadel.h:305
local NPC_RIMEFANG = 37533 -- icecrown_citadel.h:306
local NPC_SINDRAGOSA = 36853 -- boss_sindragosa.cpp:169
local SINDRAGOSA_SPAWN_POS = { 4818.7, 2483.71, 287.065, 3.089233 } -- boss_sindragosa.cpp:169

local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local ACTION_START_FROSTWYRM = -368530 -- icecrown_citadel.h:507

local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76
local RIMEFANG_HOME_Y = 2484.35 -- instance_icecrown_citadel.cpp:464

-- First bound creature wins (the instance file's firstBoundCreature
-- idiom): any creature in the instance reads the shared boss-state
-- store and anchors a grid sweep.
local function firstBoundCreature(data)
    return data.spinestalker or data.rimefang or data.sindragosa
        or data.lordMarrowgar or data.ladyDeathwhisper
        or data.deathbringerSaurfang or data.theLichKing
end

-- C++ instance GetData(DATA_SPINESTALKER / DATA_RIMEFANG) is the
-- trash-set size (instance_icecrown_citadel.cpp:736-739) — the live
-- sweep twin: any live whelp of that owner in the grid.
local function hasLiveWhelp(origin, ownerIsRimefang)
    for _, whelp in ipairs(origin:GetCreaturesInGrid(NPC_FROSTWING_WHELP, 500)) do
        if not whelp:IsDead() then
            local _, homeY = whelp:GetHomePosition()
            if (homeY < RIMEFANG_HOME_Y) == ownerIsRimefang then
                return true
            end
        end
    end
    return false
end

-- C++ instance GetData(DATA_SINDRAGOSA_FROSTWYRMS) is the
-- FrostwyrmGUIDs size (:734-735) — the live sweep twin.
local function hasLiveFrostwyrm(origin)
    for _, entry in ipairs({ NPC_SPINESTALKER, NPC_RIMEFANG }) do
        for _, frostwyrm in ipairs(origin:GetCreaturesInGrid(entry, 500)) do
            if not frostwyrm:IsDead() then
                return true
            end
        end
    end
    return false
end

-- C++ :1486-1510 at_sindragosa_lair::OnTrigger, in C++ arm order.
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_SINDRAGOSA_LAIR then
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
    local origin = firstBoundCreature(data)
    if not origin then
        return
    end
    local spinestalker = data.spinestalker
    if spinestalker and not hasLiveWhelp(origin, false) then
        spinestalker:DoAction(ACTION_START_FROSTWYRM)
    end
    local rimefang = data.rimefang
    if rimefang and not hasLiveWhelp(origin, true) then
        rimefang:DoAction(ACTION_START_FROSTWYRM)
    end
    if not hasLiveFrostwyrm(origin) and data.sindragosa == nil
        and origin:GetBossState(DATA_SINDRAGOSA) ~= BOSS_STATE_DONE then
        local boss = player:SummonCreature(NPC_SINDRAGOSA,
            SINDRAGOSA_SPAWN_POS[1], SINDRAGOSA_SPAWN_POS[2],
            SINDRAGOSA_SPAWN_POS[3], SINDRAGOSA_SPAWN_POS[4])
        if boss then
            boss:DoAction(ACTION_START_FROSTWYRM)
        end
    end
end)
