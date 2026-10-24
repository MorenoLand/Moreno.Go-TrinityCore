-- Assault Bot (Ulduar, Mimiron Aerial Command Unit phase) — Lua port of
-- npc_mimiron_assault_botAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1283-1340).
-- Entry (wowhead npc=34057, Ulduar; no C++ NPC_ constant exists for
-- this add — the cthun/telestra wowhead-only precedent, never guessed):
local NPC_MIMIRON_ASSAULT_BOT = 34057

local SPELL_MAGNETIC_FIELD = 64668

-- GUID-keyed CreateLuaEvent timers (C++ JustEngagedWith schedules the
-- EventMap; the C++ EventMap only ticks once UpdateAI runs with a
-- victim, so engage-seeding is behavior-identical — the krystallus
-- precedent).
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

-- C++ UpdateAI EVENT_MAGNETIC_FIELD: DoCastVictim(SPELL_MAGNETIC_FIELD
-- 64668), 30s re-arm.
local function magneticFieldTick(creature, guid)
    creature:CastSpell(nil, SPELL_MAGNETIC_FIELD)
    schedule(guid, "magnetic", 30000, function()
        magneticFieldTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: EVENT_MAGNETIC_FIELD in 14s.
local function assaultBotEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "magnetic", 14000, function()
        magneticFieldTick(creature, guid)
    end)
end

local function assaultBotLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function assaultBotDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function assaultBotReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_MIMIRON_ASSAULT_BOT, 1, assaultBotEnterCombat)
RegisterCreatureEvent(NPC_MIMIRON_ASSAULT_BOT, 2, assaultBotLeaveCombat)
RegisterCreatureEvent(NPC_MIMIRON_ASSAULT_BOT, 4, assaultBotDied)
RegisterCreatureEvent(NPC_MIMIRON_ASSAULT_BOT, 23, assaultBotReset)

-- Unmodeled: the UNIT_STATE_ROOT retarget arm (C++ UpdateAI: on ROOT,
-- SelectTarget(MinDistance, 0, 30.0f) + ResetAllThreat + AttackStart).
-- HasUnitState has no bridge on the handler-creature surface and
-- AttackStart has no bridge (anomalus header precedent); the magnetic
-- field timer above is the whole engage-gated behavior.
