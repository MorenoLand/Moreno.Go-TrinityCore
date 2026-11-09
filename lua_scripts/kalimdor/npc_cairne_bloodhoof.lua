-- Thunder Bluff: Cairne Bloodhoof — combat rotation port of
-- src/server/scripts/Kalimdor/zone_thunder_bluff.cpp
-- (npc_cairne_bloodhoofAI : public ScriptedAI; registered in
-- AddSC_thunder_bluff(), kalimdor loader decl :124 / call :237).
-- Entry (wowhead WotLK-verified 2026-10-18): 3057 — "Cairne
-- Bloodhoof <High Chieftain>", Thunder Bluff (High Rise);
-- https://www.wowhead.com/wotlk/npc=3057/cairne-bloodhoof. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace).
-- Ported arms (C++-exact, UpdateAI arm order):
-- - JustEngagedWith -> DoPlaySoundToSet(me, SOUND_AGGRO 5884):
--   creature:PlayDirectSound(5884) (SMSG_PLAY_SOUND, bosses_opera
--   precedent).
-- - Berserker charge: SelectTarget(Random, 0) -> nil-guarded random
--   alive pick over creature:GetAITargets() (omen idiom, jaraxxus
--   without the player filter — C++ playerOnly defaults false) +
--   DoCast(target, SPELL_BERSERKER_CHARGE 16636) non-triggered
--   (2-arg DoCast, vaelastrasz convention), init 30000 -> 25000.
-- - Uppercut: DoCastVictim(SPELL_UPPERCUT 22916) non-triggered,
--   init 10000 -> 20000. Thunderclap: DoCastVictim(SPELL_THUNDERCLAP
--   23931) non-triggered, init 15000 -> 15000. Mortal strike:
--   DoCastVictim(SPELL_MORTAL_STRIKE 16856) non-triggered, init
--   10000 -> 15000. Cleave: DoCastVictim(SPELL_CLEAVE 16044)
--   non-triggered, init 5000 -> 7000. Victim-cast arms ride the C++
--   !UpdateVictim() gate via a GetVictim() nil guard (overgrind
--   precedent); timers via CreateLuaEvent, armed on enter combat,
--   cancelled on leave combat / died / reset (timers tick only in
--   combat in the C++).
-- - DoMeleeAttackIfReady — engine-driven in Go (ouro precedent).
-- Ported gossip arms (quest 925 "A Sacred Burial" hoofprint menu,
-- zone_thunder_bluff.cpp OnGossipHello/OnGossipSelect):
-- - OnGossipHello (event 1): quest 925 INCOMPLETE (GetQuestStatus
--   bridged) -> GossipMenuAddItem(chat icon, hoofprint text, sender
--   MAIN 1, action INFO 3) + GossipSendMenu(7013) (taretha idiom);
--   otherwise return false so the default menu path serves quest
--   items (prepareCreatureGossip). PrepareQuestMenu has no Lua
--   method, so the C++ merge of quest items + menu 7013 cannot be
--   reproduced: 925 players get menu 7013 + the item but no quest
--   items, non-925 players get the default menu instead of menu
--   7013 text (documented deviation). The old documented-only
--   rationale (GetQuestRewardStatus + no player-self-cast bridge)
--   is retracted: hello uses only GetQuestStatus, and
--   player:CastSpell is bridged (lua_player.go).
-- - OnGossipSelect (event 2): ClearGossipMenu first (C++ order);
--   action INFO 3 -> player self-cast 23123 (non-triggered, C++
--   CastSpell(player, 23123, false)) + GossipSendMenu(7014);
--   other actions -> nothing further (C++ returns true after the
--   clear). Select args are (event, player, creature, sender,
--   action, code, listID); the C++ action comes from
--   GetGossipOptionAction(gossipListId), which Go forwards as
--   item.Action.

local SPELL_BERSERKER_CHARGE = 16636
local SPELL_CLEAVE = 16044
local SPELL_MORTAL_STRIKE = 16856
local SPELL_THUNDERCLAP = 23931
local SPELL_UPPERCUT = 22916

local SOUND_AGGRO = 5884

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

local function castOnVictim(creature, guid, key, spell, rearm)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
    schedule(guid, key, rearm, function()
        castOnVictim(creature, guid, key, spell, rearm)
    end)
end

-- C++ UpdateAI arm order: berserker charge, uppercut, thunderclap,
-- mortal strike, cleave.
local function onBerserkerCharge(creature, guid)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_BERSERKER_CHARGE)
    end
    schedule(guid, "charge", 25000, function()
        onBerserkerCharge(creature, guid)
    end)
end

local ENTRY = 3057

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- zone_thunder_bluff.cpp JustEngagedWith: DoPlaySoundToSet(me, 5884).
    creature:PlayDirectSound(SOUND_AGGRO)
    schedule(guid, "charge", 30000, function()
        onBerserkerCharge(creature, guid)
    end)
    schedule(guid, "uppercut", 10000, function()
        castOnVictim(creature, guid, "uppercut", SPELL_UPPERCUT, 20000)
    end)
    schedule(guid, "thunderclap", 15000, function()
        castOnVictim(creature, guid, "thunderclap", SPELL_THUNDERCLAP, 15000)
    end)
    schedule(guid, "mortal", 10000, function()
        castOnVictim(creature, guid, "mortal", SPELL_MORTAL_STRIKE, 15000)
    end)
    schedule(guid, "cleave", 5000, function()
        castOnVictim(creature, guid, "cleave", SPELL_CLEAVE, 7000)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

local QUEST_A_SACRED_BURIAL = 925
local QUEST_STATUS_INCOMPLETE = 3
local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_SENDER_INFO = 3
local GOSSIP_MENU_HELLO = 7013
local GOSSIP_MENU_SELECT = 7014
local SPELL_HOOFPRINT = 23123
local GOSSIP_HCB = "I know this is rather silly but a young ward who is a bit shy would like your hoofprint."

local function onGossipHello(event, player, creature)
    if player:GetQuestStatus(QUEST_A_SACRED_BURIAL) == QUEST_STATUS_INCOMPLETE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HCB, GOSSIP_SENDER_MAIN, GOSSIP_SENDER_INFO)
        player:GossipSendMenu(GOSSIP_MENU_HELLO, creature)
    else
        return false
    end
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_SENDER_INFO then
        player:CastSpell(player, SPELL_HOOFPRINT)
        player:GossipSendMenu(GOSSIP_MENU_SELECT, creature)
    end
    return true
end

RegisterCreatureGossipEvent(ENTRY, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
