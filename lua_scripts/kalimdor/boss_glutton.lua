-- Razorfen Downs: Glutton — Lua port of
-- src/server/scripts/Kalimdor/RazorfenDowns/boss_glutton.cpp (class
-- boss_glutton : public CreatureScript { boss_gluttonAI : public
-- BossAI(creature, DATA_GLUTTON) }; GetAI via GetRazorfenDownsAI
-- (DATA_GLUTTON = 2, razorfen_downs.h:33); AddSC_boss_glutton at
-- end registers the one script; kalimdor loader decl 71 / call 184
-- follows AddSC_boss_mordresh_fire_eye()). Entry 8567
-- tauri-cited (shoot.tauri.hu/?npc=8567 = Glutton, Razorfen; no
-- NPC_ constant in the C++ tree — vishas precedent). Eluna
-- creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnKilledUnit, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee engine-driven in Go (DoMeleeAttackIfReady).
-- Ported arms:
-- - Engage (C++ JustEngagedWith): Talk(SAY_AGGRO 0) + arm the 1s
--   health pump (the C++ UpdateAI polls HealthBelowPct every tick;
--   1s pump is the honest approximation — shade_of_aran
--   precedent).
-- - 50% (C++ UpdateAI !hp50 && HealthBelowPct(50)): Talk(SAY_HP50
--   2) one-shot latch.
-- - 15% (C++ UpdateAI !hp15 && HealthBelowPct(15)): Talk(SAY_HP15
--   3) + DoCast(me, SPELL_FRENZY 12795) self-cast, non-triggered
--   (2-arg DoCast); one-shot latch.
-- - KilledUnit (event 3): Talk(SAY_SLAY 1); C++ has no victim
--   gate, so the port is ungated — faithful.
-- - Reset (event 23): clears the hp50/hp15 latches + timers like
--   C++ Reset.
-- Unmodeled (documented-only, no bridges):
-- - SPELL_DISEASE_CLOUD 12627 is declared in the C++ enum but never
--   cast by this AI (no arm anywhere in boss_glutton.cpp) —
--   correctly omitted, like C++'s declared-but-unused enums.
-- - BossAI ctor / _Reset / _JustDied instance bookkeeping — no
--   instance bridge (standing).

local ENTRY = 8567

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_HP50  = 2
local SAY_HP15  = 3

local SPELL_FRENZY = 12795

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ UpdateAI polls HealthBelowPct(50)/(15) every tick; the 1s
-- pump is the honest approximation (shade_of_aran precedent).
local function healthPump(creature, guid, st)
    if creature:GetHealthPct() <= 50 and not st.hp50 then
        st.hp50 = true
        creature:Talk(SAY_HP50)
    end
    if creature:GetHealthPct() <= 15 and not st.hp15 then
        st.hp15 = true
        creature:Talk(SAY_HP15)
        creature:CastSpell(creature, SPELL_FRENZY)
    end
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    local st = { hp50 = false, hp15 = false }
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_SLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
