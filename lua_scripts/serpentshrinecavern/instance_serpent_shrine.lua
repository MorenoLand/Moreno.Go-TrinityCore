-- Serpent Shrine Cavern instance script — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- instance_serpent_shrine.cpp (instance_serpent_shrine,
-- InstanceMapScript "instance_serpent_shrine", map 548) and
-- serpent_shrine.h (SSDataTypes, SSWaterEventState).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_gundrak.lua precedent).
--
-- PORTED — SetData machine (OnSetData, event 8; the
-- instanceData:SetData bridge fires event 8 with (event,
-- instanceData, map, type, data) and stores nothing itself,
-- engine/world/instance_hooks.go:51-65 — the handler below runs
-- the C++ SetData switch, instance_serpent_shrine.cpp:279-346):
-- encounters 0-5 (DATA_HYDROSSTHEUNSTABLEEVENT 3 / LEOTHERAS-
-- THEBLINDEVENT 9 / THELURKERBELOWEVENT 17 / KARATHRESSEVENT 5 /
-- MOROGRIMTIDEWALKEREVENT 10 / LADYVASHJEVENT 8) in
-- instanceData.encounters[1..6] (EncounterState: NOT_STARTED 0,
-- IN_PROGRESS 1, DONE 3 — InstanceScript.h:68-76); shield
-- generators 1-4 (DATA_SHIELDGENERATOR1-4 = 12-15) as bool
-- latches, with the derived instanceData.canStartPhase3 (C++
-- GetData(DATA_CANSTARTPHASE3 1, :348-386) returns 1 iff all four
-- are deactivated); DATA_STRANGE_POOL 23, DATA_WATER 24,
-- DATA_TRASH 25 (count += 1 while < 30, SaveToDB on every write,
-- :294-298); DATA_CONTROL_CONSOLE 22 (DONE -> the three bridge
-- parts 184203/184204/184205 go GO_STATE_ACTIVE via
-- HandleGameObject(part, true), InstanceScript.cpp:479-484).
-- Any SetData with value DONE saves (the trailing `if (data ==
-- DONE) SaveToDB()`, :345); the DATA_FATHOMLORDKARATHRESSEVENT
-- 19 write from karathress JustDied hits the C++ default arm,
-- so it only saves.
-- PORTED — GetData shape: there is NO instanceData:GetData
-- method in the engine (instance_hooks.go instanceDataObject
-- attaches only SetData + Start/StopTimedAchievement) — boss
-- files read the plain fields the handler maintains
-- (encounters[i], shieldGenerator1-4, canStartPhase3,
-- strangePool, water, trashCount), the Gundrak flag-field
-- precedent. A `data:GetData(x)` call would read a nil field
-- and error; never use it.
-- PORTED — GetGuidData shape: likewise no instanceData:
-- GetGuidData/SetGuidData method exists. OnCreatureCreate
-- (event 5, :213-241) latches each boss GUID-low number
-- (creature:GetGUIDLow() — the Gundrak GUID-KEY FIX: GetGUID()
-- userdata keys never match across calls, and a float64
-- round-trip would lose the low bits) in instanceData.<boss>
-- GuidLow fields, and the two SetGuidData starters
-- (DATA_KARATHRESSEVENT_STARTER 6, DATA_LEOTHERAS_EVENT_STARTER
-- 21, :243-249) ride plain `instanceData.<...>EventStarterLow`
-- number fields written by the boss files. The C++ GetGuidData
-- reads (:251-277) resolve to live creatures in the boss files
-- via FindNearestCreature (maulgar/relay precedent) — the
-- GUID-low fields are the recorded instance state, not a
-- resolution bridge; nothing resolves a GUID-low to a unit.
-- PORTED — water machine (Update, :125-182, via OnUpdate event
-- 3, which fires per Map::Update with players present and
-- passes diff in ms — instance_hooks.go:637-640): 500ms water
-- check accumulator + 2s frenzy-spawn flag accumulator, both
-- kept in instanceData (per-instance state; file-local upvalues
-- would be shared across instances). TrashCount >= 30 ->
-- SCALDING (2) else FRENZY (1); for each map player, alive and
-- in water: scalding -> self-cast 37284 (player:CastSpell, no
-- triggered arg — the player funnel is triggered-only,
-- lua_player.go CastCustomSpell comment) if the aura is absent;
-- frenzy -> if the flag is set, SummonCreature(21508, player
-- pos, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT type 3, 2000ms)
-- + Attack(player) + SetSwim(true) + SetDisableGravity(true),
-- flag cleared (first in-water player consumes it, C++-exact).
-- Out of water -> RemoveAurasDueToSpell(37284) via
-- player:RemoveAura, unconditional on alive (C++-exact). The
-- C++ no-players early return (:140-141) freezes both timers;
-- event 3 only fires with players present, so the guard below
-- is faithful dead code.
-- PORTED — Save/Load (GetSaveData :388-396, Load :398-424):
-- the 6 encounters + TrashCount persist through the
-- instanceData table itself (MarshalInstanceData supports
-- nested tables — engine/scripting/instance_data.go:67-74);
-- OnLoad (event 2) resets any IN_PROGRESS encounter to
-- NOT_STARTED (C++ Load behavior) and restores the ctor
-- defaults for the non-persisted state (water FRENZY, shield
-- gens false, console not done, timers 500/2000/false) plus
-- clears the two starter lows (fresh-script semantics — a new
-- C++ InstanceScript constructs them empty). The create-latched
-- GUID-low fields are NOT cleared on load: OnLoad fires after
-- creature materialization (Gundrak precedent), so the latches
-- are already fresh.
-- PORTED — bridge console GO (go_bridge_console.lua, the
-- go_gundrak_altar.lua split): OnGossipHello ->
-- instanceData:SetData(DATA_CONTROL_CONSOLE, DONE).
-- Static GO legs (OnGameObjectCreate :184-211): the console /
-- bridge-part setActive + SetFarVisible arms stay out — no GO
-- SetActive bridge exists, and SetFarVisible is creature-only
-- (lua_creature_events.go:5775); the bridge-part GO_STATE legs
-- ride the event-6 handler (dynamic spawns) plus the
-- OnPlayerEnter apply (static spawns, Gundrak precedent) and
-- the event-8 DONE leg itself (the gossiping player anchors
-- the FindNearestGameObject resolution).
--
-- STAY-OUTS (triaged this tick):
-- DATA_TRASH has no writer in the C++ tree: no SSC creature
-- script calls SetData(DATA_TRASH, 1) (the only DATA_TRASH
-- writers are Hyjal's own scripts) — the count arm is
-- implemented per the instance switch, but nothing increments
-- it, so the water stays FRENZY until a trash script lands.
-- go_strange_pool (boss_lurker_below.cpp:450-488, entry
-- unnumbered in the C++ file — no GO entry constant exists for
-- it there) is not ported: its OnGossipHello (25% fishing roll,
-- 54587 cast, DATA_STRANGE_POOL transitions) needs its own GO
-- file and was outside this unit's named file set; the
-- DATA_STRANGE_POOL machine above accepts its writes, and the
-- lurker's CheckCanStart read is wired in boss_lurker_below.lua.
-- IsEncounterInProgress (:114-123) — no bridge (no event fires
-- on the check); nothing consumes it in Lua. SetHeaders /
-- DataHeader "SS" — no bridge (ICC precedent). The FishingTimer
-- member (:111, never read in Update) is dead in C++ too.

local MAP_SSC = 548

local INSTANCE_EVENT_ON_LOAD = 2
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_SET_DATA = 8

-- serpent_shrine.h SSDataTypes.
local DATA_HYDROSSTHEUNSTABLEEVENT = 3
local DATA_KARATHRESSEVENT = 5
local DATA_LADYVASHJEVENT = 8
local DATA_LEOTHERASTHEBLINDEVENT = 9
local DATA_MOROGRIMTIDEWALKEREVENT = 10
local DATA_SHIELDGENERATOR1 = 12
local DATA_SHIELDGENERATOR2 = 13
local DATA_SHIELDGENERATOR3 = 14
local DATA_SHIELDGENERATOR4 = 15
local DATA_THELURKERBELOWEVENT = 17
local DATA_CONTROL_CONSOLE = 22
local DATA_STRANGE_POOL = 23
local DATA_WATER = 24
local DATA_TRASH = 25

-- EncounterState (InstanceScript.h:68-76).
local ENCOUNTER_NOT_STARTED = 0
local ENCOUNTER_IN_PROGRESS = 1
local ENCOUNTER_DONE = 3

-- serpent_shrine.h SSWaterEventState.
local WATERSTATE_FRENZY = 1
local WATERSTATE_SCALDING = 2

-- instance_serpent_shrine.cpp Misc enum (:40-49).
local SPELL_SCALDINGWATER = 37284
local NPC_COILFANG_FRENZY = 21508
local MIN_KILLS = 30

local GO_BRIDGE_PARTS = { 184203, 184204, 184205 }

local GO_STATE_ACTIVE = 0

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

-- C++ TempSummonType (TemporarySummon.h): type 3 is
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (the vashj coilfang
-- arms use the same type).
local SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT = 3
local FRENZY_DESPAWN_MS = 2000 -- C++ 2s

-- C++ SetData encounter types (:279-346) -> encounters[1..6]
-- (C++ m_auiEncounter[0..5]).
local DATA_TO_ENCOUNTER_INDEX = {
    [DATA_HYDROSSTHEUNSTABLEEVENT] = 1,
    [DATA_LEOTHERASTHEBLINDEVENT] = 2,
    [DATA_THELURKERBELOWEVENT] = 3,
    [DATA_KARATHRESSEVENT] = 4,
    [DATA_MOROGRIMTIDEWALKEREVENT] = 5,
    [DATA_LADYVASHJEVENT] = 6,
}

-- C++ OnCreatureCreate entry -> GUID-low field (:213-241).
local ENTRY_TO_GUID_LOW_KEY = {
    [21212] = "vashjGuidLow",
    [21214] = "karathressGuidLow",
    [21966] = "sharkkisGuidLow",
    [21217] = "lurkerGuidLow",
    [21965] = "tidalvessGuidLow",
    [21964] = "caribdisGuidLow",
    [21215] = "leotherasGuidLow",
}

local SHIELD_GENERATOR_KEYS = {
    [DATA_SHIELDGENERATOR1] = "shieldGenerator1",
    [DATA_SHIELDGENERATOR2] = "shieldGenerator2",
    [DATA_SHIELDGENERATOR3] = "shieldGenerator3",
    [DATA_SHIELDGENERATOR4] = "shieldGenerator4",
}

local function encounters(instanceData)
    local enc = instanceData.encounters
    if enc == nil then
        enc = { 0, 0, 0, 0, 0, 0 }
        instanceData.encounters = enc
    end
    return enc
end

-- C++ GetData(DATA_CANSTARTPHASE3) (:348-386): 1 iff all four
-- shield generators are deactivated. Maintained as a field so
-- boss files can read it without a GetData bridge.
local function refreshCanStartPhase3(instanceData)
    instanceData.canStartPhase3 = (instanceData.shieldGenerator1 == true)
        and (instanceData.shieldGenerator2 == true)
        and (instanceData.shieldGenerator3 == true)
        and (instanceData.shieldGenerator4 == true)
end

-- Entry-based GO resolution from any player in the instance
-- (player:FindNearestGameObject covers DB spawns; the
-- instance_gundrak.lua precedent).
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

-- C++ SetData(DATA_CONTROL_CONSOLE, DONE) (:286-292):
-- HandleGameObject(BridgePart[i], true) -> GO_STATE_ACTIVE
-- (InstanceScript.cpp:479-484).
local function applyBridgeParts(instanceData, mapObj)
    if not instanceData.controlConsoleDone then
        return
    end
    for _, entry in ipairs(GO_BRIDGE_PARTS) do
        local go = findGO(mapObj, entry)
        if go then
            go:SetGoState(GO_STATE_ACTIVE)
        end
    end
end

-- C++ OnGameObjectCreate (:184-211): only the bridge parts carry
-- a state leg (the console / bridge-part setActive + SetFarVisible
-- arms have no GO bridge — header). Fires for dynamic spawns
-- only in Go; the static-spawn path is the OnPlayerEnter apply
-- below, plus the event-8 DONE leg.
local function applyGOCreateState(instanceData, mapObj, go)
    local entry = go:GetEntry()
    if entry == GO_BRIDGE_PARTS[1] or entry == GO_BRIDGE_PARTS[2] or entry == GO_BRIDGE_PARTS[3] then
        if instanceData.controlConsoleDone then
            go:SetGoState(GO_STATE_ACTIVE)
        end
    end
end

-- C++ OnCreatureCreate (instance_serpent_shrine.cpp:213-241):
-- latch the boss GUID-low numbers.
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = ENTRY_TO_GUID_LOW_KEY[creature:GetEntry()]
    if key then
        instanceData[key] = creature:GetGUIDLow()
    end
end)

-- C++ OnGameObjectCreate (instance_serpent_shrine.cpp:184-211).
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    applyGOCreateState(instanceData, mapObj, go)
end)

-- Static-spawn path for the bridge legs: re-apply the bridge
-- parts from the persisted console state on player enter
-- (idempotent — it only acts once the console is done).
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    applyBridgeParts(instanceData, mapObj)
end)

-- C++ Load (instance_serpent_shrine.cpp:398-424): the blob is
-- unmarshalled into the table before event 2 fires
-- (instance_hooks.go:414-463); IN_PROGRESS encounters reset to
-- NOT_STARTED, everything else keeps its persisted value. The
-- non-persisted members take their ctor defaults (:99-113).
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_LOAD, function(event, instanceData, mapObj)
    local enc = encounters(instanceData)
    for i = 1, 6 do
        if enc[i] == ENCOUNTER_IN_PROGRESS then
            enc[i] = ENCOUNTER_NOT_STARTED
        end
    end
    instanceData.water = WATERSTATE_FRENZY
    instanceData.shieldGenerator1 = false
    instanceData.shieldGenerator2 = false
    instanceData.shieldGenerator3 = false
    instanceData.shieldGenerator4 = false
    instanceData.canStartPhase3 = false
    instanceData.controlConsoleDone = false
    instanceData.waterCheckMs = 500
    instanceData.frenzySpawnMs = 2000
    instanceData.doSpawnFrenzy = false
    instanceData.karathressEventStarterLow = nil
    instanceData.leotherasEventStarterLow = nil
end)

-- C++ SetData (instance_serpent_shrine.cpp:279-346).
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    local saveNow = (value == ENCOUNTER_DONE)
    local encounterIndex = DATA_TO_ENCOUNTER_INDEX[type]
    if encounterIndex ~= nil then
        encounters(instanceData)[encounterIndex] = value
        if type == DATA_LADYVASHJEVENT and value == ENCOUNTER_NOT_STARTED then
            -- C++ :325-334: a Vashj NOT_STARTED write resets the
            -- shield generators.
            instanceData.shieldGenerator1 = false
            instanceData.shieldGenerator2 = false
            instanceData.shieldGenerator3 = false
            instanceData.shieldGenerator4 = false
            instanceData.canStartPhase3 = false
        end
    elseif SHIELD_GENERATOR_KEYS[type] ~= nil then
        instanceData[SHIELD_GENERATOR_KEYS[type]] = (value ~= 0)
        refreshCanStartPhase3(instanceData)
    elseif type == DATA_STRANGE_POOL then
        instanceData.strangePool = value
    elseif type == DATA_WATER then
        instanceData.water = value
    elseif type == DATA_TRASH then
        -- C++ :294-298: count += 1 while < 30, SaveToDB on every
        -- write (not just on increment).
        if value == 1 and (instanceData.trashCount or 0) < MIN_KILLS then
            instanceData.trashCount = (instanceData.trashCount or 0) + 1
        end
        saveNow = true
    elseif type == DATA_CONTROL_CONSOLE then
        if value == ENCOUNTER_DONE then
            instanceData.controlConsoleDone = true
            applyBridgeParts(instanceData, mapObj)
        end
    end
    -- C++ :345: any SetData with value DONE saves; the
    -- DATA_FATHOMLORDKARATHRESSEVENT (19) default arm lands here.
    if saveNow and mapObj then
        mapObj:SaveInstanceData()
    end
end)

-- C++ Update (instance_serpent_shrine.cpp:125-182).
RegisterMapEvent(MAP_SSC, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    local waterCheck = (instanceData.waterCheckMs or 500) - diff
    if waterCheck <= 0 then
        if (instanceData.trashCount or 0) >= MIN_KILLS then
            instanceData.water = WATERSTATE_SCALDING
        else
            instanceData.water = WATERSTATE_FRENZY
        end
        local players = mapObj and mapObj:GetPlayers() or nil
        if players == nil or #players == 0 then
            return -- C++ :140-141 returns here; both timers freeze
        end
        for _, player in ipairs(players) do
            if player:IsAlive() and player:IsInWater() then
                if instanceData.water == WATERSTATE_SCALDING then
                    if not player:HasAura(SPELL_SCALDINGWATER) then
                        player:CastSpell(player, SPELL_SCALDINGWATER)
                    end
                elseif instanceData.doSpawnFrenzy then
                    local frenzy = player:SummonCreature(NPC_COILFANG_FRENZY,
                        player:GetX(), player:GetY(), player:GetZ(), player:GetO(),
                        SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT, FRENZY_DESPAWN_MS)
                    if frenzy then
                        frenzy:Attack(player)
                        frenzy:SetSwim(true)
                        frenzy:SetDisableGravity(true)
                    end
                    instanceData.doSpawnFrenzy = false
                end
            end
            if not player:IsInWater() then
                player:RemoveAura(SPELL_SCALDINGWATER)
            end
        end
        waterCheck = 500
    end
    instanceData.waterCheckMs = waterCheck
    local frenzySpawn = (instanceData.frenzySpawnMs or 2000) - diff
    if frenzySpawn <= 0 then
        instanceData.doSpawnFrenzy = true
        frenzySpawn = 2000
    end
    instanceData.frenzySpawnMs = frenzySpawn
end)
