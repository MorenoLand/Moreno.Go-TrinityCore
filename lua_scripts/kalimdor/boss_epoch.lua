-- Culling of Stratholme: Chrono-Lord Epoch — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- boss_chrono_lord_epoch.cpp (164 lines; boss_epochAI :
-- public BossAI(creature, DATA_EPOCH); GetAI via
-- GetCullingOfStratholmeAI<boss_epochAI> (CoSScriptName
-- "instance_culling_of_stratholme" gate); AddSC_boss_epoch at end
-- registers the one script; kalimdor loader decl 47 / call 160
-- per kalimdor_script_loader.cpp — the first "// CoT Culling Of
-- Stratholme" loader group, right after the Black Morass block).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_epoch" (loader decl/call lines only otherwise).
-- Entry: npc_arthas.cpp:59 names NPC_EPOCH = 26532 and :1329 has
-- instance->instance->SummonCreature(NPC_EPOCH, ...) in the
-- RP3_EVENT_EPOCH_SPAWN leg (the arthas RP3 chain spawns Epoch at
-- RP3_EPOCH_SPAWN, 2457.008, 1113.929, 150.0776) — the name-to-entry tie
-- is C++-verified at summon strength; BossAI's ctor leg DATA_EPOCH is
-- culling_of_stratholme.h:118. The creature_template ScriptName binding
-- stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 15 OnSpellHitTarget, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - BossAI::UpdateAI CASTING gates PORTED (ScriptedCreature.cpp:578/:584,
--   thalnos precedent): each timer arm defers 500ms while
--   creature:IsCasting(), no re-arm consumed; the Time Step charge arm
--   defers before its target pick, so no step target is consumed.
-- - Engage (C++ JustEngagedWith): arm each timer at its C++ ScheduleEvent
--   cooldown; _stepTargetIndex = 0 / _stepTargets.clear() re-init the
--   per-GUID step state (the ev15 leg below populates it).
-- - Wounding Strike 52771 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init {4s,6s} ->
--   repeat {12s,18s}.
-- - Curse of Exertion 52772 on SelectTarget(Random, 0, 100.0f, true),
--   non-triggered, init {10s,17s} -> repeat 9.3s (nil target -> no cast).
-- - Time Warp: Talk SAY_TIME_WARP (2) + DoCastAOE(52766) non-triggered
--   (DoCastAOE resolves to self-cast — kazrogal/illidan precedent) +
--   the 52736 dummy fanned out over hostile threat targets (the C++
--   DoCastAOE radius fan-out lowered to the threat list — no AoE-cast
--   bridge exists; see the step machine below), init 25s -> repeat 25s.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   victim->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (3)
--   (terestian_illhoof victim:GetObjectType() == "Player" convention).
-- - Time Stop (EVENT_TIME_STOP, heroic-only): scheduled on engage
--   under creature:IsHeroic(), DoCastAOE(58848) non-triggered
--   self-cast, init 15s -> repeat 25s.
-- - Death (C++ JustDied): _JustDied() only — SetBossState(DATA_EPOCH,
--   DONE) PORTED 2026-11-06 (ScriptedCreature.cpp:513-519; live bridge
--   lua_creature_events.go:5917); the Go leg is state write + cancel.
-- - Reset (BossAI::Reset -> _Reset, ScriptedCreature.h:328 /
--   ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0) +
--   SetBossState(DATA_EPOCH, NOT_STARTED) unless already DONE, PORTED
--   2026-11-06 on ev23 (zarithrian idiom, incl. the !IsAlive early
--   return — Go fires ev23 on the death path before ev4); the
--   summons/scheduler legs are the cancelTimers above.
-- - Engage (C++ JustEngagedWith): BossAI::JustEngagedWith FIRST ->
--   SetBossState(DATA_EPOCH, IN_PROGRESS) PORTED 2026-11-06 on ev1
--   (base-first, ScriptedCreature.cpp:527-539); CheckRequiredBosses
--   has no CoS instance override, so the write is unconditional.
-- - InitializeAI (C++ :57-61): instance->GetBossState(DATA_EPOCH) ==
--   DONE -> RemoveLootMode(LOOT_MODE_DEFAULT 1, SharedDefines.h:42)
--   PORTED 2026-11-06 on ev5 (GetBossState live, :5936; RemoveLootMode
--   live, lua_creature_events.go:2246-2250 — the old "instance-data
--   bridge blocked" claim is retracted for the read + loot legs).
-- Unmodeled (documented-only, no bridges):
-- - EVENT_TIME_STEP + SpellHitTarget (ported): ev15 (svala precedent;
--   args (event, creature, target, spellId)) gated on SPELL_TIME_STEP_
--   DUMMY 52736 + creature:IsHostileTo(target) (lua_creature_events.go
--   IsHostileTo — the old "no bridge" claim is stale) pushes the target
--   object into the per-GUID step list (duplicates kept, C++-exact)
--   and reschedules the step event at 500ms (RescheduleEvent = cancel +
--   re-arm, matching the schedule helper). The step event charges
--   (DoCast triggered SPELL_TIME_STEP_CHARGE 52737) a random remaining
--   target (swap-to-front + index advance, C++ order), re-arming 500ms
--   before the cast when a list target was found; when the list
--   empties it charges the victim once with no re-arm. Deviations:
--   the C++ DoCastAOE radius fan-out is the threat list (non-threat
--   hostiles in radius never become step targets); C++ GetUnit(nil on
--   despawn) skips are unmodeled — stored target objects are used
--   as-is (party members do not despawn mid-fight in practice); the
--   fallback victim cast carries no dead-guard (C++ has none).
-- - Yell enum 0/1 are undeclared in C++; SAY_TIME_WARP (2) and SAY_SLAY
--   (3) are the only Talked lines.

local ENTRY = 26532

-- culling_of_stratholme.h:116-120 (EncounterState, InstanceScript.h:68-76:
-- NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3).
local DATA_EPOCH = 2
local LOOT_MODE_DEFAULT = 1 -- SharedDefines.h:42

local SAY_TIME_WARP = 2
local SAY_SLAY      = 3

local SPELL_CURSE_OF_EXERTION = 52772
local SPELL_TIME_WARP         = 52766
local SPELL_TIME_STOP         = 58848
local SPELL_WOUNDING_STRIKE   = 52771
local SPELL_TIME_STEP_DUMMY   = 52736
local SPELL_TIME_STEP_CHARGE  = 52737

local timers = {}

-- Per-GUID EVENT_TIME_STEP state: C++ _stepTargets (GUID vector, order
-- = push order, duplicates kept) + _stepTargetIndex (next unvisited
-- slot, 1-based here). Target objects ride the ev15 args directly, so
-- no ObjectAccessor GUID-lookup bridge is needed.
local stepState = {}

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ SelectTarget draws the threat list (UnitAI.h:217-218): position 0
-- takes any entry; dead members are offline refs in C++ (skipped at
-- list build) so they are filtered here. The old instance-player pool
-- is retired (threat-list precedent, Oct-25 strips).
local function randomThreatPlayerInRange(creature, maxDist)
    local pool = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ EVENT_WOUNDING_STRIKE: DoCastVictim(52771), non-triggered, init
-- {4s,6s} -> repeat {12s,18s}; jeklik GetVictim + CastSpell convention.
local function onWoundingStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "woundingstrike", 500, function()
            onWoundingStrike(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_WOUNDING_STRIKE)
    end
    schedule(guid, "woundingstrike", math.random(12000, 18000), function()
        onWoundingStrike(creature, guid)
    end)
end

-- C++ EVENT_CURSE_OF_EXERTION: DoCast(SelectTarget(Random, 0, 100.0f,
-- true), 52772), non-triggered, init {10s,17s} -> repeat 9.3s.
local function onCurseOfExertion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "curseofexertion", 500, function()
            onCurseOfExertion(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_EXERTION)
    end
    schedule(guid, "curseofexertion", 9300, function()
        onCurseOfExertion(creature, guid)
    end)
end

-- C++ EVENT_TIME_WARP: Talk SAY_TIME_WARP (2) + DoCastAOE(52766) +
-- DoCastAOE(52736 dummy), non-triggered, init 25s -> repeat 25s
-- (DoCastAOE resolves to self-cast for 52766 — kazrogal/illidan
-- precedent). The dummy is fanned out over hostile threat targets so
-- the ev15 step leg fires once per hit, as the C++ AoE fan-out does;
-- the caster is never its own hostile target, so no self-cast dummy.
local function onTimeWarp(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "timewarp", 500, function()
            onTimeWarp(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_TIME_WARP)
    creature:CastSpell(creature, SPELL_TIME_WARP)
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() and creature:IsHostileTo(t) then
            creature:CastSpell(t, SPELL_TIME_STEP_DUMMY)
        end
    end
    schedule(guid, "timewarp", 25000, function()
        onTimeWarp(creature, guid)
    end)
end

-- Forward declaration (tharon_ja / gal_darah idiom): the ev15 leg
-- schedules onTimeStep, assigned below with a plain assignment.
local onTimeStep

-- C++ SpellHitTarget (:131-137): dummy hit + hostile -> push + reschedule
-- EVENT_TIME_STEP at 500ms. The ev15 target object is stored, not its
-- GUID (no lookup bridge needed); duplicates are kept, C++-exact.
local function onSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_TIME_STEP_DUMMY or not target then
        return
    end
    if not creature:IsHostileTo(target) then
        return
    end
    local guid = creature:GetGUID()
    local st = stepState[guid]
    if not st then
        st = { targets = {}, index = 1 }
        stepState[guid] = st
    end
    st.targets[#st.targets + 1] = target
    schedule(guid, "timestep", 500, function()
        onTimeStep(creature, guid)
    end)
end

-- C++ EVENT_TIME_STEP (:104-128): charge a random remaining target
-- (swap-to-front + index advance), re-arming 500ms BEFORE the cast
-- when a list target was found; on exhaustion charge the victim once
-- with no re-arm.
onTimeStep = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "timestep", 500, function()
            onTimeStep(creature, guid)
        end)
        return
    end
    local st = stepState[guid]
    if not st then
        return
    end
    local target = nil
    if st.index <= #st.targets then
        local pick = math.random(st.index, #st.targets)
        st.targets[st.index], st.targets[pick] = st.targets[pick], st.targets[st.index]
        target = st.targets[st.index]
        st.index = st.index + 1
        schedule(guid, "timestep", 500, function()
            onTimeStep(creature, guid)
        end)
    else
        target = creature:GetVictim()
    end
    if target then
        creature:CastSpell(target, SPELL_TIME_STEP_CHARGE, true)
    end
end

-- C++ EVENT_TIME_STOP: DoCastAOE(58848) non-triggered, init 15s ->
-- repeat 25s, scheduled only under IsHeroic() (creature:IsHeroic()
-- bridges ScriptedAI::_isHeroic — blood_prince_council precedent).
local function onTimeStop(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "timestop", 500, function()
            onTimeStop(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TIME_STOP)
    schedule(guid, "timestop", 25000, function()
        onTimeStop(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    -- C++ JustEngagedWith (boss_chrono_lord_epoch.cpp:66-68) calls
    -- BossAI::JustEngagedWith FIRST: SetBossState(IN_PROGRESS) +
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat
    -- (ScriptedCreature.cpp:527-539); CheckRequiredBosses has no CoS
    -- instance override, so the state write is unconditional.
    creature:SetBossState(DATA_EPOCH, 1) -- IN_PROGRESS
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; base-first in boss_chrono_lord_epoch.cpp:66-68)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    stepState[guid] = { targets = {}, index = 1 }
    if creature:IsHeroic() then
        schedule(guid, "timestop", 15000, function()
            onTimeStop(creature, guid)
        end)
    end
    schedule(guid, "woundingstrike", math.random(4000, 6000), function()
        onWoundingStrike(creature, guid)
    end)
    schedule(guid, "curseofexertion", math.random(10000, 17000), function()
        onCurseOfExertion(creature, guid)
    end)
    schedule(guid, "timewarp", 25000, function()
        onTimeWarp(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    stepState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    stepState[guid] = nil
    if creature:IsDead() then
        return
    end
    -- C++ BossAI::Reset -> _Reset (ScriptedCreature.h:328 /
    -- ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0), then
    -- SetBossState NOT_STARTED unless already DONE.
    creature:SetCombatPulseDelay(0)
    if creature:GetBossState(DATA_EPOCH) ~= 3 then
        creature:SetBossState(DATA_EPOCH, 0) -- NOT_STARTED
    end
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    -- C++ JustDied (boss_chrono_lord_epoch.cpp:140-143): _JustDied()
    -- only -> SetBossState(DONE).
    creature:SetBossState(DATA_EPOCH, 3) -- DONE
    cancelTimers(guid)
    stepState[guid] = nil
end

-- C++ InitializeAI (boss_chrono_lord_epoch.cpp:57-61): when the boss
-- is already DONE, the default loot mode is removed.
local function onSpawn(event, creature)
    if creature:GetBossState(DATA_EPOCH) == 3 then
        creature:RemoveLootMode(LOOT_MODE_DEFAULT)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 15, onSpellHitTarget)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 23, onReset)
