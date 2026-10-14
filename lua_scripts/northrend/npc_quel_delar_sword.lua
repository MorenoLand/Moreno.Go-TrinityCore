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
-- 23 OnReset. Timers via CreateLuaEvent, GUID-keyed, cancelled on
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
-- EVENT_QUEL_DELAR_BLADESTORM (:2697-2702) — self-cast
--   SPELL_BLADESTORM 67541 (:2701), re-arm 10000ms (:2702).
-- EVENT_QUEL_DELAR_HEROIC_STRIKE (:2703-2706) — victim-cast
--   SPELL_HEROIC_STRIKE 29426 (:2705), re-arm 6000ms (:2706).
-- EVENT_QUEL_DELAR_WHIRLWIND (:2707-2710) — DoCastAOE(SPELL_WHIRLWIND
--   67716) (:2709), self-cast, re-arm 1000ms (:2710).
--
-- DOCUMENTED-ONLY:
-- The quest flight machine — EVENT_QUEL_DELAR_INIT (:2667-2672),
--   EVENT_QUEL_DELAR_FLIGHT_INIT (:2673-2675), EVENT_QUEL_DELAR_FLIGHT
--   (:2676-2680), EVENT_QUEL_DELAR_LAND (:2681-2686),
--   EVENT_QUEL_DELAR_FIGHT (:2687-2695) with the MoveTakeoff /
--   MoveCirclePath / MoveLand / MovePoint legs and the frostmourne-bunny
--   Talk (:2669) — no movement bridge; the SetImmuneToAll(true)
--   pre-fight leg (:2614-2619, cleared at :2693) has no bridge either,
--   so a sword engaged before its scripted landing would cast earlier
--   than C++ — a fidelity gap on the unmodeled gating, not a
--   fabricated cast (the arms themselves are engage-gated in C++ too).
-- Reset's DoCast(SPELL_WHIRLWIND_VISUAL 70300) (:2614-2616) fires on
--   the C++ spawn path; Go fires events 23/5 only on the respawn path
--   (engine/world/lua_creature_events.go:368-395), so the leg is
--   unmodeled rather than fabricated on a wrong hook.
-- JustDied (:2629-2633) — uther->AI()->DoAction(ACTION_UTHER_OUTRO)
--   via instance GetGuidData: no-DoAction / no-instance-GetGuidData
--   bridges. MovementInform (:2635-2645): no-MovementInform bridge.
-- The ctor's SetDisplayId(Modelid2) (:2610) and
--   SetSpeedRate(MOVE_FLIGHT, 4.5f) (:2615) have no bridges.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (the maiden
-- precedent).

local ENTRY_QUEL_DELAR = 37158

local SPELL_BLADESTORM  = 67541
local SPELL_HEROIC_STRIKE = 29426
local SPELL_WHIRLWIND   = 67716

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
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "heroic", 4000, function() heroicStrikeTick(creature, guid) end)
    schedule(guid, "bladestorm", 6000, function() bladestormTick(creature, guid) end)
    schedule(guid, "whirlwind", 6000, function() whirlwindTick(creature, guid) end)
end

local function swordReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_QUEL_DELAR, 1, swordEngaged)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 2, swordReset)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 4, swordReset)
RegisterCreatureEvent(ENTRY_QUEL_DELAR, 23, swordReset)
