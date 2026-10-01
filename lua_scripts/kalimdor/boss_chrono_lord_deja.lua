-- The Black Morass: Chrono Lord Deja — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- boss_chrono_lord_deja.cpp (SD%Complete: 65, "All abilities not
-- implemented"; boss_chrono_lord_dejaAI : public BossAI(creature,
-- TYPE_CRONO_LORD_DEJA); GetAI via
-- GetBlackMorassAI<boss_chrono_lord_dejaAI> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_boss_chrono_lord_deja at
-- end registers the one script; kalimdor loader decl 42 / call 155
-- per kalimdor_script_loader.cpp — second "// CoT The Black Morass"
-- loader group, right after AddSC_boss_aeonus()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_chrono_lord_deja" (loader decl/call lines only
-- otherwise). Note the C++ constant spelling "CRONO" (the_black_morass.h
-- uses NPC_CRONO_LORD_DEJA / TYPE_CRONO_LORD_DEJA) while the script name
-- spells it "chrono".
-- Entry: the_black_morass.h:60 names NPC_CRONO_LORD_DEJA = 17879 and
-- instance_the_black_morass.cpp:60 places it in the RiftWaves table as
-- the second wave's portal boss (SummonedPortalBoss) — the
-- name-to-entry tie is C++-verified (summon-strength evidence, the
-- aeonus pattern); the creature_template ScriptName binding stays
-- DB-side. Not GUID-bound in OnCreatureCreate (only NPC_MEDIVH is), so
-- no ramstein-strength GUID leg.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The C++ while(ExecuteEvent()) +
-- HasUnitState(UNIT_STATE_CASTING) gate has no casting-state bridge in
-- the Lua API (aeonus precedent — unmodeled).
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer
--   at its C++ ScheduleEvent cooldown. The IsHeroic() arm is
--   unbridgeable (no difficulty bridge), so ATTRACTION is never
--   scheduled here (see unmodeled section).
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_SLAY (3); C++ has NO TYPEID gate here (unlike aeonus, which
--   gates on TYPEID_PLAYER), so the talk is unconditional
--   (shade_of_aran convention).
-- - Arcane Blast 31457 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init {18s,23s}
--   -> {15s,25s}.
-- - Time Lapse: Talk SAY_BANISH (2) + self-cast 31467 (C++ DoCast(me)),
--   non-triggered, init {10s,15s} -> {15s,25s}. This is the only ported
--   Talk arm of SAY_BANISH; the MoveInLineOfSight Time Keeper arm is
--   unreachable (see below).
-- - Arcane Discharge 31472: C++ DoCast(SelectTarget(Random, 0),
--   31472) — unbounded random player; nil target -> no cast
--   (nefarian randomAlivePlayer convention), non-triggered, init
--   {20s,30s} -> {20s,30s}.
-- Unmodeled (documented-only, no bridges):
-- - EVENT_ATTRACTION (SPELL_ATTRACTION 38540, heroic-only): C++
--   DoCast(me, 38540), non-triggered, init {25s,35s} -> {25s,35s},
--   scheduled only under IsHeroic(). No heroic/difficulty bridge exists
--   in the Lua API, so the arm is documented, not wired (the standing
--   heroic-unmodeled case).
-- - MoveInLineOfSight's Time Keeper leg: who->GetTypeId() ==
--   TYPEID_UNIT (creature, not player) && who->GetEntry() ==
--   NPC_TIME_KEEPER (17918, the_black_morass.h:57) &&
--   IsWithinDistInMap(who, 20.0f) -> Talk SAY_BANISH (2) +
--   Unit::DealDamage full-health one-shot. Same as aeonus: the Lua API
--   exposes no nearby-creature enumeration, so there is no
--   MoveInLineOfSight / entry-proximity bridge.
-- - JustDied's instance leg: instance->SetData(TYPE_RIFT = 2, SPECIAL)
--   — instance-data bridge blocked (standing).
-- - H_SPELL_ARCANE_BLAST 38538 and H_SPELL_ARCANE_DISCHARGE 38539 are
--   declared in the C++ enum but C++ UpdateAI always casts the
--   non-heroic 31457 / 31472 via DoCastVictim / DoCast — no heroic
--   variant arm exists to port (the aeonus H_SPELL_SAND_BREATH case).
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++
--   (the archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - C++ Reset() is a no-op; BossAI::_Reset instance bookkeeping is
--   blocked (standing), so the engage re-arm collapses into onReset's
--   cancel (hyjal.lua convention).

local ENTRY = 17879

local SAY_ENTER   = 0
local SAY_AGGRO   = 1
local SAY_BANISH  = 2
local SAY_SLAY    = 3
local SAY_DEATH   = 4

local SPELL_ARCANE_BLAST       = 31457
local H_SPELL_ARCANE_BLAST     = 38538
local SPELL_ARCANE_DISCHARGE   = 31472
local H_SPELL_ARCANE_DISCHARGE = 38539
local SPELL_TIME_LAPSE         = 31467
local SPELL_ATTRACTION         = 38540

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

-- nefarian convention: unbounded random alive player in the instance.
local function randomAlivePlayer(creature)
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

-- C++ EVENT_ARCANE_BLAST: DoCastVictim(31457), non-triggered, init
-- {18s,23s} -> {15s,25s}; jeklik GetVictim + CastSpell convention.
local function onArcaneBlast(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_ARCANE_BLAST)
    end
    schedule(guid, "arcaneblast", math.random(15000, 25000), function()
        onArcaneBlast(creature, guid)
    end)
end

-- C++ EVENT_TIME_LAPSE: Talk SAY_BANISH (2) + DoCast(me, 31467),
-- non-triggered, init {10s,15s} -> {15s,25s}.
local function onTimeLapse(creature, guid)
    creature:Talk(SAY_BANISH)
    creature:CastSpell(creature, SPELL_TIME_LAPSE)
    schedule(guid, "timelapse", math.random(15000, 25000), function()
        onTimeLapse(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_DISCHARGE: DoCast(SelectTarget(Random, 0), 31472),
-- non-triggered, init {20s,30s} -> {20s,30s}; nil target -> no cast.
local function onArcaneDischarge(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_DISCHARGE)
    end
    schedule(guid, "arcanedischarge", math.random(20000, 30000), function()
        onArcaneDischarge(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "arcaneblast", math.random(18000, 23000), function()
        onArcaneBlast(creature, guid)
    end)
    schedule(guid, "timelapse", math.random(10000, 15000), function()
        onTimeLapse(creature, guid)
    end)
    schedule(guid, "arcanedischarge", math.random(20000, 30000), function()
        onArcaneDischarge(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_SLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
