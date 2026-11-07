-- Moonglade: Clintar Dreamwalker's Spirit — Lua port of
-- src/server/scripts/Kalimdor/zone_moonglade.cpp
-- (class npc_clintar_spirit : CreatureScript("npc_clintar_spirit")).
-- Entry: 22916 (wowhead npc=22916/clintar-dreamwalkers-spirit,
-- verified 2026-11-07; quest 10965 "No Mere Dream" names
-- "Clintar Dreamwalker's Spirit" as its objective; distinct from
-- Clintar Dreamwalker himself; no NPC_ constant in the C++ file).
-- Eluna creature events: 22 IsSummonedBy, 4 JustDied.
-- Ported arms:
-- - IsSummonedBy (zone_moonglade.cpp:159-176, event 22): latch the
--   first player within 5.0 yd whose quest 10965 status is
--   INCOMPLETE (3). The C++ StartEvent 41-waypoint escort machine
--   stays out (MoveWaypoint / DB rows unported); only the latch
--   is ported, on the hanes GUID-latch precedent
--   (npc_apothecary_hanes.lua).
-- - JustDied (zone_moonglade.cpp:178-189, event 4): if the latched
--   player still has quest 10965 incomplete, FailQuest(10965) and
--   clear the latch (the C++ Reset() call after clearing is a
--   no-op here — Reset only re-initializes a nil PlayerGUID).
-- Bridges: ev22 (lua_creature_events.go:7003), GetQuestStatus
-- (lua_player.go:4107), FailQuest (lua_player.go:4442),
-- GetDistance (object.go:274), GetPlayersInWorld (runtime.go:1159).

local ENTRY_CLINTAR_SPIRIT = 22916

local QUEST_NO_MERE_DREAM = 10965
local QUEST_STATUS_INCOMPLETE = 3

local clintarPlayer = {}

-- C++ IsSummonedBy: first player within 5.0 yd with quest 10965
-- incomplete starts the escort; latch that player's GUID.
local function onSummoned(event, creature, summoner)
    local guid = creature:GetGUID()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if creature:GetDistance(plr) <= 5.0 and plr:GetQuestStatus(QUEST_NO_MERE_DREAM) == QUEST_STATUS_INCOMPLETE then
            clintarPlayer[guid] = plr:GetGUID()
            break
        end
    end
end

-- C++ JustDied: latched player with quest 10965 still incomplete
-- fails the quest; latch cleared.
local function onDied(_, creature)
    local guid = creature:GetGUID()
    local playerGuid = clintarPlayer[guid]
    clintarPlayer[guid] = nil
    if not playerGuid then
        return
    end
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetGUID() == playerGuid then
            if plr:GetQuestStatus(QUEST_NO_MERE_DREAM) == QUEST_STATUS_INCOMPLETE then
                plr:FailQuest(QUEST_NO_MERE_DREAM)
            end
            break
        end
    end
end

RegisterCreatureEvent(ENTRY_CLINTAR_SPIRIT, 22, onSummoned)
RegisterCreatureEvent(ENTRY_CLINTAR_SPIRIT, 4, onDied)
