-- Gundrak instance script — Lua port of
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
-- GUID-KEY FIX (this tick): the set is keyed on
-- creature:GetGUIDLow(), NOT creature:GetGUID(). GetGUID() returns a
-- fresh UInt64 userdata per call (engine/scripting/object.go
-- pushUInt64) and Lua table keys compare userdata by identity —
-- runtime-proved on the go-lua in go.mod, even with an always-true
-- __eq a second userdata wrapping the same value misses — so the
-- old GetGUID() keys never matched across calls: the delete
-- no-opped, the set never emptied, Eck never summoned. GetGUIDLow()
-- is a stable uint32 number (blackfathom precedent).
--
-- PORTED — statue / altar machine (this tick):
-- SetData(DATA_STATUE_ACTIVATE 15) -> Update fire-beam machine +
-- bridge / trapdoor / collision toggles + SaveToDB
-- (instance_gundrak.cpp:256-260, :322-348). OnSetData (event 8)
-- maps the gossiped altar entry to its statue event and schedules
-- it 3500ms out (TIMER_STATUE_ACTIVATION, CreateLuaEvent — the
-- oculus precedent); the event fn casts the fire beam from the
-- altar trigger (NPC_ALTAR_TRIGGER 30298 within 10y of the altar —
-- go:FindNearestCreature, lua_objects.go:4133), toggles the statue
-- to READY, schedules the bridge event when all three statues are
-- READY, and saves. The bridge fn toggles statues 8-11 DESTROYED,
-- trapdoor READY, collision ACTIVE, and saves. Statue states live
-- as instanceData numbers (marshal-safe, nil = ACTIVE) and persist
-- through Map:SaveInstanceData (scripting.MarshalInstanceData).
-- The beam cast is trigger:CastSpell(trigger, spell, true): C++
-- casts with a nullptr target (trigger->CastSpell(nullptr, spell,
-- true)) and the Go bridge has no null-target arm — a nil target
-- misroutes to the victim (lua_creature_events.go:904-911) — so
-- self-cast is the twin; dormant until Spell.dbc ships (the
-- spellKnown gate), like every other creature spell leg.
-- Static GOs never fire OnGameObjectCreate in Go
-- (triggerInstanceGameObjectCreate is dynamic-spawn-only,
-- instance_hooks.go:488-503), so the C++ create-time GO-state legs
-- ride two live paths instead: the event-6 handler (dynamic spawns,
-- the ruby-sanctum precedent) and an OnPlayerEnter apply (the
-- static-spawn path — the entering player is the search anchor).
-- GO resolution is entry-based via player:FindNearestGameObject
-- (lua_player.go:6710, DB spawns + runtime GOs, 1000y — the whole
-- instance; entries are unique per GO here). NO full GUID is ever
-- stored in instanceData: GetGUID() userdata keys never match
-- across calls (see above) and a float64 round-trip would lose the
-- low bits anyway (engine/scripting/object.go:567).
-- OnLoad (event 2) schedules the bridge event when the persisted
-- states are all READY (ReadSaveDataMore, :274-290 — the blob is
-- unmarshalled into the table before event 2 fires,
-- instance_hooks.go:414-463).
--
-- PORTED — SetBossState altar-clear legs (this tick):
-- C++ SetBossState (instance_gundrak.cpp:198-225) clears
-- NOT_SELECTABLE on the boss's altar at DONE, but Go fires no event
-- on boss-state writes (setInstanceBossState,
-- instance_boss_states.go:162) and player:FindNearestCreature skips
-- dead motions (lua_player.go:6678), so no anchor can read a dead
-- boss's state. The oculus pattern instead: the three boss files
-- write instanceData.<boss>Done = true in their ev4 handlers in the
-- same breath as creature:SetBossState(DATA_X, DONE) (BossAI
-- _JustDied, ScriptedCreature.cpp:513-520), and the OnUpdate poll
-- (event 3, fires per Map::Update with players present) clears each
-- altar flag once. The create-time apply reads the same flags in
-- place of GetBossState (equivalent — flag and state are written
-- together).
--
-- STAY-OUTS (triaged this tick):
-- doorData (6 rows, :30-40) + LoadDoorData — Go has no door model:
-- setInstanceBossState explicitly skips door updates
-- (instance_boss_states.go:148-152), same as the ICC door legs.
-- CheckRequiredBosses (DATA_ECK heroic+Moorabi, DATA_GAL_DARAH all
-- three statues DESTROYED, :227-253) — no bridge
-- (creaturemotion.go:3756); the DERIVED GO-state legs (gal'darah
-- statue DESTROYED/READY, trapdoor, collision) ARE ported above —
-- they read the Lua statue states, not the gate. The Eck-door
-- non-heroic leg (:169-173) is vacuous without a door store
-- (map:IsHeroic is bridged, globals.go:1231, but there is no
-- consumer for the latch). LoadObjectData / SetHeaders /
-- SetBossNumber — no bridges (ICC precedent). The ctor statue-state
-- init (ACTIVE, :79-83) is the nil-default in statueState() below.

local MAP_GUNDRAK = 604

local INSTANCE_EVENT_ON_LOAD = 2
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_SET_DATA = 8
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local NPC_RUIN_DWELLER = 29920 -- gundrak.h NPC_RUIN_DWELLER
local NPC_ECK_THE_FEROCIOUS = 29932 -- gundrak.h NPC_ECK_THE_FEROCIOUS
local NPC_ALTAR_TRIGGER = 30298 -- gundrak.h NPC_ALTAR_TRIGGER

-- gundrak.h GDDataTypes (boss ids 0-2 live in the boss files).
local DATA_SLAD_RAN_ALTAR = 5
local DATA_DRAKKARI_COLOSSUS_ALTAR = 6
local DATA_MOORABI_ALTAR = 7
local DATA_SLAD_RAN_STATUE = 8
local DATA_DRAKKARI_COLOSSUS_STATUE = 9
local DATA_MOORABI_STATUE = 10
local DATA_GAL_DARAH_STATUE = 11
local DATA_TRAPDOOR = 12
local DATA_COLLISION = 13
local DATA_BRIDGE = 14
local DATA_STATUE_ACTIVATE = 15

-- gundrak.h GDGameObjectIds.
local GO_SLAD_RAN_ALTAR = 192518
local GO_MOORABI_ALTAR = 192519
local GO_DRAKKARI_COLOSSUS_ALTAR = 192520
local GO_SLAD_RAN_STATUE = 192564
local GO_MOORABI_STATUE = 192565
local GO_GAL_DARAH_STATUE = 192566
local GO_DRAKKARI_COLOSSUS_STATUE = 192567
local GO_TRAPDOOR = 193188
local GO_COLLISION = 192633

-- gundrak.h GDSpellIds.
local SPELL_FIRE_BEAM_MAMMOTH = 57068
local SPELL_FIRE_BEAM_SNAKE = 57071
local SPELL_FIRE_BEAM_ELEMENTAL = 57072

local TIMER_STATUE_ACTIVATION = 3500 -- gundrak.h TIMER_STATUE_ACTIVATION

local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1
local GO_STATE_DESTROYED = 2
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

-- EckSpawnPoint (instance_gundrak.cpp:60).
local ECK_SPAWN_X = 1643.877930
local ECK_SPAWN_Y = 936.278015
local ECK_SPAWN_Z = 107.204948
local ECK_SPAWN_O = 0.668432

local SUMMON_TYPE_CORPSE_TIMED_DESPAWN = 5
local ECK_DESPAWN_MS = 300000 -- 300s

-- C++ ToggleGameObject (:292-314) data-id -> GO entry.
local DATA_ID_TO_ENTRY = {
    [DATA_SLAD_RAN_ALTAR] = GO_SLAD_RAN_ALTAR,
    [DATA_DRAKKARI_COLOSSUS_ALTAR] = GO_DRAKKARI_COLOSSUS_ALTAR,
    [DATA_MOORABI_ALTAR] = GO_MOORABI_ALTAR,
    [DATA_SLAD_RAN_STATUE] = GO_SLAD_RAN_STATUE,
    [DATA_DRAKKARI_COLOSSUS_STATUE] = GO_DRAKKARI_COLOSSUS_STATUE,
    [DATA_MOORABI_STATUE] = GO_MOORABI_STATUE,
    [DATA_GAL_DARAH_STATUE] = GO_GAL_DARAH_STATUE,
    [DATA_TRAPDOOR] = GO_TRAPDOOR,
    [DATA_COLLISION] = GO_COLLISION,
}

-- C++ SetData (:256-260): altar entry -> statue event id.
local ALTAR_TO_STATUE_EVENT = {
    [GO_SLAD_RAN_ALTAR] = DATA_SLAD_RAN_STATUE,
    [GO_DRAKKARI_COLOSSUS_ALTAR] = DATA_DRAKKARI_COLOSSUS_STATUE,
    [GO_MOORABI_ALTAR] = DATA_MOORABI_STATUE,
}

-- C++ Update statue-event arms (:322-338): event id -> beam spell + altar data id.
local STATUE_EVENT_ARMS = {
    [DATA_SLAD_RAN_STATUE] = { spell = SPELL_FIRE_BEAM_SNAKE, altar = DATA_SLAD_RAN_ALTAR },
    [DATA_DRAKKARI_COLOSSUS_STATUE] = { spell = SPELL_FIRE_BEAM_ELEMENTAL, altar = DATA_DRAKKARI_COLOSSUS_ALTAR },
    [DATA_MOORABI_STATUE] = { spell = SPELL_FIRE_BEAM_MAMMOTH, altar = DATA_MOORABI_ALTAR },
}

-- Statue-state instanceData keys (numbers; nil = ACTIVE, the ctor init).
local STATUE_STATE_KEYS = {
    [DATA_SLAD_RAN_STATUE] = "sladRanStatueState",
    [DATA_DRAKKARI_COLOSSUS_STATUE] = "drakkariStatueState",
    [DATA_MOORABI_STATUE] = "moorabiStatueState",
}

-- C++ SetBossState altar-clear legs (:198-225): boss DONE flag ->
-- cleared flag + altar entry (the OnUpdate poll below).
local ALTAR_CLEAR_LEGS = {
    { doneKey = "sladRanDone", clearedKey = "sladRanAltarCleared", altarEntry = GO_SLAD_RAN_ALTAR },
    { doneKey = "drakkariDone", clearedKey = "drakkariAltarCleared", altarEntry = GO_DRAKKARI_COLOSSUS_ALTAR },
    { doneKey = "moorabiDone", clearedKey = "moorabiAltarCleared", altarEntry = GO_MOORABI_ALTAR },
}

-- GOs the create-time apply covers (statue/altar machine set; the
-- Eck door has no consumer without a door model).
local CREATE_STATE_ENTRIES = {
    GO_SLAD_RAN_ALTAR, GO_MOORABI_ALTAR, GO_DRAKKARI_COLOSSUS_ALTAR,
    GO_SLAD_RAN_STATUE, GO_MOORABI_STATUE, GO_GAL_DARAH_STATUE,
    GO_DRAKKARI_COLOSSUS_STATUE, GO_TRAPDOOR, GO_COLLISION,
}

local function statueState(instanceData, statueId)
    return instanceData[STATUE_STATE_KEYS[statueId]] or GO_STATE_ACTIVE
end

local function isBridgeReady(instanceData)
    return statueState(instanceData, DATA_SLAD_RAN_STATUE) == GO_STATE_READY
        and statueState(instanceData, DATA_DRAKKARI_COLOSSUS_STATUE) == GO_STATE_READY
        and statueState(instanceData, DATA_MOORABI_STATUE) == GO_STATE_READY
end

-- CheckRequiredBosses(DATA_GAL_DARAH) re-expressed on the Lua statue
-- states: all three DESTROYED (instance_gundrak.cpp:241-246).
local function isBridgeDestroyed(instanceData)
    return statueState(instanceData, DATA_SLAD_RAN_STATUE) == GO_STATE_DESTROYED
        and statueState(instanceData, DATA_DRAKKARI_COLOSSUS_STATUE) == GO_STATE_DESTROYED
        and statueState(instanceData, DATA_MOORABI_STATUE) == GO_STATE_DESTROYED
end

-- Entry-based GO resolution from any player in the instance
-- (player:FindNearestGameObject covers DB spawns; the entering
-- player is passed directly by the OnPlayerEnter handler).
local function findGOFromPlayer(player, entry)
    if player == nil then
        return nil
    end
    return player:FindNearestGameObject(entry, GO_SEARCH_DIST)
end

local function findGO(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local go = findGOFromPlayer(player, entry)
        if go then
            return go
        end
    end
    return nil
end

-- C++ ToggleGameObject (instance_gundrak.cpp:292-314): SetGoState
-- on the latched GO + member-state update for the three statues.
local function toggleGameObject(instanceData, mapObj, dataId, state)
    local entry = DATA_ID_TO_ENTRY[dataId]
    if entry == nil then
        return
    end
    local go = findGO(mapObj, entry)
    if go then
        go:SetGoState(state)
    end
    local key = STATUE_STATE_KEYS[dataId]
    if key then
        instanceData[key] = state
    end
end

-- C++ Update DATA_BRIDGE arm (:339-348): statues DESTROYED,
-- trapdoor READY, collision ACTIVE, SaveToDB.
local function runBridgeEvent(instanceData, mapObj)
    for dataId = DATA_SLAD_RAN_STATUE, DATA_GAL_DARAH_STATUE do
        toggleGameObject(instanceData, mapObj, dataId, GO_STATE_DESTROYED)
    end
    toggleGameObject(instanceData, mapObj, DATA_TRAPDOOR, GO_STATE_READY)
    toggleGameObject(instanceData, mapObj, DATA_COLLISION, GO_STATE_ACTIVE)
    if mapObj then
        mapObj:SaveInstanceData()
    end
end

-- C++ Update statue-event arms (:322-338): fire beam, toggle the
-- statue READY, schedule the bridge when ready, SaveToDB. The beam
-- cast is unconditional on the altar resolving — the toggle, bridge
-- check and save run regardless, exactly as in C++.
local function runStatueEvent(instanceData, mapObj, eventId)
    local arm = STATUE_EVENT_ARMS[eventId]
    if arm then
        local altar = findGO(mapObj, DATA_ID_TO_ENTRY[arm.altar])
        if altar then
            local trigger = altar:FindNearestCreature(NPC_ALTAR_TRIGGER, 10)
            if trigger then
                trigger:CastSpell(trigger, arm.spell, true)
            end
        end
    end
    toggleGameObject(instanceData, mapObj, eventId, GO_STATE_READY)
    if isBridgeReady(instanceData) then
        CreateLuaEvent(function()
            runBridgeEvent(instanceData, mapObj)
        end, TIMER_STATUE_ACTIVATION, 1)
    end
    if mapObj then
        mapObj:SaveInstanceData()
    end
end

-- C++ OnGameObjectCreate create-time legs (:101-186), applied to one
-- GO: altar DONE legs, statue state legs, gal'darah statue /
-- trapdoor / collision legs derived from the bridge state. The boss
-- DONE read uses the instanceData flags (written together with
-- SetBossState(DONE) by the boss files — see the header); the Eck
-- door leg is vacuous without a door model.
local function applyGOCreateState(instanceData, mapObj, go)
    local entry = go:GetEntry()
    if entry == GO_SLAD_RAN_ALTAR or entry == GO_DRAKKARI_COLOSSUS_ALTAR or entry == GO_MOORABI_ALTAR then
        local doneKey = nil
        local statueId = nil
        if entry == GO_SLAD_RAN_ALTAR then
            doneKey, statueId = "sladRanDone", DATA_SLAD_RAN_STATUE
        elseif entry == GO_DRAKKARI_COLOSSUS_ALTAR then
            doneKey, statueId = "drakkariDone", DATA_DRAKKARI_COLOSSUS_STATUE
        else
            doneKey, statueId = "moorabiDone", DATA_MOORABI_STATUE
        end
        if instanceData[doneKey] then
            if statueState(instanceData, statueId) == GO_STATE_ACTIVE then
                go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            else
                go:SetGoState(GO_STATE_ACTIVE)
            end
        end
        return
    end
    if entry == GO_SLAD_RAN_STATUE or entry == GO_DRAKKARI_COLOSSUS_STATUE or entry == GO_MOORABI_STATUE then
        local statueId = nil
        if entry == GO_SLAD_RAN_STATUE then
            statueId = DATA_SLAD_RAN_STATUE
        elseif entry == GO_DRAKKARI_COLOSSUS_STATUE then
            statueId = DATA_DRAKKARI_COLOSSUS_STATUE
        else
            statueId = DATA_MOORABI_STATUE
        end
        go:SetGoState(statueState(instanceData, statueId))
        return
    end
    if entry == GO_GAL_DARAH_STATUE then
        go:SetGoState(isBridgeDestroyed(instanceData) and GO_STATE_DESTROYED or GO_STATE_READY)
        return
    end
    if entry == GO_TRAPDOOR then
        go:SetGoState(isBridgeDestroyed(instanceData) and GO_STATE_READY or GO_STATE_ACTIVE)
        return
    end
    if entry == GO_COLLISION then
        go:SetGoState(isBridgeDestroyed(instanceData) and GO_STATE_ACTIVE or GO_STATE_READY)
        return
    end
end

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
        dwellerSet(instanceData)[creature:GetGUIDLow()] = true
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
    set[unit:GetGUIDLow()] = nil
    if next(set) == nil then
        unit:SummonCreature(NPC_ECK_THE_FEROCIOUS, ECK_SPAWN_X, ECK_SPAWN_Y, ECK_SPAWN_Z, ECK_SPAWN_O, SUMMON_TYPE_CORPSE_TIMED_DESPAWN, ECK_DESPAWN_MS)
    end
end)

-- C++ OnGameObjectCreate (instance_gundrak.cpp:101-186): the
-- create-time legs above. Fires only for dynamic spawns in Go; the
-- static-spawn path is the OnPlayerEnter apply below.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    applyGOCreateState(instanceData, mapObj, go)
end)

-- Static-spawn path for the create-time legs: apply every GO state
-- from the persisted statue states / boss DONE flags on player
-- enter (idempotent — it reads current state; guarded by the
-- applied flag so it runs once per instance lifetime).
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if instanceData.createStatesApplied then
        return
    end
    local allResolved = true
    for _, entry in ipairs(CREATE_STATE_ENTRIES) do
        local go = findGOFromPlayer(player, entry)
        if go then
            applyGOCreateState(instanceData, mapObj, go)
        else
            allResolved = false
        end
    end
    if allResolved then
        instanceData.createStatesApplied = true
    end
end)

-- C++ ReadSaveDataMore (instance_gundrak.cpp:274-290): bridge ready
-- on load -> schedule DATA_BRIDGE.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_LOAD, function(event, instanceData, mapObj)
    if isBridgeReady(instanceData) then
        CreateLuaEvent(function()
            runBridgeEvent(instanceData, mapObj)
        end, TIMER_STATUE_ACTIVATION, 1)
    end
end)

-- C++ SetData(DATA_STATUE_ACTIVATE) (instance_gundrak.cpp:256-260):
-- schedule the statue event 3500ms out. Fired by the altar gossip
-- (go_gundrak_altar.lua) via instanceData:SetData.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    if type ~= DATA_STATUE_ACTIVATE then
        return
    end
    local eventId = ALTAR_TO_STATUE_EVENT[value]
    if eventId == nil then
        return
    end
    CreateLuaEvent(function()
        runStatueEvent(instanceData, mapObj, eventId)
    end, TIMER_STATUE_ACTIVATION, 1)
end)

-- C++ SetBossState override (instance_gundrak.cpp:198-225) via the
-- OnUpdate poll: no boss-state-change event exists, so the boss
-- files' DONE flags (see header) drive the once-only altar flag
-- clears. Retries on later ticks if the altar GO is unresolvable.
RegisterMapEvent(MAP_GUNDRAK, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    for _, leg in ipairs(ALTAR_CLEAR_LEGS) do
        if instanceData[leg.doneKey] and not instanceData[leg.clearedKey] then
            local altar = findGO(mapObj, leg.altarEntry)
            if altar then
                altar:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
                instanceData[leg.clearedKey] = true
            end
        end
    end
end)
