-- Volkhan (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_volkhan.cpp
-- (boss_volkhan (CreatureScript) via GetHallsOfLightningAI<boss_volkhanAI>
-- (BossAI); npc_molten_golem (CreatureScript) via
-- GetHallsOfLightningAI<npc_molten_golemAI> (ScriptedAI);
-- achievement_shatter_resistant (AchievementCriteriaScript) —
-- all registered from inside AddSC_boss_volkhan(); loader decl 101 /
-- call 296 per northrend_script_loader.cpp — the FOURTH group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_ionar() (decl 100 / call 295), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_instance_halls_of_lightning() (decl 102 / call 297) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_ionar -> boss_volkhan) is followed).
-- Entry: 28587 Volkhan (halls_of_lightning.h NPC_VOLKHAN, line 41;
-- DATA_VOLKHAN = 1, line 33; GO_VOLKHAN_DOOR = 191325, line 50);
-- instance_halls_of_lightning.cpp binds { GO_VOLKHAN_DOOR,
-- DATA_VOLKHAN, DOOR_TYPE_PASSAGE } (line 27), OnCreatureCreate case
-- NPC_VOLKHAN (line 54), GetGuidData case DATA_VOLKHAN (line 107) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_volkhan"
-- hits boss_volkhan.cpp + the loader decl/call lines for
-- AddSC_boss_volkhan() only; whole-server-tree grep for
-- "npc_molten_golem" hits boss_volkhan.cpp only; whole-server-tree
-- grep for "achievement_shatter_resistant" hits boss_volkhan.cpp
-- only; zero sql/ hits for all three. No volkhan lua existed.
-- NPC_MOLTEN_GOLEM = 28695 / NPC_BRITTLE_GOLEM = 28681 /
-- NPC_VOLKHAN_ANVIL = 28823 / MAX_GOLEM = 2 /
-- DATA_SHATTER_RESISTANT = 2042 — local enums in boss_volkhan.cpp
-- (the acolyte_of_shadron / stormforged_lieutenant local-enum
-- precedent) — entry-verifiable, no registration proceeds for them
-- (bridge-blocked, see below).
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the SetPhase /
-- ScheduleEvent legs and the BossAI::JustEngagedWith passthrough
-- have no bridges — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 3) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — eighteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 4) (event 4; the DespawnGolem leg
-- (ObjectAccessor + summon-list — no bridge) and the _JustDied boss
-- bookkeeping passthrough have no bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28587):
-- boss_volkhan Reset / Initialize (m_bIsStriking / m_bHasTemper /
-- m_bCanShatterGolem / timers / phases / GolemsShattered /
-- m_uiHealthAmountModifier flag clears + _Reset() + DespawnGolem +
-- SetPhase(PHASE_INTRO) + EVENT_FORGE_CAST schedule — no bridges).
-- AttackStart (0.0f AddThreat + SetInCombatWith both ways + the
-- m_bHasTemper-gated MoveChase leg — no motion / threat bridges).
-- DespawnGolem (ObjectAccessor GUID loop over m_lGolemGUIDList,
-- DespawnOrUnsummon — no ObjectAccessor / summon-list bridges).
-- ShatterGolem (the brittle-golem-only CastSpell(SPELL_SHATTER
-- 52429) + GolemsShattered increment machine — no cast /
-- summon-list bridges).
-- JustSummoned (NPC_MOLTEN_GOLEM 28695 SummonList latch +
-- random-target MoveFollow + triggered SPELL_HEAT 52387 cast with
-- SetOriginalCaster(me) — no summon-list / cast / motion /
-- target-selection bridges).
-- MovementInform POINT_MOTION_TYPE / EVENT_FORGE_CAST (m_uiSummonPhase
-- 2 -> SetOrientation(2.29f) -> phase 3 — no motion bridge).
-- GetData(DATA_SHATTER_RESISTANT 2042) -> GolemsShattered (no
-- GetData bridge; consumer is achievement_shatter_resistant below).
-- UpdateAI event machine: EVENT_PAUSE (m_bIsStriking resume-chase +
-- flag clears, 3.5s); EVENT_SHATTERING_STOMP (m_uiHealthAmountModifier
-- >= 3 gate: Talk(SAY_STOMP 2) + DoCast(SPELL_SHATTERING_STOMP
-- 52237) + Talk(EMOTE_SHATTER 6) + m_bCanShatterGolem = true, 30s);
-- EVENT_SHATTER (ShatterGolem + 3s reschedule); EVENT_FORGE_CAST
-- (DoCast(SPELL_FORGE_VISUAL 52654), 15s, PHASE_INTRO only) — no
-- timer-event / cast bridges; the Talk arms 2, 6, 5 ride the
-- unmodeled machine.
-- UpdateAI forge summon-phase machine: the m_uiHealthAmountModifier
-- health check (HealthBelowPct(100-20*m_uiHealthAmountModifier) ->
-- Talk(SAY_FORGE 1) + interrupt + m_bHasTemper = true + phase 1);
-- phase 1 Talk(EMOTE_TO_ANVIL 5) + MovePoint(home position) ->
-- phase 2; phase 3 anvil TEMPER 52238 / TEMPER_DUMMY 52654 casts
-- (NPC_VOLKHAN_ANVIL 28823, 1000.0f) -> delay timer -> phase 4;
-- phase 4 SelectTarget(MaxThreat) MoveFollow -> phase 5; phase 5
-- MAX_GOLEM 2 triggered SPELL_SUMMON_MOLTEN_GOLEM 52405 casts on
-- the anvil + m_bIsStriking = true -> phase 0 — no timer / motion /
-- cast / target-selection bridges.
-- npc_molten_golem (NPC_MOLTEN_GOLEM 28695, local enum — no Talk
-- arms anywhere: entry-verifiable but bridge-blocked, NO
-- registration — the stormforged_lieutenant / npc_spark_of_ionar
-- no-bridgeable-arms precedent): Reset (EVENT_BLAST / EVENT_IMMOLATION
-- schedules — no timer bridge); AttackStart (no bridges);
-- DamageTaken (kill-shot -> UpdateEntry(NPC_BRITTLE_GOLEM 28681) +
-- SetHealth(1) + damage = 0 + RemoveAllAuras + AttackStop +
-- InterruptNonMeleeSpells + MotionMaster clear + m_bIsFrozen = true —
-- no DamageTaken bridge); SpellHit (SPELL_SHATTER 52429 dummy on a
-- brittle golem -> DespawnOrUnsummon — no SpellHit bridge);
-- UpdateAI (EVENT_BLAST: DoCast(SPELL_BLAST_WAVE 23113), 20s;
-- EVENT_IMMOLATION: DoCastVictim(SPELL_IMMOLATION_STRIKE 52433)
-- (C++ reschedules EVENT_BLAST at 5s — the C++ typo as written); m_bIsFrozen gate — no timer-event / cast bridges).
-- achievement_shatter_resistant (AchievementCriteriaScript
-- OnCheck: target->GetAI()->GetData(DATA_SHATTER_RESISTANT 2042) <
-- 5 — no GetData / achievement bridges; joins the
-- unmodeled-achievement queue).

local ENTRY_VOLKHAN = 28587

local SAY_AGGRO = 0
local SAY_SLAY = 3
local SAY_DEATH = 4

-- C++ JustEngagedWith: Talk(AGGRO) + SetPhase(PHASE_NORMAL) +
-- three ScheduleEvent legs + BossAI passthrough — only the Talk arm
-- is bridgeable.
local function volkhanEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function volkhanTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + DespawnGolem + _JustDied() — only
-- the Talk arm is bridgeable.
local function volkhanDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_VOLKHAN, 1, volkhanEnterCombat)
RegisterCreatureEvent(ENTRY_VOLKHAN, 3, volkhanTargetDied)
RegisterCreatureEvent(ENTRY_VOLKHAN, 4, volkhanDied)
