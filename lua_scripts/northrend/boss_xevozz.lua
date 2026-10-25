-- Xevozz (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_xevozz.cpp
-- (291 lines incl. license; 3 scripts — boss_xevozz (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_XEVOZZ = 7); npc_ethereal_sphere
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), DoAction
-- ACTION_SUMMON Talk leg, unbridged); spell_xevozz_summon_players
-- (SpellScriptLoader SpellScript); all registered from inside
-- AddSC_boss_xevozz(); loader decl 150 / call 345 per
-- northrend_script_loader.cpp — the SIXTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_moragg() (call 344); the call after it is
-- AddSC_boss_zuramat() (call 346) — the checkpoint sequence
-- (boss_moragg -> boss_xevozz) is followed).
-- Entries: 29266 Xevozz (violet_hold.h NPC_XEVOZZ, line 96) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_XEVOZZ = 7
-- (violet_hold.h line 57).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_xevozz"
-- hits boss_xevozz.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (both timer delays here are ranges).
-- creature:CastSpell(creature, spell) = DoCastAOE / DoCast(me)
-- (1/2-arg = non-triggered); creature:CastSpell(target, spell) =
-- DoCast(target, spell) (2-arg = non-triggered).
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
-- ScheduleTasks ARCANE_BARRAGE_VOLLEY — DoCastAOE(SPELL_ARCANE_
-- BARRAGE_VOLLEY 54202) = self-cast per the in-tree DoCastAOE
-- convention (boss_ai.go); init {8s,10s} / Repeat {8s,10s} —
-- unconditional re-arm.
-- ScheduleTasks ARCANE_BUFFET — SelectTarget(Random, 0, 45.0f, true)
-- -> DoCast(target, SPELL_ARCANE_BUFFET 54226), 2-arg =
-- non-triggered; init {10s,11s} / Repeat {15s,20s} — unconditional
-- re-arm. Random-player pool corrected 2026-10-25: threat-list
-- players within 45 yd (see randomThreatPlayerInRange); the old
-- instance-wide pool was the same bug class as moragg/erekem.
-- DOCUMENTED DEVIATIONS: none on the ported arms — both timers carry
-- C++-exact timing and targeting. The UpdateAI
-- DoMeleeAttackIfReady() leg has no bridge — melee is unmodeled.
-- DOCUMENTED-ONLY (in this header; entry 29266 is the sole
-- registration):
-- SAY_SPAWN (3) and SAY_CHARGED (4) are never Talk()ed anywhere in the
-- file (dead enum text, the cyanigosa precedent).
-- Talk(SAY_SUMMON_ENERGY 6) rides the SpellHit SPELL_ARCANE_POWER
-- (54160) / H_SPELL_ARCANE_POWER (59474) leg — stale "no SpellHit
-- bridge" retracted (2026-10-21): ev14 IS bridged (fires from the Go
-- spell path, boss_ai.go:2125/2134 + pet_combat.go:5222 +
-- spells.go:11765) — but the arm stays out: the only in-instance
-- source of these IDs is the unregistered npc_ethereal_sphere's
-- 3.0f-proximity DoCastAOE (boss_xevozz.cpp:227-241), and entry
-- 29271 can never exist in Go (Xevozz's sphere summon leg is
-- unbridged), so ev14 can never fire with 54160/59474.
-- The ScheduleTasks ethereal-sphere summon leg (Talk(SAY_REPEAT_
-- SUMMON 5), 5s init / 45-47s Repeat, random summon-spell pick from
-- {54102, 54137, 54138} via DoCast(me, ...), heroic 2.5s delayed
-- second cast, 33-35s delayed summons.DoAction(ACTION_SUMMON)) stays
-- out: the DoCast(me, summonSpell) legs are summon legs (the summoned
-- sphere entry 29271 carries the unbridged npc_ethereal_sphere AI —
-- its ARCANE_POWER proximity machine and SAY_ETHEREAL_SPHERE_SUMMON
-- DoAction Talk leg have no bridges; the Reset POWER_BALL_VISUAL /
-- POWER_BALL_DAMAGE_TRIGGER self-casts are bridgeable in isolation
-- (CastSpell(creature, spell)) but the sphere is unreachable — Xevozz's
-- summon leg above is unbridged, so 29271 can never exist in Go), the
-- 33-35s delayed summons.DoAction(ACTION_SUMMON) is a DoAction leg,
-- the second cast is IsHeroic()-gated (creature:IsHeroic() is bridged,
-- lua_creature_events.go:282, but it rides the same unbridged summon
-- leg), and the Talk rides the same leg — the emalon EVENT_OVERCHARGE
-- precedent (whole event out when its legs are unbridged). The JustSummoned MoveFollow leg
-- joins the no-MovementInform bridge queue; the JustReachedHome
-- instance->SetData(DATA_HANDLE_CELLS, DATA_XEVOZZ) leg joins the
-- no-instance-SetData bridge queue.
-- spell_xevozz_summon_players (SpellScript: OnEffectHitTarget
-- SPELL_EFFECT_DUMMY -> SPELL_MAGIC_PULL 50770) joins the
-- no-SpellScript-bridge queue. The file carries a TODO ("Implement
-- Ethereal Summon Target").

local ENTRY_XEVOZZ = 29266

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_ARCANE_BARRAGE_VOLLEY = 54202
local SPELL_ARCANE_BUFFET = 54226

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

-- C++ SelectTarget(Random, 0, 45.0f, true) draws the threat list
-- (UnitAI.h:162-184, playerOnly=true), not instance players: random
-- alive threat-list player within 45y (nefarian/vaelastrasz
-- precedent; the old instance-wide pool could buffet a player with
-- zero threat). Alive filter: dead targets are offline
-- (UNIT_STATE_DIED fails _IsTargetAcceptable, Creature.cpp:2508)
-- and skipped by SelectTargetList, while Go SortedEntries returns
-- dead entries too. C++ IsWithinCombatRange adds both combat
-- reaches (Unit.cpp:583); plain GetDistance is the standing
-- granularity delta.
local function randomThreatPlayerInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) <= maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks ARCANE_BARRAGE_VOLLEY: DoCastAOE(54202) =
-- self-cast per the in-tree DoCastAOE convention; init {8s,10s} /
-- Repeat {8s,10s} — re-armed unconditionally.
local function barrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_BARRAGE_VOLLEY)
    schedule(guid, "barrage", {8000, 10000}, function()
        barrageTick(creature, guid)
    end)
end

-- C++ ScheduleTasks ARCANE_BUFFET: SelectTarget(Random, 0, 45.0f,
-- true) -> DoCast(target, 54226), 2-arg = non-triggered; init
-- {10s,11s} / Repeat {15s,20s} — re-armed unconditionally.
local function buffetTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_ARCANE_BUFFET)
    end
    schedule(guid, "buffet", {15000, 20000}, function()
        buffetTick(creature, guid)
    end)
end

-- C++ boss_xevozzAI::JustEngagedWith: Talk(SAY_AGGRO) — unconditional
-- (the auriaya engage-port precedent); the two ScheduleTasks timers
-- seed their combat timers below; the BossAI::JustEngagedWith
-- passthrough leg has no bridge.
local function xevozzEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "barrage", {8000, 10000}, function()
        barrageTick(creature, guid)
    end)
    schedule(guid, "buffet", {10000, 11000}, function()
        buffetTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function xevozzCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function xevozzKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function xevozzJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_XEVOZZ, 1, xevozzEnterCombat)
RegisterCreatureEvent(ENTRY_XEVOZZ, 2, xevozzCancelCombatTimers)
RegisterCreatureEvent(ENTRY_XEVOZZ, 3, xevozzKilledUnit)
RegisterCreatureEvent(ENTRY_XEVOZZ, 4, xevozzJustDied)
RegisterCreatureEvent(ENTRY_XEVOZZ, 4, xevozzCancelCombatTimers)
RegisterCreatureEvent(ENTRY_XEVOZZ, 23, xevozzCancelCombatTimers)
