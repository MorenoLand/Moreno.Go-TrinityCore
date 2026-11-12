-- Nat Pagle (Dustwallow Marsh) --
-- Lua port of npc_nat_pagleAI in
-- src/server/scripts/Kalimdor/zone_dustwallow_marsh.cpp (lines 52-104).
-- Entry (wowhead.com/classic/npc=12919, verified 2026-11-06): 12919.
-- The C++ CreatureScript binds one AI class to the entry; the class
-- is gossip-only (no UpdateAI, no combat arms).
-- Ported arms (C++-exact):
-- - OnGossipHello (ev1): quest menu first when the creature is a
--   quest giver (player:GossipAddQuests twin, erozion precedent);
--   when IsVendor and the player has quest 8227 (Nat's Measuring
--   Tape) rewarded, offer the vendor item (icon 1, sender 1,
--   action GOSSIP_ACTION_TRADE 1, text "I'd like to browse your
--   goods." — ScriptedGossip.h:25,56,70 + GossipDef.h:61) and send
--   menu 7640; otherwise send menu 7638.
-- - OnGossipSelect (ev2): clear the menu; action GOSSIP_ACTION_TRADE
--   (1) opens the vendor list (player:SendListInventory, creature
--   GUID — lua_player.go:6604-6612).
-- The ooze_zap (42489) / ooze_zap_channel_end (42485) SpellScripts
-- are PORTED in zone_dustwallow_marsh.lua (RegisterSpellEvent ev5,
-- + ev8 CheckCast for ooze_zap); energize_aoe (42492) stays out —
-- ev7 fires only on the creature boss-AI path and its quest ID is
-- DBC-bound (census row 326).
-- Melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.

local NAT_PAGLE_ENTRY = 12919

local QUEST_NATS_MEASURING_TAPE = 8227

local GOSSIP_ICON_VENDOR = 1
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_TRADE = 1
local GOSSIP_TEXT_BROWSE_GOODS = "I'd like to browse your goods."

local GOSSIP_MENU_VENDOR = 7640
local GOSSIP_MENU_DEFAULT = 7638

local function natPagleGossipHello(_, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    if creature:IsVendor() and player:GetQuestRewardStatus(QUEST_NATS_MEASURING_TAPE) then
        player:GossipMenuAddItem(GOSSIP_ICON_VENDOR, GOSSIP_TEXT_BROWSE_GOODS,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_TRADE)
        player:GossipSendMenu(GOSSIP_MENU_VENDOR, creature)
    else
        player:GossipSendMenu(GOSSIP_MENU_DEFAULT, creature)
    end
end

local function natPagleGossipSelect(_, player, creature, _, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_TRADE then
        player:SendListInventory(creature)
    end
end

RegisterCreatureGossipEvent(NAT_PAGLE_ENTRY, 1, natPagleGossipHello)
RegisterCreatureGossipEvent(NAT_PAGLE_ENTRY, 2, natPagleGossipSelect)
