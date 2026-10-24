-- Flames (Ulduar, Mimiron hard-mode fire) — Lua port of
-- npc_mimiron_flamesAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1516-1561).
-- Entries (wowhead, no C++ NPC_ constant exists for these adds — the
-- cthun/telestra wowhead-only precedent, never guessed): 34363
-- "Flames (Initial)" (summoned by spell 64563, the C++
-- SPELL_SUMMON_FLAMES_INITIAL) and 34121 "Flames (Spread)"
-- (summoned by spell 64564, the C++ SPELL_SUMMON_FLAMES_SPREAD).
-- Both entries share this one AI: the AI's sole behavior is the
-- spread cast, and the spread chain requires every patch — initial
-- and spread alike — to cast it, or propagation terminates after
-- one generation. This is the frost-giant/mageguard multi-entry
-- precedent (one script name, several entries).
local NPC_MIMIRON_FLAMES_INITIAL = 34363
local NPC_MIMIRON_FLAMES_SPREAD = 34121

local SPELL_SUMMON_FLAMES_SPREAD_TRIGGER = 64562

-- GUID-keyed CreateLuaEvent timers (C++ Reset schedules the EventMap;
-- spawn/reset-started here with cancel-first idempotence — the cthun
-- precedent — since a summoned flame's Reset is the schedule point).
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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- C++ UpdateAI EVENT_SPREAD_FLAMES: DoCastAOE(64562), one-shot
-- (no re-arm in C++).
local function flamesSeed(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "spread", 4000, function()
        creature:CastSpell(creature, SPELL_SUMMON_FLAMES_SPREAD_TRIGGER)
    end)
end

local function flamesCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({NPC_MIMIRON_FLAMES_INITIAL, NPC_MIMIRON_FLAMES_SPREAD}) do
    RegisterCreatureEvent(entry, 5, flamesSeed)
    RegisterCreatureEvent(entry, 23, flamesSeed)
    RegisterCreatureEvent(entry, 2, flamesCancel)
    RegisterCreatureEvent(entry, 4, flamesCancel)
end

-- Unmodeled: the UpdateAI boss-state gate (despawn unless
-- BOSS_MIMIRON is IN_PROGRESS) — no instance boss-state bridge.
-- Harmless here: the spread cast is one-shot, so a lingering flame
-- has no further behavior.
