-- Centrifuge Construct (27641) — Lua port of the construct leg of
-- src/server/scripts/Northrend/Nexus/Oculus/instance_oculus.cpp
-- (class instance_oculus : public InstanceMapScript, map 578).
-- C++ has no creature script for this entry: the whole leg lives in
-- the instance script's OnUnitDeath
-- (instance_oculus.cpp:126-136) — counter -- + DoUpdateWorldState
-- (AMOUNT, count) + Varos RemoveAllAuras when the counter hits 0.
-- No Lua instance OnUnitDeath hook exists, so the leg ports as the
-- construct's own ev4 JustDied (same death instant, same entry
-- gate the C++ performs). The counter lives in the shared
-- instanceData table (instance_oculus.lua); Varos comes from the
-- instanceData bind, not GetInstanceCreature — the boss files
-- never latch GUIDs via SetGuidData, so the Go guid store holds
-- nothing for DATA_VAROS.
-- The create-time counter ++ leg ports in instance_oculus.lua
-- (ON_CREATURE_CREATE, alive only); the world-state SHOW leg
-- ports there too (Drakos/Varos DONE + player enter).

local NPC_CENTRIFUGE_CONSTRUCT = 27641 -- oculus.h:43
local DATA_VAROS = 1 -- oculus.h:38

local WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT = 3486 -- oculus.h:80

local function constructDied(event, creature, killer)
    local mapObj = creature:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local count = (data.constructCount or 0) - 1
    if count < 0 then
        count = 0
    end
    data.constructCount = count
    creature:UpdateWorldState(WORLD_STATE_CENTRIFUGE_CONSTRUCT_AMOUNT, count)
    if count == 0 and data.varos then
        data.varos:RemoveAllAuras()
    end
end

RegisterCreatureEvent(NPC_CENTRIFUGE_CONSTRUCT, 4, constructDied)
