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
-- Downstream legs ride this handler: the activation spell's instance
-- effect (instance_violet_hold.cpp:928-936 ProcessEvent
-- EVENT_ACTIVATE_CRYSTAL) has no instance-script bridge — the spell's
-- SEND_EVENT effect has no C++ raiser in-tree and no Go twin — so
-- both ProcessEvent legs ride the gossip instead (the gossip IS the
-- crystal activation): the defense-system summon (tick 487,
-- go:SummonCreature at the DefenseSystemLocation below, the
-- instance->SummonCreature twin, MANUAL_DESPAWN default) and the
-- Defenseless=false clear (tick 417). The door-integrity clear
-- (Destroy Door Seal 58040 AuraScript) and the wipe re-arm (instance
-- :893) have no bridge and stay out — see instance_violet_hold.lua.
-- GO_INTRO_ACTIVATION_CRYSTAL 193615 is NOT registered: no C++ consumer
-- references it (header enum only), so its ScriptName binding is
-- unverifiable from the C++ text.

local GO_ACTIVATION_CRYSTAL = 193611 -- violet_hold.h:135
local SPELL_CRYSTAL_ACTIVATION = 57804 -- violet_hold.cpp:1352
local NPC_DEFENSE_SYSTEM = 30837 -- violet_hold.h:121
-- DefenseSystemLocation (instance_violet_hold.cpp:40, sniff).
local DEFENSE_SYSTEM_X = 1888.146
local DEFENSE_SYSTEM_Y = 803.382
local DEFENSE_SYSTEM_Z = 58.60389
local DEFENSE_SYSTEM_O = 3.071779

local function activationCrystalGossip(_, player, go)
    player:CastSpell(player, SPELL_CRYSTAL_ACTIVATION, true)
    -- ProcessEvent summon leg (tick 487): instance->SummonCreature
    -- (NPC_DEFENSE_SYSTEM, DefenseSystemLocation); the GO twin
    -- summons at the same spot. The system despawns itself after 7s
    -- (npc_violet_hold_defense_system.lua).
    if go then
        go:SummonCreature(NPC_DEFENSE_SYSTEM, DEFENSE_SYSTEM_X, DEFENSE_SYSTEM_Y, DEFENSE_SYSTEM_Z, DEFENSE_SYSTEM_O)
    end
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
