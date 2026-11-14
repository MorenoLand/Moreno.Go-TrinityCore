-- Quel'Delar sword (Halls of Reflection) — Lua port of the combat-cast
-- arms of npc_quel_delar_sword in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :2597-2732; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 37158 NPC_QUEL_DELAR (halls_of_reflection.cpp :2453,
-- QuelDelarSword enum) — the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset, 28 OnMovementInform. Timers via CreateLuaEvent,
-- GUID-keyed, cancelled on
-- events 2/4/23 (the bronjahm / devourer_of_souls convention);
-- creature:CastSpell(nil, spellId) = victim-cast (bridge falls back to
-- TargetGUID); creature:CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention); all casts here are non-triggered. Melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arms):
-- JustEngagedWith (:2622-2627) — EVENT_QUEL_DELAR_HEROIC_STRIKE at
--   4000ms (:2623), EVENT_QUEL_DELAR_BLADESTORM at 6000ms (:2624),
--   EVENT_QUEL_DELAR_WHIRLWIND at 6000ms (:2625).
-- JustDied (:2629-2633) — uther->AI()->DoAction(ACTION_UTHER_OUTRO
--   = 2) via FindNearestCreature(37225, 100) (the tick-207 grid twin
--   for instance GetGuidData; the sword is summoned at
--   QuelDelarSummonPos {5298.473, 1994.852} and Uther stands at
--   Movement[0] {5336.830, 1981.700} when the sword dies, ~40 yd
--   away, well inside 100 yd; entry 37225 is unique in the instance
--   at sword-death time — the intro Uther is banished at intro end
--   and the sword exists only as the Quel'Delar Uther's summon).
--   PORTED 2026-11-05 via the ev38 DoAction bridge (tick 206); the
--   receiver chain (EVENT_UTHER_6..11) is ported in
--   npc_uther_quel_delar.lua. Nil = C++ null-check skip.
-- EVENT_QUEL_DELAR_BLADESTORM (:2700-2703) — self-cast
--   SPELL_BLADESTORM 67541 (:2701), re-arm 10000ms (:2702).
-- EVENT_QUEL_DELAR_HEROIC_STRIKE (:2704-2707) — victim-cast
--   SPELL_HEROIC_STRIKE 29426 (:2705), re-arm 6000ms (:2706).
-- EVENT_QUEL_DELAR_WHIRLWIND (:2708-2711) — DoCastAOE(SPELL_WHIRLWIND
--   67716) (:2709), self-cast, re-arm 1000ms (:2710).
-- Reset (:2611-2620) — _events.Reset, SetSpeedRate(MOVE_FLIGHT, 4.5),
--   _intro-gated INIT schedule / SetImmuneToAll(false) clear.
-- EVENT_QUEL_DELAR_INIT / FLIGHT_INIT / FLIGHT / LAND / FIGHT
--   (:2663-2695) — frostmourne-bunny Talk, MoveTakeoff,
--   MoveCirclePath, MoveLand, MoveTo, Talk, SetImmuneToAll(false);
--   MovementInform (:2635-2645) rides ev28.
--
-- FLIGHT-MACHINE NOTES (2026-11-14 port):
-- The _intro latch is per-GUID introDone (false only after INIT
--   runs); Reset's non-intro leg (:2619) clears immunity through it.
--   Flight timers ride their own table: ev23 cancels both tables
--   then re-arms, ev2 cancels combat only (C++ evade re-runs Reset,
--   so the INIT re-arm must survive the leave-combat fire that
--   follows it), ev4 cancels both (Go fires ev23 ahead of ev4 on
--   death, so the death re-arm is wiped). The bunny Talk rides the
--   FindNearestCreature(37704, 100) grid twin (the uther-DoAction
--   precedent: entry 37704 NPC_FROSTMOURNE_ALTAR_BUNNY,
--   halls_of_reflection.h:69, unique in the instance).
--   MoveCirclePath rides the new bridge (MotionMaster.cpp:827-865
--   twin, cyclic single-link chain run); SetSpeedRate(MOVE_FLIGHT,
--   4.5) rides the SetSpeedRate MOVE_FLIGHT=6 bridge.
--
-- DOCUMENTED-ONLY:
-- Reset's DoCast(SPELL_WHIRLWIND_VISUAL 70300) (:2614-2616) — spell
--   70300 unseeded (no Spell.dbc row, no seed-tool row); the cast
--   would be silent until a wago seed lands.
-- First-spawn gap: Go fires ev23/ev5 only on the respawn path
--   (fireCreatureSpawned in lua_creature_events.go), so a freshly
--   summoned sword never runs Reset/INIT — the flight machine
--   starts on respawn, and never starts at all until the instance
--   SummonCreature bridge lands (uther EVENT_UTHER_3 stays out).
-- The ctor's SetDisplayId(Modelid2) (:2610) is display-bridgeable
--   (creature:SetDisplayId) but has no construction/first-spawn hook
--   (event 5 fires respawn-only).
-- (The old header's SetImmuneToAll(true) pre-fight leg (:2614-2619,
--   cleared at :2693) does not exist in the C++ — only
--   SetImmuneToAll(false) at :2619 and :2687 — so its "fidelity gap"
--   claim is retracted as unfounded.)
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (the maiden
-- precedent).

local ENTRY_QUEL_DELAR = 37158
local ENTRY_UTHER = 37225
local ACTION_UTHER_OUTRO = 2

local SPELL_BLADESTORM  = 67541
local SPELL_HEROIC_STRIKE = 29426
local SPELL_WHIRLWIND   = 67716

local MOVE_FLIGHT = 6
local EFFECT_MOTION_TYPE = 16
local POINT_TAKE_OFF = 1
local ENTRY_FROSTMOURNE_ALTAR_BUNNY = 37704 -- halls_of_reflection.h:69
local SAY_FROSTMOURNE_BUNNY = 0 -- halls_of_reflection.cpp:2439
local SAY_QUEL_DELAR_SWORD = 0 -- halls_of_reflection.cpp:2434
local QUEL_DELAR_CENTER = { 5309.259, 2006.390, 718.046 } -- halls_of_reflection.cpp:2443
local QUEL_DELAR_MOVE_TAKEOFF = { 5292.870, 1998.950, 718.046 } -- QuelDelarMovement[0]
local QUEL_DELAR_MOVE_LAND = { 5295.819, 1991.912, 707.707 } -- QuelDelarMovement[1]
local QUEL_DELAR_MOVE_FIGHT = { 5295.301, 1989.782, 708.696 } -- QuelDelarMovement[2]

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
    per[key] = CreateLuaEvent(fn, delay)
end

local flightTimers = {}
local introDone = {}

local function cancelFlightTimers(guid)
    local per = flightTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        flightTimers[guid] = nil
    end
end

local function scheduleFlight(guid, key, delay, fn)
    local per = flightTimers[guid]
    if not per then
        per = {}
        flightTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_QUEL_DELAR_BLADESTORM :2697-2702.
local function bladestormTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    creature:CastSpell(creature, SPELL_BLADESTORM)
    schedule(guid, "bladestorm", 10000, function() bladestormTick(creature, guid) end)
end

-- C++ EVENT_QUEL_DELAR_HEROIC_STRIKE :2703-2706.
local function heroicStrikeTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    creature:CastSpell(nil, SPELL_HEROIC_STRIKE)
    schedule(guid, "heroic", 6000, function() heroicStrikeTick(creature, guid) end)
end

-- C++ EVENT_QUEL_DELAR_WHIRLWIND :2707-2710 — DoCastAOE is self-cast.
local function whirlwindTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", 1000, function() whirlwindTick(creature, guid) end)
end

-- C++ JustEngagedWith :2622-2627.
local function swordEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "heroic", 4000, function() heroicStrikeTick(creature, guid) end)
    schedule(guid, "bladestorm", 6000, function() bladestormTick(creature, guid) end)
    schedule(guid, "whirlwind", 6000, function() whirlwindTick(creature, guid) end)
end

-- C++ EVENT_QUEL_DELAR_FIGHT :2687-2695.
local function fightStep(creature, guid)
    if flightTimers[guid] == nil then
        return
    end
    creature:Talk(SAY_QUEL_DELAR_SWORD)
    creature:MoveTo(0, QUEL_DELAR_MOVE_FIGHT[1], QUEL_DELAR_MOVE_FIGHT[2], QUEL_DELAR_MOVE_FIGHT[3])
    creature:SetImmuneToAll(false)
end

-- C++ EVENT_QUEL_DELAR_LAND :2681-2686.
local function landStep(creature, guid)
    if flightTimers[guid] == nil then
        return
    end
    creature:StopMoving()
    creature:MoveLand(0, QUEL_DELAR_MOVE_LAND[1], QUEL_DELAR_MOVE_LAND[2], QUEL_DELAR_MOVE_LAND[3])
    scheduleFlight(guid, "fight", 6000, function() fightStep(creature, guid) end)
end

-- C++ EVENT_QUEL_DELAR_FLIGHT :2676-2680.
local function flightStep(creature, guid)
    if flightTimers[guid] == nil then
        return
    end
    creature:MoveCirclePath(QUEL_DELAR_CENTER[1], QUEL_DELAR_CENTER[2], QUEL_DELAR_CENTER[3], 18.0, true, 16)
    scheduleFlight(guid, "land", 15000, function() landStep(creature, guid) end)
end

-- C++ MovementInform :2635-2645 — EFFECT_MOTION_TYPE, POINT_TAKE_OFF.
local function swordMovementInform(event, creature, moveType, pointId)
    if moveType ~= EFFECT_MOTION_TYPE or pointId ~= POINT_TAKE_OFF then
        return
    end
    local guid = creature:GetGUIDLow()
    if flightTimers[guid] == nil then
        return
    end
    scheduleFlight(guid, "flight", 0, function() flightStep(creature, guid) end)
end

-- C++ EVENT_QUEL_DELAR_FLIGHT_INIT :2673-2675.
local function flightInitStep(creature, guid)
    if flightTimers[guid] == nil then
        return
    end
    creature:MoveTakeoff(POINT_TAKE_OFF, QUEL_DELAR_MOVE_TAKEOFF[1], QUEL_DELAR_MOVE_TAKEOFF[2], QUEL_DELAR_MOVE_TAKEOFF[3])
end

-- C++ EVENT_QUEL_DELAR_INIT :2667-2672.
local function initStep(creature, guid)
    if flightTimers[guid] == nil then
        return
    end
    local bunny = creature:FindNearestCreature(ENTRY_FROSTMOURNE_ALTAR_BUNNY, 100)
    if bunny then
        bunny:Talk(SAY_FROSTMOURNE_BUNNY)
    end
    introDone[guid] = true
    scheduleFlight(guid, "flightInit", 2500, function() flightInitStep(creature, guid) end)
end

-- C++ Reset :2611-2620 — _events.Reset, SetSpeedRate(MOVE_FLIGHT,
-- 4.5), _intro-gated INIT schedule / SetImmuneToAll(false) clear.
-- (DoCast SPELL_WHIRLWIND_VISUAL 70300 stays out: unseeded.)
local function swordReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelFlightTimers(guid)
    creature:SetSpeedRate(MOVE_FLIGHT, 4.5)
    if not introDone[guid] then
        scheduleFlight(guid, "init", 0, function() initStep(creature, guid) end)
    else
        creature:SetImmuneToAll(false)
    end
end

-- Leave-combat cancels combat timers only (C++ evade re-runs Reset
-- first, so the INIT re-arm must survive the leave-combat fire);
-- death cancels both tables (Go fires ev23 ahead of ev4 on death,
-- so the death re-arm is wiped).
local function swordLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function swordOnDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelFlightTimers(guid)
end

-- C++ JustDied :2629-2633 — uther->AI()->DoAction(ACTION_UTHER_OUTRO).
-- C++ fetches Uther by instance GetGuidData(DATA_UTHER_QUEL_DELAR);
-- FindNearestCreature(37225, 100) is the grid twin (magtheridon /
-- legoso precedent): the sword is summoned at QuelDelarSummonPos
-- {5298.473, 1994.852} and Uther stands at Movement[0] {5336.830,
-- 1981.700} when the sword dies, ~40 yd away, well inside 100 yd;
-- entry 37225 is unique in the instance at that time. Nil = C++
-- null-check skip (the C++ GetCreature may return null).
local function swordDied(event, creature)
    local uther = creature:FindNearestCreature(ENTRY_UTHER, 100)
    if uther then
        uther:DoAction(ACTION_UTHER_OUTRO)
    end
end

RegisterCreatureEvent(ENTRY_QUEL_DELAR, 1, swordEngaged)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 2, swordLeaveCombat)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 4, swordOnDied)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 4, swordDied)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 23, swordReset)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 28, swordMovementInform)
