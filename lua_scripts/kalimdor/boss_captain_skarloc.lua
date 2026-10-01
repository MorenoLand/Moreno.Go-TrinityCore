-- Escape from Durnholde Keep: Captain Skarloc — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/EscapeFromDurnholdeKeep/
-- boss_captain_skarloc.cpp (169 lines; boss_captain_skarlocAI :
-- public ScriptedAI; GetAI via GetOldHillsbradAI<boss_captain_skarlocAI>
-- (OHScriptName "instance_old_hillsbrad" gate); AddSC_boss_captain_skarloc
-- at end registers the one script; kalimdor loader decl 35 / call 148
-- per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_captain_skarloc" (loader lines only otherwise).
-- Entry: old_hillsbrad.cpp:151 names ENTRY_SCARLOC = 17862 (C++ spelling
-- "SCARLOC") and :263 summons it during the Thrall escort (the Skarloc
-- meeting event) — the name-to-entry tie is C++-verified; the
-- creature_template ScriptName binding stays DB-side. No
-- instance_old_hillsbrad.cpp GUID case (wave-summoned, not GUID-bound).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith, non-instance leg): Talk SAY_TAUNT1 (1) +
--   Talk SAY_TAUNT2 (2) + arm each timer at its C++ constructor-init
--   cooldown (hyjal.lua convention); the C++ comment "This is not
--   correct. Should taunt Thrall before engage in combat" is ported
--   as-written — the escort-waypoint fix it asks for is SD%Complete: 75
--   territory and needs a movement bridge that does not exist.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_SLAY (3); no TYPEID gate in C++ (shade_of_aran precedent).
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - Holy Light 29427 self-cast (C++ DoCast(me)), non-triggered, init
--   {20s,30s} -> 30s.
-- - Cleanse 29380 self-cast (C++ DoCast(me)), non-triggered, init 10s ->
--   10s.
-- - Hammer of Justice 13005 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init {20s,35s} ->
--   60s.
-- - Holy Shield 31904 self-cast (C++ DoCast(me)), non-triggered, init
--   240s -> 240s.
-- - Devotion Aura 8258 self-cast (C++ DoCast(me)), non-triggered, init
--   3s -> {45s,55s}.
-- Unmodeled (documented-only, no bridges):
-- - JustDied's instance leg: if instance->GetData(TYPE_THRALL_EVENT
--   = 2) == IN_PROGRESS then instance->SetData(TYPE_THRALL_PART1 = 3,
--   DONE) — instance-data bridge blocked (standing); the Thrall escort
--   AI (old_hillsbrad.cpp) is a separate AddSC unit.
-- - SPELL_CONSECRATION 38385: the C++ cast is COMMENTED OUT
--   (//DoCastVictim(SPELL_CONSECRATION)) — only the re-arm
--   urand(5000,10000) executes, so the arm has no observable behavior to
--   port.
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++ (the
--   archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - The SD%Complete: 75 comment's missing adds and the missing
--   spawn-to-Thrall waypoints + pre-combat speech need summon and EscortAI
--   movement bridges that do not exist.

local ENTRY = 17862

local SAY_ENTER  = 0
local SAY_TAUNT1 = 1
local SAY_TAUNT2 = 2
local SAY_SLAY   = 3
local SAY_DEATH  = 4

local SPELL_HOLY_LIGHT        = 29427
local SPELL_CLEANSE           = 29380
local SPELL_HAMMER_OF_JUSTICE = 13005
local SPELL_HOLY_SHIELD       = 31904
local SPELL_DEVOTION_AURA     = 8258

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

-- C++ Holy_Light_Timer: DoCast(me, 29427), non-triggered, init
-- {20s,30s} -> 30s.
local function onHolyLight(creature, guid)
    creature:CastSpell(creature, SPELL_HOLY_LIGHT)
    schedule(guid, "holylight", 30000, function()
        onHolyLight(creature, guid)
    end)
end

-- C++ Cleanse_Timer: DoCast(me, 29380), non-triggered, init 10s -> 10s.
local function onCleanse(creature, guid)
    creature:CastSpell(creature, SPELL_CLEANSE)
    schedule(guid, "cleanse", 10000, function()
        onCleanse(creature, guid)
    end)
end

-- C++ HammerOfJustice_Timer: DoCastVictim(13005), non-triggered, init
-- {20s,35s} -> 60s (jeklik GetVictim + CastSpell convention).
local function onHammerOfJustice(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_HAMMER_OF_JUSTICE)
    end
    schedule(guid, "hammer", 60000, function()
        onHammerOfJustice(creature, guid)
    end)
end

-- C++ HolyShield_Timer: DoCast(me, 31904), non-triggered, init 240s ->
-- 240s.
local function onHolyShield(creature, guid)
    creature:CastSpell(creature, SPELL_HOLY_SHIELD)
    schedule(guid, "holyshield", 240000, function()
        onHolyShield(creature, guid)
    end)
end

-- C++ DevotionAura_Timer: DoCast(me, 8258), non-triggered, init 3s ->
-- {45s,55s}.
local function onDevotionAura(creature, guid)
    creature:CastSpell(creature, SPELL_DEVOTION_AURA)
    schedule(guid, "devotion", math.random(45000, 55000), function()
        onDevotionAura(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_TAUNT1)
    creature:Talk(SAY_TAUNT2)
    schedule(guid, "holylight", math.random(20000, 30000), function()
        onHolyLight(creature, guid)
    end)
    schedule(guid, "cleanse", 10000, function()
        onCleanse(creature, guid)
    end)
    schedule(guid, "hammer", math.random(20000, 35000), function()
        onHammerOfJustice(creature, guid)
    end)
    schedule(guid, "holyshield", 240000, function()
        onHolyShield(creature, guid)
    end)
    schedule(guid, "devotion", 3000, function()
        onDevotionAura(creature, guid)
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
