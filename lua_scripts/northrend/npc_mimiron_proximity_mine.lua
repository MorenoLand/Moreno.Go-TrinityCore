-- Proximity Mine (Ulduar, Mimiron Leviathan Mk II phase) — Lua port of
-- npc_mimiron_proximity_mineAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1612-1658).
-- Entry (wowhead npc=34362, Ulduar, WotLK; no C++ NPC_ constant exists
-- for this add — the cthun/telestra wowhead-only precedent, never
-- guessed; npc=27679 rejected: its page says added in Mists of Pandaria):
local NPC_MIMIRON_PROXIMITY_MINE = 34362

local SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER = 65345
local SPELL_PROXIMITY_MINE_EXPLOSION = 66351

-- GUID-keyed CreateLuaEvent timers (C++ Reset schedules the EventMap;
-- spawn/reset-started here with cancel-first idempotence — the cthun
-- precedent — since a summoned mine's Reset is the schedule point).
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

-- C++ UpdateAI EVENT_PROXIMITY_MINE_DETONATION: if the mine still has
-- aura 65345, DoCastAOE(66351); DespawnOrUnsummon(1s) unconditionally.
local function detonationTick(creature, guid)
    if creature:HasAura(SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER) then
        creature:CastSpell(creature, SPELL_PROXIMITY_MINE_EXPLOSION)
    end
    schedule(guid, "despawn", 1000, function()
        creature:Despawn()
    end)
end

-- C++ UpdateAI EVENT_PROXIMITY_MINE_ARM: DoCast(self, 65345), then
-- EVENT_PROXIMITY_MINE_DETONATION in 33500ms.
local function armTick(creature, guid)
    creature:CastSpell(creature, SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER)
    schedule(guid, "detonation", 33500, function()
        detonationTick(creature, guid)
    end)
end

-- C++ Reset: EVENT_PROXIMITY_MINE_ARM in 1500ms.
local function mineSeed(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arm", 1500, function()
        armTick(creature, guid)
    end)
end

local function mineCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 5, mineSeed)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 23, mineSeed)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 2, mineCancel)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 4, mineCancel)
