-- The Black Morass: Temporus — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- boss_temporus.cpp (SD%Complete: 75, "More abilities need to be
-- implemented"; boss_temporusAI : public BossAI(creature, TYPE_TEMPORUS);
-- GetAI via GetBlackMorassAI<boss_temporusAI> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_boss_temporus at end
-- registers the one script; kalimdor loader decl 43 / call 156 per
-- kalimdor_script_loader.cpp — third "// CoT The Black Morass" loader
-- group, right after AddSC_boss_chrono_lord_deja()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_temporus" (loader decl/call lines only otherwise).
-- Entry: the_black_morass.h:61 names NPC_TEMPORUS = 17880 and
-- instance_the_black_morass.cpp:62 places it in the RiftWaves table as
-- the wave-4 portal boss with a 140s NextPortalTime
-- (SummonedPortalBoss) — the name-to-entry tie is C++-verified
-- (summon-strength evidence, the aeonus pattern); the
-- creature_template ScriptName binding stays DB-side. Not GUID-bound in
-- OnCreatureCreate (only NPC_MEDIVH is), so no ramstein-strength GUID
-- leg.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The C++ while(ExecuteEvent()) +
-- HasUnitState(UNIT_STATE_CASTING) gate has no casting-state bridge in
-- the Lua API (aeonus precedent — unmodeled).
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer
--   at its C++ ScheduleEvent cooldown. The IsHeroic() arm is
--   unbridgeable (no difficulty bridge), so SPELL_REFLECTION is never
--   scheduled here (see unmodeled section).
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_SLAY (3); C++ has NO TYPEID gate here (like deja, unlike
--   aeonus), so the talk is unconditional (shade_of_aran convention).
-- - Haste 31458 self-cast (C++ DoCast(me)), non-triggered, init
--   {15s,23s} -> {20s,25s}.
-- - Mortal Wound 31464 self-cast (C++ DoCast(me)), non-triggered, init
--   8s -> {10s,20s}.
-- - Wing Buffet 31475 self-cast (C++ DoCast(me)), non-triggered, init
--   {25s,35s} -> {20s,30s}.
-- Unmodeled (documented-only, no bridges):
-- - EVENT_SPELL_REFLECTION (SPELL_REFLECT 38592, heroic-only; the C++
--   enum carries the comment "//Not Implemented (Heroic mod)"): C++
--   DoCast(me, 38592), non-triggered, init 30s -> {25s,35s}, scheduled
--   only under IsHeroic(). No heroic/difficulty bridge exists in the
--   Lua API, so the arm is documented, not wired (the standing
--   heroic-unmodeled case).
-- - MoveInLineOfSight's Time Keeper leg: who->GetTypeId() ==
--   TYPEID_UNIT (creature, not player) && who->GetEntry() ==
--   NPC_TIME_KEEPER (17918, the_black_morass.h:57) &&
--   IsWithinDistInMap(who, 20.0f) -> Talk SAY_BANISH (2) +
--   Unit::DealDamage full-health one-shot. Same as aeonus/deja: the
--   Lua API exposes no nearby-creature enumeration, so there is no
--   MoveInLineOfSight / entry-proximity bridge.
-- - JustDied's instance leg: instance->SetData(TYPE_RIFT = 2, SPECIAL)
--   — instance-data bridge blocked (standing).
-- - H_SPELL_WING_BUFFET 38593 is declared in the C++ enum but C++
--   UpdateAI always casts the non-heroic 31475 via DoCast(me) — no
--   heroic variant arm exists to port (the deja H_SPELL_ARCANE_BLAST
--   case).
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++
--   (the archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - C++ Reset() is a no-op; BossAI::_Reset instance bookkeeping is
--   blocked (standing), so the engage re-arm collapses into onReset's
--   cancel (hyjal.lua convention).

local ENTRY = 17880

local SAY_ENTER  = 0
local SAY_AGGRO  = 1
local SAY_BANISH = 2
local SAY_SLAY   = 3
local SAY_DEATH  = 4

local SPELL_HASTE          = 31458
local SPELL_MORTAL_WOUND   = 31464
local SPELL_WING_BUFFET    = 31475
local H_SPELL_WING_BUFFET  = 38593
local SPELL_REFLECT        = 38592

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

-- C++ EVENT_HASTE: DoCast(me, 31458), non-triggered, init {15s,23s}
-- -> {20s,25s}.
local function onHaste(creature, guid)
    creature:CastSpell(creature, SPELL_HASTE)
    schedule(guid, "haste", math.random(20000, 25000), function()
        onHaste(creature, guid)
    end)
end

-- C++ EVENT_MORTAL_WOUND: DoCast(me, 31464), non-triggered, init 8s
-- -> {10s,20s}.
local function onMortalWound(creature, guid)
    creature:CastSpell(creature, SPELL_MORTAL_WOUND)
    schedule(guid, "mortalwound", math.random(10000, 20000), function()
        onMortalWound(creature, guid)
    end)
end

-- C++ EVENT_WING_BUFFET: DoCast(me, 31475), non-triggered, init
-- {25s,35s} -> {20s,30s}.
local function onWingBuffet(creature, guid)
    creature:CastSpell(creature, SPELL_WING_BUFFET)
    schedule(guid, "wingbuffet", math.random(20000, 30000), function()
        onWingBuffet(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "haste", math.random(15000, 23000), function()
        onHaste(creature, guid)
    end)
    schedule(guid, "mortalwound", 8000, function()
        onMortalWound(creature, guid)
    end)
    schedule(guid, "wingbuffet", math.random(25000, 35000), function()
        onWingBuffet(creature, guid)
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
