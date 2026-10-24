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
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer at
--   its C++ ScheduleEvent cooldown (7s / 5s); the BossAI::JustEngagedWith
--   instance leg is blocked (standing).
-- - Corrupting Blight 60588 on SelectTarget(Random, 0, 60.0f, true) ->
--   random alive player within 60m (nefarian convention), non-triggered
--   (C++ DoCast), init 7s -> repeat 15s; C++ reschedules the event even
--   when SelectTarget finds nothing, so the Lua re-arm is unconditional
--   too (nil target -> no cast).
-- - Void Strike 60590 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init 5s -> repeat 5s.
-- - Death (C++ JustDied): Talk SAY_DEATH (1) + cancel + guardian/rift
--   cleanup (below); the _JustDied() instance leg stays blocked.
-- Unmodeled (documented-only, no bridges):
-- - Reset's DoCastAOE(SPELL_CORRUPTION_OF_TIME_CHANNEL 60422), the
--   "implicitly targets the Guardian" channel leg — no cast-target /
--   AoE-to-guardian bridge; _Reset() is instance bookkeeping (blocked).
-- - SpellHitTarget(SPELL_CORRUPTION_OF_TIME_CHANNEL): the guardian
--   self-casts SPELL_CORRUPTION_OF_TIME_TARGET 60451, triggered. Stays
--   out on trigger-path grounds: Reset's channel cast (DoCastAOE 60422,
--   "implicitly targets the Guardian") is unported — no cast-target /
--   AoE-to-guardian bridge — so event 15 never fires for it in Go
--   (event 15 itself is live — boss_ai.go:1904).
-- - JustDied's FindNearestCreature legs, PORTED in onDied (the
--   header's "no nearby-creature enumeration bridge" claim was stale:
--   FindNearestCreature is live, lua_creature_events.go:2332, nearest
--   live only): NPC_GUARDIAN_OF_TIME 32281 within 100.0f ->
--   RemoveAura(60451), the single-spell twin of RemoveAurasDueToSpell
--   (netherspite precedent), + 5s one-shot Despawn (valithria
--   precedent; C++ DespawnOrUnsummon(5s)); NPC_TIME_RIFT 28409
--   within 100.0f -> immediate Despawn (C++ DespawnOrUnsummon()).
--   The DoAction LEAVE leg below stays out (MovePoint unbridged).
-- - EnterEvadeMode's HasReactState(REACT_PASSIVE) early-return — no
--   react-state bridge (standing).
-- - MovementInform(POINT_MOTION_TYPE, MOVEMENT_TIME_RIFT 1) ->
--   DespawnOrUnsummon(2s) + SetBossState(DATA_INFINITE_CORRUPTOR, FAIL)
--   — no movement / instance-data bridges.
-- - DoAction(-ACTION_CORRUPTOR_LEAVE) (culling_of_stratholme.h:150):
--   SetReactState(REACT_PASSIVE) + Talk SAY_FAIL (2) +
--   FindNearestCreature(NPC_TIME_RIFT 28409, 300.0f) -> MovePoint to the
--   rift (or straight to MovementInform when already within 5.0f 2d,
--   or when no rift exists) — no DoAction / movement / react-state /
--   nearby-creature bridges; instance-side caller is
--   instance_culling_of_stratholme.cpp:495.
-- - SAY_AGGRO (0) / SAY_DEATH (1) / SAY_FAIL (2) are the only Talked
--   lines.

local ENTRY = 32273

local SAY_AGGRO = 0
local SAY_DEATH = 1

local SPELL_CORRUPTING_BLIGHT = 60588
local SPELL_VOID_STRIKE       = 60590

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

-- nefarian convention: random alive player in the instance within maxDist.
local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
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
    local target = randomPlayerInRange(creature, 60)
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
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_VOID_STRIKE)
    end
    schedule(guid, "voidstrike", 5000, function()
        onVoidStrike(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "corruptingblight", 7000, function()
        onCorruptingBlight(creature, guid)
    end)
    schedule(guid, "voidstrike", 5000, function()
        onVoidStrike(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (:88-99): Talk SAY_DEATH + _JustDied() (instance,
-- blocked) + guardian/rift cleanup in C++ order: guardian within
-- 100.0f loses the corruption aura and despawns after 5s; rift
-- within 100.0f despawns at once. FindNearestCreature skips dead
-- creatures, so a dead guardian/rift reads as absent, like C++'s
-- live-pointer checks.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
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

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
