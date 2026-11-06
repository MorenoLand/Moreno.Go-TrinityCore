-- Razorfen Downs: Amnennar the Coldbringer — Lua port of
-- src/server/scripts/Kalimdor/RazorfenDowns/
-- boss_amnennar_the_coldbringer.cpp (class
-- boss_amnennar_the_coldbringer : public CreatureScript {
-- boss_amnennar_the_coldbringerAI : public BossAI(creature,
-- DATA_AMNENNAR_THE_COLD_BRINGER) }; GetAI via GetRazorfenDownsAI;
-- AddSC_boss_amnennar_the_coldbringer at end registers the one
-- script; kalimdor loader decl 72 / call 185 follows
-- AddSC_boss_glutton() (call :184), closing the "// Razorfen
-- Downs" boss group). Entry 7358 tauri-cited
-- (shoot.tauri.hu/?npc=7358 = Amnennar the Coldbringer; no NPC_
-- constant in the C++ tree — vishas precedent). Eluna creature
-- events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKilledUnit, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- engine-driven in Go (DoMeleeAttackIfReady).
-- Ported arms:
-- - Engage (C++ JustEngagedWith): arm Amnennar's Wrath at 8s,
--   Frostbolt at 1s, Frost Nova at {10s,15s}; Talk(SAY_AGGRO 0).
--   (BossAI::JustEngagedWith pulse + me-DoZone + SetBossState
--   (DATA_AMNENNAR_THE_COLD_BRINGER, IN_PROGRESS) PORTED, bridge
--   live lua_creature_events.go:5917; the CheckRequiredBosses
--   SEQUENCE_BREAK leg stays blocked.)
-- - Death (C++ JustDied, _JustDied): SetBossState
--   (DATA_AMNENNAR_THE_COLD_BRINGER, DONE) PORTED in onDied.
-- - Reset (event 23): the _Reset() SetBossState(NOT_STARTED) leg is
--   PORTED (alive-gated + != DONE-gated, BossAI::_Reset
--   ScriptedCreature.cpp:499-511); the Initialize() latch clears ride
--   the fresh per-engage `st` table.
-- - Amnennar's Wrath (EVENT_AMNENNARSWRATH): DoCastVictim
--   (SPELL_AMNENNARSWRATH 13009), non-triggered (2-arg
--   DoCastVictim); init 8s -> repeat 12s unconditional.
-- - Frostbolt (EVENT_FROSTBOLT): DoCastVictim(SPELL_FROSTBOLT
--   15530), non-triggered; init 1s -> repeat 8s unconditional.
-- - Frost Nova (EVENT_FROST_NOVA): DoCast(me, SPELL_FROST_NOVA
--   15531) self-cast, non-triggered; init {10s,15s} -> repeat 15s
--   unconditional.
-- - KilledUnit (event 3): Talk(SAY_KILL 4) gated on the victim
--   being a player (C++ TYPEID_PLAYER gate — terestian_illhoof
--   player-gated convention).
-- - Health latches (C++ UpdateAI polls every tick; the 1s pump is
--   the honest approximation — shade_of_aran precedent):
--   <=60%: Talk(SAY_SUMMON60 1) + DoCastVictim frost-spectres summon
--   12642 (non-triggered, bridged via the castVictim helper); <=50%:
--   Talk(SAY_HP 3); <=30%: Talk(SAY_SUMMON30 2) + DoCastVictim
--   frost-spectres summon 12642.
--   One-shot latches cleared on Reset like C++ Initialize().
-- Unmodeled (documented-only, no bridges):
-- - The BossAI ctor DATA_AMNENNAR_THE_COLD_BRINGER = 3 registration
--   is the boss id above; _Reset / _JustDied instance bookkeeping
--   beyond the SetBossState legs (events.Reset / summons.DespawnAll /
--   scheduler.CancelAll) needs no bridge (tick-driven AIs; the
--   per-file cancelTimers covers the timer half).
-- - The C++ UNIT_STATE_CASTING gates are PORTED (own pre-loop +
--   in-loop gates: the 3 timer arms defer 500ms while IsCasting, and
--   the health pump skips latch evaluation while IsCasting — the
--   latches sit after the event loop in C++ UpdateAI, so a CASTING
--   return skips them that tick; KilledUnit Talk is a separate
--   callback and stays ungated per C++).

local ENTRY = 7358

local DATA_AMNENNAR_THE_COLD_BRINGER = 3 -- razorfen_downs.h:34
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3
-- EncounterState (InstanceScript.h:68-76).
local SAY_AGGRO    = 0
local SAY_SUMMON60 = 1
local SAY_SUMMON30 = 2
local SAY_HP       = 3
local SAY_KILL     = 4

local SPELL_AMNENNARSWRATH = 13009
local SPELL_FROSTBOLT      = 15530
local SPELL_FROST_NOVA     = 15531
local SPELL_FROST_SPECTRES = 12642

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

local function castVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ EVENT_AMNENNARSWRATH: DoCastVictim(13009), non-triggered;
-- init 8s -> repeat 12s unconditional.
local function onAmnennarsWrath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wrath", 500, function()
            onAmnennarsWrath(creature, guid)
        end)
        return
    end
    castVictim(creature, SPELL_AMNENNARSWRATH)
    schedule(guid, "wrath", 12000, function()
        onAmnennarsWrath(creature, guid)
    end)
end

-- C++ EVENT_FROSTBOLT: DoCastVictim(15530), non-triggered; init
-- 1s -> repeat 8s unconditional.
local function onFrostbolt(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostbolt", 500, function()
            onFrostbolt(creature, guid)
        end)
        return
    end
    castVictim(creature, SPELL_FROSTBOLT)
    schedule(guid, "frostbolt", 8000, function()
        onFrostbolt(creature, guid)
    end)
end

-- C++ EVENT_FROST_NOVA: DoCast(me, 15531) self-cast,
-- non-triggered; init {10s,15s} -> repeat 15s unconditional.
local function onFrostNova(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostnova", 500, function()
            onFrostNova(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FROST_NOVA)
    schedule(guid, "frostnova", 15000, function()
        onFrostNova(creature, guid)
    end)
end

-- C++ UpdateAI polls HealthBelowPct every tick; the 1s pump is the
-- honest approximation (shade_of_aran precedent). Frost Spectres
-- 12642 arms are wired via castVictim (the cast bridge falls back to
-- the current victim, i.e. DoCastVictim).
local function healthPump(creature, guid, st)
    if creature:IsCasting() then
        schedule(guid, "pump", 1000, function()
            healthPump(creature, guid, st)
        end)
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        schedule(guid, "pump", 1000, function()
            healthPump(creature, guid, st)
        end)
        return
    end
    -- C++ HealthBelowPct(p) = strict health < floor(maxHealth*p/100)
    -- (Unit::HealthBelowPct truncates); the old float GetHealthPct()<=p
    -- fired 1 HP early at integer boundaries.
    if not st.hp60 and health < math.floor(maxHealth * 60 / 100) then
        creature:Talk(SAY_SUMMON60)
        castVictim(creature, SPELL_FROST_SPECTRES)
        st.hp60 = true
    end
    if not st.hp50 and health < math.floor(maxHealth * 50 / 100) then
        creature:Talk(SAY_HP)
        st.hp50 = true
    end
    if not st.hp30 and health < math.floor(maxHealth * 30 / 100) then
        creature:Talk(SAY_SUMMON30)
        castVictim(creature, SPELL_FROST_SPECTRES)
        st.hp30 = true
    end
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

-- C++ JustEngagedWith: base arms + timers + Talk(SAY_AGGRO). The base
-- SetBossState(IN_PROGRESS) leg (ScriptedCreature.cpp:527-542,
-- base-first in boss_amnennar_the_coldbringer.cpp:71-73) leads, ahead
-- of the pulse / DoZoneInCombat arms below.
local function onEnterCombat(event, creature, target)
    creature:SetBossState(DATA_AMNENNAR_THE_COLD_BRINGER, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; base-first in boss_amnennar_the_coldbringer.cpp:71-73)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "wrath", 8000, function()
        onAmnennarsWrath(creature, guid)
    end)
    schedule(guid, "frostbolt", 1000, function()
        onFrostbolt(creature, guid)
    end)
    schedule(guid, "frostnova", math.random(10000, 15000), function()
        onFrostNova(creature, guid)
    end)
    local st = { hp60 = false, hp50 = false, hp30 = false }
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
    creature:Talk(SAY_AGGRO)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied (boss_amnennar_the_coldbringer.cpp:86-88):
    -- _JustDied() only — BossAI::_JustDied
    -- (ScriptedCreature.cpp:513-520) writes SetBossState
    -- (DATA_AMNENNAR_THE_COLD_BRINGER, DONE).
    creature:SetBossState(DATA_AMNENNAR_THE_COLD_BRINGER, BOSS_STATE_DONE)
end

-- C++ Reset (boss_amnennar_the_coldbringer.cpp:65-67): _Reset() first,
-- then Initialize() (fresh per-engage `st` table above covers the
-- latch half). The _Reset() SetBossState(NOT_STARTED) leg
-- (BossAI::_Reset ScriptedCreature.cpp:499-511) is alive-gated and
-- != DONE-gated (Go fires ev23 on the death path before ev4,
-- moroes/algalon precedent, so both gates are live here).
local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() and creature:GetBossState(DATA_AMNENNAR_THE_COLD_BRINGER) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_AMNENNAR_THE_COLD_BRINGER, BOSS_STATE_NOT_STARTED)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
