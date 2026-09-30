-- Maiden of Virtue (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_maiden_of_virtue.cpp
-- Creature entry 16457 (TDB creature_template ScriptName "boss_maiden_of_virtue").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts are
-- packet-visual), so timers fire unconditionally; SelectTarget(random, range)
-- is approximated with GetPlayersInWorld filtered by map/instance/alive/
-- distance; instance encounter state binds through the luaBossAI shim in
-- engine/world/boss_ai.go (no Karazhan instance-script model exists yet).

local ENTRY = 16457

local SAY_AGGRO, SAY_SLAY, SAY_REPENTANCE, SAY_DEATH = 0, 1, 2, 3

local SPELL_REPENTANCE = 29511
local SPELL_HOLYFIRE = 29522
local SPELL_HOLYWRATH = 32445
local SPELL_HOLYGROUND = 29523
local SPELL_BERSERK = 26662

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function randomTargetInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onRepentance(creature, guid)
    creature:CastSpell(nil, SPELL_REPENTANCE)
    creature:Talk(SAY_REPENTANCE)
    schedule(guid, "repentance", 35000, function() onRepentance(creature, guid) end)
end

local function onHolyFire(creature, guid)
    local target = randomTargetInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_HOLYFIRE)
    end
    schedule(guid, "holyfire", {8000, 19000}, function() onHolyFire(creature, guid) end)
end

local function onHolyWrath(creature, guid)
    local target = randomTargetInRange(creature, 80)
    if target then
        creature:CastSpell(target, SPELL_HOLYWRATH)
    end
    schedule(guid, "holywrath", {15000, 25000}, function() onHolyWrath(creature, guid) end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_HOLYGROUND)
    schedule(guid, "repentance", {33000, 45000}, function() onRepentance(creature, guid) end)
    schedule(guid, "holyfire", 8000, function() onHolyFire(creature, guid) end)
    schedule(guid, "holywrath", {15000, 25000}, function() onHolyWrath(creature, guid) end)
    schedule(guid, "enrage", 600000, function() creature:CastSpell(creature, SPELL_BERSERK) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    if math.random() < 0.5 then
        creature:Talk(SAY_SLAY)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
