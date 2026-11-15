-- Blackrock Depths instance script — Lua port of the SetData /
-- Load legs of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/
-- instance_blackrock_depths.cpp (class instance_blackrock_depths :
-- public InstanceMapScript, map 230; AddSC at end registers it;
-- blackrock_depths.h:46-49 DATA_GO_CHALICE 19 / DATA_GHOSTKILL 20 /
-- DATA_EVENSTARTER 21).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_eye_of_eternity.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 230
-- (engine/world/instance_hooks.go) that the Doomrel onDied handler
-- shares with this file.
-- Shared instanceData keys (one table per (map, instance), scalars
-- only — marshalled by Map:SaveInstanceData): ghostKillCount, the
-- running Tomb of Seven kill count (C++ GhostKillCount).
-- Ported arms (C++-exact unless noted):
-- - SetData DATA_GHOSTKILL (instance_blackrock_depths.cpp:255-257):
--   GhostKillCount += data — ADDITIVE, not assignment. The writer is
--   boss_doomrel.lua onDied (JustDied SetData(DATA_GHOSTKILL, 1),
--   boss_tomb_of_seven.cpp:193) via the Go-only InstanceData:SetData
--   bridge (event 8). Save when the count reaches 7 (C++ SetData
--   saves when data == DONE or GhostKillCount >= 7; the write here
--   is always data 1, so the >= 7 arm is the save trigger).
-- - Load (instance_blackrock_depths.cpp:360-372): partial counts
--   (0 < GhostKillCount < 7) reset to 0, counts >= 7 clamp to 7.
--   IsSilverHandDead-style per-boss flags are not persisted by C++
--   (GetSaveData writes only the encounter states + GhostKillCount),
--   so the load clamp is the whole restore leg.
-- - GetData DATA_GHOSTKILL: no function port — the live consumer is
--   boss_doomrel.lua onReset, which reads instanceData.ghostKillCount
--   directly (eye_of_eternity giftBoxBunny precedent).
-- - SetData TYPE_BAR (instance_blackrock_depths.cpp:240-247) + the
--   derived GetData (instance_blackrock_depths.cpp:277-283): SPECIAL
--   only bumps the ale count, any other write sets encounter[2];
--   GetData returns SPECIAL iff encounter[2] is IN_PROGRESS and the
--   ale count is 3, else encounter[2]. The fields ride plain
--   instanceData.barEncounter / .barAleCount (serpent-shrine
--   precedent); rocknot's OnQuestReward reads the derived state
--   from them. Load (:360-372) resets IN_PROGRESS encounters to
--   NOT_STARTED; BarAleCount is a runtime member, never saved, so
--   it resets to 0. Save fires on a DONE write (:258-259).
-- Unmodeled (documented-only, no bridges / no writers):
-- - Encounter states (TYPE_RING_OF_LAW / VAULT / TOMB_OF_SEVEN /
--   LYCEUM / IRON_HALL) and their GO binds + HandleGameObject door
--   legs: no instance script port for the rest of BRD; the
--   SetData switch cases for them stay out with the instance file.
--   (TYPE_BAR is the exception above, ported for rocknot.)
-- - SetGuidData DATA_EVENSTARTER (TombOfSevenStart/Reset): no
--   SetGuidData bridge on the instance surface; the Doomrel gossip
--   + evade legs that write it stay out (boss_doomrel.lua header).
-- - TombOfSeven Update machine (instance_blackrock_depths.cpp:424-447:
--   TombTimer activation of the next boss, dead-boss recount,
--   TombOfSevenEnd chest respawn + door opens): no instance OnUpdate
--   port for BRD; the per-boss activation legs (SetFaction hostile,
--   AttackStart nearest) stay out with it.
-- - DoRespawnGameObject(DATA_GO_CHALICE) (gloomrel tribute leg):
--   no instance-GO-respawn bridge (boss_gloomrel.lua header).

local MAP_BLACKROCK_DEPTHS = 230 -- instance_blackrock_depths.cpp:84
local INSTANCE_EVENT_ON_LOAD = 2
local INSTANCE_EVENT_ON_SET_DATA = 8

local DATA_GHOSTKILL = 20 -- blackrock_depths.h:48
local TYPE_BAR = 3 -- blackrock_depths.h:30
local ENCOUNTER_IN_PROGRESS = 1 -- InstanceScript.h:71
local ENCOUNTER_DONE = 3 -- InstanceScript.h:73
local ENCOUNTER_SPECIAL = 4 -- InstanceScript.h:74

RegisterMapEvent(MAP_BLACKROCK_DEPTHS, INSTANCE_EVENT_ON_SET_DATA,
    function(event, instanceData, mapObj, type, data)
        if type == DATA_GHOSTKILL then
            -- C++ SetData DATA_GHOSTKILL (:255-257): GhostKillCount += data.
            instanceData.ghostKillCount = (instanceData.ghostKillCount or 0) + data
            if instanceData.ghostKillCount >= 7 then
                mapObj:SaveInstanceData()
            end
        elseif type == TYPE_BAR then
            -- C++ SetData TYPE_BAR (:240-247): SPECIAL only bumps the ale
            -- count; any other write sets encounter[2].
            if data == ENCOUNTER_SPECIAL then
                instanceData.barAleCount = (instanceData.barAleCount or 0) + 1
            else
                instanceData.barEncounter = data
            end
            -- C++ SetData save arm (:258-259): a DONE write saves.
            if data == ENCOUNTER_DONE then
                mapObj:SaveInstanceData()
            end
        end
    end)

RegisterMapEvent(MAP_BLACKROCK_DEPTHS, INSTANCE_EVENT_ON_LOAD,
    function(event, instanceData, mapObj)
        -- C++ Load (:366-369): partial counts reset, full counts clamp.
        local count = instanceData.ghostKillCount or 0
        if count > 0 and count < 7 then
            instanceData.ghostKillCount = 0
        elseif count >= 7 then
            instanceData.ghostKillCount = 7
        end
        -- C++ Load (:370-372): IN_PROGRESS encounters reset to
        -- NOT_STARTED. BarAleCount is a runtime member, never saved
        -- (GetSaveData writes only the encounters + GhostKillCount),
        -- so it resets to 0.
        if (instanceData.barEncounter or 0) == ENCOUNTER_IN_PROGRESS then
            instanceData.barEncounter = 0
        end
        instanceData.barAleCount = 0
    end)
