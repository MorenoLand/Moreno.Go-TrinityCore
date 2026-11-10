-- Main-chambers access panel (The Steamvault) — Lua port of
-- go_main_chambers_access_panel in src/server/scripts/Outland/
-- CoilfangReservoir/SteamVault/instance_steam_vault.cpp.
-- Entries: GO_ACCESS_PANEL_HYDRO = 184125, GO_ACCESS_PANEL_MEK
-- = 184126 (steam_vault.h SVGameObjectIds); both panels share
-- the one C++ GameObjectAI class, so both entries register the
-- same handler below.
-- OnGossipHello PORTED (instance_steam_vault.cpp:36-44):
-- the door controller (20926) Talks CONTROLLER_TEXT_ACESS_USED
-- (0), instance->SetData(ACTION_OPEN_DOOR 1, 0), the panel
-- gains GO_FLAG_NOT_SELECTABLE and goes GO_STATE_ACTIVE,
-- unconditional as in C++ (no gates, player unused). C++
-- returns true, so Lua returns true and the native use arms
-- are suppressed (go_bridge_console.lua precedent).
-- The controller resolves entry-based via the gossiping
-- player's FindNearestCreature (the C++ GetCreature GUID
-- latch has no Lua counterpart); the SetData write rides the
-- live bridge (instance_hooks.go:51-65 fires
-- INSTANCE_EVENT_ON_SET_DATA (8), handled in
-- instance_steam_vault.lua).

local GO_ACCESS_PANEL_HYDRO = 184125
local GO_ACCESS_PANEL_MEK = 184126

local NPC_COILFANG_DOOR_CONTROLLER = 20926
local CONTROLLER_TEXT_ACESS_USED = 0

local ACTION_OPEN_DOOR = 1 -- steam_vault.h SVMisc

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 0

local GO_SEARCH_DIST = 1000 -- whole-instance radius; the controller is unique

local function onAccessPanelGossip(_, player, go)
    if player ~= nil then
        local controller = player:FindNearestCreature(NPC_COILFANG_DOOR_CONTROLLER, GO_SEARCH_DIST)
        if controller then
            controller:Talk(CONTROLLER_TEXT_ACESS_USED)
        end
    end
    local mapObj = go:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data:SetData(ACTION_OPEN_DOOR, 0)
    end
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    return true
end

RegisterGameObjectGossipEvent(GO_ACCESS_PANEL_HYDRO, 1, onAccessPanelGossip)
RegisterGameObjectGossipEvent(GO_ACCESS_PANEL_MEK, 1, onAccessPanelGossip)
