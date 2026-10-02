-- Anomalus (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_anomalus.cpp
-- (boss_anomalus (CreatureScript) via GetNexusAI<boss_anomalusAI>
-- (ScriptedAI, not BossAI — no instance-state BossAI plumbing of its
-- own, instance access via GetInstanceScript/SetBossState(DATA_
-- ANOMALUS)); npc_chaotic_rift (CreatureScript) via GetNexusAI<
-- npc_chaotic_riftAI> (ScriptedAI); achievement_chaos_theory
-- (AchievementCriteriaScript); AddSC_boss_anomalus() at end registers
-- all three; loader decl 79 / call 274 per northrend_script_loader.cpp
-- — the THIRD group of the "// The Nexus: Nexus" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_magus_telestra()
-- (decl 78 / call 273); the call after it is AddSC_boss_ormorok()
-- (decl 80 / call 275) — loader order confirmed this run; the
-- checkpoint sequence (magus_telestra -> anomalus) is followed).
-- Entry: 26763 Anomalus (nexus.h NPC_ANOMALUS, line 43;
-- instance_nexus.cpp OnCreatureCreate binds case NPC_ANOMALUS (line
-- 50) — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual. NPC_CHAOTIC_RIFT (26918) / NPC_CRAZED_MANA_WRAITH
-- (26746) exist only in the .cpp's local Adds enum — local-enum-only
-- like gothik's minions — entry-unverifiable, no registration, no lua
-- for npc_chaotic_rift.
-- Sole-source verified: whole-server-tree grep for "boss_anomalus"
-- hits boss_anomalus.cpp + the loader decl/call lines only; grep for
-- "npc_chaotic_rift" / "achievement_chaos_theory" hits
-- boss_anomalus.cpp only (registered from inside
-- AddSC_boss_anomalus, no separate loader lines); zero sql/ hits. No
-- anomalus lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. No timers in the ported arms; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the instance->
-- SetBossState(DATA_ANOMALUS, IN_PROGRESS) leg has no instance bridge
-- — tharon_ja precedent).
-- JustDied — Talk(SAY_DEATH 1) (event 4; the instance->
-- SetBossState(DATA_ANOMALUS, DONE) leg has no instance bridge —
-- tharon_ja precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (Phase/timers/GUID/chaosTheory reset) +
-- instance->SetBossState(DATA_ANOMALUS, NOT_STARTED) — no instance
-- bridge.
-- GetData(DATA_CHAOS_THEORY) — chaosTheory cross-AI data leg consumed
-- only by achievement_chaos_theory's OnCheck — no GetData bridge and
-- no achievement bridge.
-- SummonedCreatureDies — chaosTheory=false when a dead summon has
-- entry NPC_CHAOTIC_RIFT — no summon / creature-death dispatch
-- bridge.
-- UpdateAI — the home-position distance >60.0f EnterEvadeMode hack —
-- no position / EnterEvadeMode bridges; the SPELL_RIFT_SHIELD 47748
-- HasAura + uiChaoticRiftGUID ObjectAccessor::GetCreature + Rift->
-- isDead() -> RemoveAurasDueToSpell leg — no aura / GUID / instance
-- bridges; the Phase 0 + HealthBelowPct(50) -> Phase=1 + Talk(SAY_
-- SHIELD 3) + DoCast(me, SPELL_RIFT_SHIELD) + SummonCreature(NPC_
-- CHAOTIC_RIFT 26918, RiftLocation[urand(0,5)], TEMPSUMMON_TIMED_
-- DESPAWN_OUT_OF_COMBAT 1s) + Rift AttackStart(SelectTarget(Random,0))
-- + Talk(SAY_RIFT 2) machine — no health-pct / summon / random-target
-- SelectTarget / cross-AI AttackStart bridges; uiSparkTimer —
-- SelectTarget(Random, 0) -> DoCast(SPELL_SPARK 47751/57062), 5s init,
-- 5s repeat — no random-target SelectTarget bridge (cairne/kazzak via
-- anubrekhan EVENT_IMPALE precedent).
-- npc_chaotic_rift — entry-unverifiable (26918, local-enum-only) — no
-- registration, no lua; its arms are documented here only:
-- SetCombatMovement(false); Reset — SetDisplayId(Modelid2) +
-- DoCast(me, SPELL_ARCANEFORM 48019) — no model-id / no instance-less
-- self-cast-on-reset surface; UpdateAI uiChaoticEnergyBurstTimer —
-- SelectTarget(Random, 0) -> DoCast(SPELL_CHAOTIC_ENERGY_BURST 47688 /
-- charged 47737 when Anomalus via instance->GetGuidData(DATA_ANOMALUS)
-- HasAura(SPELL_RIFT_SHIELD)), 1s init, 1s repeat — no instance /
-- cross-creature / random-target bridges; UpdateAI
-- uiSummonCrazedManaWraithTimer — SummonCreature(NPC_CRAZED_MANA_
-- WRAITH 26746 local-enum-only entry-unverifiable) + AttackStart(
-- SelectTarget(Random, 0)) + 5s repeat when Anomalus shielded else 10s
-- — no summon / instance / random-target bridges.
-- achievement_chaos_theory — OnCheck: Anomalus->AI()->GetData(DATA_
-- CHAOS_THEORY) != 0 — no achievement bridge (kelthuzad/thaddius
-- precedent) + the GetData leg is cross-AI — joins the
-- no-achievement-bridge queue.

local ENTRY_ANOMALUS = 26763

local SAY_AGGRO = 0
local SAY_DEATH = 1

-- C++ JustEngagedWith: Talk(SAY_AGGRO) (the instance->SetBossState
-- (DATA_ANOMALUS, IN_PROGRESS) leg has no instance bridge — tharon_ja
-- precedent).
local function anomalusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ JustDied: Talk(SAY_DEATH) (the instance->SetBossState(DATA_
-- ANOMALUS, DONE) leg has no instance bridge — tharon_ja precedent).
local function anomalusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ANOMALUS, 1, anomalusEnterCombat)
RegisterCreatureEvent(ENTRY_ANOMALUS, 4, anomalusDied)
