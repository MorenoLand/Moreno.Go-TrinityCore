-- The Nexus instance script — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/instance_nexus.cpp
-- (class instance_nexus : public InstanceMapScript, map 576) and
-- nexus.h (NexusScriptName "instance_nexus", DataHeader "NEX",
-- EncounterCount 5, NEXDataTypes / NEXCreatureIds / NEXGameObjectIds).
-- RegisterMapEvent binds the map-level Eluna instance hook (the
-- instance_ulduar.lua / instance_gundrak.lua precedent).
-- PORTED tick 511 — the tick-18575 "DOCUMENTED-ONLY, no instance
-- event surface" closure is stale: the instance surface exists now
-- (RegisterMapEvent; instance_hooks.go: event 4 OnPlayerEnter,
-- event 5 OnCreatureCreate, event 6 OnGameObjectCreate) and the
-- Gundrak / Ahn'kahet / Azjol-Nerub / Drak'Tharon ports ride it.
-- PORTED — OnPlayerEnter (instance_nexus.cpp:41-46): the team latch
-- (_teamInInstance, first entering player's team). C++ uses 0 as
-- both "unset" and TEAM_ALLIANCE and re-derives from the player
-- list while unset (:74-79); the Lua twin stores nil for unset,
-- which is the same intent without the collision.
-- PORTED — OnPlayerEnter static-spawn sphere apply (the ahnkahet
-- precedent): event 6 is dynamic-only in Go, so the DONE-time
-- sphere RemoveFlag legs are applied here too, from the entering
-- player (player:FindNearestGameObject, whole-instance radius;
-- entries are unique per GO). Idempotent — it reads the DONE
-- flags the three boss ev4 handlers write.
-- PORTED — OnCreatureCreate (instance_nexus.cpp:48-72): the
-- Anomalus (26763) + Keristrasza (26723) GUID latches, stored as
-- live creature objects on the shared instanceData table (the
-- instance_gundrak.lua dweller-set precedent) — the GetGuidData
-- dispatch (:191-213) twin; consumers read the latched objects
-- straight off instanceData (keristrasza's gossip handler uses
-- FindNearestCreature as its DATA_KERISTRASZA twin; the chaotic
-- rift's DATA_ANOMALUS read has no bearer, rift entry-blocked).
-- Dynamic-only in Go (instance_hooks.go:553-562 — static template
-- spawns never fire event 5); the handler still latches any
-- dynamic spawn that appears.
-- STAY-OUT — OnCreatureCreate Alliance faction flip (:59-67):
-- creature:SetFaction IS bridged (lua_creature_events.go:6042),
-- but the ServerAllowsTwoSideGroups() config gate has no bridge,
-- so the flip cannot be evaluated; the entries stay as spawned.
-- STAY-OUT — GetCreatureEntry team swap (:73-103): no map-level
-- spawn-entry hook exists in Go; the team latch above is recorded
-- for it, but the Alliance->Horde entry substitution (berserker
-- 26800->26799, ranger 26802->26801, cleric 26805->26803,
-- commander 27949->27947, Stoutbeard 26796->Kolurg 26798) cannot
-- run. The commander bosses are registered under both entries
-- (boss_nexus_commanders.lua, twin_valkyr shared-handler
-- precedent), so whichever entry spawns has its AI.
-- PORTED — OnGameObjectCreate (instance_nexus.cpp:105-127): the
-- three containment-sphere GO latches + the DONE-time RemoveFlag
-- (GO_FLAG_NOT_SELECTABLE) when the owning boss is already DONE.
-- Dynamic-only in Go (instance_hooks.go:488-503); the live path
-- is the SetBossState-override leg in the three boss ev4 handlers
-- (telestra/anomalus/ormorok write instanceData.<boss>Done +
-- resolve the sphere by entry and clear the flag, palehoof
-- precedent) and the OnPlayerEnter apply above.
-- DOCUMENTED-ONLY — SetBossState override (instance_nexus.cpp:129-
-- 189): no boss-state-change event exists in Go; the sphere
-- unlocks ride the DONE writes in the boss files (see above).
-- GetGuidData (:191-213) has no Lua twin beyond the instanceData
-- latches (DATA_ANOMALUS 2 / DATA_KERISTRASZA 4 / sphere data
-- 5-7). SetHeaders / SetBossNumber / doorData / boundaries have
-- no bridges (standing).

local MAP_NEXUS = 576

local ENTRY_ANOMALUS = 26763 -- nexus.h:43
local ENTRY_KERISTRASZA = 26723 -- nexus.h:44

local ENTRY_ALLIANCE_BERSERKER = 26800 -- nexus.h:47
local ENTRY_ALLIANCE_RANGER = 26802 -- nexus.h:48
local ENTRY_ALLIANCE_CLERIC = 26805 -- nexus.h:49
local ENTRY_ALLIANCE_COMMANDER = 27949 -- nexus.h:50
local ENTRY_COMMANDER_STOUTBEARD = 26796 -- nexus.h:51

local GO_ANOMALUS_CONTAINMENT_SPHERE = 188527 -- nexus.h:63
local GO_ORMOROKS_CONTAINMENT_SPHERE = 188528 -- nexus.h:64
local GO_TELESTRAS_CONTAINMENT_SPHERE = 188526 -- nexus.h:65

local GAMEOBJECT_FLAGS = 9 -- arlokk precedent
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- C++ OnGameObjectCreate DONE-time legs (instance_nexus.cpp:105-127),
-- applied to one GO: latch it, and clear NOT_SELECTABLE when the
-- owning boss is already DONE (the boss ev4 handler writes the
-- instanceData flag together with SetBossState(DONE)).
local function applySphereCreateState(instanceData, go)
    local entry = go:GetEntry()
    if entry == GO_ANOMALUS_CONTAINMENT_SPHERE then
        instanceData.anomalusSphere = go
        if instanceData.anomalusDone then
            go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    elseif entry == GO_ORMOROKS_CONTAINMENT_SPHERE then
        instanceData.ormorokSphere = go
        if instanceData.ormorokDone then
            go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    elseif entry == GO_TELESTRAS_CONTAINMENT_SPHERE then
        instanceData.telestraSphere = go
        if instanceData.telestraDone then
            go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    end
end

-- C++ OnPlayerEnter (instance_nexus.cpp:41-46): latch the first
-- entering player's team (player:GetTeam() is Player::GetTeamId:
-- 0 Alliance, 1 Horde — lua_player.go:346). Static-spawn path
-- for the sphere DONE legs (the ahnkahet precedent): event 6
-- never fires for static DB GOs in Go, so apply the DONE-time
-- flag clears from the entering player.
RegisterMapEvent(MAP_NEXUS, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if not instanceData.teamInInstance then
        instanceData.teamInInstance = player:GetTeam()
    end
    local spheres = {
        { GO_ANOMALUS_CONTAINMENT_SPHERE, "anomalusDone" },
        { GO_ORMOROKS_CONTAINMENT_SPHERE, "ormorokDone" },
        { GO_TELESTRAS_CONTAINMENT_SPHERE, "telestraDone" },
    }
    for _, sphere in ipairs(spheres) do
        if instanceData[sphere[2]] then
            local go = player:FindNearestGameObject(sphere[1], GO_SEARCH_DIST)
            if go then
                go:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            end
        end
    end
end)

-- C++ OnCreatureCreate (instance_nexus.cpp:48-72): the Anomalus +
-- Keristrasza latches. The Alliance-commander faction flip
-- (:59-67) stays out: no ServerAllowsTwoSideGroups bridge.
RegisterMapEvent(MAP_NEXUS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == ENTRY_ANOMALUS then
        instanceData.anomalus = creature
    elseif entry == ENTRY_KERISTRASZA then
        instanceData.keristrasza = creature
    elseif entry == ENTRY_ALLIANCE_BERSERKER
            or entry == ENTRY_ALLIANCE_RANGER
            or entry == ENTRY_ALLIANCE_CLERIC
            or entry == ENTRY_ALLIANCE_COMMANDER
            or entry == ENTRY_COMMANDER_STOUTBEARD then
        -- Faction flip stays out (see header): the config gate has
        -- no bridge, so the entry is recorded and left as spawned.
        return
    end
end)

-- C++ OnGameObjectCreate (instance_nexus.cpp:105-127): the
-- containment-sphere GO latches + DONE-time flag clears.
RegisterMapEvent(MAP_NEXUS, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE, function(event, instanceData, mapObj, go)
    applySphereCreateState(instanceData, go)
end)
