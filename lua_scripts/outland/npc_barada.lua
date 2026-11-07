-- Hellfire Peninsula: Anchorite Barada — Lua port of
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp
-- (class npc_barada : CreatureScript("npc_barada")).
-- Entry: 22431 (wowhead npc=22431/anchorite-barada, verified
-- 2026-11-07; quest 10935 "The Exorcism of Colonel Jules";
-- no NPC_ constant exists in the C++ file).
-- Eluna creature events: 4 JustDied.
-- Ported arm:
-- - JustDied (zone_hellfire_peninsula.cpp:626-632, event 4):
--   Colonel Jules (entry 22432) -> DoAction(ACTION_JULES_MOVE_HOME
--   4) + RemoveAllAuras(). Bridges: FindNearestCreature
--   (lua_creature_events.go:5763), DoAction (lua_creature_events.go:827),
--   RemoveAllAuras (lua_creature_events.go:1548).
-- Documented approximation: C++ resolves Jules via the GUID
-- latched when the exorcism event starts; Lua re-derives the same
-- creature with FindNearestCreature(22432, 20.0) — the identical
-- call the C++ start-event leg uses (zone_hellfire_peninsula.cpp:600).
-- The legs diverge only if Barada dies outside the event while
-- Jules idles within 20 yd; there is no GetCreatureByGUID bridge
-- (only GetPlayerByGUID, runtime.go:1160).
-- The rest of the exorcism event machine (Talk choreography,
-- SPELL_BARADAS_COMMAND casts, skull summons, Jules' hover/flight
-- DoAction arms) stays out — see npc_colonel_jules.lua.

local ENTRY_BARADA = 22431
local ENTRY_COLONEL_JULES = 22432

local ACTION_JULES_MOVE_HOME = 4

-- C++ JustDied: Jules gets ACTION_JULES_MOVE_HOME + aura strip.
local function onDied(_, creature)
    local jules = creature:FindNearestCreature(ENTRY_COLONEL_JULES, 20.0)
    if jules then
        jules:DoAction(ACTION_JULES_MOVE_HOME)
        jules:RemoveAllAuras()
    end
end

RegisterCreatureEvent(ENTRY_BARADA, 4, onDied)
