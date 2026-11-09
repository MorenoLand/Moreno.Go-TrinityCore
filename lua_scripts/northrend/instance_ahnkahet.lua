-- Ahn'kahet instance script — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/instance_ahnkahet.cpp
-- (instance_ahnkahet, InstanceMapScript "instance_ahnkahet", map 619).
-- RegisterMapEvent binds the map-level Eluna instance hook
-- (instance_gundrak.lua precedent).
--
-- PORTED — OnGameObjectCreate sphere / platform legs
-- (instance_ahnkahet.cpp:64-102): GO_SPHERE_1/2 (193093/193094) apply
-- the persisted SpheresState — set (IN_PROGRESS) -> SetGoState(ACTIVE)
-- + SetFlag(NOT_SELECTABLE), clear -> RemoveFlag(NOT_SELECTABLE) only
-- (GoState untouched, C++-exact); GO_PRINCE_TALDARAM_PLATFORM
-- (193564) -> SetGoState(ACTIVE) when Taldaram is DONE (the C++
-- HandleGameObject(Empty, true, go) open). Static GOs never fire
-- OnGameObjectCreate in Go (dynamic-spawn-only,
-- instance_hooks.go:488-503), so the legs ride two live paths: the
-- event-6 handler (dynamic spawns, ruby-sanctum precedent) and an
-- OnPlayerEnter apply (the static-spawn path — the entering player
-- is the search anchor, gundrak precedent). GO resolution is
-- entry-based via player:FindNearestGameObject (1000y, DB + runtime
-- GOs; entries are unique per GO here).
-- PORTED — SetData / GetData sphere state (:104-135): the sphere
-- gossip (boss_prince_taldaram.lua) writes instanceData:SetData(
-- DATA_SPHERE_1/2, IN_PROGRESS), which fires OnSetData (event 8,
-- instance_hooks.go:51-63); the handler stores the value as an
-- instanceData number, re-applies it to the sphere GO, and saves.
-- The boss's own CheckSpheres keeps its per-creature store
-- (boss_prince_taldaram.lua) — the instance write is additive.
-- PORTED — WriteSaveDataMore / ReadSaveDataMore (:137-148): the two
-- sphere states persist as instanceData numbers through
-- Map:SaveInstanceData (scripting.MarshalInstanceData); no OnLoad
-- handler is needed — the blob is unmarshalled into instanceData
-- before event 2 fires (instance_hooks.go:414-463, gundrak
-- precedent) and the OnPlayerEnter apply reads it.
-- PORTED — boss DONE legs: the five boss files write
-- instanceData.<boss>Done = true in their ev4 handlers in the same
-- breath as creature:SetBossState(DATA_X, DONE) (BossAI _JustDied,
-- ScriptedCreature.cpp:513-520; gundrak precedent). The platform leg
-- reads princeTaldaramDone (no boss-state-change event exists).
-- PORTED — LoadObjectData guid latches (creatureData table, :30-37):
-- the instance-side OnCreatureCreate route is DEAD for these bosses
-- (static template spawns never fire event 5 — only
-- dynamically-materialized motions pass the funnel,
-- instance_hooks.go:553-562), so the latches ride the boss files'
-- ev5 (ON_SPAWN) handlers via creature:SetGuidData(DATA_X)
-- (instance_guid_store.go; the ICC precedent). The platform GO has
-- no SetGuidData bridge (creature-side only) — its consumers already
-- twin GetGameObject via FindNearestGameObject (taldaram
-- removePrison), so no latch is needed.
--
-- STAY-OUTS (triaged this tick):
-- doorData (GO_PRINCE_TALDARAM_GATE 192236 passage on
-- DATA_PRINCE_TALDARAM, :22-26) + LoadDoorData — Go has no door
-- model: setInstanceBossState explicitly skips door updates
-- (instance_boss_states.go:148-152), same as the ICC/Gundrak door
-- legs. CheckRequiredBosses — no bridge (creaturemotion.go:3756).
-- BossBoundaryData (Jedoga parallelogram, :48-51) — no boundary
-- bridge in the runtime. LoadObjectData / SetHeaders /
-- SetBossNumber — no bridges (ICC precedent). The ctor sphere-state
-- init (0, :57-58) is the nil-default in sphereState() below.
-- spell_combined_toxins (56584, :156-175) — STAYS OUT: the C++ script
-- is a DoCheckEffectProc-only AuraScript (gates the proc on the
-- triggering spell being a non-melee poison: Dispel == POISON and
-- DmgClass != MELEE). Go has no proc-check hook — RegisterAuraEvent
-- OnProc (event 6) mirrors OnEffectProc, the action handler that
-- fires AFTER the proc gates with a PreventDefaultAction veto
-- (runtime.go:478-496), and its args (event, aura, target, actor,
-- procTarget, damage) carry no triggering-spell info, so the
-- Dispel/DmgClass check cannot be evaluated. Creature-held proc
-- auras have no proc evaluation site in Go either (session-side
-- loop only). A handler would be dead or wrong code.

local MAP_AHNKAHET = 619

local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_SET_DATA = 8

-- ahnkahet.h AKDataTypes (DATA_PRINCE_TALDARAM = 1 is the boss id
-- the platform leg's DONE flag stands for; see the header).
local DATA_SPHERE_1 = 5
local DATA_SPHERE_2 = 6

-- ahnkahet.h AKGameObjectIds.
local GO_PRINCE_TALDARAM_PLATFORM = 193564
local GO_SPHERE_1 = 193093
local GO_SPHERE_2 = 193094

local GO_STATE_ACTIVE = 0
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

-- C++ SpheresState member (:146-147), as instanceData numbers
-- (marshal-safe, nil = 0, the ctor init).
local SPHERE_STATE_KEYS = {
    [DATA_SPHERE_1] = "sphere1State",
    [DATA_SPHERE_2] = "sphere2State",
}

local SPHERE_ENTRY_TO_DATA = {
    [GO_SPHERE_1] = DATA_SPHERE_1,
    [GO_SPHERE_2] = DATA_SPHERE_2,
}

-- GOs the create-time apply covers (platform + both spheres).
local CREATE_STATE_ENTRIES = {
    GO_PRINCE_TALDARAM_PLATFORM, GO_SPHERE_1, GO_SPHERE_2,
}

local function sphereState(instanceData, dataId)
    return instanceData[SPHERE_STATE_KEYS[dataId]] or 0
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

-- C++ OnGameObjectCreate create-time legs (:64-102), applied to one
-- GO. The platform leg reads the instanceData DONE flag (written
-- together with SetBossState(DONE) by boss_prince_taldaram.lua).
local function applyGOCreateState(instanceData, go)
    local entry = go:GetEntry()
    if entry == GO_PRINCE_TALDARAM_PLATFORM then
        if instanceData.princeTaldaramDone then
            go:SetGoState(GO_STATE_ACTIVE)
        end
        return
    end
    local dataId = SPHERE_ENTRY_TO_DATA[entry]
    if dataId then
        if sphereState(instanceData, dataId) ~= 0 then
            go:SetGoState(GO_STATE_ACTIVE)
            go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        else
            go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    end
end

-- C++ OnGameObjectCreate (instance_ahnkahet.cpp:64-102): the
-- create-time legs above. Fires only for dynamic spawns in Go; the
-- static-spawn path is the OnPlayerEnter apply below.
RegisterMapEvent(MAP_AHNKAHET, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    applyGOCreateState(instanceData, go)
end)

-- Static-spawn path for the create-time legs: apply every GO state
-- from the persisted sphere states / Taldaram DONE flag on player
-- enter (idempotent — it reads current state).
RegisterMapEvent(MAP_AHNKAHET, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    for _, entry in ipairs(CREATE_STATE_ENTRIES) do
        local go = findGOFromPlayer(player, entry)
        if go then
            applyGOCreateState(instanceData, go)
        end
    end
end)

-- C++ SetData (:104-116): sphere data writes land in SpheresState,
-- re-apply to the sphere GO, and persist. Fired by the sphere
-- gossip (boss_prince_taldaram.lua) via instanceData:SetData.
RegisterMapEvent(MAP_AHNKAHET, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    local key = SPHERE_STATE_KEYS[type]
    if key == nil then
        return
    end
    instanceData[key] = value
    local entry = nil
    if type == DATA_SPHERE_1 then
        entry = GO_SPHERE_1
    else
        entry = GO_SPHERE_2
    end
    local go = findGO(mapObj, entry)
    if go then
        applyGOCreateState(instanceData, go)
    end
    if mapObj then
        mapObj:SaveInstanceData()
    end
end)
