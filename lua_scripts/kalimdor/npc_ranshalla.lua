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
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go).
-- Ported arms:
-- - OnQuestAccept (zone_winterspring.cpp:540-547, event 31): quest
--   id 4901 (QUEST_GUARDIANS_ALTAR) -> Talk(SAY_QUEST_START 0,
--   untargeted — the willix precedent). SetFaction(10
--   FACTION_ESCORTEE_A_NEUTRAL_PASSIVE, C++ order: Talk, SetFaction,
--   Start) also ported, plus StartEscort(player) + MoveWaypoint()
--   (the walk rides MoveWaypoint — StartEscort only latches the
--   player, ruul precedent; the old "EscortAI Start has no bridge"
--   note is stale).
-- - WaypointReached (zone_winterspring.cpp:351-396, event 6,
--   onReachWP below) — PORTED 2026-11-15, PARTIAL: wp3 Talk
--   (SAY_ENTER_OWL_THICKET 1); wp10/15/20/25/36 DoChannelTorchSpell
--   legs (GO_ELUNE_FIRE 177417 RemoveFlag within 10yd + Talk
--   SAY_REACH_TORCH 2 + Talk EMOTE_CHANT_SPELL 29 + self-cast
--   SPELL_LIGHT_TORCH 18953); wp42 SetEscortPaused(true) +
--   DoSummonPriestess (2x NPC_PRIESTESS_ELUNE 12116 at
--   wingThicketLocations[0]/[1], MoveTo [3]/[4], CORPSE_DESPAWN
--   type 4) + Talk(SAY_RANSHALLA_ALTAR_2 7).
--   DEVIATIONS: the torch-waypoint SetEscortPaused(true) is omitted
--   — its resume rides go_elune_fire OnGossipHello ->
--   DoContinueEscort (unported GO script), so the pause would stall
--   the escort at the first torch; she channels while walking
--   instead. The wp42 pause is kept (it holds her at the altar, the
--   destination) but its resume rides the unported altar dialogue
--   (SAY_RANSHALLA_ALTAR_6 -> SetEscortPaused(false)); the 2s
--   EVENT_RESUME dialogue-start stays out with it.
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached wp39 (StartNextDialogueText(SAY_REACH_ALTAR_1)
--   + pause — the DialogueHelper machine has no bridge, and the
--   pause without it would stall the escort with no resume), wp41
--   (GetGameObjectListWithEntryInGrid GO_ELUNE_LIGHT respawn sweep —
--   no list bridge; the altar GUID it faces is latched only by the
--   dialogue path), wp44 (pause + face altar GUID, unset without the
--   dialogue; no resume). The quest credit itself lives in the
--   end-dialogue (SAY_RANSHALLA_END_2 -> GroupEventHappens(4901) +
--   DespawnOrUnsummon(4s)), so the escort walks but the quest still
--   cannot complete in Go.
-- - The rest of the Elune-altar event machine (npc_ranshallaAI
--   UpdateAI EventMap / DialogueHelper sequences, guardian/voice
--   summons, GO 177404/177414 respawns).

local ENTRY_RANSHALLA = 10300

local SAY_QUEST_START = 0
local SAY_ENTER_OWL_THICKET = 1
local SAY_REACH_TORCH = 2
local SAY_RANSHALLA_ALTAR_2 = 7
local EMOTE_CHANT_SPELL = 29

local QUEST_GUARDIANS_ALTAR = 4901

local SPELL_LIGHT_TORCH = 18953

local NPC_PRIESTESS_ELUNE = 12116

local GO_ELUNE_FIRE = 177417
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

local SUMMON_TYPE_CORPSE_DESPAWN = 4

-- wingThicketLocations (zone_winterspring.cpp:244-255): [0]/[1] priestess
-- summon spots, [3]/[4] their MovePoint destinations.
local PRIESTESS_SUMMON_1 = { 5515.98, -4903.43, 846.30, 4.58 }
local PRIESTESS_SUMMON_2 = { 5501.94, -4920.20, 848.69, 6.15 }
local PRIESTESS_MOVE_1 = { 5518.38, -4913.47, 845.57 }
local PRIESTESS_MOVE_2 = { 5510.36, -4921.17, 846.33 }

-- C++ OnQuestAccept for quest 4901 (zone_winterspring.cpp:540-547, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(10) + StartEscort(player)
-- + MoveWaypoint() (the walk rides MoveWaypoint — StartEscort only latches
-- the player, ruul precedent).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_GUARDIANS_ALTAR then
        return
    end
    creature:Talk(SAY_QUEST_START)
    creature:SetFaction(10)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ DoChannelTorchSpell(false) (zone_winterspring.cpp:305-324): nearest
-- GO_ELUNE_FIRE within 10yd loses NOT_SELECTABLE + Talk(2) + Talk(29) +
-- self-cast SPELL_LIGHT_TORCH. The C++ SetEscortPaused(true) is omitted —
-- see header (resume rides the unported go_elune_fire gossip).
local function channelTorch(creature)
    local fire = creature:FindNearestGameObject(GO_ELUNE_FIRE, 10.0)
    if fire then
        fire:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    creature:Talk(SAY_REACH_TORCH)
    creature:Talk(EMOTE_CHANT_SPELL)
    creature:CastSpell(creature, SPELL_LIGHT_TORCH)
end

-- C++ DoSummonPriestess (zone_winterspring.cpp:326-341): 2x priestess at
-- the wingThicketLocations summon spots (CORPSE_DESPAWN), each MovePoint
-- to its altar spot. The C++ SummonedMovementInform leg
-- (MovePoint 1 arrival -> StartNextDialogueText) rides the unported
-- dialogue machine and stays out.
local function summonPriestesses(creature)
    local first = creature:SummonCreature(NPC_PRIESTESS_ELUNE,
        PRIESTESS_SUMMON_1[1], PRIESTESS_SUMMON_1[2], PRIESTESS_SUMMON_1[3],
        PRIESTESS_SUMMON_1[4], SUMMON_TYPE_CORPSE_DESPAWN, 0)
    if first then
        first:MoveTo(0, PRIESTESS_MOVE_1[1], PRIESTESS_MOVE_1[2], PRIESTESS_MOVE_1[3])
    end
    local second = creature:SummonCreature(NPC_PRIESTESS_ELUNE,
        PRIESTESS_SUMMON_2[1], PRIESTESS_SUMMON_2[2], PRIESTESS_SUMMON_2[3],
        PRIESTESS_SUMMON_2[4], SUMMON_TYPE_CORPSE_DESPAWN, 0)
    if second then
        second:MoveTo(1, PRIESTESS_MOVE_2[1], PRIESTESS_MOVE_2[2], PRIESTESS_MOVE_2[3])
    end
end

-- C++ WaypointReached (zone_winterspring.cpp:351-396): wp3 Talk(1); wp10/
-- 15/20/25/36 DoChannelTorchSpell(); wp42 SetEscortPaused(true) +
-- DoSummonPriestess() + Talk(7) (the 2s EVENT_RESUME dialogue-start stays
-- out — see header). wp39/41/44 stay out (dialogue/list-gated, see header).
-- Unlike muglash/remtravel this switch is NOT gated on GetPlayerForEscort()
-- in C++, so no player guard here.
local function onReachWP(_, creature)
    local wp = creature:GetCurrentWaypointId()
    if wp == 3 then
        creature:Talk(SAY_ENTER_OWL_THICKET)
    elseif wp == 10 or wp == 15 or wp == 20 or wp == 25 or wp == 36 then
        channelTorch(creature)
    elseif wp == 42 then
        creature:SetEscortPaused(true)
        summonPriestesses(creature)
        creature:Talk(SAY_RANSHALLA_ALTAR_2)
    end
end

RegisterCreatureEvent(ENTRY_RANSHALLA, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_RANSHALLA, 6, onReachWP)
