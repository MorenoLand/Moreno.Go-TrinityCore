-- Winterspring: Ranshalla (Guardians of the Altar) — Lua port of
-- src/server/scripts/Kalimdor/zone_winterspring.cpp
-- (class npc_ranshalla : CreatureScript("npc_ranshalla");
-- struct npc_ranshallaAI : public EscortAI).
-- AddSC_zone_winterspring registers the zone script
-- (kalimdor_script_loader.cpp decl/call — the "// Winterspring"
-- loader block).
-- Entry: 10300 (NPC_RANSHALLA, zone_winterspring.cpp:85;
-- enum-verified — the creature_template ScriptName binding is
-- DB-side).
-- Eluna creature events: 31 OnQuestAccept
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go).
-- Ported arms:
-- - OnQuestAccept (zone_winterspring.cpp:540-547, event 31): quest
--   id 4901 (QUEST_GUARDIANS_ALTAR) -> Talk(SAY_QUEST_START 0,
--   untargeted — the willix precedent). SetFaction(10
--   FACTION_ESCORTEE_A_NEUTRAL_PASSIVE, C++ order: Talk, SetFaction,
--   Start) also ported; EscortAI Start alone has no bridge —
--   documented-only.
-- Unmodeled (documented-only, no bridges):
-- - The whole Elune-altar event machine (npc_ranshallaAI UpdateAI
--   EventMap, WaypointReached Talk legs, SummonCreature priestess/
--   guardian legs, SPELL_LIGHT_TORCH channel casts, GO 177404/177414
--   respawns) — the escort-waypoint/event machine has no bridge;
--   porting the accept arm alone only adds the intro Talk, which is
--   exactly the C++-fidelity choice the willix precedent makes.

local ENTRY_RANSHALLA = 10300

local SAY_QUEST_START = 0

local QUEST_GUARDIANS_ALTAR = 4901

-- C++ OnQuestAccept for quest 4901 (zone_winterspring.cpp:540-547, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(10); EscortAI::Start
-- alone has no bridge.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_GUARDIANS_ALTAR then
        return
    end
    creature:Talk(SAY_QUEST_START)
    creature:SetFaction(10)
end

RegisterCreatureEvent(ENTRY_RANSHALLA, 31, onQuestAccept)
