-- Icecrown Citadel: at_icc_saurfang_portal — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (at_icc_saurfang_portal, :1514-1546). Trigger ID 5698 is the TDB
-- areatrigger_scripts row (25101 + 26091 agree); no in-file ID exists
-- in C++ (only AT_NERUBAR_BROODKEEPER 5611 is in-file,
-- icecrown_citadel.h:541) — the ID is read from the DB dumps, not
-- guessed (the at_zumrah 962 precedent, d8b1a516).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers receive (event, player,
-- triggerId).
-- C++ :1519-1545: if the Deathbringer Saurfang boss state is not DONE
-- nothing happens; else the player teleports to (631, 4126.35,
-- 2769.23, 350.963, 0.0) and, if DATA_COLDFLAME_JETS (26) is
-- NOT_STARTED, the instance stores IN_PROGRESS and every
-- frost-freeze trap (37744) within 120yd gets DoAction(11000/1000
-- alternating, nearest first). The trap receivers are ported in
-- npc_frost_freeze_trap.lua (DoAction 1000/11000 -> 70460 cast,
-- 22s re-arm); this trigger was their only missing sender.
-- Deviations: TeleportNear is the same-map arm (lua_player.go);
-- TeleportNear stages the destination for the client's ack
-- (movement.go:285-311), so the player's server position is still
-- pre-teleport when the sweep runs — the sweep is re-centered on the
-- destination coordinates (bound-creature anchor + 2D filter + 3D
-- sort) instead of the player object. PlayerRelocation (grid preload,
-- :1529-1530) has no bridge; the sweep reads the motion map, so traps
-- materialize on the teleport's grid load (same class as the instance
-- file's LoadGrid note). The boss-state gate rides the first bound
-- creature (the halls_of_reflection worldStateCaller idiom); Go reads
-- TO_BE_DECIDED (5) for an unset boss where C++ reads NOT_STARTED
-- (0) — both != DONE. Stale binds still read the store: despawn only
-- drops the motion from the index (boss_ai.go:221), Map/InstanceID
-- survive on the struct. SetData(26, IN_PROGRESS) fires event-8; the
-- instance file stores it (instanceData:SetData stores nothing itself,
-- instance_hooks.go:39-47) so the gate reads it back.

local TRIGGER_SAURFANG_PORTAL = 5698 -- TDB areatrigger_scripts

local DATA_DEATHBRINGER_SAURFANG = 3 -- icecrown_citadel.h:77
local DATA_COLDFLAME_JETS = 26 -- icecrown_citadel.h:102
local NPC_FROST_FREEZE_TRAP = 37744 -- icecrown_citadel.h:219

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local DEST_X = 4126.35
local DEST_Y = 2769.23
local DEST_Z = 350.963
local DEST_O = 0.0
local TRAP_RANGE = 120.0 -- icecrown_citadel.cpp:1537

-- First bound creature wins (the instance file's worldStateCaller
-- idiom): any creature in the instance reads the shared boss-state
-- store and anchors the range sweep. Keys match the bossBinds table
-- in instance_icecrown_citadel.lua.
local function firstBoundCreature(data)
    return data.lordMarrowgar or data.ladyDeathwhisper
        or data.deathbringerSaurfang or data.festergut or data.rotface
        or data.professorPutricide or data.bloodQueenLanaThel
        or data.sindragosa or data.theLichKing
end

-- C++ :1519-1545 at_icc_saurfang_portal.
local function onSaurfangPortal(player)
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local caller = firstBoundCreature(data)
    if not caller then
        return
    end
    if caller:GetBossState(DATA_DEATHBRINGER_SAURFANG) ~= BOSS_STATE_DONE then
        return
    end
    player:TeleportNear(DEST_X, DEST_Y, DEST_Z, DEST_O)
    if (data.coldflameJetsState or BOSS_STATE_NOT_STARTED) ~= BOSS_STATE_NOT_STARTED then
        return
    end
    data:SetData(DATA_COLDFLAME_JETS, BOSS_STATE_IN_PROGRESS)
    -- C++ :1536-1544: traps within 120yd of the player (post-teleport
    -- position), sorted nearest-first by ObjectDistanceOrderPred, then
    -- DoAction(11000/1000 alternating, nearest first).
    local traps = caller:GetCreaturesInRange(5000, NPC_FROST_FREEZE_TRAP)
    local near = {}
    for _, trap in ipairs(traps) do
        if trap:GetDistance2d(DEST_X, DEST_Y) <= TRAP_RANGE then
            near[#near + 1] = trap
        end
    end
    table.sort(near, function(a, b)
        return a:GetDistance(DEST_X, DEST_Y, DEST_Z) < b:GetDistance(DEST_X, DEST_Y, DEST_Z)
    end)
    local action = 11000
    for _, trap in ipairs(near) do
        trap:DoAction(action)
        if action == 11000 then
            action = 1000
        else
            action = 11000
        end
    end
end

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId == TRIGGER_SAURFANG_PORTAL then
        onSaurfangPortal(player)
    end
end)
