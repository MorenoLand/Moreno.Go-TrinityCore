-- npc_prof_blacksmith -- Lua port of
-- src/server/scripts/World/npc_professions.cpp
-- (npc_prof_blacksmith :409-671). LEARN SIDES ONLY: the unlearn arms
-- (:450/:455 weapon/armor, :468/:474/:480 hammer/axe/sword, and the
-- SendConfirmUnlearn box arms) are omitted — ProcessUnlearnAction
-- (:385) needs EquippedOk (equipped-item RequiredSpell scan), which
-- has no bridge (GetItemByPos is a nil stub). No unlearn menu items
-- are offered, so the omitted arms are unreachable.
-- Ported arms (C++-exact except the noted deviation):
-- - OnGossipHello: quest menu first (GossipAddQuests), then the
--   vendor item if IsVendor, then the trainer item if IsTrainer
--   (C++ arm order; the Lua hello vetoes the native fallthrough, so
--   these arms are replicated here rather than dropped). Then the
--   learn items: omni trainers (11145/11176) offer armor learn
--   (action 1001) when neither specialty is known and the Thorium
--   Brotherhood (faction 46) rank is Friendly or better, and weapon
--   learn (1002) on the mirrored faction-289 gate; hammer (11191),
--   axe (11192), sword (11193) trainers offer their spec learn
--   (actions 1005/1006/1007, sender LEARN) when the player knows
--   Weaponsmithing, is over level 49, has base skill >= 250, and
--   knows no weapon sub-specialty. Menu text is
--   player:GetGossipTextId(creature), as in C++.
-- - OnGossipSelect: menu cleared first; MAIN sender runs
--   SendActionMenu (trade -> SendListInventory, train ->
--   SendTrainerList, 1001/1002 -> learn cast if the specialty is not
--   yet known, then close); LEARN sender runs SendConfirmLearn (the
--   learn item re-offered with sender CHECK and the same action);
--   CHECK sender re-runs SendActionMenu (1005/1006/1007 -> learn
--   cast, then close, no re-check, as in C++).
-- Deviation: C++ casts the learn spells triggered (true); Lua
-- player:CastSpell drops the triggered flag (engineering-trinket
-- precedent) — cast-bar/power legs stay unmodeled.
-- Retires the documented-only note for CLASS 1 in
-- lua_scripts/world/npc_professions_blacksmith.lua.

local SKILL_BLACKSMITHING = 164

local S_WEAPON = 9787
local S_ARMOR = 9788
local S_HAMMER = 17040
local S_AXE = 17041
local S_SWORD = 17039

local S_LEARN_WEAPON = 9789
local S_LEARN_ARMOR = 9790
local S_LEARN_HAMMER = 39099
local S_LEARN_AXE = 39098
local S_LEARN_SWORD = 39097

local REP_ARMOR = 46
local REP_WEAPON = 289
local REP_FRIENDLY = 4

local GOSSIP_ICON_CHAT = 0
local GOSSIP_ICON_VENDOR = 1
local GOSSIP_ICON_TRAINER = 3
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_SENDER_LEARN = 50
local GOSSIP_SENDER_CHECK = 52
local GOSSIP_ACTION_TRADE = 1
local GOSSIP_ACTION_TRAIN = 2
local GOSSIP_ACTION_INFO_DEF = 1000

local GOSSIP_TEXT_BROWSE_GOODS = "I'd like to browse your goods."
local GOSSIP_TEXT_TRAIN = "Train me!"
local GOSSIP_ARMOR_LEARN = "Please teach me how to become a Armorsmith"
local GOSSIP_WEAPON_LEARN = "Please teach me how to become a Weaponsmith"
local GOSSIP_LEARN_HAMMER = "Please teach me how to become a Hammersmith, Lilith"
local GOSSIP_LEARN_AXE = "Please teach me how to become a Axesmith, Kilram"
local GOSSIP_LEARN_SWORD = "Please teach me how to become a Swordsmith, Seril"

local function hasWeaponSub(player)
    return player:HasSpell(S_HAMMER) or player:HasSpell(S_AXE) or player:HasSpell(S_SWORD)
end

local function onGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    if creature:IsVendor() then
        player:GossipMenuAddItem(GOSSIP_ICON_VENDOR, GOSSIP_TEXT_BROWSE_GOODS, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_TRADE)
    end
    if creature:IsTrainer() then
        player:GossipMenuAddItem(GOSSIP_ICON_TRAINER, GOSSIP_TEXT_TRAIN, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_TRAIN)
    end
    local entry = creature:GetEntry()
    if player:GetBaseSkillValue(SKILL_BLACKSMITHING) >= 225 then
        if entry == 11145 or entry == 11176 then
            if not player:HasSpell(S_ARMOR) and not player:HasSpell(S_WEAPON)
                and player:GetReputationRank(REP_ARMOR) >= REP_FRIENDLY then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ARMOR_LEARN, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
            end
            if not player:HasSpell(S_WEAPON) and not player:HasSpell(S_ARMOR)
                and player:GetReputationRank(REP_WEAPON) >= REP_FRIENDLY then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_WEAPON_LEARN, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
            end
        end
    end
    if player:HasSpell(S_WEAPON) and player:GetLevel() > 49
        and player:GetBaseSkillValue(SKILL_BLACKSMITHING) >= 250 then
        if entry == 11191 and not hasWeaponSub(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_HAMMER, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 5)
        elseif entry == 11192 and not hasWeaponSub(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_AXE, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 6)
        elseif entry == 11193 and not hasWeaponSub(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SWORD, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 7)
        end
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
    return true
end

local function sendActionMenu(player, creature, action)
    if action == GOSSIP_ACTION_TRADE then
        player:SendListInventory(creature:GetGUID())
    elseif action == GOSSIP_ACTION_TRAIN then
        player:SendTrainerList(creature:GetGUID())
    elseif action == GOSSIP_ACTION_INFO_DEF + 1 then
        if not player:HasSpell(S_ARMOR) then
            player:CastSpell(player, S_LEARN_ARMOR, false)
        end
        player:GossipComplete()
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        if not player:HasSpell(S_WEAPON) then
            player:CastSpell(player, S_LEARN_WEAPON, false)
        end
        player:GossipComplete()
    elseif action == GOSSIP_ACTION_INFO_DEF + 5 then
        player:CastSpell(player, S_LEARN_HAMMER, false)
        player:GossipComplete()
    elseif action == GOSSIP_ACTION_INFO_DEF + 6 then
        player:CastSpell(player, S_LEARN_AXE, false)
        player:GossipComplete()
    elseif action == GOSSIP_ACTION_INFO_DEF + 7 then
        player:CastSpell(player, S_LEARN_SWORD, false)
        player:GossipComplete()
    end
end

local function sendConfirmLearn(player, creature, action)
    if action ~= 0 then
        local entry = creature:GetEntry()
        if entry == 11191 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_HAMMER, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        elseif entry == 11192 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_AXE, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        elseif entry == 11193 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SWORD, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        end
    end
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if sender == GOSSIP_SENDER_MAIN then
        sendActionMenu(player, creature, action)
    elseif sender == GOSSIP_SENDER_LEARN then
        sendConfirmLearn(player, creature, action)
    elseif sender == GOSSIP_SENDER_CHECK then
        sendActionMenu(player, creature, action)
    end
    return true
end

for _, entry in ipairs({ 11145, 11176, 11146, 11178, 5164, 11177, 11191, 11192, 11193 }) do
    RegisterCreatureGossipEvent(entry, 1, onGossipHello)
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
