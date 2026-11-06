-- The Pit of Saron instance script — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/instance_pit_of_saron.cpp
-- (class instance_pit_of_saron : public InstanceMapScript, map 658).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_forge_of_souls.lua / instance_violet_hold.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 658
-- (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- garfrost / krick / ick / tyrannus / rimefang / tyrannusEvents —
-- the OnCreatureCreate binds (instance_pit_of_saron.cpp:71-100);
-- jainaOrSylvanas1 — the part-1 leader bind (:97-100, one GUID for
-- either Jaina or Sylvanas part 1, team decided at spawn);
-- cavernTriggers — the NPC_CAVERN_EVENT_TRIGGER 32780 vector
-- (:94-96), a list, not a single GUID.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound
--   (naxxramas LoadGrid delta).
-- - GetGuidData (instance_pit_of_saron.cpp:238-262): the Lua mirror
--   is the instanceData binds above, read as instanceData.garfrost /
--   instanceData.tyrannus / etc. (oculus precedent). Live consumer:
--   boss_forgemaster_garfrost.lua JustDied reads instanceData.tyrannus
--   for the SAY_TYRANNUS_DEATH Talk (the kelthuzad GetInstanceData
--   precedent); the ick outro relay keeps its own 500yd sweep.
-- - SetBossState: the C++ instance script HAS a boss-state override
--   (:150-197) — but its only legs are DONE-spawn arms (below) that
--   ride the unbridged team latch, so there is nothing instance-side
--   to port and NO ON_UPDATE poll (the palehoof / oculus precedent:
--   a poll only drives legs with a live consumer). The
--   IN_PROGRESS / DONE / NOT_STARTED legs run at engage / death /
--   reset time in the boss handlers (boss_forgemaster_garfrost.lua /
--   boss_krickandick.lua / boss_scourgelord_tyrannus.lua).
-- Unmodeled (documented-only, no bridges):
-- - OnPlayerEnter team latch (:62-67) + GetCreatureEntry team swap
--   (:104-148) + GetData(DATA_TEAM_IN_INSTANCE) (:200-214): no
--   instance GetData/SetData bridge from Lua, and the latch's only
--   consumers are the team swap and the DONE-spawn legs below.
-- - SetBossState override DONE-spawn legs (:157-190): DATA_GARFROST
--   DONE summons Martin Victus 1 (37591, Alliance) / Gorkun
--   Ironskull 1 (37592, Horde) at SlaveLeaderPos; DATA_TYRANNUS
--   DONE summons Jaina part 2 (38188, Alliance) / Sylvanas part 2
--   (38189, Horde) at EventLeaderPos2 — both ride the team latch
--   above + instance SummonCreature, and the override has no Lua
--   fire site (creature:SetBossState writes the store only).
-- - DoorData (:33-38: ice-wall passage doors on DATA_GARFROST /
--   DATA_ICK, portcullis on DATA_TYRANNUS): no door model in the Go
--   boss-state store (instance_boss_states.go:72).
-- - GetData(DATA_ICE_SHARDS_HIT / DATA_CAVERN_ACTIVE) + SetData
--   (:200-235, incl. HandleCavernEventTrigger :265-276): no
--   instance GetData/SetData bridge; the cavern icicle machine
--   (ScheduledIcicleSummons BasicEvent) has no bridge either.
-- - _jainaOrSylvanas2GUID is NEVER assigned in the C++ file (only
--   declared :286 and returned :257) — GetGuidData(
--   DATA_JAINA_SYLVANAS_2) always returns Empty; no bind for it.
-- - SetHeaders(DataHeader "POS") / SetBossNumber(EncounterCount 3)
--   / SaveToDB: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5917) and read via
--   creature:GetBossState (lua_creature_events.go:5936).
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_PIT_OF_SARON = 658
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local NPC_GARFROST = 36494 -- pit_of_saron.h:49
local NPC_KRICK = 36477 -- pit_of_saron.h:50
local NPC_ICK = 36476 -- pit_of_saron.h:51
local NPC_TYRANNUS = 36658 -- pit_of_saron.h:52
local NPC_RIMEFANG = 36661 -- pit_of_saron.h:53
local NPC_TYRANNUS_EVENTS = 36794 -- pit_of_saron.h:55
local NPC_SYLVANAS_PART1 = 36990 -- pit_of_saron.h:57
local NPC_JAINA_PART1 = 36993 -- pit_of_saron.h:59
local NPC_CAVERN_EVENT_TRIGGER = 32780 -- pit_of_saron.h:94

local bossBinds = {
    [NPC_GARFROST] = "garfrost",
    [NPC_KRICK] = "krick",
    [NPC_ICK] = "ick",
    [NPC_TYRANNUS] = "tyrannus",
    [NPC_RIMEFANG] = "rimefang",
    [NPC_TYRANNUS_EVENTS] = "tyrannusEvents",
    [NPC_SYLVANAS_PART1] = "jainaOrSylvanas1",
    [NPC_JAINA_PART1] = "jainaOrSylvanas1",
}

RegisterMapEvent(MAP_PIT_OF_SARON, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_CAVERN_EVENT_TRIGGER then
        if instanceData.cavernTriggers == nil then
            instanceData.cavernTriggers = {}
        end
        instanceData.cavernTriggers[#instanceData.cavernTriggers + 1] = creature
        return
    end
    local key = bossBinds[entry]
    if key then
        instanceData[key] = creature
    end
end)
