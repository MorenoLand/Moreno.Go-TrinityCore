-- The Utgarde Keep instance script — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/instance_utgarde_keep.cpp
-- (class instance_utgarde_keep : public InstanceMapScript, map 574).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua / instance_halls_of_lightning.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 574
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- keleseth / skarvald / dalronn / ingvar — the creature objects bound
-- by ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches,
-- instance_utgarde_keep.cpp:44-68).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the four binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - GetGuidData (instance_utgarde_keep.cpp:120-139): the Lua mirror is
--   the instanceData binds above, read as instanceData.<boss> by the
--   boss files (oculus precedent). The C++ consumers: the frost-tomb
--   JustDied cross-creature SetData (boss_keleseth.cpp:101-105) rides
--   the ev5 SetGuidData latch; the skarvald/dalronn otherBoss lookup
--   rides the instance GUID store (ev5 SetGuidData latches in
--   boss_skarvald_dalronn.lua, replacing the retired per-pull death
--   counter + FindNearestCreature relay); the Annhylde resurrect
--   machine (boss_ingvar_the_plunderer.cpp:302-316) rides the same
--   store via the ingvar ev5 latch.
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_GIANT_PORTCULLIS_1 186756 / GO_GIANT_PORTCULLIS_2
--   186694, DOOR_TYPE_PASSAGE tied to DATA_INGVAR,
--   instance_utgarde_keep.cpp:24-30): no boss-state door machine in Go
--   (oculus/naxxramas precedent).
-- - Minion data (NPC_SKARVALD / NPC_DALRONN -> DATA_SKARVALD_DALRONN,
--   instance_utgarde_keep.cpp:32-37): Go has no minion registry; the
--   DONE minion guard (InstanceScript.cpp:362-366) is reproduced in
--   boss_skarvald_dalronn.lua by the per-pull death counter — DONE lands
--   only on the second death of the pull (the assembly-of-iron precedent).
--   OnCreatureRemove AddMinion(creature, false) (:76-86) is a no-op here.
-- - OnGameObjectCreate forge GO latches (instance_utgarde_keep.cpp:88-118):
--   static template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:291-306, oculus precedent), so the latches are
--   vacuous here — their two effects (store the GUIDs, re-apply the
--   saved event) are reproduced by the low-GUID resolver + the
--   OnSetData consumer / player-enter re-apply above.
-- - SetData DATA_FORGE_1/2/3 (instance_utgarde_keep.cpp:141-160):
--   the OnSetData consumer below runs HandleGameObject on the
--   forge's anvil/bellow/fire via map:GetGameObjectByLowGuid
--   (the low-GUID GO resolver bridge); the event is stored on
--   instanceData (the WriteSaveDataMore / ReadSaveDataMore store,
--   :162-172) and re-applied on player enter, since C++ re-applies
--   through OnGameObjectCreate on map load and static template GOs
--   never pass the Go GO-create funnel (blackwing_lair precedent).
--   DONE also persists the table (C++ SaveToDB, :170).
-- - SetHeaders(DataHeader "UK") / SetBossNumber(EncounterCount 3) /
--   LoadDoorData / LoadMinionData: no bridges; the boss-state store is
--   written via creature:SetBossState (lua_creature_events.go:5908) and
--   read via creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).
-- No ON_UPDATE poll: unlike the Loken globe in
-- instance_halls_of_lightning.lua, this instance script has no
-- SetBossState override and no DONE-gated leg of its own — every boss
-- state is written by the boss files and consumed only by the unported
-- machines above — so there is nothing for a poll to drive.

local MAP_UTGARDE_KEEP = 574
local INSTANCE_EVENT_ON_SET_DATA = 8
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_PRINCE_KELESETH = 23953 -- utgarde_keep.h:46
local NPC_SKARVALD = 24200 -- utgarde_keep.h:47
local NPC_DALRONN = 24201 -- utgarde_keep.h:48
local NPC_INGVAR = 23954 -- utgarde_keep.h:49

local DATA_FORGE_1 = 5 -- utgarde_keep.h:39
local DATA_FORGE_2 = 6 -- utgarde_keep.h:40
local DATA_FORGE_3 = 7 -- utgarde_keep.h:41

local ENCOUNTER_DONE = 3 -- InstanceScript.h:73

local GO_STATE_ACTIVE = 0 -- GameObject.cpp:2407 via InstanceScript::HandleGameObject
local GO_STATE_READY = 1

-- TDB 26091 gameobject rows, map 574, one spawn each (verified):
-- forge 1: anvil 186609 / bellow 186688 / fire 186692
-- forge 2: anvil 186610 / bellow 186689 / fire 186693
-- forge 3: anvil 186611 / bellow 186690 / fire 186691
local FORGE_GUIDS = {
    { 65430, 65408, 65412 }, -- forge 1: anvil, bellow, fire
    { 65431, 65409, 65413 }, -- forge 2: anvil, bellow, fire
    { 65432, 65410, 65411 }, -- forge 3: anvil, bellow, fire
}

-- C++ SetData DATA_FORGE_1/2/3 (instance_utgarde_keep.cpp:159-167):
-- HandleGameObject(guid, data != NOT_STARTED) on the forge's anvil,
-- bellow and fire — open (GO_STATE_ACTIVE) while the forge-master
-- fight runs or is done, READY while NOT_STARTED. A GUID that does
-- not resolve reads nil and is skipped, the C++ GetGameObject(Empty)
-- no-op arm.
local function applyForge(mapObj, forgeIndex, data)
    local guids = FORGE_GUIDS[forgeIndex]
    if guids == nil or mapObj == nil then
        return
    end
    local state = GO_STATE_READY
    if data ~= 0 then -- data != NOT_STARTED (InstanceScript.h:70)
        state = GO_STATE_ACTIVE
    end
    for _, lowGuid in ipairs(guids) do
        local go = mapObj:GetGameObjectByLowGuid(lowGuid)
        if go ~= nil then
            go:SetGoState(state)
        end
    end
end

RegisterMapEvent(MAP_UTGARDE_KEEP, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type < DATA_FORGE_1 or type > DATA_FORGE_3 then
        return
    end
    local forgeIndex = type - DATA_FORGE_1 + 1
    instanceData["forge" .. forgeIndex] = data
    applyForge(mapObj, forgeIndex, data)
    if data == ENCOUNTER_DONE then -- C++ SaveToDB (:170)
        mapObj:SaveInstanceData()
    end
end)

-- C++ OnGameObjectCreate re-applies the saved forge event when the
-- map loads (instance_utgarde_keep.cpp:88-118); static template GOs
-- never pass the Go GO-create funnel, so the re-apply rides player
-- enter instead (blackwing_lair precedent).
RegisterMapEvent(MAP_UTGARDE_KEEP, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    for forgeIndex = 1, 3 do
        local data = instanceData["forge" .. forgeIndex]
        if data ~= nil then
            applyForge(mapObj, forgeIndex, data)
        end
    end
end)

local bossBinds = {
    [NPC_PRINCE_KELESETH] = "keleseth",
    [NPC_SKARVALD] = "skarvald",
    [NPC_DALRONN] = "dalronn",
    [NPC_INGVAR] = "ingvar",
}

RegisterMapEvent(MAP_UTGARDE_KEEP, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
