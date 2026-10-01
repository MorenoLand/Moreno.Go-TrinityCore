-- Timmy the Cruel (Stratholme)
-- Lua port of src/server/scripts/EasternKingdoms/Stratholme/
-- boss_timmy_the_cruel.cpp
-- (boss_timmy_the_cruelAI : public ScriptedAI (NOT BossAI —
-- Initialize() in the constructor zeroes the diff-timer and the
-- HasYelled latch; GetAI via GetStratholmeAI -> GetInstanceAI);
-- Reset -> Initialize(); JustEngagedWith -> Talk(SAY_SPAWN) once
-- per engagement cycle (HasYelled latch, cleared by Reset);
-- UpdateAI old-style diff-timer arm (no EventMap, no UNIT_STATE_
-- CASTING gate) + DoMeleeAttackIfReady; registered by AddSC_boss_
-- timmy_the_cruel in eastern_kingdoms_script_loader.cpp
-- (declaration line 132, call line 310)). The Stratholme block
-- is OPEN.
-- Entry (verifiable from the C++ sources): STRCreatureIds names
-- NPC_TIMMY_THE_CRUEL = 10808 in stratholme.h, and instance_
-- stratholme.cpp:115 spawns NPC_TIMMY_THE_CRUEL at
-- timmyTheCruelSpawnPosition once TIMMY_THE_CRUEL_CRUSADERS_
-- REQUIRED (15) crusaders are killed — the name-to-entry tie is
-- C++-verified (kalecgos precedent, ramstein-strength).
-- Whole-server-tree grep confirms boss_timmy_the_cruel.cpp as
-- the only source of the boss AI (loader lines only otherwise;
-- instance_stratholme.cpp's TIMMY spawn leg is the summon, not
-- the creature script).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. There is no UNIT_STATE_CASTING gate in the C++
-- arm, so the timer fires unconditionally.
-- Verifiable numbers (file's own enums): SPELL_RAVENOUSCLAW =
-- 17470 / SAY_SPAWN = 0.
-- Init-delay nuance: the C++ arms the RavenousClaw timer in
-- Initialize() (constructor, spawn time) and re-arms it in
-- Reset(), but the UpdateAI arm is gated on UpdateVictim(), so
-- the pump only executes once in combat. The Lua timer arms on
-- OnEnterCombat with the C++-exact init delay, matching the
-- observable cadence (maiden convention).
-- Ported arms (C++ JustEngagedWith + UpdateAI diff-timer):
-- - SAY_SPAWN: Talk(0) on the first OnEnterCombat after each
--   Reset; the HasYelled latch (cleared by C++ Reset) maps onto
--   a per-guid latch table cleared on OnReset (23) (kirtonos
--   latch convention; arugal Talk convention).
-- - RavenousClaw: 10s init -> 15s re-arm; DoCastVictim(17470)
--   non-triggered -> GetVictim + CastSpell (mr_smite
--   convention).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The GetStratholmeAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).

local NPC_TIMMY_THE_CRUEL = 10808

local SPELL_RAVENOUSCLAW = 17470

local SAY_SPAWN = 0

local timers = {}
local yelled = {}

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

local function onRavenousClaw(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_RAVENOUSCLAW)
    end
    schedule(guid, "ravenousclaw", 15000, function() onRavenousClaw(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if not yelled[guid] then
        creature:Talk(SAY_SPAWN)
        yelled[guid] = true
    end
    schedule(guid, "ravenousclaw", 10000, function() onRavenousClaw(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    yelled[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_TIMMY_THE_CRUEL, 1, onEnterCombat)
RegisterCreatureEvent(NPC_TIMMY_THE_CRUEL, 2, onCombatEnd)
RegisterCreatureEvent(NPC_TIMMY_THE_CRUEL, 4, onCombatEnd)
RegisterCreatureEvent(NPC_TIMMY_THE_CRUEL, 23, onReset)
