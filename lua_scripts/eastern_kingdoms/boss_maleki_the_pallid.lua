-- Maleki the Pallid (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/
-- boss_maleki_the_pallid.cpp (boss_maleki_the_pallidAI : public
-- ScriptedAI via GetStratholmeAI; registered by
-- AddSC_boss_maleki_the_pallid in eastern_kingdoms_script_loader.cpp
-- (declaration line 127, call line 305; follows barthilas :126/:304)).
-- The file holds 1 script: boss_maleki_the_pallid (pure timer-driven
-- boss AI — no Talk lines, no gossip/quest/vehicle arms, no SpellScript/
-- AuraScript loaders).
-- Entry: no NPC_ constant in the C++ tree (DB-side ScriptName binding) —
-- 10438 independently cited (wowhead npc=10438/maleki-the-pallid).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ JustEngagedWith() — FROSTBOLT 1000ms,
--   ICETOMB 16000ms, DRAINLIFE 31000ms.
-- - EVENT_FROSTBOLT: C++ `if (rand32() % 90)` (~98.9% cast chance) then
--   DoCastVictim(SPELL_FROSTBOLT 17503) non-triggered -> GetVictim +
--   CastSpell (jeklik convention, nil-victim keeps schedule); 3500ms
--   re-arm. The gate is a nil-victim-tolerant roll: the roll runs even
--   without a victim, matching C++ (the re-arm is unconditional).
-- - EVENT_ICETOMB: C++ `if (rand32() % 65)` (~98.5% cast chance) then
--   DoCastVictim(SPELL_ICETOMB 16869); 28000ms re-arm.
-- - EVENT_DRAINLIFE: C++ `if (rand32() % 55)` (~98.2% cast chance) then
--   DoCastVictim(SPELL_DRAINLIFE 20743); 31000ms re-arm.
-- - OnDied: timer/state clear modeled; the C++ JustDied leg
--   `instance->SetData(TYPE_PALLID 4, IN_PROGRESS)` is documented-only
--   (no instance-script bridge — shadowfang keep precedent).
-- Unmodeled (documented-only, no bridges):
-- - EVENT_DRAIN_MANA / SPELL_DRAIN_MANA 17243: declared in the C++
--   enums but NEVER scheduled in the file — dead event, nothing to arm.
-- - UpdateAI's `if (me->HasUnitState(UNIT_STATE_CASTING)) return` gate:
--   no casting-state bridge, so the Lua timers run on the Eluna event
--   pump without the casting tick-slip (barthilas precedent).
-- - GetStratholmeAI wrapper: plain AI factory template, no logic.
-- Verifiable numbers (file's own enums): SPELL_FROSTBOLT = 17503,
-- SPELL_DRAINLIFE = 20743, SPELL_DRAIN_MANA = 17243, SPELL_ICETOMB = 16869;
-- TYPE_PALLID = 4 (stratholme.h :31).

local ENTRY_MALEKI = 10438

local SPELL_FROSTBOLT = 17503
local SPELL_DRAINLIFE = 20743
local SPELL_ICETOMB = 16869

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

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ `if (rand32() % N)` is true for every remainder except 0 —
-- (N-1)/N cast chance; re-arm is unconditional and runs either way.
local function roll(mod)
    return math.random(0, mod - 1) ~= 0
end

-- C++ Frostbolt: DoCastVictim(SPELL_FROSTBOLT); 3500ms re-arm.
local function onFrostbolt(creature, guid)
    if roll(90) then
        doCastVictim(creature, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", 3500, function() onFrostbolt(creature, guid) end)
end

-- C++ IceTomb: DoCastVictim(SPELL_ICETOMB); 28000ms re-arm.
local function onIcetomb(creature, guid)
    if roll(65) then
        doCastVictim(creature, SPELL_ICETOMB)
    end
    schedule(guid, "icetomb", 28000, function() onIcetomb(creature, guid) end)
end

-- C++ DrainLife: DoCastVictim(SPELL_DRAINLIFE); 31000ms re-arm.
local function onDrainLife(creature, guid)
    if roll(55) then
        doCastVictim(creature, SPELL_DRAINLIFE)
    end
    schedule(guid, "drainlife", 31000, function() onDrainLife(creature, guid) end)
end

-- C++ JustEngagedWith(): FROSTBOLT 1000ms, ICETOMB 16000ms,
-- DRAINLIFE 31000ms.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frostbolt", 1000, function() onFrostbolt(creature, guid) end)
    schedule(guid, "icetomb", 16000, function() onIcetomb(creature, guid) end)
    schedule(guid, "drainlife", 31000, function() onDrainLife(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MALEKI, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MALEKI, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_MALEKI, 4, onDied)
RegisterCreatureEvent(ENTRY_MALEKI, 23, onReset)
