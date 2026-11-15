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
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 4 JustDied
-- (engine/world/quest_details.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go).
-- Ported arms:
-- - OnQuestAccept (zone_ashenvale.cpp:237-244, event 31): quest id
--   6641 (QUEST_VORSHA) -> Talk(SAY_MUG_START1 0, untargeted — the
--   willix precedent). SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, C++ order: Talk, SetFaction,
--   Start) also ported, including StartEscort(player) — the escort
--   bridges landed since (ticks 372-373).
-- - JustDied (zone_ashenvale.cpp:227-232, event 4) — PORTED 2026-11-07:
--   if IsEscorted() then GetEscortPlayer():FailQuest(QUEST_VORSHA
--   6641) (hanes precedent uses a GUID latch; the bridge idiom is
--   the legoso healing arm).
-- Unmodeled (documented-only, no bridges):
-- - JustEngagedWith Talk(SAY_MUG_ON_GUARD 4, player-targeted) and
--   JustSummoned AttackStart — both escort-state-gated, and the
--   waypoint arms that produce the summons are unported, so the
--   arms would never fire in the Go model. (ev19 fires on the
--   summoner and AttackStart is bridged — the stay-out is the
--   missing fire site, not a missing bridge.)
-- - WaypointReached (waypoints 0/24/25/26/27) — PORTED 2026-11-15
--   (ev6, onReachWP below): Talk arms SAY_MUG_START2 1 (wp0) /
--   SAY_MUG_BRAZIER 2 (wp24) / SAY_MUG_GRATITUDE 7 (wp25) /
--   SAY_MUG_PATROL 8 (wp26) / SAY_MUG_RETURN 9 (wp27), the wp24
--   brazier GO flag clear (GO_NAGA_BRAZIER 178247, 10yd =
--   INTERACTION_DISTANCE*2; GAMEOBJECT_FLAGS 9, NOT_SELECTABLE 16,
--   arlokk precedent) and the wp25 GroupEventHappens(QUEST_VORSHA
--   6641); the walk rides MoveWaypoint() from OnQuestAccept
--   (StartEscort only latches the player — ruul precedent).
--   DEVIATION: C++ also SetEscortPaused(true) at wp24; omitted —
--   its resume rides go_naga_brazier OnGossipHello
--   (_isBrazierExtinguished) + the UpdateAI wave machine (waves 1-4,
--   wave 4 resumes), both unported, so the pause would stall the
--   escort at the brazier forever. The escort therefore walks
--   straight through and credits at wp25 without the Vorsha fight;
--   porting go_naga_brazier + DoWaveSummon would restore the gating.
--   StopEscort() at wp27 (highest wp id in the AI = path end; Go
--   paths loop by default) so the wp25 credit never re-fires.
--   SAY_MUG_DONE 6 (wave 4) + the naga-wave summons stay out with
--   the wave machine. No SAY_MUG_VORSHA exists (enum
--   zone_ashenvale.cpp:149-158; SAY_MUG_BRAZIER_WAIT is 3,
--   SAY_MUG_ON_GUARD is 4).

local ENTRY_MUGLASH = 12717

local SAY_MUG_START1 = 0
local SAY_MUG_START2 = 1
local SAY_MUG_BRAZIER = 2
local SAY_MUG_GRATITUDE = 7
local SAY_MUG_PATROL = 8
local SAY_MUG_RETURN = 9

local QUEST_VORSHA = 6641

local GO_NAGA_BRAZIER = 178247
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

-- C++ OnQuestAccept for quest 6641 (zone_ashenvale.cpp:237-244, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(113) + StartEscort +
-- MoveWaypoint() (the walk rides MoveWaypoint — StartEscort only latches
-- the player, ruul precedent).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_VORSHA then
        return
    end
    creature:Talk(SAY_MUG_START1)
    creature:SetFaction(113)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ WaypointReached (zone_ashenvale.cpp:249-278), whole switch gated on
-- GetPlayerForEscort(): wp0 Talk(1, player); wp24 Talk(2, player) + brazier
-- GO RemoveFlag(NOT_SELECTABLE) — the C++ SetEscortPaused(true) is omitted,
-- see header (resume rides the unported brazier/wave machine); wp25
-- Talk(7) + player:GroupEventHappens(6641); wp26 Talk(8); wp27 Talk(9) +
-- StopEscort() (path end; Go paths loop by default).
local function onReachWP(_, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_MUG_START2, player)
    elseif wp == 24 then
        creature:Talk(SAY_MUG_BRAZIER, player)
        local brazier = creature:FindNearestGameObject(GO_NAGA_BRAZIER, 10.0)
        if brazier then
            brazier:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    elseif wp == 25 then
        creature:Talk(SAY_MUG_GRATITUDE)
        player:GroupEventHappens(QUEST_VORSHA, creature)
    elseif wp == 26 then
        creature:Talk(SAY_MUG_PATROL)
    elseif wp == 27 then
        creature:Talk(SAY_MUG_RETURN)
        creature:StopEscort()
    end
end

-- C++ JustDied (zone_ashenvale.cpp:227-232): if escorting, the escort
-- player fails QUEST_VORSHA.
local function onDied(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player then
        player:FailQuest(QUEST_VORSHA)
    end
end

RegisterCreatureEvent(ENTRY_MUGLASH, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_MUGLASH, 6, onReachWP)
RegisterCreatureEvent(ENTRY_MUGLASH, 4, onDied)
