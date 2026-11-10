-- Mimiron emergency fire bot — Lua port of
-- npc_mimiron_emergency_fire_bot in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1341-1400).
-- Entry: 34147 = NPC_EMERGENCY_FIRE_BOT (ulduar.h:125).
-- C++ constructor: REACT_PASSIVE, isWaterSprayReady = true,
-- moveNew = true. C++ Reset: schedules EVENT_WATER_SPRAY at 7s and
-- sets both flags true. C++ UpdateAI: the event map only ticks while
-- !isWaterSprayReady; on EVENT_WATER_SPRAY the bot becomes ready
-- again and reschedules 7-9s. The 7s Reset event's timer is frozen
-- while the bot is ready, so the first re-arm after a spray is
-- exactly 7s and every later one is 7-9s — the poll below models
-- that: a 1s check (the kri 2s-poll idiom) starts the re-arm timer
-- when the spray flag reads 0, first re-arm 7000ms, later
-- math.random(7000, 9000).
-- The flags live in the creature ScriptData store (creature
-- GetData/SetData, lua_creature_events.go:792-826); the fire-search
-- spell script (spell_mimiron_fire_search.lua) reads DATA_WATERSPRAY
-- and DATA_MOVE_NEW off the caster and clears them on a spray, which
-- is what starts the re-arm cycle here. C++ SetData(DATA_WATERSPRAY,
-- x) forces the flag false whatever x is; the Go store holds the
-- written 0, which reads back false — same observable state.

local NPC_EMERGENCY_FIRE_BOT = 34147 -- ulduar.h:125

local DATA_WATERSPRAY = 5 -- boss_mimiron.cpp:191
local DATA_MOVE_NEW = 6 -- boss_mimiron.cpp:192

local REACT_PASSIVE = 0 -- boss_aerial_command_unit.lua precedent

local timers = {}
local botState = {}

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

-- C++ UpdateAI (boss_mimiron.cpp:1379-1394): while the spray flag
-- is clear, the pending EVENT_WATER_SPRAY counts down; on fire it
-- sets the flag and reschedules 7-9s. The poll re-arms only when no
-- re-arm is pending — rescheduling every poll tick would push the
-- fire out forever.
local function onPoll(creature, guid)
    local state = botState[guid]
    if state == nil then
        return
    end
    if creature:GetData(DATA_WATERSPRAY) == 0 and not state.rearmPending then
        state.rearmPending = true
        local delay = 7000
        if not state.firstRearm then
            delay = math.random(7000, 9000)
        end
        state.firstRearm = false
        schedule(guid, "rearm", delay, function()
            local s = botState[guid]
            if s ~= nil then
                s.rearmPending = false
            end
            creature:SetData(DATA_WATERSPRAY, 1)
        end)
    end
    schedule(guid, "poll", 1000, function()
        onPoll(creature, guid)
    end)
end

-- C++ constructor + Reset (boss_mimiron.cpp:1349-1355, 1372-1377):
-- passive, both flags true, EVENT_WATER_SPRAY scheduled at 7s.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetData(DATA_WATERSPRAY, 1)
    creature:SetData(DATA_MOVE_NEW, 1)
    botState[guid] = { firstRearm = true, rearmPending = false }
    schedule(guid, "poll", 1000, function()
        onPoll(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    botState[guid] = nil
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    botState[guid] = nil
end

RegisterCreatureEvent(NPC_EMERGENCY_FIRE_BOT, 5, onReset)
RegisterCreatureEvent(NPC_EMERGENCY_FIRE_BOT, 23, onReset)
RegisterCreatureEvent(NPC_EMERGENCY_FIRE_BOT, 2, onLeaveCombat)
RegisterCreatureEvent(NPC_EMERGENCY_FIRE_BOT, 4, onDied)
