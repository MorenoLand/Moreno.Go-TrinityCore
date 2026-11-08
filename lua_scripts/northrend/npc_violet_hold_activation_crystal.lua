-- Violet Hold activation crystal (Northrend) — Lua port of
-- go_activation_crystalAI in src/server/scripts/Northrend/VioletHold/
-- violet_hold.cpp:1348-1365 (AddSC_violet_hold).
-- Entry: GO_ACTIVATION_CRYSTAL = 193611 (violet_hold.h:135); the
-- instance tracks 5 of them (instance_violet_hold.cpp:948).
-- OnGossipHello PORTED: the player self-casts SPELL_CRYSTAL_ACTIVATION
-- 57804, triggered (violet_hold.cpp:1352-1356). The bridge is the GO
-- gossip-hello hook: RegisterGameObjectGossipEvent(entry, 1, fn) fires
-- from the head of GameObject::Use (gameobject_hooks.go, Eluna
-- GOSSIP_EVENT_ON_HELLO polarity — arlokk gong precedent).
-- Polarity is C++-exact: C++ returns false, so GameObject::Use
-- continues into the native type arms (GameObject.cpp:1505-1506);
-- Lua false likewise lets the native use arms proceed
-- (gameobject_hooks.go:167). Player CastSpell is the bridged player
-- twin (lua_player.go:494); the triggered flag rides the session
-- cast path like every other Go cast.
-- Downstream leg stays out: the activation spell's instance effect
-- (instance_violet_hold.cpp:932 ProcessEvent EVENT_ACTIVATE_CRYSTAL
-- summoning the defense system) has no instance-script bridge, so the
-- cast fires but summons nothing — the same latent class as the
-- defense system itself. The Defenseless=false leg of that ProcessEvent
-- DOES ride this handler (tick 417): the gossip IS the crystal
-- activation, so instanceData.defenseless = 0 is written here; absent
-- reads as the C++ ctor default true. GO_INTRO_ACTIVATION_CRYSTAL 193615 is NOT
-- registered: no C++ consumer references it (header enum only), so
-- its ScriptName binding is unverifiable from the C++ text.

local GO_ACTIVATION_CRYSTAL = 193611 -- violet_hold.h:135
local SPELL_CRYSTAL_ACTIVATION = 57804 -- violet_hold.cpp:1352

local function activationCrystalGossip(_, player, go)
    player:CastSpell(player, SPELL_CRYSTAL_ACTIVATION, true)
    -- Achievement write leg (tick 417): C++ ProcessEvent
    -- EVENT_ACTIVATE_CRYSTAL (instance_violet_hold.cpp:928-936) clears
    -- the instance Defenseless latch; the gossip is the activation, so
    -- the clear rides this handler. The door-integrity clear (Destroy
    -- Door Seal 58040 AuraScript) and the wipe re-arm (instance :893)
    -- have no bridge and stay out — see instance_violet_hold.lua.
    local mapObj = go and go:GetMap() or nil
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.defenseless = 0
        end
    end
    return false
end

RegisterGameObjectGossipEvent(GO_ACTIVATION_CRYSTAL, 1, activationCrystalGossip)
