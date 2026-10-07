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
--   (BossAI::JustEngagedWith instance leg blocked — standing.)
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
--   <=60%: Talk(SAY_SUMMON60 1) + frost-spectres summon
--   documented-only; <=50%: Talk(SAY_HP 3); <=30%:
--   Talk(SAY_SUMMON30 2) + frost-spectres summon documented-only.
--   One-shot latches cleared on Reset like C++ Initialize().
-- Unmodeled (documented-only, no bridges):
-- - Frost Spectres 12642 summon arms (60% + 30%): the C++ cast is
--   DoCastVictim(12642) but the spell's whole point is the summon
--   effect — no summon bridge (jeklik/venoxis summon-STRAND
--   precedent); Talk fires, cast skipped.
-- - BossAI ctor / _Reset / _JustDied instance bookkeeping — no
--   instance bridge (standing).
-- - The C++ UNIT_STATE_CASTING early-return gates in UpdateAI —
--   mal_ganis precedent, no casting-state bridge.

local ENTRY = 7358

local SAY_AGGRO    = 0
local SAY_SUMMON60 = 1
local SAY_SUMMON30 = 2
local SAY_HP       = 3
local SAY_KILL     = 4

local SPELL_AMNENNARSWRATH = 13009
local SPELL_FROSTBOLT      = 15530
local SPELL_FROST_NOVA     = 15531

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
    castVictim(creature, SPELL_AMNENNARSWRATH)
    schedule(guid, "wrath", 12000, function()
        onAmnennarsWrath(creature, guid)
    end)
end

-- C++ EVENT_FROSTBOLT: DoCastVictim(15530), non-triggered; init
-- 1s -> repeat 8s unconditional.
local function onFrostbolt(creature, guid)
    castVictim(creature, SPELL_FROSTBOLT)
    schedule(guid, "frostbolt", 8000, function()
        onFrostbolt(creature, guid)
    end)
end

-- C++ EVENT_FROST_NOVA: DoCast(me, 15531) self-cast,
-- non-triggered; init {10s,15s} -> repeat 15s unconditional.
local function onFrostNova(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_NOVA)
    schedule(guid, "frostnova", 15000, function()
        onFrostNova(creature, guid)
    end)
end

-- C++ UpdateAI polls HealthBelowPct every tick; the 1s pump is the
-- honest approximation (shade_of_aran precedent). Frost Spectres
-- 12642 summon arms are documented-only (no summon bridge).
local function healthPump(creature, guid, st)
    local pct = creature:GetHealthPct()
    if pct <= 60 and not st.hp60 then
        st.hp60 = true
        creature:Talk(SAY_SUMMON60)
        -- DoCastVictim(12642) summon effect: no bridge, not wired.
    end
    if pct <= 50 and not st.hp50 then
        st.hp50 = true
        creature:Talk(SAY_HP)
    end
    if pct <= 30 and not st.hp30 then
        st.hp30 = true
        creature:Talk(SAY_SUMMON30)
        -- DoCastVictim(12642) summon effect: no bridge, not wired.
    end
    schedule(guid, "pump", 1000, function()
        healthPump(creature, guid, st)
    end)
end

-- C++ JustEngagedWith: arm timers + Talk(SAY_AGGRO).
local function onEnterCombat(event, creature, target)
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
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim and victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
