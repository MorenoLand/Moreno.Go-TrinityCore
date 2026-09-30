-- Zul'Aman zone script — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/zulaman.cpp
-- (npc_zulaman_hostageAI). Hostage entries 23790, 23999, 24024, 24001
-- (C++ HostageEntry[]; AddSC_zulaman registers "npc_zulaman_hostage").
-- Eluna gossip events: 1 OnGossipHello (offers "I am glad to help you."),
-- 2 OnGossipSelect (closes the menu on the GOSSIP_ACTION_INFO_DEF + 1
-- action). No combat AI, no timers; melee is engine-driven in Go.
-- Deviations from C++: the UNIT_NPC_FLAG_GOSSIP flag gate has no bridge
-- (no HasFlag/RemoveFlag on the creature Lua object) — the one-shot is
-- modeled with a per-GUID "released" table, so the option is not offered
-- again after a select; instance->SetData(DATA_CHESTLOOTED, 0) has no
-- bearer (no instance-script model); SummonGameObject(ChestEntry[i])
-- (186648 / 187021 / 186672 / 186667 per hostage entry) has no bearer
-- (no summon/gameobject-spawn bridge) — no chest is spawned; player->
-- GetGossipTextId has no bridge, so the menu title id is 0.
-- The npc_harrison_jones script from the same C++ file is NOT registered:
-- its OnGossipSelect gates on the creature's GossipMenuId and its entire
-- gong-event machine is MovePath/MovePoint waypoint work (no movement
-- model), SetEntry/SetDisplayId/SetFacingTo/equipment/emote arms (no
-- bridge), instance GetGameObject/SetData arms (no instance-script
-- model), and GetCreatureListWithEntryInGrid/SetImmuneToPC/AI SetData
-- arms for the Amanishi guardians (no bridge); its SpellHit arm fires
-- only for the creature-cast SPELL_COSMETIC_SPEAR_THROW 43647, while the
-- engine's event 14 fires only for player casts.

local HOSTAGE_ENTRIES = {23790, 23999, 24024, 24001}

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_HOSTAGE1 = "I am glad to help you."

-- Per-GUID one-shot gate: stands in for the C++ UNIT_NPC_FLAG_GOSSIP
-- flag removal (no flag bridge on the creature object).
local released = {}

local function onHello(event, player, creature)
    local guid = creature:GetGUID()
    if not released[guid] then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HOSTAGE1, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(0, creature)
end

local function onSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
    end
    -- C++-exact order: the close arms run first; the rest has no bearer.
    released[creature:GetGUID()] = true
end

for _, entry in ipairs(HOSTAGE_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 1, onHello)
    RegisterCreatureGossipEvent(entry, 2, onSelect)
end
