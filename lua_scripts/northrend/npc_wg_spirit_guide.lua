-- Wintergrasp spirit guides — Lua port of npc_wg_spirit_guideAI in
-- src/server/scripts/Northrend/zone_wintergrasp.cpp (class :202-257; AI struct :207-251).
-- Entries (TDB-verified with ScriptName 'npc_wg_spirit_guide':
-- no C++ NPC_ constant exists for these — the wg_queue
-- wowhead-only precedent, never guessed): 31841
-- "Taunka Spirit Guide" (Horde) and 31842 "Dwarven Spirit Guide"
-- (Alliance), both wowhead <Wintergrasp> pages.
local NPC_WG_SPIRIT_GUIDE_HORDE = 31841
local NPC_WG_SPIRIT_GUIDE_ALLIANCE = 31842

local SPELL_CHANNEL_SPIRIT_HEAL = 22011

-- GUID-keyed CreateLuaEvent timers, seeded on events 5/23
-- (cancel-first) and cancelled on events 2/4.
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

-- C++ UpdateAI: DoCast(me, SPELL_CHANNEL_SPIRIT_HEAL), non-triggered,
-- whenever the guide is not already casting. Ported as a 500ms
-- IsCasting defer (mal_ganis idiom; IsCasting live,
-- lua_creature_events.go:2480 — the old vacuous-false claim is
-- stale, deathwhisper precedent); the 1s re-arm is not consumed
-- while casting. One-shot chain, not a repeat
-- count (the wg_queue schedule idiom).
local function spiritHealTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "heal", 500, function()
            spiritHealTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CHANNEL_SPIRIT_HEAL)
    schedule(guid, "heal", 1000, function()
        spiritHealTick(creature, guid)
    end)
end

local function spiritGuideSeed(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    spiritHealTick(creature, guid)
end

local function spiritGuideCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

for _, entry in ipairs({NPC_WG_SPIRIT_GUIDE_HORDE, NPC_WG_SPIRIT_GUIDE_ALLIANCE}) do
    RegisterCreatureEvent(entry, 5, spiritGuideSeed)
    RegisterCreatureEvent(entry, 23, spiritGuideSeed)
    RegisterCreatureEvent(entry, 2, spiritGuideCancel)
    RegisterCreatureEvent(entry, 4, spiritGuideCancel)
end

-- Unmodeled: OnGossipHello / OnGossipSelect (Battlefield graveyard
-- vector + WorldSafeLocs teleport via sBattlefieldMgr) — no
-- battlefield bridge, same documented-only class as wg_queue gossip.
