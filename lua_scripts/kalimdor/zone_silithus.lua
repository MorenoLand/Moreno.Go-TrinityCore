-- Silithus: Wind Stones — Lua port of
-- src/server/scripts/Kalimdor/zone_silithus.cpp (class go_wind_stone :
-- GameObjectScript("go_wind_stone") { go_wind_stoneAI : GameObjectAI };
-- AddSC_silithus registers it; kalimdor loader decl 119 / call 232 per
-- kalimdor_script_loader.cpp).
--
-- TDB grounding (TDB335.26091): gameobject_template carries 12 rows with
-- ScriptName 'go_wind_stone' — the "TDB-blocked, zero rows" verdict in the
-- checkpoint was WRONG (same grep-miss class as the spell_q* rename
-- error: the check missed the gameobject_template rows). Entries by tier:
--   Lesser Wind Stone (menu 6540): 180456/180518/180529/180544/180549/180564
--   Wind Stone (menu 6542): 180461/180534/180554
--   Greater Wind Stone (menu 6543): 180466/180539/180559
-- The tier is keyed off the GO entry here, 1:1 with the menu id (Data0) on
-- every TDB row; C++ reads the menu id off the GOInfo instead.
-- gossip_menu_option texts (6540/6542/6543, option 0-4) and npc_text
-- titles are quoted from TDB 26091 below; the title id replicates
-- Player::GetGossipTextId (Player.cpp) — it takes the LAST gossip_menu
-- row whose conditions rows all pass (conditions SourceType 14):
--   6540: has 24746 -> 7771, else 7744
--   6542: has 24746+24748 -> 7773, has 24746 only -> 7772, else 7749
--   6543: all three -> 7774, 24746+24748 -> 7775, 24746 only -> 7776, else 7754
-- creature_text group 0 (4 texts per entry, SAY type 12 templar/duke,
-- YELL type 14 royal) is played by Talk(0, player) — the Go talk path
-- picks a random row in the group, like CreatureAI::Talk.
--
-- Ported arms (zone_silithus.cpp:1141-1392):
-- - OnGossipHello: rank from auras (24746 set / 24748 medallion /
--   24782 ring) gates the random + named options per tier; items from
--   HasItemCount on the four crest/signet/scepter ids per tier.
-- - OnGossipSelect: action INFO_DEF+1..15 summons the templar/duke/royal
--   at the stone, facing away from the player (player orientation - pi).
-- - SummonNPC: GO-position summon, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN
--   (C++ 1 -> Go 0, arlokk -1 scheme), 10min = 600000ms; self-cast
--   SPELL_SPAWN_IN 25035 non-triggered; Talk group 0 to the player;
--   RemoveFlag UNIT_FIELD_FLAGS/UNIT_FLAG_NON_ATTACKABLE; EngageWithTarget.
-- Unported (documented-only, no bridge): the two GO casts — the summon
-- trigger spell (24745/24747/... per tier, go->CastSpell(player, spell))
-- and SPELL_PUNISHMENT 24803 on the rank-fail branch — go:CastSpell has
-- no bridge (same open gap as the outhouse select in go_scripts_part1.lua).
-- Creature Reset legs (zone_silithus.cpp, round-72 verified): the
-- qiraj_war_spawn Reset (Initialize-only, :698-701) is PORTED in
-- npc_qiraj_war_spawn.lua onReset (ev23, entries 15414/15422/15423/
-- 15424); anachronos (15381, :362-366) + quest-trigger (15454,
-- :831-835) Resets STAY OUT — init legs of unported machines
-- (HandleAnimation 20-step cinematic / SummonNextWave wave machine).
-- The scanner row keeps surfacing (mixed row, deliberately unaliased).
-- The summon itself is NOT the trigger spell: SummonCreature spawns the
-- NPC, so the core (stone summons a templar/duke/royal that yells and
-- attacks) is intact; only the GO-cast spell effects are missing.
-- The spell_silithus_summon_cultist_periodic AuraScript (:1395+) is a
-- separate class, not part of this port.

local AURA_TWILIGHT_SET = 24746
local AURA_MEDALLION = 24748
local AURA_RING = 24782

local SPELL_SPAWN_IN = 25035

local SUMMON_TIMED_OR_DEAD_DESPAWN = 0 -- C++ TEMPSUMMON_TIMED_OR_DEAD_DESPAWN = 1, Go scheme -1
local SUMMON_DESPAWN_MS = 600000 -- 10min

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000

local MENU_LESSER = 6540
local MENU_WIND = 6542
local MENU_GREATER = 6543

local TITLE_LESSER_RANKED = 7771
local TITLE_LESSER_UNRANKED = 7744
local TITLE_WIND_RANK2 = 7773
local TITLE_WIND_RANK1 = 7772
local TITLE_WIND_UNRANKED = 7749
local TITLE_GREATER_RANK3 = 7774
local TITLE_GREATER_RANK2 = 7775
local TITLE_GREATER_RANK1 = 7776
local TITLE_GREATER_UNRANKED = 7754

-- gossip_menu_option texts, TDB 26091 (option 0 = random challenge).
local TEXT_LESSER = {
    "I am no cultist, you monster!  Come to me and face your destruction!",
    "Crimson Templar!  I hold your crest of beckoning.  Heed my summons!",
    "Azure Templar!  I hold your crest of beckoning.  Heed my summons!",
    "Earthen Templar!  I hold your crest of beckoning.  Heed my summons!",
    "Hoary Templar!  I hold your crest of beckoning.  Heed my summons!",
}
local TEXT_WIND = {
    "You will listen to this, vile duke!  I am not your Twilight's Hammer lapdog!  I am here to challenge you!  Come!  Come, and meet your death...",
    "Duke of Cynders! I hold your signet!  Heed my call!",
    "Duke of Fathoms! I hold your signet!  Heed my call!",
    "Duke of Shards! I hold your signet!  Heed my call!",
    "Duke of Zephyrs! I hold your signet!  Heed my call!",
}
local TEXT_GREATER = {
    "The day of judgement has come, fiend!  I challenge you to battle!",
    "Skaldrenox!  I hold your scepter, and charge you to enter this world!",
    "Skwol!  I hold your scepter!  I command you to enter this world!",
    "Kazum!  I hold your scepter!  I command you to enter this world!",
    "Whirlaxis!  I hold your scepter!  I command you to enter this world!",
}

-- Tier entries (TDB 26091 gameobject_template, ScriptName go_wind_stone).
local ENTRIES_LESSER = { 180456, 180518, 180529, 180544, 180549, 180564 }
local ENTRIES_WIND = { 180461, 180534, 180554 }
local ENTRIES_GREATER = { 180466, 180539, 180559 }

-- WSCreatures (zone_silithus.cpp): fire/water/air/earth per tier.
local NPC_TEMPLAR = { 15209, 15211, 15212, 15307 }
local NPC_DUKE = { 15206, 15207, 15220, 15208 }
local NPC_ROYAL = { 15203, 15305, 15204, 15205 }
-- WSSpells trigger spells, same fire/water/air/earth order; index 0 is
-- the random-summon trigger (RAND in C++ picks one of the four NPCs).
local SPELL_TEMPLAR = { 24747, 24761, 24757, 24759 }
local SPELL_DUKE = { 24766, 24773, 24769, 24771 }
local SPELL_ROYAL = { 24787, 24793, 24791, 24792 }
local SPELL_TEMPLAR_RANDOM = 24745
local SPELL_DUKE_RANDOM = 24762
local SPELL_ROYAL_RANDOM = 24785
-- WSItems: crest (templar) / signet (duke) / scepter (royal) ids.
local ITEM_TEMPLAR = { 20416, 20420, 20418, 20419 }
local ITEM_DUKE = { 20432, 20436, 20433, 20435 }
local ITEM_ROYAL = { 20447, 20450, 20448, 20449 }

local FIRE = 0x1
local WATER = 0x2
local EARTH = 0x4
local AIR = 0x8

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

-- C++ GetPlayerRank (zone_silithus.cpp:1152-1165): set+medallion+ring
-- -> 3, set+medallion -> 2, set -> 1, else 0.
local function getPlayerRank(player)
    local hasSet = player:HasAura(AURA_TWILIGHT_SET)
    local hasMedallion = player:HasAura(AURA_MEDALLION)
    local hasRing = player:HasAura(AURA_RING)
    if hasSet and hasMedallion and hasRing then
        return 3
    elseif hasSet and hasMedallion then
        return 2
    elseif hasSet then
        return 1
    end
    return 0
end

-- C++ GetItems (zone_silithus.cpp:1167-1214): bitmask of held
-- crest/signet/scepter ids for the tier (HasItemCount default count 1).
-- Tables are ordered fire/water/air/earth, matching the NPC/spell tables.
local function getItems(player, items)
    local result = 0
    if player:GetItemCount(items[1]) > 0 then
        result = result + FIRE
    end
    if player:GetItemCount(items[2]) > 0 then
        result = result + WATER
    end
    if player:GetItemCount(items[3]) > 0 then
        result = result + AIR
    end
    if player:GetItemCount(items[4]) > 0 then
        result = result + EARTH
    end
    return result
end

-- C++ title id for the menu, replicating GetGossipTextId's
-- last-passing-conditions-row rule (see header).
local function titleFor(menu, rank)
    if menu == MENU_LESSER then
        if rank >= 1 then
            return TITLE_LESSER_RANKED
        end
        return TITLE_LESSER_UNRANKED
    elseif menu == MENU_WIND then
        if rank >= 2 then
            return TITLE_WIND_RANK2
        elseif rank == 1 then
            return TITLE_WIND_RANK1
        end
        return TITLE_WIND_UNRANKED
    else
        if rank == 3 then
            return TITLE_GREATER_RANK3
        elseif rank == 2 then
            return TITLE_GREATER_RANK2
        elseif rank == 1 then
            return TITLE_GREATER_RANK1
        end
        return TITLE_GREATER_UNRANKED
    end
end

local function tierForEntry(entry)
    for _, e in ipairs(ENTRIES_LESSER) do
        if e == entry then
            return MENU_LESSER
        end
    end
    for _, e in ipairs(ENTRIES_WIND) do
        if e == entry then
            return MENU_WIND
        end
    end
    for _, e in ipairs(ENTRIES_GREATER) do
        if e == entry then
            return MENU_GREATER
        end
    end
    return nil
end

-- C++ OnGossipHello (zone_silithus.cpp:1263-1330): rank-gated options
-- per tier; the rank-fail branch casts SPELL_PUNISHMENT (GO-cast,
-- unported — see header) and adds no items, but the menu still sends
-- with the conditions-derived title. Returns true like C++ (handled).
local function onWindStoneGossip(_, player, go)
    local menu = tierForEntry(go:GetEntry())
    if menu == nil then
        return true
    end
    local rank = getPlayerRank(player)
    if menu == MENU_LESSER then
        if rank >= 1 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_LESSER[1], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
            local item = getItems(player, ITEM_TEMPLAR)
            if item % (FIRE * 2) >= FIRE then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_LESSER[2], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
            end
            if item % (WATER * 2) >= WATER then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_LESSER[3], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
            end
            if item % (EARTH * 2) >= EARTH then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_LESSER[4], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 4)
            end
            if item % (AIR * 2) >= AIR then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_LESSER[5], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 5)
            end
        end
        -- rank 0: punishment leg unported (no go:CastSpell); menu sends
        -- with the unranked title and no items, like C++.
    elseif menu == MENU_WIND then
        if rank >= 2 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_WIND[1], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 6)
            local item = getItems(player, ITEM_DUKE)
            if item % (FIRE * 2) >= FIRE then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_WIND[2], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 7)
            end
            if item % (WATER * 2) >= WATER then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_WIND[3], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 8)
            end
            if item % (EARTH * 2) >= EARTH then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_WIND[4], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 9)
            end
            if item % (AIR * 2) >= AIR then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_WIND[5], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 10)
            end
        end
    else
        if rank == 3 then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_GREATER[1], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 11)
            local item = getItems(player, ITEM_ROYAL)
            if item % (FIRE * 2) >= FIRE then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_GREATER[2], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 12)
            end
            if item % (WATER * 2) >= WATER then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_GREATER[3], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 13)
            end
            if item % (EARTH * 2) >= EARTH then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_GREATER[4], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 14)
            end
            if item % (AIR * 2) >= AIR then
                player:GossipMenuAddItem(GOSSIP_ICON_CHAT, TEXT_GREATER[5], GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 15)
            end
        end
    end
    player:GossipSendMenu(titleFor(menu, rank), go)
    return true
end

-- C++ SummonNPC (zone_silithus.cpp:1216-1251): the GO-cast trigger
-- spell leg is unported (no go:CastSpell — see header); the summon,
-- spawn-in cast, aggro Talk, flag clear and engage are exact.
local function summonNPC(go, player, npc, _spell)
    local summons = go:SummonCreature(npc, go:GetX(), go:GetY(), go:GetZ(), player:GetO() - math.pi, SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
    if summons == nil then
        return
    end
    summons:CastSpell(summons, SPELL_SPAWN_IN)
    summons:Talk(0, player)
    summons:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    summons:EngageWithTarget(player)
end

-- C++ OnGossipSelect (zone_silithus.cpp:1332-1387): switches on the
-- stored action (GetGossipOptionAction); Go forwards sender/action/
-- code/listID, so the action is read directly. RAND picks uniformly
-- among the four tier NPCs. Returns true (handled, like C++).
local function onWindStoneSelect(_, player, go, _, action)
    player:GossipClearMenu()
    player:GossipComplete()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        local pick = NPC_TEMPLAR[math.random(1, 4)]
        summonNPC(go, player, pick, SPELL_TEMPLAR_RANDOM)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        summonNPC(go, player, NPC_TEMPLAR[1], SPELL_TEMPLAR[1])
    elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
        summonNPC(go, player, NPC_TEMPLAR[2], SPELL_TEMPLAR[2])
    elseif action == GOSSIP_ACTION_INFO_DEF + 4 then
        summonNPC(go, player, NPC_TEMPLAR[4], SPELL_TEMPLAR[4])
    elseif action == GOSSIP_ACTION_INFO_DEF + 5 then
        summonNPC(go, player, NPC_TEMPLAR[3], SPELL_TEMPLAR[3])
    elseif action == GOSSIP_ACTION_INFO_DEF + 6 then
        local pick = NPC_DUKE[math.random(1, 4)]
        summonNPC(go, player, pick, SPELL_DUKE_RANDOM)
    elseif action == GOSSIP_ACTION_INFO_DEF + 7 then
        summonNPC(go, player, NPC_DUKE[1], SPELL_DUKE[1])
    elseif action == GOSSIP_ACTION_INFO_DEF + 8 then
        summonNPC(go, player, NPC_DUKE[2], SPELL_DUKE[2])
    elseif action == GOSSIP_ACTION_INFO_DEF + 9 then
        summonNPC(go, player, NPC_DUKE[4], SPELL_DUKE[4])
    elseif action == GOSSIP_ACTION_INFO_DEF + 10 then
        summonNPC(go, player, NPC_DUKE[3], SPELL_DUKE[3])
    elseif action == GOSSIP_ACTION_INFO_DEF + 11 then
        local pick = NPC_ROYAL[math.random(1, 4)]
        summonNPC(go, player, pick, SPELL_ROYAL_RANDOM)
    elseif action == GOSSIP_ACTION_INFO_DEF + 12 then
        summonNPC(go, player, NPC_ROYAL[1], SPELL_ROYAL[1])
    elseif action == GOSSIP_ACTION_INFO_DEF + 13 then
        summonNPC(go, player, NPC_ROYAL[2], SPELL_ROYAL[2])
    elseif action == GOSSIP_ACTION_INFO_DEF + 14 then
        summonNPC(go, player, NPC_ROYAL[4], SPELL_ROYAL[4])
    elseif action == GOSSIP_ACTION_INFO_DEF + 15 then
        summonNPC(go, player, NPC_ROYAL[3], SPELL_ROYAL[3])
    end
    return true
end

for _, entry in ipairs(ENTRIES_LESSER) do
    RegisterGameObjectGossipEvent(entry, 1, onWindStoneGossip)
    RegisterGameObjectGossipEvent(entry, 2, onWindStoneSelect)
end
for _, entry in ipairs(ENTRIES_WIND) do
    RegisterGameObjectGossipEvent(entry, 1, onWindStoneGossip)
    RegisterGameObjectGossipEvent(entry, 2, onWindStoneSelect)
end
for _, entry in ipairs(ENTRIES_GREATER) do
    RegisterGameObjectGossipEvent(entry, 1, onWindStoneGossip)
    RegisterGameObjectGossipEvent(entry, 2, onWindStoneSelect)
end
