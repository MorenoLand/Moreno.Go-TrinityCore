-- Ashenvale: Ruul Snowhoof — Lua port of
-- src/server/scripts/Kalimdor/zone_ashenvale.cpp
-- (class npc_ruul_snowhoof : CreatureScript("npc_ruul_snowhoof");
-- struct npc_ruul_snowhoofAI : public EscortAI).
-- Entry: 12818 (Wowhead Classic NPC page, verified this tick;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side).
-- Eluna creature events: 23 OnReset, 31 OnQuestAccept.
-- Ported arms (C++ order):
-- - Reset (zone_ashenvale.cpp:74-78): the nearest cage GO
--   (GO_CAGE 178147, 20yd) gets SetGoState(1) (GO_STATE_READY,
--   netherspite precedent). The IsDead guard is the marli ev23
--   idiom: Go fires ev23 on spawn, evade and death, while C++
--   Reset runs on spawn + evade only.
-- - OnQuestAccept (zone_ashenvale.cpp:87-94): quest id 6482
--   (QUEST_FREEDOM_TO_RUUL) -> SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, SharedDefines.h:240).
--   EscortAI::Start alone has no bridge — documented-only
--   (muglash precedent: Talk + SetFaction ported, Start out).
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached cage-open leg (waypoint 0: cage SetGoState
--   GO_STATE_ACTIVE) + Thistlefur summon waves (waypoints 13/19)
--   + finish Talk/credit (waypoint 27): the waypoint machine has
--   no bridge; JustSummoned AttackStart has no bridge.
-- - EnterEvadeMode shapechange re-apply (RUUL_SHAPECHANGE):
--   EnterEvadeMode has no Lua bridge (boss_ai.go explicit).

local ENTRY_RUUL_SNOWHOOF = 12818

local GO_CAGE = 178147
local GO_STATE_READY = 1

local QUEST_FREEDOM_TO_RUUL = 6482
local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

-- C++ npc_ruul_snowhoofAI::Reset (zone_ashenvale.cpp:74-78).
local function ruulReset(event, creature)
    if creature:IsDead() then
        return
    end
    local cage = creature:FindNearestGameObject(GO_CAGE, 20.0)
    if cage then
        cage:SetGoState(GO_STATE_READY)
    end
end

-- C++ npc_ruul_snowhoofAI::OnQuestAccept (zone_ashenvale.cpp:87-94):
-- SetFaction only; EscortAI::Start stays out (no escort bridge).
local function ruulQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_FREEDOM_TO_RUUL then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_PASSIVE)
end

RegisterCreatureEvent(ENTRY_RUUL_SNOWHOOF, 23, ruulReset)
RegisterCreatureEvent(ENTRY_RUUL_SNOWHOOF, 31, ruulQuestAccept)
