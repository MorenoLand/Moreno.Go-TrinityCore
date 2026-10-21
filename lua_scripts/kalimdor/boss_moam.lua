-- Ruins of Ahn'Qiraj: Moam — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_moam.cpp
-- (boss_moamAI : public BossAI(creature, DATA_MOAM);
-- GetAI via GetAQ20AI<boss_moamAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_moam at end
-- registers the one script; kalimdor loader decl 81 / call 194 per
-- kalimdor_script_loader.cpp — third "// Ruins of ahn'qiraj" loader
-- group, after rajaxx, before buru).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_moam" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:43 names NPC_MOAM = 15340; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- The C++ commented-out HasUnitState(UNIT_STATE_CASTING) gate has no
-- casting-state bridge in the Lua API (aeonus precedent — unmodeled).
-- Ported arms (the self-contained in-combat legs):
-- - Engage: schedule stone phase at 90s (C++ Reset's
--   ScheduleEvent(EVENT_STONE_PHASE, 90s)); no Talk in C++
--   (boss_moamAI has zero Talk() calls — the Texts enum is unused).
-- - DamageTaken (event 9, moroes convention): not in stone phase and
--   health below 45% -> enter stone phase (C++ DamageTaken leg).
-- - Stone phase entry: self-cast SUMMON_MANA_FIEND 1/2/3 (25681/25682/
--   25683) + ENERGIZE 25685 (C++ DoAction(ACTION_STONE_PHASE_START));
--   schedule stone phase end at 90s.
-- - Stone phase exit: RemoveAura(ENERGIZE 25685) (netherspite
--   RemoveAura convention), schedule next stone phase at 90s
--   (C++ DoAction(ACTION_STONE_PHASE_END)).
-- - UpdateAI mana-full leg: per-tick pump (1s, repeat-forever) checks
--   GetPower(MANA) == GetMaxPower(MANA) (C++ boss_moam.cpp UpdateAI
--   :121-127, after events.Update, before the stone-phase return): mana
--   at max -> end the stone phase (ACTION_STONE_PHASE_END), self-cast
--   ARCANE ERUPTION 25672 (DoCastAOE = self-cast), SetPower(MANA, 0).
--   Dormant: the engine models no power regen, so no fill ever lands.
-- Unmodeled (documented-only, no bridges):
-- - Mana-full -> Arcane Eruption 25672 + SetPower(MANA, 0): wired
--   via the creature:SetPower bridge (lua_creature_events.go) in the
--   UpdateAI pump (C++ order: end stone phase, self-cast eruption, zero
--   power). The engine models no power regen, so the leg stays dormant
--   unless a fill lands.
-- - EVENT_DRAIN_MANA: never scheduled in C++ Reset() (only
--   rescheduled inside its own handler) — dead code, never fires in
--   C++; not ported (C++-verbatim).
-- - SPELL_TRAMPLE 15550: defined but never cast in C++ — dead.
-- - EVENT_WIDE_SLASH / EVENT_TRASH: commented out in C++ — inactive.
-- - BossAI ctor leg DATA_MOAM (ruins_of_ahnqiraj.h:30 = 2) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15340

local SPELL_SUMMON_MANA_FIEND_1 = 25681
local SPELL_SUMMON_MANA_FIEND_2 = 25682
local SPELL_SUMMON_MANA_FIEND_3 = 25683
local SPELL_ENERGIZE = 25685
local SPELL_ARCANE_ERUPTION = 25672 -- boss_moam.cpp:123 DoCastAOE
local POWER_MANA = 0 -- Powers enum (Unit.h): MANA 0

local timers = {}
local moamState = {}
local pumpTimers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelPump(guid)
    local id = pumpTimers[guid]
    if id then
        RemoveEventById(id)
        pumpTimers[guid] = nil
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

-- Forward declaration: startStonePhase's "stonephaseend" closure
-- calls endStonePhase before its definition (malown precedent).
local endStonePhase

-- C++ DoAction(ACTION_STONE_PHASE_START): summon the three mana
-- fiends + Energize, schedule stone phase end at 90s. While
-- _isStonePhase, UpdateAI acts only on EVENT_STONE_PHASE_END — a
-- mid-phase EVENT_STONE_PHASE is silently consumed, never re-fired
-- (boss_moam.cpp UpdateAI) — so drop any still-pending "stonephase"
-- timer here (DamageTaken can trigger the phase before the
-- engage-armed 90s timer elapses).
local function startStonePhase(creature, guid)
    local state = moamState[guid]
    if state == nil then
        return
    end
    local per = timers[guid]
    if per and per["stonephase"] then
        RemoveEventById(per["stonephase"])
        per["stonephase"] = nil
    end
    state.stonePhase = true
    creature:CastSpell(creature, SPELL_SUMMON_MANA_FIEND_1)
    creature:CastSpell(creature, SPELL_SUMMON_MANA_FIEND_2)
    creature:CastSpell(creature, SPELL_SUMMON_MANA_FIEND_3)
    creature:CastSpell(creature, SPELL_ENERGIZE)
    schedule(guid, "stonephaseend", 90000, function()
        endStonePhase(creature, guid)
    end)
end

-- C++ DoAction(ACTION_STONE_PHASE_END): remove Energize, schedule
-- next stone phase at 90s, clear the stone-phase flag.
endStonePhase = function(creature, guid)
    creature:RemoveAura(SPELL_ENERGIZE)
    local state = moamState[guid]
    if state then
        state.stonePhase = false
    end
    schedule(guid, "stonephase", 90000, function()
        startStonePhase(creature, guid)
    end)
end

-- C++ UpdateAI mana-full leg (boss_moam.cpp :121-127): the check
-- runs every UpdateAI tick (after events.Update, before the stone-phase
-- early return), NOT in DamageTaken. The max>0 guard skips the
-- degenerate zero-data case (C++ templates always carry a real max).
local function moamTick(creature, guid)
    local state = moamState[guid]
    if state == nil then
        return
    end
    local maxPower = creature:GetMaxPower()
    if maxPower > 0 and creature:GetPower() == maxPower then
        if state.stonePhase then
            endStonePhase(creature, guid)
        end
        creature:CastSpell(creature, SPELL_ARCANE_ERUPTION)
        creature:SetPower(POWER_MANA, 0)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cancelPump(guid)
    moamState[guid] = { stonePhase = false }
    schedule(guid, "stonephase", 90000, function()
        startStonePhase(creature, guid)
    end)
    -- UpdateAI twin: repeat-forever 1s pump (broggok convention).
    pumpTimers[guid] = CreateLuaEvent(function()
        moamTick(creature, guid)
    end, 1000, 0)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cancelPump(guid)
    moamState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cancelPump(guid)
    moamState[guid] = { stonePhase = false }
    -- C++ Reset() :73: me->SetPower(POWER_MANA, 0) — the meter zeroes on
    -- reset (after _Reset, before Initialize/schedule).
    creature:SetPower(POWER_MANA, 0)
end

-- C++ DamageTaken: !_isStonePhase && HealthBelowPct(45) ->
-- stone phase start. HealthBelowPct reads the PRE-damage health
-- (the damage parameter is unnamed/unused in C++; event 9 fires
-- before damage is applied per engine lua_creature_events.go —
-- kurinnaxx/a1d74e6 convention), so no damage subtraction.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = moamState[guid]
    -- (The mana-full eruption leg lives in the UpdateAI pump above, not
    -- here — C++ boss_moam.cpp :121-127 is inside UpdateAI, after
    -- events.Update and before the stone-phase return.)
    if state == nil or state.stonePhase then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() < math.floor(maxHealth * 45 / 100) then
        startStonePhase(creature, guid)
    end
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cancelPump(guid)
    moamState[guid] = nil
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
