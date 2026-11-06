-- The Halls of Stone instance script — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/instance_halls_of_stone.cpp
-- (class instance_halls_of_stone : public InstanceMapScript, map 599).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua / instance_halls_of_lightning.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 599
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- krystallus / maiden / sjonnir / marnak / kaddrak / abedneum / brann
-- — the creature objects bound by ON_CREATURE_CREATE (the C++
-- OnCreatureCreate GUID latches, instance_halls_of_stone.cpp:44-68).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the seven binds above. Fires on
--   materialization (motionFor funnel, instance_hooks.go:311-320);
--   a static template spawn never materializes server-side and is
--   never bound (naxxramas LoadGrid delta).
-- - GetGuidData (instance_halls_of_stone.cpp:96-132): the Lua
--   mirror is the instanceData binds above, read as
--   instanceData.<boss> by the boss files (oculus precedent); the
--   C++ consumers (Brann's escort machine, the tribunal controller)
--   ride unported machines in halls_of_stone.cpp, so the GUID legs
--   as consumed stay out there.
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_SJONNIR_DOOR 191296, DOOR_TYPE_PASSAGE tied to
--   DATA_TRIBUNAL_OF_AGES, instance_halls_of_stone.cpp:24-30):
--   no boss-state door machine in Go (oculus/naxxramas precedent).
--   GO_MAIDEN_DOOR 191292 / GO_BRANN_DOOR 191295 are not in
--   doorData at all.
-- - OnGameObjectCreate GO latches (instance_halls_of_stone.cpp:70-94):
--   static template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:291-306, oculus precedent), so the latches are
--   vacuous here.
-- - SetBossState DATA_TRIBUNAL_OF_AGES DONE -> tribunal chest
--   RemoveFlag(GO_FLAG_NOT_SELECTABLE) (instance_halls_of_stone.cpp:
--   134-155 + the OnGameObjectCreate DONE-time leg :86-89): unreachable
--   — the only writer of DATA_TRIBUNAL_OF_AGES DONE is Brann's escort
--   step 29 (halls_of_stone.cpp:563-567), and the escort machine is
--   unported, so no ON_UPDATE poll (unlike the Loken globe in
--   instance_halls_of_lightning.lua, nothing ever writes a
--   tribunalDone flag). The chest leg re-activates the moment the
--   escort machine lands.
-- - CheckRequiredBosses DATA_SJONNIR gate (instance_halls_of_stone.cpp:
--   157-174): no required-bosses bridge; Sjonnir's JustEngagedWith
--   gate leg stays out in boss_sjonnir.lua (EnterEvadeMode has no
--   bridge either).
-- - SetHeaders(DataHeader "HOS") / SetBossNumber(EncounterCount 4) /
--   LoadDoorData: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5908) and read via
--   creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).

local MAP_HALLS_OF_STONE = 599
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_KRYSTALLUS = 27977 -- halls_of_stone.h:50
local NPC_MAIDEN = 27975 -- halls_of_stone.h:49
local NPC_SJONNIR = 27978 -- halls_of_stone.h:51
local NPC_MARNAK = 30897 -- halls_of_stone.h:52
local NPC_KADDRAK = 30898 -- halls_of_stone.h:53
local NPC_ABEDNEUM = 30899 -- halls_of_stone.h:54
local NPC_BRANN = 28070 -- halls_of_stone.h:55

local bossBinds = {
    [NPC_KRYSTALLUS] = "krystallus",
    [NPC_MAIDEN] = "maiden",
    [NPC_SJONNIR] = "sjonnir",
    [NPC_MARNAK] = "marnak",
    [NPC_KADDRAK] = "kaddrak",
    [NPC_ABEDNEUM] = "abedneum",
    [NPC_BRANN] = "brann",
}

RegisterMapEvent(MAP_HALLS_OF_STONE, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
