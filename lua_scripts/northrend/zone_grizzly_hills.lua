-- Grizzly Hills: npc_lake_frog — Lua port of
-- src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_lake_frog : public CreatureScript; registered in AddSC_zone_grizzly_hills()).
-- Entries: NPC_LAKE_FROG = 33211 / NPC_LAKE_FROG_QUEST = 33224
-- (zone_grizzly_hills.cpp:655-667 LakeFrog enum). The C++ tree has a single
-- npc_lake_frog script (one `new` at :1130), so both entries share this AI —
-- both are registered.
-- Ported: Reset's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) twin on
-- spawn (event 5) — the flag is managed by the script's kiss state machine
-- (:706/:731/:735), so the select arm below stays gated exactly like C++ —
-- plus the OnGossipSelect DoCast(player, SPELL_SUMMON_ASHWOOD_BRAND 62554)
-- arm (:95-98), non-triggered (default DoCast overload); return false lets
-- the AI/native select arms proceed (Eluna gossip polarity, gossip.go).
-- Documented-only: the whole MoveInLineOfSight/EVENT_LAKEFROG kiss machine
-- (_following/_runningScript latches, SPELL_FROG_KISS 62536,
-- SPELL_MAIDEN_OF_ASHWOOD_LAKE_TRANSFORM 62550 on the maiden 33220,
-- SAY_MAIDEN_0/1, SetFacingToObject) — no player-proximity / transform /
-- cross-creature bridges; the gossip flag can never be re-set without it.
local ENTRY_LAKE_FROG = 33211
local ENTRY_LAKE_FROG_QUEST = 33224
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local SPELL_SUMMON_ASHWOOD_BRAND = 62554

local function lakeFrogSpawn(_, creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
end

local function lakeFrogGossipSelect(_, player, creature)
    creature:CastSpell(player, SPELL_SUMMON_ASHWOOD_BRAND)
    return false
end

RegisterCreatureEvent(ENTRY_LAKE_FROG, 5, lakeFrogSpawn)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 5, lakeFrogSpawn)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG, 2, lakeFrogGossipSelect)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG_QUEST, 2, lakeFrogGossipSelect)
