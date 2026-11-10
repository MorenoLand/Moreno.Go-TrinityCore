-- Arcatraz instance script — Lua port of
-- src/server/scripts/Outland/TempestKeep/arcatraz/
-- instance_arcatraz.cpp (instance_arcatraz, InstanceMapScript
-- "instance_arcatraz", map 552) and arcatraz.h (AZDataTypes,
-- AZCreatureIds, AZGameObjectIds, AZSpellIds).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_magtheridons_lair.lua / instance_shattered_halls.lua
-- precedent).
--
-- PORTED — door data (instance_arcatraz.cpp:26-31 +
-- InstanceScript::UpdateDoorState, InstanceScript.cpp:203-205):
-- GO_CONTAINMENT_CORE_SECURITY_FIELD_ALPHA (184318,
-- DOOR_TYPE_PASSAGE) opens iff DATA_SOCCOTHRATES is DONE;
-- GO_CONTAINMENT_CORE_SECURITY_FIELD_BETA (184319,
-- DOOR_TYPE_PASSAGE) opens iff DATA_DALLIAH is DONE.
-- boss_wrath_scryer_soccothrates.lua writes SetBossState(2, 3)
-- and boss_dalliah_the_doomsayer.lua writes SetBossState(1, 3)
-- on death; the OnUpdate poll reads them back via the resolved
-- boss creatures (creature:GetBossState,
-- lua_creature_events.go:6624) and latches instanceData, so the
-- doors stay open even once the boss corpse is unresolvable.
-- The OnUpdate poll (event 3) applies the door state, retrying
-- on later ticks if a door GO is unresolvable; the
-- OnGameObjectCreate handler (event 6) covers the dynamic-spawn
-- path and the OnPlayerEnter apply covers the static-spawn path
-- (static GOs never fire event 6 in Go — dynamic-spawn-only,
-- instance_hooks.go:488-503). Doors resolve entry-based via
-- player:FindNearestGameObject (lua_player.go:6710, 1000y —
-- the whole instance); the applied state is tracked in
-- instanceData so re-fires are no-ops.
-- PORTED — SetBossState-override DONE leg
-- (instance_arcatraz.cpp:167-186): on Skyriss DONE, heroic
-- only, if Millhouse is alive, DoCastSpellOnPlayers(
-- SPELL_QID_10886 = 39564). boss_harbinger_skyriss.lua writes
-- SetBossState(3, 3) on death; the poll latches it once and
-- casts 39564 on all players (player:CastSpell(player, spell,
-- true), the shattered castOnPlayers twin).
-- STAY-OUTS: SetHeaders ("AZ") / SetBossNumber (4) — no
-- bridges (ICC precedent). OnCreatureCreate GUID latches
-- (Dalliah/Soccothrates/Mellichar/Millhouse, :58-76) — static
-- spawns never fire event 5 (instance_hooks.go:556-571);
-- Mellichar's latch is done creature-side
-- (npc_mellichar.lua ev5 SetGuidData) and is the only live
-- GetGuidData consumer (Skyriss kill leg). OnGameObjectCreate
-- GO latches (stasis pods 183961-183965, Wardens Shield
-- 184802, :78-104) — static GOs never fire event 6; the only
-- consumers are Mellichar's phase machine (HandleGameObject,
-- no Lua method) and SmartAI warden scripts. SetData/
-- GetData(DATA_WARDEN_1..5, :106-145) — creature:GetData is
-- the per-creature ScriptData store (lua_creature_events.go:
-- 809), no instance GetData/SetData bridge; the warden states
-- have no live writer (Mellichar DoPrepareForPhase, blocked)
-- or reader, so the SetBossState NOT_STARTED/FAIL warden-reset
-- leg (:169-177) is a no-op in the ported model and stays out
-- with them. DATA_CONVERSATION (4, :115-116/:133-134) — sole
-- writer is Soccothrates's MoveInLOS 70y proximity arm
-- (boss_wrath_scryer_soccothrates.cpp:129-132, ev27
-- engage-only, no Z/arc/IsWithinDist replicas) and no C++
-- reader exists outside the instance GetData return. Save/
-- Load persistence — no bridge (Magtheridon precedent).
-- Millhouse's commented-out JustDied FailQuest leg
-- (arcatraz.cpp:137-140) is dead C++ and ports nothing.

local MAP_ARCATRAZ = 552

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- arcatraz.h AZDataTypes.
local DATA_DALLIAH = 1
local DATA_SOCCOTHRATES = 2
local DATA_HARBINGER_SKYRISS = 3

-- arcatraz.h AZCreatureIds.
local NPC_DALLIAH = 20885
local NPC_SOCCOTHRATES = 20886
local NPC_HARBINGER_SKYRISS = 20912
local NPC_MILLHOUSE = 20977

-- arcatraz.h AZGameObjectIds.
local GO_CONTAINMENT_CORE_SECURITY_FIELD_ALPHA = 184318
local GO_CONTAINMENT_CORE_SECURITY_FIELD_BETA = 184319

-- arcatraz.h AZSpellIds.
local SPELL_QID_10886 = 39564

local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1
local BOSS_STATE_DONE = 3

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO/creature

-- Entry-based resolution from any player in the instance
-- (player:FindNearestCreature / FindNearestGameObject cover DB
-- spawns; the instance_steam_vault.lua precedent).
local function findCreature(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local creature = player:FindNearestCreature(entry, GO_SEARCH_DIST)
        if creature then
            return creature
        end
    end
    return nil
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
        local go = player:FindNearestGameObject(entry, GO_SEARCH_DIST)
        if go then
            return go
        end
    end
    return nil
end

local function castOnPlayers(mapObj, spell)
    if mapObj == nil then
        return
    end
    local players = mapObj:GetPlayers()
    if not players then
        return
    end
    for _, player in ipairs(players) do
        player:CastSpell(player, spell, true)
    end
end

-- C++ InstanceScript::UpdateDoorState (InstanceScript.cpp:203-205):
-- PASSAGE opens iff its boss is DONE. Each door resolves and
-- applies independently, retrying on later ticks while
-- unresolvable; the applied state is tracked in instanceData so
-- re-fires are no-ops.
local function updateDoors(instanceData, mapObj)
    local alphaOpen = instanceData.soccothratesDone == true
    if instanceData.alphaDoorOpen ~= alphaOpen then
        local door = findGO(mapObj, GO_CONTAINMENT_CORE_SECURITY_FIELD_ALPHA)
        if door then
            local state = GO_STATE_READY
            if alphaOpen then
                state = GO_STATE_ACTIVE
            end
            door:SetGoState(state)
            instanceData.alphaDoorOpen = alphaOpen
        end
    end
    local betaOpen = instanceData.dalliahDone == true
    if instanceData.betaDoorOpen ~= betaOpen then
        local door = findGO(mapObj, GO_CONTAINMENT_CORE_SECURITY_FIELD_BETA)
        if door then
            local state = GO_STATE_READY
            if betaOpen then
                state = GO_STATE_ACTIVE
            end
            door:SetGoState(state)
            instanceData.betaDoorOpen = betaOpen
        end
    end
end

-- C++ OnGameObjectCreate (instance_arcatraz.cpp:78-80 via
-- InstanceScript::OnGameObjectCreate): a door creation
-- re-evaluates its state. Fires only for dynamic spawns in Go;
-- the static-spawn path is the OnPlayerEnter apply below.
RegisterMapEvent(MAP_ARCATRAZ, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    if entry == GO_CONTAINMENT_CORE_SECURITY_FIELD_ALPHA or entry == GO_CONTAINMENT_CORE_SECURITY_FIELD_BETA then
        updateDoors(instanceData, mapObj)
    end
end)

-- C++ OnPlayerEnter: apply the door state for the static-spawn
-- path (static GOs never fire event 6 in Go).
RegisterMapEvent(MAP_ARCATRAZ, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    updateDoors(instanceData, mapObj)
end)

-- C++ Update (instance_arcatraz.cpp) + the SetBossState-override
-- DONE leg (:167-186), all driven by the OnUpdate poll
-- (event 3, diff in ms): latch Soccothrates/Dalliah DONE and
-- apply the doors; latch Skyriss DONE once and, heroic only,
-- cast the quest spell on all players if Millhouse is alive.
RegisterMapEvent(MAP_ARCATRAZ, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    updateDoors(instanceData, mapObj)
    -- Latch Soccothrates DONE from the boss-state store
    -- (boss_wrath_scryer_soccothrates.lua writes SetBossState(2, 3)
    -- on death).
    if not instanceData.soccothratesDone then
        local soccothrates = findCreature(mapObj, NPC_SOCCOTHRATES)
        if soccothrates and soccothrates:GetBossState(DATA_SOCCOTHRATES) == BOSS_STATE_DONE then
            instanceData.soccothratesDone = true
            updateDoors(instanceData, mapObj)
        end
    end
    -- Latch Dalliah DONE from the boss-state store
    -- (boss_dalliah_the_doomsayer.lua writes SetBossState(1, 3)
    -- on death).
    if not instanceData.dalliahDone then
        local dalliah = findCreature(mapObj, NPC_DALLIAH)
        if dalliah and dalliah:GetBossState(DATA_DALLIAH) == BOSS_STATE_DONE then
            instanceData.dalliahDone = true
            updateDoors(instanceData, mapObj)
        end
    end
    -- Latch Skyriss DONE from the boss-state store
    -- (boss_harbinger_skyriss.lua writes SetBossState(3, 3) on
    -- death): heroic only, Millhouse alive -> quest spell 39564
    -- on all players, once.
    if not instanceData.skyrissQuestCast then
        local skyriss = findCreature(mapObj, NPC_HARBINGER_SKYRISS)
        if skyriss and skyriss:GetBossState(DATA_HARBINGER_SKYRISS) == BOSS_STATE_DONE then
            instanceData.skyrissQuestCast = true
            if mapObj:IsHeroic() then
                local millhouse = findCreature(mapObj, NPC_MILLHOUSE)
                if millhouse and millhouse:IsAlive() then
                    castOnPlayers(mapObj, SPELL_QID_10886)
                end
            end
        end
    end
end)
