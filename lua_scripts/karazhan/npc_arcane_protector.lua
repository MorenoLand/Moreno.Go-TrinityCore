-- Arcane Protector (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp
-- (npc_arcane_protectorAI). Creature entry 16504 (wowhead wotlk
-- npc=16504/arcane-protector; TDB creature_template ScriptName
-- "npc_arcane_protector" via AddSC_karazhan).
-- Eluna creature events: 1 OnEnterCombat (arm timers), 2 OnLeaveCombat /
-- 23 OnReset (cancel timers; C++ Reset's DoCast(me, SPELL_INVI) runs on
-- the reset hook). Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: the npc_barnes and npc_image_of_medivh scripts
-- from the same C++ file are not registered (npc_mana_feeder is ported
-- in its own script — the six ApplySpellImmune school arms ride the
-- creature:ApplySpellImmune bridge); Barnes is an
-- EscortAI with gossip waypoints and instance-script arms (no gossip /
-- escort / instance-script bridge), and Medivh's dialogue machine is
-- triggered by MovementInform after a MovePoint (no movement model).

local ENTRY = 16504

local SPELL_FIST_OF_STONE = 29837
local SPELL_INVI = 41634
local SPELL_RETURN_FIRE_MELEE = 29788
local SPELL_RETURN_FIRE_SPELL = 29793
local SPELL_RETURN_FIRE_RANGED = 29794

local NPC_ASTRAL_SPARK = 17283 -- karazhan.cpp:693

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

-- C++ summonTimer (karazhan.cpp:751-758): SummonCreature(NPC_ASTRAL_
-- SPARK 17283 at me->GetPositionX/Y/Z(), o = 0,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (bridge type 3), 15000s =
-- 15000000ms) with C++-exact re-arm {15000,19000} (init urand(3000,8000)
-- from C++ Reset, :722 — the Reset-only DoCast(INVI) stays on the reset
-- hook below). The C++ ArcaneProtectorGUID re-lookup is a dead store
-- (the fetched pointer is never used), so it is not modeled.
-- PORTED 2026-10-23 (position-leg axis; stale "no summon model" header
-- claim retired).
local function onAstralSpark(creature, guid)
    creature:SummonCreature(NPC_ASTRAL_SPARK, creature:GetX(),
        creature:GetY(), creature:GetZ(), 0, 3, 15000000)
    schedule(guid, "astralspark", {15000, 19000}, function()
        onAstralSpark(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fist", 1000, function() onFistOfStone(creature, guid) end)
    schedule(guid, "astralspark", math.random(3000, 8000), function()
        onAstralSpark(creature, guid)
    end)
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
    -- C++ Reset(): DoCast(me, SPELL_INVI) is Reset-only (karazhan.cpp).
    -- Alive-gated; the timer cancel rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_INVI)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
