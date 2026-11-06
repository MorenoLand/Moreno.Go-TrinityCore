-- The Eye of Eternity instance script — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/instance_eye_of_eternity.cpp
-- (class instance_eye_of_eternity : public InstanceMapScript, map 616).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_oculus.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 616
-- (engine/world/instance_hooks.go) that the Malygos onDied
-- handler shares with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- malygos / alexstraszaBunny / giftBoxBunny — the creature
-- objects bound by ON_CREATURE_CREATE (the C++ OnCreatureCreate
-- GUID latches); vortexTriggers / portalTriggers — the trigger
-- lists (GUIDs in C++, live objects here); platform / iris /
-- exitPortal / heartOfMagic — the GO objects bound by
-- ON_GAMEOBJECT_CREATE (dynamic spawns only, see below);
-- exitPortalPosition / focusingIrisPosition — the latched GO
-- positions; malygosDone — written by the Malygos onDied
-- handler (maexxna precedent); malygosProcessed — owned by the
-- ON_UPDATE poll below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: vortex/portal trigger lists + malygos /
--   alexstrasza-bunny / gift-box-bunny binds (the C++ GUID
--   latches; no live consumer reads the trigger lists yet —
--   VortexHandling / PowerSparksHandling stay out, see below —
--   they mirror the C++ latch set, naxxramas precedent).
-- - OnGameObjectCreate: platform / iris / exit-portal /
--   heart-of-magic binds + iris/exit-portal positions. Static
--   template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:306, naxxramas precedent), so the
--   static-form binds are vacuous here; the handler is live
--   for the dynamically-spawned iris (DATA_RESPAWN_IRIS) and
--   exit portal (DONE spawn below).
-- - OnPlayerEnter: Malygos DONE -> the player casts
--   SPELL_SUMMOM_RED_DRAGON_BUDDY (56070) on self
--   (instance_eye_of_eternity.cpp:51-55). The C++ triggered
--   flag has no player-side arg (player:CastSpell takes
--   target + spell id, lua_player.go:2752); the cast rides
--   castSpellDirect.
-- - SetBossState DONE leg, re-expressed at DONE time via the
--   ON_UPDATE poll (naxxramas precedent): SpawnGameObject
--   GO_EXIT_PORTAL (193908) at the exit-portal position — the
--   position comes from the existing static portal found by
--   FindNearestGameObject from Malygos (the static GO-create
--   latch never fires in Go, so the latched position is the
--   fallback, not the source).
-- - GetGuidData: no function port — the live consumer is
--   boss_malygos.lua's JustDied heart-of-magic leg, which
--   reads instanceData.giftBoxBunny directly.
-- Unmodeled (documented-only, no bridges):
-- - SetBossState FAIL leg (portal-trigger RemoveAllAuras +
--   AI Reset, exit-portal spawn, platform RemoveFlag
--   DESTROYED): FAIL is unobservable in Go — the boss files
--   port _Reset (NOT_STARTED) on ev2/ev23, never FAIL
--   (naxxramas precedent: Go observes only DONE via onDied).
-- - ProcessEvent EVENT_FOCUSING_IRIS (20711): no instance
--   ProcessEvent bridge (oculus precedent); the receiver
--   (Malygos DoAction(0), ACTION_LAND_ENCOUNTER_START) sits
--   in the unported DoAction machine in boss_malygos.lua.
-- - SetData dispatch (DATA_VORTEX_HANDLING /
--   DATA_POWER_SPARKS_HANDLING / DATA_RESPAWN_IRIS): no
--   instance SetData bridge, so VortexHandling (needs the
--   threat list — no bridge) and PowerSparksHandling never
--   start; the iris respawn never fires.
-- - OnUnitDeath player root (SetControlled ROOT on player
--   death, the EoE falling fix): no player-death hook —
--   PlayerEventKilledByCreature (8) fires only for
--   creature kills, never for falls.
-- - Boss boundaries (CircleBoundary 280yd): no boundary model
--   in Go (naxxramas precedent).
-- - SaveToDB / instance-data persistence across restarts: the
--   table lives for the runtime lifetime only (black-morass delta).

local MAP_EYE_OF_ETERNITY = 616
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

local DATA_MALYGOS_EVENT = 0 -- eye_of_eternity.h:28

local NPC_MALYGOS = 28859 -- eye_of_eternity.h:49
local NPC_VORTEX_TRIGGER = 30090 -- eye_of_eternity.h:50
local NPC_PORTAL_TRIGGER = 30118 -- eye_of_eternity.h:51
local NPC_ALEXSTRASZA_BUNNY = 31253 -- eye_of_eternity.h:58
local NPC_ALEXSTRASZAS_GIFT = 32448 -- eye_of_eternity.h:59

local GO_NEXUS_RAID_PLATFORM = 193070 -- eye_of_eternity.h:64
local GO_EXIT_PORTAL = 193908 -- eye_of_eternity.h:65
local GO_FOCUSING_IRIS_10 = 193958 -- eye_of_eternity.h:66
local GO_FOCUSING_IRIS_25 = 193960 -- eye_of_eternity.h:67
local GO_HEART_OF_MAGIC_10 = 194158 -- eye_of_eternity.h:70
local GO_HEART_OF_MAGIC_25 = 194159 -- eye_of_eternity.h:71

local SPELL_SUMMOM_RED_DRAGON_BUDDY = 56070 -- eye_of_eternity.h:86

local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local creatureBinds = {
    [NPC_MALYGOS] = "malygos",
    [NPC_ALEXSTRASZA_BUNNY] = "alexstraszaBunny",
    [NPC_ALEXSTRASZAS_GIFT] = "giftBoxBunny",
}

local gameObjectBinds = {
    [GO_NEXUS_RAID_PLATFORM] = "platform",
    [GO_FOCUSING_IRIS_10] = "iris",
    [GO_FOCUSING_IRIS_25] = "iris",
    [GO_EXIT_PORTAL] = "exitPortal",
    [GO_HEART_OF_MAGIC_10] = "heartOfMagic",
    [GO_HEART_OF_MAGIC_25] = "heartOfMagic",
}

RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    local key = creatureBinds[entry]
    if key then
        instanceData[key] = creature
    end
    if entry == NPC_VORTEX_TRIGGER then
        if not instanceData.vortexTriggers then
            instanceData.vortexTriggers = {}
        end
        table.insert(instanceData.vortexTriggers, creature)
    elseif entry == NPC_PORTAL_TRIGGER then
        if not instanceData.portalTriggers then
            instanceData.portalTriggers = {}
        end
        table.insert(instanceData.portalTriggers, creature)
    end
end)

RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    local key = gameObjectBinds[entry]
    if key then
        instanceData[key] = go
    end
    if entry == GO_FOCUSING_IRIS_10 or entry == GO_FOCUSING_IRIS_25 then
        instanceData.focusingIrisPosition = { go:GetX(), go:GetY(), go:GetZ(), go:GetO() }
    elseif entry == GO_EXIT_PORTAL then
        instanceData.exitPortalPosition = { go:GetX(), go:GetY(), go:GetZ(), go:GetO() }
    end
end)

RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if instanceData.malygosDone then
        player:CastSpell(player, SPELL_SUMMOM_RED_DRAGON_BUDDY)
    end
end)

RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.malygosDone and not instanceData.malygosProcessed then
        instanceData.malygosProcessed = true
        -- C++ SetBossState DATA_MALYGOS_EVENT DONE
        -- (instance_eye_of_eternity.cpp:76-77): SpawnGameObject
        -- GO_EXIT_PORTAL at the exit-portal position.
        if instanceData.malygos then
            local pos = instanceData.exitPortalPosition
            if not pos then
                local portal = instanceData.malygos:FindNearestGameObject(GO_EXIT_PORTAL, 1000)
                if portal then
                    pos = { portal:GetX(), portal:GetY(), portal:GetZ(), portal:GetO() }
                end
            end
            if pos then
                instanceData.malygos:SummonGameObject(GO_EXIT_PORTAL, pos[1], pos[2], pos[3], pos[4], 0)
            end
        end
    end
end)
