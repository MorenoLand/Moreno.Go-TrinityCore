-- botgiver (NpcBot hire NPC) --
-- Lua port of script_bot_giver in
-- src/server/game/AI/NpcBots/botgiver.cpp (the NPCBot system's hire NPC
-- by Trickerer). Entry (verifiable from the C++ sources): botcommon.h:12
-- names BOT_GIVER_ENTRY = 70000; the creature_template ScriptName binding
-- is DB-side (no TDB in this workspace), but the entry itself is
-- C++-verifiable so the port is registered (npc_kalecgos precedent).
-- Verifiable numbers (file's own enums, botcommon.h): gossip senders
-- GOSSIP_SENDER_BEGIN = 6000, so GOSSIP_SENDER_BOTGIVER_HIRE = 6001 /
-- HIRE_CLASS = 6002 / HIRE_ENTRY = 6003 (botcommon.h:93-96); gossip text
-- ids GOSSIP_BOTGIVER_GREET = 70201 / HIRE = 70202 / HIRE_CLASS = 70203 /
-- HIRE_EMPTY = 70204 (botcommon.h:79-82); BOT_GOSSIP_MAX_ITEMS = 32
-- (botcommon.h:189); BOT_TEXT_NEVERMIND = 70480 ("Nevermind") /
-- BOT_TEXT_BACK = 70482 ("BACK") / BOT_TEXT_HIREFAIL_COST = 70533
-- ("You don't have enough money") / BOT_TEXT_BOTGIVER_SERVICE = 70584
-- ("I need your services") / BOT_TEXT_BOTGIVER_TOO_MANY_BOTS = 70585
-- ("You have too many bots") / BOT_TEXT_BOTGIVER_WISH_TO_HIRE_ = 70586
-- ("Do you wish to hire ") / BOT_TEXT_BOTGIVER__BOT_BUSY = 70587 (" is a
-- bit busy at the moment, try again later.") /
-- BOT_TEXT_BOTGIVER_HIRESUCCESS = 70588 ("Pleasure doing business with
-- you") / class plurals 70589-70604 (botcommon.h:480-495).
-- GOSSIP_ICON_CHAT = 0 / GOSSIP_ICON_TALK = 7 (GossipDef.h:60,67);
-- GOSSIP_ACTION_INFO_DEF = 1000 (ScriptedGossip.h:68). Class loop
-- BOT_CLASS_WARRIOR..BOT_CLASS_END is 1..17 (CLASS_DRUID = 11, 10
-- skipped; botcommon.h:701-718).
-- Eluna gossip events: 1 OnGossipHello, 2 OnGossipSelect
-- (npc_kalecgos precedent — handler args (event, player, creature)
-- and (event, player, creature, sender, action)).
-- Ported arms (C++ OnGossipHello / OnGossipSelect, botgiver.cpp):
-- OnGossipHello: close when the NpcBot module is disabled; offers "I
--   need your services" (sender 6001, action 1001) and "Nevermind"
--   (sender 0, action 1002); sends menu 70201.
-- OnGossipSelect: ClearMenus first, then per sender:
--   0 -> close (C++-exact exit);
--   1 -> re-run OnGossipHello (BACK to main menu; no menu item in this
--     script produces sender 1 — vestigial in C++, kept exact);
--   6001 (HIRE) -> menu 70202: whisper "You have too many bots" and
--     close at the max-bots cap; else one item per enabled class
--     ("<Plural> (<cost>)", sender 6002, action 1000+class), skipping
--     classes over the per-class cap and class 10 (no text id in
--     C++), capped at 31 items; zero classes -> menu 70204; plus the
--     "Nevermind" exit item (sender 0, action 1001);
--   6002 (HIRE_CLASS) -> menu 70203: cost = GetNpcBotCost(player level,
--     class); whisper "You don't have enough money" and close when
--     short; else one coded box-money item per free bot of the class
--     ("<name>", sender 6003, action 1000+entry, box message "Do you
--     wish to hire <name>?", box money = cost), capped at 31 items;
--     zero bots -> menu 70204; plus the "BACK" item (sender 6001,
--     action 1001);
--   6003 (HIRE_ENTRY) -> RecruitNpcBot(entry) == BotMgr::AddBot(bot,
--     true); whisper "Pleasure doing business with you" on
--     BotAddSuccess (0x100), else "<name> is a bit busy at the moment,
--     try again later."; then close.
-- No combat arms: the AI's UpdateAI is empty.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's me->isMoving() -> BotStopMovement() leg — no
--   movement bridge on the session creature object.
-- - HIRE_CLASS free-bot liveness filters (alive, CCed, teleporting,
--   casting, BERSERK aura): the Go hire flow acts on persisted mirror
--   rows, never a live BotAI — the alive/aura legs have no bridge
--   (FreeBotEntries documents this); the temp-bot exclusion is
--   bridged by entry (70552).
-- - HIRE_ENTRY's pre-hire busy re-check (in combat/dead/CCed/
--   teleporting/casting/owned/berserk): same no-live-model family;
--   Recruit's BotAssignResult covers owned/unavailable/cost/level
--   gates natively.
-- - The localized gossip texts are embedded as English constants
--   (botcommon.h comments); npc_text 70201-70204 stay DB-side.
local ENTRY_BOTGIVER = 70000

local GOSSIP_ICON_CHAT = 0
local GOSSIP_ICON_TALK = 7
local GOSSIP_ACTION_INFO_DEF = 1000

local SENDER_HIRE = 6001
local SENDER_HIRE_CLASS = 6002
local SENDER_HIRE_ENTRY = 6003

local TEXT_GREET = 70201
local TEXT_HIRE = 70202
local TEXT_HIRE_CLASS = 70203
local TEXT_HIRE_EMPTY = 70204

local BOT_GOSSIP_MAX_ITEMS = 32

local BOT_ADD_SUCCESS = 0x100

local TEXT_SERVICE = "I need your services" -- BOT_TEXT_BOTGIVER_SERVICE (70584)
local TEXT_NEVERMIND = "Nevermind" -- BOT_TEXT_NEVERMIND (70480)
local TEXT_BACK = "BACK" -- BOT_TEXT_BACK (70482)
local TEXT_TOO_MANY_BOTS = "You have too many bots" -- BOT_TEXT_BOTGIVER_TOO_MANY_BOTS (70585)
local TEXT_HIREFAIL_COST = "You don't have enough money" -- BOT_TEXT_HIREFAIL_COST (70533)
local TEXT_WISH_TO_HIRE = "Do you wish to hire " -- BOT_TEXT_BOTGIVER_WISH_TO_HIRE_ (70586)
local TEXT_BOT_BUSY = " is a bit busy at the moment, try again later." -- BOT_TEXT_BOTGIVER__BOT_BUSY (70587)
local TEXT_HIRESUCCESS = "Pleasure doing business with you" -- BOT_TEXT_BOTGIVER_HIRESUCCESS (70588)

-- BOT_TEXT_CLASS_*_PLU (botcommon.h:480-495), keyed by bot class id;
-- class 10 has no entry in C++ (default textId 0 -> skipped).
local CLASS_PLURALS = {
    [1] = "Warriors", -- 70589
    [2] = "Paladins", -- 70590
    [3] = "Hunters", -- 70598
    [4] = "Rogues", -- 70596
    [5] = "Priests", -- 70592
    [6] = "Death Knights", -- 70595
    [7] = "Shamans", -- 70597
    [8] = "Mages", -- 70591
    [9] = "Warlocks", -- 70593
    [11] = "Druids", -- 70594
    [12] = "Blademasters", -- 70599
    [13] = "Destroyers", -- 70600
    [14] = "Archmagi", -- 70601
    [15] = "Dreadlords", -- 70602
    [16] = "Spell Breakers", -- 70603
    [17] = "Dark Rangers", -- 70604
}

-- BotMgr::GetNpcBotCostStr (botmgr.cpp:1051): nonzero parts only,
-- concatenated directly, coin icon markup. GOLD = 10000, SILVER = 100.
local function costString(cost)
    local gold = cost // 10000
    local rem = cost % 10000
    local silver = rem // 100
    local copper = rem % 100
    local str = ""
    if gold ~= 0 then
        str = str .. gold .. " |TInterface\\Icons\\INV_Misc_Coin_01:8|t"
    end
    if silver ~= 0 then
        str = str .. silver .. " |TInterface\\Icons\\INV_Misc_Coin_03:8|t"
    end
    if copper ~= 0 then
        str = str .. copper .. " |TInterface\\Icons\\INV_Misc_Coin_05:8|t"
    end
    return str
end

-- C++ OnGossipHello: close when the module is disabled; the two-item
-- main menu; menu text 70201.
local function onBotGiverHello(event, player, creature)
    if not player:IsNpcBotEnabled() then
        player:GossipComplete()
        return
    end
    player:GossipMenuAddItem(GOSSIP_ICON_TALK, TEXT_SERVICE,
        SENDER_HIRE, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_NEVERMIND,
        0, GOSSIP_ACTION_INFO_DEF + 2)
    player:GossipSendMenu(0, creature, TEXT_GREET)
end

-- C++ OnGossipSelect HIRE arm: too-many-bots whisper at the cap, else
-- the per-class menu (menu text 70202, 70204 when empty).
local function onHireSelect(player, creature)
    local textId = TEXT_HIRE
    if player:GetNpcBotsCount() >= player:GetMaxNpcBots() then
        creature:Whisper(TEXT_TOO_MANY_BOTS)
        return textId, false
    end
    local maxClass = player:GetMaxNpcBotsPerClass()
    local haveBot = player:GetNpcBotsCount() > 0
    local availCount = 0
    for botclass = 1, 17 do
        local plural = CLASS_PLURALS[botclass]
        if plural ~= nil and player:IsNpcBotClassEnabled(botclass) then
            -- C++: per-class cap applies only when the player has a bot
            -- and the cap is configured (botgiver.cpp HIRE arm).
            if not (haveBot and maxClass > 0
                    and player:GetNpcBotClassCount(botclass) >= maxClass) then
                local cost = player:GetNpcBotCost(player:GetLevel(), botclass)
                player:GossipMenuAddItem(GOSSIP_ICON_TALK,
                    plural .. " (" .. costString(cost) .. ")",
                    SENDER_HIRE_CLASS, GOSSIP_ACTION_INFO_DEF + botclass)
                availCount = availCount + 1
                if availCount >= BOT_GOSSIP_MAX_ITEMS - 1 then -- back
                    break
                end
            end
        end
    end
    if availCount == 0 then
        textId = TEXT_HIRE_EMPTY
    end
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_NEVERMIND,
        0, GOSSIP_ACTION_INFO_DEF + 1)
    return textId, true
end

-- C++ OnGossipSelect HIRE_CLASS arm: cost re-check with the
-- HIREFAIL_COST whisper, else the per-bot coded box-money menu (menu
-- text 70203, 70204 when empty).
local function onHireClassSelect(player, creature, botclass)
    local textId = TEXT_HIRE_CLASS
    local cost = player:GetNpcBotCost(player:GetLevel(), botclass)
    if player:GetCoinage() < cost then
        creature:Whisper(TEXT_HIREFAIL_COST)
        return textId, false
    end
    local availCount = 0
    local freeCount = player:GetFreeNpcBotCount(botclass)
    for i = 0, freeCount - 1 do
        local entry, name = player:GetFreeNpcBot(botclass, i)
        -- C++ AddMenuItem(-1, ...): -1 auto-assigns the next free item
        -- id (GossipDef.cpp:44), which the Go binding already does, so
        -- only icon/message/sender/action/coded/boxMessage/boxMoney are
        -- passed; coded=false, box money = hire cost.
        player:GossipMenuAddItem(GOSSIP_ICON_TALK, name,
            SENDER_HIRE_ENTRY, GOSSIP_ACTION_INFO_DEF + entry,
            false, TEXT_WISH_TO_HIRE .. name .. "?", cost)
        availCount = availCount + 1
        if availCount >= BOT_GOSSIP_MAX_ITEMS - 1 then -- back
            break
        end
    end
    if availCount == 0 then
        textId = TEXT_HIRE_EMPTY
    end
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_BACK,
        SENDER_HIRE, GOSSIP_ACTION_INFO_DEF + 1)
    return textId, true
end

-- C++ OnGossipSelect HIRE_ENTRY arm: paid hire via Recruit (==
-- BotMgr::AddBot(bot, true)), success/busy whispers, then close.
local function onHireEntrySelect(player, creature, entry)
    local result = player:RecruitNpcBot(entry)
    if result == BOT_ADD_SUCCESS then
        creature:Whisper(TEXT_HIRESUCCESS)
    else
        creature:Whisper(player:GetNpcBotName(entry) .. TEXT_BOT_BUSY)
    end
end

-- C++ OnGossipSelect: ClearMenus first; the sender switch; subMenu
-- decides between SendGossipMenu and SendCloseGossip.
local function onBotGiverSelect(event, player, creature, sender, action)
    if not player:IsNpcBotEnabled() then
        player:GossipComplete()
        return
    end
    player:GossipClearMenu()
    local subMenu = false
    local textId = TEXT_GREET
    if sender == 1 then
        -- BACK: return to main menu (C++ returns OnGossipHello).
        onBotGiverHello(event, player, creature)
        return
    elseif sender == SENDER_HIRE then
        textId, subMenu = onHireSelect(player, creature)
    elseif sender == SENDER_HIRE_CLASS then
        textId, subMenu = onHireClassSelect(player, creature,
            action - GOSSIP_ACTION_INFO_DEF)
    elseif sender == SENDER_HIRE_ENTRY then
        onHireEntrySelect(player, creature, action - GOSSIP_ACTION_INFO_DEF)
    end
    -- sender 0 (exit) falls through with subMenu false == C++ break.
    if subMenu then
        player:GossipSendMenu(0, creature, textId)
    else
        player:GossipComplete()
    end
end

RegisterCreatureGossipEvent(ENTRY_BOTGIVER, 1, onBotGiverHello)
RegisterCreatureGossipEvent(ENTRY_BOTGIVER, 2, onBotGiverSelect)
