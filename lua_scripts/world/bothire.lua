-- bothire (per-bot hire menu) --
-- Lua port of the bot_ai.cpp free-bot hire arms: OnGossipHello
-- :5279-5336 (reason-coded "Hire this Bot" menu) + OnGossipSelect
-- GOSSIP_SENDER_HIRE arm :7202-7270 (+ :7270-7334), for bot entries
-- 70001-71000 (BOT_ENTRY_BEGIN/BOT_ENTRY_END, botcommon.h:13-14).
-- Registered per entry below; the creature object's GetEntry()
-- identifies the bot (generic object method, scripting/object.go).
-- Verifiable numbers (file's own enums): GOSSIP_SENDER_HIRE = 6073
-- (botcommon.h enum from GOSSIP_SENDER_BEGIN = 6000: 6001-6003
-- botgiver, 6004-6072 class/equipment/roles/abilities/spec/useitem
-- arms, then HIRE at 6073, DISMISS at 6074); GOSSIP_ICON_TAXI = 2
-- (GossipDef.h:62); GOSSIP_ACTION_INFO_DEF = 1000 (ScriptedGossip.h:68).
-- Greet text ids (botcommon.h:71-78): GOSSIP_GREET_NEED_SMTH = 70002
-- ("You need something?"), GOSSIP_GREET_CUSTOM_SPHYNX = 70004,
-- GOSSIP_GREET_CUSTOM_DREADLORD = 70006,
-- GOSSIP_GREET_CUSTOM_DARKRANGER = 70008.
-- Bot class ids (botcommon.h:700-718, CLASS_DRUID = 11, 10 skipped):
-- DEATH_KNIGHT = 6, SPHYNX = 13, ARCHMAGE = 14, DREADLORD = 15,
-- SPELLBREAKER = 16, DARK_RANGER = 17; hire min levels (bot_ai.cpp
-- GOSSIP_SENDER_HIRE arm, verified against npcBotHireMinLevel in the
-- 02:42 add/assign audit): DK 55, SPHYNX 60, ARCHMAGE 20,
-- DREADLORD 60, SPELLBREAKER 20, DARK_RANGER 40.
-- Hire texts (botcommon.h comments, English embedded as constants):
-- HIREDENY_DK = 70347 ("Go away, weakling"),
-- HIREDENY_SPHYNX = 70348 (" is not convinced"),
-- HIREDENY_ARCHMAGE = 70349 ("I am not going to waste my time on
--   just anything"), HIRE_SUCCESS = 70353 ("I am ready"),
-- HIREWARN_SPHYNX_1 = 70440 ("Are you sure you want to risk
--   drawing "), HIREWARN_SPHYNX_2 = 70441 ("'s attention?"),
-- HIREOPTION_SPHYNX = 70442 ("<Insert Coin>"),
-- HIREWARN_DREADLORD = 70443 ("Do you want to entice "),
-- HIREOPTION_DREADLORD = 70444 ("<Try to make an offering>"),
-- HIREWARN_DEFAULT = 70445 ("Do you wish to hire "),
-- HIREOPTION_DEFAULT = 70446 ("<Hire bot>"),
-- HIRE_EMOTE_SPHYNX = 70526 (" makes a grinding sound and begins
--   to follow "), HIREFAIL_OWNED = 70527 ("%s will not join you
--   until dismissed by the owner"), HIREFAIL_LVL60 = 70528 ("%s
--   will not join you until you are level 60"),
-- HIREFAIL_LVL55 = 70529, HIREFAIL_LVL40 = 70530,
-- HIREFAIL_LVL20 = 70531, HIREFAIL_MAXBOTS = 70532 ("You exceed
--   max npcbots (%u)"), HIREFAIL_COST = 70533 ("You don't have
--   enough money"), HIREFAIL_MAXCLASSBOTS = 70534 ("You cannot
--   have more bots of that class! %u of %u").
-- Eluna gossip events: 1 OnGossipHello, 2 OnGossipSelect
-- (npc_kalecgos precedent — handler args (event, player, creature)
-- and (event, player, creature, sender, action)).
-- Ported arms (C++ OnGossipHello / OnGossipSelect, bot_ai.cpp):
-- OnGossipHello: close when the NpcBot module or the bot's class is
--   disabled, when the entry is unknown or the temp-bot mirror image
--   (70552, BOT_ENTRY_MIRROR_IMAGE_BM, botcommon.h:17 ==
--   bot_ai::IsTempBot, bot_ai.h:114), or when the bot is owned
--   (C++ falls through to the owner-only menus this port does not
--   implement); else the single reason-coded item — AddMenuItem(-1,
--   GOSSIP_ICON_TAXI, <option>, 6073, 1000+reason, <warn>, cost,
--   false) with reason -1..4 (bot_ai.cpp:5279-5336), then
--   SendGossipMenu with the class greet text id.
-- OnGossipSelect (sender 6073): reason = action - 1000; reason 0 ->
--   owned re-check (HIREFAIL_OWNED sys message; the whisper is
--   commented out in C++), per-class level denies (whisper/emote +
--   HIREFAIL_LVL** sys messages), then RecruitNpcBot(entry) ==
--   SetBotOwner -> BotMgr::AddBot(bot, true) with the SPHYNX emote /
--   "I am ready" whisper on BotAddSuccess (0x100); reason -1 ->
--   (faction/yell/attack leg, no bridge — see below); reasons 1-4 ->
--   the C++ deny sys messages (whisper nuance excepted — see below).
--   The C++ arm never sets subMenu, so SendCloseGossip follows every
--   path (bot_ai.cpp:7782-7785).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's prelude guard: IsTempBot bridged by entry
--   (70552); the me->IsInCombat()/CCed()/IsCasting()/
--   IsDuringTeleport()/BOT_COMMAND_ISSUED_ORDER legs and the select
--   prelude's casting/CC/teleport/order legs are live-unit-state with
--   no model on the session creature object.
-- - reason -1 (HasAura(BERSERK)) at menu time and its select arm
--   (SetFaction(14)/BotYell/Attack): the aura has no Lua bridge and
--   the attack leg has no live combat-AI bridge (03:04 audit).
-- - The hello arm's me->isMoving() -> BotStopMovement() leg: no
--   movement bridge on the session creature object (botgiver.lua
--   precedent).
-- - The GM debug menu item (GOSSIP_SENDER_DEBUG arm): the Lua
--   surface has no IsGameMaster binding; the item is not ported.
-- - BotSay("...") on failed hire and in the reason 1-4 arms: no
--   creature-say bridge on the Lua surface (presentation only).
-- - reason 1's owner-name whisper ("Go away. I serve my master
--   <name|unknown (guid)>"): no character-name-by-guid binding on
--   the Lua surface; the C++-identical HIREFAIL_OWNED sys message
--   is sent instead. SendBuyError (reason 3) has no bridge either;
--   the sys message + cost string is wire-identical otherwise.
-- - The owner-only menus shown after this arm in C++ (dismiss,
--   class, equipment, ...): not part of the hire slice.
local BOT_ENTRY_BEGIN = 70001
local BOT_ENTRY_END = 71000
local BOT_ENTRY_MIRROR_IMAGE_BM = 70552 -- botcommon.h:17

local GOSSIP_ICON_TAXI = 2
local GOSSIP_ACTION_INFO_DEF = 1000
local SENDER_HIRE = 6073

local TEXT_GREET_DEFAULT = 70002
local TEXT_GREET_SPHYNX = 70004
local TEXT_GREET_DREADLORD = 70006
local TEXT_GREET_DARKRANGER = 70008

local CLASS_DEATH_KNIGHT = 6
local CLASS_SPHYNX = 13
local CLASS_ARCHMAGE = 14
local CLASS_DREADLORD = 15
local CLASS_SPELLBREAKER = 16
local CLASS_DARK_RANGER = 17

local BOT_ADD_SUCCESS = 0x100

-- Hire min levels from the bot_ai.cpp GOSSIP_SENDER_HIRE deny arms
-- (only these six classes gate; verified key-by-key in the 02:42
-- add/assign audit against npcBotHireMinLevel).
local HIRE_MIN_LEVEL = {
    [CLASS_DEATH_KNIGHT] = 55,
    [CLASS_SPHYNX] = 60,
    [CLASS_ARCHMAGE] = 20,
    [CLASS_DREADLORD] = 60,
    [CLASS_SPELLBREAKER] = 20,
    [CLASS_DARK_RANGER] = 40,
}

local TEXT_HIREDENY_DK = "Go away, weakling" -- 70347
local TEXT_HIREDENY_SPHYNX = " is not convinced" -- 70348
local TEXT_HIREDENY_ARCHMAGE = "I am not going to waste my time on just anything" -- 70349
local TEXT_HIRE_SUCCESS = "I am ready" -- 70353
local TEXT_HIREWARN_SPHYNX_1 = "Are you sure you want to risk drawing " -- 70440
local TEXT_HIREWARN_SPHYNX_2 = "'s attention?" -- 70441
local TEXT_HIREOPTION_SPHYNX = "<Insert Coin>" -- 70442
local TEXT_HIREWARN_DREADLORD = "Do you want to entice " -- 70443
local TEXT_HIREOPTION_DREADLORD = "<Try to make an offering>" -- 70444
local TEXT_HIREWARN_DEFAULT = "Do you wish to hire " -- 70445
local TEXT_HIREOPTION_DEFAULT = "<Hire bot>" -- 70446
local TEXT_HIRE_EMOTE_SPHYNX = " makes a grinding sound and begins to follow " -- 70526
local TEXT_HIREFAIL_OWNED = "%s will not join you until dismissed by the owner" -- 70527
local TEXT_HIREFAIL_LVL60 = "%s will not join you until you are level 60" -- 70528
local TEXT_HIREFAIL_LVL55 = "%s will not join you until you are level 55" -- 70529
local TEXT_HIREFAIL_LVL40 = "%s will not join you until you are level 40" -- 70530
local TEXT_HIREFAIL_LVL20 = "%s will not join you until you are level 20" -- 70531
local TEXT_HIREFAIL_MAXBOTS = "You exceed max npcbots (%u)" -- 70532
local TEXT_HIREFAIL_COST = "You don't have enough money" -- 70533
local TEXT_HIREFAIL_MAXCLASSBOTS = "You cannot have more bots of that class! %u of %u" -- 70534

-- BotMgr::GetNpcBotCostStr (botmgr.cpp:1051): nonzero parts only,
-- concatenated directly, coin icon markup. GOLD = 10000, SILVER = 100
-- (same helper as botgiver.lua; math.floor division — this runtime's
-- Lua has no // operator).
local function costString(cost)
    local gold = math.floor(cost / 10000)
    local rem = cost % 10000
    local silver = math.floor(rem / 100)
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

-- C++ OnGossipHello hire arm: module/class guard (the live-state
-- legs — in-combat/CCed/casting/teleporting/order-issued — have no
-- bridge), free-bot gate, reason computation in C++ order, the
-- class-variant warn/option messages, and the single coded=false
-- box-money item carrying 1000+reason.
local function onBotHireHello(event, player, creature)
    local entry = creature:GetEntry()
    if entry == BOT_ENTRY_MIRROR_IMAGE_BM then
        player:GossipComplete()
        return
    end
    if not player:IsNpcBotEnabled() then
        player:GossipComplete()
        return
    end
    local botclass = player:GetNpcBotClass(entry)
    if botclass == 0 or not player:IsNpcBotClassEnabled(botclass) then
        player:GossipComplete()
        return
    end
    -- C++: if (player guid != _ownerGuid) { if (IAmFree()) { ... } }.
    -- Owned bots fall through to the owner-only menus, which this
    -- port does not implement.
    if player:GetNpcBotOwnerGuid(entry) ~= 0 then
        player:GossipComplete()
        return
    end
    local cost = player:GetNpcBotCost(player:GetLevel(), botclass)
    -- C++ arm order: BERSERK (no aura bridge, stays 0), owner (0 here
    -- by the free gate above), max bots, money, max class.
    local reason = 0
    if player:GetNpcBotsCount() >= player:GetMaxNpcBots() then
        reason = 2
    elseif player:GetCoinage() < cost then
        reason = 3
    else
        local maxClass = player:GetMaxNpcBotsPerClass()
        if maxClass > 0 and player:GetNpcBotsCount() > 0
                and player:GetNpcBotClassCount(botclass) >= maxClass then
            reason = 4
        end
    end
    local botName = creature:GetName()
    local textId = TEXT_GREET_DEFAULT
    local warn1, option
    if botclass == CLASS_SPHYNX then
        textId = TEXT_GREET_SPHYNX
        warn1 = TEXT_HIREWARN_SPHYNX_1
        option = TEXT_HIREOPTION_SPHYNX
    elseif botclass == CLASS_DREADLORD then
        textId = TEXT_GREET_DREADLORD
        warn1 = TEXT_HIREWARN_DREADLORD
        option = TEXT_HIREOPTION_DREADLORD
    elseif botclass == CLASS_DARK_RANGER then
        -- Greet text only: the warn/option messages stay default for
        -- dark rangers (bot_ai.cpp:5311-5326 special-cases SPHYNX and
        -- DREADLORD only).
        textId = TEXT_GREET_DARKRANGER
        warn1 = TEXT_HIREWARN_DEFAULT
        option = TEXT_HIREOPTION_DEFAULT
    else
        warn1 = TEXT_HIREWARN_DEFAULT
        option = TEXT_HIREOPTION_DEFAULT
    end
    local warn
    if botclass == CLASS_SPHYNX then
        warn = warn1 .. botName .. TEXT_HIREWARN_SPHYNX_2
    else
        warn = warn1 .. botName .. "?"
    end
    -- C++ AddMenuItem(-1, ...): -1 auto-assigns the next free item
    -- id (GossipDef.cpp:44), which the Go binding already does, so
    -- only icon/message/sender/action/coded/boxMessage/boxMoney are
    -- passed; coded=false, box money = hire cost (botgiver.lua
    -- precedent).
    if reason == 0 then
        player:GossipMenuAddItem(GOSSIP_ICON_TAXI, option,
            SENDER_HIRE, GOSSIP_ACTION_INFO_DEF, false, warn, cost)
    else
        player:GossipMenuAddItem(GOSSIP_ICON_TAXI, option,
            SENDER_HIRE, GOSSIP_ACTION_INFO_DEF + reason)
    end
    player:GossipSendMenu(0, creature, textId)
end

-- C++ deny-then-hire select path for reason 0: the whisper/emote +
-- HIREFAIL_LVL** sys message per class, then RecruitNpcBot(entry)
-- == SetBotOwner -> BotMgr::AddBot(bot, true).
local function denyLevelGate(player, creature, botclass, botName)
    local levelFail = {
        [CLASS_DEATH_KNIGHT] = TEXT_HIREFAIL_LVL55,
        [CLASS_SPHYNX] = TEXT_HIREFAIL_LVL60,
        [CLASS_ARCHMAGE] = TEXT_HIREFAIL_LVL20,
        [CLASS_DREADLORD] = TEXT_HIREFAIL_LVL60,
        [CLASS_SPELLBREAKER] = TEXT_HIREFAIL_LVL20,
        [CLASS_DARK_RANGER] = TEXT_HIREFAIL_LVL40,
    }
    if botclass == CLASS_DEATH_KNIGHT then
        creature:Whisper(TEXT_HIREDENY_DK)
    elseif botclass == CLASS_SPHYNX then
        creature:TextEmote(botName .. TEXT_HIREDENY_SPHYNX)
    elseif botclass == CLASS_ARCHMAGE then
        creature:Whisper(TEXT_HIREDENY_ARCHMAGE)
    end
    -- DREADLORD/SPELLBREAKER/DARK_RANGER have no deny whisper in
    -- C++ (the "placeholder" arms are commented out, bot_ai.cpp
    -- :7242-7262); all six send the HIREFAIL_LVL** sys message.
    player:SendNotification(string.format(levelFail[botclass], botName))
end

-- C++ OnGossipSelect GOSSIP_SENDER_HIRE arm: reason = action - 1000;
-- the arm never sets subMenu, so every path ends in
-- SendCloseGossip == GossipComplete.
local function onBotHireSelect(event, player, creature, sender, action)
    if sender ~= SENDER_HIRE then
        player:GossipComplete()
        return
    end
    if not player:IsNpcBotEnabled() then
        player:GossipComplete()
        return
    end
    local entry = creature:GetEntry()
    local botclass = player:GetNpcBotClass(entry)
    local botName = creature:GetName()
    local reason = action - GOSSIP_ACTION_INFO_DEF
    if reason == 0 then
        if player:GetNpcBotOwnerGuid(entry) ~= 0 then
            -- C++ HIREFAIL_OWNED sys message; the whisper in this arm
            -- is commented out upstream.
            player:SendNotification(string.format(TEXT_HIREFAIL_OWNED, botName))
        else
            local minLevel = HIRE_MIN_LEVEL[botclass]
            if minLevel ~= nil and player:GetLevel() < minLevel then
                denyLevelGate(player, creature, botclass, botName)
            else
                local result = player:RecruitNpcBot(entry)
                if result == BOT_ADD_SUCCESS then
                    if botclass == CLASS_SPHYNX then
                        creature:TextEmote(botName .. TEXT_HIRE_EMOTE_SPHYNX
                            .. player:GetName())
                    else
                        creature:Whisper(TEXT_HIRE_SUCCESS)
                    end
                end
                -- C++ BotSay("...") on a failed hire: no creature-say
                -- bridge on the Lua surface (presentation only).
            end
        end
    elseif reason == -1 then
        -- SetFaction(14)/BotYell/Attack leg: no live combat-AI bridge.
    elseif reason == 1 then
        -- Owner-name whisper nuance (sCharacterCache lookup) has no
        -- Lua bridge; the C++-identical sys message is sent.
        player:SendNotification(string.format(TEXT_HIREFAIL_OWNED, botName))
    elseif reason == 2 then
        player:SendNotification(string.format(TEXT_HIREFAIL_MAXBOTS,
            player:GetMaxNpcBots()))
    elseif reason == 3 then
        local cost = player:GetNpcBotCost(player:GetLevel(), botclass)
        player:SendNotification(TEXT_HIREFAIL_COST .. " ("
            .. costString(cost) .. ")!")
    elseif reason == 4 then
        player:SendNotification(string.format(TEXT_HIREFAIL_MAXCLASSBOTS,
            player:GetNpcBotClassCount(botclass),
            player:GetMaxNpcBotsPerClass()))
    end
    player:GossipComplete()
end

for entry = BOT_ENTRY_BEGIN, BOT_ENTRY_END do
    RegisterCreatureGossipEvent(entry, 1, onBotHireHello)
    RegisterCreatureGossipEvent(entry, 2, onBotHireSelect)
end
