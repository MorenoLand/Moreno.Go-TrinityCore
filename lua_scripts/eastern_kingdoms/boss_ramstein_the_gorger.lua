-- Ramstein the Gorger (Stratholme)
-- Lua port of src/server/scripts/EasternKingdoms/Stratholme/
-- boss_ramstein_the_gorger.cpp
-- (boss_ramstein_the_gorgerAI : public ScriptedAI (NOT BossAI —
-- Initialize() in the constructor zeroes the two diff-timers;
-- GetAI via GetStratholmeAI -> GetInstanceAI); Reset ->
-- Initialize(); JustEngagedWith empty; UpdateAI old-style
-- diff-timer arms (no EventMap, no UNIT_STATE_CASTING gate) +
-- DoMeleeAttackIfReady; registered by AddSC_boss_ramstein_the_
-- gorger in eastern_kingdoms_script_loader.cpp (declaration line
-- 131, call line 309)). The Stratholme block is OPEN.
-- Entry (verifiable from the C++ sources): STRCreatureIds names
-- NPC_RAMSTEIN = 10439 in stratholme.h, and instance_stratholme.
-- cpp:349 summons NPC_RAMSTEIN (4032.84f, -3390.24f, 119.73f,
-- 4.71f) from the Baron when the abomination count hits zero,
-- logging "Ramstein spawned" — the name-to-entry tie is C++-
-- verified (kalecgos precedent, stronger than name-only).
-- Whole-server-tree grep confirms boss_ramstein_the_gorger.cpp
-- as the only source of the boss AI (loader lines only
-- otherwise; instance_stratholme.cpp's TYPE_RAMSTEIN SetData/
-- GetData legs are the data enum, not the creature script).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. There is no UNIT_STATE_CASTING gate in the C++
-- arms, so timers fire unconditionally.
-- Verifiable numbers (file's own enums): SPELL_TRAMPLE = 5568 /
-- SPELL_KNOCKOUT = 17307 / NPC_MINDLESS_UNDEAD = 11030.
-- Init-delay nuance: the C++ arms both timers in Initialize()
-- (constructor, spawn time) and re-arms them in Reset(), but the
-- UpdateAI arms are gated on UpdateVictim(), so the pumps only
-- execute once in combat. The Lua timers arm on OnEnterCombat
-- with the C++-exact init delays, matching the observable
-- cadence (maiden convention).
-- Ported arms (C++ UpdateAI diff-timers):
-- - Trample: 3s init -> 7s re-arm; DoCast(me, 5568) non-
--   triggered self-cast -> CastSpell on self (headless_horseman
--   self-cast precedent).
-- - Knockout: 12s init -> 10s re-arm; DoCastVictim(17307) non-
--   triggered -> GetVictim + CastSpell (mr_smite convention).
-- - JustDied: 30x SummonCreature(NPC_MINDLESS_UNDEAD 11030,
--   3969.35+irand(-10,10), -3391.87+irand(-10,10), 119.11, 5.91,
--   TEMPSUMMON_TIMED_OR_DEAD_DESPAWN (Go bridge type 0; the C++
--   enum is 1-based, ObjectDefines.h:59, while the Go bridge is
--   0-based, lua_creature_events.go:1653 — the old type 2 was Go
--   TIMED_DESPAWN, whose timer runs unconditionally in combat,
--   pet_combat.go:595), 30min=1800000ms) via
--   creature:SummonCreature + the
--   mob->AI()->AttackStart(SelectNearestTarget(100)) leg PORTED
--   below (nearest-alive-player twin + EngageWithTarget).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustDied's AttackStart leg: PORTED above (see Ported arms).
-- - JustDied's instance->SetData(TYPE_RAMSTEIN, DONE) leg and
--   the GetStratholmeAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).

local NPC_RAMSTEIN = 10439

local SPELL_TRAMPLE = 5568
local SPELL_KNOCKOUT = 17307
local NPC_MINDLESS_UNDEAD = 11030

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

local function onTrample(creature, guid)
    creature:CastSpell(creature, SPELL_TRAMPLE)
    schedule(guid, "trample", 7000, function() onTrample(creature, guid) end)
end

local function onKnockout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKOUT)
    end
    schedule(guid, "knockout", 10000, function() onKnockout(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "trample", 3000, function() onTrample(creature, guid) end)
    schedule(guid, "knockout", 12000, function() onKnockout(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: 30x SummonCreature(NPC_MINDLESS_UNDEAD,
-- 3969.35+irand(-10,10), -3391.87+irand(-10,10), 119.11, 5.91,
-- TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, 30min); irand is inclusive
-- on both ends, matching math.random(-10, 10). Each mob then
-- AttackStarts the nearest target within 100 yd (C++
-- SelectNearestTarget(100): nearest HOSTILE unit — the
-- players-only twin below is the EngageWithTarget funnel's
-- player-target-only limit, so a non-player C++ pick no-ops
-- through it; the pick is computed once — positions are
-- static across the synchronous loop and the new summons
-- share Ramstein's faction, so they are never candidates).
-- The instance SetData(TYPE_RAMSTEIN, DONE) leg stays out
-- (no instance bridge).
local function nearestAlivePlayer(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local best, bestDist = nil, nil
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            local dist = creature:GetDistance(p)
            if dist - creature:GetCombatReach() - 1.5 < maxDist and (bestDist == nil or dist < bestDist) then
                best, bestDist = p, dist
            end
        end
    end
    return best
end

local function onJustDied(_, creature)
    cancelTimers(creature:GetGUID())
    local target = nearestAlivePlayer(creature, 100)
    for _ = 1, 30 do
        local mob = creature:SummonCreature(NPC_MINDLESS_UNDEAD,
            3969.35 + math.random(-10, 10),
            -3391.87 + math.random(-10, 10),
            119.11, 5.91, 0, 1800000)
        if mob and target then
            mob:EngageWithTarget(target)
        end
    end
end

RegisterCreatureEvent(NPC_RAMSTEIN, 1, onEnterCombat)
RegisterCreatureEvent(NPC_RAMSTEIN, 2, onCombatEnd)
RegisterCreatureEvent(NPC_RAMSTEIN, 4, onJustDied)
RegisterCreatureEvent(NPC_RAMSTEIN, 23, onCombatEnd)
