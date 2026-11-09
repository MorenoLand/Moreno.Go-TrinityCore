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
-- DoAction ACTION_UTHER_START_SCREAM (:2486-2492, action 1) — PORTED
--   2026-11-09 via the ev38 DoAction bridge + the area-trigger sender
--   (at_hor_uther_quel_delar_start 5660, at_hor_triggers.lua): the
--   instance SetData(DATA_QUEL_DELAR_EVENT, SPECIAL) leg rides
--   instanceData:SetData (event-8 handler stores quelDelarState) and
--   EVENT_UTHER_2 schedules at 0s; the E1..E5 chain below runs it.
--   The old "area-trigger sender, no bridge" note is retracted.
-- EVENT_UTHER_1..5 (:2527-2545) — PORTED 2026-11-09: E1 Talk(16)
--   rides the Reset gate (ev23, quelDelarState IN_PROGRESS/SPECIAL);
--   E2 schedules E3 at 2s (the bunny 70698 cast stays out: the
--   invoker GUID is written only by the unported intro trigger's
--   SetGuidData leg); E3 schedules E4 at 2s (the Quel'Delar summon
--   stays out: no instance SummonCreature bridge); E4 Talk(17) +
--   E5 at 8s; E5 MovePoint(1, UtherQuelDelarMovement[0]) via :MoveTo
--   (the MovePoint twin). EVENT_UTHER_FACING/KNEEL (:2572-2579) —
--   PORTED 2026-11-09 via ev28 (the old "no ev28 bridge" claim was
--   stale: skadi rides ev28 live): FACING SetFacingToObject(bunny)
--   (lua_creature_events.go:4742) + KNEEL at 1s via EmoteState(68)
--   (SharedDefines.h:1986). MovementInform (:2502-2512) is the ev28
--   handler below.
--
-- DOCUMENTED-ONLY:

local ENTRY_UTHER = 37225
local ACTION_UTHER_START_SCREAM = 1 -- halls_of_reflection.cpp:2388
local ACTION_UTHER_OUTRO = 2
local DATA_QUEL_DELAR_EVENT = 14 -- halls_of_reflection.h:55
local BOSS_STATE_IN_PROGRESS = 1 -- InstanceScript.h:68-76
local BOSS_STATE_SPECIAL = 4
local SAY_UTHER_QUEL_DELAR_1 = 16 -- halls_of_reflection.cpp:2406
local SAY_UTHER_QUEL_DELAR_2 = 17
local EMOTE_STATE_KNEEL = 68 -- SharedDefines.h:1986
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36
local SPELL_QUEL_DELARS_WILL = 70698 -- halls_of_reflection.cpp:2422
local SPELL_ESSENCE_OF_CAPTURED_1 = 73036
local UTHER_OUTRO_DEST = { 5314.350, 1993.440, 707.726 }
local UTHER_SCREAM_DEST = { 5336.830, 1981.700, 709.319 } -- UtherQuelDelarMovement[0], :2442

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

-- EVENT_UTHER_1..5 chain timers, GUID-keyed like the outro chain;
-- cancelled on ev4/ev23, the C++ _events.Reset() twin.
local screamTimers = {}

local function cancelScream(guid)
    local id = screamTimers[guid]
    if id then
        RemoveEventById(id)
        screamTimers[guid] = nil
    end
end

-- C++ EVENT_UTHER_2..5 (:2531-2545). Delay on a step is the wait
-- before the NEXT step runs (the intro-chain runSteps idiom):
-- E2 at 0s, E3 +2s, E4 +2s, E5 +8s; E5's MoveTo drives ev28.
local SCREAM_STEPS = {
    { fn = function(creature)
        -- EVENT_UTHER_2 (:2531-2536): the bunny 70698 cast on the
        -- invoker stays out (the invoker GUID is written only by the
        -- unported intro trigger's SetGuidData leg); the E3 schedule
        -- below is unconditional.
    end, delay = 2000 },
    { fn = function(creature)
        -- EVENT_UTHER_3 (:2537-2540): SummonCreature(NPC_QUEL_DELAR
        -- 37158) stays out (no instance SummonCreature bridge).
    end, delay = 2000 },
    { fn = function(creature)
        -- EVENT_UTHER_4 (:2541-2544): Talk(SAY_UTHER_QUEL_DELAR_2 17).
        creature:Talk(SAY_UTHER_QUEL_DELAR_2)
    end, delay = 8000 },
    { fn = function(creature)
        -- EVENT_UTHER_5 (:2545): MovePoint(1,
        -- UtherQuelDelarMovement[0]) via :MoveTo, the MovePoint twin;
        -- the FACING/KNEEL legs ride ev28 below.
        creature:MoveTo(1, UTHER_SCREAM_DEST[1], UTHER_SCREAM_DEST[2], UTHER_SCREAM_DEST[3])
    end },
}

-- Forward-declared: the driver re-arms itself through CreateLuaEvent.
local runScreamSteps
runScreamSteps = function(creature, guid, index)
    local step = SCREAM_STEPS[index]
    if not step then
        screamTimers[guid] = nil
        return
    end
    step.fn(creature)
    if step.delay then
        screamTimers[guid] = CreateLuaEvent(function()
            screamTimers[guid] = nil
            runScreamSteps(creature, guid, index + 1)
        end, step.delay)
    else
        screamTimers[guid] = nil
    end
end

-- C++ DoAction ACTION_UTHER_START_SCREAM (:2490-2492): instance
-- SetData(DATA_QUEL_DELAR_EVENT, SPECIAL) + EVENT_UTHER_2 at 0s.
-- ACTION_UTHER_OUTRO (:2494-2496): schedule EVENT_UTHER_6 at 0s.
local function utherDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_UTHER_START_SCREAM then
        cancelScream(guid)
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data:SetData(DATA_QUEL_DELAR_EVENT, BOSS_STATE_SPECIAL)
            end
        end
        runScreamSteps(creature, guid, 1)
        return
    end
    if action ~= ACTION_UTHER_OUTRO then
        return
    end
    cancelOutro(guid)
    runOutroSteps(creature, guid, 1)
end

-- C++ MovementInform (:2502-2512): EVENT_UTHER_FACING (:2572-2576)
-- SetFacingToObject(bunny) + EVENT_UTHER_KNEEL (:2577-2579) at 1s.
local function utherMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE or pointId ~= 1 then
        return
    end
    local bunny = nil
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            bunny = data.frostmourneAltarBunny
        end
    end
    if bunny then
        creature:SetFacingToObject(bunny)
    end
    local guid = creature:GetGUIDLow()
    screamTimers[guid] = CreateLuaEvent(function()
        screamTimers[guid] = nil
        creature:EmoteState(EMOTE_STATE_KNEEL)
    end, 1000)
end

local function cancelChainsOnDeath(event, creature)
    local guid = creature:GetGUIDLow()
    cancelOutro(guid)
    cancelScream(guid)
end

-- C++ Reset (:2471-2478): the gate passes only while
-- DATA_QUEL_DELAR_EVENT is IN_PROGRESS or SPECIAL — then _events
-- resets and EVENT_UTHER_1 schedules at 1ms (Talk 16). ev23 fires
-- on spawn too, where the state is NOT_STARTED and the gate fails.
local function utherReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelOutro(guid)
    cancelScream(guid)
    local mapObj = creature:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local state = data.quelDelarState or 0
    if state ~= BOSS_STATE_IN_PROGRESS and state ~= BOSS_STATE_SPECIAL then
        return
    end
    screamTimers[guid] = CreateLuaEvent(function()
        screamTimers[guid] = nil
        creature:Talk(SAY_UTHER_QUEL_DELAR_1)
    end, 1)
end

-- C++ DamageTaken :2480-2484 — return the replacement damage as the
-- second return per the fireCreatureDamageTaken convention.
local function utherDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

-- C++ spell_hor_quel_delars_will (70698, :2859-2888): OnEffectHitTarget
-- EFFECT_0 SPELL_EFFECT_FORCE_CAST — the hit unit casts the DBC
-- trigger spell on the caster, triggered. DORMANT until Spell.dbc
-- ships 70698 (spell:GetTriggerSpell reads 0 without the row) and
-- until the E2 bunny-cast leg lands (the invoker GUID is unknowable).
RegisterSpellEvent(SPELL_QUEL_DELARS_WILL, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not hitTarget then
        return
    end
    local trigger = spell:GetTriggerSpell()
    if trigger == 0 then
        return
    end
    local caster = spell:GetCaster()
    if not caster then
        return
    end
    hitTarget:CastSpell(caster, trigger, true)
end)

RegisterCreatureEvent(ENTRY_UTHER, 9, utherDamageTaken)
RegisterCreatureEvent(ENTRY_UTHER, 38, utherDoAction)
RegisterCreatureEvent(ENTRY_UTHER, 28, utherMovementInform)
RegisterCreatureEvent(ENTRY_UTHER, 4, cancelChainsOnDeath)
RegisterCreatureEvent(ENTRY_UTHER, 23, utherReset)
RegisterCreatureEvent(ENTRY_UTHER, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- halls_of_reflection.cpp:2514: custom UpdateAI never calls DoMeleeAttackIfReady (RP event machine in C++, never melees)
end)
