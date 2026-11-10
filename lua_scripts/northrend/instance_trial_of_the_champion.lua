-- Instance: Trial of the Champion — Lua port of the bridgeable arms of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/instance_trial_of_the_champion.cpp
-- (class instance_trial_of_the_champion : public InstanceMapScript,
-- map 650, ToCScriptName "instance_trial_of_the_champion";
-- AddSC at end registers it).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_the_black_morass.lua precedent): the map binding covers
-- every Trial of the Champion instance, and its presence opens the
-- Map:GetInstanceData gate for map 650
-- (engine/world/instance_hooks.go:42-58) that the argent-soldier
-- JustDied arm shares (boss_eadric.lua).
-- Shared instanceData keys (one table per (map, instance), persistent
-- across fires): argentSoldierDefeated (DATA_ARGENT_SOLDIER_DEFEATED,
-- TCData value 8; nil reads as 0), argentChampionUngated (the 9-deaths
-- leg latch), announcer (the creature object, C++ uiAnnouncerGUID),
-- argentChampion (the creature object, C++ uiArgentChampionGUID).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: announcer (NPC_JAEREN 35004 / NPC_ARELAS 35005)
--   and argent champion (NPC_EADRIC 35119 / NPC_PALETRESS 34928) GUID
--   latches (instance_trial_of_the_champion.cpp:76-96; C++ stores the
--   GUID, Lua stores the object — the black-morass Medivh precedent).
--   Vehicle GUID list stays out (no vehicle spawns reach Lua).
-- - SetData DATA_ARGENT_SOLDIER_DEFEATED 9-leg
--   (instance_trial_of_the_champion.cpp:225-238): when the counter
--   reaches exactly 9, the champion walks to (746.88, 618.74, 411.06)
--   and is un-gated (RemoveFlag NON_ATTACKABLE + REACT_AGGRESSIVE).
--   The counter is written by the soldier JustDied arm, not via an
--   InstanceData:SetData call (the event-8 hook exists —
--   instance_hooks.go:43 — but the arm writes the shared table
--   directly, so no SetData fire occurs on that path), so the instance
--   script polls it on ON_UPDATE (the aeonus/medivh check-tick
--   precedent). Exact-equality preserves C++ semantics.
--   Bridges: creature:MoveTo (lua_creature_events.go:4624),
--   creature:RemoveFlag (lua_creature_events.go:5498),
--   creature:SetReactState (lua_creature_events.go:4014).
-- Unmodeled (documented-only, no bridges):
-- - SetBossState DONE arms (GRAND_CHAMPIONS / ARGENT_CHALLENGE_E/P:
--   announcer MovePoint + gossip flag + loot-chest SummonGameObject):
--   no boss-state-change instance event exists for Lua to observe the
--   transition (runtime.go InstanceEvents are 1-7 only). The grand
--   champions arm is C++-dead anyway (see boss_grand_champions.lua:
--   the instance counter runs once and stalls at 1).
-- - SetData DATA_MOVEMENT_DONE (uiMovementDone == 3 -> announcer
--   AI SetData(DATA_IN_POSITION, 0)): the announcer escort machine is
--   unported, so there is no bearer for either side.
-- - GetCreatureEntry team swap (vehicle/announcer entry remap on
--   teamInInstance): no spawn-entry-remap bridge; DB-side.
-- - OnPlayerEnter team latch: no consumer in Lua yet.
-- - SaveToDB on SetData(uiData == DONE): the counter value 3 coincides
--   with EncounterState DONE, so C++ saves on the 3rd soldier death;
--   the instanceData table lives for the runtime lifetime only (the
--   black-morass persistence delta), so the save is a no-op here.
-- - WriteSaveDataMore/ReadSaveDataMore (uiGrandChampionsDeaths /
--   uiMovementDone): same persistence delta; the boss-states blob
--   (tick 369) carries boss states only.

local MAP_TRIAL_OF_THE_CHAMPION = 650
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_EADRIC = 35119
local NPC_PALETRESS = 34928
local NPC_JAEREN = 35004
local NPC_ARELAS = 35005

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local REACT_AGGRESSIVE = 2

local ARGENT_SOLDIER_DEATHS_TO_UNGATE = 9

RegisterMapEvent(MAP_TRIAL_OF_THE_CHAMPION, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_JAEREN or entry == NPC_ARELAS then
        instanceData.announcer = creature
    elseif entry == NPC_EADRIC or entry == NPC_PALETRESS then
        instanceData.argentChampion = creature
    end
end)

-- C++ SetData DATA_ARGENT_SOLDIER_DEFEATED leg
-- (instance_trial_of_the_champion.cpp:225-238), polled: the counter is
-- written by the soldier JustDied arm (boss_eadric.lua) straight into
-- the shared table, not via InstanceData:SetData, so the event-8 hook
-- never fires on that path and ON_UPDATE observes the exact-9 transition.
RegisterMapEvent(MAP_TRIAL_OF_THE_CHAMPION, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.argentChampionUngated then
        return
    end
    if (instanceData.argentSoldierDefeated or 0) ~= ARGENT_SOLDIER_DEATHS_TO_UNGATE then
        return
    end
    local champion = instanceData.argentChampion
    if not champion then
        return
    end
    instanceData.argentChampionUngated = true
    champion:MoveTo(0, 746.88, 618.74, 411.06)
    champion:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    champion:SetReactState(REACT_AGGRESSIVE)
end)
