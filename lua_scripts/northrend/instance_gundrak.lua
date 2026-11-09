-- Gundrak instance script — Lua port of the OnUnitDeath leg of
-- src/server/scripts/Northrend/Gundrak/instance_gundrak.cpp
-- (instance_gundrak, InstanceMapScript "instance_gundrak", map 604).
-- RegisterMapEvent binds the map-level Eluna instance hook
-- (instance_ulduar.lua precedent): C++ fires both binding maps at
-- every fire site (engine/scripting/runtime.go:161).
--
-- PORTED — OnUnitDeath ruin-dweller -> Eck summon
-- (instance_gundrak.cpp:187-196): OnCreatureCreate latches every
-- alive ruin dweller (NPC_RUIN_DWELLER 29920) into the instanceData
-- set (the C++ DwellerGUIDs member); OnUnitDeath removes the dead
-- dweller, and when the set empties the dying unit summons Eck the
-- Ferocious (NPC_ECK_THE_FEROCIOUS 29932) at EckSpawnPoint
-- (1643.877930, 936.278015, 107.204948, 0.668432) as
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN, 300s (summonType 5, ms timer —
-- lua_creature_events.go:3694). No heroic / Moorabi gate in C++
-- (access is the separate CheckRequiredBosses gate). The set lives in
-- the persistent per-(map, instance) table
-- (engine/world/instance_hooks.go:24); it is runtime-only in
-- practice — no Gundrak file calls Map:SaveInstanceData, so the set
-- is rebuilt from OnCreatureCreate on every load exactly like the
-- C++ member, and no OnLoad clear is needed (OnLoad fires after
-- creature materialization in the entry flow, so a clear there
-- would wipe the live set).
--
-- STAY-OUTS (statue/altar machine — triaged this tick, queued next):
-- door/object data, OnGameObjectCreate statue/altar GO-state legs,
-- SetBossState altar flag clears, CheckRequiredBosses gates,
-- SetData(DATA_STATUE_ACTIVATE 15) -> Update fire-beam machine +
-- bridge/trapdoor/collision toggles, WriteSaveDataMore statue states.
-- Bridges verified live this tick: OnSetData (8, fires from
-- instanceData:SetData — the altar gossip trigger leg lands in
-- go_gundrak_altar.lua this tick), OnUpdate (3, instance_oculus.lua
-- poll precedent), OnGameObjectCreate (6, GO latch by entry),
-- creature:GetBossState probe (anyCreature pattern,
-- lua_creature_events.go:6438), go:SetGoState / go:SetFlag (orb
-- precedent). Gaps to verify next tick: go:RemoveFlag on GO objects,
-- FindNearestCreature on latched altar GOs (NPC_ALTAR_TRIGGER 30298,
-- 10y), Map:SaveInstanceData trigger for the statue states.

local MAP_GUNDRAK = 604

local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local NPC_RUIN_DWELLER = 29920 -- gundrak.h NPC_RUIN_DWELLER
local NPC_ECK_THE_FEROCIOUS = 29932 -- gundrak.h NPC_ECK_THE_FEROCIOUS

-- EckSpawnPoint (instance_gundrak.cpp:60).
local ECK_SPAWN_X = 1643.877930
local ECK_SPAWN_Y = 936.278015
local ECK_SPAWN_Z = 107.204948
local ECK_SPAWN_O = 0.668432

local SUMMON_TYPE_CORPSE_TIMED_DESPAWN = 5
local ECK_DESPAWN_MS = 300000 -- 300s

local function dwellerSet(instanceData)
    local set = instanceData.dwellerGUIDs
    if set == nil then
        set = {}
        instanceData.dwellerGUIDs = set
    end
    return set
end

-- C++ OnCreatureCreate (instance_gundrak.cpp:86-99): alive ruin
-- dwellers join DwellerGUIDs.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_RUIN_DWELLER and creature:IsAlive() then
        dwellerSet(instanceData)[creature:GetGUID()] = true
    end
end)

-- C++ OnUnitDeath (instance_gundrak.cpp:187-196): the dead dweller
-- leaves the set; when it empties the dying unit summons Eck at the
-- fixed spawn point.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil then
        return
    end
    if unit:GetEntry() ~= NPC_RUIN_DWELLER then
        return
    end
    local set = dwellerSet(instanceData)
    set[unit:GetGUID()] = nil
    if next(set) == nil then
        unit:SummonCreature(NPC_ECK_THE_FEROCIOUS, ECK_SPAWN_X, ECK_SPAWN_Y, ECK_SPAWN_Z, ECK_SPAWN_O, SUMMON_TYPE_CORPSE_TIMED_DESPAWN, ECK_DESPAWN_MS)
    end
end)
