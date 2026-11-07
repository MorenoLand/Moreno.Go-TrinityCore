-- Battle for Mount Hyjal instance script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/instance_hyjal.cpp
-- (class instance_hyjal : public InstanceMapScript, map 534 —
-- instance_hyjal.cpp:56; AddSC_instance_mount_hyjal :325; DATA ids in
-- hyjal.h:30-50; entries/GOs in hyjal.h:73-91).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_ruby_sanctum.lua / instance_icecrown_citadel.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 534
-- (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- rageWinterchill / anetheron / kazrogal / azgalor / archimonde /
-- jainaProudmoore / thrall / tyrandeWhisperwind — the creature objects
-- bound by ON_CREATURE_CREATE (the C++ OnCreatureCreate GUID latches,
-- instance_hyjal.cpp:118-156); channelTarget — the NPC_CHANNEL_TARGET
-- object bound by the C++ creatureData table (:40-44, DATA_CHANNEL_TARGET
-- 21: the derived OnCreatureCreate calls the base
-- InstanceScript::OnCreatureCreate at :154, whose AddObject leg stores
-- the GUID under DATA_CHANNEL_TARGET, and boss_archimonde.cpp:385 reads
-- it back via instance->GetCreature(DATA_CHANNEL_TARGET) ->
-- GetObjectGuid (InstanceScript.cpp:119-121), NOT the GetGuidData
-- override, which does not handle DATA_CHANNEL_TARGET); hordeGate /
-- elfGate — the GO objects bound by ON_GAMEOBJECT_CREATE (:96-116);
-- ancientGems — the GO list appended by the same handler (:113-115,
-- C++ m_uiAncientGemGUID push_back order).
-- trash / raidDamage — the DATA_TRASH (14) / DATA_RAIDDAMAGE (18)
-- counters, written by the hyjal_trash.lua ev9/ev4 arms (see the
-- SetData note below) and read by the wave machine when it lands.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   / ruby-sanctum delta).
-- - OnCreatureCreate ARCHIMONDE leg (:140-147): while Azgalor is not
--   DONE, Archimonde is hidden + REACT_PASSIVE (creature:SetVisible,
--   lua_creature_events.go:5092; creature:SetReactState,
--   lua_creature_events.go:3889, REACT_PASSIVE 0). The C++ gate reads
--   the encounter array via GetData(DATA_AZGALOREVENT 6); the Go
--   mirror reads creature:GetBossState(DATA_AZGALOR 5)
--   (lua_creature_events.go:5936) — the boss-state store keyed by the
--   DATA_* boss id, the ruby-sanctum convention. DEPENDENCY (unresolved):
--   boss_azgalor.lua now writes SetBossState(DATA_AZGALOR, DONE) on death
--   (tick 272), so the gate reads DONE on post-Azgalor re-entry and
--   Archimonde stays visible; on a fresh run it reads NOT_STARTED and
--   Archimonde hides — correct in both cases, with no edit here.
-- - OnGameObjectCreate: the GO binds above + the ancient-gem list append.
--   Static template GOs never pass the GO-create funnel
--   (instance_hooks.go:291-306, oculus precedent), so the gate binds are
--   vacuous for static spawns and live for dynamically-spawned GOs.
-- - GetGuidData (:158-176): no function port — the Lua mirror is the
--   instanceData binds above, read as instanceData.<key>; the boss files
--   read the same table via Map:GetInstanceData (the oculus precedent).
--   DATA_CHANNEL_TARGET is instanceData.channelTarget.
-- Unmodeled (documented-only, no bridges):
-- - SetData override (:178-260): no instance SetData bridge —
--   creature:SetData is a generic per-motion ScriptData store, NOT
--   virtual AI dispatch and NOT the instance encounter array
--   (lua_creature_events.go:767-795; the ruby-sanctum header settles
--   the same point). No fire site exists for any SetData arm, so none
--   can be re-expressed here and no ON_UPDATE poll is added (the
--   instance_icecrown_citadel.lua pattern):
--   - DATA_*EVENT encounter writes (0-4): the five-boss encounter array
--     is still never written via instance SetData in Go (no bridge); the
--     boss files now write the same states via creature:SetBossState on
--     the DATA_* boss ids instead (tick 272 — rage 10, anetheron 1,
--     kazrogal 8, azgalor 5, archimonde 3), which is what the
--     GetBossState reads in this file mirror.
--   - DATA_AZGALOREVENT DONE leg (:197-215): instance->LoadGrid has no
--     bridge; the Archimonde reveal (SetVisible(true) +
--     SetReactState(REACT_AGGRESSIVE)) is the inverse of the ported
--     create-time hide leg and has no DONE-time fire site; the
--     Talk(YELL_ARCHIMONDE_INTRO 8) rides the same leg. Covered by the
--     DEPENDENCY note above once a fire site exists.
--   - DATA_TRASH / DoUpdateWorldState(WORLD_STATE_ENEMYCOUNT 2454,
--     :227-233): PORTED in hyjal_trash.lua — the ev4 JustDied arm
--     writes instanceData.trash + creature:UpdateWorldState(2454,
--     trash), the SetData twin (no instance SetData hook in Go; the
--     shared table is the writer+reader). The Trash INCREMENT side
--     (SummonNextWave SetData(DATA_TRASH, EnemyCount)) stays with
--     the unported wave machine, as do the DATA_RESET_TRASH_COUNT /
--     DATA_RESET_RAIDDAMAGE legs (hyjalAI.cpp:417/:907).
--   - TYPE_RETREAT (:234-246): DoRespawnGameObject(*itr, 24h) has no
--     bridge (tree-wide grep: no DoRespawnGameObject in the Go engine);
--     the ancientGems list above is the live half, for a future consumer.
--   - DATA_ALLIANCE_RETREAT / DATA_HORDE_RETREAT (:247-256):
--     HandleGameObject(HordeGate/ElfGate, true) has no bridge
--     (tree-wide grep: no HandleGameObject in the Go engine); the
--     retreat-gate HandleGameObject legs in OnGameObjectCreate (:99-109)
--     stay out for the same reason. SaveToDB has no bridge either
--     (SaveInstanceData no-ops, engine/scripting/globals.go:1434-1440).
--   - DATA_RAIDDAMAGE accumulator (:257-260): PORTED in
--     hyjal_trash.lua — the ev9 DamageTaken arm writes
--     instanceData.raidDamage (clamped at MINRAIDDAMAGE).
--     DATA_RESET_TRASH_COUNT / DATA_RESET_RAIDDAMAGE: no fire site
--     (hyjalAI.cpp:417/:907, wave machine).
-- - GetData (:262-280): no instance GetData bridge; the Lua mirror is
--   instanceData table fields (trash/raidDamage now have writers in
--   hyjal_trash.lua; retreat counters still unwritten, see above)
--   and GetBossState for the encounter states.
-- - Load (:291-306) / GetSaveData (:282-285): ON_LOAD (2) has no Go
--   instance-data load path (engine/scripting/runtime.go:171-178); the
--   table lives for the runtime lifetime only (black-morass delta).
--   IN_PROGRESS encounters are therefore never reset to NOT_STARTED on
--   load — no encounter is ever IN_PROGRESS in Go (see SetData note).
-- - IsEncounterInProgress (:76-84): event 7 exists
--   (INSTANCE_EVENT_ON_CHECK_ENCOUNTER_IN_PROGRESS,
--   instance_hooks.go:334) but the encounter array it reads is never
--   written in Go, so a handler would always return false; the native
--   instanceEncounters tracking already covers the CanEnter gate.
-- - SetHeaders(DataHeader "HY") / EncounterCount 5: no bridges.
-- - No SetBossState override exists in this C++ file; boss state is
--   written by boss files via creature:SetBossState
--   (lua_creature_events.go:5917) — see the DEPENDENCY note above.
-- No ON_UPDATE poll: see the SetData note above.

local MAP_MOUNT_HYJAL = 534
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- The DATA_* boss id behind the Azgalor gate below. The other DATA ids
-- (hyjal.h:30-50) are mirrored by the instanceData keys, not by constants
-- here — no code in this file reads them.
local DATA_AZGALOR = 5 -- hyjal.h:34

local NPC_RAGE_WINTERCHILL = 17767 -- hyjal.h:78
local NPC_ANETHERON = 17808 -- hyjal.h:79
local NPC_KAZROGAL = 17888 -- hyjal.h:80
local NPC_AZGALOR = 17842 -- hyjal.h:81
local NPC_ARCHIMONDE = 17968 -- hyjal.h:82
local NPC_CHANNEL_TARGET = 22418 -- hyjal.h:84
local NPC_JAINA = 17772 -- hyjal.h:73
local NPC_THRALL = 17852 -- hyjal.h:74
local NPC_TYRANDE = 17948 -- hyjal.h:75

local GO_HORDE_ENCAMPMENT_PORTAL = 182060 -- hyjal.h:89
local GO_NIGHT_ELF_VILLAGE_PORTAL = 182061 -- hyjal.h:90
local GO_ANCIENT_GEM = 185557 -- hyjal.h:91

local REACT_PASSIVE = 0 -- UnitDefines.h (lua_creature_events.go:3889)
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local creatureBinds = {
    [NPC_RAGE_WINTERCHILL] = "rageWinterchill",
    [NPC_ANETHERON] = "anetheron",
    [NPC_KAZROGAL] = "kazrogal",
    [NPC_AZGALOR] = "azgalor",
    [NPC_ARCHIMONDE] = "archimonde",
    [NPC_JAINA] = "jainaProudmoore",
    [NPC_THRALL] = "thrall",
    [NPC_TYRANDE] = "tyrandeWhisperwind",
    [NPC_CHANNEL_TARGET] = "channelTarget",
}

local gameObjectBinds = {
    [GO_HORDE_ENCAMPMENT_PORTAL] = "hordeGate",
    [GO_NIGHT_ELF_VILLAGE_PORTAL] = "elfGate",
}

RegisterMapEvent(MAP_MOUNT_HYJAL, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    local key = creatureBinds[entry]
    if not key then
        return
    end
    instanceData[key] = creature
    -- C++ :140-147: Archimonde materializes hidden + REACT_PASSIVE
    -- while Azgalor is not DONE. The gate reads GetBossState(DATA_AZGALOR)
    -- (the encounter-array mirror — see header); the reveal half on
    -- Azgalor DONE has no fire site (see header).
    if entry == NPC_ARCHIMONDE and creature:GetBossState(DATA_AZGALOR) ~= BOSS_STATE_DONE then
        creature:SetVisible(false)
        creature:SetReactState(REACT_PASSIVE)
    end
end)

RegisterMapEvent(MAP_MOUNT_HYJAL, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    local entry = go:GetEntry()
    -- C++ :113-115: ancient gems accumulate into a list, in create order.
    if entry == GO_ANCIENT_GEM then
        if not instanceData.ancientGems then
            instanceData.ancientGems = {}
        end
        instanceData.ancientGems[#instanceData.ancientGems + 1] = go
        return
    end
    local key = gameObjectBinds[entry]
    if not key then
        return
    end
    instanceData[key] = go
    -- C++ :99-109: the retreat-gated HandleGameObject legs have no
    -- bridge (see header); the bind above is the live part, for future
    -- consumers.
end)
