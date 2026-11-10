-- Zul'Farrak instance script — Lua port of the pyramid event machine in
-- src/server/scripts/Kalimdor/ZulFarrak/instance_zulfarrak.cpp
-- (instance_zulfarrak, InstanceMapScript "instance_zulfarrak", map 209)
-- + zulfarrak.h enums (ZFEntries, ZFPyramidPhases, EVENT_PYRAMID = 1).
-- RegisterMapEvent binds the map-level Eluna instance hooks: C++ fires both
-- the map and instance binding maps at every fire site, so the map binding
-- covers every Zul'Farrak instance (engine/scripting/runtime.go:653), and
-- its presence opens the Map:GetInstanceData gate for map 209
-- (engine/world/instance_hooks.go:153-172).
--
-- Shared instanceData keys (one persistent table per (map, instance),
-- engine/world/instance_hooks.go:12-30): pyramidPhase (EVENT_PYRAMID,
-- nil reads as PYRAMID_NOT_STARTED = 0), bly / raven / oro / weegli /
-- murta (the five caged crew creature objects, latched at
-- OnCreatureCreate — the same keys npc_sergeant_bly.lua's
-- switchFactionIfAlive reads), addsAtBase / movedAdds (arrays of
-- wave-add GUID lows), majorMs / minorMs / addGroupSize (the Update
-- wave timers), gahzrillaSeen (the Gahz'rilla first-create latch).
-- Writers change the phase ONLY through instanceData:SetData, which fires
-- the Go-only INSTANCE_EVENT_ON_SET_DATA (8) hook
-- (engine/world/instance_hooks.go:43-63); the OnSetData handler below is
-- the C++ SetData switch (instance_zulfarrak.cpp:186-194, EVENT_PYRAMID
-- only). Readers (Bly/Weegli gossip + MovementInform) read
-- creature:GetMap():GetInstanceData().pyramidPhase.
-- The phase writers are the cage gossip (ported below, entries
-- 141070-141074) and Weegli's MovementInform arrival in
-- npc_weegli_blastfuse.lua.
--
-- Ported arms (C++-exact):
-- - OnCreatureCreate (instance_zulfarrak.cpp:128-164): REACT_PASSIVE (0)
--   on the five caged crew members (Bly 7604, Raven 7605, Oro 7606,
--   Weegli 7607, Murta 7608 — "starts out passive (in a cage)"); GUID
--   latching for Zumrah 7271 + the five crew via creature:SetGuidData
--   (engine/world/lua_creature_events.go:6622-6640, the population leg
--   of the instance guid store — this is the latch Bly's
--   GetInstanceCreature(ENTRY_WEEGLI / RAVEN / ORO / MURTA) reads);
--   crew objects stored in instanceData.crew for the wave machine.
--   Gahz'rilla 7273: first create latches the encounter IN_PROGRESS,
--   a later create despawns it (DisappearAndDie twin: creature:Despawn,
--   the halazzi precedent).
-- - Update wave machine (instance_zulfarrak.cpp:196-272, one dispatch
--   per tick on the phase read at tick entry, like the C++ switch):
--   ARRIVED_AT_STAIR spawns wave 1 (major 120000, minor 0, group 2);
--   WAVE_1 / WAVE_2 drip-feed adds every 10s (SendAddsUpStairs, growing
--   group size) until IsWaveAllDead, then PRE_WAVE_2 (10s) / wave 2,
--   PRE_WAVE_3 (wave 3 spawns at the WAVE_2 -> PRE_WAVE_3 edge, 5s)
--   moves the five crew to the stair bottom, WAVE_3 all-dead sets
--   KILLED_ALL_TROLLS and moves the crew to their final positions.
--   SpawnPyramidWave (instance_zulfarrak.cpp:289-301): the 54-row table
--   below is transcribed mechanically from pyramidSpawns (wave 1: 23,
--   wave 2: 24, wave 3: 7 — sums to pyramidSpawnTotal = 54); each add
--   summons at (x, y, 8.87, 0) with the default MANUAL_DESPAWN type
--   (the C++ InstanceScript::SummonCreature default) and MoveRandom(10).
--   IsWaveAllDead (instance_zulfarrak.cpp:303-321): a GUID counts dead
--   when it no longer resolves or its fresh object is not alive.
--   MoveNPCIfAlive (instance_zulfarrak.cpp:274-287): SetWalk(true) +
--   MoveTo(1, x, y, z) (the MovePoint twin, lua_creature_events.go:4974)
--   + SetHomePosition(x, y, z, o), via GetInstanceCreature.
-- - go_troll_cage OnGossipHello (zulfarrak.cpp:202-244): all five cage
--   entries 141070-141074 (gameobject_template ScriptName, AzerothCore
--   acore_world; spawned map 209 at 1881-1891/1294-1299) share one
--   handler — SetData(EVENT_PYRAMID, PYRAMID_CAGES_OPEN) first, then
--   initBlyCrewMember x5 (SetReactState AGGRESSIVE 2 + SetWalk +
--   SetHomePosition(x, y, z, 0) + MoveTo(1, x, y, z) + SetFaction(250)),
--   null-safe via GetInstanceCreature, no IsAlive gate (C++ has none).
--   Returns false like C++ so the native GO use arms proceed
--   (gameobject_hooks.go:144-202 polarity).
-- - STALE-OBJECT RULE: a creature object built by luaMotionCreature is
--   a construction-time snapshot (engine/world/lua_creature_events.go:
--   6853-6866, Fields hold spawn-time Health), so a stored crew object
--   must never be read for aliveness — it is an ACTION target only
--   (SetWalk/MoveTo/SetHomePosition/DoAction read the live motion
--   through the method closure). Every aliveness read re-resolves a
--   fresh object: GetInstanceCreature(entry) for the crew,
--   GetCreatureByGUID(low) for the wave adds
--   (lua_creature_events.go:4029-4066, nil when not live — the C++
--   GetCreature-null leg, which counts as dead).
--
-- Stay-outs (documented-only):
-- - go_troll_cage: PORTED (see above) — entries sourced 2026-11-10 from
--   AzerothCore acore_world gameobject_template.sql (ScriptName rows
--   141070-141074) + gameobject.sql spawns (map 209, pyramid cage row);
--   the old ENTRY-BLOCKED note is superseded, as are the matching notes
--   in npc_sergeant_bly.lua / npc_weegli_blastfuse.lua.
-- - SendAddsUpStairs MovePath (instance_zulfarrak.cpp:323-335):
--   add->GetMotionMaster()->MovePath(PATH_ADDS 81553, false) has NO
--   bridge — only startCreatureWaypoint (the creature's own DB path,
--   lua_creature_events.go:4352). The Lua leg keeps the C++ list
--   bookkeeping (pop from addsAtBase, push to movedAdds) so
--   IsWaveAllDead stays exact, but the adds do not walk up the stairs;
--   they hold at their spawn points until killed. No MoveTo substitute
--   is used: a stair-top teleport would fabricate movement C++ never
--   does (the 81553 path waypoints are DB-side).
-- - Spawnsway[2][3] (instance_zulfarrak.cpp:76-80): defined but never
--   read anywhere in the C++ file — dead table, nothing to port.
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (the black-morass delta).
-- - go_shallow_grave (zulfarrak.cpp:392-434): GO entry likewise
--   DB-side; stays out (boss_ai.go residual audit).
-- - at_zumrah (zulfarrak.cpp:436-460): AreaTriggerScript,
--   FindNearestCreature 7271 30y + SetFaction(FACTION_TROLL_FROSTMANE)
--   — the area-trigger dispatch (RegisterServerEvent 24) exists now
--   (broodkeeper precedent) but the trigger id is DB-side, nowhere in
--   the C++ tree; stays out until the id is sourced.

local MAP_ZULFARRAK = 209
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_SET_DATA = 8

local EVENT_PYRAMID = 1

-- zulfarrak.h ZFPyramidPhases.
local PYRAMID_NOT_STARTED = 0
local PYRAMID_CAGES_OPEN = 1
local PYRAMID_ARRIVED_AT_STAIR = 2
local PYRAMID_WAVE_1 = 3
local PYRAMID_PRE_WAVE_2 = 4
local PYRAMID_WAVE_2 = 5
local PYRAMID_PRE_WAVE_3 = 6
local PYRAMID_WAVE_3 = 7
local PYRAMID_KILLED_ALL_TROLLS = 8

-- zulfarrak.h ZFEntries + instance_zulfarrak.cpp Misc.
local ENTRY_ZUM_RAH = 7271
local ENTRY_BLY = 7604
local ENTRY_RAVEN = 7605
local ENTRY_ORO = 7606
local ENTRY_WEEGLI = 7607
local ENTRY_MURTA = 7608
local NPC_GAHZRILLA = 7273

local REACT_PASSIVE = 0

local CAGED_CREW = {
    [7604] = true, -- ENTRY_BLY
    [7605] = true, -- ENTRY_RAVEN
    [7606] = true, -- ENTRY_ORO
    [7607] = true, -- ENTRY_WEEGLI
    [7608] = true, -- ENTRY_MURTA
}

-- String keys for the crew objects (the npc_sergeant_bly.lua contract:
-- its switchFactionIfAlive reads data.raven / data.oro / data.murta).
local CREW_KEYS = {
    [7604] = "bly",
    [7605] = "raven",
    [7606] = "oro",
    [7607] = "weegli",
    [7608] = "murta",
}

-- pyramidSpawns (instance_zulfarrak.cpp:88-141): {wave, entry, x, y};
-- Z is 8.87 for every row ("no z coordinat b/c they're all the same").
-- Transcribed mechanically from the C++ table (54 rows: wave 1: 23,
-- wave 2: 24, wave 3: 7).
local PyramidSpawns = {
    {1, 7789, 1894.64, 1206.29},
    {1, 7787, 1890.08, 1218.68},
    {1, 8876, 1883.76, 1222.3},
    {1, 7789, 1874.18, 1221.24},
    {1, 7787, 1892.28, 1225.49},
    {1, 7788, 1889.94, 1212.21},
    {1, 7787, 1879.02, 1223.06},
    {1, 7789, 1874.45, 1204.44},
    {1, 8876, 1898.23, 1217.97},
    {1, 7787, 1882.07, 1225.7},
    {1, 8877, 1896.46, 1205.62},
    {1, 7787, 1886.97, 1225.86},
    {1, 7787, 1894.72, 1221.91},
    {1, 7787, 1883.5, 1218.25},
    {1, 7787, 1886.93, 1221.4},
    {1, 8876, 1889.82, 1222.51},
    {1, 7788, 1893.07, 1215.26},
    {1, 7788, 1878.57, 1214.16},
    {1, 7788, 1883.74, 1212.35},
    {1, 8877, 1877, 1207.27},
    {1, 8877, 1873.63, 1204.65},
    {1, 8876, 1877.4, 1216.41},
    {1, 8877, 1899.63, 1202.52},
    {2, 7789, 1902.83, 1223.41},
    {2, 8876, 1889.82, 1222.51},
    {2, 7787, 1883.5, 1218.25},
    {2, 7788, 1883.74, 1212.35},
    {2, 8877, 1877, 1207.27},
    {2, 7787, 1890.08, 1218.68},
    {2, 7789, 1894.64, 1206.29},
    {2, 8876, 1877.4, 1216.41},
    {2, 7787, 1892.28, 1225.49},
    {2, 7788, 1893.07, 1215.26},
    {2, 8877, 1896.46, 1205.62},
    {2, 7789, 1874.45, 1204.44},
    {2, 7789, 1874.18, 1221.24},
    {2, 7787, 1879.02, 1223.06},
    {2, 8876, 1898.23, 1217.97},
    {2, 7787, 1882.07, 1225.7},
    {2, 8877, 1873.63, 1204.65},
    {2, 7787, 1886.97, 1225.86},
    {2, 7788, 1878.57, 1214.16},
    {2, 7787, 1894.72, 1221.91},
    {2, 7787, 1886.93, 1221.4},
    {2, 8876, 1883.76, 1222.3},
    {2, 7788, 1889.94, 1212.21},
    {2, 8877, 1899.63, 1202.52},
    {3, 7788, 1878.57, 1214.16},
    {3, 7787, 1894.72, 1221.91},
    {3, 7787, 1886.93, 1221.4},
    {3, 8876, 1883.76, 1222.3},
    {3, 7788, 1889.94, 1212.21},
    {3, 7275, 1889.23, 1207.72},
    {3, 7796, 1879.77, 1207.96},
}

-- First stored crew creature, the summon / lookup handle for the wave
-- machine. The stored object is an ACTION target only (stale-object
-- rule above); every read re-resolves through it.
local function resolverFor(instanceData)
    for _, entry in ipairs({ENTRY_BLY, ENTRY_RAVEN, ENTRY_ORO, ENTRY_WEEGLI, ENTRY_MURTA}) do
        local crew = instanceData[CREW_KEYS[entry]]
        if crew then
            return crew
        end
    end
    return nil
end

-- C++ SpawnPyramidWave (instance_zulfarrak.cpp:289-301).
local function spawnPyramidWave(instanceData, resolver, wave)
    local base = instanceData.addsAtBase
    if not base then
        base = {}
        instanceData.addsAtBase = base
    end
    for _, row in ipairs(PyramidSpawns) do
        if row[1] == wave then
            local add = resolver:SummonCreature(row[2], row[3], row[4], 8.87, 0, 7, 0)
            if add then
                add:MoveRandom(10)
                base[#base + 1] = add:GetGUIDLow()
            end
        end
    end
end

-- C++ IsWaveAllDead (instance_zulfarrak.cpp:303-321): a GUID counts
-- dead when it no longer resolves (C++ GetCreature null leg) or its
-- fresh object is not alive.
local function isWaveAllDead(instanceData, resolver)
    for _, guidLow in ipairs(instanceData.addsAtBase or {}) do
        local add = resolver:GetCreatureByGUID(guidLow)
        if add and add:IsAlive() then
            return false
        end
    end
    for _, guidLow in ipairs(instanceData.movedAdds or {}) do
        local add = resolver:GetCreatureByGUID(guidLow)
        if add and add:IsAlive() then
            return false
        end
    end
    return true
end

-- C++ MoveNPCIfAlive (instance_zulfarrak.cpp:274-287).
local function moveNPCIfAlive(resolver, entry, x, y, z, o)
    local npc = resolver:GetInstanceCreature(entry)
    if npc and npc:IsAlive() then
        npc:SetWalk(true)
        npc:MoveTo(1, x, y, z)
        npc:SetHomePosition(x, y, z, o)
    end
end

-- C++ SendAddsUpStairs (instance_zulfarrak.cpp:323-335) minus the
-- MovePath leg: the list bookkeeping is kept so IsWaveAllDead stays
-- exact, but the up-stairs walk itself has no bridge (see header).
local function sendAddsUpStairs(instanceData, count)
    local base = instanceData.addsAtBase or {}
    local moved = instanceData.movedAdds
    if not moved then
        moved = {}
        instanceData.movedAdds = moved
    end
    for _ = 1, count do
        if #base == 0 then
            break
        end
        moved[#moved + 1] = base[1]
        table.remove(base, 1)
    end
end

local REACT_AGGRESSIVE = 2
local FACTION_ESCORTEE_N_NEUTRAL_ACTIVE = 250

-- Troll Cage GO entries (gameobject_template ScriptName "go_troll_cage",
-- AzerothCore acore_world; spawned map 209 at 1881-1891/1294-1299).
local GO_TROLL_CAGE = {141070, 141071, 141072, 141073, 141074}

-- C++ initBlyCrewMember positions (zulfarrak.cpp:227-232): {entry, x, y, z}.
local CAGE_CREW = {
    {ENTRY_BLY, 1884.99, 1263, 41.52},
    {ENTRY_RAVEN, 1882.5, 1263, 41.52},
    {ENTRY_ORO, 1886.47, 1270.68, 41.68},
    {ENTRY_WEEGLI, 1890, 1263, 41.52},
    {ENTRY_MURTA, 1891.19, 1272.03, 41.60},
}

-- C++ go_troll_cage::OnGossipHello (zulfarrak.cpp:212-224): SetData first,
-- then arouse each crew member (null-safe via GetInstanceCreature; no
-- IsAlive gate in C++). Returns false like C++ so the native GO use arms
-- proceed (gameobject_hooks.go:144-202 polarity).
local function onTrollCageGossip(_, player, go)
    local mapObj = player:GetMap()
    local instanceData = mapObj and mapObj:GetInstanceData()
    if not instanceData then
        return false
    end
    instanceData:SetData(EVENT_PYRAMID, PYRAMID_CAGES_OPEN)
    local resolver = resolverFor(instanceData)
    if not resolver then
        return false
    end
    for _, row in ipairs(CAGE_CREW) do
        local crew = resolver:GetInstanceCreature(row[1])
        if crew then
            crew:SetReactState(REACT_AGGRESSIVE)
            crew:SetWalk(true)
            crew:SetHomePosition(row[2], row[3], row[4], 0)
            crew:MoveTo(1, row[2], row[3], row[4])
            crew:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_ACTIVE)
        end
    end
    return false
end

for _, entry in ipairs(GO_TROLL_CAGE) do
    RegisterGameObjectGossipEvent(entry, 1, onTrollCageGossip)
end

RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_GAHZRILLA then
        -- C++ OnCreatureCreate (instance_zulfarrak.cpp:159-164): first
        -- create latches the encounter IN_PROGRESS; a later create
        -- despawns the hydra. No GUID latch in C++ (no GetGuidData
        -- reader for 7273), so none here either.
        if instanceData.gahzrillaSeen then
            creature:Despawn()
        else
            instanceData.gahzrillaSeen = true
        end
        return
    end
    if entry == ENTRY_ZUM_RAH or CAGED_CREW[entry] then
        creature:SetGuidData(entry)
    end
    if CAGED_CREW[entry] then
        creature:SetReactState(REACT_PASSIVE)
    end
    local key = CREW_KEYS[entry]
    if key then
        instanceData[key] = creature
    end
end)

-- C++ SetData switch (instance_zulfarrak.cpp:186-194): EVENT_PYRAMID
-- only; every other type falls through (Zum'rah's DATA_ZUM_RAH = 0
-- write is a dead write in C++ itself — see boss_zum_rah.lua).
RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type == EVENT_PYRAMID then
        instanceData.pyramidPhase = data
    end
end)

-- C++ Update (instance_zulfarrak.cpp:196-272). One dispatch per tick
-- on the phase read at tick entry, like the C++ switch: SetData calls
-- inside the tick update the stored phase for the NEXT tick only.
-- Timer idiom is C++-exact: `if (timer < diff) { fire; timer = X }
-- else timer -= diff` (strict <, no subtract on fire).
RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    local phase = instanceData.pyramidPhase or PYRAMID_NOT_STARTED
    if phase == PYRAMID_NOT_STARTED or phase == PYRAMID_KILLED_ALL_TROLLS then
        return
    end
    -- CAGES_OPEN has no Update case in C++ (the crew is still walking
    -- to the stair; Weegli's arrival flips the phase).
    local resolver = resolverFor(instanceData)
    if not resolver then
        return
    end
    if phase == PYRAMID_ARRIVED_AT_STAIR then
        spawnPyramidWave(instanceData, resolver, 1)
        instanceData:SetData(EVENT_PYRAMID, PYRAMID_WAVE_1)
        instanceData.majorMs = 120000
        instanceData.minorMs = 0
        instanceData.addGroupSize = 2
    elseif phase == PYRAMID_WAVE_1 then
        if isWaveAllDead(instanceData, resolver) then
            instanceData:SetData(EVENT_PYRAMID, PYRAMID_PRE_WAVE_2)
            instanceData.majorMs = 10000 -- rebuff breather before wave 2
        else
            local minor = instanceData.minorMs or 0
            if minor < diff then
                local size = instanceData.addGroupSize or 2
                sendAddsUpStairs(instanceData, size)
                instanceData.addGroupSize = size + 1
                instanceData.minorMs = 10000
            else
                instanceData.minorMs = minor - diff
            end
        end
    elseif phase == PYRAMID_PRE_WAVE_2 then
        local major = instanceData.majorMs or 0
        if major < diff then
            -- beginning 2nd wave!
            spawnPyramidWave(instanceData, resolver, 2)
            instanceData:SetData(EVENT_PYRAMID, PYRAMID_WAVE_2)
            instanceData.minorMs = 0
            instanceData.addGroupSize = 2
        else
            instanceData.majorMs = major - diff
        end
    elseif phase == PYRAMID_WAVE_2 then
        if isWaveAllDead(instanceData, resolver) then
            spawnPyramidWave(instanceData, resolver, 3)
            instanceData:SetData(EVENT_PYRAMID, PYRAMID_PRE_WAVE_3)
            instanceData.majorMs = 5000 -- NPCs return to their home spots
        else
            local minor = instanceData.minorMs or 0
            if minor < diff then
                local size = instanceData.addGroupSize or 2
                sendAddsUpStairs(instanceData, size)
                instanceData.addGroupSize = size + 1
                instanceData.minorMs = 10000
            else
                instanceData.minorMs = minor - diff
            end
        end
    elseif phase == PYRAMID_PRE_WAVE_3 then
        local major = instanceData.majorMs or 0
        if major < diff then
            -- move NPCs to bottom of stair (C++ order: Bly, Murta,
            -- Oro, Raven, Weegli).
            moveNPCIfAlive(resolver, ENTRY_BLY, 1887.92, 1228.179, 9.98, 4.78)
            moveNPCIfAlive(resolver, ENTRY_MURTA, 1891.57, 1228.68, 9.69, 4.78)
            moveNPCIfAlive(resolver, ENTRY_ORO, 1897.23, 1228.34, 9.43, 4.78)
            moveNPCIfAlive(resolver, ENTRY_RAVEN, 1883.68, 1227.95, 9.543, 4.78)
            moveNPCIfAlive(resolver, ENTRY_WEEGLI, 1878.02, 1227.65, 9.485, 4.78)
            instanceData:SetData(EVENT_PYRAMID, PYRAMID_WAVE_3)
        else
            instanceData.majorMs = major - diff
        end
    elseif phase == PYRAMID_WAVE_3 then
        if isWaveAllDead(instanceData, resolver) then
            -- move NPCs to their final positions (C++ order: Bly,
            -- Murta, Oro, Raven, Weegli).
            instanceData:SetData(EVENT_PYRAMID, PYRAMID_KILLED_ALL_TROLLS)
            moveNPCIfAlive(resolver, ENTRY_BLY, 1883.82, 1200.83, 8.87, 1.32)
            moveNPCIfAlive(resolver, ENTRY_MURTA, 1891.83, 1201.45, 8.87, 1.32)
            moveNPCIfAlive(resolver, ENTRY_ORO, 1894.50, 1204.40, 8.87, 1.32)
            moveNPCIfAlive(resolver, ENTRY_RAVEN, 1874.11, 1206.17, 8.87, 1.32)
            moveNPCIfAlive(resolver, ENTRY_WEEGLI, 1877.52, 1199.63, 8.87, 1.32)
        end
    end
end)
