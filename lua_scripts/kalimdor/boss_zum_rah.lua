-- Zul'Farrak: Zum'rah — Lua port of
-- src/server/scripts/Kalimdor/ZulFarrak/boss_zum_rah.cpp (class
-- boss_zum_rah : CreatureScript("boss_zum_rah") {
-- boss_zum_rahAI : public BossAI(creature, DATA_ZUM_RAH) }; GetAI via
-- GetZulFarrakAI<boss_zum_rahAI> (zulfarrak.h — GetInstanceAI<AI>(obj,
-- "instance_zulfarrak") gate, DATA_ZUM_RAH = 0 of ZFDataTypes,
-- ZFScriptName "instance_zulfarrak", DataHeader "ZF"); AddSC_boss_zum_rah
-- at end registers the one script; kalimdor loader decl 102 / call 215
-- per kalimdor_script_loader.cpp — the FIRST "// Zul'Farrak" loader
-- group). Whole-server-tree "zum_rah" grep hits only the ZulFarrak dir
-- files + loader (sole-source verified); zero sql/ hits.
-- Entry: zulfarrak.h ZFEntries names ENTRY_ZUM_RAH = 7271 (alongside
-- the ENTRY_BLY/RAVEN/ORO/WEEGLI/MURTA pyramid entries) — kalecgos
-- check PASSES; creature_template ScriptName binding stays DB-side.
-- lua_scripts/kalimdor/ holds no zum_rah lua (overnight window did
-- not outrun this one). Eluna creature events: 1 OnEnterCombat, 2
-- OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. No C++
-- HasUnitState(UNIT_STATE_CASTING) gates in this file.
-- Ported arms:
-- - JustEngagedWith: Talk(SAY_SANCT_INVADE = 0), Shadow Bolt 12739
--   at 1s repeating 4s, Shadowbolt Volley 15245 at 10s repeating 9s.
-- - Shadow Bolt: DoCastVictim non-triggered, reschedule 4s unconditional
--   (C++ Repeat sits outside any gate — faithful).
-- - Shadowbolt Volley: SelectTarget(Random, 0) -> random alive player
--   in the instance, no distance limit (thespia precedent — C++ dist
--   arg is the 0.0f default); nil pick casts nothing; DoCast(target,
--   15245) non-triggered; reschedule 9s unconditional.
-- - KilledUnit: Talk(SAY_KILL = 2) (C++ has no player gate — faithful;
--   event 3 is wired — nalorakk DISCOVERY holds).
-- - HP machine (C++ UpdateAI sequential ifs): via DamageTaken (event 9)
--   on pre-damage health like C++ HealthBelowPct (moroes convention);
--   one-shot flags reset in Reset like C++ Initialize():
--   HealthBelowPct(80): Talk(SAY_WARD = 1) + one-shot Ward of Zum'rah
--   11086 self-cast at 1s; HealthBelowPct(40): same again;
--   HealthBelowPct(30): one-shot Healing Wave 12491 self-cast at 3s.
--   All three can fire on the same tick — same sequential-check order
--   as C++.
-- Unmodeled (documented-only, no bridges):
-- - Reset: me->SetFaction(FACTION_FRIENDLY) — no SetFaction bridge;
--   the areatrigger leg that flips it hostile is in zulfarrak.cpp.
-- - JustDied: instance->SetData(DATA_ZUM_RAH, DONE) — no
--   instance-data bridge (standing); BossAI _JustDied ctor leg blocked.

local ENTRY = 7271

local SAY_SANCT_INVADE = 0
local SAY_WARD         = 1
local SAY_KILL         = 2

local SPELL_SHADOW_BOLT       = 12739
local SPELL_SHADOWBOLT_VOLLEY = 15245
local SPELL_WARD_OF_ZUM_RAH   = 11086
local SPELL_HEALING_WAVE      = 12491

local timers = {}
local hpFlags = {}

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

-- thespia convention: random alive player in the instance, no distance
-- limit (C++ SelectTarget(Random, 0) carries the 0.0f dist default).
local function randomPlayerInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_SHADOW_BOLT: DoCastVictim(SPELL_SHADOW_BOLT), non-triggered;
-- reschedule 4s unconditional.
local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadowbolt", 4000, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ EVENT_SHADOWBOLT_VOLLEY: SelectTarget(Random, 0) ->
-- DoCast(target, 15245), non-triggered; reschedule 9s unconditional.
local function onShadowboltVolley(creature, guid)
    local target = randomPlayerInInstance(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOWBOLT_VOLLEY)
    end
    schedule(guid, "volley", 9000, function()
        onShadowboltVolley(creature, guid)
    end)
end

-- C++ HP machine: HealthBelowPct(80/40) -> Talk(SAY_WARD) + one-shot
-- EVENT_WARD_OF_ZUM_RAH at 1s; HealthBelowPct(30) -> one-shot
-- EVENT_HEALING_WAVE at 3s. Both ward arms self-cast 11086; the heal
-- arm self-casts 12491. Non-triggered; no reschedules (one-shots).
-- DamageTaken fires pre-application, so pre-damage health matches C++
-- HealthBelowPct exactly (moroes convention).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local flags = hpFlags[guid]
    if flags == nil then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local pct = (creature:GetHealth() - damage) * 100 / maxHealth
    if not flags.ward80 and pct < 80 then
        flags.ward80 = true
        creature:Talk(SAY_WARD)
        schedule(guid, "ward80", 1000, function()
            creature:CastSpell(creature, SPELL_WARD_OF_ZUM_RAH)
        end)
    end
    if not flags.ward40 and pct < 40 then
        flags.ward40 = true
        creature:Talk(SAY_WARD)
        schedule(guid, "ward40", 1000, function()
            creature:CastSpell(creature, SPELL_WARD_OF_ZUM_RAH)
        end)
    end
    if not flags.heal30 and pct < 30 then
        flags.heal30 = true
        schedule(guid, "heal30", 3000, function()
            creature:CastSpell(creature, SPELL_HEALING_WAVE)
        end)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_SANCT_INVADE)
    schedule(guid, "shadowbolt", 1000, function()
        onShadowBolt(creature, guid)
    end)
    schedule(guid, "volley", 10000, function()
        onShadowboltVolley(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    hpFlags[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    hpFlags[guid] = { ward80 = false, ward40 = false, heal30 = false }
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
