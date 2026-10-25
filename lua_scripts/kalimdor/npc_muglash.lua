-- Ashenvale: Muglash (Vor'sha escort) — Lua port of
-- src/server/scripts/Kalimdor/zone_ashenvale.cpp
-- (class npc_muglash : CreatureScript("npc_muglash");
-- struct npc_muglashAI : public EscortAI).
-- AddSC_zone_ashenvale registers the zone script
-- (kalimdor_script_loader.cpp decl/call — the "// Ashenvale"
-- loader block).
-- Entry: 12717 (NPC_MUGLASH, zone_ashenvale.cpp:173; enum-verified,
-- like the emily precedent — the creature_template ScriptName
-- binding is DB-side).
-- Eluna creature events: 31 OnQuestAccept
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go).
-- Ported arms:
-- - OnQuestAccept (zone_ashenvale.cpp:237-244, event 31): quest id
--   6641 (QUEST_VORSHA) -> Talk(SAY_MUG_START1 0, untargeted — the
--   willix precedent). SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, C++ order: Talk, SetFaction,
--   Start) also ported; EscortAI::Start alone has no bridge —
--   documented-only.
-- Unmodeled (documented-only, no bridges):
-- - JustEngagedWith Talk(SAY_MUG_ON_GUARD 3, player-targeted) and
--   JustDied FailQuest(QUEST_VORSHA) + JustSummoned AttackStart —
--   the C++ legs are all escort-state-gated (HasEscortState), so
--   porting them without the escort machine would fabricate
--   behavior; no escort bridge exists.
-- - WaypointReached (waypoints 0/24/...): Talk arms SAY_MUG_START2
--   1 / SAY_MUG_BRAZIER 2 / SAY_MUG_ON_GUARD 3 / SAY_MUG_VORSHA 4,
--   plus SummonCreature naga-wave legs — the waypoint machine and
--   summons have no bridges.

local ENTRY_MUGLASH = 12717

local SAY_MUG_START1 = 0

local QUEST_VORSHA = 6641

-- C++ OnQuestAccept for quest 6641 (zone_ashenvale.cpp:237-244, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(113); EscortAI::Start
-- alone has no bridge.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_VORSHA then
        return
    end
    creature:Talk(SAY_MUG_START1)
    creature:SetFaction(113)
end

RegisterCreatureEvent(ENTRY_MUGLASH, 31, onQuestAccept)
