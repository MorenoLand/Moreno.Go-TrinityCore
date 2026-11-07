-- The Oculus instance script — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/instance_oculus.cpp
-- (class instance_oculus : public InstanceMapScript, map 578).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 578
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- drakos / varos / urom / eregos / belgaristrasz / eternos /
-- verdisa — the creature objects bound by ON_CREATURE_CREATE (the
-- C++ OnCreatureCreate GUID latches); constructCount — the
-- CentrifugueConstructCounter (instance_oculus.cpp:61), init 0,
-- the C++ member default; greaterWhelps — the GreaterWhelpList
-- (instance_oculus.cpp:131-135, GUIDs in C++, live objects here);
-- <boss>Done flags written by the boss onDied handlers (maexxna
-- precedent) — drakosDone, varosDone, uromDone, eregosDone;
-- <boss>Processed flags owned by the ON_UPDATE poll below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: boss/dragon/whelp binds + the DONE-gated
--   legs (phase-mask unhide for varos/urom/eregos/greater whelps,
--   gossip flag + Teleport-to-move-out for the three dragons when
--   Drakos is DONE — Teleport is the Go twin of C++ Relocate,
--   lua_creature_events.go:3044; the construct counter ++ (alive
--   only) + UpdateWorldState(AMOUNT, count), the
--   creature:UpdateWorldState bridge (lua_creature_events.go:5888).
--   Fires on materialization (motionFor funnel,
--   instance_hooks.go:320); a creature that never materializes is
--   never bound (naxxramas LoadGrid delta).
-- - SetBossState DONE legs, re-expressed at DONE time via the
--   ON_UPDATE poll (naxxramas precedent): DRAKOS DONE —
--   UpdateWorldState(SHOW, 1) + (AMOUNT, count), FreeDragons
--   (SetWalk + MoveTo to the C++ move-out positions, arrival poll
--   below), varos phase-mask unhide, Varos intro Talk 4 at 15s;
--   VAROS DONE — UpdateWorldState(SHOW, 0), urom phase-mask
--   unhide; UROM DONE — eregos phase-mask unhide, GreaterWhelps
--   phase-mask loop, Eregos intro Talk 0 at 5s; EREGOS DONE —
--   cache RemoveFlag(NOT_SELECTABLE) via FindNearestGameObject
--   (maexxna precedent; N or H entry, whichever resolves).
-- - FillInitialWorldStates, re-expressed on player enter: the
--   SHOW/AMOUNT pair per the C++ condition (Drakos DONE and
--   Varos not DONE), sent via a bound creature, guarded on one
--   being bound.
-- - GetData(DATA_CONSTRUCTS): no function port — the drake GPS
--   leg (oculus.cpp) reads instanceData.constructCount directly
--   (0 -> KILL_NO_CONSTRUCT, 1 -> KILL_ONE_CONSTRUCT, else
--   KILL_MORE_CONSTRUCT; oculus.h:92-96).
-- - OnUnitDeath construct leg: ported in
--   npc_centrifuge_construct.lua (ev4 JustDied) — counter -- +
--   UpdateWorldState(AMOUNT) + Varos RemoveAllAuras at 0, the
--   tick-251 open item (no Lua instance surface then; the ev4
--   handler is the surface now).
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_DRAGON_CAGE_DOOR 193995): no boss-state door
--   machine in Go (naxxramas precedent).
-- - ProcessEvent EVENT_CALL_DRAGON (12229): no instance
--   ProcessEvent bridge; the sender is Varos spell 51002
--   (SPELL_CALL_AZURE_RING_CAPTAIN, boss_varos.cpp:39), an
--   unported SpellScript effect, so the receiver (Varos summons
--   NPC_AZURE_RING_GUARDIAN 28236 at +40z + DoAction(1)) is
--   latent with it.
-- - Eregos cache SetRespawnTime(GetRespawnDelay()): no GO
--   GetRespawnDelay bridge (naxxramas horsemen-chest precedent);
--   only the RemoveFlag leg ports.
-- - OnGameObjectCreate cache latch in its C++ create-time form:
--   static template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:306, naxxramas precedent), so the latch
--   is vacuous here and the DONE-time FindNearestGameObject form
--   is the live one.
-- - SaveToDB / instance-data persistence across restarts: the
--   table lives for the runtime lifetime only (black-morass delta).

local MAP_OCULUS = 578
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local DATA_DRAKOS = 0
local DATA_VAROS = 1
local DATA_UROM = 2
local DATA_EREGOS = 3

local NPC_DRAKOS = 27654 -- oculus.h:37
local NPC_VAROS = 27447 -- oculus.h:38
local NPC_UROM = 27655 -- oculus.h:39
local NPC_EREGOS = 27656 -- oculus.h:40
local NPC_CENTRIFUGE_CONSTRUCT = 27641 -- oculus.h:43
local NPC_VERDISA = 27657 -- oculus.h:45
local NPC_BELGARISTRASZ = 27658 -- oculus.h:46
local NPC_ETERNOS = 27659 -- oculus.h:47
local NPC_GREATER_WHELP = 28276 -- oculus.h:48

local GO_EREGOS_CACHE_N = 191349 -- oculus.h:53
local GO_EREGOS_CACHE_H = 193603 -- oculus.h:54

local WORLD_STATE_CENTRIFUGE_CONSTRUCT_SHOW = 3524 -- oculus.h:79
local WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT = 3486 -- oculus.h:80

local SAY_EREGOS_INTRO_TEXT = 0 -- oculus.h:87
local SAY_VAROS_INTRO_TEXT = 4 -- oculus.h:88
local SAY_BELGARISTRASZ = 0 -- oculus.cpp:118 (Says enum)

local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (baltharus precedent)
local UNIT_NPC_FLAG_GOSSIP = 0x00000001 -- UnitDefines.h:186
local GAMEOBJECT_FLAGS = 9 -- boss_maexxna.lua precedent
local GO_FLAG_NOT_SELECTABLE = 16 -- boss_maexxna.lua precedent

local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local POINT_MOVE_OUT = 1 -- oculus.h:101

-- C++ move-out positions (instance_oculus.cpp:36-38).
local DRAGON_MOVES = {
    [NPC_BELGARISTRASZ] = { 941.453, 1044.1, 359.967, 0.1984709 },
    [NPC_ETERNOS] = { 943.202, 1059.35, 359.967, 5.757278 },
    [NPC_VERDISA] = { 949.188, 1032.91, 359.967, 1.093027 },
}

local creatureBinds = {
    [NPC_DRAKOS] = "drakos",
    [NPC_VAROS] = "varos",
    [NPC_UROM] = "urom",
    [NPC_EREGOS] = "eregos",
    [NPC_BELGARISTRASZ] = "belgaristrasz",
    [NPC_ETERNOS] = "eternos",
    [NPC_VERDISA] = "verdisa",
}

-- First bound creature wins: the UpdateWorldState caller for a
-- leg (any creature in the instance reaches every player in it,
-- lua_creature_events.go:5888).
local function worldStateCaller(data)
    return data.varos or data.urom or data.eregos or data.drakos
end

local function updateWorldState(data, field, value)
    local caller = worldStateCaller(data)
    if caller then
        caller:UpdateWorldState(field, value)
    end
end

-- C++ MovementInform POINT_MOVE_OUT (oculus.cpp:193-205, event 28):
-- on arrival, Belgaristrasz Talks (SAY_BELGARISTRASZ 0) and the
-- gossip flag lands on all three heralds.
local POINT_MOTION_TYPE = 8

local function dragonMoveOutInform(_, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE or id ~= POINT_MOVE_OUT then
        return
    end
    if creature:GetEntry() == NPC_BELGARISTRASZ then
        creature:Talk(SAY_BELGARISTRASZ)
    end
    creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
end

for _, entry in ipairs({ NPC_VERDISA, NPC_BELGARISTRASZ, NPC_ETERNOS }) do
    RegisterCreatureEvent(entry, 28, dragonMoveOutInform)
end

-- C++ FreeDragons (instance_oculus.cpp:278-297): SetWalk(true) +
-- MovePoint(POINT_MOVE_OUT, move) per dragon; MoveTo is the Go
-- MovePoint twin (lua_creature_events.go:4499; the id is accepted
-- but unrecorded, arrival is polled above).
local function freeDragons(data)
    for entry, move in pairs(DRAGON_MOVES) do
        local key = creatureBinds[entry]
        local dragon = data[key]
        if dragon then
            dragon:SetWalk(true)
            dragon:MoveTo(POINT_MOVE_OUT, move[1], move[2], move[3])
        end
    end
end

local function greaterWhelps(data)
    if not data.greaterWhelps then
        return
    end
    for _, whelp in ipairs(data.greaterWhelps) do
        if whelp then
            whelp:SetPhaseMask(1)
        end
    end
end

RegisterMapEvent(MAP_OCULUS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    local key = creatureBinds[entry]
    if key then
        instanceData[key] = creature
    end
    if entry == NPC_VAROS then
        if creature:GetBossState(DATA_DRAKOS) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1)
        end
    elseif entry == NPC_UROM then
        if creature:GetBossState(DATA_VAROS) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1)
        end
    elseif entry == NPC_EREGOS then
        if creature:GetBossState(DATA_UROM) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1)
        end
    elseif entry == NPC_BELGARISTRASZ or entry == NPC_ETERNOS or entry == NPC_VERDISA then
        if creature:GetBossState(DATA_DRAKOS) == BOSS_STATE_DONE then
            creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
            local move = DRAGON_MOVES[entry]
            creature:Teleport(move[1], move[2], move[3], move[4])
        end
    elseif entry == NPC_CENTRIFUGE_CONSTRUCT then
        if creature:IsAlive() then
            local count = (instanceData.constructCount or 0) + 1
            instanceData.constructCount = count
            creature:UpdateWorldState(WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT, count)
        end
    elseif entry == NPC_GREATER_WHELP then
        if creature:GetBossState(DATA_UROM) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1)
            if not instanceData.greaterWhelps then
                instanceData.greaterWhelps = {}
            end
            table.insert(instanceData.greaterWhelps, creature)
        end
    end
end)

RegisterMapEvent(MAP_OCULUS, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if not instanceData.drakosDone then
        return
    end
    -- C++ FillInitialWorldStates (instance_oculus.cpp:143-155):
    -- SHOW 1 + AMOUNT count while Drakos is DONE and Varos is not.
    if not instanceData.varosDone then
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_SHOW, 1)
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT, instanceData.constructCount or 0)
    else
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_SHOW, 0)
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT, 0)
    end
end)

RegisterMapEvent(MAP_OCULUS, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.drakosDone and not instanceData.drakosProcessed then
        instanceData.drakosProcessed = true
        -- C++ SetBossState DATA_DRAKOS DONE (instance_oculus.cpp:184-195).
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_SHOW, 1)
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT, instanceData.constructCount or 0)
        freeDragons(instanceData)
        if instanceData.varos then
            instanceData.varos:SetPhaseMask(1)
        end
        CreateLuaEvent(function()
            if instanceData.varos then
                instanceData.varos:Talk(SAY_VAROS_INTRO_TEXT)
            end
        end, 15000, 1)
    end
    if instanceData.varosDone and not instanceData.varosProcessed then
        instanceData.varosProcessed = true
        -- C++ SetBossState DATA_VAROS DONE (instance_oculus.cpp:197-204).
        updateWorldState(instanceData, WORLD_STATE_CENTRIFUGE_CONSTRUCT_SHOW, 0)
        if instanceData.urom then
            instanceData.urom:SetPhaseMask(1)
        end
    end
    if instanceData.uromDone and not instanceData.uromProcessed then
        instanceData.uromProcessed = true
        -- C++ SetBossState DATA_UROM DONE (instance_oculus.cpp:206-215).
        if instanceData.eregos then
            instanceData.eregos:SetPhaseMask(1)
            greaterWhelps(instanceData)
        end
        CreateLuaEvent(function()
            if instanceData.eregos then
                instanceData.eregos:Talk(SAY_EREGOS_INTRO_TEXT)
            end
        end, 5000, 1)
    end
    if instanceData.eregosDone and not instanceData.eregosProcessed then
        instanceData.eregosProcessed = true
        -- C++ SetBossState DATA_EREGOS DONE (instance_oculus.cpp:217-227):
        -- SetRespawnTime(GetRespawnDelay()) stays out (no bridge);
        -- RemoveFlag(NOT_SELECTABLE) ports.
        if instanceData.eregos then
            local cache = instanceData.eregos:FindNearestGameObject(GO_EREGOS_CACHE_N, 1000)
            if not cache then
                cache = instanceData.eregos:FindNearestGameObject(GO_EREGOS_CACHE_H, 1000)
            end
            if cache then
                cache:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            end
        end
    end
end)
