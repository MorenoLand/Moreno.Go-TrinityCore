-- Loatheb (Naxxramas, Plague Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_loatheb.cpp
-- (boss_loatheb (BossAI); AddSC_boss_loatheb also registers
-- achievement_spore_loser and spell_loatheb_deathbloom, both
-- unmodeled below).
-- Entry: 16011 Loatheb (creature_template table, cross-checked:
-- Anub'Rekhan 15956 / Faerlina 15953 / Thaddius 15928 / Maexxna 15952 /
-- Gluth 15932 / Heigan 15936 / Sapphiron 15989 / Kel'Thuzad 15990 all
-- match naxxramas.h NAXCreaturesIds; 25-man difficulty entry 29718 is
-- NOT registered, base-entry-only convention like anubrekhan/maexxna).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 21 OnSummonedCreatureDies, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith (event 1; BossAI::JustEngagedWith
-- instance-bookkeeping leg has no bridge — tharon_ja precedent) +
-- EVENT_NECROTIC_AURA 17s + EVENT_DEATHBLOOM 5s + EVENT_SPORE 18s +
-- EVENT_INEVITABLE_DOOM 2min; EVENT_NECROTIC_AURA DoCastAOE
-- (NECROTIC_AURA 55593, non-triggered) lowered to self-cast
-- (DoCastAOE self-cast convention, selin/mr_smite precedent) +
-- Talk(SAY_NECROTIC_AURA_APPLIED 0) + EVENT_NECROTIC_AURA_FADING
-- 14s + EVENT_NECROTIC_AURA_FADED 17s, 20s repeat; FADING
-- Talk(SAY_NECROTIC_AURA_FADING 2); FADED Talk(SAY_NECROTIC_AURA_
-- REMOVED 1); EVENT_DEATHBLOOM DoCastAOE (DEATHBLOOM 29865,
-- non-triggered) self-cast, 30s repeat; EVENT_INEVITABLE_DOOM
-- doomCounter++ then DoCastAOE (INEVITABLE_DOOM 29204,
-- non-triggered) self-cast, 30s repeat while counter <= 6, then
-- alternating 14s (odd counter) / 17s (even counter) — C++ checks
-- the incremented counter, mirrored here; EVENT_SPORE DoCastSelf
-- (SUMMON_SPORE 29234, non-triggered), 36s repeat on 10-man /
-- 15s on 25-man (C++ RAID_MODE(36s, 15s); 25-man is the victim-
-- group-size proxy, deathwhisper precedent; Is25ManRaid
-- unbridged). SummonedCreatureDies (event 21, anomalus
-- precedent: handler args are event, creature, summon, killer):
-- summon casts FUNGAL_CREEP 29232 on itself, triggered — the
-- _sporeLoser latch itself is unmodeled (its only consumer is the
-- achievement criteria script, no bridge). Reset (event 23):
-- FUNGAL_CREEP 29232 removed from every player on the boss's
-- map+instance (player:RemoveAura bridge, lua_player.go:299;
-- apoko map/instance filter precedent) + doomCounter reset.
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired): Reset/
-- JustEngagedWith/JustDied instance bookkeeping (_Reset/_JustDied);
-- achievement_spore_loser criteria script (GetData consumer — no
-- achievement bridge); spell_loatheb_deathbloom AuraScript
-- AfterRemove leg (DEATHBLOOM_FINAL_DAMAGE 55594 on aura expire —
-- no SpellScript bridge); the spore creature itself has no AI
-- class in this file (spell-summoned, DB-side); melee
-- engine-driven.

local ENTRY_LOATHEB = 16011

local SAY_NECROTIC_AURA_APPLIED = 0
local SAY_NECROTIC_AURA_REMOVED = 1
local SAY_NECROTIC_AURA_FADING = 2

local SPELL_NECROTIC_AURA = 55593
local SPELL_SUMMON_SPORE = 29234
local SPELL_DEATHBLOOM = 29865
local SPELL_INEVITABLE_DOOM = 29204
local SPELL_FUNGAL_CREEP = 29232

local timers = {}
local doomCounter = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    doomCounter[guid] = nil
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

local function is25Man(victim)
    if victim ~= nil and victim.GetGroup ~= nil then
        local group = victim:GetGroup()
        if group ~= nil then
            local members = group:GetMembers()
            return members ~= nil and #members > 10
        end
    end
    return false
end

local function necroticFadingTick(creature, guid)
    creature:Talk(SAY_NECROTIC_AURA_FADING)
end

local function necroticFadedTick(creature, guid)
    creature:Talk(SAY_NECROTIC_AURA_REMOVED)
end

local function necroticTick(creature, guid)
    creature:CastSpell(creature, SPELL_NECROTIC_AURA)
    creature:Talk(SAY_NECROTIC_AURA_APPLIED)
    schedule(guid, "necroticFading", 14000, function()
        necroticFadingTick(creature, guid)
    end)
    schedule(guid, "necroticFaded", 17000, function()
        necroticFadedTick(creature, guid)
    end)
    schedule(guid, "necrotic", 20000, function()
        necroticTick(creature, guid)
    end)
end

local function deathbloomTick(creature, guid)
    creature:CastSpell(creature, SPELL_DEATHBLOOM)
    schedule(guid, "deathbloom", 30000, function()
        deathbloomTick(creature, guid)
    end)
end

local function doomTick(creature, guid)
    doomCounter[guid] = (doomCounter[guid] or 0) + 1
    creature:CastSpell(creature, SPELL_INEVITABLE_DOOM)
    local delay
    if doomCounter[guid] > 6 then
        if doomCounter[guid] % 2 == 1 then
            delay = 14000
        else
            delay = 17000
        end
    else
        delay = 30000
    end
    schedule(guid, "doom", delay, function()
        doomTick(creature, guid)
    end)
end

local function sporeTick(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SPORE)
    local delay = 36000
    if is25Man(creature:GetVictim()) then
        delay = 15000
    end
    schedule(guid, "spore", delay, function()
        sporeTick(creature, guid)
    end)
end

local function loathebEnterCombat(event, creature)
    local guid = creature:GetGUID()
    doomCounter[guid] = 0
    schedule(guid, "necrotic", 17000, function()
        necroticTick(creature, guid)
    end)
    schedule(guid, "deathbloom", 5000, function()
        deathbloomTick(creature, guid)
    end)
    schedule(guid, "spore", 18000, function()
        sporeTick(creature, guid)
    end)
    schedule(guid, "doom", 120000, function()
        doomTick(creature, guid)
    end)
end

local function loathebLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function loathebDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function loathebSummonedDied(event, creature, summon, killer)
    if summon ~= nil then
        summon:CastSpell(summon, SPELL_FUNGAL_CREEP, true)
    end
end

local function loathebReset(event, creature)
    cancelTimers(creature:GetGUID())
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            plr:RemoveAura(SPELL_FUNGAL_CREEP)
        end
    end
end

RegisterCreatureEvent(ENTRY_LOATHEB, 1, loathebEnterCombat)
RegisterCreatureEvent(ENTRY_LOATHEB, 2, loathebLeaveCombat)
RegisterCreatureEvent(ENTRY_LOATHEB, 4, loathebDied)
RegisterCreatureEvent(ENTRY_LOATHEB, 21, loathebSummonedDied)
RegisterCreatureEvent(ENTRY_LOATHEB, 23, loathebReset)
