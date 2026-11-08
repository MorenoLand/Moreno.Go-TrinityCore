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
-- :206-213 read at DONE); trialCounter (heroic attempts left, default
-- 50 — instance :303-305 decrement on heroic FAIL, written by the
-- beasts / jaraxxus / champions / valkyr FAIL writers, read by the
-- Anub'arak tribute-chest tier; tick 406 U2); anyCreature (most
-- recently created creature, the GetBossState probe for the OnUnitDeath
-- player leg — the instance_ulduar.lua precedent);
-- tributeToImmortalityEligible (1 until the first player death while
-- an encounter is IN_PROGRESS, instance :165-169, read by the Go
-- TRIBUTE_TO_IMMORTALITY gate).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: champions controller GUID latch
--   (instance_trial_of_the_crusader.cpp:50; C++ stores the GUID, Lua
--   stores the object — the black-morass Medivh precedent).
-- - OnCreatureCreate: Anub'arak GUID latch (DATA_ANUBARAK 5,
--   creatureData :54 / LoadObjectData :108) via
--   creature:SetGuidData — the FAIL-arm TrialCounter == 0 despawn
--   leg reads it back (tick 408 U1).
-- - Update: while CrusadersSpecialState and the timer is live, count
--   it down (instance_trial_of_the_crusader.cpp:564-570).
-- - OnUnitDeath (instance_trial_of_the_crusader.cpp:165-169, tick 468):
--   a player death while any of the 6 encounters is IN_PROGRESS clears
--   tributeToImmortalityEligible (the C++ IsEncounterInProgress twin is
--   the 0..ENCOUNTER_COUNT-1 GetBossState loop over the anyCreature
--   probe; DATA_NORTHREND_BEASTS = 0 .. DATA_ANUBARAK = 5,
--   trial_of_the_crusader.h:32-37). Rides the Go player-death sender
--   (killPlayer / completeSpiritOfRedemptionDeath ->
--   INSTANCE_EVENT_ON_UNIT_DEATH 9, tick 467).
-- The DONE / FAIL / IN_PROGRESS boss-state transitions themselves are
-- written by the champion files' shared counter helpers (the
-- controller has no UpdateAI override at all — SetData-only,
-- boss_faction_champions.cpp:367-578 — so there is no cast gate to
-- port here). The announcer summon strand has landed
-- (npc_tirion_fordring.lua EVENT_SUMMON_CHAMPIONS summons the
-- controller, tick 406 U2) and the champion roster is PORTED tick 410
-- (boss_faction_champions.lua controllerReset: SelectChampions /
-- SummonChampions run on the controller's ev23 Reset, 3s unflag timer
-- for SetData(1)).
-- Unmodeled (documented-only, no bridges):
-- - DONE-time achievement criteria (defeat + killed-in-a-minute) +
--   crusaders-chest respawn / flag clear (instance :208-213): no
--   achievement / chest-respawn bridge; the fordring DoAction
--   (ACTION_CHAMPIONS_DEFEATED 13, instance :214-215) is PORTED in
--   the champion files' shared counter helpers (tick 406 U2);
--   resilienceInMinute records the timer read for them.
-- - FAIL-time fordring wipe DoAction (ACTION_FACTION_WIPE 25)
--   (instance :204-205) is PORTED in the same counter helpers
--   (tick 406 U2); the controller / summons despawn legs
--   (boss_faction_champions.cpp:530-534, :553-555) stay out: no cross-AI
--   despawn model in Lua.
-- - GetCreatureEntry TrialCounter == 0 summon prevention
--   (instance_trial_of_the_crusader.cpp:150-155): no Go hook —
--   C++ fires it in Creature::CreateFromProto (Creature.cpp:1626)
--   only for DB-data spawns, and Go has no spawn-entry override
--   point (zero GetCreatureEntry hits in engine/); static template
--   spawns never materialize server-side (instance_hooks.go
--   triggerInstanceCreatureCreate doc), so there is no object to
--   gate or despawn. Vacuous at load in any case: TrialCounter
--   resets to 50 in the instance-script ctor (:110), and the only
--   post-wipe spawns are scripted summons, which bypass the data
--   path; Anub'arak itself is covered by the tick-408 FAIL-arm
--   despawn. Revisit only if a Go spawn-entry hook lands.
-- - SaveToDB / WriteSaveDataMore / ReadSaveDataMore
--   (ResilienceWillFixItTimer, CrusadersSpecialState): the
--   instanceData table lives for the runtime lifetime only (the
--   black-morass persistence delta), so the save is a no-op here.

local MAP_TRIAL_OF_THE_CRUSADER = 649
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local BOSS_STATE_IN_PROGRESS = 1
local ENCOUNTER_COUNT = 6 -- trial_of_the_crusader.h:27

local NPC_CHAMPIONS_CONTROLLER = 34781 -- trial_of_the_crusader.h:179
local NPC_ANUBARAK = 34564 -- trial_of_the_crusader.h:219
local DATA_ANUBARAK = 5 -- trial_of_the_crusader.h:37

RegisterMapEvent(MAP_TRIAL_OF_THE_CRUSADER, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_CHAMPIONS_CONTROLLER then
        instanceData.controller = creature
    end
    -- C++ OnCreatureCreate latches NPC_ANUBARAK under DATA_ANUBARAK
    -- (creatureData :54, LoadObjectData :108); the FAIL-arm despawn
    -- leg reads it back via creature:GetInstanceCreature(5).
    if creature:GetEntry() == NPC_ANUBARAK then
        creature:SetGuidData(DATA_ANUBARAK)
    end
    -- Any-creature probe for the OnUnitDeath player leg (the
    -- instance_ulduar.lua precedent): GetBossState reads Go instance
    -- state off the creature's map/instance, so any motion works.
    instanceData.anyCreature = creature
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

-- OnUnitDeath (instance_trial_of_the_crusader.cpp:165-169) — PORTED
-- 2026-11-08 (tick 468). A player death while any encounter is
-- IN_PROGRESS clears tributeToImmortalityEligible (1 -> 0); the C++
-- IsEncounterInProgress twin is the 0..ENCOUNTER_COUNT-1 GetBossState
-- loop over the anyCreature probe. Rides the Go player-death sender
-- (tick 467). The latch defaults to 1 in the Go read (num() default),
-- matching the C++ ctor (:120); no creature leg exists in this
-- instance's OnUnitDeath. Players carry no GetEntry — the type check
-- runs first (the DTK precedent).
RegisterMapEvent(MAP_TRIAL_OF_THE_CRUSADER, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() ~= "Player" then
        return
    end
    if instanceData.tributeToImmortalityEligible == 0 then
        return
    end
    local probe = instanceData.anyCreature or instanceData.controller
    if not probe then
        return
    end
    for i = 0, ENCOUNTER_COUNT - 1 do
        if probe:GetBossState(i) == BOSS_STATE_IN_PROGRESS then
            instanceData.tributeToImmortalityEligible = 0
            return
        end
    end
end)
