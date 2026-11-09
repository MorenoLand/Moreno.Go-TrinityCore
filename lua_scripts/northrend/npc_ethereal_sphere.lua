-- Ethereal Sphere (Violet Hold) — Lua port of the npc_ethereal_sphere
-- half of src/server/scripts/Northrend/VioletHold/boss_xevozz.cpp
-- (:186-275; CreatureScript via GetVioletHoldAI (ScriptedAI)).
-- Entries: 29271 NPC_ETHEREAL_SPHERE + 32582 NPC_ETHEREAL_SPHERE2
-- (boss_xevozz.cpp:47-48) — entry-keyed dispatch, no RegisterLuaBoss
-- line needed (erekem-guard precedent).
-- DATA_XEVOZZ = 7 (violet_hold.h line 57); Xevozz latches its own
-- GUID under it in xevozzReset (boss_xevozz.lua), so the proximity
-- poll below can read it via GetInstanceCreature.
-- Ported arms (C++-exact for all modeled arms; tick 486):
-- Reset (:205-213) — scheduler cancel + the two self-casts
-- (SPELL_POWER_BALL_VISUAL 54141, SPELL_POWER_BALL_DAMAGE_TRIGGER
-- 54207, both DoCast(me) non-triggered) + DespawnOrUnsummon(40s).
-- The 1s proximity poll (:226-241) — instance->GetCreature
-- (DATA_XEVOZZ) within 3.0f -> DoCastAOE(SPELL_ARCANE_POWER 54160)
-- self-cast + DespawnOrUnsummon(8s), no re-arm after the cast
-- (C++ returns without Repeat()).
-- DoAction ACTION_SUMMON (1) (:213-222) — Talk
-- (SAY_ETHEREAL_SPHERE_SUMMON 0) + DoCastAOE(SPELL_SUMMON_PLAYERS
-- 54164) self-cast. The sender is the boss's 33-35s delayed
-- summons.DoAction(ACTION_SUMMON) leg (boss_xevozz.cpp:173-177);
-- with no summon list in Lua it rides a per-sphere self-timer seeded
-- in Reset at {33s,35s}.
-- DOCUMENTED DEVIATIONS: the per-sphere roll replaces C++'s single
-- shared boss roll (identical with 1 sphere; heroic 2 spheres may
-- drift <= 2s), and the roll origin is sphere spawn vs the boss's
-- cast-issue (identical when the summon spell is instant). The 40s
-- despawn caps the poll at 40 ticks (a stale-timer guard — the poll
-- never outlives the sphere).
-- DOCUMENTED-ONLY: spell_xevozz_summon_players (SpellScript) stays
-- out — no SpellScript bridge.

local NPC_ETHEREAL_SPHERE = 29271
local NPC_ETHEREAL_SPHERE2 = 32582
local DATA_XEVOZZ = 7

local SAY_ETHEREAL_SPHERE_SUMMON = 0

local SPELL_ARCANE_POWER = 54160
local SPELL_SUMMON_PLAYERS = 54164
local SPELL_POWER_BALL_VISUAL = 54141
local SPELL_POWER_BALL_DAMAGE_TRIGGER = 54207

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

-- C++ ScheduledTasks 1s poll (:226-241): instance->GetCreature
-- (DATA_XEVOZZ) within 3.0f -> DoCastAOE(SPELL_ARCANE_POWER) +
-- DespawnOrUnsummon(8s), no re-arm after the cast (C++ returns
-- without Repeat()). The n cap mirrors the 40s Reset despawn.
local function spherePoll(creature, guid, n)
    if n >= 40 or not creature:IsAlive() then
        return
    end
    local xevozz = creature:GetInstanceCreature(DATA_XEVOZZ)
    if xevozz and creature:IsWithinDist(xevozz, 3) then
        creature:CastSpell(creature, SPELL_ARCANE_POWER)
        creature:DespawnOrUnsummon(8000)
        return
    end
    schedule(guid, "poll", 1000, function()
        spherePoll(creature, guid, n + 1)
    end)
end

-- C++ npc_ethereal_sphereAI::DoAction (:213-222): ACTION_SUMMON (1)
-- -> Talk(SAY_ETHEREAL_SPHERE_SUMMON) + DoCastAOE(SPELL_SUMMON_
-- PLAYERS 54164) self-cast. ev38 fires on the DoAction target
-- (lua_creature_events.go:827).
local function sphereDoAction(event, creature, action)
    if action == 1 then
        creature:Talk(SAY_ETHEREAL_SPHERE_SUMMON)
        creature:CastSpell(creature, SPELL_SUMMON_PLAYERS)
    end
end

-- C++ npc_ethereal_sphereAI::Reset (:205-213): scheduler cancel +
-- the two self-casts + DespawnOrUnsummon(40s); the 1s poll seeds
-- here (ScheduledTasks), as does the 33-35s ACTION_SUMMON self-timer
-- (the boss's summons.DoAction leg, boss_xevozz.cpp:173-177 — see
-- header for the deviation). ev23 fires at spawn as well as evade
-- (creaturemotion.go:1262 + lua_creature_events.go:7264).
local function sphereReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_POWER_BALL_VISUAL)
    creature:CastSpell(creature, SPELL_POWER_BALL_DAMAGE_TRIGGER)
    creature:DespawnOrUnsummon(40000)
    schedule(guid, "poll", 1000, function()
        spherePoll(creature, guid, 0)
    end)
    schedule(guid, "summon", {33000, 35000}, function()
        creature:DoAction(1)
    end)
end

-- Shared timer wipe for event 4 (the svala / yogg_saron convention).
local function sphereCancelTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_ETHEREAL_SPHERE, 23, sphereReset)
RegisterCreatureEvent(NPC_ETHEREAL_SPHERE, 4, sphereCancelTimers)
RegisterCreatureEvent(NPC_ETHEREAL_SPHERE, 38, sphereDoAction)
RegisterCreatureEvent(NPC_ETHEREAL_SPHERE2, 23, sphereReset)
RegisterCreatureEvent(NPC_ETHEREAL_SPHERE2, 4, sphereCancelTimers)
RegisterCreatureEvent(NPC_ETHEREAL_SPHERE2, 38, sphereDoAction)
