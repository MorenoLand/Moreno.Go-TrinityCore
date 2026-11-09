-- Maiden of Grief (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_maiden_of_grief.cpp
-- (boss_maiden_of_grief (CreatureScript) via
-- GetHallsOfStoneAI<boss_maiden_of_griefAI> (BossAI), DATA_MAIDEN_OF_GRIEF
-- = 1 — registered from inside AddSC_boss_maiden_of_grief(); loader
-- decl 104 / call 299 per northrend_script_loader.cpp — the FIRST
-- group of the "// Halls of Stone" block in AddNorthrendScripts(),
-- under the "// Ulduar: Halls of Stone" decl marker at loader line
-- 103 and the "// Halls of Stone" call marker at loader line 298,
-- immediately after AddSC_instance_halls_of_lightning() (decl 102 /
-- call 297), which closed the Halls of Lightning block; the call
-- after it is AddSC_boss_krystallus() (decl 105 / call 300) —
-- loader order confirmed this run; the checkpoint sequence
-- (instance_halls_of_lightning -> boss_maiden_of_grief) is followed).
-- Entry: 27975 Maiden of Grief (halls_of_stone.h NPC_MAIDEN, line 49;
-- DATA_MAIDEN_OF_GRIEF = 1, line 32; GO_MAIDEN_DOOR = 191292, line
-- 63) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "boss_maiden_of_grief" hits boss_maiden_of_grief.cpp + the loader
-- decl/call lines for AddSC_boss_maiden_of_grief() only; zero sql/
-- hits. No maiden_of_grief lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer arms ride CreateLuaEvent
-- (the maiden CreateLuaEvent convention); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1) + the
-- BossAI::JustEngagedWith base arms (SetCombatPulseDelay(5) +
-- me-form DoZoneInCombat, ScriptedCreature.cpp:540/542) ported
-- into maidenEnterCombat; the
-- DoStartTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT 20383) leg
-- is PORTED (tick 515) via the instance-data bridge
-- (instance_hooks.go:75; the xt002/kologarn precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — nineteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 2) (event 4) + _JustDied() SetBossState
-- (DATA_MAIDEN_OF_GRIEF 1, DONE) PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; the
-- boss_moroes.lua / bjarngrim precedent) — the "no bridge" claim
-- was stale.
-- UpdateAI EVENT_STORM_OF_GRIEF — PORTED (retraction re-audit
-- 2026-10-14: the Oct-2 "no-timer-bridge" claim is stale):
-- DoCastVictim(SPELL_STORM_OF_GRIEF 50752, triggered), 10s init
-- (:67) / {15s,20s} re-arm (:113-115) — zero gating legs —
-- creature:CastSpell(nil, 50752, true).
-- UpdateAI EVENT_SHOCK_OF_SORROW — PORTED:
-- ResetThreatList (PORTED 2026-10-17: creature:ResetAllThreat,
-- bridged lua_objects.go:389) + Talk(SAY_STUN 3) +
-- DoCastAOE(SPELL_SHOCK_OF_SORROW 50760)
-- non-triggered, {20s,25s} init (:68) / {20s,30s} re-arm (:117-121)
-- — creature:Talk(3) + creature:CastSpell(creature, 50760) (the
-- DoCastAOE self-cast convention — selin_fireheart / mr_smite /
-- razuvious precedent).
-- UpdateAI EVENT_PARTING_SORROW — PORTED 2026-10-17 (partingTick —
-- the "no heroic bridge" claim was stale: creature:IsHeroic() is
-- bridged, blood-prince-council precedent): IsHeroic()-gated
-- random-target DoCast(SPELL_PARTING_SORROW 59723) non-triggered,
-- {25s,30s} init / unconditional {30s,40s} re-arm, no cast when no
-- target (C++ :108-111).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 27975):
-- Reset DoStopTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT) — PORTED
-- (tick 515) via the instance-data bridge (instance_hooks.go:110).
-- JustEngagedWith DoStartTimedAchievement — PORTED (tick 515), same bridge.
-- PORTED (retraction re-audit 2026-10-17: the "no heroic bridge" claim
-- was stale — creature:IsHeroic() is bridged on the event-handler
-- surface (engine/world/lua_creature_events.go:409-417, per-instance
-- difficulty read), used live by boss_blood_prince_council.lua):
-- UpdateAI EVENT_PARTING_SORROW (partingTick) — IsHeroic()-gated
-- random-target DoCast(SPELL_PARTING_SORROW 59723) non-triggered,
-- {25s,30s} init (:66) / unconditional {30s,40s} re-arm (:108-111),
-- SelectTarget(Random, 0, 0.0f, true) via the randomPlayer pool
-- (lavanthor precedent; offset-0 = tank stays in the pool).
-- EVENT_PILLAR_OF_WOE — PORTED (pillarTick — the "no random-target
-- bridge" claim was stale, C++ :123-128); the UNIT_STATE_CASTING gate
-- (UpdateAI :101 pre-loop + :134 in-loop) is PORTED 2026-11-04 as a
-- 500ms IsCasting defer in all 4 ticks (mal_ganis idiom; IsCasting
-- bridged lua_creature_events.go:1950); the DoMeleeAttackIfReady tail
-- has no bridge.

local ENTRY_MAIDEN_OF_GRIEF = 27975

-- DATA_MAIDEN_OF_GRIEF = 1 (halls_of_stone.h:32).
local DATA_MAIDEN_OF_GRIEF = 1

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_STUN = 3

local SPELL_STORM_OF_GRIEF = 50752
local SPELL_SHOCK_OF_SORROW = 50760
local SPELL_PILLAR_OF_WOE = 50761
local SPELL_PARTING_SORROW = 59723 -- boss_maiden_of_grief.cpp spells enum

-- ACHIEVEMENT_TIMED_TYPE_EVENT = 1 (xt002 precedent, tick 472);
-- ACHIEV_GOOD_GRIEF_START_EVENT = 20383 (boss_maiden_of_grief.cpp:49).
local ACHIEVEMENT_TIMED_TYPE_EVENT = 1
local ACHIEV_GOOD_GRIEF_START_EVENT = 20383

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

-- C++ UpdateAI EVENT_STORM_OF_GRIEF
-- (boss_maiden_of_grief.cpp:113-115): DoCastVictim(
-- SPELL_STORM_OF_GRIEF 50752, triggered), 10s init (:67) /
-- {15s,20s} re-arm — zero gating legs.
local function stormTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "storm", 500, function()
            stormTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_STORM_OF_GRIEF, true)
    schedule(guid, "storm", {15000, 20000}, function()
        stormTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_SHOCK_OF_SORROW (:117-121): ResetThreatList
-- (PORTED: creature:ResetAllThreat, bridged lua_objects.go) +
-- Talk(SAY_STUN 3) + DoCastAOE(SPELL_SHOCK_OF_SORROW 50760)
-- non-triggered, {20s,25s} init (:68) / {20s,30s} re-arm.
local function shockTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shock", 500, function()
            shockTick(creature, guid)
        end)
        return
    end
    creature:ResetAllThreat()
    creature:Talk(SAY_STUN)
    creature:CastSpell(creature, SPELL_SHOCK_OF_SORROW)
    schedule(guid, "shock", {20000, 30000}, function()
        shockTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI base arms /
-- achievement legs — Talk + base arms ported, achievement leg
-- unbridged. The Reset() ScheduleEvent legs
-- seed the timer machine here: the C++ EventMap only ticks once
-- UpdateAI runs with a victim, so engage-seeding is
-- behavior-identical (the krystallus precedent).
-- C++ SelectTarget draws the threat list (UnitAI.h), not the
-- instance-player pool; offset 1 pops the current victim BEFORE the
-- player filter (UnitAI.h builds the list victim-first, then the
-- sorted threat list; Go's list is threat-sorted, so the victim-GUID
-- exclusion models the pop — an index drop diverges whenever the
-- victim is not top threat).
-- skipFirst=true serves PILLAR (offset 1); false serves PARTING
-- (offset 0, tank eligible). The else-leg DoCastVictim (:127) is the
-- nil-target fallback.
local function randomPlayerTarget(creature, skipFirst)
    local targets = creature:GetAITargets()
    local victimGuid = nil
    if skipFirst then
        local victim = creature:GetVictim()
        if victim ~= nil then
            victimGuid = victim:GetGUID()
        end
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if skipFirst then
            if victimGuid ~= nil then
                skip = t:GetGUID() == victimGuid
            elseif not t:IsDead() then
                aliveSeen = aliveSeen + 1
                skip = aliveSeen == 1
            end
        end
        if not skip
                and t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_PARTING_SORROW (boss_maiden_of_grief.cpp:108-111):
-- SelectTarget(Random, 0, 0.0f, true) -> DoCast(SPELL_PARTING_SORROW
-- 59723) 2-arg = non-triggered; the arm is IsHeroic()-gated (:65-66,
-- creature:IsHeroic() bridged — the blood-prince-council precedent);
-- no cast when no target found, unconditional {30s,40s} re-arm (:111).
local function partingTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "parting", 500, function()
            partingTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_PARTING_SORROW)
    end
    schedule(guid, "parting", {30000, 40000}, function()
        partingTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_PILLAR_OF_WOE (boss_maiden_of_grief.cpp:123-128):
-- SelectTarget(Random, 1, 0.0f, true) -> DoCast(SPELL_PILLAR_OF_WOE
-- 50761), else DoCastVictim(SPELL_PILLAR_OF_WOE) — both 2-arg =
-- non-triggered; init {5s,15s} (:69), unconditional re-arm {5s,25s}
-- (:128).
local function pillarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pillar", 500, function()
            pillarTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerTarget(creature, true)
    if target then
        creature:CastSpell(target, SPELL_PILLAR_OF_WOE)
    else
        creature:CastSpell(nil, SPELL_PILLAR_OF_WOE)
    end
    schedule(guid, "pillar", math.random(5000, 25000), function()
        pillarTick(creature, guid)
    end)
end

local function maidenEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base-first (boss_maiden_of_grief.cpp:74-76):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:525-543) —
    -- SetBossState(DATA_MAIDEN_OF_GRIEF, IN_PROGRESS) first, then
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat, before Talk
    -- (C++ arm order); the timed-achievement leg is ported below.
    creature:SetBossState(DATA_MAIDEN_OF_GRIEF, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (:79): instance->DoStartTimedAchievement
    -- (ACHIEVEMENT_TIMED_TYPE_EVENT, ACHIEV_GOOD_GRIEF_START_EVENT) —
    -- instance-data bridge (instance_hooks.go:75; xt002 precedent).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data:StartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT,
                ACHIEV_GOOD_GRIEF_START_EVENT)
        end
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ Reset() :65-66: the EVENT_PARTING_SORROW seed is
    -- IsHeroic()-gated (creature:IsHeroic() bridged — the
    -- blood-prince-council precedent).
    if creature:IsHeroic() then
        schedule(guid, "parting", {25000, 30000}, function()
            partingTick(creature, guid)
        end)
    end
    schedule(guid, "storm", 10000, function()
        stormTick(creature, guid)
    end)
    schedule(guid, "shock", {20000, 25000}, function()
        shockTick(creature, guid)
    end)
    schedule(guid, "pillar", math.random(5000, 15000), function()
        pillarTick(creature, guid)
    end)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function maidenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function maidenLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_maiden_of_grief.cpp:61-63): _Reset() —
-- SetBossState(DATA_MAIDEN_OF_GRIEF, NOT_STARTED)
-- (ScriptedCreature.cpp:499-511; the bjarngrim precedent sets it
-- unconditionally on ev23).
local function maidenReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(DATA_MAIDEN_OF_GRIEF, 0)
    -- C++ Reset (:71): instance->DoStopTimedAchievement
    -- (ACHIEVEMENT_TIMED_TYPE_EVENT, ACHIEV_GOOD_GRIEF_START_EVENT).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data:StopTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT,
                ACHIEV_GOOD_GRIEF_START_EVENT)
        end
    end
end

-- C++ JustDied (boss_maiden_of_grief.cpp:88-91): _JustDied() then
-- Talk(DEATH) — SetBossState(DATA_MAIDEN_OF_GRIEF, DONE) first.
local function maidenDied(event, creature, killer)
    creature:SetBossState(DATA_MAIDEN_OF_GRIEF, 3)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 1, maidenEnterCombat)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 2, maidenLeaveCombat)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 3, maidenTargetDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 4, maidenDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_GRIEF, 23, maidenReset)
