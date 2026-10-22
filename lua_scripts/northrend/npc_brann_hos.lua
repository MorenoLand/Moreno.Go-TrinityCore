-- Halls of Stone: npc_brann_hos — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/halls_of_stone.cpp
-- (npc_brann_hos : public ScriptedAI).
-- Entry: NPC_BRANN_BRONZEBEARD 28070 (halls_of_stone.cpp:93).
-- Ported: OnGossipHello (halls_of_stone.cpp:703-710) -> event 1:
-- "Brann, it would be our honor!" (GOSSIP_ICON_CHAT, sender 1,
-- action 1001) + menu 13100 (TEXT_ID_START). The IsQuestGiver /
-- PrepareQuestMenu arm is documented-only (no quest-menu bridge —
-- kalecgos precedent).
-- OnGossipSelect (halls_of_stone.cpp:686-696) -> event 2: actions
-- 1001/1002 (GOSSIP_ACTION_INFO_DEF+1/+2) close the menu; the
-- StartWP() escort call is unbridged — documented-only.
-- Documented-only: the full escort event machine (tribunal waypoints,
-- Sjonnir intro) — no escort/movement bridge.

local ENTRY_BRANN_HOS = 28070

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ITEM_START = "Brann, it would be our honor!"
local TEXT_ID_START = 13100

local function brannGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_START,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(TEXT_ID_START, creature)
end

local function brannGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1
        or action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipComplete()
    end
end

RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 1, brannGossipHello)
RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 2, brannGossipSelect)
