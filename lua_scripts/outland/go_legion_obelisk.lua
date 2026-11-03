-- Legion Obelisks (Blade's Edge Mountains) — Lua port of
-- src/server/scripts/Outland/zone_blades_edge_mountains.cpp
-- (class go_legion_obelisk : GameObjectScript("go_legion_obelisk");
-- struct go_legion_obeliskAI : public GameObjectAI).
-- Quest support: You're Fired! (10821).
-- Ported arms (C++ order, zone_blades_edge_mountains.cpp:262-294):
-- - OnGossipHello: if the player's quest-10821 status is INCOMPLETE,
--   set this obelisk's flag (C++ file-scope globals, mirrored here
--   as one module-level table shared by all five entries); when all
--   five flags are set, summon Doomcryer (19963) at the C++ coords
--   (2943.40, 4778.20, 284.49, 0.94, TEMPSUMMON_TIMED_DESPAWN_
--   OUT_OF_COMBAT = type 3, 2min) and reset all five flags.
-- - Return value: C++ returns true (handled), so Lua returns true
--   and the native GO use arms are suppressed (gundrak-altar
--   precedent; Eluna gossip polarity).
-- Bridges used (all verified in-tree this tick): GO gossip hello
-- fires from the GO use path (gameobject_hooks.go), player:
-- GetQuestStatus (INCOMPLETE = 3, lua_player.go), GO:
-- SummonCreature (entry, x, y, z, o, type, despawn-ms,
-- lua_objects.go), GO:GetEntry. No boss_ai.go registration is
-- needed: GO gossip hooks are entry-keyed (go_gundrak_altar
-- precedent).

local GO_LEGION_OBELISK_ONE = 185193
local GO_LEGION_OBELISK_TWO = 185195
local GO_LEGION_OBELISK_THREE = 185196
local GO_LEGION_OBELISK_FOUR = 185197
local GO_LEGION_OBELISK_FIVE = 185198

local NPC_DOOMCRYER = 19963
local QUEST_YOURE_FIRED = 10821
local QUEST_STATUS_INCOMPLETE = 3

local OBELISK_ENTRIES = {
    GO_LEGION_OBELISK_ONE,
    GO_LEGION_OBELISK_TWO,
    GO_LEGION_OBELISK_THREE,
    GO_LEGION_OBELISK_FOUR,
    GO_LEGION_OBELISK_FIVE,
}

-- C++ file-scope globals obelisk_one..five, as one table.
local obeliskFlags = {}

local function onObeliskGossip(_, player, go)
    if player:GetQuestStatus(QUEST_YOURE_FIRED) == QUEST_STATUS_INCOMPLETE then
        obeliskFlags[go:GetEntry()] = true

        local allUsed = true
        for _, entry in ipairs(OBELISK_ENTRIES) do
            if not obeliskFlags[entry] then
                allUsed = false
                break
            end
        end

        if allUsed then
            go:SummonCreature(NPC_DOOMCRYER, 2943.40, 4778.20, 284.49, 0.94, 3, 120000)
            obeliskFlags = {}
        end
    end

    return true
end

for _, entry in ipairs(OBELISK_ENTRIES) do
    RegisterGameObjectGossipEvent(entry, 1, onObeliskGossip)
end
