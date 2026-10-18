-- Zul'Aman zone script — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/zulaman.cpp
-- (npc_zulaman_hostageAI). Hostage entries 23790, 23999, 24024, 24001
-- (C++ HostageEntry[]; AddSC_zulaman registers "npc_zulaman_hostage").
-- Eluna gossip events: 1 OnGossipHello (offers "I am glad to help you."),
-- 2 OnGossipSelect (closes the menu on the GOSSIP_ACTION_INFO_DEF + 1
-- action). No combat AI, no timers; melee is engine-driven in Go.
-- Deviations from C++: instance->SetData(DATA_CHESTLOOTED, 0) has no
-- bearer (no instance-script model); SummonGameObject(ChestEntry[i])
-- (186648 / 187021 / 186672 / 186667 per hostage entry) has no bearer
-- (no summon/gameobject-spawn bridge) — no chest is spawned; player->
-- GetGossipTextId has no bridge, so the menu title id is 0.
-- (The old "no HasFlag/RemoveFlag on the creature Lua object" claim was
-- stale — the Eluna gossip creature has GetNPCFlags/SetNPCFlags,
-- engine/world/lua_objects.go:657/666 — so the one-shot below is the
-- C++-exact UNIT_NPC_FLAG_GOSSIP removal.)
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

-- UNIT_NPC_FLAG_GOSSIP (UnitDefines.h:186) — the engine runs the gossip
-- hooks only while this flag is set (gossip.go helloCreatureNPC /
-- select flag gates), so clearing it is the C++-exact one-shot.
local UNIT_NPC_FLAG_GOSSIP = 0x00000001

local function onHello(event, player, creature)
    -- C++ adds the item unconditionally; the hook only fires while the
    -- flag is set, so the option is inherently one-shot.
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HOSTAGE1, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(0, creature)
end

local function onSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
    end
    -- C++-exact order: the close arms run first; then the flag gate.
    if bit32.band(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP) == 0 then
        return
    end
    -- C++ me->RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP).
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    -- instance->SetData(DATA_CHESTLOOTED, 0) + SummonGameObject: no bearer.
end

for _, entry in ipairs(HOSTAGE_ENTRIES) do
    RegisterCreatureGossipEvent(entry, 1, onHello)
    RegisterCreatureGossipEvent(entry, 2, onSelect)
end
