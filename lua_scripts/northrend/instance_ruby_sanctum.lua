-- The Ruby Sanctum instance script — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/instance_ruby_sanctum.cpp
-- (class instance_ruby_sanctum : public InstanceMapScript, map 724 —
-- instance_ruby_sanctum.cpp:78; AddSC_instance_ruby_sanctum :218).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_icecrown_citadel.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 724
-- (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- baltharus / baltharusClone / crystalChannelTarget /
-- generalZarithrian / savianaRagefire / halion / twilightHalion /
-- halionController / orbCarrier / orbRotationFocus / xerestrasza —
-- the creature objects bound by ON_CREATURE_CREATE (the C++
-- creatureData GUID latches, instance_ruby_sanctum.cpp:47-62);
-- flameWalls / burningTree1-4 / flameRing / twilightFlameRing —
-- the GO objects bound by ON_GAMEOBJECT_CREATE (the C++
-- gameObjectData latches, :63-76; dynamic spawns only, see below);
-- zarithrianSpawnStalkers — the two spawn-stalker objects in
-- spawn order (C++ ZarithrianSpawnStalkerGUID[2], :102-113).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above + the Zarithrian spawn
--   stalker latch (:102-113 — first stalker to [1], second to
--   [2]; a third overwrites [2], exactly as the C++ else arm
--   does). Fires on materialization (motionFor funnel,
--   instance_hooks.go:311-320); a static template spawn never
--   materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - OnGameObjectCreate: the GO binds above. Static template GOs
--   never pass the Go GO-create funnel (instance_hooks.go:291-306,
--   oculus precedent), so the static-form binds are vacuous here;
--   the handler is live for dynamically-spawned GOs.
-- - OnPlayerEnter (:92-100): Halion not latched, Halion not DONE,
--   Zarithrian DONE -> the Halion controller gets
--   DoAction(ACTION_INTRO_HALION_2). Boss state is read via a
--   bound creature (creature:GetBossState,
--   lua_creature_events.go:5932 — the map object carries no
--   GetBossState); the Halion latch is instanceData.halion.
--   instance->LoadGrid(HalionControllerSpawnPos) has no bridge,
--   so the DoAction fires only if the controller already
--   materialized and bound. The DoAction bridge itself is live
--   (ev38, lua_creature_events.go:814), but the receiver
--   (NPC_HALION_CONTROLLER 40146) is currently unregistered
--   (boss_halion.lua tick-222 triage), so the call dispatches to
--   zero handlers today — ported C++-exact so it lands if the
--   controller is registered later.
-- - FillInitialWorldStates (:200-206), re-expressed on player
--   enter (oculus precedent): corporeality MATERIAL 50 /
--   TWILIGHT 50 / TOGGLE 0 via a bound creature
--   (creature:UpdateWorldState, lua_creature_events.go:5893);
--   Halion DONE sends 0/0/0 instead, matching the end state of
--   the SetBossState HALION leg (:163-170, no fire site — see
--   below). No-op when nothing is bound yet.
-- - GetGuidData (:133-143): no function port — the Lua mirror
--   is the instanceData binds above, read as instanceData.<key>;
--   the stalker GUIDs are instanceData.zarithrianSpawnStalkers[1]
--   / [2] (DATA_ZARITHRIAN_SPAWN_STALKER_1/2 = 8/9). The boss
--   files read the same table (the oculus precedent).
-- Unmodeled (documented-only, no bridges):
-- - SetBossState override (:145-172): no fire site in Go. Map
--   events are 1-7 (engine/scripting/runtime.go:172-178) and
--   none fires on a boss-state change; creature:SetBossState
--   (lua_creature_events.go:5917) only writes the store
--   (engine/world/instance_boss_states.go:85) and never invokes
--   override legs. No ON_UPDATE poll in this file (the
--   instance_icecrown_citadel.lua pattern), so the legs cannot
--   be re-expressed at DONE time here either:
--   - BALTHARUS/SAVIANA DONE -> CheckGeneral (:174-182):
--     zarithrian RemoveFlag(UNIT_FIELD_FLAGS 59,
--     UNIT_FLAG_NOT_SELECTABLE) is bridged
--     (lua_creature_events.go:5445, field 59) but
--     SetImmuneToPC(false) has NO Lua bridge (tree-wide grep:
--     no SetImmuneToPC method on the creature event surface),
--     and there is no DONE-time fire site. The zarithrian boss
--     file likewise documents its Reset/CanAIAttack gating legs
--     as riding the instance-script model with no bridge
--     (boss_general_zarithrian.lua).
--   - ZARITHRIAN DONE -> controller
--     DoAction(ACTION_INTRO_HALION) (:158-162): the DoAction
--     bridge is live but there is no fire site, and the receiver
--     is unregistered (see above).
--   - HALION any state -> DoUpdateWorldState x3 to 0 (:163-170):
--     the UpdateWorldState bridge is live but there is no fire
--     site; the enter-time values are covered by the
--     FillInitialWorldStates re-expression above.
-- - OnGameObjectCreate burning-trees leg (:115-131):
--   HandleGameObject(ObjectGuid::Empty, true, go) when Zarithrian
--   is DONE has no bridge (tree-wide grep: no HandleGameObject
--   in the Go engine); static GOs never pass the funnel anyway.
-- - Door data (:36-45) + boss boundaries (:30-34): no
--   boss-state door machine / boundary model in Go (naxxramas /
--   oculus precedent).
-- - SetHeaders(DataHeader "RS") / SetBossNumber(EncounterCount 4)
--   / LoadObjectData / LoadDoorData / LoadBossBoundaries: no
--   bridges; the boss-state store is written via
--   creature:SetBossState and read via creature:GetBossState.
-- - SetData/GetData DATA_BALTHARUS_SHARED_HEALTH (:184-198): no
--   instance SetData bridge — creature:SetData is a generic
--   per-motion store, NOT virtual AI dispatch
--   (lua_creature_events.go:767; boss_halion triage). The
--   baltharus file documents its shared-health write as riding
--   this (boss_baltharus_the_warborn.lua:67).
-- - ACTION_INTRO_BALTHARUS / ACTION_BALTHARUS_DEATH
--   (ruby_sanctum.h:58-59): issued by at_baltharus_plateau /
--   xerestrasza, not by this instance script — not instance legs.
-- - SaveToDB / instance-data persistence across restarts: the
--   table lives for the runtime lifetime only (black-morass delta).
-- DEPENDENCY (resolved tick 270): the four RS boss files now call
-- creature:SetBossState on engage/died/reset/evade, so GetBossState
-- reads here return real states; no native engine path sets boss
-- state on death — only creature:SetBossState and the GM command
-- write the store. The DONE-gated legs above are live on that.
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_RUBY_SANCTUM = 724
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

local DATA_BALTHARUS_THE_WARBORN = 0 -- ruby_sanctum.h:31
local DATA_GENERAL_ZARITHRIAN = 1 -- ruby_sanctum.h:32
local DATA_SAVIANA_RAGEFIRE = 2 -- ruby_sanctum.h:33
local DATA_HALION = 3 -- ruby_sanctum.h:34
local DATA_HALION_CONTROLLER = 10 -- ruby_sanctum.h:43

local NPC_BALTHARUS_THE_WARBORN = 39751 -- ruby_sanctum.h:67
local NPC_BALTHARUS_THE_WARBORN_CLONE = 39899 -- ruby_sanctum.h:68
local NPC_BALTHARUS_TARGET = 26712 -- ruby_sanctum.h:69
local NPC_GENERAL_ZARITHRIAN = 39746 -- ruby_sanctum.h:72
local NPC_ZARITHRIAN_SPAWN_STALKER = 39794 -- ruby_sanctum.h:74
local NPC_SAVIANA_RAGEFIRE = 39747 -- ruby_sanctum.h:77
local NPC_HALION = 39863 -- ruby_sanctum.h:80
local NPC_TWILIGHT_HALION = 40142 -- ruby_sanctum.h:81
local NPC_HALION_CONTROLLER = 40146 -- ruby_sanctum.h:82
local NPC_ORB_CARRIER = 40081 -- ruby_sanctum.h:85
local NPC_ORB_ROTATION_FOCUS = 40091 -- ruby_sanctum.h:86
local NPC_XERESTRASZA = 40429 -- ruby_sanctum.h:102

local GO_FLAME_WALLS = 203006 -- ruby_sanctum.h:111
local GO_FLAME_RING = 203007 -- ruby_sanctum.h:112
local GO_TWILIGHT_FLAME_RING = 203624 -- ruby_sanctum.h:113
local GO_BURNING_TREE_1 = 203034 -- ruby_sanctum.h:114
local GO_BURNING_TREE_2 = 203035 -- ruby_sanctum.h:115
local GO_BURNING_TREE_3 = 203036 -- ruby_sanctum.h:116
local GO_BURNING_TREE_4 = 203037 -- ruby_sanctum.h:117

local WORLDSTATE_CORPOREALITY_MATERIAL = 5049 -- ruby_sanctum.h:122
local WORLDSTATE_CORPOREALITY_TWILIGHT = 5050 -- ruby_sanctum.h:123
local WORLDSTATE_CORPOREALITY_TOGGLE = 5051 -- ruby_sanctum.h:124

local ACTION_INTRO_HALION_2 = -4014602 -- ruby_sanctum.h:61

local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local creatureBinds = {
    [NPC_BALTHARUS_THE_WARBORN] = "baltharus",
    [NPC_BALTHARUS_THE_WARBORN_CLONE] = "baltharusClone",
    [NPC_BALTHARUS_TARGET] = "crystalChannelTarget",
    [NPC_GENERAL_ZARITHRIAN] = "generalZarithrian",
    [NPC_SAVIANA_RAGEFIRE] = "savianaRagefire",
    [NPC_HALION] = "halion",
    [NPC_TWILIGHT_HALION] = "twilightHalion",
    [NPC_HALION_CONTROLLER] = "halionController",
    [NPC_ORB_CARRIER] = "orbCarrier",
    [NPC_ORB_ROTATION_FOCUS] = "orbRotationFocus",
    [NPC_XERESTRASZA] = "xerestrasza",
}

local gameObjectBinds = {
    [GO_FLAME_WALLS] = "flameWalls",
    [GO_BURNING_TREE_1] = "burningTree1",
    [GO_BURNING_TREE_2] = "burningTree2",
    [GO_BURNING_TREE_3] = "burningTree3",
    [GO_BURNING_TREE_4] = "burningTree4",
    [GO_FLAME_RING] = "flameRing",
    [GO_TWILIGHT_FLAME_RING] = "twilightFlameRing",
}

-- First bound creature wins: the GetBossState / UpdateWorldState
-- caller for a leg (any creature in the instance reaches the
-- instance-scoped store / every player in it,
-- lua_creature_events.go:5893/5932; the oculus worldStateCaller
-- precedent).
local function anyCreature(data)
    return data.halionController or data.generalZarithrian or
        data.halion or data.baltharus or data.savianaRagefire or
        data.twilightHalion or data.xerestrasza or data.orbCarrier or
        data.orbRotationFocus or data.baltharusClone or
        data.crystalChannelTarget
end

RegisterMapEvent(MAP_RUBY_SANCTUM, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    -- C++ :102-113: the spawn stalker latch — first stalker to
    -- [1], second to [2]; a third overwrites [2], exactly as the
    -- C++ else arm does.
    if entry == NPC_ZARITHRIAN_SPAWN_STALKER then
        if not instanceData.zarithrianSpawnStalkers then
            instanceData.zarithrianSpawnStalkers = {}
        end
        if not instanceData.zarithrianSpawnStalkers[1] then
            instanceData.zarithrianSpawnStalkers[1] = creature
        else
            instanceData.zarithrianSpawnStalkers[2] = creature
        end
        return
    end
    local key = creatureBinds[entry]
    if not key then
        return
    end
    instanceData[key] = creature
end)

RegisterMapEvent(MAP_RUBY_SANCTUM, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    local key = gameObjectBinds[entry]
    if not key then
        return
    end
    instanceData[key] = go
    -- C++ :115-131: the burning-tree DONE-gated HandleGameObject
    -- leg has no bridge (see header); the bind above is the live
    -- part, for future consumers.
end)

RegisterMapEvent(MAP_RUBY_SANCTUM, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    local caller = anyCreature(instanceData)
    if not caller then
        return
    end
    -- C++ OnPlayerEnter (instance_ruby_sanctum.cpp:92-100): Halion
    -- not latched, Halion not DONE, Zarithrian DONE -> the Halion
    -- controller gets DoAction(ACTION_INTRO_HALION_2).
    -- instance->LoadGrid(HalionControllerSpawnPos, :96) has no
    -- bridge: the DoAction fires only if the controller already
    -- materialized and bound. The receiver (40146) is currently
    -- unregistered, so the call dispatches to zero handlers today
    -- (see header).
    if not instanceData.halion and caller:GetBossState(DATA_HALION) ~= BOSS_STATE_DONE and
            caller:GetBossState(DATA_GENERAL_ZARITHRIAN) == BOSS_STATE_DONE then
        if instanceData.halionController then
            instanceData.halionController:DoAction(ACTION_INTRO_HALION_2)
        end
    end
    -- C++ FillInitialWorldStates (:200-206), re-expressed on player
    -- enter (oculus precedent): corporeality MATERIAL 50 / TWILIGHT
    -- 50 / TOGGLE 0; Halion DONE zeroes all three (the SetBossState
    -- HALION leg, :163-170, has no fire site — see header).
    local corporeality = 50
    if caller:GetBossState(DATA_HALION) == BOSS_STATE_DONE then
        corporeality = 0
    end
    caller:UpdateWorldState(WORLDSTATE_CORPOREALITY_MATERIAL, corporeality)
    caller:UpdateWorldState(WORLDSTATE_CORPOREALITY_TWILIGHT, corporeality)
    caller:UpdateWorldState(WORLDSTATE_CORPOREALITY_TOGGLE, 0)
end)
