-- Sjonnir the Ironshaper (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_sjonnir.cpp
-- (boss_sjonnir (CreatureScript) via
-- GetHallsOfStoneAI<boss_sjonnirAI> (BossAI), DATA_SJONNIR = 3
-- — registered from inside AddSC_boss_sjonnir(); loader decl 106
-- / call 301 per northrend_script_loader.cpp — the THIRD group of
-- the "// Halls of Stone" block in AddNorthrendScripts(), under
-- the "// Ulduar: Halls of Stone" decl marker (line 103) and the
-- "// Halls of Stone" call marker (line 298), immediately after
-- AddSC_boss_krystallus() (decl 105 / call 300); the call after
-- it is AddSC_instance_halls_of_stone() — loader order confirmed
-- this run; the checkpoint sequence (boss_krystallus ->
-- boss_sjonnir) is followed).
-- Entry: 27978 Sjonnir the Ironshaper (halls_of_stone.h
-- NPC_SJONNIR, line 51; DATA_SJONNIR = 3, line 34;
-- GO_SJONNIR_DOOR 191296, line 65 — bound in the instance
-- script, not here) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_sjonnir" hits boss_sjonnir.cpp (+ the loader decl/call
-- lines for AddSC_boss_sjonnir()) only; whole-tree grep for
-- "npc_malformed_ooze", "npc_iron_sludge" and
-- "achievement_abuse_the_ooze" hits boss_sjonnir.cpp only; zero
-- sql/ hits for all four. boss_sjonnir.lua is the audited
-- port artifact for this unit (entry 27978 bridgeable arms).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- CheckRequiredBosses / EnterEvadeMode gate has no
-- required-bosses bridge; the BossAI::JustEngagedWith passthrough
-- has no bridge (boss_bookkeeping precedent); the six
-- ScheduleEvent legs seed the timer machine — CreateLuaEvent
-- is the timer bridge (the stale "no timer bridge for the legs"
-- claim is retracted), and the seed moves to OnEnterCombat(1)
-- with cancel on 2/4/23 (krystallus/bjarngrim/halion
-- convention).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious
-- player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player") — twenty-first
-- player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _JustDied() boss
-- bookkeeping passthrough has no bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27978):
-- Reset / Initialize — _Reset() + abuseTheOoze = 0 — no bridge.
-- DoAction(ACTION_OOZE_DEAD 1) / GetData(DATA_ABUSE_THE_OOZE 2)
-- — abuseTheOoze counter — no DoAction / GetData bridge
-- (consumer: achievement_abuse_the_ooze below).
-- UpdateAI event machine (boss_sjonnir.cpp:151-208), PORTED arms:
-- EVENT_LIGHTNING_SHIELD DoCast(me, SPELL_LIGHTING_SHIELD 50831)
-- non-triggered, one-shot 20s/25s, no re-arm — PORTED as a
-- one-shot CreateLuaEvent + CastSpell(creature, 50831).
-- EVENT_STATIC_CHARGE DoCastVictim(SPELL_STATIC_CHARGE 50834)
-- non-triggered, 20s/25s init -> 20s/25s loop — PORTED as
-- CreateLuaEvent + CastSpell(nil, 50834).
-- EVENT_LIGHTNING_RING DoCast(me, SPELL_LIGHTING_RING 51849)
-- non-triggered, 30s/35s init -> 30s/35s loop — PORTED as
-- CreateLuaEvent + CastSpell(creature, 51849) (51849’s 2s
-- periodic trigger of 50841 is spell-data-driven, no script
-- leg). The UNIT_STATE_CASTING gates have no bridge (documented
-- deviation); DoMeleeAttackIfReady is engine-driven.
-- PORTED: EVENT_SUMMON health-tier (75/50/25%) PipeLocations summon
-- machine (FORGED_IRON_DWARF 27982 / FORGED_IRON_TROGG 27979 /
-- MALFORMED_OOZE 27981 / EARTHEN_DWARF 27980,
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN Eluna type 5, 30s, 20s re-arm) — 5s seed
-- in sjonnirEnterCombat + summonTick (the stale "no SummonCreature
-- bridge" claim is retracted — the bridge is creature:SummonCreature
-- + creature:GetHealthPct() for the tier latch);
-- EVENT_FRENZY DoCast(me, SPELL_FRENZY 28747, true) — PORTED (event 1;
-- triggered self-cast one-shot, no re-arm; the stale "no triggered-cast
-- bridge" claim is retracted — the bridge is CastSpell(creature, spell, true).
-- PORTED: EVENT_CHAIN_LIGHTNING (:166-169) — DoCast(target, SPELL_CHAIN_LIGHTING
-- 50830) non-triggered on randomPlayerInRange(creature, 100) (the C++
-- SelectTarget(Random, 0, 100, true) alive-player pool with the 100.0f filter —
-- creature:IsWithinDist is bridged, moragg 50.0f precedent; the
-- cairne/kazzak "no random-target bridge" claim is STALE and retracted); init
-- {3s,8s} randtime, re-arm {10s,15s} unconditional (outside the target-if,
-- C++-exact); creature:CastSpell(target, ...) is the bridgeable shape
-- (the keleseth nil-call sweep — target:CastSpell would nil-call on
-- player targets).
-- npc_malformed_ooze (CreatureScript via
-- GetHallsOfStoneAI<npc_malformed_oozeAI> (ScriptedAI);
-- NPC_MALFORMED_OOZE = 27981, local enum in boss_sjonnir.cpp —
-- entry-verifiable but bridge-blocked, NO registration — the
-- npc_spark_of_ionar / stormforged_lieutenant no-bridgeable-arms
-- precedent; no Talk arms anywhere in the script — the merge
-- machine (FindNearestCreature 27981 3.0f -> DoSpawnCreature
-- IRON_SLUDGE 28165 + double DisappearAndDie; _mergeTimer 10s /
-- 3s) stays out on BEARER grounds only: the proximity / summon /
-- despawn / timer bridges are all live now (FindNearestCreature
-- lua_creature_events.go:2332, SummonCreature :1183, Despawn,
-- CreateLuaEvent runtime.go:798) — the old "no bridges" claim is
-- stale — but the ooze AI itself is unregistered, so the arm has
-- no bearer to run on.
-- npc_iron_sludge (CreatureScript via
-- GetHallsOfStoneAI<npc_iron_sludgeAI> (ScriptedAI);
-- NPC_IRON_SLUDGE = 28165, local enum — entry-verifiable but
-- bridge-blocked, NO registration; no Talk arms anywhere —
-- JustDied -> ObjectAccessor::GetCreature(instance GetGuidData
-- DATA_SJONNIR) -> DoAction(ACTION_OOZE_DEAD) — no
-- ObjectAccessor / GetGuidData / DoAction bridges).
-- achievement_abuse_the_ooze (AchievementCriteriaScript OnCheck:
-- Sjonnir->AI()->GetData(DATA_ABUSE_THE_OOZE) >= 5 — no GetData
-- / achievement bridges; joins the unmodeled-achievement queue).

local ENTRY_SJONNIR = 27978

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_LIGHTING_SHIELD = 50831
local SPELL_STATIC_CHARGE = 50834
local SPELL_LIGHTING_RING = 51849
local SPELL_CHAIN_LIGHTING = 50830
local SPELL_FRENZY = 28747

local NPC_FORGED_IRON_DWARF = 27982
local NPC_FORGED_IRON_TROGG = 27979
local NPC_MALFORMED_OOZE = 27981
local NPC_EARTHEN_DWARF = 27980

-- C++ PipeLocations (boss_sjonnir.cpp:67-71); o=0.0f falls back to the
-- summoner's orientation in C++ (Object.cpp:2061), same as the Go bridge.
local PIPE_LOCS = {
    { 1295.44, 734.07, 200.3, 0.0 },
    { 1297.7, 595.6, 199.9, 0.0 },
}

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

-- C++ SelectTarget(Random, 0, 100, true) draws the threat list, not
-- the instance-player pool (UnitAI.h; C++ skips offline refs, Go
-- SortedEntries returns dead too — the bronjahm/devourer/nexus
-- threat-pool fix class); creature:IsWithinDist is bridged
-- (engine/scripting/object.go:167).
local function randomPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:IsWithinDist(t, dist) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_CHAIN_LIGHTNING (:166-169): DoCast(target, 50830)
-- non-triggered on SelectTarget(Random, 0, 100, true); {3s,8s} init ->
-- {10s,15s} re-arm, unconditional (outside the target-if).
local function chainLightningTick(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTING)
    end
    schedule(guid, "chainLightning", {10000, 15000}, function()
        chainLightningTick(creature, guid)
    end)
end

-- C++ EVENT_STATIC_CHARGE (:173-175): DoCastVictim(50834)
-- non-triggered, 20s/25s init -> 20s/25s loop.
local function staticChargeTick(creature, guid)
    creature:CastSpell(nil, SPELL_STATIC_CHARGE)
    schedule(guid, "staticCharge", {20000, 25000}, function()
        staticChargeTick(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNING_RING (:177-179): DoCast(me, 51849)
-- non-triggered, 30s/35s init -> 30s/35s loop; the 2s periodic
-- trigger of 50841 is spell-data-driven.
local function lightningRingTick(creature, guid)
    creature:CastSpell(creature, SPELL_LIGHTING_RING)
    schedule(guid, "lightningRing", {30000, 35000}, function()
        lightningRingTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON (boss_sjonnir.cpp:181-195): 5s init -> 20s
-- unconditional loop; health-tier entry via HealthAbovePct(75/50/25)
-- (creature:GetHealthPct() is bridged — engine/scripting/object.go
-- generic fallback, the sindragosa precedent); urand(0,1) pipe;
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN (Eluna type 5; the bridge numbers
-- the C++ enum 0-based), 30s.
-- Deviations: C++ JustSummoned moves the summon to CenterPoint
-- (no MovePoint bridge); the add AIs (27982/27979/27981/27980) are
-- unported/unregistered, so summons are inert until those land; the
-- 30s death-timer leg is unmodeled (corpse/dead family 4/5/6 starts
-- its timer at death — documented engine delta).
local function summonTick(creature, guid)
    local pipe = PIPE_LOCS[math.random(1, 2)]
    local hp = creature:GetHealthPct()
    local entry = NPC_EARTHEN_DWARF
    if hp > 75 then
        entry = NPC_FORGED_IRON_DWARF
    elseif hp > 50 then
        entry = NPC_FORGED_IRON_TROGG
    elseif hp > 25 then
        entry = NPC_MALFORMED_OOZE
    end
    creature:SummonCreature(entry, pipe[1], pipe[2], pipe[3], pipe[4], 5, 30000)
    schedule(guid, "summon", 20000, function()
        summonTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) + required-bosses gate +
-- BossAI passthrough + ScheduleEvent legs — only the Talk arm is
-- bridgeable.
local function sjonnirEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
-- C++ JustEngagedWith: EVENT_CHAIN_LIGHTNING {3s,8s} randtime seed.
    schedule(guid, "chainLightning", {3000, 8000}, function()
        chainLightningTick(creature, guid)
    end)
-- C++ EVENT_LIGHTNING_SHIELD (:170-172): DoCast(me, 50831)
-- non-triggered, one-shot 20s/25s, no re-arm.
    schedule(guid, "lightningShield", {20000, 25000}, function()
        creature:CastSpell(creature, SPELL_LIGHTING_SHIELD)
    end)
    schedule(guid, "staticCharge", {20000, 25000}, function()
        staticChargeTick(creature, guid)
    end)
    schedule(guid, "lightningRing", {30000, 35000}, function()
        lightningRingTick(creature, guid)
    end)
-- C++ EVENT_SUMMON 5s seed (:113).
    schedule(guid, "summon", 5000, function()
        summonTick(creature, guid)
    end)
-- C++ EVENT_FRENZY (:196-199): DoCast(me, 28747, true) triggered self-cast,
-- single-fire (no reschedule in the case), seeded at 5min (:114).
    schedule(guid, "frenzy", 300000, function()
        creature:CastSpell(creature, SPELL_FRENZY, true)
    end)
end

local function sjonnirCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function sjonnirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + _JustDied() — only the Talk arm is
-- bridgeable.
local function sjonnirDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SJONNIR, 1, sjonnirEnterCombat)
RegisterCreatureEvent(ENTRY_SJONNIR, 2, sjonnirCombatEnd)
RegisterCreatureEvent(ENTRY_SJONNIR, 3, sjonnirTargetDied)
RegisterCreatureEvent(ENTRY_SJONNIR, 4, sjonnirDied)
RegisterCreatureEvent(ENTRY_SJONNIR, 23, sjonnirCombatEnd)
