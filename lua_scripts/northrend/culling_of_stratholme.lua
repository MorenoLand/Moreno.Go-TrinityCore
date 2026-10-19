-- Culling of Stratholme: npc_chromie_start — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/culling_of_stratholme.cpp
-- (npc_chromie_start : public CreatureScript).
-- Entry: NPC_CHROMIE 26527 (instance_culling_of_stratholme.cpp:61).
-- Ported: OnGossipSelect's GOSSIP_OFFSET_TELEPORT arm (:364-367):
-- CloseGossipMenuFor + me->CastSpell(player, SPELL_TELEPORT_PLAYER
-- 53435), non-triggered. The Lua select hook receives (sender,
-- action); GOSSIP_OFFSET_TELEPORT = 2 (enum :238-240), so the gate
-- is action == GOSSIP_ACTION_INFO_DEF + 2 = 1002 (exact).
-- Documented-only: the instance-gated OnGossipHello menu (skip /
-- explain arms ride instance->GetStableStateFor / AdvanceDungeon /
-- SetGuidData — no instance bridge), the EXPLAIN / SKIP menu-nav
-- select arms (fire only from the ungated menu), the AddItem
-- (ITEM_ARCANE_DISRUPTOR) leg (no item-grant bridge), and the GM
-- instance-control arms (SetData / SetGuidData — no instance
-- bridge).

local ENTRY_CHROMIE_START = 26527

local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_OFFSET_TELEPORT = 2
local SPELL_TELEPORT_PLAYER = 53435

local function chromieGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_TELEPORT then
        player:GossipComplete()
        creature:CastSpell(player, SPELL_TELEPORT_PLAYER)
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

RegisterCreatureGossipEvent(ENTRY_CHROMIE_START, 2, chromieGossipSelect)
