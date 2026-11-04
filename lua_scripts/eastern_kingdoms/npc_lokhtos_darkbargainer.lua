-- Lokhtos Darkbargainer (Blackrock Depths, Grim Guzzler) — Lua port of
-- npc_lokhtos_darkbargainerAI in
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/
-- blackrock_depths.cpp (:445-505; enum Lokhtos :436-440; gossip text
-- defines :442-443). Pure gossip script: no combat AI, no timers.
-- Entry: 12944 — no NPC_ constant exists in the C++ tree and no TDB
-- lives in this workspace, so the entry is wowhead-only (Wowhead
-- ?npc=12944, reached from the Wowwiki Lokhtos Darkbargainer page's
-- external link; the fizzule wowhead-only precedent, never guessed).
-- Eluna gossip events: 1 OnGossipHello, 2 OnGossipSelect.
-- Hello (C++ arm order): quest menu first (PrepareQuestMenu twin:
-- player:GossipAddQuests, lua_player.go), then the vendor item if
-- the creature is a vendor and the player's Thorium Brotherhood
-- reputation (faction 59) rank is Friendly or better, then the
-- contract item if quest 7604 (A Binding Contract) is unrewarded
-- and the player holds a Sulfuron Ingot (17203) but no Thorium
-- Brotherhood Contract (18628); the menu text id is 3673 below
-- Friendly, 3677 at Friendly or better (C++-exact).
-- Select (C++ arm order): the menu is cleared first, then the
-- contract arm closes the menu and the player self-casts the
-- contract-creation spell 23059 (non-triggered, C++-exact), and
-- the trade arm opens the vendor list for the creature. Both C++
-- handlers return true, so the Lua handlers return true (a Lua
-- true vetoes the native arms — nefarian polarity note).
-- Deviations: player:GetItemCount counts bags only
-- (character_inventory), while C++ HasItemCount(..., true) also
-- counts the bank — a bank-held ingot or contract does not gate
-- the offer here. The gossip text "Lothos" is verbatim C++.

local ENTRY_LOKHTOS = 12944

local QUEST_A_BINDING_CONTRACT = 7604
local ITEM_SULFURON_INGOT = 17203
local ITEM_THRORIUM_BROTHERHOOD_CONTRACT = 18628
local SPELL_CREATE_THORIUM_BROTHERHOOD_CONTRACT_DND = 23059

local FACTION_THORIUM_BROTHERHOOD = 59
local REP_FRIENDLY = 4

local GOSSIP_ICON_CHAT = 0
local GOSSIP_ICON_VENDOR = 1
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_TRADE = 1
local GOSSIP_ACTION_INFO_DEF = 1000

local GOSSIP_ITEM_SHOW_ACCESS = "Show me what I have access to, Lothos."
local GOSSIP_ITEM_GET_CONTRACT = "Get Thorium Brotherhood Contract"

local MENU_TEXT_UNFRIENDLY = 3673
local MENU_TEXT_FRIENDLY = 3677

local function onHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    if creature:IsVendor() and player:GetReputationRank(FACTION_THORIUM_BROTHERHOOD) >= REP_FRIENDLY then
        player:GossipMenuAddItem(GOSSIP_ICON_VENDOR, GOSSIP_ITEM_SHOW_ACCESS, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_TRADE)
    end
    if not player:GetQuestRewardStatus(QUEST_A_BINDING_CONTRACT)
        and player:GetItemCount(ITEM_THRORIUM_BROTHERHOOD_CONTRACT) == 0
        and player:GetItemCount(ITEM_SULFURON_INGOT) > 0 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_GET_CONTRACT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    if player:GetReputationRank(FACTION_THORIUM_BROTHERHOOD) < REP_FRIENDLY then
        player:GossipSendMenu(MENU_TEXT_UNFRIENDLY, creature)
    else
        player:GossipSendMenu(MENU_TEXT_FRIENDLY, creature)
    end
    return true
end

local function onSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        player:CastSpell(player, SPELL_CREATE_THORIUM_BROTHERHOOD_CONTRACT_DND, false)
    end
    if action == GOSSIP_ACTION_TRADE then
        player:SendListInventory(creature:GetGUID())
    end
    return true
end

RegisterCreatureGossipEvent(ENTRY_LOKHTOS, 1, onHello)
RegisterCreatureGossipEvent(ENTRY_LOKHTOS, 2, onSelect)
