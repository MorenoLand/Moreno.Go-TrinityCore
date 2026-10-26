-- Rotting Frost Giant (Icecrown Citadel weekly-quest "Securing the
-- Ramparts" spawn) — Lua port of npc_rotting_frost_giant
-- (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:448-505).
-- Script registration (icecrown_citadel.cpp:1598) is on the script name
-- "npc_rotting_frost_giant", shared by both difficulty entries
-- (icecrown_citadel.h:158-159):
local NPC_ROTTING_FROST_GIANT_10 = 38490
local NPC_ROTTING_FROST_GIANT_25 = 38494

local EMOTE_DEATH_PLAGUE_WARNING = 0
local SPELL_DEATH_PLAGUE_AURA = 72865
local SPELL_RECENTLY_INFECTED = 72884
local SPELL_STOMP = 64652
local SPELL_ARCTIC_BREATH = 72848

-- GUID-keyed CreateLuaEvent timers (C++ Reset schedules the EventMap;
-- the C++ EventMap only ticks once UpdateAI runs with a victim, so
-- engage-seeding is behavior-identical — the krystallus precedent).
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

-- C++ SelectTarget(Random, 1, DeathPlagueTargetSelector(me))
-- (icecrown_citadel.cpp:482, selector :426-443): threat-list
-- player within 100.0f, no DEATH_PLAGUE_AURA and no
-- RECENTLY_INFECTED; UnitAI.h builds the list victim-first, so
-- position=1 pops the victim BEFORE the predicate (akilzon
-- idiom) — the victim exclusion is the pop, not the positional
-- drop it replaced. The IsDead gate follows the settled strip
-- convention (C++ skips only offline refs; dead picks are no-op
-- casts in both engines).
local function selectDeathPlagueTarget(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    local aliveSeen = 0
    for _, unit in ipairs(creature:GetAITargets()) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 <= 100.0
                and not unit:HasAura(SPELL_DEATH_PLAGUE_AURA)
                and not unit:HasAura(SPELL_RECENTLY_INFECTED) then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_DEATH_PLAGUE (:473-480): if a target is found,
-- Talk(EMOTE_DEATH_PLAGUE_WARNING 0, target) + DoCast(target, 72865);
-- unconditional 15s re-arm (the cast has no target gate beyond the
-- selector).
local function deathPlagueTick(creature, guid)
    local target = selectDeathPlagueTarget(creature)
    if target then
        creature:Talk(EMOTE_DEATH_PLAGUE_WARNING, target)
        creature:CastSpell(target, SPELL_DEATH_PLAGUE_AURA)
    end
    schedule(guid, "deathplague", 15000, function()
        deathPlagueTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_STOMP (:481-484): DoCastVictim(SPELL_STOMP 64652),
-- {15s, 18s} re-arm.
local function stompTick(creature, guid)
    creature:CastSpell(nil, SPELL_STOMP)
    schedule(guid, "stomp", {15000, 18000}, function()
        stompTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_ARCTIC_BREATH (:485-488): DoCastVictim(72848),
-- {26s, 33s} re-arm.
local function arcticBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCTIC_BREATH)
    schedule(guid, "arctic", {26000, 33000}, function()
        arcticBreathTick(creature, guid)
    end)
end

-- C++ Reset (:452-457): the three initial schedules (15s / {5s,8s} /
-- {10s,15s}).
local function frostGiantEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "deathplague", 15000, function()
        deathPlagueTick(creature, guid)
    end)
    schedule(guid, "stomp", {5000, 8000}, function()
        stompTick(creature, guid)
    end)
    schedule(guid, "arctic", {10000, 15000}, function()
        arcticBreathTick(creature, guid)
    end)
end

local function frostGiantLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (:459-464): _events.Reset() + instance->
-- DoRemoveAurasDueToSpellOnPlayers(SPELL_DEATH_PLAGUE_AURA).
-- DoRemoveAurasDueToSpellOnPlayers iterates the instance's players;
-- the Go twin is the in-tree idiom: GetPlayersInWorld filtered to the
-- boss's map/instance + player:RemoveAura (lua_player.go; bridged).
local function frostGiantDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            plr:RemoveAura(SPELL_DEATH_PLAGUE_AURA)
        end
    end
end

local function frostGiantReset(event, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({NPC_ROTTING_FROST_GIANT_10, NPC_ROTTING_FROST_GIANT_25}) do
    RegisterCreatureEvent(entry, 1, frostGiantEnterCombat)
    RegisterCreatureEvent(entry, 2, frostGiantLeaveCombat)
    RegisterCreatureEvent(entry, 4, frostGiantDied)
    RegisterCreatureEvent(entry, 23, frostGiantReset)
end
