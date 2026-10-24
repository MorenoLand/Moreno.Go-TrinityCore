-- Violet Hold Defense System (Violet Hold) — Lua port of
-- npc_violet_hold_defense_systemAI in src/server/scripts/Northrend/
-- VioletHold/violet_hold.cpp:1300-1339.
-- Entry: NPC_DEFENSE_SYSTEM = 30837 (violet_hold.h:121).
-- Fight shape (C++-exact): Reset -> ScheduledTasks + DespawnOrUnsummon
-- (7s). ScheduledTasks: 4s first volley, Repeat 1s — DoCastAOE
-- (ARCANE_LIGHTNING_DAMAGE 57912 + ARCANE_LIGHTNING_DUMMY 57930) on
-- every volley, and on the 3rd volley (repeat counter 2) DoCastAOE
-- ARCANE_LIGHTNING_INSTAKILL 58152 instead of re-arming. DoCastAOE
-- ports as self-cast (boss_ai.go convention). No instance reference,
-- movement, gossip, or cross-AI call in this AI.
-- Latent until a summoner exists: the defense system arrives via the
-- unported Sinclari intro / activation-crystal ProcessEvent arms
-- (violet_hold.cpp:459, instance_violet_hold.cpp:932) — the handler
-- only runs if the creature exists, so nothing is fabricated.

local ENTRY_DEFENSE_SYSTEM = 30837 -- violet_hold.h:121
local SPELL_ARCANE_LIGHTNING_DAMAGE = 57912
local SPELL_ARCANE_LIGHTNING_DUMMY = 57930
local SPELL_ARCANE_LIGHTNING_INSTAKILL = 58152

local function defenseSystemReset(event, creature)
    local volley = 0
    local function volleyTick()
        volley = volley + 1
        creature:CastSpell(creature, SPELL_ARCANE_LIGHTNING_DAMAGE)
        creature:CastSpell(creature, SPELL_ARCANE_LIGHTNING_DUMMY)
        if volley == 3 then
            creature:CastSpell(creature, SPELL_ARCANE_LIGHTNING_INSTAKILL)
        else
            CreateLuaEvent(volleyTick, 1000)
        end
    end
    CreateLuaEvent(volleyTick, 4000)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 7000)
end

RegisterCreatureEvent(ENTRY_DEFENSE_SYSTEM, 23, defenseSystemReset)
