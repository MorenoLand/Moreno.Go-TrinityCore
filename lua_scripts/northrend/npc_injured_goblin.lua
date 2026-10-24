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
-- Unmodeled (documented-only, no bridges):
-- - Gossip-select escort start (CloseGossipMenuFor + SetFaction +
--   EscortAI::Start) and the escort legs (GroupEventHappens credit,
--   FailQuest, SAY_END_WP_REACHED 1): the EscortAI machine has no
--   bridge — the muglash / emily precedent (Talk-only quest arm
--   ports, escort legs documented-only).

local ENTRY_INJURED_GOBLIN = 29434

local SAY_QUEST_ACCEPT = 0

local QUEST_BITTER_DEPARTURE = 12832

-- C++ OnQuestAccept for quest 12832: Talk(0).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_BITTER_DEPARTURE then
        return
    end
    creature:Talk(SAY_QUEST_ACCEPT)
end

RegisterCreatureEvent(ENTRY_INJURED_GOBLIN, 31, onQuestAccept)
