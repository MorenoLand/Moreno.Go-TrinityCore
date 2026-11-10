-- Marzon the Silent Blade (Stormwind City) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_stormwind_city.cpp
-- (npc_marzon_silent_bladeAI — JustEngagedWith aggro yell only).
-- Zone-script unit per eastern_kingdoms_script_loader.cpp order
-- (isle_of_queldanas done; stormwind_city: npc_tyrion
-- documented-only, npc_tyrion_spybot documented-only, npc_lord_
-- gregor_lescovar documented-only, npc_marzon_silent_blade
-- ported).
-- Entry (LordGregorLescovar enum, verifiable from the C++ sources):
-- 1755 (NPC_MARZON_BLADE). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat (evade
-- self-despawn, ported below) — melee is engine-driven).
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): Talk(SAY_MARZON_2 1) — the C++ JustEngagedWith
-- self-Talk arm (Talk takes a textId only; the line plays nearby).
-- The C++ JustEngagedWith summoner arm (IsSummon ->
-- ToTempSummon()->GetSummonerUnit() -> summoner->AI()->
-- AttackStart(who)) is PORTED below: IsSummon, GetSummonerUnit
-- (lua_creature_events.go:5516) and AttackStart (:6831,
-- EngageWithTarget twin) are all bridged now — the "no bridges"
-- claim in earlier headers is stale. The C++ EnterEvadeMode
-- DisappearAndDie pair is PORTED below (vazruden/muru
-- convention: DisappearAndDie -> immediate creature:Despawn()),
-- self AND summoner legs (GetSummonerUnit :5516). The C++
-- Reset RestoreFaction arm stays unmodeled: Marzon's faction only
-- changes in the lescovar phase-7 flip, and the quest chain ends
-- in a despawn, so no reset can observe it (SetFaction itself is
-- bridged — lua_creature_events.go:1125). The C++ MovementInform
-- POINT_MOTION_TYPE cross-AI latch (CAST_AI into the lescovar
-- AI's uiTimer/uiPhase) is PORTED below: event 28
-- (CreatureEventOnMovementInform, runtime.go:389, args (event,
-- creature, type, id), POINT = 8) sends ACTION_MARZON_ARRIVED
-- to the summoner via DoAction (sironas precedent — the same
-- constant value is defined in npc_lord_gregor_lescovar.lua,
-- whose ev38 handler arms phase 5 + starts the lescovar pump).
-- The IsAIEnabled gate has no bridge; the summoner is the
-- lescovar AI by construction. Melee is engine-driven.
-- npc_tyrion (quest-434 OnQuestAccept -> FindNearestCreature(
-- NPC_TYRION_SPYBOT 8856) -> escort Start) is PORTED in
-- npc_tyrion.lua. npc_tyrion_spybot (escort AI: waypoint 1/5/17
-- Talk chains, UpdateEntry(7779), phase-10 lescovar escort
-- start) is PORTED in npc_tyrion_spybot.lua.
-- npc_lord_gregor_lescovar (escort AI: waypoint 14/16 chains;
-- SummonCreature(NPC_MARZON_BLADE 1755); phase-5/7 Marzon/
-- lescovar cross-creature Talk + the SetFaction(FACTION_MONSTER)
-- traitor flip; guards' GetCreaturesInGrid + Despawn;
-- AreaExploredOrEventHappens(QUEST_THE_ATTACK 434) quest credit)
-- is PORTED in npc_lord_gregor_lescovar.lua.

local NPC_MARZON_BLADE = 1755
local SAY_MARZON_2 = 1

local ACTION_MARZON_ARRIVED = 1

local POINT_MOTION_TYPE = 8

-- C++ JustEngagedWith (zone_stormwind_city.cpp:230-251): Talk(
-- SAY_MARZON_2), then the summoner arm — if IsSummon, the live
-- creature summoner not already in combat attacks the same target
-- (GetSummonerUnit :5516 returns the luaMotionCreature for a
-- creature summoner; the TYPEID_UNIT gate is GetObjectType() ==
-- "Creature"; AttackStart :6831). The event-1 target is the
-- engaging unit (phase-hunter precedent); fall back to the victim.
local function onEnterCombat(_, creature, target)
    creature:Talk(SAY_MARZON_2)
    if creature:IsSummon() then
        local summoner = creature:GetSummonerUnit()
        if summoner and summoner:GetObjectType() == "Creature" and summoner:IsAlive() and not summoner:IsInCombat() then
            local who = target or creature:GetVictim()
            if who then
                summoner:AttackStart(who)
            end
        end
    end
end

local function onLeaveCombat(_, creature)
    -- C++ npc_marzon_silent_bladeAI::EnterEvadeMode
    -- (zone_stormwind_city.cpp:253-267): me->DisappearAndDie()
    -- first, then the IsSummon summoner leg (live creature
    -- summoner DisappearAndDie) — both immediate Despawn() calls
    -- (vazruden/muru convention).
    creature:Despawn()
    if creature:IsSummon() then
        local summoner = creature:GetSummonerUnit()
        if summoner and summoner:GetObjectType() == "Creature" and summoner:IsAlive() then
            summoner:Despawn()
        end
    end
end

-- C++ MovementInform (zone_stormwind_city.cpp:268-301): only
-- POINT_MOTION_TYPE arrivals count; if summoned, the live
-- creature summoner gets the lescovar phase latch (uiTimer =
-- 2000, uiPhase = 5) — carried here as DoAction(
-- ACTION_MARZON_ARRIVED) into the summoner (see header).
local function onMovementInform(_, creature, moveType, _)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if creature:IsSummon() then
        local summoner = creature:GetSummonerUnit()
        if summoner and summoner:GetObjectType() == "Creature" then
            summoner:DoAction(ACTION_MARZON_ARRIVED)
        end
    end
end

RegisterCreatureEvent(NPC_MARZON_BLADE, 1, onEnterCombat)
RegisterCreatureEvent(NPC_MARZON_BLADE, 2, onLeaveCombat)
RegisterCreatureEvent(NPC_MARZON_BLADE, 28, onMovementInform)
