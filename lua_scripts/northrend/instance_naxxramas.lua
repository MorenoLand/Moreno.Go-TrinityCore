-- The Naxxramas instance script — Lua port of
-- src/server/scripts/Northrend/Naxxramas/instance_naxxramas.cpp
-- (class instance_naxxramas : public InstanceMapScript, map 533).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_the_black_morass.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 533
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- below share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- <boss>Done flags written by the boss onDied handlers (aeonus
-- precedent) — maexxnaDone, loathebDone, thaddiusDone, gothikDone,
-- sapphironDone, kelthuzadDone, baronDone/thaneDone/ladyDone/
-- sirDone, horsemenDone (set by the fourth horseman's onDied);
-- kelthuzad/lichKing/thane/lady/baron/sir creature objects bound
-- by ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches —
-- the remaining C++ binds, anubrekhan/faerlina/razuvious/gothik/
-- gluth/heigan/thaddius/feugen/stalagg/sapphiron, are bound too;
-- no Lua consumer reads them yet, they mirror the C++ latch set);
-- kelthuzadDoor (GO_ROOM_KELTHUZAD object found by sapphiron's
-- onDied, opened at the end of the sapphiron chain);
-- hadSapphironBirth (set by go_sapphiron_birth.lua on use; read by
-- sapphiron's ev5 spawn handler — the C++ instance
-- SetData(DATA_HAD_SAPPHIRON_BIRTH, 1) leg in Lua form);
-- wingTaunt (next SAY_KELTHUZAD wing-taunt id, starts 16).
-- Ported arms (C++-exact unless noted):
-- - SetBossState DONE legs, re-expressed at DONE time (see below):
--   wing taunts (maexxna/loatheb/thaddius/horsemen DONE each
--   schedule EVENT_KELTHUZAD_WING_TAUNT at 6s; the taunt id starts
--   at SAY_KELTHUZAD_FIRST_WING_TAUNT 16 and increments per taunt,
--   instance_naxxramas.cpp:466-525), gothik dialogue chain
--   (GOTHIK DONE + 10s: thane/lady/sir/baron Talk 5, then lady/
--   sir/thane/baron Talk 6 at 5s/6s/6s/6s/6s/6s/6s), sapphiron
--   dialogue chain (SAPPHIRON DONE + 6s: kelthuzad Talk 0,
--   lich king Talk 1, kelthuzad Talk 2, lich king Talk 2,
--   kelthuzad Talk 4, kelthuzad Talk 20 at 6s/6s/16s/9s/12s/6s,
--   then the kelthuzad door opens, GO_STATE_ACTIVE 0).
-- - The quarter-portal / throne / chest flag legs run in the boss
--   onDied handlers (anchored FindNearestGameObject, entry-unique
--   so the 1000yd radius cannot pick a wrong GO — the bjarngrim
--   fight-unique idiom), because the C++ OnGameObjectCreate legs
--   cannot fire in Go: static template GOs never pass through the
--   Go GO-create funnel (instance_hooks.go:306 — dynamic spawns
--   only), so the create-time form of those legs is vacuous here
--   and the DONE-time form is the live one.
-- Unmodeled (documented-only, no bridges):
-- - Boss boundaries + door data: no boundary model and no
--   boss-state door machine in Go (debug boundary axis blocked).
-- - OnGameObjectCreate portal/throne legs in their C++ create-time
--   form (static-create never fires; the DONE-time form ports, see
--   above); the GO_BIRTH create leg ports as the sapphironDone
--   guard in go_sapphiron_birth.lua (had-birth is the use latch).
-- - Sapphiron door-close on non-DONE SetBossState: Go observes
--   only DONE (via onDied), so the HandleGameObject(false) leg
--   runs at DONE, not on every state transition.
-- - Horsemen chest SetRespawnTime(GetRespawnDelay()): no GO
--   GetRespawnDelay bridge, so only the RemoveFlag leg ports.
-- - Kel'Thuzad LoadGrid legs: no grid-load bridge; a wing
--   taunt or chain step whose speaker object is not yet bound
--   (creature not yet materialized) is skipped, not queued.
-- - Gothik gate SetData legs: the gate state writes ride the
--   unported gothik wave machine (boss_gothik.cpp:322-377).
-- - playerDied latch + Immortal/Undying + 7600/7601 criteria:
--   no OnUnitDeath map hook and no achievement-criteria bridge.
-- - SaveToDB / instance-data persistence across restarts: the
--   table lives for the runtime lifetime only (black-morass delta).

local MAP_NAXXRAMAS = 533
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_ANUBREKHAN = 15956
local NPC_FAERLINA = 15953
local NPC_RAZUVIOUS = 16061
local NPC_GOTHIK = 16060
local NPC_THANE = 16064
local NPC_LADY = 16065
local NPC_BARON = 30549
local NPC_SIR = 16063
local NPC_GLUTH = 15932
local NPC_HEIGAN = 15936
local NPC_THADDIUS = 15928
local NPC_FEUGEN = 15930
local NPC_STALAGG = 15929
local NPC_SAPPHIRON = 15989
local NPC_KELTHUZAD = 15990
local NPC_LICH_KING = 16980

local SAY_DIALOGUE_GOTHIK_HORSEMAN = 5
local SAY_DIALOGUE_GOTHIK_HORSEMAN2 = 6
local SAY_DIALOGUE_SAPPHIRON_KELTHUZAD = 0
local SAY_DIALOGUE_SAPPHIRON_KELTHUZAD2 = 2
local SAY_DIALOGUE_SAPPHIRON_KELTHUZAD3 = 4
local SAY_DIALOGUE_SAPPHIRON_KELTHUZAD4 = 20
local SAY_DIALOGUE_SAPPHIRON_LICH_KING = 1
local SAY_DIALOGUE_SAPPHIRON_LICH_KING2 = 2
local SAY_KELTHUZAD_FIRST_WING_TAUNT = 16

local GO_STATE_ACTIVE = 0

local creatureBinds = {
    [NPC_ANUBREKHAN] = "anubrekhan",
    [NPC_FAERLINA] = "faerlina",
    [NPC_RAZUVIOUS] = "razuvious",
    [NPC_GOTHIK] = "gothik",
    [NPC_THANE] = "thane",
    [NPC_LADY] = "lady",
    [NPC_BARON] = "baron",
    [NPC_SIR] = "sir",
    [NPC_GLUTH] = "gluth",
    [NPC_HEIGAN] = "heigan",
    [NPC_THADDIUS] = "thaddius",
    [NPC_FEUGEN] = "feugen",
    [NPC_STALAGG] = "stalagg",
    [NPC_SAPPHIRON] = "sapphiron",
    [NPC_KELTHUZAD] = "kelthuzad",
    [NPC_LICH_KING] = "lichKing",
}

RegisterMapEvent(MAP_NAXXRAMAS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = creatureBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)

-- Chained one-shots (temporus idiom): each step guards its speaker
-- (LoadGrid delta — an unbound speaker's line is skipped, the
-- chain still advances).
local function gothikChain(data)
    local function step(speakerKey, talkId, nextDelay, nextFn)
        CreateLuaEvent(function()
            local speaker = data[speakerKey]
            if speaker then
                speaker:Talk(talkId)
            end
            if nextFn then
                nextFn()
            end
        end, nextDelay, 1)
    end
    step("thane", SAY_DIALOGUE_GOTHIK_HORSEMAN, 10000, function()
        step("sir", SAY_DIALOGUE_GOTHIK_HORSEMAN, 5000, function()
            step("lady", SAY_DIALOGUE_GOTHIK_HORSEMAN, 6000, function()
                step("baron", SAY_DIALOGUE_GOTHIK_HORSEMAN, 6000, function()
                    step("lady", SAY_DIALOGUE_GOTHIK_HORSEMAN2, 6000, function()
                        step("sir", SAY_DIALOGUE_GOTHIK_HORSEMAN2, 6000, function()
                            step("thane", SAY_DIALOGUE_GOTHIK_HORSEMAN2, 6000, function()
                                step("baron", SAY_DIALOGUE_GOTHIK_HORSEMAN2, 6000, nil)
                            end)
                        end)
                    end)
                end)
            end)
        end)
    end)
end

local function sapphironChain(data)
    local function step(speakerKey, talkId, delay, nextFn)
        CreateLuaEvent(function()
            local speaker = data[speakerKey]
            if speaker then
                speaker:Talk(talkId)
            end
            if nextFn then
                nextFn()
            end
        end, delay, 1)
    end
    step("kelthuzad", SAY_DIALOGUE_SAPPHIRON_KELTHUZAD, 6000, function()
        step("lichKing", SAY_DIALOGUE_SAPPHIRON_LICH_KING, 6000, function()
            step("kelthuzad", SAY_DIALOGUE_SAPPHIRON_KELTHUZAD2, 16000, function()
                step("lichKing", SAY_DIALOGUE_SAPPHIRON_LICH_KING2, 9000, function()
                    step("kelthuzad", SAY_DIALOGUE_SAPPHIRON_KELTHUZAD3, 12000, function()
                        step("kelthuzad", SAY_DIALOGUE_SAPPHIRON_KELTHUZAD4, 6000, function()
                            if data.kelthuzadDoor then
                                data.kelthuzadDoor:SetGoState(GO_STATE_ACTIVE)
                            end
                        end)
                    end)
                end)
            end)
        end)
    end)
end

local function wingTaunt(data)
    if not data.kelthuzad then
        return
    end
    local id = data.wingTaunt or SAY_KELTHUZAD_FIRST_WING_TAUNT
    data.wingTaunt = id + 1
    local kelthuzad = data.kelthuzad
    CreateLuaEvent(function()
        kelthuzad:Talk(id)
    end, 6000, 1)
end

RegisterMapEvent(MAP_NAXXRAMAS, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.maexxnaDone and not instanceData.maexxnaProcessed then
        instanceData.maexxnaProcessed = true
        wingTaunt(instanceData)
    end
    if instanceData.loathebDone and not instanceData.loathebProcessed then
        instanceData.loathebProcessed = true
        wingTaunt(instanceData)
    end
    if instanceData.thaddiusDone and not instanceData.thaddiusProcessed then
        instanceData.thaddiusProcessed = true
        wingTaunt(instanceData)
    end
    if instanceData.horsemenDone and not instanceData.horsemenProcessed then
        instanceData.horsemenProcessed = true
        wingTaunt(instanceData)
    end
    if instanceData.gothikDone and not instanceData.gothikProcessed then
        instanceData.gothikProcessed = true
        gothikChain(instanceData)
    end
    if instanceData.sapphironDone and not instanceData.sapphironProcessed then
        instanceData.sapphironProcessed = true
        sapphironChain(instanceData)
    end
end)
