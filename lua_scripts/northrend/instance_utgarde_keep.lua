-- The Utgarde Keep instance script — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/instance_utgarde_keep.cpp
-- (class instance_utgarde_keep : public InstanceMapScript, map 574).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_naxxramas.lua / instance_halls_of_lightning.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 574
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- keleseth / skarvald / dalronn / ingvar — the creature objects bound
-- by ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches,
-- instance_utgarde_keep.cpp:44-68).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the four binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - GetGuidData (instance_utgarde_keep.cpp:120-139): the Lua mirror is
--   the instanceData binds above, read as instanceData.<boss> by the
--   boss files (oculus precedent); the C++ consumers ride unported
--   machines — the frost-tomb JustDied cross-creature SetData
--   (boss_keleseth.cpp:101-105), the skarvald/dalronn otherBoss lookup
--   (ported via the per-pull death counter + FindNearestCreature in
--   boss_skarvald_dalronn.lua, not via GUIDs), the Annhylde resurrect
--   machine (boss_ingvar_the_plunderer.cpp:302-316) — so the GUID legs
--   as consumed stay out there.
-- Unmodeled (documented-only, no bridges):
-- - Door data (GO_GIANT_PORTCULLIS_1 186756 / GO_GIANT_PORTCULLIS_2
--   186694, DOOR_TYPE_PASSAGE tied to DATA_INGVAR,
--   instance_utgarde_keep.cpp:24-30): no boss-state door machine in Go
--   (oculus/naxxramas precedent).
-- - Minion data (NPC_SKARVALD / NPC_DALRONN -> DATA_SKARVALD_DALRONN,
--   instance_utgarde_keep.cpp:32-37): Go has no minion registry; the
--   DONE minion guard (InstanceScript.cpp:362-366) is reproduced in
--   boss_skarvald_dalronn.lua by the per-pull death counter — DONE lands
--   only on the second death of the pull (the assembly-of-iron precedent).
--   OnCreatureRemove AddMinion(creature, false) (:76-86) is a no-op here.
-- - OnGameObjectCreate forge GO latches (instance_utgarde_keep.cpp:88-118):
--   static template GOs never pass the Go GO-create funnel
--   (instance_hooks.go:291-306, oculus precedent), so the latches are
--   vacuous here.
-- - SetData DATA_FORGE_1/2/3 (instance_utgarde_keep.cpp:141-160):
--   the only consumer is HandleGameObject on the forge anvil/bellow/fire
--   GOs above — no GO-state bridge — so the forge-master SetData legs
--   (utgarde_keep.cpp:47-80) stay out in utgarde_keep.lua; the forge
--   Event persistence (WriteSaveDataMore / ReadSaveDataMore, :162-172)
--   rides the same unmodeled store.
-- - SetHeaders(DataHeader "UK") / SetBossNumber(EncounterCount 3) /
--   LoadDoorData / LoadMinionData: no bridges; the boss-state store is
--   written via creature:SetBossState (lua_creature_events.go:5908) and
--   read via creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).
-- No ON_UPDATE poll: unlike the Loken globe in
-- instance_halls_of_lightning.lua, this instance script has no
-- SetBossState override and no DONE-gated leg of its own — every boss
-- state is written by the boss files and consumed only by the unported
-- machines above — so there is nothing for a poll to drive.

local MAP_UTGARDE_KEEP = 574
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_PRINCE_KELESETH = 23953 -- utgarde_keep.h:46
local NPC_SKARVALD = 24200 -- utgarde_keep.h:47
local NPC_DALRONN = 24201 -- utgarde_keep.h:48
local NPC_INGVAR = 23954 -- utgarde_keep.h:49

local bossBinds = {
    [NPC_PRINCE_KELESETH] = "keleseth",
    [NPC_SKARVALD] = "skarvald",
    [NPC_DALRONN] = "dalronn",
    [NPC_INGVAR] = "ingvar",
}

RegisterMapEvent(MAP_UTGARDE_KEEP, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
