-- Warden Mellichar (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/arcatraz.cpp (npc_warden_mellichar — the AI class
-- AddSC_arcatraz registers alongside npc_millhouse_manastorm).
-- Entry: NPC_MELLICHAR = 20904 (arcatraz.h).
-- This file is the DATA_MELLICHAR = 10 guid latch + the
-- Reset() arms: the C++ instance script latches Mellichar on
-- creation (instance_arcatraz.cpp OnCreatureCreate), and
-- Skyriss's intro phase-2 kill leg reads it via
-- GetInstanceCreature(10) (boss_harbinger_skyriss.cpp:177-185).
-- SetGuidData is the Lua population leg of the instance guid
-- store (lua_creature_events.go:6643); ev5 fires on first grid
-- activation too (motionFor initial-spawn funnel,
-- creaturemotion.go:1268).
-- PORTED — Reset() (arcatraz.cpp:303-312): the Initialize()
-- locals (IsRunning/CanSpawn/Phase/EventProgress_Timer) have
-- no Lua twin — the UpdateAI phase machine stays blocked
-- (below) — so the port is the three observable arms:
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) (59,
-- 0x2; UnitDefines.h:125; SetFlag lua_creature_events.go:6022),
-- DoCast(me, SPELL_TARGET_OMEGA = 36852) as a self-cast
-- (creature:CastSpell(creature, ...), DoCastAOE convention),
-- and instance->SetBossState(DATA_HARBINGER_SKYRISS = 3,
-- NOT_STARTED) (SetBossState lua_creature_events.go:6605),
-- guarded by GetBossState(3) ~= DONE (zereketh precedent).
-- ev23 is the Reset hook: it pre-fires on spawn and on evade
-- (fireCreatureSpawned, lua_creature_events.go:3165) and also
-- fires ahead of OnDied on the kill path (Kill method), so the
-- IsDead guard is required — C++ Reset() never runs on death.
-- The rest of Mellichar's AI stays unported: the UpdateAI
-- phase machine is gated on instance GetData(DATA_WARDEN_1..4)
-- (creature:GetData is the per-creature ScriptData store,
-- lua_creature_events.go:809 — no instance GetData bridge)
-- and HandleGameObject (no Lua method); JustEngagedWith
-- (Talk YELL_INTRO1 + SPELL_BUBBLE_VISUAL + SetBossState(3,1)
-- + HandleGameObject) stays out — its trigger is the MoveInLOS
-- sighting override (ev27 engage-only, no Z/arc/
-- CanCreatureAttack replicas), not combat entry; see
-- lua_scripts/arcatraz/npc_millhouse_manastorm.lua for the
-- recorded yells/spells/coords. No RegisterLuaBoss:
-- Mellichar is not a BossAI creature.

local ENTRY_MELLICHAR = 20904
local DATA_MELLICHAR = 10 -- arcatraz.h:43
local DATA_HARBINGER_SKYRISS = 3 -- arcatraz.h:34
local SPELL_TARGET_OMEGA = 36852 -- arcatraz.cpp:270

local UNIT_FIELD_FLAGS = 59 -- OBJECT_END(0x6)+0x35, UpdateFields.h:117
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- UnitDefines.h:125

RegisterCreatureEvent(ENTRY_MELLICHAR, 5, function(_, creature)
    creature:SetGuidData(DATA_MELLICHAR)
end)

RegisterCreatureEvent(ENTRY_MELLICHAR, 23, function(_, creature)
    if creature:IsDead() then
        return
    end
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:CastSpell(creature, SPELL_TARGET_OMEGA)
    if creature:GetBossState(DATA_HARBINGER_SKYRISS) ~= 3 then
        creature:SetBossState(DATA_HARBINGER_SKYRISS, 0)
    end
end)
