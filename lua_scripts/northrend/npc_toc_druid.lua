-- npc_toc_druid (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_druidAI (AI_HEALER); C++ CreatureScript("npc_toc_druid")).
-- Currently only the EVENT_HEAL_BARKSKIN arm is ported: the rest of the
-- healer rotation (rejuvenation/tranquility/thorns/nature grasp) rides
-- the faction-champions encounter machine documented in
-- npc_toc_boomkin.lua's header.
-- Entries: 34469 Alliance restoration druid / 34459 Horde restoration
-- druid (trial_of_the_crusader.h:183/197 NPC_ALLIANCE_DRUID_RESTORATION
-- / NPC_HORDE_DRUID_RESTORATION).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 5 OnSpawn, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick).
-- Ported arms (C++-exact for all modeled arms):
-- EVENT_HEAL_BARKSKIN — HealthBelowPct(30) -> DoCastSelf(Barkskin
-- 65860) + 60s re-arm, else 3s re-arm; 15-25s init
-- (boss_faction_champions.cpp:771/816-824).

local ENTRY_DRUID_ALLIANCE = 34469 -- trial_of_the_crusader.h:183
local ENTRY_DRUID_HORDE = 34459 -- trial_of_the_crusader.h:197

local SPELL_BARKSKIN = 65860 -- boss_faction_champions.cpp:51

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ npc_toc_druidAI::EVENT_HEAL_BARKSKIN
-- (boss_faction_champions.cpp:816-824): HealthBelowPct(30) ->
-- DoCast(me, SPELL_BARKSKIN) + 60s re-arm, else 3s re-arm. The
-- GetHealthPct() form matches the in-tree sindragosa precedent.
local function onBarkskin(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 30 / 100) then
        creature:CastSpell(creature, SPELL_BARKSKIN)
        schedule(guid, "barkskin", 60000, function() onBarkskin(creature, guid) end)
    else
        schedule(guid, "barkskin", 3000, function() onBarkskin(creature, guid) end)
    end
end

local function druidEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "barkskin", {15000, 25000}, function() onBarkskin(creature, guid) end)
end

local function druidLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function druidDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function druidSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function druidReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 1, druidEnterCombat)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 2, druidLeaveCombat)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 4, druidDied)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 5, druidSpawn)
RegisterCreatureEvent(ENTRY_DRUID_ALLIANCE, 23, druidReset)

RegisterCreatureEvent(ENTRY_DRUID_HORDE, 1, druidEnterCombat)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 2, druidLeaveCombat)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 4, druidDied)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 5, druidSpawn)
RegisterCreatureEvent(ENTRY_DRUID_HORDE, 23, druidReset)
