-- The Forge of Souls instance script — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/instance_forge_of_souls.cpp
-- (class instance_forge_of_souls : public InstanceMapScript, map 632).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_violet_hold.lua / instance_vault_of_archavon.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 632
-- (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- bronjahm / devourerOfSouls — the two OnCreatureCreate binds
-- (instance_forge_of_souls.cpp:64-76).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the two boss binds above. Fires on
--   materialization (motionFor funnel, instance_hooks.go:311-320); a
--   static template spawn never materializes server-side and is never
--   bound (naxxramas LoadGrid delta).
-- - GetGuidData (instance_forge_of_souls.cpp:121-136): the Lua mirror
--   is the instanceData binds above, read as instanceData.bronjahm /
--   instanceData.devourerOfSouls (oculus precedent); the C++ consumers
--   ride unported machines in the boss files — the fragment
--   IsSummonedBy relay (boss_bronjahm.cpp:238-242, no
--   instance-GetGuidData bridge on the fragment side) and the
--   devourer outro loop (boss_devourer_of_souls.cpp:215-235, instance
--   GetData team + map summons) — so the GUID legs as consumed stay
--   out there.
-- - SetBossState: the C++ instance script has NO boss-state override
--   (no OnBossStateChange / SetBossState call anywhere in the file) —
--   the IN_PROGRESS / DONE / NOT_STARTED legs run at engage / death /
--   reset time in the boss handlers (boss_bronjahm.lua /
--   boss_devourer_of_souls.lua), so there is nothing instance-side to
--   port and NO ON_UPDATE poll (the palehoof / oculus precedent: a
--   poll only drives legs with a live consumer).
-- Unmodeled (documented-only, no bridges):
-- - OnPlayerEnter team latch (:56-61) + GetData(DATA_TEAM_IN_INSTANCE)
--   (:104-115): no instance GetData/SetData bridge from Lua, and the
--   latch's only consumers are the unported GetCreatureEntry team swap
--   and the devourer outro loop.
-- - GetCreatureEntry team swap (:78-101, NPC_SYLVANAS_PART1 /
--   NPC_LORALEN / NPC_KALIRA -> Alliance twins): rides the team latch
--   above + map-player enumeration, both unbridged.
-- - Boss boundaries (:27-31, CircleBoundary bronjahm /
--   ParallelogramBoundary devourer): no boundary bridge in Go.
-- - SetHeaders(DataHeader "FOS") / SetBossNumber(EncounterCount 2) /
--   SaveToDB: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5917) and read via
--   creature:GetBossState (lua_creature_events.go:5936).
-- - No DoorData / MinionData / LoadObjectData in the C++ file.
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_FORGE_OF_SOULS = 632
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_BRONJAHM = 36497 -- forge_of_souls.h:40
local NPC_DEVOURER = 36502 -- forge_of_souls.h:41

local bossBinds = {
    [NPC_BRONJAHM] = "bronjahm",
    [NPC_DEVOURER] = "devourerOfSouls",
}

RegisterMapEvent(MAP_FORGE_OF_SOULS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
