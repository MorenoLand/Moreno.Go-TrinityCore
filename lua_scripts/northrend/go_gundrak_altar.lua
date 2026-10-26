-- Gundrak altars (Northrend) — Lua port of go_gundrak_altarAI in
-- src/server/scripts/Northrend/Gundrak/instance_gundrak.cpp:352-375.
-- Entries: GO_SLAD_RAN_ALTAR = 192518 / GO_MOORABI_ALTAR = 192519 /
-- GO_DRAKKARI_COLOSSUS_ALTAR = 192520 (gundrak.h:68-70).
-- OnGossipHello PORTED: SetFlag(GAMEOBJECT_FLAGS, NOT_SELECTABLE) +
-- SetGoState(ACTIVE), unconditional as in C++ (no gates, player unused).
-- Polarity is C++-exact: C++ returns true, so Lua returns true and the
-- native use arms are suppressed (chromaggus-lever precedent).
-- Downstream leg stays out: instance->SetData(DATA_STATUE_ACTIVATE 15,
-- entry) schedules the statue beam/bridge machine in the unported
-- instance script (no instance-data bridge); the altar's own flag and
-- state legs land because the gossip use itself materializes live GO
-- state (keristrasza/taldaram precedent).

local GO_SLAD_RAN_ALTAR = 192518
local GO_MOORABI_ALTAR = 192519
local GO_DRAKKARI_COLOSSUS_ALTAR = 192520

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 0

local function onGundrakAltarGossip(_, _, go)
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    return true
end

RegisterGameObjectGossipEvent(GO_SLAD_RAN_ALTAR, 1, onGundrakAltarGossip)
RegisterGameObjectGossipEvent(GO_MOORABI_ALTAR, 1, onGundrakAltarGossip)
RegisterGameObjectGossipEvent(GO_DRAKKARI_COLOSSUS_ALTAR, 1, onGundrakAltarGossip)
