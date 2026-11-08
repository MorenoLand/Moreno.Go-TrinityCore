-- Culling of Stratholme: npc_chromie_start — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/culling_of_stratholme.cpp
-- (npc_chromie_start : public CreatureScript).
-- Entry: NPC_CHROMIE 26527 (instance_culling_of_stratholme.cpp:61).
-- Ported: OnGossipSelect's GOSSIP_OFFSET_TELEPORT arm (:364-367):
-- CloseGossipMenuFor + me->CastSpell(player, SPELL_TELEPORT_PLAYER
-- 53435), non-triggered. The Lua select hook receives (sender,
-- action); GOSSIP_OFFSET_TELEPORT = 2 (enum :238-240), so the gate
-- is action == GOSSIP_ACTION_INFO_DEF + 2 = 1002 (exact).
-- Ported: GOSSIP_OFFSET_SKIP_1 (1005) falls through to the TELEPORT
-- case in C++ (AdvanceDungeonFar leg drops — instance SetData, no
-- instance bridge), so it teleports exactly like TELEPORT.
-- Ported: GOSSIP_OFFSET_EXPLAIN_2 (1004) sends GOSSIP_TEXT_EXPLAIN_3
-- 12952 with no items (kalecgos terminal-step idiom); the
-- AdvanceDungeon leg drops (instance SetData) and the AddItem
-- (ITEM_ARCANE_DISRUPTOR 37888) leg is PORTED 2026-11-06:
-- player:GetItemCount (lua_player.go:2146) + player:AddItem (:2206)
-- are both live — the old "no item-grant bridge" note is retracted
-- (tick-273's AddItem retraction holds here too). Delta: GetItemCount
-- counts character_inventory incl. bank slots, while C++
-- HasItemCount defaults inBankAlso=false — a disruptor sitting only
-- in the bank suppresses the grant in Lua but not in C++.
-- Ported: OnGossipHello's GM arm (:301-345) + state-gated menu send
-- (2026-11-07) + EXPLAIN / SKIP / TELEPORT items (2026-11-07, tick
-- 403): quest menu first when the creature is a quest giver
-- (player:GossipAddQuests twin); the "[GM] Access instance control
-- panel" item only when the player IsGM (lua_player.go:1215 — the
-- CanBeGameMaster twin); progress < PURGE_STARTING sends
-- GOSSIP_TEXT_INITIAL 12939 with the EXPLAIN item (menu 9586 opt 0)
-- + the SKIP item (menu 9586 opt 2) iff every non-GM player in the
-- instance has the CoS achievement (479 normal / 500 heroic,
-- map:IsHeroic + player:HasAchieved, C++ :321-338 scan verbatim),
-- else GOSSIP_TEXT_TELEPORT 13470 with the TELEPORT item (menu 9586
-- opt 1). Option texts sourced from TDB 335.26091 (2026-09-09)
-- gossip_menu_option rows (Wowhead lists no gossip text for any CoS
-- Chromie — live-browser check 2026-11-07, quotes only); the npc_text
-- greetings (12939/12949/12950/12952/13470/15704) were verified in the
-- same dump and resolve from the deployment DB via GossipSendMenu
-- (gossip.go loadNPCText), so Lua carries no greeting strings.
-- Ported: OnGossipSelect's GM arms (:378-408) — OPEN_GM_MENU builds
-- the control panel ("Teleport all players to Arthas" + one item per
-- progress state, stable -> "Set instance progress to 0x%05X", else
-- "Force state to 0x%05X (UNSTABLE)", sent on GOSSIP_TEXT_SKIP_1
-- 15704); GM_INITIAL (1007) closes + GM-only SetGuidData
-- (DATA_GM_RECALL, player GUID); the default arm closes + GM-only
-- SetData (DATA_GM_OVERRIDE, action - INFO_DEF - GM_INITIAL), guarded
-- to exactly the actions the GM menu can produce (1007 + a
-- power-of-two state) so quest-menu clicks never land here. Both
-- writers land as one-shot instanceData flags consumed by the instance
-- pump (instance_culling_of_stratholme.lua onUpdateTick).
-- Ported: OnQuestAccept (:410-413) — accepting quest 13149
-- (Dispelling Illusions) runs AdvanceDungeon (cratesStart flag,
-- same JUST_STARTED guard, consumed by the instance pump).
-- Ported: the EXPLAIN / SKIP / EXPLAIN_1 menu-nav select arms
-- (2026-11-07, tick 403): EXPLAIN (1000) -> menu 9594 opt 0 +
-- text 12949; SKIP (1001) -> menu 11277 opt 0 + text 15704;
-- EXPLAIN_1 (1003) -> menu 9595 opt 0 + text 12950; each clears the
-- menu first (C++ ClearGossipMenuFor, :352). Texts from the same TDB
-- 335.26091 gossip_menu_option rows.
-- Documented-only (nearest-creature axis): npc_hearthsinger_forresten_cot
-- (entry 30551 — inn cinematic, culling_of_stratholme.cpp:118/153/166):
-- FindNearestCreature(talkerEntry, 80.0f, true) (:153) and
-- FindNearestCreature(NPC_BELFAST, 80.0f, true) (:118) both ride
-- npc_hearthsinger_forresten_cotAI::UpdateAI's EventMap, whose events
-- are scheduled from MoveInLineOfSight / SetData (:166 is the
-- unbridged SetGUID hook) — and the :118 arm ends in a cross-creature
-- DoAction leg. The AI is not registered in boss_ai.go; stays out as
-- unregistered AI. RE-CHECKED 2026-11-07 (tick 402) against the
-- tick-398 spline-chain bridge: Forresten has no spline chain at all
-- (Belfast's waypoint walk is DB SmartAI, not C++), and none of its
-- three triggers is bridged — ev27 is engage-only (Forresten never
-- engages), SetGUID has no dispatch, and the SetData sender is
-- Belfast's SmartAI — so the stay-out stands.
-- Ported (crate fluff event #3, tick 402): npc_malcolm_moore (entry
-- 27891 — crate event #3 cinematic, culling_of_stratholme.cpp:945-1081):
-- the FindNearestCreature(NPC_SCRUFFY, 100.0f, true) arms now ride the
-- tick-398 spline-chain bridge (MoveAlongSplineChain + ev28 type 17);
-- see the malcolm section below. The AI is registered here, not in
-- boss_ai.go (Lua registration).
-- Ported (quest-giver axis): npc_chromie_middle (entry 27915, Wowhead
-- WotLK npc=27915/chromie — Ends "Dispelling Illusions", Starts "A Royal
-- Escort", whisper "Good work with the crates! ..."; culling_of_stratholme.cpp
-- :455-549; no entry constant anywhere in the C++ tree — culling_of_stratholme.h
-- names only NPC_CHROMIE = 26527, "we don't need the other two for anything" —
-- DB ScriptName binding only). JustAppeared (:468-472) seeds the 18s
-- WhisperDelay one-shot when progress == CRATES_DONE; the UpdateAI fire leg
-- (:474-487) re-checks progress + Whispered-empty and Talks
-- WHISPER_CRATES_DONE 0. OnGossipHello (:508-514): quest menu first
-- (player:GossipAddQuests twin) + the STEP1 item (menu 9610 opt 0,
-- TDB 335.26091 row) when progress == CRATES_DONE + SendGossipMenu
-- 12992. OnGossipSelect STEP1/2/3 (:516-543): the menu sends
-- 12993/12994/12995 are ported, the STEP1/2/3 item adds are ported
-- (2026-11-07, tick 403 — menus 9611/0 "You want me to do what?" and
-- 9612/0 "Very well, Chromie.", same TDB dump; each arm clears the
-- menu first per C++ :520), and STEP3 runs AdvanceDungeon ->
-- instanceData.utherStart (consumed by the instance pump,
-- CRATES_DONE-guarded), so the chain is live end to end. MoveInLineOfSight (:489-506) stays out: ev27 fires only from
-- the aggro-engage branch (2026-10-24 note) and Chromie never engages,
-- so the WHISPER_COME_TALK 15s-per-player whisper has no fire site.
-- SPAWNGRP_CHROMIE_MID activation (instance :662, state >= CRATES_DONE)
-- has no bridge (instance file line 49); she materializes on the
-- engine-default static spawn, and the JustAppeared arm fires whenever
-- she does, with the CRATES_DONE guard C++-exact.

local ENTRY_CHROMIE_START = 26527

local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ICON_CHAT = 0
local GOSSIP_ICON_INTERACT_1 = 4
local GOSSIP_OFFSET_EXPLAIN = 0
local GOSSIP_OFFSET_SKIP = 1
local GOSSIP_OFFSET_TELEPORT = 2
local GOSSIP_OFFSET_EXPLAIN_1 = 3
local GOSSIP_OFFSET_EXPLAIN_2 = 4
local GOSSIP_OFFSET_SKIP_1 = 5
local GOSSIP_OFFSET_OPEN_GM_MENU = 6
local GOSSIP_OFFSET_GM_INITIAL = 7
local GOSSIP_TEXT_INITIAL = 12939
local GOSSIP_TEXT_TELEPORT = 13470
local GOSSIP_TEXT_EXPLAIN_1 = 12949
local GOSSIP_TEXT_EXPLAIN_2 = 12950
local GOSSIP_TEXT_EXPLAIN_3 = 12952
local GOSSIP_TEXT_SKIP_1 = 15704
local ACHIEVEMENT_NORMAL = 479
local ACHIEVEMENT_HEROIC = 500
local ITEM_ARCANE_DISRUPTOR = 37888
local SPELL_TELEPORT_PLAYER = 53435
local QUEST_DISPELLING_ILLUSIONS = 13149

-- COSProgressStates needed by the hello/GM arms
-- (culling_of_stratholme.h:104-122).
local JUST_STARTED = 0x00001
local PURGE_STARTING = 0x00020
local COMPLETE = 0x10000

-- GetStableStateFor (instance_culling_of_stratholme.cpp:193-224) copy —
-- the instance file owns the canonical table; this copy serves the GM
-- menu's stable/unstable split only.
local StableStateFor = {
    [0x00001] = 0x00001,
    [0x00002] = 0x00002,
    [0x00004] = 0x00004,
    [0x00008] = 0x00010,
    [0x00010] = 0x00010,
    [0x00020] = 0x00010,
    [0x00040] = 0x00010,
    [0x00080] = 0x00100,
    [0x00100] = 0x00100,
    [0x00200] = 0x00100,
    [0x00400] = 0x01000,
    [0x00800] = 0x01000,
    [0x01000] = 0x01000,
    [0x02000] = 0x01000,
    [0x04000] = 0x04000,
    [0x08000] = 0x04000,
    [0x10000] = 0x10000,
}

-- the_black_morass.lua instanceDataOf precedent (shared instanceData
-- table; nil reads as NOT_STARTED / JUST_STARTED progress).
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ OnGossipHello (culling_of_stratholme.cpp:321-338): the SKIP
-- item is added only when every non-GM player in the instance has the
-- Culling of Stratholme achievement (479 normal, 500 heroic by spawn
-- mode — map:IsHeroic is the GetSpawnMode() == DUNGEON_DIFFICULTY_HEROIC
-- twin, globals.go:1221). An empty instance passes vacuously, as in C++.
local function chromieShouldAddSkip(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return false
    end
    local achievement = ACHIEVEMENT_NORMAL
    if mapObj:IsHeroic() then
        achievement = ACHIEVEMENT_HEROIC
    end
    local players = mapObj:GetPlayers()
    if not players then
        return true
    end
    for _, other in ipairs(players) do
        if not other:IsGM() and not other:HasAchieved(achievement) then
            return false
        end
    end
    return true
end

local function chromieGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    if player:IsGM() then
        player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1,
            "[GM] Access instance control panel", GOSSIP_SENDER_MAIN,
            GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_OPEN_GM_MENU)
    end
    local data = instanceDataOf(creature)
    if not data then
        -- C++ fallback (:343): no instance script, plain greeting.
        player:GossipSendMenu(GOSSIP_TEXT_INITIAL, creature)
        return
    end
    if (data.progress or JUST_STARTED) >= PURGE_STARTING then
        -- C++ (:342-345): TELEPORT item (menu 9586 opt 1,
        -- "Yes, please!").
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "Yes, please!",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_TELEPORT)
        player:GossipSendMenu(GOSSIP_TEXT_TELEPORT, creature)
    else
        -- C++ (:315-339): EXPLAIN (menu 9586 opt 0) always, SKIP
        -- (menu 9586 opt 2) iff the achievement scan passes.
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT,
            "Why have I been sent back to this particular place and time?",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN)
        if chromieShouldAddSkip(creature) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT,
                "Chromie, you and I both know what's going to happen in this time stream. We've seen this all before. Can you just skip us ahead to all the real action?",
                GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_SKIP)
        end
        player:GossipSendMenu(GOSSIP_TEXT_INITIAL, creature)
    end
end

-- True iff action is one the GM menu can produce: 1007 + a
-- power-of-two progress state (1..0x10000).
local function isGMOverrideAction(action)
    local state = action - GOSSIP_ACTION_INFO_DEF - GOSSIP_OFFSET_GM_INITIAL
    if state < 1 or state > COMPLETE then
        return false
    end
    while state % 2 == 0 do
        state = math.floor(state / 2)
    end
    return state == 1
end

local function chromieGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN then
        -- C++ OnGossipSelect (culling_of_stratholme.cpp:353-356):
        -- next explain menu (9594 opt 0), text 12949.
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "What was this decision?",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN_1)
        player:GossipSendMenu(GOSSIP_TEXT_EXPLAIN_1, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_SKIP then
        -- C++ (:357-360): skip menu (11277 opt 0), text 15704.
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "Yes, please!",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_SKIP_1)
        player:GossipSendMenu(GOSSIP_TEXT_SKIP_1, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN_1 then
        -- C++ (:368-371): next explain menu (9595 opt 0), text 12950.
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT,
            "So how does the Infinite Dragonflight plan to interfere?",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN_2)
        player:GossipSendMenu(GOSSIP_TEXT_EXPLAIN_2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_TELEPORT
        or action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_SKIP_1 then
        player:GossipComplete()
        creature:CastSpell(player, SPELL_TELEPORT_PLAYER)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN_2 then
        player:GossipClearMenu()
        player:GossipSendMenu(GOSSIP_TEXT_EXPLAIN_3, creature)
        -- C++ OnGossipSelect (culling_of_stratholme.cpp:407-411):
        -- SendGossipMenuFor, then AdvanceDungeon (instance SetData,
        -- out), then AddItem(ITEM_ARCANE_DISRUPTOR) iff the player
        -- does not already have one.
        if player:GetItemCount(ITEM_ARCANE_DISRUPTOR) == 0 then
            player:AddItem(ITEM_ARCANE_DISRUPTOR, 1)
        end
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_OPEN_GM_MENU then
        -- C++ OnGossipSelect (culling_of_stratholme.cpp:378-390):
        -- "Teleport all players to Arthas" + one item per progress
        -- state (stable -> "Set instance progress to 0x%05X", else
        -- "Force state to 0x%05X (UNSTABLE)"), sent on
        -- GOSSIP_TEXT_SKIP_1 15704.
        player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1,
            "Teleport all players to Arthas", GOSSIP_SENDER_MAIN,
            GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_GM_INITIAL)
        local state = 1
        while state <= COMPLETE do
            if StableStateFor[state] == state then
                player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1,
                    string.format("Set instance progress to 0x%05X", state),
                    GOSSIP_SENDER_MAIN,
                    GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_GM_INITIAL + state)
            end
            state = state * 2
        end
        state = 1
        while state <= COMPLETE do
            if StableStateFor[state] ~= state then
                player:GossipMenuAddItem(GOSSIP_ICON_INTERACT_1,
                    string.format("Force state to 0x%05X (UNSTABLE)", state),
                    GOSSIP_SENDER_MAIN,
                    GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_GM_INITIAL + state)
            end
            state = state * 2
        end
        player:GossipSendMenu(GOSSIP_TEXT_SKIP_1, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_GM_INITIAL then
        -- C++ OnGossipSelect (:392-398): close + GM-only
        -- SetGuidData(DATA_GM_RECALL, player GUID) — the teleport leg
        -- runs in the instance pump (instance_culling_of_stratholme.lua
        -- onUpdateTick).
        player:GossipComplete()
        if player:IsGM() then
            local data = instanceDataOf(creature)
            if data then
                data.gmRecall = player:GetGUID()
            end
        end
    elseif isGMOverrideAction(action) then
        -- C++ OnGossipSelect default arm (:399-408): close + GM-only
        -- SetData(DATA_GM_OVERRIDE, action - INFO_DEF - GM_INITIAL),
        -- consumed by the instance pump (forced setProgress).
        player:GossipComplete()
        if player:IsGM() then
            local data = instanceDataOf(creature)
            if data then
                data.gmOverride = action - GOSSIP_ACTION_INFO_DEF - GOSSIP_OFFSET_GM_INITIAL
            end
        end
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

-- C++ OnQuestAccept (culling_of_stratholme.cpp:410-413): accepting
-- QUEST_DISPELLING_ILLUSIONS 13149 runs AdvanceDungeon — the
-- cratesStart flag, consumed by the instance pump with the exact
-- JUST_STARTED guard.
local function chromieQuestAccept(event, player, creature, quest)
    if quest and quest:GetId() == QUEST_DISPELLING_ILLUSIONS then
        local data = instanceDataOf(creature)
        if data then
            data.cratesStart = true
        end
    end
end

RegisterCreatureGossipEvent(ENTRY_CHROMIE_START, 1, chromieGossipHello)
RegisterCreatureGossipEvent(ENTRY_CHROMIE_START, 2, chromieGossipSelect)
RegisterCreatureEvent(ENTRY_CHROMIE_START, 31, chromieQuestAccept)

local ENTRY_CHROMIE_MIDDLE = 27915
local CRATES_DONE = 0x00004
local GOSSIP_TEXT_STEP1 = 12992
local GOSSIP_TEXT_STEP2 = 12993
local GOSSIP_TEXT_STEP3 = 12994
local GOSSIP_TEXT_STEP4 = 12995

-- C++ npc_chromie_middleAI::JustAppeared + UpdateAI WhisperDelay
-- (culling_of_stratholme.cpp:468-487): at spawn, when progress ==
-- CRATES_DONE, the 18s one-shot fires Talk(WHISPER_CRATES_DONE 0)
-- iff progress is still CRATES_DONE and no player has been whispered
-- yet (Whispered.empty() — the MoveInLineOfSight leg that fills the map
-- stays out, so the check is vacuous in practice but kept C++-exact).
local chromieMiddleWhispered = {}
local chromieMiddleTimers = {}

local function chromieMiddleCancelTimers(guid)
    local id = chromieMiddleTimers[guid]
    if id then
        RemoveEventById(id)
        chromieMiddleTimers[guid] = nil
    end
    chromieMiddleWhispered[guid] = nil
end

local function onChromieMiddleSpawn(event, creature)
    local guid = creature:GetGUID()
    chromieMiddleCancelTimers(guid)
    chromieMiddleWhispered[guid] = {}
    local data = instanceDataOf(creature)
    if data and (data.progress or JUST_STARTED) == CRATES_DONE then
        chromieMiddleTimers[guid] = CreateLuaEvent(function()
            chromieMiddleTimers[guid] = nil
            local live = instanceDataOf(creature)
            local whispered = chromieMiddleWhispered[guid]
            if live and (live.progress or JUST_STARTED) == CRATES_DONE
                and (whispered == nil or next(whispered) == nil) then
                creature:Talk(0)
            end
        end, 18000, 1)
    end
end

RegisterCreatureEvent(ENTRY_CHROMIE_MIDDLE, 5, onChromieMiddleSpawn)
RegisterCreatureEvent(ENTRY_CHROMIE_MIDDLE, 4, function(event, creature)
    chromieMiddleCancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_CHROMIE_MIDDLE, 23, function(event, creature)
    chromieMiddleCancelTimers(creature:GetGUID())
end)

-- C++ OnGossipHello (culling_of_stratholme.cpp:508-514): quest menu
-- first when the creature is a quest giver, the STEP1 item (menu 9610
-- opt 0, "What do you think they're up to?") only when progress ==
-- CRATES_DONE, then SendGossipMenuFor 12992.
local function chromieMiddleGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    local data = instanceDataOf(creature)
    if data and (data.progress or JUST_STARTED) == CRATES_DONE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT,
            "What do you think they're up to?",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF)
    end
    player:GossipSendMenu(GOSSIP_TEXT_STEP1, creature)
end

-- C++ OnGossipSelect (culling_of_stratholme.cpp:516-543): STEP1 ->
-- menu 12993 + STEP2 item (menu 9611 opt 0, "You want me to do
-- what?"), STEP2 -> menu 12994 + STEP3 item (menu 9612 opt 0, "Very
-- well, Chromie."), STEP3 -> menu 12995 + AdvanceDungeon
-- (instanceData.utherStart, consumed by the instance pump). The select
-- clears the menu first (C++ :520 ClearGossipMenuFor), so the spent
-- item and quest entries do not carry into the next menu.
local function chromieMiddleGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "You want me to do what?",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipSendMenu(GOSSIP_TEXT_STEP2, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "Very well, Chromie.",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(GOSSIP_TEXT_STEP3, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipSendMenu(GOSSIP_TEXT_STEP4, creature)
        local data = instanceDataOf(creature)
        if data and (data.progress or JUST_STARTED) == CRATES_DONE then
            data.utherStart = true
        end
    end
    return false
end

RegisterCreatureGossipEvent(ENTRY_CHROMIE_MIDDLE, 1, chromieMiddleGossipHello)
RegisterCreatureGossipEvent(ENTRY_CHROMIE_MIDDLE, 2, chromieMiddleGossipSelect)

-- Ported (crate fluff event #2, chain legs tick 402): npc_bartleby_battson
-- (entry 27907, culling_of_stratholme.cpp:852-945). InitializeAI schedules
-- the idle Talk loop (EVENT_BARTLEBY_IDLE, {60,120}s seed, Repeat
-- {120,240}s); seeded here on event 5 (spawn — the AddToWorld funnel
-- fires 5 after 36, creaturemotion.go). The DoAction ACTION_START_FLUFF
-- leg is ported via the crate_helper event-14 relay at the end of this
-- file (ev38 DoAction dispatch exists but is bypassed: the relay calls
-- bartlebyStartFluff directly): the once-guard + idle-loop cancel are
-- ported, and BARTLEBY1 {15,30}s Talks line 1 AND launches the spline
-- chain (MOVEID_EVENT1, CHAIN_BARTLEBY1). MovementInform (ev28, type
-- 17) EVENT1 kneels + schedules BARTLEBY2/2_2/3 (Talk 2, one-shot TALK
-- emote, Talk 3 + stand + CHAIN_BARTLEBY2); EVENT2 despawns in 1s.
-- Ported (crate fluff event #4, chain legs tick 402): npc_sergeant_morigan
-- (entry 27877, culling_of_stratholme.cpp:1131-1280) idle Q&A loop.
-- InitializeAI Reschedules IDLE1 {5,15}s; IDLE1 Talks the question
-- (urand(0,1): 1 -> ASK_NO 0, 0 -> ASK_YES 1; Started -> START 2) and
-- schedules IDLE2 10s; IDLE2 answers via the nearest perelli (entry
-- 27876, 10yd, FindNearestCreature bridge) with the sergeant as Talk
-- target (Question truthy -> PERELLI_NO 0, else PERELLI_YES 1) and
-- reschedules IDLE1 {15,30}s — except Question == 2, which schedules
-- CHAIN1 2s: MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_SERGEANT1).
-- MovementInform (ev28, type 17) EVENT1 kneels + schedules SERGEANT1 /
-- STAND / PERELLI1 / SERGEANT2 / PERELLI2 / SERGEANT3 / LEAVE at
-- 1/3/7/12/20/26/31s (Talk 3, facing 2.617994 + stand, Perelli Talk 2 +
-- facing 5.916666, Talk 4, Perelli Talk 3, Talk 5, CHAIN_SERGEANT2);
-- EVENT2 despawns in 1s. Started is per-GUID state flipped by the
-- crate_helper event-14 relay at the end of this file (nearest
-- sergeant, 15yd).
-- Ported (crate fluff event #1, tick 402): npc_martha_goslin (27884,
-- culling_of_stratholme.cpp:607-700) + npc_jena_anderson (27885,
-- :702-831) on the tick-398 spline-chain bridge (MoveAlongSplineChain
-- + ev28 type 17). Martha: JustAppeared emotestate USE_STANDING (69)
-- + IDLE2 {5,10}s; IDLE1/IDLE2 clear the emotestate and walk
-- CHAIN_MARTHA_IDLE1/2; arrivals set emotestate + facing (3.159046 /
-- 4.764749) and schedule the next idle {9,15}s. DoAction(0) — the
-- 12s motion interrupt Jena calls at MARTHA1/LEAVE — is delivered
-- directly as marthaInterrupt (same direct-call pattern as the relay):
-- the running chain is superseded in place (MoveTo to the current
-- position — Go's MoveClear does NOT cancel a chain run) and the
-- pending idle timer frozen; after 12s the chain re-launches from its
-- first link (C++ resumes mid-link via SplineChainResumeInfo; Go has
-- no resume primitive) and the idle timer re-rolls {9,15}s (C++
-- preserves its remaining time). Jena: InitializeAI walks
-- CHAIN_JENA_INITIAL (isMoving guard); arrivals drive the fluff
-- chain (IDLE1: Started -> JENA_START {1,3}s else IDLE2 {200,700}ms;
-- IDLE2 -> IDLE1 {200,700}ms; MOVE1 arrival -> face martha + Talk 5
-- + MARTHA1 5.5s / JENA7 11s / MOVE2 16s; MOVE2 arrival -> kneel +
-- JENA8 2s / LEAVE 8s; LEAVE arrival -> DespawnOrUnsummon 1s).
-- MARTHA1/LEAVE interrupt martha (DoAction 0) + face her + Talk 0/1;
-- JENA7 Talks 6, JENA8 stands + Talks 7/8 + faces her. DoAction
-- ACTION_START_FLUFF flips Started, delivered directly by the relay
-- (jenaStartFluff): Started is read ONLY at the INITIAL-chain arrival,
-- so a crate reveal after that arrival is a no-op in practice — the
-- flag is ported faithfully anyway. C++ SetFacingTo(o, true) calls
-- land as SetFacingTo(o): the Go bridge always turns in place.
-- Ported (crate fluff event #3, tick 402): npc_malcolm_moore (27891,
-- culling_of_stratholme.cpp:945-1081) on the same bridge.
-- InitializeAI walks CHAIN_MALCOLM1 and summons scruffy (27892) at
-- (1601.1030, 805.3391, 123.7677, 5.471561, MANUAL_DESPAWN default)
-- walking CHAIN_SCRUFFY1 (id 0). Arrivals: EVENT1 Talk 0 + CHAIN 2;
-- EVENT2 schedules SCRUFFY1 1ms (C++ 0s; CreateLuaEvent needs >= 1ms)
-- / MALCOLM2 1s / SCRUFFY_MOVE 4s / MALCOLM_MOVE 8s; EVENT3 schedules
-- FACE3 1ms / MALCOLM3 1s / MALCOLM4 6s / MOVE2 12s; EVENT4 despawns.
-- SCRUFFY1 faces scruffy (5.734883) + scruffy Talks 0; MALCOLM2 Talks
-- 1 to scruffy + faces it; SCRUFFY_MOVE walks scruffy on
-- CHAIN_SCRUFFY2 (id MOVEID_EVENT5, walk=false); MALCOLM_MOVE walks
-- CHAIN_MALCOLM3; FACE3 faces 2.303835; MALCOLM3 Talks 2 to scruffy;
-- MALCOLM4 faces scruffy + Talks 3; MOVE2 walks CHAIN_MALCOLM4 +
-- faces scruffy 5.445427. MOVEID_EVENT5 has NO case here by design:
-- scruffy's CHAIN_SCRUFFY2 arrival informs scruffy's own (default)
-- AI in C++ (SplineChainMovementGenerator::Finalize informs the
-- owner), never Malcolm's — the SCRUFFY_EMOTE leg is dead code in
-- C++ and is not ported.
-- Ported (crate fluff event #5, chain legs tick 402): npc_roger_owens
-- (27903, culling_of_stratholme.cpp:1283-1352). DoAction
-- ACTION_START_FLUFF schedules EVENT_ROGER_START {5,12}s (stand +
-- facing 1.53589 + Talk 0 + MOVE1 at +6s). MovementInform (ev28,
-- type 17): EVENT1 Talks 1 + schedules FACE3 5s / ROGER3 6s / FACE4
-- 12s / ROGER4 14s / MOVE2 18s; EVENT2 faces 1.134464 + Talks 4 +
-- schedules the EXCLAMATION emote 3s / LEAVE 8s; EVENT3 despawns
-- in 1s. The START case's MOVE1 leg + all chain legs are now ported;
-- only the relay delivery stays direct-call (see below).
-- npc_crate_helper (crate_helper_cot,
-- entry 27827): the SpellHit event-14 relay IS ported at the end of
-- this file — on spell 49590 it picks the closest fluff creature by
-- the C++ Find radii (jena 45yd, bartleby 5yd, morigan 15yd, roger
-- 30yd; ReplaceIfCloser order, strict-closest wins) and delivers the
-- in-scope legs directly (bartleby idle-cancel/Started flip +
-- BARTLEBY1 Talk(1) + chain timer, morigan Started flip, roger START
-- stand/facing/Talk(0) timer + MOVE1 chain, jena Started flip — read
-- at her INITIAL-chain arrival only, see above); the malcolm Spawn
-- fallback IS ported (tick 402): with no fluff candidate in range the
-- relay summons malcolm at malcolmSpawn + walks his CHAIN_MALCOLM1,
-- the npc_malcolm_moore::Spawn twin (map->SummonCreature, default
-- MANUAL_DESPAWN). The instance SetData (DATA_CRATE_REVEALED) and GO
-- crate swap legs stay out (no instance/GO-Delete bridge).
-- NOTE: the spline-chain bridge is live but idle until the deployment
-- DB ships script_spline_chain rows (the repo world.db carries no such
-- tables); MoveAlongSplineChain returns false with no chain, the C++
-- error-and-return, so the fluff NPCs stand still instead of walking
-- until then. The chain legs above are the C++-exact arms for when
-- the rows land.
-- Documented-only: at_stratholme_inn_stairs_cot (area trigger, no
-- RegisterAreaTrigger binding in-tree) — instance GetData gate +
-- cross-creature SetGUID leg, both unbridged; forresten's SetGUID /
-- SetData legs ride the same unbridged cross-AI hooks.

local ENTRY_BARTLEBY = 27907
local ENTRY_SERGEANT = 27877
local ENTRY_PERELLI = 27876
local ENTRY_MARTHA = 27884
local ENTRY_JENA = 27885
local ENTRY_MALCOLM = 27891
local ENTRY_SCRUFFY = 27892
local ENTRY_ROGER = 27903
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_KNEEL = 8
local EMOTE_ONESHOT_NONE = 0
local EMOTE_ONESHOT_TALK = 1
local EMOTE_ONESHOT_EXCLAMATION = 5
local EMOTE_STATE_USE_STANDING = 69
local ACTION_START_FLUFF = 9001
-- CrateGenericMisc (culling_of_stratholme.cpp:556-561): shared
-- spline-chain MovementInform ids; SPLINE_CHAIN_MOTION_TYPE is 17
-- (MovementDefines.h / creaturemotion.go:4026).
local SPLINE_CHAIN_MOTION_TYPE = 17
local MOVEID_EVENT1 = 4200
local MOVEID_EVENT2 = 4201
local MOVEID_EVENT3 = 4202
local MOVEID_EVENT4 = 4203
local MOVEID_EVENT5 = 4204
-- CrateEvent1Misc chain ids (culling_of_stratholme.cpp:588-600).
local CHAIN_MARTHA_IDLE1 = 1
local CHAIN_MARTHA_IDLE2 = 2
local CHAIN_JENA_INITIAL = 1
local CHAIN_JENA_IDLE1 = 3
local CHAIN_JENA_IDLE2 = 2
local CHAIN_JENA_MOVE1 = 70
local CHAIN_JENA_MOVE2 = 71
local CHAIN_JENA_LEAVE = 72
-- CrateEvent2Misc chain ids (:849-851).
local CHAIN_BARTLEBY1 = 1
local CHAIN_BARTLEBY2 = 2
-- CrateEvent3Misc chain ids (:964-971).
local CHAIN_MALCOLM1 = 1
local CHAIN_MALCOLM2 = 2
local CHAIN_MALCOLM3 = 3
local CHAIN_MALCOLM4 = 4
local CHAIN_SCRUFFY1 = 1
local CHAIN_SCRUFFY2 = 2
-- CrateEvent4Misc chain ids (:1126-1127).
local CHAIN_SERGEANT1 = 1
local CHAIN_SERGEANT2 = 2
-- CrateEvent5Misc chain ids (:1305-1307).
local CHAIN_ROGER1 = 1
local CHAIN_ROGER2 = 2
local CHAIN_ROGER3 = 3

local timers = {}
local moriganQuestion = {}
local moriganStarted = {}
local bartlebyStarted = {}
local jenaStarted = {}
local marthaChain = {}
local marthaIdleKey = {}
local marthaInterruptState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    moriganQuestion[guid] = nil
    moriganStarted[guid] = nil
    bartlebyStarted[guid] = nil
    jenaStarted[guid] = nil
    marthaChain[guid] = nil
    marthaIdleKey[guid] = nil
    marthaInterruptState[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ EVENT_BARTLEBY_IDLE: Talk(LINE_BARTLEBY_IDLE 0), Repeat
-- {120,240}s (new random range each firing).
local function onBartlebyIdle(creature, guid)
    creature:Talk(0)
    schedule(guid, "idle", math.random(120000, 240000), function()
        onBartlebyIdle(creature, guid)
    end)
end

-- C++ npc_bartleby_battsonAI::DoAction(ACTION_START_FLUFF): once-guard
-- + idle cancel + BARTLEBY1 Talk(1) one-shot + CHAIN_BARTLEBY1 launch
-- (tick 402 — the spline leg is now ported with the arrival handler
-- below).
-- Called directly by the crate_helper relay below — ev38 DoAction
-- dispatch exists but is bypassed: the relay already holds the
-- bartleby object, so no GUID-to-creature lookup is needed.
local function bartlebyStartFluff(creature)
    local guid = creature:GetGUID()
    if bartlebyStarted[guid] then
        return
    end
    bartlebyStarted[guid] = true
    local per = timers[guid]
    if per and per["idle"] then
        RemoveEventById(per["idle"])
        per["idle"] = nil
    end
    -- C++ EVENT_BARTLEBY1: Talk(LINE_BARTLEBY1 1) at {15,30}s +
    -- MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_BARTLEBY1, true).
    schedule(guid, "fluff1", math.random(15000, 30000), function()
        creature:Talk(1)
        creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_BARTLEBY1, true)
    end)
end

-- C++ npc_roger_owensAI::DoAction(ACTION_START_FLUFF) + EVENT_ROGER_START
-- (culling_of_stratholme.cpp:1296-1297, 1336-1342): the START case's
-- stand/facing/Talk(0) legs are ported as a one-shot {5,12}s timer +
-- the EVENT_ROGER_MOVE1 chain leg at +6s (tick 402); the arrival
-- handler below ports the MOVE1+ chain legs. No Started latch in C++;
-- schedule() replaces a pending fluff timer if a second helper
-- reveals roger (C++ would stack a second START event).
-- Called directly by the crate_helper relay below.
local function rogerStartFluff(creature)
    local guid = creature:GetGUID()
    schedule(guid, "fluff", math.random(5000, 12000), function()
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        creature:SetFacingTo(1.53589)
        creature:Talk(0)
        -- C++ EVENT_ROGER_START also schedules EVENT_ROGER_MOVE1 at
        -- +6s (chain leg, tick 402).
        schedule(guid, "move1", 6000, function()
            creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_ROGER1, true)
        end)
    end)
end

local function onBartlebySpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bartlebyStarted[guid] = false
    schedule(guid, "idle", math.random(60000, 120000), function()
        onBartlebyIdle(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_BARTLEBY, 5, onBartlebySpawn)
RegisterCreatureEvent(ENTRY_BARTLEBY, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_BARTLEBY, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

-- C++ Perelli(line[, ori]): nearest perelli Talks line to the
-- sergeant; the idle path calls Perelli without ori (default 0 = no
-- facing change) — the 5.916666f facing belongs to the chain leg
-- (EVENT_PERELLI1, tick 402).
local function moriganPerelli(creature, line, ori)
    local perelli = creature:FindNearestCreature(ENTRY_PERELLI, 10)
    if perelli then
        perelli:Talk(line, creature)
        if ori then
            perelli:SetFacingTo(ori)
        end
    end
end

-- Cyclic idle pair: forward declarations (tharon_ja/gal_darah idiom);
-- plain assignments, never re-local, or closures bind the nil slot.
local onMoriganIdle1, onMoriganIdle2

onMoriganIdle1 = function(creature, guid)
    local question
    if moriganStarted[guid] then
        question = 2
        creature:Talk(2)
    else
        question = math.random(0, 1)
        creature:Talk(question == 1 and 0 or 1)
    end
    moriganQuestion[guid] = question
    schedule(guid, "idle2", 10000, function()
        onMoriganIdle2(creature, guid)
    end)
end

onMoriganIdle2 = function(creature, guid)
    local question = moriganQuestion[guid] or 0
    moriganPerelli(creature, question == 0 and 1 or 0)
    if question == 2 then
        -- EVENT_SERGEANT_CHAIN1 (tick 402): the fluff chain starts 2s
        -- after the answer.
        schedule(guid, "chain1", 2000, function()
            creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_SERGEANT1, true)
        end)
    else
        schedule(guid, "idle1", math.random(15000, 30000), function()
            onMoriganIdle1(creature, guid)
        end)
    end
end

local function onMoriganSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    moriganStarted[guid] = false
    schedule(guid, "idle1", math.random(5000, 15000), function()
        onMoriganIdle1(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_SERGEANT, 5, onMoriganSpawn)
RegisterCreatureEvent(ENTRY_SERGEANT, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SERGEANT, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

-- C++ npc_martha_goslin (culling_of_stratholme.cpp:607-700, tick 402).
-- JustAppeared: emotestate USE_STANDING + IDLE2 {5,10}s. UpdateAI
-- IDLE1/IDLE2: emotestate NONE + walk CHAIN_MARTHA_IDLE1/2.
-- MovementInform (ev28, type 17): EVENT1/2 set emotestate USE_STANDING
-- + facing (3.159046 / 4.764749) + schedule the next idle {9,15}s.
-- DoAction(0): the 12s motion interrupt — see marthaInterrupt below.
local function marthaScheduleIdle(creature, guid, key, delay)
    marthaIdleKey[guid] = key
    schedule(guid, key, delay, function()
        marthaIdleKey[guid] = nil
        creature:EmoteState(EMOTE_ONESHOT_NONE)
        local pointID = MOVEID_EVENT1
        local chainID = CHAIN_MARTHA_IDLE1
        if key == "idle2" then
            pointID = MOVEID_EVENT2
            chainID = CHAIN_MARTHA_IDLE2
        end
        marthaChain[guid] = pointID
        creature:MoveAlongSplineChain(pointID, chainID, true)
    end)
end

local function marthaResume(creature, guid)
    local saved = marthaInterruptState[guid]
    marthaInterruptState[guid] = nil
    if saved == nil then
        return
    end
    -- C++ resumes the chain MID-link (SplineChainResumeInfo link/point
    -- + remaining wait); Go has no resume primitive, so the interrupted
    -- chain restarts from its first link with the same point id — the
    -- arrival re-schedules the next idle event exactly as C++.
    if saved.chain ~= nil then
        marthaChain[guid] = saved.chain
        local chainID = CHAIN_MARTHA_IDLE1
        if saved.chain == MOVEID_EVENT2 then
            chainID = CHAIN_MARTHA_IDLE2
        end
        creature:MoveAlongSplineChain(saved.chain, chainID, true)
    end
    -- C++ freezes the EventMap for the 12s, preserving the idle
    -- timer's remaining time; Lua re-rolls the full {9,15}s range —
    -- ambient-only timing delta on the idle loop.
    if saved.idleKey ~= nil then
        marthaScheduleIdle(creature, guid, saved.idleKey, math.random(9000, 15000))
    end
end

-- C++ npc_martha_goslinAI::DoAction(0) (culling_of_stratholme.cpp:611-617):
-- InterruptTimer = 12s + GetResumeInfo + MotionMaster::Clear() +
-- emotestate NONE; UpdateAI resumes the chain when the timer expires
-- (and freezes the EventMap meanwhile). Delivered directly by Jena's
-- MARTHA1/LEAVE legs (same direct-call pattern as the crate_helper
-- relay). A second interrupt during the first only re-arms the 12s
-- timer: C++ re-saves ResumeInfo, but with no active generator the
-- saved info is unchanged, so the first interrupt's chain/idle are
-- kept here too.
local function marthaInterrupt(creature)
    local guid = creature:GetGUID()
    local chain = marthaChain[guid]
    local idleKey = marthaIdleKey[guid]
    if chain ~= nil then
        -- MotionMaster::Clear() twin: a fresh non-chain Move* launch
        -- supersedes the running chain (launchCreatureScriptSplineSpeed
        -- nils motion.SplineChain); MoveTo to the current position
        -- halts Martha in place with no displacement. Go's MoveClear
        -- does NOT cancel a chain run (advanceSplineChain re-sends the
        -- interrupted leg), so it cannot serve here. The supersede
        -- lands as a POINT inform (type 8, id 0), ignored by the
        -- arrival handler below, matching C++ Clear (no inform).
        creature:MoveTo(0, creature:GetX(), creature:GetY(), creature:GetZ())
        marthaChain[guid] = nil
    end
    if idleKey ~= nil then
        cancelKey(guid, idleKey)
        marthaIdleKey[guid] = nil
    end
    creature:EmoteState(EMOTE_ONSHOT_NONE)
    if chain ~= nil or idleKey ~= nil then
        marthaInterruptState[guid] = { chain = chain, idleKey = idleKey }
    end
    schedule(guid, "interrupt", 12000, function()
        marthaResume(creature, guid)
    end)
end

local function onMarthaSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:EmoteState(EMOTE_STATE_USE_STANDING)
    marthaScheduleIdle(creature, guid, "idle2", math.random(5000, 10000))
end

local function onMarthaMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    marthaChain[guid] = nil
    if id == MOVEID_EVENT1 then
        creature:EmoteState(EMOTE_STATE_USE_STANDING)
        creature:SetFacingTo(3.159046)
        marthaScheduleIdle(creature, guid, "idle2", math.random(9000, 15000))
    elseif id == MOVEID_EVENT2 then
        creature:EmoteState(EMOTE_STATE_USE_STANDING)
        creature:SetFacingTo(4.764749)
        marthaScheduleIdle(creature, guid, "idle1", math.random(9000, 15000))
    end
end

RegisterCreatureEvent(ENTRY_MARTHA, 5, onMarthaSpawn)
RegisterCreatureEvent(ENTRY_MARTHA, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_MARTHA, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_MARTHA, 28, onMarthaMovementInform)

-- C++ npc_jena_anderson (culling_of_stratholme.cpp:702-831, tick 402).
-- InitializeAI walks CHAIN_JENA_INITIAL (isMoving guard); the
-- MovementInform arrivals drive the whole fluff chain; DoAction
-- ACTION_START_FLUFF flips Started (delivered directly by the relay
-- as jenaStartFluff — Started is read only at the INITIAL-chain
-- arrival, so a late reveal is a no-op in practice).
local function jenaStartFluff(creature)
    jenaStarted[creature:GetGUID()] = true
end

local function jenaFindMartha(creature)
    return creature:FindNearestCreature(ENTRY_MARTHA, 100)
end

local function jenaIdle1(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_JENA_IDLE1, true)
end

local function jenaIdle2(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT2, CHAIN_JENA_IDLE2, true)
end

local function jenaStart(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT3, CHAIN_JENA_MOVE1, true)
end

local function jenaMartha1(creature, guid)
    local martha = jenaFindMartha(creature)
    if martha then
        marthaInterrupt(martha)
        martha:SetFacingToObject(creature)
        martha:Talk(0, creature)
    end
end

local function jenaTalk7(creature, guid)
    creature:Talk(6, jenaFindMartha(creature))
end

local function jenaMove2(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT4, CHAIN_JENA_MOVE2, true)
end

local function jenaTalk8(creature, guid)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    local martha = jenaFindMartha(creature)
    creature:Talk(7, martha)
    creature:Talk(8, martha)
    if martha then
        creature:SetFacingToObject(martha)
    end
end

local function jenaLeave(creature, guid)
    local martha = jenaFindMartha(creature)
    if martha then
        marthaInterrupt(martha)
        martha:SetFacingToObject(creature)
        martha:Talk(1, creature)
    end
    creature:MoveAlongSplineChain(MOVEID_EVENT5, CHAIN_JENA_LEAVE, true)
end

local function onJenaMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == MOVEID_EVENT1 then
        if jenaStarted[guid] then
            schedule(guid, "start", math.random(1000, 3000), function()
                jenaStart(creature, guid)
            end)
        else
            schedule(guid, "idle2", math.random(200, 700), function()
                jenaIdle2(creature, guid)
            end)
        end
    elseif id == MOVEID_EVENT2 then
        schedule(guid, "idle1", math.random(200, 700), function()
            jenaIdle1(creature, guid)
        end)
    elseif id == MOVEID_EVENT3 then
        local martha = jenaFindMartha(creature)
        if martha then
            creature:SetFacingToObject(martha)
            creature:Talk(5, martha)
        end
        schedule(guid, "martha1", 5500, function()
            jenaMartha1(creature, guid)
        end)
        schedule(guid, "jena7", 11000, function()
            jenaTalk7(creature, guid)
        end)
        schedule(guid, "move2", 16000, function()
            jenaMove2(creature, guid)
        end)
    elseif id == MOVEID_EVENT4 then
        creature:SetStandState(UNIT_STAND_STATE_KNEEL)
        schedule(guid, "jena8", 2000, function()
            jenaTalk8(creature, guid)
        end)
        schedule(guid, "leave", 8000, function()
            jenaLeave(creature, guid)
        end)
    elseif id == MOVEID_EVENT5 then
        creature:DespawnOrUnsummon(1000)
    end
end

local function onJenaSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    jenaStarted[guid] = false
    if creature:IsMoving() then
        return
    end
    creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_JENA_INITIAL, true)
end

RegisterCreatureEvent(ENTRY_JENA, 5, onJenaSpawn)
RegisterCreatureEvent(ENTRY_JENA, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_JENA, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_JENA, 28, onJenaMovementInform)

-- C++ npc_malcolm_moore (culling_of_stratholme.cpp:945-1081, tick 402).
-- InitializeAI walks CHAIN_MALCOLM1 and summons scruffy walking
-- CHAIN_SCRUFFY1 (id 0). MovementInform arrivals drive the scene;
-- MOVEID_EVENT5 has no case (dead in C++ — see the arrival handler).
local MALCOLM_SPAWN = { 1605.2420, 805.4160, 122.9956, 5.284148 }
local SCRUFFY_SPAWN = { 1601.1030, 805.3391, 123.7677, 5.471561 }

local function malcolmFindScruffy(creature)
    return creature:FindNearestCreature(ENTRY_SCRUFFY, 100)
end

local function malcolmScruffy1(creature, guid)
    local scruffy = malcolmFindScruffy(creature)
    if scruffy then
        scruffy:SetFacingTo(5.734883)
        scruffy:Talk(0)
    end
end

local function malcolmTalk2(creature, guid)
    local scruffy = malcolmFindScruffy(creature)
    if scruffy then
        creature:Talk(1, scruffy)
        creature:SetFacingToObject(scruffy)
    end
end

local function malcolmScruffyMove(creature, guid)
    local scruffy = malcolmFindScruffy(creature)
    if scruffy then
        scruffy:MoveAlongSplineChain(MOVEID_EVENT5, CHAIN_SCRUFFY2, false)
    end
end

local function malcolmMove(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT3, CHAIN_MALCOLM3, true)
end

local function malcolmFace3(creature, guid)
    creature:SetFacingTo(2.303835)
end

local function malcolmTalk3(creature, guid)
    creature:Talk(2, malcolmFindScruffy(creature))
end

local function malcolmTalk4(creature, guid)
    local scruffy = malcolmFindScruffy(creature)
    if scruffy then
        creature:SetFacingToObject(scruffy)
        creature:Talk(3, scruffy)
    end
end

local function malcolmMove2(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT4, CHAIN_MALCOLM4, true)
    local scruffy = malcolmFindScruffy(creature)
    if scruffy then
        scruffy:SetFacingTo(5.445427)
    end
end

local function onMalcolmMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == MOVEID_EVENT1 then
        creature:Talk(0)
        creature:MoveAlongSplineChain(MOVEID_EVENT2, CHAIN_MALCOLM2, true)
    elseif id == MOVEID_EVENT2 then
        -- C++ Seconds(0) -> 1ms here: CreateLuaEvent needs delay >= 1.
        schedule(guid, "scruffy1", 1, function()
            malcolmScruffy1(creature, guid)
        end)
        schedule(guid, "malcolm2", 1000, function()
            malcolmTalk2(creature, guid)
        end)
        schedule(guid, "scruffyMove", 4000, function()
            malcolmScruffyMove(creature, guid)
        end)
        schedule(guid, "malcolmMove", 8000, function()
            malcolmMove(creature, guid)
        end)
    elseif id == MOVEID_EVENT3 then
        schedule(guid, "face3", 1, function()
            malcolmFace3(creature, guid)
        end)
        schedule(guid, "malcolm3", 1000, function()
            malcolmTalk3(creature, guid)
        end)
        schedule(guid, "malcolm4", 6000, function()
            malcolmTalk4(creature, guid)
        end)
        schedule(guid, "move2", 12000, function()
            malcolmMove2(creature, guid)
        end)
    elseif id == MOVEID_EVENT4 then
        creature:DespawnOrUnsummon()
    end
    -- No MOVEID_EVENT5 case: scruffy's CHAIN_SCRUFFY2 arrival informs
    -- scruffy's own (default) AI in C++
    -- (SplineChainMovementGenerator::Finalize informs the owner), never
    -- Malcolm's — the SCRUFFY_EMOTE leg is dead code in C++.
end

local function onMalcolmSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT1, CHAIN_MALCOLM1, true)
    local scruffy = creature:SummonCreature(ENTRY_SCRUFFY, SCRUFFY_SPAWN[1], SCRUFFY_SPAWN[2], SCRUFFY_SPAWN[3], SCRUFFY_SPAWN[4])
    if scruffy then
        scruffy:MoveAlongSplineChain(0, CHAIN_SCRUFFY1, true)
    end
end

RegisterCreatureEvent(ENTRY_MALCOLM, 5, onMalcolmSpawn)
RegisterCreatureEvent(ENTRY_MALCOLM, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_MALCOLM, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_MALCOLM, 28, onMalcolmMovementInform)

-- C++ npc_roger_owens MovementInform (culling_of_stratholme.cpp:1300-1323,
-- tick 402): the chain legs behind the START case ported above.
local function rogerFace3(creature, guid)
    creature:SetFacingTo(6.265732)
end

local function rogerTalk3(creature, guid)
    creature:Talk(2)
end

local function rogerFace4(creature, guid)
    creature:SetFacingTo(4.520403)
end

local function rogerTalk4(creature, guid)
    creature:Talk(3)
end

local function rogerMove2(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT2, CHAIN_ROGER2, true)
end

local function rogerEmote52(creature, guid)
    creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
end

local function rogerLeave(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT3, CHAIN_ROGER3, true)
end

local function onRogerMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == MOVEID_EVENT1 then
        creature:Talk(1)
        schedule(guid, "face3", 5000, function()
            rogerFace3(creature, guid)
        end)
        schedule(guid, "roger3", 6000, function()
            rogerTalk3(creature, guid)
        end)
        schedule(guid, "face4", 12000, function()
            rogerFace4(creature, guid)
        end)
        schedule(guid, "roger4", 14000, function()
            rogerTalk4(creature, guid)
        end)
        schedule(guid, "move2", 18000, function()
            rogerMove2(creature, guid)
        end)
    elseif id == MOVEID_EVENT2 then
        creature:SetFacingTo(1.134464)
        creature:Talk(4)
        schedule(guid, "emote52", 3000, function()
            rogerEmote52(creature, guid)
        end)
        schedule(guid, "leave", 8000, function()
            rogerLeave(creature, guid)
        end)
    elseif id == MOVEID_EVENT3 then
        creature:DespawnOrUnsummon(1000)
    end
end

RegisterCreatureEvent(ENTRY_ROGER, 28, onRogerMovementInform)

-- C++ npc_bartleby_battson MovementInform (culling_of_stratholme.cpp:887-905,
-- tick 402): the arrival legs behind the BARTLEBY1 chain launch above.
local function bartlebyTalk2(creature, guid)
    creature:Talk(2)
end

local function bartlebyEmote22(creature, guid)
    creature:PerformEmote(EMOTE_ONESHOT_TALK)
end

local function bartlebyTalk3(creature, guid)
    creature:Talk(3)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:MoveAlongSplineChain(MOVEID_EVENT2, CHAIN_BARTLEBY2, true)
end

local function onBartlebyMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == MOVEID_EVENT1 then
        creature:SetStandState(UNIT_STAND_STATE_KNEEL)
        schedule(guid, "fluff2", 4000, function()
            bartlebyTalk2(creature, guid)
        end)
        schedule(guid, "fluff22", 6000, function()
            bartlebyEmote22(creature, guid)
        end)
        schedule(guid, "fluff3", 12000, function()
            bartlebyTalk3(creature, guid)
        end)
    elseif id == MOVEID_EVENT2 then
        creature:DespawnOrUnsummon(1000)
    end
end

RegisterCreatureEvent(ENTRY_BARTLEBY, 28, onBartlebyMovementInform)

-- C++ npc_sergeant_morigan MovementInform (culling_of_stratholme.cpp:1155-1179,
-- tick 402): the arrival legs behind the CHAIN1 launch in onMoriganIdle2.
local function moriganSergeant1(creature, guid)
    creature:Talk(3)
end

local function moriganStand(creature, guid)
    creature:SetFacingTo(2.617994)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
end

local function moriganPerelli1(creature, guid)
    moriganPerelli(creature, 2, 5.916666)
end

local function moriganSergeant2(creature, guid)
    creature:Talk(4)
end

local function moriganPerelli2(creature, guid)
    moriganPerelli(creature, 3)
end

local function moriganSergeant3(creature, guid)
    creature:Talk(5)
end

local function moriganLeave(creature, guid)
    creature:MoveAlongSplineChain(MOVEID_EVENT2, CHAIN_SERGEANT2, true)
end

local function onMoriganMovementInform(event, creature, type, id)
    if type ~= SPLINE_CHAIN_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == MOVEID_EVENT1 then
        creature:SetStandState(UNIT_STAND_STATE_KNEEL)
        schedule(guid, "sergeant1", 1000, function()
            moriganSergeant1(creature, guid)
        end)
        schedule(guid, "stand", 3000, function()
            moriganStand(creature, guid)
        end)
        schedule(guid, "perelli1", 7000, function()
            moriganPerelli1(creature, guid)
        end)
        schedule(guid, "sergeant2", 12000, function()
            moriganSergeant2(creature, guid)
        end)
        schedule(guid, "perelli2", 20000, function()
            moriganPerelli2(creature, guid)
        end)
        schedule(guid, "sergeant3", 26000, function()
            moriganSergeant3(creature, guid)
        end)
        schedule(guid, "leave", 31000, function()
            moriganLeave(creature, guid)
        end)
    elseif id == MOVEID_EVENT2 then
        creature:DespawnOrUnsummon(1000)
    end
end

RegisterCreatureEvent(ENTRY_SERGEANT, 28, onMoriganMovementInform)

-- C++ npc_crate_helperAI::SpellHit (culling_of_stratholme.cpp:1410-
-- 1443): on SPELL_ARCANE_DISRUPTION 49590, once per helper (the
-- _crateRevealed latch), the instance SetData + GO crate swap legs
-- stay out (no instance/GO-Delete bridge), and the nearest-fluff
-- DoAction leg is delivered directly: the C++ npc_*::Find(me) helpers
-- are FindNearestCreature(entry, radius, true) calls — jena 27885 at
-- 45yd, bartleby 27907 at 5yd, sergeant 27877 at 15yd, roger 27903 at
-- 30yd — and ReplaceIfCloser keeps the strict-closest candidate in
-- that order (first wins ties). Lua GetDistance is the 3D metric vs
-- C++ 2D-squared, same winner on flat Stratholme ground. The
-- no-candidate malcolm Spawn fallback is ported (tick 402): the relay
-- summons malcolm and his own ev5 walks the chain + summons scruffy.
local ENTRY_CRATE_HELPER = 27827
local SPELL_ARCANE_DISRUPTION = 49590

local crateRevealed = {}

local function onCrateHelperSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCANE_DISRUPTION then
        return
    end
    local guid = creature:GetGUID()
    if crateRevealed[guid] then
        return
    end
    crateRevealed[guid] = true
    local closest = nil
    local closestDist = nil
    local candidates = {
        { ENTRY_JENA, 45 },
        { ENTRY_BARTLEBY, 5 },
        { ENTRY_SERGEANT, 15 },
        { ENTRY_ROGER, 30 },
    }
    for _, pair in ipairs(candidates) do
        local found = creature:FindNearestCreature(pair[1], pair[2])
        if found then
            local dist = creature:GetDistance(found)
            if closestDist == nil or dist < closestDist then
                closest = found
                closestDist = dist
            end
        end
    end
    if closest == nil then
        -- C++ npc_malcolm_moore::Spawn fallback (tick 402): no fluff
        -- candidate in range — summon malcolm at malcolmSpawn
        -- (MANUAL_DESPAWN default); his own ev5 (InitializeAI twin)
        -- walks CHAIN_MALCOLM1 + summons scruffy.
        creature:SummonCreature(ENTRY_MALCOLM, MALCOLM_SPAWN[1], MALCOLM_SPAWN[2], MALCOLM_SPAWN[3], MALCOLM_SPAWN[4])
        return
    end
    local entry = closest:GetEntry()
    if entry == ENTRY_BARTLEBY then
        bartlebyStartFluff(closest)
    elseif entry == ENTRY_SERGEANT then
        moriganStarted[closest:GetGUID()] = true
    elseif entry == ENTRY_ROGER then
        rogerStartFluff(closest)
    elseif entry == ENTRY_JENA then
        jenaStartFluff(closest)
    end
end

RegisterCreatureEvent(ENTRY_CRATE_HELPER, 14, onCrateHelperSpellHit)

-- Roger has no spawn behavior in C++ (no InitializeAI); these only
-- cancel a pending fluff timer if roger despawns/leaves combat.
RegisterCreatureEvent(ENTRY_ROGER, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_ROGER, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
