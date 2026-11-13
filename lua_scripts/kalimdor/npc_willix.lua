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
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 6 OnReachWP, 31
-- OnQuestAccept (engine/world/quest_details.go fires it as (event,
-- player, creature, quest); quest:GetId() wired — luaQuest surface,
-- lua_creature_events.go). Melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms:
-- - OnQuestAccept (C++ OnQuestAccept, event 31): quest id 1144
--   (QUEST_WILLIX_THE_IMPORTER) -> StartEscort(player) +
--   MoveWaypoint() (legoso precedent: the waypoint walk rides the
--   MoveWaypoint bridge + the OnReachWP (6) arrival event) + Talk
--   (SAY_READY 0) + SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, C++ order: Start, Talk,
--   SetFaction).
-- - WaypointReached (C++ WaypointReached, event 6): the waypoint id
--   is the DB point via creature:GetCurrentWaypointId() (legoso
--   precedent); every arm requires the escort player (the C++
--   GetPlayerForEscort() nil-guard at the top). wp3 Talk(SAY_POINT
--   1); wp4/14/43 summon boar 4514 (TEMPSUMMON_TIMED_DESPAWN_OUT_OF_
--   COMBAT, 25s) + AttackStart(Willix) at the summon site (the C++
--   JustSummoned leg — ev19 fires on the summoner in Go
--   (fireCreatureSummoned, lua_creature_events.go:190/:8002-8013),
--   so the inline form stands on the double-apply, not on the
--   fire site; wp8 Talk(3), wp9
--   Talk(4), wp13 Talk(5), wp25 Talk(6), wp31 Talk(7), wp42
--   Talk(8); wp45 Talk(9) + questgiver-flag restore (SetFlag(82, 2))
--   + player:GroupEventHappens(1144, creature); wp46 Talk(10) +
--   StopEscort (the C++ escort ends at the last waypoint; Go paths
--   repeat by default, so the halt keeps Willix from looping back
--   to node 0 — thrall precedent) + escort-latch clear.
-- - Engage (C++ JustEngagedWith): Talk(SAY_AGGRO1 2), ungated (the
--   C++ Talk targets the player; the bridge broadcasts — deviation,
--   documented).
-- - JustDied (razorfen_kraul.cpp:124-127): GetPlayerForEscort latch
--   (quest-1144 accept captures the escort player's GUID, mirroring
--   EscortAI Start's capture) + player:FailQuest(1144) via the
--   GetPlayersInWorld GUID scan (black-stalker precedent). No
--   escort-state gate in C++; FailQuest no-ops on a completed or
--   abandoned quest, so a stale latch is harmless.
-- Unmodeled (documented-only, no bridges):
-- - Waypoint 3's HandleEmoteCommand(EMOTE_STATE_POINT 29): no
--   SetEmoteState bridge (creature:PerformEmote is broadcast-only,
--   lua_creature_events.go:4195 — it sends SMSG_EMOTE but never
--   latches the UNIT_NPC_EMOTESTATE field C++ sets for state
--   emotes); the Talk leg ports, the pose stays out.
-- - The C++ Talk(SAY_*, player) calls target the escort player; the
--   bridge broadcasts (deviation, documented — same as the aggro
--   Talk above).

local ENTRY_WILLIX = 4508
local ENTRY_BOAR = 4514

local SAY_READY    = 0
local SAY_POINT    = 1
local SAY_AGGRO1   = 2
local SAY_BLUELEAF = 3
local SAY_DANGER   = 4
local SAY_BAD      = 5
local SAY_THINK    = 6
local SAY_SOON     = 7
local SAY_FINALY   = 8
local SAY_WIN      = 9
local SAY_END      = 10

local QUEST_WILLIX_THE_IMPORTER = 1144

-- GossipDef.h:36: GOSSIP_OPTION_QUESTGIVER = 2 (UNIT_NPC_FLAG_QUESTGIVER).
-- Field 82 = UNIT_NPC_FLAGS (UpdateFields.h:139).
local UNIT_NPC_FLAG_QUESTGIVER = 2

-- C++ WaypointReached boar summons (razorfen_kraul.cpp:77-83/96-101/
-- 119-124): TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 25s — Go
-- summon type 3 (C++ enum 4, minus-one convention per the
-- restless-soul port; ObjectDefines.h:57-67).
local BOAR_SUMMON_TYPE = 3
local BOAR_SUMMON_MS = 25000
local BOAR_SUMMONS = {
    [4]  = { 2137.66, 1843.98, 48.08, 1.54 },
    [14] = { 2078.91, 1704.54, 56.77, 1.54 },
    [43] = { 1956.43, 1596.97, 81.75, 1.54 },
}

local willixEscortPlayer = {}

-- C++ OnQuestAccept for quest 1144 (razorfen_kraul.cpp:52-59, C++ order:
-- Start, Talk, SetFaction): StartEscort + MoveWaypoint + Talk(READY) +
-- SetFaction(113).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_WILLIX_THE_IMPORTER then
        return
    end
    if player then
        creature:StartEscort(player)
    end
    creature:Talk(SAY_READY)
    creature:SetFaction(113)
    creature:MoveWaypoint()
    if player then
        willixEscortPlayer[creature:GetGUIDLow()] = player:GetGUID()
    end
end

-- C++ JustEngagedWith: ungated Talk(SAY_AGGRO1) (C++ targets the
-- player; the bridge broadcasts).
local function onEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO1)
end

-- C++ WaypointReached (razorfen_kraul.cpp:61-127): every arm requires
-- the escort player (GetPlayerForEscort() nil-guard at the top).
local function onReachWP(_, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 3 then
        -- C++ also HandleEmoteCommand(EMOTE_STATE_POINT 29) here —
        -- stays out (no SetEmoteState bridge, see header).
        creature:Talk(SAY_POINT)
    elseif wp == 4 or wp == 14 or wp == 43 then
        local pos = BOAR_SUMMONS[wp]
        local boar = creature:SummonCreature(ENTRY_BOAR, pos[1], pos[2], pos[3], pos[4], BOAR_SUMMON_TYPE, BOAR_SUMMON_MS)
        if boar then
            -- C++ JustSummoned (razorfen_kraul.cpp:135-138):
            -- summoned->AI()->AttackStart(me) — ordered at the summon
            -- site (ev19 fires on the summoned creature in Go).
            boar:AttackStart(creature)
        end
    elseif wp == 8 then
        creature:Talk(SAY_BLUELEAF)
    elseif wp == 9 then
        creature:Talk(SAY_DANGER)
    elseif wp == 13 then
        creature:Talk(SAY_BAD)
    elseif wp == 25 then
        creature:Talk(SAY_THINK)
    elseif wp == 31 then
        creature:Talk(SAY_SOON)
    elseif wp == 42 then
        creature:Talk(SAY_FINALY)
    elseif wp == 45 then
        creature:Talk(SAY_WIN)
        creature:SetFlag(82, UNIT_NPC_FLAG_QUESTGIVER)
        player:GroupEventHappens(QUEST_WILLIX_THE_IMPORTER, creature)
    elseif wp == 46 then
        creature:Talk(SAY_END)
        -- C++ escort ends at the last waypoint (EscortAI::UpdateAI
        -- runs MoveIdle + drops STATE_ESCORT_ESCORTING); Go paths
        -- repeat by default, so halt here (thrall precedent).
        creature:StopEscort()
        willixEscortPlayer[creature:GetGUIDLow()] = nil
    end
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    local playerGuid = willixEscortPlayer[guid]
    willixEscortPlayer[guid] = nil
    if not playerGuid then
        return
    end
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetGUID() == playerGuid then
            plr:FailQuest(QUEST_WILLIX_THE_IMPORTER)
            return
        end
    end
end

RegisterCreatureEvent(ENTRY_WILLIX, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WILLIX, 6, onReachWP)
RegisterCreatureEvent(ENTRY_WILLIX, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_WILLIX, 4, onDied)
