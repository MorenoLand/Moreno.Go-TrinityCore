-- Ley-Guardian Eregos (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_eregos.cpp
-- (boss_eregos (CreatureScript) via GetOculusAI<boss_eregosAI>
-- (BossAI, DATA_EREGOS); spell_eregos_planar_shift
-- (SpellScriptLoader -> AuraScript); achievement_gen_eregos_void
-- x3 (AchievementCriteriaScript: achievement_ruby_void /
-- achievement_emerald_void / achievement_amber_void); all
-- registered from inside AddSC_boss_eregos(), no separate loader
-- lines; loader decl 87 / call 282 per
-- northrend_script_loader.cpp — the FOURTH group of the
-- "// The Nexus: The Oculus" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_varos() (decl 86 / call 281);
-- the calls after it are AddSC_instance_oculus() (decl 88 /
-- call 283), AddSC_oculus() (decl 89 / call 284) — loader order
-- confirmed this run; the checkpoint sequence (varos ->
-- eregos) is followed).
-- Entry: 27656 Ley-Guardian Eregos (oculus.h NPC_EREGOS, line 44;
-- DATA_EREGOS = 3, line 34; instance_oculus.cpp OnCreatureCreate
-- binds case NPC_EREGOS (line 72) and GetGuidData cases
-- DATA_EREGOS (lines 209 / 249) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_eregos"
-- hits boss_eregos.cpp + the loader decl/call lines only; grep
-- for "spell_eregos_planar_shift" hits boss_eregos.cpp only;
-- grep for "achievement_gen_eregos_void" /
-- "achievement_ruby_void" / "achievement_emerald_void" /
-- "achievement_amber_void" hits boss_eregos.cpp only; zero sql/
-- hits. No eregos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied. No timers in the ported arms; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg has no
-- bridge — tharon_ja precedent; the drake-vehicle FindNearest-
-- Creature achievement-void legs (NPC_RUBY_DRAKE_VEHICLE 27756 /
-- NPC_EMERALD_DRAKE_VEHICLE 27692 / NPC_AMBER_DRAKE_VEHICLE
-- 27755, 500.0f) have no instance / creature-search bridge —
-- documented-only; GetData for the void booleans is consumed by
-- the achievement scripts below, also documented-only).
-- KilledUnit — Talk(SAY_KILL 3) (event 3 — C++-GATED
-- who->GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad
-- player-gate variant — tenth ported variant overall, seventh
-- identical to nalorakk / kelthuzad / gothik / thaddius /
-- keristrasza / urom).
-- JustDied — Talk(SAY_DEATH 4) (event 4; the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (_phase = PHASE_NORMAL, all three void
-- booleans = true) + _Reset() + DoAction(
-- ACTION_SET_NORMAL_EVENTS) — no instance bridge.
-- DoAction — events.SetPhase(PHASE_NORMAL) + ScheduleEvent x4
-- (EVENT_ARCANE_BARRAGE 3s/10s, EVENT_ARCANE_VOLLEY 10s/25s,
-- EVENT_ENRAGED_ASSAULT 35s/50s, EVENT_SUMMON_LEY_WHELP 15s/
-- 30s, all PHASE_NORMAL) — no timer-event / phase bridge.
-- UpdateAI — the arcane event machine: EVENT_ARCANE_BARRAGE
-- DoCastVictim(SPELL_ARCANE_BARRAGE 50804), EVENT_ARCANE_VOLLEY
-- DoCastAOE(SPELL_ARCANE_VOLLEY 51153), EVENT_ENRAGED_ASSAULT
-- Talk(SAY_ENRAGE 2) + DoCast(SPELL_ENRAGED_ASSAULT 51170),
-- EVENT_SUMMON_LEY_WHELP DoCast(SPELL_SUMMON_LEY_WHELP 51175)
-- x3 (all re-scheduled per-event); the UNIT_STATE_CASTING
-- gates have no bridge — no timer-event / cast bridges.
-- DamageTaken — heroic-only phase-shift machine: health
-- 60%..20% -> PHASE_FIRST_PLANAR (or < 20% ->
-- PHASE_SECOND_PLANAR), events.Reset() + Talk(SAY_SHIELD 5) +
-- DoCast(SPELL_PLANAR_SHIFT 51162) + summons.DespawnAll() +
-- DoCast(SPELL_PLANAR_ANOMALIES 57959) x6 — no health /
-- timer-event / cast / summon bridges.
-- JustSummoned — BossAI::JustSummoned + NPC_PLANAR_ANOMALY
-- 30879: CombatStop + REACT_PASSIVE + MoveRandom(100.0f) — no
-- motion / react-state bridges. SummonedCreatureDespawn —
-- NPC_PLANAR_ANOMALY: CastSpell(SPELL_PLANAR_BLAST 57976)
-- self — no cast bridge.
-- spell_eregos_planar_shift — AuraScript AfterEffectRemove
-- (EFFECT_0 SPELL_AURA_SCHOOL_IMMUNITY): target ToCreature
-- -> AI()->DoAction(ACTION_SET_NORMAL_EVENTS) (the
-- planar-shift end re-arms normal events) — no AuraScript
-- bridge (the keristrasza spell_intense_cold precedent); joins
-- the no-AuraScript-bridge queue.
-- achievement_gen_eregos_void x3 — OnCheck: target->GetAI()->
-- GetData(DATA_RUBY_VOID / DATA_EMERALD_VOID / DATA_AMBER_VOID)
-- (the drake-void achievements 2044 / 2045 / 2046) — no
-- achievement bridge (the kelthuzad / thaddius precedent);
-- joins the unmodeled-achievement queue.

local ENTRY_EREGOS = 27656

local SAY_AGGRO = 1
local SAY_KILL = 3
local SAY_DEATH = 4

-- C++ JustEngagedWith: BossAI::JustEngagedWith(who) +
-- Talk(SAY_AGGRO) + drake-vehicle achievement-void legs (no
-- bridges) — the Talk arm is unconditional.
local function eregosEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: gated on who->GetTypeId() == TYPEID_PLAYER —
-- the nalorakk / kelthuzad player-gate variant.
local function eregosTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) + _JustDied() (the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent).
local function eregosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_EREGOS, 1, eregosEnterCombat)
RegisterCreatureEvent(ENTRY_EREGOS, 3, eregosTargetDied)
RegisterCreatureEvent(ENTRY_EREGOS, 4, eregosDied)
