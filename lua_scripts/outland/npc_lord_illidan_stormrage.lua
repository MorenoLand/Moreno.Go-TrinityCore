-- Lord Illidan Stormrage (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_lord_illidan_stormrageAI, Reset leg only). Entry 22083
-- TDB-verified ('Lord Illidan Stormrage', ScriptName
-- npc_lord_illidan_stormrage).
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arms (C++-exact; line refs are zone_shadowmoon_valley.cpp):
-- Reset (:948-953) — Initialize() field clears (unobservable) +
-- SetVisible(false). That is the whole port: nothing in the file ever
-- sets the controller visible again.
-- STAY OUT: the wave machine (UpdateAI :1022, SummonNextWave,
-- CheckEventFail, LiveCounter) — no summon / cross-AI field-write /
-- group-iteration bridges for the controller as a unit. JustEngagedWith
-- (:955) and MoveInLineOfSight (:956) are empty defs (not registered);
-- AttackStart is overridden empty (:958), so the controller never
-- engages.

local ENTRY_LORD_ILLIDAN_STORMRAGE = 22083

local function onSpawnOrReset(_, creature)
    creature:SetVisible(false)
end

RegisterCreatureEvent(ENTRY_LORD_ILLIDAN_STORMRAGE, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_LORD_ILLIDAN_STORMRAGE, 23, onSpawnOrReset)
