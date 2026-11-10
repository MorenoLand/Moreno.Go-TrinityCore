-- Tyrion (Stormwind City) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_stormwind_city.cpp
-- (npc_tyrionAI — OnQuestAccept only).
-- Zone-script unit per eastern_kingdoms_script_loader.cpp order
-- (stormwind_city: npc_tyrion, npc_tyrion_spybot, npc_lord_gregor_
-- lescovar, npc_marzon_silent_blade ported; zone_stormwind_city
-- closes the EK zone set).
-- Entries (LordGregorLescovar / Tyrion enums, verifiable from the
-- C++ sources): 7766 (NPC_TYRION), 8856 (NPC_TYRION_SPYBOT),
-- quest 434 (QUEST_THE_ATTACK). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 31 OnQuestAccept (the C++ override has
-- no other arms — no combat, Reset, or gossip handlers).
-- Fight shape (C++-exact for the modeled arm):
-- OnQuestAccept(31) quest 434 -> FindNearestCreature(
-- NPC_TYRION_SPYBOT 8856, 5.0f) -> spybot StartEscort(player)
-- + MoveWaypoint() (legoso precedent: StartEscort takes the
-- player object; the walk rides the DB waypoint path).
-- Deliberate delta: C++ Start(false, false, player GUID) also
-- calls SetMaxPlayerDistance(200.0f) on the spybot AI — no
-- bridge (no SetMaxPlayerDistance method on the creature
-- surface); the 200yd leash is unmodeled, the escort itself
-- is exact.

local NPC_TYRION = 7766
local NPC_TYRION_SPYBOT = 8856

local QUEST_THE_ATTACK = 434

-- C++ OnQuestAccept (zone_stormwind_city.cpp:479-491): quest
-- 434 -> nearest live spybot within 5yd starts its escort for
-- the accepting player.
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_THE_ATTACK then
        return
    end
    local spybot = creature:FindNearestCreature(NPC_TYRION_SPYBOT, 5.0)
    if spybot then
        spybot:StartEscort(player)
        spybot:MoveWaypoint()
    end
end

RegisterCreatureEvent(NPC_TYRION, 31, onQuestAccept)
