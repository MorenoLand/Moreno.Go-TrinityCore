-- Bridge console (Serpentshrine Cavern) — Lua port of
-- go_bridge_console in src/server/scripts/Outland/
-- CoilfangReservoir/SerpentShrine/instance_serpent_shrine.cpp.
-- Entry: GO_CONTROL_CONSOLE = 184568 (serpent_shrine.h has no
-- GO entry constant; the entry comes from the C++ file's
-- OnGameObjectCreate, instance_serpent_shrine.cpp:188).
-- OnGossipHello PORTED: instance->SetData(DATA_CONTROL_CONSOLE
-- 22, DONE) (instance_serpent_shrine.cpp:82-88), unconditional
-- as in C++ (no gates, player unused). C++ returns true, so Lua
-- returns true and the native use arms are suppressed
-- (chromaggus-lever / go_gundrak_altar.lua precedent).
-- Kept in its own file on the go_gundrak_altar.lua split: the
-- instance file owns the RegisterMapEvent machine; this file
-- owns the RegisterGameObjectGossipEvent registration, matching
-- the C++ file's two separate script classes.
-- instance->SetData rides the live bridge
-- (instance_hooks.go:51-65 fires INSTANCE_EVENT_ON_SET_DATA
-- (8)); the bridge-part legs live in
-- instance_serpent_shrine.lua. go:GetMap():GetInstanceData() is
-- the algalon precedent; nil-guarded for unbound maps.

local GO_CONTROL_CONSOLE = 184568

local DATA_CONTROL_CONSOLE = 22 -- serpent_shrine.h DATA_CONTROL_CONSOLE
local ENCOUNTER_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local function onBridgeConsoleGossip(_, _, go)
    local mapObj = go:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data:SetData(DATA_CONTROL_CONSOLE, ENCOUNTER_DONE)
    end
    return true
end

RegisterGameObjectGossipEvent(GO_CONTROL_CONSOLE, 1, onBridgeConsoleGossip)
