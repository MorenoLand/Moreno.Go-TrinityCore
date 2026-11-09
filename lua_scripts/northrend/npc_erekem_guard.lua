-- Erekem Guard (Violet Hold) — Lua port of the npc_erekem_guard
-- half of src/server/scripts/Northrend/VioletHold/boss_erekem.cpp
-- (323 lines; CreatureScript via GetVioletHoldAI (ScriptedAI),
-- entry 29395, violet_hold.h NPC_EREKEM_GUARD line 105).
-- The guard AI itself is melee-only: ScheduledTasks (:288-309) is
-- DEAD CODE (typo'd name, never called — see boss_erekem.lua), so
-- STRIKE / HOWLING_SCREECH / GUSHING_WOUND never fire in C++; Reset
-- (:271-273) only cancels the scheduler and JustEngagedWith
-- (:274-277) only calls DoZoneInCombat() (no bridge). The guard's
-- only live Lua leg is the spawn GUID slot-fill below, which feeds
-- Erekem's phase machine (boss_erekem.lua, CheckGuardAlive).
-- C++ instance_violet_hold.cpp:234-247 (OnCreatureCreate): the first
-- guard created latches ErekemGuardGUIDs[0] (DATA_EREKEM_GUARD_1 =
-- 15), the second latches [1] (DATA_EREKEM_GUARD_2 = 16); the
-- OnCreatureRemove twin clears the slot on despawn. The Lua mirror
-- rides creature:SetGuidData (lua_creature_events.go:6457, latches
-- the calling creature's GUID under the data id) from the spawn
-- handler (event 5, motionFor fires on first spawn and on respawn):
-- a guard whose GUID is already latched in either slot (respawn
-- reuses the GUID) is a no-op, otherwise it takes the first empty
-- slot. No OnCreatureRemove twin exists in the Lua instance surface,
-- so a dead guard's slot keeps its GUID — harmless: Erekem reads the
-- slot through GetInstanceCreature + IsAlive, and a dead or
-- despawned creature reads as not-alive either way.
-- Eluna creature events: 5 OnSpawn.

local ENTRY_EREKEM_GUARD = 29395
local DATA_EREKEM_GUARD_1 = 15
local DATA_EREKEM_GUARD_2 = 16

local function erekemGuardSpawn(event, creature)
    local guid = creature:GetGUID()
    if creature:GetGuidData(DATA_EREKEM_GUARD_1) == guid
            or creature:GetGuidData(DATA_EREKEM_GUARD_2) == guid then
        return
    end
    if creature:GetGuidData(DATA_EREKEM_GUARD_1) == 0 then
        creature:SetGuidData(DATA_EREKEM_GUARD_1)
    else
        creature:SetGuidData(DATA_EREKEM_GUARD_2)
    end
end

RegisterCreatureEvent(ENTRY_EREKEM_GUARD, 5, erekemGuardSpawn)

-- C++ npc_erekem_guardAI::JustEngagedWith (boss_erekem.cpp:274-277):
-- DoZoneInCombat() only (DoZoneInCombat bridged,
-- engine/world/lua_creature_events.go:1996).
local function erekemGuardEngaged(event, creature, target)
    creature:DoZoneInCombat()
end

RegisterCreatureEvent(ENTRY_EREKEM_GUARD, 1, erekemGuardEngaged)
