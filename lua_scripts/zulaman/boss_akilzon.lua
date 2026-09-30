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
-- storm 43648 one-shot 60s on a random alive player within 50 yd, then a
-- 1s storm-sequence tick for 10 ticks with escalating zap 43137 casts
-- (C++ 800*2^(n-2) damage to units beyond 6 yd of the cloud); berserk
-- 45078 one-shot 10min with Talk(SAY_ENRAGE) and 10min repeat.
-- The soaring eagle (24858) swoops a random alive player with eagle swoop
-- 44732 (triggered) every {5s,10s} while in combat.
-- Deviations from C++: no summon model — the storm cloud SummonTrigger,
-- the zap visual triggers, and the eight soaring-eagle spawns never
-- exist, so their casts/spawns are skipped and the storm's zap damage is
-- approximated by the boss casting 43137 on random alive players (the C++
-- 6-yd cloud-exclusion check and the bp0 escalation have no bridge —
-- CastSpell carries no spell-value modifiers, and there is no Go summon
-- model); the EVENT_SUMMON_EAGLES arm keeps its Talk(SAY_SUMMON) only.
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
local SAY_SUMMON = 1
local SAY_ENRAGE = 3
local SAY_KILL = 4
local SAY_DEATH = 5

local SPELL_STATIC_DISRUPTION = 43622
local SPELL_CALL_LIGHTNING = 43661
local SPELL_GUST_OF_WIND = 43621
local SPELL_ELECTRICAL_STORM = 43648
local SPELL_BERSERK = 45078
local SPELL_ZAP = 43137
local SPELL_CLOUD_VISUAL = 44007
local SPELL_EAGLE_SWOOP = 44732

local timers = {}
local eagleTimers = {}
local state = {}

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

-- C++ EVENT_STORM_SEQUENCE / HandleStormSequence: the storm cloud starts
-- at count 1; each 1s tick increments, dealing escalating zap damage while
-- 1 < count < 10, and ends the storm past 10 (weather back to fine, rain
-- off, eagles summoned 5s later).
local function stormTick(creature, guid)
    local st = state[guid]
    if not st or not st.stormActive then
        return
    end
    st.count = st.count + 1
    if st.count > 10 then
        st.stormActive = false
        st.count = 0
        -- C++ HandleStormSequence end: ScheduleEvent(EVENT_SUMMON_EAGLES,
        -- 5s); the eagle summons have no bearer, but the Talk arm stays.
        creature:Talk(SAY_SUMMON)
        return
    end
    if st.count > 1 then
        -- C++ bp0 = 800 * 2^(count-2) zaps every unit beyond 6 yd of the
        -- cloud; no spellmod bridge and no cloud position, so approximate
        -- with up to three 43137 zap casts on random alive players.
        local candidates = alivePlayersInRange(creature, 100)
        for i = 1, math.min(3, #candidates) do
            creature:CastSpell(candidates[math.random(#candidates)], SPELL_ZAP)
        end
    end
    schedule(timers, guid, "stormtick", 1000, function()
        stormTick(creature, guid)
    end)
end

-- C++ EVENT_ELECTRICAL_STORM: random alive player within 50 yd (evade if
-- none); target casts 44007 on self (no player CastSpell bridge —
-- skipped), boss DoCast(target, 43648); cloud SummonTrigger skipped (no
-- summon model); storm sequence starts at count 1. Repeats 60s and
-- re-arms EVENT_RAIN (weather has no bridge — skipped).
local function onElectricalStorm(creature, guid)
    local target = randomPlayerInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_ELECTRICAL_STORM)
        local st = state[guid]
        if not st then
            st = {}
            state[guid] = st
        end
        st.stormActive = true
        st.count = 1
        schedule(timers, guid, "stormtick", 1000, function()
            stormTick(creature, guid)
        end)
    end
    schedule(timers, guid, "storm", 60000, function()
        onElectricalStorm(creature, guid)
    end)
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
    local st = state[guid]
    if st then
        st.stormActive = false
        st.count = 0
    end
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
local function onEagleSwoop(creature, guid)
    local target = randomPlayerInRange(creature, 100)
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
