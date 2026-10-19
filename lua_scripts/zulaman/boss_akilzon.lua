-- Aki'lzon (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_akilzon.cpp
-- (boss_akilzon + npc_akilzon_eagle); zulaman.h:29 (BOSS_AKILZON = 1),
-- zulaman.h:46 (NPC_AKILZON = 23574).
-- Creature entries: 23574 Aki'lzon (C++ ScriptName "boss_akilzon" per
-- AddSC_boss_akilzon); 24858 Soaring Eagle (C++ ScriptName
-- "npc_akilzon_eagle").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: on aggro Talk(SAY_AGGRO) and arm static disruption 43622
-- {10s,20s} first / {10s,18s} repeat on a random alive player (C++
-- Random,1) with victim fallback; gust of wind 43621 {20s,30s} /
-- {20s,30s} repeat on a random alive player with victim fallback; call
-- lightning 43661 {10s,20s} / {12s,17s} repeat on the victim; electrical
-- storm 43648 every 60s on a random alive player within 50 yd (the storm
-- zap sequence never runs — see the HandleStormSequence note below);
-- berserk 45078 one-shot 10min with Talk(SAY_ENRAGE) and 10min repeat.
-- The soaring eagle (24858) swoops a random threat-list unit with
-- eagle swoop 44732 (triggered) every {5s,10s} while in combat (C++
-- SelectTarget(Random, 0), playerOnly FALSE — pets/totems eligible).
-- Deviations from C++: LOCAL-TREE DEFECT — boss_akilzon.cpp never
-- schedules EVENT_STORM_SEQUENCE (the initial arm upstream TC has in the
-- ELECTRICAL_STORM case is missing from this tree; the only schedule is
-- the unreachable tail of HandleStormSequence itself), so
-- HandleStormSequence and the EVENT_SUMMON_EAGLES arm are dead code in
-- the C++: no storm zaps, no 5s eagle summon, no SAY_SUMMON. The Lua
-- matches the tree — the storm is the 43648 cast + 60s re-arm only. If
-- the schedule is restored in the C++, the zap sequence (8s first tick,
-- 1s ticks, zaps at counts 2..9, re-looping tail) needs re-adding here.
-- No summon model — the storm cloud SummonTrigger, the zap visual
-- triggers, and the eight soaring-eagle spawns never exist, so their
-- casts/spawns are skipped.
-- No weather bridge — SetWeather(WEATHER_STATE_HEAVY_RAIN/FINE) and the
-- whole EVENT_RAIN arm have no bearer and are skipped. No movement/gravity
-- model — MovementInform never fires, so the eagle's MovePoint swoop run
-- is modeled as a direct timer cast; SetDisableGravity and speed-rate
-- arms are skipped. The 44007 cloud-visual self-cast on the storm target
-- has no bearer (CastSpell is on the creature object only, not players).
-- The C++ EnterEvadeMode arms (storm target gone) have no bridge, so a
-- missing storm target is a no-op. No instance-script model — admission
-- only via the luaBossAI shim. No threat model — "random,1" picks any
-- random alive player and AddThreat for eagles is skipped. The commented-
-- out static-disruption AOE visual arm in C++ is not modeled (dead code).

local ENTRY_AKILZON = 23574
local ENTRY_SOARING_EAGLE = 24858

local SAY_AGGRO = 0
local SAY_ENRAGE = 3
local SAY_KILL = 4
local SAY_DEATH = 5

local SPELL_STATIC_DISRUPTION = 43622
local SPELL_CALL_LIGHTNING = 43661
local SPELL_GUST_OF_WIND = 43621
local SPELL_ELECTRICAL_STORM = 43648
local SPELL_BERSERK = 45078
local SPELL_CLOUD_VISUAL = 44007
local SPELL_EAGLE_SWOOP = 44732

local timers = {}
local eagleTimers = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_STATIC_DISRUPTION: SelectTarget(Random, 1) with victim
-- fallback; DoCast(target, STATIC_DISRUPTION, false). Repeat {10s,18s}.
local function onStaticDisruption(creature, guid)
    local target = randomPlayerInRange(creature, 100) or creature:GetVictim()
    if target then
        creature:CastSpell(target, SPELL_STATIC_DISRUPTION)
    end
    schedule(timers, guid, "static", math.random(10000, 18000), function()
        onStaticDisruption(creature, guid)
    end)
end

-- C++ EVENT_GUST_OF_WIND: SelectTarget(Random, 1) with victim fallback;
-- DoCast(target, GUST_OF_WIND). Repeat {20s,30s}.
local function onGustOfWind(creature, guid)
    local target = randomPlayerInRange(creature, 100) or creature:GetVictim()
    if target then
        creature:CastSpell(target, SPELL_GUST_OF_WIND)
    end
    schedule(timers, guid, "gust", math.random(20000, 30000), function()
        onGustOfWind(creature, guid)
    end)
end

-- C++ EVENT_CALL_LIGHTNING: DoCastVictim(CALL_LIGHTNING). Repeat {12s,17s}.
local function onCallLightning(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(nil, SPELL_CALL_LIGHTNING)
    end
    schedule(timers, guid, "lightning", math.random(12000, 17000), function()
        onCallLightning(creature, guid)
    end)
end

-- C++ EVENT_ELECTRICAL_STORM: random alive player within 50 yd; nil
-- target returns out of the case BEFORE the 60s re-arm (EnterEvadeMode
-- in C++ — no evade bridge, so the storm simply stops). Target casts
-- 44007 on self (no player CastSpell bridge — skipped), boss
-- DoCast(target, 43648) non-triggered; cloud SummonTrigger skipped (no
-- summon model). The EVENT_STORM_SEQUENCE arm is never scheduled in this
-- C++ tree (see the local-tree defect note above), so no sequence runs.
-- Repeats 60s and re-arms EVENT_RAIN (weather has no bridge — skipped)
-- on the found-target leg only.
local function onElectricalStorm(creature, guid)
    local target = randomPlayerInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_ELECTRICAL_STORM)
        schedule(timers, guid, "storm", 60000, function()
            onElectricalStorm(creature, guid)
        end)
    end
end

-- C++ EVENT_ENRAGE: Talk(SAY_ENRAGE); DoCast(me, BERSERK, true); repeat
-- 10min.
local function onEnrage(creature, guid)
    creature:Talk(SAY_ENRAGE)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(timers, guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
end

local function akilzonResetState(guid)
    cancelTimers(timers, guid)
end

local function akilzonEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    akilzonResetState(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith: 10-20s static, 20-30s gust, 10-20s lightning,
    -- 60s storm, 10min enrage. The 47-52s rain arm is weather-only and
    -- skipped (no weather bridge).
    schedule(timers, guid, "static", math.random(10000, 20000), function()
        onStaticDisruption(creature, guid)
    end)
    schedule(timers, guid, "gust", math.random(20000, 30000), function()
        onGustOfWind(creature, guid)
    end)
    schedule(timers, guid, "lightning", math.random(10000, 20000), function()
        onCallLightning(creature, guid)
    end)
    schedule(timers, guid, "storm", 60000, function()
        onElectricalStorm(creature, guid)
    end)
    schedule(timers, guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
end

local function akilzonLeaveCombat(event, creature)
    akilzonResetState(creature:GetGUID())
end

local function akilzonTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function akilzonDied(event, creature, killer)
    akilzonResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function akilzonReset(event, creature)
    akilzonResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AKILZON, 1, akilzonEnterCombat)
RegisterCreatureEvent(ENTRY_AKILZON, 2, akilzonLeaveCombat)
RegisterCreatureEvent(ENTRY_AKILZON, 3, akilzonTargetDied)
RegisterCreatureEvent(ENTRY_AKILZON, 4, akilzonDied)
RegisterCreatureEvent(ENTRY_AKILZON, 23, akilzonReset)

-- npc_akilzon_eagle: the swoop machine. C++ drives it through movement
-- (MovePoint to the target's contact point, then MovementInform casts
-- EAGLE_SWOOP triggered on the recorded target and re-arms {5s,10s}).
-- MovementInform never fires without a movement model, so the swoop is
-- modeled as a direct timer cast on a random alive player every {5s,10s}.
-- MoveInLineOfSight is suppressed in C++ (empty override) and Reset's
-- SetDisableGravity has no bridge; JustEngagedWith's DoZoneInCombat has
-- no bridge.
-- C++ npc_akilzon_eagle UpdateAI (boss_akilzon.cpp:439):
-- SelectTarget(Random, 0) — playerOnly FALSE (2-arg form) — picks from
-- the eagle's threat list, not players only. The target feeds the
-- MovePoint destination AND the TargetGUID that MovementInform casts
-- SPELL_EAGLE_SWOOP on; the swoop stays a direct timer cast (no
-- movement model). Threat-list pick, no Player filter (the midnight
-- randomTargetAnywhere precedent — C++ has no distance filter here).
local function randomThreatTargetEagle(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

local function onEagleSwoop(creature, guid)
    local target = randomThreatTargetEagle(creature)
    if target then
        creature:CastSpell(target, SPELL_EAGLE_SWOOP, true)
    end
    schedule(eagleTimers, guid, "swoop", math.random(5000, 10000), function()
        onEagleSwoop(creature, guid)
    end)
end

local function eagleEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(eagleTimers, guid)
    schedule(eagleTimers, guid, "swoop", math.random(5000, 10000), function()
        onEagleSwoop(creature, guid)
    end)
end

local function eagleLeaveCombat(event, creature)
    cancelTimers(eagleTimers, creature:GetGUID())
end

local function eagleDied(event, creature, killer)
    cancelTimers(eagleTimers, creature:GetGUID())
end

local function eagleReset(event, creature)
    cancelTimers(eagleTimers, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SOARING_EAGLE, 1, eagleEnterCombat)
RegisterCreatureEvent(ENTRY_SOARING_EAGLE, 2, eagleLeaveCombat)
RegisterCreatureEvent(ENTRY_SOARING_EAGLE, 4, eagleDied)
RegisterCreatureEvent(ENTRY_SOARING_EAGLE, 23, eagleReset)
