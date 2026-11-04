-- npc_prof_tailor -- Lua port of
-- src/server/scripts/World/npc_professions.cpp
-- (npc_prof_tailor :977-1154). LEARN SIDES ONLY: the unlearn arms
-- (:1027/:1031/:1035 spellfire/mooncloth/shadoweave, and the
-- SendConfirmUnlearn box arms) are omitted — ProcessUnlearnAction
-- (:385) needs EquippedOk (equipped-item RequiredSpell scan), which
-- has no bridge (GetItemByPos is a nil stub). No unlearn menu items
-- are offered, so the omitted arms are unreachable.
-- Ported arms (C++-exact except the noted deviations):
-- - OnGossipHello: quest menu first (GossipAddQuests), then the
--   vendor item if IsVendor, then the trainer item if IsTrainer
--   (C++ arm order; the Lua hello vetoes the native fallthrough, so
--   these arms are replicated here rather than dropped). Then the
--   learn items: when the player has Tailoring, base skill >= 350,
--   level > 59, and any of quests 10831/10832/10833 rewarded, the
--   entry's spec learn item is offered (sender LEARN) unless the
--   player already knows any tailor spec. Menu text is
--   player:GetGossipTextId(creature), as in C++.
-- - OnGossipSelect: menu cleared first; MAIN sender runs the trade /
--   train arms; LEARN sender runs SendConfirmLearn (the learn item
--   re-offered with sender CHECK and the same action); CHECK sender
--   runs ProcessCastaction (learn cast + 200000 copper charge when
--   the spec is not yet known and the player can afford it, else
--   close with no charge).
-- Deviations: (1) C++ casts the learn spells triggered (true); Lua
-- player:CastSpell drops the triggered flag (engineering-trinket
-- precedent) — cast-bar/power legs stay unmodeled. (2) C++ sends
-- BUY_ERR_NOT_ENOUGHT_MONEY on the cannot-afford/already-known leg;
-- SendBuyError has no Lua bridge, so that leg just closes the menu.
-- Retires the documented-only note for npc_prof_tailor in
-- lua_scripts/world/npc_professions_leather_tailor.lua.

local SKILL_TAILORING = 197

local S_SPELLFIRE = 26797
local S_MOONCLOTH = 26798
local S_SHADOWEAVE = 26801

local S_LEARN_SPELLFIRE = 26796
local S_LEARN_MOONCLOTH = 26799
local S_LEARN_SHADOWEAVE = 26800

local LEARN_COST = 200000

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
local GOSSIP_LEARN_SPELLFIRE = "Please teach me how to become a Spellcloth tailor"
local GOSSIP_LEARN_MOONCLOTH = "Please teach me how to become a Mooncloth tailor"
local GOSSIP_LEARN_SHADOWEAVE = "Please teach me how to become a Shadoweave tailor"

local function hasTailorSpell(player)
    return player:HasSpell(S_MOONCLOTH) or player:HasSpell(S_SHADOWEAVE) or player:HasSpell(S_SPELLFIRE)
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
    if player:HasSkill(SKILL_TAILORING) and player:GetBaseSkillValue(SKILL_TAILORING) >= 350
        and player:GetLevel() > 59
        and (player:GetQuestRewardStatus(10831) or player:GetQuestRewardStatus(10832)
            or player:GetQuestRewardStatus(10833)) then
        local entry = creature:GetEntry()
        if entry == 22213 and not hasTailorSpell(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SPELLFIRE, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 1)
        elseif entry == 22208 and not hasTailorSpell(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_MOONCLOTH, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 2)
        elseif entry == 22212 and not hasTailorSpell(player) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SHADOWEAVE, GOSSIP_SENDER_LEARN, GOSSIP_ACTION_INFO_DEF + 3)
        end
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
    return true
end

local function processCastaction(player, spec, learn)
    if not player:HasSpell(spec) and player:GetCoinage() >= LEARN_COST then
        player:CastSpell(player, learn, false)
        player:ModifyMoney(-LEARN_COST)
    end
    player:GossipComplete()
end

local function sendConfirmLearn(player, creature, action)
    if action ~= 0 then
        local entry = creature:GetEntry()
        if entry == 22213 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SPELLFIRE, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        elseif entry == 22208 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_MOONCLOTH, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        elseif entry == 22212 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_LEARN_SHADOWEAVE, GOSSIP_SENDER_CHECK, action)
            player:GossipSendMenu(player:GetGossipTextId(creature), creature)
        end
    end
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if sender == GOSSIP_SENDER_MAIN then
        if action == GOSSIP_ACTION_TRADE then
            player:SendListInventory(creature:GetGUID())
        elseif action == GOSSIP_ACTION_TRAIN then
            player:SendTrainerList(creature:GetGUID())
        end
    elseif sender == GOSSIP_SENDER_LEARN then
        sendConfirmLearn(player, creature, action)
    elseif sender == GOSSIP_SENDER_CHECK then
        if action == GOSSIP_ACTION_INFO_DEF + 1 then
            processCastaction(player, S_SPELLFIRE, S_LEARN_SPELLFIRE)
        elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
            processCastaction(player, S_MOONCLOTH, S_LEARN_MOONCLOTH)
        elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
            processCastaction(player, S_SHADOWEAVE, S_LEARN_SHADOWEAVE)
        end
    end
    return true
end

for _, entry in ipairs({ 22213, 22208, 22212 }) do
    RegisterCreatureGossipEvent(entry, 1, onGossipHello)
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
