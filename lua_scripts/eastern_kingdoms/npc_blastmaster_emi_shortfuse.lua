-- Gnomeregan: Blastmaster Emi Shortfuse — Lua port of the one
-- bridgeable WaypointReached arm of
-- src/server/scripts/EasternKingdoms/Gnomeregan/gnomeregan.cpp
-- (class npc_blastmaster_emi_shortfuse :
-- CreatureScript("npc_blastmaster_emi_shortfuse");
-- struct npc_blastmaster_emi_shortfuseAI : public EscortAI).
-- Entry: 7998 (TDB 335.26091 creature_template ScriptName
-- npc_blastmaster_emi_shortfuse; creature_text groups 0-19).
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (gnomeregan.cpp:227-265):
-- - WaypointReached wp14 Talk(SAY_BLASTMASTER_17 17).
-- Deliberate omissions (no Lua twin, not a missing bridge to chase
-- here): every other wp arm is a NextStep/SetEscortPaused/SetInFace/
-- Summon call of the uiPhase machine — wp3/7/11 SetEscortPaused +
-- NextStep, wp9/10/12 NextStep, wp13 Summon(6) + NextStep, wp14
-- SetInFace(false) + SetEscortPaused(true) + NextStep(5000, false,
-- 20). There is no NextStep/EventMap bridge, and SetFacingToObject
-- has no direct bridge (lua_creature_events.go:5609).
-- Unported scope: the gossip driver (OnGossipSelect Start +
-- SetData(1)) and the whole uiPhase/NextStep machine (SetData legs,
-- CaveDestruction, Summon cases, UpdateAI) — it is also the escort
-- driver, so this handler fires once that machine lands.

local ENTRY_BLASTMASTER = 7998

local SAY_BLASTMASTER_17 = 17 -- gnomeregan.cpp:56

-- C++ WaypointReached wp14 (gnomeregan.cpp:259-265): only the Talk
-- leg is bridgeable; the SetInFace / pause / NextStep legs stay out
-- with the phase machine (see header).
local function blastmasterReachWP(event, creature)
    if creature:GetCurrentWaypointId() == 14 then
        creature:Talk(SAY_BLASTMASTER_17)
    end
end

RegisterCreatureEvent(ENTRY_BLASTMASTER, 6, blastmasterReachWP)
