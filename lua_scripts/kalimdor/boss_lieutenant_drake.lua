-- Escape from Durnholde Keep: Lieutenant Drake — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/EscapeFromDurnholdeKeep/
-- boss_leutenant_drake.cpp (211 lines; go_barrel_old_hillsbrad
-- GameObjectScript + boss_lieutenant_drakeAI : public ScriptedAI; GetAI
-- via GetOldHillsbradAI<boss_lieutenant_drakeAI> (OHScriptName
-- "instance_old_hillsbrad" gate); AddSC_boss_lieutenant_drake at end
-- registers both scripts; kalimdor loader decl 37 / call 150 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_lieutenant_drake" and "go_barrel_old_hillsbrad" (loader
-- decl/call lines only otherwise).
-- Entry: instance_old_hillsbrad.cpp:39 names DRAKE_ENTRY = 17848 and :149
-- summons it at 2128.43, 71.01, 64.42 when the fifth barrel gossip lands
-- (TYPE_BARREL_DIVERSION = 1, old_hillsbrad.h:28 — the go_barrel script
-- in THIS cpp drives that SetData, so the name-to-entry tie is
-- C++-verified; the creature_template ScriptName binding stays DB-side).
-- Not GUID-bound in instance_old_hillsbrad.cpp (gossip-summoned, not
-- creatureData-bound). The final DrakeWP (2128.20, 70.9763, 64.4221)
-- matches the summon point above.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance/movement legs
-- below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm,
--   hyjal.lua convention).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_SLAY (2); no TYPEID gate in C++ (shade_of_aran precedent).
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (5).
-- - Whirlwind 31909 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init 20s -> 20000 +
--   rand32() % 5000.
-- - Frightening Shout 33789 on the victim (C++ DoCastVictim),
--   non-triggered, init 30s -> 25000 + rand32() % 10000 + Talk
--   SAY_SHOUT (4) before the cast.
-- - Mortal Strike 31911 on the victim (C++ DoCastVictim),
--   non-triggered, init 45s -> 20000 + rand32() % 10000 + Talk
--   SAY_MORTAL (3) before the cast.
-- Unmodeled (documented-only, no bridges):
-- - The patrol machine: C++ sets CanPatrol = true / wpId = 0 in
--   Initialize() and UpdateAI runs, marked "/// @todo make this work",
--   if (CanPatrol && wpId == 0) me->GetMotionMaster()->MovePoint(wpId,
--   DrakeWP[wpId]) then ++wpId — no movement bridge exists, so the
--   CanPatrol/wpId state and these 19 C++-verbatim waypoints are
--   documented here, not wired: (2125.84, 88.2535, 54.8830),
--   (2111.01, 93.8022, 52.6356), (2106.70, 114.753, 53.1965),
--   (2107.76, 138.746, 52.5109), (2114.83, 160.142, 52.4738),
--   (2125.24, 178.909, 52.7283), (2151.02, 208.901, 53.1551),
--   (2177.00, 233.069, 52.4409), (2190.71, 227.831, 53.2742),
--   (2178.14, 214.219, 53.0779), (2154.99, 202.795, 52.6446),
--   (2132.00, 191.834, 52.5709), (2117.59, 166.708, 52.7686),
--   (2093.61, 139.441, 52.7616), (2086.29, 104.950, 52.9246),
--   (2094.23, 81.2788, 52.6946), (2108.70, 85.3075, 53.3294),
--   (2125.50, 88.9481, 54.7953), (2128.20, 70.9763, 64.4221).
-- - ExplodingShout_Timer: Initialize() arms 25000 but UpdateAI never
--   expires it — no cast, Talk, or re-arm exists in C++ (no observable
--   behavior to port).
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++
--   (the archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - go_barrel_old_hillsbrad (GameObjectScript, registered by the same
--   AddSC): OnGossipHello reads instance->GetData(TYPE_BARREL_DIVERSION
--   = 1) and calls instance->SetData(TYPE_BARREL_DIVERSION,
--   IN_PROGRESS) unless DONE, always returning false (gossip not
--   consumed) — no instance-data or gameobject-gossip bridge exists, so
--   the script is documented here, not registered (the five-gossip ->
--   DRAKE summon at instance_old_hillsbrad.cpp:149 is quest-10283
--   territory; SD%Complete: 70 flags exactly this as missing).
-- - The SD%Complete: 70 comment's missing proper post-spawn patrol code
--   needs the movement bridge above.

local ENTRY = 17848

local SAY_ENTER  = 0
local SAY_AGGRO  = 1
local SAY_SLAY   = 2
local SAY_MORTAL = 3
local SAY_SHOUT  = 4
local SAY_DEATH  = 5

local SPELL_WHIRLWIND         = 31909
local SPELL_HAMSTRING         = 9080
local SPELL_MORTAL_STRIKE     = 31911
local SPELL_FRIGHTENING_SHOUT = 33789

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

-- C++ Whirlwind_Timer: DoCastVictim(31909), non-triggered, init
-- 20s -> 20000 + rand32() % 5000; jeklik GetVictim + CastSpell
-- convention.
local function onWhirlwind(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_WHIRLWIND)
    end
    schedule(guid, "whirlwind", 20000 + math.random(0, 4999), function()
        onWhirlwind(creature, guid)
    end)
end

-- C++ Fear_Timer: Talk SAY_SHOUT (4) then DoCastVictim(33789),
-- non-triggered, init 30s -> 25000 + rand32() % 10000.
local function onFear(creature, guid)
    creature:Talk(SAY_SHOUT)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_FRIGHTENING_SHOUT)
    end
    schedule(guid, "fear", 25000 + math.random(0, 9999), function()
        onFear(creature, guid)
    end)
end

-- C++ MortalStrike_Timer: Talk SAY_MORTAL (3) then DoCastVictim(31911),
-- non-triggered, init 45s -> 20000 + rand32() % 10000.
local function onMortalStrike(creature, guid)
    creature:Talk(SAY_MORTAL)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortalstrike", 20000 + math.random(0, 9999), function()
        onMortalStrike(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "whirlwind", 20000, function()
        onWhirlwind(creature, guid)
    end)
    schedule(guid, "fear", 30000, function()
        onFear(creature, guid)
    end)
    schedule(guid, "mortalstrike", 45000, function()
        onMortalStrike(creature, guid)
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
