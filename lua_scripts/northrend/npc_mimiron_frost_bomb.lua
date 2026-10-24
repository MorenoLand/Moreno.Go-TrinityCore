-- Frost Bomb (Ulduar, Mimiron VX-001 phase) — Lua port of
-- npc_mimiron_frost_bombAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1564-1610).
-- Entry (wowhead: spell 64627, the C++ SPELL_SUMMON_FROST_BOMB,
-- summons npc=34149; no C++ NPC_ constant exists for this add — the
-- cthun/telestra wowhead-only precedent, never guessed):
local NPC_MIMIRON_FROST_BOMB = 34149

local SPELL_FROST_BOMB_EXPLOSION = 64626
local SPELL_CLEAR_FIRES = 65354

-- GUID-keyed CreateLuaEvent timers (C++ Reset schedules the EventMap;
-- spawn/reset-started here with cancel-first idempotence — the cthun
-- precedent — since a summoned bomb's Reset is the schedule point).
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

-- C++ UpdateAI EVENT_FROST_BOMB_CLEAR_FIRES: DoCastAOE(65354), then
-- DespawnOrUnsummon(3s).
local function clearFiresTick(creature, guid)
    creature:CastSpell(creature, SPELL_CLEAR_FIRES)
    schedule(guid, "despawn", 3000, function()
        creature:Despawn()
    end)
end

-- C++ UpdateAI EVENT_FROST_BOMB_EXPLOSION: DoCastAOE(64626), then
-- EVENT_FROST_BOMB_CLEAR_FIRES in 3s.
local function explosionTick(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_BOMB_EXPLOSION)
    schedule(guid, "clear", 3000, function()
        clearFiresTick(creature, guid)
    end)
end

-- C++ Reset: EVENT_FROST_BOMB_EXPLOSION in 10s.
local function frostBombSeed(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "explosion", 10000, function()
        explosionTick(creature, guid)
    end)
end

local function frostBombCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_MIMIRON_FROST_BOMB, 5, frostBombSeed)
RegisterCreatureEvent(NPC_MIMIRON_FROST_BOMB, 23, frostBombSeed)
RegisterCreatureEvent(NPC_MIMIRON_FROST_BOMB, 2, frostBombCancel)
RegisterCreatureEvent(NPC_MIMIRON_FROST_BOMB, 4, frostBombCancel)
