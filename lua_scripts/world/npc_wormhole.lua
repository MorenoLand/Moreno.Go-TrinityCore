-- Wormhole (Northrend wormhole generator) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1505-1647
-- (npc_wormholeAI). Entry 35646 TDB-verified (ScriptName npc_wormhole).
-- Eluna creature events: 5 OnSpawn; gossip events 1 OnHello, 2 OnSelect.
-- Ported arms (C++-exact for all modeled arms):
-- OnGossipHello (:1545) — IsSummon + GetSummonerUnit() == player gate,
-- then AddGossipItemFor x5-6 (menu 10668) + SendGossipMenuFor(14785).
-- Option texts/icon TDB-verified (gossip_menu_option 10668, icon 0):
-- Borean Tundra / Howling Fjord / Sholazar Basin / Icecrown /
-- Storm Peaks / Underground...; the 6th shows on the 1/101
-- _showUnderground roll (C++ urand(0,100)==0, rolled at spawn).
-- OnGossipSelect (:1567) — action 1001-1006 -> CloseGossipMenuFor +
-- DoCast(player, 67834/67838/67835/67836/67837/68081, non-triggered).
-- The GUID comparison uses GetGUIDLow (packed GUIDs lose bits through
-- Lua numbers).

local ENTRY_WORMHOLE = 35646

local NPC_TEXT_WORMHOLE = 14785
local GOSSIP_SENDER_MAIN = 0
local GOSSIP_ACTION_INFO_DEF = 1000

local WORMHOLE_SPELLS = {
    67834, -- Borean Tundra
    67838, -- Howling Fjord
    67835, -- Sholazar Basin
    67836, -- Icecrown
    67837, -- Storm Peaks
    68081, -- Underground
}

local wormholeState = {}

local function onSpawn(_, creature)
    wormholeState[creature:GetGUIDLow()] = {
        showUnderground = math.random(0, 100) == 0,
    }
end

local function onGossipHello(_, player, creature)
    local summoner = creature:GetSummonerUnit()
    if creature:IsSummon() and summoner ~= nil
        and summoner:GetGUIDLow() == player:GetGUIDLow() then
        player:GossipMenuAddItem(0, "Borean Tundra", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipMenuAddItem(0, "Howling Fjord", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipMenuAddItem(0, "Sholazar Basin", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
        player:GossipMenuAddItem(0, "Icecrown", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 4)
        player:GossipMenuAddItem(0, "Storm Peaks", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 5)
        local guid = creature:GetGUIDLow()
        local st = wormholeState[guid]
        if not st then
            st = {showUnderground = math.random(0, 100) == 0}
            wormholeState[guid] = st
        end
        if st.showUnderground then
            player:GossipMenuAddItem(0, "Underground...", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 6)
        end
        player:GossipSendMenu(NPC_TEXT_WORMHOLE, creature)
    end
    return true
end

local function onGossipSelect(_, player, creature, _, action)
    local spell = WORMHOLE_SPELLS[action - GOSSIP_ACTION_INFO_DEF]
    if spell then
        player:GossipComplete()
        creature:CastSpell(player, spell)
    end
    return true
end

RegisterCreatureEvent(ENTRY_WORMHOLE, 5, onSpawn)
RegisterCreatureGossipEvent(ENTRY_WORMHOLE, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_WORMHOLE, 2, onGossipSelect)
