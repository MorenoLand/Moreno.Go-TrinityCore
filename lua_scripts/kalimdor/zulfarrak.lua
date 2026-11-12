-- Zul'Farrak zone scripts — Lua port of the GameObjectScript /
-- AreaTriggerScript classes in
-- src/server/scripts/Kalimdor/ZulFarrak/zulfarrak.cpp
-- (AddSC_zulfarrak(); loader decl 466-470).
--
-- TDB grounding (TDB335.25101, era-correct for the fork base):
--   go_shallow_grave: gameobject_template entries 128308 + 128403
--   ('Shallow Grave', ScriptName 'go_shallow_grave'), 40 spawns in
--   map 209. The AC "entry 1599 has no spawn in map 209" reading was
--   the wrong entry; 128308/128403 are gameobject_template rows, not
--   SAI rows.
--   at_zumrah: areatrigger_scripts row (962, 'at_zumrah') — trigger 962.
--   go_troll_cage: PORTED in lua_scripts/kalimdor/instance_zulfarrak.lua
--   (59d43ef9): gameobject_template entries 141070-141074 ('Troll Cage',
--   ScriptName 'go_troll_cage', TDB 335.26091) + 5 spawns map 209.
--   The stay-out note above was wrong — the TDB rows exist.
--
-- PORTED ARMS (C++ line refs):
--   go_shallow_grave (zulfarrak.cpp:392-434): OnGossipHello.
--   at_zumrah (zulfarrak.cpp:436-460): OnTrigger.

local NPC_ZOMBIE = 7286 -- zulfarrak.cpp:397 (ShallowGrave)
local NPC_DEAD_HERO = 7276 -- zulfarrak.cpp:398
local CHANCE_ZOMBIE = 65 -- zulfarrak.cpp:399
local CHANCE_DEAD_HERO = 10 -- zulfarrak.cpp:400

local GO_SHALLOW_GRAVE_1 = 128308 -- TDB335.25101 gameobject_template
local GO_SHALLOW_GRAVE_2 = 128403 -- TDB335.25101 gameobject_template

-- TEMPSUMMON_CORPSE_TIMED_DESPAWN is 6 in the C++ enum
-- (ObjectDefines.h:64); the Lua SummonCreature scheme is 0-based
-- (the arlokk precedent: Go 3 == TIMED_DESPAWN_OUT_OF_COMBAT,
-- C++ 4), so the Go value is 5. Despawn timer 30s == 30000ms.
local SUMMON_CORPSE_TIMED_DESPAWN = 5
local SUMMON_DESPAWN_MS = 30000

local function onShallowGraveGossip(_, _, go)
    -- C++ OnGossipHello (zulfarrak.cpp:413-430): randomly summon a
    -- zombie or dead hero the first time a grave is used.
    if go:GetUseCount() == 0 then
        local roll = math.random(0, 100)
        if roll < CHANCE_ZOMBIE then
            go:SummonCreature(NPC_ZOMBIE, go:GetX(), go:GetY(), go:GetZ(), 0, SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        elseif (roll - CHANCE_ZOMBIE) < CHANCE_DEAD_HERO then
            go:SummonCreature(NPC_DEAD_HERO, go:GetX(), go:GetY(), go:GetZ(), 0, SUMMON_CORPSE_TIMED_DESPAWN, SUMMON_DESPAWN_MS)
        end
    end
    go:AddUse()
    -- C++ returns false (default gossip proceeds); the Lua hello arm
    -- returns false likewise.
    return false
end

RegisterGameObjectGossipEvent(GO_SHALLOW_GRAVE_1, 1, onShallowGraveGossip)
RegisterGameObjectGossipEvent(GO_SHALLOW_GRAVE_2, 1, onShallowGraveGossip)

local NPC_ZUMRAH = 7271 -- zulfarrak.cpp:441 (zumrahConsts ZUMRAH_ID)
local TRIGGER_AT_ZUMRAH = 962 -- TDB335.25101 areatrigger_scripts
local FACTION_TROLL_FROSTMANE = 37 -- SharedDefines.h:232

local function onZumrahTrigger(_, player, triggerId)
    -- C++ OnTrigger (zulfarrak.cpp:449-458): the nearest Zumrah
    -- within 30y turns hostile (faction 37); no Zumrah nearby and
    -- the trigger is not consumed.
    if triggerId ~= TRIGGER_AT_ZUMRAH then
        return false
    end
    local zumrah = player:FindNearestCreature(NPC_ZUMRAH, 30.0)
    if zumrah == nil then
        return false
    end
    zumrah:SetFaction(FACTION_TROLL_FROSTMANE)
    return true
end

RegisterServerEvent(24, onZumrahTrigger)
