-- Blackrock Depths: Private Rocknot — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/blackrock_depths.cpp
-- (class npc_rocknot : CreatureScript("npc_rocknot");
-- struct npc_rocknotAI : public EscortAI).
-- Entry: 9503 (TDB 335.26091 creature_template ScriptName
-- npc_rocknot).
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, blackrock_depths.cpp:546-569): the
-- WaypointReached body is ONLY HandleEmoteCommand legs — wp1/4/5
-- EMOTE_ONESHOT_KICK, wp2/3 EMOTE_ONESHOT_ATTACK_UNARMED
-- (SharedDefines.h:1968/1981: ATTACK_UNARMED 35, KICK 60),
-- via creature:PerformEmote (lua_creature_events.go:4883,
-- thrall exclamation precedent).
-- Unported scope: the BreakKeg/BreakDoor timer machine
-- (blackrock_depths.cpp:571-604: DoGo DATA_GO_BAR_KEG/DOOR/KEG_TRAP,
-- Phalanx faction flip, instance SetData(TYPE_BAR, DONE)) is
-- UpdateAI scope; the escort driver (OnQuestReward quest 4295
-- Start) is with it, so this handler fires once that lands.

local ENTRY_ROCKNOT = 9503

local EMOTE_ONESHOT_ATTACK_UNARMED = 35
local EMOTE_ONESHOT_KICK = 60

-- C++ WaypointReached (blackrock_depths.cpp:546-569).
local function rocknotReachWP(event, creature)
    local wp = creature:GetCurrentWaypointId()
    if wp == 1 or wp == 4 or wp == 5 then
        creature:PerformEmote(EMOTE_ONESHOT_KICK)
    elseif wp == 2 or wp == 3 then
        creature:PerformEmote(EMOTE_ONESHOT_ATTACK_UNARMED)
    end
end

RegisterCreatureEvent(ENTRY_ROCKNOT, 6, rocknotReachWP)
