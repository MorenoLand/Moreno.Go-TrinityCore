-- Stratholme instance script — Lua port of the SetData /
-- GetData Silver Hand legs of
-- src/server/scripts/EasternKingdoms/Stratholme/instance_stratholme.cpp
-- (class instance_stratholme : public InstanceMapScript, map 329;
-- AddSC at end registers it; stratholme.h:20-25 TYPE_SH_QUEST 20 /
-- TYPE_SH_CATHELA 21 / TYPE_SH_GREGOR 22 / TYPE_SH_NEMAS 23 /
-- TYPE_SH_VICAR 24 / TYPE_SH_AELMAR 25).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_eye_of_eternity.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 329
-- (engine/world/instance_hooks.go) that the Silver Hand onReset /
-- onDied handlers share with this file.
-- Shared instanceData keys (one table per (map, instance), scalars
-- only — marshalled by Map:SaveInstanceData): shAelmarDead /
-- shCathelaDead / shGregorDead / shNemasDead / shVicarDead, the
-- per-boss dead flags (C++ IsSilverHandDead[0..4]); shQuest, the
-- derived all-five-dead flag (C++ GetData(TYPE_SH_QUEST): 1 iff all
-- five dead, else 0). The key names are the boss file's
-- (boss_silver_hand_bosses.lua SH_DEAD_KEY) so both writers share
-- one store.
-- Ported arms (C++-exact unless noted):
-- - SetData TYPE_SH_* (instance_stratholme.cpp:387-400):
--   IsSilverHandDead[i] = (data != 0), as the event-8 consumer
--   below. The live writers are boss_silver_hand_bosses.lua onReset
--   (0 -> alive) and onDied (2 -> dead), which write the table
--   fields directly (argent-soldier precedent); this handler is the
--   bridge-shaped twin for any future InstanceData:SetData writer
--   and runs the same switch, including the shQuest derivation and
--   the data == DONE (2) save arm (instance_stratholme.cpp:403-404).
-- - GetData TYPE_SH_QUEST (instance_stratholme.cpp:449-453): the
--   shQuest derivation below; the live consumer is
--   boss_silver_hand_bosses.lua onDied, which computes the same
--   all-five-dead check inline before the quest-credit leg.
-- - Load: no handler — C++ Load restores only EncounterState[0..5]
--   (GetSaveData writes no Silver Hand state), so the dead flags
--   start unset (nil reads as false) on every load, C++-exact.
-- Unmodeled (documented-only, no bridges / no writers):
-- - Encounter states (TYPE_BARON_RUN / BARONESS / NERUB / PALLID /
--   RAMSTEIN / BARON) and their ziggurat / gauntlet GO legs: no
--   instance script port for the rest of Stratholme; the SetData
--   switch cases for them stay out with the instance file.

local MAP_STRATHOLME = 329 -- instance_stratholme.cpp:55
local INSTANCE_EVENT_ON_SET_DATA = 8

local TYPE_SH_CATHELA = 21
local TYPE_SH_GREGOR = 22
local TYPE_SH_NEMAS = 23
local TYPE_SH_VICAR = 24
local TYPE_SH_AELMAR = 25

local DONE = 2 -- InstanceScript.h EncounterState (the SetData save arm)

local SH_DEAD_KEY = {
    [TYPE_SH_AELMAR] = "shAelmarDead",
    [TYPE_SH_CATHELA] = "shCathelaDead",
    [TYPE_SH_GREGOR] = "shGregorDead",
    [TYPE_SH_NEMAS] = "shNemasDead",
    [TYPE_SH_VICAR] = "shVicarDead",
}

RegisterMapEvent(MAP_STRATHOLME, INSTANCE_EVENT_ON_SET_DATA,
    function(event, instanceData, mapObj, type, data)
        local key = SH_DEAD_KEY[type]
        if not key then
            return
        end
        -- C++ SetData TYPE_SH_* (:387-400): IsSilverHandDead[i] = data != 0.
        instanceData[key] = data ~= 0
        -- C++ GetData TYPE_SH_QUEST (:449-453): 1 iff all five dead.
        if instanceData.shAelmarDead and instanceData.shCathelaDead
                and instanceData.shGregorDead and instanceData.shNemasDead
                and instanceData.shVicarDead then
            instanceData.shQuest = 1
        else
            instanceData.shQuest = 0
        end
        if data == DONE then
            mapObj:SaveInstanceData()
        end
    end)
