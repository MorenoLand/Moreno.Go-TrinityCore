-- npc_engineering_tele_trinket -- Lua port of
-- src/server/scripts/World/npc_professions.cpp
-- (npc_engineering_tele_trinket :673-789).
-- Entries (in-file enum): 14742 Zap, 14743 Jhordy, 21493 Kablam,
-- 21494 Smiles. ScriptName binding is DB-side as usual.
-- Ported arms (C++-exact except the two noted deviations):
-- - OnGossipHello: HasSkill(ENGINEERING) gate, then per-entry
--   CanLearn (:692): GetSkillValue(202) >= 260 (Zap/Jhordy) or >= 350
--   (Kablam/Smiles) + HasSpell(specialization) + not HasSpell(teleport
--   spell) -> gossip item (sender = creature entry, action = 1001).
--   Menu text: the entry text id, or the alt text id when the player
--   already knows the teleport (Jhordy only, 7252), else
--   player:GetGossipTextId(creature) — the C++ npcTextId ternary.
-- - OnGossipSelect: ClearGossipMenu; action 1001 closes the menu;
--   sender != creature entry returns; otherwise the player casts the
--   learn spell on self: Zap 23490 (teaches 23486 Everlook), Jhordy
--   23491 (teaches 23489 Gadgetzan), Kablam 36956 (teaches 36954
--   Area 52), Smiles 36957 (teaches 36955 Toshley's Station).
-- Deviations: (1) C++ GetBaseSkillValue = stored + perm bonus, Go
-- GetSkillValue returns the stored value, so the skill gate sits low
-- only by any perm bonus (blacksmith precedent). (2) Lua
-- player:CastSpell drops the triggered flag; the learn-spell effect
-- executes, cast-bar/power legs stay unmodeled.
-- Retires the documented-only note for this class in
-- lua_scripts/world/npc_professions_blacksmith.lua.

local SKILL_ENGINEERING = 202
local GOSSIP_ICON_CHAT = 0
local GOSSIP_ACTION_INFO_DEF = 1000

local S_GOBLIN = 20222
local S_GNOMISH = 20219

local TELE = {
    [14742] = { text = 6092, alt = 0, skill = 260, req = S_GOBLIN, spell = 23486, learn = 23490,
        item = "This Dimensional Imploder sounds dangerous! How can I make one?" },
    [14743] = { text = 7251, alt = 7252, skill = 260, req = S_GNOMISH, spell = 23489, learn = 23491,
        item = "I must build a beacon for this marvelous device!" },
    [21493] = { text = 10365, alt = 0, skill = 350, req = S_GOBLIN, spell = 36954, learn = 36956,
        item = "[PH] Unknown" },
    [21494] = { text = 10363, alt = 0, skill = 350, req = S_GNOMISH, spell = 36955, learn = 36957,
        item = "[PH] Unknown" },
}

local function onGossipHello(event, player, creature)
    local entry = creature:GetEntry()
    local cfg = TELE[entry]
    local npcTextId = 0
    local canLearn = false
    if cfg and player:HasSkill(SKILL_ENGINEERING) then
        npcTextId = cfg.text
        if player:GetSkillValue(SKILL_ENGINEERING) >= cfg.skill and player:HasSpell(cfg.req) then
            if not player:HasSpell(cfg.spell) then
                canLearn = true
            else
                npcTextId = cfg.alt
            end
        end
        if canLearn then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, cfg.item, entry, GOSSIP_ACTION_INFO_DEF + 1)
        end
    end
    if npcTextId == 0 then
        npcTextId = player:GetGossipTextId(creature)
    end
    player:GossipSendMenu(npcTextId, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
    end
    if sender ~= creature:GetEntry() then
        return
    end
    local cfg = TELE[sender]
    if cfg then
        player:CastSpell(player, cfg.learn, false)
    end
end

for _, entry in ipairs({ 14742, 14743, 21493, 21494 }) do
    RegisterCreatureGossipEvent(entry, 1, onGossipHello)
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
