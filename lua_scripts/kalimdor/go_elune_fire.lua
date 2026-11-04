-- Elune fire / altar (Winterspring) --
-- Lua port of src/server/scripts/Kalimdor/zone_winterspring.cpp
-- (class go_elune_fire : GameObjectScript("go_elune_fire"),
-- OnGossipHello only).
-- Entries (zone_winterspring.cpp:93-94, in-file enum): GO_ELUNE_ALTAR
-- 177404, GO_ELUNE_FIRE 177417. The C++ script serves both: it
-- branches on entry == GO_ELUNE_ALTAR to pick the altar vs torch
-- leg of the escort poke below.
-- Ported arm (zone_winterspring.cpp:575-592): OnGossipHello ->
-- SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE), unconditional.
-- GO gossip hello fires as (event, player, go) with Eluna polarity
-- (gameobject_hooks.go): C++ returns false, so Lua returns false and
-- the native use arms proceed. GAMEOBJECT_FLAGS field 9,
-- GO_FLAG_NOT_SELECTABLE 0x10 (SharedDefines.h:1641) — the
-- blackfathom fire precedent.
-- Unmodeled: GetClosestCreatureWithEntry(me, NPC_RANSHALLA 10300,
-- 10.0f) + DoContinueEscort(isAltar) on the escort AI — no
-- escort-continuation bridge, and ranshalla's escort machine itself
-- is unported (only the quest-accept arm lives in
-- npc_ranshalla.lua), so the poke has no consumer in the model.

local GO_ELUNE_ALTAR = 177404
local GO_ELUNE_FIRE = 177417

local GAMEOBJECT_FLAGS_FIELD = 9
local GO_FLAG_NOT_SELECTABLE = 16

local function onEluneFireGossip(_, _, go)
    go:SetFlag(GAMEOBJECT_FLAGS_FIELD, GO_FLAG_NOT_SELECTABLE)
    return false
end

RegisterGameObjectGossipEvent(GO_ELUNE_ALTAR, 1, onEluneFireGossip)
RegisterGameObjectGossipEvent(GO_ELUNE_FIRE, 1, onEluneFireGossip)
