-- Azuremyst Isle: Geezle — Lua port of src/server/scripts/Kalimdor/
-- zone_azuremyst_isle.cpp (npc_geezleAI : public ScriptedAI; registered
-- in AddSC_azuremyst_isle()).
-- Entry: 17318 'Geezle' (TDB-verified: creature_template ScriptName
-- 'npc_geezle', faction 47, level 10; 0 static spawns in the
-- 2026-09-09 TDB creature table — no C++ summon site found either,
-- so the live spawn source is unconfirmed in this DB; the script
-- binds by entry when the creature exists).
-- The 10-step Spark machine: Reset -> StartEvent summons Spark
-- (17243) at SparkPos and runs the timed dialogue/move/quest
-- chain on a SayTimer pump; step 10 despawns both and re-arms the
-- naga flag. C++ UpdateAI (:589-596): SayTimer countdown, NextStep
-- (Step++) while EventStarted; NextStep returns 99999999 when the
-- Spark is gone or the chain is done (effectively stopped).
-- C++ Reset (:496-500): Initialize() + StartEvent(). C++ ctor
-- (:476-479): Initialize() only — no StartEvent. Event 23 fires
-- on spawn in Go, so onReset == the C++ Reset at spawn (minigob
-- precedent); it also fires on evade (C++ Reset -> StartEvent
-- re-summons, same) and before ev4 on the kill path (Spark-file
-- precedent) — the IsDead guard below skips StartEvent there, since
-- C++ never re-summons on death.
-- C++ JustEngagedWith (:502) is an empty override: no ev1 arm.
-- No C++ JustDied arm: ev4 drains the timers (death halts UpdateAI,
-- minigob precedent); the Spark is NOT despawned on Geezle's death
-- (C++ despawns it only at step 10).
-- Step table (zone_azuremyst_isle.cpp:515-571), delays C++-exact:
--   start 8000; 0 MovePoint both -> 9000; 1 DespawnNagaFlag(true) +
--   Spark emote 7 -> 1000; 2 Geezle Talk 0 to Spark + face each
--   other -> 5000; 3 Spark Talk 3 -> 7000; 4 Spark Talk 4 -> 8000;
--   5 Geezle Talk 1 -> 8000; 6 Spark Talk 5 -> 9000; 7 Spark Talk 6
--   -> 8000; 8 Geezle Talk 2 -> 2000; 9 MoveTargetedHome + Spark
--   MovePoint(SparkPos) + CompleteQuest -> 9000; 10 Spark
--   DisappearAndDie + DespawnNagaFlag(false) + Geezle
--   DisappearAndDie -> stop (no re-schedule; C++ returns 99999999).
-- CompleteQuest (:574-586): players within 50yd with quest 9531
-- INCOMPLETE and aura 30298 (tree disguise) get
-- KilledMonsterCredit(NPC_SPARK).
-- DespawnNagaFlag (:588-601): GO 181694 'Naga Flag' within 100yd ->
-- SetLootState(GO_JUST_DEACTIVATED) or Respawn().
-- Bridges (all live, verified this tick):
-- SummonCreature (lua_creature_events.go:4460); GetCreatureByGUID
-- (lua_creature_events.go:4696, ObjectAccessor::GetCreature twin,
-- low-GUID convention, razorgore precedent); MoveTo(id, x, y, z)
-- (lua_creature_events.go:5769, MovePoint twin, id 0 =
-- POINT_MOTION_TYPE); GetHomePosition (lua_creature_events.go:7417,
-- MoveTargetedHome twin = MoveTo home xyz); SetFacingToObject
-- (:5673); Talk(id[, target]) (:1402 — whisper rows drop a
-- non-player target like C++, creature_text.go:135; all rows here
-- are SAY 12 / emote 16, range 0, TDB-verified for 17318 groups
-- 0-2 and 17243 groups 3-7); RemoveFlag(82, 1)
-- (unitFieldNPCFlags = 82, creatures.go:24; UNIT_NPC_FLAG_GOSSIP
-- 0x1, UnitDefines.h:186; majordomo precedent); Despawn
-- (:3659, DisappearAndDie twin — despawnCreatureMotion);
-- GetGameObjectsInGrid(entry, dist) (:7303, arlokk/mar'li
-- precedent); GO SetLootState (lua_objects.go:4558, value <= 3) +
-- Respawn (:4805); GetPlayersInRange(range, hostile, dead)
-- (lua_worldobject_range.go:608); player:GetQuestStatus
-- (lua_player.go:4932), HasAura (:3019), KilledMonsterCredit
-- (:5297); QUEST_STATUS_INCOMPLETE = 3 (in-tree Lua-local const
-- precedent); creature:GetAreaId is NOT needed here (the Spark's
-- own tree-event latch is its file's business).
-- Deviations / translations:
-- - TempSummonType: Go's SummonCreature uses vanilla 0-based
--   numbering (lua_creature_events.go:4440-4446: 5 =
--   CORPSE_TIMED_DESPAWN); this C++ fork's ObjectDefines.h numbers
--   the enum 1-based (CORPSE_TIMED_DESPAWN = 6). Pass 5, not 6.
-- - SummonCreature o: C++ passes SparkPos o = 0.0f; Go treats o
--   0 as "summoner's orientation" (Object.cpp:2061-2062 twin), so
--   the Spark spawns facing Geezle instead of 0.0.
-- - setActive(true) has no bridge; CreateLuaEvent timers are
--   global and proximity-independent (minigob precedent), so the
--   machine runs the same without it.
-- - DisappearAndDie has no bridge; Despawn() is the twin (the
--   summoned Spark is CORPSE_TIMED_DESPAWN 1s, so its corpse
--   vanishes in 1s either way; Geezle respawns on spawntime).
-- - CompleteQuest uses GetPlayersInRange(50, 0, 0): dead = 0
--   (both) — C++ AnyPlayerInObjectRangeCheck has no alive gate.
local GEEZLE_ENTRY = 17318
local NPC_SPARK = 17243

local QUEST_TREES_COMPANY = 9531
local SPELL_TREE_DISGUISE = 30298
local GO_NAGA_FLAG = 181694
local GO_JUST_DEACTIVATED = 3 -- GameObject.h:70-73 (NOT_READY 0, READY 1, ACTIVATED 2, JUST_DEACTIVATED 3)

local GEEZLE_SAY_1 = 0
local GEEZLE_SAY_4 = 1
local GEEZLE_SAY_7 = 2
local SPARK_SAY_2 = 3
local SPARK_SAY_3 = 4
local SPARK_SAY_5 = 5
local SPARK_SAY_6 = 6
local EMOTE_SPARK = 7 -- '%s picks up the naga flag.' (creature_text 17243 group 7)

local SUMMON_CORPSE_TIMED_DESPAWN = 5 -- Go numbering; C++ fork enum value is 6 (see header)
local SUMMON_DESPAWN_MS = 1000

local FIELD_UNIT_NPC_FLAGS = 82
local FLAG_GOSSIP = 1 -- UNIT_NPC_FLAG_GOSSIP

local SPARK_X, SPARK_Y, SPARK_Z, SPARK_O = -5029.91, -11291.79, 8.096, 0.0
local STEP0_SPARK_X, STEP0_SPARK_Y, STEP0_SPARK_Z = -5080.70, -11253.61, 0.56
local STEP0_GEEZLE_X, STEP0_GEEZLE_Y, STEP0_GEEZLE_Z = -5092.26, -11252, 0.71

local QUEST_STATUS_INCOMPLETE = 3

local timers = {}
local state = {} -- guid -> { sparkGuidLow, step }

-- Forward declarations: the step chain references itself in
-- CreateLuaEvent closures; a later `local function` would bind the
-- closure to a nil global (malown time-bomb). Plain assignments below.
local onStep

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

-- C++ DespawnNagaFlag (:588-601): GO 181694 within 100yd ->
-- GO_JUST_DEACTIVATED, or Respawn() when the flag comes back.
local function despawnNagaFlag(creature, despawn)
    for _, go in ipairs(creature:GetGameObjectsInGrid(GO_NAGA_FLAG, 100)) do
        if despawn then
            go:SetLootState(GO_JUST_DEACTIVATED)
        else
            go:Respawn()
        end
    end
end

-- C++ CompleteQuest (:574-586): quest-9531-INCOMPLETE + tree-disguise
-- players within 50yd get KilledMonsterCredit(NPC_SPARK).
local function completeQuest(creature)
    for _, player in ipairs(creature:GetPlayersInRange(50, 0, 0)) do
        if player:GetQuestStatus(QUEST_TREES_COMPANY) == QUEST_STATUS_INCOMPLETE and player:HasAura(SPELL_TREE_DISGUISE) then
            player:KilledMonsterCredit(NPC_SPARK)
        end
    end
end

-- C++ NextStep (:515-571). Returns the delay to the next step, or
-- nil when the chain stops (Spark gone, or step 10 done — C++
-- returns 99999999, effectively stopped).
local function nextStep(creature, guid, step)
    local st = state[guid]
    if not st then
        return nil
    end
    local spark = creature:GetCreatureByGUID(st.sparkGuidLow)
    if not spark then
        return nil
    end
    if step == 0 then
        spark:MoveTo(0, STEP0_SPARK_X, STEP0_SPARK_Y, STEP0_SPARK_Z)
        creature:MoveTo(0, STEP0_GEEZLE_X, STEP0_GEEZLE_Y, STEP0_GEEZLE_Z)
        return 9000
    elseif step == 1 then
        despawnNagaFlag(creature, true)
        spark:Talk(EMOTE_SPARK)
        return 1000
    elseif step == 2 then
        creature:Talk(GEEZLE_SAY_1, spark)
        spark:SetFacingToObject(creature)
        creature:SetFacingToObject(spark)
        return 5000
    elseif step == 3 then
        spark:Talk(SPARK_SAY_2)
        return 7000
    elseif step == 4 then
        spark:Talk(SPARK_SAY_3)
        return 8000
    elseif step == 5 then
        creature:Talk(GEEZLE_SAY_4, spark)
        return 8000
    elseif step == 6 then
        spark:Talk(SPARK_SAY_5)
        return 9000
    elseif step == 7 then
        spark:Talk(SPARK_SAY_6)
        return 8000
    elseif step == 8 then
        creature:Talk(GEEZLE_SAY_7, spark)
        return 2000
    elseif step == 9 then
        local hx, hy, hz = creature:GetHomePosition()
        creature:MoveTo(0, hx, hy, hz)
        spark:MoveTo(0, SPARK_X, SPARK_Y, SPARK_Z)
        completeQuest(creature)
        return 9000
    elseif step == 10 then
        spark:Despawn()
        despawnNagaFlag(creature, false)
        creature:Despawn()
        return nil
    end
    return nil
end

onStep = function(creature, guid)
    local st = state[guid]
    if not st then
        return
    end
    local delay = nextStep(creature, guid, st.step)
    st.step = st.step + 1
    if delay then
        schedule(guid, "step", delay, function() onStep(creature, guid) end)
    else
        cancelTimers(guid)
        state[guid] = nil
    end
end

-- C++ StartEvent (:504-513): summon Spark, clear its gossip flag,
-- first step in 8s.
local function startEvent(creature, guid)
    local spark = creature:SummonCreature(NPC_SPARK, SPARK_X, SPARK_Y, SPARK_Z, SPARK_O, SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
    if spark then
        state[guid] = { sparkGuidLow = spark:GetGUIDLow(), step = 0 }
        spark:RemoveFlag(FIELD_UNIT_NPC_FLAGS, FLAG_GOSSIP)
    else
        state[guid] = { sparkGuidLow = 0, step = 0 }
    end
    schedule(guid, "step", 8000, function() onStep(creature, guid) end)
end

-- C++ Reset (:496-500): Initialize() + StartEvent(). The IsDead
-- guard covers the kill-path ev23-before-ev4 fire (Spark-file
-- precedent): C++ never re-summons on death.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    if creature:IsDead() then
        return
    end
    startEvent(creature, guid)
end

-- C++ death halts UpdateAI; respawn re-seeds via Reset.
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

RegisterCreatureEvent(GEEZLE_ENTRY, 23, onReset)
RegisterCreatureEvent(GEEZLE_ENTRY, 4, onDied)
