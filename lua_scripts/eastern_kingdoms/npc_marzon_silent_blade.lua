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
-- Eluna creature events: 1 OnEnterCombat (the C++ overrides no
-- other event with a bridged arm — melee is engine-driven).
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): Talk(SAY_MARZON_2 1) — the C++ JustEngagedWith
-- self-Talk arm (Talk takes a textId only; the line plays nearby).
-- The C++ JustEngagedWith summoner arm (IsSummon ->
-- ToTempSummon()->GetSummonerUnit() -> summoner->AI()->
-- AttackStart(who)) has no summon bridge — unmodeled (marzon is
-- summoned by the lescovar waypoint-16 phase arm, behind the
-- no-summon bridge). The C++ EnterEvadeMode DisappearAndDie pair
-- (marzon + summoner) has no despawn bridge — unmodeled. The
-- C++ Reset RestoreFaction arm has no faction bridge —
-- unmodeled. The C++ MovementInform POINT_MOTION_TYPE cross-AI
-- latch (CAST_AI into the lescovar AI's uiTimer/uiPhase) has no
-- movement bridge — unmodeled. Melee is engine-driven.
-- npc_tyrion (quest-434 OnQuestAccept -> FindNearestCreature(
-- NPC_TYRION_SPYBOT 8856) -> escort Start) awaits the quest-
-- accept / world-search / escort bridges — documented only.
-- npc_tyrion_spybot (escort AI: waypoint 1/5/17 Talk chains to
-- Tyrion 7766 / royal guard 1756 / Lescovar 1754 behind the
-- escort / world-search / cross-creature-Talk bridges; me->
-- UpdateEntry(7779) awaits the entry-update bridge; phase-10
-- lescovar escort start behind the escort bridge;
-- DisappearAndDie behind the despawn bridge) — documented only.
-- npc_lord_gregor_lescovar (escort AI: waypoint 14/16 chains;
-- SummonCreature(NPC_MARZON_BLADE 1755) behind the no-summon
-- bridge; phase-5/7 Marzon/lescovar cross-creature Talk + the
-- SetFaction(FACTION_MONSTER) traitor flip behind the no-
-- faction bridge; guards' GetCreatureListWithEntryInGrid +
-- DisappearAndDie behind the world-search / despawn bridges;
-- AreaExploredOrEventHappens(QUEST_THE_ATTACK 434) quest credit
-- behind the no-quest bridge) — documented only.

local NPC_MARZON_BLADE = 1755
local SAY_MARZON_2 = 1

-- C++ JustEngagedWith: Talk(SAY_MARZON_2). The summoner-AttackStart
-- gate (IsSummon / GetSummonerUnit / TYPEID_UNIT / IsAlive /
-- !IsInCombat) sits behind the no-summon bridge — no bridgeable
-- arm there, so the yell stands alone (C++-exact for the modeled
-- arm).
local function onEnterCombat(_, creature)
    creature:Talk(SAY_MARZON_2)
end

RegisterCreatureEvent(NPC_MARZON_BLADE, 1, onEnterCombat)
