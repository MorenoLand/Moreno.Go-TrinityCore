-- Stable Master (vanity-pet spell vendor) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:2163-2206
-- (npc_stable_masterAI). 103 TDB entries with ScriptName
-- npc_stable_master (6749, 9896, 9976-9989, 10045-10063, 10085,
-- 11069, 11104, 11105, 11117, 11119, 13616, 15131, 16094, 16185,
-- 16586, 16656, 16764, 16824, 17485, 17666, 17896, 18244, 18250,
-- 18984, 19018, 19325, 19368, 19476, 21336, 21517, 21518, 22468,
-- 22469, 23392, 24066, 24067, 24154, 24350, 24905, 24974, 25037,
-- 25519, 26597, 26721, 27010, 27056, 27065, 27068, 27150, 27183,
-- 27194, 27385, 27948, 28057, 28690, 28790, 29250, 29251, 29658,
-- 29740, 29906, 29948, 29959, 29967, 30008, 30039, 30155, 30304,
-- 33854, 35290, 35291, 35344).
-- Eluna gossip event: 2 OnSelect.
-- Ported arm (C++-exact): OnGossipSelect (:2167) — listID 0-6 ->
-- player:CastSpell(player, 54573/54611/54619/54631/54634/54677/54688,
-- non-triggered self-cast). Menu 9820 is enforced by the engine
-- stale-menu gate (menuID not forwarded to Lua); the hello leg lives
-- in SmartAI, not this script. C++ returns false; the Eluna polarity
-- lets native arms proceed.

local STABLE_MASTER_ENTRIES = {
    6749, 9896, 9976, 9977, 9978, 9979, 9980, 9981, 9982, 9983,
    9984, 9985, 9986, 9987, 9988, 9989, 10045, 10046, 10047, 10048,
    10049, 10050, 10051, 10052, 10053, 10054, 10055, 10056, 10057,
    10058, 10059, 10060, 10061, 10062, 10063, 10085, 11069, 11104,
    11105, 11117, 11119, 13616, 15131, 16094, 16185, 16586, 16656,
    16764, 16824, 17485, 17666, 17896, 18244, 18250, 18984, 19018,
    19325, 19368, 19476, 21336, 21517, 21518, 22468, 22469, 23392,
    24066, 24067, 24154, 24350, 24905, 24974, 25037, 25519, 26597,
    26721, 27010, 27056, 27065, 27068, 27150, 27183, 27194, 27385,
    27948, 28057, 28690, 28790, 29250, 29251, 29658, 29740, 29906,
    29948, 29959, 29967, 30008, 30039, 30155, 30304, 33854, 35290,
    35291, 35344,
}

local STABLE_SPELLS = {
    54573, -- Miniwing
    54611, -- Jubling
    54619, -- Darter
    54631, -- Worg
    54634, -- Smolderweb
    54677, -- Chiken
    54688, -- Wolpertinger
}

local function onGossipSelect(_, player, _, _, _, _, listID)
    local spell = STABLE_SPELLS[listID + 1]
    if spell then
        player:CastSpell(player, spell)
        player:GossipComplete()
    end
    return false
end

for _, entry in ipairs(STABLE_MASTER_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
