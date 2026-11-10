-- Warden Mellichar (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/arcatraz.cpp (npc_warden_mellichar — the AI class
-- AddSC_arcatraz registers alongside npc_millhouse_manastorm).
-- Entry: NPC_MELLICHAR = 20904 (arcatraz.h).
-- This file is ONLY the DATA_MELLICHAR = 10 guid latch: the
-- C++ instance script latches Mellichar on creation
-- (instance_arcatraz.cpp OnCreatureCreate), and Skyriss's
-- intro phase-2 kill leg reads it via GetInstanceCreature(10)
-- (boss_harbinger_skyriss.cpp:177-185). SetGuidData is the
-- Lua population leg of the instance guid store (lua_creature_
-- events.go:6643); ev5 fires on first grid activation too
-- (motionFor initial-spawn funnel, creaturemotion.go:1268).
-- The rest of Mellichar's AI stays unported: the UpdateAI
-- phase machine is gated on instance GetData(DATA_WARDEN_1..4)
-- (no instance GetData bridge) and HandleGameObject (no Lua
-- method); see lua_scripts/arcatraz/npc_millhouse_manastorm.lua
-- for the recorded yells/spells/coords. No RegisterLuaBoss:
-- Mellichar is not a BossAI creature.

local ENTRY_MELLICHAR = 20904
local DATA_MELLICHAR = 10 -- arcatraz.h:43

RegisterCreatureEvent(ENTRY_MELLICHAR, 5, function(_, creature)
    creature:SetGuidData(DATA_MELLICHAR)
end)
