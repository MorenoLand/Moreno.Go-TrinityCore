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
-- required-bosses bridge; the BossAI::JustEngagedWith base arms
-- (SetBossState(DATA_SJONNIR, IN_PROGRESS) + SetCombatPulseDelay(5) +
-- me-form DoZoneInCombat, ScriptedCreature.cpp:525-543) are ported
-- into sjonnirEnterCombat; instance legs stay out; the six
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
-- JustDied — Talk(SAY_DEATH 2) (event 4) + _JustDied() SetBossState
-- (DATA_SJONNIR 3, DONE) PORTED 2026-11-05 via creature:SetBossState
-- (lua_creature_events.go:5917; the boss_moroes.lua / bjarngrim
-- precedent) — the "no bridge" claim was stale.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27978):
-- Reset / Initialize — _Reset() + abuseTheOoze = 0 — PORTED 2026-11-05
-- in sjonnirCombatEnd (ev2/ev23 share the handler; EnterEvadeMode ends
-- with Reset(), CreatureAI.cpp:237); the _Reset() SetBossState
-- (DATA_SJONNIR, NOT_STARTED) leg is PORTED 2026-11-05 in sjonnirReset
-- below (ev23 only, the bjarngrim precedent — ev2 leave-combat does
-- not call Reset in C++).
-- DoAction(ACTION_OOZE_DEAD 1) / GetData(DATA_ABUSE_THE_OOZE 2)
-- — abuseTheOoze counter — PORTED 2026-11-05 in sjonnirDoAction (ev38)
-- + the iron-sludge JustDied relay below (boss_sjonnir.cpp:291);
-- consumer achievement_abuse_the_ooze ported (see below).
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
-- leg). The UNIT_STATE_CASTING gates (own UpdateAI pre-loop :158 +
-- in-loop :204) are PORTED as a 500ms IsCasting defer in every timer
-- tick (mal_ganis idiom; IsCasting bridged lua_creature_events.go:1950);
-- DoMeleeAttackIfReady is engine-driven.
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
-- entry-verifiable; no Talk arms anywhere — the merge machine
-- (FindNearestCreature 27981 3.0f -> DoSpawnCreature
-- IRON_SLUDGE 28165 + double DisappearAndDie; _mergeTimer 10s /
-- 3s) is PORTED (tick 515) in
-- lua_scripts/northrend/npc_malformed_ooze.lua, registered in
-- boss_ai.go. The old "no proximity / summon / despawn / timer
-- bridges" claim was stale: all four bridges are live
-- (FindNearestCreature lua_creature_events.go:6094, SummonCreature
-- :3718, Despawn :3024, CreateLuaEvent runtime.go:798).
-- npc_iron_sludge (CreatureScript via
-- GetHallsOfStoneAI<npc_iron_sludgeAI> (ScriptedAI);
-- NPC_IRON_SLUDGE = 28165, local enum — entry-verifiable; no Talk arms
-- anywhere — JustDied -> ObjectAccessor::GetCreature(instance GetGuidData
-- DATA_SJONNIR) -> DoAction(ACTION_OOZE_DEAD) — PORTED 2026-11-05 in
-- ironSludgeDied (dispatch ev38 + lookup GetInstanceCreature bridged;
-- boss GUID latched by the ev5 SetGuidData in this file; entry 28165
-- registered in boss_ai.go).
-- achievement_abuse_the_ooze (AchievementCriteriaScript OnCheck:
-- Sjonnir->AI()->GetData(DATA_ABUSE_THE_OOZE) >= 5) — PORTED 2026-11-05:
-- checkAchievementCriteriaScript reads the victim motion's SetData latch
-- (engine/world/achievement_criteria_scripts.go), threaded from the
-- kill-credit path (kill.go creditQuestKills).

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
local NPC_IRON_SLUDGE = 28165

-- DATA_SJONNIR = 3 (halls_of_stone.h:34); DATA_ABUSE_THE_OOZE = 2 /
-- ACTION_OOZE_DEAD = 1 (boss_sjonnir.cpp:53-55).
local DATA_SJONNIR = 3
local DATA_ABUSE_THE_OOZE = 2
local ACTION_OOZE_DEAD = 1

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
-- threat-pool fix class); the gate is the IsWithinCombatRange twin
-- (Unit.cpp:583; the raw fallback IsWithinDist lacks both reaches).
local function randomPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
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
    if creature:IsCasting() then
        schedule(guid, "chainLightning", 500, function()
            chainLightningTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "staticCharge", 500, function()
            staticChargeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_STATIC_CHARGE)
    schedule(guid, "staticCharge", {20000, 25000}, function()
        staticChargeTick(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNING_RING (:177-179): DoCast(me, 51849)
-- non-triggered, 30s/35s init -> 30s/35s loop; the 2s periodic
-- trigger of 50841 is spell-data-driven.
local function lightningRingTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lightningRing", 500, function()
            lightningRingTick(creature, guid)
        end)
        return
    end
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
-- (MovePoint twin: summon:MoveTo(0, CenterPoint), sjonnirJustSummoned
-- below); the add AIs (27982/27979/27980) are unported/unregistered,
-- so those summons are inert until they land — the malformed ooze
-- (27981) merge machine IS ported (tick 515), so ooze summons merge
-- into iron sludges; the 30s death-timer leg is unmodeled
-- (corpse/dead family 4/5/6 starts its timer at death — documented
-- engine delta).
local function summonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon", 500, function()
            summonTick(creature, guid)
        end)
        return
    end
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
-- BossAI base arms + ScheduleEvent legs — Talk + base arms ported,
-- gate leg unbridged.
-- C++ EVENT_LIGHTNING_SHIELD (:170-172): DoCast(me, 50831)
-- non-triggered, one-shot 20s/25s, no re-arm; IsCasting defer
-- (500ms, cast not consumed) per the UpdateAI gates.
local function lightningShieldTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lightningShield", 500, function()
            lightningShieldTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_LIGHTING_SHIELD)
end

-- C++ EVENT_FRENZY (:196-199): DoCast(me, 28747, true) triggered
-- self-cast, single-fire; IsCasting defer likewise.
local function frenzyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            frenzyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FRENZY, true)
end

local function sjonnirEnterCombat(event, creature, target)
    -- C++ JustEngagedWith gate-then-base (boss_sjonnir.cpp:98-107):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:525-543) —
    -- SetBossState(DATA_SJONNIR, IN_PROGRESS) first, then
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat, before Talk
    -- (C++ arm order); CheckRequiredBosses/EnterEvadeMode gate and
    -- instance/setActive/ScheduleTasks legs out.
    creature:SetBossState(DATA_SJONNIR, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
-- C++ JustEngagedWith: EVENT_CHAIN_LIGHTNING {3s,8s} randtime seed.
    schedule(guid, "chainLightning", {3000, 8000}, function()
        chainLightningTick(creature, guid)
    end)
-- C++ EVENT_LIGHTNING_SHIELD (:170-172): DoCast(me, 50831)
-- non-triggered, one-shot 20s/25s, no re-arm.
    schedule(guid, "lightningShield", {20000, 25000}, function()
        lightningShieldTick(creature, guid)
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
        frenzyTick(creature, guid)
    end)
end

local sjonnirSummons = {}

local function despawnSjonnirSummons(guid)
    local list = sjonnirSummons[guid]
    if list then
        for _, summon in pairs(list) do
            summon:Despawn()
        end
        sjonnirSummons[guid] = nil
    end
end

local function sjonnirCombatEnd(event, creature)
    despawnSjonnirSummons(creature:GetGUIDLow())
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset (:92): abuseTheOoze = 0; EnterEvadeMode ends with
    -- Reset() (CreatureAI.cpp:237), and ev2/ev23 share this handler.
    creature:SetData(DATA_ABUSE_THE_OOZE, 0)
end

-- C++ Reset (boss_sjonnir.cpp:92-95): _Reset() —
-- SetBossState(DATA_SJONNIR, NOT_STARTED)
-- (ScriptedCreature.cpp:499-511; the bjarngrim precedent sets it
-- unconditionally on ev23). Split from sjonnirCombatEnd: ev2
-- leave-combat does not call Reset in C++, only ev23 does.
local function sjonnirReset(event, creature)
    despawnSjonnirSummons(creature:GetGUIDLow())
    cancelTimers(creature:GetGUIDLow())
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's counter before
    -- the achievement gate reads it; the wipe re-arm is preserved.
    if creature:IsAlive() then
        creature:SetData(DATA_ABUSE_THE_OOZE, 0)
    end
    creature:SetBossState(DATA_SJONNIR, 0)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function sjonnirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (boss_sjonnir.cpp:125-128): _JustDied() then
-- Talk(DEATH) — SetBossState(DATA_SJONNIR, DONE) first; the despawn
-- leg ports via the ev19 summons latch below.
local function sjonnirDied(event, creature, killer)
    despawnSjonnirSummons(creature:GetGUIDLow())
    creature:SetBossState(DATA_SJONNIR, 3)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_sjonnirAI::JustSummoned (boss_sjonnir.cpp:117-122):
-- summon MovePoint(0, CenterPoint) + summons latch. MovePoint
-- ported via the MoveTo twin (nesingwary precedent); the summons
-- latch rides the ev19 table below, despawned from died/reset/leave
-- (BossAI _JustDied/_Reset DespawnAll, keleseth precedent).
local function sjonnirJustSummoned(event, creature, summon)
    local guid = creature:GetGUIDLow()
    local list = sjonnirSummons[guid]
    if not list then
        list = {}
        sjonnirSummons[guid] = list
    end
    list[summon:GetGUIDLow()] = summon
    summon:MoveTo(0, 1295.21, 667.157, 189.691)
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the boss GUID goes in the instance store under DATA_SJONNIR so the
-- sludge relay can resolve it via GetInstanceCreature.
local function sjonnirSpawned(event, creature)
    creature:SetGuidData(DATA_SJONNIR)
end

-- C++ DoAction (:137-141): ACTION_OOZE_DEAD increments the counter.
local function sjonnirDoAction(event, creature, action)
    if action == ACTION_OOZE_DEAD then
        creature:SetData(DATA_ABUSE_THE_OOZE, creature:GetData(DATA_ABUSE_THE_OOZE) + 1)
    end
end

-- C++ npc_iron_sludge::JustDied (:288-292): instance GetGuidData
-- (DATA_SJONNIR) -> DoAction(ACTION_OOZE_DEAD); the GetInstanceCreature
-- twin resolves the same stored GUID.
local function ironSludgeDied(event, creature)
    local sjonnir = creature:GetInstanceCreature(DATA_SJONNIR)
    if sjonnir then
        sjonnir:DoAction(ACTION_OOZE_DEAD)
    end
end

RegisterCreatureEvent(ENTRY_SJONNIR, 1, sjonnirEnterCombat)
RegisterCreatureEvent(ENTRY_SJONNIR, 2, sjonnirCombatEnd)
RegisterCreatureEvent(ENTRY_SJONNIR, 3, sjonnirTargetDied)
RegisterCreatureEvent(ENTRY_SJONNIR, 4, sjonnirDied)
RegisterCreatureEvent(ENTRY_SJONNIR, 5, sjonnirSpawned)
RegisterCreatureEvent(ENTRY_SJONNIR, 23, sjonnirReset)
RegisterCreatureEvent(ENTRY_SJONNIR, 19, sjonnirJustSummoned)
RegisterCreatureEvent(ENTRY_SJONNIR, 38, sjonnirDoAction)
RegisterCreatureEvent(NPC_IRON_SLUDGE, 4, ironSludgeDied)
