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
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustDied's 30x me->SummonCreature(NPC_MINDLESS_UNDEAD =
--   11030, 3969.35f+irand(-10,10), -3391.87f+irand(-10,10),
--   119.11f, 5.91f, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, 30min) +
--   mob->AI()->AttackStart(me->SelectNearestTarget(100.0f)) leg
--   — STRAND: no Lua summon bridge (gurtogg/brutallus
--   DoSpawnCreature precedent) and no spawned-summon attack
--   hook.
-- - JustDied's instance->SetData(TYPE_RAMSTEIN, DONE) leg and
--   the GetStratholmeAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).

local NPC_RAMSTEIN = 10439

local SPELL_TRAMPLE = 5568
local SPELL_KNOCKOUT = 17307

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

RegisterCreatureEvent(NPC_RAMSTEIN, 1, onEnterCombat)
RegisterCreatureEvent(NPC_RAMSTEIN, 2, onCombatEnd)
RegisterCreatureEvent(NPC_RAMSTEIN, 4, onCombatEnd)
RegisterCreatureEvent(NPC_RAMSTEIN, 23, onCombatEnd)
