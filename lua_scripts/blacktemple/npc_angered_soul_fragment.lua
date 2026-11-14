-- Angered Soul Fragment (Black Temple) --
-- Lua port of src/server/scripts/Outland/BlackTemple/black_temple.cpp
-- (npc_angered_soul_fragment — Reset invisibility cycle +
-- JustEngagedWith Anger task). Entry 23398 'Angered Soul Fragment',
-- TDB-verified (ScriptName npc_angered_soul_fragment, 39 spawns
-- map 564).
-- Eluna creature events: 23 OnReset, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied.
-- C++ Reset (black_temple.cpp:185-203): CancelAll, then a
-- GROUP_OUT_OF_COMBAT task, first firing 1s: DoCastSelf(41253 Greater
-- Invisibility); schedules a 5-10s one-shot RemoveAurasDueToSpell
-- (41253); repeats 15-25s.
-- C++ JustEngagedWith (:204-217): RemoveAurasDueToSpell(41253);
-- CancelGroup(GROUP_OUT_OF_COMBAT); schedule a 1s task: victim =
-- GetVictim(); if the victim is within melee range DoCastSelf(41986
-- Anger), else repeat 1s. The task ends after a cast.
-- C++ spell_soul_fragment_anger AfterCast (:230-242): the caster
-- despawns 200ms after the Anger cast.
-- Bridges (all live, verified this round): creature:CastSpell
-- (lua_creature_events.go:954); creature:RemoveAura (:1761 — the
-- RemoveAurasDueToSpell twin, omen precedent); creature:GetVictim
-- (:376); creature:IsWithinMeleeRange (:7017); creature:Despawn
-- (:3318).
-- Documented deltas: the AfterCast despawn ports as an inline 200ms
-- delayed Despawn after the Anger cast (SpellEventAfterCast = 9 also
-- exists, runtime.go:727, firing on the creature path at
-- boss_ai.go:4379 — the inline form is deterministic here since the
-- anger task ends after its one cast either way). UpdateAI remainder
-- (scheduler pump + casting check + melee) is engine-driven; no
-- casting gate is needed because the anger task ends after its cast.

local ANGERED_SOUL_FRAGMENT_ENTRY = 23398
local SPELL_GREATER_INVISIBILITY = 41253
local SPELL_ANGER = 41986

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
    if per == nil then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ GROUP_OUT_OF_COMBAT task: self-cast 41253, schedule the
-- 5-10s removal one-shot, repeat 15-25s.
local function invisFire(creature, guid)
    creature:CastSpell(creature, SPELL_GREATER_INVISIBILITY)
    schedule(guid, "remove", math.random(5000, 10000), function()
        creature:RemoveAura(SPELL_GREATER_INVISIBILITY)
    end)
    schedule(guid, "invis", math.random(15000, 25000), function() invisFire(creature, guid) end)
end

-- C++ Reset (:185-203): CancelAll + the 1s first-fire invis task.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "invis", 1000, function() invisFire(creature, guid) end)
end

-- C++ JustEngagedWith anger task: 1s tick, cast 41986 on self once
-- the victim is in melee range, then despawn 200ms (AfterCast twin).
local function angerFire(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(creature, SPELL_ANGER)
        schedule(guid, "despawn", 200, function() creature:Despawn() end)
    else
        schedule(guid, "anger", 1000, function() angerFire(creature, guid) end)
    end
end

-- C++ JustEngagedWith (:204-217): strip 41253, cancel the
-- out-of-combat group, schedule the 1s anger task.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:RemoveAura(SPELL_GREATER_INVISIBILITY)
    schedule(guid, "anger", 1000, function() angerFire(creature, guid) end)
end

local function onLeaveCombatOrDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ANGERED_SOUL_FRAGMENT_ENTRY, 23, onReset)
RegisterCreatureEvent(ANGERED_SOUL_FRAGMENT_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ANGERED_SOUL_FRAGMENT_ENTRY, 2, onLeaveCombatOrDied)
RegisterCreatureEvent(ANGERED_SOUL_FRAGMENT_ENTRY, 4, onLeaveCombatOrDied)
