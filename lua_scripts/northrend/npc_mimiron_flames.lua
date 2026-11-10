-- Flames (Ulduar, Mimiron hard-mode fire) — Lua port of
-- npc_mimiron_flamesAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1516-1561).
-- Entries (ulduar.h:128-129: NPC_FLAME 34363, NPC_FLAME_SPREAD 34121):
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

-- ulduar.h:46: BOSS_MIMIRON = 10. EncounterState IN_PROGRESS = 1.
local BOSS_MIMIRON = 10
local BOSS_STATE_IN_PROGRESS = 1

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

-- C++ UpdateAI boss-state gate (:1535-1536): despawn unless BOSS_MIMIRON
-- is IN_PROGRESS. GetBossState is bridged (lua_creature_events.go:6612)
-- and used in-tree (karazhan maiden/midnight/curator idiom); the
-- header's "no instance boss-state bridge" claim is retracted. 1s poll
-- (the wg_spirit_guide per-tick lowering); C++ checks before
-- events.Update, so the poll is seeded alongside the spread cast.
local function despawnCheckTick(creature, guid)
    if creature:GetBossState(BOSS_MIMIRON) ~= BOSS_STATE_IN_PROGRESS then
        cancelTimers(guid)
        creature:Despawn()
        return
    end
    schedule(guid, "despawnCheck", 1000, function()
        despawnCheckTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_SPREAD_FLAMES: DoCastAOE(64562), one-shot
-- (no re-arm in C++).
local function flamesSeed(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "spread", 4000, function()
        creature:CastSpell(creature, SPELL_SUMMON_FLAMES_SPREAD_TRIGGER)
    end)
    schedule(guid, "despawnCheck", 1000, function()
        despawnCheckTick(creature, guid)
    end)
end

local function flamesCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

for _, entry in ipairs({NPC_MIMIRON_FLAMES_INITIAL, NPC_MIMIRON_FLAMES_SPREAD}) do
    RegisterCreatureEvent(entry, 5, flamesSeed)
    RegisterCreatureEvent(entry, 23, flamesSeed)
    RegisterCreatureEvent(entry, 2, flamesCancel)
    RegisterCreatureEvent(entry, 4, flamesCancel)
end
