-- Drakos the Interrogator (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_drakos.cpp
-- (boss_drakos (CreatureScript) via GetOculusAI<boss_drakosAI>
-- (BossAI, DATA_DRAKOS); npc_unstable_sphere (CreatureScript) via
-- GetOculusAI<npc_unstable_sphereAI> (ScriptedAI); AddSC_boss_drakos()
-- at end registers both; loader decl 84 / call 279 per
-- northrend_script_loader.cpp — the FIRST group of the "// The Nexus:
-- The Oculus" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_nexus() (decl 82 / call 277); the call after it is
-- AddSC_boss_urom() — loader order confirmed this run; the
-- checkpoint sequence (instance_nexus -> boss_drakos) is followed).
-- Entry: 27654 Drakos the Interrogator (oculus.h NPC_DRAKOS, line
-- 41; instance_oculus.cpp OnCreatureCreate binds case NPC_DRAKOS
-- (line 59) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- NPC_UNSTABLE_SPHERE (28166) exists only in the .cpp's local Spells
-- enum — local-enum-only like gothik's minions — entry-unverifiable,
-- no registration, no lua for npc_unstable_sphere.
-- Sole-source verified: whole-server-tree grep for "boss_drakos"
-- hits boss_drakos.cpp + the loader decl/call lines only; grep for
-- "npc_unstable_sphere" hits boss_drakos.cpp only (registered from
-- inside AddSC_boss_drakos, no separate loader lines); zero sql/
-- hits. No drakos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the BossAI::
-- JustEngagedWith instance-bookkeeping leg has no instance bridge —
-- tharon_ja precedent). The two cast ScheduleEvent legs are ported
-- as CreateLuaEvent timers seeded here, cancel on 2/4/23 (the
-- krystallus/bjarngrim convention).
-- EVENT_MAGIC_PULL — DoCast(SPELL_MAGIC_PULL 51336) + postPull=true
-- + 15s init / 15s repeat (boss_drakos.cpp:76/:113-116) — PORTED.
-- Targeting resolved via UnitAI::DoCast(uint32) -> AISpellInfo
-- (UnitAI.cpp:113-164): FillAISpellInfo (UnitAI.cpp:189-234) leaves
-- target at AITARGET_SELF unless GetMaxRange(false) != 0 — 51336 is
-- range 0 yards (Self), so the C++ cast is self-cast ->
-- creature:CastSpell(creature, 51336). The pull payload (Dummy
-- server-script effect) is engine-undelivered — documented
-- deviation. postPull kept as a per-GUID latch for machine
-- fidelity (its only consumer, BOMB_SUMMON's count, is unported).
-- EVENT_STOMP — Talk(SAY_STOMP 4) + DoCast(SPELL_THUNDERING_STOMP
-- 50774) + 15s init / 15s repeat (boss_drakos.cpp:77/:118-121) —
-- PORTED, same AISpellInfo resolution — 50774 is range 0 yards
-- (Self) -> self-cast -> creature:CastSpell(creature, 50774).
-- (The 2026-10-14 02:58 "PORTABLE-CONDITIONAL / UNRESOLVED
-- targeting" verdict is retracted: both spells are range-0, so
-- AITARGET_SELF is mechanical, not asserted.)
-- KilledUnit — Talk(SAY_KILL 1) (event 3 — C++-UNCONDITIONAL: the
-- Talk fires for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant).
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied bookkeeping
-- has no bridge — tharon_ja precedent; the instance->
-- DoStartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT,
-- ACHIEV_TIMED_START_EVENT 18153) leg has no achievement bridge —
-- kelthuzad/thaddius precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (postPull=false) + _Reset() — no instance
-- bridge; the cast ScheduleEvent legs are ported above.
-- UpdateAI — EVENT_BOMB_SUMMON: for i <= (postPull ? 3 : 0)
-- SummonCreature(NPC_UNSTABLE_SPHERE 28166, GetRandomNearPosition)
-- + 2s repeat — no summon bridge (summon STRAND absent).
-- npc_unstable_sphere — entry-unverifiable (28166, local-enum-only)
-- — no registration, no lua; its arms are documented here only:
-- Reset — SetReactState(REACT_PASSIVE) + MoveRandom(40.0f) + AddAura(
-- SPELL_UNSTABLE_SPHERE_PASSIVE 50756, SPELL_UNSTABLE_SPHERE_TIMER
-- 50758, self) + DespawnOrUnsummon(19s) — motion-blocked (no motion
-- bridge; react-state / aura / despawn legs are bridged in isolation —
-- SetReactState, AddAura on the event-handler surface
-- (lua_creature_events.go:170), Despawn in lua_objects.go — but the
-- sphere still has no lua because its entry is unverifiable and there
-- is no registration); UpdateAI pulseTimer
-- 3000ms ->
-- DoCast(SPELL_UNSTABLE_SPHERE_PULSE 50757) + 3s repeat — the
-- cast/timer/aura/despawn legs ARE bridged (CastSpell +
-- CreateLuaEvent + AddAura on the event-handler surface, Despawn in
-- lua_objects.go); the sphere has no lua solely because its entry is
-- unverifiable and there is no registration.
-- UpdateAI UNIT_STATE_CASTING suppression gate (boss_drakos.cpp:96,
-- 127): C++ defers ALL event execution while the boss is casting;
-- creature:IsCasting() is hard-coded FALSE (lua_objects.go:326-330),
-- so the 15s Lua timers fire rigidly even mid-cast — no bridge,
-- unfixable, documented here.

local ENTRY_DRAKOS = 27654

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2
local SAY_STOMP = 4

local SPELL_MAGIC_PULL = 51336
local SPELL_THUNDERING_STOMP = 50774

local timers = {}
local postPull = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    postPull[guid] = nil
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

-- C++ UpdateAI EVENT_MAGIC_PULL (boss_drakos.cpp:113-116):
-- DoCast(SPELL_MAGIC_PULL 51336) self-cast (AISpellInfo-resolved,
-- see header) + postPull=true, 15s init -> 15s loop.
local function magicPullTick(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_PULL)
    postPull[guid] = true
    schedule(guid, "pull", 15000, function()
        magicPullTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_STOMP (boss_drakos.cpp:118-121): Talk(SAY_STOMP)
-- + DoCast(SPELL_THUNDERING_STOMP 50774) self-cast, 15s init ->
-- 15s loop.
local function stompTick(creature, guid)
    creature:Talk(SAY_STOMP)
    creature:CastSpell(creature, SPELL_THUNDERING_STOMP)
    schedule(guid, "stomp", 15000, function()
        stompTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) (the BossAI::JustEngagedWith
-- instance-bookkeeping leg has no instance bridge — tharon_ja
-- precedent); the Reset ScheduleEvent legs (boss_drakos.cpp:76-78)
-- are ported as engage-seeded timers (krystallus convention).
local function drakosEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "pull", 15000, function()
        magicPullTick(creature, guid)
    end)
    schedule(guid, "stomp", 15000, function()
        stompTick(creature, guid)
    end)
end

local function drakosLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL) — C++-UNCONDITIONAL: the Talk fires
-- for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant.
local function drakosTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent; the instance->DoStartTimedAchievement
-- (ACHIEV_TIMED_START_EVENT 18153) leg has no achievement bridge —
-- kelthuzad/thaddius precedent).
local function drakosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DRAKOS, 1, drakosEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKOS, 2, drakosLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKOS, 3, drakosTargetDied)
RegisterCreatureEvent(ENTRY_DRAKOS, 4, drakosDied)
RegisterCreatureEvent(ENTRY_DRAKOS, 23, drakosLeaveCombat)
