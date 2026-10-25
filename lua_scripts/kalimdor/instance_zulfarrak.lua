-- Zul'Farrak instance script — Lua port of the bridgeable arm of
-- src/server/scripts/Kalimdor/ZulFarrak/instance_zulfarrak.cpp
-- (instance_zulfarrak, InstanceMapScript "instance_zulfarrak", map 209).
-- RegisterMapEvent binds the map-level Eluna instance hook: C++ fires both
-- the map and instance binding maps at every fire site, so the map binding
-- covers every Zul'Farrak instance (engine/scripting/runtime.go:653).
-- INSTANCE_EVENT_ON_CREATURE_CREATE (5) fires from the motion funnel on
-- materialization (engine/world/creaturemotion.go:736), the Go twin of
-- Creature::AddToWorld (Creature.cpp:301) that C++ OnCreatureCreate rides.
--
-- Ported arm (C++-exact, unconditional): OnCreatureCreate sets
-- REACT_PASSIVE (0) on the five caged Bly crew members — Bly 7604, Raven
-- 7605, Oro 7606, Weegli 7607, Murta 7608 (zulfarrak.h ZFEntries;
-- instance_zulfarrak.cpp:139-164, "starts out passive (in a cage)").
-- creature:SetReactState writes motion.ReactState (engine/world/
-- lua_creature_events.go:1934) and the engine honors it in isCreaturePassive,
-- so the arm has real effect, not just a latch.
--
-- Stay-outs from the same OnCreatureCreate: Zumrah/Bly/Raven/Oro/Weegli/
-- Murta GUID latching (no per-instance Lua data store — the instance_data
-- table passed to map hooks is empty per fire, engine/world/
-- instance_hooks.go:19); Gahz'rilla 7273 DisappearAndDie/encounter latch
-- (instance encounter state has no bridge). The release leg that flips
-- the crew aggressive lives in go_troll_cage OnGossipHello (zulfarrak.cpp:
-- 217-230, instance SetData EVENT_PYRAMID/CAGES_OPEN + GetGuidData +
-- SetReactState/SetWalk/SetHomePosition/MovePoint/SetFaction) — GO gossip
-- select + instance-data + MovePoint legs are unbridged, so until that leg
-- lands the crew stays passive after this port; that gap is pre-existing
-- (the cage gossip was never ported) and is documented, not introduced,
-- by this file.

local MAP_ZULFARRAK = 209
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local CAGED_CREW = {
    [7604] = true, -- ENTRY_BLY
    [7605] = true, -- ENTRY_RAVEN
    [7606] = true, -- ENTRY_ORO
    [7607] = true, -- ENTRY_WEEGLI
    [7608] = true, -- ENTRY_MURTA
}

RegisterMapEvent(MAP_ZULFARRAK, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if CAGED_CREW[creature:GetEntry()] then
        creature:SetReactState(0) -- REACT_PASSIVE
    end
end)
