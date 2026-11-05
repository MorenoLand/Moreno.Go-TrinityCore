-- Uther (Quel'Delar, Halls of Reflection) — Lua port of the
-- DamageTaken arm of npc_uther_quel_delar in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :2458-2595; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 37225 NPC_UTHER (halls_of_reflection.h line 65) — the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 9 OnDamageTaken,
-- 23 OnReset, 38 OnDoAction. The DamageTaken handler returns the
-- rewritten damage as the second return (the
-- engine/world/lua_creature_events.go fireCreatureDamageTaken
-- convention); the boolean first return only gates the empty
-- ScriptedAI::DamageTaken base, a provable no-op.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arm):
-- DamageTaken (:2480-2484) — ungated clamp: if damage >= current
--   health, damage = health - 1 ("Prevent to break Uther in intro
--   event during instance encounter", C++ :2471). Event 9 fires
--   pre-application, so creature:GetHealth() is the pre-damage value,
--   exactly like C++'s me->GetHealth() in the hook.
-- DoAction ACTION_UTHER_OUTRO (:2486-2500, action 2) — PORTED
--   2026-11-05 via the ev38 DoAction bridge (tick 206) + the
--   sword-JustDied relay in npc_quel_delar_sword.lua: schedules
--   EVENT_UTHER_6 at 0s, then the timer chain :2546-2571 — E6
--   SetWalk(true) + MovePoint(0, {5314.350, 1993.440, 707.726}) via
--   :MoveTo (the MovePoint twin) + E7 at 5s; E7 Talk(18) + E8 at
--   12s; E8 Talk(19) + E9 at 7s; E9 Talk(20) + E10 at 10s; E10
--   Talk(21) + E11 at 5s; E11 self-cast 73036 triggered +
--   DespawnOrUnsummon(3s) via a 3s CreateLuaEvent -> :Despawn.
--   Only the E11 instance SetData(DATA_QUEL_DELAR_EVENT, DONE) leg
--   has no bridge. The C++ UpdateAI IN_PROGRESS/SPECIAL gate is
--   unmodeled (no instance bridge) but satisfied by construction:
--   the sword exists only as this event's summon, and DONE is set
--   only at E11 downstream. Chain timers are GUID-keyed and
--   cancelled on ev4/ev23 (the C++ _events.Reset() twin).
--
-- DOCUMENTED-ONLY:
-- EVENT_UTHER_1..5 (:2527-2545 — Talk 16/17, the Quel'Delar summon
--   at E3, the E5 MovePoint) ride the Reset gate + DoAction
--   ACTION_UTHER_START_SCREAM (area-trigger sender, no bridge);
--   EVENT_UTHER_FACING/KNEEL (:2572-2579) ride MovementInform (no
--   ev28 bridge) + the instance-GUID bunny fetch. EVENT_UTHER_6..11
--   are PORTED above via the OUTRO DoAction arm.
-- DoAction ACTION_UTHER_START_SCREAM (:2486-2492): the sender is the
--   at_hor_uther_quel_delar_start area trigger (no area-trigger
--   bridge), so the arm stays out; ACTION_UTHER_OUTRO is PORTED
--   above. MovementInform (:2502-2512): no-MovementInform bridge.

local ENTRY_UTHER = 37225
local ACTION_UTHER_OUTRO = 2
local SPELL_ESSENCE_OF_CAPTURED_1 = 73036
local UTHER_OUTRO_DEST = { 5314.350, 1993.440, 707.726 }

-- EVENT_UTHER_6..11 chain timers, GUID-keyed (one outstanding at a
-- time — the chain is linear); cancelled on ev4/ev23, the C++
-- _events.Reset() twin (npc_uther_quel_delarAI::Reset :2471-2478).
local outroTimers = {}

local function cancelOutro(guid)
    local id = outroTimers[guid]
    if id then
        RemoveEventById(id)
        outroTimers[guid] = nil
    end
end

local function outroTalk(id)
    return function(creature)
        creature:Talk(id)
    end
end

-- C++ EVENT_UTHER_6..11 (:2546-2571). Delay on a step is the wait
-- before the NEXT step runs (the intro-chain runSteps idiom):
-- E6 at 0s, E7 +5s, E8 +12s, E9 +7s, E10 +10s, E11 +5s.
local OUTRO_STEPS = {
    { fn = function(creature)
        -- EVENT_UTHER_6 (:2546-2549): SetWalk(true) + MovePoint(0,
        -- UtherQuelDelarMovement[1]) via :MoveTo, the MovePoint twin.
        creature:SetWalk(true)
        creature:MoveTo(0, UTHER_OUTRO_DEST[1], UTHER_OUTRO_DEST[2], UTHER_OUTRO_DEST[3])
    end, delay = 5000 },
    { fn = outroTalk(18), delay = 12000 }, -- EVENT_UTHER_7 (:2550-2553): Talk(18).
    { fn = outroTalk(19), delay = 7000 },  -- EVENT_UTHER_8 (:2554-2557): Talk(19).
    { fn = outroTalk(20), delay = 10000 }, -- EVENT_UTHER_9 (:2558-2561): Talk(20).
    { fn = outroTalk(21), delay = 5000 },  -- EVENT_UTHER_10 (:2562-2565): Talk(21).
    { fn = function(creature)
        -- EVENT_UTHER_11 (:2567-2571): DoCast(me, 73036, true) +
        -- DespawnOrUnsummon(3s); the instance SetData(DONE) leg has
        -- no bridge.
        creature:CastSpell(creature, SPELL_ESSENCE_OF_CAPTURED_1, true)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 3000)
    end },
}

-- Forward-declared: the driver re-arms itself through CreateLuaEvent.
local runOutroSteps
runOutroSteps = function(creature, guid, index)
    local step = OUTRO_STEPS[index]
    if not step then
        outroTimers[guid] = nil
        return
    end
    step.fn(creature)
    if step.delay then
        outroTimers[guid] = CreateLuaEvent(function()
            outroTimers[guid] = nil
            runOutroSteps(creature, guid, index + 1)
        end, step.delay)
    else
        outroTimers[guid] = nil
    end
end

-- C++ DoAction ACTION_UTHER_OUTRO (:2494-2496): schedule
-- EVENT_UTHER_6 at 0s. START_SCREAM (action 1) is ignored — its
-- sender is the unported area trigger.
local function utherDoAction(event, creature, action)
    if action ~= ACTION_UTHER_OUTRO then
        return
    end
    local guid = creature:GetGUID()
    cancelOutro(guid)
    runOutroSteps(creature, guid, 1)
end

local function cancelOutroOnResetOrDeath(event, creature)
    cancelOutro(creature:GetGUID())
end

-- C++ DamageTaken :2480-2484 — return the replacement damage as the
-- second return per the fireCreatureDamageTaken convention.
local function utherDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

RegisterCreatureEvent(ENTRY_UTHER, 9, utherDamageTaken)
RegisterCreatureEvent(ENTRY_UTHER, 38, utherDoAction)
RegisterCreatureEvent(ENTRY_UTHER, 4, cancelOutroOnResetOrDeath)
RegisterCreatureEvent(ENTRY_UTHER, 23, cancelOutroOnResetOrDeath)
RegisterCreatureEvent(ENTRY_UTHER, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- halls_of_reflection.cpp:2514: custom UpdateAI never calls DoMeleeAttackIfReady (RP event machine in C++, never melees)
end)
