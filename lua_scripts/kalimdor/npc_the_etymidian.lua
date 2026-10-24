-- Un'Goro Crater: The Etymidian — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_the_etymidian : CreatureScript("npc_the_etymidian")).
-- Entry: 28092 (wowhead npc=28092/the-etymidian page, verified this
-- tick; no NPC_ constant exists in the C++ file — the telestra /
-- aeranas wowhead-only precedent, never guessed).
-- Eluna creature events: 34 OnQuestReward
-- (engine/world/quest_reward_grant.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface).
-- Ported arms:
-- - OnQuestReward (childrens_week.cpp:679-685, event 34): quest id
--   12547 (QUEST_THE_ACTIVATION_RUNE) -> Talk(SAY_ACTIVATION 0).
-- Unmodeled (documented-only, no bridges):
-- - MoveInLineOfSight orphan phase machine (quest-status-gated
--   playerGUID / orphanGUID capture, phase 1): ev27 fires from the
--   aggro-engage branch only — corrected 2026-10-24, player:
--   GetQuestStatus IS bridged since 2026-10-24, so the ev27
--   engage-only restriction alone blocks the sighting legs.

local ENTRY_THE_ETYMIDIAN = 28092

local SAY_ACTIVATION = 0

local QUEST_THE_ACTIVATION_RUNE = 12547

-- C++ OnQuestReward for quest 12547: Talk(0).
local function onQuestReward(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_THE_ACTIVATION_RUNE then
        return
    end
    creature:Talk(SAY_ACTIVATION)
end

RegisterCreatureEvent(ENTRY_THE_ETYMIDIAN, 34, onQuestReward)
