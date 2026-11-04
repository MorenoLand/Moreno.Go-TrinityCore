-- Wintergrasp spirit guides — Lua port of npc_wg_spirit_guideAI in
-- src/server/scripts/Northrend/zone_wintergrasp.cpp (lines 231-263).
-- Entries (wowhead, no C++ NPC_ constant exists for these — the
-- wg_queue wowhead-only precedent, never guessed): 31841
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
-- whenever the guide is not already casting. Handler creatures carry
-- no live cast state (the halazzi vacuous-false IsCasting twin), so the
-- gate is vacuous here and the 1s poll re-casts unconditionally — the
-- same rendering the nagrand_banner port uses for its vacuous
-- HasUnitState(CASTING) early-return. One-shot chain, not a repeat
-- count (the wg_queue schedule idiom).
local function spiritHealTick(creature, guid)
    creature:CastSpell(creature, SPELL_CHANNEL_SPIRIT_HEAL)
    schedule(guid, "heal", 1000, function()
        spiritHealTick(creature, guid)
    end)
end

local function spiritGuideSeed(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    spiritHealTick(creature, guid)
end

local function spiritGuideCancel(event, creature)
    cancelTimers(creature:GetGUID())
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
