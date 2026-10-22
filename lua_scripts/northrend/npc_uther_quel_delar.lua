-- Uther (Quel'Delar, Halls of Reflection) — Lua port of the
-- DamageTaken arm of npc_uther_quel_delar in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :2458-2595; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 37225 NPC_UTHER (halls_of_reflection.h line 65) — the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 9 OnDamageTaken. The handler returns the
-- rewritten damage as the second return (the
-- engine/world/lua_creature_events.go fireCreatureDamageTaken
-- convention); the boolean first return only gates the empty
-- ScriptedAI::DamageTaken base, a provable no-op.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arm):
-- DamageTaken (:2480-2484) — ungated clamp: if damage >= current
--   health, damage = health - 1 ("Prevent to break Uther in intro
--   event during instance encounter", C++ :2471). Event 9 fires
--   pre-application, so creature:GetHealth() is the pre-damage value,
--   exactly like C++'s me->GetHealth() in the hook.
--
-- DOCUMENTED-ONLY:
-- The whole EVENT_UTHER_1..11 Talk machine (:2527-2569 —
--   SAY_UTHER_QUEL_DELAR_1..6, movement legs, the triggered self-cast
--   SPELL_ESSENCE_OF_CAPTURED_1 73036 at :2567) rides
--   Reset/DoAction/MovementInform sequence legs with no bridges.
-- DoAction ACTION_UTHER_START_SCREAM / ACTION_UTHER_OUTRO
--   (:2486-2500): no-DoAction bridge. MovementInform (:2502-2512):
--   no-MovementInform bridge.

local ENTRY_UTHER = 37225

-- C++ DamageTaken :2480-2484 — return the replacement damage as the
-- second return per the fireCreatureDamageTaken convention.
local function utherDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

RegisterCreatureEvent(ENTRY_UTHER, 9, utherDamageTaken)
RegisterCreatureEvent(ENTRY_UTHER, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- halls_of_reflection.cpp:2514: custom UpdateAI never calls DoMeleeAttackIfReady (RP event machine in C++, never melees)
end)
