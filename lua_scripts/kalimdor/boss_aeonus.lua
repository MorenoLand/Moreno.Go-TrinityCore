-- The Black Morass: Aeonus — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- boss_aeonus.cpp (143 lines; boss_aeonusAI :
-- public BossAI(creature, TYPE_AEONUS); GetAI via
-- GetBlackMorassAI<boss_aeonusAI> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_boss_aeonus at end registers
-- the one script; kalimdor loader decl 41 / call 154
-- per kalimdor_script_loader.cpp — first "// CoT The Black Morass"
-- loader group, right after AddSC_old_hillsbrad()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_aeonus" (loader decl/call lines only otherwise).
-- Entry: the_black_morass.h:62 names NPC_AEONUS = 17881 and
-- instance_the_black_morass.cpp:64 spawns it as the sixth RiftWaves
-- boss (SummonedPortalBoss) with the wave-18 :306 GetEntry() ==
-- NPC_AEONUS medivh-threat special-case — the name-to-entry tie is
-- C++-verified; the creature_template ScriptName binding stays DB-side.
-- Not GUID-bound in OnCreatureCreate (only NPC_MEDIVH is), so no
-- ramstein-strength GUID leg.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer at
--   its C++ ScheduleEvent cooldown.
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   who->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (3)
--   (terestian_illhoof victim:GetObjectType() == "Player" convention).
-- - Sand Breath 31473 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init {15s,30s} -> re-arm
--   {15s,25s}.
-- - Time Stop 31422 on the victim (C++ DoCastVictim), non-triggered, init
--   {10s,15s} -> re-arm {20s,35s}.
-- - Frenzy: Talk EMOTE_FRENZY (5) + self-cast Enrage 37605 (C++
--   DoCast(me)), non-triggered, init {30s,45s} -> re-arm {20s,35s}.
-- Unmodeled (documented-only, no bridges):
-- - MoveInLineOfSight's Time Keeper leg: who->GetTypeId() == TYPEID_UNIT
--   (creature, not player) && who->GetEntry() == NPC_TIME_KEEPER
--   (17918, the_black_morass.h:57) && IsWithinDistInMap(who, 20.0f) ->
--   Talk SAY_BANISH (2) + Unit::DealDamage(me, who, who->GetHealth(),
--   nullptr, DIRECT_DAMAGE, SPELL_SCHOOL_MASK_NORMAL, nullptr, false)
--   (full-health hit, a one-shot kill of the Time Keeper). The Lua API
--   exposes no nearby-creature enumeration, so there is no
--   MoveInLineOfSight / entry-proximity bridge — SAY_BANISH's only Talk
--   arm is unreachable.
-- - JustDied's instance legs: instance->SetData(TYPE_RIFT = 2, DONE) +
--   instance->SetData(TYPE_MEDIVH = 1, DONE) (the "FIXME: later should be
--   removed" comment is C++-verbatim) — instance-data bridge blocked
--   (standing).
-- - SPELL_CLEAVE 40504 is declared in the C++ enum but never cast in
--   C++ — no cast arm exists to port (the lieutenant_drake
--   ExplodingShout_Timer case).
-- - H_SPELL_SAND_BREATH 39049 is declared in the C++ enum but C++
--   UpdateAI casts only the non-heroic 31473 via DoCastVictim — no
--   heroic arm exists to port.
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++ (the
--   archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - C++ Reset() is a no-op; BossAI::_Reset instance bookkeeping is
--   blocked (standing), so the engage re-arm collapses into onReset's
--   cancel (hyjal.lua convention).

local ENTRY = 17881

local SAY_ENTER   = 0
local SAY_AGGRO   = 1
local SAY_BANISH  = 2
local SAY_SLAY    = 3
local SAY_DEATH   = 4
local EMOTE_FRENZY = 5

local SPELL_CLEAVE        = 40504
local SPELL_TIME_STOP     = 31422
local SPELL_ENRAGE        = 37605
local SPELL_SAND_BREATH   = 31473
local H_SPELL_SAND_BREATH = 39049

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

-- C++ EVENT_SANDBREATH: DoCastVictim(31473), non-triggered, init
-- {15s,30s} -> {15s,25s}; jeklik GetVictim + CastSpell convention.
local function onSandBreath(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_SAND_BREATH)
    end
    schedule(guid, "sandbreath", math.random(15000, 25000), function()
        onSandBreath(creature, guid)
    end)
end

-- C++ EVENT_TIMESTOP: DoCastVictim(31422), non-triggered, init
-- {10s,15s} -> {20s,35s}.
local function onTimeStop(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_TIME_STOP)
    end
    schedule(guid, "timestop", math.random(20000, 35000), function()
        onTimeStop(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: Talk EMOTE_FRENZY (5) + DoCast(me, 37605),
-- non-triggered, init {30s,45s} -> {20s,35s}.
local function onFrenzy(creature, guid)
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "frenzy", math.random(20000, 35000), function()
        onFrenzy(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "sandbreath", math.random(15000, 30000), function()
        onSandBreath(creature, guid)
    end)
    schedule(guid, "timestop", math.random(10000, 15000), function()
        onTimeStop(creature, guid)
    end)
    schedule(guid, "frenzy", math.random(30000, 45000), function()
        onFrenzy(creature, guid)
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
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
