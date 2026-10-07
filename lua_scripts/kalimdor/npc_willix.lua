-- Razorfen Kraul: Willix the Importer (quest 1144 escort) — Lua port of
-- src/server/scripts/Kalimdor/RazorfenKraul/razorfen_kraul.cpp
-- (class npc_willix : CreatureScript("npc_willix") { npc_willixAI :
-- public EscortAI }; GetAI via GetRazorfenKraulAI<npc_willixAI>
-- (razorfen_kraul.h — GetInstanceAI<AI>(obj, "instance_razorfen_kraul")
-- gate; the instance-side legs have no bridge, standing).
-- AddSC_razorfen_kraul at end registers the single zone script
-- (kalimdor_script_loader.cpp decl :76 / call :189, the
-- "// Razorfen Kraul" loader block).
-- Entry: 4508 wowhead-verified (wowhead.com/forever/npc=4508/
-- willix-the-importer; no NPC_ constant in the C++ tree —
-- landslide/noxxion/ptheradras/belnistrasz precedent); creature_text
-- rows back Talk ids 0 and 2.
-- Eluna creature events: 1 OnEnterCombat, 31 OnQuestAccept
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go). Melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms:
-- - OnQuestAccept (C++ OnQuestAccept, event 31): quest id 1144
--   (QUEST_WILLIX_THE_IMPORTER) -> Talk(SAY_READY 0). The rest
--   (EscortAI Start, SetFaction escorted-neutral) has no bridges —
--   documented-only.
-- - Engage (C++ JustEngagedWith): Talk(SAY_AGGRO1 2), ungated (the
--   C++ Talk targets the player; the bridge broadcasts — deviation,
--   documented).
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached (waypoints 3/4/8/9/13/14/25/31/42/43/45/46):
--   Talk lines SAY_POINT 1 / SAY_BLUELEAF 3 / SAY_DANGER 4 /
--   SAY_BAD 5 / SAY_THINK 6 / SAY_SOON 7 / SAY_FINALY 8 / SAY_WIN 9 /
--   SAY_END 10, HandleEmoteCommand, questgiver-flag restore, boar
--   summons (ENTRY_BOAR 4514) and the GroupEventHappens credit —
--   the whole escort path needs a waypoint/movement bridge plus
--   summon + quest-credit bridges, none of which exist.
-- - JustSummoned -> AttackStart (needs SummonCreature + AI bridge).
-- - JustDied -> FailQuest (no quest-fail bridge).

local ENTRY_WILLIX = 4508

local SAY_READY  = 0
local SAY_AGGRO1 = 2

local QUEST_WILLIX_THE_IMPORTER = 1144

-- C++ OnQuestAccept for quest 1144: Talk(READY); the EscortAI start
-- and faction swap have no bridges.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_WILLIX_THE_IMPORTER then
        return
    end
    creature:Talk(SAY_READY)
end

-- C++ JustEngagedWith: ungated Talk(SAY_AGGRO1) (C++ targets the
-- player; the bridge broadcasts).
local function onEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO1)
end

RegisterCreatureEvent(ENTRY_WILLIX, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WILLIX, 31, onQuestAccept)
