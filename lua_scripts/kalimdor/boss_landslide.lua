-- Maraudon: Landslide — Lua port of
-- src/server/scripts/Kalimdor/Maraudon/boss_landslide.cpp
-- (SD%Complete: 100; class boss_landslide : public CreatureScript {
-- boss_landslideAI : public ScriptedAI }; GetAI via
-- GetMaraudonAI<boss_landslideAI>; AddSC_boss_landslide at end
-- registers the one script; kalimdor loader decl 61 / call 174 per
-- kalimdor_script_loader.cpp — the second "//Maraudon" loader group,
-- right after AddSC_boss_celebras_the_cursed()).
-- Entry: maraudon.h carries no NPC_ constants and the instance file
-- never names the boss — 12203 is classicdb-cited for "Landslide" (no
-- NPC_ constant in the C++ tree; celebras / vishas / gelihast
-- precedent); the creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage), 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (C++ JustEngagedWith is empty — no Talk): arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm).
-- - Knock Away 18670 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered (C++ 2-arg cast),
--   init 8s -> 15s.
-- - Trample 5568 self-cast (C++ DoCast(me)), non-triggered, init 2s
--   -> 8s.
-- - Landslide 21808 self-cast (C++ DoCast(me)), non-triggered: the C++
--   UpdateAI arm is gated on HealthBelowPct(50) — LandslideTimer starts
--   at 0 and only decrements while below 50%, so the cast fires on the
--   first AI tick after health drops under half, then re-arms 60s of
--   below-50% time (no decrement while above 50%). Modeled with an
--   event-9 crossing latch (projected health < 50% — pre-damage tick
--   granularity is the C++-nearest modelable point) firing the cast
--   immediately, then a 1s pump that decrements the 60s residual only
--   while health stays below 50% (C++-exact: the pump idles without
--   decrementing while above 50% and resumes with the residual).
--   The pre-cast me->InterruptNonMeleeSpells(false) has no
--   spell-interrupt bridge (epoch_hunter / maiden precedent) — the
--   cast still fires.
-- Unmodeled (documented-only, no bridges):
-- - The C++ file declares zero Talk lines and no KilledUnit override
--   — no event-3 registration.

local ENTRY = 12203

local SPELL_KNOCKAWAY = 18670
local SPELL_TRAMPLE   = 5568
local SPELL_LANDSLIDE = 21808

local timers = {}
local landslide = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    landslide[guid] = nil
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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ KnockAway: DoCastVictim(18670), non-triggered, init 8s -> 15s.
local function onKnockAway(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_KNOCKAWAY)
    end
    schedule(guid, "knockaway", 15000, function()
        onKnockAway(creature, guid)
    end)
end

-- C++ Trample: DoCast(me, 5568), non-triggered, init 2s -> 8s.
local function onTrample(creature, guid)
    creature:CastSpell(creature, SPELL_TRAMPLE)
    schedule(guid, "trample", 8000, function()
        onTrample(creature, guid)
    end)
end

-- C++ Landslide pump: fires 21808 (non-triggered self-cast) when the
-- 60s residual expires, but the residual only counts down while health
-- is below 50% (the C++ UpdateAI arm is entirely inside the
-- HealthBelowPct(50) gate).
local function landslidePump(creature, guid)
    local st = landslide[guid]
    if not st then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() < maxHealth * 0.5 then
        st.timer = st.timer - 1000
        if st.timer <= 0 then
            creature:CastSpell(creature, SPELL_LANDSLIDE)
            st.timer = 60000
        end
    end
    schedule(guid, "landslide_pump", 1000, function()
        landslidePump(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "knockaway", 8000, function()
        onKnockAway(creature, guid)
    end)
    schedule(guid, "trample", 2000, function()
        onTrample(creature, guid)
    end)
end

-- C++: LandslideTimer starts at 0 and the HealthBelowPct(50) gate
-- makes the first cast land on the first AI tick after health drops
-- below half. Eluna event 9 is pre-damage, so the crossing uses
-- projected health (jeklik convention); the cast fires at the crossing
-- and the pump then tracks the 60s residual with C++ tick semantics.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if landslide[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() - damage < maxHealth * 0.5 then
        landslide[guid] = { timer = 60000 }
        creature:CastSpell(creature, SPELL_LANDSLIDE)
        schedule(guid, "landslide_pump", 1000, function()
            landslidePump(creature, guid)
        end)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
