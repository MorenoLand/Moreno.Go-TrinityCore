-- Trial of the Crusader instance script — Lua port of the bridgeable arms of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/instance_trial_of_the_crusader.cpp
-- (class instance_trial_of_the_crusader : public InstanceMapScript,
-- map 649, ToCrScriptName "instance_trial_of_the_crusader",
-- :99; AddSC at end registers it).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_trial_of_the_champion.lua precedent): the map binding covers
-- every Trial of the Crusader instance, and its presence opens the
-- Map:GetInstanceData gate for map 649
-- (engine/world/instance_hooks.go:42-58) that the faction-champions
-- JustDied / JustEngagedWith / JustReachedHome arms share
-- (boss_faction_champions.lua and the six npc_toc_* champion files).
-- Shared instanceData keys (one table per (map, instance), persistent
-- across fires): controller (NPC_CHAMPIONS_CONTROLLER 34781, C++
-- OnCreatureCreate binds DATA_FACTION_CRUSADERS,
-- instance_trial_of_the_crusader.cpp:50); championsKilled /
-- championsFailed / championsTotal / championsInProgress (controller
-- SetData(2, ...) machine, boss_faction_champions.cpp:505-562);
-- resilienceTimer (ms) / crusadersSpecial / resilienceInMinute
-- (Resilience Will Fix It, instance :437-440 set, :564-570 tick,
-- :206-213 read at DONE).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: champions controller GUID latch
--   (instance_trial_of_the_crusader.cpp:50; C++ stores the GUID, Lua
--   stores the object — the black-morass Medivh precedent).
-- - Update: while CrusadersSpecialState and the timer is live, count
--   it down (instance_trial_of_the_crusader.cpp:564-570).
-- The DONE / FAIL / IN_PROGRESS boss-state transitions themselves are
-- written by the champion files' shared counter helpers (the
-- controller has no UpdateAI override at all — SetData-only,
-- boss_faction_champions.cpp:367-578 — so there is no cast gate to
-- port here). Dormant until the announcer summon strand lands
-- (trial_of_the_crusader.cpp:613 EVENT_SUMMON_CHAMPIONS); no Lua file
-- summons 344xx champion entries today.
-- Unmodeled (documented-only, no bridges):
-- - DONE-time achievement criteria (defeat + killed-in-a-minute),
--   crusaders-chest respawn / flag clear, fordring DoAction
--   (ACTION_CHAMPIONS_DEFEATED) (instance :206-213): no achievement /
--   chest-respawn / cross-AI DoAction reader in Lua; resilienceInMinute
--   records the timer read for them.
-- - FAIL-time fordring wipe DoAction (ACTION_FACTION_WIPE)
--   (instance :201-205) + controller / summons despawn legs
--   (boss_faction_champions.cpp:530-534, :553-555): no cross-AI
--   despawn model in Lua.
-- - SaveToDB / WriteSaveDataMore / ReadSaveDataMore
--   (ResilienceWillFixItTimer, CrusadersSpecialState): the
--   instanceData table lives for the runtime lifetime only (the
--   black-morass persistence delta), so the save is a no-op here.

local MAP_TRIAL_OF_THE_CRUSADER = 649
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_CHAMPIONS_CONTROLLER = 34781 -- trial_of_the_crusader.h:179

RegisterMapEvent(MAP_TRIAL_OF_THE_CRUSADER, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_CHAMPIONS_CONTROLLER then
        instanceData.controller = creature
    end
end)

-- C++ instance Update (instance_trial_of_the_crusader.cpp:564-570):
-- while CrusadersSpecialState and the timer is live, count it down.
RegisterMapEvent(MAP_TRIAL_OF_THE_CRUSADER, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.crusadersSpecial and (instanceData.resilienceTimer or 0) > 0 then
        local timer = instanceData.resilienceTimer - diff
        if timer <= 0 then
            timer = 0
        end
        instanceData.resilienceTimer = timer
    end
end)
