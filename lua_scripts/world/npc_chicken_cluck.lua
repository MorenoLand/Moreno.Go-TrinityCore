-- Chicken (Westfall, cluck quest) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:213-292
-- (npc_chicken_cluckAI). Entry 620 TDB-verified (ScriptName
-- npc_chicken_cluck; 0 spawns in this TDB snapshot — dormant here
-- until spawn rows exist).
-- Eluna creature events: 5 OnSpawn, 7 OnUpdate, 8 OnReceiveEmote,
-- 23 OnReset, 31 OnQuestAccept, 34 OnQuestReward.
-- Ported arms (C++-exact for all modeled arms):
-- Reset (:227) — SetFaction(FACTION_PREY 31) + RemoveFlag(82,
-- QUESTGIVER 0x2) + ResetFlagTimer 120000.
-- UpdateAI (:236) — 120s questgiver-flag reset: while HasFlag(82,
-- 0x2), timer expiry -> EnterEvadeMode(). Rides ev7 (100ms diff).
-- ReceiveEmote (:254) — TEXT_EMOTE_CHICKEN 22: quest 3861 NONE (0)
-- + rand32()%30==1 -> SetFlag + SetFaction(FRIENDLY 35) + Talk(hello,
-- Horde 1 / Alliance 0). TEXT_EMOTE_CHEER 21: quest COMPLETE (1) ->
-- SetFlag + SetFaction + Talk(2).
-- OnQuestAccept (:277) + OnQuestReward (:283) — both call Reset()
-- for quest 3861. The empty JustEngagedWith (:234) registers nothing.

local ENTRY_CHICKEN = 620

local QUEST_CLUCK = 3861
local QUEST_STATUS_NONE = 0
local QUEST_STATUS_COMPLETE = 1
local FACTION_PREY = 31
local FACTION_FRIENDLY = 35
local UNIT_NPC_FLAGS = 82
local UNIT_NPC_FLAG_QUESTGIVER = 0x2
local TEXT_EMOTE_CHICKEN = 22
local TEXT_EMOTE_CHEER = 21
local EMOTE_HELLO_A = 0
local EMOTE_HELLO_H = 1
local EMOTE_CLUCK_TEXT = 2

local chickenState = {}

local function resetChicken(creature)
    local guid = creature:GetGUIDLow()
    chickenState[guid] = {resetFlagTimer = 120000}
    creature:SetFaction(FACTION_PREY)
    creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
end

local function onSpawnOrReset(_, creature)
    resetChicken(creature)
end

-- C++ UpdateAI :236 (ev7 ticks with the 100ms diff).
local function onUpdate(_, creature, diff)
    local guid = creature:GetGUIDLow()
    local st = chickenState[guid]
    if not st then
        return
    end
    if creature:HasFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER) then
        if st.resetFlagTimer <= diff then
            creature:EnterEvadeMode()
        else
            st.resetFlagTimer = st.resetFlagTimer - diff
        end
    end
end

local function onReceiveEmote(_, creature, player, emote)
    if emote == TEXT_EMOTE_CHICKEN then
        if player:GetQuestStatus(QUEST_CLUCK) == QUEST_STATUS_NONE
            and math.random(0, 29) == 1 then
            creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
            creature:SetFaction(FACTION_FRIENDLY)
            if player:IsHorde() then
                creature:Talk(EMOTE_HELLO_H)
            else
                creature:Talk(EMOTE_HELLO_A)
            end
        end
    elseif emote == TEXT_EMOTE_CHEER then
        if player:GetQuestStatus(QUEST_CLUCK) == QUEST_STATUS_COMPLETE then
            creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
            creature:SetFaction(FACTION_FRIENDLY)
            creature:Talk(EMOTE_CLUCK_TEXT)
        end
    end
end

local function onQuestAcceptOrReward(_, _, creature, quest)
    if quest and quest:GetId() == QUEST_CLUCK then
        resetChicken(creature)
    end
end

RegisterCreatureEvent(ENTRY_CHICKEN, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_CHICKEN, 23, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_CHICKEN, 7, onUpdate)
RegisterCreatureEvent(ENTRY_CHICKEN, 8, onReceiveEmote)
RegisterCreatureEvent(ENTRY_CHICKEN, 31, onQuestAcceptOrReward)
RegisterCreatureEvent(ENTRY_CHICKEN, 34, onQuestAcceptOrReward)
