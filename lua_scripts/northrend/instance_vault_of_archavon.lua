-- The Vault of Archavon instance script — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/instance_vault_of_archavon.cpp
-- (class instance_vault_of_archavon : public InstanceMapScript, map 624).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_utgarde_pinnacle.lua / instance_halls_of_lightning.lua
-- precedent); its presence opens the Map:GetInstanceData gate for map 624
-- (engine/world/instance_hooks.go) that the boss onDied handlers
-- share with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- archavon / emalon / koralon / toravon — the four boss binds (the C++
-- OnCreatureCreate GUID latches via LoadObjectData creatureData,
-- instance_vault_of_archavon.cpp:36-42); archavonDeath / emalonDeath /
-- koralonDeath — death timestamps (seconds, os.time()) written by the
-- three boss onDied handlers (boss_archavon.lua / boss_emalon.lua /
-- boss_koralon.lua) for the Earth, Wind & Fire criteria check below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the four boss binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound (naxxramas
--   LoadGrid delta).
-- - GetGuidData (instance_vault_of_archavon.cpp: no override — the base
--   LoadObjectData store): the Lua mirror is the instanceData binds
--   above, read as instanceData.<boss> by the boss files (oculus
--   precedent); the C++ consumers ride unported cross-creature machines
--   in those boss files — the Emalon minion JustDied / JustEngagedWith
--   legs (boss_emalon.cpp:207-217/:219-229) and the Toravon frozen-orb
--   IsSummonedBy leg (boss_toravon.cpp:111-133) — so the GUID legs as
--   consumed stay out there, re-expressed via GetCreaturesInGrid /
--   FindNearestCreature where the boss files port them.
-- - SetBossState override DONE legs
--   (instance_vault_of_archavon.cpp:57-83): on DONE for DATA_ARCHAVON /
--   DATA_EMALON / DATA_KORALON the C++ records the death time and casts
--   SPELL_EARTH_WIND_FIRE_ACHIEVEMENT_CHECK 68308 on every instance
--   player; on DONE for DATA_TORAVON it default-returns (no leg). The
--   legs are PORTED at DONE time in the three boss onDied handlers
--   (the palehoof precedent — no ON_UPDATE poll: the poll would only
--   re-drive legs the died handlers already run), in C++ order (state
--   first, then the override legs).
-- Unmodeled (documented-only, no bridges):
-- - CheckAchievementCriteriaMeet (instance_vault_of_archavon.cpp:
--   85-110, criteria 12018/12019 Earth, Wind & Fire — all three deaths
--   within 60s of each other): PORTED this tick as the Go type-18
--   dispatch (engine/world/achievement_criteria_scripts.go
--   checkVaultOfArchavonCriteria), reading the death times banked in
--   instanceData.archavonDeath / emalonDeath / koralonDeath by the
--   three boss onDied handlers.
-- - SetHeaders(DataHeader "VA") / SetBossNumber(EncounterCount 4) /
--   LoadObjectData: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5908) and read via
--   creature:GetBossState (lua_creature_events.go:5932).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (black-morass delta).
-- No ON_UPDATE poll: this instance script has no DONE-gated leg of its
-- own — the SetBossState-override DONE legs already run at DONE time
-- in the three boss files — so there is nothing for a poll to drive.
-- No DoorData, MinionData, boss boundaries, or OnGameObjectCreate legs
-- exist in the C++ file.

local MAP_VAULT_OF_ARCHAVON = 624
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_ARCHAVON = 31125 -- vault_of_archavon.h:38
local NPC_EMALON = 33993 -- vault_of_archavon.h:39
local NPC_KORALON = 35013 -- vault_of_archavon.h:40
local NPC_TORAVON = 38433 -- vault_of_archavon.h:41

local bossBinds = {
    [NPC_ARCHAVON] = "archavon",
    [NPC_EMALON] = "emalon",
    [NPC_KORALON] = "koralon",
    [NPC_TORAVON] = "toravon",
}

RegisterMapEvent(MAP_VAULT_OF_ARCHAVON, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)
