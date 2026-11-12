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
-- lists (GUIDs in C++, live objects here); lastPortalGUID — the
-- PowerSparksHandling rotation cursor (a trigger GUID; C++
-- lastPortalGUID, instance_eye_of_eternity.cpp:274); platform /
-- iris / exitPortal / heartOfMagic — the GO objects bound by
-- ON_GAMEOBJECT_CREATE (dynamic spawns only, see below);
-- exitPortalPosition / focusingIrisPosition — the latched GO
-- positions; malygosDone — written by the Malygos onDied
-- handler (maexxna precedent); malygosProcessed — owned by the
-- ON_UPDATE poll below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: vortex/portal trigger lists + malygos /
--   alexstrasza-bunny / gift-box-bunny binds (the C++ GUID
--   latches; the trigger lists feed the SetData handlers below —
--   VortexHandling / PowerSparksHandling — they mirror the C++
--   latch set, naxxramas precedent).
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
-- - ProcessEvent EVENT_FOCUSING_IRIS (20711): PORTED this tick —
--   no instance ProcessEvent bridge (oculus precedent), so the
--   body rides the GO-use hook (event 14) on both iris entries
--   below (focusingIrisUse); the receiver (Malygos DoAction(0),
--   ACTION_LAND_ENCOUNTER_START) is PORTED in boss_malygos.lua.
-- - SetData dispatch: PORTED 2026-11-08 (tick 431 + tick 432,
--   below) via the Go-only InstanceEventOnSetData bridge
--   (event 8, InstanceData:SetData) — DATA_VORTEX_HANDLING runs
--   VortexHandling, DATA_POWER_SPARKS_HANDLING runs
--   PowerSparksHandling, DATA_RESPAWN_IRIS respawns the
--   focusing iris (the iris leg is dormant — its writer is
--   FAIL-gated, see boss_malygos.lua).
-- - OnUnitDeath player root (SetControlled ROOT on player
--   death, the EoE falling fix): PORTABLE via the Go-only
--   INSTANCE_EVENT_ON_UNIT_DEATH (9, instance_hooks.go
--   triggerInstanceUnitDeathPlayer — fires from killPlayer's
--   setDeathState leg, incl. fall deaths) + player:SetRooted()
--   (lua_player.go:3427, the SetControlled ROOT twin) — PORTED
--   tick 513: RegisterMapEvent(616, 9, ...) gating on
--   unit:GetObjectType() == "Player".
-- - Boss boundaries (CircleBoundary 280yd): no boundary model
--   in Go (naxxramas precedent).
-- - SaveToDB / instance-data persistence across restarts: the
--   table lives for the runtime lifetime only (black-morass delta).

local MAP_EYE_OF_ETERNITY = 616
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

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
local SPELL_IRIS_OPENED = 61012 -- eye_of_eternity.h:86
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3 (Go gameObjectFlags twin)
local GO_FLAG_IN_USE = 1 -- SharedDefines.h:1637 (chromaggus precedent)

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

-- C++ instance_eye_of_eternity::ProcessEvent EVENT_FOCUSING_IRIS
-- (20711, instance_eye_of_eternity.cpp:164-181): the Alexstrasza
-- bunny casts SPELL_IRIS_OPENED on itself, the iris gains
-- GO_FLAG_IN_USE, Malygos takes DoAction(0)
-- (ACTION_LAND_ENCOUNTER_START, boss_malygos.lua), the exit
-- portal is deleted. The sender is the player clicking the
-- focusing iris — the Go twin is the GameObjectEventOnUse hook
-- (event 14, gameobjects.go:1426, the sapphiron-birth precedent);
-- there is no instance ProcessEvent bridge, so the body rides
-- the GO-use hook directly. The iris and exit portal are static
-- DB spawns, so the C++ Delete() legs ride go:Despawn() (hide for
-- the instance lifetime, sapphiron precedent — Go Delete() only
-- removes dynamic objects, lua_objects.go:4567). The irisUsed
-- latch + returning true keep a second click from re-firing the
-- machine while the flag disables interaction in C++. PORTED
-- this tick.
local function focusingIrisUse(event, go, player)
    if player == nil then
        return
    end
    local mapObj = go:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil or data.irisUsed then
        return
    end
    data.irisUsed = true
    if data.alexstraszaBunny ~= nil then
        data.alexstraszaBunny:CastSpell(data.alexstraszaBunny, SPELL_IRIS_OPENED)
    end
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE)
    if data.malygos ~= nil then
        data.malygos:DoAction(0) -- ACTION_LAND_ENCOUNTER_START
    end
    if data.exitPortal ~= nil then
        data.exitPortal:Despawn()
    end
    return true
end

RegisterGameObjectEvent(GO_FOCUSING_IRIS_10, 14, focusingIrisUse)
RegisterGameObjectEvent(GO_FOCUSING_IRIS_25, 14, focusingIrisUse)

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

-- C++ instance_eye_of_eternity::SetData
-- (instance_eye_of_eternity.cpp:230-244): DATA_VORTEX_HANDLING
-- (2) -> VortexHandling (:180-207), PORTED 2026-11-08 (tick 431);
-- DATA_POWER_SPARKS_HANDLING (3) -> PowerSparksHandling (:209-226),
-- PORTED 2026-11-08 (tick 432, below);
-- DATA_RESPAWN_IRIS (4) -> SpawnGameObject the focusing iris
-- (:240-242), PORTED 2026-11-08 (tick 432, below). All three ride
-- the Go-only InstanceEventOnSetData bridge (event 8,
-- InstanceData:SetData); the writers live in boss_malygos.lua
-- (EVENT_POWER_SPARKS timer, ACTION_HANDLE_RESPAWN, and the
-- ACTION_EXECUTE_VORTEX leg from tick 431).
local INSTANCE_EVENT_ON_SET_DATA = 8
local DATA_VORTEX_HANDLING = 2 -- eye_of_eternity.h:31
local DATA_POWER_SPARKS_HANDLING = 3 -- eye_of_eternity.h:32
local DATA_RESPAWN_IRIS = 4 -- eye_of_eternity.h:33
local SPELL_VORTEX_4 = 55853 -- eye_of_eternity.h:82
local SPELL_PORTAL_OPENED = 61236 -- eye_of_eternity.h:84

RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type == DATA_VORTEX_HANDLING then
        -- C++ instance_eye_of_eternity::SetData DATA_VORTEX_HANDLING
        -- (instance_eye_of_eternity.cpp:234-236) -> VortexHandling
        -- (:180-207): for each vortex trigger, up to 5 threat-list
        -- players each cast SPELL_VORTEX_4 on the trigger to enter
        -- the vehicle (the GM skip rides target:IsGM(),
        -- lua_player.go:1411 — the same twin as the vortexLand
        -- handler in boss_malygos.lua). Rides the
        -- Go-only InstanceEventOnSetData bridge (event 8,
        -- InstanceData:SetData); the writer is the
        -- ACTION_EXECUTE_VORTEX leg in boss_malygos.lua, emulating
        -- the 56105 SpellScript's HandleScript
        -- (boss_malygos.cpp:1765), which does only the SetData
        -- write. PORTED 2026-11-08 (tick 431).
        local malygos = instanceData.malygos
        if not malygos or not instanceData.vortexTriggers then
            return
        end
        for _, trigger in ipairs(instanceData.vortexTriggers) do
            local counter = 0
            for _, target in ipairs(malygos:GetAITargets()) do
                if counter >= 5 then
                    break
                end
                if target:GetObjectType() == "Player" and not target:IsGM() and not target:HasAura(SPELL_VORTEX_4) then
                    target:CastSpell(trigger, SPELL_VORTEX_4, true)
                    counter = counter + 1
                end
            end
        end
    elseif type == DATA_POWER_SPARKS_HANDLING then
        -- C++ PowerSparksHandling (instance_eye_of_eternity.cpp:209-226):
        -- the next portal trigger after lastPortalGUID (or the first
        -- when none / the last was the list tail) casts
        -- SPELL_PORTAL_OPENED on itself, triggered; lastPortalGUID
        -- advances to it and the handler returns. The writer is
        -- EVENT_POWER_SPARKS in boss_malygos.lua (tick 432). The
        -- cast itself lands (the creature-cast path needs no DBC
        -- row); the trigger's SpellHit response (npc_portal_eoe.lua)
        -- is dormant until Spell.dbc ships — the hit tail is
        -- spellKnown-gated (boss_ai.go:3805). PORTED 2026-11-08
        -- (tick 432).
        local triggers = instanceData.portalTriggers
        if not triggers or #triggers == 0 then
            return
        end
        local lastGuid = instanceData.lastPortalGUID
        local tail = triggers[#triggers]
        local next = false
        if lastGuid == nil or (tail ~= nil and tail:GetGUID() == lastGuid) then
            next = true
        end
        for _, trigger in ipairs(triggers) do
            if next then
                instanceData.lastPortalGUID = trigger:GetGUID()
                trigger:CastSpell(trigger, SPELL_PORTAL_OPENED, true)
                return
            end
            if trigger:GetGUID() == lastGuid then
                next = true
            end
        end
    elseif type == DATA_RESPAWN_IRIS then
        -- C++ SetData DATA_RESPAWN_IRIS (instance_eye_of_eternity.cpp:240-242):
        -- SpawnGameObject the focusing iris (10-man 193958 / 25-man
        -- 193960 by raid difficulty) at the latched iris position.
        -- The Go twin spawns via the bound Malygos (the exit-portal
        -- DONE-leg precedent, above); difficulty reads off Malygos
        -- (creature:Is25ManRaid). The position is the
        -- OnGameObjectCreate latch when a dynamic iris spawn fired
        -- it; the static iris never passes the Go GO-create funnel,
        -- so the fallback reads the live static iris via
        -- FindNearestGameObject (the exit-portal position fallback,
        -- above). DORMANT: the writer (ACTION_HANDLE_RESPAWN,
        -- boss_malygos.lua) is FAIL-gated and FAIL is unobservable
        -- in Go (boss_ai.go:195); also the iris is never deleted in
        -- Go (the land machine that deletes it is unported), so a
        -- live respawn would duplicate the static iris — the
        -- handler is exact for the C++ state, dormant in Go.
        -- PORTED 2026-11-08 (tick 432).
        local malygos = instanceData.malygos
        if not malygos then
            return
        end
        local pos = instanceData.focusingIrisPosition
        if not pos then
            local iris = malygos:FindNearestGameObject(GO_FOCUSING_IRIS_10, 1000)
            if not iris then
                iris = malygos:FindNearestGameObject(GO_FOCUSING_IRIS_25, 1000)
            end
            if iris then
                pos = { iris:GetX(), iris:GetY(), iris:GetZ(), iris:GetO() }
            end
        end
        if not pos then
            return
        end
        local entry = GO_FOCUSING_IRIS_10
        if malygos:Is25ManRaid() then
            entry = GO_FOCUSING_IRIS_25
        end
        malygos:SummonGameObject(entry, pos[1], pos[2], pos[3], pos[4], 0)
    end
end)

-- C++ instance OnUnitDeath (instance_eye_of_eternity.cpp:149-160):
-- any dying player is rooted (SetControlled ROOT, the EoE falling
-- fix — a dead player keeps moving otherwise, and the release box
-- may or may not pop). PORTED tick 513 via the Go-only
-- INSTANCE_EVENT_ON_UNIT_DEATH (9, instance_hooks.go
-- triggerInstanceUnitDeathPlayer — fires from killPlayer's
-- setDeathState leg, incl. fall deaths) + player:SetRooted()
-- (lua_player.go:3427, the SetControlled ROOT twin).
RegisterMapEvent(MAP_EYE_OF_ETERNITY, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit:GetObjectType() == "Player" then
        unit:SetRooted(true)
    end
end)
