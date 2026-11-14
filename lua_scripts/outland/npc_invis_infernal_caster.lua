-- Invis Infernal Caster (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_invis_infernal_casterAI). Entry 21417 TDB-verified
-- ('Invis Infernal Caster', ScriptName npc_invis_infernal_caster).
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 5 OnSpawn,
-- 23 OnReset, 41 OnSetData.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- Reset (:78-84) — ground = own Z + SummonInfernal() + schedule
-- EVENT_CAST_SUMMON_INFERNAL in 1-3s. Runs on spawn (5) and reset (23).
-- SetData (:86-89) — id 1 (TYPE_INFERNAL) + data 1 (DATA_DIED) ->
-- SummonInfernal() (re-summon; the old infernal is dead). This is the
-- joint twin of the attacker JustDied leg (npc_infernal_attacker.lua).
-- UpdateAI (:100-117) — 12s poll: resolve the infernal by GUID; if
-- its display id is still MODEL_INVISIBLE 20577, cast 37277
-- (SPELL_SUMMON_INFERNAL) on it, triggered. Ported as a self-rearming
-- CreateLuaEvent poll (maiden idiom): first fire at math.random
-- (1000, 3000), then every 12000ms; cancelled on leave-combat (2) and
-- died (4), the millhouse convention.
-- SummonInfernal (:91-98) — SummonCreature(21419, own x/y, ground +
-- 0.05, o 0, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, 1min) + latch the
-- infernal GUID (cleared when the summon fails).
-- Deviation: UpdateGroundPositionZ has no bridge — the summon uses the
-- caster's own Z + 0.05 instead of ground Z + 0.05. Lua summon type 0
-- is the C++ TEMPSUMMON_TIMED_OR_DEAD_DESPAWN (1) minus 1
-- (spawnCreatureFromOrigin 0-based scheme, enraged-spirit precedent).

local ENTRY_INVIS_INFERNAL_CASTER = 21417

local EVENT_CAST_SUMMON_INFERNAL = 1
local NPC_INFERNAL_ATTACKER = 21419
local MODEL_INVISIBLE = 20577
local SPELL_SUMMON_INFERNAL = 37277
local TYPE_INFERNAL = 1
local DATA_DIED = 1

local casterState = {}
local casterPoll = {}

local function cancelPoll(guid)
    local id = casterPoll[guid]
    if id then
        RemoveEventById(id)
        casterPoll[guid] = nil
    end
end

local function summonInfernal(creature, guid)
    local infernal = creature:SummonCreature(NPC_INFERNAL_ATTACKER,
        creature:GetX(), creature:GetY(), creature:GetZ() + 0.05, 0.0, 0, 60000)
    local st = casterState[guid]
    if not st then
        return
    end
    if infernal then
        st.infernalGUID = infernal:GetGUIDLow()
    else
        st.infernalGUID = nil
    end
end

local function pollSummonSpell(creature, guid)
    local st = casterState[guid]
    if not st then
        return
    end
    if st.infernalGUID then
        local infernal = creature:GetCreatureByGUID(st.infernalGUID)
        if infernal and infernal:GetDisplayId() == MODEL_INVISIBLE then
            creature:CastSpell(infernal, SPELL_SUMMON_INFERNAL, true)
        end
    end
    cancelPoll(guid)
    casterPoll[guid] = CreateLuaEvent(function()
        pollSummonSpell(creature, guid)
    end, 12000, 1)
end

local function onSpawnOrReset(_, creature)
    local guid = creature:GetGUIDLow()
    casterState[guid] = { infernalGUID = nil }
    cancelPoll(guid)
    summonInfernal(creature, guid)
    casterPoll[guid] = CreateLuaEvent(function()
        pollSummonSpell(creature, guid)
    end, math.random(1000, 3000), 1)
end

local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelPoll(guid)
    casterState[guid] = nil
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelPoll(guid)
    casterState[guid] = nil
end

local function onSetData(_, creature, type, data)
    if type ~= TYPE_INFERNAL or data ~= DATA_DIED then
        return
    end
    summonInfernal(creature, creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_INVIS_INFERNAL_CASTER, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_INVIS_INFERNAL_CASTER, 23, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_INVIS_INFERNAL_CASTER, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_INVIS_INFERNAL_CASTER, 4, onDied)
RegisterCreatureEvent(ENTRY_INVIS_INFERNAL_CASTER, 41, onSetData)
