-- The Slave Pens instance script — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- instance_the_slave_pens.cpp (instance_the_slave_pens,
-- InstanceMapScript "instance_the_slave_pens", map 547) and
-- the_slave_pens.h (SPDataTypes, SPCreaturesIds).
-- RegisterMapEvent binds the map-level Eluna instance hooks
-- (instance_serpent_shrine.lua precedent).
--
-- PORTED — LoadObjectData + flamecaller GUID capture
-- (OnCreatureCreate, event 5): the C++ ctor loads creatureData
-- (instance_the_slave_pens.cpp:36-51) and OnCreatureCreate latches
-- each listed entry's GUID under its DATA_* type via the base
-- AddObject leg (InstanceScript.cpp:294-297), while the three
-- Earthen Ring flamecallers (25754) take DATA_FLAMECALLER_000/001/
-- 002 (7/8/9) in spawn order via the counter switch (:62-84).
-- The Lua twin is creature:SetGuidData(dataID)
-- (engine/world/instance_guid_store.go — "the Lua mirror of the
-- C++ OnCreatureCreate entry-switch latching"; a RegisterMapEvent
-- event-5 population route is explicitly blessed there). The
-- flamecaller counter is derived from the live guid store itself
-- (first unlatched slot wins, via creature:GetGuidDataLow) instead
-- of a persisted counter: the engine store is in-memory and empty
-- on a fresh server run, so a reload re-latches all three slots in
-- spawn order, and a mid-instance respawn finds every slot latched
-- and latches nothing — the C++ counter-past-002 behavior.
-- Consumers read the store back via creature:GetGuidData /
-- GetInstanceCreature (lua_creature_events.go:6654-6707):
-- npc_ahune_bunny (MovePoint choreography, SHAMANS_LOOK_FOR_
-- OPENING / FOUND_OPENING casts, ResetFlameCallers evade) and
-- npc_earthen_ring_flamecaller (LookOpening bonfire/beam-bunny
-- resolution, npc_earthen_ring_flamecaller.lua).
--
-- STATIC-SPAWN DELTA: C++ OnCreatureCreate fires at map load for
-- every static spawn; Go fires event 5 lazily when the motion
-- sweep materializes the creature (creaturemotion.go:4984 ->
-- motionFor -> triggerInstanceCreatureCreate), i.e. once a player
-- is in range. The latches land before any consumer reads them —
-- every GetGuidData consumer runs inside the Ahune event, after
-- players reach the flamecaller/bunny area — the same documented-
-- delta class as the Steam Vault GO-create handler.
--
-- NOTHING ELSE TO PORT: the C++ script has no SetData/GetData,
-- no Update, no save/load and no boss-state arms (EncounterCount
-- 3 is header-only; boss state rides the BossAI shim in the boss
-- files). The file's own header comment says the placeholder
-- exists for dungeon-finder credit; the Go LFG credit path does
-- not consult instance scripts.

local MAP_SLAVE_PENS = 547

local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

-- the_slave_pens.h SPDataTypes.
local DATA_AHUNE = 4
local DATA_AHUNE_BUNNY = 5
local DATA_FROZEN_CORE = 6
local DATA_FLAMECALLER_000 = 7
local DATA_FLAMECALLER_001 = 8
local DATA_FLAMECALLER_002 = 9
local DATA_BONFIRE_BUNNY_000 = 10
local DATA_BONFIRE_BUNNY_001 = 11
local DATA_BONFIRE_BUNNY_002 = 12
local DATA_BEAM_BUNNY_000 = 13
local DATA_BEAM_BUNNY_001 = 14
local DATA_BEAM_BUNNY_002 = 15
local DATA_LUMA_SKYMOTHER = 16

-- the_slave_pens.h SPCreaturesIds.
local NPC_AHUNE = 25740
local NPC_FROZEN_CORE = 25865
local NPC_LUMA_SKYMOTHER = 25697
local NPC_AHUNE_LOC_BUNNY = 25745
local NPC_EARTHEN_RING_FLAMECALLER = 25754
local NPC_SHAMAN_BONFIRE_BUNNY_000 = 25971
local NPC_SHAMAN_BONFIRE_BUNNY_001 = 25972
local NPC_SHAMAN_BONFIRE_BUNNY_002 = 25973
local NPC_SHAMAN_BEAM_BUNNY_000 = 25964
local NPC_SHAMAN_BEAM_BUNNY_001 = 25965
local NPC_SHAMAN_BEAM_BUNNY_002 = 25966

-- C++ creatureData (instance_the_slave_pens.cpp:36-51):
-- entry -> DATA_* type latched by LoadObjectData.
local ENTRY_TO_DATA = {
    [NPC_AHUNE] = DATA_AHUNE,
    [NPC_FROZEN_CORE] = DATA_FROZEN_CORE,
    [NPC_AHUNE_LOC_BUNNY] = DATA_AHUNE_BUNNY,
    [NPC_SHAMAN_BONFIRE_BUNNY_000] = DATA_BONFIRE_BUNNY_000,
    [NPC_SHAMAN_BONFIRE_BUNNY_001] = DATA_BONFIRE_BUNNY_001,
    [NPC_SHAMAN_BONFIRE_BUNNY_002] = DATA_BONFIRE_BUNNY_002,
    [NPC_SHAMAN_BEAM_BUNNY_000] = DATA_BEAM_BUNNY_000,
    [NPC_SHAMAN_BEAM_BUNNY_001] = DATA_BEAM_BUNNY_001,
    [NPC_SHAMAN_BEAM_BUNNY_002] = DATA_BEAM_BUNNY_002,
    [NPC_LUMA_SKYMOTHER] = DATA_LUMA_SKYMOTHER,
}

-- C++ flamecaller counter switch (:62-84): spawn order 000/001/002.
local FLAMECALLER_SLOTS = {
    DATA_FLAMECALLER_000,
    DATA_FLAMECALLER_001,
    DATA_FLAMECALLER_002,
}

-- C++ OnCreatureCreate (instance_the_slave_pens.cpp:62-84).
RegisterMapEvent(MAP_SLAVE_PENS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    local dataID = ENTRY_TO_DATA[entry]
    if dataID ~= nil then
        creature:SetGuidData(dataID)
        return
    end
    if entry == NPC_EARTHEN_RING_FLAMECALLER then
        for _, slot in ipairs(FLAMECALLER_SLOTS) do
            if creature:GetGuidDataLow(slot) == 0 then
                creature:SetGuidData(slot)
                return
            end
        end
    end
end)
