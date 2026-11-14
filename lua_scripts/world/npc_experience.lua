-- Experience Eliminator (Slahtz / Behsten, XP on/off) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1666-1712
-- (npc_experienceAI). Entries 35364 'Slahtz' / 35365 'Behsten'
-- TDB-verified (ScriptName npc_experience).
-- Eluna gossip events: 1 OnHello, 2 OnSelect.
-- Ported arms (C++-exact for all modeled arms):
-- OnGossipHello (:1670) — player:HasFlag(150 PLAYER_FLAGS,
-- 0x02000000 NO_XP_GAIN) -> AddGossipItemFor(0, option text, 0,
-- 1001/1002) + SendGossipMenuFor(14736). Option texts/icon
-- TDB-verified (menu 10638: 'I no longer wish to gain experience.' /
-- 'I wish to start gaining experience again.', icon 0).
-- OnGossipSelect (:1685) — action 1001 -> player:RemoveFlag(150,
-- 0x02000000); action 1002 -> player:SetFlag(150, 0x02000000);
-- GossipComplete. C++ returns true here (the handler closes the
-- menu itself).

local EXPERIENCE_ENTRIES = {
    35364, -- Slahtz
    35365, -- Behsten
}

local NPC_TEXT_XP_ON_OFF = 14736
local PLAYER_FLAGS = 150
local PLAYER_FLAGS_NO_XP_GAIN = 0x02000000
local GOSSIP_SENDER_MAIN = 0
local GOSSIP_ACTION_INFO_DEF = 1000

local function onGossipHello(_, player, creature)
    if player:HasFlag(PLAYER_FLAGS, PLAYER_FLAGS_NO_XP_GAIN) then
        player:GossipMenuAddItem(0, "I wish to start gaining experience again.", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    else
        player:GossipMenuAddItem(0, "I no longer wish to gain experience.", GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    end
    player:GossipSendMenu(NPC_TEXT_XP_ON_OFF, creature)
    return true
end

local function onGossipSelect(_, player, _, _, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:RemoveFlag(PLAYER_FLAGS, PLAYER_FLAGS_NO_XP_GAIN)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:SetFlag(PLAYER_FLAGS, PLAYER_FLAGS_NO_XP_GAIN)
    end
    player:GossipComplete()
    return true
end

for _, entry in ipairs(EXPERIENCE_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 1, onGossipHello)
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
