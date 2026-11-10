-- Mimiron hard-mode button — Lua port of go_mimiron_hardmode_buttonAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1675-1702).
-- Entry (ulduar.h:295): GO_MIMIRON_BUTTON = 194739.
-- C++ OnGossipHello: if the button has GO_FLAG_NOT_SELECTABLE, return
-- true (no-op); else the computer gets DoAction(DO_ACTIVATE_COMPUTER),
-- the button goes GO_STATE_ACTIVE and gains GO_FLAG_NOT_SELECTABLE.
-- PORTED via RegisterGameObjectGossipEvent ev1 (the
-- go_empowering_blood_orb.lua precedent; the hello hook fires before the
-- native admit gates, so the Lua side carries the flag guard).
-- DELTA: the GO object surface has no GetInstanceCreature bridge
-- (lua_creature_events.go:6680 is creature-only), so the computer is
-- found by FindNearestCreature (the orb file's GO-side twin) — the
-- computer is the only 34143 in the instance, and the button sits in
-- the Mimiron room with it.

local GO_MIMIRON_BUTTON = 194739 -- ulduar.h:295
local NPC_MIMIRON_COMPUTER = 34143 -- ulduar.h:133

local GAMEOBJECT_FLAGS = 9 -- go_empowering_blood_orb.lua precedent
local GO_FLAG_NOT_SELECTABLE = 0x10 -- SharedDefines.h:1641
local GO_STATE_ACTIVE = 0 -- GameObjectMethods.h:90

local DO_ACTIVATE_COMPUTER = 14 -- boss_mimiron.cpp:301

-- C++ OnGossipHello (boss_mimiron.cpp:1682-1692).
local function onButtonGossip(event, player, go)
    if go:HasFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE) then
        return true
    end
    local computer = go:FindNearestCreature(NPC_MIMIRON_COMPUTER, 100.0)
    if computer ~= nil then
        computer:DoAction(DO_ACTIVATE_COMPUTER)
    end
    go:SetGoState(GO_STATE_ACTIVE)
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    return true
end

RegisterGameObjectGossipEvent(GO_MIMIRON_BUTTON, 1, onButtonGossip)
