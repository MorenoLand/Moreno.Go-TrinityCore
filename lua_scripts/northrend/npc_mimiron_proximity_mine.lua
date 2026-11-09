-- Proximity Mine (Ulduar, Mimiron Leviathan Mk II phase) — Lua port of
-- npc_mimiron_proximity_mineAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1612-1658).
-- Entry (wowhead npc=34362, Ulduar, WotLK; no C++ NPC_ constant exists
-- for this add — the cthun/telestra wowhead-only precedent, never
-- guessed; npc=27679 rejected: its page says added in Mists of Pandaria):
local NPC_MIMIRON_PROXIMITY_MINE = 34362

local SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER = 65345
local SPELL_PROXIMITY_MINE_EXPLOSION = 66351

-- boss_mimiron.cpp:184-186 Data enum; ulduar.h UlduarData
-- (DATA_COLOSSUS = 20, counting up) instance GUID key for the
-- Leviathan MK II (instance_ulduar.cpp:817).
local DATA_SETUP_MINE = 0
local DATA_LEVIATHAN_MK_II = 32

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
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "arm", 1500, function()
        armTick(creature, guid)
    end)
end

local function mineCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ spell_mimiron_proximity_explosion::OnHit
-- (boss_mimiron.cpp:2090-2099, spell 66351): when the explosion
-- hits a player, the MK II's DATA_SETUP_MINE latch clears (the
-- achievement_setup_mine gate). No SpellScript bridge exists, so
-- the leg rides the creature SpellHitTarget hook instead (ev15,
-- onyxia breath precedent): the mine is the caster, the handler
-- fires per hit player, and the MK II resolves via the instance
-- GUID store (latched by boss_leviathan_mk_ii.lua ev5).
-- Dormant in Go until MK II's proximity-mine summon arm lands
-- (EVENT_PROXIMITY_MINE is DoAction-scheduled, unported), so no
-- mine can spawn and 66351 can never fire today.
local function mineSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_PROXIMITY_MINE_EXPLOSION then
        return
    end
    if not target or target:GetTypeId() ~= 4 then -- TYPEID_PLAYER
        return
    end
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        mkii:SetData(DATA_SETUP_MINE, 0)
    end
end

RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 5, mineSeed)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 23, mineSeed)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 2, mineCancel)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 4, mineCancel)
RegisterCreatureEvent(NPC_MIMIRON_PROXIMITY_MINE, 15, mineSpellHitTarget)
