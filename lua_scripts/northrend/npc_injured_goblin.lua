-- Storm Peaks: Injured Goblin Miner — Lua port of
-- src/server/scripts/Northrend/zone_storm_peaks.cpp
-- (class npc_injured_goblin : CreatureScript("npc_injured_goblin")).
-- Entry: 29434 (wowhead npc=29434/injured-goblin-miner page, verified
-- this tick; the slug matches the C++ enum name InjuredGoblinMiner
-- and no NPC_ constant exists in the C++ file — the telestra /
-- aeranas wowhead-only precedent, never guessed).
-- Eluna creature events: 31 OnQuestAccept
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface).
-- Ported arms:
-- - OnQuestAccept (zone_storm_peaks.cpp:102-105, event 31): quest id
--   12832 (QUEST_BITTER_DEPARTURE) -> Talk(SAY_QUEST_ACCEPT 0).
-- - OnGossipSelect (zone_storm_peaks.cpp:91-99, gossip event 2) —
--   PORTED 2026-11-07: listID 0 (GOSSIP_OPTION_ID) -> clear menu +
--   SetFaction(113 FACTION_ESCORTEE_N_NEUTRAL_PASSIVE) +
--   StartEscort(player). The menuId half (GOSSIP_ID 9859) is enforced
--   by the engine's stale-menu gate (gossip.go) — menuId is not
--   forwarded to Lua, and listId-0 gates port directly (grandmother /
--   halls-of-reflection precedent).
-- - JustDied (zone_storm_peaks.cpp:76-80, event 4) — PORTED 2026-11-07:
--   if IsEscorted() then GetEscortPlayer():FailQuest(
--   QUEST_BITTER_DEPARTURE 12832) (muglash precedent this tick).
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached 26/27 (Talk SAY_END_WP_REACHED 1, GroupEventHappens
--   credit): the escort walk rides MoveWaypoint + the ev6 arrival event
--   over DB waypoint_data rows — unported, so the arms never fire in
--   the Go model; the gossip start latches escort state only.

local ENTRY_INJURED_GOBLIN = 29434

local SAY_QUEST_ACCEPT = 0

local QUEST_BITTER_DEPARTURE = 12832

local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

-- C++ OnQuestAccept for quest 12832: Talk(0).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_BITTER_DEPARTURE then
        return
    end
    creature:Talk(SAY_QUEST_ACCEPT)
end

-- C++ OnGossipSelect (zone_storm_peaks.cpp:91-99): menu 9859, option 0
-- -> CloseGossipMenuFor + SetFaction(113) + Start(true, true, guid).
local function onGossipSelect(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    creature:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_PASSIVE)
    creature:StartEscort(player)
    return false
end

-- C++ JustDied (zone_storm_peaks.cpp:76-80): if escorting, the escort
-- player fails QUEST_BITTER_DEPARTURE.
local function onDied(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player then
        player:FailQuest(QUEST_BITTER_DEPARTURE)
    end
end

RegisterCreatureEvent(ENTRY_INJURED_GOBLIN, 31, onQuestAccept)
RegisterCreatureGossipEvent(ENTRY_INJURED_GOBLIN, 2, onGossipSelect)
RegisterCreatureEvent(ENTRY_INJURED_GOBLIN, 4, onDied)
