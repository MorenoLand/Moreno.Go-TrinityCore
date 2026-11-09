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
-- - Engage (C++ JustEngagedWith): BossAI base arms PORTED (head of
--   onEnterCombat) — SetBossState(DATA_GLUTTON, IN_PROGRESS) +
--   pulse + me-DoZone (bridge live, lua_creature_events.go:5917);
--   Talk(SAY_AGGRO 0) + arm the 1s health pump (the C++ UpdateAI
--   polls HealthBelowPct every tick; 1s pump is the honest
--   approximation — shade_of_aran precedent).
-- - Death (C++ JustDied, _JustDied): SetBossState(DATA_GLUTTON,
--   DONE) PORTED in onDied.
-- - Reset (event 23): clears the hp50/hp15 latches + timers like
--   C++ Reset, and the _Reset() SetBossState(NOT_STARTED) leg is
--   PORTED (alive-gated + != DONE-gated, BossAI::_Reset
--   ScriptedCreature.cpp:499-511).
-- - 50% (C++ UpdateAI !hp50 && HealthBelowPct(50)): Talk(SAY_HP50
--   2) one-shot latch.
-- - 15% (C++ UpdateAI !hp15 && HealthBelowPct(15)): Talk(SAY_HP15
--   3) + DoCast(me, SPELL_FRENZY 12795) self-cast, non-triggered
--   (2-arg DoCast); one-shot latch.
-- - KilledUnit (event 3): Talk(SAY_SLAY 1); C++ has no victim
--   gate, so the port is ungated — faithful.
-- Unmodeled (documented-only, no bridges):
-- - SPELL_DISEASE_CLOUD 12627 is declared in the C++ enum but never
--   cast by this AI (no arm anywhere in boss_glutton.cpp) —
--   correctly omitted, like C++'s declared-but-unused enums.
-- - The BossAI ctor DATA_GLUTTON = 2 registration is the boss id
--   above; _Reset / _JustDied instance bookkeeping beyond the
--   SetBossState legs (events.Reset / summons.DespawnAll /
--   scheduler.CancelAll) needs no bridge (tick-driven AIs; the
--   per-file cancelTimers covers the timer half).

local ENTRY = 8567

local DATA_GLUTTON = 2 -- razorfen_downs.h:33
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3
-- EncounterState (InstanceScript.h:68-76).
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
-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() fires up to 1 HP early on the
-- float edge.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

local function healthPump(creature, guid, st)
    if healthBelowPct(creature, 50) and not st.hp50 then
        st.hp50 = true
        creature:Talk(SAY_HP50)
    end
    if healthBelowPct(creature, 15) and not st.hp15 then
        st.hp15 = true
        creature:Talk(SAY_HP15)
        creature:CastSpell(creature, SPELL_FRENZY)
    end
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

local function onEnterCombat(event, creature, target)
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542,
    -- base-first in boss_glutton.cpp:56-58): instance->SetBossState
    -- (DATA_GLUTTON, IN_PROGRESS) ahead of the pulse / DoZoneInCombat
    -- arms below (creature:SetBossState, lua_creature_events.go:5917).
    creature:SetBossState(DATA_GLUTTON, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; base-first in boss_glutton.cpp:56-58)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    local st = { hp50 = false, hp15 = false }
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (boss_glutton.cpp:67-69): _JustDied() only —
    -- BossAI::_JustDied (ScriptedCreature.cpp:513-520) writes
    -- SetBossState(DATA_GLUTTON, DONE).
    creature:SetBossState(DATA_GLUTTON, BOSS_STATE_DONE)
end

-- C++ Reset (boss_glutton.cpp:49-51): _Reset() first, then the hp50 /
-- hp15 latches clear (fresh per-engage `st` table above covers the
-- latch half). The _Reset() SetBossState(NOT_STARTED) leg
-- (BossAI::_Reset ScriptedCreature.cpp:499-511) is alive-gated and
-- != DONE-gated (Go fires ev23 on the death path before ev4,
-- moroes/algalon precedent, so both gates are live here).
local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(DATA_GLUTTON) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_GLUTTON, BOSS_STATE_NOT_STARTED)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_SLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
