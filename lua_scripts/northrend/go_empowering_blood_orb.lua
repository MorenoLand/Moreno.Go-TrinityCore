-- go_empowering_blood_orb (Icecrown Citadel) — Lua port of
-- go_empowering_blood_orb
-- (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:1116-1164).
-- OnGossipHello + HandleObjectUse PORTED (tick 504).
-- STAYS OUT: Reset() (:1120-1124, summons the 38463 visual-stalker
-- trigger) — GameObjectEventOnSpawn (2) has no Go fire site, and
-- OnAdd (12) fires only for dynamic spawns (gameobjects.go:1845)
-- while the orb is a static DB spawn, so _triggerGuid is never set
-- in Go; the HandleObjectUse trigger leg below finds the stalker by
-- nearest-creature search instead and no-ops while none exists
-- (nothing summons 38463 in Go — zero hits). SetGoAnimProgress(255)
-- (:1138): NO BRIDGE (zero hits in engine/). SetGUID(DATA_GUID)
-- (:1149-1155, cast 70304 + HandleObjectUse): no GO SetGUID event;
-- its sole caller is the orb controller's SpellHit on 70293, which
-- has no C++ caster anywhere in-tree (const :97 + check :722 only),
-- so the hit can never fire in Go. The 3s Delete (:1145-1147)
-- ports as RemoveFromWorld — the static-spawn twin (razorscale
-- mole-machine precedent; a static DB spawn is hidden in the
-- instance, lua_objects.go:4273).

local GO_EMPOWERING_BLOOD_ORB = 201741 -- icecrown_citadel.h:403
local NPC_ORB_VISUAL_STALKER = 38463 -- icecrown_citadel.h:263
local SPELL_EMPOWERED_BLOOD = 70227 -- icecrown_citadel.h:53
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent
local GO_FLAG_IN_USE = 0x1 -- SharedDefines.h:1637
local GO_STATE_DESTROYED = 2 -- SharedDefines.h:1663

-- C++ HandleObjectUse (icecrown_citadel.cpp:1136-1148): IN_USE flag,
-- anim progress 255 (no bridge, skipped), state DESTROYED, despawn
-- the visual-stalker trigger, Delete() after 3s.
local function handleObjectUse(go)
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE)
    go:SetGoState(GO_STATE_DESTROYED)
    local trigger = go:FindNearestCreature(NPC_ORB_VISUAL_STALKER, 5.0)
    if trigger ~= nil then
        trigger:DespawnOrUnsummon()
    end
    CreateLuaEvent(function()
        go:RemoveFromWorld()
    end, 3000)
end

-- C++ OnGossipHello (icecrown_citadel.cpp:1126-1132): the orb casts
-- Empowered Blood (70227) on the player, then HandleObjectUse.
-- DELTA: the GO object has no CastSpell bridge, so the player
-- self-casts; aura 70227's OnApply self-casts 70232 and is
-- caster-independent (spell_icecrown_citadel.lua). The IN_USE guard
-- is the engine-level re-use gate (SharedDefines.h:1637, "disables
-- interaction while animated"): the Go gossip hook fires before the
-- native admit gates (gameobjects.go:831), so the Lua side carries
-- it, matching C++ where a used orb no longer answers Use.
local function onOrbGossip(event, player, go)
    if go:HasFlag(GAMEOBJECT_FLAGS, GO_FLAG_IN_USE) then
        return true
    end
    player:CastSpell(player, SPELL_EMPOWERED_BLOOD, true)
    handleObjectUse(go)
    return true
end

RegisterGameObjectGossipEvent(GO_EMPOWERING_BLOOD_ORB, 1, onOrbGossip)
