-- Uldaman — Lua port of
-- src/server/scripts/EasternKingdoms/Uldaman/uldaman.cpp
-- (go_keystone_chamber : public GameObjectScript) + the
-- instance-side legs it triggers in instance_uldaman.cpp
-- (ActivateIronaya / Update keystone timer).
-- Entry (verifiable from the C++ sources): GO_KEYSTONE = 124371
-- (uldaman.h:38); GO_IRONAYA_SEAL_DOOR = 124372 (uldaman.h:37);
-- NPC_IRONAYA = 7228 (instance_uldaman.cpp OnCreatureCreate
-- case 7228: // Ironaya).
-- Ported arms (C++-exact):
-- - OnGossipHello (go_keystone_chamberAI, uldaman.cpp:52-71):
--   instance->SetData(DATA_IRONAYA_SEAL, IN_PROGRESS) -> the
--   instance Update starts the 27s ironayaSealDoorTimer
--   (instance_uldaman.cpp:68, :328-342). Returns false in C++,
--   so Lua returns false and the native GO use arms proceed
--   (Eluna gossip polarity, gameobject_hooks.go).
-- - 27s chain (instance_uldaman.cpp:288-326 ActivateIronaya +
--   Update :328-342): ironaya SetFaction(415 FACTION_TITAN) +
--   SetControlled(false, ROOT) + RemoveFlag NOT_SELECTABLE +
--   MovePoint(0, IronayaPoint -231.228, 246.6135, -49.01617) +
--   SetHomePosition(same) + Talk(SAY_AGGRO = 0); the ironaya
--   seal door (124372) opens via SetGoState(0); the keystone is
--   blocked via GO SetFlag(GAMEOBJECT_FLAGS = 9, 0x00000004
--   GO_FLAG_INTERACT_COND). Repeat clicks while the 27s timer
--   runs do not restart it (C++ only sets keystoneCheck = true;
--   the timer keeps its remaining time); after the chain fires,
--   further clicks are ignored (the C++ DONE leg blocks the GO).
-- Fidelity deltas: the Go GO SetFlag twin ORs the bits (mirrors
-- GameObject::SetFlag ApplyModFlag) while C++ BlockGO does a
-- direct SetUInt32Value(GAMEOBJECT_FLAGS, 0x4) — identical when
-- the spawn flags are 0, as the DB spawn ships them. No
-- persistence leg: SetData(DATA_IRONAYA_DOOR, DONE) save has no
-- Lua equivalent (instance-script model blocked, standing).
-- Unmodeled: nothing else in uldaman.cpp — the file holds only
-- go_keystone_chamber.

local GO_KEYSTONE = 124371
local GO_IRONAYA_SEAL_DOOR = 124372

local NPC_IRONAYA = 7228

local FACTION_TITAN = 415

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_INTERACT_COND = 0x00000004

local SAY_AGGRO = 0

local IRONAYA_POINT = { -231.228, 246.6135, -49.01617, 0.0 }

local SEARCH_RANGE = 100.0
local SEAL_DOOR_DELAY = 27000

local keystoneState = {}

local function activateIronaya(player, go, guid)
    local ironaya = nil
    if player then
        ironaya = player:FindNearestCreature(NPC_IRONAYA, SEARCH_RANGE)
    end
    if not ironaya and go then
        ironaya = go:GetNearestCreature(SEARCH_RANGE, NPC_IRONAYA)
    end
    if ironaya then
        ironaya:SetFaction(FACTION_TITAN)
        ironaya:SetRooted(false)
        ironaya:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        ironaya:MoveTo(0, IRONAYA_POINT[1], IRONAYA_POINT[2], IRONAYA_POINT[3])
        ironaya:SetHomePosition(IRONAYA_POINT[1], IRONAYA_POINT[2], IRONAYA_POINT[3], IRONAYA_POINT[4])
        ironaya:Talk(SAY_AGGRO)
    end
    local door = nil
    if go then
        door = go:GetNearestGameObject(SEARCH_RANGE, GO_IRONAYA_SEAL_DOOR)
    end
    if not door and ironaya then
        door = ironaya:GetNearestGameObject(SEARCH_RANGE, GO_IRONAYA_SEAL_DOOR)
    end
    if door then
        door:SetGoState(0)
    end
    if go then
        go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_INTERACT_COND)
    end
    keystoneState[guid] = true
end

local function onKeystoneGossip(event, player, go)
    if not go then
        return false
    end
    local guid = go:GetGUIDLow()
    if keystoneState[guid] then
        return false
    end
    keystoneState[guid] = true
    CreateLuaEvent(function()
        activateIronaya(player, go, guid)
    end, SEAL_DOOR_DELAY)
    return false
end

RegisterGameObjectGossipEvent(GO_KEYSTONE, 1, onKeystoneGossip)
