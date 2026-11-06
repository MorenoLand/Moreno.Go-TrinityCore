-- Soothsaying for Dummies — Lua port of go_soothsaying_for_dummiesAI
-- OnGossipSelect in src/server/scripts/World/npc_professions.cpp:791-846.
-- GO entry 177226, DB menu GOSSIP_ID 7058. Learn arms only: the two
-- unlearn options (listID 0/1) ride ProcessUnlearnAction, whose EquippedOk
-- guard needs player:GetItemByPos (nil stub, lua_player.go:6250), so they
-- stay out and return false like the C++ default (:829).
-- Select hook args are (event, player, go, sender, action, code, listID):
-- listID is the C++ gossipListId (icecrown teleporter precedent). No hello
-- registration: the C++ class has no OnGossipHello, so the native DB menu
-- opens and only the select arm is ported. A Lua false return lets the
-- native select arms proceed, matching the C++ false paths; true vetoes
-- them, matching the handled paths.

local GO_SOOTHSAYING_FOR_DUMMIES = 177226 -- npc_professions.cpp:791

local S_LEARN_GNOMISH = 20220 -- npc_professions.cpp:134-146
local S_LEARN_GOBLIN = 20221
local S_LEARN_DRAGON = 10657
local S_LEARN_ELEMENTAL = 10659
local S_LEARN_TRIBAL = 10661

-- OPTION_LEARN_* (npc_professions.cpp:780-788): 2 gnomish, 3 goblin,
-- 4 dragonscale, 5 elemental, 6 tribal.
local LEARN_SPELLS = {
    [2] = S_LEARN_GNOMISH,
    [3] = S_LEARN_GOBLIN,
    [4] = S_LEARN_DRAGON,
    [5] = S_LEARN_ELEMENTAL,
    [6] = S_LEARN_TRIBAL,
}

local function onSelect(_, player, _, _, _, _, listID)
    local spell = LEARN_SPELLS[listID]
    if spell == nil then
        return false
    end
    player:GossipClearMenu()
    player:CastSpell(player, spell, true) -- C++ :814/:817/:820/:823/:826, all triggered=true
    player:GossipComplete()
    return true
end

RegisterGameObjectGossipEvent(GO_SOOTHSAYING_FOR_DUMMIES, 2, onSelect)
