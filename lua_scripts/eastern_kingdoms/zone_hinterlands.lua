-- Hinterlands — Lua port of
-- src/server/scripts/EasternKingdoms/zone_hinterlands.cpp
-- (npc_rinji, quest 2742 "Rin'ji is Trapped!" escort).
-- Entry: the C++ sources carry NO NPC_ constant for Rinji
-- (the script binds by DB ScriptName), so the entry is cited
-- from www.wowhead.com/classic/npc=7780/rinji (calvin 6784
-- precedent): Rin'ji <Witherbark Troll>, level 46, Hinterlands;
-- its Starts list shows "Rin'ji is Trapped!" (quest 2742).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 19
-- OnJustSummonedCreature, 1 OnEnterCombat, 4 OnDied, 23 OnReset.
-- Escort pattern follows npc_earthmender_wilda.lua:
-- StartEscort(player) + MoveWaypoint(); the waypoint_data rows
-- for Rinji's path live in the deployment world DB (the repo
-- world.db placeholder carries none — loadCreaturePathID falls
-- back to the spawn GUID, so a live DB without the rows leaves
-- MoveWaypoint a no-op); the waypoint walk loops the DB path at
-- its end (Go NextIdx wraps — no end-of-path idle twin).
-- Ported arms (C++-exact):
-- - OnQuestAccept(2742): FindNearestGameObject(GO_RINJI_CAGE
--   142036, INTERACTION_DISTANCE 5.0) -> UseDoorOrButton, then
--   StartEscort(player) + MoveWaypoint() (the C++ Start(false,
--   false, ...) walk/instant-respawn flags have no Go twin — the
--   walk rides the per-node DB move_type, wilda precedent).
-- - WaypointReached (event 6, DB point via
--   creature:GetCurrentWaypointId()): wp1 Talk(SAY_RIN_FREE 0,
--   player); wp7 DoSpawnAmbush(true); wp13 DoSpawnAmbush(false);
--   wp17 Talk(SAY_RIN_COMPLETE 2, player) + GroupEventHappens
--   (2742) + post-event chain (postEventCount 1).
-- - DoSpawnAmbush: 1 ranger (2694) + 2 outrunners (2691) at
--   AmbushSpawn[spawnId], TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN
--   (type 1) 1min; JustSummoned sets walk off + MoveTo(0,
--   AmbushMoveTo[spawnId]).
-- - JustEngagedWith (escorting only): first outrunner attacker
--   Talks SAY_RIN_BY_OUTRUNNER (0) once (latch); 1-in-4
--   Talk(SAY_RIN_HELP 1) (rand32()%4, wilda %5 precedent).
-- - UpdateEscortAI post-event chain: 3s timer, victimless ticks
--   only (the pump re-polls every 1s while engaged — the C++
--   timer freezes in combat); Talk PROGRESS_1 (3) then
--   PROGRESS_2 (4) to the escort player; no escort player ->
--   DespawnOrUnsummon (creature:Despawn()).
-- Deliberate deviation: SetRun() at wp17 has no bridge (no
-- SetRun method on the creature surface) — the post-event talks
-- still fire on the 3s chain; only the walk/run speed of the
-- run-off differs. Melee is engine-driven (DoMeleeAttackIfReady).

local ENTRY_RINJI = 7780

local QUEST_RINJI_TRAPPED = 2742
local NPC_RANGER = 2694
local NPC_OUTRUNNER = 2691
local GO_RINJI_CAGE = 142036

local SAY_RIN_BY_OUTRUNNER = 0
local SAY_RIN_FREE = 0
local SAY_RIN_HELP = 1
local SAY_RIN_COMPLETE = 2
local SAY_RIN_PROGRESS_1 = 3
local SAY_RIN_PROGRESS_2 = 4

local INTERACTION_DISTANCE = 5.0

local AMBUSH_SPAWN = {
    { 191.296204, -2839.329346, 107.388 },
    { 70.972466, -2848.674805, 109.459 },
}

local AMBUSH_MOVE_TO = {
    { 166.630386, -2824.780273, 108.153 },
    { 70.886589, -2874.335449, 116.675 },
}

local state = {}
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { spawnId = 0, byOutrunner = false, postEventCount = 0 }
        state[guid] = st
    end
    return st
end

-- C++ UpdateEscortAI post-event chain
-- (zone_hinterlands.cpp:175-201): victimless 3s ticks while
-- escorting; Talk PROGRESS_1 then PROGRESS_2; no escort player
-- -> DespawnOrUnsummon.
local function postEventTick(creature, guid)
    if creature:IsDead() or not creature:IsEscorted() then
        return
    end
    local st = state[guid]
    if not st or st.postEventCount == 0 then
        return
    end
    if creature:GetVictim() then
        schedule(guid, "postevent", 1000, function()
            postEventTick(creature, guid)
        end)
        return
    end
    local player = creature:GetEscortPlayer()
    if not player then
        creature:Despawn()
        return
    end
    if st.postEventCount == 1 then
        creature:Talk(SAY_RIN_PROGRESS_1, player)
        st.postEventCount = 2
        schedule(guid, "postevent", 3000, function()
            postEventTick(creature, guid)
        end)
    elseif st.postEventCount == 2 then
        creature:Talk(SAY_RIN_PROGRESS_2, player)
        st.postEventCount = 0
    end
end

-- C++ DoSpawnAmbush (zone_hinterlands.cpp:125-139): first=false
-- moves spawnId to 1; 1 ranger + 2 outrunners at
-- AmbushSpawn[spawnId], type 1, 60000ms.
local function doSpawnAmbush(creature, guid, first)
    local st = getState(guid)
    if not first then
        st.spawnId = 1
    end
    local spawn = AMBUSH_SPAWN[st.spawnId + 1]
    creature:SummonCreature(NPC_RANGER, spawn[1], spawn[2], spawn[3], 0, 1, 60000)
    for _ = 1, 2 do
        creature:SummonCreature(NPC_OUTRUNNER, spawn[1], spawn[2], spawn[3], 0, 1, 60000)
    end
end

-- C++ OnQuestAccept (zone_hinterlands.cpp:147-158): cage door,
-- then EscortAI::Start.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_RINJI_TRAPPED then
        return
    end
    local cage = creature:FindNearestGameObject(GO_RINJI_CAGE, INTERACTION_DISTANCE)
    if cage then
        cage:UseDoorOrButton()
    end
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ WaypointReached (zone_hinterlands.cpp:160-182).
local function onReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local guid = creature:GetGUIDLow()
    local wp = creature:GetCurrentWaypointId()
    if wp == 1 then
        creature:Talk(SAY_RIN_FREE, player)
    elseif wp == 7 then
        doSpawnAmbush(creature, guid, true)
    elseif wp == 13 then
        doSpawnAmbush(creature, guid, false)
    elseif wp == 17 then
        creature:Talk(SAY_RIN_COMPLETE, player)
        player:GroupEventHappens(QUEST_RINJI_TRAPPED, creature)
        local st = getState(guid)
        st.postEventCount = 1
        schedule(guid, "postevent", 3000, function()
            postEventTick(creature, guid)
        end)
    end
end

-- C++ JustSummoned (zone_hinterlands.cpp:141-145): walk off,
-- MovePoint to the ambush point for this spawnId.
local function onJustSummoned(event, creature, summoned)
    if not summoned then
        return
    end
    local st = getState(creature:GetGUIDLow())
    local dest = AMBUSH_MOVE_TO[st.spawnId + 1]
    summoned:SetWalk(false)
    summoned:MoveTo(0, dest[1], dest[2], dest[3])
end

-- C++ JustEngagedWith (zone_hinterlands.cpp:104-122).
local function onEnterCombat(event, creature, target)
    if not creature:IsEscorted() then
        return
    end
    local st = getState(creature:GetGUIDLow())
    if target and target:GetEntry() == NPC_OUTRUNNER and not st.byOutrunner then
        target:Talk(SAY_RIN_BY_OUTRUNNER)
        st.byOutrunner = true
    end
    if math.random(0, 3) ~= 0 then
        return
    end
    creature:Talk(SAY_RIN_HELP)
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local st = state[guid]
    if st then
        st.postEventCount = 0
    end
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

-- C++ JustAppeared (zone_hinterlands.cpp:95-101): latch + spawnId
-- reset (the EscortAI::JustAppeared leg has no Lua twin).
local function onSpawn(event, creature)
    local guid = creature:GetGUIDLow()
    state[guid] = { spawnId = 0, byOutrunner = false, postEventCount = 0 }
end

RegisterCreatureEvent(ENTRY_RINJI, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_RINJI, 6, onReachWP)
RegisterCreatureEvent(ENTRY_RINJI, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_RINJI, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_RINJI, 4, onDied)
RegisterCreatureEvent(ENTRY_RINJI, 23, onReset)
RegisterCreatureEvent(ENTRY_RINJI, 5, onSpawn)
