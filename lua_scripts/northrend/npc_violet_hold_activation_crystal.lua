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
-- defense system itself. GO_INTRO_ACTIVATION_CRYSTAL 193615 is NOT
-- registered: no C++ consumer references it (header enum only), so
-- its ScriptName binding is unverifiable from the C++ text.

local GO_ACTIVATION_CRYSTAL = 193611 -- violet_hold.h:135
local SPELL_CRYSTAL_ACTIVATION = 57804 -- violet_hold.cpp:1352

local function activationCrystalGossip(_, player, _go)
    player:CastSpell(player, SPELL_CRYSTAL_ACTIVATION, true)
    return false
end

RegisterGameObjectGossipEvent(GO_ACTIVATION_CRYSTAL, 1, activationCrystalGossip)
