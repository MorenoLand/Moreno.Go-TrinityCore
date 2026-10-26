-- Spiritual Reflection (Halls of Reflection) — Lua port of
-- npc_spiritual_reflection in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :1977-2035; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 37068 NPC_REFLECTION (halls_of_reflection.h line 80) — the
-- CreatureScript ScriptName binding is DB-side as usual. (The header
-- notes 37107 for the tank-only variant; no separate CreatureScript
-- exists for it, so only 37068 is registered.)
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent ({min,max} table = randtime
-- range), GUID-keyed, cancelled on events 2/4/23 (the bronjahm /
-- devourer_of_souls convention); creature:CastSpell(nil, spellId) =
-- victim-cast; creature:CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention); the cast here is non-triggered. Melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arms):
-- JustEngagedWith (:1991-1994) — EVENT_BALEFUL_STRIKE at 3000ms
--   (:1992).
-- JustDied (:1996-1999) — DoCastAOE(SPELL_SPIRIT_BURST 69900) (:1998),
--   self-cast (the DoCastAOE convention), event 4.
-- EVENT_BALEFUL_STRIKE (:2013-2019) — random player within 8 yd
--   (:2014), cast SPELL_BALEFUL_STRIKE 69933 (:2015), re-arm
--   {3000,8000}ms (:2016).
--
-- DOCUMENTED-ONLY:
-- Reset (:1987-1990) — _events.Reset() only; covered by the
--   event-2/4/23 timer cancel.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so the timer fires unconditionally (the
-- maiden precedent).

local ENTRY_SPIRITUAL_REFLECTION = 37068

local SPELL_BALEFUL_STRIKE = 69933
local SPELL_SPIRIT_BURST   = 69900

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 8.0f, true)
-- (halls_of_reflection.cpp:2014): a random alive threat-list player
-- within 8 yards.
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < 8 then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_BALEFUL_STRIKE :2013-2019.
local function balefulStrikeTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BALEFUL_STRIKE)
    end
    schedule(guid, "baleful", {3000, 8000}, function() balefulStrikeTick(creature, guid) end)
end

-- C++ JustEngagedWith :1991-1994.
local function reflectionEngaged(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "baleful", 3000, function() balefulStrikeTick(creature, guid) end)
end

-- C++ JustDied :1996-1999 — DoCastAOE(SPELL_SPIRIT_BURST) is self-cast.
local function reflectionDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_SPIRIT_BURST)
end

local function reflectionReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SPIRITUAL_REFLECTION, 1, reflectionEngaged)
RegisterCreatureEvent(ENTRY_SPIRITUAL_REFLECTION, 2, reflectionReset)
RegisterCreatureEvent(ENTRY_SPIRITUAL_REFLECTION, 4, reflectionDied)
RegisterCreatureEvent(ENTRY_SPIRITUAL_REFLECTION, 23, reflectionReset)
