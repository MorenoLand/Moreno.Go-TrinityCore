-- Arcane Protector (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp
-- (npc_arcane_protectorAI). Creature entry 16504 (wowhead wotlk
-- npc=16504/arcane-protector; TDB creature_template ScriptName
-- "npc_arcane_protector" via AddSC_karazhan).
-- Eluna creature events: 1 OnEnterCombat (arm timers), 2 OnLeaveCombat /
-- 23 OnReset (cancel timers; C++ Reset's DoCast(me, SPELL_INVI) runs on
-- the reset hook). Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: the engine has no summon model, so the Astral
-- Spark (17283) summon arm is skipped — the summon timer simply does not
-- exist in this port. The npc_mana_feeder, npc_barnes, and
-- npc_image_of_medivh scripts from the same C++ file are not registered:
-- mana feeder's only arms are ApplySpellImmune (no bridge), Barnes is an
-- EscortAI with gossip waypoints and instance-script arms (no gossip /
-- escort / instance-script bridge), and Medivh's dialogue machine is
-- triggered by MovementInform after a MovePoint (no movement model).

local ENTRY = 16504

local SPELL_FIST_OF_STONE = 29837
local SPELL_INVI = 41634
local SPELL_RETURN_FIRE_MELEE = 29788
local SPELL_RETURN_FIRE_SPELL = 29793
local SPELL_RETURN_FIRE_RANGED = 29794

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

local function onFistOfStone(creature, guid)
    creature:CastSpell(creature, SPELL_FIST_OF_STONE)
    schedule(guid, "fist", {18000, 21000}, function() onFistOfStone(creature, guid) end)
end

local function onReturnFireMelee(creature, guid)
    creature:CastSpell(creature, SPELL_RETURN_FIRE_MELEE)
    schedule(guid, "returnfire", 80000, function() onReturnFireMelee(creature, guid) end)
end

local function onReturnFireSpell(creature, guid)
    creature:CastSpell(creature, SPELL_RETURN_FIRE_SPELL)
    schedule(guid, "returnfirespell", 60000, function() onReturnFireSpell(creature, guid) end)
end

local function onReturnFireRanged(creature, guid)
    creature:CastSpell(creature, SPELL_RETURN_FIRE_RANGED)
    schedule(guid, "returnfireranged", 100000, function() onReturnFireRanged(creature, guid) end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fist", 1000, function() onFistOfStone(creature, guid) end)
    schedule(guid, "returnfire", 24000, function() onReturnFireMelee(creature, guid) end)
    schedule(guid, "returnfirespell", 3000, function() onReturnFireSpell(creature, guid) end)
    schedule(guid, "returnfireranged", 45000, function() onReturnFireRanged(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_INVI)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
