-- Zuramat the Obliterator (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_zuramat.cpp
-- (232 lines incl. license; 3 scripts — boss_zuramat (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_ZURAMAT = 8); npc_void_sentry
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), zero Talk arms,
-- unbridged); achievement_void_dance (AchievementCriteriaScript OnCheck
-- AI()->GetData(DATA_VOID_DANCE)); all registered from inside
-- AddSC_boss_zuramat(); loader decl 151 / call 346 per
-- northrend_script_loader.cpp — the SEVENTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_xevozz() (call 345); the call after it is
-- AddSC_instance_violet_hold() (call 347) — the checkpoint sequence
-- (boss_xevozz -> boss_zuramat) is followed).
-- Entries: 29314 Zuramat the Obliterator (violet_hold.h NPC_ZURAMAT,
-- line 101) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_ZURAMAT
-- = 8 (violet_hold.h line 58). NPC_VOID_SENTRY = 29364 (line 102,
-- zero Talk — not registered); NPC_VOID_SENTRY_BALL = 29365 (line
-- 103).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_zuramat"
-- hits boss_zuramat.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (VOID_SHIFT uses fixed 9s/15s; SHROUD uses
-- {18s,20s} ranges).
-- creature:CastSpell(creature, spell) = DoCast(me) (2-arg =
-- non-triggered); creature:CastSpell(target, spell) = DoCast(target,
-- spell) (2-arg = non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDeath, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent); seeds the two combat timers below; the
-- BossAI::JustEngagedWith passthrough leg has no bridge.
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent); the _JustDied() passthrough leg has no
-- bridge.
-- ScheduleTasks VOID_SHIFT — SelectTarget(Random, 0, 60.0f, true) ->
-- DoCast(target, SPELL_VOID_SHIFT 54361), 2-arg = non-triggered;
-- init 9s / Repeat 15s — unconditional re-arm. Random-player pool:
-- GetPlayersInWorld, same map/instance, alive, within 60 yd
-- (creature:GetDistance — the zuljin maxDist precedent).
-- ScheduleTasks SHROUD_OF_DARKNESS — DoCast(me, SPELL_SHROUD_OF_
-- DARKNESS 54524), 2-arg = non-triggered; init {18s,20s} / Repeat
-- {18s,20s} — unconditional re-arm.
-- DOCUMENTED DEVIATIONS: none on the ported arms — both timers carry
-- C++-exact timing and targeting. The UpdateAI
-- DoMeleeAttackIfReady() leg has no bridge — melee is unmodeled.
-- DOCUMENTED-ONLY (in this header; entry 29314 is the sole
-- registration):
-- SAY_SPAWN (3), SAY_SHIELD (4) and SAY_WHISPER (5) are never Talk()ed
-- anywhere in the file (dead enum text, the cyanigosa precedent).
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_ZURAMAT) leg joins the no-instance-SetData bridge queue (the
-- erekem/ichoron/lavanthor precedent).
-- The ScheduleTasks SUMMON_VOID_SENTRY leg (DoCast(me, SPELL_SUMMON_
-- VOID_SENTRY 54369), 4s init / 7-10s Repeat) stays out: it is a
-- summon leg, and the summoned npc_void_sentry AI is unbridged — its
-- IsSummonedBy triggered self-cast SPELL_SUMMON_VOID_SENTRY_BALL
-- (58650), REACT_PASSIVE stance, SummonedCreatureDies _voidDance =
-- false flip, and SummonedCreatureDespawn DoAction(ACTION_DESPAWN_
-- VOID_SENTRY_BALL) leg have no bridges; casting it would spawn inert
-- sentries. Joins the no-DoAction / no-SummonList bridge queues.
-- achievement_void_dance (OnCheck AI()->GetData(DATA_VOID_DANCE))
-- joins the unmodeled-achievement / no-GetData-bridge queue (the
-- cyanigosa / ichoron precedent).

local ENTRY_ZURAMAT = 29314

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_VOID_SHIFT = 54361
local SPELL_SHROUD_OF_DARKNESS = 54524

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

-- C++ SelectTarget(Random, 0, 60.0f, true) — alive players on the same
-- map/instance within 60 yd (the zuljin maxDist precedent).
local function randomPlayer(creature, maxDist)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and (maxDist == nil or creature:GetDistance(plr) <= maxDist) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks VOID_SHIFT: SelectTarget(Random, 0, 60.0f, true)
-- -> DoCast(target, 54361), 2-arg = non-triggered; init 9s / Repeat
-- 15s — re-armed unconditionally; threat-list-vs-player-pool caveat
-- (the_beast/opera-Julianne precedent): C++ picks from the threat
-- list, the Lua pool is map-wide players.
local function voidShiftTick(creature, guid)
    local target = randomPlayer(creature, 60)
    if target then
        creature:CastSpell(target, SPELL_VOID_SHIFT)
    end
    schedule(guid, "voidshift", 15000, function()
        voidShiftTick(creature, guid)
    end)
end

-- C++ ScheduleTasks SHROUD_OF_DARKNESS: DoCast(me, 54524), 2-arg =
-- non-triggered; init {18s,20s} / Repeat {18s,20s} — re-armed
-- unconditionally.
local function shroudTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHROUD_OF_DARKNESS)
    schedule(guid, "shroud", {18000, 20000}, function()
        shroudTick(creature, guid)
    end)
end

-- C++ boss_zuramatAI::JustEngagedWith: Talk(SAY_AGGRO) — unconditional
-- (the auriaya engage-port precedent); the two ScheduleTasks timers
-- seed their combat timers below; the BossAI::JustEngagedWith
-- passthrough leg has no bridge.
local function zuramatEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "voidshift", 9000, function()
        voidShiftTick(creature, guid)
    end)
    schedule(guid, "shroud", {18000, 20000}, function()
        shroudTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function zuramatCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function zuramatKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function zuramatJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ZURAMAT, 1, zuramatEnterCombat)
RegisterCreatureEvent(ENTRY_ZURAMAT, 2, zuramatCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ZURAMAT, 3, zuramatKilledUnit)
RegisterCreatureEvent(ENTRY_ZURAMAT, 4, zuramatJustDied)
RegisterCreatureEvent(ENTRY_ZURAMAT, 4, zuramatCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ZURAMAT, 23, zuramatCancelCombatTimers)
