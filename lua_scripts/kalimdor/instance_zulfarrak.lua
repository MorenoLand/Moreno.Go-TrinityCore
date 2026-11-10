-- Zul'Farrak instance script — Lua port of the pyramid event machine in
-- src/server/scripts/Kalimdor/ZulFarrak/instance_zulfarrak.cpp
-- (instance_zulfarrak, InstanceMapScript "instance_zulfarrak", map 209)
-- plus the instance-facing legs of zulfarrak.cpp (go_troll_cage,
-- npc_weegli_blastfuse MovementInform, npc_sergeant_bly gossip machine).
-- RegisterMapEvent binds the map-level Eluna instance hook: C++ fires both
-- the map and instance binding maps at every fire site, so the map binding
-- covers every Zul'Farrak instance (engine/scripting/runtime.go:653).
-- INSTANCE_EVENT_ON_CREATURE_CREATE (5) fires from the motion funnel on
-- materialization (engine/world/creaturemotion.go:736), the Go twin of
-- Creature::AddToWorld (Creature.cpp:301) that C++ OnCreatureCreate rides.
-- INSTANCE_EVENT_ON_SET_DATA (8) fires from instanceData:SetData(type,
-- data) (engine/world/instance_hooks.go:51; nested dispatch is safe, the
-- runtime lock is re-entrant). INSTANCE_EVENT_ON_UPDATE (3) fires every
-- tick with diff in ms (engine/world/instance_hooks.go:637-640).
-- Shared instanceData keys (one table per (map, instance), the
-- instance_mount_hyjal.lua / instance_icecrown_citadel.lua persistent-table
-- pattern — the old "no per-instance Lua data store" header claim was
-- STALE): bly / raven / oro / weegli / murta — the crew creature objects
-- bound by ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches,
-- instance_zulfarrak.cpp:127-164); pyramidPhase — the EVENT_PYRAMID phase,
-- written by the ON_SET_DATA receiver below; majorWaveTimer /
-- minorWaveTimer / addGroupSize — the Update timers; addsAtBase /
-- movedAdds — the summoned wave adds (C++ GuidLists, here creature
-- objects, the broodkeeperGroups precedent).
--
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: REACT_PASSIVE (0) on the five caged Bly crew
--   members — Bly 7604, Raven 7605, Oro 7606, Weegli 7607, Murta 7608
--   (zulfarrak.h ZFEntries; instance_zulfarrak.cpp:139-164, "starts out
--   passive (in a cage)"). creature:SetReactState writes motion.ReactState
--   (engine/world/lua_creature_events.go:1934) and the engine honors it in
--   isCreaturePassive, so the arm has real effect, not just a latch.
-- - SetData EVENT_PYRAMID (1): stores the phase (instance_zulfarrak.cpp:
--   187-194); the GetData twin is the pyramidPhase field, read by the Bly
--   / Weegli gossip arms via creature:GetMap():GetInstanceData().
-- - Update pyramid machine (instance_zulfarrak.cpp:196-293): ARRIVED_AT_
--   STAIR spawns wave 1 + 120s major / 0 minor / addGroupSize 2; WAVE_1 /
--   WAVE_2 advance on IsWaveAllDead (PRE_WAVE_2 10s, wave 2 spawn,
--   PRE_WAVE_3 5s) with the minor 10s SendAddsUpStairs(addGroupSize++)
--   trickle; PRE_WAVE_3 herds the five crew to the stair bottom; WAVE_3
--   completion herds them to their final positions and sets KILLED_ALL_
--   TROLLS. Timer semantics are the C++ ones verbatim (fire when timer <
--   diff, else subtract diff; the WAVE_1 all-dead arm leaves minorWaveTimer
--   stale, as C++ does).
-- - SpawnPyramidWave (instance_zulfarrak.cpp:296-309): the 54-row table
--   below, z 8.87 fixed, MoveRandom(10) per add, adds appended to
--   addsAtBase. Summoner is the first bound crew member (C++
--   instance->SummonCreature has no summoner; no map-level summon bridge
--   exists, the culling-of-stratholme precedent — a bound creature is the
--   summoner). Summon type 7 = MANUAL_DESPAWN, the C++ default.
-- - MoveNPCIfAlive (instance_zulfarrak.cpp:286-295): SetWalk(true) +
--   MoveTo(1, x, y, z) + SetHomePosition(x, y, z, o), alive-gated.
-- - IsWaveAllDead (instance_zulfarrak.cpp:311-331): no live add in either
--   list; a summoned object whose motion is gone reads dead (its last
--   state), the C++ GetCreature-nil twin.
--
-- Stay-outs / deltas:
-- - SendAddsUpStairs MovePath(PATH_ADDS 81553) has no bridge (only
--   self-path startCreatureWaypoint exists): the trickle calls are no-ops,
--   the adds stay at the pyramid base and movedAdds stays empty — wave
--   progression is unaffected (IsWaveAllDead checks both lists), but the
--   adds never walk up the stairs to the players; the fight happens at the
--   base instead of the stair top.
-- - Gahz'rilla 7273 OnCreatureCreate leg (DisappearAndDie / encounter
--   latch): no instance encounter-state bridge.
-- - go_troll_cage (zulfarrak.cpp:202-244) is ENTRY-BLOCKED, not ported:
--   the cage GO entry appears nowhere in the C++ tree (no constant in
--   zulfarrak.h, no gameobject_template in the workspace SQL — only
--   custom tables — and web searches give no numeric entry), so
--   RegisterGameObjectGossipEvent has nothing to bind. Its function —
--   SetData(EVENT_PYRAMID, PYRAMID_CAGES_OPEN) + herding the five crew to
--   the stair top (initBlyCrewMember: REACT_AGGRESSIVE + SetWalk +
--   SetHomePosition + MovePoint + SetFaction(250)) — is the machine's only
--   starter, so until the entry is sourced the wave machine can never
--   start in-game; every leg below it is ported and waiting. The herd
--   coords are preserved in the header of npc_weegli_blastfuse.lua's
--   MovementInform note for the porting tick (Bly 1884.99/1263/41.52,
--   Raven 1882.5/1263/41.52, Oro 1886.47/1270.68/41.68, Weegli 1890/1263/
--   41.52, Murta 1891.19/1272.03/41.60, all facing 0).
-- - at_zumrah (zulfarrak.cpp:402-425): AreaTriggerScript, FindNearest-
--   Creature 7271 30y + SetFaction(FACTION_TROLL_FROSTMANE) — the area-
--   trigger dispatch (RegisterServerEvent 24) exists now (broodkeeper
--   precedent) but the trigger id is DB-side, nowhere in the C++ tree;
--   stays out until the id is sourced.
-- - go_shallow_grave (zulfarrak.cpp:336-379): GO entry likewise DB-side;
--   stays out.

local MAP_ZULFARRAK = 209
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_SET_DATA = 8

local EVENT_PYRAMID = 1

-- ZFPyramidPhases (zulfarrak.h).
local PYRAMID_NOT_STARTED = 0
local PYRAMID_CAGES_OPEN = 1
local PYRAMID_ARRIVED_AT_STAIR = 2
local PYRAMID_WAVE_1 = 3
local PYRAMID_PRE_WAVE_2 = 4
local PYRAMID_WAVE_2 = 5
local PYRAMID_PRE_WAVE_3 = 6
local PYRAMID_WAVE_3 = 7
local PYRAMID_KILLED_ALL_TROLLS = 8

local ENTRY_BLY = 7604
local ENTRY_RAVEN = 7605
local ENTRY_ORO = 7606
local ENTRY_WEEGLI = 7607
local ENTRY_MURTA = 7608

local REACT_PASSIVE = 0

-- Wave spawns (instance_zulfarrak.cpp:35-88): {wave, entry, x, y}, z 8.87.
local PYRAMID_SPAWNS = {
    {1, 7789, 1894.64, 1206.29},
    {1, 7787, 1890.08, 1218.68},
    {1, 8876, 1883.76, 1222.30},
    {1, 7789, 1874.18, 1221.24},
    {1, 7787, 1892.28, 1225.49},
    {1, 7788, 1889.94, 1212.21},
    {1, 7787, 1879.02, 1223.06},
    {1, 7789, 1874.45, 1204.44},
    {1, 8876, 1898.23, 1217.97},
    {1, 7787, 1882.07, 1225.70},
    {1, 8877, 1896.46, 1205.62},
    {1, 7787, 1886.97, 1225.86},
    {1, 7787, 1894.72, 1221.91},
    {1, 7787, 1883.50, 1218.25},
    {1, 7787, 1886.93, 1221.40},
    {1, 8876, 1889.82, 1222.51},
    {1, 7788, 1893.07, 1215.26},
    {1, 7788, 1878.57, 1214.16},
    {1, 7788, 1883.74, 1212.35},
    {1, 8877, 1877.00, 1207.27},
    {1, 8877, 1873.63, 1204.65},
    {1, 8876, 1877.40, 1216.41},
    {1, 8877, 1899.63, 1202.52},
    {2, 7789, 1902.83, 1223.41},
    {2, 8876, 1889.82, 1222.51},
    {2, 7787, 1883.50, 1218.25},
    {2, 7788, 1883.74, 1212.35},
    {2, 8877, 1877.00, 1207.27},
    {2, 7787, 1890.08, 1218.68},
    {2, 7789, 1894.64, 1206.29},
    {2, 8876, 1877.40, 1216.41},
    {2, 7787, 1892.28, 1225.49},
    {2, 7788, 1893.07, 1215.26},
    {2, 8877, 1896.46, 1205.62},
    {2, 7789, 1874.45, 1204.44},
    {2, 7789, 1874.18, 1221.24},
    {2, 7787, 1879.02, 1223.06},
    {2, 8876, 1898.23, 1217.97},
    {2, 7787, 1882.07, 1225.70},
    {2, 8877, 1873.63, 1204.65},
    {2, 7787, 1886.97, 1225.86},
    {2, 7788, 1878.57, 1214.16},
    {2, 7787, 1894.72, 1221.91},
    {2, 7787, 1886.93, 1221.40},
    {2, 8876, 1883.76, 1222.30},
    {2, 7788, 1889.94, 1212.21},
    {2, 8877, 1899.63, 1202.52},
    {3, 7788, 1878.57, 1214.16},
    {3, 7787, 1894.72, 1221.91},
    {3, 7787, 1886.93, 1221.40},
    {3, 8876, 1883.76, 1222.30},
    {3, 7788, 1889.94, 1212.21},
    {3, 7275, 1889.23, 1207.72},
    {3, 7796, 1879.77, 1207.96},
}

local CREW_KEYS = {
    [ENTRY_BLY] = "bly",
    [ENTRY_RAVEN] = "raven",
    [ENTRY_ORO] = "oro",
    [ENTRY_WEEGLI] = "weegli",
    [ENTRY_MURTA] = "murta",
}

local CAGED_CREW = {
    [ENTRY_BLY] = true,
    [ENTRY_RAVEN] = true,
    [ENTRY_ORO] = true,
    [ENTRY_WEEGLI] = true,
    [ENTRY_MURTA] = true,
}

local STAIR_BOTTOM = {
    [ENTRY_BLY] = {1887.92, 1228.179, 9.98, 4.78},
    [ENTRY_MURTA] = {1891.57, 1228.68, 9.69, 4.78},
    [ENTRY_ORO] = {1897.23, 1228.34, 9.43, 4.78},
    [ENTRY_RAVEN] = {1883.68, 1227.95, 9.543, 4.78},
    [ENTRY_WEEGLI] = {1878.02, 1227.65, 9.485, 4.78},
}

local FINAL_POSITIONS = {
    [ENTRY_BLY] = {1883.82, 1200.83, 8.87, 1.32},
    [ENTRY_MURTA] = {1891.83, 1201.45, 8.87, 1.32},
    [ENTRY_ORO] = {1894.50, 1204.40, 8.87, 1.32},
    [ENTRY_RAVEN] = {1874.11, 1206.17, 8.87, 1.32},
    [ENTRY_WEEGLI] = {1877.52, 1199.63, 8.87, 1.32},
}

local CREW_ORDER = {ENTRY_BLY, ENTRY_RAVEN, ENTRY_ORO, ENTRY_WEEGLI, ENTRY_MURTA}

local function getPhase(instanceData)
    return instanceData.pyramidPhase or PYRAMID_NOT_STARTED
end

local function setPhase(instanceData, phase)
    instanceData:SetData(EVENT_PYRAMID, phase)
end

-- C++ SpawnPyramidWave (instance_zulfarrak.cpp:296-309): summon every
-- table row of the wave at z 8.87, MoveRandom(10), append to addsAtBase.
local function spawnPyramidWave(instanceData, wave)
    local summoner = nil
    for _, entry in ipairs(CREW_ORDER) do
        local crew = instanceData[CREW_KEYS[entry]]
        if crew then
            summoner = crew
            break
        end
    end
    if not summoner then
        return
    end
    if not instanceData.addsAtBase then
        instanceData.addsAtBase = {}
    end
    for _, row in ipairs(PYRAMID_SPAWNS) do
        if row[1] == wave then
            local add = summoner:SummonCreature(row[2], row[3], row[4], 8.87, 0, 7, 0)
            if add then
                add:MoveRandom(10)
                instanceData.addsAtBase[#instanceData.addsAtBase + 1] = add
            end
        end
    end
end

-- C++ IsWaveAllDead (instance_zulfarrak.cpp:311-331).
local function isWaveAllDead(instanceData)
    for _, list in ipairs({instanceData.addsAtBase, instanceData.movedAdds}) do
        if list then
            for _, add in ipairs(list) do
                if add:IsAlive() then
                    return false
                end
            end
        end
    end
    return true
end

-- C++ SendAddsUpStairs (instance_zulfarrak.cpp:333-345): MovePath(PATH_ADDS
-- 81553) has no bridge — the trickle is a no-op (see header); the count
-- argument is consumed by the caller exactly as C++ does.
local function sendAddsUpStairs(instanceData, count)
end

-- C++ MoveNPCIfAlive (instance_zulfarrak.cpp:286-295).
local function moveNPCIfAlive(instanceData, entry, pos)
    local crew = instanceData[CREW_KEYS[entry]]
    if crew and crew:IsAlive() then
        crew:SetWalk(true)
        crew:MoveTo(1, pos[1], pos[2], pos[3])
        crew:SetHomePosition(pos[1], pos[2], pos[3], pos[4])
    end
end

local function onCreatureCreate(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    local key = CREW_KEYS[entry]
    if key then
        instanceData[key] = creature
    end
    if CAGED_CREW[entry] then
        creature:SetReactState(REACT_PASSIVE) -- REACT_PASSIVE
    end
end

-- C++ SetData EVENT_PYRAMID (instance_zulfarrak.cpp:187-194): the switch
-- stores the phase; persistence is this handler's job (instance_hooks.go).
local function onSetData(event, instanceData, mapObj, type, data)
    if type == EVENT_PYRAMID then
        instanceData.pyramidPhase = data
    end
end

-- C++ Update (instance_zulfarrak.cpp:196-284), timer semantics verbatim.
local function onUpdate(event, instanceData, mapObj, diff)
    local phase = getPhase(instanceData)
    if phase == PYRAMID_NOT_STARTED or phase == PYRAMID_KILLED_ALL_TROLLS then
        return
    elseif phase == PYRAMID_ARRIVED_AT_STAIR then
        spawnPyramidWave(instanceData, 1)
        setPhase(instanceData, PYRAMID_WAVE_1)
        instanceData.majorWaveTimer = 120000
        instanceData.minorWaveTimer = 0
        instanceData.addGroupSize = 2
    elseif phase == PYRAMID_WAVE_1 then
        if isWaveAllDead(instanceData) then
            setPhase(instanceData, PYRAMID_PRE_WAVE_2)
            instanceData.majorWaveTimer = 10000
        else
            local minor = instanceData.minorWaveTimer or 0
            if minor < diff then
                local count = instanceData.addGroupSize or 2
                sendAddsUpStairs(instanceData, count)
                instanceData.addGroupSize = count + 1
                instanceData.minorWaveTimer = 10000
            else
                instanceData.minorWaveTimer = minor - diff
            end
        end
    elseif phase == PYRAMID_PRE_WAVE_2 then
        local major = instanceData.majorWaveTimer or 0
        if major < diff then
            spawnPyramidWave(instanceData, 2)
            setPhase(instanceData, PYRAMID_WAVE_2)
            instanceData.minorWaveTimer = 0
            instanceData.addGroupSize = 2
        else
            instanceData.majorWaveTimer = major - diff
        end
    elseif phase == PYRAMID_WAVE_2 then
        if isWaveAllDead(instanceData) then
            spawnPyramidWave(instanceData, 3)
            setPhase(instanceData, PYRAMID_PRE_WAVE_3)
            instanceData.majorWaveTimer = 5000
        else
            local minor = instanceData.minorWaveTimer or 0
            if minor < diff then
                local count = instanceData.addGroupSize or 2
                sendAddsUpStairs(instanceData, count)
                instanceData.addGroupSize = count + 1
                instanceData.minorWaveTimer = 10000
            else
                instanceData.minorWaveTimer = minor - diff
            end
        end
    elseif phase == PYRAMID_PRE_WAVE_3 then
        local major = instanceData.majorWaveTimer or 0
        if major < diff then
            for _, entry in ipairs(CREW_ORDER) do
                moveNPCIfAlive(instanceData, entry, STAIR_BOTTOM[entry])
            end
            setPhase(instanceData, PYRAMID_WAVE_3)
        else
            instanceData.majorWaveTimer = major - diff
        end
    elseif phase == PYRAMID_WAVE_3 then
        if isWaveAllDead(instanceData) then
            setPhase(instanceData, PYRAMID_KILLED_ALL_TROLLS)
            for _, entry in ipairs(CREW_ORDER) do
                moveNPCIfAlive(instanceData, entry, FINAL_POSITIONS[entry])
            end
        end
    end
end

RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_CREATURE_CREATE, onCreatureCreate)
RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_SET_DATA, onSetData)
RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_UPDATE, onUpdate)
