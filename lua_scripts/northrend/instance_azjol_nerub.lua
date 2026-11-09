-- Azjol-Nerub instance script — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/AzjolNerub/instance_azjol_nerub.cpp
-- (instance_azjol_nerub, InstanceMapScript "instance_azjol_nerub", map 601).
-- RegisterMapEvent binds the map-level Eluna instance hook
-- (instance_ahnkahet.lua / instance_gundrak.lua precedent).
--
-- PORTED — OnUnitDeath gatewatcher greet relay
-- (instance_azjol_nerub.cpp:79-92): any non-critter, non-charmed
-- creature death while Krik'thir is not DONE relays
-- DoAction(-ACTION_GATEWATCHER_GREET) to Krik'thir via
-- GetCreature(DATA_KRIKTHIR). The Lua twin rides the Go-only
-- INSTANCE_EVENT_ON_UNIT_DEATH (9, instance_hooks.go:505-529;
-- signature (event, instanceData, map, unit), fires for creature
-- and player deaths, before any CREATURE_EVENT_ON_DIED handler).
-- The TYPEID_UNIT gate is the unit:GetObjectType() == "Creature"
-- filter (player deaths return "Player", lua_player.go:147);
-- the GetBossState(DATA_KRIKTHIR) == DONE gate is the
-- instanceData.krikthirDone flag, written by Krik'thir's ev4
-- handler together with creature:SetBossState(DATA_KRIKTHIR, DONE)
-- (ahnkahet precedent — no boss-state-change event exists);
-- the charmed gate is unit:IsCharmed()
-- (lua_creature_events.go:875), the wider twin of
-- IsCharmedOwnedByPlayerOrPlayer (no bridge for the narrower
-- player-owned form); the relay resolves via
-- unit:GetInstanceCreature(DATA_KRIKTHIR)
-- (lua_creature_events.go:6488, latched by Krik'thir's ev5
-- SetGuidData) and dispatches creature:DoAction(-1)
-- (lua_creature_events.go:827 -> ev38), received by the
-- krikthirDoAction handler in boss_krikthir_the_gatewatcher.lua.
-- PORTED — GetData / SetData DATA_GATEWATCHER_GREET (:104-119):
-- the greet flag is a plain instance member (no save/load in this
-- file — nothing persists). The Krik'thir ev38 handler writes it
-- via instanceData:SetData(DATA_GATEWATCHER_GREET, 1), which fires
-- OnSetData (event 8, instance_hooks.go:51-63); the handler stores
-- it as instanceData.gatewatcherGreet (ahnkahet sphere precedent).
--
-- STAY-OUTS (triaged this tick):
-- doorData (GO_KRIKTHIR_DOOR 192395 passage on DATA_KRIKTHIR +
-- three GO_ANUBARAK_DOOR room doors, :22-28) + LoadDoorData — Go
-- has no door model: setInstanceBossState explicitly skips door
-- updates (instance_boss_states.go:148-152), same as the
-- ICC/Gundrak/Ahn'kahet door legs.
-- creatureData / gameobjectData LoadObjectData (:30-47) — the
-- boss/watcher GUID latches ride the boss files' ev5
-- SetGuidData handlers instead (the instance-side OnCreatureCreate
-- route is dead for static spawns — instance_hooks.go:553-562);
-- the watcher rows are ported VERBATIM with their rotated mapping
-- (narjil->GASHRA, gashra->SILTHIK, silthik->NARJIL, :39-41).
-- The Anub'arak wall GOs (DATA_ANUBARAK_WALL/_2) have no Lua
-- consumers — no latch is needed.
-- BossBoundaryData (Krik'thir rectangle, Hadronox Z-range,
-- Anub'arak circle, :49-54) — no boundary bridge in the runtime.
-- CheckRequiredBosses (:94-102, Krik'thir-DONE gate for later
-- bosses) — no bridge (creaturemotion.go:3756); the Go
-- boss-state store DOES carry the DONE writes (boss ev4
-- SetBossState legs), so the gate is live data waiting on a
-- bridge, not lost state.
-- LoadHeaders / SetBossNumber / ctor GateWatcherGreet = 0 init —
-- no bridges; the nil-default is the ctor init.
-- DEVIATION (no bridge): IsCritter has no Lua surface
-- (creaturemotion.go:893 is a comment only), so critter deaths
-- relay the greet exactly like any other creature death; C++
-- skips them. The greet flag still caps it at one Talk.

local MAP_AZJOL_NERUB = 601

local INSTANCE_EVENT_ON_SET_DATA = 8
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

-- azjol_nerub.h ANDataTypes.
local DATA_KRIKTHIR = 0
local DATA_GATEWATCHER_GREET = 8

local ACTION_GATEWATCHER_GREET = 1

-- C++ SetData (:112-119): the greet flag lands in the instance
-- member; no persistence (no WriteSaveDataMore in this file).
RegisterMapEvent(MAP_AZJOL_NERUB, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, value)
    if type == DATA_GATEWATCHER_GREET then
        instanceData.gatewatcherGreet = value
    end
end)

-- C++ OnUnitDeath (instance_azjol_nerub.cpp:79-92).
RegisterMapEvent(MAP_AZJOL_NERUB, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() ~= "Creature" then
        return
    end
    if instanceData.krikthirDone then
        return
    end
    if unit:IsCharmed() then
        return
    end
    local gatewatcher = unit:GetInstanceCreature(DATA_KRIKTHIR)
    if gatewatcher then
        gatewatcher:DoAction(-ACTION_GATEWATCHER_GREET)
    end
end)
