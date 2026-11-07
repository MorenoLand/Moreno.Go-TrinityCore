-- Outland: Earthmender Wilda (Coilskar Cistern escort) — Lua port of
-- src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (class npc_earthmender_wilda : CreatureScript("npc_earthmender_wilda");
-- struct npc_earthmender_wildaAI : public EscortAI).
-- Entry: 21027 (Wowhead retail + TBC Classic NPC pages agree on
-- npc=21027; no NPC_WILDA constant exists in the C++ tree — the
-- creature_template ScriptName binding is DB-side).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 19
-- OnJustSummonedCreature, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset.
-- Ported arms:
-- - OnQuestAccept (zone_shadowmoon_valley.cpp, event 31): quest id
--   10451 (QUEST_ESCAPE_COILSCAR) -> Talk(SAY_WIL_START 0) +
--   SetFaction(1726 FACTION_EARTHEN_RING, SharedDefines.h:254),
--   C++ order: Talk, SetFaction, Start. Start is PORTED via
--   creature:StartEscort(player) + creature:MoveWaypoint()
--   (the C++ Start(false, false, ...) walk/instant-respawn
--   flags have no Go twin — the walk rides the per-node DB
--   move_type).
-- - WaypointReached (event 6, DB point via
--   creature:GetCurrentWaypointId()): Talk arms at 13/15/39/50,
--   DoRandomSay (Talk SAY_WIL_PROGRESS2 3) at 19/26/33/37/43,
--   DoSpawnAssassin (NPC_COILSKAR_ASSASSIN 21044) at
--   13/14/20/27/34/38/44, waypoint-50 GroupEventHappens quest
--   credit. Every arm requires the escort player (the C++
--   GetPlayerForEscort() nil-guard at the top).
-- - JustSummoned (event 19): an assassin attacks wilda
--   (summoned:Attack(me, true) — the C++ AttackStart twin).
--   DoSpawnAssassin summons at the close point (Go
--   SummonCreature 0,0,0 fallback) where C++ picks a random
--   point within 15yd ("unknown where they actually appear"
--   per the C++ comment); type 3
--   (TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) + 5000ms.
-- - JustEngagedWith: rand32()%5 gate + non-player target gate +
--   urand(0,1) -> Talk(SAY_WIL_AGGRO 1); the C++ Talk targets the
--   attacker, the bridge broadcasts (willix precedent).
-- - UpdateAI healing leg: at-or-below 30% health, 15s timer ->
--   DoCast(me, SPELL_HEALING_WAVE 12491), non-triggered; the timer
--   freezes above 30% (C++ never resets it outside Reset) and the
--   first sub-30% tick casts immediately (timer starts at 0).
--   The pump runs only between enter-combat and
--   leave-combat/died/reset, matching the C++ UpdateVictim gate.
-- Deliberate deviations: the waypoint_data rows for wilda's
-- path live in the deployment world DB (the repo world.db
-- placeholder carries none — loadCreaturePathID falls back
-- to the spawn GUID, so a live DB without the rows leaves
-- MoveWaypoint a no-op); the waypoint walk loops the DB path
-- at its end (Go NextIdx wraps — no end-of-path idle twin).

local ENTRY_WILDA = 21027
local NPC_COILSKAR_ASSASSIN = 21044

local SAY_WIL_START = 0
local SAY_WIL_AGGRO = 1
local SAY_WIL_PROGRESS1 = 2
local SAY_WIL_PROGRESS2 = 3
local SAY_WIL_FIND_EXIT = 4
local SAY_WIL_JUST_AHEAD = 5
local SAY_WIL_END = 6

local QUEST_ESCAPE_COILSCAR = 10451
local FACTION_EARTHEN_RING = 1726
local SPELL_HEALING_WAVE = 12491

local healTimer = {}

local function cancelHeal(guid)
    local id = healTimer[guid]
    if id then
        RemoveEventById(id)
        healTimer[guid] = nil
    end
end

-- C++ OnQuestAccept (zone_shadowmoon_valley.cpp, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(1726) +
-- StartEscort(player) + MoveWaypoint().
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ESCAPE_COILSCAR then
        return
    end
    creature:Talk(SAY_WIL_START)
    creature:SetFaction(FACTION_EARTHEN_RING)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ DoSpawnAssassin (zone_shadowmoon_valley.cpp:585-589):
-- DoSummon(NPC_COILSKAR_ASSASSIN, me, 15.0f, 5s,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) — type 3, 5000ms.
local function doSpawnAssassin(creature)
    creature:SummonCreature(NPC_COILSKAR_ASSASSIN, 0, 0, 0, 0, 3, 5000)
end

-- C++ WaypointReached (zone_shadowmoon_valley.cpp:515-571):
-- the waypoint id is the DB point (GetCurrentWaypointId).
local function onReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 13 then
        creature:Talk(SAY_WIL_PROGRESS1)
        doSpawnAssassin(creature)
    elseif wp == 14 then
        doSpawnAssassin(creature)
    elseif wp == 15 then
        creature:Talk(SAY_WIL_FIND_EXIT)
    elseif wp == 19 or wp == 26 or wp == 33 or wp == 37 or wp == 43 then
        creature:Talk(SAY_WIL_PROGRESS2)
    elseif wp == 20 or wp == 27 or wp == 34 or wp == 38 or wp == 44 then
        doSpawnAssassin(creature)
    elseif wp == 39 then
        creature:Talk(SAY_WIL_JUST_AHEAD)
    elseif wp == 50 then
        creature:Talk(SAY_WIL_END)
        player:GroupEventHappens(QUEST_ESCAPE_COILSCAR, creature)
    end
end

-- C++ JustSummoned (zone_shadowmoon_valley.cpp:573-577): an
-- assassin attacks wilda.
local function onJustSummoned(event, creature, summoned)
    if summoned and summoned:GetEntry() == NPC_COILSKAR_ASSASSIN then
        summoned:Attack(creature, true)
    end
end

-- C++ JustEngagedWith: rand32()%5 gate, then only a non-player
-- attacker, then urand(0,1) -> Talk(1). The heal pump starts on
-- every enter-combat, ungated by the Talk gates.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    if not healTimer[guid] then
        local remaining = 0
        local function tick()
            if creature:IsDead() then
                healTimer[guid] = nil
                return
            end
            if creature:GetHealthPct() <= 30 then
                if remaining <= 0 then
                    creature:CastSpell(creature, SPELL_HEALING_WAVE)
                    remaining = 15000
                else
                    remaining = remaining - 1000
                end
            end
            healTimer[guid] = CreateLuaEvent(tick, 1000)
        end
        healTimer[guid] = CreateLuaEvent(tick, 1000)
    end
    if math.random(0, 4) ~= 0 then
        return
    end
    if not target or target:GetObjectType() == "Player" then
        return
    end
    if math.random(0, 1) == 1 then
        creature:Talk(SAY_WIL_AGGRO)
    end
end

local function onLeaveCombat(event, creature)
    cancelHeal(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WILDA, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_WILDA, 6, onReachWP)
RegisterCreatureEvent(ENTRY_WILDA, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_WILDA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WILDA, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_WILDA, 4, onLeaveCombat)
RegisterCreatureEvent(ENTRY_WILDA, 23, onLeaveCombat)
