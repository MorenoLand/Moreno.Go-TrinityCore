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
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): arm each timer at its C++ ScheduleEvent
--   cooldown; the _stepTargetIndex = 0 / _stepTargets.clear() legs
--   collapse into the cancel (nothing bridgeable populates the list).
-- - Wounding Strike 52771 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered, init {4s,6s} ->
--   repeat {12s,18s}.
-- - Curse of Exertion 52772 on SelectTarget(Random, 0, 100.0f, true),
--   non-triggered, init {10s,17s} -> repeat 9.3s (nil target -> no cast).
-- - Time Warp: Talk SAY_TIME_WARP (2) + DoCastAOE(52766) + DoCastAOE
--   (52736 dummy), both non-triggered (DoCastAOE resolves to self-cast —
--   kazrogal/illidan precedent), init 25s -> repeat 25s. The dummy's only
--   consumer is the unbridgeable SpellHitTarget time-step machine (below).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   victim->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (3)
--   (terestian_illhoof victim:GetObjectType() == "Player" convention).
-- - Death (C++ JustDied): _JustDied() only — instance bookkeeping,
--   blocked (standing); the Go leg is cancel only.
-- Unmodeled (documented-only, no bridges):
-- - InitializeAI: instance->GetBossState(DATA_EPOCH) == DONE ->
--   me->RemoveLootMode(LOOT_MODE_DEFAULT) — instance-data bridge blocked
--   (standing).
-- - BossAI::JustEngagedWith / BossAI::_Reset / BossAI::_JustDied
--   instance legs — blocked (standing).
-- - EVENT_TIME_STOP (SPELL_TIME_STOP 58848): DoCastAOE, non-triggered,
--   init 15s -> repeat 25s, scheduled only under IsHeroic() — no
--   heroic/difficulty bridge (standing heroic-unmodeled case).
-- - EVENT_TIME_STEP + SpellHitTarget: SpellHitTarget(SPELL_TIME_STEP_
--   DUMMY 52736, hostile) pushes the target's GUID into _stepTargets and
--   reschedules EVENT_TIME_STEP at 500ms; the event then charges (DoCast
--   triggered SPELL_TIME_STEP_CHARGE 52737) a random remaining target,
--   falling back to the victim when the list empties. No SpellHitTarget /
--   SpellInfo bridge and no ObjectAccessor GUID-lookup bridge exist
--   (SpellHit-15-never-fires standing queue), so the whole step machine
--   is documented, not wired.
-- - Yell enum 0/1 are undeclared in C++; SAY_TIME_WARP (2) and SAY_SLAY
--   (3) are the only Talked lines.

local ENTRY = 26532

local SAY_TIME_WARP = 2
local SAY_SLAY      = 3

local SPELL_CURSE_OF_EXERTION = 52772
local SPELL_TIME_WARP         = 52766
local SPELL_TIME_STOP         = 58848
local SPELL_WOUNDING_STRIKE   = 52771
local SPELL_TIME_STEP_DUMMY   = 52736
local SPELL_TIME_STEP_CHARGE  = 52737

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

-- nefarian convention: unbounded random alive player in the instance.
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

-- C++ EVENT_WOUNDING_STRIKE: DoCastVictim(52771), non-triggered, init
-- {4s,6s} -> repeat {12s,18s}; jeklik GetVictim + CastSpell convention.
local function onWoundingStrike(creature, guid)
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
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_EXERTION)
    end
    schedule(guid, "curseofexertion", 9300, function()
        onCurseOfExertion(creature, guid)
    end)
end

-- C++ EVENT_TIME_WARP: Talk SAY_TIME_WARP (2) + DoCastAOE(52766) +
-- DoCastAOE(52736 dummy), non-triggered, init 25s -> repeat 25s
-- (DoCastAOE resolves to self-cast — kazrogal/illidan precedent). The
-- dummy's only consumer is the unbridgeable SpellHitTarget step machine.
local function onTimeWarp(creature, guid)
    creature:Talk(SAY_TIME_WARP)
    creature:CastSpell(creature, SPELL_TIME_WARP)
    creature:CastSpell(creature, SPELL_TIME_STEP_DUMMY)
    schedule(guid, "timewarp", 25000, function()
        onTimeWarp(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
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
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
