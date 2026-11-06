-- Onyxia's Lair instance script — Lua port of the bridgeable arms of
-- src/server/scripts/Kalimdor/OnyxiasLair/instance_onyxias_lair.cpp
-- (class instance_onyxias_lair : public InstanceMapScript
-- (OnyxiaScriptName "instance_onyxias_lair", map 249); AddSC at end
-- registers the one script; kalimdor loader call follows
-- AddSC_instance_maraudon(), closing the "// Onyxia's Lair" loader
-- block). DataHeader "OL", EncounterCount = 1 (onyxias_lair.h).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_mount_hyjal.lua / instance_zulfarrak.lua precedent); its
-- presence opens the Map:GetInstanceData gate for map 249 that the
-- boss file shares with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- onyxia — the creature object bound by ON_CREATURE_CREATE (the C++
-- OnCreatureCreate GUID latch, :67-76); manyWhelpsCounter — the
-- whelp-spawner hatch count (C++ :92, :163, :210; mirror only, the
-- achievement consumer has no bridge, see below).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate (:67-76): NPC_ONYXIA (10184) binds
--   instanceData.onyxia. The C++ GetGuidData override (:192-201)
--   returns the same GUID under NPC_ONYXIA as its data id; the Lua
--   mirror is the bind (hyjal precedent: no GetGuidData function
--   port, boss files read instanceData.<key>). No in-tree C++
--   consumer calls GetGuidData(NPC_ONYXIA) — the latch's only C++
--   consumer is FloorEruption's OriginalCaster (:113), inside the
--   unported eruption machine below.
-- - OnGameObjectCreate (:77-96), whelp-spawner leg only:
--   GO_WHELP_SPAWNER (176510) summons NPC_WHELP (11262) at the GO
--   position, TEMPSUMMON_CORPSE_DESPAWN (Go type 4), with
--   DoZoneInCombat on the summon + manyWhelpsCounter++ (:89-93).
--   Static-spawn delta: the Go GO-create funnel only fires for
--   dynamic/instance spawns (instance_hooks.go, triggerInstance-
--   GameObjectCreate) — a static template whelp spawner never fires
--   this hook, so the leg is live only for a dynamically-spawned
--   spawner GO (same documented delta as the hyjal gate binds).
-- Unmodeled (documented-only, no bridges):
-- - OnGameObjectCreate floor-trap leg (:79-86): the GOInfo
--   displayId (4392/4472) half IS readable (go:GetDisplayId,
--   lua_objects.go:3865) but the trap.spellId == 17731 half has no
--   bridge (no GOInfo/trap accessor in the Go engine), so the
--   FloorEruptionGUID[0] insert cannot be identified exactly.
-- - OnGameObjectRemove (:98-105): no ON_GAMEOBJECT_REMOVE instance
--   event exists (events 1-7 only, runtime.go:172-178); the erase
--   half of the floor tracking has no fire site.
-- - FloorEruption BFS (:107-138) + SetGuidData(DATA_FLOOR_ERUPTION_
--   GUID) (:180-190) + the eruption Update pump (:215-241): need GO
--   enumeration by displayId/trap, GO CastSpell with an OriginalCaster
--   override, and the eruption queue — no bridges. No ON_UPDATE poll
--   is added (the instance_icecrown_citadel.lua pattern).
-- - SetBossState override (:140-153): DEAD NO-OP in C++, not a gap.
--   The base call handles DATA_ONYXIA (0) and the override then
--   re-calls SetBossState(DATA_SHE_DEEP_BREATH_MORE = 1, IN_PROGRESS);
--   EncounterCount = 1, so id 1 >= bosses.size() 1 and
--   InstanceScript::SetBossState returns false without writing
--   (InstanceScript.cpp, SetBossNumber resizes bosses to the count).
--   Do NOT "port" the secondary write — Go's setInstanceBossState
--   takes any boss id (instance_boss_states.go), so writing id 1
--   would fabricate state C++ never has.
-- - SetData (:155-177) / Load / Update liftoff timer (:203-213) /
--   CheckAchievementCriteriaMeet (:244-257): no instance SetData
--   bridge (closed axis, hyjal header settles it) and no achievement
--   bridge; the manyWhelpsCounter mirror above has no consumer.
-- - Boss boundaries (CircleBoundary around DATA_ONYXIA,
--   instance_onyxias_lair.cpp:29-32): no boundary bridge.
-- - The boss-state writes themselves live in the boss file:
--   boss_onyxia.lua writes SetBossState(DATA_ONYXIA, IN_PROGRESS /
--   NOT_STARTED) via creature:SetBossState; there is no JustDied
--   override in C++, so no DONE leg exists to write.

local MAP_ONYXIAS_LAIR = 249
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

local NPC_ONYXIA = 10184 -- onyxias_lair.h:62
local NPC_WHELP = 11262 -- onyxias_lair.h:60
local GO_WHELP_SPAWNER = 176510 -- onyxias_lair.h:66

RegisterMapEvent(MAP_ONYXIAS_LAIR, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_ONYXIA then
        instanceData.onyxia = creature
    end
end)

RegisterMapEvent(MAP_ONYXIAS_LAIR, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    if go:GetEntry() ~= GO_WHELP_SPAWNER then
        return
    end
    -- C++ :87-94: summon one whelp at the spawner GO position,
    -- TEMPSUMMON_CORPSE_DESPAWN (Go type 4); the summon gets
    -- DoZoneInCombat and the hatch counter increments. Only fires
    -- for a dynamically-spawned spawner GO (see header).
    local summoned = go:SummonCreature(NPC_WHELP,
        go:GetX(), go:GetY(), go:GetZ(), go:GetO(), 4, 0)
    if summoned then
        summoned:DoZoneInCombat()
        instanceData.manyWhelpsCounter = (instanceData.manyWhelpsCounter or 0) + 1
    end
end)
