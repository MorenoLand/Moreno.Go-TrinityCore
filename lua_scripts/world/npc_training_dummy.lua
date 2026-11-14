-- Training Dummy (damage sink) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1463-1500
-- (npc_training_dummy, NullCreatureAI). 15 TDB entries with
-- ScriptName npc_training_dummy: 4952, 17578, 24792, 30527, 31143,
-- 31144, 31146, 32541, 32542, 32543, 32545, 32546, 32547, 32666, 32667.
-- Eluna creature event: 9 OnDamageTaken.
-- Ported arm (C++-exact): DamageTaken (:1467) — damage = 0. The ev9
-- second return rewrites damage (firesworn precedent,
-- boss_garr.lua:160-175: return false, 0).
-- NOT ported: the 5s per-attacker combat timer (UpdateAI :1477) —
-- GetCombatManager/EndCombat have no bridge; creature:CombatStop
-- stops only own combat, and the C++ arm ends the ATTACKER's PvE
-- combat ref.

local TRAINING_DUMMY_ENTRIES = {
    4952, 17578, 24792, 30527, 31143, 31144, 31146, 32541, 32542,
    32543, 32545, 32546, 32547, 32666, 32667,
}

local function onDamageTaken()
    return false, 0
end

for _, entry in ipairs(TRAINING_DUMMY_ENTRIES) do
    RegisterCreatureEvent(entry, 9, onDamageTaken)
end
