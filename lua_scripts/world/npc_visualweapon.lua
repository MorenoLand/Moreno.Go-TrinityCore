-- npc_visualweapon (weapon-visual NPC) --
-- Lua port of NPC_VisualWeapon in
-- src/server/scripts/Custom/cs_npc_weapon_visual.cpp. ENTRY BLOCKER: the
-- creature_template entry for this custom NPC lives in Tim's DB only (this
-- workspace's world.sql has zero creature_template rows) — set
-- ENTRY_NPC_VISUALWEAPON to that entry to register the port; while it is 0
-- the handlers are parsed but not registered (documented-only placeholder).
-- Verifiable numbers (C++ sources): DEFAULT_MESSAGE = 500258
-- (cs_npc_weapon_visual.cpp:40); GOSSIP_ICON_BATTLE = 9 /
-- GOSSIP_ICON_TALK = 7 / GOSSIP_ICON_INTERACT_1 = 4 (GossipDef.h:60-69);
-- GOSSIP_SENDER_MAIN = 1 / GOSSIP_ACTION_INFO_DEF = 1000
-- (ScriptedGossip.h:68-70); EQUIPMENT_SLOT_MAINHAND = 15 /
-- EQUIPMENT_SLOT_OFFHAND = 16 (Player.h:551-556).
-- Ported arms (C++ OnGossipHello / OnGossipSelect):
-- OnGossipHello: "Main-Hand" (action 1001), "Off-Hand" (1002), "Nevermind"
--   (1003); sends menu 500258.
-- OnGossipSelect: ClearMenus first. 1001/1002 open the visual menu for the
--   chosen hand; 1003 closes the menu (CloseGossipMenuFor); other actions
--   index vData — Submenu != 0 navigates there, Submenu == 0 applies the
--   visual via SetVisual then re-shows the same menu.
-- ADAPTATION (documented, behavior-identical): C++ keeps the hand choice in
-- the AI's MainHand member; Lua encodes it in the menu-item action
-- (1100+index main-hand, 2100+index off-hand) so concurrent players cannot
-- clobber each other's choice.
-- SetVisual (C++: player->SetUInt16Value(PLAYER_VISIBLE_ITEM_1_ENCHANTMENT
-- + (item->GetSlot() * 2), 0, visual)) is the player:SetVisibleItemEnchantment
-- bridge (lua_player.go): low uint16 of the enchantment word = the visual,
-- temp-enchant high word preserved; subclass gate and the "equip an item"
-- message live in the bridge exactly as in C++.

local ENTRY_NPC_VISUALWEAPON = 0 -- Tim's DB entry for the npc_visualweapon NPC

local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ICON_INTERACT_1 = 4
local GOSSIP_ICON_TALK = 7
local GOSSIP_ICON_BATTLE = 9
local MENU_TEXT_ID = 500258
local EQUIPMENT_SLOT_MAINHAND = 15
local EQUIPMENT_SLOT_OFFHAND = 16

-- vData (cs_npc_weapon_visual.cpp:42-86): {Menu, Submenu, Icon, Id, Name}.
-- C++ indexes 0-based (action = index); Lua is 1-based, so subtract 1 when
-- building actions and add 1 when resolving them.
local vData = {
    { 1, 0, GOSSIP_ICON_BATTLE, 3789, "Berserking" },
    { 1, 0, GOSSIP_ICON_BATTLE, 3854, "Spell Power" },
    { 1, 0, GOSSIP_ICON_BATTLE, 3273, "Deathfrost" },
    { 1, 0, GOSSIP_ICON_BATTLE, 3225, "Executioner" },
    { 1, 0, GOSSIP_ICON_BATTLE, 3870, "Blood Draining" },
    { 1, 0, GOSSIP_ICON_BATTLE, 1899, "Unholy Weapon" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2674, "Spellsurge" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2675, "Battlemaster" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2671, "Arcane and Fire Spell Power" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2672, "Shadow and Frost Spell Power" },
    { 1, 0, GOSSIP_ICON_BATTLE, 3365, "Rune of Swordshattering" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2673, "Mongoose" },
    { 1, 0, GOSSIP_ICON_BATTLE, 2343, "Spell Power" },
    { 1, 2, GOSSIP_ICON_TALK, 0, "Next ->" },
    { 2, 0, GOSSIP_ICON_BATTLE, 425, "Black Temple Dummy" },
    { 2, 0, GOSSIP_ICON_BATTLE, 3855, "Spell Power III" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1894, "Icy Weapon" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1103, "Agility" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1898, "Lifestealing" },
    { 2, 0, GOSSIP_ICON_BATTLE, 3345, "Earthliving I" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1743, "MHTest02" },
    { 2, 0, GOSSIP_ICON_BATTLE, 3093, "Attack Power vs Undead and Demons" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1900, "Crusader" },
    { 2, 0, GOSSIP_ICON_BATTLE, 3846, "Spell Power II" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1606, "Attack Power" },
    { 2, 0, GOSSIP_ICON_BATTLE, 283, "Windfury I" },
    { 2, 0, GOSSIP_ICON_BATTLE, 1, "Rockbiter III" },
    { 2, 3, GOSSIP_ICON_TALK, 0, "Next ->" },
    { 2, 1, GOSSIP_ICON_TALK, 0, "Back <-" },
    { 3, 0, GOSSIP_ICON_BATTLE, 3265, "Blessed Weapon Coating" },
    { 3, 0, GOSSIP_ICON_BATTLE, 2, "Frostbrand I" },
    { 3, 0, GOSSIP_ICON_BATTLE, 3, "Flametongue III" },
    { 3, 0, GOSSIP_ICON_BATTLE, 3266, "Righteous Weapon Coating" },
    { 3, 0, GOSSIP_ICON_BATTLE, 1903, "Spirit" },
    { 3, 0, GOSSIP_ICON_BATTLE, 13, "Sharpened" },
    { 3, 0, GOSSIP_ICON_BATTLE, 26, "Frost Oil" },
    { 3, 0, GOSSIP_ICON_BATTLE, 7, "Deadly Poison" },
    { 3, 0, GOSSIP_ICON_BATTLE, 803, "Fiery Weapon" },
    { 3, 0, GOSSIP_ICON_BATTLE, 1896, "Weapon Damage" },
    { 3, 0, GOSSIP_ICON_BATTLE, 2666, "Intellect" },
    { 3, 0, GOSSIP_ICON_BATTLE, 25, "Shadow Oil" },
    -- C++ quirk kept exact: the menu-3 "Back <-" row carries Submenu = 2,
    -- so it navigates to menu 2, not back to the hand choice.
    { 3, 2, GOSSIP_ICON_TALK, 0, "Back <-" },
}

local function showMenu(player, creature, menuId, actionBase)
    for idx, row in ipairs(vData) do
        if row[1] == menuId then
            player:GossipMenuAddItem(row[3], row[5], GOSSIP_SENDER_MAIN, actionBase + idx - 1)
        end
    end
    player:GossipSendMenu(MENU_TEXT_ID, creature)
end

local function onHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1, "|TInterface/PaperDoll/UI-PaperDoll-Slot-MainHand:32:32:-32:0|t Main-Hand", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1, "|TInterface/PaperDoll/UI-PaperDoll-Slot-SecondaryHand:32:32:-32:0|t Off-Hand", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1, "|TInterface/PaperDollInfoFrame/UI-GearManager-Undo:32:32:-32:0|t Nevermind", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
    player:GossipSendMenu(MENU_TEXT_ID, creature)
end

local function onSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        showMenu(player, creature, 1, 1100)
        return
    end
    if action == GOSSIP_ACTION_INFO_DEF + 2 then
        showMenu(player, creature, 1, 2100)
        return
    end
    if action == GOSSIP_ACTION_INFO_DEF + 3 then
        player:GossipComplete()
        return
    end
    -- Visual-menu actions: 1100+index main-hand, 2100+index off-hand
    -- (1001-1003 are the hello-menu control actions; bases sit clear of them).
    local actionBase = 1100
    local mainHand = true
    if action >= 2100 then
        actionBase = 2100
        mainHand = false
    end
    local idx = action - actionBase + 1
    local row = vData[idx]
    if row == nil then
        return
    end
    local menuData = row[2]
    if menuData == 0 then
        local slot = EQUIPMENT_SLOT_OFFHAND
        if mainHand then
            slot = EQUIPMENT_SLOT_MAINHAND
        end
        player:SetVisibleItemEnchantment(slot, row[4])
        menuData = row[1]
    end
    showMenu(player, creature, menuData, actionBase)
end

if ENTRY_NPC_VISUALWEAPON > 0 then
    RegisterCreatureGossipEvent(ENTRY_NPC_VISUALWEAPON, 1, onHello)
    RegisterCreatureGossipEvent(ENTRY_NPC_VISUALWEAPON, 2, onSelect)
end
