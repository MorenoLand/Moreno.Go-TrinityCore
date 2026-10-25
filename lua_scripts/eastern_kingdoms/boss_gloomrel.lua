-- Gloomrel (Blackrock Depths, Tomb of Seven) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_tomb_of_seven.cpp (boss_gloomrel /
-- boss_gloomrelAI — a pure gossip teaching script; no combat arms).
-- Boss-script unit per eastern_kingdoms_script_loader.cpp order
-- (boss_gloomrel follows boss_doomrel in AddSC order).
-- Entry (verifiable from the C++ sources): NPC_GLOOMREL = 9037 in
-- instance_blackrock_depths.cpp line 42 (the BRD instance creatures
-- enum). The creature_template ScriptName binding is DB-side (no TDB
-- in this workspace), but the entry itself is C++-verifiable so the
-- port is registered (doomrel precedent).
-- Eluna gossip events: 1 OnGossipHello, 2 OnGossipSelect (doomrel
-- precedent — RegisterCreatureGossipEvent fires inline in gossip.go).
-- Ported arms (C++ OnGossipHello :89-100, OnGossipSelect :62-87):
-- Hello offers TEACH_1 when the player has the spectral chalice quest
-- rewarded (4083), mining >= 230, and lacks smelt dark iron (14891);
-- it offers TRIBUTE when the quest is unrewarded and mining >= 230
-- (the two gates are mutually exclusive on the rewarded flag, so at
-- most one item shows, as in C++). Select +1 shows TEACH_2 + menu
-- 2606; +11 closes the menu and teaches smelt dark iron (14894,
-- non-triggered, vezax precedent); +2 shows TEACH_3 + menu 2604;
-- +22 closes the menu. C++ arm order kept throughout (select clears
-- the menu before the switch, as in C++ ClearGossipMenuFor).
-- Unmodeled: select +22's instance->DoRespawnGameObject(DATA_GO_CHALICE)
-- leg — no instance GUID-data bridge (the menu still closes, as in
-- C++ order); the hello menu title id is 0 (player->GetGossipTextId
-- has no bridge, zulaman precedent); player:GetSkillValue returns the
-- stored skill value without the C++ temp/perm bonus fold
-- (lua_player.go), so the >= 230 gate reads low only with an active
-- skill bonus at the exact threshold.

local ENTRY_GLOOMREL = 9037

local QUEST_SPECTRAL_CHALICE = 4083
local SKILL_MINING = 186 -- SharedDefines.h:2943
local DATA_SKILLPOINT_MIN = 230 -- boss_tomb_of_seven.cpp Misc enum
local SPELL_SMELT_DARK_IRON = 14891
local SPELL_LEARN_SMELT = 14894

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local MENU_ID_TEACH_2 = 2606 -- boss_tomb_of_seven.cpp (SendGossipMenuFor select +1)
local MENU_ID_TEACH_3 = 2604 -- boss_tomb_of_seven.cpp (SendGossipMenuFor select +2)
local GOSSIP_ITEM_TEACH_1 = "Teach me the art of smelting dark iron"
local GOSSIP_ITEM_TEACH_2 = "Continue..."
local GOSSIP_ITEM_TEACH_3 = "[PH] Continue..."
local GOSSIP_ITEM_TRIBUTE = "I want to pay tribute"

local function onGossipHello(event, player, creature)
    if player:GetQuestRewardStatus(QUEST_SPECTRAL_CHALICE) and player:GetSkillValue(SKILL_MINING) >= DATA_SKILLPOINT_MIN and not player:HasSpell(SPELL_SMELT_DARK_IRON) then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TEACH_1, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    if not player:GetQuestRewardStatus(QUEST_SPECTRAL_CHALICE) and player:GetSkillValue(SKILL_MINING) >= DATA_SKILLPOINT_MIN then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TRIBUTE, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    end
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN then
        return
    end
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TEACH_2, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 11)
        player:GossipSendMenu(MENU_ID_TEACH_2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 11 then
        player:GossipComplete()
        player:CastSpell(player, SPELL_LEARN_SMELT, false)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_TEACH_3, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 22)
        player:GossipSendMenu(MENU_ID_TEACH_3, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 22 then
        player:GossipComplete()
        -- C++ instance->DoRespawnGameObject(DATA_GO_CHALICE, 5min) leg
        -- stays out: no instance GUID-data bridge.
    end
end

RegisterCreatureGossipEvent(ENTRY_GLOOMREL, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_GLOOMREL, 2, onGossipSelect)
