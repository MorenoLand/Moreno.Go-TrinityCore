-- Blackrock Depths: Private Rocknot — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/blackrock_depths.cpp
-- (class npc_rocknot : CreatureScript("npc_rocknot");
-- struct npc_rocknotAI : public EscortAI).
-- Entry: 9503 (TDB 335.26091 creature_template ScriptName
-- npc_rocknot).
-- Eluna creature events: 6 OnReachWP, 34 OnQuestReward.
-- Ported arms (C++ order, blackrock_depths.cpp:546-569): the
-- WaypointReached body is ONLY HandleEmoteCommand legs — wp1/4/5
-- EMOTE_ONESHOT_KICK, wp2/3 EMOTE_ONESHOT_ATTACK_UNARMED
-- (SharedDefines.h:1968/1981: ATTACK_UNARMED 35, KICK 60),
-- via creature:PerformEmote (lua_creature_events.go:4883,
-- thrall exclamation precedent).
-- Ported arms (blackrock_depths.cpp:603-626): OnQuestReward quest
-- 4295 — the escort driver. The instance TYPE_BAR state rides the
-- instance script (instance_blackrock_depths.lua): the derived
-- GetData is SPECIAL iff encounter[2] is IN_PROGRESS and three
-- ales were turned in, so the Talk + Drunken Rage self-cast +
-- Start fire on the third turn-in only; the first two only set
-- encounter[2] IN_PROGRESS and bump the ale count. Start's walk
-- rides creature:MoveWaypoint (the assigned path re-arms from its
-- first node) and its UNIT_NPC_FLAGS clear rides
-- creature:SetNPCFlags(0) (nefarian precedent). Deltas vs C++
-- Start(false, false) (ScriptedEscortAI.cpp:278): no escort player
-- is armed (C++ passes none, and Go StartEscort requires one), so
-- the respawn-queue leg stays out too; the flags clear + walk only
-- C++-fire when the path is non-empty, and TDB 26091 carries zero
-- waypoint_data rows for rocknot's spawn guid 46614, so both legs
-- are dormant on TDB data (willix precedent) — the Talk + cast
-- legs fire regardless.
-- Unported scope: the BreakKeg/BreakDoor timer machine
-- (blackrock_depths.cpp:571-604: DoGo DATA_GO_BAR_KEG/DOOR/KEG_TRAP,
-- Phalanx faction flip, instance SetData(TYPE_BAR, DONE)) is
-- UpdateAI scope; the escort driver above fires without it.
-- Reset (blackrock_depths.cpp:532-538) is vacuous in-model: escort-gated
-- Initialize() zeroes only the two timers of that unported machine, no
-- observable leg, so no ev23 registration (clintar/marzon precedent).

local ENTRY_ROCKNOT = 9503

local EMOTE_ONESHOT_ATTACK_UNARMED = 35
local EMOTE_ONESHOT_KICK = 60

local QUEST_ALE = 4295 -- blackrock_depths.cpp:504
local SAY_GOT_BEER = 0 -- blackrock_depths.cpp:503
local SPELL_DRUNKEN_RAGE = 14872 -- blackrock_depths.cpp:505
local TYPE_BAR = 3 -- blackrock_depths.h:30
local ENCOUNTER_IN_PROGRESS = 1 -- InstanceScript.h:71
local ENCOUNTER_DONE = 3 -- InstanceScript.h:73
local ENCOUNTER_SPECIAL = 4 -- InstanceScript.h:74

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

-- C++ GetData(TYPE_BAR) (instance_blackrock_depths.cpp:277-283):
-- SPECIAL iff encounter[2] is IN_PROGRESS and three ales were
-- turned in, else encounter[2].
local function barState(data)
    if not data then
        return 0
    end
    if (data.barEncounter or 0) == ENCOUNTER_IN_PROGRESS and (data.barAleCount or 0) == 3 then
        return ENCOUNTER_SPECIAL
    end
    return data.barEncounter or 0
end

-- C++ OnQuestReward (blackrock_depths.cpp:603-626).
local function rocknotQuestReward(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ALE then
        return
    end
    local data = instanceDataOf(creature)
    if not data then
        return
    end
    local bar = barState(data)
    if bar == ENCOUNTER_DONE or bar == ENCOUNTER_SPECIAL then
        return
    end
    if bar ~= ENCOUNTER_IN_PROGRESS then
        data:SetData(TYPE_BAR, ENCOUNTER_IN_PROGRESS)
    end
    data:SetData(TYPE_BAR, ENCOUNTER_SPECIAL)
    if barState(data) == ENCOUNTER_SPECIAL then
        creature:Talk(SAY_GOT_BEER)
        creature:CastSpell(creature, SPELL_DRUNKEN_RAGE)
        creature:SetNPCFlags(0)
        creature:MoveWaypoint()
    end
end

RegisterCreatureEvent(ENTRY_ROCKNOT, 34, rocknotQuestReward)

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
