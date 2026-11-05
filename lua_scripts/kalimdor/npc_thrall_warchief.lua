-- Orgrimmar: Thrall, Warchief of the Horde — combat rotation + quest-6566
-- gossip chain port of src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (npc_thrall_warchiefAI : public ScriptedAI; registered in
-- AddSC_orgrimmar(), kalimdor loader decl :113 / call :226).
-- Entry (wowhead WotLK-verified 2026-10-18): 4949 — "Thrall
-- <Warchief>", Orgrimmar (Grommash Hold, Valley of Wisdom);
-- https://www.wowhead.com/wotlk/npc=4949/thrall. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace).
-- Ported arms (C++-exact):
-- - JustEngagedWith -> DoPlaySoundToSet(me, SOUND_AGGRO 5880):
--   creature:PlayDirectSound(5880) (SMSG_PLAY_SOUND, bosses_opera
--   precedent).
-- - Chain lightning: DoCastVictim(SPELL_CHAIN_LIGHTNING 16033),
--   non-triggered (2-arg DoCast, vaelastrasz convention), init 2000
--   -> 9000. Shock: DoCastVictim(SPELL_SHOCK 16034), non-triggered,
--   init 8000 -> 15000. Both gated on creature:GetVictim() (the C++
--   !UpdateVictim() early return); timers via CreateLuaEvent, armed
--   on enter combat, cancelled on leave combat / died / reset
--   (timers tick only in combat in the C++).
-- - DoMeleeAttackIfReady — engine-driven in Go (ouro precedent).
-- - OnGossipHello/OnGossipSelect (quest-6566 "What the Wind
--   Carries" 7-step menu chain, zone_orgrimmar.cpp:224-317):
--   hello offers the 3664 item only while GetQuestStatus(6566) is
--   INCOMPLETE (3), after the PrepareQuestMenu twin
--   (player:GossipAddQuests, lokhtos precedent) behind the
--   IsQuestGiver guard, then sends npc_text 4477; each select
--   clears the menu first (C++ ClearGossipMenuFor), adds the next
--   option with action INFO_DEF+1..7 and sends npc_text
--   5733..5738 in order, and the last step closes the menu then
--   calls player:AreaExploredOrEventHappens(6566) (C++ order;
--   bridged, lua_player.go:4328 — the old "no
--   AreaExploredOrEventHappens bridge" claim was stale).
--   Option texts are DB-side (gossip_menu_option rows, menus
--   3664-3670; world.sql is schema-only in-tree) and were
--   browser-verified 2026-11-05 against the wowhead WotLK quest
--   page (quest=6566 comment, 2008-07-25) corroborated by the
--   wowwiki quest dialogue transcript, same order and wording;
--   the TDB rows themselves were not directly inspected. Icon is
--   GOSSIP_ICON_CHAT (0, taretha/lokhtos precedent) — the DB
--   OptionIcon values are unverified. Both handlers return true
--   (lokhtos precedent: Lua true vetoes the native arms).
-- Retired documented-only notes (all ported since): npc_shenthul
-- (quest 2460) lives in npc_shenthul.lua (entry 3401, registered);
-- npc_overlord_runthak_orgrimmar (citizen scene) lives in
-- npc_overlord_runthak_orgrimmar.lua (entry 31431, tick 229b);
-- npc_thrall_herald_of_war (scene machine) lives in
-- npc_thrall_herald_of_war.lua (tick 229).
-- Documented-only (no bridge):
-- - areatrigger_orgrimmar_herald_of_war: no area-trigger bridge.

local SPELL_CHAIN_LIGHTNING = 16033
local SPELL_SHOCK = 16034

local SOUND_AGGRO = 5880

local QUEST_WHAT_THE_WIND_CARRIES = 6566
local QUEST_STATUS_INCOMPLETE = 3

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000

local GOSSIP_MENU_MEMBERS_OF_THE_HORDE = 4477
local GOSSIP_MENU_THE_SHATTERED_HAND = 5733
local GOSSIP_MENU_IT_WOULD_APPEAR_AS = 5734
local GOSSIP_MENU_THE_BROOD_MOTHER = 5735
local GOSSIP_MENU_SO_MUCH_TO_LEARN = 5736
local GOSSIP_MENU_I_DO_NOT_FAULT_YOU = 5737
local GOSSIP_MENU_NOW_PAY_ATTENTION = 5738

local OPTION_PLEASE_SHARE_YOUR = "Please share your wisdom with me, Warchief."
local OPTION_WHAT_DISCOVERIES = "What discoveries?"
local OPTION_USURPER = "Usurper?"
local OPTION_WITH_ALL_DUE_RESPECT = "With all due respect, Warchief - why not allow them to be destroyed? Does this not strengthen our position?"
local OPTION_I_I_DID_NOT_THINK_OF = "I... I did not think of it that way, Warchief."
local OPTION_I_LIVE_ONLY_TO_SERVE = "I live only to serve, Warchief! My life is empty and meaningless without your guidance."
local OPTION_OF_COURSE_WARCHIEF = "Of course, Warchief."

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ UpdateAI arm order: chain lightning before shock.
local function onChainLightning(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain", 9000, function()
        onChainLightning(creature, guid)
    end)
end

local function onShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOCK)
    end
    schedule(guid, "shock", 15000, function()
        onShock(creature, guid)
    end)
end

local ENTRY = 4949

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- zone_orgrimmar.cpp JustEngagedWith: DoPlaySoundToSet(me, 5880).
    creature:PlayDirectSound(SOUND_AGGRO)
    schedule(guid, "chain", 2000, function()
        onChainLightning(creature, guid)
    end)
    schedule(guid, "shock", 8000, function()
        onShock(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ OnGossipHello (zone_orgrimmar.cpp:297-306): quest menu first
-- when the creature is a quest giver, then the 3664 item only while
-- quest 6566 is INCOMPLETE, then npc_text 4477.
local function onGossipHello(event, player, creature)
    if creature:IsQuestGiver() then
        player:GossipAddQuests(creature)
    end
    if player:GetQuestStatus(QUEST_WHAT_THE_WIND_CARRIES) == QUEST_STATUS_INCOMPLETE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_PLEASE_SHARE_YOUR,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(GOSSIP_MENU_MEMBERS_OF_THE_HORDE, creature)
    return true
end

-- C++ OnGossipSelect (zone_orgrimmar.cpp:224-295): the menu is
-- cleared first; each step adds the next option (action +1) and
-- sends the next npc_text; the last step closes the menu and
-- completes quest 6566.
local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_WHAT_DISCOVERIES,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(GOSSIP_MENU_THE_SHATTERED_HAND, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_USURPER,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
        player:GossipSendMenu(GOSSIP_MENU_IT_WOULD_APPEAR_AS, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_WITH_ALL_DUE_RESPECT,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 4)
        player:GossipSendMenu(GOSSIP_MENU_THE_BROOD_MOTHER, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 4 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_I_I_DID_NOT_THINK_OF,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 5)
        player:GossipSendMenu(GOSSIP_MENU_SO_MUCH_TO_LEARN, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 5 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_I_LIVE_ONLY_TO_SERVE,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 6)
        player:GossipSendMenu(GOSSIP_MENU_I_DO_NOT_FAULT_YOU, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 6 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, OPTION_OF_COURSE_WARCHIEF,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 7)
        player:GossipSendMenu(GOSSIP_MENU_NOW_PAY_ATTENTION, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 7 then
        player:GossipComplete()
        player:AreaExploredOrEventHappens(QUEST_WHAT_THE_WIND_CARRIES)
    end
    return true
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureGossipEvent(ENTRY, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
