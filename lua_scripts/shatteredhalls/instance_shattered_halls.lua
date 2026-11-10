-- Shattered Halls instance script — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/ShatteredHalls/
-- instance_shattered_halls.cpp (instance_shattered_halls,
-- InstanceMapScript "instance_shattered_halls", map 540) and
-- shattered_halls.h (SHDataTypes, SHCreatureIds, SHGameobjectIds,
-- SHInstanceSpells).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_magtheridons_lair.lua precedent).
--
-- PORTED — door data (instance_shattered_halls.cpp:38-43 +
-- InstanceScript::UpdateDoorState, InstanceScript.cpp:203-205):
-- GO_GRAND_WARLOCK_CHAMBER_DOOR_1/2 (182539/182540,
-- DOOR_TYPE_PASSAGE) open iff DATA_NETHEKURSE is DONE.
-- boss_nethekurse.lua writes SetBossState(0, 3) on death; the
-- OnUpdate poll reads it back via the resolved Nethekurse
-- creature (creature:GetBossState, lua_creature_events.go:6624)
-- and latches instanceData.nethekurseDone, so the doors stay
-- open even once the boss corpse is unresolvable. The OnUpdate
-- poll (event 3) applies the door state, retrying on later
-- ticks if a door GO is unresolvable; the OnGameObjectCreate
-- handler (event 6) covers the dynamic-spawn path and the
-- OnPlayerEnter apply covers the static-spawn path (static GOs
-- never fire event 6 in Go — dynamic-spawn-only,
-- instance_hooks.go:488-503). Doors resolve entry-based via
-- player:FindNearestGameObject (lua_player.go:6710, 1000y —
-- the whole instance); the applied state is tracked in
-- instanceData (nethekurseDoorOpen) so re-fires are no-ops.
-- PORTED — execution-timer machine (instance_shattered_halls.
-- cpp:82-97/113-131/153-165/229-266): the Shattered
-- Executioner (17301) starts a 55min timer on creation; every
-- entering player gets the staged executioner aura
-- (39288/39289/39290 by executed count) with its remaining
-- duration, and the Update countdown advances the stages
-- (executed 1 -> 39289 + 10min, 2 -> 39290 + 15min, 3 ->
-- stop), re-casting the remove spell (39291) and writing
-- SetData(DATA_PRISONERS_EXECUTED) to the executioner at each
-- advance. Creation has no Go twin — temp summons bypass the
-- motion funnel and static spawns never fire event 5
-- (instance_hooks.go:556-571) — so the OnUpdate poll starts
-- the timer the first tick the executioner resolves via a
-- player grid search (heroic only: the C++ creation paths —
-- at_nethekurse_exit + ReadSaveDataMore — are heroic-gated).
-- The SetBossState-override DONE leg (executioner DONE ->
-- remove aura all players, timer 0) rides the same poll:
-- boss_shattered_executioner.lua writes SetBossState(3, 3)
-- on death and the poll latches instanceData.executionerDone.
-- SaveToDB has no bridge — the timer is runtime-only (the
-- Magtheridon done flags are the same). GetData
-- (DATA_PRISONERS_EXECUTED / DATA_TEAM_IN_INSTANCE) is served
-- from the instanceData fields (prisonersExecuted / team) for
-- Lua readers.
--
-- STAY-OUTS: SetHeaders ("SH") / SetBossNumber (4) — no
-- bridges (ICC precedent). OnCreatureCreate GUID latches
-- (nethekurse/kargath/executioner/victims, :99-131) — static
-- spawns never fire event 5 and summons bypass the funnel, so
-- no latch fires; GetGuidData has no live Lua consumer (the
-- areatrigger + ReadSaveDataMore consumers are unported, see
-- below) and entry-based grid resolution is the twin where a
-- GUID is needed. at_nethekurse_exit (shattered_halls.cpp:
-- 36-62: summon executioner + victims on heroic) — no
-- area-trigger bridge, so the executioner only ever appears
-- via a future areatrigger port or manual summon; the timer
-- machine above activates if/when it exists. Randy
-- Whizzlesprocket -> Drisella UpdateEntry (:107-109) — the
-- trigger is OnCreatureCreate (never fires for the static
-- spawn); the UpdateEntry bridge itself is live
-- (lua_creature_events.go:4824) for a future creature-side
-- handler. ReadSaveDataMore / WriteSaveDataMore (heroic
-- persistence of executed + executionTimer) — no bridge.
-- DATA_KARGATH SetBossState leg (executioner AI Reset ->
-- IMMUNE_TO_PC removal, :146-150) — needs the executioner
-- file's Reset to read GetBossState(2); see
-- boss_shattered_executioner.lua. The two SpellScripts
-- (39288 area check, 39291 remove) have no SpellScript
-- bridge; the remove leg is ported as direct casts above.

local MAP_SHATTERED_HALLS = 540

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- shattered_halls.h SHDataTypes.
local DATA_NETHEKURSE = 0
local DATA_SHATTERED_EXECUTIONER = 3
local DATA_PRISONERS_EXECUTED = 4

-- Lua-side transport id (no C++ counterpart — the executioner
-- AI has no DoAction override): fired on the executioner right
-- after each SetData(DATA_PRISONERS_EXECUTED) write so its
-- ev38 handler runs the SetData machine once per write
-- (boss_shattered_executioner.lua; broggok precedent).
local ACTION_PRISONERS_EXECUTED = 1

-- shattered_halls.h SHCreatureIds / SHGameobjectIds.
local NPC_GRAND_WARLOCK_NETHEKURSE = 16807
local NPC_SHATTERED_EXECUTIONER = 17301
local GO_GRAND_WARLOCK_CHAMBER_DOOR_1 = 182539
local GO_GRAND_WARLOCK_CHAMBER_DOOR_2 = 182540

-- shattered_halls.h SHInstanceSpells.
local SPELL_KARGATH_EXECUTIONER_1 = 39288
local SPELL_KARGATH_EXECUTIONER_2 = 39289
local SPELL_KARGATH_EXECUTIONER_3 = 39290
local SPELL_REMOVE_KARGATH_EXECUTIONER = 39291

local EXECUTIONER_SPELLS = {
    SPELL_KARGATH_EXECUTIONER_1,
    SPELL_KARGATH_EXECUTIONER_2,
    SPELL_KARGATH_EXECUTIONER_3,
}

-- instance_shattered_halls.cpp:113: 55min initial timer;
-- Update stage timers: 10min after executed 1, 15min after 2.
local EXECUTION_TIMER_INITIAL = 55 * 60 * 1000
local EXECUTION_TIMER_STAGE_2 = 10 * 60 * 1000
local EXECUTION_TIMER_STAGE_3 = 15 * 60 * 1000

local GO_STATE_ACTIVE = 0
local GO_STATE_READY = 1
local BOSS_STATE_DONE = 3

local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO/creature

-- Entry-based resolution from any player in the instance
-- (player:FindNearestCreature / FindNearestGameObject cover DB
-- spawns; the instance_steam_vault.lua precedent).
local function findCreature(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local creature = player:FindNearestCreature(entry, GO_SEARCH_DIST)
        if creature then
            return creature
        end
    end
    return nil
end

local function findGO(mapObj, entry)
    if mapObj == nil then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local go = player:FindNearestGameObject(entry, GO_SEARCH_DIST)
        if go then
            return go
        end
    end
    return nil
end

local function castOnPlayers(mapObj, spell)
    if mapObj == nil then
        return
    end
    local players = mapObj:GetPlayers()
    if not players then
        return
    end
    for _, player in ipairs(players) do
        player:CastSpell(player, spell, true)
    end
end

-- C++ InstanceScript::UpdateDoorState (InstanceScript.cpp:203-205):
-- PASSAGE opens iff its boss is DONE. Reads the latched flag
-- (no boss-state-change event exists); idempotent across
-- re-fires via the tracked applied state.
local function updateDoors(instanceData, mapObj)
    local doorsOpen = instanceData.nethekurseDone == true
    if instanceData.nethekurseDoorOpen ~= doorsOpen then
        local door1 = findGO(mapObj, GO_GRAND_WARLOCK_CHAMBER_DOOR_1)
        local door2 = findGO(mapObj, GO_GRAND_WARLOCK_CHAMBER_DOOR_2)
        if door1 and door2 then
            local state = GO_STATE_READY
            if doorsOpen then
                state = GO_STATE_ACTIVE
            end
            door1:SetGoState(state)
            door2:SetGoState(state)
            instanceData.nethekurseDoorOpen = doorsOpen
        end
    end
end

-- C++ OnGameObjectCreate (instance_shattered_halls.cpp:78-80 via
-- InstanceScript::OnGameObjectCreate): a door creation
-- re-evaluates its state. Fires only for dynamic spawns in Go;
-- the static-spawn path is the OnPlayerEnter apply below.
RegisterMapEvent(MAP_SHATTERED_HALLS, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    if go:GetEntry() == GO_GRAND_WARLOCK_CHAMBER_DOOR_1 or go:GetEntry() == GO_GRAND_WARLOCK_CHAMBER_DOOR_2 then
        updateDoors(instanceData, mapObj)
    end
end)

-- C++ OnPlayerEnter (instance_shattered_halls.cpp:82-97):
-- latch the team from the first player, strip the executioner
-- aura (39291 triggered), then apply the staged aura
-- (39288/39289/39290 by executed count) with the remaining
-- timer as its duration (player:GetAura + SetDuration,
-- lua_player.go:2667 + lua_aura.go:452). No timer or no
-- executioner yet -> return after the strip.
RegisterMapEvent(MAP_SHATTERED_HALLS, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if not instanceData.team then
        instanceData.team = player:GetTeam()
    end
    player:CastSpell(player, SPELL_REMOVE_KARGATH_EXECUTIONER, true)
    updateDoors(instanceData, mapObj)
    if not instanceData.executionTimer or instanceData.executionTimer == 0 then
        return
    end
    if not findCreature(mapObj, NPC_SHATTERED_EXECUTIONER) then
        return
    end
    local executed = instanceData.prisonersExecuted or 0
    local spell = EXECUTIONER_SPELLS[executed + 1]
    if not spell then
        return
    end
    player:AddAura(spell)
    local aura = player:GetAura(spell)
    if aura then
        aura:SetDuration(instanceData.executionTimer)
    end
end)

-- C++ Update (instance_shattered_halls.cpp:229-266) + the
-- SetBossState-override DONE leg (:140-145) + the OnCreatureCreate
-- start leg (:124-131), all driven by the OnUpdate poll
-- (event 3, diff in ms): latch Nethekurse DONE, latch
-- executioner DONE, start the timer when the executioner
-- first resolves (heroic only), count the timer down and
-- advance the stages.
RegisterMapEvent(MAP_SHATTERED_HALLS, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    updateDoors(instanceData, mapObj)
    -- Latch Nethekurse DONE from the boss-state store
    -- (boss_nethekurse.lua writes SetBossState(0, 3) on death).
    if not instanceData.nethekurseDone then
        local nethekurse = findCreature(mapObj, NPC_GRAND_WARLOCK_NETHEKURSE)
        if nethekurse and nethekurse:GetBossState(DATA_NETHEKURSE) == BOSS_STATE_DONE then
            instanceData.nethekurseDone = true
            updateDoors(instanceData, mapObj)
        end
    end
    -- Latch executioner DONE from the boss-state store
    -- (boss_shattered_executioner.lua writes SetBossState(3, 3)
    -- on death): strip the aura from all players, stop the timer.
    if instanceData.executionTimer and instanceData.executionTimer ~= 0 and not instanceData.executionerDone then
        local executioner = findCreature(mapObj, NPC_SHATTERED_EXECUTIONER)
        if executioner and executioner:GetBossState(DATA_SHATTERED_EXECUTIONER) == BOSS_STATE_DONE then
            instanceData.executionerDone = true
            instanceData.executionTimer = 0
            castOnPlayers(mapObj, SPELL_REMOVE_KARGATH_EXECUTIONER)
            return
        end
    end
    -- Start leg: first tick the executioner resolves, heroic only.
    if not instanceData.executionerStarted then
        if mapObj:IsHeroic() then
            local executioner = findCreature(mapObj, NPC_SHATTERED_EXECUTIONER)
            if executioner then
                instanceData.executionerStarted = true
                instanceData.prisonersExecuted = 0
                instanceData.executionTimer = EXECUTION_TIMER_INITIAL
                castOnPlayers(mapObj, SPELL_KARGATH_EXECUTIONER_1)
            end
        end
        return
    end
    -- Countdown leg.
    if not instanceData.executionTimer or instanceData.executionTimer == 0 then
        return
    end
    if instanceData.executionTimer > diff then
        instanceData.executionTimer = instanceData.executionTimer - diff
        return
    end
    -- C++ Update expiry: strip, advance executed, stage the next
    -- timer, write SetData(DATA_PRISONERS_EXECUTED) to the
    -- executioner (creature:SetData store — the executioner file
    -- reads it via GetData) and fire the ev38 relay so its
    -- SetData machine runs once per write (lua_creature_events.go:792).
    castOnPlayers(mapObj, SPELL_REMOVE_KARGATH_EXECUTIONER)
    local executed = (instanceData.prisonersExecuted or 0) + 1
    instanceData.prisonersExecuted = executed
    if executed == 1 then
        castOnPlayers(mapObj, SPELL_KARGATH_EXECUTIONER_2)
        instanceData.executionTimer = EXECUTION_TIMER_STAGE_2
    elseif executed == 2 then
        castOnPlayers(mapObj, SPELL_KARGATH_EXECUTIONER_3)
        instanceData.executionTimer = EXECUTION_TIMER_STAGE_3
    else
        instanceData.executionTimer = 0
    end
    local executioner = findCreature(mapObj, NPC_SHATTERED_EXECUTIONER)
    if executioner then
        executioner:SetData(DATA_PRISONERS_EXECUTED, executed)
        executioner:DoAction(ACTION_PRISONERS_EXECUTED)
    end
end)
