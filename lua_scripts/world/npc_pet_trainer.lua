-- Pet Trainer (hunter pet talent unlearn) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1628-1661
-- (npc_pet_trainerAI). 20 TDB entries with ScriptName npc_pet_trainer
-- (543, 2878, 2879, 3306, 3545, 3620, 3622, 3624, 3688, 3698, 4320,
-- 10086, 10088, 10089, 10090, 16271, 16675, 16712, 17484, 40405).
-- Eluna gossip event: 2 OnSelect.
-- Ported arm (C++-exact): OnGossipSelect (:1632) — listID 0 ->
-- player:ResetPetTalents() + CloseGossipMenuFor. Menu 6520 option 0
-- 'Yes, please do.' (TDB; gossip_menu 6520 textID 7722). The menuId ==
-- 6520 gate is enforced by the engine's post-hook stale-menu gate
-- (gossip.go:395-400 — menuID intentionally not forwarded to Lua),
-- so the Lua handler gates on listID only. The hello leg lives in DB
-- gossip (menu 6520), not this script. C++ returns false; the Eluna
-- polarity lets native arms proceed.

local PET_TRAINER_ENTRIES = {
    543, 2878, 2879, 3306, 3545, 3620, 3622, 3624, 3688, 3698,
    4320, 10086, 10088, 10089, 10090, 16271, 16675, 16712, 17484, 40405,
}

local OPTION_ID_PLEASE_DO = 0

local function onGossipSelect(_, player, _, _, _, _, listID)
    if listID == OPTION_ID_PLEASE_DO then
        player:ResetPetTalents()
        player:GossipComplete()
    end
    return false
end

for _, entry in ipairs(PET_TRAINER_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
