-- Azuremyst Isle: Magwin — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (class npc_magwin : CreatureScript("npc_magwin");
-- struct npc_magwinAI : public EscortAI).
-- Entry: 17312 Magwin (TDB335.26091 creature_template: faction 231,
-- ScriptName 'npc_magwin'; 1 spawn, map 530, -4781.36/-11054.6/2.47597,
-- guid 62090; creature_template_addon path_id 0, no creature_addon row).
-- Quest: 9528 'A Cry For Help' (quest_template, level 10).
-- Cowlen: 17311 'Cowlen' (creature_template, faction 80).
-- creature_text rows for 17312 (TDB): groups 0-5, one row each —
-- 0 'Our house is this way, through the thicket.' (SAY_START),
-- 1 'Help me!' (SAY_AGGRO), 2 'My poor family. Everything has been
-- destroyed.' (SAY_PROGRESS), 3 'Father! Father! You're alive!'
-- (SAY_END1), 4 'You can thank $n for getting me back here safely,
-- father.' (SAY_END2), 5 '%s hugs her father.' (EMOTE_HUG).
-- Constants below are zone_azuremyst_isle.cpp:328-341 verbatim.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 6 OnReachWP,
-- 23 OnReset, 31 OnQuestAccept. No RegisterLuaBoss needed: the
-- loader walks lua_scripts/ and entry-keyed RegisterCreatureEvent
-- hooks fire regardless (ruul precedent).
-- Ported arms (C++ order):
-- - Reset (zone_azuremyst_isle.cpp:353-356): _events.Reset() — the
--   GUID-keyed CreateLuaEvent timers are cancelled and the quest-
--   player latch cleared (valiant timer-table idiom). No IsDead
--   guard: clearing timers is the correct death twin (C++ destroys
--   the EventMap with the AI), unlike the marli cage-GO leg.
-- - JustEngagedWith (:359-362): Talk(SAY_AGGRO 1, who) — ev1 passes
--   the attacker as the third arg (maiden/moroes precedent).
-- - OnQuestAccept (:364-370): quest 9528 -> latch the player GUID
--   + schedule EVENT_ACCEPT_QUEST (2s).
-- - UpdateEscortAI EventMap (:397-431), as CreateLuaEvent pumps:
--   EVENT_ACCEPT_QUEST — Talk(SAY_START 0, player) if the player
--   is still reachable, then SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, SharedDefines.h:240) +
--   schedule EVENT_START_ESCORT (1s); EVENT_START_ESCORT —
--   EscortAI::Start + MoveWaypoint() if the player is reachable,
--   then schedule EVENT_STAND (2s); EVENT_STAND — SetByteValue
--   stand-state 0 (SetStandState(0) twin, ruul wp0 precedent);
--   EVENT_TALK_END — Talk(SAY_END1 3, player) if reachable, then
--   schedule EVENT_COWLEN_TALK (2s); EVENT_COWLEN_TALK —
--   FindNearestCreature(NPC_COWLEN, 50) -> cowlen:Talk(SAY_COWLEN 0).
--   The C++ ObjectAccessor::GetPlayer(*me, _player) null-guard is
--   the findQuestPlayer twin: the stored player GUID is re-fetched
--   via GetPlayersInRange(100) (etymidian precedent); a logged-out
--   player no-ops the Talk/Start legs while the unconditional
--   SetFaction / schedule legs still run, exactly as C++.
-- - WaypointReached (:373-395): the escort-player nil-guard at the
--   top (GetEscortPlayer); wp17 Talk(SAY_PROGRESS 2, player);
--   wp28 player:GroupEventHappens(9528) + schedule EVENT_TALK_END
--   (2s) + SetRun(true) (SetWalk(false) twin, old_hillsbrad
--   precedent); wp29 FindNearestCreature(NPC_COWLEN, 50) ->
--   Talk(EMOTE_HUG 5, cowlen) if found, then Talk(SAY_END2 4,
--   player) + StopEscort (the C++ escort ends at the last handled
--   waypoint; Go paths repeat by default — ruul/willix/thrall
--   precedent). The escort walk itself rides MoveWaypoint + ev6
--   over the live-DB waypoint path (loadCreaturePathID: addon
--   path_id 0 -> spawn guid 62090; the TDB 26091 dump carries no
--   waypoint_data rows for 17312 or 62090 — same as ruul 12818,
--   whose walk is live-DB-driven).
-- No JustDied override exists in C++ (no FailQuest leg to port).

local ENTRY_MAGWIN = 17312

local NPC_COWLEN = 17311

local QUEST_A_CRY_FOR_HELP = 9528

local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

local SAY_START = 0
local SAY_AGGRO = 1
local SAY_PROGRESS = 2
local SAY_END1 = 3
local SAY_END2 = 4
local EMOTE_HUG = 5
local SAY_COWLEN = 0

local timers = {}
local questPlayer = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    questPlayer[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ ObjectAccessor::GetPlayer(*me, _player) twin: re-fetch the
-- latched quest player from the live player list; nil when the
-- player logged out or left range (etymidian GetPlayersInRange
-- precedent).
local function findQuestPlayer(creature, guid)
    local wanted = questPlayer[guid]
    if not wanted then
        return nil
    end
    for _, player in ipairs(creature:GetPlayersInRange(100)) do
        if player:GetGUID() == wanted then
            return player
        end
    end
    return nil
end

-- C++ npc_magwinAI::Reset (zone_azuremyst_isle.cpp:353-356).
local function magwinReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_magwinAI::JustEngagedWith (:359-362).
local function magwinEnterCombat(event, creature, target)
    if target then
        creature:Talk(SAY_AGGRO, target)
    else
        creature:Talk(SAY_AGGRO)
    end
end

-- C++ EVENT_STAND (:423-425): clear the kneel stand state.
local function onStand(creature, guid)
    creature:SetStandState(0)
end

-- C++ EVENT_COWLEN_TALK (:427-431).
local function onCowlenTalk(creature, guid)
    local cowlen = creature:FindNearestCreature(NPC_COWLEN, 50.0)
    if cowlen then
        cowlen:Talk(SAY_COWLEN)
    end
end

-- C++ EVENT_TALK_END (:416-421).
local function onTalkEnd(creature, guid)
    local player = findQuestPlayer(creature, guid)
    if player then
        creature:Talk(SAY_END1, player)
    end
    schedule(guid, "cowlen_talk", 2000, function() onCowlenTalk(creature, guid) end)
end

-- C++ EVENT_START_ESCORT (:407-414).
local function onStartEscort(creature, guid)
    local player = findQuestPlayer(creature, guid)
    if player then
        creature:StartEscort(player)
        creature:MoveWaypoint()
    end
    schedule(guid, "stand", 2000, function() onStand(creature, guid) end)
end

-- C++ EVENT_ACCEPT_QUEST (:400-406).
local function onAcceptQuest(creature, guid)
    local player = findQuestPlayer(creature, guid)
    if player then
        creature:Talk(SAY_START, player)
    end
    creature:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_PASSIVE)
    schedule(guid, "start_escort", 1000, function() onStartEscort(creature, guid) end)
end

-- C++ npc_magwinAI::OnQuestAccept (:364-370).
local function magwinQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_A_CRY_FOR_HELP then
        return
    end
    local guid = creature:GetGUIDLow()
    questPlayer[guid] = player:GetGUID()
    schedule(guid, "accept_quest", 2000, function() onAcceptQuest(creature, guid) end)
end

-- C++ npc_magwinAI::WaypointReached (:373-395).
local function onReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local guid = creature:GetGUIDLow()
    local wp = creature:GetCurrentWaypointId()
    if wp == 17 then
        creature:Talk(SAY_PROGRESS, player)
    elseif wp == 28 then
        player:GroupEventHappens(QUEST_A_CRY_FOR_HELP, creature)
        schedule(guid, "talk_end", 2000, function() onTalkEnd(creature, guid) end)
        creature:SetWalk(false)
    elseif wp == 29 then
        local cowlen = creature:FindNearestCreature(NPC_COWLEN, 50.0)
        if cowlen then
            creature:Talk(EMOTE_HUG, cowlen)
        end
        creature:Talk(SAY_END2, player)
        creature:StopEscort()
    end
end

RegisterCreatureEvent(ENTRY_MAGWIN, 1, magwinEnterCombat)
RegisterCreatureEvent(ENTRY_MAGWIN, 4, magwinReset)
RegisterCreatureEvent(ENTRY_MAGWIN, 6, onReachWP)
RegisterCreatureEvent(ENTRY_MAGWIN, 23, magwinReset)
RegisterCreatureEvent(ENTRY_MAGWIN, 31, magwinQuestAccept)
