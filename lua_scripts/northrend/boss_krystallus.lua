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
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1) + the
-- BossAI::JustEngagedWith base arms (SetCombatPulseDelay(5) +
-- me-form DoZoneInCombat, ScriptedCreature.cpp:540/542) ported
-- into krystallusEnterCombat; instance/setActive/ScheduleTasks
-- legs stay out (boss_bookkeeping precedent for those legs).
-- The Oct-12 "no-timer-bridge" claim is retracted for STOMP:
-- CreateLuaEvent is the registered timer bridge (engine/scripting/
-- runtime.go:798; maiden / bjarngrim-lieutenant convention).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_KILL 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — twentieth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4) + _JustDied() SetBossState
-- (DATA_KRYSTALLUS 0, DONE) PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; the
-- boss_moroes.lua / bjarngrim precedent) — the "no bridge" claim
-- was stale.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27977):
-- Reset — _Reset() SetBossState(DATA_KRYSTALLUS, NOT_STARTED)
-- PORTED 2026-11-05 via creature:SetBossState on ev23 (the
-- bjarngrim precedent).
-- JustEngagedWith ScheduleEvent legs (EVENT_BOULDER_TOSS 3s/9s,
-- EVENT_GROUND_SLAM 15s/18s, EVENT_GROUND_SPIKE 9s/14s
-- heroic-only; EVENT_STOMP 20s/29s was PORTED before).
-- RETRACTION (2026-10-17): the "no random-target bridge" claim above
-- is STALE — randomPlayer is the in-tree bridge (anomalus spark /
-- faerlina / eck precedents), and the "no heroic bridge" claim is
-- STALE — creature:IsHeroic() is bridged per-instance
-- (lua_creature_events.go:409-417, cyanigosa / saurfang precedent). So:
-- EVENT_BOULDER_TOSS — PORTED: DoCast(target, SPELL_BOULDER_TOSS
-- 50843) non-triggered on randomPlayerInRange(creature, 50), {3s,9s}
-- init / {9s,15s} repeat, unconditional re-arm (C++-exact);
-- EVENT_GROUND_SPIKE — PORTED: DoCast(target, SPELL_GROUND_SPIKE
-- 59750) non-triggered on randomPlayerInRange(creature, 100),
-- IsHeroic()-gated seed {9s,14s} / {12s,17s} repeat.
-- UpdateAI event machine (remaining):
-- EVENT_GROUND_SLAM — PORTED 2026-11-07 (tick 374): DoCast(me,
-- SPELL_GROUND_SLAM 50827) non-triggered self-cast, {15s,18s} init /
-- {15s,18s} re-arm, schedules EVENT_SHATTER 10s
-- (boss_krystallus.cpp:72/:103-106). The Oct-14 half-machine
-- STAY-OUT is retracted: the 50810 script-effect arm now lives in
-- Go (boss_ai.go creatureEffectScriptEffect + session twin), so the
-- stoned->shatter->damage chain is complete.
-- EVENT_STOMP — PORTED (DoCast(me, SPELL_STOMP 48131)
-- non-triggered self-cast, {20s,29s} init / {20s,29s} re-arm,
-- zero gating legs — boss_krystallus.cpp:73/:108-110);
-- EVENT_SHATTER — PORTED 2026-11-07 (tick 374): DoCast(me,
-- SPELL_SHATTER 50810) non-triggered self-cast, one-shot armed by
-- the slam tick (boss_krystallus.cpp:112-113). The
-- UNIT_STATE_CASTING gate rides the IsCasting 500ms-retry
-- convention shared with stomp/boulder/spike.
-- (enum Yells SAY_SHATTER = 3 is declared but never
-- Talk()ed in C++; nothing rides it.)
-- spell_krystallus_shatter (SpellScriptLoader:
-- EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT handler —
-- RemoveAurasDueToSpell(SPELL_STONED 50812) + triggered
-- CastSpell(SPELL_SHATTER_EFFECT 50811) — PORTED 2026-11-07
-- (tick 374) into creatureEffectScriptEffect + the session twin;
-- the "no SpellScript bridge" claim is retracted for this arm).
-- spell_krystallus_shatter_effect (SpellScriptLoader: OnHit
-- CalculateDamage — radius-scaled SetHitDamage via CalcRadius) —
-- NOT a scaling: the arm multiplies GetHitDamage(), which is 0 at
-- the hook (Spell.cpp:2377), so C++ deals the unmodified base; the
-- tick-372/373 table entries were removed 2026-11-09,
-- engine/world/spell_onhit.go.

local ENTRY_KRYSTALLUS = 27977

-- DATA_KRYSTALLUS = 0 (halls_of_stone.h:31).
local DATA_KRYSTALLUS = 0

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

local SPELL_STOMP = 48131
local SPELL_GROUND_SLAM = 50827
local SPELL_SHATTER = 50810
local SPELL_BOULDER_TOSS = 50843
local SPELL_GROUND_SPIKE = 59750

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

-- C++ SelectTarget(Random, 0, dist, true) draws the threat list, not
-- the instance-player pool (UnitAI.h; Go SortedEntries returns dead
-- entries too, C++ skips offline refs — the bronjahm/devourer/nexus
-- threat-pool fix class): threat-list players alive within dist;
-- offset 0 means the main tank is eligible, so no victim exclusion.
local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_BOULDER_TOSS: DoCast(target, SPELL_BOULDER_TOSS
-- 50843) non-triggered on a SelectTarget(Random, 0, 50.0f, true) target;
-- re-arm unconditional {9s,15s} (C++ re-arms outside the if-target).
local function boulderTossTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "boulder", 500, function()
            boulderTossTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_BOULDER_TOSS)
    end
    schedule(guid, "boulder", math.random(9000, 15000), function()
        boulderTossTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_GROUND_SPIKE: heroic-only (seeded in JustEngagedWith
-- under IsHeroic(); creature:IsHeroic() bridged per-instance —
-- lua_creature_events.go:409-417, the cyanigosa / saurfang precedent);
-- DoCast(target, SPELL_GROUND_SPIKE 59750) non-triggered on a
-- SelectTarget(Random, 0, 100.0f, true) target; re-arm unconditional
-- {12s,17s}.
local function groundSpikeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spike", 500, function()
            groundSpikeTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_GROUND_SPIKE)
    end
    schedule(guid, "spike", math.random(12000, 17000), function()
        groundSpikeTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_STOMP: DoCast(me, SPELL_STOMP 48131)
-- non-triggered self-cast, {20s,29s} init / {20s,29s} re-arm
-- (boss_krystallus.cpp:73/:108-110) — zero gating legs.
local function stompTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            stompTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", {20000, 29000}, function()
        stompTick(creature, guid)
    end)
end

-- Forward declaration (tharon_ja/gal_darah idiom): groundSlamTick's
-- closure schedules shatterTick, which is assigned below; a later
-- `local function shatterTick` would shadow this slot and nil the call.
local shatterTick

-- C++ UpdateAI EVENT_GROUND_SLAM (boss_krystallus.cpp:103-106):
-- DoCast(me, SPELL_GROUND_SLAM 50827) non-triggered self-cast;
-- schedules EVENT_SHATTER 10s and re-arms {15s,18s}. The Stoned
-- (50812) application rides 50827's DBC; the shatter leg below
-- completes the chain (50810 arm now in Go — boss_ai.go
-- creatureEffectScriptEffect + the session twin).
local function groundSlamTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "slam", 500, function()
            groundSlamTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_GROUND_SLAM)
    schedule(guid, "shatter", 10000, function()
        shatterTick(creature, guid)
    end)
    schedule(guid, "slam", {15000, 18000}, function()
        groundSlamTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_SHATTER (boss_krystallus.cpp:112-113):
-- DoCast(me, SPELL_SHATTER 50810) non-triggered self-cast; one-shot,
-- armed only by the slam tick 10s earlier. The 50810 script-effect
-- arm (remove 50812 + triggered 50811 self-cast per hit target)
-- lives in Go; the 50811 damage falloff rides the pre-damage OnHit
-- dispatch (ticks 372/373).
shatterTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shatter", 500, function()
            shatterTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHATTER)
end

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI base arms +
-- ScheduleEvent legs — the Talk, base, and EVENT_STOMP arms ported.
local function krystallusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith Talk-then-base (boss_krystallus.cpp:66-69):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:525-543) —
    -- SetBossState(DATA_KRYSTALLUS, IN_PROGRESS) first, then
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat, after Talk
    -- (C++ arm order); instance/setActive/ScheduleTasks legs out.
    creature:SetBossState(DATA_KRYSTALLUS, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "stomp", {20000, 29000}, function()
        stompTick(creature, guid)
    end)
    schedule(guid, "boulder", {3000, 9000}, function()
        boulderTossTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "spike", {9000, 14000}, function()
            groundSpikeTick(creature, guid)
        end)
    end
    schedule(guid, "slam", {15000, 18000}, function()
        groundSlamTick(creature, guid)
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
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_krystallus.cpp:61-63): _Reset() —
-- SetBossState(DATA_KRYSTALLUS, NOT_STARTED)
-- (ScriptedCreature.cpp:499-511; the bjarngrim precedent sets it
-- unconditionally on ev23).
local function krystallusReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ BossAI::_Reset (ScriptedCreature.cpp:499-510): returns early
    -- when dead (ev23 fires on death too in Go) and zeroes the combat
    -- pulse delay before SetBossState(NOT_STARTED).
    if creature:IsDead() then
        return
    end
    creature:SetCombatPulseDelay(0)
    creature:SetBossState(DATA_KRYSTALLUS, 0)
end

-- C++ JustDied (boss_krystallus.cpp:126-129): Talk(DEATH) then
-- _JustDied() — SetBossState(DATA_KRYSTALLUS, DONE) after Talk.
local function krystallusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_KRYSTALLUS, 3)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_KRYSTALLUS, 1, krystallusEnterCombat)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 2, krystallusLeaveCombat)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 3, krystallusTargetDied)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 4, krystallusDied)
RegisterCreatureEvent(ENTRY_KRYSTALLUS, 23, krystallusReset)
