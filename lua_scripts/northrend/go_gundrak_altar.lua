-- Gundrak altars (Northrend) — Lua port of go_gundrak_altarAI in
-- src/server/scripts/Northrend/Gundrak/instance_gundrak.cpp:352-375.
-- Entries: GO_SLAD_RAN_ALTAR = 192518 / GO_MOORABI_ALTAR = 192519 /
-- GO_DRAKKARI_COLOSSUS_ALTAR = 192520 (gundrak.h:68-70).
-- OnGossipHello PORTED: SetFlag(GAMEOBJECT_FLAGS, NOT_SELECTABLE) +
-- SetGoState(ACTIVE), unconditional as in C++ (no gates, player unused).
-- Polarity is C++-exact: C++ returns true, so Lua returns true and the
-- native use arms are suppressed (chromaggus-lever precedent).
-- instance->SetData(DATA_STATUE_ACTIVATE 15, entry) — PORTED this tick:
-- the instance-data bridge is live (instance_hooks.go SetData fires
-- the Go-only INSTANCE_EVENT_ON_SET_DATA (8); the old "no
-- instance-data bridge" claim is stale). The statue beam/bridge
-- consumer lands with instance_gundrak.lua; until then the write
-- fires event 8 with no handler. go:GetMap():GetInstanceData() is
-- the algalon precedent; nil-guarded for unbound maps.

local GO_SLAD_RAN_ALTAR = 192518
local GO_MOORABI_ALTAR = 192519
local GO_DRAKKARI_COLOSSUS_ALTAR = 192520

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 0

local DATA_STATUE_ACTIVATE = 15 -- gundrak.h DATA_STATUE_ACTIVATE

local function onGundrakAltarGossip(_, _, go)
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    -- C++ instance->SetData(DATA_STATUE_ACTIVATE, me->GetEntry())
    -- (instance_gundrak.cpp:371).
    local mapObj = go:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data:SetData(DATA_STATUE_ACTIVATE, go:GetEntry())
    end
    return true
end

RegisterGameObjectGossipEvent(GO_SLAD_RAN_ALTAR, 1, onGundrakAltarGossip)
RegisterGameObjectGossipEvent(GO_MOORABI_ALTAR, 1, onGundrakAltarGossip)
RegisterGameObjectGossipEvent(GO_DRAKKARI_COLOSSUS_ALTAR, 1, onGundrakAltarGossip)
