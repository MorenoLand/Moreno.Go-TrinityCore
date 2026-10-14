-- Krystallus (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_krystallus.cpp
-- (boss_krystallus (CreatureScript) via
-- GetHallsOfStoneAI<boss_krystallusAI> (BossAI), DATA_KRYSTALLUS
-- = 0 — registered from inside AddSC_boss_krystallus(); loader
-- decl 105 / call 300 per northrend_script_loader.cpp — the SECOND
-- group of the "// Halls of Stone" block in AddNorthrendScripts(),
-- under the "// Ulduar: Halls of Stone" decl marker at loader line
-- 103 and the "// Halls of Stone" call marker at loader line 298,
-- immediately after AddSC_boss_maiden_of_grief() (decl 104 /
-- call 299), which opened the Halls of Stone block; the call
-- after it is AddSC_boss_sjonnir() (decl 106 / call 301) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_maiden_of_grief -> boss_krystallus) is followed).
-- Entry: 27977 Krystallus (halls_of_stone.h NPC_KRYSTALLUS, line 50;
-- DATA_KRYSTALLUS = 0, line 31; GO_KRYSTALLUS doors are not bound
-- in this script) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_krystallus" hits boss_krystallus.cpp + the loader
-- decl/call lines for AddSC_boss_krystallus() only; whole-tree
-- grep for "spell_krystallus_shatter" and
-- "spell_krystallus_shatter_effect" hits boss_krystallus.cpp
-- only; zero sql/ hits for all three. No krystallus lua existed
-- at the time of the first-pass port; this file is the audited
-- artifact of the 21:58 audit run.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough has no bridge — the
-- boss_bookkeeping precedent) + the EVENT_STOMP timer arm below.
-- The Oct-12 "no-timer-bridge" claim is retracted for STOMP:
-- CreateLuaEvent is the registered timer bridge (engine/scripting/
-- runtime.go:798; maiden / bjarngrim-lieutenant convention).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_KILL 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — twentieth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27977):
-- Reset (_Reset() — no bridge).
-- JustEngagedWith ScheduleEvent legs (EVENT_BOULDER_TOSS 3s/9s,
-- EVENT_GROUND_SLAM 15s/18s, EVENT_GROUND_SPIKE 9s/14s
-- heroic-only — no bridge for the reasons below; EVENT_STOMP
-- 20s/29s is PORTED).
-- UpdateAI event machine — EVENT_BOULDER_TOSS (random-target
-- 50.0f DoCast(SPELL_BOULDER_TOSS 50843), 9s/15s — no
-- random-target bridge: CastSpell(nil)=victim,
-- CastSpell(creature)=self, no SelectTarget bridge);
-- EVENT_GROUND_SPIKE (random-target 100.0f
-- DoCast(SPELL_GROUND_SPIKE 59750), 12s/17s — heroic-only, no
-- heroic bridge, plus no random-target bridge);
-- EVENT_GROUND_SLAM (DoCast(SPELL_GROUND_SLAM 50827) + schedules
-- EVENT_SHATTER 10s, 15s/18s — SHATTER (50810) is
-- SpellScript-delivered (see below); porting SLAM alone would
-- leave SPELL_STONED (50812) unshattered — half-machine);
-- EVENT_STOMP — PORTED (DoCast(me, SPELL_STOMP 48131)
-- non-triggered self-cast, {20s,29s} init / {20s,29s} re-arm,
-- zero gating legs — boss_krystallus.cpp:73/:108-110);
-- EVENT_SHATTER (DoCast(SPELL_SHATTER 50810)) — the
-- UNIT_STATE_CASTING gates and the DoMeleeAttackIfReady tail have
-- no bridges. (enum Yells SAY_SHATTER = 3 is declared but never
-- Talk()ed in C++; nothing rides it.)
-- spell_krystallus_shatter (SpellScriptLoader:
-- EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT handler —
-- RemoveAurasDueToSpell(SPELL_STONED 50812) + triggered
-- CastSpell(SPELL_SHATTER_EFFECT 50811) — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue).
-- spell_krystallus_shatter_effect (SpellScriptLoader: OnHit
-- CalculateDamage — radius-scaled SetHitDamage via CalcRadius —
-- no SpellScript bridge; joins the no-SpellScript-bridge queue).

local ENTRY_KRYSTALLUS = 27977

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

local SPELL_STOMP = 48131

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the maiden / bjarngrim-lieutenant convention.
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

-- C++ UpdateAI EVENT_STOMP: DoCast(me, SPELL_STOMP 48131)
-- non-triggered self-cast, {20s,29s} init / {20s,29s} re-arm
-- (boss_krystallus.cpp:73/:108-110) — zero gating legs.
local function stompTick(creature, guid)
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", {20000, 29000}, function()
        stompTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI passthrough +
-- ScheduleEvent legs — the Talk arm and EVENT_STOMP are ported.
local function krystallusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "stomp", {20000, 29000}, function()
        stompTick(creature, guid)
    end)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function krystallusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function krystallusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function krystallusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KRYSTALLUS, 1, krystallusEnterCombat)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 2, krystallusLeaveCombat)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 3, krystallusTargetDied)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 4, krystallusDied)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 23, krystallusLeaveCombat)
