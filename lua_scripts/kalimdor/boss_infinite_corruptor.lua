-- Culling of Stratholme: Infinite Corruptor — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- boss_infinite_corruptor.cpp (163 lines; class boss_infinite_corruptor :
-- public CreatureScript { boss_infinite_corruptorAI : public
-- BossAI(creature, DATA_INFINITE_CORRUPTOR) }; GetAI via
-- GetCullingOfStratholmeAI<boss_infinite_corruptorAI> (CoSScriptName
-- "instance_culling_of_stratholme" gate); AddSC_boss_infinite_corruptor
-- at end registers the one script; kalimdor loader decl 49 / call 162
-- per kalimdor_script_loader.cpp — the third "// CoT Culling Of
-- Stratholme" loader group, right after AddSC_npc_arthas_stratholme()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_infinite_corruptor" (loader decl/call lines only otherwise).
-- Entry: instance_culling_of_stratholme.cpp:62 names
-- NPC_INFINITE_CORRUPTOR = 32273 and :791 has
-- instance->SummonCreature(NPC_INFINITE_CORRUPTOR, CorruptorPos) in
-- SpawnInfiniteCorruptor — the name-to-entry tie is C++-verified at
-- summon strength (CorruptorPos 2331.642, 1273.273, 132.9524, 3.717551f
-- per :167); SpawnInfiniteCorruptor's spawn gate (:787 GetSpawnMode() ==
-- DUNGEON_DIFFICULTY_HEROIC, boss not DONE/FAIL) is a spawn-only leg —
-- the creature entry itself binds DB-side. BossAI's ctor leg
-- DATA_INFINITE_CORRUPTOR is culling_of_stratholme.h:120.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset, 28 OnMovementInform, 38 OnDoAction. Timers via
-- CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- BossAI::UpdateAI CASTING gates PORTED (ScriptedCreature.cpp:578/:584, thalnos precedent): each timer arm defers 500ms while creature:IsCasting(), no re-arm consumed.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer at
--   its C++ ScheduleEvent cooldown (7s / 5s); BossAI::JustEngagedWith
--   pulse + me-DoZone PORTED, and SetBossState(DATA_INFINITE_CORRUPTOR,
--   IN_PROGRESS) PORTED 2026-11-06 on ev1 (Talk-then-base in
--   boss_infinite_corruptor.cpp:78-81, ScriptedCreature.cpp:527-539;
--   CheckRequiredBosses has no CoS instance override, so the write is
--   unconditional).
-- - Reset (C++ :66-70): _Reset() -> SetCombatPulseDelay(0) +
--   SetBossState(NOT_STARTED) unless already DONE, PORTED 2026-11-06 on
--   ev23 (zarithrian idiom, incl. the !IsAlive early return); the
--   DoCastAOE channel leg stays out below.
-- - Corrupting Blight 60588 on SelectTarget(Random, 0, 60.0f, true) ->
--   random alive threat-list player within 60m, offset 0 (no skip;
--   UnitAI.h:172-174, Oct-25 CoS strip idiom), non-triggered
--   (C++ DoCast), init 7s -> repeat 15s; C++ reschedules the event even
--   when SelectTarget finds nothing, so the Lua re-arm is unconditional
--   too (nil target -> no cast).
-- - Void Strike 60590 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init 5s -> repeat 5s.
-- - Death (C++ JustDied): Talk SAY_DEATH (1) + _JustDied() ->
--   SetBossState(DONE) PORTED 2026-11-06 on ev4 (C++ order: Talk,
--   _JustDied, cleanup) + the instance SetBossState-override legs
--   (instance_culling_of_stratholme.cpp:459-467: cancel guardian tick —
--   no tick exists in the ported model — + UpdateWorldState
--   TIME_GUARDIAN_SHOW 3932 -> 0 and TIME_GUARDIAN 3931 -> 0, marwyn
--   precedent, creature:UpdateWorldState live
--   lua_creature_events.go:5888) + guardian/rift cleanup (below).
-- Unmodeled (documented-only, no bridges):
-- - Reset's DoCastAOE(SPELL_CORRUPTION_OF_TIME_CHANNEL 60422), the
--   "implicitly targets the Guardian" channel leg — stays out on
--   implicit-target grounds: C++ DoCast(nullptr) lets spell data pick
--   the guardian (UnitAI.h:322), while Go CastSpell needs an explicit
--   target; self-casting 60422 would put the channel on the corruptor,
--   and a FindNearestCreature guardian cast would invent a range C++
--   never states. _Reset() is instance bookkeeping (blocked).
-- - SpellHitTarget (event 15, PORTED): a SPELL_CORRUPTION_OF_TIME_
--   CHANNEL 60422 hit makes the target self-cast SPELL_CORRUPTION_OF_
--   TIME_TARGET 60451, triggered (C++-exact; epoch ev15 precedent, args
--   (event, creature, target, spellId)). Dormant until a 60422 cast
--   lands — Reset's channel cast itself stays out below.
-- - JustDied's FindNearestCreature legs, PORTED in onDied (the
--   header's "no nearby-creature enumeration bridge" claim was stale:
--   FindNearestCreature is live, lua_creature_events.go:2332, nearest
--   live only): NPC_GUARDIAN_OF_TIME 32281 within 100.0f ->
--   RemoveAura(60451), the single-spell twin of RemoveAurasDueToSpell
--   (netherspite precedent), + 5s one-shot Despawn (valithria
--   precedent; C++ DespawnOrUnsummon(5s)); NPC_TIME_RIFT 28409
--   within 100.0f -> immediate Despawn (C++ DespawnOrUnsummon()).
--   The DoAction LEAVE leg is PORTED tick 555 below (ev38 + ev28).
-- - EnterEvadeMode's HasReactState(REACT_PASSIVE) early-return — no
--   react-state bridge (standing).
-- - MovementInform(POINT_MOTION_TYPE, MOVEMENT_TIME_RIFT 1) ->
--   DespawnOrUnsummon(2s) + SetBossState(DATA_INFINITE_CORRUPTOR, FAIL)
--   — PORTED tick 555 on ev28 (MoveTo bridge fires the arrival,
--   lua_creature_events.go:4876; POINT_MOTION_TYPE 8, runtime.go:397).
-- - DoAction(-ACTION_CORRUPTOR_LEAVE) (culling_of_stratholme.h:150,
--   ACTION_CORRUPTOR_LEAVE = 2, so the action id is -2) — PORTED tick
--   555 on ev38 + ev28: SetReactState(REACT_PASSIVE) + Talk SAY_FAIL
--   (2) + FindNearestCreature(NPC_TIME_RIFT 28409, 300.0f), then
--   MoveTo (the MovePoint bridge, lua_creature_events.go:4876) to the
--   rift, with the within-5.0f / no-rift branches running the
--   MovementInform body inline (boss_midnight GetDistance2d-reach
--   idiom for IsWithinDist2d). Sender is live:
--   instance_culling_of_stratholme.lua:562.
-- - SAY_AGGRO (0) / SAY_DEATH (1) / SAY_FAIL (2) are the only Talked
--   lines.

local ENTRY = 32273

-- culling_of_stratholme.h:116-120 (EncounterState, InstanceScript.h:68-76:
-- NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3); worldstates
-- instance_culling_of_stratholme.cpp:100-101.
local DATA_INFINITE_CORRUPTOR = 4
local WORLDSTATE_TIME_GUARDIAN = 3931
local WORLDSTATE_TIME_GUARDIAN_SHOW = 3932

local SAY_AGGRO = 0
local SAY_DEATH = 1
local SAY_FAIL = 2

local ACTION_CORRUPTOR_LEAVE = 2
local MOVEMENT_TIME_RIFT = 1
local POINT_MOTION_TYPE = 8

local SPELL_CORRUPTING_BLIGHT = 60588
local SPELL_VOID_STRIKE       = 60590

local SPELL_CORRUPTION_OF_TIME_CHANNEL = 60422
local SPELL_CORRUPTION_OF_TIME_TARGET = 60451

local NPC_GUARDIAN_OF_TIME = 32281
local NPC_TIME_RIFT        = 28409

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- Threat-list pool (Oct-25 CoS strip idiom, meathook precedent):
-- C++ SelectTarget draws the threat list (UnitAI.h), offset 0 here.
local function randomThreatPlayer(creature, maxDist)
    local targets = creature:GetAITargets()
    local found = {}
    for _, t in ipairs(targets) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_CORRUPTING_BLIGHT: DoCast(SelectTarget(Random, 0, 60.0f,
-- true), 60588), non-triggered, init 7s -> repeat 15s (re-arm
-- unconditional in C++, even on empty target).
local function onCorruptingBlight(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "corruptingblight", 500, function()
            onCorruptingBlight(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayer(creature, 60)
    if target then
        creature:CastSpell(target, SPELL_CORRUPTING_BLIGHT)
    end
    schedule(guid, "corruptingblight", 15000, function()
        onCorruptingBlight(creature, guid)
    end)
end

-- C++ EVENT_VOID_STRIKE: DoCastVictim(60590), non-triggered, init 5s ->
-- repeat 5s; jeklik GetVictim + CastSpell convention.
local function onVoidStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "voidstrike", 500, function()
            onVoidStrike(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_VOID_STRIKE)
    end
    schedule(guid, "voidstrike", 5000, function()
        onVoidStrike(creature, guid)
    end)
end

-- C++ SpellHitTarget (:72-76): a SPELL_CORRUPTION_OF_TIME_CHANNEL
-- 60422 hit makes the target self-cast SPELL_CORRUPTION_OF_TIME_TARGET
-- 60451, triggered. Event 15 args (event, creature, target, spellId)
-- per the epoch precedent; C++ has no hostility gate here (unlike
-- epoch's dummy leg), so none is applied.
local function onSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_CORRUPTION_OF_TIME_CHANNEL or not target then
        return
    end
    target:CastSpell(target, SPELL_CORRUPTION_OF_TIME_TARGET, true)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (boss_infinite_corruptor.cpp:78-81): Talk,
    -- then BossAI::JustEngagedWith -> SetBossState(IN_PROGRESS) +
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat
    -- (ScriptedCreature.cpp:527-539).
    creature:SetBossState(DATA_INFINITE_CORRUPTOR, 1) -- IN_PROGRESS
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; Talk-then-base in boss_infinite_corruptor.cpp:78-81)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    schedule(guid, "corruptingblight", 7000, function()
        onCorruptingBlight(creature, guid)
    end)
    schedule(guid, "voidstrike", 5000, function()
        onVoidStrike(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    -- C++ Reset (boss_infinite_corruptor.cpp:66-70) calls _Reset()
    -- (ScriptedCreature.cpp:499-510): SetCombatPulseDelay(0), then
    -- SetBossState NOT_STARTED unless already DONE. The DoCastAOE
    -- channel leg in the same override stays out (implicit target).
    creature:SetCombatPulseDelay(0)
    if creature:GetBossState(DATA_INFINITE_CORRUPTOR) ~= 3 then
        creature:SetBossState(DATA_INFINITE_CORRUPTOR, 0) -- NOT_STARTED
    end
end

-- C++ JustDied (:88-99): Talk SAY_DEATH + _JustDied() (SetBossState
-- DONE) + the instance SetBossState-override worldstate clears
-- (instance_culling_of_stratholme.cpp:459-467) + guardian/rift cleanup
-- in C++ order: guardian within 100.0f loses the corruption aura and
-- despawns after 5s; rift within 100.0f despawns at once.
-- FindNearestCreature skips dead creatures, so a dead guardian/rift
-- reads as absent, like C++'s live-pointer checks.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_INFINITE_CORRUPTOR, 3) -- DONE
    creature:UpdateWorldState(WORLDSTATE_TIME_GUARDIAN_SHOW, 0)
    creature:UpdateWorldState(WORLDSTATE_TIME_GUARDIAN, 0)
    local guardian = creature:FindNearestCreature(NPC_GUARDIAN_OF_TIME, 100)
    if guardian then
        guardian:RemoveAura(SPELL_CORRUPTION_OF_TIME_TARGET)
        CreateLuaEvent(function()
            guardian:Despawn()
        end, 5000)
    end
    local rift = creature:FindNearestCreature(NPC_TIME_RIFT, 100)
    if rift then
        rift:Despawn()
    end
end

-- C++ MovementInform (boss_infinite_corruptor.cpp:126-133): arrival at
-- the rift despawns the corruptor after 2s and FAILs the encounter
-- (C++ order: DespawnOrUnsummon(2s) scheduled, then SetBossState).
local function leaveFailed(creature)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 2000)
    creature:SetBossState(DATA_INFINITE_CORRUPTOR, 2) -- FAIL
end

local function onMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE or id ~= MOVEMENT_TIME_RIFT then
        return
    end
    leaveFailed(creature)
end

-- C++ DoAction (boss_infinite_corruptor.cpp:135-151): action
-- -ACTION_CORRUPTOR_LEAVE (-2) — SetReactState PASSIVE + Talk SAY_FAIL,
-- then walk to the nearest time rift; already at the rift (within
-- 5.0f, boss_midnight GetDistance2d-reach idiom) or no rift at all
-- runs the MovementInform body inline.
local function onDoAction(event, creature, action)
    if action ~= -ACTION_CORRUPTOR_LEAVE then
        return
    end
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:Talk(SAY_FAIL)
    local rift = creature:FindNearestCreature(NPC_TIME_RIFT, 300)
    if not rift then
        leaveFailed(creature)
        return
    end
    if creature:GetDistance2d(rift) - rift:GetCombatReach() < 5.0 then
        leaveFailed(creature)
        return
    end
    creature:MoveTo(MOVEMENT_TIME_RIFT, rift:GetX(), rift:GetY(), rift:GetZ())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 15, onSpellHitTarget)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 28, onMovementInform)
RegisterCreatureEvent(ENTRY, 38, onDoAction)
